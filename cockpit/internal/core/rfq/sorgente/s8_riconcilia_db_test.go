//go:build integrazione

// L4 — direttiva 39 §3 [U + T-39], §8 punto 2, corsia persistenza: S8 su una bozza manuale aperta. Il passo 3 non crea
// una seconda bozza: la stessa bozza prende una versione nuova (cambio-fonte-2 con la partenza step; la sorgente
// solo_prodotto con la partenza solo_prodotto), la versione manuale resta, la working non cambia. Il nodo registrato e il
// nodo manuale comuni allo STEP tengono la chiave e acquisiscono la casella, senza doppioni; la voce nuova entra con il
// suo arco. Poi la sorgente solo_prodotto diventa la struttura quando l'analisi finisce (S8 con la partenza step). Con la
// sorgente S8 valida e senza bozza, la partenza vuota si rifiuta (la strada e' S8). Dati sintetici: la scena di
// s8_db_test.go, con un registro di prova al posto del calcolo del motore.

package sorgente

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// figlioProva: una voce figlia della radice nel registro di prova.
type figlioProva struct {
	chiave, codice string
	componente     *uuid.UUID // la decisione_esistente, se c'e'
	qta            int
}

// registroConFigli: il registro della sola radice (registroRadice) con le voci figlie date, foglie, sotto la radice.
func registroConFigli(t *testing.T, s scenaS8, allegato uuid.UUID, sha string, figli []figlioProva) importazione.Registro {
	t.Helper()
	base := registroRadice(t, s, allegato, sha, "ACM-1000")
	c := base.Contenuto
	vero, falso := true, false
	radice := c.Gruppo.Radice
	c.Proposta.Nodi[0].Foglia = rawT(t, false)
	cliente := distintabozza.EmittenteCliente(s.cliente)
	for _, f := range figli {
		rif := "nodo:" + sha + ":" + f.chiave
		v := importazione.Voce{Voce: "n:" + allegato.String() + ":" + rif,
			Origine:  importazione.OrigineVoce{AllegatoID: allegato.String(), Sha256: sha, Chiave: f.chiave, Rif: rif},
			EntitaID: rawT(t, "e"+f.chiave), Foglia: rawT(t, true), RadiceDelFile: rawT(t, false), Leggibile: rawT(t, true),
			StatoMotore: importazione.StatoMotore{Raggiunto: &vero, Tolto: &falso, Deciso: &falso},
			Grezzi:      rawT(t, map[string]any{"id": f.chiave, "nome": f.codice, "formazioni": []any{}}),
			Padri:       rawT(t, []any{radice}), CodiceProposto: f.codice,
			IdentitaProposta: rawT(t, map[string]any{
				"chiave_lettura": allegato.String() + "/l:u:step:" + f.chiave, "unita_id": "u:" + f.chiave, "selettore": "nodo_step.id", "testo_unita": f.codice,
				"intervallo": map[string]any{"inizio": 0, "fine": len(f.codice)}, "letterale": f.codice, "emittente": cliente,
				"hash_grammatica": strings.Repeat("7", 64), "namespace": "acme", "famiglia": "num", "forma": "sei",
				"base":          map[string]any{"originale": f.codice, "normalizzata": f.codice, "segmenti": []any{}, "mancanti": []any{}, "completa": true},
				"qualificatori": map[string]any{}, "revisione": nil, "stato_forma": "letta", "funzione": "identita", "ruolo": "",
				"qualita": "piena", "motivi": []any{"forma_completa"}}),
			TipoProposto: rawT(t, map[string]any{"valore": "sciolto", "origine": "regola"}),
		}
		if f.componente != nil {
			v.DecisioneEsistente = rawT(t, map[string]any{"componente_id": f.componente.String(), "codice": f.codice})
			v.TipoProposto = rawT(t, map[string]any{"valore": "sottoassieme", "origine": "confermato"})
		}
		q := f.qta
		c.Proposta.Nodi = append(c.Proposta.Nodi, v)
		c.Proposta.Archi = append(c.Proposta.Archi, importazione.ArcoRegistro{AllegatoID: allegato.String(), Padre: radice, Figlio: rif, Quantita: &q})
	}
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

// salvaManuale: R2 salva sulla bozza della scena (corpo letto da LeggiRichiestaR2).
func salvaManuale(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, versione int, snap map[string]any, azioni []map[string]any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"bozza_id": bozza, "versione_attesa": versione, "prodotto_id": s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": snap, "azioni": azioni})
	if err != nil {
		t.Fatal(err)
	}
	r, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		t.Fatalf("corpo di R2: %v", err)
	}
	x := r.Salvataggio
	x.ThreadID, x.UtenteID = s.thread, s.utente
	if _, err := distintabozza.SalvaFormato3(context.Background(), p, x); err != nil {
		t.Fatalf("salvataggio della bozza manuale: %v", err)
	}
}

