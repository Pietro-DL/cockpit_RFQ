package web

// «Conferma Fascicolo» e le decisioni di «Da verificare» (B8.7b).
//
// Il piano (core/rfq/fascicolo/piano.go) dice che cosa e' pronto: file con tipo, codice e componente senza
// conflitti, strutture di STEP senza nodi da decidere, lo STEP strutturale quando e' uno solo. Una conferma
// sola li porta nel fascicolo, in una transazione: prima la struttura (nascono i componenti), poi i
// documenti (ciascuno con la conferma di sempre, che sceglie il posto sul NAS e accoda la copia), poi lo
// STEP strutturale. Un solo rifiuto annulla tutto, e l'avviso dice quale file e perche'.
//
// La conferma lavora sul piano di adesso, ricalcolato nella sua transazione con la RFQ bloccata: senza
// elenco conferma tutto il pronto, ma solo se la firma e' quella che l'operatore aveva davanti; con un
// elenco (dal cassetto «Rivedi») conferma quelle voci, e ognuna deve essere ancora pronta.
//
// Le ambiguita' vere si decidono una per una, con gesti piccoli: il tipo o il codice di un file
// (DecidiPropostaDocumento: fonte operatore, che una lettura dopo non riscrive), aggiungere il componente
// che manca, assegnare, confermare con la scelta aggiungi/sostituisce. Una voce decisa torna nel piano, e
// se e' pronta entra con la conferma successiva.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraConferma(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/fascicolo/avanzamento", s.autenticato(s.fascicoloAvanzamento))
	mux.HandleFunc("GET /thread/{id}/fascicolo/nas", s.autenticato(s.fascicoloNas))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nas/importa", s.autenticato(s.importaDalNas))
	mux.HandleFunc("POST /thread/{id}/fascicolo/conferma", s.autenticato(s.confermaFascicolo))
	mux.HandleFunc("POST /thread/{id}/fascicolo/proposta/{pid}/decidi", s.autenticato(s.decidiProposta))
}

// selezione sono le voci del piano che la conferma deve portare nel fascicolo.
type selezione struct {
	file        map[uuid.UUID]bool // proposte dei documenti
	strutture   map[uuid.UUID]bool // allegati STEP
	strutturali map[uuid.UUID]bool // prodotti finiti
}

func (s selezione) vuota() bool { return len(s.file)+len(s.strutture)+len(s.strutturali) == 0 }

// selezioneDal legge le voci dal form (voce, struttura, strutturale); senza, tutte le pronte, se la firma
// e' quella del piano.
func selezioneDal(r *http.Request, p fascicolo.PianoFascicolo) (selezione, error) {
	sel := selezione{file: map[uuid.UUID]bool{}, strutture: map[uuid.UUID]bool{}, strutturali: map[uuid.UUID]bool{}}
	for campo, m := range map[string]map[uuid.UUID]bool{"voce": sel.file, "struttura": sel.strutture, "strutturale": sel.strutturali} {
		ids, err := uuidDalForm(r.Form[campo])
		if err != nil {
			return sel, rifiuto("voce del piano non valida")
		}
		for _, id := range ids {
			m[id] = true
		}
	}
	if !sel.vuota() {
		for _, v := range p.File {
			if sel.file[v.Proposta] && v.Stato != fascicolo.VocePronta {
				return sel, rifiuto(v.Nome + ": non è più pronto (" + statoVoce(v.Stato, v.Domande) + "): ricontrolla il piano")
			}
		}
		for _, v := range p.Strutture {
			if sel.strutture[v.Allegato] && v.Stato != fascicolo.VocePronta {
				return sel, rifiuto("la struttura di " + v.Nome + " non è più pronta: ricontrolla il piano")
			}
		}
		for _, v := range p.Strutturali {
			if sel.strutturali[v.Prodotto.ComponenteID] && v.Stato != fascicolo.VocePronta {
				return sel, rifiuto("lo STEP strutturale di " + v.Prodotto.Codice + " non è più pronto: ricontrolla il piano")
			}
		}
		if n := len(sel.file) + len(sel.strutture) + len(sel.strutturali); n != contaSelezionabili(p, sel) {
			return sel, rifiuto("una voce scelta non è più nel piano: ricontrolla e conferma di nuovo")
		}
		return sel, nil
	}
	if f := strings.TrimSpace(r.FormValue("firma")); f == "" || f != p.Firma() {
		return sel, rifiuto("il piano è cambiato mentre lo guardavi (un file analizzato, una decisione di un collega): ricontrolla e conferma di nuovo")
	}
	for _, v := range p.File {
		if v.Stato == fascicolo.VocePronta {
			sel.file[v.Proposta] = true
		}
	}
	for _, v := range p.Strutture {
		if v.Stato == fascicolo.VocePronta {
			sel.strutture[v.Allegato] = true
		}
	}
	for _, v := range p.Strutturali {
		if v.Stato == fascicolo.VocePronta {
			sel.strutturali[v.Prodotto.ComponenteID] = true
		}
	}
	return sel, nil
}

