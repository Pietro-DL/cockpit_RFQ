package fascicolo

// Autorizzare uno STEP per un componente, e revocarlo (Smistamento F5b; addendum A5.4.2, A5.4.6, A5.4.7;
// decisioni del 27/09 ter: Domanda 1 = B, Domanda 5 = B).
//
// L'autorizzazione e' una seconda decisione, dopo l'associazione del file al componente (P6): una persona dice
// «questo STEP propone i figli diretti di C». Qui ci sono le due meta' del gesto:
//   - EffettoAutorizzazione, l'anteprima: che cosa entra nell'autorita' (i figli diretti), che cosa resta guida,
//     quali rimozioni si apriranno, quale autorizzazione si revoca, se serve una presa d'atto, quali deleghe si
//     possono dare. Si calcola in GET e non scrive niente; la sua firma dice che cosa la persona ha visto;
//   - DichiaraStrutturale, la scrittura: sotto il lucchetto della RFQ ricalcola l'anteprima, e se la firma non
//     e' quella vista si rifiuta. Poi la marcatura (A5.4.2) sulla riga della sorgente, per un finito anche lo
//     STEP strutturale, e la rilettura del file.
//
// Il file si autorizza per QUALUNQUE componente attivo (U3), tranne un commerciale (Domanda 5 = B: lo STEP di
// un pezzo comprato resta guida; per autorizzarlo si cambia prima il tipo). L'autorita' va livello per livello
// (Domanda 1 = B): lo stesso file si puo' autorizzare anche per un componente annidato, con la DELEGA esplicita,
// dall'anteprima dell'autorizzazione del padre (una casella mai spuntata per ogni figlio diretto che nel file ha
// dei figli) o dalla scheda di quel componente in un secondo momento. La delega non scende da sola ai nipoti:
// ognuno vuole la sua. Non esiste un «accetta guida»: la delega e' il solo modo di usare una porzione profonda.
//
// Un componente, un'autorizzazione: autorizzare un file per C revoca, nella stessa transazione, quella che C
// aveva (un altro file, o la delega dal file del padre, e viceversa), e l'anteprima lo annuncia. Revocare il
// padre revoca le sue deleghe, con la storia. Con la BOM congelata non si autorizza e non si revoca (D26).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// RichiestaAutorizzazione e' quello che una persona chiede. Il file e' uno dei due: il documento dello STEP
// associato al componente (l'autorizzazione del suo file), oppure il nodo del componente nel file autorizzato
// del padre (la delega, dalla scheda del componente).
type RichiestaAutorizzazione struct {
	Componente uuid.UUID
	Documento  uuid.UUID
	Nodo       NodoFile
	// Sorgente: con piu' radici, il nodo che la persona indica come il componente. Mai scelto dal sistema.
	Sorgente string
	// Raggruppamenti: i nodi contenitore sotto la sorgente che la persona dichiara «sono il componente»: i loro
	// figli diventano figli diretti (la versione dichiarata di «È il prodotto» dell'editor, A5.4.2).
	Raggruppamenti []string
	// Deleghe: i figli diretti (chiavi) per cui lo stesso file si autorizza anche per il loro componente.
	Deleghe []string
	// Autorizza: la casella dell'autorizzazione, mai spuntata dal sistema (P6).
	Autorizza bool
	// PresaDAtto: la persona ha letto che il file chiama il nodo in un altro modo.
	PresaDAtto bool
	// Firma: quella dell'anteprima che la persona ha visto (Effetto.Firma).
	Firma string
}

// Delega dice se la richiesta e' una delega (il nodo nel file del padre).
func (r RichiestaAutorizzazione) Delega() bool { return r.Nodo.Chiave != "" }

// NodoEffetto e' un nodo del file come lo dice l'anteprima.
type NodoEffetto struct {
	Chiave string `json:"chiave"`
	Nome   string `json:"nome"`   // come lo si legge: il codice, o il nome nel file
	Codice string `json:"codice"` // "" = il file non gli da' un codice
	Qta    int32  `json:"qta"`    // per un figlio: quante volte e' nel padre
	Figli  int    `json:"figli"`  // quanti figli ha nel file
	// Deciso: il codice del componente che una persona ha deciso per il nodo, se l'ha fatto.
	Deciso   string           `json:"deciso"`
	Scartato bool             `json:"scartato"` // scartato da una persona
	Stato    db.StatoProposta `json:"stato"`
	decisoID uuid.UUID
}

// Raggruppamento e' un nodo contenitore (senza codice, con dei figli) sotto la sorgente: la persona puo'
// dichiararlo «e' il componente», e i suoi figli diventano figli diretti. Senza, restano guida.
type Raggruppamento struct {
	Nodo  NodoEffetto   `json:"nodo"`
	Figli []NodoEffetto `json:"figli"`
}

// DelegaPossibile e' un figlio diretto che nel file ha dei figli: la casella «Autorizza questo STEP anche per
// X», mai spuntata. Spenta dice perche' non si puo' (il nodo non e' ancora deciso, e' un commerciale, …).
type DelegaPossibile struct {
	Nodo         NodoEffetto `json:"nodo"`
	ComponenteID uuid.UUID   `json:"componente_id"`
	Codice       string      `json:"codice"`
	Spenta       string      `json:"spenta"`
	Revoca       []string    `json:"revoca"` // le autorizzazioni di X che la delega revoca, nominate: tutte
}

// Effetto e' l'anteprima dell'autorizzazione (r1 §3.9). Solo lettura.
type Effetto struct {
	Componente db.Componente
	Documento  db.Documento // il documento del file: per una delega, quello del padre
	Titolare   string       // per una delega: il codice del componente del file
	Allegato   uuid.UUID    // il portatore delle righe del file (Nil se le righe non ci sono ancora)
	Sha256     string
	NomeFile   string
	Delega     bool
	// Spento: perche' non si autorizza. Il riquadro e' spento e lo dice.
	Spento string
	// Gia: l'autorizzazione c'e' gia' cosi'.
	Gia string
	// Radici del file; con piu' radici la persona indica la sorgente, e finche' non lo fa Sorgente e' nil.
	Radici   []NodoEffetto
	Sorgente *NodoEffetto
	// PresaDAtto: la frase da prendere in atto, se il file chiama la sorgente in un altro modo.
	PresaDAtto     string
	Raggruppamenti []Raggruppamento
	Figli          []NodoEffetto // i figli diretti della sorgente: entrano nell'autorita'
	Deleghe        []DelegaPossibile
	Guida          int      // nodi del file che restano guida
	GuidaSotto     []string // «sotto 7120010: 2»
	Rimozioni      []string // gli archi della working che il file non ha
	// RimozioniSospese: perche' le rimozioni non si calcolano ancora.
	RimozioniSospese string
	Revoche          []string // le autorizzazioni che si revocano, nominate
	Avvisi           []string
	Firma            string
	revocare         []Dichiarazione
	righe            map[string]db.ComponenteProposta
	motivoParziale   string
}

// Autorizzabile dice se il gesto si puo' fare adesso: niente lo spegne, la sorgente e' indicata e non c'e'
// gia'.
func (e Effetto) Autorizzabile() bool { return e.Spento == "" && e.Sorgente != nil && e.Gia == "" }

