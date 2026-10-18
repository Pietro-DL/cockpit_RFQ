package web

// «Aggiungi file alla RFQ» (14/10, collaudo dell'utente): file, zip e 7z caricati dalla pagina della RFQ, per
// esempio l'archivio che il portale del cliente dà dopo la mail («Manufacturing_documents_….zip»). È la
// strada del caricamento interno (caricamento.go), allargata in tre punti e senza schema nuovo:
//
//   - più file in una richiesta, letti a flusso dal multipart e scritti subito fra i contenuti dello staging
//     (un file per volta in memoria, mai tutta la richiesta), ognuno con il suo limite;
//   - l'origine la sceglie chi carica: `portale` (file del cliente: la revisione letta si propone come per un
//     allegato della mail) oppure `manuale` (versione interna, come nel Fascicolo). I file del cliente vanno in
//     un contenitore loro (fascicolo.NotaPortale), separato dai caricamenti interni;
//   - qualunque tipo per i file del cliente, archivi compresi: uno zip o un 7z si scompatta nel suo job
//     (dopoStaging, archivio.Estraibile) e le voci ereditano l'origine. Per le versioni interne valgono i file
//     tecnici di prima, più gli archivi.
//
// Ogni file ha la sua transazione: uno rifiutato non fa perdere gli altri, e l'esito lo dice file per file.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/archivio"
)

// maxFileCaricamentoRFQ: quanti file al massimo in una richiesta. Si contano tutte le parti «file», anche quelle
// rifiutate; oltre, la lettura si ferma.
const maxFileCaricamentoRFQ = 50

// maxRifiutiDetti: quanti rifiuti si scrivono uno per uno nell'esito; gli altri si contano.
const maxRifiutiDetti = 12

// maxRichiestaCaricamento: il limite di tutta la richiesta, otto volte quello di un file. La pagina lo dice e lo
// controlla prima dell'invio (carica_rfq.mjs): oltre, il server smette di leggere e il browser può perdere la
// risposta.
func (s *Server) maxRichiestaCaricamento() int64 { return s.maxCaricamentoEffettivo() * 8 }

// fileCaricatoRFQ: un file arrivato e messo nello staging, in attesa di registrazione.
type fileCaricatoRFQ struct {
	car  fascicolo.Caricato
	nome string
}

