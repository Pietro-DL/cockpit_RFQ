// L1 — l'impronta del prodotto (B6, V3; R90 [U], R100 A; contratto §1.7, §2.3; T-B0-29, T-B0-34, T-E1-19; fase 0 di B6,
// IM.2 e IM.3, F0-08; PO-26): la regola sui dati decisi sintetici (gli elenchi in ordine, ogni dato deciso la cambia, la
// versione), poi dalla fotografia: che cosa la copre e che cosa resta fuori, T-12, il prodotto dello scenario, e le due
// fotografie di PO-26 con la revisione di un solo componente, condiviso.
package valutazione_test

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_prodotti_test.go.

// datiSintetici: i dati decisi di un prodotto con la fonte, due componenti, una relazione e due documenti.
func datiSintetici() valutazione.DatiDecisiProdotto {
	rev := "1"
	return valutazione.DatiDecisiProdotto{Versione: valutazione.VersioneImprontaProdotto, Rif: rifProdB, Codice: "7120100A",
		Fonte:      &valutazione.ImprontaFonte{DocumentoID: dStepB, Sha256: shaStepB, Radice: "#1"},
		Componenti: []valutazione.ImprontaComponente{{ID: cProdotto, Codice: "7120100A"}, {ID: cSciolto, Codice: "7120200A", Rev: &rev}},
		Relazioni:  []valutazione.ImprontaRelazione{{Padre: cProdotto, Figlio: cSciolto, Qta: 2}},
		Documenti: []valutazione.ImprontaDocumento{{ComponenteID: cProdotto, ID: dStepB, Tipo: "cad_3d", Sha256: shaStepB},
			{ComponenteID: cSciolto, ID: dPDF2, Tipo: "disegno_2d", Sha256: sha2, Rev: &rev}}}
}

// impronta: l'impronta del prodotto dei dati dati; un errore ferma la prova.
func impronta(t *testing.T, d valutazione.DatiDecisiProdotto) string {
	t.Helper()
	h, err := valutazione.ImprontaProdotto(d)
	if err != nil || len(h) != 64 {
		t.Fatalf("ImprontaProdotto: %q %v", h, err)
	}
	return h
}

