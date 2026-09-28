package web

// La vista Documenti del Fascicolo (v3): la schermata principale. Un viewer tecnico organizzato per
// componente, non per file: il disegno grande al centro, sotto il filmstrip dei componenti del prodotto
// corrente, a destra la struttura del prodotto (con lo stato dei documenti di ogni riga), i file del
// componente scelto con le loro associazioni da confermare, e le note puntate sul disegno.
//
// Il server disegna l'indice di tutto il gruppo (albero, filmstrip, e per il viewer l'elenco dei file e delle
// note di ogni componente in un JSON), ma la sezione pesante — i gesti su ogni file — solo per quello che e'
// scelto: scegliere un altro componente la chiede a GET /thread/{id}/fascicolo/sezione, e intanto il
// disegno si apre subito. Il viewer (lo stage) e' JavaScript (fascicolo.mjs con pdf.js) e sta dentro
// #vista con hx-preserve: i gesti e il poll rifanno il resto, il disegno aperto resta aperto.

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// chiaveCorpoDocumenti e' quello che mostra il corpo del vecchio pannello di destra nella vista Documenti:
// niente (il disegno sta nello stage), e con una chiave fissa la risposta di un gesto non lo rifa'.
const chiaveCorpoDocumenti = "documenti"

// docVista e' la vista Documenti.
type docVista struct {
	Gruppi []gruppoDoc
	Gruppo *gruppoDoc
	Righe  []rigaDoc // l'albero del gruppo (prodotto o fuori dalla struttura); vuoto per i gruppi di file
	Film   []tileDoc
	// Scelto e' l'elemento aperto: "c:<componente>" o "f:<allegato>" (un file dei gruppi di file).
	Scelto  string
	Sezione *sezioneDoc
	// Indice e' quello che serve al viewer per aprire subito un altro elemento del gruppo: i suoi file e le
	// note di ciascuno. Va nella pagina come JSON.
	Indice indiceDoc
	// Proposte: gli STEP con una struttura proposta da rivedere nell'editor, per il riquadro in testa.
	Proposte []propostaStruttura
	Vuota    string // perche' non c'e' niente da mostrare
}

// gruppoDoc e' un gruppo della vista: un prodotto finito, i componenti fuori dalla struttura, i file da
// associare, i documenti della RFQ.
type gruppoDoc struct {
	Chiave   string // uuid del prodotto, oppure fuori, file, rfq
	Nome     string
	N        int
	Prodotto *db.Componente
}

// rigaDoc e' una riga dell'albero di navigazione.
type rigaDoc struct {
	C        db.Componente
	Livello  int
	Qta      int32
	Padri    int
	Ripetuto bool
	Celle    []cella
	Step     *db.VStepProdotto
	Decidere int // file del piano che aspettano una decisione per questo componente
	NNote    int
}

// Chiave e' la chiave di selezione della riga.
func (r rigaDoc) Chiave() string { return "c:" + r.C.ComponenteID.String() }

// tileDoc e' una miniatura del filmstrip.
type tileDoc struct {
	Chiave    string
	Codice    string
	Titolo    string
	Anteprima uuid.UUID // l'allegato da cui si disegna la miniatura; zero = nessuna
	Stato     string    // doc (disegno confermato), proposta (in arrivo), formato (un 2D che non si vede), manca
	Condiviso bool
	Decidere  int
	NNote     int
}

// HaAnteprima: c'e' un PDF da disegnare nella miniatura. (Un uuid e' un array: nei template e' sempre vero.)
func (t tileDoc) HaAnteprima() bool { return t.Anteprima != uuid.Nil }

// sezioneDoc e' il pannello dell'elemento scelto: il componente (o il file) con i suoi file.
type sezioneDoc struct {
	Chiave string
	C      *db.Componente
	Celle  []cella
	Step   *db.VStepProdotto
	Padri  []arcoNodo
	File   []fileDoc
	Scelto uuid.UUID // l'allegato aperto nel viewer
}

