package fascicolo

// «Conferma l'albero» (giro 4, fase 4.4a.1b; domande 27 = A, 28 = A, 29b = A, 30 seconda risposta, 6a, 6b; studio
// docs/specs/studio_albero_distinta_29-09.md § 2.8 e § 2.9, con la verifica in coda). E' il solo gesto della Distinta
// che fa nascere componenti e relazioni dall'albero proposto: il confine umano del modello «albero proposto, poi
// confermato» (27 = A, «nessuna identita' senza una persona»).
//
// Due meta', come l'autorizzazione di F5b. Il riepilogo (albero_riepilogo.go) e' una lettura: dice che cosa la
// conferma farebbe, con la sua firma. La conferma, sotto il lucchetto della RFQ e con la BOM libera (prepara, D26),
// ricalcola lo stesso riepilogo sulla stessa bozza e si rifiuta senza scrivere niente se la firma non e' quella che la
// persona ha visto (l'albero, la working o la storia dei componenti sono cambiati) o se il riepilogo non e'
// confermabile (una proposta commerciale senza ✓ o ✗, un codice quasi uguale senza «diverso», una quantita' discorde
// senza scelta, un nodo senza codice, una bozza disegnata su un albero vecchio, un blocco sulla struttura). Poi scrive,
// in UNA transazione (quella di chi chiama), quello che il riepilogo dice, con gli stessi conti (pianoConferma):
//   - i pezzi che nascono (codice, tipo, revisione e descrizione dai file o dalla bozza) e quelli che si ritrovano per
//     lo stesso codice, solo quelli che il riepilogo elenca (P13; un archiviato si ripristina);
//   - i tipi: quello proposto dall'albero o scelto nella bozza, con chi ha confermato (confermato_da). Il particolare
//     commerciale nasce solo da un gesto di una persona, in due modi: il ✓ della proposta commerciale (o la tendina su
//     un nodo con la proposta, che e' lo stesso gesto), che passa da tipoConfermatoNellAlbero e lascia nel segno la
//     risposta; oppure la tendina su un nodo senza proposta, che e' il tipo scelto da una persona come nel modulo della
//     scheda (arriva gia' nel tipo del riepilogo, «scelto nella bozza» fra i tipi, senza un segno commerciale: non c'e'
//     una proposta a cui rispondere). Il tipo di un componente che c'e' cambia con cambiaTipo, le regole della scheda
//     (il commerciale sospende le autorizzazioni), e il riepilogo ne dice prima l'effetto (effettiDeiTipi);
//   - la revisione nuova di un componente che c'e' («7121003: rev A → rev B»): una revisione aggiorna lo stesso pezzo,
//     non ne crea uno nuovo (risposta 28);
//   - i legami con le loro quantita': prima si tolgono (la cascata, 29b = A), poi le quantita', poi si aggiungono;
//     il grafo finale e' senza cicli (il riepilogo), e collega li ricontrolla uno per uno contro la working intera;
//   - le rimozioni proposte dallo STEP: accettate se il legame va via, chiuse («tenuto») se resta;
//   - i componenti che restano senza padri: archiviati se hanno documenti o storia, eliminati se no (29b = A; per
//     questo gesto cambia D29, «i figli diventano radici da sistemare», e il riepilogo lo dice pezzo per pezzo);
//   - le righe degli STEP che l'albero copre, decise da una persona con il segno «confermato nell'albero»: dei nodi che
//     restano, le righe ancora da decidere (le aperte e gli agganci per codice di prima dello Smistamento, che il
//     riepilogo elenca fra i ritrovati: U5, P13); dei nodi che vanno via, quelle da decidere e quelle che una decisione
//     di prima aveva preso (siChiudeNellAlbero, le stesse che il riepilogo conta). Una riga chiusa da un automatismo
//     (senza chi l'ha decisa, e non un aggancio per codice), di un nodo o di un arco, resta com'e' (fase 4.4a.1br):
//     il riepilogo non la dice, e la rilettura deve poterla riscrivere (E33). La nota che la lettura aveva scritto su
//     una riga aperta resta, o va nella storia se la conferma ne scrive un'altra.
//
// Il segno (studio § 2.9, strada A) sta nelle colonne e negli stati che ci sono, senza migrazione: la riga diventa
// una decisione di una persona come quando la si accetta a mano (confermata se ha fatto nascere il componente,
// duplicato se lo ritrova, scartata con la nota se la conferma la toglie; deciso_da e deciso_il) e porta in
// evidenza.albero chi, quando, la firma del riepilogo e il ✓ o il ✗ della proposta commerciale; la riga di un arco
// porta anche i due componenti del legame (la radice del prodotto non e' una riga decisa). Una riga decisa da una
// persona le letture non la riscrivono piu' (UpsertComponenteProposta, UpsertRelazioneProposta: solo le aperte e
// quelle chiuse da un automatismo), quindi il segno resta; «Riapri il nodo» lo porta nella storia e, per un nodo che la
// conferma ha tolto, riapre le righe che la stessa conferma aveva chiuso: quelle del nodo in ogni file (il segno di
// una riga tolta dice il nodo), quelle dei padri nei file tolti con lui, e gli archi che le toccano (riapriToltoNellAlbero),
// cosi' il nodo torna nell'albero proposto (studio § 2.5); su una riga che lo stesso gesto ha gia' riaperto ridice che
// cosa ha riaperto, senza rifiutare (giaRiapertoNellAlbero). Lo leggono:
//   - F7 (Provenienza e ArchiDecisiNellAutorita, riapertura.go): una riga con il segno conta come una decisione presa
//     nell'autorita' di un file autorizzato, e il pezzo e l'arco non sono «da rivedere»;
//   - l'indice di F8, con le stesse etichette (derivati.daRivedere e arcoDeciso): i pezzi dell'albero confermato
//     sono «decisi», e i loro file si preselezionano come dopo un'accettazione nello STEP autorizzato;
//   - il gate (GateStrutturale) conta come decisa una riga con chi l'ha decisa: le righe nell'autorita' di uno STEP
//     autorizzato che la conferma copre non lo fermano piu', e la guida decisa non e' piu' un avviso.
// Senza il segno la strada era una sola: accettare i nodi come accettaNodo (origine step, fuori dall'autorita') e
// lasciare che F7 li segnasse «da rivedere» e che F8 non preselezionasse niente; oppure farli nascere come carte a
// mano, perdendo il legame con i file e lasciando aperte le righe degli STEP (il Bug 2).
//
// Perche' una funzione nuova e non ApplicaStrutturaVoluta allargata: il contratto di quella e' l'editor del Fascicolo
// (una radice, solo l'autorita', gli archi e le carte che l'editor ha mostrato, P9 e la prova 97: nessun nodo di guida
// entra dall'editor). La conferma dell'albero ha un altro confine, il riepilogo firmato, che dice gia' tutto quello che
// si scrive (una foresta, i nodi di guida, i tipi, i codici, le revisioni, la cascata): allargare l'editor avrebbe
// tolto la sua guardia per dargli quella dell'albero. Le due condividono l'interno — prepara, collega e scollega,
// cambiaTipo, ArchiviaComponente e RimuoviComponente, la chiusura delle rimozioni, dopoLaDecisione — cosi' le regole
// della working sono una sola.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// ChiaveSegnoAlbero e' la chiave di evidenza (componente_proposta, relazione_proposta) dove la conferma dell'albero
// lascia il suo segno.
const ChiaveSegnoAlbero = "albero"

