package fascicolo

// L1 — Smistamento F5 (addendum A5.4): lo STEP autorizzato per un componente, l'autorita' sui figli diretti e
// la guida, come regole pure. Le prove L4 degli stessi casi, con le tabelle, stanno in autorita_db_test.go.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// rigaDich e' una riga di ListDichiarazioniRfq: una sorgente del file sha per il componente c. Il documento
// del file e' corrente e associato a c; per un finito, lo STEP strutturale e' quel file.
func rigaDich(c db.Componente, sha, chiave, origine string) db.ListDichiarazioniRfqRow {
	doc := uuid.New()
	r := db.ListDichiarazioniRfqRow{PropostaID: uuid.New(), AllegatoID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(sha)), Sha256: sha,
		Chiave: chiave, Stato: db.StatoPropostaDuplicato, RigaComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true},
		DecisoDa: uuid.NullUUID{UUID: uuid.New(), Valid: true}, CreatoIl: time.Now(), Origine: origine, NomeFile: sha[:4] + ".stp",
		Componente: c, DocumentoID: uuid.NullUUID{UUID: doc, Valid: true}, DocumentoComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true}}
	if origine == OrigineSmistamento {
		m, _ := json.Marshal(Marcatura{V: 1, Ruolo: RuoloRadice, ComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true}})
		r.Marcatura = m
	}
	if c.Tipo == db.TipoComponenteFinito {
		r.StepSha256 = pgtype.Text{String: sha, Valid: true}
	}
	return r
}

func shaDi(s string) string { return strings.Repeat(s, 64)[:64] }

// marcaSospesa registra nella marcatura della riga la sospensione, come la scrivera' la fase T al passaggio a
// commerciale: {"sospesa": {"motivo": …, "tipo": "commerciale", "da": …, "il": …}}.
func marcaSospesa(r db.ListDichiarazioniRfqRow, c db.Componente) db.ListDichiarazioniRfqRow {
	m := Marcatura{V: 1, Ruolo: RuoloRadice, ComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true},
		Sospesa: &Sospensione{Motivo: "è diventato commerciale", Tipo: string(db.TipoComponenteCommerciale),
			Da: uuid.NullUUID{UUID: uuid.New(), Valid: true}, Il: "2026-09-27T21:00:00Z"}}
	r.Marcatura, _ = json.Marshal(m)
	return r
}

