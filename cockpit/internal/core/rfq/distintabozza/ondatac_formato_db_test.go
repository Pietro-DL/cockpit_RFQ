//go:build integrazione

// L4 — V1, ondata C, corsia persistenza: le regole nuove della forma salvataggio di R2 sul database di prova.
//   - EV-02 (parte server): le evidenze documentali nuove sono file attivi della RFQ con lo sha dell'allegato
//     (sha_diverso, FILE_NON_ATTIVO); un'evidenza gia' salvata resta anche se il file e' stato messo da parte dopo;
//   - U2-P35: su un nodo del componente (riusa, modifica) un'identita' decisa con un altro codice e' rifiutata;
//   - conferma_categoria con classe_grammatica: V-1 contro il registro della versione (proposta_non_corrispondente
//     quando la voce non ha la classe), e la categoria con origine normativa che si salva.
// Solo dati sintetici: la scena di ondatab_db_test.go.

package distintabozza

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

func TestV1OndataCEvidenzeDocumentaliSulDatabase(t *testing.T) {
	sc := preparaBozza3(t)
	versione := 1
	cur := copiaProfonda(t, sc.base)
	salva := func(snap map[string]any, azioni ...Azione) (Salvato3, error) {
		t.Helper()
		r, err := sc.salva3(t, versione, uuid.New(), snap, azioni)
		if err == nil && r.NuovaVersione {
			versione, cur = r.Versione, snap
		}
		return r, err
	}
	conEvidenzaNodo := func(sha string) (map[string]any, Azione) {
		ev := []any{map[string]any{"allegato_id": sc.id.pdf.String(), "sha256": sha, "pagine": []any{1}}}
		s := copiaProfonda(t, cur)
		nodoIn3(s, "k-man")["evidenze_identita"] = ev
		return s, azione3(t, AzNodoModificato, "k-man", map[string]any{"evidenze_identita": nil}, map[string]any{"evidenze_identita": ev})
	}
	// lo sha che non e' quello dell'allegato: sha_diverso
	s, a := conEvidenzaNodo(shaProva('e'))
	_, err := salva(s, a)
	e := codice3(t, err, CodiceCorpoNonValido, MotivoShaDiverso)
	if e.Campo != "snapshot.nodi[3].evidenze_identita[0].sha256" {
		t.Fatalf("campo: %s", e.Campo)
	}
	// lo sha giusto: passa
	s, a = conEvidenzaNodo(sc.id.shaPdf)
	if _, err := salva(s, a); err != nil {
		t.Fatalf("evidenza con lo sha dell'allegato: %v", err)
	}
	// il PDF in Altri file: un'evidenza nuova sull'arco e' FILE_NON_ATTIVO, quella gia' salvata sul nodo resta
	inAltriFile(t, sc.p, sc.id.pdf, sc.utente)
	evA := []any{map[string]any{"allegato_id": sc.id.pdf.String(), "sha256": sc.id.shaPdf}}
	s = copiaProfonda(t, cur)
	for _, x := range s["archi"].([]any) {
		if m := x.(map[string]any); m["figlio"] == "k-man" {
			m["evidenze_arco"] = evA
		}
	}
	_, err = salva(s, azione3(t, AzArcoModificato, "k-man", map[string]any{"padre": "k-root", "figlio": "k-man", "evidenze_arco": nil},
		map[string]any{"padre": "k-root", "figlio": "k-man", "evidenze_arco": evA}))
	codice3(t, err, CodiceFileNonAttivo, "")
	s = copiaProfonda(t, cur)
	nodoIn3(s, "k-man")["descrizione"] = "Vite di prova"
	if _, err := salva(s, azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil}, map[string]any{"descrizione": "Vite di prova"})); err != nil {
		t.Fatalf("un salvataggio che non tocca l'evidenza gia' salvata: %v", err)
	}
}

