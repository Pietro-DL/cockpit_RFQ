//go:build integrazione

// L4 — Smistamento F5b (addendum A5.4.2, A5.4.6, A5.4.7; decisioni del 27/09 ter, Domanda 1 = B e Domanda 5 = B):
// autorizzare uno STEP per qualunque componente non commerciale, con l'anteprima e la firma di quello che si e'
// visto; la delega esplicita dello stesso file per un componente annidato, livello per livello; la revoca, con la
// storia; la sostituzione con una revisione. Contro PostgreSQL vero.
//
// Prove dell'addendum: 86, 95 (parte casella, nel core), 112, 130, 203–209, 221; e quelle chieste per F5b: lo STEP
// proprio di 7120010 rende autorita' 7120011 e non i suoi figli, la delega di 7120010 nello STEP del prodotto
// rende autorita' 7120011 e non i nipoti, la revoca del padre revoca la delega, un componente un'autorizzazione
// fra la delega e lo STEP proprio, il commerciale rifiutato nell'anteprima e nella scrittura. Le prove della
// scheda del componente (111, 113, 95 parte pagina) stanno in web/autorizzazione_db_test.go.

package fascicolo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// effetto e' l'anteprima come la chiede la scheda: una GET, fuori da una transazione.
func (b *banco) effetto(r fascicolo.RichiestaAutorizzazione) fascicolo.Effetto {
	b.t.Helper()
	e, err := fascicolo.EffettoAutorizzazione(b.ctx, db.New(b.p), b.thread, r)
	if err != nil {
		b.t.Fatalf("anteprima: %v", err)
	}
	return e
}

// dichiara e' il gesto di F5b come lo fa la scheda: l'anteprima, poi la scrittura con la firma vista, la
// casella spuntata dalla persona e, se l'anteprima la chiede, la presa d'atto. Raggruppamenti e deleghe sono
// quelli della richiesta.
func (b *banco) dichiara(r fascicolo.RichiestaAutorizzazione) (string, error) {
	b.t.Helper()
	e := b.effetto(r)
	r.Firma, r.Autorizza, r.PresaDAtto = e.Firma, true, e.PresaDAtto != ""
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, r)
	})
}

// dichiaraOk e' dichiara che deve riuscire.
func (b *banco) dichiaraOk(r fascicolo.RichiestaAutorizzazione) string {
	b.t.Helper()
	msg, err := b.dichiara(r)
	if err != nil {
		b.t.Fatalf("autorizzazione: %v", err)
	}
	return msg
}

func (b *banco) revoca(comp uuid.UUID, sha string) (string, error) {
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.RevocaStrutturale(b.ctx, q, b.thread, b.utente, comp, sha, "prova")
	})
}

// nodoIn e' la proposta del nodo chiave nelle righe dell'allegato.
func (b *banco) nodoIn(a db.Allegato, chiave string) uuid.UUID {
	b.t.Helper()
	return uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`,
		b.thread, a.AllegatoID, chiave)
}

// accettaIn accetta i nodi del file, come una persona; err e' il primo rifiuto.
func (b *banco) accettaIn(a db.Allegato, chiavi ...string) error {
	b.t.Helper()
	for _, k := range chiavi {
		if _, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.nodoIn(a, k), b.utente, "")
		}); err != nil {
			return err
		}
	}
	return nil
}

func (b *banco) accettaArco(a db.Allegato, p, f string) (string, error) {
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: p, Figlio: f}, b.utente)
	})
}

// autoritaOra e' l'autorita' della RFQ adesso, com'e' letta dall'editor e dalle accettazioni.
func (b *banco) autoritaOra() fascicolo.Autorita {
	b.t.Helper()
	a, _, _, err := fascicolo.LeggiAutorita(b.ctx, db.New(b.p), b.thread)
	if err != nil {
		b.t.Fatal(err)
	}
	return a
}

// impronta delle tabelle che il gesto scrive: servono a dire che l'anteprima non scrive niente.
func (b *banco) improntaAutorizzazione() string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM componente_proposta x WHERE thread_id = $1),
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM relazione_proposta x WHERE thread_id = $1),
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM rimozione_proposta x WHERE thread_id = $1),
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM componente x WHERE thread_id = $1),
		(SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM documento x WHERE thread_id = $1))`, b.thread)
}

// statoNodo: stato, componente deciso (codice) e chi, del nodo chiave dell'allegato: «duplicato 7120001 persona».
func (b *banco) statoNodo(a db.Allegato, chiave string) string {
	b.t.Helper()
	return uno[string](b, `SELECT cp.stato::text || ' ' || coalesce(c.codice, '-') || ' ' || CASE WHEN cp.deciso_da IS NULL THEN '-' ELSE 'persona' END ||
		CASE WHEN cp.evidenza ? 'strutturale' THEN ' marcato:' || (cp.evidenza -> 'strutturale' ->> 'ruolo') ELSE '' END
		FROM componente_proposta cp LEFT JOIN componente c ON c.componente_id = cp.componente_id
		WHERE cp.thread_id = $1 AND cp.allegato_id = $2 AND cp.chiave = $3`, b.thread, a.AllegatoID, chiave)
}

// storiaNodo: gli eventi della storia del nodo, in ordine.
func (b *banco) storiaNodo(a db.Allegato, chiave string) string {
	b.t.Helper()
	return uno[string](b, `SELECT coalesce(string_agg(e ->> 'evento', ' '), '') FROM componente_proposta cp,
		jsonb_array_elements(coalesce(cp.evidenza -> 'storia', '[]'::jsonb)) e
		WHERE cp.thread_id = $1 AND cp.allegato_id = $2 AND cp.chiave = $3`, b.thread, a.AllegatoID, chiave)
}

// casoProfondo: lo STEP completo del prodotto: la radice, che il file chiama 7120901 (non il codice della
// richiesta: la presa d'atto), i figli diretti 7120010 e 7120012 ×2; sotto 7120010 c'e' 7120011 ×2, sotto
// 7120011 c'e' 7120013.
func casoProfondo() fattiSTEP {
	return fattiSTEP{nodi: []string{"#1=7120901", "#2=7120010", "#3=7120012", "#4=7120011", "#5=7120013"},
		archi: []string{"#1>#2", "#1>#3*2", "#2>#4*2", "#4>#5"}}
}

// prodottoProfondo: la RFQ ACME con il prodotto 7120001 e il suo STEP completo, associato e analizzato.
func (b *banco) prodottoProfondo() (prodotto, doc uuid.UUID, a db.Allegato) {
	b.t.Helper()
	b.acme()
	prodotto = b.componente("7120001", db.TipoComponenteFinito)
	doc, a = b.stepDelProdotto(prodotto, "7120001", casoProfondo())
	return prodotto, doc, a
}

