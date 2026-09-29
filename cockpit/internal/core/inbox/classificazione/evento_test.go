package classificazione

import (
	"slices"
	"strings"
	"testing"
)

// L1 — Smistamento M2 (A5.16.2): l'evento della mail, separato dall'aggancio. Codici e nomi finti
// (7120001A, ACME, @acme.example, Fornitore Esempio).

// dalCliente è una mail in entrata da un buyer di ACME.
func dalCliente(oggetto, corpo string, allegati ...string) IngressoTriage {
	return IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "buyer@acme.example", Oggetto: oggetto, Corpo: corpo, NomiAllegati: allegati}
}

// 244 — l'evento non dipende dai candidati. Lo stesso messaggio con un R0 a 98 verso una RFQ aperta e
// senza nessun candidato ha lo stesso evento, la stessa forza, lo stesso atto e le stesse evidenze;
// cambia l'esito, che è la risposta all'altra domanda. Evento() non riceve nemmeno i candidati: qui si
// prova che Triage non li fa entrare dalla porta di servizio.
func TestLEventoNonDipendeDaiCandidati(t *testing.T) {
	r0 := []Candidato{NuovoCandidato("t-aperta", TipoR0InReplyTo, "In-Reply-To verso la richiesta", false)}
	casi := []struct {
		nome   string
		in     IngressoTriage
		evento string
	}{
		{"revisione dei CAD in una risposta", dalCliente("R: RFQ 7120001", "Vi mandiamo il modello aggiornato.", "7120001A_2.stp"), EventoRevisioneCAD},
		{"arrivo dei CAD in una risposta", dalCliente("R: RFQ 7120001", "Ecco i disegni.", "7120001A_1.stp"), EventoArrivoCAD},
		// la separazione chiesta dall'utente: una risposta che nel testo nuovo chiede un'offerta con uno
		// STEP è una richiesta nuova anche con un R0 a 98 verso una RFQ vecchia
		{"una richiesta nuova dentro una risposta", dalCliente("R: RFQ 7120001", "Vi chiediamo una quotazione anche per questo particolare.", "7120002A.stp"), EventoNuovaRFQ},
		{"sollecito", dalCliente("R: RFQ 7120001", "Siamo in attesa di un vostro riscontro."), EventoSollecito},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			senza := Triage(c.in)
			con := c.in
			con.Candidati = r0
			conR0 := Triage(con)
			if senza.Evento != c.evento || conR0.Evento != c.evento {
				t.Fatalf("evento senza candidati %s, con R0 %s: atteso %s (%v)", senza.Evento, conR0.Evento, c.evento, senza.EvidenzeEvento)
			}
			if senza.Atto != conR0.Atto || senza.ForzaEvento != conR0.ForzaEvento || !slices.Equal(senza.EvidenzeEvento, conR0.EvidenzeEvento) {
				t.Errorf("l'atto o le evidenze cambiano con i candidati: %s/%s %v contro %s/%s %v",
					senza.Atto, senza.ForzaEvento, senza.EvidenzeEvento, conR0.Atto, conR0.ForzaEvento, conR0.EvidenzeEvento)
			}
			// l'altra domanda sì: con R0 la risposta è «aggancia», senza no
			if conR0.Esito != "aggancia" || senza.Esito == "aggancia" {
				t.Errorf("esito senza candidati %s, con R0 %s: l'aggancio lo decidono le evidenze, non l'evento", senza.Esito, conR0.Esito)
			}
			// e il candidato proposto è quello di R0, qualunque sia l'evento
			if conR0.Candidato == nil || conR0.Candidato.ThreadID != "t-aperta" {
				t.Errorf("proposto %+v", conR0.Candidato)
			}
			// la funzione pura dice lo stesso di Triage
			if ev := Evento(c.in.ingressoEvento()); ev.Evento != senza.Evento || ev.Atto != senza.Atto {
				t.Errorf("Evento %s/%s, Triage %s/%s", ev.Evento, ev.Atto, senza.Evento, senza.Atto)
			}
			// e nell'altro verso: l'evento non tocca la precedenza. Esito, candidato, confidenza, legame e
			// motivi sono quelli della precedenza da sola, con e senza candidati
			for _, x := range []IngressoTriage{c.in, con} {
				tr, sola := Triage(x), triageEsito(x)
				if tr.Esito != sola.Esito || tr.Confidenza != sola.Confidenza || tr.Legame != sola.Legame ||
					!slices.Equal(tr.Motivi, sola.Motivi) || (tr.Candidato == nil) != (sola.Candidato == nil) ||
					tr.Candidato != nil && tr.Candidato.ThreadID != sola.Candidato.ThreadID {
					t.Errorf("l'evento ha cambiato la proposta di aggancio: %s/%d/%s contro %s/%d/%s",
						tr.Esito, tr.Confidenza, tr.Legame, sola.Esito, sola.Confidenza, sola.Legame)
				}
			}
		})
	}
	// Un'eccezione sola, dichiarata: le parole di posta non di lavoro in una risposta di un cliente con
	// l'evidenza di una RFQ aperta (evidenza_test.go, la guardia della revisione del 25/09). Senza
	// candidati il testo decide (non_business, «ignora»); con R0 si legge senza E2. Tutto il resto no.
	in := dalCliente("R: RFQ 7120001", "Vi confermiamo la quantità di 200 pezzi.\n\nIscriviti alla nostra newsletter!")
	senza := Triage(in)
	in.Candidati = r0
	conR0 := Triage(in)
	if senza.Atto != AttoNonBusiness || conR0.Atto == AttoNonBusiness || conR0.Evento != EventoRispostaCommerciale {
		t.Errorf("l'eccezione della firma: senza %s/%s, con R0 %s/%s", senza.Evento, senza.Atto, conR0.Evento, conR0.Atto)
	}
}

