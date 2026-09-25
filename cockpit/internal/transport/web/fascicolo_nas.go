package web

// «Importa dal NAS» (B8.7b): un file che sta gia' sul NAS (un disegno salvato a mano, lo STEP di una commessa
// precedente dello stesso cliente) entra nella RFQ come un caricamento interno. A scegliere e' l'operatore, a
// leggere e' il server: un <input type=file> apre il disco del PC di chi guarda, e quando il server sara' sulla
// VM il NAS sara' un disco del server, che il browser non vede. Il file scelto va nella cache dello staging con
// il suo sha256, poi fa la strada di tutti (proposta, analisi, piano, conferma): come «Carica dal PC».
//
// Confinato sotto [nas].radice: ogni percorso arriva dal browser come relativo alla radice, si ripulisce, e si
// apre con os.Root, che rifiuta «..», i percorsi assoluti e i collegamenti che escono dalla radice. La ricerca
// per nome (per codice, di solito) parte dalla cartella del cliente e si ferma dopo un numero di voci e un
// tempo fissati: il NAS e' grande, e la pagina deve rispondere.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

const (
	maxVociCartellaNas = 500   // voci mostrate in una cartella
	maxVisitateNas     = 20000 // voci guardate da una ricerca
	maxTrovatiNas      = 100   // risultati di una ricerca
	tempoRicercaNas    = 3 * time.Second
	minRicercaNas      = 3 // caratteri minimi di una ricerca
)

// nasVoce e' un file o una cartella del NAS nel cassetto.
type nasVoce struct {
	Nome        string
	Percorso    string // relativo alla radice, con le barre in avanti
	Cartella    bool
	Byte        int64
	Modificato  time.Time
	Importabile bool
	Motivo      string // perche' non si importa
}

// nasVista e' il cassetto «Importa dal NAS».
type nasVista struct {
	Radice   string    // il nome della radice, per le briciole
	Dir      string    // la cartella aperta, relativa ("." = la radice)
	Briciole []nasVoce // le cartelle dalla radice a Dir
	Voci     []nasVoce
	Troppe   bool // la cartella ha piu' voci di quelle mostrate
	Cerca    string
	Dove     string // la cartella da cui parte la ricerca
	Trovati  []nasVoce
	Troncata bool // la ricerca si e' fermata al limite
	Errore   string
}

// percorsoNas ripulisce un percorso del browser: relativo alla radice, barre in avanti, niente «..» che esce,
// niente percorsi assoluti o di unita'. "" e "." sono la radice.
func percorsoNas(p string) (string, error) {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	if p == "" {
		return ".", nil
	}
	if strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':') {
		return "", rifiuto("il percorso deve essere relativo alla radice del NAS")
	}
	c := path.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") || !fs.ValidPath(c) {
		return "", rifiuto("il percorso esce dalla radice del NAS")
	}
	return c, nil
}

// radiceNas apre la radice del NAS configurata; il chiamante la chiude.
func (s *Server) radiceNas() (*os.Root, error) {
	if s.NAS == nil || strings.TrimSpace(s.NAS.Radice) == "" {
		return nil, rifiuto("il NAS non è configurato su questo server ([nas].radice)")
	}
	r, err := os.OpenRoot(s.NAS.Radice)
	if err != nil {
		return nil, rifiuto("il NAS non è raggiungibile: " + err.Error())
	}
	return r, nil
}

// esisteCartellaNas dice se rel e' una cartella sotto la radice.
func esisteCartellaNas(r *os.Root, rel string) bool {
	if rel == "" {
		return false
	}
	c, err := percorsoNas(rel)
	if err != nil {
		return false
	}
	st, err := r.Stat(c)
	return err == nil && st.IsDir()
}

// partenzaNas e' la cartella da cui si comincia: quella della RFQ se c'e' gia' sul NAS, altrimenti quella del
// cliente, altrimenti la radice.
func partenzaNas(r *os.Root, d *fascicoloDati) string {
	if d.T.CartellaRelativa.Valid && esisteCartellaNas(r, d.T.CartellaRelativa.String) {
		c, _ := percorsoNas(d.T.CartellaRelativa.String)
		return c
	}
	if esisteCartellaNas(r, d.Cliente.CartellaNas) {
		c, _ := percorsoNas(d.Cliente.CartellaNas)
		return c
	}
	return "."
}