// Prova 86 (A5.4.7, r1 §3.9, P6): l'anteprima dell'autorizzazione sul caso guida. Il file chiama la radice
// 7120901: per la RFQ e' 7120001, con la presa d'atto. Entrano i figli diretti (7120010, 7120012 ×2), gli altri
// nodi restano guida («sotto 7120010: 2»); le rimozioni aspettano i figli decisi; per un finito il file diventa
// anche lo STEP strutturale. La delega di 7120010 e' spenta finche' il nodo non e' deciso; deciso, si puo' dare.
// L'anteprima non scrive niente (F1). La scrittura vuole la casella, la presa d'atto e la firma di quello che si
// e' visto: senza una delle tre si rifiuta e non cambia niente.
func TestLAnteprimaDellAutorizzazione(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	prima := b.improntaAutorizzazione()
	r := fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc}
	e := b.effetto(r)
	if b.improntaAutorizzazione() != prima {
		t.Fatal("l'anteprima ha scritto")
	}
	if e.Spento != "" || e.Sorgente == nil || e.Sorgente.Chiave != "#1" || !e.Autorizzabile() {
		t.Fatalf("l'anteprima di un file autorizzabile: %+v", e)
	}
	if e.PresaDAtto != "il file chiama la radice «7120901»: per questa RFQ è 7120001" {
		t.Errorf("presa d'atto: %q", e.PresaDAtto)
	}
	var figli []string
	for _, f := range e.Figli {
		figli = append(figli, fmt.Sprintf("%sx%d", f.Nome, f.Qta))
	}
	if strings.Join(figli, " ") != "7120010x1 7120012x2" {
		t.Errorf("figli diretti: %v", figli)
	}
	if e.Guida != 2 || strings.Join(e.GuidaSotto, "|") != "sotto 7120010: 2" {
		t.Errorf("guida: %d %v", e.Guida, e.GuidaSotto)
	}
	if !strings.Contains(e.RimozioniSospese, "2 figli diretti sono decisi da una persona") || len(e.Rimozioni) != 0 {
		t.Errorf("rimozioni: %q %v", e.RimozioniSospese, e.Rimozioni)
	}
	if len(e.Deleghe) != 1 || e.Deleghe[0].Nodo.Nome != "7120010" || !strings.Contains(e.Deleghe[0].Spenta, "non è ancora deciso") {
		t.Errorf("la delega di 7120010, spenta finche' il nodo non e' deciso: %+v", e.Deleghe)
	}
	if !strings.Contains(strings.Join(e.Avvisi, "|"), "diventa anche il suo STEP strutturale") {
		t.Errorf("avvisi: %v", e.Avvisi)
	}
	if e.Firma == "" || e.Firma != b.effetto(r).Firma {
		t.Errorf("la firma e' stabile: %q", e.Firma)
	}
	if !strings.HasPrefix(e.Bottone(), "Autorizza file") || !strings.HasSuffix(e.Bottone(), "per 7120001: 2 figli diretti") {
		t.Errorf("il bottone nomina l'effetto: %q", e.Bottone())
	}

	// la scrittura: senza la casella, senza la presa d'atto, con una firma vecchia, non cambia niente
	rifiuti := []struct {
		nome, frase string
		r           fascicolo.RichiestaAutorizzazione
	}{
		{"senza la casella", "la casella non nasce spuntata", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Firma: e.Firma, PresaDAtto: true}},
		{"senza la presa d'atto", "serve la presa d'atto", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Firma: e.Firma, Autorizza: true}},
		{"con una firma vecchia", "riapri l'anteprima", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Firma: "deadbeef", Autorizza: true, PresaDAtto: true}},
		{"senza firma", "riapri l'anteprima", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Autorizza: true, PresaDAtto: true}},
	}
	for _, x := range rifiuti {
		_, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, x.r)
		})
		deveRifiutare(t, err, x.frase)
	}
	if b.improntaAutorizzazione() != prima {
		t.Fatal("i rifiuti hanno scritto")
	}
	// una decisione nel frattempo cambia l'effetto, e la firma vista non vale piu'
	b.applica(a, casoProfondo().json())
	b.esegui(`UPDATE componente_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now() WHERE proposta_id = $1`, b.nodoIn(a, "#3"), b.utente)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc,
			Firma: e.Firma, Autorizza: true, PresaDAtto: true})
	})
	deveRifiutare(t, err, "è cambiato mentre lo guardavi")
	b.esegui(`UPDATE componente_proposta SET stato = 'aperta', deciso_da = NULL, deciso_il = NULL WHERE proposta_id = $1`, b.nodoIn(a, "#3"))

	msg := b.dichiaraOk(r)
	if !strings.Contains(msg, "è lo STEP autorizzato di 7120001: 2 figli diretti nell'autorità, 2 nodi restano guida") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.statoNodo(a, "#1"); got != "duplicato 7120001 persona marcato:radice" {
		t.Errorf("la radice dopo l'autorizzazione: %s", got)
	}
	presa := uno[string](b, `SELECT evidenza -> 'strutturale' ->> 'presa_d_atto' FROM componente_proposta WHERE proposta_id = $1`, b.nodoIn(a, "#1"))
	if presa != e.PresaDAtto {
		t.Errorf("la presa d'atto nella marcatura: %q", presa)
	}
	// deciso 7120010, la sua delega si puo' dare; l'anteprima del file gia' autorizzato lo dice
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	e = b.effetto(r)
	if e.Gia == "" || len(e.Deleghe) != 1 || e.Deleghe[0].Spenta != "" || e.Deleghe[0].Codice != "7120010" {
		t.Errorf("gia' autorizzato, la delega di 7120010 si puo' dare: %q %+v", e.Gia, e.Deleghe)
	}
}

