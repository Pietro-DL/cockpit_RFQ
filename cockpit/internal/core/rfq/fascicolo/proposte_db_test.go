//go:build integrazione

// L4 — B8.5: dai fatti di uno STEP alle proposte di una RFQ, e dalle proposte alla BOM working solo con
// una decisione. Contro PostgreSQL vero, con la funzione struttura_completa della 0020.
//
// Prove dell'addendum: 9 (con la parte L1 in proposte_test.go), 33, 34, 35, 36, 37, 38; quelle del piano
// B8.5 (A1.6.4) sui nodi, le relazioni, la riclassificazione e l'accettazione in una transazione.

package fascicolo_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// codici: i nomi corti delle prove e i codici veri che li rappresentano. L'estrattore generico riconosce
// «77700002» come codice e non «B», quindi nei fatti e nei componenti vanno i codici; le stringhe da
// confrontare tornano ai nomi corti con leggibile, per restare leggibili.
var codici = map[string]string{
	"P1": "77700001", "B": "77700002", "C": "77700003", "D": "77700004", "F": "77700005",
	"A100": "77710100", "B200": "77710200", "X1": "77720001", "Y1": "77720002",
}

func leggibile(s string) string {
	var coppie []string
	for nome, codice := range codici {
		coppie = append(coppie, codice, nome)
	}
	return strings.NewReplacer(coppie...).Replace(s)
}

// codiceDi: il codice vero di un nome corto (o il nome stesso, se non e' fra i codici di prova).
func codiceDi(nome string) string {
	if c, ok := codici[nome]; ok {
		return c
	}
	return nome
}

// fattiSTEP scrive i fatti di uno STEP. Un nome corto di codici diventa il suo codice. nodi: "chiave=nome" (l'id grezzo e' uguale al nome); archi:
// "#1>#2*3". Le radici sono i nodi senza padre, come le calcola il worker.
type fattiSTEP struct {
	versione int
	troncato bool
	scarti   map[string]int
	avvisi   []string
	nodi     []string
	archi    []string
	radici   []string // se vuoto: calcolate
}

func (f fattiSTEP) json() string {
	if f.versione == 0 {
		f.versione = 3
	}
	type nodo struct {
		Chiave     string `json:"chiave"`
		IDGrezzo   string `json:"id_grezzo"`
		NomeGrezzo string `json:"nome_grezzo"`
		Evidenza   any    `json:"evidenza"`
	}
	type rel struct {
		Padre  string `json:"padre"`
		Figlio string `json:"figlio"`
		Qta    int    `json:"qta"`
	}
	s := map[string]any{"versione": f.versione, "schema": "AP214", "avvisi": append([]string{}, f.avvisi...),
		"limiti": map[string]any{"troncato": f.troncato, "motivo": map[bool]string{true: "nodi"}[f.troncato]}}
	if f.versione >= 3 {
		sc := map[string]int{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}
		for k, v := range f.scarti {
			sc[k] = v
		}
		s["scarti"] = sc
	}
	var nodi []nodo
	for _, n := range f.nodi {
		k, nome, _ := strings.Cut(n, "=")
		nome = codiceDi(nome)
		nodi = append(nodi, nodo{Chiave: k, IDGrezzo: nome, NomeGrezzo: nome, Evidenza: map[string]any{"entita": "PRODUCT", "riga": k}})
	}
	var rels []rel
	figli := map[string]bool{}
	for _, a := range f.archi {
		pf, q, _ := strings.Cut(a, "*")
		qta := 1
		if q != "" {
			fmt.Sscan(q, &qta)
		}
		p, fi, _ := strings.Cut(pf, ">")
		rels = append(rels, rel{Padre: p, Figlio: fi, Qta: qta})
		figli[fi] = true
	}
	radici := f.radici
	if len(radici) == 0 {
		for _, n := range nodi {
			if !figli[n.Chiave] {
				radici = append(radici, n.Chiave)
			}
		}
	}
	s["nodi"], s["relazioni"], s["radici"] = nodi, rels, radici
	out, _ := json.Marshal(map[string]any{"struttura": s})
	return string(out)
}

// allegatoStep e' uno STEP arrivato nella RFQ del banco, con il contenuto sha.
func (b *banco) allegatoStep(nome, sha string) db.Allegato {
	b.t.Helper()
	b.n++
	conv := uno[uuid.UUID](b, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("C-B85-%d", b.n))
	msg := uno[uuid.UUID](b, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3) RETURNING messaggio_id`, fmt.Sprintf("<b85-%d@prova>", b.n), conv, b.thread)
	// un file vero in staging: la rianalisi accoda solo cio' che il worker potra' scaricare
	staging := filepath.Join(b.t.TempDir(), sha+".stp")
	if err := os.WriteFile(staging, []byte("ISO-10303-21;"), 0o644); err != nil {
		b.t.Fatal(err)
	}
	id := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, ricevuto_il)
		VALUES ($1, 1, $2, 'stp', 'file', 'outlook', 100, $3, $4, now()) RETURNING allegato_id`, msg, nome, sha, staging)
	a, err := db.New(b.p).GetAllegato(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return a
}

// stepDelProdotto e' uno STEP confermato del componente, arrivato come allegato della RFQ, con i fatti
// correnti: il documento, l'allegato con lo stesso contenuto, i fatti.
func (b *banco) stepDelProdotto(comp uuid.UUID, codice string, f fattiSTEP) (uuid.UUID, db.Allegato) {
	b.t.Helper()
	doc := b.documento(comp, db.TipoDocumentoCad3d, codice, "stp")
	b.analisi(doc, f.json())
	return doc, b.allegatoStep(codice+".stp", b.sha(doc))
}

func (b *banco) applica(a db.Allegato, fatti string) fascicolo.EsitoStruttura {
	b.t.Helper()
	es, err := b.applicaErr(a, fatti)
	if err != nil {
		b.t.Fatalf("i fatti non si applicano: %v", err)
	}
	return es
}

func (b *banco) applicaErr(a db.Allegato, fatti string) (fascicolo.EsitoStruttura, error) {
	var es fascicolo.EsitoStruttura
	err := b.tx(func(q *db.Queries) error {
		m, err := fascicolo.MotoreDellaRfq(b.ctx, q, b.thread)
		if err != nil {
			return err
		}
		es, err = fascicolo.ApplicaStruttura(b.ctx, q, b.thread, a, json.RawMessage(fatti), m)
		return err
	})
	return es, err
}

