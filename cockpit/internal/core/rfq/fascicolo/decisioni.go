package fascicolo

// Le decisioni sulle proposte di struttura (Blocco 8, B8.5; addendum A1.1, A4.4).
//
// E' l'unico posto da cui una proposta diventa BOM working: una persona accetta un nodo (nasce o si
// ritrova un componente), un arco (nasce una relazione, o cambia la sua quantita'), una rimozione (un
// arco se ne va). Ogni gesto lavora nella transazione di chi lo chiama, con la riga della RFQ bloccata:
// due decisioni sulla stessa RFQ si mettono in fila, e il controllo dei cicli vede gli archi che
// l'altra ha appena scritto. Dopo il congelamento la working non si tocca (D26): lo si dice prima del
// muro del database.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// ChiaveRelazione identifica una relazione proposta: il file e la coppia di nodi.
type ChiaveRelazione struct {
	Allegato      uuid.UUID
	Padre, Figlio string
}

// ChiaveRimozione identifica una rimozione proposta.
type ChiaveRimozione struct {
	Step          uuid.UUID
	Padre, Figlio uuid.UUID
}

func uid(u uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: u, Valid: true} }

// prepara e' l'inizio di ogni gesto che cambia la working: la RFQ bloccata, poi il controllo di D26.
func prepara(ctx context.Context, q *db.Queries, thread uuid.UUID, gesto string) error {
	if err := bloccaThread(ctx, q, thread); err != nil {
		return err
	}
	return SeBloccata(ctx, q, thread, gesto)
}

// nomeNodo e' come si chiama un nodo proposto per chi legge: il codice, altrimenti il nome del file.
func nomeNodo(p db.ComponenteProposta) string {
	if c := strings.TrimSpace(p.Codice.String); c != "" {
		return c
	}
	if p.NomeGrezzo != "" {
		return "«" + p.NomeGrezzo + "»"
	}
	return p.Chiave
}

// AccettaNodo e' «questo nodo e' un componente». Se nella RFQ c'e' gia' un componente con quel codice
// il nodo lo ritrova (duplicato), e se era archiviato lo ripristina: stesso componente, stessa storia
// (A4.9). Altrimenti nasce il componente, con il tipo scelto o quello suggerito. Nessuna relazione:
// quelle si accettano a parte (A1.1). Le altre proposte aperte dello stesso codice si riconciliano.
func AccettaNodo(ctx context.Context, q *db.Queries, thread, proposta, utente uuid.UUID, tipo db.TipoComponente) (string, error) {
	if err := prepara(ctx, q, thread, "si accetta una proposta di struttura"); err != nil {
		return "", err
	}
	p, err := q.BloccaComponenteProposta(ctx, proposta)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ThreadID != thread) {
		return "", Rifiuto("la proposta non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	msg, err := accettaNodo(ctx, q, p, utente, tipo)
	return dopoLaDecisione(ctx, q, thread, msg, err)
}

// dopoLaDecisione ricalcola le rimozioni: un nodo riconosciuto, un codice scritto, un arco nuovo cambiano
// il confronto con lo STEP strutturale, e le proposte di rimozione devono dire sempre quello che la
// working e il file dicono adesso.
func dopoLaDecisione(ctx context.Context, q *db.Queries, thread uuid.UUID, msg string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if _, err := AggiornaTutteLeRimozioni(ctx, q, thread); err != nil {
		return "", err
	}
	return msg, nil
}

func accettaNodo(ctx context.Context, q *db.Queries, p db.ComponenteProposta, utente uuid.UUID, tipo db.TipoComponente) (string, error) {
	if p.Stato != db.StatoPropostaAperta {
		return "", Rifiuto(fmt.Sprintf("%s: la proposta è già decisa (%s)", nomeNodo(p), p.Stato))
	}
	codice := strings.TrimSpace(p.Codice.String)
	if codice == "" {
		return "", Rifiuto(fmt.Sprintf("%s non ha un codice: lo si scrive prima di accettarlo", nomeNodo(p)))
	}
	if tipo == "" {
		tipo = db.TipoComponenteSciolto
		if p.TipoProposto.Valid {
			tipo = p.TipoProposto.TipoComponente
		}
	}
	if !tipo.Valid() {
		return "", Rifiuto("tipo di componente non valido: " + string(tipo))
	}
	var comp uuid.UUID
	stato := db.StatoPropostaConfermata
	msg := ""
	c, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: p.ThreadID, Upper: codice})
	switch {
	case err == nil:
		comp, stato = c.ComponenteID, db.StatoPropostaDuplicato
		msg = fmt.Sprintf("%s è già nella BOM: la proposta lo ritrova.", c.Codice)
		if c.ArchiviatoIl != nil {
			if _, err := q.RipristinaComponente(ctx, c.ComponenteID); err != nil {
				return "", err
			}
			msg = fmt.Sprintf("%s era archiviato: ripristinato, con la sua storia.", c.Codice)
		}
		codice = c.Codice
	case errors.Is(err, pgx.ErrNoRows):
		n, err := q.InsertComponente(ctx, db.InsertComponenteParams{
			ThreadID: p.ThreadID, Codice: codice, Rev: p.Rev, Descrizione: p.Descrizione, Qta: 1,
			Tipo: tipo, Origine: db.OrigineComponenteStep, ConfermatoDa: utente,
		})
		if err != nil {
			return "", err
		}
		comp = n.ComponenteID
		msg = fmt.Sprintf("%s entra nella BOM come %s.", codice, tipo)
	default:
		return "", err
	}
	k, err := q.DecidiComponenteProposta(ctx, db.DecidiComponentePropostaParams{PropostaID: p.PropostaID, Stato: stato,
		ComponenteID: uid(comp), DecisoDa: uid(utente)})
	if err != nil {
		return "", err
	}
	if k != 1 {
		return "", Rifiuto(nomeNodo(p) + ": la proposta è stata decisa nel frattempo")
	}
	if _, err := q.RiconciliaProposteNodo(ctx, db.RiconciliaProposteNodoParams{ThreadID: p.ThreadID, Codice: codice,
		ComponenteID: uid(comp), Esclusa: p.PropostaID}); err != nil {
		return "", err
	}
	return msg, nil
}

