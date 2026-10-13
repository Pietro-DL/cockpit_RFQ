// L1 — L-2D, il 2D dal formato del file (A1d, diagnosi del 14/10, estensione della notte del 14/10): un file senza tipo
// documentale registrato, o da_determinare, in un formato di un 2D (pdf, tif, tiff, dxf, dwg) è un 2D, con il suo perché
// (Riconoscimento2D dal_formato); un file con un altro tipo registrato no. Sulla fotografia: i nodi proposti hanno i loro
// 2D, con il primario deterministico; un 2D ancorato al prodotto senza struttura arriva alla voce del prodotto; i file
// dell'esito portano il perché. Scene ACME inventate.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli ID dei file di L-2D (uid 0x14xx, sha di due cifre ripetute: liberi nelle altre scene).
var (
	aL2DPDF   = uid(0x1401)
	aL2DTIF   = uid(0x1402)
	aL2DDXF   = uid(0x1403)
	aL2DProd  = uid(0x1404)
	pL2DAltro = uid(0x1405)
	shaL2DPDF = strings.Repeat("1a", 32)
	shaL2DTIF = strings.Repeat("1b", 32)
	shaL2DDXF = strings.Repeat("1c", 32)
	shaL2DPro = strings.Repeat("1d", 32)
)

// TestL2DRiconosciDisegno2D: la regola, pura, sul tipo registrato e sull'estensione.
func TestL2DRiconosciDisegno2D(t *testing.T) {
	reg, form := valutazione.Riconoscimento2DTipoRegistrato, valutazione.Riconoscimento2DDalFormato
	for _, c := range []struct {
		tipo, est string
		atteso    valutazione.Riconoscimento2D
	}{
		// il tipo registrato disegno_2d decide, qualunque sia il formato
		{"disegno_2d", "pdf", reg}, {"disegno_2d", "dwg", reg}, {"disegno_2d", "png", reg},
		// nessun tipo, o da_determinare, con un formato di un 2D (senza maiuscole, con il punto, con gli spazi)
		{"", "pdf", form}, {"", "PDF", form}, {"", ".tif", form}, {"", "tiff", form}, {"", " dxf ", form}, {"", "dwg", form},
		{"da_determinare", "pdf", form}, {"da_determinare", "TIF", form}, {"da_determinare", "dxf", form},
		// nessun tipo in un altro formato: non è un 2D
		{"", "png", ""}, {"", "jpg", ""}, {"", "step", ""}, {"", "stp", ""}, {"", "igs", ""}, {"", "iges", ""}, {"", "x_t", ""},
		{"", "sldprt", ""}, {"", "zip", ""}, {"", "docx", ""}, {"", "xls", ""}, {"", "", ""}, {"da_determinare", "png", ""},
		// un altro tipo registrato non si scavalca con il formato
		{"cad_3d", "pdf", ""}, {"sviluppo_dxf", "dxf", ""}, {"altro", "pdf", ""}, {"capitolato", "pdf", ""}, {"commerciale", "tif", ""},
	} {
		if got := valutazione.RiconosciDisegno2D(c.tipo, c.est); got != c.atteso {
			t.Errorf("RiconosciDisegno2D(%q, %q) = %q, atteso %q", c.tipo, c.est, got, c.atteso)
		}
	}
	e := valutazione.EstensioniDel2DSenzaTipo()
	if !reflect.DeepEqual(e, []string{"pdf", "tif", "tiff", "dxf", "dwg"}) {
		t.Errorf("le estensioni del 2D senza tipo %v", e)
	}
	e[0] = "png"
	if valutazione.EstensioniDel2DSenzaTipo()[0] != "pdf" || valutazione.RiconosciDisegno2D("", "png") != "" {
		t.Error("l'elenco si cambia da fuori")
	}
	if valutazione.VersioneFormati2D != 1 || len(valutazione.FormatiDisegno2D()) != 3 {
		t.Error("il riconoscimento non cambia l'elenco chiuso dei formati (T-B0-30)")
	}
}

