// L'Inbox: la schermata A e tutto quello che si fa da lì senza uscire dalla posta.
//
// La lista con i suoi quadranti, il sync (quello ordinario e quello storico, con il suo badge), il
// pannello di un messaggio, e le azioni interattive — apri in Outlook, segna letto, bozza — che
// passano dalla coda con una copia legata alla postazione di chi ha premuto.
//
// Le azioni che decidono che cosa È un messaggio stanno nei loro file (triage.go, censisci.go,
// richieste.go, allegati.go): qui c'e' la schermata e il montaggio delle loro rotte.

package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// registraInbox monta le rotte dell'Inbox: la lista, il sync, il pannello di un messaggio e le
// azioni che si fanno da li' (triage, censimento, download, bozza).
func (s *Server) registraInbox(mux *http.ServeMux) {
	mux.HandleFunc("GET /inbox", s.autenticato(s.inbox))
	mux.HandleFunc("POST /inbox/aggiorna", s.autenticato(s.aggiornaOra))
	mux.HandleFunc("POST /inbox/sync-storico", s.autenticato(s.syncStorico))
	mux.HandleFunc("GET /inbox/sync-storico/stato", s.autenticato(s.syncStoricoStato))
	mux.HandleFunc("GET /stato/worker", s.autenticato(s.statoWorker))
	mux.HandleFunc("GET /messaggio/{id}", s.autenticato(s.messaggio))
	mux.HandleFunc("POST /messaggio/{id}/apri", s.autenticato(s.apriInOutlook))
	mux.HandleFunc("POST /messaggio/{id}/bozza", s.autenticato(s.bozza))
	mux.HandleFunc("POST /messaggio/{id}/letto", s.autenticato(s.segnaLetto))
	mux.HandleFunc("POST /messaggio/{id}/analizza", s.autenticato(s.chiediAnalisi))
	// blocco 4: triage
	mux.HandleFunc("GET /messaggio/{id}/triage", s.autenticato(s.triageForm))
	mux.HandleFunc("POST /messaggio/{id}/rfq", s.autenticato(s.nuovaRFQ))
	mux.HandleFunc("POST /messaggio/{id}/aggancia", s.autenticato(s.agganciaEsistente))
	mux.HandleFunc("POST /messaggio/{id}/ignora", s.autenticato(s.ignora))
	// blocco 7A.3: «Censisci come fornitore / cliente» dal pannello, con il ritriage mirato
	mux.HandleFunc("GET /messaggio/{id}/censisci", s.autenticato(s.censisciForm))
	mux.HandleFunc("POST /messaggio/{id}/censisci", s.autenticato(s.censisci))
	// blocco 7B: la posta dei fornitori si aggancia alla richiesta; la richiesta mandata a mano si conferma
	mux.HandleFunc("POST /messaggio/{id}/risposta-fornitore", s.autenticato(s.rispostaFornitore))
	mux.HandleFunc("POST /messaggio/{id}/richiesta-fornitore", s.autenticato(s.richiestaFornitoreManuale))
	mux.HandleFunc("GET /anagrafica/buyer", s.autenticato(s.buyerSelect))
	mux.HandleFunc("GET /thread/cerca", s.autenticato(s.cercaThread))
	// download su richiesta e smistamento (blocco 5)
	mux.HandleFunc("POST /messaggio/{id}/scarica", s.autenticato(s.scarica))
	mux.HandleFunc("POST /allegato/{id}/riscarica", s.autenticato(s.riscarica))
	// blocco 8 (B8.1): il PDF si guarda da qui, senza scaricarlo e senza cercarlo sul NAS a mano.
	// GET perche' non cambia niente: e' il file, servito a pezzi a chi lo sta gia' guardando.
	mux.HandleFunc("GET /allegato/{id}/anteprima", s.autenticato(s.anteprima))
}

