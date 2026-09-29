package regole

import (
	"encoding/json"
	"strings"
	"testing"
)

// L1 — Smistamento 4.13b (le scelte del 28/09 confermate il 29/09): le voci nuove delle regole del cliente
// per la posta. I mittenti di sistema (indirizzo o host → evento, con un esempio obbligatorio), il numero
// d'ordine nella forma del cliente (domanda 22b = A: una voce del cliente) e l'ordine da verificare (domanda
// 22a = A: la voce più debole del numero d'ordine anticipato). Che cosa il motore ne fa sta in
// core/inbox/classificazione (posta_cliente_test.go). Clienti, indirizzi e numeri inventati.

const regolePosta = `{
  "numero_ordine_anticipato": true,
  "ordine_da_verificare": true,
  "numero_ordine": {"regex": "\\bODA_\\d{7}\\b", "descrizione": "ordini ODA", "esempio": "ODA_0001234"},
  "mittenti_sistema": [
    {"mittente": "avvisi@acme.example", "evento": "ALTRO", "descrizione": "avvisi del gestionale", "esempio": "avvisi@acme.example"},
    {"mittente": "server.acme.example", "evento": "ORDINE", "esempio": "ordini@server.acme.example"}
  ]
}`

// L'esempio buono di ogni voce entra, ed entra intero: lo schema lo rilegge uguale dopo il giro di
// scrittura. Un cliente senza le voci non ne scrive le chiavi.
func TestLeVociDellaPostaBuoneEntranoIntere(t *testing.T) {
	r, err := ValidaRegole([]byte(regolePosta))
	if err != nil {
		t.Fatal(err)
	}
	if !r.NumeroOrdineAnticipato || !r.OrdineDaVerificare {
		t.Errorf("le chiavi dell'ordine: %+v", r)
	}
	if r.NumeroOrdine == nil || r.NumeroOrdine.Esempio != "ODA_0001234" || r.NumeroOrdine.Descrizione != "ordini ODA" {
		t.Errorf("numero d'ordine: %+v", r.NumeroOrdine)
	}
	if len(r.MittentiSistema) != 2 || r.MittentiSistema[0].Evento != EventoMittenteAltro || r.MittentiSistema[1].Mittente != "server.acme.example" {
		t.Fatalf("mittenti: %+v", r.MittentiSistema)
	}
	// ogni voce ha la sua riga ✓ nella diagnosi, con il nome che il motore cerca
	righe := map[string]bool{}
	for _, d := range r.Verifica() {
		righe[d.Regola] = d.Ok
	}
	for _, nome := range []string{RegolaNumeroOrdine, RegolaMittenteSistema(0), RegolaMittenteSistema(1)} {
		if ok, c := righe[nome]; !c || !ok {
			t.Errorf("la riga %q della diagnosi: presente %v, ✓ %v", nome, c, ok)
		}
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := ValidaRegole(b)
	if err != nil {
		t.Fatalf("il JSON riscritto dal server non passa più la sua stessa convalida: %v", err)
	}
	if len(r2.MittentiSistema) != 2 || r2.NumeroOrdine == nil || !r2.OrdineDaVerificare || r2.MittentiSistema[0].Descrizione != "avvisi del gestionale" {
		t.Errorf("il giro di scrittura e rilettura ha perso qualcosa: %s", b)
	}
	vuoto, _ := json.Marshal(Regole{})
	for _, chiave := range []string{"mittenti_sistema", "numero_ordine", "ordine_da_verificare"} {
		if strings.Contains(string(vuoto), chiave) {
			t.Errorf("la chiave vuota %q si scrive: %s", chiave, vuoto)
		}
	}
	// e i nomi nuovi stanno fra i campi ammessi, che il messaggio d'errore elenca
	campi := strings.Join(CampiRegole(), " ")
	for _, chiave := range []string{"mittenti_sistema", "numero_ordine", "ordine_da_verificare"} {
		if !strings.Contains(campi, chiave) {
			t.Errorf("CampiRegole non elenca %q: %s", chiave, campi)
		}
	}
}

// Un mittente di sistema rotto si segna ✗ con il suo motivo, e la porta in scrittura rifiuta le regole
// intere dicendo quale riga. Ogni caso è un modo di scrivere una riga che non prenderebbe la posta giusta
// senza fallire: un host largo, un jolly, un esempio che non è di quel mittente, un evento che lo schema
// non conosce.
func TestUnMittenteDiSistemaRottoSiRifiuta(t *testing.T) {
	casi := []struct {
		nome   string
		m      MittenteSistema
		atteso string
	}{
		{"senza mittente", MittenteSistema{Evento: "ALTRO", Esempio: "avvisi@acme.example"}, "manca il mittente"},
		{"con uno spazio", MittenteSistema{Mittente: "avvisi @acme.example", Evento: "ALTRO", Esempio: "avvisi@acme.example"}, "spazi o caratteri jolly"},
		{"con un jolly", MittenteSistema{Mittente: "*.acme.example", Evento: "ALTRO", Esempio: "x@srv.acme.example"}, "spazi o caratteri jolly"},
		{"un host con la chiocciola davanti", MittenteSistema{Mittente: "@srv.acme.example", Evento: "ALTRO", Esempio: "x@srv.acme.example"}, "senza la chiocciola"},
		{"due chiocciole", MittenteSistema{Mittente: "a@b@acme.example", Evento: "ALTRO", Esempio: "a@acme.example"}, "più di una chiocciola"},
		{"un host senza punto", MittenteSistema{Mittente: "acme", Evento: "ALTRO", Esempio: "x@acme"}, "non è un host"},
		{"un indirizzo senza host vero", MittenteSistema{Mittente: "avvisi@acme", Evento: "ALTRO", Esempio: "avvisi@acme"}, "non è un host"},
		{"un evento che lo schema non conosce", MittenteSistema{Mittente: "avvisi@acme.example", Evento: "SOLLECITO", Esempio: "avvisi@acme.example"}, "non è previsto"},
		{"l'evento in minuscolo", MittenteSistema{Mittente: "avvisi@acme.example", Evento: "altro", Esempio: "avvisi@acme.example"}, "non è previsto"},
		{"senza esempio", MittenteSistema{Mittente: "avvisi@acme.example", Evento: "ALTRO"}, "manca l'esempio"},
		{"l'esempio è un host", MittenteSistema{Mittente: "srv.acme.example", Evento: "ALTRO", Esempio: "srv.acme.example"}, "non è un indirizzo"},
		{"l'esempio è di un altro indirizzo", MittenteSistema{Mittente: "avvisi@acme.example", Evento: "ALTRO", Esempio: "buyer@acme.example"}, "non corrisponde"},
		// l'host vale solo per sé: non per il dominio sopra, non per un host sotto
		{"l'esempio viene dal dominio del cliente", MittenteSistema{Mittente: "srv.acme.example", Evento: "ORDINE", Esempio: "ordini@acme.example"}, "non corrisponde"},
		{"l'esempio viene da un host sotto", MittenteSistema{Mittente: "srv.acme.example", Evento: "ORDINE", Esempio: "ordini@altro.srv.acme.example"}, "non corrisponde"},
		// Ritocco della 4.13b: un host che è un dominio intero, con l'esempio che lo mostra come il dominio di un
		// indirizzo qualunque del cliente. Prima si accettava, e ogni mail del cliente, anche le richieste dei
		// buyer, diventava un avviso ignorato (o un ordine)
		{"un dominio intero", MittenteSistema{Mittente: "acme.example", Evento: "ALTRO", Esempio: "avvisi@acme.example"}, "è un dominio intero"},
		{"un dominio intero, in maiuscolo, per gli ordini", MittenteSistema{Mittente: "ACME.Example", Evento: "ORDINE", Esempio: "ordini@acme.example"}, "è un dominio intero"},
		{"un dominio nazionale composto", MittenteSistema{Mittente: "acme.co.uk", Evento: "ALTRO", Esempio: "avvisi@acme.co.uk"}, "è un dominio intero"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d := verificaMittente("mittente di sistema 1", c.m)
			if d.Ok {
				t.Fatalf("accettato: %+v", c.m)
			}
			if !strings.Contains(d.Motivo, c.atteso) {
				t.Errorf("il motivo %q non dice %q", d.Motivo, c.atteso)
			}
			// la porta in scrittura rifiuta le regole intere, e dice quale riga
			j, _ := json.Marshal(map[string]any{"mittenti_sistema": []MittenteSistema{
				{Mittente: "avvisi@acme.example", Evento: "ALTRO", Esempio: "avvisi@acme.example"}, c.m}})
			_, err := ValidaRegole(j)
			if err == nil || !strings.Contains(err.Error(), "mittente di sistema 2") || !strings.Contains(err.Error(), c.atteso) {
				t.Errorf("la convalida delle regole: %v", err)
			}
		})
	}
	// la stessa posta due volte, con due eventi: vale la prima riga, la seconda è ✗ e dice quale
	j := []byte(`{"mittenti_sistema":[
	  {"mittente":"avvisi@acme.example","evento":"ALTRO","esempio":"avvisi@acme.example"},
	  {"mittente":"AVVISI@acme.example","evento":"ORDINE","esempio":"avvisi@acme.example"}]}`)
	if _, err := ValidaRegole(j); err == nil || !strings.Contains(err.Error(), "è già il mittente di sistema 1") {
		t.Errorf("la riga ripetuta: %v", err)
	}
	// ventuno righe sono troppe
	troppi := make([]MittenteSistema, maxMittenti+1)
	for i := range troppi {
		troppi[i] = MittenteSistema{Mittente: "srv" + string(rune('a'+i)) + ".acme.example", Evento: "ALTRO", Esempio: "x@srv" + string(rune('a'+i)) + ".acme.example"}
	}
	j, _ = json.Marshal(map[string]any{"mittenti_sistema": troppi})
	if _, err := ValidaRegole(j); err == nil || !strings.Contains(err.Error(), "il limite è 20") {
		t.Errorf("ventuno mittenti: %v", err)
	}
	j, _ = json.Marshal(map[string]any{"mittenti_sistema": troppi[:maxMittenti]})
	if _, err := ValidaRegole(j); err != nil {
		t.Errorf("venti mittenti: %v", err)
	}
}

