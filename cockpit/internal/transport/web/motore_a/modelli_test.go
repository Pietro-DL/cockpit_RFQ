package motore_a

// L1 — giro 5, A1d-Q2: i modelli della pagina e il JSON di /dati (contratto A1d v1, §2.2 e §2.4). Derivate dalle prove
// della PR #18 di Stefano Valo (67dcc02: TestMotoreAComponiLaPagina, TestMotoreADatiJSON), sul sottopackage.
//   - I campi di Pagina, Testa e Fase con il loro tag: le chiavi di §2.4, e niente indice né Scrive nel JSON (Q-A1d-11).
//   - Il round trip: il JSON della pagina composta sulla scena ACME torna, con DisallowUnknownFields, in una forma
//     scritta qui con le sole chiavi del contratto; «errore» è vuoto.
//   - ComponiPagina: la testa dalla fotografia e dalla fase, l'esito e il confronto del thread, l'indice sullo stesso
//     esito, il prodotto scelto, Calcolato = PresaIl; VistaDa e ProdottoScelto.
//
// I clienti di questi test sono inventati: la scena ACME dell'indice (indice_test.go).

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/valutazione"
)

func TestModelliIcampiEITag(t *testing.T) {
	campi := func(v any) []string {
		var out []string
		tipo := reflect.TypeOf(v)
		for i := range tipo.NumField() {
			f := tipo.Field(i)
			out = append(out, f.Name+" "+f.Tag.Get("json"))
		}
		return out
	}
	for _, c := range []struct {
		v      any
		atteso []string
	}{
		{Pagina{}, []string{"ThreadID thread_id", "Vista vista", "Prodotto prodotto", "Testa testa", "Esito esito",
			"Confronto confronto", "Errore errore", "Scrive -", "Indice -", "Calcolato calcolato"}},
		{Testa{}, []string{"Cliente cliente", "Oggetto oggetto", "RiferimentoCliente riferimento_cliente",
			"StatoThread stato_thread", "Fase fase"}},
		{Fase{}, []string{"Nome nome", "Etichetta etichetta", "Inizio inizio", "GgInFase gg_in_fase", "Semaforo semaforo",
			"Terminale terminale"}},
	} {
		if got := campi(c.v); !slices.Equal(got, c.atteso) {
			t.Errorf("%T:\n %q\natteso:\n %q", c.v, got, c.atteso)
		}
	}
}

// maRisultatoACME: il risultato di un calcolo sulla scena ACME, con la fase e il confronto.
func maRisultatoACME() *Risultato {
	f, e, c := scenaIndiceACME()
	r := &Risultato{Foto: f, Esito: e, Confronto: c, Fase: &Fase{Nome: "FATTIBILITA", Etichetta: "Fattibilità ACME",
		Inizio: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC), GgInFase: 3, Semaforo: "verde"}}
	r.Thread, r.EsitoThread = &r.Foto.Thread[0], &r.Esito.Thread[0]
	return r
}

func TestComponiPagina(t *testing.T) {
	r := maRisultatoACME()
	p := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Vista: "documenti", Prodotto: maP2, Scrive: true})
	if p.ThreadID != maUUID(1) || p.Vista != VistaDocumenti || p.Prodotto != maP2 || !p.Scrive {
		t.Errorf("pagina %+v", p)
	}
	if p.Testa != (Testa{Cliente: "ACME S.p.A.", Oggetto: "RFQ 7120001", RiferimentoCliente: "ACME-030P7120001", StatoThread: "APERTA", Fase: r.Fase}) {
		t.Errorf("testa %+v", p.Testa)
	}
	if p.Esito != r.EsitoThread || p.Confronto == nil || p.Confronto.Impronta != r.Confronto.Impronta || !p.Calcolato.Equal(r.Foto.PresaIl) {
		t.Errorf("esito %p (atteso %p), confronto %v, calcolato %s", p.Esito, r.EsitoThread, p.Confronto, p.Calcolato)
	}
	if p.Indice == nil || p.Indice.Prodotto(maP1) != &r.EsitoThread.ProdottiValutati[0] {
		t.Error("l'indice non è sullo stesso esito della pagina")
	}
	if p.Errore != "" {
		t.Errorf("errore %q", p.Errore)
	}
	// un prodotto che non c'è: il primo; un risultato vuoto: la pagina senza esito, senza panic
	if q := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Prodotto: "identificativo:7120003"}); q.Prodotto != maP1 || q.Vista != VistaDistinta {
		t.Errorf("prodotto %q, vista %q", q.Prodotto, q.Vista)
	}
	if q := ComponiPagina(nil, RichiestaPagina{Thread: maUUID(1)}); q.Esito != nil || q.Indice != nil {
		t.Errorf("senza risultato: %+v", q)
	}
	for v, atteso := range map[string]string{"": VistaDistinta, "rfq": VistaRFQ, "documenti": VistaDocumenti, "distinta": VistaDistinta,
		"RFQ": VistaDistinta, "boh": VistaDistinta} {
		if got := VistaDa(v); got != atteso {
			t.Errorf("VistaDa(%q) = %q", v, got)
		}
	}
	if ProdottoScelto(nil, maP1) != "" || ProdottoScelto(&valutazione.EsitoThread{}, maP1) != "" {
		t.Error("senza prodotti il prodotto scelto è vuoto")
	}
}

// maDatiContratto: la forma del JSON di /dati con le sole chiavi del contratto §2.4.
type maDatiContratto struct {
	ThreadID string `json:"thread_id"`
	Vista    string `json:"vista"`
	Prodotto string `json:"prodotto"`
	Testa    struct {
		Cliente            string `json:"cliente"`
		Oggetto            string `json:"oggetto"`
		RiferimentoCliente string `json:"riferimento_cliente"`
		StatoThread        string `json:"stato_thread"`
		Fase               *struct {
			Nome      string    `json:"nome"`
			Etichetta string    `json:"etichetta"`
			Inizio    time.Time `json:"inizio"`
			GgInFase  int       `json:"gg_in_fase"`
			Semaforo  string    `json:"semaforo"`
			Terminale bool      `json:"terminale"`
		} `json:"fase"`
	} `json:"testa"`
	Esito     *valutazione.EsitoThread `json:"esito"`
	Confronto *confronto.Esito         `json:"confronto"`
	Errore    string                   `json:"errore"`
	Calcolato time.Time                `json:"calcolato"`
}

func TestDatiJSONNelContratto(t *testing.T) {
	p := ComponiPagina(maRisultatoACME(), RichiestaPagina{Thread: maUUID(1), Scrive: true})
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal(b, &chiavi); err != nil {
		t.Fatal(err)
	}
	var nomi []string
	for k := range chiavi {
		nomi = append(nomi, k)
	}
	slices.Sort(nomi)
	if !slices.Equal(nomi, []string{"calcolato", "confronto", "errore", "esito", "prodotto", "testa", "thread_id", "vista"}) {
		t.Errorf("chiavi del JSON %q", nomi)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var d maDatiContratto
	if err := dec.Decode(&d); err != nil {
		t.Fatalf("il JSON di /dati non sta nella forma del contratto: %v", err)
	}
	if d.ThreadID != maUUID(1).String() || d.Vista != VistaDistinta || d.Prodotto != maP1 || d.Errore != "" ||
		d.Testa.Fase == nil || d.Testa.Fase.Nome != "FATTIBILITA" || d.Esito == nil || d.Confronto == nil {
		t.Errorf("dati %+v", d)
	}
}
