//go:build integrazione

// L4 — i target e la fonte dalla fotografia letta sul database di prova (contratto di A1c, §7, famiglia B1; PO-19 sul
// DB; R65 A, R70 A, R76 b A; T-B0-07, T-B0-08): una scena ACME scritta con SQL sul database di prova, letta con
// caricatore.Carica in sola lettura, poi ValutaProdotti. Il codice accettato alla creazione della RFQ è il target, gli
// altri candidati della mail no; la fonte del finito viene dalla vista v_step_prodotto e dal gesto 3 con la
// marcatura; la fonte dell'identificativo senza componente viene dallo STEP del thread con la sua radice.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package valutazione_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// strutturaDB: i fatti STEP della scena sul DB (struttura v3 completa per la 0020: una radice, un arco, scarti zero).
func strutturaDB(radice, figlio string) string {
	return fmt.Sprintf(`{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [], "nodi": [`+
		`{"chiave": "#1", "id_grezzo": %q, "nome_grezzo": "", "evidenza": {}}, {"chiave": "#2", "id_grezzo": %q, "nome_grezzo": "", "evidenza": {}}], `+
		`"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}], "limiti": {"troncato": false}, `+
		`"scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`, radice, figlio)
}

type scenaDB struct {
	operatore, cliente, thread, m1 uuid.UUID
	aFinito, aAssieme, finito      uuid.UUID
	docFinito                      uuid.UUID
}

// costruisciScenaDB: la RFQ ACME. Un messaggio con il gesto 1 che chiede P7120100 e P7120200; alla creazione l'operatore
// conferma P7120100 (identificativo con confermato_da), P7120200 resta proposto. Un finito manuale 7120300 con il suo
// STEP autorizzato (documento, marcatura della radice, step_strutturale_id) e un secondo STEP fra gli allegati con la
// radice 7120100, che nessuno ha ancora associato.
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
		VALUES ($1, 'outlook', now() - interval '3 hours', 'ACME\WIP\rfq-b1', 'RFQ ACME26-030', $2) RETURNING thread_id`, s.cliente, s.operatore)
	var conv uuid.UUID
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-B1', now()) RETURNING conversazione_id`)
	riga(&s.m1, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<b1@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '150 minutes', 'entrata', now() - interval '160 minutes', 'RFQ ACME26-030',
		E'Buongiorno,\r\nvi chiediamo l''offerta per P7120100 e P7120200.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`,
		conv, s.thread, s.operatore, s.cliente)

	shaFinito, shaAssieme := sha("1"), sha("2")
	allegato := func(dst *uuid.UUID, indice int, nome, sha string) {
		t.Helper()
		riga(dst, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, 'stp', 'file', 'outlook', 100, $4, 'analizzato', now()) RETURNING allegato_id`, s.m1, indice, nome, sha)
	}
	allegato(&s.aFinito, 1, "7120300.stp", shaFinito)
	allegato(&s.aAssieme, 2, "assieme_acme.stp", shaAssieme)
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, versioneAnalizzatore, hashConfigurazione)
	esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4), ($5, $2, $3, $6)`,
		shaFinito, versioneAnalizzatore, hashConfigurazione, strutturaDB("7120300", "7120301"), shaAssieme, strutturaDB("7120100", "7120101"))

	riga(&s.finito, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120300', 'finito', 'manuale', $2)
		RETURNING componente_id`, s.thread, s.operatore)
	esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '', '7120300', '7120300', 'acme', 'step', 60), ($1, $2, $3, '#2', '', '7120301', '7120301', 'acme', 'step', 60)`,
		s.thread, s.aFinito, shaFinito)
	esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1, $2, '#1', '#2', 2)`, s.thread, s.aFinito)
	s.docFinito = testutil.AutorizzaStep(t, p, s.thread, s.finito, s.aFinito, s.operatore)
	esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING`, s.docFinito, s.aFinito)

	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, 'P7120100', 'proposta_famiglia', $2)`, s.thread, s.operatore)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza) VALUES ($1, 'P7120200', 'proposta_corpo', 40)`, s.thread)
	esegui(`INSERT INTO proposta_triage (messaggio_id, esito, identificativi, confidenza, fonte, stato, deciso_il)
		VALUES ($1, 'nuova_rfq', '{P7120100,P7120200}', 80, 'deterministico', 'accettata', now())`, s.m1)
	return s
}

