package fascicolo

// L1 — Fascicolo v3: la struttura voluta dall'editor, controllata da pianifica senza scrivere niente.
// Riferimenti (c:, p:, k:), codici, quantita', archi ripetuti, la radice, l'albero, gli archi che l'editor
// non mostrava, i cicli, gli scarti. Le prove L4 che la applicano davvero stanno in voluta_db_test.go.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// bancoVoluta: il prodotto 52922757 → l'assieme 52920517 ×2 → il particolare 53017189 ×4, e il particolare
// anche sotto il prodotto ×1 (condiviso). Uno STEP propone 53011111 (nuovo), 52920517 (c'e' gia'), un nodo
// senza codice, una radice che e' il prodotto, un nodo gia' scartato.
type bancoVoluta struct {
	prodotto, assieme, particolare             db.Componente
	nuovo, ritrovato, senzaCodice, radice, via db.ComponenteProposta
	comp                                       []db.Componente
	rel                                        []db.ComponenteRelazione
	trovati                                    map[string]CodiceScritto
}

func propostaVoluta(chiave, codice string, stato db.StatoProposta) db.ComponenteProposta {
	return db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: uuid.New(), Chiave: chiave, NomeGrezzo: "nodo " + chiave,
		Codice: pgtype.Text{String: codice, Valid: codice != ""}, Stato: stato}
}

func nuovoBancoVoluta() *bancoVoluta {
	b := &bancoVoluta{
		prodotto:    componente("52922757", db.TipoComponenteFinito, false),
		assieme:     componente("52920517", db.TipoComponenteSottoassieme, false),
		particolare: componente("53017189", db.TipoComponenteSciolto, false),
		nuovo:       propostaVoluta("#3", "53011111", db.StatoPropostaAperta),
		ritrovato:   propostaVoluta("#2", "52920517", db.StatoPropostaAperta),
		senzaCodice: propostaVoluta("#4", "", db.StatoPropostaAperta),
		radice:      propostaVoluta("#1", "PRT-0001", db.StatoPropostaAperta),
		via:         propostaVoluta("#5", "53055555", db.StatoPropostaScartata),
		trovati:     map[string]CodiceScritto{"53088888": {Codice: "53088888", Rev: "B"}},
	}
	b.comp = []db.Componente{b.prodotto, b.assieme, b.particolare}
	b.rel = []db.ComponenteRelazione{
		{PadreID: b.prodotto.ComponenteID, FiglioID: b.assieme.ComponenteID, Qta: 2},
		{PadreID: b.assieme.ComponenteID, FiglioID: b.particolare.ComponenteID, Qta: 4},
		{PadreID: b.prodotto.ComponenteID, FiglioID: b.particolare.ComponenteID, Qta: 1},
	}
	return b
}

func (b *bancoVoluta) contesto() contestoVoluta {
	return nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{b.nuovo, b.ritrovato, b.senzaCodice, b.radice, b.via}, b.trovati)
}

func c(x db.Componente) string         { return "c:" + x.ComponenteID.String() }
func p(x db.ComponenteProposta) string { return "p:" + x.PropostaID.String() }

// la struttura di adesso, cosi' come l'editor la mostra: archi voluti = archi visti
func (b *bancoVoluta) comeE() StrutturaVoluta {
	v := StrutturaVoluta{Radice: b.prodotto.ComponenteID}
	for _, r := range b.rel {
		a := ArcoVoluto{Padre: "c:" + r.PadreID.String(), Figlio: "c:" + r.FiglioID.String(), Qta: r.Qta}
		v.Archi = append(v.Archi, a)
		v.Visti = append(v.Visti, a)
	}
	return v
}

