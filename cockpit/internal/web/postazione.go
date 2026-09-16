package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/db"
)

// Voce 2.7 (N50): la sessione UI sa da quale PC arriva. È la premessa del routing dei job
// interattivi: «Apri in Outlook» apre una finestra, e deve aprirla sul PC di chi ha premuto il
// pulsante. Senza una postazione le azioni interattive non partono — non «partono da qualche parte».

// sessioneUI è ciò che il server sa della sessione corrente: chi è l'utente e su quale postazione
// sta lavorando (se lo sa).
type sessioneUI struct {
	Token      string
	Utente     *db.Utente
	Postazione uuid.NullUUID
	NomeHost   string // della postazione, se c'è
	Origine    string // ip | scelta | ""
}

const ctxSessione chiaveCtx = 2

func sessioneDa(ctx context.Context) sessioneUI {
	s, _ := ctx.Value(ctxSessione).(sessioneUI)
	return s
}

// finestraAbbinamentoIP: per quanto un IP registrato da un worker vale come indizio di «questo PC è
// quella postazione». Un giorno copre un lease DHCP normale; oltre, l'abbinamento non si fa e
// l'operatore sceglie dalla testata. È una comodità, non un'autorizzazione (P1).
const finestraAbbinamentoIP = 24 * time.Hour

// puoUsarePostazione è l'autorizzazione di un utente a una postazione (P1): la postazione è attiva
// ed è la sua (postazione.utente_id), oppure l'utente è admin. Un IP che coincide non basta, una
// scelta in testata nemmeno: entrambe passano da qui.
func puoUsarePostazione(u *db.Utente, p db.Postazione) bool {
	if u == nil || !p.Attiva {
		return false
	}
	if u.Ruolo == db.RuoloUtenteAdmin {
		return true
	}
	return p.UtenteID.Valid && p.UtenteID.UUID == u.UtenteID
}

// postazioniScegliibili sono quelle fra cui l'utente può scegliere in testata.
func (s *Server) postazioniScegliibili(ctx context.Context, q *db.Queries, u *db.Utente) []db.Postazione {
	tutte, err := q.ListPostazioniAttive(ctx)
	if err != nil {
		return nil
	}
	var out []db.Postazione
	for _, p := range tutte {
		if puoUsarePostazione(u, p) {
			out = append(out, p)
		}
	}
	return out
}

