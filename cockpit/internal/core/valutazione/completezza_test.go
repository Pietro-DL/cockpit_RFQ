// L1 — la completezza documentale sulla regola (B5, fase 3; R62, R72 D [R], R82, R93, R99 A, R102 A, R103 C; contratto §1.6,
// §2.3, §2.5, §2.6; T-E1-10, T-E1-17, T-E1-18, T-E1R-01; K-01 con la lettura A; PO-08 nella seconda fotografia, PO-28,
// PO-33, PO-36): i fabbisogni con gli invarianti, la riga esplicita del cliente, la minuteria confermata con
// ConfermaCategoria, la voce del 2D su tutto il gruppo (la deroga, lo scarto, il da_determinare, il formato non
// configurato), gli esiti della vista, il perimetro e lo stato, il determinismo, i valori e i campi del contratto. Gli
// ingressi sono astratti e sintetici: la minuteria confermata non c'è mai sui dati veri (LD-19).
package valutazione_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// ---- gli ingressi sintetici ----

// dueD: un 2D sintetico del gruppo di un componente, con il formato, la validità, la provenienza e se è corrente.
func dueD(s string, all uuid.UUID, f valutazione.Formato2D, v valutazione.ValiditaDisegno2D, o ancoraggio.OrigineDato, corrente bool) valutazione.Disegno2D {
	a := all
	nd := motorea.CompatibilitaNonDeterminabile
	d := valutazione.Disegno2D{Sha256: s, AllegatoID: &a, Formato: f, Validita: v, Provenienza: o, Corrente: corrente, CompatibilitaCodice: nd,
		CompatibilitaRevisione: nd, Cartiglio: valutazione.CartiglioNonLeggibile}
	switch v {
	case valutazione.ValiditaDaVerificare:
		d.MotivoValidita = valutazione.MotivoFabbisognoContenutoNonVerificabile
	case valutazione.ValiditaFormatoNonConfigurato:
		d.MotivoValidita = valutazione.MotivoFabbisognoFormatoNonConfigurato
	}
	return d
}

// gruppo: il gruppo dei 2D con il primario (Primario); nil senza 2D.
func gruppo(d ...valutazione.Disegno2D) *valutazione.GruppoDisegni2D {
	if len(d) == 0 {
		return nil
	}
	g := valutazione.Primario(d)
	return &g
}

// dueDValido: un PDF valido, corrente e confermato sul componente.
func dueDValido(s string, all uuid.UUID) valutazione.Disegno2D {
	return dueD(s, all, valutazione.Formato2DPDF, valutazione.ValiditaValido, ancoraggio.OrigineConfermato, true)
}

// fv: un fabbisogno di una riga com'è nella vista.
func fv(tipo string, bloccante bool, esito string) valutazione.FabbisognoDellaRiga {
	return valutazione.FabbisognoDellaRiga{TipoDocumento: tipo, Bloccante: bloccante, EsitoVista: esito, CalcolataDa: valutazione.CalcolataDaVista}
}

// rigaP: una riga certa del perimetro, con i fabbisogni della vista.
func rigaP(comp uuid.UUID, codice, tipo string, delProdotto bool, f ...valutazione.FabbisognoDellaRiga) valutazione.RigaDelPerimetro {
	return valutazione.RigaDelPerimetro{ComponenteID: comp, Codice: codice, TipoComponente: tipo, DelProdotto: delProdotto, Fabbisogni: f}
}

// finitoDefault: il finito con le righe di default della vista (cad_3d presente) e il 2D valido o nessun 2D.
func finitoDefault(con2D bool) valutazione.RigaDelPerimetro {
	r := rigaP(cProdotto, "7120100A", "finito", true, fv("cad_3d", true, "ok"), fv("disegno_2d", true, "ok"))
	if con2D {
		r.Disegno.Gruppo = gruppo(dueDValido(sha1, aPDF1))
	}
	return r
}

// scioltoDefault: lo sciolto con le righe di default della vista, e il 2D valido o nessun 2D.
func scioltoDefault(comp uuid.UUID, codice string, con2D bool) valutazione.RigaDelPerimetro {
	r := rigaP(comp, codice, "sciolto", false, fv("cad_3d", false, "manca"), fv("disegno_2d", true, "manca"), fv("sviluppo_dxf", false, "manca"))
	if con2D {
		r.Disegno.Gruppo = gruppo(dueDValido(sha2, aPDF2))
	}
	return r
}

// chiusa: la struttura di un prodotto con il perimetro chiuso: il componente, la fonte confermata, la BOM di lavoro,
// niente da decidere.
func chiusa() valutazione.StrutturaDaVerificare {
	return valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true}
}

// minuteriaConfermata: la ConfermaCategoria di un componente (sintetica: nessun adattatore la produce, LD-19).
func confermaCategoria(comp uuid.UUID, categoria string) valutazione.ConfermaCategoria {
	return valutazione.ConfermaCategoria{ComponenteID: comp, Categoria: categoria, Da: operatore, Il: dataACME.Add(3 * time.Hour)}
}

// ---- la classificazione e la minuteria (PO-28) ----

