// L1 — D-P7e-1 dell'utente dell'11/10, A: una riga aperta tipo_proposto = finito su un nodo che non è identificato come
// il target della RFQ non ha autorità di classificazione (direttiva di ratifica e chiusura controllata di A1c, §1 e §4).
// Le scene della direttiva, una per prova: (a) la foglia non target, (b) il sottoassieme non target, (c) il target
// confermato, (d) la RFQ con più target, (e) il target non determinabile (la radice candidata), (f) il tipo confermato
// legacy discordante, (g) il commerciale. Le prove fissano anche le due diagnostiche, con i codici scritti per esteso: un
// codice pubblicato non cambia nome (codici_diagnostica.go).
package valutazione_test

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_emendamenti_test.go.

// I codici delle due diagnostiche di D-P7e-1, scritti per esteso.
const (
	codiceFinitoFuoriTarget          = "bom.tipo_finito_fuori_target"
	codiceFinitoTargetNonDeterminato = "bom.tipo_finito_target_non_determinato"
)

// Gli ID delle scene di D-P7e-1: il secondo target e il suo documento, il componente deciso di #3, la seconda riga di #4
// e la copia dello STEP che la porta.
var (
	cSecondoTarget   = uid(0xe7f1)
	dStepSecondo     = uid(0xe7f2)
	cDecisoFinito    = uid(0xe7f3)
	rigaB4Seconda    = uid(0xe7f4)
	aStepCopia       = uid(0xe7f5)
	rifSecondoTarget = "componente:" + uid(0xe7f1).String()
)

// diagnosticheDelFinito: le diagnostiche di D-P7e-1 del thread (i due codici), in ordine.
func diagnosticheDelFinito(et valutazione.EsitoThread) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, d := range et.Diagnostiche {
		if d.Codice == codiceFinitoFuoriTarget || d.Codice == codiceFinitoTargetNonDeterminato {
			out = append(out, d)
		}
	}
	return out
}

// rifRigaProposta: il riferimento di una riga di componente_proposta in una diagnostica.
func rifRigaProposta(id uuid.UUID) string { return "componente_proposta:" + id.String() }

// scenaFinitoSu: la scena dei tre nodi (la BOM di lavoro del finito 7120100A, la radice #1, #2 deciso dalla conferma, #3
// con il figlio #4, le righe di #3 e #4 aperte) con la riga del nodo dato che propone «finito».
func scenaFinitoSu(t *testing.T, chiave string) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	rigaDi(t, &th, chiave).TipoProposto = testo("finito")
	return th
}

// TestDP7e1LaFogliaNonTarget (a): la riga aperta di #4, una foglia che non è la radice di nessuna struttura di un target,
// propone «finito»: non ha autorità, e #4 è uno sciolto della regola, nei nodi e nei previsti (i fabbisogni dello
// sciolto, senza il cad_3d bloccante del finito). La riga resta com'è nella fotografia, e la diagnostica la elenca con il
// prodotto e il nodo. Lo stato, gli assi e l'impronta del prodotto non dipendono dalla proposta: sono quelli della stessa
// scena senza la riga «finito».
func TestDP7e1LaFogliaNonTarget(t *testing.T) {
	th := scenaFinitoSu(t, "#4")
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if n := nodoBOMDi(t, p, rif4); tipoDi(n) != "sciolto/proposto_dalla_regola" {
		t.Errorf("la foglia non target con la riga «finito»: %s", tipoDi(n))
	}
	if got := previstiDelNodo(p.Documenti, rif4); !reflect.DeepEqual(got, []string{"disegno_2d"}) {
		t.Errorf("i previsti di #4 sono quelli dello sciolto: %v", got)
	}
	if r := rigaDi(t, &th, "#4"); r.TipoProposto == nil || *r.TipoProposto != "finito" || r.Stato != "aperta" {
		t.Errorf("la riga grezza è cambiata: %+v", r)
	}
	d := diagnosticheDelFinito(et)
	if len(d) != 1 || d[0].Codice != codiceFinitoFuoriTarget || d[0].Gravita != evidenze.GravitaAvviso ||
		!reflect.DeepEqual(d[0].Rif, []string{rifProdB, rif4, rifRigaProposta(rigaB4)}) {
		t.Errorf("la diagnostica della foglia: %+v", d)
	}
	senza := prodottoDi(t, esitoSmistamento(t, scenaTreNodi(t, true)), rifProdB)
	if assiDelProdotto(t, p) != assiDelProdotto(t, senza) || p.Impronta != senza.Impronta {
		t.Errorf("la proposta senza autorità ha cambiato gli assi o l'impronta del prodotto")
	}
}