// nasVista legge il cassetto: la cartella aperta e, se c'e' una ricerca, i file trovati.
func (s *Server) nasVista(ctx context.Context, q *db.Queries, d *fascicoloDati) *nasVista {
	v := &nasVista{Cerca: d.Stato.NasCerca}
	r, err := s.radiceNas()
	if err != nil {
		v.Errore = spiegaErrore(err)
		return v
	}
	defer r.Close()
	v.Radice = path.Base(strings.ReplaceAll(s.NAS.Radice, `\`, "/"))
	dir := d.Stato.Nas
	if dir == "" {
		dir = partenzaNas(r, d)
	}
	if dir, err = percorsoNas(dir); err != nil {
		v.Errore = spiegaErrore(err)
		dir = "."
	}
	v.Dir = dir
	if dir != "." {
		acc := ""
		for _, pezzo := range strings.Split(dir, "/") {
			acc = path.Join(acc, pezzo)
			v.Briciole = append(v.Briciole, nasVoce{Nome: pezzo, Percorso: acc, Cartella: true})
		}
	}
	max := s.maxCaricamentoEffettivo()
	v.Voci, v.Troppe, err = listaNas(r, dir, max)
	if err != nil && v.Errore == "" {
		v.Errore = "la cartella non si è potuta leggere: " + err.Error()
	}
	if len([]rune(v.Cerca)) >= minRicercaNas {
		v.Dove = partenzaRicerca(r, d)
		v.Trovati, v.Troncata = cercaNas(ctx, r, v.Dove, v.Cerca, max)
	}
	return v
}

// partenzaRicerca: la cartella del cliente, dove stanno le sue RFQ precedenti; se non c'e', la radice.
func partenzaRicerca(r *os.Root, d *fascicoloDati) string {
	if esisteCartellaNas(r, d.Cliente.CartellaNas) {
		c, _ := percorsoNas(d.Cliente.CartellaNas)
		return c
	}
	return "."
}

func voceNas(rel string, e fs.DirEntry, max int64) nasVoce {
	v := nasVoce{Nome: e.Name(), Percorso: path.Join(rel, e.Name()), Cartella: e.IsDir()}
	if info, err := e.Info(); err == nil {
		v.Byte, v.Modificato = info.Size(), info.ModTime()
		if !v.Cartella && !info.Mode().IsRegular() {
			v.Motivo = "non è un file"
		}
	}
	if !v.Cartella && v.Motivo == "" {
		switch {
		case !tecnicoCaricabile(v.Nome):
			v.Motivo = "si importano CAD 3D, disegni (PDF, DWG) e sviluppi DXF"
		case v.Byte > max:
			v.Motivo = fmt.Sprintf("oltre il limite di %d MB", max>>20)
		case v.Byte == 0:
			v.Motivo = "vuoto"
		default:
			v.Importabile = true
		}
	}
	return v
}

// listaNas elenca una cartella: prima le cartelle, poi i file, per nome.
func listaNas(r *os.Root, rel string, max int64) ([]nasVoce, bool, error) {
	f, err := r.Open(rel)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	voci, err := f.ReadDir(maxVociCartellaNas + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	troppe := len(voci) > maxVociCartellaNas
	if troppe {
		voci = voci[:maxVociCartellaNas]
	}
	out := make([]nasVoce, 0, len(voci))
	for _, e := range voci {
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "~$") {
			continue
		}
		out = append(out, voceNas(rel, e, max))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Cartella != out[j].Cartella {
			return out[i].Cartella
		}
		return strings.ToLower(out[i].Nome) < strings.ToLower(out[j].Nome)
	})
	return out, troppe, nil
}

// cercaNas cerca i file il cui nome contiene il testo (senza maiuscole), da una cartella in giu', entro i
// limiti di voci, risultati e tempo.
func cercaNas(ctx context.Context, r *os.Root, da, testo string, max int64) ([]nasVoce, bool) {
	ago := strings.ToLower(strings.TrimSpace(testo))
	scadenza := time.Now().Add(tempoRicercaNas)
	var out []nasVoce
	visitate, troncata := 0, false
	_ = fs.WalkDir(r.FS(), da, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil // una cartella che non si legge non ferma la ricerca
		}
		visitate++
		if visitate > maxVisitateNas || len(out) >= maxTrovatiNas || time.Now().After(scadenza) || ctx.Err() != nil {
			troncata = true
			return fs.SkipAll
		}
		if e.IsDir() {
			if p != da && strings.HasPrefix(e.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.Contains(strings.ToLower(e.Name()), ago) {
			out = append(out, voceNas(path.Dir(p), e, max))
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Percorso < out[j].Percorso })
	return out, troncata
}

// fascicoloNas: GET /thread/{id}/fascicolo/nas, con lo stato della pagina (nas, nas_cerca). Rifa' solo il
// cassetto: sfogliare il NAS non cambia niente del resto della schermata.
func (s *Server) fascicoloNas(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	if !almeno(u, db.RuoloUtenteOperatore) {
		s.nega(w, r, "Importare dal NAS è un gesto dell'operatore.")
		return
	}
	q := db.New(s.Pool)
	t, err := q.GetThread(r.Context(), id)
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	st := leggiStatoFascicolo(r.URL.Query())
	st.Cassetto = "nas"
	d := &fascicoloDati{T: t, Stato: st, Base: "/thread/" + id.String() + "/fascicolo", Scrive: true,
		Caricamento: s.Pipeline != nil && s.Staging != ""}
	d.Cliente, _ = q.GetCliente(r.Context(), t.ClienteID)
	d.NasConfigurato = s.NAS != nil && s.NAS.Radice != "" && d.Caricamento
	d.Nas = s.nasVista(r.Context(), q, d)
	s.eseguiFascicolo(w, "fasc_cassetto_frammento", d)
}

// importaDalNas: POST /thread/{id}/fascicolo/nas/importa, campo `percorso` (relativo alla radice del NAS).
func (s *Server) importaDalNas(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	no := func(motivo string) { s.threadFrammento(w, r, thread, "Niente è cambiato: "+motivo) }
	if s.Pipeline == nil || s.Staging == "" {
		no("l'importazione non è configurata su questo server (servono lo staging e la strada dell'analisi)")
		return
	}
	rel, err := percorsoNas(r.FormValue("percorso"))
	switch {
	case err != nil:
		no(spiegaErrore(err))
		return
	case rel == ".":
		no("nessun file scelto sul NAS")
		return
	}
	radice, err := s.radiceNas()
	if err != nil {
		no(spiegaErrore(err))
		return
	}
	defer radice.Close()
	f, err := radice.Open(rel)
	if err != nil {
		no(rel + ": il file non si apre (" + err.Error() + ")")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		no(rel + ": non è un file")
		return
	}
	nome := path.Base(rel)
	if !tecnicoCaricabile(nome) {
		no(nome + ": dal NAS si importano CAD 3D, disegni (PDF, DWG) e sviluppi DXF")
		return
	}
	max := s.maxCaricamentoEffettivo()
	if info.Size() > max {
		no(fmt.Sprintf("%s supera il limite di %d MB", nome, max>>20))
		return
	}
	car, err := s.nelloStagingCaricato(f, nome, "", max)
	if err != nil {
		var ri rifiuto
		if errors.As(err, &ri) {
			no(string(ri))
			return
		}
		s.Log.Error("importazione dal NAS: staging", "rfq", thread, "file", rel, "err", err)
		no("il file non si è potuto scrivere nello staging: " + err.Error())
		return
	}
	car.PathInterno = "NAS: " + rel

	u := utenteDa(r.Context())
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	msg, err := func() (string, error) {
		nota, err := fascicolo.NotaInterna(ctx, q, thread, *u)
		if err != nil {
			return "", err
		}
		a, err := fascicolo.RegistraCaricamento(ctx, q, nota, u.UtenteID, car)
		if err != nil {
			return "", err
		}
		if err := s.Pipeline.DopoCaricamento(ctx, q, a.AllegatoID); err != nil {
			return "", err
		}
		return nome + " importato dal NAS (" + rel + "): è fra i file della RFQ, l'analisi lo legge e il piano lo propone. " +
			"Sul NAS della RFQ va solo con la conferma.", nil
	}()
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		no(spiegaErrore(err))
		return
	}
	s.threadFrammento(w, r, thread, msg)
}