// Quale posta prende una riga: l'indirizzo per uguaglianza, l'host per la parte dopo la chiocciola, senza
// maiuscole e senza le parentesi angolari di un'intestazione. Mai il dominio sopra, mai un host sotto.
func TestQualePostaPrendeUnMittenteDiSistema(t *testing.T) {
	indirizzo := MittenteSistema{Mittente: "Avvisi@ACME.example", Evento: "ALTRO"}
	host := MittenteSistema{Mittente: "srv.acme.example", Evento: "ORDINE"}
	casi := []struct {
		m        MittenteSistema
		mittente string
		atteso   bool
	}{
		{indirizzo, "avvisi@acme.example", true},
		{indirizzo, " <AVVISI@acme.EXAMPLE> ", true},
		{indirizzo, "avvisi2@acme.example", false},
		{indirizzo, "buyer@acme.example", false},
		{indirizzo, "", false},
		{host, "ordini@srv.acme.example", true},
		{host, "QUALUNQUE@SRV.ACME.EXAMPLE", true},
		{host, "ordini@acme.example", false},
		{host, "ordini@altro.srv.acme.example", false},
		{host, "srv.acme.example", false}, // un host non è un mittente
		{MittenteSistema{}, "x@acme.example", false},
	}
	for _, c := range casi {
		if got := c.m.Corrisponde(c.mittente); got != c.atteso {
			t.Errorf("%q prende %q: %v, atteso %v", c.m.Mittente, c.mittente, got, c.atteso)
		}
	}
}

