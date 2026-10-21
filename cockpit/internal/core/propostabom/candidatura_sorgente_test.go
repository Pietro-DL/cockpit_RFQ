// L1 — giro G39, corsia motore (G39-02 [U]): lo STEP candidato del prodotto finito letto con il Motore A
// (candidatura_sorgente.go). Esiti sintetici: due prodotti finiti, STEP e PDF con i loro ancoraggi. Tutto è inventato.
package propostabom_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

func TestG39LoSTEPCandidatoDelFinitoColMotoreA(t *testing.T) {
	id := func(n byte) uuid.UUID { return uuid.UUID{0x39, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, n} }
	c1, c2 := id(1), id(2)
	sUnivoco, sDue, sDiscordante, pdf, sNodo, sFuori := id(11), id(12), id(13), id(14), id(15), id(16)
	sStruttura, sFonte, sAltraFonte, sMisto := id(17), id(18), id(19), id(20)
	nome := evidenze.Selettore{Contesto: evidenze.ContestoNomeFile}
	lettura := func(lid, testo, base, marcatore string) motorea.LetturaCodice {
		l := motorea.LetturaCodice{ID: lid, Forma: motorea.LetturaForma{Selettore: nome, Originale: testo, Base: motorea.BaseLetta{Normalizzata: base}}}
		if marcatore != "" {
			l.Forma.Marcatore = &motorea.ParteLetta{Valore: marcatore}
		}
		return l
	}
	sulProdotto := func(target, lid string, base, marc motorea.Compatibilita, conflitti ...string) ancoraggio.CandidatoAncoraggio {
		return ancoraggio.CandidatoAncoraggio{Target: target, Livello: ancoraggio.LivelloProdotto, Letture: []string{lid}, Conflitti: conflitti,
			Dimensioni: []ancoraggio.DimensioneCompatibilita{{Dimensione: ancoraggio.DimensioneBase, Esito: base, Lettura: lid, Rif: target},
				{Dimensione: ancoraggio.DimensioneMarcatore, Esito: marc, Lettura: lid, Rif: target}}}
	}
	et := &valutazione.EsitoThread{
		ProdottiValutati: []valutazione.ProdottoValutato{{Rif: "p1", ComponenteID: &c1, CodiceRichiesto: "9110001A"},
			{Rif: "p2", ComponenteID: &c2, CodiceRichiesto: "9110002A"}, {Rif: "p3"}},
		File: []valutazione.FileInterpretato{
			{AllegatoID: sUnivoco, Interpretazione: motorea.Interpretazione{Letture: []motorea.LetturaCodice{lettura("l1", "9110001", "9110001", "")}}},
			{AllegatoID: sDue, Interpretazione: motorea.Interpretazione{Letture: []motorea.LetturaCodice{lettura("l2", "9110001A", "9110001", "A")}}},
		},
	}
	et.Ancoraggi.File = []ancoraggio.AncoraggioFile{
		// il nome senza il marcatore contro il prodotto con il marcatore: compatibile parziale, un candidato solo
		{AllegatoID: sUnivoco, Associazione: ancoraggio.AssociazioneCandidatoUnico,
			Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto("p1", "l1", motorea.CompatibilitaUguale, motorea.CompatibilitaParziale)}},
		// due prodotti finiti: un altro finito non è un'evidenza discordante, le due candidature sono non univoche (giro
		// G39-bis, revisione indipendente)
		{AllegatoID: sDue, Associazione: ancoraggio.AssociazioneAmbiguo, Candidati: []ancoraggio.CandidatoAncoraggio{
			sulProdotto("p1", "l2", motorea.CompatibilitaUguale, motorea.CompatibilitaUguale),
			sulProdotto("p2", "l2", motorea.CompatibilitaParziale, motorea.CompatibilitaUguale)}},
		// una dimensione discordante: non è un candidato
		{AllegatoID: sDiscordante, Associazione: ancoraggio.AssociazioneCandidatoUnico, Candidati: []ancoraggio.CandidatoAncoraggio{
			sulProdotto("p1", "l3", motorea.CompatibilitaUguale, motorea.CompatibilitaDiscordante, ancoraggio.DimensioneMarcatore)}},
		// un PDF del prodotto: non è uno STEP
		{AllegatoID: pdf, Associazione: ancoraggio.AssociazioneCandidatoUnico, Candidati: []ancoraggio.CandidatoAncoraggio{
			sulProdotto("p1", "l4", motorea.CompatibilitaUguale, motorea.CompatibilitaUguale)}},
		// lo STEP di un sottoassieme (livello componente): resta un file ordinario (PB → R5 → AR)
		{AllegatoID: sNodo, Associazione: ancoraggio.AssociazioneCandidatoUnico, Candidati: []ancoraggio.CandidatoAncoraggio{
			{Target: "componente:x", Livello: ancoraggio.LivelloComponente}}},
		// un prodotto valutato senza componente
		{AllegatoID: sFuori, Associazione: ancoraggio.AssociazioneCandidatoUnico, Candidati: []ancoraggio.CandidatoAncoraggio{
			sulProdotto("p3", "l5", motorea.CompatibilitaUguale, motorea.CompatibilitaUguale)}},
	}
	et.Ancoraggi.File = append(et.Ancoraggi.File,
		// candidato del prodotto e anche nodo di un'altra struttura: evidenza discordante, nessuna candidatura (giro G39-bis)
		ancoraggio.AncoraggioFile{AllegatoID: sMisto, Associazione: ancoraggio.AssociazioneAmbiguo, Candidati: []ancoraggio.CandidatoAncoraggio{
			sulProdotto("p1", "l6", motorea.CompatibilitaUguale, motorea.CompatibilitaUguale), {Target: "componente:y", Livello: ancoraggio.LivelloComponente}}},
	)
	// la struttura candidata del secondo prodotto con la radice dello STEP; il documento candidato della fonte del primo
	// (documento_del_prodotto); un candidato della fonte con un'altra origine e senza radice compatibile non conta
	et.ProdottiValutati[1].StruttureCandidate = []valutazione.StrutturaCandidata{{AllegatoID: sStruttura, Radice: "nodo:s:#1", RadiceDelFile: true,
		Compatibilita: motorea.CompatibilitaUguale}, {AllegatoID: sAltraFonte, Radice: "nodo:t:#4"}}
	et.ProdottiValutati[0].Fonte.Candidati = []ancoraggio.DocumentoCandidato{{Origine: ancoraggio.CandidatoDaDocumentoDelProdotto, AllegatoID: &sFonte},
		{Origine: "altro", AllegatoID: &sAltraFonte}}
	step := map[uuid.UUID]bool{sUnivoco: true, sDue: true, sDiscordante: true, sNodo: true, sFuori: true, sStruttura: true, sFonte: true,
		sAltraFonte: true, sMisto: true}
	th := &fotorfq.Thread{Componenti: []fotorfq.Componente{{ID: c1, Codice: "9110001A"}}}
	cc := propostabom.CandidatureSorgenteDelFinito(et, th, func(a uuid.UUID) bool { return step[a] })
	attese := []propostabom.CandidaturaSorgente{
		{AllegatoID: sUnivoco, ProdottoID: c1, Prodotto: "p1", CodiceProdotto: "9110001A", Univoca: true, Evidenze: []propostabom.EvidenzaCandidatura{
			{Lettura: "l1", Selettore: "nome_file", CodiceLetto: "9110001", BaseLetta: "9110001", EsitoBase: "uguale", EsitoMarcatore: "compatibile_parziale"}}},
		{AllegatoID: sDue, ProdottoID: c1, Prodotto: "p1", CodiceProdotto: "9110001A", Evidenze: []propostabom.EvidenzaCandidatura{
			{Lettura: "l2", Selettore: "nome_file", CodiceLetto: "9110001A", BaseLetta: "9110001", MarcatoreLetto: "A", EsitoBase: "uguale", EsitoMarcatore: "uguale"}}},
		{AllegatoID: sDue, ProdottoID: c2, Prodotto: "p2", CodiceProdotto: "9110002A", Evidenze: []propostabom.EvidenzaCandidatura{
			{Lettura: "l2", Selettore: "nome_file", CodiceLetto: "9110001A", BaseLetta: "9110001", MarcatoreLetto: "A", EsitoBase: "compatibile_parziale", EsitoMarcatore: "uguale"}}},
		{AllegatoID: sStruttura, ProdottoID: c2, Prodotto: "p2", CodiceProdotto: "9110002A", Univoca: true, Evidenze: []propostabom.EvidenzaCandidatura{
			{Selettore: propostabom.SelettoreStrutturaCandidata, CodiceLetto: "nodo:s:#1", EsitoBase: "uguale"}}},
		{AllegatoID: sFonte, ProdottoID: c1, Prodotto: "p1", CodiceProdotto: "9110001A", Univoca: true, Evidenze: []propostabom.EvidenzaCandidatura{
			{Selettore: propostabom.PrefissoSelettoreFonte + "documento_del_prodotto"}}},
	}
	if !reflect.DeepEqual(cc, attese) {
		t.Fatalf("le candidature:\n%+v\nattese\n%+v", cc, attese)
	}
	if got := propostabom.StepCandidatiDelFinito(cc); !reflect.DeepEqual(got, map[uuid.UUID]bool{sUnivoco: true, sDue: true, sStruttura: true, sFonte: true}) {
		t.Errorf("gli STEP candidati: %v", got)
	}
	if cc := propostabom.CandidatureSorgenteDelFinito(nil, nil, nil); len(cc) != 0 {
		t.Errorf("senza esito: %+v", cc)
	}
}