type inboxDati struct {
	// Sync dice all'operatore se e ogni quanto il server accoda il sync: prima la schermata diceva
	// «ogni minuto» qualunque fosse la configurazione, e con intervallo_sync_s = 0 era falso.
	Sync   string
	Filtro string
	// Quadrante e Direzione sono l'Inbox del blocco 7 (D34, 7C.0): Clienti, Fornitori, Interni,
	// Altro, Da validare, con la direzione come filtro. Un messaggio sta in UN quadrante, deciso
	// dalla sola controparte, e il calcolo sta in v_inbox (colonna `quadrante`): qui non c'è un
	// secondo calcolo, e non c'è nemmeno più in messaggi.sql.
	Quadrante string // clienti | fornitori | interni | altro | validare | "" (tutti)
	Direzione string // entrata | uscita | "" (tutte)
	Quadranti db.ContaQuadrantiRow
	Righe     []db.VInbox
	Conta     db.ContaInboxRow
	Selezion  string
	// Caselle sono quelle attive; Casella è quella scelta nel selettore (vuoto = tutte). È un FILTRO
	// della schermata, non un'autorizzazione: che cosa un utente possa vedere è la voce 2.2, e fino
	// ad allora resta la regola restrittiva di D12.
	Caselle []db.Casella
	Casella string
	// Nuovi sono i messaggi arrivati dopo l'ultima visita: la lista li segna con un pallino, e il
	// numero sta in testata (voce 2.16, SV3). Vale per la pagina appena caricata: chi resta fermo
	// sulla schermata vede crescere il contatore e comparire i pallini ai poll successivi.
	Nuovi  map[uuid.UUID]bool
	NNuove int
}

// ENuovo dice se una riga della lista è arrivata dopo l'ultima visita dell'operatore.
func (d inboxDati) ENuovo(id uuid.UUID) bool { return d.Nuovi[id] }

// QuadranteURL è il quadrante come va scritto nei link: «tutti» e non vuoto, perché un `q` vuoto
// tornerebbe al predefinito (Clienti) e la schermata cambierebbe quadrante da sola.
func (d inboxDati) QuadranteURL() string {
	if d.Quadrante == "" {
		return "tutti"
	}
	return d.Quadrante
}

// quadranti sono i cinque riquadri dell'Inbox (7C.0), nell'ordine in cui si mostrano. Le chiavi
// sono i valori della colonna `quadrante` di v_inbox.
var quadranti = []struct{ Chiave, Nome, Aiuto string }{
	{"clienti", "Clienti", "La posta dei clienti, in entrata e in uscita: le richieste d'offerta e tutto ciò che ci gira intorno."},
	{"fornitori", "Fornitori", "La posta dei fornitori censiti: offerte, domande, solleciti, e le nostre richieste a loro."},
	{"interni", "Interni", "Posta fra i nostri indirizzi: un collega che gira una richiesta, una nota interna."},
	{"altro", "Altro", "Soggetti censiti come diversi da cliente, fornitore e interno: corrieri, banche, notifiche di un servizio."},
	{"validare", "Da validare", "Mittenti non censiti e indirizzi in più anagrafiche: qui si decide chi sono, non che cosa vogliono."},
}

func (d inboxDati) QuadrantiDisponibili() []struct{ Chiave, Nome, Aiuto string } { return quadranti }

func (d inboxDati) ContaQuadrante(chiave string) int64 {
	switch chiave {
	case "clienti":
		return d.Quadranti.Clienti
	case "fornitori":
		return d.Quadranti.Fornitori
	case "interni":
		return d.Quadranti.Interni
	case "altro":
		return d.Quadranti.Altro
	case "validare":
		return d.Quadranti.Validare
	}
	return 0
}

// quadranteValido normalizza il parametro: un valore sconosciuto è Clienti, che è l'Inbox di ieri.
// "tutti" resta possibile (vuoto) per chi cerca un messaggio senza sapere di chi è. «buyer» era
// il nome di Clienti fino al 7B: i link salvati continuano a funzionare.
func quadranteValido(q string) string {
	switch q {
	case "clienti", "fornitori", "interni", "altro", "validare":
		return q
	case "buyer":
		return "clienti"
	case "tutti":
		return ""
	}
	return "clienti"
}

func direzioneValida(d string) string {
	if d == "entrata" || d == "uscita" {
		return d
	}
	return ""
}

