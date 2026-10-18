package distintabozza

// L1 — lo snapshot formato 3 (V1, passo 2 di PM23 §14; EM §3.2-§3.7, §3.10, §7.2): la lettura stretta e le regole di
// forma, la forma canonica e la rilettura, la regola dell'NFC (N-01, la parte che non vuole il database: T-02), la
// regola unica dell'emittente contro il validatore delle grammatiche (T-14, la parte L1).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

func leggiProva3(t *testing.T, s map[string]any) Snapshot3 {
	t.Helper()
	out, err := LeggiSnapshot3(snapshotJSON(t, s))
	if err != nil {
		t.Fatalf("LeggiSnapshot3: %v", err)
	}
	return out
}

func TestV1LoSnapshotFormato3SiLeggeESiRilegge(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	s := leggiProva3(t, snapshotProva3(id, imp))
	can, err := s.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	// la rilettura dei byte canonici da' gli stessi byte (il lettore normalizza come lo scrittore, EM §3.10)
	s2, err := LeggiSnapshot3(can)
	if err != nil {
		t.Fatalf("rilettura: %v", err)
	}
	can2, _ := s2.Canonico()
	if !bytes.Equal(can, can2) {
		t.Fatalf("la rilettura cambia i byte:\n%s\n%s", can, can2)
	}
	// l'ordine degli elenchi nel corpo non conta: nodi, archi, decisioni, preassociazioni si ordinano
	alRovescio := copiaProfonda(t, snapshotProva3(id, imp))
	for _, k := range []string{"nodi", "archi", "decisioni"} {
		l := alRovescio[k].([]any)
		for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
			l[i], l[j] = l[j], l[i]
		}
	}
	s3 := leggiProva3(t, alRovescio)
	if imp3, _ := s3.Impronta(); imp3 != jsoncanonico.Impronta(can) {
		t.Fatal("l'ordine degli elenchi nel corpo cambia l'impronta")
	}
	if s.Nodi[0].ChiaveBozza != "k-0007" || s.Decisioni[0].ID != "di-0001" || s.Archi[0].Padre != "k-0007" {
		t.Fatalf("elenchi non ordinati: %s %s %s", s.Nodi[0].ChiaveBozza, s.Decisioni[0].ID, s.Archi[0].Padre)
	}
	// i campi della richiesta non stanno nello snapshot salvato; i quattro elenchi di testa si scrivono sempre
	for _, k := range []string{`"bozza_id"`, `"chiave_idempotenza"`} {
		if bytes.Contains(can, []byte(k)) {
			t.Fatalf("lo snapshot canonico porta %s", k)
		}
	}
	vuoto := copiaProfonda(t, snapshotProva3(id, imp))
	vuoto["documenti_preassociati"] = nil
	sv := leggiProva3(t, vuoto)
	cv, _ := sv.Canonico()
	if !bytes.Contains(cv, []byte(`"documenti_preassociati":[]`)) {
		t.Fatal("un elenco di testa vuoto non si scrive")
	}
	if s.Formato != FormatoSnapshot3 || VocabolarioDelFormato(FormatoSnapshot3) != 2 || VocabolarioDelFormato(FormatoSnapshot) != 1 {
		t.Fatal("formato o vocabolario")
	}
}

