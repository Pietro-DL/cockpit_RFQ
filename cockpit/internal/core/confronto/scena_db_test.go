//go:build integrazione

// L4 — la scena sintetica delle L4 di confronto (A1c-L4S-08 e -09; T-B6-01): una RFQ inventata sul database di prova,
// letta con caricatore.Carica in sola lettura, valutata con valutazione.Calcola e confrontata con Confronta, come fanno
// il banco e l'anteprima. Dentro: il gesto 1 sul messaggio con i codici della richiesta, un identificativo confermato
// (il target), uno STEP con la radice del target e un figlio, il 2D del figlio confermato su un componente (con la
// lettura del vecchio motore nella proposta, per la misura delle correzioni), un file con
// un codice ignoto e una proposta aperta, un file con due letture discordanti nel nome, una proposta scartata. Gli
// ingressi dell'agente (suggerimenti, candidati, triage) li aggiunge la prova, solo dove serve.
//
// Le prove stanno nel pacchetto di prova confronto_test, che importa anche valutazione, il caricatore e testutil: le
// frecce delle prove non contano nel grafo (G1 legge i soli file non di prova), e confronto, nel codice di prodotto,
// non importa valutazione (fase 0 di B6, I.1). La copia dai record piatti di valutazione ai DTO di confronto qui passa
// dal JSON, con i campi sconosciuti rifiutati: la copia campo per campo del banco (passaggio.go) e la prova dei gemelli
// (A1c-L1-31) sono di P9.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package confronto_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// La terna corrente dei fatti della scena.
const versioneAnalizzatore = 4

var hashConfigurazione = strings.Repeat("c", 64)

// Gli sha256 dei contenuti della scena.
var (
	shaAssieme  = strings.Repeat("1", 64) // lo STEP con la radice del target e un figlio
	shaFiglio   = strings.Repeat("2", 64) // il 2D del figlio, deciso
	shaIgnoto   = strings.Repeat("3", 64) // un PDF con un codice che nessuna struttura conosce
	shaAmbiguo  = strings.Repeat("4", 64) // un PDF con due codici discordanti nel nome
	shaScartato = strings.Repeat("5", 64) // un PDF la cui proposta è scartata
)

// strutturaACME: i fatti STEP della scena (struttura v3 completa: una radice, un arco, nessuno scarto).
func strutturaACME(radice, figlio string) string {
	return fmt.Sprintf(`{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [], "nodi": [`+
		`{"chiave": "#1", "id_grezzo": %q, "nome_grezzo": "", "evidenza": {}}, {"chiave": "#2", "id_grezzo": %q, "nome_grezzo": "", "evidenza": {}}], `+
		`"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}], "limiti": {"troncato": false}, `+
		`"scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`, radice, figlio)
}

// testoPDF: i fatti di un PDF con il testo dato.
func testoPDF(testo string) string {
	return fmt.Sprintf(`{"testo_pdf": {"pagine": [{"testo": %q}]}}`, testo)
}

type scenaDB struct {
	operatore, cliente, thread, m1                uuid.UUID
	aAssieme, aFiglio, aIgnoto, aAmbiguo, aScarto uuid.UUID
	figlio, docFiglio                             uuid.UUID
}

