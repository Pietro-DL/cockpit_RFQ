package web

// L1 — Fascicolo v3, la vista Documenti su dati sintetici: un sottoassieme condiviso da due prodotti si vede
// sotto tutti e due; un documento arrivato due volte si mostra una volta, con le note contate una volta; le
// note della revisione precedente sono nell'indice del viewer per ogni elemento, non solo per quello scelto dal
// server; le linguette dei gruppi portano il gruppo; spostando un documento si puo' dire quale sostituisce.

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func datiDocumenti(comp []db.Componente, rel []db.ComponenteRelazione) *fascicoloDati {
	d := &fascicoloDati{Base: "/thread/x/fascicolo", Scrive: true, Albero: fascicolo.NuovoAlbero(comp, rel),
		Componenti: map[uuid.UUID]db.Componente{}, Completezza: map[uuid.UUID][]cella{}, DocumentiDi: map[uuid.UUID][]db.Documento{},
		AllegatoDi: map[uuid.UUID]uuid.UUID{}, StepProdotto: map[uuid.UUID]db.VStepProdotto{}}
	for _, c := range comp {
		d.Componenti[c.ComponenteID] = c
	}
	return d
}

func codiciDelleRighe(v *docVista) string {
	var out []string
	for _, r := range v.Righe {
		s := r.C.Codice
		if r.Ripetuto {
			s += "↗"
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

// Un sottoassieme sotto due prodotti: ogni prodotto e' un gruppo, e ciascuno ha i figli del sottoassieme.
func TestDocumentiUnSottoassiemeCondivisoSiVedeInOgniProdotto(t *testing.T) {
	mk := func(codice string, tipo db.TipoComponente) db.Componente {
		return db.Componente{ComponenteID: uuid.New(), Codice: codice, Tipo: tipo, Qta: 1}
	}
	p1, p2 := mk("52900001", db.TipoComponenteFinito), mk("52900002", db.TipoComponenteFinito)
	s, c1, c2 := mk("52900010", db.TipoComponenteSottoassieme), mk("53000011", db.TipoComponenteSciolto), mk("53000012", db.TipoComponenteSciolto)
	rel := []db.ComponenteRelazione{{PadreID: p1.ComponenteID, FiglioID: s.ComponenteID, Qta: 1}, {PadreID: p2.ComponenteID, FiglioID: s.ComponenteID, Qta: 3},
		{PadreID: s.ComponenteID, FiglioID: c1.ComponenteID, Qta: 2}, {PadreID: s.ComponenteID, FiglioID: c2.ComponenteID, Qta: 1},
		{PadreID: p1.ComponenteID, FiglioID: c1.ComponenteID, Qta: 1}}
	d := datiDocumenti([]db.Componente{p1, p2, s, c1, c2}, rel)

	d.Stato = statoFascicolo{Gruppo: p2.ComponenteID.String()}
	v := costruisciDocumenti(d, nil, uuid.Nil)
	if v.Indice.Gruppo != p2.ComponenteID.String() {
		t.Errorf("il gruppo aperto nell'indice: %q", v.Indice.Gruppo)
	}
	if got := codiciDelleRighe(v); got != "52900002 52900010 53000011 53000012" {
		t.Errorf("il secondo prodotto: %s", got)
	}
	for _, g := range v.Gruppi {
		if g.Chiave == p2.ComponenteID.String() && g.N != 4 {
			t.Errorf("il gruppo del secondo prodotto conta %d componenti", g.N)
		}
	}
	if len(v.Film) != 4 || len(v.Indice.Elementi) != 4 {
		t.Errorf("filmstrip e indice del secondo prodotto: %d %d", len(v.Film), len(v.Indice.Elementi))
	}
	// nel primo prodotto 53000011 sta sotto il prodotto e sotto il sottoassieme: la seconda volta e' un rimando
	d.Stato = statoFascicolo{Gruppo: p1.ComponenteID.String()}
	v = costruisciDocumenti(d, nil, uuid.Nil)
	if got := codiciDelleRighe(v); got != "52900001 52900010 53000011 53000012 53000011↗" && got != "52900001 53000011 52900010 53000011↗ 53000012" {
		t.Errorf("il primo prodotto: %s", got)
	}
	// il nodo di un figlio del sottoassieme apre il gruppo giusto anche se e' condiviso
	d.Stato = statoFascicolo{Nodo: c2.ComponenteID}
	if v = costruisciDocumenti(d, nil, uuid.Nil); v.Gruppo == nil || v.Scelto != "c:"+c2.ComponenteID.String() {
		t.Errorf("il nodo di un figlio condiviso: %+v %s", v.Gruppo, v.Scelto)
	}
}

// Lo stesso file arrivato due volte (due allegati, un documento): una riga sola, quella che si apre, e le note
// contate una volta.
func TestDocumentiUnDocumentoArrivatoDueVolteSiMostraUnaVolta(t *testing.T) {
	s := fascicoloSintetico()
	d := s.d
	sha := strings.Repeat("a", 64)
	doc := db.Documento{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.particolare.ComponenteID, Valid: true}, Tipo: db.TipoDocumentoDisegno2d,
		NomeFile: "53017189.pdf", Estensione: "pdf", Sha256: sha, StatoNas: db.StatoNasInCoda}
	primo := rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: uuid.New(), NomeFile: "53017189.pdf", Estensione: txtT("pdf"), Sha256: txtT(sha),
		DataEvento: time.Now()}, Doc: &doc, Tipo: "disegno_2d"}
	secondo := primo
	secondo.A.AllegatoID = uuid.New()
	secondo.A.PathStaging = txtT(`C:\staging\b.pdf`) // questo si apre
	d.File = append(d.File, primo, secondo)
	note := []db.ListAnnotazioniThreadRow{{AnnotazioneID: uuid.New(), Sha256: txtT(sha), Pagina: 1, X: 0.1, Y: 0.1, Testo: "uno", CreataIl: time.Now()},
		{AnnotazioneID: uuid.New(), Sha256: txtT(sha), Pagina: 1, X: 0.2, Y: 0.2, Testo: "due", CreataIl: time.Now()}}
	d.Stato = statoFascicolo{Nodo: s.particolare.ComponenteID}
	v := costruisciDocumenti(d, note, uuid.Nil)
	if v.Sezione == nil {
		t.Fatal("nessuna sezione")
	}
	var righe []string
	for _, f := range v.Sezione.File {
		righe = append(righe, f.F.A.AllegatoID.String())
	}
	if len(righe) != 1 || righe[0] != secondo.A.AllegatoID.String() {
		t.Errorf("il documento arrivato due volte: %v (atteso solo %s)", righe, secondo.A.AllegatoID)
	}
	for _, r := range v.Righe {
		if r.C.ComponenteID == s.particolare.ComponenteID && r.NNote != 2 {
			t.Errorf("le note del particolare contate %d volte", r.NNote)
		}
	}
}