func TestV1LeRegoleDiFormaDelFormato3(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	altro := uuid.MustParse("00000000-0000-4000-8000-0000000a0099").String()
	casi := []struct {
		nome, campo, motivo string
		cambia              func(s map[string]any)
	}{
		{"formato 2", "formato", "", func(s map[string]any) { s["formato"] = 2 }},
		{"campo sconosciuto", "snapshot", "", func(s map[string]any) { s["uuid_inventato"] = "x" }},
		{"nome con la maiuscola", "snapshot", "", func(s map[string]any) { s["Nodi"] = s["nodi"]; delete(s, "nodi") }},
		{"campo della richiesta", "chiave_idempotenza", "", func(s map[string]any) { s["chiave_idempotenza"] = altro }},
		{"campo sconosciuto nell'identita'", "snapshot", "", func(s map[string]any) {
			nodoIn3(s, "k-0007")["identita"].(map[string]any)["proposta"] = "ACM-1100"
		}},
		{"righe di proposta del formato 2", "snapshot", "", func(s map[string]any) { nodoIn3(s, "k-0007")["righe_proposta"] = []any{altro} }},
		{"codice al primo livello del nodo", "snapshot", "", func(s map[string]any) { nodoIn3(s, "k-0007")["codice"] = "ACM-1100" }},
		{"nodo senza identita'", "nodi[1].identita", "", func(s map[string]any) { delete(nodoIn3(s, "k-0007"), "identita") }},
		{"arco senza qta", "archi[0].qta", MotivoArcoSenzaQuantita, func(s map[string]any) { delete(s["archi"].([]any)[0].(map[string]any), "qta") }},
		{"arco con qta zero", "archi[0].qta", MotivoArcoSenzaQuantita, func(s map[string]any) { s["archi"].([]any)[0].(map[string]any)["qta"] = 0 }},
		{"arco step senza step", "step", "", func(s map[string]any) { delete(s["archi"].([]any)[0].(map[string]any), "step") }},
		{"arco utente con step", "step", "", func(s map[string]any) {
			s["archi"].([]any)[3].(map[string]any)["step"] = map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3}
		}},
		{"step di un'altra occorrenza", "step", MotivoRifFuoriImportazione, func(s map[string]any) {
			s["archi"].([]any)[0].(map[string]any)["step"] = map[string]any{"allegato_id": altro, "gruppo": gruppoProva3}
		}},
		{"arco step verso un nodo senza casella", "step", "", func(s map[string]any) {
			a := s["archi"].([]any)[3].(map[string]any)
			a["origine"], a["step"] = "step", map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3}
		}},
		{"casella di un'altra occorrenza", "caselle[0]", MotivoRifFuoriImportazione, func(s map[string]any) {
			nodoIn3(s, "k-0007")["caselle"].([]any)[0].(map[string]any)["allegato_id"] = altro
		}},
		{"casella di un altro gruppo", "caselle[0]", MotivoRifFuoriImportazione, func(s map[string]any) {
			nodoIn3(s, "k-0007")["caselle"].([]any)[0].(map[string]any)["gruppo"] = "cand:x"
		}},
		{"rif di un altro sha", "rif", MotivoRifFuoriImportazione, func(s map[string]any) {
			nodoIn3(s, "k-0007")["caselle"].([]any)[0].(map[string]any)["rif"] = "nodo:" + strings.Repeat("d", 64) + ":#7"
		}},
		{"stesso rif in due nodi", "caselle", "", func(s map[string]any) {
			nodoIn3(s, "k-0009")["caselle"].([]any)[0].(map[string]any)["rif"] = id.rif("#7")
		}},
		{"proposta senza registro", "importazione", "", func(s map[string]any) {
			delete(s["sorgente"].(map[string]any), "importazione")
			s["decisioni"] = []any{}
			for _, k := range []string{"k-0009", "k-0011"} {
				nodoIn3(s, k)["identita"] = map[string]any{"emittente": id.clienteE(), "modo": "da_decidere"}
			}
		}},
		{"radice importata senza casella", "nodi", "", func(s map[string]any) { delete(nodoIn3(s, "k-root"), "caselle") }},
		{"presunzione di una proposta", "emittente_presunto", MotivoEmittentePresuntoDiverso, func(s map[string]any) {
			nodoIn3(s, "k-0007")["identita"].(map[string]any)["emittente_presunto"] = true
		}},
		{"presunzione di un nodo a mano", "emittente_presunto", MotivoEmittentePresuntoDiverso, func(s map[string]any) {
			delete(nodoIn3(s, "k-man")["identita"].(map[string]any), "emittente_presunto")
		}},
		{"presunzione di un'alternativa letta", "emittente_presunto", MotivoEmittentePresuntoDiverso, func(s map[string]any) {
			nodoIn3(s, "k-0009")["identita"].(map[string]any)["emittente_presunto"] = true
		}},
		{"presunzione del codice manuale adottato", "emittente_presunto", MotivoEmittentePresuntoDiverso, func(s map[string]any) {
			delete(nodoIn3(s, "k-0011")["identita"].(map[string]any), "emittente_presunto")
		}},
		{"nodo a mano di un partner", "emittente", "", func(s map[string]any) {
			nodoIn3(s, "k-man")["identita"].(map[string]any)["emittente"] = "partner:beta"
		}},
		{"nodo a mano con namespace", "emittente", "", func(s map[string]any) {
			nodoIn3(s, "k-man")["identita"].(map[string]any)["namespace"] = "acme"
		}},
		{"manuale con casella", "modo", "", func(s map[string]any) {
			n := nodoIn3(s, "k-0007")["identita"].(map[string]any)
			n["modo"], n["emittente_presunto"] = "manuale", true
			delete(n, "namespace")
		}},
		{"da decidere con codice", "codice", "", func(s map[string]any) {
			n := nodoIn3(s, "k-0007")["identita"].(map[string]any)
			n["modo"] = "da_decidere"
			delete(n, "namespace")
		}},
		{"registrato su un nodo da creare", "modo", "", func(s map[string]any) { nodoIn3(s, "k-0007")["identita"].(map[string]any)["modo"] = "registrato" }},
		{"modo sconosciuto", "modo", "", func(s map[string]any) { nodoIn3(s, "k-0007")["identita"].(map[string]any)["modo"] = "letto" }},
		{"decisione senza modo decisione", "decisione", "", func(s map[string]any) {
			nodoIn3(s, "k-0007")["identita"].(map[string]any)["decisione"] = "di-0001"
		}},
		{"decisione che non c'e'", "decisione", "", func(s map[string]any) {
			nodoIn3(s, "k-0009")["identita"].(map[string]any)["decisione"] = "di-9999"
			var resto []any
			for _, d := range s["decisioni"].([]any) {
				if d.(map[string]any)["id"] != "di-0001" {
					resto = append(resto, d)
				}
			}
			s["decisioni"] = resto
		}},
		{"identita' diversa da quella decisa", "nodi[2].identita", "", func(s map[string]any) {
			nodoIn3(s, "k-0009")["identita"].(map[string]any)["codice"] = "400124"
		}},
		{"decisione identita di un nodo che non la cita", "chiave_bozza", "", func(s map[string]any) {
			decisioneIn3(s, "di-0001")["chiave_bozza"] = "k-0007"
		}},
		{"decisione su una chiave che non c'e'", "chiave_bozza", "", func(s map[string]any) { decisioneIn3(s, "dv-0001")["chiave_bozza"] = "k-nessuno" }},
		{"emittente con la maiuscola", "emittente", "", func(s map[string]any) {
			nodoIn3(s, "k-0009")["identita"].(map[string]any)["emittente"] = "partner:Beta"
		}},
		{"emittente senza prefisso", "emittente", "", func(s map[string]any) { nodoIn3(s, "k-0007")["identita"].(map[string]any)["emittente"] = "acme" }},
		{"cliente in maiuscolo", "emittente", "", func(s map[string]any) {
			nodoIn3(s, "k-0007")["identita"].(map[string]any)["emittente"] = strings.ToUpper(id.clienteE())
		}},
		{"rev minuscola", "nodi[1].identita.rev", "", func(s map[string]any) { nodoIn3(s, "k-0007")["identita"].(map[string]any)["rev"] = "a" }},
		{"rev con uno spazio su un riusa", "nodi[0].identita.rev", "", func(s map[string]any) {
			nodoIn3(s, "k-root")["identita"].(map[string]any)["rev"] = "B 1"
		}},
		{"proposta senza codice", "nodi[1].identita.codice", "", func(s map[string]any) {
			delete(nodoIn3(s, "k-0007")["identita"].(map[string]any), "codice")
		}},
		{"proposta su un nodo da riusare", "nodi[1].identita.modo", "", func(s map[string]any) {
			n := nodoIn3(s, "k-0007")
			n["op"], n["componente_id"] = "riusa", altro
		}},
		{"decisione identita presa da un altro nodo", "nodi[3].identita.decisione", "", func(s map[string]any) {
			nodoIn3(s, "k-0011")["identita"] = map[string]any{"emittente": "partner:beta", "namespace": "beta", "codice": "400123",
				"modo": "decisione", "decisione": "di-0001"}
			var resto []any
			for _, d := range s["decisioni"].([]any) {
				if d.(map[string]any)["id"] != "di-0002" {
					resto = append(resto, d)
				}
			}
			s["decisioni"] = resto
		}},
		{"rev minuscola di una correzione su un nodo da creare", "nodi[2].identita.rev", "", func(s map[string]any) {
			nodoIn3(s, "k-0009")["identita"].(map[string]any)["rev"] = "b"
			decisioneIn3(s, "di-0001")["identita"].(map[string]any)["rev"] = "b"
		}},
		{"codice nuovo con uno spazio", "codice", "", func(s map[string]any) { nodoIn3(s, "k-man")["identita"].(map[string]any)["codice"] = "ACM 9" }},
		{"riusa senza codice", "codice", "", func(s map[string]any) { delete(nodoIn3(s, "k-root")["identita"].(map[string]any), "codice") }},
		{"finito fuori dalla radice", "tipo", "", func(s map[string]any) { nodoIn3(s, "k-0007")["tipo"] = "finito" }},
		{"non_includere sul rif di una casella", "casella.rif", "", func(s map[string]any) {
			decisioneIn3(s, "ni-0001")["casella"].(map[string]any)["rif"] = id.rif("#7")
		}},
		{"due non_includere sullo stesso rif", "casella.rif", "", func(s map[string]any) {
			d := copiaProfonda(t, decisioneIn3(s, "ni-0001"))
			d["id"] = "ni-0002"
			s["decisioni"] = append(s["decisioni"].([]any), d)
		}},
		{"non_includere di un'altra occorrenza", "casella", MotivoRifFuoriImportazione, func(s map[string]any) {
			decisioneIn3(s, "ni-0001")["casella"].(map[string]any)["allegato_id"] = altro
		}},
		{"id di decisione ripetuto", "id", "", func(s map[string]any) { decisioneIn3(s, "dv-0001")["id"] = "sv-0001" }},
		{"numero con esponente", "snapshot", "", func(s map[string]any) { s["archi"].([]any)[0].(map[string]any)["qta"] = jsonNumero("1e0") }},
		{"U+0000 in una descrizione", "snapshot", "", func(s map[string]any) { nodoIn3(s, "k-0007")["descrizione"] = "a\x00b" }},
		{"origine conferma senza conferme", "origine", "", func(s map[string]any) { nodoIn3(s, "k-man")["origine"] = "conferma" }},
		{"preassociazione su un nodo che non c'e'", "chiave_bozza", "", func(s map[string]any) {
			s["documenti_preassociati"].([]any)[0].(map[string]any)["chiave_bozza"] = "k-x"
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			s := copiaProfonda(t, snapshotProva3(id, imp))
			c.cambia(s)
			_, err := LeggiSnapshot3(snapshotJSON(t, s))
			var e *Errore
			if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido {
				t.Fatalf("atteso CORPO_NON_VALIDO, %v", err)
			}
			if !strings.Contains(e.Campo, c.campo) {
				t.Errorf("campo %q, atteso %q (%v)", e.Campo, c.campo, err)
			}
			if c.motivo != "" && e.Motivo != c.motivo {
				t.Errorf("motivo %q, atteso %q", e.Motivo, c.motivo)
			}
		})
	}
	// una bozza tutta a mano non ha ne' sorgente ne' registro
	aMano := map[string]any{"formato": 3, "prodotto_id": id.prodotto.String(), "nodi": []any{
		map[string]any{"chiave_bozza": "r", "componente_id": id.prodotto.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
			"identita": map[string]any{"emittente": id.clienteE(), "emittente_presunto": true, "codice": "ACM-1000", "modo": "registrato"}}},
		"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}}
	if _, err := LeggiSnapshot3(snapshotJSON(t, aMano)); err != nil {
		t.Fatalf("una bozza tutta a mano: %v", err)
	}
}

