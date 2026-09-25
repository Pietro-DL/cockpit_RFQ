package classificazione

import (
	"strings"
	"testing"
)

// TestPropostaDaNome: la classificazione dal solo nome del file.
//
// Dal checkpoint 3R un PDF non e' piu' `disegno_2d`. Il tipo di un PDF si sa dopo averlo aperto, e
// prima si dice `da_determinare`: il nome del file e' un indizio sul CODICE, non sul contenuto.
// «1234567A.pdf» e' il disegno tanto quanto e' l'offerta del fornitore per quel pezzo.
func TestPropostaDaNome(t *testing.T) {
	casi := []struct {
		nome      string
		bytes     int64
		direzione string
		tipo      string
		codice    string
		rev       string
		spunta    bool
	}{
		{"1234567A_4.pdf", 300_000, "entrata", "da_determinare", "1234567A", "4", true},
		{"1234567A.stp", 2_000_000, "entrata", "cad_3d", "1234567A", "", true},
		{"assieme.STEP", 2_000_000, "entrata", "cad_3d", "", "", true},
		{"Locandina.pdf", 300_000, "entrata", "da_determinare", "", "", true},
		{"TIROCINIO_AZIENDA-SRL.pdf", 200_000, "entrata", "da_determinare", "", "", true},
		{"SO 5467.pdf", 100_000, "uscita", "offerta_promatec", "", "", true},
		{"SO 5467.pdf", 100_000, "entrata", "da_determinare", "", "", true},
		{"capitolato_generale.pdf", 60_000_000, "entrata", "da_determinare", "", "", false}, // troppo grande: si scarica a mano
		{"image001.png", 12_000, "entrata", "rumore", "", "", false},
		{"Screenshot_20260903_090239_Chrome.jpg", 776_662, "entrata", "rumore", "", "", false},
		{"foto_pezzo.jpg", 3_000_000, "entrata", "altro", "", "", false},
		{"disegni.zip", 5_000_000, "entrata", "altro", "", "", true},
		{"listino.xlsx", 50_000, "entrata", "commerciale", "", "", true},
		{"12-34567_REV2.dxf", 50_000, "entrata", "sviluppo_dxf", "12-34567", "2", true},
		{"Re: RFQ.msg", 50_000, "entrata", "corrispondenza", "", "", false},
		// 7C.1, P0: un nome che CONTIENE un codice non E' un codice. Prima l'intero nome diventava il
		// codice della proposta (82 caratteri in una colonna da 60: result di stage rifiutato).
		{"Offerta 12345678 per fornitura staffe zincate rev finale allegato tecnico completo.pdf", 16_658, "entrata", "da_determinare", "", "", true},
		{"AB 12345.pdf", 300_000, "entrata", "da_determinare", "", "", true},
		{"1234567A rev4 staffa sinistra.dxf", 50_000, "entrata", "sviluppo_dxf", "", "", true},
	}
	for _, c := range casi {
		p := PropostaDaNome(c.nome, c.bytes, c.direzione)
		if p.Tipo != c.tipo || p.Codice != c.codice || p.Rev != c.rev || p.PreSpunta != c.spunta {
			t.Errorf("%s: got %+v, atteso tipo=%s codice=%s rev=%s spunta=%v", c.nome, p, c.tipo, c.codice, c.rev, c.spunta)
		}
		if p.Confidenza < 0 || p.Confidenza > 100 {
			t.Errorf("%s: confidenza fuori scala %d", c.nome, p.Confidenza)
		}
		if len(p.Codice) > MaxCodice || len(p.Rev) > MaxRev {
			t.Errorf("%s: codice %q o rev %q oltre i limiti del dominio", c.nome, p.Codice, p.Rev)
		}
	}
}

// TestUnNomeLungoNonEUnCodiceMaICodiciDentroSiConservano: il difetto del banco del 20/09/2026
// (7C.1, P0). Il codice della proposta resta vuoto, e i numeri trovati nel nome non si buttano:
// stanno in CodiciNelNome, da dove finiscono nei dettagli.
func TestUnNomeLungoNonEUnCodiceMaICodiciDentroSiConservano(t *testing.T) {
	nome := "Offerta 12345678 per fornitura staffe zincate 1234567A e 1234568B rev finale allegato tecnico completo e definitivo.pdf"
	p := PropostaDaNome(nome, 16_658, "entrata")
	if p.Codice != "" {
		t.Fatalf("il nome intero e' diventato il codice: %q (%d caratteri)", p.Codice, len(p.Codice))
	}
	attesi := []string{"12345678", "1234567A", "1234568B"}
	if len(p.CodiciNelNome) != len(attesi) {
		t.Fatalf("codici nel nome = %v, attesi %v", p.CodiciNelNome, attesi)
	}
	for i, c := range attesi {
		if p.CodiciNelNome[i] != c {
			t.Errorf("codici nel nome = %v, attesi %v", p.CodiciNelNome, attesi)
		}
	}
	// un nome che E' un codice non finisce in CodiciNelNome: sta in Codice e basta
	if q := PropostaDaNome("1234567A_4.pdf", 1000, "entrata"); q.Codice != "1234567A" || len(q.CodiciNelNome) != 0 {
		t.Errorf("1234567A_4.pdf: codice=%q nel_nome=%v", q.Codice, q.CodiciNelNome)
	}
}

// I limiti che il server applica a cio' che arriva da fuori: dentro MaxCodice/MaxRev e senza spazi.
func TestCodiceERevAmmissibili(t *testing.T) {
	if !CodiceAmmissibile("1234567A") || !CodiceAmmissibile("12-34567/B") {
		t.Error("un codice normale deve essere ammissibile")
	}
	if CodiceAmmissibile("") || CodiceAmmissibile("AB 12345") || CodiceAmmissibile("A\t1") {
		t.Error("vuoto o con spazi: non ammissibile")
	}
	lungo := "1234567A" + strings.Repeat("Z", MaxCodice)
	if CodiceAmmissibile(lungo) {
		t.Errorf("%d caratteri: oltre MaxCodice (%d)", len(lungo), MaxCodice)
	}
	if !RevAmmissibile("4") || !RevAmmissibile("REV12") || RevAmmissibile("REVISIONE_02") || RevAmmissibile("a b") {
		t.Error("revisione: entro MaxRev e senza spazi")
	}
}