// Prove 206 e 130 (A5.4.2): un finito autorizzato scrive anche lo STEP strutturale, nella stessa transazione,
// e la dichiarazione e' valida (marcatura e colonna dicono lo stesso file, I10); anche su un documento nato
// nella stessa transazione. Un sottoassieme autorizzato non scrive la colonna (il CHECK la vuole solo per un
// finito) e la sua autorizzazione vale lo stesso.
func TestUnFinitoAutorizzatoScriveAncheLoStepStrutturale(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, _ := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if got := uno[string](b, `SELECT coalesce(step_strutturale_id::text, '-') FROM componente WHERE componente_id = $1`, prodotto); got != doc.String() {
		t.Errorf("step_strutturale_id: %s", got)
	}
	if got := uno[string](b, `SELECT esito FROM v_step_prodotto WHERE componente_id = $1`, prodotto); got != "presente_analizzato" {
		t.Errorf("v_step_prodotto: %s", got)
	}
	d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if x, trovata := d.Di(prodotto); !trovata || x.Origine != fascicolo.OrigineSmistamento || x.Documento.UUID != doc {
		t.Errorf("la dichiarazione del finito: %+v", d)
	}

	// il sottoassieme: la marcatura sola
	s := b.componente("7120010", db.TipoComponenteSottoassieme)
	sdoc, _ := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}})
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: sdoc})
	if got := uno[bool](b, `SELECT step_strutturale_id IS NULL FROM componente WHERE componente_id = $1`, s); !got {
		t.Error("un sottoassieme non ha lo STEP strutturale")
	}
	d, err = fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if _, trovata := d.Di(s); !trovata {
		t.Errorf("l'autorizzazione del sottoassieme vale: %+v", d.DelComponente(s))
	}

	// prova 130: il documento nasce nella stessa transazione dell'autorizzazione (senza 0022)
	b2 := nuovoBanco(t)
	b2.acme()
	p2 := b2.componente("7120001", db.TipoComponenteFinito)
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010"}, archi: []string{"#1>#2"}}
	sha := strings.Repeat("e3", 32)
	a2 := b2.allegatoStep("7120001.stp", sha)
	b2.fattiCorrenti(sha, f.json())
	var nuovo uuid.UUID
	ok(t, b2.tx(func(q *db.Queries) error {
		d, err := q.InsertDocumento(b2.ctx, db.InsertDocumentoParams{ThreadID: b2.thread, ComponenteID: uuid.NullUUID{UUID: p2, Valid: true},
			Tipo: db.TipoDocumentoCad3d, Codice: pgtype.Text{String: "7120001", Valid: true}, NomeFile: a2.NomeFile, Estensione: "stp",
			Sha256: sha, PathRelativo: `ELENCO DISEGNI\7120001\7120001.stp`, ConfermatoDa: b2.utente})
		if err != nil {
			return err
		}
		nuovo = d.DocumentoID
		r := fascicolo.RichiestaAutorizzazione{Componente: p2, Documento: nuovo}
		e, err := fascicolo.EffettoAutorizzazione(b2.ctx, q, b2.thread, r)
		if err != nil {
			return err
		}
		r.Firma, r.Autorizza = e.Firma, true
		_, err = fascicolo.DichiaraStrutturale(b2.ctx, q, b2.thread, b2.utente, r)
		return err
	}))
	if got := uno[string](b2, `SELECT coalesce(step_strutturale_id::text, '-') FROM componente WHERE componente_id = $1`, p2); got != nuovo.String() {
		t.Errorf("il documento appena nato e' lo STEP strutturale: %s", got)
	}
	if got := b2.statoNodo(a2, "#1"); got != "duplicato 7120001 persona marcato:radice" {
		t.Errorf("la radice del documento appena nato: %s", got)
	}
}

// Prova 205 (A5.4.7 passo 3): il file si autorizza solo se ha un documento corrente associato al componente, e'
// uno STEP, viene da una fonte del cliente e ha un'analisi corrente. Altrimenti il riquadro e' spento e dice
// perche'. Con piu' radici nessuna e' scelta: la persona indica il nodo, e senza la scrittura si rifiuta.
func TestLAutorizzazioneVuoleIlFileAssociatoAlComponente(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, _ := b.prodottoProfondo()
	altro := b.componente("7120050", db.TipoComponenteSottoassieme)
	disegno := b.documento(prodotto, db.TipoDocumentoDisegno2d, "7120001", "pdf")
	senzaAnalisi := b.documento(prodotto, db.TipoDocumentoCad3d, "7120001", "stp")
	b.allegatoStep("senza.stp", b.sha(senzaAnalisi))
	uscita, au := b.stepDelProdotto(prodotto, "7120001", casoProfondo())
	b.esegui(`UPDATE messaggio SET direzione = 'uscita' WHERE messaggio_id = $1`, au.MessaggioID)
	casi := []struct {
		nome   string
		r      fascicolo.RichiestaAutorizzazione
		spento string
	}{
		{"nessuno STEP scelto", fascicolo.RichiestaAutorizzazione{Componente: prodotto}, "si sceglie lo STEP"},
		{"il documento di un altro componente", fascicolo.RichiestaAutorizzazione{Componente: altro, Documento: doc}, "non è associato a 7120050"},
		{"un 2D", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: disegno}, "non è un file STEP"},
		{"senza analisi", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: senzaAnalisi}, "non ha ancora un'analisi corrente"},
		{"da una nostra mail in uscita", fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: uscita}, "non viene da una fonte del cliente"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := b.effetto(c.r)
			if !strings.Contains(e.Spento, c.spento) || e.Autorizzabile() {
				t.Errorf("spento: %q, atteso %q", e.Spento, c.spento)
			}
			c.r.Firma, c.r.Autorizza, c.r.PresaDAtto = e.Firma, true, true
			_, err := b.gesto(func(q *db.Queries) (string, error) {
				return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, c.r)
			})
			deveRifiutare(t, err, c.spento)
		})
	}
	// sostituito
	s2 := b.documento(prodotto, db.TipoDocumentoCad3d, "7120001", "stp")
	b.esegui(`UPDATE documento SET sostituito_da = $2 WHERE documento_id = $1`, doc, s2)
	if e := b.effetto(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc}); !strings.Contains(e.Spento, "è stato sostituito") {
		t.Errorf("sostituito: %q", e.Spento)
	}
	b.esegui(`UPDATE documento SET sostituito_da = NULL WHERE documento_id = $1`, doc)

	// piu' radici: nessuna scelta di partenza
	due, _ := b.stepDelProdotto(prodotto, "7120001", fattiSTEP{nodi: []string{"#1=7120001", "#2=7120002", "#3=7120010"}, archi: []string{"#1>#3"}})
	r := fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: due}
	e := b.effetto(r)
	if e.Spento != "" || e.Sorgente != nil || len(e.Radici) != 2 || e.Autorizzabile() {
		t.Fatalf("due radici, nessuna scelta: %+v", e)
	}
	r.Firma, r.Autorizza = e.Firma, true
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, r)
	})
	deveRifiutare(t, err, "ha 2 radici: si indica quale nodo è 7120001")
	r = fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: due, Sorgente: "#1"}
	if e := b.effetto(r); e.Sorgente == nil || e.Sorgente.Chiave != "#1" || e.PresaDAtto != "" || len(e.Figli) != 1 {
		t.Errorf("indicata la radice #1: %+v", e)
	}
	b.dichiaraOk(r)
}