// Bottone e' la frase del bottone, che nomina l'effetto (P6).
func (e Effetto) Bottone() string {
	s := fmt.Sprintf("Autorizza %s per %s", e.NomeFile, e.Componente.Codice)
	if e.Delega {
		s = fmt.Sprintf("Autorizza %s anche per %s", e.NomeFile, e.Componente.Codice)
	}
	return s + ": " + quanti(len(e.Figli), "figlio diretto", "figli diretti")
}

// EffettoAutorizzazione calcola l'anteprima. Non scrive niente: si chiama in GET, e di nuovo sotto il
// lucchetto nella scrittura, che confronta le firme. Un errore e' del database o una richiesta che non e'
// della RFQ; tutto il resto (un file che non si puo' autorizzare) e' Effetto.Spento.
func EffettoAutorizzazione(ctx context.Context, q *db.Queries, thread uuid.UUID, r RichiestaAutorizzazione) (Effetto, error) {
	var e Effetto
	c, err := q.GetComponente(ctx, r.Componente)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ThreadID != thread) {
		return e, Rifiuto("il componente non è di questa RFQ")
	}
	if err != nil {
		return e, err
	}
	e.Componente, e.Delega = c, r.Delega()
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return e, err
	}

	// prima il componente: un archiviato, un commerciale, una BOM congelata spengono il riquadro qualunque sia il file
	switch {
	case c.ArchiviatoIl != nil:
		e.Spento = c.Codice + " è archiviato: si ripristina prima di autorizzare un suo STEP"
	case c.Tipo == db.TipoComponenteCommerciale:
		// Domanda 5 = B: lo STEP di un pezzo comprato resta guida, ne' sorgente ne' delega
		e.Spento = c.Codice + " è un commerciale: il suo STEP resta guida; per autorizzarlo cambia prima il tipo"
	}
	if e.Spento == "" {
		if n, bloccata, err := WorkingBloccata(ctx, q, thread); err != nil {
			return e, err
		} else if bloccata {
			e.Spento = fmt.Sprintf("la BOM è congelata nella V%d: niente autorizzazioni né revoche finché non si apre una revisione", n)
		}
	}
	if e.Spento != "" {
		return e.firmata(), nil
	}
	// il file: il documento di C, oppure il nodo di C nel file del padre
	var sorgente string
	if e.Delega {
		sorgente, err = e.fileDellaDelega(ctx, q, thread, r.Nodo, dich)
	} else {
		err = e.fileProprio(ctx, q, thread, r.Documento)
	}
	if err != nil || e.Spento != "" {
		return e.firmata(), err
	}

	// la lettura del file: i fatti correnti, con la classificazione della RFQ; le righe, se ci sono, portano le
	// decisioni delle persone e i codici corretti dall'operatore
	af, err := q.GetAnalisiCorrente(ctx, e.Sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		e.Spento = e.NomeFile + " non ha ancora un'analisi corrente: si autorizza quando l'analisi è pronta"
		return e.firmata(), nil
	}
	if err != nil {
		return e, err
	}
	grezza, st, ok := struttura(af.Fatti)
	if !ok {
		e.Spento = "l'analisi di " + e.NomeFile + " non ha la struttura dello STEP"
		return e.firmata(), nil
	}
	if e.motivoParziale, err = q.MotivoParziale(ctx, &grezza); err != nil {
		return e, err
	}
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return e, err
	}
	nodi := map[string]NodoEffetto{}
	for _, n := range ClassificaNodi(m, *st) {
		nodi[n.Chiave] = NodoEffetto{Chiave: n.Chiave, Codice: n.Codice, Nome: nomeNodoClassificato(n), Stato: db.StatoPropostaAperta}
	}
	e.righe = map[string]db.ComponenteProposta{}
	if id, err := q.PortatoreDelFile(ctx, db.PortatoreDelFileParams{ThreadID: thread, Sha256: e.Sha256}); err == nil {
		e.Allegato = id
		righe, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: thread, AllegatoID: id})
		if err != nil {
			return e, err
		}
		codici, err := codiciDeiComponenti(ctx, q, thread)
		if err != nil {
			return e, err
		}
		for _, p := range righe {
			e.righe[p.Chiave] = p
			n, ok := nodi[p.Chiave]
			if !ok {
				continue
			}
			n.Codice, n.Nome, n.Stato = strings.TrimSpace(p.Codice.String), nomeNodo(p), p.Stato
			if comp, deciso := DecisoDaUnaPersona(p); deciso {
				n.Deciso, n.decisoID = codici[comp].Codice, comp
			}
			n.Scartato = p.Stato == db.StatoPropostaScartata && p.DecisoDa.Valid
			nodi[p.Chiave] = n
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return e, err
	}
	figliDi := map[string][]worker.RelazioneSTEP{}
	for _, rel := range st.Relazioni {
		figliDi[rel.Padre] = append(figliDi[rel.Padre], rel)
	}
	for k, n := range nodi {
		n.Figli = len(figliDi[k])
		nodi[k] = n
	}

	// la sorgente: la radice (con piu' radici quella che la persona indica), o il nodo della delega
	if e.Delega {
		n := nodi[sorgente]
		e.Sorgente = &n
		if n.Figli == 0 {
			e.Spento = "nel file " + n.Nome + " non ha figli: non c'è niente da autorizzare"
			return e.firmata(), nil
		}
	} else {
		for _, k := range st.Radici {
			if n, ok := nodi[k]; ok {
				e.Radici = append(e.Radici, n)
			}
		}
		switch {
		case len(e.Radici) == 0:
			e.Spento = e.NomeFile + " non ha una radice leggibile: non si autorizza"
			return e.firmata(), nil
		case r.Sorgente != "":
			for i := range e.Radici {
				if e.Radici[i].Chiave == r.Sorgente {
					n := e.Radici[i]
					e.Sorgente = &n
				}
			}
			if e.Sorgente == nil {
				return e, Rifiuto("il nodo indicato non è una radice di " + e.NomeFile)
			}
		case len(e.Radici) == 1:
			n := e.Radici[0]
			e.Sorgente = &n
		}
	}
	if e.Sorgente == nil {
		// piu' radici e nessuna indicata: nessuna scelta di partenza (U7)
		return e.firmata(), nil
	}
	s := *e.Sorgente
	cosa := "la radice"
	if e.Delega {
		cosa = "il nodo"
	}
	// P7: la sorgente decisa da una persona come un altro componente, o scartata da una persona
	switch {
	case s.decisoID != uuid.Nil && s.decisoID != c.ComponenteID:
		e.Spento = fmt.Sprintf("%s è già il componente %s: archivialo o scegli un altro file", cosa, s.Deciso)
		if e.Delega {
			e.Spento = fmt.Sprintf("il nodo è già il componente %s: si autorizza lo STEP per quello", s.Deciso)
		}
		return e.firmata(), nil
	case s.Scartato:
		e.Spento = fmt.Sprintf("%s %s è stato scartato da una persona: si riapre prima il nodo", cosa, s.Nome)
		return e.firmata(), nil
	}
	if p, ok := e.righe[s.Chiave]; ok && AgganciatoPerCodice(p) || (ok && p.Stato == db.StatoPropostaScartata && !p.DecisoDa.Valid) {
		e.Avvisi = append(e.Avvisi, fmt.Sprintf("%s era chiuso da un automatismo di prima: l'autorizzazione lo prende come %s, e com'era resta nella storia", s.Nome, c.Codice))
	}
	if s.Codice == "" || !strings.EqualFold(s.Codice, c.Codice) {
		e.PresaDAtto = fmt.Sprintf("il file chiama %s «%s»: per questa RFQ è %s", cosa, s.Nome, c.Codice)
	}

	// i figli diretti, i raggruppamenti possibili, le deleghe possibili
	comp, err := codiciDeiComponenti(ctx, q, thread)
	if err != nil {
		return e, err
	}
	figlioDi := func(rel worker.RelazioneSTEP) NodoEffetto {
		n := nodi[rel.Figlio]
		n.Qta = int32(rel.Qta)
		return n
	}
	// i raggruppamenti gia' dichiarati da una persona per C in questo file (la stessa autorizzazione, rifatta
	// o completata): sono C, e i loro figli sono figli diretti
	gia := map[string]bool{}
	for _, d := range dich.DelComponente(c.ComponenteID) {
		if d.Valida() && d.Sha256 == e.Sha256 && d.Delega() == e.Delega {
			for k, ruolo := range d.Sorgenti {
				if ruolo == RuoloRaggruppamento {
					gia[k] = true
				}
			}
		}
	}
	vistoFiglio := map[string]bool{}
	var aggiungiFigli func(padre string)
	aggiungiFigli = func(padre string) {
		for _, rel := range figliDi[padre] {
			if gia[rel.Figlio] {
				if !vistoFiglio[rel.Figlio] {
					vistoFiglio[rel.Figlio] = true
					aggiungiFigli(rel.Figlio)
				}
				continue
			}
			n := figlioDi(rel)
			if n.Codice == "" && n.Figli > 0 && n.decisoID == uuid.Nil && !n.Scartato {
				g := Raggruppamento{Nodo: n}
				for _, sotto := range figliDi[n.Chiave] {
					g.Figli = append(g.Figli, figlioDi(sotto))
				}
				e.Raggruppamenti = append(e.Raggruppamenti, g)
			}
			e.Figli = append(e.Figli, n)
		}
	}
	aggiungiFigli(s.Chiave)
	candidati := append([]NodoEffetto{}, e.Figli...)
	for _, g := range e.Raggruppamenti {
		candidati = append(candidati, g.Figli...)
	}
	vistoDelega := map[string]bool{}
	for _, n := range candidati {
		// un nodo che e' gia' C (un secondo PRODUCT con lo stesso codice) non si delega a nessuno
		if n.Figli == 0 || vistoDelega[n.Chiave] || n.decisoID == c.ComponenteID {
			continue
		}
		if len(e.Raggruppamenti) > 0 && raggruppamentoFra(e.Raggruppamenti, n.Chiave) {
			continue
		}
		vistoDelega[n.Chiave] = true
		e.Deleghe = append(e.Deleghe, delegaPossibile(n, c, comp, dich, e.Sha256))
	}

	// la guida: i nodi del file che nessuna sorgente valida, dopo il gesto, porta nell'autorita'
	sorgenti := map[string]bool{s.Chiave: true}
	for k, cc := range dich.Sorgenti(e.Sha256) {
		if cc != c.ComponenteID || gia[k] {
			sorgenti[k] = true
		}
	}
	autorita := map[string]bool{}
	for k := range sorgenti {
		autorita[k] = true
		for _, rel := range figliDi[k] {
			autorita[rel.Figlio] = true
		}
	}
	for k := range nodi {
		if !autorita[k] {
			e.Guida++
		}
	}
	for _, n := range e.Figli {
		if sorgenti[n.Chiave] {
			continue
		}
		if sotto := discendenti(figliDi, n.Chiave); sotto > 0 {
			e.GuidaSotto = append(e.GuidaSotto, fmt.Sprintf("sotto %s: %d", n.Nome, sotto))
		}
	}

	// le autorizzazioni che il gesto revoca: un componente, un'autorizzazione (anche la delega, e viceversa)
	for _, d := range dich.DelComponente(c.ComponenteID) {
		stessa := d.Sha256 == e.Sha256 && d.Delega() == e.Delega && d.Origine == OrigineSmistamento
		if stessa && d.Valida() {
			if _, ok := d.Sorgenti[s.Chiave]; ok {
				e.Gia = fmt.Sprintf("%s è già lo STEP autorizzato di %s", e.NomeFile, c.Codice)
				if e.Delega {
					e.Gia = fmt.Sprintf("%s è già autorizzato anche per %s (delega)", e.NomeFile, c.Codice)
				}
				continue
			}
		}
		if d.Origine == OrigineStepStrutturale && d.Sha256 == e.Sha256 && !e.Delega {
			continue // la forma di prima dello stesso file: la marcatura la rifa' e basta
		}
		e.revocare = append(e.revocare, d)
		e.Revoche = append(e.Revoche, fraseRevoca(d, dich))
	}

	// le rimozioni che si apriranno (A5.4.6): solo con tutti i figli diretti decisi da una persona e una lettura
	// completa; con dei raggruppamenti possibili dipendono da quello che la persona dichiara
	e.rimozioni(ctx, q, thread, *st, comp)

	if c.Tipo == db.TipoComponenteSciolto {
		e.Avvisi = append(e.Avvisi, c.Codice+" è un particolare: diventa un assieme al primo figlio accettato")
	}
	if c.Tipo == db.TipoComponenteFinito && !e.Delega {
		e.Avvisi = append(e.Avvisi, "è un prodotto finito: il file diventa anche il suo STEP strutturale")
	}
	if a, err := strutturaDiversa(ctx, q, thread, c, e.Sha256, e.Figli); err != nil {
		return e, err
	} else if a != "" {
		e.Avvisi = append(e.Avvisi, a)
	}
	return e.firmata(), nil
}

