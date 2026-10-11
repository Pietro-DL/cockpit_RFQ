package web

// L1 — giro 5, A1d: l'anteprima JSON dell'autorizzazione dello STEP strutturale (BE-11; contratto A1d §3.4 A; v1.1
// §4.8; piano A1d backend §5.6 e §7.2; T-A1d-03, T-A1d-28; RAC-10). anteprimaStepDa su effetti ACME scritti a mano:
// le frasi e le scelte del modulo legacy fasc_autorizzazione, nello stesso ordine, e la prova che sono ancora quelle
// (la deriva, contro il modulo reso dal suo template); gli elenchi [] e mai null; la firma e «inviabile» solo dal
// ruolo operatore in su; la forma del JSON, campo per campo con il suo tag. Il gestore: i rifiuti della richiesta
// sono 200 con errore e non leggono il database (Pool nil). La transazione: READ ONLY e REPEATABLE READ, chiusa con il
// rollback, un 25006 nel log, provata con un iniziatore finto; la stessa prova sul database vero è A1d-L4-05
// (motore_a_db_test.go, A1d-Q3, NON ESEGUIBILE QUI).
//
// I clienti di questi test sono inventati: codici 712xxxx, il file 7120100_C.stp, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlesc "html"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// maStepFirma: una firma inventata, 24 cifre esadecimali come quelle di Effetto.firmata.
const maStepFirma = "a1d0a1d0a1d0a1d0a1d0a1d0"

// maStepID: un UUID ACME, 00000000-0000-4000-8000-0000000000nn.
func maStepID(nn int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", nn))
}

// maStepEffetti: gli effetti ACME delle prove, uno per forma del riquadro, con i soli campi esportati (quelli che
// EffettoAutorizzazione riempie): spento; più radici e nessuna indicata; la sorgente con figli, guida, rimozioni,
// revoche, avvisi, presa d'atto, un raggruppamento e due deleghe (una spenta); «già» con una delega da scegliere e
// «già» senza scelte; la delega dal file del padre, con il titolare.
func maStepEffetti() map[string]fascicolo.Effetto {
	prodotto := db.Componente{ComponenteID: maStepID(10), ThreadID: maStepID(1), Codice: "7120100", Tipo: db.TipoComponenteFinito}
	sotto := db.Componente{ComponenteID: maStepID(11), ThreadID: maStepID(1), Codice: "7120110", Tipo: db.TipoComponenteSottoassieme}
	doc := db.Documento{DocumentoID: maStepID(41), ThreadID: maStepID(1), NomeFile: "7120100_C.stp"}
	sha := strings.Repeat("c1", 32)
	radice := fascicolo.NodoEffetto{Chiave: "#1", Nome: "7120100", Codice: "7120100", Figli: 3, Stato: db.StatoPropostaAperta}
	figli := []fascicolo.NodoEffetto{
		{Chiave: "#2", Nome: "7120110", Codice: "7120110", Qta: 2, Figli: 1, Deciso: "7120110", Stato: db.StatoPropostaConfermata},
		{Chiave: "#3", Nome: "7129020", Codice: "7129020", Qta: 8, Stato: db.StatoPropostaAperta},
		{Chiave: "#4", Nome: "ACME-CONTENITORE", Figli: 2, Stato: db.StatoPropostaAperta},
	}
	nodo6 := fascicolo.NodoEffetto{Chiave: "#6", Nome: "7120160", Codice: "7120160", Qta: 1, Figli: 2, Stato: db.StatoPropostaAperta}
	nodo7 := fascicolo.NodoEffetto{Chiave: "#7", Nome: "7120170", Codice: "7120170", Qta: 1, Stato: db.StatoPropostaAperta}
	delegaDecisa := fascicolo.DelegaPossibile{Nodo: figli[0], ComponenteID: maStepID(11), Codice: "7120110",
		Revoca: []string{"7120110_A.stp (STEP autorizzato di 7120110)"}}
	delegaSpenta := fascicolo.DelegaPossibile{Nodo: nodo6, Spenta: "il nodo non è ancora un componente: si delega quando una persona lo decide"}
	delegato := figli[0]
	return map[string]fascicolo.Effetto{
		"spento": {Componente: prodotto, Spento: "7120100 è archiviato: si ripristina prima di autorizzare un suo STEP", Firma: maStepFirma},
		"radici": {Componente: prodotto, Documento: doc, Allegato: maStepID(31), Sha256: sha, NomeFile: "7120100_C.stp",
			Radici: []fascicolo.NodoEffetto{radice, {Chiave: "#5", Nome: "7120105", Codice: "7120105", Figli: 1, Stato: db.StatoPropostaAperta}},
			Firma:  maStepFirma},
		"sorgente": {Componente: prodotto, Documento: doc, Allegato: maStepID(31), Sha256: sha, NomeFile: "7120100_C.stp",
			Radici: []fascicolo.NodoEffetto{radice}, Sorgente: &radice,
			PresaDAtto:     "il file chiama la radice «PRODOTTO ACME»: per questa RFQ è 7120100",
			Raggruppamenti: []fascicolo.Raggruppamento{{Nodo: figli[2], Figli: []fascicolo.NodoEffetto{nodo6, nodo7}}},
			Figli:          figli, Deleghe: []fascicolo.DelegaPossibile{delegaDecisa, delegaSpenta},
			Guida: 3, GuidaSotto: []string{"sotto 7120110: 1"},
			Rimozioni: []string{"7120100 → 7120200 ×1: il file non ha il legame"},
			Revoche:   []string{"7120100_B.stp (STEP autorizzato di 7120100)"},
			Avvisi:    []string{"è un prodotto finito: il file diventa anche il suo STEP strutturale"},
			Firma:     maStepFirma},
		"gia_con_scelte": {Componente: prodotto, Documento: doc, Allegato: maStepID(31), Sha256: sha, NomeFile: "7120100_C.stp",
			Gia:    "7120100_C.stp è già lo STEP autorizzato di 7120100",
			Radici: []fascicolo.NodoEffetto{radice}, Sorgente: &radice, Figli: figli[:1],
			Deleghe: []fascicolo.DelegaPossibile{delegaDecisa}, Guida: 1,
			RimozioniSospese: "si calcolano quando una persona ha deciso tutti i figli diretti", Firma: maStepFirma},
		"gia_senza_scelte": {Componente: prodotto, Documento: doc, Allegato: maStepID(31), Sha256: sha, NomeFile: "7120100_C.stp",
			Gia:    "7120100_C.stp è già lo STEP autorizzato di 7120100",
			Radici: []fascicolo.NodoEffetto{radice}, Sorgente: &radice, Figli: figli, Firma: maStepFirma},
		"delega": {Componente: sotto, Documento: doc, Titolare: "7120100", Allegato: maStepID(31), Sha256: sha,
			NomeFile: "7120100_C.stp", Delega: true, Sorgente: &delegato,
			Figli: []fascicolo.NodoEffetto{{Chiave: "#8", Nome: "7120111", Codice: "7120111", Qta: 4, Stato: db.StatoPropostaAperta}},
			Guida: 5, Firma: maStepFirma},
	}
}

