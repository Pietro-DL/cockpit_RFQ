package bancoa

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/dataset"
)

// Gli aiuti delle prove delle modalità dsn ed exports (A1c, Q9): una scena ACME scritta in t.TempDir(), fuori dal
// modulo, con il manifest, l'indice delle regole con il puntatore ai casi, la grammatica, il file dei casi, gli attesi
// e gli export, tutti con gli sha256 e i byte calcolati sui byte scritti.
//
// I clienti di questi test sono inventati. Non è pigrizia: gli export veri, gli attesi veri, il file dei casi vero e il
// manifest vero sono dati privati, e questo repository è pubblico. Il cliente è ACME, gli UUID hanno la forma
// 00000000-0000-4000-8000-0000000000nn, i codici sono 912xxxx e 765xxxx, i domini acme.example, e i file sintetici
// hanno nomi con _acme, diversi da quelli del dataset privato (E-13).

// uidACME: un UUID della scena, con la forma delle fixture ACME.
func uidACME(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

// shaACME: uno sha256 inventato della scena.
func shaACME(n int) string { return fmt.Sprintf("%064x", n) }

// Gli ID della scena.
var (
	threadScenario  = uidACME(0x71)
	threadDecisioni = uidACME(0x72)
	msgScenario     = uidACME(0x81)
	msgDecisioni    = uidACME(0x82)
	msgFuori1       = uidACME(0x91)
	msgFuori2       = uidACME(0x92)
	msgNonEsportato = uidACME(0x99)
	allRadice       = uidACME(0x101) // 9123456A_2.pdf, radice dello scenario
	allFiglio       = uidACME(0x102) // lo STEP del figlio
	allFuori        = uidACME(0x103) // fuori dallo scenario
	allArchivio     = uidACME(0x104) // un archivio dei file reali
	allBaseline     = uidACME(0x111) // il 2D deciso della baseline
	allDaRivedere   = uidACME(0x112) // il 2D deciso della C5
	allFuoriRFQ     = uidACME(0x121)
	allNonEsportato = uidACME(0x199)
	compBaseline    = uidACME(0x31)
	compDaRivedere  = uidACME(0x32)
	docBaseline     = uidACME(0x221)
	docDaRivedere   = uidACME(0x222)
	utenteACME      = uidACME(0x51)
	convACME        = uidACME(0xc1)
)

// Gli istanti della scena: ISO, UTC, al millisecondo, come negli export.
const (
	istante0 = "2026-10-01T08:40:25.885Z"
	istante1 = "2026-10-01T09:00:00.000Z"
)

// La terna degli export della scena (dichiarata nel manifest).
const versioneTernaACME = 4

var hashTernaACME = strings.Repeat("c", 64)

// mailInoltrataACME: una richiesta inoltrata senza confine nel corpo (R48 A): la storia non è separabile.
const mailInoltrataACME = "Buongiorno,\r\nvi giro la richiesta ACME26-030.\r\n\r\nCodice\r\nQ.TA\r\nP9123456\r\n5\r\n\r\nGrazie"

// riga: una riga di un export, colonna per colonna.
type riga map[string]any

// righeExportACME: le righe degli export della scena, per sezione.
func righeExportACME() map[string][]riga {
	return map[string][]riga{
		ExportThread: {
			{"thread_id": threadScenario.String(), "cliente_id": clienteACME.String(), "stato": "APERTA", "unito_in": nil,
				"oggetto": "Richiesta ACME26-030", "riferimento_cliente": nil, "creato_da": nil, "creato_il": istante0, "canale": "outlook"},
			{"thread_id": threadDecisioni.String(), "cliente_id": clienteACME.String(), "stato": "APERTA", "unito_in": nil,
				"oggetto": "Disegni ACME", "riferimento_cliente": "ACME26-031", "creato_da": utenteACME.String(), "creato_il": istante0, "canale": "outlook"},
		},
		ExportMessaggi: {
			messaggioACME(msgScenario, &threadScenario, "I: Richiesta ACME26-030", mailInoltrataACME),
			messaggioACME(msgDecisioni, &threadDecisioni, "Disegni", "In allegato i disegni."),
			messaggioACME(msgFuori1, nil, "Domanda", "Per il PN 7654321 vi chiedo una conferma."),
			messaggioACME(msgFuori2, nil, "Altra domanda", "Nessun codice qui."),
		},
		ExportHTML: {
			{"messaggio_id": msgScenario.String(), "corpo_html": "<table><tr><td>P9123456</td><td>5</td></tr></table>"},
		},
		ExportAllegati: {
			allegatoACME(allRadice, msgScenario, 0, "9123456A_2.pdf", "pdf", shaACME(0x101)),
			allegatoACME(allFiglio, msgScenario, 1, "9123457A_1.stp", "stp", shaACME(0x102)),
			allegatoACME(allFuori, msgScenario, 2, "9123458.pdf", "pdf", shaACME(0x103)),
			allegatoACME(allArchivio, msgScenario, 3, "9123460_00.7z", "7z", shaACME(0x104)),
			allegatoACME(allBaseline, msgDecisioni, 0, "9123456A_2.pdf", "pdf", shaACME(0x111)),
			allegatoACME(allDaRivedere, msgDecisioni, 1, "9123459A_3.pdf", "pdf", shaACME(0x112)),
			allegatoACME(allFuoriRFQ, msgFuori1, 0, "7654321.pdf", "pdf", shaACME(0x121)),
			allegatoACME(allNonEsportato, msgNonEsportato, 0, "9123499.pdf", "pdf", shaACME(0x199)),
		},
		ExportFatti: {
			{"sha256": shaACME(0x104), "versione_analizzatore": versioneTernaACME, "hash_configurazione": hashTernaACME,
				"fatti": `{"archivio": {"voci": []}}`, "calcolato_il": istante0},
			{"sha256": shaACME(0x103), "versione_analizzatore": versioneTernaACME - 1, "hash_configurazione": hashTernaACME,
				"fatti": `{"vecchio": true}`, "calcolato_il": istante0},
		},
		ExportProposte: {
			propostaACME(0x341, allRadice, threadScenario, "9123456A", "2", nil, "aperta", nil),
			propostaACME(0x342, allBaseline, threadDecisioni, "9123456A", "2", &compBaseline, "confermata", strPtr(istante1)),
			propostaACME(0x343, allDaRivedere, threadDecisioni, "9123459A", "3", &compDaRivedere, "confermata", strPtr(istante1)),
			propostaACME(0x344, allNonEsportato, threadDecisioni, "9123499", "", nil, "aperta", nil),
		},
		ExportDocumenti: {
			documentoACME(docBaseline, compBaseline, "9123456A", "2", "9123456A_2.pdf", shaACME(0x111), allBaseline),
			documentoACME(docDaRivedere, compDaRivedere, "9123459A", "3", "9123459A_3.pdf", shaACME(0x112), allDaRivedere),
			documentoACME(docDaRivedere, compDaRivedere, "9123459A", "3", "9123459A_3.pdf", shaACME(0x112), allNonEsportato),
		},
		ExportIdentificativi: {
			{"thread_id": threadDecisioni.String(), "codice": "9123456", "origine": "manuale", "confidenza": nil,
				"confermato_da": utenteACME.String(), "creato_il": istante0},
		},
		ExportRelazioni: {
			{"thread_id": threadDecisioni.String(), "padre_id": compBaseline.String(), "figlio_id": compDaRivedere.String(),
				"qta": 2, "posizione": nil, "origine": "manuale", "confermato_da": utenteACME.String(), "creato_il": istante0},
		},
		ExportTriage: {
			{"triage_id": uidACME(0x401).String(), "messaggio_id": msgScenario.String(), "esito": "nuova_rfq", "atto": "richiesta_offerta",
				"legame": nil, "stato": "accettata", "identificativi": "{ACME26-030}", "motivi": "[]", "fonte": "deterministico",
				"creato_il": istante0, "deciso_il": istante1},
			{"triage_id": uidACME(0x402).String(), "messaggio_id": msgScenario.String(), "esito": "nuova_rfq", "atto": nil,
				"legame": nil, "stato": "proposta", "identificativi": "{}", "motivi": "[]", "fonte": "ag" + "ente",
				"creato_il": istante0, "deciso_il": nil},
		},
		ExportClienti: {
			{"cliente_id": clienteACME.String(), "ragione_sociale": "ACME S.p.A.", "cartella_nas": "ACME"},
			{"cliente_id": uidACME(0xac09).String(), "ragione_sociale": "Altro ACME S.r.l.", "cartella_nas": "ALTRO"},
		},
	}
}

func strPtr(s string) *string { return &s }

// writeFile: un file della scena riscritto sul disco, dopo il manifest.
func writeFile(t testing.TB, p, contenuto string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(contenuto), 0o644); err != nil {
		t.Fatal(err)
	}
}

