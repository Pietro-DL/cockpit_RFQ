// L1 — il collegamento con ancoraggio (B5, fase 1; piano A, 6.4.6 passi 7-9; contratto §1.3, §1.4, §1.5, §2.2; le
// NOTE per B5 dei resoconti di B4): una scena ACME costruita in Go come la darebbe il caricatore, e ValutaProdotti, che
// porta le strutture, la BOM di lavoro, gli ancoraggi dei file e la catena del codice. I campi convertiti: il target
// con il componente, la fonte confermata e il marcatore; il codice manuale solo dalla riga aperta corretta
// dall'operatore (T-B0-33); le righe decise senza il loro codice né la revisione (T-E1-22, contratto §3); i codici
// composti (T-08) e quelli dei messaggi (T-E1-06); le associazioni decise, confermate e manuali (T-B4-33); la
// disponibilità (6.4.6 passo 8, T-B0-31) e il segno del 2D (T-B4-31). La fonte allineata alla regola del marcatore
// (T-B4-30, T-B2-02); un prodotto dello scenario con la sua lettura per intero e con l'origine «scenario» nell'impronta
// dei target (T-B5-06); il determinismo, anche con le rimozioni aperte permutate.
package valutazione_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli altri file della scena del collegamento.
var (
	aScansione = uid(0x713)
	aVuoto     = uid(0x714)
	aInAttesa  = uid(0x715)
	pScansione = uid(0x741)
)

// scenaCollegamento: la scena della BOM con quattro nodi (#1 7120100A, la radice; #2 7120200A, deciso come lo sciolto
// confermato, con la riga legacy che porta un altro codice e un'altra revisione; #3 7120300A, aperto e corretto
// dall'operatore in 7120300B rev 2; #4 7120400A, aperto con il codice del motore legacy), il 2D dello sciolto con il
// cartiglio 7120200A1, una scansione assegnata allo sciolto, un file senza contenuto, un file in analisi, e un messaggio
// che nomina 7120200A2.
func scenaCollegamento(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", "1"}, {"#2", "7120200A", "1"}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}},
		[]arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}, {"#1", "#4", 1}})
	th.Messaggi[0].CorpoTesto = testo("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A, con il particolare 7120200A2.\r\nGrazie")
	confermaComponente(&th, cSciolto, "7120200A", cProdotto, 2)
	th.Componenti[1].Rev = testo("1")
	op := operatore
	decidi(t, &th, "#2", "confermata", cSciolto, &op)
	r2 := rigaDi(t, &th, "#2")
	r2.Codice, r2.Rev, r2.OrigineCodice = testo("7129999A"), testo("9"), testo("famiglia")
	arco := arcoDi(t, &th, "#1", "#2")
	arco.Stato, arco.DecisoDa = "confermata", &op
	r3 := rigaDi(t, &th, "#3")
	r3.Codice, r3.Rev, r3.OrigineCodice = testo("7120300B"), testo("2"), testo("operatore")
	r4 := rigaDi(t, &th, "#4")
	r4.Codice, r4.Rev, r4.OrigineCodice = testo("7120499A"), testo("8"), testo("famiglia")

	conDisegno(t, &th, cSciolto, "7120200A1", testo("1"))
	scan := fattiPDF(t, sha("c"), "")
	conAllegato(&th, allegato(aScansione, 3, "scansione-acme.pdf", "pdf", sha("c")), &scan)
	cs := cSciolto
	th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pScansione, AllegatoID: aScansione, Tipo: "disegno_2d", ComponenteID: &cs,
		Fonte: "operatore", Stato: "aperta"})
	vuoto := allegato(aVuoto, 4, "vuoto-acme.pdf", "pdf", "")
	vuoto.Sha256 = nil
	conAllegato(&th, vuoto, nil)
	conAllegato(&th, allegato(aInAttesa, 5, "in-analisi-acme.pdf", "pdf", sha("d")), nil)
	th.InAttesa = []uuid.UUID{aInAttesa}
	return th
}