// TestImprontaProdottoSullaRegola (R90, T-B0-29; IM.3): lo sha256 del canonico dei dati decisi, con gli elenchi in ordine
// (gli stessi dati in un altro ordine danno la stessa impronta, e l'ingresso non cambia); ogni dato deciso la cambia, e
// così la versione; il canonico ha i nomi snake_case del contratto.
func TestImprontaProdottoSullaRegola(t *testing.T) {
	d := datiSintetici()
	prima := canonicoDi(t, d)
	h := impronta(t, d)
	if canonicoDi(t, d) != prima {
		t.Fatal("ImprontaProdotto ha cambiato l'ingresso")
	}
	p := datiSintetici()
	p.Componenti = rovescia(p.Componenti)
	p.Documenti = rovescia(p.Documenti)
	p.Relazioni = append(p.Relazioni, valutazione.ImprontaRelazione{Padre: cSciolto, Figlio: cTerzo, Qta: 1}, valutazione.ImprontaRelazione{Padre: cProdotto,
		Figlio: cTerzo, Qta: 3}, valutazione.ImprontaRelazione{Padre: cProdotto, Figlio: cSciolto, Qta: 1})
	q := datiSintetici()
	q.Relazioni = append([]valutazione.ImprontaRelazione{{Padre: cProdotto, Figlio: cSciolto, Qta: 1}, {Padre: cProdotto, Figlio: cTerzo, Qta: 3}},
		append(q.Relazioni, valutazione.ImprontaRelazione{Padre: cSciolto, Figlio: cTerzo, Qta: 1})...)
	if impronta(t, p) != impronta(t, q) {
		t.Error("gli stessi dati in un altro ordine danno un'altra impronta")
	}
	stessoFiglio := datiSintetici()
	stessoFiglio.Relazioni = []valutazione.ImprontaRelazione{{Padre: cSciolto, Figlio: cTerzo, Qta: 1}, {Padre: cProdotto, Figlio: cTerzo, Qta: 1}}
	rovesciate := datiSintetici()
	rovesciate.Relazioni = rovescia(stessoFiglio.Relazioni)
	if impronta(t, stessoFiglio) != impronta(t, rovesciate) {
		t.Error("due relazioni con lo stesso figlio e la stessa quantità in un altro ordine danno un'altra impronta")
	}
	docStessoComponente := datiSintetici()
	docStessoComponente.Documenti = append(docStessoComponente.Documenti, valutazione.ImprontaDocumento{ComponenteID: cSciolto, ID: dDisegno, Tipo: "cad_3d",
		Sha256: sha("9")}, valutazione.ImprontaDocumento{ComponenteID: cSciolto, ID: dPDF1, Tipo: "disegno_2d", Sha256: sha("8")})
	rovesciato := docStessoComponente
	rovesciato.Documenti = rovescia(docStessoComponente.Documenti)
	if impronta(t, docStessoComponente) != impronta(t, rovesciato) {
		t.Error("i documenti dello stesso componente in un altro ordine danno un'altra impronta")
	}
	due := "2"
	cambia := map[string]func(*valutazione.DatiDecisiProdotto){
		"la versione":                   func(d *valutazione.DatiDecisiProdotto) { d.Versione = 2 },
		"il Rif":                        func(d *valutazione.DatiDecisiProdotto) { d.Rif = rifP2 },
		"il codice":                     func(d *valutazione.DatiDecisiProdotto) { d.Codice = "7120100B" },
		"la fonte superata":             func(d *valutazione.DatiDecisiProdotto) { d.Fonte.Superato = true },
		"la radice della fonte":         func(d *valutazione.DatiDecisiProdotto) { d.Fonte.Radice = "#2" },
		"lo sha256 della fonte":         func(d *valutazione.DatiDecisiProdotto) { d.Fonte.Sha256 = sha("7") },
		"il documento della fonte":      func(d *valutazione.DatiDecisiProdotto) { d.Fonte.DocumentoID = dStepNuovo },
		"senza la fonte":                func(d *valutazione.DatiDecisiProdotto) { d.Fonte = nil },
		"la revisione di un componente": func(d *valutazione.DatiDecisiProdotto) { d.Componenti[1].Rev = &due },
		"il codice di un componente":    func(d *valutazione.DatiDecisiProdotto) { d.Componenti[1].Codice = "7120200B" },
		"un componente in più": func(d *valutazione.DatiDecisiProdotto) {
			d.Componenti = append(d.Componenti, valutazione.ImprontaComponente{ID: cTerzo})
		},
		"la quantità di una relazione": func(d *valutazione.DatiDecisiProdotto) { d.Relazioni[0].Qta = 3 },
		"lo sha256 di un documento":    func(d *valutazione.DatiDecisiProdotto) { d.Documenti[1].Sha256 = sha("6") },
		"la revisione di un documento": func(d *valutazione.DatiDecisiProdotto) { d.Documenti[1].Rev = &due },
		"il tipo di un documento":      func(d *valutazione.DatiDecisiProdotto) { d.Documenti[1].Tipo = "altro" },
		"la decisione su un documento": func(d *valutazione.DatiDecisiProdotto) { d.Documenti[1].Confermata = &due },
		"il codice confermato":         func(d *valutazione.DatiDecisiProdotto) { d.Documenti[1].CodiceConfermato = testo("7120200A2") },
	}
	for nome, f := range cambia {
		x := datiSintetici()
		f(&x)
		if impronta(t, x) == h {
			t.Errorf("%s non cambia l'impronta (T-B0-29)", nome)
		}
	}
	if valutazione.VersioneImprontaProdotto != 1 {
		t.Errorf("VersioneImprontaProdotto = %d: si cambia con un commit che lo dichiara (IM.2)", valutazione.VersioneImprontaProdotto)
	}
	c := canonicoDi(t, datiSintetici())
	for _, chiave := range []string{`"versione":1`, `"documento_id":`, `"superato":false`, `"componente_id":`, `"qta":2`, `"sha256":`, `"radice":"#1"`} {
		if !strings.Contains(c, chiave) {
			t.Errorf("il canonico %s non ha %s", c, chiave)
		}
	}
}

