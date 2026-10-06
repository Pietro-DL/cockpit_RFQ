//go:build integrazione

// L4 — la scena sintetica delle prove del caricatore (A1c-L4S-02…07, -10): una RFQ inventata con tutti i record che
// il caricatore legge (i tre gesti, i documenti con le provenienze, le righe legacy con la marcatura e il segno
// dell'albero, le viste, le deroghe, le rimozioni, i fabbisogni del cliente, due versioni della BOM, i suggerimenti
// dell'agente da escludere), una seconda RFQ minima e un messaggio senza RFQ. È scritta con SQL sul database di
// prova, che le prove azzerano.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package caricatore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/fotorfq"
)

// La terna corrente e un'altra terna, per la prova della terna esatta.
const (
	versioneCorrente = 4
	versioneAltra    = 3
)

var (
	hashCorrente = strings.Repeat("c", 64)
	hashAltro    = strings.Repeat("d", 64)
)

// Gli sha256 dei contenuti della scena.
var (
	shaStep      = strings.Repeat("1", 64) // lo STEP del finito: allegato e documento
	shaPDF       = strings.Repeat("2", 64) // il 2D dello sciolto: allegato, documento e allegato del messaggio senza RFQ
	shaSenzaFatt = strings.Repeat("3", 64) // un PDF ancora da analizzare (lavoro pendente)
	shaSoloDoc   = strings.Repeat("5", 64) // un documento senza allegato nel thread (T-04)
)

// varianti: le strutture STEP per la prova della completezza (A1c-L4S-06), con lo sha del loro allegato.
var varianti = []struct {
	nome, sha, struttura string
}{
	{"completa", strings.Repeat("a", 64), strutturaSTEP(3, `["#1"]`, true, false)},
	{"solo parti", strings.Repeat("b", 64), strutturaSTEP(3, `["#1"]`, false, false)},
	{"due radici", strings.Repeat("e", 64), strutturaSTEP(3, `["#1", "#2"]`, true, false)},
	{"troncata", strings.Repeat("f", 64), strutturaSTEP(3, `["#1"]`, true, true)},
	{"v2", strings.Repeat("9", 64), strutturaSTEP(2, `["#1"]`, true, false)},
}

// strutturaSTEP: la struttura dei fatti di uno STEP (la forma di fattiStep delle prove di transport/web), con due
// nodi; con archi, l'arco #1 → #2 per 2.
func strutturaSTEP(versione int, radici string, archi, troncata bool) string {
	rel := `[]`
	if archi {
		rel = `[{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}]`
	}
	limiti := `{"troncato": false}`
	if troncata {
		limiti = `{"troncato": true, "motivo": "tempo inventato"}`
	}
	return fmt.Sprintf(`{"versione": %d, "schema": "AP214", "radici": %s, "avvisi": [], "nodi": [`+
		`{"chiave": "#1", "id_grezzo": "ACME-030P7120001", "nome_grezzo": "7120001", "evidenza": {}}, `+
		`{"chiave": "#2", "id_grezzo": "7120002", "nome_grezzo": "7120002", "evidenza": {}}], "relazioni": %s, "limiti": %s, `+
		`"scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}`,
		versione, radici, rel, limiti)
}

// segretoPassword e marcatoreRegole: valori inventati che la fotografia non deve mai portare (password_hash,
// cliente.regole).
const (
	segretoPassword = "hash-inventato-della-password-acme"
	marcatoreRegole = "regole-inventate-acme-da-non-leggere"
)

type scena struct {
	operatore, secondo, cliente  uuid.UUID
	thread, thread2              uuid.UUID
	m1, m2, m4, fuori            uuid.UUID
	aStep, aPDF, aSenzaFatti     uuid.UUID
	aFuori                       uuid.UUID
	finito, sciolto              uuid.UUID
	docStep, docPDF, docSoloDoc  uuid.UUID
	rigaRadice, rigaFiglio       uuid.UUID
	proposta2D, propostaAssegna  uuid.UUID
	deroga, v1, v2               uuid.UUID
	allegatiVarianti             []uuid.UUID
	evidenzaRadice, evidenzaArco string
}