// fileProprio legge il documento di C: associato a C, uno STEP corrente, di una fonte del cliente.
func (e *Effetto) fileProprio(ctx context.Context, q *db.Queries, thread, documento uuid.UUID) error {
	if documento == uuid.Nil {
		e.Spento = "si sceglie lo STEP da autorizzare"
		return nil
	}
	d, err := q.GetDocumento(ctx, documento)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.ThreadID != thread) {
		return Rifiuto("il documento non è di questa RFQ")
	}
	if err != nil {
		return err
	}
	e.Documento, e.Sha256, e.NomeFile = d, d.Sha256, d.NomeFile
	switch {
	case !d.ComponenteID.Valid || d.ComponenteID.UUID != e.Componente.ComponenteID:
		e.Spento = fmt.Sprintf("%s non è associato a %s: prima si associa il file al componente", d.NomeFile, e.Componente.Codice)
	case !Step(d):
		e.Spento = fmt.Sprintf("%s non è un file STEP (un 3D .stp o .step)", d.NomeFile)
	case d.SostituitoDa.Valid:
		e.Spento = fmt.Sprintf("%s è stato sostituito: si autorizza un documento corrente", d.NomeFile)
	}
	if e.Spento != "" {
		return nil
	}
	nel, err := fileNelFlusso(ctx, q, thread, d.Sha256)
	if err != nil {
		return err
	}
	if !nel {
		e.Spento = d.NomeFile + " non viene da una fonte del cliente, o è stato messo da parte: la sua struttura non si legge nella RFQ"
	}
	return nil
}

