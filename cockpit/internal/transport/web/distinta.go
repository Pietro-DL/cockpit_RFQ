package web

// La Distinta (cockpit/_fasi/PROPOSTA_DISTINTA.md): la stessa RFQ del Fascicolo, in quattro passi nell'ordine
// del lavoro — Richiesta, Distinta, Documenti e NAS, Fattibilita' — con i dati e i gesti che ci sono gia'.
//
// La schermata legge caricaThread (la mail, il cliente) e caricaFascicolo (la BOM, il piano, la completezza,
// le versioni), e non scrive niente. I gesti sono le rotte di sempre: quando la richiesta viene da qui
// (HX-Current-URL, dallaDistinta) threadFrammento risponde con il corpo di questa pagina e l'avviso. L'albero
// lo disegna distinta.mjs sull'albero proposto, e lo conferma con /distinta/albero/conferma (distinta_albero.go);
// i disegni e le note sono quelli della vista Documenti (allegato/{id}/anteprima, fascicolo/nota). Il Fascicolo
// resta, per i gesti che qui non ci sono.
//
// Giro 4, fase 4.4a.3 (domande 27, 28, 29, 30, 9a, 10): il passo 2 e' l'albero proposto (distinta_albero.go): la
// pagina lo legge, Luigi lo corregge in una bozza che resta nel browser (per RFQ e per utente) e solo «Conferma
// l'albero» scrive. Il passo 3 non ha piu' il gesto cumulativo sul NAS (9a = A): ogni file si conferma da solo, e la
// conferma dice prima il percorso sul NAS; «Documento della richiesta» e' solo per i file non tecnici, con il tipo
// scelto e la conferma; «Congela la V1» chiede conferma. Le GET della pagina non si tengono in cache (bug 5): dopo un
// gesto il tasto Indietro del browser non ripresenta la pagina di prima.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/core/rfq/documenti"
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
	// Tecnico: il file descrive un pezzo (3D, 2D, DXF) o non si sa ancora che cos'e'. Non diventa un «Documento della
	// richiesta»: quel gesto e' solo per i file non tecnici (domanda 9), e un disegno messo li' per sbaglio andava sul
	// NAS come documento generale.
	Tecnico bool
	// Conferma: la domanda di «✓ Conferma», con il pezzo, il tipo e il percorso che il file prende sul NAS (domanda 9:
	// la conferma chiede, e dice dove va, prima di scrivere e di copiare).
	Conferma string
	// CodiceDaScrivere: il codice con cui parte «che cos'e' questo file?», solo se e' un codice di una famiglia del
	// cliente (bug 8: il nome del file non si precompila come codice). CodiceNelNome: quello che il nome dice, come
	// suggerimento nel campo vuoto.
	CodiceDaScrivere, CodiceNelNome string
}

// padreBlocco e' un padre di un pezzo nel passo 3, con la quantita' del legame.
type padreBlocco struct {
	Codice string
	Qta    int32
}

// bloccoDistinta e' un pezzo della BOM con i suoi requisiti e i suoi file.
type bloccoDistinta struct {
	N           *fascicolo.Nodo
	PadreCodice string
	// Padri: tutti i padri del pezzo con le quantita', e Totale quanti pezzi in tutto (bug 11: il blocco di un pezzo in
	// comune nominava un padre solo). Fuori: una radice che non e' un prodotto, cioe' un pezzo fuori dalla distinta
	// (bug 10: non e' «il prodotto»).
	Padri  []padreBlocco
	Totale int64
	Fuori  bool
	Celle  []cella
	File   []fileDistinta
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
	// NDecidere: i file da decidere che stanno gia' sotto un pezzo (il blocco del pezzo li mostra). Con quelli «da
	// sistemare» sono i file del passo 3 su cui serve una persona: la linguetta conta loro (bug 13), non la struttura,
	// che e' del passo 2.
	NDecidere int

	// Albero: quello che l'albero proposto ha ancora da confermare (fase 4.4a.3): i pezzi e i legami proposti, le
	// rimozioni che lo STEP propone. La linguetta del passo 2 conta quello che «Conferma l'albero» decide. Senza
	// l'albero (una lettura non riuscita) la linguetta conta le proposte come prima (AlberoLetto falso).
	AlberoLetto                                bool
	AlberoPezzi, AlberoLegami, AlberoRimozioni int

	Produrre, Comprare []rigaQta

	// motore: le regole del cliente, per dire se il codice di un file e' di una sua famiglia (bug 8); percorsi: il
	// percorso sul NAS che «✓ Conferma» darebbe a ogni file pronto (solo nel passo 3). Tutti e due li legge
	// caricaDistinta; senza, documentiDistinta resta pura e piu' prudente (niente codice precompilato, la domanda
	// senza il percorso).
	motore   *classificazione.Motore
	percorsi map[uuid.UUID]percorsoFile

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
	// V: l'impronta del contenuto (lo sha256) per l'indirizzo dell'anteprima (cache C1: il browser tiene il file dello
	// staging senza richiederlo). Cod: il codice che il file dice, perche' un pezzo dell'albero proposto (che non e'
	// ancora un componente) mostri il suo disegno con la scritta «proposto» (A5.3.10).
	V   string `json:"v,omitempty"`
	Cod string `json:"cod,omitempty"`
}

