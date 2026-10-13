// L1 — la verifica della BOM dall'ultima conferma strutturale (P4, passi B3.2, B3.3, B3.4; direttiva 11 D-SHOCK-S1 B,
// D-SHOCK-S4 A con SHOCK-05, D-P4A-02 A, D-P4B1-02 A; contratto rev. 3 §8.3, §11.1 n. 3; sintesi ABC §B.3). La regola
// è pura: ingressi sintetici, scena ACME con codici inventati (finito ACM-9000, figli ACM-9000-01…13), nessun DB. Le
// prove della regola di A1c e dell'adattatore legacy (bom_test.go) restano com'erano.
package valutazione_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// ---- la scena sintetica ----

var (
	shaFonteACM  = sha("a")
	docFonteACM  = uid(0x9c01)
	docAltroACM  = uid(0x9c02)
	finitoACM    = uid(0x9c10)
	confermaACM  = uid(0x9c03)
	derogaACM    = uid(0x9c04)
	radiceStepAC = ancoraggio.RifNodo(shaFonteACM, "#1")
	analisiACM   = valutazione.ChiaveAnalisi{VersioneAnalizzatore: 3, HashConfigurazione: sha("c"), CalcolataIl: "2026-10-12T20:00:00Z"}
	// le qta dei 13 figli della ricostruzione manuale: inventate, con qualche 2 e un 4
	qtaACM = []int{1, 1, 2, 1, 1, 1, 4, 1, 1, 2, 1, 1, 1}
)

func figlioACM(i int) uuid.UUID { return uid(0x9c20 + i) }

func rifC(id uuid.UUID) string { return ancoraggio.RifComponente(id) }

// radiceACM: la riga radice della conferma, il finito con lo STEP della fonte (o senza, con step falso).
func radiceACM(step bool) valutazione.ComponenteConfermato {
	r := valutazione.ComponenteConfermato{ComponenteID: finitoACM, Codice: "ACM-9000", Tipo: "finito", Radice: true}
	if step {
		d := docFonteACM
		r.StepStrutturaleID, r.StepSha256 = &d, shaFonteACM
	}
	return r
}

// confermaManualeACM: la conferma n. 1 del finito con i figli e le qta date, archi decisi a mano (origine utente),
// criterio step (la radice certificata ha lo STEP autorizzato).
func confermaManualeACM(qta []int) *valutazione.ConfermaStrutturale {
	c := &valutazione.ConfermaStrutturale{ConfermaID: confermaACM, Numero: 1, Valida: true, ConfermatoDa: operatore,
		ConfermatoIl: "2026-10-12T21:00:00Z", Componenti: []valutazione.ComponenteConfermato{radiceACM(true)}, Criterio: valutazione.CriterioStep}
	for i, q := range qta {
		c.Componenti = append(c.Componenti, valutazione.ComponenteConfermato{ComponenteID: figlioACM(i), Codice: fmt.Sprintf("ACM-9000-%02d", i+1), Tipo: "sciolto"})
		c.Archi = append(c.Archi, valutazione.ArcoConfermato{PadreID: finitoACM, FiglioID: figlioACM(i), Qta: q, Origine: "utente"})
	}
	return c
}

// monoACM: la conferma di un finito pezzo unico, con la dichiarazione motivata.
func monoACM() *valutazione.ConfermaStrutturale {
	return &valutazione.ConfermaStrutturale{ConfermaID: confermaACM, Numero: 1, Valida: true, ConfermatoDa: operatore, ConfermatoIl: "2026-10-12T21:00:00Z",
		Componenti: []valutazione.ComponenteConfermato{radiceACM(true)}, MonoPezzo: true, MonoPezzoMotivo: "un solo pezzo tornito, nessun assieme",
		Criterio: valutazione.CriterioMonoPezzo}
}

func correntiDi(c *valutazione.ConfermaStrutturale) []valutazione.ArcoCorrente {
	var out []valutazione.ArcoCorrente
	for _, a := range c.Archi {
		out = append(out, valutazione.ArcoCorrente{PadreID: a.PadreID, FiglioID: a.FiglioID, Qta: a.Qta})
	}
	return out
}

// strutturaSoloPartiACM: la BOM di lavoro di uno STEP «solo parti»: la sola radice, nessun arco (con la fonte confermata).
func strutturaSoloPartiACM(fonte bool) valutazione.StrutturaDaVerificare {
	return valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: fonte, BOMDiLavoro: fonte, Radici: []string{radiceStepAC},
		Nodi: []string{radiceStepAC}}
}

func analisiSoloPartiACM() valutazione.AnalisiStruttura {
	return valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, MotivoParziale: "nessuna occorrenza di assieme: solo parti"}
}

func fonteACM() *valutazione.FonteDellaVerifica {
	return &valutazione.FonteDellaVerifica{DocumentoID: docFonteACM, Sha256: shaFonteACM, Analisi: analisiACM}
}

func derogaValidaACM() valutazione.DerogaStruttura {
	return valutazione.DerogaStruttura{ID: derogaACM, StepDocumentoID: docFonteACM, StepSha256: shaFonteACM, Analisi: analisiACM,
		Motivo: "lo STEP è una parte multicorpo: la gerarchia viene dalla distinta del PDF", Stato: valutazione.StatoDerogaValida}
}

// ricostruzioneACM: la ricostruzione manuale con 13 figli, la fonte «solo parti» confermata, la deroga valida.
func ricostruzioneACM() valutazione.IngressoVerificaConferma {
	c := confermaManualeACM(qtaACM)
	return valutazione.IngressoVerificaConferma{Conferma: c, Struttura: strutturaSoloPartiACM(true), ArchiCorrenti: correntiDi(c), Fonte: fonteACM(),
		Analisi: analisiSoloPartiACM(), Deroghe: []valutazione.DerogaStruttura{derogaValidaACM()}}
}

func esitoConferma(v valutazione.VerificaBOM) string {
	return fmt.Sprintf("verificata=%v nomenclatura=%s gerarchia=%s criterio=%s", v.Verificata, asse(v.Nomenclatura), asse(v.Gerarchia), v.Criterio)
}

// ---- B3.4: gerarchia_step ----

