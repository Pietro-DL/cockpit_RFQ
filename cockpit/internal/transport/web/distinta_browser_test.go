//go:build integrazione && browser

// L7 — la Distinta (PR #6, #7, #8) in un BROWSER VERO, quando si confermano le cose (29/09).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7Distinta ./internal/transport/web/
//
// La scena e' fittizia ma ha la forma della mail difficile del 28/09 (quattro assiemi con molti pezzi e trenta
// file, «lo smistamento file e' critico»): un prodotto 7120001 con il suo STEP autorizzato, quattro assiemi
// 7120010…7120013 ciascuno con il suo STEP (guida) e il suo PDF, un quinto STEP 7120014 che non sta sotto il
// prodotto, otto pezzi con i figli in comune fra piu' assiemi (7121001 in due, 7121003 in tre), uno STEP e un
// PDF per pezzo, un DXF, un capitolato, un PDF d'assieme il cui cartiglio e' stato letto con il codice di un
// figlio e uno letto solo dal nome. Due forme della scena: con la sola distinta del prodotto (il lavoro del
// passo 2, «Distinta») e con la distinta gia' fatta (il passo 3, «Documenti e NAS»).
//
// Lo script del browser (e2e/distinta.py) guida la pagina come un operatore e dopo OGNI gesto guarda la
// pagina (niente errori, niente frammenti nel posto sbagliato, niente blocchi doppi o spariti, contatori
// coerenti, niente testi strani), il database (la «sonda» qui sotto: una fotografia delle tabelle della RFQ
// che lo script chiede e confronta con quella di prima: una riga giusta per gesto, niente di piu') e che
// ricaricando la pagina lo stato sia lo stesso. Le staging e il NAS stanno in cartelle temporanee della prova
// (con COCKPIT_BANCO_DISTINTA, dentro quella cartella: si possono guardare mentre la prova gira).
//
// Le prove che fissano un DIFETTO trovato il 29/09 (docs/specs/BUG_DISTINTA_29-09.md) sono lettere
// minuscole nello script: falliscono finche' il difetto c'e'. Con COCKPIT_DISTINTA_NOTI=1 lo script le
// esegue e scrive l'esito senza farle fallire (serve a vedere i difetti noti senza rompere la corsa).
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/testutil"
)

// famiglieDistinta: i codici 712xxxx del cliente di prova, perche' i nodi degli STEP si classifichino come codici.
const famiglieDistinta = `{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "codici 712", "esempio": "7120001"}]}`

// arcoStep e' un arco di uno STEP della scena: padre → figlio ×qta.
type arcoStep struct {
	padre, figlio string
	qta           int
}

// La distinta vera della scena (quella che l'operatore deve arrivare a costruire): il prodotto, quattro
// assiemi, otto pezzi. 7121001 sta sotto due assiemi, 7121003 sotto tre; 7121007 e' un particolare commerciale.
var (
	assiemiDistinta = []arcoStep{{"7120001", "7120010", 1}, {"7120001", "7120011", 2}, {"7120001", "7120012", 1}, {"7120001", "7120013", 1}}
	pezziDistinta   = []arcoStep{
		{"7120010", "7121001", 1}, {"7120010", "7121002", 1}, {"7120010", "7121003", 2},
		{"7120011", "7121003", 1}, {"7120011", "7121004", 2}, {"7120011", "7121005", 1},
		{"7120012", "7121003", 1}, {"7120012", "7121006", 1},
		{"7120013", "7121001", 1}, {"7120013", "7121007", 4}, {"7120013", "7121008", 1},
	}
	// il quinto STEP: un assieme che nella distinta del prodotto non c'e' (come uno STEP della mail vera)
	quintoDistinta = []arcoStep{{"7120014", "7121006", 1}, {"7120014", "7121009", 1}}
)