// Le frasi, le scelte, il bottone e i due booleani di ogni forma del riquadro, scritti per intero: il titolo, «già»,
// la domanda delle radici, i figli diretti (1 al singolare, 0 e 3 al plurale), la guida con «sotto …», rimozioni,
// rimozioni sospese, revoche, avvisi; il riquadro spento ha solo il titolo e il motivo, senza firma; con più radici
// nessuna è scelta e niente è inviabile; con «già» il bottone è «Autorizza le scelte» solo se c'è da scegliere.
func TestAnteprimaStepDaEffetto(t *testing.T) {
	ef := maStepEffetti()
	vuoti := func(a anteprimaStepMA) anteprimaStepMA { return a.conElenchi() }
	atteso := map[string]anteprimaStepMA{
		"spento": vuoti(anteprimaStepMA{Spento: "7120100 è archiviato: si ripristina prima di autorizzare un suo STEP",
			Frasi: []string{"Autorizzazione: lo STEP per 7120100"}}),
		"radici": vuoti(anteprimaStepMA{
			Frasi: []string{"Autorizzazione: 7120100_C.stp per 7120100", "Il file ha 2 radici: quale è 7120100? Nessuna è scelta."},
			Firma: maStepFirma,
			Sorgenti: []sceltaMA{{Chiave: "#1", Etichetta: "7120100 (3 figli nel file)"},
				{Chiave: "#5", Etichetta: "7120105 (1 figli nel file)"}}}),
		"sorgente": vuoti(anteprimaStepMA{Autorizzabile: true, Inviabile: true,
			Frasi: []string{
				"Autorizzazione: 7120100_C.stp per 7120100",
				"3 figli diretti entrano nell'autorità di 7120100:",
				"3 nodi del file restano guida · sotto 7120110: 1: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
				"Rimozione: 7120100 → 7120200 ×1: il file non ha il legame",
				"Un componente, un'autorizzazione: 7120100_B.stp (STEP autorizzato di 7120100) si revoca.",
				"è un prodotto finito: il file diventa anche il suo STEP strutturale.",
			},
			Figli:      ef["sorgente"].Figli,
			Bottone:    "Autorizza 7120100_C.stp per 7120100: 3 figli diretti",
			Firma:      maStepFirma,
			PresaDAtto: "il file chiama la radice «PRODOTTO ACME»: per questa RFQ è 7120100",
			Raggruppamenti: []sceltaMA{{Chiave: "#4",
				Etichetta: "ACME-CONTENITORE è 7120100 (un contenitore): i suoi 2 figli diventano figli diretti"}},
			Deleghe: []delegaMA{
				{Chiave: "#2", Etichetta: "Autorizza questo STEP anche per 7120110 (7120110, 1 figli nel file)",
					Revoca: []string{"7120110_A.stp (STEP autorizzato di 7120110)"}},
				{Chiave: "#6", Etichetta: "Autorizza questo STEP anche per il componente di 7120160, quando sarà deciso (7120160, 2 figli nel file)",
					Spenta: "il nodo non è ancora un componente: si delega quando una persona lo decide", Revoca: []string{}},
			}}),
		"gia_con_scelte": vuoti(anteprimaStepMA{Gia: "7120100_C.stp è già lo STEP autorizzato di 7120100", Inviabile: true,
			Frasi: []string{
				"Autorizzazione: 7120100_C.stp per 7120100",
				"7120100_C.stp è già lo STEP autorizzato di 7120100.",
				"1 figlio diretto entra nell'autorità di 7120100:",
				"1 nodo del file resta guida: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
				"Rimozioni: si calcolano quando una persona ha deciso tutti i figli diretti.",
			},
			Figli:   ef["gia_con_scelte"].Figli,
			Bottone: "Autorizza le scelte",
			Firma:   maStepFirma,
			Deleghe: []delegaMA{{Chiave: "#2", Etichetta: "Autorizza questo STEP anche per 7120110 (7120110, 1 figli nel file)",
				Revoca: []string{"7120110_A.stp (STEP autorizzato di 7120110)"}}}}),
		"gia_senza_scelte": vuoti(anteprimaStepMA{Gia: "7120100_C.stp è già lo STEP autorizzato di 7120100",
			Frasi: []string{
				"Autorizzazione: 7120100_C.stp per 7120100",
				"7120100_C.stp è già lo STEP autorizzato di 7120100.",
				"3 figli diretti entrano nell'autorità di 7120100:",
				"0 nodi del file restano guida: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
			},
			Figli: ef["gia_senza_scelte"].Figli,
			Firma: maStepFirma}),
		"delega": vuoti(anteprimaStepMA{Autorizzabile: true, Inviabile: true,
			Frasi: []string{
				"Delega: 7120100_C.stp per 7120110 (nel file, sotto 7120100)",
				"1 figlio diretto entra nell'autorità di 7120110:",
				"5 nodi del file restano guida: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
			},
			Figli:   ef["delega"].Figli,
			Bottone: "Autorizza 7120100_C.stp anche per 7120110: 1 figlio diretto",
			Firma:   maStepFirma}),
	}
	if len(atteso) != len(ef) {
		t.Fatalf("%d effetti, %d attesi", len(ef), len(atteso))
	}
	for caso, e := range ef {
		t.Run(caso, func(t *testing.T) {
			out := anteprimaStepDa(e, true)
			if !reflect.DeepEqual(out, atteso[caso]) {
				t.Errorf("anteprima:\n %#v\natteso:\n %#v", out, atteso[caso])
			}
			if out.Autorizzabile != e.Autorizzabile() {
				t.Errorf("autorizzabile %v, Effetto.Autorizzabile() %v", out.Autorizzabile, e.Autorizzabile())
			}
			// gli elenchi ci sono sempre, anche vuoti, e la revoca di ogni delega: [] e mai null (v1.1 §4.8)
			b, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			m := maStepChiavi(t, b)
			for _, k := range []string{"frasi", "figli", "sorgenti", "raggruppamenti", "deleghe"} {
				if !bytes.HasPrefix(m[k], []byte("[")) {
					t.Errorf("%s = %s, atteso un elenco", k, m[k])
				}
			}
			var deleghe []map[string]json.RawMessage
			if err := json.Unmarshal(m["deleghe"], &deleghe); err != nil {
				t.Fatal(err)
			}
			for i, d := range deleghe {
				if !bytes.HasPrefix(d["revoca"], []byte("[")) {
					t.Errorf("deleghe[%d].revoca = %s, atteso un elenco", i, d["revoca"])
				}
			}
		})
	}
}

