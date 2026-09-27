package classificazione

import (
	"strings"
	"testing"
)

// L1 — Smistamento M1 (A5.16.3): i candidati della posta a livelli, R0 verificato, R1 con il
// ConversationIndex, il pari merito. Codici e indirizzi inventati (ACME, @acme.example).

// 238 — la gerarchia di A5.16.3: sei livelli, uno score per tipo, e dalla riga del database si torna al
// tipo. Le righe scritte con le regole di prima si leggono verso il basso, mai più in alto.
func TestLaGerarchiaDelleEvidenzeHaSeiLivelli(t *testing.T) {
	attesi := []struct {
		tipo, regola string
		score        int
		livello      Livello
	}{
		{TipoR0InReplyTo, R0Reply, 98, MoltoForte},
		{TipoR0Cockpit, R0Reply, 97, MoltoForte},
		{TipoMarcatore, RegolaMarcatore, 97, MoltoForte},
		{TipoR0References, R0Reply, 96, MoltoForte},
		{TipoR0NonVerificato, R0Reply, 88, Forte},
		{TipoR1Forte, R1Conversazione, 86, Forte},
		{TipoR4Riferimento, R4Riferimento, 84, Forte},
		{TipoR3CodiceBuyer, R3Codice, 72, MedioForte},
		{TipoR3Codice, R3Codice, 62, Media},
		{TipoR1Indice, R1Conversazione, 45, Debole},
		{TipoR1Solo, R1Conversazione, 40, Debole},
		{TipoR2Oggetto, R2Oggetto, 20, MoltoDebole},
		{TipoR5Buyer, R5Buyer, 15, MoltoDebole},
	}
	tipi := TipiEvidenza()
	if len(tipi) != len(attesi) {
		t.Fatalf("%d tipi, attesi %d", len(tipi), len(attesi))
	}
	livelli := map[Livello]bool{}
	for i, a := range attesi {
		x := tipi[i]
		if x.Nome != a.tipo || x.Regola != a.regola || x.Score != a.score || x.Livello != a.livello {
			t.Errorf("riga %d: %+v, attesa %+v", i, x, a)
		}
		livelli[x.Livello] = true
		// dalla riga del database si torna al tipo (il marcatore non ha riga: si costruisce in lettura)
		if a.regola != RegolaMarcatore {
			if got := TipoDaRiga(a.regola, a.score); got != a.tipo {
				t.Errorf("TipoDaRiga(%s, %d) = %s, atteso %s", a.regola, a.score, got, a.tipo)
			}
		}
		// lo score è un ordine, non una probabilità: nessuna frase lo scrive come percentuale
		if strings.Contains(x.Livello.String(), "%") {
			t.Errorf("il livello %q contiene «%%»", x.Livello)
		}
		// l'ordine della tabella è quello degli score: il livello non smentisce mai lo score
		if i > 0 && (tipi[i-1].Score < x.Score || tipi[i-1].Livello < x.Livello) {
			t.Errorf("la tabella non scende: %+v prima di %+v", tipi[i-1], x)
		}
	}
	if len(livelli) != 6 {
		t.Errorf("livelli distinti: %d, attesi 6", len(livelli))
	}
	for l, s := range map[Livello]string{MoltoForte: "molto forte", Forte: "forte", MedioForte: "medio-forte", Media: "media", Debole: "debole", MoltoDebole: "molto debole"} {
		if l.String() != s {
			t.Errorf("%d: %q, atteso %q", l, l.String(), s)
		}
	}
	if MedioForte.Chiave() != "medio_forte" || MoltoDebole.Chiave() != "molto_debole" {
		t.Errorf("chiavi: %q %q", MedioForte.Chiave(), MoltoDebole.Chiave())
	}
	// PuntiRegola è la variante più forte di ogni regola
	for r, p := range map[string]int{R0Reply: 98, R1Conversazione: 86, R4Riferimento: 84, R3Codice: 72, R2Oggetto: 20, R5Buyer: 15} {
		if PuntiRegola[r] != p {
			t.Errorf("PuntiRegola[%s] = %d, atteso %d", r, PuntiRegola[r], p)
		}
	}
	if SogliaEvidenza != 60 {
		t.Errorf("soglia %d, attesa 60", SogliaEvidenza)
	}
	// le righe di prima (A5.10): verso il basso, e riconosciute come tali
	prima := []struct {
		regola   string
		punti    int
		evidenza string
		tipo     string
	}{
		{R1Conversazione, 95, "la conversazione di Outlook è stata collegata a questa richiesta da un operatore", TipoR1Solo},
		{R0Reply, 98, "In-Reply-To punta a un messaggio già agganciato a questa richiesta (<x@acme.example>)", TipoR0NonVerificato},
		{R3Codice, 80, "il codice 7120001 è già un identificativo di questa richiesta", TipoR3Codice},
		{R2Oggetto, 55, "stesso oggetto", TipoR2Oggetto},
		{R5Buyer, 35, "stesso buyer", TipoR5Buyer},
		{R4Riferimento, 90, "il riferimento", TipoR4Riferimento},
	}
	for _, p := range prima {
		tipo, diPrima := LeggiRiga(p.regola, p.punti, p.evidenza)
		if tipo != p.tipo || !diPrima {
			t.Errorf("riga di prima %s %d: %s (di prima %v), atteso %s", p.regola, p.punti, tipo, diPrima, p.tipo)
		}
		x, _ := Tipo(tipo)
		if x.Score > p.punti {
			t.Errorf("riga di prima %s %d letta PIÙ IN ALTO (%d)", p.regola, p.punti, x.Score)
		}
	}
	// una riga nuova a 98 non è di prima
	if tipo, diPrima := LeggiRiga(R0Reply, 98, "In-Reply-To punta alla mail del 12/09 di questa richiesta"); tipo != TipoR0InReplyTo || diPrima {
		t.Errorf("riga nuova a 98: %s, di prima %v", tipo, diPrima)
	}
	// la confidenza del triage letta senza regola: 95 (R1 di prima) è debole, non «molto forte»
	for score, l := range map[int]Livello{98: MoltoForte, 86: Forte, 72: MedioForte, 62: Media, 40: Debole, 95: Debole, 80: Media, 55: MoltoDebole} {
		if got := LivelloDaScore(score); got != l {
			t.Errorf("LivelloDaScore(%d) = %s, atteso %s", score, got, l)
		}
	}
}

