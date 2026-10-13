package confermabom

// L1 — A1d P4, B2.3 parte pura: la validita' dell'ultima conferma (contratto rev. 3 §5.1, «Parole», R-18) e il
// controllo della base (§5.1 «Controllo sotto lucchetto», motivi conferma_cambiata e struttura_cambiata di
// BASE_SUPERATA, §4.4 n. 1). Le prove con il database (TestConfermaValidaPerContatoreOPerFirma,
// TestContatorePerRFQAltroProdotto, Δ44 con i trigger) sono della parte B2.3 con le query.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
)

func sha(c byte) string { return strings.Repeat(string(c), 64) }

func conferme() []Conferma {
	return []Conferma{
		{ID: id(0x31), Numero: 1, Contatore: 4, FirmaStruttura: sha('1')},
		{ID: id(0x33), Numero: 3, Contatore: 9, FirmaStruttura: sha('3')},
		{ID: id(0x32), Numero: 2, Contatore: 7, FirmaStruttura: sha('2')},
	}
}

// L'ultima e' quella col numero piu' alto, in qualunque ordine arrivino.
func TestUltimaConferma(t *testing.T) {
	u, ok, err := Ultima(conferme())
	if err != nil || !ok || u.ID != id(0x33) {
		t.Fatalf("ultima: %+v %v %v", u, ok, err)
	}
	if _, ok, err := Ultima(nil); ok || err != nil {
		t.Errorf("senza conferme: %v %v", ok, err)
	}
	doppia := append(conferme(), Conferma{ID: id(0x34), Numero: 3, Contatore: 1, FirmaStruttura: sha('4')})
	if _, _, err := Ultima(doppia); !errors.Is(err, ErrIngresso) {
		t.Errorf("due conferme con lo stesso numero: %v", err)
	}
}

// Valida = ultima e (contatore uguale oppure firma uguale). Il contatore e' per RFQ: una scrittura in un altro
// prodotto lo muove, e allora decide la firma. Una conferma che non e' l'ultima non vale, anche con tutto uguale.
func TestConfermaValidaUltimaEContatoreOFirma(t *testing.T) {
	cs := conferme()
	ultima, vecchia := cs[1], cs[2]
	casi := []struct {
		nome   string
		c      Conferma
		w      Working
		attesa Validita
	}{
		{"contatore e firma uguali", ultima, Working{Contatore: 9, Firma: sha('3')}, Validita{Valida: true, Ultima: true, PerContatore: true, PerFirma: true}},
		{"solo il contatore (firma non calcolata)", ultima, Working{Contatore: 9}, Validita{Valida: true, Ultima: true, PerContatore: true}},
		{"contatore mosso da un altro prodotto, firma uguale", ultima, Working{Contatore: 12, Firma: sha('3')}, Validita{Valida: true, Ultima: true, PerFirma: true}},
		{"contatore e firma diversi", ultima, Working{Contatore: 12, Firma: sha('e')}, Validita{Ultima: true}},
		{"contatore diverso, firma non calcolata", ultima, Working{Contatore: 12}, Validita{Ultima: true}},
		{"non e' l'ultima, tutto uguale", vecchia, Working{Contatore: 7, Firma: sha('2')}, Validita{PerContatore: true, PerFirma: true}},
	}
	for _, c := range casi {
		got, err := ConfermaValida(c.c, cs, c.w)
		if err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		if !reflect.DeepEqual(got, c.attesa) {
			t.Errorf("%s: %+v, attesa %+v", c.nome, got, c.attesa)
		}
	}
	u, v, ok, err := UltimaValida(cs, Working{Contatore: 12, Firma: sha('3')})
	if err != nil || !ok || u.ID != ultima.ID || !v.Valida || !v.PerFirma {
		t.Errorf("UltimaValida: %+v %+v %v %v", u, v, ok, err)
	}
	if _, _, ok, err := UltimaValida(nil, Working{}); ok || err != nil {
		t.Errorf("UltimaValida senza conferme: %v %v", ok, err)
	}
}

// Ingressi impossibili: la conferma non e' fra quelle del prodotto, o vi e' diversa; firme malformate; contatori
// negativi.
func TestConfermaValidaIngressiNonValidi(t *testing.T) {
	cs := conferme()
	w := Working{Contatore: 9, Firma: sha('3')}
	estranea := Conferma{ID: id(0x40), Numero: 4, Contatore: 9, FirmaStruttura: sha('3')}
	diversa := cs[1]
	diversa.Contatore = 1
	casi := map[string]func() error{
		"non e' del prodotto": func() error { _, err := ConfermaValida(estranea, cs, w); return err },
		"vi e' diversa":       func() error { _, err := ConfermaValida(diversa, cs, w); return err },
		"firma maiuscola": func() error {
			c := cs[1]
			c.FirmaStruttura = strings.ToUpper(sha('a'))
			_, err := ConfermaValida(c, []Conferma{c}, w)
			return err
		},
		"firma della working corta": func() error { _, err := ConfermaValida(cs[1], cs, Working{Firma: "abc"}); return err },
		"contatore negativo":        func() error { _, err := ConfermaValida(cs[1], cs, Working{Contatore: -1}); return err },
		"senza conferme":            func() error { _, err := ConfermaValida(cs[1], nil, w); return err },
	}
	for nome, f := range casi {
		if err := f(); !errors.Is(err, ErrIngresso) {
			t.Errorf("%s: atteso ErrIngresso, ottenuto %v", nome, err)
		}
	}
}