// Prova 209 (P7, A5.4.7 passo 5): la radice decisa da una persona come un altro componente ferma
// l'autorizzazione; quella chiusa da un automatismo di prima (duplicato senza chi l'ha deciso, anche come un
// altro componente) l'autorizzazione la prende, e com'era resta nella storia.
func TestLaRadiceDecisaComeAltroFermaLAutorizzazione(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	altro := b.componente("7120001A", db.TipoComponenteSottoassieme)
	b.applica(a, casoProfondo().json())
	radice := b.nodoIn(a, "#1")
	b.esegui(`UPDATE componente_proposta SET stato = 'duplicato', componente_id = $2, deciso_da = $3, deciso_il = now() WHERE proposta_id = $1`, radice, altro, b.utente)
	r := fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc}
	e := b.effetto(r)
	if e.Spento != "la radice è già il componente 7120001A: archivialo o scegli un altro file" {
		t.Errorf("P7: %q", e.Spento)
	}
	_, err := b.dichiara(r)
	deveRifiutare(t, err, "la radice è già il componente 7120001A")

	// la stessa riga, ma decisa da un automatismo di prima: si prende
	b.esegui(`UPDATE componente_proposta SET deciso_da = NULL WHERE proposta_id = $1`, radice)
	e = b.effetto(r)
	if e.Spento != "" || !strings.Contains(strings.Join(e.Avvisi, "|"), "chiuso da un automatismo di prima") {
		t.Errorf("l'aggancio di prima si prende, e l'anteprima lo dice: %q %v", e.Spento, e.Avvisi)
	}
	b.dichiaraOk(r)
	if got := b.statoNodo(a, "#1"); got != "duplicato 7120001 persona marcato:radice" {
		t.Errorf("la radice presa: %s", got)
	}
	if got := b.storiaNodo(a, "#1"); got != "preso_dall_autorizzazione" {
		t.Errorf("la storia della radice presa: %q", got)
	}
	if got := uno[string](b, `SELECT e ->> 'componente_id' FROM componente_proposta, jsonb_array_elements(evidenza -> 'storia') e WHERE proposta_id = $1`, radice); got != altro.String() {
		t.Errorf("la storia ricorda il componente di prima: %s", got)
	}
}

// Prova 204 (Domanda 5 = B): un commerciale non si autorizza, ne' come sorgente ne' come delega. L'anteprima e'
// spenta e dice che cosa fare; la scrittura si rifiuta. Nell'anteprima del padre la casella della delega per un
// figlio commerciale e' spenta, e una delega mandata lo stesso si rifiuta; dalla scheda del commerciale la
// delega si rifiuta.
func TestUnCommercialeNonSiAutorizza(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	comm := b.componente("7120099", db.TipoComponenteCommerciale)
	cdoc, _ := b.stepDelProdotto(comm, "7120099", fattiSTEP{nodi: []string{"#1=7120099", "#2=7120098"}, archi: []string{"#1>#2"}})
	r := fascicolo.RichiestaAutorizzazione{Componente: comm, Documento: cdoc}
	const frase = "7120099 è un commerciale: il suo STEP resta guida; per autorizzarlo cambia prima il tipo"
	if e := b.effetto(r); e.Spento != frase || e.Autorizzabile() {
		t.Errorf("anteprima di un commerciale: %q", e.Spento)
	}
	_, err := b.dichiara(r)
	deveRifiutare(t, err, frase)

	// il figlio 7120010 del prodotto, deciso come commerciale: la sua delega e' spenta e si rifiuta
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	c10 := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE codice = '7120010'`)
	b.esegui(`UPDATE componente SET tipo = 'commerciale' WHERE componente_id = $1`, c10)
	rp := fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Deleghe: []string{"#2"}}
	e := b.effetto(rp)
	if len(e.Deleghe) != 1 || !strings.Contains(e.Deleghe[0].Spenta, "7120010 è un commerciale") {
		t.Errorf("la delega di un commerciale nell'anteprima del padre: %+v", e.Deleghe)
	}
	_, err = b.dichiara(rp)
	deveRifiutare(t, err, "7120010 è un commerciale")
	rd := fascicolo.RichiestaAutorizzazione{Componente: c10, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#2"}}
	if e := b.effetto(rd); !strings.Contains(e.Spento, "7120010 è un commerciale") {
		t.Errorf("la delega dalla scheda del commerciale: %q", e.Spento)
	}
	_, err = b.dichiara(rd)
	deveRifiutare(t, err, "7120010 è un commerciale")
	if got := uno[int](b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND evidenza -> 'strutturale' ->> 'ruolo' = 'delega'`, b.thread); got != 0 {
		t.Errorf("%d deleghe scritte per un commerciale", got)
	}
}

// Prova 203 (A5.4.1, E37): un particolare si autorizza (l'anteprima avvisa), e al primo figlio accettato dal
// suo STEP diventa un assieme.
func TestUnParticolareAutorizzatoDiventaAssiemeAlPrimoFiglio(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	p := b.componente("7120012", db.TipoComponenteSciolto)
	doc, a := b.stepDelProdotto(p, "7120012", fattiSTEP{nodi: []string{"#1=7120012", "#2=7120021", "#3=7120022"}, archi: []string{"#1>#2", "#1>#3*4"}})
	r := fascicolo.RichiestaAutorizzazione{Componente: p, Documento: doc}
	if e := b.effetto(r); e.Spento != "" || !strings.Contains(strings.Join(e.Avvisi, "|"), "7120012 è un particolare: diventa un assieme al primo figlio accettato") {
		t.Errorf("anteprima di un particolare: %q %v", e.Spento, e.Avvisi)
	}
	b.dichiaraOk(r)
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	if got := uno[string](b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, p); got != "sciolto" {
		t.Errorf("un nodo accettato non basta, serve l'arco: %s", got)
	}
	msg, err := b.accettaArco(a, "#1", "#2")
	ok(t, err)
	if !strings.Contains(msg, "7120012 diventa un assieme") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, p); got != "sottoassieme" {
		t.Errorf("al primo figlio il particolare diventa un assieme: %s", got)
	}
}

