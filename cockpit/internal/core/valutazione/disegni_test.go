// L1 — i disegni 2D di un componente (B5, fase 2; R82, R83, R84; T-B0-26, T-B0-30, T-B0-31, T-E1-08, T-E1-21; PO-08,
// PO-09, PO-10, PO-11, PO-12, PO-22 nella parte di B5): la validità sulla regola e dai fatti di oggi, i formati, il
// cartiglio e l'anteprima come tre cose, il gruppo con il primario (con la validità nel criterio del formato) e le
// relazioni da confermare, una voce per contenuto (la voce viva prima del documento non corrente), lo stesso 2D in più
// componenti, il determinismo, i valori e i campi del contratto.
package valutazione_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli ID dei 2D delle prove: gli allegati, i documenti, gli sha256.
var (
	aPDF1  = uid(0x7a1)
	aPDF2  = uid(0x7a2)
	aTIFF  = uid(0x7a3)
	aPNG   = uid(0x7a4)
	aAltro = uid(0x7a5)
	dPDF1  = uid(0x7b1)
	dPDF2  = uid(0x7b2)
	dTIFF  = uid(0x7b3)
	dPNG   = uid(0x7b4)
	dAltro = uid(0x7b5)
	sha1   = sha("1")
	sha2   = sha("2")
	sha3   = sha("3")
	sha4   = sha("4")
)

// ---- la validità e i formati ----

// TestLaValiditaSullaRegola (R83; T-B0-22, T-B0-30, T-B0-31; PO-08 e PO-09 sulla regola): un TIFF o un PNG decodificato è
// valido, come un PDF aperto anche senza testo; un contenuto non decodificato è da verificare con il suo motivo, e un
// motivo che non è della validità vale contenuto_non_verificabile; un formato fuori dall'elenco non è configurato.
func TestLaValiditaSullaRegola(t *testing.T) {
	for _, c := range []struct {
		nome   string
		in     valutazione.ContenutoDisegno
		valid  valutazione.ValiditaDisegno2D
		motivo valutazione.MotivoFabbisogno
	}{
		{"PO-08: un TIFF decodificato", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DTIFF, Decodificato: true}, valutazione.ValiditaValido, ""},
		{"PO-09: un PNG decodificato", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPNG, Decodificato: true}, valutazione.ValiditaValido, ""},
		{"PO-10: un PDF aperto senza testo", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Decodificato: true}, valutazione.ValiditaValido, ""},
		{"un PDF con il testo", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Decodificato: true, Testo: true}, valutazione.ValiditaValido, ""},
		{"il testo senza il contenuto decodificato non basta", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Testo: true},
			valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonVerificabile},
		{"un PDF non analizzato", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Motivo: valutazione.MotivoFabbisognoContenutoNonAnalizzato},
			valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonAnalizzato},
		{"un PDF che non si apre", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Motivo: valutazione.MotivoFabbisognoContenutoNonAperto},
			valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonAperto},
		{"un PDF non letto", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Motivo: valutazione.MotivoFabbisognoContenutoNonLetto},
			valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonLetto},
		{"LD-01: un TIFF senza fatti di decodifica", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DTIFF, Motivo: valutazione.MotivoFabbisognoContenutoNonVerificabile},
			valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonVerificabile},
		{"senza motivo", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPNG}, valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonVerificabile},
		{"un motivo che non è della validità", valutazione.ContenutoDisegno{Formato: valutazione.Formato2DPDF, Motivo: valutazione.MotivoFabbisognoSulPortale},
			valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonVerificabile},
		{"nessun formato, anche decodificato", valutazione.ContenutoDisegno{Decodificato: true}, valutazione.ValiditaFormatoNonConfigurato, valutazione.MotivoFabbisognoFormatoNonConfigurato},
		{"un formato fuori elenco", valutazione.ContenutoDisegno{Formato: "dwg", Decodificato: true}, valutazione.ValiditaFormatoNonConfigurato, valutazione.MotivoFabbisognoFormatoNonConfigurato},
	} {
		if v, m := valutazione.ValiditaDisegno(c.in); v != c.valid || m != c.motivo {
			t.Errorf("%s: %s/%s, atteso %s/%s", c.nome, v, m, c.valid, c.motivo)
		}
	}
}

// TestIFormati (T-B0-30, LD-02): l'elenco chiuso con le estensioni, nell'ordine del criterio 3; il formato
// dall'estensione dichiarata, senza il punto, senza maiuscole e senza gli spazi ai bordi; DWG, DFT, JPG non configurati;
// l'elenco non si cambia da fuori; VersioneFormati2D = 1.
func TestIFormati(t *testing.T) {
	for est, atteso := range map[string]valutazione.Formato2D{"pdf": "pdf", "PDF": "pdf", ".pdf": "pdf", " tif ": "tiff", "tiff": "tiff", "TIF": "tiff",
		"png": "png", "dwg": "", "dft": "", "jpg": "", "jpeg": "", "": "", "pdfx": ""} {
		if f := valutazione.FormatoDi(est); f != atteso {
			t.Errorf("FormatoDi(%q) = %q, atteso %q", est, f, atteso)
		}
	}
	elenco := valutazione.FormatiDisegno2D()
	if canonicoDi(t, elenco) != `[{"estensioni":["pdf"],"formato":"pdf"},{"estensioni":["tif","tiff"],"formato":"tiff"},{"estensioni":["png"],"formato":"png"}]` {
		t.Errorf("elenco %s", canonicoDi(t, elenco))
	}
	elenco[0].Formato, elenco[1].Estensioni[0] = "dwg", "dwg"
	if valutazione.FormatoDi("tif") != valutazione.Formato2DTIFF || valutazione.FormatiDisegno2D()[0].Formato != valutazione.Formato2DPDF {
		t.Error("l'elenco dei formati si cambia da fuori")
	}
	if valutazione.VersioneFormati2D != 1 {
		t.Errorf("VersioneFormati2D %d", valutazione.VersioneFormati2D)
	}
}

