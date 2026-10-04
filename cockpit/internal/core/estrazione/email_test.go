// L1 — l'adattatore della mail (A1b-16, A1b-25; piano A, 5.4.5, «Mappatura mail»): oltre ai golden di F-MAIL-1…6
// e F-MAIL-4b (golden_test.go), le proprietà della mappatura che un golden da solo non dice: l'inoltro senza
// confine nel corpo ha la storia non separabile e nessuna promozione (R48 A); i segmenti coprono il corpo senza
// buchi, sui byte originali; i livelli della storia hanno il tipo della riga che li apre (R27 c) e un tetto
// (maxLivelliStoria); una cella è esatta solo quando coincide con le sue righe, ed è una sola evidenza (R28 b);
// le tabelle che non si agganciano in un segmento solo non danno unità; i limiti della vista sono gli stessi di
// lettura.
//
// Tutti i dati sono sintetici (ACME, acme.example, fornitore.example, codici di fantasia, l'intestazione
// inventata «Q.TA», UUID 00000000-0000-4000-8000-0000000000nn): il repository è pubblico.
package estrazione

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/lettura"
)

// mailACME: un messaggio sintetico con oggetto, corpo e HTML dati (nil = assente).
func mailACME(oggetto, corpo, html *string) fotorfq.Messaggio {
	return fotorfq.Messaggio{
		ID:                uuidDi(0x60),
		ConversazioneID:   uuidDi(0xc0),
		Canale:            "outlook",
		Direzione:         "entrata",
		DataEvento:        time.Date(2026, 10, 1, 7, 30, 0, 0, time.UTC),
		MittenteIndirizzo: "acquisti@acme.example",
		Oggetto:           oggetto,
		CorpoTesto:        corpo,
		CorpoHTML:         html,
	}
}

