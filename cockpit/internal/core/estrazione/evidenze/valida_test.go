package evidenze

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// L1 — le due porte del contratto (A1a-DOC; parte 1 §3.1-§3.4, §7.2, §9.3; A-C03; R49 C): ogni violazione
// di un documento o di un uso dei segmenti dà esattamente la sua diagnostica documento.*, errore di
// contratto, con il percorso del campo. Nessun ripiego: un documento sbagliato non diventa «parziale» e un
// uso sbagliato non diventa «sconosciuto».
//
// I clienti di questi test sono inventati: i documenti sono quelli di documento_test.go, con i codici di
// ACME, e ogni caso ne cambia un pezzo solo.

type casoDocumento struct {
	nome     string
	base     func(*testing.T) DocumentoEvidenze
	cambia   func(*DocumentoEvidenze)
	codice   string
	percorso string
}

func unita(d *DocumentoEvidenze, id string) *UnitaEvidenza {
	for i := range d.Unita {
		if d.Unita[i].ID == id {
			return &d.Unita[i]
		}
	}
	panic("unità di prova assente: " + id)
}

func TestA1aDOCValidaDocumento(t *testing.T) {
	msg, all, txt := documentoMessaggio, documentoAllegato, documentoTesto
	inizioE := strings.Index(testoCartiglioACME, "è")
	casi := []casoDocumento{
		// riferimenti
		{"segmento dell'unità assente", msg, func(d *DocumentoEvidenze) { unita(d, "u:oggetto").SegmentoID = "s:nessuno" },
			CodiceDocumentoRiferimentoPendente, "unita[u:oggetto].segmento_id"},
		{"entità dell'unità assente", all, func(d *DocumentoEvidenze) { unita(d, "u:pdf:cartiglio:1").EntitaID = "e:pdf:p9" },
			CodiceDocumentoRiferimentoPendente, "unita[u:pdf:cartiglio:1].entita_id"},
		{"unità di un'altra fonte", msg, func(d *DocumentoEvidenze) { unita(d, "u:oggetto").FonteID = "messaggio:altro" },
			CodiceDocumentoRiferimentoPendente, "unita[u:oggetto].fonte_id"},
		{"testo del localizzatore assente", msg, func(d *DocumentoEvidenze) { unita(d, "u:oggetto").Posizione.Testo.TestoID = "messaggio.corpo_html" },
			CodiceDocumentoRiferimentoPendente, "unita[u:oggetto].posizione.testo.intervallo"},
		{"segmento padre assente", msg, func(d *DocumentoEvidenze) { d.Segmenti[1].PadreID = "s:altro" },
			CodiceDocumentoRiferimentoPendente, "segmenti[s:storia].padre_id"},
		{"segmento dell'entità assente", msg, func(d *DocumentoEvidenze) { d.Entita[0].SegmentoID = "s:altro" },
			CodiceDocumentoRiferimentoPendente, "entita[e:tab:1:r1].segmento_id"},
		{"estremo del legame assente", msg, func(d *DocumentoEvidenze) { d.Legami[0].A = "e:tab:9:r9" },
			CodiceDocumentoRiferimentoPendente, "legami[l:intestazione:1].a"},
		{"cella esatta senza il corpo", all, func(d *DocumentoEvidenze) {
			d.Unita = append(d.Unita, UnitaEvidenza{ID: "u:tab", FonteID: d.Fonte.ID, Selettore: Selettore{Contesto: ContestoCorpo, Campo: CampoFonte{Variante: VarianteNessuno}},
				Testo: "A", Posizione: Localizzatore{Tipo: "tabella", Tabella: &PosTabella{Tabella: 1, Riga: 1, Cella: 1, Esatto: &Intervallo{0, 1}}},
				Qualita: QualitaUnita{Localizzazione: "parziale"}})
		}, CodiceDocumentoRiferimentoPendente, "unita[u:tab].posizione.tabella.esatto"},
		{"nome del file senza il suo testo", all, func(d *DocumentoEvidenze) { unita(d, "u:nome").Posizione.NomeFile.Campo = "allegato.path_interno" },
			CodiceDocumentoRiferimentoPendente, "unita[u:nome].posizione.nome_file.intervallo"},

		// intervalli
		{"intervallo oltre il testo", msg, func(d *DocumentoEvidenze) { d.Segmenti[1].Posizione.Testo.Intervallo.Fine = len(corpoACME) + 1 },
			CodiceDocumentoIntervalloNonValido, "segmenti[s:storia].posizione.testo.intervallo"},
		{"intervallo rovesciato", msg, func(d *DocumentoEvidenze) { d.Segmenti[0].Posizione.Testo.Intervallo = Intervallo{5, 2} },
			CodiceDocumentoIntervalloNonValido, "segmenti[s:corrente].posizione.testo.intervallo"},
		{"intervallo negativo", msg, func(d *DocumentoEvidenze) { d.Segmenti[0].Posizione.Testo.Intervallo.Inizio = -1 },
			CodiceDocumentoIntervalloNonValido, "segmenti[s:corrente].posizione.testo.intervallo"},
		{"intervallo a metà runa nel corpo", msg, func(d *DocumentoEvidenze) {
			trattino := strings.Index(corpoACME, "—") // tre byte
			d.Segmenti[0].Posizione.Testo.Intervallo.Fine = trattino + 1
		}, CodiceDocumentoIntervalloNonValido, "segmenti[s:corrente].posizione.testo.intervallo"},
		{"intervallo a metà runa nell'unità esatta", msg, func(d *DocumentoEvidenze) {
			emoji := strings.Index(corpoACME, "😀") // quattro byte
			unita(d, "u:corpo:s:corrente").Posizione.Testo.Intervallo.Fine = emoji + 2
		}, CodiceDocumentoIntervalloNonValido, "unita[u:corpo:s:corrente].posizione.testo.intervallo"},
		{"intervallo PDF a metà runa del testo dell'unità", all, func(d *DocumentoEvidenze) {
			unita(d, "u:pdf:cartiglio:1").Posizione.PDF.Intervallo = &Intervallo{inizioE + 1, inizioE + 2}
		}, CodiceDocumentoIntervalloNonValido, "unita[u:pdf:cartiglio:1].posizione.pdf.intervallo"},
		{"estensione oltre il nome", all, func(d *DocumentoEvidenze) { unita(d, "u:nome").Posizione.NomeFile.Estensione.Fine++ },
			CodiceDocumentoIntervalloNonValido, "unita[u:nome].posizione.nome_file.estensione"},

		// UTF-8
		{"testo originale non UTF-8", txt, func(d *DocumentoEvidenze) {
			d.Testi[0].Testo = "ACME-030P71\xff0100"
			unita(d, "u:testo").Qualita.Localizzazione = "parziale"
		},
			CodiceDocumentoUTF8NonValido, "testi[testo].testo"},
		{"testo dell'unità non UTF-8", txt, func(d *DocumentoEvidenze) { unita(d, "u:testo").Testo = "ACME-030P7120\xc3" },
			CodiceDocumentoUTF8NonValido, "unita[u:testo].testo"},

		// enum e coppie
		{"versione dello schema", txt, func(d *DocumentoEvidenze) { d.VersioneSchema = 2 }, CodiceDocumentoEnumIgnoto, "versione_schema"},
		{"tipo di fonte", msg, func(d *DocumentoEvidenze) { d.Fonte.Tipo = "posta" }, CodiceDocumentoEnumIgnoto, "fonte.tipo"},
		{"tipo di riferimento ai fatti", all, func(d *DocumentoEvidenze) { d.Fonte.RiferimentoFatti.Tipo = "export" }, CodiceDocumentoEnumIgnoto, "fonte.riferimento_fatti.tipo"},
		{"fonte «testo» con un'origine nel DB", txt, func(d *DocumentoEvidenze) { d.Fonte.OrigineID = uuid.MustParse("6a1d0c2e-0000-4000-8000-000000000003") },
			CodiceDocumentoEnumIgnoto, "fonte"},
		{"fonte «testo» con dei fatti", txt, func(d *DocumentoEvidenze) { d.Fonte.RiferimentoFatti.Tipo = "messaggio" }, CodiceDocumentoEnumIgnoto, "fonte"},
		{"tipo di segmento", msg, func(d *DocumentoEvidenze) { d.Segmenti[1].Tipo = "storico" }, CodiceDocumentoEnumIgnoto, "segmenti[s:storia].tipo"},
		{"origine del segmento vuota", msg, func(d *DocumentoEvidenze) { d.Segmenti[0].Origine = "" }, CodiceDocumentoEnumIgnoto, "segmenti[s:corrente].origine"},
		{"tipo di entità", all, func(d *DocumentoEvidenze) { d.Entita[0].Tipo = "tavola" }, CodiceDocumentoEnumIgnoto, "entita[e:pdf:p1].tipo"},
		{"coppia del selettore non ammessa", all, func(d *DocumentoEvidenze) {
			unita(d, "u:pdf:cartiglio:1").Selettore = Selettore{Contesto: ContestoCartiglio, Campo: CampoFonte{Variante: VarianteStep, Valore: "id"}}
		}, CodiceDocumentoEnumIgnoto, "unita[u:pdf:cartiglio:1].selettore"},
		{"selettore vuoto", txt, func(d *DocumentoEvidenze) { unita(d, "u:testo").Selettore = Selettore{} }, CodiceDocumentoEnumIgnoto, "unita[u:testo].selettore"},
		{"localizzazione vuota", txt, func(d *DocumentoEvidenze) { unita(d, "u:testo").Qualita.Localizzazione = "" }, CodiceDocumentoEnumIgnoto, "unita[u:testo].qualita.localizzazione"},
		{"metodo ignoto", txt, func(d *DocumentoEvidenze) { unita(d, "u:testo").Qualita.Metodo = "llm" }, CodiceDocumentoEnumIgnoto, "unita[u:testo].qualita.metodo"},
		{"tipo di localizzatore ignoto", all, func(d *DocumentoEvidenze) { d.Entita[0].Posizione = Localizzatore{Tipo: "foglio"} },
			CodiceDocumentoEnumIgnoto, "entita[e:pdf:p1].posizione.tipo"},
		{"variante del localizzatore in più", txt, func(d *DocumentoEvidenze) { unita(d, "u:testo").Posizione.PDF = &PosPDF{Pagina: 1} },
			CodiceDocumentoEnumIgnoto, "unita[u:testo].posizione"},
		{"variante del localizzatore mancante", all, func(d *DocumentoEvidenze) { d.Entita[0].Posizione.PDF = nil },
			CodiceDocumentoEnumIgnoto, "entita[e:pdf:p1].posizione"},
		{"zona del PDF", all, func(d *DocumentoEvidenze) { d.Entita[0].Posizione.PDF.Zona = "alto_sinistra" }, CodiceDocumentoEnumIgnoto, "entita[e:pdf:p1].posizione.pdf.zona"},
		{"attributo STEP", all, func(d *DocumentoEvidenze) {
			d.Entita[0].Posizione = Localizzatore{Tipo: "step", STEP: &PosSTEP{Chiave: "#19", Attributo: "colore"}}
		}, CodiceDocumentoEnumIgnoto, "entita[e:pdf:p1].posizione.step.attributo"},
		{"campo del nome del file", all, func(d *DocumentoEvidenze) { unita(d, "u:nome").Posizione.NomeFile.Campo = "allegato.oggetto" },
			CodiceDocumentoEnumIgnoto, "unita[u:nome].posizione.nome_file.campo"},
		{"tipo di legame", msg, func(d *DocumentoEvidenze) { d.Legami[0].Tipo = "somiglianza" }, CodiceDocumentoEnumIgnoto, "legami[l:intestazione:1].tipo"},
		{"quantità su un legame che non la porta", msg, func(d *DocumentoEvidenze) { q := 4; d.Legami[0].Quantita = &q },
			CodiceDocumentoEnumIgnoto, "legami[l:intestazione:1].quantita"},
		{"stato della fonte", txt, func(d *DocumentoEvidenze) { d.Qualita.Stato = "assente" }, CodiceDocumentoEnumIgnoto, "qualita.stato"},
		{"mappatura", all, func(d *DocumentoEvidenze) { d.Qualita.Mappatura = "certa" }, CodiceDocumentoEnumIgnoto, "qualita.mappatura"},
		{"stato di una capacità", msg, func(d *DocumentoEvidenze) { d.Qualita.Capacita[1].Stato = "errore" }, CodiceDocumentoEnumIgnoto, "qualita.capacita[1].stato"},

		// A-C03
		{"testo diverso dall'originale", msg, func(d *DocumentoEvidenze) { unita(d, "u:oggetto").Testo = strings.ToUpper(oggettoACME) },
			CodiceDocumentoTestoDiversoDaOriginale, "unita[u:oggetto].testo"},
		{"cella esatta diversa dal corpo", msg, func(d *DocumentoEvidenze) { unita(d, "u:tab:1:r1:c1").Testo = "P7120101" },
			CodiceDocumentoTestoDiversoDaOriginale, "unita[u:tab:1:r1:c1].testo"},
		{"nome esatto diverso dal campo", all, func(d *DocumentoEvidenze) { unita(d, "u:nome").Testo = "acme-030p7120100.pdf" },
			CodiceDocumentoTestoDiversoDaOriginale, "unita[u:nome].testo"},
		{"testo con lo spazio tolto", txt, func(d *DocumentoEvidenze) {
			d.Testi[0].Testo = " ACME-030P7120100"
			unita(d, "u:testo").Posizione.Testo.Intervallo = Intervallo{0, len(" ACME-030P7120100")}
		}, CodiceDocumentoTestoDiversoDaOriginale, "unita[u:testo].testo"},

		// ID
		{"due unità con lo stesso ID", msg, func(d *DocumentoEvidenze) { unita(d, "u:corpo:s:storia").ID = "u:oggetto" },
			CodiceDocumentoIDRipetuto, "unita[u:oggetto]"},
		{"un'unità con l'ID di un segmento", msg, func(d *DocumentoEvidenze) { unita(d, "u:oggetto").ID = "s:corrente" },
			CodiceDocumentoIDRipetuto, "unita[s:corrente]"},
		{"due testi con lo stesso ID", txt, func(d *DocumentoEvidenze) { d.Testi = append(d.Testi, d.Testi[0]) },
			CodiceDocumentoIDRipetuto, "testi[testo]"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d := c.base(t)
			c.cambia(&d)
			diag := ValidaDocumento(d)
			if len(diag) != 1 {
				t.Fatalf("attesa una diagnostica %s, ottenute %d: %+v", c.codice, len(diag), diag)
			}
			x := diag[0]
			if x.Codice != c.codice || x.Gravita != GravitaErrore || x.Natura != NaturaContratto {
				t.Errorf("%+v, atteso il codice %s, errore di contratto", x, c.codice)
			}
			if x.Percorso != c.percorso {
				t.Errorf("percorso %q, atteso %q", x.Percorso, c.percorso)
			}
			if x.Messaggio == "" {
				t.Error("diagnostica senza messaggio")
			}
		})
	}
}

