package distintabozza

// L1 — R5 formato 3 (conferma3*.go): la lettura del corpo (obblighi, presa d'atto, impronta del corpo senza
// bozza_versione), la regola unica del tipo effettivo, l'ordine dei controlli sul riepilogo di S (base, gate prima della
// firma, firma, blocchi, presa d'atto, NESSUNA_MODIFICA, parti non applicabili) e la traduzione dei rifiuti del COMMIT.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/platform/db"
)

func corpoR5Prova(t *testing.T, cambia func(m map[string]any)) []byte {
	t.Helper()
	prodotto := uuid.MustParse("00000000-0000-4000-8000-000000000101")
	bozza := map[string]any{"formato": 3, "prodotto_id": prodotto, "versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0},
		"firma_base": strings.Repeat("1", 64),
		"nodi": []any{map[string]any{"chiave_bozza": "k-root", "componente_id": prodotto, "op": "riusa", "tipo": "finito", "origine": "registrato",
			"identita": map[string]any{"emittente": "cliente:00000000-0000-4000-8000-000000000909", "emittente_presunto": true, "codice": "ACM-1000", "modo": "registrato"}}},
		"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{},
		"bozza_id": "00000000-0000-4000-8000-000000000201", "bozza_versione": 3, "chiave_idempotenza": "00000000-0000-4000-8000-000000000301"}
	m := map[string]any{"firma_anteprima": strings.Repeat("2", 64), "bozza": bozza}
	if cambia != nil {
		cambia(m)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestV1D39R5LaLetturaDelCorpo(t *testing.T) {
	r, err := LeggiRichiestaR5(corpoR5Prova(t, nil))
	if err != nil {
		t.Fatalf("corpo valido: %v", err)
	}
	if r.Bozza.BozzaID == nil || r.Bozza.BozzaVersione != 3 || r.Bozza.ChiaveIdempotenza == nil || r.PresaDAttoPresente || !reSha256.MatchString(r.ImprontaCorpo) {
		t.Fatalf("letto: %+v", r)
	}
	// l'impronta non dipende da bozza_versione (C3 §5.2), dipende dalla firma e dalla presa d'atto
	r2, err := LeggiRichiestaR5(corpoR5Prova(t, func(m map[string]any) { m["bozza"].(map[string]any)["bozza_versione"] = 4 }))
	if err != nil || r2.ImprontaCorpo != r.ImprontaCorpo {
		t.Fatalf("impronta senza bozza_versione: %v", err)
	}
	r3, _ := LeggiRichiestaR5(corpoR5Prova(t, func(m map[string]any) { m["firma_anteprima"] = strings.Repeat("3", 64) }))
	if r3.ImprontaCorpo == r.ImprontaCorpo {
		t.Fatal("l'impronta deve cambiare con la firma")
	}
	a, b := "00000000-0000-4000-8000-0000000000a2", "00000000-0000-4000-8000-0000000000a1"
	r4, err := LeggiRichiestaR5(corpoR5Prova(t, func(m map[string]any) { m["presa_d_atto_in_analisi"] = []string{a, b, a} }))
	if err != nil || !r4.PresaDAttoPresente || len(r4.PresaDAtto) != 2 || r4.PresaDAtto[0].String() != b || r4.ImprontaCorpo == r.ImprontaCorpo {
		t.Fatalf("presa d'atto in ordine, senza ripetizioni: %+v %v", r4.PresaDAtto, err)
	}
	r5, _ := LeggiRichiestaR5(corpoR5Prova(t, func(m map[string]any) { m["presa_d_atto_in_analisi"] = []string{b, a} }))
	if r5.ImprontaCorpo != r4.ImprontaCorpo {
		t.Fatal("l'ordine della presa d'atto non cambia l'impronta")
	}
	for _, c := range []struct {
		nome, campo, motivo string
		cambia              func(m map[string]any)
	}{
		{"senza firma", "firma_anteprima", "", func(m map[string]any) { delete(m, "firma_anteprima") }},
		{"firma corta", "firma_anteprima", "", func(m map[string]any) { m["firma_anteprima"] = "abc" }},
		{"senza bozza", "bozza", MotivoBozzaObbligatoria, func(m map[string]any) { delete(m, "bozza") }},
		{"senza bozza_id", "bozza.bozza_id", MotivoBozzaObbligatoria, func(m map[string]any) { delete(m["bozza"].(map[string]any), "bozza_id") }},
		{"senza bozza_versione", "bozza.bozza_versione", "", func(m map[string]any) { delete(m["bozza"].(map[string]any), "bozza_versione") }},
		{"senza chiave", "bozza.chiave_idempotenza", "", func(m map[string]any) { delete(m["bozza"].(map[string]any), "chiave_idempotenza") }},
		{"formato 2", "formato", MotivoFormatoDaConvertire, func(m map[string]any) { m["bozza"].(map[string]any)["formato"] = 2 }},
		{"campo in piu'", "autorizza", "", func(m map[string]any) { m["autorizza"] = true }},
		{"presa d'atto non UUID", "presa_d_atto_in_analisi", "", func(m map[string]any) { m["presa_d_atto_in_analisi"] = []string{"x"} }},
	} {
		_, err := LeggiRichiestaR5(corpoR5Prova(t, c.cambia))
		var e *Errore
		if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido || (c.campo != "" && e.Campo != c.campo) || e.Motivo != c.motivo {
			t.Errorf("%s: %+v", c.nome, err)
		}
	}
}

// La regola unica del tipo effettivo (R4 e R5): cambia_tipo, poi d1_a_particolare, poi d1_a_sottoassieme.
func TestV1D39R5IlTipoEffettivo(t *testing.T) {
	c := uuid.New()
	n := Nodo3{ChiaveBozza: "k", ComponenteID: &c, Tipo: "sottoassieme"}
	s := Snapshot3{Decisioni: []Decisione3{{Tipo: DecisioneD1ASottoassieme, ChiaveBozza: "k"}, {Tipo: DecisioneCambiaTipo, ComponenteID: &c, TipoNuovo: "commerciale"}}}
	if got := tipoEffettivo3(s, n); got != "sottoassieme" {
		t.Fatalf("d1_a_sottoassieme vince su cambia_tipo: %s", got)
	}
	s.Decisioni = []Decisione3{{Tipo: DecisioneD1AParticolare, ChiaveBozza: "k"}}
	if got := tipoEffettivo3(s, n); got != "sciolto" {
		t.Fatalf("d1_a_particolare: %s", got)
	}
	s.Decisioni = []Decisione3{{Tipo: DecisioneCambiaTipo, ComponenteID: &c, TipoNuovo: "commerciale"}}
	if got := tipoEffettivo3(s, n); got != "commerciale" {
		t.Fatalf("cambia_tipo: %s", got)
	}
}

// L'ordine dei controlli sul riepilogo di S (controlliR5).
func TestV1D39R5LOrdineDeiControlli(t *testing.T) {
	firma := strings.Repeat("2", 64)
	r := RichiestaR5{FirmaAnteprima: firma}
	gate := Rilievo{Codice: BloccoStepSorgenteNonConfermata, Motivo: SorgenteMotivoRevocata}
	altro := Rilievo{Codice: BloccoCodiceDaAssegnare}
	ok := RiepilogoR4{Base: StatoBase{Stato: BaseValida}, FirmaAnteprima: firma}
	codiceDi := func(err error) (string, *RifiutoR5) {
		var e *Errore
		if !errors.As(err, &e) {
			return "", nil
		}
		var r5 *RifiutoR5
		errors.As(err, &r5)
		return e.Codice, r5
	}
	casi := []struct {
		nome   string
		rp     RiepilogoR4
		r      RichiestaR5
		p      *pianoR5
		codice string
	}{
		{"base superata prima di tutto", func() RiepilogoR4 { x := ok; x.Base.Stato = BaseSuperata; x.Blocchi = []Rilievo{gate}; return x }(), r, &pianoR5{}, CodiceBaseSuperata},
		{"il gate prima della firma", func() RiepilogoR4 {
			x := ok
			x.FirmaAnteprima = strings.Repeat("9", 64)
			x.Blocchi = []Rilievo{gate}
			return x
		}(), r, &pianoR5{}, CodiceBlocchi},
		{"la firma prima degli altri blocchi", func() RiepilogoR4 {
			x := ok
			x.FirmaAnteprima = strings.Repeat("9", 64)
			x.Blocchi = []Rilievo{altro}
			return x
		}(), r, &pianoR5{}, CodiceAnteprimaSuperata},
		{"un altro blocco", func() RiepilogoR4 { x := ok; x.Blocchi = []Rilievo{altro}; return x }(), r, &pianoR5{}, CodiceBlocchi},
		{"documenti in analisi senza presa d'atto", func() RiepilogoR4 {
			x := ok
			x.FileInAnalisi = []FileInAnalisi{{AllegatoID: uuid.New()}}
			return x
		}(), r, &pianoR5{}, CodiceCorpoNonValido},
		{"documenti in analisi cambiati", func() RiepilogoR4 {
			x := ok
			x.FileInAnalisi = []FileInAnalisi{{AllegatoID: uuid.New()}}
			return x
		}(), RichiestaR5{FirmaAnteprima: firma, PresaDAttoPresente: true, PresaDAtto: []uuid.UUID{uuid.New()}}, &pianoR5{}, CodiceAnteprimaSuperata},
		{"uguale alla base", func() RiepilogoR4 { x := ok; x.UgualeAllaBase = true; return x }(), r, &pianoR5{}, CodiceNessunaModifica},
		{"parti non applicabili", ok, r, &pianoR5{nonApplicabili: []Rilievo{nonApplicabile("ricodifica", "x", "", nil)}}, CodiceBlocchi},
	}
	for _, c := range casi {
		codice, r5 := codiceDi(controlliR5(c.r, c.rp, c.p))
		if codice != c.codice {
			t.Errorf("%s: %s", c.nome, codice)
		}
		if c.codice == CodiceAnteprimaSuperata && (r5 == nil || r5.Riepilogo == nil) {
			t.Errorf("%s: il 409 porta il riepilogo nuovo", c.nome)
		}
	}
	if err := controlliR5(r, ok, &pianoR5{}); err != nil {
		t.Fatalf("tutto a posto: %v", err)
	}
}

// I rifiuti del COMMIT: CONTROLLO_FINALE non registrato; 40P01 e simili restano errori da tradurre in riprova.
func TestV1D39R5IRifiutiDelCommit(t *testing.T) {
	for _, c := range []struct {
		codice, atteso string
	}{{"P4X01", CodiceControlloFinale}, {"P4X10", CodiceControlloFinale}, {"23514", CodiceControlloFinale}, {"BOM01", CodiceControlloFinale}} {
		err := erroreAlCommit(&pgconn.PgError{Code: c.codice})
		var r5 *RifiutoR5
		if !errors.As(err, &r5) || r5.Err.Codice != c.atteso || !r5.NonRegistrato || r5.Err.Motivo != c.codice {
			t.Errorf("%s: %v", c.codice, err)
		}
	}
	for _, c := range []string{"40P01", "55P03", "40001"} {
		err := traduciR5(erroreAlCommit(&pgconn.PgError{Code: c}))
		var r5 *RifiutoR5
		if !errors.As(err, &r5) || r5.Err.Codice != CodiceAnteprimaSuperata || r5.Err.Motivo != MotivoRiprova {
			t.Errorf("%s: %v", c, err)
		}
	}
	if err := erroreAlCommit(errors.New("altro")); err == nil || err.Error() != "altro" {
		t.Fatal("un errore qualunque resta com'e'")
	}
}

// decidibile: le righe che i segni toccano senza anche_decise.
func TestV1D39R5LeRigheDecidibili(t *testing.T) {
	c := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	for _, x := range []struct {
		r  db.ListRighePerShaChiaveRow
		ok bool
	}{
		{db.ListRighePerShaChiaveRow{Stato: db.StatoPropostaAperta}, true},
		{db.ListRighePerShaChiaveRow{Stato: db.StatoPropostaDuplicato, ComponenteID: c}, true},
		{db.ListRighePerShaChiaveRow{Stato: db.StatoPropostaDuplicato, ComponenteID: c, DecisoDa: c}, false},
		{db.ListRighePerShaChiaveRow{Stato: db.StatoPropostaScartata}, false},
	} {
		if decidibile(x.r) != x.ok {
			t.Errorf("%+v", x.r)
		}
	}
}
