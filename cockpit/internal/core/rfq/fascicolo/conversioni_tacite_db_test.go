//go:build integrazione

// L4 — A1d P4, B2.11 (R-14, K6; contratto rev. 3 §14, Δ41; piano B0 §2.1 e prova A17, parte delle conversioni):
// le tre conversioni di tipo tacite dei percorsi legacy diventano un rifiuto, salvo il campo di decisione esplicito.
// Contro PostgreSQL vero. Le strade:
//
//  1. relazione/accetta (AccettaRelazione): un particolare con uno STEP autorizzato non diventa assieme al primo
//     figlio accettato. Prima lo diventava da solo (E37, Smistamento F5b, A5.4.1; Prova 203,
//     TestUnParticolareAutorizzatoDiventaAssiemeAlPrimoFiglio in autorizzazione_db_test.go, da riscrivere: vedi il
//     resoconto della corsia D). Lo stesso per file/{aid}/accetta e il lotto con struttura[] (AccettaFile), che
//     passano dalla stessa funzione.
//  2. bom/applica, carta «n:» particolare con dei figli: pianifica la rifiuta, e l'applicazione non forza piu' il tipo.
//  3. bom/applica, tipoVoluto: un nodo proposto che lo STEP dice particolare e che la struttura vuole con dei figli
//     non nasce assieme in silenzio.
//
// Ogni rifiuto lascia la working com'era (b.bom) e le proposte come erano. Codici della famiglia di prova ACME 712.
// Il rifiuto del particolare viene dopo quello del ciclo: TestUnCicloVieneRifiutatoAllAccettazione
// (proposte_db_test.go) accetta un arco che chiuderebbe un ciclo sotto un particolare e si aspetta il ciclo
// (revisione della corsia D, A1).
//
// Dalla direttiva 12 (13/10/2026, D-K6-01 = A [U]) il rifiuto della strada 1 e' fascicolo.RichiestaConversione: per
// chi non sa chiedere la conferma e' ancora un Rifiuto con lo stesso testo (deveRifiutare), e il Fascicolo lo mostra
// come domanda per quel componente. Le prove dello Scenario G sono in fondo.
//
// Stato: eseguite e passate nell'integrazione della corsia D (13/10/2026, L4).

package fascicolo_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// particolareAutorizzato: il particolare 7120012 con il suo STEP autorizzato, che propone i figli diretti 7120021 e
// 7120022 ×4 (la scena della Prova 203).
func (b *banco) particolareAutorizzato() (uuid.UUID, db.Allegato) {
	b.t.Helper()
	b.acme()
	p := b.componente("7120012", db.TipoComponenteSciolto)
	doc, a := b.stepDelProdotto(p, "7120012", fattiSTEP{nodi: []string{"#1=7120012", "#2=7120021", "#3=7120022"}, archi: []string{"#1>#2", "#1>#3*4"}})
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: p, Documento: doc})
	return p, a
}

func (b *banco) tipoDi(comp uuid.UUID) string {
	return uno[string](b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, comp)
}

