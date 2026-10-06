// L1 — i conflitti di nomenclatura (T-B0-24, R95 A; T-E1-15, T-E1-20, T-E1R-08; R97 B; PO-23 e PO-37 nelle parti di
// B5): dalle discordanze della riconciliazione di ancoraggio, con l'effetto conflitto, il pezzo per prodotto con l'asse
// e le evidenze dei due lati; la decisione resta il valore corrente; i doppioni dello stesso nodo in più strutture si
// tolgono, due 2D diversi restano due conflitti; l'indicatore di R97 B non blocca; la decisione tracciata contro
// un'evidenza nuova blocca, contro una vista no.
package valutazione_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// TestPO23IlCartiglioControLaNomenclaturaConfermata (PO-23, parte B5; T-B0-24, R95 A, T-E1-15): l'albero confermato da
// «Conferma l'albero» e un PDF confermato sullo sciolto, con il cartiglio che porta un altro marcatore: la nomenclatura
// va in conflitto, con i due valori e le evidenze; la decisione resta; la gerarchia non si ferma; la BOM non è
// verificata.
func TestPO23IlCartiglioControLaNomenclaturaConfermata(t *testing.T) {
	m := motoreCatena(t)
	th := scenaAlbero(t)
	conDisegno(t, &th, cSciolto, "7120200B1", nil)
	v := valuta(t, th, m, nil)
	p := prodotto(t, v, rifProdB)
	rifSciolto := "componente:" + cSciolto.String()
	if asse(p.BOM.Nomenclatura) != "conflitto/conflitto" || !reflect.DeepEqual(p.BOM.Nomenclatura.Conflitti, []string{rifSciolto}) ||
		asse(p.BOM.Gerarchia) != "verificata/" || p.BOM.Verificata {
		t.Fatalf("BOM %+v", p.BOM)
	}
	if len(v.Conflitti) != 1 {
		t.Fatalf("conflitti %+v", v.Conflitti)
	}
	c := v.Conflitti[0]
	if c.Tipo != valutazione.ConflittoCodice || c.Asse != valutazione.AsseNomenclatura || c.Rif != rifSciolto || c.Prodotto != rifProdB ||
		c.Decisione != "7120200A" || c.OrigineDecisione != ancoraggio.OrigineConfermato || c.Proposta != "7120200B1" ||
		c.Motivo != ancoraggio.ParteDiscordanzaCodice || c.Documentale != ancoraggio.RiconciliazioneCorrezioneProposta {
		t.Errorf("conflitto %+v", c)
	}
	e := c.EvidenzaProposta
	if e.AllegatoID == nil || *e.AllegatoID != aDisegno || e.DocumentoID == nil || *e.DocumentoID != dDisegno || e.UnitaID == "" ||
		e.Posizione.Tipo == "" || e.Evidenza != ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120200B1") ||
		!reflect.DeepEqual(e.Riferimenti, []string{nodoB("#2")}) {
		t.Errorf("evidenza della proposta %+v", e)
	}
	if d := c.EvidenzaDecisione; d.Origine != ancoraggio.OrigineConfermato || d.Da != nil || d.Il != nil {
		t.Errorf("evidenza della decisione %+v: il codice confermato del legacy non ha chi né quando (LD-13)", d)
	}
	n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
	if n.Codice.Confermato == nil || n.Codice.Confermato.Codice != "7120200A" || n.Decisione == nil || n.Decisione.ComponenteID != cSciolto {
		t.Errorf("nodo #2: la decisione resta il valore corrente (R95 A): %+v", n.Codice.Confermato)
	}
}