type jsonNumero string

func (n jsonNumero) MarshalJSON() ([]byte, error) { return []byte(n), nil }

// TestV1LaRevDaDoveViene (proposta di emendamento P-18; EM §3.4.2, §3.6.1): la rev di un nodo copiata da una fonte che
// ammette le minuscole ASCII resta byte per byte, con la sola forma di RevAmmissibile (la working di un riusa, anche
// legacy, la bozza formato 2 di un nodo formato_2, il codice manuale adottato con riferimento manuale_legacy); una rev
// che introducono l'ingegnere o il registro resta senza minuscole (TestV1LeRegoleDiFormaDelFormato3).
func TestV1LaRevDaDoveViene(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	for nome, cambia := range map[string]func(s map[string]any){
		"rev minuscola della working su un riusa": func(s map[string]any) {
			nodoIn3(s, "k-root")["identita"].(map[string]any)["rev"] = "b"
		},
		"rev minuscola del codice manuale adottato": func(s map[string]any) {
			nodoIn3(s, "k-0011")["identita"].(map[string]any)["rev"] = "a"
			decisioneIn3(s, "di-0002")["identita"].(map[string]any)["rev"] = "a"
		},
		"rev minuscola di un nodo formato_2": func(s map[string]any) {
			n := nodoIn3(s, "k-man")["identita"].(map[string]any)
			n["modo"], n["rev"] = "formato_2", "a"
		},
		"rev minuscola della working fissata con una correzione dell'emittente (riusa)": func(s map[string]any) {
			n := nodoIn3(s, "k-root")
			n["identita"] = map[string]any{"emittente": "partner:beta", "codice": "ACM-1000", "rev": "b", "modo": "decisione", "decisione": "dc-0001"}
			s["decisioni"] = append(s["decisioni"].([]any), map[string]any{"id": "dc-0001", "tipo": "identita", "chiave_bozza": "k-root",
				"modo": "correzione", "importazione": imp, "identita": map[string]any{"emittente": "partner:beta", "codice": "ACM-1000", "rev": "b"},
				"motivo": "il componente e' del partner"})
		},
	} {
		t.Run(nome, func(t *testing.T) {
			s := copiaProfonda(t, snapshotProva3(id, imp))
			cambia(s)
			letto, err := LeggiSnapshot3(snapshotJSON(t, s))
			if err != nil {
				t.Fatalf("rifiutato: %v", err)
			}
			can, err := letto.Canonico()
			if err != nil {
				t.Fatalf("lo scrittore lo rifiuta: %v", err)
			}
			if !bytes.Contains(can, []byte(`"rev":"`)) {
				t.Fatal("la rev non si scrive")
			}
		})
	}
}