// 239 — tre indizi deboli non battono una prova. La RFQ A ha solo In-Reply-To verificato; la B ha il
// ConversationID, l'oggetto e il buyer. A è prima, B è debole: non si somma niente.
func TestTreIndiziDeboliNonBattonoUnaProva(t *testing.T) {
	c := []Candidato{
		NuovoCandidato("B", TipoR1Solo, "stessa conversazione", false),
		NuovoCandidato("B", TipoR2Oggetto, "stesso oggetto", false),
		NuovoCandidato("B", TipoR5Buyer, "stesso buyer", false),
		NuovoCandidato("A", TipoR0InReplyTo, "In-Reply-To", false),
	}
	g := RaggruppaEOrdina(c)
	if len(g) != 2 {
		t.Fatalf("%d RFQ, attese 2: %+v", len(g), g)
	}
	if g[0].ThreadID != "A" || g[0].Rango != 1 || g[0].Livello != MoltoForte || g[0].Score != 98 {
		t.Errorf("prima: %+v", g[0])
	}
	if g[1].ThreadID != "B" || g[1].Rango != 2 || g[1].Livello != Debole || g[1].Score != 40 || len(g[1].Evidenze) != 3 {
		t.Errorf("seconda: %+v", g[1])
	}
	// le evidenze di B sono in ordine, la più forte in testa
	if g[1].Evidenze[0].TipoDi() != TipoR1Solo || g[1].Evidenze[2].TipoDi() != TipoR5Buyer {
		t.Errorf("evidenze di B: %+v", g[1].Evidenze)
	}
	if PariMerito(g) {
		t.Error("una prova contro tre indizi non è un pari merito")
	}
	if k, ok := MiglioreCandidatoAperto(c); !ok || k.ThreadID != "A" {
		t.Errorf("proposta: %+v %v", k, ok)
	}
	// a parità di livello e di score, due evidenze indipendenti vengono prima di una
	d := RaggruppaEOrdina([]Candidato{
		NuovoCandidato("uno", TipoR3Codice, "codice", false),
		NuovoCandidato("due", TipoR3Codice, "codice", false),
		NuovoCandidato("due", TipoR1Indice, "indice", false),
	})
	if d[0].ThreadID != "due" || d[0].NTipi() != 2 {
		t.Errorf("a pari score vince chi ha più tipi distinti: %+v", d)
	}
	// e gli indizi molto deboli non contano come tipi in più
	e := RaggruppaEOrdina([]Candidato{
		NuovoCandidato("uno", TipoR3Codice, "codice", false),
		NuovoCandidato("uno", TipoR1Solo, "conversazione", false),
		NuovoCandidato("due", TipoR3Codice, "codice", false),
		NuovoCandidato("due", TipoR2Oggetto, "oggetto", false),
		NuovoCandidato("due", TipoR5Buyer, "buyer", false),
	})
	if e[0].ThreadID != "uno" {
		t.Errorf("due indizi molto deboli contano più di un indizio debole: %+v", e)
	}
}

