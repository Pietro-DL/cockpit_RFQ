//go:build integrazione

// L4 — R5 formato 3 completo, la ricodifica (direttiva 39, blocco «R5 completo» della corsia persistenza; Prova 4 della
// direttiva 35, D-ASS-01 [U]; EM §9.5 passo 5, H.5), sul database di prova. Dati sintetici (ACME, codici ACM-xxxx).

package distintabozza

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ricodificaInBozzaDB: la bozza da-conferma-1 dell'altro finito con la ricodifica del componente (codice prima → dopo) sul
// suo nodo (modifica, identita' della ricodifica) e le PB date sul nodo; restituisce la bozza, la versione e la chiave.
func ricodificaInBozzaDB(t *testing.T, p *pgxpool.Pool, s scenaDB, comp uuid.UUID, prima, dopo string, pb ...uuid.UUID) (uuid.UUID, int, string) {
	t.Helper()
	b, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, b.BozzaID, 1)
	em := EmittenteCliente(idDi("cliente"))
	d := map[string]any{"id": "d-ric", "tipo": "ricodifica", "componente_id": comp, "codice_prima": prima, "codice_dopo": dopo,
		"motivo": "il codice giusto del pezzo e' un altro", "emittente_dopo": em}
	k := ""
	var primaNodo, dopoNodo map[string]any
	for _, x := range m["nodi"].([]any) {
		n := x.(map[string]any)
		if id, _ := n["componente_id"].(string); id == comp.String() {
			k = n["chiave_bozza"].(string)
			primaNodo = map[string]any{"op": n["op"], "identita": n["identita"]}
			n["op"] = OpModifica
			n["identita"] = map[string]any{"emittente": em, "emittente_presunto": false, "codice": dopo, "modo": ModoDecisione, "decisione": "d-ric"}
			dopoNodo = map[string]any{"op": n["op"], "identita": n["identita"]}
		}
	}
	if k == "" {
		t.Fatalf("il componente %s non c'e' nella bozza da-conferma-1", comp)
	}
	m["decisioni"] = append(m["decisioni"].([]any), d)
	az := []Azione{azione3(t, AzDecisioneAggiunta, "", nil, d), azione3(t, AzNodoModificato, k, primaNodo, dopoNodo)}
	for _, a := range pb {
		x := map[string]any{"allegato_id": a, "chiave_bozza": k}
		m["documenti_preassociati"] = append(m["documenti_preassociati"].([]any), x)
		az = append(az, Azione{Azione: AzFilePreassociato, ChiaveBozza: k, Dopo: mustJSON(x), Origine: "utente"})
	}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, b.BozzaID, s.altroFinito, 1, uuid.New(), m, az); err != nil {
		t.Fatalf("R2 della ricodifica: %v", err)
	}
	return b.BozzaID, 2, k
}

