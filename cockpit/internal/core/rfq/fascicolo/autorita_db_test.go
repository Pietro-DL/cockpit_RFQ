//go:build integrazione

// L4 — Smistamento F5 (addendum A5.4): lo STEP autorizzato per un componente, l'autorita' sui figli diretti e
// la guida, contro PostgreSQL vero. Le regole pure stanno in autorita_test.go.
//
// Prove dell'addendum: 90, 109, 110, 115, 116, 117 (parte), 127, 128, 202, 210, 215 (parte); e le due chieste
// dall'utente il 27/09: nessuna riga di proposta sparisce, e il codice uguale e' un suggerimento, mai
// un'identita' (quest'ultima, con i dati dell'editor, in web/autorita_db_test.go).

package fascicolo_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// acme: le regole del cliente ACME, la famiglia 712 dei codici di prova.
func (b *banco) acme() {
	b.t.Helper()
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`, b.thread)
}

// casoGuida: lo STEP del prodotto (7120001A_1.stp): la radice 7120001, i figli diretti 7120010 e 7120012 ×2,
// e 7120011 ×2 sotto 7120010.
func casoGuida() fattiSTEP {
	return fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120012", "#4=7120011"}, archi: []string{"#1>#2", "#1>#3*2", "#2>#4*2"}}
}

// righeProposte conta le righe di proposta della RFQ, nodi e archi.
func (b *banco) righeProposte() int {
	return uno[int](b, `SELECT (SELECT count(*) FROM componente_proposta WHERE thread_id = $1) + (SELECT count(*) FROM relazione_proposta WHERE thread_id = $1)`, b.thread)
}

// strutturali: le decisioni strutturali aperte che il gate conta, con le sorgenti e le rimozioni valide della
// RFQ (come LeggiGate).
func (b *banco) strutturali() db.GateStrutturaleRow {
	b.t.Helper()
	var g db.GateStrutturaleRow
	ok(b.t, b.tx(func(q *db.Queries) error {
		d, err := fascicolo.LeggiDichiarazioni(b.ctx, q, b.thread)
		if err != nil {
			return err
		}
		s, r := d.PerIlGate()
		g, err = q.GateStrutturale(b.ctx, db.GateStrutturaleParams{ThreadID: b.thread, Sorgenti: s, RimozioniValide: r})
		return err
	}))
	return g
}

// La prova chiesta dall'utente (27/09): dopo ApplicaStruttura il numero di righe di proposta non diminuisce mai.
// Tutti gli STEP si analizzano e la struttura resta; F5 cambia che cosa le righe significano, non le perde: una
// lettura come guida, l'autorizzazione, un figlio deciso, le regole del cliente cambiate, una rianalisi che ha
// perso un nodo, lo stesso contenuto in un'altra copia, un file escluso riletto.
func TestApplicaStrutturaNonTogliMaiRighe(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	sha := strings.Repeat("a1", 32)
	a := b.allegatoStep("7120001A_1.stp", sha)
	passi := []struct {
		nome string
		fai  func()
	}{
		{"la prima lettura, tutta guida", func() { b.applica(a, casoGuida().json()) }},
		{"l'autorizzazione per il prodotto", func() {
			testutil.AutorizzaStep(t, b.p, b.thread, prodotto, a.AllegatoID, b.utente)
			b.applica(a, casoGuida().json())
		}},
		{"un figlio diretto deciso", func() { b.accetta("#2") }},
		{"le regole del cliente cambiate", func() {
			b.esegui(`UPDATE cliente SET regole = '{}' FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $1`, b.thread)
			b.applica(a, casoGuida().json())
		}},
		{"una rianalisi che ha perso un nodo e un arco", func() {
			b.applica(a, fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120012"}, archi: []string{"#1>#2", "#1>#3*2"}}.json())
		}},
		{"lo stesso contenuto in un'altra copia", func() { b.applica(b.allegatoStep("copia.stp", sha), casoGuida().json()) }},
		{"il file escluso e riletto", func() {
			b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte, stato, deciso_da, deciso_il)
				VALUES ($1, $2, 'cad_3d', 90, 'estensione', 'scartata', $3, now())`, a.AllegatoID, b.thread, b.utente)
			if es := b.applica(a, casoGuida().json()); !es.FuoriFlusso {
				t.Errorf("il file escluso non si rilegge: %+v", es)
			}
		}},
	}
	prima := 0
	for _, p := range passi {
		p.fai()
		n := b.righeProposte()
		if n < prima {
			t.Errorf("%s: le righe di proposta sono passate da %d a %d", p.nome, prima, n)
		}
		if n == 0 {
			t.Errorf("%s: nessuna riga di proposta", p.nome)
		}
		prima = n
	}
	// il nodo e l'arco che la rianalisi non ha piu' restano, com'erano
	if got := uno[string](b, `SELECT (SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND chiave = '#4') || ' ' ||
		(SELECT count(*) FROM relazione_proposta WHERE thread_id = $1 AND padre_chiave = '#2' AND figlio_chiave = '#4')`, b.thread); got != "1 1" {
		t.Errorf("il nodo e l'arco persi dalla rianalisi: %s, attesi ancora li'", got)
	}
}

// Prova 109 (P9, A5.4.7): fuori dall'autorita' le proposte non si accettano. Un file che nessuno ha autorizzato,
// con i codici dei componenti che la RFQ ha gia': il nodo, l'arco, il sottoalbero, il file e l'editor con un
// «p:» si rifiutano, e la BOM non cambia.
func TestFuoriDallAutoritaLeProposteNonSiAccettano(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	b.componente("7120010", db.TipoComponenteSottoassieme)
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("b1", 32))
	b.applica(a, casoGuida().json())
	prima, righe := b.bom(), b.nodiProposti()+" | "+b.relazioniProposte()
	if righe != "#1:aperta #2:aperta #3:aperta #4:aperta | #1>#2*1:aperta #1>#3*2:aperta #2>#4*2:aperta" {
		t.Errorf("la lettura di un file non autorizzato, con i codici dei componenti della RFQ: %s", righe)
	}
	k := fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"}
	gesti := []struct {
		nome, frase string
		f           func(q *db.Queries) (string, error)
	}{
		{"nodo", "7120010 è guida", func(q *db.Queries) (string, error) {
			return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
		}},
		{"arco", "è guida", func(q *db.Queries) (string, error) {
			return fascicolo.AccettaRelazione(b.ctx, q, b.thread, k, b.utente)
		}},
		{"sottoalbero", "7120001 è guida", func(q *db.Queries) (string, error) {
			return fascicolo.AccettaSottoalbero(b.ctx, q, b.thread, a.AllegatoID, "#1", b.utente)
		}},
		{"file", "non è autorizzato", func(q *db.Queries) (string, error) {
			return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
		}},
		{"editor con p:", "7120010 è guida", func(q *db.Queries) (string, error) {
			return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, fascicolo.StrutturaVoluta{Radice: prodotto,
				Archi: []fascicolo.ArcoVoluto{{Padre: "c:" + prodotto.String(), Figlio: "p:" + b.proposta("#2").String(), Qta: 1}}})
		}},
	}
	for _, g := range gesti {
		_, err := b.gesto(g.f)
		deveRifiutare(t, err, g.frase)
	}
	if dopo := b.bom(); dopo != prima {
		t.Errorf("la BOM e' cambiata: %s → %s", prima, dopo)
	}
	if dopo := b.nodiProposti() + " | " + b.relazioniProposte(); dopo != righe {
		t.Errorf("le proposte sono cambiate: %s → %s", righe, dopo)
	}
}

