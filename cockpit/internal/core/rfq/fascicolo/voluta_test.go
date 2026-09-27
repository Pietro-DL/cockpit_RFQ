package fascicolo

// L1 — Fascicolo v3: la struttura voluta dall'editor, controllata da pianifica senza scrivere niente.
// Riferimenti (c:, p:, n:; k: rifiutato), codici, quantita', archi ripetuti, la radice, l'albero, gli archi
// che l'editor non mostrava, i cicli, gli scarti. Le prove L4 che la applicano davvero stanno in
// web/v3_db_test.go.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// bancoVoluta: il prodotto 77722757 → l'assieme 77720517 ×2 → il particolare 77817189 ×4, e il particolare
// anche sotto il prodotto ×1 (condiviso). Uno STEP propone 77811111 (nuovo), 77720517 (c'e' gia'), un nodo
// senza codice, una radice con un altro nome (un codice interno del CAD), un nodo gia' scartato. 77750000 e'
// un codice della richiesta.
type bancoVoluta struct {
	prodotto, assieme, particolare             db.Componente
	nuovo, ritrovato, senzaCodice, radice, via db.ComponenteProposta
	comp                                       []db.Componente
	rel                                        []db.ComponenteRelazione
	richiesta                                  []string
}

func propostaVoluta(chiave, codice string, stato db.StatoProposta) db.ComponenteProposta {
	return db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: uuid.New(), Chiave: chiave, NomeGrezzo: "nodo " + chiave,
		Codice: pgtype.Text{String: codice, Valid: codice != ""}, Stato: stato}
}

