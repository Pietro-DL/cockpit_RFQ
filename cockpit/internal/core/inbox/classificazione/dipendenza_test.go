package classificazione

import (
	"encoding/json"
	"testing"
)

// Le prove della Domanda 7 = B (27/09, generalizzata): una lettura dichiarata dipendente da un'altra
// (DipendeDa) resta registrata come evidenza, ma non vale piu' della lettura da cui dipende, non fa una
// seconda fonte concorde e non diventa la regola vincente al posto di quella (A5.14.3, C5, P16).

// TestIlProductUgualeAlNomeNonAlzaLaRev (Domanda 7 = B, il caso guida dell'utente): «7120001A_1.stp» con il
// PRODUCT «7120001A_1». La rev resta quella del nome, rev_suffisso_nome 40, e non rev_step_product 45; la
// lettura del PRODUCT resta fra le evidenze con lo score del nome e quello della sua regola in score_regola.
// Il codice resta del nome (45), e la colonna dice il nome.
func TestIlProductUgualeAlNomeNonAlzaLaRev(t *testing.T) {
	v := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "7120001A_1.stp", Direzione: "entrata", Motore: motoreACME(t),
		Esito: &Esito{"cad_3d", "step"}, Fatti: json.RawMessage(`{"product_step": "7120001A_1"}`)})
	controllaDim(t, "caso guida", "rev", v.Rev, dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica})
	controllaDim(t, "caso guida", "codice", v.Codice, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica})
	if len(v.Rev.Evidenze) != 2 {
		t.Fatalf("la rev del PRODUCT resta registrata: %+v", v.Rev.Evidenze)
	}
	if e := v.Rev.Evidenze[1]; e.Regola != "rev_step_product" || e.DipendeDa != "nome_file" || e.Score != 40 || e.ScoreRegola != 45 ||
		e.ScoreDellaRegola() != Punteggi["rev_step_product"].Score {
		t.Errorf("la rev del PRODUCT, dipendente: %+v", e)
	}
	if e := v.Codice.Evidenze[1]; e.Regola != "step_primo_product" || e.DipendeDa != "nome_file" || e.Score != 30 || e.ScoreRegola != 0 {
		t.Errorf("il codice del PRODUCT vale gia' meno del nome, e resta com'e': %+v", e)
	}
	if r := v.Riepilogo(); r != (Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}) {
		t.Errorf("colonne: %+v", r)
	}
}

// TestLaRadiceDiFamigliaUgualeAlNomeNonAlzaIlCodice (Domanda 7 = B, una dipendenza sulla dimensione del
// codice): la radice dello STEP uguale al nome, riconosciuta da una famiglia del cliente, non porta il codice
// a step_radice_famiglia 80. Con il nome che la famiglia non riconosce per intero («7120001A») il codice resta
// nome_codice_generico 45 e la rev della famiglia (65) resta a 40; con il nome che la famiglia riconosce
// («7120001») il codice resta nome_codice_famiglia 70: a parita' di score vince la lettura indipendente, anche
// se la regola dipendente viene prima nella tabella. La colonna dice il nome, con la sua confidenza.
func TestLaRadiceDiFamigliaUgualeAlNomeNonAlzaIlCodice(t *testing.T) {
	acme := motoreACME(t)
	casi := []struct {
		nome           string
		file           string
		radice         Radice
		codice, rev    dimAttesa
		colonne        Riepilogo
		scoreRadice    int
		scoreRevRadice int
	}{
		{"nome generico", "7120001A_1.stp", Radice{Codice: "7120001A", Rev: "1", DiFamiglia: true, Famiglia: "ACME 712", Dove: "id", Testo: "7120001A_1"},
			dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica}, dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica},
			Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}, 45, 40},
		{"nome di famiglia", "7120001.stp", Radice{Codice: "7120001", DiFamiglia: true, Famiglia: "ACME 712", Dove: "id", Testo: "7120001"},
			dimAttesa{"7120001", 70, "nome_codice_famiglia", StatoUnica}, dimAttesa{"", 0, "", StatoNessuna},
			Riepilogo{"cad_3d", "7120001", "", 70, "nome_file"}, 70, 0},
	}
	for _, c := range casi {
		r := c.radice
		v := Valuta(IngressoFile{Da: DaStruttura, NomeFile: c.file, Direzione: "entrata", Motore: acme, Radice: &r})
		controllaDim(t, c.nome, "codice", v.Codice, c.codice)
		controllaDim(t, c.nome, "rev", v.Rev, c.rev)
		if got := v.Riepilogo(); got != c.colonne {
			t.Errorf("%s: colonne %+v, attese %+v", c.nome, got, c.colonne)
		}
		trovata := false
		for _, e := range v.Codice.Evidenze {
			if e.Regola == "step_radice_famiglia" {
				trovata = true
				if e.DipendeDa != "nome_file" || e.Score != c.scoreRadice || e.ScoreRegola != 80 || e.Famiglia != "ACME 712" {
					t.Errorf("%s: la radice resta registrata, dipendente, con lo score del nome: %+v", c.nome, e)
				}
			}
		}
		if !trovata {
			t.Errorf("%s: la radice non e' fra le evidenze: %+v", c.nome, v.Codice.Evidenze)
		}
		for _, e := range v.Rev.Evidenze {
			if e.Regola == "rev_famiglia_cliente" && (e.DipendeDa != "nome_file" || e.Score != c.scoreRevRadice || e.ScoreRegola != 65) {
				t.Errorf("%s: la rev della radice, dipendente: %+v", c.nome, e)
			}
		}
	}
}