func snapshotVersione(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int) (distintabozza.Snapshot3, string) {
	t.Helper()
	var raw []byte
	var imp string
	if err := p.QueryRow(context.Background(), `SELECT snapshot, impronta FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).
		Scan(&raw, &imp); err != nil {
		t.Fatal(err)
	}
	s, err := distintabozza.LeggiSnapshot3(raw)
	if err != nil {
		t.Fatal(err)
	}
	return s, imp
}

func nodoDi(s distintabozza.Snapshot3, chiave string) (distintabozza.Nodo3, bool) {
	for _, n := range s.Nodi {
		if n.ChiaveBozza == chiave {
			return n, true
		}
	}
	return distintabozza.Nodo3{}, false
}

func TestV1D39S8RiconciliaLaBozzaManuale(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	cliente := distintabozza.EmittenteCliente(s.cliente)
	root := distintabozza.ChiaveDelComponente(s.prodotto)
	kf := distintabozza.ChiaveDelComponente(s.figlio)
	// la bozza manuale: il figlio della working riusato (registrato, senza casella), un nodo manuale, una PB sul manuale
	bozza := creaVuota(t, p, s)
	snap1, _ := snapshotVersione(t, p, bozza, 1)
	var m map[string]any
	b, _ := json.Marshal(snap1)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	figlio := map[string]any{"chiave_bozza": kf, "componente_id": s.figlio.String(), "op": "riusa", "tipo": "sottoassieme", "origine": "registrato",
		"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-1100", "modo": "registrato"}}
	man := map[string]any{"chiave_bozza": "m-staffa", "op": "crea", "tipo": "sciolto", "origine": "utente",
		"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-1150", "modo": "manuale"}}
	af := map[string]any{"padre": root, "figlio": kf, "qta": 1, "origine": "utente"}
	am := map[string]any{"padre": kf, "figlio": "m-staffa", "qta": 4, "origine": "utente"}
	pb := map[string]any{"allegato_id": s.pdf.String(), "chiave_bozza": "m-staffa"}
	m["nodi"] = append(m["nodi"].([]any), figlio, man)
	m["archi"] = append(m["archi"].([]any), af, am)
	m["documenti_preassociati"] = []any{pb}
	az := func(verbo, chiave string, dopo any) map[string]any {
		return map[string]any{"azione": verbo, "chiave_bozza": chiave, "prima": nil, "dopo": dopo, "origine": "utente"}
	}
	salvaManuale(t, p, s, bozza, 1, m, []map[string]any{az("nodo_aggiunto", kf, figlio), az("nodo_aggiunto", "m-staffa", man),
		az("arco_aggiunto", kf, af), az("arco_aggiunto", "m-staffa", am), az("file_preassociato", "m-staffa", pb)})
	_, impManuale := snapshotVersione(t, p, bozza, 2)
	prima := fotoVietate(t, p)

	// lo STEP B: la radice, il figlio della working (decisione_esistente), la staffa (stesso codice del nodo manuale), una
	// voce nuova
	figlioID := s.figlio
	regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#2", codice: "ACM-1100", componente: &figlioID, qta: 1},
		{chiave: "#3", codice: "ACM-1150", qta: 4}, {chiave: "#4", codice: "ACM-1160", qta: 2}})
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}}
	chiave := uuid.New()
	corpo := corpoStep(t, s, chiave, regB, map[string]any{"bozza": map[string]any{"bozza_id": bozza, "versione_attesa": 2}, "motivo": "arriva lo STEP"})
	es, err := s8(t, p, s, s.stepB, corpo, prep)
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil || es.Bozza.BozzaID != bozza || es.Bozza.Versione != 3 || es.Bozza.Importazione == nil {
		t.Fatalf("S8 sulla bozza manuale: %+v %v", es, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1`, s.prodotto); n != 1 {
		t.Fatalf("bozze del prodotto: %d (nessuna seconda bozza)", n)
	}
	if _, imp := snapshotVersione(t, p, bozza, 2); imp != impManuale {
		t.Fatal("la versione manuale e' cambiata")
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("S8 ha toccato la working: %s -> %s", prima, dopo)
	}
	s3, _ := snapshotVersione(t, p, bozza, 3)
	nf, _ := nodoDi(s3, kf)
	ns, _ := nodoDi(s3, "m-staffa")
	if len(nf.Caselle) != 1 || nf.Caselle[0].Rif != "nodo:"+s.shaB+":#2" || nf.Identita.Modo != distintabozza.ModoRegistrato ||
		len(ns.Caselle) != 1 || ns.Caselle[0].Rif != "nodo:"+s.shaB+":#3" || ns.Identita.Modo != distintabozza.ModoProposta || ns.Identita.Codice != "ACM-1150" {
		t.Fatalf("i nodi comuni: %+v %+v", nf, ns)
	}
	componenti, voce4 := 0, false
	for _, n := range s3.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == s.figlio {
			componenti++
		}
		voce4 = voce4 || (len(n.Caselle) == 1 && n.Caselle[0].Rif == "nodo:"+s.shaB+":#4")
	}
	if componenti != 1 || !voce4 || len(s3.Nodi) != 4 || len(s3.DocumentiPreassociati) != 1 || s3.DocumentiPreassociati[0].ChiaveBozza != "m-staffa" {
		t.Fatalf("doppioni, voce nuova o PB: %d %v %+v", componenti, voce4, s3)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_cambiata' AND prima IS NULL
		AND dopo->>'algoritmo' = 'cambio-fonte-2'`, bozza); n != 1 {
		t.Fatalf("sorgente_cambiata: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'scelta' AND bozza_id = $1 AND allegato_id = $2`, bozza, s.stepB); n != 1 {
		t.Fatalf("la scelta della bozza nella storia della fonte: %d", n)
	}
	if sv, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaB); !sv.Valida {
		t.Fatalf("la sorgente B: %+v", sv)
	}
	// il reinvio: riproposto
	if es2, err := s8(t, p, s, s.stepB, corpo, prep); err != nil || !es2.Riproposto {
		t.Fatalf("reinvio: %+v %v", es2, err)
	}
	// abbandonata la bozza, con la sorgente S8 valida la partenza vuota si rifiuta: la strada e' S8
	if _, err := distintabozza.Abbandona(ctx, p, distintabozza.Abbandono{ThreadID: s.thread, BozzaID: bozza, UtenteID: s.utente, VersioneAttesa: 3}); err != nil {
		t.Fatal(err)
	}
	_, err = distintabozza.CreaFormato3(ctx, p, distintabozza.Creazione3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s.prodotto, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea:   distintabozza.CreaR2{Partenza: distintabozza.PartenzaVuota}})
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, distintabozza.MotivoPartenzaNonAmmessa)
}