// TestImprontaDelProdottoDallaFotografia (IM.3; T-B0-29, T-B0-34; F0-08): sul prodotto altrimenti verificato l'impronta è
// quella dei dati decisi scritti a mano (la fonte del gesto 3, il perimetro confermato con codici e revisioni registrate,
// la relazione con la quantità, i documenti confermati correnti); un candidato, una proposta, un «assegna», una deroga, un
// documento generale, un documento sostituito, il codice manuale di una riga aperta non la toccano; un documento
// confermato nuovo, la revisione registrata, la quantità sì.
func TestImprontaDelProdottoDallaFotografia(t *testing.T) {
	th := scenaSmistamento(t)
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	atteso := valutazione.DatiDecisiProdotto{Versione: valutazione.VersioneImprontaProdotto, Rif: rifProdB, Codice: "7120100A",
		Fonte:      &valutazione.ImprontaFonte{DocumentoID: dStepB, Sha256: shaStepB, Radice: "#1"},
		Componenti: []valutazione.ImprontaComponente{{ID: cProdotto, Codice: "7120100A"}, {ID: cSciolto, Codice: "7120200A"}},
		Relazioni:  []valutazione.ImprontaRelazione{{Padre: cProdotto, Figlio: cSciolto, Qta: 2}},
		Documenti: []valutazione.ImprontaDocumento{{ComponenteID: cProdotto, ID: dStepB, Tipo: "cad_3d", Sha256: shaStepB},
			{ComponenteID: cProdotto, ID: dPDF1, Tipo: "disegno_2d", Sha256: sha1}, {ComponenteID: cSciolto, ID: dPDF2, Tipo: "disegno_2d", Sha256: sha2}}}
	if p.Impronta != impronta(t, atteso) {
		t.Fatalf("l'impronta del prodotto non è quella dei suoi dati decisi")
	}

	op := operatore
	fuori := map[string]func(*fotorfq.Thread){
		"un file candidato nuovo": func(th *fotorfq.Thread) {
			fileNelMessaggio(th, uid(0xf71), idM1, "7120200A_1.pdf", "pdf", shaN(350), scansione(t, shaN(350)))
		},
		"una proposta con l'«assegna»": func(th *fotorfq.Thread) {
			fileNelMessaggio(th, uid(0xf72), idM1, "allegato.pdf", "pdf", shaN(351), scansione(t, shaN(351)))
			c := cSciolto
			conProposta(th, uid(0xf73), uid(0xf72), "disegno_2d", "nome_file", "aperta", &c, nil)
		},
		"una deroga": func(th *fotorfq.Thread) {
			th.Deroghe = append(th.Deroghe, fotorfq.DerogaFabbisogno{ID: uid(0xf74), ComponenteID: cSciolto, Tipo: "disegno_2d", Motivo: "prova", UtenteID: op})
		},
		"un documento generale": func(th *fotorfq.Thread) {
			th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: uid(0xf75), ThreadID: threadACME, Tipo: "capitolato", NomeFile: "cap.pdf",
				Estensione: "pdf", Sha256: shaN(352), ConfermatoDa: op, ConfermatoIl: dataACME})
		},
		"un documento sostituito": func(th *fotorfq.Thread) {
			nuovo := dPDF2
			c := cSciolto
			th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: uid(0xf76), ThreadID: threadACME, ComponenteID: &c, Tipo: "disegno_2d",
				NomeFile: "vecchio.pdf", Estensione: "pdf", Sha256: shaN(353), ConfermatoDa: op, ConfermatoIl: dataACME, SostituitoDa: &nuovo})
		},
		"il codice manuale di una riga aperta (F0-08)": func(th *fotorfq.Thread) {
			r := rigaDi(t, th, "#1")
			r.OrigineCodice, r.Codice = testo("operatore"), testo("7120100B")
		},
		"un componente fuori dal perimetro, con il suo documento": func(th *fotorfq.Thread) {
			th.Componenti = append(th.Componenti, componente(cTerzo, "7120300A", "sciolto", "step"))
			c := cTerzo
			th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: uid(0xf78), ThreadID: threadACME, ComponenteID: &c, Tipo: "disegno_2d",
				NomeFile: "terzo.pdf", Estensione: "pdf", Sha256: shaN(355), ConfermatoDa: op, ConfermatoIl: dataACME})
		},
		"una relazione di un componente archiviato": func(th *fotorfq.Thread) {
			th.Componenti = append(th.Componenti, componente(cTerzo, "7120300A", "sciolto", "step"))
			ieri := dataACME
			th.Componenti[len(th.Componenti)-1].ArchiviatoIl = &ieri
			th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cProdotto, FiglioID: cTerzo, Qta: 1, Origine: "step", ConfermatoDa: op})
		},
	}
	for nome, f := range fuori {
		x := scenaSmistamento(t)
		f(&x)
		if q := prodottoDi(t, esitoSmistamento(t, x), rifProdB); q.Impronta != p.Impronta {
			t.Errorf("%s cambia l'impronta (T-B0-29: copre solo dati decisi)", nome)
		}
	}
	dentro := map[string]func(*fotorfq.Thread){
		"la revisione registrata dello sciolto (T-B0-34)": func(th *fotorfq.Thread) {
			for i := range th.Componenti {
				if th.Componenti[i].ID == cSciolto {
					th.Componenti[i].Rev = testo("1")
				}
			}
		},
		"la quantità della relazione": func(th *fotorfq.Thread) { th.Relazioni[0].Qta = 3 },
		"la revisione registrata di un documento": func(th *fotorfq.Thread) {
			for i := range th.Documenti {
				if th.Documenti[i].ID == dPDF2 {
					th.Documenti[i].Rev = testo("1")
				}
			}
		},
		"un documento confermato nuovo": func(th *fotorfq.Thread) {
			c := cSciolto
			th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: uid(0xf77), ThreadID: threadACME, ComponenteID: &c, Tipo: "sviluppo_dxf",
				NomeFile: "sviluppo.dxf", Estensione: "dxf", Sha256: shaN(354), ConfermatoDa: op, ConfermatoIl: dataACME})
		},
		"la fonte superata": func(th *fotorfq.Thread) {
			conFonteSuperata(t, th, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}}, true)
		},
	}
	for nome, f := range dentro {
		x := scenaSmistamento(t)
		f(&x)
		if q := prodottoDi(t, esitoSmistamento(t, x), rifProdB); q.Impronta == p.Impronta {
			t.Errorf("%s non cambia l'impronta", nome)
		}
	}

	// La fonte superata, scritta a mano: il riferimento con Superato, il documento di prima fuori (sostituito), il nuovo dentro.
	x := scenaSmistamento(t)
	conFonteSuperata(t, &x, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}}, true)
	sup := atteso
	sup.Fonte = &valutazione.ImprontaFonte{DocumentoID: dStepB, Sha256: shaStepB, Radice: "#1", Superato: true}
	sup.Documenti = []valutazione.ImprontaDocumento{{ComponenteID: cProdotto, ID: dStepNuovo, Tipo: "cad_3d", Sha256: shaN(310)}, atteso.Documenti[1], atteso.Documenti[2]}
	if q := prodottoDi(t, esitoSmistamento(t, x), rifProdB); q.Impronta != impronta(t, sup) {
		t.Error("con la fonte superata l'impronta non è quella dei suoi dati decisi")
	}
}

