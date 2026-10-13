// L1 — T-6 / L1, il 3D proposto dal motore A (A1d, diagnosi del 14/10, notte del 14/10): la voce del cad_3d che la vista
// dice «manca» è da verificare quando il motore A propone uno STEP (ancorato con un candidato unico sul nodo o sul
// prodotto, o sorgente della struttura che rappresenta il componente), con lo stesso vocabolario del 2D proposto; gli
// esiti della vista che dicono qualcosa vincono sempre. La regola (EsitoConIlMotoreA), la regola della completezza con i
// candidati della riga, e la fotografia. Scene ACME inventate.
package valutazione_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli ID dei file di T-6 (uid 0x146x, sha di due cifre ripetute: liberi nelle altre scene).
var (
	aT6Step   = uid(0x1461)
	pT6Step   = uid(0x1463)
	shaT6Step = strings.Repeat("2a", 32)
)

// TestT6EsitoConIlMotoreA: la vista vince quando dice qualcosa; sulla vista che dice «manca», il candidato del motore A
// (prima l'ancorato, poi la sorgente della struttura, a parità l'allegato minore), qualunque sia l'ordine; senza
// candidati «manca».
func TestT6EsitoConIlMotoreA(t *testing.T) {
	a1, a2, a3 := uid(0x1471), uid(0x1472), uid(0x1473)
	anc := func(a uuid.UUID) valutazione.CandidatoDocumento {
		return valutazione.CandidatoDocumento{AllegatoID: a, Via: valutazione.ViaCandidatoAncorato}
	}
	sor := func(a uuid.UUID) valutazione.CandidatoDocumento {
		return valutazione.CandidatoDocumento{AllegatoID: a, Via: valutazione.ViaCandidatoSorgenteStruttura}
	}
	tutti := []valutazione.CandidatoDocumento{sor(a1), anc(a3), anc(a2)}
	for _, c := range []struct {
		nome      string
		vista     string
		candidati []valutazione.CandidatoDocumento
		atteso    string
		file      *uuid.UUID
	}{
		{"ok vince", "ok", tutti, "presente/", nil},
		{"ok in coda vince", "ok_in_coda", tutti, "presente/", nil},
		{"la deroga vince", "derogato", tutti, "presente/", nil},
		{"la proposta legacy vince", "da_confermare", tutti, "da_verificare/associazione_non_confermata", nil},
		{"il portale vince", "sul_portale", tutti, "da_verificare/sul_portale", nil},
		{"un valore sconosciuto resta da verificare", "altro", tutti, "da_verificare/", nil},
		{"manca senza candidati", "manca", nil, "manca/nessun_documento", nil},
		{"manca con l'ancorato prima della sorgente", "manca", tutti, "da_verificare/associazione_non_confermata", &a2},
		{"manca con la sola sorgente", "manca", []valutazione.CandidatoDocumento{sor(a3), sor(a1)}, "da_verificare/associazione_non_confermata", &a1},
	} {
		for _, giro := range [][]valutazione.CandidatoDocumento{c.candidati, rovesciaCandidati(c.candidati)} {
			e, m, f := valutazione.EsitoConIlMotoreA(c.vista, giro)
			if string(e)+"/"+string(m) != c.atteso || (f == nil) != (c.file == nil) || (f != nil && *f != *c.file) {
				t.Errorf("%s: %s/%s %v", c.nome, e, m, f)
			}
		}
	}
}

// rovesciaCandidati: una copia dei candidati in ordine inverso.
func rovesciaCandidati(c []valutazione.CandidatoDocumento) []valutazione.CandidatoDocumento {
	out := make([]valutazione.CandidatoDocumento, 0, len(c))
	for i := len(c) - 1; i >= 0; i-- {
		out = append(out, c[i])
	}
	return out
}

