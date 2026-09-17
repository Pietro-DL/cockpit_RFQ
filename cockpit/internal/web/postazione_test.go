package web

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
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
		{WorkerNome: "outlook@PC-FRANCESCO", WorkerTipo: db.WorkerTipoOutlook, UltimoContatto: ora.Add(-20 * time.Second), OutlookOk: true, CaselleAperte: []uuid.UUID{francesco},
			Avviso: pgtype.Text{String: "1 caselle dichiarate ma non autorizzate", Valid: true}},
		// fermo da dieci minuti
		{WorkerNome: "outlook@PC-LUIGI", WorkerTipo: db.WorkerTipoOutlook, UltimoContatto: ora.Add(-10 * time.Minute), OutlookOk: true, CaselleAperte: []uuid.UUID{luigi, commerciale}},
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

// L1 — blocco 2 del 3R: la testata legge ultimo_contatto, e ultimo_claim non c'entra.
//
// Qui in una riga c'è il difetto e la correzione insieme: un worker dentro un sync lungo ha un
// ultimo_claim di tre minuti fa (il claim si conclude solo quando NON sta lavorando) e un contatto di
// cinque secondi fa (il battito). Con la regola vecchia era OFFLINE; è il caso che l'operatore
// vedeva mentre il sync girava.
func TestPresenzaLeggeIlContattoNonIlClaim(t *testing.T) {
	ora := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	casella, pc := uuid.New(), uuid.New()
	caselle := []db.Casella{{CasellaID: casella, Nome: "Francesco", Canale: db.CanaleOutlook, Attiva: true}}
	credenziali := []db.WorkerCredenziale{
		{WorkerNome: "outlook@PC-FRANCESCO", WorkerTipo: db.WorkerTipoOutlook, Attivo: true,
			PostazioneID: uuid.NullUUID{UUID: pc, Valid: true}, Caselle: []uuid.UUID{casella}},
		{WorkerNome: "analisi@PC-FRANCESCO", WorkerTipo: db.WorkerTipoAnalisi, Attivo: true},
	}
	postazioni := []db.Postazione{{PostazioneID: pc, NomeHost: "PC-FRANCESCO"}}
	vecchio := ora.Add(-3 * time.Minute)

	casi := []struct {
		nome     string
		contatto time.Time
		stato    string
	}{
		{"dentro un job lungo: claim di 3 minuti fa, battito di 5 secondi fa", ora.Add(-5 * time.Second), "attiva"},
		{"un secondo prima della soglia: un giro di claim perso, ci sta", ora.Add(-api.PresenzaOnlineEntro + time.Second), "attiva"},
		{"un secondo dopo la soglia: due giri persi, è un guasto", ora.Add(-api.PresenzaOnlineEntro - time.Second), "offline"},
	}
	for _, c := range casi {
		presenze := []db.ListWorkerPresenzaRow{
			{WorkerNome: "outlook@PC-FRANCESCO", WorkerTipo: db.WorkerTipoOutlook, UltimoClaim: &vecchio,
				UltimoContatto: c.contatto, OutlookOk: true, CaselleAperte: []uuid.UUID{casella}},
			{WorkerNome: "analisi@PC-FRANCESCO", WorkerTipo: db.WorkerTipoAnalisi, UltimoClaim: &vecchio,
				UltimoContatto: c.contatto},
		}
		out := statoCaselle(caselle, credenziali, presenze, postazioni, ora)
		if len(out) != 1 || out[0].Stato != c.stato {
			t.Errorf("%s: stato %q, atteso %q (%q)", c.nome, out[0].Stato, c.stato, out[0].Dettaglio)
		}
		// il chip dell'analisi è scritto a parte e deve dire la stessa cosa
		atteso := "analisi attiva"
		if c.stato == "offline" {
			atteso = "analisi OFFLINE"
		}
		if a := statoAnalisi(credenziali, presenze, ora); a.Etichetta != atteso {
			t.Errorf("%s: chip analisi %q, atteso %q", c.nome, a.Etichetta, atteso)
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
