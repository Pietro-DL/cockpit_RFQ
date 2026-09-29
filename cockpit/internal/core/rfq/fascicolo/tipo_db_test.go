//go:build integrazione

// L4 — Smistamento, fase T (decisioni del 27/09 ter, «Tipo componente / commerciale», Domanda 5 = B;
// precisazioni dell'utente del 27/09 sera): il tipo del componente lo decide una persona, con l'anteprima e la
// firma di quello che ha visto. Una prova per regola (finito solo dove si puo', da finito ad altro, sciolto con
// dei figli, commerciale che sospende l'autorita' invece di rifiutarsi, la BOM congelata, la firma), e le prove
// F5b del cambio sottoassieme ↔ commerciale chieste dall'utente: passando a commerciale l'autorita' si sospende;
// tornando a sottoassieme resta sospesa finche' una persona non sceglie «Riattiva»; lo stesso per una delega
// nello STEP del prodotto; un commerciale non si riattiva; un commerciale senza autorizzazioni che torna
// sottoassieme diventa autorizzabile, e niente si autorizza da solo. Contro PostgreSQL vero.

package fascicolo_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// effettoTipo e' l'anteprima del cambio di tipo come la chiede la scheda: una GET, fuori da una transazione.
func (b *banco) effettoTipo(comp uuid.UUID, tipo db.TipoComponente) fascicolo.EffettoTipo {
	b.t.Helper()
	e, err := fascicolo.EffettoCambioTipo(b.ctx, db.New(b.p), b.thread, comp, tipo)
	if err != nil {
		b.t.Fatalf("anteprima del tipo: %v", err)
	}
	return e
}

// cambiaTipo e' il gesto come lo fa la scheda: l'anteprima, poi la scrittura con la firma vista.
func (b *banco) cambiaTipo(comp uuid.UUID, tipo db.TipoComponente) (string, error) {
	b.t.Helper()
	e := b.effettoTipo(comp, tipo)
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CambiaTipoComponenteVisto(b.ctx, q, b.thread, comp, tipo, b.utente, e.Firma)
	})
}

func (b *banco) cambiaTipoOk(comp uuid.UUID, tipo db.TipoComponente) string {
	b.t.Helper()
	msg, err := b.cambiaTipo(comp, tipo)
	if err != nil {
		b.t.Fatalf("cambio di tipo in %s: %v", tipo, err)
	}
	return msg
}

// commercialeDiPrima porta a commerciale, con il gesto della fase T, un componente che nella working ha dei figli,
// come si faceva fino alla PR #7 (Distinta): il passaggio non si rifiutava, e i figli restavano. Oggi il gesto con
// dei figli si rifiuta (il commerciale e' sempre una foglia: domanda 6a, 29/09 sera), e lo si prova per primo;
// poi gli archi sotto il componente si tolgono per il gesto e si rimettono com'erano con una scrittura diretta:
// sono i dati che una RFQ di prima puo' avere, e sui quali la sospensione, il gate e le rimozioni devono reggere.
// Restituisce il messaggio del gesto.
func (b *banco) commercialeDiPrima(comp uuid.UUID) string {
	b.t.Helper()
	righe, err := b.p.Query(b.ctx, `SELECT * FROM componente_relazione WHERE padre_id = $1 ORDER BY figlio_id`, comp)
	if err != nil {
		b.t.Fatal(err)
	}
	archi, err := pgx.CollectRows(righe, pgx.RowToStructByName[db.ComponenteRelazione])
	if err != nil {
		b.t.Fatal(err)
	}
	if len(archi) == 0 {
		b.t.Fatal("commercialeDiPrima: il componente non ha figli, il gesto si fa da se'")
	}
	_, err = b.cambiaTipo(comp, db.TipoComponenteCommerciale)
	deveRifiutare(b.t, err, "è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi")
	b.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1`, comp)
	msg := b.cambiaTipoOk(comp, db.TipoComponenteCommerciale)
	for _, a := range archi {
		b.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, posizione, origine, confermato_da, creato_il)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, a.ThreadID, a.PadreID, a.FiglioID, a.Qta, a.Posizione, a.Origine, a.ConfermatoDa, a.CreatoIl)
	}
	return msg
}

// effettoRiattivazione e riattiva: la riattivazione dalla scheda, anteprima e scrittura con la firma vista.
func (b *banco) effettoRiattivazione(comp uuid.UUID, sha string) fascicolo.EffettoRiattivazione {
	b.t.Helper()
	e, err := fascicolo.EffettoRiattivazioneDi(b.ctx, db.New(b.p), b.thread, comp, sha)
	if err != nil {
		b.t.Fatalf("anteprima della riattivazione: %v", err)
	}
	return e
}

func (b *banco) riattiva(comp uuid.UUID, sha string) (string, error) {
	b.t.Helper()
	e := b.effettoRiattivazione(comp, sha)
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.RiattivaStrutturale(b.ctx, q, b.thread, b.utente, comp, sha, e.Firma)
	})
}

// dichiarazioneDi dice la dichiarazione del componente: «valida», oppure il suo problema.
func (b *banco) dichiarazioneDi(comp uuid.UUID) string {
	b.t.Helper()
	d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	if err != nil {
		b.t.Fatal(err)
	}
	if _, ok := d.Di(comp); ok {
		return "valida"
	}
	var pp []string
	for _, x := range d.DelComponente(comp) {
		pp = append(pp, x.Problema)
	}
	if len(pp) == 0 {
		return "nessuna"
	}
	return strings.Join(pp, " | ")
}

func (b *banco) gateOra() fascicolo.Gate {
	b.t.Helper()
	var g fascicolo.Gate
	ok(b.t, b.tx(func(q *db.Queries) (err error) { g, err = fascicolo.LeggiGate(b.ctx, q, b.thread); return err }))
	return g
}

func (b *banco) codiceComp(codice string) uuid.UUID {
	b.t.Helper()
	return uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = $2`, b.thread, codice)
}

