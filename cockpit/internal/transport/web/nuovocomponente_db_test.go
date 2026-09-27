//go:build integrazione

// L4 — Smistamento U4 (addendum A5.4.7, prova 107): il componente scritto nell'editor della struttura. Il
// codice lo scrive l'operatore; l'editor chiede prima a GET …/bom/codice che cosa la RFQ ne sa; la conferma
// rifa' gli stessi controlli sotto il lucchetto: un codice quasi uguale vuole la lettura «e' un pezzo
// diverso», uno che c'e' e' quel componente (un arco in piu', mai una seconda riga), uno della richiesta non
// diventa un componente da qui. I codici trovati nella RFQ non sono piu' carte dell'editor.

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// codiceNellEditor e' la risposta di GET …/bom/codice per il codice scritto.
func (s *scenaB87) codiceNellEditor(t *testing.T, w *browser, codice string) codiceEditor {
	t.Helper()
	resp, corpo := w.daFascicolo(http.MethodGet, s.base()+"/bom/codice?codice="+codice, nil, s.thread, "?vista=bom")
	if resp.StatusCode != 200 {
		t.Fatalf("GET bom/codice?codice=%s: %d", codice, resp.StatusCode)
	}
	var e codiceEditor
	if err := json.Unmarshal([]byte(corpo), &e); err != nil {
		t.Fatalf("la risposta non si legge (%v): %s", err, corpo)
	}
	return e
}