// TestT6LaRegolaDellaCompletezza: in Completezza, la voce del cad_3d con la vista «manca» e un candidato del motore A è
// da verificare con il file, calcolata in Go; con la vista «ok» resta presente, della vista; con la proposta legacy da
// confermare tiene il file della vista. Gli ingressi non cambiano.
func TestT6LaRegolaDellaCompletezza(t *testing.T) {
	aLegacy := uid(0x1474)
	riga := func(esito string, file *uuid.UUID) valutazione.RigaDelPerimetro {
		return valutazione.RigaDelPerimetro{ComponenteID: cProdotto, Codice: "7120100A", TipoComponente: "finito", DelProdotto: true,
			Fabbisogni: []valutazione.FabbisognoDellaRiga{{TipoDocumento: "cad_3d", Bloccante: true, EsitoVista: esito, CalcolataDa: valutazione.CalcolataDaVista,
				FileCandidato: file, CandidatiMotoreA: []valutazione.CandidatoDocumento{{AllegatoID: aT6Step, Via: valutazione.ViaCandidatoAncorato}}}}}
	}
	chiusa := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true}
	for _, c := range []struct {
		esito     string
		file      *uuid.UUID
		atteso    string
		calcolata string
		candidato *uuid.UUID
	}{
		{"manca", nil, "da_verificare/associazione_non_confermata", valutazione.CalcolataDaGo, &aT6Step},
		{"ok", nil, "presente/", valutazione.CalcolataDaVista, nil},
		{"da_confermare", &aLegacy, "da_verificare/associazione_non_confermata", valutazione.CalcolataDaVista, &aLegacy},
	} {
		r := riga(c.esito, c.file)
		d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa, Righe: []valutazione.RigaDelPerimetro{r}})
		v := voceDi(t, d, cProdotto, "cad_3d")
		if esitoDi(v) != c.atteso || v.CalcolataDa != c.calcolata || (v.FileCandidato == nil) != (c.candidato == nil) ||
			(v.FileCandidato != nil && *v.FileCandidato != *c.candidato) {
			t.Errorf("%s: %+v", c.esito, v)
		}
		if r.Fabbisogni[0].CalcolataDa != valutazione.CalcolataDaVista || len(r.Fabbisogni[0].CandidatiMotoreA) != 1 {
			t.Errorf("%s: l'ingresso è cambiato %+v", c.esito, r.Fabbisogni[0])
		}
	}
	// Una voce da verificare non è una mancanza certa: l'unica voce «manca» è il 2D del finito (l'invariante, senza 2D).
	d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa, Righe: []valutazione.RigaDelPerimetro{riga("manca", nil)}})
	for _, v := range d.Voci {
		if v.Esito == valutazione.EsitoManca && v.TipoDocumento != "disegno_2d" {
			t.Errorf("una mancanza certa che non c'è: %+v", v)
		}
	}
}

// TestT6IlProdottoConLoSTEPAncorato (T-6, la via ancorato): senza struttura (lo STEP senza fatti) uno STEP letto come il
// prodotto è un candidato unico a livello prodotto: la voce del cad_3d del finito, che la vista dice «manca», è da
// verificare con lo STEP, calcolata in Go, e non è un 2D. Con la proposta dello STEP scartata da una persona torna
// «manca» (T-E1R-10).
func TestT6IlProdottoConLoSTEPAncorato(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	scena := func(t *testing.T) fotorfq.Thread {
		th := scenaSenzaSTEP(t)
		conAllegato(&th, allegato(aT6Step, 45, "7120100A_1.stp", "stp", shaT6Step), nil)
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pT6Step, AllegatoID: aT6Step, Tipo: "cad_3d", Fonte: "estensione", Stato: "aperta"})
		return th
	}
	t.Run("ancorato", func(t *testing.T) {
		v := valutaConVista(t, scena(t), m)
		x := voceDi(t, documentiDi(t, v, rifProdB), cProdotto, "cad_3d")
		if esitoDi(x) != "da_verificare/associazione_non_confermata" || x.FileCandidato == nil || *x.FileCandidato != aT6Step ||
			x.CalcolataDa != valutazione.CalcolataDaGo {
			t.Errorf("la voce del cad_3d %+v", x)
		}
		for _, d := range v.Disegni {
			if d.ComponenteID == cProdotto {
				t.Errorf("lo STEP non è un 2D: %+v", d.Gruppo)
			}
		}
	})
	t.Run("lo scarto di una persona", func(t *testing.T) {
		th := scena(t)
		il := dataACME.Add(5 * time.Hour)
		p := &th.Proposte[len(th.Proposte)-1]
		p.Stato, p.DecisoDa, p.DecisoIl = "scartata", &op, &il
		if x := voceDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), cProdotto, "cad_3d"); esitoDi(x) != "manca/nessun_documento" || x.FileCandidato != nil {
			t.Errorf("lo scarto non soddisfa la voce: %+v", x)
		}
	})
}