// TestUnaLetturaDipendenteQualunque (Domanda 7 = B, generalizzata: una dipendenza su un'altra dimensione e con
// un'altra fonte). La regola non e' del PRODUCT dello STEP: vale per ogni evidenza dichiarata DipendeDa. Sul
// tipo, termini d'offerta in entrata (50) dichiarati dipendenti dall'estensione di un foglio (40): il tipo resta
// ext_foglio 40, una fonte sola.
func TestUnaLetturaDipendenteQualunque(t *testing.T) {
	dip := ev("pdf_termini_offerta_entrata", "commerciale")
	dip.DipendeDa = "estensione"
	for _, l := range [][]Evidenza{{dip, ev("ext_foglio", "commerciale")}, {ev("ext_foglio", "commerciale"), dip}} {
		d := Componi(l)
		controllaDim(t, "tipo dipendente dall'estensione", "tipo", d, dimAttesa{"commerciale", 40, "ext_foglio", StatoUnica})
		if len(d.Evidenze) != 2 || d.Evidenze[1].Regola != "pdf_termini_offerta_entrata" || d.Evidenze[1].Score != 40 || d.Evidenze[1].ScoreRegola != 50 {
			t.Errorf("la lettura dipendente resta, con lo score della sua fonte: %+v", d.Evidenze)
		}
	}
	// la stessa lettura senza la dipendenza e' una seconda fonte e vince con la sua regola
	d := Componi([]Evidenza{ev("pdf_termini_offerta_entrata", "commerciale"), ev("ext_foglio", "commerciale")})
	controllaDim(t, "tipo indipendente", "tipo", d, dimAttesa{"commerciale", 50, "pdf_termini_offerta_entrata", StatoConcorde})

	// una lettura dipendente da una fonte che nella dimensione non c'e' non vale niente da sola
	orfana := ev("step_radice_famiglia", "7120001")
	orfana.DipendeDa = "nome_file"
	d = Componi([]Evidenza{orfana, ev("step_primo_product", "7120099")})
	controllaDim(t, "dipendente senza la sua fonte", "codice", d, dimAttesa{"7120099", 30, "step_primo_product", StatoUnica})
	if e := d.Evidenze[1]; e.Regola != "step_radice_famiglia" || e.Score != 0 || e.ScoreRegola != 80 {
		t.Errorf("la dipendente senza fonte: %+v", e)
	}
	// una lettura del nome con un ALTRO valore non e' la sua fonte: la dipendente non si appoggia a lei
	d = Componi([]Evidenza{orfana, ev("nome_codice_generico", "7120001A")})
	controllaDim(t, "fonte con un altro valore", "codice", d, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica})

	// ricomporre le evidenze gia' limitate da' la stessa dimensione (ConRispostaFornitore, le righe v1)
	dipRev := ev("rev_step_product", "1")
	dipRev.DipendeDa = "nome_file"
	prima := Componi([]Evidenza{dipRev, ev("rev_suffisso_nome", "1")})
	dopo := Componi(prima.Evidenze)
	a, _ := json.Marshal(prima)
	b, _ := json.Marshal(dopo)
	if string(a) != string(b) {
		t.Errorf("ricomporre cambia la dimensione:\n%s\n%s", a, b)
	}
	// se la fonte vale di piu' della regola dipendente, la dipendente resta alla sua regola
	dipBassa := ev("step_primo_product", "7120001")
	dipBassa.DipendeDa = "nome_file"
	d = Componi([]Evidenza{dipBassa, ev("nome_codice_famiglia", "7120001")})
	if e := d.Evidenze[1]; e.Score != 30 || e.ScoreRegola != 0 {
		t.Errorf("una dipendente che vale gia' meno della fonte: %+v", e)
	}
}