// Prova 4 della direttiva 35 (D-ASS-01 [U]) nella parte di R5: la ricodifica di un componente registrato con una AR attiva.
// Una PB nuova sul nodo con il codice di prima resta in conflitto (codice_diverso, nessuna deroga); senza, R5 applica la
// ricodifica: il codice nella working, la AR sulla stessa riga, la copia allineata, la traccia in correzione_documento
// (ricodifica_bom); il componente certificato con il codice nuovo. Con un documento confermato del componente:
// RICODIFICA_FILE_NAS (documento_confermato) e R5 422 senza scritture.
func TestV1D39R5Prova4RicodificaDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	_, pezzo, c1 := primaConfermaDB(t, p, s)
	var ar, pdf uuid.UUID
	if err := p.QueryRow(ctx, `SELECT associazione_id, allegato_id FROM file_rfq_associazione WHERE componente_id = $1 AND stato = 'attiva'`, pezzo).Scan(&ar, &pdf); err != nil {
		t.Fatalf("la AR della prima conferma: %v", err)
	}
	vecchio := allegatoFileDB(t, p, s.thread, "ACM-6500_B.pdf", "ACM-6500", '7')
	bozza, v, _ := ricodificaInBozzaDB(t, p, s, pezzo, "ACM-6500", "ACM-6501", vecchio)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	if b := trova(rp.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Motivo != "codice_diverso" || len(rp.Blocchi) != 1 {
		t.Fatalf("la PB con il codice di prima resta in conflitto: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	// tolta la PB in conflitto (lo stesso gesto della pagina: file_svincolato), la ricodifica si conferma
	m, _ := snapshotDellaVersione(t, p, bozza, v)
	var k string
	restano := []any{}
	for _, x := range m["documenti_preassociati"].([]any) {
		if x.(map[string]any)["allegato_id"] == vecchio.String() {
			k = x.(map[string]any)["chiave_bozza"].(string)
			continue
		}
		restano = append(restano, x)
	}
	m["documenti_preassociati"] = restano
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, bozza, s.altroFinito, v, uuid.New(), m, []Azione{azione3(t, AzFileSvincolato, k,
		map[string]any{"allegato_id": vecchio, "chiave_bozza": k}, map[string]any{"allegato_id": vecchio, "motivo": SvincoloUtente})}); err != nil {
		t.Fatalf("R2 svincolo della PB: %v", err)
	}
	v++
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4 della ricodifica: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 della ricodifica: %v", err)
	}
	if len(out.IdentitaRicodificate) != 1 || out.IdentitaRicodificate[0] != pezzo || out.CorrezioniRicodifica != 1 || out.NumeroConferma != 2 {
		t.Fatalf("risposta: %+v", out)
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND codice = 'ACM-6501'`, pezzo); n != 1 {
		t.Fatal("il codice nuovo nella working")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE associazione_id = $1 AND stato = 'attiva' AND componente_id = $2
		AND conferma_id = $3`, ar, pezzo, c1.ConfermaID); n != 1 {
		t.Fatal("la AR resta sulla stessa riga, con la sua conferma d'origine")
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id = $2 AND codice = 'ACM-6501'`, pdf, pezzo); n != 1 {
		t.Fatal("la copia allineata al codice nuovo")
	}
	if n := conta3(t, p, `SELECT count(*) FROM correzione_documento WHERE associazione_id = $1 AND origine = 'ricodifica_bom' AND codice_prima = 'ACM-6500'
		AND codice_dopo = 'ACM-6501' AND motivo = $3 AND componente_id = $2 AND autore_id = $4`, ar, pezzo, "il codice giusto del pezzo e' un altro", s.utente); n != 1 {
		t.Fatal("la traccia della ricodifica in correzione_documento")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_componente WHERE conferma_id = $1 AND componente_id = $2 AND codice = 'ACM-6501'
		AND emittente_presunto = false`, out.ConfermaID, pezzo); n != 1 {
		t.Fatal("il componente certificato con il codice nuovo")
	}
	// un documento confermato del componente: RICODIFICA_FILE_NAS (documento_confermato), R5 422 senza scritture
	esegui3(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'disegno_2d', 'ACM-6501', 'ACM-6501.pdf', 'pdf', repeat('7', 64), '2D/ACM-6501.pdf', $3)`, s.thread, pezzo, s.utente)
	b3, v3, _ := ricodificaInBozzaDB(t, p, s, pezzo, "ACM-6501", "ACM-6502")
	rp3 := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, b3, v3), s.utente)
	if b := trova(rp3.Blocchi, BloccoRicodificaFileNAS); len(b) != 1 || b[0].Motivo != MotivoRicodificaDocumentoConfermato {
		t.Fatalf("RICODIFICA_FILE_NAS con un documento confermato: %v %+v", codici(rp3.Blocchi), rp3.Blocchi)
	}
	prima := fotoR5(t, p)
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, b3, v3, rp3.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	rifiutoR5Di(t, err, CodiceBlocchi)
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("R5 rifiutata ha scritto in %v", d)
	}
}