// costruisciScenaDB scrive la scena sul database di prova (lo schema è già pulito).
func costruisciScenaDB(t *testing.T, p *pgxpool.Pool) scenaDB {
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
	var s scenaDB
	riga(&s.operatore, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('AC', 'Prova ACME', 'Tecnico') RETURNING utente_id`)
	riga(&s.cliente, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'ACME S.p.A.') RETURNING cliente_id`)
	riga(&s.thread, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, creato_da)
		VALUES ($1, 'outlook', now() - interval '3 hours', 'ACME\WIP\rfq-b6', 'RFQ ACME26-030', $2) RETURNING thread_id`, s.cliente, s.operatore)
	var conv uuid.UUID
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-B6', now()) RETURNING conversazione_id`)
	riga(&s.m1, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<b6@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '150 minutes', 'entrata', now() - interval '160 minutes', 'RFQ ACME26-030',
		E'Buongiorno,\r\nvi chiediamo l''offerta per P7120100 e P7120200.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`,
		conv, s.thread, s.operatore, s.cliente)

	allegato := func(dst *uuid.UUID, indice int, nome, est, sha string) {
		t.Helper()
		riga(dst, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, 'analizzato', now()) RETURNING allegato_id`, s.m1, indice, nome, est, sha)
	}
	allegato(&s.aAssieme, 1, "assieme_acme.stp", "stp", shaAssieme)
	allegato(&s.aFiglio, 2, "7120101.pdf", "pdf", shaFiglio)
	allegato(&s.aIgnoto, 3, "7129999.pdf", "pdf", shaIgnoto)
	allegato(&s.aAmbiguo, 4, "7120100_7120101.pdf", "pdf", shaAmbiguo)
	allegato(&s.aScarto, 5, "7120400.pdf", "pdf", shaScartato)
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, versioneAnalizzatore, hashConfigurazione)
	for _, f := range []struct{ sha, fatti string }{
		{shaAssieme, strutturaACME("7120100", "7120101")},
		{shaFiglio, testoPDF("7120101 REV 1")},
		{shaIgnoto, testoPDF("7129999")},
		{shaAmbiguo, testoPDF("7120100 7120101")},
		{shaScartato, testoPDF("7120400")},
	} {
		esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
			f.sha, versioneAnalizzatore, hashConfigurazione, f.fatti)
	}

	// Il figlio confermato, con il suo 2D deciso: la proposta confermata e il documento con la provenienza.
	riga(&s.figlio, `INSERT INTO componente (thread_id, codice, rev, tipo, origine, confermato_da) VALUES ($1, '7120101', '1', 'sciolto', 'step', $2)
		RETURNING componente_id`, s.thread, s.operatore)
	riga(&s.docFiglio, `INSERT INTO documento (thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, stato_nas,
		confermato_da) VALUES ($1, $2, 'disegno_2d', '7120101', '1', '7120101.pdf', 'pdf', $3, 'DISEGNI\7120101.pdf', 'scritto', $4)
		RETURNING documento_id`, s.thread, s.figlio, shaFiglio, s.operatore)
	esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())`, s.docFiglio, s.aFiglio)
	// Il nodo del figlio nello STEP, deciso sul componente: il motore A propone il componente, non il nodo.
	esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza,
		stato, componente_id, deciso_da, deciso_il) VALUES ($1, $2, $3, '#2', '', '7120101', '7120101', 'acme', 'step', 60, 'confermata', $4, $5, now())`,
		s.thread, s.aAssieme, shaAssieme, s.figlio, s.operatore)
	// La proposta del 2D porta la lettura del vecchio motore (dettagli.valutazione v2, la dimensione del codice con una
	// regola che non è «operatore»), come la scrive il legacy: senza, la misura delle correzioni escluderebbe il deciso
	// con senza_lettura_vecchia e non avrebbe niente da valutare (R114, precisata dall'utente il 07/10).
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, componente_id, confidenza, fonte, stato,
		deciso_da, deciso_il, dettagli) VALUES ($1, $2, 'disegno_2d', '7120101', '1', $3, 90, 'nome_file', 'confermata', $4, now(),
		'{"valutazione": {"v": 2, "codice": {"valore": "7120101", "regola": "nome_codice_famiglia"}}, "destinazione": {"esito": "figlio"}}')`,
		s.aFiglio, s.thread, s.figlio, s.operatore)
	// Il codice ignoto: una proposta aperta dal nome del file. La proposta scartata da una persona.
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', '7129999', 50, 'nome_file')`, s.aIgnoto, s.thread)
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, stato, deciso_da, deciso_il)
		VALUES ($1, $2, 'disegno_2d', '7120400', 50, 'nome_file', 'scartata', $3, now())`, s.aScarto, s.thread, s.operatore)

	// Il target: il codice confermato alla creazione; il secondo resta proposto. Il triage deterministico e un candidato
	// di codice non dell'agente.
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, 'P7120100', 'proposta_corpo', $2)`, s.thread, s.operatore)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza) VALUES ($1, 'P7120200', 'proposta_corpo', 40)`, s.thread)
	esegui(`INSERT INTO proposta_triage (messaggio_id, esito, identificativi, confidenza, fonte, stato, deciso_il)
		VALUES ($1, 'nuova_rfq', '{P7120100,P7120200}', 80, 'deterministico', 'accettata', now())`, s.m1)
	esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio) VALUES ($1, 'P7120100', 'prodotto', 'famiglia', 80)`, s.m1)
	return s
}

