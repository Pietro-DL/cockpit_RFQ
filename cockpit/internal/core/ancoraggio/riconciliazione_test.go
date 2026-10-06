// L1 — la riconciliazione fra lo STEP e il cartiglio dei 2D associati ai nodi (piano A, 6.0.6; workflow, passo 10 e il
// caso del §4; contratto §0 punto 4, §1.4, §2.2, §2.5, §2.6, §7 famiglia B4; commit P6b, fase 3): dai fatti STEP e PDF
// del worker all'adattatore, a Interpreta, a StrutturaDa e a ProponiAncoraggi. I cinque esiti (concorda,
// completamento_proposto, correzione_proposta, discordante, non_verificabile: T-B0-11, T-B0-27, R64 A, R87) con i loro
// motivi, mai applicati; il cartiglio come candidato di revisione con l'entità (T-E1-06); l'origine dell'associazione;
// la discordanza con la decisione del nodo e le evidenze dei due lati (T-E1-15, T-B0-24, R97 B, T-E1R-08);
// RevisioneInferiore (T-E1-20); PO-07, PO-23 e PO-37 nella parte di B4, con la variante di E1R; il caso del workflow
// sulle fixture ACME (t2, t2', t2″, t2T); il determinismo, gli errori di contratto, i valori.
package ancoraggio_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example); la famiglia è acme-catena di catena_test.go (712xxxx
// con il marcatore A o B e la revisione di una cifra: «7120100A_1» nel nome del file, «7120100A» nei nodi che non
// danno la revisione, «7120100A1» nel cartiglio), più una famiglia di un altro spazio di codici («ALT» e quattro
// cifre); gli UUID sono 00000000-0000-4000-8000-0000000009nn. Dove la grammatica ACME non produce il caso (una base
// non confrontabile, una revisione sospesa, due letture diverse dello stesso cartiglio), la lettura si costruisce a
// mano sul documento vero, ed è scritto. Le prove citano i requisiti (R64 A, R87, T-E1-20, PO-07, PO-23, PO-37), mai i
// casi degli attesi.

// ---- la scena ----

var (
	idStepRic  = uidS(0x901)
	shaStepRic = fmt.Sprintf("%064x", 0x901)
	idProdRic  = uidS(0x910) // il componente del prodotto 7120100A
	idComp1Ric = uidS(0x911) // 7120101A
	idComp2Ric = uidS(0x912) // 7120102A
	idComp3Ric = uidS(0x913) // 7120103A
	idUtRic    = uidS(0x9f0)
	ilRic      = time.Date(2026, 10, 3, 8, 15, 0, 0, time.FixedZone("CEST", 2*3600))
)

// nodiScenaRic: lo STEP della scena, come il caso del workflow (§4): la radice 7120100A legge base e marcatore senza
// revisione (con la formazione «1»); il figlio 7120101A, parziale, con la formazione «1»; il figlio 7120102A_3 con
// l'identità completa (formazione «3»); il figlio 7120103A senza formazione; una saldatura che nessuna famiglia legge.
func nodiScenaRic() ([]nodoR, []arcoS) {
	return []nodoR{{"#1", "7120100A", "TELAIO", "1"}, {"#2", "7120101A", "SUPPORTO", "1"}, {"#3", "7120102A_3", "PIASTRA", "3"},
			{"#4", "7120103A", "BOCCOLA", ""}, {"#5", "SALDATURA_1", "", ""}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#1", "#4", 2, []string{"#41", "#42"}}, {"#1", "#5", 1, nil}}
}

func rRic(chiave string) string { return ancoraggio.RifNodo(shaStepRic, chiave) }

// scenaRic: il motore, lo STEP (file e struttura), il target confermato con il suo componente e il contesto con i codici
// composti, come li preparerà valutazione.
type scenaRic struct {
	m    *motorea.Motore
	step ancoraggio.FileInterpretato
	s    ancoraggio.StrutturaFile
	tg   ancoraggio.ProdottoRichiesto
}

func nuovaScenaRic(t *testing.T, m *motorea.Motore, nomeStep string) scenaRic {
	t.Helper()
	nodi, archi := nodiScenaRic()
	f := conDisponibilita(fileS(t, m, idStepRic, nomeStep, "stp", shaStepRic, fattiR(t, shaStepRic, nodi, archi)), ancoraggio.DisponibilitaDisponibile)
	return scenaRic{m: m, step: f, s: strutturaS(t, f), tg: targetC(t, m, ancoraggio.RifComponente(idProdRic), ancoraggio.AutoritaConfermata, "7120100A", uuidP(idProdRic))}
}

// ctx: il contesto della scena, con le aggiunte date.
func (sc scenaRic) ctx(modifica func(*ancoraggio.ContestoStrutturale)) ancoraggio.ContestoStrutturale {
	c := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{sc.s}, CodiciProposti: codiciProposti(sc.m, sc.s)}
	if modifica != nil {
		modifica(&c)
	}
	return c
}

// ancora: ProponiAncoraggi con lo STEP e i file dati, con gli invarianti di ancoraA (e quelli della riconciliazione).
func (sc scenaRic) ancora(t *testing.T, ctx ancoraggio.ContestoStrutturale, file ...ancoraggio.FileInterpretato) ancoraggio.EsitoAncoraggi {
	t.Helper()
	return ancoraA(t, append([]ancoraggio.FileInterpretato{sc.step}, file...), []ancoraggio.ProdottoRichiesto{sc.tg}, ctx)
}

// pdfRic: un 2D PDF, segnato come disegno da valutazione, con il codice del cartiglio dato e il nome dato.
func pdfRic(t *testing.T, m *motorea.Motore, n int, nome, codice string) ancoraggio.FileInterpretato {
	t.Helper()
	f := pdfF(t, m, uidS(n), nome, fmt.Sprintf("%064x", n), []campoP{{"codice", codice}})
	f.Disegno = true
	return f
}

// nodoRic: il nodo dell'unica struttura del target della scena.
func nodoRic(t *testing.T, e ancoraggio.EsitoAncoraggi, rif string) ancoraggio.NodoProposto {
	t.Helper()
	if len(e.Strutture) != 1 {
		t.Fatalf("%d strutture, attesa una", len(e.Strutture))
	}
	return nodoC(t, e.Strutture[0], rif)
}

// documentaleDi: il codice documentale del 2D sul nodo, che deve esserci.
func documentaleDi(t *testing.T, n ancoraggio.NodoProposto, allegato uuid.UUID) ancoraggio.CodiceDocumentale {
	t.Helper()
	for _, cd := range n.Codice.Documentale {
		if cd.AllegatoID == allegato {
			return cd
		}
	}
	t.Fatalf("il nodo %s non ha il codice documentale di %s: %+v", n.Rif, allegato, n.Codice.Documentale)
	return ancoraggio.CodiceDocumentale{}
}

// candidatiDa: i valori dei candidati di revisione del nodo con quella fonte, nell'ordine dell'identità.
func candidatiDa(n ancoraggio.NodoProposto, fonte string) []string {
	var out []string
	for _, c := range n.Codice.Identita.CandidatiRevisione {
		if c.Fonte == fonte {
			out = append(out, c.Valore)
		}
	}
	return out
}

// conCodice: le diagnostiche con quel codice.
func diagnosticheCon(e ancoraggio.EsitoAncoraggi, codice string) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, d := range e.Diagnostiche {
		if d.Codice == codice {
			out = append(out, d)
		}
	}
	return out
}

// unaDiscordanza: l'unica discordanza del codice documentale, o nil se non ce n'è; più d'una è un errore della prova
// (servono solo dove il nodo ha una decisione sola).
func unaDiscordanza(t *testing.T, cd ancoraggio.CodiceDocumentale) *ancoraggio.DiscordanzaDecisione {
	t.Helper()
	switch len(cd.Discordanze) {
	case 0:
		return nil
	case 1:
		return &cd.Discordanze[0]
	}
	t.Fatalf("%d discordanze, attesa al più una: %+v", len(cd.Discordanze), cd.Discordanze)
	return nil
}

// decisa: la riga decisa (confermata, da una persona) del nodo della scena con quella chiave, sul componente.
func decisaRic(n int, chiave string, componente uuid.UUID) ancoraggio.RigaDecisaLegacy {
	return rigaDecisa(uidS(n), idStepRic, shaStepRic, chiave, ancoraggio.StatoRigaConfermata, uuidP(componente), uuidP(idUtRic))
}

// decisioneRic: una DecisioneIdentita sul componente, con le evidenze viste date.
func decisioneRic(componente uuid.UUID, codice, revisione string, viste ...ancoraggio.EvidenzaVista) ancoraggio.DecisioneIdentita {
	return ancoraggio.DecisioneIdentita{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: componente, Codice: codice, Revisione: revisione,
		EvidenzeViste: viste, Da: idUtRic, Il: ilRic}
}

// ---- gli invarianti di ogni esito ----

// senzaRiconciliazione: la catena senza ciò che aggiunge la riconciliazione (il codice documentale, i candidati e le
// fonti del cartiglio) e con lo stato della revisione che ne segue: deve essere quella di ProponiStrutture.
func senzaRiconciliazione(c ancoraggio.CatenaCodice) ancoraggio.CatenaCodice {
	c.Documentale = nil
	var cand []ancoraggio.CandidatoRevisione
	for _, x := range c.Identita.CandidatiRevisione {
		if x.Fonte != ancoraggio.FonteRevisioneCartiglio {
			cand = append(cand, x)
		}
	}
	var senza []ancoraggio.FonteSenzaRevisione
	for _, x := range c.Identita.FontiSenzaRevisione {
		if x.Fonte != ancoraggio.FonteRevisioneCartiglio {
			senza = append(senza, x)
		}
	}
	c.Identita.CandidatiRevisione, c.Identita.FontiSenzaRevisione = cand, senza
	switch {
	case c.Confermato != nil && c.Confermato.RevProvenienza == ancoraggio.RevProvenienzaDecisioneTracciata:
		c.Identita.StatoRevisione = ancoraggio.StatoRevisioneConfermata
	case c.Identita.Revisione != nil || len(cand) > 0:
		c.Identita.StatoRevisione = ancoraggio.StatoRevisioneCandidata
	default:
		c.Identita.StatoRevisione = ancoraggio.StatoRevisioneAssente
	}
	return c
}