// AccettaRelazione e' «il padre contiene il figlio, n volte». I due nodi devono essere gia' accettati o
// ritrovati: prima i nodi (A1.1). Un arco che chiude un ciclo si rifiuta; con piu' padri va bene.
//
// Se l'arco c'e' gia': con la stessa quantita' la proposta e' un duplicato; con una quantita' diversa e'
// una proposta di quantita' solo se il server l'ha fatta da una lettura completa contro la quantita' che
// la working ha ancora (evidenza.qta_working), e accettarla cambia la quantita'. Altrimenti e' la stessa
// coppia vista da un altro file: resta la quantita' che c'e', e la proposta diventa un duplicato con la
// nota, perche' la scelta e' dell'ingegnere e non del secondo file.
func AccettaRelazione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRelazione, utente uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si accetta una proposta di struttura"); err != nil {
		return "", err
	}
	msg, err := accettaRelazione(ctx, q, thread, k, utente, nil)
	return dopoLaDecisione(ctx, q, thread, msg, err)
}

// accettaRelazione e' il cuore di AccettaRelazione. archi, se non e' nil, sono gli archi attivi della
// working gia' letti da chi chiama, e vi si aggiunge l'arco scritto: accettare un file intero li legge
// una volta sola invece che una per arco (su uno STEP vero da 246 archi era la meta' del tempo).
func accettaRelazione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRelazione, utente uuid.UUID, archi *[]Arco) (string, error) {
	r, err := q.BloccaRelazioneProposta(ctx, db.BloccaRelazionePropostaParams{ThreadID: thread, AllegatoID: k.Allegato,
		PadreChiave: k.Padre, FiglioChiave: k.Figlio})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Rifiuto("la relazione proposta non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	pn, err := nodoDecisoPer(ctx, q, thread, k.Allegato, k.Padre)
	if err != nil {
		return "", err
	}
	fn, err := nodoDecisoPer(ctx, q, thread, k.Allegato, k.Figlio)
	if err != nil {
		return "", err
	}
	nome := nomeNodo(pn) + " → " + nomeNodo(fn)
	if r.Stato != db.StatoPropostaAperta {
		return "", Rifiuto(fmt.Sprintf("%s: la proposta è già decisa (%s)", nome, r.Stato))
	}
	for _, n := range []db.ComponenteProposta{pn, fn} {
		switch {
		case n.Stato == db.StatoPropostaScartata:
			return "", Rifiuto(fmt.Sprintf("%s: il nodo %s è stato scartato, la relazione non si può accettare", nome, nomeNodo(n)))
		case n.Stato == db.StatoPropostaAperta || !n.ComponenteID.Valid:
			return "", Rifiuto(fmt.Sprintf("%s: prima i nodi, %s non è ancora accettato", nome, nomeNodo(n)))
		}
	}
	padre, figlio := pn.ComponenteID.UUID, fn.ComponenteID.UUID
	for _, id := range []uuid.UUID{padre, figlio} {
		c, err := q.GetComponente(ctx, id)
		if err != nil {
			return "", err
		}
		if c.ArchiviatoIl != nil {
			return "", Rifiuto(fmt.Sprintf("%s: %s è archiviato, prima lo si ripristina", nome, c.Codice))
		}
	}
	if padre == figlio {
		return "", Rifiuto(fmt.Sprintf("%s: padre e figlio sono lo stesso componente", nome))
	}
	if archi == nil {
		letti, err := archiAttivi(ctx, q, thread)
		if err != nil {
			return "", err
		}
		archi = &letti
	}

	esistente, err := q.GetRelazione(ctx, db.GetRelazioneParams{PadreID: padre, FiglioID: figlio})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if giro := CreerebbeCiclo(*archi, padre, figlio); giro != nil {
			return "", Rifiuto(fmt.Sprintf("%s: la relazione chiuderebbe un ciclo (%s)", nome, ciclo(ctx, q, giro)))
		}
		if _, err := q.InsertRelazione(ctx, db.InsertRelazioneParams{ThreadID: thread, PadreID: padre, FiglioID: figlio,
			Qta: r.Qta, Origine: db.OrigineComponenteStep, ConfermatoDa: utente}); err != nil {
			return "", err
		}
		*archi = append(*archi, Arco{Padre: padre, Figlio: figlio})
		if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaConfermata, "", utente); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s ×%d entra nella BOM.", nome, r.Qta), riconciliaRelazione(ctx, q, thread, padre, figlio, r.Qta)
	case err != nil:
		return "", err
	}
	if esistente.Qta == r.Qta {
		if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaDuplicato, "", utente); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s ×%d è già nella BOM.", nome, r.Qta), nil
	}
	if qtaWorking(r.Evidenza) == esistente.Qta {
		if _, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{PadreID: padre, FiglioID: figlio, Qta: r.Qta, ConfermatoDa: utente}); err != nil {
			return "", err
		}
		if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaConfermata, "", utente); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s: quantità %d → %d.", nome, esistente.Qta, r.Qta), riconciliaRelazione(ctx, q, thread, padre, figlio, r.Qta)
	}
	nota := fmt.Sprintf("qta diversa: %d contro %d", r.Qta, esistente.Qta)
	if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaDuplicato, nota, utente); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s è già nella BOM con quantità %d: resta quella (%s).", nome, esistente.Qta, nota), nil
}

