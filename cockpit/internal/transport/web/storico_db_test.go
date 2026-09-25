//go:build integrazione

// L4 — SS1–SS4: «Carica precedenti» scarica l'archivio a pezzi piccoli (voce 2.8, checkpoint del
// 16/09/2026).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/transport/web/
//
// Il motivo di tutto il gruppo è la forma del worker: ce n'è uno per PC ed è seriale. Finché macina
// un job storico non prende «Apri in Outlook», non scarica un allegato e non fa il sync ordinario.
// Un job da trenta giorni su una casella viva era il Cockpit che smetteva di rispondere per un tempo
// che nessuno sapeva dire in anticipo — e chi guardava la schermata non vedeva un lavoro in corso,
// vedeva dei pulsanti che non facevano niente.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// jobStorici è l'ULTIMO sync storico accodato per ogni casella. Storico = con un limite superiore:
// è quello a distinguerlo dal sync ordinario, sia nel payload sia nel comportamento (per lo storico
// il cursore in avanti non si muove).
//
// L'ultimo, e non «uno qualsiasi», perché i job dei clic precedenti restano in tabella: prendere
// quello sbagliato farebbe passare SS2 leggendo due volte la stessa finestra.
func (b *bancoWeb) jobStorici() map[uuid.UUID]worker.PayloadSyncOutlook {
	b.t.Helper()
	ultimo := map[uuid.UUID]int64{}
	out := map[uuid.UUID]worker.PayloadSyncOutlook{}
	for _, j := range b.jobInterattivi() { // ListJob: job_id decrescente
		if j.Tipo != db.TipoJobSyncOutlook {
			continue
		}
		var p worker.PayloadSyncOutlook
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
//
// Scrive in database invece di passare dalla porta perché qui interessa il CALCOLO della finestra
// successiva, non chi la scrive. Che sia il server a decidere se scriverla — e che non la scriva su
// una finestra interrotta — lo prova SS5, che passa da claim e result veri.
func (b *bancoWeb) finiscono() {
	b.t.Helper()
	for _, j := range b.jobInterattivi() {
		var p worker.PayloadSyncOutlook
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

// riportaJob manda un risultato dalla porta vera del worker, con il tentativo che il claim ha
// consegnato. È il percorso completo — claim, lease, result, applicaRisultato — e serve dove la
// scrittura diretta in database non proverebbe niente: la decisione su una frontiera la prende il
// server guardando quel risultato.
func (b *bancoWeb) riportaJob(nome string, j *worker.Job, cartelle []worker.CartellaEsito) {
	b.t.Helper()
	dati, err := json.Marshal(worker.RisultatoSync{Cartelle: cartelle})
	if err != nil {
		b.t.Fatal(err)
	}
	corpo, _ := json.Marshal(worker.RisultatoRichiesta{Esito: "ok", Dati: dati, WorkerID: nome, LeaseToken: j.LeaseToken})
	r, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/jobs/%d/result", b.srv.URL, j.JobID), strings.NewReader(string(corpo)))
	r.Header.Set("X-Cockpit-Token", tokenDelWorker(nome))
	r.Header.Set("X-Prova-IP", "10.0.0.5:4000")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b.t.Fatalf("result del job %d: %d", j.JobID, resp.StatusCode)
	}
}

// cartelleDel è l'elenco delle cartelle di un payload di sync, con l'esito che si vuole dichiarare.
func cartelleDel(p worker.PayloadSyncOutlook, completa bool, errore string) []worker.CartellaEsito {
	out := make([]worker.CartellaEsito, 0, len(p.Cartelle))
	for _, c := range p.Cartelle {
		out = append(out, worker.CartellaEsito{Cartella: c.Cartella, NMessaggi: 2, Completa: completa, Errore: errore})
	}
	return out
}

// G — blocco 3 del 3R: uno storico interrotto a metà non sposta il limite storico.
//
// È il caso che il revisore ha chiesto di aggiungere qui, e passa tutto dalla porta: clic, claim,
// result. Il job HTTP finisce bene — `esito: ok` — e la cartella non riporta nemmeno un errore: solo
// non dichiara di aver percorso la finestra. Se il limite storico avanzasse lo stesso, quei due
// giorni resterebbero segnati come coperti e il clic successivo partirebbe da sotto, saltandoli.
//
// La condizione precedente era `c.Errore == ""`, cioè l'assenza di un guasto: qui il guasto non c'è,
// e la finestra non è stata letta lo stesso.
func TestSS5UnoStoricoInterrottoNonAvanzaIlLimiteStorico(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")
	clic := func() map[uuid.UUID]worker.PayloadSyncOutlook {
		t.Helper()
		if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
			t.Fatalf("carica precedenti: %d %s", resp.StatusCode, corpo)
		}
		return b.jobStorici()
	}

	prima := clic()
	if len(prima) == 0 {
		t.Fatal("nessun job storico accodato")
	}
	// il worker prende i job e ne riporta uno per volta, dichiarando le finestre NON percorse
	for i := 0; i < len(prima)+2; i++ {
		j := b.claimaUnJob("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
		if j == nil {
			break
		}
		if j.Tipo != string(db.TipoJobSyncOutlook) {
			continue
		}
		var p worker.PayloadSyncOutlook
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			t.Fatal(err)
		}
		b.riportaJob("outlook@PC-FRANCESCO", j, cartelleDel(p, false, ""))
	}

	// nessuna cartella può essersi dichiarata coperta
	righe, err := b.q.ListSyncCursori(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range righe {
		if r.StoricoFinoA != nil {
			t.Errorf("%s/%s: limite storico a %v dopo una finestra mai percorsa: quei due giorni "+
				"risultano coperti e il clic successivo li salterà", r.CasellaIndirizzo, r.Cartella, r.StoricoFinoA)
		}
		if r.CopertoFinoA != nil {
			t.Errorf("%s/%s: uno storico ha mosso la copertura RECENTE a %v: sono due frontiere diverse",
				r.CasellaIndirizzo, r.Cartella, r.CopertoFinoA)
		}
	}

	// e il clic successivo ripropone la STESSA finestra, non quella di due giorni più in giù
	dopo := clic()
	for casella, seconda := range dopo {
		primaFinestra, ok := prima[casella]
		if !ok {
			continue
		}
		// Se il limite storico fosse avanzato, la seconda finestra finirebbe esattamente dove
		// cominciava la prima: e' la forma di SS2, cioe' di un clic che prosegue. Qui deve invece
		// ricalcolare la stessa finestra.
		if d := seconda.Al.Sub(primaFinestra.Dal); d > -time.Hour && d < time.Hour {
			t.Errorf("%s: la seconda finestra finisce a %v, cioe' dove cominciava la prima (%v): il "+
				"limite storico e' avanzato su due giorni mai letti", casella, *seconda.Al, primaFinestra.Dal)
		}
		// e' la stessa finestra, ricalcolata: il ripiego e' «adesso» e fra i due clic passa un attimo
		if d := seconda.Dal.Sub(primaFinestra.Dal); d > time.Minute || d < -time.Minute {
			t.Errorf("%s: dopo l'interruzione il clic riparte da %v invece che da %v: i due giorni "+
				"non letti sono stati saltati", casella, seconda.Dal, primaFinestra.Dal)
		}
	}
}

// SS6 — J del blocco 3, dal lato dello storico: ogni (casella, cartella) ha il SUO limite storico,
// e la finestra di un clic e' la sua, non quella della vicina.
//
// IL DIFETTO, che stava qui dal principio: `finestraStorico` prendeva il piu' VECCHIO degli
// `storico_fino_a` della casella e applicava quella finestra a tutte le cartelle. Due cartelle che
// scendono a velocita' diverse bastano a produrne il danno — succede appena una fallisce un giro, o
// appena se ne aggiunge una a una casella gia' in archivio. La cartella rimasta piu' avanti riceveva
// una finestra che non arrivava a toccare il proprio limite, e a fine job si scriveva lo stesso il
// nuovo `storico_fino_a`: l'intervallo fra il proprio limite e quello della vicina risultava coperto
// senza che nessuno l'avesse mai letto. Nessun errore, nessun avviso, e nessun modo di accorgersene
// dopo.
func TestSS6OgniCartellaScendeConLaSuaFinestra(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")

	// la Posta in arrivo e' gia' scesa di una settimana, la Posta inviata solo di un giorno
	inbox := time.Now().AddDate(0, 0, -7).UTC().Truncate(time.Second)
	inviata := time.Now().AddDate(0, 0, -1).UTC().Truncate(time.Second)
	for cartella, fin := range map[string]time.Time{"Inbox": inbox, "Sent Items": inviata} {
		q := fin
		if err := b.q.SetStoricoFinoA(b.ctx, db.SetStoricoFinoAParams{
			CasellaID: b.francesco, Cartella: cartella, StoricoFinoA: &q,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
		t.Fatalf("carica precedenti: %d %s", resp.StatusCode, corpo)
	}
	p, ok := b.jobStorici()[b.francesco]
	if !ok {
		t.Fatal("nessun job storico per la casella con i due limiti")
	}
	atteso := map[string]time.Time{"Inbox": inbox, "Sent Items": inviata}
	visti := map[string]bool{}
	for _, c := range p.Cartelle {
		fin, previsto := atteso[c.Cartella]
		if !previsto {
			continue
		}
		visti[c.Cartella] = true
		if c.Al == nil || c.Dal == nil {
			t.Fatalf("%s: finestra senza estremi nel payload", c.Cartella)
		}
		if d := c.Al.Sub(fin); d > time.Millisecond || d < -time.Millisecond {
			t.Errorf("%s: la finestra finisce a %v invece che al suo limite storico %v. Con la finestra "+
				"della cartella piu' indietro, l'intervallo fra i due limiti risulterebbe coperto senza "+
				"essere stato letto", c.Cartella, *c.Al, fin)
		}
		if ampiezza := c.Al.Sub(*c.Dal); ampiezza != dueGiorni {
			t.Errorf("%s: finestra di %v, attesi %v", c.Cartella, ampiezza, dueGiorni)
		}
	}
	if len(visti) != 2 {
		t.Fatalf("le due cartelle non sono tutte e due nel payload: %v", visti)
	}

	// e a finestra conclusa ciascuna scende AL SUO passo: il nuovo limite di una cartella e' il `dal`
	// della SUA finestra. Scrivere per tutte l'inviluppo del payload — il piu' vecchio dei due `dal` —
	// farebbe scendere la cartella piu' avanti di piu' di due giorni in un colpo solo, dichiarando
	// coperto un tratto che quel job non ha letto.
	for i := 0; i < 6; i++ {
		j := b.claimaUnJob("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
		if j == nil {
			break
		}
		var q worker.PayloadSyncOutlook
		if j.Tipo != string(db.TipoJobSyncOutlook) || json.Unmarshal(j.Payload, &q) != nil {
			continue
		}
		b.riportaJob("outlook@PC-FRANCESCO", j, cartelleDel(q, true, ""))
	}
	for _, c := range p.Cartelle {
		fin, previsto := atteso[c.Cartella]
		if !previsto || c.Dal == nil {
			continue
		}
		cur, err := b.q.GetSyncCursore(b.ctx, db.GetSyncCursoreParams{CasellaID: b.francesco, Cartella: c.Cartella})
		if err != nil {
			t.Fatalf("%s: %v", c.Cartella, err)
		}
		if cur.StoricoFinoA == nil {
			t.Fatalf("%s: finestra conclusa e limite storico non scritto", c.Cartella)
		}
		if d := cur.StoricoFinoA.Sub(*c.Dal); d > time.Millisecond || d < -time.Millisecond {
			t.Errorf("%s: il nuovo limite storico e' %v invece del `dal` della sua finestra (%v). "+
				"Era %v: e' sceso di %v invece dei %v letti", c.Cartella, *cur.StoricoFinoA, *c.Dal,
				fin, fin.Sub(*cur.StoricoFinoA), dueGiorni)
		}
	}
}

// claimaUnJob fa un claim vero, dalla porta, e restituisce il job che il server consegna (nil se non
// ne consegna nessuno). Serve a SS4: quale job il worker riceve non si deduce dalle priorità scritte
// nel codice, si guarda.
func (b *bancoWeb) claimaUnJob(nome, ip string, caselle ...uuid.UUID) *worker.Job {
	b.t.Helper()
	aperte := make([]worker.CasellaAperta, 0, len(caselle))
	for _, c := range caselle {
		aperte = append(aperte, worker.CasellaAperta{CasellaID: c, StoreID: "STORE-" + c.String()[:8]})
	}
	corpo, _ := json.Marshal(worker.ClaimRichiesta{Worker: "outlook", WorkerID: nome, AttesaS: 1, OutlookOk: true, CaselleAperte: aperte})
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
	var j worker.Job
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
	carica := func() map[uuid.UUID]worker.PayloadSyncOutlook {
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

// copiaIn mette in una casella la copia di un messaggio, nella cartella e con la data di ricezione date:
// e' l'archivio che «Carica precedenti» guarda per sapere da dove cominciare.
func (b *bancoWeb) copiaIn(casella uuid.UUID, cartella string, ricevuto time.Time) {
	b.t.Helper()
	nCopie++
	chiave := fmt.Sprintf("<ss7-%d@acme.example>", nCopie)
	msg := b.messaggioIn(chiave)
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, cartella, ricevuto_il)
		VALUES ($1, $2, $3, $4, $5)`, msg, casella, "ENTRY-SS7-"+fmt.Sprint(nCopie), cartella, ricevuto); err != nil {
		b.t.Fatal(err)
	}
}

var nCopie int

// SS7 — una cartella senza un suo limite storico riparte dalla SUA mail piu' vecchia, mai dal limite di
// un'altra cartella (revisione del 25/09).
//
// IL DIFETTO: la cartella senza `storico_fino_a` prendeva il piu' vecchio dei limiti delle ALTRE. Primo
// clic: la Posta in arrivo si conclude, la Posta inviata no. Secondo clic: la Posta inviata riceve la
// finestra sotto il limite della Posta in arrivo, il tratto fra la sua mail piu' vecchia e quel limite
// non si legge mai, e a fine job risulta coperto. Qui la Posta in arrivo e' scesa di dieci giorni, la
// Posta inviata ha la sua mail piu' vecchia di tre giorni fa (con il nome che Outlook le da', «Posta
// inviata»), e una terza cartella non ha ne' limite ne' posta.
func TestSS7UnaCartellaSenzaLimiteRipartedallaSuaMailPiuVecchia(t *testing.T) {
	b := preparaBancoWeb(t)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")

	inbox := time.Now().AddDate(0, 0, -10).UTC().Truncate(time.Second)
	if err := b.q.SetStoricoFinoA(b.ctx, db.SetStoricoFinoAParams{CasellaID: b.francesco, Cartella: "Inbox", StoricoFinoA: &inbox}); err != nil {
		t.Fatal(err)
	}
	// le altre due cartelle hanno il loro cursore (il sync ordinario le ha viste) ma nessun limite storico
	adesso := time.Now().UTC().Truncate(time.Second)
	for _, cartella := range []string{"Sent Items", `Commerciale\Ordini`} {
		if err := b.q.SetCopertoFinoA(b.ctx, db.SetCopertoFinoAParams{CasellaID: b.francesco, Cartella: cartella, CopertoFinoA: &adesso}); err != nil {
			t.Fatal(err)
		}
	}
	inviata := time.Now().AddDate(0, 0, -3).UTC().Truncate(time.Second)
	b.copiaIn(b.francesco, "Posta inviata", inviata)
	b.copiaIn(b.francesco, "Posta inviata", inviata.Add(24*time.Hour))
	// la Posta in arrivo ha copie vecchie, portate giu' dalle sue finestre storiche: non contano per le altre
	b.copiaIn(b.francesco, "Posta in arrivo", inbox.Add(time.Hour))
	// e un'altra casella ha posta ancora piu' vecchia nella sua Posta inviata: non conta per questa
	b.copiaIn(b.luigi, "Posta inviata", inbox.AddDate(0, 0, -30))

	if resp, corpo := chi.fai(http.MethodPost, "/inbox/sync-storico", url.Values{}, true); resp.StatusCode != 200 {
		t.Fatalf("carica precedenti: %d %s", resp.StatusCode, corpo)
	}
	p, ok := b.jobStorici()[b.francesco]
	if !ok {
		t.Fatal("nessun job storico per la casella")
	}
	fine := map[string]time.Time{}
	for _, c := range p.Cartelle {
		if c.Al == nil || c.Dal == nil {
			t.Fatalf("%s: finestra senza estremi", c.Cartella)
		}
		fine[c.Cartella] = *c.Al
		if ampiezza := c.Al.Sub(*c.Dal); ampiezza != dueGiorni {
			t.Errorf("%s: finestra di %v, attesi %v", c.Cartella, ampiezza, dueGiorni)
		}
	}
	vicino := func(a, b time.Time) bool { d := a.Sub(b); return d < time.Millisecond && d > -time.Millisecond }
	if f, ok := fine["Inbox"]; !ok || !vicino(f, inbox) {
		t.Errorf("Inbox: la finestra finisce a %v, non al suo limite %v", f, inbox)
	}
	if f, ok := fine["Sent Items"]; !ok || !vicino(f, inviata) {
		t.Errorf("Sent Items: la finestra finisce a %v invece che alla sua mail piu' vecchia (%v). Sotto il limite della "+
			"Posta in arrivo (%v) il tratto fra le due non si leggerebbe mai, e risulterebbe coperto", f, inviata, inbox)
	}
	if f, ok := fine[`Commerciale\Ordini`]; !ok || time.Since(f) > time.Minute {
		t.Errorf("una cartella senza limite e senza posta riparte da adesso, non dal limite di un'altra: %v", f)
	}
}