func rifiutata(t *testing.T, cx contestoVoluta, v StrutturaVoluta, pezzo string) {
	t.Helper()
	_, err := pianifica(cx, v)
	if err == nil {
		t.Fatalf("attesa un rifiuto con %q, invece la struttura passa", pezzo)
	}
	if _, ok := err.(Rifiuto); !ok {
		t.Fatalf("il rifiuto non e' un Rifiuto: %T %v", err, err)
	}
	if !strings.Contains(err.Error(), pezzo) {
		t.Fatalf("rifiuto %q, atteso che dica %q", err, pezzo)
	}
}

// La struttura com'e', piu' il nodo nuovo sotto l'assieme e il nodo proposto che c'e' gia': il nuovo nasce
// da quella proposta, l'altro si ritrova nel componente con il suo codice, l'albero e' tutto.
func TestPianificaLaStrutturaConINodiProposti(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: p(b.nuovo), Qta: 2},
		ArcoVoluto{Padre: p(b.ritrovato), Figlio: "k:53088888", Qta: 3})
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	if ids := pv.Nuovi["n:53011111"]; len(ids) != 1 || ids[0] != b.nuovo.PropostaID {
		t.Errorf("53011111 nasce dalla sua proposta: %v", pv.Nuovi)
	}
	if pv.Ritrovati[b.ritrovato.PropostaID] != b.assieme.ComponenteID {
		t.Errorf("il nodo 52920517 si ritrova nell'assieme: %v", pv.Ritrovati)
	}
	if got := pv.Trovati["n:53088888"]; got.Codice != "53088888" || got.Rev != "B" || pv.Nuovi["n:53088888"] != nil {
		t.Errorf("il codice trovato nasce da se', con la sua revisione: %+v %v", got, pv.Nuovi["n:53088888"])
	}
	for _, k := range []chiaveNodo{chiaveComponente(b.prodotto.ComponenteID), chiaveComponente(b.assieme.ComponenteID),
		chiaveComponente(b.particolare.ComponenteID), "n:53011111", "n:53088888"} {
		if !pv.Albero[k] {
			t.Errorf("%s non e' nell'albero", k)
		}
	}
	if pv.Figli[chiaveComponente(b.assieme.ComponenteID)] != 3 || len(pv.Archi) != 5 {
		t.Errorf("figli dell'assieme %d, archi %d", pv.Figli[chiaveComponente(b.assieme.ComponenteID)], len(pv.Archi))
	}
}

// La radice: un prodotto finito di questa RFQ, non archiviato, che non va sotto nessuno.
func TestPianificaLaRadice(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.Radice = b.assieme.ComponenteID
	rifiutata(t, b.contesto(), v, "non è un prodotto finito")
	v.Radice = uuid.New()
	rifiutata(t, b.contesto(), v, "non è un componente di questa RFQ")

	v = b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.particolare), Figlio: c(b.prodotto), Qta: 1})
	rifiutata(t, b.contesto(), v, "è la radice della struttura")

	archiviato := nuovoBancoVoluta()
	a := componente("52922757", db.TipoComponenteFinito, true)
	archiviato.prodotto = a
	archiviato.comp[0] = a
	rifiutata(t, archiviato.contesto(), StrutturaVoluta{Radice: a.ComponenteID}, "è archiviato")
}

// Le quantita' e le ripetizioni: da 1 al massimo, un arco una volta sola, un pezzo non sotto se stesso.
func TestPianificaQuantitaERipetizioni(t *testing.T) {
	b := nuovoBancoVoluta()
	for _, q := range []int32{0, -1, MaxQtaArco + 1} {
		v := b.comeE()
		v.Archi[0].Qta = q
		rifiutata(t, b.contesto(), v, "la quantità va da 1")
	}
	v := b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.prodotto), Figlio: c(b.assieme), Qta: 5})
	rifiutata(t, b.contesto(), v, "è due volte sotto")

	v = b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: c(b.assieme), Qta: 1})
	rifiutata(t, b.contesto(), v, "non sta sotto se stesso")

	// due nodi proposti con lo stesso codice, uno dentro l'altro (STEP veri): non e' un errore, l'arco si salta
	gemello := propostaVoluta("#9", "53011111", db.StatoPropostaAperta)
	cx := nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{b.nuovo, gemello}, nil)
	v = b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: p(b.nuovo), Qta: 1}, ArcoVoluto{Padre: p(b.nuovo), Figlio: p(gemello), Qta: 1})
	pv, err := pianifica(cx, v)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Archi) != 4 || len(pv.Nuovi["n:53011111"]) != 2 {
		t.Errorf("l'arco fra i due gemelli si salta, tutti e due portano 53011111: %d archi, %v", len(pv.Archi), pv.Nuovi)
	}
}

