// L1 — la guardia statica del caricatore e le conversioni (A1c-L1-04, riscritta per le 29 query del contratto di A1c,
// §3.4; piano A, 6.4.2; modello: classificazione/scrittori_test.go). Dai sorgenti, con go/parser: i metodi di
// db.Queries chiamati sono esattamente le 29 query dell'elenco chiuso, e il loro SQL generato è solo lettura, senza
// lucchetti, senza le tabelle escluse e senza password_hash né regole; db.New riceve solo la transazione e il pool
// non esegue niente; SQL scritto a mano c'è solo per i due SHOW e per SELECT now(). Poi le conversioni, da righe sqlc
// sintetiche ai tipi puri: ogni campo del contratto arriva, i NULL restano nil, i tempi diventano UTC al
// millisecondo, di evidenza entrano solo strutturale e albero.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID di prova): il repository è pubblico.

package caricatore

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/db"
)

// queryDelContratto: l'elenco chiuso del contratto di A1c, §3.4 (17 + 6 + 6 = 29).
var queryDelContratto = []string{
	// riusate, già nel 6.4.2 (17)
	"GetAnalizzatoreCorrente", "ListRfqPerLaRiapertura", "GetThread", "ListMessaggiThread", "ListAllegatiThread",
	"ListProposteDocumentoThread", "ListDocumentiThread", "ListProvenienzeThread", "ListIdentificativi",
	"ListComponentiThread", "ListRelazioniDellaRfq", "ListComponenteProposteThread", "ListRelazioneProposteThread",
	"GetUltimaVersione", "ListLavoroPendenteRfq", "GetMessaggio", "ListAllegatiMessaggio",
	// riusate, nuove nell'elenco (6)
	"ListStepProdotto", "ListFascicolo", "ListDerogheThread", "ListRimozioniAperte", "ListFabbisognoEffettivo",
	"GetUltimaCongelata",
	// nuove, queries/fotografia.sql (6)
	"ListFattiDelThread", "ListFattiPerSha", "ListClientiDellaFotografia", "ListSigleUtenti",
	"ListCandidatiCodiceThread", "ListTriageThread",
}

// sqlAMano: l'unico SQL scritto a mano nel caricatore, per nome della costante e testo.
var sqlAMano = map[string]string{
	"sqlSolaLettura": "SHOW transaction_read_only",
	"sqlIsolamento":  "SHOW transaction_isolation",
	"sqlAdesso":      "SELECT now()",
}

func cartellaDB(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "platform", "db")
}

// metodiDiQueries: i nomi dei metodi di *db.Queries, letti dai file generati.
func metodiDiQueries(t *testing.T) map[string]bool {
	t.Helper()
	file, err := filepath.Glob(filepath.Join(cartellaDB(t), "*.go"))
	if err != nil || len(file) == 0 {
		t.Fatalf("i file di platform/db non si trovano: %v", err)
	}
	out := map[string]bool{}
	for _, p := range file {
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if st, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := st.X.(*ast.Ident); ok && id.Name == "Queries" {
					out[fn.Name.Name] = true
				}
			}
		}
	}
	return out
}