// TestPO37LaRevisioneRegistrataNonBlocca (PO-37, parte B5; R97 B, T-E1-22): componente.rev 1 e il cartiglio con la
// revisione 2: un indicatore, mai un conflitto; la nomenclatura resta verificata.
func TestPO37LaRevisioneRegistrataNonBlocca(t *testing.T) {
	th := scenaAlbero(t)
	th.Componenti[1].Rev = testo("1")
	conDisegno(t, &th, cSciolto, "7120200A2", nil)
	v := valuta(t, th, motoreCatena(t), nil)
	b := prodotto(t, v, rifProdB).BOM
	if !b.Verificata || len(v.Conflitti) != 0 {
		t.Fatalf("BOM %+v, conflitti %+v", b, v.Conflitti)
	}
	n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
	indicatori := 0
	for _, cd := range n.Codice.Documentale {
		for _, d := range cd.Discordanze {
			if d.Effetto == ancoraggio.EffettoDiscordanzaIndicatore && d.Motivo == ancoraggio.MotivoDiscordanzaRevisioneRegistrata {
				indicatori++
			}
		}
	}
	if indicatori != 1 {
		t.Errorf("indicatori %d: la discordanza resta visibile, come indicatore", indicatori)
	}
}

// TestIlCodiceManualeContraddetto (T-B0-24, T-B0-33; R95 A): il codice che l'operatore ha scritto su una riga aperta e
// il cartiglio di un 2D candidato sul nodo, con un altro marcatore: un conflitto sul nodo, con l'origine manuale.
func TestIlCodiceManualeContraddetto(t *testing.T) {
	th := scenaAlbero(t)
	aggiungiNodo(t, &th, "#3", "7120300A")
	r := rigaDi(t, &th, "#3")
	r.Codice, r.OrigineCodice = testo("7120300B"), testo("operatore")
	f := fattiPDF(t, shaDisegno, "7120300A1")
	conAllegato(&th, allegato(aDisegno, 2, "disegno-acme.pdf", "pdf", shaDisegno), &f)
	th.Proposte = []fotorfq.PropostaAttuale{{ID: uid(0x742), AllegatoID: aDisegno, Tipo: "disegno_2d", Fonte: "estensione", Stato: "aperta"}}
	v := valuta(t, th, motoreCatena(t), nil)
	b := prodotto(t, v, rifProdB).BOM
	if b.Nomenclatura.Stato != valutazione.StatoAsseConflitto || len(v.Conflitti) != 1 {
		t.Fatalf("nomenclatura %s, conflitti %+v", asse(b.Nomenclatura), v.Conflitti)
	}
	c := v.Conflitti[0]
	if c.Rif != nodoB("#3") || c.OrigineDecisione != ancoraggio.OrigineManuale || c.Decisione != "7120300B" || c.Proposta != "7120300A1" ||
		c.Motivo != ancoraggio.ParteDiscordanzaCodice {
		t.Errorf("conflitto %+v", c)
	}
}

// TestIDoppioniDelloStessoNodo (le NOTE per B5 di B4): lo stesso contenuto in due allegati, senza la fonte confermata,
// dà due strutture candidate con lo stesso nodo deciso; la stessa discordanza arriva due volte, e il conflitto è uno.
func TestIDoppioniDelloStessoNodo(t *testing.T) {
	th := scenaAlbero(t)
	senzaFonte(&th)
	secondo := uid(0x716)
	conAllegato(&th, allegato(secondo, 6, "copia-7120100A_1.stp", "stp", shaStepB), nil)
	op, c := operatore, cSciolto
	il := dataACME.Add(2 * time.Hour)
	th.RigheComponenteProposta = append(th.RigheComponenteProposta,
		fotorfq.RigaComponenteProposta{ID: uid(0x737), AllegatoID: secondo, Sha256: shaStepB, Chiave: "#1", IDGrezzo: "7120100A", Fonte: "step", Stato: "aperta"},
		fotorfq.RigaComponenteProposta{ID: uid(0x738), AllegatoID: secondo, Sha256: shaStepB, Chiave: "#2", IDGrezzo: "7120200A", Fonte: "step",
			Stato: "confermata", ComponenteID: &c, DecisoDa: &op, DecisoIl: &il})
	conDisegno(t, &th, cSciolto, "7120200B1", nil)
	v := valuta(t, th, motoreCatena(t), nil)
	discordanze := 0
	for _, s := range v.Ancoraggi.Strutture {
		for _, n := range s.Nodi {
			for _, cd := range n.Codice.Documentale {
				discordanze += len(cd.Discordanze)
			}
		}
	}
	if discordanze != 2 || len(v.Conflitti) != 1 {
		t.Errorf("discordanze %d nelle strutture, conflitti %d: attese due e uno", discordanze, len(v.Conflitti))
	}
}