// TestL4TargetEFonteDalDatabase (PO-19 sul DB; R65 A; R76 b A; T-B0-08; il gate di B1, condizione 1; MOTORE-SENZA-LLM):
// dalla fotografia del caricatore, il target è il solo codice confermato e il finito manuale; i tre esiti della fonte
// vengono dalla vista, dal gesto 3 e dal motore A. Due letture della stessa scena danno la stessa impronta della
// fotografia e gli stessi byte canonici del risultato; un suggerimento dell'agente discordante, scritto dopo, non
// cambia né l'una né gli altri; il registro SQL non ha testi vietati; ValutaProdotti non parla con il database, e il
// database non cambia.
func TestL4TargetEFonteDalDatabase(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaDB(t, p)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	m := motoreACME(t)

	// giro: una Carica sul pool con il registro e una ValutaProdotti, con i controlli del registro.
	giro := func(nome string) (fotorfq.Fotografia, valutazione.ValutazioneProdotti, string, []byte) {
		t.Helper()
		reg.Azzera()
		f, err := caricatore.Carica(context.Background(), pr, caricatore.Richiesta{Thread: []uuid.UUID{s.thread}})
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
		if vietati := reg.Vietati(); len(vietati) > 0 {
			t.Errorf("%s: testi vietati nel registro: %q", nome, vietati)
		}
		if len(f.Thread) != 1 {
			t.Fatalf("%s: thread nella fotografia: %d", nome, len(f.Thread))
		}
		h, err := fotorfq.ImprontaFotografia(f)
		if err != nil {
			t.Fatal(err)
		}
		reg.Azzera()
		v, err := valutazione.ValutaProdotti(f, f.Thread[0], m, nil)
		if err != nil {
			t.Fatalf("%s: ValutaProdotti: %v", nome, err)
		}
		if len(reg.Voci()) != 0 {
			t.Errorf("%s: ValutaProdotti ha parlato con il database: %v", nome, reg.Voci())
		}
		b, err := jsoncanonico.Codifica(v)
		if err != nil {
			t.Fatal(err)
		}
		return f, v, h, b
	}

	prima := testutil.FotoDelDatabase(t, p)
	_, v, h1, b1 := giro("primo")
	_, _, h2, b2 := giro("secondo")
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("la lettura ha cambiato il database: %v", d)
	}
	if h1 != h2 || !bytes.Equal(b1, b2) {
		t.Errorf("stessa scena: impronte %s e %s, risultati uguali %v", h1, h2, bytes.Equal(b1, b2))
	}
	// MOTORE-SENZA-LLM: un suggerimento dell'agente discordante non entra né nella fotografia né nel risultato.
	testutil.AnalisiMessaggioDiProva(t, p, s.m1, json.RawMessage(`{"codici": ["7129999"], "esito": "nuova_rfq"}`))
	prima = testutil.FotoDelDatabase(t, p)
	_, _, h3, b3 := giro("con il suggerimento")
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("con il suggerimento: il database è cambiato: %v", d)
	}
	if h3 != h1 || !bytes.Equal(b3, b1) {
		t.Errorf("il suggerimento dell'agente ha cambiato la fotografia (%v) o il risultato (%v)", h3 != h1, !bytes.Equal(b3, b1))
	}

	rifFinito, rifIdent := "componente:"+s.finito.String(), "identificativo:P7120100"
	if got := rifDi(v); len(got) != 2 || got[0] != rifFinito || got[1] != rifIdent { // in ordine di codice: 7120300, P7120100
		t.Fatalf("target %v: attesi il codice confermato e il finito manuale (PO-19, R70 A)", got)
	}
	if v.Richiesta.Stato != ancoraggio.StatoRichiestaValutata || v.Richiesta.Decisioni == "" || v.Richiesta.Evento != "nuova_rfq" {
		t.Errorf("richiesta %+v", v.Richiesta)
	}
	var candidati []string
	for _, c := range v.Prodotti.Candidati {
		candidati = append(candidati, c.CodiceRichiesto+"/"+c.Motivo)
	}
	if fmt.Sprint(candidati) != "[P7120100/da_confermare P7120200/da_confermare]" {
		t.Errorf("candidati %v: i due codici della mail restano da confermare (R60 A)", candidati)
	}

	ident := prodotto(t, v, rifIdent)
	if !valutazione.TargetConfermato(ident) || ident.Base.Normalizzata != "7120100" || statoMotivo(ident.Fonte) != "in_attesa_di_conferma/documento_candidato" ||
		len(ident.Fonte.Candidati) != 1 || ident.Fonte.Candidati[0].AllegatoID == nil || *ident.Fonte.Candidati[0].AllegatoID != s.aAssieme ||
		ident.Fonte.Candidati[0].RadiceCompatibile != "#1" || !ident.Fonte.Candidati[0].GrafoCompleto {
		t.Errorf("identificativo: %+v", ident)
	}
	fin := prodotto(t, v, rifFinito)
	r := fin.Fonte.Riferimento
	if statoMotivo(fin.Fonte) != "confermata/estrazione_riuscita" || fin.Fonte.EsitoVista != "presente_analizzato" || r == nil ||
		r.DocumentoID != s.docFinito || r.Forma != ancoraggio.FormaRiferimentoSmistamento || r.Radice != "#1" || r.Ruolo != "radice" ||
		r.ConfermatoDa == nil || *r.ConfermatoDa != s.operatore || r.ConfermatoIl == "" || r.Superato || fin.Fonte.Estrazione != ancoraggio.EstrazioneRiuscita {
		t.Errorf("finito: fonte %+v, riferimento %+v", fin.Fonte, r)
	}
	// Nessuna diagnostica dei target e della fonte. C'è solo la contraddizione fra il contesto e la decisione (R106 B,
	// precisata dall'utente il 07/10): lo STEP autorizzato del finito 7120300 è allegato al messaggio che nomina solo il
	// codice confermato P7120100, e la contraddizione resta visibile, senza bloccare niente.
	attesa := []string{s.aFinito.String(), "contesto:" + rifIdent, "decisione:" + rifFinito}
	if len(v.Diagnostiche) != 1 || v.Diagnostiche[0].Codice != valutazione.CodiceContestoDiscorde || fmt.Sprint(v.Diagnostiche[0].Rif) != fmt.Sprint(attesa) {
		t.Errorf("diagnostiche %+v: attesa solo %s con %v", v.Diagnostiche, valutazione.CodiceContestoDiscorde, attesa)
	}
}
