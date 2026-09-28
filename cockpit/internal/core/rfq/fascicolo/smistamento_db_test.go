//go:build integrazione

// L4 — Smistamento F8: il flusso ancorato al prodotto contro PostgreSQL vero (addendum A5.13.6, A5.13.9).
// Prove 184, 186, 191, 192, 233 dell'addendum, e quella chiesta per questa fase: il flusso, su una RFQ con STEP,
// PDF e prodotti, non cambia il numero di righe di componente, componente_relazione e documento. Le prove degli
// inneschi dei worker (181-183, 185, 187) stanno in workerapi, quelle del web (188, 189, IN9) in web.

package fascicolo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// letto scrive la proposta di documento dell'allegato con la lettura dell'analisi (Valuta con le regole del
// cliente della RFQ, le colonne dal riepilogo), come la lascia il worker.
func (b *banco) letto(allegato uuid.UUID, nome string, esito *classificazione.Esito, fatti string) uuid.UUID {
	b.t.Helper()
	var m *classificazione.Motore
	ok(b.t, b.tx(func(q *db.Queries) (err error) { m, err = fascicolo.MotoreDellaRfq(b.ctx, q, b.thread); return err }))
	in := classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: nome, Direzione: "entrata", Motore: m, Esito: esito}
	if fatti != "" {
		in.Fatti = json.RawMessage(fatti)
	}
	dett, rp := classificazione.ConValutazione(in.Fatti, classificazione.Valuta(in), time.Now())
	return uno[uuid.UUID](b, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8)
		ON CONFLICT (allegato_id) DO UPDATE SET dettagli = EXCLUDED.dettagli RETURNING proposta_id`,
		allegato, b.thread, rp.Tipo, rp.Codice, rp.Rev, rp.Confidenza, rp.Fonte, dett)
}

// disegnoPdf e' un PDF della RFQ letto come disegno (i termini del cartiglio nel testo).
func (b *banco) disegnoPdf(msg uuid.UUID, nome string) uuid.UUID {
	b.t.Helper()
	b.n++
	a := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il, stato)
		VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, now(), 'analizzato') RETURNING allegato_id`, msg, b.n, nome, fmt.Sprintf("%064x", 70000+b.n))
	b.letto(a, nome, &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, `{"cartiglio": true, "termini_trovati": ["SCALA"]}`)
	return a
}