// TestPO28LaMinuteria (PO-28 per intero; R93, R103 C; T-E1-18, T-E1R-01; E1R §4.2): la minuteria confermata
// (ConfermaCategoria) su uno sciolto non ha la voce del 2D, il documento ricevuto resta nel suo gruppo, gli altri
// bloccanti del tipo restano; il finito con ConfermaCategoria minuteria chiede il 2D (R99 A); una minuteria solo
// proposta ha Proposta = minuteria e il 2D resta, fra le voci o fra i previsti; il commerciale confermato chiede il 2D con
// schema_senza_2d; il commerciale della famiglia minuteria ha Categoria = commerciale, Proposta = minuteria, e chiede il
// 2D; lo sciolto chiede il 2D.
func TestPO28LaMinuteria(t *testing.T) {
	minuteria := rigaP(cMinuteria, "7129001", "sciolto", false, fv("cad_3d", true, "ok"), fv("disegno_2d", true, "manca"))
	minuteria.RegolaCliente = true
	minuteria.Disegno.Gruppo = gruppo(dueD(sha3, aPDF3, valutazione.Formato2DPDF, valutazione.ValiditaValido, ancoraggio.OrigineProposto, false))
	prima := canonicoDi(t, minuteria)
	proposta := scioltoDefault(cSciolto, "7120200A", false)
	proposta.Categorie = []string{valutazione.CategoriaMinuteria}
	commerciale := rigaP(cCommerciale, "7120400A", "commerciale", false)
	comMinut := rigaP(cComMinut, "7129002", "commerciale", false)
	comMinut.Categorie = []string{valutazione.CategoriaMinuteria}
	in := valutazione.IngressoCompletezza{Struttura: chiusa(),
		Righe:    []valutazione.RigaDelPerimetro{finitoDefault(true), minuteria, proposta, commerciale, comMinut},
		Nodi:     []valutazione.NodoDaPrevedere{{Nodo: "nodo:minuteria", TipoComponente: "sciolto", Categorie: []string{valutazione.CategoriaMinuteria}, Regole: []valutazione.RegolaFabbisogno{{TipoDocumento: "disegno_2d", Bloccante: true}}, Motivo: valutazione.MotivoPrevistoNodoProposto}},
		Conferme: []valutazione.ConfermaCategoria{confermaCategoria(cMinuteria, valutazione.CategoriaMinuteria)}}
	d := valutazione.Completezza(in)

	t.Run("la minuteria confermata su uno sciolto: nessuna voce del 2D, gli altri bloccanti restano", func(t *testing.T) {
		if haVoce(d, cMinuteria, "disegno_2d") {
			t.Errorf("la minuteria confermata ha la voce del 2D: %s", vociDi(d))
		}
		if _, ok := nonBloccanteDi(d, cMinuteria, "disegno_2d"); ok {
			t.Error("il 2D della minuteria confermata non è nemmeno fra i non bloccanti")
		}
		if v := voceDi(t, d, cMinuteria, "cad_3d"); esitoDi(v) != "presente/" || v.Categoria != valutazione.CategoriaMinuteria || v.Invariante {
			t.Errorf("il cad_3d bloccante della regola resta: %+v", v)
		}
		if canonicoDi(t, minuteria) != prima || minuteria.Disegno.Gruppo.Primario == nil {
			t.Error("il gruppo dei 2D della minuteria (il documento ricevuto) non si tocca")
		}
	})
	t.Run("il finito con ConfermaCategoria minuteria chiede il 2D (R99 A)", func(t *testing.T) {
		in2 := in
		in2.Conferme = append(append([]valutazione.ConfermaCategoria(nil), in.Conferme...), confermaCategoria(cProdotto, valutazione.CategoriaMinuteria))
		d2 := valutazione.Completezza(in2)
		v := voceDi(t, d2, cProdotto, "disegno_2d")
		if !v.Invariante || !v.Bloccante || v.Categoria != valutazione.CategoriaMinuteria || esitoDi(v) != "presente/" {
			t.Errorf("il 2D del finito %+v", v)
		}
		c := valutazione.Classifica(valutazione.ComponenteDaClassificare{ComponenteID: &cProdotto, Tipo: "finito"}, in2.Conferme)
		if c.Ruolo != valutazione.RuoloProdotto || c.Categoria != valutazione.CategoriaMinuteria || !c.Confermata ||
			c.Motivo != valutazione.MotivoCategoriaFinitoSempre2D || valutazione.EsenteDal2D(c) {
			t.Errorf("classificazione del finito %+v", c)
		}
	})
	t.Run("una minuteria solo proposta non toglie niente", func(t *testing.T) {
		v := voceDi(t, d, cSciolto, "disegno_2d")
		if esitoDi(v) != "manca/nessun_documento" || v.Categoria != valutazione.CategoriaFabbricato || !v.Invariante {
			t.Errorf("il 2D dello sciolto della famiglia minuteria %+v", v)
		}
		c := valutazione.Classifica(valutazione.ComponenteDaClassificare{ComponenteID: &cSciolto, Tipo: "sciolto", Categorie: proposta.Categorie}, nil)
		if c.Proposta != valutazione.CategoriaMinuteria || c.Categoria != valutazione.CategoriaFabbricato || c.Confermata ||
			c.Origine != valutazione.OrigineCategoriaDerivata || valutazione.EsenteDal2D(c) {
			t.Errorf("classificazione %+v", c)
		}
		if p := previstoDi(t, d, "nodo:minuteria", "disegno_2d"); p.Motivo != valutazione.MotivoPrevistoNodoProposto {
			t.Errorf("il 2D del nodo della minuteria proposta resta fra i previsti: %+v", p)
		}
	})
	t.Run("il commerciale confermato chiede il 2D (R103 C)", func(t *testing.T) {
		v := voceDi(t, d, cCommerciale, "disegno_2d")
		if !v.Invariante || v.NotaRegola != valutazione.NotaSchemaSenza2D || v.Categoria != valutazione.CategoriaCommerciale ||
			v.CalcolataDa != valutazione.CalcolataDaGo || esitoDi(v) != "manca/nessun_documento" || v.RegolaCliente {
			t.Errorf("il 2D del commerciale %+v", v)
		}
		for _, x := range d.Voci {
			if x.ComponenteID == cCommerciale && x.TipoDocumento != "disegno_2d" {
				t.Errorf("il commerciale senza righe ha solo il 2D: %+v", x)
			}
		}
	})
	t.Run("il commerciale della famiglia minuteria chiede il 2D", func(t *testing.T) {
		v := voceDi(t, d, cComMinut, "disegno_2d")
		if v.Categoria != valutazione.CategoriaCommerciale || v.NotaRegola != valutazione.NotaSchemaSenza2D {
			t.Errorf("voce %+v", v)
		}
		c := valutazione.Classifica(valutazione.ComponenteDaClassificare{ComponenteID: &cComMinut, Tipo: "commerciale", Categorie: comMinut.Categorie}, nil)
		if c.Categoria != valutazione.CategoriaCommerciale || c.Proposta != valutazione.CategoriaMinuteria || !c.Confermata ||
			c.Origine != valutazione.OrigineCategoriaConfermata || valutazione.EsenteDal2D(c) {
			t.Errorf("classificazione %+v", c)
		}
	})
	t.Run("lo sciolto chiede il 2D", func(t *testing.T) {
		v := voceDi(t, valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa(),
			Righe: []valutazione.RigaDelPerimetro{finitoDefault(true), scioltoDefault(cSciolto, "7120200A", true)}}), cSciolto, "disegno_2d")
		if esitoDi(v) != "presente/" || !v.Invariante || v.NotaRegola != "" {
			t.Errorf("voce %+v", v)
		}
	})
	if d.Stato != valutazione.DocumentiIncompleta {
		t.Errorf("stato %s: il commerciale e lo sciolto senza 2D mancano con certezza", statoDi(d))
	}
}

