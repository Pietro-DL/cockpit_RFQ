package distintabozza

// L1 — direttiva 39 §4 (D-PB03 A [U]; R39-24 [T-39]): la provenienza e il registro d'origine di una PB li decide il
// server. La pagina li rimanda come li ha letti o li omette; non li mette e non li cambia, salvo il registro di un
// suggerimento accettato (provenienza3.go). Le regole della decisione: rimasta, spostata (corretta), nuova (manuale),
// suggerimento (motore), legacy spostata (manuale); gli eventi portano la provenienza della PB che nominano.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

const impA, impB = "6666666666666666666666666666666666666666666666666666666666666666", "7777777777777777777777777777777777777777777777777777777777777777"

func TestV1D39LaProvenienzaDellaPBLaDecideIlServer(t *testing.T) {
	a, b, l := uuid.New(), uuid.New(), uuid.New()
	prima := Snapshot3{DocumentiPreassociati: []Preassociazione3{
		{AllegatoID: a, ChiaveBozza: "k-1", Provenienza: ProvenienzaMotore, Importazione: impA},
		{AllegatoID: b, ChiaveBozza: "k-2"},
		{AllegatoID: l, ChiaveBozza: "k-3", Provenienza: ProvenienzaLegacy},
	}}
	casi := []struct {
		nome    string
		nuovo   []Preassociazione3
		rifiuto bool
	}{
		{"tutto com'era", prima.DocumentiPreassociati, false},
		{"la PB rimasta senza i campi del server (la pagina non li ha riletti)", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-1"}}, false},
		{"la PB rimasta cambia la provenienza", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-1", Provenienza: ProvenienzaManuale}}, true},
		{"la PB rimasta cambia il registro", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-1", Provenienza: ProvenienzaMotore, Importazione: impB}}, true},
		{"la PB rimasta perde il solo registro", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-1", Provenienza: ProvenienzaMotore}}, true},
		{"la PB senza provenienza ne prende una dalla pagina", []Preassociazione3{{AllegatoID: b, ChiaveBozza: "k-2", Provenienza: ProvenienzaManuale}}, true},
		{"la legacy diventa motore dalla pagina", []Preassociazione3{{AllegatoID: l, ChiaveBozza: "k-3", Provenienza: ProvenienzaMotore}}, true},
		{"una PB nuova con la provenienza manuale", []Preassociazione3{{AllegatoID: uuid.New(), ChiaveBozza: "k-4", Provenienza: ProvenienzaManuale}}, true},
		{"una PB nuova motore senza registro", []Preassociazione3{{AllegatoID: uuid.New(), ChiaveBozza: "k-4", Provenienza: ProvenienzaMotore}}, true},
		{"una PB nuova corretta con un registro", []Preassociazione3{{AllegatoID: uuid.New(), ChiaveBozza: "k-4", Provenienza: ProvenienzaCorretta, Importazione: impA}}, true},
		{"una PB nuova senza provenienza", []Preassociazione3{{AllegatoID: uuid.New(), ChiaveBozza: "k-4"}}, false},
		{"un suggerimento accettato (il registro)", []Preassociazione3{{AllegatoID: uuid.New(), ChiaveBozza: "k-4", Importazione: impB}}, false},
		{"un suggerimento accettato (motore e il registro)", []Preassociazione3{{AllegatoID: uuid.New(), ChiaveBozza: "k-4", Provenienza: ProvenienzaMotore, Importazione: impB}}, false},
		{"la PB spostata con i campi letti", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-9", Provenienza: ProvenienzaMotore, Importazione: impA}}, false},
		{"la PB spostata senza i campi", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-9"}}, false},
		{"la PB spostata diventa corretta dalla pagina", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-9", Provenienza: ProvenienzaCorretta, Importazione: impA}}, true},
		{"la PB spostata sul suggerimento di un altro registro", []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-9", Importazione: impB}}, false},
	}
	for _, c := range casi {
		err := controllaDichiarate(prima, Snapshot3{DocumentiPreassociati: c.nuovo})
		var e *Errore
		switch {
		case !c.rifiuto && err != nil:
			t.Errorf("%s: %v", c.nome, err)
		case c.rifiuto && (!errors.As(err, &e) || e.Codice != CodiceCorpoNonValido || e.Motivo != MotivoProvenienzaDelServer):
			t.Errorf("%s: atteso provenienza_dal_server, avuto %v", c.nome, err)
		}
	}

	// la decisione del server
	pp := map[uuid.UUID]Preassociazione3{}
	for _, p := range prima.DocumentiPreassociati {
		pp[p.AllegatoID] = p
	}
	nuovo := uuid.New()
	attese := []struct {
		nome                  string
		a                     uuid.UUID
		chiave                string
		provenienza, registro string
		rimasta               bool
	}{
		{"motore rimasta: com'era", a, "k-1", ProvenienzaMotore, impA, true},
		{"motore spostata: corretta, conserva il registro A", a, "k-9", ProvenienzaCorretta, impA, false},
		{"senza provenienza rimasta: manuale", b, "k-2", ProvenienzaManuale, "", true},
		{"senza provenienza spostata: manuale", b, "k-7", ProvenienzaManuale, "", false},
		{"legacy rimasta: legacy", l, "k-3", ProvenienzaLegacy, "", true},
		{"legacy spostata: rifatta a mano, manuale", l, "k-1", ProvenienzaManuale, "", false},
		{"nuova: manuale", nuovo, "k-4", ProvenienzaManuale, "", false},
	}
	for _, x := range attese {
		p := decisa(pp, x.a, x.chiave)
		if p.Provenienza != x.provenienza || p.Importazione != x.registro || p.ChiaveBozza != x.chiave || p.AllegatoID != x.a ||
			rimasta(pp, x.a, x.chiave) != x.rimasta {
			t.Errorf("%s: %+v", x.nome, p)
		}
	}
	// una correzione due volte resta corretta e tiene il registro d'origine
	corretta := map[uuid.UUID]Preassociazione3{a: {AllegatoID: a, ChiaveBozza: "k-9", Provenienza: ProvenienzaCorretta, Importazione: impA}}
	if p := decisa(corretta, a, "k-5"); p.Provenienza != ProvenienzaCorretta || p.Importazione != impA {
		t.Errorf("seconda correzione: %+v", p)
	}

	// gli eventi: file_svincolato da k-1 (motore A) e file_preassociato su k-9 (corretta A); una PB nuova (manuale)
	ev := []Evento{
		{Azione: AzFileSvincolato, Prima: canonico(Preassociazione3{AllegatoID: a, ChiaveBozza: "k-1"}), Dopo: canonico(DopoSvincolo{AllegatoID: a, Motivo: SvincoloUtente})},
		{Azione: AzFilePreassociato, Dopo: canonico(map[string]any{"allegato_id": a, "chiave_bozza": "k-9", "provenienza": "legacy"})},
		{Azione: AzFilePreassociato, Dopo: canonico(Preassociazione3{AllegatoID: nuovo, ChiaveBozza: "k-4"})},
	}
	finali := []Preassociazione3{{AllegatoID: a, ChiaveBozza: "k-9", Provenienza: ProvenienzaCorretta, Importazione: impA},
		{AllegatoID: nuovo, ChiaveBozza: "k-4", Provenienza: ProvenienzaManuale}}
	conProvenienzaNegliEventi(ev, prima, finali)
	leggi := func(raw json.RawMessage) Preassociazione3 {
		var p Preassociazione3
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := leggi(ev[0].Prima); p.Provenienza != ProvenienzaMotore || p.Importazione != impA {
		t.Errorf("svincolo: %+v", p)
	}
	if p := leggi(ev[1].Dopo); p.Provenienza != ProvenienzaCorretta || p.Importazione != impA {
		t.Errorf("preassociazione spostata: %+v", p)
	}
	if p := leggi(ev[2].Dopo); p.Provenienza != ProvenienzaManuale || p.Importazione != "" {
		t.Errorf("preassociazione nuova: %+v", p)
	}
}

// La forma della provenienza nello snapshot (validaProvenienza, come ck_dbv_preassociazioni) e il round-trip: lo
// snapshot con provenienza e registro si scrive e si rilegge con gli stessi byte.
func TestV1D39LaFormaDellaProvenienzaEIlRoundTrip(t *testing.T) {
	ok := []Preassociazione3{{Provenienza: ProvenienzaMotore, Importazione: impA}, {Provenienza: ProvenienzaCorretta, Importazione: impA},
		{Provenienza: ProvenienzaManuale}, {Provenienza: ProvenienzaLegacy}, {}, {Importazione: impA}}
	for _, p := range ok {
		if e := validaProvenienza("x", p); e != nil {
			t.Errorf("%+v: %v", p, e)
		}
	}
	no := []Preassociazione3{{Provenienza: "ignota"}, {Provenienza: ProvenienzaManuale, Importazione: impA}, {Provenienza: ProvenienzaLegacy, Importazione: impA},
		{Provenienza: ProvenienzaMotore, Importazione: "ABC"}, {Provenienza: ProvenienzaMotore, Importazione: impA + " "}}
	for _, p := range no {
		if e := validaProvenienza("x", p); e == nil {
			t.Errorf("%+v: atteso un rifiuto", p)
		}
	}
	id := idSintetici3()
	reg := componiProva3(t, id)
	s := snapshotProva3(id, reg.Impronta)
	s["documenti_preassociati"] = []any{
		map[string]any{"allegato_id": id.pdf.String(), "chiave_bozza": "k-0007", "provenienza": "motore", "importazione": reg.Impronta},
	}
	letto, err := LeggiSnapshot3(snapshotJSON(t, s))
	if err != nil {
		t.Fatal(err)
	}
	b, err := letto.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	riletto, err := LeggiSnapshot3(b)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := riletto.Canonico()
	if err != nil || string(b) != string(b2) {
		t.Fatalf("round-trip: %v", err)
	}
	if p := riletto.DocumentiPreassociati[0]; p.Provenienza != ProvenienzaMotore || p.Importazione != reg.Impronta {
		t.Fatalf("PB riletta: %+v", p)
	}
	// una legacy con il registro non e' una forma ammessa
	s["documenti_preassociati"] = []any{map[string]any{"allegato_id": id.pdf.String(), "chiave_bozza": "k-0007", "provenienza": "legacy", "importazione": reg.Impronta}}
	if _, err := LeggiSnapshot3(snapshotJSON(t, s)); err == nil {
		t.Fatal("legacy con il registro: atteso un rifiuto")
	}
}