// TestImprontaConT12EPerLoScenario (IM.3, T-12; R-82 della revisione di V3): con una sezione dell'impronta assente
// l'impronta è vuota, per ogni prodotto (nessun campo nuovo); lo stesso con una sezione della fonte assente, perché
// l'impronta comprende il riferimento della fonte, che senza quella sezione non si calcola (RV3-07: prima si calcolava
// senza la fonte, un valore diverso da quello dei dati decisi). Con le altre sezioni assenti si calcola, e senza
// grammatica (T-B1-11) è quella dei dati decisi. Il prodotto dello scenario e quello senza componente hanno solo Rif e
// codice.
func TestImprontaConT12EPerLoScenario(t *testing.T) {
	th := scenaSmistamento(t)
	vistaCome(&th)
	piena := prodottoDi(t, esitoThread(t, calcola(t, fotografia(th), insiemeACME(t), valutazione.Ingressi{}), threadACME), rifProdB)
	if piena.Fonte.Riferimento == nil || len(piena.Impronta) != 64 {
		t.Fatalf("la scena: fonte %+v, impronta %q", piena.Fonte, piena.Impronta)
	}
	for _, k := range []string{fotorfq.SezioneComponenti, fotorfq.SezioneRelazioni, fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze, fotorfq.SezioneStepProdotto,
		fotorfq.SezioneAllegati, fotorfq.SezioneRigheComponenteProposta, fotorfq.SezioneRigheRelazioneProposta, fotorfq.SezioneLavoroPendente,
		fotorfq.SezioneProposteDocumento} {
		f := fotografia(th)
		f.Sezioni[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		if p := prodottoDi(t, esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME), rifProdB); p.Impronta != "" {
			t.Errorf("senza %s: impronta %q (la fonte calcolata %v, con il riferimento %v)", k, p.Impronta, p.Fonte.Calcolata, p.Fonte.Riferimento != nil)
		}
	}
	f := fotografia(th)
	f.Sezioni[fotorfq.SezioneTriage] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
	if p := prodottoDi(t, esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME), rifProdB); p.Impronta != piena.Impronta {
		t.Errorf("senza il triage l'impronta si calcola, uguale: %q", p.Impronta)
	}
	if p := prodottoDi(t, esitoThread(t, calcola(t, fotografia(th), nil, valutazione.Ingressi{}), threadACME), rifProdB); p.Impronta != piena.Impronta ||
		p.Fonte.Riferimento == nil {
		t.Errorf("senza grammatica l'impronta è quella dei dati decisi: %q, fonte %+v", p.Impronta, p.Fonte)
	}

	s := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A2.\r\nGrazie")
	s.Identificativi = []fotorfq.Identificativo{confermato("7120900A")}
	tid := threadACME
	caso := valutazione.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: "scenario",
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:corrente", Uso: "pertinente", Origine: "scenario"}}}
	et := esitoSmistamento(t, s, caso)
	for _, pv := range et.ProdottiValutati {
		if pv.Impronta != impronta(t, valutazione.DatiDecisiProdotto{Versione: valutazione.VersioneImprontaProdotto, Rif: pv.Rif, Codice: pv.CodiceRichiesto}) {
			t.Errorf("%s: l'impronta non è quella del solo Rif e codice", pv.Rif)
		}
	}
	if len(et.ProdottiValutati) != 2 {
		t.Errorf("prodotti %d: il confermato senza componente e lo scenario", len(et.ProdottiValutati))
	}
}