// improntaProposte: le righe di proposta e di rimozione della RFQ, per dire che un gesto non le ha toccate.
func (b *banco) improntaProposte() string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM componente_proposta x WHERE thread_id = $1),
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM relazione_proposta x WHERE thread_id = $1),
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM rimozione_proposta x WHERE thread_id = $1))`, b.thread)
}

// scenaTipo: la RFQ ACME del prodotto 7120001, con lo STEP completo del prodotto autorizzato e 7120010 accettato
// sotto il prodotto; lo STEP proprio di 7120010 (7120010.stp: 7120011 ×2 e 7120099) autorizzato per 7120010;
// nella working 7120010 → 7120098, che il file non ha. 7120011 e' accettato (l'arco no), 7120099 scartato e poi
// riaperto: la rimozione 7120010 → 7120098 e' aperta, e 7120099 e' un figlio diretto da decidere.
type scenaTipo struct {
	prodotto, s, vecchio uuid.UUID
	a, sa                db.Allegato
}

func (b *banco) scenaTipo() scenaTipo {
	b.t.Helper()
	var sc scenaTipo
	var doc uuid.UUID
	sc.prodotto, doc, sc.a = b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.prodotto, Documento: doc})
	if err := b.accettaIn(sc.a, "#2"); err != nil {
		b.t.Fatal(err)
	}
	if _, err := b.accettaArco(sc.a, "#1", "#2"); err != nil {
		b.t.Fatal(err)
	}
	sc.s = b.codiceComp("7120010")
	sc.vecchio = b.componente("7120098", db.TipoComponenteSciolto)
	b.arco(sc.s, sc.vecchio, 1)
	var sdoc uuid.UUID
	sdoc, sc.sa = b.stepDelProdotto(sc.s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120099"}, archi: []string{"#1>#2*2", "#1>#3"}})
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.s, Documento: sdoc})
	if err := b.accettaIn(sc.sa, "#2"); err != nil {
		b.t.Fatal(err)
	}
	for _, f := range []func(q *db.Queries) (string, error){
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.nodoIn(sc.sa, "#3"), b.utente)
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.RiapriNodo(b.ctx, q, b.thread, b.nodoIn(sc.sa, "#3"), b.utente)
		},
	} {
		if _, err := b.gesto(f); err != nil {
			b.t.Fatal(err)
		}
	}
	return sc
}

func (b *banco) rimozioneDi(padre, figlio uuid.UUID) string {
	b.t.Helper()
	return uno[string](b, `SELECT r.stato || ':' || coalesce(r.nota, '') FROM rimozione_proposta r
		WHERE r.thread_id = $1 AND r.padre_id = $2 AND r.figlio_id = $3`, b.thread, padre, figlio)
}

// Le prove F5b del cambio sottoassieme ↔ commerciale (precisazione dell'utente del 27/09 sera), sul file
// autorizzato di 7120010:
//   - con un figlio nella working il passaggio a commerciale e' spento e si rifiuta, e niente cambia (il
//     commerciale e' una foglia); una persona toglie il figlio accettando la rimozione che il file propone;
//   - passa a commerciale: il gesto si fa (non si rifiuta), l'anteprima lo annuncia senza scrivere; la
//     sospensione e' registrata nella marcatura (chi, quando, «è diventato commerciale»); 7120011 e 7120099 non
//     sono piu' autorita' (le accettazioni si rifiutano), il gate non conta niente del file, l'avviso dice
//     «sospesa»; la rimozione decisa da una persona resta decisa;
//   - un commerciale non si riattiva (anteprima spenta, scrittura rifiutata);
//   - torna sottoassieme: ANCORA sospesa, e il cambio non tocca proposte, gate, accettazioni;
//   - «Riattiva» esplicita (anteprima, firma): l'autorita' torna, l'arco si accetta; la rimozione accettata non si
//     riapre, e l'arco tolto non torna.
//
// Riscritta per lo Smistamento (Distinta): prima fissava il passaggio a commerciale di 7120010 con il figlio
// 7120098 nella working: l'anteprima con «I figli già nella BOM restano (7120098)» e una rimozione che si chiude,
// il messaggio con i figli che restano, la rimozione aperta chiusa dalla sospensione e riaperta (E33) dopo
// «Riattiva», l'arco che restava nella working. Con la regola della PR #7, confermata dall'utente il 29/09 sera
// (domanda 6a: «il commerciale è SEMPRE una foglia e mai un ramo»), il passaggio con un figlio si spegne e si
// rifiuta; qui la persona accetta prima la rimozione che il file autorizzato propone (7120010 → 7120098), e il
// resto della prova e' quello di prima sulla sospensione. La chiusura delle rimozioni alla sospensione e la loro
// riapertura (E33), con una working di prima, restano in TestUnaDichiarazioneSospesaNonFermaIlGateENonSiRiattivaDaSola.
// Il nome del tipo nel messaggio e' «particolare commerciale» (NomeTipo, PR #7). Asserzioni (chiamate t.Error,
// t.Fatal, deveRifiutare, ok): prima 44, dopo 51.
func TestIlCambioSottoassiemeCommercialeSospendeLAutorita(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaTipo()
	sha := sc.sa.Sha256.String
	if _, figlio := b.autoritaOra().FiglioDiretto(sc.sa.AllegatoID, "#3"); !figlio {
		t.Fatal("prima: 7120099 e' figlio diretto di 7120010 nel suo file")
	}
	if got := b.rimozioneDi(sc.s, sc.vecchio); got != "aperta:" {
		t.Fatalf("prima: la rimozione 7120010 → 7120098 e' aperta: %q", got)
	}
	prima := b.strutturali()
	soloProdotto := db.GateStrutturaleRow{NFigliDaDecidere: 1, NArchiDaDecidere: 1}
	if prima.NRimozioni != 1 || prima.NFigliDaDecidere != 2 || prima.NArchiDaDecidere != 3 {
		t.Fatalf("prima il gate conta anche il file di 7120010: %+v", prima)
	}

	// con il figlio 7120098 nella working: spento, rifiutato, e niente cambia
	const conFiglio = "7120010 ha 1 figlio: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi"
	foto, proposte := b.improntaAutorizzazione(), b.improntaProposte()
	if e := b.effettoTipo(sc.s, db.TipoComponenteCommerciale); e.Spento != conFiglio || len(e.Sospende) != 0 {
		t.Errorf("anteprima del passaggio a commerciale con un figlio: %q, sospende %v", e.Spento, e.Sospende)
	}
	_, err := b.cambiaTipo(sc.s, db.TipoComponenteCommerciale)
	deveRifiutare(t, err, conFiglio)
	if b.improntaAutorizzazione() != foto || b.improntaProposte() != proposte || b.strutturali() != prima || b.dichiarazioneDi(sc.s) != "valida" {
		t.Error("il rifiuto ha cambiato le autorizzazioni, le proposte o il gate")
	}
	if got := b.bom(); !strings.Contains(got, "7120010>7120098*1") || !strings.Contains(got, "7120010:sottoassieme") {
		t.Errorf("il rifiuto non cambia la BOM: %s", got)
	}
	// il figlio lo toglie una persona: accetta la rimozione che il file autorizzato propone
	docSotto := uno[uuid.UUID](b, `SELECT documento_id FROM documento WHERE thread_id = $1 AND sha256 = $2`, b.thread, sha)
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRimozione(b.ctx, q, b.thread, fascicolo.ChiaveRimozione{Step: docSotto, Padre: sc.s, Figlio: sc.vecchio}, b.utente)
	})
	ok(t, err)
	if msg != "7120010 → 7120098 tolto dalla BOM working." || b.rimozioneDi(sc.s, sc.vecchio) != "confermata:" {
		t.Errorf("la rimozione accettata: %q, %q", msg, b.rimozioneDi(sc.s, sc.vecchio))
	}

	// l'anteprima: si fa, annuncia la sospensione; nessuna rimozione da chiudere, nessun figlio; non scrive
	foto = b.improntaAutorizzazione()
	e := b.effettoTipo(sc.s, db.TipoComponenteCommerciale)
	if b.improntaAutorizzazione() != foto {
		t.Fatal("l'anteprima del cambio di tipo ha scritto")
	}
	if e.Spento != "" || strings.Join(e.Sospende, "|") != "l'autorizzazione di 7120010.stp per 7120010" || e.Rimozioni != 0 ||
		len(e.FigliRestano) != 0 || len(e.ConLei) != 0 {
		t.Fatalf("anteprima del passaggio a commerciale: %+v", e)
	}
	if !strings.Contains(strings.Join(e.Avvisi, "|"), "i suoi discendenti negli STEP restano guida") || !strings.Contains(e.Bottone(), "1 autorizzazione si sospende") {
		t.Errorf("anteprima: %v · %q", e.Avvisi, e.Bottone())
	}

	msg = b.cambiaTipoOk(sc.s, db.TipoComponenteCommerciale)
	for _, c := range []string{"7120010: tipo assieme → particolare commerciale.", "Sospesa l'autorizzazione di 7120010.stp per 7120010"} {
		if !strings.Contains(msg, c) {
			t.Errorf("messaggio: manca %q in %q", c, msg)
		}
	}
	if strings.Contains(msg, "I figli già nella BOM restano") {
		t.Errorf("un commerciale non ha figli che restano: %q", msg)
	}
	if got := uno[string](b, `SELECT tipo::text || ':' || (confermato_da = $2)::text FROM componente WHERE componente_id = $1`, sc.s, b.utente); got != "commerciale:true" {
		t.Errorf("il tipo, confermato da chi lo cambia: %s", got)
	}
	if got := uno[string](b, `SELECT concat_ws('|', evidenza -> 'strutturale' -> 'sospesa' ->> 'motivo', evidenza -> 'strutturale' -> 'sospesa' ->> 'tipo',
		evidenza -> 'strutturale' -> 'sospesa' ->> 'da', (evidenza -> 'strutturale' -> 'sospesa' ->> 'il') IS NOT NULL)
		FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#1'`, b.thread, sc.sa.AllegatoID); got != "è diventato commerciale|commerciale|"+b.utente.String()+"|t" {
		t.Errorf("la sospensione registrata nella marcatura: %s", got)
	}
	if got := b.storiaNodo(sc.sa, "#1"); !strings.HasSuffix(got, "autorizzazione_sospesa") {
		t.Errorf("la storia della sorgente: %q", got)
	}
	if got := b.statoNodo(sc.sa, "#1"); got != "duplicato 7120010 persona marcato:radice" {
		t.Errorf("la sorgente resta decisa e marcata (sospesa non e' revocata): %s", got)
	}
	if got := b.dichiarazioneDi(sc.s); !strings.HasPrefix(got, "sospesa: 7120010: è diventato commerciale") {
		t.Errorf("la dichiarazione: %s", got)
	}
	aut := b.autoritaOra()
	for _, k := range []string{"#2", "#3"} {
		if _, figlio := aut.FiglioDiretto(sc.sa.AllegatoID, k); figlio {
			t.Errorf("%s non e' piu' autorita'", k)
		}
	}
	deveRifiutare(t, b.accettaIn(sc.sa, "#3"), "7120099 è guida")
	_, err = b.accettaArco(sc.sa, "#1", "#2")
	deveRifiutare(t, err, "7120010 → 7120011 è guida")
	if s := b.strutturali(); s != soloProdotto {
		t.Errorf("il gate non conta niente del file sospeso: %+v", s)
	}
	if got := b.rimozioneDi(sc.s, sc.vecchio); got != "confermata:" {
		t.Errorf("la rimozione decisa da una persona resta decisa: %q", got)
	}
	g := b.gateOra()
	if av := strings.Join(g.Avvisi, " | "); !strings.Contains(av, "autorizzazione sospesa: 7120010 · STEP autorizzato 7120010.stp: sospesa: 7120010: è diventato commerciale") {
		t.Errorf("l'avviso dice sospesa: %s", av)
	}
	if strings.Contains(g.Motivo(), "7120010.stp") {
		t.Errorf("la dichiarazione sospesa non ferma il gate: %s", g.Motivo())
	}
	if got := b.bom(); strings.Contains(got, "7120010>") || !strings.Contains(got, "7120010:commerciale") {
		t.Errorf("il commerciale resta nella BOM, senza figli: %s", got)
	}

	// un commerciale non si riattiva
	er := b.effettoRiattivazione(sc.s, sha)
	const nonCommerciale = "7120010 è un commerciale: il suo STEP resta guida; per riattivare l'autorizzazione cambia prima il tipo"
	if er.Spento != nonCommerciale {
		t.Errorf("riattivare un commerciale: %q", er.Spento)
	}
	_, err = b.riattiva(sc.s, sha)
	deveRifiutare(t, err, nonCommerciale)

	// torna sottoassieme: ancora sospesa, e niente cambia su proposte, gate, accettazioni
	e = b.effettoTipo(sc.s, db.TipoComponenteSottoassieme)
	if e.Spento != "" || strings.Join(e.RestaSospesa, "|") != "l'autorizzazione di 7120010.stp per 7120010" || e.Autorizzabile || len(e.Sospende) != 0 {
		t.Fatalf("anteprima del ritorno a sottoassieme: %+v", e)
	}
	proposte = b.improntaProposte()
	msg = b.cambiaTipoOk(sc.s, db.TipoComponenteSottoassieme)
	if !strings.Contains(msg, "Resta sospesa l'autorizzazione di 7120010.stp per 7120010") {
		t.Errorf("messaggio: %q", msg)
	}
	if b.improntaProposte() != proposte {
		t.Error("il ritorno a sottoassieme ha toccato le proposte o le rimozioni")
	}
	if got := b.dichiarazioneDi(sc.s); !strings.HasPrefix(got, "sospesa: 7120010: è diventato commerciale") {
		t.Errorf("tornato sottoassieme, la dichiarazione resta sospesa: %s", got)
	}
	if s := b.strutturali(); s != soloProdotto {
		t.Errorf("tornato sottoassieme, il gate non conta ancora il file: %+v", s)
	}
	deveRifiutare(t, b.accettaIn(sc.sa, "#3"), "7120099 è guida")
	_, err = b.accettaArco(sc.sa, "#1", "#2")
	deveRifiutare(t, err, "7120010 → 7120011 è guida")
	if got := b.rimozioneDi(sc.s, sc.vecchio); got != "confermata:" {
		t.Errorf("la rimozione resta decisa: %q", got)
	}

	// «Riattiva» esplicita: l'anteprima (non scrive), la firma, poi l'autorita' torna
	foto = b.improntaAutorizzazione()
	er = b.effettoRiattivazione(sc.s, sha)
	if b.improntaAutorizzazione() != foto {
		t.Fatal("l'anteprima della riattivazione ha scritto")
	}
	if er.Spento != "" || er.FigliDiretti != 2 || er.Delega || er.Sospensione == nil || er.Sospensione.Motivo != "è diventato commerciale" ||
		er.Bottone() != "Riattiva l'autorizzazione dello STEP 7120010.stp per 7120010" {
		t.Fatalf("anteprima della riattivazione: %+v", er)
	}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.RiattivaStrutturale(b.ctx, q, b.thread, b.utente, sc.s, sha, "deadbeef")
	})
	deveRifiutare(t, err, "riapri l'anteprima")
	msg, err = b.riattiva(sc.s, sha)
	ok(t, err)
	if msg != "Riattivata l'autorizzazione di 7120010.stp per 7120010: 2 figli diretti tornano nell'autorità." {
		t.Errorf("messaggio della riattivazione: %q", msg)
	}
	if got := b.dichiarazioneDi(sc.s); got != "valida" {
		t.Errorf("riattivata: %s", got)
	}
	if got := b.storiaNodo(sc.sa, "#1"); !strings.HasSuffix(got, "autorizzazione_sospesa autorizzazione_riattivata") {
		t.Errorf("la storia della sorgente: %q", got)
	}
	if got := uno[bool](b, `SELECT (evidenza -> 'strutturale') ? 'sospesa' FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#1'`,
		b.thread, sc.sa.AllegatoID); got {
		t.Error("riattivata, la marcatura non porta piu' la sospensione")
	}
	if _, figlio := b.autoritaOra().FiglioDiretto(sc.sa.AllegatoID, "#3"); !figlio {
		t.Error("riattivata, 7120099 e' di nuovo figlio diretto di 7120010")
	}
	if s := b.strutturali(); s.NFigliDaDecidere != prima.NFigliDaDecidere || s.NArchiDaDecidere != prima.NArchiDaDecidere {
		t.Errorf("riattivata, il gate conta di nuovo il file: %+v (prima %+v)", s, prima)
	}
	if _, err := b.accettaArco(sc.sa, "#1", "#2"); err != nil {
		t.Errorf("riattivata, l'arco 7120010 → 7120011 si accetta: %v", err)
	}
	// scartato di nuovo 7120099, la rimozione accettata da una persona non si riapre, e l'arco tolto non torna
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.nodoIn(sc.sa, "#3"), b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozioneDi(sc.s, sc.vecchio); got != "confermata:" {
		t.Errorf("riattivata, la rimozione decisa da una persona resta decisa: %q", got)
	}
	if got := b.bom(); !strings.Contains(got, "7120010>7120011*2") || strings.Contains(got, "7120010>7120098") || !strings.Contains(got, "7120010:sottoassieme") {
		t.Errorf("la BOM: %s", got)
	}
}