// TestGerarchiaStepDi (B3.4; lente struttura §5; SHOCK-01 parte gerarchia_step): i quattro valori dai numeri della
// lettura, e il motivo parziale che resta com'è (vincolo V2).
func TestGerarchiaStepDi(t *testing.T) {
	soloParti := analisiSoloPartiACM()
	for _, c := range []struct {
		nome   string
		a      valutazione.AnalisiStruttura
		atteso valutazione.GerarchiaStep
	}{
		{"SHOCK-01: una radice, zero relazioni, lettura completa", soloParti, valutazione.GerarchiaStepNonRiconosciuta},
		{"una radice, zero relazioni, senza motivo", valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1}, valutazione.GerarchiaStepNonRiconosciuta},
		{"lettura troncata prima delle occorrenze", valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Troncato: true, MotivoParziale: "lettura troncata"}, valutazione.GerarchiaStepParziale},
		{"scarti", valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Relazioni: 4, Scarti: 2}, valutazione.GerarchiaStepParziale},
		{"più radici", valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 2, Relazioni: 3}, valutazione.GerarchiaStepParziale},
		{"relazioni con un motivo parziale", valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Relazioni: 3, MotivoParziale: "occorrenze non risolte"}, valutazione.GerarchiaStepParziale},
		{"relazioni, lettura completa", valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Relazioni: 12}, valutazione.GerarchiaStepRiconosciuta},
		{"nessuna analisi", valutazione.AnalisiStruttura{}, valutazione.GerarchiaStepNonAnalizzabile},
		{"versione sconosciuta", valutazione.AnalisiStruttura{Letta: true, Versione: 2, Radici: 1}, valutazione.GerarchiaStepNonAnalizzabile},
		{"nessuna radice", valutazione.AnalisiStruttura{Letta: true, Versione: 3}, valutazione.GerarchiaStepNonAnalizzabile},
	} {
		if got := valutazione.GerarchiaStepDi(c.a); got != c.atteso {
			t.Errorf("%s: %s, atteso %s", c.nome, got, c.atteso)
		}
	}
	// V2: il motivo parziale non si svuota, né nell'ingresso né nell'uscita
	in := ricostruzioneACM()
	v := valutazione.VerificaDallaConferma(in)
	if in.Analisi.MotivoParziale != soloParti.MotivoParziale || v.Conferma == nil || v.Conferma.MotivoParzialeStep != soloParti.MotivoParziale {
		t.Errorf("il motivo parziale è cambiato: ingresso %q, uscita %+v", in.Analisi.MotivoParziale, v.Conferma)
	}
}

// ---- SHOCK-01 ----

// TestShock01UnaRadiceZeroRelazioni (SHOCK-01; contratto rev. 3 §8.3 ultima riga, E1.1): uno STEP con una radice e
// nessuna relazione, confermato senza figli e senza dichiarazione (criterio nessuno): nessun mono_pezzo automatico, la
// gerarchia non è verificata (senza_figli_non_dichiarato), il criterio è nessuno, gerarchia_step non_riconosciuta. La
// nomenclatura della radice sì (R81).
func TestShock01UnaRadiceZeroRelazioni(t *testing.T) {
	c := confermaManualeACM(nil)
	c.Criterio = valutazione.CriterioNessuno
	v := valutazione.VerificaDallaConferma(valutazione.IngressoVerificaConferma{Conferma: c, Struttura: strutturaSoloPartiACM(true), Fonte: fonteACM(),
		Analisi: analisiSoloPartiACM(), Deroghe: []valutazione.DerogaStruttura{derogaValidaACM()}})
	if v.Verificata || asse(v.Gerarchia) != "da_verificare/senza_figli_non_dichiarato" || asse(v.Nomenclatura) != "verificata/" ||
		v.Criterio != valutazione.CriterioNessuno || v.GerarchiaStep != valutazione.GerarchiaStepNonRiconosciuta || v.OrigineGerarchia != "" {
		t.Errorf("SHOCK-01: %s, gerarchia_step %s, origine %q", esitoConferma(v), v.GerarchiaStep, v.OrigineGerarchia)
	}
	if v.Conferma.Deroga != nil {
		t.Errorf("SHOCK-01: senza figli la deroga non entra: %+v", v.Conferma.Deroga)
	}
}

// ---- B3.2 e B3.3: la ricostruzione manuale e la deroga ----

// TestRicostruzioneManualeConDeroga (D-SHOCK-S1 B; SHOCK-02 parte regola; precisazioni a, c, d): 13 figli decisi a mano
// con le loro qta, lo STEP della fonte «solo parti» confermato, la deroga valida per quello STEP, quello sha e
// quell'analisi: la BOM è verificata, con il criterio step, l'origine della gerarchia manuale, la radice STEP mappata
// sulla radice certificata e la deroga applicata. Il gesto è nuovo (non legacy) e copre i due assi.
func TestRicostruzioneManualeConDeroga(t *testing.T) {
	v := valutazione.VerificaDallaConferma(ricostruzioneACM())
	if !v.Verificata || asse(v.Nomenclatura) != "verificata/" || asse(v.Gerarchia) != "verificata/" || v.Criterio != valutazione.CriterioStep ||
		v.OrigineGerarchia != valutazione.OrigineGerarchiaManuale || v.GerarchiaStep != valutazione.GerarchiaStepNonRiconosciuta || !v.FonteRegistrata ||
		v.Nomenclatura.Legacy || v.Gerarchia.Legacy || v.Nomenclatura.DaDecidere != 0 || v.Gerarchia.DaDecidere != 0 {
		t.Fatalf("ricostruzione: %s, origine %s, BOM %+v", esitoConferma(v), v.OrigineGerarchia, v)
	}
	d := v.Conferma
	if d == nil || !d.RadiceMappata || !d.Valida || d.Numero != 1 || len(d.Cambiamenti) != 0 || d.Deroga == nil ||
		d.Deroga.Stato != valutazione.DerogaApplicata || d.Deroga.ID == nil || *d.Deroga.ID != derogaACM || d.Deroga.Motivo == "" {
		t.Errorf("dettaglio %+v, deroga %+v", d, d.Deroga)
	}
}

// TestRicostruzioneManualeSenzaDerogaValida (D-SHOCK-S1 B): senza deroga, con una deroga scaduta, di un altro STEP o di
// un altro sha, di un'altra analisi o senza motivo, la BOM non è verificata, con un motivo della gerarchia che lo dice;
// la nomenclatura resta verificata, il criterio resta quello della conferma.
func TestRicostruzioneManualeSenzaDerogaValida(t *testing.T) {
	scaduta := derogaValidaACM()
	scaduta.Stato = valutazione.StatoDerogaSuperata
	altroSha := derogaValidaACM()
	altroSha.StepSha256 = sha("b")
	altroDoc := derogaValidaACM()
	altroDoc.StepDocumentoID = docAltroACM
	altraAnalisi := derogaValidaACM()
	altraAnalisi.Analisi.CalcolataIl = "2026-10-11T08:00:00Z"
	senzaMotivo := derogaValidaACM()
	senzaMotivo.Motivo = "  "
	for _, c := range []struct {
		nome    string
		deroghe []valutazione.DerogaStruttura
		motivo  string
	}{
		{"senza deroga", nil, valutazione.MotivoAsseDerogaAssente},
		{"deroga scaduta", []valutazione.DerogaStruttura{scaduta}, valutazione.MotivoAsseDerogaScaduta},
		{"deroga di un altro sha", []valutazione.DerogaStruttura{altroSha}, valutazione.MotivoAsseDerogaAltroStep},
		{"deroga di un altro STEP", []valutazione.DerogaStruttura{altroDoc}, valutazione.MotivoAsseDerogaAltroStep},
		{"deroga di un'altra analisi", []valutazione.DerogaStruttura{altraAnalisi}, valutazione.MotivoAsseDerogaAltraAnalisi},
		{"deroga senza motivo", []valutazione.DerogaStruttura{senzaMotivo}, valutazione.MotivoAsseDerogaSenzaMotivo},
	} {
		in := ricostruzioneACM()
		in.Deroghe = c.deroghe
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Gerarchia) != "da_verificare/"+c.motivo || asse(v.Nomenclatura) != "verificata/" || v.Criterio != valutazione.CriterioStep {
			t.Errorf("%s: %s", c.nome, esitoConferma(v))
		}
		if v.Conferma.Deroga == nil || "deroga_struttura_"+v.Conferma.Deroga.Stato != c.motivo {
			t.Errorf("%s: deroga esposta %+v", c.nome, v.Conferma.Deroga)
		}
	}
	// una deroga che vale dopo una che non vale: si applica quella che vale
	in := ricostruzioneACM()
	in.Deroghe = []valutazione.DerogaStruttura{scaduta, derogaValidaACM()}
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata {
		t.Errorf("deroga valida dopo una scaduta: %s, %+v", esitoConferma(v), v.Conferma.Deroga)
	}
}