// Il numero d'ordine è una regex con il suo esempio, come il riferimento: rotta, senza esempio o con
// l'esempio che non le corrisponde non entra, e la diagnosi la chiama per nome.
func TestIlNumeroDOrdineVuoleIlSuoEsempio(t *testing.T) {
	casi := []struct{ jsonIn, atteso string }{
		{`{"numero_ordine":{"regex":"ODA_[0-9","esempio":"ODA_1"}}`, "non compila"},
		{`{"numero_ordine":{"regex":"\\bODA_\\d{7}\\b"}}`, "manca l'esempio"},
		{`{"numero_ordine":{"regex":"\\bODA_\\d{7}\\b","esempio":"ODA_12"}}`, "non corrisponde"},
		{`{"numero_ordine":{"esempio":"ODA_0001234"}}`, "manca la regex"},
		{`{"ordine_da_verificare":"si"}`, "ordine_da_verificare"},
		// Ritocco della 4.13b: una regex che prende anche il testo vuoto passa l'esempio, e il motore, che toglie
		// i numeri d'ordine dal testo prima di leggerci i codici, toglieva il vuoto fra ogni lettera. Prima si
		// accettava. L'ultima non trova niente nel testo vuoto (lì non c'è un confine di parola), e il vuoto lo
		// trova in ogni testo vero: la forma della regex lo dice lo stesso
		{`{"numero_ordine":{"regex":"(ODA_\\d{7})?","esempio":"ODA_0001234"}}`, "la regex prende anche il testo vuoto"},
		{`{"numero_ordine":{"regex":"\\d*","esempio":"4500012345"}}`, "la regex prende anche il testo vuoto"},
		{`{"numero_ordine":{"regex":"^$|\\bODA_\\d{7}\\b","esempio":"ODA_0001234"}}`, "la regex prende anche il testo vuoto"},
		{`{"numero_ordine":{"regex":"\\b(?:ODA_)?\\d*\\b","esempio":"ODA_0001234"}}`, "la regex prende anche il testo vuoto"},
	}
	for _, c := range casi {
		_, err := ValidaRegole([]byte(c.jsonIn))
		if err == nil {
			t.Errorf("accettato: %s", c.jsonIn)
			continue
		}
		if !strings.Contains(err.Error(), c.atteso) {
			t.Errorf("%s: l'errore non dice %q: %v", c.jsonIn, c.atteso, err)
		}
		if strings.Contains(c.jsonIn, `"numero_ordine"`) && !strings.Contains(err.Error(), RegolaNumeroOrdine) {
			t.Errorf("%s: l'errore non dice quale voce: %v", c.jsonIn, err)
		}
	}
}