// La stessa prova per una delega nello STEP del prodotto: 7120010 e' autorizzato per delega nel file del
// prodotto, e 7120011, sotto, ha la sua delega sopra quella di 7120010. 7120010 commerciale: la sua delega si
// sospende (registrata) e quella di 7120011 si sospende con lei, senza catena; 7120011 e 7120013 non sono piu'
// autorita'. Tornato sottoassieme resta tutto sospeso; la riattivazione di 7120010 annuncia che torna anche la
// delega di 7120011, e l'autorita' dei due livelli torna.
func TestIlCambioSospendeLaDelegaNelloStepDelProdotto(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	s := b.codiceComp("7120010")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Deleghe: []string{"#2"}})
	if err := b.accettaIn(a, "#4"); err != nil {
		t.Fatal(err)
	}
	n := b.codiceComp("7120011")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: n, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#4"}})
	aut := b.autoritaOra()
	if _, f4 := aut.FiglioDiretto(a.AllegatoID, "#4"); !f4 {
		t.Fatal("prima: 7120011 e' figlio diretto della delega di 7120010")
	}
	if _, f5 := aut.FiglioDiretto(a.AllegatoID, "#5"); !f5 {
		t.Fatal("prima: 7120013 e' figlio diretto della delega di 7120011")
	}
	prima := b.strutturali()

	e := b.effettoTipo(s, db.TipoComponenteCommerciale)
	if e.Spento != "" || strings.Join(e.Sospende, "|") != "la delega di 7120001.stp per 7120010" || strings.Join(e.ConLei, ",") != "7120011" {
		t.Fatalf("anteprima: %+v", e)
	}
	msg := b.cambiaTipoOk(s, db.TipoComponenteCommerciale)
	if !strings.Contains(msg, "Sospesa la delega di 7120001.stp per 7120010") || !strings.Contains(msg, "Si sospendono con lei le deleghe di 7120011") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.dichiarazioneDi(s); !strings.HasPrefix(got, "sospesa: 7120010: è diventato commerciale") {
		t.Errorf("la delega di 7120010: %s", got)
	}
	if got := b.dichiarazioneDi(n); !strings.Contains(got, "catena") {
		t.Errorf("la delega di 7120011 resta senza catena: %s", got)
	}
	if got := b.statoNodo(a, "#4"); got != "confermata 7120011 persona marcato:delega" {
		t.Errorf("la delega di 7120011 resta scritta (sospesa, non revocata): %s", got)
	}
	aut = b.autoritaOra()
	for _, k := range []string{"#4", "#5"} {
		if _, figlio := aut.FiglioDiretto(a.AllegatoID, k); figlio {
			t.Errorf("%s non e' piu' autorita'", k)
		}
	}
	if c, figlio := aut.FiglioDiretto(a.AllegatoID, "#2"); !figlio || c != prodotto {
		t.Error("7120010 resta figlio diretto del prodotto: l'autorizzazione del prodotto non cambia")
	}
	_, err := b.accettaArco(a, "#2", "#4")
	deveRifiutare(t, err, "7120010 → 7120011 è guida")
	deveRifiutare(t, b.accettaIn(a, "#5"), "7120013 è guida")
	sosp := b.strutturali()
	if sosp.NFigliDaDecidere >= prima.NFigliDaDecidere || sosp.NArchiDaDecidere >= prima.NArchiDaDecidere {
		t.Errorf("il gate non conta piu' i livelli delegati: %+v (prima %+v)", sosp, prima)
	}
	if av := strings.Join(b.gateOra().Avvisi, " | "); !strings.Contains(av, "7120010 · STEP autorizzato 7120001.stp (delega): sospesa: 7120010: è diventato commerciale") ||
		!strings.Contains(av, "7120011 · STEP autorizzato 7120001.stp (delega): sospesa") {
		t.Errorf("gli avvisi: %s", av)
	}

	// tornato sottoassieme: ancora tutto sospeso
	b.cambiaTipoOk(s, db.TipoComponenteSottoassieme)
	if got := b.dichiarazioneDi(s); !strings.HasPrefix(got, "sospesa: 7120010: è diventato commerciale") {
		t.Errorf("tornato sottoassieme: %s", got)
	}
	if _, figlio := b.autoritaOra().FiglioDiretto(a.AllegatoID, "#4"); figlio {
		t.Error("tornato sottoassieme, 7120011 resta guida")
	}
	if got := b.strutturali(); got != sosp {
		t.Errorf("tornato sottoassieme, il gate non cambia: %+v", got)
	}

	// la riattivazione di 7120010: torna anche la delega di 7120011
	er := b.effettoRiattivazione(s, a.Sha256.String)
	if er.Spento != "" || !er.Delega || strings.Join(er.ConLei, ",") != "7120011" || er.FigliDiretti != 1 ||
		er.Bottone() != "Riattiva la delega dello STEP 7120001.stp per 7120010" {
		t.Fatalf("anteprima della riattivazione della delega: %+v", er)
	}
	msg, err = b.riattiva(s, a.Sha256.String)
	ok(t, err)
	if msg != "Riattivata la delega di 7120001.stp per 7120010: 1 figlio diretto torna nell'autorità. Tornano valide con lei le deleghe di 7120011." {
		t.Errorf("messaggio: %q", msg)
	}
	for _, c := range []uuid.UUID{s, n} {
		if got := b.dichiarazioneDi(c); got != "valida" {
			t.Errorf("riattivata: %s", got)
		}
	}
	aut = b.autoritaOra()
	if _, f5 := aut.FiglioDiretto(a.AllegatoID, "#5"); !f5 {
		t.Error("riattivata, 7120013 e' di nuovo figlio diretto della delega di 7120011")
	}
	if _, err := b.accettaArco(a, "#2", "#4"); err != nil {
		t.Errorf("riattivata, l'arco 7120010 → 7120011 della delega si accetta: %v", err)
	}
	if got := b.strutturali(); got.NFigliDaDecidere != prima.NFigliDaDecidere {
		t.Errorf("riattivata, il gate conta di nuovo i livelli delegati: %+v (prima %+v)", got, prima)
	}
}