// TestDueDisegniDiversiSulloStessoNodo (T-E1-15; T-B0-24): due 2D diversi confermati sullo sciolto, con lo
// stesso cartiglio che contraddice il codice confermato, sono due conflitti, ognuno con la sua evidenza: si tolgono solo
// i doppioni dello stesso 2D, mai due 2D. L'asse porta il Rif una volta sola.
func TestDueDisegniDiversiSulloStessoNodo(t *testing.T) {
	th := scenaAlbero(t)
	conDisegno(t, &th, cSciolto, "7120200B1", nil)
	secondo, dSecondo := uid(0x71f), uid(0x725)
	f := fattiPDF(t, sha("5"), "7120200B1")
	conAllegato(&th, allegato(secondo, 3, "disegno-2-acme.pdf", "pdf", sha("5")), &f)
	cs := cSciolto
	th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dSecondo, ThreadID: threadACME, ComponenteID: &cs, Tipo: "disegno_2d",
		NomeFile: "disegno-2-acme.pdf", Estensione: "pdf", Sha256: sha("5"), StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME,
		Allegati: []uuid.UUID{secondo}})
	v := valuta(t, th, motoreCatena(t), nil)
	rifSciolto := "componente:" + cSciolto.String()
	allegati := map[uuid.UUID]bool{}
	for _, c := range v.Conflitti {
		if c.Rif != rifSciolto || c.Asse != valutazione.AsseNomenclatura || c.Proposta != "7120200B1" || c.EvidenzaProposta.AllegatoID == nil {
			t.Errorf("conflitto %+v", c)
			continue
		}
		allegati[*c.EvidenzaProposta.AllegatoID] = true
	}
	if len(v.Conflitti) != 2 || !allegati[aDisegno] || !allegati[secondo] {
		t.Errorf("conflitti %+v: attesi due, uno per 2D", v.Conflitti)
	}
	if b := prodotto(t, v, rifProdB).BOM; asse(b.Nomenclatura) != "conflitto/conflitto" || !reflect.DeepEqual(b.Nomenclatura.Conflitti, []string{rifSciolto}) {
		t.Errorf("nomenclatura %+v", b.Nomenclatura)
	}
}

// ---- PO-37, la variante del modello nuovo: una DecisioneIdentita sul componente ----

// letturaDi: la lettura completa di tutto il codice su quel selettore, che deve esserci.
func letturaDi(t *testing.T, m *motorea.Motore, selettore, codice string) motorea.LetturaForma {
	t.Helper()
	sel, err := evidenze.LeggiSelettore(selettore)
	if err != nil {
		t.Fatal(err)
	}
	letture, _ := m.Riconosci(sel, codice)
	for _, l := range letture {
		if l.Intervallo.Inizio == 0 && l.Intervallo.Fine == len(codice) && l.Base.Completa {
			return l
		}
	}
	t.Fatalf("%q non si legge su %s", codice, selettore)
	return motorea.LetturaForma{}
}