// TestLaClassificazioneSullaRegola (T-E1-18, T-E1R-01; emendamento E1 §6.3, E1R §4.2): il ruolo e la categoria dal tipo,
// la conferma più recente, una conferma fuori elenco che non conta, un nodo senza tipo deciso, l'esenzione solo per la
// minuteria confermata che non è il finito.
func TestLaClassificazioneSullaRegola(t *testing.T) {
	k := uid(0x7c9)
	for _, c := range []struct {
		nome     string
		in       valutazione.ComponenteDaClassificare
		conferme []valutazione.ConfermaCategoria
		atteso   valutazione.Classificazione
		esente   bool
	}{
		{"il finito", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "finito"}, nil,
			valutazione.Classificazione{Ruolo: "prodotto", Categoria: "fabbricato", Origine: "derivato_dal_tipo"}, false},
		{"il sottoassieme", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sottoassieme"}, nil,
			valutazione.Classificazione{Ruolo: "componente", Categoria: "fabbricato", Origine: "derivato_dal_tipo"}, false},
		{"il commerciale", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "commerciale"}, nil,
			valutazione.Classificazione{Ruolo: "componente", Categoria: "commerciale", Origine: "confermato", Confermata: true}, false},
		{"un nodo senza tipo deciso", valutazione.ComponenteDaClassificare{}, nil,
			valutazione.Classificazione{Ruolo: "componente", Categoria: "non_determinata", Origine: "proposto", Motivo: "tipo_non_determinato"}, false},
		{"un nodo della famiglia minuteria", valutazione.ComponenteDaClassificare{Categorie: []string{"minuteria"}}, nil,
			valutazione.Classificazione{Ruolo: "componente", Categoria: "non_determinata", Origine: "proposto", Motivo: "tipo_non_determinato", Proposta: "minuteria"}, false},
		{"la minuteria confermata su un sottoassieme", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sottoassieme", Categorie: []string{"minuteria"}},
			[]valutazione.ConfermaCategoria{confermaCategoria(k, "minuteria")},
			valutazione.Classificazione{Ruolo: "componente", Categoria: "minuteria", Origine: "confermato", Confermata: true}, true},
		{"la conferma più recente vale", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sciolto"},
			[]valutazione.ConfermaCategoria{confermaCategoria(k, "minuteria"), {ComponenteID: k, Categoria: "fabbricato", Da: operatore, Il: dataACME.Add(5 * time.Hour)}},
			valutazione.Classificazione{Ruolo: "componente", Categoria: "fabbricato", Origine: "confermato", Confermata: true}, false},
		{"una conferma fuori elenco non conta", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sciolto"},
			[]valutazione.ConfermaCategoria{confermaCategoria(k, "vite")},
			valutazione.Classificazione{Ruolo: "componente", Categoria: "fabbricato", Origine: "derivato_dal_tipo", Motivo: "conferma_non_ammessa"}, false},
		{"la conferma di un altro componente non conta", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sciolto"},
			[]valutazione.ConfermaCategoria{confermaCategoria(cSciolto, "minuteria")},
			valutazione.Classificazione{Ruolo: "componente", Categoria: "fabbricato", Origine: "derivato_dal_tipo"}, false},
		{"il componente del prodotto è prodotto, qualunque sia il tipo (R-23)", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sciolto", DelProdotto: true}, nil,
			valutazione.Classificazione{Ruolo: "prodotto", Categoria: "fabbricato", Origine: "derivato_dal_tipo"}, false},
		{"il finito minuteria confermata non è esente", valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "finito"},
			[]valutazione.ConfermaCategoria{confermaCategoria(k, "minuteria")},
			valutazione.Classificazione{Ruolo: "prodotto", Categoria: "minuteria", Origine: "confermato", Confermata: true, Motivo: "finito_chiede_sempre_il_2d"}, false},
	} {
		got := valutazione.Classifica(c.in, c.conferme)
		if !reflect.DeepEqual(got, c.atteso) || valutazione.EsenteDal2D(got) != c.esente {
			t.Errorf("%s: %+v (esente %v), atteso %+v (esente %v)", c.nome, got, valutazione.EsenteDal2D(got), c.atteso, c.esente)
		}
	}
	// EsenteDal2D sulla regola: solo la minuteria confermata che non è il prodotto.
	for _, c := range []struct {
		cl     valutazione.Classificazione
		esente bool
	}{
		{valutazione.Classificazione{Ruolo: "componente", Categoria: "minuteria", Confermata: true}, true},
		{valutazione.Classificazione{Ruolo: "componente", Categoria: "minuteria"}, false},
		{valutazione.Classificazione{Ruolo: "prodotto", Categoria: "minuteria", Confermata: true}, false},
		{valutazione.Classificazione{Ruolo: "componente", Categoria: "commerciale", Confermata: true, Proposta: "minuteria"}, false},
	} {
		if valutazione.EsenteDal2D(c.cl) != c.esente {
			t.Errorf("EsenteDal2D(%+v) = %v", c.cl, !c.esente)
		}
	}
	// A parità di «il», la conferma con il «da» minore; poi la categoria: l'ordine delle conferme non conta.
	a := valutazione.ConfermaCategoria{ComponenteID: k, Categoria: "minuteria", Da: uid(0xa2), Il: dataACME}
	b := valutazione.ConfermaCategoria{ComponenteID: k, Categoria: "commerciale", Da: uid(0xa1), Il: dataACME}
	in := valutazione.ComponenteDaClassificare{ComponenteID: &k, Tipo: "sciolto"}
	if x, y := valutazione.Classifica(in, []valutazione.ConfermaCategoria{a, b}), valutazione.Classifica(in, []valutazione.ConfermaCategoria{b, a}); x != y || x.Categoria != "commerciale" {
		t.Errorf("a parità: %+v e %+v", x, y)
	}
	c := valutazione.ConfermaCategoria{ComponenteID: k, Categoria: "fabbricato", Da: uid(0xa1), Il: dataACME}
	if x, y := valutazione.Classifica(in, []valutazione.ConfermaCategoria{b, c}), valutazione.Classifica(in, []valutazione.ConfermaCategoria{c, b}); x != y || x.Categoria != "commerciale" {
		t.Errorf("a parità di «il» e di «da»: %+v e %+v", x, y)
	}
}

// ---- le regole del cliente e K-01 (PO-33) ----