// Prova 107 (U4, P4, P5): il componente nuovo quasi uguale vuole la lettura; quello che c'e' si collega.
func TestIlNuovoComponenteQuasiUgualeVuoleLaLettura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("NUOVO107")
	w := operatore(b)
	s.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '77750000', 'manuale', $2)`, s.thread, s.utente)
	// un codice trovato nella RFQ, che prima era una carta «k:» dell'editor
	s.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
		VALUES ($1, '77899999', 'prodotto', '', 'generico', '', 30, 'corpo')`, s.msg)

	// i dati dell'editor: nessun codice trovato fra le carte, nessun codice precompilato da scrivere
	_, corpo := w.daFascicolo(http.MethodGet, s.base()+"/bom/dati", nil, s.thread, "?vista=bom")
	var d map[string]json.RawMessage
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatal(err)
	}
	var nodi map[string]json.RawMessage
	if err := json.Unmarshal(d["nodi"], &nodi); err != nil {
		t.Fatal(err)
	}
	for ref := range nodi {
		if strings.HasPrefix(ref, "k:") || strings.HasPrefix(ref, "n:") {
			t.Errorf("i dati dell'editor hanno la carta %s", ref)
		}
	}
	if _, c := d["trovati"]; c || strings.Contains(corpo, "77899999") {
		t.Error("i dati dell'editor portano ancora i codici trovati")
	}

	// che cosa la RFQ sa del codice scritto
	if e := s.codiceNellEditor(t, w, "77722757A"); e.Esiste != nil || e.Richiesta || len(e.Vicini) != 1 || e.Vicini[0].Codice != "77722757" || e.Vicini[0].Motivo != fascicolo.VicinoLetteraFinale {
		t.Errorf("77722757A: %+v", e)
	}
	if e := s.codiceNellEditor(t, w, "77817189"); e.Esiste == nil || e.Esiste.Ref != refC(s.particolare) || e.Esiste.Archiviato || len(e.Vicini) != 0 {
		t.Errorf("77817189 c'e' gia': %+v", e)
	}
	if e := s.codiceNellEditor(t, w, "77750000"); !e.Richiesta || e.Esiste != nil {
		t.Errorf("77750000 e' un codice della richiesta: %+v", e)
	}
	if e := s.codiceNellEditor(t, w, ""); e.Errore == "" {
		t.Errorf("il codice vuoto: %+v", e)
	}
	if e := s.codiceNellEditor(t, w, "77722758"); e.Errore != "" || e.Esiste != nil || len(e.Vicini) != 0 {
		t.Errorf("77722758 non e' vicino di 77722757: %+v", e)
	}

	// la conferma: quasi uguale senza la lettura si rifiuta, e niente e' scritto
	prima := s.archi() + " | " + s.valore(`SELECT string_agg(codice, ' ' ORDER BY codice) FROM componente WHERE thread_id = $1`, s.thread)
	v := s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: "n:77722757A", Qta: 2})
	v.Nuovi = []fascicolo.ComponenteNuovo{{Codice: "77722757A", Tipo: "sottoassieme"}}
	a, ev, _ := s.applica(w, v)
	if a != "Niente è cambiato: 77722757A è quasi uguale a 77722757 (lettera finale): se è lo stesso pezzo si usa quello; se è un pezzo diverso togli la carta di 77722757A e riscrivila con «+ Componente con codice», confermando che è un pezzo diverso" {
		t.Errorf("senza la lettura: %q", a)
	}
	if ok, _ := esitoEditor(t, ev); ok {
		t.Error("l'evento dice fatto")
	}
	if dopo := s.archi() + " | " + s.valore(`SELECT string_agg(codice, ' ' ORDER BY codice) FROM componente WHERE thread_id = $1`, s.thread); dopo != prima {
		t.Fatalf("il rifiuto ha scritto:\n  prima %s\n  dopo  %s", prima, dopo)
	}

	// con la lettura «e' un pezzo diverso» nasce: il tipo scelto, origine manuale, chi l'ha deciso
	v.Nuovi[0].Diverso = true
	if a, ev, _ := s.applica(w, v); a != "Struttura di 77722757 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Fatalf("con la lettura: %q", a)
	} else if ok, _ := esitoEditor(t, ev); !ok {
		t.Error("l'evento dice rifiutato")
	}
	if got := s.valore(`SELECT tipo::text || '/' || origine::text || '/' || (confermato_da = $2)::text FROM componente WHERE thread_id = $1 AND codice = '77722757A'`,
		s.thread, s.utente); got != "sottoassieme/manuale/true" {
		t.Errorf("il componente scritto: %s", got)
	}
	nuovo := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77722757A'`, s.thread)

	// un codice che c'e': la carta «n:» e' quel componente. Il particolare ha un secondo padre, e nessuna riga
	// nuova (l'identita' e' (thread, upper(codice)): U4)
	quanti := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, s.thread)
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(nuovo), Figlio: "n:77817189", Qta: 3})
	v.Nuovi = []fascicolo.ComponenteNuovo{{Codice: "77817189", Tipo: "sciolto"}}
	if a, _, _ := s.applica(w, v); a != "Struttura di 77722757 confermata: 1 codice scritto già nella BOM, collegato lo stesso componente, 1 legame aggiunto." {
		t.Fatalf("codice che c'e': %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, s.thread); n != quanti {
		t.Errorf("componenti: %d, erano %d", n, quanti)
	}
	if n := s.conta(`SELECT count(*) FROM componente_relazione WHERE thread_id = $1 AND figlio_id = $2`, s.thread, s.particolare); n != 3 {
		t.Errorf("i padri del particolare: %d, attesi 3 (prodotto, assieme e il nuovo)", n)
	}
	if got := s.archi(); !strings.Contains(got, "77722757A>77817189x3") || !strings.Contains(got, "77720517>77722757Ax2") {
		t.Errorf("archi: %s", got)
	}

	// un codice della richiesta non diventa un componente da qui
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.prodotto), Figlio: "n:77750000", Qta: 1})
	v.Nuovi = []fascicolo.ComponenteNuovo{{Codice: "77750000", Tipo: "sciolto"}}
	if a, _, _ := s.applica(w, v); !strings.Contains(a, "77750000 è un codice della richiesta") {
		t.Errorf("codice della richiesta: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '77750000'`, s.thread); n != 0 {
		t.Errorf("77750000 e' nato dall'editor")
	}
}