// controllaRiconciliazione: gli invarianti della riconciliazione di ogni esito (ancoraA li chiama sempre):
//   - un codice documentale solo per un 2D (FileInterpretato.Disegno), uno per 2D e nodo, in ordine di allegato;
//     l'origine proposto solo con un candidato del file su quel nodo (e senza documento), manuale e confermato solo con
//     un'associazione decisa di quel file con quell'origine; ogni 2D candidato su un nodo ha il suo codice documentale;
//   - l'esito è uno dei cinque; il motivo solo per non_verificabile e discordante; la correzione mai per concorda e
//     non_verificabile, sempre per completamento e correzione, e il completamento solo su un'identità senza revisione;
//     la correzione dice il file e l'unità della lettura;
//   - l'associazione del codice documentale è quella dell'ancoraggio del file (T-B4-39);
//   - ogni discordanza porta una decisione del nodo com'è (la confermata, la manuale), una per decisione, in ordine di
//     origine (T-B4-40); con la decisione tracciata il conflitto vuol dire evidenza nuova, l'indicatore evidenza vista,
//     con chi e quando; senza, nessun chi né quando; contro il codice manuale un conflitto, nel codice e nella revisione
//     (T-B4-34 corretta); contro il codice confermato del legacy il codice è un conflitto e la revisione sempre un
//     indicatore (R97 B, mai un conflitto); un 2D solo proposto con l'associazione ambigua o discordante dà solo
//     indicatori (T-B4-39); l'evidenza è la coppia (cartiglio, testo grezzo del campo); RevisioneInferiore della
//     correzione è vera se lo è in almeno una discordanza;
//   - i candidati e le fonti del cartiglio vengono da un 2D del nodo;
//   - una diagnostica della riconciliazione per ogni correzione accanto, in avviso, dati.
//
// Che niente si applichi (la catena, le decisioni, gli ancoraggi) lo controlla ancoraA, con le strutture di
// ProponiStrutture.
func controllaRiconciliazione(t *testing.T, e ancoraggio.EsitoAncoraggi, file []ancoraggio.FileInterpretato, ctx ancoraggio.ContestoStrutturale) {
	t.Helper()
	disegni := map[uuid.UUID]bool{}
	for _, f := range file {
		if f.Disegno {
			disegni[f.AllegatoID] = true
		}
	}
	proposte := map[string]map[uuid.UUID]bool{}
	for _, a := range e.File {
		if !disegni[a.AllegatoID] {
			continue
		}
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				k := p.Target + "|" + p.AllegatoID.String() + "|" + p.Radice + "|" + p.Nodo
				if proposte[k] == nil {
					proposte[k] = map[uuid.UUID]bool{}
				}
				proposte[k][a.AllegatoID] = true
			}
		}
	}
	associazioni := map[uuid.UUID]ancoraggio.Associazione{}
	for _, a := range e.File {
		associazioni[a.AllegatoID] = a.Associazione
	}
	decise := map[string]bool{}
	for _, x := range ctx.AssociazioniDecise {
		decise[x.AllegatoID.String()+"|"+string(x.Origine)] = true
		if x.DocumentoID != nil {
			decise[x.AllegatoID.String()+"|"+x.DocumentoID.String()] = true
		}
	}
	correzioni := 0
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			k := s.Target + "|" + s.AllegatoID.String() + "|" + s.Radice + "|" + n.Rif
			visti := map[uuid.UUID]bool{}
			for i, cd := range n.Codice.Documentale {
				if i > 0 && n.Codice.Documentale[i-1].AllegatoID.String() >= cd.AllegatoID.String() {
					t.Errorf("nodo %s: codici documentali fuori ordine o ripetuti", n.Rif)
				}
				visti[cd.AllegatoID] = true
				if !disegni[cd.AllegatoID] {
					t.Errorf("nodo %s: un codice documentale di %s, che non è un 2D", n.Rif, cd.AllegatoID)
				}
				switch cd.OrigineAssociazione {
				case ancoraggio.OrigineProposto:
					if !proposte[k][cd.AllegatoID] || cd.DocumentoID != nil {
						t.Errorf("nodo %s: origine proposto senza un candidato del file sul nodo, o con il documento: %+v", n.Rif, cd)
					}
				case ancoraggio.OrigineManuale, ancoraggio.OrigineConfermato:
					if !decise[cd.AllegatoID.String()+"|"+string(cd.OrigineAssociazione)] || (cd.DocumentoID != nil && !decise[cd.AllegatoID.String()+"|"+cd.DocumentoID.String()]) {
						t.Errorf("nodo %s: origine %s senza l'associazione decisa: %+v", n.Rif, cd.OrigineAssociazione, cd)
					}
				default:
					t.Errorf("nodo %s: origine dell'associazione %q", n.Rif, cd.OrigineAssociazione)
				}
				switch cd.Esito {
				case ancoraggio.RiconciliazioneConcorda:
					if cd.Motivo != "" || cd.Correzione != nil {
						t.Errorf("nodo %s: concorda con il motivo o la correzione: %+v", n.Rif, cd)
					}
				case ancoraggio.RiconciliazioneCompletamentoProposto:
					if cd.Motivo != "" || cd.Correzione == nil || cd.Correzione.Revisione == nil || n.Codice.Identita.Revisione != nil {
						t.Errorf("nodo %s: completamento %+v sull'identità %+v (R87)", n.Rif, cd, n.Codice.Identita)
					}
				case ancoraggio.RiconciliazioneCorrezioneProposta:
					if cd.Motivo != "" || cd.Correzione == nil {
						t.Errorf("nodo %s: correzione senza la correzione: %+v", n.Rif, cd)
					}
				case ancoraggio.RiconciliazioneDiscordante:
					if cd.Motivo == "" {
						t.Errorf("nodo %s: discordante senza motivo", n.Rif)
					}
				case ancoraggio.RiconciliazioneNonVerificabile:
					if cd.Motivo == "" || cd.Correzione != nil {
						t.Errorf("nodo %s: non verificabile %+v", n.Rif, cd)
					}
				default:
					t.Errorf("nodo %s: esito %q", n.Rif, cd.Esito)
				}
				if c := cd.Correzione; c != nil {
					correzioni++
					if c.AllegatoID != cd.AllegatoID || c.UnitaID != cd.UnitaID || cd.Lettura == "" || c.Codice == "" {
						t.Errorf("nodo %s: la correzione %+v non dice la provenienza della lettura %+v", n.Rif, c, cd)
					}
				}
				if cd.Associazione != associazioni[cd.AllegatoID] {
					t.Errorf("nodo %s: l'associazione %q, l'ancoraggio del file dice %q (T-B4-39)", n.Rif, cd.Associazione, associazioni[cd.AllegatoID])
				}
				incerta := cd.OrigineAssociazione == ancoraggio.OrigineProposto &&
					(cd.Associazione == ancoraggio.AssociazioneAmbiguo || cd.Associazione == ancoraggio.AssociazioneDiscordante)
				inferiore := false
				for j, d := range cd.Discordanze {
					var deciso *ancoraggio.CodiceDeciso
					switch d.Decisione.Origine {
					case ancoraggio.OrigineConfermato:
						deciso = n.Codice.Confermato
					case ancoraggio.OrigineManuale:
						deciso = n.Codice.Manuale
					}
					if deciso == nil || canonico(t, d.Decisione) != canonico(t, *deciso) || (j > 0 && cd.Discordanze[j-1].Decisione.Origine >= d.Decisione.Origine) {
						t.Errorf("nodo %s: la discordanza porta %+v, non una decisione del nodo, o è fuori ordine (T-B4-40)", n.Rif, d.Decisione)
						continue
					}
					inferiore = inferiore || d.RevisioneInferiore
					if d.Parte != ancoraggio.ParteDiscordanzaCodice && d.Parte != ancoraggio.ParteDiscordanzaRevisione {
						t.Errorf("nodo %s: parte %q", n.Rif, d.Parte)
					}
					if incerta && d.Effetto != ancoraggio.EffettoDiscordanzaIndicatore {
						t.Errorf("nodo %s: un 2D solo proposto con l'associazione %s dà un conflitto %+v (T-B4-39)", n.Rif, cd.Associazione, d)
					}
					if deciso.RevProvenienza == ancoraggio.RevProvenienzaDecisioneTracciata {
						nuova := d.Motivo == ancoraggio.MotivoDiscordanzaEvidenzaNuova
						if (!nuova && d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaVista) || (d.Effetto == ancoraggio.EffettoDiscordanzaConflitto) != (nuova && !incerta) ||
							d.DecisaDa == nil || d.DecisaIl == nil {
							t.Errorf("nodo %s: discordanza con la decisione tracciata %+v (T-E1R-08)", n.Rif, d)
						}
					} else {
						attesi := map[string][2]string{ancoraggio.ParteDiscordanzaRevisione: {ancoraggio.EffettoDiscordanzaIndicatore, ancoraggio.MotivoDiscordanzaRevisioneRegistrata}}
						attesi[ancoraggio.ParteDiscordanzaCodice] = [2]string{ancoraggio.EffettoDiscordanzaConflitto, ancoraggio.MotivoDiscordanzaCodiceConfermato}
						if deciso.Origine == ancoraggio.OrigineManuale {
							manuale := [2]string{ancoraggio.EffettoDiscordanzaConflitto, ancoraggio.MotivoDiscordanzaCodiceManuale}
							attesi[ancoraggio.ParteDiscordanzaCodice], attesi[ancoraggio.ParteDiscordanzaRevisione] = manuale, manuale
						}
						atteso := attesi[d.Parte]
						if incerta {
							atteso[0] = ancoraggio.EffettoDiscordanzaIndicatore
						}
						if got := [2]string{d.Effetto, d.Motivo}; got != atteso || d.DecisaDa != nil || d.DecisaIl != nil {
							t.Errorf("nodo %s: discordanza %+v con una decisione del legacy (T-B0-24, R97 B)", n.Rif, d)
						}
					}
					if d.Evidenza != ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, cd.Originale) {
						t.Errorf("nodo %s: l'evidenza %+v non è la coppia del cartiglio %q (T-B4-22)", n.Rif, d.Evidenza, cd.Originale)
					}
				}
				if cd.Correzione != nil && cd.Correzione.RevisioneInferiore != inferiore {
					t.Errorf("nodo %s: RevisioneInferiore della correzione %v, delle discordanze %v", n.Rif, cd.Correzione.RevisioneInferiore, inferiore)
				}
			}
			for _, c := range n.Codice.Identita.CandidatiRevisione {
				if c.Fonte == ancoraggio.FonteRevisioneCartiglio && (c.AllegatoID == nil || !visti[*c.AllegatoID] || c.Entita == "") {
					t.Errorf("nodo %s: candidato del cartiglio %+v senza un 2D del nodo", n.Rif, c)
				}
			}
			for _, c := range n.Codice.Identita.FontiSenzaRevisione {
				if c.Fonte == ancoraggio.FonteRevisioneCartiglio && (c.AllegatoID == nil || !visti[*c.AllegatoID]) {
					t.Errorf("nodo %s: fonte del cartiglio %+v senza un 2D del nodo", n.Rif, c)
				}
			}
			for a := range proposte[k] {
				if !visti[a] {
					t.Errorf("nodo %s: il 2D %s è candidato sul nodo e non ha il codice documentale", n.Rif, a)
				}
			}
		}
	}
	diag := 0
	for _, d := range e.Diagnostiche {
		if d.Codice != ancoraggio.CodiceCompletamentoDocumentale && d.Codice != ancoraggio.CodiceCorrezioneDocumentale {
			continue
		}
		diag++
		if d.Gravita != evidenze.GravitaAvviso || d.Natura != evidenze.NaturaDati || len(d.Rif) != 4 {
			t.Errorf("diagnostica della riconciliazione %+v", d)
		}
	}
	if diag != correzioni {
		t.Errorf("%d diagnostiche della riconciliazione per %d correzioni accanto", diag, correzioni)
	}
}

// ---- il caso del workflow (§4), sulle fixture ACME ----