// Ogni arco e' appeso all'albero della radice; un arco della working che parte dall'albero e che l'editor non
// mostrava vuol dire dati vecchi.
func TestPianificaAlberoEArchiNonVisti(t *testing.T) {
	b := nuovoBancoVoluta()
	// il particolare esce dall'albero (i suoi due archi visti e tolti) e porta sotto di se' il nodo nuovo:
	// quell'arco non e' appeso a niente
	v := StrutturaVoluta{Radice: b.prodotto.ComponenteID, Visti: b.comeE().Visti,
		Archi: []ArcoVoluto{{Padre: c(b.prodotto), Figlio: c(b.assieme), Qta: 2}, {Padre: c(b.particolare), Figlio: p(b.nuovo), Qta: 1}}}
	rifiutata(t, b.contesto(), v, "non è appeso alla struttura di 52922757")

	// togliere un arco visto va bene
	v.Archi = v.Archi[:1]
	if _, err := pianifica(b.contesto(), v); err != nil {
		t.Errorf("togliere archi che l'editor mostrava: %v", err)
	}
	// ma uno che l'editor non conosceva (un collega nel frattempo) fa rifiutare tutto
	v.Visti = v.Visti[:1]
	rifiutata(t, b.contesto(), v, "che l'editor non mostrava")

	// un arco visto malformato
	v = b.comeE()
	v.Visti[0].Padre = "c:non-un-uuid"
	rifiutata(t, b.contesto(), v, "riapri l'editor")

	// troppi archi
	v = b.comeE()
	for i := 0; i <= MaxArchiVoluti; i++ {
		v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.prodotto), Figlio: c(b.assieme), Qta: 1})
	}
	rifiutata(t, b.contesto(), v, "il limite è")
}

// Il grafo finale non ha cicli, anche contando gli archi della working fuori dall'albero.
func TestPianificaRifiutaICicli(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.particolare), Figlio: c(b.assieme), Qta: 1})
	rifiutata(t, b.contesto(), v, "chiuderebbe un ciclo")

	// il ciclo passa per un nodo nuovo: si nomina con il suo codice
	v = b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.particolare), Figlio: p(b.nuovo), Qta: 1}, ArcoVoluto{Padre: p(b.nuovo), Figlio: c(b.assieme), Qta: 1})
	_, err := pianifica(b.contesto(), v)
	if err == nil || !strings.Contains(err.Error(), "53011111") {
		t.Errorf("il ciclo con il nodo nuovo: %v", err)
	}

	// un altro prodotto con l'assieme sotto: portato nell'albero, porta i suoi archi. Se l'editor non li
	// mostrava la struttura e' vecchia; se li mostrava e li tiene, il giro si chiude
	altro := componente("52999999", db.TipoComponenteFinito, false)
	b.comp = append(b.comp, altro)
	b.rel = append(b.rel, db.ComponenteRelazione{PadreID: altro.ComponenteID, FiglioID: b.assieme.ComponenteID, Qta: 1})
	v = b.comeE()
	v.Archi, v.Visti = v.Archi[:3], v.Visti[:3]
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.particolare), Figlio: c(altro), Qta: 1})
	rifiutata(t, b.contesto(), v, "che l'editor non mostrava")
	tenuto := ArcoVoluto{Padre: c(altro), Figlio: c(b.assieme), Qta: 1}
	v.Visti = append(v.Visti, tenuto)
	v.Archi = append(v.Archi, tenuto)
	rifiutata(t, b.contesto(), v, "chiuderebbe un ciclo")
	// fuori dall'albero lo stesso arco non si tocca e non conta: nessun giro
	v = b.comeE()
	v.Archi, v.Visti = v.Archi[:3], v.Visti[:3]
	if _, err := pianifica(b.contesto(), v); err != nil {
		t.Errorf("l'arco di un altro prodotto, fuori dall'albero: %v", err)
	}
}

