package distintabozza

// L1 — gli avvisi e i conflitti del Motore A in R4 formato 3 (riepilogo3_motore.go; EM §8.2, §9.4 con U2 §6.3.5): la
// condizione (f) di prima-istanza-1 con i conflitti del prodotto (e la verifica che puo' essere «si'»), gli errori del
// calcolo, PROPOSTA_CAMBIATA nei tre motivi, NON_INCLUSO_CONDIVISO, la firma. Dati sintetici.

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// avvisiMotoreDi: gli avvisi del motore della risposta, letti.
func avvisiMotoreDi(t *testing.T, r RiepilogoR4) []AvvisoMotoreR4 {
	t.Helper()
	var out []AvvisoMotoreR4
	for _, raw := range r.AvvisiMotore {
		var a AvvisoMotoreR4
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

// U2 §5.4 (f), R95 A [U]: i conflitti aperti del motore sul prodotto; senza conflitti e con le altre condizioni vere la
// verifica prevista e' «si'»; con un conflitto «no»; con il motore che non valuta il prodotto non e' mai «si'».
func TestR4ConflittiDelMotoreECondizioneF(t *testing.T) {
	f := nuovaFixR4(t)
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.arco("k-root", "k-a", 1)
	f.sorgenteValida(shaProva('e'))
	prepara := func(pv *valutazione.ProdottoValutato) func(in *ingressiR4) {
		return func(in *ingressiR4) {
			in.struttura = &db.GetStrutturaDellaSorgenteRow{Analizzata: true}
			in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{Prodotto: pv}}
		}
	}
	pv := &valutazione.ProdottoValutato{}
	r := f.calcolaCon(prepara(pv))
	if v := r.VerificaPrevista; len(r.Blocchi) != 0 || v.Stato != confermabom.StatoVerificaCalcolata || v.Verificata == nil || !*v.Verificata ||
		!v.ConflittiMotoreCalcolati || !r.Confermabile || len(r.ConflittiMotore) != 0 {
		t.Fatalf("verifica prevista «si'» con il motore senza conflitti: %+v (blocchi %v)", v, codici(r.Blocchi))
	}
	pv.BOM.Gerarchia.Conflitti = []string{"arco:a>b"}
	pv.Smistamento.Conflitti = []string{"file:x", "arco:a>b"}
	r2 := f.calcolaCon(prepara(pv))
	if v := r2.VerificaPrevista; v.Verificata == nil || *v.Verificata || !contiene(v.Motivi, confermabom.MotivoConflittiMotore) ||
		fmt.Sprint(r2.ConflittiMotore) != "[arco:a>b file:x]" || r2.FirmaAnteprima == r.FirmaAnteprima {
		t.Fatalf("con i conflitti: %+v %v", v, r2.ConflittiMotore)
	}
	// il motore che non valuta il prodotto: conflitti non calcolati, mai «si'»
	r3 := f.calcolaCon(prepara(nil))
	if v := r3.VerificaPrevista; v.Stato != confermabom.StatoVerificaNonCalcolata || v.ConflittiMotoreCalcolati || !r3.AvvisiMotoreCalcolati {
		t.Fatalf("prodotto non valutato: %+v", v)
	}
	// gli errori del calcolo: il codice, mai confermabile
	for _, c := range []struct {
		err    error
		codice string
	}{{ErrMotoreOccupato, MotoreErroreOccupato}, {ErrMotoreNonPronto, MotoreErroreOccupato}, {fmt.Errorf("x: %w", ErrMotoreScaduto), MotoreErroreScaduto},
		{errors.New("panic"), MotoreErroreCalcolo}} {
		r := f.calcolaCon(func(in *ingressiR4) { in.esitoMotore = &EsitoMotoreR4{Err: c.err} })
		if r.AvvisiMotoreCalcolati || r.AvvisiMotoreErrore == nil || r.AvvisiMotoreErrore.Codice != c.codice || r.Confermabile ||
			(r.VerificaPrevista.Verificata != nil && *r.VerificaPrevista.Verificata) || r.VerificaPrevista.ConflittiMotoreCalcolati {
			t.Fatalf("%v: %+v %+v", c.err, r.AvvisiMotoreErrore, r.VerificaPrevista)
		}
	}
}

// EM §8.2: PROPOSTA_CAMBIATA sul gruppo corrente della sorgente (contenuto, altra_occorrenza, gruppo_non_proposto) e
// NON_INCLUSO_CONDIVISO.
func TestR4AvvisiDelMotore(t *testing.T) {
	in, id := ingressiProva3R4(t, nil)
	bom := propostabom.Gruppo{Chiave: propostabom.GruppoBOM, Tipo: propostabom.GruppoBOM, AllegatoID: &id.allegato, Sha256: id.sha, Radice: id.rif("#1")}
	regUguale := *in.registro
	// stessa occorrenza, stesso contenuto: niente
	in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{Gruppi: []propostabom.Gruppo{bom},
		Importa: func(string) (importazione.Registro, error) { return regUguale, nil }}}
	r := calcolaIn(t, in)
	if !r.AvvisiMotoreCalcolati || len(r.AvvisiMotore) != 0 {
		t.Fatalf("proposta uguale: %v %s", r.AvvisiMotoreCalcolati, r.AvvisiMotore)
	}
	fUguale := r.FirmaAnteprima
	// stessa occorrenza, contenuto cambiato (il codice proposto del telaio): motivo contenuto, il telaio fra i nodi non toccati
	c := registroProva3(t, id)
	for i := range c.Proposta.Nodi {
		if c.Proposta.Nodi[i].Origine.Rif == id.rif("#7") {
			c.Proposta.Nodi[i].CodiceProposto = "ACM-1101"
		}
	}
	regNuovo, err := importazione.Componi(c)
	if err != nil {
		t.Fatal(err)
	}
	in, _ = ingressiProva3R4(t, nil)
	in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{Gruppi: []propostabom.Gruppo{bom},
		Importa: func(g string) (importazione.Registro, error) {
			if g != propostabom.GruppoBOM {
				return importazione.Registro{}, errors.New("gruppo inatteso " + g)
			}
			return regNuovo, nil
		}}}
	r = calcolaIn(t, in)
	aa := avvisiMotoreDi(t, r)
	if len(aa) != 1 || aa[0].Codice != AvvisoMotorePropostaCambiata || aa[0].Motivo != PropostaCambiataContenuto || aa[0].Differenze == nil ||
		aa[0].Differenze.Nodi == 0 || fmt.Sprint(aa[0].Nodi) != "[k-0007]" || aa[0].GruppoCorrente == nil || aa[0].GruppoCorrente.Come != propostabom.ComeStessaOccorrenza {
		t.Fatalf("PROPOSTA_CAMBIATA contenuto: %+v", aa)
	}
	if r.FirmaAnteprima == fUguale {
		t.Fatal("gli avvisi del motore sono nella firma")
	}
	// la BOM di lavoro su un'altra copia dello stesso file: altra_occorrenza
	copia := uuid.MustParse("00000000-0000-4000-8000-0000000c0999")
	altra := bom
	altra.AllegatoID = &copia
	in, _ = ingressiProva3R4(t, nil)
	in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{Gruppi: []propostabom.Gruppo{altra}}}
	aa = avvisiMotoreDi(t, calcolaIn(t, in))
	if len(aa) != 1 || aa[0].Motivo != PropostaCambiataAltraOccorrenza || aa[0].AllegatoID == nil || *aa[0].AllegatoID != copia {
		t.Fatalf("PROPOSTA_CAMBIATA altra_occorrenza: %+v", aa)
	}
	// nessun gruppo con la struttura: gruppo_non_proposto, con il motivo del gruppo corrente
	in, _ = ingressiProva3R4(t, nil)
	in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{}}
	aa = avvisiMotoreDi(t, calcolaIn(t, in))
	if len(aa) != 1 || aa[0].Motivo != PropostaCambiataGruppoNonProposto || aa[0].GruppoCorrente == nil || aa[0].GruppoCorrente.Motivo != propostabom.MotivoNessunGruppo {
		t.Fatalf("PROPOSTA_CAMBIATA gruppo_non_proposto: %+v", aa)
	}
	// la proposta di adesso che non si importa: gli avvisi del motore non sono calcolati (mai un riepilogo confermabile)
	in, _ = ingressiProva3R4(t, nil)
	in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{Gruppi: []propostabom.Gruppo{bom},
		Importa: func(string) (importazione.Registro, error) {
			return importazione.Registro{}, errors.New("non importabile")
		}}}
	if r = calcolaIn(t, in); r.AvvisiMotoreCalcolati || r.AvvisiMotoreErrore == nil || r.Confermabile {
		t.Fatalf("proposta non importabile: %+v", r.AvvisiMotoreErrore)
	}
	// NON_INCLUSO_CONDIVISO: lo STEP del cordone non incluso e' la struttura di un altro prodotto
	altroProdotto := idDi("altro-prodotto-motore")
	in, _ = ingressiProva3R4(t, nil)
	in.esitoMotore = &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{Gruppi: []propostabom.Gruppo{bom},
		Importa:     func(string) (importazione.Registro, error) { return regUguale, nil },
		AltriGruppi: map[uuid.UUID][]propostabom.Gruppo{altroProdotto: {{Chiave: "cand:x", Tipo: propostabom.GruppoCandidata, Sha256: id.sha}}}}}
	aa = avvisiMotoreDi(t, calcolaIn(t, in))
	if len(aa) != 1 || aa[0].Codice != AvvisoMotoreNonInclusoCondiviso || fmt.Sprint(aa[0].Prodotti) != fmt.Sprint([]uuid.UUID{altroProdotto}) ||
		len(aa[0].Chiavi) != 1 || aa[0].Chiavi[0] != id.rif("#31") {
		t.Fatalf("NON_INCLUSO_CONDIVISO: %+v", aa)
	}
}