// La firma serve solo a chi può mandare il gesto: dal ruolo operatore in su, con la stessa regola del gestore
// (almeno(utenteDa(ctx), operatore)), come nell'anteprima del Fascicolo e del tipo (distinta.go:839). Chi consulta, o
// un ruolo che non si riconosce, vede lo stesso riquadro senza la firma e senza niente di inviabile, e nel JSON la
// chiave firma non c'è (M-A1d-18: «Firma sempre» fa cadere questa prova).
func TestAnteprimaStepFirmaSoloDaOperatore(t *testing.T) {
	ef := maStepEffetti()
	for _, c := range []struct {
		nome  string
		u     *db.Utente
		firma bool
	}{
		{"senza utente", nil, false},
		{"consultazione", &db.Utente{Ruolo: db.RuoloUtenteConsultazione}, false},
		{"ruolo sconosciuto", &db.Utente{Ruolo: db.RuoloUtente("boh")}, false},
		{"operatore", &db.Utente{Ruolo: db.RuoloUtenteOperatore}, true},
		{"tecnico", &db.Utente{Ruolo: db.RuoloUtenteTecnico}, true},
		{"admin", &db.Utente{Ruolo: db.RuoloUtenteAdmin}, true},
	} {
		t.Run(c.nome, func(t *testing.T) {
			if got := almeno(c.u, db.RuoloUtenteOperatore); got != c.firma {
				t.Fatalf("almeno(%v, operatore) = %v", c.u, got)
			}
			inviabili := 0
			for caso, e := range ef {
				out := anteprimaStepDa(e, almeno(c.u, db.RuoloUtenteOperatore))
				conFirma := c.firma && e.Spento == ""
				if (out.Firma != "") != conFirma || (conFirma && out.Firma != e.Firma) {
					t.Errorf("%s: firma %q (attesa: %v)", caso, out.Firma, conFirma)
				}
				if out.Inviabile && !c.firma {
					t.Errorf("%s: inviabile senza la firma", caso)
				}
				if out.Inviabile {
					inviabili++
				}
				b, err := json.Marshal(out)
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := maStepChiavi(t, b)["firma"]; ok != conFirma {
					t.Errorf("%s: la chiave firma nel JSON: %v (attesa: %v)", caso, ok, conFirma)
				}
				// il resto del riquadro è lo stesso per tutti: chi consulta vede l'effetto, non lo manda
				pieno := anteprimaStepDa(e, true)
				out.Firma, out.Inviabile, pieno.Firma, pieno.Inviabile = "", false, "", false
				if !reflect.DeepEqual(out, pieno) {
					t.Errorf("%s: senza la firma il riquadro cambia:\n %#v\n %#v", caso, out, pieno)
				}
			}
			// sorgente, gia_con_scelte e delega sono inviabili per chi firma (TestAnteprimaStepDaEffetto)
			if c.firma && inviabili != 3 {
				t.Errorf("con la firma %d riquadri inviabili, attesi 3", inviabili)
			}
		})
	}
}