// fileDellaDelega legge il nodo di C nel file autorizzato del padre: il file ha un documento corrente nella RFQ,
// il nodo e' figlio diretto di una sorgente valida dello stesso file. Restituisce la chiave del nodo.
func (e *Effetto) fileDellaDelega(ctx context.Context, q *db.Queries, thread uuid.UUID, nodo NodoFile, dich Dichiarazioni) (string, error) {
	p, err := q.GetComponentePropostaPerChiave(ctx, db.GetComponentePropostaPerChiaveParams{ThreadID: thread, AllegatoID: nodo.Allegato, Chiave: nodo.Chiave})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Rifiuto("il nodo indicato non è fra le proposte di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	e.Sha256 = p.Sha256
	a, err := q.GetAllegato(ctx, nodo.Allegato)
	if err != nil {
		return "", err
	}
	e.NomeFile = a.NomeFile
	d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: p.Sha256})
	if errors.Is(err, pgx.ErrNoRows) {
		e.Spento = e.NomeFile + " non è associato a nessun componente: si delega lo STEP autorizzato del padre"
		return p.Chiave, nil
	}
	if err != nil {
		return "", err
	}
	e.Documento, e.NomeFile = d, d.NomeFile
	if d.SostituitoDa.Valid {
		e.Spento = d.NomeFile + " è stato sostituito: si delega lo STEP corrente del padre"
		return p.Chiave, nil
	}
	if x, ok := dich.PerSha[p.Sha256]; ok {
		for _, y := range x {
			if _, sorgente := y.Sorgenti[p.Chiave]; sorgente && y.Componente.ComponenteID != e.Componente.ComponenteID {
				e.Spento = fmt.Sprintf("nel file il nodo è già la sorgente di %s", y.Componente.Codice)
				return p.Chiave, nil
			}
		}
	}
	// figlio diretto di una sorgente valida dello stesso file: la catena, un livello alla volta
	padri, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: nodo.Allegato})
	if err != nil {
		return "", err
	}
	sorg := dich.Sorgenti(p.Sha256)
	for _, r := range padri {
		if r.FiglioChiave != p.Chiave {
			continue
		}
		if cc, ok := sorg[r.PadreChiave]; ok && cc != e.Componente.ComponenteID {
			if x, ok := dich.Di(cc); ok {
				e.Titolare = x.Componente.Codice
			}
			return p.Chiave, nil
		}
	}
	e.Spento = "il nodo non è un figlio diretto di una sorgente autorizzata del file: si autorizza prima lo STEP per il suo padre, o si delega il padre"
	return p.Chiave, nil
}

// fileNelFlusso dice se il contenuto e' arrivato nella RFQ da una fonte del cliente (o da un caricamento
// interno) e non e' stato messo da parte: la sua struttura si legge nella RFQ (P27, E26).
func fileNelFlusso(ctx context.Context, q *db.Queries, thread uuid.UUID, sha string) (bool, error) {
	step, err := q.ListStepDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return false, err
	}
	for _, a := range step {
		if a.Sha256.String == sha {
			return true, nil
		}
	}
	return false, nil
}

// delegaPossibile dice la casella della delega per un figlio diretto n con dei figli nel file.
func delegaPossibile(n NodoEffetto, c db.Componente, comp map[uuid.UUID]db.Componente, dich Dichiarazioni, sha string) DelegaPossibile {
	d := DelegaPossibile{Nodo: n}
	if n.decisoID == uuid.Nil {
		d.Spenta = n.Nome + " non è ancora deciso: si accetta prima il nodo, poi si autorizza da qui o dalla scheda del suo componente"
		return d
	}
	x := comp[n.decisoID]
	d.ComponenteID, d.Codice = x.ComponenteID, x.Codice
	switch {
	case x.ComponenteID == c.ComponenteID:
		d.Spenta = "è lo stesso componente"
	case x.ArchiviatoIl != nil:
		d.Spenta = x.Codice + " è archiviato"
	case x.Tipo == db.TipoComponenteCommerciale:
		d.Spenta = x.Codice + " è un commerciale: il suo STEP resta guida; per autorizzarlo cambia prima il tipo"
	}
	if d.Spenta != "" {
		return d
	}
	for _, y := range dich.DelComponente(x.ComponenteID) {
		if y.Sha256 == sha && y.Delega() && y.Valida() {
			if _, ok := y.Sorgenti[n.Chiave]; ok {
				d.Spenta = "già autorizzato anche per " + x.Codice
				d.Revoca = nil
				return d
			}
		}
		// un componente, un'autorizzazione: la scrittura revoca TUTTE le dichiarazioni di X (valide, sospese, in
		// conflitto), e l'anteprima le nomina tutte
		d.Revoca = append(d.Revoca, fraseRevoca(y, dich))
	}
	return d
}

// fraseRevoca nomina l'autorizzazione che si revoca, con le deleghe che si porta dietro: «l'autorizzazione di
// 7120010.stp per 7120010, con le sue deleghe (7120011)».
func fraseRevoca(d Dichiarazione, dich Dichiarazioni) string {
	s := fmt.Sprintf("l'autorizzazione di %s per %s", d.NomeFile, d.Componente.Codice)
	if d.Delega() {
		s = fmt.Sprintf("la delega di %s per %s", d.NomeFile, d.Componente.Codice)
	}
	if d.Problema != "" {
		s += " (" + d.Problema + ")"
	}
	var deleghe []string
	for _, xx := range dich.tutte {
		for _, y := range xx {
			if y.Delega() && y.Sha256 == d.Sha256 && y.titolare.Valid && y.titolare.UUID == d.Componente.ComponenteID && !d.Delega() {
				deleghe = append(deleghe, y.Componente.Codice)
			}
		}
	}
	if len(deleghe) > 0 {
		sort.Strings(deleghe)
		s += ", con le sue deleghe (" + strings.Join(deleghe, ", ") + ")"
	}
	return s
}

// rimozioni calcola le rimozioni che il gesto aprira', o dice perche' non si calcolano ancora.
func (e *Effetto) rimozioni(ctx context.Context, q *db.Queries, thread uuid.UUID, st worker.StrutturaSTEP, comp map[uuid.UUID]db.Componente) {
	var daDecidere int
	nelFile := map[Arco]bool{}
	for _, n := range e.Figli {
		switch {
		case n.decisoID != uuid.Nil:
			nelFile[Arco{Padre: e.Componente.ComponenteID, Figlio: n.decisoID}] = true
		case !n.Scartato:
			daDecidere++
		}
	}
	switch {
	case e.motivoParziale != "":
		e.RimozioniSospese = "lo STEP è letto in parte (" + e.motivoParziale + "): le rimozioni restano sospese"
	case len(st.Radici) != 1:
		e.RimozioniSospese = fmt.Sprintf("lo STEP ha %d radici: le rimozioni restano sospese", len(st.Radici))
	case len(e.Raggruppamenti) > 0:
		e.RimozioniSospese = "le rimozioni si calcolano dopo l'autorizzazione, con i raggruppamenti dichiarati"
	case daDecidere > 0:
		e.RimozioniSospese = fmt.Sprintf("le rimozioni si calcolano quando %s da una persona", quanti(daDecidere, "figlio diretto è deciso", "figli diretti sono decisi"))
	}
	if e.RimozioniSospese != "" {
		return
	}
	w, err := leggiWorking(ctx, q, thread)
	if err != nil {
		e.RimozioniSospese = "la BOM working non si legge"
		return
	}
	for _, r := range RimozioniFigliDiretti(e.Componente.ComponenteID, w.Archi, nelFile) {
		e.Rimozioni = append(e.Rimozioni, fmt.Sprintf("%s → %s ×%d non è nel file: si proporrà di toglierlo",
			e.Componente.Codice, comp[r.Figlio].Codice, r.QtaWorking))
	}
}

