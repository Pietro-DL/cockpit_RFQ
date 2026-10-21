//go:build integrazione

// L4 — G39-02 [U] (20/10 sera), corsia persistenza parte A: lo STEP candidato sorgente del finito si riconosce con il Motore
// A (StepCandidatiDalMotore, qui un riconoscimento di prova al posto del calcolo vero: la prova con il motore vero e le
// regole ACME e' in transport/web). Con il calcolo, il riconoscimento e' quello del motore, non la lettura legacy di
// documento_proposta: le PB iniziali non toccano lo STEP che il motore riconosce; R2 rifiuta la PB su uno STEP che solo il
// motore riconosce e accetta quella su uno STEP che solo la lettura legacy diceva del finito; R4 con il motore blocca.
// Dati sintetici.

package distintabozza

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// stepDelMotore: il riconoscimento del motore di prova: gli allegati dati, candidati sorgente univoci del prodotto (come
// li darebbe StepCandidatiDalMotore per uno STEP legato al solo prodotto; giro G39-ter: la PB su una candidatura non
// univoca verso una parte e' la scelta dell'utente, G39-06 [U], e la prova di R2 e R4 vuole quella univoca).
func stepDelMotore(prodotto uuid.UUID, aa ...uuid.UUID) *StepCandidati {
	s := &StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{}, Univoche: map[uuid.UUID]uuid.UUID{}}
	for _, a := range aa {
		s.Prodotti[a] = []uuid.UUID{prodotto}
		s.Univoche[a] = prodotto
	}
	return s
}