// TestLaValiditaDaiFattiDiOggi (T-B0-31, T-09, T-04; LD-01, LD-02, LD-03): l'adattatore dei fatti, un 2D per caso sullo
// sciolto della scena: un PDF con il testo è valido con il cartiglio letto; errore_pdf, i fatti senza testo né errore,
// nessun fatto sono da verificare con il loro motivo; DWG e JPG non sono configurati, senza lettura del cartiglio né
// anteprima; un documento senza allegato nel thread si legge dai suoi fatti, e uno il cui allegato non c'è dal file del
// thread con lo stesso contenuto. L'anteprima c'è per ogni PDF tranne quello che il worker non apre (errore_pdf,
// contenuto_non_aperto: T-B5-38, riscritta dopo la revisione della fase 2).
func TestLaValiditaDaiFattiDiOggi(t *testing.T) {
	th := scenaAlbero(t)
	type caso struct {
		sha, nome, est string
		fatti          *fotorfq.Fatti
		valid          valutazione.ValiditaDisegno2D
		motivo         valutazione.MotivoFabbisogno
		cartiglio      string
		testo          bool
	}
	testoOk := fattiPDFCampi(t, sha("1"), [2]string{"codice", "7120200A2"})
	errore, soloEsito := fattiPDFErrore(t, sha("2")), fattiSoloEsito(t, sha("3"))
	casi := []caso{
		{sha("1"), "disegno-1.pdf", "pdf", &testoOk, valutazione.ValiditaValido, "", valutazione.CartiglioLetto, true},
		{sha("2"), "disegno-2.pdf", "pdf", &errore, valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonAperto, valutazione.CartiglioNonLeggibile, false},
		{sha("3"), "disegno-3.pdf", "pdf", &soloEsito, valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonLetto, valutazione.CartiglioNonLeggibile, false},
		{sha("4"), "disegno-4.pdf", "pdf", nil, valutazione.ValiditaDaVerificare, valutazione.MotivoFabbisognoContenutoNonAnalizzato, valutazione.CartiglioNonLeggibile, false},
		{sha("5"), "disegno-5.dwg", "dwg", nil, valutazione.ValiditaFormatoNonConfigurato, valutazione.MotivoFabbisognoFormatoNonConfigurato, valutazione.CartiglioFormatoSenzaLettura, false},
		{sha("6"), "disegno-6.JPG", "JPG", nil, valutazione.ValiditaFormatoNonConfigurato, valutazione.MotivoFabbisognoFormatoNonConfigurato, valutazione.CartiglioFormatoSenzaLettura, false},
	}
	for i, c := range casi {
		a := allegato(uid(0x7c0+i), int16(10+i), c.nome, c.est, c.sha)
		d := documento2D(uid(0x7d0+i), cSciolto, a.ID, c.nome, c.est, c.sha)
		conFile2D(&th, a, c.fatti, &d, uuid.Nil)
	}
	// Un documento senza allegato nel thread, con i fatti del documento (T-04).
	senza := fattiPDFCampi(t, sha("7"), [2]string{"codice", "7120200A2"})
	th.Fatti[sha("7")] = senza
	th.Documenti = append(th.Documenti, documento2D(uid(0x7d7), cSciolto, uuid.Nil, "disegno-7.pdf", "pdf", sha("7")))
	// Un documento con un allegato di un altro thread, e nel thread un file con lo stesso contenuto.
	copia := fattiPDFCampi(t, sha("8"), [2]string{"codice", "7120200A2"})
	conAllegato(&th, allegato(uid(0x7c8), 18, "copia.pdf", "pdf", sha("8")), &copia)
	th.Documenti = append(th.Documenti, documento2D(uid(0x7d8), cSciolto, uid(0x7e8), "disegno-8.pdf", "pdf", sha("8")))

	g := gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto)
	for i, c := range casi {
		d := disegnoDi(t, g, c.sha)
		if d.Validita != c.valid || d.MotivoValidita != c.motivo || d.Cartiglio != c.cartiglio || d.Testo != c.testo {
			t.Errorf("%s: validità %s/%s, cartiglio %s, testo %v", c.nome, d.Validita, d.MotivoValidita, d.Cartiglio, d.Testo)
		}
		if d.DocumentoID == nil || *d.DocumentoID != uid(0x7d0+i) || d.AllegatoID == nil || *d.AllegatoID != uid(0x7c0+i) || d.NomeFile != c.nome ||
			d.Provenienza != ancoraggio.OrigineConfermato || !d.Corrente || d.ConfermatoIl == nil || !d.ConfermatoIl.Equal(dataACME.Add(time.Hour)) {
			t.Errorf("%s: il file %+v", c.nome, d)
		}
		pdf := c.est == "pdf" && c.motivo != valutazione.MotivoFabbisognoContenutoNonAperto
		if d.Anteprima.Disponibile != pdf || d.Anteprima.Sha256 != c.sha || d.Anteprima.DocumentoID == nil || *d.Anteprima.DocumentoID != uid(0x7d0+i) ||
			d.Anteprima.AllegatoID == nil || *d.Anteprima.AllegatoID != uid(0x7c0+i) || d.Anteprima.Formato != d.Formato {
			t.Errorf("%s: anteprima %+v (LD-03: solo il PDF che si apre)", c.nome, d.Anteprima)
		}
	}
	if d := disegnoDi(t, g, sha("6")); d.Estensione != "jpg" || d.Formato != "" {
		t.Errorf("JPG: estensione %q, formato %q", d.Estensione, d.Formato)
	}
	for _, s := range []string{sha("7"), sha("8")} {
		if d := disegnoDi(t, g, s); d.Validita != valutazione.ValiditaValido || d.Cartiglio != valutazione.CartiglioLetto || d.AllegatoID != nil || d.DocumentoID == nil {
			t.Errorf("il documento %s senza il suo allegato nel thread: %+v", s[:4], d)
		}
	}
}

// ---- PO-08, PO-09, PO-10 ----

