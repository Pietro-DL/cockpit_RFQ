package motore_a

// L1 — giro G39, corsia motore (G39-02 [U]; aperto (b) del verificatore del 20/10): lo STEP candidato del prodotto finito,
// letto con il Motore A, sulla scena di C-4 (sorgente_c4_test.go) con uno STEP in più che l'ancoraggio candida al primo
// prodotto e che lo smistamento proporrebbe su un componente condiviso dai due prodotti:
//   - in S9 non è mai una riga proposto (né in_conflitto del motore) in nessuna sezione, non è in da_associare né nella
//     chip «attende la sorgente»;
//   - nella fonte del primo prodotto (S9 e /dati) è una proposta di sorgente, con le evidenze della lettura e il testo; il
//     secondo prodotto non la ha;
//   - una PB della bozza su quello STEP è in conflitto (step_candidato_del_finito);
//   - già sorgente valida di S8 del primo prodotto: non si propone più; sorgente valida di S8 del secondo: la proposta
//     dice sorgente_di.
// Tutto è inventato.

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/valutazione"
)

var g39Step = maUUID(77)

// risultatoG39: la scena di C-4 con lo STEP candidato del primo prodotto (letto dal nome del file).
func risultatoG39() *Risultato {
	r := risultatoC4()
	stp := "stp"
	r.Thread.Allegati = append(r.Thread.Allegati, fotorfq.Allegato{ID: g39Step, MessaggioID: maUUID(2), NomeFile: "9110001.stp", Estensione: &stp})
	r.EsitoThread.Associazioni = append(r.EsitoThread.Associazioni,
		valutazione.AssociazioneFile{AllegatoID: g39Step, Proposta: []string{"componente/" + maComp(14)}, Pertinente: []string{maP1, maP2}})
	r.EsitoThread.DaSmistare = append(r.EsitoThread.DaSmistare, valutazione.FileDaSmistare{AllegatoID: g39Step, NomeFile: "9110001.stp"})
	r.EsitoThread.File = append(r.EsitoThread.File, valutazione.FileInterpretato{AllegatoID: g39Step, Interpretazione: motorea.Interpretazione{
		Letture: []motorea.LetturaCodice{{ID: "l-nome", Forma: motorea.LetturaForma{Selettore: evidenze.Selettore{Contesto: evidenze.ContestoNomeFile},
			Originale: "9110001", Base: motorea.BaseLetta{Normalizzata: "9110001"}}}}}})
	r.EsitoThread.Ancoraggi.File = append(r.EsitoThread.Ancoraggi.File, ancoraggio.AncoraggioFile{AllegatoID: g39Step,
		Associazione: ancoraggio.AssociazioneCandidatoUnico, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: maP1, Livello: ancoraggio.LivelloProdotto,
			Letture: []string{"l-nome"}, Dimensioni: []ancoraggio.DimensioneCompatibilita{
				{Dimensione: ancoraggio.DimensioneBase, Esito: motorea.CompatibilitaUguale, Lettura: "l-nome", Rif: maP1},
				{Dimensione: ancoraggio.DimensioneMarcatore, Esito: motorea.CompatibilitaParziale, Lettura: "l-nome", Rif: maP1}}}}})
	return r
}