// TestDP7e1IlSottoassiemeNonTarget (b): la riga aperta di #3, che ha il figlio #4, propone «finito»: #3 è un sottoassieme
// della regola, nei nodi e nei previsti, con la diagnostica; #4 resta lo sciolto della regola, senza diagnostica.
func TestDP7e1IlSottoassiemeNonTarget(t *testing.T) {
	et := esitoSmistamento(t, scenaFinitoSu(t, "#3"))
	p := prodottoDi(t, et, rifProdB)
	if n := nodoBOMDi(t, p, rifNodo3); tipoDi(n) != "sottoassieme/proposto_dalla_regola" {
		t.Errorf("il sottoassieme non target con la riga «finito»: %s", tipoDi(n))
	}
	if got := previstiDelNodo(p.Documenti, rifNodo3); !reflect.DeepEqual(got, []string{"disegno_2d"}) {
		t.Errorf("i previsti di #3 sono quelli del sottoassieme: %v", got)
	}
	if n := nodoBOMDi(t, p, rif4); tipoDi(n) != "sciolto/proposto_dalla_regola" {
		t.Errorf("#4: %s", tipoDi(n))
	}
	if d := diagnosticheDelFinito(et); len(d) != 1 || d[0].Codice != codiceFinitoFuoriTarget || d[0].Rif[1] != rifNodo3 {
		t.Errorf("la diagnostica del sottoassieme: %+v", d)
	}
}

// TestDP7e1IlTargetConfermato (c): la radice della BOM di lavoro è il target, identificato dalla fonte confermata con la
// radice registrata, e prende il componente del target (T-B4-38): il suo tipo è quello del componente, finito e
// confermato, qualunque cosa proponga la sua riga, e nessuna diagnostica. Con la riga che propone «sottoassieme» il tipo
// resta lo stesso: la decisione prevale sulla proposta (D3), come prima.
func TestDP7e1IlTargetConfermato(t *testing.T) {
	for _, proposto := range []string{"finito", "sottoassieme"} {
		th := scenaTreNodi(t, true)
		rigaDi(t, &th, "#1").TipoProposto = testo(proposto)
		et := esitoSmistamento(t, th)
		p := prodottoDi(t, et, rifProdB)
		if n := nodoBOMDi(t, p, nodoB("#1")); tipoDi(n) != "finito/confermato" {
			t.Errorf("%s: la radice del target: %s", proposto, tipoDi(n))
		}
		if d := diagnosticheDelFinito(et); len(d) != 0 {
			t.Errorf("%s: nessuna diagnostica per il target: %+v", proposto, d)
		}
	}
}

// scenaDueTarget: la scena dei tre nodi con un secondo target nello stesso STEP: il finito manuale 7120300A (il nodo #3,
// con il figlio #4), con la sua fonte confermata sullo stesso contenuto (un suo documento cad_3d, lo
// step_strutturale_id, la vista presente_analizzato, la marcatura della radice sulla riga di #3). #3 è così la radice
// scelta della BOM di lavoro del secondo target e, senza una decisione propria, un nodo della BOM di lavoro del primo.
func scenaDueTarget(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	k := componente(cSecondoTarget, "7120300A", "finito", "manuale")
	d := dStepSecondo
	k.StepStrutturaleID = &d
	th.Componenti = append(th.Componenti, k)
	c := cSecondoTarget
	th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dStepSecondo, ThreadID: threadACME, ComponenteID: &c, Tipo: "cad_3d",
		NomeFile: "7120100A_1.stp", Estensione: "stp", Sha256: shaStepB, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME,
		Allegati: []uuid.UUID{aStepB}})
	th.StepProdotto = append(th.StepProdotto, fotorfq.RigaStepProdotto{ComponenteID: cSecondoTarget, Codice: "7120300A", StepStrutturaleID: &d,
		NStepCorrenti: 1, AnalisiCompleta: true, Esito: "presente_analizzato"})
	op := operatore
	rigaDi(t, &th, "#3").Marcatura = &fotorfq.MarcaturaStrutturale{Versione: 1, Ruolo: "radice", ComponenteID: &c, DocumentoID: &d,
		DichiaratoDa: &op, DichiaratoIl: "2026-10-01T08:00:00Z"}
	return th
}

