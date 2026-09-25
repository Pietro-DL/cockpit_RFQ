package aggancio

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// L1 — le minuterie pure di questo pacchetto, quelle che decidono che cosa arriva al database.

// Il tetto delle chiavi di R0 si paga sulle References, mai sull'In-Reply-To. In una catena lunga
// l'In-Reply-To e' il messaggio a cui si risponde davvero, e prima era la prima cosa a sparire: stava
// in testa all'elenco, e l'elenco si tagliava tenendo la coda.
func TestLInReplyToNonSiPerdeNelTettoDelleChiavi(t *testing.T) {
	const irt = "<risposta-a@acme.example>"
	var rif []string
	for i := 0; i < 60; i++ {
		rif = append(rif, fmt.Sprintf("<rif-%02d@acme.example>", i))
	}
	chiavi := ChiaviCitate(irt, rif)
	if len(chiavi) > maxChiaviCitate {
		t.Fatalf("%d chiavi: il tetto e' %d", len(chiavi), maxChiaviCitate)
	}
	for _, forma := range []string{"<risposta-a@acme.example>", "risposta-a@acme.example"} {
		if !contiene(chiavi, forma) {
			t.Errorf("l'In-Reply-To %q e' stato tagliato dal tetto: %v", forma, chiavi)
		}
	}
	// delle References restano le piu' recenti, che stanno in fondo
	if !contiene(chiavi, "<rif-59@acme.example>") || contiene(chiavi, "<rif-00@acme.example>") {
		t.Errorf("delle References dovevano restare le ultime: %v", chiavi)
	}
	// con poche chiavi non si taglia niente, e l'ordine resta: prima l'In-Reply-To
	poche := ChiaviCitate(irt, []string{"<rif-a@acme.example>"})
	if len(poche) != 4 || poche[0] != "<risposta-a@acme.example>" {
		t.Errorf("chiavi senza tetto: %v", poche)
	}
}

// Il taglio e' a caratteri: le colonne sono varchar(n), che PostgreSQL conta in caratteri, e un
// taglio a byte in mezzo a una lettera accentata lascia UTF-8 non valido. Il database lo rifiuta e il
// messaggio intero finiva in scarto per la descrizione di una famiglia di codici.
func TestIlTaglioEACaratteriENonRompeLeAccentate(t *testing.T) {
	// 71 caratteri e 141 byte: con il taglio a 120 byte il 120esimo byte cadeva a meta' di una «è»
	famiglia := "a" + strings.Repeat("è", 70)
	if len(famiglia) <= 120 {
		t.Fatalf("la prova ha senso solo oltre 120 byte: %d", len(famiglia))
	}
	if got := tronca(famiglia, 120); got != famiglia {
		t.Errorf("71 caratteri stanno in varchar(120): tagliata a %d caratteri", utf8.RuneCountInString(got))
	}
	lunga := strings.Repeat("àè", 70) // 140 caratteri
	got := tronca(lunga, 120)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 120 {
		t.Errorf("taglio di 140 caratteri a 120: valido=%v, %d caratteri", utf8.ValidString(got), utf8.RuneCountInString(got))
	}
	if got := tronca("ÀBC-123-ÈÉ", 5); got != "ÀBC-1" {
		t.Errorf("tronca(ÀBC-123-ÈÉ, 5) = %q", got)
	}
}

func contiene(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}

// La revisione scritta adesso vince su quella citata sotto: candidato_codice tiene l'ultima riga
// scritta per (messaggio, codice), quindi la storia va scritta per prima.
func TestLaRevisioneDellaStoriaNonCopreQuellaNuova(t *testing.T) {
	cc := []classificazione.CodiceTrovato{
		{Codice: "PZ-001", Rev: "C", Dove: "corpo"},
		{Codice: "PZ-001", Rev: "B", Dove: classificazione.DoveStoria},
		{Codice: "PZ-002", Rev: "A", Dove: "allegato PZ-002_A.pdf"},
	}
	out := perSalvare(cc)
	if len(out) != len(cc) {
		t.Fatalf("perSalvare ha perso o aggiunto codici: %+v", out)
	}
	ultima := map[string]classificazione.CodiceTrovato{}
	for _, c := range out {
		ultima[c.Codice] = c // come fa ON CONFLICT DO UPDATE: vince l'ultima
	}
	if r := ultima["PZ-001"]; r.Rev != "C" || r.Dove != "corpo" {
		t.Errorf("la riga di PZ-001 sarebbe %+v: la revisione della storia copre quella del corpo", r)
	}
	if out[1].Codice != "PZ-001" || out[2].Codice != "PZ-002" {
		t.Errorf("fuori dalla storia l'ordine dell'estrazione deve restare quello: %+v", out)
	}
}