func (b *banco) scegliStep(comp, doc uuid.UUID) string {
	b.t.Helper()
	var msg string
	if err := b.tx(func(q *db.Queries) (err error) {
		msg, err = fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, comp, doc)
		return err
	}); err != nil {
		b.t.Fatalf("scelta dello STEP strutturale: %v", err)
	}
	return msg
}

func (b *banco) gesto(f func(q *db.Queries) (string, error)) (string, error) {
	var msg string
	err := b.tx(func(q *db.Queries) (err error) {
		msg, err = f(q)
		return err
	})
	return msg, err
}

// nodiProposti: "chiave:stato" per ogni proposta di nodo della RFQ, in ordine di chiave.
func (b *banco) nodiProposti() string {
	return uno[string](b, `SELECT coalesce(string_agg(chiave || ':' || stato, ' ' ORDER BY chiave, allegato_id), '') FROM componente_proposta WHERE thread_id = $1`, b.thread)
}

func (b *banco) relazioniProposte() string {
	return uno[string](b, `SELECT coalesce(string_agg(padre_chiave || '>' || figlio_chiave || '*' || qta || ':' || stato || coalesce('(' || nota || ')', ''), ' '
		ORDER BY padre_chiave, figlio_chiave, allegato_id), '') FROM relazione_proposta WHERE thread_id = $1`, b.thread)
}

// rimozioni: "PADRE>FIGLIO:stato" con i nomi corti, in ordine.
func (b *banco) rimozioni() string {
	return leggibile(uno[string](b, `SELECT coalesce(string_agg(p.codice || '>' || f.codice || ':' || r.stato, ' ' ORDER BY p.codice, f.codice), '')
		FROM rimozione_proposta r JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id
		WHERE r.thread_id = $1`, b.thread))
}

// bom: l'impronta della working, per dire «non e' cambiata», con i nomi corti.
func (b *banco) bom() string {
	return leggibile(uno[string](b, `SELECT concat_ws(' # ',
		(SELECT string_agg(codice || ':' || tipo || ':' || coalesce(archiviato_il::text, '-'), '|' ORDER BY codice) FROM componente WHERE thread_id = $1),
		(SELECT string_agg(p.codice || '>' || f.codice || '*' || r.qta, '|' ORDER BY p.codice, f.codice)
		   FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id
		  WHERE r.thread_id = $1))`, b.thread))
}

// comp e' un componente di prova con il codice vero del suo nome corto.
func (b *banco) comp(nome string, tipo db.TipoComponente) uuid.UUID {
	return b.componente(codiceDi(nome), tipo)
}