// NotaToltoNellAlbero e' la nota delle righe che la conferma dell'albero chiude perche' la bozza le toglie.
const NotaToltoNellAlbero = "tolto nell'albero confermato"

// SegnoAlbero e' il segno «confermato nell'albero» su una riga decisa dalla conferma.
type SegnoAlbero struct {
	Da    uuid.UUID `json:"da"`
	Il    string    `json:"il"`
	Firma string    `json:"firma"`
	// Padre e Figlio: i componenti del legame, sulla riga di un arco che la conferma tiene.
	Padre  uuid.NullUUID `json:"padre"`
	Figlio uuid.NullUUID `json:"figlio"`
	// Nodo: sulla riga di un nodo che la conferma toglie, la chiave del nodo nell'albero («cod:<CODICE>»): le righe dello
	// stesso pezzo negli altri file hanno la stessa, e «Riapri il nodo» le riapre insieme (fase 4.4a.1br).
	Nodo string `json:"nodo,omitempty"`
	// Commerciale: la risposta di una persona alla proposta commerciale del nodo.
	Commerciale *SegnoCommerciale `json:"commerciale,omitempty"`
}

// SegnoCommerciale e' il ✓ o il ✗ sulla proposta commerciale, con il motivo della proposta: cosi' il censimento conta
// anche i «mancati», le proposte che l'ingegnere ha scartato (risposta 30).
type SegnoCommerciale struct {
	Risposta string `json:"risposta"` // RispostaSi, RispostaNo
	Tendina  bool   `json:"tendina,omitempty"`
	Motivo   string `json:"motivo"`
}

// ConfermatoNellAlbero dice se la riga di un nodo e' una decisione di una persona presa con la conferma dell'albero.
func ConfermatoNellAlbero(p db.ComponenteProposta) bool {
	_, deciso := DecisoDaUnaPersona(p)
	return deciso && haChiave(p.Evidenza, ChiaveSegnoAlbero)
}

// ArcoConfermatoNellAlbero dice se la riga di un arco e' un legame tenuto dalla conferma dell'albero, e quale: i due
// componenti che il segno dice.
func ArcoConfermatoNellAlbero(r db.RelazioneProposta) (CoppiaDiComponenti, bool) {
	if !r.DecisoDa.Valid || (r.Stato != db.StatoPropostaConfermata && r.Stato != db.StatoPropostaDuplicato) {
		return CoppiaDiComponenti{}, false
	}
	var e struct {
		Albero *SegnoAlbero `json:"albero"`
	}
	if json.Unmarshal(r.Evidenza, &e) != nil || e.Albero == nil || !e.Albero.Padre.Valid || !e.Albero.Figlio.Valid {
		return CoppiaDiComponenti{}, false
	}
	return CoppiaDiComponenti{e.Albero.Padre.UUID, e.Albero.Figlio.UUID}, true
}

// toltoNellAlbero dice se la riga di un nodo l'ha chiusa una conferma dell'albero perche' la bozza lo toglieva, con il
// segno di quella conferma (la firma del riepilogo e il nodo): «Riapri il nodo» riapre con lei quello che la stessa
// conferma aveva chiuso (riapriToltoNellAlbero).
func toltoNellAlbero(p db.ComponenteProposta) (SegnoAlbero, bool) {
	if p.Stato != db.StatoPropostaScartata || !p.DecisoDa.Valid || p.Nota.String != NotaToltoNellAlbero {
		return SegnoAlbero{}, false
	}
	var e struct {
		Albero *SegnoAlbero `json:"albero"`
	}
	if json.Unmarshal(p.Evidenza, &e) != nil || e.Albero == nil || e.Albero.Firma == "" {
		return SegnoAlbero{}, false
	}
	return *e.Albero, true
}

// nodoTolto e' il nodo di una riga che la conferma con quella firma ha tolto (nodoDelSegno).
func nodoTolto(p db.ComponenteProposta, firma string) (string, bool) {
	s, ok := toltoNellAlbero(p)
	if !ok || s.Firma != firma {
		return "", false
	}
	return nodoDelSegno(s, p), true
}