func messaggioACME(id uuid.UUID, thread *uuid.UUID, oggetto, corpo string) riga {
	var th any
	if thread != nil {
		th = thread.String()
	}
	return riga{"messaggio_id": id.String(), "conversazione_id": convACME.String(), "parent_messaggio_id": nil, "thread_id": th,
		"aggancio": "operatore", "agganciato_da": utenteACME.String(), "agganciato_il": istante1, "direzione": "entrata",
		"data_evento": istante0, "mittente_nome": "Ufficio acquisti ACME", "mittente_indirizzo": "acquisti@acme.example",
		"destinatari": `[{"nome": "Banco", "indirizzo": "banco@acme.example", "tipo": "a"}]`, "oggetto": oggetto,
		"corpo_testo": corpo, "controparte_tipo": "cliente", "controparte_cliente_id": clienteACME.String(), "interno": false}
}

func allegatoACME(id, msg uuid.UUID, indice int, nome, est, sha string) riga {
	return riga{"allegato_id": id.String(), "messaggio_id": msg.String(), "contenitore_id": nil, "indice": indice, "nome_file": nome,
		"path_interno": nil, "estensione": est, "content_type": "application/octet-stream", "natura": "file", "origine": "outlook",
		"bytes": 1024, "sha256": sha, "stato": "analizzato", "path_staging": nil, "errore": nil, "ricevuto_il": istante0, "caricato_da": nil}
}