// manualeConPBAdesso: come manualeConPB, con il calcolo di adesso nel salvataggio.
func manualeConPBAdesso(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto, a uuid.UUID, adesso *CalcoloAdesso) (uuid.UUID, error) {
	t.Helper()
	c, err := creaDB(t, p, s, prodotto, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	id := map[string]any{"emittente": radice["identita"].(map[string]any)["emittente"], "emittente_presunto": true, "codice": "ACM-6500", "modo": "manuale"}
	nodo := map[string]any{"chiave_bozza": "k-man", "op": "crea", "tipo": "sciolto", "origine": "utente", "identita": id}
	arco := map[string]any{"padre": radice["chiave_bozza"], "figlio": "k-man", "qta": 2, "origine": "utente"}
	m["nodi"] = append(m["nodi"].([]any), nodo)
	m["archi"] = []any{arco}
	x := map[string]any{"allegato_id": a, "chiave_bozza": "k-man"}
	m["documenti_preassociati"] = []any{x}
	raw, err := json.Marshal(map[string]any{"bozza_id": c.BozzaID, "versione_attesa": 1, "prodotto_id": prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m, "azioni": []Azione{
			{Azione: AzNodoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(nodo), Origine: "utente"},
			{Azione: AzArcoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(arco), Origine: "utente"},
			{Azione: AzFilePreassociato, ChiaveBozza: "k-man", Dopo: mustJSON(x), Origine: "utente"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := LeggiRichiestaR2(raw)
	if err != nil {
		t.Fatal(err)
	}
	sv := r.Salvataggio
	sv.ThreadID, sv.UtenteID, sv.Adesso = s.thread, s.utente, adesso
	// la stessa domanda che fa l'adattatore prima della transazione
	tx, err := p.BeginTx(context.Background(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	registro, step, err := MotoreDelSalvataggio(context.Background(), db.New(tx), sv)
	_ = tx.Rollback(context.Background())
	if err != nil || registro || !step {
		t.Fatalf("MotoreDelSalvataggio: registro %v, step %v, %v (attesi falso, vero)", registro, step, err)
	}
	_, err = SalvaFormato3(context.Background(), p, sv)
	return c.BozzaID, err
}

func TestV1G39_02PBInizialiConIlMotoreDB(t *testing.T) {
	p, s := poolPulito(t)
	id := idSintetici3()
	reg := componiProva3(t, id) // la voce #7 propone id.pdf (= s.allegato2, uno STEP: ACM-1000_bis.stp), univoca
	confermaS8DB(t, p, s, s.allegato, strings.Repeat("b", 64))
	corpo := map[string]any{"partenza": "step", "sorgente": map[string]any{"allegato_id": s.allegato, "gruppo": "bom"},
		"impronta_contenuto": reg.Contenuto.ImprontaContenuto}
	r := formaR2DB(t, s.prodotto, nil, 0, uuid.New(), FormaCrea, corpo)
	// il motore riconosce lo STEP come candidato sorgente del prodotto: ne' PB iniziale ne' suggerimento
	c, err := CreaFormato3(context.Background(), p, r.CreaDaRichiesta(s.thread, s.utente,
		&PropostaCalcolata{Registro: reg, StepCandidati: stepDelMotore(s.prodotto, id.pdf)}))
	if err != nil {
		t.Fatalf("crea step: %v", err)
	}
	if pb := pbDi(t, c.Snapshot, id.pdf); pb.AllegatoID != uuid.Nil || len(c.PreassociazioniProposte) != 0 {
		t.Fatalf("lo STEP riconosciuto dal motore e' diventato una PB o un suggerimento: %+v %+v", pb, c.PreassociazioniProposte)
	}
	// R1 con il calcolo di adesso (lo stesso registro, lo stesso riconoscimento): nemmeno fra i suggerimenti; e' fra gli
	// STEP candidati del prodotto (l'avviso della pagina)
	l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{Adesso: &CalcoloAdesso{Registro: &reg, Step: stepDelMotore(s.prodotto, id.pdf)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range *l.PreassociazioniProposte {
		if x.AllegatoID == id.pdf {
			t.Fatalf("lo STEP del finito fra i suggerimenti di R1: %+v", x)
		}
	}
	if l.StepCandidati == nil || len(*l.StepCandidati) != 1 || (*l.StepCandidati)[0].AllegatoID != id.pdf || !(*l.StepCandidati)[0].DalMotore {
		t.Fatalf("step_candidati di R1: %+v", l.StepCandidati)
	}
	// lo STEP sorgente legato (s.allegato) non e' un candidato da avvisare: e' gia' confermato
	l, err = leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{Adesso: &CalcoloAdesso{Step: stepDelMotore(s.prodotto, s.allegato)}})
	if err != nil || l.StepCandidati == nil || len(*l.StepCandidati) != 0 {
		t.Fatalf("lo STEP gia' confermato fra gli step_candidati: %+v %v", l.StepCandidati, err)
	}
}

func TestV1G39_02R2ER4ConIlMotoreDB(t *testing.T) {
	p, s := poolPulito(t)
	// uno STEP che solo il motore riconosce (nessuna proposta di documento): la PB manuale e' rifiutata con il calcolo
	soloMotore := stepDB(t, p, s.thread, "ACM-6000-vista.stp", '7')
	_, err := manualeConPBAdesso(t, p, s, s.altroFinito, soloMotore, &CalcoloAdesso{Step: stepDelMotore(s.altroFinito, soloMotore)})
	var e *Errore
	if !errors.As(err, &e) || e.Codice != CodiceStepCandidatoDelFinito || e.AllegatoID == nil || *e.AllegatoID != soloMotore {
		t.Fatalf("lo STEP riconosciuto dal motore: atteso 422 %s, avuto %v", CodiceStepCandidatoDelFinito, err)
	}
	// senza il calcolo (il ripiego) quello STEP passa: la lettura legacy non lo dice; R4 con il motore lo blocca (la bozza
	// di un terzo finito, perche' quella del primo e' rimasta aperta)
	terzo := uuid.New()
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-3000', 'finito', 'manuale', $3)`,
		terzo, s.thread, s.utente)
	bozza, err := manualeConPB(t, p, s, terzo, soloMotore)
	if err != nil {
		t.Fatalf("lo stesso STEP senza il calcolo: %v", err)
	}
	corpo := corpoDellaVersione(t, p, bozza, 2)
	if rp := mustRiepilogoDB(t, p, s.thread, corpo, s.utente); len(trovaMotivo(rp.Blocchi, BloccoPreassociazioneNonApplicab, "step_candidato_del_finito")) != 0 {
		t.Fatalf("R4 senza il riconoscimento del motore: %v", codici(rp.Blocchi))
	}
	raw, err := json.Marshal(corpo)
	if err != nil {
		t.Fatal(err)
	}
	rq, err := LeggiRichiestaR4(raw)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.BeginTx(context.Background(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	rp, err := Riepilogo3(context.Background(), db.New(tx), s.thread, rq, OpzioniR4{UtenteID: s.utente, AvvisiMotore: true,
		Motore: &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{StepCandidati: stepDelMotore(terzo, soloMotore)}}})
	if err != nil {
		t.Fatal(err)
	}
	if g := trovaMotivo(rp.Blocchi, BloccoPreassociazioneNonApplicab, "step_candidato_del_finito"); len(g) != 1 || g[0].AllegatoID == nil || *g[0].AllegatoID != soloMotore {
		t.Fatalf("R4 con il motore: %v", codici(rp.Blocchi))
	}
	_ = tx.Rollback(context.Background())
	// uno STEP che solo la lettura legacy diceva del finito (ACM-2000 contro ACM-2000Z), che il motore non riconosce: con il
	// calcolo la PB passa (il riconoscimento e' del motore, non di documento_proposta)
	_, legacy := finitoConMarcatore(t, p, s)
	if _, err := manualeConPBAdesso(t, p, s, s.prodotto, legacy, &CalcoloAdesso{Step: stepDelMotore(s.altroFinito, soloMotore)}); err != nil {
		t.Fatalf("lo STEP della sola lettura legacy, con il calcolo del motore: %v", err)
	}
}

// trovaMotivo: i rilievi con il codice e il motivo.
func trovaMotivo(rr []Rilievo, codice, motivo string) []Rilievo {
	var out []Rilievo
	for _, r := range rr {
		if r.Codice == codice && r.Motivo == motivo {
			out = append(out, r)
		}
	}
	return out
}
