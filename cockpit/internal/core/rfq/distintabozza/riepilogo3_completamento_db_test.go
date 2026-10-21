//go:build integrazione

// L4 — il completamento di R4 formato 3 (direttiva 39, blocco «R4 completo» della corsia persistenza), sul database di
// prova, sempre in una transazione REPEATABLE READ READ ONLY:
//   - CANONICO_COLLIDE con le regole del cliente lette dal database (suffisso decorativo), e niente senza le regole;
//   - le esclusioni fatte nelle pagine di prima (origine legacy) e le righe da decidere dello STEP sorgente (condizione (c)
//     di prima-istanza-1), le rimozioni aperte, gli ingressi fuori firma.
// Dati sintetici.

package distintabozza

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

// EM §5.2 W5, §8.1 riga 11: la bozza manuale con il codice «ACM-6500» e il componente registrato «ACM-6500_PRT»; le regole
// del cliente con il suffisso decorativo _PRT fanno nascere CANONICO_COLLIDE, senza regole niente.
func TestV1D39R4CanonicoDB(t *testing.T) {
	p, s := poolPulito(t)
	prt := uuid.New()
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6500_PRT', 'sciolto', 'manuale', $3)`,
		prt, s.thread, s.utente)
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	id := map[string]any{"emittente": radice["identita"].(map[string]any)["emittente"], "emittente_presunto": true, "codice": "ACM-6500", "modo": "manuale"}
	m["nodi"] = append(m["nodi"].([]any), map[string]any{"chiave_bozza": "k-man", "op": "crea", "tipo": "sciolto", "origine": "utente", "identita": id})
	m["archi"] = []any{map[string]any{"padre": radice["chiave_bozza"], "figlio": "k-man", "qta": 1, "origine": "utente"}}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{
		{Azione: AzNodoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(m["nodi"].([]any)[1]), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(m["archi"].([]any)[0]), Origine: "utente"},
	}); err != nil {
		t.Fatalf("R2: %v", err)
	}
	r := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente)
	if n := len(trova(r.Blocchi, BloccoCanonicoCollide)); n != 0 {
		t.Fatalf("senza regole del cliente: %v", codici(r.Blocchi))
	}
	esegui3(t, p, `UPDATE cliente SET regole = '{"suffissi_decorativi": ["_PRT"]}'::jsonb WHERE cliente_id = $1`, idDi("cliente"))
	r2 := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente)
	cc := trova(r2.Blocchi, BloccoCanonicoCollide)
	if len(cc) != 1 || cc[0].ChiaveBozza != "k-man" || cc[0].ComponenteID == nil || *cc[0].ComponenteID != prt || cc[0].ChiaveWorking != "ACM-6500" {
		t.Fatalf("CANONICO_COLLIDE: %+v (blocchi %v)", cc, codici(r2.Blocchi))
	}
	if r2.FirmaAnteprima == r.FirmaAnteprima || r2.Confermabile {
		t.Fatal("il blocco del canonico cambia la firma e non e' confermabile")
	}
}

