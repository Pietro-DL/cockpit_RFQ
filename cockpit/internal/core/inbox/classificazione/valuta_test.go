package classificazione

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/core/registro/regole"
)

// motoreACME e' il cliente finto con la famiglia «ACME 712»: sette cifre che cominciano per 712. La regex non
// chiude con \b, come le famiglie vere: in «7120001A» trova 7120001.
func motoreACME(t *testing.T) *Motore {
	t.Helper()
	m := Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})`, Descrizione: "ACME 712", Esempio: "7120001"}}})
	if !m.HaFamiglie() {
		t.Fatal("la famiglia di prova non e' entrata nel motore")
	}
	return m
}

// dimAttesa e' cio' che una prova si aspetta da una dimensione.
type dimAttesa struct {
	valore string
	score  int
	regola string
	stato  string
}

func controllaDim(t *testing.T, caso, quale string, d Dimensione, att dimAttesa) {
	t.Helper()
	if d.Valore != att.valore || d.Score != att.score || d.Regola != att.regola || d.Stato != att.stato {
		t.Errorf("%s, %s: %q score %d regola %q stato %q; atteso %+v", caso, quale, d.Valore, d.Score, d.Regola, d.Stato, att)
	}
}

// ev costruisce un'evidenza di prova con lo score della sua regola.
func ev(regola, valore string) Evidenza { return evidenza(regola, valore, "") }

// TestComponi (Smistamento, prova 223, A5.14.3): il massimo e non la somma; `concorde` solo fra fonti
// indipendenti; `discorde` quando un altro valore ha almeno 30; a parita' di score vince la precedenza della
// tabella, e la dimensione e' comunque discorde. Nessuna aritmetica: la concordanza non alza il numero.
func TestComponi(t *testing.T) {
	// il massimo, mai la somma: tre letture da 40 restano 40
	d := Componi([]Evidenza{ev("rev_suffisso_nome", "1"), ev("pdf_metadati", "1"), ev("ext_foglio", "1")})
	if d.Score != 40 || d.Valore != "1" {
		t.Errorf("tre evidenze deboli: score %d valore %q", d.Score, d.Valore)
	}
	if d.Stato != StatoConcorde {
		t.Errorf("tre fonti indipendenti che dicono la stessa cosa: %s", d.Stato)
	}
	// il PRODUCT uguale al nome dipende dal nome: una fonte sola
	dip := ev("step_primo_product", "7120001A")
	dip.DipendeDa = "nome_file"
	d = Componi([]Evidenza{dip, ev("nome_codice_generico", "7120001A")})
	controllaDim(t, "PRODUCT uguale al nome", "codice", d, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica})
	if len(d.Evidenze) != 2 || d.Evidenze[0].Regola != "nome_codice_generico" {
		t.Errorf("le evidenze in ordine di score: %+v", d.Evidenze)
	}
	// una seconda lettura diversa con almeno 30 fa la discordanza; il vincente resta scritto
	d = Componi([]Evidenza{ev("nome_codice_generico", "7120001A"), ev("step_radice_famiglia", "7120001")})
	controllaDim(t, "radice di famiglia diversa", "codice", d, dimAttesa{"7120001", 80, "step_radice_famiglia", StatoDiscorde})
	// sotto 30 non contraddice
	d = Componi([]Evidenza{ev("nome_codice_generico", "7120001A"), ev("nome_contiene_codice", "7120012")})
	controllaDim(t, "citato nel nome", "codice", d, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica})
	// a parita' di score vince la regola che viene prima nella tabella, in qualunque ordine arrivino, e la
	// dimensione e' discorde
	for _, l := range [][]Evidenza{
		{ev("step_primo_product", "7120099"), ev("step_radice_generico", "7120010")},
		{ev("step_radice_generico", "7120010"), ev("step_primo_product", "7120099")},
	} {
		d = Componi(l)
		controllaDim(t, "parita'", "codice", d, dimAttesa{"7120010", 30, "step_radice_generico", StatoDiscorde})
	}
	// le evidenze senza valore restano e non votano; senza nessun valore la dimensione e' `nessuna`
	d = Componi([]Evidenza{ev("ext_pdf", "")})
	controllaDim(t, "PDF non letto", "tipo", d, dimAttesa{"", 0, "ext_pdf", StatoNessuna})
	if len(d.Evidenze) != 1 {
		t.Errorf("l'evidenza senza valore si perde: %+v", d.Evidenze)
	}
	d = Componi(nil)
	if d.Stato != StatoNessuna || d.Evidenze == nil || len(d.Evidenze) != 0 {
		t.Errorf("nessuna evidenza: %+v", d)
	}
	// al piu' otto evidenze (A5.14.2), le piu' forti: il numero e' scritto qui, non preso dalla costante
	if MaxEvidenze != 8 {
		t.Errorf("MaxEvidenze = %d: A5.14.2 dice al piu' 8 evidenze per dimensione", MaxEvidenze)
	}
	var tante []Evidenza
	for i := 0; i < 12; i++ {
		tante = append(tante, ev("nome_contiene_codice", ""))
	}
	tante = append(tante, ev("nome_codice_generico", "7120001"))
	d = Componi(tante)
	if len(d.Evidenze) != 8 || d.Evidenze[0].Regola != "nome_codice_generico" || d.Valore != "7120001" {
		t.Errorf("evidenze troppe: %d, prima %+v", len(d.Evidenze), d.Evidenze[0])
	}
}

// TestValutaSuiCasiGuida (Smistamento, prova 224, A5.14.2-A5.14.3): i casi dell'addendum, con valori, score,
// stati e riepilogo in colonna.
//
// Riscritta per lo Smistamento (Domanda 7 = B, 27/09): prima fissava il comportamento A, con la rev di
// «7120001A_1.stp» letto a 45 rev_step_product (il PRODUCT uguale al nome vinceva sul nome) e la radice di
// famiglia uguale al nome «7120001.stp» a 80 step_radice_famiglia. Adesso la lettura dipendente non vale piu'
// di quella da cui dipende: la rev resta 40 rev_suffisso_nome, il codice 70 nome_codice_famiglia, e la lettura
// dipendente resta fra le evidenze con lo score del nome e quello della sua regola in `score_regola`.
func TestValutaSuiCasiGuida(t *testing.T) {
	acme := motoreACME(t)
	stepLetto := json.RawMessage(`{"product_step": "7120001A_1", "struttura": {"versione": 3, "radici": ["#1"], "nodi": [{"chiave": "#1"}, {"chiave": "#2"}], "relazioni": [{"padre": "#1", "figlio": "#2"}]}}`)
	casi := []struct {
		nome                string
		in                  IngressoFile
		tipo, codice, rev   dimAttesa
		colonne             Riepilogo
		evidenzeTipo, evCod int
	}{
		{"7120001A_1.stp dal solo nome", IngressoFile{Da: DaIngest, NomeFile: "7120001A_1.stp", Bytes: 2_000_000, Direzione: "entrata", Motore: acme},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoUnica}, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica},
			dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica}, Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}, 1, 1},
		{"7120001A_1.stp letto, PRODUCT uguale al nome", IngressoFile{Da: DaAnalisi, NomeFile: "7120001A_1.stp", Direzione: "entrata", Motore: acme,
			Esito: &Esito{"cad_3d", "step"}, Fatti: stepLetto},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoConcorde}, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica},
			dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica}, Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}, 2, 2},
		// lo stesso file con una radice di famiglia diversa dal nome: discorde, e la colonna tiene il nome (D49)
		{"7120001A_1.stp con la radice 7120001 di famiglia", IngressoFile{Da: DaStruttura, NomeFile: "7120001A_1.stp", Direzione: "entrata", Motore: acme,
			Fatti: stepLetto, Radice: &Radice{Codice: "7120001", DiFamiglia: true, Famiglia: "ACME 712", Dove: "id", Testo: "7120001"}},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoConcorde}, dimAttesa{"7120001", 80, "step_radice_famiglia", StatoDiscorde},
			dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica}, Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}, 2, 2},
		// e con la rev B della famiglia: anche la rev e' discorde (65 contro 40), e con il codice discorde la
		// colonna prende dal nome anche la rev, non la piu' forte (Riepilogo, ramo dalNome)
		{"7120001A_1.stp con la radice 7120001 rev B di famiglia", IngressoFile{Da: DaStruttura, NomeFile: "7120001A_1.stp", Direzione: "entrata", Motore: acme,
			Fatti: stepLetto, Radice: &Radice{Codice: "7120001", Rev: "B", DiFamiglia: true, Famiglia: "ACME 712", Dove: "id", Testo: "7120001"}},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoConcorde}, dimAttesa{"7120001", 80, "step_radice_famiglia", StatoDiscorde},
			dimAttesa{"B", 65, "rev_famiglia_cliente", StatoDiscorde}, Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}, 2, 2},
		// il primo PRODUCT diverso dal nome (senza struttura classificata): 30 contro 45, discorde lo stesso
		{"7120001A_1.stp con un PRODUCT diverso", IngressoFile{Da: DaAnalisi, NomeFile: "7120001A_1.stp", Direzione: "entrata",
			Esito: &Esito{"cad_3d", "step"}, Fatti: json.RawMessage(`{"product_step": "7120001"}`)},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoConcorde}, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoDiscorde},
			dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica}, Riepilogo{"cad_3d", "7120001A", "1", 45, "nome_file"}, 2, 2},
		// uno STEP senza codice nel nome: la radice di famiglia e' il codice, e la colonna la dice
		{"assieme.stp con la radice di famiglia", IngressoFile{Da: DaStruttura, NomeFile: "assieme.stp", Motore: acme, Fatti: stepLetto,
			Radice: &Radice{Codice: "7120001", Rev: "B", RevDalFile: true, DiFamiglia: true, Famiglia: "ACME 712", Dove: "nome", Testo: "7120001"}},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoConcorde}, dimAttesa{"7120001", 80, "step_radice_famiglia", StatoUnica},
			dimAttesa{"B", 70, "rev_step_nodo", StatoUnica}, Riepilogo{"cad_3d", "7120001", "B", 80, "regola_cliente"}, 2, 1},
		// il cartiglio del worker e' il nome: una lettura sola, del nome, e con la famiglia vale 70
		{"7120010.pdf con i termini del cartiglio", IngressoFile{Da: DaAnalisi, NomeFile: "7120010.pdf", Direzione: "entrata", Motore: acme,
			Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA", "TOLLERANZE GENERALI"]}`)},
			dimAttesa{"disegno_2d", 75, "pdf_termini_cartiglio", StatoUnica}, dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"disegno_2d", "7120010", "", 70, "nome_file"}, 1, 1},
		{"7120010.pdf senza famiglie", IngressoFile{Da: DaAnalisi, NomeFile: "7120010.pdf", Direzione: "entrata",
			Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)},
			dimAttesa{"disegno_2d", 75, "pdf_termini_cartiglio", StatoUnica}, dimAttesa{"7120010", 45, "nome_codice_generico", StatoUnica},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"disegno_2d", "7120010", "", 45, "nome_file"}, 1, 1},
		// un nome che cita un codice: l'evidenza c'e', senza valore; in colonna niente codice
		{"Offerta 7120012 staffe.pdf", IngressoFile{Da: DaIngest, NomeFile: "Offerta 7120012 staffe.pdf", Bytes: 50_000, Direzione: "entrata"},
			dimAttesa{"", 0, "ext_pdf", StatoNessuna}, dimAttesa{"", 0, "nome_contiene_codice", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"da_determinare", "", "", 0, "estensione"}, 1, 1},
		{"archivio", IngressoFile{Da: DaArchivio, NomeFile: "disegni.zip", Bytes: 5_000_000},
			dimAttesa{"", 0, "ext_archivio", StatoNessuna}, dimAttesa{"", 0, "", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"da_determinare", "", "", 0, "estensione"}, 1, 0},
		{"SO 0001.pdf in uscita", IngressoFile{Da: DaIngest, NomeFile: "SO 0001.pdf", Bytes: 80_000, Direzione: "uscita"},
			dimAttesa{"offerta_promatec", 90, "nome_so_uscita", StatoUnica}, dimAttesa{"", 0, "", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"offerta_promatec", "", "", 90, "direzione"}, 2, 0},
		{"SO 0001.pdf in entrata", IngressoFile{Da: DaIngest, NomeFile: "SO 0001.pdf", Bytes: 80_000, Direzione: "entrata"},
			dimAttesa{"", 0, "ext_pdf", StatoNessuna}, dimAttesa{"", 0, "", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"da_determinare", "", "", 0, "estensione"}, 1, 0},
		{"immagine piccola", IngressoFile{Da: DaIngest, NomeFile: "image001.png", Bytes: 12_000, Direzione: "entrata"},
			dimAttesa{"rumore", 60, "rumore_immagine", StatoUnica}, dimAttesa{"", 0, "", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"rumore", "", "", 60, "rumore"}, 1, 0},
		// la nostra offerta arrivata in entrata e' un documento commerciale di chi la manda (era il −30)
		{"offerta in entrata", IngressoFile{Da: DaAnalisi, NomeFile: "offerta.pdf", Direzione: "entrata",
			Esito: &Esito{"offerta_promatec", "cartiglio"}, Fatti: json.RawMessage(`{"commerciale": true, "termini_trovati": ["INCOTERMS"]}`)},
			dimAttesa{"commerciale", 50, "pdf_termini_offerta_entrata", StatoUnica}, dimAttesa{"", 0, "", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"commerciale", "", "", 50, "cartiglio"}, 1, 0},
		{"PDF letto senza termini", IngressoFile{Da: DaAnalisi, NomeFile: "scansione.pdf", Direzione: "entrata",
			Esito: &Esito{"da_determinare", "estensione"}, Fatti: json.RawMessage(`{"codice_riconosciuto": "", "testo_letto": 120}`)},
			dimAttesa{"", 0, "pdf_nessun_termine", StatoNessuna}, dimAttesa{"", 0, "", StatoNessuna},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"da_determinare", "", "", 0, "estensione"}, 1, 0},
		{"PDF che non si apre", IngressoFile{Da: DaAnalisi, NomeFile: "7120010.pdf", Direzione: "entrata",
			Esito: &Esito{"da_determinare", "estensione"}, Fatti: json.RawMessage(`{"errore_pdf": "cannot open broken document"}`)},
			dimAttesa{"", 0, "pdf_illeggibile", StatoNessuna}, dimAttesa{"7120010", 45, "nome_codice_generico", StatoUnica},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"da_determinare", "7120010", "", 45, "nome_file"}, 1, 1},
		// un .igs che il worker dice «altro» (non conosce l'estensione) resta un CAD 3D: l'esito senza contenuto
		// e' l'estensione, e l'estensione qui si legge
		{"IGES", IngressoFile{Da: DaAnalisi, NomeFile: "7120011.igs", Esito: &Esito{"altro", "estensione"}, Fatti: json.RawMessage(`{}`)},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoUnica}, dimAttesa{"7120011", 45, "nome_codice_generico", StatoUnica},
			dimAttesa{"", 0, "", StatoNessuna}, Riepilogo{"cad_3d", "7120011", "", 45, "nome_file"}, 1, 1},
		// un caricamento a mano: la rev si dice e non si propone (B8.7)
		{"caricato a mano", IngressoFile{Da: DaStage, NomeFile: "7120001A_3.stp", Interno: true},
			dimAttesa{"cad_3d", 95, "ext_3d", StatoUnica}, dimAttesa{"7120001A", 45, "nome_codice_generico", StatoUnica},
			dimAttesa{"", 0, "rev_trattenuta_interno", StatoNessuna}, Riepilogo{"cad_3d", "7120001A", "", 45, "nome_file"}, 1, 1},
	}
	for _, c := range casi {
		v := Valuta(c.in)
		if v.V != VersioneValutazione || v.Tabella != TabellaPunteggi || v.Da != c.in.Da || v.Ricostruita {
			t.Errorf("%s: testata %+v", c.nome, v)
		}
		controllaDim(t, c.nome, "tipo", v.Tipo, c.tipo)
		controllaDim(t, c.nome, "codice", v.Codice, c.codice)
		controllaDim(t, c.nome, "rev", v.Rev, c.rev)
		if got := v.Riepilogo(); got != c.colonne {
			t.Errorf("%s: colonne %+v, attese %+v", c.nome, got, c.colonne)
		}
		if len(v.Tipo.Evidenze) != c.evidenzeTipo || len(v.Codice.Evidenze) != c.evCod {
			t.Errorf("%s: evidenze del tipo %d, del codice %d; attese %d e %d: %+v %+v", c.nome, len(v.Tipo.Evidenze),
				len(v.Codice.Evidenze), c.evidenzeTipo, c.evCod, v.Tipo.Evidenze, v.Codice.Evidenze)
		}
		for q, d := range map[string]Dimensione{DimTipo: v.Tipo, DimCodice: v.Codice, DimRev: v.Rev} {
			for _, e := range d.Evidenze {
				if r := Punteggi[e.Regola]; r.Dimensione != q || r.Score != e.ScoreDellaRegola() || r.Fonte != e.Fonte {
					t.Errorf("%s: evidenza %+v fuori dalla sua regola %+v", c.nome, e, r)
				}
				// solo una lettura dipendente vale meno della sua regola, e mai di piu' (Domanda 7 = B)
				if e.Score > e.ScoreDellaRegola() || (e.Score != e.ScoreDellaRegola() && e.DipendeDa == "") {
					t.Errorf("%s: evidenza %+v con uno score che non e' della sua regola", c.nome, e)
				}
			}
		}
	}
	// la radice di famiglia porta la famiglia, dove e' stata letta e il testo; uguale al nome dipende dal nome, e
	// non vale piu' del nome: il codice resta alla regola del nome, 70 perche' la famiglia riconosce il nome stesso
	v := Valuta(IngressoFile{NomeFile: "7120001.stp", Motore: acme, Radice: &Radice{Codice: "7120001", DiFamiglia: true, Famiglia: "ACME 712", Dove: "id", Testo: "7120001"}})
	if len(v.Codice.Evidenze) != 2 {
		t.Fatalf("la radice uguale al nome resta fra le evidenze: %+v", v.Codice.Evidenze)
	}
	if e := v.Codice.Evidenze[1]; e.Regola != "step_radice_famiglia" || e.Famiglia != "ACME 712" || e.Dove != "id" || e.DipendeDa != "nome_file" ||
		e.Score != 70 || e.ScoreRegola != 80 {
		t.Errorf("la radice uguale al nome: %+v", v.Codice.Evidenze)
	}
	if v.Codice.Stato != StatoUnica || v.Codice.Score != 70 || v.Codice.Regola != "nome_codice_famiglia" || v.Codice.Evidenze[0].Regola != "nome_codice_famiglia" {
		t.Errorf("uguale al nome non fa due fonti e non vale piu' del nome: %+v", v.Codice)
	}
	if r := v.Riepilogo(); r.Codice != "7120001" || r.Confidenza != 70 || r.Fonte != "nome_file" {
		t.Errorf("in colonna la lettura del nome: %+v", r)
	}
	// il nome del caso guida con la famiglia che riconosce il nome intero vale 70
	if v := Valuta(IngressoFile{NomeFile: "7120001_2.pdf", Motore: acme}); v.Codice.Regola != "nome_codice_famiglia" || v.Codice.Score != 70 ||
		v.Codice.Evidenze[0].Famiglia != "ACME 712" {
		t.Errorf("il nome di famiglia: %+v", v.Codice)
	}
	// il riepilogo di una rev discorde tiene la rev del nome; la trattenuta di un caricamento a mano e' nota
	if v := Valuta(IngressoFile{NomeFile: "7120001A_3.stp", Interno: true}); v.Trattenuta != "3" {
		t.Errorf("la rev trattenuta: %q", v.Trattenuta)
	}
}