// Le prove chieste per F5b (Domanda 1 = B): lo STEP proprio di 7120010, autorizzato per 7120010, rende autorita'
// i suoi figli diretti (7120011), che nello STEP del prodotto restavano guida; l'autorita' non scende ai figli di
// 7120011 (7120013 resta guida anche nel file di 7120010, e si rifiuta). Due padri, due file: l'arco
// 7120001 → 7120010 e' del file del prodotto, 7120010 → 7120011 del file di 7120010.
func TestLoStepProprioDelSottoassiemeRendeAutoritaIFigliDiretti(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	s := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE codice = '7120010'`)
	if _, figlio := b.autoritaOra().FiglioDiretto(a.AllegatoID, "#4"); figlio {
		t.Fatal("prima: 7120011 e' guida nello STEP del prodotto")
	}
	deveRifiutare(t, b.accettaIn(a, "#4"), "7120011 è guida")

	sdoc, sa := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120013"}, archi: []string{"#1>#2*2", "#2>#3"}})
	e := b.effetto(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: sdoc})
	if e.Spento != "" || e.PresaDAtto != "" || len(e.Figli) != 1 || e.Figli[0].Nome != "7120011" || e.Guida != 1 {
		t.Fatalf("anteprima di 7120010.stp: %+v", e)
	}
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: sdoc})
	aut := b.autoritaOra()
	if c, figlio := aut.FiglioDiretto(sa.AllegatoID, "#2"); !figlio || c != s {
		t.Errorf("7120011 e' figlio diretto di 7120010 nel suo file: %v %v", c, figlio)
	}
	if _, figlio := aut.FiglioDiretto(sa.AllegatoID, "#3"); figlio {
		t.Error("7120013, sotto 7120011, resta guida nel file di 7120010")
	}
	if _, figlio := aut.FiglioDiretto(a.AllegatoID, "#4"); figlio {
		t.Error("nel file del prodotto 7120011 resta guida: lo comanda il file di 7120010")
	}
	if err := b.accettaIn(sa, "#2"); err != nil {
		t.Fatalf("7120011 dal file di 7120010: %v", err)
	}
	deveRifiutare(t, b.accettaIn(sa, "#3"), "7120013 è guida")
	if _, err := b.accettaArco(sa, "#1", "#2"); err != nil {
		t.Fatal(err)
	}
	if got := b.bom(); !strings.Contains(got, "7120010>7120011*2") || strings.Contains(got, "7120013") {
		t.Errorf("la BOM: %s", got)
	}
}

// Prova 221 (Domanda 1 = B, A5.4.6 D3): la delega esplicita dello stesso file per un componente annidato. Nello
// STEP completo del prodotto, deciso 7120010, la casella dell'anteprima del prodotto autorizza lo stesso file
// anche per 7120010: 7120011 diventa autorita' (figlio diretto della delega) e 7120013, nipote, resta guida. La
// delega non scende da sola: quella di 7120011 si da' dalla sua scheda, sopra la delega di 7120010 (senza,
// si rifiuta). Revocare il padre revoca le deleghe, con la storia; i nodi decisi da persone restano.
func TestUnaDelegaAutorizzaIlSottoalberoDelloStepDelPadre(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	s := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE codice = '7120010'`)

	// la delega del nipote prima di quella del figlio: il nodo non e' figlio diretto di una sorgente
	n := b.componente("7120011", db.TipoComponenteSottoassieme)
	rn := fascicolo.RichiestaAutorizzazione{Componente: n, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#4"}}
	if e := b.effetto(rn); !strings.Contains(e.Spento, "non è un figlio diretto di una sorgente autorizzata") {
		t.Errorf("il nipote senza la delega del figlio: %q", e.Spento)
	}

	// la casella nell'anteprima del prodotto (gia' autorizzato: si danno le deleghe)
	msg := b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc, Deleghe: []string{"#2"}})
	if !strings.Contains(msg, "elegato anche a 7120010") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.statoNodo(a, "#2"); got != "confermata 7120010 persona marcato:delega" {
		t.Errorf("il nodo delegato: %s", got)
	}
	aut := b.autoritaOra()
	if c, figlio := aut.FiglioDiretto(a.AllegatoID, "#4"); !figlio || c != s {
		t.Errorf("con la delega 7120011 e' figlio diretto di 7120010: %v %v", c, figlio)
	}
	if _, figlio := aut.FiglioDiretto(a.AllegatoID, "#5"); figlio {
		t.Error("7120013, nipote, resta guida: la delega non scende da sola")
	}
	if c, figlio := aut.FiglioDiretto(a.AllegatoID, "#2"); !figlio || c != prodotto {
		t.Errorf("7120010 resta figlio diretto del prodotto: %v %v", c, figlio)
	}
	if err := b.accettaIn(a, "#4"); err != nil {
		t.Fatalf("7120011 dalla delega: %v", err)
	}
	deveRifiutare(t, b.accettaIn(a, "#5"), "7120013 è guida")
	if _, err := b.accettaArco(a, "#2", "#4"); err != nil {
		t.Fatalf("l'arco 7120010 → 7120011 della delega: %v", err)
	}

	// la delega di 7120011 dalla sua scheda, sopra quella di 7120010: 7120013 diventa autorita'
	n = uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE codice = '7120011'`)
	rn = fascicolo.RichiestaAutorizzazione{Componente: n, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#4"}}
	e := b.effetto(rn)
	if e.Spento != "" || !e.Delega || e.Titolare != "7120010" || len(e.Figli) != 1 {
		t.Fatalf("la delega di 7120011 dalla sua scheda: %+v", e)
	}
	b.dichiaraOk(rn)
	if c, figlio := b.autoritaOra().FiglioDiretto(a.AllegatoID, "#5"); !figlio || c != n {
		t.Errorf("con le due deleghe 7120013 e' figlio diretto di 7120011: %v %v", c, figlio)
	}

	// revocare il padre revoca le deleghe, con la storia; i nodi decisi restano
	msg, err := b.revoca(prodotto, "")
	ok(t, err)
	if !strings.Contains(msg, "Revocate con lui le deleghe di 7120010, 7120011") {
		t.Errorf("messaggio della revoca: %q", msg)
	}
	if got := b.statoNodo(a, "#1"); got != "aperta - -" {
		t.Errorf("la radice revocata torna aperta: %s", got)
	}
	if got := b.statoNodo(a, "#2"); got != "confermata 7120010 persona" {
		t.Errorf("il nodo della delega revocata resta 7120010, senza marcatura: %s", got)
	}
	if got := b.storiaNodo(a, "#2"); got != "revocata_con_il_padre" {
		t.Errorf("la storia della delega: %q", got)
	}
	if got := b.storiaNodo(a, "#1"); got != "autorizzazione_revocata" {
		t.Errorf("la storia della radice: %q", got)
	}
	d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if len(d.PerComponente)+len(d.Sospese)+len(d.DaSistemare) != 0 {
		t.Errorf("dopo la revoca non resta nessuna dichiarazione: %+v", d)
	}
	if got := uno[bool](b, `SELECT step_strutturale_id IS NULL FROM componente WHERE componente_id = $1`, prodotto); !got {
		t.Error("revocato, il finito non ha piu' lo STEP strutturale")
	}
}

// Un componente, un'autorizzazione (A5.4.6, decisioni ter): autorizzare 7120010.stp per 7120010 revoca la delega
// che 7120010 aveva nel file del prodotto, e l'anteprima lo annuncia; e viceversa, delegare 7120010 revoca il
// suo STEP proprio. La revoca della delega non tocca l'autorizzazione del prodotto.
func TestUnComponenteUnaAutorizzazioneFraDelegaEStepProprio(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if err := b.accettaIn(a, "#2"); err != nil {
		t.Fatal(err)
	}
	s := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE codice = '7120010'`)
	delega := fascicolo.RichiestaAutorizzazione{Componente: s, Nodo: fascicolo.NodoFile{Allegato: a.AllegatoID, Chiave: "#2"}}
	b.dichiaraOk(delega)

	sdoc, _ := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}})
	proprio := fascicolo.RichiestaAutorizzazione{Componente: s, Documento: sdoc}
	e := b.effetto(proprio)
	if len(e.Revoche) != 1 || !strings.HasPrefix(e.Revoche[0], "la delega di 7120001.stp per 7120010") {
		t.Errorf("l'anteprima annuncia la revoca della delega: %v", e.Revoche)
	}
	msg := b.dichiaraOk(proprio)
	if !strings.Contains(msg, "Revocata la delega di") {
		t.Errorf("messaggio: %q", msg)
	}
	d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if x, trovata := d.Di(s); !trovata || x.Delega() || x.Documento.UUID != sdoc || len(d.DelComponente(s)) != 1 {
		t.Errorf("7120010 ha il suo STEP e nessuna delega: %+v", d.DelComponente(s))
	}
	if _, trovata := d.Di(prodotto); !trovata {
		t.Error("l'autorizzazione del prodotto resta")
	}
	if got := b.statoNodo(a, "#2"); got != "confermata 7120010 persona" {
		t.Errorf("il nodo della delega revocata: %s", got)
	}

	// viceversa: la delega revoca lo STEP proprio
	e = b.effetto(delega)
	if len(e.Revoche) != 1 || !strings.Contains(e.Revoche[0], "l'autorizzazione di") {
		t.Errorf("l'anteprima della delega annuncia la revoca dello STEP proprio: %v", e.Revoche)
	}
	b.dichiaraOk(delega)
	d, err = fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if x, trovata := d.Di(s); !trovata || !x.Delega() || len(d.DelComponente(s)) != 1 {
		t.Errorf("7120010 ha la delega e nessuno STEP proprio: %+v", d.DelComponente(s))
	}
}