// contaSelezionabili conta quante voci scelte esistono davvero nel piano.
func contaSelezionabili(p fascicolo.PianoFascicolo, sel selezione) int {
	n := 0
	for _, v := range p.File {
		if sel.file[v.Proposta] {
			n++
		}
	}
	for _, v := range p.Strutture {
		if sel.strutture[v.Allegato] {
			n++
		}
	}
	for _, v := range p.Strutturali {
		if sel.strutturali[v.Prodotto.ComponenteID] {
			n++
		}
	}
	return n
}

func statoVoce(s fascicolo.StatoVoce, dd []fascicolo.Domanda) string {
	switch s {
	case fascicolo.VoceAttesa:
		return "in preparazione"
	case fascicolo.VoceDecidere:
		if len(dd) > 0 {
			return dd[0].Testo
		}
		return "da decidere"
	}
	return string(s)
}

// confermaFascicolo: POST /thread/{id}/fascicolo/conferma. Campi: `firma` (conferma tutto il pronto del piano
// con quella firma) oppure `voce`, `struttura`, `strutturale` (le voci scelte nel cassetto «Rivedi»).
func (s *Server) confermaFascicolo(w http.ResponseWriter, r *http.Request) {
	u := utenteDa(r.Context())
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		p, err := fascicolo.LeggiPianoFascicolo(ctx, q, thread)
		if err != nil {
			return "", err
		}
		sel, err := selezioneDal(r, p)
		if err != nil {
			return "", err
		}
		if sel.vuota() {
			return "", rifiuto("nel piano non c'è niente di pronto da confermare")
		}
		return s.applicaPiano(ctx, q, thread, u, p, sel)
	})
}

// applicaPiano porta nel fascicolo le voci scelte: la struttura, poi i documenti, poi lo STEP strutturale.
func (s *Server) applicaPiano(ctx context.Context, q *db.Queries, thread uuid.UUID, u *db.Utente, p fascicolo.PianoFascicolo, sel selezione) (string, error) {
	// le dipendenze: un file che va a un componente che nasce da uno STEP si conferma con quella struttura,
	// uno STEP strutturale che e' un file del piano con quel file
	propostaDi := map[uuid.UUID]uuid.UUID{}
	for _, v := range p.File {
		propostaDi[v.Allegato] = v.Proposta
		if sel.file[v.Proposta] && v.DaStep != nil && !sel.strutture[v.DaStep.Allegato] {
			return "", rifiuto(fmt.Sprintf("%s va a %s, che nasce dalla struttura di %s: si confermano insieme", v.Nome, v.DaStep.Codice, v.DaStep.File))
		}
	}
	for _, v := range p.Strutturali {
		if sel.strutturali[v.Prodotto.ComponenteID] && v.Allegato != uuid.Nil && !sel.file[propostaDi[v.Allegato]] {
			return "", rifiuto(fmt.Sprintf("%s diventa lo STEP strutturale di %s solo se entra nel fascicolo: si confermano insieme", v.Nome, v.Prodotto.Codice))
		}
	}

	var parti []string
	for _, v := range p.Strutture {
		if !sel.strutture[v.Allegato] {
			continue
		}
		msg, err := fascicolo.AccettaFile(ctx, q, thread, v.Allegato, u.UtenteID)
		if err != nil {
			return "", rifiuto("la struttura di " + v.Nome + ": " + spiegaErrore(err))
		}
		parti = append(parti, "struttura di "+v.Nome+": "+strings.TrimSuffix(strings.ToLower(msg[:1])+msg[1:], "."))
	}

	nDoc, nProv, attesa := 0, 0, false
	for _, v := range p.File {
		if !sel.file[v.Proposta] {
			continue
		}
		pr, err := q.BloccaProposta(ctx, v.Proposta)
		if err != nil {
			return "", err
		}
		a, err := q.GetAllegato(ctx, v.Allegato)
		if err != nil {
			return "", err
		}
		m, err := q.GetMessaggio(ctx, a.MessaggioID)
		if err != nil {
			return "", err
		}
		var comp uuid.NullUUID
		switch {
		case v.Componente != nil:
			comp = uuid.NullUUID{UUID: v.Componente.ComponenteID, Valid: true}
		case v.DaStep != nil:
			c, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: thread, Upper: v.DaStep.Codice})
			if errors.Is(err, pgx.ErrNoRows) {
				return "", rifiuto(fmt.Sprintf("%s: il componente %s non è nato dalla struttura di %s", v.Nome, v.DaStep.Codice, v.DaStep.File))
			}
			if err != nil {
				return "", err
			}
			comp = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
		}
		scelta := sceltaRevisione{}
		if v.Aggiunge {
			scelta.data = true // fogli arrivati insieme: si aggiungono l'uno all'altro
		}
		msg, err := s.confermaProposta(ctx, q, u, pr, a, m, v.Tipo, v.Codice, v.Rev, "", comp, false, scelta)
		if err != nil {
			return "", rifiuto(v.Nome + ": " + spiegaErrore(err))
		}
		switch {
		case strings.HasPrefix(msg, "File già presente"):
			nProv++
		default:
			nDoc++
			attesa = attesa || strings.Contains(msg, "IN ATTESA")
		}
	}
	if nDoc > 0 {
		copia := "copie sul NAS in coda"
		if attesa {
			copia = "copie sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta"
		}
		parti = append(parti, fmt.Sprintf("%s (%s)", conta(nDoc, "documento", "documenti"), copia))
	}
	if nProv > 0 {
		parti = append(parti, conta(nProv, "file già nel fascicolo: registrata la provenienza", "file già nel fascicolo: registrate le provenienze"))
	}

	for _, v := range p.Strutturali {
		if !sel.strutturali[v.Prodotto.ComponenteID] {
			continue
		}
		doc := v.Documento
		if !doc.Valid {
			a, err := q.GetAllegato(ctx, v.Allegato)
			if err != nil {
				return "", err
			}
			d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: a.Sha256.String})
			if err != nil {
				return "", rifiuto(v.Nome + ": il documento dello STEP non c'è")
			}
			doc = uuid.NullUUID{UUID: d.DocumentoID, Valid: true}
		}
		if _, err := fascicolo.ScegliStepStrutturale(ctx, q, thread, v.Prodotto.ComponenteID, doc.UUID); err != nil {
			return "", rifiuto("STEP strutturale di " + v.Prodotto.Codice + ": " + spiegaErrore(err))
		}
		parti = append(parti, "STEP strutturale di "+v.Prodotto.Codice+": "+v.Nome)
	}
	if len(parti) == 0 {
		return "", rifiuto("niente da confermare")
	}
	return "Fascicolo confermato: " + strings.Join(parti, "; ") + ".", nil
}

