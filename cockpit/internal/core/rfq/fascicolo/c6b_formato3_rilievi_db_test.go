//go:build integrazione

// L4 — C-6b, corsia file, rilievi della revisione (direttiva 39 §2, §5 [U + T-39]): nelle RFQ formato 3 i gesti legacy
// non scrivono documento.sostituito_da ne' componente.step_strutturale_id su uno STEP del prodotto finito fuori da R5.
//   - «Sostituisci» con «no» (e con «sì») al riferimento, quando A e' lo STEP della working e B un secondo STEP del
//     prodotto: rifiuto con il rimando a S8 e R5, nessuna tabella cambia, A non e' sostituito_da prima di R5;
//   - «Annulla sostituzione» di una sostituzione A → B con B STEP strutturale (lo stato che R5 lascia dopo una revisione):
//     rifiuto, la sostituzione resta;
//   - la «Revoca» legacy dello STEP strutturale di un finito e il cambio di tipo che lo svuoterebbe: rifiuto, lo STEP
//     strutturale resta;
//   - quello che resta: la sostituzione e il suo annullamento fra due disegni 2D, la revoca di un sottoassieme.
// La scena e' quella della sonda della revisione: il prodotto 7120001 con lo STEP A autorizzato in legacy (prima che la
// RFQ diventasse formato 3), il secondo STEP B, poi la bozza formato 3 (rfq_e_formato3 = true). Solo dati sintetici.

package fascicolo_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// scenaRilievi: il prodotto con lo STEP A autorizzato in legacy (lo STEP strutturale) e il secondo STEP B; formato3 =
// la bozza formato 3 creata dopo.
func (b *banco) scenaRilievi(formato3 bool) (prodotto, docA, docB uuid.UUID) {
	b.t.Helper()
	prodotto, docA, _ = b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: docA})
	docB, _ = b.stepDelProdotto(prodotto, "7120001", fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120012"},
		archi: []string{"#1>#2", "#1>#3*2"}})
	if got := uno[bool](b, `SELECT step_strutturale_id = $2 FROM componente WHERE componente_id = $1`, prodotto, docA); !got {
		b.t.Fatal("la scena: A e' lo STEP strutturale del prodotto")
	}
	if formato3 {
		b.rendiFormato3(prodotto)
	}
	return prodotto, docA, docB
}

func (b *banco) sostituisci(vecchio, nuovo uuid.UUID, si bool) (string, error) {
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.Sostituisci(b.ctx, q, b.thread, vecchio, nuovo, si, b.utente)
	})
}

func (b *banco) annullaSostituzione(vecchio uuid.UUID) (string, error) {
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AnnullaSostituzione(b.ctx, q, b.thread, vecchio)
	})
}

// rifiutoF3: il rifiuto del formato 3 con il rimando atteso, e nessuna tabella cambiata dal gesto.
func (b *banco) rifiutoF3(nome string, prima map[string]string, err error, frasi ...string) {
	b.t.Helper()
	if err == nil {
		b.t.Fatalf("%s nel formato 3: riuscito, atteso il rifiuto", nome)
	}
	for _, f := range append([]string{"formato 3", "Niente è cambiato"}, frasi...) {
		if !strings.Contains(err.Error(), f) {
			b.t.Errorf("%s nel formato 3: %q senza %q", nome, err, f)
		}
	}
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		b.t.Errorf("%s: il rifiuto ha scritto in %s", nome, d)
	}
}

// La revisione nel formato 3 (direttiva 39 §2, §8 «A non e' sostituito_da prima di R5»): «Sostituisci A con B» con «no»
// (la strada che non passava da DichiaraStrutturale) e con «sì» si rifiutano; anche B → A. Una sostituzione fra due
// disegni 2D del prodotto resta, e si annulla.
func TestC6bRilieviSostituisciLoStepDelFinitoNelFormato3(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, docA, docB := b.scenaRilievi(true)
	d2a := b.documento(prodotto, db.TipoDocumentoDisegno2d, "7120001", "pdf")
	d2b := b.documento(prodotto, db.TipoDocumentoDisegno2d, "7120001", "pdf")

	prima := b.fotoTabelle()
	_, err := b.sostituisci(docA, docB, false)
	b.rifiutoF3("Sostituisci A con B, «no»", prima, err, "non si sostituisce da qui", "S8", "conferma della BOM")
	_, err = b.sostituisci(docA, docB, true)
	b.rifiutoF3("Sostituisci A con B, «sì»", prima, err, "non si sostituisce da qui")
	_, err = b.sostituisci(docB, docA, false)
	b.rifiutoF3("Sostituisci B con A", prima, err, "non si sostituisce da qui")
	if got := uno[bool](b, `SELECT sostituito_da IS NULL FROM documento WHERE documento_id = $1`, docA); !got {
		t.Error("A sostituito_da prima di R5")
	}
	if got := uno[bool](b, `SELECT step_strutturale_id = $2 FROM componente WHERE componente_id = $1`, prodotto, docA); !got {
		t.Error("lo STEP strutturale non e' piu' A")
	}

	// quello che resta: due disegni 2D (non sono STEP del finito)
	msg, err := b.sostituisci(d2a, d2b, false)
	ok(t, err)
	if !strings.Contains(msg, "sostituito da") {
		t.Errorf("la sostituzione fra due 2D: %q", msg)
	}
	if _, err := b.annullaSostituzione(d2a); err != nil {
		t.Fatalf("l'annullamento fra due 2D: %v", err)
	}
}

