package motorea

import (
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — decorazioni, involucri, livelli, token e suffissi (A1a-DCR; piano A, par.4.4.5, 4.7.2; E-03…E-09; D2,
// D8, D10): involucro di pacchetto e involucro tecnico; livello «B1» mai revisione e senza diagnostiche di
// revisione; stato PDM fuori dall'identità; token «00» conservato e non attribuito; suffisso documento con la
// base ripetuta, concordante e discordante, e in quel caso stato discordante senza scegliere la prima.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono acme-prefisso e acme-documento di sintetiche_test.go.

// fuoriDallaBase: l'intervallo non tocca quello della base (le decorazioni non stanno mai nella base).
func fuoriDallaBase(t *testing.T, l LetturaForma, cosa string, iv evidenze.Intervallo) {
	t.Helper()
	b := l.Base.Segmenti
	if len(b) == 0 {
		t.Fatalf("lettura senza segmenti")
	}
	inizio, fine := b[0].Intervallo.Inizio, b[len(b)-1].Intervallo.Fine
	if iv.Inizio < fine && inizio < iv.Fine {
		t.Errorf("%s in %v dentro la base [%d,%d)", cosa, iv, inizio, fine)
	}
}

// decorazione: la decorazione letta con quella regola.
func decorazione(t *testing.T, l LetturaForma, regola string) DecorazioneLetta {
	t.Helper()
	for _, d := range l.Decorazioni {
		if d.Regola == regola {
			return d
		}
	}
	t.Fatalf("decorazione %q non letta: %+v", regola, l.Decorazioni)
	return DecorazioneLetta{}
}

func TestDecorazioneInvolucroDiPacchettoETecnico(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso(), famDocumento()))

	letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120100.pdf")
	l := unaLettura(t, letture, "acme-prefisso", "nome-pdf")
	d := decorazione(t, l, "pacchetto")
	if d.Tipo != grammatica.TipoDecorazioneInvolucro || d.Valore != "ACME-030" || d.Originale != "ACME-030" {
		t.Errorf("involucro di pacchetto %+v", d)
	}
	fuoriDallaBase(t, l, "involucro", d.Intervallo)

	letture, _ = riconosci(t, m, "nome_file", "97123456-ACME_DRW_2D-coda.dxf")
	l = unaLettura(t, letture, "acme-documento", "2d-dxf")
	d = decorazione(t, l, "drw2d")
	if d.Tipo != grammatica.TipoDecorazioneInvolucro || d.Valore != "ACME_DRW_2D" {
		t.Errorf("involucro tecnico %+v", d)
	}
	fuoriDallaBase(t, l, "involucro", d.Intervallo)
	if l.Base.Normalizzata != "97123456" || l.CodiceRichiesto != "97123456" {
		t.Errorf("base %q, codice richiesto %q", l.Base.Normalizzata, l.CodiceRichiesto)
	}
	if l.Originale != "97123456-ACME_DRW_2D" {
		t.Errorf("originale %q: la coda del nome resta fuori dalla lettura", l.Originale)
	}
}

// TestDecorazioneLivelloMaiRevisione: «B1» e «B2» sono un livello del nome, mai una revisione, e non danno
// diagnostiche di revisione (D8).
func TestDecorazioneLivelloMaiRevisione(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famDocumento()))
	for _, livello := range []string{"B1", "B2"} {
		letture, diag := riconosci(t, m, "nome_file", "97123456-ACME_MOD_3D-"+livello+".stp")
		l := unaLettura(t, letture, "acme-documento", "3d")
		if l.Revisione != nil {
			t.Errorf("%s: revisione letta %+v, il livello non è una revisione", livello, l.Revisione)
		}
		if len(diag) != 0 {
			t.Errorf("%s: diagnostiche sul livello:\n%s", livello, elenco(diag))
		}
		d := decorazione(t, l, "livello")
		if d.Tipo != grammatica.TipoDecorazioneLivelloNomeFile || d.Valore != livello {
			t.Errorf("%s: decorazione %+v", livello, d)
		}
		fuoriDallaBase(t, l, "livello", d.Intervallo)
		inv := decorazione(t, l, "mod3d")
		if inv.Tipo != grammatica.TipoDecorazioneInvolucro || inv.Valore != "ACME_MOD_3D" {
			t.Errorf("%s: involucro tecnico %+v", livello, inv)
		}
		if l.Stato != StatoCompleta || l.Token != nil {
			t.Errorf("%s: stato %q, token %+v", livello, l.Stato, l.Token)
		}
	}
	if letture, _ := riconosci(t, m, "nome_file", "97123456-ACME_MOD_3D-B3.stp"); len(letture) != 0 {
		t.Errorf("livello non dichiarato: nessuna lettura attesa, trovate %s", riassunto(letture))
	}
}