// Ritocco della 4.13b: quale regex prende il testo vuoto. Non basta il testo vuoto (MatchString("")): un
// confine di parola, un'ancora, una ripetizione da zero trovano il vuoto dentro un testo vero, e si vede dalla
// forma. Una regex che legge sempre almeno un carattere passa, con le ancore e i confini intorno. La porta in
// lettura (LeggiRegole) segna la riga ✗ con lo stesso motivo, e il motore non la usa (Compila guarda la
// diagnosi): una regola scritta a mano nel database non passa per ValidaRegole.
func TestQualeRegexDelNumeroDOrdinePrendeIlVuoto(t *testing.T) {
	casi := []struct {
		regex string
		vuoto bool
	}{
		{`\bODA_\d{7}\b`, false},
		{`ODA_\d+`, false},
		{`(?i)\boda_\d{7}\b`, false},
		{`^ODA_\d{7}$`, false},
		{`(?:ODA|ODV)_\d{7}`, false},
		{`ODA_\d{0,7}`, false}, // «ODA_» c'è sempre
		{`(?:ODA_\d{7})+`, false},
		{``, true},
		{`\b`, true},
		{`$`, true},
		{`\d*`, true},
		{`(ODA_\d{7})?`, true},
		{`(?:ODA_\d{7}){0,1}`, true},
		{`ODA_\d{7}|`, true},
		{`(?:x*)+`, true},
		{`\b(?:ODA_)?\d*\b`, true},
		{`ODA_[0-9`, false}, // non compila: lo dice verificaRegex, non questa
	}
	for _, c := range casi {
		if got := prendeIlVuoto(c.regex); got != c.vuoto {
			t.Errorf("%q prende il vuoto: %v, atteso %v", c.regex, got, c.vuoto)
		}
	}
	// la stessa regex dalla porta in lettura: letta, segnata ✗ con il motivo, non usabile
	_, diag := LeggiRegole([]byte(`{"numero_ordine":{"regex":"(?:ODA_\\d{7})?","esempio":"ODA_0001234"}}`))
	if len(diag) != 1 || diag[0].Regola != RegolaNumeroOrdine || diag[0].Ok || !strings.Contains(diag[0].Motivo, "prende anche il testo vuoto") {
		t.Errorf("la diagnosi della regex che prende il vuoto: %+v", diag)
	}
	// il controllo è della voce che toglie testo: la regex buona entra, come prima
	if _, err := ValidaRegole([]byte(`{"numero_ordine":{"regex":"\\bODA_\\d{7}\\b","esempio":"ODA_0001234"}}`)); err != nil {
		t.Errorf("la regex buona: %v", err)
	}
}