// TestIlCollegamentoConAncoraggio (6.4.6 passi 7-9; T-B0-33, T-E1-22, T-08, T-E1-06, T-B4-31, T-B4-33, T-B0-31, T-B4-38):
// ValutaProdotti porta le strutture con la BOM di lavoro sotto la radice scelta, la catena del codice di ogni nodo con
// le decisioni accanto, gli ancoraggi dei file con la disponibilità e la riconciliazione del 2D; ogni campo convertito
// dalla fotografia è quello del contratto, e niente del legacy che il contratto esclude.
func TestIlCollegamentoConAncoraggio(t *testing.T) {
	m := motoreCatena(t)
	th := scenaCollegamento(t)
	v := valuta(t, th, m, nil)
	pv := prodotto(t, v, rifProdB)
	if statoMotivo(pv.Fonte) != "confermata/estrazione_riuscita" || pv.Struttura != ancoraggio.StatoBOMDiLavoroProposta {
		t.Fatalf("fonte %s, struttura %s: attese la fonte confermata e la BOM di lavoro (R76 A)", statoMotivo(pv.Fonte), pv.Struttura)
	}
	bom := strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
	if bom.Radice != nodoB("#1") || bom.AllegatoID != aStepB || len(bom.Nodi) != 4 || bom.RigaRadice != ancoraggio.RifRigaProposta(rigaRadB) {
		t.Fatalf("BOM di lavoro: radice %s, allegato %s, %d nodi, riga della radice %q", bom.Radice, bom.AllegatoID, len(bom.Nodi), bom.RigaRadice)
	}

	t.Run("la radice scelta porta il codice confermato del prodotto (T-B4-38)", func(t *testing.T) {
		r := nodoIn(t, bom, nodoB("#1"))
		if r.Decisione != nil || r.Codice.Confermato == nil || r.Codice.Confermato.Codice != "7120100A" || r.Codice.Confermato.Origine != ancoraggio.OrigineConfermato {
			t.Errorf("radice: decisione %+v, confermato %+v", r.Decisione, r.Codice.Confermato)
		}
	})
	t.Run("la riga decisa lega il nodo al componente, senza il suo codice né la sua revisione (T-E1-22)", func(t *testing.T) {
		n := nodoIn(t, bom, nodoB("#2"))
		if n.Decisione == nil || n.Decisione.ComponenteID != cSciolto || n.RigaDecisa == nil || n.RigaDecisa.ID != rigaB2 || !n.DecisoDaPersona || n.RigaLegacy != nil {
			t.Fatalf("nodo #2: decisione %+v, riga decisa %+v, da una persona %v", n.Decisione, n.RigaDecisa, n.DecisoDaPersona)
		}
		k := n.Codice.Confermato
		if k == nil || k.Codice != "7120200A" || k.Rev == nil || *k.Rev != "1" || k.Base.Normalizzata != "7120200" ||
			k.RevProvenienza != ancoraggio.RevProvenienzaFormazioneSTEP {
			t.Errorf("codice confermato %+v: il codice e la revisione del componente, letti con la grammatica, con la formazione del nodo (T-E1-22)", k)
		}
		if n.Codice.Manuale != nil {
			t.Errorf("manuale %+v: una riga decisa non porta il codice manuale", n.Codice.Manuale)
		}
	})
	t.Run("il codice manuale solo dalla riga aperta corretta dall'operatore (T-B0-33)", func(t *testing.T) {
		n3, n4 := nodoIn(t, bom, nodoB("#3")), nodoIn(t, bom, nodoB("#4"))
		if n3.RigaLegacy == nil || n3.RigaLegacy.ID != rigaB3 || n3.RigaLegacy.CodiceManuale == nil || *n3.RigaLegacy.CodiceManuale != "7120300B" ||
			n3.RigaLegacy.RevManuale == nil || *n3.RigaLegacy.RevManuale != "2" || n3.RigaLegacy.LetturaManuale == nil ||
			n3.RigaLegacy.LetturaManuale.Base.Normalizzata != "7120300" {
			t.Errorf("nodo #3: riga %+v", n3.RigaLegacy)
		}
		if n3.Codice.Manuale == nil || n3.Codice.Manuale.Origine != ancoraggio.OrigineManuale || n3.Codice.Manuale.Codice != "7120300B" {
			t.Errorf("nodo #3: manuale %+v", n3.Codice.Manuale)
		}
		if n4.RigaLegacy == nil || n4.RigaLegacy.ID != rigaB4 || n4.RigaLegacy.CodiceManuale != nil || n4.RigaLegacy.RevManuale != nil || n4.Codice.Manuale != nil {
			t.Errorf("nodo #4: riga %+v, manuale %+v: il codice del motore legacy non entra", n4.RigaLegacy, n4.Codice.Manuale)
		}
		c := canonicoDi(t, v)
		for _, legacy := range []string{"7129999", "7120499"} {
			if strings.Contains(c, legacy) {
				t.Errorf("il codice %s del motore legacy compare nell'esito", legacy)
			}
		}
	})
	t.Run("i codici composti e quelli dei messaggi (T-08, T-E1-06)", func(t *testing.T) {
		n := nodoIn(t, bom, nodoB("#2"))
		if n.Codice.Proposto != "" || n.Codice.MotivoProposto != motorea.MotivoComposizioneRevisioneNonDeterminata || !n.Codice.Identita.Parziale {
			t.Errorf("nodo #2: proposto %q, motivo %q, parziale %v: il compositore dice che la forma del cartiglio chiede la revisione",
				n.Codice.Proposto, n.Codice.MotivoProposto, n.Codice.Identita.Parziale)
		}
		var dalMessaggio []string
		for _, c := range n.Codice.Identita.CandidatiRevisione {
			if c.Fonte == ancoraggio.FonteRevisioneMessaggio && c.MessaggioID != nil && *c.MessaggioID == idM1 {
				dalMessaggio = append(dalMessaggio, c.Valore+"@"+c.Entita)
			}
		}
		if len(dalMessaggio) != 1 || dalMessaggio[0] != "2@componente:"+cSciolto.String() {
			t.Errorf("candidati dal messaggio %v: la revisione 2 del messaggio per lo sciolto", dalMessaggio)
		}
	})
	t.Run("le associazioni decise e il 2D (T-B4-31, T-B4-33)", func(t *testing.T) {
		n := nodoIn(t, bom, nodoB("#2"))
		per := map[uuid.UUID]ancoraggio.CodiceDocumentale{}
		for _, cd := range n.Codice.Documentale {
			per[cd.AllegatoID] = cd
		}
		d, ok := per[aDisegno]
		if !ok || d.OrigineAssociazione != ancoraggio.OrigineConfermato || d.DocumentoID == nil || *d.DocumentoID != dDisegno ||
			d.Esito != ancoraggio.RiconciliazioneCompletamentoProposto || len(d.Discordanze) != 0 {
			t.Errorf("il 2D confermato: %+v", d)
		}
		s, ok := per[aScansione]
		if !ok || s.OrigineAssociazione != ancoraggio.OrigineManuale || s.DocumentoID != nil || s.Esito != ancoraggio.RiconciliazioneNonVerificabile ||
			s.Motivo != ancoraggio.MotivoDocumentaleCartiglioNonLetto {
			t.Errorf("la scansione assegnata: %+v", s)
		}
		if len(n.Codice.Documentale) != 2 {
			t.Errorf("codici documentali %d: solo i due 2D", len(n.Codice.Documentale))
		}
	})
	t.Run("un documento sostituito non è un'associazione decisa (dubbio T-B5-08)", func(t *testing.T) {
		th := scenaCollegamento(t)
		vecchio, dVecchio := uid(0x718), uid(0x723)
		f := fattiPDF(t, sha("7"), "7120200A1")
		conAllegato(&th, allegato(vecchio, 8, "vecchio-acme.pdf", "pdf", sha("7")), &f)
		cs, nuovo := cSciolto, dDisegno
		th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dVecchio, ThreadID: threadACME, ComponenteID: &cs, Tipo: "disegno_2d",
			NomeFile: "vecchio-acme.pdf", Estensione: "pdf", Sha256: sha("7"), StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME,
			SostituitoDa: &nuovo, Allegati: []uuid.UUID{vecchio}})
		v := valuta(t, th, m, nil)
		n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
		trovato := false
		for _, cd := range n.Codice.Documentale {
			if cd.AllegatoID == vecchio {
				trovato = true
				if cd.OrigineAssociazione != ancoraggio.OrigineProposto || cd.DocumentoID != nil {
					t.Errorf("il 2D sostituito: %+v (solo il candidato del motore A)", cd)
				}
			}
		}
		if !trovato {
			t.Error("il 2D sostituito, candidato sul nodo, ha il suo codice documentale")
		}
	})
	t.Run("la disponibilità di ogni file (6.4.6 passo 8, T-B0-31)", func(t *testing.T) {
		for _, c := range []struct {
			allegato uuid.UUID
			atteso   ancoraggio.Disponibilita
		}{
			{aStepB, ancoraggio.DisponibilitaDisponibile}, {aDisegno, ancoraggio.DisponibilitaDisponibile},
			{aScansione, ancoraggio.DisponibilitaSenzaTesto}, {aVuoto, ancoraggio.DisponibilitaMancante}, {aInAttesa, ancoraggio.DisponibilitaPendente},
		} {
			if a := ancoraggioDel(t, v, c.allegato); a.Disponibilita != c.atteso {
				t.Errorf("allegato %s: disponibilità %s, attesa %s", c.allegato, a.Disponibilita, c.atteso)
			}
		}
		if len(v.Ancoraggi.File) != 5 {
			t.Errorf("file %d: tutti gli allegati di natura file", len(v.Ancoraggi.File))
		}
	})
	t.Run("gli archi con la decisione accanto", func(t *testing.T) {
		for _, a := range bom.Archi {
			deciso := a.Decisione != nil
			if (a.Figlio == nodoB("#2")) != deciso {
				t.Errorf("arco %s→%s: decisione %+v (solo l'arco verso lo sciolto confermato)", a.Padre, a.Figlio, a.Decisione)
			}
		}
	})
}

