package classificazione

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

// L1 — Smistamento 4.13b: le scelte del 28/09 confermate il 29/09 («Nessuna contestata»), nel motore della
// posta. I mittenti di sistema del cliente (scelta 1), il numero d'ordine anticipato e la sua voce più
// debole (scelta 2, domanda 22a = A), il numero d'ordine nella forma del cliente (scelta 3, domanda 22b =
// A), le parole chiave dentro le regex dei clienti (scelta 6), i formati di Solid Edge (scelta 7). Lo schema
// delle voci sta in core/registro/regole (mittenti_test.go). Clienti, indirizzi e numeri inventati.

// motorePosta compila le regole ACME della prova; una regola che non passa la convalida ferma la prova, che
// altrimenti proverebbe un motore senza la voce.
func motorePosta(t *testing.T, j string) *Motore {
	t.Helper()
	r, err := regole.ValidaRegole([]byte(j))
	if err != nil {
		t.Fatalf("regole della prova: %v", err)
	}
	return Compila("ACME S.p.A.", r)
}

// le regole ACME con i due mittenti di sistema: un indirizzo che manda avvisi, uno che manda ordini, e un
// host del gestionale che manda avvisi
const regoleMittenti = `{
  "famiglie_codice": [{"regex": "\\b7120\\d{3}\\b", "descrizione": "codici ACME", "esempio": "7120001"}],
  "mittenti_sistema": [
    {"mittente": "avvisi@acme.example", "evento": "ALTRO", "descrizione": "avvisi del gestionale", "esempio": "avvisi@acme.example"},
    {"mittente": "ordini@acme.example", "evento": "ORDINE", "descrizione": "ordini generati dal sistema", "esempio": "ordini@acme.example"},
    {"mittente": "srv.acme.example", "evento": "ALTRO", "esempio": "rfq@srv.acme.example"},
    {"mittente": "noreply@acme.example", "evento": "ORDINE", "esempio": "noreply@acme.example"}
  ]
}`

// daMittente è la mail di un indirizzo di ACME, con il motore dato.
func daMittente(m *Motore, mittente, oggetto, corpo string, allegati ...string) IngressoTriage {
	in := dalCliente(oggetto, corpo, allegati...)
	in.Mittente, in.Motore = mittente, m
	return in
}

