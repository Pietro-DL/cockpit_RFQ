// Package config legge cockpit.toml (accanto all'exe o passato con -config).
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server  Server   `toml:"server"`
	DB      DB       `toml:"db"`
	NAS     NAS      `toml:"nas"`
	Outlook Outlook  `toml:"outlook"`
	Utenti  []Utente `toml:"utenti"`
}

type Server struct {
	Indirizzo       string `toml:"indirizzo"`        // es. "127.0.0.1:8080" in sviluppo, "0.0.0.0:8080" in LAN
	TokenWorker     string `toml:"token_worker"`     // condiviso con i worker Python (header X-Cockpit-Token)
	SegretoSessione string `toml:"segreto_sessione"` // riservato a usi futuri (firma cookie); le sessioni vivono nel DB
	LogLivello      string `toml:"log_livello"`      // debug | info | warn
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
	c.Outlook.Cartelle = []string{"Inbox", "Sent Items"}
	c.Outlook.IntervalloSyncS = 60
	c.Outlook.Lotto = 50
	if _, err := toml.DecodeFile(percorso, c); err != nil {
		return nil, fmt.Errorf("config %s: %w", percorso, err)
	}
	if c.DB.DSN == "" {
		return nil, fmt.Errorf("config: [db].dsn mancante")
	}
	if c.Server.TokenWorker == "" {
		return nil, fmt.Errorf("config: [server].token_worker mancante")
	}
	if c.NAS.Staging == "" {
		c.NAS.Staging = filepath.Join(filepath.Dir(percorso), "staging")
	}
	if err := os.MkdirAll(c.NAS.Staging, 0o755); err != nil {
		return nil, fmt.Errorf("staging %s: %w", c.NAS.Staging, err)
	}
	return c, nil
}