func (b *banco) proposta(chiave string) uuid.UUID {
	return uno[uuid.UUID](b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND chiave = $2 ORDER BY creato_il LIMIT 1`, b.thread, chiave)
}

// working: P1 finito → B ×1, C ×1, D ×1.
func (b *banco) workingPBCD() map[string]uuid.UUID {
	c := map[string]uuid.UUID{"P1": b.comp("P1", db.TipoComponenteFinito)}
	for _, k := range []string{"B", "C", "D"} {
		c[k] = b.comp(k, db.TipoComponenteSciolto)
		b.arco(c["P1"], c[k], 1)
	}
	return c
}

// stepPBCD e' lo STEP strutturale di P1 con A → B ×2, A → C, C → D, A → F: una quantita' diversa, un
// cambio di padre per D, un nodo nuovo.
func stepPBCD() fattiSTEP {
	return fattiSTEP{nodi: []string{"#1=P1", "#2=B", "#3=C", "#4=D", "#5=F"}, archi: []string{"#1>#2*2", "#1>#3", "#3>#4", "#1>#5"}}
}

// ---------------------------------------------------------------- prove

// Prova 9 (L4): uno STEP strutturale letto per intero vede aggiunte, quantita', padri e rimozioni, e
// nessuna di queste cambia la BOM finche' nessuno decide.
func TestLaDifferenzaDelloStepVedeAggiunteRimozioniQtaEPadri(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), stepPBCD())
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	prima := b.bom()
	es := b.applica(a, stepPBCD().json())
	if !es.Completa || !es.Rimozioni.Calcolate {
		t.Fatalf("lettura completa dello STEP strutturale: %+v", es)
	}
	if got, want := b.nodiProposti(), "#1:duplicato #2:duplicato #3:duplicato #4:duplicato #5:aperta"; got != want {
		t.Errorf("nodi = %q, attesi %q", got, want)
	}
	if got, want := b.relazioniProposte(), "#1>#2*2:aperta(qta diversa: 2 contro 1) #1>#3*1:duplicato #1>#5*1:aperta #3>#4*1:aperta"; got != want {
		t.Errorf("relazioni = %q, attese %q", got, want)
	}
	if got := b.rimozioni(); got != "P1>D:aperta" {
		t.Errorf("rimozioni = %q, attesa P1>D (il vecchio padre di D)", got)
	}
	if dopo := b.bom(); dopo != prima {
		t.Errorf("le proposte hanno cambiato la BOM:\nprima %s\ndopo  %s", prima, dopo)
	}

	// le decisioni: la quantita', il nuovo padre, la rimozione
	k := func(p, f string) fascicolo.ChiaveRelazione {
		return fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: p, Figlio: f}
	}
	for _, r := range []fascicolo.ChiaveRelazione{k("#1", "#2"), k("#3", "#4")} {
		if _, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.AccettaRelazione(b.ctx, q, b.thread, r, b.utente)
		}); err != nil {
			t.Fatalf("accetta %v: %v", r, err)
		}
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRimozione(b.ctx, q, b.thread, fascicolo.ChiaveRimozione{Step: doc, Padre: c["P1"], Figlio: c["D"]}, b.utente)
	}); err != nil {
		t.Fatalf("accetta la rimozione: %v", err)
	}
	if got, want := b.bom(), "P1:finito:-|B:sciolto:-|C:sciolto:-|D:sciolto:- # P1>B*2|P1>C*1|C>D*1"; got != want { // in ordine di codice
		t.Errorf("BOM dopo le decisioni = %q, attesa %q", got, want)
	}
	if got := b.rimozioni(); got != "P1>D:confermata" {
		t.Errorf("rimozioni = %q", got)
	}
}

// Prove 33 e 34: una lettura troncata propone le aggiunte, ma ne' quantita' ne' rimozioni.
func TestUnoStepTroncatoNonProponeRimozioniMaProponeLeAggiunte(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	f := stepPBCD()
	f.troncato = true
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), f)
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	es := b.applica(a, f.json())
	if es.Completa || es.Rimozioni.Calcolate || !strings.Contains(es.Rimozioni.Sospese, "lettura completa") {
		t.Fatalf("una lettura troncata non e' completa: %+v", es)
	}
	if got := b.rimozioni(); got != "" {
		t.Errorf("rimozioni da una lettura troncata: %q", got)
	}
	rel := b.relazioniProposte()
	if !strings.Contains(rel, "#1>#2*2:duplicato(qta diversa (2 contro 1) in una lettura incompleta: non proposta)") {
		t.Errorf("la quantita' di una lettura troncata non e' una proposta: %s", rel)
	}
	if !strings.Contains(rel, "#1>#5*1:aperta") || !strings.Contains(rel, "#3>#4*1:aperta") || !strings.Contains(b.nodiProposti(), "#5:aperta") {
		t.Errorf("le aggiunte si propongono anche da una lettura troncata: %s / %s", rel, b.nodiProposti())
	}
}

// Prova 35: ogni scarto che puo' nascondere un arco o un codice blocca le rimozioni; le occorrenze di un
// pezzo dentro se stesso no. Una lettura fallita le blocca.
func TestGliScartiDelParserBloccanoLeRimozioni(t *testing.T) {
	casi := []struct {
		nome    string
		cambia  func(*fattiSTEP)
		rimuove bool
	}{
		{"prodotti senza definizione", func(f *fattiSTEP) { f.scarti = map[string]int{"prodotti_senza_definizione": 1} }, false},
		{"occorrenze non risolte", func(f *fattiSTEP) { f.scarti = map[string]int{"occorrenze_non_risolte": 2} }, false},
		{"testi troncati", func(f *fattiSTEP) { f.scarti = map[string]int{"testi_troncati": 1} }, false},
		{"struttura non letta", func(f *fattiSTEP) { f.avvisi = []string{"struttura non letta: OSError: disco"} }, false},
		{"non e' uno STEP", func(f *fattiSTEP) { f.avvisi = []string{"non e' un file STEP Part 21"} }, false},
		{"pezzi dentro se stessi", func(f *fattiSTEP) { f.scarti = map[string]int{"occorrenze_su_se_stesse": 3} }, true},
	}
	for _, caso := range casi {
		t.Run(caso.nome, func(t *testing.T) {
			b := nuovoBanco(t)
			c := b.workingPBCD()
			f := stepPBCD()
			caso.cambia(&f)
			doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), f)
			b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
			b.applica(a, f.json())
			got := b.rimozioni()
			if caso.rimuove && got != "P1>D:aperta" {
				t.Errorf("gli anelli non nascondono niente: rimozioni = %q, attesa P1>D", got)
			}
			if !caso.rimuove && got != "" {
				t.Errorf("rimozioni da una lettura con scarti: %q", got)
			}
		})
	}
}

// Prova 36: un file di sole parti, o con piu' radici, non e' una distinta, e non rimuove niente.
func TestUnFileDiSolePartiOConPiuRadiciNonRimuove(t *testing.T) {
	for nome, f := range map[string]fattiSTEP{
		"sole parti":  {nodi: []string{"#1=P1"}},
		"due radici":  {nodi: []string{"#1=P1", "#2=B", "#9=ALTRO"}, archi: []string{"#1>#2"}},
		"nessun nodo": {},
	} {
		t.Run(nome, func(t *testing.T) {
			b := nuovoBanco(t)
			c := b.workingPBCD()
			doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), f)
			b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
			b.applica(a, f.json())
			if got := b.rimozioni(); got != "" {
				t.Errorf("rimozioni = %q", got)
			}
		})
	}
}

// Prova 37: i fatti v2 propongono le aggiunte, ma ne' quantita' ne' rimozioni.
func TestIFattiV2NonPropongonoRimozioniNeQuantita(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	f := stepPBCD()
	f.versione = 2
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), f)
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	es := b.applica(a, f.json())
	if es.Completa || !strings.Contains(es.MotivoParziale, "v2") {
		t.Fatalf("una v2 non e' completa: %+v", es)
	}
	if got := b.rimozioni(); got != "" {
		t.Errorf("rimozioni da fatti v2: %q", got)
	}
	rel := b.relazioniProposte()
	if strings.Contains(rel, "#1>#2*2:aperta") || !strings.Contains(rel, "#1>#5*1:aperta") {
		t.Errorf("v2: aggiunte si', quantita' no: %s", rel)
	}
}

// Prova 38: due STEP del prodotto letti per intero; le rimozioni vengono solo dal riferimento.
func TestSoloLoStepStrutturaleProponeRimozioni(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	altro := fattiSTEP{nodi: []string{"#1=P1", "#2=B"}, archi: []string{"#1>#2"}} // non ha C e D
	docAltro, aAltro := b.stepDelProdotto(c["P1"], codiceDi("P1"), altro)
	docRif, aRif := b.stepDelProdotto(c["P1"], codiceDi("P1"), stepPBCD())
	b.applica(aAltro, altro.json())
	if got := b.rimozioni(); got != "" {
		t.Fatalf("un STEP che non e' il riferimento non propone rimozioni: %q", got)
	}
	msg := b.scegliStep(c["P1"], docRif)
	if !strings.Contains(msg, "proposti per la rimozione") {
		t.Errorf("la scelta del riferimento calcola le rimozioni e lo dice: %q", msg)
	}
	b.applica(aRif, stepPBCD().json())
	if got := b.rimozioni(); got != "P1>D:aperta" {
		t.Errorf("rimozioni = %q, attesa P1>D dal riferimento", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM rimozione_proposta WHERE step_documento_id = $1`, docAltro); n != 0 {
		t.Errorf("%d rimozioni dall'altro STEP", n)
	}
	// cambiare riferimento chiude le rimozioni del vecchio
	b.scegliStep(c["P1"], docAltro)
	if got := b.rimozioni(); !strings.Contains(got, "P1>D:scartata") {
		t.Errorf("cambiato il riferimento, la rimozione del vecchio si chiude: %q", got)
	}
	_ = aAltro
}