// Prova 110 (A5.4.5): la guida si affina. Il codice di un nodo di guida si corregge, un nodo si scarta e si
// riapre, con la storia; sono correzioni dell'evidenza, non della BOM, e sopravvivono alla rilettura e
// all'autorizzazione: autorizzato il file, il nodo corretto e' un figlio diretto con il codice dell'operatore, e
// quello scartato resta scartato.
func TestLaGuidaSiCorreggeSiScartaESiRiapre(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=Part2", "#3=7120012", "#4=7120099"}, archi: []string{"#1>#2", "#1>#3", "#1>#4"}}
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("c1", 32))
	b.applica(a, f.json())
	bom := b.bom()
	gesti := []func(q *db.Queries) (string, error){
		func(q *db.Queries) (string, error) {
			return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#2"), "7120010", "")
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.proposta("#4"), b.utente)
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.RiapriNodo(b.ctx, q, b.thread, b.proposta("#4"), b.utente)
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.proposta("#4"), b.utente)
		},
	}
	for i, g := range gesti {
		if _, err := b.gesto(g); err != nil {
			t.Fatalf("gesto %d sulla guida: %v", i, err)
		}
	}
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.RiapriNodo(b.ctx, q, b.thread, b.proposta("#3"), b.utente)
	})
	deveRifiutare(t, err, "non è scartato")
	if b.bom() != bom {
		t.Errorf("le correzioni della guida hanno cambiato la BOM")
	}
	stato := func() string {
		return uno[string](b, `SELECT string_agg(chiave || '=' || coalesce(codice, '-') || '/' || coalesce(origine_codice::text, '-') || ':' || stato ||
			':' || (deciso_da IS NOT NULL), ' ' ORDER BY chiave) FROM componente_proposta WHERE thread_id = $1 AND chiave IN ('#2', '#4')`, b.thread)
	}
	const atteso = "#2=7120010/operatore:aperta:false #4=7120099/famiglia:scartata:true"
	if got := stato(); got != atteso {
		t.Errorf("dopo le correzioni: %s, atteso %s", got, atteso)
	}
	if got := uno[string](b, `SELECT (evidenza -> 'storia' -> 0 ->> 'evento') || ':' || (evidenza -> 'storia' -> 0 ->> 'riaperto_da')
		FROM componente_proposta WHERE thread_id = $1 AND chiave = '#4'`, b.thread); got != "nodo_riaperto:"+b.utente.String() {
		t.Errorf("la storia della riapertura: %s", got)
	}
	b.applica(a, f.json())
	if got := stato(); got != atteso {
		t.Errorf("la rilettura ha perso le correzioni: %s", got)
	}
	b.autorizza(prodotto, a, f.json())
	if got := stato(); got != atteso {
		t.Errorf("l'autorizzazione ha perso le correzioni: %s", got)
	}
	b.accetta("#2")
	if got := b.bom(); !strings.Contains(got, "7120010:sciolto:-") {
		t.Errorf("il nodo corretto, adesso figlio diretto, entra con il codice dell'operatore: %s", got)
	}
}

// Prova 90 (K2, E34): la classificazione del server resta accanto al codice dell'operatore. Ogni lettura scrive
// evidenza.classificato; il codice scritto a mano sta nella colonna e non si riclassifica; una rilettura uguale
// non riscrive niente (stessoNodo stabile); con le regole cambiate cambia la classificazione, non il codice
// dell'operatore; una riga letta prima di F5 (senza classificato) lo riceve quando l'operatore scrive il codice.
func TestLaClassificazioneDelServerRestaAccantoAlCodiceDellOperatore(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010"}, archi: []string{"#1>#2"}}
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("d1", 32))
	b.applica(a, f.json())
	classificato := func(k string) string {
		return uno[string](b, `SELECT coalesce(codice, '-') || ' | ' || (evidenza -> 'classificato' ->> 'codice') || '/' || (evidenza -> 'classificato' ->> 'origine') ||
			'/' || (evidenza -> 'classificato' ->> 'famiglia') FROM componente_proposta WHERE thread_id = $1 AND chiave = $2`, b.thread, k)
	}
	if got := classificato("#2"); got != "7120010 | 7120010/famiglia/ACME 712" {
		t.Fatalf("la lettura scrive la classificazione: %s", got)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#2"), "7120011", "")
	}); err != nil {
		t.Fatal(err)
	}
	if es := b.applica(a, f.json()); es.Nodi != 0 {
		t.Errorf("una rilettura uguale ha riscritto %d nodi", es.Nodi)
	}
	if got := classificato("#2"); got != "7120011 | 7120010/famiglia/ACME 712" {
		t.Errorf("il codice dell'operatore e la classificazione del server: %s", got)
	}
	b.esegui(`UPDATE cliente SET regole = '{}' FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $1`, b.thread)
	b.applica(a, f.json())
	if got := classificato("#2"); !strings.HasPrefix(got, "7120011 | ") || strings.Contains(got, "famiglia/ACME") {
		t.Errorf("regole cambiate: il codice dell'operatore resta, la classificazione si rifa': %s", got)
	}
	// una riga di prima di F5, senza la classificazione: la riceve quando l'operatore scrive il codice
	b.esegui(`UPDATE componente_proposta SET evidenza = evidenza - 'classificato', codice = '7120001', origine_codice = 'famiglia', famiglia = 'ACME 712'
		WHERE thread_id = $1 AND chiave = '#1'`, b.thread)
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#1"), "7120002", "")
	}); err != nil {
		t.Fatal(err)
	}
	if got := classificato("#1"); got != "7120002 | 7120001/famiglia/ACME 712" {
		t.Errorf("la classificazione salvata prima del codice dell'operatore: %s", got)
	}
}

// Prova 215 (parte, P30): la storia del nodo sopravvive alla rilettura. Un nodo riaperto ha la storia; la
// lettura lo riscrive con un'evidenza nuova, e la storia resta; una rilettura uguale non lo riscrive.
func TestLaStoriaDelNodoSopravviveAllaRilettura(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("e1", 32))
	b.applica(a, casoGuida().json())
	for _, g := range []func(q *db.Queries) (string, error){
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.proposta("#3"), b.utente)
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.RiapriNodo(b.ctx, q, b.thread, b.proposta("#3"), b.utente)
		},
	} {
		if _, err := b.gesto(g); err != nil {
			t.Fatal(err)
		}
	}
	storia := func() string {
		return uno[string](b, `SELECT coalesce(evidenza -> 'storia' ->> 0, '-') || ' | ' || coalesce(codice, '-')
			FROM componente_proposta WHERE thread_id = $1 AND chiave = '#3'`, b.thread)
	}
	prima := storia()
	if !strings.Contains(prima, "nodo_riaperto") {
		t.Fatalf("la storia della riapertura: %s", prima)
	}
	// una rianalisi che chiama il nodo in un altro modo: la riga si riscrive, la storia resta
	g := casoGuida()
	g.nodi[2] = "#3=7120013"
	if es := b.applica(a, g.json()); es.Nodi == 0 {
		t.Fatalf("la rianalisi doveva riscrivere il nodo")
	}
	if got := storia(); !strings.HasPrefix(got, strings.Split(prima, " | ")[0]) || !strings.HasSuffix(got, "| 7120013") {
		t.Errorf("dopo la rilettura: %s, prima %s", got, prima)
	}
	if es := b.applica(a, g.json()); es.Nodi != 0 {
		t.Errorf("una rilettura uguale ha riscritto %d nodi: la storia non conta nel confronto", es.Nodi)
	}
}

