//go:build integrazione

// L4 — voce 6.9: l'interfaccia operativa e quella amministrativa sono due cose diverse, e la
// differenza la fa il RUOLO, non la sigla e non l'ufficio.
//
// Le rotte si chiamano attraverso il mux vero, montato come in produzione (`Registra`) e con la
// protezione CSRF sopra: un controllo che esistesse solo nei gestori chiamati a mano non direbbe
// niente su ciò che risponde alla porta.
package web

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/testutil"
)

// rotteAdmin sono le sette che l'addendum chiede di chiudere: le tre schermate e le quattro azioni
// che cambiano qualcosa. L'elenco sta qui in una forma sola perché ogni caso lo attraversa tutto:
// aggiungere una rotta /admin/ senza aggiungerla qui è l'unico modo di lasciarla scoperta, e si
// vede subito confrontando questo elenco con `Registra`.
func rotteAdmin(job, scarto int64) []struct{ metodo, percorso string } {
	j, s := strconv.FormatInt(job, 10), strconv.FormatInt(scarto, 10)
	return []struct{ metodo, percorso string }{
		{http.MethodGet, "/admin/job"},
		{http.MethodGet, "/admin/scarti"},
		{http.MethodGet, "/admin/postazioni"},
		{http.MethodPost, "/admin/job/" + j + "/riaccoda"},
		{http.MethodPost, "/admin/job/" + j + "/annulla"},
		{http.MethodPost, "/admin/scarti/" + s + "/riprova"},
		{http.MethodPost, "/admin/postazioni/PC-FRANCESCO/pacchetto"},
	}
}

// unJob accoda lavoro vero come lo accoda l'operatore («Aggiorna ora») e restituisce l'id del primo.
func (b *bancoWeb) unJob(chi *browser) int64 {
	b.t.Helper()
	if resp, corpo := chi.fai(http.MethodPost, "/inbox/aggiorna", url.Values{}, true); resp.StatusCode != 200 {
		b.t.Fatalf("aggiorna ora: %d %s", resp.StatusCode, corpo)
	}
	righe := b.jobInterattivi()
	if len(righe) == 0 {
		b.t.Fatal("«Aggiorna ora» non ha accodato niente: il resto del test non proverebbe nulla")
	}
	return righe[0].JobID
}

func (b *bancoWeb) statoJob(id int64) string {
	b.t.Helper()
	var stato string
	if err := b.pool.QueryRow(b.ctx, "SELECT stato FROM job WHERE job_id = $1", id).Scan(&stato); err != nil {
		b.t.Fatal(err)
	}
	return stato
}

