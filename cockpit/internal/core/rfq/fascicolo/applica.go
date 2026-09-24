package fascicolo

// Dai fatti di uno STEP alle proposte di una RFQ (Blocco 8, B8.5): il fan-out.
//
// Il worker produce fatti (analisi_fatti.fatti.struttura) e non modifica mai la BOM. Qui il server li
// classifica con le regole del cliente della RFQ, li confronta con la BOM working e scrive le proposte:
// nodi, archi, quantita' e, se il file e' lo STEP strutturale di un prodotto letto per intero,
// rimozioni. Nessuna riga di componente o di componente_relazione nasce o cambia da qui: le scrive solo
// una decisione (decisioni.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// EsitoStruttura dice che cosa ha fatto l'applicazione dei fatti di un file a una RFQ.
type EsitoStruttura struct {
	Nodi, Relazioni int // righe di proposta scritte o riscritte (le altre erano gia' cosi', o decise)
	NodiAperti      int // nodi del file che chiedono una decisione
	RelazioniAperte int
	Completa        bool
	MotivoParziale  string
	Rimozioni       EsitoRimozioni
}

// EsitoRimozioni e' l'esito del confronto con lo STEP strutturale, quando il file lo e'.
type EsitoRimozioni struct {
	Prodotto  string // codice del prodotto finito; "" se il file non e' uno STEP strutturale
	Proposte  int    // rimozioni aperte adesso
	Chiuse    int    // rimozioni aperte che non valevano piu'
	Sospese   string // perche' non si sono calcolate ("" = calcolate)
	Calcolate bool
}