// TestPO33LaRegolaDelCliente (PO-33 sulla regola; R99 A, R103 C; T-E1-17, T-E1R-01; E1R §4.2): una regola del cliente sui
// finiti senza disegno_2d e senza cad_3d: il 2D richiesto e bloccante con regola_cliente_senza_2d, e nessuna voce dello
// STEP (resta l'asse della fonte); la riga esplicita sui finiti con il 2D non bloccante: resta bloccante,
// regola_cliente_2d_non_bloccante; sugli sciolti e sui commerciali senza disegno_2d: richiesto, regola_cliente_senza_2d; la
// riga esplicita sugli sciolti o sui commerciali con il 2D non bloccante: con la lettura A di K-01 fra i non bloccanti,
// con la nota. Le regole di default: il 2D senza nota, il commerciale con schema_senza_2d; una riga di default non
// bloccante sul 2D, che lo schema non ha, non toglie l'invariante (T-B5-54).
func TestPO33LaRegolaDelCliente(t *testing.T) {
	r := func(tipo string, b bool) valutazione.RegolaFabbisogno {
		return valutazione.RegolaFabbisogno{TipoDocumento: tipo, Bloccante: b}
	}
	f := func(tipo string, b, inv bool, nota string, cliente bool) valutazione.FabbisognoRisolto {
		return valutazione.FabbisognoRisolto{TipoDocumento: tipo, Bloccante: b, Invariante: inv, NotaRegola: nota, RegolaCliente: cliente}
	}
	fab := valutazione.Classificazione{Ruolo: "componente", Categoria: "fabbricato"}
	pro := valutazione.Classificazione{Ruolo: "prodotto", Categoria: "fabbricato"}
	com := valutazione.Classificazione{Ruolo: "componente", Categoria: "commerciale", Confermata: true}
	for _, c := range []struct {
		nome    string
		finito  bool
		cliente bool
		regole  []valutazione.RegolaFabbisogno
		cl      valutazione.Classificazione
		atteso  []valutazione.FabbisognoRisolto
	}{
		{"finito, regola del cliente senza 2D e senza cad_3d", true, true, []valutazione.RegolaFabbisogno{r("sviluppo_dxf", false)}, pro,
			[]valutazione.FabbisognoRisolto{f("disegno_2d", true, true, "regola_cliente_senza_2d", true), f("sviluppo_dxf", false, false, "", true)}},
		{"finito, riga esplicita con il 2D non bloccante", true, true, []valutazione.RegolaFabbisogno{r("cad_3d", true), r("disegno_2d", false)}, pro,
			[]valutazione.FabbisognoRisolto{f("cad_3d", true, false, "", true), f("disegno_2d", true, true, "regola_cliente_2d_non_bloccante", true)}},
		{"sciolto, regola del cliente senza 2D", false, true, []valutazione.RegolaFabbisogno{r("cad_3d", true)}, fab,
			[]valutazione.FabbisognoRisolto{f("cad_3d", true, false, "", true), f("disegno_2d", true, true, "regola_cliente_senza_2d", true)}},
		{"commerciale, regola del cliente senza 2D", false, true, []valutazione.RegolaFabbisogno{r("cad_3d", false)}, com,
			[]valutazione.FabbisognoRisolto{f("cad_3d", false, false, "", true), f("disegno_2d", true, true, "regola_cliente_senza_2d", true)}},
		{"sciolto, riga esplicita con il 2D non bloccante (K-01 A)", false, true, []valutazione.RegolaFabbisogno{r("disegno_2d", false)}, fab,
			[]valutazione.FabbisognoRisolto{f("disegno_2d", false, true, "regola_cliente_2d_non_bloccante", true)}},
		{"commerciale, riga esplicita con il 2D non bloccante (K-01 A)", false, true, []valutazione.RegolaFabbisogno{r("disegno_2d", false)}, com,
			[]valutazione.FabbisognoRisolto{f("disegno_2d", false, true, "regola_cliente_2d_non_bloccante", true)}},
		{"finito, regole di default", true, false, []valutazione.RegolaFabbisogno{r("disegno_2d", true), r("cad_3d", true)}, pro,
			[]valutazione.FabbisognoRisolto{f("cad_3d", true, false, "", false), f("disegno_2d", true, true, "", false)}},
		{"commerciale, regole di default (nessuna riga)", false, false, nil, com,
			[]valutazione.FabbisognoRisolto{f("disegno_2d", true, true, "schema_senza_2d", false)}},
		{"una riga di default non bloccante sul 2D non toglie l'invariante (T-B5-54)", false, false, []valutazione.RegolaFabbisogno{r("disegno_2d", false)}, fab,
			[]valutazione.FabbisognoRisolto{f("disegno_2d", true, true, "", false)}},
		{"due righe dello stesso tipo: vale la più bloccante", false, true, []valutazione.RegolaFabbisogno{r("cad_3d", false), r("cad_3d", true)}, fab,
			[]valutazione.FabbisognoRisolto{f("cad_3d", true, false, "", true), f("disegno_2d", true, true, "regola_cliente_senza_2d", true)}},
	} {
		if got := valutazione.FabbisogniDelComponente(c.finito, c.cliente, c.regole, c.cl); !reflect.DeepEqual(got, c.atteso) {
			t.Errorf("%s:\n%+v\natteso\n%+v", c.nome, got, c.atteso)
		}
	}

	// Nella completezza: il 2D del finito resta fra le voci, quello dello sciolto va fra i non bloccanti, con la nota.
	finito := rigaP(cProdotto, "7120100A", "finito", true, fv("cad_3d", true, "ok"), fv("disegno_2d", false, "manca"))
	finito.RegolaCliente = true
	sciolto := rigaP(cSciolto, "7120200A", "sciolto", false, fv("disegno_2d", false, "manca"))
	sciolto.RegolaCliente = true
	d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa(), Righe: []valutazione.RigaDelPerimetro{finito, sciolto}})
	if v := voceDi(t, d, cProdotto, "disegno_2d"); !v.Bloccante || v.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante || !v.RegolaCliente ||
		esitoDi(v) != "manca/nessun_documento" {
		t.Errorf("il 2D del finito %+v", v)
	}
	if haVoce(d, cSciolto, "") {
		t.Errorf("lo sciolto ha voci: %s", vociDi(d))
	}
	if n, ok := nonBloccanteDi(d, cSciolto, "disegno_2d"); !ok || n.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante || n.EsitoVista != "manca" {
		t.Errorf("il 2D dello sciolto fra i non bloccanti: %+v %v", n, ok)
	}
	if d.Stato != valutazione.DocumentiIncompleta {
		t.Errorf("stato %s: il 2D del finito manca con certezza", statoDi(d))
	}
}

