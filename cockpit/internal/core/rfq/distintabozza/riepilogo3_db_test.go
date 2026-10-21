//go:build integrazione

// L4 — R4 formato 3 (riepilogo3*.go; direttiva 39 §1, §2, §4, §5, §6 [U + T-39]), corsia persistenza, sul database di
// prova, sempre in una transazione REPEATABLE READ READ ONLY (una scrittura la farebbe fallire):
//   - la bozza senza STEP (partenza vuota) e le PB manuali salvate con R2: R4 completo con il blocco firmato
//     STEP_SORGENTE_NON_CONFERMATA, la PB nel gruppo «manuale»; nessuna riga cambia;
//   - la partenza vuota su una working con figli: ASSENZA_NON_DECISA per ogni componente registrato;
//   - la sorgente: lo STEP storico confermato dalle pagine di prima senza S8 (legame storico) blocca; la conferma di S8 con la
//     sola radice o senza gerarchia (zero figli) toglie il gate senza dedurre mono_pezzo ne' la verifica; la revoca blocca;
//   - i gruppi di provenienza sulla bozza salvata (PB motore iniziale, proposta rimossa dall'ingegnere);
//   - Prova 7 della direttiva 35: una AR attiva (gesto legacy in una RFQ interamente legacy) verso un altro componente blocca,
//     con lo svincolo nella stessa bozza e' una sostituzione;
//   - CICLO sul grafo della bozza unito agli archi della working fuori dal perimetro (PD35 INC-G13).
// Dati sintetici.

package distintabozza

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

// riepilogoDB: R4 in una transazione in sola lettura.
func riepilogoDB(t *testing.T, p *pgxpool.Pool, thread uuid.UUID, corpo map[string]any, utente uuid.UUID) (RiepilogoR4, error) {
	t.Helper()
	raw, err := json.Marshal(corpo)
	if err != nil {
		t.Fatal(err)
	}
	r, err := LeggiRichiestaR4(raw)
	if err != nil {
		return RiepilogoR4{}, err
	}
	ctx := context.Background()
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	return Riepilogo3(ctx, db.New(tx), thread, r, OpzioniR4{UtenteID: utente, AvvisiMotore: true, Motore: &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{}}})
}

// motoreFintoDB: il calcolo del motore delle prove di R5 nel core (il motore vero lo da' l'adattatore web): un calcolo
// riuscito che non valuta il prodotto, lo stesso che riepilogoDB da' a R4, cosi' le due firme coincidono.
func motoreFintoDB(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (*IngressoMotoreR4, error) {
	return &IngressoMotoreR4{}, nil
}

func mustRiepilogoDB(t *testing.T, p *pgxpool.Pool, thread uuid.UUID, corpo map[string]any, utente uuid.UUID) RiepilogoR4 {
	t.Helper()
	r, err := riepilogoDB(t, p, thread, corpo, utente)
	if err != nil {
		t.Fatalf("R4: %v", err)
	}
	return r
}

// corpoDellaVersione: il corpo di R4 dalla versione salvata (lo snapshot piu' bozza_id e bozza_versione).
func corpoDellaVersione(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int) map[string]any {
	t.Helper()
	m, _ := snapshotDellaVersione(t, p, bozza, versione)
	m["bozza_id"], m["bozza_versione"] = bozza, versione
	return m
}

// allegatoFileDB: un'occorrenza «file» della RFQ (un PDF) con la proposta di documento che ne legge il codice.
func allegatoFileDB(t *testing.T, p *pgxpool.Pool, thread uuid.UUID, nome, codice string, sha byte) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var msg uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE thread_id = $1 ORDER BY data_evento LIMIT 1`, thread).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	a := uuid.New()
	esegui3(t, p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il, natura)
		VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), $3, 'pdf', repeat($4, 64), now(), 'file')`,
		a, msg, nome, string(sha))
	esegui3(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', $3, 80, 'nome_file')`, a, thread, codice)
	return a
}

func contiR4(t *testing.T, p *pgxpool.Pool) map[string]int {
	t.Helper()
	out := contiTabelleVietate(t, p)
	for _, tab := range []string{"distinta_bozza", "distinta_bozza_versione", "distinta_bozza_evento", "conferma_strutturale", "fonte_step_evento", "esito_intento"} {
		out[tab] = conta3(t, p, `SELECT count(*) FROM `+tab)
	}
	return out
}

