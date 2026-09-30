package fascicolo

// Le decisioni sulle proposte di struttura (Blocco 8, B8.5; addendum A1.1, A4.4).
//
// E' l'unico posto da cui una proposta diventa BOM working: una persona accetta un nodo (nasce o si
// ritrova un componente), un arco (nasce una relazione, o cambia la sua quantita'), una rimozione (un
// arco se ne va). Ogni gesto lavora nella transazione di chi lo chiama, con la riga della RFQ bloccata:
// due decisioni sulla stessa RFQ si mettono in fila, e il controllo dei cicli vede gli archi che
// l'altra ha appena scritto. Dopo il congelamento la working non si tocca (D26): lo si dice prima del
// muro del database.
//
// Smistamento F5 (A5.4.7, P9). Si accetta solo quello che sta nell'AUTORITA' di un file autorizzato: un nodo
// figlio diretto di una sorgente, un arco che parte da una sorgente verso un figlio deciso da una persona.
// La guida (il resto del file, e i file non autorizzati) si corregge come evidenza — «Correggi», «Scarta»,
// «Riapri il nodo» — ma non si accetta. Il ritrovamento per codice resta, perche' lo fa la persona che
// accetta; le riconciliazioni (le altre proposte con lo stesso codice che diventavano quel componente da
// sole) non ci sono piu': ogni nodo si decide dove lo si vede.

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
// quelle si accettano a parte (A1.1). Solo un figlio diretto di una sorgente autorizzata (F5).
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
	aut, _, _, err := autoritaDelFile(ctx, q, thread, p.AllegatoID)
	if err != nil {
		return "", err
	}
	msg, err := accettaNodo(ctx, q, p, utente, tipo, aut)
	if err == nil {
		err = rileggiIlFile(ctx, q, thread, p.AllegatoID)
	}
	return dopoLaDecisione(ctx, q, thread, msg, err)
}

// rileggiIlFile rilegge nella RFQ i fatti correnti del file che porta le proposte: dopo che una persona ha
// deciso un figlio diretto, l'arco dalla sorgente verso di lui tiene i conti con la working (A5.4.4:
// duplicato se la working ha gia' lo stesso arco, la quantita' diversa con la nota, «stesso componente»).
// Prima lo faceva la lettura da sola, perche' il codice uguale bastava a dire che il figlio era quel
// componente. Senza fatti correnti non fa niente: gli archi restano aperti, e si decidono uno per uno.
func rileggiIlFile(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) error {
	a, err := q.GetAllegato(ctx, allegato)
	if err != nil {
		return err
	}
	if !a.Sha256.Valid || a.Sha256.String == "" {
		return nil
	}
	af, err := q.GetAnalisiCorrente(ctx, a.Sha256.String)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return err
	}
	_, err = ApplicaStruttura(ctx, q, thread, a, af.Fatti, m)
	return err
}

// dopoLaDecisione ricalcola le rimozioni: un nodo riconosciuto, un codice scritto, un arco nuovo cambiano
// il confronto con i file autorizzati, e le proposte di rimozione devono dire sempre quello che la
// working e i file dicono adesso.
func dopoLaDecisione(ctx context.Context, q *db.Queries, thread uuid.UUID, msg string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if _, err := AggiornaTutteLeRimozioni(ctx, q, thread); err != nil {
		return "", err
	}
	return msg, nil
}

// rifiutoGuida dice perche' un nodo non si accetta: e' la sorgente (e' gia' C), oppure e' guida.
func rifiutoGuida(p db.ComponenteProposta, aut Autorita) error {
	if d, ok := aut.Sorgente(p.AllegatoID, p.Chiave); ok {
		return Rifiuto(fmt.Sprintf("%s è la radice dello STEP autorizzato di %s: è quel componente, non si accetta come un pezzo nuovo",
			nomeNodo(p), d.Componente.Codice))
	}
	return Rifiuto(fmt.Sprintf("%s è guida: nessuno STEP autorizzato lo propone come figlio diretto. Lo propone lo STEP autorizzato "+
		"del suo padre, se ne ha uno", nomeNodo(p)))
}