// fileDoc e' un file nel pannello: la riga di sempre, la voce del piano se e' ancora da confermare, e le
// note sul suo contenuto.
type fileDoc struct {
	F         rigaFile
	Voce      *fascicolo.VoceFile
	Servibile bool   // il PDF si puo' aprire: nello staging, o confermato e scritto sul NAS
	Etichetta string // 2D, 3D, DXF, capitolato…
	Note      []notaDoc
	// Precedente: la revisione che questo documento ha sostituito, con le sue note (non migrano).
	NotePrecedenti     int
	AllegatoPrecedente uuid.UUID
	NomePrecedente     string
	// Spostabile: un documento confermato che si puo' spostare su un altro componente senza toccare il NAS
	// (non ancora scritto, o nessun cambio di codice); altrimenti Fermo dice perche' no.
	Spostabile bool
	Fermo      string
	// Sostituibili: spostandolo, i documenti correnti dello stesso tipo degli altri componenti che puo'
	// sostituire (il server controlla che siano del componente scelto).
	Sostituibili []sostituibile
	// CodiceLetto: il codice che il file dice, quando non e' quello con cui e' agganciato.
	CodiceLetto string
}

// sostituibile e' un documento corrente che un documento spostato puo' sostituire.
type sostituibile struct {
	Doc    uuid.UUID
	Nome   string
	Codice string
}

// Scelto dice se il file e' quello aperto nel viewer.
func (f fileDoc) Aperto(s *sezioneDoc) bool { return s != nil && s.Scelto == f.F.A.AllegatoID }

// notaDoc e' una nota puntata su un disegno, come la mostra il pannello.
type notaDoc struct {
	ID     uuid.UUID `json:"id"`
	N      int       `json:"n"`
	Pagina int32     `json:"p"`
	X      float64   `json:"x"`
	Y      float64   `json:"y"`
	Testo  string    `json:"t"`
	Autore string    `json:"chi"`
	Quando string    `json:"quando"`
	Mia    bool      `json:"mia"`
	Su     string    `json:"su,omitempty"` // il componente su cui e' stata scritta, se non e' quello che si guarda
}

// indiceDoc e' il JSON del viewer.
type indiceDoc struct {
	Elementi []elementoIndice      `json:"elementi"`
	Note     map[string][]notaDoc  `json:"note"` // allegato → note sul suo contenuto
	Scelto   string                `json:"scelto"`
	File     string                `json:"file"`
	Scrive   bool                  `json:"scrive"`
	Bloccata int32                 `json:"bloccata"`
	Prec     map[string]precedente `json:"prec"`   // allegato → la revisione precedente, se ha note
	Gruppo   string                `json:"gruppo"` // il gruppo aperto: l'indirizzo lo porta
}

type precedente struct {
	Allegato string `json:"a"`
	Nome     string `json:"nome"`
	Note     int    `json:"n"`
}

type elementoIndice struct {
	K      string     `json:"k"`
	Codice string     `json:"codice"`
	Desc   string     `json:"desc,omitempty"`
	Comp   string     `json:"comp,omitempty"`
	File   []fileIndi `json:"file"`
}

type fileIndi struct {
	A     string `json:"a"`
	Nome  string `json:"nome"`
	Tipo  string `json:"tipo"`
	Pdf   bool   `json:"pdf"`
	Ok    bool   `json:"ok"`    // si puo' aprire
	Stato string `json:"stato"` // doc, proposta
}

// propostaStruttura e' uno STEP con una struttura proposta da rivedere nell'editor.
type propostaStruttura struct {
	Allegato uuid.UUID
	Nome     string
	Nodi     int
	Archi    int
}

// voceVista e' una voce del piano con la schermata, per i gesti condivisi fra il cassetto e il pannello.
type voceVista struct {
	D *fascicoloDati
	V fascicolo.VoceFile
}

// voceVistaDi accetta la voce o il suo puntatore: il cassetto la scorre per valore, il pannello la tiene come
// puntatore.
func voceVistaDi(d *fascicoloDati, v any) voceVista {
	switch x := v.(type) {
	case fascicolo.VoceFile:
		return voceVista{D: d, V: x}
	case *fascicolo.VoceFile:
		if x != nil {
			return voceVista{D: d, V: *x}
		}
	}
	return voceVista{D: d}
}