func propostaACME(n int, allegato, thread uuid.UUID, codice, rev string, comp *uuid.UUID, stato string, deciso *string) riga {
	r := riga{"proposta_id": uidACME(n).String(), "allegato_id": allegato.String(), "thread_id": thread.String(),
		"tipo_proposto": "disegno_2d", "codice": codice, "rev": nil, "componente_id": nil, "confidenza": 80, "fonte": "nome_file",
		"regola_id": nil, "stato": stato, "deciso_il": nil}
	if rev != "" {
		r["rev"] = rev
	}
	if comp != nil {
		r["componente_id"] = comp.String()
	}
	if deciso != nil {
		r["deciso_il"] = *deciso
	}
	return r
}

func documentoACME(id, comp uuid.UUID, codice, rev, nome, sha string, allegato uuid.UUID) riga {
	return riga{"documento_id": id.String(), "thread_id": threadDecisioni.String(), "componente_id": comp.String(), "tipo": "disegno_2d",
		"codice": codice, "rev": rev, "nome_file": nome, "estensione": "pdf", "sha256": sha, "stato_nas": "scritto",
		"confermato_il": istante1, "sostituito_da": nil, "allegato_id": allegato.String()}
}

// nomeFileExport: il nome di file sintetico di un export; quello dei documenti, come nel corpus, non ha l'ora.
func nomeFileExport(sez string) string {
	if sez == ExportDocumenti {
		return "documenti_senza_ora_acme.json"
	}
	return sez + "_202610010840_acme.json"
}