// Un nodo del riferimento senza codice potrebbe essere proprio il pezzo che sembra sparito: finche'
// qualcuno non gli scrive il codice le rimozioni aspettano.
func TestUnNodoSenzaCodiceSospendeLeRimozioni(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	f := fattiSTEP{nodi: []string{"#1=P1", "#2=B", "#3=C", "#4=Part1"}, archi: []string{"#1>#2", "#1>#3", "#1>#4"}}
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), f)
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	es := b.applica(a, f.json())
	if es.Rimozioni.Calcolate || !strings.Contains(es.Rimozioni.Sospese, "Part1") {
		t.Fatalf("rimozioni con un nodo senza codice: %+v", es.Rimozioni)
	}
	if got := b.rimozioni(); got != "" {
		t.Fatalf("rimozioni = %q", got)
	}
	// l'operatore scrive il codice: Part1 e' D. Adesso il file contiene tutti gli archi della working.
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#4"), codiceDi("D"), "")
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozioni(); got != "" {
		t.Errorf("con Part1 = D nessun arco manca: %q", got)
	}
	// se invece l'operatore dice che Part1 non e' un pezzo della distinta, D manca davvero
	b2 := nuovoBanco(t)
	c2 := b2.workingPBCD()
	doc2, a2 := b2.stepDelProdotto(c2["P1"], codiceDi("P1"), f)
	b2.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c2["P1"], doc2)
	b2.applica(a2, f.json())
	if _, err := b2.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ScartaNodo(b2.ctx, q, b2.thread, b2.proposta("#4"), b2.utente)
	}); err != nil {
		t.Fatal(err)
	}
	if got := b2.rimozioni(); got != "P1>D:aperta" {
		t.Errorf("scartato Part1, D manca: rimozioni = %q", got)
	}
}

func TestAccettareUnNodoNonAccettaGliAltri(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517", "#3=77720518"}, archi: []string{"#1>#2", "#1>#3"}}
	a := b.allegatoStep("assieme.stp", strings.Repeat("a", 64))
	b.applica(a, f.json())
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "77720517 entra nella BOM come sciolto") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.bom(); got != "77720517:sciolto:-" {
		t.Errorf("BOM = %q: accettare un nodo crea quel componente e nessuna relazione", got)
	}
	if got := b.nodiProposti(); got != "#1:aperta #2:confermata #3:aperta" {
		t.Errorf("nodi = %q", got)
	}
	if got := b.relazioniProposte(); got != "#1>#2*1:aperta #1>#3*1:aperta" {
		t.Errorf("relazioni = %q", got)
	}
}

func TestAccettareUnaRelazioneRichiedeINodi(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517"}, archi: []string{"#1>#2*3"}}
	a := b.allegatoStep("assieme.stp", strings.Repeat("b", 64))
	b.applica(a, f.json())
	k := fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"}
	accetta := func() (string, error) {
		return b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.AccettaRelazione(b.ctx, q, b.thread, k, b.utente)
		})
	}
	_, err := accetta()
	deveRifiutare(t, err, "prima i nodi")
	for _, ch := range []string{"#1", "#2"} {
		if _, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta(ch), b.utente, "")
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := accetta(); err != nil {
		t.Fatal(err)
	}
	if got := b.bom(); got != "77720517:sciolto:-|77722757:sottoassieme:- # 77722757>77720517*3" {
		t.Errorf("BOM = %q", got)
	}
}

// A1.1: la stessa coppia da due file con quantita' diverse. La prima crea la relazione; la seconda
// diventa un duplicato con la nota, e la quantita' resta quella della prima: la scelta e'
// dell'ingegnere, non del secondo file.
func TestLaStessaCoppiaDaDueFileConQtaDiversaDiventaDuplicatoConNota(t *testing.T) {
	b := nuovoBanco(t)
	f1 := fattiSTEP{nodi: []string{"#1=A100", "#2=B200"}, archi: []string{"#1>#2"}}
	f2 := fattiSTEP{nodi: []string{"#7=A100", "#8=B200"}, archi: []string{"#7>#8*2"}}
	a1 := b.allegatoStep("uno.stp", strings.Repeat("1", 64))
	a2 := b.allegatoStep("due.stp", strings.Repeat("2", 64))
	b.applica(a1, f1.json())
	b.applica(a2, f2.json())
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a1.AllegatoID, b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	// i nodi del secondo file si sono riconciliati con i componenti appena nati
	if got := b.nodiProposti(); got != "#1:confermata #2:confermata #7:duplicato #8:duplicato" {
		t.Errorf("nodi = %q", got)
	}
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a2.AllegatoID, Padre: "#7", Figlio: "#8"}, b.utente)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "resta quella") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.relazioniProposte(); got != "#1>#2*1:confermata #7>#8*2:duplicato(qta diversa: 2 contro 1)" {
		t.Errorf("relazioni = %q", got)
	}
	if got := b.bom(); !strings.HasSuffix(got, "A100>B200*1") {
		t.Errorf("la quantita' e' quella della prima decisione: %q", got)
	}
}

// Accettare un file intero e' una transazione sola: un nodo senza codice annulla tutto.
func TestAccettareTuttiEUnaTransazione(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517", "#3=Part1"}, archi: []string{"#1>#2", "#2>#3"}}
	a := b.allegatoStep("assieme.stp", strings.Repeat("c", 64))
	b.applica(a, f.json())
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	})
	deveRifiutare(t, err, "non ha un codice")
	if got := b.bom(); got != "" {
		t.Errorf("un rifiuto annulla tutto, anche i nodi gia' accettati nel giro: %q", got)
	}
	if got := b.nodiProposti(); got != "#1:aperta #2:aperta #3:aperta" {
		t.Errorf("nodi = %q", got)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#3"), "77720599", "")
	}); err != nil {
		t.Fatal(err)
	}
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg != "Accettati 3 nodi e 2 relazioni." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.bom(); got != "77720517:sottoassieme:-|77720599:sciolto:-|77722757:sottoassieme:- # 77720517>77720599*1|77722757>77720517*1" {
		t.Errorf("BOM = %q", got)
	}
	// il sottoalbero: su un file nuovo, accettato solo da #2 in giu'
	b2 := nuovoBanco(t)
	a2 := b2.allegatoStep("assieme.stp", strings.Repeat("d", 64))
	g := fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517", "#3=77720599"}, archi: []string{"#1>#2", "#2>#3"}}
	b2.applica(a2, g.json())
	if _, err := b2.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaSottoalbero(b2.ctx, q, b2.thread, a2.AllegatoID, "#2", b2.utente)
	}); err != nil {
		t.Fatal(err)
	}
	if got := b2.bom(); got != "77720517:sottoassieme:-|77720599:sciolto:- # 77720517>77720599*1" {
		t.Errorf("sottoalbero di #2: BOM = %q", got)
	}
}

