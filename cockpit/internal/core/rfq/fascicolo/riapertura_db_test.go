//go:build integrazione

// L4 — Smistamento F7 (addendum A5.15): il comando U5 contro PostgreSQL vero. L'anteprima non scrive niente;
// l'applicazione riapre, nelle RFQ in corso, i nodi agganciati per codice senza una persona (F-A) e gli archi
// chiusi in automatico che li toccavano (F-B), con la storia, e tiene la radice di prima dello Smistamento
// (F-C); le RFQ congelate e chiuse restano intatte; una RFQ in revisione riapre la bozza e le versioni
// congelate e le istantanee restano identiche riga per riga; una seconda esecuzione non trova niente; una RFQ
// congelata dopo l'anteprima si salta; una RFQ in cui le righe scritte non tornano si annulla da sola.
//
// Prove dell'addendum: 216, 217 (anche con -rfq), 218, 219, piu' la RFQ in revisione, il conteggio che non
// torna e la radice di prima tenuta. La parte della riga di comando (il database giusto, P38) sta in app/runtime.

package fascicolo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// altraRfq e' un'altra RFQ dello stesso cliente, nello stesso database, con la fase FATTIBILITA aperta. Il
// contatore dei contenuti parte piu' avanti: gli sha e le chiavi esterne non si scontrano con quelli di b.
func (b *banco) altraRfq(nome string) *banco {
	b.t.Helper()
	c := *b
	c.n = b.n + 10000*(1+int(uno[int64](b, `SELECT count(*) FROM thread_offerta`)))
	cliente := uno[uuid.UUID](b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, b.thread)
	c.thread = uno[uuid.UUID](b, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa)
		VALUES ($1, 'outlook', now(), $2) RETURNING thread_id`, cliente, `PROVAA4\WIP\`+nome)
	c.esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now() - interval '1 hour')`, c.thread)
	return &c
}

// agganciaPerCodice riscrive un nodo com'era prima dello Smistamento: agganciato al componente da una lettura,
// per il codice, senza una persona (F-A; per la radice dello STEP strutturale e' la forma di prima, F-C).
func (b *banco) agganciaPerCodice(a db.Allegato, chiave string, comp uuid.UUID) {
	b.t.Helper()
	tag, err := b.p.Exec(b.ctx, `UPDATE componente_proposta SET stato = 'duplicato', componente_id = $4, deciso_da = NULL,
		deciso_il = now() - interval '30 days' WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`, b.thread, a.AllegatoID, chiave, comp)
	if err != nil || tag.RowsAffected() != 1 {
		b.t.Fatalf("aggancio per codice di %s: %v (%d righe)", chiave, err, tag.RowsAffected())
	}
}

// decisoDaUnaPersona riscrive un nodo come deciso da una persona (duplicato del componente).
func (b *banco) decisoDaUnaPersona(a db.Allegato, chiave string, comp uuid.UUID) {
	b.t.Helper()
	b.esegui(`UPDATE componente_proposta SET stato = 'duplicato', componente_id = $4, deciso_da = $5, deciso_il = now()
		WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`, b.thread, a.AllegatoID, chiave, comp, b.utente)
}

// arcoAutomatico riscrive un arco come chiuso da un automatismo di prima (senza chi l'ha deciso).
func (b *banco) arcoAutomatico(a db.Allegato, padre, figlio, stato, nota string) {
	b.t.Helper()
	tag, err := b.p.Exec(b.ctx, `UPDATE relazione_proposta SET stato = $5::stato_proposta, nota = NULLIF($6, ''), deciso_da = NULL,
		deciso_il = now() - interval '30 days' WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = $3 AND figlio_chiave = $4`,
		b.thread, a.AllegatoID, padre, figlio, stato, nota)
	if err != nil || tag.RowsAffected() != 1 {
		b.t.Fatalf("arco automatico %s>%s: %v", padre, figlio, err)
	}
}

// righe: "chiave:stato:codice del componente" dei nodi di un file, in ordine di chiave.
func (b *banco) righe(a db.Allegato) string {
	return leggibile(uno[string](b, `SELECT coalesce(string_agg(cp.chiave || ':' || cp.stato || ':' || coalesce(c.codice, '-'), ' ' ORDER BY cp.chiave), '')
		FROM componente_proposta cp LEFT JOIN componente c ON c.componente_id = cp.componente_id
		WHERE cp.thread_id = $1 AND cp.allegato_id = $2`, b.thread, a.AllegatoID))
}

// archiDi: "padre>figlio:stato" degli archi di un file.
func (b *banco) archiDi(a db.Allegato) string {
	return uno[string](b, `SELECT coalesce(string_agg(padre_chiave || '>' || figlio_chiave || ':' || stato, ' ' ORDER BY padre_chiave, figlio_chiave), '')
		FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2`, b.thread, a.AllegatoID)
}