func TestG39S9LoSTEPCandidatoDelFinitoEUnaPropostaDiSorgente(t *testing.T) {
	p1, p2 := maUUID(10), maUUID(20)
	// il primo prodotto prima di S8 (le proposte di sorgente sono di prima di S8); il secondo con la sua sorgente valida
	nuovo := func() *Risultato {
		r := risultatoG39()
		delete(r.V1.SorgentiS8, p1)
		return r
	}
	propostaDi := func(pp []PropostaSorgente, a uuid.UUID) *PropostaSorgente {
		for i := range pp {
			if pp[i].AllegatoID == a {
				return &pp[i]
			}
		}
		return nil
	}
	r := nuovo()
	s := ComponiSorgente(r, maUUID(1))
	for _, p := range []uuid.UUID{p1, p2} {
		if rr := righeDi(s, p, g39Step); len(rr) != 0 {
			t.Errorf("lo STEP candidato ha righe nel prodotto %s: %+v", p, rr)
		}
		for _, c := range sezioneDi(t, s, p).InAttesaSorgente {
			if c.AllegatoID == g39Step {
				t.Errorf("lo STEP candidato è nella chip del prodotto %s", p)
			}
		}
	}
	for _, d := range s.DaAssociare {
		if d.AllegatoID == g39Step {
			t.Errorf("lo STEP candidato univoco è da associare: %+v", d)
		}
	}
	codice := ""
	for _, c := range r.Thread.Componenti {
		if c.ID == p1 {
			codice = c.Codice
		}
	}
	x := propostaDi(sezioneDi(t, s, p1).Fonte.ProposteSorgente, g39Step)
	if x == nil || x.NomeFile != "9110001.stp" || x.CodiceProdotto != codice || x.SorgenteDi != nil ||
		!strings.Contains(x.Testo, "molto probabilmente lo STEP strutturale di "+codice) || len(x.Evidenze) != 1 ||
		x.Evidenze[0].Selettore != "nome_file" || x.Evidenze[0].CodiceLetto != "9110001" || x.Evidenze[0].EsitoMarcatore != "compatibile_parziale" {
		t.Errorf("la proposta di sorgente del primo prodotto: %+v (tutte: %+v)", x, sezioneDi(t, s, p1).Fonte.ProposteSorgente)
	}
	// il secondo prodotto ha la sua sorgente valida di S8: nessuna proposta
	if pp := sezioneDi(t, s, p2).Fonte.ProposteSorgente; len(pp) != 0 {
		t.Errorf("il secondo prodotto, con la sorgente confermata, ha proposte di sorgente: %+v", pp)
	}
	// /dati: la stessa proposta nella fonte della pagina del prodotto
	if f := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Prodotto: maP1}).Fonte; f == nil || propostaDi(f.ProposteSorgente, g39Step) == nil {
		t.Errorf("/dati.fonte: %+v", f)
	}

	// una PB della bozza del primo prodotto sullo STEP candidato: in conflitto, con il motivo
	r = nuovo()
	b := &r.V1.LetturaBozze.Bozze[0]
	b.Preassociazioni = append(b.Preassociazioni, distintabozza.PreassociazioneDellaBozza{AllegatoID: g39Step, ChiaveBozza: "k-a",
		Nodo: distintabozza.NodoDellaBozza{ChiaveBozza: "k-a", Codice: "7120050", Rev: "A"}})
	unaRiga(t, ComponiSorgente(r, maUUID(1)), p1, g39Step, StatoRigaInConflitto, MotivoRigaStepCandidatoDelFinito, OrigineRigaBozza)

	// confermato con S8 come sorgente del primo prodotto: nessuna proposta
	r = nuovo()
	r.V1.SorgentiS8[p1] = SorgenteS8{AllegatoID: g39Step, Sha256: maSha(77), Stato: confermabom.StatoSorgente{Valida: true, Documento: maUUID(78)}}
	if pp := sezioneDi(t, ComponiSorgente(r, maUUID(1)), p1).Fonte.ProposteSorgente; len(pp) != 0 {
		t.Errorf("con la sorgente confermata il prodotto ha ancora proposte: %+v", pp)
	}
	// la sorgente valida di S8 del secondo prodotto: la proposta dice sorgente_di (G39-03)
	r = nuovo()
	r.V1.SorgentiS8[p2] = SorgenteS8{AllegatoID: g39Step, Sha256: maSha(77), Stato: confermabom.StatoSorgente{Valida: true, Documento: maUUID(78)}}
	if x := propostaDi(sezioneDi(t, ComponiSorgente(r, maUUID(1)), p1).Fonte.ProposteSorgente, g39Step); x == nil || x.SorgenteDi == nil || *x.SorgenteDi != p2 {
		t.Errorf("lo STEP sorgente di un altro prodotto: %+v", x)
	}
}

