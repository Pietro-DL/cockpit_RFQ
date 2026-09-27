package classificazione

import (
	"encoding/json"
	"testing"
)

// TestValutazioneDaRiga (Smistamento, prova 227, A5.14.2): le righe di documento_proposta scritte prima della
// valutazione si rileggono per dimensione. I casi sono le combinazioni di colonne che il dump di
// cockpit_dev contiene (tipo × fonte × codice presente × dettagli del worker), con nomi finti.
func TestValutazioneDaRiga(t *testing.T) {
	type dim struct {
		valore string
		score  int
		regola string
		stato  string
	}
	casi := []struct {
		nome                       string
		tipo, codice, rev, fonte   string
		conf                       int
		dettagli                   string
		file                       string
		tipoAtt, codiceAtt, revAtt dim
	}{
		{"PDF col codice nel nome", "da_determinare", "7120001A", "1", "nome_file", 50, `{}`, "7120001A_1.pdf",
			dim{"", 0, "ext_pdf", StatoNessuna}, dim{"7120001A", 45, "nome_codice_generico", StatoUnica}, dim{"1", 40, "rev_suffisso_nome", StatoUnica}},
		{"PDF anonimo", "da_determinare", "", "", "estensione", 40, `{}`, "Capitolato generale.pdf",
			dim{"", 0, "ext_pdf", StatoNessuna}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"altro", "altro", "", "", "estensione", 20, `{}`, "foto pezzo.docx",
			dim{"altro", 20, "ext_altro", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"posta", "corrispondenza", "", "", "estensione", 60, `{}`, "Re RFQ.msg",
			dim{"corrispondenza", 90, "ext_posta", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"nostra offerta in uscita", "offerta_promatec", "", "", "direzione", 90, `{}`, "SO 0001.pdf",
			dim{"offerta_promatec", 90, "nome_so_uscita", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"STEP dal nome", "cad_3d", "7120001A", "1", "nome_file", 90, `{}`, "7120001A_1.stp",
			dim{"cad_3d", 95, "ext_3d", StatoUnica}, dim{"7120001A", 45, "nome_codice_generico", StatoUnica}, dim{"1", 40, "rev_suffisso_nome", StatoUnica}},
		{"foglio", "commerciale", "", "", "estensione", 50, `{}`, "listino.xlsx",
			dim{"commerciale", 40, "ext_foglio", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		// il worker l'ha letto e non si apre (`errore_pdf`): non e' «non ancora letto» (giro di correzione di F3)
		{"PDF che non si apre", "da_determinare", "", "", "estensione", 20, `{"errore_pdf": "cannot open broken document"}`, "Capitolato rotto.pdf",
			dim{"", 0, "pdf_illeggibile", StatoNessuna}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"PDF che non si apre, col codice nel nome", "da_determinare", "", "", "estensione", 20, `{"errore_pdf": "x"}`, "7120010.pdf",
			dim{"", 0, "pdf_illeggibile", StatoNessuna}, dim{"7120010", 45, "nome_codice_generico", StatoUnica}, dim{"", 0, "", StatoNessuna}},
		{"PDF letto senza termini", "da_determinare", "", "", "estensione", 30, `{"codice_riconosciuto": "", "testo_letto": 120}`, "scansione.pdf",
			dim{"", 0, "pdf_nessun_termine", StatoNessuna}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		// il codice di un tipo `altro` era scritto con la fonte `estensione`: e' una lettura del nome
		{"altro col codice", "altro", "7120001", "", "estensione", 20, `{}`, "7120001.docx",
			dim{"altro", 20, "ext_altro", StatoUnica}, dim{"7120001", 45, "nome_codice_generico", StatoUnica}, dim{"", 0, "", StatoNessuna}},
		// il PRODUCT uguale al nome dipende dal nome: una fonte sola, e vince il nome (45 contro 30)
		{"STEP col PRODUCT uguale al nome", "cad_3d", "7120001A", "1", "step", 95, `{"product_step": "7120001A_1", "struttura": {"nodi": []}}`, "7120001A_1.stp",
			dim{"cad_3d", 95, "ext_3d", StatoConcorde}, dim{"7120001A", 45, "nome_codice_generico", StatoUnica}, dim{"1", 40, "rev_suffisso_nome", StatoUnica}},
		{"STEP col PRODUCT diverso", "cad_3d", "7120099", "", "step", 95, `{"product_step": "7120099", "struttura": {"nodi": []}}`, "7120001A_1.stp",
			dim{"cad_3d", 95, "ext_3d", StatoConcorde}, dim{"7120001A", 45, "nome_codice_generico", StatoDiscorde}, dim{"1", 40, "rev_suffisso_nome", StatoUnica}},
		{"PDF letto col codice nel nome", "da_determinare", "7120010", "", "nome_file", 40, `{"codice_riconosciuto": "7120010", "testo_letto": 900}`, "7120010.pdf",
			dim{"", 0, "pdf_nessun_termine", StatoNessuna}, dim{"7120010", 45, "nome_codice_generico", StatoUnica}, dim{"", 0, "", StatoNessuna}},
		{"DXF con la rev esplicita", "sviluppo_dxf", "7120010", "2", "nome_file", 90, `{}`, "7120010_REV2.dxf",
			dim{"sviluppo_dxf", 80, "ext_dxf", StatoUnica}, dim{"7120010", 45, "nome_codice_generico", StatoUnica}, dim{"2", 55, "rev_esplicita_nome", StatoUnica}},
		{"nome nella forma del NAS", "cad_3d", "7120010", "B", "nome_file", 90, `{}`, "7120010_REV_B.stp",
			dim{"cad_3d", 95, "ext_3d", StatoUnica}, dim{"7120010", 45, "nome_codice_generico", StatoUnica}, dim{"B", 80, "rev_nas", StatoUnica}},
		{"DWG", "disegno_2d", "", "", "estensione", 70, `{}`, "tavola.dwg",
			dim{"disegno_2d", 85, "ext_dwg", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		// la D16: la radice di famiglia diversa dal nome vince, ma dichiarata discorde (A5.14.3, esempio di §3.3)
		{"radice di famiglia diversa dal nome", "cad_3d", "7120001", "", "regola_cliente", 80,
			`{"famiglia": "ACME 712", "dove": "nome", "testo": "7120001", "struttura": {"nodi": []}}`, "7120001A_1.stp",
			dim{"cad_3d", 95, "ext_3d", StatoConcorde}, dim{"7120001", 80, "step_radice_famiglia", StatoDiscorde}, dim{"1", 40, "rev_suffisso_nome", StatoUnica}},
		{"rumore dall'ingest", "rumore", "", "", "rumore", 70, `{}`, "image001.png",
			dim{"rumore", 60, "rumore_immagine", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"rumore gia' scartato per il dominio", "rumore", "", "", "rumore", 95, `{}`, "logo.png",
			dim{"rumore", 90, "rumore_hash_dominio", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		// P16: il «cartiglio» del worker col codice del nome e' una lettura sola, del nome
		{"cartiglio col codice del nome", "disegno_2d", "7120010", "", "cartiglio", 95, `{"cartiglio": true, "termini_trovati": ["SCALA", "TOLLERANZE GENERALI"]}`, "7120010.pdf",
			dim{"disegno_2d", 75, "pdf_termini_cartiglio", StatoUnica}, dim{"7120010", 45, "nome_codice_generico", StatoUnica}, dim{"", 0, "", StatoNessuna}},
		{"file caricato a mano: la rev non e' del cliente", "cad_3d", "7120001A", "", "nome_file", 90, `{"rev_letta": "3"}`, "7120001A_3.stp",
			dim{"cad_3d", 95, "ext_3d", StatoUnica}, dim{"7120001A", 45, "nome_codice_generico", StatoUnica}, dim{"", 0, "rev_trattenuta_interno", StatoNessuna}},
		// assegnata a un componente con un altro codice: la lettura resta quella messa da parte
		{"assegnata, il file dice altro", "disegno_2d", "7120001", "", "cartiglio", 95, `{"codice_letto": "7120001A", "termini_trovati": ["SCALA"]}`, "7120001A.pdf",
			dim{"disegno_2d", 75, "pdf_termini_cartiglio", StatoUnica}, dim{"7120001A", 45, "nome_codice_generico", StatoUnica}, dim{"", 0, "", StatoNessuna}},
		{"un nome che cita un codice", "da_determinare", "", "", "estensione", 40, `{"codici_nel_nome": ["7120012"]}`, "Offerta 7120012 staffe.pdf",
			dim{"", 0, "ext_pdf", StatoNessuna}, dim{"", 0, "nome_contiene_codice", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"risposta del fornitore", "offerta_fornitore", "", "", "estensione", 40, `{}`, "offerta.pdf",
			dim{"offerta_fornitore", 70, "risposta_fornitore", StatoUnica}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
		{"archivio", "altro", "", "", "estensione", 20, `{}`, "disegni.zip",
			dim{"", 0, "ext_archivio", StatoNessuna}, dim{"", 0, "", StatoNessuna}, dim{"", 0, "", StatoNessuna}},
	}
	controlla := func(caso, quale string, d Dimensione, att dim) {
		t.Helper()
		if d.Valore != att.valore || d.Score != att.score || d.Regola != att.regola || d.Stato != att.stato {
			t.Errorf("%s, %s: %q score %d regola %q stato %q; atteso %+v", caso, quale, d.Valore, d.Score, d.Regola, d.Stato, att)
		}
	}
	for _, c := range casi {
		v := ValutazioneDaRiga(c.tipo, c.codice, c.rev, c.fonte, c.conf, []byte(c.dettagli), c.file, "")
		if !v.Ricostruita || v.V != VersioneValutazione || v.Tabella != TabellaPunteggi || v.Decisa() {
			t.Errorf("%s: ricostruita %v, v %d, tabella %q, decisa %v", c.nome, v.Ricostruita, v.V, v.Tabella, v.Decisa())
		}
		controlla(c.nome, "tipo", v.Tipo, c.tipoAtt)
		controlla(c.nome, "codice", v.Codice, c.codiceAtt)
		controlla(c.nome, "rev", v.Rev, c.revAtt)
	}

	// P16: una sola evidenza, del nome, anche se la colonna diceva «cartiglio 95»
	v := ValutazioneDaRiga("disegno_2d", "7120010", "", "cartiglio", 95, []byte(`{"termini_trovati": ["SCALA"]}`), "7120010.pdf", "pdf")
	if len(v.Codice.Evidenze) != 1 || v.Codice.Evidenze[0].Fonte != "nome_file" {
		t.Errorf("il codice del cartiglio che era il nome e' una lettura del nome: %+v", v.Codice.Evidenze)
	}
	// la concordanza non alza il numero, la dipendenza non fa una seconda fonte
	v = ValutazioneDaRiga("cad_3d", "7120001A", "1", "step", 95, []byte(`{"product_step": "7120001A_1"}`), "7120001A_1.stp", "stp")
	if len(v.Codice.Evidenze) != 2 || v.Codice.Evidenze[1].DipendeDa != "nome_file" || v.Codice.Score != 45 {
		t.Errorf("il PRODUCT uguale al nome dipende dal nome: %+v", v.Codice)
	}
	if v.Tipo.Score != 95 || len(v.Tipo.Evidenze) != 2 {
		t.Errorf("due fonti del tipo, lo score resta il massimo: %+v", v.Tipo)
	}
	// una decisione non ha score da mostrare
	v = ValutazioneDaRiga("disegno_2d", "7120010", "B", "operatore", 100, []byte(`{}`), "qualunque.pdf", "pdf")
	if !v.Decisa() || v.Tipo.Valore != "disegno_2d" || v.Codice.Valore != "7120010" || v.Rev.Valore != "B" || v.Codice.Regola != RegolaOperatore {
		t.Errorf("la riga dell'operatore riporta la decisione: %+v", v)
	}
}

// TestUnCodiceNonHaMaiFonteEstensione (prova 227, seconda parte): su tutte le combinazioni di tipo, fonte,
// codice presente e nome, nessuna evidenza del codice o della rev ha la fonte `estensione`, ogni evidenza ha
// lo score e la fonte della sua regola (la stessa evidenza, lo stesso numero) e la riga resta ricostruita.
func TestUnCodiceNonHaMaiFonteEstensione(t *testing.T) {
	tipi := []string{"da_determinare", "altro", "cad_3d", "disegno_2d", "sviluppo_dxf", "commerciale", "offerta_promatec",
		"offerta_fornitore", "corrispondenza", "capitolato", "distinta_cliente", "rumore"}
	fonti := []string{"estensione", "nome_file", "step", "cartiglio", "regola_cliente", "direzione", "rumore"}
	nomi := []string{"7120001A_1.stp", "7120001.pdf", "7120001.docx", "Offerta 7120012 staffe.pdf", "listino.xlsx", "disegni.zip", "image001.png", "7120010_REV2.dxf"}
	dettagli := []string{`{}`, `{"product_step": "7120099", "struttura": {}}`, `{"termini_trovati": ["SCALA"]}`, `{"testo_letto": 10}`,
		`{"famiglia": "ACME 712"}`, `non json`}
	n := 0
	for _, tipo := range tipi {
		for _, fonte := range fonti {
			for _, nome := range nomi {
				for _, cod := range []string{"", "7120001"} {
					for _, det := range dettagli {
						v := ValutazioneDaRiga(tipo, cod, "", fonte, 50, []byte(det), nome, "")
						n++
						if !v.Ricostruita {
							t.Fatalf("%s %s %s: non ricostruita", tipo, fonte, nome)
						}
						for q, d := range map[string]Dimensione{"tipo": v.Tipo, "codice": v.Codice, "rev": v.Rev} {
							if d.Evidenze == nil {
								t.Errorf("%s %s %s: %s senza elenco di evidenze", tipo, fonte, nome, q)
							}
							for _, e := range d.Evidenze {
								r, ok := Punteggi[e.Regola]
								if !ok || r.Score != e.Score || r.Fonte != e.Fonte || (r.Dimensione != q && e.Regola != RegolaOperatore) {
									t.Errorf("%s %s %s: evidenza %+v fuori dalla sua regola %+v", tipo, fonte, nome, e, r)
								}
								if q != "tipo" && e.Fonte == "estensione" {
									t.Errorf("%s %s %s: %s con fonte estensione: %+v", tipo, fonte, nome, q, e)
								}
							}
							if d.Stato == StatoNessuna && (d.Valore != "" || d.Score != 0) {
								t.Errorf("%s %s %s: %s senza evidenza ma con un valore: %+v", tipo, fonte, nome, q, d)
							}
						}
					}
				}
			}
		}
	}
	if n < 1000 {
		t.Errorf("combinazioni provate: %d", n)
	}
}

// TestLeggiValutazione: `dettagli.valutazione` si legge solo nella forma che si conosce; il resto e' una riga
// di prima, da ricostruire.
func TestLeggiValutazione(t *testing.T) {
	v := Valutazione{V: 1, Tabella: TabellaPunteggi, Da: "analisi",
		Tipo: Dimensione{Valore: "cad_3d", Score: 95, Regola: "ext_3d", Stato: StatoUnica, Evidenze: []Evidenza{evidenza("ext_3d", "cad_3d", ".stp")}}}
	b, _ := json.Marshal(map[string]any{"valutazione": v, "bytes": 10})
	letta, ok := LeggiValutazione(b)
	if !ok || letta.Ricostruita || letta.Tipo.Score != 95 || letta.Da != "analisi" {
		t.Errorf("valutazione salvata: %+v %v", letta, ok)
	}
	for _, d := range []string{`{}`, `{"valutazione": {"v": 2}}`, `{"valutazione": null}`, ``, `non json`} {
		if _, ok := LeggiValutazione([]byte(d)); ok {
			t.Errorf("%q non e' una valutazione v1", d)
		}
	}
}
