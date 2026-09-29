//go:build integrazione

// L4 — Smistamento, giro 4, fase 4.17a: il censimento legge il database vero in sola lettura. Le righe ACME
// inventate coprono ogni fonte (mail, file, nodi STEP, cartiglio dai fatti, pezzi, prodotti confermati e
// documenti), il messaggio orfano attribuito dall'anagrafica e la posta di un fornitore, che resta fuori.
// Prima e dopo, ogni tabella dello schema ha le stesse righe con lo stesso contenuto.
package censimento

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

type bancoCensimento struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	n    int
}

func (b *bancoCensimento) id(sql string, arg ...any) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&id); err != nil {
		b.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (b *bancoCensimento) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%s: %v", sql, err)
	}
}

// messaggio scrive una mail in entrata con la sua controparte; thread uuid.Nil = orfana.
func (b *bancoCensimento) messaggio(thread uuid.UUID, oggetto, corpo, mittente, tipo string, cliente, fornitore uuid.UUID, via string) uuid.UUID {
	b.t.Helper()
	b.n++
	conv := b.id(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("CONV-CENS-%d", b.n))
	return b.id(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, corpo_testo, thread_id,
			mittente_indirizzo, controparte_tipo, controparte_cliente_id, controparte_fornitore_id, controparte_via)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3, $4, $5, $6, $7::text::tipo_controparte, $8, $9, NULLIF($10::text, '')::via_controparte)
		RETURNING messaggio_id`,
		fmt.Sprintf("<cens-%d@acme.example>", b.n), conv, oggetto, corpo, uuid.NullUUID{UUID: thread, Valid: thread != uuid.Nil}, mittente,
		tipo, uuid.NullUUID{UUID: cliente, Valid: cliente != uuid.Nil}, uuid.NullUUID{UUID: fornitore, Valid: fornitore != uuid.Nil}, via)
}

func (b *bancoCensimento) allegato(msg uuid.UUID, indice int, nome, sha string) uuid.UUID {
	b.t.Helper()
	return b.id(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, bytes, sha256, stato, ricevuto_il)
		VALUES ($1, $2, $3::text, lower(split_part($3::text, '.', 2)), 'file', 9000, $4, 'analizzato', now()) RETURNING allegato_id`, msg, indice, nome, sha)
}