// fileVista e' un file del pannello con la schermata e la sezione.
type fileVista struct {
	D *fascicoloDati
	S *sezioneDoc
	F fileDoc
}

// ------------------------------------------------------------------ costruzione

// servibile dice se un file si apre nel viewer: la stessa regola dell'anteprima (un PDF nello staging, o un
// documento scritto sul NAS con lo stesso contenuto).
func servibile(f rigaFile) bool {
	return f.Pdf() && ((f.A.PathStaging.Valid && !f.FileMancante) || (f.Doc != nil && f.Doc.StatoNas == db.StatoNasScritto))
}

// ordineTipo mette i file di un componente in un ordine che serve a chi guarda: prima i disegni.
func ordineTipo(t string) int {
	switch db.TipoDocumento(t) {
	case db.TipoDocumentoDisegno2d:
		return 0
	case db.TipoDocumentoCad3d:
		return 1
	case db.TipoDocumentoSviluppoDxf:
		return 2
	}
	return 3
}

// costruisciDocumenti costruisce la vista Documenti dai dati gia' letti e dalle note della RFQ. Pura.
func costruisciDocumenti(d *fascicoloDati, note []db.ListAnnotazioniThreadRow, utente uuid.UUID) *docVista {
	v := &docVista{Indice: indiceDoc{Note: map[string][]notaDoc{}, Prec: map[string]precedente{}, Scrive: d.Scrive, Bloccata: d.Bloccata}}

	// le voci del piano per allegato, e i file per componente
	voce := map[uuid.UUID]*fascicolo.VoceFile{}
	for i := range d.Piano.File {
		voce[d.Piano.File[i].Allegato] = &d.Piano.File[i]
	}
	perCodice := map[string]uuid.UUID{}
	for _, c := range d.Componenti {
		if c.ArchiviatoIl == nil {
			perCodice[strings.ToUpper(strings.TrimSpace(c.Codice))] = c.ComponenteID
		}
	}
	fileDi := map[uuid.UUID][]rigaFile{}
	var daAssociare, dellaRfq []rigaFile
	perAllegato := map[uuid.UUID]rigaFile{}
	// un documento arrivato piu' volte (lo stesso file rimandato in una risposta: una provenienza in piu') e'
	// un file solo: si mostra quello che si apre, altrimenti il primo
	unaVolta := map[uuid.UUID]rigaFile{}
	for _, f := range d.File {
		if f.Doc == nil {
			continue
		}
		if g, ok := unaVolta[f.Doc.DocumentoID]; !ok || (!servibile(g) && servibile(f)) {
			unaVolta[f.Doc.DocumentoID] = f
		}
	}
	for _, f := range d.File {
		perAllegato[f.A.AllegatoID] = f
		if f.estensione() == "zip" {
			continue
		}
		switch {
		case f.Doc != nil:
			if f.Doc.SostituitoDa.Valid || unaVolta[f.Doc.DocumentoID].A.AllegatoID != f.A.AllegatoID {
				continue
			}
			if f.Doc.ComponenteID.Valid {
				if c, ok := d.Componenti[f.Doc.ComponenteID.UUID]; ok && c.ArchiviatoIl == nil {
					fileDi[c.ComponenteID] = append(fileDi[c.ComponenteID], f)
					continue
				}
			}
			dellaRfq = append(dellaRfq, f)
		case f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaAperta && !f.Rumore():
			comp := uuid.Nil
			if v := voce[f.A.AllegatoID]; v != nil && v.Componente != nil && v.Componente.ArchiviatoIl == nil {
				comp = v.Componente.ComponenteID
			} else if v != nil && v.Alias != nil {
				comp = v.Alias.ComponenteID // lo stesso pezzo con il suffisso del cliente: la domanda sta nel suo pannello
			} else if f.Proposta.ComponenteID.Valid {
				comp = f.Proposta.ComponenteID.UUID
			} else if id, ok := perCodice[strings.ToUpper(strings.TrimSpace(f.Codice))]; ok && f.Codice != "" {
				comp = id
			}
			if c, ok := d.Componenti[comp]; ok && c.ArchiviatoIl == nil {
				fileDi[comp] = append(fileDi[comp], f)
			} else {
				daAssociare = append(daAssociare, f)
			}
		}
	}
	for k := range fileDi {
		ff := fileDi[k]
		sort.SliceStable(ff, func(i, j int) bool {
			if oi, oj := ordineTipo(ff[i].Tipo), ordineTipo(ff[j].Tipo); oi != oj {
				return oi < oj
			}
			if (ff[i].Doc != nil) != (ff[j].Doc != nil) {
				return ff[i].Doc != nil
			}
			return ff[i].A.NomeFile < ff[j].A.NomeFile
		})
	}

	// le note, per contenuto
	perSha := map[string][]db.ListAnnotazioniThreadRow{}
	for _, n := range note {
		if n.Sha256.Valid {
			perSha[n.Sha256.String] = append(perSha[n.Sha256.String], n)
		}
	}
	noteDi := func(f rigaFile, comp uuid.UUID) []notaDoc {
		if !f.A.Sha256.Valid {
			return nil
		}
		var out []notaDoc
		for i, n := range perSha[f.A.Sha256.String] {
			x := notaDoc{ID: n.AnnotazioneID, N: i + 1, Pagina: n.Pagina, X: n.X, Y: n.Y, Testo: n.Testo,
				Autore: n.AutoreNome, Quando: n.CreataIl.Local().Format("02/01/2006 15:04"), Mia: n.CreataDa == utente}
			if n.ComponenteID.Valid && n.ComponenteID.UUID != comp {
				x.Su = n.CodiceComponente
			}
			out = append(out, x)
		}
		return out
	}
	nNote := func(comp uuid.UUID) int {
		n, visti := 0, map[string]bool{}
		for _, f := range fileDi[comp] {
			if f.A.Sha256.Valid && !visti[f.A.Sha256.String] {
				visti[f.A.Sha256.String] = true
				n += len(perSha[f.A.Sha256.String])
			}
		}
		return n
	}
	// precedenteDi e' la revisione che un documento ha sostituito, con il suo allegato, se ha note
	precedenteDi := func(f rigaFile) (db.Documento, uuid.UUID, bool) {
		if f.Doc == nil || !f.Doc.ComponenteID.Valid {
			return db.Documento{}, uuid.Nil, false
		}
		for _, x := range d.DocumentiDi[f.Doc.ComponenteID.UUID] {
			if x.SostituitoDa.Valid && x.SostituitoDa.UUID == f.Doc.DocumentoID {
				return x, d.AllegatoDi[x.DocumentoID], true
			}
		}
		return db.Documento{}, uuid.Nil, false
	}
	decidere := map[uuid.UUID]int{}
	for _, f := range d.Piano.File {
		switch {
		case f.Stato != fascicolo.VoceDecidere:
		case f.Componente != nil:
			decidere[f.Componente.ComponenteID]++
		case f.Alias != nil:
			decidere[f.Alias.ComponenteID]++
		}
	}

	// i gruppi: i prodotti (radici finite), i componenti che nessun prodotto raggiunge, i file
	var prodotti, fuori []*fascicolo.Nodo
	for _, r := range d.Albero.Radici {
		if r.C.Tipo == db.TipoComponenteFinito {
			prodotti = append(prodotti, r)
		} else {
			fuori = append(fuori, r)
		}
	}
	// d.Albero espande un sottoassieme condiviso una volta sola, sotto il primo prodotto; qui ogni prodotto e'
	// un gruppo, e il suo albero lo si percorre da capo: i figli di un componente sono quelli della sua prima
	// espansione, e un rimando e' un componente gia' visto IN QUESTO gruppo (o un giro)
	espansi := map[uuid.UUID][]*fascicolo.Nodo{}
	var raccogli func(n *fascicolo.Nodo)
	raccogli = func(n *fascicolo.Nodo) {
		if n.Ripetuto || n.Giro {
			return
		}
		if _, ok := espansi[n.C.ComponenteID]; !ok {
			espansi[n.C.ComponenteID] = n.Figli
		}
		for _, f := range n.Figli {
			raccogli(f)
		}
	}
	for _, r := range d.Albero.Radici {
		raccogli(r)
	}
	percorri := func(radici []*fascicolo.Nodo, fn func(n *fascicolo.Nodo, livello int, rimando bool)) {
		visti := map[uuid.UUID]bool{}
		var visita func(n *fascicolo.Nodo, livello int)
		visita = func(n *fascicolo.Nodo, livello int) {
			id := n.C.ComponenteID
			rimando := visti[id] || n.Giro
			fn(n, livello, rimando)
			if rimando {
				return
			}
			visti[id] = true
			for _, f := range espansi[id] {
				visita(f, livello+1)
			}
		}
		for _, r := range radici {
			visita(r, 0)
		}
	}
	contenuti := func(radici []*fascicolo.Nodo) map[uuid.UUID]bool {
		out := map[uuid.UUID]bool{}
		percorri(radici, func(n *fascicolo.Nodo, _ int, _ bool) { out[n.C.ComponenteID] = true })
		return out
	}
	for _, p := range prodotti {
		c := p.C
		v.Gruppi = append(v.Gruppi, gruppoDoc{Chiave: c.ComponenteID.String(), Nome: c.Codice, N: len(contenuti([]*fascicolo.Nodo{p})), Prodotto: &c})
	}
	if len(fuori) > 0 {
		v.Gruppi = append(v.Gruppi, gruppoDoc{Chiave: gruppoFuori, Nome: "Fuori dalla struttura", N: len(contenuti(fuori))})
	}
	if len(daAssociare) > 0 {
		v.Gruppi = append(v.Gruppi, gruppoDoc{Chiave: gruppoFile, Nome: "Da associare", N: len(daAssociare)})
	}
	if len(dellaRfq) > 0 {
		v.Gruppi = append(v.Gruppi, gruppoDoc{Chiave: gruppoRfq, Nome: "Documenti della RFQ", N: len(dellaRfq)})
	}
	if len(v.Gruppi) == 0 {
		v.Vuota = "Niente da mostrare: la RFQ non ha ancora componenti né file. I codici della richiesta, confermati, diventano prodotti da soli; i file arrivano con la posta, con «+ Aggiungi file» o dal NAS."
		return v
	}

	// il gruppo aperto: quello dell'indirizzo, altrimenti quello del nodo o del file, altrimenti il primo
	scegli := func(k string) *gruppoDoc {
		for i := range v.Gruppi {
			if v.Gruppi[i].Chiave == k {
				return &v.Gruppi[i]
			}
		}
		return nil
	}
	st := d.Stato
	if st.Nodo == uuid.Nil && st.File != uuid.Nil {
		for comp, ff := range fileDi {
			for _, f := range ff {
				if f.A.AllegatoID == st.File {
					st.Nodo = comp
				}
			}
		}
	}
	v.Gruppo = scegli(st.Gruppo)
	if v.Gruppo == nil && st.Nodo != uuid.Nil {
		for _, p := range prodotti {
			if contenuti([]*fascicolo.Nodo{p})[st.Nodo] {
				v.Gruppo = scegli(p.C.ComponenteID.String())
				break
			}
		}
		if v.Gruppo == nil && contenuti(fuori)[st.Nodo] {
			v.Gruppo = scegli(gruppoFuori)
		}
	}
	if v.Gruppo == nil && st.File != uuid.Nil {
		for _, f := range daAssociare {
			if f.A.AllegatoID == st.File {
				v.Gruppo = scegli(gruppoFile)
			}
		}
		for _, f := range dellaRfq {
			if f.A.AllegatoID == st.File {
				v.Gruppo = scegli(gruppoRfq)
			}
		}
	}
	if v.Gruppo == nil {
		v.Gruppo = &v.Gruppi[0]
	}
	v.Indice.Gruppo = v.Gruppo.Chiave

	// l'albero e il filmstrip del gruppo
	aggiungiRiga := func(n *fascicolo.Nodo, livello int, rimando bool) {
		c := n.C
		r := rigaDoc{C: c, Livello: livello, Qta: n.Qta, Padri: n.Padri, Ripetuto: rimando,
			Celle: d.Completezza[c.ComponenteID], Step: d.Step(c.ComponenteID), Decidere: decidere[c.ComponenteID], NNote: nNote(c.ComponenteID)}
		v.Righe = append(v.Righe, r)
	}
	var radici []*fascicolo.Nodo
	switch v.Gruppo.Chiave {
	case gruppoFuori:
		radici = fuori
	case gruppoFile, gruppoRfq:
	default:
		for _, p := range prodotti {
			if p.C.ComponenteID.String() == v.Gruppo.Chiave {
				radici = []*fascicolo.Nodo{p}
			}
		}
	}
	percorri(radici, aggiungiRiga)
	visto := map[uuid.UUID]bool{}
	elemento := func(c db.Componente) elementoIndice {
		e := elementoIndice{K: "c:" + c.ComponenteID.String(), Codice: c.Codice, Desc: c.Descrizione.String, Comp: c.ComponenteID.String()}
		for _, f := range fileDi[c.ComponenteID] {
			e.File = append(e.File, fileIndice(f))
		}
		return e
	}
	for _, r := range v.Righe {
		if visto[r.C.ComponenteID] {
			continue
		}
		visto[r.C.ComponenteID] = true
		v.Film = append(v.Film, tileDi(r, fileDi[r.C.ComponenteID]))
		v.Indice.Elementi = append(v.Indice.Elementi, elemento(r.C))
	}
	var fileGruppo []rigaFile
	switch v.Gruppo.Chiave {
	case gruppoFile:
		fileGruppo = daAssociare
	case gruppoRfq:
		fileGruppo = dellaRfq
	}
	for _, f := range fileGruppo {
		k := "f:" + f.A.AllegatoID.String()
		t := tileDoc{Chiave: k, Codice: f.A.NomeFile, Titolo: f.A.NomeFile, Stato: "manca"}
		if servibile(f) {
			t.Anteprima, t.Stato = f.A.AllegatoID, "proposta"
			if f.Doc != nil {
				t.Stato = "doc"
			}
		} else if f.Pdf() {
			t.Stato = "formato"
		}
		if f.A.Sha256.Valid {
			t.NNote = len(perSha[f.A.Sha256.String])
		}
		v.Film = append(v.Film, t)
		v.Indice.Elementi = append(v.Indice.Elementi, elementoIndice{K: k, Codice: f.A.NomeFile, File: []fileIndi{fileIndice(f)}})
	}

	// l'elemento scelto: il nodo o il file dell'indirizzo, se e' del gruppo; altrimenti il primo
	for _, e := range v.Indice.Elementi {
		if (st.Nodo != uuid.Nil && e.K == "c:"+st.Nodo.String()) || (st.File != uuid.Nil && e.K == "f:"+st.File.String()) {
			v.Scelto = e.K
		}
	}
	if v.Scelto == "" && len(v.Indice.Elementi) > 0 {
		v.Scelto = v.Indice.Elementi[0].K
	}
	v.Indice.Scelto = v.Scelto

	// la sezione dell'elemento scelto
	if v.Scelto != "" {
		s := &sezioneDoc{Chiave: v.Scelto}
		var files []rigaFile
		comp := uuid.Nil
		if id, err := uuid.Parse(strings.TrimPrefix(v.Scelto, "c:")); err == nil && strings.HasPrefix(v.Scelto, "c:") {
			c := d.Componenti[id]
			s.C, comp = &c, id
			s.Celle, s.Step = d.Completezza[id], d.Step(id)
			s.Padri = padriNellAlbero(d.Albero, id)
			files = fileDi[id]
		} else if id, err := uuid.Parse(strings.TrimPrefix(v.Scelto, "f:")); err == nil {
			files = []rigaFile{perAllegato[id]}
		}
		for _, f := range files {
			fd := fileDoc{F: f, Voce: voce[f.A.AllegatoID], Servibile: servibile(f), Etichetta: etichettaTipoDoc(f.Tipo),
				Note: noteDi(f, comp), CodiceLetto: f.CodiceLetto}
			if f.Doc != nil {
				fd.Spostabile, fd.Fermo = spostabile(*f.Doc, d.Bloccata)
				if fd.Spostabile && !f.Interno() {
					// una versione interna sostituisce solo con un motivo: da qui non si puo' dire, si fa dall'Elenco file
					fd.Sostituibili = sostituibiliDa(d, *f.Doc)
				}
				// la revisione che questo documento ha sostituito, e le sue note
				if x, a, ok := precedenteDi(f); ok {
					fd.NotePrecedenti, fd.AllegatoPrecedente, fd.NomePrecedente = len(perSha[x.Sha256]), a, x.NomeFile
				}
			}
			s.File = append(s.File, fd)
		}
		// il file aperto: quello dell'indirizzo se e' di questo elemento, altrimenti il primo che si vede
		for _, f := range s.File {
			if f.F.A.AllegatoID == st.File {
				s.Scelto = st.File
			}
		}
		if s.Scelto == uuid.Nil {
			for _, f := range s.File {
				if f.Servibile {
					s.Scelto = f.F.A.AllegatoID
					break
				}
			}
		}
		if s.Scelto == uuid.Nil && len(s.File) > 0 {
			s.Scelto = s.File[0].F.A.AllegatoID
		}
		v.Sezione = s
		if s.Scelto != uuid.Nil {
			v.Indice.File = s.Scelto.String()
		}
	}
	// le note di tutti i file del gruppo, per il viewer, e quelle della revisione che ciascun documento ha
	// sostituito («Vedi» le apre anche per un elemento scelto nel browser, senza chiedere niente al server)
	for _, e := range v.Indice.Elementi {
		comp, _ := uuid.Parse(e.Comp)
		for _, fi := range e.File {
			id, _ := uuid.Parse(fi.A)
			f, ok := perAllegato[id]
			if !ok {
				continue
			}
			if nn := noteDi(f, comp); len(nn) > 0 {
				v.Indice.Note[fi.A] = nn
			}
			if x, a, ok := precedenteDi(f); ok && a != uuid.Nil && len(perSha[x.Sha256]) > 0 {
				v.Indice.Prec[fi.A] = precedente{Allegato: a.String(), Nome: x.NomeFile, Note: len(perSha[x.Sha256])}
				if pf, ok := perAllegato[a]; ok {
					v.Indice.Note[a.String()] = noteDi(pf, comp)
				}
			}
		}
	}

	// gli STEP con una struttura proposta da rivedere nell'editor
	if d.Bloccata == 0 {
		for _, s := range d.Piano.Strutture {
			if s.Nodi+s.Archi > 0 {
				v.Proposte = append(v.Proposte, propostaStruttura{Allegato: s.Allegato, Nome: s.Nome, Nodi: s.Nodi, Archi: s.Archi})
			}
		}
	}
	return v
}