// La deriva (T-A1d-28): lo stesso Effetto reso dal modulo legacy come lo rende anteprimaAutorizzazione
// (fascicolo_rotte.go:369, con Scrive vero). I paragrafi del modulo sono, nello stesso ordine, le frasi del JSON (più
// il motivo del riquadro spento); le radici da indicare sono le voci dell'elenco; raggruppamenti, deleghe, presa
// d'atto sono etichette del modulo; il motivo di una delega spenta e le sue revoche sono note del modulo; il bottone
// c'è quando il JSON dice inviabile, con lo stesso testo. Se il template cambia una riga, la prova cade.
func TestAnteprimaStepLeFrasiSonoQuelleDelModuloLegacy(t *testing.T) {
	s := serverTest(t)
	tag := regexp.MustCompile(`<[^>]*>`)
	testo := func(h string) string {
		return strings.Join(strings.Fields(htmlesc.UnescapeString(tag.ReplaceAllString(h, " "))), " ")
	}
	elementi := func(h, nome string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`(?s)<`+nome+`\b[^>]*>(.*?)</`+nome+`>`).FindAllStringSubmatch(h, -1) {
			out = append(out, testo(m[1]))
		}
		return out
	}
	thread := maStepID(1)
	for caso, e := range maStepEffetti() {
		t.Run(caso, func(t *testing.T) {
			v := autorizzazioneVista{Base: "/thread/" + thread.String() + "/fascicolo", Cid: e.Componente.ComponenteID, E: e, Scrive: true}
			var buf bytes.Buffer
			if err := s.pagine["fascicolo.html"].ExecuteTemplate(&buf, "fasc_autorizzazione", vista{Dati: v, Frammento: true}); err != nil {
				t.Fatal(err)
			}
			h := buf.String()
			out := anteprimaStepDa(e, true)

			paragrafi := slices.Clone(out.Frasi)
			if out.Spento != "" {
				paragrafi = append(paragrafi, out.Spento)
			}
			if got := elementi(h, "p"); !slices.Equal(got, paragrafi) {
				t.Errorf("paragrafi del modulo legacy:\n %q\nfrasi del JSON:\n %q", got, paragrafi)
			}
			if e.Sorgente == nil {
				var etichette []string
				for _, x := range out.Sorgenti {
					etichette = append(etichette, x.Etichetta)
				}
				if got := elementi(h, "li"); !slices.Equal(got, etichette) {
					t.Errorf("radici del modulo legacy %q, sorgenti del JSON %q", got, etichette)
				}
			}
			etichette := elementi(h, "label")
			note := elementi(h, "small")
			for _, x := range out.Raggruppamenti {
				if !slices.Contains(etichette, x.Etichetta) {
					t.Errorf("raggruppamento %q non è un'etichetta del modulo legacy %q", x.Etichetta, etichette)
				}
			}
			for _, d := range out.Deleghe {
				if !slices.Contains(etichette, d.Etichetta) {
					t.Errorf("delega %q non è un'etichetta del modulo legacy %q", d.Etichetta, etichette)
				}
				if d.Spenta != "" && !slices.Contains(note, d.Spenta) {
					t.Errorf("delega spenta %q non è una nota del modulo legacy %q", d.Spenta, note)
				}
				for _, x := range d.Revoca {
					if r := "Un componente, un'autorizzazione: " + x + " si revoca."; !slices.Contains(note, r) {
						t.Errorf("revoca della delega %q non è una nota del modulo legacy %q", r, note)
					}
				}
			}
			if out.PresaDAtto != "" && !slices.Contains(etichette, "Presa d'atto: "+out.PresaDAtto) {
				t.Errorf("presa d'atto %q non è un'etichetta del modulo legacy %q", out.PresaDAtto, etichette)
			}
			bottoni := elementi(h, "button")
			if out.Inviabile != (len(bottoni) == 1) || (out.Inviabile && bottoni[0] != out.Bottone) {
				t.Errorf("bottoni del modulo legacy %q; inviabile %v, bottone %q", bottoni, out.Inviabile, out.Bottone)
			}
		})
	}
}