// Scelta 1: un mittente di sistema del cliente ha l'evento dichiarato, letto prima delle parole, e la sua
// mail non propone una RFQ né i suoi codici come prodotti. «avvisi@» è ALTRO anche con «RFQ» e «richiesta
// d'offerta» nell'oggetto; «ordini@» è ORDINE; l'host vale per la posta che viene da lì. La controprova è
// la stessa mail dal buyer: una richiesta nuova, con i codici proposti.
func TestIMittentiDiSistemaDelCliente(t *testing.T) {
	m := motorePosta(t, regoleMittenti)
	const oggetto = "RFQ 7120001 - richiesta d'offerta"
	const corpo = "Nuova RFQ disponibile per il codice 7120001 e 7120010."
	casi := []struct {
		nome, mittente, evento, atto string
	}{
		{"un avviso, per indirizzo", "avvisi@acme.example", EventoAltro, AttoNotifica},
		{"un ordine generato da un sistema", "ordini@acme.example", EventoOrdine, AttoOrdine},
		{"un avviso, per host", "RFQ@srv.acme.example", EventoAltro, AttoNotifica},
		// E1b viene prima di E2: le parole generiche di un indirizzo automatico non spengono la voce del cliente
		{"un ordine da un indirizzo «noreply»", "noreply@acme.example", EventoOrdine, AttoOrdine},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			in := daMittente(m, c.mittente, oggetto, corpo, "7120001A_1.stp")
			tr := Triage(in)
			if tr.Evento != c.evento || tr.Atto != c.atto || tr.ForzaEvento != ForzaChiaro {
				t.Fatalf("evento %s/%s (%s), atteso %s/%s (chiaro): %v", tr.Evento, tr.Atto, tr.ForzaEvento, c.evento, c.atto, tr.EvidenzeEvento)
			}
			if ev := strings.Join(tr.EvidenzeEvento, " | "); !strings.Contains(ev, "mittente di sistema del cliente") {
				t.Errorf("evidenze dell'evento: %q", ev)
			}
			if d := EventoDa(ControparteCliente, "entrata", tr.Atto, tr.Legame); d != tr.Evento {
				t.Errorf("EventoDa rilegge %s, Evento dice %s", d, tr.Evento)
			}
			if tr.Esito != "ignora" || tr.Confidenza != 0 {
				t.Errorf("esito %s a %d: un mittente di sistema non apre una RFQ", tr.Esito, tr.Confidenza)
			}
			if len(tr.Codici) != 0 || len(tr.Estrazione.Proponibili()) != 0 {
				t.Errorf("codici proposti: %v %+v", tr.Codici, tr.Estrazione.Proponibili())
			}
			// i codici si vedono, fra gli altri numeri, con il ruolo «non classificato»
			altri := SoloCodici(tr.Estrazione.Altri())
			if !slices.Contains(altri, "7120001") || !slices.Contains(altri, "7120010") {
				t.Errorf("i codici della mail non si vedono più: %v", altri)
			}
			for _, x := range tr.Estrazione.Codici {
				if x.Ruolo != RuoloIgnoto {
					t.Errorf("%s ha il ruolo %q: un mittente di sistema non propone prodotti", x.Codice, x.Ruolo)
				}
			}
			if len(tr.Motivi) == 0 || !strings.HasPrefix(tr.Motivi[0], "mittente di sistema del cliente: ") {
				t.Errorf("la frase del mittente di sistema non è in testa ai motivi: %q", tr.Motivi)
			}
		})
	}

	// la controprova: la stessa mail dal buyer è una richiesta nuova, con i codici della famiglia proposti
	tr := Triage(daMittente(m, "buyer@acme.example", oggetto, corpo, "7120001A_1.stp"))
	if tr.Evento != EventoNuovaRFQ || tr.Esito != "nuova_rfq" || !slices.Contains(tr.Codici, "7120001") {
		t.Fatalf("dal buyer: %s, %s, %v", tr.Evento, tr.Esito, tr.Codici)
	}
	// l'host vale solo per sé: dal dominio del cliente la mail è del buyer, non del gestionale
	if tr := Triage(daMittente(m, "rfq@acme.example", oggetto, corpo, "7120001A_1.stp")); tr.Evento != EventoNuovaRFQ {
		t.Errorf("rfq@acme.example letto come il gestionale: %s", tr.Evento)
	}
	// e senza la voce «noreply@» è posta che non è di lavoro, come prima
	senza := Compila("ACME S.p.A.", regole.Regole{FamiglieCodice: m.Regole.FamiglieCodice})
	if tr := Triage(daMittente(senza, "noreply@acme.example", oggetto, corpo)); tr.Atto != AttoNonBusiness || tr.Esito != "ignora" {
		t.Errorf("noreply senza la voce: %s/%s", tr.Atto, tr.Esito)
	}
	// e un ordine di sistema su una RFQ che c'è si aggancia: la voce toglie la RFQ nuova, non le evidenze
	in := daMittente(m, "ordini@acme.example", "R: RFQ 7120001", "Ordine per 7120001.")
	in.Candidati = []Candidato{NuovoCandidato("t-aperta", TipoR0InReplyTo, "In-Reply-To verso la richiesta", false)}
	if tr := Triage(in); tr.Esito != "aggancia" || tr.Candidato == nil || tr.Candidato.ThreadID != "t-aperta" || tr.Evento != EventoOrdine {
		t.Errorf("l'ordine di sistema con R0: %s %+v %s", tr.Esito, tr.Candidato, tr.Evento)
	}
}

// Ritocco della 4.13b: l'indirizzo prima dell'host. L'host del gestionale manda avvisi e, sullo stesso host,
// un indirizzo manda gli ordini; le righe sono scritte con l'host PRIMA. L'indirizzo dice di più e vince lo
// stesso: la mail di «ordini@srv.acme.example» è un ORDINE, ogni altra mail di quell'host un avviso. È
// l'ordinamento di Compila (gli indirizzi prima degli host); senza, vinceva la prima riga scritta.
func TestUnIndirizzoVinceSulSuoHostAncheSeScrittoDopo(t *testing.T) {
	m := motorePosta(t, `{"mittenti_sistema": [
	  {"mittente": "srv.acme.example", "evento": "ALTRO", "descrizione": "avvisi del gestionale", "esempio": "avvisi@srv.acme.example"},
	  {"mittente": "ordini@srv.acme.example", "evento": "ORDINE", "descrizione": "ordini generati dal sistema", "esempio": "ordini@srv.acme.example"}
	]}`)
	if r := m.Regole.MittentiSistema; len(r) != 2 || r[0].Mittente != "srv.acme.example" {
		t.Fatalf("la prova vuole l'host scritto prima dell'indirizzo: %+v", r)
	}
	casi := []struct{ mittente, evento, atto, riga string }{
		{"ordini@srv.acme.example", EventoOrdine, AttoOrdine, "ordini@srv.acme.example"},
		{"<ORDINI@Srv.ACME.example>", EventoOrdine, AttoOrdine, "ordini@srv.acme.example"},
		{"avvisi@srv.acme.example", EventoAltro, AttoNotifica, "srv.acme.example"},
		{"rfq@srv.acme.example", EventoAltro, AttoNotifica, "srv.acme.example"},
	}
	for _, c := range casi {
		if ms, ok := m.MittenteDiSistema(c.mittente); !ok || regole.NormaIndirizzo(ms.Mittente) != c.riga {
			t.Errorf("%s: la riga che lo prende è %q (%v), attesa %q", c.mittente, ms.Mittente, ok, c.riga)
		}
		tr := Triage(daMittente(m, c.mittente, "RFQ 7120001", "Ordine per il codice 7120001."))
		if tr.Evento != c.evento || tr.Atto != c.atto || tr.ForzaEvento != ForzaChiaro {
			t.Errorf("%s: evento %s/%s (%s), atteso %s/%s (chiaro): %v", c.mittente, tr.Evento, tr.Atto, tr.ForzaEvento, c.evento, c.atto, tr.EvidenzeEvento)
		}
		if ev := strings.Join(tr.EvidenzeEvento, " | "); !strings.Contains(ev, "mittente di sistema del cliente: "+c.riga+" ") {
			t.Errorf("%s: le evidenze non dicono la riga %q: %q", c.mittente, c.riga, ev)
		}
	}
	// e dal dominio del cliente, fuori dall'host, nessuna delle due righe
	if ms, ok := m.MittenteDiSistema("ordini@acme.example"); ok {
		t.Errorf("ordini@acme.example preso dalla riga %q", ms.Mittente)
	}
}