// D39 §1 [U] e prova obbligatoria del §8 (prima parte, R4): crea vuota senza STEP → R2 salva PB manuali → R4 mostra il
// riepilogo e il blocco; nessuna scrittura.
func TestV1D39R4BozzaSenzaStepDB(t *testing.T) {
	p, s := poolPulito(t)
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	a := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	id := map[string]any{"emittente": radice["identita"].(map[string]any)["emittente"], "emittente_presunto": true, "codice": "ACM-6500", "modo": "manuale"}
	m["nodi"] = append(m["nodi"].([]any), map[string]any{"chiave_bozza": "k-man", "op": "crea", "tipo": "sciolto", "origine": "utente", "identita": id})
	m["archi"] = []any{map[string]any{"padre": radice["chiave_bozza"], "figlio": "k-man", "qta": 2, "origine": "utente"}}
	m["documenti_preassociati"] = []any{map[string]any{"allegato_id": a, "chiave_bozza": "k-man"}}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{
		{Azione: AzNodoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(m["nodi"].([]any)[1]), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(m["archi"].([]any)[0]), Origine: "utente"},
		{Azione: AzFilePreassociato, ChiaveBozza: "k-man", Dopo: mustJSON(map[string]any{"allegato_id": a, "chiave_bozza": "k-man"}), Origine: "utente"},
	}); err != nil {
		t.Fatalf("R2 con la PB manuale: %v", err)
	}
	prima := contiR4(t, p)
	r := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoSenzaSorgente {
		t.Fatalf("gate: %+v", r.Blocchi)
	}
	if len(r.Blocchi) != 1 || r.Confermabile || r.Base.Stato != BaseValida {
		t.Fatalf("solo il gate: %v %+v", codici(r.Blocchi), r.Base)
	}
	if len(r.DocumentiPreassociati) != 1 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoApplicabile ||
		r.DocumentiPreassociati[0].Provenienza != ProvenienzaManuale || len(r.EffettiDocumentali.Manuale) != 1 {
		t.Fatalf("la PB manuale: %+v", r.DocumentiPreassociati)
	}
	if r.BozzaID == nil || *r.BozzaID != c.BozzaID || r.BozzaVersione == nil || *r.BozzaVersione != 2 || !reFirma64.MatchString(r.FirmaAnteprima) {
		t.Fatalf("testata: %v %v %s", r.BozzaID, r.BozzaVersione, r.FirmaAnteprima)
	}
	if r2 := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente); r2.FirmaAnteprima != r.FirmaAnteprima {
		t.Fatal("due letture, due firme")
	}
	if dopo := contiR4(t, p); !stessiConti(prima, dopo) {
		t.Fatalf("R4 ha scritto: %v → %v", prima, dopo)
	}
	// i rifiuti: un prodotto di un'altra RFQ, una versione che non c'e', un corpo formato 2
	altro := corpoDellaVersione(t, p, c.BozzaID, 2)
	altro["bozza_versione"] = 9
	if _, err := riepilogoDB(t, p, s.thread, altro, s.utente); err == nil || err.(*Errore).Campo != "bozza_versione" {
		t.Fatalf("versione che non c'e': %v", err)
	}
	if _, err := riepilogoDB(t, p, s.thread2, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente); err == nil || err.(*Errore).Codice != CodiceFuoriRFQ {
		t.Fatalf("un'altra RFQ: %v", err)
	}
}

func stessiConti(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// La partenza vuota su un prodotto con la working: ogni componente registrato e' un'assenza da decidere (EM App. I.5).
func TestV1D39R4AssenzeDellaWorkingDB(t *testing.T) {
	p, s := poolPulito(t)
	c, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	r := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 1), s.utente)
	if n := len(trova(r.Blocchi, BloccoAssenzaNonDecisa)); n != len(s.sc.snap.Nodi)-1 {
		t.Fatalf("assenze: %d, attese %d", n, len(s.sc.snap.Nodi)-1)
	}
	if len(trova(r.Blocchi, BloccoStepSorgenteNonConfermata)) != 1 || len(r.Fuori) != len(s.sc.snap.Nodi)-1 {
		t.Fatalf("gate e fuori: %v %d", codici(r.Blocchi), len(r.Fuori))
	}
}