// TestLaRevDelProductSiLeggeDalGrezzo (Smistamento, prova 77, K6, P10): con un PRODUCT senza rev l'esito del
// worker porta la rev del nome, e non e' una seconda lettura: la rev dello STEP si separa dal PRODUCT grezzo.
// Con un PRODUCT che dice un'altra rev la dimensione e' discorde, e la colonna tiene quella del nome.
func TestLaRevDelProductSiLeggeDalGrezzo(t *testing.T) {
	senza := Valuta(IngressoFile{NomeFile: "7120001A_1.stp", Esito: &Esito{"cad_3d", "step"}, Fatti: json.RawMessage(`{"product_step": "7120001A"}`)})
	controllaDim(t, "PRODUCT senza rev", "rev", senza.Rev, dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica})
	for _, e := range senza.Rev.Evidenze {
		if e.Fonte == "step" {
			t.Errorf("una rev dello STEP che lo STEP non dice: %+v", e)
		}
	}
	altra := Valuta(IngressoFile{NomeFile: "7120001A_1.stp", Esito: &Esito{"cad_3d", "step"}, Fatti: json.RawMessage(`{"product_step": "7120001A_B"}`)})
	controllaDim(t, "PRODUCT con un'altra rev", "rev", altra.Rev, dimAttesa{"B", 45, "rev_step_product", StatoDiscorde})
	if r := altra.Riepilogo(); r.Rev != "1" || r.Codice != "7120001A" {
		t.Errorf("con la rev discorde la colonna tiene il nome: %+v", r)
	}
	if !strings.Contains(altra.Rev.Evidenze[0].Testo, "7120001A_B") {
		t.Errorf("l'evidenza dice il PRODUCT grezzo: %+v", altra.Rev.Evidenze)
	}
	// il PRODUCT uguale al nome con la stessa rev dipende dal nome anche per la rev
	uguale := Valuta(IngressoFile{NomeFile: "7120001A_1.stp", Fatti: json.RawMessage(`{"product_step": "7120001A_1"}`)})
	if uguale.Rev.Stato != StatoUnica || uguale.Rev.Valore != "1" {
		t.Errorf("la stessa rev dal nome e dal PRODUCT uguale al nome: %+v", uguale.Rev)
	}
}

