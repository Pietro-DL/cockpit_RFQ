//go:build integrazione

// L4 — blocco 4A: la cartella del cliente si legge dall'Anagrafica, e l'anteprima segue il cliente.
//
// Il difetto era tutto in interfaccia, e per questo era insidioso: il backend la cartella la usava
// gia' bene — quando crea la RFQ prende `cliente.cartella_nas` — ma in pagina non si vedeva da
// nessuna parte, perche' il campo «Cartella NAS» compare soltanto nel riquadro «nuovo cliente», che
// per un cliente esistente e' nascosto. E l'anteprima della destinazione veniva calcolata UNA VOLTA
// all'apertura del form: cambiando cliente dalla tendina, HTMX ricaricava solo l'elenco dei buyer e
// l'anteprima restava quella di prima.
//
// Chi guardava ne ricavava che il Cockpit non sapesse dove mettere i file di quel cliente, e la
// tentazione successiva era riscrivere la cartella a mano — cioe' scavalcare l'anagrafica.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// clienteDiProva censisce un cliente con la sua cartella NAS e un dominio di posta.
func (b *bancoWeb) clienteDiProva(cartella, ragione, dominio string) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1,$2)
		RETURNING cliente_id`, cartella, ragione).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	if dominio != "" {
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO dominio_cliente (dominio, cliente_id) VALUES ($1,$2)`, dominio, id); err != nil {
			b.t.Fatal(err)
		}
	}
	return id
}