// Ritocco della 4.13b: un numero d'ordine con una regex che prende anche il testo vuoto. La convalida la
// rifiuta, la porta in lettura la segna ✗ e Compila non la usa: i codici della mail restano. E se il motore
// la ricevesse lo stesso (la seconda difesa), le corrispondenze vuote non tolgono niente. Prima il testo in
// cui si leggono i codici diventava « 7 1 2 0 0 0 1 » e ogni codice della posta spariva, anche dai nomi degli
// allegati; il codice di un nome citato fuori dall'ordine spariva; e il primo vuoto nascondeva l'ordine vero.
func TestIlNumeroDOrdineCheTrovaIlVuotoNonToglieICodici(t *testing.T) {
	const j = `{
	  "famiglie_codice": [{"regex": "\\b7120\\d{3}\\b", "descrizione": "codici ACME", "esempio": "7120001"}],
	  "numero_ordine": {"regex": "(?:ODA_\\d{7})?", "descrizione": "ordini ODA", "esempio": "ODA_0001234"}
	}`
	const oggetto = "RFQ 7120001"
	const corpo = "Richiesta d'offerta per i pezzi 7120001 e 7120010, nostro ordine ODA_0001234."
	if _, err := regole.ValidaRegole([]byte(j)); err == nil || !strings.Contains(err.Error(), "prende anche il testo vuoto") {
		t.Fatalf("la convalida: %v", err)
	}
	lette, _ := regole.LeggiRegole([]byte(j))
	letto := Compila("ACME S.p.A.", lette)
	if letto.ordine != nil || len(letto.NumeriOrdine(corpo)) != 0 {
		t.Errorf("Compila usa una regex del numero d'ordine segnata ✗")
	}
	if tr := Triage(daMittente(letto, "buyer@acme.example", oggetto, corpo, "7120011.stp")); !slices.Equal(tr.Codici, []string{"7120001", "7120010", "7120011"}) {
		t.Errorf("con la riga ✗ i codici proposti sono %v", tr.Codici)
	}

	// la seconda difesa: il motore con la regex che prende il vuoto, come se fosse arrivata per un'altra strada
	m := Compila("ACME S.p.A.", lette)
	m.ordine, m.ordineNome = regexp.MustCompile(`(?:ODA_\d{7})?`), "ordini ODA"
	if got := m.NumeriOrdine(corpo); !slices.Equal(got, []string{"ODA_0001234"}) {
		t.Errorf("numeri d'ordine %q: le corrispondenze vuote non sono numeri", got)
	}
	if got := m.NumeriOrdine("pezzi 7120001 e 7120010"); len(got) != 0 {
		t.Errorf("numeri d'ordine in un testo senza ordine: %q", got)
	}
	if got := m.senzaNumeriOrdine("pezzi 7120001 e 7120010"); got != "pezzi 7120001 e 7120010" {
		t.Errorf("il testo senza i numeri d'ordine è %q", got)
	}
	// la mail: i codici restano, anche quello nel nome dell'allegato; l'ordine vero no
	tr := Triage(daMittente(m, "buyer@acme.example", oggetto, corpo, "7120011.stp"))
	if !slices.Equal(tr.Codici, []string{"7120001", "7120010", "7120011"}) {
		t.Errorf("i codici proposti sono %v", tr.Codici)
	}
	for _, x := range tr.Estrazione.Codici {
		if strings.Contains(x.Codice, "0001234") {
			t.Errorf("il numero d'ordine è fra i codici: %+v", x)
		}
	}
	// il nome di un file: il codice citato fuori dall'ordine resta
	if got := m.CitatiNelNome("ODA_7120001 7120001.pdf", []string{"ODA_7120001", "7120001"}); !slices.Equal(got, []string{"7120001"}) {
		t.Errorf("i codici citati dal nome: %v", got)
	}
	// l'evento: l'ordine vero dopo il testo si legge, e un testo senza ordine non è un ordine
	e := Evento(IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", Mittente: "buyer@acme.example",
		Oggetto: "Codice 7120001", Corpo: "In allegato il documento ODA_0001234 per 20 pezzi.", Motore: m})
	if e.Evento != EventoOrdine || !strings.Contains(strings.Join(e.Evidenze, " | "), "«ODA_0001234», un numero d'ordine del cliente") {
		t.Errorf("l'ordine dopo il testo: %s %v", e.Evento, e.Evidenze)
	}
	e = Evento(IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", Mittente: "buyer@acme.example",
		Oggetto: "Codice 7120001", Corpo: "Richiesta d'offerta per 20 pezzi del 7120001.", Motore: m})
	if e.Evento != EventoNuovaRFQ {
		t.Errorf("la richiesta senza ordine: %s %v", e.Evento, e.Evidenze)
	}
}