// Strada 1: relazione/accetta. Senza il campo l'arco si rifiuta, il particolare resta particolare, l'arco non c'e' e
// la proposta resta aperta. Una dichiarazione che nomina un altro componente non vale per questo (revisione della
// corsia D, M2). Con il campo che nomina il particolare (AccettaRelazioneDecisa, DecisioneTipo.Assiemi) l'arco entra e
// il particolare diventa assieme, con il messaggio che lo dice.
func TestRelazioneAccettaNonConverteIlParticolare(t *testing.T) {
	b := nuovoBanco(t)
	p, a := b.particolareAutorizzato()
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	prima, righe := b.bom(), b.relazioniProposte()
	_, err := b.accettaArco(a, "#1", "#2")
	deveRifiutare(t, err, "7120012 è un particolare, e il tipo non cambia da solo: se contiene dei pezzi, cambialo prima in assieme dalla sua scheda")
	if got := b.tipoDi(p); got != "sciolto" {
		t.Errorf("rifiutato l'arco, il particolare resta particolare: %s", got)
	}
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la working e' cambiata: %s → %s", prima, dopo)
	}
	if dopo := b.relazioniProposte(); dopo != righe {
		t.Errorf("le proposte di arco sono cambiate: %s → %s", righe, dopo)
	}

	// la dichiarazione nomina il suo componente: un id che non e' il padre non converte niente
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazioneDecisa(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"},
			b.utente, fascicolo.DecisioneTipo{Assiemi: []uuid.UUID{uuid.New()}})
	})
	deveRifiutare(t, err, "7120012 è un particolare, e il tipo non cambia da solo")
	if got := b.tipoDi(p); got != "sciolto" {
		t.Errorf("dichiarato un altro componente, il particolare resta particolare: %s", got)
	}

	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazioneDecisa(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"},
			b.utente, fascicolo.DecisioneTipo{Assiemi: []uuid.UUID{p}})
	})
	ok(t, err)
	if !strings.Contains(msg, "7120012 diventa un assieme") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.tipoDi(p); got != "sottoassieme" {
		t.Errorf("dichiarato assieme, il particolare diventa assieme: %s", got)
	}
	if got := b.bom(); !strings.Contains(got, "7120012>7120021*1") {
		t.Errorf("l'arco dichiarato: %s", got)
	}
}

// Strada 1, file/{aid}/accetta (e il lotto con struttura[], che chiama AccettaFile): il primo arco dalla sorgente
// particolare si rifiuta e con lui tutto il gesto, nodi compresi; con il campo (AccettaFileDeciso) entra tutto.
func TestAccettaFileNonConverteLaSorgenteParticolare(t *testing.T) {
	b := nuovoBanco(t)
	p, a := b.particolareAutorizzato()
	prima, righe := b.bom(), b.nodiProposti()+" | "+b.relazioniProposte()
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	})
	deveRifiutare(t, err, "7120012 è un particolare, e il tipo non cambia da solo")
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la working e' cambiata: %s → %s", prima, dopo)
	}
	if dopo := b.nodiProposti() + " | " + b.relazioniProposte(); dopo != righe {
		t.Errorf("le proposte sono cambiate: %s → %s", righe, dopo)
	}
	if got := b.tipoDi(p); got != "sciolto" {
		t.Errorf("il particolare resta particolare: %s", got)
	}

	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFileDeciso(b.ctx, q, b.thread, a.AllegatoID, b.utente, fascicolo.DecisioneTipo{Assiemi: []uuid.UUID{p}})
	})
	ok(t, err)
	if got := b.tipoDi(p); got != "sottoassieme" {
		t.Errorf("dichiarato assieme: %s", got)
	}
	if got := b.bom(); !strings.Contains(got, "7120012>7120021*1") || !strings.Contains(got, "7120012>7120022*4") {
		t.Errorf("i figli diretti dichiarati: %s", got)
	}
}

// prodottoConFoglia: il prodotto 7120001 con il suo STEP autorizzato, che propone un figlio diretto foglia 7120021
// (nel file non ha figli: lo STEP lo propone particolare). Restituisce il prodotto, l'allegato e la proposta di #2.
func (b *banco) prodottoConFoglia() (uuid.UUID, db.Allegato, uuid.UUID) {
	b.t.Helper()
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=7120021"}, archi: []string{"#1>#2"}}
	a := b.allegatoStep("7120001.stp", strings.Repeat("d2", 32))
	b.autorizza(prodotto, a, f.json())
	return prodotto, a, b.proposta("#2")
}