// nodoDelSegno e' il nodo che il segno di una riga tolta dice, o la riga stessa per una riga chiusa prima che il segno
// dicesse il nodo (fase 4.4a.1b).
func nodoDelSegno(s SegnoAlbero, p db.ComponenteProposta) string {
	if s.Nodo != "" {
		return s.Nodo
	}
	return "riga:" + p.PropostaID.String()
}

// Gli eventi della storia di una riga riaperta (RiapriComponenteProposta, RiapriArchiToltiNellAlbero).
const (
	eventoNodoRiaperto = "nodo_riaperto"
	eventoArcoRiaperto = "arco_riaperto"
)

// riapertura e' la riapertura di una riga tolta da una conferma dell'albero, come la storia della riga la ricorda: il
// segno di quella conferma e l'istante (riaperto_il, il now() della transazione: lo stesso per tutte le righe che lo
// stesso gesto ha riaperto).
type riapertura struct {
	segno SegnoAlbero
	il    string
}

// riapertoNellAlbero dice se una riga (di un nodo o di un arco: evento) e' aperta perche' «Riapri il nodo» l'ha
// riaperta annullando una conferma dell'albero, e con quale riapertura: l'ultimo evento della sua storia e' quello, con
// il segno e la nota della conferma. Dopo, una decisione la chiude (e non e' piu' aperta) e una rilettura lascia la
// storia com'e' (UpsertComponenteProposta): finche' la riga e' aperta, la riapertura e' l'ultima cosa che le e' successa.
func riapertoNellAlbero(stato db.StatoProposta, evidenza []byte, evento string) (riapertura, bool) {
	if stato != db.StatoPropostaAperta {
		return riapertura{}, false
	}
	var e struct {
		Storia []json.RawMessage `json:"storia"`
	}
	if json.Unmarshal(evidenza, &e) != nil || len(e.Storia) == 0 {
		return riapertura{}, false
	}
	var ev struct {
		Evento string       `json:"evento"`
		Albero *SegnoAlbero `json:"albero"`
		Nota   *string      `json:"nota"`
		Il     string       `json:"riaperto_il"`
	}
	if json.Unmarshal(e.Storia[len(e.Storia)-1], &ev) != nil || ev.Evento != evento || ev.Albero == nil || ev.Albero.Firma == "" ||
		ev.Nota == nil || *ev.Nota != NotaToltoNellAlbero || ev.Il == "" {
		return riapertura{}, false
	}
	return riapertura{segno: *ev.Albero, il: ev.Il}, true
}

// stessoGesto dice se due riaperture le ha fatte lo stesso gesto: la stessa conferma annullata, nello stesso istante.
func (x riapertura) stessoGesto(y riapertura) bool {
	return x.segno.Firma == y.segno.Firma && x.il == y.il
}

// riaperturaAlbero e' quello che «Riapri il nodo» ha riaperto di una conferma dell'albero: le righe del nodo, i padri
// riaperti con lui, gli archi.
type riaperturaAlbero struct {
	righe, archi int
	padri        []string
}

// rigaNelFile e' una riga di uno STEP: il file e la chiave del nodo nel file.
type rigaNelFile struct {
	allegato uuid.UUID
	chiave   string
}

// camminoTolto e' quello che «Riapri il nodo» riporta di una conferma dell'albero, partendo dalla riga p: il nodo di p
// (il primo in ordine) e, risalendo negli stessi file, i padri tolti dalla stessa conferma, con le loro righe (perNodo)
// e i nomi dei padri.
type camminoTolto struct {
	ordine  []string
	perNodo map[string][]db.ComponenteProposta
	padri   []string
}

// camminoDaRiaprire calcola il cammino di p. nodoDi dice il nodo di una riga, se la riga fa parte del gesto: tolta
// dalla conferma che si annulla (per riaprire), o gia' riaperta dallo stesso gesto di p (per ridire che cosa ha
// riaperto, senza scrivere). p ne fa parte: chi chiama l'ha letta cosi'.
func camminoDaRiaprire(righe []db.ListComponenteProposteThreadRow, archi []db.ListRelazioneProposteThreadRow, p db.ComponenteProposta,
	nodoDi func(db.ComponenteProposta) (string, bool)) camminoTolto {
	c := camminoTolto{perNodo: map[string][]db.ComponenteProposta{}}
	rigaDi := map[rigaNelFile]db.ComponenteProposta{}
	for _, x := range righe {
		r := x.ComponenteProposta
		rigaDi[rigaNelFile{r.AllegatoID, r.Chiave}] = r
		if k, ok := nodoDi(r); ok {
			c.perNodo[k] = append(c.perNodo[k], r)
		}
	}
	padriNelFile := map[rigaNelFile][]string{}
	for _, x := range archi {
		r := x.RelazioneProposta
		padriNelFile[rigaNelFile{r.AllegatoID, r.FiglioChiave}] = append(padriNelFile[rigaNelFile{r.AllegatoID, r.FiglioChiave}], r.PadreChiave)
	}
	primo, _ := nodoDi(p)
	visti := map[string]bool{primo: true}
	c.ordine = []string{primo}
	for i := 0; i < len(c.ordine); i++ {
		for _, r := range c.perNodo[c.ordine[i]] {
			for _, pk := range padriNelFile[rigaNelFile{r.AllegatoID, r.Chiave}] {
				pr, ok := rigaDi[rigaNelFile{r.AllegatoID, pk}]
				if !ok {
					continue
				}
				k, ok := nodoDi(pr)
				if !ok || visti[k] {
					continue
				}
				visti[k] = true
				c.ordine = append(c.ordine, k)
				c.padri = append(c.padri, nomeNodo(pr))
			}
		}
	}
	return c
}

