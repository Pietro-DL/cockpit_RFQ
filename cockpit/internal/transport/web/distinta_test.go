package web

// L1 — la Distinta (PR #6, #7, #8), le funzioni pure della pagina, portate nel ramo delle prove con la fase 4.3 del
// giro 4: dallaDistinta (da dove viene un gesto e su quale passo si torna), passiDistinta (lo stato di ogni
// linguetta in una riga) e documentiDistinta (quale file va in quale gruppo del passo «Documenti e NAS»). Le prove
// con il database (le GET non scrivono, il codice solo dal testo del PDF) stanno in distinta_db_test.go.

import (
	"bytes"
	htmlesc "html"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
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
// congelata (prima di tutto), quello che l'albero proposto ha da confermare (pezzi, legami, rimozioni: quello che
// «Conferma l'albero» decide), se c'e' solo il prodotto, o quanti pezzi; i documenti dicono i bloccanti (prima di
// tutto), i file su cui serve una persona (quelli da sistemare e quelli da decidere accanto a un pezzo), i pronti, o
// «a posto»; la fattibilita' e' «in corso» nella sua fase, altrimenti la fase successiva. Il passo attivo e' uno solo.
//
// Riscritta per lo Smistamento (Distinta, giro 4, fase 4.4a.3): prima fissava la linguetta della distinta sulle
// proposte da decidere dell'autorita' («1 proposta da decidere»: bug 2, nessun gesto nella pagina per deciderle) e
// quella dei documenti su NDaVerificare («1 da verificare», che contava anche la struttura degli STEP: bug 13, un file
// in piu' delle righe). Ora la distinta conta l'albero proposto, e le proposte restano solo quando l'albero non si
// legge; i documenti contano i file. Asserzioni: prima 14, dopo 20.
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
	// senza l'albero proposto (una lettura non riuscita) la linguetta conta le proposte, come prima
	f.NProposte = 1
	controlla("una proposta", riga(v, "distinta"), "1 proposta da decidere", "warn")
	f.NProposte = 3
	controlla("le proposte", riga(v, "distinta"), "3 proposte da decidere", "warn")
	// con l'albero proposto conta quello che la conferma dell'albero decide, e non le proposte dell'autorita'
	v.AlberoLetto = true
	controlla("l'albero senza niente da confermare", riga(v, "distinta"), "3 pezzi", "ok")
	v.AlberoPezzi, v.AlberoLegami = 12, 15
	controlla("l'albero da confermare", riga(v, "distinta"), "12 pezzi, 15 legami da confermare", "warn")
	v.AlberoPezzi, v.AlberoLegami, v.AlberoRimozioni = 0, 1, 1
	controlla("un legame e una rimozione", riga(v, "distinta"), "1 legame, 1 rimozione da confermare", "warn")
	v.AlberoPezzi = 1
	controlla("un pezzo", riga(v, "distinta"), "1 pezzo, 1 legame, 1 rimozione da confermare", "warn")
	f.Bloccata = 2
	controlla("congelata", riga(v, "distinta"), "congelata nella V2", "ok")

	v.NPronti = 2
	controlla("i pronti", riga(v, "documenti"), "2 pronti da confermare", "warn")
	// la struttura degli STEP da decidere (nel piano) non e' un file: la linguetta non la conta (bug 13)
	f.Piano.File = []fascicolo.VoceFile{{Nome: "tavola.pdf", Stato: fascicolo.VoceDecidere}, {Nome: "7120011.pdf", Stato: fascicolo.VocePronta}}
	f.Piano.Strutture = []fascicolo.VoceStruttura{{Stato: fascicolo.VoceDecidere}}
	controlla("la struttura non e' un file", riga(v, "documenti"), "2 pronti da confermare", "warn")
	v.DaSistemare = []fileDistinta{{Stato: "decidere"}}
	controlla("un file da sistemare", riga(v, "documenti"), "1 file da sistemare", "warn")
	v.NDecidere = 2
	controlla("da sistemare e da decidere accanto ai pezzi", riga(v, "documenti"), "3 file da sistemare", "warn")
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

// distintaSintetica e' la Distinta di una RFQ ACME di prova, come la costruisce caricaDistinta ma senza database (fase
// 4.4a.3, per le prove dei template): il prodotto 7120001 con gli assiemi 7120010 (×1) e 7120011 (×2), il particolare
// 7121003 in comune (×2 sotto 7120010, ×1 sotto 7120011) e 7129000, un particolare che non sta sotto nessun prodotto.
// I file: il 2D di 7121003 pronto (con il percorso sul NAS che «✓ Conferma» gli darebbe), il suo STEP da decidere, il
// PDF di 7120011 con il codice da confermare, «ACME-7120012.pdf» letto solo dal nome, un capitolato da sistemare, un
// documento della richiesta gia' confermato e un logo messo da parte. Le regole del cliente hanno la famiglia 712xxxx.
func distintaSintetica(passo string, conPercorsi bool) *distintaVista {
	p := componenteDistinta("7120001", db.TipoComponenteFinito)
	a := componenteDistinta("7120010", db.TipoComponenteSottoassieme)
	b := componenteDistinta("7120011", db.TipoComponenteSottoassieme)
	x := componenteDistinta("7121003", db.TipoComponenteSciolto)
	fuori := componenteDistinta("7129000", db.TipoComponenteSciolto)
	cc := []db.Componente{p, a, b, x, fuori}
	rel := []db.ComponenteRelazione{{PadreID: p.ComponenteID, FiglioID: a.ComponenteID, Qta: 1}, {PadreID: p.ComponenteID, FiglioID: b.ComponenteID, Qta: 2},
		{PadreID: a.ComponenteID, FiglioID: x.ComponenteID, Qta: 2}, {PadreID: b.ComponenteID, FiglioID: x.ComponenteID, Qta: 1}}
	_, _, thd := datiSintetici()
	tid := thd.T.ThreadID
	f := &fascicoloDati{Albero: fascicolo.NuovoAlbero(cc, rel), Componenti: map[uuid.UUID]db.Componente{}, Scrive: true, PuoCongelare: true,
		Base: "/thread/" + tid.String() + "/fascicolo", T: thd.T, Riga: db.VCruscotto{Cliente: "ACME"},
		Completezza: map[uuid.UUID][]cella{x.ComponenteID: {
			{Tipo: db.TipoDocumentoDisegno2d, Etichetta: "2D", Simbolo: "✓°", Classe: "coda", Titolo: "presente, copia sul NAS in coda"},
			{Tipo: db.TipoDocumentoCad3d, Etichetta: "3D", Simbolo: "?", Classe: "warn", Titolo: "da confermare"}}},
		Gate: fascicolo.Gate{Problemi: []string{"1 requisito bloccante del fascicolo non soddisfatto né derogato"}}}
	for _, c := range cc {
		f.Componenti[c.ComponenteID] = c
	}
	aperta := func(r *rigaFile, tipo db.TipoDocumento) *db.DocumentoProposta {
		r.Proposta = &db.DocumentoProposta{PropostaID: uuid.New(), AllegatoID: r.A.AllegatoID, Stato: db.StatoPropostaAperta, TipoProposto: tipo}
		return r.Proposta
	}
	disegno := fileDiProva("7121003.pdf", "pdf", "disegno_2d")
	step := fileDiProva("7121003.stp", "stp", "cad_3d")
	assieme := fileDiProva("7120011.pdf", "pdf", "disegno_2d")
	dalNome := fileDiProva("ACME-7120012.pdf", "pdf", "da_determinare")
	capitolato := fileDiProva("Capitolato ACME.pdf", "pdf", "altro")
	generale := fileDiProva("Capitolato vecchio.pdf", "pdf", "capitolato")
	generale.Doc = &db.Documento{DocumentoID: uuid.New(), StatoNas: db.StatoNasScritto}
	logo := fileDiProva("logo.png", "png", "rumore")
	logo.Proposta = &db.DocumentoProposta{Stato: db.StatoPropostaScartata, TipoProposto: db.TipoDocumentoRumore}
	pd, ps, pa, pn, pc := aperta(&disegno, db.TipoDocumentoDisegno2d), aperta(&step, db.TipoDocumentoCad3d), aperta(&assieme, db.TipoDocumentoDisegno2d),
		aperta(&dalNome, db.TipoDocumentoDaDeterminare), aperta(&capitolato, db.TipoDocumentoAltro)
	f.File = []rigaFile{disegno, step, assieme, dalNome, capitolato, generale, logo}
	f.Piano.File = []fascicolo.VoceFile{
		{Allegato: disegno.A.AllegatoID, Proposta: pd.PropostaID, Nome: disegno.A.NomeFile, Tipo: db.TipoDocumentoDisegno2d, Codice: "7121003", Stato: fascicolo.VocePronta, Componente: &x},
		{Allegato: step.A.AllegatoID, Proposta: ps.PropostaID, Nome: step.A.NomeFile, Tipo: db.TipoDocumentoCad3d, Codice: "7121003", Stato: fascicolo.VoceDecidere, Componente: &x,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaRevisione, Testo: "la revisione del file non è quella del pezzo"}}},
		{Allegato: assieme.A.AllegatoID, Proposta: pa.PropostaID, Nome: assieme.A.NomeFile, Tipo: db.TipoDocumentoDisegno2d, Codice: "7120011", Stato: fascicolo.VoceDecidere, Componente: &b,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaCodice, Testo: "il codice viene dal testo del PDF, non dal nome: è 7120011? Si conferma, o si corregge"}}},
		{Allegato: dalNome.A.AllegatoID, Proposta: pn.PropostaID, Nome: dalNome.A.NomeFile, Tipo: db.TipoDocumentoDaDeterminare, Codice: "ACME-7120012", Stato: fascicolo.VoceDecidere,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaTipo, Testo: "che cos'è questo file? Il contenuto non lo dice"}}},
		{Allegato: capitolato.A.AllegatoID, Proposta: pc.PropostaID, Nome: capitolato.A.NomeFile, Tipo: db.TipoDocumentoAltro, Stato: fascicolo.VoceDecidere,
			Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaComponente, Testo: "non va a nessun pezzo"}}},
	}
	v := &distintaVista{F: f, Th: thd, Base: "/thread/" + tid.String() + "/distinta", Passo: passo, Prodotti: []db.Componente{p}}
	v.motore = classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{Regex: `(?P<codice>712\d{4})`, Descrizione: "ACME 712", Esempio: "7120001"}}})
	if conPercorsi {
		v.percorsi = map[uuid.UUID]percorsoFile{pd.PropostaID: {Percorso: `ELENCO DISEGNI\7121003\7121003_REV_ND.pdf`}}
	}
	s := &Server{}
	s.documentiDistinta(v)
	for _, n := range f.Albero.Righe {
		if n.C.Tipo == db.TipoComponenteCommerciale {
			v.Comprare = append(v.Comprare, rigaQta{C: n.C, Totale: f.Albero.Totali[n.C.ComponenteID]})
		} else {
			v.Produrre = append(v.Produrre, rigaQta{C: n.C, Totale: f.Albero.Totali[n.C.ComponenteID]})
		}
	}
	v.Passi = s.passiDistinta(v)
	for i := range v.Passi {
		if v.Passi[i].Attivo {
			if i > 0 {
				v.Prec = &v.Passi[i-1]
			}
			if i+1 < len(v.Passi) {
				v.Succ = &v.Passi[i+1]
			}
		}
	}
	v.Dati = datiDistinta{Pagina: v.Base, Base: f.Base, Thread: tid.String(), Scrive: true, Prodotti: []prodottoDistinta{{ID: p.ComponenteID.String(), Codice: p.Codice}},
		Pdf: map[string][]pdfDistinta{}, Tutti: []pdfDistinta{}, Note: map[string][]notaDoc{}, Celle: map[string][]cellaDistinta{}, Fuori: []fuoriDistinta{}}
	return v
}