// TestDP7e1PiuTarget (d): nella RFQ con due target nessuna unicità nuova. La riga «finito» di #3, la radice scelta della
// BOM di lavoro del secondo target, vale anche dove #3 è un nodo senza decisione della BOM di lavoro del primo
// (finito/proposto_dalla_riga): #3 è identificato come un target dalle decisioni pertinenti, anche se ha un padre; nella
// BOM di lavoro del secondo target #3 ha il suo componente, finito e confermato. La riga «finito» di #4, che non è la
// radice di nessuna struttura di un target, non vale né nell'una né nell'altra BOM di lavoro: la diagnostica è una per
// prodotto.
func TestDP7e1PiuTarget(t *testing.T) {
	th := scenaDueTarget(t)
	rigaDi(t, &th, "#3").TipoProposto = testo("finito")
	rigaDi(t, &th, "#4").TipoProposto = testo("finito")
	et := esitoSmistamento(t, th)
	p1, p2 := prodottoDi(t, et, rifProdB), prodottoDi(t, et, rifSecondoTarget)
	if len(p2.Nodi) == 0 {
		t.Fatalf("il secondo target non ha la BOM di lavoro (struttura %s)", p2.Struttura)
	}
	if n := nodoBOMDi(t, p1, rifNodo3); tipoDi(n) != "finito/proposto_dalla_riga" || n.Nodo.Decisione != nil {
		t.Errorf("#3 nella BOM di lavoro del primo target: %s", tipoDi(n))
	}
	if n := nodoBOMDi(t, p2, rifNodo3); tipoDi(n) != "finito/confermato" {
		t.Errorf("#3, la radice del secondo target: %s", tipoDi(n))
	}
	for _, p := range []valutazione.ProdottoValutato{p1, p2} {
		if n := nodoBOMDi(t, p, rif4); tipoDi(n) != "sciolto/proposto_dalla_regola" {
			t.Errorf("%s: #4 non target: %s", p.Rif, tipoDi(n))
		}
	}
	d := diagnosticheDelFinito(et)
	var prodotti []string
	for _, x := range d {
		if x.Codice != codiceFinitoFuoriTarget || x.Rif[1] != rif4 {
			t.Errorf("una diagnostica inattesa: %+v", x)
		}
		prodotti = append(prodotti, x.Rif[0])
	}
	if !reflect.DeepEqual(prodotti, []string{rifProdB, rifSecondoTarget}) && !reflect.DeepEqual(prodotti, []string{rifSecondoTarget, rifProdB}) {
		t.Errorf("le diagnostiche di #4, una per prodotto: %v", prodotti)
	}
}