// TestUnaLetturaIndipendenteConcordeContaDue (Domanda 7 = B, la controparte): una lettura del contenuto che
// NON dipende dal nome e dice lo stesso codice e' una seconda fonte: la dimensione e' concorde e vince la
// regola piu' forte, anche se non e' quella del nome (il codice di famiglia nel testo del PDF, F9).
func TestUnaLetturaIndipendenteConcordeContaDue(t *testing.T) {
	for _, l := range [][]Evidenza{
		{ev("nome_codice_generico", "7120010"), ev("pdf_testo_famiglia", "7120010")},
		{ev("pdf_testo_famiglia", "7120010"), ev("nome_codice_generico", "7120010")},
	} {
		d := Componi(l)
		controllaDim(t, "indipendente concorde", "codice", d, dimAttesa{"7120010", 85, "pdf_testo_famiglia", StatoConcorde})
		for _, e := range d.Evidenze {
			if e.ScoreRegola != 0 || e.Score != Punteggi[e.Regola].Score {
				t.Errorf("una lettura indipendente tiene lo score della sua regola: %+v", e)
			}
		}
	}
	// la stessa radice di famiglia senza la dipendenza (un nome diverso che non e' un codice, o la radice letta
	// in un altro file) vince con la sua regola; dichiarata dipendente no
	d := Componi([]Evidenza{ev("nome_codice_generico", "7120001"), ev("step_radice_famiglia", "7120001")})
	controllaDim(t, "radice indipendente", "codice", d, dimAttesa{"7120001", 80, "step_radice_famiglia", StatoConcorde})
	dip := ev("step_radice_famiglia", "7120001")
	dip.DipendeDa = "nome_file"
	d = Componi([]Evidenza{ev("nome_codice_generico", "7120001"), dip})
	controllaDim(t, "radice dipendente", "codice", d, dimAttesa{"7120001", 45, "nome_codice_generico", StatoUnica})
}

// TestUnaRigaV1SiRileggeConLaDomanda7 (Domanda 7 = B, le righe scritte prima): una valutazione v1 salvata con
// la rev vinta dal PRODUCT uguale al nome (45 rev_step_product) e il codice vinto dalla radice di famiglia
// uguale al nome (80) si legge v2, con le dimensioni ricomposte: rev 40 del nome, codice 70 del nome. Le
// evidenze restano le sue; una dimensione decisa da una persona non si ricompone.
func TestUnaRigaV1SiRileggeConLaDomanda7(t *testing.T) {
	radice := evidenza("step_radice_famiglia", "7120001", "7120001")
	radice.DipendeDa, radice.Famiglia = "nome_file", "ACME 712"
	nome := evidenza("nome_codice_famiglia", "7120001", "7120001_1.stp")
	nome.Famiglia = "ACME 712"
	prod := evidenza("rev_step_product", "1", "PRODUCT('7120001_1')")
	prod.DipendeDa = "nome_file"
	v1 := Valutazione{V: 1, Tabella: TabellaPunteggi, Da: DaStruttura,
		Tipo:   Dimensione{Valore: "cad_3d", Score: 95, Regola: "ext_3d", Stato: StatoUnica, Evidenze: []Evidenza{evidenza("ext_3d", "cad_3d", ".stp")}},
		Codice: Dimensione{Valore: "7120001", Score: 80, Regola: "step_radice_famiglia", Stato: StatoUnica, Evidenze: []Evidenza{radice, nome}},
		Rev:    Dimensione{Valore: "1", Score: 45, Regola: "rev_step_product", Stato: StatoUnica, Evidenze: []Evidenza{prod, evidenza("rev_suffisso_nome", "1", "_1")}},
	}
	b, _ := json.Marshal(map[string]any{"valutazione": v1, "product_step": "7120001_1"})
	v, ok := LeggiValutazione(b)
	if !ok || v.V != VersioneValutazione || v.Ricostruita || v.Da != DaStruttura {
		t.Fatalf("la riga v1 si legge: %+v %v", v, ok)
	}
	controllaDim(t, "riga v1", "codice", v.Codice, dimAttesa{"7120001", 70, "nome_codice_famiglia", StatoUnica})
	controllaDim(t, "riga v1", "rev", v.Rev, dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica})
	controllaDim(t, "riga v1", "tipo", v.Tipo, dimAttesa{"cad_3d", 95, "ext_3d", StatoUnica})
	if len(v.Codice.Evidenze) != 2 || len(v.Rev.Evidenze) != 2 {
		t.Errorf("le evidenze della riga restano: %+v %+v", v.Codice.Evidenze, v.Rev.Evidenze)
	}
	if r := v.Riepilogo(); r.Confidenza != 70 || r.Fonte != "nome_file" || r.Rev != "1" {
		t.Errorf("il riepilogo della riga v1 riletta: %+v", r)
	}
	// ValutazioneDellaRiga passa di qui: la schermata mostra la riga v1 con la regola nuova
	vr := ValutazioneDellaRiga("cad_3d", "7120001", "1", "regola_cliente", 80, b, "7120001_1.stp", "stp")
	if vr.Codice.Regola != "nome_codice_famiglia" || vr.Rev.Regola != "rev_suffisso_nome" || vr.Ricostruita {
		t.Errorf("ValutazioneDellaRiga sulla riga v1: %+v %+v", vr.Codice, vr.Rev)
	}
}