// Prova 115: il riesame ignora la guida, anche con la BOM congelata. v_thread_da_riesaminare conta ogni
// proposta aperta nata dopo la baseline (E28: e' una vista, e senza migrazione resta cosi'); il Go la riconta
// solo nell'autorita'. Un file che nessuno ha autorizzato non accende il riesame; lo STEP strutturale del
// prodotto, riletto dopo il congelamento, si'.
func TestIlRiesameIgnoraLaGuida(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	if _, err := b.congela("prima baseline"); err != nil {
		t.Fatal(err)
	}
	riesame := func() (vista, go_ string) {
		var v, g []string
		ok(t, b.tx(func(q *db.Queries) error {
			righe, err := q.ListThreadDaRiesaminare(b.ctx, b.thread)
			for _, x := range righe {
				v = append(v, fmt.Sprintf("%s:%d", x.TipoMotivo, x.NProposte))
			}
			if err != nil {
				return err
			}
			filtrate, err := fascicolo.RiesameNellAutorita(b.ctx, q, b.thread)
			for _, x := range filtrate {
				g = append(g, fmt.Sprintf("%s:%d", x.TipoMotivo, x.NProposte))
			}
			return err
		}))
		return strings.Join(v, " "), strings.Join(g, " ")
	}
	guida := b.allegatoStep("altro.stp", strings.Repeat("f1", 32))
	b.applica(guida, fattiSTEP{nodi: []string{"#1=A100", "#2=B200"}, archi: []string{"#1>#2"}}.json())
	if v, g := riesame(); v != "evidenze_nuove:3" || g != "" {
		t.Errorf("la guida: vista %q (conta la guida), riesame %q (atteso vuoto)", v, g)
	}
	// lo STEP strutturale di P1 (la forma di prima dell'autorizzazione), letto dopo il congelamento
	f := fattiSTEP{nodi: []string{"#1=P1", "#2=F1", "#3=X1"}, archi: []string{"#1>#2*2", "#1>#3"}}
	b.analisi(r.step, f.json())
	b.applica(b.allegatoStep("P1.stp", b.sha(r.step)), f.json())
	v, g := riesame()
	if v != "evidenze_nuove:8" || g != "evidenze_nuove:4" {
		t.Errorf("con l'autorita': vista %q, riesame %q (attesi 2 figli diretti e 2 archi)", v, g)
	}
}

// Prova 116 (critica L1, E26): scartare uno STEP non lascia proposte utilizzabili. Escluso da una persona, il file
// non si rilegge (ApplicaStruttura lo dice fuori dal flusso, RileggiStepDellaRfq lo salta), le sue righe restano
// com'erano (fatti), nessuna si accetta, e l'analisi resta fra quelle da fare.
func TestScartareUnoStepNonLasciaProposteUtilizzabili(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	b.componente("7120001", db.TipoComponenteFinito)
	a := b.allegatoStep("scartato.stp", strings.Repeat("a2", 32))
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'cad_3d', 90, 'estensione')`,
		a.AllegatoID, b.thread)
	b.applica(a, casoGuida().json())
	righe := b.nodiProposti() + " | " + b.relazioniProposte()
	b.esegui(`UPDATE documento_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, a.AllegatoID, b.utente)
	if es := b.applica(a, fattiSTEP{nodi: []string{"#1=7120001", "#9=7120099"}, archi: []string{"#1>#9"}}.json()); !es.FuoriFlusso || es.Nodi+es.Relazioni != 0 {
		t.Errorf("il file escluso si e' riletto: %+v", es)
	}
	if got := b.nodiProposti() + " | " + b.relazioniProposte(); got != righe {
		t.Errorf("le righe del file escluso sono cambiate: %s → %s", righe, got)
	}
	ok(t, b.tx(func(q *db.Queries) error {
		step, err := q.ListStepDellaRfq(b.ctx, uuid.NullUUID{UUID: b.thread, Valid: true})
		if err != nil {
			return err
		}
		tutti, err := q.ListStepDaAnalizzareDellaRfq(b.ctx, uuid.NullUUID{UUID: b.thread, Valid: true})
		if err != nil {
			return err
		}
		if len(step) != 0 || len(tutti) != 1 {
			t.Errorf("STEP da rileggere %d (atteso 0), da analizzare %d (atteso 1)", len(step), len(tutti))
		}
		return nil
	}))
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
	})
	deveRifiutare(t, err, "è guida")
	if g := b.strutturali(); g.NFigliDaDecidere+g.NArchiDaDecidere != 0 {
		t.Errorf("le proposte dello STEP escluso contano nel gate: %+v", g)
	}
}