// TestPO08IlSoloTIFF (PO-08; R83, T-E1-21, LD-01, LD-04): un componente con un TIFF solo. Sulla regola, con un contenuto
// decodificato, il TIFF è valido senza cartiglio: validità e cartiglio sono due cose. Con i fatti di oggi: da_verificare,
// contenuto_non_verificabile, cartiglio formato_senza_lettura, nessuna anteprima, e la riconciliazione del nodo
// non_verificabile; la revisione viene dal nome del file. La prima fotografia del percorso manuale ha «assegna», la
// seconda il documento confermato: l'associazione diventa confermata e corrente (la voce del fabbisogno la completa la
// fase 3).
func TestPO08IlSoloTIFF(t *testing.T) {
	m := motoreCatena(t)
	t.Run("sulla regola: valido, senza cartiglio", func(t *testing.T) {
		v, motivo := valutazione.ValiditaDisegno(valutazione.ContenutoDisegno{Formato: valutazione.FormatoDi("tif"), Decodificato: true})
		in := valutazione.DisegnoDaValutare{Disegno: valutazione.Disegno2D{Sha256: sha1, Formato: valutazione.Formato2DTIFF, Validita: v, MotivoValidita: motivo,
			Cartiglio: valutazione.CartiglioFormatoSenzaLettura, Provenienza: ancoraggio.OrigineConfermato, Corrente: true}}
		d, _ := valutazione.ValutaDisegno(in, valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto}, nil)
		if d.Validita != valutazione.ValiditaValido || d.MotivoValidita != "" || d.Cartiglio != valutazione.CartiglioFormatoSenzaLettura {
			t.Errorf("il TIFF decodificato %+v", d)
		}
	})
	for _, foto := range []string{"prima fotografia: assegna", "seconda fotografia: il documento confermato"} {
		t.Run(foto, func(t *testing.T) {
			th := scenaAlbero(t)
			a := allegato(aTIFF, 3, "7120200A_2.tif", "tif", sha3)
			if foto[0] == 'p' {
				conFile2D(&th, a, nil, nil, cSciolto)
			} else {
				d := documento2D(dTIFF, cSciolto, aTIFF, "7120200A_2.tif", "tif", sha3)
				conFile2D(&th, a, nil, &d, uuid.Nil)
			}
			v := valuta(t, th, m, nil)
			g := gruppoDi(t, v, cSciolto)
			if len(tuttiDi(g)) != 1 {
				t.Fatalf("gruppo %+v", g)
			}
			d := *g.Primario
			if d.Formato != valutazione.Formato2DTIFF || d.Estensione != "tif" || d.Validita != valutazione.ValiditaDaVerificare ||
				d.MotivoValidita != valutazione.MotivoFabbisognoContenutoNonVerificabile || d.Cartiglio != valutazione.CartiglioFormatoSenzaLettura || d.Testo ||
				d.Anteprima.Disponibile || d.Anteprima.Formato != valutazione.Formato2DTIFF {
				t.Errorf("il TIFF %+v", d)
			}
			if p := d.RevisioneProposta; val(p.Valore) != "2" || p.Fonte != valutazione.FonteEvidenzaNomeFile || p.Motivo != valutazione.MotivoPropostaRevisioneNonDeterminata {
				t.Errorf("la proposta %+v: il nome del file, perché il nodo dello STEP non ha la revisione", p)
			}
			if e := evidenzaDi(t, d, valutazione.FonteEvidenzaCartiglio); e.Motivo != valutazione.MotivoEvidenzaFormatoSenzaLettura || e.Valore != nil {
				t.Errorf("il cartiglio %+v", e)
			}
			manuale := foto[0] == 'p'
			if manuale && (d.Provenienza != ancoraggio.OrigineManuale || d.Corrente || d.ConfermatoIl != nil || d.DocumentoID != nil) {
				t.Errorf("assegna: %+v", d)
			}
			if !manuale && (d.Provenienza != ancoraggio.OrigineConfermato || !d.Corrente || d.DocumentoID == nil || *d.DocumentoID != dTIFF) {
				t.Errorf("il documento confermato: %+v", d)
			}
			n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
			if len(n.Codice.Documentale) != 1 || n.Codice.Documentale[0].Esito != ancoraggio.RiconciliazioneNonVerificabile ||
				n.Codice.Documentale[0].Motivo != ancoraggio.MotivoDocumentaleCartiglioNonLetto {
				t.Errorf("riconciliazione %+v (LD-04)", n.Codice.Documentale)
			}
		})
	}
}

// TestPO09IlSoloPNG (PO-09, come PO-08; LD-01): un PNG con i fatti del worker che non lo decodifica (il solo esito):
// da_verificare, contenuto_non_verificabile, senza lettura del cartiglio né anteprima.
func TestPO09IlSoloPNG(t *testing.T) {
	th := scenaAlbero(t)
	f := fattiSoloEsito(t, sha4)
	d := documento2D(dPNG, cSciolto, aPNG, "7120200A_2.png", "png", sha4)
	conFile2D(&th, allegato(aPNG, 4, "7120200A_2.png", "png", sha4), &f, &d, uuid.Nil)
	g := gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto)
	p := *g.Primario
	if p.Formato != valutazione.Formato2DPNG || p.Validita != valutazione.ValiditaDaVerificare || p.MotivoValidita != valutazione.MotivoFabbisognoContenutoNonVerificabile ||
		p.Cartiglio != valutazione.CartiglioFormatoSenzaLettura || p.Anteprima.Disponibile || !p.Corrente {
		t.Errorf("il PNG %+v", p)
	}
	if v, _ := valutazione.ValiditaDisegno(valutazione.ContenutoDisegno{Formato: valutazione.FormatoDi("png"), Decodificato: true}); v != valutazione.ValiditaValido {
		t.Errorf("sulla regola, un PNG decodificato: %s", v)
	}
}

// TestPO10IlPDFScansionato (PO-10; R83, T-B0-31, T-E1-21, LD-04): un PDF scansionato senza OCR è valido, senza testo,
// con il cartiglio non leggibile e l'anteprima; la riconciliazione del nodo è non_verificabile.
func TestPO10IlPDFScansionato(t *testing.T) {
	th := scenaAlbero(t)
	f := fatti(t, sha1, payloadPDF(t, false), nil)
	d := documento2D(dPDF1, cSciolto, aPDF1, "scansione-acme.pdf", "pdf", sha1)
	conFile2D(&th, allegato(aPDF1, 3, "scansione-acme.pdf", "pdf", sha1), &f, &d, uuid.Nil)
	v := valuta(t, th, motoreCatena(t), nil)
	p := *gruppoDi(t, v, cSciolto).Primario
	if p.Validita != valutazione.ValiditaValido || p.MotivoValidita != "" || p.Testo || p.Cartiglio != valutazione.CartiglioNonLeggibile || !p.Anteprima.Disponibile {
		t.Errorf("la scansione %+v", p)
	}
	if e := evidenzaDi(t, p, valutazione.FonteEvidenzaCartiglio); e.Motivo != valutazione.MotivoEvidenzaCartiglioNonLeggibile || e.Valore != nil || e.Interpretabile {
		t.Errorf("il cartiglio %+v", e)
	}
	n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
	if len(n.Codice.Documentale) != 1 || n.Codice.Documentale[0].Esito != ancoraggio.RiconciliazioneNonVerificabile {
		t.Errorf("riconciliazione %+v", n.Codice.Documentale)
	}
}

// ---- PO-11, PO-12, PO-22: il gruppo e il primario ----

// scenaDueDisegni: la scena dell'albero con lo sciolto alla revisione registrata data (vuota: nessuna) e due 2D
// confermati sullo sciolto: un PDF con il cartiglio dato e un TIFF con il nome dato.
func scenaDueDisegni(t *testing.T, rev, cartiglio, nomeTIFF string) fotorfq.Thread {
	t.Helper()
	th := scenaAlbero(t)
	if rev != "" {
		th.Componenti[1].Rev = testo(rev)
	}
	f := fattiPDFCampi(t, sha1, [2]string{"codice", cartiglio})
	pdf := documento2D(dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", "pdf", sha1)
	conFile2D(&th, allegato(aPDF1, 3, "disegno-acme.pdf", "pdf", sha1), &f, &pdf, uuid.Nil)
	tif := documento2D(dTIFF, cSciolto, aTIFF, nomeTIFF, "tif", sha3)
	conFile2D(&th, allegato(aTIFF, 4, nomeTIFF, "tif", sha3), nil, &tif, uuid.Nil)
	return th
}

// TestPO11PDFETIFFDellaStessaRevisione (PO-11; R84, T-E1-08): un PDF e un TIFF dello sciolto, della stessa revisione:
// tutti e due restano, il primario è il PDF per il formato, il TIFF è l'alternativo, e la relazione è
// rappresentazione_alternativa, da confermare.
func TestPO11PDFETIFFDellaStessaRevisione(t *testing.T) {
	th := scenaDueDisegni(t, "2", "7120200A2", "7120200A_2.tif")
	g := gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto)
	if g.Primario == nil || g.Primario.Sha256 != sha1 || len(g.Alternativi) != 1 || g.Alternativi[0].Sha256 != sha3 || g.RegolaPrimario != valutazione.RegolaPrimarioFormato {
		t.Fatalf("gruppo %+v", g)
	}
	for _, d := range tuttiDi(g) {
		if d.CompatibilitaCodice != motorea.CompatibilitaUguale || d.CompatibilitaRevisione != motorea.CompatibilitaUguale || val(d.Revisione) != "2" ||
			d.ConfrontatoCon != valutazione.ConfrontatoConDeciso || d.Codice == "" {
			t.Errorf("%s: codice %s %q, revisione %s %s, confrontato con %s", d.NomeFile, d.CompatibilitaCodice, d.Codice, d.CompatibilitaRevisione, val(d.Revisione), d.ConfrontatoCon)
		}
	}
	if g.Primario.FonteIdentita != valutazione.FonteIdentitaCartiglio || g.Alternativi[0].FonteIdentita != valutazione.FonteIdentitaNomeFile {
		t.Errorf("fonti dell'identità %s, %s", g.Primario.FonteIdentita, g.Alternativi[0].FonteIdentita)
	}
	if !reflect.DeepEqual(g.RelazioniDaConfermare, []valutazione.RelazioneDisegni{{A: sha1, B: sha3, Tipo: valutazione.RelazioneRappresentazioneAlternativa,
		Stato: valutazione.StatoRelazioneDaConfermare}}) {
		t.Errorf("relazioni %+v", g.RelazioniDaConfermare)
	}
}