// Le note della revisione che un documento ha sostituito stanno nell'indice del viewer per ogni elemento del
// gruppo: «Vedi» le mostra anche per un componente scelto nel browser.
func TestDocumentiLeNoteDellaRevisionePrecedenteSonoNellIndice(t *testing.T) {
	s := fascicoloSintetico()
	d := s.d
	shaVecchio, shaNuovo := strings.Repeat("b", 64), strings.Repeat("d", 64)
	nuovo := db.Documento{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.particolare.ComponenteID, Valid: true}, Tipo: db.TipoDocumentoDisegno2d,
		NomeFile: "53017189 rev B.pdf", Estensione: "pdf", Sha256: shaNuovo, StatoNas: db.StatoNasInCoda}
	vecchio := db.Documento{DocumentoID: uuid.New(), ComponenteID: nuovo.ComponenteID, Tipo: db.TipoDocumentoDisegno2d, NomeFile: "53017189 rev A.pdf",
		Estensione: "pdf", Sha256: shaVecchio, StatoNas: db.StatoNasScritto, SostituitoDa: uuid.NullUUID{UUID: nuovo.DocumentoID, Valid: true}}
	aNuovo, aVecchio := uuid.New(), uuid.New()
	d.File = append(d.File,
		rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: aNuovo, NomeFile: nuovo.NomeFile, Estensione: txtT("pdf"), Sha256: txtT(shaNuovo), PathStaging: txtT(`C:\s\n.pdf`)}, Doc: &nuovo, Tipo: "disegno_2d"},
		rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: aVecchio, NomeFile: vecchio.NomeFile, Estensione: txtT("pdf"), Sha256: txtT(shaVecchio)}, Doc: &vecchio, Tipo: "disegno_2d"})
	d.DocumentiDi[s.particolare.ComponenteID] = []db.Documento{vecchio, nuovo}
	d.AllegatoDi = map[uuid.UUID]uuid.UUID{vecchio.DocumentoID: aVecchio, nuovo.DocumentoID: aNuovo}
	note := []db.ListAnnotazioniThreadRow{{AnnotazioneID: uuid.New(), Sha256: txtT(shaVecchio), Pagina: 1, X: 0.5, Y: 0.5, Testo: "sulla rev A", CreataIl: time.Now()}}
	d.Stato = statoFascicolo{Nodo: s.prodotto.ComponenteID} // il server sceglie il prodotto, non il particolare
	v := costruisciDocumenti(d, note, uuid.Nil)
	if p, ok := v.Indice.Prec[aNuovo.String()]; !ok || p.Allegato != aVecchio.String() || p.Note != 1 {
		t.Errorf("la revisione precedente nell'indice: %+v", v.Indice.Prec)
	}
	if nn := v.Indice.Note[aVecchio.String()]; len(nn) != 1 || nn[0].Testo != "sulla rev A" {
		t.Errorf("le note della revisione precedente: %+v", nn)
	}
	for _, e := range v.Indice.Elementi {
		for _, f := range e.File {
			if f.A == aVecchio.String() {
				t.Error("la revisione sostituita non e' un file dell'elemento")
			}
		}
	}
}