// L1 — giro G39-bis (21/10), corsia motore, S9 sulla scena di risultatoG39 (lo STEP «9110001.stp» ancorato al primo
// prodotto con la sola base):
//   - un altro componente della RFQ con il codice 9110001 (il nome del file): evidenza discordante (D-2 (c) [U]), lo STEP
//     non è una proposta di sorgente e la PB su di lui non è in conflitto step_candidato_del_finito;
//   - lo STEP anche nella fonte del secondo prodotto (trovato dal motore): legato a due prodotti finiti, le due
//     candidature non sono univoche (un altro finito non è un'evidenza discordante: revisione indipendente del giro
//     G39-bis); niente proposta, niente righe proposto del motore. La PB su di lui verso una parte (il nodo nuovo k-a)
//     non è in conflitto: è la scelta dell'utente (G39-06 [U], giro G39-ter, come R2 che la salva e R4 che non la
//     blocca); verso un nodo di tipo finito è in conflitto step_candidato_del_finito (G39-02 [U]: la strada è S8).
//
// Tutto è inventato.
func TestG39bisS9EvidenzaDiscordanteENonUnivoca(t *testing.T) {
	p1 := maUUID(10)
	tipoNodo := ""
	conPB := func(r *Risultato) *Risultato {
		delete(r.V1.SorgentiS8, p1)
		b := &r.V1.LetturaBozze.Bozze[0]
		b.Preassociazioni = append(b.Preassociazioni, distintabozza.PreassociazioneDellaBozza{AllegatoID: g39Step, ChiaveBozza: "k-a",
			Nodo: distintabozza.NodoDellaBozza{ChiaveBozza: "k-a", Codice: "7120050", Rev: "A", Tipo: tipoNodo}})
		return r
	}
	controlla := func(nome string, r *Risultato, conflitto bool) {
		t.Helper()
		for _, c := range r.CandidatureSorgente() {
			if c.AllegatoID == g39Step && c.Univoca {
				t.Errorf("%s: lo STEP è una candidatura univoca: %+v", nome, c)
			}
		}
		s := ComponiSorgente(r, maUUID(1))
		for _, x := range sezioneDi(t, s, p1).Fonte.ProposteSorgente {
			if x.AllegatoID == g39Step {
				t.Errorf("%s: lo STEP è una proposta di sorgente: %+v", nome, x)
			}
		}
		inConflitto := false
		for _, rr := range righeDi(s, p1, g39Step) {
			if rr.Motivo == MotivoRigaStepCandidatoDelFinito {
				inConflitto = rr.Stato == StatoRigaInConflitto && rr.Origine == OrigineRigaBozza
			}
		}
		if inConflitto != conflitto {
			t.Errorf("%s: la PB sullo STEP in conflitto step_candidato_del_finito: %v, atteso %v (%+v)", nome, inConflitto, conflitto, righeDi(s, p1, g39Step))
		}
	}
	r := conPB(risultatoG39())
	r.Thread.Componenti = append(r.Thread.Componenti, fotorfq.Componente{ID: maUUID(99), Codice: "9110001", Tipo: "sottoassieme"})
	controlla("il nome è il codice di un altro componente", r, false)

	dueProdotti := func() *Risultato {
		r := conPB(risultatoG39())
		for i := range r.EsitoThread.ProdottiValutati {
			if r.EsitoThread.ProdottiValutati[i].Rif == maP2 {
				pv := &r.EsitoThread.ProdottiValutati[i]
				pv.Fonte.Candidati = append(pv.Fonte.Candidati, ancoraggio.DocumentoCandidato{Origine: ancoraggio.CandidatoDaMotoreA, AllegatoID: &g39Step})
			}
		}
		return r
	}
	tipoNodo = "finito"
	controlla("legato a due prodotti, la PB verso un nodo finito", dueProdotti(), true)
	tipoNodo = "sciolto"
	r = dueProdotti()
	controlla("legato a due prodotti, la PB verso una parte", r, false)
	cc := r.CandidatureSorgente()
	n := 0
	for _, c := range cc {
		if c.AllegatoID == g39Step {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("legato a due prodotti: le candidature dello STEP (attese le due non univoche): %+v", cc)
	}
	for _, c := range cc {
		if c.AllegatoID == g39Step {
			if rr := righeDi(ComponiSorgente(r, maUUID(1)), p1, g39Step); len(rr) != 1 || rr[0].Origine != OrigineRigaBozza {
				t.Errorf("legato a due prodotti: le righe dello STEP (solo la PB, nessuna riga del motore): %+v", rr)
			}
		}
	}
}

// L1 — giro G39, G39-01: SOLO LA FORMA (rilievo della revisione del giro G39: l'impronta qui è finta e R2 non la
// riconoscerebbe). La composizione pura di S9 passa i suggerimenti che riceve nella forma di R1, tale e quale
// (PreassociazioneProposta3: allegato_id, chiave_bozza, provenienza motore, importazione, motivo), anche quando
// l'importazione non è il registro della versione, e la riga proposto dello stesso file resta. G39-01 sulla strada vera
// (il file arrivato dopo l'apertura nei suggerimenti di S9 e di R1 con il calcolo di adesso, R2 «motore» e «corretta») è
// la L4 TestL4V1G39_01FileDopoLAperturaS9R1R2 di transport/web.
func TestG39S9IlSuggerimentoDiUnFileArrivatoDopoLApertura(t *testing.T) {
	p1 := maUUID(10)
	r := risultatoC4()
	dopo := []distintabozza.PreassociazioneProposta3{{AllegatoID: c4Proposto, ChiaveBozza: "k-14", Provenienza: distintabozza.ProvenienzaMotore,
		Importazione: "impronta-del-calcolo-di-adesso", Motivo: distintabozza.PropostaDopoApertura}}
	r.V1.Suggerimenti = map[uuid.UUID][]distintabozza.PreassociazioneProposta3{p1: dopo}
	s := ComponiSorgente(r, maUUID(1))
	sez := sezioneDi(t, s, p1)
	if len(sez.PreassociazioniProposte) != 1 || sez.PreassociazioniProposte[0] != dopo[0] {
		t.Errorf("il suggerimento di R1 per il file arrivato dopo l'apertura: %+v", sez.PreassociazioniProposte)
	}
	if x := unaRiga(t, s, p1, c4Proposto, StatoRigaProposto, "", OrigineRigaMotore); !x.Azioni.SalvaNellaBozza.Disponibile {
		t.Errorf("la riga proposto del file: %+v", x)
	}
}