// TestPO12IlPDFVecchioEIlTIFFDellaRevisioneGiusta (PO-12; R84, T-B0-34): il PDF della revisione precedente e il TIFF della
// revisione corrente: primario il TIFF, per la compatibilità; con il PDF sostituito, per il criterio del corrente. Un PDF
// vecchio non vince per il formato.
func TestPO12IlPDFVecchioEIlTIFFDellaRevisioneGiusta(t *testing.T) {
	m := motoreCatena(t)
	t.Run("la revisione precedente", func(t *testing.T) {
		g := gruppoDi(t, valuta(t, scenaDueDisegni(t, "2", "7120200A1", "7120200A_2.tif"), m, nil), cSciolto)
		if g.Primario.Sha256 != sha3 || g.RegolaPrimario != valutazione.RegolaPrimarioCompatibilita ||
			g.Alternativi[0].CompatibilitaRevisione != motorea.CompatibilitaDiscordante {
			t.Fatalf("gruppo %+v", g)
		}
		if len(g.RelazioniDaConfermare) != 1 || g.RelazioniDaConfermare[0].Tipo != valutazione.RelazioneRevisioneDiversa {
			t.Errorf("relazioni %+v", g.RelazioniDaConfermare)
		}
	})
	t.Run("il PDF sostituito", func(t *testing.T) {
		th := scenaDueDisegni(t, "2", "7120200A2", "7120200A_2.tif")
		sost := dTIFF
		th.Documenti[1].SostituitoDa = &sost
		g := gruppoDi(t, valuta(t, th, m, nil), cSciolto)
		if g.Primario.Sha256 != sha3 || g.RegolaPrimario != valutazione.RegolaPrimarioCorrente || g.Alternativi[0].Corrente {
			t.Fatalf("gruppo %+v", g)
		}
	})
}