// TestK01LetturaA (K-01, lettura A, confermata dall'utente il 06/10 [U]; E1R §4.4; T-E1R-02): la riga esplicita del
// cliente che rende il 2D non bloccante si rispetta sui componenti (sottoassieme, sciolto, commerciale), mai sul finito;
// la nota regola_cliente_2d_non_bloccante c'è in tutti i casi. Senza la riga del cliente (una riga di default) la lettura
// non entra.
func TestK01LetturaA(t *testing.T) {
	regole := []valutazione.RegolaFabbisogno{{TipoDocumento: "disegno_2d", Bloccante: false}}
	for _, c := range []struct {
		tipo      string
		finito    bool
		bloccante bool
	}{
		{"finito", true, true},
		{"sottoassieme", false, false},
		{"sciolto", false, false},
		{"commerciale", false, false},
	} {
		id := uid(0x7ca)
		cl := valutazione.Classifica(valutazione.ComponenteDaClassificare{ComponenteID: &id, Tipo: c.tipo}, nil)
		got := valutazione.FabbisogniDelComponente(c.finito, true, regole, cl)
		if len(got) != 1 || got[0].Bloccante != c.bloccante || got[0].NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante || !got[0].Invariante {
			t.Errorf("%s: %+v", c.tipo, got)
		}
		dflt := valutazione.FabbisogniDelComponente(c.finito, false, regole, cl)
		if len(dflt) != 1 || !dflt[0].Bloccante || dflt[0].NotaRegola != "" {
			t.Errorf("%s, riga di default: %+v", c.tipo, dflt)
		}
	}
}

// ---- la voce del 2D ----

// TestLaVoceDelDisegno (contratto §1.6, righe «Voce di un fabbisogno», «La deroga», «Da verificare»; R62 b A, R62 D.4;
// LD-17; T-E1R-10; decisioni sull'analista, punto 4; PO-08 nella seconda fotografia): la voce guarda tutto il gruppo; un
// 2D corrente, confermato e valido è presente anche senza cartiglio; il contenuto non verificato, l'associazione non
// confermata («assegna», il candidato, la proposta della vista, il da_determinare del motore A) danno da_verificare; uno
// scarto non soddisfa la voce; la deroga non sostituisce il 2D; un documento in un formato non configurato non lo
// soddisfa.
func TestLaVoceDelDisegno(t *testing.T) {
	pdf, tiff := valutazione.Formato2DPDF, valutazione.Formato2DTIFF
	val, ver, nonConf := valutazione.ValiditaValido, valutazione.ValiditaDaVerificare, valutazione.ValiditaFormatoNonConfigurato
	conf, man, prop := ancoraggio.OrigineConfermato, ancoraggio.OrigineManuale, ancoraggio.OrigineProposto
	tiffSenzaCartiglio := dueD(sha2, aTIFF, tiff, val, conf, true)
	tiffSenzaCartiglio.Cartiglio = valutazione.CartiglioFormatoSenzaLettura
	for _, c := range []struct {
		nome   string
		in     valutazione.DisegnoDellaVoce
		atteso string
		file   *uuid.UUID
	}{
		{"nessun 2D", valutazione.DisegnoDellaVoce{}, "manca/nessun_documento", nil},
		{"un PDF corrente, confermato e valido", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueDValido(sha1, aPDF1))}, "presente/", nil},
		{"PO-08, la seconda fotografia: il TIFF decodificato e confermato, senza cartiglio", valutazione.DisegnoDellaVoce{Gruppo: gruppo(tiffSenzaCartiglio)}, "presente/", nil},
		{"PO-08 con i fatti di oggi: il TIFF confermato è da verificare", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha2, aTIFF, tiff, ver, conf, true))},
			"da_verificare/contenuto_non_verificabile", nil},
		{"un documento non corrente non conta", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, val, conf, false))}, "manca/nessun_documento", nil},
		{"tutto il gruppo, non solo il primario: un TIFF valido accanto a un PDF da verificare",
			valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, ver, conf, true), dueD(sha2, aTIFF, tiff, val, conf, true))}, "presente/", nil},
		{"«assegna» non chiude il fabbisogno (R62 b A)", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, val, man, false))},
			"da_verificare/associazione_non_confermata", &aPDF1},
		{"un candidato del motore A", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, val, prop, false))},
			"da_verificare/associazione_non_confermata", &aPDF1},
		{"uno scarto non soddisfa la voce (T-E1R-10)", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, val, prop, false)), Scartati: []uuid.UUID{aPDF1}},
			"manca/nessun_documento", nil},
		{"un candidato in un formato non configurato non conta", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aAltro, "", nonConf, prop, false))},
			"manca/nessun_documento", nil},
		{"la proposta aperta della vista", valutazione.DisegnoDellaVoce{PropostaAperta: ptr(uid(0x901)), FileProposto: &aPDF2},
			"da_verificare/associazione_non_confermata", &aPDF2},
		{"la proposta aperta della vista senza file utilizzabile non conta", valutazione.DisegnoDellaVoce{PropostaAperta: ptr(uid(0x901))}, "manca/nessun_documento", nil},
		{"un da_determinare candidato del motore A (LD-17)", valutazione.DisegnoDellaVoce{CandidatiDaDeterminare: []uuid.UUID{aDaDet}},
			"da_verificare/associazione_non_confermata", &aDaDet},
		{"un da_determinare scartato non conta", valutazione.DisegnoDellaVoce{CandidatiDaDeterminare: []uuid.UUID{aDaDet}, Scartati: []uuid.UUID{aDaDet}},
			"manca/nessun_documento", nil},
		{"la deroga non sostituisce il 2D (R62 D.4)", valutazione.DisegnoDellaVoce{DerogaID: &derogaA}, "manca/derogato_non_sostituisce_2d", nil},
		{"la deroga con un 2D valido", valutazione.DisegnoDellaVoce{DerogaID: &derogaA, Gruppo: gruppo(dueDValido(sha1, aPDF1))}, "presente/", nil},
		{"un DWG confermato non soddisfa il 2D (T-B0-30)", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(shaDWG, aDWG, "", nonConf, conf, true))},
			"manca/formato_non_configurato", nil},
		{"la deroga prima del formato non configurato", valutazione.DisegnoDellaVoce{DerogaID: &derogaA, Gruppo: gruppo(dueD(shaDWG, aDWG, "", nonConf, conf, true))},
			"manca/derogato_non_sostituisce_2d", nil},
		{"il contenuto prima dell'associazione", valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, ver, conf, true), dueD(sha2, aPDF2, pdf, val, man, false))},
			"da_verificare/contenuto_non_verificabile", nil},
		{"«assegna» prima della proposta della vista e del da_determinare",
			valutazione.DisegnoDellaVoce{Gruppo: gruppo(dueD(sha1, aPDF1, pdf, val, man, false)), FileProposto: &aPDF2, CandidatiDaDeterminare: []uuid.UUID{aDaDet}},
			"da_verificare/associazione_non_confermata", &aPDF1},
		{"la proposta della vista prima del da_determinare", valutazione.DisegnoDellaVoce{FileProposto: &aPDF2, CandidatiDaDeterminare: []uuid.UUID{aDaDet}},
			"da_verificare/associazione_non_confermata", &aPDF2},
	} {
		e, m, f := valutazione.VoceDelDisegno(c.in)
		if string(e)+"/"+string(m) != c.atteso || !reflect.DeepEqual(f, c.file) {
			t.Errorf("%s: %s/%s, file %v; atteso %s, file %v", c.nome, e, m, f, c.atteso, c.file)
		}
	}
	// I da_determinare in ordine di allegato, qualunque sia l'ordine dell'ingresso.
	a, b := uid(0x7f2), uid(0x7f1)
	if _, _, f := valutazione.VoceDelDisegno(valutazione.DisegnoDellaVoce{CandidatiDaDeterminare: []uuid.UUID{a, b}}); f == nil || *f != b {
		t.Errorf("file %v, atteso %s", f, b)
	}
}

