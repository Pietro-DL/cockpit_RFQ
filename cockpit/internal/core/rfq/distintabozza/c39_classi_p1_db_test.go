//go:build integrazione

// L4 — giro G39, G39-04 [T, delegata dall'utente all'orchestratore: nessuno script di backfill in blocco, la conversione
// delle pre-assegnazioni resta alla presa in carico, D-AR14 A [T-39]; mancavano le prove per classe], corsia persistenza
// parte B. Sul database di prova, una RFQ con un'occorrenza per ciascuna classe della vista diagnostica della 0023
// (v_ar_backfill_diagnostica; piano delle dipendenze della direttiva 35, §5.1), comprese quelle che i due dump non hanno
// (C1s e D1s sintetiche, P2 e P3):
//   - C1  documento confermato con componente, proposta sullo stesso componente («Assegna» legacy, poi conferma);
//   - C1s la stessa cosa per uno STEP del prodotto finito;
//   - D1  documento confermato con componente, proposta senza componente;
//   - D1s documento STEP del prodotto finito senza proposta;
//   - P1  proposta aperta con la copia del componente, occorrenza attiva, senza autore registrato;
//   - P1s la stessa cosa per uno STEP del prodotto finito;
//   - P2  proposta aperta con la copia, occorrenza in Altri file;
//   - P3  proposta scartata con il componente.
// Alla presa in carico del prodotto (R2 crea, partenza registrato) si converte la sola P1: la PB legacy sul nodo, l'evento
// conversione_legacy con l'autore che prende in carico, la copia tolta. Gli invarianti di D-AR04 [U]: nessuna AR, nessun
// evento ne' autore fittizio (la proposta resta senza deciso_da, nessun evento senza autore), documenti, provenienze e
// storia della fonte invariati, le altre proposte invariate; P1s (STEP del finito, candidato di S8) e P2 (non attiva)
// restano con il motivo. La ripresa dopo l'abbandono non scrive niente nel giornale. Dati sintetici (ACME, ACM-xxxx).

package distintabozza

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// impronta3: md5 di tutte le righe della query (ordinate), "-" se non ce ne sono.
func impronta3(t *testing.T, p *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var h *string
	if err := p.QueryRow(context.Background(), `SELECT md5(string_agg(x::text, '|' ORDER BY x::text)) FROM (`+sql+`) x`, args...).Scan(&h); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	if h == nil {
		return "-"
	}
	return *h
}

