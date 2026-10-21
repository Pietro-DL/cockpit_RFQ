//go:build integrazione

// L4 — direttiva 39 §2, §3, §8 [U + T-39] (prova P39-05 sulla strada reale), corsia persistenza: la revisione con un
// secondo STEP del prodotto. S8 su A (partenza step) → R2 con una PB manuale → R4 → R5 (conferma 1, formato 3); poi arriva
// lo STEP B con la stessa struttura e gli stessi codici: S8 su B apre da-conferma-1 e cambia la fonte su B. Le caselle
// sono legate allo sha256 dello STEP, quindi nessuna voce di B ha il rif di una casella di A: senza l'abbinamento per
// identita' i nodi registrati uscivano (fuori_per_cambio_fonte) e le voci di B rientravano come pezzi da creare con gli
// stessi codici (R4: ASSENZA_NON_DECISA, CREA_SU_ESISTENTE, PREASSOCIAZIONE_NON_APPLICABILE). Con l'abbinamento i nodi
// registrati restano con la loro chiave e prendono la casella di B; R4 e' confermabile; R5 promuove B e sostituisce A dopo,
// la storia di A resta nella conferma 1, la AR del particolare resta. Un caso non certo (stesso codice, rev diversa) resta
// fuori dagli abbinamenti automatici: il nodo resta senza casella e la voce non entra (abbinamenti_proposti[]).
// Dati sintetici: la scena di s8_db_test.go, con registri di prova al posto del calcolo del motore.

package sorgente

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

const hashCfgRev = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