func TestV1OndataCIdentitaDecisaSuUnRegistrato(t *testing.T) {
	sc := preparaBozza3(t)
	cur := copiaProfonda(t, sc.base)
	// il componente k-s1 della scena ha il codice ACM-1100: una correzione con un altro codice su un nodo riusa e' 422
	conDecisione := func(codice string) (map[string]any, []Azione) {
		d := map[string]any{"id": "dc-0001", "tipo": "identita", "chiave_bozza": "k-s1", "modo": "correzione",
			"identita": map[string]any{"emittente": sc.id.clienteE(), "codice": codice}, "motivo": "il disegno dice cosi'"}
		n := map[string]any{"chiave_bozza": "k-s1", "componente_id": idDi("k-s1").String(), "op": "riusa", "tipo": "sottoassieme", "origine": "registrato",
			"identita": map[string]any{"emittente": sc.id.clienteE(), "codice": codice, "modo": "decisione", "decisione": "dc-0001"}}
		x := map[string]any{"padre": "k-root", "figlio": "k-s1", "qta": 1, "origine": "utente"}
		s := copiaProfonda(t, cur)
		s["nodi"] = append(s["nodi"].([]any), n)
		s["archi"] = append(s["archi"].([]any), x)
		s["decisioni"] = append(s["decisioni"].([]any), d)
		return s, []Azione{azione3(t, AzNodoAggiunto, "k-s1", nil, n), azione3(t, AzArcoAggiunto, "k-s1", nil, x),
			azione3(t, AzDecisioneAggiunta, "k-s1", nil, d)}
	}
	s, aa := conDecisione("ACM-1199")
	_, err := sc.salva3(t, 1, uuid.New(), s, aa)
	e := codice3(t, err, CodiceCorpoNonValido, MotivoRegistratoNonCorrispondente)
	if e.Campo != "snapshot.nodi[5].identita.codice" {
		t.Fatalf("campo: %s", e.Campo)
	}
	// con il codice di oggi la decisione (per esempio con un'evidenza) passa
	s, aa = conDecisione("ACM-1100")
	if _, err := sc.salva3(t, 1, uuid.New(), s, aa); err != nil {
		t.Fatalf("decisione identita con il codice registrato: %v", err)
	}
}

func TestV1OndataCConfermaCategoriaSulDatabase(t *testing.T) {
	sc := preparaBozza3(t)
	versione := 1
	cur := copiaProfonda(t, sc.base)
	conCat := func(d map[string]any) (map[string]any, Azione) {
		s := copiaProfonda(t, cur)
		s["decisioni"] = append(s["decisioni"].([]any), d)
		return s, azione3(t, AzDecisioneAggiunta, "", nil, d)
	}
	// la classe copiata dal registro, ma la voce del nodo non ne ha una: proposta_non_corrispondente
	s, a := conCat(map[string]any{"id": "cc-0001", "tipo": "conferma_categoria", "soggetto": map[string]any{"chiave_bozza": "k-0007"},
		"categoria": "minuteria", "esito": "conferma", "origine_categoria": "classe_grammatica", "ingresso": "editor",
		"classe_proposta": map[string]any{"categoria": "minuteria", "fonte": "acme-norma", "valore": "2.1051"}, "importazione": sc.reg.Impronta})
	_, err := sc.salva3(t, versione, uuid.New(), s, []Azione{a})
	codice3(t, err, CodiceCorpoNonValido, MotivoPropostaNonCorrispondente)
	// con l'origine normativa e il motivo: si salva, con il punto d'ingresso
	s, a = conCat(map[string]any{"id": "cc-0002", "tipo": "conferma_categoria", "soggetto": map[string]any{"chiave_bozza": "k-man"},
		"categoria": "minuteria", "esito": "conferma", "origine_categoria": "normativa", "ingresso": "wizard", "motivo": "vite ISO di serie"})
	r, err := sc.salva3(t, versione, uuid.New(), s, []Azione{a})
	if err != nil {
		t.Fatalf("conferma_categoria normativa: %v", err)
	}
	if !r.NuovaVersione {
		t.Fatal("nessuna versione nuova")
	}
}

// OB-08 (parte L4), DEC-B PB-34: una chiave tolta in un salvataggio e rimessa in un altro per un nodo diverso → 422; la
// chiave si legge dal giornale salvato (nodo_tolto).
func TestV1OndataCChiaveToltaDalGiornale(t *testing.T) {
	sc := preparaBozza3(t)
	man := nodoIn3(sc.base, "k-man")
	arco := map[string]any{"padre": "k-root", "figlio": "k-man", "qta": 4, "origine": "utente", "posizione": "Pos." + eAccento}
	senza := senzaChiave(t, sc.base, "k-man")
	r, err := sc.salva3(t, 1, uuid.New(), senza, []Azione{azione3(t, AzArcoTolto, "k-man", arco, nil), azione3(t, AzNodoTolto, "k-man", man, nil)})
	if err != nil {
		t.Fatalf("togliere il nodo: %v", err)
	}
	diverso := nodoManuale3(sc.id, "k-man", "ACM-7777")
	con := copiaProfonda(t, senza)
	con["nodi"] = append(con["nodi"].([]any), diverso)
	_, err = sc.salva3(t, r.Versione, uuid.New(), con, []Azione{azione3(t, AzNodoAggiunto, "k-man", nil, diverso)})
	if e := codice3(t, err, CodiceCorpoNonValido, ""); e.Campo != "azioni[0].chiave_bozza" {
		t.Fatalf("campo: %s", e.Campo)
	}
	// con una chiave nuova passa
	nuovo := nodoManuale3(sc.id, "k-man-2", "ACM-7777")
	con = copiaProfonda(t, senza)
	con["nodi"] = append(con["nodi"].([]any), nuovo)
	if _, err := sc.salva3(t, r.Versione, uuid.New(), con, []Azione{azione3(t, AzNodoAggiunto, "k-man-2", nil, nuovo)}); err != nil {
		t.Fatalf("chiave nuova: %v", err)
	}
}

