//go:build integrazione

// L4 — G39-02 [U] nel passo 2 di S8 (difetto trovato dall'integrazione nel collaudo da dati azzerati, 21/10): lo STEP che il
// Motore A propone come sorgente del prodotto (il codice letto dal nome del file ha la base del prodotto e il marcatore
// compatibile, come 9120001 contro il prodotto 9120001A) si conferma con S8 senza il motivo, anche se la vecchia lettura
// del file (documento_proposta.codice) non e' il codice del prodotto. Prima della correzione S8 finiva «parziale» con
// MOTIVO_OBBLIGATORIO nel passo 2, e la pagina non chiede il motivo sulla proposta del motore. Scelta [T-39 integrazione,
// da ratificare], ristretta nel giro G39-bis (rilievo della revisione del contratto): vale solo con la candidatura
// UNIVOCA del prodotto nello stesso calcolo del motore (quella che la pagina propone); una candidatura non univoca, il
// ripiego, nessun calcolo o lo STEP di un altro prodotto vogliono il motivo. Quando il codice del documento si corregge
// senza il motivo dell'utente, la riga di S8 ha come motivo la traccia del server con le evidenze del motore (nessun DDL),
// e il reinvio dello stesso corpo resta «gia' fatto». Dati inventati (ACME).

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

func TestV1G39S8IlCodiceLettoDalMotoreNonChiedeIlMotivo(t *testing.T) {
	evidenze := []propostabom.EvidenzaCandidatura{{Lettura: "l1", Selettore: "nome_file", CodiceLetto: "ACM-100", BaseLetta: "ACM-100",
		EsitoBase: "uguale", EsitoMarcatore: "compatibile"}}
	univoca := func(s scenaS8) *distintabozza.StepCandidati {
		return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto}},
			Univoche: map[uuid.UUID]uuid.UUID{s.stepA: s.prodotto}, Evidenze: map[uuid.UUID][]propostabom.EvidenzaCandidatura{s.stepA: evidenze}}
	}
	casi := []struct {
		nome      string
		letto     string // la vecchia lettura del nome del file (documento_proposta.codice)
		candidati func(s scenaS8) *distintabozza.StepCandidati
		motivo    bool // il passo 2 chiede il motivo
		traccia   bool // la riga di S8 ha la traccia del Motore A come motivo
	}{
		{"candidatura univoca del prodotto nel calcolo del motore", "ACM-100", univoca, false, true},
		{"candidatura univoca con il codice letto uguale a quello del prodotto: nessuna correzione, nessuna traccia", "ACM-1000", univoca, false, false},
		{"candidatura non univoca del solo prodotto (giro G39-bis, revisione del contratto)", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto}}}
		}, true, false},
		{"senza il riconoscimento del motore", "ACM-100", func(scenaS8) *distintabozza.StepCandidati { return nil }, true, false},
		{"il ripiego senza il motore", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: false, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto}},
				Univoche: map[uuid.UUID]uuid.UUID{s.stepA: s.prodotto}}
		}, true, false},
		{"candidato di due prodotti", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.prodotto, s.altroFinito}}}
		}, true, false},
		{"candidato univoco di un altro prodotto", "ACM-100", func(s scenaS8) *distintabozza.StepCandidati {
			return &distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{s.stepA: {s.altroFinito}},
				Univoche: map[uuid.UUID]uuid.UUID{s.stepA: s.altroFinito}}
		}, true, false},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			p, s := preparaS8(t)
			// la vecchia lettura del nome del file: il prodotto e' ACM-1000, il file dice c.letto
			esegui(t, p, `UPDATE documento_proposta SET codice = $2 WHERE allegato_id = $1`, s.stepA, c.letto)
			reg := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
			prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: reg, StepCandidati: c.candidati(s)}}
			corpo := corpoStep(t, s, uuid.New(), reg, nil)
			es, err := s8(t, p, s, s.stepA, corpo, prep)
			if err != nil {
				t.Fatalf("S8: %v", err)
			}
			if !c.motivo {
				if es.Esito != EsitoCompletata || len(es.Passi) != 3 || es.Passi[1].Esito != EsitoFatto || es.Bozza == nil {
					t.Fatalf("lo STEP proposto dal motore: atteso S8 completata senza il motivo, avuto %+v", es)
				}
				if n := conta(t, p, `SELECT count(*) FROM documento WHERE componente_id = $1 AND tipo = 'cad_3d' AND codice = 'ACM-1000' AND sha256 = $2`, s.prodotto, s.shaA); n != 1 {
					t.Fatalf("il documento dello STEP con il codice del prodotto: %d", n)
				}
				var motivo *string
				if err := p.QueryRow(context.Background(), `SELECT motivo FROM fonte_step_evento WHERE azione = 'conferma_documento' AND prodotto_id = $1`, s.prodotto).Scan(&motivo); err != nil {
					t.Fatalf("la riga di S8: %v", err)
				}
				if !c.traccia {
					if motivo != nil {
						t.Fatalf("senza correzione del codice la riga di S8 non ha un motivo del server: %q", *motivo)
					}
					return
				}
				atteso := "codice dal Motore A: letto «ACM-100», prodotto «ACM-1000»; candidatura univoca dello STEP del finito; evidenze: nome_file ACM-100 (base uguale, marcatore compatibile)"
				if motivo == nil || *motivo != atteso {
					t.Fatalf("la traccia del Motore A nella riga di S8:\navuto  %v\natteso %q", stringa(motivo), atteso)
				}
				// il reinvio dello stesso corpo con la stessa chiave (la procedura rifatta): il passo 2 riconosce la traccia
				// come il motivo di questo corpo, «gia' fatto», niente CHIAVE_RIUSATA e niente scritto
				prima := fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`), conta(t, p, `SELECT count(*) FROM fonte_step_evento`))
				r, imp, err := LeggiRichiestaS8(corpo)
				if err != nil {
					t.Fatal(err)
				}
				es2, err := ConfermaS8(context.Background(), p, s.thread, s.stepA, s.utente, r, imp, prep)
				if err != nil || !es2.Riproposto || es2.Esito != EsitoCompletata {
					t.Fatalf("la procedura rifatta con la stessa chiave: %+v %v", es2, err)
				}
				if dopo := fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`), conta(t, p, `SELECT count(*) FROM fonte_step_evento`)); dopo != prima {
					t.Fatalf("il reinvio ha scritto: %s -> %s", prima, dopo)
				}
				return
			}
			if es.Esito == EsitoCompletata || len(es.Passi) < 2 || es.Passi[1].Codice != distintabozza.CodiceMotivoObbligatorio {
				t.Fatalf("senza la candidatura univoca del motore il passo 2 vuole il motivo: %+v", es)
			}
			if n := conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1`, s.shaA); n != 0 {
				t.Fatalf("il rifiuto ha scritto il documento: %d", n)
			}
			// con il motivo passa, come sempre, e il motivo e' quello dell'utente
			es, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), reg, map[string]any{"motivo": "lo STEP e' quello del prodotto"}), prep)
			if err != nil || es.Esito != EsitoCompletata {
				t.Fatalf("S8 con il motivo: %+v %v", es, err)
			}
			if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'conferma_documento' AND prodotto_id = $1 AND motivo = 'lo STEP e'' quello del prodotto'`, s.prodotto); n != 1 {
				t.Fatalf("la riga di S8 con il motivo dell'utente: %d", n)
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