// TestDecorazioneStatoPDMETokenFuoriDallIdentita: nel nome con il token, lo stato PDM e il token stanno fuori
// dalla base e dal codice richiesto; il token «00» si conserva, non è una revisione e non è attribuito
// (D10, Q1).
func TestDecorazioneStatoPDMETokenFuoriDallIdentita(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
	letture, diag := riconosci(t, m, "nome_file", "ACME-030P7120100 00 IN_WORK.stp")
	l := unaLettura(t, letture, "acme-prefisso", "nome-step")
	pdm := decorazione(t, l, "pdm")
	if pdm.Tipo != grammatica.TipoDecorazioneStatoPDM || pdm.Valore != "IN_WORK" {
		t.Errorf("stato PDM %+v", pdm)
	}
	fuoriDallaBase(t, l, "stato PDM", pdm.Intervallo)
	if l.Token == nil || l.Token.Valore != "00" || l.Token.Originale != "00" {
		t.Fatalf("token %+v, atteso «00» conservato", l.Token)
	}
	fuoriDallaBase(t, l, "token", l.Token.Intervallo)
	if l.Revisione != nil {
		t.Errorf("revisione %+v: il token «00» non è una revisione (Q1)", l.Revisione)
	}
	if l.CodiceRichiesto != "P7120100" || l.Base.Normalizzata != "7120100" {
		t.Errorf("codice richiesto %q, base %q: senza involucro, token e stato PDM", l.CodiceRichiesto, l.Base.Normalizzata)
	}
	if len(conCodice(diag, CodiceRevisioneDaVerificare)) != 0 {
		t.Errorf("il token di forma non è un token di revisione:\n%s", elenco(diag))
	}
	if len(l.Decorazioni) != 2 {
		t.Errorf("decorazioni %+v, attesi involucro e stato PDM", l.Decorazioni)
	}
}

// TestDecorazioneSuffissoDocumentoConcordante: il suffisso ripete la base, che concorda; il token «1» del
// suffisso si conserva (Q1).
func TestDecorazioneSuffissoDocumentoConcordante(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famDocumento()))
	testo := "97123456#1#R97123456#.pdf"
	letture, _ := riconosci(t, m, "nome_file", testo)
	l := unaLettura(t, letture, "acme-documento", "documento")
	if l.Originale != "97123456#1#R97123456#" {
		t.Errorf("originale %q", l.Originale)
	}
	if len(l.Ripetizioni) != 1 || !l.Ripetizioni[0].Concorda || l.Ripetizioni[0].Base.Normalizzata != "97123456" {
		t.Fatalf("ripetizioni %+v, attesa una ripetizione concordante", l.Ripetizioni)
	}
	if l.Ripetizioni[0].Base.Segmenti[0].Intervallo.Inizio != 12 {
		t.Errorf("la ripetizione è nel suffisso: %+v", l.Ripetizioni[0].Base.Segmenti)
	}
	if l.Stato != StatoCompleta {
		t.Errorf("stato %q, atteso completa", l.Stato)
	}
	if l.Token == nil || l.Token.Valore != "1" {
		t.Errorf("token %+v, atteso «1» conservato", l.Token)
	}
	if l.Revisione != nil {
		t.Errorf("revisione %+v: «1» non è attribuito", l.Revisione)
	}
	d := decorazione(t, l, "doc")
	if d.Tipo != grammatica.TipoDecorazioneSuffissoDocumento {
		t.Errorf("suffisso %+v", d)
	}
	fuoriDallaBase(t, l, "suffisso", d.Intervallo)
	if l.Base.Normalizzata != "97123456" || l.CodiceRichiesto != "97123456" {
		t.Errorf("base %q, codice richiesto %q", l.Base.Normalizzata, l.CodiceRichiesto)
	}
}

// TestDecorazioneSuffissoDocumentoDiscordante: se la base ripetuta è diversa, la lettura è discordante e le
// due basi restano tutte e due, senza scegliere la prima (D2, T19).
func TestDecorazioneSuffissoDocumentoDiscordante(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famDocumento()))
	letture, _ := riconosci(t, m, "nome_file", "97123456#1#R98123456#.pdf")
	l := unaLettura(t, letture, "acme-documento", "documento")
	if l.Stato != StatoDiscordante {
		t.Errorf("stato %q, atteso discordante", l.Stato)
	}
	if len(l.Ripetizioni) != 1 || l.Ripetizioni[0].Concorda {
		t.Fatalf("ripetizioni %+v, attesa una ripetizione discordante", l.Ripetizioni)
	}
	if l.Base.Normalizzata != "97123456" || l.Ripetizioni[0].Base.Normalizzata != "98123456" {
		t.Errorf("basi %q e %q: tutte e due conservate", l.Base.Normalizzata, l.Ripetizioni[0].Base.Normalizzata)
	}
}

// TestDecorazioneFuoriDaiSelettori: una decorazione si legge solo nei selettori dichiarati: lo stesso testo
// sul corpo non dà la lettura del nome.
func TestDecorazioneFuoriDaiSelettori(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famDocumento()))
	if letture, _ := riconosci(t, m, "corpo", "97123456-ACME_MOD_3D-B1"); len(letture) != 0 {
		t.Errorf("letture fuori dai selettori: %s", riassunto(letture))
	}
}