// TestParzialeVuoleLaDeroga ([T] di B3.3, prudente): una lettura parziale (troncata) con il criterio step vuole la
// stessa deroga, come il cancello del congelamento per presente_parziale; uno STEP non analizzabile vuole la deroga come
// gli altri due valori.
//
// Riscritta per D12 §2.1 [U] (direttiva 12, T-2 superata): prima la parte «non analizzabile» fissava il blocco anche con
// la deroga (gerarchia_step_non_analizzabile). Ora la deroga si valuta. Un'analisi corrente con la struttura che non si
// legge (presente_parziale della 0020): la fonte ha la chiave vera, la deroga di quella chiave vale, quella con la chiave
// vuota no (altra_analisi). Uno STEP senza analisi corrente (chiave vuota della 0020): vale la deroga con la chiave vuota.
// La forma di questa prova (radice STEP nei nodi con l'analisi vuota)
// nascondeva il caso dello STEP che non dà nodi: lo coprono le prove dello scenario A (bom_conferma_d12_test.go).
func TestParzialeVuoleLaDeroga(t *testing.T) {
	in := ricostruzioneACM()
	in.Analisi = valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Troncato: true, MotivoParziale: "lettura troncata"}
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || v.GerarchiaStep != valutazione.GerarchiaStepParziale {
		t.Errorf("parziale con la deroga: %s, %s", esitoConferma(v), v.GerarchiaStep)
	}
	in.Deroghe = nil
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseDerogaAssente {
		t.Errorf("parziale senza la deroga: %s", esitoConferma(v))
	}
	in = ricostruzioneACM()
	in.Analisi = valutazione.AnalisiStruttura{MotivoParziale: "struttura non letta"}
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || v.GerarchiaStep != valutazione.GerarchiaStepNonAnalizzabile ||
		v.Conferma.Deroga == nil || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata {
		t.Errorf("struttura illeggibile con la deroga di quell'analisi: %s, %s", esitoConferma(v), v.GerarchiaStep)
	}
	in.Deroghe[0].Analisi = valutazione.ChiaveAnalisi{}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseDerogaAltraAnalisi {
		t.Errorf("struttura illeggibile con la deroga dello STEP non analizzato: %s", esitoConferma(v))
	}
	in.Fonte.Analisi, in.Analisi.MotivoParziale = valutazione.ChiaveAnalisi{}, "nessuna analisi corrente dello STEP"
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || v.GerarchiaStep != valutazione.GerarchiaStepNonAnalizzabile ||
		v.Conferma.Deroga == nil || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata {
		t.Errorf("non analizzabile con la deroga 0020 dello STEP non analizzato: %s, %s", esitoConferma(v), v.GerarchiaStep)
	}
	in.Deroghe = nil
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseDerogaAssente {
		t.Errorf("non analizzabile senza la deroga: %s", esitoConferma(v))
	}
}

// TestK02FonteRevocata (K-02 lettura A, invariata; E1.2; scenario C5 della lente): la fonte revocata o superata toglie
// la verifica anche con la conferma valida e la deroga valida: gerarchia fonte_non_confermata, criterio nessuno, nessuna
// fonte registrata.
func TestK02FonteRevocata(t *testing.T) {
	in := ricostruzioneACM()
	in.Struttura = strutturaSoloPartiACM(false)
	v := valutazione.VerificaDallaConferma(in)
	if v.Verificata || asse(v.Gerarchia) != "da_verificare/fonte_non_confermata" || v.Criterio != valutazione.CriterioNessuno || v.FonteRegistrata {
		t.Errorf("fonte revocata: %s, fonte registrata %v", esitoConferma(v), v.FonteRegistrata)
	}
}

// TestArchiCambiatiDopoLaConferma (precisazioni b ed e; DOM-10; contratto rev. 3 §11.1 n. 3): un arco aggiunto, un arco
// tolto o una qta cambiata dopo la conferma tolgono la verifica sui due assi, con il motivo
// struttura_cambiata_dopo_conferma e il cambiamento esposto. Arco aggiunto e poi tolto: la struttura torna uguale e la
// verifica torna (contratto rev. 3 §8.4, ritorno della firma).
func TestArchiCambiatiDopoLaConferma(t *testing.T) {
	arco := func(i int) string { return valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(i))) }
	for _, c := range []struct {
		nome        string
		cambia      func([]valutazione.ArcoCorrente) []valutazione.ArcoCorrente
		cambiamento string
	}{
		{"arco aggiunto", func(a []valutazione.ArcoCorrente) []valutazione.ArcoCorrente {
			return append(a, valutazione.ArcoCorrente{PadreID: finitoACM, FiglioID: figlioACM(20), Qta: 1})
		}, "aggiunto:" + arco(20)},
		{"arco tolto", func(a []valutazione.ArcoCorrente) []valutazione.ArcoCorrente { return a[1:] }, "tolto:" + arco(0)},
		{"qta cambiata", func(a []valutazione.ArcoCorrente) []valutazione.ArcoCorrente { a[6].Qta = 3; return a }, "qta:" + arco(6) + ":4->3"},
		{"qta cresciuta", func(a []valutazione.ArcoCorrente) []valutazione.ArcoCorrente { a[0].Qta = 5; return a }, "qta:" + arco(0) + ":1->5"},
	} {
		in := ricostruzioneACM()
		in.ArchiCorrenti = c.cambia(in.ArchiCorrenti)
		v := valutazione.VerificaDallaConferma(in)
		m := "da_verificare/" + valutazione.MotivoAsseStrutturaCambiata
		if v.Verificata || asse(v.Nomenclatura) != m || asse(v.Gerarchia) != m || v.Criterio != valutazione.CriterioStep {
			t.Errorf("%s: %s", c.nome, esitoConferma(v))
		}
		if !reflect.DeepEqual(v.Conferma.Cambiamenti, []string{c.cambiamento}) {
			t.Errorf("%s: cambiamenti %v, atteso %s", c.nome, v.Conferma.Cambiamenti, c.cambiamento)
		}
	}
	in := ricostruzioneACM()
	in.ArchiCorrenti = append(in.ArchiCorrenti, valutazione.ArcoCorrente{PadreID: finitoACM, FiglioID: figlioACM(20), Qta: 1})
	in.ArchiCorrenti = in.ArchiCorrenti[:len(in.ArchiCorrenti)-1]
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata {
		t.Errorf("arco aggiunto e tolto: %s", esitoConferma(v))
	}
}