// 240 — a pari livello fra le prime due RFQ aperte, nessuna si propone (P37). L'esito resta «aggancia»,
// senza candidato, con il motivo, e fra i motivi non c'è l'evidenza di una sola delle due (in nessun
// ramo: cliente, fornitore, sconosciuto). Il controllo: con una sola RFQ la proposta c'è.
func TestAPariLivelloNessunaRFQSiPropone(t *testing.T) {
	in := IngressoTriage{Oggetto: "R: RFQ 7120001", Corpo: "Ecco i disegni.", Direzione: "entrata",
		Controparte: ControparteCliente, ClienteNoto: true, BuyerNoto: true, Mittente: "buyer@acme.example",
		Candidati: []Candidato{
			NuovoCandidato("T1", TipoR3CodiceBuyer, "codice 7120001 nella prima", false),
			NuovoCandidato("T2", TipoR3CodiceBuyer, "codice 7120001 nella seconda", false),
		}}
	// evidenzaDiUna: fra i motivi c'è la frase di una sola RFQ, che a pari merito direbbe di quella ciò che
	// vale per tutte e due
	evidenzaDiUna := func(m []string) bool {
		return contieneMotivo(m, "nella prima") || contieneMotivo(m, "nella seconda")
	}
	if !PariMeritoFra(in.Candidati) {
		t.Fatal("due RFQ con R3 dello stesso buyer sono pari merito")
	}
	if k, ok := MiglioreCandidatoAperto(in.Candidati); ok {
		t.Errorf("a pari merito MiglioreCandidatoAperto propone %+v", k)
	}
	e := Triage(in)
	if e.Esito != "aggancia" || e.Candidato != nil || e.Confidenza != 72 {
		t.Errorf("pari merito: esito %s, candidato %+v, confidenza %d", e.Esito, e.Candidato, e.Confidenza)
	}
	if !contieneMotivo(e.Motivi, "stessa forza") || evidenzaDiUna(e.Motivi) {
		t.Errorf("i motivi del pari merito: %v", e.Motivi)
	}
	// anche nei rami fornitore e sconosciuto nessuna delle due si propone, né coi motivi
	for _, c := range []string{ControparteFornitore, ControparteSconosciuto} {
		x := in
		x.Controparte, x.ClienteNoto = c, false
		if e := Triage(x); e.Candidato != nil || e.Esito != "aggancia" || !contieneMotivo(e.Motivi, "stessa forza") || evidenzaDiUna(e.Motivi) {
			t.Errorf("%s a pari merito: esito %s, candidato %+v, motivi %v", c, e.Esito, e.Candidato, e.Motivi)
		}
	}
	// una RFQ chiusa non fa pari merito con un'aperta: la chiusa si vede, l'aperta si propone
	chiusa := in
	chiusa.Candidati = []Candidato{in.Candidati[0], NuovoCandidato("T2", TipoR3CodiceBuyer, "codice", true)}
	if e := Triage(chiusa); e.Candidato == nil || e.Candidato.ThreadID != "T1" {
		t.Errorf("con l'altra chiusa: %+v", e.Candidato)
	}
	// due indizi deboli pari non sono una discordanza da segnalare
	if PariMeritoFra([]Candidato{NuovoCandidato("T1", TipoR1Solo, "", false), NuovoCandidato("T2", TipoR1Solo, "", false)}) {
		t.Error("il pari merito vale dal livello «media» in su")
	}
	// il controllo: con una sola RFQ la proposta c'è
	uno := in
	uno.Candidati = in.Candidati[:1]
	if e := Triage(uno); e.Candidato == nil || e.Candidato.ThreadID != "T1" || contieneMotivo(e.Motivi, "stessa forza") || !contieneMotivo(e.Motivi, "nella prima") {
		t.Errorf("con una sola RFQ: %+v %v", e.Candidato, e.Motivi)
	}
	for _, c := range []string{ControparteFornitore, ControparteSconosciuto} {
		x := uno
		x.Controparte, x.ClienteNoto = c, false
		if e := Triage(x); e.Candidato == nil || !contieneMotivo(e.Motivi, "nella prima") {
			t.Errorf("%s con una sola RFQ: %+v %v", c, e.Candidato, e.Motivi)
		}
	}
	// livelli diversi: si propone la più forte, anche se l'altra ha più evidenze
	diversi := in
	diversi.Candidati = []Candidato{in.Candidati[0], NuovoCandidato("T2", TipoR3Codice, "codice", false), NuovoCandidato("T2", TipoR1Solo, "conv", false)}
	if e := Triage(diversi); e.Candidato == nil || e.Candidato.ThreadID != "T1" {
		t.Errorf("livelli diversi: %+v", e.Candidato)
	}
}