func TestUnCicloVieneRifiutatoAllAccettazione(t *testing.T) {
	b := nuovoBanco(t)
	x := b.comp("X1", db.TipoComponenteSottoassieme)
	y := b.comp("Y1", db.TipoComponenteSciolto)
	b.arco(x, y, 1)
	f := fattiSTEP{nodi: []string{"#1=Y1", "#2=X1"}, archi: []string{"#1>#2"}} // Y1 contiene X1: il contrario della working
	a := b.allegatoStep("rovescio.stp", strings.Repeat("e", 64))
	b.applica(a, f.json())
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: "#1", Figlio: "#2"}, b.utente)
	})
	deveRifiutare(t, err, fmt.Sprintf("chiuderebbe un ciclo (%s → %s → %s)", codiceDi("X1"), codiceDi("Y1"), codiceDi("X1")))
}

// Due STEP con lo stesso codice danno un componente solo: il secondo nodo ritrova il primo.
func TestDueStepConLoStessoCodiceDannoUnComponenteSolo(t *testing.T) {
	b := nuovoBanco(t)
	a1 := b.allegatoStep("uno.stp", strings.Repeat("f", 64))
	a2 := b.allegatoStep("due.stp", strings.Repeat("9", 64))
	b.applica(a1, fattiSTEP{nodi: []string{"#1=1234567A"}}.json())
	b.applica(a2, fattiSTEP{nodi: []string{"#5=1234567a"}}.json()) // lo stesso codice, scritto in minuscolo
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#5"), b.utente, "")
	}); err != nil {
		t.Fatal(err)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1`, b.thread); n != 1 {
		t.Errorf("%d componenti, atteso 1", n)
	}
	if got := b.nodiProposti(); got != "#1:duplicato #5:confermata" {
		t.Errorf("nodi = %q", got)
	}
}

// Rianalizzare non tocca le proposte decise; riclassifica solo quelle aperte, e un codice scritto
// dall'operatore resta.
func TestUnaRianalisiNonToccaLeProposteDecise(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=77722757", "#2=Part2", "#3=Part3", "#4=Part4"}, archi: []string{"#1>#2", "#1>#3", "#1>#4"}}
	a := b.allegatoStep("assieme.stp", strings.Repeat("7", 64))
	b.applica(a, f.json())
	for _, g := range []func(q *db.Queries) (string, error){
		func(q *db.Queries) (string, error) {
			return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#1"), b.utente, "")
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente)
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#3"), "OP-33", "")
		},
	} {
		if _, err := b.gesto(g); err != nil {
			t.Fatal(err)
		}
	}
	prima := uno[string](b, `SELECT string_agg(chiave || ':' || stato || ':' || coalesce(codice, '-') || ':' || coalesce(deciso_il::text, '-'), ' ' ORDER BY chiave)
		FROM componente_proposta WHERE thread_id = $1 AND chiave IN ('#1', '#2')`, b.thread)
	// la rianalisi: nomi diversi nei fatti
	g := fattiSTEP{nodi: []string{"#1=99999999", "#2=88888888", "#3=77777777", "#4=66666666"}, archi: f.archi}
	b.applica(a, g.json())
	dopo := uno[string](b, `SELECT string_agg(chiave || ':' || stato || ':' || coalesce(codice, '-') || ':' || coalesce(deciso_il::text, '-'), ' ' ORDER BY chiave)
		FROM componente_proposta WHERE thread_id = $1 AND chiave IN ('#1', '#2')`, b.thread)
	if dopo != prima {
		t.Errorf("la rianalisi ha toccato proposte decise:\nprima %s\ndopo  %s", prima, dopo)
	}
	if c := uno[string](b, `SELECT codice FROM componente_proposta WHERE thread_id = $1 AND chiave = '#3'`, b.thread); c != "OP-33" {
		t.Errorf("il codice scritto dall'operatore e' stato riclassificato: %s", c)
	}
	if c := uno[string](b, `SELECT coalesce(codice, '') FROM componente_proposta WHERE thread_id = $1 AND chiave = '#4'`, b.thread); c != "66666666" {
		t.Errorf("la proposta aperta non e' stata riletta: %q", c)
	}
	// e la seconda volta, con gli stessi fatti, non si riscrive niente
	if es := b.applica(a, g.json()); es.Nodi+es.Relazioni != 0 {
		t.Errorf("una rilettura senza cambiamenti ha riscritto %d nodi e %d relazioni", es.Nodi, es.Relazioni)
	}
}

// A1.2: cambiare le regole del cliente riclassifica le proposte aperte, e solo quelle.
func TestUnCambioDiRegoleRiclassificaSoloLeProposteAperte(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=77722757_B", "#2=77720517_C"}, archi: []string{"#1>#2"}}
	a := b.allegatoStep("assieme.stp", strings.Repeat("8", 64))
	b.applica(a, f.json())
	origini := func() string {
		return uno[string](b, `SELECT string_agg(chiave || ':' || coalesce(origine_codice::text, '-') || ':' || coalesce(codice, '-') || ':' || coalesce(rev, '-'), ' ' ORDER BY chiave)
			FROM componente_proposta WHERE thread_id = $1`, b.thread)
	}
	if got := origini(); strings.Contains(got, "famiglia") {
		t.Fatalf("senza famiglie non c'e' un codice di famiglia: %s", got)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#1"), b.utente, "")
	}); err != nil {
		t.Fatal(err)
	}
	decisa := uno[string](b, `SELECT coalesce(origine_codice::text, '-') || ':' || codice FROM componente_proposta WHERE thread_id = $1 AND chiave = '#1'`, b.thread)
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`,
		`{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})(?:_(?P<rev>[A-Z]))?", "descrizione": "disegni 777", "rev_nel_codice": true, "esempio": "77722757_B"}]}`, b.thread)
	b.applica(a, f.json())
	if got := uno[string](b, `SELECT origine_codice::text || ':' || codice || ':' || rev || ':' || famiglia FROM componente_proposta WHERE thread_id = $1 AND chiave = '#2'`, b.thread); got != "famiglia:77720517:C:disegni 777" {
		t.Errorf("la proposta aperta si riclassifica con la regola nuova: %s", got)
	}
	if got := uno[string](b, `SELECT coalesce(origine_codice::text, '-') || ':' || codice FROM componente_proposta WHERE thread_id = $1 AND chiave = '#1'`, b.thread); got != decisa {
		t.Errorf("la proposta decisa e' cambiata: %s, era %s", got, decisa)
	}
}

