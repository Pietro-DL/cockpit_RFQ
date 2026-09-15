// Package config legge cockpit.toml (accanto all'exe o passato con -config).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server     Server       `toml:"server"`
	DB         DB           `toml:"db"`
	NAS        NAS          `toml:"nas"`
	Outlook    Outlook      `toml:"outlook"`
	Utenti     []Utente     `toml:"utenti"`
	Caselle    []Casella    `toml:"casella"`    // [[casella]] — fase 0, voce 0.6
	Postazioni []Postazione `toml:"postazione"` // [[postazione]]
	Worker     []Worker     `toml:"worker"`     // [[worker]] — credenziali individuali
	Retention  Retention    `toml:"retention"`  // [retention] — fase 1, voce 1.6
	Analisi    Analisi      `toml:"analisi"`    // [analisi] — fase 1, voce 1.12
}

// Retention: per quanto si tengono i job chiusi. Una coda che non si svuota mai diventa illeggibile e
// rallenta le interrogazioni di amministrazione; 0 = non cancellare nulla.
type Retention struct {
	GiorniJob int `toml:"giorni_job"`
}

// Analisi identifica CON CHE COSA un file è stato analizzato (voce 1.12, A15).
//
// I fatti estratti da un file dipendono da tre cose e solo da tre: il contenuto del file, la versione
// dell'analizzatore e la configurazione che gli è stata data. A parità di tutte e tre, rifare
// l'analisi è lavoro sprecato e il risultato è per definizione lo stesso; se cambia una delle tre, il
// risultato precedente non vale più. Senza queste due informazioni il server non può distinguere i
// due casi, e finisce per fare sempre la scelta sbagliata: o rianalizza tutto ogni volta, o riusa
// fatti calcolati con un dizionario che non è più quello.
//
// Parametri è il blocco che viene mandato al worker; il suo sha256, insieme alla versione, è la
// chiave con cui i fatti vengono conservati e ritrovati. Cambiare un termine qui basta a far
// rianalizzare tutto, senza toccare il codice.
type Analisi struct {
	Versione  int            `toml:"versione"`
	Parametri map[string]any `toml:"parametri"`
}

type Server struct {
	Indirizzo       string `toml:"indirizzo"`        // es. "127.0.0.1:8080" in sviluppo, "0.0.0.0:8080" in LAN
	TokenWorker     string `toml:"token_worker"`     // condiviso con i worker Python (header X-Cockpit-Token)
	SegretoSessione string `toml:"segreto_sessione"` // riservato a usi futuri (firma cookie); le sessioni vivono nel DB
	LogLivello      string `toml:"log_livello"`      // debug | info | warn
	// MaxUploadMB è il limite di un singolo allegato caricato dal worker con
	// PUT /api/v1/allegati/{id}/file (voce 2.3). Oltre, il server risponde 413 prima di leggere il
	// corpo e l'allegato va in errore con il motivo visibile; senza un limite un allegato da qualche
	// gigabyte riempirebbe lo staging del server in silenzio. Zero = il default (64).
	MaxUploadMB int `toml:"max_upload_mb"`
}

type DB struct {
	DSN string `toml:"dsn"`
}

type NAS struct {
	Radice  string `toml:"radice"`  // \nas01\TECNICO - PREVENTIVI\PREVENTIVI DA FARE  (in sviluppo: cartella locale)
	DryRun  bool   `toml:"dry_run"` // true = nessuna scrittura reale, solo log
	Staging string `toml:"staging"` // cartella locale dove il worker-outlook salva gli allegati
}

type Outlook struct {
	Cartelle        []string `toml:"cartelle"`          // es. ["Inbox", "Sent Items"]
	IntervalloSyncS int      `toml:"intervallo_sync_s"` // ogni quanti secondi accodare sync_outlook
	Dal             string   `toml:"dal"`               // "2026-09-01": data minima al primo avvio (cursore vuoto)
	Lotto           int      `toml:"lotto"`             // messaggi per POST ingest
	ConsentiInvio   bool     `toml:"consenti_invio"`    // false = solo bozze (regola aziendale)
	CasellaDefault  string   `toml:"casella_default"`   // indirizzo della casella attribuita ai messaggi che non la dichiarano (fase 1)
}