// impronta-ingressi-1 (U2 §6.3.3; parte di CN-01…CN-05 lato server): le parti cambiano con il contenuto, non con gli
// orari. Un salvataggio cambia solo la parte bozza; una riga di proposta riscritta sul posto con un altro contenuto cambia
// la parte proposta; i fatti riscritti con lo stesso contenuto (solo calcolato_il) no, con altri fatti si'; un lavoro di
// analisi pendente mette l'allegato in analisi.
func TestV1OndataCImprontaIngressi(t *testing.T) {
	sc := preparaBozza3(t)
	leggi := func() Ingressi {
		t.Helper()
		ctx := context.Background()
		tx, err := sc.p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		x, err := LeggiIngressi(ctx, db.New(tx), sc.thread, sc.prodotto, "regole-di-prova")
		if err != nil {
			t.Fatalf("ingressi: %v", err)
		}
		return x
	}
	a := leggi()
	if a.Parti.Bozza.BozzaID == nil || *a.Parti.Bozza.BozzaID != sc.bozza || *a.Parti.Bozza.Versione != 1 || len(a.Parti.Analisi.InAnalisi) != 0 {
		t.Fatalf("ingressi di partenza: %+v", a)
	}
	if b := leggi(); b.ImprontaIngressi != a.ImprontaIngressi {
		t.Fatal("due letture danno impronte diverse")
	}
	// un salvataggio: cambia la bozza, non la proposta ne' il bom
	s := copiaProfonda(t, sc.base)
	nodoIn3(s, "k-man")["descrizione"] = "Vite di prova"
	if _, err := sc.salva3(t, 1, uuid.New(), s, []Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil},
		map[string]any{"descrizione": "Vite di prova"})}); err != nil {
		t.Fatal(err)
	}
	b := leggi()
	if b.Parti.Bozza.Impronta == a.Parti.Bozza.Impronta || b.Parti.Proposta.Impronta != a.Parti.Proposta.Impronta || b.Parti.Bom.Impronta != a.Parti.Bom.Impronta {
		t.Fatalf("dopo il salvataggio: bozza %v, proposta %v, bom %v", b.Parti.Bozza.Impronta != a.Parti.Bozza.Impronta,
			b.Parti.Proposta.Impronta != a.Parti.Proposta.Impronta, b.Parti.Bom.Impronta != a.Parti.Bom.Impronta)
	}
	// i fatti riscritti con lo stesso contenuto: nessun cambio; con un altro contenuto: la proposta cambia
	esegui3(t, sc.p, `UPDATE analisi_fatti SET calcolato_il = calcolato_il + interval '1 hour' WHERE sha256 = $1`, sc.id.sha)
	if c := leggi(); c.Parti.Proposta.Impronta != b.Parti.Proposta.Impronta {
		t.Fatal("un orario diverso ha cambiato la parte proposta (conflitto artificiale)")
	}
	esegui3(t, sc.p, `UPDATE analisi_fatti SET fatti = $2 WHERE sha256 = $1`, sc.id.sha, fattiStep3(t, "#1", "#7", "#9", "#11", "#31", "#40"))
	c := leggi()
	if c.Parti.Proposta.Impronta == b.Parti.Proposta.Impronta {
		t.Fatal("fatti nuovi senza cambiare la parte proposta")
	}
	// una proposta di documento con un altro codice (riscritta sul posto)
	esegui3(t, sc.p, `UPDATE documento_proposta SET codice = 'ACM-1101' WHERE proposta_id = $1`, sc.proposta)
	d := leggi()
	if d.Parti.Proposta.Impronta == c.Parti.Proposta.Impronta {
		t.Fatal("una proposta riscritta sul posto non cambia la parte proposta")
	}
	// un lavoro d'analisi pendente sullo STEP: l'allegato e' in analisi
	esegui3(t, sc.p, `INSERT INTO job (tipo, worker_tipo, payload) VALUES ('analizza_allegato', 'analisi', jsonb_build_object('sha256', $1::text))`, sc.id.sha)
	e := leggi()
	if len(e.Parti.Analisi.InAnalisi) != 1 || e.Parti.Analisi.InAnalisi[0].AllegatoID != sc.id.allegato || e.Parti.Analisi.InAnalisi[0].Stato != AnalisiInAnalisi ||
		e.Parti.Analisi.Impronta == d.Parti.Analisi.Impronta || e.ImprontaIngressi == d.ImprontaIngressi {
		t.Fatalf("in analisi: %+v", e.Parti.Analisi)
	}
	// un prodotto che non e' della RFQ: FUORI_RFQ
	ctx := context.Background()
	tx, err := sc.p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	_, err = LeggiIngressi(ctx, db.New(tx), sc.thread, uuid.New(), "")
	codice3(t, err, CodiceFuoriRFQ, "")
}
