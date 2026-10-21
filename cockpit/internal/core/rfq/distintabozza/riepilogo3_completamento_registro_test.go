package distintabozza

// L1 — il completamento di R4 formato 3 sul registro sintetico del formato 3 (formato3_aiuti_test.go): gli avvisi del
// registro (EM §8.2, §3.7, §7.3), gli effetti e le origini degli esclusi (EM §7.4, U2 §6.1), le condizioni (c) ed (e) di
// prima-istanza-1 (U2 §5.4). Dati sintetici.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// ingressiProva3R4: gli ingressi di R4 sullo snapshot e sul registro sintetici del formato 3, con la sorgente valida; modC
// cambia il contenuto del registro prima della composizione.
func ingressiProva3R4(t *testing.T, modC func(id idProva3, c *importazione.Contenuto)) (*ingressiR4, idProva3) {
	t.Helper()
	id := idSintetici3()
	c := registroProva3(t, id)
	if modC != nil {
		modC(id, &c)
	}
	reg, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	s, err := LeggiSnapshot3(snapshotJSON(t, snapshotProva3(id, reg.Impronta)))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	can, err := s.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	st := confermabom.NuovoStatoRFQ(id.thread, 7, []confermabom.Componente{{ID: id.prodotto, CodiceLetterale: "ACM-1000", Tipo: "finito"}}, nil, nil,
		map[uuid.UUID]confermabom.FonteAutorizzata{})
	vc, err := st.VersioneCorrente(id.prodotto)
	if err != nil {
		t.Fatal(err)
	}
	doc := idDi("documento-sorgente")
	al := id.allegato
	in := &ingressiR4{thread: id.thread, prodotto: id.prodotto, utente: id.utente, snap: s, canonico: can, stato: st, cert: map[uuid.UUID]Certificazione{},
		cliente: id.clienteE(), vc: vc, base: StatoBase{Stato: BaseValida, Motivi: []string{}}, scaduti: []RifScaduto{},
		sorgente: SorgenteR4{AllegatoID: &al, Sha256: id.sha, Gruppo: gruppoProva3, Valida: true, DocumentoID: &doc}, registro: &reg,
		fatti: associazioni.FattiPiano{Occorrenze: map[uuid.UUID]associazioni.OccorrenzaPiano{}, Attive: map[uuid.UUID]associazioni.Associazione{},
			Chiuse: map[[2]uuid.UUID]bool{}, CopieSenzaAR: map[uuid.UUID]uuid.UUID{}, Documenti: map[uuid.UUID][]associazioni.DocumentoPiano{},
			DelProdotto: map[uuid.UUID]bool{}, ProdottiDi: map[uuid.UUID][]uuid.UUID{}},
		scrittiNAS: map[uuid.UUID]int{}, effettiTipo: map[string]string{}, stessoStep: map[string][]ProdottoStessoStep{}, shaS8: map[string]bool{},
		righeNI: map[string][]db.ListRighePerShaChiaveRow{}, righeSha: map[string][]db.ListRigheStrutturaR4Row{},
		archiNI: map[string][]db.ListArchiPerShaChiaveRow{}, avvisiMotore: true, esitoMotore: &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{}}}
	in.completaFattiPiano()
	return in, id
}

func calcolaIn(t *testing.T, in *ingressiR4) RiepilogoR4 {
	t.Helper()
	r, err := calcolaR4(in)
	if err != nil {
		t.Fatalf("riepilogo: %v", err)
	}
	return r
}

// aggiungiRiferimento: un riferimento in piu' fra quelli della voce del rif.
func aggiungiRiferimento(t *testing.T, c *importazione.Contenuto, rif string, r map[string]any) {
	t.Helper()
	for i := range c.Proposta.Nodi {
		if c.Proposta.Nodi[i].Origine.Rif != rif {
			continue
		}
		var rr []any
		if !nullo(c.Proposta.Nodi[i].Riferimenti) {
			if err := json.Unmarshal(c.Proposta.Nodi[i].Riferimenti, &rr); err != nil {
				t.Fatal(err)
			}
		}
		c.Proposta.Nodi[i].Riferimenti = rawDi(t, append(rr, r))
		return
	}
	t.Fatalf("voce %s assente", rif)
}