// Prova 117 (parte, P27, E27): la posta del fornitore e le nostre mail in uscita restano fuori dalla struttura:
// il loro STEP non fa guida ne' autorita', non si rilegge e non riceve il fan-out. Un caricamento interno (il
// canale nota) invece e' del progetto.
func TestLaPostaDelFornitoreEMessaDaParteENonGuida(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	fornitore := uno[uuid.UUID](b, `INSERT INTO fornitore (ragione_sociale, tipo) VALUES ('Fornitore Esempio', 'processi') RETURNING fornitore_id`)
	allegato := func(canale, direzione, controparte string, contr uuid.NullUUID, sha string) db.Allegato {
		b.n++
		conv := uno[uuid.UUID](b, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ($1, $2, now()) RETURNING conversazione_id`,
			canale, fmt.Sprintf("C-F5-%d", b.n))
		msg := uno[uuid.UUID](b, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id, controparte_tipo, controparte_fornitore_id)
			VALUES ($1, $2, $3, $4, now(), $5, $6, $7) RETURNING messaggio_id`, canale, fmt.Sprintf("<f5-%d@acme.example>", b.n), conv, direzione, b.thread, controparte, contr)
		id := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il)
			VALUES ($1, 1, '7120001.stp', 'stp', 'file', 'outlook', 100, $2, now()) RETURNING allegato_id`, msg, sha)
		a, err := db.New(b.p).GetAllegato(b.ctx, id)
		ok(t, err)
		return a
	}
	fuori := []db.Allegato{
		allegato("outlook", "entrata", "fornitore", uuid.NullUUID{UUID: fornitore, Valid: true}, strings.Repeat("b2", 32)),
		allegato("outlook", "uscita", "sconosciuto", uuid.NullUUID{}, strings.Repeat("c2", 32)),
	}
	for _, a := range fuori {
		if es := b.applica(a, casoGuida().json()); !es.FuoriFlusso {
			t.Errorf("%s: il file fuori dal flusso si e' letto: %+v", a.Sha256.String[:4], es)
		}
	}
	if n := b.righeProposte(); n != 0 {
		t.Errorf("%d righe di proposta da file che non sono del cliente", n)
	}
	nota := allegato("nota", "entrata", "interno", uuid.NullUUID{}, strings.Repeat("d2", 32))
	if es := b.applica(nota, casoGuida().json()); es.FuoriFlusso || es.Nodi != 4 {
		t.Errorf("il caricamento interno e' del progetto: %+v", es)
	}
	ok(t, b.tx(func(q *db.Queries) error {
		step, err := q.ListStepDellaRfq(b.ctx, uuid.NullUUID{UUID: b.thread, Valid: true})
		if err != nil {
			return err
		}
		if len(step) != 1 || step[0].AllegatoID != nota.AllegatoID {
			t.Errorf("STEP della RFQ da rileggere: %d, atteso il solo caricamento interno", len(step))
		}
		for _, a := range fuori {
			rfq, err := q.ListRfqAperteConContenuto(b.ctx, a.Sha256)
			if err != nil {
				return err
			}
			if len(rfq) != 0 {
				t.Errorf("il fan-out raggiunge un file che non e' del cliente: %d", len(rfq))
			}
		}
		return nil
	}))
}

// Prova 127 (E33): le chiusure automatiche si riaprono. Nodi e archi chiusi da un automatismo (un file
// sostituito: scartati senza chi li ha decisi) tornano aperti alla rilettura; lo scarto di una persona resta.
// Una rimozione chiusa perche' l'arco era uscito dalla working si riapre quando l'arco torna; quella scartata da
// una persona no. RiapriArchiAutomaticiDiUnFile riapre gli archi automatici dell'autorita' (per la revoca di
// F5b), con la storia, e lascia quelli decisi da una persona.
func TestLeChiusureAutomaticheSiRiaprono(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("e2", 32))
	b.applica(a, casoGuida().json())
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.proposta("#4"), b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	nota := pgtype.Text{String: "superata da 7120001A_2.stp", Valid: true}
	ok(t, b.tx(func(q *db.Queries) error {
		if _, err := q.ChiudiProposteComponenteDiUnFile(b.ctx, db.ChiudiProposteComponenteDiUnFileParams{ThreadID: b.thread, Sha256: a.Sha256.String, Nota: nota}); err != nil {
			return err
		}
		_, err := q.ChiudiProposteRelazioneDiUnFile(b.ctx, db.ChiudiProposteRelazioneDiUnFileParams{ThreadID: b.thread, Sha256: a.Sha256, Nota: nota})
		return err
	}))
	if got := b.nodiProposti(); got != "#1:scartata #2:scartata #3:scartata #4:scartata" {
		t.Fatalf("chiusi: %s", got)
	}
	b.applica(a, casoGuida().json())
	if got, want := b.nodiProposti()+" | "+b.relazioniProposte(), "#1:aperta #2:aperta #3:aperta #4:scartata | #1>#2*1:aperta #1>#3*2:aperta #2>#4*2:aperta"; got != want {
		t.Errorf("riletti: %s, attesi %s", got, want)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND nota IS NOT NULL`, b.thread); n != 0 {
		t.Errorf("%d nodi riaperti con la nota della chiusura", n)
	}

	// le rimozioni: P1 → B, C, D, lo STEP strutturale di P1 con i figli diretti decisi propone P1 → D
	b2 := nuovoBanco(t)
	c := b2.workingPBCD()
	doc, a2 := b2.stepDelProdotto(c["P1"], codiceDi("P1"), stepPBCD())
	b2.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	b2.applica(a2, stepPBCD().json())
	b2.accetta("#2", "#3", "#5")
	if got := b2.rimozioni(); got != "P1>D:aperta" {
		t.Fatalf("rimozioni = %q", got)
	}
	ricalcola := func() {
		ok(t, b2.tx(func(q *db.Queries) error {
			_, err := fascicolo.AggiornaTutteLeRimozioni(b2.ctx, q, b2.thread)
			return err
		}))
	}
	b2.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, c["P1"], c["D"])
	ricalcola()
	if got := b2.rimozioni(); got != "P1>D:scartata" {
		t.Fatalf("l'arco uscito dalla working chiude la rimozione: %q", got)
	}
	b2.arco(c["P1"], c["D"], 1)
	ricalcola()
	if got := b2.rimozioni(); got != "P1>D:aperta" {
		t.Errorf("l'arco tornato riapre la rimozione chiusa da un automatismo: %q", got)
	}
	if _, err := b2.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ScartaRimozione(b2.ctx, q, b2.thread, fascicolo.ChiaveRimozione{Step: doc, Padre: c["P1"], Figlio: c["D"]}, b2.utente)
	}); err != nil {
		t.Fatal(err)
	}
	ricalcola()
	if got := b2.rimozioni(); got != "P1>D:scartata" {
		t.Errorf("lo scarto di una persona resta: %q", got)
	}

	// gli archi automatici dell'autorita': P1 → C (duplicato, tenuta dei conti) torna aperto; P1 → F, deciso da
	// una persona, resta
	if _, err := b2.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b2.ctx, q, b2.thread, fascicolo.ChiaveRelazione{Allegato: a2.AllegatoID, Padre: "#1", Figlio: "#5"}, b2.utente)
	}); err != nil {
		t.Fatal(err)
	}
	var n int64
	ok(t, b2.tx(func(q *db.Queries) (err error) {
		n, err = q.RiapriArchiAutomaticiDiUnFile(b2.ctx, db.RiapriArchiAutomaticiDiUnFileParams{ThreadID: b2.thread, Sha256: a2.Sha256})
		return err
	}))
	if n != 1 {
		t.Errorf("archi automatici riaperti: %d, atteso 1 (P1 → C)", n)
	}
	if got := b2.relazioniProposte(); !strings.Contains(got, "#1>#3*1:aperta") || !strings.Contains(got, "#1>#5*1:confermata") {
		t.Errorf("relazioni = %q", got)
	}
	if got := uno[string](b2, `SELECT (evidenza -> 'storia' -> 0 ->> 'evento') || ':' || (evidenza -> 'storia' -> 0 ->> 'stato')
		FROM relazione_proposta WHERE thread_id = $1 AND padre_chiave = '#1' AND figlio_chiave = '#3'`, b2.thread); got != "arco_automatico_riaperto:duplicato" {
		t.Errorf("la storia dell'arco riaperto: %s", got)
	}
}