// indirizzoDi è l'IP del browser. Come nel worker API, X-Forwarded-For non si legge in produzione:
// il server non sta dietro a un proxy e un header lo scrive chiunque. I test sostituiscono
// IndirizzoClient per simulare due PC.
func (s *Server) indirizzoDi(r *http.Request) (netip.Addr, bool) {
	grezzo := r.RemoteAddr
	if s.IndirizzoClient != nil {
		grezzo = s.IndirizzoClient(r)
	}
	if h, _, err := net.SplitHostPort(grezzo); err == nil {
		grezzo = h
	}
	a, err := netip.ParseAddr(strings.TrimSpace(grezzo))
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// abbinaPostazione cerca la postazione da cui l'utente sta usando il browser: quella (una sola) da
// cui un worker ha fatto claim di recente con lo stesso IP, E a cui l'utente è abilitato. Due
// postazioni con lo stesso IP, o nessuna, o una non autorizzata: nessun abbinamento, si sceglie a
// mano. Restituisce anche il motivo, per il log.
func (s *Server) abbinaPostazione(ctx context.Context, q *db.Queries, u *db.Utente, ip netip.Addr) (uuid.NullUUID, string) {
	righe, err := q.PostazioniConWorkerAllIndirizzo(ctx, db.PostazioniConWorkerAllIndirizzoParams{IndirizzoIp: ip, EntroS: int32(finestraAbbinamentoIP / time.Second)})
	if err != nil || len(righe) == 0 {
		return uuid.NullUUID{}, "nessun worker ha fatto claim da questo IP"
	}
	var trovata *db.Postazione
	for i := range righe {
		if !puoUsarePostazione(u, righe[i]) {
			continue
		}
		if trovata != nil {
			return uuid.NullUUID{}, "più postazioni autorizzate con lo stesso IP: scelta manuale"
		}
		trovata = &righe[i]
	}
	if trovata == nil {
		return uuid.NullUUID{}, fmt.Sprintf("l'IP corrisponde a %s ma l'utente non è abilitato a quella postazione", righe[0].NomeHost)
	}
	return uuid.NullUUID{UUID: trovata.PostazioneID, Valid: true}, trovata.NomeHost
}

// scegliPostazione è POST /sessione/postazione: la scelta esplicita dell'operatore in testata
// (origine 'scelta'), o la rimozione. Solo fra le postazioni a cui è abilitato: un utente
// autorizzato a X che chiede Y riceve 403 (P1).
func (s *Server) scegliPostazione(w http.ResponseWriter, r *http.Request) {
	sess := sessioneDa(r.Context())
	q := db.New(s.Pool)
	grezzo := strings.TrimSpace(r.FormValue("postazione_id"))
	if grezzo == "" {
		if err := q.SetSessionePostazione(r.Context(), db.SetSessionePostazioneParams{Token: sess.Token}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.Log.Info("postazione della sessione rimossa", "utente", sess.Utente.Sigla)
		s.dopoScelta(w, r)
		return
	}
	id, err := uuid.Parse(grezzo)
	if err != nil {
		http.Error(w, "postazione non valida", 400)
		return
	}
	p, err := q.GetPostazione(r.Context(), id)
	if err != nil {
		http.Error(w, "postazione sconosciuta", 404)
		return
	}
	if !puoUsarePostazione(sess.Utente, p) {
		s.Log.Warn("scelta di una postazione non autorizzata", "utente", sess.Utente.Sigla, "postazione", p.NomeHost)
		http.Error(w, fmt.Sprintf("non sei abilitato alla postazione %s", p.NomeHost), 403)
		return
	}
	if err := q.SetSessionePostazione(r.Context(), db.SetSessionePostazioneParams{
		Token: sess.Token, PostazioneID: uuid.NullUUID{UUID: p.PostazioneID, Valid: true}, PostazioneOrigine: pgtype.Text{String: "scelta", Valid: true},
	}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// «registrata nel log» (piano §2.2): chi ha scelto che cosa, e da quale IP
	ip, _ := s.indirizzoDi(r)
	s.Log.Info("postazione della sessione scelta dall'operatore", "utente", sess.Utente.Sigla, "postazione", p.NomeHost, "origine", "scelta", "ip", ip.String())
	s.dopoScelta(w, r)
}

func (s *Server) dopoScelta(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/inbox", http.StatusFound)
}

// ---------------------------------------------------------------- testata: postazione, caselle, worker

// statoUI è ciò che la testata dice sempre: su quale PC è la sessione, e per ogni casella se c'è un
// worker che la serve davvero (M2: tre stati distinti, mai un «outlook attivo» che non dice di chi).
type statoUI struct {
	Postazione   string // nome host o "" (la testata mostra «—»)
	PostazioneID uuid.NullUUID
	Origine      string
	Scelte       []db.Postazione
	Caselle      []statoCasella
	Analisi      statoChip
	// SenzaPostazione è la frase mostrata quando la sessione non ha una postazione.
	SenzaPostazione string
	// Nuove è il totale dei messaggi arrivati dopo l'ultima visita all'Inbox (voce 2.16, SV3).
	Nuove int
	// Avviso è l'esito dell'ultima azione fatta dalla testata («Aggiorna ora»): vive un frammento
	// solo, finché il poll successivo non lo sostituisce.
	Avviso string
	// Shadow è la frase della modalità di sola lettura; vuota in produzione (voce 9.5).
	Shadow string
}

// statoCasella è lo stato di UNA casella in testata.
//
//	attiva          un worker autorizzato l'ha risolta nel proprio profilo e fa claim (≤ 60 s)
//	in_corso        attiva, e in questo momento un sync della casella è in coda o in esecuzione (2.16)
//	offline         c'è un worker configurato per lei ma non fa claim (o non l'ha mai fatto)
//	non_risolta     il worker è attivo ma non ha trovato la casella nel proprio profilo (o COM non risponde)
//	non_configurata nessun [[worker]] la elenca fra le proprie caselle
type statoCasella struct {
	ID        uuid.UUID
	Nome      string
	Stato     string
	Classe    string // classe CSS della chip
	Dettaglio string // il testo del tooltip: chi, dove, da quanto
	// UltimoSync è la fine dell'ultimo sync RIUSCITO di questa casella; nil = mai (voce 2.16).
	UltimoSync *time.Time
	// Nuove sono i messaggi entrati in questa casella dopo l'ultima visita all'Inbox.
	Nuove int
}

type statoChip struct {
	Etichetta string
	Classe    string
	Dettaglio string
}

// classiStato: attiva = verde, in corso = azzurro, offline = rosso, non_risolta = giallo,
// non_configurata = grigio.
var classiStato = map[string]string{"attiva": "fatto", "in_corso": "lavoro", "offline": "fallito", "non_risolta": "in_corso", "non_configurata": ""}

const workerOnlineEntro = 60 * time.Second

// stato calcola la testata per la sessione corrente.
func (s *Server) stato(ctx context.Context, sess sessioneUI) *statoUI {
	q := db.New(s.Pool)
	st := &statoUI{Postazione: sess.NomeHost, PostazioneID: sess.Postazione, Origine: sess.Origine}
	if sess.Utente != nil {
		st.Scelte = s.postazioniScegliibili(ctx, q, sess.Utente)
	}
	if !sess.Postazione.Valid {
		st.SenzaPostazione = "Nessuna postazione associata a questa sessione: le azioni su Outlook (Apri, Segna letto, Bozza) sono disabilitate finché non scegli il PC su cui stai lavorando."
		if len(st.Scelte) == 0 {
			st.SenzaPostazione = "Nessuna postazione associata a questa sessione e nessuna postazione a cui sei abilitato: le azioni su Outlook sono disabilitate."
		}
	}
	presenze, _ := q.ListWorkerPresenza(ctx)
	credenziali, _ := q.ListWorkerCredenziali(ctx)
	caselle, _ := q.ListCaselleAttive(ctx)
	postazioni, _ := q.ListPostazioni(ctx)
	st.Caselle = statoCaselle(caselle, credenziali, presenze, postazioni, time.Now())
	// voce 2.16: sopra lo stato del worker si sovrappone lo stato del LAVORO (sync in corso, ultimo
	// sync riuscito) e ciò che è arrivato da quando l'operatore ha guardato l'ultima volta.
	nov := s.novitaPer(ctx, q, sess.Utente)
	st.Caselle = conSync(st.Caselle, s.lavoroSync(ctx, q), nov.PerCasella)
	st.Nuove = nov.Totale
	st.Shadow = s.avvisoShadow()
	st.Analisi = statoAnalisi(credenziali, presenze, time.Now())
	return st
}

// statoCaselle è la parte pura di stato: da caselle, credenziali e presenze ai tre stati. Separata
// così il test M2 la prova senza HTTP.
func statoCaselle(caselle []db.Casella, credenziali []db.WorkerCredenziale, presenze []db.ListWorkerPresenzaRow, postazioni []db.Postazione, ora time.Time) []statoCasella {
	host := map[uuid.UUID]string{}
	for _, p := range postazioni {
		host[p.PostazioneID] = p.NomeHost
	}
	presenza := map[string]db.ListWorkerPresenzaRow{}
	for _, p := range presenze {
		presenza[p.WorkerNome] = p
	}
	var out []statoCasella
	for _, c := range caselle {
		if c.Canale != db.CanaleOutlook {
			continue
		}
		sc := statoCasella{ID: c.CasellaID, Nome: c.Nome, Stato: "non_configurata", Dettaglio: "nessun [[worker]] in cockpit.toml elenca questa casella: nessuno la sincronizza"}
		var ripiego *statoCasella // il primo stato non «attiva» trovato, se nessun worker è attivo
		for _, w := range credenziali {
			if !w.Attivo || w.WorkerTipo != db.WorkerTipoOutlook || !contiene(w.Caselle, c.CasellaID) {
				continue
			}
			dove := w.WorkerNome
			if w.PostazioneID.Valid {
				if h, ok := host[w.PostazioneID.UUID]; ok {
					dove = h
				}
			}
			p, vista := presenza[w.WorkerNome]
			var cand statoCasella
			switch {
			case !vista:
				cand = statoCasella{ID: c.CasellaID, Nome: c.Nome, Stato: "offline", Dettaglio: fmt.Sprintf("il worker %s non è mai stato avviato", w.WorkerNome)}
			case ora.Sub(p.UltimoClaim) > workerOnlineEntro:
				cand = statoCasella{ID: c.CasellaID, Nome: c.Nome, Stato: "offline", Dettaglio: fmt.Sprintf("worker su %s OFFLINE: ultimo contatto %s fa", dove, durataBreve(ora.Sub(p.UltimoClaim)))}
			case contiene(p.CaselleAperte, c.CasellaID):
				cand = statoCasella{ID: c.CasellaID, Nome: c.Nome, Stato: "attiva", Dettaglio: fmt.Sprintf("attiva su %s (ultimo contatto %d s fa)", dove, int(ora.Sub(p.UltimoClaim).Seconds()))}
			case !p.OutlookOk:
				cand = statoCasella{ID: c.CasellaID, Nome: c.Nome, Stato: "non_risolta", Dettaglio: fmt.Sprintf("il worker su %s è attivo ma Outlook non risponde", dove)}
			default:
				cand = statoCasella{ID: c.CasellaID, Nome: c.Nome, Stato: "non_risolta", Dettaglio: fmt.Sprintf("il worker su %s è attivo ma non trova questa casella nel proprio profilo Outlook", dove)}
			}
			if p.Avviso.Valid && p.Avviso.String != "" {
				cand.Dettaglio += " · " + p.Avviso.String
			}
			if cand.Stato == "attiva" {
				sc = cand
				ripiego = nil
				break
			}
			if ripiego == nil {
				x := cand
				ripiego = &x
			}
		}
		if ripiego != nil {
			sc = *ripiego
		}
		sc.Classe = classiStato[sc.Stato]
		out = append(out, sc)
	}
	return out
}

func statoAnalisi(credenziali []db.WorkerCredenziale, presenze []db.ListWorkerPresenzaRow, ora time.Time) statoChip {
	configurato := false
	for _, w := range credenziali {
		if w.Attivo && w.WorkerTipo == db.WorkerTipoAnalisi {
			configurato = true
		}
	}
	for _, p := range presenze {
		if p.WorkerTipo != db.WorkerTipoAnalisi {
			continue
		}
		if ora.Sub(p.UltimoClaim) <= workerOnlineEntro {
			return statoChip{Etichetta: "analisi attiva", Classe: "fatto", Dettaglio: fmt.Sprintf("%s, ultimo contatto %d s fa", p.WorkerNome, int(ora.Sub(p.UltimoClaim).Seconds()))}
		}
		return statoChip{Etichetta: "analisi OFFLINE", Classe: "fallito", Dettaglio: fmt.Sprintf("%s, ultimo contatto %s fa", p.WorkerNome, durataBreve(ora.Sub(p.UltimoClaim)))}
	}
	if !configurato {
		return statoChip{Etichetta: "analisi non configurata", Classe: "", Dettaglio: "nessun [[worker]] di tipo analisi in cockpit.toml"}
	}
	return statoChip{Etichetta: "analisi mai avviata", Classe: "fallito", Dettaglio: "il worker di analisi è censito ma non ha mai fatto claim"}
}

func contiene(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func durataBreve(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
}
