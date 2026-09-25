//go:build integrazione

// L4 — blocco 4A: per un cliente già censito NON ci sono campi da compilare.
//
// Il passo precedente aveva reso visibile la cartella dell'Anagrafica, e quella parte funzionava. Ma
// sotto continuavano a esserci «Ragione sociale», «Cartella NAS» e «Dominio mail», vuoti e
// modificabili, subito dopo la riga che diceva «dall'Anagrafica — non va riscritta». Due frasi in
// contraddizione a tre centimetri di distanza: una dice che la cartella è decisa, l'altra sembra
// chiederla.
//
// Il riquadro aveva l'attributo `hidden`, e quell'attributo non nascondeva niente: su un elemento con
// `class="griglia3"` la regola d'autore `display:grid` batte la regola `[hidden]{display:none}` del
// browser — le regole dell'autore vincono su quelle dello user agent — e i campi restavano in pagina.
//
// La correzione non è rimetterli nascosti né disabilitarli: per un cliente censito quei campi non
// vengono proprio scritti nella pagina. Ciò che non esiste non si può riempire per sbaglio, e non
// può essere rimesso a mano da chi guarda l'HTML.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// campiDiCensimento sono i tre campi con cui si crea un cliente NUOVO. Si cercano scritti come
// l'attributo `name` di un input, non come parola libera: «cliente_cartella» compare anche
// nell'`hx-include` della tendina, dove è giusto che ci sia.
var campiDiCensimento = []string{`name="cliente_nome"`, `name="cliente_cartella"`, `name="cliente_dominio"`}

func campiPresenti(html string) []string {
	var presenti []string
	for _, c := range campiDiCensimento {
		if strings.Contains(html, c) {
			presenti = append(presenti, c)
		}
	}
	return presenti
}

// All'apertura del form su un cliente riconosciuto: la cartella si legge, e non c'è niente da
// riempire.
func TestPerUnClienteCensitoNonCiSonoCampiDaCompilare(t *testing.T) {
	b := preparaBancoWeb(t)
	// il mittente di messaggioIn e' sempre mario.rossi@acme.example: censire quel dominio e' cio' che
	// fa riconoscere il cliente all'apertura del form
	b.clienteDiProva("ACME MACCHINE", "ACME Macchine S.p.A.", "acme.example")
	msg := b.messaggioIn("<campi-1@acme.example>", b.francesco)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	resp, html := w.fai(http.MethodGet, "/messaggio/"+msg.String()+"/triage?azione=nuova", nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("triage: HTTP %d", resp.StatusCode)
	}
	if !strings.Contains(html, "<code>ACME MACCHINE</code>") {
		t.Fatal("il form non mostra la cartella dell'Anagrafica: senza quella, i campi qui sotto sembrerebbero l'unica fonte")
	}
	if p := campiPresenti(html); len(p) > 0 {
		t.Errorf("per un cliente già censito la pagina chiede ancora %v:\n%s", p, estrai(html, `id="cliente-nuovo"`))
	}
	// e il riquadro vuoto deve comunque esserci, con il suo id: è il bersaglio dello scambio quando
	// si passa a «nuovo cliente»
	if !strings.Contains(html, `id="cliente-nuovo"`) {
		t.Error("manca il riquadro «cliente-nuovo»: scegliendo «nuovo cliente» non ci sarebbe dove mettere i campi")
	}
}

// Scegliendo «➕ nuovo cliente…» i tre campi tornano, con il dominio del mittente già proposto.
func TestScegliendoNuovoClienteICampiCompaiono(t *testing.T) {
	b := preparaBancoWeb(t)
	// nessun dominio censito per il mittente: e' un cliente da censire
	b.clienteDiProva("ACME MACCHINE", "ACME Macchine S.p.A.", "")
	msg := b.messaggioIn("<campi-2@acme.example>", b.francesco)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	par := url.Values{"cliente_id": {"__nuovo__"}, "messaggio": {msg.String()}, "oggetto": {"Flange"}}
	resp, html := w.fai(http.MethodGet, "/anagrafica/buyer?"+par.Encode(), nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("cambio cliente: HTTP %d", resp.StatusCode)
	}
	if p := campiPresenti(html); len(p) != len(campiDiCensimento) {
		t.Fatalf("per un cliente nuovo la pagina offre solo %v: senza questi campi il cliente non si può censire", p)
	}
	if !strings.Contains(estrai(html, `id="cliente-nuovo"`), "hx-swap-oob") {
		t.Error("il riquadro non torna come scambio fuori bersaglio: comparirebbe dentro l'elenco dei buyer")
	}
	// il dominio del mittente è già lì: è il dato che serve perché la prossima mail di quel cliente
	// venga riconosciuta da sola
	if !strings.Contains(html, "acme.example") {
		t.Errorf("il dominio del mittente non è proposto:\n%s", estrai(html, `name="cliente_dominio"`))
	}
	// e il campo deve sapere a quale messaggio si riferisce, altrimenti digitando la cartella
	// l'anteprima verrebbe ricalcolata con la data di oggi invece che con quella della mail
	if !strings.Contains(estrai(html, `name="cliente_cartella"`), msg.String()) {
		t.Errorf("il campo «Cartella NAS» non sa di quale messaggio si sta parlando:\n%s", estrai(html, `name="cliente_cartella"`))
	}
}

// E tornando indietro su un cliente censito i campi devono SPARIRE dalla pagina, non restare in un
// riquadro che nessuno sostituisce: è il caso in cui l'operatore ha già scritto qualcosa.
func TestTornandoSuUnClienteCensitoICampiSpariscono(t *testing.T) {
	b := preparaBancoWeb(t)
	acmeMacchine := b.clienteDiProva("ACME MACCHINE", "ACME Macchine S.p.A.", "")
	msg := b.messaggioIn("<campi-3@acme.example>", b.francesco)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	par := url.Values{
		"cliente_id":       {acmeMacchine.String()},
		"cliente_cartella": {"QUELLO CHE AVEVO SCRITTO"},
		"messaggio":        {msg.String()},
		"oggetto":          {"Flange"},
	}
	_, html := w.fai(http.MethodGet, "/anagrafica/buyer?"+par.Encode(), nil, true)

	riquadro := estrai(html, `id="cliente-nuovo"`)
	if !strings.Contains(riquadro, "hx-swap-oob") {
		t.Fatalf("il riquadro «cliente-nuovo» non viene sostituito: i campi di prima restano in pagina, compilati\n%s", riquadro)
	}
	if p := campiPresenti(html); len(p) > 0 {
		t.Errorf("scegliendo un cliente censito i campi di censimento sono rimasti: %v", p)
	}
	if strings.Contains(html, "QUELLO CHE AVEVO SCRITTO") {
		t.Errorf("ciò che era stato digitato è tornato in pagina:\n%s", html)
	}
	if !strings.Contains(html, "<code>ACME MACCHINE</code>") {
		t.Error("la cartella dell'Anagrafica non ha preso il posto di quella digitata")
	}
}