// Il controllo della base: conferma_cambiata se l'ultima non e' quella della base; struttura_cambiata se contatore e
// firma sono entrambi diversi dalla base. Nessun motivo = la base vale.
func TestControllaBase(t *testing.T) {
	cs := conferme()
	ultima := cs[1].ID
	vecchia := cs[2].ID
	casi := []struct {
		nome   string
		b      Base
		cs     []Conferma
		w      Working
		motivi []string
	}{
		{"base corrente", Base{ConfermaID: &ultima, Numero: 3, Contatore: 9, Firma: sha('3')}, cs, Working{Contatore: 9, Firma: sha('3')}, nil},
		{"contatore mosso altrove, firma uguale", Base{ConfermaID: &ultima, Numero: 3, Contatore: 9, Firma: sha('3')}, cs, Working{Contatore: 11, Firma: sha('3')}, nil},
		{"struttura cambiata", Base{ConfermaID: &ultima, Numero: 3, Contatore: 9, Firma: sha('3')}, cs, Working{Contatore: 11, Firma: sha('e')}, []string{MotivoStrutturaCambiata}},
		{"un'altra conferma nel frattempo", Base{ConfermaID: &vecchia, Numero: 2, Contatore: 7, Firma: sha('2')}, cs, Working{Contatore: 9, Firma: sha('3')}, []string{MotivoConfermaCambiata, MotivoStrutturaCambiata}},
		{"nessuna conferma nella base, nessuna adesso", Base{Contatore: 5, Firma: sha('5')}, nil, Working{Contatore: 5, Firma: sha('5')}, nil},
		{"nessuna conferma nella base, una adesso", Base{Contatore: 5, Firma: sha('5')}, cs[:1], Working{Contatore: 5, Firma: sha('5')}, []string{MotivoConfermaCambiata}},
		{"stesso ID, numero diverso", Base{ConfermaID: &ultima, Numero: 2, Contatore: 9, Firma: sha('3')}, cs, Working{Contatore: 9, Firma: sha('3')}, []string{MotivoConfermaCambiata}},
		// revisione della corsia D, B2: la base cita una conferma e il prodotto adesso non ne ha (la conferma di un
		// altro prodotto, o una conferma che non c'e' piu')
		{"una conferma nella base, nessuna adesso", Base{ConfermaID: &ultima, Numero: 3, Contatore: 9, Firma: sha('3')}, nil, Working{Contatore: 9, Firma: sha('3')}, []string{MotivoConfermaCambiata}},
	}
	for _, c := range casi {
		got, err := ControllaBase(c.b, c.cs, c.w)
		if err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		if !reflect.DeepEqual(got, c.motivi) {
			t.Errorf("%s: motivi %v, attesi %v", c.nome, got, c.motivi)
		}
	}
	x := uuid.New()
	for nome, b := range map[string]Base{
		"conferma senza numero":   {ConfermaID: &x, Numero: 0, Firma: sha('1')},
		"numero senza conferma":   {Numero: 2, Firma: sha('1')},
		"firma della base vuota":  {},
		"contatore della base -1": {Contatore: -1, Firma: sha('1')},
	} {
		if _, err := ControllaBase(b, cs, Working{Firma: sha('1')}); !errors.Is(err, ErrIngresso) {
			t.Errorf("%s: atteso ErrIngresso, ottenuto %v", nome, err)
		}
	}
	// revisione della corsia D, B3: sotto lucchetto la firma della working e' obbligatoria, anche con il contatore
	// uguale
	base := Base{ConfermaID: &ultima, Numero: 3, Contatore: 9, Firma: sha('3')}
	for _, w := range []Working{{Contatore: 11}, {Contatore: 9}} {
		if _, err := ControllaBase(base, cs, w); !errors.Is(err, ErrIngresso) {
			t.Errorf("firma della working non calcolata (contatore %d): atteso ErrIngresso, ottenuto %v", w.Contatore, err)
		}
	}
}