// TestConfermaNonValidaOIncoerente: una conferma non valida (contatore e firma cambiati, lo dice il chiamante) non dà un
// gesto; una conferma con due radici non si crede (blocco prudente); il criterio nessuno con figli e la fonte confermata
// non regge; la radice STEP che non è quella certificata lascia la nomenclatura non verificata (precisazione a).
func TestConfermaNonValidaOIncoerente(t *testing.T) {
	in := ricostruzioneACM()
	in.Conferma.Valida = false
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/conferma_non_valida" ||
		asse(v.Gerarchia) != "da_verificare/conferma_non_valida" || v.Conferma.Valida || v.Criterio != valutazione.CriterioNessuno {
		t.Errorf("non valida: %s", esitoConferma(v))
	}
	in = ricostruzioneACM()
	in.Conferma.Componenti[1].Radice = true
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/conferma_incoerente" ||
		asse(v.Gerarchia) != "da_verificare/conferma_incoerente" || v.Criterio != valutazione.CriterioNessuno {
		t.Errorf("due radici: %s", esitoConferma(v))
	}
	in = ricostruzioneACM()
	in.Conferma.Criterio = valutazione.CriterioNessuno
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/criterio_conferma_nessuno" ||
		v.Criterio != valutazione.CriterioNessuno {
		t.Errorf("criterio nessuno con figli: %s", esitoConferma(v))
	}
	in = ricostruzioneACM()
	in.Conferma.Componenti[0].StepSha256 = sha("b")
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/radice_step_non_mappata" ||
		v.Conferma.RadiceMappata {
		t.Errorf("radice non mappata: %s", esitoConferma(v))
	}
}

// ---- il ramo mono_pezzo ----

// TestMonoPezzo (D-P4B1-02 A, D-P4A-02 A; contratto rev. 3 §8.3 righe 4 e 5; SHOCK-06 parte regola): con lo STEP del
// finito confermato, la dichiarazione motivata e nessun figlio, la BOM è verificata con il criterio mono_pezzo, senza
// deroga (la dichiarazione è la classificazione del nodo); senza la fonte confermata no (K-02 resta: il ramo non lo
// salta); con un figlio aggiunto dopo la dichiarazione decade; senza lo STEP sulla radice certificata, su una radice
// che non è un finito o senza motivo non si applica.
func TestMonoPezzo(t *testing.T) {
	ingresso := func() valutazione.IngressoVerificaConferma {
		return valutazione.IngressoVerificaConferma{Conferma: monoACM(), Struttura: strutturaSoloPartiACM(true), Fonte: fonteACM(), Analisi: analisiSoloPartiACM()}
	}
	v := valutazione.VerificaDallaConferma(ingresso())
	if !v.Verificata || v.Criterio != valutazione.CriterioMonoPezzo || v.OrigineGerarchia != "" || v.Conferma.Deroga != nil ||
		v.Conferma.MonoPezzoMotivo == "" || !v.FonteRegistrata {
		t.Errorf("mono con STEP: %s, %+v", esitoConferma(v), v.Conferma)
	}
	// R-06 della revisione di B3: la dichiarazione esce con l'autore e il momento (contratto rev. 3 §8.3 riga 4)
	if v.Conferma.ConfermatoDa == nil || *v.Conferma.ConfermatoDa != operatore || v.Conferma.ConfermatoIl == "" {
		t.Errorf("mono con STEP: autore della dichiarazione %v %q", v.Conferma.ConfermatoDa, v.Conferma.ConfermatoIl)
	}

	in := ingresso()
	in.Struttura = strutturaSoloPartiACM(false)
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/fonte_non_confermata" || v.Criterio != valutazione.CriterioMonoPezzo {
		t.Errorf("mono senza STEP confermato: %s", esitoConferma(v))
	}
	// il ritocco T-9 vale solo quando la nomenclatura è ferma soltanto per la radice STEP che senza fonte non si mappa: con
	// un altro nodo aperto resta il motivo di A1c (R-01 della revisione di B3: fonte_non_confermata solo se è l'unica cosa
	// che manca)
	in = ingresso()
	in.Struttura = strutturaSoloPartiACM(false)
	in.Struttura.Nodi = append(in.Struttura.Nodi, ancoraggio.RifNodo(shaFonteACM, "#5"))
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/da_decidere" ||
		asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseNomenclaturaRadice {
		t.Errorf("mono senza STEP confermato, con un altro nodo aperto: %s", esitoConferma(v))
	}

	in = ingresso()
	in.ArchiCorrenti = []valutazione.ArcoCorrente{{PadreID: finitoACM, FiglioID: figlioACM(0), Qta: 1}}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/mono_pezzo_decaduto" {
		t.Errorf("mono con un figlio aggiunto dopo: %s", esitoConferma(v))
	}

	for nome, cambia := range map[string]func(*valutazione.ConfermaStrutturale){
		"radice senza STEP": func(c *valutazione.ConfermaStrutturale) { c.Componenti[0] = radiceACM(false) },
		"radice non finito": func(c *valutazione.ConfermaStrutturale) { c.Componenti[0].Tipo = "sottoassieme" },
		"motivo vuoto":      func(c *valutazione.ConfermaStrutturale) { c.MonoPezzoMotivo = " " },
		"archi nella conferma": func(c *valutazione.ConfermaStrutturale) {
			c.Componenti = append(c.Componenti, valutazione.ComponenteConfermato{ComponenteID: figlioACM(0), Codice: "ACM-9000-01", Tipo: "sciolto"})
			c.Archi = []valutazione.ArcoConfermato{{PadreID: finitoACM, FiglioID: figlioACM(0), Qta: 1, Origine: "utente"}}
		},
	} {
		in := ingresso()
		cambia(in.Conferma)
		in.ArchiCorrenti = correntiDi(in.Conferma)
		if v := valutazione.VerificaDallaConferma(in); v.Verificata || v.Gerarchia.Stato == valutazione.StatoAsseVerificata {
			t.Errorf("mono, %s: %s", nome, esitoConferma(v))
		} else if asse(v.Gerarchia) != "da_verificare/mono_pezzo_non_applicabile" {
			t.Errorf("mono, %s: %s", nome, esitoConferma(v))
		}
	}
}

// ---- SHOCK-05 ----