// fuoriDistinta e' un componente della RFQ che non sta sotto nessun prodotto: nel passo 2 si rimette nell'albero
// (con la bozza) o si elimina.
type fuoriDistinta struct {
	ID     string `json:"id"`
	Codice string `json:"codice"`
	Tipo   string `json:"tipo"`
	Desc   string `json:"desc,omitempty"`
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
	// Utente: chi guarda. La bozza dell'albero sta nel browser per RFQ e per utente (domanda 28 = A, il primo tempo):
	// su una postazione condivisa un collega non riprende la bozza di un altro.
	Utente string `json:"utente"`
	// Analisi: le analisi ancora in corso sui file della RFQ (l'albero proposto puo' cambiare). Caricamento: il
	// caricamento dei file c'e' su questo server (sta nel Fascicolo completo, «+ Aggiungi file»).
	Analisi     int64 `json:"analisi"`
	Caricamento bool  `json:"caricamento"`
	// Fuori: i componenti che non stanno sotto nessun prodotto.
	Fuori []fuoriDistinta `json:"fuori"`
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
	// la pagina non va in nessuna cache (bug 5 del 29/09): i gesti htmx non cambiano l'indirizzo, e con la cache HTTP il
	// tasto Indietro del browser ripresentava l'HTML di prima del gesto (un file confermato di nuovo «pronto»)
	w.Header().Set("Cache-Control", "no-store")
	v, err := s.caricaDistinta(r.Context(), id, passoValido(r.URL.Query().Get("passo")), r)
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	s.rendi(w, r, "distinta.html", "distinta_corpo", "Distinta", v)
}

