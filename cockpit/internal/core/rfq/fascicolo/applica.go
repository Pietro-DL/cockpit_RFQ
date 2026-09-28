package fascicolo

// Dai fatti di uno STEP alle proposte di una RFQ (Blocco 8, B8.5): il fan-out.
//
// Il worker produce fatti (analisi_fatti.fatti.struttura) e non modifica mai la BOM. Qui il server li
// classifica con le regole del cliente della RFQ, li confronta con la BOM working e scrive le proposte:
// nodi, archi, quantita' e, se il file e' autorizzato a proporre i figli diretti di un componente ed e'
// letto per intero, rimozioni. Nessuna riga di componente o di componente_relazione nasce o cambia da qui:
// le scrive solo una decisione (decisioni.go).
//
// Smistamento F5 (A5.4.4). La lettura non da' piu' identita': nessun nodo diventa il componente con lo
// stesso codice, nessuna radice diventa il prodotto da sola. Le righe nascono tutte aperte; solo gli archi
// che partono da una sorgente autorizzata (dichiarazioni.go) tengono i conti con la working. La
// classificazione del server resta nella riga (evidenza.classificato, K2) anche quando il codice l'ha
// scritto l'operatore; le righe chiuse da un automatismo si riaprono (E33) e la storia sopravvive (P30).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

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
	// FuoriFlusso: il file non viene da una fonte del cliente, o una persona l'ha escluso (P27, E26): la sua
	// struttura non si legge nella RFQ, ne' come guida ne' come autorita'. L'analisi resta.
	FuoriFlusso bool
}

// EsitoRimozioni e' l'esito del confronto con il file autorizzato, quando il file lo e'.
type EsitoRimozioni struct {
	Prodotto  string // codice del componente per cui il file e' autorizzato; "" se non lo e'
	Proposte  int    // rimozioni aperte adesso
	Chiuse    int    // rimozioni aperte che non valevano piu'
	Sospese   string // perche' non si sono calcolate ("" = calcolate)
	Calcolate bool
}

// ApplicaStruttura scrive le proposte che i fatti di un file danno nella RFQ thread. a e' l'allegato
// della RFQ con quel contenuto; fatti e' analisi_fatti.fatti (o i dettagli del risultato): quello che
// conta e' la chiave "struttura". Senza struttura non fa niente: un PDF, o fatti v1.
//
// Idempotente, e scrive solo cio' che cambia: la si chiama a ogni risultato, a ogni riuso dei fatti e a
// ogni rilettura, e la seconda volta non riscrive niente. Una proposta decisa da una persona non si tocca.
// Nessuna riga di proposta sparisce: le righe si scrivono o si riscrivono, mai si cancellano.
func ApplicaStruttura(ctx context.Context, q *db.Queries, thread uuid.UUID, a db.Allegato, fatti json.RawMessage,
	m *classificazione.Motore) (EsitoStruttura, error) {
	var es EsitoStruttura
	grezza, st, ok := struttura(fatti)
	if !ok || !a.Sha256.Valid || a.Sha256.String == "" {
		return es, nil
	}
	nel, err := q.FileNelFlusso(ctx, a.AllegatoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return es, nil
	}
	if err != nil {
		return es, fmt.Errorf("fonte del file: %w", err)
	}
	if !nel {
		es.FuoriFlusso = true
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
		if c, ok := DecisoDaUnaPersona(e); ok {
			decisi[e.Chiave] = c
		}
	}
	for i, n := range nodi {
		// la classificazione del server, sempre, anche quando il codice della riga e' dell'operatore (K2): la
		// correzione non cancella l'evidenza da cui e' partita
		nodi[i].Evidenza["classificato"] = classificato(n)
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
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return es, err
	}
	ctxFile := Contesto{Working: w, Decisi: decisi, Sorgenti: dich.Sorgenti(sha), Completa: es.Completa}
	piano := Pianifica(nodi, *st, ctxFile)
	for _, pn := range piano.Nodi {
		arg := argNodo(thread, portatore, sha, pn)
		if e, c := perChiave[pn.Chiave]; c && (!riscrivibile(e.Stato, e.DecisoDa) || stessoNodo(e, arg)) {
			continue
		}
		if err := q.UpsertComponenteProposta(ctx, arg); err != nil {
			return es, fmt.Errorf("proposta del nodo %s: %w", pn.Chiave, err)
		}
		es.Nodi++
	}
	scritti := map[[2]string]bool{}
	if err := scriviArchi(ctx, q, thread, portatore, piano, scritti); err != nil {
		return es, err
	}
	// La forma di prima riconosce la radice dalle righe del file (K1: il nodo senza un arco entrante): alla
	// prima lettura le righe non c'erano, e le sorgenti si vedono solo adesso. Se sono cambiate gli archi si
	// ripianificano: nessun arco resta aperto o chiuso per una lettura fatta senza sapere chi comanda.
	dopo, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return es, err
	}
	if !stesseSorgenti(ctxFile.Sorgenti, dopo.Sorgenti(sha)) {
		dich, ctxFile.Sorgenti = dopo, dopo.Sorgenti(sha)
		piano = Pianifica(nodi, *st, ctxFile)
		if err := scriviArchi(ctx, q, thread, portatore, piano, scritti); err != nil {
			return es, err
		}
	}
	es.Relazioni = len(scritti)
	if es.NodiAperti, es.RelazioniAperte, err = ancoraAperte(ctx, q, thread, portatore, ctxFile.Sorgenti); err != nil {
		return es, err
	}

	if err := valutaLaRadice(ctx, q, a, nodi, *st, fatti, m); err != nil {
		return es, err
	}
	// le rimozioni di ogni autorizzazione valida di questo file (A5.4.6): di solito una
	for i, d := range dich.PerSha[sha] {
		r, err := AggiornaRimozioni(ctx, q, thread, d)
		if err != nil {
			return es, err
		}
		if i == 0 {
			es.Rimozioni = r
		}
	}
	return es, nil
}