// descrizioneSync è la frase della schermata sullo stato della sincronizzazione automatica.
func (s *Server) descrizioneSync() string {
	apertura := ""
	if s.SyncAperturaInbox {
		apertura = " Un aggiornamento parte da solo alla prima apertura di questa schermata."
	}
	if s.IntervalloSync <= 0 {
		return "Sincronizzazione periodica disattivata (intervallo_sync_s = 0)." + apertura +
			" «Aggiorna ora» e «Carica precedenti» restano disponibili."
	}
	return fmt.Sprintf("Il worker Outlook sincronizza ogni %d s.", int(s.IntervalloSync/time.Second)) + apertura
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	filtro := r.URL.Query().Get("filtro")
	if filtro != "agganciati" && filtro != "tutti" && filtro != "ignorati" {
		filtro = "orfani"
	}
	caselle, _ := q.ListCaselleAttive(r.Context())
	var scelta uuid.NullUUID
	grezzo := r.URL.Query().Get("casella")
	if id, err := uuid.Parse(grezzo); err == nil {
		for _, c := range caselle {
			if c.CasellaID == id {
				scelta = uuid.NullUUID{UUID: id, Valid: true}
			}
		}
	}
	if !scelta.Valid {
		grezzo = ""
	}
	quadrante := quadranteValido(r.URL.Query().Get("q"))
	direzione := direzioneValida(r.URL.Query().Get("dir"))
	righe, err := q.ListInbox(r.Context(), db.ListInboxParams{Filtro: filtro, Casella: scelta, Quadrante: quadrante, Direzione: direzione, Limite: 200, Salta: 0})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaInbox(r.Context(), db.ContaInboxParams{Casella: scelta, Quadrante: quadrante, Direzione: direzione})
	perQuadrante, _ := q.ContaQuadranti(r.Context(), scelta)
	u := utenteDa(r.Context())
	nov := s.novitaPer(r.Context(), q, u)
	d := inboxDati{Filtro: filtro, Quadrante: quadrante, Direzione: direzione, Quadranti: perQuadrante,
		Righe: righe, Conta: conta, Selezion: r.URL.Query().Get("sel"),
		Caselle: caselle, Casella: grezzo, Sync: s.descrizioneSync(), Nuovi: nov.Id, NNuove: nov.Totale}
	// Il frammento e' «inbox_stato», non «inbox_lista»: i comandi e la lista si rifanno INSIEME.
	// Rispondere con la sola lista lasciava sullo schermo le linguette del quadrante precedente
	// (checkpoint 7B.5), cioe' una schermata che diceva una cosa e ne mostrava un'altra.
	s.rendi(w, r, "inbox.html", "inbox_stato", "Inbox", d)
	// Il seguito va fatto DOPO aver reso la pagina, e solo se è una pagina: il poll HTMX ogni 15 s
	// chiede lo stesso indirizzo, e se contasse come una visita il contatore delle novità direbbe
	// sempre zero (SV3); se contasse come un'apertura accoderebbe un sync ogni quindici secondi.
	if !frammentoRichiesto(r) {
		s.segnaVista(r.Context(), q, u)
		s.syncAllApertura(r.Context(), q, sessioneDa(r.Context()))
	}
}

// GiorniStorico è quanto archivio scarica UN clic su «Carica precedenti», per casella: due giorni.
//
// Non è una misura di prudenza generica, è la forma del worker. Il worker Outlook è seriale e ce n'è
// uno per PC: finché macina un job storico non prende «Apri in Outlook», non scarica un allegato e
// non fa il sync ordinario. Un job da trenta giorni su una casella viva è il Cockpit che smette di
// rispondere per un tempo che nessuno sa dire in anticipo — e l'operatore non vede un lavoro in
// corso, vede dei pulsanti che non fanno niente.
//
// Due giorni per clic sono una scelta diversa: il worker torna al claim spesso, e chi vuole risalire
// preme ancora. La catena non si costruisce da sola di proposito (nessun job storico ne accoda un
// altro): una catena lunga occuperebbe il worker per settimane senza che nessuno l'abbia chiesto,
// che è esattamente il difetto di prima con un vestito nuovo.
const GiorniStorico = 2