// La radice riconosciuta da una famiglia e' un'evidenza della proposta del documento, finche' e' aperta.
//
// Riscritta per lo Smistamento (F4, D16 → evidenza, D49): prima era la D16, che correggeva la proposta del
// documento con la radice (codice, rev, fonte `regola_cliente`) quando il codice non era gia' di famiglia, e
// non toccava le righe dell'operatore e quelle decise. Adesso la radice entra nella valutazione del codice
// accanto al nome del file, e le colonne sono il riepilogo: con un nome che non e' un codice la colonna dice
// la radice (come prima); con un nome di famiglia diverso la dimensione e' discorde e la colonna tiene il nome
// (prima restava per la regola «gia' di famiglia», adesso per D49); con un nome uguale alla radice la radice
// vince (80) ma dipende dal nome, e non fa due fonti. Le righe dell'operatore, quelle decise e quelle
// assegnate a un componente non si toccano; applicare di nuovo gli stessi fatti non riscrive niente.
func TestLaRadiceDiFamigliaAggiornaLaPropostaDelDocumento(t *testing.T) {
	b := nuovoBanco(t)
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`,
		`{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})(?:_(?P<rev>[A-Z]))?", "descrizione": "disegni 777", "rev_nel_codice": true, "esempio": "77722757_B"}]}`, b.thread)
	f := fattiSTEP{nodi: []string{"#1=77722757_B", "#2=1234567A"}, archi: []string{"#1>#2"}}
	comp := b.componente("77799999", db.TipoComponenteSciolto)
	casi := []struct {
		nome, file, codice, fonte, stato string
		assegnata                        bool
		atteso                           string // codice:rev:fonte:confidenza:stato della dimensione codice; "" = invariata
	}{
		{"nome che non e' un codice", "assieme 7.stp", "ASSIEME 7", "nome_file", "aperta", false, "77722757:B:regola_cliente:80:unica"},
		{"nome di famiglia diverso", "77720000.stp", "77720000", "nome_file", "aperta", false, "77720000:-:nome_file:70:discorde"},
		{"nome uguale alla radice", "77722757_B.stp", "77722757", "nome_file", "aperta", false, "77722757:B:regola_cliente:80:unica"},
		{"scritta dall'operatore", "assieme 8.stp", "XYZ", "operatore", "aperta", false, ""},
		{"gia' decisa", "assieme 9.stp", "ASSIEME 7", "nome_file", "scartata", false, ""},
		{"assegnata a un componente", "assieme 10.stp", "77799999", "nome_file", "aperta", true, ""},
	}
	for i, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			a := b.allegatoStep(c.file, fmt.Sprintf("%064d", 7000+i))
			b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, stato) VALUES ($1, $2, 'cad_3d', $3, 40, $4, $5)`,
				a.AllegatoID, b.thread, c.codice, c.fonte, c.stato)
			if c.assegnata {
				b.esegui(`UPDATE documento_proposta SET componente_id = $2 WHERE allegato_id = $1`, a.AllegatoID, comp)
			}
			b.applica(a, f.json())
			const lettura = `SELECT codice || ':' || coalesce(rev, '-') || ':' || fonte || ':' || confidenza || ':' ||
				coalesce(dettagli #>> '{valutazione,codice,stato}', '-') FROM documento_proposta WHERE allegato_id = $1`
			got := uno[string](b, lettura, a.AllegatoID)
			switch {
			case c.atteso != "" && got != c.atteso:
				t.Errorf("proposta del documento = %q, attesa %q", got, c.atteso)
			case c.atteso == "" && got != c.codice+":-:"+c.fonte+":40:-":
				t.Errorf("la proposta non doveva cambiare: %q", got)
			}
			if c.atteso == "" {
				return
			}
			// la radice e' un'evidenza, con la famiglia; uguale al nome dipende dal nome
			ev := uno[string](b, `SELECT e ->> 'famiglia' || ':' || coalesce(e ->> 'dipende_da', '-') FROM documento_proposta,
				jsonb_array_elements(dettagli #> '{valutazione,codice,evidenze}') e WHERE allegato_id = $1 AND e ->> 'regola' = 'step_radice_famiglia'`, a.AllegatoID)
			if want := map[bool]string{true: "disegni 777:nome_file", false: "disegni 777:-"}[c.file == "77722757_B.stp"]; ev != want {
				t.Errorf("l'evidenza della radice: %q, attesa %q", ev, want)
			}
			// la seconda volta non si riscrive niente
			prima := uno[string](b, `SELECT xmin::text FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
			b.applica(a, f.json())
			if dopo := uno[string](b, `SELECT xmin::text FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID); dopo != prima {
				t.Errorf("applicare di nuovo gli stessi fatti ha riscritto la proposta")
			}
		})
	}
}