// fattiStep sono i fatti di uno STEP (analizzatore 3) con questi archi; la radice e' il primo padre.
func fattiStep(archi []arcoStep) string {
	chiavi := map[string]string{}
	var nodi []string
	chiave := func(codice string) string {
		if k, ok := chiavi[codice]; ok {
			return k
		}
		k := fmt.Sprintf("#%d", len(chiavi)+1)
		chiavi[codice] = k
		nodi = append(nodi, fmt.Sprintf(`{"chiave": %q, "id_grezzo": %q, "nome_grezzo": %q, "evidenza": {}}`, k, codice, codice))
		return k
	}
	var rel []string
	for _, a := range archi {
		p, f := chiave(a.padre), chiave(a.figlio)
		rel = append(rel, fmt.Sprintf(`{"padre": %q, "figlio": %q, "qta": %d, "evidenza": {}}`, p, f, a.qta))
	}
	return `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [], "nodi": [` + strings.Join(nodi, ", ") +
		`], "relazioni": [` + strings.Join(rel, ", ") + `], "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0,
		"occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
}

// fattiPezzo sono i fatti dello STEP di un pezzo solo, senza figli.
func fattiPezzo(codice string) string {
	return `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [], "nodi": [{"chiave": "#1", "id_grezzo": "` + codice +
		`", "nome_grezzo": "` + codice + `", "evidenza": {}}], "relazioni": [], "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0,
		"occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
}

// sottoDi sono gli archi dello STEP di un assieme: i suoi figli.
func sottoDi(assieme string, archi []arcoStep) []arcoStep {
	var out []arcoStep
	for _, a := range archi {
		if a.padre == assieme {
			out = append(out, a)
		}
	}
	return out
}

type scenaDistinta struct {
	*rfqFascicolo
	prodotto uuid.UUID
	pezzi    map[string]uuid.UUID // codice → componente (solo quelli che la scena scrive)
	proposte map[string]uuid.UUID // nome del file → proposta aperta
	sonda    *httptest.Server
	nas      string
}

// cartellaBanco e' una cartella temporanea per la scena (staging, NAS), tolta a fine prova: quella della prova
// (t.TempDir), oppure una dentro COCKPIT_BANCO_DISTINTA quando si vuole guardarla mentre la prova gira. Nessun
// percorso di una macchina scritto nel codice.
func cartellaBanco(t *testing.T, prefisso string) string {
	t.Helper()
	base := os.Getenv("COCKPIT_BANCO_DISTINTA")
	if base == "" {
		return t.TempDir()
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, prefisso+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// scenaDistinta costruisce la RFQ. Con distintaFatta la BOM ha gia' assiemi e pezzi (il passo 3); senza, c'e'
// solo il prodotto e il resto lo propongono gli STEP (il passo 2).
func (b *bancoWeb) scenaDistinta(t *testing.T, chiave string, distintaFatta bool) *scenaDistinta {
	return b.scenaDistintaCon(t, chiave, distintaFatta, distintaFatta)
}

// scenaDistintaCon: con chiudi, le proposte dello STEP autorizzato (i quattro assiemi sotto il prodotto, che
// nella distinta fatta ci sono gia') si chiudono come le chiude «Salva la distinta» del Fascicolo: la
// struttura di adesso, con le proposte viste. Senza, restano aperte (Bug 2 di BUG_DISTINTA_29-09.md).
func (b *bancoWeb) scenaDistintaCon(t *testing.T, chiave string, distintaFatta, chiudi bool) *scenaDistinta {
	t.Helper()
	ImpostaCapacitaProva(t, tutteAccese)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	r.staging = cartellaBanco(t, "staging")
	s := &scenaDistinta{rfqFascicolo: r, pezzi: map[string]uuid.UUID{}, proposte: map[string]uuid.UUID{}, nas: cartellaBanco(t, "nas")}
	r.fase("FATTIBILITA")
	// l'analizzatore corrente, come lo scrive il server all'avvio: senza, lo STEP del prodotto e' «non analizzato»
	r.esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)
		ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = EXCLUDED.versione_analizzatore, hash_configurazione = EXCLUDED.hash_configurazione`,
		an.Versione, an.Hash())
	r.esegui(`UPDATE cliente SET regole = $1 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $2)`, famiglieDistinta, r.thread)
	r.esegui(`UPDATE messaggio SET oggetto = 'I: Elenco codici ACME', mittente_indirizzo = 'buyer@acme.example' WHERE messaggio_id = $1`, r.msg)
	r.esegui(`UPDATE thread_offerta SET oggetto = 'I: Elenco codici ACME' WHERE thread_id = $1`, r.thread)
	b.caricamentoAcceso(t)
	b.ws.Staging = r.staging
	b.ws.NAS = &nas.Scrittore{Radice: s.nas}
	t.Cleanup(func() { b.ws.NAS = nil })

	s.prodotto = r.componenteTipo("7120001", "finito")
	s.pezzi["7120001"] = s.prodotto
	if distintaFatta {
		for _, a := range append(append([]arcoStep{}, assiemiDistinta...), pezziDistinta...) {
			if _, ok := s.pezzi[a.figlio]; !ok {
				tipo := "sciolto"
				switch {
				case strings.HasPrefix(a.figlio, "71200"):
					tipo = "sottoassieme"
				case a.figlio == "7121007":
					tipo = "commerciale"
				}
				s.pezzi[a.figlio] = r.componenteTipo(a.figlio, tipo)
			}
			r.arco(s.pezzi[a.padre], s.pezzi[a.figlio], a.qta)
		}
	}

	// il prodotto: il suo 2D sul NAS e il suo STEP, autorizzato, con tutta la struttura
	_, aPdf := r.documentoDa("ACME-7120001.pdf", "pdf", db.TipoDocumentoDisegno2d, "7120001", s.prodotto, db.StatoNasScritto)
	s.pdfVero(aPdf, "7120001")
	_, aStep := r.documentoDa("ACME-7120001 00 IN_WORK.stp", "stp", db.TipoDocumentoCad3d, "7120001", s.prodotto, db.StatoNasScritto)
	s.fatti(aStep, fattiStep(append(append([]arcoStep{}, assiemiDistinta...), pezziDistinta...)), an)

	// gli assiemi: uno STEP ciascuno (guida: nessuno e' autorizzato) e un PDF; il quinto STEP non sta sotto il prodotto
	for _, a := range assiemiDistinta {
		stp := s.file("ACME-"+a.figlio+" 00 IN_WORK.stp", "stp", "cad_3d", a.figlio, "00", "nome_file", 80, "")
		s.fatti(stp, fattiStep(sottoDi(a.figlio, pezziDistinta)), an)
	}
	stp := s.file("ACME-7120014 00 IN_WORK.stp", "stp", "cad_3d", "7120014", "00", "nome_file", 80, "")
	s.fatti(stp, fattiStep(quintoDistinta), an)
	s.pdf("ACME-7120010.pdf", "disegno_2d", "7120010", "cartiglio", 85, "")
	// il cartiglio del PDF d'assieme letto con il codice del primo pezzo dell'elenco (come un PDF d'assieme della mail vera).
	// E' una riga di prima, senza la valutazione nei dettagli: la fase 4.2 (piano.go, codiceSoloDalTesto) guarda la
	// valutazione, e su questa riga non scatta; la prova «l» (Bug 3) resta rossa finche' la 4.4 non toglie il gesto
	// cumulativo o la riga non viene rivalutata
	s.pdf("ACME-7120011.pdf", "disegno_2d", "7121003", "cartiglio", 85, "")
	// letto solo dal nome: il codice e' il nome del file, e non si sa che cos'e' (come un PDF della mail vera)
	s.pdf("ACME-7120012.pdf", "da_determinare", "ACME-7120012", "nome_file", 45, "")
	s.pdf("ACME-7120013.pdf", "disegno_2d", "7120013", "cartiglio", 85, "")

	// i pezzi: uno STEP e un PDF ciascuno (il commerciale solo lo STEP), un DXF, il capitolato
	pezzi := map[string]bool{}
	for _, a := range append(append([]arcoStep{}, pezziDistinta...), quintoDistinta...) {
		pezzi[a.figlio] = true
	}
	codici := make([]string, 0, len(pezzi))
	for c := range pezzi {
		codici = append(codici, c)
	}
	sort.Strings(codici)
	for _, c := range codici {
		// lo STEP di un pezzo e' analizzato: un nodo solo (senza, la preparazione della pagina lo accoderebbe)
		stp := s.file("ACME-"+c+" 00 IN_WORK.stp", "stp", "cad_3d", c, "00", "nome_file", 80, "")
		s.fatti(stp, fattiPezzo(c), an)
		if c != "7121007" {
			s.pdf("ACME-"+c+".pdf", "disegno_2d", c, "cartiglio", 85, "")
		}
	}
	s.file("ACME-7121005.dxf", "dxf", "sviluppo_dxf", "7121005", "", "nome_file", 80, "")
	s.pdf("Capitolato fornitura ACME.pdf", "altro", "", "estensione", 30, "")

	// le proposte degli STEP, come le scrive l'aggancio; poi l'autorizzazione dello STEP del prodotto
	if _, err := fascicolo.RileggiStepDellaRfq(b.ctx, b.q, r.thread, an); err != nil {
		t.Fatal(err)
	}
	testutil.AutorizzaStep(t, b.pool, r.thread, s.prodotto, aStep, r.utente)
	if _, err := fascicolo.RileggiStepDellaRfq(b.ctx, b.q, r.thread, an); err != nil {
		t.Fatal(err)
	}
	if chiudi {
		s.chiudiProposte(t)
	}
	s.sonda = httptest.NewServer(http.HandlerFunc(s.fotografia))
	t.Cleanup(s.sonda.Close)
	return s
}

// chiudiProposte manda la struttura di adesso, con le proposte e i ritrovati che l'editor mostra: e' «Salva la
// distinta» senza modifiche, e chiude le proposte che dicono gli archi che ci sono gia'.
func (s *scenaDistinta) chiudiProposte(t *testing.T) {
	t.Helper()
	w := operatore(s.b)
	_, js := w.fai(http.MethodGet, s.base()+"/bom/dati?prodotto="+s.prodotto.String(), nil, true)
	var d datiEditor
	if err := json.Unmarshal([]byte(js), &d); err != nil {
		t.Fatalf("bom/dati: %v: %s", err, js)
	}
	v := fascicolo.StrutturaVoluta{Radice: s.prodotto}
	for _, a := range d.Archi {
		x := fascicolo.ArcoVoluto{Padre: a.Padre, Figlio: a.Figlio, Qta: a.Qta}
		v.Archi, v.Visti = append(v.Archi, x), append(v.Visti, x)
	}
	for _, p := range d.Proposti {
		v.RelazioniViste = append(v.RelazioniViste, fascicolo.RelazioneVista{Allegato: p.Allegato, Padre: p.PK, Figlio: p.FK})
	}
	for _, r := range d.Ritrovati {
		v.RitrovatiVisti = append(v.RitrovatiVisti, r.Proposta)
	}
	j, _ := json.Marshal(v)
	_, html := w.daFascicolo(http.MethodPost, s.base()+"/bom/applica", url.Values{"struttura": {string(j)}}, s.thread, "")
	if a := avvisoF(html); strings.HasPrefix(a, "Niente") {
		t.Fatalf("la struttura di adesso non si salva: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM relazione_proposta WHERE thread_id = $1 AND stato = 'aperta' AND padre_chiave = '#1' AND allegato_id IN
		(SELECT allegato_id FROM documento_provenienza dp JOIN componente c ON c.step_strutturale_id = dp.documento_id)`, s.thread); n != 0 {
		t.Fatalf("dopo il salvataggio restano %d proposte aperte sotto il prodotto nello STEP del prodotto", n)
	}
}

// file e' un allegato arrivato con la sua proposta aperta.
func (s *scenaDistinta) file(nome, ext, tipo, codice, rev, fonte string, confidenza int, dettagli string) uuid.UUID {
	s.b.t.Helper()
	a, _ := s.allegatoExt(nome, ext)
	if dettagli == "" {
		dettagli = "{}"
	}
	var id uuid.UUID
	if err := s.b.pool.QueryRow(s.b.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8) RETURNING proposta_id`, a, s.thread, tipo, codice, rev, confidenza, fonte, dettagli).Scan(&id); err != nil {
		s.b.t.Fatal(err)
	}
	s.proposte[nome] = id
	return a
}

// pdf e' un file PDF vero (le miniature e il visore lo disegnano) con la sua proposta.
func (s *scenaDistinta) pdf(nome, tipo, codice, fonte string, confidenza int, dettagli string) {
	s.b.t.Helper()
	a := s.file(nome, "pdf", tipo, codice, "", fonte, confidenza, dettagli)
	s.pdfVero(a, strings.TrimSuffix(nome, ".pdf"))
}

func (s *scenaDistinta) pdfVero(allegato uuid.UUID, cartiglio string) {
	s.b.t.Helper()
	pdf := pdfTavola(cartiglio)
	var percorso string
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, allegato).Scan(&percorso); err != nil {
		s.b.t.Fatal(err)
	}
	if err := os.WriteFile(percorso, pdf, 0o644); err != nil {
		s.b.t.Fatal(err)
	}
	s.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, allegato, shaDi(pdf), len(pdf))
	s.esegui(`UPDATE documento SET sha256 = $2, bytes = $3 WHERE documento_id IN
		(SELECT documento_id FROM documento_provenienza WHERE allegato_id = $1)`, allegato, shaDi(pdf), len(pdf))
}