// riapriToltoNellAlbero riapre il nodo della riga p, che una conferma dell'albero ha tolto (il suo segno s), con quello
// che la stessa conferma aveva chiuso e che serve perche' il nodo torni nell'albero proposto (studio § 2.5, «RiapriNodo
// li riporta»; fase 4.4a.1br, dalla verifica della 4.4a.1b: prima si riapriva la riga sola e i suoi archi, e la frase
// diceva «torna» anche quando non tornava):
//   - le righe del nodo in ogni file (stessa firma, stesso nodo nel segno): la conferma le aveva chiuse tutte, e una
//     sola non basta (la riga della radice nello STEP proprio di un sottoassieme tiene insieme quello STEP);
//   - risalendo, i padri nei file che la stessa conferma aveva tolto, con tutte le loro righe: senza, il nodo resterebbe
//     sotto un padre scartato da una persona, e nessun cammino da un prodotto lo raggiungerebbe (le strutture saltano
//     le righe scartate da una persona). La frase li nomina;
//   - gli archi degli stessi file che toccano quelle righe, chiusi dalla stessa conferma (RiapriArchiToltiNellAlbero).
//
// Solo quello che quella conferma aveva tolto: un padre scartato a mano, o da un'altra conferma, e' un'altra decisione e
// resta com'e'; i figli tolti con il nodo restano tolti (si vedono, scartati, sotto di lui). Tutto e' una correzione
// dell'evidenza: la working non cambia, e i pezzi nascono solo con la conferma dopo.
func riapriToltoNellAlbero(ctx context.Context, q *db.Queries, thread uuid.UUID, p db.ComponenteProposta, s SegnoAlbero,
	utente uuid.UUID) (riaperturaAlbero, error) {
	var ra riaperturaAlbero
	righe, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return ra, err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return ra, err
	}
	// p e' tolta da questa conferma: chi chiama l'ha letta con toltoNellAlbero
	c := camminoDaRiaprire(righe, archi, p, func(r db.ComponenteProposta) (string, bool) { return nodoTolto(r, s.Firma) })
	ra.padri = c.padri
	for i, k := range c.ordine {
		for _, r := range c.perNodo[k] {
			n, err := q.RiapriComponenteProposta(ctx, db.RiapriComponentePropostaParams{PropostaID: r.PropostaID, Utente: utente})
			if err != nil {
				return ra, err
			}
			if i == 0 {
				ra.righe += int(n)
			}
			a, err := q.RiapriArchiToltiNellAlbero(ctx, db.RiapriArchiToltiNellAlberoParams{ThreadID: thread, AllegatoID: r.AllegatoID,
				Chiave: r.Chiave, Nota: NotaToltoNellAlbero, Firma: s.Firma, Utente: utente})
			if err != nil {
				return ra, err
			}
			ra.archi += int(a)
		}
	}
	if ra.righe == 0 {
		// la riga del gesto e' bloccata (BloccaComponenteProposta) e scartata: qui non si arriva, se non per un errore
		return ra, Rifiuto(nomeNodo(p) + ": la riga non è più quella che la conferma dell'albero aveva tolto: ricarica la pagina")
	}
	return ra, nil
}

// giaRiapertoNellAlbero ridice, senza scrivere niente, che cosa ha riaperto il gesto che ha riaperto la riga p (x, la
// sua riapertura): le righe di quel gesto sono quelle con la stessa riapertura in fondo alla storia, e il cammino e' lo
// stesso di riapriToltoNellAlbero, quindi i conti e la frase sono quelli che il gesto aveva detto (fase 4.4a.1br, dalla
// verifica). Serve a chi riapre un pezzo riga per riga (la pagina della Distinta, «Riapri il pezzo»): la prima
// riapertura porta con se' le altre righe dello stesso pezzo tolte dalla stessa conferma, e la chiamata dopo, su una di
// quelle, trova la riga gia' aperta; con il rifiuto «non è scartato» la pagina mostrava un errore dopo una riapertura
// riuscita, e non si rifaceva.
func giaRiapertoNellAlbero(ctx context.Context, q *db.Queries, thread uuid.UUID, p db.ComponenteProposta, x riapertura) (riaperturaAlbero, error) {
	var ra riaperturaAlbero
	righe, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return ra, err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return ra, err
	}
	c := camminoDaRiaprire(righe, archi, p, func(r db.ComponenteProposta) (string, bool) {
		y, ok := riapertoNellAlbero(r.Stato, r.Evidenza, eventoNodoRiaperto)
		if !ok || !y.stessoGesto(x) {
			return "", false
		}
		return nodoDelSegno(y.segno, r), true
	})
	ra.righe, ra.padri = len(c.perNodo[c.ordine[0]]), c.padri
	// gli archi: quelli dello stesso gesto che toccano le righe del cammino (il gesto li aveva riaperti per quelle)
	toccate := map[rigaNelFile]bool{}
	for _, k := range c.ordine {
		for _, r := range c.perNodo[k] {
			toccate[rigaNelFile{r.AllegatoID, r.Chiave}] = true
		}
	}
	for _, a := range archi {
		r := a.RelazioneProposta
		y, ok := riapertoNellAlbero(r.Stato, r.Evidenza, eventoArcoRiaperto)
		if ok && y.stessoGesto(x) && (toccate[rigaNelFile{r.AllegatoID, r.PadreChiave}] || toccate[rigaNelFile{r.AllegatoID, r.FiglioChiave}]) {
			ra.archi++
		}
	}
	return ra, nil
}