// fileDi: un allegato con i suoi fatti, passato dall'adattatore e da Interpreta, come lo prepara valutazione.
func fileDi(t *testing.T, m *motorea.Motore, a fotorfq.Allegato, f fotorfq.Fatti) ancoraggio.FileInterpretato {
	t.Helper()
	d, err := estrazione.DaAllegato(a, &f, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Interpreta(d, evidenze.UsoSconosciuto(d.BundleID))
	if err != nil {
		t.Fatal(err)
	}
	return ancoraggio.FileInterpretato{AllegatoID: a.ID, Documento: d, Interpretazione: r, Disponibilita: ancoraggio.DisponibilitaDisponibile}
}

// strutturePO37: le strutture di ancoraggio della scena dell'albero (la BOM di lavoro sotto la radice scelta, lo
// sciolto deciso sul nodo #2), con una DecisioneIdentita sullo sciolto (codice 7120200A, revisione 1, le evidenze viste
// date) e il 2D confermato sullo sciolto con il cartiglio dato. È la forma degli ingressi che valutazione prepara dalla
// fotografia; la DecisioneIdentita non ha un adattatore in A1c (LD-27), quindi si passa qui ad ancoraggio.
func strutturePO37(t *testing.T, m *motorea.Motore, cartiglio string, viste ...ancoraggio.EvidenzaVista) []ancoraggio.StrutturaProdotto {
	t.Helper()
	step := fileDi(t, m, allegato(aStepB, 1, "7120100A_1.stp", "stp", shaStepB),
		fattiStepF(t, shaStepB, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}}))
	pdf := fileDi(t, m, allegato(aDisegno, 2, "disegno-acme.pdf", "pdf", shaDisegno), fattiPDF(t, shaDisegno, cartiglio))
	pdf.Disegno = true
	s, ok := ancoraggio.StrutturaDa(step)
	if !ok {
		t.Fatal("lo STEP non dà una struttura")
	}
	codici := map[string]motorea.CodiceComposto{}
	for _, n := range s.Nodi {
		for _, l := range n.Letture {
			codici[ancoraggio.ChiaveCodiceProposto(s.BundleID, l.ID)] = m.ComponiCodiceDocumentale(l.Forma)
		}
	}
	lp, ls := letturaDi(t, m, "nodo_step.id", "7120100A"), letturaDi(t, m, "nodo_step.id", "7120200A")
	cp, cs, op, dd, as := cProdotto, cSciolto, operatore, dDisegno, aStepB
	q := 2
	target := ancoraggio.ProdottoRichiesto{Rif: rifProdB, Autorita: ancoraggio.AutoritaConfermata, ClienteID: clienteACME, Namespace: lp.Namespace,
		CodiceRichiesto: "7120100A", Base: lp.Base, Marcatore: "A", ComponenteID: &cp,
		FonteConfermata: &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: dStepB, Sha256: shaStepB, AllegatoID: &as,
			Radice: "#1", Forma: ancoraggio.FormaRiferimentoSmistamento}}
	ctx := ancoraggio.ContestoStrutturale{
		Strutture: []ancoraggio.StrutturaFile{s},
		Confermato: []ancoraggio.ComponenteDeciso{
			{ComponenteID: cProdotto, Codice: "7120100A", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato, Lettura: &lp},
			{ComponenteID: cSciolto, Codice: "7120200A", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato, Lettura: &ls},
		},
		ArchiConfermati: []ancoraggio.ArcoPercorso{{Padre: ancoraggio.RifComponente(cProdotto), Figlio: ancoraggio.RifComponente(cSciolto), Quantita: &q,
			Origine: ancoraggio.OrigineArcoConfermato}},
		Proposto: []ancoraggio.RigaPropostaLegacy{{ID: rigaRadB, AllegatoID: aStepB, Sha256: shaStepB, Chiave: "#1", Autorita: ancoraggio.AutoritaProposta,
			Origine: ancoraggio.OrigineProposto}},
		Decise:         []ancoraggio.RigaDecisaLegacy{{ID: rigaB2, AllegatoID: aStepB, Sha256: shaStepB, Chiave: "#2", Stato: "confermata", ComponenteID: &cs, DecisoDa: &op}},
		CodiciProposti: codici,
		DecisioniIdentita: []ancoraggio.DecisioneIdentita{{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: cSciolto, Codice: "7120200A", Revisione: "1",
			EvidenzeViste: viste, Da: operatore, Il: dataACME.Add(3 * time.Hour)}},
		AssociazioniDecise: []ancoraggio.AssociazioneDecisa{{AllegatoID: aDisegno, DocumentoID: &dd, ComponenteID: cSciolto, Origine: ancoraggio.OrigineConfermato}},
	}
	e, err := ancoraggio.ProponiAncoraggi([]ancoraggio.FileInterpretato{step, pdf}, []ancoraggio.ProdottoRichiesto{target}, ctx)
	if err != nil {
		t.Fatalf("ProponiAncoraggi: %v", err)
	}
	return e.Strutture
}