func archiAttivi(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]Arco, error) {
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return nil, err
	}
	archi := make([]Arco, len(rel))
	for i, x := range rel {
		archi[i] = Arco{Padre: x.PadreID, Figlio: x.FiglioID}
	}
	return archi, nil
}

// qtaWorking e' la quantita' della working contro cui il server ha fatto la proposta; 0 se non c'era.
func qtaWorking(evidenza []byte) int32 {
	var e struct {
		QtaWorking *int32 `json:"qta_working"`
	}
	if json.Unmarshal(evidenza, &e) != nil || e.QtaWorking == nil {
		return 0
	}
	return *e.QtaWorking
}

func nodoDecisoPer(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, chiave string) (db.ComponenteProposta, error) {
	n, err := q.GetComponentePropostaPerChiave(ctx, db.GetComponentePropostaPerChiaveParams{ThreadID: thread, AllegatoID: allegato, Chiave: chiave})
	if errors.Is(err, pgx.ErrNoRows) {
		return n, Rifiuto("il nodo " + chiave + " non c'è fra le proposte")
	}
	return n, err
}

func decidiRelazione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRelazione, stato db.StatoProposta, nota string, utente uuid.UUID) error {
	n, err := q.DecidiRelazioneProposta(ctx, db.DecidiRelazionePropostaParams{ThreadID: thread, AllegatoID: k.Allegato,
		PadreChiave: k.Padre, FiglioChiave: k.Figlio, Stato: stato, Nota: testo(nota), DecisoDa: uid(utente)})
	if err != nil {
		return err
	}
	if n != 1 {
		return Rifiuto("la relazione proposta è stata decisa nel frattempo")
	}
	return nil
}

