// L1 — DaTesto, il documento di una sola unità per gli esempi e per i casi degli attesi (piano A, 5.4.5): per i
// selettori del cartiglio e dello STEP nasce l'entità, per oggetto, corpo e storia il segmento, per il nome il
// localizzatore con stem ed estensione; la fonte è «testo», senza origine nel DB; un selettore non valido o un
// testo non UTF-8 sono errori.
//
// I testi sono inventati (ACME1111, ACME-030PB07XX0001): il repository è pubblico.
package estrazione

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// TestDaTestoCreaLEntitaOIlSegmento: un caso per ognuno degli undici contesti.
func TestDaTestoCreaLEntitaOIlSegmento(t *testing.T) {
	const testo = "ACME1111 REV 02"
	for _, c := range []struct {
		selettore string
		segmento  string // atteso: ID e tipo
		tipoSeg   string
		entita    string // tipo atteso dell'entità
		nome      string // campo atteso del localizzatore del nome
	}{
		{"oggetto", idSegCorrente, evidenze.SegmentoCorrente, "", ""},
		{"corpo", idSegCorrente, evidenze.SegmentoCorrente, "", ""},
		{"storia", idPrimaStoria, evidenze.SegmentoCitazione, "", ""},
		{"nome_file", "", "", "", campoNomeFile},
		{"voce_archivio", "", "", "", campoPercorso},
		{"cartiglio.codice", "", "", "disegno", ""},
		{"cartiglio.sconosciuto", "", "", "disegno", ""},
		{"elenco_pdf.codice", "", "", "", ""},
		{"testo_pdf", "", "", "", ""},
		{"metadati_pdf", "", "", "", ""},
		{"radice_step.id", "", "", "nodo_step", ""},
		{"nodo_step.revisione", "", "", "nodo_step", ""},
	} {
		t.Run(c.selettore, func(t *testing.T) {
			sel, err := evidenze.LeggiSelettore(c.selettore)
			if err != nil {
				t.Fatal(err)
			}
			d, err := DaTesto(sel, testo)
			if err != nil {
				t.Fatal(err)
			}
			if diag := evidenze.ValidaDocumento(d); diag != nil {
				t.Fatalf("documento non valido: %v", diag)
			}
			f := d.Fonte
			if f.Tipo != fonteTesto || f.OrigineID != uuid.Nil || f.RiferimentoFatti.Tipo != riferimentoNessuno || f.ID != "testo:"+uuid.Nil.String() {
				t.Errorf("fonte %+v", f)
			}
			if len(d.Unita) != 1 || len(d.Testi) != 1 {
				t.Fatalf("unità %d e testi %d, attesi uno e uno", len(d.Unita), len(d.Testi))
			}
			u := d.Unita[0]
			if u.ID != idUnitaTesto || u.Selettore != sel || u.Testo != testo || u.Qualita.Localizzazione != localizzazioneEsatta || u.FonteID != f.ID {
				t.Errorf("unità %+v", u)
			}
			if u.SegmentoID != c.segmento || (c.segmento == "") != (len(d.Segmenti) == 0) {
				t.Errorf("segmento %q (%d segmenti), atteso %q", u.SegmentoID, len(d.Segmenti), c.segmento)
			}
			if c.segmento != "" && d.Segmenti[0].Tipo != c.tipoSeg {
				t.Errorf("tipo del segmento %q, atteso %q", d.Segmenti[0].Tipo, c.tipoSeg)
			}
			if (c.entita == "") != (len(d.Entita) == 0) || (c.entita != "" && (d.Entita[0].Tipo != c.entita || u.EntitaID != d.Entita[0].ID)) {
				t.Errorf("entità %+v, unità legata a %q, atteso il tipo %q", d.Entita, u.EntitaID, c.entita)
			}
			if c.nome != "" {
				p := u.Posizione.NomeFile
				if p == nil || p.Campo != c.nome || d.Testi[0].ID != c.nome || d.Testi[0].Origine != idTestoIsolato {
					t.Errorf("localizzatore del nome %+v, testo %+v", p, d.Testi[0])
				}
			} else if u.Posizione.Testo == nil || u.Posizione.Testo.Intervallo != intero(testo) {
				t.Errorf("localizzatore %+v: atteso tutto il testo isolato", u.Posizione)
			}
		})
	}

	t.Run("il BundleID cambia con il selettore e con il testo", func(t *testing.T) {
		a, _ := DaTesto(selettoreDi(evidenze.ContestoCorpo), "ACME1111")
		b, _ := DaTesto(selettoreDi(evidenze.ContestoOggetto), "ACME1111")
		c, _ := DaTesto(selettoreDi(evidenze.ContestoCorpo), "ACME2222")
		a2, _ := DaTesto(selettoreDi(evidenze.ContestoCorpo), "ACME1111")
		if a.BundleID == b.BundleID || a.BundleID == c.BundleID || a.BundleID != a2.BundleID {
			t.Errorf("BundleID %s %s %s %s", a.BundleID, b.BundleID, c.BundleID, a2.BundleID)
		}
	})

	t.Run("un selettore non valido o un testo non UTF-8 sono errori", func(t *testing.T) {
		if _, err := DaTesto(evidenze.Selettore{Contesto: "figlio_step"}, "ACME1111"); err == nil {
			t.Error("un selettore fuori elenco passa")
		}
		if _, err := DaTesto(selettoreDi(evidenze.ContestoCorpo), "ACME\xff1111"); err == nil {
			t.Error("un testo non UTF-8 passa")
		}
	})
}