// caricaNellaRFQ: POST /thread/{id}/carica, multipart con i campi `origine` (portale | interna) e `file`
// (uno o più).
func (s *Server) caricaNellaRFQ(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	no := func(motivo string) { s.threadFrammento(w, r, thread, "Niente è cambiato: "+motivo) }
	if s.Pipeline == nil || s.Staging == "" {
		no("il caricamento non è configurato su questo server (serve lo staging e la strada dell'analisi)")
		return
	}
	max := s.maxCaricamentoEffettivo()
	totale := s.maxRichiestaCaricamento()
	r.Body = http.MaxBytesReader(w, r.Body, totale+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		no("la richiesta non è un caricamento di file")
		return
	}

	origine := "portale"
	var arrivati []fileCaricatoRFQ
	var rifiutati []string
	parti := 0
	superato := func() string {
		return fmt.Sprintf("la richiesta supera il limite totale di %d MB: i file da qui in poi non sono arrivati, caricali in un altro invio", totale>>20)
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var troppo *http.MaxBytesError
			if errors.As(err, &troppo) {
				rifiutati = append(rifiutati, superato())
			} else {
				rifiutati = append(rifiutati, "la richiesta si è interrotta: "+err.Error())
			}
			break
		}
		switch part.FormName() {
		case "origine":
			v, _ := io.ReadAll(io.LimitReader(part, 32))
			origine = strings.TrimSpace(string(v))
		case "file":
			nome := strings.TrimSpace(filepath.Base(strings.ReplaceAll(part.FileName(), `\`, "/")))
			if nome == "" || nome == "." || nome == "/" {
				break // un input senza file scelto manda una parte vuota senza nome
			}
			parti++
			if parti > maxFileCaricamentoRFQ {
				rifiutati = append(rifiutati, fmt.Sprintf("al massimo %d file per volta: da %s in poi non sono stati letti", maxFileCaricamentoRFQ, nome))
				_ = part.Close()
				goto finito
			}
			car, err := s.nelloStagingCaricato(part, nome, part.Header.Get("Content-Type"), max)
			if err != nil {
				var rf rifiuto
				var troppo *http.MaxBytesError
				if errors.As(err, &troppo) {
					rifiutati = append(rifiutati, nome+": "+superato())
					_ = part.Close()
					goto finito
				}
				if errors.As(err, &rf) {
					rifiutati = append(rifiutati, nome+": "+string(rf))
				} else {
					s.Log.Error("caricamento nella RFQ: staging", "rfq", thread, "file", nome, "err", err)
					rifiutati = append(rifiutati, nome+": non si è potuto scrivere nello staging ("+err.Error()+")")
				}
				break
			}
			arrivati = append(arrivati, fileCaricatoRFQ{car: car, nome: nome})
		}
		_ = part.Close()
	}
finito:

	interna := origine == "interna"
	if !interna && origine != "portale" {
		no("origine sconosciuta: " + origine)
		return
	}
	if len(arrivati) == 0 {
		if len(rifiutati) > 0 {
			no(elencoRifiuti(rifiutati))
			return
		}
		no("nessun file scelto")
		return
	}

	u := utenteDa(r.Context())
	ctx := r.Context()
	var caricati []string
	for _, f := range arrivati {
		if interna && !tecnicoCaricabile(f.nome) && !archivio.Estraibile(filepath.Ext(f.nome)) {
			rifiutati = append(rifiutati, f.nome+": come versione interna si caricano CAD 3D, disegni, sviluppi DXF e archivi")
			continue
		}
		car := f.car
		car.Origine = db.OrigineAllegatoPortale
		if interna {
			car.Origine = db.OrigineAllegatoManuale
		}
		err := func() error {
			tx, err := s.Pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)
			q := db.New(tx)
			var nota db.Messaggio
			if interna {
				nota, err = fascicolo.NotaInterna(ctx, q, thread, *u)
			} else {
				nota, err = fascicolo.NotaPortale(ctx, q, thread, *u)
			}
			if err != nil {
				return err
			}
			// lo stesso contenuto già caricato in questo contenitore (un nuovo invio dopo una risposta persa) non
			// diventa un secondo allegato con una seconda analisi. V1 (EM §10.9, E1′; G-14; giro 1, TR-10): il
			// controllo sta dopo il lucchetto del contenitore, e un file scartato nella Sorgente (anche col gesto
			// precedente) non conta: il ricaricamento e' un'occorrenza nuova, e quella scartata riceve l'evento
			// ricaricato_dopo_scarto (origine legacy), scritto dal servizio del ciclo dei file
			if _, err := q.BloccaMessaggio(ctx, nota.MessaggioID); err != nil {
				return err
			}
			doppione, err := filerfq.CercaDoppione(ctx, q, nota.MessaggioID, car.Sha256)
			if err != nil {
				return err
			}
			if doppione.Presente != nil {
				return errGiaCaricato
			}
			a, err := fascicolo.RegistraCaricamento(ctx, q, nota, u.UtenteID, car)
			if err != nil {
				return err
			}
			if err := s.Pipeline.DopoCaricamento(ctx, q, a.AllegatoID); err != nil {
				return err
			}
			if err := filerfq.SegnaRicaricato(ctx, q, thread, doppione.Scartate, a.AllegatoID, u.UtenteID, filerfq.OrigineLegacy); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}()
		if errors.Is(err, errGiaCaricato) {
			rifiutati = append(rifiutati, f.nome+": già caricato in questa RFQ con lo stesso contenuto")
			continue
		}
		if err != nil {
			rifiutati = append(rifiutati, f.nome+": "+spiegaErrore(err))
			continue
		}
		caricati = append(caricati, f.nome)
	}

	var b strings.Builder
	if len(caricati) > 0 {
		come := "come file del cliente"
		if interna {
			come = "come versioni interne"
		}
		fmt.Fprintf(&b, "%s %s: %s. Fanno la strada degli allegati della mail: gli archivi si scompattano, poi l'analisi legge ogni file; "+
			"la Distinta nuova li vede con «Aggiorna» quando l'analisi è finita.",
			fraseConta(len(caricati), "file caricato", "file caricati"), come, strings.Join(caricati, ", "))
	}
	if len(rifiutati) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "Non caricati: %s.", elencoRifiuti(rifiutati))
	}
	if len(caricati) == 0 {
		no(b.String())
		return
	}
	s.threadFrammento(w, r, thread, b.String())
}

// errGiaCaricato: il file c'è già, con lo stesso sha256, nel contenitore dove andrebbe.
var errGiaCaricato = errors.New("già caricato")

// elencoRifiuti: i primi maxRifiutiDetti rifiuti per esteso, gli altri contati.
func elencoRifiuti(r []string) string {
	if len(r) <= maxRifiutiDetti {
		return strings.Join(r, " · ")
	}
	return strings.Join(r[:maxRifiutiDetti], " · ") + fmt.Sprintf(" · e altri %d", len(r)-maxRifiutiDetti)
}