// Prove 112 e 207 (A5.4.5, A5.4.7, E33): revocare l'autorizzazione rimette in guida. La radice torna aperta con
// la storia, le righe decise da persone restano, gli archi chiusi da un automatismo tornano aperti con la storia,
// le rimozioni aperte si chiudono («revocato lo STEP autorizzato»), il finito perde lo STEP strutturale, e le
// proposte del file tornano guida (non si accettano). Anche con una radice che ha lo stesso codice del prodotto
// (K1, la forma di prima): la revoca toglie lo STEP strutturale e la radice resta aperta come stava.
func TestRevocareLAutorizzazioneRimetteInGuidaEChiudeLeRimozioni(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), stepPBCD())
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: c["P1"], Documento: doc})
	if err := b.accettaIn(a, "#2", "#3", "#5"); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozioni(); got != "P1>D:aperta" {
		t.Fatalf("rimozioni di partenza: %q", got)
	}
	if got := b.relazioniProposte(); !strings.Contains(got, "#1>#3*1:duplicato") {
		t.Fatalf("l'arco P1 → C uguale alla working, chiuso dall'automatismo: %s", got)
	}
	prima := b.bom()
	msg, err := b.revoca(c["P1"], "")
	ok(t, err)
	if !strings.Contains(msg, "i figli diretti tornano guida; le decisioni già prese restano") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.rimozioni(); got != "P1>D:scartata" {
		t.Errorf("rimozioni dopo la revoca: %q", got)
	}
	if nota := uno[string](b, `SELECT nota FROM rimozione_proposta WHERE thread_id = $1`, b.thread); nota != "revocato lo STEP autorizzato" {
		t.Errorf("la nota della rimozione chiusa: %q", nota)
	}
	if got := uno[string](b, `SELECT e ->> 'motivo' FROM componente_proposta, jsonb_array_elements(evidenza -> 'storia') e WHERE proposta_id = $1`,
		b.nodoIn(a, "#1")); got != "prova" {
		t.Errorf("il motivo della revoca e' nella storia della radice: %q", got)
	}
	if got := b.relazioniProposte(); strings.Contains(got, "duplicato") || !strings.Contains(got, "#1>#3*1:aperta") {
		t.Errorf("gli archi automatici tornano aperti: %s", got)
	}
	if got := uno[string](b, `SELECT e ->> 'evento' FROM relazione_proposta, jsonb_array_elements(evidenza -> 'storia') e
		WHERE allegato_id = $1 AND padre_chiave = '#1' AND figlio_chiave = '#3'`, a.AllegatoID); got != "arco_automatico_riaperto" {
		t.Errorf("la storia dell'arco riaperto: %q", got)
	}
	if got := b.statoNodo(a, "#1"); got != "aperta - -" {
		t.Errorf("la radice torna aperta: %s", got)
	}
	if got := b.statoNodo(a, "#2"); !strings.HasPrefix(got, "duplicato") || !strings.HasSuffix(got, "persona") {
		t.Errorf("il figlio deciso da una persona resta deciso: %s", got)
	}
	if b.bom() != prima {
		t.Error("la revoca ha cambiato la working")
	}
	if got := uno[bool](b, `SELECT step_strutturale_id IS NULL FROM componente WHERE componente_id = $1`, c["P1"]); !got {
		t.Error("il finito revocato perde lo STEP strutturale")
	}
	if _, err := b.accettaArco(a, "#1", "#2"); err == nil || !strings.Contains(err.Error(), "è guida") {
		t.Errorf("dopo la revoca le proposte sono guida: %v", err)
	}
	if g := b.strutturali(); g.NFigliDaDecidere+g.NArchiDaDecidere+g.NRimozioni != 0 {
		t.Errorf("il gate non conta la guida: %+v", g)
	}
	_, err = b.revoca(c["P1"], "")
	deveRifiutare(t, err, "non ha uno STEP autorizzato")

	// K1: la forma di prima, con la radice aperta che ha lo stesso codice del prodotto
	b2 := nuovoBanco(t)
	c2 := b2.workingPBCD()
	doc2, a2 := b2.stepDelProdotto(c2["P1"], codiceDi("P1"), stepPBCD())
	b2.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c2["P1"], doc2)
	b2.applica(a2, stepPBCD().json())
	if _, figlio := b2.autoritaOra().FiglioDiretto(a2.AllegatoID, "#2"); !figlio {
		t.Fatal("la forma di prima autorizza i figli diretti")
	}
	_, err = b2.revoca(c2["P1"], "")
	ok(t, err)
	if got := b2.statoNodo(a2, "#1"); got != "aperta - -" {
		t.Errorf("la radice K1 resta aperta: %s", got)
	}
	if _, figlio := b2.autoritaOra().FiglioDiretto(a2.AllegatoID, "#2"); figlio {
		t.Error("revocata la forma di prima, i figli tornano guida")
	}
	if got := uno[bool](b2, `SELECT step_strutturale_id IS NULL FROM componente WHERE componente_id = $1`, c2["P1"]); !got {
		t.Error("la forma di prima revocata: lo STEP strutturale va via")
	}
}