// frase dice all'operatore che cosa «Riapri il nodo» ha riaperto di una conferma dell'albero.
func (ra riaperturaAlbero) frase(nome string) string {
	var con []string
	if ra.righe > 1 {
		con = append(con, quanti(ra.righe-1, "altra riga dello stesso pezzo", "altre righe dello stesso pezzo"))
	}
	switch len(ra.padri) {
	case 0:
	case 1:
		con = append(con, "il padre "+ra.padri[0])
	default:
		con = append(con, "i padri "+strings.Join(ra.padri, ", "))
	}
	if ra.archi > 0 {
		con = append(con, quanti(ra.archi, "legame dello STEP", "legami dello STEP"))
	}
	frase := nome + " riaperto: torna fra le proposte"
	switch len(con) {
	case 0:
	case 1:
		frase += ", con " + con[0] + " che la conferma dell'albero aveva tolto"
	default:
		frase += ", con " + strings.Join(con[:len(con)-1], ", ") + " e " + con[len(con)-1] + " che la conferma dell'albero aveva tolto"
	}
	return frase + "."
}

// tipoConfermatoNellAlbero e' il tipo con cui nasce un pezzo dell'albero confermato: quello del riepilogo (proposto
// dall'albero, che per un pezzo che nasce non e' mai commerciale, o scelto con la tendina da una persona), o il
// particolare commerciale quando una persona ha confermato la proposta commerciale (il ✓, o la tendina). E' il solo
// posto, fuori da un confronto e dalle tendine dei tipi, dove il codice nomina il tipo commerciale: la guardia
// TestNessunAutomatismoScriveIlTipoCommerciale lo ammette per nome, e ammette una chiamata sola, in ConfermaAlbero con
// la risposta della bozza, perche' qui il tipo lo sceglie una persona (domanda 30, seconda risposta: «la
// responsabilita' e' di chi conferma») e la conferma registra chi (confermato_da, e il segno sulle righe). Senza la
// conferma di una persona non da' mai il commerciale.
func tipoConfermatoNellAlbero(proposto db.TipoComponente, confermatoDaUnaPersona bool) db.TipoComponente {
	if confermatoDaUnaPersona {
		return db.TipoComponenteCommerciale
	}
	return proposto
}

// CorpoConferma e' il corpo della POST della conferma: la firma del riepilogo che la persona ha visto e la bozza su cui
// e' stato calcolato.
type CorpoConferma struct {
	Firma string          `json:"firma"`
	Bozza json.RawMessage `json:"bozza"`
}

// LeggiConferma legge il corpo della conferma: la firma e la bozza (LeggiBozza, con il suo formato).
func LeggiConferma(raw []byte) (BozzaAlbero, string, error) {
	var c CorpoConferma
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil || dec.More() {
		return BozzaAlbero{}, "", Rifiuto("la conferma dell'albero non si legge: ricarica la pagina")
	}
	if strings.TrimSpace(c.Firma) == "" {
		return BozzaAlbero{}, "", Rifiuto("la conferma arriva senza la firma del riepilogo: rileggi il riepilogo e conferma di nuovo")
	}
	b, err := LeggiBozza(c.Bozza)
	return b, strings.TrimSpace(c.Firma), err
}

// esitoConferma conta quello che la conferma ha fatto, per la frase all'operatore.
type esitoConferma struct {
	nuovi, ritrovati, ripristinati, commerciali, tipi, revisioni, legami, quantita, tolti, righe, chiuse int
	archiviati, eliminati                                                                                []string
	parti                                                                                                []string
}