// TestGliEsitiDallaVista (contratto §1.6, «per gli altri tipi bloccanti: come nella vista»; S3; decisioni sull'analista,
// punto 4; T-B5-55): gli esiti di v_fascicolo; un valore che la vista non ha non è né presente né mancante.
func TestGliEsitiDallaVista(t *testing.T) {
	for esito, atteso := range map[string]string{
		"ok": "presente/", "ok_in_coda": "presente/", "ok_errore_nas": "presente/", "derogato": "presente/",
		"da_confermare": "da_verificare/associazione_non_confermata", "sul_portale": "da_verificare/sul_portale",
		"manca": "manca/nessun_documento", "altro": "da_verificare/", "": "da_verificare/",
	} {
		if e, m := valutazione.EsitoDallaVista(esito); string(e)+"/"+string(m) != atteso {
			t.Errorf("%q: %s/%s, atteso %s", esito, e, m, atteso)
		}
	}
}

// ---- il perimetro e lo stato (PO-36) ----

// TestLaChiusuraDelPerimetro (R102 A, T-E1-10; T-B5-90, T-B5-92): ogni motivo in ordine; una BOM di lavoro assente non
// chiude il perimetro anche con la fonte confermata e niente da decidere; una BOM di lavoro di un solo nodo lo chiude.
func TestLaChiusuraDelPerimetro(t *testing.T) {
	tutto := valutazione.StrutturaDaVerificare{RigheDaDecidere: []string{"r"}, ArchiDaDecidere: []string{"a"}, RimozioniAperte: []string{"x"}}
	passi := []struct {
		cambia func(*valutazione.StrutturaDaVerificare)
		motivo string
	}{
		{func(*valutazione.StrutturaDaVerificare) {}, valutazione.MotivoPerimetroTargetSenzaComponente},
		{func(s *valutazione.StrutturaDaVerificare) { s.ConComponente = true }, valutazione.MotivoPerimetroFonteNonConfermata},
		{func(s *valutazione.StrutturaDaVerificare) { s.FonteConfermata = true }, valutazione.MotivoPerimetroBOMDiLavoroAssente},
		{func(s *valutazione.StrutturaDaVerificare) { s.BOMDiLavoro = true }, valutazione.MotivoPerimetroNodiDaDecidere},
		{func(s *valutazione.StrutturaDaVerificare) { s.RigheDaDecidere = nil }, valutazione.MotivoPerimetroArchiDaDecidere},
		{func(s *valutazione.StrutturaDaVerificare) { s.ArchiDaDecidere = nil }, valutazione.MotivoPerimetroRimozioniAperte},
		{func(s *valutazione.StrutturaDaVerificare) { s.RimozioniAperte = nil }, ""},
	}
	for _, p := range passi {
		p.cambia(&tutto)
		if chiuso, m := valutazione.ChiusuraDelPerimetro(tutto); chiuso != (p.motivo == "") || m != p.motivo {
			t.Errorf("%+v: chiuso %v, motivo %q; atteso %q", tutto, chiuso, m, p.motivo)
		}
	}
	senzaBOM := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, Radici: []string{"n"}, Nodi: []string{"n"}}
	if chiuso, m := valutazione.ChiusuraDelPerimetro(senzaBOM); chiuso || m != valutazione.MotivoPerimetroBOMDiLavoroAssente {
		t.Errorf("senza la BOM di lavoro: %v %q (T-B5-92)", chiuso, m)
	}
	unNodo := senzaBOM
	unNodo.BOMDiLavoro = true
	if chiuso, m := valutazione.ChiusuraDelPerimetro(unNodo); !chiuso || m != "" {
		t.Errorf("la BOM di lavoro della sola radice: %v %q", chiuso, m)
	}
}

