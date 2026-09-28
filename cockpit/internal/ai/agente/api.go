package agente

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Il client verso il servizio esterno.
//
// Sta dietro l'interfaccia `Modello` e non è mai il soggetto dei test: tutto ciò che questo pacchetto
// garantisce — lo schema, il grounding, l'idempotenza — è codice nostro e si prova senza rete. Qui
// c'è solo il trasporto.
//
// Tre cose per cui è scritto così:
//
//   - la CHIAVE si legge da una variabile d'ambiente e non dal file di configurazione, perché il
//     file di configurazione finisce nei backup e ogni tanto in un repository;
//   - senza chiave il costruttore restituisce nil, e un `Servizio` con Modello nil è spento. Non
//     esiste un percorso in cui una configurazione incompleta produca una chiamata;
//   - il timeout è corto e i tentativi sono pochi. Un'analisi che non risponde non è un'emergenza:
//     è una proposta in meno, e l'operatore lavora come prima.
type ClientAPI struct {
	URL     string
	Chiave  string
	Modello string
	Client  *http.Client
}

// DaAmbiente costruisce il client se c'è tutto, altrimenti nil. `nomeVariabile` è il nome della
// variabile d'ambiente che contiene la chiave (es. ANTHROPIC_API_KEY): il file di configurazione dice
// DOVE sta la chiave, non qual è.
func DaAmbiente(url, modello, nomeVariabile string) *ClientAPI {
	chiave := strings.TrimSpace(os.Getenv(nomeVariabile))
	if chiave == "" || modello == "" {
		return nil
	}
	if url == "" {
		url = "https://api.anthropic.com/v1/messages"
	}
	return &ClientAPI{URL: url, Chiave: chiave, Modello: modello,
		Client: &http.Client{Timeout: 60 * time.Second}}
}

func (c *ClientAPI) Nome() string { return c.Modello }

type richiestaAPI struct {
	Modello  string         `json:"model"`
	MaxToken int            `json:"max_tokens"`
	Sistema  string         `json:"system"`
	Messaggi []messaggioAPI `json:"messages"`
	Temp     float64        `json:"temperature"`
}

type messaggioAPI struct {
	Ruolo     string `json:"role"`
	Contenuto string `json:"content"`
}

type rispostaAPI struct {
	Contenuto []struct {
		Tipo  string `json:"type"`
		Testo string `json:"text"`
	} `json:"content"`
	Uso struct {
		In  int `json:"input_tokens"`
		Out int `json:"output_tokens"`
	} `json:"usage"`
	Errore *struct {
		Tipo      string `json:"type"`
		Messaggio string `json:"message"`
	} `json:"error"`
}

// Chiedi manda il prompt e restituisce il testo della risposta.
//
// `temperature` a zero: non si vuole varietà, si vuole che lo stesso messaggio dia lo stesso esito.
// Una proposta che cambia a ogni rianalisi non si può confrontare con niente, e la misura sul corpus
// (L6) smetterebbe di voler dire qualcosa.
func (c *ClientAPI) Chiedi(ctx context.Context, sistema, utente string) ([]byte, int, int, error) {
	if c == nil || c.Chiave == "" {
		return nil, 0, 0, ErrSpento
	}
	corpo, err := json.Marshal(richiestaAPI{
		Modello: c.Modello, MaxToken: 2000, Sistema: sistema, Temp: 0,
		Messaggi: []messaggioAPI{{Ruolo: "user", Contenuto: utente}},
	})
	if err != nil {
		return nil, 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(corpo))
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.Chiave)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()
	var r rispostaAPI
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, 0, 0, fmt.Errorf("risposta illeggibile (HTTP %d): %w", resp.StatusCode, err)
	}
	if r.Errore != nil {
		return nil, r.Uso.In, r.Uso.Out, fmt.Errorf("servizio: %s: %s", r.Errore.Tipo, r.Errore.Messaggio)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, r.Uso.In, r.Uso.Out, fmt.Errorf("HTTP %d dal servizio di analisi", resp.StatusCode)
	}
	var testo strings.Builder
	for _, p := range r.Contenuto {
		if p.Tipo == "text" {
			testo.WriteString(p.Testo)
		}
	}
	if testo.Len() == 0 {
		return nil, r.Uso.In, r.Uso.Out, errors.New("il servizio ha risposto senza testo")
	}
	return []byte(testo.String()), r.Uso.In, r.Uso.Out, nil
}