func accettaNodo(ctx context.Context, q *db.Queries, p db.ComponenteProposta, utente uuid.UUID, tipo db.TipoComponente, aut Autorita) (string, error) {
	if _, ok := aut.FiglioDiretto(p.AllegatoID, p.Chiave); !ok {
		return "", rifiutoGuida(p, aut)
	}
	if AgganciatoPerCodice(p) {
		// un aggancio per codice di prima dello Smistamento, dentro l'autorita': una persona lo conferma
		n, err := q.ConfermaNodoAgganciato(ctx, db.ConfermaNodoAgganciatoParams{PropostaID: p.PropostaID, DecisoDa: utente})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", Rifiuto(nomeNodo(p) + ": la proposta è stata decisa nel frattempo")
		}
		return fmt.Sprintf("%s confermato: è il componente %s già nella BOM.", nomeNodo(p), codiceDi(ctx, q, p.ComponenteID.UUID)), nil
	}
	if p.Stato != db.StatoPropostaAperta {
		return "", Rifiuto(fmt.Sprintf("%s: la proposta è già decisa (%s)", nomeNodo(p), p.Stato))
	}
	codice := strings.TrimSpace(p.Codice.String)
	if codice == "" {
		return "", Rifiuto(fmt.Sprintf("%s non ha un codice: lo si scrive prima di accettarlo", nomeNodo(p)))
	}
	if tipo == "" {
		// il tipo suggerito dalla lettura, mai commerciale: il make/buy lo decide solo una persona, scegliendo
		// il tipo (decisioni del 27/09 ter). Una riga di prima che lo proponesse vale come sciolto
		tipo = db.TipoComponenteSciolto
		if p.TipoProposto.Valid && p.TipoProposto.TipoComponente != db.TipoComponenteCommerciale {
			tipo = p.TipoProposto.TipoComponente
		}
	}
	if !tipo.Valid() {
		return "", Rifiuto("tipo di componente non valido: " + string(tipo))
	}
	var comp uuid.UUID
	stato := db.StatoPropostaConfermata
	msg := ""
	// il ritrovamento per codice: lo fa la persona che accetta, con il gesto (A5.4.7)
	c, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: p.ThreadID, Upper: codice})
	if errors.Is(err, pgx.ErrNoRows) {
		// il pezzo nato con il suffisso decorativo del cliente («X_PRT») e' lo stesso pezzo di «X»
		if a, ok, e := componenteConSuffisso(ctx, q, p.ThreadID, codice); e != nil {
			return "", e
		} else if ok {
			c, err = a, nil
		}
	}
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
	return msg, nil
}

// componenteConSuffisso cerca, per un cliente con suffissi decorativi, il componente il cui codice senza
// suffisso e' codice: il pezzo accettato come «X_PRT» prima della regola. ok = false se non c'e'.
//
// Un errore nel leggere le regole del cliente si restituisce: e' un errore del database (la RFQ e il suo
// cliente ci sono, la riga e' bloccata), e ignorarlo farebbe nascere «X» accanto a «X_PRT», due
// componenti per lo stesso pezzo, su una transazione che comunque non arriverebbe in fondo.
func componenteConSuffisso(ctx context.Context, q *db.Queries, thread uuid.UUID, codice string) (db.Componente, bool, error) {
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return db.Componente{}, false, err
	}
	if !m.HaSuffissi() {
		return db.Componente{}, false, nil
	}
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return db.Componente{}, false, err
	}
	sort.Slice(comp, func(i, j int) bool { return comp[i].Codice < comp[j].Codice })
	for _, c := range comp {
		if can, _ := m.Canonico(c.Codice, ""); !strings.EqualFold(c.Codice, codice) && strings.EqualFold(strings.TrimSpace(can), codice) {
			return c, true, nil
		}
	}
	return db.Componente{}, false, nil
}

