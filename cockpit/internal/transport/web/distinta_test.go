package web

// L1 — la Distinta (PR #6, #7, #8), le funzioni pure della pagina, portate nel ramo delle prove con la fase 4.3 del
// giro 4: dallaDistinta (da dove viene un gesto e su quale passo si torna), passiDistinta (lo stato di ogni
// linguetta in una riga) e documentiDistinta (quale file va in quale gruppo del passo «Documenti e NAS»). Le prove
// con il database (le GET non scrivono, il codice solo dal testo del PDF) stanno in distinta_db_test.go.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// Un gesto viene dalla Distinta quando HX-Current-URL e' la pagina di QUESTA RFQ (con o senza la barra in fondo):
// il passo e' quello dell'indirizzo, e un passo che non esiste torna alla Distinta. La pagina di un'altra RFQ, il
// Fascicolo, i dati della pagina, un indirizzo vuoto o rotto non vengono dalla Distinta: il gesto risponde come
// prima.
func TestDallaDistintaRiconosceLaPaginaEIlPasso(t *testing.T) {
	id := uuid.New()
	base := "http://127.0.0.1:8080/thread/" + id.String()
	casi := []struct {
		url   string
		passo string
		ok    bool
	}{
		{base + "/distinta", "distinta", true},
		{base + "/distinta/", "distinta", true},
		{base + "/distinta?passo=richiesta", "richiesta", true},
		{base + "/distinta?passo=documenti", "documenti", true},
		{base + "/distinta?passo=fattibilita&x=1", "fattibilita", true},
		{base + "/distinta?passo=boh", "distinta", true},
		{"/thread/" + id.String() + "/distinta?passo=documenti", "documenti", true},
		{"http://127.0.0.1:8080/thread/" + uuid.New().String() + "/distinta", "", false},
		{base + "/fascicolo", "", false},
		{base + "/distinta/dati", "", false},
		{base, "", false},
		{"", "", false},
		{"%zz", "", false},
	}
	for _, c := range casi {
		h := http.Header{}
		if c.url != "" {
			h.Set("HX-Current-URL", c.url)
		}
		passo, ok := dallaDistinta(h, id)
		if ok != c.ok || passo != c.passo {
			t.Errorf("%q: %q %v, atteso %q %v", c.url, passo, ok, c.passo, c.ok)
		}
	}
	// i passi validi, nell'ordine del lavoro; tutto il resto e' la Distinta
	var chiavi []string
	for _, p := range passiDistinta {
		chiavi = append(chiavi, p.Chiave)
		if passoValido(p.Chiave) != p.Chiave {
			t.Errorf("passoValido(%q) = %q", p.Chiave, passoValido(p.Chiave))
		}
	}
	if strings.Join(chiavi, " ") != "richiesta distinta documenti fattibilita" {
		t.Errorf("i passi: %v", chiavi)
	}
	for _, p := range []string{"", "DOCUMENTI", "fascicolo", "../documenti"} {
		if passoValido(p) != "distinta" {
			t.Errorf("passoValido(%q) = %q, atteso distinta", p, passoValido(p))
		}
	}
}

// componenteDistinta e' un componente attivo di prova.
func componenteDistinta(codice string, tipo db.TipoComponente) db.Componente {
	return db.Componente{ComponenteID: uuid.New(), Codice: codice, Tipo: tipo, Qta: 1}
}

// distintaDiProva: il prodotto 7120001 con l'assieme 7120010 e il particolare 7120011 sotto; con piu' = false la
// distinta ha il solo prodotto.
func distintaDiProva(piu bool) (*fascicoloDati, []db.Componente) {
	p := componenteDistinta("7120001", db.TipoComponenteFinito)
	comp := []db.Componente{p}
	var rel []db.ComponenteRelazione
	if piu {
		a := componenteDistinta("7120010", db.TipoComponenteSottoassieme)
		x := componenteDistinta("7120011", db.TipoComponenteSciolto)
		comp = append(comp, a, x)
		rel = []db.ComponenteRelazione{{PadreID: p.ComponenteID, FiglioID: a.ComponenteID, Qta: 2}, {PadreID: a.ComponenteID, FiglioID: x.ComponenteID, Qta: 4}}
	}
	f := &fascicoloDati{Albero: fascicolo.NuovoAlbero(comp, rel), Componenti: map[uuid.UUID]db.Componente{}}
	for _, c := range comp {
		f.Componenti[c.ComponenteID] = c
	}
	return f, comp
}