// Casella è una voce [[casella]]: una casella di posta censita, personale o condivisa.
type Casella struct {
	Indirizzo string `toml:"indirizzo"` // 'nome.cognome@azienda.it' (normalizzato in minuscolo)
	Nome      string `toml:"nome"`      // etichetta in UI: 'Francesco'
	Canale    string `toml:"canale"`    // default 'outlook'
	Condivisa bool   `toml:"condivisa"` // true = cassetta condivisa Exchange (nessun proprietario)
	Utente    string `toml:"utente"`    // sigla del proprietario; vuoto o ignorato se condivisa
	// Attiva: assente = true. Serve a dichiarare nel file quale casella è in uso, invece di doverlo
	// fare a mano in database a ogni avvio. Finché lo schema è alla 0003 ne può essere attiva una sola.
	Attiva *bool `toml:"attiva"`
}

// EAttiva risolve il default: una casella senza `attiva` nel file è attiva.
func (c Casella) EAttiva() bool { return c.Attiva == nil || *c.Attiva }

// Postazione è una voce [[postazione]]: un PC con Outlook e un worker.
type Postazione struct {
	NomeHost    string `toml:"nome_host"` // 'PC-FRANCESCO' (normalizzato in maiuscolo)
	Descrizione string `toml:"descrizione"`
	Utente      string `toml:"utente"` // sigla di chi ci lavora
}

// Worker è una voce [[worker]]: credenziale individuale e caselle autorizzate.
// Il token sta solo qui: nel DB finisce il suo sha256.
type Worker struct {
	Nome       string   `toml:"nome"`       // 'outlook@PC-FRANCESCO'
	Tipo       string   `toml:"tipo"`       // outlook | analisi
	Token      string   `toml:"token"`      // segreto individuale
	Postazione string   `toml:"postazione"` // nome host
	Caselle    []string `toml:"caselle"`    // indirizzi autorizzati
}

type Utente struct {
	Sigla    string `toml:"sigla"`
	Nome     string `toml:"nome"`
	Ufficio  string `toml:"ufficio"`
	Ruolo    string `toml:"ruolo"`
	Password string `toml:"password"` // solo per il seed iniziale; nel DB va l'hash bcrypt
}

func Carica(percorso string) (*Config, error) {
	c := &Config{}
	c.Server.Indirizzo = "127.0.0.1:8080"
	c.Server.LogLivello = "info"
	c.Server.MaxUploadMB = 64
	c.Outlook.Cartelle = []string{"Inbox", "Sent Items"}
	c.Outlook.IntervalloSyncS = 60
	c.Outlook.Lotto = 50
	c.Retention.GiorniJob = 30
	c.Analisi.Versione = 1
	if _, err := toml.DecodeFile(percorso, c); err != nil {
		return nil, fmt.Errorf("config %s: %w", percorso, err)
	}
	if c.DB.DSN == "" {
		return nil, fmt.Errorf("config: [db].dsn mancante")
	}
	if c.Server.TokenWorker == "" {
		return nil, fmt.Errorf("config: [server].token_worker mancante")
	}
	if c.Server.MaxUploadMB < 1 {
		return nil, fmt.Errorf("config: [server].max_upload_mb = %d non valido (almeno 1)", c.Server.MaxUploadMB)
	}
	if c.NAS.Staging == "" {
		c.NAS.Staging = filepath.Join(filepath.Dir(percorso), "staging")
	}
	if err := os.MkdirAll(c.NAS.Staging, 0o755); err != nil {
		return nil, fmt.Errorf("staging %s: %w", c.NAS.Staging, err)
	}
	if err := c.normalizzaFondazioni(); err != nil {
		return nil, err
	}
	return c, nil
}