// L1 — giro G39-bis (21/10), corsia motore: l'evidenza discordante (D-2 (c) [U], D-AR04 (iii) [U]) e la rete di sicurezza
// del nome esatto (G39-02 [U], [T-39]). Il finito 12345678Z e un altro finito 12345679Y, il sottoassieme 12345678-100 e un
// componente archiviato 12345678-200 della stessa RFQ. Tutti codici inventati.
func TestG39bisEvidenzaDiscordanteEReteDelNome(t *testing.T) {
	id := func(n byte) uuid.UUID { return uuid.UUID{0x39, 0xb1, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, n} }
	p, q, sotto, arch := id(1), id(2), id(3), id(4)
	rp, rq := "componente:"+p.String(), "componente:"+q.String()
	aSotto, aArch, aNodo, aProprio, aProprioAltro, aConflitto, aDueFonti, aMezzo := id(11), id(12), id(13), id(14), id(15), id(16), id(17), id(18)
	aNome, aNomeMinuscolo, aNomePdf, aNomeNodo, aNomeQ := id(21), id(22), id(23), id(24), id(25)
	sulProdotto := func(rif string, conflitti ...string) ancoraggio.CandidatoAncoraggio {
		return ancoraggio.CandidatoAncoraggio{Target: rif, Livello: ancoraggio.LivelloProdotto, Conflitti: conflitti}
	}
	nodo := func(radici ...string) ancoraggio.CandidatoAncoraggio {
		return ancoraggio.CandidatoAncoraggio{Target: "componente:" + sotto.String(), Livello: ancoraggio.LivelloComponente, RadiciRaggiungibili: radici}
	}
	et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{
		{Rif: rp, ComponenteID: &p, CodiceRichiesto: "12345678Z", StruttureCandidate: []valutazione.StrutturaCandidata{
			{AllegatoID: aProprio, RadiceDelFile: true}, {AllegatoID: aProprioAltro, RadiceDelFile: true}, {AllegatoID: aMezzo, RadiceDelFile: true}},
			Fonte: valutazione.FonteProdotto{Candidati: []ancoraggio.DocumentoCandidato{{Origine: ancoraggio.CandidatoDaMotoreA, AllegatoID: &aDueFonti}}}},
		{Rif: rq, ComponenteID: &q, CodiceRichiesto: "12345679Y",
			Fonte: valutazione.FonteProdotto{Candidati: []ancoraggio.DocumentoCandidato{{Origine: ancoraggio.CandidatoDaMotoreA, AllegatoID: &aDueFonti}}}},
	}}
	et.Ancoraggi.File = []ancoraggio.AncoraggioFile{
		{AllegatoID: aSotto, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rp)}},
		{AllegatoID: aArch, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rp)}},
		{AllegatoID: aNodo, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rp), nodo(rp)}},
		{AllegatoID: aProprio, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rp), nodo(rp)}},
		{AllegatoID: aProprioAltro, Candidati: []ancoraggio.CandidatoAncoraggio{nodo(rp, rq)}},
		{AllegatoID: aConflitto, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rp), sulProdotto(rq, ancoraggio.DimensioneRevisione)}},
		{AllegatoID: aMezzo, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rq)}},
		{AllegatoID: aNome},
		{AllegatoID: aNomeNodo, Candidati: []ancoraggio.CandidatoAncoraggio{nodo(rp)}},
		{AllegatoID: aNomeQ, Candidati: []ancoraggio.CandidatoAncoraggio{sulProdotto(rp)}},
	}
	adesso := time.Now()
	stp := "stp"
	th := &fotorfq.Thread{
		Componenti: []fotorfq.Componente{{ID: p, Codice: "12345678Z", Tipo: "finito"}, {ID: q, Codice: "12345679Y", Tipo: "finito"},
			{ID: sotto, Codice: " 12345678-100 ", Tipo: "sottoassieme"}, {ID: arch, Codice: "12345678-200", Tipo: "sciolto", ArchiviatoIl: &adesso}},
		Allegati: []fotorfq.Allegato{
			{ID: aSotto, NomeFile: "12345678-100.stp", Estensione: &stp}, {ID: aArch, NomeFile: "12345678-200.stp", Estensione: &stp},
			{ID: aNodo, NomeFile: "assieme.stp", Estensione: &stp}, {ID: aProprio, NomeFile: "12345678Z_rev1.stp", Estensione: &stp},
			{ID: aProprioAltro, NomeFile: "telaio.stp", Estensione: &stp}, {ID: aConflitto, NomeFile: "x.stp", Estensione: &stp},
			{ID: aDueFonti, NomeFile: "y.stp", Estensione: &stp}, {ID: aMezzo, NomeFile: "z.stp", Estensione: &stp},
			{ID: aNome, NomeFile: "12345678Z.stp", Estensione: &stp}, {ID: aNomeMinuscolo, NomeFile: `cartella\ 12345678z .STEP`},
			{ID: aNomePdf, NomeFile: "12345678Z.pdf"}, {ID: aNomeNodo, NomeFile: "12345678Z.stp", Estensione: &stp},
			{ID: aNomeQ, NomeFile: "12345679Y.stp", Estensione: &stp},
		},
	}
	tutti := func(uuid.UUID) bool { return true }
	cc := propostabom.CandidatureSorgenteDelFinito(et, th, tutti)
	got := map[[2]uuid.UUID]bool{} // (allegato, prodotto) -> univoca
	for _, c := range cc {
		got[[2]uuid.UUID{c.AllegatoID, c.ProdottoID}] = c.Univoca
	}
	attese := map[[2]uuid.UUID]bool{
		{aArch, p}: true, {aProprio, p}: true, {aConflitto, p}: false, {aDueFonti, p}: false, {aDueFonti, q}: false, {aMezzo, q}: false,
		{aNome, p}: true, {aNomeMinuscolo, p}: true,
		// un altro finito non toglie la candidatura, la rende non univoca (revisione indipendente del giro G39-bis):
		// aMezzo, legato a P dalla struttura e ancorato al finito Q; aNomeQ, ancorato a P con il nome del finito Q
		{aMezzo, p}: false, {aNomeQ, p}: false,
	}
	if !reflect.DeepEqual(got, attese) {
		t.Fatalf("le candidature (allegato, prodotto) -> univoca:\n%v\nattese\n%v\n%+v", got, attese, cc)
	}
	for _, c := range cc {
		if c.AllegatoID == aNome || c.AllegatoID == aNomeMinuscolo {
			if len(c.Evidenze) != 1 || c.Evidenze[0].Selettore != propostabom.SelettoreNomeFileCodiceProdotto || c.Evidenze[0].EsitoBase != "uguale" {
				t.Errorf("l'evidenza della rete del nome: %+v", c.Evidenze)
			}
		}
	}
	// senza la fotografia: niente confronto del nome con i codici e niente rete del nome
	got = map[[2]uuid.UUID]bool{}
	for _, c := range propostabom.CandidatureSorgenteDelFinito(et, nil, tutti) {
		got[[2]uuid.UUID{c.AllegatoID, c.ProdottoID}] = c.Univoca
	}
	if _, ok := got[[2]uuid.UUID{aSotto, p}]; !ok {
		t.Errorf("senza la fotografia lo STEP del nome del sottoassieme resta legato al finito: %v", got)
	}
	if _, ok := got[[2]uuid.UUID{aNome, p}]; ok {
		t.Errorf("senza la fotografia la rete del nome non c'e': %v", got)
	}
	// eStep falso: niente rete del nome
	for _, c := range propostabom.CandidatureSorgenteDelFinito(et, th, func(a uuid.UUID) bool { return a != aNome }) {
		if c.AllegatoID == aNome {
			t.Errorf("la rete del nome su un allegato che non e' uno STEP per chi chiama: %+v", c)
		}
	}
}