// fattiRevisione: i fatti correnti dello STEP con le chiavi date (la struttura v3 che legge R4).
func fattiRevisione(t *testing.T, p *pgxpool.Pool, sha string, chiavi ...string) {
	t.Helper()
	nodi := make([]map[string]any, 0, len(chiavi))
	for _, k := range chiavi {
		nodi = append(nodi, map[string]any{"chiave": k})
	}
	b, err := json.Marshal(map[string]any{"struttura": map[string]any{"versione": 3, "radici": []string{chiavi[0]}, "nodi": nodi,
		"relazioni": []any{}, "avvisi": []any{}, "limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}})
	if err != nil {
		t.Fatal(err)
	}
	esegui(t, p, `INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (3, $1)
		ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = 3, hash_configurazione = $1`, hashCfgRev)
	esegui(t, p, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)`, sha, hashCfgRev, string(b))
}

// riepilogoRev: R4 sulla versione salvata, con un calcolo del motore riuscito che non valuta il prodotto (come le prove di
// R5 nel core: le due firme coincidono).
func riepilogoRev(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, versione int) distintabozza.RiepilogoR4 {
	t.Helper()
	ctx := context.Background()
	m := corpoVersioneRev(t, p, bozza, versione)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	r, err := distintabozza.LeggiRichiestaR4(raw)
	if err != nil {
		t.Fatalf("corpo di R4: %v", err)
	}
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rp, err := distintabozza.Riepilogo3(ctx, db.New(tx), s.thread, r, distintabozza.OpzioniR4{UtenteID: s.utente, AvvisiMotore: true,
		Motore: &distintabozza.EsitoMotoreR4{Ingresso: &distintabozza.IngressoMotoreR4{}}})
	if err != nil {
		t.Fatalf("R4: %v", err)
	}
	return rp
}

func corpoVersioneRev(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int) map[string]any {
	t.Helper()
	var raw []byte
	if err := p.QueryRow(context.Background(), `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["bozza_id"], m["bozza_versione"] = bozza, versione
	return m
}

func motoreRev(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*distintabozza.IngressoMotoreR4, error) {
	return &distintabozza.IngressoMotoreR4{}, nil
}

// tabelleRev: le tabelle che R5 scrive o non deve scrivere (la fotografia dei guasti iniettati).
var tabelleRev = []string{"componente", "componente_relazione", "conferma_strutturale", "conferma_strutturale_componente",
	"conferma_strutturale_arco", "conferma_strutturale_non_incluso", "file_rfq_associazione", "associazione_evento", "documento",
	"documento_proposta", "componente_proposta", "relazione_proposta", "fonte_step_evento", "distinta_bozza", "esito_intento",
	"f8_stato", "job", "working_contatore"}

func fotoRev(t *testing.T, p *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tab := range tabelleRev {
		var h *string
		if err := p.QueryRow(context.Background(), `SELECT md5(string_agg(x::text, '|' ORDER BY x::text)) FROM `+tab+` x`).Scan(&h); err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		out[tab] = "-"
		if h != nil {
			out[tab] = *h
		}
	}
	return out
}

// confermaRev: R5 sulla versione salvata con la firma di R4.
func confermaRev(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, versione int, firma string) (distintabozza.ConfermaR5, error) {
	t.Helper()
	return confermaRevCon(t, p, s, bozza, versione, firma, nil)
}

// confermaRevCon: come confermaRev, con il guasto iniettato dopo un passo delle scritture (solo prove).
func confermaRevCon(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, versione int, firma string, guasto func(string) error) (distintabozza.ConfermaR5, error) {
	t.Helper()
	m := corpoVersioneRev(t, p, bozza, versione)
	m["chiave_idempotenza"] = uuid.New()
	raw, err := json.Marshal(map[string]any{"firma_anteprima": firma, "bozza": m})
	if err != nil {
		t.Fatal(err)
	}
	r, err := distintabozza.LeggiRichiestaR5(raw)
	if err != nil {
		t.Fatalf("corpo di R5: %v", err)
	}
	return distintabozza.Conferma3(context.Background(), p, s.thread, r, distintabozza.OpzioniR5{UtenteID: s.utente, Motore: motoreRev, Guasto: guasto})
}

func codiciR4(rp distintabozza.RiepilogoR4) []string {
	var out []string
	for _, b := range rp.Blocchi {
		out = append(out, b.Codice+"/"+b.Motivo)
	}
	return out
}

// nodoPerCodice: il nodo della versione con il codice dato (l'identita' del nodo).
func nodiPerCodice(s distintabozza.Snapshot3, codice string) []distintabozza.Nodo3 {
	var out []distintabozza.Nodo3
	for _, n := range s.Nodi {
		if n.Identita.Codice == codice {
			out = append(out, n)
		}
	}
	return out
}

// pdfDi: un PDF della RFQ con la proposta di documento che ne legge il codice.
func pdfDi(t *testing.T, p *pgxpool.Pool, s scenaS8, codice string, cifra byte) uuid.UUID {
	t.Helper()
	a := uuid.New()
	esegui(t, p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
		VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), $3, 'pdf', 'file', 'outlook', 10, repeat($4, 64),
		'analizzato', now())`, a, s.msg, codice+".pdf", string(cifra))
	esegui(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', $3, 80, 'nome_file')`,
		a, s.thread, codice)
	return a
}

func TestV1D39S8RevisioneConDueStepFinoAR5(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	figlio := s.figlio
	// il figlio della working e' un pezzo sciolto (un sottoassieme senza figli e' un blocco D1 di R4)
	esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
	// --- la prima conferma, con lo STEP A: S8 (partenza step), PB manuale con R2, R4, R5
	regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#2", codice: "ACM-1100", componente: &figlio, qta: 1},
		{chiave: "#3", codice: "ACM-1150", qta: 4}})
	fattiRevisione(t, p, s.shaA, "#1", "#2", "#3")
	esA, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"motivo": "prima sorgente"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esA.Esito != EsitoCompletata || esA.Bozza == nil || esA.Bozza.Versione != 1 {
		t.Fatalf("S8 su A: %+v %v", esA, err)
	}
	bozzaA := esA.Bozza.BozzaID
	sA, _ := snapshotVersione(t, p, bozzaA, 1)
	staffaA := nodiPerCodice(sA, "ACM-1150")
	if len(staffaA) != 1 {
		t.Fatalf("la staffa nella bozza di A: %+v", sA.Nodi)
	}
	pdf := pdfDi(t, p, s, "ACM-1150", '6')
	var m map[string]any
	b, _ := json.Marshal(sA)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	pb := map[string]any{"allegato_id": pdf.String(), "chiave_bozza": staffaA[0].ChiaveBozza}
	m["documenti_preassociati"] = []any{pb}
	salvaManuale(t, p, s, bozzaA, 1, m, []map[string]any{{"azione": "file_preassociato", "chiave_bozza": staffaA[0].ChiaveBozza, "prima": nil, "dopo": pb,
		"origine": "utente"}})
	rpA := riepilogoRev(t, p, s, bozzaA, 2)
	if len(rpA.Blocchi) != 0 || !rpA.Confermabile {
		t.Fatalf("R4 della prima conferma: %v", codiciR4(rpA))
	}
	c1, err := confermaRev(t, p, s, bozzaA, 2, rpA.FirmaAnteprima)
	if err != nil || c1.NumeroConferma != 1 || len(c1.IdentitaCreate) != 1 || c1.AssociazioniApplicate != 1 {
		t.Fatalf("R5 della prima conferma: %+v %v", c1, err)
	}
	staffa := c1.IdentitaCreate[0]
	var docA uuid.UUID
	if err := p.QueryRow(ctx, `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, s.prodotto).Scan(&docA); err != nil {
		t.Fatal(err)
	}
	arA := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND stato = 'attiva' AND conferma_id = $3`,
		pdf, staffa, c1.ConfermaID)
	if arA != 1 {
		t.Fatalf("la AR del PDF della staffa dopo la conferma 1: %d", arA)
	}

	// --- lo STEP B (rev 2): stessa struttura, stessi codici, sha256 diverso (i rif di B non sono quelli di A), nessuna
	// decisione_esistente nelle voci (come nella prova sul server vero), un pezzo in piu' (ACM-1160)
	regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#5", codice: "ACM-1100", qta: 1}, {chiave: "#6", codice: "ACM-1150", qta: 4},
		{chiave: "#7", codice: "ACM-1160", qta: 2}})
	fattiRevisione(t, p, s.shaB, "#1", "#5", "#6", "#7")
	prima := fotoVietate(t, p)
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "revisione 2"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Esito != EsitoCompletata || esB.Bozza == nil || esB.Bozza.Versione != 2 {
		t.Fatalf("S8 su B: %+v %v", esB, err)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("S8 su B ha toccato la working: %s -> %s", prima, dopo)
	}
	bozza := esB.Bozza.BozzaID
	s1, _ := snapshotVersione(t, p, bozza, 1)
	s2, _ := snapshotVersione(t, p, bozza, 2)
	// i nodi registrati restano con la loro chiave e prendono la casella di B; nessun doppione; la voce nuova entra
	for _, x := range []struct {
		codice, rif string
		comp        uuid.UUID
	}{{"ACM-1100", "nodo:" + s.shaB + ":#5", s.figlio}, {"ACM-1150", "nodo:" + s.shaB + ":#6", staffa}} {
		n1, n2 := nodiPerCodice(s1, x.codice), nodiPerCodice(s2, x.codice)
		if len(n1) != 1 || len(n2) != 1 {
			t.Fatalf("%s: nodi prima %d, dopo %d (nessun doppione: %+v)", x.codice, len(n1), len(n2), s2.Nodi)
		}
		if n2[0].ChiaveBozza != n1[0].ChiaveBozza || n2[0].ComponenteID == nil || *n2[0].ComponenteID != x.comp || len(n2[0].Caselle) != 1 ||
			n2[0].Caselle[0].Rif != x.rif || n2[0].Caselle[0].AllegatoID != s.stepB || n2[0].Origine != distintabozza.OrigineStep {
			t.Fatalf("%s: il nodo della revisione %+v (prima %+v)", x.codice, n2[0], n1[0])
		}
	}
	if n := nodiPerCodice(s2, "ACM-1160"); len(n) != 1 || len(n[0].Caselle) != 1 || n[0].Caselle[0].Rif != "nodo:"+s.shaB+":#7" {
		t.Fatalf("la voce nuova di B: %+v", n)
	}
	for _, a := range s2.Archi {
		if a.Origine == distintabozza.OrigineStep && (a.Step == nil || a.Step.AllegatoID != s.stepB) {
			t.Fatalf("arco step non riscritto su B: %+v", a)
		}
	}
	var dopo map[string]any
	var raw []byte
	if err := p.QueryRow(ctx, `SELECT dopo FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_cambiata'`, bozza).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &dopo); err != nil {
		t.Fatal(err)
	}
	if f, _ := dopo["fuori_per_cambio_fonte"].([]any); len(f) != 0 {
		t.Fatalf("nodi usciti per il cambio della fonte: %v", f)
	}
	// R4: confermabile, la promozione di B e la sostituzione di A previste
	rp := riepilogoRev(t, p, s, bozza, 2)
	if len(rp.Blocchi) != 0 || !rp.Confermabile || !rp.EffettoSullaFonte.Promozione || rp.EffettoSullaFonte.SostituzioneDi == nil ||
		*rp.EffettoSullaFonte.SostituzioneDi != docA {
		t.Fatalf("R4 della revisione: %v %+v", codiciR4(rp), rp.EffettoSullaFonte)
	}
	// il rollback iniettato fra promozione, sostituzione, testata, righe certificate, AR, segni e chiusura: niente resta
	fotoPrima := fotoRev(t, p)
	guasto := errors.New("guasto iniettato")
	for _, passo := range []string{"promozione", "sostituzione", "testata", "certificati", "associazioni", "segni", "chiusura"} {
		_, err := confermaRevCon(t, p, s, bozza, 2, rp.FirmaAnteprima, func(x string) error {
			if x == passo {
				return guasto
			}
			return nil
		})
		if !errors.Is(err, guasto) {
			t.Fatalf("guasto dopo %s: %v", passo, err)
		}
		for _, tab := range tabelleRev {
			if a := fotoRev(t, p); a[tab] != fotoPrima[tab] {
				t.Fatalf("guasto dopo %s: effetti parziali in %s", passo, tab)
			}
		}
	}
	// R5: conferma 2, B promosso, A sostituito da B dopo, nessun componente nuovo per i codici che c'erano, la AR resta
	c2, err := confermaRev(t, p, s, bozza, 2, rp.FirmaAnteprima)
	if err != nil {
		t.Fatalf("R5 della revisione: %v", err)
	}
	if c2.NumeroConferma != 2 || !c2.Fonte.Promossa || c2.Fonte.SostituzioneDi == nil || *c2.Fonte.SostituzioneDi != docA || len(c2.IdentitaCreate) != 1 {
		t.Fatalf("risposta della revisione: %+v", c2)
	}
	var docB uuid.UUID
	if err := p.QueryRow(ctx, `SELECT documento_id FROM documento WHERE sha256 = $1`, s.shaB).Scan(&docB); err != nil {
		t.Fatal(err)
	}
	if n := conta(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND step_strutturale_id = $2`, s.prodotto, docB); n != 1 {
		t.Fatal("R5 promuove B")
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE documento_id = $1 AND sostituito_da = $2`, docA, docB); n != 1 {
		t.Fatal("A sostituito da B")
	}
	if n := conta(t, p, `SELECT count(*) FROM componente WHERE thread_id = $1 AND codice IN ('ACM-1100', 'ACM-1150')`, s.thread); n != 2 {
		t.Fatalf("componenti con i codici che c'erano: %d (nessun doppione)", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND stato = 'attiva' AND conferma_id = $3`,
		pdf, staffa, c1.ConfermaID); n != 1 {
		t.Fatal("la AR del PDF della staffa resta con la sua conferma d'origine")
	}
	if n := conta(t, p, `SELECT count(*) FROM conferma_strutturale_componente WHERE conferma_id = $1 AND componente_id = ANY($2::uuid[])`,
		c1.ConfermaID, []uuid.UUID{s.figlio, staffa}); n != 2 {
		t.Fatal("la storia della conferma 1")
	}
	if n := conta(t, p, `SELECT count(*) FROM conferma_strutturale_componente WHERE conferma_id = $1 AND componente_id = ANY($2::uuid[])`,
		c2.ConfermaID, []uuid.UUID{s.figlio, staffa}); n != 2 {
		t.Fatal("la conferma 2 certifica i componenti che restano")
	}
}

// Il caso non certo nella revisione: stesso codice con una rev diversa. Nessun abbinamento automatico, nessuna uscita
// silenziosa del componente registrato: il nodo resta senza casella, la voce non entra, abbinamenti_proposti[] lo dice.
func TestV1D39S8RevisioneAbbinamentoNonCerto(t *testing.T) {
	p, s := preparaS8(t)
	figlio := s.figlio
	esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
	regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#2", codice: "ACM-1100", componente: &figlio, qta: 1}})
	fattiRevisione(t, p, s.shaA, "#1", "#2")
	esA, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"motivo": "prima sorgente"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esA.Bozza == nil {
		t.Fatalf("S8 su A: %+v %v", esA, err)
	}
	rpA := riepilogoRev(t, p, s, esA.Bozza.BozzaID, 1)
	if _, err := confermaRev(t, p, s, esA.Bozza.BozzaID, 1, rpA.FirmaAnteprima); err != nil {
		t.Fatalf("R5 della prima conferma: %v (%v)", err, codiciR4(rpA))
	}
	// B: lo stesso codice con la rev «B» (una lettura diversa dall'identita' registrata)
	regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#5", codice: "ACM-1100", qta: 1}})
	c := regB.Contenuto
	for i := range c.Proposta.Nodi {
		if c.Proposta.Nodi[i].CodiceProposto == "ACM-1100" {
			c.Proposta.Nodi[i].RevProposta = "B"
		}
	}
	regB, err = importazione.Componi(c)
	if err != nil {
		t.Fatal(err)
	}
	fattiRevisione(t, p, s.shaB, "#1", "#5")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "revisione 2"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Bozza == nil || esB.Bozza.Versione != 2 {
		t.Fatalf("S8 su B: %+v %v", esB, err)
	}
	s2, _ := snapshotVersione(t, p, esB.Bozza.BozzaID, 2)
	n := nodiPerCodice(s2, "ACM-1100")
	if len(n) != 1 || n[0].ComponenteID == nil || *n[0].ComponenteID != s.figlio || len(n[0].Caselle) != 0 || n[0].Origine == distintabozza.OrigineStep {
		t.Fatalf("il nodo registrato non certo: %+v (tutti: %+v)", n, s2.Nodi)
	}
	for _, a := range s2.Archi {
		if a.Figlio == n[0].ChiaveBozza && a.Origine == distintabozza.OrigineStep {
			t.Fatalf("l'arco verso il nodo senza casella e' ancora step: %+v", a)
		}
	}
	var raw []byte
	if err := p.QueryRow(context.Background(), `SELECT dopo FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_cambiata'`,
		esB.Bozza.BozzaID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"abbinamenti_proposti"`) || !strings.Contains(string(raw), `"incompatibile"`) {
		t.Fatalf("abbinamenti_proposti nell'evento: %s", raw)
	}
}