// Giro 4, fase 4.4a.3 (bug 10, 11, 13 e la domanda 9): il passo 3 mette ogni pezzo con TUTTI i suoi padri e il
// totale (7121003: 4 in tutto, ×2 sotto 7120010 e ×1 sotto 7120011, che sta 2 volte sotto il prodotto), e un pezzo
// che nessun prodotto raggiunge in fondo, «fuori dalla distinta», mai «il prodotto». Conta i file da decidere accanto
// ai pezzi (la linguetta li somma a quelli da sistemare). La domanda di «✓ Conferma» dice il tipo, il pezzo e il
// percorso sul NAS (senza il percorso, che il posto lo sceglie la conferma). Il codice di «che cos'e' questo file?»
// parte solo da un codice di famiglia: il nome del file resta un suggerimento (bug 8). «Documento della richiesta»
// solo per i file non tecnici.
func TestDocumentiDistintaNominaIPadriIlFuoriELaConferma(t *testing.T) {
	v := distintaSintetica("documenti", true)
	var ordine []string
	blocchi := map[string]bloccoDistinta{}
	for _, b := range v.Blocchi {
		ordine = append(ordine, b.N.C.Codice)
		blocchi[b.N.C.Codice] = b
	}
	if strings.Join(ordine, " ") != "7120001 7120010 7121003 7120011 7129000" {
		t.Errorf("i blocchi: %v (il pezzo fuori dalla distinta in fondo)", ordine)
	}
	padri := func(b bloccoDistinta) string {
		var out []string
		for _, p := range b.Padri {
			out = append(out, p.Codice+"×"+itoa(int(p.Qta)))
		}
		return strings.Join(out, " ")
	}
	if b := blocchi["7121003"]; padri(b) != "7120010×2 7120011×1" || b.Totale != 4 || b.Fuori {
		t.Errorf("7121003 in comune: padri %q, totale %d, fuori %v", padri(b), b.Totale, b.Fuori)
	}
	if b := blocchi["7120011"]; padri(b) != "7120001×2" || b.Totale != 2 {
		t.Errorf("7120011: padri %q, totale %d", padri(b), b.Totale)
	}
	if b := blocchi["7129000"]; !b.Fuori || len(b.Padri) != 0 {
		t.Errorf("7129000 non sta sotto nessun prodotto: fuori %v, padri %q", b.Fuori, padri(b))
	}
	if b := blocchi["7120001"]; b.Fuori || b.PadreCodice != "" {
		t.Errorf("il prodotto: fuori %v, padre %q", b.Fuori, b.PadreCodice)
	}
	if v.NDecidere != 2 || len(v.DaSistemare) != 2 || v.NPronti != 1 {
		t.Errorf("da decidere accanto ai pezzi %d (attesi 2), da sistemare %d (attesi 2), pronti %d (atteso 1)", v.NDecidere, len(v.DaSistemare), v.NPronti)
	}
	righe := map[string]fileDistinta{}
	for _, b := range v.Blocchi {
		for _, r := range b.File {
			righe[r.F.A.NomeFile] = r
		}
	}
	for _, r := range append(append(append([]fileDistinta{}, v.DaSistemare...), v.Generali...), v.DaParte...) {
		righe[r.F.A.NomeFile] = r
	}
	if c := righe["7121003.pdf"].Conferma; !strings.HasPrefix(c, "Confermare «7121003.pdf» come 2D di 7121003?") || !strings.Contains(c, `Va sul NAS in ELENCO DISEGNI\7121003\7121003_REV_ND.pdf`) {
		t.Errorf("la domanda di «✓ Conferma»: %q", c)
	}
	for nome, attesi := range map[string][2]string{"7120011.pdf": {"7120011", "7120011"}, "ACME-7120012.pdf": {"", "ACME-7120012"}} {
		if r := righe[nome]; r.CodiceDaScrivere != attesi[0] || r.CodiceNelNome != attesi[1] {
			t.Errorf("%s: parte con il codice %q (atteso %q), suggerisce %q (atteso %q)", nome, r.CodiceDaScrivere, attesi[0], r.CodiceNelNome, attesi[1])
		}
	}
	for nome, tecnico := range map[string]bool{"7121003.pdf": true, "7121003.stp": true, "ACME-7120012.pdf": true, "Capitolato ACME.pdf": false, "Capitolato vecchio.pdf": false} {
		if righe[nome].Tecnico != tecnico {
			t.Errorf("%s: tecnico %v, atteso %v", nome, righe[nome].Tecnico, tecnico)
		}
	}
	// senza i percorsi (una lettura non riuscita) la domanda resta, e dice che il posto lo sceglie la conferma; senza le
	// regole del cliente non si precompila nessun codice
	v = distintaSintetica("documenti", false)
	v.motore = nil
	v.Blocchi, v.DaSistemare, v.Generali, v.DaParte, v.NPronti, v.NDecidere, v.NFile = nil, nil, nil, nil, 0, 0, 0
	(&Server{}).documentiDistinta(v)
	visti := 0
	for _, b := range v.Blocchi {
		for _, r := range b.File {
			switch r.F.A.NomeFile {
			case "7121003.pdf":
				visti++
				if !strings.Contains(r.Conferma, "Il posto sul NAS lo sceglie la conferma") {
					t.Errorf("senza il percorso: %q", r.Conferma)
				}
			case "7120011.pdf":
				visti++
				if r.CodiceDaScrivere != "" {
					t.Errorf("senza le regole del cliente il codice non si precompila: %q", r.CodiceDaScrivere)
				}
			}
		}
	}
	if visti != 2 {
		t.Errorf("le righe senza percorso e senza regole: %d, attese 2", visti)
	}
}