// storia e' l'ultima voce di evidenza.storia di un nodo (vuota se non c'e').
func (b *banco) storia(a db.Allegato, chiave string) map[string]any {
	b.t.Helper()
	var raw []byte
	b.riga(`SELECT coalesce(evidenza -> 'storia', '[]'::jsonb) FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3`,
		&raw, b.thread, a.AllegatoID, chiave)
	var s []map[string]any
	if err := json.Unmarshal(raw, &s); err != nil || len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}

// impronteDelDatabase e' il contenuto di ogni tabella del database, riga per riga: quante righe e l'md5 di
// tutte. Dice «non e' cambiato niente» senza sapere com'e' fatto lo schema.
func impronteDelDatabase(t *testing.T, b *banco) map[string]string {
	t.Helper()
	righe, err := b.p.Query(b.ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var tabelle []string
	for righe.Next() {
		var n string
		if err := righe.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tabelle = append(tabelle, n)
	}
	righe.Close()
	out := map[string]string{}
	for _, tab := range tabelle {
		out[tab] = uno[string](b, fmt.Sprintf(`SELECT count(*) || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '') FROM %q x`, tab))
	}
	return out
}

func stessoDatabase(t *testing.T, cosa string, prima, dopo map[string]string) {
	t.Helper()
	if len(prima) != len(dopo) {
		t.Errorf("%s: le tabelle sono cambiate (%d prima, %d dopo)", cosa, len(prima), len(dopo))
	}
	for tab, p := range prima {
		if d := dopo[tab]; d != p {
			t.Errorf("%s: la tabella %s e' cambiata: %s → %s", cosa, tab, p, d)
		}
	}
}

// laBomNonCambia sono componente, componente_relazione e documento della RFQ, riga per riga: il comando
// non li scrive mai.
func (b *banco) laBom() string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY componente_id) FROM componente x WHERE thread_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY padre_id, figlio_id) FROM componente_relazione x WHERE thread_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY documento_id) FROM documento x WHERE thread_id = $1))`, b.thread)
}

// leVersioni sono tutte le versioni della RFQ e le loro quattro istantanee, riga per riga.
func (b *banco) leVersioni() string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY numero) FROM bom_versione x WHERE thread_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY x.bom_versione_id, x.componente_id) FROM bom_versione_componente x WHERE thread_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY x.bom_versione_id, x.padre_id, x.figlio_id) FROM bom_versione_relazione x
		  JOIN bom_versione v USING (bom_versione_id) WHERE v.thread_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY x.bom_versione_id, x.documento_id) FROM bom_versione_documento x
		  JOIN bom_versione v USING (bom_versione_id) WHERE v.thread_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY x.bom_versione_id, x.componente_id, x.tipo) FROM bom_versione_deroga x
		  JOIN bom_versione v USING (bom_versione_id) WHERE v.thread_id = $1))`, b.thread)
}

// riapri lancia il comando sul database di prova: anteprima o applicazione, tutte le RFQ.
func (b *banco) riapri(applica bool, dopo func(fascicolo.RapportoRiapertura) error) (fascicolo.RapportoRiapertura, error) {
	return fascicolo.RiapriAgganci(context.Background(), b.p, nil, applica, dopo)
}

func rfqDi(t *testing.T, r fascicolo.RapportoRiapertura, thread uuid.UUID) fascicolo.RfqRiapertura {
	t.Helper()
	for _, x := range r.Rfq {
		if x.ThreadID == thread {
			return x
		}
	}
	t.Fatalf("la RFQ %s non e' nel rapporto: %+v", thread, r.Rfq)
	return fascicolo.RfqRiapertura{}
}

func esclusa(r fascicolo.RapportoRiapertura, thread uuid.UUID) string {
	for _, x := range r.Escluse {
		if x.ThreadID == thread {
			return x.Motivo
		}
	}
	return ""
}

// scenaInCorso: la RFQ del banco, com'era prima dello Smistamento.
//
//	P1 (finito) → B, C, D nella working; lo STEP strutturale di P1 (S) e' la forma di prima: la radice #1
//	agganciata a P1 da una lettura (F-C, si tiene); #2 (B) e #3 (C) agganciati per codice (F-A, nell'autorita');
//	#4 (D, sotto C) deciso da una persona. Gli archi #1 → #2, #1 → #3, #3 → #4 chiusi in automatico (F-B).
//	Un secondo STEP (G), guida: #1 (C) agganciato per codice (F-A, guida); #2 (B) deciso da una persona e #3
//	(D) deciso da una persona, con l'arco #2 → #3 chiuso in automatico fra due decisioni (resta); #4 chiuso
//	per sostituzione (F-E, resta).
type scenaInCorso struct {
	c    map[string]uuid.UUID
	s, g db.Allegato
	fS   string
	fG   string
}