// la struttura voluta: prodotto → il nodo #2 → la carta «n:7120099» (particolare scritto dall'operatore).
func strutturaConNipote(prodotto uuid.UUID, a db.Allegato, nodo uuid.UUID) fascicolo.StrutturaVoluta {
	return fascicolo.StrutturaVoluta{Radice: prodotto,
		Archi: []fascicolo.ArcoVoluto{{Padre: "c:" + prodotto.String(), Figlio: "p:" + nodo.String(), Qta: 1},
			{Padre: "p:" + nodo.String(), Figlio: "n:7120099", Qta: 2}},
		RelazioniViste: []fascicolo.RelazioneVista{{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"}},
		Nuovi:          []fascicolo.ComponenteNuovo{{Codice: "7120099", Tipo: db.TipoComponenteSciolto}},
	}
}

// Strada 3: bom/applica, tipoVoluto. Il nodo #2, foglia nel file, con un figlio nella struttura: senza dichiarazione
// la struttura si rifiuta e niente cambia; con StrutturaVoluta.Assiemi 7120021 nasce assieme con il suo figlio.
func TestBomApplicaNonConverteIlNodoProposto(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, a, nodo := b.prodottoConFoglia()
	prima, righe := b.bom(), b.nodiProposti()+" | "+b.relazioniProposte()
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, strutturaConNipote(prodotto, a, nodo))
	})
	deveRifiutare(t, err, "7120021: nello STEP è un particolare")
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la working e' cambiata: %s → %s", prima, dopo)
	}
	if dopo := b.nodiProposti() + " | " + b.relazioniProposte(); dopo != righe {
		t.Errorf("le proposte sono cambiate: %s → %s", righe, dopo)
	}

	v := strutturaConNipote(prodotto, a, nodo)
	v.Assiemi = []uuid.UUID{nodo}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	})
	ok(t, err)
	got := b.bom()
	for _, atteso := range []string{"7120021:sottoassieme", "7120099:sciolto", "7120001>7120021*1", "7120021>7120099*2"} {
		if !strings.Contains(got, atteso) {
			t.Errorf("dichiarato assieme, manca %s: %s", atteso, got)
		}
	}
}

// Strada 2: bom/applica, carta «n:» particolare con un figlio: si rifiuta (pianifica), e nessun tipo si forza.
func TestBomApplicaLaCartaParticolareNonDiventaAssieme(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, _, _ := b.prodottoConFoglia()
	prima := b.bom()
	v := fascicolo.StrutturaVoluta{Radice: prodotto,
		Archi: []fascicolo.ArcoVoluto{{Padre: "c:" + prodotto.String(), Figlio: "n:7120098", Qta: 1},
			{Padre: "n:7120098", Figlio: "n:7120099", Qta: 1}},
		Nuovi: []fascicolo.ComponenteNuovo{{Codice: "7120098", Tipo: db.TipoComponenteSciolto}, {Codice: "7120099", Tipo: db.TipoComponenteSciolto}},
	}
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	})
	deveRifiutare(t, err, "7120098 è un particolare: sotto non ci va niente")
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la working e' cambiata: %s → %s", prima, dopo)
	}
	// la carta assieme, invece, entra con il suo tipo
	v.Nuovi[0].Tipo = db.TipoComponenteSottoassieme
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	})
	ok(t, err)
	if got := b.bom(); !strings.Contains(got, "7120098:sottoassieme") || !strings.Contains(got, "7120098>7120099*1") {
		t.Errorf("la carta assieme: %s", got)
	}
}

// Strade 2 e 3 insieme (revisione della corsia D, B5): la carta «n:7120021» assieme e il nodo #2, foglia nel file, con
// lo stesso codice e tutti e due con un figlio. Sono lo stesso codice nuovo: pianifica lo accetta per la carta, e
// l'applicazione deve far nascere assieme anche il nodo (pianoVoluta.Assieme), senza rifiutare quello che pianifica
// ha accettato. Senza dichiarazione: la carta e' una dichiarazione scritta dell'operatore, non una conversione.
func TestBomApplicaLaCartaAssiemeValeAnchePerIlNodoDelloStessoCodice(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, a, nodo := b.prodottoConFoglia()
	// il nodo sotto il prodotto (strutturaConNipote); la carta sotto un assieme nuovo 7120097, perche' il padre sia
	// un altro e l'arco non si sovrapponga a quello del nodo
	v := strutturaConNipote(prodotto, a, nodo)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: "c:" + prodotto.String(), Figlio: "n:7120097", Qta: 1},
		fascicolo.ArcoVoluto{Padre: "n:7120097", Figlio: "n:7120021", Qta: 3})
	v.Nuovi = append(v.Nuovi, fascicolo.ComponenteNuovo{Codice: "7120097", Tipo: db.TipoComponenteSottoassieme},
		fascicolo.ComponenteNuovo{Codice: "7120021", Tipo: db.TipoComponenteSottoassieme})
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	})
	ok(t, err)
	got := b.bom()
	for _, atteso := range []string{"7120021:sottoassieme", "7120021>7120099*2", "7120001>7120021*1", "7120097>7120021*3"} {
		if !strings.Contains(got, atteso) {
			t.Errorf("carta assieme e nodo dello stesso codice, manca %s: %s", atteso, got)
		}
	}
}