// decidiProposta: POST /thread/{id}/fascicolo/proposta/{pid}/decidi, campi `tipo`, `codice`, `rev`. E' la
// risposta a una domanda del piano su un file non ancora confermato (che cos'e', che codice ha): la proposta
// prende quei valori con fonte operatore, e una lettura che arriva dopo non la riscrive.
func (s *Server) decidiProposta(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		pid, err := idDa(r.PathValue("pid"), "proposta")
		if err != nil {
			return "", err
		}
		p, err := q.BloccaProposta(ctx, pid)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!p.ThreadID.Valid || p.ThreadID.UUID != thread)) {
			return "", rifiuto("la proposta non è di questa RFQ")
		}
		if err != nil {
			return "", err
		}
		a, err := q.GetAllegato(ctx, p.AllegatoID)
		if err != nil {
			return "", err
		}
		if p.Stato != db.StatoPropostaAperta {
			return "", rifiuto(a.NomeFile + ": la proposta è già decisa")
		}
		if p.ComponenteID.Valid {
			return "", rifiuto(a.NomeFile + ": è assegnato a un componente, e ne ha il codice: si sgancia prima di cambiarlo")
		}
		tipo := db.TipoDocumento(strings.TrimSpace(r.FormValue("tipo")))
		if tipo == "" {
			tipo = p.TipoProposto
		}
		if !tipo.Valid() || tipo == db.TipoDocumentoDaDeterminare || tipo == db.TipoDocumentoRumore {
			return "", rifiuto("scegli che cos'è il file (un file che non serve si scarta)")
		}
		codice := strings.ToUpper(strings.TrimSpace(r.FormValue("codice")))
		rev := strings.ToUpper(strings.TrimSpace(r.FormValue("rev")))
		if codice != "" && !classificazione.CodiceAmmissibile(codice) {
			return "", rifiuto(fmt.Sprintf("%s: il codice ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxCodice))
		}
		if rev != "" && !classificazione.RevAmmissibile(rev) {
			return "", rifiuto(fmt.Sprintf("%s: la revisione ha più di %d caratteri o caratteri non ammessi", rev, classificazione.MaxRev))
		}
		if documenti.Tecnico(tipo) && codice == "" {
			return "", rifiuto("un CAD 3D, un disegno 2D o uno sviluppo DXF entra nel fascicolo solo con il codice del pezzo")
		}
		if _, err := q.DecidiPropostaDocumento(ctx, db.DecidiPropostaDocumentoParams{PropostaID: pid, TipoProposto: tipo,
			Codice: ptxt(codice), Rev: ptxt(rev)}); err != nil {
			return "", err
		}
		msg := a.NomeFile + ": " + etichettaTipoDoc(string(tipo))
		if codice != "" {
			msg += ", codice " + codice
		}
		if rev != "" {
			msg += " rev " + rev
		}
		return msg + ". Il piano lo riprende: se è pronto entra con «Conferma Fascicolo».", nil
	})
}