// strutturaDiversa confronta i figli diretti del file con quelli che lo STEP autorizzato di un padre dice di
// C (A5.4.5, «confronta i file»): un avviso di discordanza, che non sceglie niente.
func strutturaDiversa(ctx context.Context, q *db.Queries, thread uuid.UUID, c db.Componente, sha string, figli []NodoEffetto) (string, error) {
	nodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return "", err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return "", err
	}
	perNodo := map[NodoFile]db.ListComponenteProposteThreadRow{}
	for _, n := range nodi {
		perNodo[NodoFile{n.ComponenteProposta.AllegatoID, n.ComponenteProposta.Chiave}] = n
	}
	questo := elencoFigli(figli)
	for _, n := range nodi {
		p := n.ComponenteProposta
		if p.Sha256 == sha {
			continue
		}
		if id, ok := DecisoDaUnaPersona(p); !ok || id != c.ComponenteID {
			continue
		}
		var altri []NodoEffetto
		for _, a := range archi {
			x := a.RelazioneProposta
			if x.AllegatoID == p.AllegatoID && x.PadreChiave == p.Chiave {
				f := perNodo[NodoFile{x.AllegatoID, x.FiglioChiave}].ComponenteProposta
				altri = append(altri, NodoEffetto{Nome: nomeNodo(f), Qta: x.Qta})
			}
		}
		if len(altri) == 0 {
			continue
		}
		if quello := elencoFigli(altri); quello != questo {
			return fmt.Sprintf("lo STEP %s dice che %s contiene %s; questo file dice %s", n.NomeFile, c.Codice, quello, questo), nil
		}
	}
	return "", nil
}

func elencoFigli(nn []NodoEffetto) string {
	var s []string
	for _, n := range nn {
		x := n.Nome
		if n.Qta > 1 {
			x += fmt.Sprintf(" ×%d", n.Qta)
		}
		s = append(s, x)
	}
	sort.Strings(s)
	if len(s) == 0 {
		return "niente"
	}
	return strings.Join(s, ", ")
}

func raggruppamentoFra(gg []Raggruppamento, chiave string) bool {
	for _, g := range gg {
		if g.Nodo.Chiave == chiave {
			return true
		}
	}
	return false
}

// discendenti conta i nodi sotto k nel file, una volta ciascuno.
func discendenti(figliDi map[string][]worker.RelazioneSTEP, k string) int {
	visto := map[string]bool{}
	coda := []string{k}
	for len(coda) > 0 {
		x := coda[0]
		coda = coda[1:]
		for _, r := range figliDi[x] {
			if !visto[r.Figlio] && r.Figlio != k {
				visto[r.Figlio] = true
				coda = append(coda, r.Figlio)
			}
		}
	}
	return len(visto)
}

func nomeNodoClassificato(n NodoClassificato) string {
	if n.Codice != "" {
		return n.Codice
	}
	if n.NomeGrezzo != "" {
		return "«" + n.NomeGrezzo + "»"
	}
	return n.Chiave
}

func codiciDeiComponenti(ctx context.Context, q *db.Queries, thread uuid.UUID) (map[uuid.UUID]db.Componente, error) {
	cc, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]db.Componente, len(cc))
	for _, c := range cc {
		out[c.ComponenteID] = c
	}
	return out, nil
}

// firmata calcola la firma dell'anteprima: che cosa la persona ha visto. Non i campi che cambiano da soli (lo
// stato della copia sul NAS del documento), si': il componente, il file, la sorgente, i figli con le loro
// decisioni, le deleghe, la guida, le rimozioni, le revoche, gli avvisi, e perche' e' spento. Non le righe in
// quanto tali: la scrittura le fa nascere, se mancano, com'erano nell'anteprima (tutte aperte), e la firma
// resta la stessa; una riga decisa nel frattempo cambia i figli, e con loro la firma.
func (e Effetto) firmata() Effetto {
	x := struct {
		Componente, Tipo, Documento, Sha, Titolare, Spento, Gia, Presa, Sospese string
		Archiviato, Sostituito, Delega                                          bool
		Radici, Figli                                                           []NodoEffetto
		Sorgente                                                                *NodoEffetto
		Raggruppamenti                                                          []Raggruppamento
		Deleghe                                                                 []DelegaPossibile
		Guida                                                                   int
		GuidaSotto, Rimozioni, Revoche, Avvisi                                  []string
	}{
		Componente: e.Componente.ComponenteID.String(), Tipo: string(e.Componente.Tipo), Documento: e.Documento.DocumentoID.String(),
		Sha: e.Sha256, Titolare: e.Titolare, Spento: e.Spento, Gia: e.Gia, Presa: e.PresaDAtto, Sospese: e.RimozioniSospese,
		Archiviato: e.Componente.ArchiviatoIl != nil, Sostituito: e.Documento.SostituitoDa.Valid, Delega: e.Delega,
		Radici: e.Radici, Figli: e.Figli, Sorgente: e.Sorgente, Raggruppamenti: e.Raggruppamenti, Deleghe: e.Deleghe,
		Guida: e.Guida, GuidaSotto: e.GuidaSotto, Rimozioni: e.Rimozioni, Revoche: e.Revoche, Avvisi: e.Avvisi,
	}
	b, _ := json.Marshal(x)
	h := sha256.Sum256(b)
	e.Firma = hex.EncodeToString(h[:12])
	return e
}

// ------------------------------------------------------------------ la scrittura