// Prova 202 (U3, A5.4.6): lo STEP di un sottoassieme, autorizzato per quel sottoassieme, ne propone i figli
// diretti; il nipote dal file del padre si rifiuta. L'autorita' si compone livello per livello: il file del
// prodotto comanda 7120001 → 7120010, il file di 7120010 comanda 7120010 → 7120011. Le rimozioni del
// sottoassieme si propongono a profondita' 1, con il documento del suo file.
func TestLoStepDiUnSottoassiemeNeProponeIFigliDiretti(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	pa := b.allegatoStep("7120001A_1.stp", strings.Repeat("f2", 32))
	b.autorizza(prodotto, pa, casoGuida().json())
	b.accetta("#2")
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: pa.AllegatoID, Padre: "#1", Figlio: "#2"}, b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	sotto := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '7120010'`, b.thread)
	vecchio := b.componente("7120098", db.TipoComponenteSciolto)
	b.arco(sotto, vecchio, 1)

	// il file di 7120010, non ancora autorizzato: guida
	sa := b.allegatoStep("7120010.stp", strings.Repeat("a3", 32))
	g := fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120099"}, archi: []string{"#1>#2*2", "#1>#3"}}
	b.fattiCorrenti(sa.Sha256.String, g.json())
	b.applica(sa, g.json())
	figlio := func(a db.Allegato, k string) uuid.UUID {
		return uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`, b.thread, a.AllegatoID, k)
	}
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, figlio(sa, "#2"), b.utente, "")
	})
	deveRifiutare(t, err, "7120011 è guida")

	// autorizzato per 7120010: i suoi figli diretti si accettano, e con loro l'arco
	testutil.AutorizzaStep(t, b.p, b.thread, sotto, sa.AllegatoID, b.utente)
	b.applica(sa, g.json())
	for _, k := range []string{"#2", "#3"} {
		if _, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.AccettaNodo(b.ctx, q, b.thread, figlio(sa, k), b.utente, "")
		}); err != nil {
			t.Fatalf("accetta %s dal file di 7120010: %v", k, err)
		}
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: sa.AllegatoID, Padre: "#1", Figlio: "#2"}, b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.bom(); !strings.Contains(got, "7120010>7120011*2") {
		t.Errorf("7120010 → 7120011 dal file di 7120010: %s", got)
	}
	// il nipote dal file del prodotto: guida, anche adesso che 7120011 e' un componente
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, figlio(pa, "#4"), b.utente, "")
	})
	deveRifiutare(t, err, "7120011 è guida")
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: pa.AllegatoID, Padre: "#2", Figlio: "#4"}, b.utente)
	})
	deveRifiutare(t, err, "è guida")
	// la rimozione del sottoassieme: 7120098 non e' nel file di 7120010, con il documento di quel file
	docSotto := uno[uuid.UUID](b, `SELECT documento_id FROM documento WHERE thread_id = $1 AND sha256 = $2`, b.thread, sa.Sha256.String)
	if got := uno[string](b, `SELECT string_agg(f.codice || ':' || r.stato || ':' || (r.step_documento_id = $2)::text, ' ') FROM rimozione_proposta r
		JOIN componente f ON f.componente_id = r.figlio_id WHERE r.thread_id = $1 AND r.padre_id = $3`, b.thread, docSotto, sotto); got != "7120098:aperta:true" {
		t.Errorf("rimozioni del sottoassieme: %s", got)
	}
	if got := b.rimozioni(); strings.Contains(got, "7120001>") {
		t.Errorf("il file del prodotto non propone rimozioni sotto un figlio: %s", got)
	}
	// archiviato il sottoassieme, la sua rimozione si chiude e l'autorizzazione e' sospesa (la marcatura resta);
	// il ripristino la rende di nuovo valida
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ArchiviaComponente(b.ctx, q, b.thread, sotto, b.utente, "il cliente lo compra finito")
	}); err != nil {
		t.Fatal(err)
	}
	if got := uno[string](b, `SELECT stato || ':' || coalesce(nota, '') FROM rimozione_proposta WHERE thread_id = $1 AND padre_id = $2`, b.thread, sotto); got != "scartata:componente archiviato" {
		t.Errorf("la rimozione del sottoassieme archiviato: %s", got)
	}
	stato := func() string {
		var s string
		ok(t, b.tx(func(q *db.Queries) error {
			d, err := fascicolo.LeggiDichiarazioni(b.ctx, q, b.thread)
			_, valida := d.Di(sotto)
			s = fmt.Sprintf("%v:%d", valida, len(d.Sospese))
			return err
		}))
		return s
	}
	if got := stato(); got != "false:1" {
		t.Errorf("l'autorizzazione del sottoassieme archiviato: %s, attesa sospesa", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND evidenza ? 'strutturale'`, b.thread); n != 2 {
		t.Errorf("le marcature restano: %d, attese 2", n)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) { return fascicolo.RipristinaComponente(b.ctx, q, b.thread, sotto) }); err != nil {
		t.Fatal(err)
	}
	if got := stato(); got != "true:0" {
		t.Errorf("ripristinato, l'autorizzazione torna valida: %s", got)
	}
}

// Prova 210 (A5.4.8, L2, L3): il gate conta solo l'autorita'. Le proposte di un file non autorizzato non
// contano; quelle di un file autorizzato si' (i figli diretti e gli archi dalla sorgente); un aggancio per codice
// di prima dello Smistamento dentro l'autorita' conta da decidere, finche' una persona non lo conferma;
// un'autorizzazione in conflitto ferma il gate.
func TestIlGateContaSoloLAutorita(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	altro := b.componente("7120010", db.TipoComponenteSottoassieme)
	guida := b.allegatoStep("guida.stp", strings.Repeat("b3", 32))
	b.applica(guida, casoGuida().json())
	if g := b.strutturali(); g.NFigliDaDecidere+g.NArchiDaDecidere+g.NRimozioni != 0 {
		t.Errorf("la guida non conta: %+v", g)
	}
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("c3", 32))
	b.autorizza(prodotto, a, casoGuida().json())
	if g := b.strutturali(); g.NFigliDaDecidere != 2 || g.NArchiDaDecidere != 2 {
		t.Errorf("l'autorita': %+v, attesi 2 figli diretti e 2 archi", g)
	}
	// un aggancio per codice di prima (duplicato senza chi l'ha deciso) dentro l'autorita': conta, con il suo arco
	b.esegui(`UPDATE componente_proposta SET stato = 'duplicato', componente_id = $2, deciso_il = now() WHERE thread_id = $1 AND allegato_id = $3 AND chiave = '#2'`,
		b.thread, altro, a.AllegatoID)
	b.esegui(`UPDATE relazione_proposta SET stato = 'duplicato', deciso_il = now() WHERE thread_id = $1 AND allegato_id = $2 AND figlio_chiave = '#2'`,
		b.thread, a.AllegatoID)
	if g := b.strutturali(); g.NFigliDaDecidere != 2 || g.NArchiDaDecidere != 2 {
		t.Errorf("l'aggancio per codice di prima conta da decidere: %+v", g)
	}
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, a.AllegatoID), b.utente, "")
	})
	ok(t, err)
	if !strings.Contains(msg, "confermato") {
		t.Errorf("messaggio: %q", msg)
	}
	if g := b.strutturali(); g.NFigliDaDecidere != 1 || g.NArchiDaDecidere != 1 {
		t.Errorf("confermato da una persona non conta piu' (ne' il suo arco automatico): %+v", g)
	}
	// il conflitto: 7120010 con due file autorizzati. Nel gate e' un problema, e nessuno dei due da' autorita'
	for i, sha := range []string{strings.Repeat("d3", 32), strings.Repeat("e3", 32)} {
		x := b.allegatoStep(fmt.Sprintf("7120010_%d.stp", i), sha)
		b.autorizza(altro, x, fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2"}}.json())
	}
	var g fascicolo.Gate
	ok(t, b.tx(func(q *db.Queries) (err error) { g, err = fascicolo.LeggiGate(b.ctx, q, b.thread); return err }))
	if !strings.Contains(g.Motivo(), "autorizzazione da sistemare: 7120010 · STEP autorizzato 7120010_") || !strings.Contains(g.Motivo(), "conflitto") {
		t.Errorf("il conflitto ferma il gate: %s", g.Motivo())
	}
	if !strings.Contains(g.Motivo(), "1 figli diretti, 1 relazioni") {
		t.Errorf("i file in conflitto non danno autorita': %s", g.Motivo())
	}
}

// Prova 128 (P13): i ritrovati per codice vanno visti, e gli archi entrano solo a richiesta. Nell'autorita' del
// file del prodotto il nodo 7120010 ha il codice di un componente che c'e': l'editor lo disegna come quel
// componente, e la conferma lo accetta solo se l'editor l'ha mostrato come ritrovato. L'arco proposto verso
// 7120012 non entra finche' la struttura voluta non lo contiene.
func TestIRitrovatiVannoVistiEGliArchiEntranoSoloARichiesta(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	ritrovato := b.componente("7120010", db.TipoComponenteSottoassieme)
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("f3", 32))
	b.autorizza(prodotto, a, casoGuida().json())
	v := fascicolo.StrutturaVoluta{Radice: prodotto,
		Archi:          []fascicolo.ArcoVoluto{{Padre: "c:" + prodotto.String(), Figlio: "c:" + ritrovato.String(), Qta: 1}},
		RelazioniViste: []fascicolo.RelazioneVista{{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"}, {Allegato: a.AllegatoID, Padre: "#1", Figlio: "#3"}}}
	applica := func(v fascicolo.StrutturaVoluta) (string, error) {
		return b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
		})
	}
	_, err := applica(v)
	deveRifiutare(t, err, "l'editor non l'ha mostrato come ritrovato")
	v.RitrovatiVisti = []uuid.UUID{b.proposta("#2")}
	msg, err := applica(v)
	ok(t, err)
	if !strings.Contains(msg, "1 nodo proposto ritrovato nella BOM") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT stato || ':' || (componente_id = $2)::text || ':' || (deciso_da IS NOT NULL)::text FROM componente_proposta
		WHERE thread_id = $1 AND chiave = '#2'`, b.thread, ritrovato); got != "duplicato:true:true" {
		t.Errorf("il ritrovato accettato dalla persona: %s", got)
	}
	// l'arco verso 7120012 l'editor lo mostrava, ma la struttura non lo voleva: si chiude con il motivo, e
	// 7120012 non nasce
	if got := b.relazioniProposte(); !strings.Contains(got, "#1>#2*1:confermata") || !strings.Contains(got, "#1>#3*2:scartata(tolta nella struttura confermata)") {
		t.Errorf("relazioni = %q", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '7120012'`, b.thread); n != 0 {
		t.Errorf("7120012 e' nato senza che l'operatore lo mettesse nella struttura")
	}
	// e la marcatura della sorgente si legge: e' una dichiarazione valida, della forma dello Smistamento
	ok(t, b.tx(func(q *db.Queries) error {
		d, err := fascicolo.LeggiDichiarazioni(b.ctx, q, b.thread)
		if x, ok := d.Di(prodotto); !ok || x.Origine != fascicolo.OrigineSmistamento || x.Sorgenti["#1"] != fascicolo.RuoloRadice {
			t.Errorf("la dichiarazione del prodotto: %+v", d)
		}
		var m fascicolo.Marcatura
		_ = json.Unmarshal(uno[[]byte](b, `SELECT evidenza -> 'strutturale' FROM componente_proposta WHERE thread_id = $1 AND chiave = '#1'`, b.thread), &m)
		if m.Ruolo != fascicolo.RuoloRadice || m.ComponenteID.UUID != prodotto {
			t.Errorf("la marcatura: %+v", m)
		}
		return err
	}))
}

// Prova 210 (parte; decisioni del 27/09 ter, Domanda 5 = B; precisazione dell'utente del 27/09 sera): una
// dichiarazione sospesa e' un avviso, non ferma il gate, e non si riattiva da sola. Lo STEP di 7120010,
// autorizzato per 7120010, propone di togliere 7120010 → 7120098. Portato 7120010 a commerciale, la
// dichiarazione e' sospesa: il ricalcolo chiude la sua rimozione con la nota, il gate non conta ne' i figli
// diretti del file ne' la rimozione (anche se una rimozione restasse aperta), la dice fra gli avvisi, e la
// rimozione non si accetta. Con la sospensione registrata nella marcatura (come la scrivera' la fase T),
// riportato 7120010 a sottoassieme la dichiarazione resta sospesa e la rimozione chiusa; tolta la
// sospensione (la riattivazione esplicita di una persona, F5b/T), la dichiarazione torna valida e la
// rimozione si riapre da sola (E33).
//
// Riscritta per lo Smistamento (fase T): prima fissava che «Modifica» portasse 7120010 a commerciale senza
// scrivere niente nella marcatura (la dichiarazione era sospesa solo perche' il tipo era commerciale), e la
// sospensione registrata e la riattivazione erano scritte a mano nel database. Adesso il cambio di tipo e' il
// gesto della fase T (CambiaTipoComponente: «Modifica» il tipo non lo cambia piu'), che registra la sospensione
// nella marcatura (chi, «è diventato commerciale»), e la riattivazione e' il gesto RiattivaStrutturale, con
// l'anteprima e la firma.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che il cambio di tipo portasse a commerciale 7120010, che
// nella working ha il figlio 7120098. Con la regola della PR #7, confermata dall'utente il 29/09 sera (domanda 6a:
// il commerciale e' sempre una foglia), quel passaggio si rifiuta e niente cambia. Un commerciale con un figlio
// resta possibile nei dati di prima (fino alla PR #7 il passaggio si faceva, e i figli restavano), ed e' proprio
// il caso di questa prova: l'arco si toglie per il gesto e si rimette con una scrittura diretta, come i dati che
// una RFQ puo' avere; il resto della prova e' com'era. Asserzioni (chiamate t.Error, t.Fatal, deveRifiutare,
// ok): prima 24, dopo 28.
func TestUnaDichiarazioneSospesaNonFermaIlGateENonSiRiattivaDaSola(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	pa := b.allegatoStep("7120001A_1.stp", strings.Repeat("f4", 32))
	b.autorizza(prodotto, pa, casoGuida().json())
	b.accetta("#2")
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: pa.AllegatoID, Padre: "#1", Figlio: "#2"}, b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	sotto := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '7120010'`, b.thread)
	vecchio := b.componente("7120098", db.TipoComponenteSciolto)
	b.arco(sotto, vecchio, 1)
	sa := b.allegatoStep("7120010.stp", strings.Repeat("a4", 32))
	g := fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120099"}, archi: []string{"#1>#2*2", "#1>#3"}}
	b.fattiCorrenti(sa.Sha256.String, g.json())
	b.applica(sa, g.json())
	testutil.AutorizzaStep(t, b.p, b.thread, sotto, sa.AllegatoID, b.utente)
	b.applica(sa, g.json())
	figlio := func(k string) uuid.UUID {
		return uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`, b.thread, sa.AllegatoID, k)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, figlio("#2"), b.utente, "")
	}); err != nil {
		t.Fatal(err)
	}
	// il figlio diretto #3 ancora aperto sospende le rimozioni; scartato da una persona, si calcolano
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ScartaNodo(b.ctx, q, b.thread, figlio("#3"), b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	rimozione := func() string {
		return uno[string](b, `SELECT r.stato || ':' || coalesce(r.nota, '') FROM rimozione_proposta r
			WHERE r.thread_id = $1 AND r.padre_id = $2 AND r.figlio_id = $3`, b.thread, sotto, vecchio)
	}
	if got := rimozione(); got != "aperta:" {
		t.Fatalf("la rimozione di 7120010 → 7120098: %q", got)
	}
	// del file del prodotto restano da decidere il figlio diretto 7120012 e il suo arco: sono dell'autorita' di
	// 7120001, e restano; il file di 7120010 aggiunge la rimozione e i suoi archi da decidere
	soloProdotto := db.GateStrutturaleRow{NFigliDaDecidere: 1, NArchiDaDecidere: 1}
	if s := b.strutturali(); s.NRimozioni != 1 || s.NFigliDaDecidere != 1 || s.NArchiDaDecidere <= 1 {
		t.Fatalf("prima: il gate conta la rimozione e gli archi da decidere del file di 7120010: %+v", s)
	}
	gate := func() fascicolo.Gate {
		var gg fascicolo.Gate
		ok(t, b.tx(func(q *db.Queries) (err error) { gg, err = fascicolo.LeggiGate(b.ctx, q, b.thread); return err }))
		return gg
	}
	tipo := func(tp db.TipoComponente) {
		t.Helper()
		if _, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.CambiaTipoComponente(b.ctx, q, b.thread, sotto, tp, b.utente)
		}); err != nil {
			t.Fatal(err)
		}
	}
	dichiarazione := func() string {
		var s string
		ok(t, b.tx(func(q *db.Queries) error {
			d, err := fascicolo.LeggiDichiarazioni(b.ctx, q, b.thread)
			_, valida := d.Di(sotto)
			s = fmt.Sprintf("%v:%d:%d", valida, len(d.Sospese), len(d.DaSistemare))
			return err
		}))
		return s
	}

	// con il figlio 7120098 nella working il passaggio a commerciale si rifiuta, e niente cambia
	_, errTipo := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CambiaTipoComponente(b.ctx, q, b.thread, sotto, db.TipoComponenteCommerciale, b.utente)
	})
	deveRifiutare(t, errTipo, "7120010 ha 1 figlio: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi")
	if got := dichiarazione(); got != "true:0:0" {
		t.Errorf("rifiutato il passaggio, la dichiarazione: %s", got)
	}
	if got := rimozione(); got != "aperta:" {
		t.Errorf("rifiutato il passaggio, la rimozione: %q", got)
	}
	if got := b.bom(); !strings.Contains(got, "7120010>7120098*1") || !strings.Contains(got, "7120010:sottoassieme") {
		t.Errorf("rifiutato il passaggio, la BOM: %s", got)
	}
	// la working di prima: il commerciale con il suo figlio
	b.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, sotto, vecchio)
	tipo(db.TipoComponenteCommerciale)
	b.arco(sotto, vecchio, 1)
	if got := dichiarazione(); got != "false:1:0" {
		t.Errorf("7120010 commerciale: la dichiarazione %s, attesa sospesa e non da sistemare", got)
	}
	if got := rimozione(); !strings.HasPrefix(got, "scartata:l'autorizzazione non vale: sospesa: 7120010: è diventato commerciale") {
		t.Errorf("il ricalcolo chiude la rimozione della dichiarazione sospesa: %q", got)
	}
	gg := gate()
	if !strings.Contains(gg.Motivo(), "(1 figli diretti, 1 relazioni, 0 rimozioni)") || strings.Contains(gg.Motivo(), "7120010") {
		t.Errorf("la dichiarazione sospesa non ferma il gate (restano solo le decisioni del file del prodotto): %s", gg.Motivo())
	}
	if av := strings.Join(gg.Avvisi, " | "); !strings.Contains(av, "autorizzazione sospesa: 7120010 · STEP autorizzato 7120010.stp: sospesa") {
		t.Errorf("la dichiarazione sospesa e' un avviso: %s", av)
	}
	// anche una rimozione rimasta aperta (per esempio prima di un ricalcolo) non conta e non si accetta
	b.esegui(`UPDATE rimozione_proposta SET stato = 'aperta', nota = NULL, deciso_il = NULL WHERE thread_id = $1 AND padre_id = $2`, b.thread, sotto)
	if s := b.strutturali(); s != soloProdotto {
		t.Errorf("il gate non conta niente del file sospeso: %+v", s)
	}
	docSotto := uno[uuid.UUID](b, `SELECT documento_id FROM documento WHERE thread_id = $1 AND sha256 = $2`, b.thread, sa.Sha256.String)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRimozione(b.ctx, q, b.thread, fascicolo.ChiaveRimozione{Step: docSotto, Padre: sotto, Figlio: vecchio}, b.utente)
	})
	deveRifiutare(t, err, "la rimozione non si decide, sospesa: 7120010: è diventato commerciale")
	if got := b.bom(); !strings.Contains(got, "7120010>7120098*1") {
		t.Errorf("l'arco resta nella working: %s", got)
	}

	// la sospensione e' registrata nella marcatura dal cambio di tipo: riportato a sottoassieme, resta sospesa
	if got := uno[string](b, `SELECT concat_ws('|', evidenza -> 'strutturale' -> 'sospesa' ->> 'motivo', evidenza -> 'strutturale' -> 'sospesa' ->> 'tipo',
		evidenza -> 'strutturale' -> 'sospesa' ->> 'da') FROM componente_proposta WHERE thread_id = $1 AND componente_id = $2 AND evidenza ? 'strutturale'`,
		b.thread, sotto); got != "è diventato commerciale|commerciale|"+b.utente.String() {
		t.Errorf("la sospensione registrata dal cambio di tipo: %s", got)
	}
	tipo(db.TipoComponenteSottoassieme)
	if got := dichiarazione(); got != "false:1:0" {
		t.Errorf("tornato sottoassieme con la sospensione registrata: %s, attesa ancora sospesa", got)
	}
	if got := rimozione(); !strings.HasPrefix(got, "scartata:l'autorizzazione non vale: sospesa: 7120010: è diventato commerciale") {
		t.Errorf("la rimozione resta chiusa: %q", got)
	}
	if s := b.strutturali(); s != soloProdotto {
		t.Errorf("il gate non conta niente del file sospeso: %+v", s)
	}
	// la riattivazione esplicita di una persona (il gesto della fase T, con la firma dell'anteprima): tolta la
	// sospensione, e' valida e la rimozione si riapre
	er, err := fascicolo.EffettoRiattivazioneDi(b.ctx, db.New(b.p), b.thread, sotto, sa.Sha256.String)
	ok(t, err)
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.RiattivaStrutturale(b.ctx, q, b.thread, b.utente, sotto, sa.Sha256.String, er.Firma)
	}); err != nil {
		t.Fatal(err)
	}
	if got := dichiarazione(); got != "true:0:0" {
		t.Errorf("riattivata: %s", got)
	}
	if got := rimozione(); got != "aperta:" {
		t.Errorf("riattivata, la rimozione si riapre (E33): %q", got)
	}
	if s := b.strutturali(); s.NRimozioni != 1 {
		t.Errorf("riattivata, il gate conta di nuovo la rimozione: %+v", s)
	}
}

// Nessuna accettazione fa nascere un commerciale (decisioni del 27/09 ter, make/buy): il tipo commerciale lo
// sceglie solo una persona. «Accetta il file» su uno STEP autorizzato e la struttura confermata dall'editor fanno
// nascere i figli diretti come sottoassieme o sciolto, anche quando una riga di prima proponeva «commerciale»
// (qui scritto a mano in tipo_proposto). Nessun componente commerciale nella RFQ, alla fine.
func TestNessunaAccettazioneFaNascereUnCommerciale(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("a5", 32))
	b.autorizza(prodotto, a, casoGuida().json())
	commerciali := func() int {
		return uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND tipo = 'commerciale'`, b.thread)
	}
	proponiCommerciale := func(allegato uuid.UUID, chiavi ...string) {
		for _, k := range chiavi {
			b.esegui(`UPDATE componente_proposta SET tipo_proposto = 'commerciale' WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`,
				b.thread, allegato, k)
		}
	}
	proponiCommerciale(a.AllegatoID, "#3")
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	if got := uno[string](b, `SELECT string_agg(codice || ':' || tipo, ' ' ORDER BY codice) FROM componente WHERE thread_id = $1`, b.thread); got != "7120001:finito 7120010:sottoassieme 7120012:sciolto" {
		t.Errorf("accetta il file: %s", got)
	}
	c10 := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '7120010'`, b.thread)
	c12 := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '7120012'`, b.thread)

	// l'editor: lo STEP di 7120010, autorizzato, con due figli diretti proposti «commerciale» da una riga di prima
	sa := b.allegatoStep("7120010.stp", strings.Repeat("b5", 32))
	g := fattiSTEP{nodi: []string{"#1=7120010", "#2=7120099", "#3=7120098"}, archi: []string{"#1>#2", "#1>#3*2"}}
	b.autorizza(c10, sa, g.json())
	proponiCommerciale(sa.AllegatoID, "#2", "#3")
	carta := func(k string) string {
		return "p:" + uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`,
			b.thread, sa.AllegatoID, k).String()
	}
	cc := func(id uuid.UUID) string { return "c:" + id.String() }
	visti := []fascicolo.ArcoVoluto{{Padre: cc(prodotto), Figlio: cc(c10), Qta: 1}, {Padre: cc(prodotto), Figlio: cc(c12), Qta: 2}}
	v := fascicolo.StrutturaVoluta{Radice: prodotto, Visti: visti,
		Archi:          append(append([]fascicolo.ArcoVoluto{}, visti...), fascicolo.ArcoVoluto{Padre: cc(c10), Figlio: carta("#2"), Qta: 1}, fascicolo.ArcoVoluto{Padre: cc(c10), Figlio: carta("#3"), Qta: 2}),
		RelazioniViste: []fascicolo.RelazioneVista{{Allegato: sa.AllegatoID, Padre: "#1", Figlio: "#2"}, {Allegato: sa.AllegatoID, Padre: "#1", Figlio: "#3"}}}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	}); err != nil {
		t.Fatal(err)
	}
	if got := uno[string](b, `SELECT string_agg(codice || ':' || tipo, ' ' ORDER BY codice) FROM componente WHERE thread_id = $1 AND codice IN ('7120098', '7120099')`, b.thread); got != "7120098:sciolto 7120099:sciolto" {
		t.Errorf("la struttura confermata dall'editor: %s", got)
	}
	if n := commerciali(); n != 0 {
		t.Errorf("%d componenti commerciali nati da un'accettazione", n)
	}
}

// Una carta dell'editor decide solo le proposte del suo file (A5.4.7: le riconciliazioni no). Gli STEP di 7120010
// e di 7120012, autorizzati, hanno tutti e due il figlio diretto 7120099. La struttura confermata mette sotto
// 7120010 la carta del suo file: 7120099 nasce da quella proposta, e la proposta dell'altro file resta aperta,
// senza componente, finche' una persona non la decide dove la vede. Prima la carta prendeva tutte le proposte
// aperte con lo stesso codice, e la seconda diventava «duplicato» di 7120099 per uguaglianza di codice.
func TestUnaCartaDellEditorDecideSoloIlSuoFile(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	prodotto := b.componente("7120001", db.TipoComponenteFinito)
	c10 := b.componente("7120010", db.TipoComponenteSottoassieme)
	c12 := b.componente("7120012", db.TipoComponenteSottoassieme)
	b.arco(prodotto, c10, 1)
	b.arco(prodotto, c12, 1)
	s10 := b.allegatoStep("7120010.stp", strings.Repeat("c5", 32))
	s12 := b.allegatoStep("7120012.stp", strings.Repeat("d5", 32))
	b.autorizza(c10, s10, fattiSTEP{nodi: []string{"#1=7120010", "#2=7120099"}, archi: []string{"#1>#2"}}.json())
	b.autorizza(c12, s12, fattiSTEP{nodi: []string{"#1=7120012", "#2=7120099"}, archi: []string{"#1>#2*4"}}.json())
	proposta := func(a db.Allegato) uuid.UUID {
		return uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`, b.thread, a.AllegatoID)
	}
	cc := func(id uuid.UUID) string { return "c:" + id.String() }
	visti := []fascicolo.ArcoVoluto{{Padre: cc(prodotto), Figlio: cc(c10), Qta: 1}, {Padre: cc(prodotto), Figlio: cc(c12), Qta: 1}}
	v := fascicolo.StrutturaVoluta{Radice: prodotto, Visti: visti,
		Archi:          append(append([]fascicolo.ArcoVoluto{}, visti...), fascicolo.ArcoVoluto{Padre: cc(c10), Figlio: "p:" + proposta(s10).String(), Qta: 1}),
		RelazioniViste: []fascicolo.RelazioneVista{{Allegato: s10.AllegatoID, Padre: "#1", Figlio: "#2"}}}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	}); err != nil {
		t.Fatal(err)
	}
	stato := func(a db.Allegato) string {
		return uno[string](b, `SELECT stato || ':' || coalesce(componente_id::text, 'senza componente') || ':' || (deciso_da IS NOT NULL)::text
			FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`, b.thread, a.AllegatoID)
	}
	nato := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '7120099'`, b.thread)
	if got := stato(s10); got != "confermata:"+nato.String()+":true" {
		t.Errorf("la carta del file di 7120010: %s", got)
	}
	if got := stato(s12); got != "aperta:senza componente:false" {
		t.Errorf("la proposta dell'altro file resta aperta, senza componente: %s", got)
	}
	if got := b.bom(); strings.Contains(got, "7120012>7120099") || !strings.Contains(got, "7120010>7120099*1") {
		t.Errorf("la working: %s", got)
	}
}