// TestPO36LaCompletezzaSullaRegola (PO-36; R72 D [R], R102 A, T-E1-10; T-B0-07): il 2D del finito manca e restano
// proposte: incompleta; i documenti delle righe confermate presenti e proposte aperte: non_calcolabile, con i previsti;
// il perimetro chiuso e tutto presente: completa; senza la fonte confermata mai completa (con il 2D del finito che manca
// incompleta, senza mancanze certe non_calcolabile); con la fonte confermata ma un nodo o una rimozione da decidere mai
// completa; a perimetro chiuso una voce da verificare dà incompleta; il target senza componente non_calcolabile; una riga
// con una rimozione aperta non dà voci certe, e le sue voci vanno fra i previsti.
func TestPO36LaCompletezzaSullaRegola(t *testing.T) {
	nodo := valutazione.NodoDaPrevedere{Nodo: "nodo:proposto", CodiceProposto: "7120300A", TipoComponente: "sciolto",
		Regole: []valutazione.RegolaFabbisogno{{TipoDocumento: "disegno_2d", Bloccante: true}, {TipoDocumento: "sviluppo_dxf"}}, Motivo: valutazione.MotivoPrevistoNodoProposto}
	aperta := chiusa()
	aperta.RigheDaDecidere = []string{"componente_proposta:x"}
	senzaFonte := chiusa()
	senzaFonte.FonteConfermata, senzaFonte.BOMDiLavoro = false, false
	conRimozione := chiusa()
	conRimozione.RimozioniAperte = []string{"arco:x>y"}
	daVerificare := scioltoDefault(cSciolto, "7120200A", false)
	daVerificare.Disegno.Gruppo = gruppo(dueD(sha2, aPDF2, valutazione.Formato2DPDF, valutazione.ValiditaValido, ancoraggio.OrigineManuale, false))
	tutti := []valutazione.RigaDelPerimetro{finitoDefault(true), scioltoDefault(cSciolto, "7120200A", true)}
	for _, c := range []struct {
		nome   string
		s      valutazione.StrutturaDaVerificare
		righe  []valutazione.RigaDelPerimetro
		nodi   []valutazione.NodoDaPrevedere
		atteso string
	}{
		{"il 2D del finito manca e restano proposte", aperta, []valutazione.RigaDelPerimetro{finitoDefault(false)}, []valutazione.NodoDaPrevedere{nodo},
			"incompleta/voce_mancante aperto/nodi_da_decidere"},
		{"i documenti presenti e proposte aperte", aperta, tutti, []valutazione.NodoDaPrevedere{nodo}, "non_calcolabile/perimetro_aperto aperto/nodi_da_decidere"},
		{"il perimetro chiuso e tutto presente", chiusa(), tutti, nil, "completa/ chiuso/"},
		{"senza la fonte confermata e tutto presente", senzaFonte, tutti, nil, "non_calcolabile/perimetro_aperto aperto/fonte_non_confermata"},
		{"senza la fonte confermata e il 2D del finito che manca", senzaFonte, []valutazione.RigaDelPerimetro{finitoDefault(false)}, nil,
			"incompleta/voce_mancante aperto/fonte_non_confermata"},
		{"senza la fonte confermata e una voce da verificare", senzaFonte, []valutazione.RigaDelPerimetro{finitoDefault(true), daVerificare}, nil,
			"non_calcolabile/perimetro_aperto aperto/fonte_non_confermata"},
		{"la fonte confermata e una rimozione aperta", conRimozione, tutti, nil, "non_calcolabile/perimetro_aperto aperto/rimozioni_aperte"},
		{"a perimetro chiuso una voce da verificare", chiusa(), []valutazione.RigaDelPerimetro{finitoDefault(true), daVerificare}, nil,
			"incompleta/voce_non_presente chiuso/"},
		{"il target senza componente", valutazione.StrutturaDaVerificare{}, nil, nil, "non_calcolabile/target_senza_componente aperto/target_senza_componente"},
	} {
		d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: c.s, Righe: c.righe, Nodi: c.nodi})
		if statoDi(d) != c.atteso {
			t.Errorf("%s: %s, atteso %s (%s)", c.nome, statoDi(d), c.atteso, vociDi(d))
		}
		if len(c.nodi) > 0 {
			if p := previstoDi(t, d, "nodo:proposto", "disegno_2d"); p.CodiceProposto != "7120300A" || len(d.Previsti) != 1 {
				t.Errorf("%s: previsti %+v (solo i bloccanti)", c.nome, d.Previsti)
			}
		}
	}

	t.Run("una riga con una rimozione aperta non dà voci certe", func(t *testing.T) {
		rimossa := scioltoDefault(cSciolto, "7120200A", false)
		rimossa.RimozioneAperta = true
		d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: conRimozione, Righe: []valutazione.RigaDelPerimetro{finitoDefault(true), rimossa}})
		if haVoce(d, cSciolto, "") || len(d.NonBloccanti) != 0 {
			t.Errorf("voci %s, non bloccanti %+v", vociDi(d), d.NonBloccanti)
		}
		p := previstoDi(t, d, ancoraggio.RifComponente(cSciolto), "disegno_2d")
		if p.Motivo != valutazione.MotivoPrevistoRimozioneAperta || p.CodiceProposto != "7120200A" || len(d.Previsti) != 1 {
			t.Errorf("previsti %+v", d.Previsti)
		}
		if statoDi(d) != "non_calcolabile/perimetro_aperto aperto/rimozioni_aperte" {
			t.Errorf("stato %s: il 2D dello sciolto non è una mancanza certa", statoDi(d))
		}
	})
	t.Run("i campi della voce dalla vista", func(t *testing.T) {
		doc, der, pa, file := uid(0xd11), uid(0xd12), uid(0xd13), uid(0xd14)
		r := finitoDefault(true)
		r.Fabbisogni[0] = valutazione.FabbisognoDellaRiga{TipoDocumento: "cad_3d", Bloccante: true, EsitoVista: "da_confermare", CalcolataDa: valutazione.CalcolataDaVista,
			DocumentoID: &doc, StatoNas: "scritto", DerogaID: &der, PropostaAperta: &pa, FileCandidato: &file}
		v := voceDi(t, valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa(), Righe: []valutazione.RigaDelPerimetro{r}}), cProdotto, "cad_3d")
		if esitoDi(v) != "da_verificare/associazione_non_confermata" || v.CalcolataDa != valutazione.CalcolataDaVista || *v.DocumentoID != doc ||
			v.StatoNas != "scritto" || *v.DerogaID != der || *v.PropostaAperta != pa || v.FileCandidato == nil || *v.FileCandidato != file ||
			!v.DelProdotto || v.Codice != "7120100A" || v.TipoComponente != "finito" || v.Invariante || v.Disegni != nil {
			t.Errorf("voce %+v", v)
		}
		r.Fabbisogni[0].EsitoVista = "sul_portale"
		v = voceDi(t, valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa(), Righe: []valutazione.RigaDelPerimetro{r}}), cProdotto, "cad_3d")
		if esitoDi(v) != "da_verificare/sul_portale" || v.FileCandidato != nil {
			t.Errorf("sul portale: %+v", v)
		}
	})
}