// Scelta 1, i confini: la voce è del cliente e vale per la sua posta in entrata. Non per un collega che
// gira la mail, non per un fornitore; e una riga ✗ non si usa.
func TestUnMittenteDiSistemaValeSoloPerLaPostaDelCliente(t *testing.T) {
	m := motorePosta(t, regoleMittenti)
	e := Evento(IngressoEvento{Controparte: ControparteFornitore, Direzione: "entrata", Mittente: "avvisi@acme.example",
		Oggetto: "Offerta 7120001", Corpo: "In allegato la nostra offerta.", NomiAllegati: []string{"offerta.pdf"}, Motore: m})
	if e.Evento != EventoOffertaFornitore {
		t.Errorf("un fornitore con l'indirizzo di un avviso: %s", e.Evento)
	}
	in := daMittente(m, "avvisi@acme.example", "I: RFQ 7120001", "Ti giro questa.\n\n-----Messaggio inoltrato-----\nDa: Buyer <buyer@acme.example>\nOggetto: RFQ 7120001\n\nVi chiediamo un preventivo per 7120001.")
	in.Controparte, in.Direzione, in.Interno = ControparteInterno, "uscita", true
	if tr := Triage(in); tr.Evento == EventoAltro || strings.Contains(strings.Join(tr.Motivi, " "), "mittente di sistema") {
		t.Errorf("il collega che gira: %s %q", tr.Evento, tr.Motivi)
	}
	// una riga ✗ (l'esempio non le corrisponde) non si usa: la mail torna a essere letta dalle parole
	rotte, _ := regole.LeggiRegole([]byte(`{"mittenti_sistema":[{"mittente":"avvisi@acme.example","evento":"ALTRO","esempio":"altro@acme.example"}]}`))
	if tr := Triage(daMittente(Compila("ACME S.p.A.", rotte), "avvisi@acme.example", "RFQ 7120001", "Richiesta d'offerta per 7120001.")); tr.Evento != EventoNuovaRFQ {
		t.Errorf("una riga ✗ usata: %s", tr.Evento)
	}
	// Correzione 1 della 4.13b: con la controparte non dichiarata (il banco di prova dell'Anagrafica, i messaggi
	// di prima della 0014) il triage e l'evento riconoscono il mittente di sistema con la stessa condizione.
	// Prima i motivi dicevano «mittente di sistema» e l'evento si leggeva dalle parole («rfq»).
	for _, x := range []struct{ mittente, atto string }{{"avvisi@acme.example", AttoNotifica}, {"ordini@acme.example", AttoOrdine}} {
		in := daMittente(m, x.mittente, "RFQ 7120001 - richiesta d'offerta", "Nuova RFQ disponibile per il codice 7120001.")
		in.Controparte = ""
		tr := Triage(in)
		if len(tr.Motivi) == 0 || !strings.HasPrefix(tr.Motivi[0], "mittente di sistema del cliente: ") || tr.Esito != "ignora" {
			t.Errorf("%s senza controparte, il triage: %s %q", x.mittente, tr.Esito, tr.Motivi)
		}
		if ev := strings.Join(tr.EvidenzeEvento, " | "); !strings.HasPrefix(ev, "mittente di sistema del cliente: ") {
			t.Errorf("%s senza controparte, l'evento del triage si legge dalle parole: %q", x.mittente, ev)
		}
		// l'atto del triage resta vuoto per una controparte non dichiarata (come prima della 0014): quello
		// dell'evento è l'atto dichiarato dalla voce
		if e := Evento(in.ingressoEvento()); e.Atto != x.atto || tr.Atto != "" {
			t.Errorf("%s senza controparte: atto dell'evento %q (atteso %q), del triage %q", x.mittente, e.Atto, x.atto, tr.Atto)
		}
	}
	// gli eventi dello schema sono eventi di questo package, e l'atto con cui si salvano li rilegge
	for _, ev := range regole.EventiMittenteSistema {
		if !slices.Contains(Eventi, ev) {
			t.Errorf("l'evento %q dello schema non è un evento", ev)
		}
		if d := EventoDa(ControparteCliente, "entrata", attoMittenteSistema(ev), LegameNessuno); d != ev {
			t.Errorf("%s si salva con l'atto %s, che si rilegge %s", ev, attoMittenteSistema(ev), d)
		}
	}
}