// capitolatoPdf e' un PDF con i termini di un capitolato.
func (b *banco) capitolatoPdf(msg uuid.UUID, nome string) uuid.UUID {
	b.t.Helper()
	b.n++
	a := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il, stato)
		VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, now(), 'analizzato') RETURNING allegato_id`, msg, b.n, nome, fmt.Sprintf("%064x", 80000+b.n))
	b.letto(a, nome, &classificazione.Esito{Tipo: "capitolato", Fonte: "cartiglio"}, `{"termini_trovati": ["REQUISITI", "FORNITURA"]}`)
	return a
}

// stepLetto e' uno STEP della RFQ con la sua lettura e la sua struttura applicata.
func (b *banco) stepLetto(nome, sha string, f fattiSTEP) db.Allegato {
	b.t.Helper()
	a := b.allegatoStep(nome, sha)
	b.letto(a.AllegatoID, nome, &classificazione.Esito{Tipo: "cad_3d", Fonte: "step"}, "")
	b.applica(a, f.json())
	return a
}

// prodottoConfermato e' un codice della richiesta confermato al triage, con il suo finito.
func (b *banco) prodottoConfermato(codice string) uuid.UUID {
	b.t.Helper()
	b.identificativo(codice, "proposta_famiglia", true)
	return b.componente(codice, db.TipoComponenteFinito)
}

func (b *banco) rismista() fascicolo.EsitoRismista {
	b.t.Helper()
	es, err := fascicolo.Rismista(b.ctx, b.p, b.thread, fascicolo.AmbitoRfq(), analizzatoreProva)
	ok(b.t, err)
	return es
}

// destinazione legge dettagli.destinazione della proposta dell'allegato; ok falso se non c'e'.
func (b *banco) destinazione(allegato uuid.UUID) (fascicolo.Destinazione, bool) {
	b.t.Helper()
	var raw []byte
	b.riga(`SELECT coalesce(dettagli -> 'destinazione', 'null'::jsonb) FROM documento_proposta WHERE allegato_id = $1`, &raw, allegato)
	var d *fascicolo.Destinazione
	if err := json.Unmarshal(raw, &d); err != nil {
		b.t.Fatal(err)
	}
	if d == nil {
		return fascicolo.Destinazione{}, false
	}
	return *d, true
}

func (b *banco) dest(allegato uuid.UUID) fascicolo.Destinazione {
	b.t.Helper()
	d, trovata := b.destinazione(allegato)
	if !trovata {
		b.t.Fatalf("nessuna destinazione scritta per %s", allegato)
	}
	return d
}

func righeDest(d fascicolo.Destinazione) string {
	var parti []string
	for _, c := range d.Candidati {
		x := fmt.Sprintf("%d %s %s", c.Rango, c.Codice, c.Regola)
		if c.Codice == "" {
			x = fmt.Sprintf("%d %s %s", c.Rango, c.Chiave, c.Regola)
		}
		if c.Bloccato != "" {
			x += " [" + c.Bloccato + "]"
		}
		parti = append(parti, x)
	}
	return strings.Join(parti, " | ")
}

// fotoNonFlusso e' tutto cio' che il flusso non deve toccare: le righe delle tabelle delle decisioni e della
// struttura, e le colonne delle proposte di documento (i dettagli senza la destinazione).
func (b *banco) fotoNonFlusso() string {
	b.t.Helper()
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT count(*) FROM componente)::text, (SELECT count(*) FROM componente_relazione)::text, (SELECT count(*) FROM documento)::text,
		(SELECT count(*) FROM documento_provenienza)::text, (SELECT count(*) FROM identificativo_thread)::text,
		(SELECT count(*) FROM documento_proposta)::text, (SELECT count(*) FROM componente_proposta)::text,
		(SELECT count(*) FROM relazione_proposta)::text, (SELECT count(*) FROM rimozione_proposta)::text,
		(SELECT coalesce(string_agg(to_jsonb(c)::text, '|' ORDER BY componente_id), '') FROM componente c),
		(SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY padre_id, figlio_id), '') FROM componente_relazione r),
		(SELECT coalesce(string_agg(to_jsonb(d)::text, '|' ORDER BY documento_id), '') FROM documento d),
		(SELECT coalesce(string_agg(to_jsonb(cp)::text, '|' ORDER BY proposta_id), '') FROM componente_proposta cp),
		(SELECT coalesce(string_agg(concat_ws(':', proposta_id, tipo_proposto, codice, rev, componente_id, confidenza, fonte, stato, deciso_da,
		        (dettagli - 'destinazione')::text), '|' ORDER BY proposta_id), '') FROM documento_proposta))`)
}

// scenaFlusso (senza regole del cliente: l'estrattore generico): il prodotto 7120001 con lo STEP autorizzato (7120010 figlio diretto accettato da una persona,
// 7120012 no), il prodotto 7120002 senza riferimento strutturale, tre disegni e un capitolato.
type scenaFlusso struct {
	prodotto, sotto                        uuid.UUID
	step                                   db.Allegato
	d7120010, d7120012, d7120002, capitolo uuid.UUID
	msg                                    uuid.UUID
}

func (b *banco) scenaFlusso() scenaFlusso {
	b.t.Helper()
	var s scenaFlusso
	s.prodotto = b.prodottoConfermato("7120001")
	b.prodottoConfermato("7120002")
	s.step = b.stepLetto("7120001A_1.stp", strings.Repeat("f8", 32), casoGuida())
	testutil.AutorizzaStep(b.t, b.p, b.thread, s.prodotto, s.step.AllegatoID, b.utente)
	b.applica(s.step, casoGuida().json())
	s.sotto = b.componente("7120010", db.TipoComponenteSottoassieme)
	ok(b.t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
		return err
	}))
	s.msg = b.mail()
	s.d7120010 = b.disegnoPdf(s.msg, "7120010.pdf")
	s.d7120012 = b.disegnoPdf(s.msg, "7120012.pdf")
	s.d7120002 = b.disegnoPdf(s.msg, "7120002.pdf")
	s.capitolo = b.capitolatoPdf(s.msg, "Capitolato fornitura.pdf")
	return s
}

