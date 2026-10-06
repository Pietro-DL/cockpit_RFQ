// L1 — i 2D nei casi di confine (B5, fase 2; i casi che le mutazioni non vedevano): sulla regola, un «corrente» non
// confermato, l'entità solo dallo STEP, il confronto senza la revisione del componente, il namespace e il marcatore nel
// confronto dei codici, «a» e «A» come due revisioni, la fonte senza coppia; dalla fotografia, le fonti dell'identità
// nell'ordine, «assegna» di un file che non è un 2D, un candidato su un nodo senza decisione, la radice scelta senza la
// base del prodotto, la voce più forte per contenuto, il cartiglio di una scansione letto dall'OCR e quello del worker di
// prima, un contenuto che l'adattatore non legge, i campi del cartiglio (anche con la revisione facoltativa),
// documento.rev e le righe di un altro componente; il testo non interpretato della revisione con un'altra maiuscola
// (T-B5-46).
package valutazione_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// TestLaRegolaDeiDisegniNeiCasiDiConfine (R84, E1R §5.2, §5.3, §5.6; T-B0-34, T-B4-30): i casi della regola che
// l'adattatore non produce, ma che la regola deve dire giusti con ingressi qualunque.
func TestLaRegolaDeiDisegniNeiCasiDiConfine(t *testing.T) {
	m := motoreCatenaRev(t)
	nd := motorea.CompatibilitaNonDeterminabile
	t.Run("un corrente non confermato non è il criterio 1", func(t *testing.T) {
		incoerente := disegnoSintetico(sha2, "png", "", nd)
		incoerente.Provenienza, incoerente.Corrente = ancoraggio.OrigineManuale, true
		g := valutazione.Primario([]valutazione.Disegno2D{incoerente, disegnoSintetico(sha1, "pdf", "", nd)})
		if g.Primario.Sha256 != sha1 || g.RegolaPrimario != valutazione.RegolaPrimarioFormato {
			t.Errorf("gruppo %+v", g)
		}
	})
	t.Run("l'entità della proposta solo dallo STEP", func(t *testing.T) {
		nome := valutazione.EvidenzaRevisione{Fonte: valutazione.FonteEvidenzaNomeFile, Valore: testo("4"), Interpretabile: true, Entita: nodoB("#9")}
		if p := valutazione.PropostaDiRevisione([]valutazione.EvidenzaRevisione{nome}); p.Entita != "" || val(p.Valore) != "4" {
			t.Errorf("proposta %+v", p)
		}
	})
	ld := letturaDi(t, m, "nodo_step.id", "7120200A")
	cart := func(codice string) valutazione.FonteDelDisegno {
		l := letturaDi(t, m, "cartiglio.codice", codice)
		return fonteSintetica(valutazione.FonteEvidenzaCartiglio, codice, l.Revisione.Normalizzata, &l, "")
	}
	t.Run("senza la revisione del componente non si confronta", func(t *testing.T) {
		d, _ := valutazione.ValutaDisegno(pdfDeciso(cart("7120200A2")), valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Lettura: &ld,
			Confronto: valutazione.ConfrontatoConProposto}, nil)
		if d.ConfrontatoCon != valutazione.ConfrontatoConNessuno || d.CompatibilitaRevisione != nd || d.CompatibilitaCodice != motorea.CompatibilitaUguale {
			t.Errorf("%s %s %s", d.ConfrontatoCon, d.CompatibilitaRevisione, d.CompatibilitaCodice)
		}
	})
	t.Run("un altro spazio di codici non si confronta", func(t *testing.T) {
		altro := letturaDi(t, motoreACME(t), "nodo_step.id", "7120200")
		d, _ := valutazione.ValutaDisegno(pdfDeciso(cart("7120200A2")), valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Lettura: &altro}, nil)
		if d.CompatibilitaCodice != nd {
			t.Errorf("codice %s: due namespace diversi non sono confrontabili", d.CompatibilitaCodice)
		}
	})
	t.Run("il marcatore scritto da una parte sola", func(t *testing.T) {
		senza := ld
		senza.Marcatore = nil
		d, _ := valutazione.ValutaDisegno(pdfDeciso(cart("7120200A2")), valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Lettura: &senza}, nil)
		if d.CompatibilitaCodice != motorea.CompatibilitaUguale {
			t.Errorf("codice %s (T-B4-30: un marcatore scritto da una parte sola è compatibile)", d.CompatibilitaCodice)
		}
	})
	t.Run("«a» e «A» sono due revisioni (T-B5-34)", func(t *testing.T) {
		// Le revisioni si confrontano esatte dopo gli spazi ai bordi, come in ancoraggio e come le tiene la grammatica.
		a, A := " a ", "A"
		in := valutazione.DisegnoDaValutare{Disegno: valutazione.Disegno2D{Sha256: sha1, Formato: valutazione.Formato2DPDF, Provenienza: ancoraggio.OrigineConfermato},
			Fonti: []valutazione.FonteDelDisegno{{Revisione: valutazione.EvidenzaRevisione{Fonte: valutazione.FonteEvidenzaCartiglio, Valore: &a, Interpretabile: true}}}}
		d, _ := valutazione.ValutaDisegno(in, valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Revisione: &A,
			FonteRevisione: valutazione.FonteComponenteStepEntita, Confronto: valutazione.ConfrontatoConProposto}, nil)
		if d.CompatibilitaRevisione != motorea.CompatibilitaDiscordante || len(d.Discordanze) != 1 || d.Discordanze[0].Tra != valutazione.TraDocumentoComponente ||
			d.Discordanze[0].A != "a" || d.Discordanze[0].B != "A" {
			t.Errorf("compatibilità %s, discordanze %+v", d.CompatibilitaRevisione, d.Discordanze)
		}
		A = " a"
		if d, _ := valutazione.ValutaDisegno(in, valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Revisione: &A,
			FonteRevisione: valutazione.FonteComponenteStepEntita, Confronto: valutazione.ConfrontatoConProposto}, nil); d.CompatibilitaRevisione != motorea.CompatibilitaUguale ||
			len(d.Discordanze) != 0 {
			t.Errorf("gli spazi ai bordi non contano: compatibilità %s, discordanze %+v", d.CompatibilitaRevisione, d.Discordanze)
		}
	})
	t.Run("una fonte senza coppia non dà un conflitto", func(t *testing.T) {
		in := pdfDeciso(fonteSintetica(valutazione.FonteEvidenzaStepEntita, "", "5", nil, nodoB("#2")))
		in.Decisione, in.LetturaDecisa = decisioneDoc("3"), &ld
		if _, c := valutazione.ValutaDisegno(in, valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto}, nil); len(c) != 0 {
			t.Errorf("conflitti %+v: senza la coppia l'evidenza non si dice nuova", c)
		}
	})
}

