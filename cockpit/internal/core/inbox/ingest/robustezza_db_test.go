//go:build integrazione

// L4 — revisione del 25/09: i punti in cui un solo elemento poteva ancora fermare il lotto, o finire
// in scarto per colpa di qualcosa che non era suo.
package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// Un byte NUL nell'oggetto, nella cartella o nel Message-ID fa rifiutare il messaggio — ed è giusto:
// finisce in scarto. Ma lo scarto scriveva gli stessi campi, e PostgreSQL rifiutava anche lui: l'errore
// dello scarto non ha un savepoint sotto, abortiva il lotto, 5xx, e il worker ripeteva all'infinito lo
// stesso lotto con lo stesso elemento. È il poison pill che la fase 1 doveva togliere, rientrato da un
// campo diverso dal corpo.
func TestUnNulNeiCampiDellElementoFaUnoScartoENonFermaIlLotto(t *testing.T) {
	casi := []struct {
		nome    string
		guasta  func(*worker.MessaggioIn)
		colonna string
	}{
		{"oggetto", func(m *worker.MessaggioIn) { m.Oggetto = "RFQ di prova\x00 2" }, "oggetto"},
		{"cartella", func(m *worker.MessaggioIn) { m.Cartella = "In\x00box" }, "cartella"},
		{"message-id", func(m *worker.MessaggioIn) { m.MessageID = "<pp-2\x00@acme.example>" }, "message_id"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			p, s, casella, ctx := preparaPP(t)
			r, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: tre(c.guasta),
				Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}})
			if err != nil {
				t.Fatalf("NUL in %s: il lotto intero è fallito (sarebbe un 5xx ripetuto all'infinito): %v", c.nome, err)
			}
			if r.Inseriti != 2 || r.Falliti != 1 {
				t.Fatalf("inseriti=%d falliti=%d (attesi 2/1)", r.Inseriti, r.Falliti)
			}
			sc := scarti(t, p)
			if len(sc) != 1 || sc[0].Origine != "ingest" {
				t.Fatalf("scarti: %+v", sc)
			}
			var valore string
			if err := p.QueryRow(ctx, `SELECT coalesce(`+c.colonna+`, '') FROM ingest_scarto`).Scan(&valore); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(valore, "\x00") || !strings.Contains(valore, "�") {
				t.Errorf("%s dello scarto = %q: il NUL va sostituito con U+FFFD, non perso né conservato", c.colonna, valore)
			}
			var rimesso worker.MessaggioIn
			if err := json.Unmarshal(sc[0].Payload, &rimesso); err != nil {
				t.Errorf("payload illeggibile: %v", err)
			}
			if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
				t.Errorf("cursore = %v, atteso %v", cur, cursoreFinale)
			}
		})
	}
	// e il NUL nel CORPO resta ciò che era: l'elemento è uno scarto (TestPoisonPillVarianti)
}

// Lo stesso per un elemento che il worker non è riuscito a leggere: oggetto, cartella, Message-ID ed
// errore vengono da Outlook e dal worker, e il JSON del payload veniva scritto senza ripulirlo.
func TestUnNulInUnElementoSaltatoNonFermaIlLotto(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	r, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: tre(nil),
		Saltati: []worker.ElementoSaltato{{EntryID: "ENTRY-PP-SALTATO", Cartella: "In\x00box", MessageID: "<s\x00@acme.example>",
			Oggetto: "ogg\x00etto", Errore: "com_error\x00 su Body"}},
		Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}})
	if err != nil {
		t.Fatalf("un NUL in un elemento saltato ha fatto fallire il lotto: %v", err)
	}
	if r.Inseriti != 3 || r.Falliti != 1 {
		t.Fatalf("inseriti=%d falliti=%d (attesi 3/1)", r.Inseriti, r.Falliti)
	}
	var cartella, messageID, oggetto, errore string
	var payload []byte
	if err := p.QueryRow(ctx, `SELECT cartella, message_id, oggetto, errore, payload FROM ingest_scarto WHERE origine = 'lettura'`).
		Scan(&cartella, &messageID, &oggetto, &errore, &payload); err != nil {
		t.Fatalf("scarto di lettura: %v", err)
	}
	for nome, v := range map[string]string{"cartella": cartella, "message_id": messageID, "oggetto": oggetto, "errore": errore} {
		if strings.Contains(v, "\x00") || !strings.Contains(v, "�") {
			t.Errorf("%s = %q", nome, v)
		}
	}
	var sal worker.ElementoSaltato
	if err := json.Unmarshal(payload, &sal); err != nil || sal.EntryID != "ENTRY-PP-SALTATO" {
		t.Errorf("payload dello scarto di lettura: %+v %v", sal, err)
	}
}