// 241 — il ConversationIndex: 44 caratteri di intestazione, 10 per ogni risposta. Solo un indice che
// comincia con quello del padre ed è più lungo di blocchi interi ne discende.
func TestIlConversationIndexDiUnaRispostaDiscende(t *testing.T) {
	intest := strings.Repeat("01D2A3B4C5", 4) + "E6F7" // 44
	risposta := intest + "0000A1B2C3"
	seconda := risposta + "00001F2E3D"
	if len(intest) != 44 || len(risposta) != 54 {
		t.Fatalf("misure di prova sbagliate: %d %d", len(intest), len(risposta))
	}
	if EUnaRisposta(intest) {
		t.Error("44 caratteri: la sola intestazione non è una risposta")
	}
	if !EUnaRisposta(risposta) || !EUnaRisposta(seconda) {
		t.Error("44 + 10k caratteri: è una risposta")
	}
	if !DiscendeDa(risposta, intest) || !DiscendeDa(seconda, intest) || !DiscendeDa(seconda, risposta) {
		t.Error("il prefisso giusto discende")
	}
	if DiscendeDa(intest, risposta) || DiscendeDa(risposta, risposta) {
		t.Error("un padre non discende dal figlio, né un indice da se stesso")
	}
	altro := strings.Repeat("99", 22)
	if DiscendeDa(altro+"0000A1B2C3", intest) {
		t.Error("il prefisso sbagliato non discende")
	}
	if !DiscendeDa(strings.ToLower(risposta), intest) || !DiscendeDa(" "+risposta+" ", strings.ToLower(intest)) {
		t.Error("maiuscole, minuscole e spazi ai bordi sono indifferenti")
	}
	if DiscendeDa(intest+"0000A1B", intest) || EUnaRisposta(intest+"0000A1B") {
		t.Error("+7 caratteri non è un blocco intero")
	}
	if DiscendeDa(intest+"ZZZZZZZZZZ", intest) || EUnaRisposta(intest+"ZZZZZZZZZZ") {
		t.Error("un valore non esadecimale non discende da niente")
	}
	if DiscendeDa("", intest) || DiscendeDa(risposta, "") || EUnaRisposta("") {
		t.Error("un indice vuoto non è niente")
	}
}

// 242 — R1 sale solo con l'indice E il cliente: il solo ConversationID è debole, l'indice di un altro
// cliente anche; forte solo tutti e due.
func TestR1SiAlzaSoloConIndiceECliente(t *testing.T) {
	padre := strings.Repeat("A1", 22)
	figlio := padre + "0000B2C3D4"
	if got := TipoR1("", []string{padre}, true); got != TipoR1Solo {
		t.Errorf("conversazione sola, senza indice: %s", got)
	}
	if got := TipoR1(strings.Repeat("C3", 22), []string{padre}, true); got != TipoR1Solo {
		t.Errorf("indice di sola intestazione (conversazione per oggetto): %s", got)
	}
	if got := TipoR1(figlio, []string{padre}, false); got != TipoR1Indice {
		t.Errorf("indice che discende, cliente diverso: %s", got)
	}
	if got := TipoR1(figlio, []string{"", strings.Repeat("F0", 22), padre}, true); got != TipoR1Forte {
		t.Errorf("indice che discende e stesso cliente: %s", got)
	}
	for tipo, livello := range map[string]Livello{TipoR1Solo: Debole, TipoR1Indice: Debole, TipoR1Forte: Forte} {
		if x, _ := Tipo(tipo); x.Livello != livello {
			t.Errorf("%s: %s", tipo, x.Livello)
		}
	}
}

