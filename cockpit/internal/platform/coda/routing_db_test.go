//go:build integrazione

// L4 — voci 2.2, 2.6, 2.7: routing per casella e postazione (Q8, Q17, Q21, M3, M6, M9, M10, M12).
//
// Il filo conduttore: un job va SOLO a chi può eseguirlo davvero — un worker che serve quella casella
// nel proprio profilo, sulla postazione del richiedente quando è interattivo — e se nessuno può,
// resta lì (o non nasce) invece di finire «alla prima copia disponibile» su un altro PC.
package coda

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// scenario è la configurazione delle prove di routing: tre caselle (Commerciale condivisa, Francesco,
// Luigi), due postazioni, due worker Outlook. Il worker di PC-FRANCESCO serve Francesco e
// Commerciale; quello di PC-LUIGI serve Luigi e Commerciale. È il caso «due worker sulla stessa
// Commerciale» di M9, e il caso «Francesco senza accesso a Luigi» di M10.
type scenario struct {
	pool                          *pgxpool.Pool
	q                             *db.Queries
	ctx                           context.Context
	commerciale, francesco, luigi uuid.UUID
	pcFrancesco, pcLuigi          uuid.UUID
	utenteFP, utenteLU            uuid.UUID
	destFrancesco, destLuigi      Destinazione
}

func configRouting() *config.Config {
	c := &config.Config{}
	c.Outlook.CasellaDefault = "francesco@azienda.example"
	c.Utenti = []config.Utente{
		{Sigla: "FP", Nome: "Francesco", Ufficio: "Commerciale", Ruolo: "operatore", Password: "prova"},
		{Sigla: "LU", Nome: "Luigi", Ufficio: "Tecnico", Ruolo: "tecnico", Password: "prova"},
	}
	c.Caselle = []config.Casella{
		{Indirizzo: "commerciale@azienda.example", Nome: "Commerciale", Canale: "outlook", Condivisa: true},
		{Indirizzo: "francesco@azienda.example", Nome: "Francesco", Canale: "outlook", Utente: "FP"},
		{Indirizzo: "luigi@azienda.example", Nome: "Luigi", Canale: "outlook", Utente: "LU"},
	}
	c.Postazioni = []config.Postazione{{NomeHost: "PC-FRANCESCO", Utente: "FP"}, {NomeHost: "PC-LUIGI", Utente: "LU"}}
	c.Worker = []config.Worker{
		{Nome: "outlook@PC-FRANCESCO", Tipo: "outlook", Token: "segreto-francesco", Postazione: "PC-FRANCESCO",
			Caselle: []string{"francesco@azienda.example", "commerciale@azienda.example"}},
		{Nome: "outlook@PC-LUIGI", Tipo: "outlook", Token: "segreto-luigi", Postazione: "PC-LUIGI",
			Caselle: []string{"luigi@azienda.example", "commerciale@azienda.example"}},
	}
	return c
}