// Scelta 2 (domanda 22a = A): il numero d'ordine anticipato e la sua voce più debole. Con la chiave, un
// ordine accanto a una parola di richiesta (o a «SI RFQ», «SI CTE») non decide: la mail si legge come senza
// l'ordine, e l'ordine resta fra le evidenze. Una richiesta è una richiesta nuova; un «Sollecito RDO …» resta
// un sollecito e una revisione dei disegni resta una revisione (la precedenza della 4.13, la risposta 16 = A).
// Senza parola di richiesta resta un ordine. Senza la chiave (la controprova) l'ordine è chiaro come prima.
// Con «ordine_da_verificare» l'ordine scende a probabile e sceglie l'operatore.
//
// Correzione 1 della 4.13b: prima, con la chiave e una richiesta, E4 decideva NUOVA_RFQ e saltava E5 ed E6. I
// casi del sollecito, della revisione e della formula di chiusura sono nuovi; e per ogni caso con una
// `senzaOrdine` la stessa mail senza il numero d'ordine deve avere lo stesso evento, senza l'evidenza
// dell'ordine: con la chiave, il numero d'ordine accanto a una richiesta non cambia l'evento.
func TestIlNumeroDOrdineAnticipato(t *testing.T) {
	anticipato := motorePosta(t, `{"numero_ordine_anticipato": true}`)
	daVerificare := motorePosta(t, `{"ordine_da_verificare": true}`)
	const richiesta = "Buongiorno, richiesta d'offerta per il 7120001: il nostro ordine n. 4500012345 è già emesso."
	const richiestaSenzaOrdine = "Buongiorno, richiesta d'offerta per il 7120001."
	casi := []struct {
		nome                string
		motore              *Motore
		oggetto, corpo      string
		allegati            []string
		evento, atto, forza string
		evidenze            []string
		senzaOrdine         string // il corpo senza il numero d'ordine: stesso evento, senza l'evidenza dell'ordine
	}{
		{"con la chiave e una parola di richiesta vince la richiesta", anticipato, "Codice 7120001", richiesta, nil,
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaProbabile, []string{"«richiesta d'offerta»", "«ordine n. 4500012345»", "numero_ordine_anticipato"},
			richiestaSenzaOrdine},
		{"con la chiave e «SI CTE» del portale", anticipato, "SI CTE 12345 26999999 - codici nuovi", "Ordine n. 4500012345 per i codici in elenco.", nil,
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaProbabile, []string{"l'oggetto dice «si cte»", "«ordine n. 4500012345»"}, ""},
		// «SI CTE» accanto all'ordine vale come le altre parole di richiesta anche per la formula di chiusura di E5
		// (4.13): la CTE che chiude con «Resto in attesa di Vs. riscontro» non è un sollecito
		{"con la chiave, «SI CTE» e la formula di chiusura", anticipato, "SI CTE 12345 26999999 - codici nuovi",
			"Ordine n. 4500012345 per i codici in elenco.\nResto in attesa di Vs. riscontro.", nil,
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaProbabile, []string{"l'oggetto dice «si cte»", "«ordine n. 4500012345»"}, ""},
		{"con la chiave e «commande» accanto a «devis»", anticipato, "Devis 7120001", "Bonjour, merci de nous envoyer un devis pour une commande de 20 pièces.", nil,
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaProbabile, []string{"«devis»", "«commande»"}, ""},
		{"con la chiave e i disegni la richiesta è chiara", anticipato, "Codice 7120001", richiesta, []string{"7120001A_1.stp"},
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaChiaro, []string{"1 file tecnico", "«ordine n. 4500012345»"}, richiestaSenzaOrdine},
		// la correzione 1: il sollecito e la revisione con un numero d'ordine e una parola di richiesta
		{"con la chiave un sollecito resta un sollecito", anticipato, "Sollecito RDO 12345", "Vi sollecitiamo l'offerta, nostro ordine n. 4500012345.", nil,
			EventoSollecito, AttoSollecito, ForzaProbabile, []string{"«sollecitiamo»", "«ordine n. 4500012345»", "numero_ordine_anticipato"},
			"Vi sollecitiamo l'offerta."},
		{"con la chiave una revisione resta una revisione", anticipato, "Revisione disegno 7120001",
			"Vi inviamo la nuova revisione per la richiesta d'offerta, ordine n. 4500012345.", []string{"7120001A_2.stp"},
			EventoRevisioneCAD, AttoRevisioneDocumenti, ForzaChiaro, []string{"7120001A_2.stp porta la rev 2", "«revisione»", "«ordine n. 4500012345»", "numero_ordine_anticipato"},
			"Vi inviamo la nuova revisione per la richiesta d'offerta."},
		{"con la chiave senza parola di richiesta resta un ordine", anticipato, "Codice 7120001", "Vi mandiamo l'ordine n. 4500012345 per 20 pezzi.", nil,
			EventoOrdine, AttoOrdine, ForzaChiaro, []string{"«ordine n. 4500012345»"}, ""},
		{"senza la chiave l'ordine è chiaro", nil, "Codice 7120001", richiesta, nil,
			EventoOrdine, AttoOrdine, ForzaChiaro, []string{"«ordine n. 4500012345»"}, ""},
		{"senza la chiave il sollecito con l'ordine è un ordine, come prima", nil, "Sollecito RDO 12345", "Vi sollecitiamo l'offerta, nostro ordine n. 4500012345.", nil,
			EventoOrdine, AttoOrdine, ForzaChiaro, []string{"«ordine n. 4500012345»"}, ""},
		{"senza la chiave «SI CTE» non è una richiesta", nil, "SI CTE 12345 26999999 - codici nuovi", "Ordine n. 4500012345 per i codici in elenco.", nil,
			EventoOrdine, AttoOrdine, ForzaChiaro, []string{"«ordine n. 4500012345»"}, ""},
		{"la voce più debole: l'ordine è probabile, e la richiesta si dice", daVerificare, "Codice 7120001", richiesta, nil,
			EventoOrdine, AttoOrdine, ForzaProbabile, []string{"«ordine n. 4500012345»", "ordine_da_verificare", "dice anche «richiesta d'offerta»"}, ""},
		{"la voce più debole senza parola di richiesta", daVerificare, "Codice 7120001", "Vi mandiamo l'ordine n. 4500012345 per 20 pezzi.", nil,
			EventoOrdine, AttoOrdine, ForzaProbabile, []string{"ordine_da_verificare"}, ""},
		// «commande» è già probabile: la voce non la tocca
		{"la voce più debole con «commande»", daVerificare, "Commande 7120001", "Bonjour, ci-joint notre commande de 20 pièces.", nil,
			EventoOrdine, AttoOrdine, ForzaProbabile, []string{"che da sola non basta"}, ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			in := IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", Mittente: "buyer@acme.example",
				Oggetto: c.oggetto, Corpo: c.corpo, NomiAllegati: c.allegati, Motore: c.motore}
			e := Evento(in)
			ev := strings.Join(e.Evidenze, " | ")
			if e.Evento != c.evento || e.Atto != c.atto || e.Forza != c.forza {
				t.Fatalf("%s/%s (%s), atteso %s/%s (%s): %s", e.Evento, e.Atto, e.Forza, c.evento, c.atto, c.forza, ev)
			}
			for _, x := range c.evidenze {
				if !strings.Contains(ev, x) {
					t.Errorf("evidenze %q: manca %q", ev, x)
				}
			}
			if d := EventoDa(ControparteCliente, "entrata", e.Atto, ""); d != e.Evento {
				t.Errorf("EventoDa rilegge %s, Evento dice %s", d, e.Evento)
			}
			if c.senzaOrdine == "" {
				return
			}
			in.Corpo = c.senzaOrdine
			s := Evento(in)
			if s.Evento != e.Evento || s.Atto != e.Atto || s.Forza != e.Forza {
				t.Errorf("senza il numero d'ordine %s/%s (%s), con il numero %s/%s (%s): l'ordine ha cambiato l'evento",
					s.Evento, s.Atto, s.Forza, e.Evento, e.Atto, e.Forza)
			}
			if sv := strings.Join(s.Evidenze, " | "); strings.Contains(sv, "4500012345") || len(s.Evidenze) != len(e.Evidenze)-1 {
				t.Errorf("senza il numero d'ordine le evidenze sono %q; con il numero %q", sv, ev)
			}
		})
	}
	// e Triage passa il motore all'evento: la stessa mail, dal triage, si salva come richiesta d'offerta
	tr := Triage(daMittente(anticipato, "buyer@acme.example", "Codice 7120001", richiesta))
	if tr.Evento != EventoNuovaRFQ || tr.Atto != AttoRichiestaOfferta {
		t.Errorf("Triage con la chiave: %s/%s", tr.Evento, tr.Atto)
	}
}