// TestIlCartiglioCheEIlNomeEUnaSolaEvidenza (Smistamento, prova 225, P16): l'esito del worker «cartiglio 95»
// con il codice e la rev letti dal nome del file e' UNA evidenza, del nome, con lo score del nome; la colonna
// dice `nome_file`, non `cartiglio`. E il codice dell'esito non passa a una copia con un altro nome (P15).
func TestIlCartiglioCheEIlNomeEUnaSolaEvidenza(t *testing.T) {
	fatti := json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)
	v := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "7120010_2.pdf", Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: fatti})
	if len(v.Codice.Evidenze) != 1 || v.Codice.Evidenze[0].Fonte != "nome_file" || v.Codice.Evidenze[0].Regola != "nome_codice_generico" {
		t.Errorf("il codice del «cartiglio» che era il nome: %+v", v.Codice.Evidenze)
	}
	if len(v.Rev.Evidenze) != 1 || v.Rev.Evidenze[0].Fonte != "nome_file" {
		t.Errorf("la rev del «cartiglio» che era il nome: %+v", v.Rev.Evidenze)
	}
	if r := v.Riepilogo(); r.Fonte != "nome_file" || r.Confidenza != 45 || r.Codice != "7120010" || r.Rev != "2" || r.Tipo != "disegno_2d" {
		t.Errorf("le colonne: %+v", r)
	}
	// lo stesso contenuto con un altro nome: il suo nome, non quello della prima copia
	altra := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "tavola.pdf", Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: fatti})
	if altra.Codice.Stato != StatoNessuna || altra.Riepilogo().Codice != "" || altra.Tipo.Valore != "disegno_2d" {
		t.Errorf("una copia senza codice nel nome riceve il tipo e non il codice: %+v %+v", altra.Tipo, altra.Codice)
	}
}