func riconciliaRelazione(ctx context.Context, q *db.Queries, thread, padre, figlio uuid.UUID, qta int32) error {
	_, err := q.RiconciliaProposteRelazione(ctx, db.RiconciliaProposteRelazioneParams{ThreadID: thread, PadreID: uid(padre), FiglioID: uid(figlio), Qta: qta})
	return err
}

// ciclo racconta il cammino con i codici: «A → B → A».
func ciclo(ctx context.Context, q *db.Queries, giro []uuid.UUID) string {
	nomi := make([]string, 0, len(giro)+1)
	for _, id := range giro {
		nomi = append(nomi, codiceDi(ctx, q, id))
	}
	return strings.Join(append(nomi, nomi[0]), " → ")
}

func codiceDi(ctx context.Context, q *db.Queries, id uuid.UUID) string {
	if c, err := q.GetComponente(ctx, id); err == nil {
		return c.Codice
	}
	return id.String()
}

// AccettaSottoalbero accetta un nodo del file e tutto quello che gli pende sotto: prima i nodi, per
// profondita', poi gli archi fra nodi accettati. Una sola transazione, quella di chi chiama: un solo
// rifiuto (un nodo senza codice, un ciclo) annulla tutto (A1.1). I nodi gia' decisi restano come sono;
// un nodo scartato ferma la discesa sotto di lui, e i suoi archi restano aperti.
func AccettaSottoalbero(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, chiave string, utente uuid.UUID) (string, error) {
	return accettaGrafo(ctx, q, thread, allegato, []string{chiave}, utente)
}

// AccettaFile accetta tutto il file: il sottoalbero di ogni sua radice.
func AccettaFile(ctx context.Context, q *db.Queries, thread, allegato, utente uuid.UUID) (string, error) {
	nodi, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return "", err
	}
	rel, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return "", err
	}
	if len(nodi) == 0 {
		return "", Rifiuto("il file non ha proposte di struttura in questa RFQ")
	}
	figli := map[string]bool{}
	for _, r := range rel {
		figli[r.FiglioChiave] = true
	}
	var radici []string
	for _, n := range nodi {
		if !figli[n.Chiave] {
			radici = append(radici, n.Chiave)
		}
	}
	return accettaGrafo(ctx, q, thread, allegato, radici, utente)
}

