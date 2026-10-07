// L1 — le correzioni della revisione di V2 (B6; Q7c): il contesto del messaggio con la formula dell'opzione A di R107,
// alla lettera (lettura [T]; R107 precisata il 07/10, senza lettera; R-71: il corrente di ogni messaggio, anche senza
// il gesto 1, più i segmenti che un caso dichiara pertinenti); il documento confermato fuori perimetro contraddetto da
// un'evidenza nuova (R-72); nuovo_file solo ai prodotti con il componente (R-73); l'orfano discordante senza candidati
// (R-74); nessun orfano quando la pertinenza non si calcola (R-75); un caso per ognuna delle mutazioni vive della
// revisione (R-76). Ogni prova fallisce senza la sua correzione (provato con -overlay).
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli ID delle prove della revisione di V2.
var (
	rv2M1  = uid(0x2b01) // un messaggio in più
	rv2M2  = uid(0x2b02) // un altro messaggio in più
	rv2F1  = uid(0x2b11)
	rv2F2  = uid(0x2b12)
	rv2F3  = uid(0x2b13)
	rv2C   = uid(0x2b21) // un componente fuori da ogni perimetro
	rv2R   = uid(0x2b22) // un secondo finito, senza STEP
	rv2D   = uid(0x2b31) // un documento in più
	rv2P   = uid(0x2b41) // una proposta in più
	rv2P2  = uid(0x2b42)
	rv2Fp  = uid(0x2b51) // uno STEP proposto per il secondo prodotto
	rv2Cas = "ACME-RV2"
)

// conCaso: il caso del thread ACME con i segmenti dati.
func conCaso(segmenti ...valutazione.SegmentoDichiarato) valutazione.IngressoCaso {
	tid := threadACME
	return valutazione.IngressoCaso{ID: rv2Cas, ThreadID: &tid, ClienteID: clienteACME, Segmenti: segmenti}
}

// avvisiOrfani: gli avvisi «orfano:» del fascicolo.
func avvisiOrfani(et valutazione.EsitoThread) []string {
	var out []string
	for _, a := range et.Fascicolo.Avvisi {
		if strings.HasPrefix(a, "orfano:") {
			out = append(out, a)
		}
	}
	return out
}

