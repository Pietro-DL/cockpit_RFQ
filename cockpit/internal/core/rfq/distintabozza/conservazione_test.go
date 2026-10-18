package distintabozza

// La tabella di conservazione del formato 3 (EM §4) come casi di prova: per ogni riga C-nn, che cosa del registro
// d'importazione e dello snapshot formato 3 si ritrova, byte per byte, dopo la rilettura. E' l'ossatura di
// TestV1ConservazioneFormato3 (PM23 §10, righe 2 e 10) per i campi che esistono al passo 2 di PM23 §14: la prova L4
// (conservazione_db_test.go) rilegge dal database, la L1 qui sotto dalla sola forma canonica. Le colonne «Conferma» e
// «R7» della tabella arrivano con il passo 6: ogni caso le dice in «dopo», e la prova le aggiungera' allora.
// T-01 (PM23 §11) tiene legate la tabella e i casi.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// datiConservazione: quello che la prova rilegge: il registro della versione e lo snapshot formato 3 della bozza.
type datiConservazione struct {
	id       idProva3
	registro importazione.Registro
	testata  importazione.Testata // le colonne del registro (dal database nelle L4, da Colonne nella L1)
	snapshot Snapshot3
	canonico []byte // lo snapshot canonico riletto
}

type casoConservazione struct {
	id    string // la riga C-nn della tabella di conservazione (EM §4)
	cosa  string
	prova func(t *testing.T, d datiConservazione)
	dopo  string // le colonne della riga che arrivano con il passo 6 (conferma, R7)
}

// ------------------------------------------------------------------ aiuti dei casi

func (d datiConservazione) voce(t *testing.T, chiave string) importazione.Voce {
	t.Helper()
	v, ok := d.registro.Contenuto.VocePerRif(d.id.allegato, d.id.rif(chiave))
	if !ok {
		t.Fatalf("il registro non ha la voce %s", chiave)
	}
	return v
}

func (d datiConservazione) lettura(t *testing.T, chiave string) map[string]any {
	t.Helper()
	l, ok := d.registro.Contenuto.TrovaLettura(chiave)
	if !ok {
		t.Fatalf("il registro non ha la lettura %s", chiave)
	}
	_ = l
	// la lettura intera, con tutti i suoi campi, com'e' nel registro
	for _, v := range d.registro.Contenuto.Proposta.Nodi {
		for _, r := range []json.RawMessage{v.IdentitaProposta} {
			if m := oggettoJSON(t, r); m != nil && m["chiave_lettura"] == chiave {
				return m
			}
		}
		for _, x := range elencoJSON(t, v.Alternative) {
			if m, _ := x.(map[string]any); m != nil && m["chiave_lettura"] == chiave {
				return m
			}
		}
	}
	for _, f := range d.registro.Contenuto.Proposta.File {
		for _, x := range elencoJSON(t, f.Letture) {
			if m, _ := x.(map[string]any); m != nil && m["chiave_lettura"] == chiave {
				return m
			}
		}
	}
	t.Fatalf("la lettura %s non si ritrova intera", chiave)
	return nil
}

func oggettoJSON(t *testing.T, r json.RawMessage) map[string]any {
	t.Helper()
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(r, &m); err != nil {
		t.Fatalf("%s: %v", r, err)
	}
	return m
}

func elencoJSON(t *testing.T, r json.RawMessage) []any {
	t.Helper()
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	var l []any
	if err := json.Unmarshal(r, &l); err != nil {
		t.Fatalf("%s: %v", r, err)
	}
	return l
}

func uguale(t *testing.T, cosa string, got, want any) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: %v, atteso %v", cosa, got, want)
	}
}

// unisci: i valori separati da uno spazio (fmt.Sprint non mette lo spazio fra due stringhe).
func unisci(a ...any) string { return strings.TrimSuffix(fmt.Sprintln(a...), "\n") }

func (d datiConservazione) chiaveLettura(id string) string { return d.id.allegato.String() + "/" + id }

const (
	letturaTelaio = "l:u:step:#7:id:acme-num/sei:0-8"
	letturaPdf    = "l:u:pdf:1:id:beta-num/sei:0-6"
)

// ------------------------------------------------------------------ i casi

