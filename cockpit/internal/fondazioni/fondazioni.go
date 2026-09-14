// Package fondazioni porta in DB ciò che cockpit.toml dichiara nelle sezioni [[casella]],
// [[postazione]] e [[worker]] (piano di correzione, voce 0.6).
//
// Il seed è NON distruttivo: aggiorna per chiave naturale (indirizzo, nome host, nome worker) e non
// disattiva mai una riga che non compare più nel file — la segnala soltanto, perché disattivare una
// casella significa fermarne il sync e non deve succedere per una svista di configurazione.
//
// Il token di un worker resta nel file: in DB va il suo sha256 (`worker_credenziale.token_hash`).
package fondazioni

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/config"
	"promatec/cockpit/internal/db"
)

// Esito riassume cosa ha fatto il seed; il chiamante lo logga e i test lo verificano.
type Esito struct {
	Caselle        int
	Postazioni     int
	Worker         int
	CasellaDefault uuid.NullUUID
	NonPiuNelFile  []string // righe presenti in DB e assenti dal file: mai disattivate d'ufficio
	UtentiMancanti []string // sigle citate dal file e non presenti in `utente`
}

// HashToken calcola l'impronta con cui il server riconosce il token di un worker.
func HashToken(token string) string {
	somma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(somma[:])
}

