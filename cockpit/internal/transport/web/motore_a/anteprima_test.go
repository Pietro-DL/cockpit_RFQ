package motore_a

// L1 — giro 5, A1d-Q2: l'anteprima JSON dell'autorizzazione dello STEP strutturale (direttiva A1d §3.3; contratto A1d
// §3.4 A; v1.1 §4.8; T-A1d-03, T-A1d-28; RAC-10). Derivate dalle prove della PR #18 di Stefano Valo (67dcc02:
// motore_a_anteprima_test.go), con la forma nuova del DTO (figli come NodoSTEP, la sorgente, rimozioni, revoche, guida,
// avvisi anche come campi, il motore dichiarato) e con la RFQ inesistente che è ErrRFQInesistente (F-10).
//   - AnteprimaDa su effetti ACME scritti a mano: le frasi e le scelte del modulo legacy, nello stesso ordine; gli
//     elenchi [] e mai null; la firma e «inviabile» solo con la firma. La deriva contro il modulo reso dal template
//     legacy sta nel package web (motore_a_test.go), che ha il template.
//   - La transazione: REPEATABLE READ READ ONLY, GetThread prima di tutto (pgx.ErrNoRows = ErrRFQInesistente, senza
//     nessun'altra lettura), il rifiuto del Fascicolo che torna com'è, un 25006 nel log, sempre il rollback e mai il
//     commit; con un iniziatore finto. La stessa prova sul database vero è nelle L4 (servizio_db_test.go).
//
// I clienti di questi test sono inventati: codici 712xxxx, il file 7120100_C.stp, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
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
	vuoti := func(a AnteprimaSTEP) AnteprimaSTEP { return a.conElenchi() }
	atteso := map[string]AnteprimaSTEP{
		"spento": vuoti(AnteprimaSTEP{Spento: "7120100 è archiviato: si ripristina prima di autorizzare un suo STEP",
			Frasi: []string{"Autorizzazione: lo STEP per 7120100"}}),
		"radici": vuoti(AnteprimaSTEP{
			Frasi: []string{"Autorizzazione: 7120100_C.stp per 7120100", "Il file ha 2 radici: quale è 7120100? Nessuna è scelta."},
			Firma: maStepFirma,
			Sorgenti: []Scelta{{Chiave: "#1", Etichetta: "7120100 (3 figli nel file)"},
				{Chiave: "#5", Etichetta: "7120105 (1 figli nel file)"}}}),
		"sorgente": vuoti(AnteprimaSTEP{Autorizzabile: true, Inviabile: true,
			Frasi: []string{
				"Autorizzazione: 7120100_C.stp per 7120100",
				"3 figli diretti entrano nell'autorità di 7120100:",
				"3 nodi del file restano guida · sotto 7120110: 1: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
				"Rimozione: 7120100 → 7120200 ×1: il file non ha il legame",
				"Un componente, un'autorizzazione: 7120100_B.stp (STEP autorizzato di 7120100) si revoca.",
				"è un prodotto finito: il file diventa anche il suo STEP strutturale.",
			},
			Sorgente: "#1",
			Figli:    maNodiSTEP(ef["sorgente"].Figli),
			Guida:    3, GuidaSotto: []string{"sotto 7120110: 1"},
			Rimozioni:  []string{"7120100 → 7120200 ×1: il file non ha il legame"},
			Revoche:    []string{"7120100_B.stp (STEP autorizzato di 7120100)"},
			Avvisi:     []string{"è un prodotto finito: il file diventa anche il suo STEP strutturale"},
			Bottone:    "Autorizza 7120100_C.stp per 7120100: 3 figli diretti",
			Firma:      maStepFirma,
			PresaDAtto: "il file chiama la radice «PRODOTTO ACME»: per questa RFQ è 7120100",
			Raggruppamenti: []Scelta{{Chiave: "#4",
				Etichetta: "ACME-CONTENITORE è 7120100 (un contenitore): i suoi 2 figli diventano figli diretti"}},
			Deleghe: []Delega{
				{Chiave: "#2", Etichetta: "Autorizza questo STEP anche per 7120110 (7120110, 1 figli nel file)",
					Revoca: []string{"7120110_A.stp (STEP autorizzato di 7120110)"}},
				{Chiave: "#6", Etichetta: "Autorizza questo STEP anche per il componente di 7120160, quando sarà deciso (7120160, 2 figli nel file)",
					Spenta: "il nodo non è ancora un componente: si delega quando una persona lo decide", Revoca: []string{}},
			}}),
		"gia_con_scelte": vuoti(AnteprimaSTEP{Gia: "7120100_C.stp è già lo STEP autorizzato di 7120100", Inviabile: true,
			Frasi: []string{
				"Autorizzazione: 7120100_C.stp per 7120100",
				"7120100_C.stp è già lo STEP autorizzato di 7120100.",
				"1 figlio diretto entra nell'autorità di 7120100:",
				"1 nodo del file resta guida: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
				"Rimozioni: si calcolano quando una persona ha deciso tutti i figli diretti.",
			},
			Sorgente: "#1",
			Figli:    maNodiSTEP(ef["gia_con_scelte"].Figli), Guida: 1,
			RimozioniSospese: "si calcolano quando una persona ha deciso tutti i figli diretti",
			Bottone:          "Autorizza le scelte",
			Firma:            maStepFirma,
			Deleghe: []Delega{{Chiave: "#2", Etichetta: "Autorizza questo STEP anche per 7120110 (7120110, 1 figli nel file)",
				Revoca: []string{"7120110_A.stp (STEP autorizzato di 7120110)"}}}}),
		"gia_senza_scelte": vuoti(AnteprimaSTEP{Gia: "7120100_C.stp è già lo STEP autorizzato di 7120100",
			Frasi: []string{
				"Autorizzazione: 7120100_C.stp per 7120100",
				"7120100_C.stp è già lo STEP autorizzato di 7120100.",
				"3 figli diretti entrano nell'autorità di 7120100:",
				"0 nodi del file restano guida: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
			},
			Sorgente: "#1",
			Figli:    maNodiSTEP(ef["gia_senza_scelte"].Figli),
			Firma:    maStepFirma}),
		"delega": vuoti(AnteprimaSTEP{Autorizzabile: true, Inviabile: true,
			Frasi: []string{
				"Delega: 7120100_C.stp per 7120110 (nel file, sotto 7120100)",
				"1 figlio diretto entra nell'autorità di 7120110:",
				"5 nodi del file restano guida: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
			},
			Sorgente: "#2",
			Figli:    maNodiSTEP(ef["delega"].Figli), Guida: 5,
			Bottone: "Autorizza 7120100_C.stp anche per 7120110: 1 figlio diretto",
			Firma:   maStepFirma}),
	}
	if len(atteso) != len(ef) {
		t.Fatalf("%d effetti, %d attesi", len(ef), len(atteso))
	}
	for caso, e := range ef {
		t.Run(caso, func(t *testing.T) {
			out := AnteprimaDa(e, true)
			// di che cosa parla (componente, file, documento) lo prova TestAnteprimaStepDiCheCosaParla: qui il riquadro
			senza := out
			senza.Componente, senza.Delega, senza.Titolare, senza.Allegato = nil, false, "", nil
			senza.Sha256, senza.NomeFile, senza.DocumentoID = "", "", nil
			if !reflect.DeepEqual(senza, atteso[caso]) {
				t.Errorf("anteprima:\n %#v\natteso:\n %#v", senza, atteso[caso])
			}
			if out.Motore != MotoreAnteprima {
				t.Errorf("motore %q", out.Motore)
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
			for _, k := range []string{"frasi", "figli", "sorgenti", "raggruppamenti", "deleghe", "guida_sotto", "rimozioni", "revoche", "avvisi"} {
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

// La firma serve solo a chi può mandare il gesto (l'adattatore la chiede con almeno(utenteDa(ctx), operatore), come il
// Fascicolo): senza firma lo stesso riquadro, senza la firma e senza niente di inviabile, e nel JSON la chiave firma non
// c'è.
func TestAnteprimaStepFirmaSoloDaOperatore(t *testing.T) {
	ef := maStepEffetti()
	for _, firma := range []bool{false, true} {
		inviabili := 0
		for caso, e := range ef {
			out := AnteprimaDa(e, firma)
			conFirma := firma && e.Spento == ""
			if (out.Firma != "") != conFirma || (conFirma && out.Firma != e.Firma) {
				t.Errorf("firma=%v %s: firma %q (attesa: %v)", firma, caso, out.Firma, conFirma)
			}
			if out.Inviabile && !firma {
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
				t.Errorf("firma=%v %s: la chiave firma nel JSON: %v (attesa: %v)", firma, caso, ok, conFirma)
			}
			pieno := AnteprimaDa(e, true)
			out.Firma, out.Inviabile, pieno.Firma, pieno.Inviabile = "", false, "", false
			if !reflect.DeepEqual(out, pieno) {
				t.Errorf("%s: senza la firma il riquadro cambia:\n %#v\n %#v", caso, out, pieno)
			}
		}
		if firma && inviabili != 3 {
			t.Errorf("con la firma %d riquadri inviabili, attesi 3", inviabili)
		}
	}
}

// maNodiSTEP: i figli dell'effetto nel DTO dell'anteprima, scritti qui campo per campo (non con nodoSTEP).
func maNodiSTEP(nn []fascicolo.NodoEffetto) []NodoSTEP {
	var out []NodoSTEP
	for _, n := range nn {
		out = append(out, NodoSTEP{Chiave: n.Chiave, Nome: n.Nome, Codice: n.Codice, Qta: n.Qta, Figli: n.Figli, Deciso: n.Deciso,
			Scartato: n.Scartato, Stato: string(n.Stato)})
	}
	return out
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

// La forma del JSON (contratto §3.4 A; v1.1 §4.8): i campi con il loro tipo e il loro tag; le chiavi che escono per ogni
// forma del riquadro (omitempty solo su spento, errore, gia, sorgente, rimozioni_sospese, presa_d_atto, bottone, firma e
// deleghe[].spenta); figli[] con le chiavi di NodoSTEP, che sono quelle di fascicolo.NodoEffetto; nessun tipo del dominio
// nel DTO. Il round trip con DisallowUnknownFields: il JSON non ha chiavi in più del tipo.
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
		{AnteprimaSTEP{}, []string{"Motore string motore", "Componente *motore_a.ComponenteSTEP componente",
			"Delega bool delega", "Titolare string titolare,omitempty", "Allegato *uuid.UUID allegato", "Sha256 string sha256",
			"NomeFile string nome_file", "DocumentoID *uuid.UUID documento_id",
			"Spento string spento,omitempty", "Errore string errore,omitempty",
			"Gia string gia,omitempty", "Autorizzabile bool autorizzabile", "Inviabile bool inviabile",
			"Frasi []string frasi", "Sorgente string sorgente,omitempty", "Figli []motore_a.NodoSTEP figli",
			"Sorgenti []motore_a.Scelta sorgenti", "Raggruppamenti []motore_a.Scelta raggruppamenti",
			"Deleghe []motore_a.Delega deleghe", "Guida int guida", "GuidaSotto []string guida_sotto",
			"Rimozioni []string rimozioni", "RimozioniSospese string rimozioni_sospese,omitempty", "Revoche []string revoche",
			"Avvisi []string avvisi", "PresaDAtto string presa_d_atto,omitempty", "Bottone string bottone,omitempty",
			"Firma string firma,omitempty"}},
		{NodoSTEP{}, []string{"Chiave string chiave", "Nome string nome", "Codice string codice", "Qta int32 qta",
			"Figli int figli", "Deciso string deciso", "Scartato bool scartato", "Stato string stato"}},
		{Scelta{}, []string{"Chiave string chiave", "Etichetta string etichetta"}},
		{ComponenteSTEP{}, []string{"ID uuid.UUID id", "Codice string codice"}},
		{Delega{}, []string{"Chiave string chiave", "Etichetta string etichetta", "Spenta string spenta,omitempty",
			"Revoca []string revoca"}},
	} {
		if got := campi(c.v); !slices.Equal(got, c.atteso) {
			t.Errorf("%T:\n %q\natteso:\n %q", c.v, got, c.atteso)
		}
	}
	// le chiavi di NodoSTEP sono quelle di fascicolo.NodoEffetto (lo stesso contratto della v1.1, RAC-10)
	var dom []string
	tn := reflect.TypeOf(fascicolo.NodoEffetto{})
	for i := range tn.NumField() {
		if j := tn.Field(i).Tag.Get("json"); j != "" {
			dom = append(dom, j)
		}
	}
	var nostre []string
	tn = reflect.TypeOf(NodoSTEP{})
	for i := range tn.NumField() {
		nostre = append(nostre, tn.Field(i).Tag.Get("json"))
	}
	if !slices.Equal(dom, nostre) {
		t.Errorf("chiavi di NodoSTEP %q, di fascicolo.NodoEffetto %q", nostre, dom)
	}

	chiavi := func(b []byte) string {
		var out []string
		for k := range maStepChiavi(t, b) {
			out = append(out, k)
		}
		slices.Sort(out)
		return strings.Join(out, " ")
	}
	json1 := func(v any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	sempre := "autorizzabile avvisi deleghe figli frasi guida guida_sotto inviabile motore raggruppamenti revoche rimozioni sorgenti" +
		" componente delega allegato sha256 nome_file documento_id"
	ef := maStepEffetti()
	for _, c := range []struct {
		nome   string
		b      []byte
		atteso string
	}{
		{"spento", json1(AnteprimaDa(ef["spento"], true)), sempre + " spento"},
		{"radici", json1(AnteprimaDa(ef["radici"], true)), sempre + " firma"},
		{"sorgente", json1(AnteprimaDa(ef["sorgente"], true)), sempre + " bottone firma presa_d_atto sorgente"},
		{"gia_con_scelte", json1(AnteprimaDa(ef["gia_con_scelte"], true)), sempre + " bottone firma gia rimozioni_sospese sorgente"},
		{"gia_senza_scelte senza firma", json1(AnteprimaDa(ef["gia_senza_scelte"], false)), sempre + " gia sorgente"},
		{"errore", json1(AnteprimaConErrore("componente non valido")), sempre + " errore"},
		{"delega", json1(AnteprimaDa(ef["delega"], true)), sempre + " bottone firma sorgente titolare"},
	} {
		parole := strings.Fields(c.atteso)
		slices.Sort(parole)
		if got := chiavi(c.b); got != strings.Join(parole, " ") {
			t.Errorf("%s: chiavi %q, attese %q", c.nome, got, strings.Join(parole, " "))
		}
		dec := json.NewDecoder(bytes.NewReader(c.b))
		dec.DisallowUnknownFields()
		var back AnteprimaSTEP
		if err := dec.Decode(&back); err != nil {
			t.Errorf("%s: il JSON non torna nel tipo: %v", c.nome, err)
		}
	}
	if a := AnteprimaConErrore("x"); a.Motore != MotoreAnteprima || a.Errore != "x" || a.Frasi == nil || a.Figli == nil ||
		a.Componente != nil || a.Allegato != nil || a.DocumentoID != nil {
		t.Errorf("AnteprimaConErrore: %#v", a)
	}
}

// Di che cosa parla l'anteprima (IX-04 della revisione di A1d-P2): il componente, il file (allegato, sha256, nome) e il
// documento, anche con il riquadro spento; in modo delega il file del padre e il titolare. Con lo sha256 e la chiave di
// un nodo si ricompone il Rif del motore A (ancoraggio.RifNodo), quindi la discordanza di Q-A1d-05 si calcola dal JSON.
func TestAnteprimaStepDiCheCosaParla(t *testing.T) {
	ef := maStepEffetti()
	sha := strings.Repeat("c1", 32)
	for _, c := range []struct {
		nome                string
		comp                ComponenteSTEP
		delega              bool
		titolare            string
		allegato, documento *uuid.UUID
		sha, file           string
	}{
		{"sorgente", ComponenteSTEP{ID: maStepID(10), Codice: "7120100"}, false, "", ptrUUID(maStepID(31)), ptrUUID(maStepID(41)), sha, "7120100_C.stp"},
		{"delega", ComponenteSTEP{ID: maStepID(11), Codice: "7120110"}, true, "7120100", ptrUUID(maStepID(31)), ptrUUID(maStepID(41)), sha, "7120100_C.stp"},
		{"spento", ComponenteSTEP{ID: maStepID(10), Codice: "7120100"}, false, "", nil, nil, "", ""},
	} {
		a := AnteprimaDa(ef[c.nome], false)
		if a.Componente == nil || *a.Componente != c.comp || a.Delega != c.delega || a.Titolare != c.titolare ||
			!reflect.DeepEqual(a.Allegato, c.allegato) || !reflect.DeepEqual(a.DocumentoID, c.documento) || a.Sha256 != c.sha || a.NomeFile != c.file {
			t.Errorf("%s: componente %+v, delega %v, titolare %q, allegato %v, documento %v, sha %q, file %q", c.nome,
				a.Componente, a.Delega, a.Titolare, a.Allegato, a.DocumentoID, a.Sha256, a.NomeFile)
		}
	}
	// copie: cambiare il DTO non cambia l'effetto
	e := ef["sorgente"]
	a := AnteprimaDa(e, false)
	*a.Allegato = uuid.Nil
	if e.Allegato != maStepID(31) {
		t.Error("l'anteprima condivide l'allegato con l'effetto")
	}
}

func ptrUUID(u uuid.UUID) *uuid.UUID { return &u }

// maStepTx: una transazione finta. Ogni lettura prende la risposta successiva di risposte (dopo l'ultima, l'ultima): la
// prima è GetThread, la seconda la prima lettura di EffettoAutorizzazione (GetComponente). I metodi che non sono scritti
// qui vanno in panic sull'interfaccia nil: l'anteprima non li usa.
type maStepTx struct {
	pgx.Tx
	risposte []error
	sql      []string
	rollback int
	commit   int
}

func (x *maStepTx) prossima(sql string) error {
	x.sql = append(x.sql, sql)
	if len(x.risposte) == 0 {
		return nil
	}
	i := min(len(x.sql)-1, len(x.risposte)-1)
	return x.risposte[i]
}

func (x *maStepTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	return maStepRiga{x.prossima(sql)}
}

func (x *maStepTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	return nil, x.prossima(sql)
}

func (x *maStepTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, x.prossima(sql)
}

func (x *maStepTx) Rollback(context.Context) error { x.rollback++; return nil }

func (x *maStepTx) Commit(context.Context) error { x.commit++; return nil }

// maStepRiga: la riga di una QueryRow finta, che risponde con il suo errore (nil: Scan non scrive niente).
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

// La transazione dell'anteprima (T-A1d-03; RK-09), senza database: una sola, REPEATABLE READ e READ ONLY e nient'altro;
// la prima lettura è GetThread, e una RFQ che non c'è è ErrRFQInesistente senza altre letture (F-10); poi
// EffettoAutorizzazione legge sulla stessa transazione; si chiude sempre con il rollback, mai con il commit, anche dopo un
// errore. Un rifiuto del Fascicolo torna com'è (l'adattatore ne fa un 200 con errore); un SQLSTATE 25006 torna come
// ErrLettura e va nel log con il codice; un altro errore del database torna come ErrLettura senza quella riga; se la
// transazione non si apre, nessuna lettura; senza Pool, nessuna transazione.
func TestAnteprimaStepInSolaLettura(t *testing.T) {
	ro := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	thread := maStepID(1)
	req := fascicolo.RichiestaAutorizzazione{Componente: maStepID(10), Documento: maStepID(41)}
	errRete := errors.New("connessione persa")
	errPool := errors.New("pool chiuso")
	for _, c := range []struct {
		nome        string
		risposte    []error
		apertura    error
		rifiuto     string
		inesistente bool
		sqlstate    string
		letture     int
		rollback    int
		nelLog      bool
		comeErrore  error
	}{
		{nome: "la RFQ non c'è", risposte: []error{pgx.ErrNoRows}, inesistente: true, letture: 1, rollback: 1},
		{nome: "il componente non c'è", risposte: []error{nil, pgx.ErrNoRows}, rifiuto: "il componente non è di questa RFQ", letture: 2, rollback: 1},
		{nome: "una lettura che scrive", risposte: []error{nil, &pgconn.PgError{Code: "25006", Message: "cannot execute UPDATE in a read-only transaction"}},
			sqlstate: "25006", letture: 2, rollback: 1, nelLog: true, comeErrore: ErrLettura},
		{nome: "un altro errore", risposte: []error{nil, errRete}, letture: 2, rollback: 1, comeErrore: errRete},
		{nome: "GetThread non risponde", risposte: []error{errRete}, letture: 1, rollback: 1, comeErrore: ErrLettura},
		{nome: "la transazione non si apre", apertura: errPool, comeErrore: errPool},
	} {
		t.Run(c.nome, func(t *testing.T) {
			tx := &maStepTx{risposte: c.risposte}
			ini := &maStepIniziatore{tx: tx, errore: c.apertura}
			var registro bytes.Buffer
			s := Servizio{Pool: ini, Log: slog.New(slog.NewTextHandler(&registro, nil))}
			_, err := s.AnteprimaSTEP(context.Background(), thread, req, true)
			if err == nil {
				t.Fatal("nessun errore")
			}
			if len(ini.opzioni) != 1 || ini.opzioni[0] != ro {
				t.Errorf("opzioni della transazione %+v, attese una volta %+v", ini.opzioni, ro)
			}
			if len(tx.sql) != c.letture || tx.rollback != c.rollback || tx.commit != 0 {
				t.Errorf("%d letture sulla transazione, %d rollback, %d commit; attesi %d, %d, 0", len(tx.sql), tx.rollback, tx.commit, c.letture, c.rollback)
			}
			if c.letture > 0 && !strings.Contains(tx.sql[0], "GetThread") {
				t.Errorf("la prima lettura non è GetThread: %.80q", tx.sql[0])
			}
			if errors.Is(err, ErrRFQInesistente) != c.inesistente {
				t.Errorf("errore %v: RFQ inesistente atteso %v", err, c.inesistente)
			}
			var rf fascicolo.Rifiuto
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
	if _, err := (Servizio{}).AnteprimaSTEP(context.Background(), thread, req, true); !errors.Is(err, ErrLettura) {
		t.Errorf("senza Pool: %v, atteso ErrLettura", err)
	}
}