// conFileSenzaTipo: un allegato del thread con i suoi fatti (nil: nessuno) e nessun documento né proposta: il DB non gli
// registra un tipo.
func conFileSenzaTipo(th *fotorfq.Thread, id uuid.UUID, indice int16, nome, est, s string, f *fotorfq.Fatti) {
	conAllegato(th, allegato(id, indice, nome, est, s), f)
}

// TestL2DINodiConIlDisegnoDalFormato (L-2D): nella scena dei tre nodi, un PDF senza tipo con il cartiglio di #4 (nessun
// documento, nessuna proposta) è il 2D candidato del nodo #4, nei nodi della BOM di lavoro e nei previsti, con il perché
// dal_formato e l'associazione proposta; con un TIFF e un DXF dello stesso pezzo, senza tipo, il primario è il PDF (R84:
// la validità e il formato), poi il TIFF, poi il DXF (formato non configurato), qualunque sia l'ordine degli allegati.
// I 2D registrati restano tipo_registrato; i file dell'esito portano il perché, e lo STEP non è un 2D.
func TestL2DINodiConIlDisegnoDalFormato(t *testing.T) {
	m := motoreCatena(t)
	scena := func(t *testing.T, alContrario bool) fotorfq.Thread {
		th := scenaTreNodi(t, true)
		conFileSenzaTipo(&th, aL2DPDF, 41, "disegno4-acme.pdf", "pdf", shaL2DPDF, conCartiglio(t, shaL2DPDF, [2]string{"codice", "7120310A1"}))
		conFileSenzaTipo(&th, aL2DTIF, 42, "7120310A_1.tif", "tif", shaL2DTIF, nil)
		conFileSenzaTipo(&th, aL2DDXF, 43, "7120310A_1.dxf", "dxf", shaL2DDXF, nil)
		if alContrario {
			for i, j := 0, len(th.Allegati)-1; i < j; i, j = i+1, j-1 {
				th.Allegati[i], th.Allegati[j] = th.Allegati[j], th.Allegati[i]
			}
		}
		return th
	}
	var primo valutazione.GruppoDisegni2D
	for _, alContrario := range []bool{false, true} {
		v := valutaConVista(t, scena(t, alContrario), m)
		n := nodoBOMDi(t, prodotto(t, v, rifProdB), nodoB("#4"))
		g := n.Disegni
		if g.Primario == nil || g.Primario.AllegatoID == nil || *g.Primario.AllegatoID != aL2DPDF {
			t.Fatalf("il primario del nodo #4 è il PDF: %+v", g.Primario)
		}
		if g.Primario.Riconoscimento2D != valutazione.Riconoscimento2DDalFormato || g.Primario.Provenienza != ancoraggio.OrigineProposto ||
			g.Primario.Formato != valutazione.Formato2DPDF {
			t.Errorf("il PDF: %+v", g.Primario)
		}
		if n.Associazione == nil || *n.Associazione != ancoraggio.OrigineProposto {
			t.Errorf("l'associazione del nodo %+v", n.Associazione)
		}
		if len(g.Alternativi) != 2 || *g.Alternativi[0].AllegatoID != aL2DTIF || *g.Alternativi[1].AllegatoID != aL2DDXF ||
			g.Alternativi[0].Formato != valutazione.Formato2DTIFF || g.Alternativi[1].Formato != "" ||
			g.Alternativi[1].Validita != valutazione.ValiditaFormatoNonConfigurato {
			t.Fatalf("gli alternativi del nodo #4: %+v", g.Alternativi)
		}
		for _, x := range g.Alternativi {
			if x.Riconoscimento2D != valutazione.Riconoscimento2DDalFormato {
				t.Errorf("il perché del 2D %+v", x)
			}
		}
		if p := previstoDi(t, documentiDi(t, v, rifProdB), nodoB("#4"), "disegno_2d"); p.Disegni == nil || p.Disegni.Primario == nil ||
			*p.Disegni.Primario.AllegatoID != aL2DPDF {
			t.Errorf("il previsto del 2D di #4: %+v", p.Disegni)
		}
		if gs := gruppoDi(t, v, cSciolto); gs.Primario == nil || gs.Primario.Riconoscimento2D != valutazione.Riconoscimento2DTipoRegistrato {
			t.Errorf("il 2D confermato dello sciolto è registrato: %+v", gs.Primario)
		}
		if !alContrario {
			primo = g
		} else if !reflect.DeepEqual(primo, g) {
			t.Errorf("l'ordine degli allegati cambia il gruppo del nodo")
		}
	}

	et := esitoSmistamento(t, scena(t, false))
	visti := 0
	for _, f := range et.File {
		switch f.AllegatoID {
		case aL2DPDF, aL2DTIF, aL2DDXF:
			visti++
			if !f.Disegno || f.Riconoscimento2D != valutazione.Riconoscimento2DDalFormato {
				t.Errorf("il file %s dell'esito: disegno %v, perché %q", f.AllegatoID, f.Disegno, f.Riconoscimento2D)
			}
		case aPDF1, aPDF2:
			if !f.Disegno || f.Riconoscimento2D != valutazione.Riconoscimento2DTipoRegistrato {
				t.Errorf("il 2D registrato %s dell'esito: %v %q", f.AllegatoID, f.Disegno, f.Riconoscimento2D)
			}
		case aStepB:
			if f.Disegno || f.Riconoscimento2D != "" {
				t.Errorf("lo STEP non è un 2D: %v %q", f.Disegno, f.Riconoscimento2D)
			}
		}
	}
	if visti != 3 {
		t.Errorf("i file senza tipo dell'esito: %d", visti)
	}
}