// «Annulla sostituzione» nel formato 3: la sostituzione A → B con B STEP strutturale (scritta qui dalla pagina legacy
// quando la RFQ era interamente legacy: lo stesso stato del documento e del componente che R5 lascia dopo una revisione)
// non si disfa da qui. Nella RFQ ancora legacy lo stesso annullamento riesce (la prova di sempre, su un secondo banco).
func TestC6bRilieviAnnullaSostituzioneDelloStepNelFormato3(t *testing.T) {
	scena := func(t *testing.T, formato3 bool) (*banco, uuid.UUID, uuid.UUID, uuid.UUID) {
		b := nuovoBanco(t)
		prodotto, docA, docB := b.scenaRilievi(false)
		if _, err := b.sostituisci(docA, docB, true); err != nil {
			t.Fatalf("la sostituzione legacy con «sì»: %v", err)
		}
		if got := uno[bool](b, `SELECT d.sostituito_da = $2 AND c.step_strutturale_id = $2 FROM documento d, componente c
			WHERE d.documento_id = $1 AND c.componente_id = $3`, docA, docB, prodotto); !got {
			t.Fatal("la scena: A sostituito da B, B STEP strutturale")
		}
		if formato3 {
			b.rendiFormato3(prodotto)
		}
		return b, prodotto, docA, docB
	}

	b, prodotto, docA, docB := scena(t, true)
	prima := b.fotoTabelle()
	_, err := b.annullaSostituzione(docA)
	b.rifiutoF3("Annulla sostituzione", prima, err, "non si annulla da qui", "Revoca come sorgente")
	if got := uno[bool](b, `SELECT d.sostituito_da = $2 AND c.step_strutturale_id = $2 FROM documento d, componente c
		WHERE d.documento_id = $1 AND c.componente_id = $3`, docA, docB, prodotto); !got {
		t.Error("la sostituzione o lo STEP strutturale sono cambiati")
	}

	t.Run("RFQ legacy", func(t *testing.T) {
		b, _, docA, _ := scena(t, false)
		if _, err := b.annullaSostituzione(docA); err != nil {
			t.Fatalf("l'annullamento nella RFQ legacy: %v", err)
		}
	})
}

// La «Revoca» legacy dello STEP strutturale di un finito nel formato 3 (la sonda della revisione: prima riusciva e
// svuotava step_strutturale_id) e il cambio di tipo del finito a commerciale, che lo svuoterebbe: rifiuto, niente
// cambia. Il sottoassieme si revoca ancora (non scrive step_strutturale_id). Nella RFQ legacy la revoca del finito resta
// (le prove di sempre, TestLaRevoca* e le altre di autorizzazione_db_test.go).
func TestC6bRilieviRevocaECambioDiTipoNelFormato3(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, docA, _ := b.scenaRilievi(true)
	// il sottoassieme autorizzato (anche nel formato 3, come in TestC6bAutorizzaNelFormato3RimandaAS8)
	s := b.componente("7120010", db.TipoComponenteSottoassieme)
	sdoc, sa := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}})
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: sdoc})

	prima := b.fotoTabelle()
	_, err := b.revoca(prodotto, "")
	b.rifiutoF3("Revoca legacy del finito", prima, err, "non si revoca da qui", "Revoca come sorgente")
	_, err = b.revoca(prodotto, b.sha(docA))
	b.rifiutoF3("Revoca legacy del finito, un file", prima, err, "non si revoca da qui")
	_, err = b.cambiaTipo(prodotto, db.TipoComponenteCommerciale)
	b.rifiutoF3("cambio di tipo a commerciale", prima, err, "il cambio di tipo lo svuoterebbe")
	if got := uno[bool](b, `SELECT step_strutturale_id = $2 AND tipo = 'finito' FROM componente WHERE componente_id = $1`, prodotto, docA); !got {
		t.Error("lo STEP strutturale o il tipo del finito sono cambiati")
	}

	// il sottoassieme: la revoca resta
	if _, err := b.revoca(s, sa.Sha256.String); err != nil {
		t.Fatalf("la revoca del sottoassieme nel formato 3: %v", err)
	}
}