// La sorgente (EM App. I.2): STEP storico senza S8 → blocco con il legame storico; S8 con la sola radice (zero figli) → niente
// gate, niente mono_pezzo ne' verifica dedotti; la revoca → blocco.
func TestV1D39R4SorgenteDB(t *testing.T) {
	p, s := poolPulito(t)
	sha := shaProva('c')
	corpo := func() map[string]any {
		return map[string]any{"formato": 3, "prodotto_id": s.altroFinito,
			"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": conta3(t, p, `SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0)`, s.thread)},
			"firma_base":        firmaDiDB(t, p, s.thread, s.altroFinito),
			"sorgente":          map[string]any{"allegato_id": s.allegato2, "sha256": sha, "gruppo": GruppoSoloProdotto},
			"nodi": []any{map[string]any{"chiave_bozza": ChiaveDelComponente(s.altroFinito), "componente_id": s.altroFinito, "op": "riusa", "tipo": "finito",
				"origine": "registrato", "identita": map[string]any{"emittente": EmittenteCliente(idDi("cliente")), "emittente_presunto": true, "codice": "ACM-6000",
					"modo": "registrato"}}},
			"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}}
	}
	r := mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoNonConfermata || g[0].LegameStorico {
		t.Fatalf("senza conferma: %+v", r.Blocchi)
	}
	// lo STEP storico: documento del finito e conferma_documento delle pagine di prima (senza la chiave di S8)
	var doc uuid.UUID
	if err := p.QueryRow(context.Background(), `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256,
		path_relativo, confermato_da) VALUES ($1, $2, 'cad_3d', 'ACM-6000', 'ACM-6000.stp', 'stp', $3, '3D/ACM-6000.stp', $4) RETURNING documento_id`,
		s.thread, s.altroFinito, sha, s.utente).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, $6)`, s.thread, s.altroFinito, s.allegato2, sha, doc, s.utente)
	r = mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoNonConfermata || !g[0].LegameStorico {
		t.Fatalf("lo STEP storico non soddisfa il gate: %+v", r.Blocchi)
	}
	if len(r.LegamiStorici) != 1 || r.LegamiStorici[0].DocumentoID != doc {
		t.Fatalf("legame storico mostrato: %+v", r.LegamiStorici)
	}
	// S8: conferma_documento con la chiave del passo 2
	esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.altroFinito, s.allegato2, sha, doc, s.utente)
	r = mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if len(r.Blocchi) != 0 || !r.Confermabile || !r.Sorgente.Valida || r.Sorgente.DocumentoID == nil || *r.Sorgente.DocumentoID != doc {
		t.Fatalf("S8 valida, radice sola: %v %+v", codici(r.Blocchi), r.Sorgente)
	}
	if r.CriterioPrevisto.Criterio != "nessuno" || (r.VerificaPrevista.Verificata != nil && *r.VerificaPrevista.Verificata) {
		t.Fatalf("zero figli: niente dedotto: %+v %+v", r.CriterioPrevisto, r.VerificaPrevista)
	}
	if r.SorgenteLegata == nil || !r.SorgenteLegata.Confermata || !r.EffettoSullaFonte.Promozione || len(r.LegamiStorici) != 0 {
		t.Fatalf("legato come sorgente: %+v %+v %+v", r.SorgenteLegata, r.EffettoSullaFonte, r.LegamiStorici)
	}
	// la revoca della sorgente
	esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, motivo, autore_id)
		VALUES ($1, $2, 'prodotto', 'revoca_sorgente', $3, $4, $5, gen_random_uuid(), 'scelto per errore', $6)`, s.thread, s.altroFinito, s.allegato2, sha, doc, s.utente)
	r = mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoRevocata {
		t.Fatalf("revoca: %+v", r.Blocchi)
	}
}

