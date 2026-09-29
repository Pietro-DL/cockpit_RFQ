package web

// L1 — giro 4, fase 4.1b (domanda 9b = A): «Conferma Fascicolo» con la conferma scritta. Il riepilogo dice, per
// ogni file pronto, il componente, il tipo di documento e il percorso che ricevera' sul NAS, e ha la firma di tutto
// questo; il modulo che scrive sta solo nel riepilogo, con quella firma e il bottone «Conferma e copia sul NAS»;
// nella pagina non c'e' nessun modulo con la firma del piano gia' dentro. Senza database: il percorso lo da'
// percorsoFinto, con le regole della conferma (cartella del tipo e del codice, nome sul NAS, progressivo).

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// percorsoFinto fa quello che fa riepilogoDelPiano senza il database: la cartella del tipo e del codice (i
// tecnici in ELENCO DISEGNI\<codice>, i capitolati in CAPITOLATI), il nome sul NAS, il primo progressivo che non
// e' ne' preso dal riepilogo ne' occupato. esistenti sono i documenti gia' nel fascicolo, per contenuto.
func percorsoFinto(esistenti map[string]string, occupati ...string) percorsoNelRiepilogo {
	occ := map[string]bool{}
	for _, o := range occupati {
		occ[strings.ToLower(o)] = true
	}
	return func(v fascicolo.VoceFile, codice string, presi map[string]bool) (string, error) {
		if v.Duplicato != "" {
			if p, ok := esistenti[v.Sha256]; ok {
				return p, nil
			}
			return "", rifiuto("il documento con lo stesso contenuto non è nel fascicolo")
		}
		l := documenti.LayoutDocumento{Sottocartella: "CAPITOLATI"}
		if documenti.Tecnico(v.Tipo) {
			l = documenti.LayoutDocumento{Sottocartella: "ELENCO DISEGNI", PerCodice: true}
		}
		cartella, err := documenti.CartellaDocumento(l, true, codice)
		if err != nil {
			return "", err
		}
		nome := documenti.NomeSulNas(v.Tipo, codice, v.Rev, v.Estensione, v.Nome)
		for n := 1; n < 10; n++ {
			p := documenti.NellaCartella(cartella, documenti.ConProgressivo(nome, n))
			if !presi[strings.ToLower(p)] && !occ[strings.ToLower(p)] {
				return p, nil
			}
		}
		return "", errors.New("nessun nome libero")
	}
}

// pianoDelRiepilogo: il prodotto 7120001 con due fogli del suo disegno arrivati insieme, lo stesso primo foglio
// arrivato due volte, un capitolato della RFQ, un file gia' nel fascicolo con un altro nome, il 3D di 7120012 che
// nasce dalla struttura di uno STEP, un file pronto con il codice diverso dal suo componente, e un file da decidere.
func pianoDelRiepilogo() (fascicolo.PianoFascicolo, map[string]uuid.UUID) {
	tid := uuid.New()
	prodotto := db.Componente{ComponenteID: uuid.New(), ThreadID: tid, Codice: "7120001", Tipo: db.TipoComponenteFinito}
	altro := db.Componente{ComponenteID: uuid.New(), ThreadID: tid, Codice: "7120011", Tipo: db.TipoComponenteSciolto}
	id := map[string]uuid.UUID{}
	voce := func(nome string, tipo db.TipoDocumento, codice, sha string, comp *db.Componente) fascicolo.VoceFile {
		id[nome] = uuid.New()
		ext := nome[strings.LastIndex(nome, ".")+1:]
		return fascicolo.VoceFile{Proposta: id[nome], Allegato: uuid.New(), Nome: nome, Estensione: ext, Sha256: sha, Tipo: tipo, Codice: codice,
			Componente: comp, Stato: fascicolo.VocePronta}
	}
	f1 := voce("7120001.pdf", db.TipoDocumentoDisegno2d, "7120001", "a1", &prodotto)
	f2 := voce("7120001 foglio 2.pdf", db.TipoDocumentoDisegno2d, "7120001", "a2", &prodotto)
	f1.Aggiunge, f2.Aggiunge = true, true
	doppio := voce("7120001 copia.pdf", db.TipoDocumentoDisegno2d, "7120001", "a1", &prodotto)
	doppio.Duplicato = "7120001.pdf"
	capitolato := voce("Capitolato ACME.pdf", db.TipoDocumentoCapitolato, "", "a4", nil)
	gia := voce("7120001 rev A.pdf", db.TipoDocumentoDisegno2d, "7120001", "b1", &prodotto)
	gia.Duplicato = "7120001_A.pdf"
	step := voce("7120012.stp", db.TipoDocumentoCad3d, "7120012", "a5", nil)
	step.DaStep = &fascicolo.NodoInArrivo{Allegato: uuid.New(), File: "7120001A_1.stp", Proposta: uuid.New(), Codice: "7120012"}
	diverso := voce("7120010.pdf", db.TipoDocumentoDisegno2d, "7120010", "a6", &altro)
	decidere := voce("anonimo.pdf", db.TipoDocumentoDaDeterminare, "", "a7", nil)
	decidere.Stato = fascicolo.VoceDecidere
	return fascicolo.PianoFascicolo{File: []fascicolo.VoceFile{f1, f2, doppio, capitolato, gia, step, diverso, decidere}}, id
}