// EM §8.2, §3.7, §7.3: gli avvisi del registro della versione, senza il motore.
func TestR4AvvisiDelRegistro(t *testing.T) {
	in, id := ingressiProva3R4(t, nil)
	r := calcolaIn(t, in)
	for _, c := range []string{AvvisoEquivalenteNonAdottato, AvvisoCorrezioneCartiglioNonAdottat, AvvisoPreassociazioneEmittenteDiv, AvvisoNonInclusoSegnaleCambiato} {
		if len(trova(r.Avvisi, c)) != 0 {
			t.Fatalf("%s senza motivo: %+v", c, trova(r.Avvisi, c))
		}
	}
	f0 := r.FirmaAnteprima
	// EQUIVALENTE_NON_ADOTTATO: il P/N K4 della voce del telaio e' un altro codice
	eq := map[string]any{"tipo": "equivalente_k4", "equivalente": map[string]any{"namespace": "acme", "base": "7120010", "prove": []any{}}, "fonte": "cartiglio"}
	conEq := func(id idProva3, c *importazione.Contenuto) { aggiungiRiferimento(t, c, id.rif("#7"), eq) }
	in, _ = ingressiProva3R4(t, conEq)
	r = calcolaIn(t, in)
	if a := trova(r.Avvisi, AvvisoEquivalenteNonAdottato); len(a) != 1 || a[0].ChiaveBozza != "k-0007" || a[0].Valore != "7120010" || len(a[0].Rimedi) != 2 {
		t.Fatalf("EQUIVALENTE_NON_ADOTTATO: %+v", a)
	}
	if r.FirmaAnteprima == f0 {
		t.Fatal("gli avvisi del registro sono nella firma")
	}
	// con una decisione alternativa motivata sul nodo (adotta o rifiuta) l'avviso cade
	in, _ = ingressiProva3R4(t, conEq)
	in.snap.Decisioni = append(in.snap.Decisioni, Decisione3{ID: "di-0009", Tipo: DecisioneIdentita, ChiaveBozza: "k-0007", Modo: ModoAlternativa,
		Motivo: "il P/N del cartiglio e' di un'altra variante"})
	if r = calcolaIn(t, in); len(trova(r.Avvisi, AvvisoEquivalenteNonAdottato)) != 0 {
		t.Fatalf("con il rifiuto motivato: %+v", trova(r.Avvisi, AvvisoEquivalenteNonAdottato))
	}
	// nella working va il P/N (con i separatori diversi): niente avviso
	in, _ = ingressiProva3R4(t, func(id idProva3, c *importazione.Contenuto) {
		aggiungiRiferimento(t, c, id.rif("#7"), map[string]any{"tipo": "equivalente_k4",
			"equivalente": map[string]any{"namespace": "acme", "base": "ACM 1100", "prove": []any{}}})
	})
	if r = calcolaIn(t, in); len(trova(r.Avvisi, AvvisoEquivalenteNonAdottato)) != 0 {
		t.Fatalf("il P/N e' il codice della working: %+v", trova(r.Avvisi, AvvisoEquivalenteNonAdottato))
	}
	// CORREZIONE_CARTIGLIO_NON_ADOTTATA: il cartiglio di un 2D del telaio propone un altro codice
	conCc := func(id idProva3, c *importazione.Contenuto) {
		aggiungiRiferimento(t, c, id.rif("#7"), map[string]any{"tipo": "documentale", "allegato_id": id.pdf.String(), "valore": "ACM-1101",
			"esito": "correzione_proposta", "correzione": map[string]any{"codice": "ACM-1101"}, "fonte": "cartiglio"})
	}
	in, id = ingressiProva3R4(t, conCc)
	r = calcolaIn(t, in)
	if a := trova(r.Avvisi, AvvisoCorrezioneCartiglioNonAdottat); len(a) != 1 || a[0].AllegatoID == nil || *a[0].AllegatoID != id.pdf || a[0].Valore != "ACM-1101" {
		t.Fatalf("CORREZIONE_CARTIGLIO_NON_ADOTTATA: %+v", a)
	}
	in, _ = ingressiProva3R4(t, conCc)
	in.snap.Decisioni = append(in.snap.Decisioni, Decisione3{ID: "di-0010", Tipo: DecisioneIdentita, ChiaveBozza: "k-0007", Modo: ModoCorrezione,
		Motivo: "il cartiglio e' vecchio"})
	if r = calcolaIn(t, in); len(trova(r.Avvisi, AvvisoCorrezioneCartiglioNonAdottat)) != 0 {
		t.Fatalf("con la decisione: %+v", trova(r.Avvisi, AvvisoCorrezioneCartiglioNonAdottat))
	}
	// PREASSOCIAZIONE_EMITTENTE_DIVERSO: il PDF del telaio e' letto dal partner con il codice del nodo
	in, _ = ingressiProva3R4(t, func(id idProva3, c *importazione.Contenuto) {
		l := letturaProva3(id.pdf, "l:u:pdf:1:id:beta-num/sei:0-6", "partner:beta", "beta", "ACM-1100", map[string]any{}, nil)
		l["codice_proposto"] = "ACM-1100"
		c.Proposta.File[0].Letture = rawDi(t, []any{l})
	})
	r = calcolaIn(t, in)
	if a := trova(r.Avvisi, AvvisoPreassociazioneEmittenteDiv); len(a) != 1 || a[0].ChiaveBozza != "k-0007" || a[0].Valore != "partner:beta" {
		t.Fatalf("PREASSOCIAZIONE_EMITTENTE_DIVERSO: %+v", a)
	}
	// NON_INCLUSO_SEGNALE_CAMBIATO: la non_includere ha visto il segnale di un'altra versione del servizio
	in, id = ingressiProva3R4(t, nil)
	for i := range in.snap.Decisioni {
		if in.snap.Decisioni[i].ID == "ni-0001" {
			in.snap.Decisioni[i].SegnaleVisto = &importazione.SegnaleVisto{Codice: "ancoraggio.possibile_elemento_cad", Regola: "elemento_cad_nome_foglia_v1",
				VersioneServizio: "ancoraggio-2"}
		}
	}
	r = calcolaIn(t, in)
	if a := trova(r.Avvisi, AvvisoNonInclusoSegnaleCambiato); len(a) != 1 || len(a[0].Chiavi) != 1 || a[0].Chiavi[0] != id.rif("#31") {
		t.Fatalf("NON_INCLUSO_SEGNALE_CAMBIATO: %+v", a)
	}
	var ni *EsclusoR4
	for i := range r.Esclusi {
		if r.Esclusi[i].Origine == DecisioneNonIncludere {
			ni = &r.Esclusi[i]
		}
	}
	if ni == nil || ni.SegnaleRegistro == nil || ni.SegnaleRegistro.VersioneServizio != "ancoraggio-3" || ni.SegnaleVisto.VersioneServizio != "ancoraggio-2" {
		t.Fatalf("segnale_registro e segnale_visto: %+v", ni)
	}
}

