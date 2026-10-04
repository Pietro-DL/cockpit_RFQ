package motorea

import (
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — A-C06 pubblica (piano A, par.4.7.2; P1 §12, §5.3; D2): un affisso si toglie solo nei selettori
// dichiarati, e solo lì ha il suo significato. Sul corpo e sulla storia (R48 A) la P è riconosciuta e
// attribuita con la fase prototipo; nel nome sta dopo l'involucro; nel cartiglio non c'è, e una P lì toglie
// la lettura; nel corpo un involucro davanti alla P toglie la lettura: nessuna rimozione globale di prefissi.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. La grammatica è acme-prefisso di sintetiche_test.go.

func TestAC06AffissoSoloNeiSelettoriDichiarati(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))

	for _, s := range []string{"corpo", "storia"} {
		t.Run(s, func(t *testing.T) {
			letture, _ := riconosci(t, m, s, "P7120100")
			l := unaLettura(t, letture, "acme-prefisso", "mail")
			if l.Base.Normalizzata != "7120100" || l.Base.Originale != "7120100" {
				t.Errorf("base %+v: la P non entra nella base", l.Base)
			}
			if l.CodiceRichiesto != "P7120100" {
				t.Errorf("codice richiesto %q, atteso «P7120100» (affissi e base)", l.CodiceRichiesto)
			}
			if len(l.Affissi) != 1 {
				t.Fatalf("affissi %+v", l.Affissi)
			}
			a := l.Affissi[0]
			if a.Regola != "P" || a.Originale != "P" || a.Posizione != grammatica.PosizionePrefisso || !a.Attribuito {
				t.Errorf("affisso %+v: atteso P prefisso, attribuito", a)
			}
			if a.Valore == nil || a.Valore.Fase != grammatica.FasePrototipo || a.Valore.Destinazione != "" {
				t.Errorf("valore %+v, attesa la sola fase prototipo", a.Valore)
			}
			if a.Intervallo.Inizio != 0 || a.Intervallo.Fine != 1 {
				t.Errorf("intervallo dell'affisso %v", a.Intervallo)
			}
			if l.Stato != StatoCompleta || !l.Base.Completa {
				t.Errorf("stato %q, completa %v", l.Stato, l.Base.Completa)
			}
		})
	}

	t.Run("nome", func(t *testing.T) {
		letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120100.pdf")
		l := unaLettura(t, letture, "acme-prefisso", "nome-pdf")
		if l.Base.Normalizzata != "7120100" || len(l.Affissi) != 1 || !l.Affissi[0].Attribuito {
			t.Errorf("base %q, affissi %+v", l.Base.Normalizzata, l.Affissi)
		}
		if len(l.Decorazioni) != 1 || l.Decorazioni[0].Tipo != grammatica.TipoDecorazioneInvolucro || l.Decorazioni[0].Valore != "ACME-030" {
			t.Errorf("decorazioni %+v, atteso l'involucro", l.Decorazioni)
		}
		if l.CodiceRichiesto != "P7120100" {
			t.Errorf("codice richiesto %q: senza involucro (D2)", l.CodiceRichiesto)
		}
	})

	t.Run("cartiglio con la P", func(t *testing.T) {
		if letture, _ := riconosci(t, m, "cartiglio.codice", "P7120100"); len(letture) != 0 {
			t.Errorf("fuori dal selettore la P non si toglie: nessuna lettura attesa, trovate %s", riassunto(letture))
		}
	})

	t.Run("cartiglio senza la P", func(t *testing.T) {
		letture, _ := riconosci(t, m, "cartiglio.codice", "7120100")
		l := unaLettura(t, letture, "acme-prefisso", "cartiglio")
		if l.Base.Normalizzata != "7120100" || len(l.Affissi) != 0 {
			t.Errorf("base %q, affissi %+v: base senza fase", l.Base.Normalizzata, l.Affissi)
		}
		if l.CodiceRichiesto != "7120100" {
			t.Errorf("codice richiesto %q", l.CodiceRichiesto)
		}
	})

	t.Run("corpo con l'involucro", func(t *testing.T) {
		if letture, _ := riconosci(t, m, "corpo", "ACME-030P7120100"); len(letture) != 0 {
			t.Errorf("nessuna rimozione globale di prefissi: nessuna lettura attesa, trovate %s", riassunto(letture))
		}
	})

	t.Run("corpo senza la P", func(t *testing.T) {
		if letture, _ := riconosci(t, m, "corpo", "7120100"); len(letture) != 0 {
			t.Errorf("la forma della mail vuole la P: nessuna lettura attesa, trovate %s", riassunto(letture))
		}
	})
}

// TestAC06AttribuzioneSoloDoveDichiarata: un affisso riconosciuto su un selettore fuori dalla sua
// attribuzione si legge, ma non è attribuito e non ha valore (P1 §5.3: attribuzione ⊆ riconoscimento).
func TestAC06AttribuzioneSoloDoveDichiarata(t *testing.T) {
	f := famPrefisso()
	f.Affissi[0].Attribuzione = sel("corpo", "nome_file")
	m, _ := compilaBene(t, grammaticaACME(f))

	letture, _ := riconosci(t, m, "storia", "P7120100")
	l := unaLettura(t, letture, "acme-prefisso", "mail")
	if len(l.Affissi) != 1 || l.Affissi[0].Attribuito || l.Affissi[0].Valore != nil {
		t.Errorf("affissi %+v: sulla storia la P è riconosciuta ma non attribuita, senza valore", l.Affissi)
	}
	if l.CodiceRichiesto != "P7120100" {
		t.Errorf("codice richiesto %q: l'affisso riconosciuto resta nel codice richiesto", l.CodiceRichiesto)
	}

	letture, _ = riconosci(t, m, "corpo", "P7120100")
	l = unaLettura(t, letture, "acme-prefisso", "mail")
	if len(l.Affissi) != 1 || !l.Affissi[0].Attribuito || l.Affissi[0].Valore == nil {
		t.Errorf("affissi %+v: sul corpo la P è attribuita", l.Affissi)
	}
}

// TestAC06AffissoNonAttribuito: l'affisso S è riconosciuto e mai attribuito, su nessun selettore (D5): resta
// nel codice richiesto, senza valore.
func TestAC06AffissoNonAttribuito(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	for _, s := range []string{"corpo", "oggetto"} {
		letture, _ := riconosci(t, m, s, "S9.123.4567.3")
		l := unaLettura(t, letture, "acme-punti", "completa")
		if len(l.Affissi) != 1 || l.Affissi[0].Originale != "S" || l.Affissi[0].Attribuito || l.Affissi[0].Valore != nil {
			t.Errorf("%s: affissi %+v, atteso S non attribuito", s, l.Affissi)
		}
		if l.CodiceRichiesto != "S9.123.4567.3" || l.Base.Normalizzata != "9.123.4567.3" {
			t.Errorf("%s: codice richiesto %q, base %q", s, l.CodiceRichiesto, l.Base.Normalizzata)
		}
	}
}
