package domain

import "testing"

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
		{"6674611A_4.pdf", 300_000, "entrata", "disegno_2d", "6674611A", "4", true},
		{"6674611A.stp", 2_000_000, "entrata", "cad_3d", "6674611A", "", true},
		{"assieme.STEP", 2_000_000, "entrata", "cad_3d", "", "", true},
		{"Locandina.pdf", 300_000, "entrata", "altro", "", "", false},
		{"TIROCINIO_PROMATEC-SRL.pdf", 200_000, "entrata", "altro", "", "", false},
		{"SO 5467.pdf", 100_000, "uscita", "offerta_promatec", "", "", true},
		{"SO 5467.pdf", 100_000, "entrata", "altro", "", "", false},
		{"image001.png", 12_000, "entrata", "rumore", "", "", false},
		{"Screenshot_20260903_090239_Chrome.jpg", 776_662, "entrata", "rumore", "", "", false},
		{"foto_pezzo.jpg", 3_000_000, "entrata", "altro", "", "", false},
		{"disegni.zip", 5_000_000, "entrata", "altro", "", "", true},
		{"listino.xlsx", 50_000, "entrata", "commerciale", "", "", true},
		{"12-34567_REV2.dxf", 50_000, "entrata", "sviluppo_dxf", "12-34567", "2", true},
		{"Re: RFQ.msg", 50_000, "entrata", "corrispondenza", "", "", false},
	}
	for _, c := range casi {
		p := PropostaDaNome(c.nome, c.bytes, c.direzione)
		if p.Tipo != c.tipo || p.Codice != c.codice || p.Rev != c.rev || p.PreSpunta != c.spunta {
			t.Errorf("%s: got %+v, atteso tipo=%s codice=%s rev=%s spunta=%v", c.nome, p, c.tipo, c.codice, c.rev, c.spunta)
		}
		if p.Confidenza < 0 || p.Confidenza > 100 {
			t.Errorf("%s: confidenza fuori scala %d", c.nome, p.Confidenza)
		}
	}
}
