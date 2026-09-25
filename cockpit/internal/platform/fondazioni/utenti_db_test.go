//go:build integrazione

// L4 — la password di cockpit.toml fa nascere un utente (sicurezza M4): un utente nuovo non nasce con la
// password vuota ne' con il segnaposto dell'esempio, gli utenti gia' in database non cambiano, e chi non e'
// piu' nel file ma entra ancora lo si dice nel log.

package fondazioni_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/testutil"
)

type utenteSeme = struct{ Sigla, Nome, Ufficio, Ruolo, Password string }

func TestUnUtenteNuovoNonNasceConLaPasswordDellEsempio(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	q := db.New(p)
	ctx := context.Background()

	for _, caso := range []struct {
		nome, password, frase string
	}{
		{"segnaposto dell'esempio", fondazioni.PasswordEsempio, "quella dell'esempio"},
		{"segnaposto con spazi e minuscole", "  inserisci_password_iniziale ", "quella dell'esempio"},
		{"password vuota", "", "manca la password iniziale"},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			err := fondazioni.SeedUtenti(ctx, q, []utenteSeme{
				{"AM", "Amministratore", "IT", "admin", "una-password-vera"},
				{"NU", "Nuovo", "Commerciale", "operatore", caso.password},
			}, testutil.LogSilenzioso())
			if err == nil || !strings.Contains(err.Error(), "utente NU") || !strings.Contains(err.Error(), caso.frase) {
				t.Fatalf("errore %v, atteso che nomini NU e dica %q", err, caso.frase)
			}
			// il controllo viene prima di ogni scrittura: nemmeno l'altro utente del file e' nato
			if n := testutil.Conta(t, p, "utente"); n != 0 {
				t.Errorf("utenti in database dopo il rifiuto: %d", n)
			}
		})
	}
}

func TestGliUtentiGiaInDatabaseNonCambianoPerIlSegnaposto(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	q := db.New(p)
	ctx := context.Background()
	if err := fondazioni.SeedUtenti(ctx, q, []utenteSeme{{"AM", "Amministratore", "IT", "admin", "la-sua-password"}}, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	// uno nato prima senza password (per esempio da un file vecchio)
	if _, err := p.Exec(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ('VE', 'Vecchio', 'Commerciale', 'operatore')`); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := fondazioni.SeedUtenti(ctx, q, []utenteSeme{
		{"AM", "Amministratore", "IT", "admin", fondazioni.PasswordEsempio},
		{"VE", "Vecchio", "Commerciale", "operatore", fondazioni.PasswordEsempio},
	}, slog.New(slog.NewTextHandler(&log, nil))); err != nil {
		t.Fatalf("gli utenti gia' in database non fermano l'avvio: %v", err)
	}
	am, err := q.GetUtentePerSigla(ctx, "AM")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(am.PasswordHash.String), []byte("la-sua-password")) != nil {
		t.Error("la password di AM e' cambiata")
	}
	ve, err := q.GetUtentePerSigla(ctx, "VE")
	if err != nil {
		t.Fatal(err)
	}
	if ve.PasswordHash.Valid {
		t.Error("il segnaposto dell'esempio e' diventato la password di VE")
	}
	if !strings.Contains(log.String(), "VE") || !strings.Contains(log.String(), "esempio") {
		t.Errorf("il log non dice che la password di VE non e' stata usata: %s", log.String())
	}
}

func TestChiNonENelFileMaEAttivoSiDiceNelLog(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	q := db.New(p)
	ctx := context.Background()
	if err := fondazioni.SeedUtenti(ctx, q, []utenteSeme{
		{"AM", "Amministratore", "IT", "admin", "una-password"},
		{"EX", "Uscito", "Commerciale", "operatore", "un'altra-password"},
		{"SP", "Spento", "Commerciale", "operatore", "un'altra-ancora"},
	}, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE utente SET attivo = false WHERE sigla = 'SP'`); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if err := fondazioni.SeedUtenti(ctx, q, []utenteSeme{{"AM", "Amministratore", "IT", "admin", ""}},
		slog.New(slog.NewTextHandler(&log, nil))); err != nil {
		t.Fatal(err)
	}
	righe := strings.Split(log.String(), "\n")
	trovata := false
	for _, r := range righe {
		if strings.Contains(r, "non e' piu' fra gli [[utenti]]") {
			if strings.Contains(r, "utente=SP") || strings.Contains(r, "utente=AM") {
				t.Errorf("segnalato chi non doveva esserlo: %s", r)
			}
			trovata = trovata || strings.Contains(r, "utente=EX")
		}
	}
	if !trovata {
		t.Errorf("EX entra ancora e il log non lo dice:\n%s", log.String())
	}
}