// I riferimenti p: e k:, e i codici scritti dall'operatore.
func TestPianificaRiferimentiECodici(t *testing.T) {
	b := nuovoBancoVoluta()
	sotto := func(ref string) StrutturaVoluta {
		v := b.comeE()
		v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: ref, Qta: 1})
		return v
	}
	rifiutata(t, b.contesto(), sotto(p(b.senzaCodice)), "non ha un codice")
	rifiutata(t, b.contesto(), sotto(p(b.via)), "è stato scartato")
	rifiutata(t, b.contesto(), sotto("p:"+uuid.NewString()), "non è di questa RFQ")
	rifiutata(t, b.contesto(), sotto("c:"+uuid.NewString()), "non è di questa RFQ")
	rifiutata(t, b.contesto(), sotto("k:53077777"), "non è fra i codici trovati")
	rifiutata(t, b.contesto(), sotto("x:qualcosa"), "non è valido")

	// un codice trovato che e' gia' un componente e' quel componente
	v := sotto("k:53017189")
	v.Archi = v.Archi[:len(v.Archi)-1]
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.prodotto), Figlio: "k:52920517", Qta: 7})
	rifiutata(t, b.contesto(), v, "è due volte sotto")

	// il codice scritto dall'operatore fa del nodo senza codice un codice nuovo
	v = sotto(p(b.senzaCodice))
	v.Codici = map[string]CodiceScritto{b.senzaCodice.PropostaID.String(): {Codice: "53066666", Rev: "a"}}
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	if ids := pv.Nuovi["n:53066666"]; len(ids) != 1 || ids[0] != b.senzaCodice.PropostaID {
		t.Errorf("il nodo senza codice nasce con il codice scritto: %v", pv.Nuovi)
	}
	if pv.Codici[b.senzaCodice.PropostaID].Rev != "A" {
		t.Errorf("la revisione scritta, in maiuscolo: %+v", pv.Codici)
	}
	// codici non ammessi, su nodi non aperti, su nodi di altre RFQ
	for _, cs := range []CodiceScritto{{Codice: "53 06/6666"}, {Codice: strings.Repeat("9", 80)}, {Codice: "53066666", Rev: "REV MOLTO LUNGA"}} {
		v.Codici = map[string]CodiceScritto{b.senzaCodice.PropostaID.String(): cs}
		rifiutata(t, b.contesto(), v, "caratteri non ammessi")
	}
	v.Codici = map[string]CodiceScritto{b.via.PropostaID.String(): {Codice: "53066666"}}
	rifiutata(t, b.contesto(), v, "la proposta è già decisa")
	v.Codici = map[string]CodiceScritto{uuid.NewString(): {Codice: "53066666"}}
	rifiutata(t, b.contesto(), v, "non è di questa RFQ")
	v.Codici = map[string]CodiceScritto{"non-un-uuid": {Codice: "53066666"}}
	rifiutata(t, b.contesto(), v, "non valido")
}

