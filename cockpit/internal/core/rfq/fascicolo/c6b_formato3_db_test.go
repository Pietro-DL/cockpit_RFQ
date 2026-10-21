//go:build integrazione

// L4 — C-6b, corsia file (direttiva 39 §2, §6; D-TAB-01 e D-TAB-02 B rivista [T-39]): nelle RFQ formato 3 «Autorizza come
// STEP» di un finito non scrive step_strutturale_id e rimanda a S8 (le autorizzazioni dei sottoassiemi restano); lo
// spostamento diretto di un nodo fra prodotti e' vietato su tutte le RFQ, quello interno resta. Solo dati sintetici.

package fascicolo_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// rendiFormato3 fa della RFQ una RFQ formato 3 nel modo vero: una bozza formato 3 del prodotto, creata da R2 con la
// partenza vuota (il segno della RFQ formato 3 lo scrive il database alla nascita della bozza).
func (b *banco) rendiFormato3(prodotto uuid.UUID) uuid.UUID {
	b.t.Helper()
	raw, _ := json.Marshal(map[string]any{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "crea": map[string]any{"partenza": "vuota"}})
	req, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		b.t.Fatalf("crea vuota: %v", err)
	}
	c, err := distintabozza.CreaFormato3(b.ctx, b.p, req.CreaDaRichiesta(b.thread, b.utente, nil))
	if err != nil {
		b.t.Fatalf("crea vuota: %v", err)
	}
	if !uno[bool](b, `SELECT rfq_e_formato3($1)`, b.thread) {
		b.t.Fatal("dopo la bozza formato 3 la RFQ e' formato 3")
	}
	return c.BozzaID
}

// «Autorizza come STEP» di un finito: nella RFQ legacy scrive lo STEP strutturale (invariato, la prova di sempre); nella
// RFQ formato 3 si rifiuta con il rimando a S8 e non scrive niente, anche con la firma vista e la casella spuntata.
// L'autorizzazione di un sottoassieme (marcatura, senza STEP strutturale) resta.
func TestC6bAutorizzaNelFormato3RimandaAS8(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, _ := b.prodottoProfondo()
	b.rendiFormato3(prodotto)
	r := fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc}
	e := b.effetto(r)
	r.Firma, r.Autorizza, r.PresaDAtto = e.Firma, true, e.PresaDAtto != ""
	prima := b.fotoTabelle()
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, r)
	})
	if err == nil || !strings.Contains(err.Error(), "Conferma la sorgente") || !strings.Contains(err.Error(), "formato 3") {
		t.Fatalf("Autorizza nel formato 3: %v", err)
	}
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		t.Errorf("il rifiuto ha scritto in %s", d)
	}
	if got := uno[bool](b, `SELECT step_strutturale_id IS NULL FROM componente WHERE componente_id = $1`, prodotto); !got {
		t.Error("lo STEP strutturale nel formato 3 lo scrive solo R5")
	}

	// il sottoassieme: la marcatura sola, senza STEP strutturale, resta anche nel formato 3
	s := b.componente("7120010", db.TipoComponenteSottoassieme)
	sdoc, _ := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}})
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: sdoc})
	d, err := fascicolo.LeggiDichiarazioni(b.ctx, db.New(b.p), b.thread)
	ok(t, err)
	if _, trovata := d.Di(s); !trovata {
		t.Errorf("l'autorizzazione del sottoassieme nel formato 3: %+v", d.DelComponente(s))
	}
}

// D-TAB-01 [T-39]: «Sposta» fra due padri di prodotti diversi si rifiuta senza scrivere (anche in una RFQ legacy); lo
// spostamento interno allo stesso prodotto riesce; un padre condiviso fra due prodotti verso un padre di uno solo e'
// uno spostamento fra prodotti; senza padre di partenza (un collegamento) resta com'era.
func TestC6bSpostaFraProdottiVietatoSuTutteLeRFQ(t *testing.T) {
	b := nuovoBanco(t)
	p1 := b.componente("7120001", db.TipoComponenteFinito)
	p2 := b.componente("7120002", db.TipoComponenteFinito)
	s1 := b.componente("7120010", db.TipoComponenteSottoassieme)
	s1b := b.componente("7120011", db.TipoComponenteSottoassieme)
	s2 := b.componente("7120020", db.TipoComponenteSottoassieme)
	cond := b.componente("7120030", db.TipoComponenteSottoassieme)
	x := b.componente("7121001", db.TipoComponenteSciolto)
	y := b.componente("7121002", db.TipoComponenteSciolto)
	b.arco(p1, s1, 1)
	b.arco(p1, s1b, 1)
	b.arco(p2, s2, 1)
	b.arco(p1, cond, 1)
	b.arco(p2, cond, 1)
	b.arco(s1, x, 2)
	b.arco(cond, y, 1)
	nn := func(u uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: u, Valid: true} }
	sposta := func(figlio uuid.UUID, da, a uuid.NullUUID) error {
		_, err := b.gesto(func(q *db.Queries) (string, error) {
			return fascicolo.Sposta(b.ctx, q, b.thread, figlio, da, a, b.utente, 1)
		})
		return err
	}
	prima := b.fotoTabelle()
	if err := sposta(x, nn(s1), nn(s2)); err == nil || !strings.Contains(err.Error(), "fra prodotti") ||
		!strings.Contains(err.Error(), "7120001") || !strings.Contains(err.Error(), "7120002") {
		t.Fatalf("da 7120010 (7120001) a 7120020 (7120002): %v", err)
	}
	if err := sposta(y, nn(cond), nn(s1)); err == nil || !strings.Contains(err.Error(), "fra prodotti") {
		t.Fatalf("dal condiviso a un padre di un solo prodotto: %v", err)
	}
	if err := sposta(x, nn(s1), nn(p2)); err == nil || !strings.Contains(err.Error(), "fra prodotti") {
		t.Fatalf("verso un altro prodotto finito: %v", err)
	}
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		t.Errorf("i rifiuti hanno scritto in %s", d)
	}

	// interno allo stesso prodotto
	if err := sposta(x, nn(s1), nn(s1b)); err != nil {
		t.Fatalf("interno a 7120001: %v", err)
	}
	if got := uno[int](b, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, s1b, x); got != 1 {
		t.Errorf("lo spostamento interno: %d", got)
	}
	// senza padre di partenza: e' un collegamento, non uno spostamento fra prodotti
	z := b.componente("7121003", db.TipoComponenteSciolto)
	if err := sposta(z, uuid.NullUUID{}, nn(s2)); err != nil {
		t.Fatalf("collegare un nodo senza padre: %v", err)
	}
	// anche nella RFQ formato 3 la regola e' la stessa
	b.rendiFormato3(p1)
	if err := sposta(x, nn(s1b), nn(s2)); err == nil || !strings.Contains(err.Error(), "fra prodotti") {
		t.Fatalf("nel formato 3: %v", err)
	}
}