// L1 — giro G39-bis, revisione indipendente (rilievo bloccante su D-2 (c) [U]): il nome dello STEP che comincia con il
// codice di un altro componente seguito da un separatore. Ogni STEP è ancorato al finito con la sola base, come fa la
// grammatica che legge solo la base. Tre RFQ: il finito 12345678Z con il sottoassieme 12345678-100; il finito 12345678Z
// con il componente 12345678 (il cui codice è l'inizio di quello del finito: il nome è ambiguo, [T-39] domanda G39-06); il
// finito 12345690-100-A con il sottoassieme 12345690-100. Codici inventati.
func TestG39bisNomeCheCominciaColCodiceDiUnAltroComponente(t *testing.T) {
	type caso struct {
		nome  string
		esito string // "tolta", "univoca", "non_univoca"
	}
	prova := func(t *testing.T, codiceFinito, codiceAltro, tipoAltro string, casi []caso) {
		t.Helper()
		fin, altro := uuid.New(), uuid.New()
		rif := "componente:" + fin.String()
		et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{{Rif: rif, ComponenteID: &fin, CodiceRichiesto: codiceFinito}}}
		stp := "stp"
		th := &fotorfq.Thread{Componenti: []fotorfq.Componente{{ID: fin, Codice: codiceFinito, Tipo: "finito"}, {ID: altro, Codice: codiceAltro, Tipo: tipoAltro}}}
		ids := map[uuid.UUID]caso{}
		for _, c := range casi {
			a := uuid.New()
			ids[a] = c
			et.Ancoraggi.File = append(et.Ancoraggi.File, ancoraggio.AncoraggioFile{AllegatoID: a,
				Candidati: []ancoraggio.CandidatoAncoraggio{{Target: rif, Livello: ancoraggio.LivelloProdotto}}})
			th.Allegati = append(th.Allegati, fotorfq.Allegato{ID: a, NomeFile: c.nome, Estensione: &stp})
		}
		got := map[uuid.UUID]string{}
		for _, c := range propostabom.CandidatureSorgenteDelFinito(et, th, func(uuid.UUID) bool { return true }) {
			if c.ProdottoID != fin {
				t.Errorf("%s: candidatura di un altro prodotto: %+v", ids[c.AllegatoID].nome, c)
			}
			got[c.AllegatoID] = map[bool]string{true: "univoca", false: "non_univoca"}[c.Univoca]
		}
		for a, c := range ids {
			esito := got[a]
			if esito == "" {
				esito = "tolta"
			}
			if esito != c.esito {
				t.Errorf("finito %s, %s %s: lo STEP «%s» è %s, atteso %s", codiceFinito, tipoAltro, codiceAltro, c.nome, esito, c.esito)
			}
		}
	}
	prova(t, "12345678Z", "12345678-100", "sottoassieme", []caso{
		{"12345678-100.stp", "tolta"}, {"12345678-100_rev2.stp", "tolta"}, {"12345678-100 A.stp", "tolta"},
		{"12345678-100-B.stp", "tolta"}, {"12345678-100.1.stp", "tolta"}, {`cartella\12345678-100_rev2.STEP`, "tolta"},
		// giro G39-ter: dopo il codice del sottoassieme una cifra, e il codice del finito non copre più nome: il motore non
		// sa decidere (era univoca nel giro G39-bis)
		{"12345678-1000.stp", "non_univoca"}, {"12345678Z_rev2.stp", "univoca"}, {"12345678-001.stp", "univoca"},
		{"rev2_12345678-100.stp", "univoca"}, // il codice non all'inizio del nome non conta (limite dichiarato)
	})
	prova(t, "12345678Z", "12345678", "sciolto", []caso{
		{"12345678.stp", "non_univoca"}, {"12345678 Z.stp", "non_univoca"}, {"12345678_rev2.stp", "non_univoca"},
		{"12345678Z.stp", "univoca"}, {"12345678Z_rev1.stp", "univoca"},
	})
	prova(t, "12345690-100-A", "12345690-100", "sottoassieme", []caso{
		{"12345690-100-A.stp", "univoca"}, {"12345690-100-A_rev3.stp", "univoca"}, {"12345690-100_rev1.stp", "non_univoca"},
	})
	// un componente archiviato non conta
	t.Run("archiviato", func(t *testing.T) {
		fin, sotto, a := uuid.New(), uuid.New(), uuid.New()
		rif := "componente:" + fin.String()
		adesso := time.Now()
		stp := "stp"
		et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{{Rif: rif, ComponenteID: &fin}}}
		et.Ancoraggi.File = []ancoraggio.AncoraggioFile{{AllegatoID: a, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: rif, Livello: ancoraggio.LivelloProdotto}}}}
		th := &fotorfq.Thread{Componenti: []fotorfq.Componente{{ID: fin, Codice: "12345678Z"}, {ID: sotto, Codice: "12345678-100", ArchiviatoIl: &adesso}},
			Allegati: []fotorfq.Allegato{{ID: a, NomeFile: "12345678-100_rev2.stp", Estensione: &stp}}}
		if cc := propostabom.CandidatureSorgenteDelFinito(et, th, func(uuid.UUID) bool { return true }); len(cc) != 1 || !cc[0].Univoca {
			t.Errorf("con il sottoassieme archiviato: %+v", cc)
		}
	})
}