// 245 — gli otto eventi, una sottoprova per evento, e i casi limite di A5.16.2.
func TestGliOttoEventi(t *testing.T) {
	casi := []struct {
		nome              string
		in                IngressoEvento
		evento, atto      string
		forza             string
		evidenzaContiene  string
		evidenzaNonDeveDa string
	}{
		{"NUOVA_RFQ: le parole e lo STEP", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Richiesta d'offerta 7120001", Corpo: "Buongiorno, vi chiediamo un preventivo per il particolare in allegato.", NomiAllegati: []string{"7120001A_1.stp", "7120001A_1.pdf"}},
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaChiaro, "«preventivo»", ""},
		{"NUOVA_RFQ: la prima mail con i CAD e basta", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "7120001", Corpo: "Buongiorno, in allegato.", NomiAllegati: []string{"7120001A.stp"}},
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaProbabile, "senza parole di richiesta", ""},
		{"ARRIVO_CAD", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", InReplyTo: "<m1@acme.example>",
			Oggetto: "R: RFQ 7120001", Corpo: "Ecco i modelli che mancavano.", NomiAllegati: []string{"7120001A_1.stp", "7120001B_1.stp"}},
			EventoArrivoCAD, AttoDocumentiAggiuntivi, ForzaProbabile, "7120001A_1.stp, 7120001B_1.stp", ""},
		{"REVISIONE_CAD: la rev nel nome e la parola", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RFQ 7120001", Corpo: "Vi giro il modello aggiornato.", NomiAllegati: []string{"7120001A_2.stp"}},
			EventoRevisioneCAD, AttoRevisioneDocumenti, ForzaChiaro, "7120001A_2.stp porta la rev 2", ""},
		{"REVISIONE_CAD: solo la rev nel nome", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RFQ 7120001", Corpo: "In allegato.", NomiAllegati: []string{"7120001A_B.stp"}},
			EventoRevisioneCAD, AttoRevisioneDocumenti, ForzaProbabile, "porta la rev B", ""},
		{"RISPOSTA_COMMERCIALE: una domanda sul lotto", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: offerta 7120001", Corpo: "Che prezzo ci fate per un lotto di 500 pezzi?"},
			EventoRispostaCommerciale, AttoDomandaChiarimento, ForzaProbabile, "«prezzo»", ""},
		{"RISPOSTA_COMMERCIALE: rifiuto", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: offerta 7120001", Corpo: "Purtroppo non siete competitivi sul target, non procediamo."},
			EventoRispostaCommerciale, AttoRifiuto, ForzaProbabile, "risponde sull'offerta", ""},
		{"OFFERTA_FORNITORE", IngressoEvento{Controparte: ControparteFornitore, Direzione: "entrata", Mittente: "info@fornitore-esempio.example",
			Oggetto: "R: richiesta 7120001", Corpo: "In allegato la nostra offerta.", NomiAllegati: []string{"offerta.pdf"}},
			EventoOffertaFornitore, AttoOfferta, ForzaProbabile, "offerta", ""},
		{"ORDINE", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: offerta 7120001", Corpo: "Vi inviamo il purchase order per 500 pezzi.", NomiAllegati: []string{"PO_7120001.pdf"}},
			EventoOrdine, AttoOrdine, ForzaChiaro, "«purchase order»", ""},
		{"SOLLECITO", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RFQ 7120001", Corpo: "Vi sollecitiamo l'offerta, grazie."},
			EventoSollecito, AttoSollecito, ForzaProbabile, "«sollecitiamo»", ""},
		{"ALTRO: la nostra mail in uscita", IngressoEvento{Controparte: ControparteCliente, Direzione: "uscita", DalCockpit: true,
			Oggetto: "R: RFQ 7120001", Corpo: "Vi inviamo la nostra offerta.", NomiAllegati: []string{"offerta.pdf"}},
			EventoAltro, AttoOfferta, ForzaChiaro, "preparata dal Cockpit", ""},
		{"ALTRO: una newsletter", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", Mittente: "newsletter@acme.example",
			Oggetto: "Le novità di settembre", Corpo: "Richiesta d'offerta in un clic: iscriviti al webinar!"},
			EventoAltro, AttoNonBusiness, ForzaChiaro, "", ""},

		// i casi limite
		{"una revisione annunciata senza file è un annuncio, non un arrivo", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RICHIESTA D'OFFERTA", Corpo: "Vi inviamo la rev. B."},
			EventoAltro, AttoIncerto, ForzaIncerto, "nessun segno", ""},
		{"le parole dell'oggetto di una risposta non contano (3R)", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RICHIESTA OFFERTA 7120001", Corpo: "Ecco i disegni.", NomiAllegati: []string{"7120001A_1.stp"}},
			EventoArrivoCAD, AttoDocumentiAggiuntivi, ForzaProbabile, "risposta con 1 file tecnico", ""},
		{"una risposta che chiede un'offerta con uno STEP è una richiesta nuova", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			InReplyTo: "<vecchia@acme.example>", Oggetto: "R: ordine settembre", Corpo: "Vi chiediamo anche una quotazione per questo pezzo.", NomiAllegati: []string{"7120003A.stp"}},
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaChiaro, "il testo nuovo dice «quotazione»", ""},
		{"le parole della storia citata non contano", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: offerta", Corpo: "Grazie, ricevuto.\n\nDa: Mario Rossi <buyer@acme.example>\nInviato: giovedì 17 settembre 2026 09:12\nOggetto: sollecito\n\nVi sollecitiamo la richiesta d'offerta."},
			EventoAltro, AttoIncerto, ForzaIncerto, "", ""},
		{"un sollecito con gli STEP non è un sollecito", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RFQ 7120001", Corpo: "Reminder: ecco anche i modelli aggiornati.", NomiAllegati: []string{"7120001A_3.stp"}},
			EventoRevisioneCAD, AttoRevisioneDocumenti, ForzaChiaro, "porta la rev 3", ""},
		{"la prima emissione nel nome non è una revisione", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "R: RFQ 7120001", Corpo: "In allegato.", NomiAllegati: []string{"7120001A_A.stp", "foto_2.stp"}},
			EventoArrivoCAD, AttoDocumentiAggiuntivi, ForzaProbabile, "", "porta la rev"},
		{"la posta fuori da Outlook: la risposta si riconosce dalla storia citata", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "RFQ 7120001", Corpo: "Ecco lo STEP.\n\n-----Messaggio originale-----\nDa: Ufficio tecnico <tecnico@azienda.example>\nOggetto: RFQ 7120001\n\nCi mandate il modello?", NomiAllegati: []string{"7120001A_1.stp"}},
			EventoArrivoCAD, AttoDocumentiAggiuntivi, ForzaProbabile, "", ""},
		{"un mittente da censire: prima chi è, poi che cosa vuole", IngressoEvento{Controparte: ControparteSconosciuto, Direzione: "entrata",
			Oggetto: "Richiesta d'offerta", Corpo: "Richiesta d'offerta in allegato.", NomiAllegati: []string{"7120001A.stp"}},
			EventoAltro, AttoIncerto, ForzaIncerto, "censire", ""},
		{"il collega che gira una richiesta: l'atto è del contenuto girato", IngressoEvento{Controparte: ControparteInterno, Direzione: "uscita", Interno: true,
			Oggetto: "I: RFQ 7120001", Corpo: "Ti giro questa.\n\n-----Messaggio inoltrato-----\nDa: Buyer <buyer@acme.example>\nInviato: 17/09/2026 09:12\nOggetto: RFQ 7120001\n\nVi chiediamo un preventivo per 7120001A.", NomiAllegati: []string{"7120001A.stp"}},
			EventoNuovaRFQ, AttoRichiestaOfferta, ForzaChiaro, "«preventivo»", ""},
		{"un fornitore che scrive «richiesta d'offerta» resta un fornitore: mai NUOVA_RFQ, l'atto è di AttoFornitore", IngressoEvento{Controparte: ControparteFornitore, Direzione: "entrata", Mittente: "info@fornitore-esempio.example",
			Oggetto: "Richiesta d'offerta 7120001", Corpo: "Vi chiediamo quotazione per la zincatura? Allego il disegno.", NomiAllegati: []string{"7120001A_2.stp"}},
			EventoOffertaFornitore, AttoOfferta, ForzaProbabile, "l'offerta del fornitore", ""},
	}
	visti := map[string]bool{}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := Evento(c.in)
			if e.Evento != c.evento || e.Atto != c.atto || e.Forza != c.forza {
				t.Fatalf("%s/%s (%s), atteso %s/%s (%s): %v", e.Evento, e.Atto, e.Forza, c.evento, c.atto, c.forza, e.Evidenze)
			}
			ev := strings.Join(e.Evidenze, " | ")
			if len(e.Evidenze) == 0 || ev == "" {
				t.Errorf("un evento senza evidenze non si spiega: %+v", e)
			}
			if c.evidenzaContiene != "" && !strings.Contains(ev, c.evidenzaContiene) {
				t.Errorf("evidenze %q: manca %q", ev, c.evidenzaContiene)
			}
			if c.evidenzaNonDeveDa != "" && strings.Contains(ev, c.evidenzaNonDeveDa) {
				t.Errorf("evidenze %q: non doveva esserci %q", ev, c.evidenzaNonDeveDa)
			}
			// l'evento detto da Evento è quello che EventoDa rilegge dall'atto salvato
			legame := ""
			if c.in.Controparte == ControparteInterno {
				legame = LegameInoltro
			}
			if d := EventoDa(c.in.Controparte, c.in.Direzione, e.Atto, legame); d != e.Evento {
				t.Errorf("EventoDa rilegge %s, Evento dice %s", d, e.Evento)
			}
			// e l'atto è uno di quelli che il database accetta
			if e.Atto != "" && !AttoValido(e.Atto) {
				t.Errorf("atto inventato: %q", e.Atto)
			}
			visti[e.Evento] = true
		})
	}
	for _, ev := range Eventi {
		if !visti[ev] {
			t.Errorf("l'evento %s non ha una sottoprova", ev)
		}
	}
	// le righe dei motivi: prefisso, evento, forza e prima evidenza; e si rileggono uguali
	e := Evento(casi[3].in)
	m := MotiviEvento(e)
	if len(m) == 0 || !strings.HasPrefix(m[0], "evento · REVISIONE_CAD (chiaro): 7120001A_2.stp porta la rev 2") {
		t.Fatalf("motivi: %q", m)
	}
	letto, altri := LeggiMotiviEvento(append(append([]string{}, m...), "In-Reply-To verso la richiesta"))
	if letto.Evento != e.Evento || letto.Forza != e.Forza || !slices.Equal(letto.Evidenze, e.Evidenze) || !slices.Equal(altri, []string{"In-Reply-To verso la richiesta"}) {
		t.Errorf("riletto %+v (altri %q), scritto %+v", letto, altri, e)
	}
	for _, x := range m {
		if strings.Contains(x, "%") {
			t.Errorf("una riga dell'evento con una percentuale: %q", x)
		}
	}
}