// Prova 208 (A5.4.7, P26): sostituire lo STEP autorizzato. La risposta «il nuovo file diventa lo STEP
// autorizzato?» la da' una persona. «No»: l'autorizzazione resta sul file sostituito ed e' superata (il gate si
// ferma, da sistemare). «Si'»: il file nuovo e' autorizzato (DichiaraStrutturale), il vecchio no; la radice
// vecchia non torna aperta, e la sua marcatura va nella storia con sostituita_da. Se il file nuovo non si
// autorizza cosi' com'e' (due radici), il «si'» si rifiuta e niente cambia. Vale per un sottoassieme come per un
// finito.
func TestSostituireLoStepAutorizzatoSenzaRispostaPreselezionata(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	s := b.componente("7120010", db.TipoComponenteSottoassieme)
	f := fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}}
	vecchio, va := b.stepDelProdotto(s, "7120010", f)
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: vecchio})
	sostituisci := func(vecchio, nuovo uuid.UUID, si bool) (string, error) {
		return b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.Sostituisci(b.ctx, q, b.thread, vecchio, nuovo, si, b.utente)
		})
	}

	// il «si'» su un file che non si autorizza cosi' com'e': niente cambia
	dueRadici, _ := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120019"}})
	_, err := sostituisci(vecchio, dueRadici, true)
	deveRifiutare(t, err, "ha 2 radici")
	if n := uno[int](b, `SELECT count(*) FROM documento WHERE sostituito_da IS NOT NULL`); n != 0 {
		t.Fatalf("il rifiuto ha lasciato %d sostituzioni", n)
	}

	// «no»: superata, da sistemare
	nuovo, na := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120014"}, archi: []string{"#1>#2*2", "#1>#3"}})
	msg, err := sostituisci(vecchio, nuovo, false)
	ok(t, err)
	if !strings.Contains(msg, "resta sul file sostituito ed è superata") {
		t.Errorf("messaggio del no: %q", msg)
	}
	d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if len(d.DaSistemare) != 1 || !strings.HasPrefix(d.DaSistemare[0].Problema, "superata") {
		t.Errorf("la dichiarazione superata ferma il gate: %+v", d.DaSistemare)
	}
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.AnnullaSostituzione(b.ctx, q, b.thread, vecchio)
		return err
	}))

	// «si'»: il nuovo file e' lo STEP autorizzato
	msg, err = sostituisci(vecchio, nuovo, true)
	ok(t, err)
	if !strings.Contains(msg, "è lo STEP autorizzato di 7120010") {
		t.Errorf("messaggio del si': %q", msg)
	}
	d, err = fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if x, trovata := d.Di(s); !trovata || x.Documento.UUID != nuovo || len(d.DaSistemare) != 0 || len(d.DelComponente(s)) != 1 {
		t.Errorf("dopo il si': %+v", d.DelComponente(s))
	}
	if got := b.statoNodo(va, "#1"); got != "duplicato 7120010 persona" {
		t.Errorf("la radice vecchia resta 7120010, senza marcatura: %s", got)
	}
	if got := b.storiaNodo(va, "#1"); got != "sostituita_da" {
		t.Errorf("la storia della radice vecchia: %q", got)
	}
	if got := b.statoNodo(na, "#1"); got != "duplicato 7120010 persona marcato:radice" {
		t.Errorf("la radice nuova: %s", got)
	}
}

// Revocare il padre revoca le sue deleghe (A5.4.6, A5.4.7) anche quando il file del padre e' stato SOSTITUITO: la
// delega di un file sostituito e' «superata» prima di essere senza catena, e restava ferma fra quelle da
// sistemare (il gate si fermava) dopo che il prodotto era passato al file nuovo o era stato revocato. Tre strade,
// su tre banchi: «si'» alla sostituzione (il prodotto passa al file nuovo, e la risposta dice la delega revocata
// con lui); «no» e poi la revoca del prodotto; «no» e poi l'autorizzazione del file nuovo dalla scheda, la cui
// anteprima annuncia la delega. In tutte la delega se ne va con la storia, il nodo resta 7120010 (una decisione
// di una persona), e non resta niente da sistemare.
func TestLeDelegheDiUnFileSostituitoSeNeVannoConIlPadre(t *testing.T) {
	// il prodotto autorizzato con 7120001.stp, 7120010 accettato e delegato; il file nuovo del prodotto, analizzato
	scena := func(t *testing.T) (b *banco, prodotto, vecchio, nuovo uuid.UUID, a db.Allegato) {
		b = nuovoBanco(t)
		prodotto, vecchio, a = b.prodottoProfondo()
		b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: vecchio})
		if err := b.accettaIn(a, "#2"); err != nil {
			t.Fatal(err)
		}
		b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: vecchio, Deleghe: []string{"#2"}})
		if got := b.statoNodo(a, "#2"); got != "confermata 7120010 persona marcato:delega" {
			t.Fatalf("la delega di partenza: %s", got)
		}
		nuovo, _ = b.stepDelProdotto(prodotto, "7120001", fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120012"}, archi: []string{"#1>#2", "#1>#3*2"}})
		return b, prodotto, vecchio, nuovo, a
	}
	sostituisci := func(b *banco, vecchio, nuovo uuid.UUID, si bool) string {
		b.t.Helper()
		msg, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.Sostituisci(b.ctx, q, b.thread, vecchio, nuovo, si, b.utente)
		})
		ok(b.t, err)
		return msg
	}
	// nessuna delega resta: ne' valida, ne' sospesa, ne' da sistemare
	senzaDelega := func(b *banco, a db.Allegato) {
		b.t.Helper()
		d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
		ok(b.t, err)
		for _, x := range append(append(d.Valide(), d.Sospese...), d.DaSistemare...) {
			if x.Delega() {
				b.t.Errorf("la delega resta: %s", x.Frase())
			}
		}
		if len(d.DaSistemare) != 0 {
			b.t.Errorf("resta qualcosa da sistemare (il gate si ferma): %+v", d.DaSistemare)
		}
		if got := b.statoNodo(a, "#2"); got != "confermata 7120010 persona" {
			b.t.Errorf("il nodo della delega resta 7120010, senza marcatura: %s", got)
		}
		if got := b.storiaNodo(a, "#2"); got != "sostituita_da" {
			b.t.Errorf("la storia della delega del file sostituito: %q", got)
		}
	}

	t.Run("si", func(t *testing.T) {
		b, prodotto, vecchio, nuovo, a := scena(t)
		msg := sostituisci(b, vecchio, nuovo, true)
		for _, c := range []string{"è lo STEP autorizzato di 7120001", "con le sue deleghe (7120010)", "evocate con il padre le deleghe di 7120010"} {
			if !strings.Contains(msg, c) {
				t.Errorf("la risposta del si': manca %q in %q", c, msg)
			}
		}
		senzaDelega(b, a)
		d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
		ok(t, err)
		if x, trovata := d.Di(prodotto); !trovata || x.Documento.UUID != nuovo {
			t.Errorf("il prodotto ha il file nuovo: %+v", d.DelComponente(prodotto))
		}
	})

	t.Run("no, poi la revoca", func(t *testing.T) {
		b, prodotto, vecchio, nuovo, a := scena(t)
		sostituisci(b, vecchio, nuovo, false)
		d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
		ok(t, err)
		if len(d.DaSistemare) != 2 {
			t.Fatalf("dopo il no il prodotto e la sua delega sono superati: %+v", d.DaSistemare)
		}
		msg, err := b.revoca(prodotto, "")
		ok(t, err)
		if !strings.Contains(msg, "Revocate con lui le deleghe di 7120010") {
			t.Errorf("la revoca del prodotto dice la delega: %q", msg)
		}
		senzaDelega(b, a)
	})

	t.Run("no, poi l'autorizzazione del file nuovo", func(t *testing.T) {
		b, prodotto, vecchio, nuovo, a := scena(t)
		sostituisci(b, vecchio, nuovo, false)
		e := b.effetto(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: nuovo})
		if len(e.Revoche) != 1 || !strings.Contains(e.Revoche[0], "con le sue deleghe (7120010)") {
			t.Errorf("l'anteprima del file nuovo annuncia la delega che se ne va: %v", e.Revoche)
		}
		msg := b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: nuovo})
		if !strings.Contains(msg, "evocate con il padre le deleghe di 7120010") {
			t.Errorf("la risposta dice la delega revocata: %q", msg)
		}
		senzaDelega(b, a)
	})
}