// sorgentiDelCaricatore: i file non di prova del pacchetto, analizzati.
func sorgentiDelCaricatore(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	nomi, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []*ast.File
	for _, n := range nomi {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	if len(out) < 2 {
		t.Fatalf("file del caricatore trovati: %d", len(out))
	}
	return fset, out
}

func TestIlCaricatoreUsaSoloLeQueryDelContratto(t *testing.T) {
	if len(queryDelContratto) != 29 {
		t.Fatalf("l'elenco del contratto ha %d query, il contratto ne dice 29", len(queryDelContratto))
	}
	tutte := metodiDiQueries(t)
	for _, q := range queryDelContratto {
		if !tutte[q] {
			t.Errorf("la query %s del contratto non esiste in platform/db", q)
		}
	}
	fset, file := sorgentiDelCaricatore(t)
	usate := map[string]bool{}
	nuovi := 0
	for _, f := range file {
		for _, is := range f.Imports {
			if imp, _ := strconv.Unquote(is.Path.Value); strings.HasSuffix(imp, "/pgxpool") {
				t.Errorf("%s importa %s: il caricatore riceve un Iniziatore, il pool non esegue query", fset.Position(is.Pos()), imp)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			dove := fset.Position(c.Pos())
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "db" && sel.Sel.Name == "New" {
				nuovi++
				if len(c.Args) != 1 {
					t.Errorf("%s: db.New con %d argomenti", dove, len(c.Args))
				} else if a, ok := c.Args[0].(*ast.Ident); !ok || a.Name != "tx" {
					t.Errorf("%s: db.New riceve qualcosa che non è la transazione", dove)
				}
				return true
			}
			if tutte[sel.Sel.Name] {
				usate[sel.Sel.Name] = true
			}
			switch sel.Sel.Name {
			case "QueryRow", "Query", "Exec":
				if len(c.Args) < 2 {
					t.Errorf("%s: %s senza testo SQL", dove, sel.Sel.Name)
					return true
				}
				if id, ok := c.Args[1].(*ast.Ident); !ok || sqlAMano[id.Name] == "" {
					t.Errorf("%s: %s con un SQL che non è dei tre ammessi (i due SHOW, SELECT now())", dove, sel.Sel.Name)
				}
			}
			return true
		})
	}
	if nuovi == 0 {
		t.Error("nessun db.New: la prova non legge i file giusti")
	}
	var fuori, mancanti []string
	for q := range usate {
		if !slices.Contains(queryDelContratto, q) {
			fuori = append(fuori, q)
		}
	}
	for _, q := range queryDelContratto {
		if !usate[q] {
			mancanti = append(mancanti, q)
		}
	}
	slices.Sort(fuori)
	if len(fuori) > 0 {
		t.Errorf("query fuori dall'elenco chiuso del contratto: %v", fuori)
	}
	if len(mancanti) > 0 {
		t.Errorf("query dell'elenco che il caricatore non usa (l'elenco va tenuto uguale al codice): %v", mancanti)
	}
}

// I testi a mano: le costanti valgono i tre testi ammessi, e nessun'altra stringa del caricatore è SQL.
func TestIlCaricatoreScriveAManoSoloDueShowEUnOra(t *testing.T) {
	fset, file := sorgentiDelCaricatore(t)
	// una stringa è SQL se comincia con un comando; i messaggi d'errore in italiano non cominciano così
	sql := regexp.MustCompile(`(?i)^\s*(select|insert|update|delete|show|begin|commit|rollback|lock|truncate|copy|with|set)\s`)
	trovate := map[string]string{}
	for _, f := range file {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if _, ok := sqlAMano[n.Name]; ok && i < len(vs.Values) {
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
							trovate[n.Name], _ = strconv.Unquote(lit.Value)
						}
					}
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, _ := strconv.Unquote(lit.Value)
			ammessa := false
			for _, testo := range sqlAMano {
				ammessa = ammessa || v == testo
			}
			if !ammessa && sql.MatchString(v) {
				t.Errorf("%s: una stringa che sembra SQL scritto a mano: %q", fset.Position(lit.Pos()), v)
			}
			return true
		})
	}
	if !reflect.DeepEqual(trovate, sqlAMano) {
		t.Errorf("le costanti dell'SQL a mano: %v, attese %v", trovate, sqlAMano)
	}
}

// sqlGenerato: il testo delle query generate, per nome.
func sqlGenerato(t *testing.T) map[string]string {
	t.Helper()
	file, _ := filepath.Glob(filepath.Join(cartellaDB(t), "*.sql.go"))
	nome := regexp.MustCompile(`^-- name: (\w+) :`)
	out := map[string]string{}
	for _, p := range file {
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, s := range g.Specs {
				for _, v := range s.(*ast.ValueSpec).Values {
					lit, ok := v.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					testo, _ := strconv.Unquote(lit.Value)
					if m := nome.FindStringSubmatch(testo); m != nil {
						out[m[1]] = testo
					}
				}
			}
		}
	}
	return out
}