// TestIlCollegamentoNeiCasiDiConfine (6.4.6 passi 3 e 8; T-B0-31, T-B4-31, T-B4-33, T-E1-06; dubbio T-B5-07; i casi che
// le mutazioni non vedevano): la disponibilità di un PDF che non si apre, di uno STEP che il worker non ha letto e di un
// file senza fatti; un contenitore e un allegato che non è un file non sono file, e un documento su di loro non è
// un'associazione decisa; il tipo del documento corrente prima di quello sostituito; una riga di un altro contenuto e
// una riga confermata senza componente restano fuori, senza fermare il thread; le righe d'arco decise non sono archi
// proposti; una proposta chiusa non è «assegna»; la revisione del codice del target confermato è un candidato del
// prodotto; la posizione del codice di un messaggio è quella esatta della lettura.
func TestIlCollegamentoNeiCasiDiConfine(t *testing.T) {
	m := motoreCatena(t)

	t.Run("la disponibilità degli altri file (6.4.6 passo 8)", func(t *testing.T) {
		th := scenaCollegamento(t)
		rotto, stepRotto, soloNome := uid(0x719), uid(0x71a), uid(0x71b)
		fr := fatti(t, sha("8"), `{"errore_pdf": "file danneggiato"}`, nil)
		conAllegato(&th, allegato(rotto, 9, "rotto-acme.pdf", "pdf", sha("8")), &fr)
		fs := fattiSTEPIllegibili(t, sha("9"))
		conAllegato(&th, allegato(stepRotto, 10, "rotto-acme.stp", "stp", sha("9")), &fs)
		conAllegato(&th, allegato(soloNome, 11, "solo-nome-acme.pdf", "pdf", sha("0")), nil)
		v := valuta(t, th, m, nil)
		for _, c := range []struct {
			allegato uuid.UUID
			atteso   ancoraggio.Disponibilita
		}{{rotto, ancoraggio.DisponibilitaIlleggibile}, {stepRotto, ancoraggio.DisponibilitaErrore}, {soloNome, ancoraggio.DisponibilitaParziale}} {
			if a := ancoraggioDel(t, v, c.allegato); a.Disponibilita != c.atteso {
				t.Errorf("allegato %s: disponibilità %s, attesa %s", c.allegato, a.Disponibilita, c.atteso)
			}
		}
	})
	t.Run("un contenitore e un allegato che non è un file non sono file", func(t *testing.T) {
		th := scenaCollegamento(t)
		zip, voce, inline, dZip := uid(0x71c), uid(0x71d), uid(0x71e), uid(0x724)
		conAllegato(&th, allegato(zip, 12, "pacco-acme.zip", "zip", sha("f")), nil)
		v := allegato(voce, 1, "voce-acme.pdf", "pdf", sha("1"))
		v.ContenitoreID = &zip
		conAllegato(&th, v, nil)
		in := allegato(inline, 13, "immagine-acme.png", "png", sha("2"))
		in.Natura = "inline"
		conAllegato(&th, in, nil)
		cs := cSciolto
		th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dZip, ThreadID: threadACME, ComponenteID: &cs, Tipo: "altro",
			NomeFile: "pacco-acme.zip", Estensione: "zip", Sha256: sha("f"), StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME,
			Allegati: []uuid.UUID{zip, inline}})
		e := valuta(t, th, m, nil)
		file := map[uuid.UUID]bool{}
		for _, a := range e.Ancoraggi.File {
			file[a.AllegatoID] = true
		}
		if file[zip] || file[inline] || !file[voce] {
			t.Errorf("file %v: il contenitore e l'allegato inline non sono file, la voce sì", file)
		}
	})
	t.Run("il tipo del documento corrente prima di quello sostituito (T-B4-31)", func(t *testing.T) {
		th := scenaCollegamento(t)
		cs, nuovo := cSciolto, dDisegno
		th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: uid(0x720), ThreadID: threadACME, ComponenteID: &cs, Tipo: "altro",
			NomeFile: "disegno-acme.pdf", Estensione: "pdf", Sha256: shaDisegno, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME,
			SostituitoDa: &nuovo, Allegati: []uuid.UUID{aDisegno}})
		v := valuta(t, th, m, nil)
		n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
		trovato := false
		for _, cd := range n.Codice.Documentale {
			trovato = trovato || (cd.AllegatoID == aDisegno && cd.OrigineAssociazione == ancoraggio.OrigineConfermato)
		}
		if !trovato {
			t.Errorf("codici documentali %+v: il 2D è del tipo del documento corrente", n.Codice.Documentale)
		}
	})
	t.Run("una riga di un altro contenuto e una riga confermata senza componente restano fuori (dubbio T-B5-07)", func(t *testing.T) {
		th := scenaCollegamento(t)
		th.RigheComponenteProposta = append(th.RigheComponenteProposta,
			fotorfq.RigaComponenteProposta{ID: uid(0x73c), AllegatoID: aStepB, Sha256: sha("9"), Chiave: "#9", IDGrezzo: "7120900A", Fonte: "step", Stato: "aperta"},
			fotorfq.RigaComponenteProposta{ID: uid(0x73d), AllegatoID: aStepB, Sha256: shaStepB, Chiave: "#8", IDGrezzo: "7120800A", Fonte: "step", Stato: "confermata"})
		bom := strutturaDelProdotto(t, valuta(t, th, m, nil), rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
		for _, r := range bom.RigheDaDecidere {
			if r == ancoraggio.RifRigaProposta(uid(0x73c)) || r == ancoraggio.RifRigaProposta(uid(0x73d)) {
				t.Errorf("righe da decidere %v", bom.RigheDaDecidere)
			}
		}
	})
	t.Run("le righe d'arco decise non sono archi proposti", func(t *testing.T) {
		bom := strutturaDelProdotto(t, valuta(t, scenaCollegamento(t), m, nil), rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
		var proposti []string
		for _, a := range bom.ArchiContesto {
			if a.Origine == ancoraggio.OrigineArcoProposto {
				proposti = append(proposti, a.Padre+">"+a.Figlio)
			}
		}
		if len(proposti) != 2 || proposti[0] != nodoB("#1")+">"+nodoB("#3") || proposti[1] != nodoB("#1")+">"+nodoB("#4") {
			t.Errorf("archi proposti del contesto %v: solo le due righe aperte", proposti)
		}
	})
	t.Run("una proposta chiusa non è «assegna» (T-B4-33)", func(t *testing.T) {
		th := scenaCollegamento(t)
		th.Proposte[len(th.Proposte)-1].Stato = "scartata"
		n := nodoIn(t, strutturaDelProdotto(t, valuta(t, th, m, nil), rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
		if len(n.Codice.Documentale) != 1 || n.Codice.Documentale[0].AllegatoID != aDisegno {
			t.Errorf("codici documentali %+v: la scansione con la proposta scartata non è associata", n.Codice.Documentale)
		}
	})
	t.Run("la revisione del codice del target confermato è un candidato del prodotto (T-E1-06, T-B4-04)", func(t *testing.T) {
		th := scenaCollegamento(t)
		th.Componenti[0].Codice = "7120100A1"
		r := nodoIn(t, strutturaDelProdotto(t, valuta(t, th, m, nil), rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#1"))
		trovato := false
		for _, c := range r.Codice.Identita.CandidatiRevisione {
			trovato = trovato || (c.Fonte == ancoraggio.FonteRevisioneCodiceTarget && c.Valore == "1" && c.Entita == rifProdB)
		}
		if !trovato {
			t.Errorf("candidati della radice %+v: la revisione 1 del codice del target", r.Codice.Identita.CandidatiRevisione)
		}
	})
	t.Run("la posizione del codice di un messaggio è quella della lettura (T-B4-04)", func(t *testing.T) {
		n := nodoIn(t, strutturaDelProdotto(t, valuta(t, scenaCollegamento(t), m, nil), rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#2"))
		for _, c := range n.Codice.Identita.CandidatiRevisione {
			if c.Fonte != ancoraggio.FonteRevisioneMessaggio {
				continue
			}
			if p := c.Posizione.Testo; c.Posizione.Tipo != "testo" || p == nil || p.Intervallo.Fine-p.Intervallo.Inizio != len("7120200A2") {
				t.Errorf("posizione %+v: l'intervallo della lettura nel testo del messaggio", c.Posizione)
			}
			return
		}
		t.Error("nessun candidato dal messaggio")
	})
}

// TestIlMarcatoreScrittoDaUnaParteSola (T-B4-30, T-B2-02; R86): con la forma dello STEP che legge anche la sola base, il
// target con il marcatore A e la radice senza marcatore hanno la stessa identità: lo STEP è candidato della fonte, e
// ancoraggio ne fa la struttura candidata del prodotto, con la stessa radice.
func TestIlMarcatoreScrittoDaUnaParteSola(t *testing.T) {
	f := famigliaCatena()
	for i := range f.Forme {
		if f.Forme[i].ID == "step" {
			f.Forme[i].Parti = []grammatica.Parte{parteC(grammatica.TipoParteBase),
				{Tipo: grammatica.TipoParteMarcatore, Letterali: []string{"A", "B"}, Min: 0, Max: 1}}
		}
	}
	m := motoreConFamiglia(t, f)
	th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
	th.Identificativi = []fotorfq.Identificativo{confermato("7120100A")}
	fs := fattiStepF(t, sha("e"), []nodoF{{"#1", "7120100", ""}, {"#2", "7120200", ""}}, []arcoF{{"#1", "#2", 1}})
	conAllegato(&th, allegato(uid(0x781), 1, "assieme-acme.stp", "stp", sha("e")), &fs)
	v := valuta(t, th, m, nil)
	p := prodotto(t, v, "identificativo:7120100A")
	if statoMotivo(p.Fonte) != "in_attesa_di_conferma/documento_candidato" || len(p.Fonte.Candidati) != 1 || p.Struttura != ancoraggio.StatoStrutturaCandidata {
		t.Fatalf("fonte %s, %d candidati, struttura %s: il marcatore scritto da una parte sola non cambia niente", statoMotivo(p.Fonte), len(p.Fonte.Candidati), p.Struttura)
	}
	s := strutturaDelProdotto(t, v, "identificativo:7120100A", ancoraggio.StatoStrutturaCandidata)
	if len(s.Fonti) != 1 || s.Fonti[0].RadiceCompatibile != p.Fonte.Candidati[0].RadiceCompatibile {
		t.Errorf("le fonti della struttura %+v e i candidati della fonte %+v dicono la stessa cosa (T-B2-02)", s.Fonti, p.Fonte.Candidati)
	}
}

// TestIQualificatoriDelTarget (6.4.5, regola 7; R75 A): un prodotto dello scenario porta i qualificatori attribuiti
// della sua lettura, e il candidato a livello prodotto di un file che li scrive nel nome ha la dimensione dei
// qualificatori uguale. La famiglia ACME con il prefisso di fase anche nel nome del file attiva un meccanismo, mai il
// significato di un cliente.
func TestIQualificatoriDelTarget(t *testing.T) {
	f := famigliaACME()
	conNome := []string{"oggetto", "corpo", "storia", "nome_file"}
	f.Forme[0].Selettori = conNome
	f.Affissi[0].Riconoscimento, f.Affissi[0].Attribuzione = conNome, conNome
	m := motoreConFamiglia(t, f)
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per P7120100.\r\nGrazie")
	conAllegato(&th, allegato(uid(0x781), 1, "P7120100.pdf", "pdf", sha("e")), nil)
	tid := threadACME
	caso := &valutazione.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: ancoraggio.AutoritaScenario,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:corrente", Uso: "pertinente", Origine: "scenario"}}}
	v := valuta(t, th, m, caso)
	a := ancoraggioDel(t, v, uid(0x781))
	esito := ""
	for _, c := range a.Candidati {
		if c.Target != "scenario:ACME-SCENARIO:1" || c.Livello != ancoraggio.LivelloProdotto {
			continue
		}
		for _, d := range c.Dimensioni {
			if d.Dimensione == ancoraggio.DimensioneQualificatori {
				esito = string(d.Esito)
			}
		}
	}
	if esito != string(motorea.CompatibilitaUguale) {
		t.Errorf("candidati %+v: la dimensione dei qualificatori %q, attesa uguale", a.Candidati, esito)
	}
}

// TestIlCollegamentoÈDeterministico: gli elenchi della fotografia permutati, anche le rimozioni aperte, danno gli stessi
// byte canonici, e la fotografia di chi chiama non cambia. Nella scena ci sono due rimozioni aperte dello stesso arco da
// due documenti STEP: i due conflitti differiscono solo per il documento, e l'ordine non dipende dalla fotografia.
func TestIlCollegamentoÈDeterministico(t *testing.T) {
	m := motoreCatena(t)
	th := scenaCollegamento(t)
	th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cSciolto, QtaWorking: 2},
		{StepDocumentoID: uid(0x7aa), PadreID: cProdotto, FiglioID: cSciolto, QtaWorking: 2}}
	prima := canonicoDi(t, th)
	v := valuta(t, th, m, nil)
	b1 := canonicoDi(t, v)
	if canonicoDi(t, th) != prima {
		t.Fatal("ValutaProdotti ha cambiato la fotografia di chi chiama")
	}
	if len(v.Conflitti) != 2 {
		t.Fatalf("conflitti %+v: la scena chiede le due rimozioni dello stesso arco", v.Conflitti)
	}
	p := th
	p.Allegati = rovescia(th.Allegati)
	p.RigheComponenteProposta = rovescia(th.RigheComponenteProposta)
	p.RigheRelazioneProposta = rovescia(th.RigheRelazioneProposta)
	p.Componenti = rovescia(th.Componenti)
	p.Relazioni = rovescia(th.Relazioni)
	p.Documenti = rovescia(th.Documenti)
	p.Proposte = rovescia(th.Proposte)
	p.RimozioniAperte = rovescia(th.RimozioniAperte)
	if b2 := canonicoDi(t, valuta(t, p, m, nil)); b2 != b1 {
		t.Error("gli elenchi permutati danno un altro esito")
	}
}

// TestLOrigineDelloScenarioInAncoraggio (T-B5-06; R75 A): il prodotto dello scenario arriva ad
// ancoraggio con l'origine «scenario», che entra nell'impronta dei target (EsitoAncoraggi.HashTarget). La prova chiama
// ancoraggio.ProponiAncoraggi con gli stessi ingressi, preparati qui (il messaggio con il suo uso, la lettura principale
// del candidato, il contesto senza file né decisioni): HashTarget coincide con l'origine «scenario», non con
// «confermato».
func TestLOrigineDelloScenarioInAncoraggio(t *testing.T) {
	m := motoreCatena(t)
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100B.\r\nGrazie")
	tid := threadACME
	caso := &valutazione.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: ancoraggio.AutoritaScenario,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:corrente", Uso: "pertinente", Origine: "scenario"}}}
	v := valuta(t, th, m, caso)
	p := prodotto(t, v, "scenario:ACME-SCENARIO:1")
	if len(v.Prodotti.Candidati) != 1 {
		t.Fatalf("candidati %+v: la scena chiede un candidato solo", v.Prodotti.Candidati)
	}

	msg := th.Messaggi[0]
	d, err := estrazione.DaMessaggio(msg)
	if err != nil {
		t.Fatal(err)
	}
	_, usi := valutazione.RichiestaDelThread(th, caso, map[uuid.UUID]evidenze.DocumentoEvidenze{msg.ID: d})
	uso, ok := usi[d.BundleID]
	if !ok {
		uso = evidenze.UsoSconosciuto(d.BundleID)
	}
	interp, err := m.Interpreta(d, uso)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]evidenze.Localizzatore{}
	for _, u := range d.Unita {
		pos[u.ID] = u.Posizione
	}
	ctx := ancoraggio.ContestoStrutturale{CodiciProposti: map[string]motorea.CodiceComposto{}}
	var lettura *motorea.LetturaForma
	for _, l := range interp.Letture {
		posizione := pos[l.UnitaID]
		if l.Assoluto != nil {
			a := *l.Assoluto
			posizione = evidenze.Localizzatore{Tipo: "testo", Testo: &a}
		}
		ctx.CodiciMessaggi = append(ctx.CodiciMessaggi, ancoraggio.CodiceDiMessaggio{MessaggioID: msg.ID, Lettura: l, Posizione: posizione})
		if l.ID == v.Prodotti.Candidati[0].Lettura {
			f := l.Forma
			lettura = &f
		}
	}
	if lettura == nil || lettura.Marcatore == nil {
		t.Fatalf("la lettura principale del candidato, con il marcatore: %+v", lettura)
	}
	target := ancoraggio.ProdottoRichiesto{Rif: p.Rif, Autorita: p.Autorita, ClienteID: clienteACME, Namespace: lettura.Namespace,
		CodiceRichiesto: p.CodiceRichiesto, Base: p.Base, Marcatore: lettura.Marcatore.Valore, Revisione: lettura.Revisione}
	if len(lettura.Affissi) > 0 {
		target.Qualificatori = lettura.Affissi
	}
	for origine, uguale := range map[ancoraggio.OrigineDato]bool{ancoraggio.OrigineScenario: true, ancoraggio.OrigineConfermato: false} {
		target.Origine = string(origine)
		e, err := ancoraggio.ProponiAncoraggi(nil, []ancoraggio.ProdottoRichiesto{target}, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if (e.HashTarget == v.Ancoraggi.HashTarget) != uguale {
			t.Errorf("origine %q: HashTarget %s, quello di ValutaProdotti %s (atteso uguale: %v)", origine, e.HashTarget, v.Ancoraggi.HashTarget, uguale)
		}
	}
}

func rovescia[T any](s []T) []T {
	out := make([]T, len(s))
	for i, x := range s {
		out[len(s)-1-i] = x
	}
	return out
}

// TestLaFonteConLaRegolaDelMarcatore (T-B4-30, T-B2-02; R76 b A, R86): un target con il marcatore B e uno STEP la cui
// radice legge la A: la radice non ha l'identità del prodotto, quindi lo STEP non è candidato della fonte, e ancoraggio
// non ne fa una struttura del prodotto; con il marcatore A lo STEP è candidato in valutazione e in ancoraggio, con la
// stessa radice.
func TestLaFonteConLaRegolaDelMarcatore(t *testing.T) {
	m := motoreCatena(t)
	scena := func(codice string) valutazione.ValutazioneProdotti {
		th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
		th.Identificativi = []fotorfq.Identificativo{confermato(codice)}
		f := fattiStepF(t, sha("e"), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 1}})
		conAllegato(&th, allegato(uid(0x781), 1, "assieme-acme.stp", "stp", sha("e")), &f)
		return valuta(t, th, m, nil)
	}

	b := scena("7120100B")
	pb := prodotto(t, b, "identificativo:7120100B")
	if statoMotivo(pb.Fonte) != "assente/prodotto_senza_componente" || len(pb.Fonte.Candidati) != 0 {
		t.Errorf("marcatore B contro la radice A: fonte %s, candidati %d", statoMotivo(pb.Fonte), len(pb.Fonte.Candidati))
	}
	if pb.Struttura != ancoraggio.StatoStrutturaNessuna || len(b.Ancoraggi.Strutture) != 0 {
		t.Errorf("marcatore B: struttura %s, %d strutture in ancoraggio", pb.Struttura, len(b.Ancoraggi.Strutture))
	}
	senza := false
	for _, d := range b.Ancoraggi.Diagnostiche {
		senza = senza || d.Codice == ancoraggio.CodiceTargetSenzaStruttura
	}
	if !senza {
		t.Error("marcatore B: ancoraggio dice il target senza struttura")
	}

	a := scena("7120100A")
	pa := prodotto(t, a, "identificativo:7120100A")
	if statoMotivo(pa.Fonte) != "in_attesa_di_conferma/documento_candidato" || len(pa.Fonte.Candidati) != 1 || pa.Struttura != ancoraggio.StatoStrutturaCandidata {
		t.Fatalf("marcatore A: fonte %s, %d candidati, struttura %s", statoMotivo(pa.Fonte), len(pa.Fonte.Candidati), pa.Struttura)
	}
	s := strutturaDelProdotto(t, a, "identificativo:7120100A", ancoraggio.StatoStrutturaCandidata)
	if len(s.Fonti) != 1 || s.Fonti[0].Sha256 != pa.Fonte.Candidati[0].Sha256 || s.Fonti[0].RadiceCompatibile != pa.Fonte.Candidati[0].RadiceCompatibile ||
		s.Fonti[0].RadiceCompatibile != "#1" {
		t.Errorf("le fonti della struttura %+v e i candidati della fonte %+v dicono la stessa cosa (T-B2-02)", s.Fonti, pa.Fonte.Candidati)
	}
}

// TestIlProdottoDelloScenarioConLaSuaLettura (R75 A; T-B4-30): un prodotto dello scenario porta la lettura principale
// della mail per intero, con il marcatore: con la B, la radice A di uno STEP del thread non è sua, né per la fonte né
// per le strutture.
func TestIlProdottoDelloScenarioConLaSuaLettura(t *testing.T) {
	m := motoreCatena(t)
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100B.\r\nGrazie")
	f := fattiStepF(t, sha("e"), []nodoF{{"#1", "7120100A", ""}}, nil)
	conAllegato(&th, allegato(uid(0x781), 1, "assieme-acme.stp", "stp", sha("e")), &f)
	tid := threadACME
	caso := &valutazione.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: ancoraggio.AutoritaScenario,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:corrente", Uso: "pertinente", Origine: "scenario"}}}
	v := valuta(t, th, m, caso)
	p := prodotto(t, v, "scenario:ACME-SCENARIO:1")
	if p.Base.Normalizzata != "7120100" || len(p.Fonte.Candidati) != 0 || p.Fonte.Stato != valutazione.FonteAssente || p.Struttura != ancoraggio.StatoStrutturaNessuna {
		t.Errorf("scenario 7120100B: base %q, fonte %s con %d candidati, struttura %s", p.Base.Normalizzata, statoMotivo(p.Fonte), len(p.Fonte.Candidati), p.Struttura)
	}
}