// TestLaStessaEvidenzaHaLoStessoScore (Smistamento, prova 226): il difetto degli otto numeri. Lo stesso nome
// letto dall'ingest, dallo stage, dall'archivio, dall'analisi (con i fatti del worker, qualunque ramo) e dai
// fatti gia' esistenti da' lo stesso score del codice; e le righe di prima, scritte con 40, 50, 80, 85, 90 o
// 95, si rileggono con lo stesso numero.
func TestLaStessaEvidenzaHaLoStessoScore(t *testing.T) {
	for _, nome := range []string{"7120001A_1.stp", "7120010.pdf", "7120010_REV2.dxf", "7120011.dwg", "7120012.docx", "7120013.zip"} {
		atteso := Valuta(IngressoFile{Da: DaIngest, NomeFile: nome, Bytes: 500_000, Direzione: "entrata"}).Codice
		if atteso.Score != 45 || atteso.Regola != "nome_codice_generico" {
			t.Errorf("%s: dal nome %+v", nome, atteso)
		}
		for _, in := range []IngressoFile{
			{Da: DaStage, NomeFile: nome, Bytes: 500_000, Direzione: "entrata"},
			{Da: DaArchivio, NomeFile: nome, Bytes: 500_000},
			{Da: DaAnalisi, NomeFile: nome, Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)},
			{Da: DaAnalisi, NomeFile: nome, Esito: &Esito{"sviluppo_dxf", "nome_file"}, Fatti: json.RawMessage(`{}`)},
			{Da: DaAnalisi, NomeFile: nome, Esito: &Esito{"da_determinare", "nome_file"}, Fatti: json.RawMessage(`{"codice_riconosciuto": "X", "testo_letto": 10}`)},
			{Da: DaFattiEsistenti, NomeFile: nome, Esito: &Esito{"altro", "estensione"}, Fatti: json.RawMessage(`{}`)},
		} {
			got := Valuta(in).Codice
			if got.Score != atteso.Score || got.Regola != atteso.Regola || got.Valore != atteso.Valore {
				t.Errorf("%s da %s: %+v, atteso %+v", nome, in.Da, got, atteso)
			}
		}
		// le righe di prima: i numeri di ciascuno scrittore, riletti
		for _, riga := range []struct {
			fonte string
			conf  int
		}{{"nome_file", 50}, {"nome_file", 90}, {"nome_file", 40}, {"nome_file", 80}, {"nome_file", 85}, {"estensione", 20}, {"estensione", 30}, {"cartiglio", 95}} {
			cod, _ := CodiceRev(strings.TrimSuffix(nome, nome[strings.LastIndex(nome, "."):]))
			v := ValutazioneDaRiga("disegno_2d", cod, "", riga.fonte, riga.conf, []byte(`{}`), nome, "")
			if v.Codice.Score != 45 {
				t.Errorf("%s, riga %s %d: score del codice %d", nome, riga.fonte, riga.conf, v.Codice.Score)
			}
		}
	}
}