// Revocare il padre revoca anche la delega sospesa (F5b, «revocare il padre revoca le sue deleghe»): 7120011,
// delegato sotto la delega di 7120010, e' diventato commerciale (la sua delega e' sospesa, registrata). La revoca
// della delega di 7120010 porta via anche quella di 7120011, con la storia: la catena, senza sospensioni, non
// c'e' piu'. Finche' il padre c'era, la delega sospesa restava sospesa.
func TestRevocareIlPadreRevocaAncheLaDelegaSospesa(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	s := b.codiceComp("7120010")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Deleghe: []string{"#2"}})
	if err := b.accettaIn(a, "#4"); err != nil {
		t.Fatal(err)
	}
	n := b.codiceComp("7120011")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: n, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#4"}})
	b.cambiaTipoOk(n, db.TipoComponenteCommerciale)
	if got := b.dichiarazioneDi(n); !strings.HasPrefix(got, "sospesa: 7120011: è diventato commerciale") {
		t.Fatalf("la delega di 7120011: %s", got)
	}
	if got := b.dichiarazioneDi(s); got != "valida" {
		t.Errorf("la delega di 7120010 non cambia: %s", got)
	}
	msg, err := b.revoca(s, "")
	ok(t, err)
	if !strings.Contains(msg, "Revocate con lui le deleghe di 7120011") {
		t.Errorf("messaggio della revoca: %q", msg)
	}
	if got := b.dichiarazioneDi(n); got != "nessuna" {
		t.Errorf("la delega sospesa di 7120011 se ne va con il padre: %s", got)
	}
	if got := b.storiaNodo(a, "#4"); !strings.HasSuffix(got, "autorizzazione_sospesa revocata_con_il_padre") {
		t.Errorf("la storia: %q", got)
	}
	if got := b.dichiarazioneDi(prodotto); got != "valida" {
		t.Errorf("l'autorizzazione del prodotto resta: %s", got)
	}
}