// Il testo delle 29 query: solo letture, senza lucchetti, senza le tabelle escluse né le colonne vietate (contratto
// §3.4; piano A, 6.5: «Perché sono sicure in READ ONLY»).
func TestLeQueryDelCaricatoreSonoSoloLetture(t *testing.T) {
	testi := sqlGenerato(t)
	vietato := regexp.MustCompile(`(?i)\b(insert|update|delete|truncate|copy|lock)\b|\bfor\s+(no\s+key\s+update|key\s+share|share|update)\b` +
		`|analisi` + `_messaggio|worker_credenziale|\bsessione\b|password_hash|\bregole\b|messaggio_aggancio_log`)
	for _, q := range queryDelContratto {
		testo, ok := testi[q]
		if !ok {
			t.Errorf("%s: il testo generato non si trova", q)
			continue
		}
		corpo := strings.TrimSpace(strings.SplitN(testo, "\n", 2)[1])
		if inizio := strings.ToUpper(strings.Fields(corpo)[0]); inizio != "SELECT" && inizio != "WITH" {
			t.Errorf("%s: non comincia con SELECT o WITH: %q", q, inizio)
		}
		if m := vietato.FindString(corpo); m != "" {
			t.Errorf("%s: il testo contiene %q", q, m)
		}
	}
	// le tre query che leggono tabelle con colonne vietate le nominano una per una, mai con *
	for _, q := range []string{"ListClientiDellaFotografia", "ListSigleUtenti", "ListTriageThread", "ListCandidatiCodiceThread"} {
		if strings.Contains(testi[q], "*") {
			t.Errorf("%s: legge con *, le colonne vanno nominate", q)
		}
	}
	for _, c := range []struct{ q, filtro string }{
		{"ListCandidatiCodiceThread", "c.origine <> 'agente'"},
		{"ListTriageThread", "pt.fonte = 'deterministico'"},
	} {
		if !strings.Contains(testi[c.q], c.filtro) {
			t.Errorf("%s: manca il filtro %q", c.q, c.filtro)
		}
	}
}

// ---- le conversioni ----

func id(n int) uuid.UUID {
	u := uuid.MustParse("00000000-0000-4000-8000-000000000000")
	u[14], u[15] = byte(n>>8), byte(n)
	return u
}

func nid(n int) uuid.NullUUID  { return uuid.NullUUID{UUID: id(n), Valid: true} }
func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

// istante: un tempo con i microsecondi e un fuso diverso da UTC, come può arrivare da pgx.
var istante = time.Date(2026, 10, 6, 10, 0, 0, 123456789, time.FixedZone("CEST", 7200))

func pistante() *time.Time { t := istante; return &t }

// atteso: lo stesso istante, UTC al millisecondo.
var atteso = time.Date(2026, 10, 6, 8, 0, 0, 123000000, time.UTC)

// pieno controlla che nessun campo del record convertito sia vuoto: la riga di partenza ha tutto pieno, quindi un
// campo vuoto è un campo perso dalla conversione.
func pieno(t *testing.T, cosa string, v any) {
	t.Helper()
	rv := reflect.ValueOf(v)
	for i := 0; i < rv.NumField(); i++ {
		if rv.Field(i).IsZero() {
			t.Errorf("%s.%s: vuoto dopo la conversione (campo perso)", cosa, rv.Type().Field(i).Name)
		}
	}
}

// tempiUTC controlla, con la riflessione, che ogni time.Time del record sia UTC al millisecondo.
func tempiUTC(t *testing.T, cosa string, v any) {
	t.Helper()
	rv := reflect.ValueOf(v)
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		var tm time.Time
		switch x := f.Interface().(type) {
		case time.Time:
			tm = x
		case *time.Time:
			if x == nil {
				continue
			}
			tm = *x
		default:
			continue
		}
		if tm.Location() != time.UTC || !tm.Equal(atteso) {
			t.Errorf("%s.%s: %v, atteso %v (UTC al millisecondo)", cosa, rv.Type().Field(i).Name, tm, atteso)
		}
	}
}