// Il rumore e' un'evidenza del tipo, e mai per un file tecnico (P14, prova 118 nella parte di F4): lo stesso
// contenuto scartato come rumore per il dominio fa sparire un logo, non uno STEP ne' un PDF.
func TestIlRumoreNonToccaITecnici(t *testing.T) {
	for _, c := range []struct {
		nome   string
		rumore bool
	}{{"logo.png", true}, {"condizioni.docx", true}, {"listino.xlsx", true}, {"7120001A_1.stp", false}, {"7120010.dxf", false},
		{"7120011.dwg", false}, {"7120010.pdf", false}, {"scansione.tif", false}} {
		v := Valuta(IngressoFile{Da: DaStage, NomeFile: c.nome, Bytes: 500_000, Rumore: "rumore_hash_dominio"})
		if got := v.Tipo.Valore == "rumore"; got != c.rumore {
			t.Errorf("%s: rumore %v, atteso %v (%+v)", c.nome, got, c.rumore, v.Tipo)
		}
		if c.rumore && (v.Tipo.Regola != "rumore_hash_dominio" || v.Tipo.Score != 90 || v.Riepilogo().Fonte != "rumore") {
			t.Errorf("%s: %+v %+v", c.nome, v.Tipo, v.Riepilogo())
		}
		if !c.rumore && v.HaEvidenza("rumore_hash_dominio") {
			t.Errorf("%s: un file tecnico con l'evidenza del rumore", c.nome)
		}
	}
	// l'immagine grande e con un nome qualunque non e' rumore; quella da telefono si'
	if v := Valuta(IngressoFile{NomeFile: "foto_pezzo.jpg", Bytes: 3_000_000}); v.Tipo.Valore != "altro" {
		t.Errorf("foto grande: %+v", v.Tipo)
	}
	if v := Valuta(IngressoFile{NomeFile: "Screenshot_20260903_090239_Chrome.jpg", Bytes: 776_662}); v.Tipo.Valore != "rumore" {
		t.Errorf("foto da telefono: %+v", v.Tipo)
	}
}