// Le linguette della Distinta, una riga ciascuna: la richiesta conta i messaggi; la distinta dice se e'
// congelata (prima di tutto), le proposte da decidere, se c'e' solo il prodotto, o quanti pezzi; i documenti dicono
// i bloccanti (prima di tutto), le decisioni da verificare, i pronti, o «a posto»; la fattibilita' e' «in corso»
// nella sua fase, altrimenti la fase successiva. Il passo attivo e' uno solo.
func TestIPassiDellaDistintaDiconoLoStato(t *testing.T) {
	s := &Server{}
	riga := func(v *distintaVista, chiave string) passoVista {
		t.Helper()
		for _, p := range s.passiDistinta(v) {
			if p.Chiave == chiave {
				return p
			}
		}
		t.Fatalf("manca il passo %s", chiave)
		return passoVista{}
	}
	vista := func(f *fascicoloDati, prodotti []db.Componente, passo string) *distintaVista {
		return &distintaVista{F: f, Th: &threadDati{Messaggi: make([]messaggioThread, 2)}, Passo: passo, Prodotti: prodotti[:1]}
	}

	f, comp := distintaDiProva(false)
	v := vista(f, comp, "documenti")
	passi := s.passiDistinta(v)
	var attivi, numeri []string
	for _, p := range passi {
		numeri = append(numeri, p.Nome)
		if p.Attivo {
			attivi = append(attivi, p.Chiave)
		}
	}
	if len(passi) != 4 || passi[0].Numero != 1 || passi[3].Numero != 4 || strings.Join(attivi, " ") != "documenti" {
		t.Errorf("le linguette: %v, attive %v", numeri, attivi)
	}
	controlla := func(nome string, got passoVista, stato, classe string) {
		t.Helper()
		if got.Stato != stato || got.Classe != classe {
			t.Errorf("%s: %q %q, atteso %q %q", nome, got.Stato, got.Classe, stato, classe)
		}
	}
	controlla("richiesta", riga(v, "richiesta"), "2 messaggi", "ok")
	controlla("solo il prodotto", riga(v, "distinta"), "solo il prodotto", "warn")
	controlla("documenti a posto", riga(v, "documenti"), "a posto", "ok")
	if fa := riga(v, "fattibilita"); fa.Stato != "fase successiva" || fa.Classe != "neu" || !fa.Dopo {
		t.Errorf("fattibilita' fuori dalla sua fase: %+v", fa)
	}

	f, comp = distintaDiProva(true)
	v = vista(f, comp, "distinta")
	controlla("i pezzi", riga(v, "distinta"), "3 pezzi", "ok")
	f.NProposte = 1
	controlla("una proposta", riga(v, "distinta"), "1 proposta da decidere", "warn")
	f.NProposte = 3
	controlla("le proposte", riga(v, "distinta"), "3 proposte da decidere", "warn")
	f.Bloccata = 2
	controlla("congelata", riga(v, "distinta"), "congelata nella V2", "ok")

	v.NPronti = 2
	controlla("i pronti", riga(v, "documenti"), "2 pronti da confermare", "warn")
	f.Piano.File = []fascicolo.VoceFile{{Nome: "tavola.pdf", Stato: fascicolo.VoceDecidere}, {Nome: "7120011.pdf", Stato: fascicolo.VocePronta}}
	controlla("da verificare", riga(v, "documenti"), "1 da verificare", "warn")
	f.NBloccanti = 1
	controlla("un bloccante", riga(v, "documenti"), "1 documento obbligatorio manca", "bad")
	f.NBloccanti = 2
	controlla("i bloccanti", riga(v, "documenti"), "2 documenti obbligatori mancano", "bad")

	f.Fase = &db.FaseLog{NomeFase: db.FaseFATTIBILITA}
	if fa := riga(v, "fattibilita"); fa.Stato != "in corso" || fa.Classe != "warn" || fa.Dopo {
		t.Errorf("fattibilita' nella sua fase: %+v", fa)
	}
}

