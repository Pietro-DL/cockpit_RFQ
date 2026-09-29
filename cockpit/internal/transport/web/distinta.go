package web

// La Distinta (cockpit/_fasi/PROPOSTA_DISTINTA.md): la stessa RFQ del Fascicolo, in quattro passi nell'ordine
// del lavoro — Richiesta, Distinta, Documenti e NAS, Fattibilita' — con i dati e i gesti che ci sono gia'.
//
// La schermata legge caricaThread (la mail, il cliente) e caricaFascicolo (la BOM, il piano, la completezza,
// le versioni), e non scrive niente. I gesti sono le rotte di sempre: quando la richiesta viene da qui
// (HX-Current-URL, dallaDistinta) threadFrammento risponde con il corpo di questa pagina e l'avviso. La
// struttura la disegna distinta.mjs e la manda a /fascicolo/bom/applica, con la stessa StrutturaVoluta
// dell'editor; i disegni e le note sono quelli della vista Documenti (allegato/{id}/anteprima,
// fascicolo/nota). Il Fascicolo resta, per i gesti che qui non ci sono.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraDistinta(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/distinta", s.autenticato(s.distintaPagina))
	mux.HandleFunc("GET /thread/{id}/distinta/dati", s.autenticato(s.distintaDati))
	mux.HandleFunc("GET /thread/{id}/distinta/tipo", s.autenticato(s.distintaTipo))
}

// I passi, nell'ordine del lavoro.
var passiDistinta = []struct{ Chiave, Nome string }{
	{"richiesta", "Richiesta"},
	{"distinta", "Distinta"},
	{"documenti", "Documenti e NAS"},
	{"fattibilita", "Fattibilità"},
}

func passoValido(p string) string {
	for _, x := range passiDistinta {
		if x.Chiave == p {
			return p
		}
	}
	return "distinta"
}

// dallaDistinta dice se la richiesta viene da questa pagina, e da quale passo.
func dallaDistinta(h interface{ Get(string) string }, thread uuid.UUID) (string, bool) {
	u, err := url.Parse(h.Get("HX-Current-URL"))
	if err != nil || u.Path == "" {
		return "", false
	}
	if strings.TrimSuffix(u.Path, "/") != "/thread/"+thread.String()+"/distinta" {
		return "", false
	}
	return passoValido(u.Query().Get("passo")), true
}

// passoVista e' una linguetta: il numero, il nome e lo stato in una riga.
type passoVista struct {
	Numero        int
	Chiave, Nome  string
	Stato, Classe string // Classe: ok, warn, bad, neu
	Attivo, Dopo  bool
}

// regoleVista e' quello che il cliente chiede, dalle regole dell'anagrafica.
type regoleVista struct {
	RispostaEntroGg int      `json:"risposta_entro_gg"`
	RichiedeCbd     bool     `json:"richiede_cbd"`
	DatiRichiesti   []string `json:"dati_richiesti"`
	LinguaRisposta  string   `json:"lingua_risposta"`
}

// fileDistinta e' un file nella pagina Documenti: il file, la sua voce del piano (se e' ancora da confermare) e che
// cosa se ne fa.
type fileDistinta struct {
	F    rigaFile
	Voce *fascicolo.VoceFile
	// Stato: confermato, pronto, decidere, attesa, generale (confermato senza componente), parte (messo da parte).
	Stato   string
	Domanda string
	Apre    bool // PDF che si apre nel visore
}

// bloccoDistinta e' un pezzo della BOM con i suoi requisiti e i suoi file.
type bloccoDistinta struct {
	N           *fascicolo.Nodo
	PadreCodice string
	Celle       []cella
	File        []fileDistinta
}

// rigaQta e' un pezzo con la quantita' complessiva, per la Fattibilita'.
type rigaQta struct {
	C      db.Componente
	Totale int64
}