// TestLaRadiceDiFamigliaDiversaDalNomeEDiscorde (Smistamento, prova 232, A5.14.3, D49): il caso guida. Lo
// STEP «7120001A_1.stp» ha la radice 7120001, riconosciuta dalla famiglia ACME 712: il codice e' discorde (la
// radice 80 contro il nome 45), la colonna tiene la lettura del nome con la sua rev, e la confidenza e la fonte
// sono quelle di quella lettura. Nessuna scrittura su una riga dell'operatore o assegnata a un componente.
func TestLaRadiceDiFamigliaDiversaDalNomeEDiscorde(t *testing.T) {
	b := nuovoBanco(t)
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`, b.thread)
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120011"}, archi: []string{"#1>#2", "#2>#3*2"}}
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("7", 64))
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '7120001A', '1', 45, 'nome_file')`,
		a.AllegatoID, b.thread)
	b.applica(a, f.json())
	got := uno[string](b, `SELECT tipo_proposto || ':' || codice || ':' || rev || ':' || confidenza || ':' || fonte || ' | ' ||
		(dettagli #>> '{valutazione,codice,valore}') || ':' || (dettagli #>> '{valutazione,codice,score}') || ':' ||
		(dettagli #>> '{valutazione,codice,regola}') || ':' || (dettagli #>> '{valutazione,codice,stato}') || ':' ||
		(dettagli #>> '{valutazione,da}') || ':' || (dettagli #>> '{valutazione,tipo,stato}')
		FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
	if want := "cad_3d:7120001A:1:45:nome_file | 7120001:80:step_radice_famiglia:discorde:struttura:concorde"; got != want {
		t.Errorf("proposta = %q\natteso      %q", got, want)
	}
	// la lettura del nome resta fra le evidenze, con il suo score
	if n := uno[int](b, `SELECT count(*)::int FROM documento_proposta, jsonb_array_elements(dettagli #> '{valutazione,codice,evidenze}') e
		WHERE allegato_id = $1 AND e ->> 'regola' = 'nome_codice_generico' AND e ->> 'valore' = '7120001A' AND (e ->> 'score')::int = 45`, a.AllegatoID); n != 1 {
		t.Errorf("la lettura del nome fra le evidenze: %d", n)
	}
	// una riga dell'operatore e una assegnata a un componente non cambiano
	comp := b.componente("7120099", db.TipoComponenteSciolto)
	for i, c := range []struct{ fonte, componente string }{{"operatore", ""}, {"nome_file", "si"}} {
		x := b.allegatoStep("7120001A_1.stp", strings.Repeat(fmt.Sprint(i+1), 64))
		codice := map[bool]string{true: "7120099", false: "7120001A"}[c.componente != ""]
		b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', $3, 45, $4)`,
			x.AllegatoID, b.thread, codice, c.fonte)
		if c.componente != "" {
			b.esegui(`UPDATE documento_proposta SET componente_id = $2 WHERE allegato_id = $1`, x.AllegatoID, comp)
		}
		prima := uno[string](b, `SELECT xmin::text FROM documento_proposta WHERE allegato_id = $1`, x.AllegatoID)
		b.applica(x, f.json())
		if dopo := uno[string](b, `SELECT xmin::text || ':' || codice || ':' || fonte FROM documento_proposta WHERE allegato_id = $1`, x.AllegatoID); dopo != prima+":"+codice+":"+c.fonte {
			t.Errorf("%s: la riga e' cambiata: %s", c.fonte, dopo)
		}
	}
}

// La radice dello STEP rilegge il file con Valuta, e un'evidenza del gesto «e' la risposta del fornitore» che
// la riga ha gia' resta (Smistamento F4, A5.14.7): lo scrittore «struttura» non la perde. Il gesto della
// schermata prende oggi solo PDF e fogli (ListProposteRispostaFornitore); qui la riga lo riceve con le stesse
// funzioni, perche' quello che si prova e' lo scrittore, che non deve dimenticare un'evidenza chiunque l'abbia
// messa. Il tipo resta cad_3d (95 contro 70), discorde.
func TestLaRadiceNonPerdeIlGestoDelFornitore(t *testing.T) {
	b := nuovoBanco(t)
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`, b.thread)
	f := fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010"}, archi: []string{"#1>#2"}}
	a := b.allegatoStep("7120001A_1.stp", strings.Repeat("8", 64))
	v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaStage, NomeFile: a.NomeFile}).ConRispostaFornitore()
	dett, rp := classificazione.ConValutazione(nil, v, time.Now())
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, a.AllegatoID, b.thread, rp.Tipo, rp.Codice, rp.Rev, rp.Confidenza, rp.Fonte, dett)
	b.applica(a, f.json())
	got := uno[string](b, `SELECT tipo_proposto || ':' || codice || ':' || fonte || ' | ' || (dettagli #>> '{valutazione,da}') || ':' ||
		(dettagli #>> '{valutazione,tipo,stato}') || ':' || (dettagli #>> '{valutazione,codice,regola}') FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
	if want := "cad_3d:7120001A:nome_file | struttura:discorde:step_radice_famiglia"; got != want {
		t.Errorf("proposta = %q\natteso      %q", got, want)
	}
	if n := uno[int](b, `SELECT count(*)::int FROM documento_proposta, jsonb_array_elements(dettagli #> '{valutazione,tipo,evidenze}') e
		WHERE allegato_id = $1 AND e ->> 'regola' = 'risposta_fornitore' AND e ->> 'valore' = 'offerta_fornitore'`, a.AllegatoID); n != 1 {
		t.Errorf("l'evidenza del gesto dopo la radice: %d", n)
	}
}

// Lo stesso contenuto arrivato due volte nella stessa RFQ propone una volta sola.
func TestLoStessoFileDueVolteNellaRfqProponeUnaVolta(t *testing.T) {
	b := nuovoBanco(t)
	sha := strings.Repeat("5", 64)
	f := fattiSTEP{nodi: []string{"#1=A1", "#2=B2"}, archi: []string{"#1>#2"}}
	b.applica(b.allegatoStep("prima.stp", sha), f.json())
	b.applica(b.allegatoStep("seconda.stp", sha), f.json())
	if got := b.nodiProposti(); got != "#1:aperta #2:aperta" {
		t.Errorf("nodi = %q", got)
	}
}

// Dopo il congelamento le proposte nascono lo stesso (sono interpretazione), ma nessuna diventa BOM
// senza aprire una revisione (D26).
func TestConLaBomCongelataLeProposteNasconoMaNonSiAccettano(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	if _, err := b.congela("prima baseline"); err != nil {
		t.Fatal(err)
	}
	f := fattiSTEP{nodi: []string{"#1=P1", "#2=F1", "#3=NUOVO9"}, archi: []string{"#1>#2*2", "#1>#3"}}
	a := b.allegatoStep("dopo.stp", strings.Repeat("6", 64))
	prima := b.bom()
	b.applica(a, f.json())
	if !strings.Contains(b.nodiProposti(), "#3:aperta") {
		t.Fatalf("la proposta del nodo nuovo non e' nata: %s", b.nodiProposti())
	}
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#3"), b.utente, "")
	})
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	if b.bom() != prima {
		t.Errorf("la BOM congelata e' cambiata")
	}
	_ = r
}