// fatti scrive i fatti correnti dello STEP (l'analisi fatta).
func (s *scenaDistinta) fatti(allegato uuid.UUID, fatti string, an coda.Analizzatore) {
	s.b.t.Helper()
	s.esegui(`UPDATE allegato SET estensione = 'stp' WHERE allegato_id = $1`, allegato)
	s.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti)
		SELECT sha256, $2, $3, $4 FROM allegato WHERE allegato_id = $1`, allegato, an.Versione, an.Hash(), fatti)
}

// fotografia e' la sonda: le righe della RFQ che un gesto della Distinta puo' toccare, in forma confrontabile.
func (s *scenaDistinta) fotografia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	elenco := func(sql string) []string {
		righe, err := s.b.pool.Query(ctx, sql, s.thread)
		if err != nil {
			panic(err)
		}
		defer righe.Close()
		out := []string{}
		for righe.Next() {
			var x string
			if err := righe.Scan(&x); err != nil {
				panic(err)
			}
			out = append(out, x)
		}
		sort.Strings(out)
		return out
	}
	mappa := func(sql string) map[string]string {
		righe, err := s.b.pool.Query(ctx, sql, s.thread)
		if err != nil {
			panic(err)
		}
		defer righe.Close()
		out := map[string]string{}
		for righe.Next() {
			var k, v string
			if err := righe.Scan(&k, &v); err != nil {
				panic(err)
			}
			out[k] = v
		}
		return out
	}
	foto := map[string]any{
		"componenti": elenco(`SELECT codice || '|' || tipo::text || CASE WHEN archiviato_il IS NULL THEN '' ELSE '|archiviato' END FROM componente WHERE thread_id = $1`),
		"relazioni": elenco(`SELECT p.codice || '>' || f.codice || 'x' || r.qta FROM componente_relazione r
			JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id WHERE r.thread_id = $1`),
		"documenti": elenco(`SELECT d.nome_file || '|' || d.tipo::text || '|' || coalesce(c.codice, '-') || '|' || d.stato_nas::text
			|| CASE WHEN d.sostituito_da IS NULL THEN '' ELSE '|sostituito' END
			FROM documento d LEFT JOIN componente c ON c.componente_id = d.componente_id WHERE d.thread_id = $1`),
		"proposte": mappa(`SELECT a.nome_file, p.stato::text || '|' || coalesce(c.codice, '-') || '|' || coalesce(p.codice, '') || '|' || p.tipo_proposto::text
			FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
			LEFT JOIN componente c ON c.componente_id = p.componente_id WHERE p.thread_id = $1`),
		"copie": mappa(`SELECT d.nome_file, count(j.job_id)::text FROM documento d
			JOIN job j ON j.chiave_idempotenza = 'nas:' || d.documento_id::text AND j.tipo = 'copia_nas'
			WHERE d.thread_id = $1 GROUP BY d.nome_file`),
		"provenienze": mappa(`SELECT a.nome_file, count(*)::text FROM documento_provenienza dp JOIN allegato a ON a.allegato_id = dp.allegato_id
			JOIN documento d ON d.documento_id = dp.documento_id WHERE d.thread_id = $1 GROUP BY a.nome_file`),
		"nodi":     mappa(`SELECT stato::text, count(*)::text FROM componente_proposta WHERE thread_id = $1 GROUP BY stato`),
		"archi":    mappa(`SELECT stato::text, count(*)::text FROM relazione_proposta WHERE thread_id = $1 GROUP BY stato`),
		"versioni": elenco(`SELECT numero::text || '|' || stato::text FROM bom_versione WHERE thread_id = $1`),
		"job":      mappa(`SELECT tipo::text, count(*)::text FROM job WHERE $1::uuid IS NOT NULL GROUP BY tipo`),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(foto)
}

// lanciaDistinta esegue lo script del browser sulle prove indicate.
func lanciaDistinta(t *testing.T, s *scenaDistinta, prove string) {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "distinta.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	pezzi := make([]string, 0, len(s.pezzi))
	for c, id := range s.pezzi {
		pezzi = append(pezzi, c+"="+id.String())
	}
	sort.Strings(pezzi)
	arg := []string{script, "--url", s.b.srv.URL, "--sonda", s.sonda.URL, "--ip", "10.0.0.5:51000", "--sigla", "FP", "--password", "prova-fp",
		"--thread", s.thread.String(), "--pezzi", strings.Join(pezzi, ","), "--prove", prove}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	if os.Getenv("COCKPIT_DISTINTA_NOTI") != "" {
		arg = append(arg, "--noti")
	}
	uscita, err := exec.Command(python, arg...).CombinedOutput()
	testo := strings.TrimRight(string(uscita), "\r\n")
	if err != nil {
		if strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("le prove nel browser sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
}

// Esplorazione: la pagina, passo per passo, fotografata (serve a scrivere le prove, non verifica niente). Le
// fotografie devono restare dopo la prova: vanno in COCKPIT_FOTO_DIR, o in «esplora» dentro
// COCKPIT_BANCO_DISTINTA; senza nessuna delle due non c'e' dove lasciarle.
func TestL7DistintaEsplora(t *testing.T) {
	if os.Getenv("COCKPIT_DISTINTA_ESPLORA") == "" {
		t.Skip("solo con COCKPIT_DISTINTA_ESPLORA=1")
	}
	if os.Getenv("COCKPIT_FOTO_DIR") == "" {
		base := os.Getenv("COCKPIT_BANCO_DISTINTA")
		if base == "" {
			t.Skip("serve COCKPIT_FOTO_DIR o COCKPIT_BANCO_DISTINTA: la cartella dove restano le fotografie")
		}
		t.Setenv("COCKPIT_FOTO_DIR", filepath.Join(base, "esplora"))
	}
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTX", os.Getenv("COCKPIT_DISTINTA_ESPLORA") == "fatta")
	lanciaDistinta(t, s, "Z")
}

// Il passo 3 con la distinta fatta: i gesti uno per uno, in sequenza sulla stessa RFQ.
func TestL7DistintaDocumenti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTD", true)
	lanciaDistinta(t, s, "FGHIJKPLN")
}

// «Indietro» e «Avanti» fra i passi dopo un gesto.
func TestL7DistintaIndietroAvanti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTO", true)
	lanciaDistinta(t, s, "O")
}

// Due schede sulla stessa RFQ, nel passo 3.
func TestL7DistintaDueSchede(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTM", true)
	lanciaDistinta(t, s, "M")
}

// Difetti del passo 3 (BUG_DISTINTA_29-09.md): falliscono finche' ci sono.
func TestL7DistintaDifettiDocumenti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTK", true)
	lanciaDistinta(t, s, "lhpkqjso")
}

// Il passo 2 con la sola distinta del prodotto: la struttura dagli STEP.
func TestL7DistintaStruttura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTA", false)
	lanciaDistinta(t, s, "A")
}

// Difetti del passo 2 (BUG_DISTINTA_29-09.md): falliscono finche' ci sono.
func TestL7DistintaDifettiStruttura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTB", false)
	lanciaDistinta(t, s, "gra")
}

// Il passo 2 con la distinta fatta: trascinare e salvare, due schede.
func TestL7DistintaTrascina(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTE", true)
	lanciaDistinta(t, s, "BDE")
}

// Difetti del passo 2 con la distinta fatta: le proposte dello STEP del prodotto restano aperte (c), un pezzo
// tolto dalla distinta e' «il prodotto» nel passo 3 (d).
func TestL7DistintaDifettiDistintaFatta(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistintaCon(t, "DSTC", true, false)
	lanciaDistinta(t, s, "cd")
}

// Difetto con due schede (Bug 7): il rifiuto di «Metti da parte» e' mostrato come riuscito.
func TestL7DistintaDifettiDueSchede(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistinta(t, "DSTN", true)
	lanciaDistinta(t, s, "n")
}
