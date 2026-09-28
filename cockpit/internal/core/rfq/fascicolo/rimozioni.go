package fascicolo

// Le proposte di rimozione (Blocco 8, B8.5; addendum A4.4, D27, D31; Smistamento F5, A5.4.6, D77).
//
// «Che cosa e' sparito dalla distinta?» ha senso solo rispetto a UN file scelto da una persona, il file
// autorizzato a proporre i figli diretti di un componente, e solo se quel file e' stato letto per intero:
// una mancanza in una lettura parziale non e' un'informazione. Qui si confrontano gli archi della working
// che partono dal componente con quelli che il suo file gli propone, a profondita' 1: il file di C ha
// autorita' sui figli diretti di C e basta (prima si scendeva tutto il sottoalbero, e il file del prodotto
// proponeva di togliere archi su cui comanda lo STEP di un sottoassieme). Ogni arco che il file non
// contiene piu' diventa una rimozione_proposta. La proposta non toglie niente: l'arco lo toglie chi la
// accetta.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// AggiornaRimozioni ricalcola le rimozioni proposte dal file autorizzato d.
//
// Le condizioni, tutte (A4.4, A5.4.6):
//   - l'autorizzazione vale (documento corrente, nessun conflitto: ValutaDichiarazioni);
//   - l'analisi corrente del file e' completa (struttura_motivo_parziale vuoto), con una radice sola;
//   - ogni figlio diretto delle sorgenti e' deciso da una persona: accettato (e allora e' un componente) o
//     scartato. Un figlio ancora aperto, o agganciato per codice da un automatismo di prima, sospende il
//     calcolo: potrebbe essere proprio il pezzo che sembra sparito. Prima bastava che avesse un codice, e un
//     nodo aperto con il codice di un componente contava come quel componente: era un'identita' decisa dal
//     codice, e non c'e' piu'.
//
// Le rimozioni aperte che non valgono piu' (l'arco non e' piu' nella working, o il file lo contiene di
// nuovo) si chiudono con la nota. Una rimozione decisa da una persona non si riapre.
func AggiornaRimozioni(ctx context.Context, q *db.Queries, thread uuid.UUID, d Dichiarazione) (EsitoRimozioni, error) {
	es := EsitoRimozioni{Prodotto: d.Componente.Codice}
	if !d.Valida() {
		es.Sospese = "l'autorizzazione non vale: " + d.Problema
		return es, nil
	}
	af, err := q.GetAnalisiCorrente(ctx, d.Sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		es.Sospese = "nessuna analisi corrente dello STEP autorizzato: lo STEP non ha una lettura completa"
		return es, nil
	}
	if err != nil {
		return es, err
	}
	grezza, st, ok := struttura(af.Fatti)
	if !ok {
		es.Sospese = "la struttura dello STEP autorizzato non si legge: lo STEP non ha una lettura completa"
		return es, nil
	}
	motivo, err := q.MotivoParziale(ctx, &grezza)
	if err != nil {
		return es, err
	}
	if motivo != "" {
		es.Sospese = "lo STEP autorizzato non ha una lettura completa (" + motivo + ")"
		return es, nil
	}
	if len(st.Radici) != 1 {
		es.Sospese = fmt.Sprintf("lo STEP autorizzato ha %d radici: non e' una distinta sola", len(st.Radici))
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
	// Per ogni nodo, la riga che conta: una decisa da una persona vale piu' di una aperta (lo stesso contenuto
	// ha un solo portatore per RFQ, ma una riga vecchia di un altro allegato puo' esserci ancora).
	perChiave := map[string]db.ComponenteProposta{}
	for _, r := range proposte {
		if e, c := perChiave[r.Chiave]; !c || (!e.DecisoDa.Valid && r.DecisoDa.Valid) {
			perChiave[r.Chiave] = r
		}
	}
	c := d.Componente.ComponenteID
	nomi := map[string]string{}
	for _, n := range st.Nodi {
		nomi[n.Chiave] = n.NomeGrezzo
	}
	componenteDi := map[string]uuid.UUID{}
	var daDecidere []string
	visto := map[string]bool{}
	for _, r := range st.Relazioni {
		if _, sorgente := d.Sorgenti[r.Padre]; !sorgente {
			continue
		}
		if _, sorgente := d.Sorgenti[r.Figlio]; sorgente || visto[r.Figlio] {
			continue // un raggruppamento e' C stesso; un figlio sotto due sorgenti si guarda una volta
		}
		visto[r.Figlio] = true
		p, ok := perChiave[r.Figlio]
		switch {
		case !ok:
			daDecidere = append(daDecidere, nomi[r.Figlio])
		case p.Stato == db.StatoPropostaScartata && p.DecisoDa.Valid:
			// una persona ha detto che non e' un pezzo della distinta
		default:
			if comp, deciso := DecisoDaUnaPersona(p); deciso {
				componenteDi[r.Figlio] = comp
			} else {
				daDecidere = append(daDecidere, nomeNodo(p))
			}
		}
	}
	if len(daDecidere) > 0 {
		sort.Strings(daDecidere)
		es.Sospese = fmt.Sprintf("%s da decidere (%s): le rimozioni aspettano che una persona li accetti o li scarti",
			quanti(len(daDecidere), "figlio diretto", "figli diretti"), elencoNomi(daDecidere))
		return es, nil
	}
	nelFile := map[Arco]bool{}
	for _, r := range st.Relazioni {
		if _, sorgente := d.Sorgenti[r.Padre]; !sorgente {
			continue
		}
		if fi, ok := componenteDi[r.Figlio]; ok {
			nelFile[Arco{Padre: c, Figlio: fi}] = true
		}
	}
	es.Calcolate = true
	doc := d.Documento.UUID
	gia, err := q.ListRimozioniDiUnoStep(ctx, db.ListRimozioniDiUnoStepParams{ThreadID: thread, StepDocumentoID: doc})
	if err != nil {
		return es, err
	}
	// lo stesso documento puo' portare le rimozioni di piu' dichiarazioni (una delega e' il file del padre):
	// qui contano solo quelle che partono da C
	mie := gia[:0:0]
	for _, r := range gia {
		if r.PadreID == c {
			mie = append(mie, r)
		}
	}
	gia = mie
	perArco := map[Arco]db.RimozioneProposta{}
	for _, r := range gia {
		perArco[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r
	}
	// Si scrive solo cio' che cambia: il calcolo si rifa' dopo ogni decisione, e una riga uguale riscritta ogni
	// volta e' lavoro per niente. Una rimozione decisa da una persona non si tocca: uno scarta resta scartato.
	// Una chiusa da un automatismo si riapre, se vale di nuovo (E33).
	vive := map[Arco]bool{}
	for _, r := range RimozioniFigliDiretti(c, w.Archi, nelFile) {
		a := Arco{Padre: r.Padre, Figlio: r.Figlio}
		vive[a] = true
		e, esiste := perArco[a]
		if esiste && !riscrivibile(e.Stato, e.DecisoDa) {
			continue
		}
		es.Proposte++
		if esiste && e.Stato == db.StatoPropostaAperta && e.QtaWorking == r.QtaWorking {
			continue
		}
		if err := q.UpsertRimozioneProposta(ctx, db.UpsertRimozionePropostaParams{ThreadID: thread, StepDocumentoID: doc,
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
		nota := "lo STEP autorizzato contiene di nuovo l'arco"
		if _, c := w.Archi[a]; !c {
			nota = "l'arco non è più nella BOM working"
		}
		if _, err := q.ChiudiRimozione(ctx, db.ChiudiRimozioneParams{ThreadID: thread, StepDocumentoID: doc,
			PadreID: r.PadreID, FiglioID: r.FiglioID, Nota: pgtype.Text{String: nota, Valid: true}}); err != nil {
			return es, err
		}
		es.Chiuse++
	}
	return es, nil
}

// AggiornaTutteLeRimozioni ricalcola le rimozioni di ogni autorizzazione valida della RFQ: dopo una
// decisione che cambia i nodi decisi o la working.
//
// Le rimozioni aperte di un'autorizzazione che non vale piu' (sospesa per un commerciale, in conflitto,
// superata, revocata) si chiudono con la nota, senza chi le ha decise, come fa l'archiviazione del
// componente: il file non ha piu' autorita', e una rimozione che resta aperta si potrebbe accettare e
// fermerebbe il gate. Chiuse da un automatismo, si riaprono da sole se l'autorizzazione torna valida (E33).
func AggiornaTutteLeRimozioni(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]EsitoRimozioni, error) {
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	valide := dich.Valide()
	out := make([]EsitoRimozioni, 0, len(valide))
	for _, d := range valide {
		es, err := AggiornaRimozioni(ctx, q, thread, d)
		if err != nil {
			return nil, err
		}
		out = append(out, es)
	}
	aperte, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return nil, err
	}
	for _, r := range aperte {
		if dich.RimozioneValida(r.StepDocumentoID, r.PadreID) {
			continue
		}
		nota := "l'autorizzazione dello STEP non vale più"
		for _, x := range dich.DelComponente(r.PadreID) {
			if x.Documento.Valid && x.Documento.UUID == r.StepDocumentoID && x.Problema != "" {
				nota = "l'autorizzazione non vale: " + x.Problema
			}
		}
		if _, err := q.ChiudiRimozione(ctx, db.ChiudiRimozioneParams{ThreadID: thread, StepDocumentoID: r.StepDocumentoID,
			PadreID: r.PadreID, FiglioID: r.FiglioID, Nota: pgtype.Text{String: nota, Valid: true}}); err != nil {
			return nil, err
		}
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