// La firma del perimetro e la validita' insieme: una conferma registrata con la firma del perimetro resta valida per
// firma quando la working torna uguale (Δ44), anche se il contatore e' cresciuto.
func TestValidaPerFirmaDopoArcoAggiuntoETolto(t *testing.T) {
	s := nuovaScena()
	registrata := Conferma{ID: id(0x51), Numero: 1, Contatore: 4, FirmaStruttura: s.firma(t)}
	s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(pezzo12), Qta: 1})
	v, err := ConfermaValida(registrata, []Conferma{registrata}, Working{Contatore: 5, Firma: s.firma(t)})
	if err != nil || v.Valida {
		t.Fatalf("con l'arco in piu': %+v %v", v, err)
	}
	s.archi = s.archi[:len(s.archi)-1]
	v, err = ConfermaValida(registrata, []Conferma{registrata}, Working{Contatore: 6, Firma: s.firma(t)})
	if err != nil || !v.Valida || v.PerContatore || !v.PerFirma {
		t.Errorf("tolto l'arco: %+v %v, attesa valida per firma", v, err)
	}
}

// Scenario F (direttiva 12 §10; D-FIRMA-01 = b [U]). La conferma n. 1 e' registrata con la firma del perimetro.
//   - Cambiano solo le regole di canonizzazione del cliente (ACME smette di togliere «_PRT»): il codice canonico
//     di 7120010_prt cambia, la firma no. Il contatore si muove per una scrittura su un altro prodotto della RFQ:
//     la conferma resta valida, per firma. Con la firma di prima (il canonico dentro) sarebbe decaduta qui.
//   - Cambia davvero un componente (il codice letterale, il tipo) o un arco (la qta, un arco in piu') del perimetro:
//     la firma cambia, il contatore si e' mosso, e la conferma non vale piu' (ultima e, contatore oppure firma).
func TestScenarioFValiditaDopoUnCambioDiRegoleODiStruttura(t *testing.T) {
	conRegole := classificazione.Compila("ACME", regole.Regole{SuffissiDecorativi: []string{"_PRT"}})
	senzaRegole := classificazione.Compila("ACME", regole.Regole{})
	canonico := func(m *classificazione.Motore, codice string) string {
		k, _ := m.Canonico(codice, "")
		return strings.ToUpper(k)
	}
	if a, b := canonico(conRegole, "7120010_prt"), canonico(senzaRegole, "7120010_prt"); a == b {
		t.Fatalf("la scena vuole due letture diverse del cliente: %q, %q", a, b)
	}
	s := nuovaScena()
	registrata := Conferma{ID: id(0x61), Numero: 1, Contatore: 4, FirmaStruttura: s.firma(t)}
	cs := []Conferma{registrata}

	// le regole cambiano, la working no; il contatore si muove per un altro prodotto (7120002 → 7120099 qta 3)
	s.a(altro, fuori).Qta = 3
	v, err := ConfermaValida(registrata, cs, Working{Contatore: 5, Firma: s.firma(t)})
	if err != nil || !v.Valida || v.PerContatore || !v.PerFirma {
		t.Errorf("cambio delle sole regole del cliente: %+v %v, attesa valida per firma", v, err)
	}
	if motivi, err := ControllaBase(Base{ConfermaID: &registrata.ID, Numero: 1, Contatore: 4, Firma: registrata.FirmaStruttura}, cs,
		Working{Contatore: 5, Firma: s.firma(t)}); err != nil || len(motivi) != 0 {
		t.Errorf("la base non e' superata da un cambio di regole: %v %v", motivi, err)
	}

	// un cambio vero del perimetro: la firma cambia e la conferma decade
	for _, c := range []struct {
		nome   string
		cambia func(s *scena)
	}{
		{"codice letterale di un componente", func(s *scena) { s.c(assieme).CodiceLetterale = "7120010" }},
		{"tipo di un componente", func(s *scena) { s.c(pezzo11).Tipo = "commerciale" }},
		{"qta di un arco", func(s *scena) { s.a(assieme, pezzo11).Qta = 5 }},
		{"arco in piu'", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(pezzo12), Qta: 1}) }},
	} {
		s := nuovaScena()
		c.cambia(s)
		v, err := ConfermaValida(registrata, cs, Working{Contatore: 6, Firma: s.firma(t)})
		if err != nil || v.Valida || v.PerFirma {
			t.Errorf("%s: %+v %v, attesa non valida", c.nome, v, err)
		}
		motivi, err := ControllaBase(Base{ConfermaID: &registrata.ID, Numero: 1, Contatore: 4, Firma: registrata.FirmaStruttura}, cs,
			Working{Contatore: 6, Firma: s.firma(t)})
		if err != nil || !reflect.DeepEqual(motivi, []string{MotivoStrutturaCambiata}) {
			t.Errorf("%s: motivi %v %v, atteso struttura_cambiata", c.nome, motivi, err)
		}
	}
}
