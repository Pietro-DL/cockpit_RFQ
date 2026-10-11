package web

// L1 — Giro 5, A1d (BE-10, BE-17; contratto A1d §2.3; v1.1 §4.4; T-A1d-29; RAC-11): etichetta e la sua tabella. La
// funzione segue i puntatori e i tipi con nome e mette fra quadre quello che non conosce; ogni costante di ogni dominio,
// letta dai sorgenti dei pacchetti con go/parser come dice l'appendice B del piano di A1d backend (più
// origine_categoria, RAC-11), ha il suo testo, e per quei valori etichetta non risponde mai «[…]»; i testi sono puliti.
// Una costante aggiunta domani a una di quelle sorgenti fa cadere la prova finché non ha il suo testo.
//
// I clienti di questi test sono inventati: qui non ce n'è nessuno, solo i valori degli enum del codice.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

func TestEtichettaSegueIPuntatoriEITipiConNome(t *testing.T) {
	origine := ancoraggio.OrigineConfermato
	pOrigine := &origine
	var interfaccia any = valutazione.ProdottoNonPronto
	for _, c := range []struct {
		dominio string
		valore  any
		atteso  string
	}{
		{"stato_prodotto", valutazione.ProdottoNonPronto, "non pronto"},
		{"stato_prodotto", "pronto_fattibilita", "pronto per la fattibilità"},
		{"origine_dato", &origine, "confermato"},
		{"origine_dato", &pOrigine, "confermato"},
		{"origine_dato", (*ancoraggio.OrigineDato)(nil), "[]"},
		{"stato_prodotto", &interfaccia, "non pronto"},
		{"stato_prodotto", "", "[]"},
		{"stato_prodotto", valutazione.StatoProdotto(""), "[]"},
		{"stato_prodotto", nil, "[]"},
		{"stato_prodotto", "boh", "[boh]"},
		{"boh", "x", "[x]"},
		{"stato_prodotto", true, "[true]"},
		{"stato_prodotto", 3, "[3]"},
		{"stato_prodotto", maUUID(1), "[" + maUUID(1).String() + "]"},
		{"origine_categoria", valutazione.OrigineCategoriaDerivata, "dal tipo"},
		{"motivo_conflitto", valutazione.MotivoConflittoRimozione, "legame tolto dallo STEP"},
		{"motivo_conflitto", "disegno_2d", "2D"}, // il motivo di un conflitto nuovo_file è il tipo del documento
		{"tipo_documento", db.TipoDocumentoDisegno2d, "2D"},
		{"stato_thread", db.StatoThreadCHIUSA, "chiusa"},
		{"fase", db.FaseRICEVUTA, "ricevuta"},
		{"motivo_grafo", "completezza del grafo non dichiarata dal documento", "completezza del grafo non dichiarata dal documento"},
		{"motivo_grafo", "un motivo del caricatore", "[un motivo del caricatore]"},
	} {
		if got := etichetta(c.dominio, c.valore); got != c.atteso {
			t.Errorf("etichetta(%q, %#v) = %q, atteso %q", c.dominio, c.valore, got, c.atteso)
		}
	}
	// conTipiDocumento non tocca le voci proprie del dominio.
	m := conTipiDocumento(map[string]string{"disegno_2d": "proprio"})
	if m["disegno_2d"] != "proprio" || m["cad_3d"] != etichetteTipoDocumentoMA["cad_3d"] || len(m) != len(etichetteTipoDocumentoMA) {
		t.Errorf("conTipiDocumento = %v", m)
	}
}

// maCostante: una costante stringa di un pacchetto, letta dal sorgente: il blocco const (un numero per dichiarazione),
// il tipo dichiarato ("" = senza tipo; con la ripetizione implicita di Go il tipo e il valore della riga sopra), il nome e
// il valore letterale.
type maCostante struct {
	blocco             int
	tipo, nome, valore string
}