// maStepChiavi: le chiavi di primo livello di un oggetto JSON, con il loro valore grezzo.
func maStepChiavi(t *testing.T, b []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON %s: %v", b, err)
	}
	return m
}

// maStepChiama chiama il gestore dell'anteprima con il percorso che monta registraMotoreA ({id} nel percorso) e
// l'utente nel contesto. Il Pool è nil: una lettura del database andrebbe in panic, e la prova lo dice.
func maStepChiama(t *testing.T, id, query string, u *db.Utente) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Log: slog.New(slog.DiscardHandler)}
	req := httptest.NewRequest(http.MethodGet, "/thread/"+id+"/motore-a/anteprima/step-strutturale?"+query, nil)
	req.SetPathValue("id", id)
	if u != nil {
		req = req.WithContext(context.WithValue(req.Context(), ctxUtente, u))
	}
	rec := httptest.NewRecorder()
	var panico any
	func() {
		defer func() { panico = recover() }()
		s.motoreAAnteprimaStep(rec, req)
	}()
	if panico != nil {
		t.Fatalf("il gestore ha letto il database (Pool nil): %v", panico)
	}
	return rec
}

// Un componente che non è un UUID: 200 con errore, come distintaTipo (distinta.go:822-824), gli elenchi [] e nessuna
// lettura del database; JSON con application/json; charset=utf-8, no-store e nosniff.
func TestAnteprimaStepComponenteNonValido(t *testing.T) {
	admin := &db.Utente{Ruolo: db.RuoloUtenteAdmin}
	for _, query := range []string{"componente=xyz", "", "componente=xyz&documento=" + maStepID(41).String(), "documento=" + maStepID(41).String()} {
		t.Run(query, func(t *testing.T) {
			rec := maStepChiama(t, maStepID(1).String(), query, admin)
			if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" ||
				rec.Header().Get("X-Content-Type-Options") != "nosniff" { // nosniff: I-02 della revisione di A1d-P2
				t.Fatalf("%d %q %q nosniff %q: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"),
					rec.Header().Get("X-Content-Type-Options"), rec.Body)
			}
			var out anteprimaStepMA
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if want := (anteprimaStepMA{Errore: "componente non valido"}).conElenchi(); !reflect.DeepEqual(out, want) {
				t.Errorf("anteprima %#v, attesa %#v", out, want)
			}
			m := maStepChiavi(t, rec.Body.Bytes())
			for _, k := range []string{"frasi", "figli", "sorgenti", "raggruppamenti", "deleghe"} {
				if string(m[k]) != "[]" {
					t.Errorf("%s = %s, atteso []", k, m[k])
				}
			}
		})
	}
}

