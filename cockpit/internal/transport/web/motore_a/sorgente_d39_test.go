package motore_a

// L1 — direttiva 39, corsia motore: le regole nuove di S9 sulla scena di C-4 (sorgente_c4_test.go).
//   - D-AR11 B [U]: senza la sorgente confermata da S8 nessuna riga proposto e nessun «salva nella bozza» dal motore; il
//     file resta nella chip «attende la sorgente» con il motivo; le PB manuali e le AR restano; con S8 le proposte ci sono;
//   - lo STEP sorgente: riga legato_come_sorgente (conferma di S8) in sola lettura, oppure legame_storico (D-AR04 (iii),
//     senza conferme di S8), distinti; una PB sullo STEP sorgente è in conflitto;
//   - D-PB02 A [U]: i suggerimenti dopo l'apertura in preassociazioni_proposte[] sono quelli di R1 (una sola sorgente e una
//     sola forma, [T-39]), mai dal calcolo di adesso; le proposte ambigue hanno il motivo proposta_ambigua sulle righe;
//   - la provenienza motore delle proposte, quella dello snapshot per le PB, e l'ordine delle anteprime (priorita_anteprima 1, 2, 3);
//   - nessuna lettura di documento_proposta.componente_id come fonte (la copia senza AR resta senza righe).
// Tutto è inventato.

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/valutazione"
)

func TestV1D39S9SenzaS8NessunaPropostaDelMotore(t *testing.T) {
	p1, p2 := maUUID(10), maUUID(20)
	r := risultatoC4()
	r.V1.SorgentiS8 = nil
	s := ComponiSorgente(r, maUUID(1))
	for _, p := range []uuid.UUID{p1, p2} {
		sez := sezioneDi(t, s, p)
		for _, x := range sez.Righe {
			if x.Origine == OrigineRigaMotore || x.Stato == StatoRigaProposto || x.Azioni.SalvaNellaBozza.Disponibile {
				t.Errorf("prodotto %s senza S8: una riga del motore o un «salva nella bozza» %+v", p, x)
			}
		}
		if sez.Conti.Proposti != 0 || len(sez.PreassociazioniProposte) != 0 {
			t.Errorf("prodotto %s senza S8: proposti %d, suggerimenti %+v", p, sez.Conti.Proposti, sez.PreassociazioniProposte)
		}
		trovato := false
		for _, c := range sez.InAttesaSorgente {
			if c.AllegatoID == c4Proposto && c.Motivo == MotivoAttesaNonConfermata {
				trovato = true
			}
		}
		if !trovato {
			t.Errorf("prodotto %s senza S8: il file proposto non è nella chip «attende la sorgente»: %+v", p, sez.InAttesaSorgente)
		}
	}
	// le PB manuali e le AR restano
	unaRiga(t, s, p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza)
	unaRiga(t, s, p1, c4Condiviso, StatoRigaRegistrato, "", OrigineRigaWorking)
	// il file proposto non è fra quelli da associare (è nella chip, visibile)
	for _, d := range s.DaAssociare {
		if d.AllegatoID == c4Proposto {
			t.Errorf("il file proposto è anche da associare: %+v", d)
		}
	}
	// con S8 (la scena di C-4) la proposta c'è, con la provenienza e la priorità del candidato del motore; la PB porta la
	// provenienza dello snapshot (D-PB03 A [U]: la lettura delle bozze la dà)
	r = risultatoC4()
	for i := range r.V1.LetturaBozze.Bozze[0].Preassociazioni {
		if pb := &r.V1.LetturaBozze.Bozze[0].Preassociazioni[i]; pb.AllegatoID == c4Salvato {
			pb.Provenienza, pb.Importazione = "corretta", "impronta-del-registro"
		}
	}
	s = ComponiSorgente(r, maUUID(1))
	x := unaRiga(t, s, p1, c4Proposto, StatoRigaProposto, "", OrigineRigaMotore)
	if x.Provenienza != ProvenienzaMotore || x.PrioritaAnteprima != PrioritaCandidatoMotore || !x.Azioni.SalvaNellaBozza.Disponibile {
		t.Errorf("la proposta con S8: %+v", x)
	}
	// l'ordine delle anteprime: documento ufficiale 1, PB salvata 2
	if y := unaRiga(t, s, p1, c4Condiviso, StatoRigaRegistrato, "", OrigineRigaWorking); y.PrioritaAnteprima != PrioritaDocumentoUfficiale {
		t.Errorf("la AR: priorità %d", y.PrioritaAnteprima)
	}
	if y := unaRiga(t, s, p1, c4Documento, StatoRigaRegistrato, MotivoRigaDocumentoConfermato, OrigineRigaWorking); y.PrioritaAnteprima != PrioritaDocumentoUfficiale {
		t.Errorf("il documento confermato: priorità %d", y.PrioritaAnteprima)
	}
	if y := unaRiga(t, s, p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza); y.PrioritaAnteprima != PrioritaNellaBozza || y.Provenienza != "corretta" {
		t.Errorf("la PB: priorità %d, provenienza %q (attesa quella dello snapshot)", y.PrioritaAnteprima, y.Provenienza)
	}
	// una PB salvata prima che il server scrivesse la provenienza: vuota, mai inventata
	if y := unaRiga(t, ComponiSorgente(risultatoC4(), maUUID(1)), p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza); y.Provenienza != "" {
		t.Errorf("la PB senza provenienza nello snapshot: %q", y.Provenienza)
	}
	// la copia senza AR non fa righe (mai documento_proposta.componente_id come fonte)
	if rr := righeDi(s, p1, c4Copia); len(rr) != 0 {
		t.Errorf("la copia senza AR ha righe: %+v", rr)
	}
}

