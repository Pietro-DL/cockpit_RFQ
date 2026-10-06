// L1 — gli aiuti delle prove della completezza documentale (B5, fase 3): i fabbisogni di default dello schema, le righe di
// v_fascicolo calcolate come le calcola la vista, la famiglia ACME della minuteria, le letture dell'esito, costruiti in Go
// come li darebbe il caricatore.
package valutazione_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example); i codici sono di fantasia (712xxxx, con il marcatore A
// o B nella famiglia acme-catena, senza marcatore nella famiglia acme-minuteria), gli UUID sono
// 00000000-0000-4000-8000-0000000000nn. Le prove citano i requisiti (R62, R72 D, R82, R99 A, R102 A, R103 C, K-01,
// T-E1R-01, T-E1R-03, PO-28, PO-33, PO-36), mai i casi degli attesi.

// Gli ID della completezza: un commerciale, un componente fuori dal perimetro, uno archiviato, uno manuale, le deroghe, i
// file da_determinare e DWG.
var (
	cCommerciale = uid(0x7c1)
	cFuori       = uid(0x7c2)
	cArchiviato  = uid(0x7c3)
	cManuale     = uid(0x7c4)
	cMinuteria   = uid(0x7c5)
	cComMinut    = uid(0x7c6)
	derogaA      = uid(0x7d1)
	derogaB      = uid(0x7d2)
	aDaDet       = uid(0x7e1)
	aDWG         = uid(0x7e2)
	dDWG         = uid(0x7e3)
	aNodo3       = uid(0x7e4)
	aPDF3        = uid(0x7e6)
	pDaDet       = uid(0x7e5)
	shaDaDet     = sha("d")
	shaDWG       = sha("e")
	shaNodo3     = sha("f")
)

// fabbisogniDefault: le otto righe di default dello schema (0001, ritoccate dalla 0018), come le dà ListFabbisognoEffettivo
// per un cliente senza righe proprie: il finito con cad_3d e disegno_2d bloccanti; sottoassieme e sciolto con il
// disegno_2d bloccante e cad_3d e sviluppo_dxf non bloccanti; nessuna riga per il commerciale.
func fabbisogniDefault() []fotorfq.RigaFabbisogno {
	r := func(tc, td string, b bool) fotorfq.RigaFabbisogno {
		return fotorfq.RigaFabbisogno{TipoComponente: tc, TipoDocumento: td, Bloccante: b}
	}
	return []fotorfq.RigaFabbisogno{r("finito", "cad_3d", true), r("finito", "disegno_2d", true),
		r("sciolto", "cad_3d", false), r("sciolto", "disegno_2d", true), r("sciolto", "sviluppo_dxf", false),
		r("sottoassieme", "cad_3d", false), r("sottoassieme", "disegno_2d", true), r("sottoassieme", "sviluppo_dxf", false)}
}

// regolaCliente: una riga di fabbisogno del cliente (Proprio).
func regolaCliente(tc, td string, b bool) fotorfq.RigaFabbisogno {
	return fotorfq.RigaFabbisogno{TipoComponente: tc, TipoDocumento: td, Bloccante: b, Proprio: true}
}

// conRegole: i fabbisogni effettivi con le righe del cliente al posto di quelle di default dei loro tipi (la
// sostituzione in blocco per tipo, come la fa ListFabbisognoEffettivo).
func conRegole(cliente ...fotorfq.RigaFabbisogno) []fotorfq.RigaFabbisogno {
	suoi := map[string]bool{}
	for _, r := range cliente {
		suoi[r.TipoComponente] = true
	}
	var out []fotorfq.RigaFabbisogno
	for _, r := range fabbisogniDefault() {
		if !suoi[r.TipoComponente] {
			out = append(out, r)
		}
	}
	return append(out, cliente...)
}

// vistaCome: le righe di v_fascicolo come le calcola la vista (0020), dai fabbisogni effettivi, dai componenti attivi, dai
// documenti correnti, dalle deroghe e dalle proposte aperte del thread. Serve alle prove sulla fotografia costruita in
// Go; la L4 di equivalenza confronta con la vista vera.
func vistaCome(th *fotorfq.Thread) {
	th.Fascicolo = nil
	for _, k := range th.Componenti {
		if k.ArchiviatoIl != nil {
			continue
		}
		for _, f := range th.Fabbisogni {
			if f.TipoComponente != k.Tipo {
				continue
			}
			r := fotorfq.RigaFascicolo{ComponenteID: k.ID, Codice: k.Codice, Rev: k.Rev, TipoComponente: k.Tipo, TipoDocumento: f.TipoDocumento,
				Bloccante: f.Bloccante, Esito: "manca"}
			var doc *fotorfq.DocumentoConfermato
			for i := range th.Documenti {
				d := &th.Documenti[i]
				if d.ComponenteID != nil && *d.ComponenteID == k.ID && d.Tipo == f.TipoDocumento && d.SostituitoDa == nil &&
					(doc == nil || d.ConfermatoIl.After(doc.ConfermatoIl)) {
					doc = d
				}
			}
			for _, d := range th.Deroghe {
				if d.ComponenteID == k.ID && d.Tipo == f.TipoDocumento {
					id := d.ID
					r.DerogaID = &id
				}
			}
			for _, p := range th.Proposte {
				if p.Stato == "aperta" && p.Tipo == f.TipoDocumento &&
					((p.ComponenteID != nil && *p.ComponenteID == k.ID) || (p.ComponenteID == nil && p.Codice != nil && strings.EqualFold(*p.Codice, k.Codice))) {
					if r.PropostaAperta == nil {
						id := p.ID
						r.PropostaAperta = &id
					}
					r.NProposteAperte++
				}
			}
			switch {
			case doc != nil:
				id, sn := doc.ID, doc.StatoNas
				r.DocumentoID, r.StatoNas, r.DocumentoRev, r.Esito = &id, &sn, doc.Rev, "ok"
				switch sn {
				case "in_coda":
					r.Esito = "ok_in_coda"
				case "errore":
					r.Esito = "ok_errore_nas"
				}
			case r.DerogaID != nil:
				r.Esito = "derogato"
			case r.PropostaAperta != nil:
				r.Esito = "da_confermare"
			}
			th.Fascicolo = append(th.Fascicolo, r)
		}
	}
}

// famigliaMinuteria: la famiglia ACME della minuteria: base 712 più quattro cifre, senza marcatore, con la categoria
// minuteria della famiglia (R7: un'annotazione della lettura, mai un riconoscitore sul nome). Accanto ad acme-catena non
// legge i codici con il marcatore (il confine dopo la base), e acme-catena non legge i suoi. È un meccanismo, mai il
// significato di un cliente.
func famigliaMinuteria() grammatica.FamigliaCodice {
	base := grammatica.Base{
		Segmenti:  []grammatica.SegmentoBase{{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}},
		Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
	return grammatica.FamigliaCodice{
		ID: "acme-minuteria", Namespace: "acme-minuteria",
		Ruoli:     []grammatica.Ruolo{grammatica.RuoloComponente},
		Categorie: []grammatica.Categoria{grammatica.CategoriaMinuteria},
		Base:      base,
		Forme: []grammatica.FormaCodice{formaCatena("codice", []string{"nome_file", "cartiglio.codice", "radice_step.id", "nodo_step.id"},
			parteC(grammatica.TipoParteBase))},
		Esempi: []grammatica.EsempioCodice{{ID: "e-minuteria", Origine: grammatica.OrigineSintetico, Selettore: "nodo_step.id", Testo: "7129001",
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "codice", Base: "7129001"}}}}},
	}
}