// rispondiDistinta e' la risposta di un gesto partito da questa pagina: il corpo con l'avviso.
func (s *Server) rispondiDistinta(w http.ResponseWriter, r *http.Request, id uuid.UUID, passo, avviso string) {
	w.Header().Set("Cache-Control", "no-store")
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

// avvisoNegativo: le frasi con cui un gesto dice che non ha fatto niente (le stesse di fasc_avviso). Anche «La
// proposta era gia' stata decisa: nessun cambiamento.» (allegati.go, il «Metti da parte» su un file che un collega ha
// gia' deciso): usciva verde, come un gesto riuscito (bug 7 del 29/09).
func avvisoNegativo(a string) bool {
	for _, p := range []string{"Niente è cambiato", "Assegnazione non riuscita", "Conferma non riuscita", "Correzione non riuscita", "Preparazione dei file non riuscita",
		"La proposta era già stata decisa"} {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return strings.Contains(a, "nessun cambiamento")
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
	q := db.New(s.Pool)
	lette, _ := regole.LeggiRegole(f.Cliente.Regole)
	v.motore = classificazione.Compila(f.Cliente.RagioneSociale, lette)
	if passo == "documenti" {
		// il percorso sul NAS di ogni file pronto, per la domanda di «✓ Conferma»: senza (una lettura non riuscita) la
		// domanda resta, e dice che il posto lo sceglie la conferma
		if pp, err := percorsiDistinta(ctx, q, id, f.Piano); err != nil {
			s.Log.Warn("distinta: i percorsi sul NAS dei file pronti non si leggono", "rfq", id, "err", err)
		} else {
			v.percorsi = pp
		}
	}
	s.documentiDistinta(v)
	// quello che l'albero proposto ha ancora da confermare, per la linguetta del passo 2 (fase 4.4a.3): la stessa lettura
	// di GET .../distinta/albero, che non scrive
	if a, err := fascicolo.LeggiAlberoProposto(ctx, q, id, s.Analizzatore); err != nil {
		s.Log.Warn("distinta: l'albero proposto non si legge", "rfq", id, "err", err)
	} else {
		v.contaAlbero(a)
	}
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
		rd := fileDistinta{F: x, Voce: voce[x.A.AllegatoID], Apre: servibile(x), Tecnico: fileTecnico(x.Tipo)}
		if rd.Voce != nil {
			rd.CodiceDaScrivere, rd.CodiceNelNome = codiceDaScrivere(v.motore, rd.Voce)
		}
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
				pf, ok := v.percorsi[rd.Voce.Proposta]
				rd.Conferma = fraseConfermaFile(x, rd.Voce, pf, ok)
			}
			if rd.Voce.Componente != nil {
				if rd.Stato == string(fascicolo.VoceDecidere) {
					v.NDecidere++
				}
				perComp[rd.Voce.Componente.ComponenteID] = append(perComp[rd.Voce.Componente.ComponenteID], rd)
			} else {
				v.DaSistemare = append(v.DaSistemare, rd)
			}
		default:
			rd.Stato = "attesa"
			v.DaSistemare = append(v.DaSistemare, rd)
		}
	}
	// i padri di ogni pezzo con le quantita' (bug 11): Albero.Righe ha una riga per componente, con il padre della prima
	// volta; gli altri stanno nei nodi ripetuti. Nel prodotto: quello che una radice che e' un prodotto raggiunge
	padri := map[uuid.UUID][]padreBlocco{}
	nelProdotto := map[uuid.UUID]bool{}
	var scendi func(n *fascicolo.Nodo, prodotto bool)
	scendi = func(n *fascicolo.Nodo, prodotto bool) {
		if prodotto {
			nelProdotto[n.C.ComponenteID] = true
		}
		if n.Padre != uuid.Nil {
			gia := false
			for _, p := range padri[n.C.ComponenteID] {
				gia = gia || p.Codice == f.CodiceDi(n.Padre)
			}
			if !gia {
				padri[n.C.ComponenteID] = append(padri[n.C.ComponenteID], padreBlocco{Codice: f.CodiceDi(n.Padre), Qta: n.Qta})
			}
		}
		for _, x := range n.Figli {
			scendi(x, prodotto)
		}
	}
	for _, n := range f.Albero.Radici {
		scendi(n, n.C.Tipo == db.TipoComponenteFinito)
	}
	var fuori []bloccoDistinta
	for _, n := range f.Albero.Righe {
		b := bloccoDistinta{N: n, Celle: f.Completezza[n.C.ComponenteID], File: perComp[n.C.ComponenteID],
			Padri: padri[n.C.ComponenteID], Totale: f.Albero.Totali[n.C.ComponenteID]}
		if n.Padre != uuid.Nil {
			b.PadreCodice = f.CodiceDi(n.Padre)
		}
		sort.SliceStable(b.Padri, func(i, j int) bool { return b.Padri[i].Codice < b.Padri[j].Codice })
		sort.SliceStable(b.File, func(i, j int) bool {
			oi, oj := ordineTipo(b.File[i].F.Tipo), ordineTipo(b.File[j].F.Tipo)
			if oi != oj {
				return oi < oj
			}
			return b.File[i].F.A.NomeFile < b.File[j].F.A.NomeFile
		})
		// un pezzo che nessun prodotto raggiunge e' fuori dalla distinta: in fondo, e non «il prodotto» (bug 10)
		if !nelProdotto[n.C.ComponenteID] {
			b.Fuori = true
			fuori = append(fuori, b)
			continue
		}
		v.Blocchi = append(v.Blocchi, b)
	}
	v.Blocchi = append(v.Blocchi, fuori...)
}

// fileTecnico: un file che descrive un pezzo (3D, 2D, DXF), o che non si sa ancora che cos'e' (un tipo non
// riconosciuto resta nella strada dei tecnici, come in F8). Non diventa un «Documento della richiesta» (domanda 9).
func fileTecnico(tipo string) bool {
	switch db.TipoDocumento(tipo) {
	case db.TipoDocumentoAltro, db.TipoDocumentoCapitolato, db.TipoDocumentoDistintaCliente, db.TipoDocumentoCommerciale,
		db.TipoDocumentoOffertaFornitore, db.TipoDocumentoOffertaPromatec, db.TipoDocumentoOrdineCliente, db.TipoDocumentoCorrispondenza:
		return false
	}
	return true
}