// Il form di «Nuova RFQ» dice la cartella del cliente e dove finira' la richiesta, senza che nessuno
// debba riscrivere niente.
func TestIlFormMostraLaCartellaDellAnagrafica(t *testing.T) {
	b := preparaBancoWeb(t)
	b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	msg := b.messaggioIn("<cartella-1@acme.example>", b.francesco)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	resp, html := w.fai(http.MethodGet, "/messaggio/"+msg.String()+"/triage?azione=nuova", nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("triage: HTTP %d", resp.StatusCode)
	}
	if !strings.Contains(html, `id="cartella-cliente"`) {
		t.Fatal("il form non dice qual e' la cartella NAS del cliente")
	}
	if !strings.Contains(html, "<code>ACME</code>") {
		t.Error("la cartella del cliente non compare in pagina")
	}
	if !strings.Contains(html, "Anagrafica") {
		t.Error("non si capisce che la cartella viene dall'Anagrafica e non va riscritta")
	}
	if !strings.Contains(html, `ACME\WIP\`) {
		t.Errorf("l'anteprima della destinazione non parte dalla cartella del cliente:\n%s", estrai(html, "anteprima-cartella"))
	}
	// e alla prima apertura le due righe NON sono scambi fuori bersaglio: se lo fossero, HTMX le
	// scarterebbe perche' nel DOM quegli id non ci sono ancora
	if strings.Contains(html, "hx-swap-oob") {
		t.Error("il form disegna le righe della cartella come scambi fuori bersaglio")
	}
	// La tendina deve mandare al server anche l'oggetto e il cognome: senza, il server ricalcolerebbe
	// l'anteprima con i campi vuoti e la destinazione mostrata si svuoterebbe a ogni cambio di
	// cliente. Qui si guarda l'HTML perche' e' l'unico posto in cui quella decisione e' scritta; che
	// poi il browser li mandi davvero e' L7.
	sel := estrai(html, `name="cliente_id"`)
	for _, campo := range []string{"oggetto", "buyer_cognome", "cliente_cartella"} {
		if !strings.Contains(sel, campo) {
			t.Errorf("la tendina del cliente non manda %q al server:\n%s", campo, sel)
		}
	}
}

// Cambiando cliente dalla tendina cambia tutto: buyer, cartella e destinazione.
func TestCambiareClienteCambiaLaDestinazione(t *testing.T) {
	b := preparaBancoWeb(t)
	b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	altro := b.clienteDiProva("BETA SPORT", "Beta Sport S.p.A.", "betasport.example")
	msg := b.messaggioIn("<cartella-2@acme.example>", b.francesco)

	// la mail e' di sei giorni fa: e' la SUA data a dare il nome alla cartella, non quella di oggi
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET data_evento = '2026-09-11 10:00:00+02' WHERE messaggio_id = $1`, msg); err != nil {
		t.Fatal(err)
	}

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	par := url.Values{
		"cliente_id": {altro.String()},
		"messaggio":  {msg.String()},
		"oggetto":    {"Pinze di carico"},
	}
	resp, html := w.fai(http.MethodGet, "/anagrafica/buyer?"+par.Encode(), nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("cambio cliente: HTTP %d", resp.StatusCode)
	}
	if !strings.Contains(html, `id="anteprima-cartella"`) || !strings.Contains(html, "hx-swap-oob") {
		t.Fatalf("il cambio cliente non riporta l'anteprima come scambio fuori bersaglio:\n%s", html)
	}
	if !strings.Contains(html, `BETA SPORT\WIP\`) {
		t.Errorf("l'anteprima non segue il cliente scelto:\n%s", html)
	}
	if strings.Contains(html, `ACME\WIP\`) {
		t.Error("l'anteprima mostra ancora la cartella del cliente di prima")
	}
	if !strings.Contains(html, "<code>BETA SPORT</code>") {
		t.Error("la riga «Cartella NAS» non e' stata aggiornata")
	}
	// l'oggetto digitato entra nel nome della cartella: e' quello che l'operatore sta guardando
	if !strings.Contains(html, "Pinze di carico") {
		t.Errorf("l'anteprima non tiene conto dell'oggetto digitato:\n%s", html)
	}
	if !strings.Contains(html, "2026 09 11") {
		t.Errorf("la cartella non porta la data della MAIL: una RFQ aperta oggi su una mail di settimana scorsa "+
			"si chiama con il giorno della mail\n%s", html)
	}
}

// Un campo «Cartella NAS» lasciato in pagina NON deve scavalcare l'anagrafica: sarebbe il modo di
// portare i disegni di un cliente nella cartella di un altro con un clic distratto.
func TestLaCartellaDigitataNonScavalcaLAnagrafica(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	msg := b.messaggioIn("<cartella-3@acme.example>", b.francesco)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	par := url.Values{
		"cliente_id":       {acme.String()},
		"cliente_cartella": {"CARTELLA-SBAGLIATA"},
		"messaggio":        {msg.String()},
		"oggetto":          {"Staffe"},
	}
	_, html := w.fai(http.MethodGet, "/anagrafica/buyer?"+par.Encode(), nil, true)
	if strings.Contains(html, "CARTELLA-SBAGLIATA") {
		t.Errorf("la cartella digitata ha scavalcato l'anagrafica:\n%s", html)
	}
	if !strings.Contains(html, `ACME\WIP\`) {
		t.Errorf("per un cliente censito la destinazione deve venire dall'anagrafica:\n%s", html)
	}
}

// Per un cliente NUOVO la cartella e' quella che si sta digitando, normalizzata in maiuscolo: li' il
// campo c'e' ed e' l'unica fonte possibile.
func TestPerUnClienteNuovoValeLaCartellaDigitata(t *testing.T) {
	b := preparaBancoWeb(t)
	msg := b.messaggioIn("<cartella-4@sconosciuto.example>", b.francesco)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	par := url.Values{
		"cliente_id":       {"__nuovo__"},
		"cliente_cartella": {"acme macchine"},
		"messaggio":        {msg.String()},
		"oggetto":          {"Supporto cofano"},
	}
	_, html := w.fai(http.MethodGet, "/anagrafica/buyer?"+par.Encode(), nil, true)
	if !strings.Contains(html, "<code>ACME MACCHINE</code>") {
		t.Errorf("la cartella di un cliente nuovo non e' quella digitata (in maiuscolo):\n%s", html)
	}
	if !strings.Contains(html, `ACME MACCHINE\WIP\`) {
		t.Errorf("l'anteprima non usa la cartella digitata:\n%s", html)
	}
}

// estrai ritaglia l'elemento HTML che contiene `ago`, per i messaggi d'errore e per guardare gli
// attributi di un tag intero invece di un numero di caratteri deciso a occhio.
func estrai(html, ago string) string {
	i := strings.Index(html, ago)
	if i < 0 {
		return "(" + ago + " non presente)"
	}
	fine := strings.Index(html[i:], ">")
	if fine < 0 || fine > 2000 {
		fine = 220
	}
	if i+fine+1 > len(html) {
		return html[i:]
	}
	return html[i : i+fine+1]
}