// TestDP7e1IlTargetNonDeterminabile (e): senza la fonte confermata la struttura è candidata, e la sua radice #1 ha il
// codice del target: che sia il target non si determina (D4: la radice candidata non coincide automaticamente con il
// prodotto). La riga «finito» della radice non si applica: la radice prende la regola, come ogni radice candidata senza
// decisione (sottoassieme, perché ha dei figli), e la diagnostica è quella del caso ambiguo, che non dice che non è il
// target. Il prodotto, fra i confermati senza STEP, resta finito e confermato.
func TestDP7e1IlTargetNonDeterminabile(t *testing.T) {
	th := scenaCandidata(t)
	rigaDi(t, &th, "#1").TipoProposto = testo("finito")
	et := esitoSmistamentoCon(t, th, insiemeConMinuteria(t))
	p := prodottoDi(t, et, rifProdB)
	if len(p.StruttureCandidate) != 1 {
		t.Fatalf("strutture candidate %v", chiaviCandidate(p))
	}
	if r := nodoFra(t, p.StruttureCandidate[0].Nodi, nodoB("#1")); tipoDi(r) != "sottoassieme/proposto_dalla_regola" {
		t.Errorf("la radice candidata con la riga «finito»: %s", tipoDi(r))
	}
	if len(p.ConfermatiSenzaSTEP) == 0 || tipoDi(p.ConfermatiSenzaSTEP[0]) != "finito/confermato" {
		t.Errorf("il prodotto fra i confermati senza STEP: %v", rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	d := diagnosticheDelFinito(et)
	if len(d) != 1 || d[0].Codice != codiceFinitoTargetNonDeterminato ||
		!reflect.DeepEqual(d[0].Rif, []string{rifProdB, nodoB("#1"), rifRigaProposta(rigaRadB)}) {
		t.Errorf("la diagnostica del caso ambiguo: %+v", d)
	}
}

// TestDP7e1IlTipoConfermatoLegacy (f): un nodo deciso ha il tipo del suo componente, confermato, e la proposta della sua
// riga (decisa, quindi non aperta) non conta: lo sciolto resta sciolto anche con la riga che proponeva «finito», e un
// componente confermato come «finito» che non è un target (un dato legacy discordante: origine step, quindi non un
// target) resta finito e confermato. Nessuna decisione registrata si riscrive, e nessuna diagnostica di D-P7e-1.
func TestDP7e1IlTipoConfermatoLegacy(t *testing.T) {
	for _, tipo := range []string{"sciolto", "finito"} {
		th := scenaTreNodi(t, true)
		confermaComponente(&th, cDecisoFinito, "7120300A", cProdotto, 1)
		th.Componenti[len(th.Componenti)-1].Tipo = tipo
		op := operatore
		decidi(t, &th, "#3", "confermata", cDecisoFinito, &op)
		rigaDi(t, &th, "#3").TipoProposto = testo("finito")
		et := esitoSmistamento(t, th)
		p := prodottoDi(t, et, rifProdB)
		if n := nodoBOMDi(t, p, rifNodo3); tipoDi(n) != tipo+"/confermato" || n.Nodo.Decisione == nil {
			t.Errorf("il nodo deciso con il componente %s: %s", tipo, tipoDi(n))
		}
		if d := diagnosticheDelFinito(et); len(d) != 0 {
			t.Errorf("%s: nessuna diagnostica per un nodo deciso: %+v", tipo, d)
		}
	}
}

// TestDP7e1IlCommerciale (g): la riga aperta che propone «commerciale» vale come prima, anche per un nodo che non è il
// target. Con due righe aperte dello stesso nodo (lo stesso contenuto in un secondo allegato), la prima per ID «finito» e
// la seconda «commerciale», la prima non ha
// autorità e vale la seconda: #4 è commerciale/proposto_dalla_riga, nei nodi e nei previsti (il commerciale chiede il 2D
// come gli altri componenti, R103 C), e la diagnostica elenca la sola riga «finito».
func TestDP7e1IlCommerciale(t *testing.T) {
	th := scenaTreNodi(t, true)
	rigaDi(t, &th, "#4").TipoProposto = testo("commerciale")
	et := esitoSmistamento(t, th)
	if n := nodoBOMDi(t, prodottoDi(t, et, rifProdB), rif4); tipoDi(n) != "commerciale/proposto_dalla_riga" {
		t.Errorf("la riga «commerciale»: %s", tipoDi(n))
	}
	if d := diagnosticheDelFinito(et); len(d) != 0 {
		t.Errorf("nessuna diagnostica per il commerciale: %+v", d)
	}

	th = scenaFinitoSu(t, "#4")
	conAllegato(&th, allegato(aStepCopia, 9, "7120100A_1-copia.stp", "stp", shaStepB), nil)
	seconda := *rigaDi(t, &th, "#4")
	seconda.ID, seconda.AllegatoID, seconda.NomeFile, seconda.TipoProposto = rigaB4Seconda, aStepCopia, "7120100A_1-copia.stp", testo("commerciale")
	th.RigheComponenteProposta = append(th.RigheComponenteProposta, seconda)
	et = esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if n := nodoBOMDi(t, p, rif4); tipoDi(n) != "commerciale/proposto_dalla_riga" {
		t.Errorf("la seconda riga «commerciale» dopo la «finito»: %s", tipoDi(n))
	}
	if got := previstiDelNodo(p.Documenti, rif4); !reflect.DeepEqual(got, []string{"disegno_2d"}) {
		t.Errorf("i previsti di #4 commerciale: %v", got)
	}
	if d := diagnosticheDelFinito(et); len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{rifProdB, rif4, rifRigaProposta(rigaB4)}) {
		t.Errorf("la diagnostica elenca la sola riga «finito»: %+v", d)
	}
}

// Gli ID delle scene della revisione indipendente: la riga del secondo STEP e la riga «finito» della copia.
var (
	rigaStepB2Radice = uid(0xe7f6)
	rigaCopiaFinito  = uid(0xe7f7)
)

// conCopiaDelloSTEP: lo stesso STEP del prodotto in un secondo allegato, senza righe proprie: la sua struttura è una
// candidata, con gli stessi Rif della BOM di lavoro (lo stesso contenuto).
func conCopiaDelloSTEP(th *fotorfq.Thread) {
	conAllegato(th, allegato(aStepCopia, 9, "7120100A_1-copia.stp", "stp", shaStepB), nil)
}

// TestDP7e1LaDiagnosticaSoloDoveServe (revisione indipendente dell'11/10, R3): la diagnostica di D-P7e-1 esce solo per le
// caselle che chiedono il tipo proposto, cioè senza decisione e raggiunte. Il nodo #3 deciso nella BOM di lavoro, con una
// seconda riga aperta «finito» nella copia dello STEP, resta sciolto e confermato; nella candidata della copia lo stesso
// Rif non ha una decisione (la riga di quell'allegato è aperta), prende la regola, e la diagnostica è per lui, con la riga
// della copia. Un nodo raggiunto solo da un arco tolto da una persona è uscito dalla BOM: nessuna diagnostica.
func TestDP7e1LaDiagnosticaSoloDoveServe(t *testing.T) {
	th := scenaTreNodi(t, true)
	confermaComponente(&th, cDecisoFinito, "7120300A", cProdotto, 1)
	op := operatore
	decidi(t, &th, "#3", "confermata", cDecisoFinito, &op)
	conCopiaDelloSTEP(&th)
	seconda := *rigaDi(t, &th, "#3")
	seconda.ID, seconda.AllegatoID, seconda.NomeFile, seconda.Stato, seconda.ComponenteID, seconda.DecisoDa, seconda.DecisoIl =
		rigaCopiaFinito, aStepCopia, "7120100A_1-copia.stp", "aperta", nil, nil, nil
	seconda.TipoProposto = testo("finito")
	th.RigheComponenteProposta = append(th.RigheComponenteProposta, seconda)
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if n := nodoBOMDi(t, p, rifNodo3); tipoDi(n) != "sciolto/confermato" {
		t.Errorf("il nodo deciso nella BOM di lavoro: %s", tipoDi(n))
	}
	if len(p.StruttureCandidate) == 0 {
		t.Fatalf("la copia dello STEP non dà una candidata")
	}
	if n := nodoFra(t, p.StruttureCandidate[0].Nodi, rifNodo3); tipoDi(n) != "sottoassieme/proposto_dalla_regola" || n.Nodo.Decisione != nil {
		t.Errorf("#3 nella candidata della copia: %s", tipoDi(n))
	}
	if d := diagnosticheDelFinito(et); len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{rifProdB, rifNodo3, rifRigaProposta(rigaCopiaFinito)}) {
		t.Errorf("la diagnostica della casella senza decisione: %+v", d)
	}

	th = scenaTreNodiArcoTolto(t)
	rigaDi(t, &th, "#4").TipoProposto = testo("finito")
	if d := diagnosticheDelFinito(esitoSmistamento(t, th)); len(d) != 0 {
		t.Errorf("nessuna diagnostica per il nodo uscito dalla BOM: %+v", d)
	}
}