// Il riepilogo dice di ogni file pronto il componente, il tipo e il percorso sul NAS, nell'ordine della conferma:
// il secondo foglio prende il progressivo, lo stesso contenuto arrivato due volte e il file gia' nel fascicolo
// restano dove sta il primo (solo la provenienza), un nome gia' occupato sul NAS sposta tutti di uno. Un file che
// la conferma rifiuterebbe ha la riga in errore; un errore vero del database ferma il riepilogo.
func TestIlRiepilogoDiceComponenteTipoEPercorso(t *testing.T) {
	p, id := pianoDelRiepilogo()
	esistenti := map[string]string{"b1": `ELENCO DISEGNI\7120001\7120001_REV_A.pdf`}
	r, err := riepilogoDa(p, `ACME\WIP\2026 09 29 RFQ 7120001`, percorsoFinto(esistenti))
	if err != nil {
		t.Fatal(err)
	}
	if r.Cartella != `ACME\WIP\2026 09 29 RFQ 7120001` || len(r.File) != 7 {
		t.Fatalf("il riepilogo: cartella %q, %d file (attesi i 7 pronti)", r.Cartella, len(r.File))
	}
	attesi := []struct {
		nome, componente, percorso, provenienza, daStep string
		tipo                                            db.TipoDocumento
	}{
		{"7120001.pdf", "7120001", `ELENCO DISEGNI\7120001\7120001_REV_ND.pdf`, "", "", db.TipoDocumentoDisegno2d},
		{"7120001 foglio 2.pdf", "7120001", `ELENCO DISEGNI\7120001\7120001_REV_ND_2.pdf`, "", "", db.TipoDocumentoDisegno2d},
		{"7120001 copia.pdf", "7120001", `ELENCO DISEGNI\7120001\7120001_REV_ND.pdf`, "7120001.pdf", "", db.TipoDocumentoDisegno2d},
		{"Capitolato ACME.pdf", "", `CAPITOLATI\Capitolato ACME.pdf`, "", "", db.TipoDocumentoCapitolato},
		{"7120001 rev A.pdf", "7120001", `ELENCO DISEGNI\7120001\7120001_REV_A.pdf`, "7120001_A.pdf", "", db.TipoDocumentoDisegno2d},
		{"7120012.stp", "7120012", `ELENCO DISEGNI\7120012\7120012_REV_ND.stp`, "", "7120001A_1.stp", db.TipoDocumentoCad3d},
		{"7120010.pdf", "7120011", "", "", "", db.TipoDocumentoDisegno2d},
	}
	for i, a := range attesi {
		f := r.File[i]
		if f.Nome != a.nome || f.Proposta != id[a.nome] || f.Componente != a.componente || f.Percorso != a.percorso ||
			f.Provenienza != a.provenienza || f.DaStep != a.daStep || f.Tipo != a.tipo {
			t.Errorf("riga %d: %+v, attesa %+v", i, f, a)
		}
	}
	if e := r.File[6].Errore; !strings.Contains(e, "codice diverso: 7120010 contro 7120011") {
		t.Errorf("il file con il codice diverso dal componente: errore %q", e)
	}
	for _, f := range r.File[:6] {
		if f.Errore != "" {
			t.Errorf("%s: errore %q", f.Nome, f.Errore)
		}
	}
	if !r.File[0].Aggiunge || r.File[0].Codice != "7120001" || r.riga(id["anonimo.pdf"]) != nil || r.riga(id["7120012.stp"]) != &r.File[5] {
		t.Errorf("fogli, codice, righe: %+v", r.File[0])
	}

	// un nome gia' occupato sul NAS (un documento, uno spostamento, un orfano): i due fogli scalano di uno
	r, err = riepilogoDa(p, `ACME\WIP\2026 09 29 RFQ 7120001`, percorsoFinto(esistenti, `elenco disegni\7120001\7120001_rev_nd.PDF`))
	if err != nil {
		t.Fatal(err)
	}
	if r.File[0].Percorso != `ELENCO DISEGNI\7120001\7120001_REV_ND_2.pdf` || r.File[1].Percorso != `ELENCO DISEGNI\7120001\7120001_REV_ND_3.pdf` ||
		r.File[2].Percorso != r.File[0].Percorso {
		t.Errorf("con il nome occupato: %q %q %q", r.File[0].Percorso, r.File[1].Percorso, r.File[2].Percorso)
	}

	// un rifiuto resta sulla riga; un errore vero ferma il riepilogo
	r, err = riepilogoDa(p, "", func(v fascicolo.VoceFile, _ string, _ map[string]bool) (string, error) {
		return "", rifiuto("la RFQ non ha una cartella sul NAS")
	})
	if err != nil || r.File[0].Errore != "la RFQ non ha una cartella sul NAS" || r.File[0].Percorso != "" {
		t.Errorf("il rifiuto sulla riga: %v %+v", err, r.File[0])
	}
	guasto := errors.New("connessione chiusa")
	if _, err := riepilogoDa(p, "", func(fascicolo.VoceFile, string, map[string]bool) (string, error) { return "", guasto }); !errors.Is(err, guasto) {
		t.Errorf("l'errore del database: %v", err)
	}
}