// payloadPDFSenzaCartiglio: i fatti di un PDF con il testo e un frammento, senza i campi del cartiglio.
func payloadPDFSenzaCartiglio(t *testing.T, s string) fotorfq.Fatti {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"testo_pdf": map[string]any{
		"versione": 2, "estraibile": true, "pagine": 1, "pagine_lette": 1, "caratteri": 10, "troncato": false, "formato_pagina1": []float64{1191, 842},
		"frammenti": []map[string]any{{"testo": "NOTE GENERALI", "pagina": 1, "riquadro": []float64{100, 100, 300, 120}, "zona": "basso_destra", "fonte": "nativo"}},
		"cartiglio": []map[string]any{}, "metadati": map[string]any{"titolo": "", "soggetto": "", "parole_chiave": "", "creatore": "", "produttore": ""},
		"ocr": map[string]any{"stato": "non_necessario", "motivo": "", "motore": "", "tentativi": []string{}},
		"limiti": map[string]any{"pagine_max": 11, "frammenti_max": 200, "frammento_max": 512, "caratteri_max": 16384, "campi_max": 24,
			"ocr_soglia_pagina": 20, "ocr_soglia_cartiglio": 8, "ocr_pagine_max": 2, "ocr_tempo_max_s": 60, "ocr_dpi": 300},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fatti(t, s, string(raw), nil)
}

// payloadTestoPDF: i fatti testo_pdf di un PDF con la sottoversione del worker, il testo nativo o no, lo stato dell'OCR e
// la fonte dei campi del cartiglio dati (nativo, ocr).
func payloadTestoPDF(t *testing.T, versione int, estraibile bool, statoOCR, fonte string, campi ...[2]string) string {
	t.Helper()
	cs := []map[string]any{}
	for _, c := range campi {
		cs = append(cs, map[string]any{"etichetta": c[0], "letta": c[0], "valore": c[1], "pagina": 1, "zona": "basso_destra",
			"fonte": fonte, "riquadro": []float64{800, 700, 900, 711}, "confidenza": 0.9})
	}
	raw, err := json.Marshal(map[string]any{"testo_pdf": map[string]any{
		"versione": versione, "estraibile": estraibile, "pagine": 1, "pagine_lette": 1, "caratteri": 0, "troncato": false, "formato_pagina1": []float64{1191, 842},
		"frammenti": []map[string]any{}, "cartiglio": cs, "metadati": map[string]any{"titolo": "", "soggetto": "", "parole_chiave": "", "creatore": "", "produttore": ""},
		"ocr": map[string]any{"stato": statoOCR, "motivo": "", "motore": "acme-ocr", "tentativi": []string{}},
		"limiti": map[string]any{"pagine_max": 11, "frammenti_max": 200, "frammento_max": 512, "caratteri_max": 16384, "campi_max": 24,
			"ocr_soglia_pagina": 20, "ocr_soglia_cartiglio": 8, "ocr_pagine_max": 2, "ocr_tempo_max_s": 60, "ocr_dpi": 300},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestIDisegniNeiCasiDiConfine (R84, R104; T-E1R-05…07; dubbi T-B5-33, T-B5-35, T-B5-36, T-B5-37, T-B5-41, T-B5-43).
func TestIDisegniNeiCasiDiConfine(t *testing.T) {
	m := motoreCatenaRev(t)
	unico := func(t *testing.T, th fotorfq.Thread, mm *motorea.Motore, comp uuid.UUID) valutazione.Disegno2D {
		t.Helper()
		g := gruppoDi(t, valuta(t, th, mm, nil), comp)
		if len(tuttiDi(g)) != 1 {
			t.Fatalf("gruppo %+v", tuttiDi(g))
		}
		return *g.Primario
	}
	t.Run("il cartiglio prima del nome, per la revisione e per il codice", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "7120200A_3.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", "7120200A2"}))
		d := unico(t, th, m, cSciolto)
		if d.FonteIdentita != valutazione.FonteIdentitaCartiglio || val(d.Revisione) != "2" || d.Codice != "7120200A2" {
			t.Errorf("identità %s %s %q", d.FonteIdentita, val(d.Revisione), d.Codice)
		}
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaNomeFile); val(e.Valore) != "3" {
			t.Errorf("il nome resta un'evidenza: %+v", e)
		}
	})
	t.Run("«assegna» su un file che non è un 2D", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		conAllegato(&th, allegato(aPDF2, 9, "7120200A_2.pdf", "pdf", sha2), nil)
		c := cSciolto
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: uid(0x7f7), AllegatoID: aPDF2, Tipo: "altro", ComponenteID: &c, Fonte: "operatore", Stato: "aperta"})
		unico(t, th, m, cSciolto)
	})
	t.Run("un candidato su un nodo senza decisione non è del prodotto", func(t *testing.T) {
		th := scenaRevisioni(t)
		aggiungiNodo(t, &th, "#3", "7120300A1")
		conFile2D(&th, allegato(aPDF1, 9, "7120300A_1.pdf", "pdf", sha1), scansione(t, sha1), nil, uuid.Nil)
		v := valuta(t, th, m, nil)
		for _, x := range v.Disegni {
			t.Errorf("il componente %s ha un 2D: il nodo #3 non è deciso", x.ComponenteID)
		}
	})
	t.Run("la radice scelta senza la base del prodotto", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120900A1", ""}, {"#2", "7120200A3", ""}}, []arcoF{{"#1", "#2", 2}})
		confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
		conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "assieme-acme.pdf", sha1, scansione(t, sha1))
		v := valuta(t, th, m, nil)
		strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
		d := *gruppoDi(t, v, cProdotto).Primario
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaStepEntita); e.Entita != nodoB("#1") || val(e.Valore) != "1" {
			t.Errorf("lo STEP del prodotto %+v (R76 A: la radice scelta rappresenta il prodotto)", e)
		}
	})
	t.Run("la voce più forte per contenuto", func(t *testing.T) {
		th := scenaRevisioni(t)
		f := scansione(t, sha1)
		conAllegato(&th, allegato(aPDF1, 9, "disegno-acme.pdf", "pdf", sha1), f)
		vecchio := documento2D(dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", "pdf", sha1)
		sost := dPDF2
		vecchio.SostituitoDa, vecchio.ConfermatoIl = &sost, dataACME.Add(9*time.Hour)
		corrente := documento2D(dPDF2, cSciolto, aPDF1, "disegno-acme.pdf", "pdf", sha1)
		th.Documenti = append(th.Documenti, vecchio, corrente)
		if d := unico(t, th, m, cSciolto); *d.DocumentoID != dPDF2 || !d.Corrente {
			t.Errorf("il documento corrente prima del sostituito più recente: %s", *d.DocumentoID)
		}
		th.Documenti[len(th.Documenti)-2].SostituitoDa = nil
		if d := unico(t, th, m, cSciolto); *d.DocumentoID != dPDF1 {
			t.Errorf("fra due correnti, il confermato più di recente: %s", *d.DocumentoID)
		}
	})
	t.Run("«assegna» prima del candidato con lo stesso contenuto", func(t *testing.T) {
		th := scenaRevisioni(t)
		conFile2D(&th, allegato(aPDF2, 9, "7120200A_3.pdf", "pdf", sha1), scansione(t, sha1), nil, cSciolto)
		conFile2D(&th, allegato(aPDF1, 10, "7120200A_3.pdf", "pdf", sha1), nil, nil, uuid.Nil)
		if d := unico(t, th, m, cSciolto); d.Provenienza != ancoraggio.OrigineManuale || *d.AllegatoID != aPDF2 {
			t.Errorf("la voce %s %s", d.Provenienza, *d.AllegatoID)
		}
	})
	t.Run("una scansione con i campi del cartiglio dall'OCR (T-B5-37)", func(t *testing.T) {
		// Riscritta dopo la revisione della fase 2: il cartiglio è letto se c'è un'unità del cartiglio, dal testo nativo o
		// dall'OCR, come lo legge ancoraggio sullo stesso file; la capacità cartiglio dell'adattatore dice il testo nativo.
		th := scenaRevisioni(t)
		f := fatti(t, sha1, payloadTestoPDF(t, 2, false, "eseguito", "ocr", [2]string{"codice", "7120200A2"}), nil)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, &f)
		v := valuta(t, th, m, nil)
		d := *gruppoDi(t, v, cSciolto).Primario
		if d.Cartiglio != valutazione.CartiglioLetto || d.Validita != valutazione.ValiditaValido || d.Testo {
			t.Errorf("cartiglio %s, validità %s, testo %v", d.Cartiglio, d.Validita, d.Testo)
		}
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaCartiglio); val(e.Valore) != "2" || !e.Interpretabile {
			t.Errorf("l'evidenza del cartiglio %+v", e)
		}
		if d.RevisioneProposta.Fonte != valutazione.FonteEvidenzaCartiglio || val(d.RevisioneProposta.Valore) != "2" || d.Codice != "7120200A2" ||
			d.FonteIdentita != valutazione.FonteIdentitaCartiglio {
			t.Errorf("proposta %+v, codice %q dalla fonte %s", d.RevisioneProposta, d.Codice, d.FonteIdentita)
		}
		letto := false
		for _, cd := range nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2")).Codice.Documentale {
			letto = letto || (cd.AllegatoID == aPDF1 && cd.Originale == "7120200A2")
		}
		if !letto {
			t.Error("ancoraggio legge lo stesso cartiglio dall'OCR: le due uscite non si contraddicono")
		}
	})
	t.Run("il testo del worker di prima: il cartiglio resta non leggibile", func(t *testing.T) {
		th := scenaRevisioni(t)
		f := fatti(t, sha1, payloadTestoPDF(t, 1, true, "non_necessario", "nativo", [2]string{"codice", "7120200A2"}), nil)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, &f)
		d := unico(t, th, m, cSciolto)
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaCartiglio); d.Cartiglio != valutazione.CartiglioNonLeggibile || e.Valore != nil ||
			d.RevisioneProposta.Fonte != valutazione.FonteEvidenzaStepEntita {
			t.Errorf("cartiglio %s, evidenza %+v, proposta %+v: i campi del worker di prima sono unità testo_pdf", d.Cartiglio, e, d.RevisioneProposta)
		}
	})
	t.Run("un contenuto che l'adattatore non legge non è valido (T-B5-36)", func(t *testing.T) {
		// I fatti del documento con un payload che non è un oggetto JSON: l'adattatore li rifiuta.
		th := scenaRevisioni(t)
		th.Fatti[sha1] = fatti(t, sha1, `[]`, nil)
		th.Documenti = append(th.Documenti, documento2D(dPDF1, cSciolto, uuid.Nil, "disegno-acme.pdf", "pdf", sha1))
		if d := unico(t, th, m, cSciolto); d.Validita != valutazione.ValiditaDaVerificare || d.MotivoValidita != valutazione.MotivoFabbisognoContenutoNonLetto {
			t.Errorf("validità %s/%s", d.Validita, d.MotivoValidita)
		}
	})
	t.Run("due codici nel cartiglio con la revisione facoltativa: vale quello con la revisione (T-B5-43)", func(t *testing.T) {
		f := famigliaCatenaRev()
		for i := range f.Forme {
			if f.Forme[i].ID != "cartiglio" {
				continue
			}
			for j := range f.Forme[i].Parti {
				if f.Forme[i].Parti[j].Rif == "rev-c" {
					f.Forme[i].Parti[j].Min = 0
				}
			}
		}
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", "7120200A"}, [2]string{"codice", "7120200A2"}))
		if d := unico(t, th, motoreConFamiglia(t, f), cSciolto); d.Codice != "7120200A2" || val(d.Revisione) != "2" {
			t.Errorf("codice %q, revisione %s: il codice del cartiglio è quello che porta la revisione", d.Codice, val(d.Revisione))
		}
	})
	t.Run("un PDF con il testo ma senza i campi del cartiglio", func(t *testing.T) {
		th := scenaRevisioni(t)
		f := payloadPDFSenzaCartiglio(t, sha1)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, &f)
		d := unico(t, th, m, cSciolto)
		if d.Cartiglio != valutazione.CartiglioNonLeggibile || !d.Testo || d.Validita != valutazione.ValiditaValido {
			t.Errorf("cartiglio %s, testo %v, validità %s", d.Cartiglio, d.Testo, d.Validita)
		}
	})
	t.Run("due codici nel cartiglio", func(t *testing.T) {
		for _, c := range []struct {
			a, b, codice string
		}{
			{"7120200A2", "7120300A2", ""},
			{"7120200A2", "7120200A3", "7120200A2"},
		} {
			th := scenaRevisioni(t)
			conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", c.a}, [2]string{"codice", c.b}))
			d := unico(t, th, m, cSciolto)
			if e := evidenzaDi(t, d, valutazione.FonteEvidenzaCartiglio); e.Motivo != valutazione.MotivoEvidenzaLettureDiscordanti || e.Valore != nil {
				t.Errorf("%s e %s: %+v", c.a, c.b, e)
			}
			if d.Codice != c.codice {
				t.Errorf("%s e %s: codice %q", c.a, c.b, d.Codice)
			}
		}
	})
	t.Run("documento.rev e la formazione di un altro componente", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "assieme-acme.pdf", sha1, scansione(t, sha1))
		th.Documenti[len(th.Documenti)-1].Rev = testo("B")
		if d := *gruppoDi(t, valuta(t, th, m, nil), cProdotto).Primario; d.RevProvenienza != ancoraggio.RevProvenienzaNonRegistrata {
			t.Errorf("provenienza %s: la formazione «B» è del nodo dello sciolto", d.RevProvenienza)
		}
	})
	t.Run("la formazione da una riga duplicato", func(t *testing.T) {
		th := scenaRevisioni(t)
		rigaDi(t, &th, "#2").Stato = "duplicato"
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		th.Documenti[len(th.Documenti)-1].Rev = testo("B")
		if d := *gruppoDi(t, valuta(t, th, m, nil), cSciolto).Primario; d.RevProvenienza != ancoraggio.RevProvenienzaFormazioneSTEP {
			t.Errorf("provenienza %s", d.RevProvenienza)
		}
	})
}

