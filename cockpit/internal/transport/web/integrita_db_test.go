//go:build integrazione

// L4 — blocco 5B: la schermata «Integrità NAS» e le sue due azioni, contro un server HTTP vero.
//
// La schermata deve saper dire tre cose diverse che a prima vista si somigliano: «ho guardato e va
// tutto bene», «ho guardato e c'è questo», «non ho mai guardato». Una tabella vuota, da sola, le
// confonde tutte e tre — ed è il modo più facile per costruire un controllo che rassicura senza
// controllare niente.
package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/testutil"
)

const contenutoNas = "il disegno vero"

// rfqSulNasFinto prepara una RFQ con un documento confermato, un NAS finto (una cartella
// temporanea) e il ricognitore montato sul server web. `contenuto` vuoto = il file non c'è.
func (b *bancoWeb) rfqSulNasFinto(contenuto string, stato db.StatoNas) (uuid.UUID, uuid.UUID, string) {
	b.t.Helper()
	radice := b.t.TempDir()
	sorgente := filepath.Join(b.t.TempDir(), "disegno.pdf")
	if err := os.WriteFile(sorgente, []byte(contenutoNas), 0o644); err != nil {
		b.t.Fatal(err)
	}
	sha, _, err := nas.Sha256File(sorgente)
	if err != nil {
		b.t.Fatal(err)
	}

	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	var thread, utente, doc uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook', now(), 'RFQ integrita', 'ACME\WIP\2026 09 17 prova', 1) RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&utente); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes,
		path_relativo, stato_nas, confermato_da, confermato_il)
		VALUES ($1,'disegno_2d','disegno.pdf','pdf',$2,$3,$4,$5,$6, now() - interval '3 days') RETURNING documento_id`,
		thread, sha, len(contenutoNas), `ELENCO DISEGNI\disegno.pdf`, stato, utente).Scan(&doc); err != nil {
		b.t.Fatal(err)
	}

	if contenuto != "" {
		dst := filepath.Join(radice, "ACME", "WIP", "2026 09 17 prova", "ELENCO DISEGNI")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			b.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "disegno.pdf"), []byte(contenuto), 0o644); err != nil {
			b.t.Fatal(err)
		}
	}

	scrittore := &nas.Scrittore{Radice: radice}
	b.ws.NAS = scrittore
	b.ws.Ricognitore = &documenti.Ricognitore{Pool: b.pool, NAS: scrittore, Log: testutil.LogSilenzioso(),
		Ogni: 15 * time.Minute}
	return thread, doc, radice
}

// La schermata distingue «mai controllato» da «controllato e a posto»: sono due notizie diverse.
func TestLaSchermataDiceSeHaMaiGuardato(t *testing.T) {
	b := preparaBancoWeb(t)
	_, _, _ = b.rfqSulNasFinto(contenutoNas, db.StatoNasScritto)

	w := b.browser("10.0.0.1")
	w.login("AD", "prova-ad")

	_, pagina := w.fai(http.MethodGet, "/admin/nas", nil, true)
	if !strings.Contains(leggibile(pagina), "Mai controllato") {
		t.Errorf("senza nessuna passata la schermata non lo dice: una tabella vuota sembra un sistema sano\n%s", estrai(pagina, "controllo"))
	}

	_, pagina = w.fai(http.MethodPost, "/admin/nas/controlla", nil, true)
	if !strings.Contains(leggibile(pagina), "niente da segnalare") {
		t.Errorf("dopo il controllo non dice di aver guardato:\n%s", estrai(pagina, "avviso"))
	}
	if strings.Contains(leggibile(pagina), "Mai controllato") {
		t.Errorf("dice ancora di non aver mai controllato")
	}
}

// File assente mentre il database dice «scritto»: la riga compare, con l'azione che serve.
func TestUnFileMancanteCompareInAdminConLaSuaAzione(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	_, doc, _ := b.rfqSulNasFinto("", db.StatoNasScritto)

	w := b.browser("10.0.0.1")
	w.login("AD", "prova-ad")
	_, pagina := w.fai(http.MethodPost, "/admin/nas/controlla", nil, true)

	if !strings.Contains(pagina, "mancante") {
		t.Fatalf("il documento che dice di essere sul NAS senza esserci non compare:\n%s", estrai(pagina, "tabella"))
	}
	if !strings.Contains(pagina, "/admin/nas/"+doc.String()+"/riaccoda") {
		t.Errorf("la riga non offre nessuna azione: sarebbe una segnalazione senza uscita")
	}

	_, pagina = w.fai(http.MethodPost, "/admin/nas/"+doc.String()+"/riaccoda", nil, true)
	if !strings.Contains(pagina, "rimessa in coda") {
		t.Errorf("il riaccodamento non dice che cosa ha fatto:\n%s", estrai(pagina, "avviso"))
	}
	if n := len(b.jobDiTipo(db.TipoJobCopiaNas)); n != 1 {
		t.Errorf("copie accodate: %d, attesa 1", n)
	}
}

// Conflitto: nessuna azione, e il pulsante «riaccoda» NON deve esistere. Se qualcuno lo chiama
// ugualmente — a mano, o da una pagina vecchia — la rotta rifiuta e il file resta com'è.
func TestUnConflittoNonSiRiaccodaEIlFileNonSiTocca(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	_, doc, radice := b.rfqSulNasFinto("un file di qualcun altro", db.StatoNasScritto)

	w := b.browser("10.0.0.1")
	w.login("AD", "prova-ad")
	_, pagina := w.fai(http.MethodPost, "/admin/nas/controlla", nil, true)

	if !strings.Contains(pagina, "conflitto") {
		t.Fatalf("il file diverso non viene segnalato come conflitto:\n%s", estrai(pagina, "tabella"))
	}
	if strings.Contains(pagina, "/admin/nas/"+doc.String()+"/riaccoda") {
		t.Error("su un conflitto la schermata offre «riaccoda»: un pulsante che non può funzionare")
	}
	if !strings.Contains(leggibile(pagina), "nessuna azione automatica") {
		t.Errorf("la riga non dice perché non c'è niente da premere:\n%s", estrai(pagina, "conflitto"))
	}

	// chiamata a mano: rifiutata, e nessuna copia in coda
	_, pagina = w.fai(http.MethodPost, "/admin/nas/"+doc.String()+"/riaccoda", nil, true)
	if !strings.Contains(leggibile(pagina), "non è questo documento") {
		t.Errorf("la rotta non spiega perché rifiuta:\n%s", estrai(pagina, "avviso"))
	}
	if n := len(b.jobDiTipo(db.TipoJobCopiaNas)); n != 0 {
		t.Errorf("accodate %d copie sopra un conflitto", n)
	}
	file := filepath.Join(radice, "ACME", "WIP", "2026 09 17 prova", "ELENCO DISEGNI", "disegno.pdf")
	if c, err := os.ReadFile(file); err != nil || string(c) != "un file di qualcun altro" {
		t.Fatalf("il file di qualcun altro è stato toccato: %q (%v)", string(c), err)
	}
}

// File già corretto con il documento ancora `in_coda`: si allinea, e non si copia niente.
func TestUnFileGiaCorrettoSiAllineaSenzaCopiare(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, doc, _ := b.rfqSulNasFinto(contenutoNas, db.StatoNasInCoda)

	w := b.browser("10.0.0.1")
	w.login("AD", "prova-ad")
	_, pagina := w.fai(http.MethodPost, "/admin/nas/controlla", nil, true)

	if !strings.Contains(pagina, "gia_presente") {
		t.Fatalf("il file già al suo posto non viene riconosciuto:\n%s", estrai(pagina, "tabella"))
	}
	if !strings.Contains(pagina, "/admin/nas/"+doc.String()+"/allinea") {
		t.Fatal("la riga non offre «allinea»: il documento resterebbe in_coda per sempre")
	}

	_, pagina = w.fai(http.MethodPost, "/admin/nas/"+doc.String()+"/allinea", nil, true)
	if !strings.Contains(leggibile(pagina), "non è stato copiato niente") {
		t.Errorf("l'avviso non dice che non c'era niente da copiare:\n%s", estrai(pagina, "avviso"))
	}
	if n := len(b.jobDiTipo(db.TipoJobCopiaNas)); n != 0 {
		t.Errorf("allineare ha accodato %d copie: il file era già quello giusto", n)
	}
	var stato string
	if err := b.pool.QueryRow(b.ctx, `SELECT stato_nas::text FROM documento WHERE documento_id=$1`, doc).Scan(&stato); err != nil {
		t.Fatal(err)
	}
	if stato != "scritto" {
		t.Errorf("stato_nas = %q, atteso scritto", stato)
	}
	// e la RFQ non mostra più né il pulsante delle copie né la segnalazione
	_, rfq := w.fai(http.MethodGet, "/thread/"+thread.String(), nil, true)
	if strings.Contains(rfq, "riprova-copie") {
		t.Error("la RFQ offre ancora «Riprova copie» per un documento che è sul NAS")
	}
}

// La segnalazione arriva anche in fondo alla RFQ: chi aspetta quel disegno guarda quella pagina, non
// l'Admin.
func TestLaRfqDiceCheUnSuoDocumentoEeSegnalato(t *testing.T) {
	b := preparaBancoWeb(t)
	thread, _, _ := b.rfqSulNasFinto("", db.StatoNasScritto)

	admin := b.browser("10.0.0.1")
	admin.login("AD", "prova-ad")
	admin.fai(http.MethodPost, "/admin/nas/controlla", nil, true)

	// la notizia deve arrivare a chi lavora sulla RFQ, che non e' un amministratore
	w := b.browser("10.0.0.2")
	w.login("FP", "prova-fp")
	_, rfq := w.fai(http.MethodGet, "/thread/"+thread.String(), nil, true)
	if !strings.Contains(leggibile(rfq), "non corrisponde") {
		t.Errorf("la pagina della RFQ non dice che un suo documento è segnalato: la riga «scritto» continua "+
			"a promettere un file che non c'è\n%s", estrai(rfq, "Documenti sul NAS"))
	}
}

// Con la scrittura spenta il riaccodamento non finge: lo dice, con il nome della riga da cambiare.
func TestConLaScritturaSpentaIlRiaccodamentoLoDice(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{})
	_, doc, _ := b.rfqSulNasFinto("", db.StatoNasScritto)

	w := b.browser("10.0.0.1")
	w.login("AD", "prova-ad")
	w.fai(http.MethodPost, "/admin/nas/controlla", nil, true)
	_, pagina := w.fai(http.MethodPost, "/admin/nas/"+doc.String()+"/riaccoda", nil, true)

	if !strings.Contains(pagina, coda.CapNasScrittura) {
		t.Errorf("l'avviso non nomina la capacità spenta:\n%s", estrai(pagina, "avviso"))
	}
	if n := len(b.jobDiTipo(db.TipoJobCopiaNas)); n != 0 {
		t.Errorf("con nas_scrittura spenta sono entrate %d copie in coda", n)
	}
}

// La schermata è dell'amministratore: un operatore non la apre.
func TestIntegritaNasEeSoloDellAmministratore(t *testing.T) {
	b := preparaBancoWeb(t)
	b.rfqSulNasFinto("", db.StatoNasScritto)

	w := b.browser("10.0.0.2")
	w.login("FP", "prova-fp") // operatore
	resp, _ := w.fai(http.MethodGet, "/admin/nas", nil, true)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("un operatore apre l'Integrità NAS: HTTP %d, atteso 403", resp.StatusCode)
	}
	resp, _ = w.fai(http.MethodPost, "/admin/nas/controlla", nil, true)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("un operatore lancia il controllo: HTTP %d, atteso 403", resp.StatusCode)
	}
}
