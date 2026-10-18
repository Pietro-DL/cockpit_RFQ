//go:build integrazione

package runtime

// L4 — V1, ondata B, corsia file, secondo passaggio (rilievi della revisione): il deposito degli «Altri file» si prepara
// all'avvio solo con [motore_a] acceso. A flag spento la radice scritta in [conservazione] si ignora: niente cartella,
// niente segnaposto, e una radice che non andrebbe (senza il segnaposto delle copie registrate) non ferma l'avvio.
// Cartelle temporanee, DB di prova, nomi sintetici.

import (
	"os"
	"path/filepath"
	"testing"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/testutil"
)

func TestDepositoSoloConIlFlagAcceso(t *testing.T) {
	p, _, ctx := preparaDB(t)
	radice := filepath.Join(t.TempDir(), "conservazione")
	cfg := &config.Config{}
	cfg.Conservazione.Radice = radice
	log := testutil.LogSilenzioso()

	// a flag spento: nessun deposito, niente sul disco
	svc, err := preparaCicloFile(ctx, cfg, p, t.TempDir(), log)
	if err != nil {
		t.Fatalf("a flag spento l'avvio si ferma: %v", err)
	}
	if svc.Deposito != nil || svc.ConservazioneConfigurata() {
		t.Errorf("a flag spento il deposito e' preparato: %+v", svc.Deposito)
	}
	if _, err := os.Stat(radice); !os.IsNotExist(err) {
		t.Errorf("a flag spento l'avvio ha creato la cartella del deposito: %v", err)
	}

	// a flag acceso: la cartella, _parti e il segnaposto
	cfg.MotoreA.Anteprima = true
	svc, err = preparaCicloFile(ctx, cfg, p, t.TempDir(), log)
	if err != nil {
		t.Fatalf("a flag acceso: %v", err)
	}
	if svc.Deposito == nil {
		t.Fatal("a flag acceso il deposito non c'e'")
	}
	for _, f := range []string{filerfq.NomeSegnaposto, filerfq.CartellaPartiDeposito} {
		if _, err := os.Stat(filepath.Join(radice, f)); err != nil {
			t.Errorf("a flag acceso manca %s: %v", f, err)
		}
	}

	// una copia verificata registrata in un altro deposito, e una radice nuova senza il suo segnaposto: a flag acceso
	// l'avvio si ferma (EM §10.5), a flag spento no, e non scrive niente
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	riga := func(sql string, arg ...any) string {
		t.Helper()
		var v string
		if err := p.QueryRow(ctx, sql, arg...).Scan(&v); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return v
	}
	cliente := riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme Prova') RETURNING cliente_id::text`)
	thread := riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
		VALUES ($1, 'outlook', now(), 'RFQ prova', 'ACME\WIP\prova') RETURNING thread_id::text`, cliente)
	conv := riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'CONV-DEP', now())
		RETURNING conversazione_id::text`)
	msg := riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook', '<dep@prova>', $1, 'entrata', now(), 'RFQ', $2) RETURNING messaggio_id::text`, conv, thread)
	allegato := riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, sha256, stato, ricevuto_il)
		VALUES ($1, 1, 'x.pdf', 'pdf', 'file', 'outlook', repeat('a', 64), 'analizzato', now()) RETURNING allegato_id::text`, msg)
	// la riga si scrive a mano, gia' copiata e di un altro deposito: i trigger della riga, che lo vieterebbero, spenti
	// per l'istante dell'INSERT (come PF-20)
	esegui(`ALTER TABLE file_rfq_conservazione DISABLE TRIGGER USER`)
	esegui(`INSERT INTO file_rfq_conservazione (conservazione_id, thread_id, allegato_id, sha256, percorso_relativo, deposito_id, stato,
		verificato_il, sorgente_byte) OVERRIDING SYSTEM VALUE
		VALUES (1, $1::uuid, $2::uuid, repeat('a', 64), $3, gen_random_uuid(), 'copiata', now(), 'staging')`,
		thread, allegato, thread+`\`+allegato+".1.pdf")
	esegui(`ALTER TABLE file_rfq_conservazione ENABLE TRIGGER USER`)
	cfg.Conservazione.Radice = filepath.Join(t.TempDir(), "vuota")
	if _, err := preparaCicloFile(ctx, cfg, p, t.TempDir(), log); err == nil {
		t.Error("a flag acceso una radice senza il segnaposto delle copie registrate non ferma l'avvio")
	}
	if err := os.RemoveAll(cfg.Conservazione.Radice); err != nil {
		t.Fatal(err)
	}
	cfg.MotoreA.Anteprima = false
	if svc, err := preparaCicloFile(ctx, cfg, p, t.TempDir(), log); err != nil || svc.Deposito != nil {
		t.Errorf("a flag spento la radice si ignora: %v %+v", err, svc)
	}
	if _, err := os.Stat(cfg.Conservazione.Radice); !os.IsNotExist(err) {
		t.Errorf("a flag spento l'avvio ha creato la cartella: %v", err)
	}
}