// conIngressiDellAgente aggiunge gli ingressi dell'agente, tutti discordanti: un suggerimento sul messaggio, un
// candidato di codice di origine agente, un triage di fonte agente. Il motore non deve vederne nessuno (MOTORE-SENZA-LLM).
func conIngressiDellAgente(t *testing.T, p *pgxpool.Pool, s scenaDB) {
	t.Helper()
	ctx := context.Background()
	testutil.AnalisiMessaggioDiProva(t, p, s.m1, json.RawMessage(`{"codici": ["7129999", "7120400"], "esito": "aggancia"}`))
	for _, sql := range []string{
		`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio) VALUES ($1, 'P7129999', 'prodotto', 'agente', 99)`,
		`INSERT INTO proposta_triage (messaggio_id, esito, identificativi, confidenza, fonte) VALUES ($1, 'aggancia', '{P7129999}', 95, 'agente')`,
	} {
		if _, err := p.Exec(ctx, sql, s.m1); err != nil {
			t.Fatalf("ingressi dell'agente: %v\n%s", err, sql)
		}
	}
}

// ---- la grammatica ACME, per l'insieme delle regole ----

func limitiACME() grammatica.Limiti {
	return grammatica.Limiti{
		Versione: "limiti-acme-1",
		Grammatica: grammatica.LimitiGrammatica{
			MaxFamiglie: 20, MaxFormePerFamiglia: 16, MaxPartiPerForma: 12, MaxEsempi: 64,
			MaxLunghezzaPattern: 64, MaxRipetizione: 40, MaxLunghezzaLetterale: 40,
		},
		Riconoscimento: grammatica.LimitiRiconoscimento{
			MaxByteUnita: 1048576, MaxUnitaDocumento: 10000, MaxLetturePerUnita: 1000, MaxLettureDocumento: 20000,
		},
		Anteprima: grammatica.LimitiAnteprima{TempoMassimoMs: 20000},
	}
}

// grammaticaACME: base 712 più quattro cifre; nella mail con la P di fase prototipo davanti, nei file e nello STEP
// senza. Ruoli {prodotto, componente}. Il cliente è quello che il database ha dato alla scena.
func grammaticaACME(cliente uuid.UUID) grammatica.Grammatica {
	selMail := []string{"oggetto", "corpo", "storia"}
	selCodice := []string{"nome_file", "cartiglio.codice", "radice_step.id", "radice_step.nome", "nodo_step.id", "nodo_step.nome"}
	base := grammatica.Base{
		Segmenti:  []grammatica.SegmentoBase{{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}},
		Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
	pBase := grammatica.Parte{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}
	esempio := func(id, sel, testo, forma string, affissi ...string) grammatica.EsempioCodice {
		return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: sel, Testo: testo,
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: forma, Base: "7120100", Affissi: affissi}}}}
	}
	return grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: cliente, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie: []grammatica.FamigliaCodice{{
			ID: "acme-prefisso", Namespace: "acme-prefisso",
			Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
			Base:  base,
			Forme: []grammatica.FormaCodice{
				{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true,
					Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteAffisso, Rif: "P", Min: 1, Max: 1}, pBase}},
				{ID: "codice", Selettori: selCodice, Stato: grammatica.StatoAttiva, Completa: true, Parti: []grammatica.Parte{pBase}},
			},
			Affissi: []grammatica.Affisso{{
				ID: "P", Letterali: []string{"P"}, Posizione: grammatica.PosizionePrefisso,
				Riconoscimento: selMail, Attribuzione: selMail,
				Valore: &grammatica.ValoreQualificatore{Fase: grammatica.FasePrototipo},
			}},
			Esempi: []grammatica.EsempioCodice{
				esempio("e-mail", "corpo", "P7120100", "mail", "P"),
				esempio("e-step", "radice_step.id", "7120100", "codice"),
			},
		}},
	}
}

