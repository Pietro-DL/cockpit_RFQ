//go:build integrazione

// L4 — l'albero proposto e il riepilogo contro PostgreSQL vero (giro 4, fase 4.4a.1a): la lettura dell'albero e il
// riepilogo, anche con una bozza, non scrivono niente (ogni tabella dello schema ha le stesse righe con lo stesso
// contenuto, prima e dopo); l'albero si legge dalle righe che ApplicaStruttura ha scritto; la storia dei componenti
// (ListComponentiConStoria) decide fra archiviare ed eliminare un componente che resterebbe senza padri (29b = A).

package fascicolo_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// fotoTabelle e' lo stato di ogni tabella dello schema: quante righe e l'impronta del contenuto (come la prova 98 del
// web, fotoDelDatabase).
func (b *banco) fotoTabelle() map[string]string {
	b.t.Helper()
	righe, err := b.p.Query(b.ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`)
	ok(b.t, err)
	tabelle, err := pgx.CollectRows(righe, pgx.RowTo[string])
	ok(b.t, err)
	if len(tabelle) < 20 {
		b.t.Fatalf("lo schema ha %d tabelle: non e' quello del Cockpit", len(tabelle))
	}
	foto := map[string]string{}
	for _, tab := range tabelle {
		id := pgx.Identifier{tab}.Sanitize()
		foto[tab] = uno[string](b, `SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM `+id+` x`)
	}
	return foto
}

func tabelleCambiate(prima, dopo map[string]string) string {
	var out []string
	for k, v := range dopo {
		if prima[k] != v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// scenaAlberoDB e' la RFQ ACME del prodotto 7120001 con lo STEP del prodotto (7120010, 7120011 ×2; 7120011 → 7121003)
// e quello di 7120010 (7121003 ×2, 7121005), le righe scritte da ApplicaStruttura; nella working, sotto il prodotto,
// 7121098 (senza storia) e 7121099 (con un disegno).
func (b *banco) scenaAlberoDB() (prodotto, p98, p99 uuid.UUID) {
	b.t.Helper()
	pr := b.prodottoConfermato("7120001")
	b.stepLetto("7120001.stp", strings.Repeat("a1", 32), fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120011", "#4=7121003"},
		archi: []string{"#1>#2", "#1>#3*2", "#3>#4"}})
	b.stepLetto("7120010.stp", strings.Repeat("a2", 32), fattiSTEP{nodi: []string{"#1=7120010", "#2=7121003", "#3=7121005"},
		archi: []string{"#1>#2*2", "#1>#3"}})
	c98 := b.componente("7121098", db.TipoComponenteSciolto)
	c99 := b.componente("7121099", db.TipoComponenteSciolto)
	b.arco(pr, c98, 1)
	b.arco(pr, c99, 1)
	b.documento(c99, db.TipoDocumentoDisegno2d, "7121099", "pdf")
	return pr, c98, c99
}

// La lettura dell'albero e il riepilogo (con la bozza vuota e con una bozza che toglie due pezzi) non cambiano una riga
// del database; l'albero e' quello delle righe degli STEP e della working, e la storia dei componenti dice la sorte di
// quelli che resterebbero senza padri: 7121099, con un disegno, si archivia; 7121098 si elimina.
func TestAlberoPropostoERiepilogoNonScrivono(t *testing.T) {
	b := nuovoBanco(t)
	_, c98, c99 := b.scenaAlberoDB()
	prima := b.fotoTabelle()
	var a fascicolo.AlberoProposto
	var vuota, conBozza fascicolo.RiepilogoAlbero
	ok(t, b.tx(func(q *db.Queries) (err error) {
		if a, err = fascicolo.LeggiAlberoProposto(b.ctx, q, b.thread, analizzatoreProva); err != nil {
			return err
		}
		if _, vuota, err = fascicolo.LeggiRiepilogo(b.ctx, q, b.thread, analizzatoreProva, nil); err != nil {
			return err
		}
		bozza := fascicolo.BozzaAlbero{Formato: fascicolo.FormatoBozza, Base: a.Firma,
			Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121098"}, {Nodo: "cod:7121099"}}}
		_, conBozza, err = fascicolo.LeggiRiepilogo(b.ctx, q, b.thread, analizzatoreProva, &bozza)
		return err
	}))
	if d := tabelleCambiate(prima, b.fotoTabelle()); d != "" {
		t.Fatalf("la lettura dell'albero e il riepilogo hanno scritto in: %s", d)
	}

	var archi []string
	for _, x := range a.Archi {
		archi = append(archi, fmt.Sprintf("%s>%s*%d:%s", strings.TrimPrefix(x.Padre, "cod:"), strings.TrimPrefix(x.Figlio, "cod:"), x.Qta, x.Stato))
	}
	atteso := "7120001>7120010*1:proposto 7120001>7120011*2:proposto 7120001>7121098*1:nella_distinta 7120001>7121099*1:nella_distinta " +
		"7120010>7121003*2:proposto 7120010>7121005*1:proposto 7120011>7121003*1:proposto"
	if strings.Join(archi, " ") != atteso {
		t.Errorf("l'albero:\n%s\natteso:\n%s", strings.Join(archi, " "), atteso)
	}
	if n, trovato := a.Nodo("cod:7121003"); !trovato || strings.Join(n.Padri, " ") != "cod:7120010 cod:7120011" || len(n.Righe) != 2 {
		t.Errorf("7121003: %+v", n)
	}
	if n, trovato := a.Nodo("cod:7120010"); !trovato || n.Step == nil || strings.Join(n.Step.File, ",") != "7120010.stp" {
		t.Errorf("7120010 e il suo STEP: %+v", n)
	}
	if len(a.Prodotti) != 1 || a.Prodotti[0].Ancora != fascicolo.LivelloPiena || a.Firma == "" {
		t.Errorf("il prodotto: %+v, firma %q", a.Prodotti, a.Firma)
	}
	if !vuota.Confermabile || vuota.AlberoFirma != a.Firma || len(vuota.Nuovi) != 4 || len(vuota.LegamiNuovi) != 5 {
		t.Errorf("la bozza vuota: %+v", vuota)
	}
	var fuori []string
	for _, f := range conBozza.Fuori {
		fuori = append(fuori, fmt.Sprintf("%s:%s:%s", f.Codice, f.Esito, strings.Join(f.Perche, ",")))
		if (f.Codice == "7121098" && f.Componente != c98) || (f.Codice == "7121099" && f.Componente != c99) {
			t.Errorf("%s: il componente e' un altro", f.Codice)
		}
	}
	if strings.Join(fuori, " | ") != "7121098:si_elimina:non ha documenti né storia | 7121099:si_archivia:ha dei documenti" {
		t.Errorf("i componenti senza padri: %v", fuori)
	}
}

// La storia dei componenti (ListComponentiConStoria) vede ogni riga che ferma la cancellazione: il documento, la
// proposta di struttura decisa, la baseline. Un componente senza niente non ha storia.
func TestLaStoriaDeiComponenti(t *testing.T) {
	b := nuovoBanco(t)
	_, c98, c99 := b.scenaAlberoDB()
	accettato := b.componente("7121005", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente_proposta SET stato = 'confermata', componente_id = $2, deciso_da = $3, deciso_il = now()
		WHERE thread_id = $1 AND codice = '7121005'`, b.thread, accettato, b.utente)
	righe, err := db.New(b.p).ListComponentiConStoria(b.ctx, b.thread)
	ok(t, err)
	per := map[uuid.UUID]db.ListComponentiConStoriaRow{}
	for _, r := range righe {
		per[r.ComponenteID] = r
	}
	if r := per[c98]; r.Documenti || r.Proposte || r.Deroghe || r.Baseline || r.Note {
		t.Errorf("7121098 non ha storia: %+v", r)
	}
	if r := per[c99]; !r.Documenti || r.Proposte {
		t.Errorf("7121099 ha un disegno: %+v", r)
	}
	if r := per[accettato]; r.Documenti || !r.Proposte {
		t.Errorf("7121005 ha una proposta di struttura decisa: %+v", r)
	}
	if len(righe) != 4 {
		t.Errorf("i componenti della RFQ: %d", len(righe))
	}
}

