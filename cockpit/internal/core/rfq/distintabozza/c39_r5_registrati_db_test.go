//go:build integrazione

// L4 — R5 formato 3 completo, le parti dei registrati (direttiva 39, blocco «R5 completo» della corsia persistenza; EM §5.2
// W1, W3, §9.5), sul database di prova: D1 su un registrato (il tipo nella working), il riuso di un archiviato (il
// ripristino), due nodi dello stesso componente (due pezzi nuovi con lo stesso codice: un componente con due archi; un
// pezzo nuovo risolto sul registrato gia' nella bozza: un altro arco dello stesso componente), poi cambia_tipo verso
// commerciale di un registrato senza autorizzazione STEP. Dati sintetici (ACME, codici ACM-xxxx).

package distintabozza

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func TestV1D39R5RegistratiDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	_, pezzo, _ := primaConfermaDB(t, p, s)
	// un componente archiviato della RFQ, che la bozza riusa
	arch := uuid.New()
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6530', 'sciolto', 'manuale', $3)`,
		arch, s.thread, s.utente)
	esegui3(t, p, `UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'prova' WHERE componente_id = $1`, arch, s.utente)
	b, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, b.BozzaID, 1)
	em := EmittenteCliente(idDi("cliente"))
	rk, kp := ChiaveDelComponente(s.altroFinito), ""
	for _, x := range m["nodi"].([]any) {
		n := x.(map[string]any)
		switch n["componente_id"] {
		case s.altroFinito.String():
			rk = n["chiave_bozza"].(string)
		case pezzo.String():
			kp = n["chiave_bozza"].(string)
		}
	}
	if kp == "" {
		t.Fatal("il pezzo registrato non c'e' nella bozza")
	}
	crea := func(k, codice, tipo string) map[string]any {
		return map[string]any{"chiave_bozza": k, "op": "crea", "tipo": tipo, "origine": "utente",
			"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": codice, "modo": "manuale"}}
	}
	arco := func(pa, fi string, q int) map[string]any {
		return map[string]any{"padre": pa, "figlio": fi, "qta": q, "origine": "utente"}
	}
	nuovi := []any{crea("k-arc", "ACM-6530", "sciolto"), crea("k-sa", "ACM-6550", "sottoassieme"), crea("k-n1", "ACM-6540", "sciolto"),
		crea("k-n2", "ACM-6540", "sciolto"), crea("k-p2", "ACM-6500", "sciolto"), crea("k-c", "ACM-6520", "sciolto")}
	archi := []any{arco(rk, "k-arc", 1), arco(rk, "k-sa", 1), arco(rk, "k-n1", 5), arco("k-sa", "k-n2", 1), arco("k-sa", "k-p2", 3), arco(kp, "k-c", 4)}
	dec := []any{map[string]any{"id": "ru-arc", "tipo": "riusa_esistente", "chiave_bozza": "k-arc", "componente_id": arch},
		map[string]any{"id": "ru-p2", "tipo": "riusa_esistente", "chiave_bozza": "k-p2", "componente_id": pezzo},
		map[string]any{"id": "d1-p", "tipo": "d1_a_sottoassieme", "chiave_bozza": kp}}
	m["nodi"] = append(m["nodi"].([]any), nuovi...)
	m["archi"] = append(m["archi"].([]any), archi...)
	m["decisioni"] = append(m["decisioni"].([]any), dec...)
	var az []Azione
	for _, x := range nuovi {
		az = append(az, azione3(t, AzNodoAggiunto, x.(map[string]any)["chiave_bozza"].(string), nil, x))
	}
	for _, x := range archi {
		az = append(az, azione3(t, AzArcoAggiunto, x.(map[string]any)["figlio"].(string), nil, x))
	}
	for _, x := range dec {
		az = append(az, azione3(t, AzDecisioneAggiunta, "", nil, x))
	}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, b.BozzaID, s.altroFinito, 1, uuid.New(), m, az); err != nil {
		t.Fatalf("R2 dei registrati: %v", err)
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, b.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, b.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	n1, n2, sa, c := out.Mappa["k-n1"], out.Mappa["k-n2"], out.Mappa["k-sa"], out.Mappa["k-c"]
	if n1 == uuid.Nil || n1 != n2 || out.Mappa["k-p2"] != pezzo || out.Mappa["k-arc"] != arch || len(out.IdentitaCreate) != 3 ||
		len(out.IdentitaRipristinate) != 1 || out.IdentitaRipristinate[0] != arch || len(out.TipiCambiati) != 1 || out.TipiCambiati[0] != pezzo {
		t.Fatalf("risposta: %+v", out)
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND archiviato_il IS NULL`, arch); n != 1 {
		t.Fatal("l'archiviato riusato e' di nuovo nella working")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND tipo = 'sottoassieme'`, pezzo); n != 1 {
		t.Fatal("D1 sul registrato: il tipo nella working")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE thread_id = $1 AND upper(codice) = 'ACM-6540'`, s.thread); n != 1 {
		t.Fatal("due pezzi nuovi con lo stesso codice sono un componente solo")
	}
	for _, x := range []struct {
		padre, figlio uuid.UUID
		qta           int
	}{{s.altroFinito, n1, 5}, {sa, n1, 1}, {sa, pezzo, 3}, {s.altroFinito, pezzo, 2}, {pezzo, c, 4}, {s.altroFinito, arch, 1}} {
		if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2 AND qta = $3`, x.padre, x.figlio, x.qta); n != 1 {
			t.Fatalf("arco della working %s → %s ×%d", x.padre, x.figlio, x.qta)
		}
		if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_arco WHERE conferma_id = $1 AND padre_id = $2 AND figlio_id = $3 AND qta = $4`,
			out.ConfermaID, x.padre, x.figlio, x.qta); n != 1 {
			t.Fatalf("arco certificato %s → %s ×%d", x.padre, x.figlio, x.qta)
		}
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_componente WHERE conferma_id = $1 AND componente_id IN ($2, $3)`, out.ConfermaID, n1, pezzo); n != 2 {
		t.Fatalf("una riga certificata per componente: %d", n)
	}
	// cambia_tipo verso commerciale di un registrato senza autorizzazione STEP, con l'effetto letto dalla scheda
	b3, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1 (terza): %v", err)
	}
	e, err := fascicolo.EffettoCambioTipo(ctx, db.New(p), s.thread, arch, db.TipoComponenteCommerciale)
	if err != nil || e.Spento != "" {
		t.Fatalf("effetto del cambio di tipo: %+v %v", e, err)
	}
	m3, _ := snapshotDellaVersione(t, p, b3.BozzaID, 1)
	ct := map[string]any{"id": "ct-arc", "tipo": "cambia_tipo", "componente_id": arch, "tipo_nuovo": "commerciale", "firma_effetto": e.Firma}
	m3["decisioni"] = append(m3["decisioni"].([]any), ct)
	if _, err := sc.salva3Su(t, b3.BozzaID, s.altroFinito, 1, uuid.New(), m3, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ct)}); err != nil {
		t.Fatalf("R2 cambia_tipo: %v", err)
	}
	rp3 := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, b3.BozzaID, 2), s.utente)
	if len(rp3.Blocchi) != 0 {
		t.Fatalf("R4 cambia_tipo: %v %+v", codici(rp3.Blocchi), rp3.Blocchi)
	}
	out3, err := confermaR5DB(t, p, s, richiestaR5(t, p, b3.BozzaID, 2, rp3.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 cambia_tipo: %v", err)
	}
	if len(out3.TipiCambiati) != 1 || out3.TipiCambiati[0] != arch {
		t.Fatalf("risposta cambia_tipo: %+v", out3)
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND tipo = 'commerciale'`, arch); n != 1 {
		t.Fatal("cambia_tipo: il tipo nella working")
	}
}