// costruisciScena scrive la scena sul database di prova (lo schema è già pulito). L'ordine conta: la working
// (componenti, archi, documenti dei componenti, deroghe) si scrive prima di congelare la V1, che la blocca (D26).
func costruisciScena(t *testing.T, p *pgxpool.Pool) scena {
	t.Helper()
	ctx := context.Background()
	riga := func(dst any, sql string, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	var s scena
	riga(&s.operatore, `INSERT INTO utente (sigla, nome, ufficio, password_hash) VALUES ('AC', 'Prova ACME', 'Tecnico', $1) RETURNING utente_id`, segretoPassword)
	riga(&s.secondo, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('AB', 'Seconda prova ACME', 'Tecnico') RETURNING utente_id`)
	riga(&s.cliente, `INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ('ACME', 'ACME S.p.A.', $1) RETURNING cliente_id`,
		`{"`+marcatoreRegole+`": true}`)
	riga(&s.thread, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, creato_da, riferimento_cliente)
		VALUES ($1, 'outlook', now() - interval '3 hours', 'ACME\WIP\rfq1', 'RFQ ACME26-030', $2, 'ACME26-030') RETURNING thread_id`, s.cliente, s.operatore)
	riga(&s.thread2, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa)
		VALUES ($1, 'outlook', now() - interval '2 hours', 'ACME\WIP\rfq2') RETURNING thread_id`, s.cliente)
	esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now() - interval '1 hour')`, s.thread)

	var conv, convFuori uuid.UUID
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-1', now()) RETURNING conversazione_id`)
	riga(&convFuori, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-2', now()) RETURNING conversazione_id`)
	riga(&s.m1, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo) VALUES ('outlook', '<m1@acme.example>', $1, $2, 'operatore', $3, now() - interval '150 minutes',
		'entrata', now() - interval '160 minutes', 'RFQ ACME26-030', 'Richiesta per il codice 7120001') RETURNING messaggio_id`, conv, s.thread, s.operatore)
	riga(&s.m2, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, direzione, data_evento)
		VALUES ('outlook', '<m2@acme.example>', $1, $2, 'auto_conversazione', 'entrata', now() - interval '100 minutes') RETURNING messaggio_id`, conv, s.thread)
	riga(&s.m4, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione, data_evento)
		VALUES ('outlook', '<m4@acme.example>', $1, $2, 'operatore', $3, now(), 'entrata', now() - interval '90 minutes') RETURNING messaggio_id`,
		conv, s.thread2, s.secondo)
	riga(&s.fuori, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento)
		VALUES ('outlook', '<fuori@acme.example>', $1, 'entrata', now() - interval '200 minutes') RETURNING messaggio_id`, convFuori)

	allegato := func(dst *uuid.UUID, msg uuid.UUID, indice int, nome, est, sha string) {
		t.Helper()
		riga(dst, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, 'analizzato', now()) RETURNING allegato_id`, msg, indice, nome, est, sha)
	}
	allegato(&s.aStep, s.m1, 1, "ACME-030P7120001.stp", "stp", shaStep)
	allegato(&s.aPDF, s.m1, 2, "7120002.pdf", "pdf", shaPDF)
	allegato(&s.aSenzaFatti, s.m2, 1, "7120003.pdf", "pdf", shaSenzaFatt)
	allegato(&s.aFuori, s.fuori, 1, "9999999A_2.pdf", "pdf", shaPDF)
	for i, v := range varianti {
		var a uuid.UUID
		allegato(&a, s.m2, 10+i, fmt.Sprintf("struttura_acme_%d.stp", i), "stp", v.sha)
		s.allegatiVarianti = append(s.allegatiVarianti, a)
	}

	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, versioneCorrente, hashCorrente)
	fatti := func(sha string, versione int, hash, payload string) {
		t.Helper()
		esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`, sha, versione, hash, payload)
	}
	fatti(shaStep, versioneCorrente, hashCorrente, `{"struttura": `+strutturaSTEP(3, `["#1"]`, true, false)+`}`)
	fatti(shaStep, versioneAltra, hashAltro, `{"struttura": `+strutturaSTEP(3, `["#1"]`, false, false)+`, "terna": "vecchia"}`)
	fatti(shaPDF, versioneCorrente, hashCorrente, `{"testo_pdf": {"pagine": [{"testo": "7120002 REV 1"}]}}`)
	fatti(shaSoloDoc, versioneCorrente, hashCorrente, `{"testo_pdf": {"pagine": [{"testo": "capitolato ACME"}]}}`)
	for _, v := range varianti {
		fatti(v.sha, versioneCorrente, hashCorrente, `{"struttura": `+v.struttura+`}`)
	}

	riga(&s.finito, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120001', 'finito', 'codice_rilevato', $2)
		RETURNING componente_id`, s.thread, s.operatore)
	riga(&s.sciolto, `INSERT INTO componente (thread_id, codice, rev, descrizione, tipo, origine, confermato_da)
		VALUES ($1, '7120002', '1', 'staffa ACME', 'sciolto', 'step', $2) RETURNING componente_id`, s.thread, s.operatore)
	riga(&s.docStep, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', '7120001', 'ACME-030P7120001.stp', 'stp', $3, 'CAD\7120001.stp', $4) RETURNING documento_id`,
		s.thread, s.finito, shaStep, s.operatore)
	riga(&s.docPDF, `INSERT INTO documento (thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, stato_nas, confermato_da)
		VALUES ($1, $2, 'disegno_2d', '7120002', '1', '7120002.pdf', 'pdf', $3, 'DISEGNI\7120002.pdf', 'scritto', $4) RETURNING documento_id`,
		s.thread, s.sciolto, shaPDF, s.operatore)
	riga(&s.docSoloDoc, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, 'capitolato', 'capitolato_acme.pdf', 'pdf', $2, 'CAPITOLATO\capitolato_acme.pdf', $3) RETURNING documento_id`,
		s.thread, shaSoloDoc, s.operatore)
	esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now()), ($3, $4, now())`,
		s.docStep, s.aStep, s.docPDF, s.aPDF)
	esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, s.finito, s.docStep)
	esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, posizione, origine, confermato_da)
		VALUES ($1, $2, $3, 2, '10', 'step', $4)`, s.thread, s.finito, s.sciolto, s.operatore)
	esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'sviluppo_dxf', 'lo facciamo noi', $3)`,
		s.thread, s.sciolto, s.operatore)
	riga(&s.deroga, `SELECT deroga_id FROM deroga_fabbisogno WHERE componente_id = $1`, s.sciolto)

	s.evidenzaRadice = fmt.Sprintf(`{"classificato": {"famiglia": "acme"}, "storia": [{"evento": "inventato"}], "strutturale": {"v": 1, "ruolo": "radice",
		"componente_id": %q, "documento_id": %q, "dichiarato_da": %q, "dichiarato_il": "2026-10-06T08:00:00Z"}}`, s.finito, s.docStep, s.operatore)
	evidenzaFiglio := fmt.Sprintf(`{"albero": {"da": %q, "il": "2026-10-06T08:05:00Z", "firma": "f-acme", "nodo": "cod:7120002",
		"commerciale": {"risposta": "no", "motivo": "inventato"}}}`, s.operatore)
	s.evidenzaArco = fmt.Sprintf(`{"albero": {"da": %q, "il": "2026-10-06T08:05:00Z", "firma": "f-acme", "padre": %q, "figlio": %q}}`,
		s.operatore, s.finito, s.sciolto)
	riga(&s.rigaRadice, `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte,
		confidenza, evidenza, stato, componente_id, deciso_da, deciso_il) VALUES ($1, $2, $3, '#1', '7120001', 'ACME-030P7120001', '7120001', 'acme',
		'step', 60, $4, 'confermata', $5, $6, now()) RETURNING proposta_id`, s.thread, s.aStep, shaStep, s.evidenzaRadice, s.finito, s.operatore)
	riga(&s.rigaFiglio, `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, origine_codice,
		famiglia, tipo_proposto, fonte, confidenza, evidenza) VALUES ($1, $2, $3, '#2', '7120002', '7120002', '7120002', 'operatore', 'acme',
		'sciolto', 'step', 60, $4) RETURNING proposta_id`, s.thread, s.aStep, shaStep, evidenzaFiglio)
	esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta, evidenza, stato, deciso_da, deciso_il)
		VALUES ($1, $2, '#1', '#2', 2, $3, 'confermata', $4, now())`, s.thread, s.aStep, s.evidenzaArco, s.operatore)
	esegui(`INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 2)`,
		s.thread, s.docStep, s.finito, s.sciolto)
	riga(&s.proposta2D, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, componente_id, confidenza, fonte, stato,
		deciso_da, deciso_il, dettagli) VALUES ($1, $2, 'disegno_2d', '7120002', '1', $3, 90, 'nome_file', 'confermata', $4, now(),
		'{"destinazione": {"esito": "figlio"}}') RETURNING proposta_id`, s.aPDF, s.thread, s.sciolto, s.operatore)
	riga(&s.propostaAssegna, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', '7120002', $3, 50, 'estensione') RETURNING proposta_id`, s.aSenzaFatti, s.thread, s.sciolto)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '7120001', 'manuale', $2)`, s.thread, s.operatore)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza) VALUES ($1, '7120009', 'proposta_corpo', 40)`, s.thread)
	esegui(`INSERT INTO proposta_triage (messaggio_id, esito, identificativi, confidenza, fonte, stato, deciso_il)
		VALUES ($1, 'nuova_rfq', '{7120001}', 80, 'deterministico', 'accettata', now())`, s.m1)
	esegui(`INSERT INTO proposta_triage (messaggio_id, esito, confidenza, fonte) VALUES ($1, 'aggancia', 70, 'agente')`, s.m1)
	esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio) VALUES ($1, '7120001', 'prodotto', 'famiglia', 80)`, s.m1)
	esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio) VALUES ($1, '7129999', 'prodotto', 'agente', 90)`, s.m1)
	esegui(`INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza) VALUES ('stage_allegato', 'outlook', $1, 'stage:acme')`,
		`{"allegato_id": "`+s.aSenzaFatti.String()+`"}`)
	esegui(`INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza) VALUES ('analizza_allegato', 'analisi', $1, 'analisi:acme')`,
		`{"sha256": "`+shaSenzaFatt+`"}`)
	esegui(`INSERT INTO fabbisogno_documento (cliente_id, tipo_componente, tipo, bloccante, fonte_attesa) VALUES ($1, 'sciolto', 'sviluppo_dxf', true, 'cliente')`,
		s.cliente)

	// la V1 congelata, poi la fase della scheda costo e la V2 in bozza: la working torna modificabile
	var fase uuid.UUID
	riga(&fase, `SELECT fase_log_id FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, s.thread)
	riga(&s.v1, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prima versione ACME', $3) RETURNING bom_versione_id`, s.thread, fase, s.operatore)
	esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, s.v1, s.operatore)
	esegui(`UPDATE fase_log SET fine = now(), esito = 'OK' WHERE thread_id = $1 AND fine IS NULL`, s.thread)
	riga(&fase, `INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'SCHEDA_COSTO', now()) RETURNING fase_log_id`, s.thread)
	riga(&s.v2, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, versione_precedente_id,
		numero_precedente, stato_precedente, motivo, creata_da) VALUES ($1, 2, 'preventivo', 'SCHEDA_COSTO', $2, $3, 1, 'congelata',
		'revisione ACME', $4) RETURNING bom_versione_id`, s.thread, fase, s.v1, s.operatore)
	return s
}

// threadDi: il thread della fotografia con quell'ID; la prova si ferma se non c'è.
func threadDi(t *testing.T, f fotorfq.Fotografia, id uuid.UUID) fotorfq.Thread {
	t.Helper()
	for _, th := range f.Thread {
		if th.ID == id {
			return th
		}
	}
	t.Fatalf("la RFQ %s non è nella fotografia", id)
	return fotorfq.Thread{}
}

// inJSON: un valore in JSON, per cercare nella fotografia ciò che non deve esserci.
func inJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
