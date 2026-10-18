package distintabozza

// Gli aiuti delle prove del formato 3 (V1, passo 2): un registro e uno snapshot sintetici coerenti fra loro, con tutti i
// campi che la tabella di conservazione (EM §4) chiede di ritrovare. ACME e BETA sono inventati; gli UUID sono di prova
// o, nelle L4, quelli della scena del database.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// eAccento: «e» + accento combinante (U+0301), la stringa della prova N-01.
const eAccento = "é"

// idProva3: gli identificativi di una prova del formato 3.
type idProva3 struct {
	thread, prodotto, cliente uuid.UUID
	allegato, pdf             uuid.UUID
	sha, shaPdf               string
	utente                    uuid.UUID
}

func idSintetici3() idProva3 {
	return idProva3{thread: idDi("rfq"), prodotto: idDi("prodotto"), cliente: idDi("cliente"), allegato: idDi("allegato"),
		pdf: idDi("allegato2"), sha: shaProva('b'), shaPdf: shaProva('c'), utente: idDi("utente")}
}

func (id idProva3) rif(chiave string) string { return "nodo:" + id.sha + ":" + chiave }
func (id idProva3) voce(chiave string) string {
	return "n:" + id.allegato.String() + ":" + id.rif(chiave)
}
func (id idProva3) clienteE() string { return EmittenteCliente(id.cliente) }

const (
	gruppoProva3      = "bom"
	idLetturaBeta     = "l:u:step:#9:id:beta-num/sei:0-8"
	letteraleBeta     = "400123-" + eAccento
	codiceManuale     = "ACM-1900"
	codiceAccentato   = "ACM-9" + eAccento
	descrizioneTelaio = "T" + eAccento + "laio"
)

// letturaProva3: una lettura del registro, con il letterale e il testo dell'unita' byte per byte.
func letturaProva3(allegato uuid.UUID, id, emittente, namespace, letterale string, qualificatori, revisione any) map[string]any {
	return map[string]any{
		"chiave_lettura": allegato.String() + "/" + id, "unita_id": "u:" + id, "selettore": "nodo_step.id", "testo_unita": letterale,
		"intervallo": map[string]any{"inizio": 0, "fine": len(letterale)}, "letterale": letterale, "emittente": emittente,
		"hash_grammatica": strings.Repeat("7", 64), "namespace": namespace, "famiglia": "num", "forma": "sei",
		"base":          map[string]any{"originale": letterale, "normalizzata": letterale, "segmenti": []any{}, "mancanti": []any{}, "completa": true},
		"qualificatori": qualificatori, "revisione": revisione, "stato_forma": "letta", "funzione": "identita", "ruolo": "",
		"qualita": "piena", "motivi": []any{},
	}
}