func daMessaggio(t *testing.T, m fotorfq.Messaggio) evidenze.DocumentoEvidenze {
	t.Helper()
	d, err := DaMessaggio(m)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func segmentoDi(d evidenze.DocumentoEvidenze, id string) (evidenze.Segmento, bool) {
	for _, s := range d.Segmenti {
		if s.ID == id {
			return s, true
		}
	}
	return evidenze.Segmento{}, false
}

func ingressoMail(t *testing.T, nome string) fotorfq.Messaggio {
	t.Helper()
	return leggiIngressoMessaggio(t, casoGolden{tipo: "email", nome: nome})
}

// TestUnInoltroSenzaConfineHaLaStoriaNonSeparabile (A1b-25, nella parte dell'adattatore; R48 A). Con l'oggetto
// «I: …» e nessun confine nel corpo (F-MAIL-4): un solo segmento s:storia:1, tipo inoltro, su tutto il corpo;
// s:corrente vuoto, con l'oggetto; nessuna unità del corpo corrente; le celle della tabella hanno il selettore
// storia e le loro righe stanno in s:storia:1; email.storia_non_separabile con il motivo dell'indizio. Lo stesso
// corpo con l'oggetto senza prefisso (F-MAIL-4b): nessuna storia, corpo corrente, segmentazione disponibile con
// la copertura dichiarata. L'uso dei segmenti (menzioni o richiesta) lo prova A1b.10 con Interpreta.
func TestUnInoltroSenzaConfineHaLaStoriaNonSeparabile(t *testing.T) {
	t.Run("con il prefisso d'inoltro", func(t *testing.T) {
		m := ingressoMail(t, "mail_04_inoltro_tabella")
		d := daMessaggio(t, m)
		corpo := *m.CorpoTesto

		storia, ok := segmentoDi(d, "s:storia:1")
		if !ok || storia.Tipo != evidenze.SegmentoInoltro || storia.MessaggioLogicoID != "m1" || storia.Origine != origineIgnota ||
			storia.Posizione.Testo.Intervallo != (evidenze.Intervallo{Inizio: 0, Fine: len(corpo)}) {
			t.Errorf("s:storia:1 %+v: atteso un inoltro su tutto il corpo", storia)
		}
		if len(d.Segmenti) != 2 {
			t.Errorf("%d segmenti, attesi s:corrente e s:storia:1", len(d.Segmenti))
		}
		corrente, _ := segmentoDi(d, idSegCorrente)
		if corrente.Posizione.Testo.Intervallo != (evidenze.Intervallo{}) {
			t.Errorf("s:corrente %+v: atteso vuoto all'inizio del corpo", corrente.Posizione)
		}
		if u, ok := unitaDi(d, idUnitaOggetto); !ok || u.SegmentoID != idSegCorrente {
			t.Errorf("l'oggetto %+v non sta in s:corrente", u)
		}
		for _, u := range d.Unita {
			if u.Selettore.Contesto == evidenze.ContestoCorpo {
				t.Errorf("l'unità %s è corpo corrente: la storia non separabile non si promuove", u.ID)
			}
			if strings.HasPrefix(u.ID, "u:tab:") && u.Selettore.Contesto != evidenze.ContestoStoria {
				t.Errorf("la cella %s ha il selettore %s, attesa storia", u.ID, u.Selettore)
			}
		}
		for _, e := range d.Entita {
			if e.SegmentoID != "s:storia:1" {
				t.Errorf("la riga %s sta nel segmento %q, attesa s:storia:1", e.ID, e.SegmentoID)
			}
		}
		seg, _ := capacitaDi(d, capSegmentazione)
		if seg.Stato != statoParziale || seg.Motivo != motivoNonSeparabileInoltro {
			t.Errorf("segmentazione %+v, attesa parziale con il motivo dell'indizio", seg)
		}
		if !slices.Contains(codiciDi(d), CodiceEmailStoriaNonSeparabile) {
			t.Errorf("diagnostiche %v, attesa email.storia_non_separabile", codiciDi(d))
		}
	})

	t.Run("senza il prefisso", func(t *testing.T) {
		d := daMessaggio(t, ingressoMail(t, "mail_04b_senza_prefisso"))
		if len(d.Segmenti) != 1 || d.Segmenti[0].ID != idSegCorrente {
			t.Errorf("segmenti %+v: atteso il solo s:corrente", d.Segmenti)
		}
		if u, ok := unitaDi(d, "u:corpo:s:corrente"); !ok || u.Selettore.Contesto != evidenze.ContestoCorpo {
			t.Errorf("unità del corpo corrente %+v", u)
		}
		seg, _ := capacitaDi(d, capSegmentazione)
		if seg.Stato != statoDisponibile || seg.Motivo != motivoCopertura {
			t.Errorf("segmentazione %+v, attesa disponibile con la copertura dei riconoscitori", seg)
		}
		if slices.Contains(codiciDi(d), CodiceEmailStoriaNonSeparabile) {
			t.Error("senza indizio la storia non separabile non nasce")
		}
	})

	t.Run("un corpo vuoto non ha storia, anche con il prefisso", func(t *testing.T) {
		d := daMessaggio(t, mailACME(ptr("I: ACME1111"), ptr(""), nil))
		if _, ok := segmentoDi(d, "s:storia:1"); ok {
			t.Error("un corpo vuoto ha una storia")
		}
	})
}

// TestSenzaCommentoLaStoriaResta (E-22, il primo dei due casi di 5.4.5): un inoltro con il confine e senza
// commento (F-MAIL-3) ha la storia non separabile con il motivo del confine, s:corrente vuoto e nessuna unità
// del corpo corrente; i livelli sono l'inoltro e poi la citazione più vecchia.
func TestSenzaCommentoLaStoriaResta(t *testing.T) {
	d := daMessaggio(t, ingressoMail(t, "mail_03_inoltro"))
	seg, _ := capacitaDi(d, capSegmentazione)
	if seg.Stato != statoParziale || seg.Motivo != motivoNonSeparabileConfine {
		t.Errorf("segmentazione %+v, attesa parziale con il motivo del confine", seg)
	}
	if _, ok := unitaDi(d, "u:corpo:s:corrente"); ok {
		t.Error("la storia non separabile è diventata corpo corrente")
	}
	var tipi []string
	for _, s := range d.Segmenti {
		tipi = append(tipi, s.ID+"="+s.Tipo)
	}
	if strings.Join(tipi, " ") != "s:corrente=corrente s:storia:1=inoltro s:storia:2=citazione" {
		t.Errorf("segmenti %v", tipi)
	}
	if s, _ := segmentoDi(d, "s:storia:2"); s.PadreID != "s:storia:1" || s.MessaggioLogicoID != "m2" {
		t.Errorf("il secondo livello %+v: padre s:storia:1, messaggio m2", s)
	}
	if u, _ := unitaDi(d, "u:storia:s:storia:2"); !strings.Contains(u.Testo, "ACME2222") || strings.Contains(u.Testo, "ACME1111") {
		t.Errorf("la citazione più vecchia %q: solo il suo testo", u.Testo)
	}
}

// TestISegmentiCopronoIlCorpoSuiByteOriginali: su ogni ingresso dei golden, i segmenti del corpo lo coprono
// senza buchi né sovrapposizioni, in ordine (s:corrente, poi s:storia:1, 2, …), e ogni unità del corpo è il suo
// segmento senza gli spazi ai bordi, misurata sul testo originale (CRLF, NBSP, emoji compresi: A-C03).
func TestISegmentiCopronoIlCorpoSuiByteOriginali(t *testing.T) {
	for _, c := range casiGolden {
		if c.tipo != "email" {
			continue
		}
		t.Run(c.nome, func(t *testing.T) {
			d := daMessaggio(t, leggiIngressoMessaggio(t, c))
			idCorpo := d.Segmenti[0].Posizione.Testo.TestoID
			var testo string
			for _, x := range d.Testi {
				if x.ID == idCorpo {
					testo = x.Testo
				}
			}
			fine := 0
			for k, s := range d.Segmenti {
				atteso := idSegCorrente
				if k > 0 {
					atteso = idStoria(k)
				}
				iv := s.Posizione.Testo.Intervallo
				if s.ID != atteso || iv.Inizio != fine || s.Posizione.Testo.TestoID != idCorpo {
					t.Errorf("segmento %d: %s da %d, atteso %s da %d", k, s.ID, iv.Inizio, atteso, fine)
				}
				fine = iv.Fine
				idUnita := "u:storia:" + s.ID
				if k == 0 {
					idUnita = "u:corpo:" + s.ID
				}
				if u, ok := unitaDi(d, idUnita); ok {
					if u.Testo != strings.TrimSpace(testo[iv.Inizio:iv.Fine]) || u.SegmentoID != s.ID {
						t.Errorf("unità %s: %q non è il suo segmento senza spazi ai bordi", u.ID, u.Testo)
					}
				} else if strings.TrimSpace(testo[iv.Inizio:iv.Fine]) != "" {
					t.Errorf("il segmento %s ha testo, ma non ha unità", s.ID)
				}
			}
			if fine != len(testo) {
				t.Errorf("i segmenti finiscono a %d, il corpo a %d", fine, len(testo))
			}
		})
	}
}

// TestLaStoriaHaUnTettoDiLivelli (5.4.3 punto 6): oltre maxLivelliStoria livelli, il resto resta nell'ultimo,
// che finisce alla fine del corpo, con email.livelli_oltre_limite e la capacità storia_annidata parziale.
func TestLaStoriaHaUnTettoDiLivelli(t *testing.T) {
	var b strings.Builder
	b.WriteString("Ecco la catena.\r\n\r\n")
	for i := 1; i <= maxLivelliStoria+2; i++ {
		fmt.Fprintf(&b, "Il giorno %d set 2026, alle ore 10:00, Ufficio ACME <acquisti@acme.example> ha scritto:\r\nMessaggio ACME%04d\r\n\r\n", i, i)
	}
	corpo := strings.TrimSpace(b.String())
	d := daMessaggio(t, mailACME(ptr("R: catena ACME"), &corpo, nil))
	if len(d.Segmenti) != 1+maxLivelliStoria {
		t.Fatalf("%d segmenti, attesi s:corrente e %d livelli", len(d.Segmenti), maxLivelliStoria)
	}
	ultimo, _ := segmentoDi(d, idStoria(maxLivelliStoria))
	if ultimo.Posizione.Testo.Intervallo.Fine != len(corpo) {
		t.Errorf("l'ultimo livello finisce a %d, il corpo a %d", ultimo.Posizione.Testo.Intervallo.Fine, len(corpo))
	}
	if u, _ := unitaDi(d, "u:storia:"+idStoria(maxLivelliStoria)); !strings.Contains(u.Testo, fmt.Sprintf("ACME%04d", maxLivelliStoria+2)) {
		t.Errorf("il resto della storia non è nell'ultimo livello: %q", u.Testo)
	}
	if !slices.Contains(codiciDi(d), CodiceEmailLivelliOltreLimite) {
		t.Errorf("diagnostiche %v, attesa email.livelli_oltre_limite", codiciDi(d))
	}
	if c, _ := capacitaDi(d, capStoriaAnnidata); c.Stato != statoParziale {
		t.Errorf("storia_annidata %+v, attesa parziale", c)
	}
}

// TestUnaCellaEUnaSolaEvidenza (R28 b; A1b-11 nella parte dell'adattatore): con una cella per riga del testo
// (F-MAIL-4b) ogni cella è esatta, con PosTabella.Esatto sul corpo_testo che ricostruisce il suo testo; con la
// tabella a TAB (F-MAIL-5a) le celle non coincidono e sono parziali, senza Esatto; con il testo ricavato
// dall'HTML (F-MAIL-5c) nessuna cella è esatta. Ogni cella ha una riga per entità, e nessun SegmentoID suo: la
// riga sta nel segmento (legami-1).
func TestUnaCellaEUnaSolaEvidenza(t *testing.T) {
	for _, c := range []struct {
		nome   string
		esatte bool
	}{
		{"mail_04b_senza_prefisso", true},
		{"mail_05a_tab_e_non_combacia", false},
		{"mail_05c_solo_html", false},
	} {
		t.Run(c.nome, func(t *testing.T) {
			m := ingressoMail(t, c.nome)
			d := daMessaggio(t, m)
			celle := 0
			for _, u := range d.Unita {
				if !strings.HasPrefix(u.ID, "u:tab:") {
					continue
				}
				celle++
				p := u.Posizione.Tabella
				if u.SegmentoID != "" || !strings.HasPrefix(u.EntitaID, "e:tab:") || p == nil {
					t.Errorf("cella %s: segmento %q, entità %q", u.ID, u.SegmentoID, u.EntitaID)
					continue
				}
				esatta := u.Qualita.Localizzazione == localizzazioneEsatta
				if esatta != c.esatte || (p.Esatto != nil) != c.esatte {
					t.Errorf("cella %s: localizzazione %s, Esatto %v", u.ID, u.Qualita.Localizzazione, p.Esatto)
				}
				if p.Esatto != nil && (*m.CorpoTesto)[p.Esatto.Inizio:p.Esatto.Fine] != u.Testo {
					t.Errorf("cella %s: l'intervallo esatto non ricostruisce %q", u.ID, u.Testo)
				}
			}
			if celle == 0 {
				t.Fatal("nessuna cella")
			}
		})
	}
}

// TestUnaTabellaFuoriDaUnSegmentoNonDaUnita (A1b-12 nella parte dell'adattatore): la tabella che il testo non
// dice e quella a cavallo del taglio non danno né entità né unità, hanno la loro diagnostica, e la capacità
// delle tabelle è parziale.
func TestUnaTabellaFuoriDaUnSegmentoNonDaUnita(t *testing.T) {
	for _, c := range []struct{ nome, codice string }{
		{"mail_05a_tab_e_non_combacia", CodiceEmailTabellaNonAgganciata},
		{"mail_05b_a_cavallo", CodiceEmailTabellaACavallo},
	} {
		t.Run(c.nome, func(t *testing.T) {
			d := daMessaggio(t, ingressoMail(t, c.nome))
			if !slices.Contains(codiciDi(d), c.codice) {
				t.Errorf("diagnostiche %v, attesa %s", codiciDi(d), c.codice)
			}
			for _, u := range d.Unita {
				if strings.HasPrefix(u.ID, "u:tab:2:") || c.codice == CodiceEmailTabellaACavallo && strings.HasPrefix(u.ID, "u:tab:") {
					t.Errorf("unità %s di una tabella che non si aggancia in un segmento", u.ID)
				}
			}
			if tb, _ := capacitaDi(d, capTabelle); tb.Stato != statoParziale {
				t.Errorf("capacità delle tabelle %+v, attesa parziale", tb)
			}
		})
	}
}

// TestILimitiDellaVistaSonoQuelliDiLettura: i limiti ripetuti qui per dire perché le tabelle mancano sono quelli
// di lettura; un HTML oltre il limite dà email.html_oltre_limite e le tabelle non disponibili; senza HTML le
// tabelle non sono disponibili, senza diagnostica.
func TestILimitiDellaVistaSonoQuelliDiLettura(t *testing.T) {
	if limiteHTMLMail != lettura.LimiteHTML || limiteTestoMail != lettura.LimiteTesto {
		t.Fatalf("limiti %d e %d, lettura ha %d e %d", limiteHTMLMail, limiteTestoMail, lettura.LimiteHTML, lettura.LimiteTesto)
	}
	lungo := "<table><tr><td>ACME1111</td><td>5</td></tr></table>" + strings.Repeat(" ", lettura.LimiteHTML)
	d := daMessaggio(t, mailACME(ptr("Elenco ACME"), ptr("ACME1111 5"), &lungo))
	if tb, _ := capacitaDi(d, capTabelle); tb.Stato != statoNonDisponibile || !slices.Contains(codiciDi(d), CodiceEmailHTMLOltreLimite) {
		t.Errorf("tabelle %+v, diagnostiche %v", tb, codiciDi(d))
	}
	d = daMessaggio(t, mailACME(ptr("Elenco ACME"), ptr("ACME1111 5"), nil))
	if tb, _ := capacitaDi(d, capTabelle); tb.Stato != statoNonDisponibile || len(codiciDi(d)) != 0 {
		t.Errorf("senza HTML: tabelle %+v, diagnostiche %v", tb, codiciDi(d))
	}
}

// TestLeCapacitaDellaMailSonoDichiarate (R27 a; parte 1 §13.4): firma e sezione tecnica non sono mai
// disponibili, perché non c'è un riconoscitore; la storia annidata sì. Un codice nella firma resta nel segmento
// in cui sta (F-MAIL-3: la firma dell'inoltro sta in s:storia:1).
func TestLeCapacitaDellaMailSonoDichiarate(t *testing.T) {
	d := daMessaggio(t, ingressoMail(t, "mail_03_inoltro"))
	for nome, stato := range map[string]string{capFirma: statoNonDisponibile, capSezioneTecnica: statoNonDisponibile, capStoriaAnnidata: statoDisponibile} {
		if c, ok := capacitaDi(d, nome); !ok || c.Stato != stato || c.Motivo == "" {
			t.Errorf("capacità %s %+v, attesa %s con il motivo", nome, c, stato)
		}
	}
	if u, _ := unitaDi(d, "u:storia:s:storia:1"); !strings.Contains(u.Testo, "ACME9999") {
		t.Errorf("la firma dell'inoltro non è nel suo segmento: %q", u.Testo)
	}
}

// TestLAssenzaDeiTestiSiDistingue (E-21): un oggetto assente e uno vuoto danno due riferimenti ai fatti diversi,
// e nessuno dei due dà l'unità dell'oggetto; il corpo assente senza HTML è un corpo vuoto, senza storia.
func TestLAssenzaDeiTestiSiDistingue(t *testing.T) {
	assente := daMessaggio(t, mailACME(nil, ptr("ACME1111"), nil))
	vuoto := daMessaggio(t, mailACME(ptr(""), ptr("ACME1111"), nil))
	if assente.Fonte.RiferimentoFatti.DigestTesti == vuoto.Fonte.RiferimentoFatti.DigestTesti {
		t.Error("oggetto assente e oggetto vuoto hanno la stessa impronta dei testi")
	}
	for _, d := range []evidenze.DocumentoEvidenze{assente, vuoto} {
		if _, ok := unitaDi(d, idUnitaOggetto); ok {
			t.Error("un oggetto assente o vuoto dà un'unità")
		}
	}
	senzaCorpo := daMessaggio(t, mailACME(ptr("I: ACME1111"), nil, nil))
	if len(senzaCorpo.Segmenti) != 1 || len(senzaCorpo.Unita) != 1 {
		t.Errorf("senza corpo: segmenti %+v, unità %d", senzaCorpo.Segmenti, len(senzaCorpo.Unita))
	}
}
