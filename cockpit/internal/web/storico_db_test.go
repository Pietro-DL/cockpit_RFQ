//go:build integrazione

// L4 — SS1–SS4: «Carica precedenti» scarica l'archivio a pezzi piccoli (voce 2.8, checkpoint del
// 16/09/2026).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/web/
//
// Il motivo di tutto il gruppo è la forma del worker: ce n'è uno per PC ed è seriale. Finché macina
// un job storico non prende «Apri in Outlook», non scarica un allegato e non fa il sync ordinario.
// Un job da trenta giorni su una casella viva era il Cockpit che smetteva di rispondere per un tempo
// che nessuno sapeva dire in anticipo — e chi guardava la schermata non vedeva un lavoro in corso,
// vedeva dei pulsanti che non facevano niente.
package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// jobStorici è l'ULTIMO sync storico accodato per ogni casella. Storico = con un limite superiore:
// è quello a distinguerlo dal sync ordinario, sia nel payload sia nel comportamento (per lo storico
// il cursore in avanti non si muove).
//
// L'ultimo, e non «uno qualsiasi», perché i job dei clic precedenti restano in tabella: prendere
// quello sbagliato farebbe passare SS2 leggendo due volte la stessa finestra.
func (b *bancoWeb) jobStorici() map[uuid.UUID]api.PayloadSyncOutlook {
	b.t.Helper()
	ultimo := map[uuid.UUID]int64{}
	out := map[uuid.UUID]api.PayloadSyncOutlook{}
	for _, j := range b.jobInterattivi() { // ListJob: job_id decrescente
		if j.Tipo != db.TipoJobSyncOutlook {
			continue
		}
		var p api.PayloadSyncOutlook
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			b.t.Fatal(err)
		}
		if p.Al == nil || p.CasellaID == nil {
			continue
		}
		if visto, gia := ultimo[*p.CasellaID]; gia && visto >= j.JobID {
			continue
		}
		ultimo[*p.CasellaID], out[*p.CasellaID] = j.JobID, p
	}
	return out
}

// finiscono fa quello che farebbe il worker quando un sync storico riesce: segna la finestra come
// coperta (`storico_fino_a` = il `dal` di quella finestra) e chiude il job. È la stessa scrittura di
// workerapi.applica, e senza di essa il clic successivo non saprebbe da dove ripartire.
func (b *bancoWeb) finiscono() {
	b.t.Helper()
	for _, j := range b.jobInterattivi() {
		var p api.PayloadSyncOutlook
		if j.Tipo != db.TipoJobSyncOutlook || json.Unmarshal(j.Payload, &p) != nil || p.Al == nil {
			continue
		}
		for _, c := range p.Cartelle {
			if err := b.q.SetStoricoFinoA(b.ctx, db.SetStoricoFinoAParams{
				CasellaID: *p.CasellaID, Cartella: c.Cartella, StoricoFinoA: &p.Dal,
			}); err != nil {
				b.t.Fatal(err)
			}
		}
		if _, err := b.pool.Exec(b.ctx, "UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1", j.JobID); err != nil {
			b.t.Fatal(err)
		}
	}
}

// claimaUnJob fa un claim vero, dalla porta, e restituisce il job che il server consegna (nil se non
// ne consegna nessuno). Serve a SS4: quale job il worker riceve non si deduce dalle priorità scritte
// nel codice, si guarda.
func (b *bancoWeb) claimaUnJob(nome, ip string, caselle ...uuid.UUID) *api.Job {
	b.t.Helper()
	aperte := make([]api.CasellaAperta, 0, len(caselle))
	for _, c := range caselle {
		aperte = append(aperte, api.CasellaAperta{CasellaID: c, StoreID: "STORE-" + c.String()[:8]})
	}
	corpo, _ := json.Marshal(api.ClaimRichiesta{Worker: "outlook", WorkerID: nome, AttesaS: 1, OutlookOk: true, CaselleAperte: aperte})
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", strings.NewReader(string(corpo)))
	r.Header.Set("X-Cockpit-Token", tokenDelWorker(nome))
	r.Header.Set("X-Prova-IP", ip)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		b.t.Fatalf("claim di %s: %d", nome, resp.StatusCode)
	}
	var j api.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		b.t.Fatal(err)
	}
	return &j
}

// dueGiorni è scritto qui a mano, e non come `GiorniStorico * 24h`, di proposito: un test che legge
// la costante che dovrebbe sorvegliare resta verde anche quando qualcuno la porta a trenta.
const dueGiorni = 48 * time.Hour

// SS1 — un clic scarica due giorni, per casella, con la finestra esatta.
func TestSS1CaricaPrecedentiScaricaDueGiorniPerVolta(t *testing.T) {
	b := preparaBancoWeb(t)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")

	if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
		t.Fatalf("carica precedenti: %d %s", resp.StatusCode, corpo)
	}
	storici := b.jobStorici()
	if len(storici) != 3 {
		t.Fatalf("un job storico per casella attiva: attesi 3, ottenuti %d", len(storici))
	}
	for casella, p := range storici {
		if p.Al == nil {
			t.Fatalf("%s: un job storico senza limite superiore leggerebbe fino a oggi", casella)
		}
		if ampiezza := p.Al.Sub(p.Dal); ampiezza != dueGiorni {
			t.Errorf("%s: finestra di %v, attesi esattamente %v [%v, %v]", casella, ampiezza, dueGiorni, p.Dal, *p.Al)
		}
		// nessun messaggio in archivio: si parte da adesso e si va indietro
		if d := time.Since(*p.Al); d > time.Minute {
			t.Errorf("%s: il limite superiore è %v fa, atteso adesso", casella, d)
		}
	}
}