// 246 — ogni atto ha un solo evento: EventoDa è una funzione, per ogni controparte, direzione e legame
// dà uno degli otto eventi; ogni evento ha almeno un atto che ci porta; il vecchio atto `inoltro`,
// che dal M2 non si scrive più, si legge come ALTRO; e un atto che non è nel vocabolario è ALTRO.
func TestOgniAttoHaUnSoloEvento(t *testing.T) {
	controparti := []string{"", ControparteCliente, ControparteFornitore, ControparteInterno, ControparteAltro, ControparteSconosciuto, ControparteAmbiguo}
	legami := []string{"", LegameNuovo, LegameRisposta, LegameAggiornamento, LegameInoltro, LegameNessuno, LegameIncerto}
	raggiunti := map[string]bool{}
	for _, atto := range append(append([]string{}, Atti...), "", "inventato") {
		for _, c := range controparti {
			for _, d := range []string{"entrata", "uscita"} {
				for _, l := range legami {
					ev := EventoDa(c, d, atto, l)
					if !slices.Contains(Eventi, ev) {
						t.Fatalf("EventoDa(%q, %q, %q, %q) = %q: non è un evento", c, d, atto, l, ev)
					}
					if again := EventoDa(c, d, atto, l); again != ev {
						t.Fatalf("EventoDa non è una funzione: %q poi %q", ev, again)
					}
					raggiunti[ev] = true
					if atto == AttoInoltro || atto == "" || atto == "inventato" {
						if ev != EventoAltro {
							t.Errorf("l'atto %q con %s/%s/%s dà %s, atteso ALTRO", atto, c, d, l, ev)
						}
					}
					// le nostre mail in uscita non sono mai eventi del cliente (salvo l'inoltro di un collega)
					if d == "uscita" && !(c == ControparteInterno && l == LegameInoltro) && ev != EventoAltro {
						t.Errorf("una nostra mail in uscita (%s, %s, %s) dà %s", c, atto, l, ev)
					}
				}
			}
		}
	}
	for _, ev := range Eventi {
		if !raggiunti[ev] {
			t.Errorf("nessun atto porta a %s", ev)
		}
	}
	// un solo evento per atto, per un cliente in entrata: la tabella di A5.16.2
	attesi := map[string]string{
		AttoRichiestaOfferta: EventoNuovaRFQ, AttoDocumentiAggiuntivi: EventoArrivoCAD, AttoRevisioneDocumenti: EventoRevisioneCAD,
		AttoDomandaChiarimento: EventoRispostaCommerciale, AttoRispostaChiarimento: EventoRispostaCommerciale,
		AttoAccettazione: EventoRispostaCommerciale, AttoRifiuto: EventoRispostaCommerciale, AttoOrdine: EventoOrdine,
		AttoSollecito: EventoSollecito, AttoOfferta: EventoAltro, AttoConfermaRicezione: EventoAltro, AttoInoltro: EventoAltro,
		AttoNotifica: EventoAltro, AttoComunicazioneGenerica: EventoAltro, AttoNonBusiness: EventoAltro, AttoIncerto: EventoAltro,
	}
	for _, atto := range Atti {
		want, ok := attesi[atto]
		if !ok {
			t.Errorf("l'atto %q non è nella tabella della prova: il vocabolario è cambiato", atto)
			continue
		}
		if ev := EventoDa(ControparteCliente, "entrata", atto, LegameRisposta); ev != want {
			t.Errorf("cliente in entrata, atto %s: %s, atteso %s", atto, ev, want)
		}
		// l'inoltro di un collega legge l'atto come un cliente
		if ev := EventoDa(ControparteInterno, "uscita", atto, LegameInoltro); ev != want {
			t.Errorf("inoltro, atto %s: %s, atteso %s", atto, ev, want)
		}
		// il fornitore in entrata: solo l'offerta è un evento suo
		wantF := EventoAltro
		if atto == AttoOfferta {
			wantF = EventoOffertaFornitore
		}
		if ev := EventoDa(ControparteFornitore, "entrata", atto, LegameRisposta); ev != wantF {
			t.Errorf("fornitore in entrata, atto %s: %s, atteso %s", atto, ev, wantF)
		}
	}
	// ogni evento ha un'etichetta per la schermata, e nessuna è il codice grezzo
	for _, ev := range Eventi {
		if et := EtichettaEvento(ev); et == "" || et == ev {
			t.Errorf("etichetta di %s: %q", ev, et)
		}
	}
}