// AccettaRelazione e' «il padre contiene il figlio, n volte». Il padre e' la sorgente di un file
// autorizzato (il componente per cui il file e' autorizzato), il figlio un nodo gia' deciso da una persona:
// prima i nodi (A1.1). Un arco della guida non si accetta (F5). Un arco che chiude un ciclo si rifiuta; con
// piu' padri va bene.
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
	aut, _, _, err := autoritaDelFile(ctx, q, thread, k.Allegato)
	if err != nil {
		return "", err
	}
	msg, err := accettaRelazione(ctx, q, thread, k, utente, nil, aut)
	return dopoLaDecisione(ctx, q, thread, msg, err)
}

// accettaRelazione e' il cuore di AccettaRelazione. archi, se non e' nil, sono gli archi attivi della
// working gia' letti da chi chiama, e vi si aggiunge l'arco scritto: accettare un file intero li legge
// una volta sola invece che una per arco (su uno STEP vero da 246 archi era la meta' del tempo).
func accettaRelazione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRelazione, utente uuid.UUID, archi *[]Arco, aut Autorita) (string, error) {
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
	padre, autorizzato := aut.ArcoAutorizzato(k)
	if !autorizzato {
		return "", Rifiuto(fmt.Sprintf("%s → %s è guida: %s non è la radice di uno STEP autorizzato. L'arco lo propone lo STEP autorizzato "+
			"di %s, se ne ha uno", nomeNodo(pn), nomeNodo(fn), nomeNodo(pn), nomeNodo(pn)))
	}
	nome := codiceDi(ctx, q, padre) + " → " + nomeNodo(fn)
	if r.Stato != db.StatoPropostaAperta {
		return "", Rifiuto(fmt.Sprintf("%s: la proposta è già decisa (%s)", nome, r.Stato))
	}
	figlio, deciso := DecisoDaUnaPersona(fn)
	switch {
	case fn.Stato == db.StatoPropostaScartata:
		return "", Rifiuto(fmt.Sprintf("%s: il nodo %s è stato scartato, la relazione non si può accettare", nome, nomeNodo(fn)))
	case !deciso:
		return "", Rifiuto(fmt.Sprintf("%s: prima i nodi, %s non è ancora accettato", nome, nomeNodo(fn)))
	}
	nome = codiceDi(ctx, q, padre) + " → " + codiceDi(ctx, q, figlio)
	var tipoPadre db.TipoComponente
	for _, id := range []uuid.UUID{padre, figlio} {
		c, err := q.GetComponente(ctx, id)
		if err != nil {
			return "", err
		}
		if c.ArchiviatoIl != nil {
			return "", Rifiuto(fmt.Sprintf("%s: %s è archiviato, prima lo si ripristina", nome, c.Codice))
		}
		if id == padre {
			tipoPadre = c.Tipo
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
		if tipoPadre == db.TipoComponenteSciolto {
			// E37 (Smistamento F5b, A5.4.1): un particolare con uno STEP autorizzato diventa un assieme al primo
			// figlio accettato, come nell'editor
			if err := q.SetTipoComponente(ctx, db.SetTipoComponenteParams{ComponenteID: padre, Tipo: db.TipoComponenteSottoassieme, ConfermatoDa: utente}); err != nil {
				return "", err
			}
			return fmt.Sprintf("%s ×%d entra nella BOM; %s diventa un assieme.", nome, r.Qta, codiceDi(ctx, q, padre)), nil
		}
		return fmt.Sprintf("%s ×%d entra nella BOM.", nome, r.Qta), nil
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
		return fmt.Sprintf("%s: quantità %d → %d.", nome, esistente.Qta, r.Qta), nil
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
		PadreChiave: k.Padre, FiglioChiave: k.Figlio, Stato: stato, Nota: testo(tagliaNota(nota)), DecisoDa: uid(utente)})
	if err != nil {
		return err
	}
	if n != 1 {
		return Rifiuto("la relazione proposta è stata decisa nel frattempo")
	}
	return nil
}

// maxNota e' la larghezza delle colonne nota delle proposte: varchar(200) in relazione_proposta (0018),
// componente_proposta e rimozione_proposta (0020).
const maxNota = 200