// EM §7.4, U2 §6.1: gli effetti dei non inclusi, le esclusioni legacy ed ereditate, la voce esclusione_legacy.
func TestR4EffettiEOriginiDegliEsclusi(t *testing.T) {
	persona := idDi("persona-di-allora")
	in, id := ingressiProva3R4(t, func(id idProva3, c *importazione.Contenuto) {
		for i := range c.Proposta.Nodi {
			if c.Proposta.Nodi[i].Origine.Rif == id.rif("#31") {
				c.Proposta.Nodi[i].FabbisogniPrevisti = rawDi(t, []any{map[string]any{"tipo_documento": "disegno_2d", "bloccante": true, "motivo": "pezzo"}})
			}
		}
	})
	riga := idDi("riga-31")
	in.righeNI[id.sha+"|#31"] = []db.ListRighePerShaChiaveRow{
		{PropostaID: riga, AllegatoID: id.allegato, Sha256: id.sha, Chiave: "#31", Stato: db.StatoPropostaAperta},
		{PropostaID: idDi("riga-31-decisa"), AllegatoID: id.allegato, Sha256: id.sha, Chiave: "#31", Stato: db.StatoPropostaConfermata,
			DecisoDa: uuid.NullUUID{UUID: persona, Valid: true}}}
	in.archiNI[id.sha+"|#31"] = []db.ListArchiPerShaChiaveRow{
		{AllegatoID: id.allegato, PadreChiave: "#7", FiglioChiave: "#31", Qta: 1, Stato: db.StatoPropostaAperta, Sha256: id.sha},
		{AllegatoID: id.allegato, PadreChiave: "#99", FiglioChiave: "#31", Qta: 1, Stato: db.StatoPropostaAperta, Sha256: id.sha},
		{AllegatoID: id.allegato, PadreChiave: "#98", FiglioChiave: "#31", Qta: 1, Stato: db.StatoPropostaScartata, Sha256: id.sha}}
	in.righeSha[id.sha] = []db.ListRigheStrutturaR4Row{
		{PropostaID: idDi("r-60"), AllegatoID: id.allegato, Sha256: id.sha, Chiave: "#60", Stato: db.StatoPropostaScartata,
			DecisoDa: uuid.NullUUID{UUID: persona, Valid: true}, Nota: pgtype.Text{String: "duplicato", Valid: true}},
		{PropostaID: idDi("r-61"), AllegatoID: id.allegato, Sha256: id.sha, Chiave: "#61", Stato: db.StatoPropostaScartata},
		{PropostaID: idDi("r-62"), AllegatoID: id.allegato, Sha256: id.sha, Chiave: "#62", Stato: db.StatoPropostaScartata,
			DecisoDa: uuid.NullUUID{UUID: persona, Valid: true}, Nota: pgtype.Text{String: NotaNonInclusoR5, Valid: true}},
		{PropostaID: idDi("r-7"), AllegatoID: id.allegato, Sha256: id.sha, Chiave: "#7", Stato: db.StatoPropostaScartata,
			DecisoDa: uuid.NullUUID{UUID: persona, Valid: true}}}
	in.nonInclusiBase = []db.ListNonInclusiConfermaRow{
		{DecisioneID: pgtype.Text{String: "ni-base", Valid: true}, Sha256: id.sha, Chiave: "#40", AllegatoID: id.allegato, Gruppo: gruppoProva3,
			Origine: DecisioneNonIncludere, MotivoCodice: pgtype.Text{String: "non_articolo", Valid: true}, Motivo: pgtype.Text{String: "etichetta", Valid: true}},
		{DecisioneID: pgtype.Text{String: "ni-base-2", Valid: true}, Sha256: id.sha, Chiave: "#31", AllegatoID: id.allegato, Gruppo: gruppoProva3,
			Origine: DecisioneNonIncludere}}
	r := calcolaIn(t, in)
	per := map[string][]EsclusoR4{}
	for _, e := range r.Esclusi {
		per[e.Origine] = append(per[e.Origine], e)
	}
	ni := per[DecisioneNonIncludere]
	if len(ni) != 1 || ni[0].Effetti == nil {
		t.Fatalf("non_includere: %+v", ni)
	}
	e := ni[0].Effetti
	if len(e.RigheDaChiudere) != 1 || e.RigheDaChiudere[0].PropostaID != riga || len(e.ArchiDaChiudere) != 2 || !e.ArchiDaChiudere[0].PadreNellaBozza ||
		e.ArchiDaChiudere[1].PadreNellaBozza || len(e.Figli) != 0 || len(e.FabbisogniTolti) != 1 || !strings.Contains(string(e.FabbisogniTolti[0]), "disegno_2d") {
		t.Fatalf("effetti del non incluso: %+v", e)
	}
	if lg := per[OrigineEsclusoLegacy]; len(lg) != 1 || lg[0].Casella.Rif != id.rif("#60") || lg[0].AutoreID == nil || *lg[0].AutoreID != persona ||
		lg[0].Motivo != "duplicato" {
		t.Fatalf("esclusioni legacy (solo la riga scartata da una persona, non inclusa e non di R5): %+v", lg)
	}
	if er := per[OrigineEsclusoEreditata]; len(er) != 1 || er[0].ID != "ni-base" || er[0].MotivoCodice != "non_articolo" {
		t.Fatalf("esclusioni ereditate (la #31 e' gia' una non_includere della bozza): %+v", er)
	}
	voci := 0
	for _, v := range r.RegistroDecisioni {
		if v.Tipo == VoceEsclusioneLegacy {
			voci++
			if v.AutoreID == nil || *v.AutoreID != persona {
				t.Fatalf("la voce esclusione_legacy porta la persona di allora: %+v", v)
			}
		}
	}
	if voci != 1 {
		t.Fatalf("voci esclusione_legacy: %d", voci)
	}
}