// ancoraAperte conta le righe del file che chiedono ancora una decisione dopo la lettura: i nodi aperti che
// non sono una sorgente (la sorgente e' gia' il componente dell'autorizzazione, anche quando la riga di prima
// e' ancora aperta) e gli archi aperti.
func ancoraAperte(ctx context.Context, q *db.Queries, thread, portatore uuid.UUID, sorgenti map[string]uuid.UUID) (nodi, archi int, err error) {
	nn, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: thread, AllegatoID: portatore})
	if err != nil {
		return 0, 0, err
	}
	for _, n := range nn {
		if _, sorgente := sorgenti[n.Chiave]; n.Stato == db.StatoPropostaAperta && !sorgente {
			nodi++
		}
	}
	aa, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: portatore})
	if err != nil {
		return 0, 0, err
	}
	for _, a := range aa {
		if a.Stato == db.StatoPropostaAperta {
			archi++
		}
	}
	return nodi, archi, nil
}

// scriviArchi scrive gli archi del piano che cambiano; scritti tiene quelli gia' scritti in questa lettura.
func scriviArchi(ctx context.Context, q *db.Queries, thread, portatore uuid.UUID, piano Piano, scritti map[[2]string]bool) error {
	relEsistenti, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: portatore})
	if err != nil {
		return err
	}
	perCoppia := map[[2]string]db.RelazioneProposta{}
	for _, r := range relEsistenti {
		perCoppia[[2]string{r.PadreChiave, r.FiglioChiave}] = r
	}
	for _, pr := range piano.Relazioni {
		arg := argRelazione(thread, portatore, pr)
		if e, c := perCoppia[[2]string{pr.Padre, pr.Figlio}]; c && (!riscrivibile(e.Stato, e.DecisoDa) || stessaRelazione(e, arg)) {
			continue
		}
		if err := q.UpsertRelazioneProposta(ctx, arg); err != nil {
			return fmt.Errorf("proposta della relazione %s → %s: %w", pr.Padre, pr.Figlio, err)
		}
		scritti[[2]string{pr.Padre, pr.Figlio}] = true
	}
	return nil
}

// riscrivibile dice se una lettura puo' riscrivere una riga di proposta: aperta, oppure chiusa da un
// automatismo (scartata senza chi l'ha decisa: E33). Una decisione di una persona resta.
func riscrivibile(stato db.StatoProposta, decisoDa uuid.NullUUID) bool {
	return stato == db.StatoPropostaAperta || (stato == db.StatoPropostaScartata && !decisoDa.Valid)
}

// stesseSorgenti confronta due insiemi di sorgenti.
func stesseSorgenti(a, b map[string]uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// classificato e' la classificazione del server di un nodo, com'e' prima che l'operatore la corregga (K2).
func classificato(n NodoClassificato) map[string]any {
	return map[string]any{"codice": n.Codice, "rev": n.Rev, "origine": n.Origine, "famiglia": n.Famiglia,
		"confidenza": n.Confidenza, "dove": n.Dove}
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
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return Working{}, err
	}
	return NuovaWorking(rel), nil
}