func accettaGrafo(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, partenze []string, utente uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si accettano proposte di struttura"); err != nil {
		return "", err
	}
	nodi, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return "", err
	}
	rel, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return "", err
	}
	perChiave := map[string]db.ComponenteProposta{}
	for _, n := range nodi {
		perChiave[n.Chiave] = n
	}
	figli := map[string][]string{}
	for _, r := range rel {
		figli[r.PadreChiave] = append(figli[r.PadreChiave], r.FiglioChiave)
	}
	for k := range figli {
		sort.Strings(figli[k])
	}
	// in ampiezza: ogni nodo compare al livello in cui lo si incontra la prima volta
	visto := map[string]bool{}
	var ordine []string
	coda := append([]string(nil), partenze...)
	for _, c := range coda {
		visto[c] = true
	}
	for len(coda) > 0 {
		n := coda[0]
		coda = coda[1:]
		p, c := perChiave[n]
		if !c {
			return "", Rifiuto("il nodo " + n + " non c'è fra le proposte del file")
		}
		ordine = append(ordine, n)
		if p.Stato == db.StatoPropostaScartata {
			continue
		}
		for _, f := range figli[n] {
			if !visto[f] {
				visto[f] = true
				coda = append(coda, f)
			}
		}
	}
	nNodi, nArchi := 0, 0
	archi, err := archiAttivi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	for _, chiave := range ordine {
		p, err := q.BloccaComponenteProposta(ctx, perChiave[chiave].PropostaID)
		if err != nil {
			return "", err
		}
		if p.Stato != db.StatoPropostaAperta {
			continue
		}
		if _, err := accettaNodo(ctx, q, p, utente, ""); err != nil {
			return "", err
		}
		nNodi++
	}
	for _, r := range rel {
		if !visto[r.PadreChiave] || !visto[r.FiglioChiave] || r.Stato != db.StatoPropostaAperta {
			continue
		}
		if perChiave[r.PadreChiave].Stato == db.StatoPropostaScartata || perChiave[r.FiglioChiave].Stato == db.StatoPropostaScartata {
			continue
		}
		// Riletta adesso: un arco accettato poco fa nello stesso giro puo' averla gia' riconciliata (due
		// PRODUCT dello stesso codice sotto lo stesso padre finiscono sulla stessa coppia di componenti).
		k := ChiaveRelazione{Allegato: allegato, Padre: r.PadreChiave, Figlio: r.FiglioChiave}
		ora, err := q.BloccaRelazioneProposta(ctx, db.BloccaRelazionePropostaParams{ThreadID: thread, AllegatoID: allegato,
			PadreChiave: k.Padre, FiglioChiave: k.Figlio})
		if err != nil {
			return "", err
		}
		if ora.Stato != db.StatoPropostaAperta {
			continue
		}
		// Due PRODUCT con lo stesso codice, uno dentro l'altro (succede negli STEP veri: A1.1,
		// configurazioni): dopo i nodi sono lo stesso componente, e un pezzo non contiene se stesso. Il
		// fan-out lo scarta quando i due nodi sono gia' noti; qui lo si scarta con la stessa nota, e il giro
		// continua.
		pn, err := nodoDecisoPer(ctx, q, thread, allegato, k.Padre)
		if err != nil {
			return "", err
		}
		fn, err := nodoDecisoPer(ctx, q, thread, allegato, k.Figlio)
		if err != nil {
			return "", err
		}
		if pn.ComponenteID.Valid && pn.ComponenteID == fn.ComponenteID {
			if _, err := q.DecidiRelazioneProposta(ctx, db.DecidiRelazionePropostaParams{ThreadID: thread, AllegatoID: allegato,
				PadreChiave: k.Padre, FiglioChiave: k.Figlio, Stato: db.StatoPropostaScartata,
				Nota: testo("padre e figlio sono lo stesso componente: un pezzo non contiene se stesso")}); err != nil {
				return "", err
			}
			continue
		}
		if _, err := accettaRelazione(ctx, q, thread, k, utente, &archi); err != nil {
			return "", err
		}
		nArchi++
	}
	return dopoLaDecisione(ctx, q, thread, fmt.Sprintf("Accettati %d nodi e %d relazioni.", nNodi, nArchi), nil)
}

// ScartaNodo: «questo nodo non e' un pezzo della distinta». Le relazioni che lo toccano restano aperte e
// non si possono accettare: una decisione per riga, niente scarti a cascata (A1.1).
func ScartaNodo(ctx context.Context, q *db.Queries, thread, proposta, utente uuid.UUID) (string, error) {
	p, err := q.BloccaComponenteProposta(ctx, proposta)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ThreadID != thread) {
		return "", Rifiuto("la proposta non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	n, err := q.DecidiComponenteProposta(ctx, db.DecidiComponentePropostaParams{PropostaID: proposta, Stato: db.StatoPropostaScartata, DecisoDa: uid(utente)})
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", Rifiuto(nomeNodo(p) + ": la proposta è già decisa")
	}
	return dopoLaDecisione(ctx, q, thread, nomeNodo(p)+" scartato.", nil)
}

// ScartaRelazione: «questo arco non entra».
func ScartaRelazione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRelazione, utente uuid.UUID) (string, error) {
	if _, err := q.BloccaRelazioneProposta(ctx, db.BloccaRelazionePropostaParams{ThreadID: thread, AllegatoID: k.Allegato,
		PadreChiave: k.Padre, FiglioChiave: k.Figlio}); errors.Is(err, pgx.ErrNoRows) {
		return "", Rifiuto("la relazione proposta non è di questa RFQ")
	} else if err != nil {
		return "", err
	}
	if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaScartata, "", utente); err != nil {
		return "", err
	}
	return "Relazione scartata.", nil
}