// maCostantiDi: le costanti stringa dei file non di prova di una cartella, con go/parser (appendice B).
func maCostantiDi(t *testing.T, cartella string) []maCostante {
	t.Helper()
	voci, err := os.ReadDir(cartella)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []maCostante
	blocco := 0
	for _, v := range voci {
		n := v.Name()
		if v.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(cartella, n), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			blocco++
			tipo := ""
			var ultimi []ast.Expr
			for _, sp := range g.Specs {
				vs := sp.(*ast.ValueSpec)
				switch {
				case vs.Type != nil:
					tipo = ""
					if id, ok := vs.Type.(*ast.Ident); ok {
						tipo = id.Name
					}
				case len(vs.Values) > 0:
					tipo = ""
				}
				valori := vs.Values
				if len(valori) == 0 {
					valori = ultimi
				} else {
					ultimi = valori
				}
				for i, nome := range vs.Names {
					if i >= len(valori) {
						continue
					}
					bl, ok := valori[i].(*ast.BasicLit)
					if !ok || bl.Kind != token.STRING {
						continue
					}
					s, err := strconv.Unquote(bl.Value)
					if err != nil {
						t.Fatal(err)
					}
					out = append(out, maCostante{blocco: blocco, tipo: tipo, nome: nome.Name, valore: s})
				}
			}
		}
	}
	return out
}

// maStringhe: i valori di un enum del DB (db.All…Values) come stringhe.
func maStringhe[T ~string](vv []T) []string {
	var out []string
	for _, v := range vv {
		out = append(out, string(v))
	}
	return out
}

