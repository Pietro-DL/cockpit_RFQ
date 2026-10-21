package distintabozza

// L1 — la lettura delle bozze di tutti i prodotti per S9 (direttiva 35, blocco C-4, parte persistenza): le parti pure.
//   - la lettura del giornale dal database porta solo AzioniDelVassoio: togliere gli altri eventi non cambia il vassoio;
//   - il nodo della PB: codice e rev dell'identita', la casella dello STEP sorgente (stesso allegato e gruppo), altrimenti
//     la prima; nessuna casella per un nodo manuale;
//   - le query della lettura non leggono documento_proposta (mai la copia come fonte delle AR, [U] §8 C-4).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestV1C4IlVassoioDaiSoliEventiDeiFile(t *testing.T) {
	a1, a2, a3 := uuid.New().String(), uuid.New().String(), uuid.New().String()
	ev := func(n int, az string, prima, dopo any) EventoGiornale {
		var p, d json.RawMessage
		if prima != nil {
			p, _ = json.Marshal(prima)
		}
		if dopo != nil {
			d, _ = json.Marshal(dopo)
		}
		return EventoGiornale{N: n, Azione: az, ChiaveBozza: "k-1", Prima: p, Dopo: d}
	}
	// un giornale con i file e, fra un evento e l'altro, ogni altra azione con campi che la regola leggerebbe
	altre := append([]string{AzBozzaAperta, AzPropostaRiallineata, AzSorgenteCambiata}, AzioniDelVocabolario2...)
	del := map[string]bool{}
	for _, a := range AzioniDelVassoio {
		del[a] = true
	}
	var tutti []EventoGiornale
	n := 0
	rumore := func() {
		for _, a := range altre {
			if del[a] {
				continue
			}
			n++
			campi := map[string]any{"allegato_id": a3, "motivo": SvincoloNodoEscluso, "chiave_bozza": "k-9", "n": 1}
			tutti = append(tutti, ev(n, a, campi, campi))
		}
	}
	file := func(az string, prima, dopo any) int {
		rumore()
		n++
		tutti = append(tutti, ev(n, az, prima, dopo))
		return n
	}
	file(AzFilePreassociato, nil, map[string]any{"allegato_id": a1, "chiave_bozza": "k-1"})
	s1 := file(AzFileSvincolato, map[string]any{"allegato_id": a1, "chiave_bozza": "k-1"}, map[string]any{"allegato_id": a1, "motivo": SvincoloNodoEscluso})
	file(AzFileSvincolato, map[string]any{"allegato_id": a2, "chiave_bozza": "k-2"}, map[string]any{"allegato_id": a2, "motivo": SvincoloNodoEscluso})
	file(AzAnnulla, nil, map[string]any{"n": s1})
	file(AzRifai, nil, map[string]any{"n": s1})
	file(AzFileLasciato, nil, map[string]any{"allegato_id": a2})
	rumore()
	var soloFile []EventoGiornale
	for _, e := range tutti {
		if del[e.Azione] {
			soloFile = append(soloFile, e)
		}
	}
	atteso := []VoceVassoio{{AllegatoID: a1, Motivo: SvincoloNodoEscluso, ChiaveBozza: "k-1"}}
	if v := VassoioDa(tutti); !reflect.DeepEqual(v, atteso) {
		t.Fatalf("vassoio di tutto il giornale: %+v", v)
	}
	if v := VassoioDa(soloFile); !reflect.DeepEqual(v, atteso) {
		t.Fatalf("vassoio dei soli eventi dei file: %+v", v)
	}
}

func TestV1C4IlNodoDellaPreassociazione(t *testing.T) {
	step, altro := uuid.New(), uuid.New()
	comp := uuid.New()
	sorg := &Sorgente3{AllegatoID: &step, Gruppo: "bom"}
	cAltro := Casella3{AllegatoID: altro, Gruppo: "bom", Rif: "nodo:aa:#1"}
	cStep := Casella3{AllegatoID: step, Gruppo: "bom", Rif: "nodo:bb:#7"}
	cGruppo := Casella3{AllegatoID: step, Gruppo: "altro", Rif: "nodo:bb:#8"}
	n := Nodo3{ChiaveBozza: "k-7", ComponenteID: &comp, Op: OpRiusa, Tipo: "sottoassieme", Origine: "step",
		Caselle:  []Casella3{cAltro, cGruppo, cStep},
		Identita: Identita{Emittente: "cliente:x", Codice: "ACM-1100", Rev: "A", Modo: ModoRegistrato}}
	got := nodoDellaBozza(n, sorg)
	if got.Codice != "ACM-1100" || got.Rev != "A" || got.ComponenteID == nil || *got.ComponenteID != comp || got.Modo != ModoRegistrato ||
		got.Casella == nil || *got.Casella != cStep || len(got.Caselle) != 3 {
		t.Fatalf("nodo con la casella dello STEP sorgente: %+v", got)
	}
	// la casella non e' una copia condivisa: cambiarla non cambia il nodo
	got.Caselle[0].Rif = "cambiato"
	if n.Caselle[0].Rif != "nodo:aa:#1" {
		t.Fatal("la lettura condivide le caselle dello snapshot")
	}
	// senza la casella della sorgente (o senza sorgente): la prima
	if got := nodoDellaBozza(n, nil); got.Casella == nil || *got.Casella != cAltro {
		t.Fatalf("nodo senza sorgente: %+v", got.Casella)
	}
	// un nodo manuale: nessuna casella
	m := Nodo3{ChiaveBozza: "k-man", Op: OpCrea, Tipo: "commerciale", Origine: "utente", Identita: Identita{Emittente: "cliente:x", Codice: "ACM-9", Modo: ModoManuale}}
	if got := nodoDellaBozza(m, sorg); got.Casella != nil || len(got.Caselle) != 0 || got.Codice != "ACM-9" || got.Rev != "" {
		t.Fatalf("nodo manuale: %+v", got)
	}
}

func TestV1C4LeQueryDellaLetturaNonLeggonoLaCopia(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "platform", "db", "queries", "v1_lettura_bozze.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// i commenti possono nominarla; le istruzioni no
	var sql []string
	for _, riga := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(riga), "--") {
			sql = append(sql, riga)
		}
	}
	corpo := strings.Join(sql, "\n")
	if regexp.MustCompile(`(?i)documento_proposta|componente_id`).MatchString(corpo) {
		t.Fatalf("la lettura delle bozze legge la copia:\n%s", corpo)
	}
	for _, q := range []string{"-- name: ListBozzeAperteDellaRfq :many", "-- name: ListEventiFileBozzeAperteDellaRfq :many"} {
		if !strings.Contains(string(raw), q) {
			t.Fatalf("manca %q", q)
		}
	}
}