// Prova chiesta per F8: il flusso, su una RFQ con STEP, PDF e prodotti, scrive solo destinazioni. Il numero di
// righe di componente, componente_relazione e documento (e di provenienze, codici della richiesta, proposte
// di documento e di struttura) non cambia, e nemmeno una colonna: cambia solo dettagli.destinazione.
func TestIlFlussoNonCreaComponentiRelazioniDocumenti(t *testing.T) {
	b := nuovoBanco(t)
	s := b.scenaFlusso()
	prima := b.fotoNonFlusso()
	es := b.rismista()
	if es.Scritte == 0 || es.File < 5 {
		t.Fatalf("il flusso non ha scritto le destinazioni: %+v", es)
	}
	if dopo := b.fotoNonFlusso(); dopo != prima {
		t.Errorf("il flusso ha toccato altro che le destinazioni:\nprima %s\ndopo  %s", prima, dopo)
	}
	if n := uno[int](b, `SELECT count(*) FROM documento_proposta WHERE thread_id = $1 AND dettagli ? 'destinazione'`, b.thread); n < 5 {
		t.Errorf("destinazioni scritte: %d", n)
	}
	if d := b.dest(s.d7120010); !d.Preselezionabile || righeDest(d) != "1 7120010 dest_nodo_diretto_nome" {
		t.Errorf("il disegno del figlio diretto accettato: %s %v", righeDest(d), d.Preselezionabile)
	}
}

// Prova 184 (P34, A5.13.9): il flusso non tocca le proposte decise, nemmeno con una decisione concorrente: la
// guardia sta nel WHERE, e l'UPDATE che aspetta la riga bloccata la rilegge decisa e non la scrive.
func TestIlFlussoNonToccaLeProposteDecise(t *testing.T) {
	b := nuovoBanco(t)
	s := b.scenaFlusso()
	b.esegui(`UPDATE documento_proposta SET stato = 'confermata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, s.d7120012, b.utente)
	b.esegui(`UPDATE documento_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, s.capitolo, b.utente)
	b.rismista()
	for _, a := range []uuid.UUID{s.d7120012, s.capitolo} {
		if _, trovata := b.destinazione(a); trovata {
			t.Errorf("una proposta decisa ha ricevuto una destinazione: %s", a)
		}
	}
	if _, trovata := b.destinazione(s.d7120010); !trovata {
		t.Fatal("la proposta aperta non ha la destinazione")
	}

	// la decisione concorrente: la riga e' bloccata da chi decide, lo scrittore aspetta e poi non la tocca
	p := uno[uuid.UUID](b, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, s.d7120002)
	tx, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer tx.Rollback(b.ctx)
	if _, err := tx.Exec(b.ctx, `UPDATE documento_proposta SET stato = 'confermata', deciso_da = $2, deciso_il = now() WHERE proposta_id = $1`, p, b.utente); err != nil {
		t.Fatal(err)
	}
	fatto := make(chan int64, 1)
	go func() {
		n, err := db.New(b.p).ScriviDestinazione(context.Background(), db.ScriviDestinazioneParams{PropostaID: p,
			Destinazione: json.RawMessage(`{"v": 1, "firma": "nuova", "esito": "proposta"}`), Firma: "nuova"})
		if err != nil {
			t.Error(err)
		}
		fatto <- n
	}()
	select {
	case n := <-fatto:
		t.Fatalf("lo scrittore non ha aspettato la decisione in corso: %d righe", n)
	case <-time.After(300 * time.Millisecond):
	}
	ok(t, tx.Commit(b.ctx))
	if n := <-fatto; n != 0 {
		t.Errorf("la destinazione ha riscritto una proposta decisa nel frattempo: %d righe", n)
	}
	if got := uno[string](b, `SELECT coalesce(dettagli -> 'destinazione' ->> 'firma', '') FROM documento_proposta WHERE proposta_id = $1`, p); got == "nuova" {
		t.Error("la firma nuova e' sulla proposta decisa")
	}
}

// Prova 186 (A5.13.6): ripetere l'evento non riscrive niente. La seconda volta ScriviDestinazione non tocca
// nessuna riga: lo xmin delle proposte resta quello.
func TestRipetereLEventoNonRiscriveNiente(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaFlusso()
	b.rismista()
	xmin := func() string {
		return uno[string](b, `SELECT string_agg(proposta_id::text || '@' || xmin::text, ' ' ORDER BY proposta_id) FROM documento_proposta WHERE thread_id = $1`, b.thread)
	}
	prima := xmin()
	if es := b.rismista(); es.Scritte != 0 || es.File == 0 {
		t.Errorf("il secondo giro ha riscritto: %+v", es)
	}
	if dopo := xmin(); dopo != prima {
		t.Errorf("lo xmin delle proposte e' cambiato:\n%s\n%s", prima, dopo)
	}
}