// Le altre risposte del gestore che non leggono il database: id che non è un UUID e form illeggibile sono 400 (come
// il gestore legacy, fascicolo_rotte.go:339-353, e sempre no-store); i rifiuti di richiestaAutorizzazione (il codice
// del Fascicolo, riusato: documento o nodo non validi, file e nodo insieme) sono 200 con errore, in parole.
func TestAnteprimaStepRichiestaNonValida(t *testing.T) {
	admin := &db.Utente{Ruolo: db.RuoloUtenteAdmin}
	cid := "componente=" + maStepID(10).String()
	for _, c := range []struct{ nome, id, query, errore string }{
		{"id", "boh", cid, ""},
		{"form", maStepID(1).String(), "componente=%zz", ""},
		{"documento", maStepID(1).String(), cid + "&documento=xyz", "documento non valido"},
		{"nodo senza chiave", maStepID(1).String(), cid + "&nodo=" + maStepID(31).String() + ":", "nodo non valido"},
		{"nodo senza allegato", maStepID(1).String(), cid + "&nodo=boh:%231", "nodo non valido"},
		{"file e nodo", maStepID(1).String(), cid + "&documento=" + maStepID(41).String() + "&nodo=" + maStepID(31).String() + ":%231",
			"si autorizza un file o si delega un nodo, non tutti e due"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			rec := maStepChiama(t, c.id, c.query, admin)
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
			}
			if c.errore == "" {
				if rec.Code != 400 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
					t.Fatalf("%d %q: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
				}
				return
			}
			if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("%d %q: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
			}
			var out anteprimaStepMA
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if want := (anteprimaStepMA{Errore: c.errore}).conElenchi(); !reflect.DeepEqual(out, want) {
				t.Errorf("anteprima %#v, attesa %#v", out, want)
			}
		})
	}
}

