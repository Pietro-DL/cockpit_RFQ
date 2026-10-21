package confermabom

// L1 — prima-istanza-1 (U2 §5.4): le sei condizioni, i motivi nell'ordine, «non calcolata» con i blocchi aperti o con i
// conflitti del motore non calcolati, la deroga di struttura, la colonna salvata (solo la regola).

import (
	"encoding/json"
	"reflect"
	"testing"
)

func ingressiBuoni() IngressiPrimaIstanza {
	return IngressiPrimaIstanza{Criterio: CriterioStep, FigliDellaRadice: 3, FonteConfermata: true, FonteAutorizzata: true,
		SorgenteUgualeAllaFonte: true, ConflittiCalcolati: true}
}

func TestPrimaIstanzaVera(t *testing.T) {
	v := PrimaIstanza(ingressiBuoni())
	if v.Stato != StatoVerificaCalcolata || v.Verificata == nil || !*v.Verificata || len(v.Motivi) != 0 {
		t.Fatalf("attesa vera: %+v", v)
	}
	if v.OrigineGerarchia != OrigineGerarchiaStep || !v.GerarchiaStep || v.Regola != RegolaPrimaIstanza {
		t.Fatalf("gerarchia e regola: %+v", v)
	}
	mp := IngressiPrimaIstanza{Criterio: CriterioMonoPezzo, MotivoMonoPezzo: "pezzo unico", FonteConfermata: true, FonteAutorizzata: true,
		SorgenteUgualeAllaFonte: true, ConflittiCalcolati: true}
	if v := PrimaIstanza(mp); v.Verificata == nil || !*v.Verificata {
		t.Fatalf("mono_pezzo con motivo e senza figli: %+v", v)
	}
}

func TestPrimaIstanzaICasiNo(t *testing.T) {
	casi := []struct {
		nome   string
		muta   func(*IngressiPrimaIstanza)
		motivi []string
	}{
		{"senza criterio", func(i *IngressiPrimaIstanza) { i.Criterio = "" }, []string{MotivoCriterioAssente}},
		{"step senza figli", func(i *IngressiPrimaIstanza) { i.FigliDellaRadice = 0 }, []string{MotivoStepSenzaFigli}},
		{"mono_pezzo con figli e senza motivo", func(i *IngressiPrimaIstanza) { i.Criterio = CriterioMonoPezzo },
			[]string{MotivoMonoPezzoConFigli, MotivoMonoPezzoSenzaMotivo}},
		{"fonte non autorizzata", func(i *IngressiPrimaIstanza) { i.FonteAutorizzata = false }, []string{MotivoFonteNonAutorizzata}},
		{"fonte sostituita e non confermata", func(i *IngressiPrimaIstanza) { i.FonteSostituita = true; i.FonteConfermata = false },
			[]string{MotivoFonteNonConfermata, MotivoFonteSostituita}},
		{"sorgente diversa", func(i *IngressiPrimaIstanza) { i.SorgenteUgualeAllaFonte = false }, []string{MotivoSorgenteDiversa}},
		{"righe e rimozioni", func(i *IngressiPrimaIstanza) { i.RigheDaDecidere = 2; i.RimozioniAperte = 1 },
			[]string{MotivoRigheDaDecidere, MotivoRimozioniAperte}},
		{"gerarchia non ricostruibile", func(i *IngressiPrimaIstanza) { i.MotivoParzialeStruttura = "relazioni_incomplete" },
			[]string{MotivoGerarchiaNonRicostruibile}},
		{"secondari aperti", func(i *IngressiPrimaIstanza) { i.RigheSecondarieAperte = 1 }, []string{MotivoSecondariAperti}},
		{"conflitti aperti", func(i *IngressiPrimaIstanza) { i.ConflittiAperti = 1 }, []string{MotivoConflittiMotore}},
		{"conflitti non calcolati con un altro no", func(i *IngressiPrimaIstanza) { i.ConflittiCalcolati = false; i.FonteAutorizzata = false },
			[]string{MotivoFonteNonAutorizzata, MotivoConflittiNonCalcolati}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			in := ingressiBuoni()
			c.muta(&in)
			v := PrimaIstanza(in)
			if v.Stato != StatoVerificaCalcolata || v.Verificata == nil || *v.Verificata || !reflect.DeepEqual(v.Motivi, c.motivi) {
				t.Fatalf("atteso no con %v: %+v", c.motivi, v)
			}
		})
	}
}

func TestPrimaIstanzaNonCalcolata(t *testing.T) {
	in := ingressiBuoni()
	in.ConflittiCalcolati = false
	v := PrimaIstanza(in)
	if v.Stato != StatoVerificaNonCalcolata || v.Verificata != nil || !reflect.DeepEqual(v.Motivi, []string{MotivoConflittiNonCalcolati}) {
		t.Fatalf("conflitti non calcolati: %+v", v)
	}
	in = ingressiBuoni()
	in.BlocchiAperti = true
	in.FonteAutorizzata = false
	v = PrimaIstanza(in)
	if v.Stato != StatoVerificaNonCalcolata || v.Verificata != nil || !reflect.DeepEqual(v.Motivi, []string{MotivoBlocchiAperti}) {
		t.Fatalf("blocchi aperti: %+v", v)
	}
	// l'esito non calcolato non si serializza con un «verificata»
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["verificata"]; ok {
		t.Fatalf("verificata nel JSON di una verifica non calcolata: %s", b)
	}
}

func TestPrimaIstanzaConDeroga(t *testing.T) {
	in := ingressiBuoni()
	in.MotivoParzialeStruttura = "relazioni_incomplete"
	in.Deroga = &DerogaStruttura{Stato: "valida", Motivo: "gerarchia dal PDF dell'elenco particolari"}
	v := PrimaIstanza(in)
	if v.Verificata == nil || !*v.Verificata || v.OrigineGerarchia != OrigineGerarchiaDeroga || v.GerarchiaStep || v.Deroga == nil {
		t.Fatalf("con deroga: %+v", v)
	}
}

func TestColonnaVerificaFormato3(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(ColonnaVerificaFormato3(), &m); err != nil || len(m) != 1 || m["regola"] != RegolaPrimaIstanza {
		t.Fatalf("colonna: %s %v", ColonnaVerificaFormato3(), err)
	}
}