var casiConservazione = []casoConservazione{
	{id: "C-01", cosa: "emittente proposto, esplicito anche per il cliente", prova: func(t *testing.T, d datiConservazione) {
		l := d.lettura(t, d.chiaveLettura(letturaTelaio))
		uguale(t, "emittente letto", l["emittente"], d.id.clienteE())
		uguale(t, "hash della grammatica", l["hash_grammatica"], strings.Repeat("7", 64))
		uguale(t, "sorgente.importazione", d.snapshot.Sorgente.Importazione, d.registro.Impronta)
		d.voce(t, "#7") // la casella del nodo cita la voce
	}, dopo: "delta.identita[].proposta.emittente e importazione_id della conferma; dettaglio di R7"},
	{id: "C-02", cosa: "namespace proposto", prova: func(t *testing.T, d datiConservazione) {
		uguale(t, "namespace letto", d.lettura(t, d.chiaveLettura(letturaTelaio))["namespace"], "acme")
	}, dopo: "delta…proposta.namespace; R7"},
	{id: "C-03", cosa: "letterale e testo dell'unita', byte per byte", prova: func(t *testing.T, d datiConservazione) {
		l := d.lettura(t, d.chiaveLettura(idLetturaBeta))
		uguale(t, "letterale", l["letterale"], letteraleBeta)
		uguale(t, "testo dell'unita'", l["testo_unita"], letteraleBeta)
		uguale(t, "fine dell'intervallo in byte", l["intervallo"].(map[string]any)["fine"], float64(len(letteraleBeta)))
		if !bytes.Contains(d.registro.Canonico, []byte(letteraleBeta)) {
			t.Error("il registro canonico ha perso l'accento combinante")
		}
		uguale(t, "letterale della copia nella decisione", decisioneDi3(d.snapshot, "di-0001").Lettura.Letterale, letteraleBeta)
	}, dopo: "delta…proposta.letterale (byte per byte); R7"},
	{id: "C-04", cosa: "base", prova: func(t *testing.T, d datiConservazione) {
		b := d.lettura(t, d.chiaveLettura(letturaTelaio))["base"].(map[string]any)
		uguale(t, "base originale", b["originale"], "ACM-1100-S a")
		uguale(t, "base completa", b["completa"], true)
	}, dopo: "delta…proposta.base (originale, normalizzata, completa)"},
	{id: "C-05", cosa: "qualificatori", prova: func(t *testing.T, d datiConservazione) {
		q := d.lettura(t, d.chiaveLettura(letturaTelaio))["qualificatori"].(map[string]any)
		uguale(t, "marcatore", q["marcatore"], "-")
		a := q["affissi"].([]any)[0].(map[string]any)
		uguale(t, "affisso attribuito con fase e destinazione", unisci(a["attribuito"], a["fase"], a["destinazione"]), "true saldato codice")
	}, dopo: "delta…proposta.qualificatori (A-21)"},
	{id: "C-06", cosa: "famiglia, forma, selettore, stato, qualita', motivi", prova: func(t *testing.T, d datiConservazione) {
		l := d.lettura(t, d.chiaveLettura(letturaTelaio))
		uguale(t, "famiglia, forma, selettore, stato, qualita'", unisci(l["famiglia"], l["forma"], l["selettore"], l["stato_forma"], l["qualita"]),
			"num sei nodo_step.id letta piena")
		if _, ok := l["motivi"]; !ok {
			t.Error("i motivi della lettura mancano")
		}
	}, dopo: "delta…proposta.famiglia, forma, stato"},
	{id: "C-07", cosa: "revisione letta (la decisa e' identita.rev)", prova: func(t *testing.T, d datiConservazione) {
		r := d.lettura(t, d.chiaveLettura(letturaTelaio))["revisione"].(map[string]any)
		uguale(t, "revisione letta", r["originale"], "a")
		uguale(t, "rev decisa", nodoDi3(d.snapshot, "k-0007").Identita.Rev, "A")
	}, dopo: "delta…proposta.revisione"},
	{id: "C-08", cosa: "identita' decisa, con la presunzione dell'emittente", prova: func(t *testing.T, d datiConservazione) {
		r := nodoDi3(d.snapshot, "k-root").Identita
		uguale(t, "radice", unisci(r.Emittente, r.EmittentePresunto, r.Codice, r.Rev, r.Modo), d.id.clienteE()+" true ACM-1000 B registrato")
		p := nodoDi3(d.snapshot, "k-0009").Identita
		uguale(t, "nodo deciso", unisci(p.Emittente, p.EmittentePresunto, p.Namespace, p.Codice), "partner:beta false beta 400123")
		uguale(t, "nodo a mano presunto", nodoDi3(d.snapshot, "k-man").Identita.EmittentePresunto, true)
	}, dopo: "conferma_strutturale_componente (codice, rev, emittente, emittente_presunto, namespace), snapshot e mappa della conferma (M-13 e' gia' sul database)"},
	{id: "C-09", cosa: "la decisione che ha accettato o corretto l'identita', con il registro e la lettura", prova: func(t *testing.T, d datiConservazione) {
		x := decisioneDi3(d.snapshot, "di-0001")
		uguale(t, "registro della decisione", x.Importazione, d.registro.Impronta)
		l, ok := d.registro.Contenuto.TrovaLettura(x.ChiaveLettura)
		if !ok {
			t.Fatal("la lettura citata non e' nel registro")
		}
		cp, err := l.Compatta()
		if err != nil {
			t.Fatal(err)
		}
		if !importazione.StessaLettura(cp, *x.Lettura) {
			t.Error("la copia della lettura nella decisione non e' quella del registro (V-1)")
		}
		uguale(t, "registri citati", d.snapshot.RegistriCitati(), []string{d.registro.Impronta})
	}, dopo: "decisioni della conferma (accetta_proposta o la decisione), importazioni_citate; RA-06 fino a R7"},
	{id: "C-10", cosa: "letture alternative", prova: func(t *testing.T, d datiConservazione) {
		alt := elencoJSON(t, d.voce(t, "#9").Alternative)
		if len(alt) != 1 || alt[0].(map[string]any)["chiave_lettura"] != d.chiaveLettura(idLetturaBeta) {
			t.Errorf("alternative della voce #9: %v", alt)
		}
	}, dopo: "delta…alternative (numero e chiavi)"},
	{id: "C-11", cosa: "riferimenti con ruolo e provenienza, nessuna promozione implicita", prova: func(t *testing.T, d datiConservazione) {
		rr := elencoJSON(t, d.voce(t, "#1").Riferimenti)
		if len(rr) != 1 || rr[0].(map[string]any)["tipo"] != "documentale" {
			t.Errorf("riferimenti della radice: %v", rr)
		}
		uguale(t, "l'identita' della radice resta quella registrata", nodoDi3(d.snapshot, "k-root").Identita.Modo, ModoRegistrato)
	}, dopo: "delta…riferimenti[] (compatti); A-04, A-06, A-20"},
	{id: "C-12", cosa: "relazioni proposte, con tutti e due gli estremi", prova: func(t *testing.T, d datiConservazione) {
		r := d.registro.Contenuto.Proposta.Relazioni[0]
		uguale(t, "lettura sorgente", string(r.LetturaSorgente), `"`+d.chiaveLettura(letturaTelaio)+`"`)
		uguale(t, "letture bersaglio", string(r.LettureBersaglio), `["`+d.id.pdf.String()+"/"+letturaPdf+`"]`)
		uguale(t, "stato, modello, entita', riconoscimento, evidenze", string(r.Stato)+string(r.Modello)+string(r.EntitaSorgente)+string(r.Riconoscimento)+string(r.Evidenze),
			`"proposta""leg-1""e#7""legame"["u:#7"]`)
		for _, k := range []string{d.chiaveLettura(letturaTelaio), d.id.pdf.String() + "/" + letturaPdf} {
			if _, ok := d.registro.Contenuto.TrovaLettura(k); !ok {
				t.Errorf("la lettura %s di un estremo non e' nel registro", k)
			}
		}
	}, dopo: "delta.relazioni[] con ratifica non_ratificata; A-07 e A-20 fino a R7"},
	{id: "C-13", cosa: "occorrenza: allegato, sha256, nome ricevuto, contenitore, percorso", prova: func(t *testing.T, d datiConservazione) {
		f := d.registro.Contenuto.Proposta.Fonte
		uguale(t, "fonte", unisci(f.AllegatoID, f.Sha256, string(f.NomeRicevuto), string(f.PercorsoRicevuto)),
			unisci(d.id.allegato, d.id.sha, `"ACM-1000.stp"`, `"STEP/ACM-1000.stp"`))
		uguale(t, "file", d.registro.Contenuto.Proposta.File[0].AllegatoID, d.id.pdf.String())
		uguale(t, "occorrenza della casella", nodoDi3(d.snapshot, "k-0007").Caselle[0].AllegatoID, d.id.allegato)
		uguale(t, "sorgente", *d.snapshot.Sorgente.AllegatoID, d.id.allegato)
	}, dopo: "fonte_* e delta…fonte della conferma; conferma_strutturale_non_incluso.allegato_id; A-08"},
	{id: "C-14", cosa: "unita' di lettura o rif del nodo", prova: func(t *testing.T, d datiConservazione) {
		uguale(t, "unita' di lettura", d.lettura(t, d.chiaveLettura(letturaTelaio))["unita_id"], "u:"+letturaTelaio)
		uguale(t, "rif della casella", nodoDi3(d.snapshot, "k-0007").Caselle[0].Rif, d.voce(t, "#7").Origine.Rif)
	}, dopo: "snapshot e delta della conferma"},
	{id: "C-15", cosa: "terna dei fatti, digest, momento del calcolo, bundle: fuori dalle impronte di contenuto", prova: func(t *testing.T, d datiConservazione) {
		c := oggettoJSON(t, d.registro.Contenuto.Calcolo)
		if c["bundle_id"] != "b-1" || !bytes.Contains(d.registro.Contenuto.Calcolo, []byte("calcolato_il")) {
			t.Errorf("calcolo: %s", d.registro.Contenuto.Calcolo)
		}
		altro := bytes.Replace(d.registro.Canonico, []byte("2026-10-17T10:00:00Z"), []byte("2026-10-18T11:00:00Z"), 1)
		imp, err := importazione.ImpronteParziali(altro)
		if err != nil {
			t.Fatal(err)
		}
		if imp.Contenuto != d.registro.Contenuto.ImprontaContenuto || imp.Evidenze != d.registro.Contenuto.ImprontaEvidenze {
			t.Error("un orario del calcolo cambia un'impronta parziale")
		}
	}, dopo: "importazione_id della conferma; R7 (RA-04)"},
	{id: "C-16", cosa: "versioni del motore e dell'adattatore", prova: func(t *testing.T, d datiConservazione) {
		v := d.registro.Contenuto.Regole.Versioni
		uguale(t, "versioni", unisci(v.Importazione, v.Ancoraggio, v.Canonico, string(v.Valutazione)), `importazione-1 ancoraggio-3 canonico-1 "v-4"`)
	}, dopo: "importazione_id; R7 (A-18)"},
	{id: "C-17", cosa: "grammatica del cliente e partner attivi", prova: func(t *testing.T, d datiConservazione) {
		g := d.registro.Contenuto.Regole.GrammaticaCliente
		uguale(t, "grammatica del cliente", unisci(g.ClienteID, g.Hash), unisci(d.id.cliente, strings.Repeat("7", 64)))
		uguale(t, "partner attivi", d.registro.Contenuto.PartnerAttivi(), []string{"partner:beta"})
	}, dopo: "importazione_id; A-20 con il contesto e senza, A-23"},
	{id: "C-18", cosa: "contesto: id, impronta, contenuto risolto, autore del documento", prova: func(t *testing.T, d datiConservazione) {
		uguale(t, "impronta del contesto", d.registro.Contenuto.Regole.Contesto.Impronta, strings.Repeat("5", 64))
		uguale(t, "copia del contesto", oggettoJSON(t, d.registro.Contenuto.Contesto)["id"], "ctx-acme-beta")
		i := oggettoJSON(t, d.registro.Contenuto.Proposta.File[0].Interpretazione)
		uguale(t, "interpretazione del file nel perimetro", unisci(i["contesto_id"], i["autore_documento"]), "ctx-acme-beta partner:beta")
	}, dopo: "importazione_id; A-23 (anche un file fuori dal perimetro, senza i due campi)"},
	{id: "C-19", cosa: "impronte d'importazione", prova: func(t *testing.T, d datiConservazione) {
		if err := importazione.VerificaColonne(d.testata, d.registro); err != nil {
			t.Error(err)
		}
		uguale(t, "impronta della sorgente", d.snapshot.Sorgente.Importazione, d.testata.Impronta)
	}, dopo: "importazione_id della conferma; RA-02, RA-03, RA-04 (c), RA-08"},
	{id: "C-20", cosa: "chiave d'origine del nodo", prova: func(t *testing.T, d datiConservazione) {
		for _, n := range d.snapshot.Nodi {
			for _, c := range n.Caselle {
				v, ok := d.registro.Contenuto.VocePerRif(c.AllegatoID, c.Rif)
				if !ok || v.Origine.Rif != c.Rif || v.Origine.AllegatoID != c.AllegatoID.String() {
					t.Errorf("la casella di %s non cita una voce intera del registro (V-3)", n.ChiaveBozza)
				}
			}
		}
	}, dopo: "conferma_strutturale_componente.chiave_bozza e mappa; conferma_strutturale_non_incluso (RA-04)"},
	{id: "C-21", cosa: "chiave d'origine dell'arco e quantita' dello STEP", prova: func(t *testing.T, d datiConservazione) {
		a := arcoDi3(d.snapshot, "k-0007", "k-0009")
		uguale(t, "step dell'arco", unisci(a.Origine, a.Step.AllegatoID, a.Step.Gruppo), unisci("step", d.id.allegato, gruppoProva3))
		for _, r := range d.registro.Contenuto.Proposta.Archi {
			if r.Padre == d.id.rif("#7") && r.Figlio == d.id.rif("#9") {
				uguale(t, "qta = quantita' dello STEP", a.Qta, *r.Quantita)
			}
		}
	}, dopo: "conferma_strutturale_arco.origine e snapshot della conferma (M-13); R-05b, RA-08"},
	{id: "C-22", cosa: "segnale LS-02", prova: func(t *testing.T, d datiConservazione) {
		ss, err := d.registro.Contenuto.SegnaliDellaVoce(d.id.voce("#31"))
		if err != nil || len(ss) != 1 {
			t.Fatalf("segnali della voce #31: %v %v", ss, err)
		}
		uguale(t, "segnale visto = segnale del registro (V-5)", *decisioneDi3(d.snapshot, "ni-0001").SegnaleVisto, ss[0])
	}, dopo: "conferma_strutturale_non_incluso.segnale (Q-01…Q-04)"},
	{id: "C-23", cosa: "decisione «non includere»", prova: func(t *testing.T, d datiConservazione) {
		x := decisioneDi3(d.snapshot, "ni-0001")
		uguale(t, "non_includere", unisci(x.Casella.Rif, x.MotivoCodice, x.Motivo, x.AzioneID.String(), x.Importazione),
			unisci(d.id.rif("#31"), "elemento_cad", "cordone di saldatura: annotazione CAD", "00000000-0000-4000-8000-0000000e0101", d.registro.Impronta))
		for _, n := range d.snapshot.Nodi {
			for _, c := range n.Caselle {
				if c.Rif == x.Casella.Rif {
					t.Error("il nodo non incluso sta fra i nodi della bozza")
				}
			}
		}
	}, dopo: "decisioni della conferma, conferma_strutturale_non_incluso e segno sulle righe legacy (Q-05)"},
	{id: "C-24", cosa: "altre decisioni strutturali (13 del contratto rev. 3)", prova: func(t *testing.T, d datiConservazione) {
		x := decisioneDi3(d.snapshot, "dv-0001")
		uguale(t, "da_valutare", unisci(x.Tipo, x.ChiaveBozza), "da_valutare k-0007")
	}, dopo: "decisioni della conferma (come il contratto rev. 3)"},
	{id: "C-25", cosa: "preassociazioni e svincoli", prova: func(t *testing.T, d datiConservazione) {
		uguale(t, "preassociazione", unisci(d.snapshot.DocumentiPreassociati[0].PropostaID, d.snapshot.DocumentiPreassociati[0].ChiaveBozza),
			"00000000-0000-4000-8000-0000000d0001 k-0007")
		x := decisioneDi3(d.snapshot, "sv-0001")
		uguale(t, "svincolo", unisci(x.PropostaID.String(), x.ComponenteID.String()), "00000000-0000-4000-8000-0000000d0007 "+uuidD1)
	}, dopo: "associazione_evento (preassociazione, sgancia), delta.identita[].documenti, delta.svincoli[] (MAT §4.5)"},
	{id: "C-26", cosa: "fonte della bozza", prova: func(t *testing.T, d datiConservazione) {
		s := d.snapshot.Sorgente
		uguale(t, "sorgente", unisci(*s.AllegatoID, s.Sha256, s.Gruppo, s.Importazione),
			unisci(d.id.allegato, d.id.sha, d.registro.Contenuto.Gruppo.Chiave, d.registro.Impronta))
	}, dopo: "fonte_* della conferma; fonte_step_evento (R-24)"},
	{id: "C-27", cosa: "tipo proposto, classe, origine del tipo", prova: func(t *testing.T, d datiConservazione) {
		tp := oggettoJSON(t, d.voce(t, "#7").TipoProposto)
		uguale(t, "tipo proposto", unisci(tp["valore"], tp["origine"], tp["classe"]), "sottoassieme regola saldato")
		uguale(t, "tipo deciso", nodoDi3(d.snapshot, "k-0007").Tipo, "sottoassieme")
	}, dopo: "conferma_strutturale_componente.tipo"},
	{id: "C-28", cosa: "diagnostiche per nodo", prova: func(t *testing.T, d datiConservazione) {
		for _, v := range d.registro.Contenuto.Proposta.Nodi {
			if len(elencoJSON(t, v.Diagnostiche)) == 0 {
				t.Errorf("la voce %s ha perso le diagnostiche", v.Voce)
			}
		}
	}, dopo: "importazione_id (A-16)"},
	{id: "C-29", cosa: "grezzi del PRODUCT", prova: func(t *testing.T, d datiConservazione) {
		g := oggettoJSON(t, d.voce(t, "#9").Grezzi)
		uguale(t, "grezzi", unisci(g["id"], g["nome"]), "#9 pezzo #9")
	}, dopo: "importazione_id"},
	{id: "C-30", cosa: "completezza del grafo", prova: func(t *testing.T, d datiConservazione) {
		g := d.registro.Contenuto.Gruppo
		uguale(t, "grafo", string(g.GrafoCompleto)+string(g.MotivoGrafo), `false"riferimenti_mancanti"`)
	}, dopo: "importazione_id (A-19)"},
	{id: "C-31", cosa: "codice e rev proposti e codice composto", prova: func(t *testing.T, d datiConservazione) {
		v := d.voce(t, "#7")
		n := nodoDi3(d.snapshot, "k-0007").Identita
		uguale(t, "identita' proposta = codice e rev proposti", unisci(n.Modo, n.Codice, n.Rev), unisci("proposta", v.CodiceProposto, v.RevProposta))
		uguale(t, "codice composto della radice", oggettoJSON(t, d.voce(t, "#1").CodiceComposto)["testo"], "ACM-1000 B")
	}, dopo: "delta…proposta.codice_proposto, rev_proposta (A-03, A-10, A-24)"},
	{id: "C-32", cosa: "nodi fuori per decisione e stato nel motore", prova: func(t *testing.T, d datiConservazione) {
		x, ok := d.registro.Contenuto.FuoriPerRif(d.id.allegato, d.id.rif("#12"))
		if !ok {
			t.Fatal("il nodo tolto non e' fra i fuori per decisione")
		}
		uguale(t, "traccia", unisci(x.Segno, string(x.Chi), string(x.Motivo)), unisci("tolto", `"`+d.id.utente.String()+`"`, `"duplicato nel file"`))
		sm := d.voce(t, "#7").StatoMotore
		uguale(t, "stato nel motore", unisci(*sm.Raggiunto, *sm.Tolto, *sm.Deciso), "true false false")
	}, dopo: "importazione_id (A-11)"},
	{id: "C-33", cosa: "proposte d'associazione e 2D previsti all'importazione", prova: func(t *testing.T, d datiConservazione) {
		v := d.voce(t, "#7")
		if len(elencoJSON(t, v.AssociazioniProposte)) != 1 || len(elencoJSON(t, v.FabbisogniPrevisti)) != 1 {
			t.Errorf("associazioni e fabbisogni: %s %s", v.AssociazioniProposte, v.FabbisogniPrevisti)
		}
	}, dopo: "importazione_id; delta.non_inclusi[].fabbisogni_tolti_dopo_conferma (Q-04)"},
	{id: "C-34", cosa: "codice corretto a mano nella pagina vecchia", prova: func(t *testing.T, d datiConservazione) {
		m := oggettoJSON(t, d.voce(t, "#11").CodiceManualeLegacy)
		uguale(t, "codice manuale nel registro", unisci(m["codice"], m["rev"]), codiceManuale+" A")
		x := decisioneDi3(d.snapshot, "di-0002")
		uguale(t, "decisione che lo adotta", unisci(x.Riferimento, x.Importazione, x.Identita.Codice, x.Identita.Rev),
			unisci("manuale_legacy", d.registro.Impronta, codiceManuale, "A"))
		uguale(t, "presunto", nodoDi3(d.snapshot, "k-0011").Identita.EmittentePresunto, true)
	}, dopo: "decisioni della conferma e importazioni_citate; A-10b fino a R7"},
}