// Togliere il nodo non toglie il gruppo, in qualunque ordine si scrivano le coppie; sceglierne uno si'.
func TestConTieneIlGruppoQuandoSiTogliIlNodo(t *testing.T) {
	st := statoFascicolo{Nodo: uuid.New(), File: uuid.New(), Gruppo: gruppoFile}
	for _, q := range []string{st.Con("gruppo", gruppoRfq, "nodo", "", "file", ""), st.Con("nodo", "", "file", "", "gruppo", gruppoRfq)} {
		v, _ := url.ParseQuery(strings.TrimPrefix(q, "?"))
		if v.Get("gruppo") != gruppoRfq || v.Get("nodo") != "" || v.Get("file") != "" {
			t.Errorf("la linguetta del gruppo: %q", q)
		}
	}
	v, _ := url.ParseQuery(strings.TrimPrefix(st.Con("nodo", uuid.NewString()), "?"))
	if v.Get("gruppo") != "" {
		t.Errorf("scegliere un nodo toglie il gruppo: %v", v)
	}
	// le linguette della vista Documenti, come le disegna il template
	s := fascicoloSintetico()
	s.d.Stato = statoFascicolo{Nodo: s.assieme.ComponenteID}
	s.d.Documenti = costruisciDocumenti(s.d, nil, uuid.Nil)
	s.d.ChiaveCorpo = chiaveCorpoDocumenti
	html := rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "linguette", html, `/parti?gruppo=`+s.prodotto.ComponenteID.String()+`"`)
}

// Spostando un documento confermato su un altro componente si puo' dire quale dei suoi documenti sostituisce.
func TestSpostareUnDocumentoOffreDiSostituire(t *testing.T) {
	s := fascicoloSintetico()
	d := s.d
	mio := db.Documento{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.particolare.ComponenteID, Valid: true}, Tipo: db.TipoDocumentoDisegno2d,
		NomeFile: "53017189.pdf", Estensione: "pdf", Sha256: strings.Repeat("e", 64), StatoNas: db.StatoNasInCoda}
	suo := db.Documento{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.assieme.ComponenteID, Valid: true}, Tipo: db.TipoDocumentoDisegno2d,
		NomeFile: "52920517 foglio 1.pdf", Estensione: "pdf", Sha256: strings.Repeat("f", 64), StatoNas: db.StatoNasScritto}
	altroTipo := db.Documento{DocumentoID: uuid.New(), ComponenteID: suo.ComponenteID, Tipo: db.TipoDocumentoCad3d, NomeFile: "52920517.stp", Estensione: "stp"}
	d.DocumentiDi[s.particolare.ComponenteID] = []db.Documento{mio}
	d.DocumentiDi[s.assieme.ComponenteID] = []db.Documento{suo, altroTipo}
	d.File = append(d.File, rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: uuid.New(), NomeFile: mio.NomeFile, Estensione: txtT("pdf"), Sha256: txtT(mio.Sha256)},
		Doc: &mio, Tipo: "disegno_2d"})
	got := sostituibiliDa(d, mio)
	if len(got) != 1 || got[0].Doc != suo.DocumentoID || got[0].Codice != "52920517" {
		t.Fatalf("i documenti che puo' sostituire: %+v", got)
	}
	// lo STEP strutturale non si sostituisce da qui (vuole la risposta sul riferimento)
	strutt := d.Componenti[s.assieme.ComponenteID]
	strutt.StepStrutturaleID = uuid.NullUUID{UUID: suo.DocumentoID, Valid: true}
	d.Componenti[s.assieme.ComponenteID] = strutt
	for i := range d.Albero.Righe {
		if d.Albero.Righe[i].C.ComponenteID == strutt.ComponenteID {
			d.Albero.Righe[i].C = strutt
		}
	}
	if got := sostituibiliDa(d, mio); len(got) != 0 {
		t.Errorf("lo STEP strutturale fra i sostituibili: %+v", got)
	}
	strutt.StepStrutturaleID = uuid.NullUUID{}
	d.Componenti[s.assieme.ComponenteID] = strutt
	for i := range d.Albero.Righe {
		if d.Albero.Righe[i].C.ComponenteID == strutt.ComponenteID {
			d.Albero.Righe[i].C = strutt
		}
	}
	d.Stato = statoFascicolo{Nodo: s.particolare.ComponenteID}
	d.Documenti = costruisciDocumenti(d, nil, uuid.Nil)
	html := rendiParte(t, "fasc_doc_sezione", d)
	haTesto(t, "sezione", html, `<option value="`+suo.DocumentoID.String()+`">sostituisce 52920517 foglio 1.pdf (52920517)</option>`, "Cambia componente")
	senzaTesto(t, "sezione", html, "sostituisce 52920517.stp")
}