// Una radice dello STEP che e' il prodotto: i suoi archi diventano archi del prodotto.
func TestPianificaLaRadiceDelloStepEIlProdotto(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.RadiciProposte = []uuid.UUID{b.radice.PropostaID, b.radice.PropostaID}
	v.Archi = append(v.Archi, ArcoVoluto{Padre: p(b.radice), Figlio: p(b.nuovo), Qta: 2})
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	ultimo := pv.Archi[len(pv.Archi)-1]
	if ultimo.Padre != chiaveComponente(b.prodotto.ComponenteID) || ultimo.Figlio != "n:53011111" {
		t.Errorf("l'arco della radice dello STEP e' del prodotto: %+v", ultimo)
	}
	if len(pv.Radici) != 1 || pv.Radici[0] != b.radice.PropostaID {
		t.Errorf("la radice si ritrova una volta: %v", pv.Radici)
	}
	v.RadiciProposte = []uuid.UUID{b.via.PropostaID}
	rifiutata(t, b.contesto(), v, "la proposta è già decisa")
}

// Gli scarti: nodi aperti, non nella struttura, non il prodotto.
func TestPianificaGliScarti(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.Scarta = []uuid.UUID{b.senzaCodice.PropostaID, b.senzaCodice.PropostaID}
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Scarta) != 1 {
		t.Errorf("lo scarto si conta una volta: %v", pv.Scarta)
	}
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: p(b.nuovo), Qta: 1})
	v.Scarta = []uuid.UUID{b.nuovo.PropostaID}
	rifiutata(t, b.contesto(), v, "o l'uno o l'altro")

	v = b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.prodotto), Figlio: p(b.ritrovato), Qta: 2})
	v.Archi = v.Archi[1:] // il ritrovato e' l'assieme: l'arco prodotto → assieme lo dice lui
	v.Scarta = []uuid.UUID{b.ritrovato.PropostaID}
	rifiutata(t, b.contesto(), v, "o l'uno o l'altro")

	v = b.comeE()
	v.RadiciProposte = []uuid.UUID{b.radice.PropostaID}
	v.Scarta = []uuid.UUID{b.radice.PropostaID}
	rifiutata(t, b.contesto(), v, "è il prodotto")

	v = b.comeE()
	v.Scarta = []uuid.UUID{b.via.PropostaID}
	rifiutata(t, b.contesto(), v, "già decisa")
	v.Scarta = []uuid.UUID{uuid.New()}
	rifiutata(t, b.contesto(), v, "non è di questa RFQ")
}

// La frase all'operatore dice che cosa e' cambiato, e chi e' rimasto senza padre.
func TestFraseVoluta(t *testing.T) {
	if got := fraseVoluta("52922757", esitoVoluta{}); got != "Nessun cambiamento: la struttura di 52922757 è già così." {
		t.Errorf("niente: %q", got)
	}
	got := fraseVoluta("52922757", esitoVoluta{nuovi: 1, aggiunti: 2, tolti: 1, chiuse: 1, senzaPadre: []string{"53017189"}})
	if got != "Struttura di 52922757 confermata: 1 componente nuovo, 2 legami aggiunti, 1 legame tolto, 1 proposta di legame chiusa. Fuori dalla struttura e senza padre, da sistemare: 53017189." {
		t.Errorf("frase: %q", got)
	}
}

// Il tipo con cui nasce un nodo: assieme con dei figli, altrimenti quello suggerito (mai prodotto).
func TestTipoVoluto(t *testing.T) {
	n := propostaVoluta("#3", "53011111", db.StatoPropostaAperta)
	if got := tipoVoluto(n, 2); got != db.TipoComponenteSottoassieme {
		t.Errorf("con figli: %s", got)
	}
	if got := tipoVoluto(n, 0); got != db.TipoComponenteSciolto {
		t.Errorf("senza suggerimento: %s", got)
	}
	n.TipoProposto = db.NullTipoComponente{TipoComponente: db.TipoComponenteFinito, Valid: true}
	if got := tipoVoluto(n, 0); got != db.TipoComponenteSciolto {
		t.Errorf("un nodo sotto un padre non nasce prodotto: %s", got)
	}
	n.TipoProposto = db.NullTipoComponente{TipoComponente: db.TipoComponenteSottoassieme, Valid: true}
	if got := tipoVoluto(n, 0); got != db.TipoComponenteSottoassieme {
		t.Errorf("il suggerimento dello STEP: %s", got)
	}
}