type distintaVista struct {
	F        *fascicoloDati
	Th       *threadDati
	Base     string // /thread/{id}/distinta
	Passo    string
	Passi    []passoVista
	Prec     *passoVista
	Succ     *passoVista
	Avviso   string
	AvvisoNo bool
	Regole   regoleVista
	Prodotti []db.Componente
	// GiorniScadenza: fra quanti giorni scade la richiesta (negativo = scaduta); nil = senza scadenza.

	Blocchi     []bloccoDistinta
	Generali    []fileDistinta
	DaSistemare []fileDistinta
	DaParte     []fileDistinta
	Archivi     []fileDistinta // archivi gia' aperti: i loro file sono elencati uno per uno
	NPronti     int
	NFile       int

	Produrre, Comprare []rigaQta

	// ScadenzaFrase e ScadenzaClasse: fra quanti giorni scade, detto per chi legge (ok, warn, bad).
	ScadenzaFrase, ScadenzaClasse string

	// Dati e' il JSON per distinta.mjs: i prodotti, i PDF di ogni componente e di tutta la RFQ, le note, la
	// completezza. Nel template sta in un <script type="application/json">: html/template lo codifica.
	Dati datiDistinta
}

// pdfDistinta e' un PDF della RFQ per le miniature e il visore.
type pdfDistinta struct {
	A     string `json:"a"`
	Nome  string `json:"nome"`
	Tipo  string `json:"tipo"`
	Ok    bool   `json:"ok"` // si apre: nello staging, o scritto sul NAS
	Comp  string `json:"comp,omitempty"`
	CC    string `json:"cc,omitempty"` // il codice del componente
	Stato string `json:"stato"`        // doc, proposta
}

type prodottoDistinta struct {
	ID     string `json:"id"`
	Codice string `json:"codice"`
	Desc   string `json:"desc,omitempty"`
}

type datiDistinta struct {
	Pagina   string                   `json:"pagina"`
	Base     string                   `json:"base"` // /thread/{id}/fascicolo: le rotte dei gesti
	Thread   string                   `json:"thread"`
	Scrive   bool                     `json:"scrive"`
	Bloccata int32                    `json:"bloccata"`
	Prodotti []prodottoDistinta       `json:"prodotti"`
	Pdf      map[string][]pdfDistinta `json:"pdf"` // componente → i suoi PDF, il disegno 2D per primo
	Tutti    []pdfDistinta            `json:"tutti"`
	Note     map[string][]notaDoc     `json:"note"` // allegato → note sul suo contenuto
	// Celle: componente → i requisiti del fascicolo, come nella griglia (etichetta, classe, simbolo, titolo).
	Celle map[string][]cellaDistinta `json:"celle"`
}

type cellaDistinta struct {
	E string `json:"e"` // 3D, 2D, DXF…
	C string `json:"c"` // ok, coda, urg, warn, info, no
	S string `json:"s"`
	T string `json:"t"`
}

func (s *Server) distintaPagina(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	v, err := s.caricaDistinta(r.Context(), id, passoValido(r.URL.Query().Get("passo")), r)
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	s.rendi(w, r, "distinta.html", "distinta_corpo", "Distinta", v)
}

// rispondiDistinta e' la risposta di un gesto partito da questa pagina: il corpo con l'avviso.
func (s *Server) rispondiDistinta(w http.ResponseWriter, r *http.Request, id uuid.UUID, passo, avviso string) {
	v, err := s.caricaDistinta(r.Context(), id, passo, r)
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	v.Avviso = avviso
	v.AvvisoNo = avvisoNegativo(avviso)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["distinta.html"].ExecuteTemplate(w, "distinta_corpo", vista{Utente: utenteDa(r.Context()), Titolo: "Distinta", Dati: v, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "distinta_corpo", "err", err)
	}
}

