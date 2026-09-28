package web

// «Carica nuova versione interna», B8.7: un CAD o un disegno rifatto in casa entra nella RFQ come entrano
// i file dei clienti. Nessuno schema nuovo e nessuna scorciatoia:
//
//   - il file va fra i contenuti dello staging, con il suo sha256 per nome, come quelli dei worker;
//   - diventa un allegato di origine `manuale` del contenitore della RFQ, la nota interna (un messaggio del
//     canale `nota`, uno per RFQ, gia' agganciato);
//   - da li' fa la strada di tutti: proposta dal nome, analisi (il worker), proposta raffinata;
//   - diventa un documento solo con una decisione, la conferma, che chiede come per gli altri file
//     «aggiungi» o «sostituisce quale». Per una versione interna sostituire vuole anche un motivo, e se il
//     predecessore e' lo STEP strutturale si dice se il nuovo diventa il riferimento (fascicolo.go).
//
// Il caricamento non inventa una revisione del cliente: quella letta nel nome o nel cartiglio resta nei
// dettagli della proposta (fascicolo.RevisioneProponibile). Con la BOM congelata il caricamento e
// l'analisi sono ammessi, e nascono proposte; la working non cambia finche' non si apre una revisione.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// maxCaricamentoPredefinito vale quando [server].max_upload_mb non arriva fin qui.
const maxCaricamentoPredefinito = 64 << 20

// tecnicoCaricabile: si caricano qui i file che possono essere un documento tecnico (A2.2): CAD 3D,
// disegni e sviluppi. Un PDF non dice che cosa e' finche' l'analisi non lo legge, ma puo' essere un
// disegno, quindi entra.
func tecnicoCaricabile(nome string) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	switch tipo, _, _ := classificazione.TipoDaEstensione(ext); tipo {
	case "cad_3d", "disegno_2d", "sviluppo_dxf", "da_determinare":
		return true
	}
	return false
}

// caricaVersioneInterna: POST /thread/{id}/fascicolo/carica, multipart con il campo `file`.
func (s *Server) caricaVersioneInterna(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	no := func(motivo string) { s.threadFrammento(w, r, thread, "Niente è cambiato: "+motivo) }
	if s.Pipeline == nil || s.Staging == "" {
		no("il caricamento interno non è configurato su questo server (serve lo staging e la strada dell'analisi)")
		return
	}
	max := s.maxCaricamentoEffettivo()
	r.Body = http.MaxBytesReader(w, r.Body, max+(1<<20))
	f, h, err := r.FormFile("file")
	if err != nil {
		var troppo *http.MaxBytesError
		if errors.As(err, &troppo) {
			no(fmt.Sprintf("il file supera il limite di %d MB", max>>20))
			return
		}
		no("nessun file scelto")
		return
	}
	defer f.Close()
	nome := strings.TrimSpace(filepath.Base(strings.ReplaceAll(h.Filename, `\`, "/")))
	if nome == "" || nome == "." || nome == "/" {
		no("il file non ha un nome")
		return
	}
	if !tecnicoCaricabile(nome) {
		no(nome + ": qui si caricano versioni interne di CAD 3D, disegni (PDF, DWG) e sviluppi DXF")
		return
	}
	car, err := s.nelloStagingCaricato(f, nome, h.Header.Get("Content-Type"), max)
	if err != nil {
		var r rifiuto
		if errors.As(err, &r) {
			no(string(r))
			return
		}
		s.Log.Error("caricamento interno: staging", "rfq", thread, "file", nome, "err", err)
		no("il file non si è potuto scrivere nello staging: " + err.Error())
		return
	}

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
		return nome + " caricato come versione interna: entra fra i file della RFQ, con la sua proposta, e l'analisi lo legge. " +
			"Diventa un documento con «Conferma», dove si sceglie se si aggiunge o che cosa sostituisce (con il motivo).", nil
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

// nelloStagingCaricato scrive il file fra i contenuti dello staging: prima in un .parte con un token suo,
// calcolando lo sha256 mentre scrive, poi con il nome definitivo, che e' l'hash. Se quel contenuto c'e'
// gia' (verificato), il .parte si butta.
func (s *Server) nelloStagingCaricato(f io.Reader, nome, tipo string, max int64) (fascicolo.Caricato, error) {
	token := uuid.New()
	parte := staging.PercorsoParte(s.Staging, token, token)
	if err := os.MkdirAll(filepath.Dir(parte), 0o755); err != nil {
		return fascicolo.Caricato{}, err
	}
	out, err := os.OpenFile(parte, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fascicolo.Caricato{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(f, max+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		staging.RimuoviParte(parte)
		return fascicolo.Caricato{}, err
	}
	switch {
	case n > max:
		staging.RimuoviParte(parte)
		return fascicolo.Caricato{}, rifiuto(fmt.Sprintf("il file supera il limite di %d MB", max>>20))
	case n == 0:
		staging.RimuoviParte(parte)
		return fascicolo.Caricato{}, rifiuto(nome + " è vuoto")
	}
	sha := hex.EncodeToString(h.Sum(nil))
	def, err := staging.PercorsoContenuto(s.Staging, sha, nome)
	if err != nil {
		staging.RimuoviParte(parte)
		return fascicolo.Caricato{}, err
	}
	if staging.ContenutoGiaPresente(def, sha) {
		staging.RimuoviParte(parte)
	} else {
		if err := os.MkdirAll(filepath.Dir(def), 0o755); err != nil {
			staging.RimuoviParte(parte)
			return fascicolo.Caricato{}, err
		}
		if err := staging.Promuovi(parte, def); err != nil {
			staging.RimuoviParte(parte)
			return fascicolo.Caricato{}, err
		}
	}
	return fascicolo.Caricato{Nome: nome, Sha256: sha, Bytes: n, Percorso: def, Tipo: tipo}, nil
}