// sostituibiliDa sono i documenti correnti dello stesso tipo, negli altri componenti della BOM, che il documento
// spostato puo' sostituire: uno per documento, nell'ordine dell'albero.
func sostituibiliDa(d *fascicoloDati, doc db.Documento) []sostituibile {
	var out []sostituibile
	visti := map[uuid.UUID]bool{}
	for _, r := range d.Albero.Righe {
		c := r.C
		if visti[c.ComponenteID] || (doc.ComponenteID.Valid && c.ComponenteID == doc.ComponenteID.UUID) || c.ArchiviatoIl != nil {
			continue
		}
		visti[c.ComponenteID] = true
		for _, x := range d.DocumentiDi[c.ComponenteID] {
			// lo STEP strutturale vuole la risposta sul riferimento: da qui non si da', si fa dall'Elenco file
			strutturale := c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == x.DocumentoID
			if x.Tipo == doc.Tipo && !x.SostituitoDa.Valid && x.DocumentoID != doc.DocumentoID && !strutturale {
				out = append(out, sostituibile{Doc: x.DocumentoID, Nome: x.NomeFile, Codice: c.Codice})
			}
		}
	}
	return out
}

// fileIndice e' un file nell'indice del viewer.
func fileIndice(f rigaFile) fileIndi {
	stato := "proposta"
	if f.Doc != nil {
		stato = "doc"
	}
	return fileIndi{A: f.A.AllegatoID.String(), Nome: f.A.NomeFile, Tipo: etichettaTipoDoc(f.Tipo), Pdf: f.Pdf(), Ok: servibile(f), Stato: stato}
}