// ---------------------------------------------------------------- Scenario G (direttiva 12 §6 e §10)
//
// D-K6-01 = A [U] con la precisazione della direttiva 12: nessuna conversione silenziosa, ma una conferma
// contestuale per il singolo componente. Il gesto senza la dichiarazione risponde con fascicolo.RichiestaConversione
// (il componente, il suo codice, il figlio) e non scrive niente; la conferma rimanda lo stesso gesto con l'id di
// QUEL componente in DecisioneTipo.Assiemi, e conversione e arco entrano nella stessa transazione. Un rifiuto della
// persona non rimanda niente: lo stato resta quello di prima. Un errore strutturale (qui un ciclo) vince anche con la
// dichiarazione, e lo stato resta invariato. Stato: eseguite e passate nell'integrazione della corsia D.

// Accettazione atomica: la domanda nomina 7120012 e 7120021 e non scrive; dopo la conferma tipo e arco sono cambiati
// insieme, e la proposta di arco e' confermata. Il secondo figlio dello stesso file non chiede piu' niente: 7120012
// e' ormai un assieme.
func TestScenarioGConfermaContestualeAtomica(t *testing.T) {
	b := nuovoBanco(t)
	p, a := b.particolareAutorizzato()
	if err := b.accettaIn(a, "#2", "#3"); err != nil {
		t.Fatal(err)
	}
	prima, righe := b.bom(), b.relazioniProposte()
	_, err := b.accettaArco(a, "#1", "#2")
	var rc fascicolo.RichiestaConversione
	if !errors.As(err, &rc) {
		t.Fatalf("atteso fascicolo.RichiestaConversione, ottenuto %v", err)
	}
	if rc.Componente != p || rc.Codice != "7120012" || rc.Figlio != "7120021" {
		t.Errorf("la domanda nomina il componente e il figlio: %+v", rc)
	}
	if !strings.Contains(rc.Domanda(), "Il componente 7120012 è attualmente un particolare. Aggiungendo 7120021 come figlio, 7120012 diventerà un sottoassieme.") {
		t.Errorf("domanda: %q", rc.Domanda())
	}
	// il rifiuto della persona: nessun gesto parte, e la domanda non ha scritto niente
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la domanda ha cambiato la working: %s → %s", prima, dopo)
	}
	if dopo := b.relazioniProposte(); dopo != righe {
		t.Errorf("la domanda ha cambiato le proposte: %s → %s", righe, dopo)
	}

	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazioneDecisa(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"},
			b.utente, fascicolo.DecisioneTipo{Assiemi: []uuid.UUID{rc.Componente}})
	})
	ok(t, err)
	if !strings.Contains(msg, "7120012 → 7120021 ×1 entra nella BOM; 7120012 diventa un assieme.") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.tipoDi(p); got != "sottoassieme" {
		t.Errorf("confermato, il particolare diventa assieme: %s", got)
	}
	if got := b.bom(); !strings.Contains(got, "7120012:sottoassieme") || !strings.Contains(got, "7120012>7120021*1") {
		t.Errorf("tipo e arco insieme: %s", got)
	}
	if got := b.relazioniProposte(); !strings.Contains(got, "#1>#2*1:confermata") {
		t.Errorf("la proposta di arco confermata: %s", got)
	}

	msg, err = b.accettaArco(a, "#1", "#3")
	ok(t, err)
	if strings.Contains(msg, "diventa un assieme") || !strings.Contains(msg, "7120012 → 7120022 ×4 entra nella BOM.") {
		t.Errorf("il secondo figlio di un assieme non chiede e non converte: %q", msg)
	}
}