// TestRiconciliazioneIlCasoDelWorkflow (workflow §4 e passo 10; R87, T-B0-27, T-B0-35, R64 A, LD-04; PO-07): lo STEP
// «7120100A_1.stp» con la radice che non dà la revisione, il target confermato 7120100A con il suo componente.
//   - t2 (PO-07): arriva il PDF del prodotto con il cartiglio «7120100A1»: completamento_proposto con la revisione 1,
//     dal cartiglio, mai applicato: l'identità resta parziale, senza revisione e senza stringa canonica; il cartiglio
//     sta fra i candidati del prodotto, accanto al nome del file; la diagnostica del completamento; associazione e
//     riconciliazione sono due campi (T-B0-11); la revisione del nome dello STEP non fa discordare lo STEP (T-B0-35).
//   - t2': il cartiglio dice «7120100A2»: completamento con la 2, e accanto il candidato del nome del file con la 1,
//     senza una scelta; con il nome del PDF che dice la 1, il file ha identità discordanti: associazione discordante,
//     riconciliazione discordante, con il completamento accanto (R64 A).
//   - t2″: l'operatore aveva corretto a mano il codice della radice in X: X resta, accanto la proposta dello STEP e il
//     documentale, con il segnale del conflitto (codice_manuale, T-B0-24, T-B0-33).
//   - t2T: al posto del PDF un TIFF (e una scansione PDF senza testo): il cartiglio non si legge, non_verificabile;
//     resta l'identità proposta dallo STEP.
func TestRiconciliazioneIlCasoDelWorkflow(t *testing.T) {
	sc := nuovaScenaRic(t, motoreCatena(t), "7120100A_1.stp")
	ctx := sc.ctx(nil)
	prima := nodoRic(t, sc.ancora(t, ctx), rRic("#1"))
	if prima.Codice.Identita.Revisione != nil || !prima.Codice.Identita.Parziale || len(prima.Codice.Documentale) != 0 {
		t.Fatalf("la radice prima del 2D: %+v", prima.Codice)
	}
	nonApplicato := func(t *testing.T, r ancoraggio.NodoProposto) {
		t.Helper()
		id := r.Codice.Identita
		if id.Revisione != nil || !id.Parziale || id.Base != "7120100" || id.Marcatore != "A" || r.Codice.Proposto != "" ||
			r.Codice.MotivoProposto != motorea.MotivoComposizioneRevisioneNonDeterminata || r.Codice.Confermato != nil || r.Decisione != nil {
			t.Errorf("il cartiglio è stato applicato alla radice (R64 A, R87): %+v", r.Codice)
		}
		if canonico(t, r.Codice.Grezzo) != canonico(t, prima.Codice.Grezzo) || canonico(t, r.Codice.Manuale) != canonico(t, prima.Codice.Manuale) {
			t.Errorf("il grezzo o il manuale sono cambiati: %+v", r.Codice)
		}
	}

	t.Run("t2 e PO-07: il cartiglio completa con la revisione 1, mai applicata", func(t *testing.T) {
		pdf := pdfRic(t, sc.m, 0x921, "disegno-acme-telaio.pdf", "7120100A1")
		e := sc.ancora(t, ctx, pdf)
		if a := ancoraggioDi(t, e, pdf.AllegatoID); a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneRadice {
			t.Errorf("il PDF del prodotto: %s, %s", a.Associazione, a.Collocazione)
		}
		if a := ancoraggioDi(t, e, idStepRic); a.Associazione == ancoraggio.AssociazioneDiscordante {
			t.Errorf("la revisione del nome dello STEP fa discordare una radice senza revisione (T-B0-35): %+v", a)
		}
		r := nodoRic(t, e, rRic("#1"))
		nonApplicato(t, r)
		cd := documentaleDi(t, r, pdf.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneCompletamentoProposto || cd.OrigineAssociazione != ancoraggio.OrigineProposto || cd.Originale != "7120100A1" ||
			cd.Base.Normalizzata != "7120100" || cd.Revisione == nil || cd.Revisione.Normalizzata != "1" || cd.Lettura == "" || cd.Posizione.PDF == nil || len(cd.Discordanze) != 0 {
			t.Fatalf("t2: %+v", cd)
		}
		if c := cd.Correzione; c.Codice != "7120100A1" || testoP(c.Revisione) != "1" || c.AllegatoID != pdf.AllegatoID || c.Posizione.PDF == nil || c.RevisioneInferiore {
			t.Errorf("t2, il completamento: %+v", c)
		}
		if got := candidatiDa(r, ancoraggio.FonteRevisioneCartiglio); len(got) != 1 || got[0] != "1" {
			t.Errorf("il cartiglio fra i candidati: %v", got)
		}
		for _, c := range r.Codice.Identita.CandidatiRevisione {
			if c.Fonte == ancoraggio.FonteRevisioneCartiglio && (c.Entita != sc.tg.Rif || *c.AllegatoID != pdf.AllegatoID || c.Posizione.PDF == nil) {
				t.Errorf("il candidato del cartiglio è del prodotto, con il suo 2D (T-E1-06): %+v", c)
			}
		}
		if got := candidatiDa(r, ancoraggio.FonteRevisioneNomeFileSTEP); len(got) != 1 || got[0] != "1" || r.Codice.Identita.StatoRevisione != ancoraggio.StatoRevisioneCandidata {
			t.Errorf("il nome del file resta accanto: %v, stato %q", got, r.Codice.Identita.StatoRevisione)
		}
		d := diagnosticheCon(e, ancoraggio.CodiceCompletamentoDocumentale)
		if len(d) != 1 || d[0].Rif[2] != rRic("#1") || d[0].Rif[3] != "allegato:"+pdf.AllegatoID.String() || !strings.Contains(d[0].Messaggio, "«1»") {
			t.Errorf("la diagnostica del completamento: %+v", d)
		}
	})

	t.Run("t2': il cartiglio con la 2, il nome del file con la 1", func(t *testing.T) {
		pdf := pdfRic(t, sc.m, 0x922, "disegno-acme-telaio.pdf", "7120100A2")
		r := nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#1"))
		nonApplicato(t, r)
		if cd := documentaleDi(t, r, pdf.AllegatoID); cd.Esito != ancoraggio.RiconciliazioneCompletamentoProposto || testoP(cd.Correzione.Revisione) != "2" {
			t.Errorf("t2': %+v", cd)
		}
		if c, n := candidatiDa(r, ancoraggio.FonteRevisioneCartiglio), candidatiDa(r, ancoraggio.FonteRevisioneNomeFileSTEP); len(c) != 1 || c[0] != "2" || len(n) != 1 || n[0] != "1" {
			t.Errorf("t2': i due candidati restano tutti e due, senza scelta: cartiglio %v, nome %v", c, n)
		}
	})

	t.Run("t2': il nome del PDF dice la 1 e il cartiglio la 2: discordante, con il completamento accanto", func(t *testing.T) {
		pdf := pdfRic(t, sc.m, 0x923, "7120100A_1.pdf", "7120100A2")
		e := sc.ancora(t, ctx, pdf)
		if a := ancoraggioDi(t, e, pdf.AllegatoID); a.Associazione != ancoraggio.AssociazioneDiscordante || !contiene(a.Motivi, ancoraggio.MotivoAncoraggioIdentitaDiscordanti) {
			t.Errorf("il file con nome e cartiglio che non concordano: %+v (T-B0-35)", a)
		}
		r := nodoRic(t, e, rRic("#1"))
		nonApplicato(t, r)
		cd := documentaleDi(t, r, pdf.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneDiscordante || cd.Motivo != ancoraggio.MotivoAncoraggioIdentitaDiscordanti || cd.Correzione == nil ||
			cd.Correzione.Codice != "7120100A2" || testoP(cd.Correzione.Revisione) != "2" {
			t.Errorf("R64 A: discordante con la correzione accanto: %+v", cd)
		}
		if d := diagnosticheCon(e, ancoraggio.CodiceCompletamentoDocumentale); len(d) != 1 || !strings.Contains(d[0].Messaggio, "discordante") {
			t.Errorf("la diagnostica del completamento accanto al file discordante: %+v", d)
		}
	})

	t.Run("t2'': il codice corretto a mano in X resta, accanto il documentale e il segnale del conflitto", func(t *testing.T) {
		manuale, lm := "7120109A", letturaC(t, sc.m, "nodo_step.id", "7120109A")
		riga := rigaAperta(uidS(0x931), idStepRic, shaStepRic, "#1")
		riga.CodiceManuale, riga.LetturaManuale = &manuale, &lm
		ctxM := sc.ctx(func(c *ancoraggio.ContestoStrutturale) { c.Proposto = []ancoraggio.RigaPropostaLegacy{riga} })
		pdf := pdfRic(t, sc.m, 0x924, "disegno-acme-telaio.pdf", "7120100A1")
		r := nodoRic(t, sc.ancora(t, ctxM, pdf), rRic("#1"))
		if r.Codice.Manuale == nil || r.Codice.Manuale.Codice != "7120109A" || r.Codice.Identita.Revisione != nil || r.Codice.Proposto != "" {
			t.Fatalf("la decisione manuale non resta il valore corrente: %+v", r.Codice)
		}
		cd := documentaleDi(t, r, pdf.AllegatoID)
		d := unaDiscordanza(t, cd)
		if cd.Esito != ancoraggio.RiconciliazioneCompletamentoProposto || d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice ||
			d.Effetto != ancoraggio.EffettoDiscordanzaConflitto || d.Motivo != ancoraggio.MotivoDiscordanzaCodiceManuale ||
			d.Decisione.Codice != "7120109A" || d.Decisione.Origine != ancoraggio.OrigineManuale || d.Evidenza.Valore != "7120100A1" || d.DecisaDa != nil {
			t.Errorf("t2'': %+v, la discordanza %+v", cd, d)
		}
	})

	t.Run("t2T: un TIFF, e una scansione senza testo: non_verificabile", func(t *testing.T) {
		tiff := conDisponibilita(fileS(t, sc.m, uidS(0x925), "7120100A_1.tif", "tif", fmt.Sprintf("%064x", 0x925), nil), ancoraggio.DisponibilitaSenzaTesto)
		tiff.Disegno = true
		scansione := pdfF(t, sc.m, uidS(0x926), "7120100A_1.pdf", fmt.Sprintf("%064x", 0x926), nil)
		scansione.Disegno = true
		e := sc.ancora(t, ctx, tiff, scansione)
		r := nodoRic(t, e, rRic("#1"))
		nonApplicato(t, r)
		for _, id := range []uuid.UUID{tiff.AllegatoID, scansione.AllegatoID} {
			if a := ancoraggioDi(t, e, id); a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
				t.Errorf("t2T, %s: la revisione del nome del file non discorda con una radice senza revisione (T-B0-35): %+v", id, a)
			}
			cd := documentaleDi(t, r, id)
			if cd.Esito != ancoraggio.RiconciliazioneNonVerificabile || cd.Motivo != ancoraggio.MotivoDocumentaleCartiglioNonLetto || cd.Lettura != "" ||
				cd.Correzione != nil || len(cd.Discordanze) != 0 {
				t.Errorf("t2T, %s: %+v (LD-04)", id, cd)
			}
		}
		if got := candidatiDa(r, ancoraggio.FonteRevisioneCartiglio); len(got) != 0 || len(e.Diagnostiche) != 0 {
			t.Errorf("t2T: nessun candidato del cartiglio, nessuna diagnostica: %v %+v", got, e.Diagnostiche)
		}
	})

	t.Run("un file che non è un 2D non ha un codice documentale", func(t *testing.T) {
		pdf := pdfRic(t, sc.m, 0x927, "disegno-acme-telaio.pdf", "7120100A1")
		pdf.Disegno = false
		r := nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#1"))
		if len(r.Codice.Documentale) != 0 || len(candidatiDa(r, ancoraggio.FonteRevisioneCartiglio)) != 0 {
			t.Errorf("riconciliazione di un file che valutazione non dice 2D (T-B4-31): %+v", r.Codice.Documentale)
		}
	})
}

// ---- i cinque esiti e i motivi ----

// TestGliEsitiDellaRiconciliazione (T-B0-11, T-B0-27; R86, R87; workflow, passo 10): un 2D per ogni figlio della scena.
//   - 7120102A_3 con il cartiglio «7120102A3»: concorda, nessuna correzione, nessuna diagnostica; «7120102A4»:
//     correzione_proposta (la revisione); 7120101A con «7120101B2» (associato sul componente, perché un altro marcatore
//     scritto non è un candidato): correzione_proposta (R86), con il marcatore diverso fra le fonti senza revisione;
//     la base del nodo parziale che il cartiglio completa: correzione_proposta.
//   - il candidato del cartiglio è del nodo senza decisione e del componente per il nodo deciso (T-E1-06).
//   - non_verificabile con i motivi: il nodo senza identità (la saldatura decisa), il cartiglio letto solo in un altro
//     spazio di codici, il cartiglio senza revisione con lo STEP che la dà, una revisione sospesa, una base non
//     completa, una base non confrontabile (le ultime quattro con la lettura costruita a mano sul documento vero).
//   - discordante: due letture diverse dello stesso cartiglio, nessuna scelta (con l'associazione candidato_unico:
//     due campi, T-B0-11).
func TestGliEsitiDellaRiconciliazione(t *testing.T) {
	m := motore(t, famCatena(), famAltroRic())
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	conf := func(c *ancoraggio.ContestoStrutturale) {
		c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp1Ric, "7120101A", nil), componenteC(t, m, idComp2Ric, "7120102A", nil),
			componenteC(t, m, idComp3Ric, "7120103A", nil)}
	}

	t.Run("concorda e correzione_proposta sulla revisione", func(t *testing.T) {
		uguale, diversa := pdfRic(t, m, 0x941, "disegno-a.pdf", "7120102A3"), pdfRic(t, m, 0x942, "disegno-b.pdf", "7120102A4")
		e := sc.ancora(t, sc.ctx(nil), uguale, diversa)
		n := nodoRic(t, e, rRic("#3"))
		if cd := documentaleDi(t, n, uguale.AllegatoID); cd.Esito != ancoraggio.RiconciliazioneConcorda || cd.Correzione != nil || cd.Motivo != "" {
			t.Errorf("concorda: %+v", cd)
		}
		cd := documentaleDi(t, n, diversa.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneCorrezioneProposta || cd.Correzione.Codice != "7120102A4" || testoP(cd.Correzione.Revisione) != "4" {
			t.Errorf("la revisione diversa: %+v", cd)
		}
		if *n.Codice.Identita.Revisione != "3" || n.Codice.Proposto != "7120102A3" {
			t.Errorf("la correzione è stata applicata: %+v", n.Codice.Identita)
		}
		for _, c := range n.Codice.Identita.CandidatiRevisione {
			if c.Fonte == ancoraggio.FonteRevisioneCartiglio && c.Entita != rRic("#3") {
				t.Errorf("il candidato del cartiglio di un nodo senza decisione è del nodo (T-E1-06): %+v", c)
			}
		}
		if got := candidatiDa(n, ancoraggio.FonteRevisioneCartiglio); strings.Join(got, ",") != "3,4" {
			t.Errorf("i candidati dei due cartigli: %v", got)
		}
		if d := diagnosticheCon(e, ancoraggio.CodiceCorrezioneDocumentale); len(d) != 1 || d[0].Rif[3] != "allegato:"+diversa.AllegatoID.String() || len(diagnosticheCon(e, ancoraggio.CodiceCompletamentoDocumentale)) != 0 {
			t.Errorf("una diagnostica per la correzione, nessuna per la concordanza: %+v", e.Diagnostiche)
		}
	})

	t.Run("un altro marcatore scritto: correzione_proposta e marcatore_diverso", func(t *testing.T) {
		pdf := pdfRic(t, m, 0x943, "disegno-c.pdf", "7120101B2")
		ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
			conf(c)
			c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x951, "#2", idComp1Ric)}
			c.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{{AllegatoID: pdf.AllegatoID, ComponenteID: idComp1Ric, Origine: ancoraggio.OrigineManuale}}
		})
		e := sc.ancora(t, ctx, pdf)
		if a := ancoraggioDi(t, e, pdf.AllegatoID); len(a.Candidati) != 0 {
			t.Errorf("un altro marcatore scritto non è un candidato del nodo (T-B4-30): %+v", a.Candidati)
		}
		n := nodoRic(t, e, rRic("#2"))
		cd := documentaleDi(t, n, pdf.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneCorrezioneProposta || cd.OrigineAssociazione != ancoraggio.OrigineManuale || cd.Correzione.Codice != "7120101B2" {
			t.Errorf("il marcatore diverso: %+v", cd)
		}
		trovata := false
		for _, f := range n.Codice.Identita.FontiSenzaRevisione {
			trovata = trovata || (f.Fonte == ancoraggio.FonteRevisioneCartiglio && f.Motivo == ancoraggio.MotivoSenzaRevisioneMarcatoreDiverso && f.Entita == ancoraggio.RifComponente(idComp1Ric))
		}
		if !trovata || len(candidatiDa(n, ancoraggio.FonteRevisioneCartiglio)) != 0 {
			t.Errorf("il cartiglio con un altro marcatore non è un candidato: %+v", n.Codice.Identita)
		}
		if d := unaDiscordanza(t, cd); d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice || d.Motivo != ancoraggio.MotivoDiscordanzaCodiceConfermato {
			t.Errorf("il marcatore diverso contro il codice confermato: %+v", d)
		}
	})

	t.Run("la base del nodo parziale, che il cartiglio completa: correzione_proposta", func(t *testing.T) {
		s := sc.s
		s.Nodi = append([]ancoraggio.NodoStruttura(nil), sc.s.Nodi...)
		for i := range s.Nodi {
			if s.Nodi[i].Rif == rRic("#4") {
				s.Nodi[i].Letture = append([]motorea.LetturaCodice(nil), s.Nodi[i].Letture...)
				s.Nodi[i].Letture[0].Forma.Base.Completa = false // costruita a mano: una base parziale (A-C07)
			}
		}
		pdf := pdfRic(t, m, 0x944, "disegno-d.pdf", "7120103A1")
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)}
		e := ancoraA(t, []ancoraggio.FileInterpretato{pdf}, []ancoraggio.ProdottoRichiesto{sc.tg}, ctx)
		if cd := documentaleDi(t, nodoRic(t, e, rRic("#4")), pdf.AllegatoID); cd.Esito != ancoraggio.RiconciliazioneCorrezioneProposta {
			t.Errorf("la base parziale del nodo: %+v", cd)
		}
	})

	t.Run("non_verificabile: il nodo senza identità e un altro spazio di codici", func(t *testing.T) {
		saldatura, altro := pdfRic(t, m, 0x945, "disegno-e.pdf", "7120103A1"), pdfRic(t, m, 0x946, "disegno-f.pdf", "ALT1234")
		ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
			conf(c)
			c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x952, "#5", idComp3Ric), decisaRic(0x953, "#3", idComp2Ric)}
			d1, d2 := uidS(0x961), uidS(0x962)
			c.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{
				{AllegatoID: saldatura.AllegatoID, DocumentoID: &d1, ComponenteID: idComp3Ric, Origine: ancoraggio.OrigineConfermato},
				{AllegatoID: altro.AllegatoID, DocumentoID: &d2, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineConfermato}}
		})
		e := sc.ancora(t, ctx, saldatura, altro)
		s5 := nodoRic(t, e, rRic("#5"))
		cd := documentaleDi(t, s5, saldatura.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneNonVerificabile || cd.Motivo != ancoraggio.MotivoDocumentaleNodoSenzaIdentita || cd.Originale != "7120103A1" ||
			cd.Lettura == "" || cd.OrigineAssociazione != ancoraggio.OrigineConfermato || cd.DocumentoID == nil || *cd.DocumentoID != uidS(0x961) || len(cd.Discordanze) != 0 {
			t.Errorf("il nodo senza identità: %+v", cd)
		}
		if len(s5.Codice.Identita.CandidatiRevisione) != 0 || len(s5.Codice.Identita.FontiSenzaRevisione) != 0 {
			t.Errorf("nessun indizio senza un'identità: %+v", s5.Codice.Identita)
		}
		if cd := documentaleDi(t, nodoRic(t, e, rRic("#3")), altro.AllegatoID); cd.Esito != ancoraggio.RiconciliazioneNonVerificabile ||
			cd.Motivo != ancoraggio.MotivoDocumentaleAltroNamespace || cd.Originale != "ALT1234" || len(cd.Discordanze) != 0 {
			t.Errorf("il cartiglio di un altro spazio di codici: %+v", cd)
		}
	})

	// Le letture costruite a mano sul documento vero: il caso che la grammatica ACME non produce.
	conLettura := func(f ancoraggio.FileInterpretato, cambia func(*motorea.LetturaForma)) ancoraggio.FileInterpretato {
		f.Interpretazione.Letture = append([]motorea.LetturaCodice(nil), f.Interpretazione.Letture...)
		for i := range f.Interpretazione.Letture {
			if f.Interpretazione.Letture[i].Forma.Selettore.String() == "cartiglio.codice" {
				cambia(&f.Interpretazione.Letture[i].Forma)
			}
		}
		return f
	}
	for _, c := range []struct {
		nome   string
		cambia func(*motorea.LetturaForma)
		motivo string
		fonte  string // il motivo della fonte senza revisione del cartiglio, "" se nessuna
	}{
		{"senza revisione, lo STEP la dà", func(f *motorea.LetturaForma) { f.Revisione = nil }, ancoraggio.MotivoDocumentaleRevisioneAssente, ancoraggio.MotivoSenzaRevisioneNonLetta},
		{"una revisione sospesa (D5)", func(f *motorea.LetturaForma) {
			r := *f.Revisione
			r.Stato, r.Normalizzata, r.Segmenti = motorea.StatoRevisioneNonInterpretabile, "", nil
			f.Revisione, f.Stato = &r, motorea.StatoDaVerificare
		}, ancoraggio.MotivoDocumentaleRevisioneAmbigua, ancoraggio.MotivoSenzaRevisioneAmbigua},
		{"una base non completa", func(f *motorea.LetturaForma) {
			b := f.Base
			b.Completa = false
			f.Base = b
		}, ancoraggio.MotivoDocumentaleCartiglioNonCompleto, ""},
		{"una base non confrontabile", func(f *motorea.LetturaForma) {
			b := f.Base
			b.Segmenti = append([]motorea.SegmentoLetto(nil), b.Segmenti...)
			b.Segmenti[0].Nome = "altro"
			f.Base = b
		}, ancoraggio.MotivoDocumentaleBaseNonConfrontabile, ""},
	} {
		t.Run("non_verificabile: "+c.nome, func(t *testing.T) {
			pdf := conLettura(pdfRic(t, m, 0x947, "disegno-g.pdf", "7120102A3"), c.cambia)
			ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
				conf(x)
				x.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x954, "#3", idComp2Ric)}
				x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{{AllegatoID: pdf.AllegatoID, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineManuale}}
			})
			n := nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#3"))
			cd := documentaleDi(t, n, pdf.AllegatoID)
			if cd.Esito != ancoraggio.RiconciliazioneNonVerificabile || cd.Motivo != c.motivo || cd.Correzione != nil || cd.Lettura == "" {
				t.Errorf("%s: %+v", c.nome, cd)
			}
			var fonti []string
			for _, f := range n.Codice.Identita.FontiSenzaRevisione {
				if f.Fonte == ancoraggio.FonteRevisioneCartiglio {
					fonti = append(fonti, f.Motivo)
				}
			}
			if (c.fonte == "" && len(fonti) != 0) || (c.fonte != "" && (len(fonti) != 1 || fonti[0] != c.fonte)) || len(candidatiDa(n, ancoraggio.FonteRevisioneCartiglio)) != 0 {
				t.Errorf("%s: le fonti senza revisione del cartiglio %v, attesa %q", c.nome, fonti, c.fonte)
			}
		})
	}

	t.Run("discordante: due letture diverse dello stesso cartiglio, nessuna scelta", func(t *testing.T) {
		pdf := pdfRic(t, m, 0x948, "disegno-h.pdf", "7120102A3")
		var altra motorea.LetturaCodice
		for _, l := range pdf.Interpretazione.Letture {
			if l.Forma.Selettore.String() == "cartiglio.codice" {
				altra = l
			}
		}
		// Costruita a mano: una seconda famiglia dello stesso spazio di codici che legge la stessa occorrenza con
		// un'altra revisione.
		r := *altra.Forma.Revisione
		r.Normalizzata, r.Segmenti = "5", nil
		altra.ID, altra.Forma.Famiglia, altra.Forma.Revisione = altra.ID+":altra", "acme-catena-bis", &r
		pdf.Interpretazione.Letture = append(append([]motorea.LetturaCodice(nil), pdf.Interpretazione.Letture...), altra)
		e := sc.ancora(t, sc.ctx(nil), pdf)
		if a := ancoraggioDi(t, e, pdf.AllegatoID); a.Associazione != ancoraggio.AssociazioneCandidatoUnico {
			t.Errorf("due letture della stessa occorrenza non fanno un file discordante: %+v", a)
		}
		n := nodoRic(t, e, rRic("#3"))
		cd := documentaleDi(t, n, pdf.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneDiscordante || cd.Motivo != ancoraggio.MotivoLettureDiscordanti || cd.Correzione != nil || cd.Lettura != "" {
			t.Errorf("le letture discordanti del cartiglio: %+v (T-E1-07)", cd)
		}
		if got := candidatiDa(n, ancoraggio.FonteRevisioneCartiglio); strings.Join(got, ",") != "3,5" {
			t.Errorf("le due letture restano due indizi, senza scelta: %v", got)
		}
	})
}