// DichiaraStrutturale autorizza il file per il componente, o lo delega (A5.4.7, passi 1–9). Una sola
// transazione, quella di chi chiama: la RFQ bloccata, l'anteprima ricalcolata e confrontata con quella vista,
// le revoche che il gesto annuncia, la marcatura della sorgente (e dei raggruppamenti e delle deleghe scelti),
// per un finito lo STEP strutturale, la rilettura del file e le rimozioni.
func DichiaraStrutturale(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, r RichiestaAutorizzazione) (string, error) {
	if err := prepara(ctx, q, thread, "si autorizza uno STEP"); err != nil {
		return "", err
	}
	// le righe del file: se non ci sono ancora (i fatti arrivati prima dell'aggancio), la lettura le fa adesso,
	// com'erano nell'anteprima (la classificazione della RFQ, tutte aperte)
	if !r.Delega() && r.Documento != uuid.Nil {
		if d, err := q.GetDocumento(ctx, r.Documento); err == nil && d.ThreadID == thread {
			if _, err := q.PortatoreDelFile(ctx, db.PortatoreDelFileParams{ThreadID: thread, Sha256: d.Sha256}); errors.Is(err, pgx.ErrNoRows) {
				if err := rileggiIlContenuto(ctx, q, thread, d.Sha256); err != nil {
					return "", err
				}
			} else if err != nil {
				return "", err
			}
		}
	}
	e, err := EffettoAutorizzazione(ctx, q, thread, r)
	if err != nil {
		return "", err
	}
	switch {
	case e.Spento != "":
		return "", Rifiuto(e.Spento)
	case e.Sorgente == nil:
		return "", Rifiuto(fmt.Sprintf("%s ha %d radici: si indica quale nodo è %s", e.NomeFile, len(e.Radici), e.Componente.Codice))
	case r.Firma == "" || r.Firma != e.Firma:
		return "", Rifiuto("quello che l'autorizzazione farebbe è cambiato mentre lo guardavi (un'analisi, una decisione di un collega): riapri l'anteprima e guarda di nuovo")
	case !r.Autorizza:
		return "", Rifiuto("si spunta «" + e.Bottone() + "»: l'autorizzazione è una decisione, la casella non nasce spuntata")
	case e.PresaDAtto != "" && !r.PresaDAtto:
		return "", Rifiuto("serve la presa d'atto: " + e.PresaDAtto)
	}
	scelti := map[string]bool{}
	for _, k := range r.Raggruppamenti {
		if !raggruppamentoFra(e.Raggruppamenti, k) {
			return "", Rifiuto("il nodo " + k + " non è un raggruppamento della sorgente")
		}
		scelti[k] = true
	}
	var deleghe []DelegaPossibile
	for _, k := range r.Deleghe {
		var trovata *DelegaPossibile
		for i := range e.Deleghe {
			if e.Deleghe[i].Nodo.Chiave == k {
				trovata = &e.Deleghe[i]
			}
		}
		switch {
		case trovata == nil:
			return "", Rifiuto("il nodo " + k + " non è un figlio diretto che si possa delegare")
		case trovata.Spenta != "":
			return "", Rifiuto(trovata.Nodo.Nome + ": " + trovata.Spenta)
		}
		deleghe = append(deleghe, *trovata)
	}

	c := e.Componente
	portatore := e.Allegato
	if portatore == uuid.Nil {
		return "", Rifiuto("le proposte di " + e.NomeFile + " non ci sono: si rilegge il file con «Rianalizza»")
	}
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return "", err
	}
	// un componente, un'autorizzazione: quella che C aveva, e quelle dei componenti delegati adesso
	revocate := map[string]bool{}
	var parti []string
	for _, d := range e.revocare {
		if err := revocaDichiarazione(ctx, q, thread, utente, d, "autorizzazione_sostituita", "cambiato lo STEP autorizzato", "cambiato lo STEP autorizzato"); err != nil {
			return "", err
		}
		revocate[d.Sha256] = true
	}
	for _, x := range deleghe {
		for _, d := range dich.DelComponente(x.ComponenteID) {
			if err := revocaDichiarazione(ctx, q, thread, utente, d, "autorizzazione_sostituita", "cambiato lo STEP autorizzato", "cambiato lo STEP autorizzato"); err != nil {
				return "", err
			}
			revocate[d.Sha256] = true
		}
	}
	if len(e.revocare) > 0 {
		parti = append(parti, "revocata "+strings.Join(e.Revoche, "; "))
	}

	// la marcatura: la sorgente, i raggruppamenti, le deleghe
	ruolo := RuoloRadice
	if e.Delega {
		ruolo = RuoloDelega
	}
	effetto := map[string]any{"figli_diretti": len(e.Figli), "guida": e.Guida, "rimozioni": len(e.Rimozioni)}
	if len(e.revocare) > 0 {
		effetto["sostituisce"] = e.revocare[0].NomeFile
	}
	marca := func(chiave string, comp uuid.UUID, come, presa string) error {
		return marcaSorgente(ctx, q, thread, portatore, chiave, comp, utente, Marcatura{V: 1, Ruolo: come,
			ComponenteID: uid(comp), DocumentoID: uid(e.Documento.DocumentoID), DichiaratoDa: uid(utente),
			DichiaratoIl: time.Now().UTC().Format(time.RFC3339), PresaDAtto: presa}, effetto)
	}
	if e.Gia == "" {
		if err := marca(e.Sorgente.Chiave, c.ComponenteID, ruolo, e.PresaDAtto); err != nil {
			return "", err
		}
	}
	for _, k := range r.Raggruppamenti {
		if err := marca(k, c.ComponenteID, RuoloRaggruppamento, ""); err != nil {
			return "", err
		}
	}
	for _, x := range deleghe {
		if err := marca(x.Nodo.Chiave, x.ComponenteID, RuoloDelega, ""); err != nil {
			return "", err
		}
	}
	if c.Tipo == db.TipoComponenteFinito && !e.Delega {
		// la materializzazione per un finito (A5.4.2): BOM04 e la FK controllano ancora
		if _, err := q.SetStepStrutturale(ctx, db.SetStepStrutturaleParams{ComponenteID: c.ComponenteID,
			StepStrutturaleID: uid(e.Documento.DocumentoID)}); err != nil {
			return "", err
		}
	}
	// le deleghe rimaste senza catena dopo le revoche se ne vanno con la loro storia
	cascata, err := revocaLeDelegheSenzaCatena(ctx, q, thread, utente, revocate)
	if err != nil {
		return "", err
	}
	// la rilettura: il file, con le sorgenti nuove, e i file revocati; poi le rimozioni
	revocate[e.Sha256] = true
	if err := rileggiIContenuti(ctx, q, thread, revocate); err != nil {
		return "", err
	}
	dopo, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s è lo STEP autorizzato di %s: %s nell'autorità, %s guida.", e.NomeFile, c.Codice,
		quanti(len(e.Figli), "figlio diretto", "figli diretti"), quanti(e.Guida, "nodo resta", "nodi restano"))
	if e.Delega {
		msg = fmt.Sprintf("%s è autorizzato anche per %s (delega, sotto %s nel file): %s nell'autorità.", e.NomeFile, c.Codice,
			e.Titolare, quanti(len(e.Figli), "figlio diretto", "figli diretti"))
	}
	if e.Gia != "" {
		msg = e.Gia + "."
	}
	if len(r.Raggruppamenti) > 0 {
		parti = append(parti, quanti(len(r.Raggruppamenti), "raggruppamento dichiarato", "raggruppamenti dichiarati")+" come "+c.Codice)
	}
	for _, x := range deleghe {
		parti = append(parti, "delegato anche a "+x.Codice)
	}
	if len(cascata) > 0 {
		parti = append(parti, "revocate con il padre le deleghe di "+strings.Join(cascata, ", "))
	}
	if x, ok := dopo.Di(c.ComponenteID); !ok {
		for _, y := range dopo.DelComponente(c.ComponenteID) {
			parti = append(parti, "l'autorizzazione non vale: "+y.Problema)
		}
	} else if es, err := AggiornaRimozioni(ctx, q, thread, x); err != nil {
		return "", err
	} else if es.Sospese != "" {
		parti = append(parti, "rimozioni non calcolate: "+es.Sospese)
	} else if es.Proposte > 0 {
		parti = append(parti, fmt.Sprintf("il file non contiene %s della BOM: proposti per la rimozione", quanti(es.Proposte, "arco", "archi")))
	}
	if len(parti) > 0 {
		msg += " " + strings.ToUpper(parti[0][:1]) + strings.Join(parti, "; ")[1:] + "."
	}
	return dopoLaDecisione(ctx, q, thread, msg, nil)
}