func rawDi(t testing.TB, v any) json.RawMessage {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// registroProva3: il registro del gruppo bom dello STEP della scena, con il contesto attivo (BETA partner attivo).
func registroProva3(t testing.TB, id idProva3) importazione.Contenuto {
	t.Helper()
	vero, falso := true, false
	cliente := id.clienteE()
	voce := func(chiave string) importazione.Voce {
		return importazione.Voce{Voce: id.voce(chiave),
			Origine:  importazione.OrigineVoce{AllegatoID: id.allegato.String(), Sha256: id.sha, Chiave: chiave, Rif: id.rif(chiave)},
			EntitaID: rawDi(t, "e"+chiave), Foglia: rawDi(t, false), RadiceDelFile: rawDi(t, chiave == "#1"), Leggibile: rawDi(t, true),
			StatoMotore: importazione.StatoMotore{Raggiunto: &vero, Tolto: &falso, Deciso: &falso},
			Grezzi:      rawDi(t, map[string]any{"id": chiave, "nome": "pezzo " + chiave, "formazioni": []any{}}),
			Diagnostiche: rawDi(t, []any{map[string]any{"codice": "valutazione.prova", "gravita": "avviso", "natura": "dato",
				"messaggio": "diagnostica della voce " + chiave, "rif": []any{id.rif(chiave)}}}),
		}
	}
	revB := map[string]any{"regola": "rev_lettera", "originale": "B", "normalizzata": "B", "stato": "letta", "sorgente": "nome",
		"segmenti": []any{}, "richiede_verifica": false, "intervallo": map[string]any{"inizio": 9, "fine": 10}}
	radice := voce("#1")
	radice.Padri = rawDi(t, []any{})
	radice.IdentitaProposta = rawDi(t, letturaProva3(id.allegato, "l:u:step:#1:id:acme-num/sei:0-10", cliente, "acme", "ACM-1000 B", map[string]any{}, revB))
	radice.CodiceProposto, radice.RevProposta = "ACM-1000", "B"
	radice.CodiceComposto = rawDi(t, map[string]any{"testo": "ACM-1000 B", "motivo": "cartiglio"})
	radice.DecisioneEsistente = rawDi(t, map[string]any{"componente_id": id.prodotto.String(), "codice": "ACM-1000", "rev": "B"})
	radice.TipoProposto = rawDi(t, map[string]any{"valore": "finito", "origine": "confermato"})
	radice.Riferimenti = rawDi(t, []any{map[string]any{"tipo": "documentale", "valore": "ACM-1000", "esito": "concorde",
		"correzione":  map[string]any{"codice": "ACM-1000", "revisione_inferiore": false},
		"discordanze": []any{map[string]any{"decisione": "ACM-1000", "evidenza": "ACM-1000", "revisione_inferiore": true}}, "fonte": "pdf"}})
	telaio := voce("#7")
	telaio.Padri = rawDi(t, []any{id.rif("#1")})
	qual := map[string]any{"marcatore": "-", "affissi": []any{map[string]any{"regola": "suffisso", "originale": "-S", "posizione": "dopo",
		"intervallo": map[string]any{"inizio": 8, "fine": 10}, "attribuito": true, "fase": "saldato", "destinazione": "codice"}},
		"decorazioni": []any{}, "ripetizioni": []any{}}
	revA := map[string]any{"regola": "rev_lettera", "originale": "a", "normalizzata": "a", "stato": "letta", "sorgente": "nome",
		"segmenti": []any{}, "richiede_verifica": false, "intervallo": map[string]any{"inizio": 11, "fine": 12}}
	telaio.IdentitaProposta = rawDi(t, letturaProva3(id.allegato, "l:u:step:#7:id:acme-num/sei:0-8", cliente, "acme", "ACM-1100-S a", qual, revA))
	telaio.CodiceProposto, telaio.RevProposta = "ACM-1100", "A"
	telaio.TipoProposto = rawDi(t, map[string]any{"valore": "sottoassieme", "origine": "regola", "classe": "saldato"})
	telaio.Attributi = rawDi(t, []any{map[string]any{"id": "at-1", "tipo": "formazione", "grezzo": "S", "stato": "letto", "unita_id": "u:#7",
		"entita_id": "e#7", "letture_compatibili": []any{id.allegato.String() + "/l:u:step:#7:id:acme-num/sei:0-8"}, "evidenze": []any{}}})
	telaio.AssociazioniProposte = rawDi(t, []any{map[string]any{"allegato_id": id.pdf.String(), "motivo": "nome", "livello": "alto"}})
	telaio.FabbisogniPrevisti = rawDi(t, []any{map[string]any{"tipo": "disegno_2d", "bloccante": true}})
	telaio.RigaLegacy = rawDi(t, map[string]any{"id": "00000000-0000-4000-8000-0000000c0007", "stato": "aperta", "decisa_da_persona": false})
	pezzo := voce("#9")
	pezzo.Padri = rawDi(t, []any{id.rif("#7")})
	pezzo.Foglia = rawDi(t, true)
	pezzo.MotivoProposto = rawDi(t, "letture_discordanti")
	pezzo.MotivoLettura = rawDi(t, "letture_discordanti")
	pezzo.Alternative = rawDi(t, []any{letturaProva3(id.allegato, idLetturaBeta, "partner:beta", "beta", letteraleBeta, map[string]any{}, nil)})
	manuale := voce("#11")
	manuale.Padri = rawDi(t, []any{id.rif("#7")})
	manuale.Foglia = rawDi(t, true)
	manuale.CodiceManualeLegacy = rawDi(t, map[string]any{"codice": codiceManuale, "rev": "A", "riga_legacy": "00000000-0000-4000-8000-0000000c0011"})
	cordone := voce("#31")
	cordone.Padri = rawDi(t, []any{id.rif("#7")})
	cordone.Foglia, cordone.Leggibile = rawDi(t, true), rawDi(t, false)
	cordone.Segnali = rawDi(t, []any{map[string]any{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1",
		"riscontri": []any{map[string]any{"campo": "nome", "unita_id": "u:#31", "parole": []any{"cordone"},
			"intervalli": []any{map[string]any{"inizio": 6, "fine": 13}}}}, "spiegazione": "possibile elemento CAD"}})
	q1, q2, q3 := 1, 2, 3
	partner := []importazione.PartnerAttivo{{Emittente: "partner:beta", Hash: strings.Repeat("8", 64)}}
	citati := []string{id.allegato.String(), id.pdf.String()}
	if citati[1] < citati[0] {
		citati[0], citati[1] = citati[1], citati[0]
	}
	return importazione.Contenuto{
		FormatoRegistro: importazione.FormatoRegistro, VersioneImportazione: importazione.VersioneImportazione1,
		ThreadID: id.thread.String(), ProdottoID: id.prodotto.String(),
		Gruppo: importazione.Gruppo{Chiave: gruppoProva3, Tipo: rawDi(t, "bom"), AllegatoID: id.allegato.String(), Sha256: id.sha,
			Radice: id.rif("#1"), NomeFile: rawDi(t, "ACM-1000.stp"), StatoStruttura: rawDi(t, "bom_di_lavoro_proposta"),
			RadiceDelFile: rawDi(t, true), Compatibilita: rawDi(t, "uguale"), GrafoCompleto: rawDi(t, false), MotivoGrafo: rawDi(t, "riferimenti_mancanti")},
		Regole: importazione.Regole{
			Versioni: importazione.Versioni{Algoritmo: rawDi(t, "a1-1"), Router: rawDi(t, "r-2"), Risultato: rawDi(t, "es-3"), Ancoraggio: "ancoraggio-3",
				Valutazione: rawDi(t, "v-4"), Composizione: rawDi(t, "gp-1"), Importazione: importazione.VersioneImportazione1,
				Canonico: jsoncanonico.Versione, Limiti: rawDi(t, "lim-1")},
			GrammaticaCliente: &importazione.GrammaticaCliente{ClienteID: id.cliente.String(), Hash: strings.Repeat("7", 64)},
			Partner:           &partner,
			Contesto:          &importazione.ContestoRegole{ID: rawDi(t, "ctx-acme-beta"), Impronta: strings.Repeat("5", 64)}},
		Calcolo: rawDi(t, map[string]any{"voci": map[string]any{id.voce("#7"): map[string]any{"calcolato_il": "2026-10-17T10:00:00Z",
			"digest_payload": "d1", "sottoversione_step": 3}}, "impronta_indice": "i-1", "bundle_id": "b-1"}),
		Contesto: rawDi(t, map[string]any{"id": "ctx-acme-beta", "partner": []any{"beta"}, "perimetro": []any{id.pdf.String()}}),
		Proposta: importazione.Proposta{
			Fonte: importazione.FonteProposta{AllegatoID: id.allegato.String(), Sha256: id.sha, NomeRicevuto: rawDi(t, "ACM-1000.stp"),
				PercorsoRicevuto: rawDi(t, "STEP/ACM-1000.stp")},
			Nodi: []importazione.Voce{radice, telaio, pezzo, manuale, cordone},
			Archi: []importazione.ArcoRegistro{
				{AllegatoID: id.allegato.String(), Padre: id.rif("#1"), Figlio: id.rif("#7"), Quantita: &q1, Occorrenze: rawDi(t, []any{"nauo-1"})},
				{AllegatoID: id.allegato.String(), Padre: id.rif("#7"), Figlio: id.rif("#9"), Quantita: &q2},
				{AllegatoID: id.allegato.String(), Padre: id.rif("#7"), Figlio: id.rif("#11"), Quantita: &q3},
				{AllegatoID: id.allegato.String(), Padre: id.rif("#7"), Figlio: id.rif("#31"), Quantita: &q1}},
			File: []importazione.FileRegistro{{Voce: rawDi(t, "f:"+id.pdf.String()), AllegatoID: id.pdf.String(), Sha256: id.shaPdf,
				NomeRicevuto: rawDi(t, "ACM-1100.pdf"), Origine: rawDi(t, "allegato"),
				Interpretazione: rawDi(t, map[string]any{"stato": "letta", "contesto_id": "ctx-acme-beta", "impronta_contesto": strings.Repeat("5", 64),
					"autore_documento": "partner:beta"}),
				Letture:   rawDi(t, []any{letturaProva3(id.pdf, "l:u:pdf:1:id:beta-num/sei:0-6", "partner:beta", "beta", "400123", map[string]any{}, nil)}),
				Candidati: rawDi(t, []any{id.voce("#7")})}},
			Relazioni: []importazione.Relazione{
				{Voce: rawDi(t, "r:1"), IDMotore: rawDi(t, "rel-1"), Tipo: rawDi(t, "documentato_da"), Stato: rawDi(t, "proposta"),
					Modello: rawDi(t, "leg-1"), EntitaSorgente: rawDi(t, "e#7"), Riconoscimento: rawDi(t, "legame"),
					Sorgente:         importazione.SorgenteRelazione{Tipo: "nodo", Voce: rawDi(t, id.voce("#7"))},
					LetturaSorgente:  rawDi(t, id.allegato.String()+"/l:u:step:#7:id:acme-num/sei:0-8"),
					LettureBersaglio: rawDi(t, []any{id.pdf.String() + "/l:u:pdf:1:id:beta-num/sei:0-6"}), Evidenze: rawDi(t, []any{"u:#7"})},
				{Voce: rawDi(t, "r:2"), IDMotore: rawDi(t, "rel-2"), Tipo: rawDi(t, "varianti_nel_documento"), Stato: rawDi(t, "proposta"),
					Sorgente: importazione.SorgenteRelazione{Tipo: "file", Voce: rawDi(t, "f:"+id.pdf.String())}}},
			FuoriPerDecisione: []importazione.FuoriPerDecisione{{Voce: id.voce("#12"), Segno: importazione.SegnoTolto,
				Chi: rawDi(t, id.utente.String()), Quando: rawDi(t, "2026-10-16T09:00:00Z"), Motivo: rawDi(t, "duplicato nel file")}},
			Diagnostiche: rawDi(t, []any{map[string]any{"codice": "importazione.prova", "gravita": "avviso"}}),
		},
		AllegatiCitati: citati,
	}
}

// componiProva3: il registro composto.
func componiProva3(t testing.TB, id idProva3) importazione.Registro {
	t.Helper()
	r, err := importazione.Componi(registroProva3(t, id))
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

// letturaCompattaBeta: la copia compatta dell'alternativa di BETA, come la porta la decisione di-0001.
func letturaCompattaBeta() map[string]any {
	return map[string]any{"emittente": "partner:beta", "hash_grammatica": strings.Repeat("7", 64), "namespace": "beta",
		"letterale": letteraleBeta, "base": map[string]any{"originale": letteraleBeta, "normalizzata": letteraleBeta},
		"qualificatori": map[string]any{}, "revisione": nil}
}

// snapshotProva3: lo snapshot formato 3 della bozza del prodotto sul registro di impronta imp: la radice registrata
// (presunta), il telaio con l'identita' proposta, il pezzo deciso con un'alternativa di BETA, il pezzo con il codice
// manuale adottato, un nodo scritto a mano con un codice con l'accento combinante; il cordone non incluso.
func snapshotProva3(id idProva3, imp string) map[string]any {
	cliente := id.clienteE()
	casella := func(chiave string) []any {
		return []any{map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3, "rif": id.rif(chiave)}}
	}
	step := map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3}
	return map[string]any{
		"formato": 3, "prodotto_id": id.prodotto.String(),
		"sorgente":          map[string]any{"allegato_id": id.allegato.String(), "sha256": id.sha, "gruppo": gruppoProva3, "importazione": imp},
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 12},
		"firma_base":        firmaProva,
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-root", "componente_id": id.prodotto.String(), "op": "riusa", "tipo": "finito",
				"descrizione": "Gruppo base", "origine": "registrato", "caselle": casella("#1"),
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-1000", "rev": "B", "modo": "registrato"}},
			map[string]any{"chiave_bozza": "k-0007", "op": "crea", "tipo": "sottoassieme", "descrizione": descrizioneTelaio, "origine": "step",
				"caselle":  casella("#7"),
				"identita": map[string]any{"emittente": cliente, "namespace": "acme", "codice": "ACM-1100", "rev": "A", "modo": "proposta"}},
			map[string]any{"chiave_bozza": "k-0009", "op": "crea", "tipo": "sciolto", "origine": "step", "caselle": casella("#9"),
				"identita": map[string]any{"emittente": "partner:beta", "namespace": "beta", "codice": "400123", "modo": "decisione", "decisione": "di-0001"}},
			map[string]any{"chiave_bozza": "k-0011", "op": "crea", "tipo": "sciolto", "origine": "step", "caselle": casella("#11"),
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": codiceManuale, "rev": "A",
					"modo": "decisione", "decisione": "di-0002"}},
			map[string]any{"chiave_bozza": "k-man", "op": "crea", "tipo": "commerciale", "origine": "utente",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": codiceAccentato, "modo": "manuale"}},
		},
		"archi": []any{
			map[string]any{"padre": "k-root", "figlio": "k-0007", "qta": 1, "origine": "step", "step": step},
			map[string]any{"padre": "k-0007", "figlio": "k-0009", "qta": 2, "origine": "step", "step": step},
			map[string]any{"padre": "k-0007", "figlio": "k-0011", "qta": 3, "origine": "step", "step": step},
			map[string]any{"padre": "k-root", "figlio": "k-man", "qta": 4, "origine": "utente", "posizione": "Pos." + eAccento},
		},
		"decisioni": []any{
			map[string]any{"id": "di-0001", "tipo": "identita", "chiave_bozza": "k-0009", "modo": "alternativa", "importazione": imp,
				"chiave_lettura": id.allegato.String() + "/" + idLetturaBeta, "lettura": letturaCompattaBeta(),
				"identita": map[string]any{"emittente": "partner:beta", "namespace": "beta", "codice": "400123"},
				"motivo":   "il disegno del fornitore conferma la lettura " + eAccento},
			map[string]any{"id": "di-0002", "tipo": "identita", "chiave_bozza": "k-0011", "modo": "alternativa", "importazione": imp,
				"riferimento": "manuale_legacy", "identita": map[string]any{"emittente": cliente, "codice": codiceManuale, "rev": "A"},
				"motivo": "il codice corretto a mano e' quello del disegno"},
			map[string]any{"id": "dv-0001", "tipo": "da_valutare", "chiave_bozza": "k-0007"},
			map[string]any{"id": "ni-0001", "tipo": "non_includere",
				"casella":       map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3, "rif": id.rif("#31")},
				"motivo_codice": "elemento_cad", "motivo": "cordone di saldatura: annotazione CAD", "azione_id": "00000000-0000-4000-8000-0000000e0101",
				"importazione": imp, "segnale_visto": map[string]any{"codice": "ancoraggio.possibile_elemento_cad",
					"regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"}},
			map[string]any{"id": "sv-0001", "tipo": "scollega_documento", "proposta_id": "00000000-0000-4000-8000-0000000d0007",
				"componente_id": "00000000-0000-4000-8000-000000002200", "motivo": "il disegno e' di un'altra variante"},
		},
		"documenti_preassociati": []any{map[string]any{"proposta_id": "00000000-0000-4000-8000-0000000d0001", "chiave_bozza": "k-0007"}},
	}
}

// snapshotJSON: lo snapshot come JSON.
func snapshotJSON(t testing.TB, s map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// copiaProfonda: una copia di una mappa JSON da cambiare in una prova.
func copiaProfonda(t testing.TB, s map[string]any) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(snapshotJSON(t, s), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func nodoIn3(s map[string]any, chiave string) map[string]any {
	for _, n := range s["nodi"].([]any) {
		if m := n.(map[string]any); m["chiave_bozza"] == chiave {
			return m
		}
	}
	panic("nodo " + chiave)
}

func decisioneIn3(s map[string]any, id string) map[string]any {
	for _, d := range s["decisioni"].([]any) {
		if m := d.(map[string]any); m["id"] == id {
			return m
		}
	}
	panic("decisione " + id)
}