// Errore strutturale: X1 (assieme) contiene Y1 (particolare) e lo STEP autorizzato per Y1 propone Y1 → X1. Anche con
// Y1 dichiarato assieme l'arco chiuderebbe un ciclo: si rifiuta con il ciclo, e Y1 resta particolare, senza archi
// nuovi. Senza dichiarazione il gesto dice il ciclo e non chiede la conversione (la domanda non nasconde il ciclo).
func TestScenarioGErroreStrutturaleLasciaTuttoComEra(t *testing.T) {
	b := nuovoBanco(t)
	x := b.comp("X1", db.TipoComponenteSottoassieme)
	y := b.comp("Y1", db.TipoComponenteSciolto)
	b.arco(x, y, 1)
	f := fattiSTEP{nodi: []string{"#1=Y1", "#2=X1"}, archi: []string{"#1>#2"}}
	a := b.allegatoStep("rovescio.stp", strings.Repeat("e", 64))
	b.autorizza(y, a, f.json())
	b.accetta("#2")
	prima, righe := b.bom(), b.relazioniProposte()
	ciclo := fmt.Sprintf("chiuderebbe un ciclo (%s → %s → %s)", codiceDi("X1"), codiceDi("Y1"), codiceDi("X1"))

	_, err := b.accettaArco(a, "#1", "#2")
	var rc fascicolo.RichiestaConversione
	if errors.As(err, &rc) {
		t.Fatalf("un ciclo non diventa una domanda di conversione: %+v", rc)
	}
	deveRifiutare(t, err, ciclo)

	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazioneDecisa(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"},
			b.utente, fascicolo.DecisioneTipo{Assiemi: []uuid.UUID{y}})
	})
	deveRifiutare(t, err, ciclo)
	if got := b.tipoDi(y); got != "sciolto" {
		t.Errorf("con l'errore strutturale la conversione non entra: %s", got)
	}
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la working e' cambiata: %s → %s", prima, dopo)
	}
	if dopo := b.relazioniProposte(); dopo != righe {
		t.Errorf("le proposte sono cambiate: %s → %s", righe, dopo)
	}
}

// file/{aid}/accetta: la domanda arriva anche dal gesto sul file intero, e la transazione annullata non lascia
// nemmeno i nodi; con la conferma per 7120012 entrano nodi, archi e conversione in un colpo solo.
func TestScenarioGAccettaFileChiedePerLaSorgente(t *testing.T) {
	b := nuovoBanco(t)
	p, a := b.particolareAutorizzato()
	prima, righe := b.bom(), b.nodiProposti()+" | "+b.relazioniProposte()
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	})
	var rc fascicolo.RichiestaConversione
	if !errors.As(err, &rc) || rc.Componente != p {
		t.Fatalf("il file chiede la conferma per la sua sorgente 7120012: %v", err)
	}
	if b.bom() != prima || b.nodiProposti()+" | "+b.relazioniProposte() != righe {
		t.Errorf("la domanda non scrive niente, nodi compresi")
	}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFileDeciso(b.ctx, q, b.thread, a.AllegatoID, b.utente, fascicolo.DecisioneTipo{Assiemi: []uuid.UUID{rc.Componente}})
	})
	ok(t, err)
	if got := b.bom(); !strings.Contains(got, "7120012:sottoassieme") || !strings.Contains(got, "7120012>7120021*1") || !strings.Contains(got, "7120012>7120022*4") {
		t.Errorf("nodi, archi e conversione insieme: %s", got)
	}
}