// TestL2DIlProdottoSenzaStruttura (L-2D, componentiDelCandidato): senza STEP (nessuna struttura del target) un PDF senza
// tipo letto come il prodotto è un candidato a livello prodotto senza posizioni, e arriva al gruppo dei 2D del componente
// del target: la voce del 2D del prodotto è da verificare, con il file. Lo stesso PDF con il tipo «altro» registrato non
// è un 2D, e la voce resta «manca».
func TestL2DIlProdottoSenzaStruttura(t *testing.T) {
	m := motoreCatena(t)
	t.Run("senza tipo", func(t *testing.T) {
		th := scenaSenzaSTEP(t)
		conFileSenzaTipo(&th, aL2DProd, 44, "7120100A_1.pdf", "pdf", shaL2DPro, conCartiglio(t, shaL2DPro, [2]string{"codice", "7120100A1"}))
		v := valutaConVista(t, th, m)
		g := gruppoDi(t, v, cProdotto)
		if g.Primario == nil || g.Primario.AllegatoID == nil || *g.Primario.AllegatoID != aL2DProd ||
			g.Primario.Provenienza != ancoraggio.OrigineProposto || g.Primario.Riconoscimento2D != valutazione.Riconoscimento2DDalFormato {
			t.Fatalf("il 2D del prodotto senza struttura: %+v", g.Primario)
		}
		if x := voceDi(t, documentiDi(t, v, rifProdB), cProdotto, "disegno_2d"); esitoDi(x) != "da_verificare/associazione_non_confermata" ||
			x.FileCandidato == nil || *x.FileCandidato != aL2DProd {
			t.Errorf("la voce del 2D del prodotto %+v", x)
		}
	})
	t.Run("con un altro tipo registrato", func(t *testing.T) {
		th := scenaSenzaSTEP(t)
		conFileSenzaTipo(&th, aL2DProd, 44, "7120100A_1.pdf", "pdf", shaL2DPro, conCartiglio(t, shaL2DPro, [2]string{"codice", "7120100A1"}))
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pL2DAltro, AllegatoID: aL2DProd, Tipo: "altro", Fonte: "estensione", Stato: "aperta"})
		v := valutaConVista(t, th, m)
		for _, d := range v.Disegni {
			if d.ComponenteID == cProdotto {
				t.Errorf("un file «altro» non è un 2D del prodotto: %+v", d.Gruppo)
			}
		}
		if x := voceDi(t, documentiDi(t, v, rifProdB), cProdotto, "disegno_2d"); esitoDi(x) != "manca/nessun_documento" {
			t.Errorf("la voce del 2D del prodotto %+v", x)
		}
	})
}
