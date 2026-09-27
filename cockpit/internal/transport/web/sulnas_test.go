package web

// L1 — Smistamento F6 (A5.4.8, U2): la barra «Sul NAS». La materializzazione dei documenti decisi si
// mostra in testata (Fascicolo e pagina della RFQ) e in fondo al Fascicolo, con le copie in coda, gli
// errori, le anomalie e «Riprova copie»; nel dialogo del congelamento e' il terzo elenco, che non ferma.

import (
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// Prova 198, parte schermata: la barra legge gli stessi numeri del gate, li dice senza percentuali e
// non li mette fra i motivi per cui non si congela.
func TestLaBarraSulNasDiceLaMaterializzazione(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	// come caricaFascicolo con la working libera: la barra e' la materializzazione del gate
	imposta := func(m fascicolo.Materializzazione) {
		s.d.Gate = fascicolo.Valuta(fascicolo.Conti{Nas: m}, nil, nil, nil)
		s.d.SulNas = s.d.Gate.Nas
	}
	imposta(fascicolo.Materializzazione{Documenti: 11, Scritti: 9, InCoda: 1, Errore: 1, FraMinuti: 7})
	html := rendiFascicolo(t, "fasc_corpo", s.d)
	tid := s.d.T.ThreadID.String()

	haTesto(t, "testata", html, `<span class="chip urg" id="sul-nas"`, ">Sul NAS: 9 di 11</span>")
	haTesto(t, "barra", html, `id="sul-nas-barra"`, "<b>Sul NAS: 9 di 11</b> scritti",
		"1 documento in coda per la copia sul NAS (la prima fra 7 min)", "1 documento in errore sul NAS",
		`hx-post="/thread/`+tid+`/riprova-copie"`, ">Riprova copie</button>", "la copia non ferma il congelamento")
	haTesto(t, "dialogo del congelamento", html, "Il gate è verde", `id="gate-materializzazione"`, "Sul NAS, non ferma il congelamento")
	if m := regexp.MustCompile(`(?s)id="sul-nas-barra".*?</div>`).FindString(html); strings.Contains(m, "%") {
		t.Errorf("la barra ha una percentuale: %s", m)
	}
	// il terzo elenco non e' fra i problemi: niente «Non si congela ancora» per il NAS
	senzaTesto(t, "dialogo del congelamento", html, "Non si congela ancora", `<ul class="gate"><li>1 documento`)

	// chi consulta vede la barra, non il gesto
	s.d.Scrive = false
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "consultazione", html, `id="sul-nas-barra"`, "1 documento in errore sul NAS")
	senzaTesto(t, "consultazione", html, "riprova-copie")

	// tutto scritto: la voce e' verde, nessuna frase, nessun gesto
	s.d.Scrive = true
	imposta(fascicolo.Materializzazione{Documenti: 4, Scritti: 4})
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "tutto scritto", html, `<span class="chip ok" id="sul-nas"`, "<b>Sul NAS: 4 di 4</b> scritti")
	senzaTesto(t, "tutto scritto", html, "riprova-copie", "in coda per la copia", `id="gate-materializzazione"`)

	// solo copie in coda: non e' un guasto
	imposta(fascicolo.Materializzazione{Documenti: 4, Scritti: 3, InCoda: 1})
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "in coda", html, `<span class="chip coda" id="sul-nas"`, `<span class="nas-conta info"><b>Sul NAS: 3 di 4</b>`)

	// nessun documento deciso: niente da dire
	imposta(fascicolo.Materializzazione{})
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	senzaTesto(t, "senza documenti", html, `id="sul-nas"`, `id="sul-nas-barra"`)

	// la pagina della RFQ: la voce in testata, con le frasi
	thd, _ := pannelloSintetico(0)
	thd.SulNas = fascicolo.Materializzazione{Documenti: 3, Scritti: 1, InCoda: 1, Errore: 1}
	pagina := rendiPannello(t, thd)
	haTesto(t, "pagina della RFQ", pagina, `<b id="sul-nas"`, ">Sul NAS: 1 di 3</b>",
		"(1 documento in coda per la copia sul NAS; 1 documento in errore sul NAS)")
	thd.SulNas = fascicolo.Materializzazione{}
	senzaTesto(t, "pagina della RFQ senza documenti", rendiPannello(t, thd), `id="sul-nas"`)
}