// TestPO22DueRappresentazioniEquivalenti (PO-22, parte B5; T-E1-08): il PDF confermato, poi un TIFF della stessa
// revisione solo proposto: la relazione rappresentazione_alternativa da confermare, il primario il PDF confermato; nessuna
// revisione cambia. La variante: un secondo PDF della stessa revisione con un altro contenuto: non_determinata, con le
// letture elaborato_aggiuntivo, revisione_diversa, riemissione.
func TestPO22DueRappresentazioniEquivalenti(t *testing.T) {
	m := motoreCatena(t)
	base := func(t *testing.T) fotorfq.Thread {
		th := scenaAlbero(t)
		f := fattiPDFCampi(t, sha1, [2]string{"codice", "7120200A2"})
		pdf := documento2D(dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", "pdf", sha1)
		conFile2D(&th, allegato(aPDF1, 3, "disegno-acme.pdf", "pdf", sha1), &f, &pdf, uuid.Nil)
		return th
	}
	t.Run("il TIFF della stessa revisione", func(t *testing.T) {
		th := base(t)
		conFile2D(&th, allegato(aTIFF, 4, "7120200A_2.tif", "tif", sha3), nil, nil, uuid.Nil)
		g := gruppoDi(t, valuta(t, th, m, nil), cSciolto)
		if g.Primario.Sha256 != sha1 || g.RegolaPrimario != valutazione.RegolaPrimarioCorrente || g.Alternativi[0].Provenienza != ancoraggio.OrigineProposto {
			t.Fatalf("gruppo %+v", g)
		}
		if len(g.RelazioniDaConfermare) != 1 || g.RelazioniDaConfermare[0].Tipo != valutazione.RelazioneRappresentazioneAlternativa ||
			g.RelazioniDaConfermare[0].Stato != valutazione.StatoRelazioneDaConfermare {
			t.Errorf("relazioni %+v", g.RelazioniDaConfermare)
		}
		if val(g.Primario.Revisione) != "2" || val(g.Alternativi[0].Revisione) != "2" {
			t.Errorf("revisioni %s, %s", val(g.Primario.Revisione), val(g.Alternativi[0].Revisione))
		}
	})
	t.Run("un secondo PDF della stessa revisione", func(t *testing.T) {
		th := base(t)
		f := fattiPDFCampi(t, sha2, [2]string{"codice", "7120200A2"}, [2]string{"titolo", "VISTA DI DETTAGLIO"})
		d := documento2D(dPDF2, cSciolto, aPDF2, "dettaglio-acme.pdf", "pdf", sha2)
		conFile2D(&th, allegato(aPDF2, 5, "dettaglio-acme.pdf", "pdf", sha2), &f, &d, uuid.Nil)
		g := gruppoDi(t, valuta(t, th, m, nil), cSciolto)
		if !reflect.DeepEqual(g.RelazioniDaConfermare, []valutazione.RelazioneDisegni{{A: sha1, B: sha2, Tipo: valutazione.RelazioneNonDeterminata,
			Letture: []string{valutazione.RelazioneElaboratoAggiuntivo, valutazione.RelazioneRevisioneDiversa, valutazione.LetturaRiemissione},
			Stato:   valutazione.StatoRelazioneDaConfermare}}) {
			t.Errorf("relazioni %+v", g.RelazioniDaConfermare)
		}
	})
}

// ---- il primario e le relazioni sulla regola ----

// disegnoSintetico: un 2D per le prove della regola, con lo sha256, il formato, la revisione e il codice compatibili dati.
func disegnoSintetico(s string, f valutazione.Formato2D, rev string, codice motorea.Compatibilita) valutazione.Disegno2D {
	d := valutazione.Disegno2D{Sha256: s, Formato: f, CompatibilitaCodice: codice, CompatibilitaRevisione: motorea.CompatibilitaNonDeterminabile,
		Provenienza: ancoraggio.OrigineProposto}
	if rev != "" {
		d.Revisione = testo(rev)
	}
	return d
}

// TestIlPrimarioSullaRegola (R84, T-B0-26, T-E1R-06): i criteri uno per uno, ognuno con la sua regola: il corrente e
// confermato, la compatibilità del codice e poi della revisione (uguale, parziale, non determinabile, discordante), il
// formato (prima la validità, poi PDF, TIFF, PNG, un formato non configurato dopo il PNG: T-B5-39, con la precisazione
// E1 su R84), il confermato più di recente, lo sha256; la validità non viene prima del corrente né della
// compatibilità; un 2D solo non ha una regola; un gruppo vuoto non ha primario; l'ingresso non cambia.
func TestIlPrimarioSullaRegola(t *testing.T) {
	ieri, oggi := dataACME, dataACME.Add(24*time.Hour)
	conferma := func(d valutazione.Disegno2D, il *time.Time, corrente bool) valutazione.Disegno2D {
		d.Provenienza, d.ConfermatoIl, d.Corrente = ancoraggio.OrigineConfermato, il, corrente
		return d
	}
	conRev := func(d valutazione.Disegno2D, c motorea.Compatibilita) valutazione.Disegno2D {
		d.CompatibilitaRevisione = c
		return d
	}
	conValidita := func(d valutazione.Disegno2D, v valutazione.ValiditaDisegno2D) valutazione.Disegno2D {
		d.Validita = v
		return d
	}
	nd := motorea.CompatibilitaNonDeterminabile
	valido, daVerificare, nonConf := valutazione.ValiditaValido, valutazione.ValiditaDaVerificare, valutazione.ValiditaFormatoNonConfigurato
	for _, c := range []struct {
		nome         string
		primo, altro valutazione.Disegno2D
		regola       string
	}{
		{"il corrente e confermato prima di un candidato PDF", conferma(disegnoSintetico(sha2, "png", "", nd), &ieri, true), disegnoSintetico(sha1, "pdf", "", nd), valutazione.RegolaPrimarioCorrente},
		{"il corrente prima del sostituito", conferma(disegnoSintetico(sha2, "tiff", "", nd), &ieri, true), conferma(disegnoSintetico(sha1, "pdf", "", nd), &oggi, false), valutazione.RegolaPrimarioCorrente},
		{"il codice uguale prima del parziale", disegnoSintetico(sha2, "png", "", motorea.CompatibilitaUguale), disegnoSintetico(sha1, "pdf", "", motorea.CompatibilitaParziale), valutazione.RegolaPrimarioCompatibilita},
		{"il parziale prima del non determinabile", disegnoSintetico(sha2, "png", "", motorea.CompatibilitaParziale), disegnoSintetico(sha1, "pdf", "", nd), valutazione.RegolaPrimarioCompatibilita},
		{"il non determinabile prima del discordante", disegnoSintetico(sha2, "png", "", nd), disegnoSintetico(sha1, "pdf", "", motorea.CompatibilitaDiscordante), valutazione.RegolaPrimarioCompatibilita},
		{"PO-38: la revisione giusta prima di nessuna revisione", conRev(disegnoSintetico(sha2, "tiff", "3", nd), motorea.CompatibilitaUguale), disegnoSintetico(sha1, "pdf", "", nd), valutazione.RegolaPrimarioCompatibilita},
		{"l'equivalente come l'uguale, poi il formato", conRev(disegnoSintetico(sha2, "pdf", "3", nd), motorea.CompatibilitaEquivalente), conRev(disegnoSintetico(sha1, "tiff", "3", nd), motorea.CompatibilitaUguale), valutazione.RegolaPrimarioFormato},
		{"il PDF prima del TIFF", disegnoSintetico(sha2, "pdf", "", nd), disegnoSintetico(sha1, "tiff", "", nd), valutazione.RegolaPrimarioFormato},
		{"il TIFF prima del PNG", disegnoSintetico(sha2, "tiff", "", nd), disegnoSintetico(sha1, "png", "", nd), valutazione.RegolaPrimarioFormato},
		{"il PNG prima di un formato non configurato", disegnoSintetico(sha2, "png", "", nd), disegnoSintetico(sha1, "", "", nd), valutazione.RegolaPrimarioFormato},
		{"un TIFF valido prima di un PDF da verificare", conValidita(disegnoSintetico(sha2, "tiff", "", nd), valido),
			conValidita(disegnoSintetico(sha1, "pdf", "", nd), daVerificare), valutazione.RegolaPrimarioFormato},
		{"un PNG da verificare prima di un formato non configurato", conValidita(disegnoSintetico(sha2, "png", "", nd), daVerificare),
			conValidita(disegnoSintetico(sha1, "pdf", "", nd), nonConf), valutazione.RegolaPrimarioFormato},
		{"il valido prima del più recente", conValidita(conferma(disegnoSintetico(sha2, "pdf", "", nd), &ieri, true), valido),
			conValidita(conferma(disegnoSintetico(sha1, "pdf", "", nd), &oggi, true), daVerificare), valutazione.RegolaPrimarioFormato},
		{"la compatibilità prima della validità", conValidita(disegnoSintetico(sha2, "pdf", "", motorea.CompatibilitaUguale), daVerificare),
			conValidita(disegnoSintetico(sha1, "pdf", "", nd), valido), valutazione.RegolaPrimarioCompatibilita},
		{"il corrente prima della validità: un DWG confermato prima di un PDF valido proposto", conValidita(conferma(disegnoSintetico(sha2, "", "", nd), &ieri, true), nonConf),
			conValidita(disegnoSintetico(sha1, "pdf", "", nd), valido), valutazione.RegolaPrimarioCorrente},
		{"il confermato più di recente", conferma(disegnoSintetico(sha2, "pdf", "", nd), &oggi, true), conferma(disegnoSintetico(sha1, "pdf", "", nd), &ieri, true), valutazione.RegolaPrimarioRecente},
		{"un confermato prima di uno senza data", conferma(disegnoSintetico(sha2, "pdf", "", nd), &ieri, false), disegnoSintetico(sha1, "pdf", "", nd), valutazione.RegolaPrimarioRecente},
		{"a parità, lo sha256", disegnoSintetico(sha1, "pdf", "", nd), disegnoSintetico(sha2, "pdf", "", nd), valutazione.RegolaPrimarioSha256},
	} {
		for _, ordine := range [][]valutazione.Disegno2D{{c.primo, c.altro}, {c.altro, c.primo}} {
			prima := canonicoDi(t, ordine)
			g := valutazione.Primario(ordine)
			if g.Primario == nil || g.Primario.Sha256 != c.primo.Sha256 || len(g.Alternativi) != 1 || g.Alternativi[0].Sha256 != c.altro.Sha256 || g.RegolaPrimario != c.regola {
				t.Errorf("%s: primario %v, regola %q", c.nome, g.Primario, g.RegolaPrimario)
			}
			if canonicoDi(t, ordine) != prima {
				t.Errorf("%s: l'ingresso è cambiato", c.nome)
			}
		}
	}
	if g := valutazione.Primario([]valutazione.Disegno2D{disegnoSintetico(sha1, "pdf", "", nd)}); g.Primario == nil || g.RegolaPrimario != "" || len(g.RelazioniDaConfermare) != 0 {
		t.Errorf("un 2D solo: %+v", g)
	}
	if g := valutazione.Primario(nil); g.Primario != nil || g.Alternativi != nil || g.RegolaPrimario != "" {
		t.Errorf("nessun 2D: %+v", g)
	}
	// Senza sha256: l'ordine fisso dal documento e dall'allegato, e nessuna relazione.
	a, b := disegnoSintetico("", "pdf", "", nd), disegnoSintetico("", "pdf", "", nd)
	da, db := uid(0x7f1), uid(0x7f2)
	a.DocumentoID, b.AllegatoID = &db, &da
	for _, ordine := range [][]valutazione.Disegno2D{{a, b}, {b, a}} {
		if g := valutazione.Primario(ordine); g.Primario.AllegatoID == nil || g.RegolaPrimario != valutazione.RegolaPrimarioSha256 || len(g.RelazioniDaConfermare) != 0 {
			t.Errorf("senza sha256: %+v", g)
		}
	}
}

// TestLeRelazioniSullaRegola (T-E1-08): il tipo solo quando le evidenze lo sostengono: revisioni diverse, senza un
// codice discordante; la stessa identità e revisione con un formato diverso; la stessa identità, revisione e formato;
// altrimenti non_determinata con le letture possibili, con lo stesso formato o con un formato diverso (anche con le
// revisioni uguali ma l'identità che non si sa, e con le revisioni diverse ma un codice discordante). Le revisioni si
// confrontano esatte dopo gli spazi ai bordi: «a» e «A» sono due revisioni (T-B5-34). Sempre da confermare; A < B; mai
// fra due voci con lo stesso sha256. Riscritta dopo la revisione della fase 2 (il codice discordante, T-B5-34).
func TestLeRelazioniSullaRegola(t *testing.T) {
	u, nd, disc := motorea.CompatibilitaUguale, motorea.CompatibilitaNonDeterminabile, motorea.CompatibilitaDiscordante
	stessoFormato := []string{valutazione.RelazioneElaboratoAggiuntivo, valutazione.RelazioneRevisioneDiversa, valutazione.LetturaRiemissione}
	altroFormato := []string{valutazione.RelazioneRappresentazioneAlternativa, valutazione.RelazioneRevisioneDiversa, valutazione.RelazioneElaboratoAggiuntivo}
	for _, c := range []struct {
		nome    string
		a, b    valutazione.Disegno2D
		tipo    string
		letture []string
	}{
		{"revisioni diverse", disegnoSintetico(sha2, "pdf", "1", u), disegnoSintetico(sha1, "pdf", "2", u), valutazione.RelazioneRevisioneDiversa, nil},
		{"revisioni diverse con l'identità che non si sa", disegnoSintetico(sha2, "pdf", "1", u), disegnoSintetico(sha1, "tiff", "2", nd), valutazione.RelazioneRevisioneDiversa, nil},
		{"revisioni diverse con un codice discordante", disegnoSintetico(sha2, "pdf", "1", u), disegnoSintetico(sha1, "pdf", "2", disc), valutazione.RelazioneNonDeterminata, stessoFormato},
		{"revisioni uguali dopo gli spazi ai bordi", disegnoSintetico(sha2, "pdf", "b", u), disegnoSintetico(sha1, "tiff", " b ", u), valutazione.RelazioneRappresentazioneAlternativa, nil},
		{"«a» e «A» sono due revisioni", disegnoSintetico(sha2, "pdf", "a", u), disegnoSintetico(sha1, "tiff", "A", u), valutazione.RelazioneRevisioneDiversa, nil},
		{"stessa identità, revisione e formato", disegnoSintetico(sha2, "tiff", "2", u), disegnoSintetico(sha1, "tiff", "2", u), valutazione.RelazioneNonDeterminata, stessoFormato},
		{"una revisione che non si sa, stesso formato", disegnoSintetico(sha2, "pdf", "2", u), disegnoSintetico(sha1, "pdf", "", u), valutazione.RelazioneNonDeterminata, stessoFormato},
		{"una revisione che non si sa, altro formato", disegnoSintetico(sha2, "pdf", "", u), disegnoSintetico(sha1, "png", "2", u), valutazione.RelazioneNonDeterminata, altroFormato},
		{"l'identità che non si sa", disegnoSintetico(sha2, "pdf", "2", nd), disegnoSintetico(sha1, "tiff", "2", u), valutazione.RelazioneNonDeterminata, altroFormato},
		{"un codice discordante", disegnoSintetico(sha2, "pdf", "2", u), disegnoSintetico(sha1, "tiff", "2", disc), valutazione.RelazioneNonDeterminata, altroFormato},
	} {
		g := valutazione.Primario([]valutazione.Disegno2D{c.a, c.b})
		want := []valutazione.RelazioneDisegni{{A: sha1, B: sha2, Tipo: c.tipo, Letture: c.letture, Stato: valutazione.StatoRelazioneDaConfermare}}
		if !reflect.DeepEqual(g.RelazioniDaConfermare, want) {
			t.Errorf("%s: %+v", c.nome, g.RelazioniDaConfermare)
		}
	}
	tre := valutazione.Primario([]valutazione.Disegno2D{disegnoSintetico(sha3, "pdf", "1", u), disegnoSintetico(sha1, "pdf", "1", u), disegnoSintetico(sha2, "pdf", "1", u)})
	var coppie []string
	for _, r := range tre.RelazioniDaConfermare {
		coppie = append(coppie, r.A[:1]+r.B[:1])
	}
	if !reflect.DeepEqual(coppie, []string{"12", "13", "23"}) {
		t.Errorf("tre 2D: coppie %v", coppie)
	}
	if g := valutazione.Primario([]valutazione.Disegno2D{disegnoSintetico(sha1, "pdf", "1", u), disegnoSintetico(sha1, "tiff", "1", u)}); len(g.RelazioniDaConfermare) != 0 {
		t.Errorf("lo stesso contenuto: %+v", g.RelazioniDaConfermare)
	}
}

// ---- il gruppo dalla fotografia ----

// TestUnContenutoPerGruppoEPiuComponenti (R84, T-B5-96): il documento e il suo file candidato sul nodo dello stesso
// componente sono una voce sola, quella confermata; lo stesso file portato da due documenti su due componenti sta nei
// gruppi di tutti e due, senza il presupposto di un componente solo; «assegna» su un file con un documento dello stesso
// contenuto resta il documento; un documento su un componente archiviato non fa un gruppo; un file che non è un 2D non
// entra.
func TestUnContenutoPerGruppoEPiuComponenti(t *testing.T) {
	th := scenaAlbero(t)
	f := fattiPDFCampi(t, sha1, [2]string{"codice", "7120200A2"})
	d1 := documento2D(dPDF1, cSciolto, aPDF1, "7120200A_2.pdf", "pdf", sha1)
	conFile2D(&th, allegato(aPDF1, 3, "7120200A_2.pdf", "pdf", sha1), &f, &d1, uuid.Nil)
	th.Documenti = append(th.Documenti, documento2D(dPDF2, cProdotto, aPDF1, "7120200A_2.pdf", "pdf", sha1))
	cs := cSciolto
	th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: uid(0x7f5), AllegatoID: aPDF1, Tipo: "disegno_2d", ComponenteID: &cs, Fonte: "operatore", Stato: "aperta"})
	archiviato := uid(0x7f6)
	quando := dataACME
	th.Componenti = append(th.Componenti, fotorfq.Componente{ID: archiviato, Codice: "7120900A", Tipo: "sciolto", Origine: "step", ConfermatoDa: operatore,
		CreatoIl: dataACME, ArchiviatoIl: &quando})
	g := fattiPDFCampi(t, sha2, [2]string{"codice", "7120900A1"})
	da := documento2D(dAltro, archiviato, aAltro, "7120900A_1.pdf", "pdf", sha2)
	conFile2D(&th, allegato(aAltro, 4, "7120900A_1.pdf", "pdf", sha2), &g, &da, uuid.Nil)
	// Un file che non è un 2D, con il codice dello sciolto nel nome.
	conAllegato(&th, allegato(aPNG, 5, "7120200A_2.png", "png", sha4), nil)

	v := valuta(t, th, motoreCatena(t), nil)
	s := gruppoDi(t, v, cSciolto)
	if len(tuttiDi(s)) != 1 || s.Primario.Provenienza != ancoraggio.OrigineConfermato || s.Primario.DocumentoID == nil || *s.Primario.DocumentoID != dPDF1 {
		t.Errorf("lo sciolto: %+v", tuttiDi(s))
	}
	p := gruppoDi(t, v, cProdotto)
	if len(tuttiDi(p)) != 1 || p.Primario.Sha256 != sha1 || *p.Primario.DocumentoID != dPDF2 {
		t.Errorf("il prodotto: %+v", tuttiDi(p))
	}
	for _, x := range v.Disegni {
		if x.ComponenteID == archiviato {
			t.Error("un componente archiviato ha un gruppo")
		}
	}
	if len(v.Disegni) != 2 || v.Disegni[0].ComponenteID.String() > v.Disegni[1].ComponenteID.String() {
		t.Errorf("i gruppi %d, in ordine di componente", len(v.Disegni))
	}
}