// La forma del JSON (contratto §3.4 A; v1.1 §4.8; RAC-10): i campi con il loro tipo e il loro tag, nell'ordine del
// piano (§5.6); le chiavi che escono per ogni forma del riquadro (omitempty solo su spento, errore, gia, bottone,
// firma, presa_d_atto e deleghe[].spenta), insieme tutte e tredici; figli[] con i tag di fascicolo.NodoEffetto.
func TestAnteprimaStepLaFormaDelJSON(t *testing.T) {
	campi := func(v any) []string {
		var out []string
		tipo := reflect.TypeOf(v)
		for i := range tipo.NumField() {
			f := tipo.Field(i)
			out = append(out, f.Name+" "+f.Type.String()+" "+f.Tag.Get("json"))
		}
		return out
	}
	for _, c := range []struct {
		v      any
		atteso []string
	}{
		{anteprimaStepMA{}, []string{"Spento string spento,omitempty", "Errore string errore,omitempty",
			"Gia string gia,omitempty", "Autorizzabile bool autorizzabile", "Inviabile bool inviabile",
			"Frasi []string frasi", "Figli []fascicolo.NodoEffetto figli", "Bottone string bottone,omitempty",
			"Firma string firma,omitempty", "PresaDAtto string presa_d_atto,omitempty", "Sorgenti []web.sceltaMA sorgenti",
			"Raggruppamenti []web.sceltaMA raggruppamenti", "Deleghe []web.delegaMA deleghe"}},
		{sceltaMA{}, []string{"Chiave string chiave", "Etichetta string etichetta"}},
		{delegaMA{}, []string{"Chiave string chiave", "Etichetta string etichetta", "Spenta string spenta,omitempty",
			"Revoca []string revoca"}},
	} {
		if got := campi(c.v); !slices.Equal(got, c.atteso) {
			t.Errorf("%T:\n %q\natteso:\n %q", c.v, got, c.atteso)
		}
	}

	chiavi := func(b []byte) []string {
		var out []string
		for k := range maStepChiavi(t, b) {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	}
	json1 := func(v any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	ef := maStepEffetti()
	tutte := map[string]bool{}
	for _, c := range []struct {
		nome   string
		b      []byte
		atteso string
	}{
		{"spento", json1(anteprimaStepDa(ef["spento"], true)), "autorizzabile deleghe figli frasi inviabile raggruppamenti sorgenti spento"},
		{"radici", json1(anteprimaStepDa(ef["radici"], true)), "autorizzabile deleghe figli firma frasi inviabile raggruppamenti sorgenti"},
		{"sorgente", json1(anteprimaStepDa(ef["sorgente"], true)),
			"autorizzabile bottone deleghe figli firma frasi inviabile presa_d_atto raggruppamenti sorgenti"},
		{"gia_con_scelte", json1(anteprimaStepDa(ef["gia_con_scelte"], true)),
			"autorizzabile bottone deleghe figli firma frasi gia inviabile raggruppamenti sorgenti"},
		{"gia_senza_scelte senza firma", json1(anteprimaStepDa(ef["gia_senza_scelte"], false)),
			"autorizzabile deleghe figli frasi gia inviabile raggruppamenti sorgenti"},
		{"errore del gestore", maStepChiama(t, maStepID(1).String(), "componente=xyz", &db.Utente{Ruolo: db.RuoloUtenteAdmin}).Body.Bytes(),
			"autorizzabile deleghe errore figli frasi inviabile raggruppamenti sorgenti"},
	} {
		got := chiavi(c.b)
		if strings.Join(got, " ") != c.atteso {
			t.Errorf("%s: chiavi %q, attese %q", c.nome, got, c.atteso)
		}
		for _, k := range got {
			tutte[k] = true
		}
	}
	if len(tutte) != 13 {
		t.Errorf("chiavi viste in tutto: %d, attese le 13 del tipo: %v", len(tutte), tutte)
	}

	// figli[] con le chiavi di NodoEffetto, mostrate come sono (RAC-10); le scelte e le deleghe con le loro
	var a struct {
		Figli          []map[string]json.RawMessage `json:"figli"`
		Raggruppamenti []map[string]json.RawMessage `json:"raggruppamenti"`
		Deleghe        []map[string]json.RawMessage `json:"deleghe"`
	}
	if err := json.Unmarshal(json1(anteprimaStepDa(ef["sorgente"], true)), &a); err != nil {
		t.Fatal(err)
	}
	var r struct {
		Sorgenti []map[string]json.RawMessage `json:"sorgenti"`
	}
	if err := json.Unmarshal(json1(anteprimaStepDa(ef["radici"], true)), &r); err != nil {
		t.Fatal(err)
	}
	ordina := func(m map[string]json.RawMessage) string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		slices.Sort(out)
		return strings.Join(out, " ")
	}
	if len(a.Figli) != 3 || len(a.Raggruppamenti) != 1 || len(a.Deleghe) != 2 || len(r.Sorgenti) != 2 {
		t.Fatalf("elenchi: %d figli, %d raggruppamenti, %d deleghe, %d sorgenti", len(a.Figli), len(a.Raggruppamenti), len(a.Deleghe), len(r.Sorgenti))
	}
	for _, c := range []struct{ nome, got, atteso string }{
		{"figli[0]", ordina(a.Figli[0]), "chiave codice deciso figli nome qta scartato stato"},
		{"figli[1]", ordina(a.Figli[1]), "chiave codice deciso figli nome qta scartato stato"},
		{"raggruppamenti[0]", ordina(a.Raggruppamenti[0]), "chiave etichetta"},
		{"sorgenti[0]", ordina(r.Sorgenti[0]), "chiave etichetta"},
		{"deleghe[0]", ordina(a.Deleghe[0]), "chiave etichetta revoca"},
		{"deleghe[1] spenta", ordina(a.Deleghe[1]), "chiave etichetta revoca spenta"},
	} {
		if c.got != c.atteso {
			t.Errorf("%s: chiavi %q, attese %q", c.nome, c.got, c.atteso)
		}
	}
	if string(a.Figli[0]["stato"]) != `"confermata"` || string(a.Figli[0]["deciso"]) != `"7120110"` || string(a.Figli[0]["qta"]) != "2" {
		t.Errorf("figli[0] %s %s %s", a.Figli[0]["stato"], a.Figli[0]["deciso"], a.Figli[0]["qta"])
	}
}

// maStepTx: una transazione finta. Le letture rispondono tutte con errore (la prima di EffettoAutorizzazione è
// GetComponente, una QueryRow): basta per vedere su quale transazione legge e come si chiude. I metodi che non sono
// scritti qui (Begin, Prepare, CopyFrom, SendBatch, …) vanno in panic sull'interfaccia nil: l'anteprima non li usa.
type maStepTx struct {
	pgx.Tx
	errore   error
	sql      []string
	rollback int
	commit   int
}

func (x *maStepTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	x.sql = append(x.sql, sql)
	return maStepRiga{x.errore}
}

func (x *maStepTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	x.sql = append(x.sql, sql)
	return nil, x.errore
}

func (x *maStepTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	x.sql = append(x.sql, sql)
	return pgconn.CommandTag{}, x.errore
}

func (x *maStepTx) Rollback(context.Context) error { x.rollback++; return nil }

func (x *maStepTx) Commit(context.Context) error { x.commit++; return nil }

// maStepRiga: la riga di una QueryRow finta, che risponde con il suo errore.
type maStepRiga struct{ err error }

func (r maStepRiga) Scan(...any) error { return r.err }

// maStepIniziatore: l'iniziatore finto. Registra le opzioni chieste e dà la transazione finta, o il suo errore.
type maStepIniziatore struct {
	opzioni []pgx.TxOptions
	tx      *maStepTx
	errore  error
}

func (i *maStepIniziatore) BeginTx(_ context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	i.opzioni = append(i.opzioni, o)
	if i.errore != nil {
		return nil, i.errore
	}
	return i.tx, nil
}

// La transazione dell'anteprima (T-A1d-03; RK-09), senza database: una sola, REPEATABLE READ e READ ONLY e
// nient'altro (le opzioni del caricatore, caricatore.go:76); EffettoAutorizzazione legge su di lei; si chiude sempre
// con il rollback, mai con il commit, anche dopo un errore. Un rifiuto del dominio torna com'è (il gestore ne fa un
// 200 con errore); un SQLSTATE 25006 torna come errore (500) e va nel log con il codice; un altro errore del database
// torna senza quella riga; se la transazione non si apre, nessuna lettura. Che s.Pool in produzione apra davvero una
// transazione in sola lettura, e che nessuna lettura di EffettoAutorizzazione scriva, lo prova A1d-L4-05 sul DB.
func TestAnteprimaStepInSolaLettura(t *testing.T) {
	ro := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	thread := maStepID(1)
	req := fascicolo.RichiestaAutorizzazione{Componente: maStepID(10), Documento: maStepID(41)}
	errRete := errors.New("connessione persa")
	errPool := errors.New("pool chiuso")
	for _, c := range []struct {
		nome       string
		lettura    error // la risposta delle letture
		apertura   error // l'errore di BeginTx
		rifiuto    string
		sqlstate   string
		letture    int
		rollback   int
		nelLog     bool
		comeErrore error
	}{
		{nome: "il componente non c'è", lettura: pgx.ErrNoRows, rifiuto: "il componente non è di questa RFQ", letture: 1, rollback: 1},
		{nome: "una lettura che scrive", lettura: &pgconn.PgError{Code: "25006", Message: "cannot execute UPDATE in a read-only transaction"},
			sqlstate: "25006", letture: 1, rollback: 1, nelLog: true},
		{nome: "un altro errore", lettura: errRete, letture: 1, rollback: 1, comeErrore: errRete},
		{nome: "la transazione non si apre", apertura: errPool, comeErrore: errPool},
	} {
		t.Run(c.nome, func(t *testing.T) {
			tx := &maStepTx{errore: c.lettura}
			ini := &maStepIniziatore{tx: tx, errore: c.apertura}
			var registro bytes.Buffer
			_, err := effettoInSolaLetturaDa(context.Background(), ini, slog.New(slog.NewTextHandler(&registro, nil)), thread, req)
			if err == nil {
				t.Fatal("nessun errore")
			}
			if len(ini.opzioni) != 1 || ini.opzioni[0] != ro {
				t.Errorf("opzioni della transazione %+v, attese una volta %+v", ini.opzioni, ro)
			}
			if len(tx.sql) != c.letture || tx.rollback != c.rollback || tx.commit != 0 {
				t.Errorf("%d letture sulla transazione, %d rollback, %d commit; attesi %d, %d, 0", len(tx.sql), tx.rollback, tx.commit, c.letture, c.rollback)
			}
			var rf rifiuto
			if isRifiuto := errors.As(err, &rf); isRifiuto != (c.rifiuto != "") || (isRifiuto && string(rf) != c.rifiuto) {
				t.Errorf("errore %v: rifiuto %v, atteso %q", err, isRifiuto, c.rifiuto)
			}
			var pg *pgconn.PgError
			if errors.As(err, &pg) != (c.sqlstate != "") {
				t.Errorf("errore %v: SQLSTATE atteso %q", err, c.sqlstate)
			}
			if c.comeErrore != nil && !errors.Is(err, c.comeErrore) {
				t.Errorf("errore %v, atteso dentro %v", err, c.comeErrore)
			}
			riga := registro.String()
			if c.nelLog != (strings.Contains(riga, "sqlstate=25006") && strings.Contains(riga, "thread="+thread.String())) {
				t.Errorf("log %q (atteso il 25006 con il thread: %v)", riga, c.nelLog)
			}
			if !c.nelLog && riga != "" {
				t.Errorf("log %q, atteso vuoto", riga)
			}
		})
	}
}