func TestLeConversioniNonPerdonoCampi(t *testing.T) {
	m := db.Messaggio{
		MessaggioID: id(1), Canale: "outlook", ConversazioneID: id(2), ParentMessaggioID: nid(3), ThreadID: nid(4),
		Aggancio: "operatore", AgganciatoDa: nid(5), AgganciatoIl: pistante(), Direzione: "entrata", DataEvento: istante,
		MittenteNome: txt("Ufficio acquisti ACME"), MittenteIndirizzo: txt("acquisti@acme.example"), Oggetto: txt("RFQ ACME26-030"),
		CorpoTesto: txt("corpo"), CorpoHtml: txt("<p>corpo</p>"), Interno: true, ControparteTipo: "cliente", ControparteClienteID: nid(6),
	}
	mm := messaggio(m)
	pieno(t, "Messaggio", mm)
	tempiUTC(t, "Messaggio", mm)
	a := aggancio(m)
	pieno(t, "AggancioMessaggio", a)
	tempiUTC(t, "AggancioMessaggio", a)

	al := allegato(db.Allegato{AllegatoID: id(10), MessaggioID: id(1), ContenitoreID: nid(11), Indice: 2, NomeFile: "7120002.pdf",
		PathInterno: txt("disegni/7120002.pdf"), Estensione: txt("pdf"), ContentType: txt("application/pdf"), Natura: "file",
		Origine: "outlook", Bytes: pgtype.Int8{Int64: 100, Valid: true}, Sha256: txt("ab"), Stato: "analizzato", RicevutoIl: istante})
	pieno(t, "Allegato", al)
	tempiUTC(t, "Allegato", al)

	doc := documento(db.Documento{DocumentoID: id(20), ThreadID: id(4), ComponenteID: nid(21), Tipo: "disegno_2d", Codice: txt("7120002"),
		Rev: txt("1"), NomeFile: "7120002.pdf", Estensione: "pdf", Sha256: "ab", StatoNas: "scritto", ConfermatoDa: id(5),
		ConfermatoIl: istante, SostituitoDa: nid(22)}, []uuid.UUID{id(10)})
	pieno(t, "DocumentoConfermato", doc)
	tempiUTC(t, "DocumentoConfermato", doc)

	pr := proposta(db.DocumentoProposta{PropostaID: id(30), AllegatoID: id(10), ThreadID: nid(4), TipoProposto: "disegno_2d", Codice: txt("7120002"),
		Rev: txt("1"), ComponenteID: nid(21), Fonte: "operatore", Stato: "confermata", DecisoDa: nid(5), DecisoIl: pistante(),
		Dettagli: json.RawMessage(`{"destinazione": {"esito": "figlio"}}`)})
	pieno(t, "PropostaAttuale", pr)
	tempiUTC(t, "PropostaAttuale", pr)

	idf := identificativo(db.IdentificativoThread{Codice: "7120001", Origine: "manuale", Confidenza: pgtype.Int2{Int16: 90, Valid: true},
		ConfermatoDa: nid(5), CreatoIl: istante})
	pieno(t, "Identificativo", idf)

	co := componente(db.Componente{ComponenteID: id(21), Codice: "7120002", Rev: txt("1"), Descrizione: txt("staffa"), Tipo: "sciolto",
		Origine: "step", ConfermatoDa: id(5), CreatoIl: istante, ArchiviatoIl: pistante(), StepStrutturaleID: nid(23)})
	pieno(t, "Componente", co)
	tempiUTC(t, "Componente", co)

	re := relazione(db.ComponenteRelazione{PadreID: id(24), FiglioID: id(21), Qta: 2, Posizione: txt("10"), Origine: "step",
		ConfermatoDa: id(5), CreatoIl: istante})
	pieno(t, "Relazione", re)

	evid := json.RawMessage(`{"classificato": {"x": 1}, "storia": [1],
		"strutturale": {"v": 1, "ruolo": "radice", "componente_id": "` + id(24).String() + `", "documento_id": "` + id(23).String() + `",
			"dichiarato_da": "` + id(5).String() + `", "dichiarato_il": "2026-10-06T08:00:00Z", "presa_d_atto": "2026-10-06T08:01:00Z",
			"sospesa": {"motivo": "commerciale"}},
		"albero": {"da": "` + id(5).String() + `", "il": "2026-10-06T08:02:00Z", "firma": "f1", "padre": "` + id(24).String() + `",
			"figlio": "` + id(21).String() + `", "nodo": "cod:7120002", "commerciale": {"risposta": "no"}}}`)
	rc, err := rigaComponente(db.ListComponenteProposteThreadRow{NomeFile: "ACME-030P7120001.stp", ComponenteProposta: db.ComponenteProposta{
		PropostaID: id(40), ThreadID: id(4), AllegatoID: id(10), Sha256: "ab", Chiave: "#2", NomeGrezzo: "7120002", IDGrezzo: "ACME-030P7120002",
		Descrizione: txt("staffa"), Codice: txt("7120002"), Rev: txt("1"),
		OrigineCodice: db.NullOrigineCodice{OrigineCodice: "operatore", Valid: true}, Famiglia: "acme",
		TipoProposto: db.NullTipoComponente{TipoComponente: "sciolto", Valid: true}, Fonte: "step", Evidenza: evid, Stato: "confermata",
		ComponenteID: nid(21), DecisoDa: nid(5), DecisoIl: pistante(), Nota: txt("nota"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	pieno(t, "RigaComponenteProposta", rc)
	tempiUTC(t, "RigaComponenteProposta", rc)
	pieno(t, "MarcaturaStrutturale", *rc.Marcatura)
	pieno(t, "SegnoAlbero", *rc.Albero)
	if !rc.Marcatura.Sospesa || rc.Marcatura.Ruolo != "radice" || *rc.Albero.Figlio != id(21) {
		t.Errorf("evidenza letta male: %+v %+v", rc.Marcatura, rc.Albero)
	}

	rr, err := rigaRelazione(db.ListRelazioneProposteThreadRow{NomeFile: "ACME-030P7120001.stp", RelazioneProposta: db.RelazioneProposta{
		ThreadID: id(4), AllegatoID: id(10), PadreChiave: "#1", FiglioChiave: "#2", Qta: 2, Evidenza: evid, Stato: "confermata",
		Nota: txt("nota"), DecisoDa: nid(5), DecisoIl: pistante()}})
	if err != nil {
		t.Fatal(err)
	}
	pieno(t, "RigaRelazioneProposta", rr)

	sp := rigaStepProdotto(db.VStepProdotto{ComponenteID: id(24), Codice: "7120001", StepStrutturaleID: nid(23), NStepCorrenti: 1,
		AnalisiCompleta: true, MotivoParziale: txt("troncata"), DerogaStrutturaID: nid(25), Altro3dDocumentoID: nid(26),
		PropostaAperta: nid(27), AttesoDaPortale: nid(28), Esito: "confermato"})
	pieno(t, "RigaStepProdotto", sp)

	fa := rigaFascicolo(db.VFascicolo{ComponenteID: id(21), Codice: "7120002", Rev: txt("1"), TipoComponente: "sciolto",
		TipoDocumento: "disegno_2d", Bloccante: true, DocumentoID: nid(20), StatoNas: txt("scritto"), PathRelativo: txt("non si porta"),
		PropostaAperta: nid(30), AttesoDaPortale: nid(28), DerogaID: nid(29), Esito: "ok",
		FonteAttesa: db.NullFonteFabbisogno{FonteFabbisogno: "cliente", Valid: true}, NProposteAperte: 1, DocumentoRev: txt("1"),
		RevDiversa: true, AnomaliaID: pgtype.Int8{Int64: 7, Valid: true}})
	pieno(t, "RigaFascicolo", fa)

	tr := triage(db.ListTriageThreadRow{TriageID: id(50), MessaggioID: id(1), Esito: "nuova_rfq", Atto: txt("richiesta"),
		Legame: db.NullLegameOperativo{LegameOperativo: "rfq", Valid: true}, Stato: "accettata", Identificativi: []string{"7120001"},
		CreatoIl: istante, DecisoIl: pistante()})
	pieno(t, "Triage", tr)
	tempiUTC(t, "Triage", tr)

	vb := versioneBOM(&db.BomVersione{BomVersioneID: id(60), Numero: 1, Stato: "congelata", Contesto: "preventivo",
		CongelataDa: nid(5), CongelataIl: pistante()})
	pieno(t, "VersioneBOM", *vb)
	tempiUTC(t, "VersioneBOM", *vb)
	if versioneBOM(nil) != nil {
		t.Error("nessuna versione della BOM: atteso nil")
	}

	fx, err := fattiDi("ab", 4, "cfg", json.RawMessage(`{"struttura": {}}`), istante, true, "troncata")
	if err != nil {
		t.Fatal(err)
	}
	pieno(t, "Fatti", fx)
	if d, _ := fotorfq.ImprontaPayload(fx.Payload); fx.Digest != d || *fx.MotivoParziale != "troncata" || !fx.CalcolatoIl.Equal(atteso) {
		t.Errorf("fatti: %+v", fx)
	}
}

// I NULL del DB restano nil, mai valori vuoti; i fatti senza struttura non hanno il motivo; il motivo «completa» è
// il testo vuoto, non nil.
func TestINullRestanoNil(t *testing.T) {
	m := messaggio(db.Messaggio{MessaggioID: id(1), DataEvento: istante})
	if m.ThreadID != nil || m.ParentID != nil || m.Oggetto != nil || m.CorpoTesto != nil || m.CorpoHTML != nil || m.ControparteClienteID != nil {
		t.Errorf("messaggio con i NULL: %+v", m)
	}
	if a := aggancio(db.Messaggio{MessaggioID: id(1), Aggancio: "auto_conversazione"}); a.AgganciatoDa != nil || a.AgganciatoIl != nil {
		t.Errorf("aggancio automatico: %+v", a)
	}
	c := componente(db.Componente{ComponenteID: id(2), Codice: "7120001"})
	if c.Rev != nil || c.Descrizione != nil || c.ArchiviatoIl != nil || c.StepStrutturaleID != nil {
		t.Errorf("componente con i NULL: %+v", c)
	}
	if i := identificativo(db.IdentificativoThread{Codice: "7120001"}); i.Confidenza != nil || i.ConfermatoDa != nil {
		t.Errorf("identificativo con i NULL: %+v", i)
	}
	if p := proposta(db.DocumentoProposta{PropostaID: id(3)}); p.Dettagli != nil || p.ComponenteID != nil || p.DecisoIl != nil {
		t.Errorf("proposta con i NULL: %+v", p)
	}
	if tr := triage(db.ListTriageThreadRow{TriageID: id(4)}); tr.Atto != "" || tr.Legame != "" || tr.DecisoIl != nil {
		t.Errorf("triage con i NULL: %+v", tr)
	}
	senza, err := fattiDi("ab", 4, "cfg", json.RawMessage(`{"testo_pdf": {}}`), istante, false, "")
	if err != nil || senza.MotivoParziale != nil {
		t.Errorf("fatti senza struttura: %+v %v", senza, err)
	}
	completa, err := fattiDi("ab", 4, "cfg", json.RawMessage(`{"struttura": {}}`), istante, true, "")
	if err != nil || completa.MotivoParziale == nil || *completa.MotivoParziale != "" {
		t.Errorf("fatti con la struttura completa: %+v %v", completa, err)
	}
	if _, err := fattiDi("ab", 4, "cfg", nil, istante, false, ""); err == nil {
		t.Error("fatti senza payload: un'impronta inventata")
	}
}

// Di evidenza entrano solo strutturale e albero: chiavi assenti o null danno nil; una forma sbagliata è un errore.
func TestLEvidenzaDellaRiga(t *testing.T) {
	for _, raw := range []string{``, `{}`, `null`, `{"classificato": {"x": 1}, "storia": []}`, `{"strutturale": null, "albero": null}`} {
		m, a, err := evidenzaDellaRiga(json.RawMessage(raw))
		if err != nil || m != nil || a != nil {
			t.Errorf("evidenza %q: %+v %+v %v", raw, m, a, err)
		}
	}
	m, _, err := evidenzaDellaRiga(json.RawMessage(`{"strutturale": {"v": 1, "ruolo": "delega", "componente_id": null}}`))
	if err != nil || m == nil || m.Sospesa || m.ComponenteID != nil || m.Ruolo != "delega" {
		t.Errorf("marcatura senza sospensione e senza componente: %+v %v", m, err)
	}
	for _, raw := range []string{
		`[]`,
		`{"strutturale": {"componente_id": 7}}`,
		`{"strutturale": {"componente_id": "non-un-uuid"}}`,
		`{"albero": {"il": "2026-10-06", "firma": "f"}}`,
		`{"albero": "testo"}`,
	} {
		if _, _, err := evidenzaDellaRiga(json.RawMessage(raw)); err == nil {
			t.Errorf("evidenza %q accettata", raw)
		}
	}
}

// componi: tutte le sezioni del contratto, i fatti assenti senza analizzatore con la diagnosi, la fotografia
// ordinata, gli allegati in attesa senza ripetizioni.
func TestComponiLaFotografia(t *testing.T) {
	l := letture{schema: 21, presaIl: istante, thread: []letturaThread{{
		thread: db.ThreadOfferta{ThreadID: id(4), ClienteID: id(6), Stato: "APERTA", CreatoIl: istante},
		pendenti: []db.ListLavoroPendenteRfqRow{
			{JobID: 1, AllegatoID: id(11)}, {JobID: 2, AllegatoID: id(10)}, {JobID: 3, AllegatoID: id(11)},
		},
		provenienze: []db.ListProvenienzeThreadRow{{DocumentoID: id(20), AllegatoID: nid(12)}, {DocumentoID: id(20), AllegatoID: nid(10)}},
		documenti:   []db.Documento{{DocumentoID: id(20), ThreadID: id(4), ConfermatoIl: istante}},
	}}, utenti: []db.ListSigleUtentiRow{{UtenteID: id(9), Sigla: "AB"}, {UtenteID: id(8), Sigla: "AC"}}}
	f, err := componi(l, "127.0.0.1:5432/acme_prova_test come prove_acme")
	if err != nil {
		t.Fatal(err)
	}
	if f.VersioneSchema != fotorfq.VersioneSchema || f.Origine != fotorfq.OrigineDSN || !f.Coerente || f.SchemaDB != 21 ||
		f.SolaLettura != "on" || f.Isolamento != "repeatable read" || !f.PresaIl.Equal(atteso) {
		t.Errorf("testata: %+v", f)
	}
	for _, k := range fotorfq.ChiaviSezioni {
		s, ok := f.Sezioni[k]
		switch {
		case !ok:
			t.Errorf("sezione %s assente", k)
		case k == fotorfq.SezioneFatti && s.Stato != fotorfq.StatoSezioneAssente:
			t.Errorf("senza analizzatore la sezione dei fatti è %+v", s)
		case k != fotorfq.SezioneFatti && s.Stato != fotorfq.StatoSezioneCompleta:
			t.Errorf("sezione %s: %+v", k, s)
		}
	}
	if len(f.Sezioni) != 21 {
		t.Errorf("sezioni: %d, il contratto ne elenca 21", len(f.Sezioni))
	}
	if f.Analizzatore != nil || len(f.Diagnostiche) != 1 || f.Diagnostiche[0].Codice != fotorfq.CodiceNessunAnalizzatore {
		t.Errorf("senza analizzatore: %+v %+v", f.Analizzatore, f.Diagnostiche)
	}
	th := f.Thread[0]
	if !slices.Equal(th.InAttesa, []uuid.UUID{id(10), id(11)}) {
		t.Errorf("allegati in attesa: %v", th.InAttesa)
	}
	if !slices.Equal(th.Documenti[0].Allegati, []uuid.UUID{id(10), id(12)}) {
		t.Errorf("provenienze del documento: %v", th.Documenti[0].Allegati)
	}
	if f.Utenti[0].ID != id(8) {
		t.Errorf("utenti non ordinati: %v", f.Utenti)
	}

	l.analizzatore = &db.AnalizzatoreCorrente{VersioneAnalizzatore: 4, HashConfigurazione: "cfg"}
	if f, err = componi(l, ""); err != nil || f.Analizzatore == nil || f.Analizzatore.Versione != 4 || len(f.Diagnostiche) != 0 ||
		f.Sezioni[fotorfq.SezioneFatti].Stato != fotorfq.StatoSezioneCompleta {
		t.Errorf("con l'analizzatore: %+v %v", f, err)
	}
	if _, err := os.Stat("README.md"); err != nil {
		t.Errorf("il pacchetto non ha il README: %v", err)
	}
}