// A5.4.2 (raggruppamento): un nodo contenitore senza codice sotto la radice («Assembly_1») si dichiara «è il
// componente» nell'anteprima, con una casella mai spuntata: i suoi figli diventano figli diretti, e l'arco radice
// → contenitore e' «stesso componente». Senza dichiararlo, i suoi figli restano guida. E' la versione dichiarata
// di «È il prodotto» dell'editor, che non c'e' piu'.
func TestUnRaggruppamentoDichiaratoPortaISuoiFigli(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	p := b.componente("7120001", db.TipoComponenteFinito)
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=Assembly_1", "#3=7120010", "#4=7120012"}, archi: []string{"#1>#2", "#2>#3", "#2>#4*3"}}
	doc, a := b.stepDelProdotto(p, "7120001", f)
	r := fascicolo.RichiestaAutorizzazione{Componente: p, Documento: doc}
	e := b.effetto(r)
	if len(e.Raggruppamenti) != 1 || e.Raggruppamenti[0].Nodo.Chiave != "#2" || len(e.Raggruppamenti[0].Figli) != 2 {
		t.Fatalf("il contenitore si puo' dichiarare: %+v", e.Raggruppamenti)
	}
	if !strings.Contains(e.RimozioniSospese, "raggruppamenti dichiarati") {
		t.Errorf("le rimozioni aspettano i raggruppamenti: %q", e.RimozioniSospese)
	}
	// senza dichiararlo: i figli del contenitore restano guida
	b.dichiaraOk(r)
	if _, figlio := b.autoritaOra().FiglioDiretto(a.AllegatoID, "#3"); figlio {
		t.Error("senza il raggruppamento 7120010 resta guida")
	}
	// dichiarato: 7120010 e 7120012 sono figli diretti di 7120001
	r.Raggruppamenti = []string{"#2"}
	msg := b.dichiaraOk(r)
	if !strings.Contains(msg, "1 raggruppamento dichiarato come 7120001") {
		t.Errorf("messaggio: %q", msg)
	}
	aut := b.autoritaOra()
	for _, k := range []string{"#3", "#4"} {
		if c, figlio := aut.FiglioDiretto(a.AllegatoID, k); !figlio || c != p {
			t.Errorf("%s e' figlio diretto del prodotto: %v %v", k, c, figlio)
		}
	}
	if got := b.statoNodo(a, "#2"); got != "duplicato 7120001 persona marcato:raggruppamento" {
		t.Errorf("il contenitore: %s", got)
	}
	if got := b.relazioniProposte(); !strings.Contains(got, "#1>#2*1:scartata") {
		t.Errorf("l'arco radice → contenitore e' «stesso componente»: %s", got)
	}
	// l'anteprima del file gia' autorizzato vede attraverso il contenitore dichiarato
	e = b.effetto(fascicolo.RichiestaAutorizzazione{Componente: p, Documento: doc})
	if e.Gia == "" || len(e.Raggruppamenti) != 0 || len(e.Figli) != 2 || e.Figli[0].Nome != "7120010" || e.Figli[1].Nome != "7120012" {
		t.Errorf("i figli diretti attraverso il contenitore: %q %+v %+v", e.Gia, e.Raggruppamenti, e.Figli)
	}
	if err := b.accettaIn(a, "#3"); err != nil {
		t.Errorf("7120010, figlio del contenitore dichiarato: %v", err)
	}
	if _, err := b.accettaArco(a, "#2", "#3"); err != nil {
		t.Errorf("l'arco contenitore → 7120010 e' del prodotto: %v", err)
	}
	if got := b.bom(); !strings.Contains(got, "7120001>7120010*1") {
		t.Errorf("la BOM: %s", got)
	}
	// la revoca riporta aperti la radice e il contenitore
	_, err := b.revoca(p, "")
	ok(t, err)
	if got := b.statoNodo(a, "#2"); got != "aperta - -" {
		t.Errorf("il contenitore dopo la revoca: %s", got)
	}
}

// A5.4.11 (D26): con la BOM congelata niente autorizzazioni ne' revoche: l'anteprima e' spenta e lo dice, la
// scrittura e la revoca si rifiutano.
func TestConLaBomCongelataNienteAutorizzazioniNeRevoche(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	_, err := b.congela("prima baseline")
	ok(t, err)
	req := fascicolo.RichiestaAutorizzazione{Componente: r.p1, Documento: r.step}
	if e := b.effetto(req); !strings.Contains(e.Spento, "la BOM è congelata nella V1") {
		t.Errorf("anteprima con la BOM congelata: %q", e.Spento)
	}
	_, err = b.dichiara(req)
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	_, err = b.revoca(r.p1, "")
	deveRifiutare(t, err, "la BOM è congelata nella V1")
}