// TestV1LoScrittoreChiudeComeIlLettore (EM §3.10: «il lettore del formato 3 normalizza esattamente come lo scrittore»):
// per ogni snapshot che lo scrittore (Canonico) accetta, RileggiVersione con i suoi byte e la sua impronta riesce e
// restituisce gli stessi byte; uno snapshot costruito come struct con testi NFD, elenchi in disordine o vuoti da'
// gli stessi byte di quello letto; uno fuori forma lo scrittore lo rifiuta con CORPO_NON_VALIDO, mai uno snapshot
// salvato che la rilettura non ritrova.
func TestV1LoScrittoreChiudeComeIlLettore(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	fresco := func() Snapshot3 { return leggiProva3(t, snapshotProva3(id, imp)) }
	base, err := fresco().Canonico()
	if err != nil {
		t.Fatal(err)
	}
	indiceNodo := func(s *Snapshot3, k string) int {
		for i := range s.Nodi {
			if s.Nodi[i].ChiaveBozza == k {
				return i
			}
		}
		t.Fatalf("nodo %s", k)
		return -1
	}
	indiceDecisione := func(s *Snapshot3, k string) int {
		for i := range s.Decisioni {
			if s.Decisioni[i].ID == k {
				return i
			}
		}
		t.Fatalf("decisione %s", k)
		return -1
	}
	uguali := map[string]func(s *Snapshot3){
		"riletto":         func(s *Snapshot3) {},
		"descrizione NFD": func(s *Snapshot3) { s.Nodi[indiceNodo(s, "k-0007")].Descrizione = descrizioneTelaio },
		"posizione NFD":   func(s *Snapshot3) { s.Archi[len(s.Archi)-1].Posizione = "Pos." + eAccento },
		"motivo NFD": func(s *Snapshot3) {
			s.Decisioni[indiceDecisione(s, "di-0001")].Motivo = "il disegno del fornitore conferma la lettura " + eAccento
		},
		"elenchi al rovescio": func(s *Snapshot3) {
			for i, j := 0, len(s.Nodi)-1; i < j; i, j = i+1, j-1 {
				s.Nodi[i], s.Nodi[j] = s.Nodi[j], s.Nodi[i]
			}
			for i, j := 0, len(s.Decisioni)-1; i < j; i, j = i+1, j-1 {
				s.Decisioni[i], s.Decisioni[j] = s.Decisioni[j], s.Decisioni[i]
			}
		},
		"copia della lettura con i vuoti": func(s *Snapshot3) {
			d := &s.Decisioni[indiceDecisione(s, "di-0001")]
			l := *d.Lettura
			l.Qualificatori, l.Revisione = json.RawMessage(`{"affissi":[],"decorazioni":[]}`), json.RawMessage(`null`)
			d.Lettura = &l
		},
	}
	for nome, cambia := range uguali {
		t.Run(nome, func(t *testing.T) {
			s := fresco()
			cambia(&s)
			b, err := s.Canonico()
			if err != nil {
				t.Fatalf("lo scrittore rifiuta: %v", err)
			}
			if !bytes.Equal(b, base) {
				t.Fatalf("lo stesso snapshot da' altri byte:\n%s\n%s", b, base)
			}
			imp3, _ := s.Impronta()
			letto, err := RileggiVersione(FormatoSnapshot3, comeJsonb(t, b), imp3)
			if err != nil {
				t.Fatalf("la rilettura non ritrova lo snapshot dello scrittore: %v", err)
			}
			if !bytes.Equal(letto.Canonico, b) {
				t.Fatal("la rilettura cambia i byte")
			}
		})
	}
	diversi := map[string]func(s *Snapshot3){
		"un elenco di testa nil": func(s *Snapshot3) { s.DocumentiPreassociati = nil },
		"rev minuscola su un riusa": func(s *Snapshot3) {
			s.Nodi[indiceNodo(s, "k-root")].Identita.Rev = "b"
		},
	}
	for nome, cambia := range diversi {
		t.Run(nome, func(t *testing.T) {
			s := fresco()
			cambia(&s)
			b, err := s.Canonico()
			if err != nil {
				t.Fatalf("lo scrittore rifiuta: %v", err)
			}
			imp3, _ := s.Impronta()
			letto, err := RileggiVersione(FormatoSnapshot3, comeJsonb(t, b), imp3)
			if err != nil || !bytes.Equal(letto.Canonico, b) {
				t.Fatalf("la rilettura non ritrova lo snapshot dello scrittore: %v", err)
			}
		})
	}
	rifiutati := map[string]func(s *Snapshot3){
		"rev minuscola su un nodo da creare": func(s *Snapshot3) { s.Nodi[indiceNodo(s, "k-0007")].Identita.Rev = "a" },
		"U+0000 in un namespace":             func(s *Snapshot3) { s.Nodi[indiceNodo(s, "k-root")].Identita.Namespace = "acme\x00" },
		"decisione con un campo di un altro tipo": func(s *Snapshot3) {
			s.Decisioni[indiceDecisione(s, "dv-0001")].CodiceDopo = "ACM-2"
		},
		"non_includere senza casella": func(s *Snapshot3) { s.Decisioni[indiceDecisione(s, "ni-0001")].Casella = nil },
		"descrizione oltre il limite": func(s *Snapshot3) {
			s.Nodi[indiceNodo(s, "k-0007")].Descrizione = strings.Repeat("d", MaxDescrizione+1)
		},
		"formato 2":             func(s *Snapshot3) { s.Formato = FormatoSnapshot },
		"proposta senza codice": func(s *Snapshot3) { s.Nodi[indiceNodo(s, "k-0007")].Identita.Codice = "" },
	}
	for nome, cambia := range rifiutati {
		t.Run(nome, func(t *testing.T) {
			s := fresco()
			cambia(&s)
			_, err := s.Canonico()
			var e *Errore
			if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido {
				t.Fatalf("atteso CORPO_NON_VALIDO dallo scrittore, %v", err)
			}
			if _, err := s.Impronta(); err == nil {
				t.Fatal("l'impronta di uno snapshot fuori forma si calcola")
			}
		})
	}
}

