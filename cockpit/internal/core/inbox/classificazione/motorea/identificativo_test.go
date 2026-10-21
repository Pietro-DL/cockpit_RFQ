package motorea

import (
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — l'identificativo verso il cliente da codice e revisione decisi (V1, emendamento U2 §4.2 punto 4; prova A-33, la
// parte del motore): la forma del cartiglio della grammatica decide la stringa (la concatenazione per una famiglia, il
// separatore «/» per un'altra: mai una regola universale); senza una forma componibile dai due campi nessuna stringa,
// con il motivo. Le grammatiche sono quelle inventate delle prove del compositore (ACME).
func TestA33LIdentificativoDalCodiceEDallaRevisione(t *testing.T) {
	rev := grammatica.TipoParteRevisione
	t.Run("la concatenazione della forma del cartiglio", func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famMarcatore()))
		got := m.ComponiIdentificativo("9123456A", "2", "acme-marcatore")
		want := CodiceComposto{Testo: "9123456A2", Famiglia: "acme-marcatore", Forma: "cartiglio"}
		if got != want {
			t.Fatalf("%+v, atteso %+v", got, want)
		}
		if x := m.ComponiIdentificativo("9123456A", "2", ""); x != want {
			t.Errorf("senza namespace: %+v", x)
		}
		// la forma chiede la revisione: senza, nessuna stringa (R87)
		if x := m.ComponiIdentificativo("9123456A", "", "acme-marcatore"); x.Testo != "" || x.Motivo != MotivoComposizioneRevisioneNonDeterminata {
			t.Errorf("senza revisione: %+v", x)
		}
		// un altro namespace non legge il codice
		if x := m.ComponiIdentificativo("9123456A", "2", "acme-altro"); x.Testo != "" || x.Motivo != MotivoIdentificativoCodiceNonLetto {
			t.Errorf("un altro namespace: %+v", x)
		}
		// una revisione che la regola non legge (una lettera, la regola vuole una cifra): la rilettura non la ritrova
		if x := m.ComponiIdentificativo("9123456A", "B", "acme-marcatore"); x.Testo != "" || x.Motivo == "" {
			t.Errorf("una revisione fuori dalla regola: %+v", x)
		}
	})
	t.Run("il separatore della forma del cartiglio", func(t *testing.T) {
		barra := formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pSep("/"), pRif(rev, "rev", 1)), "8123456A/2"}
		m, _ := compilaBene(t, grammaticaACME(famKit(barra)))
		got := m.ComponiIdentificativo("8123456A", "2", famigliaKit)
		if got.Testo != "8123456A/2" || got.Motivo != "" {
			t.Fatalf("%+v, atteso 8123456A/2", got)
		}
	})
	t.Run("una forma senza revisione", func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famKit(formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A")), "8123456A"})))
		if x := m.ComponiIdentificativo("8123456A", "", famigliaKit); x.Testo != "8123456A" {
			t.Errorf("senza revisione decisa: %+v", x)
		}
		if x := m.ComponiIdentificativo("8123456A", "2", famigliaKit); x.Testo != "" || x.Motivo != MotivoComposizioneQualificatoreNonTrasferibile {
			t.Errorf("la revisione non ha posto: %+v", x)
		}
	})
	t.Run("nessuna forma del cartiglio", func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
		if x := m.ComponiIdentificativo("9123456X", "1", "acme-marcatore"); x.Testo != "" || x.Motivo != MotivoComposizioneFormaAssente {
			t.Errorf("la famiglia senza cartiglio: %+v", x)
		}
	})
	t.Run("due forme del cartiglio", func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famKit(cartiglioStd(),
			formaEsempio{forma("cartiglio-trattino", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pSep("-"),
				pRif(rev, "rev", 1)), "8123456A-2"})))
		if x := m.ComponiIdentificativo("8123456A", "2", famigliaKit); x.Testo != "" || x.Motivo != MotivoComposizioneFormeMultiple {
			t.Errorf("due forme: %+v", x)
		}
	})
	t.Run("un codice che la grammatica non legge", func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famMarcatore()))
		for _, c := range []string{"", "ACM-1200", "9123456A2"} {
			if x := m.ComponiIdentificativo(c, "2", ""); x.Testo != "" || x.Motivo != MotivoIdentificativoCodiceNonLetto {
				t.Errorf("%q: %+v", c, x)
			}
		}
		var nessuno *Motore
		if x := nessuno.ComponiIdentificativo("9123456A", "2", ""); x.Testo != "" || x.Motivo != MotivoIdentificativoCodiceNonLetto {
			t.Errorf("motore nullo: %+v", x)
		}
	})
}