// tagliaNota porta una nota alla misura della colonna, contando i caratteri e non i byte. Una nota che
// contiene un nome — un file fino a 300 caratteri, il nome grezzo di un nodo — altrimenti fa fallire la
// decisione con un errore grezzo del database, invece di arrivare un po' piu' corta.
func tagliaNota(s string) string { return taglia(s, maxNota) }

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

// AccettaSottoalbero accetta dal file quello che la chiave indica, nell'autorita': per una sorgente i suoi
// figli diretti con gli archi, per un figlio diretto quel nodo con il suo arco. Sotto non si scende: i
// nipoti sono guida, e li propone lo STEP autorizzato del loro padre, o lo stesso file delegato a quel padre
// da una persona (F5, Domanda 1 = B). Una sola transazione, quella di chi chiama: un solo rifiuto (un nodo
// senza codice, un ciclo) annulla tutto (A1.1).
func AccettaSottoalbero(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, chiave string, utente uuid.UUID) (string, error) {
	return accettaGrafo(ctx, q, thread, allegato, []string{chiave}, utente)
}

// AccettaFile e' «accetta i figli diretti di C da questo STEP» (F5): i figli diretti di ogni sorgente del
// file, con gli archi. Prima era il file intero, con i nipoti.
func AccettaFile(ctx context.Context, q *db.Queries, thread, allegato, utente uuid.UUID) (string, error) {
	return accettaGrafo(ctx, q, thread, allegato, nil, utente)
}

// accettaGrafo accetta i figli diretti indicati (partenze nil = tutte le sorgenti del file): prima i nodi,
// poi gli archi dalle sorgenti.
func accettaGrafo(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, partenze []string, utente uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si accettano proposte di struttura"); err != nil {
		return "", err
	}
	aut, nodi, rel, err := autoritaDelFile(ctx, q, thread, allegato)
	if err != nil {
		return "", err
	}
	if len(nodi) == 0 {
		return "", Rifiuto("il file non ha proposte di struttura in questa RFQ")
	}
	perChiave := map[string]db.ComponenteProposta{}
	for _, n := range nodi {
		perChiave[n.Chiave] = n
	}
	sort.Slice(rel, func(i, j int) bool {
		if rel[i].PadreChiave != rel[j].PadreChiave {
			return rel[i].PadreChiave < rel[j].PadreChiave
		}
		return rel[i].FiglioChiave < rel[j].FiglioChiave
	})
	if partenze == nil {
		for _, n := range nodi {
			if _, ok := aut.Sorgente(allegato, n.Chiave); ok {
				partenze = append(partenze, n.Chiave)
			}
		}
		if len(partenze) == 0 {
			return "", Rifiuto("il file non è autorizzato a proporre i figli diretti di nessun componente: le sue proposte sono guida. " +
				"Si autorizza prima lo STEP per il suo componente")
		}
	}
	// i figli diretti scelti, in un ordine fisso
	scelti := map[string]bool{}
	var ordine []string
	aggiungi := func(k string) {
		if !scelti[k] {
			scelti[k] = true
			ordine = append(ordine, k)
		}
	}
	for _, k := range partenze {
		p, c := perChiave[k]
		if !c {
			return "", Rifiuto("il nodo " + k + " non c'è fra le proposte del file")
		}
		if _, ok := aut.Sorgente(allegato, k); ok {
			for _, r := range rel {
				if _, figlio := aut.FiglioDiretto(allegato, r.FiglioChiave); r.PadreChiave == k && figlio {
					aggiungi(r.FiglioChiave)
				}
			}
			continue
		}
		if _, ok := aut.FiglioDiretto(allegato, k); !ok {
			return "", rifiutoGuida(p, aut)
		}
		aggiungi(k)
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
		if p.Stato != db.StatoPropostaAperta && !AgganciatoPerCodice(p) {
			continue
		}
		if _, err := accettaNodo(ctx, q, p, utente, "", aut); err != nil {
			return "", err
		}
		nNodi++
	}
	if nNodi > 0 {
		// i figli appena decisi: gli archi dalla sorgente tengono i conti con la working prima di decidere
		if err := rileggiIlFile(ctx, q, thread, allegato); err != nil {
			return "", err
		}
	}
	for _, r := range rel {
		k := ChiaveRelazione{Allegato: allegato, Padre: r.PadreChiave, Figlio: r.FiglioChiave}
		padre, autorizzato := aut.ArcoAutorizzato(k)
		if !autorizzato || !scelti[r.FiglioChiave] || r.Stato != db.StatoPropostaAperta {
			continue
		}
		// Riletta adesso: un arco accettato poco fa nello stesso giro puo' averla gia' decisa.
		ora, err := q.BloccaRelazioneProposta(ctx, db.BloccaRelazionePropostaParams{ThreadID: thread, AllegatoID: allegato,
			PadreChiave: k.Padre, FiglioChiave: k.Figlio})
		if err != nil {
			return "", err
		}
		if ora.Stato != db.StatoPropostaAperta {
			continue
		}
		fn, err := nodoDecisoPer(ctx, q, thread, allegato, k.Figlio)
		if err != nil {
			return "", err
		}
		if fn.Stato == db.StatoPropostaScartata {
			continue // un nodo scartato: il suo arco resta, e si scarta a parte (una decisione per riga)
		}
		// Due PRODUCT con lo stesso codice, uno dentro l'altro (succede negli STEP veri: A1.1, configurazioni):
		// dopo i nodi sono lo stesso componente, e un pezzo non contiene se stesso. Il fan-out lo scarta quando
		// il figlio e' gia' deciso; qui lo si scarta con la stessa nota, e il giro continua.
		if f, ok := DecisoDaUnaPersona(fn); ok && f == padre {
			if _, err := q.DecidiRelazioneProposta(ctx, db.DecidiRelazionePropostaParams{ThreadID: thread, AllegatoID: allegato,
				PadreChiave: k.Padre, FiglioChiave: k.Figlio, Stato: db.StatoPropostaScartata,
				Nota: testo("padre e figlio sono lo stesso componente: un pezzo non contiene se stesso")}); err != nil {
				return "", err
			}
			continue
		}
		if _, err := accettaRelazione(ctx, q, thread, k, utente, &archi, aut); err != nil {
			return "", err
		}
		nArchi++
	}
	return dopoLaDecisione(ctx, q, thread, fmt.Sprintf("Accettati %s e %s.", quanti(nNodi, "nodo", "nodi"), quanti(nArchi, "relazione", "relazioni")), nil)
}