// Giro 4, fase 4.4a.3 (bug 2): la linguetta del passo 2 conta quello che «Conferma l'albero» decide: i pezzi proposti,
// quelli ritrovati per codice con le righe dei file ancora da decidere (anche se sono gia' nella distinta: la conferma
// decide le loro righe, e il riepilogo li elenca), i legami proposti e le rimozioni che lo STEP propone. Non i prodotti,
// non i pezzi della distinta senza righe aperte, non gli scartati, non i legami della distinta.
func TestLaLinguettaDellAlberoContaQuelloCheLaConfermaDecide(t *testing.T) {
	c := uuid.New()
	a := fascicolo.AlberoProposto{
		Nodi: []fascicolo.NodoAlbero{
			{Chiave: "cod:7120001", Stato: fascicolo.StatoAlberoNellaDistinta, Prodotto: true},
			{Chiave: "cod:7120099", Stato: fascicolo.StatoAlberoProposto, Prodotto: true},
			{Chiave: "cod:7120010", Stato: fascicolo.StatoAlberoProposto},
			{Chiave: "cod:7120011", Stato: fascicolo.StatoAlberoNellaDistinta, Ritrovato: &fascicolo.RitrovatoAlbero{Componente: c, Codice: "7120011"}},
			{Chiave: "cod:7120012", Stato: fascicolo.StatoAlberoNellaDistinta},
			{Chiave: "cod:7120013", Stato: fascicolo.StatoAlberoScartato},
		},
		Archi: []fascicolo.ArcoAlbero{
			{Padre: "cod:7120001", Figlio: "cod:7120010", Stato: fascicolo.StatoAlberoProposto},
			{Padre: "cod:7120001", Figlio: "cod:7120011", Stato: fascicolo.StatoAlberoNellaDistinta},
			{Padre: "cod:7120001", Figlio: "cod:7120012", Stato: fascicolo.StatoAlberoTolto},
			{Padre: "cod:7120001", Figlio: "cod:7120013", Stato: fascicolo.StatoAlberoScartato},
		},
	}
	v := &distintaVista{}
	v.contaAlbero(a)
	if !v.AlberoLetto || v.AlberoPezzi != 2 || v.AlberoLegami != 1 || v.AlberoRimozioni != 1 {
		t.Errorf("letto %v, pezzi %d (attesi 2: il proposto e il ritrovato), legami %d (atteso 1), rimozioni %d (attesa 1)",
			v.AlberoLetto, v.AlberoPezzi, v.AlberoLegami, v.AlberoRimozioni)
	}
}