// TestShock05SecondarioCheAggiungeSoltantoFigli (D-SHOCK-S4 A [U]; SHOCK-05 parte regola): la conferma resta valida e la
// struttura non è cambiata (il contatore non si muove), ma uno STEP secondario autorizzato nel perimetro propone due
// figli nuovi con i loro archi, righe aperte: la verifica non è positiva, da_decidere, anche con la deroga valida.
func TestShock05SecondarioCheAggiungeSoltantoFigli(t *testing.T) {
	in := ricostruzioneACM()
	shaB := sha("d")
	in.SecondarieAperte = []valutazione.RigaSecondariaAperta{
		{Rif: ancoraggio.RifNodo(shaB, "#7")}, {Rif: ancoraggio.RifNodo(shaB, "#8")},
		{Rif: valutazione.RifArco(ancoraggio.RifNodo(shaB, "#1"), ancoraggio.RifNodo(shaB, "#7")), Arco: true},
		{Rif: valutazione.RifArco(ancoraggio.RifNodo(shaB, "#1"), ancoraggio.RifNodo(shaB, "#8")), Arco: true},
	}
	v := valutazione.VerificaDallaConferma(in)
	if v.Verificata || asse(v.Gerarchia) != "da_verificare/da_decidere" || asse(v.Nomenclatura) != "da_verificare/da_decidere" ||
		v.Gerarchia.DaDecidere != 4 || v.Nomenclatura.DaDecidere != 2 || v.Conferma.SecondarieAperte != 4 || len(v.Conferma.Cambiamenti) != 0 {
		t.Errorf("SHOCK-05: %s (%d, %d), %+v", esitoConferma(v), v.Nomenclatura.DaDecidere, v.Gerarchia.DaDecidere, v.Conferma)
	}
}

// ---- STEP con relazioni: origine mista, contraddizione aperta, conflitto deciso ----

// stepConRelazioniACM: uno STEP riconosciuto con la radice #1 e due figli #2, #3 decisi da una persona come i figli 0 e
// 1; la conferma certifica quei due archi (origine step) più un terzo figlio aggiunto a mano (origine utente).
func stepConRelazioniACM() valutazione.IngressoVerificaConferma {
	n2, n3 := ancoraggio.RifNodo(shaFonteACM, "#2"), ancoraggio.RifNodo(shaFonteACM, "#3")
	c := confermaManualeACM([]int{2, 1, 1})
	c.Archi[0].Origine, c.Archi[1].Origine = "step", "step"
	s := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true, Radici: []string{radiceStepAC},
		Nodi: []string{radiceStepAC, n2, n3}, Archi: []string{valutazione.RifArco(radiceStepAC, n2), valutazione.RifArco(radiceStepAC, n3)}}
	return valutazione.IngressoVerificaConferma{Conferma: c, Struttura: s, ArchiCorrenti: correntiDi(c), Fonte: fonteACM(),
		DecisioniStep: valutazione.DecisioniStep{Nodi: map[string]uuid.UUID{n2: figlioACM(0), n3: figlioACM(1)}},
		Analisi:       valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Relazioni: 2}}
}

// TestOrigineMistaEContraddizioni (E2.1: conflitto deciso ≠ evidenza insufficiente ≠ contraddizione aperta; precisazione
// c): con lo STEP riconosciuto, due archi dallo STEP e uno a mano danno la verifica con l'origine mista, senza deroga.
// Un arco STEP proposto e non recepito dalla conferma è una contraddizione aperta: da_decidere. Lo stesso arco tolto da
// una persona è un conflitto deciso: la verifica torna. Con lo STEP riconosciuto la deroga non si legge; il caso in cui
// la deroga si legge e non converte la contraddizione è quello parziale, più sotto (R-04 della revisione di B3, sonda
// S9). Un conflitto aperto di gerarchia blocca anche con la deroga.
func TestOrigineMistaEContraddizioni(t *testing.T) {
	v := valutazione.VerificaDallaConferma(stepConRelazioniACM())
	if !v.Verificata || v.OrigineGerarchia != valutazione.OrigineGerarchiaMista || v.GerarchiaStep != valutazione.GerarchiaStepRiconosciuta ||
		v.Conferma.Deroga != nil || v.Criterio != valutazione.CriterioStep {
		t.Errorf("origine mista: %s, origine %s, deroga %+v", esitoConferma(v), v.OrigineGerarchia, v.Conferma.Deroga)
	}

	n4 := ancoraggio.RifNodo(shaFonteACM, "#4")
	aperto := valutazione.RifArco(radiceStepAC, n4)
	in := stepConRelazioniACM()
	in.Struttura.Nodi = append(in.Struttura.Nodi, n4)
	in.Struttura.Archi = append(in.Struttura.Archi, aperto)
	in.Deroghe = []valutazione.DerogaStruttura{derogaValidaACM()}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/da_decidere" || asse(v.Nomenclatura) != "da_verificare/da_decidere" {
		t.Errorf("arco STEP aperto non recepito: %s", esitoConferma(v))
	}
	in.DecisioniStep.NodiTolti, in.DecisioniStep.ArchiTolti = []string{n4}, []string{aperto}
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata {
		t.Errorf("arco STEP tolto da una persona: %s", esitoConferma(v))
	}

	// lo STEP parziale (relazioni con occorrenze non risolte) vuole la deroga e la legge: una deroga valida non converte
	// l'arco STEP aperto e non recepito (da_decidere sui due assi, deroga applicata ma senza effetto); con l'arco tolto
	// da una persona la verifica torna, con la deroga applicata
	in = stepConRelazioniACM()
	in.Analisi = valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Relazioni: 2, MotivoParziale: "occorrenze non risolte"}
	in.Struttura.Nodi = append(in.Struttura.Nodi, n4)
	in.Struttura.Archi = append(in.Struttura.Archi, aperto)
	in.Deroghe = []valutazione.DerogaStruttura{derogaValidaACM()}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/da_decidere" ||
		asse(v.Nomenclatura) != "da_verificare/da_decidere" || v.GerarchiaStep != valutazione.GerarchiaStepParziale ||
		v.Conferma.Deroga == nil || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata {
		t.Errorf("parziale, arco STEP aperto, deroga valida: %s, %s, %+v", esitoConferma(v), v.GerarchiaStep, v.Conferma.Deroga)
	}
	in.DecisioniStep.NodiTolti, in.DecisioniStep.ArchiTolti = []string{n4}, []string{aperto}
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata {
		t.Errorf("parziale, arco STEP tolto da una persona, deroga valida: %s", esitoConferma(v))
	}
	in.Deroghe = nil
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseDerogaAssente {
		t.Errorf("parziale, arco STEP tolto da una persona, senza deroga: %s", esitoConferma(v))
	}

	in = ricostruzioneACM()
	in.Struttura.Conflitti = []valutazione.Conflitto{{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia,
		Rif: valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(2))), Motivo: "qta decisa contro lo STEP"}}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "conflitto/conflitto" {
		t.Errorf("conflitto aperto con la deroga valida: %s", esitoConferma(v))
	}
}

// ---- senza conferma e legacy invariato ----

