package distintabozza

// L1 — direttiva 35, blocco C-2 (corsia persistenza): le preassociazioni del formato 3 per allegato_id. Il tipo sdoppiato
// (INC-G1): Preassociazione3 {allegato_id, chiave_bozza} nel formato 3, Preassociazione2 {proposta_id, chiave_bozza} nel
// formato 2, che si rilegge come era; l'ordine canonico per allegato; il vassoio per allegato; le voci del punto 5 di
// contenuto-atto-1. Le verifiche contro il database (FUORI_RFQ, natura, FILE_NON_ATTIVO, AR attiva dello svincolo, la
// conversione) sono nella L4 c2_preassociazioni_db_test.go. Solo dati sintetici.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// La PB formato 3: l'ordine canonico e' per allegato_id, l'ordine del corpo non cambia l'impronta, nei byte canonici non
// c'e' proposta_id.
func TestV1C2LaPreassociazioneFormato3(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	a1, a2 := "00000000-0000-4000-8000-0000000a0001", "00000000-0000-4000-8000-0000000a0002"
	conPB := func(pb ...map[string]any) map[string]any {
		s := copiaProfonda(t, snapshotProva3(id, imp))
		l := make([]any, 0, len(pb))
		for _, p := range pb {
			l = append(l, p)
		}
		s["documenti_preassociati"] = l
		return s
	}
	x := leggiProva3(t, conPB(map[string]any{"allegato_id": a2, "chiave_bozza": "k-0007"}, map[string]any{"allegato_id": a1, "chiave_bozza": "k-0009"}))
	y := leggiProva3(t, conPB(map[string]any{"allegato_id": a1, "chiave_bozza": "k-0009"}, map[string]any{"allegato_id": a2, "chiave_bozza": "k-0007"}))
	if x.DocumentiPreassociati[0] != (Preassociazione3{AllegatoID: uuid.MustParse(a1), ChiaveBozza: "k-0009"}) ||
		x.DocumentiPreassociati[1] != (Preassociazione3{AllegatoID: uuid.MustParse(a2), ChiaveBozza: "k-0007"}) {
		t.Fatalf("ordine canonico delle PB: %+v", x.DocumentiPreassociati)
	}
	ix, err := x.Impronta()
	if err != nil {
		t.Fatal(err)
	}
	if iy, _ := y.Impronta(); ix != iy {
		t.Fatal("l'ordine delle PB nel corpo cambia l'impronta")
	}
	can, _ := x.Canonico()
	if bytes.Contains(can, []byte("proposta_id")) ||
		!bytes.Contains(can, []byte(`"documenti_preassociati":[{"allegato_id":"`+a1+`","chiave_bozza":"k-0009"},{"allegato_id":"`+a2+`","chiave_bozza":"k-0007"}]`)) {
		t.Fatalf("byte canonici delle PB: %s", can)
	}
	// lo svincolo formato 3 cita {allegato_id, componente_id}
	if !bytes.Contains(can, []byte(`"allegato_id":"`+allegatoSvincolo3+`"`)) {
		t.Fatalf("lo svincolo non cita l'allegato: %s", can)
	}
	// due allegati diversi sullo stesso nodo si preassociano
	if s := leggiProva3(t, conPB(map[string]any{"allegato_id": a1, "chiave_bozza": "k-0007"}, map[string]any{"allegato_id": a2, "chiave_bozza": "k-0007"})); len(s.DocumentiPreassociati) != 2 {
		t.Fatalf("due allegati sullo stesso nodo: %+v", s.DocumentiPreassociati)
	}
	// l'allegato nullo
	if _, err := LeggiSnapshot3(snapshotJSON(t, conPB(map[string]any{"allegato_id": uuid.Nil.String(), "chiave_bozza": "k-0007"}))); err == nil ||
		!strings.Contains(err.Error(), "documenti_preassociati[0].allegato_id") {
		t.Fatalf("PB con l'allegato nullo: %v", err)
	}
}