// famAltroRic: una famiglia di un altro spazio di codici, che legge sul cartiglio «ALT» e quattro cifre.
func famAltroRic() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-altro", Namespace: "acme-altro",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "ALT[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{formaC("altro", []string{"cartiglio.codice"}, pBaseC)},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-altro", "cartiglio.codice", "ALT1234", false, grammatica.LetturaAttesa{Forma: "altro", Base: "ALT1234"}),
		},
	}
}

// TestUnaBaseRipetutaCheNonConcorda (D2; R64 A, T-B0-35): il cartiglio «7120110_R7120111» (la forma con la base
// ripetuta della scena degli ancoraggi) legge due basi: il file è discordante, e sui due nodi la riconciliazione è
// discordante, senza correzione, senza indizi e senza discordanza con la decisione del nodo (il pezzo 7120110 è un
// componente rinominato in 7120119), perché la prima base non si sceglie.
func TestUnaBaseRipetutaCheNonConcorda(t *testing.T) {
	m := motoreAncoraggi(t)
	_, strutture := scenaFileACME(t, m, "")
	pdf := pdfF(t, m, uidS(0x971), "disegno-acme.pdf", fmt.Sprintf("%064x", 0x971), []campoP{{"codice", "7120110_R7120111"}})
	pdf.Disegno = true
	ctx := ancoraggio.ContestoStrutturale{Strutture: strutture,
		Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, uidS(0x972), "7120119", nil)},
		Decise:     []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x973), idAssieme100, shaAssieme100, "#2", ancoraggio.StatoRigaConfermata, uuidP(uidS(0x972)), uuidP(idUtRic))}}
	e := ancoraA(t, []ancoraggio.FileInterpretato{pdf}, targetScenaACME(t, m), ctx)
	if a := ancoraggioDi(t, e, pdf.AllegatoID); a.Associazione != ancoraggio.AssociazioneDiscordante {
		t.Fatalf("la base ripetuta che non concorda: %+v", a)
	}
	visti := 0
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			for _, cd := range n.Codice.Documentale {
				visti++
				if cd.Esito != ancoraggio.RiconciliazioneDiscordante || cd.Motivo != ancoraggio.MotivoAncoraggioIdentitaDiscordanti || cd.Correzione != nil || len(cd.Discordanze) != 0 {
					t.Errorf("nodo %s: %+v", n.Rif, cd)
				}
				fonti := 0
				for _, f := range n.Codice.Identita.FontiSenzaRevisione {
					if f.Fonte == ancoraggio.FonteRevisioneCartiglio {
						fonti++
					}
				}
				if len(candidatiDa(n, ancoraggio.FonteRevisioneCartiglio)) != 0 || fonti != 0 {
					t.Errorf("nodo %s: un indizio da una base ripetuta che non concorda: %+v", n.Rif, n.Codice.Identita)
				}
			}
		}
	}
	if n := nodoC(t, strutturaDiE(t, e, "scenario:caso-acme:1", idAssieme100), ancoraggio.RifNodo(shaAssieme100, "#2")); n.Codice.Confermato == nil ||
		len(n.Codice.Documentale) != 1 {
		t.Errorf("il pezzo deciso con il suo 2D: %+v", n.Codice)
	}
	if visti != 2 || len(diagnosticheCon(e, ancoraggio.CodiceCorrezioneDocumentale)) != 0 {
		t.Errorf("i due nodi delle due basi, nessuna diagnostica: %d %+v", visti, e.Diagnostiche)
	}
}