// firmaDiDB: la firma del perimetro del prodotto adesso.
func firmaDiDB(t *testing.T, p *pgxpool.Pool, thread, prodotto uuid.UUID) string {
	t.Helper()
	ctx := context.Background()
	st, err := confermabom.LeggiStatoRFQ(ctx, db.New(p), thread)
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.Firma(prodotto)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// D-PB-04 A [U] sulla bozza salvata: la PB iniziale del motore (S8 + registro) nel gruppo «motore intatta»; tolta
// dall'ingegnere diventa una «proposta rimossa».
func TestV1D39R4GruppiDiProvenienzaDB(t *testing.T) {
	p, s := poolPulito(t)
	id := idSintetici3()
	reg := componiProva3(t, id)
	k7 := ChiaveDellaVoce(id.allegato, id.rif("#7"))
	confermaS8DB(t, p, s, s.allegato, strings.Repeat("b", 64))
	c, err := creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
	if err != nil {
		t.Fatalf("crea step con S8: %v", err)
	}
	r := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 1), s.utente)
	if len(r.EffettiDocumentali.MotoreIntatta) != 1 || r.EffettiDocumentali.MotoreIntatta[0].AllegatoID != id.pdf ||
		r.EffettiDocumentali.MotoreIntatta[0].Importazione != reg.Impronta || r.EffettiDocumentali.MotoreIntatta[0].ChiaveBozza != k7 {
		t.Fatalf("motore intatta: %+v", r.EffettiDocumentali)
	}
	if !r.Sorgente.Valida || len(trova(r.Blocchi, BloccoStepSorgenteNonConfermata)) != 0 || r.Importazione == nil || r.Importazione.Impronta != reg.Impronta {
		t.Fatalf("sorgente e registro: %+v %v %+v", r.Sorgente, codici(r.Blocchi), r.Importazione)
	}
	// la pagina toglie la PB (svincolo dell'ingegnere): versione 2
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	m["documenti_preassociati"] = []any{}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 1, uuid.New(), m, []Azione{{Azione: AzFileSvincolato, ChiaveBozza: k7,
		Prima: mustJSON(map[string]any{"allegato_id": id.pdf, "chiave_bozza": k7}), Dopo: mustJSON(map[string]any{"allegato_id": id.pdf, "motivo": SvincoloUtente}),
		Origine: "utente"}}); err != nil {
		t.Fatalf("svincolo: %v", err)
	}
	r = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente)
	if len(r.EffettiDocumentali.MotoreIntatta) != 0 || len(r.EffettiDocumentali.ProposteRimosse) != 1 ||
		r.EffettiDocumentali.ProposteRimosse[0].AllegatoID != id.pdf || r.EffettiDocumentali.ProposteRimosse[0].Importazione != reg.Impronta {
		t.Fatalf("proposta rimossa: %+v", r.EffettiDocumentali)
	}
	// la stessa PB rimessa sul nodo proposto con il registro dichiarato (un suggerimento accettato) torna motore
	m["documenti_preassociati"] = []any{map[string]any{"allegato_id": id.pdf, "chiave_bozza": k7, "importazione": reg.Impronta}}
	m["bozza_id"], m["bozza_versione"] = c.BozzaID, 2
	r = mustRiepilogoDB(t, p, s.thread, m, s.utente)
	if len(r.EffettiDocumentali.MotoreIntatta) != 1 || len(r.EffettiDocumentali.ProposteRimosse) != 0 {
		t.Fatalf("suggerimento accettato: %+v", r.EffettiDocumentali)
	}
	// senza il registro dichiarato (la pagina la rifa' a mano) e' manuale
	m["documenti_preassociati"] = []any{map[string]any{"allegato_id": id.pdf, "chiave_bozza": k7}}
	r = mustRiepilogoDB(t, p, s.thread, m, s.utente)
	if len(r.EffettiDocumentali.Manuale) != 1 || r.DocumentiPreassociati[0].Provenienza != ProvenienzaManuale {
		t.Fatalf("rifatta a mano: %+v", r.EffettiDocumentali)
	}
}

