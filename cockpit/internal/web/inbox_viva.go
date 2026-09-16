package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/jobs"
)

// Voce 2.16 — Inbox viva.
//
// Il difetto che questa voce chiude non è tecnico, è di fiducia: se l'Inbox mostra le stesse mail di
// un'ora fa, l'operatore non sa se non è arrivato niente o se il Cockpit è fermo, e nel dubbio
// riapre Outlook. Da lì in poi il Cockpit è un doppione che nessuno guarda.
//
// Tre cose, e sono quelle che rispondono alle tre domande che ci si fa davanti a una lista di mail:
//
//	«sta lavorando?»         lo stato per casella in testata: attiva, in corso, OFFLINE, non risolta,
//	                         con l'ora dell'ultimo sync riuscito;
//	«posso forzare?»         «Aggiorna ora», che accoda un sync per ogni casella attiva. Premuto
//	                         dieci volte accoda un job solo: la chiave di idempotenza è fissa per
//	                         casella (SV1);
//	«che c'è di nuovo?»      il conteggio dei messaggi registrati dopo l'ultima visita, in testata e
//	                         con un pallino sulle righe (SV3).
//
// Nessuna delle tre è un'autorizzazione: dicono che cosa è entrato e quando, non chi può leggerlo.
// La visibilità resta la regola restrittiva di D12.

// nuoveMax è il tetto degli id da segnare con il pallino. Oltre, la lista è comunque tutta nuova e
// contarli uno per uno non aggiunge niente.
const nuoveMax = 500

// novita è ciò che l'Inbox sa delle mail arrivate dopo l'ultima visita dell'utente.
type novita struct {
	Da         *time.Time // quando ha guardato l'ultima volta; nil = mai
	Totale     int
	PerCasella map[uuid.UUID]int
	Id         map[uuid.UUID]bool // i messaggi da segnare nella lista
}

// novitaPer legge le novità per un utente. Non tocca `ultima_vista_inbox`: leggere non è visitare.
func (s *Server) novitaPer(ctx context.Context, q *db.Queries, u *db.Utente) novita {
	n := novita{PerCasella: map[uuid.UUID]int{}, Id: map[uuid.UUID]bool{}}
	if u == nil {
		return n
	}
	n.Da = u.UltimaVistaInbox
	tot, _ := q.ContaNuoveDallaVisita(ctx, n.Da)
	n.Totale = int(tot)
	righe, _ := q.ContaNuovePerCasella(ctx, n.Da)
	for _, r := range righe {
		n.PerCasella[r.CasellaID] = int(r.N)
	}
	ids, _ := q.ListMessaggiNuoviDa(ctx, db.ListMessaggiNuoviDaParams{Da: n.Da, Limite: nuoveMax})
	for _, id := range ids {
		n.Id[id] = true
	}
	return n
}

// lavoroSync è lo stato del sync per casella: quando è finito l'ultimo riuscito e quali stanno
// girando adesso.
type lavoroSync struct {
	Ultimo  map[uuid.UUID]time.Time
	InCorso map[uuid.UUID]bool
}

func (s *Server) lavoroSync(ctx context.Context, q *db.Queries) lavoroSync {
	l := lavoroSync{Ultimo: map[uuid.UUID]time.Time{}, InCorso: map[uuid.UUID]bool{}}
	ultimi, _ := q.UltimoSyncPerCasella(ctx)
	for _, r := range ultimi {
		if r.CasellaID.Valid {
			l.Ultimo[r.CasellaID.UUID] = r.UltimoSync
		}
	}
	correnti, _ := q.CaselleConSyncInCorso(ctx)
	for _, c := range correnti {
		if c.Valid {
			l.InCorso[c.UUID] = true
		}
	}
	return l
}

// conSync aggiunge a ogni chip della testata lo stato del sync e le novità della casella.
//
// «in corso» si mette SOLO su una casella che risulta attiva: se il worker è spento e un sync è in
// coda, la verità è che è spento, e scrivere «in corso» significherebbe promettere che sta per
// arrivare qualcosa. È separata da statoCaselle perché è una decisione a sé, e si prova senza
// database (SV2).
func conSync(caselle []statoCasella, l lavoroSync, nuove map[uuid.UUID]int) []statoCasella {
	out := make([]statoCasella, 0, len(caselle))
	for _, c := range caselle {
		if t, ok := l.Ultimo[c.ID]; ok {
			q := t
			c.UltimoSync = &q
		}
		if l.InCorso[c.ID] && c.Stato == "attiva" {
			c.Stato = "in_corso"
			c.Dettaglio = "sincronizzazione in corso · " + c.Dettaglio
		}
		c.Nuove = nuove[c.ID]
		c.Classe = classiStato[c.Stato]
		out = append(out, c)
	}
	return out
}