// Prova 197 (A5.4.3, I10, I12, P7): le regole di validita' di ValutaDichiarazioni, una per caso. Un
// componente, un'autorizzazione: due file validi per lo stesso componente sono un conflitto e nessuno dei due
// vale; superata, incoerente, P7 della forma di prima, sospesa per un archiviato; la marcatura prevale sulla
// forma di prima per lo stesso file. Sospesa anche per un componente commerciale (decisioni del 27/09 ter,
// Domanda 5 = B), e con una sospensione registrata nella marcatura qualunque sia il tipo di oggi: tornato
// sottoassieme, non si riattiva da sola (precisazione dell'utente del 27/09 sera). Le sospese non fermano il
// gate. Il ruolo «delega» non e' incoerente: e' la delega della Domanda 1 = B (i suoi casi in
// TestLaDelegaValeSoloConIlFileDelPadre).
func TestDueAutorizzazioniSulloStessoComponenteSonoUnConflitto(t *testing.T) {
	s := componente("7120010", db.TipoComponenteSottoassieme, false)
	p := componente("7120001", db.TipoComponenteFinito, false)
	altro := componente("7120099", db.TipoComponenteSciolto, false)

	valida := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)})
	if d, ok := valida.Di(s.ComponenteID); !ok || d.Sorgenti["#1"] != RuoloRadice || !valida.Autorizzato(shaDi("a"), "#1") || valida.Autorizzato(shaDi("a"), "#2") {
		t.Fatalf("una marcatura valida: %+v", valida)
	}

	casi := []struct {
		nome     string
		righe    []db.ListDichiarazioniRfqRow
		problema string // contenuto nella frase; "" = valida
		sospesa  bool
	}{
		{nome: "conflitto: due file per lo stesso componente", righe: []db.ListDichiarazioniRfqRow{
			rigaDich(s, shaDi("a"), "#1", OrigineSmistamento), rigaDich(s, shaDi("b"), "#1", OrigineSmistamento)}, problema: "conflitto: 7120010 ha 2 STEP autorizzati"},
		{nome: "superata: il documento e' sostituito", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.DocumentoSostituitoDa = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "superata"},
		{nome: "incoerente: il documento e' di un altro componente", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.DocumentoComponenteID = uuid.NullUUID{UUID: altro.ComponenteID, Valid: true}
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "il documento del file non è di 7120010"},
		{nome: "incoerente: niente documento", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.DocumentoID = uuid.NullUUID{}
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "non ha un documento"},
		{nome: "incoerente: finito marcato senza lo STEP strutturale su quel file (I10)", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(p, shaDi("a"), "#1", OrigineSmistamento)
			r.StepSha256 = pgtype.Text{String: shaDi("c"), Valid: true}
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "non dicono lo stesso file"},
		{nome: "incoerente: marcatura senza chi l'ha decisa", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.DecisoDa = uuid.NullUUID{}
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "nessuna persona l'ha deciso"},
		{nome: "incoerente: un ruolo che non si usa", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: "ereditata"})
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "«ereditata»"},
		{nome: "incoerente: una delega sul file del componente stesso", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: RuoloDelega})
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "non dice il file di un padre"},
		{nome: "P7: la radice di prima e' decisa come un altro componente", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(p, shaDi("a"), "#1", OrigineStepStrutturale)
			r.RigaComponenteID = uuid.NullUUID{UUID: altro.ComponenteID, Valid: true}
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "conflitto P7"},
		{nome: "forma di prima con la radice ancora aperta: vale, e resta aperta", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(p, shaDi("a"), "#1", OrigineStepStrutturale)
			r.Stato, r.RigaComponenteID, r.DecisoDa = db.StatoPropostaAperta, uuid.NullUUID{}, uuid.NullUUID{}
			return []db.ListDichiarazioniRfqRow{r}
		}()},
		{nome: "forma di prima con piu' radici e nessuna decisa come C", righe: func() []db.ListDichiarazioniRfqRow {
			r1, r2 := rigaDich(p, shaDi("a"), "#1", OrigineStepStrutturale), rigaDich(p, shaDi("a"), "#9", OrigineStepStrutturale)
			r1.Stato, r1.RigaComponenteID, r1.DecisoDa = db.StatoPropostaAperta, uuid.NullUUID{}, uuid.NullUUID{}
			r2.Stato, r2.RigaComponenteID, r2.DecisoDa = db.StatoPropostaAperta, uuid.NullUUID{}, uuid.NullUUID{}
			return []db.ListDichiarazioniRfqRow{r1, r2}
		}(), problema: "ha 2 radici"},
		{nome: "sospesa: il componente e' archiviato", righe: []db.ListDichiarazioniRfqRow{
			rigaDich(componente("7120012", db.TipoComponenteSottoassieme, true), shaDi("a"), "#1", OrigineSmistamento)}, problema: "sospesa", sospesa: true},
		// decisioni del 27/09 ter, Domanda 5 = B: lo STEP di un componente commerciale resta guida
		{nome: "sospesa: il componente e' commerciale", righe: []db.ListDichiarazioniRfqRow{
			rigaDich(componente("7120011", db.TipoComponenteCommerciale, false), shaDi("a"), "#1", OrigineSmistamento)}, problema: "è commerciale", sospesa: true},
		// precisazione dell'utente del 27/09 sera: tornato sottoassieme, la sospensione registrata tiene
		{nome: "sospesa: sospensione registrata su un sottoassieme", righe: []db.ListDichiarazioniRfqRow{
			marcaSospesa(rigaDich(s, shaDi("a"), "#1", OrigineSmistamento), s)}, problema: "è diventato commerciale (si riattiva solo con una scelta esplicita)", sospesa: true},
		{nome: "sospesa: sospensione registrata su un commerciale", righe: func() []db.ListDichiarazioniRfqRow {
			k := componente("7120011", db.TipoComponenteCommerciale, false)
			return []db.ListDichiarazioniRfqRow{marcaSospesa(rigaDich(k, shaDi("a"), "#1", OrigineSmistamento), k)}
		}(), problema: "si riattiva solo con una scelta esplicita", sospesa: true},
		{nome: "sospesa: sospensione registrata senza motivo, su un file altrimenti da rifare", righe: func() []db.ListDichiarazioniRfqRow {
			r := rigaDich(s, shaDi("a"), "#1", OrigineSmistamento)
			r.DocumentoComponenteID = uuid.NullUUID{UUID: altro.ComponenteID, Valid: true}
			r.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: RuoloRadice, Sospesa: &Sospensione{}})
			return []db.ListDichiarazioniRfqRow{r}
		}(), problema: "l'autorizzazione è sospesa", sospesa: true},
	}
	for _, cs := range casi {
		t.Run(cs.nome, func(t *testing.T) {
			d := ValutaDichiarazioni(cs.righe)
			if cs.problema == "" {
				if len(d.DaSistemare) != 0 || len(d.PerComponente) != 1 {
					t.Fatalf("doveva valere: %+v", d)
				}
				return
			}
			if len(d.PerComponente) != 0 || len(d.Sorgenti(shaDi("a"))) != 0 {
				t.Errorf("una dichiarazione che non vale non da' sorgenti: %+v", d.PerComponente)
			}
			elenco := d.DaSistemare
			if cs.sospesa {
				elenco = d.Sospese
				if len(d.DaSistemare) != 0 {
					t.Errorf("una sospesa non ferma il gate: %+v", d.DaSistemare)
				}
			}
			if len(elenco) == 0 || !strings.Contains(elenco[0].Problema, cs.problema) {
				t.Errorf("problema %+v, atteso che dica %q", elenco, cs.problema)
			}
		})
	}

	// la sospensione registrata si legge anche quando e' su una sola delle sorgenti del file (il raggruppamento)
	rr, rg := rigaDich(s, shaDi("a"), "R", OrigineSmistamento), marcaSospesa(rigaDich(s, shaDi("a"), "G", OrigineSmistamento), s)
	if d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rr, rg}); len(d.Sospese) != 1 || d.Sospese[0].Sospensione == nil ||
		d.Sospese[0].Sospensione.Tipo != string(db.TipoComponenteCommerciale) || len(d.PerComponente) != 0 {
		t.Errorf("sospensione su una sorgente sola: %+v", d)
	}

	// la marcatura prevale sulla forma di prima per lo stesso file e lo stesso componente: e' la stessa
	// dichiarazione, rifatta da una persona (una, non un conflitto)
	prima := rigaDich(p, shaDi("a"), "#1", OrigineStepStrutturale)
	marcata := rigaDich(p, shaDi("a"), "#1", OrigineSmistamento)
	d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{marcata, prima})
	if x, ok := d.Di(p.ComponenteID); !ok || x.Origine != OrigineSmistamento || len(d.DaSistemare) != 0 {
		t.Errorf("marcatura e forma di prima sullo stesso file: %+v", d)
	}
	// con piu' radici della forma di prima vale quella che una persona ha deciso come C
	r1, r2 := rigaDich(p, shaDi("a"), "#1", OrigineStepStrutturale), rigaDich(p, shaDi("a"), "#9", OrigineStepStrutturale)
	r2.Stato, r2.RigaComponenteID, r2.DecisoDa = db.StatoPropostaAperta, uuid.NullUUID{}, uuid.NullUUID{}
	d = ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{r1, r2})
	if x, ok := d.Di(p.ComponenteID); !ok || len(x.Sorgenti) != 1 || x.Sorgenti["#1"] == "" {
		t.Errorf("piu' radici, una decisa come C: %+v", d)
	}
}