func (b *banco) scenaInCorso() scenaInCorso {
	b.t.Helper()
	var sc scenaInCorso
	sc.c = map[string]uuid.UUID{"P1": b.comp("P1", db.TipoComponenteFinito)}
	for _, k := range []string{"B", "C", "D"} {
		sc.c[k] = b.comp(k, db.TipoComponenteSciolto)
	}
	b.arco(sc.c["P1"], sc.c["B"], 1)
	b.arco(sc.c["P1"], sc.c["C"], 1)
	b.arco(sc.c["C"], sc.c["D"], 1)
	fS := fattiSTEP{nodi: []string{"#1=P1", "#2=B", "#3=C", "#4=D"}, archi: []string{"#1>#2", "#1>#3", "#3>#4"}}
	sc.fS = fS.json()
	doc, s := b.stepDelProdotto(sc.c["P1"], codiceDi("P1"), fS)
	sc.s = s
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, sc.c["P1"], doc)
	b.applica(s, sc.fS)
	b.agganciaPerCodice(s, "#1", sc.c["P1"])
	b.agganciaPerCodice(s, "#2", sc.c["B"])
	b.agganciaPerCodice(s, "#3", sc.c["C"])
	b.decisoDaUnaPersona(s, "#4", sc.c["D"])
	b.arcoAutomatico(s, "#1", "#2", "duplicato", "")
	b.arcoAutomatico(s, "#1", "#3", "duplicato", "")
	b.arcoAutomatico(s, "#3", "#4", "scartata", "padre e figlio sono lo stesso componente: un pezzo non contiene se stesso")

	fG := fattiSTEP{nodi: []string{"#1=C", "#2=B", "#3=D", "#4=F"}, archi: []string{"#1>#2", "#2>#3", "#1>#4"}}
	sc.fG = fG.json()
	sc.g = b.allegatoStep("guida.stp", strings.Repeat("9", 64))
	b.applica(sc.g, sc.fG)
	b.agganciaPerCodice(sc.g, "#1", sc.c["C"])
	b.decisoDaUnaPersona(sc.g, "#2", sc.c["B"])
	b.decisoDaUnaPersona(sc.g, "#3", sc.c["D"])
	b.arcoAutomatico(sc.g, "#2", "#3", "duplicato", "")
	b.esegui(`UPDATE componente_proposta SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = 'superata da guida_2.stp'
		WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#4'`, b.thread, sc.g.AllegatoID)
	return sc
}