// Il gesto «e' la risposta del fornitore» (A5.14.7): un'evidenza del tipo accanto alle altre. Vince su un PDF
// non letto; contro il disegno letto nel contenuto (75) perde, e il tipo e' discorde.
func TestLaRispostaDelFornitoreEUnEvidenzaDelTipo(t *testing.T) {
	v := Valuta(IngressoFile{NomeFile: "offerta 7120001.pdf", Direzione: "entrata"}).ConRispostaFornitore()
	controllaDim(t, "PDF non letto", "tipo", v.Tipo, dimAttesa{"offerta_fornitore", 70, "risposta_fornitore", StatoUnica})
	if r := v.Riepilogo(); r.Tipo != "offerta_fornitore" || r.Fonte != "direzione" || r.Confidenza != 70 || v.Da != DaRispostaFornitore {
		t.Errorf("le colonne: %+v (da %s)", r, v.Da)
	}
	disegno := Valuta(IngressoFile{NomeFile: "7120001.pdf", Esito: &Esito{"disegno_2d", "cartiglio"},
		Fatti: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)}).ConRispostaFornitore()
	controllaDim(t, "disegno rimandato", "tipo", disegno.Tipo, dimAttesa{"disegno_2d", 75, "pdf_termini_cartiglio", StatoDiscorde})
	// il gesto resta vero a ogni rilettura del file
	riletto := Valuta(IngressoFile{NomeFile: "offerta 7120001.pdf", Direzione: "entrata", RispostaFornitore: true})
	if !riletto.HaEvidenza("risposta_fornitore") || riletto.Tipo.Valore != "offerta_fornitore" {
		t.Errorf("riletto: %+v", riletto.Tipo)
	}
	// due volte il gesto: un'evidenza sola
	if d := v.ConRispostaFornitore(); len(d.Tipo.Evidenze) != len(v.Tipo.Evidenze) {
		t.Errorf("il gesto ripetuto duplica l'evidenza: %+v", d.Tipo.Evidenze)
	}
}