// 243 — R0 è verificato solo se chi scrive era fra i partecipanti del messaggio citato (o il suo dominio,
// se non è pubblico) e il messaggio non è un inoltro.
func TestR0VerificatoVuoleIPartecipantiENonUnInoltro(t *testing.T) {
	partecipanti := []string{"commerciale@azienda.example", "Buyer@ACME.example", "collega@acme.example"}
	if ok, _ := R0Verificato("buyer@acme.example", partecipanti, false); !ok {
		t.Error("il mittente fra i destinatari: verificato")
	}
	if ok, _ := R0Verificato("altro.buyer@acme.example", partecipanti, false); !ok {
		t.Error("lo stesso dominio del cliente: verificato")
	}
	if ok, perche := R0Verificato("terzo@esterno.example", partecipanti, false); ok || !strings.Contains(perche, "non era fra i destinatari") {
		t.Errorf("un terzo: %v %q", ok, perche)
	}
	if ok, perche := R0Verificato("buyer@acme.example", partecipanti, true); ok || !strings.Contains(perche, "inoltro") {
		t.Errorf("un inoltro: %v %q", ok, perche)
	}
	// il dominio pubblico è vero (serve alla prova), gli indirizzi sono palesemente finti
	pubblico := []string{"buyer.prova@gmail.com"}
	if ok, _ := R0Verificato("altro.prova@gmail.com", pubblico, false); ok {
		t.Error("un dominio pubblico non basta")
	}
	if ok, _ := R0Verificato("", partecipanti, false); ok {
		t.Error("senza mittente non si verifica niente")
	}
	// l'inoltro si riconosce dall'oggetto o dal separatore in testa alla storia; una risposta a un
	// inoltro non è un inoltro
	casi := []struct {
		oggetto, corpo string
		inoltro        bool
	}{
		{"FW: RFQ 7120001", "", true},
		{"I: RFQ 7120001", "", true},
		{"WG: Anfrage 7120001", "", true},
		{"R: I: RFQ 7120001", "Ricevuto.", false},
		{"RFQ 7120001", "ti giro questa\n\n---------- Forwarded message ----------\nDa: Buyer <buyer@acme.example>\nOggetto: RFQ", true},
		{"R: RFQ 7120001", "Ok.\n\n-----Messaggio originale-----\nDa: Buyer <buyer@acme.example>\nInviato: lunedì\nOggetto: RFQ", false},
		{"RFQ 7120001", "richiesta nuova", false},
	}
	for _, c := range casi {
		if got := EInoltro(c.oggetto, c.corpo); got != c.inoltro {
			t.Errorf("EInoltro(%q) = %v, atteso %v", c.oggetto, got, c.inoltro)
		}
	}
}

// 252 (parte pura) — per quale RFQ è nata la nostra mail preparata dal Cockpit: una, due discordanti
// o nessuna. Il resto (pannello, GET senza scritture) è L4 in web.
func TestLOrigineDellaBozzaEUnaDueONessuna(t *testing.T) {
	for _, c := range []struct {
		thread, risposta string
		attese           []string
	}{
		{"T1", "", []string{"T1"}},
		{"", "T2", []string{"T2"}},
		{"T1", "T1", []string{"T1"}},
		{"T1", "T2", []string{"T1", "T2"}},
		{"", "", nil},
	} {
		got := OrigineDaBozza(c.thread, c.risposta)
		if strings.Join(got, ",") != strings.Join(c.attese, ",") {
			t.Errorf("OrigineDaBozza(%q, %q) = %v, attese %v", c.thread, c.risposta, got, c.attese)
		}
	}
	// due origini discordi sono due candidati marcatore pari: nessuno si propone
	if !PariMeritoFra([]Candidato{NuovoCandidato("T1", TipoMarcatore, "", false), NuovoCandidato("T2", TipoMarcatore, "", false)}) {
		t.Error("due origini discordi devono essere un pari merito")
	}
}