// CodiceDelNodo scrive il codice di un nodo che il server non ha saputo classificare («Part1»): origine
// operatore, e da qui una riclassificazione non lo tocca piu' (A1.2). I limiti sono quelli di ogni
// codice che entra in una colonna.
func CodiceDelNodo(ctx context.Context, q *db.Queries, thread, proposta uuid.UUID, codice, rev string) (string, error) {
	codice, rev = strings.TrimSpace(codice), strings.TrimSpace(rev)
	if codice == "" {
		return "", Rifiuto("il codice non può essere vuoto")
	}
	if !classificazione.CodiceAmmissibile(codice) {
		return "", Rifiuto(fmt.Sprintf("il codice ha più di %d caratteri o caratteri non ammessi", classificazione.MaxCodice))
	}
	if rev != "" && !classificazione.RevAmmissibile(rev) {
		return "", Rifiuto(fmt.Sprintf("la revisione ha più di %d caratteri o caratteri non ammessi", classificazione.MaxRev))
	}
	p, err := q.BloccaComponenteProposta(ctx, proposta)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ThreadID != thread) {
		return "", Rifiuto("la proposta non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	n, err := q.SetCodiceComponenteProposta(ctx, db.SetCodiceComponentePropostaParams{PropostaID: proposta,
		Codice: pgtype.Text{String: codice, Valid: true}, Rev: testo(rev)})
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", Rifiuto(nomeNodo(p) + ": la proposta è già decisa, il codice si corregge sul componente")
	}
	return dopoLaDecisione(ctx, q, thread, fmt.Sprintf("«%s» ha il codice %s.", p.NomeGrezzo, codice), nil)
}

// AccettaRimozione toglie dalla working l'arco che lo STEP strutturale non contiene piu'. Solo la
// working: le baseline hanno le loro istantanee e non cambiano (A4.6). Il figlio non si archivia da
// solo: se resta senza padre compare come radice da sistemare (D29).
func AccettaRimozione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRimozione, utente uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si toglie un arco dalla BOM"); err != nil {
		return "", err
	}
	r, err := q.BloccaRimozioneProposta(ctx, db.BloccaRimozionePropostaParams{ThreadID: thread, StepDocumentoID: k.Step, PadreID: k.Padre, FiglioID: k.Figlio})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Rifiuto("la rimozione proposta non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	if r.Stato != db.StatoPropostaAperta {
		return "", Rifiuto("la rimozione proposta è già decisa")
	}
	nome := codiceDi(ctx, q, k.Padre) + " → " + codiceDi(ctx, q, k.Figlio)
	tolti, err := q.DeleteRelazione(ctx, db.DeleteRelazioneParams{PadreID: k.Padre, FiglioID: k.Figlio})
	if err != nil {
		return "", err
	}
	if _, err := q.DecidiRimozione(ctx, db.DecidiRimozioneParams{ThreadID: thread, StepDocumentoID: k.Step, PadreID: k.Padre,
		FiglioID: k.Figlio, Stato: db.StatoPropostaConfermata, DecisoDa: uid(utente)}); err != nil {
		return "", err
	}
	if tolti == 0 {
		return nome + ": l'arco non c'era già più nella BOM working.", nil
	}
	return nome + " tolto dalla BOM working.", nil
}

// ScartaRimozione: «l'arco resta». Uno scarta resta scartato: la stessa rimozione non si ripropone.
func ScartaRimozione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRimozione, utente uuid.UUID) (string, error) {
	n, err := q.DecidiRimozione(ctx, db.DecidiRimozioneParams{ThreadID: thread, StepDocumentoID: k.Step, PadreID: k.Padre,
		FiglioID: k.Figlio, Stato: db.StatoPropostaScartata, DecisoDa: uid(utente)})
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", Rifiuto("la rimozione proposta non c'è, o è già decisa")
	}
	return "Rimozione scartata: l'arco resta.", nil
}
