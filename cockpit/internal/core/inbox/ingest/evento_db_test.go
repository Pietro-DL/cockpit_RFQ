//go:build integrazione

// L4 — Smistamento M2 (A5.16.2): l'evento sull'ingest vero. Si salva nell'atto e fra i motivi, e non
// aggancia: non scrive thread_id, non scrive log, non cambia i candidati né la proposta di aggancio.
// Nomi e domini inventati (ACME, @acme.example), codici finti (7120001A).

package ingest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// conAllegato aggiunge a una mail un allegato con il nome dato.
func conAllegato(m worker.MessaggioIn, nome string) worker.MessaggioIn {
	ext := nome[strings.LastIndex(nome, ".")+1:]
	m.Allegati = append(m.Allegati, worker.AllegatoIn{Indice: len(m.Allegati) + 1, NomeFile: nome, Estensione: ext, Natura: "file", Bytes: 900000})
	return m
}

// firmaCandidati riduce i candidati di aggancio a ciò che l'ordinamento guarda: RFQ, regola, punteggio.
func firmaCandidati(k []db.ListCandidatiAggancioRow) []string {
	var out []string
	for _, x := range k {
		out = append(out, fmt.Sprintf("%s|%s|%d", x.ThreadID, x.Regola, x.Punteggio))
	}
	slices.Sort(out)
	return out
}

// 258 — l'evento si salva nell'atto e non aggancia. Tre risposte alla stessa mail della RFQ, identiche
// salvo il testo e il nome dello STEP: «aggiornato» con 7120001A_2.stp (REVISIONE_CAD chiaro), la stessa
// senza la parola (REVISIONE_CAD probabile), la prima emissione 7120001A_1.stp (ARRIVO_CAD). L'atto e i
// motivi «evento ·» cambiano; thread_id resta NULL, il log resta vuoto, e candidati, esito, RFQ proposta
// e confidenza sono gli stessi per tutte e tre. Una quarta con lo stesso contenuto e senza candidati ha
// lo stesso evento e un altro esito.
func TestLEventoSiSalvaNellAttoENonAggancia(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	th := b.rfq(acme, "RFQ 7120001", "7120001A")
	const m = "<m2-258-m@acme.example>"
	b.ingerisci(b.dalCliente(m, "CONV-258", "", "buyer@acme.example", "RFQ 7120001", "Richiesta d'offerta per 7120001A."))
	b.agganciaAMano(m, th)

	casi := []struct {
		chiave, corpo, file string
		atto, evento, forza string
	}{
		{"<m2-258-a@acme.example>", "Vi mandiamo il modello aggiornato.", "7120001A_2.stp", classificazione.AttoRevisioneDocumenti, classificazione.EventoRevisioneCAD, classificazione.ForzaChiaro},
		{"<m2-258-b@acme.example>", "Vi mandiamo il modello.", "7120001A_2.stp", classificazione.AttoRevisioneDocumenti, classificazione.EventoRevisioneCAD, classificazione.ForzaProbabile},
		{"<m2-258-c@acme.example>", "Vi mandiamo il modello.", "7120001A_1.stp", classificazione.AttoDocumentiAggiuntivi, classificazione.EventoArrivoCAD, classificazione.ForzaProbabile},
	}
	var primi []string
	var primo db.PropostaTriage
	for i, c := range casi {
		// conversazioni diverse: a parlare sono l'In-Reply-To e l'oggetto, gli stessi per tutte e tre
		r := conAllegato(b.dalCliente(c.chiave, "CONV-258-"+string(rune('A'+i)), "", "buyer@acme.example", "R: RFQ 7120001", c.corpo), c.file)
		r.InReplyTo, r.Riferimenti = m, []string{m}
		b.ingerisci(r)
		b.nessunLegame(c.chiave, "l'evento non aggancia")
		p, ok := b.proposta(b.messaggio(c.chiave).MessaggioID)
		if !ok {
			t.Fatalf("%s: nessuna proposta", c.chiave)
		}
		if p.Atto.String != c.atto {
			t.Errorf("%s: atto %q, atteso %q", c.chiave, p.Atto.String, c.atto)
		}
		var motivi []string
		_ = json.Unmarshal(p.Motivi, &motivi)
		testa := "evento · " + c.evento + " (" + c.forza + ")"
		if len(motivi) == 0 || !strings.HasPrefix(motivi[0], testa) {
			t.Errorf("%s: i motivi non cominciano con %q: %q", c.chiave, testa, motivi)
		}
		if c.file == "7120001A_2.stp" && !strings.Contains(strings.Join(motivi, " | "), "7120001A_2.stp porta la rev 2") {
			t.Errorf("%s: manca l'evidenza della rev nel nome: %q", c.chiave, motivi)
		}
		// l'evento salvato si rilegge dall'atto
		if ev := classificazione.EventoDa(string(b.messaggio(c.chiave).ControparteTipo), "entrata", p.Atto.String, string(p.Legame.LegameOperativo)); ev != c.evento {
			t.Errorf("%s: EventoDa rilegge %s, atteso %s", c.chiave, ev, c.evento)
		}
		firma := firmaCandidati(b.candidatiAggancio(c.chiave))
		if i == 0 {
			primi, primo = firma, p
			if len(firma) == 0 {
				t.Fatal("la scena non ha candidati: la prova non prova niente")
			}
			if p.Esito != db.EsitoTriageAggancia || !p.ThreadProposto.Valid || p.ThreadProposto.UUID != th.ThreadID {
				t.Fatalf("la proposta di aggancio della scena: %+v", p)
			}
			continue
		}
		if !slices.Equal(firma, primi) {
			t.Errorf("%s: i candidati cambiano con l'evento: %v contro %v", c.chiave, firma, primi)
		}
		if p.Esito != primo.Esito || p.ThreadProposto != primo.ThreadProposto || p.Confidenza != primo.Confidenza || p.Legame != primo.Legame {
			t.Errorf("%s: la proposta di aggancio cambia con l'evento: %s %v %d %v contro %s %v %d %v", c.chiave,
				p.Esito, p.ThreadProposto, p.Confidenza, p.Legame, primo.Esito, primo.ThreadProposto, primo.Confidenza, primo.Legame)
		}
	}

	// lo stesso contenuto della prima, senza nessun candidato: lo stesso evento, un altro esito
	const d = "<m2-258-d@acme.example>"
	b.ingerisci(conAllegato(b.dalCliente(d, "CONV-258-D", "", "buyer@acme.example", "R: modello", "Vi mandiamo il modello aggiornato."), "7120001A_2.stp"))
	b.nessunLegame(d, "l'evento non aggancia")
	if k := b.candidatiAggancio(d); len(k) != 0 {
		t.Fatalf("la mail senza candidati ne ha: %+v", k)
	}
	p, _ := b.proposta(b.messaggio(d).MessaggioID)
	if p.Atto.String != classificazione.AttoRevisioneDocumenti || p.Esito == db.EsitoTriageAggancia || p.ThreadProposto.Valid {
		t.Errorf("senza candidati: atto %q, esito %s, proposto %v", p.Atto.String, p.Esito, p.ThreadProposto)
	}
}