// Un componente commerciale senza autorizzazioni che passa a sottoassieme diventa autorizzabile (F5b): la sua
// anteprima dell'autorizzazione non e' piu' spenta. Ma niente si autorizza da solo: nessuna marcatura, nessuna
// dichiarazione, le proposte del suo STEP restano guida.
func TestUnCommercialeCheTornaSottoassiemeDiventaAutorizzabileMaNienteSiAutorizzaDaSolo(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	comm := b.componente("7120099", db.TipoComponenteCommerciale)
	cdoc, ca := b.stepDelProdotto(comm, "7120099", fattiSTEP{nodi: []string{"#1=7120099", "#2=7120098"}, archi: []string{"#1>#2"}})
	b.applica(ca, fattiSTEP{nodi: []string{"#1=7120099", "#2=7120098"}, archi: []string{"#1>#2"}}.json())
	r := fascicolo.RichiestaAutorizzazione{Componente: comm, Documento: cdoc}
	if e := b.effetto(r); !strings.Contains(e.Spento, "è un commerciale") {
		t.Fatalf("da commerciale l'autorizzazione e' spenta: %q", e.Spento)
	}
	e := b.effettoTipo(comm, db.TipoComponenteSottoassieme)
	if e.Spento != "" || !e.Autorizzabile || len(e.RestaSospesa) != 0 {
		t.Fatalf("anteprima: %+v", e)
	}
	msg := b.cambiaTipoOk(comm, db.TipoComponenteSottoassieme)
	if !strings.Contains(msg, "Il suo STEP si può autorizzare dalla scheda: niente si autorizza da solo") {
		t.Errorf("messaggio: %q", msg)
	}
	if ea := b.effetto(r); ea.Spento != "" || !ea.Autorizzabile() {
		t.Errorf("da sottoassieme lo STEP si puo' autorizzare: %q", ea.Spento)
	}
	if got := b.dichiarazioneDi(comm); got != "nessuna" {
		t.Errorf("nessuna autorizzazione nata da sola: %s", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND (evidenza ? 'strutturale' OR deciso_da IS NOT NULL)`, b.thread); n != 0 {
		t.Errorf("%d righe marcate o decise dopo il cambio di tipo", n)
	}
	if _, figlio := b.autoritaOra().FiglioDiretto(ca.AllegatoID, "#2"); figlio {
		t.Error("7120098 resta guida finche' una persona non autorizza lo STEP")
	}
}

// Uscire da commerciale non riattiva niente nemmeno quando la sospensione non era registrata (un commerciale
// scritto prima della fase T, con una marcatura rimasta): il gesto la registra adesso («era commerciale») e la
// dichiarazione resta sospesa finche' una persona non la riattiva.
func TestUscireDaCommercialeNonRiattivaNienteSenzaRegistrazione(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaTipo()
	b.esegui(`UPDATE componente SET tipo = 'commerciale' WHERE componente_id = $1`, sc.s)
	if got := b.dichiarazioneDi(sc.s); !strings.HasPrefix(got, "sospesa: 7120010 è commerciale") {
		t.Fatalf("commerciale senza registrazione: %s", got)
	}
	e := b.effettoTipo(sc.s, db.TipoComponenteSottoassieme)
	if strings.Join(e.RestaSospesa, "|") != "l'autorizzazione di 7120010.stp per 7120010" {
		t.Fatalf("anteprima: %+v", e)
	}
	b.cambiaTipoOk(sc.s, db.TipoComponenteSottoassieme)
	if got := b.dichiarazioneDi(sc.s); !strings.HasPrefix(got, "sospesa: 7120010: era commerciale") {
		t.Errorf("tornato sottoassieme, la dichiarazione resta sospesa (registrata adesso): %s", got)
	}
	if got := uno[string](b, `SELECT evidenza -> 'strutturale' -> 'sospesa' ->> 'tipo' FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#1'`,
		b.thread, sc.sa.AllegatoID); got != "commerciale" {
		t.Errorf("la causa registrata: %s", got)
	}
	if _, err := b.riattiva(sc.s, sc.sa.Sha256.String); err != nil {
		t.Fatalf("la riattivazione esplicita: %v", err)
	}
	if got := b.dichiarazioneDi(sc.s); got != "valida" {
		t.Errorf("riattivata: %s", got)
	}
}

// Fase T, sciolto: un componente con dei figli nella working non diventa un particolare («ha 2 figli: è un
// assieme»); senza figli si'. La tendina lo dice prima. «Modifica» il tipo non lo cambia: la strada e' una sola,
// il gesto con l'anteprima e la firma (giro di correzione: prima «Modifica» passava dalle stesse regole).
//
// Riscritta per lo Smistamento (Distinta): prima fissava la frase «7120010 ha 2 figli: è un assieme». La PR #7
// aggiunge il consiglio («; per farlo diventare un particolare si spostano prima i suoi pezzi») e spegne con la
// stessa regola il particolare commerciale (domanda 6a, 29/09 sera: il commerciale e' sempre una foglia), che qui
// si controlla in piu'. Asserzioni: prima 8, dopo 9.
func TestUnoSciltoConFigliSiRifiuta(t *testing.T) {
	b := nuovoBanco(t)
	s := b.componente("7120010", db.TipoComponenteSottoassieme)
	f1, f2 := b.componente("7120011", db.TipoComponenteSciolto), b.componente("7120012", db.TipoComponenteSciolto)
	b.arco(s, f1, 1)
	b.arco(s, f2, 3)
	const frase = "7120010 ha 2 figli: è un assieme; per farlo diventare un particolare si spostano prima i suoi pezzi"
	e := b.effettoTipo(s, db.TipoComponenteSciolto)
	if e.Spento != frase {
		t.Errorf("anteprima: %q", e.Spento)
	}
	if e := b.effettoTipo(s, db.TipoComponenteCommerciale); e.Spento != "7120010 ha 2 figli: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi" {
		t.Errorf("anteprima del commerciale: %q", e.Spento)
	}
	for _, o := range b.effettoTipo(s, "").Opzioni {
		if o.Tipo == db.TipoComponenteSciolto && o.Spenta != frase {
			t.Errorf("la tendina: %+v", o)
		}
	}
	_, err := b.cambiaTipo(s, db.TipoComponenteSciolto)
	deveRifiutare(t, err, frase)
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ModificaComponente(b.ctx, q, b.thread, s, b.utente, db.TipoComponenteSciolto, "A", "")
	})
	deveRifiutare(t, err, fascicolo.RifiutoTipoDaModifica)
	if got := uno[string](b, `SELECT tipo::text || '/' || coalesce(rev, '-') FROM componente WHERE componente_id = $1`, s); got != "sottoassieme/-" {
		t.Errorf("niente e' cambiato: %s", got)
	}
	// senza figli si'
	b.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1`, s)
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ModificaComponente(b.ctx, q, b.thread, s, b.utente, db.TipoComponenteSciolto, "A", "")
	})
	deveRifiutare(t, err, fascicolo.RifiutoTipoDaModifica)
	if msg := b.cambiaTipoOk(s, db.TipoComponenteSciolto); msg != "7120010: tipo assieme → particolare." {
		t.Errorf("messaggio: %q", msg)
	}
	// «Modifica» con il tipo di adesso (o senza) cambia revisione e descrizione
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ModificaComponente(b.ctx, q, b.thread, s, b.utente, db.TipoComponenteSciolto, "A", "")
	})
	if err != nil || msg != "7120010: rev — → A." {
		t.Errorf("«Modifica» con lo stesso tipo: %q %v", msg, err)
	}
}

// Fase T, finito: solo per un codice della richiesta confermato da una persona, radice, senza delega; con lo
// STEP autorizzato proprio (valido) il file diventa anche lo STEP strutturale nella stessa transazione, e la
// dichiarazione resta valida (I10). Da finito ad assieme o particolare non si passa con lo STEP strutturale: si
// revoca prima, oppure diventa commerciale (TestIlFinitoAutorizzatoDiventaCommercialeESiSospende). Giro di
// correzione: prima fissava anche il rifiuto del commerciale da «Modifica»; adesso «Modifica» il tipo non lo
// cambia affatto, e il commerciale di un finito autorizzato si fa.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che il finito autorizzato 7120010, che nella working ha il
// figlio 7120098, potesse diventare commerciale. Con la regola della PR #7 (domanda 6a, 29/09 sera: il
// commerciale e' sempre una foglia) con il figlio il commerciale e' spento; tolto il figlio, si puo', e
// l'anteprima dice che lo STEP strutturale si svuota, come prima. Asserzioni (righe con t.Error, t.Fatal,
// deveRifiutare): prima 17, dopo 18.
func TestIlFinitoSoloDoveSiPuo(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaTipo()
	// 7120010 non e' un codice della richiesta; poi lo e', ma non confermato; poi confermato, ma sta sotto il prodotto
	if e := b.effettoTipo(sc.s, db.TipoComponenteFinito); !strings.Contains(e.Spento, "7120010 non è un codice della richiesta") {
		t.Errorf("fuori dalla richiesta: %q", e.Spento)
	}
	b.identificativo("7120010", "manuale", false)
	if e := b.effettoTipo(sc.s, db.TipoComponenteFinito); !strings.Contains(e.Spento, "non è un codice della richiesta") {
		t.Errorf("un codice non confermato non basta: %q", e.Spento)
	}
	b.esegui(`UPDATE identificativo_thread SET confermato_da = $2 WHERE thread_id = $1 AND codice = '7120010'`, b.thread, b.utente)
	if e := b.effettoTipo(sc.s, db.TipoComponenteFinito); !strings.Contains(e.Spento, "7120010 sta sotto 1 padre nella BOM: un prodotto finito è una radice") {
		t.Errorf("sotto un padre: %q", e.Spento)
	}
	_, err := b.cambiaTipo(sc.s, db.TipoComponenteFinito)
	deveRifiutare(t, err, "un prodotto finito è una radice")

	// tolto dal prodotto: con lo STEP autorizzato proprio, il file diventa lo STEP strutturale
	b.esegui(`DELETE FROM componente_relazione WHERE figlio_id = $1`, sc.s)
	e := b.effettoTipo(sc.s, db.TipoComponenteFinito)
	if e.Spento != "" || e.StepStrutturale != "7120010.stp" {
		t.Fatalf("anteprima del finito: %+v", e)
	}
	msg := b.cambiaTipoOk(sc.s, db.TipoComponenteFinito)
	if !strings.Contains(msg, "7120010: tipo assieme → prodotto.") || !strings.Contains(msg, "7120010.stp diventa anche il suo STEP strutturale") {
		t.Errorf("messaggio: %q", msg)
	}
	docStep := uno[string](b, `SELECT d.nome_file FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id WHERE c.componente_id = $1`, sc.s)
	if got := uno[string](b, `SELECT d.sha256 FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id WHERE c.componente_id = $1`, sc.s); got != sc.sa.Sha256.String {
		t.Errorf("lo STEP strutturale e' il documento di 7120010.stp: %s", got)
	}
	if got := b.dichiarazioneDi(sc.s); got != "valida" {
		t.Errorf("la dichiarazione del finito resta valida (marcatura e colonna dicono lo stesso file): %s", got)
	}

	// da finito ad assieme: non con lo STEP strutturale (si revoca, oppure diventa commerciale); da «Modifica» no
	frase := "7120010 ha uno STEP strutturale (" + docStep + "): resta un prodotto finito finché quel riferimento c'è"
	if e := b.effettoTipo(sc.s, db.TipoComponenteSottoassieme); !strings.Contains(e.Spento, frase) || !strings.Contains(e.Spento, "oppure diventa commerciale") {
		t.Errorf("da finito con lo STEP strutturale: %q", e.Spento)
	}
	if e := b.effettoTipo(sc.s, db.TipoComponenteCommerciale); e.Spento != "7120010 ha 1 figlio: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi" {
		t.Errorf("da finito con lo STEP autorizzato e un figlio, commerciale e' spento: %q", e.Spento)
	}
	b.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1`, sc.s)
	if e := b.effettoTipo(sc.s, db.TipoComponenteCommerciale); e.Spento != "" || e.SvuotaStep != docStep {
		t.Errorf("da finito con lo STEP autorizzato, senza figli, commerciale si puo': %+v", e)
	}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ModificaComponente(b.ctx, q, b.thread, sc.s, b.utente, db.TipoComponenteCommerciale, "", "")
	})
	deveRifiutare(t, err, fascicolo.RifiutoTipoDaModifica)
	if got := uno[string](b, `SELECT tipo::text || ':' || (step_strutturale_id IS NOT NULL)::text FROM componente WHERE componente_id = $1`, sc.s); got != "finito:true" {
		t.Errorf("«Modifica» non ha cambiato niente: %s", got)
	}
	if _, err := b.revoca(sc.s, ""); err != nil {
		t.Fatal(err)
	}
	e = b.effettoTipo(sc.s, db.TipoComponenteSottoassieme)
	if e.Spento != "" || !strings.Contains(strings.Join(e.Avvisi, "|"), "7120010 resta fra i codici della richiesta, ma nella BOM non è più un prodotto finito") {
		t.Errorf("revocato, da finito si esce: %+v", e)
	}
	b.cambiaTipoOk(sc.s, db.TipoComponenteSottoassieme)

	// una delega spegne il finito
	b.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 1, 'manuale', $4)`,
		b.thread, sc.prodotto, sc.s, b.utente)
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.s, Nodo: fascicolo.NodoFile{Allegato: sc.a.AllegatoID, Chiave: "#2"}})
	b.esegui(`DELETE FROM componente_relazione WHERE figlio_id = $1`, sc.s)
	if e := b.effettoTipo(sc.s, db.TipoComponenteFinito); !strings.Contains(e.Spento, "ha una delega nello STEP 7120001.stp") {
		t.Errorf("con una delega: %q", e.Spento)
	}

	// un finito senza STEP: si puo', e l'anteprima dice che il gate gli chiedera' lo STEP
	x := b.componente("7120050", db.TipoComponenteSottoassieme)
	b.identificativo("7120050", "manuale", true)
	if e := b.effettoTipo(x, db.TipoComponenteFinito); e.Spento != "" || !strings.Contains(strings.Join(e.Avvisi, "|"), "il gate chiede il suo STEP strutturale") {
		t.Errorf("un finito senza STEP: %+v", e)
	}
	b.cambiaTipoOk(x, db.TipoComponenteFinito)
	if got := uno[string](b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, x); got != "finito" {
		t.Errorf("il tipo: %s", got)
	}
}

// Fase T: la BOM congelata non cambia tipo (D26), ne' dal gesto ne' da «Modifica»; la tendina e' tutta spenta.
// La firma dell'anteprima si ricontrolla: una vecchia, o un'anteprima di un altro tipo, si rifiuta. Una
// sospensione registrata prima del congelamento (F1, con il suo STEP autorizzato, diventato commerciale e tornato
// particolare) non si riattiva con la BOM congelata: l'anteprima e' spenta e la scrittura si rifiuta. Giro di
// correzione: aggiunti «Modifica» e la riattivazione con la BOM congelata, che prima nessuna prova guardava.
func TestIlTipoConLaBomCongelataEConLaFirmaVecchia(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	e := b.effettoTipo(r.f1, db.TipoComponenteSottoassieme)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CambiaTipoComponenteVisto(b.ctx, q, b.thread, r.f1, db.TipoComponenteSottoassieme, b.utente, "deadbeef")
	})
	deveRifiutare(t, err, "riapri l'anteprima")
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CambiaTipoComponenteVisto(b.ctx, q, b.thread, r.f1, db.TipoComponenteCommerciale, b.utente, e.Firma)
	})
	deveRifiutare(t, err, "riapri l'anteprima")
	fatti := fattiSTEP{nodi: []string{"#1=F1", "#2=7120011"}, archi: []string{"#1>#2"}}
	_, fa := b.stepDelProdotto(r.f1, "F1", fatti)
	b.autorizza(r.f1, fa, fatti.json())
	b.cambiaTipoOk(r.f1, db.TipoComponenteCommerciale)
	b.cambiaTipoOk(r.f1, db.TipoComponenteSciolto)
	sha := fa.Sha256.String
	if er := b.effettoRiattivazione(r.f1, sha); er.Spento != "" || er.Sospensione == nil {
		t.Fatalf("prima del congelamento la sospensione registrata si riattiverebbe: %+v", er)
	}
	if _, err := b.congela("prova"); err != nil {
		t.Fatal(err)
	}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ModificaComponente(b.ctx, q, b.thread, r.f1, b.utente, db.TipoComponenteCommerciale, "", "")
	})
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	if er := b.effettoRiattivazione(r.f1, sha); !strings.Contains(er.Spento, "la BOM è congelata nella V1") {
		t.Errorf("la riattivazione con la BOM congelata: %q", er.Spento)
	}
	_, err = b.riattiva(r.f1, sha)
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	if got := b.dichiarazioneDi(r.f1); !strings.HasPrefix(got, "sospesa: F1: è diventato commerciale") {
		t.Errorf("con la BOM congelata resta sospesa: %s", got)
	}
	for _, o := range b.effettoTipo(r.f1, "").Opzioni {
		if !strings.Contains(o.Spenta, "la BOM è congelata nella V1") {
			t.Errorf("la tendina con la BOM congelata: %+v", o)
		}
	}
	_, err = b.cambiaTipo(r.f1, db.TipoComponenteCommerciale)
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CambiaTipoComponente(b.ctx, q, b.thread, r.f1, db.TipoComponenteCommerciale, b.utente)
	})
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	if got := uno[string](b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, r.f1); got != "sciolto" {
		t.Errorf("niente e' cambiato: %s", got)
	}
}

// Fase T, precisazione dell'utente (giro di correzione): un prodotto finito con lo STEP autorizzato nella forma
// dello Smistamento (la marcatura) passa a commerciale, non si rifiuta. Nella stessa transazione lo STEP
// strutturale si svuota (il CHECK di step_strutturale_id lo vuole solo per un finito, 0020:214), la sospensione si
// registra nella marcatura, la rimozione aperta si chiude con la nota, e gli archi che un automatismo aveva chiuso
// nell'autorita' tornano aperti con la storia (E33): il gate non li conta, le proposte sono guida. L'anteprima lo
// annuncia senza scrivere. Il ritorno e' esplicito: assieme (ancora sospesa, lo STEP strutturale resta vuoto),
// «Riattiva» (la rilettura richiude l'arco uguale alla working), prodotto finito (il file torna il suo STEP
// strutturale, e la dichiarazione vale: I10). Assieme e particolare, da finito, restano spenti.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che P1, con i figli B, C e D nella working, diventasse
// commerciale con il gesto. Con la regola della PR #7, confermata dall'utente il 29/09 sera (domanda 6a: il
// commerciale e' sempre una foglia), con i figli il passaggio e' spento e si rifiuta, e niente cambia. Un
// prodotto commerciale con i suoi figli resta il caso dei dati di prima (fino alla PR #7 il passaggio si faceva e
// i figli restavano), ed e' quello su cui la sospensione, le rimozioni e gli archi chiusi da un automatismo devono
// reggere: gli archi si tolgono per il gesto (l'anteprima e il gesto sono quelli di un prodotto senza figli) e si
// rimettono com'erano con una scrittura diretta; il resto della prova e' com'era. Il nome del tipo nel messaggio
// e' «particolare commerciale» (NomeTipo, PR #7). Asserzioni (righe con t.Error, t.Fatal, deveRifiutare, ok): prima
// 29, dopo 34.
func TestIlFinitoAutorizzatoDiventaCommercialeESiSospende(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	p1 := codiceDi("P1")
	b.identificativo(p1, "manuale", true)
	doc, a := b.stepDelProdotto(c["P1"], p1, stepPBCD())
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: c["P1"], Documento: doc})
	if err := b.accettaIn(a, "#2", "#3", "#5"); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozioni(); got != "P1>D:aperta" {
		t.Fatalf("rimozioni di partenza: %q", got)
	}
	if got := b.relazioniProposte(); !strings.Contains(got, "#1>#3*1:duplicato") {
		t.Fatalf("l'arco P1 → C uguale alla working, chiuso dall'automatismo: %s", got)
	}
	prima := b.strutturali()
	if prima.NArchiDaDecidere == 0 || prima.NRimozioni != 1 {
		t.Fatalf("prima il gate conta il file di P1: %+v", prima)
	}
	nome := uno[string](b, `SELECT nome_file FROM documento WHERE documento_id = $1`, doc)
	stepDi := func() string {
		return uno[string](b, `SELECT tipo::text || ':' || coalesce(step_strutturale_id::text, '-') FROM componente WHERE componente_id = $1`, c["P1"])
	}

	// assieme e particolare restano spenti, con il consiglio; commerciale con i figli e' spento e si rifiuta
	if e := b.effettoTipo(c["P1"], db.TipoComponenteSottoassieme); !strings.Contains(e.Spento, "ha uno STEP strutturale ("+nome+")") ||
		!strings.Contains(e.Spento, "oppure diventa commerciale") {
		t.Errorf("da finito ad assieme: %q", e.Spento)
	}
	conFigli := p1 + " ha 3 figli: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi"
	foto := b.improntaAutorizzazione()
	if e := b.effettoTipo(c["P1"], db.TipoComponenteCommerciale); e.Spento != conFigli {
		t.Errorf("da finito con i figli a commerciale: %q", e.Spento)
	}
	_, err := b.cambiaTipo(c["P1"], db.TipoComponenteCommerciale)
	deveRifiutare(t, err, conFigli)
	if b.improntaAutorizzazione() != foto || stepDi() != "finito:"+doc.String() || b.rimozioni() != "P1>D:aperta" {
		t.Fatal("il rifiuto ha cambiato qualcosa")
	}

	// i dati di prima: il gesto su P1 senza gli archi, che poi si rimettono com'erano
	righe, err := b.p.Query(b.ctx, `SELECT * FROM componente_relazione WHERE padre_id = $1 ORDER BY figlio_id`, c["P1"])
	ok(t, err)
	archi, err := pgx.CollectRows(righe, pgx.RowToStructByName[db.ComponenteRelazione])
	ok(t, err)
	b.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1`, c["P1"])
	e := b.effettoTipo(c["P1"], db.TipoComponenteCommerciale)
	if b.improntaAutorizzazione() != foto || stepDi() != "finito:"+doc.String() {
		t.Fatal("l'anteprima del cambio di tipo ha scritto")
	}
	if e.Spento != "" || e.SvuotaStep != nome || strings.Join(e.Sospende, "|") != "l'autorizzazione di "+a.NomeFile+" per "+p1 || e.Rimozioni != 1 {
		t.Fatalf("anteprima del finito che diventa commerciale: %+v", e)
	}
	if !strings.Contains(strings.Join(e.Avvisi, "|"), p1+" resta fra i codici della richiesta") {
		t.Errorf("gli avvisi: %v", e.Avvisi)
	}

	msg := b.cambiaTipoOk(c["P1"], db.TipoComponenteCommerciale)
	for _, x := range archi {
		b.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, posizione, origine, confermato_da, creato_il)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, x.ThreadID, x.PadreID, x.FiglioID, x.Qta, x.Posizione, x.Origine, x.ConfermatoDa, x.CreatoIl)
	}
	for _, x := range []string{p1 + ": tipo prodotto → particolare commerciale.", "Sospesa l'autorizzazione di " + a.NomeFile + " per " + p1,
		nome + " non è più il suo STEP strutturale"} {
		if !strings.Contains(msg, x) {
			t.Errorf("messaggio: manca %q in %q", x, msg)
		}
	}
	if got := stepDi(); got != "commerciale:-" {
		t.Errorf("il tipo e lo STEP strutturale svuotato: %s", got)
	}
	if got := b.dichiarazioneDi(c["P1"]); !strings.HasPrefix(got, "sospesa: "+p1+": è diventato commerciale") {
		t.Errorf("la dichiarazione: %s", got)
	}
	if got := b.statoNodo(a, "#1"); got != "duplicato "+p1+" persona marcato:radice" {
		t.Errorf("la sorgente resta decisa e marcata (sospesa non e' revocata): %s", got)
	}
	if got := b.rimozioni(); got != "P1>D:scartata" {
		t.Errorf("la rimozione si chiude: %q", got)
	}
	if got := b.relazioniProposte(); strings.Contains(got, "duplicato") || !strings.Contains(got, "#1>#3*1:aperta") {
		t.Errorf("gli archi chiusi dall'automatismo tornano aperti: %s", got)
	}
	if got := uno[string](b, `SELECT coalesce(string_agg(e ->> 'evento', ' '), '') FROM relazione_proposta, jsonb_array_elements(coalesce(evidenza -> 'storia', '[]'::jsonb)) e
		WHERE allegato_id = $1 AND padre_chiave = '#1' AND figlio_chiave = '#3'`, a.AllegatoID); got != "arco_automatico_riaperto" {
		t.Errorf("la storia dell'arco riaperto: %q", got)
	}
	if g := b.strutturali(); g.NFigliDaDecidere+g.NArchiDaDecidere+g.NRimozioni != 0 {
		t.Errorf("il gate non conta niente del file sospeso: %+v", g)
	}
	_, err = b.accettaArco(a, "#1", "#3")
	deveRifiutare(t, err, "è guida")

	// assieme: ancora sospesa, lo STEP strutturale resta vuoto, l'arco resta aperto
	proposte := b.improntaProposte()
	if msg := b.cambiaTipoOk(c["P1"], db.TipoComponenteSottoassieme); !strings.Contains(msg, "Resta sospesa l'autorizzazione di "+a.NomeFile) {
		t.Errorf("di nuovo assieme: %q", msg)
	}
	if b.improntaProposte() != proposte {
		t.Error("il ritorno ad assieme ha toccato le proposte")
	}
	if got := stepDi(); got != "sottoassieme:-" {
		t.Errorf("da assieme lo STEP strutturale resta vuoto: %s", got)
	}
	if got := b.dichiarazioneDi(c["P1"]); !strings.HasPrefix(got, "sospesa: "+p1+": è diventato commerciale") {
		t.Errorf("tornato assieme resta sospesa: %s", got)
	}

	// «Riattiva»: la rilettura richiude l'arco uguale alla working, il gate conta di nuovo il file
	if _, err := b.riattiva(c["P1"], a.Sha256.String); err != nil {
		t.Fatalf("la riattivazione: %v", err)
	}
	if got := b.dichiarazioneDi(c["P1"]); got != "valida" {
		t.Errorf("riattivata: %s", got)
	}
	if got := b.relazioniProposte(); !strings.Contains(got, "#1>#3*1:duplicato") {
		t.Errorf("riattivata, la rilettura richiude l'arco uguale alla working: %s", got)
	}
	if g := b.strutturali(); g.NArchiDaDecidere != prima.NArchiDaDecidere {
		t.Errorf("riattivata, il gate conta di nuovo gli archi del file: %+v (prima %+v)", g, prima)
	}

	// prodotto finito: il file torna il suo STEP strutturale
	if e := b.effettoTipo(c["P1"], db.TipoComponenteFinito); e.Spento != "" || e.StepStrutturale != a.NomeFile {
		t.Fatalf("anteprima del ritorno a prodotto finito: %+v", e)
	}
	if msg := b.cambiaTipoOk(c["P1"], db.TipoComponenteFinito); !strings.Contains(msg, a.NomeFile+" diventa anche il suo STEP strutturale") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := stepDi(); got != "finito:"+doc.String() {
		t.Errorf("il file torna lo STEP strutturale: %s", got)
	}
	if got := b.dichiarazioneDi(c["P1"]); got != "valida" {
		t.Errorf("da prodotto finito la dichiarazione vale (I10): %s", got)
	}
}