// TestDP7e1LeLettureDaRatificare (revisione indipendente dell'11/10, R4; D-CHIUS-2 = C [UR] dell'11/10): le letture di
// D-P7e-1 che la decisione fissa.
//   - 1 (C: «Il Rif identifica, ma non conferisce autorità alla struttura candidata»): la copia dello STEP dà una candidata
//     con la stessa radice #1 della BOM di lavoro. È lo stesso articolo, non la rappresentazione confermata: nella casella
//     della candidata la riga «finito» non vale, la radice prende la regola (sottoassieme, in via provvisoria) e la
//     diagnostica è quella del caso non determinato. Nella BOM di lavoro la radice resta finito e confermata.
//   - 2: la radice candidata di un secondo STEP di un prodotto che ha già la BOM di lavoro non è identificata (nessuna BOM
//     di lavoro sceglie quella radice): la regola e la diagnostica del caso non determinato.
//   - 3: prima della regola vale la riga aperta successiva: «finito» poi «sottoassieme» dà sottoassieme/proposto_dalla_riga.
func TestDP7e1LeLettureDaRatificare(t *testing.T) {
	th := scenaFinitoSu(t, "#1")
	conCopiaDelloSTEP(&th)
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if len(p.StruttureCandidate) == 0 {
		t.Fatalf("1: la copia dello STEP non dà una candidata")
	}
	if r := nodoFra(t, p.StruttureCandidate[0].Nodi, nodoB("#1")); tipoDi(r) != "sottoassieme/proposto_dalla_regola" {
		t.Errorf("1: la radice della candidata con lo stesso Rif della BOM di lavoro: %s", tipoDi(r))
	}
	if n := nodoBOMDi(t, p, nodoB("#1")); tipoDi(n) != "finito/confermato" {
		t.Errorf("1: la radice della BOM di lavoro: %s", tipoDi(n))
	}
	if d := diagnosticheDelFinito(et); len(d) != 1 || d[0].Codice != codiceFinitoTargetNonDeterminato ||
		!reflect.DeepEqual(d[0].Rif, []string{rifProdB, nodoB("#1"), rifRigaProposta(rigaRadB)}) {
		t.Errorf("1: la diagnostica del caso non determinato: %+v", d)
	}

	th = scenaTreNodi(t, true)
	conSecondoSTEP(t, &th)
	th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: rigaStepB2Radice, AllegatoID: aStepB2,
		NomeFile: "7120100A_2.stp", Sha256: shaStepB2, Chiave: "#1", IDGrezzo: "7120100A", Famiglia: "acme-catena", Fonte: "step", Stato: "aperta",
		TipoProposto: testo("finito")})
	et = esitoSmistamento(t, th)
	p = prodottoDi(t, et, rifProdB)
	radice2 := ancoraggio.RifNodo(shaStepB2, "#1")
	var trovata bool
	for _, c := range p.StruttureCandidate {
		if c.Radice == radice2 {
			trovata = true
			if r := nodoFra(t, c.Nodi, radice2); tipoDi(r) != "sottoassieme/proposto_dalla_regola" {
				t.Errorf("2: la radice candidata del secondo STEP: %s", tipoDi(r))
			}
		}
	}
	if !trovata {
		t.Fatalf("2: il secondo STEP non dà una candidata: %v", chiaviCandidate(p))
	}
	if d := diagnosticheDelFinito(et); len(d) != 1 || d[0].Codice != codiceFinitoTargetNonDeterminato || d[0].Rif[1] != radice2 {
		t.Errorf("2: la diagnostica del caso non determinato: %+v", d)
	}

	th = scenaFinitoSu(t, "#4")
	conCopiaDelloSTEP(&th)
	s := *rigaDi(t, &th, "#4")
	s.ID, s.AllegatoID, s.NomeFile, s.TipoProposto = rigaB4Seconda, aStepCopia, "7120100A_1-copia.stp", testo("sottoassieme")
	th.RigheComponenteProposta = append(th.RigheComponenteProposta, s)
	if n := nodoBOMDi(t, prodottoDi(t, esitoSmistamento(t, th), rifProdB), rif4); tipoDi(n) != "sottoassieme/proposto_dalla_riga" {
		t.Errorf("3: «finito» poi «sottoassieme»: %s", tipoDi(n))
	}
}
