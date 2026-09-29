//go:build integrazione

// L4 — lo scenario «MG» (prova da fare del giro 4; docs/domande/scenario_MG_28-09.md, fuori da git) contro
// PostgreSQL vero: la RFQ della mail con quattro prodotti chiesti con la P, cinque STEP d'assieme con la radice
// senza, figli in comune, 49 file con il prefisso di progetto nel nome. Le stesse letture delle prove L1
// (smistamento_scenario_mg_test.go), qui scritte come le scrive l'analisi (documento_proposta, ApplicaStruttura)
// e smistate da Rismista. Nessuna identita' senza una persona: ne' la struttura degli STEP ne' il flusso creano
// un componente, un arco o un documento.

package fascicolo_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// regoleMG: la famiglia ACME con la forma di quella del cliente vero (P facoltativa dentro il codice, -revNN).
const regoleMG = `{"famiglie_codice": [{"regex": "(?i)(?:^|[^0-9A-Za-z])(?P<codice>P?712[0-9]{4})(?:-(?P<rev>rev\\d{2}))?(?:[^0-9A-Za-z]|$)",
	"descrizione": "codici ACME: 712 + 4 cifre, eventuale P davanti, revisione -revNN nei file", "esempio": "7120001-rev01", "rev_nel_codice": true}]}`

type assiemeMGDB struct {
	codice, speculare string
	figli             []string
}

var assiemiMGDB = []assiemeMGDB{
	{"7120100", "7120101", []string{"7120110", "7120111", "7120112", "7120113", "7120130", "7120131", "7120114", "7120115", "7120116", "7120117"}},
	{"7120101", "7120100", []string{"7120111", "7120112", "7120113", "7120130", "7120131", "7120114", "7120115", "7120116", "7120117", "7120118"}},
	{"7120102", "", []string{"7120121", "7120120", "7120135"}},
	{"7120103", "7120104", []string{"7120122", "7120132", "7120133*2", "7120134"}},
	{"7120104", "7120103", []string{"7120122", "7120132", "7120133*2", "7120134"}},
}

var particolariMGDB = []struct {
	codice string
	dxf    bool
}{{"7120105", false}, {"7120106", true}, {"7120110", true}, {"7120111", true}, {"7120112", false}, {"7120113", true},
	{"7120114", true}, {"7120115", true}, {"7120116", true}, {"7120117", true}, {"7120120", true}, {"7120121", true},
	{"7120122", false}, {"7120118", true}}

// fattiPdfMG sono i fatti dell'analizzatore 4 di un disegno del cliente: elenco particolari (se c'e') e cartiglio
// nella zona in basso a destra, il campo «codice» del worker sulla prima riga dell'elenco.
func fattiPdfMG(codice, speculare string, figli []string) string {
	var b strings.Builder
	if len(figli) > 0 {
		b.WriteString("Pos. Part Number Descrizione Q.ty\n")
		for i, f := range figli {
			c, q, _ := strings.Cut(f, "*")
			if q == "" {
				q = "1"
			}
			fmt.Fprintf(&b, "%d %s PEZZO %s\n", i+1, c, q)
		}
	}
	if speculare != "" {
		b.WriteString("SPECULARE DI " + speculare + "\n")
	}
	b.WriteString("Rev Mod. N. Type\n00 ACME-P003 A Prima emissione\nFamily: Part Nr: Descrizione: Description:\n-- " + codice + " PEZZO ACME\n")
	bd := []float64{560, 400, 830, 590}
	tp := worker.TestoPDF{Versione: 1, Estraibile: true, Pagine: 1, PagineLette: 1, FormatoPagina1: []float64{842, 595}, Caratteri: b.Len(),
		OCR:       worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Frammenti: []worker.FrammentoPDF{{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Testo: b.String(), Riquadro: bd}}}
	if len(figli) > 0 {
		c, _, _ := strings.Cut(figli[0], "*")
		tp.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "Part Number", Valore: c, Pagina: 1,
			Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: bd}}
	}
	out, _ := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"},
		"fonti": map[string]string{"tipo": "termini_pdf"}, "testo_pdf": tp})
	return string(out)
}