func nuovoBancoVoluta() *bancoVoluta {
	b := &bancoVoluta{
		prodotto:    componente("77722757", db.TipoComponenteFinito, false),
		assieme:     componente("77720517", db.TipoComponenteSottoassieme, false),
		particolare: componente("77817189", db.TipoComponenteSciolto, false),
		nuovo:       propostaVoluta("#3", "77811111", db.StatoPropostaAperta),
		ritrovato:   propostaVoluta("#2", "77720517", db.StatoPropostaAperta),
		senzaCodice: propostaVoluta("#4", "", db.StatoPropostaAperta),
		radice:      propostaVoluta("#1", "PRT-0001", db.StatoPropostaAperta),
		via:         propostaVoluta("#5", "77855555", db.StatoPropostaScartata),
		richiesta:   []string{"77750000"},
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
	return nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{b.nuovo, b.ritrovato, b.senzaCodice, b.radice, b.via}, b.richiesta)
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

// La struttura com'e', piu' il nodo nuovo sotto l'assieme, il nodo proposto che c'e' gia' e un componente
// scritto dall'operatore: il nuovo nasce da quella proposta, l'altro si ritrova nel componente con il suo
// codice, lo scritto nasce da se' con il suo tipo e la sua revisione, l'albero e' tutto.
//
// Riscritta per lo Smistamento (R1, U4, fase F2): prima il terzo pezzo era la carta «k:77888888», un codice
// trovato nella RFQ che nasceva con la revisione delle sue evidenze. Adesso e' la carta «n:77888888», con il
// codice scritto dall'operatore (StrutturaVoluta.Nuovi).
func TestPianificaLaStrutturaConINodiProposti(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: p(b.nuovo), Qta: 2},
		ArcoVoluto{Padre: p(b.ritrovato), Figlio: "n:77888888", Qta: 3})
	v.Nuovi = []ComponenteNuovo{{Codice: "77888888", Tipo: db.TipoComponenteSciolto, Rev: "b"}}
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	if ids := pv.Nuovi["n:77811111"]; len(ids) != 1 || ids[0] != b.nuovo.PropostaID {
		t.Errorf("77811111 nasce dalla sua proposta: %v", pv.Nuovi)
	}
	if pv.Ritrovati[b.ritrovato.PropostaID] != b.assieme.ComponenteID {
		t.Errorf("il nodo 77720517 si ritrova nell'assieme: %v", pv.Ritrovati)
	}
	if got := pv.Scritti["n:77888888"]; got.Codice != "77888888" || got.Rev != "B" || got.Tipo != db.TipoComponenteSciolto || pv.Nuovi["n:77888888"] != nil {
		t.Errorf("il componente scritto nasce da se', con il tipo e la revisione scritti: %+v %v", got, pv.Nuovi["n:77888888"])
	}
	for _, k := range []chiaveNodo{chiaveComponente(b.prodotto.ComponenteID), chiaveComponente(b.assieme.ComponenteID),
		chiaveComponente(b.particolare.ComponenteID), "n:77811111", "n:77888888"} {
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
	a := componente("77722757", db.TipoComponenteFinito, true)
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
	gemello := propostaVoluta("#9", "77811111", db.StatoPropostaAperta)
	cx := nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{b.nuovo, gemello}, nil)
	v = b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: p(b.nuovo), Qta: 1}, ArcoVoluto{Padre: p(b.nuovo), Figlio: p(gemello), Qta: 1})
	pv, err := pianifica(cx, v)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Archi) != 4 || len(pv.Nuovi["n:77811111"]) != 2 {
		t.Errorf("l'arco fra i due gemelli si salta, tutti e due portano 77811111: %d archi, %v", len(pv.Archi), pv.Nuovi)
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
	rifiutata(t, b.contesto(), v, "non è appeso alla struttura di 77722757")

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

	// troppi archi visti, con pochi archi voluti: il rifiuto dice gli archi visti e il loro limite, non
	// «la struttura ha 3 archi: il limite è 5000», che non spiega niente
	v = b.comeE()
	for len(v.Visti) <= 4*MaxArchiVoluti {
		v.Visti = append(v.Visti, v.Visti[0])
	}
	rifiutata(t, b.contesto(), v, fmt.Sprintf("%d archi della BOM mostrata: il limite è %d", len(v.Visti), 4*MaxArchiVoluti))
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
	if err == nil || !strings.Contains(err.Error(), "77811111") {
		t.Errorf("il ciclo con il nodo nuovo: %v", err)
	}

	// un altro prodotto con l'assieme sotto: portato nell'albero, porta i suoi archi. Se l'editor non li
	// mostrava la struttura e' vecchia; se li mostrava e li tiene, il giro si chiude
	altro := componente("77799999", db.TipoComponenteFinito, false)
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

// I riferimenti p:, n: e k:, e i codici scritti dall'operatore.
//
// Riscritta per lo Smistamento (R1, fase F2): prima un «k:» non trovato si rifiutava («non è fra i codici
// trovati») e un «k:» con il codice di un componente era quel componente. Adesso ogni «k:» si rifiuta, anche
// quello di un componente che c'e'; il componente che c'e' lo ritrova la carta «n:» con il codice scritto.
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
	rifiutata(t, b.contesto(), sotto("k:77877777"), "un codice trovato nella RFQ non diventa un componente")
	rifiutata(t, b.contesto(), sotto("k:77817189"), "un codice trovato nella RFQ non diventa un componente")
	rifiutata(t, b.contesto(), sotto("x:qualcosa"), "non è valido")

	// un codice scritto che e' gia' un componente e' quel componente
	v := b.comeE()
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.prodotto), Figlio: "n:77720517", Qta: 7})
	v.Nuovi = []ComponenteNuovo{{Codice: "77720517", Tipo: db.TipoComponenteSciolto}}
	rifiutata(t, b.contesto(), v, "è due volte sotto")

	// il codice scritto dall'operatore fa del nodo senza codice un codice nuovo
	v = sotto(p(b.senzaCodice))
	v.Codici = map[string]CodiceScritto{b.senzaCodice.PropostaID.String(): {Codice: "77866666", Rev: "a"}}
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	if ids := pv.Nuovi["n:77866666"]; len(ids) != 1 || ids[0] != b.senzaCodice.PropostaID {
		t.Errorf("il nodo senza codice nasce con il codice scritto: %v", pv.Nuovi)
	}
	if pv.Codici[b.senzaCodice.PropostaID].Rev != "A" {
		t.Errorf("la revisione scritta, in maiuscolo: %+v", pv.Codici)
	}
	// codici non ammessi, su nodi non aperti, su nodi di altre RFQ
	for _, cs := range []CodiceScritto{{Codice: "53 06/6666"}, {Codice: strings.Repeat("9", 80)}, {Codice: "77866666", Rev: "REV MOLTO LUNGA"}} {
		v.Codici = map[string]CodiceScritto{b.senzaCodice.PropostaID.String(): cs}
		rifiutata(t, b.contesto(), v, "caratteri non ammessi")
	}
	v.Codici = map[string]CodiceScritto{b.via.PropostaID.String(): {Codice: "77866666"}}
	rifiutata(t, b.contesto(), v, "la proposta è già decisa")
	v.Codici = map[string]CodiceScritto{uuid.NewString(): {Codice: "77866666"}}
	rifiutata(t, b.contesto(), v, "non è di questa RFQ")
	v.Codici = map[string]CodiceScritto{"non-un-uuid": {Codice: "77866666"}}
	rifiutata(t, b.contesto(), v, "non valido")
}