// TestLAssegnaPiuForteDelCandidato (R84; la voce più forte per contenuto): un file con «assegna» sullo sciolto, che è
// anche candidato sul suo nodo, è una voce sola, manuale.
func TestLAssegnaPiuForteDelCandidato(t *testing.T) {
	th := scenaAlbero(t)
	f := fattiPDFCampi(t, sha1, [2]string{"codice", "7120200A2"})
	conFile2D(&th, allegato(aPDF1, 3, "7120200A_2.pdf", "pdf", sha1), &f, nil, cSciolto)
	g := gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto)
	if len(tuttiDi(g)) != 1 || g.Primario.Provenienza != ancoraggio.OrigineManuale {
		t.Errorf("gruppo %+v", tuttiDi(g))
	}
}

// TestLaVoceVivaPrimaDelDocumentoNonCorrente (T-B5-33 dopo la revisione della fase 2; contratto §1.6, «Da verificare»;
// R62 b A, R84): per lo stesso contenuto, «assegna» e il candidato vengono prima di un documento non corrente, che per la
// voce non conta: un'associazione aperta non sparisce dietro un documento sostituito.
func TestLaVoceVivaPrimaDelDocumentoNonCorrente(t *testing.T) {
	m := motoreCatena(t)
	t.Run("«assegna» prima del documento sostituito", func(t *testing.T) {
		// Il documento sullo sciolto, sostituito da un documento dello stesso file sul prodotto, e «assegna» aperto dello
		// stesso file di nuovo sullo sciolto.
		th := scenaAlbero(t)
		f := fattiPDFCampi(t, sha1, [2]string{"codice", "7120200A2"})
		vecchio := documento2D(dPDF1, cSciolto, aPDF1, "7120200A_2.pdf", "pdf", sha1)
		s := dPDF2
		vecchio.SostituitoDa = &s
		conFile2D(&th, allegato(aPDF1, 3, "7120200A_2.pdf", "pdf", sha1), &f, &vecchio, uuid.Nil)
		th.Documenti = append(th.Documenti, documento2D(dPDF2, cProdotto, aPDF1, "7120200A_2.pdf", "pdf", sha1))
		cs := cSciolto
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: uid(0x7f9), AllegatoID: aPDF1, Tipo: "disegno_2d", ComponenteID: &cs, Fonte: "operatore", Stato: "aperta"})
		g := gruppoDi(t, valuta(t, th, m, nil), cSciolto)
		if len(tuttiDi(g)) != 1 || g.Primario.Provenienza != ancoraggio.OrigineManuale || g.Primario.DocumentoID != nil || g.Primario.Corrente {
			t.Errorf("la voce dello sciolto %+v: attesa «assegna», non il documento sostituito", tuttiDi(g))
		}
	})
	t.Run("il candidato prima del documento sostituito", func(t *testing.T) {
		// Il documento vecchio sullo sciolto, sostituito da un altro file: il file vecchio è ancora candidato sul nodo.
		th := scenaAlbero(t)
		f1 := fattiPDFCampi(t, sha1, [2]string{"codice", "7120200A2"})
		vecchio := documento2D(dPDF1, cSciolto, aPDF1, "7120200A_2.pdf", "pdf", sha1)
		s := dPDF2
		vecchio.SostituitoDa = &s
		conFile2D(&th, allegato(aPDF1, 3, "7120200A_2.pdf", "pdf", sha1), &f1, &vecchio, uuid.Nil)
		f2 := fattiPDFCampi(t, sha2, [2]string{"codice", "7120200A2"})
		nuovo := documento2D(dPDF2, cSciolto, aPDF2, "7120200A_2-bis.pdf", "pdf", sha2)
		conFile2D(&th, allegato(aPDF2, 4, "7120200A_2-bis.pdf", "pdf", sha2), &f2, &nuovo, uuid.Nil)
		g := gruppoDi(t, valuta(t, th, m, nil), cSciolto)
		if d := disegnoDi(t, g, sha1); d.Provenienza != ancoraggio.OrigineProposto || d.DocumentoID != nil {
			t.Errorf("la voce del file vecchio: provenienza %s, documento %v: atteso il candidato", d.Provenienza, d.DocumentoID)
		}
		if d := disegnoDi(t, g, sha2); d.Provenienza != ancoraggio.OrigineConfermato || !d.Corrente {
			t.Errorf("il documento corrente: %+v", d)
		}
	})
}