func argNodo(thread, allegato uuid.UUID, sha string, pn PropostaNodo) db.UpsertComponentePropostaParams {
	ev, _ := json.Marshal(pn.Evidenza)
	arg := db.UpsertComponentePropostaParams{
		ThreadID: thread, AllegatoID: allegato, Sha256: sha, Chiave: pn.Chiave,
		NomeGrezzo: pn.NomeGrezzo, IDGrezzo: pn.IDGrezzo, Descrizione: testo(pn.Descrizione),
		Codice: testo(pn.Codice), Rev: testo(pn.Rev), Famiglia: pn.Famiglia,
		TipoProposto: db.NullTipoComponente{TipoComponente: pn.Tipo, Valid: pn.Tipo != ""},
		Confidenza:   int16(pn.Confidenza), Evidenza: ev, Stato: pn.Stato,
		// nessuna lettura da' un componente a un nodo (F5): la colonna la scrive solo una decisione
		ComponenteID: uuid.NullUUID{},
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

// stessoNodo: la riga dice gia' quello che il piano direbbe. Allora non si riscrive. La storia della riga
// non conta: la lettura non la scrive, e l'upsert la conserva (P30).
func stessoNodo(e db.ComponenteProposta, a db.UpsertComponentePropostaParams) bool {
	return e.NomeGrezzo == a.NomeGrezzo && e.IDGrezzo == a.IDGrezzo && e.Descrizione == a.Descrizione &&
		e.Codice == a.Codice && e.Rev == a.Rev && e.OrigineCodice == a.OrigineCodice && e.Famiglia == a.Famiglia &&
		e.TipoProposto == a.TipoProposto && e.Confidenza == a.Confidenza && e.Stato == a.Stato &&
		e.ComponenteID == a.ComponenteID && e.Nota == a.Nota && stessoJSON(e.Evidenza, a.Evidenza)
}

func stessaRelazione(e db.RelazioneProposta, a db.UpsertRelazionePropostaParams) bool {
	return e.Qta == a.Qta && e.Stato == a.Stato && e.Nota == a.Nota && stessoJSON(e.Evidenza, a.Evidenza)
}

// stessoJSON confronta due documenti JSON per contenuto: jsonb riordina le chiavi. La chiave «storia» non
// conta: e' della riga, non della lettura.
func stessoJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	for _, v := range []any{x, y} {
		if m, ok := v.(map[string]any); ok {
			delete(m, "storia")
		}
	}
	return reflect.DeepEqual(x, y)
}

func testo(s string) pgtype.Text {
	if strings.TrimSpace(s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// valutaLaRadice e' lo scrittore «struttura» della valutazione (Smistamento F4, A5.14.7). Prima era
// propostaDelDocumento, la D16, che con la radice riscriveva codice e fonte della proposta del documento;
// dal F5 il nome dice quello che fa: la radice dello STEP, letta con le regole del cliente di QUESTA RFQ, e'
// un'evidenza del codice del documento, e la valutazione si ricalcola con Valuta, dallo stesso file (il
// nome, i fatti dell'analisi) e con la radice al posto del primo PRODUCT; le colonne sono il suo riepilogo.
// Una radice diversa dal nome non riscrive la colonna del codice: la dimensione e' discorde e la colonna
// tiene il nome (D49). Una radice uguale al nome dipende dal nome e non fa una seconda fonte. Non da'
// nessuna identita': la riga resta una proposta aperta.
//
// La guardia e' quella di prima: solo una proposta aperta, non scritta dall'operatore, non assegnata a un
// componente. E si scrive solo se la lettura cambia: ApplicaStruttura si chiama a ogni risultato e a ogni
// riuso dei fatti, e la seconda volta non riscrive niente.
func valutaLaRadice(ctx context.Context, q *db.Queries, a db.Allegato, nodi []NodoClassificato, st worker.StrutturaSTEP,
	fatti json.RawMessage, m *classificazione.Motore) error {
	radice, ok := RadiceDelloStep(nodi, st)
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
	if p.Stato != db.StatoPropostaAperta || p.Fonte == db.FontePropostaOperatore || p.ComponenteID.Valid {
		return nil
	}
	direzione := ""
	if msg, err := q.GetMessaggio(ctx, a.MessaggioID); err == nil {
		direzione = string(msg.Direzione)
	}
	prima := classificazione.ValutazioneDellaRiga(string(p.TipoProposto), p.Codice.String, p.Rev.String, string(p.Fonte),
		int(p.Confidenza), p.Dettagli, a.NomeFile, a.Estensione.String)
	v := classificazione.Valuta(classificazione.IngressoFile{
		Da: classificazione.DaStruttura, NomeFile: a.NomeFile, Bytes: a.Bytes.Int64, Direzione: direzione,
		Interno: a.Origine == db.OrigineAllegatoManuale, Motore: m, Fatti: fatti, Radice: &radice,
		RispostaFornitore: prima.HaEvidenza("risposta_fornitore"),
	})
	if rp := v.Riepilogo(); !prima.Ricostruita && prima.StesseLetture(v) && string(p.TipoProposto) == rp.Tipo &&
		p.Codice.String == rp.Codice && p.Rev.String == rp.Rev && int(p.Confidenza) == rp.Confidenza && string(p.Fonte) == rp.Fonte {
		return nil
	}
	dett, rp := classificazione.ConValutazione(nil, v, time.Now())
	tipo, fonte := db.TipoDocumento(rp.Tipo), db.FonteProposta(rp.Fonte)
	if !tipo.Valid() || !fonte.Valid() {
		return fmt.Errorf("riepilogo della proposta di %s fuori enum: %+v", a.NomeFile, rp)
	}
	_, err = q.AggiornaValutazioneProposta(ctx, db.AggiornaValutazionePropostaParams{
		PropostaID: p.PropostaID, TipoProposto: tipo, Codice: testo(rp.Codice), Rev: testo(rp.Rev),
		Confidenza: int16(rp.Confidenza), Fonte: fonte, Dettagli: dett,
	})
	return err
}