// Bug 7 del 29/09: il rifiuto di «Metti da parte» su un file che un collega ha gia' deciso («La proposta era gia' stata
// decisa: nessun cambiamento.», da allegati.go) e' un avviso negativo, non verde come un gesto riuscito.
func TestAvvisoNegativoRiconosceIlRifiutoDiMettiDaParte(t *testing.T) {
	for a, no := range map[string]bool{
		"La proposta era già stata decisa: nessun cambiamento.": true,
		"Niente è cambiato: il file è già stato deciso":         true,
		"Assegnazione non riuscita, nessun file cambiato: x":    true,
		"Fascicolo confermato: 1 documento":                     false,
		"7121003.pdf messo da parte.":                           false,
	} {
		if avvisoNegativo(a) != no {
			t.Errorf("%q: negativo %v, atteso %v", a, avvisoNegativo(a), no)
		}
	}
}

// eseguiDistinta esegue il corpo della Distinta con i dati sintetici, come lo manda un gesto.
func eseguiDistinta(t *testing.T, v *distintaVista) string {
	t.Helper()
	var buf bytes.Buffer
	if err := serverTest(t).pagine["distinta.html"].ExecuteTemplate(&buf, "distinta_corpo", vista{Dati: v, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Giro 4, fase 4.4a.3 (domande 9 e 9a = A, bug 8, 9, 10, 11 e 14; piano, fase 4.4, «L1 sul template»): il passo 3
// della Distinta non ha piu' il gesto cumulativo («Conferma i N file pronti e copia sul NAS», che confermava in blocco
// i file scelti dal sistema); ogni modulo che non torna indietro (conferma, documento della richiesta, metti da parte,
// assegna o sposta, congela) chiede conferma, e «✓ Conferma» dice il percorso sul NAS; «Documento della richiesta»
// solo per un file non tecnico, con il tipo da scegliere; «che cos'e' questo file?» parte vuoto e obbligatorio, senza
// il nome del file come codice; un documento della richiesta si porta a un pezzo; il blocco di un pezzo in comune dice
// tutti i padri, e uno fuori dalla distinta non e' «il prodotto»; le caselle dei pezzi non usano «✓°» e hanno la
// legenda vicino; i passi sono collegamenti con aria-current, non linguette. Controprova: rimesso nel template il
// modulo cumulativo (o tolto un hx-confirm), la prova fallisce.
func TestIlPasso3DellaDistintaChiedeConfermaENonHaGestiCumulativi(t *testing.T) {
	html := eseguiDistinta(t, distintaSintetica("documenti", true))
	for _, vietato := range []string{"file pronti e copia sul NAS", `name="firma"`, `role="tab"`, `role="tablist"`, `role="tabpanel"`, "aria-selected"} {
		if strings.Contains(html, vietato) {
			t.Errorf("il passo 3 contiene ancora %q", vietato)
		}
	}
	// ogni modulo (o bottone) che manda un gesto che non torna indietro chiede conferma
	irreversibile := regexp.MustCompile(`/(conferma|generale|scarta|congela|assegna)$`)
	tag := regexp.MustCompile(`<(form|button)\b[^>]*\bhx-post="([^"]*)"[^>]*>`)
	n := 0
	for _, m := range tag.FindAllStringSubmatch(html, -1) {
		if !irreversibile.MatchString(m[2]) {
			continue
		}
		n++
		if !strings.Contains(m[0], `hx-confirm="`) {
			t.Errorf("un gesto che non torna indietro senza conferma: %s", m[0])
		}
	}
	if n < 7 {
		t.Errorf("gesti che non tornano indietro trovati: %d, attesi almeno 7 (conferma, documento della richiesta, metti da parte, sposta, assegna un documento, congela): la prova non guarda la pagina giusta", n)
	}
	leggi := htmlesc.UnescapeString(html)
	if !strings.Contains(leggi, `hx-confirm="Confermare «7121003.pdf» come 2D di 7121003? Va sul NAS in ELENCO DISEGNI\7121003\7121003_REV_ND.pdf`) {
		t.Error("«✓ Conferma» non chiede conferma con il percorso sul NAS")
	}
	riga := func(nome string) string {
		m := regexp.MustCompile(`(?s)<tr class="dst-file [a-z]+"[^>]*>\s*<td class="nome"><span class="mono">` + regexp.QuoteMeta(nome) + `</span>.*?</tr>`).FindString(html)
		if m == "" {
			t.Fatalf("la riga di %s non c'e'", nome)
		}
		return m
	}
	if r := htmlesc.UnescapeString(riga("Capitolato ACME.pdf")); !strings.Contains(r, "Documento della richiesta") || !strings.Contains(r, `<select name="tipo" required aria-label="che documento è"><option value="">`) {
		t.Errorf("il capitolato: «Documento della richiesta» con il tipo da scegliere: %s", r)
	}
	for _, nome := range []string{"7121003.stp", "ACME-7120012.pdf", "7120011.pdf"} {
		if strings.Contains(riga(nome), "Documento della richiesta") {
			t.Errorf("%s e' un file tecnico (o non si sa che cos'e'): niente «Documento della richiesta»", nome)
		}
	}
	dalNome := htmlesc.UnescapeString(riga("ACME-7120012.pdf"))
	if !strings.Contains(dalNome, `<select name="tipo" aria-label="che cos'è" required><option value="">— che cos'è? —</option>`) ||
		strings.Contains(dalNome, "selected") || !strings.Contains(dalNome, `name="codice" value=""`) || !strings.Contains(dalNome, `placeholder="il nome dice ACME-7120012"`) {
		t.Errorf("«che cos'e' questo file?» per un PDF letto dal nome: niente tipo scelto, niente nome come codice: %s", dalNome)
	}
	if !strings.Contains(riga("7120011.pdf"), `name="codice" value="7120011"`) {
		t.Error("un codice di famiglia parte scritto")
	}
	if r := riga("Capitolato vecchio.pdf"); !strings.Contains(r, `name="documento"`) || !strings.Contains(r, "Assegna a un pezzo") {
		t.Error("un documento della richiesta si porta a un pezzo (bug 9)")
	}
	if !strings.Contains(leggi, "× 4 in tutto · sotto 7120010 ×2, 7120011 ×1") {
		t.Error("il blocco di 7121003 non dice tutti i padri e il totale (bug 11)")
	}
	blocco := regexp.MustCompile(`(?s)<section class="dst-blocco fuori"[^>]*>.*?</section>`).FindString(html)
	if !strings.Contains(blocco, "7129000") || !strings.Contains(blocco, "fuori dalla distinta (passo 2)") || strings.Contains(blocco, "il prodotto") {
		t.Errorf("il pezzo fuori dalla distinta (bug 10): %s", blocco)
	}
	slot := regexp.MustCompile(`(?s)<span class="dst-slot [a-z]+"[^>]*>.*?</span>`).FindAllString(html, -1)
	if len(slot) == 0 {
		t.Fatal("nessuna casella di pezzo")
	}
	for _, s := range slot {
		if strings.Contains(s, "✓°") {
			t.Errorf("una casella di pezzo usa «✓°» (bug 14): %s", s)
		}
	}
	if !strings.Contains(leggi, "confermato, la copia sul NAS è in coda") {
		t.Error("la legenda delle caselle non e' accanto ai blocchi (bug 14)")
	}
	if !strings.Contains(html, `aria-current="step"`) {
		t.Error("il passo di adesso non ha aria-current")
	}
	if !strings.Contains(html, `data-riga="`) || !strings.Contains(html, `data-fuoco="conferma"`) {
		t.Error("le righe e i bottoni non dicono a distinta.mjs dove rimettere il fuoco dopo il gesto")
	}
}

// Giro 4, fase 4.4a.3 (domande 27, 28, 29a, 9a; piano, «Dopo gli studi del 29/09»): il passo 2 e' l'albero proposto.
// Non ci sono piu' i gesti di prima («Crea questi pezzi nella distinta», che metteva la guida sotto il prodotto — bug 1;
// «Accetta la struttura proposta» e «Salva la distinta», con la StrutturaVoluta dell'editor): c'e' «Rivedi e conferma»,
// il posto per la bozza trovata nel browser, gli strumenti per aggiungere e togliere, la finestra dei comandi e il menu
// del tasto destro fuori dal corpo (un gesto non li chiude). Chi consulta non ha ne' strumenti ne' conferma. E
// distinta.mjs lavora sulle rotte dell'albero (l'albero, il riepilogo, la conferma), tiene la bozza nel browser per RFQ
// e per utente, apre il menu anche da tastiera, e chiede i PDF con l'impronta del contenuto (cache C1).
func TestIlPasso2DellaDistintaEAlberoProposto(t *testing.T) {
	v := distintaSintetica("distinta", true)
	pagina := eseguiDistinta(t, v)
	for _, vietato := range []string{"Crea questi pezzi", "Accetta la struttura proposta", "Salva la distinta", "bom/applica", "accetta-tutto"} {
		if strings.Contains(pagina, vietato) {
			t.Errorf("il passo 2 contiene ancora %q", vietato)
		}
	}
	for _, atteso := range []string{`id="dst-salva"`, `data-azione="rivedi"`, `data-azione="annulla-tutto"`, `<div class="dst-box tono-warn" id="dst-ripresa" hidden>`,
		`data-nuovo="sottoassieme"`, `data-azione="elimina"`, `data-azione="indietro"`, `id="dst-tree"`, `id="dst-dettaglio"`, `id="dst-vassoio"`} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("il passo 2 non ha %q", atteso)
		}
	}
	// la pagina intera: la finestra dei comandi e il menu stanno fuori dal corpo che i gesti rifanno
	var buf bytes.Buffer
	if err := serverTest(t).pagine["distinta.html"].ExecuteTemplate(&buf, "contenuto", vista{Dati: v}); err != nil {
		t.Fatal(err)
	}
	intera := buf.String()
	fuori := intera[strings.LastIndex(intera, `id="distinta"`):]
	for _, atteso := range []string{`<dialog class="dst-dialogo" id="dst-dialogo"`, `id="dst-menu" role="menu"`, `id="dst-vis-finestra"`} {
		if !strings.Contains(fuori, atteso) {
			t.Errorf("la pagina non ha %q", atteso)
		}
	}
	// chi consulta: l'albero si guarda, niente strumenti, niente conferma
	v.F.Scrive = false
	consulta := eseguiDistinta(t, v)
	for _, vietato := range []string{`data-azione="rivedi"`, `data-nuovo=`, `data-azione="elimina"`, "Rianalizza gli STEP"} {
		if strings.Contains(consulta, vietato) {
			t.Errorf("chi consulta vede %q", vietato)
		}
	}

	mjs, err := fs.ReadFile(risorse.FS, "web/static/distinta.mjs")
	if err != nil {
		t.Fatal(err)
	}
	js := string(mjs)
	for _, vietato := range []string{"Crea questi pezzi", "/bom/applica", "preparaGuida", "applicaGuida", "accettaTutto", "beforeunload"} {
		if strings.Contains(js, vietato) {
			t.Errorf("distinta.mjs contiene ancora %q", vietato)
		}
	}
	for _, atteso := range []string{`"/albero"`, `"/albero/riepilogo"`, `"/albero/conferma"`, `"cockpit.distinta.bozza." + (S.dati ? S.dati.thread : "") + "." + ((S.dati && S.dati.utente) || "")`,
		"localStorage", `e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")`, `"contextmenu"`, `"?v=" + encodeURIComponent(v)`, "formato: FORMATO_BOZZA", "const FORMATO_BOZZA = " + itoa(fascicolo.FormatoBozza) + ";"} {
		if !strings.Contains(js, atteso) {
			t.Errorf("distinta.mjs non ha %q", atteso)
		}
	}
}
