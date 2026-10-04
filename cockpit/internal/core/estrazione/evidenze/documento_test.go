package evidenze

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — il documento delle evidenze (A1a-DOC; parte 1 §3.1-§3.4, §7.2, §9.3; R49 C): i DTO costruiti in Go
// per tre fonti (un messaggio, un allegato, un testo isolato) passano la validazione, si scrivono in JSON
// canonico e tornano uguali; «nessuna selezione» ha una forma canonica sola.
//
// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un
// dato dell'azienda e questo repository è pubblico, e un test che dipendesse da esse diventerebbe rosso il
// giorno in cui un cliente cambia convenzione. I codici sono quelli di ACME, già pubblici nelle prove.

const (
	oggettoACME        = "Richiesta d'offerta ACME-030P7120100"
	corpoACME          = "Buongiorno,\nservono 4 pezzi di P7120100 — grazie 😀\n> già chiesto: 9123456\n"
	nomeACME           = "ACME-030P7120100.pdf"
	testoCartiglioACME = "codice: è P7120100"
)

func sel(t *testing.T, s string) Selettore {
	t.Helper()
	x, err := LeggiSelettore(s)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func posTesto(id string, inizio, fine int) Localizzatore {
	return Localizzatore{Tipo: "testo", Testo: &PosTesto{TestoID: id, Intervallo: Intervallo{inizio, fine}}}
}

// documentoMessaggio: un messaggio con oggetto e corpo, due segmenti, una riga di tabella agganciata in modo
// esatto al corpo, un legame.
func documentoMessaggio(t *testing.T) DocumentoEvidenze {
	t.Helper()
	storia := strings.Index(corpoACME, "> già")
	codice := strings.Index(corpoACME, "P7120100")
	id := uuid.MustParse("6a1d0c2e-0000-4000-8000-000000000001")
	fonte := "messaggio:" + id.String()
	return DocumentoEvidenze{
		BundleID:           "bundle-messaggio",
		VersioneSchema:     VersioneSchemaDocumento,
		VersioneAdattatore: "adattatori-prova",
		Fonte: Fonte{
			ID:        fonte,
			Tipo:      "messaggio",
			OrigineID: id,
			Provenienza: Provenienza{
				Canale: "outlook",
				Autore: "ufficio@acme.example",
				Data:   time.Date(2026, 10, 4, 8, 15, 30, 250000000, time.UTC),
			},
			RiferimentoFatti: RiferimentoFatti{Tipo: "messaggio", DigestTesti: strings.Repeat("ab", 32)},
		},
		Testi: []TestoOriginale{
			{ID: "messaggio.oggetto", Testo: oggettoACME, Origine: "oggetto"},
			{ID: "messaggio.corpo_testo", Testo: corpoACME, Origine: "corpo_testo"},
		},
		Segmenti: []Segmento{
			{ID: "s:corrente", Tipo: SegmentoCorrente, MessaggioLogicoID: "m0", Origine: "mittente_del_messaggio", Posizione: posTesto("messaggio.corpo_testo", 0, storia)},
			{ID: "s:storia", Tipo: SegmentoCitazione, MessaggioLogicoID: "m1", PadreID: "s:corrente", Origine: "ignota", Posizione: posTesto("messaggio.corpo_testo", storia, len(corpoACME))},
		},
		Entita: []EntitaLocale{
			{ID: "e:tab:1:r1", Tipo: "riga", SegmentoID: "s:corrente", ChiaveOriginale: "tabella 1 riga 1",
				Posizione: Localizzatore{Tipo: "tabella", Tabella: &PosTabella{Tabella: 1, Riga: 1, Cella: 1, RigheTesto: &[2]int{2, 2}}}},
		},
		Unita: []UnitaEvidenza{
			{ID: "u:oggetto", FonteID: fonte, Selettore: sel(t, "oggetto"), Testo: oggettoACME,
				Posizione: posTesto("messaggio.oggetto", 0, len(oggettoACME)),
				Qualita:   QualitaUnita{Localizzazione: "esatta", Metodo: "adattatore"}},
			{ID: "u:corpo:s:corrente", FonteID: fonte, SegmentoID: "s:corrente", Selettore: sel(t, "corpo"), Testo: corpoACME[:storia],
				Posizione: posTesto("messaggio.corpo_testo", 0, storia),
				Qualita:   QualitaUnita{Localizzazione: "esatta", Metodo: "adattatore"}},
			{ID: "u:corpo:s:storia", FonteID: fonte, SegmentoID: "s:storia", Selettore: sel(t, "storia"), Testo: corpoACME[storia:],
				Posizione: posTesto("messaggio.corpo_testo", storia, len(corpoACME)),
				Qualita:   QualitaUnita{Localizzazione: "esatta", Metodo: "adattatore"}},
			{ID: "u:tab:1:r1:c1", FonteID: fonte, SegmentoID: "s:corrente", EntitaID: "e:tab:1:r1", Selettore: sel(t, "corpo"), Testo: "P7120100",
				CampoOriginale: CampoOriginale{Etichetta: "Codice", Parser: "tabella_html"},
				Posizione: Localizzatore{Tipo: "tabella", Tabella: &PosTabella{Tabella: 1, Riga: 1, Cella: 1, Intestazione: "Codice",
					RigheTesto: &[2]int{2, 2}, Esatto: &Intervallo{codice, codice + len("P7120100")}}},
				Qualita: QualitaUnita{Localizzazione: "esatta", Metodo: "parser"}},
		},
		Legami: []LegameFonte{
			{ID: "l:intestazione:1", Tipo: "intestazione_di_cella", Da: "u:tab:1:r1:c1", A: "e:tab:1:r1"},
		},
		Qualita: QualitaFonte{
			Stato: "disponibile", Metodo: "adattatore", Mappatura: "verificata",
			Capacita: []Capacita{{Nome: "segmentazione", Stato: "parziale", Motivo: "riconoscitori dei confini non per tutte le lingue"}, {Nome: "firma", Stato: "non_disponibile"}},
		},
	}
}

// documentoAllegato: un PDF con il nome del file e un campo del cartiglio.
func documentoAllegato(t *testing.T) DocumentoEvidenze {
	t.Helper()
	id := uuid.MustParse("6a1d0c2e-0000-4000-8000-000000000002")
	fonte := "allegato:" + id.String()
	byteLetti := int64(48213)
	codice := strings.Index(testoCartiglioACME, "P7120100")
	return DocumentoEvidenze{
		BundleID:           "bundle-allegato",
		VersioneSchema:     VersioneSchemaDocumento,
		VersioneAdattatore: "adattatori-prova",
		Fonte: Fonte{
			ID:            fonte,
			Tipo:          "allegato",
			OrigineID:     id,
			ContenitoreID: "messaggio:6a1d0c2e-0000-4000-8000-000000000001",
			Provenienza:   Provenienza{NomeRicevuto: nomeACME, ContentType: "application/pdf", Bytes: &byteLetti, Ignoti: []string{"autore"}},
			RiferimentoFatti: RiferimentoFatti{Tipo: "analisi_file", Sha256: strings.Repeat("cd", 32), VersioneAnalizzatore: 3,
				HashConfigurazione: strings.Repeat("ef", 32), CalcolatoIl: time.Date(2026, 10, 3, 17, 0, 0, 0, time.UTC), SottoversionePDF: 2},
		},
		Testi: []TestoOriginale{{ID: "allegato.nome_file", Testo: nomeACME, Origine: "allegato.nome_file"}},
		Entita: []EntitaLocale{
			{ID: "e:pdf:p1", Tipo: "disegno", ChiaveOriginale: "pagina 1", Posizione: Localizzatore{Tipo: "pdf", PDF: &PosPDF{Pagina: 1, Zona: "pagina"}}},
		},
		Unita: []UnitaEvidenza{
			{ID: "u:nome", FonteID: fonte, Selettore: sel(t, "nome_file"), Testo: nomeACME,
				Posizione: Localizzatore{Tipo: "nome_file", NomeFile: &PosNomeFile{Campo: "allegato.nome_file", Intervallo: Intervallo{0, len(nomeACME)},
					Stem: &Intervallo{0, len(nomeACME) - 4}, Estensione: &Intervallo{len(nomeACME) - 3, len(nomeACME)}}},
				Qualita: QualitaUnita{Localizzazione: "esatta", Metodo: "adattatore"}},
			{ID: "u:pdf:cartiglio:1", FonteID: fonte, EntitaID: "e:pdf:p1", Selettore: sel(t, "cartiglio.codice"), Testo: testoCartiglioACME,
				CampoOriginale: CampoOriginale{Etichetta: "codice", Parser: "cartiglio", Mappatura: "1"},
				Posizione: Localizzatore{Tipo: "pdf", PDF: &PosPDF{Pagina: 1, RiquadroDecimi: &[4]int{4210, 5600, 5890, 5820}, Zona: "basso_destra", Fonte: "nativo",
					Intervallo: &Intervallo{codice, codice + len("P7120100")}}},
				Qualita: QualitaUnita{Localizzazione: "parziale", Motivo: "coordinate solo per pagina", Metodo: "nativo"}},
		},
		Qualita: QualitaFonte{Stato: "disponibile", Metodo: "nativo", Mappatura: "plausibile"},
	}
}

// documentoTesto: la fonte «testo», senza origine nel DB (gli esempi delle grammatiche, un testo isolato).
func documentoTesto(t *testing.T) DocumentoEvidenze {
	t.Helper()
	return DocumentoEvidenze{
		BundleID:           "bundle-testo",
		VersioneSchema:     VersioneSchemaDocumento,
		VersioneAdattatore: "adattatori-prova",
		Fonte:              Fonte{ID: "testo:acme-1", Tipo: "testo", OrigineID: uuid.Nil, RiferimentoFatti: RiferimentoFatti{Tipo: "nessuno"}},
		Testi:              []TestoOriginale{{ID: "testo", Testo: "ACME-030P7120100", Origine: "esempio"}},
		Unita: []UnitaEvidenza{
			{ID: "u:testo", FonteID: "testo:acme-1", Selettore: sel(t, "corpo"), Testo: "ACME-030P7120100",
				Posizione: posTesto("testo", 0, len("ACME-030P7120100")), Qualita: QualitaUnita{Localizzazione: "esatta"}},
		},
		Qualita: QualitaFonte{Stato: "disponibile"},
	}
}

// TestA1aDOCVersioniECostanti: le costanti che entrano nel contratto e nelle impronte.
func TestA1aDOCVersioniECostanti(t *testing.T) {
	if VersioneSchemaDocumento != 1 {
		t.Fatalf("VersioneSchemaDocumento = %d", VersioneSchemaDocumento)
	}
	tipi := []string{SegmentoCorrente, SegmentoCitazione, SegmentoInoltro, SegmentoFirma, SegmentoSezioneTecnica}
	if strings.Join(tipi, " ") != "corrente citazione inoltro firma sezione_tecnica" {
		t.Fatalf("tipi dei segmenti: %v", tipi)
	}
}

// TestA1aDOCFixtureValide: le tre fonti passano ValidaDocumento senza diagnostiche, anche la «testo».
func TestA1aDOCFixtureValide(t *testing.T) {
	for nome, d := range map[string]DocumentoEvidenze{
		"messaggio": documentoMessaggio(t),
		"allegato":  documentoAllegato(t),
		"testo":     documentoTesto(t),
	} {
		if diag := ValidaDocumento(d); diag != nil {
			t.Errorf("%s: %+v", nome, diag)
		}
	}
}

// TestA1aDOCJSONCanonicoAndataERitorno: il JSON canonico di un documento si rilegge nello stesso documento,
// che si riscrive con gli stessi byte. Il selettore viaggia nella sua forma testuale; i campi assenti
// (omitempty, omitzero) restano assenti.
func TestA1aDOCJSONCanonicoAndataERitorno(t *testing.T) {
	for nome, d := range map[string]DocumentoEvidenze{
		"messaggio": documentoMessaggio(t),
		"allegato":  documentoAllegato(t),
		"testo":     documentoTesto(t),
	} {
		b, err := jsoncanonico.Codifica(d)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		var di DocumentoEvidenze
		if err := json.Unmarshal(b, &di); err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if !reflect.DeepEqual(di, d) {
			t.Errorf("%s: il documento riletto è diverso\n got %+v\nwant %+v", nome, di, d)
		}
		b2, err := jsoncanonico.Codifica(di)
		if err != nil || string(b2) != string(b) {
			t.Errorf("%s: la seconda scrittura è diversa (%v)", nome, err)
		}
	}
	b, _ := jsoncanonico.Codifica(documentoTesto(t))
	s := string(b)
	for _, assente := range []string{`"data"`, `"calcolato_il"`, `"bytes"`, `"segmenti"`, `"legami"`, `"contenitore_id"`} {
		if strings.Contains(s, assente) {
			t.Errorf("la fonte «testo» scrive %s, che non ha", assente)
		}
	}
	if !strings.Contains(s, `"selettore":"corpo"`) || !strings.Contains(s, `"origine_id":"00000000-0000-0000-0000-000000000000"`) {
		t.Errorf("selettore o origine non nella forma attesa: %s", s)
	}
}

// TestA1aDOCUsoSconosciuto: «nessuna selezione» ha una forma canonica fissa, con la sua impronta, mai NULL
// (parte 1 §9.3); appartiene al suo documento.
func TestA1aDOCUsoSconosciuto(t *testing.T) {
	u := UsoSconosciuto("bundle-messaggio")
	b, err := jsoncanonico.Codifica(u)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"bundle_id":"bundle-messaggio","stato":"sconosciuto","versione":1}`; got != want {
		t.Fatalf("%s, atteso %s", got, want)
	}
	if diag := ValidaUso(u, documentoMessaggio(t)); diag != nil {
		t.Fatalf("%+v", diag)
	}
	i1, _ := jsoncanonico.ImprontaDi(UsoSconosciuto("bundle-messaggio"))
	i2, _ := jsoncanonico.ImprontaDi(UsoSconosciuto("bundle-allegato"))
	if len(i1) != 64 || i1 == i2 {
		t.Fatalf("impronte dell'uso sconosciuto: %s %s", i1, i2)
	}
}