// fotoDB e' lo stato di ogni tabella dello schema: quante righe e l'impronta del contenuto (come la prova 98
// del pacchetto web), cosi' che si veda anche una lettura che riscrive una riga senza aggiungerne.
func fotoDB(t *testing.T, ctx context.Context, p *pgxpool.Pool) map[string]string {
	t.Helper()
	righe, err := p.Query(ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	tabelle, err := pgx.CollectRows(righe, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(tabelle) < 20 {
		t.Fatalf("lo schema ha %d tabelle: non e' quello del Cockpit", len(tabelle))
	}
	foto := map[string]string{}
	for _, tab := range tabelle {
		id := pgx.Identifier{tab}.Sanitize()
		var v string
		if err := p.QueryRow(ctx, `SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM `+id+` x`).Scan(&v); err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		foto[tab] = v
	}
	return foto
}

func diverse(prima, dopo map[string]string) []string {
	var out []string
	for tab, v := range dopo {
		if prima[tab] != v {
			out = append(out, tab)
		}
	}
	slices.Sort(out)
	return out
}

// TestIlCensimentoLeggeTutteLeFontiInSolaLettura: il censimento vero sul database (Leggi e Raccogli). Ogni fonte
// arriva al suo cliente: le mail di ACME (nella RFQ, e una orfana attribuita dal dominio) e non quella del
// fornitore nella stessa RFQ; i file tecnici con il loro contenuto e la decisione sullo stesso contenuto (il
// PDF rimandato con lo stesso nome e un contenuto nuovo e' un altro file, senza decisione); il nodo dello
// STEP; il campo codice del cartiglio dai fatti dell'analizzatore 4, letti dalla classificazione; i pezzi con
// il tipo scelto e i nomi dei nodi accettati, non quello archiviato; il prodotto confermato (quello non
// confermato no) e il codice del documento. Le mail lette all'arrivo: con un esito diverso da «ignora», con
// «ignora» e un codice salvato (tutte e due nel confronto) e con «ignora» senza niente salvato (a parte). La
// transazione e' di sola lettura, e nessuna tabella cambia.
func TestIlCensimentoLeggeTutteLeFontiInSolaLettura(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	b := &bancoCensimento{t: t, ctx: context.Background(), pool: pool}
	utente := b.id(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('PR', 'Prova', 'Tecnico') RETURNING utente_id`)
	acme := b.id(`INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ('ACME', 'Acme S.p.A.', $1) RETURNING cliente_id`, regoleACME)
	b.esegui(`INSERT INTO dominio_cliente (dominio, cliente_id) VALUES ('acme.example', $1)`, acme)
	fornitore := b.id(`INSERT INTO fornitore (ragione_sociale, tipo) VALUES ('Fornitore Esempio', 'processi') RETURNING fornitore_id`)
	t1 := b.id(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto) VALUES ($1, 'outlook', now(), 'RFQ 7120001') RETURNING thread_id`, acme)

	m1 := b.messaggio(t1, "RDO 400012345 - Richiesta offerta 7120001", "In allegato il disegno 7120001.", "mario.rossi@acme.example",
		"cliente", acme, uuid.Nil, "contatto")
	orfana := b.messaggio(uuid.Nil, "Anfrage_400012348 7120005", "", "ufficio.acquisti@acme.example", "cliente", acme, uuid.Nil, "dominio")
	// la risposta automatica: il triage ha risposto «ignora» senza estrarre (posta non di lavoro), e non ha
	// salvato niente
	auto := b.messaggio(t1, "Risposta automatica: RDO 400012345 - 7120001", "Sono fuori ufficio fino al 12/10.", "mario.rossi@acme.example",
		"cliente", acme, uuid.Nil, "contatto")
	// il disegno rimandato con lo stesso nome e un contenuto nuovo, non ancora deciso
	m3 := b.messaggio(t1, "R: RDO 400012345 - Richiesta offerta 7120001", "Disegno aggiornato.", "mario.rossi@acme.example",
		"cliente", acme, uuid.Nil, "contatto")
	mf := b.messaggio(t1, "Offerta 7120001", "La nostra offerta per 7120001", "vendite@fornitore-esempio.example", "fornitore", uuid.Nil, fornitore, "dominio")
	b.messaggio(uuid.Nil, "Newsletter", "", "info@sconosciuto.example", "sconosciuto", uuid.Nil, uuid.Nil, "")

	shaStp, shaPdf := fmt.Sprintf("%064x", 7120001), fmt.Sprintf("%064x", 7120011)
	stp := b.allegato(m1, 1, "7120001_PRT.stp", shaStp)
	pdf := b.allegato(m1, 2, "7120011A_1.pdf", shaPdf)
	// quello che l'ingest ha scritto all'arrivo di m1: la proposta del triage, i candidati di codice (il
	// riferimento della richiesta resta fuori dal confronto) e la proposta di file del PDF
	b.esegui(`INSERT INTO proposta_triage (messaggio_id, esito, confidenza, fonte) VALUES ($1, 'nuova_rfq', 60, 'deterministico')`, m1)
	b.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio, evidenza)
		VALUES ($1, 'RDO 400012345', 'riferimento_rfq', 'riferimento', 90, 'oggetto'),
		       ($1, '7120001', 'prodotto', 'famiglia', 80, 'oggetto'),
		       ($1, '7120077', 'prodotto', 'famiglia', 80, 'corpo')`, m1)
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', '7120011A', '1', 45, 'nome_file')`, pdf, t1)
	shaPdf2 := fmt.Sprintf("%064x", 7120012)
	b.allegato(m3, 1, "7120011A_1.pdf", shaPdf2)
	// l'orfana: «ignora» con un codice salvato (un mittente del dominio, letto e non proposto); la risposta
	// automatica: «ignora» senza niente
	b.esegui(`INSERT INTO proposta_triage (messaggio_id, esito, confidenza, fonte) VALUES ($1, 'ignora', 0, 'deterministico'), ($2, 'ignora', 0, 'deterministico')`,
		orfana, auto)
	b.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio, evidenza) VALUES ($1, '7120005', 'prodotto', 'famiglia', 80, 'oggetto')`, orfana)
	b.allegato(m1, 3, "image001.png", fmt.Sprintf("%064x", 1))
	b.allegato(mf, 1, "OFFERTA 7120001.pdf", fmt.Sprintf("%064x", 2))

	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, $3)`, shaPdf, fmt.Sprintf("%064x", 4),
		`{"testo_pdf": {"versione": 1, "estraibile": true,
			"frammenti": [{"pagina": 1, "zona": "basso_destra", "fonte": "nativo", "testo": "DISEGNO N. 7120011 PIASTRA"}],
			"cartiglio": [{"etichetta": "codice", "valore": "7120011", "fonte": "nativo", "zona": "basso_destra", "pagina": 1},
			{"etichetta": "titolo", "valore": "PIASTRA", "fonte": "nativo", "zona": "basso_destra", "pagina": 1}]}}`)

	prod := b.id(`INSERT INTO componente (thread_id, codice, tipo, confermato_da, descrizione) VALUES ($1, '7120001', 'finito', $2, 'SUPPORTO') RETURNING componente_id`, t1, utente)
	vite := b.id(`INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, '7120010', 'commerciale', $2) RETURNING componente_id`, t1, utente)
	b.esegui(`INSERT INTO componente (thread_id, codice, tipo, confermato_da, descrizione) VALUES ($1, '7120011', 'sciolto', $2, 'PIASTRA SP.6')`, t1, utente)
	// un commerciale che la persona ha tolto dalla distinta (A4.9): non e' una decisione da contare
	b.esegui(`INSERT INTO componente (thread_id, codice, tipo, confermato_da, descrizione, archiviato_il, archiviato_da, motivo_archiviazione)
		VALUES ($1, '7120013', 'commerciale', $2, 'DADO M8', now(), $2, 'doppione')`, t1, utente)
	b.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, fonte, confidenza, stato, componente_id, deciso_da, deciso_il)
		VALUES ($1, $2, $3, '#1', '7120001_PRT', '7120001_PRT', 'step', 70, 'duplicato', $4, $5, now()),
		       ($1, $2, $3, '#2', 'VITE TE M8X20 UNI 5739', '7120010', 'step', 70, 'confermata', $6, $5, now()),
		       ($1, $2, $3, '#3', 'NON DECISO', '7120099', 'step', 70, 'aperta', NULL, NULL, NULL)`, t1, stp, shaStp, prod, utente, vite)
	b.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '7120001', 'manuale', $2), ($1, '7120004', 'proposta_oggetto', $2)`, t1, utente)
	b.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, '7120098', 'proposta_oggetto')`, t1)
	b.esegui(`INSERT INTO documento (thread_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, 'disegno_2d', '7120011', 'A1', '7120011A_1.pdf', 'pdf', $2, 'DISEGNI\7120011_REV_A1.pdf', $3)`, t1, shaPdf, utente)

	prima := fotoDB(t, b.ctx, pool)
	in, err := Leggi(b.ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if !in.SolaLettura {
		t.Error("la transazione del censimento non e' di sola lettura")
	}
	if len(in.Clienti) != 1 || in.Clienti[0].ID != acme || in.Clienti[0].Nome != "ACME" {
		t.Errorf("clienti: %+v", in.Clienti)
	}
	if len(in.Messaggi) != 4 {
		t.Fatalf("mail del cliente: %d, attese 4 (le tre della RFQ e l'orfana; non il fornitore ne' lo sconosciuto): %+v", len(in.Messaggi), in.Messaggi)
	}
	for _, m := range in.Messaggi {
		if m.Cliente != acme || (m.Thread != t1 && m.Thread != uuid.Nil) {
			t.Errorf("mail attribuita male: %+v", m)
		}
		switch {
		case m.Oggetto == "RDO 400012345 - Richiesta offerta 7120001":
			if !slices.Equal(m.Allegati, []string{"7120001_PRT.stp", "7120011A_1.pdf", "image001.png"}) || m.ViaDominio ||
				!m.Interpretato || !m.Estratto || m.Troncato || !slices.Equal(m.Salvati, []string{"7120001", "7120077"}) {
				t.Errorf("la mail della RFQ: allegati %v, via dominio %v, letta all'arrivo %v con estrazione %v, tagliata %v, salvati %v",
					m.Allegati, m.ViaDominio, m.Interpretato, m.Estratto, m.Troncato, m.Salvati)
			}
		case m.Thread == uuid.Nil:
			if !m.ViaDominio || m.Mittente != "ufficio.acquisti@acme.example" || !m.Interpretato || !m.Estratto || !slices.Equal(m.Salvati, []string{"7120005"}) {
				t.Errorf("l'orfana, «ignora» con un codice salvato: %+v", m)
			}
		case strings.HasPrefix(m.Oggetto, "Risposta automatica"):
			if !m.Interpretato || m.Estratto || len(m.Salvati) != 0 {
				t.Errorf("la risposta automatica, «ignora» senza niente salvato: %+v", m)
			}
		default:
			if m.Interpretato || m.Estratto || !slices.Equal(m.Allegati, []string{"7120011A_1.pdf"}) {
				t.Errorf("il disegno rimandato: %+v", m)
			}
		}
	}
	if len(in.File) != 4 {
		t.Errorf("file del cliente: %+v", in.File)
	}
	contenuti := map[string]string{"7120001_PRT.stp": shaStp, "image001.png": fmt.Sprintf("%064x", 1)}
	for _, f := range in.File {
		deciso := f.Contenuto == shaPdf
		if f.Nome == "7120011A_1.pdf" {
			if f.Contenuto != shaPdf && f.Contenuto != shaPdf2 {
				t.Errorf("il contenuto del PDF: %+v", f)
			}
		} else if f.Contenuto != contenuti[f.Nome] {
			t.Errorf("il contenuto di %s: %+v", f.Nome, f)
		}
		if f.Deciso != deciso || (deciso && (f.Codice != "7120011" || f.Rev != "A1")) || f.Cliente != acme || f.Thread != t1 ||
			f.Proposto != deciso || (deciso && (f.CodiceProposto != "7120011A" || f.RevProposta != "1")) {
			t.Errorf("file %s: %+v", f.Nome, f)
		}
	}
	if len(in.Nodi) != 3 || len(in.Campi) != 1 || in.Campi[0] != (Campo{Cliente: acme, Thread: t1, Contenuto: shaPdf, Etichetta: "codice", Valore: "7120011"}) {
		t.Errorf("nodi %d, campi %+v", len(in.Nodi), in.Campi)
	}
	if len(in.Pezzi) != 3 {
		t.Fatalf("pezzi (non quello archiviato): %+v", in.Pezzi)
	}
	for _, p := range in.Pezzi {
		switch p.Codice {
		case "7120001":
			if p.Deciso != db.TipoComponenteFinito || !slices.Equal(p.Nomi, []string{"SUPPORTO", "7120001_PRT"}) {
				t.Errorf("il prodotto: %+v", p)
			}
		case "7120010":
			if p.Deciso != db.TipoComponenteCommerciale || !slices.Equal(p.Nomi, []string{"VITE TE M8X20 UNI 5739"}) || p.Proposto != "" {
				t.Errorf("il commerciale, con il nome del nodo accettato e senza proposta (4.4a.1): %+v", p)
			}
		}
	}
	var decisi []string
	for _, d := range in.Decisi {
		decisi = append(decisi, d.Codice)
	}
	if !slices.Equal(decisi, []string{"7120001", "7120004", "7120011"}) {
		t.Errorf("codici decisi (i prodotti confermati e i documenti, non 7120098): %v", decisi)
	}
	if len(in.DominiNonCensiti) != 1 || in.DominiNonCensiti[0].Testo != "sconosciuto.example" || in.DominiNonCensiti[0].N != 1 {
		t.Errorf("domini non censiti: %+v", in.DominiNonCensiti)
	}

	// i fatti arrivano al Go interi, tranne i frammenti del testo: li legge la classificazione, versione compresa
	cartigli, err := db.New(pool).CensimentoCartigli(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cartigli) != 1 || strings.Contains(string(cartigli[0].Fatti), "frammenti") || !strings.Contains(string(cartigli[0].Fatti), `"versione": 1`) {
		t.Errorf("i fatti del cartiglio: %+v", cartigli)
	}

	c, err := Raccogli(b.ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	s := c.Schede[0]
	if s.NMessaggi != 4 || s.NFile != 3 || s.Nome.File != 2 || s.Firme.Commerciali != 1 || s.Minuteria.NonProposte != 1 || s.Minuteria.Proposte {
		t.Errorf("scheda: mail %d file %d (nomi %d) commerciali %d minuteria %+v", s.NMessaggi, s.NFile, s.Nome.File, s.Firme.Commerciali, s.Minuteria)
	}
	if a, ok := aggiuntaDi(s.Code, "_PRT"); !ok || a.N != 2 || a.Alias {
		t.Errorf("_PRT dopo 7120001 nel nome e nello STEP, una sola RFQ: %+v", s.Code)
	}
	if a, ok := aggiuntaDi(s.Code, "A_1"); !ok || a.N != 1 {
		t.Errorf("A_1 dopo 7120011: %+v", s.Code)
	}
	if n := s.Nome; n.Confrontati != 1 || n.CodiceUguale != 0 || len(n.Diversi) != 1 || n.Diversi[0].Deciso != "7120011 rev A1" ||
		n.ConProposta != 1 || n.PropostaUguale != 1 {
		t.Errorf("letture del nome: %+v", n)
	}
	if m := s.Mail; m.Interpretati != 2 || m.SenzaEstrazione != 1 || m.Salvati != 3 || m.Persi != 1 || !slices.Equal(m.EsempiPersi, []string{"7120077"}) {
		t.Errorf("accanto all'arrivo: %+v", m)
	}

	if d := diverse(prima, fotoDB(t, b.ctx, pool)); len(d) > 0 {
		t.Errorf("il censimento ha scritto in: %v", d)
	}
}