// SyncInCorso dice se almeno una casella sta sincronizzando: la testata usa questo per chiedersi da
// sola ogni 3 secondi invece che ogni 15, e tornare al passo lento quando ha finito.
func (st *statoUI) SyncInCorso() bool {
	for _, c := range st.Caselle {
		if c.Stato == "in_corso" {
			return true
		}
	}
	return false
}

// AttesaTestata è l'intervallo con cui la testata si ricarica: stretto mentre un sync gira, largo
// quando non succede niente. Un poll ogni 3 secondi per sempre sarebbe 28 000 richieste al giorno
// per operatore per non dire niente.
func (st *statoUI) AttesaTestata() string {
	if st.SyncInCorso() {
		return "3s"
	}
	return "15s"
}

// aggiornaOra è POST /inbox/aggiorna: «Aggiorna ora» della testata (voce 2.16).
//
// Accoda un `sync_outlook` per ogni casella attiva con la stessa chiave dello scheduler. Dieci clic
// in dieci secondi restano un job per casella, e se lo scheduler ne ha già uno pendente il clic non
// ne aggiunge un secondo: la risposta lo dice, invece di far finta di aver fatto qualcosa (SV1).
func (s *Server) aggiornaOra(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	caselle, err := q.ListCaselleAttive(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	accodati, gia := 0, 0
	for _, c := range caselle {
		if c.Canale != db.CanaleOutlook {
			continue
		}
		j, err := jobs.AccodaSyncCasella(ctx, q, c, s.Sync)
		switch {
		case err != nil:
			s.Log.Error("aggiorna ora: sync non accodato", "casella", c.Indirizzo, "err", err)
		case j == nil:
			gia++
		default:
			accodati++
			s.Log.Info("sync accodato su richiesta dell'operatore", "casella", c.Indirizzo, "job", j.JobID)
		}
	}
	s.testata(w, r, frase(accodati, gia))
}

func frase(accodati, gia int) string {
	switch {
	case accodati == 0 && gia == 0:
		return "Nessuna casella Outlook attiva da sincronizzare."
	case accodati == 0:
		return fmt.Sprintf("Sincronizzazione già in corso su %d caselle: niente da accodare.", gia)
	case gia == 0:
		return fmt.Sprintf("Sincronizzazione richiesta per %d caselle.", accodati)
	}
	return fmt.Sprintf("Sincronizzazione richiesta per %d caselle (%d erano già in corso).", accodati, gia)
}

// testata rende il frammento della testata con un avviso facoltativo.
func (s *Server) testata(w http.ResponseWriter, r *http.Request, avviso string) {
	st := s.stato(r.Context(), sessioneDa(r.Context()))
	st.Avviso = avviso
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["inbox.html"].ExecuteTemplate(w, "stato_worker", vista{Utente: utenteDa(r.Context()), Stato: st, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "stato_worker", "err", err)
	}
}

// segnaVista sposta `ultima_vista_inbox` a adesso. La chiama SOLO il caricamento completo della
// pagina: ai poll HTMX no, altrimenti il conteggio delle novità sarebbe sempre zero e il pallino
// non comparirebbe mai su niente.
func (s *Server) segnaVista(ctx context.Context, q *db.Queries, u *db.Utente) {
	if u == nil {
		return
	}
	if err := q.ToccaVistaInbox(ctx, u.UtenteID); err != nil {
		s.Log.Warn("ultima_vista_inbox non aggiornata", "utente", u.Sigla, "err", err)
	}
}

// avvisoShadow è la frase della testata quando il server gira in sola lettura (voce 9.5).
func (s *Server) avvisoShadow() string {
	if s.Modalita != "shadow" {
		return ""
	}
	return "MODALITÀ SHADOW: il Cockpit legge Outlook e il NAS ma non li modifica. Bozze, «segna letto», " +
		"spostamenti e copie sul NAS non vengono eseguiti; «Apri in Outlook» sì. Si cambia con [server].modalita in cockpit.toml."
}