// Smistamento 4.13 (giro 4, risposta 12 del 29/09) — la formula di chiusura non fa un sollecito.
//
// «Resto in attesa di Vs. riscontro» chiude quasi ogni richiesta di preventivo scritta in italiano, e
// E5 veniva prima di E8: una richiesta con quella riga in fondo diventava un SOLLECITO. Ora la formula
// fa un sollecito solo in un testo senza parole di richiesta; le parole di sollecito vere («sollecito»,
// «reminder») decidono come prima, anche accanto a una richiesta. Testi e numeri inventati.
func TestLaChiusuraDiUnaRichiestaNonEUnSollecito(t *testing.T) {
	casi := []struct {
		nome             string
		in               IngressoEvento
		evento, forza    string
		evidenzaContiene string
	}{
		{"una richiesta dal portale che chiude con la formula", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "SI CTE 12345 26999999 - codici nuovi",
			Corpo:   "Buongiorno,\nvi chiediamo un preventivo per i codici 7120001 e 7120010, i disegni sono sul portale.\nResto in attesa di Vs. riscontro.\nCordiali saluti"},
			EventoNuovaRFQ, ForzaProbabile, "«preventivo»"},
		{"una risposta che chiede una quotazione e chiude con la formula", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			InReplyTo: "<m1@acme.example>", Oggetto: "R: RFQ 7120001",
			Corpo: "Vi chiediamo anche la quotazione del 7120010.\nIn attesa di un vostro riscontro, cordiali saluti."},
			EventoNuovaRFQ, ForzaProbabile, "«quotazione»"},
		{"la formula da sola resta un sollecito", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			InReplyTo: "<m1@acme.example>", Oggetto: "R: RFQ 7120001", Corpo: "Buongiorno, resto in attesa di Vs. riscontro."},
			EventoSollecito, ForzaProbabile, "«in attesa di vs. riscontro»"},
		{"«vi sollecito un riscontro» senza richiesta resta un sollecito", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			InReplyTo: "<m1@acme.example>", Oggetto: "R: RFQ 7120001", Corpo: "Buongiorno, vi sollecito un riscontro sulla nostra richiesta."},
			EventoSollecito, ForzaProbabile, "«sollecito»"},
		// le parole di sollecito vere battono ancora quelle di richiesta: «Sollecito RDO …» è un sollecito
		{"un sollecito che nomina la RDO resta un sollecito", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Sollecito RDO 400099999", Corpo: "Buongiorno, vi sollecitiamo l'offerta. Resto in attesa di Vs. riscontro."},
			EventoSollecito, ForzaProbabile, "«sollecitiamo»"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := Evento(c.in)
			ev := strings.Join(e.Evidenze, " | ")
			if e.Evento != c.evento || e.Forza != c.forza {
				t.Fatalf("%s (%s), atteso %s (%s): %s", e.Evento, e.Forza, c.evento, c.forza, ev)
			}
			if !strings.Contains(ev, c.evidenzaContiene) {
				t.Errorf("evidenze %q: manca %q", ev, c.evidenzaContiene)
			}
			// l'atto salvato rilegge lo stesso evento
			if d := EventoDa(c.in.Controparte, c.in.Direzione, e.Atto, ""); d != e.Evento {
				t.Errorf("EventoDa rilegge %s, Evento dice %s", d, e.Evento)
			}
		})
	}
	// e Triage dice lo stesso: la richiesta del portale si salva come richiesta d'offerta
	tr := Triage(dalCliente(casi[0].in.Oggetto, casi[0].in.Corpo))
	if tr.Evento != EventoNuovaRFQ || tr.Atto != AttoRichiestaOfferta {
		t.Errorf("Triage: %s/%s, atteso NUOVA_RFQ/%s", tr.Evento, tr.Atto, AttoRichiestaOfferta)
	}
}