// ApplicaStruttura scrive le proposte che i fatti di un file danno nella RFQ thread. a e' l'allegato
// della RFQ con quel contenuto; fatti e' analisi_fatti.fatti (o i dettagli del risultato): quello che
// conta e' la chiave "struttura". Senza struttura non fa niente: un PDF, o fatti v1.
//
// Idempotente, e scrive solo cio' che cambia: la si chiama a ogni risultato, a ogni riuso dei fatti e
// all'apertura della RFQ, e la seconda volta non riscrive niente. Una proposta decisa non si tocca.
func ApplicaStruttura(ctx context.Context, q *db.Queries, thread uuid.UUID, a db.Allegato, fatti json.RawMessage,
	m *classificazione.Motore) (EsitoStruttura, error) {
	var es EsitoStruttura
	grezza, st, ok := struttura(fatti)
	if !ok || !a.Sha256.Valid || a.Sha256.String == "" {
		return es, nil
	}
	sha := a.Sha256.String
	motivo, err := q.MotivoParziale(ctx, &grezza)
	if err != nil {
		return es, fmt.Errorf("completezza della struttura: %w", err)
	}
	es.Completa, es.MotivoParziale = motivo == "", motivo

	// Lo stesso contenuto arrivato due volte nella stessa RFQ propone una volta sola.
	portatore := a.AllegatoID
	if id, err := q.PortatoreDelFile(ctx, db.PortatoreDelFileParams{ThreadID: thread, Sha256: sha}); err == nil {
		portatore = id
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return es, err
	}

	esistenti, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: thread, AllegatoID: portatore})
	if err != nil {
		return es, err
	}
	nodi := ClassificaNodi(m, *st)
	perChiave := map[string]db.ComponenteProposta{}
	decisi := map[string]uuid.UUID{}
	for _, e := range esistenti {
		perChiave[e.Chiave] = e
		if (e.Stato == db.StatoPropostaConfermata || e.Stato == db.StatoPropostaDuplicato) && e.ComponenteID.Valid {
			decisi[e.Chiave] = e.ComponenteID.UUID
		}
	}
	for i, n := range nodi {
		// un codice scritto dall'operatore vale piu' di qualunque classificazione, anche nel confronto
		if e, c := perChiave[n.Chiave]; c && e.OrigineCodice.Valid && e.OrigineCodice.OrigineCodice == db.OrigineCodiceOperatore {
			nodi[i].Codice, nodi[i].Rev, nodi[i].Origine = e.Codice.String, e.Rev.String, string(db.OrigineCodiceOperatore)
			nodi[i].Famiglia, nodi[i].Confidenza = e.Famiglia, int(e.Confidenza)
		}
	}

	w, err := leggiWorking(ctx, q, thread)
	if err != nil {
		return es, err
	}
	ctxFile := Contesto{Working: w, Decisi: decisi, Completa: es.Completa, Identificativi: map[string]bool{}}
	ids, err := q.ListIdentificativi(ctx, thread)
	if err != nil {
		return es, err
	}
	for _, i := range ids {
		ctxFile.Identificativi[strings.ToUpper(strings.TrimSpace(i.Codice))] = true
	}
	var prodotto *db.Componente
	if x, err := q.ProdottoDelloStepStrutturale(ctx, db.ProdottoDelloStepStrutturaleParams{ThreadID: thread, Sha256: sha}); err == nil {
		prodotto = &x
		ctxFile.Radice = prodotto
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return es, err
	}

	piano := Pianifica(nodi, *st, ctxFile)
	es.NodiAperti, es.RelazioniAperte = piano.Aperte()
	for _, pn := range piano.Nodi {
		arg := argNodo(thread, portatore, sha, pn)
		if e, c := perChiave[pn.Chiave]; c && (e.Stato != db.StatoPropostaAperta || stessoNodo(e, arg)) {
			continue
		}
		if err := q.UpsertComponenteProposta(ctx, arg); err != nil {
			return es, fmt.Errorf("proposta del nodo %s: %w", pn.Chiave, err)
		}
		es.Nodi++
	}
	relEsistenti, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: portatore})
	if err != nil {
		return es, err
	}
	perCoppia := map[[2]string]db.RelazioneProposta{}
	for _, r := range relEsistenti {
		perCoppia[[2]string{r.PadreChiave, r.FiglioChiave}] = r
	}
	for _, pr := range piano.Relazioni {
		arg := argRelazione(thread, portatore, pr)
		if e, c := perCoppia[[2]string{pr.Padre, pr.Figlio}]; c && (e.Stato != db.StatoPropostaAperta || stessaRelazione(e, arg)) {
			continue
		}
		if err := q.UpsertRelazioneProposta(ctx, arg); err != nil {
			return es, fmt.Errorf("proposta della relazione %s → %s: %w", pr.Padre, pr.Figlio, err)
		}
		es.Relazioni++
	}

	if err := propostaDelDocumento(ctx, q, a, nodi, *st, m); err != nil {
		return es, err
	}
	if prodotto != nil {
		r, err := AggiornaRimozioni(ctx, q, thread, *prodotto)
		if err != nil {
			return es, err
		}
		es.Rimozioni = r
	}
	return es, nil
}

// struttura estrae la chiave "struttura" dai fatti, grezza (per la funzione SQL) e decodificata.
func struttura(fatti json.RawMessage) (json.RawMessage, *worker.StrutturaSTEP, bool) {
	st, ok := worker.DecodificaStruttura(fatti)
	if !ok {
		return nil, nil, false
	}
	var involucro struct {
		Struttura json.RawMessage `json:"struttura"`
	}
	if err := json.Unmarshal(fatti, &involucro); err != nil {
		return nil, nil, false
	}
	return involucro.Struttura, st, true
}

func leggiWorking(ctx context.Context, q *db.Queries, thread uuid.UUID) (Working, error) {
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return Working{}, err
	}
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return Working{}, err
	}
	return NuovaWorking(comp, rel), nil
}