// TestV2R71IlContestoAllaLettera (R-71; la formula dell'opzione A di R107 alla lettera, lettura [T]; R107 precisata il
// 07/10, senza lettera; E1R §6.4; T-E1R-11; T-E1-12): il corrente di un messaggio che nomina solo il prodotto dà il
// contesto anche senza il gesto 1, con la controparte sconosciuta e con la direzione non nota: il file conta (dentro),
// è pertinente al prodotto per il contesto e ne tiene aperto lo smistamento, invece di essere orfano. Con un caso che
// dichiara pertinente solo la storia di un messaggio, il corrente dello stesso messaggio conta ancora (il caso
// aggiunge, non toglie): il corrente e la storia nominano due prodotti diversi, e il file è orfano con tutti e due
// nell'avviso. Un corrente che il caso dichiara escluso non conta (la riga 3 del router: menzione).
func TestV2R71IlContestoAllaLettera(t *testing.T) {
	corpo := "Buongiorno,\r\nper 7120100A vi mandiamo il disegno.\r\nGrazie"
	for _, c := range []struct {
		nome   string
		gesto  bool
		cambia func(m *fotorfq.Messaggio)
	}{
		{"senza il gesto 1", false, func(*fotorfq.Messaggio) {}},
		{"la controparte sconosciuta", true, func(m *fotorfq.Messaggio) { m.ControparteTipo, m.ControparteClienteID = "sconosciuto", nil }},
		{"la direzione non nota", true, func(m *fotorfq.Messaggio) { m.Direzione = "" }},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := scenaSmistamento(t)
			c.cambia(conMessaggio(&th, rv2M1, 15, "Disegni", corpo, c.gesto))
			pdfSenzaCodice(t, &th, rv2F1, rv2M1, "allegato-del-cliente.pdf", 401)
			et := esitoSmistamento(t, th)
			a := associazioneDi(t, et, rv2F1)
			s := prodottoDi(t, et, rifProdB).Smistamento
			if a.Perimetro != valutazione.PerimetroDentro || !reflect.DeepEqual(a.PertinenzaContesto, []string{rifProdB}) ||
				!reflect.DeepEqual(a.Pertinente, []string{rifProdB}) {
				t.Errorf("associazione %+v", a)
			}
			if !nonTerminale(s, rv2F1) || s.Stato == valutazione.SmistamentoVerificato {
				t.Errorf("il file del contesto non tiene aperto lo smistamento: %+v", s)
			}
			if d, ok := daSmistareDi(et, rv2F1); !ok || d.Orfano || len(avvisiOrfani(et)) != 0 {
				t.Errorf("da smistare %+v %v, avvisi %v", d, ok, et.Fascicolo.Avvisi)
			}
		})
	}

	t.Run("il caso aggiunge la storia, non toglie il corrente dello stesso messaggio", func(t *testing.T) {
		r := insiemeConStoria(t)
		th := scenaSmistamento(t)
		secondoProdotto(t, &th)
		conMessaggio(&th, rv2M2, 20, "I: Richiesta 7120100A", "Buongiorno,\r\nvi giro la richiesta per 7120900A.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, rv2F2, rv2M2, "allegato-inoltro.pdf", 402)
		et := esitoSmistamentoCon(t, th, r)
		if a := associazioneDi(t, et, rv2F2); !reflect.DeepEqual(a.PertinenzaContesto, []string{rifProdB}) {
			t.Fatalf("senza caso conta il corrente (l'oggetto), non la storia: %+v", a)
		}
		et = esitoSmistamentoCon(t, th, r, conCaso(valutazione.SegmentoDichiarato{MessaggioID: rv2M2, SegmentoID: "s:storia:1", Uso: "pertinente", Origine: "scenario"}))
		d, ok := daSmistareDi(et, rv2F2)
		if !ok || !d.Orfano || !reflect.DeepEqual(d.ProdottiContesto, []string{rifProdB, rifP2}) {
			t.Errorf("con il caso sulla storia: %+v %v (associazione %+v)", d, ok, associazioneDi(t, et, rv2F2))
		}
		if !contieneRif(et.Fascicolo.Avvisi, "orfano:"+rv2F2.String()+":prodotti_contesto:"+rifProdB+"|"+rifP2) {
			t.Errorf("l'avviso dell'orfano fra %v", et.Fascicolo.Avvisi)
		}
	})

	t.Run("un corrente escluso dal caso non conta", func(t *testing.T) {
		th := scenaSmistamento(t)
		conMessaggio(&th, rv2M1, 15, "Disegni", corpo, true)
		pdfSenzaCodice(t, &th, rv2F1, rv2M1, "allegato-escluso.pdf", 403)
		et := esitoSmistamento(t, th, conCaso(valutazione.SegmentoDichiarato{MessaggioID: rv2M1, SegmentoID: "s:corrente", Uso: "escluso", Origine: "scenario"}))
		if d, ok := daSmistareDi(et, rv2F1); !ok || !d.Orfano {
			t.Errorf("il file del corrente escluso: %+v %v (associazione %+v)", d, ok, associazioneDi(t, et, rv2F1))
		}
	})
}

// scenaFuoriPerimetro: la scena del prodotto altrimenti verificato con il 2D confermato sullo sciolto spostato in un
// messaggio di un fornitore, con il nome dato.
func scenaFuoriPerimetro(t *testing.T, nome string) fotorfq.Thread {
	t.Helper()
	th := scenaSmistamento(t)
	m := conMessaggio(&th, rv2M1, 30, "Offerta", "Buongiorno,\r\nil disegno.\r\nSaluti", false)
	m.ControparteTipo, m.ControparteClienteID = "fornitore", nil
	for i := range th.Allegati {
		if th.Allegati[i].ID == aPDF2 {
			th.Allegati[i].NomeFile = nome
			th.Allegati[i].MessaggioID = rv2M1
		}
	}
	return th
}