// TestIDisegniSonoDeterministici (determinismo): gli elenchi della fotografia in un altro ordine danno gli stessi 2D, e
// la fotografia di chi chiama non cambia.
func TestIDisegniSonoDeterministici(t *testing.T) {
	m := motoreCatena(t)
	th := scenaDueDisegni(t, "2", "7120200A1", "7120200A_2.tif")
	conFile2D(&th, allegato(aPNG, 5, "7120200A_2.png", "png", sha4), nil, nil, uuid.Nil)
	th.Documenti = append(th.Documenti, documento2D(dPDF2, cSciolto, uuid.Nil, "disegno-vecchio.pdf", "pdf", sha2))
	prima := canonicoDi(t, th)
	v1 := canonicoDi(t, valuta(t, th, m, nil).Disegni)
	if canonicoDi(t, th) != prima {
		t.Error("la fotografia è cambiata")
	}
	rovescia := func(n int, swap func(i, j int)) {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			swap(i, j)
		}
	}
	rovescia(len(th.Documenti), func(i, j int) { th.Documenti[i], th.Documenti[j] = th.Documenti[j], th.Documenti[i] })
	rovescia(len(th.Allegati), func(i, j int) { th.Allegati[i], th.Allegati[j] = th.Allegati[j], th.Allegati[i] })
	rovescia(len(th.Proposte), func(i, j int) { th.Proposte[i], th.Proposte[j] = th.Proposte[j], th.Proposte[i] })
	rovescia(len(th.Componenti), func(i, j int) { th.Componenti[i], th.Componenti[j] = th.Componenti[j], th.Componenti[i] })
	if v2 := canonicoDi(t, valuta(t, th, m, nil).Disegni); v2 != v1 {
		t.Errorf("con gli elenchi in un altro ordine i 2D cambiano:\n%s\n%s", v1, v2)
	}
}