// righeAutorita: il file del caso guida (7120001A_1.stp) con la radice #1, i figli diretti #2 (7120010), #3
// (7120012) e il nipote #4 (7120011, sotto 7120010).
func righeAutorita(sha string) (uuid.UUID, []db.ComponenteProposta, []db.RelazioneProposta) {
	all := uuid.New()
	nodo := func(k, codice string) db.ComponenteProposta {
		return db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: all, Sha256: sha, Chiave: k, Stato: db.StatoPropostaAperta,
			Codice: pgtype.Text{String: codice, Valid: true}, CreatoIl: time.Now()}
	}
	arco := func(p, f string) db.RelazioneProposta {
		return db.RelazioneProposta{AllegatoID: all, PadreChiave: p, FiglioChiave: f, Qta: 1, Stato: db.StatoPropostaAperta, CreatoIl: time.Now()}
	}
	return all, []db.ComponenteProposta{nodo("#1", "7120001A"), nodo("#2", "7120010"), nodo("#3", "7120012"), nodo("#4", "7120011")},
		[]db.RelazioneProposta{arco("#1", "#2"), arco("#1", "#3"), arco("#2", "#4")}
}

// Prova 193 (U3, Domanda 1 = B): le sorgenti autorizzano solo i figli diretti. Il file autorizzato per 7120001 ha
// autorita' sugli archi 7120001 → 7120010 e 7120001 → 7120012 e su quei due nodi; il nipote 7120011 (sotto
// 7120010) e il suo arco sono guida, anche se sono nello stesso file: l'autorita' non si eredita per
// profondita'. Un file senza autorizzazione e' tutto guida.
func TestLeSorgentiAutorizzanoSoloIFigliDiretti(t *testing.T) {
	p := componente("7120001", db.TipoComponenteFinito, false)
	sha := shaDi("a")
	all, nodi, archi := righeAutorita(sha)
	d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rigaDich(p, sha, "#1", OrigineSmistamento)})
	a := CalcolaAutorita(d, nodi, archi)
	if x, ok := a.Sorgente(all, "#1"); !ok || x.Componente.ComponenteID != p.ComponenteID {
		t.Fatalf("la radice e' la sorgente di 7120001: %+v %v", x, ok)
	}
	for _, k := range []string{"#2", "#3"} {
		if c, ok := a.FiglioDiretto(all, k); !ok || c != p.ComponenteID || !a.NodoNellAutorita(all, k) {
			t.Errorf("%s e' figlio diretto di 7120001: %v %v", k, c, ok)
		}
	}
	if _, ok := a.FiglioDiretto(all, "#4"); ok || a.NodoNellAutorita(all, "#4") {
		t.Errorf("il nipote 7120011 e' guida")
	}
	for k, atteso := range map[ChiaveRelazione]bool{
		{Allegato: all, Padre: "#1", Figlio: "#2"}: true, {Allegato: all, Padre: "#1", Figlio: "#3"}: true, {Allegato: all, Padre: "#2", Figlio: "#4"}: false} {
		if _, ok := a.ArcoAutorizzato(k); ok != atteso {
			t.Errorf("%v nell'autorita': %v, atteso %v", k, ok, atteso)
		}
	}
	// il codice uguale non da' niente: senza autorizzazione, anche con 7120001 nella BOM, tutto e' guida
	vuota := CalcolaAutorita(ValutaDichiarazioni(nil), nodi, archi)
	for _, n := range nodi {
		if vuota.NodoNellAutorita(all, n.Chiave) {
			t.Errorf("%s nell'autorita' senza nessuna autorizzazione", n.Chiave)
		}
	}
	// ComponenteDi: la sorgente e' C; un nodo aperto non e' niente; un aggancio per codice di prima non e' una
	// decisione (E08); uno deciso da una persona e' il suo componente
	if c, ok := a.ComponenteDi(nodi[0]); !ok || c != p.ComponenteID {
		t.Errorf("la sorgente e' il componente dell'autorizzazione: %v %v", c, ok)
	}
	if _, ok := a.ComponenteDi(nodi[1]); ok {
		t.Errorf("un figlio aperto non e' un componente")
	}
	legacy := nodi[1]
	legacy.Stato, legacy.ComponenteID = db.StatoPropostaDuplicato, uuid.NullUUID{UUID: uuid.New(), Valid: true}
	if _, ok := a.ComponenteDi(legacy); ok || !AgganciatoPerCodice(legacy) {
		t.Errorf("un duplicato senza chi l'ha deciso non e' una decisione")
	}
	legacy.DecisoDa = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	if c, ok := a.ComponenteDi(legacy); !ok || c != legacy.ComponenteID.UUID || AgganciatoPerCodice(legacy) {
		t.Errorf("deciso da una persona e' il suo componente: %v %v", c, ok)
	}
}