// TestV2R72IlDocumentoConfermatoFuoriPerimetro (R-72, decisione [T]; R95 A; R93 b: il perimetro vale per la seconda e la
// terza condizione, non per il conflitto di un documento confermato; T-B0-25; F0-13): il 2D confermato sullo sciolto
// viene da un messaggio di un fornitore, e il suo nome ora porta al prodotto. Il file resta fuori perimetro, non
// pertinente e fuori da «da smistare», ma la decisione contraddetta dà il conflitto di associazione al prodotto che ha lo
// sciolto nel perimetro, e lo smistamento va in conflitto. Con il nome dello sciolto nessun conflitto. Un documento su un
// componente fuori da ogni perimetro dà il conflitto con il prodotto vuoto (mai perso). L'«assegna» di un file fuori
// perimetro non dà conflitto (dubbio T-B6-95). La seconda metà dell'attribuzione (R-78 della controprova di V2): il
// conflitto va ai prodotti con K nel perimetro e anche a quelli a cui il file è pertinente.
func TestV2R72IlDocumentoConfermatoFuoriPerimetro(t *testing.T) {
	et := esitoSmistamento(t, scenaFuoriPerimetro(t, "7120100A_1.pdf"))
	a := associazioneDi(t, et, aPDF2)
	if a.Perimetro != valutazione.PerimetroAltraControparte || a.Pertinente != nil || !a.Conflitto || !a.Terminale {
		t.Errorf("associazione %+v", a)
	}
	if _, ok := daSmistareDi(et, aPDF2); ok {
		t.Error("il file fuori perimetro è da smistare")
	}
	c := conflittiDi(et, valutazione.ConflittoAssociazione)
	if len(c) != 1 || c[0].Prodotto != rifProdB || c[0].Rif != valutazione.RifDocumento(dPDF2) || c[0].Decisione != ancoraggio.RifComponente(cSciolto) ||
		c[0].Motivo != valutazione.MotivoConflittoComponenteDiverso || c[0].OrigineDecisione != ancoraggio.OrigineConfermato {
		t.Fatalf("conflitti %+v", c)
	}
	if s := prodottoDi(t, et, rifProdB).Smistamento; s.Stato != valutazione.SmistamentoConflitto || motiviDi(s) != "associazione_in_conflitto" {
		t.Errorf("smistamento %+v", s)
	}

	t.Run("con il nome dello sciolto nessun conflitto", func(t *testing.T) {
		et := esitoSmistamento(t, scenaFuoriPerimetro(t, "7120200A_1.pdf"))
		if c := conflittiDi(et, valutazione.ConflittoAssociazione); len(c) != 0 {
			t.Errorf("conflitti %+v", c)
		}
		if s := prodottoDi(t, et, rifProdB).Smistamento; s.Stato != valutazione.SmistamentoVerificato {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("un componente fuori da ogni perimetro: il prodotto vuoto", func(t *testing.T) {
		th := scenaFuoriPerimetro(t, "7120200A_1.pdf")
		th.Componenti = append(th.Componenti, componente(rv2C, "7120400A", "sciolto", "manuale"))
		fileNelMessaggio(&th, rv2F1, rv2M1, "7120100A_7.pdf", "pdf", shaN(404), scansione(t, shaN(404)))
		th.Documenti = append(th.Documenti, documento2D(rv2D, rv2C, rv2F1, "7120100A_7.pdf", "pdf", shaN(404)))
		et := esitoSmistamento(t, th)
		c := conflittiDi(et, valutazione.ConflittoAssociazione)
		if len(c) != 1 || c[0].Prodotto != "" || c[0].Decisione != ancoraggio.RifComponente(rv2C) || !associazioneDi(t, et, rv2F1).Conflitto {
			t.Errorf("conflitti %+v", c)
		}
	})
	t.Run("R-78: il conflitto va al prodotto con K nel perimetro e a quello a cui il file è pertinente", func(t *testing.T) {
		// Il 2D confermato sullo sciolto del primo prodotto (K nella sua BOM confermata) ha ora nel nome il codice del
		// secondo prodotto: il file è pertinente al secondo per il candidato, e il conflitto va a tutti e due. Senza il
		// secondo, il suo smistamento non vedrebbe la decisione contraddetta, e il file confermato (terminale) non lo
		// fermerebbe: una chiusura in silenzio.
		th := scenaSmistamento(t)
		secondoProdotto(t, &th)
		for i := range th.Allegati {
			if th.Allegati[i].ID == aPDF2 {
				th.Allegati[i].NomeFile = "7120900A_1.pdf"
			}
		}
		et := esitoSmistamento(t, th)
		var prodotti []string
		for _, c := range conflittiDi(et, valutazione.ConflittoAssociazione) {
			if c.Rif != valutazione.RifDocumento(dPDF2) || c.Decisione != ancoraggio.RifComponente(cSciolto) || c.OrigineDecisione != ancoraggio.OrigineConfermato {
				t.Errorf("conflitto %+v", c)
			}
			prodotti = append(prodotti, c.Prodotto)
		}
		if !reflect.DeepEqual(prodotti, []string{rifProdB, rifP2}) {
			t.Errorf("conflitti sui prodotti %v, attesi il primo (K nel perimetro) e il secondo (pertinente)", prodotti)
		}
		if a := associazioneDi(t, et, aPDF2); !a.Conflitto || len(a.Pertinente) == 0 {
			t.Errorf("associazione %+v", a)
		}
		if s := prodottoDi(t, et, rifP2).Smistamento; s.Stato != valutazione.SmistamentoConflitto || len(s.Conflitti) == 0 {
			t.Errorf("lo smistamento del secondo prodotto %+v", s)
		}
	})
	t.Run("l'«assegna» fuori perimetro non dà conflitto", func(t *testing.T) {
		th := scenaFuoriPerimetro(t, "7120200A_1.pdf")
		fileNelMessaggio(&th, rv2F1, rv2M1, "7120100A_8.pdf", "pdf", shaN(405), scansione(t, shaN(405)))
		cs := cSciolto
		conProposta(&th, rv2P, rv2F1, "disegno_2d", "nome_file", "aperta", &cs, nil)
		et := esitoSmistamento(t, th)
		if c := conflittiDi(et, valutazione.ConflittoAssociazione); len(c) != 0 || associazioneDi(t, et, rv2F1).Conflitto {
			t.Errorf("conflitti %+v", c)
		}
	})
}

// TestV2R73IlNuovoFileSoloAiProdottiConIlComponente (R-73; F0-13: una voce per prodotto che condivide il componente;
// T-B0-29; T-E1-19): lo STEP nuovo con la radice del primo prodotto, e la destinazione F8 sul codice del secondo, è
// pertinente a tutti e due; il nuovo_file sul cad_3d del primo va solo al primo, che ha il componente. Il secondo resta
// fermo per il file non terminale, senza il conflitto né il suo motivo.
func TestV2R73IlNuovoFileSoloAiProdottiConIlComponente(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	f := fattiStepF(t, shaN(406), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
	fileNelMessaggio(&th, rv2F1, idM1, "7120100A_2.stp", "stp", shaN(406), &f)
	conProposta(&th, rv2P, rv2F1, "cad_3d", "step", "aperta", nil, nil).Dettagli = destinazione(t, "identificativo:7120900A")
	et := esitoSmistamento(t, th)
	if a := associazioneDi(t, et, rv2F1); !reflect.DeepEqual(a.Pertinente, []string{rifProdB, rifP2}) {
		t.Fatalf("lo STEP è pertinente ai due prodotti: %+v", a)
	}
	c := conflittiDi(et, valutazione.ConflittoNuovoFile)
	if len(c) != 1 || c[0].Prodotto != rifProdB || c[0].Rif != ancoraggio.RifComponente(cProdotto) {
		t.Errorf("nuovo_file %+v", c)
	}
	p1, p2 := prodottoDi(t, et, rifProdB).Smistamento, prodottoDi(t, et, rifP2).Smistamento
	if p1.Stato != valutazione.SmistamentoConflitto || !strings.Contains(motiviDi(p1), "nuovo_file_su_componente_deciso") {
		t.Errorf("il primo prodotto: %+v", p1)
	}
	if p2.Stato != valutazione.SmistamentoDaVerificare || len(p2.Conflitti) != 0 || strings.Contains(motiviDi(p2), "nuovo_file_su_componente_deciso") ||
		!nonTerminale(p2, rv2F1) {
		t.Errorf("il secondo prodotto: %+v", p2)
	}
}

// TestV2R74LOrfanoDiscordanteSenzaCandidati (R-74; E1 §2.5, FileDaSmistare: un orfano senza candidati è nessun_candidato, un
// fuori_richiesta proposto è fuori_richiesta_non_confermato; T-B6-63): un PDF con il nome e il cartiglio che non
// concordano, nessuno dei due nella richiesta, è discordante, senza candidati e orfano. Con il prodotto che ha la sua
// struttura la collocazione è fuori richiesta, e il motivo è fuori_richiesta_non_confermato; con un target senza struttura
// la collocazione non è determinabile, e il motivo è nessun_candidato. La discordanza resta nell'associazione.
func TestV2R74LOrfanoDiscordanteSenzaCandidati(t *testing.T) {
	for _, c := range []struct {
		nome         string
		senzaStrutt  bool
		collocazione ancoraggio.Collocazione
		motivo       valutazione.MotivoSmistamento
	}{
		{"fuori richiesta", false, ancoraggio.CollocazioneFuoriRichiesta, valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato},
		{"un target senza struttura", true, ancoraggio.CollocazioneNonDeterminabile, valutazione.MotivoSmistamentoNessunCandidato},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := scenaSmistamento(t)
			if c.senzaStrutt {
				th.Identificativi = []fotorfq.Identificativo{confermato("7120888A")}
			}
			conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
			fileNelMessaggio(&th, rv2F1, mNessuno, "7120555A_1.pdf", "pdf", shaN(407), conCartiglio(t, shaN(407), [2]string{"codice", "7120556A1"}))
			et := esitoSmistamento(t, th)
			a := associazioneDi(t, et, rv2F1)
			if a.Associazione != ancoraggio.AssociazioneDiscordante || a.Collocazione != c.collocazione || len(a.Proposta) != 0 {
				t.Fatalf("associazione %+v", a)
			}
			if d, ok := daSmistareDi(et, rv2F1); !ok || !d.Orfano || d.Motivo != c.motivo {
				t.Errorf("da smistare %+v %v", d, ok)
			}
		})
	}
}

// TestV2R75NessunOrfanoSenzaPertinenza (R-75; T-12; T-B6-09; T-B6-61; R93): quando la pertinenza non si calcola nessun file
// è orfano, e il fascicolo non ha avvisi di orfani. Con la sezione delle proposte assente (gli export) lo scarto non si
// vede: il file scartato sta in «da smistare» con il suo motivo, non orfano; senza grammatica un file con il nome dello
// sciolto sta in «da smistare», non orfano. Lo smistamento dei prodotti non è calcolato. Con una sezione assente solo
// della completezza la pertinenza si calcola, e l'orfano resta, con l'avviso.
func TestV2R75NessunOrfanoSenzaPertinenza(t *testing.T) {
	t.Run("T-12 sulla sezione delle proposte", func(t *testing.T) {
		th := scenaSmistamento(t)
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, rv2F1, mNessuno, "scartato.pdf", 408)
		op := operatore
		conProposta(&th, rv2P, rv2F1, "altro", "nome_file", "scartata", nil, &op)
		vistaCome(&th)
		th.Proposte = nil
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneProposteDocumento] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		et := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME)
		d, ok := daSmistareDi(et, rv2F1)
		if !ok || d.Orfano || d.Motivo != valutazione.MotivoSmistamentoNessunCandidato || et.Fascicolo.Orfani != 0 || len(avvisiOrfani(et)) != 0 {
			t.Errorf("da smistare %+v %v; orfani %d, avvisi %v", d, ok, et.Fascicolo.Orfani, et.Fascicolo.Avvisi)
		}
		if s := prodottoDi(t, et, rifProdB).Smistamento; s.Calcolata {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("T-12 sulle provenienze", func(t *testing.T) {
		th := scenaSmistamento(t)
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, rv2F1, mNessuno, "senza-evidenze.pdf", 409)
		vistaCome(&th)
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneProvenienze] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		et := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME)
		if d, ok := daSmistareDi(et, rv2F1); !ok || d.Orfano || et.Fascicolo.Orfani != 0 || len(avvisiOrfani(et)) != 0 {
			t.Errorf("da smistare %+v %v; orfani %d, avvisi %v", d, ok, et.Fascicolo.Orfani, et.Fascicolo.Avvisi)
		}
	})
	t.Run("T-12 e un messaggio che nomina due prodotti: niente prodotti del contesto", func(t *testing.T) {
		th := scenaSmistamento(t)
		secondoProdotto(t, &th)
		conMessaggio(&th, mDue, 20, "Altri file", "Buongiorno,\r\nper 7120100A e 7120900A vi mandiamo i file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, rv2F2, mDue, "allegato-due.pdf", 421)
		vistaCome(&th)
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneProvenienze] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		et := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME)
		if d, ok := daSmistareDi(et, rv2F2); !ok || d.Orfano || d.ProdottiContesto != nil || len(avvisiOrfani(et)) != 0 {
			t.Errorf("da smistare %+v %v; avvisi %v", d, ok, et.Fascicolo.Avvisi)
		}
	})
	t.Run("senza grammatica", func(t *testing.T) {
		th := scenaSmistamento(t)
		fileNelMessaggio(&th, rv2F1, idM1, "7120200A_1.pdf", "pdf", shaN(410), scansione(t, shaN(410)))
		vistaCome(&th)
		et := esitoThread(t, calcola(t, fotografia(th), nil, valutazione.Ingressi{}), threadACME)
		d, ok := daSmistareDi(et, rv2F1)
		if et.Valutato || !ok || d.Orfano || et.Fascicolo.Orfani != 0 || len(avvisiOrfani(et)) != 0 {
			t.Errorf("valutato %v; da smistare %+v %v; orfani %d, avvisi %v", et.Valutato, d, ok, et.Fascicolo.Orfani, et.Fascicolo.Avvisi)
		}
		if s := prodottoDi(t, et, rifProdB).Smistamento; s.Calcolata || s.Stato != valutazione.SmistamentoDaVerificare {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("una sezione solo della completezza: l'orfano resta", func(t *testing.T) {
		th := scenaSmistamento(t)
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, rv2F1, mNessuno, "orfano.pdf", 411)
		vistaCome(&th)
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneDeroghe] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		et := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME)
		if d, ok := daSmistareDi(et, rv2F1); !ok || !d.Orfano || !contieneRif(et.Fascicolo.Avvisi, "orfano:"+rv2F1.String()) {
			t.Errorf("da smistare %+v %v; avvisi %v", d, ok, et.Fascicolo.Avvisi)
		}
	})
}