// TestLaCompletezzaEDeterministica (R72 D; il determinismo del motore): le righe, i nodi e le conferme in un altro ordine
// danno gli stessi byte canonici; gli ingressi di chi chiama non cambiano; l'uscita non condivide memoria con l'ingresso.
func TestLaCompletezzaEDeterministica(t *testing.T) {
	nodoA := valutazione.NodoDaPrevedere{Nodo: "nodo:a", TipoComponente: "sciolto", Regole: []valutazione.RegolaFabbisogno{{TipoDocumento: "disegno_2d", Bloccante: true}},
		Motivo: valutazione.MotivoPrevistoNodoProposto}
	nodoB := valutazione.NodoDaPrevedere{Nodo: "nodo:b", TipoComponente: "sottoassieme", Regole: []valutazione.RegolaFabbisogno{{TipoDocumento: "disegno_2d", Bloccante: true}},
		Motivo: valutazione.MotivoPrevistoNodoProposto, Disegni: gruppo(dueD(sha3, aPDF3, valutazione.Formato2DPDF, valutazione.ValiditaValido, ancoraggio.OrigineProposto, false))}
	commerciale := rigaP(cCommerciale, "7120400A", "commerciale", false)
	in := valutazione.IngressoCompletezza{Struttura: chiusa(),
		Righe:    []valutazione.RigaDelPerimetro{finitoDefault(true), scioltoDefault(cSciolto, "7120200A", true), commerciale},
		Nodi:     []valutazione.NodoDaPrevedere{nodoA, nodoB},
		Conferme: []valutazione.ConfermaCategoria{confermaCategoria(cCommerciale, "minuteria"), confermaCategoria(cSciolto, "fabbricato")}}
	prima := canonicoDi(t, in)
	a := valutazione.Completezza(in)
	rov := valutazione.IngressoCompletezza{Struttura: in.Struttura, Righe: rovescia(in.Righe), Nodi: rovescia(in.Nodi), Conferme: rovescia(in.Conferme)}
	if canonicoDi(t, valutazione.Completezza(rov)) != canonicoDi(t, a) {
		t.Error("l'ordine degli ingressi cambia l'esito")
	}
	if canonicoDi(t, in) != prima {
		t.Error("la completezza ha cambiato gli ingressi")
	}
	if haVoce(a, cCommerciale, "disegno_2d") || !haVoce(a, cSciolto, "disegno_2d") || a.Voci[0].ComponenteID != cProdotto {
		t.Errorf("voci %s", vociDi(a))
	}
	v := voceDi(t, a, cSciolto, "disegno_2d")
	v.Disegni.Primario.NomeFile = "cambiato"
	if in.Righe[1].Disegno.Gruppo.Primario.NomeFile == "cambiato" {
		t.Error("la voce condivide il gruppo dei 2D con l'ingresso")
	}
	// Due previsti con la stessa chiave (nodo, tipo), che l'adattatore non dà: vale quello con il motivo e il codice
	// minori, in qualunque ordine.
	uno := valutazione.NodoDaPrevedere{Nodo: "nodo:x", CodiceProposto: "7120900A", TipoComponente: "sciolto", Motivo: valutazione.MotivoPrevistoNodoProposto}
	due := uno
	due.CodiceProposto, due.Motivo = "7120800A", valutazione.MotivoPrevistoNodoDecisoFuoriPerimetro
	for _, nodi := range [][]valutazione.NodoDaPrevedere{{uno, due}, {due, uno}} {
		d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa(), Nodi: nodi})
		if len(d.Previsti) != 1 || d.Previsti[0].Motivo != valutazione.MotivoPrevistoNodoDecisoFuoriPerimetro || d.Previsti[0].CodiceProposto != "7120800A" {
			t.Errorf("previsti %+v", d.Previsti)
		}
	}
	// Il componente del prodotto viene prima anche quando il suo ID è il più grande.
	grande := finitoDefault(true)
	grande.ComponenteID = uid(0x7ff)
	d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa(), Righe: []valutazione.RigaDelPerimetro{scioltoDefault(cSciolto, "7120200A", true), grande}})
	if len(d.Voci) != 3 || d.Voci[0].ComponenteID != uid(0x7ff) || d.Voci[1].ComponenteID != uid(0x7ff) || d.Voci[0].TipoDocumento != "cad_3d" {
		t.Errorf("voci %s", vociDi(d))
	}
}

// TestIValoriDellaCompletezza: i valori e i campi del contratto (§2.3, §2.5, §2.6). Si riscrive con «Riscritta per …».
func TestIValoriDellaCompletezza(t *testing.T) {
	for _, c := range [][2]string{
		{string(valutazione.DocumentiNonCalcolabile), "non_calcolabile"}, {string(valutazione.DocumentiIncompleta), "incompleta"},
		{string(valutazione.DocumentiCompleta), "completa"},
		{string(valutazione.EsitoPresente), "presente"}, {string(valutazione.EsitoDaVerificare), "da_verificare"}, {string(valutazione.EsitoManca), "manca"},
		{valutazione.NotaSchemaSenza2D, "schema_senza_2d"}, {valutazione.NotaRegolaClienteSenza2D, "regola_cliente_senza_2d"},
		{valutazione.NotaRegolaCliente2DNonBloccante, "regola_cliente_2d_non_bloccante"},
		{valutazione.RuoloProdotto, "prodotto"}, {valutazione.RuoloComponente, "componente"},
		{valutazione.CategoriaFabbricato, "fabbricato"}, {valutazione.CategoriaCommerciale, "commerciale"}, {valutazione.CategoriaMinuteria, "minuteria"},
		{valutazione.CategoriaNonDeterminata, "non_determinata"},
		{valutazione.OrigineCategoriaConfermata, "confermato"}, {valutazione.OrigineCategoriaDerivata, "derivato_dal_tipo"},
		{valutazione.OrigineCategoriaProposta, "proposto"},
		{valutazione.CodiceDocumentiDerogaNonSostituisce2D, "documenti.deroga_non_sostituisce_2d"},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, atteso %q", c[0], c[1])
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{valutazione.VoceFabbisogno{}, "ComponenteID:componente_id Codice:codice TipoComponente:tipo_componente DelProdotto:del_prodotto TipoDocumento:tipo_documento " +
			"Bloccante:bloccante RegolaCliente:regola_cliente Esito:esito Motivo:motivo Disegni:disegni DocumentoID:documento_id StatoNas:stato_nas DerogaID:deroga_id " +
			"PropostaAperta:proposta_aperta FileCandidato:file_candidato CalcolataDa:calcolata_da Invariante:invariante NotaRegola:nota_regola Categoria:categoria"},
		{valutazione.FabbisognoPrevisto{}, "Nodo:nodo CodiceProposto:codice_proposto TipoDocumento:tipo_documento Disegni:disegni Motivo:motivo"},
		{valutazione.FabbisognoInformativo{}, "ComponenteID:componente_id TipoDocumento:tipo_documento EsitoVista:esito_vista DerogaID:deroga_id NotaRegola:nota_regola"},
		{valutazione.CompletezzaDocumentale{}, "Stato:stato Voci:voci Previsti:previsti NonBloccanti:non_bloccanti Motivo:motivo PerimetroChiuso:perimetro_chiuso " +
			"MotivoPerimetro:motivo_perimetro"},
		{valutazione.Classificazione{}, "Ruolo:ruolo Categoria:categoria Origine:origine Confermata:confermata Motivo:motivo Proposta:proposta"},
		{valutazione.ConfermaCategoria{}, "ComponenteID:componente_id Categoria:categoria Da:da Il:il"},
		{valutazione.ComponenteClassificato{}, "ComponenteID:componente_id Classificazione:classificazione"},
	} {
		if got := campiJSON(c.tipo); got != c.campi {
			t.Errorf("%T: campi %q, attesi %q", c.tipo, got, c.campi)
		}
	}
	if f, ok := reflect.TypeOf(valutazione.ProdottoValutato{}).FieldByName("Documenti"); !ok || f.Tag.Get("json") != "documenti" ||
		f.Type != reflect.TypeOf(valutazione.CompletezzaDocumentale{}) {
		t.Error("ProdottoValutato.Documenti")
	}
	if f, ok := reflect.TypeOf(valutazione.ValutazioneProdotti{}).FieldByName("Classificazioni"); !ok || f.Tag.Get("json") != "classificazioni,omitempty" {
		t.Error("ValutazioneProdotti.Classificazioni")
	}
}