// eseguiCasiConservazione esegue i casi su quello che una prova ha riletto.
func eseguiCasiConservazione(t *testing.T, d datiConservazione) {
	for _, c := range casiConservazione {
		t.Run(c.id, func(t *testing.T) {
			c.prova(t, d)
			t.Logf("%s: registro e snapshot ritrovati; dopo il passo 6: %s", c.id, c.dopo)
		})
	}
}

// TestV1ConservazioneFormato3SenzaDatabase: i casi della tabella sul registro e sullo snapshot riletti dalla sola forma
// canonica (come la restituirebbe PostgreSQL). La L4 TestV1ConservazioneFormato3 li rilegge dal database.
func TestV1ConservazioneFormato3SenzaDatabase(t *testing.T) {
	id := idSintetici3()
	reg := componiProva3(t, id)
	riletto, err := importazione.Leggi(comeJsonb(t, reg.Canonico))
	if err != nil {
		t.Fatal(err)
	}
	s := leggiProva3(t, snapshotProva3(id, reg.Impronta))
	can, _ := s.Canonico()
	letto, err := RileggiVersione(FormatoSnapshot3, comeJsonb(t, can), reg.Impronta)
	if err == nil {
		t.Fatal("un'impronta che non e' quella dello snapshot passa")
	}
	imp, _ := s.Impronta()
	if letto, err = RileggiVersione(FormatoSnapshot3, comeJsonb(t, can), imp); err != nil {
		t.Fatal(err)
	}
	eseguiCasiConservazione(t, datiConservazione{id: id, registro: riletto, testata: riletto.Colonne(), snapshot: *letto.Formato3, canonico: letto.Canonico})
}