// ---- l'origine dell'associazione ----

// TestLOrigineDellAssociazione (contratto §1.5, §2.2: CodiceDocumentale.OrigineAssociazione, DocumentoID; lettura
// T-B4-33): il 2D candidato e confermato sullo stesso componente ha l'origine confermato, con il documento; il 2D
// assegnato a mano a un componente, senza essere candidato, ha l'origine manuale; il 2D confermato sul componente del
// prodotto sta sulla radice che rappresenta il prodotto; un'associazione decisa su un componente che nessun nodo
// porta non dà niente; un candidato non diventa mai una decisione (l'ancoraggio resta proposto).
func TestLOrigineDellAssociazione(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	candidatoEConfermato := pdfRic(t, m, 0x981, "disegno-a.pdf", "7120102A3")
	soloManuale := pdfRic(t, m, 0x982, "disegno-b.pdf", "7120199A1")
	prodotto := pdfRic(t, m, 0x983, "disegno-c.pdf", "7120100A1")
	nessuno := pdfRic(t, m, 0x984, "disegno-d.pdf", "7120198A1")
	doc1, doc2, doc3 := uidS(0x991), uidS(0x992), uidS(0x993)
	ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
		c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp2Ric, "7120102A", nil), componenteC(t, m, idProdRic, "7120100A", nil)}
		c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x985, "#3", idComp2Ric)}
		c.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{
			{AllegatoID: candidatoEConfermato.AllegatoID, DocumentoID: &doc1, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineConfermato},
			{AllegatoID: soloManuale.AllegatoID, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineManuale},
			{AllegatoID: prodotto.AllegatoID, DocumentoID: &doc2, ComponenteID: idProdRic, Origine: ancoraggio.OrigineConfermato},
			{AllegatoID: nessuno.AllegatoID, DocumentoID: &doc3, ComponenteID: idComp3Ric, Origine: ancoraggio.OrigineConfermato},
		}
	})
	e := sc.ancora(t, ctx, candidatoEConfermato, soloManuale, prodotto, nessuno)
	n3 := nodoRic(t, e, rRic("#3"))
	if len(n3.Codice.Documentale) != 2 {
		t.Fatalf("il nodo deciso ha i due 2D del suo componente: %+v", n3.Codice.Documentale)
	}
	if cd := documentaleDi(t, n3, candidatoEConfermato.AllegatoID); cd.OrigineAssociazione != ancoraggio.OrigineConfermato || cd.DocumentoID == nil || *cd.DocumentoID != doc1 ||
		cd.Esito != ancoraggio.RiconciliazioneConcorda {
		t.Errorf("candidato e confermato: l'origine più forte, con il documento: %+v", cd)
	}
	cd := documentaleDi(t, n3, soloManuale.AllegatoID)
	if cd.OrigineAssociazione != ancoraggio.OrigineManuale || cd.DocumentoID != nil || cd.Esito != ancoraggio.RiconciliazioneCorrezioneProposta {
		t.Errorf("assegnato a mano con un'altra base: %+v", cd)
	}
	if d := unaDiscordanza(t, cd); d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice || d.Effetto != ancoraggio.EffettoDiscordanzaConflitto || d.Decisione.Codice != "7120102A" {
		t.Errorf("un'altra base contro il codice confermato: %+v (T-B0-24)", d)
	}
	r := nodoRic(t, e, rRic("#1"))
	if cd := documentaleDi(t, r, prodotto.AllegatoID); cd.OrigineAssociazione != ancoraggio.OrigineConfermato || *cd.DocumentoID != doc2 || r.Decisione != nil {
		t.Errorf("il 2D del prodotto sulla radice che lo rappresenta, senza una decisione della radice (T-B4-23): %+v %+v", cd, r.Decisione)
	}
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			for _, cd := range n.Codice.Documentale {
				if cd.AllegatoID == nessuno.AllegatoID {
					t.Errorf("un 2D deciso su un componente senza nodo sta sul nodo %s", n.Rif)
				}
			}
		}
	}
	for _, a := range e.File {
		for _, c := range a.Candidati {
			if c.Origine != ancoraggio.OrigineProposto {
				t.Errorf("un candidato non è mai una decisione: %+v", c)
			}
		}
	}
}

// ---- le decisioni accanto: PO-23, PO-37, la decisione tracciata ----

// TestPO23UnPDFNuovoControUnaNomenclaturaConfermata (PO-23, parte B4; T-B0-24, R95 A, T-E1-15, T-E1R-08): il figlio
// 7120102A_3 è il componente 7120102A, confermato.
//   - Con una decisione tracciata (codice 7120102A, revisione 3, vista l'entità dello STEP) arriva un PDF con il
//     cartiglio «7120102A4»: correzione proposta con i due valori; il segnale del conflitto sulla revisione, con
//     l'evidenza nuova, chi e quando; la decisione conservata (lo stato confermata, il codice e la revisione decisi).
//   - Con la rinomina del componente nel legacy (7120108A) e il cartiglio che concorda con lo STEP: concorda con lo
//     STEP, e il conflitto sul codice contro il codice confermato.
func TestPO23UnPDFNuovoControUnaNomenclaturaConfermata(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	t.Run("decisione tracciata", func(t *testing.T) {
		ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
			c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp2Ric, "7120102A", testoS("3"))}
			c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9a1, "#3", idComp2Ric)}
			c.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp2Ric, "7120102A", "3", ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaStepEntita, "7120102A_3"))}
		})
		pdf := pdfRic(t, m, 0x9a2, "disegno-a.pdf", "7120102A4")
		e := sc.ancora(t, ctx, pdf)
		n := nodoRic(t, e, rRic("#3"))
		if c := n.Codice.Confermato; c == nil || c.Codice != "7120102A" || testoP(c.Rev) != "3" || c.RevProvenienza != ancoraggio.RevProvenienzaDecisioneTracciata ||
			n.Codice.Identita.StatoRevisione != ancoraggio.StatoRevisioneConfermata {
			t.Fatalf("la decisione non è conservata: %+v, stato %q", c, n.Codice.Identita.StatoRevisione)
		}
		cd := documentaleDi(t, n, pdf.AllegatoID)
		if cd.Esito != ancoraggio.RiconciliazioneCorrezioneProposta || cd.Correzione.Codice != "7120102A4" || testoP(cd.Correzione.Revisione) != "4" || cd.Correzione.RevisioneInferiore {
			t.Errorf("la correzione proposta: %+v", cd)
		}
		d := unaDiscordanza(t, cd)
		if d == nil || d.Parte != ancoraggio.ParteDiscordanzaRevisione || d.Effetto != ancoraggio.EffettoDiscordanzaConflitto || d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaNuova ||
			d.Decisione.Codice != "7120102A" || testoP(d.Decisione.Rev) != "3" || d.Decisione.Origine != ancoraggio.OrigineConfermato || *d.Decisione.ComponenteID != idComp2Ric ||
			*d.DecisaDa != idUtRic || !d.DecisaIl.Equal(ilRic) || d.DecisaIl.Location() != time.UTC ||
			d.Evidenza != (ancoraggio.EvidenzaVista{Fonte: ancoraggio.FonteEvidenzaCartiglio, Valore: "7120102A4"}) {
			t.Errorf("il segnale del conflitto con i due lati: %+v", d)
		}
		if len(diagnosticheCon(e, ancoraggio.CodiceCorrezioneDocumentale)) != 1 {
			t.Errorf("la diagnostica della correzione: %+v", e.Diagnostiche)
		}
	})
	t.Run("rinomina del legacy", func(t *testing.T) {
		ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
			c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp2Ric, "7120108A", nil)}
			c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9a3, "#3", idComp2Ric)}
		})
		pdf := pdfRic(t, m, 0x9a4, "disegno-b.pdf", "7120102A3")
		e := sc.ancora(t, ctx, pdf)
		n := nodoRic(t, e, rRic("#3"))
		cd := documentaleDi(t, n, pdf.AllegatoID)
		d := unaDiscordanza(t, cd)
		if cd.Esito != ancoraggio.RiconciliazioneConcorda || d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice || d.Effetto != ancoraggio.EffettoDiscordanzaConflitto ||
			d.Motivo != ancoraggio.MotivoDiscordanzaCodiceConfermato || d.Decisione.Codice != "7120108A" || d.DecisaDa != nil || n.Codice.Confermato.Codice != "7120108A" {
			t.Errorf("il codice confermato contro il cartiglio: %+v, %+v", cd, d)
		}
		if len(e.Diagnostiche) != 0 {
			t.Errorf("con il cartiglio che concorda con lo STEP non c'è una correzione: %+v", e.Diagnostiche)
		}
	})
}

// TestPO37LaRevisioneRegistrataELaDecisioneTracciata (PO-37, parte B4, con la variante di E1R; R97 B, T-E1-22,
// T-E1R-08): il figlio 7120101A (la formazione «1») è il componente 7120101A.
//   - componente.rev = 1 e il cartiglio con la revisione 2: completamento proposto, un indicatore, mai un conflitto,
//     con RevProvenienza coincide_con_formazione_step; il figlio 7120103A (nessuna formazione) con la revisione 1:
//     non_registrata; lo stato della revisione non è mai confermata (R97 B).
//   - La variante del modello nuovo: una DecisioneIdentita su 7120101A con la revisione 1, che aveva visto il cartiglio
//     «7120101A2»: lo stesso cartiglio non è un conflitto (un indicatore, evidenza_vista); un cartiglio nuovo
//     «7120101A3» sì (evidenza_nuova); il cartiglio con la revisione decisa non discorda. La decisione resta.
func TestPO37LaRevisioneRegistrataELaDecisioneTracciata(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	legacy := func(c *ancoraggio.ContestoStrutturale) {
		c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp1Ric, "7120101A", testoS("1")), componenteC(t, m, idComp3Ric, "7120103A", testoS("1"))}
		c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9b1, "#2", idComp1Ric), decisaRic(0x9b2, "#4", idComp3Ric)}
	}
	t.Run("R97 B: componente.rev è un indicatore", func(t *testing.T) {
		p2, p4 := pdfRic(t, m, 0x9b3, "disegno-a.pdf", "7120101A2"), pdfRic(t, m, 0x9b4, "disegno-b.pdf", "7120103A2")
		e := sc.ancora(t, sc.ctx(legacy), p2, p4)
		for _, c := range []struct {
			nodo, provenienza string
			pdf               uuid.UUID
		}{{rRic("#2"), ancoraggio.RevProvenienzaFormazioneSTEP, p2.AllegatoID}, {rRic("#4"), ancoraggio.RevProvenienzaNonRegistrata, p4.AllegatoID}} {
			n := nodoRic(t, e, c.nodo)
			cd := documentaleDi(t, n, c.pdf)
			d := unaDiscordanza(t, cd)
			if cd.Esito != ancoraggio.RiconciliazioneCompletamentoProposto || testoP(cd.Correzione.Revisione) != "2" || d == nil ||
				d.Parte != ancoraggio.ParteDiscordanzaRevisione || d.Effetto != ancoraggio.EffettoDiscordanzaIndicatore ||
				d.Motivo != ancoraggio.MotivoDiscordanzaRevisioneRegistrata || d.Decisione.RevProvenienza != c.provenienza || testoP(d.Decisione.Rev) != "1" {
				t.Errorf("nodo %s: %+v, %+v (R97 B, T-E1-22)", c.nodo, cd, d)
			}
			if n.Codice.Identita.StatoRevisione == ancoraggio.StatoRevisioneConfermata || testoP(n.Codice.Confermato.Rev) != "1" {
				t.Errorf("nodo %s: la revisione registrata non diventa confermata e non cambia: %+v", c.nodo, n.Codice)
			}
			for _, k := range n.Codice.Identita.CandidatiRevisione {
				if k.Fonte == ancoraggio.FonteRevisioneCartiglio && k.Entita != ancoraggio.RifComponente(n.Decisione.ComponenteID) {
					t.Errorf("nodo %s: il candidato del cartiglio è del componente deciso (T-E1-06): %+v", c.nodo, k)
				}
			}
		}
	})
	t.Run("E1R: la decisione tracciata e le evidenze viste", func(t *testing.T) {
		tracciata := func(c *ancoraggio.ContestoStrutturale) {
			legacy(c)
			c.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp1Ric, "7120101A", "1", ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120101A2"))}
		}
		for _, c := range []struct {
			cartiglio       string
			effetto, motivo string // "" se nessuna discordanza
		}{
			{"7120101A2", ancoraggio.EffettoDiscordanzaIndicatore, ancoraggio.MotivoDiscordanzaEvidenzaVista},
			{"7120101A3", ancoraggio.EffettoDiscordanzaConflitto, ancoraggio.MotivoDiscordanzaEvidenzaNuova},
			{"7120101A1", "", ""},
		} {
			pdf := pdfRic(t, m, 0x9b5, "disegno-c.pdf", c.cartiglio)
			n := nodoRic(t, sc.ancora(t, sc.ctx(tracciata), pdf), rRic("#2"))
			cd := documentaleDi(t, n, pdf.AllegatoID)
			d := unaDiscordanza(t, cd)
			switch {
			case c.effetto == "" && d != nil:
				t.Errorf("%s: la revisione decisa non discorda: %+v", c.cartiglio, d)
			case c.effetto != "" && (d == nil || d.Effetto != c.effetto || d.Motivo != c.motivo || d.Parte != ancoraggio.ParteDiscordanzaRevisione):
				t.Errorf("%s: %+v, attesi %s e %s (T-E1R-08)", c.cartiglio, d, c.effetto, c.motivo)
			}
			if n.Codice.Identita.StatoRevisione != ancoraggio.StatoRevisioneConfermata || testoP(n.Codice.Confermato.Rev) != "1" || cd.Esito != ancoraggio.RiconciliazioneCompletamentoProposto {
				t.Errorf("%s: la decisione resta il valore corrente: %+v, %+v", c.cartiglio, n.Codice.Confermato, cd)
			}
		}
	})
}