// ConfermaAlbero conferma l'albero proposto con la bozza della persona: vedi l'intestazione del file. firma e' la
// firma del riepilogo che la persona ha visto. Una sola transazione, quella di chi chiama: un rifiuto a meta' (un
// ciclo con un arco fuori dall'albero, un componente che la FK non lascia eliminare) annulla tutto.
func ConfermaAlbero(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, an coda.Analizzatore, b BozzaAlbero, firma string) (string, error) {
	if err := prepara(ctx, q, thread, "si conferma l'albero"); err != nil {
		return "", err
	}
	_, r, pc, err := leggiRiepilogo(ctx, q, thread, an, &b)
	if err != nil {
		return "", err
	}
	if firma == "" || firma != r.Firma {
		return "", Rifiuto("quello che la conferma farebbe è cambiato da quando hai letto il riepilogo (una decisione nel frattempo, " +
			"un file nuovo): rileggi il riepilogo e conferma di nuovo")
	}
	if !r.Confermabile {
		return "", Rifiuto("l'albero non si conferma ancora: " + strings.Join(r.Blocchi, "; "))
	}
	segno := SegnoAlbero{Da: utente, Il: time.Now().UTC().Format(time.RFC3339), Firma: r.Firma}
	var es esitoConferma
	compDi := map[*gruppo]uuid.UUID{}
	idNodo := func(chiave string) (uuid.UUID, error) {
		if n, ok := pc.a.Nodo(chiave); ok && n.Componente.Valid {
			return n.Componente.UUID, nil
		}
		return uuid.Nil, Rifiuto(chiave + ": il componente del legame non si trova: rileggi il riepilogo")
	}

	// 1. i codici scritti nella bozza, sulle righe dei nodi rinominati: come «Correggi il codice del nodo»
	// (CodiceDelNodo, origine operatore), perche' una rilettura non li tocchi piu'
	for _, g := range pc.gruppi {
		for _, m := range g.membri {
			if !m.rinominato || m.albero == nil {
				continue
			}
			for _, rr := range m.albero.Righe {
				if rr.Stato != db.StatoPropostaAperta {
					continue
				}
				rev := m.revScritta
				if rev == "" {
					rev = rr.Rev
				}
				if _, err := q.SetCodiceComponenteProposta(ctx, db.SetCodiceComponentePropostaParams{PropostaID: rr.Proposta,
					Codice: pgtype.Text{String: m.codice, Valid: true}, Rev: testo(rev)}); err != nil {
					return "", err
				}
			}
		}
	}

	// 2. i legami della working che vanno via (la cascata), e le rimozioni proposte dallo STEP: accettate se il legame
	// va via, chiuse se resta
	toltiOra := map[*ArcoAlbero]bool{}
	for _, l := range pc.tolti {
		p, err := idNodo(l.Padre)
		if err != nil {
			return "", err
		}
		f, err := idNodo(l.Figlio)
		if err != nil {
			return "", err
		}
		if _, err := scollega(ctx, q, thread, p, f); err != nil {
			return "", err
		}
		toltiOra[l] = true
		es.tolti++
	}
	for _, l := range pc.rimozioni {
		p, err := idNodo(l.Padre)
		if err != nil {
			return "", err
		}
		f, err := idNodo(l.Figlio)
		if err != nil {
			return "", err
		}
		if toltiOra[l] {
			if _, err := q.DecidiRimozione(ctx, db.DecidiRimozioneParams{ThreadID: thread, StepDocumentoID: l.Rimozione.Step, PadreID: p,
				FiglioID: f, Stato: db.StatoPropostaConfermata, DecisoDa: uid(utente)}); err != nil {
				return "", err
			}
			continue
		}
		if _, err := q.TieniArcoDellaRimozione(ctx, db.TieniArcoDellaRimozioneParams{ThreadID: thread, StepDocumentoID: l.Rimozione.Step,
			PadreID: p, FiglioID: f, DecisoDa: uid(utente)}); err != nil {
			return "", err
		}
	}

	// 3. i pezzi: il prodotto che non ha ancora il suo componente nasce come alla preparazione; un ritrovato archiviato
	// si ripristina; un pezzo nuovo nasce con il codice, il tipo, la revisione e la descrizione del riepilogo
	for _, g := range pc.gruppi {
		switch {
		case g.esiste:
			if g.comp.ArchiviatoIl != nil {
				if _, err := q.RipristinaComponente(ctx, g.comp.ComponenteID); err != nil {
					return "", err
				}
				es.ripristinati++
			}
			compDi[g] = g.comp.ComponenteID
		case g.prodotto:
			codice := g.codice
			if _, err := assicuraProdotti(ctx, q, thread, func(i db.IdentificativoThread) bool {
				return strings.EqualFold(strings.TrimSpace(i.Codice), codice)
			}); err != nil {
				return "", err
			}
			c, err := componenteNato(ctx, q, thread, codice)
			if err != nil {
				return "", err
			}
			compDi[g] = c.ComponenteID
			es.nuovi++
		default:
			rp := g.rappresentante()
			origine, descrizione := db.OrigineComponenteManuale, ""
			for _, m := range g.membri {
				if m.albero != nil {
					origine = db.OrigineComponenteStep
					if descrizione == "" {
						descrizione = m.albero.Descrizione
					}
				}
			}
			// il ✓ di una persona passa qui, e solo qui (la guardia TestNessunAutomatismoScriveIlTipoCommerciale vuole
			// questa chiamata, una sola, con la risposta della bozza come argomento)
			c, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: thread, Codice: g.codice, Rev: testo(rp.rev),
				Descrizione: testo(taglia(descrizione, 200)), Qta: 1, Tipo: tipoConfermatoNellAlbero(g.tipo, g.commercialeConfermato()),
				Origine: origine, ConfermatoDa: utente})
			if err != nil {
				return "", err
			}
			compDi[g] = c.ComponenteID
			es.nuovi++
			if g.commercialeConfermato() {
				es.commerciali++
			}
		}
	}

	// 4. il tipo e la revisione dei componenti che c'erano: il tipo con le regole della scheda (cambiaTipo: un
	// commerciale sospende le autorizzazioni, un particolare che riceve dei figli diventa assieme prima degli archi).
	// L'effetto il riepilogo l'ha gia' detto, calcolato su come il componente e' adesso (effettiDeiTipi): uno spento
	// qui, dopo un riepilogo confermabile, resta un rifiuto che annulla tutto
	for _, g := range pc.gruppi {
		if !g.esiste || g.prodotto {
			continue
		}
		id := compDi[g]
		c, err := q.GetComponente(ctx, id)
		if err != nil {
			return "", err
		}
		if g.tipo != c.Tipo {
			e, err := EffettoCambioTipo(ctx, q, thread, id, g.tipo)
			if err != nil {
				return "", err
			}
			if e.Spento != "" {
				return "", Rifiuto(c.Codice + ": " + e.Spento)
			}
			parti, err := cambiaTipo(ctx, q, thread, e, utente)
			if err != nil {
				return "", err
			}
			es.tipi++
			es.parti = append(es.parti, parti...)
			if c, err = q.GetComponente(ctx, id); err != nil {
				return "", err
			}
		}
		if rev, ok := pc.revisioni[g.identita]; ok {
			if _, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{ComponenteID: id, Tipo: c.Tipo, Rev: testo(rev),
				Descrizione: c.Descrizione, ConfermatoDa: utente}); err != nil {
				return "", err
			}
			es.revisioni++
		}
	}

	// 5. i legami: la quantita' di quelli che c'erano, poi quelli nuovi. Un legame che viene dalle righe di uno STEP
	// nasce con l'origine step (le righe restano legate a lui, con il segno); uno scritto nella bozza, a mano
	scritto := map[*legameFinale]bool{}
	for _, f := range pc.legami {
		p, fi := compDi[f.padre], compDi[f.figlio]
		ora, err := q.GetRelazione(ctx, db.GetRelazioneParams{PadreID: p, FiglioID: fi})
		if err == nil {
			if ora.Qta != f.qta {
				if _, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{PadreID: p, FiglioID: fi, Qta: f.qta, ConfermatoDa: utente}); err != nil {
					return "", err
				}
				scritto[f] = true
				es.quantita++
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		origine := db.OrigineComponenteManuale
		if len(righeDelLegame(f)) > 0 {
			origine = db.OrigineComponenteStep
		}
		if _, _, err := collegaDa(ctx, q, thread, p, fi, utente, f.qta, origine); err != nil {
			return "", err
		}
		scritto[f] = true
		es.legami++
	}

	// 6. le righe degli STEP che l'albero copre, con il segno. I nodi: la prima riga di un pezzo nuovo lo ha fatto
	// nascere (confermata), le altre lo ritrovano (duplicato); le righe dei nodi che escono dall'albero si chiudono. Gli
	// archi: tenuti (confermata se la conferma ha scritto il legame, duplicato se c'era gia'), o chiusi.
	// Di un nodo che resta si decidono solo le righe ancora da decidere (RigaAlbero.daDecidere: le aperte e gli agganci
	// per codice di prima), e di un componente che c'e' solo quelle dei nodi che il riepilogo elenca fra i ritrovati
	// (P13: «solo i ritrovati elencati»; fase 4.4a.1b, dalla verifica); di un nodo che va via, quelle che il riepilogo
	// conta (siChiudeNellAlbero), con il nodo nel segno (per «Riapri il nodo»). Una riga chiusa da un automatismo resta
	// com'e', di un nodo come di un arco (fase 4.4a.1br: prima si chiudevano anche quelle dei nodi che vanno via e degli
	// archi, e la rilettura non le riscriveva piu'); per gli archi, che l'albero porta senza lo stato, la salta la query
	for _, g := range pc.gruppi {
		if g.prodotto {
			continue
		}
		stato := db.StatoPropostaDuplicato
		if !g.esiste {
			stato = db.StatoPropostaConfermata
		}
		decise := 0
		for _, m := range g.membri {
			if m.albero == nil || (g.esiste && m.albero.Ritrovato == nil && !m.rinominato) {
				continue
			}
			s := segno
			s.Commerciale = segnoCommercialeDi(g, m)
			for _, rr := range m.albero.Righe {
				if !rr.daDecidere() {
					continue
				}
				n, err := decidiNodoNellAlbero(ctx, q, rr.Proposta, stato, uuid.NullUUID{UUID: compDi[g], Valid: true}, "", s, utente, false)
				if err != nil {
					return "", err
				}
				if n > 0 {
					decise++
					stato = db.StatoPropostaDuplicato
				}
			}
		}
		es.righe += decise
		if g.esiste && decise > 0 {
			es.ritrovati++
		}
	}
	for _, n := range pc.via {
		s := segno
		s.Nodo = n.Chiave
		for _, rr := range n.Righe {
			if !rr.siChiudeNellAlbero() {
				continue
			}
			k, err := decidiNodoNellAlbero(ctx, q, rr.Proposta, db.StatoPropostaScartata, uuid.NullUUID{}, NotaToltoNellAlbero, s, utente, true)
			if err != nil {
				return "", err
			}
			es.chiuse += int(k)
		}
	}
	for _, f := range pc.legami {
		stato := db.StatoPropostaDuplicato
		if scritto[f] {
			stato = db.StatoPropostaConfermata
		}
		s := segno
		s.Padre, s.Figlio = uid(compDi[f.padre]), uid(compDi[f.figlio])
		for _, l := range f.da {
			if l.albero == nil {
				continue
			}
			for _, fo := range l.albero.Fonti {
				if fo.Scartata {
					continue
				}
				nota := ""
				if fo.Qta != f.qta {
					nota = fmt.Sprintf("quantità decisa nell'albero: ×%d (il file dice ×%d)", f.qta, fo.Qta)
				}
				for _, rr := range fo.Righe {
					n, err := decidiArcoNellAlbero(ctx, q, thread, rr, stato, nota, s, utente, false)
					if err != nil {
						return "", err
					}
					es.righe += int(n)
				}
			}
		}
	}
	for i := range pc.a.Archi {
		l := &pc.a.Archi[i]
		if pc.usati[l] || l.Stato == StatoAlberoScartato {
			continue
		}
		for _, fo := range l.Fonti {
			if fo.Scartata {
				continue
			}
			for _, rr := range fo.Righe {
				n, err := decidiArcoNellAlbero(ctx, q, thread, rr, db.StatoPropostaScartata, NotaToltoNellAlbero, segno, utente, true)
				if err != nil {
					return "", err
				}
				es.chiuse += int(n)
			}
		}
	}

	// 7. i componenti che restano senza padri: archiviati con la loro storia, o eliminati
	for _, fu := range pc.fuori {
		if fu.Esito == FuoriArchivia {
			if _, err := ArchiviaComponente(ctx, q, thread, fu.Componente, utente, NotaToltoNellAlbero); err != nil {
				return "", err
			}
			es.archiviati = append(es.archiviati, fu.Codice)
			continue
		}
		if _, err := RimuoviComponente(ctx, q, thread, fu.Componente); err != nil {
			return "", err
		}
		es.eliminati = append(es.eliminati, fu.Codice)
	}

	// 8. le rimozioni ricalcolate; quelle che lo STEP autorizzato propone su un legame appena confermato si chiudono:
	// la persona l'ha tenuto (come l'editor, ApplicaStrutturaVoluta)
	msg, err := dopoLaDecisione(ctx, q, thread, fraseConferma(es), nil)
	if err != nil {
		return "", err
	}
	for _, f := range pc.legami {
		if err := tieniArcoMessoAMano(ctx, q, thread, Arco{Padre: compDi[f.padre], Figlio: compDi[f.figlio]}, utente); err != nil {
			return "", err
		}
	}
	return msg, nil
}