// quanti e' il numero con il nome, al singolare o al plurale: «1 nodo», «2 nodi».
func quanti(n int, uno, molti string) string {
	if n == 1 {
		return "1 " + uno
	}
	return fmt.Sprintf("%d %s", n, molti)
}

// ScartaNodo: «questo nodo non e' un pezzo della distinta». Le relazioni che lo toccano restano aperte e
// non si possono accettare: una decisione per riga, niente scarti a cascata (A1.1).
//
// Gli scarti e il codice di un nodo non cambiano la working, e per questo valgono anche con la BOM
// congelata; ma la RFQ la bloccano come gli altri gesti: cambiano che cosa il ricalcolo delle rimozioni
// vede, e due decisioni sulla stessa RFQ si mettono in fila. Valgono anche per un nodo di guida (F5,
// A5.4.5): sono correzioni dell'evidenza, che cambiano l'indice dei codici e non la BOM.
func ScartaNodo(ctx context.Context, q *db.Queries, thread, proposta, utente uuid.UUID) (string, error) {
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
	}
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

// RiapriNodo: «Riapri il nodo» (F5, A5.4.5). Un nodo scartato torna aperto, con la storia di chi l'aveva
// scartato e di chi lo riapre. Come lo scarto e' una correzione dell'evidenza: vale per un nodo di guida come
// per uno nell'autorita', e anche con la BOM congelata. Un nodo accettato non si riapre da qui.
//
// Giro 4, fase 4.4a.1b (ritoccata nella 4.4a.1br): un nodo che la conferma dell'albero ha tolto torna nell'albero
// proposto (riapriToltoNellAlbero). Si riaprono, chiuse dalla stessa conferma (la stessa firma nel segno): le righe
// del nodo in tutti i file, non solo quella del gesto; i padri nei file che la stessa conferma aveva tolto, risalendo,
// con tutte le loro righe (senza, il nodo resterebbe sotto un padre scartato e fuori dall'albero, mentre la frase
// diceva che tornava); gli archi degli stessi file che toccano quelle righe. La frase nomina i padri riaperti. Un padre
// scartato a mano, o da un'altra conferma, resta scartato: e' un'altra decisione, e si riapre con il suo gesto.
//
// Su una riga che un «Riapri» ha gia' riaperto cosi' (aperta, con quella riapertura in fondo alla storia) non si
// rifiuta: si ridice che cosa quel gesto ha riaperto, senza scrivere niente (giaRiapertoNellAlbero; fase 4.4a.1br,
// dalla verifica). Chi riapre un pezzo riga per riga — la pagina della Distinta chiama la rotta per ogni riga scartata
// del pezzo, e rifa' la pagina all'ultima — trovava le righe dopo la prima gia' riaperte dalla prima, e riceveva un
// rifiuto dopo una riapertura riuscita. Una riga aperta che nessun «Riapri» dell'albero ha riaperto resta un rifiuto.
func RiapriNodo(ctx context.Context, q *db.Queries, thread, proposta, utente uuid.UUID) (string, error) {
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
	}
	p, err := q.BloccaComponenteProposta(ctx, proposta)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ThreadID != thread) {
		return "", Rifiuto("la proposta non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	// un nodo tolto dalla conferma dell'albero (giro 4, fase 4.4a.1b; studio § 2.5): con lui torna quello che la stessa
	// conferma aveva chiuso e che gli serve per tornare nell'albero
	if s, ok := toltoNellAlbero(p); ok {
		ra, err := riapriToltoNellAlbero(ctx, q, thread, p, s, utente)
		if err != nil {
			return "", err
		}
		return dopoLaDecisione(ctx, q, thread, ra.frase(nomeNodo(p)), nil)
	}
	// una riga che un «Riapri» ha gia' riaperto con il suo nodo: la frase di quel gesto, e niente cambia
	if x, ok := riapertoNellAlbero(p.Stato, p.Evidenza, eventoNodoRiaperto); ok {
		ra, err := giaRiapertoNellAlbero(ctx, q, thread, p, x)
		if err != nil {
			return "", err
		}
		return ra.frase(nomeNodo(p)), nil
	}
	n, err := q.RiapriComponenteProposta(ctx, db.RiapriComponentePropostaParams{PropostaID: proposta, Utente: utente})
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", Rifiuto(fmt.Sprintf("%s non è scartato (%s): si riapre solo un nodo scartato", nomeNodo(p), p.Stato))
	}
	return dopoLaDecisione(ctx, q, thread, nomeNodo(p)+" riaperto: torna fra le proposte.", nil)
}

// ScartaRelazione: «questo arco non entra».
func ScartaRelazione(ctx context.Context, q *db.Queries, thread uuid.UUID, k ChiaveRelazione, utente uuid.UUID) (string, error) {
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
	}
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
// codice che entra in una colonna. Vale anche per un nodo di guida (F5): «Correggi il codice del nodo» e'
// una correzione dell'evidenza, e la classificazione del server resta accanto (K2).
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
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
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

// AccettaRimozione toglie dalla working l'arco che il file autorizzato non contiene piu'. Solo la
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
	// solo dall'autorita': una rimozione di un'autorizzazione che non vale (sospesa per un commerciale, in
	// conflitto, superata) non toglie niente, finche' una persona non la rende di nuovo valida (A5.4.6)
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return "", err
	}
	if !dich.RimozioneValida(k.Step, k.Padre) {
		motivo := "lo STEP non è più autorizzato per " + codiceDi(ctx, q, k.Padre)
		for _, x := range dich.DelComponente(k.Padre) {
			if x.Documento.Valid && x.Documento.UUID == k.Step && x.Problema != "" {
				motivo = x.Problema
			}
		}
		return "", Rifiuto(nome + ": la rimozione non si decide, " + motivo)
	}
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
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
	}
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