// TestLaRevisioneCheNonRegredisce (T-E1-20; R97, R95 A): una revisione del cartiglio minore di quella decisa resta una
// proposta, con RevisioneInferiore nella correzione e nella discordanza: contro la decisione tracciata è il segnale di un
// conflitto, contro componente.rev del legacy un indicatore. Le revisioni si ordinano solo quando sono numeri (gli zeri
// a sinistra non contano); una revisione con lettere non si dice minore. Una decisione tracciata senza revisione
// contro un cartiglio con la revisione è un conflitto (T-B4-21), e non è una revisione inferiore.
func TestLaRevisioneCheNonRegredisce(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	for i, c := range []struct {
		nome       string
		rev        *string // componente.rev
		tracciata  *string // la revisione della decisione tracciata; nil senza decisione tracciata
		cartiglio  string
		inferiore  bool
		effetto    string // "" senza discordanza
		correzione bool
	}{
		{"decisa 3, cartiglio 2, tracciata", nil, testoS("3"), "7120101A2", true, ancoraggio.EffettoDiscordanzaConflitto, true},
		{"decisa 3, cartiglio 2, legacy", testoS("3"), nil, "7120101A2", true, ancoraggio.EffettoDiscordanzaIndicatore, true},
		{"decisa 03, cartiglio 2, legacy", testoS("03"), nil, "7120101A2", true, ancoraggio.EffettoDiscordanzaIndicatore, true},
		{"decisa 02, cartiglio 2: non minore, ma diversa", testoS("02"), nil, "7120101A2", false, ancoraggio.EffettoDiscordanzaIndicatore, true},
		{"decisa 1, cartiglio 2", testoS("1"), nil, "7120101A2", false, ancoraggio.EffettoDiscordanzaIndicatore, true},
		{"decisa B, cartiglio 2: non si ordina", testoS("B"), nil, "7120101A2", false, ancoraggio.EffettoDiscordanzaIndicatore, true},
		{"decisa 12, cartiglio 2", testoS("12"), nil, "7120101A2", true, ancoraggio.EffettoDiscordanzaIndicatore, true},
		{"decisa senza revisione, tracciata", nil, testoS(""), "7120101A2", false, ancoraggio.EffettoDiscordanzaConflitto, true},
		{"nessuna revisione decisa, legacy", nil, nil, "7120101A2", false, "", true},
	} {
		t.Run(c.nome, func(t *testing.T) {
			ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
				x.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp1Ric, "7120101A", c.rev)}
				x.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9c0+i, "#2", idComp1Ric)}
				if c.tracciata != nil {
					x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp1Ric, "7120101A", *c.tracciata)}
				}
			})
			pdf := pdfRic(t, m, 0x9d0+i, "disegno.pdf", c.cartiglio)
			cd := documentaleDi(t, nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#2")), pdf.AllegatoID)
			if (cd.Correzione != nil) != c.correzione || (cd.Correzione != nil && cd.Correzione.RevisioneInferiore != c.inferiore) {
				t.Errorf("la correzione %+v: RevisioneInferiore attesa %v", cd.Correzione, c.inferiore)
			}
			switch d := unaDiscordanza(t, cd); {
			case c.effetto == "" && d != nil:
				t.Errorf("nessuna discordanza attesa: %+v", d)
			case c.effetto != "" && (d == nil || d.Effetto != c.effetto || d.RevisioneInferiore != c.inferiore || d.Parte != ancoraggio.ParteDiscordanzaRevisione):
				t.Errorf("la discordanza %+v: attesi %s, inferiore %v", d, c.effetto, c.inferiore)
			}
		})
	}
	t.Run("un altro codice: la revisione non si confronta", func(t *testing.T) {
		ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
			x.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp2Ric, "7120108A", testoS("3"))}
			x.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9cf, "#3", idComp2Ric)}
		})
		pdf := pdfRic(t, m, 0x9df, "disegno.pdf", "7120102A2")
		cd := documentaleDi(t, nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#3")), pdf.AllegatoID)
		if d := unaDiscordanza(t, cd); cd.Esito != ancoraggio.RiconciliazioneCorrezioneProposta || cd.Correzione.RevisioneInferiore || d == nil ||
			d.Parte != ancoraggio.ParteDiscordanzaCodice || d.RevisioneInferiore {
			t.Errorf("la revisione di un altro codice non è inferiore alla revisione decisa: %+v, %+v", cd, d)
		}
	})
}

// TestLaLetturaDelCodiceDeciso (lettura T-B4-32): una DecisioneIdentita su 7120102A con un
// codice che non è quello del componente (7120107A). Senza la sua lettura la base del codice deciso è vuota e il
// codice non si confronta con il cartiglio (resta la revisione); con la lettura che valutazione mette in LettureDecise
// la base c'è, e un cartiglio con l'identità dello STEP è un conflitto sul codice, con l'evidenza nuova.
func TestLaLetturaDelCodiceDeciso(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	pdf := pdfRic(t, m, 0x9e1, "disegno.pdf", "7120102A3")
	foto := func(letture map[string]motorea.LetturaForma) ancoraggio.NodoProposto {
		t.Helper()
		ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
			c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp2Ric, "7120102A", nil)}
			c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9e2, "#3", idComp2Ric)}
			c.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp2Ric, "7120107A", "3")}
			c.LettureDecise = letture
		})
		return nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#3"))
	}
	senza := foto(nil)
	if b := senza.Codice.Confermato.Base; len(b.Segmenti) != 0 || len(documentaleDi(t, senza, pdf.AllegatoID).Discordanze) != 0 {
		t.Errorf("senza la lettura del codice deciso: base %+v, discordanza %+v", b, documentaleDi(t, senza, pdf.AllegatoID).Discordanze)
	}
	con := foto(map[string]motorea.LetturaForma{ancoraggio.RifComponente(idComp2Ric): letturaC(t, m, "nodo_step.id", "7120107A")})
	d := unaDiscordanza(t, documentaleDi(t, con, pdf.AllegatoID))
	if con.Codice.Confermato.Base.Normalizzata != "7120107" || d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice || d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaNuova {
		t.Errorf("con la lettura del codice deciso: base %+v, discordanza %+v", con.Codice.Confermato.Base, d)
	}
	if documentaleDi(t, con, pdf.AllegatoID).Esito != ancoraggio.RiconciliazioneConcorda {
		t.Error("la riconciliazione è con lo STEP, non con la decisione: concorda")
	}
}

// ---- il determinismo, i contratti, i valori ----

// TestRiconciliazioneDeterministica (determinismo; 6.4.5 regola 8): i file, le associazioni decise e le decisioni in
// un altro ordine danno gli stessi byte canonici; il segno del 2D cambia HashIngresso, un'associazione decisa
// HashTarget.
func TestRiconciliazioneDeterministica(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "7120100A_1.stp")
	file := []ancoraggio.FileInterpretato{sc.step, pdfRic(t, m, 0x9f1, "disegno-a.pdf", "7120100A1"), pdfRic(t, m, 0x9f2, "7120101A_1.pdf", "7120101A2"),
		pdfRic(t, m, 0x9f3, "disegno-c.pdf", "7120102A4")}
	doc := uidS(0x9f9)
	ctx := sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
		c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp1Ric, "7120101A", testoS("1")), componenteC(t, m, idComp2Ric, "7120102A", testoS("3"))}
		c.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9f4, "#2", idComp1Ric), decisaRic(0x9f5, "#3", idComp2Ric)}
		c.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp2Ric, "7120102A", "3")}
		c.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{{AllegatoID: file[1].AllegatoID, DocumentoID: &doc, ComponenteID: idProdRic, Origine: ancoraggio.OrigineConfermato},
			{AllegatoID: file[3].AllegatoID, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineManuale}}
	})
	tg := []ancoraggio.ProdottoRichiesto{sc.tg}
	e := ancoraA(t, file, tg, ctx)
	inv := ctx
	inv.AssociazioniDecise = rovescia(ctx.AssociazioniDecise)
	inv.Decise = rovescia(ctx.Decise)
	inv.Confermato = rovescia(ctx.Confermato)
	f := ancoraA(t, rovescia(file), tg, inv)
	if canonico(t, e) != canonico(t, f) || e.Impronta != f.Impronta {
		t.Error("la riconciliazione dipende dall'ordine degli ingressi")
	}
	documentali := 0
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			documentali += len(n.Codice.Documentale)
		}
	}
	if documentali != 3 {
		t.Errorf("tre 2D riconciliati, %d", documentali)
	}
	senzaSegno := append([]ancoraggio.FileInterpretato(nil), file...)
	senzaSegno[2].Disegno = false
	if g := ancoraA(t, senzaSegno, tg, ctx); g.HashIngresso == e.HashIngresso || g.HashTarget != e.HashTarget {
		t.Error("il segno del 2D è un ingresso del file (HashIngresso)")
	}
	altra := ctx
	altra.AssociazioniDecise = ctx.AssociazioniDecise[:1]
	if g := ancoraA(t, file, tg, altra); g.HashTarget == e.HashTarget || g.HashIngresso != e.HashIngresso {
		t.Error("un'associazione decisa è del contesto (HashTarget)")
	}
}