// ---- i valori e i campi del contratto ----

// TestIValoriDeiDisegni: i valori e i campi del contratto (§2.3, §2.5, §2.6). Si riscrive con «Riscritta per …».
func TestIValoriDeiDisegni(t *testing.T) {
	for _, c := range [][2]string{
		{string(valutazione.Formato2DPDF), "pdf"}, {string(valutazione.Formato2DTIFF), "tiff"}, {string(valutazione.Formato2DPNG), "png"},
		{string(valutazione.ValiditaValido), "valido"}, {string(valutazione.ValiditaDaVerificare), "da_verificare"},
		{string(valutazione.ValiditaFormatoNonConfigurato), "formato_non_configurato"},
		{string(valutazione.MotivoFabbisognoContenutoNonAnalizzato), "contenuto_non_analizzato"},
		{string(valutazione.MotivoFabbisognoContenutoNonAperto), "contenuto_non_aperto"},
		{string(valutazione.MotivoFabbisognoContenutoNonLetto), "contenuto_non_letto"},
		{string(valutazione.MotivoFabbisognoContenutoNonVerificabile), "contenuto_non_verificabile"},
		{string(valutazione.MotivoFabbisognoFormatoNonConfigurato), "formato_non_configurato"},
		{string(valutazione.MotivoFabbisognoAssociazioneNonConfermata), "associazione_non_confermata"},
		{string(valutazione.MotivoFabbisognoNessunDocumento), "nessun_documento"},
		{string(valutazione.MotivoFabbisognoDerogatoNonSostituisce2D), "derogato_non_sostituisce_2d"},
		{string(valutazione.MotivoFabbisognoSulPortale), "sul_portale"},
		{valutazione.CartiglioLetto, "letto"}, {valutazione.CartiglioNonLeggibile, "non_leggibile"}, {valutazione.CartiglioFormatoSenzaLettura, "formato_senza_lettura"},
		{valutazione.FonteIdentitaConfermata, "confermata"}, {valutazione.FonteIdentitaCartiglio, "cartiglio"}, {valutazione.FonteIdentitaNomeFile, "nome_file"},
		{valutazione.FonteIdentitaDocumento, "documento"},
		{valutazione.ConfrontatoConDeciso, "deciso"}, {valutazione.ConfrontatoConProposto, "proposto"}, {valutazione.ConfrontatoConNessuno, "nessuno"},
		{valutazione.RegolaPrimarioCorrente, "corrente"}, {valutazione.RegolaPrimarioCompatibilita, "compatibilita"}, {valutazione.RegolaPrimarioFormato, "formato"},
		{valutazione.RegolaPrimarioRecente, "recente"}, {valutazione.RegolaPrimarioSha256, "sha256"},
		{valutazione.RelazioneRappresentazioneAlternativa, "rappresentazione_alternativa"}, {valutazione.RelazioneRevisioneDiversa, "revisione_diversa"},
		{valutazione.RelazioneElaboratoAggiuntivo, "elaborato_aggiuntivo"}, {valutazione.RelazioneNonDeterminata, "non_determinata"},
		{valutazione.StatoRelazioneDaConfermare, "da_confermare"},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, atteso %q", c[0], c[1])
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{valutazione.ContenutoDisegno{}, "Formato:formato Decodificato:decodificato Testo:testo Motivo:motivo"},
		{valutazione.RiferimentoAnteprima{}, "Sha256:sha256 Formato:formato DocumentoID:documento_id AllegatoID:allegato_id Disponibile:disponibile"},
		{valutazione.Disegno2D{}, "DocumentoID:documento_id AllegatoID:allegato_id Sha256:sha256 NomeFile:nome_file Formato:formato Estensione:estensione " +
			"Validita:validita MotivoValidita:motivo_validita Testo:testo Cartiglio:cartiglio Codice:codice Revisione:revisione FonteIdentita:fonte_identita " +
			"Provenienza:provenienza Corrente:corrente ConfermatoIl:confermato_il CompatibilitaCodice:compatibilita_codice " +
			"CompatibilitaRevisione:compatibilita_revisione ConfrontatoCon:confrontato_con Anteprima:anteprima Confermata:confermata " +
			"CodiceConfermato:codice_confermato EvidenzeRevisione:evidenze_revisione RevisioneProposta:revisione_proposta Registrata:registrata " +
			"RevProvenienza:rev_provenienza StatoRevisione:stato_revisione Discordanze:discordanze"},
		{valutazione.GruppoDisegni2D{}, "Primario:primario Alternativi:alternativi RegolaPrimario:regola_primario RelazioniDaConfermare:relazioni_da_confermare"},
		{valutazione.RelazioneDisegni{}, "A:a B:b Tipo:tipo Letture:letture Stato:stato"},
	} {
		if got := campiJSON(c.tipo); got != c.campi {
			t.Errorf("%T: campi %q, attesi %q", c.tipo, got, c.campi)
		}
	}
	if f, ok := reflect.TypeOf(valutazione.ValutazioneProdotti{}).FieldByName("Disegni"); !ok || f.Tag.Get("json") != "disegni,omitempty" {
		t.Error("ValutazioneProdotti.Disegni")
	}
	if campiJSON(valutazione.DisegniDelComponente{}) != "ComponenteID:componente_id Gruppo:gruppo" {
		t.Errorf("DisegniDelComponente: %s", campiJSON(valutazione.DisegniDelComponente{}))
	}
}
