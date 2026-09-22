// Gli utenti di cockpit.toml: chi esiste, con che ruolo, e la password soltanto per nascere.
//
// Sta con le fondazioni e non più in `transport/web` perché è la stessa cosa che fa `Semina` per
// caselle, postazioni e worker: portare in database ciò che il file dichiara, prima che il server
// ascolti. Che poi il login legga quell'hash è un caso d'uso del web, non il posto in cui l'utente
// viene al mondo — e finché stava lì, il test delle fondazioni doveva importare `web` per poter
// seminare le sigle a cui le caselle si riferiscono.
//
// L'ordine resta quello: prima gli utenti, poi `Semina`, che risolve le sigle in `utente_id`.

package fondazioni

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"promatec/cockpit/internal/platform/db"
)

// SeedUtenti crea gli utenti di cockpit.toml e li tiene allineati al file — tranne la password
// (voce 6.4).
//
// # La password nel file serve a NASCERE, non a essere riscritta ogni notte
//
// Prima, un `password` nel TOML veniva rihashata e riscritta a ogni avvio: il file era la verita', e
// una password cambiata dall'utente sarebbe tornata indietro al riavvio successivo — cioe' il cambio
// password non poteva esistere. Ora il TOML e' il BOOTSTRAP: fa esistere un utente che altrimenti
// non potrebbe entrare la prima volta, e appena in database c'e' un hash valido e' quello a vincere.
// Il cambio si fa dalla UI (voce 6.4, `/profilo/password`), che e' l'unico posto in cui la password
// nuova la conosce solo chi la sta scegliendo.
//
// La regola vale anche al contrario, e non e' un dettaglio: scrivere una password nuova nel file NON
// e' piu' il modo di reimpostarla. Chi ci prova non vedrebbe nessun errore — vedrebbe un login che
// continua a rifiutarlo — quindi il seed lo scrive nel log, una riga per utente.
//
// # Un ruolo che non esiste ferma l'avvio
//
// Prima diventava `operatore` in silenzio. Una parola sbagliata (`amministratore`, `Admin` con uno
// spazio) dava quindi un utente convinto di essere amministratore e che non lo era; e nell'altro
// verso, un errore di battitura in un ruolo qualunque che nessuno notava. Un ruolo che non sappiamo
// leggere e' una frase che non sappiamo eseguire: si dice, e non si parte.
func SeedUtenti(ctx context.Context, q *db.Queries, utenti []struct{ Sigla, Nome, Ufficio, Ruolo, Password string }, log *slog.Logger) error {
	righe, err := q.ListUtenti(ctx)
	if err != nil {
		return fmt.Errorf("utenti gia' in database: %w", err)
	}
	esistenti := map[string]db.Utente{}
	for _, u := range righe {
		esistenti[strings.ToUpper(u.Sigla)] = u
	}
	for _, u := range utenti {
		sigla := strings.ToUpper(strings.TrimSpace(u.Sigla))
		ruolo := db.RuoloUtente(strings.ToLower(strings.TrimSpace(u.Ruolo)))
		if !ruolo.Valid() {
			return fmt.Errorf("utente %s: ruolo %q non valido (ammessi: %s)", sigla, u.Ruolo, RuoliAmmessi())
		}
		// hash non valido = nessuna scrittura: la query fa COALESCE(EXCLUDED, esistente), quindi
		// lasciarlo vuoto e' esattamente «non toccare la password che c'e' gia'».
		var hash pgtype.Text
		switch {
		case passwordImpostata(esistenti[sigla]):
			if u.Password != "" {
				log.Info("la password e' gia' impostata in database: quella in cockpit.toml non la sostituisce (serve solo al primo avvio)", "utente", sigla)
			}
		case u.Password != "":
			h, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("utente %s: %w", sigla, err)
			}
			hash = pgtype.Text{String: string(h), Valid: true}
		default:
			log.Warn("utente senza password: non potra' entrare finche' non gliene viene data una in cockpit.toml", "utente", sigla)
		}
		if _, err := q.UpsertUtente(ctx, db.UpsertUtenteParams{Sigla: sigla, Nome: u.Nome, Ufficio: u.Ufficio, Ruolo: ruolo, PasswordHash: hash}); err != nil {
			return fmt.Errorf("utente %s: %w", sigla, err)
		}
	}
	return nil
}

// RuoliAmmessi e' l'elenco per i messaggi d'errore, preso dall'enum dello schema: se un domani i
// ruoli diventano cinque, il messaggio lo dice da solo.
func RuoliAmmessi() string {
	nomi := make([]string, 0, len(db.AllRuoloUtenteValues()))
	for _, r := range db.AllRuoloUtenteValues() {
		nomi = append(nomi, string(r))
	}
	return strings.Join(nomi, ", ")
}

// passwordImpostata: in database c'e' un hash bcrypt leggibile. Una colonna vuota, o piena di
// qualcosa che bcrypt non riconosce, non e' una password da proteggere: e' un utente che non entra.
func passwordImpostata(u db.Utente) bool {
	if !u.PasswordHash.Valid {
		return false
	}
	_, err := bcrypt.Cost([]byte(u.PasswordHash.String))
	return err == nil
}