func TestV1G39ConversioneP1PerClasseDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	msg := idDi("msg-0")
	var figlio uuid.UUID
	for k, c := range s.sc.componenti {
		if k != "k-root" {
			figlio = c
			break
		}
	}
	var codiceP, codiceF string
	if err := p.QueryRow(ctx, `SELECT (SELECT codice FROM componente WHERE componente_id = $1), (SELECT codice FROM componente WHERE componente_id = $2)`,
		s.prodotto, figlio).Scan(&codiceP, &codiceF); err != nil {
		t.Fatal(err)
	}
	documento := func(allegato, comp uuid.UUID, codice, tipo, ext, sha string) {
		t.Helper()
		var d uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING documento_id`, s.thread, comp, tipo, codice, codice+"."+ext, ext, sha,
			"DOC/"+codice+"_"+sha[:4]+"."+ext, s.utente).Scan(&d); err != nil {
			t.Fatal(err)
		}
		esegui3(t, p, `INSERT INTO documento_provenienza (documento_id, allegato_id, messaggio_id, ricevuto_il) VALUES ($1, $2, $3, now())`, d, allegato, msg)
	}
	proposta := func(allegato uuid.UUID, comp *uuid.UUID, codice, tipo, stato string) {
		t.Helper()
		var deciso any
		if stato != "aperta" {
			deciso = s.utente
		}
		esegui3(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte, stato, deciso_da, deciso_il)
			VALUES ($1, $2, $3, $4, $5, 60, 'nome_file', $6::stato_proposta, $7, CASE WHEN $6::text = 'aperta' THEN NULL ELSE now() END)`,
			allegato, s.thread, tipo, codice, nullUUID(comp), stato, deciso)
	}
	pr := s.prodotto
	cl := map[string]uuid.UUID{}
	cl["C1"] = nuovoAllegato(t, p, msg, 31, codiceF+"_c1.pdf", strings.Repeat("1", 64))
	documento(cl["C1"], figlio, codiceF, "disegno_2d", "pdf", strings.Repeat("1", 64))
	proposta(cl["C1"], &figlio, codiceF, "disegno_2d", "confermata")
	cl["C1s"] = nuovoAllegato(t, p, msg, 32, codiceP+"_c1s.stp", strings.Repeat("2", 64))
	documento(cl["C1s"], pr, codiceP, "cad_3d", "stp", strings.Repeat("2", 64))
	proposta(cl["C1s"], &pr, codiceP, "cad_3d", "confermata")
	cl["D1"] = nuovoAllegato(t, p, msg, 33, codiceF+"_d1.pdf", strings.Repeat("3", 64))
	documento(cl["D1"], figlio, codiceF, "disegno_2d", "pdf", strings.Repeat("3", 64))
	proposta(cl["D1"], nil, codiceF, "disegno_2d", "confermata")
	cl["D1s"] = nuovoAllegato(t, p, msg, 34, codiceP+"_d1s.stp", strings.Repeat("4", 64))
	documento(cl["D1s"], pr, codiceP, "cad_3d", "stp", strings.Repeat("4", 64))
	cl["P1"] = nuovoAllegato(t, p, msg, 35, codiceF+"_p1.pdf", strings.Repeat("5", 64))
	proposta(cl["P1"], &figlio, codiceF, "disegno_2d", "aperta")
	cl["P1s"] = nuovoAllegato(t, p, msg, 36, codiceP+"_p1s.stp", strings.Repeat("6", 64))
	proposta(cl["P1s"], &pr, codiceP, "cad_3d", "aperta")
	cl["P2"] = nuovoAllegato(t, p, msg, 37, codiceF+"_p2.pdf", strings.Repeat("7", 64))
	proposta(cl["P2"], &figlio, codiceF, "disegno_2d", "aperta")
	inAltriFile(t, p, cl["P2"], s.utente)
	cl["P3"] = nuovoAllegato(t, p, msg, 38, codiceF+"_p3.pdf", strings.Repeat("8", 64))
	proposta(cl["P3"], &figlio, codiceF, "disegno_2d", "scartata")

	// la vista diagnostica classifica ogni occorrenza (in sola lettura)
	for classe, a := range cl {
		var c string
		if err := p.QueryRow(ctx, `SELECT classe FROM v_ar_backfill_diagnostica WHERE allegato_id = $1`, a).Scan(&c); err != nil || c != classe {
			t.Fatalf("la classe di %s nella diagnostica: %q %v", classe, c, err)
		}
	}
	if n := conta3(t, p, `SELECT count(*) FROM v_ar_backfill_diagnostica WHERE allegato_id = $1 AND NOT autore_registrato AND occorrenza_attiva AND NOT step_su_finito
		AND NOT ha_ar_attiva`, cl["P1"]); n != 1 {
		t.Fatal("le colonne della P1: senza autore registrato, attiva, non STEP del finito, senza AR")
	}
	// la fotografia prima della presa in carico
	foto := func() map[string]string {
		return map[string]string{
			"documento":             impronta3(t, p, `SELECT * FROM documento`),
			"documento_provenienza": impronta3(t, p, `SELECT * FROM documento_provenienza`),
			"proposte_non_p1":       impronta3(t, p, `SELECT * FROM documento_proposta WHERE allegato_id <> $1`, cl["P1"]),
			"fonte_step_evento":     impronta3(t, p, `SELECT * FROM fonte_step_evento`),
			"file_rfq":              impronta3(t, p, `SELECT * FROM file_rfq`),
			"step_strutturale":      impronta3(t, p, `SELECT componente_id, step_strutturale_id FROM componente`),
		}
	}
	prima := foto()
	ev0 := conta3(t, p, `SELECT count(*) FROM associazione_evento`)

	// la presa in carico del prodotto
	chiave := uuid.New()
	c, err := creaDB(t, p, s, s.prodotto, chiave, map[string]any{"partenza": "registrato"})
	if err != nil {
		t.Fatalf("la presa in carico: %v", err)
	}
	nodo := ChiaveDelComponente(figlio)
	if len(c.ConversioniLegacy) != 1 || c.ConversioniLegacy[0].AllegatoID != cl["P1"] || c.ConversioniLegacy[0].ComponentePrima != figlio ||
		c.ConversioniLegacy[0].ChiaveBozza != nodo || c.ConversioniLegacy[0].Ripresa {
		t.Fatalf("si converte la sola P1: %+v", c.ConversioniLegacy)
	}
	non := map[uuid.UUID]string{}
	for _, x := range c.PreassegnazioniNonConvertite {
		non[x.AllegatoID] = x.Motivo
	}
	if len(non) != 2 || non[cl["P1s"]] != P1StepDelFinito || non[cl["P2"]] != P1FileNonAttivo {
		t.Fatalf("le non convertite (P1s step_del_finito, P2 file_non_attivo): %+v", c.PreassegnazioniNonConvertite)
	}
	// le PB: la sola P1, legacy, sul nodo del suo componente; nessuna PB dei documenti (C1, C1s, D1, D1s) ne' degli STEP
	snap, err := LeggiSnapshot3(c.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DocumentiPreassociati) != 1 || snap.DocumentiPreassociati[0] != (Preassociazione3{AllegatoID: cl["P1"], ChiaveBozza: nodo, Provenienza: ProvenienzaLegacy}) {
		t.Fatalf("le PB della versione 1: %+v", snap.DocumentiPreassociati)
	}
	// gli invarianti di D-AR04: nessuna AR, un solo evento (la conversione, con l'autore vero), niente autore fittizio
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != 0 {
		t.Fatalf("AR dopo la presa in carico: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento`); n != ev0+1 {
		t.Fatalf("eventi del giornale: %d -> %d", ev0, n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE allegato_id = $1 AND origine = 'conversione_legacy' AND azione = 'sgancia'
		AND componente_prima = $2 AND componente_dopo IS NULL AND associazione_id IS NULL AND conferma_id IS NULL AND autore_id = $3 AND chiave_idempotenza = $4`,
		cl["P1"], figlio, s.utente, chiave); n != 1 {
		t.Fatal("l'evento conversione_legacy della P1")
	}
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE autore_id IS NULL`); n != 0 {
		t.Fatalf("eventi senza autore: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id IS NULL AND stato = 'aperta' AND deciso_da IS NULL
		AND deciso_il IS NULL`, cl["P1"]); n != 1 {
		t.Fatal("la proposta della P1: copia tolta, ancora aperta, nessun autore scritto")
	}
	dopo := foto()
	for k, v := range prima {
		if dopo[k] != v {
			t.Fatalf("la presa in carico ha cambiato %s", k)
		}
	}
	// la vista dopo: la P1 non e' piu' una pre-assegnazione; le altre classi restano
	if n := conta3(t, p, `SELECT count(*) FROM v_ar_backfill_diagnostica WHERE allegato_id = $1`, cl["P1"]); n != 0 {
		t.Fatal("la P1 convertita e' ancora nella diagnostica")
	}
	if s := strings.Join(classiDiagnostica(t, p, cl), ","); s != "C1,C1s,D1,D1s,P1s,P2,P3" {
		t.Fatalf("le classi dopo la presa in carico: %s", s)
	}

	// la ripresa dopo l'abbandono: la PB legacy torna, nessun evento nuovo, P1s e P2 ancora com'erano
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: c.BozzaID, VersioneAttesa: c.Versione, Motivo: "prova", UtenteID: s.utente}); err != nil {
		t.Fatal(err)
	}
	c2, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "registrato"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c2.ConversioniLegacy) != 1 || !c2.ConversioniLegacy[0].Ripresa || pbDi(t, c2.Snapshot, cl["P1"]).Provenienza != ProvenienzaLegacy {
		t.Fatalf("la ripresa: %+v", c2.ConversioniLegacy)
	}
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento`); n != ev0+1 {
		t.Fatalf("la ripresa ha scritto nel giornale: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != 0 {
		t.Fatalf("AR dopo la ripresa: %d", n)
	}
	finale := foto()
	for k, v := range prima {
		if finale[k] != v {
			t.Fatalf("la ripresa ha cambiato %s", k)
		}
	}
}

// classiDiagnostica: le classi della vista per le occorrenze date, in ordine.
func classiDiagnostica(t *testing.T, p *pgxpool.Pool, cl map[string]uuid.UUID) []string {
	t.Helper()
	ids := make([]uuid.UUID, 0, len(cl))
	for _, a := range cl {
		ids = append(ids, a)
	}
	var s *string
	if err := p.QueryRow(context.Background(), `SELECT string_agg(classe, ',' ORDER BY classe) FROM v_ar_backfill_diagnostica WHERE allegato_id = ANY($1::uuid[])`,
		ids).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s == nil {
		return nil
	}
	return strings.Split(*s, ",")
}