func TestV1D39S9LoSTEPSorgenteInSolaLettura(t *testing.T) {
	p1, p2 := maUUID(10), maUUID(20)
	s := ComponiSorgente(risultatoC4(), maUUID(1))
	x := unaRiga(t, s, p1, c4Step, StatoRigaLegatoComeSorgente, MotivoRigaConfermaS8, OrigineRigaSorgente)
	if x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != p1 || x.PrioritaAnteprima != PrioritaDocumentoUfficiale {
		t.Errorf("lo STEP legato come sorgente: %+v", x)
	}
	for i, a := range []AzioneSorgente{x.Azioni.SalvaNellaBozza, x.Azioni.TogliDallaBozza, x.Azioni.Sposta, x.Azioni.ScollegaAllaConferma} {
		if a.Disponibile || a.Motivo != MotivoAzioneSorgente {
			t.Errorf("lo STEP sorgente, azione %d: %+v", i, a)
		}
	}
	if sez := sezioneDi(t, s, p1); sez.Conti.LegatiComeSorgente != 1 || sez.Conti.LegamiStorici != 0 || !sez.Fonte.SorgenteConfermata && sez.Fonte.SorgenteS8 == nil {
		t.Errorf("i conti e la fonte del primo prodotto: %+v %+v", sez.Conti, sez.Fonte)
	}
	if sez := sezioneDi(t, s, p2); sez.Conti.LegatiComeSorgente != 1 {
		t.Errorf("il secondo prodotto ha la sua sorgente: %+v", sez.Conti)
	}

	// senza S8: lo STEP storico univoco del primo prodotto (il documento 41 dell'allegato 31) è un legame storico, distinto
	r := risultatoC4()
	r.V1.SorgentiS8 = nil
	s = ComponiSorgente(r, maUUID(1))
	x = unaRiga(t, s, p1, c4Step, StatoRigaLegameStorico, MotivoRigaLegameStorico, OrigineRigaSorgente)
	if x.Azioni.SalvaNellaBozza.Disponibile || x.Azioni.Sposta.Disponibile || x.Azioni.Sposta.Motivo != MotivoAzioneLegameStorico || x.PrioritaAnteprima != 0 {
		t.Errorf("il legame storico: %+v", x)
	}
	if sez := sezioneDi(t, s, p1); sez.Conti.LegamiStorici != 1 || sez.Conti.LegatiComeSorgente != 0 || sez.Fonte.LegameStorico == nil || sez.Fonte.SorgenteConfermata {
		t.Errorf("i conti e la fonte senza S8: %+v %+v", sez.Conti, sez.Fonte)
	}

	// una S8 revocata: niente riga legata, niente legame storico (il prodotto ha eventi di S8), niente proposte
	r = risultatoC4()
	v := r.V1.SorgentiS8[p1]
	v.Stato.Valida, v.Stato.Motivo = false, "revocata"
	r.V1.SorgentiS8[p1] = v
	s = ComponiSorgente(r, maUUID(1))
	if rr := righeDi(s, p1, c4Step); len(rr) != 0 {
		t.Errorf("una sorgente revocata ha righe: %+v", rr)
	}
	if rr := righeDi(s, p1, c4Proposto); len(rr) != 0 {
		t.Errorf("con la sorgente revocata il motore propone: %+v", rr)
	}

	// una PB sullo STEP sorgente: in conflitto, con il motivo
	r = risultatoC4()
	b := &r.V1.LetturaBozze.Bozze[0]
	b.Preassociazioni = append(b.Preassociazioni, b.Preassociazioni[0])
	b.Preassociazioni[len(b.Preassociazioni)-1].AllegatoID = c4Step
	s = ComponiSorgente(r, maUUID(1))
	rr := righeDi(s, p1, c4Step)
	if len(rr) != 2 || rr[0].Stato != StatoRigaLegatoComeSorgente || rr[1].Stato != StatoRigaInConflitto || rr[1].Motivo != MotivoRigaLegatoComeSorgente {
		t.Errorf("la PB sullo STEP sorgente: %+v", rr)
	}
}