// marcaSorgente decide (se serve) e marca la riga della sorgente come il componente comp. Una riga aperta la
// decide la persona che autorizza; una chiusa da un automatismo di prima la prende, con la storia; una decisa
// da una persona come comp resta com'e'. Poi la marcatura.
func marcaSorgente(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, chiave string, comp, utente uuid.UUID, m Marcatura, effetto map[string]any) error {
	p, err := q.GetComponentePropostaPerChiave(ctx, db.GetComponentePropostaPerChiaveParams{ThreadID: thread, AllegatoID: allegato, Chiave: chiave})
	if errors.Is(err, pgx.ErrNoRows) {
		return Rifiuto("il nodo " + chiave + " non c'è fra le proposte del file")
	}
	if err != nil {
		return err
	}
	if p, err = q.BloccaComponenteProposta(ctx, p.PropostaID); err != nil {
		return err
	}
	switch {
	case p.Stato == db.StatoPropostaAperta:
		n, err := q.DecidiComponenteProposta(ctx, db.DecidiComponentePropostaParams{PropostaID: p.PropostaID, Stato: db.StatoPropostaDuplicato,
			ComponenteID: uid(comp), DecisoDa: uid(utente)})
		if err != nil {
			return err
		}
		if n != 1 {
			return Rifiuto(nomeNodo(p) + ": la proposta è stata decisa nel frattempo")
		}
	case !p.DecisoDa.Valid:
		n, err := q.PrendiNodoAutomatico(ctx, db.PrendiNodoAutomaticoParams{PropostaID: p.PropostaID, ComponenteID: comp, DecisoDa: utente})
		if err != nil {
			return err
		}
		if n != 1 {
			return Rifiuto(nomeNodo(p) + ": la proposta è stata decisa nel frattempo")
		}
	default:
		if c, ok := DecisoDaUnaPersona(p); !ok || c != comp {
			return Rifiuto(nomeNodo(p) + " è già deciso da una persona come un altro componente, o scartato")
		}
	}
	type conEffetto struct {
		Marcatura
		Effetto map[string]any `json:"effetto,omitempty"`
	}
	b, err := json.Marshal(conEffetto{Marcatura: m, Effetto: effetto})
	if err != nil {
		return err
	}
	n, err := q.MarcaDichiarazione(ctx, db.MarcaDichiarazioneParams{Marcatura: b, PropostaID: p.PropostaID, ComponenteID: uid(comp)})
	if err != nil {
		return err
	}
	if n != 1 {
		return Rifiuto(nomeNodo(p) + ": la marcatura non si scrive, la riga non è decisa come il componente")
	}
	return nil
}

// ------------------------------------------------------------------ la revoca

// RevocaStrutturale revoca l'autorizzazione del componente (A5.4.7): quelle del file sha, o tutte se sha e'
// vuoto (in un conflitto se ne revoca una). Le sorgenti marcate tornano aperte con la storia (una delega
// resta il componente che una persona ha deciso), gli archi chiusi da un automatismo si riaprono (E33), le
// rimozioni aperte si chiudono («revocato lo STEP autorizzato»), per un finito lo STEP strutturale torna
// vuoto, e le deleghe dello stesso file che restano senza catena se ne vanno con il padre. Le righe decise da
// persone restano: sono decisioni.
func RevocaStrutturale(ctx context.Context, q *db.Queries, thread, utente, comp uuid.UUID, sha, motivo string) (string, error) {
	if err := prepara(ctx, q, thread, "si revoca un'autorizzazione"); err != nil {
		return "", err
	}
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		motivo = "revocato lo STEP autorizzato"
	}
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return "", err
	}
	revocate := map[string]bool{}
	var file []string
	for _, d := range dich.DelComponente(comp) {
		if sha != "" && d.Sha256 != sha {
			continue
		}
		if err := revocaDichiarazione(ctx, q, thread, utente, d, "autorizzazione_revocata", motivo, "revocato lo STEP autorizzato"); err != nil {
			return "", err
		}
		revocate[d.Sha256] = true
		f := d.NomeFile
		if d.Delega() {
			f += " (delega)"
		}
		file = append(file, f)
	}
	if len(file) == 0 {
		return "", Rifiuto(c.Codice + " non ha uno STEP autorizzato")
	}
	cascata, err := revocaLeDelegheSenzaCatena(ctx, q, thread, utente, revocate)
	if err != nil {
		return "", err
	}
	if err := rileggiIContenuti(ctx, q, thread, revocate); err != nil {
		return "", err
	}
	sort.Strings(file)
	msg := fmt.Sprintf("Revocata l'autorizzazione di %s per %s: i figli diretti tornano guida; le decisioni già prese restano.",
		strings.Join(file, ", "), c.Codice)
	if len(cascata) > 0 {
		msg += " Revocate con lui le deleghe di " + strings.Join(cascata, ", ") + "."
	}
	return dopoLaDecisione(ctx, q, thread, msg, nil)
}