// TestV1LeRegoleDelCodiceUgualiAClassificazione: la foglia del registro ripete le regole del codice e della rev di
// classificazione (non la importa): le due devono dire la stessa cosa.
func TestV1LeRegoleDelCodiceUgualiAClassificazione(t *testing.T) {
	casi := []string{"", "ACM-1000", "acm-1000", "ACM 1000", "ACM\u00a01000", "ACM\u202f1000", "ACM\u200b1000", "ACM\t1", "A\x00B",
		strings.Repeat("A", 40), strings.Repeat("A", 41), strings.Repeat("é", 20), strings.Repeat("é", 21), "B", "b", "AB 1",
		strings.Repeat("B", 10), strings.Repeat("B", 11), "Ä", "ACM-9" + eAccento}
	for _, c := range casi {
		if importazione.CodiceAmmissibile(c) != classificazione.CodiceAmmissibile(c) {
			t.Errorf("CodiceAmmissibile(%q): registro %v, classificazione %v", c, importazione.CodiceAmmissibile(c), classificazione.CodiceAmmissibile(c))
		}
		if importazione.RevAmmissibile(c) != classificazione.RevAmmissibile(c) {
			t.Errorf("RevAmmissibile(%q): registro %v, classificazione %v", c, importazione.RevAmmissibile(c), classificazione.RevAmmissibile(c))
		}
	}
	if importazione.MaxCodiceProposto != classificazione.MaxCodice || importazione.MaxRevProposta != classificazione.MaxRev {
		t.Error("i tetti del codice e della rev del registro non sono quelli di classificazione")
	}
}