func TestV1D39S9ISuggerimentiDopoLApertura(t *testing.T) {
	p1, p2 := maUUID(10), maUUID(20)
	comp := func(nn int) string { return "componente/" + maComp(nn) }
	ambiguo := maUUID(720)
	r := risultatoC4()
	r.EsitoThread.Associazioni = append(r.EsitoThread.Associazioni,
		// la PB del primo è su un nodo nuovo; il motore di adesso propone 7120013
		valutazione.AssociazioneFile{AllegatoID: c4Salvato, Proposta: []string{comp(13)}, Pertinente: []string{maP1}},
		// la AR su un altro componente; il motore di adesso propone 7120013 nel primo
		valutazione.AssociazioneFile{AllegatoID: c4Altrove, Proposta: []string{comp(13)}, Pertinente: []string{maP1}},
		// due destinazioni: ambiguo
		valutazione.AssociazioneFile{AllegatoID: ambiguo, Proposta: []string{comp(13), comp(14)}, Pertinente: []string{maP1}},
	)
	// senza i suggerimenti di R1 nessun suggerimento: le proposte del calcolo di adesso restano righe (una sola sorgente)
	s := ComponiSorgente(r, maUUID(1))
	sez := sezioneDi(t, s, p1)
	if len(sez.PreassociazioniProposte) != 0 {
		t.Errorf("suggerimenti dal calcolo di adesso: %+v", sez.PreassociazioniProposte)
	}
	unaRiga(t, s, p1, c4Proposto, StatoRigaProposto, "", OrigineRigaMotore)
	rr := righeDi(s, p1, ambiguo)
	if len(rr) != 2 || rr[0].Motivo != MotivoRigaPropostaAmbigua || rr[1].Motivo != MotivoRigaPropostaAmbigua {
		t.Errorf("le righe della proposta ambigua: %+v", rr)
	}
	// i suggerimenti di R1 (la lettura del caricatore) passano tali e quali, con la chiave_bozza e il registro
	r1 := []distintabozza.PreassociazioneProposta3{
		{AllegatoID: c4Proposto, ChiaveBozza: "k-13", Provenienza: distintabozza.ProvenienzaMotore, Importazione: "impronta-a", Motivo: MotivoSuggerimentoApertura},
		{AllegatoID: c4Salvato, ChiaveBozza: "k-13", Provenienza: distintabozza.ProvenienzaMotore, Importazione: "impronta-a", Motivo: MotivoSuggerimentoPBToccata},
	}
	r.V1.Suggerimenti = map[uuid.UUID][]distintabozza.PreassociazioneProposta3{p1: r1, p2: r1}
	s = ComponiSorgente(r, maUUID(1))
	if sez := sezioneDi(t, s, p1); !reflect.DeepEqual(sez.PreassociazioniProposte, r1) {
		t.Errorf("i suggerimenti di R1 in S9: %+v", sez.PreassociazioniProposte)
	}
	// la PB resta com'era: il suggerimento non la tocca
	unaRiga(t, s, p1, c4Salvato, StatoRigaSalvato, "", OrigineRigaBozza)
	// i motivi sono quelli di R1
	if MotivoSuggerimentoApertura != distintabozza.PropostaDopoApertura || MotivoSuggerimentoPBToccata != distintabozza.PropostaPBToccata ||
		MotivoSuggerimentoARAttiva != distintabozza.PropostaARAttiva || MotivoSuggerimentoConflitto != distintabozza.PropostaConflitto {
		t.Error("i motivi dei suggerimenti di S9 non sono quelli di R1")
	}

	// senza la bozza nessun suggerimento (il secondo prodotto senza bozza), anche se la lettura ne avesse
	r.V1.LetturaBozze.Bozze = r.V1.LetturaBozze.Bozze[:1]
	s = ComponiSorgente(r, maUUID(1))
	if sez := sezioneDi(t, s, p2); len(sez.PreassociazioniProposte) != 0 {
		t.Errorf("senza bozza: %+v", sez.PreassociazioniProposte)
	}
}