// ConValutazione mette la valutazione accanto ai dettagli che ci sono, senza toccarli, con l'ora; restituisce
// le colonne dal riepilogo; la rev trattenuta di un caricamento a mano resta in `rev_letta`.
func TestConValutazioneTieneIDettagliERiassume(t *testing.T) {
	v := Valuta(IngressoFile{Da: DaStage, NomeFile: "7120001A_3.stp", Interno: true})
	ora := time.Date(2026, 10, 2, 10, 14, 0, 0, time.UTC)
	dett, rp := ConValutazione(json.RawMessage(`{"bytes": 12345678901, "struttura": {"nodi": []}, "zip": "a.zip"}`), v, ora)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(dett, &m); err != nil {
		t.Fatal(err)
	}
	if string(m["bytes"]) != "12345678901" || string(m["zip"]) != `"a.zip"` || m["struttura"] == nil || string(m["rev_letta"]) != `"3"` {
		t.Errorf("dettagli: %s", dett)
	}
	letta, ok := LeggiValutazione(dett)
	if !ok || letta.CalcolataIl != "2026-10-02T10:14:00Z" || letta.Da != DaStage || !letta.StesseLetture(v) {
		t.Errorf("valutazione nei dettagli: %+v %v", letta, ok)
	}
	if rp != v.Riepilogo() || rp.Rev != "" || rp.Codice != "7120001A" {
		t.Errorf("riepilogo: %+v", rp)
	}
	if len(dett) > 2000 {
		t.Errorf("una valutazione deve restare piccola: %d byte", len(dett))
	}
	// una riga di prima, e una riga dell'operatore, si leggono con ValutazioneDellaRiga
	if v := ValutazioneDellaRiga("cad_3d", "7120001A", "", "nome_file", 90, []byte(`{}`), "7120001A.stp", ""); !v.Ricostruita {
		t.Errorf("riga di prima: %+v", v)
	}
	if v := ValutazioneDellaRiga("cad_3d", "7120001", "", "operatore", 100, dett, "7120001A_3.stp", ""); !v.Decisa() || v.Codice.Valore != "7120001" {
		t.Errorf("la riga dell'operatore e' la sua decisione anche con una valutazione accanto: %+v", v)
	}
	if v := ValutazioneDellaRiga("cad_3d", "7120001A", "", "nome_file", 45, dett, "7120001A_3.stp", ""); v.Ricostruita || v.Da != DaStage {
		t.Errorf("la valutazione salvata: %+v", v)
	}
}