// ---- le letture dell'esito ----

// documentiDi: la completezza del prodotto.
func documentiDi(t *testing.T, v valutazione.ValutazioneProdotti, rif string) valutazione.CompletezzaDocumentale {
	t.Helper()
	return prodotto(t, v, rif).Documenti
}

// voceDi: la voce certa (componente, tipo di documento); se non c'è, la prova fallisce.
func voceDi(t *testing.T, d valutazione.CompletezzaDocumentale, comp uuid.UUID, tipo string) valutazione.VoceFabbisogno {
	t.Helper()
	for _, v := range d.Voci {
		if v.ComponenteID == comp && v.TipoDocumento == tipo {
			return v
		}
	}
	t.Fatalf("nessuna voce (%s, %s) fra %s", comp, tipo, vociDi(d))
	return valutazione.VoceFabbisogno{}
}

// haVoce: c'è una voce certa del componente (di qualunque tipo, con tipo vuoto).
func haVoce(d valutazione.CompletezzaDocumentale, comp uuid.UUID, tipo string) bool {
	for _, v := range d.Voci {
		if v.ComponenteID == comp && (tipo == "" || v.TipoDocumento == tipo) {
			return true
		}
	}
	return false
}

// vociDi: le voci come testo «componente/tipo=esito/motivo», per i messaggi.
func vociDi(d valutazione.CompletezzaDocumentale) string {
	var out []string
	for _, v := range d.Voci {
		out = append(out, v.ComponenteID.String()[30:]+"/"+v.TipoDocumento+"="+string(v.Esito)+"/"+string(v.Motivo))
	}
	return strings.Join(out, " ")
}

// esitoDi: l'esito e il motivo di una voce, per i confronti.
func esitoDi(v valutazione.VoceFabbisogno) string { return string(v.Esito) + "/" + string(v.Motivo) }

// statoDi: lo stato, il motivo, il perimetro e il suo motivo della completezza, per i confronti.
func statoDi(d valutazione.CompletezzaDocumentale) string {
	p := "aperto"
	if d.PerimetroChiuso {
		p = "chiuso"
	}
	return string(d.Stato) + "/" + d.Motivo + " " + p + "/" + d.MotivoPerimetro
}

// previstoDi: il fabbisogno previsto (nodo, tipo di documento); se non c'è, la prova fallisce.
func previstoDi(t *testing.T, d valutazione.CompletezzaDocumentale, nodo, tipo string) valutazione.FabbisognoPrevisto {
	t.Helper()
	for _, p := range d.Previsti {
		if p.Nodo == nodo && p.TipoDocumento == tipo {
			return p
		}
	}
	t.Fatalf("nessun previsto (%s, %s) fra %+v", nodo, tipo, d.Previsti)
	return valutazione.FabbisognoPrevisto{}
}

// nonBloccanteDi: il fabbisogno non bloccante (componente, tipo); ok falso se non c'è.
func nonBloccanteDi(d valutazione.CompletezzaDocumentale, comp uuid.UUID, tipo string) (valutazione.FabbisognoInformativo, bool) {
	for _, n := range d.NonBloccanti {
		if n.ComponenteID == comp && n.TipoDocumento == tipo {
			return n, true
		}
	}
	return valutazione.FabbisognoInformativo{}, false
}

// classificazioneDi: la classificazione del componente fra quelle del thread; se non c'è, la prova fallisce.
func classificazioneDi(t *testing.T, v valutazione.ValutazioneProdotti, comp uuid.UUID) valutazione.Classificazione {
	t.Helper()
	for _, c := range v.Classificazioni {
		if c.ComponenteID == comp {
			return c.Classificazione
		}
	}
	t.Fatalf("nessuna classificazione per %s", comp)
	return valutazione.Classificazione{}
}