// righeDelLegame sono le righe degli STEP che fanno un legame finale: quelle delle fonti non scartate.
func righeDelLegame(f *legameFinale) []RigaArco {
	var out []RigaArco
	for _, l := range f.da {
		if l.albero == nil {
			continue
		}
		for _, fo := range l.albero.Fonti {
			if !fo.Scartata {
				out = append(out, fo.Righe...)
			}
		}
	}
	return out
}

// segnoCommercialeDi e' la risposta alla proposta commerciale del nodo, da scrivere nel segno: il ✓ (o la tendina)
// se il pezzo nasce commerciale per quella risposta, il ✗ se una persona l'ha dato. Una proposta decaduta non ha
// risposta.
func segnoCommercialeDi(g *gruppo, m *pezzoBozza) *SegnoCommerciale {
	if m.albero == nil || m.albero.Commerciale == nil {
		return nil
	}
	switch {
	case g.commercialeConfermato():
		return &SegnoCommerciale{Risposta: RispostaSi, Tendina: m.risposta == "", Motivo: m.albero.Commerciale.Motivo}
	case m.risposta == RispostaNo:
		return &SegnoCommerciale{Risposta: RispostaNo, Motivo: m.albero.Commerciale.Motivo}
	}
	return nil
}

// decidiNodoNellAlbero decide la riga di un nodo con il segno. 0: la riga era gia' decisa da una persona, o chiusa da un
// automatismo (fase 4.4a.1br), e non si tocca. anche: una riga che una decisione di prima aveva preso si decide di nuovo
// (un pezzo che la conferma toglie); una scartata da una persona resta com'e'. nota vuota: resta quella della lettura.
func decidiNodoNellAlbero(ctx context.Context, q *db.Queries, proposta uuid.UUID, stato db.StatoProposta, comp uuid.NullUUID, nota string,
	s SegnoAlbero, utente uuid.UUID, anche bool) (int64, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return q.DecidiNodoNellAlbero(ctx, db.DecidiNodoNellAlberoParams{Stato: stato, ComponenteID: comp, DecisoDa: utente,
		Nota: testo(tagliaNota(nota)), Segno: raw, PropostaID: proposta, AncheDecise: anche})
}