// Ritocco della 4.13b: un host che è un dominio intero non è un mittente di sistema. Il nome di un sistema
// dentro il dominio del cliente sì, anche con un dominio nazionale composto; e un indirizzo al dominio intero
// resta l'indirizzo di uno solo. Il motivo dice perché e che cosa scrivere invece.
func TestUnDominioInteroNonEUnMittenteDiSistema(t *testing.T) {
	for _, m := range []MittenteSistema{
		{Mittente: "srv.acme.example", Evento: "ALTRO", Esempio: "avvisi@srv.acme.example"},
		{Mittente: "srv.acme.co.uk", Evento: "ALTRO", Esempio: "avvisi@srv.acme.co.uk"},
		{Mittente: "acme.com.example", Evento: "ORDINE", Esempio: "ordini@acme.com.example"}, // «example» non è un paese
		{Mittente: "avvisi@acme.example", Evento: "ALTRO", Esempio: "avvisi@acme.example"},
		{Mittente: "avvisi@acme.co.uk", Evento: "ALTRO", Esempio: "avvisi@acme.co.uk"},
	} {
		if d := verificaMittente("mittente di sistema 1", m); !d.Ok {
			t.Errorf("%s rifiutato: %s", m.Mittente, d.Motivo)
		}
	}
	d := verificaMittente("mittente di sistema 1", MittenteSistema{Mittente: "acme.example", Evento: "ALTRO", Esempio: "avvisi@acme.example"})
	for _, atteso := range []string{"«acme.example» è un dominio intero", "@acme.example", "«avvisi@acme.example»", "un avviso ignorato", "«server.acme.example»"} {
		if !strings.Contains(d.Motivo, atteso) {
			t.Errorf("il motivo %q non dice %q", d.Motivo, atteso)
		}
	}
	if d := verificaMittente("mittente di sistema 1", MittenteSistema{Mittente: "acme.example", Evento: "ORDINE", Esempio: "x@acme.example"}); !strings.Contains(d.Motivo, "un ordine") {
		t.Errorf("il motivo per gli ordini: %q", d.Motivo)
	}
}