// TestSenzaConfermaEJSONLegacy (punto 8 del mandato): senza conferma la regola non inventa un gesto (nessun_gesto,
// criterio nessuno); il chiamante usa l'adattatore legacy, e una VerificaBOM legacy (campi nuovi vuoti) si scrive in JSON
// con le sole chiavi di prima, byte per byte.
func TestSenzaConfermaEJSONLegacy(t *testing.T) {
	v := valutazione.VerificaDallaConferma(valutazione.IngressoVerificaConferma{Struttura: strutturaSoloPartiACM(true), Analisi: analisiSoloPartiACM()})
	if v.Verificata || asse(v.Nomenclatura) != "da_verificare/nessun_gesto" || v.Criterio != valutazione.CriterioNessuno || v.Conferma != nil {
		t.Errorf("senza conferma: %s", esitoConferma(v))
	}
	legacy := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Legacy: true}, strutturaSoloPartiACM(true))
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal(b, &chiavi); err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range chiavi {
		got = append(got, k)
	}
	sort.Strings(got)
	if want := []string{"fonte_registrata", "gerarchia", "nomenclatura", "rimozioni_aperte", "senza_figli", "verificata"}; !reflect.DeepEqual(got, want) {
		t.Errorf("chiavi JSON della verifica legacy: %v, attese %v", got, want)
	}
	if strings.Contains(string(b), "criterio") || strings.Contains(string(b), "conferma") {
		t.Errorf("la verifica legacy espone i campi nuovi: %s", b)
	}
}

// ---- l'ordine dei motivi (R-01 e R-04 della revisione di B3) ----

// TestOrdineDeiMotivi (rangoDelMotivo; A1c T-E1-14 e T-E1R-04; D-SHOCK-S1 B; R-01 e R-04 della revisione di B3): due
// condizioni che mancano insieme, e il motivo che esce. Una mancanza indipendente dalla fonte (conflitto, conferma che
// non vale, struttura cambiata, copertura, dichiarazione) prevale sulla deroga e su K-02; senza la fonte confermata la
// deroga e la gerarchia dello STEP non si valutano, e fonte_non_confermata esce solo quando è l'unica cosa che manca.
func TestOrdineDeiMotivi(t *testing.T) {
	type passo = func(*valutazione.IngressoVerificaConferma)
	senzaDeroga := func(in *valutazione.IngressoVerificaConferma) { in.Deroghe = nil }
	senzaFonte := func(in *valutazione.IngressoVerificaConferma) { in.Struttura = strutturaSoloPartiACM(false) }
	conflitto := func(in *valutazione.IngressoVerificaConferma) {
		in.Struttura.Conflitti = []valutazione.Conflitto{{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia,
			Rif: valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(2))), Motivo: "qta decisa contro lo STEP"}}
	}
	tolto := func(in *valutazione.IngressoVerificaConferma) { in.ArchiCorrenti = in.ArchiCorrenti[1:] }
	secondaria := func(in *valutazione.IngressoVerificaConferma) {
		in.SecondarieAperte = []valutazione.RigaSecondariaAperta{{Rif: ancoraggio.RifNodo(sha("d"), "#7")}}
	}
	nonAnalizzabile := func(in *valutazione.IngressoVerificaConferma) { in.Analisi = valutazione.AnalisiStruttura{} }
	nonValida := func(in *valutazione.IngressoVerificaConferma) { in.Conferma.Valida = false }
	altraRadice := func(in *valutazione.IngressoVerificaConferma) { in.Conferma.Componenti[0].StepSha256 = sha("b") }
	criterioNessuno := func(in *valutazione.IngressoVerificaConferma) { in.Conferma.Criterio = valutazione.CriterioNessuno }
	senzaFigli := func(in *valutazione.IngressoVerificaConferma) {
		in.Conferma = confermaManualeACM(nil)
		in.Conferma.Criterio = valutazione.CriterioNessuno
		in.ArchiCorrenti = nil
	}
	m := func(s string) string { return "da_verificare/" + s }
	for _, c := range []struct {
		nome           string
		passi          []passo
		nomenclatura   string // "" = non si guarda
		gerarchia      string
		derogaValutata bool
		criterio       valutazione.CriterioVerifica
		gerarchiaStep  valutazione.GerarchiaStep
	}{
		{"conflitto + deroga mancante", []passo{conflitto, senzaDeroga},
			"verificata/", "conflitto/conflitto", true, valutazione.CriterioStep, valutazione.GerarchiaStepNonRiconosciuta},
		{"struttura cambiata + deroga mancante", []passo{tolto, senzaDeroga},
			m(valutazione.MotivoAsseStrutturaCambiata), m(valutazione.MotivoAsseStrutturaCambiata), true, valutazione.CriterioStep, valutazione.GerarchiaStepNonRiconosciuta},
		{"SHOCK-05 + deroga mancante", []passo{secondaria, senzaDeroga},
			m(valutazione.MotivoAsseDaDecidere), m(valutazione.MotivoAsseDaDecidere), true, valutazione.CriterioStep, valutazione.GerarchiaStepNonRiconosciuta},
		{"conferma non valida + deroga mancante", []passo{nonValida, senzaDeroga},
			m(valutazione.MotivoAsseConfermaNonValida), m(valutazione.MotivoAsseConfermaNonValida), true, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonRiconosciuta},
		{"radice non mappata + deroga mancante", []passo{altraRadice, senzaDeroga},
			m(valutazione.MotivoAsseRadiceNonMappata), m(valutazione.MotivoAsseDerogaAssente), true, valutazione.CriterioStep, valutazione.GerarchiaStepNonRiconosciuta},
		// Riscritta per D12 §2.1 (T-2 superata): lo STEP non analizzabile chiede la deroga, quindi la deroga si valuta
		// (prima false); il motivo resta criterio_conferma_nessuno, che viene prima
		{"criterio nessuno + STEP non analizzabile", []passo{criterioNessuno, nonAnalizzabile},
			"verificata/", m(valutazione.MotivoAsseCriterioConfermaNessuno), true, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonAnalizzabile},
		{"fonte non confermata + deroga mancante", []passo{senzaFonte, senzaDeroga},
			"", m(valutazione.MotivoAsseFonteNonConfermata), false, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonRiconosciuta},
		{"fonte non confermata + STEP non analizzabile", []passo{senzaFonte, nonAnalizzabile},
			"", m(valutazione.MotivoAsseFonteNonConfermata), false, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonAnalizzabile},
		{"fonte non confermata + struttura cambiata", []passo{senzaFonte, tolto},
			m(valutazione.MotivoAsseStrutturaCambiata), m(valutazione.MotivoAsseStrutturaCambiata), false, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonRiconosciuta},
		{"fonte non confermata + conflitto", []passo{senzaFonte, conflitto},
			"", "conflitto/conflitto", false, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonRiconosciuta},
		{"fonte non confermata + conferma non valida", []passo{senzaFonte, nonValida},
			m(valutazione.MotivoAsseConfermaNonValida), m(valutazione.MotivoAsseConfermaNonValida), false, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonRiconosciuta},
		{"fonte non confermata + senza figli non dichiarato", []passo{senzaFonte, senzaFigli},
			"", m(valutazione.MotivoAsseSenzaFigliNonDichiarato), false, valutazione.CriterioNessuno, valutazione.GerarchiaStepNonRiconosciuta},
	} {
		in := ricostruzioneACM()
		for _, p := range c.passi {
			p(&in)
		}
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Gerarchia) != c.gerarchia || (c.nomenclatura != "" && asse(v.Nomenclatura) != c.nomenclatura) {
			t.Errorf("%s: %s", c.nome, esitoConferma(v))
		}
		if (v.Conferma.Deroga != nil) != c.derogaValutata {
			t.Errorf("%s: deroga valutata %+v, attesa %v", c.nome, v.Conferma.Deroga, c.derogaValutata)
		}
		if v.Criterio != c.criterio || v.GerarchiaStep != c.gerarchiaStep {
			t.Errorf("%s: criterio %s, gerarchia_step %s; attesi %s, %s", c.nome, v.Criterio, v.GerarchiaStep, c.criterio, c.gerarchiaStep)
		}
	}
}