// fileMG e' un allegato della mail con la sua lettura (tipo dal contenuto per i PDF, dal formato per i DXF). I
// fatti di un PDF sono anche i fatti CORRENTI dell'analisi (analizzatore di prova): nel ramo delle prove, sopra la
// fase 4.2, il testo del PDF arriva al flusso solo da li' (EvidenzeContenutoPDF), non dalla valutazione.
func (b *banco) fileMG(msg uuid.UUID, nome, ext, fatti string, esito *classificazione.Esito) uuid.UUID {
	b.t.Helper()
	b.n++
	sha := fmt.Sprintf("%064x", 90000+b.n)
	a := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il, stato)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, now(), 'analizzato') RETURNING allegato_id`, msg, b.n, nome, ext, sha)
	if fatti != "" {
		b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
			sha, analizzatoreProva.Versione, analizzatoreProva.Hash(), fatti)
	}
	b.letto(a, nome, esito, fatti)
	return a
}

func TestScenarioMGNelDatabase(t *testing.T) {
	b := nuovoBanco(t)
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`, regoleMG, b.thread)
	for _, p := range []string{"P7120100", "P7120103", "P7120101", "P7120104"} {
		b.prodottoConfermato(p)
	}
	componentiPrima := uno[int](b, `SELECT count(*) FROM componente`)

	pdf := map[string]uuid.UUID{}
	msg := b.mail()
	n := 0
	for _, a := range assiemiMGDB {
		nodi := []string{"#1=" + a.codice}
		var archi []string
		for i, f := range a.figli {
			c, q, _ := strings.Cut(f, "*")
			k := fmt.Sprintf("#%d", i+2)
			nodi = append(nodi, k+"="+c)
			if q != "" {
				archi = append(archi, "#1>"+k+"*"+q)
			} else {
				archi = append(archi, "#1>"+k)
			}
		}
		n++
		b.stepLetto("ACME-030P"+a.codice+" 00 IN_WORK.stp", fmt.Sprintf("%064x", 50000+n), fattiSTEP{nodi: nodi, archi: archi})
		nome := "ACME-030P" + a.codice + ".pdf"
		pdf[a.codice] = b.fileMG(msg, nome, "pdf", fattiPdfMG(a.codice, a.speculare, a.figli), &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"})
	}
	for _, p := range particolariMGDB {
		n++
		b.stepLetto("ACME-030P"+p.codice+" 00 IN_WORK.stp", fmt.Sprintf("%064x", 50000+n), fattiSTEP{nodi: []string{"#1=" + p.codice}})
		if p.dxf {
			b.fileMG(msg, "ACME-030P"+p.codice+".dxf", "dxf", "", nil)
		}
		pdf[p.codice] = b.fileMG(msg, "ACME-030P"+p.codice+".pdf", "pdf", fattiPdfMG(p.codice, "", nil), &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"})
	}
	if nf := uno[int](b, `SELECT count(*) FROM allegato`); nf != 49 {
		t.Fatalf("lo scenario ha %d allegati, non 49", nf)
	}

	// Invariante (27/09 «bis»): la lettura degli STEP (ApplicaStruttura) scrive proposte, non componenti.
	t.Run("la struttura degli STEP non crea componenti", func(t *testing.T) {
		if dopo := uno[int](b, `SELECT count(*) FROM componente`); dopo != componentiPrima {
			t.Errorf("componenti prima %d, dopo le letture %d", componentiPrima, dopo)
		}
		if r := uno[int](b, `SELECT count(*) FROM componente_relazione`); r != 0 {
			t.Errorf("archi della working nati dalle letture: %d", r)
		}
	})

	// Invariante (F8): il flusso scrive solo destinazioni.
	prima := b.fotoNonFlusso()
	es := b.rismista()
	t.Run("il flusso scrive solo destinazioni", func(t *testing.T) {
		if dopo := b.fotoNonFlusso(); dopo != prima {
			t.Errorf("il flusso ha toccato altro che le destinazioni:\nprima %s\ndopo  %s", prima, dopo)
		}
		if es.File == 0 {
			t.Errorf("il flusso non ha guardato i file: %+v", es)
		}
	})

	// Invariante: nessuna destinazione preselezionata su un pezzo che non e' quello del file.
	t.Run("nessun PDF preselezionato su un pezzo che non e' il suo", func(t *testing.T) {
		for codice, a := range pdf {
			d, ok := b.destinazione(a)
			if ok && d.Preselezionabile && (len(d.Candidati) == 0 || strings.TrimPrefix(d.Candidati[0].Codice, "P") != codice) {
				t.Errorf("ACME-030P%s.pdf preselezionato su %s", codice, righeDest(d))
			}
		}
	})

	t.Run("da fare: il PDF d'assieme ha per destinazione il prodotto chiesto con la P", func(t *testing.T) {
		d, _ := b.destinazione(pdf["7120100"])
		soddisfatta := len(d.Candidati) > 0 && strings.TrimPrefix(d.Candidati[0].Codice, "P") == "7120100"
		if soddisfatta {
			t.Log("soddisfatta: togliere il salto e lasciare l'asserzione")
			return
		}
		t.Skipf("da fare giro 4: cartiglio «Part Nr» 7120100 = prodotto P7120100 della mail (alias), l'elenco particolari non e' la destinazione (oggi: %q, esito %q, motivo %q)",
			righeDest(d), d.Esito, d.Motivo)
	})
}