// SS2 — due clic coprono due finestre contigue: nessun buco fra l'una e l'altra, e nessuna
// sovrapposizione oltre quella voluta (il limite superiore della seconda è il limite inferiore della
// prima, e un messaggio esattamente su quell'istante viene deduplicato per Message-ID).
func TestSS2IlSecondoClicRetrocedeDiAltriDueGiorniSenzaBuchi(t *testing.T) {
	b := preparaBancoWeb(t)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")
	carica := func() map[uuid.UUID]api.PayloadSyncOutlook {
		t.Helper()
		if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
			t.Fatalf("carica precedenti: %d %s", resp.StatusCode, corpo)
		}
		return b.jobStorici()
	}

	prima := carica()
	b.finiscono() // il worker ha fatto il suo giro
	dopo := carica()

	if len(dopo) != len(prima) {
		t.Fatalf("secondo clic: %d job storici, attesi %d", len(dopo), len(prima))
	}
	for casella, seconda := range dopo {
		primaFinestra, ok := prima[casella]
		if !ok {
			t.Fatalf("la casella %s non aveva una prima finestra", casella)
		}
		// Un millisecondo di tolleranza, non zero: il confine passa da `storico_fino_a`, cioè da una
		// colonna timestamptz, e PostgreSQL tiene i microsecondi mentre Go tiene i nanosecondi. La
		// differenza che si vuole escludere qui è di due giorni, non di 600 nanosecondi.
		if d := seconda.Al.Sub(primaFinestra.Dal); d > time.Millisecond || d < -time.Millisecond {
			t.Errorf("%s: la seconda finestra finisce a %v e la prima cominciava a %v (%v di scarto): c'è un buco o una sovrapposizione",
				casella, *seconda.Al, primaFinestra.Dal, d)
		}
		if ampiezza := seconda.Al.Sub(seconda.Dal); ampiezza != dueGiorni {
			t.Errorf("%s: la seconda finestra è di %v, attesi %v", casella, ampiezza, dueGiorni)
		}
	}
}

// SS3 — finché il job storico di una casella è pendente, premere ancora non ne accoda un altro: il
// worker è uno, e una catena di finestre accodate a furia di clic lo occuperebbe per settimane senza
// che nessuno l'abbia chiesto.
func TestSS3UnJobStoricoPendenteNonSiDuplica(t *testing.T) {
	b := preparaBancoWeb(t)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")

	for i := 0; i < 5; i++ {
		if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
			t.Fatalf("clic %d: %d %s", i+1, resp.StatusCode, corpo)
		}
	}
	var n int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM job WHERE tipo = 'sync_outlook' AND chiave_idempotenza LIKE 'sync_storico:%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("cinque clic hanno accodato %d job storici, atteso uno per casella attiva (3)", n)
	}
}

// SS4 — i job di chi sta guardando la schermata passano davanti all'archivio.
//
// È la metà che rende sopportabili le finestre piccole: farle piccole serve a riportare il worker al
// claim, ma se al claim l'archivio avesse la precedenza il pulsante resterebbe muto lo stesso.
//
// Il caso che discrimina è «segna letto», non «Apri in Outlook»: con la vecchia priorità il sync
// storico stava a 2, cioè ALLA PARI con «segna letto», e a parità vince il job più vecchio — che è
// sempre l'archivio, perché è in coda da prima. «Apri» (priorità 1) passava avanti anche allora: un
// test con il solo «Apri» resterebbe verde con il difetto rimesso.
func TestSS4IJobInterattiviPassanoDavantiAllArchivio(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	msg := b.messaggioIn("<ss4@acme.example>", b.francesco)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")

	// prima l'archivio: è in coda da prima di tutto il resto
	if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
		t.Fatalf("carica precedenti: %d %s", resp.StatusCode, corpo)
	}
	// poi l'operatore, che è davanti allo schermo e aspetta
	if resp, corpo := chi.fai(http.MethodPost, "/messaggio/"+msg.String()+"/apri", url.Values{}, true); resp.StatusCode != 200 {
		t.Fatalf("apri in Outlook: %d %s", resp.StatusCode, corpo)
	}
	if resp, corpo := chi.fai(http.MethodPost, "/messaggio/"+msg.String()+"/letto", url.Values{"letto": {"1"}}, true); resp.StatusCode != 200 {
		t.Fatalf("segna letto: %d %s", resp.StatusCode, corpo)
	}

	atteso := []string{
		string(db.TipoJobApriElementoOutlook), // priorità 1
		string(db.TipoJobSegnaLetto),          // priorità 2
		string(db.TipoJobSyncOutlook),         // l'archivio, in fondo
	}
	for i, vuole := range atteso {
		j := b.claimaUnJob("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
		if j == nil {
			t.Fatalf("claim %d: nessun job, atteso %s", i+1, vuole)
		}
		if j.Tipo != vuole {
			t.Fatalf("claim %d: il worker ha ricevuto %q, atteso %q (ordine finora: %v)", i+1, j.Tipo, vuole, atteso[:i])
		}
	}
}