// Prova 191 (A5.13.7, E28): con la BOM congelata il flusso gira (sono proposte), i candidati verso un componente
// portano bom_congelata e non si preselezionano, e il riesame non si accende: il flusso non inserisce righe.
func TestConLaBomCongelataIlFlussoNonAccendeIlRiesame(t *testing.T) {
	b := nuovoBanco(t)
	s := b.scenaFlusso()
	fase := uno[uuid.UUID](b, `SELECT fase_log_id FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, b.thread)
	v1 := uno[uuid.UUID](b, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prova', $3) RETURNING bom_versione_id`, b.thread, fase, b.utente)
	b.esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, v1, b.utente)
	riesame := func() int {
		return uno[int](b, `SELECT count(*) FROM v_thread_da_riesaminare WHERE thread_id = $1`, b.thread)
	}
	prima, foto := riesame(), b.fotoNonFlusso()
	b.rismista()
	if riesame() != prima {
		t.Errorf("il riesame si e' acceso: %d → %d", prima, riesame())
	}
	if b.fotoNonFlusso() != foto {
		t.Error("con la BOM congelata il flusso ha toccato altro che le destinazioni")
	}
	d := b.dest(s.d7120010)
	if d.Preselezionabile || !strings.Contains(strings.Join(d.Candidati[0].Discordanze, " "), fascicolo.DiscBomCongelata) {
		t.Errorf("il candidato verso un componente con la BOM congelata: %v %+v", d.Preselezionabile, d.Candidati[0])
	}
	if c := b.dest(s.capitolo); !c.Preselezionabile {
		t.Errorf("il documento generale non dipende dalla BOM: %+v", c)
	}
}

// Prova 192 (R8, A5.13.6): la GET sa che le proposte non sono aggiornate e non scrive. Dopo il giro la firma
// dell'indice torna; cambiate le regole del cliente non torna piu', e chiederlo non cambia una riga; «Aggiorna
// le proposte» (un giro) la rimette a posto.
func TestLaGetSegnalaLeProposteDaAggiornareSenzaScrivere(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaFlusso()
	daAggiornare := func() bool {
		var si bool
		ok(t, b.tx(func(q *db.Queries) (err error) {
			si, err = fascicolo.DestinazioniDaAggiornare(b.ctx, q, b.thread, analizzatoreProva)
			return err
		}))
		return si
	}
	if !daAggiornare() {
		t.Fatal("prima del primo giro le destinazioni mancano: vanno aggiornate")
	}
	b.rismista()
	if daAggiornare() {
		t.Fatal("dopo il giro le destinazioni sono aggiornate")
	}
	b.esegui(`UPDATE cliente SET regole = '{"suffissi_decorativi": ["_PRT"]}' FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $1`, b.thread)
	stato := func() string {
		return uno[string](b, `SELECT concat_ws(' ', (SELECT string_agg(xmin::text, ',' ORDER BY proposta_id) FROM documento_proposta),
			(SELECT string_agg(xmin::text, ',' ORDER BY proposta_id) FROM componente_proposta), (SELECT count(*) FROM job)::text)`)
	}
	prima := stato()
	if !daAggiornare() {
		t.Error("con le regole del cliente cambiate le destinazioni non sono aggiornate")
	}
	if stato() != prima {
		t.Error("la domanda della GET ha scritto")
	}
	if es := b.rismista(); es.Scritte == 0 {
		t.Errorf("«Aggiorna le proposte» non ha riscritto: %+v", es)
	}
	if daAggiornare() {
		t.Error("dopo «Aggiorna le proposte» le destinazioni sono ancora da aggiornare")
	}
}

