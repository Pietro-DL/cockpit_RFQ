//go:build integrazione

// L4 — direttiva 39 §4 (D-AR14 A [T-39]; conversionep1.go), corsia persistenza, due rilievi della revisione del 20/10:
//   - conversione → abbandono → nuova crea: la pre-assegnazione legacy (P1) convertita alla presa in carico non si perde
//     se la bozza si abbandona senza R5. La bozza successiva del prodotto rimette la PB legacy «da verificare» dal
//     giornale delle AR (ripresa: true, l'evento conversione_legacy d'origine), senza scrivere niente nel giornale, anche
//     dopo un secondo abbandono; una conferma formato 3 del prodotto dopo la conversione la consuma (non si riprende);
//   - la conversione delle P1 dentro converti_da (formato 2 → 3): la PB legacy sul nodo, l'evento conversione_legacy, la
//     copia tolta, nessuna AR; una P1 il cui file e' gia' una PB del formato 2 resta quella PB (manuale), con la copia
//     tolta; la P1 di un componente del perimetro fuori dalla bozza resta com'era.
// Dati sintetici (la scena di persistenza_db_test.go).

package distintabozza

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestV1D39ConversioneP1AbbandonoENuovaCreaDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	root := ChiaveDelComponente(s.prodotto)
	pdf := nuovoAllegato(t, p, idDi("msg-0"), 20, "ACM-1000.pdf", strings.Repeat("d", 64))
	nuovaProposta(t, p, pdf, s.thread, &s.prodotto)
	// la presa in carico converte la P1
	c1, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "vuota"})
	if err != nil || len(c1.ConversioniLegacy) != 1 || c1.ConversioniLegacy[0].Ripresa {
		t.Fatalf("crea vuota: %+v %v", c1.ConversioniLegacy, err)
	}
	var evento int64
	if err := p.QueryRow(ctx, `SELECT evento_id FROM associazione_evento WHERE allegato_id = $1 AND origine = 'conversione_legacy'`, pdf).Scan(&evento); err != nil {
		t.Fatal(err)
	}
	giornale := func() int {
		return conta3(t, p, `SELECT count(*) FROM associazione_evento`)
	}
	eventi := giornale()
	// abbandono senza R5, poi una nuova crea: la PB legacy torna dal giornale, niente si scrive nel giornale delle AR
	abbandona := func(bozza uuid.UUID, versione int) {
		t.Helper()
		if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: bozza, VersioneAttesa: versione, Motivo: "prova", UtenteID: s.utente}); err != nil {
			t.Fatal(err)
		}
	}
	riprendi := func(nome string) Creata {
		t.Helper()
		c, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "vuota"})
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if pb := pbDi(t, c.Snapshot, pdf); pb != (Preassociazione3{AllegatoID: pdf, ChiaveBozza: root, Provenienza: ProvenienzaLegacy}) {
			t.Fatalf("%s: la PB legacy non e' tornata: %+v", nome, pb)
		}
		if len(c.ConversioniLegacy) != 1 || !c.ConversioniLegacy[0].Ripresa || c.ConversioniLegacy[0].Evento != evento ||
			c.ConversioniLegacy[0].ComponentePrima != s.prodotto || c.ConversioniLegacy[0].ChiaveBozza != root || c.ConversioniLegacy[0].PropostaID == uuid.Nil {
			t.Fatalf("%s: conversioni: %+v", nome, c.ConversioniLegacy)
		}
		if n := giornale(); n != eventi {
			t.Fatalf("%s: la ripresa ha scritto nel giornale delle AR: %d -> %d", nome, eventi, n)
		}
		if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id IS NULL`, pdf); n != 1 {
			t.Fatalf("%s: la copia e' tornata", nome)
		}
		if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != 0 {
			t.Fatalf("%s: AR di backfill: %d", nome, n)
		}
		ee := eventiDB(t, p, c.BozzaID, 1)
		if !strings.Contains(campoJSON(t, ee[0].Dopo, "conversioni_legacy"), `"ripresa":true`) ||
			!strings.Contains(campoJSON(t, ee[0].Dopo, "preassociazioni_iniziali"), pdf.String()) {
			t.Fatalf("%s: bozza_aperta: %s", nome, ee[0].Dopo)
		}
		return c
	}
	abbandona(c1.BozzaID, 1)
	c2 := riprendi("dopo il primo abbandono")
	abbandona(c2.BozzaID, 1)
	c3 := riprendi("dopo il secondo abbandono")
	abbandona(c3.BozzaID, 1)

	// una conferma formato 3 del prodotto dopo la conversione l'ha decisa: non si riprende piu'
	snap, err := LeggiSnapshot3(c3.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snap.DocumentiPreassociati = nil
	confermaC3DB(t, p, s, s.prodotto, c3.BozzaID, 1, nil, snap, map[string]uuid.UUID{root: s.prodotto}, nil)
	c4, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "conferma"})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	if pb := pbDi(t, c4.Snapshot, pdf); pb.AllegatoID != uuid.Nil || len(c4.ConversioniLegacy) != 0 {
		t.Fatalf("la conversione consumata da una conferma e' tornata: %+v %+v", pb, c4.ConversioniLegacy)
	}
}

func TestV1D39ConversioneP1InConvertiDaDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	// la bozza formato 2: la radice r e il pezzo k-1; il PDF b e' gia' una PB del formato 2 su k-1
	pdfA := nuovoAllegato(t, p, idDi("msg-0"), 20, "ACM-1000.pdf", strings.Repeat("d", 64))
	nuovaProposta(t, p, pdfA, s.thread, &s.prodotto)
	pdfB := nuovoAllegato(t, p, idDi("msg-0"), 21, "ACM-1000-B.pdf", strings.Repeat("e", 64))
	propB := nuovaProposta(t, p, pdfB, s.thread, &s.prodotto)
	var sotto uuid.UUID
	for k, comp := range s.sc.componenti {
		if k != "k-root" {
			sotto = comp
			break
		}
	}
	pdfC := nuovoAllegato(t, p, idDi("msg-0"), 22, "ACM-1100.pdf", strings.Repeat("f", 64))
	nuovaProposta(t, p, pdfC, s.thread, &sotto)
	sn := soloRadice(s.prodotto, "ACM-1000")
	sn.Nodi = append(sn.Nodi, Nodo{ChiaveBozza: "k-1", Op: OpCrea, Codice: "ACM-1199", Tipo: "sciolto", Origine: "utente"})
	sn.Archi = []Arco{{Padre: "r", Figlio: "k-1", Qta: 1, Origine: "utente"}}
	sn.DocumentiPreassociati = []Preassociazione2{{PropostaID: propB, ChiaveBozza: "k-1"}}
	b2 := bozza2DB(t, p, s.thread, s.prodotto, s.utente, sn)
	chiave := uuid.New()
	c, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: b2, VersioneAttesa: 1,
		Motivo: "si passa al formato 3", ChiaveSalvataggio: chiave, ImprontaRichiesta: strings.Repeat("6", 64)})
	if err != nil {
		t.Fatalf("converti_da: %v", err)
	}
	n3, err := rileggiSnapshot3V(t, p, c.BozzaID, c.Versione)
	if err != nil {
		t.Fatal(err)
	}
	var rootKey string
	for _, n := range n3.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == s.prodotto {
			rootKey = n.ChiaveBozza
		}
	}
	pbs := map[uuid.UUID]Preassociazione3{}
	for _, x := range n3.DocumentiPreassociati {
		pbs[x.AllegatoID] = x
	}
	if pbs[pdfA] != (Preassociazione3{AllegatoID: pdfA, ChiaveBozza: rootKey, Provenienza: ProvenienzaLegacy}) {
		t.Fatalf("la P1 convertita in converti_da: %+v", pbs)
	}
	if pbs[pdfB] != (Preassociazione3{AllegatoID: pdfB, ChiaveBozza: "k-1", Provenienza: ProvenienzaManuale}) {
		t.Fatalf("la PB del formato 2 dello stesso file resta (manuale): %+v", pbs[pdfB])
	}
	if _, ok := pbs[pdfC]; ok {
		t.Fatalf("la P1 fuori dalla bozza e' diventata una PB: %+v", pbs[pdfC])
	}
	conv := map[uuid.UUID]ConversioneLegacy3{}
	for _, x := range c.ConversioniLegacy {
		conv[x.AllegatoID] = x
	}
	if len(conv) != 2 || conv[pdfA].PBEsistente || !conv[pdfB].PBEsistente || conv[pdfA].ChiaveBozza != rootKey {
		t.Fatalf("conversioni: %+v", c.ConversioniLegacy)
	}
	if len(c.PreassegnazioniNonConvertite) != 1 || c.PreassegnazioniNonConvertite[0].AllegatoID != pdfC || c.PreassegnazioniNonConvertite[0].Motivo != P1FuoriBozza {
		t.Fatalf("non convertite: %+v", c.PreassegnazioniNonConvertite)
	}
	for _, a := range []uuid.UUID{pdfA, pdfB} {
		if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE allegato_id = $1 AND origine = 'conversione_legacy' AND azione = 'sgancia'
			AND componente_prima = $2 AND associazione_id IS NULL AND conferma_id IS NULL AND chiave_idempotenza = $3`, a, s.prodotto, chiave); n != 1 {
			t.Fatalf("evento conversione_legacy di %s: %d", a, n)
		}
		if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id IS NULL`, a); n != 1 {
			t.Fatalf("la copia di %s non e' tolta", a)
		}
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id = $2`, pdfC, sotto); n != 1 {
		t.Fatal("la P1 fuori dalla bozza e' cambiata")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != 0 {
		t.Fatalf("AR di backfill: %d", n)
	}
	// la conversione di converti_da si riprende anche lei dopo l'abbandono
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: c.BozzaID, VersioneAttesa: c.Versione, Motivo: "prova", UtenteID: s.utente}); err != nil {
		t.Fatal(err)
	}
	c2, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "vuota"})
	if err != nil {
		t.Fatal(err)
	}
	root := ChiaveDelComponente(s.prodotto)
	if pb := pbDi(t, c2.Snapshot, pdfA); pb != (Preassociazione3{AllegatoID: pdfA, ChiaveBozza: root, Provenienza: ProvenienzaLegacy}) {
		t.Fatalf("ripresa della conversione di converti_da (PDF della radice): %+v", pb)
	}
	if pb := pbDi(t, c2.Snapshot, pdfB); pb != (Preassociazione3{AllegatoID: pdfB, ChiaveBozza: root, Provenienza: ProvenienzaLegacy}) {
		t.Fatalf("ripresa della conversione di converti_da (il PDF che era PB del formato 2 torna sul suo componente legacy): %+v", pb)
	}
}