// Scelta 3 (domanda 22b = A): il numero d'ordine nella forma del cliente. Con la voce «ODA_0001234» non è
// un codice prodotto (né nella mail, né nel nome di un file) ed è una parola d'ordine; senza la voce (la
// controprova) è un codice generico come prima, e la mail non è un ordine.
func TestIlNumeroDOrdineDelCliente(t *testing.T) {
	const conOrdine = `{
	  "famiglie_codice": [{"regex": "7120\\d{3}", "descrizione": "codici ACME", "esempio": "7120001"}],
	  "numero_ordine": {"regex": "\\bODA_\\d{7}\\b", "descrizione": "ordini ODA", "esempio": "ODA_0001234"}
	}`
	m := motorePosta(t, conOrdine)
	senza := Compila("ACME S.p.A.", regole.Regole{FamiglieCodice: m.Regole.FamiglieCodice})
	const oggetto = "ODA_0001234"
	const corpo = "In allegato il documento per 20 pezzi del 7120010."

	tr := Triage(daMittente(m, "buyer@acme.example", oggetto, corpo, "ODA_0001234.pdf"))
	if tr.Evento != EventoOrdine || tr.ForzaEvento != ForzaChiaro {
		t.Fatalf("evento %s (%s): %v", tr.Evento, tr.ForzaEvento, tr.EvidenzeEvento)
	}
	if ev := strings.Join(tr.EvidenzeEvento, " | "); !strings.Contains(ev, "«ODA_0001234», un numero d'ordine del cliente (ordini ODA)") {
		t.Errorf("evidenze: %q", ev)
	}
	for _, x := range tr.Estrazione.Codici {
		if strings.Contains(x.Codice, "0001234") {
			t.Errorf("il numero d'ordine è fra i codici: %+v", x)
		}
	}
	if !slices.Contains(tr.Codici, "7120010") {
		t.Errorf("il codice vero non c'è più: %v", tr.Codici)
	}
	// anche una famiglia senza confini, che dentro «ODA_7120001» vedrebbe 7120001, non lo prende
	if e := m.Estrai(Testo{Dove: "oggetto", Corpo: "ODA_7120001"}); len(e.Codici) != 0 {
		t.Errorf("un pezzo del numero d'ordine è un codice: %+v", e.Codici)
	}
	// Correzione 1 della 4.13b: l'ordine si toglie per posizione, non per testo. Il 7120001 citato fuori da
	// «ODA_7120001» resta un codice della famiglia; quello dentro l'ordine no (prima spariva anche il primo)
	e := m.Estrai(Testo{Dove: "corpo", Corpo: "ordine ODA_7120001 per il pezzo 7120001"})
	if got := SoloCodici(e.Proponibili()); len(got) != 1 || got[0] != "7120001" || len(e.Codici) != 1 {
		t.Errorf("il pezzo citato fuori dall'ordine: proponibili %v, codici %+v", got, e.Codici)
	}
	// il nome del file: niente codice, niente codice citato
	v := Valuta(IngressoFile{NomeFile: "ODA_0001234.pdf", Bytes: 1000, Direzione: "entrata", Motore: m})
	if v.Codice.Valore != "" || len(v.Codice.Evidenze) != 0 {
		t.Errorf("il nome «ODA_0001234.pdf» ha un codice: %+v", v.Codice)
	}
	// Correzione 1 della 4.13b: un nome che non è un codice e cita l'ordine. PropostaDaNome non ha il motore, e
	// lo mette fra i codici citati; CitatiNelNome (ciò che l'ingest scrive in `codici_nel_nome` e il Fascicolo
	// suggerisce) lo toglie, come la valutazione. Un codice citato fuori dall'ordine resta.
	for _, x := range []struct {
		nome   string
		citati []string
	}{
		{"Ordine ODA_0001234.pdf", nil},
		{"ODA_0001234 conferma.pdf", nil},
		{"Ordine ODA_0001234 per 7120010.pdf", []string{"7120010"}},
		{"ODA_7120001 7120001.pdf", []string{"7120001"}},
	} {
		grezzi := PropostaDaNome(x.nome, 1000, "entrata").CodiciNelNome
		if !slices.ContainsFunc(grezzi, func(c string) bool { return strings.HasPrefix(c, "ODA_") }) {
			t.Fatalf("%s: la prova vuole l'ordine fra i codici di PropostaDaNome: %v", x.nome, grezzi)
		}
		if got := m.CitatiNelNome(x.nome, grezzi); !slices.Equal(got, x.citati) {
			t.Errorf("%s: codici citati %v, attesi %v", x.nome, got, x.citati)
		}
		var citati []string
		for _, ev := range Valuta(IngressoFile{NomeFile: x.nome, Bytes: 1000, Direzione: "entrata", Motore: m}).Codice.Evidenze {
			citati = append(citati, ev.Testo)
		}
		if !slices.Equal(citati, x.citati) {
			t.Errorf("%s: evidenze del codice %v, attese %v", x.nome, citati, x.citati)
		}
		// la controprova: senza la voce l'ordine resta fra i codici citati, come prima
		if got := senza.CitatiNelNome(x.nome, grezzi); !slices.Equal(got, grezzi) {
			t.Errorf("%s senza la voce: %v, attesi %v", x.nome, got, grezzi)
		}
	}

	// la controprova: senza la voce è un codice generico, e la mail non è un ordine
	tr = Triage(daMittente(senza, "buyer@acme.example", oggetto, corpo, "ODA_0001234.pdf"))
	if tr.Evento == EventoOrdine {
		t.Errorf("senza la voce la mail è un ordine: %v", tr.EvidenzeEvento)
	}
	if !slices.Contains(SoloCodici(tr.Estrazione.Codici), "ODA_0001234") {
		t.Errorf("senza la voce il numero non si vede più come prima: %+v", tr.Estrazione.Codici)
	}
	if v := Valuta(IngressoFile{NomeFile: "ODA_0001234.pdf", Bytes: 1000, Direzione: "entrata", Motore: senza}); v.Codice.Valore != "ODA_0001234" {
		t.Errorf("senza la voce il nome: %+v", v.Codice)
	}
}