// TestLaValutazioneRestaPiccola (Smistamento, A5.14.2): la sola valutazione, con l'ora, sta sotto i 900 byte
// nei casi guida con al piu' cinque letture con un testo (quante ne ha l'esempio dell'addendum), e con un nome
// che cita cinque codici, che non ripete il nome in ogni evidenza; il testo di un'evidenza si tronca a 200
// caratteri.
//
// Con sei evidenze (tre dimensioni con due letture ciascuna: lo STEP letto con il PRODUCT uguale al nome, o
// con la radice di famiglia e la sua rev) il formato di A5.14.2 fa circa 1000 byte, e l'esempio stesso
// dell'addendum, con cinque, ne fa 877: il limite di 900 non si tiene col formato, ed e' un punto
// dell'addendum da correggere, non un numero da forzare qui.
func TestLaValutazioneRestaPiccola(t *testing.T) {
	acme := motoreACME(t)
	misura := func(v Valutazione) int {
		v.CalcolataIl = "2026-10-02T10:14:00Z"
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return len(b)
	}
	for _, in := range []IngressoFile{
		{Da: DaIngest, NomeFile: "7120001A_1.stp", Bytes: 2_000_000, Direzione: "entrata", Motore: acme},
		{Da: DaStruttura, NomeFile: "7120001A_1.stp", Direzione: "entrata", Motore: acme, Fatti: json.RawMessage(`{"product_step": "7120001A_1", "struttura": {"nodi": [{"chiave": "#1"}], "relazioni": []}}`),
			Radice: &Radice{Codice: "7120001", DiFamiglia: true, Famiglia: "ACME 712", Dove: "id", Testo: "7120001"}},
		{Da: DaStruttura, NomeFile: "assieme.stp", Motore: acme, Fatti: json.RawMessage(`{"struttura": {"nodi": [{"chiave": "#1"}], "relazioni": []}}`),
			Radice: &Radice{Codice: "7120001", Rev: "B", RevDalFile: true, DiFamiglia: true, Famiglia: "ACME 712", Dove: "nome", Testo: "7120001"}},
		{Da: DaAnalisi, NomeFile: "7120010_2.pdf", Direzione: "entrata", Motore: acme, Esito: &Esito{"disegno_2d", "cartiglio"},
			Fatti: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA", "TOLLERANZE GENERALI"]}`)},
		{Da: DaStage, NomeFile: "7120001A_3.stp", Interno: true},
		{Da: DaIngest, NomeFile: "Offerta 7120012 7120013 7120014 7120015 7120016 staffe.pdf", Bytes: 50_000, Direzione: "entrata"},
	} {
		if n := misura(Valuta(in)); n > 900 {
			t.Errorf("%s: la valutazione fa %d byte, oltre i 900 di A5.14.2", in.NomeFile, n)
		}
	}
	// il nome che cita cinque codici: cinque evidenze senza valore, ciascuna col suo codice e basta (prima
	// ognuna ripeteva il nome intero, e la valutazione faceva 1389 byte)
	v := Valuta(IngressoFile{NomeFile: "Offerta 7120012 7120013 7120014 7120015 7120016 staffe.pdf"})
	if len(v.Codice.Evidenze) != 5 {
		t.Fatalf("i codici citati: %+v", v.Codice.Evidenze)
	}
	for i, e := range v.Codice.Evidenze {
		if want := fmt.Sprintf("712001%d", i+2); e.Regola != "nome_contiene_codice" || e.Testo != want || e.Valore != "" {
			t.Errorf("il codice citato %d: %+v, testo atteso %q", i, e, want)
		}
	}
	if r := ValutazioneDaRiga("da_determinare", "", "", "estensione", 0, []byte(`{}`), "Offerta 7120012 staffe.pdf", "pdf"); len(r.Codice.Evidenze) != 1 ||
		r.Codice.Evidenze[0].Testo != "7120012" {
		t.Errorf("il codice citato nella riga di prima: %+v", r.Codice.Evidenze)
	}
	// un testo lungo si tronca a 200 caratteri, in ogni evidenza
	lungo := strings.Repeat("è", 250) + "7120001A_1"
	v = Valuta(IngressoFile{NomeFile: lungo + ".stp", Radice: &Radice{Codice: "7120001", Testo: strings.Repeat("x", 500)}})
	for _, d := range []Dimensione{v.Tipo, v.Codice, v.Rev} {
		for _, e := range d.Evidenze {
			if n := len([]rune(e.Testo)); n > 200 {
				t.Errorf("%s: testo di %d caratteri", e.Regola, n)
			}
		}
	}
}