// normalizzaFondazioni mette in forma canonica [[casella]], [[postazione]], [[worker]] e
// [outlook].casella_default, e rifiuta i riferimenti incrociati che non esistono. Un errore qui
// blocca l'avvio: meglio non partire che partire con un routing sbagliato.
func (c *Config) normalizzaFondazioni() error {
	indirizzi := map[string]bool{}
	for i := range c.Caselle {
		k := &c.Caselle[i]
		k.Indirizzo = strings.ToLower(strings.TrimSpace(k.Indirizzo))
		if k.Indirizzo == "" {
			return fmt.Errorf("config: [[casella]] #%d senza indirizzo", i+1)
		}
		if indirizzi[k.Indirizzo] {
			return fmt.Errorf("config: casella %q dichiarata due volte", k.Indirizzo)
		}
		indirizzi[k.Indirizzo] = true
		if k.Canale == "" {
			k.Canale = "outlook"
		}
		if k.Nome == "" {
			k.Nome, _, _ = strings.Cut(k.Indirizzo, "@")
		}
		k.Utente = strings.ToUpper(strings.TrimSpace(k.Utente))
		if k.Condivisa && k.Utente != "" {
			return fmt.Errorf("config: casella %q è condivisa e non può avere un proprietario (utente = %q)", k.Indirizzo, k.Utente)
		}
	}
	host := map[string]bool{}
	for i := range c.Postazioni {
		p := &c.Postazioni[i]
		p.NomeHost = strings.ToUpper(strings.TrimSpace(p.NomeHost))
		if p.NomeHost == "" {
			return fmt.Errorf("config: [[postazione]] #%d senza nome_host", i+1)
		}
		if host[p.NomeHost] {
			return fmt.Errorf("config: postazione %q dichiarata due volte", p.NomeHost)
		}
		host[p.NomeHost] = true
		p.Utente = strings.ToUpper(strings.TrimSpace(p.Utente))
	}
	nomi := map[string]bool{}
	for i := range c.Worker {
		w := &c.Worker[i]
		w.Nome = strings.TrimSpace(w.Nome)
		if w.Nome == "" {
			return fmt.Errorf("config: [[worker]] #%d senza nome", i+1)
		}
		if nomi[w.Nome] {
			return fmt.Errorf("config: worker %q dichiarato due volte", w.Nome)
		}
		nomi[w.Nome] = true
		if w.Tipo != "outlook" && w.Tipo != "analisi" {
			return fmt.Errorf("config: worker %q: tipo %q non valido (outlook | analisi)", w.Nome, w.Tipo)
		}
		if w.Token == "" {
			return fmt.Errorf("config: worker %q senza token", w.Nome)
		}
		w.Postazione = strings.ToUpper(strings.TrimSpace(w.Postazione))
		if w.Postazione != "" && !host[w.Postazione] {
			return fmt.Errorf("config: worker %q: postazione %q non dichiarata in [[postazione]]", w.Nome, w.Postazione)
		}
		for j, ind := range w.Caselle {
			w.Caselle[j] = strings.ToLower(strings.TrimSpace(ind))
			if !indirizzi[w.Caselle[j]] {
				return fmt.Errorf("config: worker %q: casella %q non dichiarata in [[casella]]", w.Nome, w.Caselle[j])
			}
		}
	}
	c.Outlook.CasellaDefault = strings.ToLower(strings.TrimSpace(c.Outlook.CasellaDefault))
	switch {
	case c.Outlook.CasellaDefault != "":
		if !indirizzi[c.Outlook.CasellaDefault] {
			return fmt.Errorf("config: [outlook].casella_default = %q non è fra le [[casella]] dichiarate", c.Outlook.CasellaDefault)
		}
	case len(c.Caselle) == 1:
		c.Outlook.CasellaDefault = c.Caselle[0].Indirizzo
	case len(c.Caselle) > 1:
		return fmt.Errorf("config: con più [[casella]] serve [outlook].casella_default")
	}
	return nil
}