// tileDi e' la miniatura di un componente: il 2D corrente, altrimenti il 2D in arrivo (tratteggiato), altrimenti
// «un 2D c'e' ma non si vede» (un TIF, un DWG, un PDF non ancora sceso), altrimenti «disegno non presente».
func tileDi(r rigaDoc, files []rigaFile) tileDoc {
	t := tileDoc{Chiave: r.Chiave(), Codice: r.C.Codice, Titolo: r.C.Codice + " " + r.C.Descrizione.String, Stato: "manca",
		Condiviso: r.Padri > 1, Decidere: r.Decidere, NNote: r.NNote}
	var formato bool
	for _, pass := range []bool{true, false} { // prima i documenti confermati, poi le proposte
		for _, f := range files {
			if (f.Doc != nil) != pass || db.TipoDocumento(f.Tipo) != db.TipoDocumentoDisegno2d {
				continue
			}
			if servibile(f) {
				t.Anteprima = f.A.AllegatoID
				t.Stato = map[bool]string{true: "doc", false: "proposta"}[pass]
				return t
			}
			formato = true
		}
	}
	if formato {
		t.Stato = "formato"
	}
	return t
}

// spostabile dice se un documento confermato si puo' spostare su un altro componente da qui: con la BOM
// congelata no; se e' gia' scritto sul NAS, cambiare componente vorrebbe dire spostarlo sul NAS (B8.8),
// quindi no. Il server lo rifiuterebbe comunque: qui lo si dice prima.
func spostabile(doc db.Documento, bloccata int32) (bool, string) {
	switch {
	case bloccata > 0:
		return false, "la BOM è congelata: un file si assegna aprendo una revisione"
	case doc.StatoNas == db.StatoNasScritto:
		return false, "è già sul NAS nella cartella del suo componente: spostarlo sul NAS non è ancora possibile da qui"
	}
	return true, ""
}