// Un elemento saltato senza entry_id non è rileggibile e non ha una chiave con cui stare fra gli
// scarti. Era un errore del lotto: 5xx, lotto ripetuto identico, e con lui restavano fuori tutti gli
// altri. Ora si conta come fallito e il lotto va avanti.
func TestUnElementoSaltatoSenzaEntryIDNonFermaIlLotto(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	r, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: tre(nil),
		Saltati: []worker.ElementoSaltato{
			{EntryID: "", Cartella: cartellaPP, MessageID: "<senza-entry@acme.example>", Errore: "EntryID illeggibile"},
			{EntryID: "ENTRY-PP-SALTATO", Cartella: cartellaPP, Errore: "com_error su Body"},
		},
		Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}})
	if err != nil {
		t.Fatalf("un elemento saltato senza entry_id ha fatto fallire il lotto: %v", err)
	}
	if r.Inseriti != 3 || r.Falliti != 2 {
		t.Errorf("inseriti=%d falliti=%d (attesi 3/2: il saltato senza entry_id conta fra i falliti)", r.Inseriti, r.Falliti)
	}
	if sc := scarti(t, p); len(sc) != 1 || sc[0].EntryID != "ENTRY-PP-SALTATO" {
		t.Errorf("scarti = %+v: atteso il solo elemento rileggibile", sc)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
		t.Errorf("cursore = %v", cur)
	}
}