// Scelta 6: le parole chiave «PN», «P/N», «cod.» con i separatori «:», «.», «#» e «n.» stanno nella regex
// del cliente. Il motore prende il codice dal gruppo `codice` e lo attribuisce alla famiglia; senza la parola
// chiave la famiglia non lo vede (lo vede solo l'estrattore generico, che per un cliente con famiglie non
// propone). Nessun codice nuovo: la prova dice che quello che c'è basta.
func TestLeParoleChiaveDeiCodiciNellaRegexDelCliente(t *testing.T) {
	m := motorePosta(t, `{"famiglie_codice":[{"regex":"(?:\\bPN|\\bP/N|\\bcod\\.)\\s*(?:[:.#]|n\\.)?\\s*(?P<codice>7120\\d{3})\\b",
	  "descrizione":"ACME con la parola chiave","esempio":"PN: 7120001"}]}`)
	for _, testo := range []string{"PN: 7120001", "P/N # 7120001", "cod. n. 7120001", "PN.7120001", "vedi P/N:7120001 e basta"} {
		e := m.Estrai(Testo{Dove: "corpo", Corpo: testo})
		p := e.Proponibili()
		if len(p) != 1 || p[0].Codice != "7120001" || p[0].Origine != "famiglia" || p[0].Famiglia != "ACME con la parola chiave" {
			t.Errorf("%q: %+v", testo, e.Codici)
		}
	}
	e := m.Estrai(Testo{Dove: "corpo", Corpo: "il particolare 7120001 in allegato"})
	if len(e.Proponibili()) != 0 || len(DiFamiglia(e.Codici)) != 0 {
		t.Errorf("senza la parola chiave: %+v", e.Codici)
	}
	if !slices.Contains(SoloCodici(e.Altri()), "7120001") {
		t.Errorf("senza la parola chiave il numero non si vede più: %+v", e.Codici)
	}
}