func argNodo(thread, allegato uuid.UUID, sha string, pn PropostaNodo) db.UpsertComponentePropostaParams {
	ev, _ := json.Marshal(pn.Evidenza)
	arg := db.UpsertComponentePropostaParams{
		ThreadID: thread, AllegatoID: allegato, Sha256: sha, Chiave: pn.Chiave,
		NomeGrezzo: pn.NomeGrezzo, IDGrezzo: pn.IDGrezzo, Descrizione: testo(pn.Descrizione),
		Codice: testo(pn.Codice), Rev: testo(pn.Rev), Famiglia: pn.Famiglia,
		TipoProposto: db.NullTipoComponente{TipoComponente: pn.Tipo, Valid: pn.Tipo != ""},
		Confidenza:   int16(pn.Confidenza), Evidenza: ev, Stato: pn.Stato, ComponenteID: pn.ComponenteID,
		Nota: testo(pn.Nota),
	}
	if o := db.OrigineCodice(pn.Origine); pn.Codice != "" && o.Valid() {
		arg.OrigineCodice = db.NullOrigineCodice{OrigineCodice: o, Valid: true}
	}
	return arg
}

func argRelazione(thread, allegato uuid.UUID, pr PropostaRelazione) db.UpsertRelazionePropostaParams {
	ev, _ := json.Marshal(pr.Evidenza)
	return db.UpsertRelazionePropostaParams{ThreadID: thread, AllegatoID: allegato, PadreChiave: pr.Padre,
		FiglioChiave: pr.Figlio, Qta: pr.Qta, Evidenza: ev, Stato: pr.Stato, Nota: testo(pr.Nota)}
}

// stessoNodo: la riga aperta dice gia' quello che il piano direbbe. Allora non si riscrive.
func stessoNodo(e db.ComponenteProposta, a db.UpsertComponentePropostaParams) bool {
	return e.NomeGrezzo == a.NomeGrezzo && e.IDGrezzo == a.IDGrezzo && e.Descrizione == a.Descrizione &&
		e.Codice == a.Codice && e.Rev == a.Rev && e.OrigineCodice == a.OrigineCodice && e.Famiglia == a.Famiglia &&
		e.TipoProposto == a.TipoProposto && e.Confidenza == a.Confidenza && e.Stato == a.Stato &&
		e.ComponenteID == a.ComponenteID && e.Nota == a.Nota && stessoJSON(e.Evidenza, a.Evidenza)
}

func stessaRelazione(e db.RelazioneProposta, a db.UpsertRelazionePropostaParams) bool {
	return e.Qta == a.Qta && e.Stato == a.Stato && e.Nota == a.Nota && stessoJSON(e.Evidenza, a.Evidenza)
}

// stessoJSON confronta due documenti JSON per contenuto: jsonb riordina le chiavi.
func stessoJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func testo(s string) pgtype.Text {
	if strings.TrimSpace(s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// propostaDelDocumento e' D16: quando una famiglia del cliente riconosce il codice della radice, la
// proposta del documento STEP prende quel codice, con fonte regola_cliente. Solo se la proposta e'
// aperta, se non l'ha scritta l'operatore, e se il suo codice non e' gia' di una famiglia: allora il
// nome del file diceva gia' la cosa giusta, e un generico dal PRODUCT non la cambia.
func propostaDelDocumento(ctx context.Context, q *db.Queries, a db.Allegato, nodi []NodoClassificato, st worker.StrutturaSTEP, m *classificazione.Motore) error {
	radice, ok := RadiceDiFamiglia(nodi, st)
	if !ok {
		return nil
	}
	p, err := q.GetPropostaDocumentoDiAllegato(ctx, a.AllegatoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.Stato != db.StatoPropostaAperta || p.Fonte == db.FontePropostaOperatore {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(p.Codice.String), radice.Codice) && p.Fonte == db.FontePropostaRegolaCliente {
		return nil
	}
	if p.Codice.Valid && len(classificazione.DiFamiglia(m.Codici(p.Codice.String))) > 0 {
		return nil
	}
	dett, _ := json.Marshal(map[string]any{"famiglia": radice.Famiglia, "radice_step": radice.Chiave, "nome_grezzo": radice.NomeGrezzo})
	_, err = q.PropostaDocumentoDaRadice(ctx, db.PropostaDocumentoDaRadiceParams{
		AllegatoID: a.AllegatoID, Codice: pgtype.Text{String: radice.Codice, Valid: true}, Rev: testo(radice.Rev),
		Confidenza: int16(radice.Confidenza), Dettagli: dett,
	})
	return err
}