// codiceDaScrivere: il codice con cui parte il campo di «che cos'e' questo file?» (bug 8). Quello della voce solo se e'
// per intero un codice di una famiglia del cliente: un nome di file («ACME-7120012») non si precompila, e un «Salva»
// distratto non lo registrava come codice di chi decide. Il secondo valore e' quello che il nome dice, per il
// suggerimento nel campo vuoto. Senza le regole del cliente (motore nil) non si precompila niente.
func codiceDaScrivere(m *classificazione.Motore, v *fascicolo.VoceFile) (string, string) {
	c := strings.TrimSpace(v.Codice)
	if c == "" {
		c = strings.TrimSpace(v.Suggerito)
	}
	if c == "" {
		return "", ""
	}
	if m == nil || !m.HaFamiglie() {
		return "", c
	}
	for _, x := range m.Codici(c) {
		if x.Origine == "famiglia" && strings.EqualFold(x.Codice, c) {
			return c, c
		}
	}
	return "", c
}

// percorsoFile e' dove «✓ Conferma» mette un file sul NAS, o perche' non si sa (Errore).
type percorsoFile struct {
	Percorso, Errore string
}

// percorsiDistinta: per ogni file pronto il percorso che «✓ Conferma» gli darebbe sul NAS, dentro la cartella della
// richiesta. Il gesto del passo 3 conferma un file per volta: ogni file si calcola da solo, come se nessun altro file
// pronto prendesse il suo nome prima di lui (il riepilogo di «Conferma Fascicolo», che li conferma tutti, da' il
// progressivo al secondo). Le regole sono quelle della conferma (riepilogoDa: il codice del componente, la cartella del
// tipo e del codice, il nome sul NAS, il primo nome libero; lo stesso contenuto gia' confermato resta dov'e'). Legge
// soltanto.
func percorsiDistinta(ctx context.Context, q *db.Queries, thread uuid.UUID, p fascicolo.PianoFascicolo) (map[uuid.UUID]percorsoFile, error) {
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	pp, err := nuoviPercorsi(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	rp, err := riepilogoDa(p, t.CartellaRelativa.String, func(v fascicolo.VoceFile, codice string, _ map[string]bool) (string, error) {
		if !t.CartellaRelativa.Valid {
			return "", rifiuto("la RFQ non ha una cartella sul NAS")
		}
		if v.Duplicato != "" {
			d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: v.Sha256})
			if errors.Is(err, pgx.ErrNoRows) {
				return "", rifiuto("il documento con lo stesso contenuto (" + v.Duplicato + ") non è nel fascicolo")
			}
			if err != nil {
				return "", err
			}
			return d.PathRelativo, nil
		}
		cartella, err := pp.cartellaPer(ctx, q, v.Tipo, codice)
		if err != nil {
			return "", err
		}
		return documenti.PercorsoPrevisto(ctx, q, thread, cartella, documenti.NomeSulNas(v.Tipo, codice, v.Rev, v.Estensione, v.Nome), map[string]bool{})
	})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]percorsoFile, len(rp.File))
	for _, f := range rp.File {
		out[f.Proposta] = percorsoFile{Percorso: f.Percorso, Errore: f.Errore}
	}
	return out, nil
}

// fraseConfermaFile e' la domanda di «✓ Conferma» (domanda 9): che cosa diventa il file, di quale pezzo, e dove va sul
// NAS. conosciuto: il percorso e' stato calcolato (senza, la domanda lo dice).
func fraseConfermaFile(x rigaFile, v *fascicolo.VoceFile, p percorsoFile, conosciuto bool) string {
	s := "Confermare «" + x.A.NomeFile + "» come " + etichettaTipoDoc(string(v.Tipo))
	if d := v.Destinazione(); d != "" {
		s += " di " + d + "?"
	} else {
		s += " della richiesta, senza pezzo?"
	}
	switch {
	case !conosciuto:
		s += " Il posto sul NAS lo sceglie la conferma, e la copia parte subito."
	case p.Errore != "":
		s += " Il percorso sul NAS non si calcola (" + p.Errore + "): la conferma probabilmente si fermerà."
	case v.Duplicato != "":
		s += fmt.Sprintf(" Ha lo stesso contenuto di %s, già sul NAS in %s: si registra soltanto da dove è arrivato.", v.Duplicato, p.Percorso)
	default:
		s += " Va sul NAS in " + p.Percorso + " (nella cartella della richiesta), e la copia parte subito."
	}
	return s
}