// decidiArcoNellAlbero decide la riga di un arco con il segno, come decidiNodoNellAlbero: una chiusa da un automatismo
// (la «tenuta dei conti» di Pianifica, lo stesso componente, un file sostituito) resta com'e'.
func decidiArcoNellAlbero(ctx context.Context, q *db.Queries, thread uuid.UUID, rr RigaArco, stato db.StatoProposta, nota string,
	s SegnoAlbero, utente uuid.UUID, anche bool) (int64, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return q.DecidiArcoNellAlbero(ctx, db.DecidiArcoNellAlberoParams{Stato: stato, Nota: testo(tagliaNota(nota)), DecisoDa: utente,
		Segno: raw, ThreadID: thread, AllegatoID: rr.Allegato, PadreChiave: rr.Padre, FiglioChiave: rr.Figlio, AncheDecise: anche})
}

// fraseConferma dice all'operatore che cosa la conferma ha fatto.
func fraseConferma(es esitoConferma) string {
	var parti []string
	if es.nuovi > 0 {
		s := quanti(es.nuovi, "pezzo nuovo", "pezzi nuovi")
		if es.commerciali > 0 {
			s += " (" + quanti(es.commerciali, "particolare commerciale confermato", "particolari commerciali confermati") + ")"
		}
		parti = append(parti, s)
	}
	if es.ritrovati > 0 {
		parti = append(parti, quanti(es.ritrovati, "pezzo che c'era già", "pezzi che c'erano già"))
	}
	if es.ripristinati > 0 {
		parti = append(parti, quanti(es.ripristinati, "pezzo archiviato ripristinato", "pezzi archiviati ripristinati"))
	}
	if es.tipi > 0 {
		parti = append(parti, quanti(es.tipi, "tipo cambiato", "tipi cambiati"))
	}
	if es.revisioni > 0 {
		parti = append(parti, quanti(es.revisioni, "revisione nuova", "revisioni nuove"))
	}
	if es.legami > 0 {
		parti = append(parti, quanti(es.legami, "legame nuovo", "legami nuovi"))
	}
	if es.quantita > 0 {
		parti = append(parti, quanti(es.quantita, "quantità cambiata", "quantità cambiate"))
	}
	if es.tolti > 0 {
		parti = append(parti, quanti(es.tolti, "legame tolto", "legami tolti"))
	}
	if es.righe > 0 {
		parti = append(parti, quanti(es.righe, "riga degli STEP decisa", "righe degli STEP decise"))
	}
	if es.chiuse > 0 {
		parti = append(parti, quanti(es.chiuse, "riga degli STEP chiusa", "righe degli STEP chiuse"))
	}
	frase := "Albero confermato: niente da cambiare, le righe degli STEP erano già decise."
	if len(parti) > 0 {
		frase = "Albero confermato: " + strings.Join(parti, ", ") + "."
	}
	if len(es.archiviati) > 0 {
		frase += " Archiviati, senza padri: " + strings.Join(es.archiviati, ", ") + "."
	}
	if len(es.eliminati) > 0 {
		frase += " Eliminati, senza padri e senza storia: " + strings.Join(es.eliminati, ", ") + "."
	}
	if len(es.parti) > 0 {
		frase += " " + strings.Join(es.parti, " ")
	}
	return frase
}