// insiemeACME: l'insieme delle regole con la sola grammatica ACME, dalla porta del prodotto (l'indice con lo sha256
// del file, CompilaInsieme).
func insiemeACME(t *testing.T, cliente uuid.UUID) *motorea.InsiemeRegole {
	t.Helper()
	raw, err := json.Marshal(grammaticaACME(cliente))
	if err != nil {
		t.Fatal(err)
	}
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: limitiACME(),
		Grammatiche: []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: jsoncanonico.Impronta(raw)}}}
	r, d, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": raw})
	if err != nil || r.Motori[cliente] == nil {
		t.Fatalf("l'insieme delle regole ACME non compila: %v %+v", err, d)
	}
	return &r
}

// ---- il percorso del banco e dell'anteprima ----

// inConfronto copia un record piatto di valutazione nel DTO gemello di confronto passando dal JSON, con i campi
// sconosciuti rifiutati: un campo di valutazione che confronto non ha ferma la prova.
func inConfronto(t *testing.T, sorgente, destinazione any) {
	t.Helper()
	b, err := json.Marshal(sorgente)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destinazione); err != nil {
		t.Fatalf("copia nei DTO di confronto: %v", err)
	}
}

// percorso: il risultato di un giro della scena: la fotografia con la sua impronta, l'esito di valutazione e, per
// ogni thread in ordine, l'esito di confronto.
type percorso struct {
	foto       fotorfq.Fotografia
	impronta   string
	valutato   valutazione.Esito
	confronti  []confronto.Esito
	sqlVietato []string
}

// giro: Carica sul pool con il registro, poi Calcola e Confronta, che non parlano con il database. Controlla che il
// registro finisca con il ROLLBACK, su una connessione sola, senza testi vietati.
func giro(t *testing.T, nome string, pr *pgxpool.Pool, reg *testutil.RegistroSQL, thread uuid.UUID, r *motorea.InsiemeRegole) percorso {
	t.Helper()
	reg.Azzera()
	f, err := caricatore.Carica(context.Background(), pr, caricatore.Richiesta{Thread: []uuid.UUID{thread}})
	if err != nil {
		t.Fatalf("%s: Carica: %v", nome, err)
	}
	voci := reg.Voci()
	if len(voci) == 0 || strings.ToLower(strings.TrimSpace(voci[len(voci)-1].SQL)) != "rollback" {
		t.Errorf("%s: il registro non finisce con il ROLLBACK", nome)
	}
	for _, x := range voci {
		if x.Connessione != voci[0].Connessione {
			t.Errorf("%s: un testo su un'altra connessione: %q", nome, x.SQL)
		}
	}
	out := percorso{foto: f, sqlVietato: reg.Vietati()}
	if out.impronta, err = fotorfq.ImprontaFotografia(f); err != nil {
		t.Fatalf("%s: impronta della fotografia: %v", nome, err)
	}
	reg.Azzera()
	if out.valutato, err = valutazione.Calcola(f, r, valutazione.Ingressi{}); err != nil {
		t.Fatalf("%s: Calcola: %v", nome, err)
	}
	for _, th := range out.valutato.Thread {
		var file []confronto.File
		var prodotti []confronto.ProdottoNuovo
		inConfronto(t, th.Confrontabili, &file)
		inConfronto(t, th.ProdottiConfrontabili, &prodotti)
		out.confronti = append(out.confronti, confronto.Confronta(file, prodotti, nil))
	}
	if len(reg.Voci()) != 0 {
		t.Errorf("%s: Calcola o Confronta hanno parlato con il database: %v", nome, reg.Voci())
	}
	return out
}