func TestV1D39S8SoloProdottoSullaBozzaManuale(t *testing.T) {
	p, s := preparaS8(t)
	bozza := creaVuota(t, p, s)
	prima := fotoVietate(t, p)
	solo := func(chiave uuid.UUID, extra map[string]any) []byte {
		c := map[string]any{"chiave_idempotenza": chiave, "prodotto_id": s.prodotto, "partenza": "solo_prodotto", "stato_analisi": "in_analisi",
			"presa_d_atto": true, "motivo": "STEP ancora in analisi", "bozza": map[string]any{"bozza_id": bozza, "versione_attesa": 1}}
		for k, v := range extra {
			c[k] = v
		}
		b, _ := json.Marshal(c)
		return b
	}
	es, err := s8(t, p, s, s.stepC, solo(uuid.New(), nil), Preparazione{StatoAnalisi: AnalisiInCorso})
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil || es.Bozza.BozzaID != bozza || es.Bozza.Versione != 2 || es.Bozza.Importazione != nil {
		t.Fatalf("solo_prodotto sulla bozza manuale: %+v %v", es, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1`, s.prodotto); n != 1 {
		t.Fatalf("bozze del prodotto: %d", n)
	}
	s2, _ := snapshotVersione(t, p, bozza, 2)
	if s2.Sorgente == nil || *s2.Sorgente.AllegatoID != s.stepC || s2.Sorgente.Gruppo != distintabozza.GruppoSoloProdotto || s2.Sorgente.Importazione != "" ||
		len(s2.Nodi) != 1 {
		t.Fatalf("la versione solo_prodotto: %+v", s2)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione`); n != 0 {
		t.Fatalf("registri: %d (niente registro fittizio)", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'scelta' AND gruppo = 'solo_prodotto' AND bozza_id = $1`, bozza); n != 1 {
		t.Fatalf("la scelta solo_prodotto della bozza: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_cambiata' AND prima IS NULL
		AND dopo->>'algoritmo' = 'solo-prodotto-1' AND dopo->>'presa_d_atto_analisi' = 'in_analisi'`, bozza); n != 1 {
		t.Fatalf("sorgente_cambiata solo_prodotto: %d", n)
	}
	// l'analisi finisce: S8 sullo stesso STEP con la partenza step fa il cambio della fonte sulla stessa bozza (la sorgente
	// solo_prodotto vale come assenza: le voci entrano)
	regC := registroConFigli(t, s, s.stepC, s.shaC, []figlioProva{{chiave: "#2", codice: "ACM-1180", qta: 1}})
	es, err = s8(t, p, s, s.stepC, corpoStep(t, s, uuid.New(), regC, map[string]any{"bozza": map[string]any{"bozza_id": bozza, "versione_attesa": 2}}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regC}})
	if err != nil || es.Bozza == nil || es.Bozza.BozzaID != bozza || es.Bozza.Versione != 3 {
		t.Fatalf("lo STEP analizzato: %+v %v", es, err)
	}
	s3, _ := snapshotVersione(t, p, bozza, 3)
	if s3.Sorgente.Gruppo != "bom" || len(s3.Nodi) != 2 {
		t.Fatalf("la struttura dopo l'analisi: %+v", s3)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'sostituzione' AND bozza_id = $1 AND precedente_gruppo = 'solo_prodotto'`, bozza); n != 1 {
		t.Fatalf("la sostituzione del gruppo riservato: %d", n)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("S8 ha toccato la working: %s -> %s", prima, dopo)
	}
}