// famigliaCatenaLettere: la famiglia acme-catena con la revisione di una lettera maiuscola ([base][A][rev], «7120200AC»
// nel cartiglio). È un meccanismo (una revisione che la grammatica tiene com'è scritta, senza cambiare le maiuscole), mai
// il significato di un cliente.
func famigliaCatenaLettere() grammatica.FamigliaCodice {
	f := famigliaCatena()
	f.Revisioni[0].Segmenti[0].Pattern = "[A-Z]"
	for i, e := range f.Esempi {
		switch e.ID {
		case "e-cartiglio", "e-mail":
			f.Esempi[i].Testo, f.Esempi[i].Atteso.Letture[0].Revisione = "7120100AC", "C"
		case "e-nome":
			f.Esempi[i].Testo, f.Esempi[i].Atteso.Letture[0].Revisione = "7120100A_C.stp", "C"
		}
	}
	return f
}

// TestUnTestoNonInterpretatoConUnAltraMaiuscola (T-B5-46, decisione dell'orchestratore [T]; R-14 della revisione della
// fase 2, T-B5-34, T-B5-43): nel cartiglio il testo non interpretato del campo a sé della revisione si confronta con la
// revisione letta in linea esatto dopo gli spazi ai bordi, come stessaRevisione e come ancoraggio: «c» accanto a «C» non
// è la stessa revisione, e l'evidenza è revisione_non_interpretabile, senza valore; «C» accanto a «C» (anche con gli
// spazi ai bordi) resta la revisione C, interpretabile.
func TestUnTestoNonInterpretatoConUnAltraMaiuscola(t *testing.T) {
	m := motoreConFamiglia(t, famigliaCatenaLettere())
	for _, c := range []struct {
		campo, valore  string
		interpretabile bool
		motivo         string
	}{
		{"c", "<nil>", false, valutazione.MotivoEvidenzaRevisioneNonInterpretabile},
		{"C", "C", true, ""},
		{" C ", "C", true, ""},
	} {
		th := scenaAlbero(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1,
			conCartiglio(t, sha1, [2]string{"codice", "7120200AC"}, [2]string{"revisione", c.campo}))
		d := *gruppoDi(t, valuta(t, th, m, nil), cSciolto).Primario
		e := evidenzaDi(t, d, valutazione.FonteEvidenzaCartiglio)
		if val(e.Valore) != c.valore || e.Interpretabile != c.interpretabile || e.Motivo != c.motivo {
			t.Errorf("campo della revisione %q accanto a «C»: %+v", c.campo, e)
		}
	}
}