// fileExport: i byte di un export: {testo della query: [righe]}, con le chiavi delle righe in ordine.
func fileExport(t testing.TB, sez string, righe []riga) string {
	t.Helper()
	b, err := json.MarshalIndent(map[string][]riga{"select * from tabella_" + sez + " -- acme": righe}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// scenaBanco: la scena scritta su disco.
type scenaBanco struct {
	dir      string // la cartella del dataset (il manifest e i file dei modi)
	manifest string
	export   string // la cartella degli export
	uscita   string // la cartella dei rapporti
	shaFile  map[string]string
}

func (s scenaBanco) percorso(rel string) string { return filepath.Join(s.dir, filepath.FromSlash(rel)) }

// mutaBanco: le modifiche della scena prima di calcolare gli sha256. Le chiavi: «attesi», «casi», «indice», «manifest»
// (sul JSON finale), «export.<sezione>» (sul testo dell'export); righe cambia le righe degli export prima di scriverle.
type mutaBanco struct {
	testi map[string]func(string) string
	righe func(map[string][]riga)
}

func (m mutaBanco) applica(k, s string) string {
	if f := m.testi[k]; f != nil {
		return f(s)
	}
	return s
}

// preparaBanco: la scena ACME delle modalità di A1c. Gli attesi sono testdata/attesi_acme.yaml con le fonti degli
// export inserite al posto di «fonti: []» (gli sha256 degli export si conoscono solo qui).
func preparaBanco(t testing.TB, m mutaBanco) scenaBanco {
	t.Helper()
	base := t.TempDir()
	s := scenaBanco{dir: filepath.Join(base, "dataset"), export: filepath.Join(base, "export"), uscita: filepath.Join(base, "banco"),
		shaFile: map[string]string{}}
	scrivi := func(p, contenuto string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contenuto), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	righe := righeExportACME()
	if m.righe != nil {
		m.righe(righe)
	}
	var voci []dataset.Voce
	var fonti []string
	sezioni := make([]string, 0, len(righe))
	for sez := range righe {
		sezioni = append(sezioni, sez)
	}
	sort.Strings(sezioni)
	for _, sez := range sezioni {
		testo := m.applica("export."+sez, fileExport(t, sez, righe[sez]))
		nome := nomeFileExport(sez)
		scrivi(filepath.Join(s.export, nome), testo)
		sha := shaDi([]byte(testo))
		s.shaFile[sez] = sha
		voci = append(voci, dataset.Voce{Nome: prefissoExport + sez, Percorso: "export/" + nome, Ruolo: dataset.RuoloExport,
			Sha256: sha, Byte: int64(len(testo))})
		fonti = append(fonti, fmt.Sprintf("  - file: %s\n    sha256: %s\n", nome, sha))
	}

	g := m.applica("grammatica", leggiTestdata(t, "regole/acme.v1.json"))
	scrivi(s.percorso("regole/acme.v1.json"), g)
	casi := m.applica("casi", leggiTestdata(t, "regole/casi_acme.v1.json"))
	scrivi(s.percorso("regole/casi_acme.v1.json"), casi)
	ix := leggiTestdata(t, "regole/indice_acme.v1.json")
	ix = strings.Replace(ix, zero64, shaDi([]byte(g)), 1)
	ix = strings.Replace(ix, zero64, shaDi([]byte(casi)), 1)
	ix = m.applica("indice", ix)
	scrivi(s.percorso("regole/indice_acme.v1.json"), ix)
	at := strings.Replace(leggiTestdata(t, "attesi_acme.yaml"), "fonti: []\n", "fonti:\n"+strings.Join(fonti, ""), 1)
	at = m.applica("attesi", at)
	scrivi(s.percorso("attesi_acme.yaml"), at)

	man, err := dataset.Leggi([]byte(leggiTestdata(t, "manifest_acme.json")), s.dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range man.Voci {
		switch v.Nome {
		case VoceAttesi:
			man.Voci[i].Sha256, man.Voci[i].Byte = shaDi([]byte(at)), int64(len(at))
		case VoceIndice:
			man.Voci[i].Sha256, man.Voci[i].Byte = shaDi([]byte(ix)), int64(len(ix))
		case VoceCasi:
			man.Voci[i].Sha256, man.Voci[i].Byte = shaDi([]byte(casi)), int64(len(casi))
		}
	}
	man.Voci = append(man.Voci, voci...)
	man.Export = &dataset.ExportDichiarato{Versione: versioneTernaACME, HashConfigurazione: hashTernaACME, Schema: 21}
	js, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	scrivi(s.percorso("manifest_acme.json"), m.applica("manifest", string(js)))
	s.manifest = s.percorso("manifest_acme.json")
	return s
}

// opzioniExport: le opzioni della modalità exports sulla scena.
func (s scenaBanco) opzioniExport(attesi bool) Opzioni {
	return Opzioni{Modalita: ModalitaExports, Dataset: s.manifest, Uscita: s.uscita, Exports: s.export, Attesi: attesi}
}

// controlloBanco: un controllo del rapporto per nome.
func controlloBanco(r RapportoBanco, nome string) (Controllo, bool) {
	for _, c := range r.Controlli {
		if c.Nome == nome {
			return c, true
		}
	}
	return Controllo{}, false
}