// syncStorico accoda la finestra di GiorniStorico PRIMA di quanto già coperto: il limite superiore è
// il cursore storico_fino_a (se un "Carica precedenti" è già riuscito) oppure la mail più vecchia in
// archivio.
//
// Dalla 0004 è PER CASELLA: ogni casella ha i suoi cursori e la sua storia, e un unico job storico
// avrebbe letto l'archivio di una casella sola facendo credere di averle coperte tutte. Un job per
// casella attiva, chiave `sync_storico:<casella_id>`: al più uno pendente per ciascuna — il clic
// dato mentre il precedente gira non accoda niente e riporta il badge di quello in corso.
func (s *Server) syncStorico(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	caselle, err := q.ListCaselleAttive(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var ultimo *db.Job
	for _, c := range caselle {
		if c.Canale != db.CanaleOutlook {
			continue
		}
		chiave := "sync_storico:" + c.CasellaID.String()
		if j, err := q.JobPendentePerChiave(ctx, pgtype.Text{String: chiave, Valid: true}); err == nil {
			ultimo = &j
			continue
		}
		al, dal, cartelle, err := s.finestraStorico(ctx, q, c.CasellaID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		cid := c.CasellaID
		payload := worker.PayloadSyncOutlook{CasellaID: &cid, Modo: worker.ModoStorico, Cartelle: cartelle,
			Dal: dal, Al: &al, SovrapposizioneS: 0, Lotto: 50}
		job, err := coda.AccodaCon(ctx, q, db.TipoJobSyncOutlook, payload, chiave, coda.PrioritaSyncStorico,
			coda.Opzioni{Casella: uuid.NullUUID{UUID: cid, Valid: true}})
		if err != nil {
			s.Log.Error("accoda sync storico", "casella", c.Indirizzo, "err", err)
			http.Error(w, err.Error(), 500)
			return
		}
		if job != nil {
			ultimo = job
		}
	}
	if ultimo == nil {
		s.avviso(w, "Nessuna casella attiva da cui caricare l'archivio.")
		return
	}
	s.badgeStorico(w, ultimo)
}

// finestraStorico calcola la finestra di «Carica precedenti» per UNA casella: una PER CARTELLA, più
// l'inviluppo [dal, al] che il badge mostra all'operatore.
//
// «Quanto indietro siamo già andati» è una proprietà di (casella, cartella), e va trattata come
// tale. Prima si prendeva il più VECCHIO degli `storico_fino_a` della casella e lo si applicava a
// tutte le cartelle: la cartella rimasta più avanti riceveva una finestra che non arrivava a
// toccare la propria frontiera, e a fine job si scriveva lo stesso il nuovo limite — cioè si
// dichiarava coperto un intervallo che quella cartella non aveva mai letto. Due cartelle che
// scendono a velocità diverse bastavano a produrlo, e nessuno se ne sarebbe accorto.
//
// Le finestre si incastrano senza buchi perché ciascuna riparte esattamente dal proprio limite: `al`
// è lo `storico_fino_a` lasciato dal clic precedente su QUELLA cartella, cioè il suo `dal`, quindi
// la successiva è [al - GiorniStorico, al] esatta. Il primo clic parte dalla mail più vecchia che la
// casella ha in archivio (o da adesso, se non ne ha nessuna).
func (s *Server) finestraStorico(ctx context.Context, q *db.Queries, casella uuid.UUID) (al, dal time.Time, cartelle []worker.CartellaCursore, err error) {
	cursori, _ := q.ListSyncCursoriCasella(ctx, casella)
	// il fondo di ripiego, per le cartelle che non hanno ancora un limite storico: la mail più
	// vecchia della casella, o adesso se non ce n'è nessuna
	partenza := time.Time{}
	for _, c := range cursori {
		if c.StoricoFinoA != nil && (partenza.IsZero() || c.StoricoFinoA.Before(partenza)) {
			partenza = *c.StoricoFinoA
		}
	}
	if partenza.IsZero() {
		var minData *time.Time
		if err = s.Pool.QueryRow(ctx, `SELECT min(mc.ricevuto_il) FROM messaggio_casella mc WHERE mc.casella_id = $1`, casella).Scan(&minData); err != nil {
			return
		}
		if minData != nil {
			partenza = *minData
		} else {
			partenza = time.Now()
		}
	}
	nomi := make([]string, 0, len(cursori))
	limite := map[string]time.Time{}
	for _, c := range cursori {
		nomi = append(nomi, c.Cartella)
		if c.StoricoFinoA != nil {
			limite[c.Cartella] = *c.StoricoFinoA
		}
	}
	if len(nomi) == 0 {
		nomi = []string{"Inbox", "Sent Items"}
	}
	for _, nome := range nomi {
		fine, ok := limite[nome]
		if !ok {
			fine = partenza
		}
		inizio := fine.AddDate(0, 0, -GiorniStorico)
		f, i := fine, inizio
		cartelle = append(cartelle, worker.CartellaCursore{Cartella: nome, Dal: &i, Al: &f})
		if al.IsZero() || fine.After(al) {
			al = fine
		}
		if dal.IsZero() || inizio.Before(dal) {
			dal = inizio
		}
	}
	return
}

// syncStoricoStato è il frammento che il badge ricarica finché il job non è chiuso.
func (s *Server) syncStoricoStato(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	if j, err := q.JobPendenteConPrefisso(r.Context(), "sync_storico"); err == nil {
		s.badgeStorico(w, &j)
		return
	}
	j, err := q.UltimoJobPerChiavePrefisso(r.Context(), "sync_storico")
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.badgeStorico(w, &j)
}

func (s *Server) badgeStorico(w http.ResponseWriter, j *db.Job) {
	var p worker.PayloadSyncOutlook
	_ = json.Unmarshal(j.Payload, &p)
	finestra := p.Dal.Local().Format("02/01/2006")
	if p.Al != nil {
		finestra += " – " + p.Al.Local().Format("02/01/2006")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch j.Stato {
	case db.StatoJobPronto, db.StatoJobInCorso:
		fmt.Fprintf(w, `<span class="badge avviso" hx-get="/inbox/sync-storico/stato" hx-trigger="every 3s" hx-swap="outerHTML">Sync storico #%d %s [%s]…</span>`, j.JobID, j.Stato, finestra)
	case db.StatoJobFatto:
		fmt.Fprintf(w, `<span class="badge verde">Sync storico #%d completato [%s]. Premi di nuovo per i %d giorni prima.</span>`, j.JobID, finestra, GiorniStorico)
	default:
		fmt.Fprintf(w, `<span class="badge errore">Sync storico #%d fallito: %s</span>`, j.JobID, template.HTMLEscapeString(j.Errore.String))
	}
}

type messaggioDati struct {
	M    db.Messaggio
	Riga db.VInbox
	// Copia è la copia su cui agiscono i pulsanti («Apri in Outlook», «Segna letto», «Bozza»): quella
	// che il worker della POSTAZIONE DELLA SESSIONE serve (voci 2.2 e 2.7). Nil = le azioni non sono
	// disponibili, e MotivoAzioni dice perché (nessuna postazione, o nessun worker idoneo su quella).
	// Presenze è l'elenco completo, che è ciò che l'operatore deve vedere per capire da dove arriva.
	Copia        *coda.Copia
	MotivoAzioni string
	Presenze     []db.ListPresenzeRow
	Allegati     []AllegatoUI
	Portale      []db.RiferimentoPortale
	Bozze        []db.Bozza
	Thread       *db.ThreadOfferta
	Avviso       string
	// Analisi è la proposta dell'agente semantico, già verificata contro il testo del messaggio
	// (checkpoint 3R §9). Vuota finché nessuno l'ha chiesta; `Disponibile` dice se il pulsante ha
	// senso su questa casella.
	Analisi analisiUI
	// Candidati sono le proposte di aggancio R0–R5 con l'evidenza: si vedono nel pannello, prima di
	// aprire un form, perché è lì che si decide se questo messaggio è una richiesta nuova o no.
	Candidati []db.ListCandidatiAggancioRow
	// Blocco 7B: la posta dei fornitori. CandidatiRichiesta sono le richieste nostre a cui questa
	// mail potrebbe rispondere (R0, R1, R3f); PropostaThread + PropostaFornitore è «richiesta a X
	// per la RFQ Y» su una nostra mail mandata a mano; Richiesta è quella già collegata.
	CandidatiRichiesta []db.ListCandidatiRichiestaRow
	PropostaThread     *db.ThreadOfferta
	PropostaFornitore  *db.Fornitore
	Richiesta          *db.RichiestaFornitore
	RichiestaFornitore string
	Lavorazioni        []db.Lavorazione
}

// Agganciato: il messaggio appartiene a una RFQ (i download sono consentiti).
func (d *messaggioDati) Agganciato() bool { return d.Thread != nil }

// allegatiVista e rigaAllegato sono i dati passati ai frammenti "allegati_tabella" e "allegato_riga",
// condivisi fra il pannello del messaggio e la schermata B.
type allegatiVista struct {
	Allegati      []AllegatoUI
	MessaggioID   uuid.UUID
	Agganciato    bool
	RitornaThread string
	NScaricabili  int
}

type rigaAllegato struct {
	A             AllegatoUI
	Agganciato    bool
	RitornaThread string
	Figlio        bool
}

func (s *Server) messaggio(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaMessaggio(r.Context(), id, sessioneDa(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		// il link diretto riapre l'Inbox nel quadrante del messaggio, con «tutti» perché il
		// messaggio potrebbe essere già agganciato o ignorato
		http.Redirect(w, r, "/inbox?q="+d.Riga.Quadrante+"&filtro=tutti&sel="+id.String(), http.StatusFound)
		return
	}
	// Aprire un messaggio cambia lo stato della schermata: la riga scelta va evidenziata, e il
	// messaggio appena letto perde il pallino delle novita'. Il clic pero' sostituisce solo il
	// pannello, e la colonna di sinistra resterebbe ferma fino al poll successivo — fino a quindici
	// secondi con l'indirizzo che dice `sel=B` e l'evidenziatura ancora su A. Qui il server dice
	// alla schermata di rifare la colonna, DOPO che l'indirizzo e' stato aggiornato
	// (After-Settle): la ricarica rilegge l'indirizzo e torna con la lista giusta e la riga accesa.
	w.Header().Set("HX-Trigger-After-Settle", "inbox-aggiorna")
	s.frammento(w, "messaggio_pannello", d)
}

func (s *Server) caricaMessaggio(ctx context.Context, id uuid.UUID, sess sessioneUI) (*messaggioDati, error) {
	q := db.New(s.Pool)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &messaggioDati{M: m}
	d.Riga, _ = q.GetInboxRiga(ctx, id)
	d.Presenze, _ = q.ListPresenze(ctx, id)
	if len(d.Presenze) > 0 {
		d.Copia, d.MotivoAzioni = s.copiaInterattiva(ctx, q, id, sess)
	}
	if m.ThreadID.Valid {
		if t, err := q.GetThread(ctx, m.ThreadID.UUID); err == nil {
			d.Thread = &t
		}
	}
	d.Allegati, _ = s.allegatiUI(ctx, q, id)
	rp, _ := s.Pool.Query(ctx, `SELECT * FROM riferimento_portale WHERE messaggio_id = $1 ORDER BY creato_il`, id)
	if rp != nil {
		d.Portale, _ = pgx.CollectRows(rp, pgx.RowToStructByName[db.RiferimentoPortale])
	}
	bz, _ := s.Pool.Query(ctx, `SELECT * FROM bozza WHERE in_risposta_a = $1 ORDER BY creata_il DESC`, id)
	if bz != nil {
		d.Bozze, _ = pgx.CollectRows(bz, pgx.RowToStructByName[db.Bozza])
	}
	if !m.ThreadID.Valid {
		d.Candidati, _ = q.ListCandidatiAggancio(ctx, id)
		d.CandidatiRichiesta, _ = q.ListCandidatiRichiesta(ctx, id)
		// «richiesta a X per la RFQ Y»: una nostra mail a un fornitore che cita una RFQ aperta (7B)
		if p, err := q.GetTriageMessaggio(ctx, id); err == nil && p.Atto.String == classificazione.AttoRichiestaOfferta && m.Direzione == db.DirezioneUscita &&
			p.ThreadProposto.Valid && p.FornitoreProposto.Valid && p.Stato == db.StatoTriageProposta {
			if t, err := q.GetThread(ctx, p.ThreadProposto.UUID); err == nil {
				if f, err := q.GetFornitore(ctx, p.FornitoreProposto.UUID); err == nil {
					d.PropostaThread, d.PropostaFornitore = &t, &f
					d.Lavorazioni, _ = q.ListLavorazioni(ctx)
				}
			}
		}
	}
	if m.RichiestaFornitoreID.Valid {
		if ric, err := q.GetRichiesta(ctx, m.RichiestaFornitoreID.UUID); err == nil {
			d.Richiesta = &ric
			if f, err := q.GetFornitore(ctx, ric.FornitoreID); err == nil {
				d.RichiestaFornitore = f.RagioneSociale
			}
		}
	}
	var caselle []string
	for _, p := range d.Presenze {
		caselle = append(caselle, p.CasellaIndirizzo)
	}
	d.Analisi = s.analisiPer(ctx, q, id, caselle)
	return d, nil
}

// copiaInterattiva decide su quale copia agisce un job interattivo chiesto in QUESTA sessione, o
// spiega perché non può: senza postazione, o senza un worker idoneo su quella postazione. Il motivo
// è per l'operatore, quindi è una frase e non un codice.
func (s *Server) copiaInterattiva(ctx context.Context, q *db.Queries, id uuid.UUID, sess sessioneUI) (*coda.Copia, string) {
	var richiedente uuid.UUID
	if sess.Utente != nil {
		richiedente = sess.Utente.UtenteID
	}
	c, err := coda.CopiaPerPostazione(ctx, q, id, sess.Postazione, richiedente)
	switch {
	case err == nil:
		return &c, ""
	case errors.Is(err, coda.ErrNessunaPostazione):
		return nil, "nessuna postazione associata a questa sessione: scegli dalla testata il PC su cui stai lavorando"
	}
	var nessuno *coda.ErrNessunWorkerIdoneo
	if errors.As(err, &nessuno) {
		return nil, nessuno.Motivo()
	}
	return nil, err.Error()
}

// accodaInterattivo è il percorso comune di «Apri in Outlook» e «Segna letto» (voci 2.2, 2.7, M3):
// la copia servita dalla postazione della sessione, il job con casella + postazione + richiedente +
// scadenza. Se non c'è una copia servibile NON accoda niente e restituisce il motivo (M4, M10): un
// job «alla prima copia disponibile» aprirebbe la finestra su un altro PC.
func (s *Server) accodaInterattivo(ctx context.Context, q *db.Queries, id uuid.UUID, sess sessioneUI, tipo db.TipoJob, payload func(coda.Copia, db.Messaggio) any, priorita int16) (*coda.Copia, string, error) {
	c, motivo := s.copiaInterattiva(ctx, q, id, sess)
	if c == nil {
		return nil, motivo, nil
	}
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if _, err := coda.AccodaCon(ctx, q, tipo, payload(*c, m), "", priorita, coda.OpzioniInterattive(tipo, *c, sess.Postazione, sess.Utente.UtenteID)); err != nil {
		if errors.Is(err, coda.ErrCapacitaSpenta) {
			return nil, motivoCapacita(err, tipo), nil
		}
		return nil, "", err
	}
	return c, "", nil
}

// motivoShadow è la frase che l'operatore legge quando preme un pulsante che in shadow non parte.
// Dice tre cose: che cosa non è successo, perché, e dove si cambia — perché un rifiuto senza la
// terza è indistinguibile da un guasto.
// motivoCapacita e' la frase che legge l'operatore quando un'azione non parte perche' la capacita'
// che le serve e' spenta. Dice QUALE capacita' e DOVE si accende: prima diceva «il server e' in
// modalita' shadow», che era vero ma non aiutava — spegneva tre cose insieme e non si capiva quale
// riguardasse il pulsante appena premuto.
func motivoCapacita(err error, tipo db.TipoJob) string {
	cap := coda.CapacitaMancante(err)
	if cap == "" {
		cap = coda.CapacitaPer(tipo)
	}
	return fmt.Sprintf("%s non viene eseguito: la capacità [sicurezza].%s è spenta su questo server. "+
		"Si accende in cockpit.toml (e richiede [server].modalita = \"produzione\")", tipo, cap)
}

// apriInOutlook accoda apri_elemento_outlook con priorità massima: il worker della postazione della
// sessione fa Display() sull'elemento nella casella che serve.
func (s *Server) apriInOutlook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	sess := sessioneDa(r.Context())
	c, motivo, err := s.accodaInterattivo(r.Context(), db.New(s.Pool), id, sess, db.TipoJobApriElementoOutlook, func(c coda.Copia, m db.Messaggio) any {
		return worker.PayloadApriElemento{EntryID: c.EntryID, RiferimentoElemento: rifIn(m, c.CasellaID)}
	}, 1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if c == nil {
		s.avvisoErrore(w, "Non aperto: "+motivo)
		return
	}
	s.avviso(w, fmt.Sprintf("Richiesta inviata a Outlook: la copia in %s si apre su %s.", c.CasellaNome, sess.NomeHost))
}

func (s *Server) segnaLetto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	letto := r.FormValue("letto") != "0"
	sess := sessioneDa(r.Context())
	c, motivo, err := s.accodaInterattivo(r.Context(), q, id, sess, db.TipoJobSegnaLetto, func(c coda.Copia, m db.Messaggio) any {
		return worker.PayloadSegnaLetto{EntryID: c.EntryID, Letto: letto, RiferimentoElemento: rifIn(m, c.CasellaID)}
	}, 2)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if c == nil {
		s.avvisoErrore(w, "Non aggiornato: "+motivo)
		return
	}
	// Si segna letta la COPIA su cui si è agito, non «il messaggio»: le altre caselle hanno il loro
	// stato di lettura e nessuno ha chiesto di toccarlo.
	if err := q.SetNonLettoPresenza(r.Context(), db.SetNonLettoPresenzaParams{MessaggioID: id, CasellaID: c.CasellaID, NonLetto: !letto}); err != nil {
		s.Log.Warn("stato di lettura non aggiornato", "messaggio", id, "err", err)
	}
	s.avviso(w, "Aggiornato in Outlook ("+c.CasellaNome+").")
}

// bozza prepara la risposta nel Cockpit e la apre come bozza in Outlook (l'invio resta manuale, SPEC §6.1).
func (s *Server) bozza(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	sess := sessioneDa(r.Context())
	tipo := db.TipoBozza(r.FormValue("tipo"))
	if !tipo.Valid() || tipo == db.TipoBozzaNuovo {
		tipo = db.TipoBozzaRisposta
	}
	corpo := strings.TrimSpace(r.FormValue("corpo"))
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	// La bozza si apre in Outlook sul PC del richiedente: stessa regola di «Apri», nessun ripiego.
	pr, motivo := s.copiaInterattiva(ctx, q, id, sess)
	if pr == nil {
		s.avvisoErrore(w, "Bozza non preparata: "+motivo)
		return
	}
	b, err := q.InsertBozza(ctx, db.InsertBozzaParams{
		ThreadID: m.ThreadID, InRispostaA: uuid.NullUUID{UUID: id, Valid: true}, Tipo: tipo,
		Destinatari: json.RawMessage("[]"), Oggetto: m.Oggetto, Corpo: pgtype.Text{String: corpo, Valid: corpo != ""}, Documenti: []uuid.UUID{}, CreataDa: u.UtenteID,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	html := ""
	if corpo != "" {
		html = "<div style=\"font-family:Calibri,sans-serif;font-size:11pt\">" + strings.ReplaceAll(template.HTMLEscapeString(corpo), "\n", "<br>") + "</div><br>"
	}
	if _, err := coda.AccodaCon(ctx, q, db.TipoJobCreaBozzaOutlook, worker.PayloadCreaBozza{
		BozzaID: b.BozzaID, Tipo: string(tipo), EntryID: pr.EntryID, Destinatari: []worker.Destinatario{},
		CorpoHTML: html, CorpoTesto: corpo, Allegati: []string{}, Mostra: true, Invia: false, RiferimentoElemento: rifIn(m, pr.CasellaID),
	}, "bozza:"+b.BozzaID.String(), 1, coda.OpzioniInterattive(db.TipoJobCreaBozzaOutlook, *pr, sess.Postazione, u.UtenteID)); err != nil {
		if errors.Is(err, coda.ErrCapacitaSpenta) {
			// niente Commit: la bozza non è stata preparata, e una riga `bozza` senza la finestra in
			// Outlook sarebbe una risposta che l'operatore crede di avere e non ha
			s.avvisoErrore(w, "Bozza non preparata: "+motivoCapacita(err, db.TipoJobCreaBozzaOutlook))
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.avviso(w, fmt.Sprintf("Bozza in preparazione: si apre in Outlook su %s tra pochi secondi. Rileggi e premi Invia lì.", sess.NomeHost))
}

// rif costruisce il riferimento stabile all'elemento (Message-ID) per i job che lo devono ritrovare in Outlook.
func rif(m db.Messaggio) worker.RiferimentoElemento {
	id := m.MessaggioID
	return worker.RiferimentoElemento{MessaggioID: &id, MessageID: m.ChiaveEsterna}
}

// rifIn è rif per un'azione su UNA copia: porta anche la casella, così quando il worker ritrova
// l'elemento a un EntryID diverso il server riallinea quella presenza e non un'altra.
func rifIn(m db.Messaggio, casella uuid.UUID) worker.RiferimentoElemento {
	r := rif(m)
	c := casella
	r.CasellaID = &c
	return r
}

func (s *Server) statoWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["inbox.html"].ExecuteTemplate(w, "stato_worker", vista{Utente: utenteDa(r.Context()), Stato: s.stato(r.Context(), sessioneDa(r.Context())), Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "stato_worker", "err", err)
	}
}