// TestPO37LaDecisioneTracciata (PO-37, la variante del modello nuovo, parte B5; E1R §5.3; T-E1R-08, R95 A): una
// DecisioneIdentita sullo sciolto con la revisione 1. Un cartiglio con la revisione 2 che non era fra le evidenze viste
// è un conflitto di nomenclatura, con chi e quando della decisione; lo stesso cartiglio già visto è un indicatore e
// non blocca. Sulla regola, il primo blocca la nomenclatura, il secondo no.
func TestPO37LaDecisioneTracciata(t *testing.T) {
	m := motoreCatena(t)
	struttura := func(c []valutazione.Conflitto) valutazione.StrutturaDaVerificare {
		return valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true, Radici: []string{nodoB("#1")},
			Nodi: []string{nodoB("#1"), nodoB("#2")}, Archi: []string{valutazione.RifArco(nodoB("#1"), nodoB("#2"))},
			ArchiConfermati: []string{valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())}, Conflitti: c}
	}
	gesti := valutazione.GestiVerificaBOM{Legacy: true,
		Nomenclatura: gestoSintetico(nil, []string{valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())}),
		Gerarchia:    gestoSintetico(nil, []string{valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())})}

	t.Run("contro un'evidenza nuova: conflitto", func(t *testing.T) {
		c := valutazione.ConflittiDellaNomenclatura(rifProdB, strutturePO37(t, m, "7120200A2", ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120200A1")))
		if len(c) != 1 {
			t.Fatalf("conflitti %+v", c)
		}
		x := c[0]
		if x.Rif != "componente:"+cSciolto.String() || x.Decisione != "7120200A 1" || x.Proposta != "7120200A2" || x.Motivo != ancoraggio.ParteDiscordanzaRevisione ||
			x.EvidenzaDecisione.RevProvenienza != ancoraggio.RevProvenienzaDecisioneTracciata || x.EvidenzaDecisione.Da == nil ||
			*x.EvidenzaDecisione.Da != operatore || x.EvidenzaDecisione.Il == nil || !x.EvidenzaDecisione.Il.Equal(dataACME.Add(3*time.Hour)) {
			t.Errorf("conflitto %+v", x)
		}
		if v := valutazione.VerificaDellaBOM(gesti, struttura(c)); v.Nomenclatura.Stato != valutazione.StatoAsseConflitto || v.Verificata {
			t.Errorf("BOM %+v", v)
		}
	})
	t.Run("contro un'evidenza vista: nessun conflitto", func(t *testing.T) {
		s := strutturePO37(t, m, "7120200A2", ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120200A2"))
		c := valutazione.ConflittiDellaNomenclatura(rifProdB, s)
		if len(c) != 0 {
			t.Fatalf("conflitti %+v", c)
		}
		vista := false
		for _, st := range s {
			for _, n := range st.Nodi {
				for _, cd := range n.Codice.Documentale {
					for _, d := range cd.Discordanze {
						vista = vista || d.Motivo == ancoraggio.MotivoDiscordanzaEvidenzaVista
					}
				}
			}
		}
		if !vista {
			t.Error("la discordanza con l'evidenza vista resta, come indicatore (R104)")
		}
		if v := valutazione.VerificaDellaBOM(gesti, struttura(c)); !v.Verificata {
			t.Errorf("BOM %+v", v)
		}
	})
	t.Run("un altro prodotto non prende il conflitto", func(t *testing.T) {
		s := strutturePO37(t, m, "7120200A2", ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120200A1"))
		if c := valutazione.ConflittiDellaNomenclatura("componente:"+uuid.Nil.String(), s); len(c) != 0 {
			t.Errorf("conflitti %+v", c)
		}
	})
}