// TestRiconciliazioneErroriDiContratto: gli ingressi della riconciliazione sbagliati sono errori di contratto, con i
// codici della foglia: un'associazione decisa con un'origine proposto, senza allegato, di un allegato che non è fra i
// file, senza componente, confermata senza documento, ripetuta; la lettura di un codice deciso senza una decisione sul
// componente, o con una chiave che non è il Rif di un componente.
func TestRiconciliazioneErroriDiContratto(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	pdf := pdfRic(t, m, 0x9a9, "disegno.pdf", "7120102A3")
	doc := uidS(0x9aa)
	buona := ancoraggio.AssociazioneDecisa{AllegatoID: pdf.AllegatoID, DocumentoID: &doc, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineConfermato}
	lettura := letturaC(t, m, "nodo_step.id", "7120107A")
	for _, c := range []struct {
		nome     string
		cambia   func(*ancoraggio.ContestoStrutturale)
		codice   string
		percorso string
	}{
		{"origine proposto", func(x *ancoraggio.ContestoStrutturale) {
			a := buona
			a.Origine = ancoraggio.OrigineProposto
			x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{a}
		}, evidenze.CodiceDocumentoEnumIgnoto, "contesto.associazioni_decise[0].origine"},
		{"senza allegato", func(x *ancoraggio.ContestoStrutturale) {
			a := buona
			a.AllegatoID = uuid.Nil
			x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{a}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.associazioni_decise[0].allegato_id"},
		{"allegato fuori dai file", func(x *ancoraggio.ContestoStrutturale) {
			a := buona
			a.AllegatoID = uidS(0x9ab)
			x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{a}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.associazioni_decise[0].allegato_id"},
		{"senza componente", func(x *ancoraggio.ContestoStrutturale) {
			a := buona
			a.ComponenteID = uuid.Nil
			x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{a}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.associazioni_decise[0].componente_id"},
		{"confermata senza documento", func(x *ancoraggio.ContestoStrutturale) {
			a := buona
			a.DocumentoID = nil
			x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{a}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.associazioni_decise[0].documento_id"},
		{"ripetuta", func(x *ancoraggio.ContestoStrutturale) {
			x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{buona, buona}
		}, evidenze.CodiceDocumentoIDRipetuto, "contesto.associazioni_decise[1]"},
		{"lettura senza decisione", func(x *ancoraggio.ContestoStrutturale) {
			x.LettureDecise = map[string]motorea.LetturaForma{ancoraggio.RifComponente(idComp2Ric): lettura}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.letture_decise[" + ancoraggio.RifComponente(idComp2Ric) + "]"},
		{"lettura con una chiave che non è un componente", func(x *ancoraggio.ContestoStrutturale) {
			x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp2Ric, "7120107A", "3")}
			x.LettureDecise = map[string]motorea.LetturaForma{idComp2Ric.String(): lettura}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.letture_decise[" + idComp2Ric.String() + "]"},
		{"lettura di una decisione su un documento", func(x *ancoraggio.ContestoStrutturale) {
			d := decisioneRic(idComp2Ric, "7120107A", "3")
			d.Oggetto = ancoraggio.OggettoDecisioneDocumento
			x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{d}
			x.LettureDecise = map[string]motorea.LetturaForma{ancoraggio.RifComponente(idComp2Ric): lettura}
		}, evidenze.CodiceDocumentoRiferimentoPendente, "contesto.letture_decise[" + ancoraggio.RifComponente(idComp2Ric) + "]"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			_, err := ancoraggio.ProponiAncoraggi([]ancoraggio.FileInterpretato{sc.step, pdf}, []ancoraggio.ProdottoRichiesto{sc.tg}, sc.ctx(c.cambia))
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) {
				t.Fatalf("errore %v, atteso un errore di contratto", err)
			}
			trovato := false
			for _, d := range ec.Diagnostiche {
				trovato = trovato || (d.Codice == c.codice && d.Percorso == c.percorso)
			}
			if !trovato {
				t.Errorf("atteso %s su %s, avuto %+v", c.codice, c.percorso, ec.Diagnostiche)
			}
		})
	}
	ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
		x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp2Ric, "7120107A", "3")}
		x.LettureDecise = map[string]motorea.LetturaForma{ancoraggio.RifComponente(idComp2Ric): lettura}
		x.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{buona, {AllegatoID: pdf.AllegatoID, ComponenteID: idComp2Ric, Origine: ancoraggio.OrigineManuale}}
	})
	if _, err := ancoraggio.ProponiAncoraggi([]ancoraggio.FileInterpretato{sc.step, pdf}, []ancoraggio.ProdottoRichiesto{sc.tg}, ctx); err != nil {
		t.Errorf("gli ingressi buoni: %v", err)
	}
}

// TestIValoriDellaRiconciliazione: i valori delle costanti e i campi dei tipi della riconciliazione (contratto §2.2; le
// letture T-B4-31…T-B4-36).
func TestIValoriDellaRiconciliazione(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{ancoraggio.CodiceCompletamentoDocumentale, "ancoraggio.completamento_documentale"},
		{ancoraggio.CodiceCorrezioneDocumentale, "ancoraggio.correzione_documentale"},
		{ancoraggio.ParteDiscordanzaCodice, "codice"},
		{ancoraggio.ParteDiscordanzaRevisione, "revisione"},
		{ancoraggio.EffettoDiscordanzaConflitto, "conflitto"},
		{ancoraggio.EffettoDiscordanzaIndicatore, "indicatore"},
		{ancoraggio.MotivoDiscordanzaEvidenzaNuova, "evidenza_nuova"},
		{ancoraggio.MotivoDiscordanzaEvidenzaVista, "evidenza_vista"},
		{ancoraggio.MotivoDiscordanzaCodiceConfermato, "codice_confermato"},
		{ancoraggio.MotivoDiscordanzaCodiceManuale, "codice_manuale"},
		{ancoraggio.MotivoDiscordanzaRevisioneRegistrata, "revisione_registrata"},
		{ancoraggio.MotivoDocumentaleCartiglioNonLetto, "cartiglio_non_letto"},
		{ancoraggio.MotivoDocumentaleNodoSenzaIdentita, "nodo_senza_identita"},
		{ancoraggio.MotivoDocumentaleAltroNamespace, "cartiglio_altro_namespace"},
		{ancoraggio.MotivoDocumentaleCartiglioNonCompleto, "cartiglio_non_completo"},
		{ancoraggio.MotivoDocumentaleBaseNonConfrontabile, "base_non_confrontabile"},
		{ancoraggio.MotivoDocumentaleRevisioneAssente, "revisione_cartiglio_assente"},
		{ancoraggio.MotivoDocumentaleRevisioneAmbigua, "revisione_cartiglio_ambigua"},
	} {
		if c.got != c.want {
			t.Errorf("valore %q, il contratto dice %q", c.got, c.want)
		}
	}
	campi := func(v any) string {
		tp := reflect.TypeOf(v)
		var out []string
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			out = append(out, f.Name+":"+tag)
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{ancoraggio.AssociazioneDecisa{}, "AllegatoID:allegato_id DocumentoID:documento_id ComponenteID:componente_id Origine:origine"},
		{ancoraggio.DiscordanzaDecisione{}, "Parte:parte Effetto:effetto Motivo:motivo Decisione:decisione DecisaDa:decisa_da DecisaIl:decisa_il Evidenza:evidenza " +
			"RevisioneInferiore:revisione_inferiore"},
	} {
		if got := campi(c.tipo); got != c.campi {
			t.Errorf("%T: campi %s, il contratto dice %s", c.tipo, got, c.campi)
		}
	}
}

// ---- i ritocchi finali di B4 ----

// TestLaRevisioneDelCodiceManuale (T-B4-34 corretta; R95 A, T-B0-33): la revisione che l'operatore ha scritto sulla
// riga aperta, contraddetta dal cartiglio, è un conflitto con il motivo codice_manuale, non un indicatore: l'ha scritta
// l'operatore, è una decisione. RevisioneInferiore vale anche qui. La stessa revisione non discorda; un codice manuale
// senza revisione non discorda con la revisione del cartiglio. R97 B (l'indicatore revisione_registrata) resta solo per
// componente.rev del legacy.
func TestLaRevisioneDelCodiceManuale(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	for i, c := range []struct {
		rev       *string
		cartiglio string
		conflitto bool
		inferiore bool
	}{
		{testoS("1"), "7120101A2", true, false},
		{testoS("3"), "7120101A2", true, true},
		{testoS("2"), "7120101A2", false, false},
		{nil, "7120101A2", false, false},
	} {
		manuale, lm := "7120101A", letturaC(t, m, "nodo_step.id", "7120101A")
		riga := rigaAperta(uidS(0x9b8), idStepRic, shaStepRic, "#2")
		riga.CodiceManuale, riga.RevManuale, riga.LetturaManuale = &manuale, c.rev, &lm
		ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) { x.Proposto = []ancoraggio.RigaPropostaLegacy{riga} })
		pdf := pdfRic(t, m, 0x9b9+i, "disegno.pdf", c.cartiglio)
		n := nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#2"))
		d := unaDiscordanza(t, documentaleDi(t, n, pdf.AllegatoID))
		switch {
		case !c.conflitto && d != nil:
			t.Errorf("revisione manuale %s: nessuna discordanza attesa, avuta %+v", testoP(c.rev), d)
		case c.conflitto && (d == nil || d.Parte != ancoraggio.ParteDiscordanzaRevisione || d.Effetto != ancoraggio.EffettoDiscordanzaConflitto ||
			d.Motivo != ancoraggio.MotivoDiscordanzaCodiceManuale || d.RevisioneInferiore != c.inferiore || d.Decisione.Origine != ancoraggio.OrigineManuale):
			t.Errorf("revisione manuale %s contro il cartiglio: %+v, attesi conflitto, codice_manuale, inferiore %v", testoP(c.rev), d, c.inferiore)
		}
		if n.Codice.Manuale == nil || testoP(n.Codice.Manuale.Rev) != testoP(c.rev) {
			t.Errorf("la decisione manuale resta il valore corrente: %+v", n.Codice.Manuale)
		}
	}
}

// TestLaRadiceSceltaPortaIlCodiceDelProdotto (T-B4-38; R76 A, T-B4-23; il caso t2″ del workflow con il prodotto
// rinominato): sulla BOM di lavoro la radice scelta con il gesto 3 rappresenta il prodotto, e la sua catena porta
// accanto il codice confermato del componente del target (rinominato in 7120109A nel legacy): il cartiglio del 2D del
// prodotto completa lo STEP e contraddice la decisione, con il conflitto sul codice; la decisione resta, la Decisione
// della radice resta nil. Con la DecisioneIdentita sul prodotto (e la lettura del codice deciso) lo stato è confermata
// e il conflitto ha l'evidenza nuova. Sulla struttura candidata niente. Con la radice che ha la base del prodotto il
// candidato del 2D è base_del_target, senza il confronto con la radice scelta, che vale solo senza la base (T-B4-25).
func TestLaRadiceSceltaPortaIlCodiceDelProdotto(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "7120100A_1.stp")
	bom := sc.tg
	bom.FonteConfermata = fonteR59(shaStepRic, "#1", uuidP(idStepRic))
	rinominato := func(c *ancoraggio.ContestoStrutturale) {
		c.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idProdRic, "7120109A", nil)}
	}
	pdf := pdfRic(t, m, 0x9e5, "disegno-acme-telaio.pdf", "7120100A1")
	radice := func(tg ancoraggio.ProdottoRichiesto, ctx ancoraggio.ContestoStrutturale) (ancoraggio.NodoProposto, ancoraggio.EsitoAncoraggi) {
		t.Helper()
		e := ancoraA(t, []ancoraggio.FileInterpretato{sc.step, pdf}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
		return nodoRic(t, e, rRic("#1")), e
	}

	t.Run("t2″ sulla BOM di lavoro: il prodotto rinominato in X", func(t *testing.T) {
		r, e := radice(bom, sc.ctx(rinominato))
		if e.Strutture[0].Stato != ancoraggio.StatoBOMDiLavoroProposta || r.Decisione != nil || r.Codice.Confermato == nil ||
			r.Codice.Confermato.Codice != "7120109A" || *r.Codice.Confermato.ComponenteID != idProdRic || r.Codice.Identita.StatoRevisione == ancoraggio.StatoRevisioneConfermata {
			t.Fatalf("la radice scelta senza il codice del prodotto accanto, o con una Decisione: %+v", r)
		}
		cd := documentaleDi(t, r, pdf.AllegatoID)
		d := unaDiscordanza(t, cd)
		if cd.Esito != ancoraggio.RiconciliazioneCompletamentoProposto || d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice ||
			d.Effetto != ancoraggio.EffettoDiscordanzaConflitto || d.Motivo != ancoraggio.MotivoDiscordanzaCodiceConfermato || d.Decisione.Codice != "7120109A" {
			t.Errorf("il conflitto fra il cartiglio del prodotto e la decisione: %+v, %+v", cd, d)
		}
		a := ancoraggioDi(t, e, pdf.AllegatoID)
		if len(a.Candidati) != 1 || a.Candidati[0].Motivo != ancoraggio.MotivoCandidatoBaseDelTarget {
			t.Fatalf("il 2D del prodotto: %+v", a.Candidati)
		}
		for _, x := range a.Candidati[0].Dimensioni {
			if x.Rif == rRic("#1") {
				t.Errorf("con la radice che ha la base del prodotto niente confronto con la radice scelta (T-B4-25): %+v", x)
			}
		}
	})

	t.Run("la decisione tracciata sul prodotto", func(t *testing.T) {
		r, _ := radice(bom, sc.ctx(func(c *ancoraggio.ContestoStrutturale) {
			rinominato(c)
			c.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idProdRic, "7120108A", "1")}
			c.LettureDecise = map[string]motorea.LetturaForma{ancoraggio.RifComponente(idProdRic): letturaC(t, m, "nodo_step.id", "7120108A")}
		}))
		d := unaDiscordanza(t, documentaleDi(t, r, pdf.AllegatoID))
		if r.Codice.Identita.StatoRevisione != ancoraggio.StatoRevisioneConfermata || r.Codice.Confermato.Codice != "7120108A" ||
			r.Codice.Confermato.Base.Normalizzata != "7120108" || d == nil || d.Parte != ancoraggio.ParteDiscordanzaCodice || d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaNuova {
			t.Errorf("la decisione tracciata sul prodotto: %+v, %+v", r.Codice.Confermato, d)
		}
	})

	t.Run("sulla struttura candidata niente", func(t *testing.T) {
		r, e := radice(sc.tg, sc.ctx(rinominato))
		if e.Strutture[0].Stato != ancoraggio.StatoStrutturaCandidata || r.Codice.Confermato != nil || len(documentaleDi(t, r, pdf.AllegatoID).Discordanze) != 0 {
			t.Errorf("la struttura candidata non lega la radice al prodotto (R59 A): %+v", r.Codice)
		}
	})
}