// TestA1aDOCValidaDocumentoDeterministica: due chiamate sullo stesso documento danno lo stesso elenco, nello
// stesso ordine; più violazioni danno più diagnostiche, nell'ordine del documento.
func TestA1aDOCValidaDocumentoDeterministica(t *testing.T) {
	d := documentoMessaggio(t)
	d.Fonte.Tipo = "posta"
	d.Segmenti[1].Tipo = "storico"
	unita(&d, "u:oggetto").Testo = "altro"
	d.Qualita.Stato = "?"
	primo := ValidaDocumento(d)
	var codici []string
	for _, x := range primo {
		codici = append(codici, x.Codice)
	}
	attesi := []string{CodiceDocumentoEnumIgnoto, CodiceDocumentoEnumIgnoto, CodiceDocumentoTestoDiversoDaOriginale, CodiceDocumentoEnumIgnoto}
	if !reflect.DeepEqual(codici, attesi) {
		t.Fatalf("codici %v, attesi %v", codici, attesi)
	}
	for i := 0; i < 10; i++ {
		if !reflect.DeepEqual(ValidaDocumento(d), primo) {
			t.Fatal("ValidaDocumento non è deterministica")
		}
	}
}

func TestA1aDOCValidaUso(t *testing.T) {
	valido := UsoSegmenti{BundleID: "bundle-messaggio", Versione: 1, Stato: "valutato", Selezioni: []SelezioneSegmento{
		{SegmentoID: "s:corrente", Uso: "pertinente", Origine: "riconoscimento", Motivo: "parte corrente"},
		{SegmentoID: "s:storia", Uso: "da_valutare", Origine: "scenario", Rif: "caso-acme-1"},
	}}
	if diag := ValidaUso(valido, documentoMessaggio(t)); diag != nil {
		t.Fatalf("uso valido: %+v", diag)
	}
	casi := []struct {
		nome     string
		cambia   func(*UsoSegmenti)
		percorso string
	}{
		{"di un altro bundle", func(u *UsoSegmenti) { u.BundleID = "bundle-allegato" }, "uso.bundle_id"},
		{"versione ignota", func(u *UsoSegmenti) { u.Versione = 2 }, "uso.versione"},
		{"stato ignoto", func(u *UsoSegmenti) { u.Stato = "deciso" }, "uso.stato"},
		{"sconosciuto con selezioni", func(u *UsoSegmenti) { u.Stato = "sconosciuto" }, "uso.selezioni"},
		{"segmento che non c'è", func(u *UsoSegmenti) { u.Selezioni[1].SegmentoID = "s:storia:2" }, "uso.selezioni[1].segmento_id"},
		{"segmento selezionato due volte", func(u *UsoSegmenti) { u.Selezioni[1].SegmentoID = "s:corrente" }, "uso.selezioni[1].segmento_id"},
		{"uso ignoto", func(u *UsoSegmenti) { u.Selezioni[0].Uso = "richiesta" }, "uso.selezioni[0].uso"},
		{"origine ignota", func(u *UsoSegmenti) { u.Selezioni[0].Origine = "agente_automatico" }, "uso.selezioni[0].origine"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			u := valido
			u.Selezioni = append([]SelezioneSegmento(nil), valido.Selezioni...)
			c.cambia(&u)
			diag := ValidaUso(u, documentoMessaggio(t))
			if len(diag) != 1 {
				t.Fatalf("attesa una diagnostica, ottenute %d: %+v", len(diag), diag)
			}
			x := diag[0]
			if x.Codice != CodiceDocumentoUsoNonValido || x.Gravita != GravitaErrore || x.Natura != NaturaContratto || x.Percorso != c.percorso {
				t.Errorf("%+v, atteso documento.uso_non_valido su %q", x, c.percorso)
			}
		})
	}
	// Un uso sconosciuto di un altro documento non diventa valido perché «sconosciuto».
	if diag := ValidaUso(UsoSconosciuto("bundle-allegato"), documentoMessaggio(t)); len(diag) != 1 || diag[0].Codice != CodiceDocumentoUsoNonValido {
		t.Fatalf("uso sconosciuto di un altro bundle: %+v", diag)
	}
}
