//go:build integrazione

// L4 — i suggerimenti dell'agente scritti dalle prove (A1c-L4S-11; piano A, 6.4.7 e 3.5; R10): sul database di
// prova AnalisiMessaggioDiProva scrive la riga con il risultato dato; la guardia legge dal catalogo il nome e il
// commento del database collegato, e con quei valori rifiuta un database senza «test» e senza il marcatore delle
// copie usa e getta, cioè la copia intatta del dump. Il database di prova ha «test» nel nome, quindi il rifiuto
// si prova sulla parte pura con i valori letti, cambiato il solo nome.
//
// Il messaggio e il suggerimento sono inventati (ACME): nessun dato reale, il repository è pubblico.

package testutil

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAnalisiMessaggioDiProvaSoloDoveAmmesso(t *testing.T) {
	p := Pool(t)
	SchemaPulito(t, p)
	ctx := context.Background()

	nome, commento, err := nomeECommentoDelDatabase(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if nome != p.Config().ConnConfig.Database {
		t.Fatalf("il nome letto dal catalogo (%q) non è quello del DSN (%q)", nome, p.Config().ConnConfig.Database)
	}
	if err := scritturaDiProvaAmmessa(nome, commento); err != nil {
		t.Fatalf("il database di prova è rifiutato: %v", err)
	}
	// la copia intatta: lo stesso commento, ma senza «test» nel nome
	senzaTest := strings.NewReplacer("test", "copia", "TEST", "COPIA", "Test", "Copia").Replace(nome)
	if strings.HasPrefix(commento, marcatoreCopiaUsaEGetta) {
		t.Fatalf("il database di prova ha il marcatore delle copie usa e getta nel commento: %q", commento)
	}
	if err := scritturaDiProvaAmmessa(senzaTest, commento); err == nil {
		t.Errorf("un database senza «test» e senza marcatore (%q, commento %q) è ammesso", senzaTest, commento)
	}
	if err := scritturaDiProvaAmmessa(senzaTest, marcatoreCopiaUsaEGetta+"acme_copia (prova)"); err != nil {
		t.Errorf("una copia usa e getta con il marcatore è rifiutata: %v", err)
	}

	var conv, msg uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', 'C-ACME-AGENTE', now()) RETURNING conversazione_id`).Scan(&conv); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento)
		VALUES ('outlook', '<agente@acme.example>', $1, 'entrata', now()) RETURNING messaggio_id`, conv).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	risultato := json.RawMessage(`{"codici": ["7129999"], "nota": "suggerimento discordante inventato"}`)
	id := AnalisiMessaggioDiProva(t, p, msg, risultato)
	altro := AnalisiMessaggioDiProva(t, p, msg, json.RawMessage(`{"codici": []}`))
	if id == altro {
		t.Fatal("due risultati diversi per lo stesso messaggio hanno dato la stessa riga")
	}
	var stato string
	var letto json.RawMessage
	if err := p.QueryRow(ctx, `SELECT stato::text, risultato FROM analisi_messaggio WHERE analisi_id = $1 AND messaggio_id = $2`,
		id, msg).Scan(&stato, &letto); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Codici []string `json:"codici"`
	}
	if err := json.Unmarshal(letto, &v); err != nil || stato != "completata" || len(v.Codici) != 1 || v.Codici[0] != "7129999" {
		t.Errorf("riga scritta: stato %q, risultato %s (%v)", stato, letto, err)
	}
	if n := Conta(t, p, "analisi_messaggio"); n != 2 {
		t.Errorf("righe dei suggerimenti: %d, attese 2", n)
	}
}