// TestUnDisegnoIncertoNonFaConflitto (T-B4-39; R95 A, T-E1R-08, PO-23): lo STEP ha il pezzo 7120105A nella revisione 1 e
// nella 2, due nodi; il nodo della revisione 2 è un componente con la decisione tracciata sulla revisione 2. Un PDF
// «7120105A1» solo proposto è ambiguo fra i due nodi: sul nodo deciso la discordanza con la decisione è un indicatore,
// mai un conflitto, perché il disegno forse non è suo; il codice documentale dice l'associazione del file (ambiguo).
// Lo stesso PDF confermato sul componente resta un conflitto, come un candidato unico (PO-23).
func TestUnDisegnoIncertoNonFaConflitto(t *testing.T) {
	m := motoreCatena(t)
	sha, idStep, comp := fmt.Sprintf("%064x", 0x9a5), uidS(0x9a5), uidS(0x9a6)
	s := stepR(t, m, idStep, "telaio-acme.stp", sha, []nodoR{{"#1", "7120100A", "TELAIO", ""}, {"#2", "7120105A_1", "PIASTRA", "1"}, {"#3", "7120105A_2", "PIASTRA", "2"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}})
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	pdf := pdfRic(t, m, 0x9a7, "piastra-acme.pdf", "7120105A1")
	doc := uidS(0x9a8)
	ctx := func(decisa bool) ancoraggio.ContestoStrutturale {
		c := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
			Confermato:        []ancoraggio.ComponenteDeciso{componenteC(t, m, comp, "7120105A", nil)},
			Decise:            []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x9a9), idStep, sha, "#3", ancoraggio.StatoRigaConfermata, uuidP(comp), uuidP(idUtRic))},
			DecisioniIdentita: []ancoraggio.DecisioneIdentita{decisioneRic(comp, "7120105A", "2")}}
		if decisa {
			c.AssociazioniDecise = []ancoraggio.AssociazioneDecisa{{AllegatoID: pdf.AllegatoID, DocumentoID: &doc, ComponenteID: comp, Origine: ancoraggio.OrigineConfermato}}
		}
		return c
	}
	t.Run("solo proposto e discordante: un indicatore", func(t *testing.T) {
		// Il nome del PDF dice la 3 e il cartiglio la 4: il file è discordante (T-B0-35), e la decisione tracciata sulla 3
		// del pezzo 7120102A non ha un conflitto da un disegno che forse non è suo.
		sc := nuovaScenaRic(t, m, "telaio-acme.stp")
		ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
			x.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp2Ric, "7120102A", nil)}
			x.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9af, "#3", idComp2Ric)}
			x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp2Ric, "7120102A", "3")}
		})
		disc := pdfRic(t, m, 0x9ae, "7120102A_3.pdf", "7120102A4")
		e := sc.ancora(t, ctx, disc)
		cd := documentaleDi(t, nodoRic(t, e, rRic("#3")), disc.AllegatoID)
		d := unaDiscordanza(t, cd)
		if ancoraggioDi(t, e, disc.AllegatoID).Associazione != ancoraggio.AssociazioneDiscordante || cd.Associazione != ancoraggio.AssociazioneDiscordante ||
			d == nil || d.Effetto != ancoraggio.EffettoDiscordanzaIndicatore || d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaNuova {
			t.Errorf("il PDF discordante solo proposto: %+v, la discordanza %+v", cd, d)
		}
	})
	for _, c := range []struct {
		nome    string
		decisa  bool
		effetto string
	}{
		{"solo proposto e ambiguo: un indicatore", false, ancoraggio.EffettoDiscordanzaIndicatore},
		{"confermato sul componente: un conflitto", true, ancoraggio.EffettoDiscordanzaConflitto},
	} {
		t.Run(c.nome, func(t *testing.T) {
			e := ancoraA(t, []ancoraggio.FileInterpretato{pdf}, []ancoraggio.ProdottoRichiesto{tg}, ctx(c.decisa))
			if a := ancoraggioDi(t, e, pdf.AllegatoID); a.Associazione != ancoraggio.AssociazioneAmbiguo {
				t.Fatalf("il PDF fra i due nodi della stessa base: %+v", a)
			}
			n3 := nodoC(t, e.Strutture[0], ancoraggio.RifNodo(sha, "#3"))
			cd := documentaleDi(t, n3, pdf.AllegatoID)
			d := unaDiscordanza(t, cd)
			if cd.Associazione != ancoraggio.AssociazioneAmbiguo || d == nil || d.Parte != ancoraggio.ParteDiscordanzaRevisione || d.Effetto != c.effetto ||
				d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaNuova {
				t.Errorf("%s: %+v, la discordanza %+v", c.nome, cd, d)
			}
			if n2 := nodoC(t, e.Strutture[0], ancoraggio.RifNodo(sha, "#2")); len(documentaleDi(t, n2, pdf.AllegatoID).Discordanze) != 0 {
				t.Errorf("il nodo senza decisione non ha discordanze: %+v", n2.Codice.Documentale)
			}
		})
	}
}

// TestDueDecisioniDueSegnali (T-B4-40; T-B4-38, T-B0-33, R95 A): sulla BOM di lavoro la radice scelta porta accanto il
// codice confermato del prodotto e il codice manuale della sua riga aperta corretta dall'operatore: sono due decisioni,
// e ogni contraddizione del cartiglio dà il suo segnale, in ordine (confermato, poi manuale). Un cartiglio che
// contraddice solo il manuale dà il segnale del manuale; uno che li contraddice tutti e due ne dà due.
func TestDueDecisioniDueSegnali(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "7120100A_1.stp")
	bom := sc.tg
	bom.FonteConfermata = fonteR59(shaStepRic, "#1", uuidP(idStepRic))
	manuale, lm := "7120109A", letturaC(t, m, "nodo_step.id", "7120109A")
	riga := rigaAperta(uidS(0x9ea), idStepRic, shaStepRic, "#1")
	riga.CodiceManuale, riga.LetturaManuale = &manuale, &lm
	pdf := pdfRic(t, m, 0x9eb, "disegno-acme-telaio.pdf", "7120100A1")
	for _, c := range []struct {
		prodotto string
		origini  []ancoraggio.OrigineDato
	}{
		{"7120100A", []ancoraggio.OrigineDato{ancoraggio.OrigineManuale}},
		{"7120108A", []ancoraggio.OrigineDato{ancoraggio.OrigineConfermato, ancoraggio.OrigineManuale}},
	} {
		ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
			x.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idProdRic, c.prodotto, nil)}
			x.Proposto = []ancoraggio.RigaPropostaLegacy{riga}
		})
		e := ancoraA(t, []ancoraggio.FileInterpretato{sc.step, pdf}, []ancoraggio.ProdottoRichiesto{bom}, ctx)
		r := nodoRic(t, e, rRic("#1"))
		if r.Codice.Confermato == nil || r.Codice.Manuale == nil {
			t.Fatalf("la radice scelta con le due decisioni: %+v", r.Codice)
		}
		var origini []ancoraggio.OrigineDato
		for _, d := range documentaleDi(t, r, pdf.AllegatoID).Discordanze {
			origini = append(origini, d.Decisione.Origine)
			if d.Parte != ancoraggio.ParteDiscordanzaCodice || d.Effetto != ancoraggio.EffettoDiscordanzaConflitto {
				t.Errorf("prodotto %s: %+v", c.prodotto, d)
			}
		}
		if fmt.Sprint(origini) != fmt.Sprint(c.origini) {
			t.Errorf("prodotto %s e manuale %s contro il cartiglio 7120100A1: segnali %v, attesi %v", c.prodotto, manuale, origini, c.origini)
		}
	}

	// Le due revisioni decise, la registrata del prodotto 3 e la manuale 1, contro il cartiglio con la 2: due segnali
	// sulla revisione (l'indicatore di R97 B e il conflitto del manuale), e la revisione del cartiglio è minore di una
	// delle due: la correzione lo dice (T-E1-20).
	stesso, lms := "7120100A", letturaC(t, m, "nodo_step.id", "7120100A")
	rigaRev := riga
	rigaRev.CodiceManuale, rigaRev.RevManuale, rigaRev.LetturaManuale = &stesso, testoS("1"), &lms
	ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
		x.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idProdRic, "7120100A", testoS("3"))}
		x.Proposto = []ancoraggio.RigaPropostaLegacy{rigaRev}
	})
	pdf2 := pdfRic(t, m, 0x9ee, "disegno-acme-telaio.pdf", "7120100A2")
	cd := documentaleDi(t, nodoRic(t, ancoraA(t, []ancoraggio.FileInterpretato{sc.step, pdf2}, []ancoraggio.ProdottoRichiesto{bom}, ctx), rRic("#1")), pdf2.AllegatoID)
	if len(cd.Discordanze) != 2 || cd.Correzione == nil || !cd.Correzione.RevisioneInferiore ||
		cd.Discordanze[0].Effetto != ancoraggio.EffettoDiscordanzaIndicatore || !cd.Discordanze[0].RevisioneInferiore ||
		cd.Discordanze[1].Effetto != ancoraggio.EffettoDiscordanzaConflitto || cd.Discordanze[1].RevisioneInferiore {
		t.Errorf("le due revisioni decise contro il cartiglio con la 2: %+v, la correzione %+v", cd.Discordanze, cd.Correzione)
	}
}

// TestLaCoppiaDelCartiglioDalTestoDelCampo (T-B4-22; T-E1R-08): il campo del cartiglio è più lungo del codice letto
// («DIS. 7120101A2 ACME»): il codice documentale porta il testo grezzo del campo (Originale), la correzione il codice
// letto, e la coppia dell'evidenza viene dal testo del campo, non dalla lettura. Così la decisione tracciata che aveva
// visto quel campo lo riconosce: indicatore con l'evidenza vista, non un conflitto.
func TestLaCoppiaDelCartiglioDalTestoDelCampo(t *testing.T) {
	m := motoreCatena(t)
	sc := nuovaScenaRic(t, m, "telaio-acme.stp")
	campo := "DIS. 7120101A2 ACME"
	ctx := sc.ctx(func(x *ancoraggio.ContestoStrutturale) {
		x.Confermato = []ancoraggio.ComponenteDeciso{componenteC(t, m, idComp1Ric, "7120101A", nil)}
		x.Decise = []ancoraggio.RigaDecisaLegacy{decisaRic(0x9ec, "#2", idComp1Ric)}
		x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisioneRic(idComp1Ric, "7120101A", "1", ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, campo))}
	})
	pdf := pdfRic(t, m, 0x9ed, "disegno.pdf", campo)
	cd := documentaleDi(t, nodoRic(t, sc.ancora(t, ctx, pdf), rRic("#2")), pdf.AllegatoID)
	d := unaDiscordanza(t, cd)
	if cd.Originale != campo || cd.Correzione == nil || cd.Correzione.Codice != "7120101A2" || d == nil || d.Evidenza.Valore != campo ||
		d.Effetto != ancoraggio.EffettoDiscordanzaIndicatore || d.Motivo != ancoraggio.MotivoDiscordanzaEvidenzaVista {
		t.Errorf("il campo più lungo del codice: %+v, la discordanza %+v", cd, d)
	}
}