// Prova 194 (A5.4.2): un raggruppamento porta i suoi figli sotto il componente. La radice «Assembly_1» senza
// codice e il nodo di raggruppamento sono tutti e due sorgenti per C: l'arco radice → raggruppamento e' «stesso
// componente» (scartato da solo), e i figli del raggruppamento sono figli diretti di C.
func TestUnRaggruppamentoPortaISuoiFigliSottoIlComponente(t *testing.T) {
	b := nuovaBom("C", "X").arco("C", "X", 1)
	nodi, s := file([]string{"R"}, "R>G", "G>X", "G>Y")
	sorgenti := map[string]uuid.UUID{"R": b.id("C"), "G": b.id("C")}
	piano := Pianifica(nodi, s, Contesto{Working: b.working(), Sorgenti: sorgenti, Decisi: b.decisi("X"), Completa: true})
	r := perCoppia(piano)
	if x := r["R>G"]; x.Stato != db.StatoPropostaScartata || !strings.Contains(x.Nota, "stesso componente") {
		t.Errorf("radice → raggruppamento e' lo stesso componente: %+v", x)
	}
	if x := r["G>X"]; x.Stato != db.StatoPropostaDuplicato {
		t.Errorf("X e' figlio diretto di C (attraverso il raggruppamento), e C → X c'e': %+v", x)
	}
	if x := r["G>Y"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("Y e' un figlio diretto nuovo: %+v", x)
	}
	// e nell'autorita': il raggruppamento non e' un figlio, i suoi figli si'
	sha := shaDi("b")
	all := uuid.New()
	var righe []db.ComponenteProposta
	for _, k := range []string{"R", "G", "X", "Y"} {
		righe = append(righe, db.ComponenteProposta{AllegatoID: all, Sha256: sha, Chiave: k, Stato: db.StatoPropostaAperta})
	}
	archi := []db.RelazioneProposta{{AllegatoID: all, PadreChiave: "R", FiglioChiave: "G"}, {AllegatoID: all, PadreChiave: "G", FiglioChiave: "X"},
		{AllegatoID: all, PadreChiave: "G", FiglioChiave: "Y"}}
	rr, rg := rigaDich(b.comp["C"], sha, "R", OrigineSmistamento), rigaDich(b.comp["C"], sha, "G", OrigineSmistamento)
	rg.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: RuoloRaggruppamento})
	a := CalcolaAutorita(ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rr, rg}), righe, archi)
	if _, ok := a.FiglioDiretto(all, "G"); ok {
		t.Errorf("il raggruppamento e' C, non un suo figlio")
	}
	for _, k := range []string{"X", "Y"} {
		if _, ok := a.FiglioDiretto(all, k); !ok {
			t.Errorf("%s e' figlio diretto di C attraverso il raggruppamento", k)
		}
	}
}

