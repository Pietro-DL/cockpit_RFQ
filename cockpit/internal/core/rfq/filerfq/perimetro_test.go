package filerfq

// L1 — V1, ondata C, corsia file (U2 §3.10): il perimetro dei gesti su un archivio.
//   - il corpo: perimetro solo per «Metti in Altri file» e «Riattiva»; con contenitore niente n_voci e impronta_voci,
//     con contenitore_e_voci obbligatori (422 CORPO_NON_VALIDO con il campo); un valore fuori dall'elenco e' 422;
//   - le dipendenze: l'elenco e l'impronta restano quelli dell'archivio con le voci (lo stesso di S1), ma blocchi e
//     risoluzioni valgono solo per le occorrenze che il gesto scrive;
//   - «Scarta» di un archivio con una voce attiva: 422 VOCI_ATTIVE con l'elenco; senza voci attive niente rifiuto.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCorpoConIlPerimetro(t *testing.T) {
	imp := strings.Repeat("a", 64)
	base := `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","impronta_dipendenze":"` + imp + `"`
	buoni := []struct{ corpo, gesto string }{
		{base + `,"perimetro":"contenitore"}`, GestoAltriFile},
		{base + `,"perimetro":"contenitore_e_voci","n_voci":2,"impronta_voci":"` + imp + `"}`, GestoAltriFile},
		{`{"versione_attesa":1,"chiave_idempotenza":"` + chiaveV4 + `","perimetro":"contenitore"}`, GestoRiattiva},
		{base + `,"n_voci":2,"impronta_voci":"` + imp + `"}`, GestoAltriFile}, // la forma di prima
	}
	for _, b := range buoni {
		c, rf := corpo(t, b.corpo, b.gesto)
		if rf != nil {
			t.Errorf("%s: rifiutato %v", b.corpo, rf)
			continue
		}
		if strings.Contains(b.corpo, `"contenitore"`) != c.soloContenitore() {
			t.Errorf("%s: soloContenitore %v", b.corpo, c.soloContenitore())
		}
	}
	cattivi := []struct{ nome, corpo, gesto, campo string }{
		{"scarta con il perimetro", base + `,"motivo":"x","perimetro":"contenitore"}`, GestoScarta, "perimetro"},
		{"solo ZIP con n_voci", base + `,"perimetro":"contenitore","n_voci":2,"impronta_voci":"` + imp + `"}`, GestoAltriFile, "n_voci"},
		{"ZIP e contenuti senza n_voci", base + `,"perimetro":"contenitore_e_voci"}`, GestoAltriFile, "n_voci"},
		{"valore fuori elenco", base + `,"perimetro":"tutto"}`, GestoAltriFile, "perimetro"},
	}
	for _, c := range cattivi {
		_, rf := corpo(t, c.corpo, c.gesto)
		if rf == nil || rf.Codice != "CORPO_NON_VALIDO" || rf.Stato != http.StatusUnprocessableEntity || rf.Extra["campo"] != c.campo {
			t.Errorf("%s: %v", c.nome, rf)
		}
	}
}

func TestDipendenzeSoloDelleOccorrenzeScritte(t *testing.T) {
	zip, voce := uuid.New(), uuid.New()
	blocco := Dipendenza{AllegatoID: voce, Codice: "D-1", Esito: EsitoBlocca, Testo: "x"}
	scoll := Dipendenza{AllegatoID: voce, Codice: "D-7", Esito: EsitoRisoluzione, Risoluzione: RisoluzioneScollega, Testo: "x"}
	el, _ := chiudiElenco([]Dipendenza{blocco, scoll})
	solo := map[uuid.UUID]bool{zip: true}
	if rf := controllaDipendenze(el, el.Impronta, false, false, false, solo); rf != nil {
		t.Errorf("il blocco e la risoluzione di una voce fermano il gesto sul solo ZIP: %v", rf)
	}
	if rf := controllaDipendenze(el, "diversa", false, false, false, solo); rf == nil || rf.Codice != "ANTEPRIMA_SUPERATA" {
		t.Errorf("l'impronta dell'elenco intero non conta piu': %v", rf)
	}
	if rf := controllaDipendenze(el, el.Impronta, false, false, false, map[uuid.UUID]bool{zip: true, voce: true}); rf == nil ||
		rf.Codice != "DIPENDENZE_ATTIVE" {
		t.Errorf("con le voci nel gesto il blocco non vale: %v", rf)
	}
}

func TestVociAttive(t *testing.T) {
	attiva := Occorrenza{}
	attiva.AllegatoID, attiva.NomeFile = uuid.New(), "V1.pdf"
	inAltri := Occorrenza{}
	inAltri.AllegatoID, inAltri.NomeFile, inAltri.Stato = uuid.New(), "V2.pdf", StatoAltriFile
	if rf := vociAttive(Perimetro{Archivio: true, Voci: []Occorrenza{inAltri}}); rf != nil {
		t.Errorf("senza voci attive: %v", rf)
	}
	rf := vociAttive(Perimetro{Archivio: true, Voci: []Occorrenza{attiva, inAltri}})
	if rf == nil || rf.Codice != "VOCI_ATTIVE" || rf.Stato != http.StatusUnprocessableEntity || rf.Extra["n_voci_attive"] != 1 {
		t.Fatalf("con una voce attiva: %v", rf)
	}
	if !strings.Contains(rf.Testo, "1 file estratti ancora attivi") {
		t.Errorf("testo %q", rf.Testo)
	}
}