// TestV2R76LeMutazioniVive (R-76): un caso per ognuna delle mutazioni vive, non equivalenti, della revisione di V2.
func TestV2R76LeMutazioniVive(t *testing.T) {
	// M01 (T-B6-09, T-12): la completezza non determinabile, con le sezioni dello smistamento presenti, basta a non
	// calcolare lo smistamento: il primo requisito dipende dalle voci certe.
	t.Run("M01 la completezza non determinabile", func(t *testing.T) {
		th := scenaSmistamento(t)
		vistaCome(&th)
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneDeroghe] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		p := prodottoDi(t, esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME), rifProdB)
		if p.Documenti.Motivo != valutazione.MotivoDocumentiNonDeterminabile || p.Smistamento.Calcolata || p.Smistamento.Stato != valutazione.SmistamentoDaVerificare {
			t.Errorf("documenti %s/%s, smistamento %+v", p.Documenti.Stato, p.Documenti.Motivo, p.Smistamento)
		}
	})

	// M02 (R76 b; contratto §1.5, la quarta evidenza): solo il candidato della fonte trovato dal motore A (lo STEP con la
	// radice del prodotto) è un'evidenza; la proposta aperta di v_step_prodotto no.
	t.Run("M02 il candidato della fonte da una proposta aperta", func(t *testing.T) {
		th := scenaSmistamento(t)
		secondoProdotto(t, &th)
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		f := fattiStepF(t, shaN(412), []nodoF{{"#1", "7120555A", ""}, {"#2", "7120556A", ""}}, []arcoF{{"#1", "#2", 1}})
		fileNelMessaggio(&th, rv2Fp, mNessuno, "assieme.stp", "stp", shaN(412), &f)
		codice := "7120900A"
		conProposta(&th, rv2P2, rv2Fp, "cad_3d", "step", "aperta", nil, nil).Codice = &codice
		pid := rv2P2
		th.StepProdotto[len(th.StepProdotto)-1].PropostaAperta = &pid
		et := esitoSmistamento(t, th)
		proposta := false
		for _, c := range prodottoDi(t, et, rifP2).Fonte.Candidati {
			proposta = proposta || (c.Origine == ancoraggio.CandidatoDaPropostaAperta && c.AllegatoID != nil && *c.AllegatoID == rv2Fp)
		}
		if !proposta {
			t.Fatalf("lo STEP non è candidato della fonte del secondo prodotto: %+v", prodottoDi(t, et, rifP2).Fonte.Candidati)
		}
		if a := associazioneDi(t, et, rv2Fp); a.Pertinente != nil {
			t.Errorf("la proposta aperta di v_step_prodotto è un'evidenza: %+v", a)
		}
	})

	// M03 (T-B0-32): il duplicato senza deciso_da non è terminale, e tiene aperto lo smistamento.
	t.Run("M03 il duplicato senza deciso_da", func(t *testing.T) {
		th := scenaSmistamento(t)
		fileNelMessaggio(&th, rv2F1, idM1, "7120200A_1.pdf", "pdf", shaN(413), scansione(t, shaN(413)))
		conProposta(&th, rv2P, rv2F1, "disegno_2d", "nome_file", "duplicato", nil, nil)
		et := esitoSmistamento(t, th)
		s := prodottoDi(t, et, rifProdB).Smistamento
		if associazioneDi(t, et, rv2F1).Terminale || !nonTerminale(s, rv2F1) || s.Stato == valutazione.SmistamentoVerificato {
			t.Errorf("associazione %+v, smistamento %+v", associazioneDi(t, et, rv2F1), s)
		}
	})

	// M04 (T-B0-25; R62 b A): un documento sostituito non è la decisione del conflitto: il file che porta solo lui, con il
	// nome del prodotto, non dà conflitto di associazione.
	t.Run("M04 il documento sostituito", func(t *testing.T) {
		th := scenaSmistamento(t)
		fileNelMessaggio(&th, rv2F1, idM1, "7120100A_8.pdf", "pdf", shaN(414), scansione(t, shaN(414)))
		d := documento2D(rv2D, cSciolto, rv2F1, "7120100A_8.pdf", "pdf", shaN(414))
		nuovo := dPDF2
		d.SostituitoDa = &nuovo
		th.Documenti = append(th.Documenti, d)
		et := esitoSmistamento(t, th)
		for _, c := range conflittiDi(et, valutazione.ConflittoAssociazione) {
			if c.EvidenzaProposta.AllegatoID != nil && *c.EvidenzaProposta.AllegatoID == rv2F1 {
				t.Errorf("il documento sostituito è la decisione di un conflitto: %+v", c)
			}
		}
		if a := associazioneDi(t, et, rv2F1); a.Conflitto {
			t.Errorf("associazione %+v", a)
		}
	})

	// M08 (T-B0-12; dubbio T-B6-66): l'«assegna» su un componente, con l'identità letta che porta altrove, è in conflitto:
	// il riferimento è l'allegato, la decisione è manuale, senza chi né quando.
	t.Run("M08 il conflitto con l'«assegna»", func(t *testing.T) {
		th := scenaSmistamento(t)
		fileNelMessaggio(&th, rv2F1, idM1, "7120100A_8.pdf", "pdf", shaN(415), scansione(t, shaN(415)))
		cs := cSciolto
		conProposta(&th, rv2P, rv2F1, "disegno_2d", "nome_file", "aperta", &cs, nil)
		et := esitoSmistamento(t, th)
		c := conflittiDi(et, valutazione.ConflittoAssociazione)
		if len(c) != 1 || c[0].Rif != "allegato:"+rv2F1.String() || c[0].Prodotto != rifProdB || c[0].Decisione != ancoraggio.RifComponente(cSciolto) ||
			c[0].OrigineDecisione != ancoraggio.OrigineManuale || c[0].Motivo != valutazione.MotivoConflittoComponenteDiverso || c[0].EvidenzaDecisione.Da != nil ||
			c[0].EvidenzaProposta.DocumentoID != nil {
			t.Fatalf("conflitti %+v", c)
		}
		if s := prodottoDi(t, et, rifProdB).Smistamento; s.Stato != valutazione.SmistamentoConflitto || !associazioneDi(t, et, rv2F1).Conflitto {
			t.Errorf("smistamento %+v", s)
		}
	})

	// M09 (T-E1-12; dubbio T-B6-65): la controparte cliente senza l'ID del cliente resta dentro il perimetro.
	t.Run("M09 la controparte cliente senza ID", func(t *testing.T) {
		th := scenaSmistamento(t)
		m := conMessaggio(&th, rv2M1, 15, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		m.ControparteClienteID = nil
		pdfSenzaCodice(t, &th, rv2F1, rv2M1, "allegato.pdf", 416)
		if a := associazioneDi(t, esitoSmistamento(t, th), rv2F1); a.Perimetro != valutazione.PerimetroDentro {
			t.Errorf("associazione %+v", a)
		}
	})

	// M10 (contratto §1.5, la seconda evidenza): la collocazione non determinabile è un'evidenza per P solo se P non ha
	// nessuna struttura. Con la famiglia che ha solo il ruolo «componente», il prodotto con il suo STEP (la radice non è un
	// nodo da componente) e un secondo target senza struttura, il file con il codice del prodotto non ha candidati, non è
	// determinabile, e resta orfano.
	t.Run("M10 la collocazione non determinabile con la struttura", func(t *testing.T) {
		fam := famigliaCatena()
		fam.Ruoli = []grammatica.Ruolo{grammatica.RuoloComponente}
		r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", fam)})
		rifQ := ancoraggio.RifComponente(cQ)
		th := threadBase("Buongiorno,\r\nper 7120777A vi mandiamo i file.\r\nGrazie")
		th.Componenti = []fotorfq.Componente{componente(cQ, "7120777A", "finito", "manuale"), componente(rv2R, "7120666A", "finito", "manuale")}
		th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cQ, Codice: "7120777A", Esito: "da_scegliere"},
			{ComponenteID: rv2R, Codice: "7120666A", Esito: "da_scegliere"}}
		th.Fabbisogni = fabbisogniDefault()
		fileNelMessaggio(&th, rv2F1, idM1, "7120777A_1.pdf", "pdf", shaN(417), scansione(t, shaN(417)))
		st := fattiStepF(t, shaN(418), []nodoF{{"#1", "7120777A", ""}, {"#2", "7120778A", ""}}, []arcoF{{"#1", "#2", 1}})
		fileNelMessaggio(&th, rv2F2, idM1, "7120777A_1.stp", "stp", shaN(418), &st)
		et := esitoSmistamentoCon(t, th, r)
		if prodottoDi(t, et, rifQ).Struttura == ancoraggio.StatoStrutturaNessuna {
			t.Fatal("il prodotto non ha la struttura dello STEP")
		}
		a := associazioneDi(t, et, rv2F1)
		if a.Collocazione != ancoraggio.CollocazioneNonDeterminabile || len(a.Proposta) != 0 || a.Pertinente != nil {
			t.Errorf("il file con il codice del prodotto che ha una struttura: %+v", a)
		}
	})

	// M13, M14 (contratto §1.5, «Corretta a mano o proposta e accettata»): la prima chiave «generale» non coincide con un
	// documento su un componente; «identificativo:<CODICE>» coincide con il documento sul componente del target.
	t.Run("M13 M14 DestinazioneCoincide", func(t *testing.T) {
		th := scenaSmistamento(t)
		op := operatore
		conProposta(&th, rv2P, aPDF2, "disegno_2d", "nome_file", "confermata", nil, &op).Dettagli = destinazione(t, "generale")
		conProposta(&th, rv2P2, aPDF1, "disegno_2d", "nome_file", "confermata", nil, &op).Dettagli = destinazione(t, "identificativo:7120100A")
		et := esitoSmistamento(t, th)
		if a := associazioneDi(t, et, aPDF2); a.DestinazioneCoincide == nil || *a.DestinazioneCoincide {
			t.Errorf("«generale» con il documento sullo sciolto: %+v", a)
		}
		if a := associazioneDi(t, et, aPDF1); a.DestinazioneCoincide == nil || !*a.DestinazioneCoincide {
			t.Errorf("«identificativo:7120100A» con il documento sul prodotto: %+v", a)
		}
	})

	// M16 (T-B6-08): solo la voce da_verificare con associazione_non_confermata è la prima condizione; una voce con
	// un'altra ragione, anche con una proposta aperta, la dice la completezza.
	t.Run("M16 la voce con un'altra ragione e una proposta aperta", func(t *testing.T) {
		in := ingressoVerificato()
		p := uid(0x2b61)
		in.Voci = append(in.Voci, valutazione.VoceFabbisogno{TipoDocumento: "disegno_2d", Esito: valutazione.EsitoDaVerificare,
			Motivo: valutazione.MotivoFabbisognoContenutoNonLetto, PropostaAperta: &p})
		if s := valutazione.Smistamento(in, nil); s.Stato != valutazione.SmistamentoVerificato || len(s.Motivi) != 0 {
			t.Errorf("smistamento %+v", s)
		}
	})

	// M21 (T-B0-29, T-B0-32): un file terminale (scartato o duplicato con deciso_da) non dà nuovo_file.
	t.Run("M21 nessun nuovo_file per un file terminale", func(t *testing.T) {
		for _, stato := range []string{"scartata", "duplicato"} {
			th := scenaSmistamento(t)
			f := fattiStepF(t, shaN(419), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
			fileNelMessaggio(&th, rv2F1, idM1, "7120100A_2.stp", "stp", shaN(419), &f)
			op := operatore
			conProposta(&th, rv2P, rv2F1, "cad_3d", "step", stato, nil, &op)
			et := esitoSmistamento(t, th)
			if c := conflittiDi(et, valutazione.ConflittoNuovoFile); len(c) != 0 || !associazioneDi(t, et, rv2F1).Terminale {
				t.Errorf("%s: conflitti %+v", stato, c)
			}
			if s := prodottoDi(t, et, rifProdB).Smistamento; s.Stato != valutazione.SmistamentoVerificato {
				t.Errorf("%s: smistamento %+v", stato, s)
			}
		}
	})

	// M22 (contratto §1.5, la prima evidenza; R64 A): il discordante con candidati nel prodotto è pertinente al prodotto
	// per i candidati, non orfano, e blocca con associazione_discordante.
	t.Run("M22 il discordante con candidati", func(t *testing.T) {
		th := scenaSmistamento(t)
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		fileNelMessaggio(&th, rv2F3, mNessuno, "7120200A_1.pdf", "pdf", shaN(420), conCartiglio(t, shaN(420), [2]string{"codice", "7120100A1"}))
		et := esitoSmistamento(t, th)
		a := associazioneDi(t, et, rv2F3)
		if a.Associazione != ancoraggio.AssociazioneDiscordante || len(a.Proposta) == 0 || !reflect.DeepEqual(a.Pertinente, []string{rifProdB}) || a.PertinenzaContesto != nil {
			t.Fatalf("associazione %+v", a)
		}
		d, ok := daSmistareDi(et, rv2F3)
		if !ok || d.Orfano || d.Motivo != valutazione.MotivoSmistamentoAssociazioneDiscordante || !strings.Contains(motiviDi(prodottoDi(t, et, rifProdB).Smistamento), "associazione_discordante") {
			t.Errorf("da smistare %+v %v", d, ok)
		}
	})
}