// Prova 195 (A5.4.4): fuori dall'autorita' nessun arco si chiude da solo. Con i nodi decisi da una persona e
// l'arco uguale nella working, un arco che non parte da una sorgente resta aperto (e senza i conti); un
// arco dalla sorgente verso un figlio NON deciso resta aperto anche se il codice del figlio e' quello di un
// componente con l'arco nella working; solo verso un figlio deciso si tengono i conti.
func TestFuoriDallAutoritaNessunArcoSiChiudeDaSolo(t *testing.T) {
	b := nuovaBom("P", "A", "B").arco("P", "A", 1).arco("A", "B", 2)
	nodi, s := file([]string{"P"}, "P>A", "A>B*2")
	// senza sorgenti: tutto aperto, anche con A e B decisi
	r := perCoppia(Pianifica(nodi, s, Contesto{Working: b.working(), Decisi: b.decisi("P", "A", "B"), Completa: true}))
	for k, x := range r {
		if x.Stato != db.StatoPropostaAperta || x.Evidenza["qta_working"] != nil {
			t.Errorf("%s senza autorizzazione: %+v", k, x)
		}
	}
	// con la sorgente P e il figlio A non deciso: aperto, nonostante il codice e l'arco nella working
	r = perCoppia(Pianifica(nodi, s, Contesto{Working: b.working(), Sorgenti: map[string]uuid.UUID{"P": b.id("P")}, Completa: true}))
	if x := r["P>A"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("P → A con A non deciso: %+v", x)
	}
	// con A deciso: la tenuta dei conti; A → B resta guida anche con B deciso
	r = perCoppia(Pianifica(nodi, s, Contesto{Working: b.working(), Sorgenti: map[string]uuid.UUID{"P": b.id("P")}, Decisi: b.decisi("A", "B"), Completa: true}))
	if x := r["P>A"]; x.Stato != db.StatoPropostaDuplicato {
		t.Errorf("P → A con A deciso e l'arco nella working: %+v", x)
	}
	if x := r["A>B"]; x.Stato != db.StatoPropostaAperta || x.Evidenza["qta_working"] != nil {
		t.Errorf("A → B e' guida: %+v", x)
	}
}

// Prova 196 (A5.4.6, D77): le rimozioni guardano solo i figli diretti. Il file di P dice P → A; la working ha
// P → A, P → X e A → Y: si propone di togliere P → X, non A → Y (su A comanda lo STEP di A, se ne ha uno).
func TestLeRimozioniGuardanoSoloIFigliDiretti(t *testing.T) {
	b := nuovaBom("P", "A", "X", "Y").arco("P", "A", 1).arco("P", "X", 3).arco("A", "Y", 1)
	rim := RimozioniFigliDiretti(b.id("P"), b.working().Archi, map[Arco]bool{{Padre: b.id("P"), Figlio: b.id("A")}: true})
	if len(rim) != 1 || rim[0].Padre != b.id("P") || rim[0].Figlio != b.id("X") || rim[0].QtaWorking != 3 {
		t.Errorf("rimozioni = %+v, attesa solo P → X", rim)
	}
	if rim := RimozioniFigliDiretti(b.id("A"), b.working().Archi, nil); len(rim) != 1 || rim[0].Figlio != b.id("Y") {
		t.Errorf("il file di A, se non contiene Y, propone A → Y: %+v", rim)
	}
}

// La guida in una riga (A5.3.11, A5.4.5): i file senza autorizzazione, e i figli che restano guida sotto un
// figlio diretto gia' deciso. E il riesame conta solo l'autorita' (prova 115, la parte pura).
func TestLaGuidaInUnaRigaEIlRiesameSullAutorita(t *testing.T) {
	p := componente("7120001", db.TipoComponenteFinito, false)
	s := componente("7120010", db.TipoComponenteSottoassieme, false)
	sha := shaDi("a")
	_, nodi, archi := righeAutorita(sha)
	nodi[1].Stato, nodi[1].ComponenteID, nodi[1].DecisoDa = db.StatoPropostaConfermata, uuid.NullUUID{UUID: s.ComponenteID, Valid: true},
		uuid.NullUUID{UUID: uuid.New(), Valid: true}
	_, altri, altriArchi := righeAutorita(shaDi("b")) // un secondo STEP, non autorizzato
	a := CalcolaAutorita(ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rigaDich(p, sha, "#1", OrigineSmistamento)}),
		append(append([]db.ComponenteProposta{}, nodi...), altri...), append(append([]db.RelazioneProposta{}, archi...), altriArchi...))
	g := a.LaGuida(append(append([]db.ComponenteProposta{}, nodi...), altri...), append(append([]db.RelazioneProposta{}, archi...), altriArchi...),
		map[uuid.UUID]string{s.ComponenteID: "7120010"})
	if g.FileSenzaAutorizzazione != 1 || g.FigliDi["7120010"] != 1 || g.Nodi != 5 {
		t.Errorf("guida: %+v", g)
	}
	if f := g.Frase(); !strings.Contains(f, "1 STEP analizzato non è autorizzato per nessun componente") || !strings.Contains(f, "7120010 ha 1 figlio che resta guida") {
		t.Errorf("frase: %q", f)
	}
	// il riesame: dopo la baseline sono nati il figlio diretto #3 (aperto) e i suoi archi; la guida non conta
	prima := time.Now().Add(-time.Hour)
	n := ProposteNellAutoritaDopo(a, append(append([]db.ComponenteProposta{}, nodi...), altri...),
		append(append([]db.RelazioneProposta{}, archi...), altriArchi...), nil, &prima)
	if n != 3 { // #3 aperto, e gli archi #1 → #2, #1 → #3 aperti
		t.Errorf("proposte nell'autorita' dopo la baseline: %d, attese 3", n)
	}
	dopo := time.Now().Add(time.Hour)
	if n := ProposteNellAutoritaDopo(a, nodi, archi, nil, &dopo); n != 0 {
		t.Errorf("niente e' nato dopo: %d", n)
	}
}