// ------------------------------------------------------------------ le rotte della vista

// fascicoloSezione: GET /thread/{id}/fascicolo/sezione, con lo stato della pagina. Il pannello dell'elemento
// scelto nella vista Documenti: lo chiede il viewer quando si sceglie un altro componente.
func (s *Server) fascicoloSezione(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	st := leggiStatoFascicolo(r.URL.Query())
	st.Vista = ""
	d, err := s.caricaFascicolo(r.Context(), id, st, utenteDa(r.Context()))
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	s.eseguiFascicolo(w, "fasc_doc_sezione_frammento", d)
}

// vistaDocumenti legge le note e costruisce la vista.
func (s *Server) vistaDocumenti(ctx context.Context, q *db.Queries, d *fascicoloDati, u *db.Utente) (*docVista, error) {
	note, err := q.ListAnnotazioniThread(ctx, d.T.ThreadID)
	if err != nil {
		return nil, err
	}
	var utente uuid.UUID
	if u != nil {
		utente = u.UtenteID
	}
	return costruisciDocumenti(d, note, utente), nil
}

// padriNellAlbero sono i padri di un componente nella working, con la quantita' dell'arco.
func padriNellAlbero(a fascicolo.Albero, comp uuid.UUID) []arcoNodo {
	var out []arcoNodo
	visto := map[uuid.UUID]bool{}
	var visita func(n *fascicolo.Nodo)
	visita = func(n *fascicolo.Nodo) {
		for _, f := range n.Figli {
			if f.C.ComponenteID == comp && !visto[n.C.ComponenteID] {
				visto[n.C.ComponenteID] = true
				out = append(out, arcoNodo{Padre: n.C, Qta: f.Qta})
			}
			if !f.Ripetuto && !f.Giro {
				visita(f)
			}
		}
	}
	for _, r := range a.Radici {
		visita(r)
	}
	return out
}