func preparaScenario(t *testing.T) *scenario {
	t.Helper()
	p, q, ctx := preparaDB(t)
	cfg := configRouting()
	// gli utenti si inseriscono qui (senza password): il seed di web non è importabile da un test
	// interno di coda senza un ciclo di import
	for _, u := range cfg.Utenti {
		if _, err := p.Exec(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ($1, $2, $3, $4::ruolo_utente)`, u.Sigla, u.Nome, u.Ufficio, u.Ruolo); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	s := &scenario{pool: p, q: q, ctx: ctx}
	casella := func(ind string) uuid.UUID {
		c, err := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: ind})
		if err != nil {
			t.Fatal(err)
		}
		return c.CasellaID
	}
	s.commerciale, s.francesco, s.luigi = casella("commerciale@azienda.example"), casella("francesco@azienda.example"), casella("luigi@azienda.example")
	post := func(host string) uuid.UUID {
		p, err := q.GetPostazionePerHost(ctx, host)
		if err != nil {
			t.Fatal(err)
		}
		return p.PostazioneID
	}
	s.pcFrancesco, s.pcLuigi = post("PC-FRANCESCO"), post("PC-LUIGI")
	utente := func(sigla string) uuid.UUID {
		u, err := q.GetUtentePerSigla(ctx, sigla)
		if err != nil {
			t.Fatal(err)
		}
		return u.UtenteID
	}
	s.utenteFP, s.utenteLU = utente("FP"), utente("LU")
	// ciò che i due worker dichiarano al claim, già intersecato con le credenziali
	s.destFrancesco = Destinazione{Caselle: []uuid.UUID{s.francesco, s.commerciale}, Postazione: uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}}
	s.destLuigi = Destinazione{Caselle: []uuid.UUID{s.luigi, s.commerciale}, Postazione: uuid.NullUUID{UUID: s.pcLuigi, Valid: true}}
	return s
}

// messaggioIn crea un messaggio con una presenza in ognuna delle caselle indicate.
func (s *scenario) messaggioIn(t *testing.T, chiave string, caselle ...uuid.UUID) uuid.UUID {
	t.Helper()
	var convID, msgID uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now())
		RETURNING conversazione_id`, "CONV-"+chiave).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, mittente_indirizzo)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ routing', 'mario.rossi@acme.example') RETURNING messaggio_id`, chiave, convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	for i, c := range caselle {
		if _, err := s.pool.Exec(s.ctx, `INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, cartella, ricevuto_il)
			VALUES ($1, $2, $3, 'Posta in arrivo', now() - make_interval(mins => $4))`, msgID, c, "ENTRY-"+chiave+"-"+c.String()[:8], 10-i); err != nil {
			t.Fatal(err)
		}
	}
	return msgID
}

func claimCon(t *testing.T, s *scenario, worker string, d Destinazione) *db.Job {
	t.Helper()
	j, err := Claim(s.ctx, s.q, db.WorkerTipoOutlook, worker, d, 0)
	if err != nil {
		t.Fatalf("claim %s: %v", worker, err)
	}
	return j
}

// Q8 — routing per casella e postazione: job A (Commerciale), B (Francesco + PC-FRANCESCO), C (server).
// PC-LUIGI con [Luigi, Commerciale] prende A; PC-FRANCESCO prende B; il server prende C.
func TestQ8RoutingPerCasellaEPostazione(t *testing.T) {
	s := preparaScenario(t)
	a := accoda(t, s.ctx, s.q, db.TipoJobSyncOutlook, "A", Opzioni{Casella: uuid.NullUUID{UUID: s.commerciale, Valid: true}})
	b := accoda(t, s.ctx, s.q, db.TipoJobApriElementoOutlook, "B", Opzioni{
		Casella: uuid.NullUUID{UUID: s.francesco, Valid: true}, Postazione: uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}})
	c := accoda(t, s.ctx, s.q, db.TipoJobCopiaNas, "C", Opzioni{})

	// PC-LUIGI: può prendere A (Commerciale è sua) ma non B (casella e postazione di Francesco)
	if j := claimCon(t, s, "outlook@PC-LUIGI", s.destLuigi); j == nil || j.JobID != a.JobID {
		t.Fatalf("PC-LUIGI doveva prendere A (#%d), ha preso %v", a.JobID, j)
	}
	if j := claimCon(t, s, "outlook@PC-LUIGI", s.destLuigi); j != nil {
		t.Fatalf("PC-LUIGI ha preso anche #%d (%s): non è né la sua casella né la sua postazione", j.JobID, j.Tipo)
	}
	// PC-FRANCESCO prende B
	if j := claimCon(t, s, "outlook@PC-FRANCESCO", s.destFrancesco); j == nil || j.JobID != b.JobID {
		t.Fatalf("PC-FRANCESCO doveva prendere B (#%d), ha preso %v", b.JobID, j)
	}
	// il server prende C, e non tocca i job Outlook
	if j, err := Claim(s.ctx, s.q, db.WorkerTipoServer, "server", Destinazione{}, 0); err != nil || j == nil || j.JobID != c.JobID {
		t.Fatalf("il server doveva prendere C (#%d): job=%v err=%v", c.JobID, j, err)
	}
}