// TestV1N01NelloSnapshot (T-02, la parte L1): l'identita' e le copie dal registro restano byte per byte, i testi
// umani passano in NFC, dopo la lettura, la forma canonica e la rilettura, e nell'impronta.
func TestV1N01NelloSnapshot(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	s := leggiProva3(t, snapshotProva3(id, imp))
	can, _ := s.Canonico()
	s2, err := LeggiSnapshot3(can)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []Snapshot3{s, s2} {
		man := nodoDi3(x, "k-man")
		if man.Identita.Codice != codiceAccentato || norm.NFC.IsNormalString(man.Identita.Codice) {
			t.Fatalf("il codice ha perso la «e» con l'accento combinante: %q", man.Identita.Codice)
		}
		if tel := nodoDi3(x, "k-0007"); tel.Descrizione != norm.NFC.String(descrizioneTelaio) || tel.Descrizione == descrizioneTelaio {
			t.Fatalf("la descrizione non e' in NFC: %q", tel.Descrizione)
		}
		d := decisioneDi3(x, "di-0001")
		if d.Lettura == nil || d.Lettura.Letterale != letteraleBeta || d.Lettura.Base.Originale != letteraleBeta {
			t.Fatalf("la copia della lettura ha cambiato il letterale: %+v", d.Lettura)
		}
		if !norm.NFC.IsNormalString(d.Motivo) || !strings.HasSuffix(d.Motivo, "é") {
			t.Fatalf("il motivo non e' in NFC: %q", d.Motivo)
		}
		if a := arcoDi3(x, "k-root", "k-man"); a.Posizione != "Pos.é" {
			t.Fatalf("la posizione non e' in NFC: %q", a.Posizione)
		}
	}
	if !bytes.Contains(can, []byte(codiceAccentato)) || !bytes.Contains(can, []byte(letteraleBeta)) {
		t.Fatal("i byte canonici hanno perso l'accento combinante del codice o del letterale")
	}
	if bytes.Contains(can, []byte(descrizioneTelaio)) {
		t.Fatal("i byte canonici portano la descrizione non normalizzata")
	}
	// la stessa bozza con il codice gia' in NFC e' un'altra bozza: un'altra impronta
	nfcDelCodice := copiaProfonda(t, snapshotProva3(id, imp))
	nodoIn3(nfcDelCodice, "k-man")["identita"].(map[string]any)["codice"] = norm.NFC.String(codiceAccentato)
	sn := leggiProva3(t, nfcDelCodice)
	if i1, _ := s.Impronta(); func() string { i2, _ := sn.Impronta(); return i2 }() == i1 {
		t.Fatal("il codice in NFC e quello con l'accento combinante danno la stessa impronta")
	}
	// la descrizione composta o scomposta da' la stessa bozza
	composta := copiaProfonda(t, snapshotProva3(id, imp))
	nodoIn3(composta, "k-0007")["descrizione"] = norm.NFC.String(descrizioneTelaio)
	sc := leggiProva3(t, composta)
	if i1, _ := s.Impronta(); func() string { i2, _ := sc.Impronta(); return i2 }() != i1 {
		t.Fatal("la descrizione composta e quella scomposta danno impronte diverse")
	}
}