// EM §7.4, U2 §5.4, §6.1: lo STEP sorgente confermato da S8 con le righe legacy: la riga scartata da una persona nelle
// pagine di prima e' un'esclusione legacy (con la voce esclusione_legacy), la riga aperta conta fra le righe da decidere
// (c); una rimozione aperta fra due componenti della bozza conta (c); gli ingressi stanno fuori dalla firma.
func TestV1D39R4CompletamentoDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	sha := shaProva('c')
	var doc uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-6000', 'ACM-6000.stp', 'stp', $3, '3D/ACM-6000.stp', $4) RETURNING documento_id`,
		s.thread, s.altroFinito, sha, s.utente).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.altroFinito, s.allegato2, sha, doc, s.utente)
	// le righe legacy dello STEP sorgente: la radice, una scartata da una persona, una aperta, una chiusa da un automatismo
	for _, x := range []struct {
		chiave, stato string
		persona       bool
		nota          string
	}{{"#1", "aperta", false, ""}, {"#5", "scartata", true, "elemento di montaggio"}, {"#6", "aperta", false, ""}, {"#8", "scartata", false, "superato"}} {
		var deciso any
		if x.persona {
			deciso = s.utente
		}
		var nota any
		if x.nota != "" {
			nota = x.nota
		}
		esegui3(t, p, `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza, stato, deciso_da, nota)
			VALUES ($1, $2, $3, $4, 'pezzo', 'step', 50, $5::stato_proposta, $6, $7)`, s.thread, s.allegato2, sha, x.chiave, x.stato, deciso, nota)
	}
	// la working del prodotto: un sottoassieme con un figlio, e una rimozione aperta fra i due
	pz := uuid.New()
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6101', 'sciolto', 'manuale', $3)`,
		pz, s.thread, s.utente)
	esegui3(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 1, 'manuale', $5),
		($1, $3, $4, 2, 'manuale', $5)`, s.thread, s.altroFinito, s.sotto, pz, s.utente)
	esegui3(t, p, `INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 2)`,
		s.thread, doc, s.sotto, pz)
	em := EmittenteCliente(idDi("cliente"))
	reg := func(chiave string, id uuid.UUID, codice, tipo string) map[string]any {
		return map[string]any{"chiave_bozza": chiave, "componente_id": id, "op": "riusa", "tipo": tipo, "origine": "registrato",
			"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": codice, "modo": "registrato"}}
	}
	corpo := map[string]any{"formato": 3, "prodotto_id": s.altroFinito,
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": conta3(t, p, `SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0)`, s.thread)},
		"firma_base":        firmaDiDB(t, p, s.thread, s.altroFinito),
		"sorgente":          map[string]any{"allegato_id": s.allegato2, "sha256": sha, "gruppo": GruppoSoloProdotto},
		"nodi": []any{reg(ChiaveDelComponente(s.altroFinito), s.altroFinito, "ACM-6000", "finito"), reg("k-sa", s.sotto, "ACM-6100", "sottoassieme"),
			reg("k-pz", pz, "ACM-6101", "sciolto")},
		"archi": []any{map[string]any{"padre": ChiaveDelComponente(s.altroFinito), "figlio": "k-sa", "qta": 1, "origine": "utente"},
			map[string]any{"padre": "k-sa", "figlio": "k-pz", "qta": 2, "origine": "utente"}},
		"decisioni": []any{}, "documenti_preassociati": []any{}}
	r := mustRiepilogoDB(t, p, s.thread, corpo, s.utente)
	if len(r.Blocchi) != 0 {
		t.Fatalf("blocchi: %+v", r.Blocchi)
	}
	var legacy []EsclusoR4
	for _, e := range r.Esclusi {
		if e.Origine == OrigineEsclusoLegacy {
			legacy = append(legacy, e)
		}
	}
	if len(legacy) != 1 || legacy[0].Casella.Rif != "nodo:"+sha+":#5" || legacy[0].AutoreID == nil || *legacy[0].AutoreID != s.utente ||
		legacy[0].Motivo != "elemento di montaggio" {
		t.Fatalf("esclusione legacy: %+v", legacy)
	}
	voce := false
	for _, v := range r.RegistroDecisioni {
		voce = voce || v.Tipo == VoceEsclusioneLegacy
	}
	if !voce {
		t.Fatal("la voce esclusione_legacy manca nel registro delle decisioni")
	}
	v := r.VerificaPrevista
	if v.Stato != confermabom.StatoVerificaCalcolata || v.Verificata == nil || *v.Verificata || !contiene(v.Motivi, confermabom.MotivoRigheDaDecidere) ||
		!contiene(v.Motivi, confermabom.MotivoRimozioniAperte) {
		t.Fatalf("verifica prevista con le righe da decidere e la rimozione aperta: %+v", v)
	}
	// gli ingressi: solo con ConIngressi, fuori dalla firma
	raw := mustJSON(corpo)
	rich, err := LeggiRichiestaR4(raw)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	ri, err := Riepilogo3(ctx, db.New(tx), s.thread, rich, OpzioniR4{UtenteID: s.utente, ConIngressi: true, ImprontaRegole: "regole-prova",
		AvvisiMotore: true, Motore: &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{}}})
	if err != nil {
		t.Fatal(err)
	}
	if ri.Ingressi == nil || ri.Ingressi.ImprontaIngressi == "" || ri.FirmaAnteprima != r.FirmaAnteprima || r.Ingressi != nil {
		t.Fatalf("ingressi fuori firma: %+v %s %s", ri.Ingressi, ri.FirmaAnteprima, r.FirmaAnteprima)
	}
}