// fileDiProva e' un file della RFQ come lo legge il Fascicolo.
func fileDiProva(nome, ext, tipo string) rigaFile {
	return rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: uuid.New(), NomeFile: nome, Estensione: pgtype.Text{String: ext, Valid: true}}, Tipo: tipo}
}

// documentiDistinta mette ogni file della RFQ al suo posto: un documento confermato a un pezzo sta nel blocco del
// pezzo; confermato senza pezzo fra i documenti della richiesta; scartato fra i messi da parte; una voce del piano
// con il pezzo nel blocco del pezzo, con il suo stato e la sua domanda (quella della 4.2 sul codice letto nel
// testo del PDF compresa); una voce senza pezzo fra quelli da sistemare, con la domanda nelle parole della pagina;
// un file senza voce e' in preparazione; un archivio aperto fra gli archivi, uno non aperto da sistemare, uno gia'
// deciso da nessuna parte. Nel blocco prima i disegni, poi il 3D; i pronti e i file si contano (gli archivi no).
func TestDocumentiDistintaMetteOgniFileAlSuoPosto(t *testing.T) {
	f, comp := distintaDiProva(true)
	p, a, x := comp[0], comp[1], comp[2]
	doc := func(c *db.Componente) *db.Documento {
		d := &db.Documento{DocumentoID: uuid.New(), StatoNas: db.StatoNasScritto}
		if c != nil {
			d.ComponenteID = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
		}
		return d
	}
	confermato := fileDiProva("7120001.pdf", "pdf", "disegno_2d")
	confermato.Doc = doc(&p)
	generale := fileDiProva("capitolato.pdf", "pdf", "capitolato")
	generale.Doc = doc(nil)
	scartato := fileDiProva("logo.png", "png", "rumore")
	scartato.Proposta = &db.DocumentoProposta{Stato: db.StatoPropostaScartata, TipoProposto: db.TipoDocumentoAltro}
	step := fileDiProva("7120011.stp", "stp", "cad_3d")
	disegno := fileDiProva("7120011.pdf", "pdf", "disegno_2d")
	tavola := fileDiProva("tavola.pdf", "pdf", "disegno_2d")
	ignoto := fileDiProva("7129999.pdf", "pdf", "disegno_2d")
	daStep := fileDiProva("7120012.stp", "stp", "cad_3d")
	inArrivo := fileDiProva("7120011.dxf", "dxf", "sviluppo_dxf")
	aperto := fileDiProva("disegni.zip", "zip", "")
	step.A.ContenitoreID = uuid.NullUUID{UUID: aperto.A.AllegatoID, Valid: true}
	chiuso := fileDiProva("altro.zip", "zip", "")
	deciso := fileDiProva("vecchio.zip", "zip", "")
	deciso.Proposta = &db.DocumentoProposta{Stato: db.StatoPropostaScartata}
	f.File = []rigaFile{confermato, generale, scartato, step, disegno, tavola, ignoto, daStep, inArrivo, aperto, chiuso, deciso}

	domandaTesto := "il codice viene dal testo del PDF, non dal nome: è 7120010? Si conferma, o si corregge"
	f.Piano.File = []fascicolo.VoceFile{
		{Allegato: step.A.AllegatoID, Nome: step.A.NomeFile, Stato: fascicolo.VocePronta, Componente: &x},
		{Allegato: disegno.A.AllegatoID, Nome: disegno.A.NomeFile, Stato: fascicolo.VocePronta, Componente: &x},
		{Allegato: tavola.A.AllegatoID, Nome: tavola.A.NomeFile, Codice: "7120010", Stato: fascicolo.VoceDecidere, Componente: &a,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaCodice, Testo: domandaTesto}}},
		{Allegato: ignoto.A.AllegatoID, Nome: ignoto.A.NomeFile, Codice: "7129999", Stato: fascicolo.VoceDecidere,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaComponente, Testo: "7129999 non è nella BOM"}}},
		{Allegato: daStep.A.AllegatoID, Nome: daStep.A.NomeFile, Codice: "7120012", Stato: fascicolo.VoceAttesa,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaStrutturaEditor, Testo: "nell'editor"}}},
	}
	v := &distintaVista{F: f}
	(&Server{}).documentiDistinta(v)

	nomi := func(ff []fileDistinta) string {
		var out []string
		for _, r := range ff {
			out = append(out, r.F.A.NomeFile+":"+r.Stato)
		}
		return strings.Join(out, " ")
	}
	if len(v.Blocchi) != 3 {
		t.Fatalf("un blocco per pezzo, nell'ordine dell'albero: %d", len(v.Blocchi))
	}
	blocchi := map[string]bloccoDistinta{}
	var ordine []string
	for _, b := range v.Blocchi {
		blocchi[b.N.C.Codice] = b
		ordine = append(ordine, b.N.C.Codice+"<"+b.PadreCodice)
	}
	if strings.Join(ordine, " ") != "7120001< 7120010<7120001 7120011<7120010" {
		t.Errorf("i blocchi e i loro padri: %v", ordine)
	}
	for _, c := range []struct{ codice, file string }{
		{"7120001", "7120001.pdf:confermato"},
		{"7120010", "tavola.pdf:decidere"},
		{"7120011", "7120011.pdf:pronto 7120011.stp:pronto"},
	} {
		if got := nomi(blocchi[c.codice].File); got != c.file {
			t.Errorf("il blocco di %s: %q, atteso %q", c.codice, got, c.file)
		}
	}
	if got := blocchi["7120010"].File; len(got) != 1 || got[0].Domanda != domandaTesto || got[0].Voce == nil {
		t.Errorf("il PDF con il codice solo dal testo sta sotto il pezzo, da decidere, con la domanda della 4.2: %+v", got)
	}
	if got := nomi(v.Generali); got != "capitolato.pdf:generale" {
		t.Errorf("i documenti della richiesta: %q", got)
	}
	if got := nomi(v.DaParte); got != "logo.png:parte" {
		t.Errorf("i messi da parte: %q", got)
	}
	if got := nomi(v.DaSistemare); got != "7129999.pdf:decidere 7120012.stp:attesa 7120011.dxf:attesa altro.zip:archivio" {
		t.Errorf("da sistemare: %q", got)
	}
	if got := nomi(v.Archivi); got != "disegni.zip:archivio" {
		t.Errorf("gli archivi aperti: %q", got)
	}
	domande := map[string]string{}
	for _, r := range append(append([]fileDistinta{}, v.DaSistemare...), v.Archivi...) {
		domande[r.F.A.NomeFile] = r.Domanda
	}
	for nome, attesa := range map[string]string{
		"7129999.pdf": "Il codice «7129999» non è un pezzo della distinta: scegli qui a destra il pezzo a cui va",
		"7120012.stp": "Va a un pezzo che nasce dalla struttura di uno STEP: prima si salva la distinta (passo 2).",
		"7120011.dxf": "",
		"altro.zip":   "Archivio non aperto: il suo contenuto non è elencato.",
		"disegni.zip": "Archivio aperto: i suoi 1 file è qui sopra, uno per uno.",
	} {
		if got := domande[nome]; (attesa == "" && got != "") || !strings.HasPrefix(got, attesa) {
			t.Errorf("la domanda di %s: %q, attesa %q", nome, got, attesa)
		}
	}
	if v.NPronti != 2 || v.NFile != 9 {
		t.Errorf("i conteggi: %d pronti (attesi 2), %d file (attesi 9: gli archivi non contano)", v.NPronti, v.NFile)
	}
}