// rigaDelega: il nodo chiave del file del padre (la riga padre di rigaDich), marcato come delega per c. Il
// documento e' quello del file, cioe' del padre. Nel file il nodo e' figlio della sorgente del padre (padri):
// la catena che ValutaDichiarazioni controlla (F5b).
func rigaDelega(padre db.ListDichiarazioniRfqRow, c db.Componente, chiave string) db.ListDichiarazioniRfqRow {
	r := rigaDich(c, padre.Sha256, chiave, OrigineSmistamento)
	r.AllegatoID, r.NomeFile = padre.AllegatoID, padre.NomeFile
	r.DocumentoID, r.DocumentoComponenteID, r.StepSha256 = padre.DocumentoID, padre.DocumentoComponenteID, pgtype.Text{}
	r.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: RuoloDelega, ComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true}})
	r.Padri = []string{padre.Chiave}
	return r
}

// Domanda 1 = B (decisioni del 27/09 ter, A5.4.6): la delega esplicita dello stesso file per un componente
// annidato. Una marcatura con il ruolo «delega» sul nodo 7120010 del file autorizzato per 7120001 vale finche'
// lo stesso file e' autorizzato, valido, per il componente del suo documento: allora il file ha due sorgenti,
// 7120010 resta figlio diretto di 7120001 ed e' sorgente per il suo figlio 7120011 (il nipote entra
// nell'autorita' solo cosi', non per profondita'). Con il padre sospeso la delega e' sospesa con lui (un avviso,
// non ferma il gate); senza il padre, o con il padre in conflitto, la catena e' rotta (da sistemare: ferma il
// gate); con lo STEP proprio di 7120010 e' un conflitto; delega e radice per lo stesso componente sullo stesso
// file sono incoerenti. La delega non tiene il documento al suo componente (lo tiene il padre), e le sue
// rimozioni hanno il documento del padre e il padre 7120010.
//
// Riscritta per lo Smistamento (F5b, prova 221 parte pura): prima fissava che una delega valesse con il solo
// file autorizzato per il padre, in qualunque punto del file. Adesso vale solo con la catena: il nodo delegato
// e' figlio diretto, nel file, di una sorgente valida dello stesso file, fino a una radice o a un
// raggruppamento. La delega del nipote 7120011 vale solo con quella del figlio 7120010; un nodo che nel file
// non sta sotto una sorgente, o due deleghe che si tengono fra loro senza arrivare alla radice, restano fuori,
// e la revoca del padre le porta via con se'.
//
// Riscritta per lo Smistamento (G, A5.4.8, L3): prima fissava che una delega senza catena fosse sempre
// «sospesa» (un avviso, fuori dal gate), anche senza il padre, con il padre in conflitto, con il nodo fuori
// dalla catena del file. Adesso e' sospesa solo quando a toglierle la catena e' una sospensione sopra di lei (il
// padre sospeso, la delega del livello sopra sospesa: aspetta con loro); in ogni altro caso la catena e' rotta,
// la delega e' fra quelle da sistemare e il gate si ferma. Ogni caso controlla in piu' in quale dei due elenchi
// sta; e c'e' il caso del nipote sotto la delega sospesa del figlio.
func TestLaDelegaValeSoloConIlFileDelPadre(t *testing.T) {
	p := componente("7120001", db.TipoComponenteFinito, false)
	s := componente("7120010", db.TipoComponenteSottoassieme, false)
	sha := shaDi("a")
	padre := rigaDich(p, sha, "#1", OrigineSmistamento)
	delega := rigaDelega(padre, s, "#2")

	d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{padre, delega})
	x, ok := d.Di(s.ComponenteID)
	if _, okp := d.Di(p.ComponenteID); !okp || !ok || !x.Delega() || len(d.DaSistemare)+len(d.Sospese) != 0 {
		t.Fatalf("padre e delega validi: %+v", d)
	}
	if src := d.Sorgenti(sha); src["#1"] != p.ComponenteID || src["#2"] != s.ComponenteID || len(src) != 2 {
		t.Errorf("le sorgenti del file: %v", src)
	}
	if f := x.Frase(); !strings.Contains(f, "(delega)") {
		t.Errorf("la frase dice la delega: %q", f)
	}
	if y, ok := d.DelDocumento(padre.DocumentoID.UUID); !ok || y.Componente.ComponenteID != p.ComponenteID {
		t.Errorf("il documento lo tiene il padre, non la delega: %+v %v", y, ok)
	}
	if !d.RimozioneValida(padre.DocumentoID.UUID, s.ComponenteID) || !d.RimozioneValida(padre.DocumentoID.UUID, p.ComponenteID) ||
		d.RimozioneValida(uuid.New(), s.ComponenteID) {
		t.Errorf("le rimozioni della delega: il documento del padre, il padre 7120010")
	}
	if _, rim := d.PerIlGate(); len(rim) != 2 {
		t.Errorf("il gate conta le rimozioni delle due dichiarazioni: %v", rim)
	}
	all, nodi, archi := righeAutorita(sha)
	a := CalcolaAutorita(d, nodi, archi)
	if c, ok := a.FiglioDiretto(all, "#2"); !ok || c != p.ComponenteID {
		t.Errorf("7120010 resta figlio diretto di 7120001: %v %v", c, ok)
	}
	if c, ok := a.FiglioDiretto(all, "#4"); !ok || c != s.ComponenteID {
		t.Errorf("con la delega il nipote 7120011 e' figlio diretto di 7120010: %v %v", c, ok)
	}
	if c, ok := a.ArcoAutorizzato(ChiaveRelazione{Allegato: all, Padre: "#2", Figlio: "#4"}); !ok || c != s.ComponenteID {
		t.Errorf("l'arco 7120010 → 7120011 e' nell'autorita' della delega: %v %v", c, ok)
	}

	casi := []struct {
		nome     string
		righe    []db.ListDichiarazioniRfqRow
		problema string
		sospesa  bool
	}{
		{"senza il file autorizzato per il padre", []db.ListDichiarazioniRfqRow{delega}, "catena rotta: la delega vale con l'autorizzazione", false},
		{"il padre in conflitto", []db.ListDichiarazioniRfqRow{padre, delega, rigaDich(p, shaDi("b"), "#1", OrigineSmistamento)},
			"catena rotta: la delega vale con l'autorizzazione", false},
		{"il padre sospeso", []db.ListDichiarazioniRfqRow{marcaSospesa(padre, p), delega}, "sospesa: la delega vale con l'autorizzazione", true},
		{"lo STEP proprio di 7120010: conflitto", []db.ListDichiarazioniRfqRow{padre, delega, rigaDich(s, shaDi("c"), "#1", OrigineSmistamento)},
			"conflitto: 7120010 ha 2 STEP autorizzati", false},
		{"delega e radice per lo stesso componente", func() []db.ListDichiarazioniRfqRow {
			r := rigaDelega(padre, s, "#9")
			r.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: RuoloRadice})
			return []db.ListDichiarazioniRfqRow{padre, delega, r}
		}(), "autorizzato e delegato", false},
		{"il componente delegato e' archiviato", []db.ListDichiarazioniRfqRow{padre,
			rigaDelega(padre, componente("7120010", db.TipoComponenteSottoassieme, true), "#2")}, "archiviato", true},
		{"il nodo delegato non sta sotto una sorgente del file", func() []db.ListDichiarazioniRfqRow {
			r := rigaDelega(padre, s, "#2")
			r.Padri = []string{"#7"}
			return []db.ListDichiarazioniRfqRow{padre, r}
		}(), "catena rotta: la delega vale solo con la catena del file", false},
		{"il nodo delegato e' una radice del file", func() []db.ListDichiarazioniRfqRow {
			r := rigaDelega(padre, s, "#2")
			r.Padri = nil
			return []db.ListDichiarazioniRfqRow{padre, r}
		}(), "catena rotta: la delega vale solo con la catena del file", false},
		{"due deleghe che si tengono fra loro, senza la radice", func() []db.ListDichiarazioniRfqRow {
			r := rigaDelega(padre, s, "#2")
			r.Padri = []string{"#5"}
			altra := rigaDelega(padre, componente("7120012", db.TipoComponenteSottoassieme, false), "#5")
			altra.Padri = []string{"#2"}
			return []db.ListDichiarazioniRfqRow{padre, r, altra}
		}(), "catena rotta: la delega vale solo con la catena del file", false},
	}
	for _, cs := range casi {
		t.Run(cs.nome, func(t *testing.T) {
			d := ValutaDichiarazioni(cs.righe)
			var trovata *Dichiarazione
			for _, xx := range d.tutte {
				for i := range xx {
					if xx[i].Componente.Codice == "7120010" {
						trovata = &xx[i]
					}
				}
			}
			if trovata == nil || trovata.Valida() || !strings.Contains(trovata.Problema, cs.problema) || trovata.Sospesa() != cs.sospesa {
				t.Fatalf("la delega: %+v, atteso %q (sospesa %v)", trovata, cs.problema, cs.sospesa)
			}
			if _, ok := d.Sorgenti(sha)["#2"]; ok {
				t.Errorf("una delega che non vale non da' sorgenti")
			}
			// una sospesa sta fra le sospese e non ferma il gate; una da sistemare (la catena rotta, il conflitto,
			// l'incoerenza) sta fra quelle da sistemare, e il gate si ferma
			inSistemare, inSospese := false, false
			for _, y := range d.DaSistemare {
				inSistemare = inSistemare || y.Componente.Codice == "7120010"
			}
			for _, y := range d.Sospese {
				inSospese = inSospese || y.Componente.Codice == "7120010"
			}
			if cs.sospesa && (inSistemare || !inSospese) {
				t.Errorf("una delega sospesa sta fra le sospese e non ferma il gate: sospese %v, da sistemare %v", inSospese, inSistemare)
			}
			if !cs.sospesa && (!inSistemare || inSospese) {
				t.Errorf("una delega che non vale e non e' sospesa ferma il gate: sospese %v, da sistemare %v", inSospese, inSistemare)
			}
		})
	}
	// con lo STEP proprio di 7120010 in conflitto, il file del padre resta valido
	d = ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{padre, delega, rigaDich(s, shaDi("c"), "#1", OrigineSmistamento)})
	if _, ok := d.Di(p.ComponenteID); !ok {
		t.Errorf("il conflitto di 7120010 non tocca l'autorizzazione di 7120001: %+v", d)
	}

	// livello per livello: il nipote 7120011 (#4, figlio di #2 nel file) si delega solo sopra la delega di
	// 7120010. Con tutte e due, il file ha tre sorgenti; senza quella del figlio, il nipote resta senza catena
	// e non da' sorgenti, con la catena rotta; con il padre revocato (la riga del padre non c'e' piu') nessuna
	// delle due vale, e tutte e due hanno la catena rotta; con la delega del figlio sospesa (registrata), il
	// nipote aspetta con lei: sospeso anche lui, e il gate non si ferma.
	n := componente("7120011", db.TipoComponenteSottoassieme, false)
	nipote := rigaDelega(padre, n, "#4")
	nipote.Padri = []string{"#2"}
	d = ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{padre, delega, nipote})
	if src := d.Sorgenti(sha); len(src) != 3 || src["#4"] != n.ComponenteID || len(d.Sospese)+len(d.DaSistemare) != 0 {
		t.Errorf("la catena radice → 7120010 → 7120011: sorgenti %v, sospese %d", src, len(d.Sospese))
	}
	d = ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{padre, nipote})
	if _, ok := d.Di(n.ComponenteID); ok || len(d.DaSistemare) != 1 || !d.DaSistemare[0].SenzaCatena() || len(d.Sospese) != 0 ||
		!strings.HasPrefix(d.DaSistemare[0].Problema, "catena rotta") {
		t.Errorf("il nipote senza la delega del figlio: da sistemare %+v, sospese %d", d.DaSistemare, len(d.Sospese))
	}
	d = ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{delega, nipote})
	if len(d.PerComponente) != 0 || len(d.DaSistemare) != 2 || len(d.Sospese) != 0 {
		t.Errorf("senza il padre nessuna delega vale: valide %d, da sistemare %d, sospese %d", len(d.PerComponente), len(d.DaSistemare), len(d.Sospese))
	}
	delegaSospesa := delega
	delegaSospesa.Marcatura, _ = json.Marshal(Marcatura{V: 1, Ruolo: RuoloDelega, ComponenteID: uuid.NullUUID{UUID: s.ComponenteID, Valid: true},
		Sospesa: &Sospensione{Motivo: "è diventato commerciale", Tipo: string(db.TipoComponenteCommerciale)}})
	d = ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{padre, delegaSospesa, nipote})
	if _, ok := d.Di(p.ComponenteID); !ok || len(d.DaSistemare) != 0 || len(d.Sospese) != 2 {
		t.Errorf("sotto la delega sospesa del figlio il nipote aspetta: valide %d, da sistemare %+v, sospese %d", len(d.PerComponente), d.DaSistemare, len(d.Sospese))
	}
	for _, x := range d.Sospese {
		if x.Componente.ComponenteID == n.ComponenteID && (!x.SenzaCatena() || !x.Sospesa()) {
			t.Errorf("il nipote e' sospeso per la catena: %q", x.Problema)
		}
	}
}