// TestPO26LaRevisioneDiUnSoloComponente (PO-26; T-E1-19, T-B0-29): due fotografie; nella seconda cambia la revisione
// registrata dello sciolto, che sta nella BOM confermata del primo prodotto e del secondo. Cambiano l'identità dello
// sciolto (il codice deciso del suo nodo) e l'impronta dei due prodotti che lo contengono; non cambiano l'impronta del
// terzo prodotto, che non lo contiene, né il codice deciso della radice. Il nodo dello sciolto dice AncheIn.
func TestPO26LaRevisioneDiUnSoloComponente(t *testing.T) {
	prima := scenaSmistamento(t)
	secondoProdotto(t, &prima, "7120200A")
	prima.Relazioni = append(prima.Relazioni, fotorfq.Relazione{PadreID: cP2, FiglioID: cSciolto, Qta: 1, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
	prima.Identificativi = append(prima.Identificativi, confermato("7120888A"))
	dopo := prima
	dopo.Componenti = append([]fotorfq.Componente(nil), prima.Componenti...)
	for i := range dopo.Componenti {
		if dopo.Componenti[i].ID == cSciolto {
			dopo.Componenti[i].Rev = testo("2")
		}
	}
	a, b := esitoSmistamento(t, prima), esitoSmistamento(t, dopo)
	for _, c := range []struct {
		rif    string
		cambia bool
	}{{rifProdB, true}, {rifP2, true}, {"identificativo:7120888A", false}} {
		if x, y := prodottoDi(t, a, c.rif).Impronta, prodottoDi(t, b, c.rif).Impronta; (x != y) != c.cambia {
			t.Errorf("%s: l'impronta cambia %v, attesa %v", c.rif, x != y, c.cambia)
		}
	}
	pa, pb := prodottoDi(t, a, rifProdB), prodottoDi(t, b, rifProdB)
	sa, sb := nodoBOMDi(t, pa, nodoB("#2")), nodoBOMDi(t, pb, nodoB("#2"))
	if sa.Nodo.Codice.Confermato == nil || sa.Nodo.Codice.Confermato.Rev != nil || sb.Nodo.Codice.Confermato == nil || sb.Nodo.Codice.Confermato.Rev == nil ||
		*sb.Nodo.Codice.Confermato.Rev != "2" {
		t.Errorf("l'identità dello sciolto: prima %+v, dopo %+v", sa.Nodo.Codice.Confermato, sb.Nodo.Codice.Confermato)
	}
	ra, rb := nodoBOMDi(t, pa, nodoB("#1")), nodoBOMDi(t, pb, nodoB("#1"))
	if canonicoDi(t, ra.Nodo.Codice.Confermato) != canonicoDi(t, rb.Nodo.Codice.Confermato) {
		t.Errorf("la revisione della radice cambia: %+v → %+v", ra.Nodo.Codice.Confermato, rb.Nodo.Codice.Confermato)
	}
	if len(sb.AncheIn) != 1 || sb.AncheIn[0] != rifP2 {
		t.Errorf("AncheIn dello sciolto %v", sb.AncheIn)
	}
}