// La firma del riepilogo e' dei file e dei percorsi: gli stessi dati danno la stessa firma; un percorso diverso,
// un'altra cartella della RFQ o un piano diverso la cambiano. Non e' la firma del piano.
func TestLaFirmaDelRiepilogoEDeiFileEDeiPercorsi(t *testing.T) {
	p, _ := pianoDelRiepilogo()
	cartella := `ACME\WIP\2026 09 29 RFQ 7120001`
	firma := func(p fascicolo.PianoFascicolo, cartella string, occupati ...string) string {
		t.Helper()
		r, err := riepilogoDa(p, cartella, percorsoFinto(map[string]string{"b1": `ELENCO DISEGNI\7120001\7120001_REV_A.pdf`}, occupati...))
		if err != nil {
			t.Fatal(err)
		}
		return r.Firma
	}
	prima := firma(p, cartella)
	if prima == "" || prima != firma(p, cartella) {
		t.Fatalf("stessi dati, stessa firma: %q", prima)
	}
	if prima == p.Firma() {
		t.Error("la firma del riepilogo non e' quella del piano")
	}
	if firma(p, cartella, `ELENCO DISEGNI\7120001\7120001_REV_ND.pdf`) == prima {
		t.Error("un percorso sul NAS diverso (a parita' di piano) cambia la firma")
	}
	if firma(p, `ACME\WIP\2026 09 30 RFQ 7120001`) == prima {
		t.Error("un'altra cartella della RFQ cambia la firma")
	}
	p.File[3].Stato = fascicolo.VoceDecidere
	if firma(p, cartella) == prima {
		t.Error("un piano diverso cambia la firma")
	}
}

var reModuloConferma = regexp.MustCompile(`(?s)<form[^>]*hx-post="[^"]*/fascicolo/conferma"[^>]*>.*?</form>`)