// unScarto mette in tabella un elemento rifiutato dall'ingest, per avere un id vero su cui provare
// «Riprova» (il contenuto non conta: conta che la riga esista e resti com'è).
func (b *bancoWeb) unScarto() int64 {
	b.t.Helper()
	var id int64
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO ingest_scarto (casella_id, entry_id, cartella, origine, payload, errore)
		VALUES ($1, 'ENTRY-RBAC', 'Posta in arrivo', 'ingest', '{}'::jsonb, 'ricevuto_il nel futuro')
		RETURNING scarto_id`, b.francesco).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	return id
}

// W1 — un operatore non entra nelle schermate tecniche, nemmeno scrivendo l'indirizzo a mano, e non
// le usa nemmeno in POST. Con il controllo solo sul menu, tutte queste righe sarebbero verdi sul
// browser e rosse su `curl`.
func TestW1UnOperatoreNonEntraInAdmin(t *testing.T) {
	b := preparaBancoWeb(t)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp") // operatore, e proprietario di PC-FRANCESCO: il PC è suo, le credenziali no
	job, scarto := b.unJob(fp), b.unScarto()

	for _, rt := range rotteAdmin(job, scarto) {
		var form url.Values
		if rt.metodo == http.MethodPost {
			form = url.Values{}
		}
		resp, corpo := fp.fai(rt.metodo, rt.percorso, form, true)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: %d, atteso 403", rt.metodo, rt.percorso, resp.StatusCode)
			continue
		}
		// HTMX innesta la risposta nella pagina: deve essere un frammento, non una riga di testo.
		if !strings.Contains(corpo, "Non autorizzato") || !strings.Contains(corpo, "<div") {
			t.Errorf("%s %s: il 403 non è un frammento leggibile: %q", rt.metodo, rt.percorso, corpo)
		}
	}

	// L'indirizzo scritto a mano nella barra: nessun header HTMX, quindi la risposta è una pagina
	// intera — con la testata, il motivo e il modo di tornare indietro. Un 403 che in quel caso
	// rispondesse un frammento lascerebbe una riga di HTML sospesa in una finestra bianca.
	resp, corpo := fp.fai(http.MethodGet, "/admin/job", nil, false)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("/admin/job scritto nella barra degli indirizzi: %d", resp.StatusCode)
	}
	for _, atteso := range []string{"<html", "Non autorizzato", "admin", `href="/inbox"`} {
		if !strings.Contains(corpo, atteso) {
			t.Errorf("la pagina del 403 non contiene %q", atteso)
		}
	}
	if strings.Contains(corpo, `href="/admin/scarti"`) {
		t.Error("la pagina del 403 offre comunque le altre voci tecniche")
	}

	// E il 403 è arrivato PRIMA di fare qualunque cosa: il job che l'operatore ha accodato è ancora
	// lì, com'era. Un controllo messo dopo l'effetto sarebbe un controllo che non serve a niente.
	if s := b.statoJob(job); s != "pronto" {
		t.Errorf("il job è stato toccato da un operatore: stato %q", s)
	}
	var tentativi int
	if err := b.pool.QueryRow(b.ctx, "SELECT tentativi FROM ingest_scarto WHERE scarto_id = $1", scarto).Scan(&tentativi); err != nil || tentativi != 1 {
		t.Errorf("lo scarto è stato ripreso da un operatore: tentativi=%d err=%v", tentativi, err)
	}

	// L'interfaccia di lavoro invece è la sua, e resta intera.
	for _, percorso := range []string{"/inbox", "/richieste", "/stato/worker"} {
		if resp, _ := fp.fai(http.MethodGet, percorso, nil, false); resp.StatusCode != 200 {
			t.Errorf("%s: %d per un operatore", percorso, resp.StatusCode)
		}
	}
	if resp, corpo := fp.fai(http.MethodPost, "/inbox/aggiorna", url.Values{}, true); resp.StatusCode != 200 {
		t.Errorf("«Aggiorna ora» per un operatore: %d %s", resp.StatusCode, corpo)
	}
}

// L'altra metà di W1: l'admin le apre tutte. Senza questa, «403 a tutti» passerebbe il test di sopra
// e romperebbe il Cockpit.
func TestUnAdminUsaTutteLeSchermateTecniche(t *testing.T) {
	b := preparaBancoWeb(t)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	job, scarto := b.unJob(fp), b.unScarto()

	ad := b.browser("10.0.0.77:51000")
	ad.login("AD", "prova-ad")
	for _, rt := range rotteAdmin(job, scarto) {
		var form url.Values
		if rt.metodo == http.MethodPost {
			form = url.Values{}
		}
		resp, corpo := ad.fai(rt.metodo, rt.percorso, form, true)
		if resp.StatusCode == http.StatusForbidden {
			t.Errorf("%s %s: 403 a un amministratore: %s", rt.metodo, rt.percorso, corpo)
		}
	}
	// E l'azione è successa davvero: «Annulla» su un job pronto lo annulla. (È l'ultima delle due
	// sul job, quindi lo stato finale è questo.)
	if s := b.statoJob(job); s != "annullato" {
		t.Errorf("l'admin ha premuto Annulla e il job è %q", s)
	}
}

// W15 — la barra di navigazione, dal vivo: le tre voci tecniche compaiono solo all'admin. La testata
// con lo stato delle caselle resta a tutti e due: serve a lavorare, non ad amministrare.
func TestW15LaTestataDalVivo(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)

	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	_, pagina := fp.fai(http.MethodGet, "/inbox", nil, false)
	for _, voce := range []string{"/admin/job", "/admin/scarti", "/admin/postazioni"} {
		if strings.Contains(pagina, `href="`+voce+`"`) {
			t.Errorf("l'operatore vede la voce %s", voce)
		}
	}
	if !strings.Contains(pagina, "Sei su:") || !strings.Contains(pagina, `href="/richieste"`) {
		t.Errorf("all'operatore manca l'interfaccia di lavoro: %s", estratto(pagina, "barra"))
	}

	ad := b.browser("10.0.0.77:51000")
	ad.login("AD", "prova-ad")
	_, pagina = ad.fai(http.MethodGet, "/inbox", nil, false)
	for _, voce := range []string{"/admin/job", "/admin/scarti", "/admin/postazioni"} {
		if !strings.Contains(pagina, `href="`+voce+`"`) {
			t.Errorf("l'amministratore non vede la voce %s", voce)
		}
	}
}

// W16 — `consultazione` guarda e non tocca: ogni POST è 403, in un punto solo e per tutte le rotte,
// comprese quelle che nel blocco 3 non sono ancora state scritte.
func TestW16ConsultazioneELaSolaLettura(t *testing.T) {
	b := preparaBancoWeb(t)
	co := b.browser("10.0.0.5:51000")
	co.login("CO", "prova-co")

	for _, percorso := range []string{"/inbox", "/richieste", "/stato/worker"} {
		if resp, _ := co.fai(http.MethodGet, percorso, nil, false); resp.StatusCode != 200 {
			t.Errorf("%s: %d per chi consulta", percorso, resp.StatusCode)
		}
	}
	msg := b.messaggioIn("<w16@acme.example>", b.francesco)
	for _, percorso := range []string{"/inbox/aggiorna", "/messaggio/" + msg.String() + "/apri", "/sessione/postazione"} {
		resp, corpo := co.fai(http.MethodPost, percorso, url.Values{}, true)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("POST %s: %d per chi consulta, atteso 403", percorso, resp.StatusCode)
		} else if !strings.Contains(corpo, "sola lettura") && !strings.Contains(corpo, "non lo cambia") {
			t.Errorf("POST %s: il motivo non si legge: %q", percorso, corpo)
		}
	}
	if n := len(b.jobInterattivi()); n != 0 {
		t.Errorf("chi consulta ha accodato %d job", n)
	}
	// Ma può uscire: /logout non sta dietro all'autenticazione, e una sessione che non si chiude
	// sarebbe un modo curioso di proteggere qualcosa.
	if resp, _ := co.fai(http.MethodPost, "/logout", url.Values{}, false); resp.StatusCode != 302 {
		t.Errorf("logout di chi consulta: %d", resp.StatusCode)
	}
}

// W12 — la password del TOML serve al primo avvio. Dopo, il segreto è in database e nessun riavvio
// lo riscrive: altrimenti il cambio password (voce 6.4) non potrebbe esistere, perché ogni notte il
// file rimetterebbe quella vecchia.
func TestW12LaPasswordSopravviveAlRiavvio(t *testing.T) {
	b := preparaBancoWeb(t)

	// riavvio con una password DIVERSA nel file, e un utente nuovo che invece nasce adesso
	var log bytes.Buffer
	utenti := []struct{ Sigla, Nome, Ufficio, Ruolo, Password string }{
		{"FP", "Francesco", "Commerciale", "operatore", "quella-nuova-del-file"},
		{"NU", "Nuovo", "Commerciale", "operatore", "prova-nu"},
	}
	if err := SeedUtenti(context.Background(), b.q, utenti, slog.New(slog.NewTextHandler(&log, nil))); err != nil {
		t.Fatal(err)
	}

	vecchia := b.browser("10.0.0.5:51000")
	vecchia.login("FP", "prova-fp") // fallisce il test se non entra

	nuova := b.browser("10.0.0.5:51000")
	resp, _ := nuova.fai(http.MethodPost, "/login", url.Values{"sigla": {"FP"}, "password": {"quella-nuova-del-file"}}, false)
	if resp.Header.Get("Location") != "/login?errore=credenziali" {
		t.Errorf("la password scritta nel file ha sostituito quella in database: %s", resp.Header.Get("Location"))
	}
	// e chi ci ha provato lo legge nel log, invece di restare a chiedersi perché non entra
	if !strings.Contains(log.String(), "FP") || !strings.Contains(log.String(), "cockpit.toml") {
		t.Errorf("il seed non dice che la password del file non è stata usata: %s", log.String())
	}

	// l'utente nuovo invece nasce con la sua: il bootstrap continua a funzionare
	b.browser("10.0.0.5:51000").login("NU", "prova-nu")

	// e il resto della riga sì: nome, ufficio e RUOLO restano allineati al file
	u, err := b.q.GetUtentePerSigla(b.ctx, "FP")
	if err != nil || u.Ufficio != "Commerciale" || u.Ruolo != db.RuoloUtenteOperatore {
		t.Errorf("il seed non ha allineato il resto: %+v %v", u, err)
	}
}

// Il seed non trasforma più un ruolo sconosciuto in `operatore`: si ferma. È la stessa regola di
// CF1, sull'altro lato — `Carica` protegge il file, questa protegge chiunque chiami il seed.
func TestIlSeedRifiutaUnRuoloSconosciuto(t *testing.T) {
	b := preparaBancoWeb(t)
	err := SeedUtenti(context.Background(), b.q, []struct{ Sigla, Nome, Ufficio, Ruolo, Password string }{
		{"ZZ", "Zeta", "IT", "amministratore", "x"},
	}, testutil.LogSilenzioso())
	if err == nil || !strings.Contains(err.Error(), "ruolo") {
		t.Fatalf("un ruolo inventato è passato: %v", err)
	}
	if _, err := b.q.GetUtentePerSigla(b.ctx, "ZZ"); err == nil {
		t.Error("l'utente con il ruolo inventato è stato creato lo stesso")
	}
}