// Il formato 2 si rilegge come era (PD35 §6.3): la versione e la conferma salvate con {proposta_id} tornano con
// Preassociazione2, gli stessi byte e la stessa impronta; il lettore del formato 3 non le legge.
func TestV1C2IlFormato2SiRileggeComEra(t *testing.T) {
	prodotto, proposta := idDi("prodotto"), idDi("proposta-f2")
	s := Snapshot{Formato: FormatoSnapshot, ProdottoID: prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Nodi: []Nodo{
			{ChiaveBozza: "r", ComponenteID: pu(prodotto), Op: OpRiusa, Codice: "ACM-1000", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-1", Op: OpCrea, Codice: "ACM-1100", Tipo: "sottoassieme", Origine: "utente"},
		},
		Archi:                 []Arco{{Padre: "r", Figlio: "k-1", Qta: 1, Origine: "utente"}},
		DocumentiPreassociati: []Preassociazione2{{PropostaID: proposta, ChiaveBozza: "k-1"}},
	}
	can, err := s.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(can, []byte(`"documenti_preassociati":[{"chiave_bozza":"k-1","proposta_id":"`+proposta.String()+`"}]`)) {
		t.Fatalf("la PB formato 2 non e' {proposta_id, chiave_bozza}: %s", can)
	}
	imp := jsoncanonico.Impronta(can)
	v, err := RileggiVersione(FormatoSnapshot, can, imp)
	if err != nil || v.Formato2 == nil || len(v.Formato2.DocumentiPreassociati) != 1 ||
		v.Formato2.DocumentiPreassociati[0] != (Preassociazione2{PropostaID: proposta, ChiaveBozza: "k-1"}) || !bytes.Equal(v.Canonico, can) || v.Impronta != imp {
		t.Fatalf("rilettura della versione formato 2: %+v %v", v, err)
	}
	c := LeggiSnapshotConferma(FormatoSnapshot, can, imp)
	if c.NonLeggibile || c.Formato2 == nil || c.Formato2.DocumentiPreassociati[0].PropostaID != proposta || !bytes.Equal(c.Canonico, can) {
		t.Fatalf("rilettura della conferma formato 2: %+v", c)
	}
	// il lettore del formato 3 rifiuta lo snapshot formato 2, e una PB formato 2 in uno snapshot formato 3
	if _, err := LeggiSnapshot3(can); err == nil {
		t.Fatal("il lettore del formato 3 legge uno snapshot formato 2")
	}
	if _, err := LeggiSnapshot(can); err != nil {
		t.Fatalf("il lettore del formato 2: %v", err)
	}
}

// Il vassoio per allegato (EM §3.7 con la direttiva 35): ultima azione efficace per allegato, file_lasciato lo toglie,
// file_preassociato lo ricolloca; le voci sono {allegato_id, motivo, chiave_bozza}, in ordine di allegato.
func TestV1C2IlVassoioPerAllegato(t *testing.T) {
	a1, a2 := "00000000-0000-4000-8000-0000000a0001", "00000000-0000-4000-8000-0000000a0002"
	ev := func(n int, verbo, prima, dopo string) EventoGiornale {
		e := EventoGiornale{N: n, Azione: verbo, Dopo: json.RawMessage(dopo)}
		if prima != "" {
			e.Prima = json.RawMessage(prima)
		}
		return e
	}
	eventi := []EventoGiornale{
		ev(1, AzFileSvincolato, `{"allegato_id":"`+a2+`","chiave_bozza":"k-2"}`, `{"allegato_id":"`+a2+`","motivo":"nodo_tolto"}`),
		ev(2, AzFileSvincolato, `{"allegato_id":"`+a1+`","chiave_bozza":"k-1"}`, `{"allegato_id":"`+a1+`","motivo":"non_incluso"}`),
		ev(3, AzFileSvincolato, `{"allegato_id":"`+a1+`","chiave_bozza":"k-9"}`, `{"allegato_id":"`+a1+`","motivo":"utente"}`),
	}
	got := VassoioDa(eventi)
	b, _ := json.Marshal(got)
	if string(b) != `[{"allegato_id":"`+a2+`","motivo":"nodo_tolto","chiave_bozza":"k-2"}]` {
		t.Fatalf("vassoio: %s", b)
	}
	got = VassoioDa(append(eventi[:2:2], ev(3, AzFileLasciato, "", `{"allegato_id":"`+a2+`"}`)))
	if len(got) != 1 || got[0] != (VoceVassoio{AllegatoID: a1, Motivo: SvincoloNonIncluso, ChiaveBozza: "k-1"}) {
		t.Fatalf("vassoio dopo Lascia: %+v", got)
	}
	got = VassoioDa(append(eventi[:2:2], ev(3, AzFilePreassociato, "", `{"allegato_id":"`+a1+`","chiave_bozza":"k-5"}`)))
	if len(got) != 1 || got[0].AllegatoID != a2 {
		t.Fatalf("vassoio dopo la ricollocazione: %+v", got)
	}
	// un evento con la forma di prima (proposta_id) non e' nel vassoio: il vocabolario 2 e' emendato sul posto (D-5)
	if got := VassoioDa([]EventoGiornale{ev(1, AzFileSvincolato, `{"proposta_id":"p","chiave_bozza":"k"}`, `{"proposta_id":"p","motivo":"nodo_tolto"}`)}); len(got) != 0 {
		t.Fatalf("vassoio da un evento con proposta_id: %+v", got)
	}
}

// Il dopo di file_svincolato e di file_lasciato cita l'allegato; proposta_id e' un campo sconosciuto.
func TestV1C2IDopoDeiFile(t *testing.T) {
	a := "00000000-0000-4000-8000-0000000a0001"
	if d, e := LeggiDopoSvincolo("dopo", json.RawMessage(`{"allegato_id":"`+a+`","motivo":"nodo_tolto"}`)); e != nil || d.AllegatoID.String() != a {
		t.Fatalf("dopo dello svincolo: %+v %v", d, e)
	}
	if d, e := LeggiDopoLasciato("dopo", json.RawMessage(`{"allegato_id":"`+a+`","motivo":"non serve"}`)); e != nil || d.AllegatoID.String() != a {
		t.Fatalf("dopo di file_lasciato: %+v %v", d, e)
	}
	for _, raw := range []string{`{"proposta_id":"` + a + `","motivo":"nodo_tolto"}`, `{"allegato_id":"` + a + `","proposta_id":"` + a + `","motivo":"nodo_tolto"}`} {
		if _, e := LeggiDopoSvincolo("dopo", json.RawMessage(raw)); e == nil {
			t.Fatalf("dopo dello svincolo accettato: %s", raw)
		}
	}
	if _, e := LeggiDopoLasciato("dopo", json.RawMessage(`{"proposta_id":"`+a+`"}`)); e == nil {
		t.Fatal("dopo di file_lasciato con proposta_id accettato")
	}
	if _, e := LeggiDopoLasciato("dopo", json.RawMessage(`{"allegato_id":"00000000-0000-0000-0000-000000000000"}`)); e == nil || e.Campo != "dopo.allegato_id" {
		t.Fatalf("allegato nullo: %v", e)
	}
}

// Le voci del punto 5 di contenuto-atto-1 citano l'occorrenza.
func TestV1C2LeVociDelPunto5(t *testing.T) {
	a, c := uuid.MustParse("00000000-0000-4000-8000-0000000a0001"), uuid.MustParse("00000000-0000-4000-8000-000000002200")
	if v := VocePreassociazione(Preassociazione3{AllegatoID: a, ChiaveBozza: "k-1"}); v != "preassociazione:"+a.String()+":k-1" {
		t.Fatal(v)
	}
	if v := VoceSvincolo(a, c); v != "svincolo:"+a.String()+":"+c.String() {
		t.Fatal(v)
	}
}