// Scelta 7: il disegno (`.dft`) e la lamiera 3D (`.psm`) di Solid Edge sono tipi tecnici per tutti. Nella
// valutazione del file con le regole che c'erano (la tabella S1 non cambia), nella proposta dal nome, nella
// pre-spunta e fra gli allegati tecnici dell'evento. La controprova è un'estensione qualunque.
func TestIFormatiDiSolidEdgeSonoTecnici(t *testing.T) {
	casi := []struct{ nome, tipo, regola string }{
		{"7120001.dft", "disegno_2d", "ext_dwg"},
		{"7120001.DFT", "disegno_2d", "ext_dwg"},
		{"7120001.psm", "cad_3d", "ext_3d"},
		{"7120001.xyz", "altro", "ext_altro"},
	}
	for _, c := range casi {
		v := Valuta(IngressoFile{NomeFile: c.nome, Bytes: 1000, Direzione: "entrata"})
		if v.Tipo.Valore != c.tipo || v.Tipo.Regola != c.regola {
			t.Errorf("%s: tipo %s (%s), atteso %s (%s)", c.nome, v.Tipo.Valore, v.Tipo.Regola, c.tipo, c.regola)
		}
		ext := strings.ToLower(c.nome[strings.LastIndex(c.nome, ".")+1:])
		if len(v.Tipo.Evidenze) == 0 || v.Tipo.Evidenze[0].Testo != "."+ext {
			t.Errorf("%s: l'evidenza non dice l'estensione vera: %+v", c.nome, v.Tipo.Evidenze)
		}
		p := PropostaDaNome(c.nome, 1000, "entrata")
		tecnico := c.tipo != "altro"
		if p.Tipo != c.tipo || p.PreSpunta != tecnico {
			t.Errorf("%s: proposta %s, pre-spunta %v", c.nome, p.Tipo, p.PreSpunta)
		}
		if possibileTecnico(ext) != tecnico {
			t.Errorf("%s: possibileTecnico %v", c.nome, !tecnico)
		}
		// la prima mail con il solo file: una richiesta probabile se è tecnico, niente se non lo è
		e := Evento(IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", Oggetto: "7120001", Corpo: "In allegato.", NomiAllegati: []string{c.nome}})
		if (e.Evento == EventoNuovaRFQ) != tecnico {
			t.Errorf("%s: evento %s %v", c.nome, e.Evento, e.Evidenze)
		}
	}
	// la tabella degli score è la stessa: nessuna regola nuova
	if Punteggi["ext_dwg"].Score != 85 || Punteggi["ext_3d"].Score != 95 || TabellaPunteggi != "S1" {
		t.Error("la tabella S1 è cambiata")
	}
}