// Semina applica le sezioni di cockpit.toml al DB. Va chiamata dopo le migrazioni e dopo il seed
// degli utenti, perché casella.utente_id e postazione.utente_id li referenziano per sigla.
func Semina(ctx context.Context, q *db.Queries, cfg *config.Config, log *slog.Logger) (Esito, error) {
	var e Esito

	utenti, err := q.ListUtenti(ctx)
	if err != nil {
		return e, fmt.Errorf("fondazioni: utenti: %w", err)
	}
	perSigla := make(map[string]uuid.UUID, len(utenti))
	for _, u := range utenti {
		perSigla[strings.ToUpper(u.Sigla)] = u.UtenteID
	}
	risolviUtente := func(sigla, dove string) uuid.NullUUID {
		if sigla == "" {
			return uuid.NullUUID{}
		}
		id, ok := perSigla[sigla]
		if !ok {
			e.UtentiMancanti = append(e.UtentiMancanti, fmt.Sprintf("%s (%s)", sigla, dove))
			return uuid.NullUUID{}
		}
		return uuid.NullUUID{UUID: id, Valid: true}
	}

	// ---------------------------------------------------------------- caselle
	idCasella := map[string]uuid.UUID{}
	for _, k := range cfg.Caselle {
		canale := db.Canale(k.Canale)
		if !canale.Valid() {
			return e, fmt.Errorf("fondazioni: casella %s: canale %q non valido", k.Indirizzo, k.Canale)
		}
		riga, err := q.UpsertCasella(ctx, db.UpsertCasellaParams{
			Canale:    canale,
			Indirizzo: k.Indirizzo,
			Nome:      k.Nome,
			Condivisa: k.Condivisa,
			UtenteID:  risolviUtente(k.Utente, "casella "+k.Indirizzo),
		})
		if err != nil {
			return e, fmt.Errorf("fondazioni: casella %s: %w", k.Indirizzo, err)
		}
		idCasella[k.Indirizzo] = riga.CasellaID
		e.Caselle++
	}

	// ---------------------------------------------------------------- postazioni
	idPostazione := map[string]uuid.UUID{}
	for _, p := range cfg.Postazioni {
		riga, err := q.UpsertPostazione(ctx, db.UpsertPostazioneParams{
			NomeHost:    p.NomeHost,
			Descrizione: p.Descrizione,
			UtenteID:    risolviUtente(p.Utente, "postazione "+p.NomeHost),
		})
		if err != nil {
			return e, fmt.Errorf("fondazioni: postazione %s: %w", p.NomeHost, err)
		}
		idPostazione[p.NomeHost] = riga.PostazioneID
		e.Postazioni++
	}

	// ---------------------------------------------------------------- worker
	for _, w := range cfg.Worker {
		tipo := db.WorkerTipo(w.Tipo)
		if !tipo.Valid() || tipo == db.WorkerTipoServer {
			return e, fmt.Errorf("fondazioni: worker %s: tipo %q non valido", w.Nome, w.Tipo)
		}
		var post uuid.NullUUID
		if w.Postazione != "" {
			id, ok := idPostazione[w.Postazione]
			if !ok {
				return e, fmt.Errorf("fondazioni: worker %s: postazione %s non censita", w.Nome, w.Postazione)
			}
			post = uuid.NullUUID{UUID: id, Valid: true}
		}
		caselle := make([]uuid.UUID, 0, len(w.Caselle))
		for _, ind := range w.Caselle {
			id, ok := idCasella[ind]
			if !ok {
				return e, fmt.Errorf("fondazioni: worker %s: casella %s non censita", w.Nome, ind)
			}
			caselle = append(caselle, id)
		}
		if _, err := q.UpsertWorkerCredenziale(ctx, db.UpsertWorkerCredenzialeParams{
			WorkerNome:   w.Nome,
			WorkerTipo:   tipo,
			TokenHash:    HashToken(w.Token),
			PostazioneID: post,
			Caselle:      caselle,
		}); err != nil {
			return e, fmt.Errorf("fondazioni: worker %s: %w", w.Nome, err)
		}
		e.Worker++
	}

	// ---------------------------------------------------------------- casella predefinita
	if cfg.Outlook.CasellaDefault != "" {
		id, ok := idCasella[cfg.Outlook.CasellaDefault]
		if !ok {
			return e, fmt.Errorf("fondazioni: [outlook].casella_default %s non censita", cfg.Outlook.CasellaDefault)
		}
		e.CasellaDefault = uuid.NullUUID{UUID: id, Valid: true}
	}

	// ---------------------------------------------------------------- righe orfane: solo avviso
	inDB, err := q.ListCaselle(ctx)
	if err != nil {
		return e, err
	}
	for _, k := range inDB {
		if _, presente := idCasella[k.Indirizzo]; !presente && k.Attiva {
			e.NonPiuNelFile = append(e.NonPiuNelFile, "casella "+k.Indirizzo)
		}
	}
	credenziali, err := q.ListWorkerCredenziali(ctx)
	if err != nil {
		return e, err
	}
	nelFile := map[string]bool{}
	for _, w := range cfg.Worker {
		nelFile[w.Nome] = true
	}
	for _, w := range credenziali {
		if !nelFile[w.WorkerNome] && w.Attivo {
			e.NonPiuNelFile = append(e.NonPiuNelFile, "worker "+w.WorkerNome)
		}
	}

	if log != nil {
		log.Info("fondazioni", "caselle", e.Caselle, "postazioni", e.Postazioni, "worker", e.Worker,
			"casella_default", cfg.Outlook.CasellaDefault)
		for _, r := range e.NonPiuNelFile {
			log.Warn("in DB ma non più in cockpit.toml: lasciata attiva, disattivare a mano se voluto", "riga", r)
		}
		for _, u := range e.UtentiMancanti {
			log.Warn("sigla utente citata in cockpit.toml e assente in [[utenti]]", "utente", u)
		}
	}
	return e, nil
}

// CasellaPerIndirizzo risolve un indirizzo (case-insensitive) in casella_id.
func CasellaPerIndirizzo(ctx context.Context, q *db.Queries, canale, indirizzo string) (uuid.UUID, error) {
	c := db.Canale(canale)
	if !c.Valid() {
		return uuid.Nil, fmt.Errorf("canale %q non valido", canale)
	}
	riga, err := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: c, Indirizzo: strings.ToLower(strings.TrimSpace(indirizzo))})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("casella %s non censita", indirizzo)
	}
	if err != nil {
		return uuid.Nil, err
	}
	return riga.CasellaID, nil
}