// TestV1T01LaTabellaDiConservazioneEICasi (T-01): un caso per riga, da C-01 a C-34, ognuno con la sua prova e le
// colonne che restano. Con COCKPIT_EM_V1 (il percorso dell'emendamento di V1, fuori dal repository) gli id dei casi si
// confrontano con quelli della tabella del §4: una riga senza caso, o un caso senza riga, fa fallire.
func TestV1T01LaTabellaDiConservazioneEICasi(t *testing.T) {
	visti := map[string]bool{}
	for i, c := range casiConservazione {
		atteso := fmt.Sprintf("C-%02d", i+1)
		if c.id != atteso || visti[c.id] || c.prova == nil || c.cosa == "" || c.dopo == "" {
			t.Errorf("caso %d: %q (atteso %s, con la prova, la descrizione e le colonne che restano)", i, c.id, atteso)
		}
		visti[c.id] = true
	}
	if len(casiConservazione) != 34 {
		t.Errorf("%d casi, la tabella ha 34 righe", len(casiConservazione))
	}
	t.Run("contro la tabella dell'emendamento", func(t *testing.T) {
		percorso := os.Getenv("COCKPIT_EM_V1")
		if percorso == "" {
			t.Skip("SALTATA-AMBIENTE: COCKPIT_EM_V1 non indica l'emendamento di V1 (sta fuori dal repository)")
		}
		f, err := os.Open(percorso)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		riga := regexp.MustCompile(`^\| (C-\d\d) \|`)
		inTabella := map[string]bool{}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			if m := riga.FindStringSubmatch(sc.Text()); m != nil {
				inTabella[m[1]] = true
			}
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
		var mancano, inPiu []string
		for k := range inTabella {
			if !visti[k] {
				mancano = append(mancano, k)
			}
		}
		for k := range visti {
			if !inTabella[k] {
				inPiu = append(inPiu, k)
			}
		}
		sort.Strings(mancano)
		sort.Strings(inPiu)
		if len(mancano) > 0 || len(inPiu) > 0 {
			t.Fatalf("righe senza caso %v, casi senza riga %v", mancano, inPiu)
		}
	})
}