// «E' il prodotto» non c'e' piu': una struttura che lo manda si rifiuta, qualunque nodo nomini; senza, la
// radice dello STEP resta un nodo come gli altri e i suoi archi restano suoi.
//
// Riscritta per lo Smistamento (R5, fase F2): prima era TestPianificaLaRadiceDelloStepEIlProdotto e fissava
// che la radice di uno STEP qualunque, detta «il prodotto», si ritrovasse nel prodotto (i suoi archi
// diventavano archi del prodotto, la radice una volta sola in pv.Radici; una proposta gia' decisa si
// rifiutava). Era una decisione sulla struttura presa senza decidere il file: la radice la fissa lo STEP
// strutturale.
func TestPianificaRifiutaEIlProdotto(t *testing.T) {
	b := nuovoBancoVoluta()
	v := b.comeE()
	v.RadiciProposte = []uuid.UUID{b.radice.PropostaID, b.radice.PropostaID}
	v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.prodotto), Figlio: p(b.radice), Qta: 1}, ArcoVoluto{Padre: p(b.radice), Figlio: p(b.nuovo), Qta: 2})
	rifiutata(t, b.contesto(), v, "«È il prodotto» non c'è più")
	v.RadiciProposte = []uuid.UUID{b.via.PropostaID}
	rifiutata(t, b.contesto(), v, "«È il prodotto» non c'è più")

	v.RadiciProposte = nil
	pv, err := pianifica(b.contesto(), v)
	if err != nil {
		t.Fatal(err)
	}
	ultimo := pv.Archi[len(pv.Archi)-1]
	if ultimo.Padre != "n:PRT-0001" || ultimo.Figlio != "n:77811111" {
		t.Errorf("l'arco della radice dello STEP resta della radice: %+v", ultimo)
	}
	if ids := pv.Nuovi["n:PRT-0001"]; len(ids) != 1 || ids[0] != b.radice.PropostaID {
		t.Errorf("la radice e' un nodo nuovo come gli altri, non il prodotto: %v", pv.Nuovi)
	}
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

	// riscritta per lo Smistamento (R5, fase F2): prima «e' il prodotto e fra gli scartati»; adesso «E' il
	// prodotto» si rifiuta da se'
	v = b.comeE()
	v.RadiciProposte = []uuid.UUID{b.radice.PropostaID}
	v.Scarta = []uuid.UUID{b.radice.PropostaID}
	rifiutata(t, b.contesto(), v, "«È il prodotto» non c'è più")

	v = b.comeE()
	v.Scarta = []uuid.UUID{b.via.PropostaID}
	rifiutata(t, b.contesto(), v, "già decisa")
	v.Scarta = []uuid.UUID{uuid.New()}
	rifiutata(t, b.contesto(), v, "non è di questa RFQ")
}

// La frase all'operatore dice che cosa e' cambiato, e chi e' rimasto senza padre.
func TestFraseVoluta(t *testing.T) {
	if got := fraseVoluta("77722757", esitoVoluta{}); got != "Nessun cambiamento: la struttura di 77722757 è già così." {
		t.Errorf("niente: %q", got)
	}
	got := fraseVoluta("77722757", esitoVoluta{nuovi: 1, aggiunti: 2, tolti: 1, chiuse: 1, senzaPadre: []string{"77817189"}})
	if got != "Struttura di 77722757 confermata: 1 componente nuovo, 2 legami aggiunti, 1 legame tolto, 1 proposta di legame chiusa. Fuori dalla struttura e senza padre, da sistemare: 77817189." {
		t.Errorf("frase: %q", got)
	}
}

