//go:build integrazione

// L4 — il codice letto dal file nel passo 2 di S8, dal messaggio 46 [U] (21/10, corsia MOTIVO): il motivo scritto
// dall'utente si toglie da ogni widget. Lo STEP la cui vecchia lettura del file (documento_proposta.codice) non e' il codice
// del prodotto si conferma con S8 SENZA il motivo, con qualunque candidatura (prima, dal giro G39-bis, solo con la
// candidatura univoca del Motore A; gli altri casi finivano «parziale» con MOTIVO_OBBLIGATORIO nel passo 2): la conferma
// e' il gesto esplicito dell'utente, il documento prende il codice del prodotto, e la riga di S8 ha come motivo la traccia
// del gesto scritta dal server (TracciaS8), con le evidenze del Motore A quando la candidatura e' univoca. Il reinvio dello
// stesso corpo resta «gia' fatto» (la riga si riconosce confrontando il motivo con la traccia ESATTA che il server genera),
// lo stesso passo con un'altra traccia e' CHIAVE_RIUSATA; il motivo di un client di prima vale com'e'. Senza correzione
// del codice la riga resta senza motivo. Dati inventati (ACME).

package sorgente

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1M46S8IlCodiceLettoDiversoSenzaMotivo(t *testing.T) {
	evidenze := []propostabom.EvidenzaCandidatura{{Lettura: "l1", Selettore: "nome_file", CodiceLetto: "ACM-100", BaseLetta: "ACM-100",
		EsitoBase: "uguale", EsitoMarcatore: "compatibile"}}
	univoca := func(s scenaS8) *distintabozza.StepCandidati {
		return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto}},
			Univoche: map[uuid.UUID]uuid.UUID{s.stepA: s.prodotto}, Evidenze: map[uuid.UUID][]propostabom.EvidenzaCandidatura{s.stepA: evidenze}}
	}
	base := "gesto di S8P da «Conferma la sorgente»: lo STEP «ACM-1000.stp» è la sorgente di ACM-1000; codice letto dal file «ACM-100», il documento prende quello del prodotto"
	casi := []struct {
		nome      string
		letto     string // la vecchia lettura del nome del file (documento_proposta.codice)
		candidati func(s scenaS8) *distintabozza.StepCandidati
		motivo    string // il motivo di un client di prima, "" per la pagina di oggi
		atteso    string // il motivo della riga di S8 ("" = NULL)
	}{
		{"candidatura univoca del prodotto nel calcolo del motore", "ACM-100", univoca, "",
			base + "; candidatura univoca dello STEP del finito; evidenze: nome_file ACM-100 (base uguale, marcatore compatibile)"},
		{"candidatura univoca con il codice letto uguale a quello del prodotto: nessuna correzione, nessuna traccia", "ACM-1000", univoca, "", ""},
		{"candidatura non univoca del solo prodotto", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto}}}
		}, "", base},
		{"senza il riconoscimento del motore", "ACM-100", func(scenaS8) *distintabozza.StepCandidati { return nil }, "", base},
		{"il ripiego senza il motore", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: false, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto}},
				Univoche: map[uuid.UUID]uuid.UUID{s.stepA: s.prodotto}}
		}, "", base},
		{"candidato di due prodotti", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto, s.altroFinito}}}
		}, "", base},
		{"candidato univoco di un altro prodotto, con il motivo di un client di prima", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.altroFinito}},
				Univoche: map[uuid.UUID]uuid.UUID{s.stepA: s.altroFinito}}
		}, "lo STEP e' quello del prodotto", "lo STEP e' quello del prodotto"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			p, s := preparaS8(t)
			ctx := context.Background()
			// la vecchia lettura del nome del file: il prodotto e' ACM-1000, il file dice c.letto
			esegui(t, p, `UPDATE documento_proposta SET codice = $2 WHERE allegato_id = $1`, s.stepA, c.letto)
			reg := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
			prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: reg, StepCandidati: c.candidati(s)}}
			var extra map[string]any
			if c.motivo != "" {
				extra = map[string]any{"motivo": c.motivo}
			}
			corpo := corpoStep(t, s, uuid.New(), reg, extra)
			es, err := s8(t, p, s, s.stepA, corpo, prep)
			if err != nil {
				t.Fatalf("S8: %v", err)
			}
			if es.Esito != EsitoCompletata || len(es.Passi) != 3 || es.Passi[1].Esito != EsitoFatto || es.Bozza == nil {
				t.Fatalf("atteso S8 completata senza il motivo dell'utente, avuto %+v", es)
			}
			if n := conta(t, p, `SELECT count(*) FROM documento WHERE componente_id = $1 AND tipo = 'cad_3d' AND codice = 'ACM-1000' AND sha256 = $2`, s.prodotto, s.shaA); n != 1 {
				t.Fatalf("il documento dello STEP con il codice del prodotto: %d", n)
			}
			var motivo *string
			if err := p.QueryRow(ctx, `SELECT motivo FROM fonte_step_evento WHERE azione = 'conferma_documento' AND prodotto_id = $1`, s.prodotto).Scan(&motivo); err != nil {
				t.Fatalf("la riga di S8: %v", err)
			}
			if c.atteso == "" {
				if motivo != nil {
					t.Fatalf("senza correzione del codice la riga di S8 non ha un motivo del server: %q", *motivo)
				}
				return
			}
			if motivo == nil || *motivo != c.atteso {
				t.Fatalf("il motivo della riga di S8:\navuto  %v\natteso %q", stringa(motivo), c.atteso)
			}
			if c.motivo != "" {
				return
			}
			// il reinvio dello stesso corpo con la stessa chiave (la procedura rifatta): il passo 2 riconosce la traccia esatta
			// come il motivo di questo corpo, «gia' fatto», niente CHIAVE_RIUSATA e niente scritto
			prima := fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`), conta(t, p, `SELECT count(*) FROM fonte_step_evento`))
			r, imp, err := LeggiRichiestaS8(corpo)
			if err != nil {
				t.Fatal(err)
			}
			es2, err := ConfermaS8(ctx, p, s.thread, s.stepA, s.utente, r, imp, prep)
			if err != nil || !es2.Riproposto || es2.Esito != EsitoCompletata {
				t.Fatalf("la procedura rifatta con la stessa chiave: %+v %v", es2, err)
			}
			if dopo := fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`), conta(t, p, `SELECT count(*) FROM fonte_step_evento`)); dopo != prima {
				t.Fatalf("il reinvio ha scritto: %s -> %s", prima, dopo)
			}
			// il passo 2 con la stessa chiave: la traccia esatta e' «gia' fatto»; un'altra traccia (anche un suo pezzo, che
			// con il confronto per prefisso di prima passava) e' CHIAVE_RIUSATA
			cx, err := contestoInLettura(ctx, p, s.thread, s.stepA, r)
			if err != nil {
				t.Fatal(err)
			}
			rt, err := conTraccia(ctx, p, s.stepA, s.utente, r, cx, prep)
			if err != nil || rt.traccia != c.atteso {
				t.Fatalf("la traccia rigenerata: %q %v", rt.traccia, err)
			}
			if esito, _, _, err := passo2(ctx, p, s.thread, s.stepA, s.utente, rt); err != nil || esito != EsitoGiaFatto {
				t.Fatalf("il passo 2 rifatto con la stessa traccia: %s %v", esito, err)
			}
			pezzo := []rune(c.atteso)
			for _, altra := range []string{string(pezzo[:len(pezzo)-5]), c.atteso + "; in piu'"} {
				ra := rt
				ra.traccia = altra
				_, _, _, err := passo2(ctx, p, s.thread, s.stepA, s.utente, ra)
				codiceDi(t, err, distintabozza.CodiceChiaveRiusata, "")
			}
		})
	}
}

func stringa(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("%q", strings.TrimSpace(*s))
}