// U2 §5.4 (c), (e) sullo stato dopo R5: le righe dello STEP sorgente che R5 non segna, le rimozioni aperte, gli STEP
// secondari autorizzati.
func TestR4PrimaIstanzaCondizioniCeE(t *testing.T) {
	in, id := ingressiProva3R4(t, func(id idProva3, c *importazione.Contenuto) {
		vero, falso := true, false
		v := c.Proposta.Nodi[2]
		v.Voce, v.Origine.Chiave, v.Origine.Rif, v.EntitaID = id.voce("#50"), "#50", id.rif("#50"), rawDi(t, "e#50")
		v.Padri = rawDi(t, []any{id.rif("#7")})
		v.StatoMotore = importazione.StatoMotore{Raggiunto: &vero, Tolto: &falso, Deciso: &falso}
		v.Grezzi = rawDi(t, map[string]any{"id": "#50", "nome": "pezzo #50", "formazioni": []any{}})
		v.Diagnostiche = nil
		c.Proposta.Nodi = append(c.Proposta.Nodi, v)
		q := 1
		c.Proposta.Archi = append(c.Proposta.Archi, importazione.ArcoRegistro{AllegatoID: id.allegato.String(), Padre: id.rif("#7"), Figlio: id.rif("#50"), Quantita: &q})
	})
	riga := func(ch string, stato db.StatoProposta, persona bool) db.ListRigheStrutturaR4Row {
		r := db.ListRigheStrutturaR4Row{PropostaID: idDi("r" + ch), AllegatoID: id.allegato, Sha256: id.sha, Chiave: ch, Stato: stato}
		if persona {
			r.DecisoDa = uuid.NullUUID{UUID: idDi("persona"), Valid: true}
		}
		return r
	}
	in.righeSha[id.sha] = []db.ListRigheStrutturaR4Row{
		riga("#1", db.StatoPropostaAperta, false),     // la radice: fuori
		riga("#7", db.StatoPropostaAperta, false),     // nodo incluso: lo segna R5
		riga("#31", db.StatoPropostaAperta, false),    // non_includere: la chiude R5
		riga("#12", db.StatoPropostaAperta, false),    // fuori per decisione, non raggiunto: non conta
		riga("#50", db.StatoPropostaDuplicato, false), // raggiunto, assente dalla bozza: la casella conta come segnata da R5 (assenza, U2 §6.1)
	}
	righe, rim, sec := nuovaAnalisi(in).contiPrimaIstanza()
	if righe != 0 || rim != 0 || sec != 0 {
		t.Fatalf("(c) righe %d rimozioni %d (e) %d", righe, rim, sec)
	}
	// senza registro contano tutte le righe che R5 non segna: #12 e #50 (decise da un automatismo o aperte)
	reg := in.registro
	in.registro = nil
	if righe, _, _ = nuovaAnalisi(in).contiPrimaIstanza(); righe != 2 {
		t.Fatalf("(c) senza registro: righe %d (attese 2)", righe)
	}
	in.righeSha[id.sha][4] = riga("#50", db.StatoPropostaConfermata, true)
	if righe, _, _ = nuovaAnalisi(in).contiPrimaIstanza(); righe != 1 {
		t.Fatalf("una riga decisa da una persona non conta: %d", righe)
	}
	in.registro = reg
	// senza una sorgente valida non si conta niente (la verifica e' gia' «non calcolata»)
	in.sorgente.Valida = false
	if a, b, c := nuovaAnalisi(in).contiPrimaIstanza(); a+b+c != 0 {
		t.Fatalf("senza sorgente: %d %d %d", a, b, c)
	}
	// rimozioni e STEP secondari sui componenti della bozza (con la fixture dei registrati)
	f := nuovaFixR4(t)
	sa := f.componente("sa", "ACM-2000", "sottoassieme")
	pz := f.componente("pz", "ACM-2001", "sciolto")
	altro := f.componente("fuori", "ACM-2999", "sciolto")
	f.arcoW(f.prodotto, sa, 1)
	f.arcoW(sa, pz, 1)
	f.nodoReg("k-sa", sa, "sottoassieme")
	f.nodoReg("k-pz", pz, "sciolto")
	f.arco("k-root", "k-sa", 1)
	f.arco("k-sa", "k-pz", 1)
	f.sorgenteValida(shaProva('e'))
	shaSec := shaProva('f')
	r := f.calcolaCon(func(in *ingressiR4) {
		in.rimozioni = []db.RimozioneProposta{
			{StepDocumentoID: idDi("step-b"), PadreID: sa, FiglioID: pz, Stato: db.StatoPropostaAperta},
			{StepDocumentoID: idDi("step-c"), PadreID: sa, FiglioID: pz, Stato: db.StatoPropostaAperta},
			{StepDocumentoID: idDi("step-b"), PadreID: sa, FiglioID: altro, Stato: db.StatoPropostaAperta}, // il figlio non e' nella bozza
		}
		in.secondari = []stepSecondario{{componente: sa, sha256: shaSec}, {componente: altro, sha256: shaProva('9')}}
		in.righeSha[shaSec] = []db.ListRigheStrutturaR4Row{{Chiave: "#2", Stato: db.StatoPropostaAperta}, {Chiave: "#3", Stato: db.StatoPropostaAperta},
			{Chiave: "#4", Stato: db.StatoPropostaConfermata}}
		in.righeSha[shaProva('9')] = []db.ListRigheStrutturaR4Row{{Chiave: "#2", Stato: db.StatoPropostaAperta}}
		if _, rim, sec := nuovaAnalisi(in).contiPrimaIstanza(); rim != 2 || sec != 2 {
			t.Fatalf("rimozioni %d (attese 2), righe secondarie %d (attese 2)", rim, sec)
		}
	})
	if v := r.VerificaPrevista; v.Stato != confermabom.StatoVerificaCalcolata || v.Verificata == nil || *v.Verificata ||
		!contiene(v.Motivi, confermabom.MotivoRimozioniAperte) || !contiene(v.Motivi, confermabom.MotivoSecondariAperti) {
		t.Fatalf("verifica prevista con (c) ed (e): %+v (blocchi %v)", v, codici(r.Blocchi))
	}
}

func contiene(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