// Prova 7 (dir. 35) nella parte di R4 su database: una AR attiva (gesto legacy, prima che la RFQ diventi formato 3) verso un
// altro componente dello stesso prodotto: blocco gia_associata_altrove; con lo svincolo nella bozza: sostituzione.
func TestV1D39R4Prova7DB(t *testing.T) {
	p, s := poolPulito(t)
	a := allegatoFileDB(t, p, s.thread, "ACM-1102.pdf", "ACM-1102", '7')
	x, y := s.sc.componenti["k-p1-1"], s.sc.componenti["k-p1-2"]
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(tx).BloccaThread(ctx, s.thread); err != nil {
		t.Fatal(err)
	}
	if _, err := associazioni.RegistraLegacy(ctx, db.New(tx), associazioni.Atto{Thread: s.thread, Autore: s.utente, Chiave: uuid.New()}, a, x); err != nil {
		t.Fatalf("AR legacy: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": PartenzaRegistrato})
	if err != nil {
		t.Fatalf("crea registrato: %v", err)
	}
	corpo := corpoDellaVersione(t, p, c.BozzaID, 1)
	corpo["documenti_preassociati"] = []any{map[string]any{"allegato_id": a, "chiave_bozza": ChiaveDelComponente(y)}}
	r := mustRiepilogoDB(t, p, s.thread, corpo, s.utente)
	b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab)
	if len(b) != 1 || b[0].Motivo != associazioni.MotivoGiaAssociataAltrove || b[0].Variante != associazioni.VarianteStessoProdotto ||
		b[0].ComponenteID == nil || *b[0].ComponenteID != x {
		t.Fatalf("gia' associata altrove: %+v", r.Blocchi)
	}
	corpo["decisioni"] = []any{map[string]any{"id": "d-sv", "tipo": "scollega_documento", "allegato_id": a, "componente_id": x}}
	r = mustRiepilogoDB(t, p, s.thread, corpo, s.utente)
	if len(trova(r.Blocchi, BloccoPreassociazioneNonApplicab)) != 0 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoSostituzione {
		t.Fatalf("sostituzione: %v %+v", codici(r.Blocchi), r.DocumentiPreassociati)
	}
	if len(r.EffettiDocumentali.SpostamentiAR) != 2 {
		t.Fatalf("spostamenti di AR: %+v", r.EffettiDocumentali.SpostamentiAR)
	}
}

// CICLO (PD35 INC-G13, D-ASS-02 [U]) su database: la bozza dell'altro finito mette il sottoassieme S1 sotto un suo particolare
// P, mentre la working del prodotto ha P sotto S1 (fuori dal perimetro dell'altro finito).
func TestV1D39R4CicloDB(t *testing.T) {
	p, s := poolPulito(t)
	s1, p11 := s.sc.componenti["k-s1"], s.sc.componenti["k-p1-1"]
	cl := EmittenteCliente(idDi("cliente"))
	reg := func(k string, id uuid.UUID, codice, tipo string) map[string]any {
		return map[string]any{"chiave_bozza": k, "componente_id": id, "op": "riusa", "tipo": tipo, "origine": "registrato",
			"identita": map[string]any{"emittente": cl, "emittente_presunto": true, "codice": codice, "rev": revDi(s, id), "modo": "registrato"}}
	}
	corpo := map[string]any{"formato": 3, "prodotto_id": s.altroFinito,
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": firmaDiDB(t, p, s.thread, s.altroFinito),
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-root", "componente_id": s.altroFinito, "op": "riusa", "tipo": "finito", "origine": "registrato",
				"identita": map[string]any{"emittente": cl, "emittente_presunto": true, "codice": "ACM-6000", "modo": "registrato"}},
			reg("k-p", p11, "ACM-1101", "sciolto"), reg("k-s", s1, "ACM-1100", "sottoassieme")},
		"archi":     []any{map[string]any{"padre": "k-root", "figlio": "k-p", "qta": 1, "origine": "utente"}, map[string]any{"padre": "k-p", "figlio": "k-s", "qta": 1, "origine": "utente"}},
		"decisioni": []any{map[string]any{"id": "d-d1", "tipo": "d1_a_sottoassieme", "chiave_bozza": "k-p"}}, "documenti_preassociati": []any{}}
	r := mustRiepilogoDB(t, p, s.thread, corpo, s.utente)
	if c := trova(r.Blocchi, BloccoCiclo); len(c) != 1 {
		t.Fatalf("ciclo: %v %+v", codici(r.Blocchi), r.Blocchi)
	}
}

// revDi: la rev registrata del componente della scena (vuota se non c'e').
func revDi(s scenaDB, id uuid.UUID) string {
	for _, n := range s.sc.snap.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == id {
			return n.Rev
		}
	}
	return ""
}
