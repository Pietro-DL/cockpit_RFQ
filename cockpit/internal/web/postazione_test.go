package web

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/db"
)

// L1 — M2: i tre stati della testata per casella, calcolati da caselle, credenziali e presenze
// senza database. Ogni riga è un caso che la testata deve distinguere, non riassumere.
func TestM2StatoCaselleTreStati(t *testing.T) {
	ora := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	francesco, commerciale, luigi, archivio := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	pcF, pcL := uuid.New(), uuid.New()
	caselle := []db.Casella{
		{CasellaID: francesco, Nome: "Francesco", Canale: db.CanaleOutlook, Attiva: true},
		{CasellaID: commerciale, Nome: "Commerciale", Canale: db.CanaleOutlook, Attiva: true},
		{CasellaID: luigi, Nome: "Luigi", Canale: db.CanaleOutlook, Attiva: true},
		{CasellaID: archivio, Nome: "Archivio", Canale: db.CanaleOutlook, Attiva: true},
	}
	credenziali := []db.WorkerCredenziale{
		{WorkerNome: "outlook@PC-FRANCESCO", WorkerTipo: db.WorkerTipoOutlook, Attivo: true, PostazioneID: uuid.NullUUID{UUID: pcF, Valid: true}, Caselle: []uuid.UUID{francesco, commerciale}},
		{WorkerNome: "outlook@PC-LUIGI", WorkerTipo: db.WorkerTipoOutlook, Attivo: true, PostazioneID: uuid.NullUUID{UUID: pcL, Valid: true}, Caselle: []uuid.UUID{luigi, commerciale}},
		{WorkerNome: "analisi@PC-FRANCESCO", WorkerTipo: db.WorkerTipoAnalisi, Attivo: true},
	}
	postazioni := []db.Postazione{{PostazioneID: pcF, NomeHost: "PC-FRANCESCO"}, {PostazioneID: pcL, NomeHost: "PC-LUIGI"}}
	presenze := []db.ListWorkerPresenzaRow{
		// vivo, ha risolto Francesco ma non Commerciale
		{WorkerNome: "outlook@PC-FRANCESCO", WorkerTipo: db.WorkerTipoOutlook, UltimoClaim: ora.Add(-20 * time.Second), OutlookOk: true, CaselleAperte: []uuid.UUID{francesco},
			Avviso: pgtype.Text{String: "1 caselle dichiarate ma non autorizzate", Valid: true}},
		// fermo da dieci minuti
		{WorkerNome: "outlook@PC-LUIGI", WorkerTipo: db.WorkerTipoOutlook, UltimoClaim: ora.Add(-10 * time.Minute), OutlookOk: true, CaselleAperte: []uuid.UUID{luigi, commerciale}},
	}
	out := statoCaselle(caselle, credenziali, presenze, postazioni, ora)
	per := map[string]statoCasella{}
	for _, s := range out {
		per[s.Nome] = s
	}
	casi := []struct{ nome, stato, dentro string }{
		{"Francesco", "attiva", "attiva su PC-FRANCESCO"},
		{"Commerciale", "non_risolta", "non trova questa casella"}, // PC-FRANCESCO vivo senza averla; PC-LUIGI offline: vince il primo
		{"Luigi", "offline", "OFFLINE: ultimo contatto 10 min fa"},
		{"Archivio", "non_configurata", "nessun [[worker]]"},
	}
	for _, c := range casi {
		s, ok := per[c.nome]
		if !ok {
			t.Fatalf("%s: assente dalla testata", c.nome)
		}
		if s.Stato != c.stato || !strings.Contains(s.Dettaglio, c.dentro) {
			t.Errorf("%s: stato=%s dettaglio=%q; attesi %s con %q", c.nome, s.Stato, s.Dettaglio, c.stato, c.dentro)
		}
	}
	if !strings.Contains(per["Francesco"].Dettaglio, "non autorizzate") {
		t.Errorf("l'avviso del claim (Q18) non compare nel dettaglio: %q", per["Francesco"].Dettaglio)
	}
	if a := statoAnalisi(credenziali, presenze, ora); a.Etichetta != "analisi mai avviata" {
		t.Errorf("analisi censita e mai vista: %q", a.Etichetta)
	}

	// Outlook che non risponde: il worker è vivo, la casella non è servita, e la testata lo dice
	presenze[0].OutlookOk, presenze[0].CaselleAperte = false, nil
	out = statoCaselle(caselle, credenziali, presenze, postazioni, ora)
	for _, s := range out {
		if s.Nome == "Francesco" && (s.Stato != "non_risolta" || !strings.Contains(s.Dettaglio, "Outlook non risponde")) {
			t.Errorf("Outlook giù: stato=%s dettaglio=%q", s.Stato, s.Dettaglio)
		}
	}
	// nessun worker mai visto: offline con «mai stato avviato»
	out = statoCaselle(caselle, credenziali, nil, postazioni, ora)
	for _, s := range out {
		if s.Nome == "Francesco" && (s.Stato != "offline" || !strings.Contains(s.Dettaglio, "mai stato avviato")) {
			t.Errorf("nessuna presenza: stato=%s dettaglio=%q", s.Stato, s.Dettaglio)
		}
	}
}

// P1 — chi può usare una postazione: il proprietario o un admin, e solo se attiva.
func TestPuoUsarePostazione(t *testing.T) {
	fp, lu := uuid.New(), uuid.New()
	p := db.Postazione{PostazioneID: uuid.New(), NomeHost: "PC-FRANCESCO", UtenteID: uuid.NullUUID{UUID: fp, Valid: true}, Attiva: true}
	if !puoUsarePostazione(&db.Utente{UtenteID: fp, Ruolo: db.RuoloUtenteOperatore}, p) {
		t.Error("il proprietario non può usare la propria postazione")
	}
	if puoUsarePostazione(&db.Utente{UtenteID: lu, Ruolo: db.RuoloUtenteTecnico}, p) {
		t.Error("un altro utente può usare la postazione di Francesco")
	}
	if !puoUsarePostazione(&db.Utente{UtenteID: lu, Ruolo: db.RuoloUtenteAdmin}, p) {
		t.Error("l'admin non può usare una postazione")
	}
	p.Attiva = false
	if puoUsarePostazione(&db.Utente{UtenteID: fp, Ruolo: db.RuoloUtenteAdmin}, p) {
		t.Error("una postazione disattivata è utilizzabile")
	}
	if puoUsarePostazione(nil, p) {
		t.Error("nessun utente può usare una postazione")
	}
}