// ---- ingressi incoerenti e radice (R-03, R-05, R-07 della revisione di B3) ----

// TestIngressiIncoerenti (R-03 e R-07 della revisione di B3; D-SHOCK-S1 B: nessuna verifica positiva da una scorciatoia):
// un ingresso che contraddice un altro non si scarta in silenzio e non dà un true.
//   - Struttura.ArchiConfermati con un figlio e ArchiCorrenti mancanti, su una conferma mono_pezzo: ingresso_incoerente;
//   - Struttura.ArchiConfermati con le stesse coppie degli archi correnti: coerente, la verifica resta;
//   - Struttura.ArchiConfermati con una coppia in meno: ingresso_incoerente;
//   - analisi letta con la chiave d'analisi della fonte vuota (e una deroga con la chiave vuota): ingresso_incoerente;
//   - due radici STEP dello stesso sha nella BOM di lavoro: nessuna si mappa sulla radice certificata.
func TestIngressiIncoerenti(t *testing.T) {
	m := "da_verificare/" + valutazione.MotivoAsseIngressoIncoerente
	in := valutazione.IngressoVerificaConferma{Conferma: monoACM(), Struttura: strutturaSoloPartiACM(true), Fonte: fonteACM(), Analisi: analisiSoloPartiACM()}
	in.Struttura.ArchiConfermati = []string{valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(0)))}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != m || asse(v.Gerarchia) != m || v.Criterio != valutazione.CriterioNessuno {
		t.Errorf("mono con ArchiConfermati e senza ArchiCorrenti: %s", esitoConferma(v))
	}

	in = ricostruzioneACM()
	for _, a := range in.ArchiCorrenti {
		in.Struttura.ArchiConfermati = append(in.Struttura.ArchiConfermati, valutazione.RifArco(rifC(a.PadreID), rifC(a.FiglioID)))
	}
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata {
		t.Errorf("ArchiConfermati uguali agli archi correnti: %s", esitoConferma(v))
	}
	in.Struttura.ArchiConfermati = in.Struttura.ArchiConfermati[1:]
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != m || asse(v.Gerarchia) != m {
		t.Errorf("ArchiConfermati con una coppia in meno: %s", esitoConferma(v))
	}

	in = ricostruzioneACM()
	in.Fonte.Analisi = valutazione.ChiaveAnalisi{}
	in.Deroghe[0].Analisi = valutazione.ChiaveAnalisi{}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != m {
		t.Errorf("chiave d'analisi vuota con l'analisi letta: %s", esitoConferma(v))
	}

	in = ricostruzioneACM()
	seconda := ancoraggio.RifNodo(shaFonteACM, "#9")
	in.Struttura.Radici = append(in.Struttura.Radici, seconda)
	in.Struttura.Nodi = append(in.Struttura.Nodi, seconda)
	in.Analisi = valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 2}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/"+valutazione.MotivoAsseRadiceNonMappata ||
		v.Conferma.RadiceMappata || v.Nomenclatura.DaDecidere != 2 {
		t.Errorf("due radici STEP dello stesso sha: %s, da decidere %d, %+v", esitoConferma(v), v.Nomenclatura.DaDecidere, v.Conferma)
	}
}

// TestRadiceDiUnAltroDocumentoODiUnAltroSha (precisazione a; R-05 della revisione di B3): la radice certificata con lo
// stesso sha ma un altro documento, o una BOM di lavoro la cui radice è di un altro sha, non si mappano.
func TestRadiceDiUnAltroDocumentoODiUnAltroSha(t *testing.T) {
	m := "da_verificare/" + valutazione.MotivoAsseRadiceNonMappata
	in := ricostruzioneACM()
	d := docAltroACM
	in.Conferma.Componenti[0].StepStrutturaleID = &d
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != m || v.Conferma.RadiceMappata {
		t.Errorf("radice certificata di un altro documento: %s", esitoConferma(v))
	}
	in = ricostruzioneACM()
	altra := ancoraggio.RifNodo(sha("b"), "#1")
	in.Struttura.Radici, in.Struttura.Nodi = []string{altra}, []string{altra}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != m || v.Conferma.RadiceMappata {
		t.Errorf("radice della BOM di lavoro di un altro sha: %s", esitoConferma(v))
	}
}

// TestRamiDellIncoerenzaDellaConferma (T-6; R-05 e R-11 della revisione di B3): ogni ramo di una conferma che la regola non
// crede dà conferma_incoerente sui due assi, nessun gesto e il criterio nessuno.
func TestRamiDellIncoerenzaDellaConferma(t *testing.T) {
	m := "da_verificare/" + valutazione.MotivoAsseConfermaIncoerente
	for nome, cambia := range map[string]func(*valutazione.IngressoVerificaConferma){
		"nessuna radice": func(in *valutazione.IngressoVerificaConferma) { in.Conferma.Componenti[0].Radice = false },
		"arco con un estremo non certificato": func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Archi = append(in.Conferma.Archi, valutazione.ArcoConfermato{PadreID: finitoACM, FiglioID: figlioACM(30), Qta: 1, Origine: "utente"})
			in.ArchiCorrenti = correntiDi(in.Conferma)
		},
		"qta non positiva": func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Archi[0].Qta = 0
			in.ArchiCorrenti = correntiDi(in.Conferma)
		},
		"autoarco": func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Archi[0].FiglioID = finitoACM
			in.ArchiCorrenti = correntiDi(in.Conferma)
		},
		"criterio fuori elenco": func(in *valutazione.IngressoVerificaConferma) { in.Conferma.Criterio = "misto" },
		"mono_pezzo con il criterio step": func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma = monoACM()
			in.Conferma.Criterio = valutazione.CriterioStep
			in.ArchiCorrenti = nil
		},
	} {
		in := ricostruzioneACM()
		cambia(&in)
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Nomenclatura) != m || asse(v.Gerarchia) != m || v.Criterio != valutazione.CriterioNessuno || v.Conferma.RadiceMappata {
			t.Errorf("%s: %s", nome, esitoConferma(v))
		}
	}
}

// ---- le condizioni nuove di un prodotto con una conferma (R-02 della revisione di B3) ----

