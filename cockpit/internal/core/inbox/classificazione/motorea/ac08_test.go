package motorea

import (
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — A-C08 pubblica (piano A, par.4.7.2; P1 §12, §10.3): una famiglia con l'etichetta obbligatoria si legge
// solo con l'etichetta, a uno spazio al più. «PN 7654321» dà la lettura con l'etichetta; il numero da solo,
// l'etichetta attaccata a una lettera e due spazi non danno nessuna lettura della famiglia; una forma senza
// l'etichetta obbligatoria non attiva la grammatica (contratto.etichetta_obbligatoria_assente). Accanto, la
// famiglia a punti con l'affisso GT in coda, attribuito con la destinazione ricambio (D3).
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono acme-etichetta e acme-etichetta-pn di sintetiche_test.go.

func TestAC08EtichettaObbligatoria(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famEtichetta(), famEtichettaPN()))
	for _, s := range []string{"corpo", "oggetto"} {
		letture, _ := riconosci(t, m, s, "PN 7654321")
		l := unaLettura(t, letture, "acme-etichetta-pn", "pn")
		if l.Etichetta == nil || l.Etichetta.Valore != "PN" {
			t.Fatalf("%s: etichetta %+v, attesa «PN»", s, l.Etichetta)
		}
		if l.Etichetta.Intervallo.Inizio != 0 {
			t.Errorf("%s: etichetta in %v", s, l.Etichetta.Intervallo)
		}
		if l.Base.Normalizzata != "7654321" || l.Base.Segmenti[0].Intervallo.Inizio != 3 {
			t.Errorf("%s: base %+v", s, l.Base)
		}
		if l.CodiceRichiesto != "7654321" {
			t.Errorf("%s: codice richiesto %q, senza etichetta", s, l.CodiceRichiesto)
		}
		if l.Originale != "PN 7654321" {
			t.Errorf("%s: originale %q, l'etichetta fa parte della lettura", s, l.Originale)
		}
	}
	if letture, _ := riconosci(t, m, "corpo", "PN7654321"); len(letture) != 1 {
		t.Errorf("«PN7654321» (zero spazi, entro spazi_max): attesa una lettura, trovate %s", riassunto(letture))
	}
	for _, testo := range []string{"7654321", "XPN 7654321", "PN  7654321", "PN 76543210", "PN 7654321", "PN\t7654321"} {
		if letture, _ := riconosci(t, m, "corpo", testo); len(letture) != 0 {
			t.Errorf("%q: nessuna lettura della famiglia attesa, trovate %s", testo, riassunto(letture))
		}
	}
}

// TestAC08FormaSenzaEtichettaObbligatoria: una seconda forma della famiglia senza l'etichetta obbligatoria è
// un errore di contratto, sia dalla porta dello snapshot sia da CompilaVerificato, che valida da sé.
func TestAC08FormaSenzaEtichettaObbligatoria(t *testing.T) {
	f := famEtichettaPN()
	f.Forme = append(f.Forme, forma("nuda", sel("corpo"), pBase()))
	f.Esempi = append(f.Esempi, positivo("e-nuda", "corpo", "1234567", grammatica.LetturaAttesa{Forma: "nuda"}))
	g := grammaticaACME(f)
	compilaErrore(t, g, grammatica.CodiceEtichettaObbligatoriaAssente)

	m, d, err := CompilaVerificato(grammatica.SnapshotRegole{ClienteID: clienteACME, Grammatica: g}, limitiACME())
	if m != nil || err == nil || len(conCodice(d, grammatica.CodiceEtichettaObbligatoriaAssente)) == 0 {
		t.Fatalf("CompilaVerificato su uno snapshot non validato: motore %v, errore %v\n%s", m != nil, err, elenco(d))
	}
}

// TestAC08AffissoInCodaConDestinazione: «12.3456.7890GT» dà la base e l'affisso GT in coda, attribuito con la
// destinazione ricambio; il codice richiesto comprende l'affisso, nell'ordine del testo.
func TestAC08AffissoInCodaConDestinazione(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famEtichetta(), famEtichettaPN()))
	letture, _ := riconosci(t, m, "corpo", "Ricambio 12.3456.7890GT, urgente")
	l := unaLettura(t, letture, "acme-etichetta", "corpo")
	if l.Base.Normalizzata != "12.3456.7890" {
		t.Errorf("base %q", l.Base.Normalizzata)
	}
	if len(l.Affissi) != 1 || l.Affissi[0].Posizione != grammatica.PosizioneSuffisso || !l.Affissi[0].Attribuito ||
		l.Affissi[0].Valore == nil || l.Affissi[0].Valore.Destinazione != grammatica.DestinazioneRicambio || l.Affissi[0].Valore.Fase != "" {
		t.Errorf("affissi %+v: atteso GT in coda, attribuito, destinazione ricambio e nessuna fase", l.Affissi)
	}
	if l.CodiceRichiesto != "12.3456.7890GT" {
		t.Errorf("codice richiesto %q", l.CodiceRichiesto)
	}
	letture, _ = riconosci(t, m, "corpo", "12.3456.7890")
	l = unaLettura(t, letture, "acme-etichetta", "corpo")
	if len(l.Affissi) != 0 || l.CodiceRichiesto != "12.3456.7890" {
		t.Errorf("senza GT: affissi %+v, codice richiesto %q", l.Affissi, l.CodiceRichiesto)
	}
}