func nodoDi3(s Snapshot3, k string) Nodo3 {
	for _, n := range s.Nodi {
		if n.ChiaveBozza == k {
			return n
		}
	}
	panic("nodo " + k)
}

func arcoDi3(s Snapshot3, padre, figlio string) Arco3 {
	for _, a := range s.Archi {
		if a.Padre == padre && a.Figlio == figlio {
			return a
		}
	}
	panic("arco " + padre + " " + figlio)
}

func decisioneDi3(s Snapshot3, id string) Decisione3 {
	for _, d := range s.Decisioni {
		if d.ID == id {
			return d
		}
	}
	panic("decisione " + id)
}

func TestV1IRegistriCitati(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	s := copiaProfonda(t, snapshotProva3(id, imp))
	vecchio := strings.Repeat("9", 64)
	decisioneIn3(s, "di-0002")["importazione"] = vecchio
	got := leggiProva3(t, s).RegistriCitati()
	want := []string{imp, vecchio}
	if want[1] < want[0] {
		want[0], want[1] = want[1], want[0]
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("registri citati %v, attesi %v", got, want)
	}
}

// TestV1T14LaRegolaDellEmittenteL1 (T-14, la parte L1): gli stessi casi danno lo stesso esito al validatore delle
// grammatiche (l'id dell'emittente di una grammatica partner, il namespace di una famiglia) e alle funzioni Go. La
// sola differenza dichiarata e' il tetto di sicurezza della 0023 (MaxIDPartner, MaxNamespace), che la grammatica non
// ha (EM §3.4.2; PM23 §4.6): oltre il tetto il Go rifiuta e la grammatica accetta. Il CHECK e' la parte L4
// (platform/migrazioni, TestV1T14LaRegolaDellEmittenteDelDatabase).
func TestV1T14LaRegolaDellEmittenteL1(t *testing.T) {
	lim := grammatica.TettiLimiti()
	lim.Versione = "limiti-prova-t14"
	if d := lim.Valida(); len(d) > 0 {
		t.Fatalf("i limiti della prova non sono validi: %v", d)
	}
	for _, c := range CasiT14() {
		t.Run(c.Nome, func(t *testing.T) {
			if c.Partner != "" {
				g := grammatica.Grammatica{Emittente: &grammatica.EmittenteGrammatica{ID: c.Partner, Nome: "Beta"}}
				grammaticaOk := true
				for _, d := range g.Valida(lim) {
					if d.Percorso == "emittente.id" {
						grammaticaOk = false
					}
				}
				goOk := EmittenteAmmissibile("partner:" + c.Partner)
				if goOk != c.Atteso {
					t.Errorf("EmittenteAmmissibile(partner:%s) = %v, atteso %v", c.Partner, goOk, c.Atteso)
				}
				if attesoGrammatica := c.Atteso || c.OltreIlTetto; grammaticaOk != attesoGrammatica {
					t.Errorf("grammatica: %v, attesa %v", grammaticaOk, attesoGrammatica)
				}
			}
			if c.Namespace != "" {
				g := grammatica.Grammatica{Famiglie: []grammatica.FamigliaCodice{{ID: "f1", Namespace: c.Namespace}}}
				grammaticaOk := true
				for _, d := range g.Valida(lim) {
					if strings.HasPrefix(d.Percorso, "famiglie_codice") && strings.HasSuffix(d.Percorso, ".namespace") {
						grammaticaOk = false
					}
				}
				if NamespaceAmmissibile(c.Namespace) != c.Atteso {
					t.Errorf("NamespaceAmmissibile(%q) = %v, atteso %v", c.Namespace, !c.Atteso, c.Atteso)
				}
				if attesoGrammatica := c.Atteso || c.OltreIlTetto; grammaticaOk != attesoGrammatica {
					t.Errorf("grammatica sul namespace: %v, attesa %v", grammaticaOk, attesoGrammatica)
				}
			}
		})
	}
	if !EmittenteAmmissibile(EmittenteCliente(idDi("cliente"))) || EmittenteAmmissibile("cliente:"+strings.ToUpper(idDi("cliente").String())) ||
		EmittenteAmmissibile("cliente:ACME") || EmittenteAmmissibile("partner:") || EmittenteAmmissibile("fornitore:beta") {
		t.Fatal("la forma cliente:<uuid> o il prefisso non tengono")
	}
}