// Il finito nella forma di prima (la sola colonna step_strutturale_id, senza la marcatura) non ha dove registrare
// la sospensione: il passaggio a commerciale resta spento, con il motivo, e niente cambia.
func TestIlFinitoNellaFormaDiPrimaNonDiventaCommerciale(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	e := b.effettoTipo(r.p1, db.TipoComponenteCommerciale)
	if !strings.Contains(e.Spento, "P1 ha uno STEP strutturale") || !strings.Contains(e.Spento, "è nella forma di prima") || e.SvuotaStep != "" {
		t.Errorf("il finito nella forma di prima: %+v", e)
	}
	_, err := b.cambiaTipo(r.p1, db.TipoComponenteCommerciale)
	deveRifiutare(t, err, "è nella forma di prima")
	if got := uno[string](b, `SELECT tipo::text || ':' || (step_strutturale_id IS NOT NULL)::text FROM componente WHERE componente_id = $1`, r.p1); got != "finito:true" {
		t.Errorf("niente e' cambiato: %s", got)
	}
}

// Fase T, E33 al passaggio a commerciale (giro di correzione): nella working c'e' gia' 7120010 → 7120011 ×2, e
// la rilettura del file autorizzato di 7120010 chiude l'arco proposto come «duplicato» senza chi l'ha deciso.
// Diventato commerciale, l'arco torna aperto con la storia (arco_automatico_riaperto) e il gate non lo conta (e'
// guida); tornato assieme resta aperto (l'autorita' e' sospesa, la rilettura non lo richiude); dopo «Riattiva» la
// rilettura lo richiude, e il gate conta di nuovo i figli e gli archi del file (le rimozioni aspettano che i figli
// diretti siano decisi: TestIlCambioSottoassiemeCommercialeSospendeLAutorita).
//
// Riscritta per lo Smistamento (Distinta): prima fissava che il gesto portasse a commerciale 7120010, che nella
// working ha i figli 7120011 e 7120098. Con la regola della PR #7 (domanda 6a, 29/09 sera: il commerciale e'
// sempre una foglia) quel gesto si rifiuta (lo prova commercialeDiPrima); un arco uguale alla working sotto un
// commerciale c'e' solo nei dati di prima, che commercialeDiPrima ricostruisce, e il resto della prova e' com'era.
// Asserzioni (righe con t.Error, t.Fatal, deveRifiutare): prima 9, dopo 10, piu' il rifiuto che
// commercialeDiPrima controlla.
func TestIlCommercialeRiapreGliArchiChiusiDaUnAutomatismo(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaTipo()
	b.arco(sc.s, b.codiceComp("7120011"), 2)
	b.applica(sc.sa, fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120099"}, archi: []string{"#1>#2*2", "#1>#3"}}.json())
	arco := func() string {
		return uno[string](b, `SELECT stato::text || ':' || (deciso_da IS NULL)::text FROM relazione_proposta
			WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = '#1' AND figlio_chiave = '#2'`, b.thread, sc.sa.AllegatoID)
	}
	if got := arco(); got != "duplicato:true" {
		t.Fatalf("prima: l'arco 7120010 → 7120011, uguale alla working, e' chiuso dall'automatismo: %s", got)
	}
	prima := b.strutturali()

	if msg := b.commercialeDiPrima(sc.s); !strings.Contains(msg, "Sospesa l'autorizzazione di 7120010.stp per 7120010") {
		t.Errorf("il passaggio a commerciale (dati di prima): %q", msg)
	}
	if got := arco(); got != "aperta:true" {
		t.Errorf("commerciale: l'arco chiuso dall'automatismo torna aperto: %s", got)
	}
	if got := uno[string](b, `SELECT coalesce(string_agg(e ->> 'evento', ' '), '') FROM relazione_proposta, jsonb_array_elements(coalesce(evidenza -> 'storia', '[]'::jsonb)) e
		WHERE allegato_id = $1 AND padre_chiave = '#1' AND figlio_chiave = '#2'`, sc.sa.AllegatoID); got != "arco_automatico_riaperto" {
		t.Errorf("la storia dell'arco riaperto: %q", got)
	}
	if s := b.strutturali(); s != (db.GateStrutturaleRow{NFigliDaDecidere: 1, NArchiDaDecidere: 1}) {
		t.Errorf("il gate non conta l'arco riaperto di un file sospeso: %+v", s)
	}
	_, err := b.accettaArco(sc.sa, "#1", "#2")
	deveRifiutare(t, err, "7120010 → 7120011 è guida")

	b.cambiaTipoOk(sc.s, db.TipoComponenteSottoassieme)
	if got := arco(); got != "aperta:true" {
		t.Errorf("tornato assieme, l'arco resta aperto: %s", got)
	}
	if _, err := b.riattiva(sc.s, sc.sa.Sha256.String); err != nil {
		t.Fatalf("la riattivazione: %v", err)
	}
	if got := arco(); got != "duplicato:true" {
		t.Errorf("riattivata, la rilettura richiude l'arco uguale alla working: %s", got)
	}
	if s := b.strutturali(); s.NFigliDaDecidere != prima.NFigliDaDecidere || s.NArchiDaDecidere != prima.NArchiDaDecidere {
		t.Errorf("riattivata, il gate conta di nuovo il file: %+v (prima %+v)", s, prima)
	}
}

// Il consiglio della riattivazione di una delega che resterebbe senza la catena (giro di correzione): 7120010 e
// 7120011, delegati uno sotto l'altro nello STEP del prodotto, diventano commerciali uno dopo l'altro e tornano
// assiemi. La riattivazione di 7120011 e' spenta, e dice di riattivare prima la delega di 7120010 (revocarla la
// perderebbe senza motivo); riattivata quella, la delega di 7120011 non torna da sola (la sua sospensione e'
// registrata), e la sua riattivazione si fa.
func TestLaRiattivazioneDellaDelegaSottoDiceDiRiattivarePrimaIlPadre(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	s := b.codiceComp("7120010")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Deleghe: []string{"#2"}})
	if err := b.accettaIn(a, "#4"); err != nil {
		t.Fatal(err)
	}
	n := b.codiceComp("7120011")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: n, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#4"}})
	sha := a.Sha256.String
	b.cambiaTipoOk(s, db.TipoComponenteCommerciale)
	b.cambiaTipoOk(n, db.TipoComponenteCommerciale)
	b.cambiaTipoOk(n, db.TipoComponenteSottoassieme)
	b.cambiaTipoOk(s, db.TipoComponenteSottoassieme)
	for _, c := range []uuid.UUID{s, n} {
		if got := b.dichiarazioneDi(c); !strings.Contains(got, "è diventato commerciale") {
			t.Fatalf("tutte e due sospese, registrate: %s", got)
		}
	}

	er := b.effettoRiattivazione(n, sha)
	if !strings.Contains(er.Spento, "riattivata, la delega di 7120001.stp per 7120011 non varrebbe") ||
		!strings.HasSuffix(er.Spento, ": si riattiva prima la delega di 7120001.stp per 7120010") {
		t.Errorf("il consiglio della riattivazione di 7120011: %q", er.Spento)
	}
	_, err := b.riattiva(n, sha)
	deveRifiutare(t, err, "si riattiva prima la delega di 7120001.stp per 7120010")

	er = b.effettoRiattivazione(s, sha)
	if er.Spento != "" || len(er.ConLei) != 0 {
		t.Fatalf("la riattivazione di 7120010 si fa, e non porta con se' la delega sospesa di 7120011: %+v", er)
	}
	if _, err := b.riattiva(s, sha); err != nil {
		t.Fatal(err)
	}
	if got := b.dichiarazioneDi(n); !strings.Contains(got, "è diventato commerciale") {
		t.Errorf("la delega di 7120011 non torna da sola: %s", got)
	}
	if er := b.effettoRiattivazione(n, sha); er.Spento != "" {
		t.Errorf("riattivato il padre, la delega di 7120011 si riattiva: %q", er.Spento)
	}
	if _, err := b.riattiva(n, sha); err != nil {
		t.Fatal(err)
	}
	for _, c := range []uuid.UUID{s, n} {
		if got := b.dichiarazioneDi(c); got != "valida" {
			t.Errorf("riattivate: %s", got)
		}
	}
}