// TestCondizioniNuoveDallaConferma (A1c T-E1-14, T-E1R-04; R-02 della revisione di B3; consegna a B3.5): l'adattatore
// delle condizioni nuove dei prodotti con una conferma parte dall'uscita della regola della conferma. La gerarchia ferma
// solo per fonte_non_confermata dopo una fonte superata è ferma per una condizione nuova; un conflitto aperto lo è; la
// deroga mancante, una riga SHOCK-05 già presente alla conferma e la struttura cambiata no (il prodotto resta
// non_pronto); una riga SHOCK-05 sopravvenuta sì (direttiva 12 §3.5, riscritta: vedi il caso). Senza conferma nessuna
// condizione nuova da qui. Le altre classi di AD-4c sono in TestCondizioniNuoveRiesameONonPronto.
func TestCondizioniNuoveDallaConferma(t *testing.T) {
	superata := valutazione.ProdottoValutato{Fonte: valutazione.FonteProdotto{Calcolata: true, Motivo: valutazione.MotivoFonteRiferimentoSuperato}}
	confermata := valutazione.ProdottoValutato{Fonte: valutazione.FonteProdotto{Calcolata: true, Stato: valutazione.FonteConfermata, Motivo: valutazione.MotivoFonteEstrazioneRiuscita}}
	nonAnalizzata := valutazione.ProdottoValutato{Fonte: valutazione.FonteProdotto{Calcolata: true, Motivo: valutazione.MotivoFonteNonAnalizzata}}
	// senza la fonte confermata la radice STEP si copre con la decisione di una persona, non con la mappatura
	senzaFonte := func() valutazione.IngressoVerificaConferma {
		in := ricostruzioneACM()
		in.Struttura = strutturaSoloPartiACM(false)
		in.DecisioniStep.Nodi = map[string]uuid.UUID{radiceStepAC: finitoACM}
		return in
	}
	conflitto := []valutazione.Conflitto{{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia,
		Rif: valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(2))), Motivo: "qta decisa contro lo STEP"}}
	for _, c := range []struct {
		nome   string
		pv     valutazione.ProdottoValutato
		in     func() valutazione.IngressoVerificaConferma
		attese valutazione.CondizioniNuove
		motivo string // il motivo della gerarchia della regola, per leggere il caso
	}{
		{"fonte superata, il resto a posto", superata, senzaFonte,
			valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}, valutazione.MotivoAsseFonteNonConfermata},
		{"fonte superata, senza deroga (senza fonte la deroga non si valuta)", superata, func() valutazione.IngressoVerificaConferma {
			in := senzaFonte()
			in.Deroghe = nil
			return in
		}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}, valutazione.MotivoAsseFonteNonConfermata},
		{"fonte non confermata e non superata", nonAnalizzata, senzaFonte,
			valutazione.CondizioniNuove{Nomenclatura: true}, valutazione.MotivoAsseFonteNonConfermata},
		{"fonte superata e struttura cambiata", superata, func() valutazione.IngressoVerificaConferma {
			in := senzaFonte()
			in.ArchiCorrenti = in.ArchiCorrenti[1:]
			return in
		}, valutazione.CondizioniNuove{}, valutazione.MotivoAsseStrutturaCambiata},
		{"fonte confermata, deroga mancante", confermata, func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.Deroghe = nil
			return in
		}, valutazione.CondizioniNuove{Nomenclatura: true}, valutazione.MotivoAsseDerogaAssente},
		{"fonte confermata, conflitto aperto di gerarchia", confermata, func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.Struttura.Conflitti = conflitto
			return in
		}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}, valutazione.MotivoAsseConflitto},
		{"fonte confermata, conflitto aperto e deroga mancante", confermata, func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.Struttura.Conflitti = conflitto
			in.Deroghe = nil
			return in
		}, valutazione.CondizioniNuove{Nomenclatura: true}, valutazione.MotivoAsseConflitto},
		// Riscritta per D12 §3.5 (T-17 superata): una riga secondaria già presente alla conferma resta un requisito che
		// manca (non_pronto); una sopravvenuta è un'evidenza nuova (da_riesaminare). Prima tutte e due davano {}
		{"fonte confermata, SHOCK-05 con una riga già presente alla conferma", confermata, func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.SecondarieAperte = []valutazione.RigaSecondariaAperta{{Rif: ancoraggio.RifNodo(sha("d"), "#7")}}
			return in
		}, valutazione.CondizioniNuove{}, valutazione.MotivoAsseDaDecidere},
		{"fonte confermata, SHOCK-05 con una riga sopravvenuta", confermata, func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.SecondarieAperte = []valutazione.RigaSecondariaAperta{{Rif: ancoraggio.RifNodo(sha("d"), "#7"), Sopravvenuta: true}}
			return in
		}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}, valutazione.MotivoAsseDaDecidere},
		{"senza conferma", confermata, func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.Conferma = nil
			return in
		}, valutazione.CondizioniNuove{}, valutazione.MotivoAsseNessunGesto},
	} {
		in := c.in()
		if v := valutazione.VerificaDallaConferma(in); v.Gerarchia.Motivo != c.motivo {
			t.Errorf("%s: la regola dà %s, atteso il motivo %s", c.nome, esitoConferma(v), c.motivo)
		}
		if got := valutazione.CondizioniNuoveDallaConfermaPerProva(c.pv, in, nil, fotorfq.Thread{}); got != c.attese {
			t.Errorf("%s: condizioni nuove %+v, attese %+v", c.nome, got, c.attese)
		}
	}

	// SHOCK-05 dopo una fonte superata, con le due versioni dello STEP nella fotografia (R111 A): le righe aperte di uno
	// STEP secondario non sono voci del nuovo STEP, quindi non diventano condizioni nuove nemmeno quando le voci della
	// struttura principale sono vuote (senza la guardia R111 le darebbe per nuove: nessuna voce vecchia)
	docNuovo, shaNuovo := uid(0x9c05), sha("e")
	pv := valutazione.ProdottoValutato{Rif: "ACM-9000", Fonte: valutazione.FonteProdotto{Calcolata: true, Motivo: valutazione.MotivoFonteRiferimentoSuperato,
		Riferimento: &ancoraggio.RiferimentoFonte{DocumentoID: docFonteACM, Sha256: shaFonteACM}}}
	th := fotorfq.Thread{Documenti: []fotorfq.DocumentoConfermato{{ID: docFonteACM, Sha256: shaFonteACM, SostituitoDa: &docNuovo}, {ID: docNuovo, Sha256: shaNuovo}}}
	strutture := []ancoraggio.StrutturaProdotto{
		{Target: pv.Rif, Sha256: shaFonteACM, Nodi: []ancoraggio.NodoProposto{{Rif: radiceStepAC}}},
		{Target: pv.Rif, Sha256: shaNuovo, Nodi: []ancoraggio.NodoProposto{{Rif: ancoraggio.RifNodo(shaNuovo, "#1")}}},
	}
	in := senzaFonte()
	if got := valutazione.CondizioniNuoveDallaConfermaPerProva(pv, in, strutture, th); got != (valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}) {
		t.Errorf("fonte superata con le due versioni, senza secondari: %+v", got)
	}
	in.SecondarieAperte = []valutazione.RigaSecondariaAperta{{Rif: ancoraggio.RifNodo(sha("d"), "#7")}}
	if got := valutazione.CondizioniNuoveDallaConfermaPerProva(pv, in, strutture, th); got != (valutazione.CondizioniNuove{}) {
		t.Errorf("fonte superata con le due versioni e SHOCK-05: %+v", got)
	}
}