// maSemafori: i letterali dopo THEN ed ELSE nel CASE di v_thread_fase (migrations/0001, da risorse.FS), e la guardia
// che nessun'altra migrazione ridefinisca la vista.
func maSemafori(t *testing.T) []string {
	t.Helper()
	b, err := fs.ReadFile(risorse.FS, "migrations/0001_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	testo := string(b)
	i := strings.Index(testo, "CREATE VIEW v_thread_fase")
	if i < 0 {
		t.Fatal("v_thread_fase non è in migrations/0001")
	}
	vista := testo[i:]
	vista = vista[:strings.Index(vista, ";")]
	var out []string
	for _, m := range regexp.MustCompile(`(?:THEN|ELSE)\s+'([^']*)'`).FindAllStringSubmatch(vista, -1) {
		out = append(out, m[1])
	}
	altre, err := fs.Glob(risorse.FS, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, nome := range altre {
		if b, err := fs.ReadFile(risorse.FS, nome); err == nil && nome != "migrations/0001_schema.sql" &&
			regexp.MustCompile(`(?i)VIEW\s+v_thread_fase\b`).Match(b) {
			t.Errorf("%s ridefinisce v_thread_fase: i semafori si rileggono da lì", nome)
		}
	}
	return out
}

// TestEtichettaCopreOgniCostante (M-A1d-06): per ogni dominio, i valori delle sue sorgenti (appendice B del piano di
// A1d backend, più origine_categoria: RAC-11) hanno un testo nel loro dominio, e etichetta li traduce. Nessuna sorgente
// è vuota (un nome cambiato non fa passare la prova in silenzio), e nessuna voce della tabella è senza sorgente, tranne
// il testo libero noto di motivo_grafo (T-A1d-29).
func TestEtichettaCopreOgniCostante(t *testing.T) {
	const (
		val  = "../../core/valutazione"
		anc  = "../../core/ancoraggio"
		conf = "../../core/confronto"
		mot  = "../../core/inbox/classificazione/motorea"
	)
	costanti := map[string][]maCostante{}
	di := func(cartella string) []maCostante {
		if _, ok := costanti[cartella]; !ok {
			costanti[cartella] = maCostantiDi(t, cartella)
		}
		return costanti[cartella]
	}
	type sorgente struct {
		descr  string
		valori func() []string
	}
	// tipo T in cartella: le costanti con il tipo dichiarato T.
	tipo := func(cartella, nome string) sorgente {
		return sorgente{"tipo " + nome + " in " + cartella, func() []string {
			var out []string
			for _, c := range di(cartella) {
				if c.tipo == nome {
					out = append(out, c.valore)
				}
			}
			return out
		}}
	}
	// il blocco const di X: tutte le costanti stringa della dichiarazione che contiene X (trovata dal nome).
	blocco := func(cartella, nome string) sorgente {
		return sorgente{"il blocco const di " + nome + " in " + cartella, func() []string {
			b := -1
			for _, c := range di(cartella) {
				if c.nome == nome {
					b = c.blocco
				}
			}
			var out []string
			for _, c := range di(cartella) {
				if c.blocco == b {
					out = append(out, c.valore)
				}
			}
			return out
		}}
	}
	// le costanti senza tipo con il prefisso P.
	prefisso := func(cartella, p string) sorgente {
		return sorgente{"le costanti senza tipo " + p + "* in " + cartella, func() []string {
			var out []string
			for _, c := range di(cartella) {
				if c.tipo == "" && strings.HasPrefix(c.nome, p) {
					out = append(out, c.valore)
				}
			}
			return out
		}}
	}
	// la costante X, sola.
	nome := func(cartella, n string) sorgente {
		return sorgente{"la costante " + n + " in " + cartella, func() []string {
			var out []string
			for _, c := range di(cartella) {
				if c.nome == n {
					out = append(out, c.valore)
				}
			}
			return out
		}}
	}
	// valori Go diretti: un riferimento a una costante alias, o db.All…Values().
	diretti := func(descr string, vv []string) sorgente {
		return sorgente{descr, func() []string { return vv }}
	}
	tipiDocumento := diretti("db.AllTipoDocumentoValues()", maStringhe(db.AllTipoDocumentoValues()))
	sorgenti := map[string][]sorgente{
		"stato_prodotto":         {tipo(val, "StatoProdotto")},
		"motivo_prodotto":        {tipo(val, "MotivoProdotto")},
		"stato_fonte":            {tipo(val, "StatoFonte")},
		"motivo_fonte":           {tipo(val, "MotivoFonte")},
		"stato_verifica_asse":    {tipo(val, "StatoVerificaAsse")},
		"stato_smistamento":      {tipo(val, "StatoSmistamento")},
		"motivo_smistamento":     {tipo(val, "MotivoSmistamento")},
		"stato_documenti":        {tipo(val, "StatoDocumenti")},
		"esito_fabbisogno":       {tipo(val, "EsitoFabbisogno")},
		"motivo_fabbisogno":      {tipo(val, "MotivoFabbisogno")},
		"disponibilita":          {tipo(anc, "Disponibilita")},
		"stato_struttura":        {tipo(anc, "StatoStruttura")},
		"tipo_componente":        {diretti("db.AllTipoComponenteValues()", maStringhe(db.AllTipoComponenteValues()))},
		"origine_tipo":           {tipo(val, "OrigineTipo")},
		"categoria":              {blocco(val, "CategoriaFabbricato")},
		"origine_categoria":      {blocco(val, "OrigineCategoriaConfermata")},
		"origine_dato":           {tipo(anc, "OrigineDato")},
		"stato_nas":              {diretti("db.AllStatoNasValues()", maStringhe(db.AllStatoNasValues()))},
		"badge":                  {tipo(conf, "Badge")},
		"motivo_non_congelabile": {blocco(val, "NonCongelabileThreadNonValutato")},
		"motivo_thread":          {tipo(val, "MotivoThread")},
		"tipo_conflitto":         {tipo(val, "TipoConflitto")},
		"perimetro":              {blocco(val, "PerimetroDentro")},
		"asse_conflitto":         {tipo(val, "AsseConflitto")},
		"associazione":           {tipo(anc, "Associazione")},
		"autorita":               {tipo(anc, "Autorita")},
		"cartiglio":              {blocco(val, "CartiglioLetto")},
		"collocazione":           {tipo(anc, "Collocazione")},
		"compatibilita":          {tipo(mot, "Compatibilita")},
		"discordanza":            {prefisso(val, "Tra")},
		"esito_riconciliazione":  {tipo(anc, "EsitoRiconciliazione")},
		"esito_vista":            {blocco(val, "esitoPresenteAnalizzato"), blocco(val, "esitoFascicoloOK")},
		"estrazione":             {tipo(anc, "EsitoEstrazione")},
		"fonte_revisione":        {blocco(anc, "FonteEvidenzaCartiglio"), prefisso(val, "Fonte")},
		"motivo_abbinamento":     {blocco(anc, "MotivoAbbinamentoPerBase")},
		"motivo_asse":            {blocco(val, "MotivoAsseConflitto")},
		"motivo_classificazione": {blocco(val, "MotivoCategoriaFinitoSempre2D")},
		"motivo_conflitto": {blocco(val, "MotivoConflittoQuantita"), blocco(val, "MotivoConflittoComponenteDiverso"),
			diretti("valutazione.MotivoConflittoCodiceConfermato (un alias, non un letterale)", []string{string(valutazione.MotivoConflittoCodiceConfermato)}),
			prefisso(anc, "ParteDiscordanza"), tipiDocumento},
		"motivo_documentale": {blocco(anc, "MotivoDocumentaleCartiglioNonLetto"), nome(anc, "MotivoLettureDiscordanti"),
			nome(anc, "MotivoAncoraggioIdentitaDiscordanti")},
		"motivo_documenti":     {blocco(val, "MotivoDocumentiNonDeterminabile")},
		"motivo_file":          {blocco(val, "MotivoFileContenitore")},
		"motivo_grafo":         {blocco(anc, "motivoGrafoNonDichiarato")},
		"motivo_lettura":       {blocco(anc, "MotivoLetturaAssente"), blocco(val, "MotivoLetturaCodiceVuoto")},
		"motivo_non_congelato": {blocco(val, "NonCongelatoGestoNonRegistrato")},
		"motivo_perimetro":     {prefisso(val, "MotivoPerimetro")},
		"motivo_previsto":      {blocco(val, "MotivoPrevistoNodoProposto")},
		"motivo_proposto": {blocco(anc, "MotivoPropostoNonComposto"), blocco(anc, "MotivoLetturaAssente"),
			blocco(mot, "MotivoComposizioneFormaAssente")},
		"nota_regola":           {blocco(val, "NotaSchemaSenza2D")},
		"origine_candidato":     {tipo(anc, "OrigineCandidato")},
		"regola_primario":       {blocco(val, "RegolaPrimarioCorrente")},
		"relazione_disegni":     {prefisso(val, "Relazione")},
		"rev_provenienza":       {blocco(anc, "RevProvenienzaNonRegistrata")},
		"ruolo":                 {blocco(val, "RuoloProdotto")},
		"stato_identita_target": {tipo(val, "StatoIdentitaTarget")},
		"stato_revisione":       {blocco(val, "StatoRevisione2DAssente"), blocco(anc, "StatoRevisioneCandidata")},
		"stato_riga":            {diretti("db.AllStatoPropostaValues()", maStringhe(db.AllStatoPropostaValues()))},
		"stato_versione":        {diretti("db.AllStatoBomValues()", maStringhe(db.AllStatoBomValues()))},
		"tipo_documento":        {tipiDocumento},
		"validita_2d":           {tipo(val, "ValiditaDisegno2D")},
		"stato_thread":          {diretti("db.AllStatoThreadValues()", maStringhe(db.AllStatoThreadValues()))},
		"semaforo":              {diretti("il CASE di v_thread_fase (migrations/0001)", maSemafori(t))},
		"fase":                  {diretti("db.AllFaseValues()", maStringhe(db.AllFaseValues()))},
	}
	// Il testo libero noto (T-A1d-29, D-A1d-10): nessuna costante lo enumera.
	senzaSorgente := map[string]map[string]bool{"motivo_grafo": {"completezza non determinabile: nessun motivo dal caricatore": true}}

	var domini []string
	for d := range sorgenti {
		domini = append(domini, d)
	}
	sort.Strings(domini)
	for d := range etichetteMotoreA {
		if _, ok := sorgenti[d]; !ok {
			t.Errorf("il dominio %q della tabella non ha sorgenti nella prova", d)
		}
	}
	enumerati := 0
	for _, d := range domini {
		tabella, ok := etichetteMotoreA[d]
		if !ok {
			t.Errorf("il dominio %q manca dalla tabella", d)
			continue
		}
		visti := map[string]bool{}
		for _, s := range sorgenti[d] {
			vv := s.valori()
			if len(vv) == 0 {
				t.Errorf("%s: la sorgente %s non dà nessun valore (un nome cambiato?)", d, s.descr)
			}
			for _, v := range vv {
				visti[v] = true
				if _, ok := tabella[v]; !ok {
					t.Errorf("%s: %q (%s) non ha un'etichetta", d, v, s.descr)
				}
				if got := etichetta(d, v); strings.HasPrefix(got, "[") || got != tabella[v] {
					t.Errorf("etichetta(%q, %q) = %q", d, v, got)
				}
			}
		}
		enumerati += len(visti)
		for v := range tabella {
			if !visti[v] && !senzaSorgente[d][v] {
				t.Errorf("%s: la voce %q non viene da nessuna sorgente (un refuso, o un valore tolto dal codice?)", d, v)
			}
		}
	}
	if len(domini) != 62 {
		t.Errorf("%d domini nella prova, attesi 62 (61 più origine_categoria, RAC-11)", len(domini))
	}
	t.Logf("%d domini, %d valori enumerati dalle sorgenti", len(domini), enumerati)
}

// I testi della tabella: non vuoti, senza spazi ai bordi, senza HTML né quadre (le quadre dicono «sconosciuto»); ci
// sono i 22 domini del contratto §2.3, quelli della testa e origine_categoria (RAC-11).
func TestEtichettaTestiPuliti(t *testing.T) {
	for d, voci := range etichetteMotoreA {
		if len(voci) == 0 {
			t.Errorf("il dominio %q è vuoto", d)
		}
		for v, testo := range voci {
			if v == "" || testo == "" || testo != strings.TrimSpace(testo) || strings.ContainsAny(testo, "<>[]") {
				t.Errorf("%s/%q: testo %q", d, v, testo)
			}
		}
	}
	for _, d := range []string{
		"stato_prodotto", "motivo_prodotto", "stato_fonte", "motivo_fonte", "stato_verifica_asse", "stato_smistamento",
		"motivo_smistamento", "stato_documenti", "esito_fabbisogno", "motivo_fabbisogno", "disponibilita", "stato_struttura",
		"tipo_componente", "origine_tipo", "categoria", "origine_dato", "stato_nas", "badge", "motivo_non_congelabile",
		"motivo_thread", "tipo_conflitto", "perimetro",
		"stato_thread", "semaforo", "fase", "origine_categoria", "collocazione",
	} {
		if _, ok := etichetteMotoreA[d]; !ok {
			t.Errorf("manca il dominio %q", d)
		}
	}
	if len(etichetteMotoreA) != 62 {
		t.Errorf("%d domini, attesi 62", len(etichetteMotoreA))
	}
	// motivo_conflitto: le sue 6 voci più tutte quelle di tipo_documento, con gli stessi testi.
	mc := etichetteMotoreA["motivo_conflitto"]
	for v, testo := range etichetteMotoreA["tipo_documento"] {
		if mc[v] != testo {
			t.Errorf("motivo_conflitto/%q = %q, atteso %q (tipo_documento)", v, mc[v], testo)
		}
	}
	if len(mc) != 6+len(etichetteMotoreA["tipo_documento"]) {
		t.Errorf("motivo_conflitto ha %d voci", len(mc))
	}
}