// contaAlbero conta quello che l'albero proposto ha ancora da confermare: i pezzi proposti (non i prodotti, che vengono
// dal triage) e quelli ritrovati per codice con le righe dei file ancora da decidere (il riepilogo li elenca e la
// conferma li decide: senza, una distinta fatta a mano con le proposte dello STEP aperte diceva «niente da confermare»
// e il gate restava fermo, bug 2), i legami proposti, le rimozioni proposte dallo STEP.
func (v *distintaVista) contaAlbero(a fascicolo.AlberoProposto) {
	v.AlberoLetto = true
	for _, n := range a.Nodi {
		if !n.Prodotto && (n.Stato == fascicolo.StatoAlberoProposto || n.Ritrovato != nil) {
			v.AlberoPezzi++
		}
	}
	for _, l := range a.Archi {
		switch l.Stato {
		case fascicolo.StatoAlberoProposto:
			v.AlberoLegami++
		case fascicolo.StatoAlberoTolto:
			v.AlberoRimozioni++
		}
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
			// con l'albero proposto (fase 4.4a.3) la linguetta dice quello che «Conferma l'albero» decide, come il
			// riepilogo: prima contava le proposte dell'autorita', e una distinta gia' fatta a mano diceva «8 proposte da
			// decidere» senza un gesto per deciderle (bug 2)
			switch {
			case f.Bloccata > 0:
				pv.Stato, pv.Classe = "congelata nella V"+itoa(int(f.Bloccata)), "ok"
			case v.AlberoLetto && v.AlberoPezzi+v.AlberoLegami+v.AlberoRimozioni > 0:
				var parti []string
				if v.AlberoPezzi > 0 {
					parti = append(parti, fraseConta(v.AlberoPezzi, "pezzo", "pezzi"))
				}
				if v.AlberoLegami > 0 {
					parti = append(parti, fraseConta(v.AlberoLegami, "legame", "legami"))
				}
				if v.AlberoRimozioni > 0 {
					parti = append(parti, fraseConta(v.AlberoRimozioni, "rimozione", "rimozioni"))
				}
				pv.Stato, pv.Classe = strings.Join(parti, ", ")+" da confermare", "warn"
			case !v.AlberoLetto && f.NProposte > 0:
				pv.Stato, pv.Classe = fraseConta(f.NProposte, "proposta da decidere", "proposte da decidere"), "warn"
			case f.Albero.Componenti() <= len(v.Prodotti):
				pv.Stato, pv.Classe = "solo il prodotto", "warn"
			default:
				pv.Stato, pv.Classe = fraseConta(f.Albero.Componenti(), "pezzo", "pezzi"), "ok"
			}
		case "documenti":
			// i file su cui serve una persona, come li mostra il passo: quelli da sistemare e quelli da decidere sotto un
			// pezzo. La struttura degli STEP e' del passo 2: contarla qui dava un file in piu' delle righe (bug 13)
			switch n := len(v.DaSistemare) + v.NDecidere; {
			case f.NBloccanti > 0:
				pv.Stato, pv.Classe = fraseConta(int(f.NBloccanti), "documento obbligatorio manca", "documenti obbligatori mancano"), "bad"
			case n > 0:
				pv.Stato, pv.Classe = fraseConta(n, "file da sistemare", "file da sistemare"), "warn"
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
		Pdf: map[string][]pdfDistinta{}, Note: map[string][]notaDoc{}, Celle: map[string][]cellaDistinta{}, Tutti: []pdfDistinta{}, Prodotti: []prodottoDistinta{},
		Analisi: f.NAnalisi, Caricamento: f.Caricamento, Fuori: []fuoriDistinta{}}
	if u != nil {
		out.Utente = u.UtenteID.String()
	}
	for id, cc := range f.Completezza {
		for _, c := range cc {
			out.Celle[id.String()] = append(out.Celle[id.String()], cellaDistinta{E: c.Etichetta, C: c.Classe, S: c.Simbolo, T: c.Titolo})
		}
	}
	for _, p := range v.Prodotti {
		out.Prodotti = append(out.Prodotti, prodottoDistinta{ID: p.ComponenteID.String(), Codice: p.Codice, Desc: p.Descrizione.String})
	}
	for _, b := range v.Blocchi {
		if b.Fuori {
			out.Fuori = append(out.Fuori, fuoriDistinta{ID: b.N.C.ComponenteID.String(), Codice: b.N.C.Codice, Tipo: string(b.N.C.Tipo), Desc: b.N.C.Descrizione.String})
		}
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
		p := pdfDistinta{A: x.A.AllegatoID.String(), Nome: x.A.NomeFile, Tipo: x.Tipo, Ok: servibile(x), Stato: "proposta",
			V: impronta(x.A.Sha256.String), Cod: x.Codice}
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
			out.Spento, out.Bottone = e.Spento, e.Bottone()
			// la firma serve solo a chi puo' mandare il gesto: chi consulta vede l'effetto e non la riceve, come
			// nell'anteprima del Fascicolo (fase 4.3, dall'elenco del frontendista; la prova e' in distinta_db_test.go)
			if almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore) {
				out.Firma = e.Firma
			}
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