// Smistamento 4.13 — «commande» da sola non è più un ORDINE chiaro, e il confine delle parole d'ordine
// ammette il «_» («Bestellung_4500099999», la parola attaccata al numero da un gestionale).
//
// «commande» stava fra le parole d'ordine con forza «chiaro»: una richiesta di prezzo in francese per
// «une commande de 20 pièces» diventava un ORDINE sicuro. Ora «commande» è nella lista a parte: è un
// ordine probabile, anche accanto a una parola di richiesta («devis» sta anche negli ordini veri: «suite
// à votre devis»), e sceglie l'operatore. «bon de commande» resta chiaro. Per `\b` il «_» è una lettera,
// e dopo «bestellung_» il confine non c'era. Numeri inventati.
func TestLeParoleDOrdineChiareEQuelleProbabili(t *testing.T) {
	casi := []struct {
		nome             string
		in               IngressoEvento
		evento, forza    string
		evidenzaContiene string
	}{
		{"«commande» in una domanda di prezzo non è un ordine chiaro", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Demande de prix 7120001", Corpo: "Bonjour, demande de prix pour une commande de 20 pièces du 7120001."},
			EventoOrdine, ForzaProbabile, "«commande», che da sola non basta"},
		// accanto a una parola di richiesta resta un ordine probabile: il piano non dà la precedenza alla
		// richiesta, e «devis» sta anche in un ordine vero che risponde a un preventivo
		{"«commande» accanto a «devis» in un ordine resta un ordine probabile", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Commande 7120001", Corpo: "Bonjour, suite à votre devis n° 123, veuillez trouver ci-joint notre commande de 20 pièces."},
			EventoOrdine, ForzaProbabile, "«commande»"},
		{"«commande» accanto a «devis» in una richiesta resta un ordine probabile", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Devis 7120001", Corpo: "Bonjour, merci de nous envoyer un devis pour une commande de 20 pièces du 7120001."},
			EventoOrdine, ForzaProbabile, "«commande»"},
		{"«bon de commande» resta un ordine chiaro", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Commande 7120001", Corpo: "Bonjour, ci-joint le bon de commande n° 4500099999 pour 20 pièces."},
			EventoOrdine, ForzaChiaro, "«bon de commande»"},
		{"«Bestellung_» seguito dal numero, nel testo", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "7120001", Corpo: "Anbei Bestellung_4500099999 für 20 Stück."},
			EventoOrdine, ForzaChiaro, "il testo nuovo dice «bestellung»"},
		{"«Bestellung_» seguito dal numero, nell'oggetto", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "Bestellung_4500099999", Corpo: "Mit freundlichen Grüßen"},
			EventoOrdine, ForzaChiaro, "l'oggetto dice «bestellung»"},
		{"«ordine n.» attaccato al «_»", IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata",
			Oggetto: "7120001", Corpo: "Vi mandiamo l'ordine n.4500099999_1 per 20 pezzi."},
			EventoOrdine, ForzaChiaro, "«ordine n.4500099999»"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := Evento(c.in)
			ev := strings.Join(e.Evidenze, " | ")
			if e.Evento != c.evento || e.Forza != c.forza {
				t.Fatalf("%s (%s), atteso %s (%s): %s", e.Evento, e.Forza, c.evento, c.forza, ev)
			}
			if !strings.Contains(ev, c.evidenzaContiene) {
				t.Errorf("evidenze %q: manca %q", ev, c.evidenzaContiene)
			}
			if strings.Contains(ev, "_»") || strings.Contains(ev, "«_") {
				t.Errorf("il «_» del confine è finito nell'evidenza: %q", ev)
			}
		})
	}
	// il confine si apre al «_», non a qualunque lettera: dentro una parola «commande» non c'è
	e := Evento(IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", InReplyTo: "<m1@acme.example>",
		Oggetto: "R: 7120001", Corpo: "Je vous recommande ce fournisseur."})
	if e.Evento == EventoOrdine {
		t.Errorf("«recommandé» letto come ordine: %+v", e)
	}
}