// Il modulo che scrive sta solo nel riepilogo. La pagina senza cassetto non ha nessun modulo che mandi la conferma
// con una firma, e la firma del piano non c'e' da nessuna parte; nel riepilogo il modulo va anche senza JavaScript
// (method/action), porta la firma del riepilogo (non quella del piano), l'elenco delle voci e il bottone «Conferma e
// copia sul NAS», che e' l'unico che manda `conferma`. Chi consulta vede il riepilogo senza modulo.
func TestIlModuloDellaConfermaStaSoloNelRiepilogo(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	p, _ := pianoDelRiepilogo()
	s.d.Piano = p
	pagina := rendiFascicolo(t, "contenuto", s.d)
	if strings.Contains(pagina, s.d.Piano.Firma()) {
		t.Error("la pagina porta la firma del piano")
	}
	for _, m := range reModuloConferma.FindAllString(pagina, -1) {
		if strings.Contains(m, `name="firma"`) || strings.Contains(m, `name="conferma"`) || strings.Count(m, `name="voce"`) != 1 {
			t.Errorf("fuori dal riepilogo un modulo manda la conferma con la firma, o con piu' voci:\n%s", m)
		}
	}
	haTesto(t, "pagina", pagina, `id="conferma-fascicolo" href="`+s.d.Base+`?cassetto=piano"`)

	s.d.Stato.Cassetto = "piano"
	rp, err := riepilogoDa(p, `ACME\WIP\2026 09 29 RFQ 7120001`, percorsoFinto(map[string]string{"b1": `ELENCO DISEGNI\7120001\7120001_REV_A.pdf`}))
	if err != nil {
		t.Fatal(err)
	}
	s.d.Riepilogo = &rp
	pagina = rendiFascicolo(t, "contenuto", s.d)
	moduli := reModuloConferma.FindAllString(pagina, -1)
	var riepilogo []string
	for _, m := range moduli {
		if strings.Contains(m, `name="firma"`) {
			riepilogo = append(riepilogo, m)
		}
	}
	if len(riepilogo) != 1 {
		t.Fatalf("moduli con la firma: %d, atteso quello del riepilogo", len(riepilogo))
	}
	m := riepilogo[0]
	haTesto(t, "riepilogo", m, `method="post" action="`+s.d.Base+`/conferma"`, `name="firma" value="`+rp.Firma+`"`, `name="selezione" value="1"`,
		`<button type="submit" class="btn primary" name="conferma" value="1">Conferma e copia sul NAS</button>`,
		`ACME\WIP\2026 09 29 RFQ 7120001`, `sul NAS: <span class="mono">ELENCO DISEGNI\7120001\7120001_REV_ND_2.pdf</span>`,
		`sul NAS: <span class="mono">CAPITOLATI\Capitolato ACME.pdf</span>`, "capitolato della RFQ, senza componente",
		`già nel fascicolo come 7120001_A.pdf, in <span class="mono">ELENCO DISEGNI\7120001\7120001_REV_A.pdf</span>: solo la provenienza, nessuna copia`,
		"(nasce dallo STEP 7120001A_1.stp)", "nessun percorso sul NAS: codice diverso")
	if n := strings.Count(m, `type="checkbox" name="voce"`); n != 7 {
		t.Errorf("caselle del riepilogo: %d, attese le 7 voci pronte", n)
	}
	if strings.Contains(pagina, s.d.Piano.Firma()) || strings.Count(pagina, `name="conferma"`) != 1 {
		t.Error("la firma del piano, o un secondo `conferma`, nella pagina con il riepilogo")
	}

	s.d.Scrive = false
	html := rendiParte(t, "fasc_rivedi", s.d)
	haTesto(t, "consultazione", html, `<div class="rivedi">`, `ELENCO DISEGNI\7120001\7120001_REV_ND.pdf`)
	senzaTesto(t, "consultazione", html, "<form", "<input", "Conferma e copia sul NAS", rp.Firma)
}

// Il «✓ Conferma» di un file solo (una voce e nient'altro) resta un gesto a parte; tutto il resto e' «Conferma
// Fascicolo», e passa dal riepilogo: la firma del piano di prima, piu' voci, una struttura, l'elenco del riepilogo.
func TestSoloIlFileSoloSaltaIlRiepilogo(t *testing.T) {
	a, b := uuid.New().String(), uuid.New().String()
	for _, c := range []struct {
		form url.Values
		solo bool
	}{
		{url.Values{"voce": {a}}, true},
		{url.Values{"voce": {a}, "corpo_chiave": {"vuoto"}}, true},
		{url.Values{}, false},
		{url.Values{"firma": {"0123456789abcdef"}}, false},
		{url.Values{"voce": {a, b}}, false},
		{url.Values{"voce": {a}, "firma": {"0123456789abcdef"}}, false},
		{url.Values{"voce": {a}, "conferma": {"1"}}, false},
		{url.Values{"voce": {a}, "selezione": {"1"}}, false},
		{url.Values{"voce": {a}, "struttura": {b}}, false},
		{url.Values{"voce": {a}, "strutturale": {b}}, false},
		{url.Values{"struttura": {b}}, false},
	} {
		if got := fileSolo(c.form); got != c.solo {
			t.Errorf("%v: file solo %v, atteso %v", c.form, got, c.solo)
		}
	}
}