// avvisoNegativo: le frasi con cui un gesto dice che non ha fatto niente (le stesse di fasc_avviso).
func avvisoNegativo(a string) bool {
	for _, p := range []string{"Niente è cambiato", "Assegnazione non riuscita", "Conferma non riuscita", "Correzione non riuscita", "Preparazione dei file non riuscita"} {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

func (s *Server) caricaDistinta(ctx context.Context, id uuid.UUID, passo string, r *http.Request) (*distintaVista, error) {
	u := utenteDa(ctx)
	f, err := s.caricaFascicolo(ctx, id, statoFascicolo{}, u)
	if err != nil {
		return nil, err
	}
	th, err := s.caricaThread(ctx, id, sessioneDa(ctx))
	if err != nil {
		return nil, err
	}
	v := &distintaVista{F: f, Th: th, Base: "/thread/" + id.String() + "/distinta", Passo: passo}
	if len(f.Cliente.Regole) > 0 {
		_ = json.Unmarshal(f.Cliente.Regole, &v.Regole)
	}
	if sc := f.T.DataScadenza; sc != nil {
		oggi := time.Now()
		a := time.Date(oggi.Year(), oggi.Month(), oggi.Day(), 0, 0, 0, 0, time.Local)
		l := sc.In(time.Local)
		b := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
		g := int(b.Sub(a).Hours() / 24)
		switch {
		case g < 0:
			v.ScadenzaFrase, v.ScadenzaClasse = "scaduta da "+fraseConta(-g, "giorno", "giorni"), "bad"
		case g == 0:
			v.ScadenzaFrase, v.ScadenzaClasse = "scade oggi", "bad"
		case g <= 7:
			v.ScadenzaFrase, v.ScadenzaClasse = "fra "+fraseConta(g, "giorno", "giorni"), "warn"
		default:
			v.ScadenzaFrase, v.ScadenzaClasse = "fra "+fraseConta(g, "giorno", "giorni"), "ok"
		}
	}
	for _, n := range f.Albero.Radici {
		if n.C.Tipo == db.TipoComponenteFinito {
			v.Prodotti = append(v.Prodotti, n.C)
		}
	}
	s.documentiDistinta(v)
	for _, n := range f.Albero.Righe {
		rq := rigaQta{C: n.C, Totale: f.Albero.Totali[n.C.ComponenteID]}
		if n.C.Tipo == db.TipoComponenteCommerciale {
			v.Comprare = append(v.Comprare, rq)
		} else {
			v.Produrre = append(v.Produrre, rq)
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
	dati, err := s.datiPerDistinta(ctx, v, u)
	if err != nil {
		return nil, err
	}
	v.Dati = dati
	return v, nil
}

// documentiDistinta mette ogni file della RFQ al suo posto: sotto il pezzo a cui e' confermato o a cui il piano
// lo manda, fra i documenti della richiesta, fra quelli da sistemare o messi da parte.
func (s *Server) documentiDistinta(v *distintaVista) {
	f := v.F
	voce := map[uuid.UUID]*fascicolo.VoceFile{}
	for i := range f.Piano.File {
		voce[f.Piano.File[i].Allegato] = &f.Piano.File[i]
	}
	perComp := map[uuid.UUID][]fileDistinta{}
	figliDi := map[uuid.UUID]int{}
	for _, x := range f.File {
		if x.A.ContenitoreID.Valid {
			figliDi[x.A.ContenitoreID.UUID]++
		}
	}
	for _, x := range f.File {
		rd := fileDistinta{F: x, Voce: voce[x.A.AllegatoID], Apre: servibile(x)}
		if x.Doc == nil && strings.EqualFold(x.estensione(), "zip") {
			// un archivio non e' un documento: o i suoi file sono gia' qui uno per uno, o non e' stato aperto
			rd.Stato = "archivio"
			if n := figliDi[x.A.AllegatoID]; n > 0 {
				rd.Domanda = "Archivio aperto: i suoi " + fraseConta(n, "file è", "file sono") + " qui sopra, uno per uno."
				v.Archivi = append(v.Archivi, rd)
			} else if x.Proposta == nil || x.Proposta.Stato == db.StatoPropostaAperta {
				rd.Domanda = "Archivio non aperto: il suo contenuto non è elencato. Se contiene dei file utili (un DXF, un disegno), segnalalo: per ora si apre a mano."
				v.DaSistemare = append(v.DaSistemare, rd)
			}
			continue
		}
		v.NFile++
		switch {
		case x.Doc != nil && x.Doc.ComponenteID.Valid:
			rd.Stato = "confermato"
			perComp[x.Doc.ComponenteID.UUID] = append(perComp[x.Doc.ComponenteID.UUID], rd)
		case x.Doc != nil:
			rd.Stato = "generale"
			v.Generali = append(v.Generali, rd)
		case x.Rumore():
			rd.Stato = "parte"
			v.DaParte = append(v.DaParte, rd)
		case rd.Voce != nil:
			rd.Stato = string(rd.Voce.Stato)
			if len(rd.Voce.Domande) > 0 {
				rd.Domanda = domandaDistinta(rd.Voce)
			}
			if rd.Stato == string(fascicolo.VocePronta) {
				v.NPronti++
			}
			if rd.Voce.Componente != nil {
				perComp[rd.Voce.Componente.ComponenteID] = append(perComp[rd.Voce.Componente.ComponenteID], rd)
			} else {
				v.DaSistemare = append(v.DaSistemare, rd)
			}
		default:
			rd.Stato = "attesa"
			v.DaSistemare = append(v.DaSistemare, rd)
		}
	}
	for _, n := range f.Albero.Righe {
		b := bloccoDistinta{N: n, Celle: f.Completezza[n.C.ComponenteID], File: perComp[n.C.ComponenteID]}
		if n.Padre != uuid.Nil {
			b.PadreCodice = f.CodiceDi(n.Padre)
		}
		sort.SliceStable(b.File, func(i, j int) bool {
			oi, oj := ordineTipo(b.File[i].F.Tipo), ordineTipo(b.File[j].F.Tipo)
			if oi != oj {
				return oi < oj
			}
			return b.File[i].F.A.NomeFile < b.File[j].F.A.NomeFile
		})
		v.Blocchi = append(v.Blocchi, b)
	}
}

// passiDistinta dice lo stato di ogni passo in una riga.
func (s *Server) passiDistinta(v *distintaVista) []passoVista {
	f := v.F
	out := make([]passoVista, 0, len(passiDistinta))
	for i, p := range passiDistinta {
		pv := passoVista{Numero: i + 1, Chiave: p.Chiave, Nome: p.Nome, Attivo: p.Chiave == v.Passo}
		switch p.Chiave {
		case "richiesta":
			n := len(v.Th.Messaggi)
			pv.Stato, pv.Classe = fraseConta(n, "messaggio", "messaggi"), "ok"
		case "distinta":
			switch {
			case f.Bloccata > 0:
				pv.Stato, pv.Classe = "congelata nella V"+itoa(int(f.Bloccata)), "ok"
			case f.NProposte > 0:
				pv.Stato, pv.Classe = fraseConta(f.NProposte, "proposta da decidere", "proposte da decidere"), "warn"
			case f.Albero.Componenti() <= len(v.Prodotti):
				pv.Stato, pv.Classe = "solo il prodotto", "warn"
			default:
				pv.Stato, pv.Classe = fraseConta(f.Albero.Componenti(), "pezzo", "pezzi"), "ok"
			}
		case "documenti":
			switch {
			case f.NBloccanti > 0:
				pv.Stato, pv.Classe = fraseConta(int(f.NBloccanti), "documento obbligatorio manca", "documenti obbligatori mancano"), "bad"
			case f.NDaVerificare() > 0:
				pv.Stato, pv.Classe = fraseConta(f.NDaVerificare(), "da verificare", "da verificare"), "warn"
			case v.NPronti > 0:
				pv.Stato, pv.Classe = fraseConta(v.NPronti, "pronto da confermare", "pronti da confermare"), "warn"
			default:
				pv.Stato, pv.Classe = "a posto", "ok"
			}
		case "fattibilita":
			pv.Dopo = true
			if f.Fase != nil && f.Fase.NomeFase == db.FaseFATTIBILITA {
				pv.Stato, pv.Classe, pv.Dopo = "in corso", "warn", false
			} else {
				pv.Stato, pv.Classe = "fase successiva", "neu"
			}
		}
		out = append(out, pv)
	}
	return out
}

func fraseConta(n int, uno, tanti string) string {
	if n == 1 {
		return "1 " + uno
	}
	return itoa(n) + " " + tanti
}

func itoa(n int) string { return strconv.Itoa(n) }

// datiPerDistinta costruisce il JSON di distinta.mjs: i PDF di ogni componente (il 2D per primo) e di tutta la
// RFQ, con le note per contenuto, come nella vista Documenti.
func (s *Server) datiPerDistinta(ctx context.Context, v *distintaVista, u *db.Utente) (datiDistinta, error) {
	f := v.F
	out := datiDistinta{Pagina: v.Base, Base: f.Base, Thread: f.T.ThreadID.String(), Scrive: f.Scrive, Bloccata: f.Bloccata,
		Pdf: map[string][]pdfDistinta{}, Note: map[string][]notaDoc{}, Celle: map[string][]cellaDistinta{}, Tutti: []pdfDistinta{}, Prodotti: []prodottoDistinta{}}
	for id, cc := range f.Completezza {
		for _, c := range cc {
			out.Celle[id.String()] = append(out.Celle[id.String()], cellaDistinta{E: c.Etichetta, C: c.Classe, S: c.Simbolo, T: c.Titolo})
		}
	}
	for _, p := range v.Prodotti {
		out.Prodotti = append(out.Prodotti, prodottoDistinta{ID: p.ComponenteID.String(), Codice: p.Codice, Desc: p.Descrizione.String})
	}
	note, err := db.New(s.Pool).ListAnnotazioniThread(ctx, f.T.ThreadID)
	if err != nil {
		return out, err
	}
	perSha := map[string][]db.ListAnnotazioniThreadRow{}
	for _, n := range note {
		if n.Sha256.Valid {
			perSha[n.Sha256.String] = append(perSha[n.Sha256.String], n)
		}
	}
	var utente uuid.UUID
	if u != nil {
		utente = u.UtenteID
	}
	comp := func(x rigaFile) uuid.UUID {
		switch {
		case x.Doc != nil && x.Doc.ComponenteID.Valid:
			return x.Doc.ComponenteID.UUID
		case x.Proposta != nil && x.Proposta.ComponenteID.Valid:
			return x.Proposta.ComponenteID.UUID
		}
		for _, vf := range f.Piano.File {
			if vf.Allegato == x.A.AllegatoID && vf.Componente != nil {
				return vf.Componente.ComponenteID
			}
		}
		return uuid.Nil
	}
	for _, x := range f.File {
		if !x.Pdf() {
			continue
		}
		p := pdfDistinta{A: x.A.AllegatoID.String(), Nome: x.A.NomeFile, Tipo: x.Tipo, Ok: servibile(x), Stato: "proposta"}
		if x.Doc != nil {
			p.Stato = "doc"
		}
		if c := comp(x); c != uuid.Nil {
			p.Comp, p.CC = c.String(), f.CodiceDi(c)
			out.Pdf[p.Comp] = append(out.Pdf[p.Comp], p)
		}
		out.Tutti = append(out.Tutti, p)
		if x.A.Sha256.Valid {
			for i, n := range perSha[x.A.Sha256.String] {
				nd := notaDoc{ID: n.AnnotazioneID, N: i + 1, Pagina: n.Pagina, X: n.X, Y: n.Y, Testo: n.Testo,
					Autore: n.AutoreNome, Quando: n.CreataIl.Local().Format("02/01/2006 15:04"), Mia: n.CreataDa == utente}
				if n.ComponenteID.Valid && n.ComponenteID.UUID.String() != p.Comp {
					nd.Su = n.CodiceComponente
				}
				out.Note[p.A] = append(out.Note[p.A], nd)
			}
		}
	}
	for k := range out.Pdf {
		sort.SliceStable(out.Pdf[k], func(i, j int) bool {
			a, b := out.Pdf[k][i], out.Pdf[k][j]
			if (a.Tipo == string(db.TipoDocumentoDisegno2d)) != (b.Tipo == string(db.TipoDocumentoDisegno2d)) {
				return a.Tipo == string(db.TipoDocumentoDisegno2d)
			}
			if a.Ok != b.Ok {
				return a.Ok
			}
			return a.Nome < b.Nome
		})
	}
	return out, nil
}

// distintaDati: GET /thread/{id}/distinta/dati. Il JSON della pagina, per rileggere le note dopo un gesto del
// visore senza ricaricare tutto.
func (s *Server) distintaDati(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	v, err := s.caricaDistinta(r.Context(), id, "distinta", r)
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v.Dati)
}

// tipoDistinta e' l'anteprima del cambio di tipo, in JSON: la stessa di anteprimaTipo, con la firma che il
// gesto rimanda.
type tipoDistinta struct {
	Spento  string   `json:"spento,omitempty"`
	Frasi   []string `json:"frasi,omitempty"`
	Bottone string   `json:"bottone,omitempty"`
	Firma   string   `json:"firma,omitempty"`
	Errore  string   `json:"errore,omitempty"`
}

// distintaTipo: GET /thread/{id}/distinta/tipo?componente=…&tipo=…
func (s *Server) distintaTipo(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	var out tipoDistinta
	cid, err := uuid.Parse(r.URL.Query().Get("componente"))
	if err != nil {
		out.Errore = "componente non valido"
	} else {
		e, err := fascicolo.EffettoCambioTipo(r.Context(), db.New(s.Pool), thread, cid, db.TipoComponente(strings.TrimSpace(r.URL.Query().Get("tipo"))))
		if err != nil {
			var rf rifiuto
			if !errors.As(err, &rf) {
				http.Error(w, "anteprima non riuscita", 500)
				return
			}
			out.Errore = spiegaErrore(err)
		} else {
			out.Spento, out.Firma, out.Bottone = e.Spento, e.Firma, e.Bottone()
			for _, x := range e.Sospende {
				out.Frasi = append(out.Frasi, "Si sospende "+x+": il file resta guida e non propone più i figli diretti, finché una persona non la riattiva.")
			}
			if len(e.ConLei) > 0 {
				out.Frasi = append(out.Frasi, "Si sospendono con lei le deleghe di "+strings.Join(e.ConLei, ", ")+".")
			}
			if e.SvuotaStep != "" {
				out.Frasi = append(out.Frasi, e.SvuotaStep+" non è più il suo STEP strutturale.")
			}
			if e.StepStrutturale != "" {
				out.Frasi = append(out.Frasi, e.StepStrutturale+" diventa anche il suo STEP strutturale.")
			}
			for _, a := range e.Avvisi {
				out.Frasi = append(out.Frasi, a+".")
			}
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

// domandaDistinta e' la domanda di una voce del piano con le parole di questa pagina: la struttura si fa nel
// passo 2, non «nell'editor».
func domandaDistinta(v *fascicolo.VoceFile) string {
	d := v.Domande[0]
	switch d.Chiave {
	case fascicolo.DomandaComponente:
		if v.Codice != "" {
			return "Il codice «" + v.Codice + "» non è un pezzo della distinta: scegli qui a destra il pezzo a cui va (il file ne prende il codice), oppure aggiungi il pezzo nel passo 2."
		}
		return "Il file non va ancora a nessun pezzo: scegli qui a destra il pezzo a cui va, oppure aggiungi il pezzo nel passo 2."
	case fascicolo.DomandaStrutturaEditor:
		return "Va a un pezzo che nasce dalla struttura di uno STEP: prima si salva la distinta (passo 2)."
	}
	return d.Testo
}
