package fascicolo

// Le proposte di rimozione (Blocco 8, B8.5; addendum A4.4, D27, D31).
//
// «Che cosa e' sparito dalla distinta?» ha senso solo rispetto a UN file scelto da una persona, lo STEP
// strutturale del prodotto finito, e solo se quel file e' stato letto per intero: una mancanza in una
// lettura parziale non e' un'informazione. Qui si confrontano gli archi della working che pendono dal
// prodotto con quelli del suo STEP strutturale, e ogni arco che il file non contiene piu' diventa una
// rimozione_proposta. La proposta non toglie niente: l'arco lo toglie chi la accetta.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// AggiornaRimozioni ricalcola le rimozioni proposte dallo STEP strutturale del prodotto finito p.
//
// Le condizioni, tutte (A4.4):
//   - lo STEP strutturale c'e' ed e' corrente;
//   - la sua analisi corrente e' completa (v_step_prodotto dice presente_analizzato, cioe'
//     struttura_completa sui fatti con la chiave di analizzatore_corrente);
//   - ogni nodo del file ha un componente, oppure e' un pezzo nuovo con il suo codice, oppure e' stato
//     scartato da una persona. Un nodo senza codice ancora aperto sospende il calcolo: potrebbe essere
//     proprio il pezzo che sembra sparito.
//
// Le rimozioni aperte che non valgono piu' (l'arco non e' piu' nella working, o il file lo contiene di
// nuovo) si chiudono con la nota. Una rimozione gia' decisa non si riapre.
func AggiornaRimozioni(ctx context.Context, q *db.Queries, thread uuid.UUID, p db.Componente) (EsitoRimozioni, error) {
	es := EsitoRimozioni{Prodotto: p.Codice}
	if !p.StepStrutturaleID.Valid {
		es.Sospese = "nessuno STEP strutturale scelto"
		return es, nil
	}
	d, err := q.GetDocumento(ctx, p.StepStrutturaleID.UUID)
	if err != nil {
		return es, err
	}
	if d.SostituitoDa.Valid {
		es.Sospese = "lo STEP strutturale è stato sostituito: si sceglie il nuovo riferimento"
		return es, nil
	}
	sp, err := q.GetStepProdotto(ctx, p.ComponenteID)
	if err != nil {
		return es, err
	}
	if sp.Esito != StepAnalizzato {
		es.Sospese = "lo STEP strutturale non ha una lettura completa: " + EtichettaStep(sp.Esito)
		if sp.MotivoParziale.Valid {
			es.Sospese += " (" + sp.MotivoParziale.String + ")"
		}
		return es, nil
	}
	af, err := q.GetAnalisiCorrente(ctx, d.Sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		es.Sospese = "nessuna analisi corrente dello STEP strutturale"
		return es, nil
	}
	if err != nil {
		return es, err
	}
	_, st, ok := struttura(af.Fatti)
	if !ok || len(st.Radici) != 1 {
		es.Sospese = "la struttura dello STEP strutturale non si legge"
		return es, nil
	}

	w, err := leggiWorking(ctx, q, thread)
	if err != nil {
		return es, err
	}
	proposte, err := q.ListComponenteProposteStessoFile(ctx, db.ListComponenteProposteStessoFileParams{ThreadID: thread, Sha256: d.Sha256})
	if err != nil {
		return es, err
	}
	// Per ogni nodo, la riga che conta: una decisa vale piu' di una aperta (lo stesso contenuto ha un
	// solo portatore per RFQ, ma una riga vecchia di un altro allegato puo' esserci ancora).
	perChiave := map[string]db.ComponenteProposta{}
	for _, r := range proposte {
		if e, c := perChiave[r.Chiave]; !c || (e.Stato == db.StatoPropostaAperta && r.Stato != db.StatoPropostaAperta) {
			perChiave[r.Chiave] = r
		}
	}
	componenteDi := map[string]uuid.UUID{st.Radici[0]: p.ComponenteID} // la dichiarazione: la radice e' il prodotto
	var senzaCodice []string
	for _, n := range st.Nodi {
		if n.Chiave == st.Radici[0] {
			continue
		}
		r, c := perChiave[n.Chiave]
		switch {
		case !c:
			senzaCodice = append(senzaCodice, n.NomeGrezzo)
		case r.Stato == db.StatoPropostaScartata:
			// una persona ha detto che non e' un pezzo della distinta
		case (r.Stato == db.StatoPropostaConfermata || r.Stato == db.StatoPropostaDuplicato) && r.ComponenteID.Valid:
			componenteDi[n.Chiave] = r.ComponenteID.UUID
		case strings.TrimSpace(r.Codice.String) == "":
			senzaCodice = append(senzaCodice, n.NomeGrezzo)
		default:
			if x, c := w.PerCodice[strings.ToUpper(strings.TrimSpace(r.Codice.String))]; c && x.ArchiviatoIl == nil {
				componenteDi[n.Chiave] = x.ComponenteID
			}
			// altrimenti e' un pezzo nuovo: i suoi archi sono aggiunte, non tolgono niente alla working
		}
	}
	if len(senzaCodice) > 0 {
		es.Sospese = fmt.Sprintf("%d nodi dello STEP strutturale senza codice (%s): le rimozioni aspettano che l'abbiano",
			len(senzaCodice), elencoNomi(senzaCodice))
		return es, nil
	}
	nelFile := map[Arco]bool{}
	for _, r := range st.Relazioni {
		pa, okP := componenteDi[r.Padre]
		fi, okF := componenteDi[r.Figlio]
		if okP && okF {
			nelFile[Arco{Padre: pa, Figlio: fi}] = true
		}
	}
	es.Calcolate = true
	gia, err := q.ListRimozioniDiUnoStep(ctx, db.ListRimozioniDiUnoStepParams{ThreadID: thread, StepDocumentoID: d.DocumentoID})
	if err != nil {
		return es, err
	}
	perArco := map[Arco]db.RimozioneProposta{}
	for _, r := range gia {
		perArco[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r
	}
	// Si scrive solo cio' che cambia: il calcolo si rifa' a ogni apertura della RFQ, e una riga uguale
	// riscritta ogni volta e' lavoro per niente. Una rimozione decisa non si tocca: uno scarta resta
	// scartato.
	vive := map[Arco]bool{}
	for _, r := range Rimozioni(p.ComponenteID, w.Archi, nelFile) {
		a := Arco{Padre: r.Padre, Figlio: r.Figlio}
		vive[a] = true
		e, c := perArco[a]
		if c && e.Stato != db.StatoPropostaAperta {
			continue
		}
		es.Proposte++
		if c && e.QtaWorking == r.QtaWorking {
			continue
		}
		if err := q.UpsertRimozioneProposta(ctx, db.UpsertRimozionePropostaParams{ThreadID: thread, StepDocumentoID: d.DocumentoID,
			PadreID: r.Padre, FiglioID: r.Figlio, QtaWorking: r.QtaWorking}); err != nil {
			return es, err
		}
	}
	for _, r := range gia {
		if r.Stato != db.StatoPropostaAperta {
			continue
		}
		a := Arco{Padre: r.PadreID, Figlio: r.FiglioID}
		if vive[a] {
			continue
		}
		nota := "lo STEP strutturale contiene di nuovo l'arco"
		if _, c := w.Archi[a]; !c {
			nota = "l'arco non è più nella BOM working"
		}
		if _, err := q.ChiudiRimozione(ctx, db.ChiudiRimozioneParams{ThreadID: thread, StepDocumentoID: d.DocumentoID,
			PadreID: r.PadreID, FiglioID: r.FiglioID, Nota: pgtype.Text{String: nota, Valid: true}}); err != nil {
			return es, err
		}
		es.Chiuse++
	}
	return es, nil
}

// rileggiLoStep applica alla RFQ i fatti correnti dello STEP strutturale di p, se c'e' un allegato della
// RFQ con quel contenuto: nascono o si aggiornano le sue proposte, e alla fine si ricalcolano le
// rimozioni (ApplicaStruttura lo fa da se'). Senza fatti correnti o senza allegato, solo le rimozioni.
func rileggiLoStep(ctx context.Context, q *db.Queries, thread uuid.UUID, p db.Componente) (EsitoRimozioni, error) {
	if !p.StepStrutturaleID.Valid {
		return AggiornaRimozioni(ctx, q, thread, p)
	}
	d, err := q.GetDocumento(ctx, p.StepStrutturaleID.UUID)
	if err != nil {
		return EsitoRimozioni{}, err
	}
	af, err := q.GetAnalisiCorrente(ctx, d.Sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		return AggiornaRimozioni(ctx, q, thread, p)
	}
	if err != nil {
		return EsitoRimozioni{}, err
	}
	step, err := q.ListStepDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return EsitoRimozioni{}, err
	}
	for _, a := range step {
		if a.Sha256.String != d.Sha256 {
			continue
		}
		m, err := MotoreDellaRfq(ctx, q, thread)
		if err != nil {
			return EsitoRimozioni{}, err
		}
		es, err := ApplicaStruttura(ctx, q, thread, a, af.Fatti, m)
		return es.Rimozioni, err
	}
	return AggiornaRimozioni(ctx, q, thread, p)
}

// AggiornaTutteLeRimozioni ricalcola le rimozioni di ogni prodotto finito della RFQ che ha uno STEP
// strutturale corrente: dopo una decisione che cambia i nodi riconosciuti, o all'apertura.
func AggiornaTutteLeRimozioni(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]EsitoRimozioni, error) {
	prodotti, err := q.ListProdottiConStepStrutturale(ctx, thread)
	if err != nil {
		return nil, err
	}
	out := make([]EsitoRimozioni, 0, len(prodotti))
	for _, p := range prodotti {
		es, err := AggiornaRimozioni(ctx, q, thread, p)
		if err != nil {
			return nil, err
		}
		out = append(out, es)
	}
	return out, nil
}

func elencoNomi(nomi []string) string {
	const max = 3
	if len(nomi) <= max {
		return strings.Join(nomi, ", ")
	}
	return strings.Join(nomi[:max], ", ") + fmt.Sprintf(" e altri %d", len(nomi)-max)
}