// Un nodo senza codice a cui l'operatore scrive il codice di un assieme che c'e' gia' (sotto un altro prodotto):
// il nodo ritrova l'assieme, e i figli dell'assieme — che l'editor non mostrava sotto quella carta — restano.
func TestPianificaTieneIFigliDiUnComponenteRitrovatoPerCodice(t *testing.T) {
	b := nuovoBancoVoluta()
	altro := componente("52999999", db.TipoComponenteFinito, false)
	b.comp = append(b.comp, altro)
	b.rel = []db.ComponenteRelazione{
		{PadreID: altro.ComponenteID, FiglioID: b.assieme.ComponenteID, Qta: 2},
		{PadreID: b.assieme.ComponenteID, FiglioID: b.particolare.ComponenteID, Qta: 4},
	}
	v := StrutturaVoluta{Radice: b.prodotto.ComponenteID,
		Archi:  []ArcoVoluto{{Padre: c(b.prodotto), Figlio: p(b.senzaCodice), Qta: 1}},
		Visti:  []ArcoVoluto{{Padre: c(altro), Figlio: c(b.assieme), Qta: 2}, {Padre: c(b.assieme), Figlio: c(b.particolare), Qta: 4}},
		Codici: map[string]CodiceScritto{b.senzaCodice.PropostaID.String(): {Codice: "52920517"}}}
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Ritrovati[b.senzaCodice.PropostaID] != b.assieme.ComponenteID {
		t.Fatalf("il codice scritto ritrova l'assieme: %v", pv.Ritrovati)
	}
	tenuto := false
	for _, a := range pv.Archi {
		tenuto = tenuto || (a.Padre == chiaveComponente(b.assieme.ComponenteID) && a.Figlio == chiaveComponente(b.particolare.ComponenteID) && a.Qta == 4)
	}
	if !tenuto || pv.Tenuti != 1 || !pv.Albero[chiaveComponente(b.particolare.ComponenteID)] {
		t.Errorf("l'arco assieme → particolare resta, con la sua quantita': %+v, tenuti %d", pv.Archi, pv.Tenuti)
	}
	// l'assieme non e' disegnato (ci si arriva dal codice scritto): le proposte e le rimozioni sotto di lui non si
	// decidono da qui; la carta del nodo si'
	if pv.Disegnati[b.assieme.ComponenteID] || !pv.Disegnati[b.prodotto.ComponenteID] || !pv.Carte[b.senzaCodice.PropostaID] ||
		!pv.Tenute[Arco{Padre: b.assieme.ComponenteID, Figlio: b.particolare.ComponenteID}] {
		t.Errorf("disegnati %v, carte %v, tenute %v", pv.Disegnati, pv.Carte, pv.Tenute)
	}
	// un ciclo passando per gli archi tenuti si vede lo stesso
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.particolare), Figlio: c(b.prodotto), Qta: 1})
	rifiutata(t, b.contesto(), v, "è la radice della struttura")
}