// Lo staging automatico è una comodità: un suo errore non fa cadere l'elemento. Era vero solo per gli
// errori detti da Go; un errore SQL abortiva la transazione, e l'istruzione successiva dell'elemento
// (il triage) falliva con «current transaction is aborted»: l'elemento finiva in scarto. L'errore qui
// è vero — un trigger che rifiuta l'inserimento del job di download —, non simulato.
func TestUnErroreSQLNelloStagingAutomaticoNonScartaLElemento(t *testing.T) {
	b := nuovoBancoControparte(t)
	b.cliente("ACME", "acme.example") // tre() scrive da mario.rossi@acme.example: il mittente è riconosciuto
	b.s.StagingAutomatico = true
	if _, err := b.pool.Exec(b.ctx, `
		CREATE FUNCTION prova_stage_rifiutato() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.tipo = 'stage_allegato' THEN RAISE EXCEPTION 'download rifiutato dalla prova'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER prova_stage_rifiutato BEFORE INSERT ON job FOR EACH ROW EXECUTE FUNCTION prova_stage_rifiutato();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = b.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS prova_stage_rifiutato ON job; DROP FUNCTION IF EXISTS prova_stage_rifiutato()`)
	})

	r, err := b.s.Ingerisci(b.ctx, Lotto{Casella: b.casella, Messaggi: tre(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Inseriti != 3 || r.Falliti != 0 {
		t.Fatalf("inseriti=%d falliti=%d (attesi 3/0): lo staging ha fatto cadere gli elementi: %+v", r.Inseriti, r.Falliti, r.Esiti)
	}
	if n := testutil.Conta(t, b.pool, "ingest_scarto"); n != 0 {
		t.Errorf("%d scarti: un download non accodato non è un motivo per scartare il messaggio", n)
	}
	// il trigger ha lavorato davvero: nessun download in coda
	var stage int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	if stage != 0 {
		t.Fatalf("%d download accodati: il trigger non ha rifiutato niente e la prova non prova niente", stage)
	}
	// dopo lo staging l'elemento ha fatto il resto: il triage è scritto
	if n := testutil.Conta(t, b.pool, "proposta_triage"); n != 3 {
		t.Errorf("proposte di triage = %d, attese 3: l'interpretazione dopo lo staging non è arrivata", n)
	}
	// e lo stato «in coda» scritto sull'allegato prima del job è stato annullato con lui
	var inCoda int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM allegato WHERE errore = 'in coda'`).Scan(&inCoda); err != nil {
		t.Fatal(err)
	}
	if inCoda != 0 {
		t.Errorf("%d allegati segnati «in coda» senza un job che li scarichi", inCoda)
	}
}

// La descrizione di una famiglia con le lettere accentate, più lunga di 120 byte: il taglio a byte la
// spezzava a metà di una «è», e l'UTF-8 non valido faceva rifiutare il candidato di codice — e con lui
// il messaggio intero, finito in scarto a ogni sync per la descrizione di una regola.
func TestUnaFamigliaConLeAccentateNonScartaIlMessaggio(t *testing.T) {
	b := nuovoBancoControparte(t)
	corta := "Famiglia " + strings.Repeat("è", 70)  // 79 caratteri, 149 byte: sta in varchar(120)
	lunga := "Famiglia " + strings.Repeat("è", 130) // 139 caratteri: si accorcia a 120, senza romperla
	regoleCon := func(descr string) string {
		r, _ := json.Marshal(map[string]any{"famiglie_codice": []map[string]any{
			{"regex": `\b\d{7}[A-Z]\b`, "descrizione": descr, "esempio": "1234567A"}}})
		return string(r)
	}
	b.clienteConRegole("ACME", regoleCon(corta), "acme.example")
	b.clienteConRegole("ACME DUE", regoleCon(lunga), "acme-due.example")

	m1 := b.richiestaDOfferta("acquisti@acme.example")
	m1.Oggetto = "RICHIESTA D'OFFERTA 1234567A"
	m2 := b.richiestaDOfferta("acquisti@acme-due.example")
	m2.Oggetto = "RICHIESTA D'OFFERTA 1234568B"
	r, err := b.s.Ingerisci(b.ctx, Lotto{Casella: b.casella, Messaggi: []worker.MessaggioIn{m1, m2}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Inseriti != 2 || r.Falliti != 0 {
		t.Fatalf("inseriti=%d falliti=%d (attesi 2/0): %+v", r.Inseriti, r.Falliti, r.Esiti)
	}
	famiglia := func(chiave, codice string) string {
		var f string
		if err := b.pool.QueryRow(b.ctx, `SELECT famiglia FROM candidato_codice WHERE messaggio_id = $1 AND codice = $2`,
			b.messaggio(chiave).MessaggioID, codice).Scan(&f); err != nil {
			t.Fatalf("candidato %s: %v", codice, err)
		}
		return f
	}
	if f := famiglia(m1.MessageID, "1234567A"); f != corta {
		t.Errorf("famiglia di 79 caratteri riscritta: %q", f)
	}
	f := famiglia(m2.MessageID, "1234568B")
	if !utf8.ValidString(f) || utf8.RuneCountInString(f) != 120 || !strings.HasPrefix(lunga, f) {
		t.Errorf("famiglia di 139 caratteri: %d caratteri, valida=%v", utf8.RuneCountInString(f), utf8.ValidString(f))
	}
}

// Dopo un ricalcolo in cui il ramo fornitore non vale più, i candidati verso una richiesta calcolati
// quando il mittente era un fornitore non restano accanto alla proposta nuova: si sostituivano solo
// quelli di aggancio e di codice.
func TestIlRicalcoloToglieICandidatiRichiestaDiUnaLetturaVecchia(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	f := b.fornitore("Fornitore Esempio", db.TipoFornitoreProcessi, "fornitore.example")
	th := b.rfq(acme, "RFQ PZ-001", "")

	const nostra = "<rev-nostra@azienda.example>"
	b.ingerisci(b.nostraMail(nostra, "CONV-REV-RICH", "vendite@fornitore.example", "RFQ ACME PZ-001", "Vi chiediamo offerta."))
	r := b.richiesta(th, f, db.StatoRichiestaFornitoreInviata, uuid.NullUUID{UUID: b.messaggio(nostra).MessaggioID, Valid: true}, "")
	const risposta = "<rev-risposta@fornitore.example>"
	b.ingerisci(b.rispostaDelFornitore(risposta, "CONV-REV-RICH", "vendite@fornitore.example", "R: RFQ ACME PZ-001", "In allegato la nostra offerta."))
	b.candidatoVerso(risposta, r.RichiestaID, db.RegolaRichiestaR1Conversazione, 80)

	// lo stesso dominio viene censito anche come cliente: il mittente diventa ambiguo, e il ramo
	// fornitore non vale più
	b.cliente("FORNITORE ESEMPIO CLIENTE", "fornitore.example")
	if _, err := b.s.Ritriage(b.ctx, "vendite@fornitore.example", "fornitore.example"); err != nil {
		t.Fatal(err)
	}
	m := b.messaggio(risposta)
	if m.ControparteTipo != db.TipoControparteAmbiguo {
		t.Fatalf("controparte dopo il ricalcolo: %s (attesa ambiguo, altrimenti la prova non prova niente)", m.ControparteTipo)
	}
	cand, err := b.q.ListCandidatiRichiesta(b.ctx, m.MessaggioID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cand) != 0 {
		t.Errorf("%d candidati verso una richiesta rimasti da quando il mittente era un fornitore: %+v", len(cand), cand)
	}
	if p, ok := b.proposta(m.MessaggioID); !ok || p.RichiestaProposta.Valid {
		t.Errorf("la proposta ricalcolata indica ancora una richiesta: %+v", p)
	}
}