// Il tipo con cui nasce un nodo: assieme con dei figli, altrimenti quello suggerito (mai prodotto).
func TestTipoVoluto(t *testing.T) {
	n := propostaVoluta("#3", "77811111", db.StatoPropostaAperta)
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
	altro := componente("77799999", db.TipoComponenteFinito, false)
	b.comp = append(b.comp, altro)
	b.rel = []db.ComponenteRelazione{
		{PadreID: altro.ComponenteID, FiglioID: b.assieme.ComponenteID, Qta: 2},
		{PadreID: b.assieme.ComponenteID, FiglioID: b.particolare.ComponenteID, Qta: 4},
	}
	v := StrutturaVoluta{Radice: b.prodotto.ComponenteID,
		Archi:  []ArcoVoluto{{Padre: c(b.prodotto), Figlio: p(b.senzaCodice), Qta: 1}},
		Visti:  []ArcoVoluto{{Padre: c(altro), Figlio: c(b.assieme), Qta: 2}, {Padre: c(b.assieme), Figlio: c(b.particolare), Qta: 4}},
		Codici: map[string]CodiceScritto{b.senzaCodice.PropostaID.String(): {Codice: "77720517"}}}
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

// Una carta dell'editor sono tutte le proposte aperte con lo stesso codice (lo stesso nodo in due STEP):
// «scarta» vale per tutte.
//
// Riscritta per lo Smistamento (R5, fase F2): prima valeva per tutte anche «e' il prodotto» (le due radici
// PRT-0001 in pv.Radici). Il gesto e' tolto: la prova fissa lo scarto della carta PRT-0001, che prende
// tutte e due le radici.
func TestPianificaUnaCartaSonoTutteLeProposteDelCodice(t *testing.T) {
	b := nuovoBancoVoluta()
	radice2 := propostaVoluta("#1", "PRT-0001", db.StatoPropostaAperta)
	nuovo2 := propostaVoluta("#7", "77811111", db.StatoPropostaAperta)
	cx := nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{b.nuovo, b.radice, radice2, nuovo2, b.senzaCodice}, nil)
	v := b.comeE()
	v.Scarta = []uuid.UUID{b.radice.PropostaID, b.nuovo.PropostaID}
	pv, err := pianifica(cx, v)
	if err != nil {
		t.Fatal(err)
	}
	if !contieneID(pv.Scarta, b.radice.PropostaID) || !contieneID(pv.Scarta, radice2.PropostaID) {
		t.Errorf("le radici PRT-0001 si scartano tutte e due: %v", pv.Scarta)
	}
	if len(pv.Scarta) != 4 || !contieneID(pv.Scarta, nuovo2.PropostaID) {
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
	aperto := propostaVoluta("#2", "77720517", db.StatoPropostaAperta)
	cx := nuovoContestoVoluta(b.comp, b.rel, []db.ComponenteProposta{aperto, b.nuovo}, nil)
	v := b.comeE()
	v.RelazioniViste = []RelazioneVista{{Allegato: aperto.AllegatoID, Padre: "#2", Figlio: "#9"}}
	pv, err := pianifica(cx, v)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Ritrovati[aperto.PropostaID] != b.assieme.ComponenteID {
		t.Errorf("il nodo 77720517 dell'arco mostrato si ritrova nell'assieme: %v", pv.Ritrovati)
	}
}

// Prova 107 (parte pura, Smistamento U4): la carta «n:» dell'editor. Il codice e' scritto, il tipo e'
// assieme o particolare; se la RFQ ha gia' quel codice la carta e' quel componente (nessuna riga nuova), se
// e' archiviato si ripristina prima; un codice della richiesta non diventa un componente da qui; uno quasi
// uguale (P4) vuole la conferma che e' un pezzo diverso, e con la conferma nasce.
func TestPianificaLaCartaDelComponenteScritto(t *testing.T) {
	b := nuovoBancoVoluta()
	archiviato := componente("77731111", db.TipoComponenteSciolto, true)
	b.comp = append(b.comp, archiviato)
	sotto := func(codice string, n ...ComponenteNuovo) StrutturaVoluta {
		v := b.comeE()
		v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: "n:" + codice, Qta: 2})
		v.Nuovi = n
		return v
	}
	nuovo := func(codice string) ComponenteNuovo {
		return ComponenteNuovo{Codice: codice, Tipo: db.TipoComponenteSciolto}
	}

	// il codice si scrive: senza la sua carta, con un tipo che non si crea qui, non ammesso, due volte
	rifiutata(t, b.contesto(), sotto("77899999"), "non ha il suo codice scritto")
	rifiutata(t, b.contesto(), sotto("77899999", ComponenteNuovo{Codice: "77899999", Tipo: db.TipoComponenteFinito}), "è un assieme o un particolare")
	rifiutata(t, b.contesto(), sotto("77899999", ComponenteNuovo{Codice: "77899999", Tipo: db.TipoComponenteCommerciale}), "è un assieme o un particolare")
	rifiutata(t, b.contesto(), sotto("77899999", ComponenteNuovo{Codice: "778 99999", Tipo: db.TipoComponenteSciolto}), "caratteri non ammessi")
	rifiutata(t, b.contesto(), sotto("77899999", nuovo("77899999"), nuovo("77899999")), "è scritto due volte")

	// un codice nuovo e lontano da tutti nasce, con il tipo scelto
	pv, err := pianifica(b.contesto(), sotto("77899999", nuovo("77899999")))
	if err != nil {
		t.Fatal(err)
	}
	if n := pv.Scritti["n:77899999"]; n.Codice != "77899999" || n.Tipo != db.TipoComponenteSciolto || !pv.Albero["n:77899999"] || len(pv.Nuovi["n:77899999"]) != 0 {
		t.Errorf("77899999 nasce dalla carta scritta: %+v, albero %v, proposte %v", n, pv.Albero["n:77899999"], pv.Nuovi["n:77899999"])
	}

	// un codice che c'e' e' quel componente: nessuna riga nuova, un arco in piu'
	pv, err = pianifica(b.contesto(), func() StrutturaVoluta {
		v := b.comeE()
		v.Archi = append(v.Archi, ArcoVoluto{Padre: c(b.assieme), Figlio: "n:77817189", Qta: 1})
		v.Archi = senzaArco(v, b.assieme, b.particolare).Archi
		v.Nuovi = []ComponenteNuovo{nuovo("77817189")}
		return v
	}())
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Scritti) != 0 || len(pv.Nuovi) != 0 || !pv.Esistenti[b.particolare.ComponenteID] {
		t.Errorf("77817189 c'e' gia': nessun componente nuovo, e' quello: scritti %v, nuovi %v, esistenti %v", pv.Scritti, pv.Nuovi, pv.Esistenti)
	}
	rifiutata(t, b.contesto(), sotto("77731111", nuovo("77731111")), "c'è già ed è archiviato")
	rifiutata(t, b.contesto(), sotto("77750000", nuovo("77750000")), "è un codice della richiesta")

	// quasi uguale: senza conferma si rifiuta con i vicini, con la conferma nasce. 77720518 e' un altro numero
	rifiutata(t, b.contesto(), sotto("77720517A", nuovo("77720517A")), "77720517A è quasi uguale a 77720517 (lettera finale)")
	rifiutata(t, b.contesto(), sotto("77750000_F2", nuovo("77750000_F2")), "quasi uguale a 77750000")
	diverso := nuovo("77720517A")
	diverso.Diverso = true
	if pv, err := pianifica(b.contesto(), sotto("77720517A", diverso)); err != nil || pv.Scritti["n:77720517A"].Codice != "77720517A" {
		t.Errorf("con la conferma che e' un pezzo diverso nasce: %v %+v", err, pv.Scritti)
	}
	if _, err := pianifica(b.contesto(), sotto("77720518", nuovo("77720518"))); err != nil {
		t.Errorf("77720518 non e' vicino di 77720517: %v", err)
	}

	// le maiuscole non fanno un pezzo nuovo: «7120001a» scritto in minuscolo e' il 7120001A che la RFQ ha gia'
	// (l'identita' e' (thread, upper(codice)))
	lettera := componente("7120001A", db.TipoComponenteSciolto, false)
	b.comp = append(b.comp, lettera)
	pv, err = pianifica(b.contesto(), sotto("7120001a", nuovo("7120001a")))
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Scritti) != 0 || len(pv.Nuovi) != 0 || !pv.Esistenti[lettera.ComponenteID] || !pv.Albero[chiaveComponente(lettera.ComponenteID)] {
		t.Errorf("7120001a e' 7120001A: scritti %v, nuovi %v, esistenti %v", pv.Scritti, pv.Nuovi, pv.Esistenti)
	}

	// un prodotto della RFQ non va sotto un altro prodotto da un codice scritto: lo ferma l'editor prima di
	// mettere la carta, e lo rifiuta il core se la carta arriva lo stesso
	altro := componente("77790001", db.TipoComponenteFinito, false)
	b.comp = append(b.comp, altro)
	rifiutata(t, b.contesto(), sotto("77790001", nuovo("77790001")), "77790001 è un prodotto della RFQ: si sceglie in alto, non si mette sotto un altro prodotto")
	rifiutata(t, b.contesto(), sotto("77722757", nuovo("77722757")), "77722757 è un prodotto della RFQ")
}
