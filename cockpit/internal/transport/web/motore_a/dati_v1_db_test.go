//go:build integrazione

// L4 — V1 (emendamento di V1 §9.9; piano della 0023 §14 passo 3, corsia motore; A-27 e A-28 la parte L4): le letture
// nuove del modello di /motore-a/dati sul database di prova (COCKPIT_TEST_DSN), nello stesso snapshot della fotografia,
// sulla scena ACME di servizio_db_test.go (scenaMA):
//   - senza righe di V1: nessuna occorrenza non attiva, F8 mai previsto, nessuna bozza; i gruppi del prodotto con
//     l'impronta_contenuto di un'importazione fatta adesso (64 esadecimali) e i nodi, la fonte confermata e autorizzata
//     (lo STEP autorizzato della scena);
//   - con il PDF scartato (le righe del ciclo dei file della 0023: stato, evento, rimozione) e una riga di F8 in errore:
//     il PDF fra le occorrenze non attive, f8_stato «errore» nel JSON;
//   - il calcolo non cambia il database;
//   - (revisione della corsia, ondata B) l'identità registrata da una conferma formato 3 vera, la ricodifica legacy e il
//     componente archiviato, la bozza aperta formato 3 con un registro vero e lo STEP in «Altri file», la completezza
//     dopo un caricamento (TestL4MotoreALIdentitaRegistrataELaBozza).
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx), come nella scena.
package motore_a

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func TestL4MotoreALeLettureDiV1(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	prima := testutil.FotoDelDatabase(t, p)
	ris, err := svc.Calcola(ctx, s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if len(ris.V1.NonAttivi) != 0 || ris.V1.F8 != "" || len(ris.V1.Bozze) != 0 {
		t.Errorf("le letture di V1 senza righe: %+v", ris.V1)
	}
	pg, err := svc.Pagina(ctx, RichiestaPagina{Thread: s.thread})
	if err != nil {
		t.Fatal(err)
	}
	var bom *propostabom.GruppoDelModello
	for i := range pg.Gruppi {
		if pg.Gruppi[i].Chiave == propostabom.GruppoBOM {
			bom = &pg.Gruppi[i]
		}
	}
	if bom == nil || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(bom.ImprontaContenuto) || len(bom.Nodi) != 2 || bom.Importazione != nil ||
		bom.RadiceLetterale != "9120100A" || bom.NomeFile != "9120100A.stp" {
		t.Fatalf("il gruppo bom della scena: %+v", bom)
	}
	if pg.Fonte == nil || !pg.Fonte.Confermata || !pg.Fonte.Autorizzata || pg.Fonte.DocumentoID == nil || *pg.Fonte.DocumentoID != s.docStep {
		t.Errorf("la fonte del prodotto: %+v", pg.Fonte)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo ha cambiato il database: %v", d)
	}

	// il PDF scartato e F8 in errore
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, q := range []struct {
		sql string
		arg []any
	}{
		{`INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
			estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
		  SELECT m.thread_id, a.allegato_id, 'scartato', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno,
			a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, $2, $2
		    FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{s.aPdf, s.utente}},
		{`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, origine, autore_id)
		  VALUES ($1, $2, 'scarta', 'attivo', 'scartato', 'non serve alla RFQ', '{}', '{}', 'ponte', $3)`, []any{s.thread, s.aPdf, s.utente}},
		{`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id, sha256)
		  SELECT e.thread_id, e.allegato_id, e.evento_id, a.sha256 FROM file_rfq_evento e JOIN allegato a ON a.allegato_id = e.allegato_id
		   WHERE e.allegato_id = $1 AND e.azione = 'scarta'`, []any{s.aPdf}},
		{`INSERT INTO f8_stato (thread_id, chiave_intento, stato, errore) VALUES ($1, gen_random_uuid(), 'errore', 'prova L4 di V1')`, []any{s.thread}},
	} {
		if _, err := tx.Exec(ctx, q.sql, q.arg...); err != nil {
			t.Fatalf("le righe di V1: %v\n%s", err, q.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT delle righe di V1: %v", err)
	}
	prima = testutil.FotoDelDatabase(t, p)
	ris, err = svc.Calcola(ctx, s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if ris.V1.NonAttivi[s.aPdf] != "scartato" || len(ris.V1.NonAttivi) != 1 || ris.V1.F8 != F8Errore {
		t.Errorf("le letture di V1 con il PDF scartato e F8 in errore: %+v", ris.V1)
	}
	pg, err = svc.Pagina(ctx, RichiestaPagina{Thread: s.thread})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(pg)
	if err != nil {
		t.Fatal(err)
	}
	var j struct {
		F8Stato string            `json:"f8_stato"`
		Gruppi  []json.RawMessage `json:"gruppi"`
	}
	if err := json.Unmarshal(b, &j); err != nil || j.F8Stato != F8Errore || len(j.Gruppi) == 0 {
		t.Errorf("il JSON di /dati: f8_stato %q, gruppi %d (%v)", j.F8Stato, len(j.Gruppi), err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo ha cambiato il database: %v", d)
	}
}

// passoSQL: un'istruzione SQL con i suoi argomenti.
type passoSQL struct {
	sql string
	arg []any
}

// inTransazione: le istruzioni in una transazione, con il COMMIT (i controlli differiti della 0022 e della 0023 girano lì).
func inTransazione(t *testing.T, p *pgxpool.Pool, passi ...passoSQL) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, x := range passi {
		if _, err := tx.Exec(ctx, x.sql, x.arg...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT: %v", err)
	}
}

// datiV1JSON: i campi di V1 del JSON di /dati che la prova guarda.
type datiV1JSON struct {
	Gruppi []struct {
		Chiave            string `json:"chiave"`
		ImprontaContenuto string `json:"impronta_contenuto"`
		Nodi              []struct {
			Rif         string `json:"rif"`
			Completezza *struct {
				Requisito string `json:"requisito"`
				Esito     string `json:"esito"`
			} `json:"completezza"`
			DecisioneEsistente *struct {
				ComponenteID       string                         `json:"componente_id"`
				IdentitaRegistrata propostabom.IdentitaRegistrata `json:"identita_registrata"`
				Superata           bool                           `json:"certificazione_superata"`
			} `json:"decisione_esistente"`
		} `json:"nodi"`
	} `json:"gruppi"`
	MotivoSenzaGruppi *struct {
		Codice   string `json:"codice"`
		Testo    string `json:"testo"`
		Allegati []struct {
			AllegatoID string `json:"allegato_id"`
			Stato      string `json:"stato"`
		} `json:"allegati"`
	} `json:"motivo_senza_gruppi"`
	BozzaDelProdotto *struct {
		BozzaID        string                     `json:"bozza_id"`
		Versione       int                        `json:"versione"`
		GruppoCorrente propostabom.GruppoCorrente `json:"gruppo_corrente"`
	} `json:"bozza_del_prodotto"`
	// ondata C (U2 §2.4, §5.3)
	Fonte *struct {
		Confermata        bool           `json:"confermata"`
		Autorizzata       bool           `json:"autorizzata"`
		ConfermataIl      *string        `json:"confermata_il"`
		ConfermataDa      *maPersonaJSON `json:"confermata_da"`
		AutorizzataIl     *string        `json:"autorizzata_il"`
		AutorizzataDa     *maPersonaJSON `json:"autorizzata_da"`
		AutorizzataMotivo string         `json:"autorizzata_motivo"`
	} `json:"fonte"`
	ConfermaCorrente *struct {
		ConfermaID     string `json:"conferma_id"`
		Numero         int    `json:"numero"`
		Valida         bool   `json:"valida"`
		VerificaAdesso struct {
			Stato      string   `json:"stato"`
			Verificata *bool    `json:"verificata"`
			Motivi     []string `json:"motivi"`
		} `json:"verifica_adesso"`
		PropostaAggiornata     json.RawMessage `json:"proposta_aggiornata"`
		PropostaNonConfrontata string          `json:"proposta_non_confrontata"`
	} `json:"conferma_corrente"`
}

// TestL4MotoreALIdentitaRegistrataELaBozza (A-26 e A-27, la parte L4 di motore_a; A-28; EM §5.1, §6.4.1, §9.9; revisione
// della corsia motore, ondata B): sulla scena ACME, con righe vere delle tabelle della 0022 e della 0023 scritte come le
// scrive R5 e R2 (le stesse forme dei casi di platform/migrazioni):
//   - la riga legacy di #2 decisa con il figlio; una conferma formato 3 del prodotto che certifica il figlio con
//     l'emittente del partner, non presunto, con il namespace: /dati dà nel nodo #2 decisione_esistente.identita_registrata
//     con quell'identità, letta da ListIdentitaRegistrate nello snapshot;
//   - una ricodifica legacy del figlio (il codice di oggi non è quello certificato): il cliente presunto, senza namespace,
//     con il codice di oggi e certificazione_superata; la colonna valida della query dice lo stesso, ma il Go la ricalcola
//     (IdentitaDelRegistrato) e non la legge;
//   - il figlio archiviato: la certificazione superata, nel Go come nella colonna valida;
//   - una bozza aperta formato 3 del prodotto con un registro vero dell'adattatore: bozza_del_prodotto con il gruppo
//     corrente «bom» sulla stessa occorrenza; con lo STEP in «Altri file» il gruppo corrente è assente /
//     occorrenza_non_attiva con l'allegato, il gruppo «bom» non sta fra i gruppi e motivo_senza_gruppi è step_non_attivo;
//   - dopo il caricamento di un 2D del prodotto la completezza della radice in /dati cambia, il registro della bozza no.
func TestL4MotoreALIdentitaRegistrataELaBozza(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	cliente := "cliente:" + s.cliente.String()
	rifProdotto := "componente:" + s.prodotto.String()
	shaStep := strings.Repeat("a1", 32)
	pagina := func(t *testing.T) datiV1JSON {
		t.Helper()
		pg, err := svc.Pagina(ctx, RichiestaPagina{Thread: s.thread, Prodotto: rifProdotto})
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(pg)
		if err != nil {
			t.Fatal(err)
		}
		var j datiV1JSON
		if err := json.Unmarshal(b, &j); err != nil {
			t.Fatal(err)
		}
		return j
	}
	nodoDue := func(t *testing.T, j datiV1JSON) (decisione *struct {
		ComponenteID       string                         `json:"componente_id"`
		IdentitaRegistrata propostabom.IdentitaRegistrata `json:"identita_registrata"`
		Superata           bool                           `json:"certificazione_superata"`
	}) {
		t.Helper()
		for _, g := range j.Gruppi {
			if g.Chiave != propostabom.GruppoBOM {
				continue
			}
			for _, n := range g.Nodi {
				if strings.HasSuffix(n.Rif, ":#2") {
					return n.DecisioneEsistente
				}
			}
		}
		t.Fatalf("il nodo #2 del gruppo bom non c'è in /dati: %+v", j.Gruppi)
		return nil
	}
	valida := func(t *testing.T) bool {
		t.Helper()
		rr, err := db.New(p).ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: s.thread, Componenti: []uuid.UUID{s.figlio}})
		if err != nil || len(rr) != 1 {
			t.Fatalf("ListIdentitaRegistrate: %+v %v", rr, err)
		}
		return rr[0].Valida
	}

	// la riga di #2 decisa con il figlio, e l'arco della working prodotto → figlio (qta 2), che la conferma certifica
	inTransazione(t, p,
		passoSQL{`UPDATE componente_proposta SET stato = 'confermata', componente_id = $2, deciso_da = $3, deciso_il = now()
		           WHERE thread_id = $1 AND chiave = '#2'`, []any{s.thread, s.figlio, s.utente}},
		passoSQL{`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 2, 'step', $4)`,
			[]any{s.thread, s.prodotto, s.figlio, s.utente}})

	// la bozza formato 3 che la conferma chiude, e la conferma formato 3 n. 1 del prodotto (le forme di R5)
	snap := map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-p", "componente_id": s.prodotto.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "9120100", "modo": "registrato"}},
			map[string]any{"chiave_bozza": "k-f", "componente_id": s.figlio.String(), "op": "riusa", "tipo": "sciolto", "origine": "registrato",
				"identita": map[string]any{"emittente": "partner:beta", "emittente_presunto": false, "namespace": "beta-dis", "codice": "9120101", "rev": "2",
					"modo": "decisione", "decisione": "id-1"}},
		},
		"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-f", "qta": 2, "origine": "conferma"}}, "decisioni": []any{}}
	sj, _ := json.Marshal(snap)
	mappa, _ := json.Marshal(map[string]string{"k-p": s.prodotto.String(), "k-f": s.figlio.String()})
	// la testata e la sua prima versione nella stessa transazione: la testata cita la versione corrente (una FK differita)
	b1 := uuid.New()
	var c1 uuid.UUID
	inTransazione(t, p,
		passoSQL{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ($1, $2, $3, 3, 0, 0, repeat('1', 64), $4, $4)`, []any{b1, s.thread, s.prodotto, s.utente}},
		passoSQL{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
			chiave_salvataggio, impronta_richiesta) VALUES ($1, 1, $2::jsonb, repeat('5', 64), 0, 0, repeat('1', 64), $3, gen_random_uuid(), repeat('4', 64))`,
			[]any{b1, string(sj), s.utente}},
		passoSQL{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
			[]any{b1, s.utente}})
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
		firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot,
		delta, mappa, decisioni, criterio_verifica, chiave_idempotenza, confermato_da, importazioni_citate, verifica)
		SELECT $1, $2, 1, NULL, 3, false, repeat('3', 64), repeat('2', 64), NULL, 0, repeat('1', 64),
		       COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = $1), 0),
		       $3, 1, '{"rotta": "motore-a/bom/conferma"}', $4::jsonb, repeat('5', 64), '{"formato": 3}', $5::jsonb, '[]', 'nessuno', gen_random_uuid(), $6,
		       '{}'::uuid[], '{"regola": "prima-istanza-1"}'
		RETURNING conferma_id`, s.thread, s.prodotto, b1, string(sj), string(mappa), s.utente).Scan(&c1); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("la conferma formato 3: %v", err)
	}
	for _, x := range []passoSQL{
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo,
			step_strutturale_id, step_sha256, emittente, emittente_presunto, namespace)
		  SELECT $1, $2, $3, true, 'k-p', '9120100', NULL, NULL, 'finito', $4, d.sha256, $5, true, NULL FROM documento d WHERE d.documento_id = $4`,
			[]any{c1, s.thread, s.prodotto, s.docStep, cliente}},
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo,
			emittente, emittente_presunto, namespace) VALUES ($1, $2, $3, false, 'k-f', '9120101', '2', NULL, 'sciolto', 'partner:beta', false, 'beta-dis')`,
			[]any{c1, s.thread, s.figlio}},
		{`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ($1, $2, $3, 2, 'conferma')`, []any{c1, s.prodotto, s.figlio}},
		{`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = $2 WHERE bozza_id = $1`, []any{b1, s.utente}},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.arg...); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("la conferma formato 3: %v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT della conferma formato 3: %v", err)
	}

	// ondata C (U2 §2.4, §5.3; A-31): la data e l'autore della fonte, prima dagli atti di prima di V1 (il documento,
	// l'autorizzazione non registrata), poi dagli eventi della fonte; conferma_corrente della conferma formato 3 n. 1,
	// senza registro: il motivo del confronto mancato, la verifica di adesso non calcolata, mai «sì»
	t.Run("la fonte con date e autori e conferma_corrente", func(t *testing.T) {
		j := pagina(t)
		f := j.Fonte
		if f == nil || !f.Confermata || !f.Autorizzata || f.ConfermataIl == nil || f.ConfermataDa == nil ||
			f.ConfermataDa.UtenteID != s.utente.String() || f.ConfermataDa.Nome != "Prova ACME A1d" ||
			f.AutorizzataIl != nil || f.AutorizzataMotivo != AutorizzazioneNonRegistrata {
			t.Errorf("la fonte prima di V1: %+v", f)
		}
		cc := j.ConfermaCorrente
		if cc == nil || cc.ConfermaID != c1.String() || cc.Numero != 1 || cc.PropostaNonConfrontata != MotivoConfermaSenzaRegistro ||
			len(cc.PropostaAggiornata) != 0 || cc.VerificaAdesso.Stato != StatoVerificaNonCalcolata || cc.VerificaAdesso.Verificata != nil {
			t.Errorf("conferma_corrente: %+v", cc)
		}
		var autore uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('BZ', 'Seconda ACME', 'Tecnico') RETURNING utente_id`).Scan(&autore); err != nil {
			t.Fatal(err)
		}
		inTransazione(t, p,
			passoSQL{`INSERT INTO fonte_step_evento (thread_id, prodotto_id, azione, allegato_id, sha256, documento_id, autore_id)
				SELECT $1, $2, 'conferma_documento', $3, d.sha256, d.documento_id, $4 FROM documento d WHERE d.documento_id = $5`,
				[]any{s.thread, s.prodotto, s.aStep, autore, s.docStep}},
			passoSQL{`INSERT INTO fonte_step_evento (thread_id, prodotto_id, azione, allegato_id, sha256, documento_id, autore_id)
				SELECT $1, $2, 'autorizzazione', $3, d.sha256, d.documento_id, $4 FROM documento d WHERE d.documento_id = $5`,
				[]any{s.thread, s.prodotto, s.aStep, autore, s.docStep}})
		f = pagina(t).Fonte
		if f == nil || f.ConfermataDa == nil || f.ConfermataDa.Nome != "Seconda ACME" || f.AutorizzataDa == nil ||
			f.AutorizzataDa.UtenteID != autore.String() || f.AutorizzataIl == nil || f.AutorizzataMotivo != "" {
			t.Errorf("la fonte dagli eventi: %+v", f)
		}
	})

	t.Run("la certificazione valida", func(t *testing.T) {
		d := nodoDue(t, pagina(t))
		want := propostabom.IdentitaRegistrata{Emittente: "partner:beta", Namespace: "beta-dis", Codice: "9120101", Rev: "2", Modo: propostabom.ModoRegistrato}
		if d == nil || d.ComponenteID != s.figlio.String() || d.IdentitaRegistrata != want || d.Superata {
			t.Errorf("l'identità registrata del figlio certificato: %+v", d)
		}
		if !valida(t) {
			t.Error("la colonna valida della query dice superata una certificazione valida")
		}
	})
	t.Run("la ricodifica legacy", func(t *testing.T) {
		inTransazione(t, p, passoSQL{`UPDATE componente SET codice = '9120109' WHERE componente_id = $1`, []any{s.figlio}})
		d := nodoDue(t, pagina(t))
		want := propostabom.IdentitaRegistrata{Emittente: cliente, EmittentePresunto: true, Codice: "9120109", Rev: "2", Modo: propostabom.ModoRegistrato}
		if d == nil || d.IdentitaRegistrata != want || !d.Superata {
			t.Errorf("l'identità registrata dopo la ricodifica legacy: %+v", d)
		}
		if valida(t) {
			t.Error("la colonna valida della query dice valida una certificazione superata")
		}
		inTransazione(t, p, passoSQL{`UPDATE componente SET codice = '9120101' WHERE componente_id = $1`, []any{s.figlio}})
		if d := nodoDue(t, pagina(t)); d == nil || d.Superata || !valida(t) {
			t.Errorf("con il codice di nuovo quello certificato la certificazione torna valida: %+v", d)
		}
	})

	// la bozza aperta formato 3 con il registro vero dell'adattatore (la forma di R2: la versione 1 cita il registro)
	ris, err := svc.Calcola(ctx, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &ris.Esito, Thread: ris.EsitoThread, Regole: ris.Regole, Prodotto: rifProdotto,
		Gruppo: propostabom.GruppoBOM, NonAttivi: ris.V1.NonAttivi, Foto: &ris.Foto})
	if err != nil {
		t.Fatalf("il registro dell'adattatore: %v", err)
	}
	col := reg.Colonne()
	var r1 uuid.UUID
	b2 := uuid.New()
	if err := p.QueryRow(ctx, `INSERT INTO distinta_importazione (thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole,
		formato_registro, versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15) RETURNING importazione_id`,
		col.ThreadID, col.ProdottoID, col.Impronta, col.ImprontaContenuto, col.ImprontaEvidenze, col.ImprontaRegole, col.FormatoRegistro,
		col.VersioneImportazione, col.Gruppo, col.FonteAllegatoID, col.FonteSha256, col.FonteRadice, col.AllegatiCitati, string(reg.Canonico), s.utente).Scan(&r1); err != nil {
		t.Fatalf("il registro: %v", err)
	}
	snap2, _ := json.Marshal(map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"sorgente": map[string]any{"allegato_id": s.aStep.String(), "sha256": shaStep, "gruppo": propostabom.GruppoBOM, "importazione": reg.Impronta},
		"nodi": []any{map[string]any{"chiave_bozza": "k-p", "componente_id": s.prodotto.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
			"caselle":  []any{map[string]any{"allegato_id": s.aStep.String(), "gruppo": propostabom.GruppoBOM, "rif": col.FonteRadice}},
			"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "9120100", "modo": "registrato"}}},
		"archi": []any{}, "decisioni": []any{}})
	inTransazione(t, p,
		passoSQL{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore, firma_base, creata_da,
			aggiornata_da) VALUES ($1, $2, $3, 3, $4, 1, 0, repeat('1', 64), $5, $5)`, []any{b2, s.thread, s.prodotto, c1, s.utente}},
		passoSQL{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
			VALUES ($1, 1, $2::jsonb, repeat('6', 64), $3, 1, 0, repeat('1', 64), $4, $5, 'bom', $6, $7, gen_random_uuid(), repeat('4', 64))`,
			[]any{b2, string(snap2), c1, s.aStep, shaStep, s.utente, r1}},
		passoSQL{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
			[]any{b2, s.utente}})

	j := pagina(t)
	if j.BozzaDelProdotto == nil || j.BozzaDelProdotto.BozzaID != b2.String() || j.BozzaDelProdotto.Versione != 1 ||
		j.BozzaDelProdotto.GruppoCorrente != (propostabom.GruppoCorrente{Chiave: propostabom.GruppoBOM, Come: propostabom.ComeStessaOccorrenza}) {
		t.Errorf("la bozza del prodotto con lo STEP attivo: %+v", j.BozzaDelProdotto)
	}
	var radicePrima string
	for _, g := range j.Gruppi {
		if g.Chiave == propostabom.GruppoBOM {
			if g.ImprontaContenuto != col.ImprontaContenuto {
				t.Errorf("l'impronta del gruppo bom in /dati %s non è quella del registro della bozza %s", g.ImprontaContenuto, col.ImprontaContenuto)
			}
			for _, n := range g.Nodi {
				if n.Rif == col.FonteRadice && n.Completezza != nil {
					radicePrima = n.Completezza.Esito
				}
			}
		}
	}

	t.Run("il caricamento di un 2D del prodotto", func(t *testing.T) {
		// il file arriva come gli altri della scena: l'allegato del messaggio, i fatti, la proposta del 2D dal nome del file
		shaPdf := strings.Repeat("a3", 32)
		var a uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, 3, '9120100A_1.pdf', 'pdf', 'file', 'outlook', 100, $2, 'analizzato', now()) RETURNING allegato_id`, s.messaggio, shaPdf).Scan(&a); err != nil {
			t.Fatal(err)
		}
		inTransazione(t, p,
			passoSQL{`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, $3)`,
				[]any{shaPdf, strings.Repeat("c", 64), `{"testo_pdf": {"pagine": [{"testo": "9120100A1"}]}}`}},
			passoSQL{`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte)
				VALUES ($1, $2, 'disegno_2d', '9120100', '1', 50, 'nome_file')`, []any{a, s.thread}})
		var radiceDopo string
		for _, g := range pagina(t).Gruppi {
			for _, n := range g.Nodi {
				if g.Chiave == propostabom.GruppoBOM && n.Rif == col.FonteRadice && n.Completezza != nil {
					radiceDopo = n.Completezza.Esito
				}
			}
		}
		t.Logf("A-27 L4: la completezza della radice prima del caricamento %q, dopo %q", radicePrima, radiceDopo)
		if radicePrima == "" || radiceDopo == "" || radicePrima == radiceDopo {
			t.Errorf("il caricamento del 2D del prodotto non cambia la completezza della radice in /dati: prima %q, dopo %q", radicePrima, radiceDopo)
		}
		var imp string
		if err := p.QueryRow(ctx, `SELECT i.impronta FROM distinta_bozza_versione v JOIN distinta_importazione i ON i.importazione_id = v.importazione_id
			WHERE v.bozza_id = $1 AND v.versione = 1`, b2).Scan(&imp); err != nil || imp != col.Impronta {
			t.Errorf("il registro della bozza dopo il caricamento: %q %v", imp, err)
		}
	})
	t.Run("lo STEP in Altri file", func(t *testing.T) {
		inTransazione(t, p,
			passoSQL{`INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, bytes, percorso_relativo, radice_id)
				SELECT $1, a.allegato_id, a.sha256, 10, 'ALTRO\' || a.allegato_id || '.stp', gen_random_uuid() FROM allegato a WHERE a.allegato_id = $2`, []any{s.thread, s.aStep}},
			passoSQL{`UPDATE file_rfq_conservazione SET stato = 'copiata', sorgente_byte = 'staging' WHERE thread_id = $1 AND allegato_id = $2 AND stato = 'in_copia'`,
				[]any{s.thread, s.aStep}},
			passoSQL{`INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
				estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
			  SELECT m.thread_id, a.allegato_id, 'altri_file', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno,
				a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, $2, $2
			    FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{s.aStep, s.utente}},
			passoSQL{`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, dipendenze, origine, autore_id)
				VALUES ($1, $2, 'altri_file', 'attivo', 'altri_file', '{}', '{}', 'ponte', $3)`, []any{s.thread, s.aStep, s.utente}})
		j := pagina(t)
		gc := j.BozzaDelProdotto
		if gc == nil || gc.GruppoCorrente.Come != propostabom.ComeAssente || gc.GruppoCorrente.Motivo != propostabom.MotivoOccorrenzaNonAttiva ||
			gc.GruppoCorrente.AllegatoID == nil || *gc.GruppoCorrente.AllegatoID != s.aStep || gc.GruppoCorrente.Chiave != "" {
			t.Errorf("la bozza del prodotto con lo STEP in Altri file: %+v", gc)
		}
		for _, g := range j.Gruppi {
			if g.Chiave == propostabom.GruppoBOM {
				t.Errorf("il gruppo bom sullo STEP in Altri file sta fra i gruppi: %+v", g)
			}
		}
		if j.MotivoSenzaGruppi == nil || j.MotivoSenzaGruppi.Codice != propostabom.MotivoSenzaGruppiStepNonAttivo || len(j.MotivoSenzaGruppi.Allegati) != 1 ||
			j.MotivoSenzaGruppi.Allegati[0].AllegatoID != s.aStep.String() || j.MotivoSenzaGruppi.Allegati[0].Stato != "altri_file" ||
			j.MotivoSenzaGruppi.Testo != propostabom.TestoStepInAltriFile {
			t.Errorf("il motivo senza gruppi con lo STEP in Altri file: %+v", j.MotivoSenzaGruppi)
		}
	})
	// per ultimo: un componente archiviato non arriva al nodo (la valutazione lo lascia fuori dal contesto), quindi la
	// certificazione superata si guarda nel Go sulle letture dello snapshot, accanto alla colonna valida
	t.Run("il componente archiviato", func(t *testing.T) {
		inTransazione(t, p, passoSQL{`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'prova L4 di V1'
			WHERE componente_id = $1`, []any{s.figlio, s.utente}})
		ris, err := svc.Calcola(ctx, s.thread)
		if err != nil {
			t.Fatal(err)
		}
		cert, ok := ris.V1.Certificazioni[s.figlio]
		var w propostabom.ComponenteDellaWorking
		for _, c := range ris.Thread.Componenti {
			if c.ID == s.figlio {
				w = propostabom.ComponenteDellaWorking{Codice: c.Codice, Archiviato: c.ArchiviatoIl != nil}
			}
		}
		_, superata := propostabom.IdentitaDelRegistrato(s.cliente, w, &cert)
		if !ok || cert.Emittente != "partner:beta" || cert.EmittentePresunto || cert.Namespace != "beta-dis" || !w.Archiviato || !superata || valida(t) {
			t.Errorf("il figlio archiviato: certificazione %+v (%v), working %+v, superata %v, valida %v", cert, ok, w, superata, valida(t))
		}
	})
}