// congelata e' una RFQ con la BOM congelata nella V1 e, dopo, un nodo agganciato per codice in uno STEP arrivato
// dopo (le proposte nascono anche con la BOM congelata).
func (b *banco) congelataConUnAggancio() db.Allegato {
	b.t.Helper()
	r := b.rfqCongelabile()
	_, err := b.congela("prima baseline")
	ok(b.t, err)
	a := b.allegatoStep("tardi.stp", strings.Repeat("7", 64))
	b.applica(a, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	b.agganciaPerCodice(a, "#2", r.f1)
	b.arcoAutomatico(a, "#1", "#2", "duplicato", "")
	return a
}

// ---------------------------------------------------------------- prove

// Prova 216 (A5.15.2): l'anteprima non scrive niente. Ogni tabella del database ha le stesse righe prima e
// dopo, e la transazione in cui legge e' in sola lettura (lo dice il database). Il rapporto dice che cosa
// l'applicazione farebbe.
func TestIlComandoInAnteprimaNonScrive(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaInCorso()
	prima := impronteDelDatabase(t, b)
	var passi int
	r, err := b.riapri(false, func(fascicolo.RapportoRiapertura) error { passi++; return nil })
	ok(t, err)
	stessoDatabase(t, "anteprima", prima, impronteDelDatabase(t, b))
	if r.Modalita != fascicolo.ModalitaAnteprima || !r.SolaLettura {
		t.Errorf("anteprima: modalita %q, sola lettura %v", r.Modalita, r.SolaLettura)
	}
	x := rfqDi(t, r, b.thread)
	if x.Esito != fascicolo.EsitoAnteprima || len(x.Forme.Agganci) != 3 || len(x.Forme.Archi) != 3 || x.Forme.Effetto.FigliDaConfermare != 2 ||
		len(x.Forme.RadiciTenute) != 1 || x.Forme.ChiusureAutomatiche.Nodi != 1 || x.NodiRiaperti != 0 {
		t.Errorf("anteprima della RFQ: %+v", x)
	}
	if x.Forme.Effetto.Frase != "il gate si fermerà su 2 figli diretti da confermare in 1 file autorizzato" {
		t.Errorf("effetto: %q", x.Forme.Effetto.Frase)
	}
	// il rapporto passa tre volte: prima di cominciare (il percorso si puo' scrivere), dopo la RFQ, alla fine
	if passi != 3 || r.Fine == nil || len(r.Avvisi) == 0 || !strings.Contains(strings.Join(r.Avvisi, " "), "F10/F11") {
		t.Errorf("rapporto: %d passi, fine %v, avvisi %v", passi, r.Fine, r.Avvisi)
	}
	if got := b.righe(sc.s); got != "#1:duplicato:P1 #2:duplicato:B #3:duplicato:C #4:duplicato:D" {
		t.Errorf("l'anteprima ha toccato i nodi: %s", got)
	}
}

// Prova 216 (A5.15.2), senza RFQ nel perimetro: l'anteprima dice «sola lettura» anche quando non legge
// nessuna RFQ (qui -rfq di una RFQ congelata, esclusa), e lo dice gia' nel primo rapporto, prima di qualunque
// RFQ: la sola lettura si misura su una transazione come quelle dell'anteprima (ReadOnly) aperta prima del
// perimetro, non alla prima RFQ e non sul pool (qui b.p, scrivibile: il lucchetto del pool e' del runtime).
func TestLAnteprimaSenzaRfqNelPerimetroESolaLettura(t *testing.T) {
	b := nuovoBanco(t)
	b.congelataConUnAggancio()
	prima := impronteDelDatabase(t, b)
	var primo *fascicolo.RapportoRiapertura
	r, err := fascicolo.RiapriAgganci(context.Background(), b.p, &b.thread, false, func(x fascicolo.RapportoRiapertura) error {
		if primo == nil {
			primo = &x
		}
		return nil
	})
	ok(t, err)
	stessoDatabase(t, "anteprima senza perimetro", prima, impronteDelDatabase(t, b))
	if !r.SolaLettura || primo == nil || !primo.SolaLettura || len(primo.Rfq) != 0 || len(primo.Escluse) != 0 {
		t.Errorf("sola lettura: rapporto %v, primo passo %+v", r.SolaLettura, primo)
	}
	if len(r.Rfq) != 0 || r.Totali.EscluseCongelate != 1 || esclusa(r, b.thread) == "" {
		t.Errorf("perimetro: rfq %+v, escluse %+v", r.Rfq, r.Escluse)
	}
}

// Prova 217 (A5.15.2), con -rfq: il comando lavora su una RFQ sola. Con due RFQ del perimetro, ciascuna con un
// aggancio per codice (F-A), e una RFQ congelata, l'anteprima e l'applicazione con -rfq della seconda vedono
// solo lei: il rapporto ha una RFQ, il perimetro e' 1 e non ci sono escluse; la seconda si riapre, la prima e
// la congelata restano identiche riga per riga. -rfq di una RFQ che non c'e' e' un errore che lo dice, prima
// di qualunque rapporto e senza una scrittura. E' la voce che limita le scritture sui dati veri: se il filtro
// si perdesse, «-riapri-agganci <db> -rfq X» riaprirebbe in silenzio tutto il perimetro.
func TestIlComandoConRfqLavoraSoloSuQuellaRfq(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaInCorso()
	altra := b.altraRfq("altra")
	ra := altra.rfqCongelabile()
	aa := altra.allegatoStep("a.stp", strings.Repeat("1", 64))
	altra.applica(aa, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	altra.agganciaPerCodice(aa, "#2", ra.f1)
	altra.arcoAutomatico(aa, "#1", "#2", "duplicato", "")
	cong := b.altraRfq("congelata")
	cong.congelataConUnAggancio()
	primaPrima, congPrima := impronteRfq(t, b), impronteRfq(t, cong)
	ctx := context.Background()

	for _, applica := range []bool{false, true} {
		r, err := fascicolo.RiapriAgganci(ctx, b.p, &altra.thread, applica, nil)
		ok(t, err)
		if len(r.Rfq) != 1 || r.Rfq[0].ThreadID != altra.thread || r.Totali.NelPerimetro != 1 || len(r.Escluse) != 0 ||
			r.Totali.EscluseCongelate != 0 || r.Totali.Agganci != 1 || r.Totali.Archi != 1 {
			t.Errorf("-rfq (applica %v): il rapporto deve avere solo la RFQ chiesta: rfq %+v, escluse %+v, totali %+v", applica, r.Rfq, r.Escluse, r.Totali)
		}
		if applica && (r.Rfq[0].Esito != fascicolo.EsitoRiaperta || r.Totali.NodiRiaperti != 1 || r.Totali.ArchiRiaperti != 1) {
			t.Errorf("-rfq in applicazione: %+v", r.Rfq[0])
		}
	}
	if got := altra.righe(aa); got != "#1:aperta:- #2:aperta:-" {
		t.Errorf("la RFQ chiesta: %s", got)
	}
	if got := altra.archiDi(aa); got != "#1>#2:aperta" {
		t.Errorf("l'arco della RFQ chiesta: %s", got)
	}
	stessoDatabase(t, "-rfq: l'altra RFQ del perimetro", primaPrima, impronteRfq(t, b))
	stessoDatabase(t, "-rfq: la RFQ congelata", congPrima, impronteRfq(t, cong))
	if got := b.righe(sc.s); got != "#1:duplicato:P1 #2:duplicato:B #3:duplicato:C #4:duplicato:D" {
		t.Errorf("l'altra RFQ del perimetro e' stata toccata: %s", got)
	}

	// una RFQ che non c'e': errore, nessun rapporto, nessuna scrittura
	prima := impronteDelDatabase(t, b)
	ignota := uuid.New()
	for _, applica := range []bool{false, true} {
		passi := 0
		_, err := fascicolo.RiapriAgganci(ctx, b.p, &ignota, applica, func(fascicolo.RapportoRiapertura) error { passi++; return nil })
		if err == nil || !strings.Contains(err.Error(), ignota.String()) || !strings.Contains(err.Error(), "non c'è in questo database") {
			t.Errorf("-rfq di una RFQ che non c'e' (applica %v): %v", applica, err)
		}
		if passi != 0 {
			t.Errorf("-rfq di una RFQ che non c'e' (applica %v): il rapporto e' passato %d volte", applica, passi)
		}
	}
	stessoDatabase(t, "-rfq di una RFQ che non c'e'", prima, impronteDelDatabase(t, b))
}

// Prova 217 (A5.15.1, A5.15.3): l'applicazione riapre gli agganci per codice nelle RFQ in corso. Nella RFQ in
// corso i nodi F-A tornano aperti, senza componente, con lo stato di prima nella storia; gli archi F-B tornano
// aperti; la radice di prima dello Smistamento (F-C), un nodo deciso da una persona, un arco automatico fra due
// decisioni e una chiusura per sostituzione (F-E) restano; la BOM working non cambia. La RFQ congelata e la RFQ
// chiusa sono escluse e contate, e le loro righe restano com'erano. Dopo una rilettura dello STEP (ApplicaStruttura)
// la storia c'e' ancora e nessun nodo si riaggancia.
func TestIlComandoRiapreGliAgganciNelleRfqInCorso(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaInCorso()
	cong := b.altraRfq("congelata")
	aCong := cong.congelataConUnAggancio()
	chiusa := b.altraRfq("chiusa")
	aChiusa := chiusa.allegatoStep("chiusa.stp", strings.Repeat("5", 64))
	chiusa.applica(aChiusa, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	chiusa.agganciaPerCodice(aChiusa, "#2", chiusa.comp("Y1", db.TipoComponenteSciolto))
	chiusa.passaA("PERSA")
	if s := uno[string](b, `SELECT stato::text FROM thread_offerta WHERE thread_id = $1`, chiusa.thread); s != "CHIUSA" {
		t.Fatalf("la RFQ persa e' %s", s)
	}
	bomPrima := b.laBom()
	congPrima, chiusaPrima := impronteRfq(t, cong), impronteRfq(t, chiusa)

	r, err := b.riapri(true, nil)
	ok(t, err)
	x := rfqDi(t, r, b.thread)
	if x.Esito != fascicolo.EsitoRiaperta || x.NodiRiaperti != 3 || x.ArchiRiaperti != 3 {
		t.Fatalf("la RFQ in corso: %+v", x)
	}
	if got := b.righe(sc.s); got != "#1:duplicato:P1 #2:aperta:- #3:aperta:- #4:duplicato:D" {
		t.Errorf("nodi dello STEP strutturale: %s (la radice di prima e il nodo deciso restano, gli agganci si riaprono)", got)
	}
	if got := b.righe(sc.g); got != "#1:aperta:- #2:duplicato:B #3:duplicato:D #4:scartata:-" {
		t.Errorf("nodi della guida: %s", got)
	}
	if got := b.archiDi(sc.s); got != "#1>#2:aperta #1>#3:aperta #3>#4:aperta" {
		t.Errorf("archi dello STEP strutturale: %s", got)
	}
	if got := b.archiDi(sc.g); !strings.Contains(got, "#2>#3:duplicato") {
		t.Errorf("l'arco automatico fra due decisioni resta: %s", got)
	}
	// la storia: com'era, e chi l'ha riaperto
	s := b.storia(sc.s, "#2")
	if s == nil || s["evento"] != "riaperta_u5" || s["stato"] != "duplicato" || s["componente_id"] != sc.c["B"].String() ||
		s["codice_componente"] != codiceDi("B") || s["da"] != "cockpit -riapri-agganci" || s["deciso_il"] == nil {
		t.Errorf("storia del nodo riaperto: %v", s)
	}
	if n := uno[string](b, `SELECT coalesce(nota, '') || '|' || (deciso_il IS NULL)::text || '|' || (deciso_da IS NULL)::text FROM componente_proposta
		WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`, b.thread, sc.s.AllegatoID); n !=
		"agganciata per codice a "+codiceDi("B")+" prima dello Smistamento, senza una persona: da confermare|true|true" {
		t.Errorf("nota del nodo riaperto: %s", n)
	}
	var arco []map[string]any
	b.riga(`SELECT evidenza -> 'storia' FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = '#3' AND figlio_chiave = '#4'`,
		&arco, b.thread, sc.s.AllegatoID)
	if len(arco) != 1 || arco[0]["evento"] != "riaperta_u5" || arco[0]["stato"] != "scartata" || !strings.HasPrefix(fmt.Sprint(arco[0]["nota"]), "padre e figlio") {
		t.Errorf("storia dell'arco riaperto: %v", arco)
	}
	if b.laBom() != bomPrima {
		t.Errorf("la BOM working (componenti, archi, documenti) e' cambiata")
	}
	// le escluse, contate, e intatte
	if esclusa(r, cong.thread) != fascicolo.EsclusaCongelata || esclusa(r, chiusa.thread) != fascicolo.EsclusaChiusa ||
		r.Totali.EscluseCongelate != 1 || r.Totali.EscluseChiuse != 1 || r.Totali.NelPerimetro != 1 {
		t.Errorf("escluse: %+v, totali %+v", r.Escluse, r.Totali)
	}
	if got := cong.righe(aCong); got != "#1:aperta:- #2:duplicato:F1" {
		t.Errorf("RFQ congelata: %s", got)
	}
	if got := chiusa.righe(aChiusa); !strings.Contains(got, "#2:duplicato:") {
		t.Errorf("RFQ chiusa: %s", got)
	}
	stessoDatabase(t, "RFQ congelata", congPrima, impronteRfq(t, cong))
	stessoDatabase(t, "RFQ chiusa", chiusaPrima, impronteRfq(t, chiusa))

	// la rilettura dello STEP: la storia resta, nessun aggancio torna, la radice di prima resta
	b.applica(sc.s, sc.fS)
	if got := b.righe(sc.s); got != "#1:duplicato:P1 #2:aperta:- #3:aperta:- #4:duplicato:D" {
		t.Errorf("dopo la rilettura: %s", got)
	}
	if s := b.storia(sc.s, "#2"); s == nil || s["evento"] != "riaperta_u5" {
		t.Errorf("la storia non e' sopravvissuta alla rilettura: %v", s)
	}
	// e il gate adesso conta i figli diretti da confermare (erano gia' contati come agganci: niente cambia nel numero)
	if g := b.strutturali(); g.NFigliDaDecidere != 2 {
		t.Errorf("il gate: %+v, attesi 2 figli diretti da decidere", g)
	}
}

// impronteRfq sono le righe di una RFQ nelle tabelle che il comando potrebbe toccare, riga per riga.
func impronteRfq(t *testing.T, b *banco) map[string]string {
	t.Helper()
	out := map[string]string{"bom": b.laBom(), "versioni": b.leVersioni()}
	for _, tab := range []string{"componente_proposta", "relazione_proposta", "rimozione_proposta", "documento_proposta"} {
		out[tab] = uno[string](b, fmt.Sprintf(`SELECT count(*) || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '') FROM %s x WHERE thread_id = $1`, tab), b.thread)
	}
	return out
}

// Una RFQ in revisione (una V1 congelata e la V2 in bozza) e' dentro il perimetro (Domanda 6 = A): gli
// agganci della bozza si riaprono, e la V1, la V2, bom_versione e le quattro istantanee sono identiche prima e
// dopo, riga per riga. Il rapporto la dice a parte.
func TestIlComandoInRevisioneRiapreSoloLaBozza(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	v1, err := b.congela("prima baseline")
	ok(t, err)
	_, err = b.apri(nil, "il cliente cambia la staffa")
	ok(t, err)
	a := b.allegatoStep("revisione.stp", strings.Repeat("3", 64))
	b.applica(a, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	b.agganciaPerCodice(a, "#2", r.f1)
	b.arcoAutomatico(a, "#1", "#2", "duplicato", "")
	versioniPrima, v1Prima, bomPrima := b.leVersioni(), b.impronta(v1.Versione.BomVersioneID), b.laBom()

	ant, err := b.riapri(false, nil)
	ok(t, err)
	if x := rfqDi(t, ant, b.thread); !x.InRevisione || x.Versione != "V2 bozza" || ant.Totali.InRevisione != 1 || len(x.Forme.Agganci) != 1 {
		t.Errorf("anteprima della RFQ in revisione: %+v", x)
	}
	rap, err := b.riapri(true, nil)
	ok(t, err)
	if x := rfqDi(t, rap, b.thread); !x.InRevisione || x.Esito != fascicolo.EsitoRiaperta || x.NodiRiaperti != 1 || x.ArchiRiaperti != 1 {
		t.Errorf("applicazione nella RFQ in revisione: %+v", x)
	}
	if got := b.righe(a); got != "#1:aperta:- #2:aperta:-" {
		t.Errorf("la bozza: %s", got)
	}
	if got := b.leVersioni(); got != versioniPrima {
		t.Errorf("le versioni e le istantanee sono cambiate:\nprima %s\ndopo  %s", versioniPrima, got)
	}
	if got := b.impronta(v1.Versione.BomVersioneID); got != v1Prima {
		t.Errorf("la V1 congelata e' cambiata")
	}
	if b.laBom() != bomPrima {
		t.Errorf("la working e' cambiata")
	}
}

// Prova 218 (A5.15.2): il comando e' idempotente. Una seconda applicazione, e una seconda anteprima, non
// trovano niente da riaprire e non cambiano niente nel database.
func TestIlComandoEIdempotente(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaInCorso()
	_, err := b.riapri(true, nil)
	ok(t, err)
	dopo := impronteDelDatabase(t, b)
	for _, applica := range []bool{true, false} {
		r, err := b.riapri(applica, nil)
		ok(t, err)
		if r.Totali.Agganci != 0 || r.Totali.Archi != 0 || r.Totali.NodiRiaperti != 0 || r.Totali.Riaperte != 0 {
			t.Errorf("seconda esecuzione (applica %v): %+v", applica, r.Totali)
		}
		if x := rfqDi(t, r, b.thread); applica && x.Esito != fascicolo.EsitoNiente {
			t.Errorf("seconda applicazione: esito %q", x.Esito)
		}
		// la radice di prima si tiene, sempre
		if r.Totali.RadiciTenute != 1 {
			t.Errorf("la radice di prima: %+v", r.Totali)
		}
		stessoDatabase(t, fmt.Sprintf("seconda esecuzione (applica %v)", applica), dopo, impronteDelDatabase(t, b))
	}
}

// Prova 219 (A5.15.2): una RFQ congelata dopo l'anteprima si salta. Congelata prima che l'applicazione parta,
// resta fuori dal perimetro; congelata mentre l'applicazione lavora su un'altra RFQ (fra l'elenco e la sua
// transazione), il perimetro ricontrollato dentro la transazione la salta. In tutti e due i casi le sue righe
// restano com'erano, e il comando non fallisce. Lo stesso per una RFQ chiusa mentre il comando lavora, che il
// rapporto dice con lo stato che ha adesso.
func TestIlComandoSaltaUnaRfqCongelataDopoLAnteprima(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaInCorso()
	primaX := b.altraRfq("prima")
	rx := primaX.rfqCongelabile()
	ax := primaX.allegatoStep("x.stp", strings.Repeat("4", 64))
	primaX.applica(ax, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	primaX.agganciaPerCodice(ax, "#2", rx.f1)

	ant, err := b.riapri(false, nil)
	ok(t, err)
	if x := rfqDi(t, ant, primaX.thread); len(x.Forme.Agganci) != 1 {
		t.Fatalf("anteprima: %+v", x)
	}
	_, err = primaX.congela("dopo l'anteprima")
	ok(t, err)
	r, err := b.riapri(true, nil)
	ok(t, err)
	if esclusa(r, primaX.thread) != fascicolo.EsclusaCongelata {
		t.Errorf("la RFQ congelata dopo l'anteprima non e' esclusa: %+v", r.Escluse)
	}
	if got := primaX.righe(ax); got != "#1:aperta:- #2:duplicato:F1" {
		t.Errorf("RFQ congelata dopo l'anteprima: %s", got)
	}

	// congelata fra l'elenco e la sua transazione: dopo la prima RFQ, mentre il comando lavora
	c := nuovoBanco(t)
	c.scenaInCorso()
	durante := c.altraRfq("durante")
	rd := durante.rfqCongelabile()
	ad := durante.allegatoStep("d.stp", strings.Repeat("2", 64))
	durante.applica(ad, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	durante.agganciaPerCodice(ad, "#2", rd.f1)
	congelata := false
	r, err = c.riapri(true, func(x fascicolo.RapportoRiapertura) error {
		if len(x.Rfq) == 1 && !congelata {
			congelata = true
			_, err := durante.congela("mentre il comando lavora")
			return err
		}
		return nil
	})
	ok(t, err)
	x := rfqDi(t, r, durante.thread)
	if x.Esito != fascicolo.EsitoSaltata || !strings.Contains(x.Motivo, "congelata nella V1 dopo l'anteprima") || r.Totali.Saltate != 1 || x.NodiRiaperti != 0 {
		t.Errorf("RFQ congelata durante il comando: %+v, totali %+v", x, r.Totali)
	}
	if got := durante.righe(ad); got != "#1:aperta:- #2:duplicato:F1" {
		t.Errorf("RFQ congelata durante il comando: %s", got)
	}
	if rfqDi(t, r, c.thread).Esito != fascicolo.EsitoRiaperta {
		t.Errorf("la prima RFQ doveva essere riaperta")
	}

	// chiusa fra l'elenco e la sua transazione: saltata, e il rapporto dice lo stato riletto dopo BloccaThread
	// (CHIUSA), non quello dell'elenco
	e := nuovoBanco(t)
	e.scenaInCorso()
	persa := e.altraRfq("persa")
	ap := persa.allegatoStep("p.stp", strings.Repeat("6", 64))
	persa.applica(ap, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	persa.agganciaPerCodice(ap, "#2", persa.comp("Y1", db.TipoComponenteSciolto))
	chiusa := false
	r, err = e.riapri(true, func(x fascicolo.RapportoRiapertura) error {
		if len(x.Rfq) == 1 && !chiusa {
			chiusa = true
			persa.passaA("PERSA")
		}
		return nil
	})
	ok(t, err)
	x = rfqDi(t, r, persa.thread)
	if x.Esito != fascicolo.EsitoSaltata || x.Motivo != "chiusa dopo l'anteprima" || x.Stato != "CHIUSA" || x.NodiRiaperti != 0 {
		t.Errorf("RFQ chiusa durante il comando: %+v", x)
	}
	if got := persa.righe(ap); !strings.Contains(got, "#2:duplicato:") {
		t.Errorf("RFQ chiusa durante il comando: %s", got)
	}
}

// Il conteggio che non torna (A5.15.2, passo 4): se le righe scritte non sono quelle lette, la RFQ si annulla
// per intero (nessuna delle sue righe resta riaperta a meta'), le altre si fanno, e il comando esce con un
// errore che la nomina. Le righe «saltate» le simula un trigger di prova che rifiuta l'UPDATE di un nodo.
func TestUnaRfqCheNonTornaSiAnnullaELeAltreVanno(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaInCorso()
	altra := b.altraRfq("altra")
	ra := altra.rfqCongelabile()
	aa := altra.allegatoStep("a.stp", strings.Repeat("1", 64))
	altra.applica(aa, fattiSTEP{nodi: []string{"#1=X1", "#2=Y1"}, archi: []string{"#1>#2"}}.json())
	altra.agganciaPerCodice(aa, "#2", ra.f1)
	b.esegui(fmt.Sprintf(`CREATE FUNCTION prova_salta_u5() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF OLD.thread_id = '%s' AND OLD.chiave = '#3' AND NEW.stato = 'aperta' THEN RETURN NULL; END IF; RETURN NEW; END $$`, b.thread))
	b.esegui(`CREATE TRIGGER prova_salta_u5 BEFORE UPDATE ON componente_proposta FOR EACH ROW EXECUTE FUNCTION prova_salta_u5()`)
	t.Cleanup(func() { b.p.Exec(context.Background(), `DROP TRIGGER IF EXISTS prova_salta_u5 ON componente_proposta`) })

	var passi []string
	r, err := b.riapri(true, func(x fascicolo.RapportoRiapertura) error {
		if n := len(x.Rfq); n > 0 {
			passi = append(passi, x.Rfq[n-1].Esito)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), b.thread.String()) || !strings.Contains(err.Error(), "non riaperte") {
		t.Fatalf("il comando doveva uscire con un errore che nomina la RFQ: %v", err)
	}
	x := rfqDi(t, r, b.thread)
	if x.Esito != fascicolo.EsitoFallita || !strings.Contains(x.Motivo, "le righe scritte non sono quelle lette") || x.NodiRiaperti != 0 {
		t.Errorf("RFQ che non torna: %+v", x)
	}
	if got := b.righe(sc.s); got != "#1:duplicato:P1 #2:duplicato:B #3:duplicato:C #4:duplicato:D" {
		t.Errorf("la RFQ che non torna e' stata riaperta a meta': %s", got)
	}
	if got := altra.righe(aa); got != "#1:aperta:- #2:aperta:-" {
		t.Errorf("l'altra RFQ: %s", got)
	}
	if r.Totali.Fallite != 1 || r.Totali.Riaperte != 1 || strings.Join(passi, ",") != "fallita,riaperta,riaperta" {
		t.Errorf("totali %+v, passi %v", r.Totali, passi)
	}
}