// RD-04 / RSC-03 (revisioni della notte): le tabelle della 0022 che tengono un componente entrano nella storia. Un
// componente tenuto solo dalla storia della fonte STEP (una fonte secondaria scelta per lui) o solo da una copertura
// documentale si archivia, e il riepilogo lo dice prima della conferma, invece di promettere «si elimina» e poi cadere
// sulla FK.
func TestLaStoriaDellaDistintaTieneIlComponente(t *testing.T) {
	b := nuovoBanco(t)
	pr, c98, c99 := b.scenaAlberoDB()
	c97 := b.componente("7121097", db.TipoComponenteSciolto)
	b.arco(pr, c97, 1)
	var allegato uuid.UUID
	var shaAllegato string
	if err := b.p.QueryRow(b.ctx, `SELECT a.allegato_id, a.sha256 FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.thread_id = $1 AND a.sha256 IS NOT NULL ORDER BY a.allegato_id LIMIT 1`, b.thread).Scan(&allegato, &shaAllegato); err != nil {
		t.Fatal(err)
	}
	// 7121098: una fonte STEP secondaria scelta per lui (ruolo sottoassieme, destinazione con UUID)
	b.esegui(`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, componente_id, azione, allegato_id, sha256, gruppo, autore_id)
		VALUES ($1, $2, 'sottoassieme', $3, 'scelta', $4, $5, 'cand:a:r', $6)`, b.thread, pr, c98, allegato, shaAllegato, b.utente)
	// 7121097: coperto dalla pagina 1 del disegno di 7121099
	var doc uuid.UUID
	var shaDoc string
	if err := b.p.QueryRow(b.ctx, `SELECT documento_id, sha256 FROM documento WHERE componente_id = $1 AND tipo = 'disegno_2d'`, c99).Scan(&doc, &shaDoc); err != nil {
		t.Fatal(err)
	}
	b.esegui(`INSERT INTO copertura_documento (thread_id, documento_id, sha256, documento_componente, componente_id, tipo, azione, pagine, pagine_totali,
			rev_documento, rev_componente, motivo, chiave_idempotenza, autore_id)
		VALUES ($1, $2, $3, $4, $5, 'disegno_2d', 'copre', '{1}', 1, NULL, NULL, 'il disegno della staffa e'' nel PDF di 7121099', gen_random_uuid(), $6)`,
		b.thread, doc, shaDoc, c99, c97, b.utente)

	righe, err := db.New(b.p).ListComponentiConStoria(b.ctx, b.thread)
	ok(t, err)
	per := map[uuid.UUID]db.ListComponentiConStoriaRow{}
	for _, r := range righe {
		per[r.ComponenteID] = r
	}
	if r := per[c98]; !r.Distinta || r.Coperture || r.Documenti {
		t.Errorf("7121098 ha la storia della fonte STEP: %+v", r)
	}
	if r := per[c97]; r.Distinta || !r.Coperture || r.Documenti {
		t.Errorf("7121097 e' coperto da un documento condiviso: %+v", r)
	}
	if r := per[pr]; !r.Distinta {
		t.Errorf("il prodotto ha la storia della fonte STEP (prodotto_id): %+v", r)
	}
	var conBozza fascicolo.RiepilogoAlbero
	ok(t, b.tx(func(q *db.Queries) (err error) {
		a, err := fascicolo.LeggiAlberoProposto(b.ctx, q, b.thread, analizzatoreProva)
		if err != nil {
			return err
		}
		bozza := fascicolo.BozzaAlbero{Formato: fascicolo.FormatoBozza, Base: a.Firma,
			Tolti: []fascicolo.ToltoBozza{{Nodo: "cod:7121097"}, {Nodo: "cod:7121098"}}}
		_, conBozza, err = fascicolo.LeggiRiepilogo(b.ctx, q, b.thread, analizzatoreProva, &bozza)
		return err
	}))
	var fuori []string
	for _, f := range conBozza.Fuori {
		fuori = append(fuori, fmt.Sprintf("%s:%s:%s", f.Codice, f.Esito, strings.Join(f.Perche, ",")))
	}
	atteso := "7121097:si_archivia:è coperto da un documento condiviso | " +
		"7121098:si_archivia:ha una storia della distinta (conferme, bozze, fonte STEP o fascicolo attestato)"
	if strings.Join(fuori, " | ") != atteso {
		t.Errorf("i componenti senza padri: %v; attesi: %s", strings.Join(fuori, " | "), atteso)
	}
}