// Prova 233: le destinazioni sulle scene principali, nel JSON della proposta. Radice uguale (lo STEP del
// prodotto: il suo 3D), radice vicina (bloccata), figlio diretto di uno STEP autorizzato accettato
// (preselezionabile) e da accettare (no), candidato bloccato di uno STEP non autorizzato, «sospesa» senza
// riferimento, il capitolato generale; nessuna scrittura sulle righe decise, e la GET non scrive.
func TestLeDestinazioniSulleScenePrincipali(t *testing.T) {
	b := nuovoBanco(t)
	s := b.scenaFlusso()
	b.prodottoConfermato("7120003")
	vicina := b.stepLetto("7120003A.stp", strings.Repeat("a3", 32), fattiSTEP{nodi: []string{"#1=7120003A", "#2=7120030"}, archi: []string{"#1>#2"}})
	b.prodottoConfermato("7120004")
	nonAut := b.stepLetto("7120004.stp", strings.Repeat("a4", 32), fattiSTEP{nodi: []string{"#1=7120004", "#2=7120040"}, archi: []string{"#1>#2"}})
	d7120040 := b.disegnoPdf(s.msg, "7120040.pdf")
	decisa := b.disegnoPdf(s.msg, "7120010 vecchio.pdf")
	b.esegui(`UPDATE documento_proposta SET stato = 'confermata', deciso_da = $2 WHERE allegato_id = $1`, decisa, b.utente)
	b.rismista()

	casi := []struct {
		nome        string
		allegato    uuid.UUID
		esito, riga string
		presel      bool
	}{
		{"radice uguale, lo STEP autorizzato del prodotto", s.step.AllegatoID, fascicolo.EsitoProposta, "1 7120001 dest_radice_uguale", false},
		{"radice vicina", vicina.AllegatoID, fascicolo.EsitoProposta, "1 7120003 dest_radice_vicina [radice_diversa]", false},
		{"radice uguale di uno STEP non autorizzato", nonAut.AllegatoID, fascicolo.EsitoProposta, "1 7120004 dest_radice_uguale", true},
		{"figlio diretto accettato", s.d7120010, fascicolo.EsitoProposta, "1 7120010 dest_nodo_diretto_nome", true},
		{"figlio diretto da accettare", s.d7120012, fascicolo.EsitoProposta, "1 7120012 dest_nodo_diretto_nome", false},
		{"figlio di uno STEP non autorizzato", d7120040, fascicolo.EsitoProposta, "1 7120040 dest_nodo_non_autorizzato [step_non_autorizzato]", false},
		{"prodotto senza riferimento", s.d7120002, fascicolo.EsitoSospesa, "", false},
		{"capitolato", s.capitolo, fascicolo.EsitoProposta, "1 generale dest_generale_contenuto", true},
	}
	for _, c := range casi {
		d := b.dest(c.allegato)
		if d.Esito != c.esito || righeDest(d) != c.riga || d.Preselezionabile != c.presel {
			t.Errorf("%s: %s «%s» preselezionabile %v; attesi %s «%s» %v", c.nome, d.Esito, righeDest(d), d.Preselezionabile, c.esito, c.riga, c.presel)
		}
		if d.V != fascicolo.VersioneDestinazione || d.Tabella != classificazione.TabellaPunteggi || d.Firma == "" || d.IndiceFirma == "" {
			t.Errorf("%s: testata della destinazione %+v", c.nome, d)
		}
	}
	if d := b.dest(s.d7120002); d.Motivo != fascicolo.MotivoManca || strings.Join(d.ProdottiSenzaAncora, " ") != "7120002" {
		t.Errorf("il prodotto senza riferimento: %s %v", d.Motivo, d.ProdottiSenzaAncora)
	}
	if d := b.dest(vicina.AllegatoID); d.Candidati[0].Ruolo != "candidato_strutturale" {
		t.Errorf("la radice vicina: %+v", d.Candidati[0])
	}
	if _, trovata := b.destinazione(decisa); trovata {
		t.Error("una proposta decisa ha ricevuto una destinazione")
	}
	// la GET: nessuna scrittura
	prima := uno[string](b, `SELECT string_agg(xmin::text, ',' ORDER BY proposta_id) FROM documento_proposta`)
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.DestinazioniDaAggiornare(b.ctx, q, b.thread, analizzatoreProva)
		return err
	}))
	if dopo := uno[string](b, `SELECT string_agg(xmin::text, ',' ORDER BY proposta_id) FROM documento_proposta`); dopo != prima {
		t.Error("la lettura delle destinazioni ha scritto")
	}
}