// Ritocco della 4.13b: il controllo dei mittenti di sistema che vuole il database (il salvataggio
// dall'Anagrafica legge domini e contatti del cliente, e lo chiama). Si rifiuta un host che è il dominio
// di un contatto, o un dominio del cliente che non sta dentro un altro suo dominio (il principale, anche con
// tre parti). Passa l'host di un sistema registrato dentro un dominio del cliente, che è ciò che serve perché
// la sua posta arrivi alle regole del cliente; passa ogni indirizzo; passa tutto per un cliente che non ha
// quei domini né quei contatti (la controprova).
func TestIMittentiDiSistemaSuiDominiESuiContattiDelCliente(t *testing.T) {
	chi := ChiScriveDalCliente{
		Domini:   []string{"acme.example", "ACME-italia.gruppo.example", "srv.acme.example", "srv.xacme.example"},
		Contatti: []Contatto{{Nome: "Bianchi Anna", Indirizzo: "anna.bianchi@acme.example"}, {Nome: "Rossi Mario", Indirizzo: " Mario.Rossi@Uffici.ACME.example "}},
	}
	regola := func(mittente, evento, esempio string) Regole {
		return Regole{MittentiSistema: []MittenteSistema{
			{Mittente: "avvisi@acme.example", Evento: "ALTRO", Esempio: "avvisi@acme.example"},
			{Mittente: mittente, Evento: evento, Esempio: esempio}}}
	}
	rifiutati := []struct {
		r      Regole
		attesi []string
	}{
		{regola("uffici.acme.example", "ALTRO", "x@uffici.acme.example"),
			[]string{"mittente di sistema 2", "«uffici.acme.example» è il dominio di Rossi Mario (mario.rossi@uffici.acme.example)", "un contatto di questo cliente", "un avviso ignorato"}},
		{regola("acme-italia.gruppo.example", "ORDINE", "x@acme-italia.gruppo.example"),
			[]string{"mittente di sistema 2", "«acme-italia.gruppo.example» è un dominio di questo cliente", "non sta dentro un altro", "un ordine", "«avvisi@acme-italia.gruppo.example»"}},
		// il confine è il punto: «srv.xacme.example» non sta dentro «acme.example»
		{regola("srv.xacme.example", "ALTRO", "x@srv.xacme.example"), []string{"«srv.xacme.example» è un dominio di questo cliente"}},
	}
	for _, c := range rifiutati {
		if _, err := ValidaRegole(mustJSON(t, c.r)); err != nil {
			t.Fatalf("la prova vuole regole che la convalida senza database accetta: %v", err)
		}
		err := MittentiSulCliente(c.r, chi)
		for _, atteso := range c.attesi {
			if err == nil || !strings.Contains(err.Error(), atteso) {
				t.Errorf("%s: l'errore non dice %q: %v", c.r.MittentiSistema[1].Mittente, atteso, err)
			}
		}
	}
	accettati := []Regole{
		regola("srv.acme.example", "ALTRO", "rfq@srv.acme.example"),                // registrato dentro «acme.example»
		regola("posta.acme.example", "ALTRO", "x@posta.acme.example"),              // non è né un dominio né un contatto del cliente
		regola("anna.bianchi@acme.example", "ORDINE", "anna.bianchi@acme.example"), // un indirizzo vale da solo
		{},
	}
	for _, r := range accettati {
		if err := MittentiSulCliente(r, chi); err != nil {
			t.Errorf("rifiutato: %v", err)
		}
	}
	// la controprova: lo stesso host, per un cliente che non ha quei domini né quei contatti
	for _, c := range rifiutati {
		if err := MittentiSulCliente(c.r, ChiScriveDalCliente{Domini: []string{"beta.example"}}); err != nil {
			t.Errorf("per un altro cliente: %v", err)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Scelta 6 del 28/09: dopo «PN», «P/N», «cod.» il cliente scrive anche «:», «.», «#» e «n.». Le parole
// chiave stanno dentro le regex dei clienti, che sono dati: lo schema accetta una famiglia che le ha, e
// l'esempio con il separatore la prova. Che cosa il motore ne estrae sta in classificazione.
func TestUnaFamigliaConLaParolaChiaveEntra(t *testing.T) {
	for _, esempio := range []string{"PN: 7120001", "P/N # 7120001", "cod. n. 7120001", "PN.7120001", "P/N 7120001"} {
		j, _ := json.Marshal(map[string]any{"famiglie_codice": []map[string]any{{
			"regex":   `(?:\bPN|\bP/N|\bcod\.)\s*(?:[:.#]|n\.)?\s*(?P<codice>7120\d{3})\b`,
			"esempio": esempio, "descrizione": "ACME con la parola chiave"}}})
		if _, err := ValidaRegole(j); err != nil {
			t.Errorf("esempio %q: %v", esempio, err)
		}
	}
	// e senza la parola chiave l'esempio non le corrisponde: la parola resta obbligatoria
	j, _ := json.Marshal(map[string]any{"famiglie_codice": []map[string]any{{
		"regex":   `(?:\bPN|\bP/N|\bcod\.)\s*(?:[:.#]|n\.)?\s*(?P<codice>7120\d{3})\b`,
		"esempio": "7120001"}}})
	if _, err := ValidaRegole(j); err == nil || !strings.Contains(err.Error(), "non corrisponde") {
		t.Errorf("l'esempio senza parola chiave: %v", err)
	}
}
