//go:build integrazione

// L4 — la AR tiene il componente (direttiva 35, blocco C-1c; INC-G26, M-46 di PM23): la FK di file_rfq_associazione
// verso componente vale anche per le righe chiuse, quindi la storia dei componenti (ListComponentiConStoria) e il
// riepilogo dell'albero la conoscono e dicono «si archivia», con la frase di cheCosaLoTiene, invece di promettere «si
// elimina». Solo dati sintetici.

package fascicolo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// arDiProva: una AR attiva (allegato → componente) con la conferma e il componente certificato che la 0023 vuole
// (fk_fra_conferma_componente), scritte a mano con i trigger delle tre tabelle spenti solo in questa transazione (la
// nascita vera e' di R5, C-5). Restituisce la AR.
func (b *banco) arDiProva(prodotto, allegato, componente uuid.UUID) uuid.UUID {
	b.t.Helper()
	conferma, ar := uuid.New(), uuid.New()
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			SELECT $1, $2, $3, COALESCE(u.numero, 0) + 1, u.conferma_id, 1, true, repeat('3', 64), 0, 0, 0, '{"rotta":"prova"}', '{"formato":1}',
				repeat('9', 64), '{}', '{}', 'nessuno', $4
			  FROM (SELECT 1) uno LEFT JOIN LATERAL (SELECT numero, conferma_id FROM conferma_strutturale WHERE prodotto_id = $3
				ORDER BY numero DESC LIMIT 1) u ON true`, []any{conferma, b.thread, prodotto, b.utente}},
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, codice, tipo)
			SELECT $1, c.thread_id, c.componente_id, c.codice, c.tipo FROM componente c WHERE c.componente_id = $2`, []any{conferma, componente}},
		{`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
				provenienza_pb, chiave_idempotenza, creata_da)
			VALUES ($1, $2, $3, $4, 'conferma_bom', $5, 'k-prova', 'manuale', gen_random_uuid(), $6)`, []any{ar, b.thread, allegato, componente, conferma, b.utente}},
		{`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_dopo, conferma_id, origine, autore_id)
			VALUES ($1, $2, $3, 'preassociazione', $4, $5, 'conferma_bom', $6)`, []any{b.thread, allegato, ar, componente, conferma, b.utente}},
		// le FK differite si controllano qui: con eventi in sospeso la tabella non si riaccende (55006)
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(b.ctx, x.sql, x.args...); err != nil {
			b.t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
	return ar
}

// M-46: un componente con una AR, attiva o chiusa, ha la storia «associazioni»; il riepilogo dell'albero lo archivia
// e lo dice con la frase di cheCosaLoTiene. Gli altri componenti non hanno quella storia.
func TestLaARTieneIlComponente(t *testing.T) {
	b := nuovoBanco(t)
	pr, c98, _ := b.scenaAlberoDB()
	c97 := b.componente("7121097", db.TipoComponenteSciolto)
	b.arco(pr, c97, 1)
	allegato := uno[uuid.UUID](b, `SELECT a.allegato_id FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.thread_id = $1 AND NOT EXISTS (SELECT 1 FROM documento_proposta p WHERE p.allegato_id = a.allegato_id AND p.componente_id IS NOT NULL)
		ORDER BY a.allegato_id LIMIT 1`, b.thread)
	b.arDiProva(pr, allegato, c97)

	storia := func() map[uuid.UUID]db.ListComponentiConStoriaRow {
		t.Helper()
		righe, err := db.New(b.p).ListComponentiConStoria(b.ctx, b.thread)
		ok(t, err)
		per := map[uuid.UUID]db.ListComponentiConStoriaRow{}
		for _, r := range righe {
			per[r.ComponenteID] = r
		}
		return per
	}
	per := storia()
	if !per[c97].Associazioni || per[c98].Associazioni || per[pr].Associazioni {
		t.Fatalf("la storia delle associazioni: 7121097 %v, 7121098 %v, prodotto %v", per[c97].Associazioni, per[c98].Associazioni, per[pr].Associazioni)
	}
	// la AR chiusa (Scollega e scarta della pagina nuova) tiene ancora il componente
	ok(t, b.tx(func(q *db.Queries) error {
		if _, err := q.BloccaThreadCondiviso(b.ctx, b.thread); err != nil {
			return err
		}
		_, err := associazioni.Scollega(b.ctx, q, associazioni.Atto{Thread: b.thread, Autore: b.utente, Chiave: uuid.New()},
			associazioni.Scollegamento{Allegato: allegato, Componente: c97, Motivo: associazioni.MotivoScollegaFile})
		return err
	}))
	if got := uno[string](b, `SELECT stato FROM file_rfq_associazione WHERE componente_id = $1`, c97); got != "chiusa" {
		t.Fatalf("la AR non si e' chiusa: %s", got)
	}
	if per = storia(); !per[c97].Associazioni {
		t.Error("la AR chiusa non tiene piu' il componente")
	}
	// il riepilogo dell'albero: tolto dalla bozza, 7121097 si archivia e il riepilogo dice perche'
	var conBozza fascicolo.RiepilogoAlbero
	ok(t, b.tx(func(q *db.Queries) (err error) {
		a, err := fascicolo.LeggiAlberoProposto(b.ctx, q, b.thread, analizzatoreProva)
		if err != nil {
			return err
		}
		bozza := fascicolo.BozzaAlbero{Formato: fascicolo.FormatoBozza, Base: a.Firma, Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121097"}}}
		_, conBozza, err = fascicolo.LeggiRiepilogo(b.ctx, q, b.thread, analizzatoreProva, &bozza)
		return err
	}))
	var fuori []string
	for _, f := range conBozza.Fuori {
		if f.Codice == "7121097" {
			fuori = append(fuori, fmt.Sprintf("%s:%s", f.Esito, strings.Join(f.Perche, ",")))
		}
	}
	if len(fuori) != 1 || !strings.HasPrefix(fuori[0], "si_archivia:") || !strings.Contains(fuori[0], fascicolo.FraseAssociazioniRegistrate) {
		t.Errorf("7121097 nel riepilogo: %v", fuori)
	}
}