// La struttura disegnata su dati vecchi: un arco che l'editor mostrava e che nel frattempo e' stato tolto (e la
// struttura lo vuole), o che ha cambiato quantita'; una proposta dello STEP mostrata e decisa nel frattempo.
func TestPianificaRifiutaIDatiCambiatiNelFrattempo(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	for i := range v.Visti {
		if v.Visti[i].Figlio == c(b.particolare) && v.Visti[i].Padre == c(b.assieme) {
			v.Visti[i].Qta = 3 // l'editor mostrava ×3: adesso e' ×4
		}
	}
	rifiutata(t, b.contesto(), v, "è diventato ×4 (l'editor mostrava ×3): riapri l'editor")

	// l'editor non dice la quantita' (0): non si confronta
	for i := range v.Visti {
		v.Visti[i].Qta = 0
	}
	if _, err := pianifica(b.contesto(), v); err != nil {
		t.Errorf("visti senza quantita': %v", err)
	}

	// tolto nel frattempo: la struttura lo vuole ancora → rifiuto; non lo vuole piu' → va bene
	cx := b.contesto()
	cx.Attivi = cx.Attivi[:2] // P→A, A→X: P→X non c'e' piu'
	rifiutata(t, cx, b.comeE(), "nel frattempo è stato tolto")
	if _, err := pianifica(cx, senzaArco(b.comeE(), b.prodotto, b.particolare)); err != nil {
		t.Errorf("tolto da tutti e due: %v", err)
	}

	// una proposta di arco mostrata e decisa nel frattempo
	cx = b.contesto()
	all := uuid.New()
	cx.Relazioni = map[ChiaveRelazione]db.RelazioneProposta{
		{Allegato: all, Padre: "#2", Figlio: "#3"}: {AllegatoID: all, PadreChiave: "#2", FiglioChiave: "#3", Stato: db.StatoPropostaScartata},
	}
	v = b.comeE()
	v.RelazioniViste = []RelazioneVista{{Allegato: all, Padre: "#2", Figlio: "#3"}}
	rifiutata(t, cx, v, "è stata decisa nel frattempo: riapri l'editor")
}

func senzaArco(v StrutturaVoluta, padre, figlio db.Componente) StrutturaVoluta {
	var out []ArcoVoluto
	for _, a := range v.Archi {
		if a.Padre != c(padre) || a.Figlio != c(figlio) {
			out = append(out, a)
		}
	}
	v.Archi = out
	return v
}

// Una carta dell'editor sono tutte le proposte aperte con lo stesso codice (lo stesso nodo in due STEP): «e' il
// prodotto» e «scarta» valgono per tutte.
func TestPianificaUnaCartaSonoTutteLeProposteDelCodice(t *testing.T) {
	b := nuovoBancoVoluta()
	radice2 := propostaVoluta("#1", "PRT-0001", db.StatoPropostaAperta)
	nuovo2 := propostaVoluta("#7", "53011111", db.StatoPropostaAperta)
	cx := nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{b.nuovo, b.radice, radice2, nuovo2, b.senzaCodice}, nil)
	v := b.comeE()
	v.RadiciProposte = []uuid.UUID{b.radice.PropostaID}
	v.Scarta = []uuid.UUID{b.nuovo.PropostaID}
	pv, err := pianifica(cx, v)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Radici) != 2 || !contieneID(pv.Radici, radice2.PropostaID) {
		t.Errorf("le radici: %v", pv.Radici)
	}
	if len(pv.Scarta) != 2 || !contieneID(pv.Scarta, nuovo2.PropostaID) {
		t.Errorf("gli scarti: %v", pv.Scarta)
	}
	// un nodo senza codice non ha una carta comune con nessuno
	v = b.comeE()
	v.Scarta = []uuid.UUID{b.senzaCodice.PropostaID}
	if pv, err := pianifica(cx, v); err != nil || len(pv.Scarta) != 1 {
		t.Errorf("lo scarto di un nodo senza codice: %v %v", pv.Scarta, err)
	}
}

// Un arco proposto che l'editor ha mostrato verso un nodo aperto con il codice di un componente che c'e':
// l'editor lo disegna come quel componente, e il nodo si ritrova (non resta aperto per sempre).
func TestPianificaRitrovaINodiDegliArchiMostrati(t *testing.T) {
	b := nuovoBancoVoluta()
	aperto := propostaVoluta("#2", "52920517", db.StatoPropostaAperta)
	cx := nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{aperto, b.nuovo}, nil)
	v := b.comeE()
	v.RelazioniViste = []RelazioneVista{{Allegato: aperto.AllegatoID, Padre: "#2", Figlio: "#9"}}
	pv, err := pianifica(cx, v)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Ritrovati[aperto.PropostaID] != b.assieme.ComponenteID {
		t.Errorf("il nodo 52920517 dell'arco mostrato si ritrova nell'assieme: %v", pv.Ritrovati)
	}
}