// Accettare una rimozione toglie l'arco dalla working, non dalla baseline.
func TestAccettareUnaRimozioneTogliLArcoSoloDallaWorking(t *testing.T) {
	b := nuovoBanco(t)
	c := b.workingPBCD()
	f := stepPBCD()
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), f)
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	// congelare vuole la BOM senza proposte aperte: la baseline si fa prima di leggere il file
	for _, k := range []string{"B", "C", "D"} {
		b.esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'disegno_2d', 'prova', $3)`, b.thread, c[k], b.utente)
	}
	b.esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'disegno_2d', 'prova', $3)`, b.thread, c["P1"], b.utente)
	if _, err := b.congela("baseline"); err != nil {
		t.Fatalf("congelamento: %v", err)
	}
	b.passaA("SCHEDA_COSTO")
	if _, err := b.apri(nil, "arriva lo STEP nuovo"); err != nil {
		t.Fatalf("revisione: %v", err)
	}
	b.applica(a, f.json())
	if got := b.rimozioni(); got != "P1>D:aperta" {
		t.Fatalf("rimozioni = %q", got)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaRimozione(b.ctx, q, b.thread, fascicolo.ChiaveRimozione{Step: doc, Padre: c["P1"], Figlio: c["D"]}, b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, c["P1"], c["D"]); n != 0 {
		t.Errorf("l'arco e' ancora nella working")
	}
	if n := uno[int](b, `SELECT count(*) FROM bom_versione_relazione r JOIN bom_versione v USING (bom_versione_id)
		WHERE v.numero = 1 AND r.padre_id = $1 AND r.figlio_id = $2`, c["P1"], c["D"]); n != 1 {
		t.Errorf("la baseline V1 ha perso l'arco: %d", n)
	}
}

// La rianalisi della RFQ rilegge gli STEP con i fatti correnti e accoda gli altri, con un limite.
func TestLaRianalisiRileggeEAccodaConUnLimite(t *testing.T) {
	b := nuovoBanco(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	letto := b.allegatoStep("letto.stp", strings.Repeat("3", 64))
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)`,
		letto.Sha256.String, an.Hash(), fattiSTEP{nodi: []string{"#1=A1", "#2=B2"}, archi: []string{"#1>#2"}}.json())
	for i := 0; i < 3; i++ {
		b.allegatoStep(fmt.Sprintf("nuovo%d.stp", i), fmt.Sprintf("%064d", 400+i))
	}
	senza := b.allegatoStep("sparito.stp", strings.Repeat("4", 64))
	b.esegui(`UPDATE allegato SET path_staging = NULL WHERE allegato_id = $1`, senza.AllegatoID)
	tolto := b.allegatoStep("tolto dalla cache.stp", strings.Repeat("5", 63)+"a")
	if err := os.Remove(tolto.PathStaging.String); err != nil {
		t.Fatal(err)
	}
	rianalizza := func() fascicolo.Rianalisi {
		var ri fascicolo.Rianalisi
		if err := b.tx(func(q *db.Queries) (err error) {
			ri, err = fascicolo.RianalizzaRfq(b.ctx, q, b.thread, an, 2)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return ri
	}
	ri := rianalizza()
	if ri.Riletti != 1 || ri.Accodati != 2 || ri.Rimandati != 1 || ri.SenzaStaging != 2 {
		t.Errorf("prima rianalisi: %+v", ri)
	}
	if got := b.nodiProposti(); got != "#1:aperta #2:aperta" {
		t.Errorf("il file con i fatti correnti e' stato riletto: %q", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 2 {
		t.Errorf("job accodati = %d, attesi 2", n)
	}
	ri = rianalizza()
	if ri.GiaInCoda != 2 || ri.Accodati != 1 || ri.Rimandati != 0 {
		t.Errorf("seconda rianalisi: %+v", ri)
	}
}

// In uno STEP vero due PRODUCT diversi portano lo stesso codice sotto lo stesso padre: dopo i nodi, le
// loro due relazioni sono la stessa coppia di componenti. Accettando la prima la seconda si riconcilia
// (duplicato), e «accetta tutto il file» deve andare avanti, non rifiutarla come gia' decisa. Il primo
// codice di B8.5 lo rifiutava: trovato rileggendo gli STEP veri, fuori dal repository.
func TestAccettareIlFileConDueNodiDelloStessoCodiceSottoLoStessoPadre(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=P1", "#2=B", "#3=B"}, archi: []string{"#1>#2", "#1>#3"}}
	a := b.allegatoStep("configurazioni.stp", strings.Repeat("0", 63)+"1")
	b.applica(a, f.json())
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	})
	if err != nil {
		t.Fatalf("accetta tutto il file: %v", err)
	}
	if msg != "Accettati 2 nodi e 1 relazione." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.bom(); got != "P1:sottoassieme:-|B:sciolto:- # P1>B*1" {
		t.Errorf("BOM = %q", got)
	}
	if got := b.relazioniProposte(); got != "#1>#2*1:confermata #1>#3*1:duplicato" {
		t.Errorf("relazioni = %q", got)
	}
}

// Nello stesso STEP vero un PRODUCT contiene un altro PRODUCT con lo stesso codice (A1.1,
// configurazioni). Dopo i nodi sono lo stesso componente: l'arco fra loro si scarta con la nota, quelli
// sotto il secondo finiscono sotto il componente, e «accetta tutto il file» arriva in fondo.
func TestAccettareIlFileConUnNodoDentroUnoDelloStessoCodice(t *testing.T) {
	b := nuovoBanco(t)
	f := fattiSTEP{nodi: []string{"#1=P1", "#2=P1", "#3=B"}, archi: []string{"#1>#2", "#2>#3*2"}}
	a := b.allegatoStep("se-stesso.stp", strings.Repeat("0", 63)+"2")
	b.applica(a, f.json())
	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaFile(b.ctx, q, b.thread, a.AllegatoID, b.utente)
	})
	if err != nil {
		t.Fatalf("accetta tutto il file: %v", err)
	}
	if msg != "Accettati 2 nodi e 1 relazione." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := b.bom(); got != "P1:sottoassieme:-|B:sciolto:- # P1>B*2" {
		t.Errorf("BOM = %q", got)
	}
	if got := b.relazioniProposte(); got != "#1>#2*1:scartata(padre e figlio sono lo stesso componente: un pezzo non contiene se stesso) #2>#3*2:confermata" {
		t.Errorf("relazioni = %q", got)
	}
}