// M12 (lato server) — un worker che NON ha la casella nel profilo (non la dichiara) non riceve il
// job, anche se la credenziale lo autorizzerebbe; e il payload non contiene nessuno store_id.
func TestM12NessunoStoreIDNeiPayloadEClaimSoloConCasellaAperta(t *testing.T) {
	s := preparaScenario(t)
	msg := s.messaggioIn(t, "<m12@acme.example>", s.commerciale)
	m, _ := s.q.GetMessaggio(s.ctx, msg)
	pr, err := s.q.PresenzaDaAprire(s.ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	var allID uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, ricevuto_il)
		VALUES ($1, 1, 'disegno.pdf', 'pdf', 'file', 'outlook', 100, now()) RETURNING allegato_id`, msg).Scan(&allID); err != nil {
		t.Fatal(err)
	}
	a, _ := s.q.GetAllegato(s.ctx, allID)
	_, j, err := AccodaStage(s.ctx, s.q, stagingFinto{}, a, m, Copia{CasellaID: pr.CasellaID, EntryID: pr.EntryID}, 1)
	if err != nil || j == nil {
		t.Fatalf("accoda stage: job=%v err=%v", j, err)
	}
	var payload map[string]any
	_ = json.Unmarshal(j.Payload, &payload)
	if _, c := payload["store_id"]; c {
		t.Errorf("il payload dello stage porta uno store_id: %s", j.Payload)
	}
	if payload["casella_id"] != s.commerciale.String() || payload["entry_id"] != pr.EntryID || payload["message_id"] != "<m12@acme.example>" {
		t.Errorf("il payload non porta le identità logiche attese: %s", j.Payload)
	}
	if !j.CasellaID.Valid || j.CasellaID.UUID != s.commerciale {
		t.Errorf("il job non è vincolato alla casella: %v", j.CasellaID)
	}

	// PC-LUIGI è AUTORIZZATO su Commerciale ma oggi non la dichiara (non l'ha trovata nel profilo)
	senzaCommerciale := Destinazione{Caselle: []uuid.UUID{s.luigi}, Postazione: s.destLuigi.Postazione}
	if got := claimCon(t, s, "outlook@PC-LUIGI", senzaCommerciale); got != nil {
		t.Fatalf("il job è stato assegnato a un worker che non ha la casella aperta (#%d)", got.JobID)
	}
	// quando la dichiara, lo prende
	if got := claimCon(t, s, "outlook@PC-LUIGI", s.destLuigi); got == nil || got.JobID != j.JobID {
		t.Fatalf("con Commerciale aperta PC-LUIGI doveva prendere #%d, ha preso %v", j.JobID, got)
	}
}

// M3 — «Apri» va alla postazione del richiedente: la copia è quella nella casella che il worker di
// PC-FRANCESCO serve, e il job porta casella, postazione, richiesto_da e scadenza.
func TestM3ApriVaAllaPostazioneDelRichiedente(t *testing.T) {
	s := preparaScenario(t)
	msg := s.messaggioIn(t, "<m3@acme.example>", s.commerciale, s.francesco)
	c, err := CopiaPerPostazione(s.ctx, s.q, msg, uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}, s.utenteFP)
	if err != nil {
		t.Fatalf("copia per PC-FRANCESCO: %v", err)
	}
	// preferisce la casella personale del richiedente, anche se la copia in Commerciale è più vecchia
	if c.CasellaID != s.francesco || c.WorkerNome != "outlook@PC-FRANCESCO" {
		t.Fatalf("copia scelta: casella=%s worker=%s, attesa Francesco/outlook@PC-FRANCESCO", c.CasellaNome, c.WorkerNome)
	}
	mid := msg
	j, err := AccodaCon(s.ctx, s.q, db.TipoJobApriElementoOutlook, worker.PayloadApriElemento{EntryID: c.EntryID,
		RiferimentoElemento: worker.RiferimentoElemento{MessaggioID: &mid, CasellaID: &c.CasellaID, MessageID: "<m3@acme.example>"}},
		"", 1, OpzioniInterattive(db.TipoJobApriElementoOutlook, c, uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}, s.utenteFP))
	if err != nil || j == nil {
		t.Fatalf("accoda apri: %v", err)
	}
	if !j.CasellaID.Valid || j.CasellaID.UUID != s.francesco || !j.PostazioneID.Valid || j.PostazioneID.UUID != s.pcFrancesco ||
		!j.RichiestoDa.Valid || j.RichiestoDa.UUID != s.utenteFP || j.ScadeIl == nil {
		t.Fatalf("job senza i vincoli attesi: casella=%v postazione=%v richiesto_da=%v scade=%v", j.CasellaID, j.PostazioneID, j.RichiestoDa, j.ScadeIl)
	}
	if d := time.Until(*j.ScadeIl); d < 9*time.Minute || d > 11*time.Minute {
		t.Errorf("scadenza di «apri» a %v, attesi ~10 minuti", d)
	}
	if strings.Contains(string(j.Payload), "store_id") {
		t.Errorf("il payload di «apri» porta uno store_id: %s", j.Payload)
	}
	// M9: «Apri» solo sulla postazione richiesta. PC-LUIGI serve Commerciale, dove il messaggio c'è,
	// ma il job è di PC-FRANCESCO: non lo prende.
	if got := claimCon(t, s, "outlook@PC-LUIGI", s.destLuigi); got != nil {
		t.Fatalf("PC-LUIGI ha preso il job interattivo di PC-FRANCESCO (#%d)", got.JobID)
	}
	if got := claimCon(t, s, "outlook@PC-FRANCESCO", s.destFrancesco); got == nil || got.JobID != j.JobID {
		t.Fatalf("PC-FRANCESCO doveva prendere #%d, ha preso %v", j.JobID, got)
	}
}

// M10 / M4 — nessun ripiego alla prima copia: il messaggio è solo in Luigi, il richiedente è su
// PC-FRANCESCO il cui worker non serve Luigi. Nessun job, motivo esplicito. E senza postazione
// nessun job nemmeno se la casella sarebbe servibile.
func TestM10NessunFallbackAllaPrimaCopia(t *testing.T) {
	s := preparaScenario(t)
	msg := s.messaggioIn(t, "<m10@acme.example>", s.luigi)
	_, err := CopiaPerPostazione(s.ctx, s.q, msg, uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}, s.utenteFP)
	var nessuno *ErrNessunWorkerIdoneo
	if !errors.As(err, &nessuno) {
		t.Fatalf("attesa ErrNessunWorkerIdoneo, ottenuto %v", err)
	}
	motivo := nessuno.Motivo()
	for _, atteso := range []string{"PC-FRANCESCO", "Luigi", "non viene dirottata"} {
		if !strings.Contains(motivo, atteso) {
			t.Errorf("il motivo non dice %q: %s", atteso, motivo)
		}
	}
	if n := testutil.Conta(t, s.pool, "job"); n != 0 {
		t.Fatalf("è stato creato un job (%d) pur senza worker idoneo", n)
	}
	// senza postazione: ErrNessunaPostazione, anche se Luigi sarebbe servibile da PC-LUIGI
	if _, err := CopiaPerPostazione(s.ctx, s.q, msg, uuid.NullUUID{}, s.utenteLU); !errors.Is(err, ErrNessunaPostazione) {
		t.Fatalf("senza postazione attesa ErrNessunaPostazione, ottenuto %v", err)
	}
	// da PC-LUIGI invece la copia c'è
	if c, err := CopiaPerPostazione(s.ctx, s.q, msg, uuid.NullUUID{UUID: s.pcLuigi, Valid: true}, s.utenteLU); err != nil || c.CasellaID != s.luigi {
		t.Fatalf("da PC-LUIGI attesa la copia in Luigi: %+v %v", c, err)
	}
}

// CopiaPerDownload — un download non apre finestre: senza postazione va sulla copia di riferimento;
// con una postazione che serve una copia, preferisce quella.
func TestCopiaPerDownloadNonRichiedeLaPostazione(t *testing.T) {
	s := preparaScenario(t)
	msg := s.messaggioIn(t, "<dl@acme.example>", s.commerciale, s.luigi)
	c, err := CopiaPerDownload(s.ctx, s.q, msg, uuid.NullUUID{}, s.utenteFP)
	if err != nil {
		t.Fatal(err)
	}
	if c.CasellaID != s.commerciale { // la più vecchia: la prima inserita
		t.Errorf("senza postazione attesa la copia di riferimento (Commerciale), ottenuta %s", c.CasellaNome)
	}
	c, err = CopiaPerDownload(s.ctx, s.q, msg, uuid.NullUUID{UUID: s.pcLuigi, Valid: true}, s.utenteLU)
	if err != nil || c.CasellaID != s.luigi {
		t.Errorf("da PC-LUIGI attesa la copia personale di Luigi: %+v %v", c, err)
	}
}

// M6 / M9 — sync per casella: tre caselle attive → tre job con chiave sync_outlook:<casella>, anche
// dopo cinque tick; e i due worker che servono Commerciale si contendono UN solo sync.
func TestM6SyncPerCasellaEUnSoloSyncPerCommerciale(t *testing.T) {
	s := preparaScenario(t)
	sched := &Scheduler{Q: s.q, Log: testutil.LogSilenzioso(), Cartelle: []string{"Inbox"}, Lotto: 10}
	for i := 0; i < 5; i++ {
		if err := sched.accodaSync(s.ctx); err != nil {
			t.Fatal(err)
		}
	}
	righe, err := s.q.ListJob(s.ctx, db.ListJobParams{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	chiavi := map[string]bool{}
	for _, j := range righe {
		if j.Tipo != db.TipoJobSyncOutlook || !j.CasellaID.Valid {
			t.Errorf("job inatteso: %s casella=%v", j.Tipo, j.CasellaID)
		}
		chiavi[j.ChiaveIdempotenza.String] = true
		if !strings.HasPrefix(j.ChiaveIdempotenza.String, "sync_outlook:"+j.CasellaID.UUID.String()) {
			t.Errorf("chiave %q non è per casella", j.ChiaveIdempotenza.String)
		}
	}
	if len(righe) != 3 || len(chiavi) != 3 {
		t.Fatalf("attesi 3 job sync (uno per casella) dopo 5 tick, trovati %d con %d chiavi", len(righe), len(chiavi))
	}
	// PC-FRANCESCO e PC-LUIGI servono entrambi Commerciale: un solo sync in corso per Commerciale
	presi := map[uuid.UUID]string{}
	for _, w := range []struct {
		nome string
		d    Destinazione
	}{{"outlook@PC-FRANCESCO", s.destFrancesco}, {"outlook@PC-LUIGI", s.destLuigi}, {"outlook@PC-FRANCESCO", s.destFrancesco}, {"outlook@PC-LUIGI", s.destLuigi}} {
		if j := claimCon(t, s, w.nome, w.d); j != nil {
			if altro, c := presi[j.CasellaID.UUID]; c {
				t.Fatalf("il sync di %s è stato preso da %s e da %s", j.CasellaID.UUID, altro, w.nome)
			}
			presi[j.CasellaID.UUID] = w.nome
		}
	}
	if len(presi) != 3 || presi[s.francesco] != "outlook@PC-FRANCESCO" || presi[s.luigi] != "outlook@PC-LUIGI" {
		t.Fatalf("assegnazione dei sync: %v", presi)
	}
}

// Q17 — job interattivo scaduto o annullato: 'annullato', mai claimato.
func TestQ17ScadutoOAnnullatoMaiClaimato(t *testing.T) {
	s := preparaScenario(t)
	fra1s := time.Now().Add(1 * time.Second)
	scade := accoda(t, s.ctx, s.q, db.TipoJobApriElementoOutlook, "q17-scade", Opzioni{
		Casella: uuid.NullUUID{UUID: s.francesco, Valid: true}, Postazione: uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}, ScadeIl: &fra1s})
	annulla := accoda(t, s.ctx, s.q, db.TipoJobApriElementoOutlook, "q17-annulla", Opzioni{
		Casella: uuid.NullUUID{UUID: s.francesco, Valid: true}, Postazione: uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}})

	// «Annulla» dalla UI
	if _, err := s.q.AnnullaJob(s.ctx, annulla.JobID); err != nil {
		t.Fatalf("annulla: %v", err)
	}
	// il tempo passa, lo scheduler passa
	time.Sleep(1200 * time.Millisecond)
	if n, err := s.q.AnnullaJobScaduti(s.ctx); err != nil || n != 1 {
		t.Fatalf("annullati dallo scheduler: n=%d err=%v", n, err)
	}
	for _, id := range []int64{scade.JobID, annulla.JobID} {
		if j := statoDi(t, s.ctx, s.q, id); j.Stato != db.StatoJobAnnullato || j.ChiusoIl == nil {
			t.Errorf("job #%d: stato %s, atteso annullato (chiuso_il=%v, errore=%q)", id, j.Stato, j.ChiusoIl, j.Errore.String)
		}
	}
	if j := claimCon(t, s, "outlook@PC-FRANCESCO", s.destFrancesco); j != nil {
		t.Fatalf("un job annullato è stato claimato: #%d", j.JobID)
	}
	// annullare un job chiuso non fa niente (zero righe)
	if _, err := s.q.AnnullaJob(s.ctx, annulla.JobID); err == nil {
		t.Error("annullare un job già annullato doveva dare zero righe")
	}
}

// Q21 — scadenza verificata AL CLAIM: scade_il già passato ma stato ancora 'pronto' (lo scheduler
// non è ancora passato). Il claim non lo restituisce; lo scheduler lo porta ad annullato.
func TestQ21ScadenzaVerificataAlClaim(t *testing.T) {
	s := preparaScenario(t)
	j := accoda(t, s.ctx, s.q, db.TipoJobApriElementoOutlook, "q21", Opzioni{
		Casella: uuid.NullUUID{UUID: s.francesco, Valid: true}, Postazione: uuid.NullUUID{UUID: s.pcFrancesco, Valid: true}})
	if _, err := s.pool.Exec(s.ctx, `UPDATE job SET scade_il = now() - interval '1 second' WHERE job_id = $1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	if got := claimCon(t, s, "outlook@PC-FRANCESCO", s.destFrancesco); got != nil {
		t.Fatalf("il claim ha restituito un job già scaduto (#%d)", got.JobID)
	}
	if st := statoDi(t, s.ctx, s.q, j.JobID); st.Stato != db.StatoJobPronto {
		t.Fatalf("prima dello scheduler il job doveva essere ancora 'pronto', è %s", st.Stato)
	}
	if n, err := s.q.AnnullaJobScaduti(s.ctx); err != nil || n != 1 {
		t.Fatalf("scheduler: n=%d err=%v", n, err)
	}
	if st := statoDi(t, s.ctx, s.q, j.JobID); st.Stato != db.StatoJobAnnullato {
		t.Fatalf("dopo lo scheduler atteso annullato, è %s", st.Stato)
	}
}

// Il worker Outlook con la destinazione vuota (niente caselle dichiarate) non prende job di casella:
// è il caso del worker appena avviato senza Outlook, o di un worker più vecchio del server.
func TestDestinazioneVuotaNonPrendeJobDiCasella(t *testing.T) {
	s := preparaScenario(t)
	accoda(t, s.ctx, s.q, db.TipoJobSyncOutlook, "vuota", Opzioni{Casella: uuid.NullUUID{UUID: s.francesco, Valid: true}})
	if j := claimCon(t, s, "outlook@PC-FRANCESCO", Destinazione{Postazione: s.destFrancesco.Postazione}); j != nil {
		t.Fatalf("un worker senza caselle dichiarate ha preso un job di casella (#%d)", j.JobID)
	}
}