// revocaDichiarazione revoca una dichiarazione: la marcatura in storia con l'evento e il motivo (le radici e i
// raggruppamenti tornano aperti; una delega no), per un finito lo STEP strutturale vuoto, le rimozioni aperte
// chiuse con la nota, gli archi automatici del file riaperti. Una dichiarazione il cui documento e' stato
// sostituito si revoca come «sostituita_da»: la radice e' davvero il componente nella revisione di prima, e
// resta decisa; le chiusure del file vecchio le ha fatte la sostituzione, e restano.
func revocaDichiarazione(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, d Dichiarazione, evento, motivo, nota string) error {
	sostituita := false
	if d.Documento.Valid {
		doc, err := q.GetDocumento(ctx, d.Documento.UUID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		sostituita = err == nil && doc.SostituitoDa.Valid
	}
	if sostituita {
		evento = "sostituita_da"
	}
	if d.Origine == OrigineSmistamento {
		for _, k := range d.ChiaviSorgenti() {
			p, err := q.GetComponentePropostaPerChiave(ctx, db.GetComponentePropostaPerChiaveParams{ThreadID: thread, AllegatoID: d.Allegato, Chiave: k})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if _, err := q.RevocaDichiarazione(ctx, db.RevocaDichiarazioneParams{PropostaID: p.PropostaID,
				Riapri: !sostituita && d.Sorgenti[k] != RuoloDelega, Evento: evento, RevocataDa: uid(utente), Motivo: motivo}); err != nil {
				return err
			}
		}
	}
	c, err := q.GetComponente(ctx, d.Componente.ComponenteID)
	if err != nil {
		return err
	}
	if !d.Delega() && d.Documento.Valid && c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == d.Documento.UUID {
		if _, err := q.SetStepStrutturale(ctx, db.SetStepStrutturaleParams{ComponenteID: c.ComponenteID}); err != nil {
			return err
		}
	}
	if d.Documento.Valid {
		rim, err := q.ListRimozioniDiUnoStep(ctx, db.ListRimozioniDiUnoStepParams{ThreadID: thread, StepDocumentoID: d.Documento.UUID})
		if err != nil {
			return err
		}
		for _, r := range rim {
			if r.PadreID != c.ComponenteID || r.Stato != db.StatoPropostaAperta {
				continue
			}
			if _, err := q.ChiudiRimozione(ctx, db.ChiudiRimozioneParams{ThreadID: thread, StepDocumentoID: r.StepDocumentoID,
				PadreID: r.PadreID, FiglioID: r.FiglioID, Nota: pgtype.Text{String: tagliaNota(nota), Valid: true}}); err != nil {
				return err
			}
		}
	}
	if !sostituita {
		if _, err := q.RiapriArchiAutomaticiDiUnFile(ctx, db.RiapriArchiAutomaticiDiUnFileParams{ThreadID: thread, Sha256: pgtype.Text{String: d.Sha256, Valid: true}}); err != nil {
			return err
		}
	}
	return nil
}

// revocaLeDelegheSenzaCatena revoca le deleghe dei file toccati che, dopo le revoche, non arrivano piu' a una
// radice o a un raggruppamento autorizzati dello stesso file: «revocare il padre revoca le sue deleghe». Si
// ripete finche' ce ne sono (una delega revocata puo' lasciare senza catena quella del livello sotto).
// Restituisce i codici dei componenti le cui deleghe si sono revocate.
//
// Le deleghe si guardano fra le sospese (senza catena, o con il file del padre che non vale) e fra quelle da
// sistemare (la catena rotta, Smistamento G, sta li'): la delega di un file SOSTITUITO e' «superata» (problemaComune lo dice prima della catena), e
// senza questo resterebbe ferma nel gate dopo che il padre e' passato al file nuovo o e' stato revocato. Una
// delega da sistemare se ne va solo quando il suo titolare (il componente del documento del file) non ha
// piu', su quel file, un'autorizzazione propria: finche' c'e' (anche superata, in attesa della risposta alla
// sostituzione) la delega aspetta con lui.
//
// Smistamento, fase T: una delega con una sospensione registrata (il suo componente e' diventato commerciale)
// dice la sospensione prima della catena, e da sola non si riconoscerebbe. Per lei conta la catena del file
// come sarebbe senza sospensioni (CatenaSenzaSospensioni): se il padre che la teneva e' stato revocato, se ne va
// con lui; se il padre c'e' ancora, anche sospeso, resta sospesa e aspetta la scelta di una persona.
func revocaLeDelegheSenzaCatena(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, file map[string]bool) ([]string, error) {
	var out []string
	for giro := 0; giro < 50; giro++ {
		righe, err := q.ListDichiarazioniRfq(ctx, thread)
		if err != nil {
			return nil, fmt.Errorf("autorizzazioni della RFQ: %w", err)
		}
		dich, strutturale := ValutaDichiarazioni(righe), ValutaDichiarazioni(CatenaSenzaSospensioni(righe))
		n := 0
		for _, d := range append(append([]Dichiarazione{}, dich.Sospese...), dich.DaSistemare...) {
			if !d.Delega() || !file[d.Sha256] || d.Origine != OrigineSmistamento {
				continue
			}
			senzaPadre := d.SenzaCatena() || d.SenzaTitolare() || titolareRevocato(dich, d) || (d.Sospensione != nil && senzaCatenaStrutturale(strutturale, d))
			if !senzaPadre {
				continue
			}
			if err := revocaDichiarazione(ctx, q, thread, utente, d, "revocata_con_il_padre", "revocata l'autorizzazione del padre", "revocata l'autorizzazione del padre"); err != nil {
				return nil, err
			}
			out = append(out, d.Componente.Codice)
			n++
		}
		if n == 0 {
			sort.Strings(out)
			return out, nil
		}
	}
	return nil, errors.New("revoca delle deleghe: la catena non si chiude")
}

// CatenaSenzaSospensioni sono le righe delle autorizzazioni come sarebbero senza nessuna sospensione: niente
// sospensioni registrate, nessun componente archiviato o commerciale. Valutate, dicono se la catena di una
// delega c'e' nel file, a prescindere da chi e' sospeso adesso. Pura; le righe date non cambiano.
func CatenaSenzaSospensioni(righe []db.ListDichiarazioniRfqRow) []db.ListDichiarazioniRfqRow {
	tutte := map[uuid.UUID]bool{}
	for _, r := range righe {
		tutte[r.PropostaID] = true
	}
	out := SenzaSospensione(righe, tutte)
	for i := range out {
		out[i].Componente.ArchiviatoIl = nil
		if out[i].Componente.Tipo == db.TipoComponenteCommerciale {
			out[i].Componente.Tipo = db.TipoComponenteSottoassieme
		}
	}
	return out
}

// senzaCatenaStrutturale dice se la delega d, valutata senza sospensioni, non arriva piu' a una radice o a un
// raggruppamento autorizzati dello stesso file.
func senzaCatenaStrutturale(strutturale Dichiarazioni, d Dichiarazione) bool {
	for _, y := range strutturale.DelComponente(d.Componente.ComponenteID) {
		if y.Sha256 == d.Sha256 && y.Delega() {
			return y.SenzaCatena() || y.SenzaTitolare()
		}
	}
	return true
}

// titolareRevocato dice se il titolare della delega (il componente del documento del suo file) non ha piu',
// su quel file, un'autorizzazione propria, valida o no: il padre e' stato revocato.
func titolareRevocato(dich Dichiarazioni, d Dichiarazione) bool {
	if !d.titolare.Valid {
		return true
	}
	for _, y := range dich.DelComponente(d.titolare.UUID) {
		if !y.Delega() && y.Sha256 == d.Sha256 {
			return false
		}
	}
	return true
}

// rileggiIContenuti rilegge nella RFQ i file indicati (sha), in ordine: le sorgenti cambiate cambiano gli
// archi che tengono i conti con la working.
func rileggiIContenuti(ctx context.Context, q *db.Queries, thread uuid.UUID, file map[string]bool) error {
	sha := make([]string, 0, len(file))
	for s := range file {
		sha = append(sha, s)
	}
	sort.Strings(sha)
	for _, s := range sha {
		if err := rileggiIlContenuto(ctx, q, thread, s); err != nil {
			return err
		}
	}
	return nil
}

// rileggiIlContenuto applica alla RFQ i fatti correnti di un contenuto, dal primo allegato della RFQ che lo
// porta (ApplicaStruttura sceglie il portatore). Senza fatti correnti, o senza un allegato nel flusso, non fa
// niente. E' rileggiLoStep di prima, generalizzata a qualunque file (A5.4.7, passo 9).
func rileggiIlContenuto(ctx context.Context, q *db.Queries, thread uuid.UUID, sha string) error {
	// un file sostituito non si rilegge qui: le sue proposte le ha chiuse la sostituzione, e una rilettura le
	// riaprirebbe (E33) per un file che non e' piu' corrente
	if d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: sha}); err == nil && d.SostituitoDa.Valid {
		return nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	af, err := q.GetAnalisiCorrente(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	step, err := q.ListStepDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return err
	}
	for _, a := range step {
		if a.Sha256.String != sha {
			continue
		}
		m, err := MotoreDellaRfq(ctx, q, thread)
		if err != nil {
			return err
		}
		_, err = ApplicaStruttura(ctx, q, thread, a, af.Fatti, m)
		return err
	}
	return nil
}
