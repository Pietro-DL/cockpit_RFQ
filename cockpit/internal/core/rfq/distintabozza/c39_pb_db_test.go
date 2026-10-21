//go:build integrazione

// L4 — direttiva 39 §4 (D-PB01…04, D-AR11, D-AR14), corsia persistenza, sul database di prova con i registri di prova
// al posto del calcolo del motore: le PB iniziali del server solo con la sorgente confermata da S8 (non con la sola fonte
// legacy), le ambigue non applicate, i suggerimenti (preassociazioni_proposte[]) nella risposta e in R1, la provenienza
// dopo una correzione e dopo un suggerimento accettato, la PB rimasta con il registro A dopo il passaggio a B, la
// conversione delle pre-assegnazioni legacy (P1) alla presa in carico. Dati sintetici.

package distintabozza

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// confermaS8DB: la conferma della sorgente di S8 per lo STEP del prodotto con quel contenuto (il documento cad_3d e la
// riga conferma_documento con la chiave del passo 2, come la scrive sorgente.ConfermaS8): la sorgente e' valida per R5.
func confermaS8DB(t *testing.T, p *pgxpool.Pool, s scenaDB, allegato uuid.UUID, sha string) {
	t.Helper()
	var doc uuid.UUID
	if err := p.QueryRow(context.Background(), `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256,
		path_relativo, confermato_da) VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-1000.stp', 'stp', $3, '3D/ACM-1000.stp', $4) RETURNING documento_id`,
		s.thread, s.prodotto, sha, s.utente).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.prodotto, allegato, sha, doc, s.utente)
}

// pbDi: la PB dell'allegato nello snapshot (zero se non c'e').
func pbDi(t *testing.T, raw json.RawMessage, a uuid.UUID) Preassociazione3 {
	t.Helper()
	s, err := LeggiSnapshot3(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range s.DocumentiPreassociati {
		if p.AllegatoID == a {
			return p
		}
	}
	return Preassociazione3{}
}

// snapshotDellaVersione: lo snapshot salvato di una versione, come mappa (per il salvataggio successivo) e come byte.
func snapshotDellaVersione(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int) (map[string]any, json.RawMessage) {
	t.Helper()
	var raw json.RawMessage
	if err := p.QueryRow(context.Background(), `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m, raw
}

// spostaPB: il salvataggio R2 che sposta la PB dell'allegato da un nodo a un altro (svincolo utente e preassociazione),
// con lo snapshot nuovo senza i campi del server (la pagina non li ha riletti) o con il registro dichiarato.
func spostaPB(t *testing.T, sc scena3DB, bozza uuid.UUID, versione int, a uuid.UUID, da, a2, registro string) (Salvato3, error) {
	t.Helper()
	m, _ := snapshotDellaVersione(t, sc.p, bozza, versione)
	pb := map[string]any{"allegato_id": a, "chiave_bozza": a2}
	if registro != "" {
		pb["importazione"] = registro
	}
	var tenute []any
	for _, x := range m["documenti_preassociati"].([]any) {
		if x.(map[string]any)["allegato_id"] != a.String() {
			tenute = append(tenute, x)
		}
	}
	m["documenti_preassociati"] = append(tenute, pb)
	return sc.salva3Su(t, bozza, sc.prodotto, versione, uuid.New(), m, []Azione{
		{Azione: AzFileSvincolato, ChiaveBozza: da, Prima: mustJSON(map[string]any{"allegato_id": a, "chiave_bozza": da}),
			Dopo: mustJSON(map[string]any{"allegato_id": a, "motivo": SvincoloUtente}), Origine: "utente"},
		{Azione: AzFilePreassociato, ChiaveBozza: a2, Dopo: mustJSON(map[string]any{"allegato_id": a, "chiave_bozza": a2}), Origine: "utente"},
	})
}

func TestV1D39PBInizialiEProvenienzaDB(t *testing.T) {
	p, s := poolPulito(t)
	id := idSintetici3()
	reg := componiProva3(t, id) // la voce #7 propone id.pdf (= s.allegato2), univoca
	k7, k9 := ChiaveDellaVoce(id.allegato, id.rif("#7")), ChiaveDellaVoce(id.allegato, id.rif("#9"))
	root := ChiaveDelComponente(s.prodotto)
	confermaS8DB(t, p, s, s.allegato, strings.Repeat("b", 64))
	c, err := creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
	if err != nil {
		t.Fatalf("crea step con S8: %v", err)
	}
	// D-PB01 A: la proposta univoca e' una PB iniziale motore con il registro d'origine
	if pb := pbDi(t, c.Snapshot, id.pdf); pb != (Preassociazione3{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaMotore, Importazione: reg.Impronta}) {
		t.Fatalf("PB iniziale: %+v", pb)
	}
	if len(c.PreassociazioniProposte) != 0 {
		t.Fatalf("suggerimenti alla nascita: %+v", c.PreassociazioniProposte)
	}
	// D-PB03 A: il giornale tiene la provenienza (bozza_aperta.dopo.preassociazioni_iniziali)
	ee := eventiDB(t, p, c.BozzaID, 1)
	if !strings.Contains(campoJSON(t, ee[0].Dopo, "preassociazioni_iniziali"), `"provenienza":"motore"`) ||
		!strings.Contains(campoJSON(t, ee[0].Dopo, "preassociazioni_iniziali"), reg.Impronta) {
		t.Fatalf("bozza_aperta: %s", ee[0].Dopo)
	}
	// R1: nessun suggerimento (la proposta e' applicata)
	l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{})
	if err != nil || l.PreassociazioniProposte == nil || len(*l.PreassociazioniProposte) != 0 {
		t.Fatalf("R1 alla nascita: %v %+v", err, l.PreassociazioniProposte)
	}
	sc := scena3DB{scenaDB: s, p: p}
	// la correzione dell'ingegnere: spostata sulla radice → corretta, conserva il registro d'origine
	if _, err := spostaPB(t, sc, c.BozzaID, 1, id.pdf, k7, root, ""); err != nil {
		t.Fatalf("correzione: %v", err)
	}
	_, raw := snapshotDellaVersione(t, p, c.BozzaID, 2)
	if pb := pbDi(t, raw, id.pdf); pb.Provenienza != ProvenienzaCorretta || pb.Importazione != reg.Impronta || pb.ChiaveBozza != root {
		t.Fatalf("dopo la correzione: %+v", pb)
	}
	ee = eventiDB(t, p, c.BozzaID, 3)
	if len(ee) != 2 || ee[0].Azione != AzFileSvincolato || campoJSON(t, ee[0].Prima, "provenienza") != `"motore"` ||
		ee[1].Azione != AzFilePreassociato || campoJSON(t, ee[1].Dopo, "provenienza") != `"corretta"` ||
		campoJSON(t, ee[1].Dopo, "importazione") != `"`+reg.Impronta+`"` {
		t.Fatalf("eventi della correzione: %+v", ee)
	}
	// R1: la proposta del registro su #7 e' un suggerimento pb_toccata, fuori da da_rivedere[]
	l, err = leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{Eventi: true})
	if err != nil || len(*l.PreassociazioniProposte) != 1 {
		t.Fatalf("R1 dopo la correzione: %v %+v", err, l.PreassociazioniProposte)
	}
	if x := (*l.PreassociazioniProposte)[0]; x.AllegatoID != id.pdf || x.ChiaveBozza != k7 || x.Motivo != PropostaPBToccata || x.Importazione != reg.Impronta ||
		x.Provenienza != ProvenienzaMotore || strings.Contains(string(l.DaRivedere), id.pdf.String()) {
		t.Fatalf("suggerimento: %+v %s", x, l.DaRivedere)
	}
	// un «suggerimento» che il registro non contiene (un file nuovo con il registro dichiarato): 422
	// proposta_non_corrispondente, niente scritto; la PB spostata che rimanda il registro che aveva resta corretta
	altroPDF := nuovoAllegato(t, p, idDi("msg-0"), 12, "ACM-1100.pdf", strings.Repeat("a", 64))
	m0, _ := snapshotDellaVersione(t, p, c.BozzaID, 2)
	nuovaPB := map[string]any{"allegato_id": altroPDF, "chiave_bozza": k9, "importazione": reg.Impronta}
	m0["documenti_preassociati"] = append(m0["documenti_preassociati"].([]any), nuovaPB)
	_, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 2, uuid.New(), m0, []Azione{{Azione: AzFilePreassociato, ChiaveBozza: k9,
		Dopo: mustJSON(map[string]any{"allegato_id": altroPDF, "chiave_bozza": k9}), Origine: "utente"}})
	codice3(t, err, CodiceCorpoNonValido, MotivoPropostaNonCorrispondente)
	// un registro che non e' del prodotto: importazione_sconosciuta
	_, err = spostaPB(t, sc, c.BozzaID, 2, id.pdf, root, k7, strings.Repeat("9", 64))
	codice3(t, err, CodiceCorpoNonValido, MotivoImportazioneSconosciuta)
	// una provenienza messa dalla pagina: provenienza_dal_server
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 2)
	for _, x := range m["documenti_preassociati"].([]any) {
		x.(map[string]any)["provenienza"] = "manuale"
		delete(x.(map[string]any), "importazione")
	}
	_, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 2, uuid.New(), m, nil)
	codice3(t, err, CodiceCorpoNonValido, MotivoProvenienzaDelServer)
	// il suggerimento accettato sul nodo proposto: torna motore con il registro
	if _, err := spostaPB(t, sc, c.BozzaID, 2, id.pdf, root, k7, reg.Impronta); err != nil {
		t.Fatalf("suggerimento accettato: %v", err)
	}
	_, raw = snapshotDellaVersione(t, p, c.BozzaID, 3)
	if pb := pbDi(t, raw, id.pdf); pb.Provenienza != ProvenienzaMotore || pb.Importazione != reg.Impronta || pb.ChiaveBozza != k7 {
		t.Fatalf("dopo il suggerimento: %+v", pb)
	}
	// un salvataggio che non tocca le PB e le rimanda senza i campi del server: restano com'erano
	m, _ = snapshotDellaVersione(t, p, c.BozzaID, 3)
	for _, x := range m["documenti_preassociati"].([]any) {
		delete(x.(map[string]any), "provenienza")
		delete(x.(map[string]any), "importazione")
	}
	if sv, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 3, uuid.New(), m, nil); err != nil || sv.NuovaVersione {
		t.Fatalf("salvataggio senza modifiche: %v %+v", err, sv)
	}

	// il passaggio a B (la copia dello stesso STEP, confermata per contenuto): la PB rimasta tiene il registro A
	copia := nuovoAllegato(t, p, idDi("msg-0"), 10, "ACM-1000_copia.stp", strings.Repeat("b", 64))
	regB := componi(t, spostaRegistro(t, registroProva3(t, id), id.allegato, copia, "", ""))
	v, err := cambiaDB(t, p, s, c.BozzaID, 3, uuid.New(), copia, regB, nil)
	if err != nil {
		t.Fatalf("cambio verso B: %v", err)
	}
	if pb := pbDi(t, v.Snapshot, id.pdf); pb.Provenienza != ProvenienzaMotore || pb.Importazione != reg.Impronta || pb.ChiaveBozza != k7 ||
		regB.Impronta == reg.Impronta {
		t.Fatalf("PB rimasta dopo il passaggio a B: %+v (B %s)", pb, regB.Impronta)
	}
	if len(v.Importazione.PreassociazioniIniziali) != 0 || len(v.Importazione.PreassociazioniProposte) != 0 {
		t.Fatalf("il cambio non scrive PB nuove: %+v", v.Importazione)
	}
	// R1 sulla versione con B: la proposta di B e' gia' applicata dalla PB di A (stesso nodo): nessun suggerimento
	l, err = leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{})
	if err != nil || len(*l.PreassociazioniProposte) != 0 {
		t.Fatalf("R1 dopo B: %v %+v", err, l.PreassociazioniProposte)
	}
	// nessuna AR, nessun documento nuovo, nessuna copia: le PB restano nella bozza fino a R5
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != 0 {
		t.Fatalf("AR scritte: %d", n)
	}
}

func TestV1D39PBInizialiSenzaS8AmbigueEConflittiDB(t *testing.T) {
	id := idSintetici3()
	t.Run("senza S8 nessuna PB del motore", func(t *testing.T) {
		p, s := poolPulito(t)
		reg := componiProva3(t, id)
		// la fonte confermata alla maniera legacy (documento senza S8): crea passa, ma il motore non inizializza PB (D-AR11 B)
		esegui3(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-1000.stp', 'stp', repeat('b', 64), '3D/ACM-1000.stp', $3)`, s.thread, s.prodotto, s.utente)
		c, err := creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if pb := pbDi(t, c.Snapshot, id.pdf); pb.AllegatoID != uuid.Nil || len(c.PreassociazioniProposte) != 0 {
			t.Fatalf("PB senza S8: %+v %+v", pb, c.PreassociazioniProposte)
		}
		l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{})
		if err != nil || len(*l.PreassociazioniProposte) != 0 {
			t.Fatalf("R1 senza S8: %v %+v", err, l.PreassociazioniProposte)
		}
		// la PB manuale resta possibile senza S8, ed e' manuale
		sc := scena3DB{scenaDB: s, p: p}
		m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
		k7 := ChiaveDellaVoce(id.allegato, id.rif("#7"))
		pb := map[string]any{"allegato_id": id.pdf, "chiave_bozza": k7}
		m["documenti_preassociati"] = []any{pb}
		if _, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 1, uuid.New(), m, []Azione{{Azione: AzFilePreassociato, ChiaveBozza: k7, Dopo: mustJSON(pb), Origine: "utente"}}); err != nil {
			t.Fatal(err)
		}
		_, raw := snapshotDellaVersione(t, p, c.BozzaID, 2)
		if x := pbDi(t, raw, id.pdf); x.Provenienza != ProvenienzaManuale || x.Importazione != "" {
			t.Fatalf("PB manuale: %+v", x)
		}
	})
	t.Run("proposta ambigua non applicata", func(t *testing.T) {
		p, s := poolPulito(t)
		amb := registroConProposte(t, id, map[string][]uuid.UUID{id.rif("#7"): {id.pdf}, id.rif("#9"): {id.pdf}})
		confermaS8DB(t, p, s, s.allegato, strings.Repeat("b", 64))
		c, err := creaStepDB(t, p, s, uuid.New(), amb, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if pb := pbDi(t, c.Snapshot, id.pdf); pb.AllegatoID != uuid.Nil || len(c.PreassociazioniProposte) != 0 {
			t.Fatalf("ambigua: %+v %+v", pb, c.PreassociazioniProposte)
		}
	})
	t.Run("AR attiva altrove: suggerimento ar_attiva", func(t *testing.T) {
		p, s := poolPulito(t)
		reg := componiProva3(t, id)
		var altro uuid.UUID
		for k, comp := range s.sc.componenti {
			if k != "k-root" {
				altro = comp
				break
			}
		}
		arAttivaDiProva(t, p, s.thread, s.prodotto, id.pdf, altro, s.utente)
		confermaS8DB(t, p, s, s.allegato, strings.Repeat("b", 64))
		c, err := creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if pb := pbDi(t, c.Snapshot, id.pdf); pb.AllegatoID != uuid.Nil {
			t.Fatalf("PB con una AR altrove: %+v", pb)
		}
		if len(c.PreassociazioniProposte) != 1 || c.PreassociazioniProposte[0].Motivo != PropostaARAttiva {
			t.Fatalf("suggerimenti: %+v", c.PreassociazioniProposte)
		}
		ee := eventiDB(t, p, c.BozzaID, 1)
		if !strings.Contains(campoJSON(t, ee[0].Dopo, "preassociazioni_proposte"), `"ar_attiva"`) {
			t.Fatalf("bozza_aperta: %s", ee[0].Dopo)
		}
	})
}

func TestV1D39ConversioneP1AllaPresaInCaricoDB(t *testing.T) {
	p, s := poolPulito(t)
	// tre pre-assegnazioni aperte: un PDF sulla radice (si converte), lo STEP del prodotto su un altro allegato (P1s: resta
	// candidata come sorgente), un PDF su un componente del perimetro che la partenza vuota non ha (resta, fuori dalla bozza)
	pdf := nuovoAllegato(t, p, idDi("msg-0"), 20, "ACM-1000.pdf", strings.Repeat("d", 64))
	nuovaProposta(t, p, pdf, s.thread, &s.prodotto)
	nuovaProposta(t, p, s.allegato2, s.thread, &s.prodotto)
	var sotto uuid.UUID
	for k, comp := range s.sc.componenti {
		if k != "k-root" {
			sotto = comp
			break
		}
	}
	pdf2 := nuovoAllegato(t, p, idDi("msg-0"), 21, "ACM-1100.pdf", strings.Repeat("f", 64))
	nuovaProposta(t, p, pdf2, s.thread, &sotto)
	chiave := uuid.New()
	c, err := creaDB(t, p, s, s.prodotto, chiave, map[string]any{"partenza": "vuota"})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	root := ChiaveDelComponente(s.prodotto)
	if pb := pbDi(t, c.Snapshot, pdf); pb != (Preassociazione3{AllegatoID: pdf, ChiaveBozza: root, Provenienza: ProvenienzaLegacy}) {
		t.Fatalf("PB legacy: %+v", pb)
	}
	if pb := pbDi(t, c.Snapshot, s.allegato2); pb.AllegatoID != uuid.Nil {
		t.Fatalf("lo STEP del finito e' diventato una PB: %+v", pb)
	}
	if len(c.ConversioniLegacy) != 1 || c.ConversioniLegacy[0].AllegatoID != pdf || c.ConversioniLegacy[0].ComponentePrima != s.prodotto {
		t.Fatalf("conversioni: %+v", c.ConversioniLegacy)
	}
	motivi := map[uuid.UUID]string{}
	for _, x := range c.PreassegnazioniNonConvertite {
		motivi[x.AllegatoID] = x.Motivo
	}
	if len(motivi) != 2 || motivi[s.allegato2] != P1StepDelFinito || motivi[pdf2] != P1FuoriBozza {
		t.Fatalf("non convertite: %+v", c.PreassegnazioniNonConvertite)
	}
	// l'evento conversione_legacy con componente_prima, senza AR ne' conferma, e la copia tolta; nessuna AR
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE allegato_id = $1 AND origine = 'conversione_legacy' AND azione = 'sgancia'
		AND componente_prima = $2 AND associazione_id IS NULL AND conferma_id IS NULL AND autore_id = $3`, pdf, s.prodotto, s.utente); n != 1 {
		t.Fatalf("evento conversione_legacy: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id IS NULL`, pdf); n != 1 {
		t.Fatal("la copia della P1 convertita non e' tolta")
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = ANY($1) AND componente_id IS NOT NULL`, []uuid.UUID{s.allegato2, pdf2}); n != 2 {
		t.Fatalf("le P1 non convertite sono cambiate: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != 0 {
		t.Fatalf("AR di backfill: %d", n)
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	if !strings.Contains(campoJSON(t, ee[0].Dopo, "conversioni_legacy"), pdf.String()) ||
		!strings.Contains(campoJSON(t, ee[0].Dopo, "preassociazioni_iniziali"), `"provenienza":"legacy"`) {
		t.Fatalf("bozza_aperta: %s", ee[0].Dopo)
	}
	// il reinvio ritrova la stessa versione e gli elenchi, senza scrivere di nuovo
	c2, err := creaDB(t, p, s, s.prodotto, chiave, map[string]any{"partenza": "vuota"})
	if err != nil || !c2.Riproposto || len(c2.ConversioniLegacy) != 1 || len(c2.PreassegnazioniNonConvertite) != 2 {
		t.Fatalf("reinvio: %v %+v", err, c2)
	}
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE origine = 'conversione_legacy'`); n != 1 {
		t.Fatalf("il reinvio ha scritto di nuovo: %d", n)
	}
	// la PB legacy si sposta a mano: diventa manuale (rifatta dall'ingegnere)
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	man := map[string]any{"chiave_bozza": "man-1", "op": "crea", "tipo": "commerciale", "origine": "utente",
		"identita": map[string]any{"emittente": EmittenteCliente(idDi("cliente")), "emittente_presunto": true, "codice": "ACM-1999", "modo": "manuale"}}
	m["nodi"] = append(m["nodi"].([]any), man)
	m["archi"] = append(m["archi"].([]any), map[string]any{"padre": root, "figlio": "man-1", "qta": 1, "origine": "utente"})
	m["documenti_preassociati"] = []any{map[string]any{"allegato_id": pdf, "chiave_bozza": "man-1"}}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 1, uuid.New(), m, []Azione{
		{Azione: AzNodoAggiunto, ChiaveBozza: "man-1", Dopo: mustJSON(man), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: "man-1", Dopo: mustJSON(map[string]any{"padre": root, "figlio": "man-1", "qta": 1, "origine": "utente"}), Origine: "utente"},
		{Azione: AzFileSvincolato, ChiaveBozza: root, Prima: mustJSON(map[string]any{"allegato_id": pdf, "chiave_bozza": root}),
			Dopo: mustJSON(map[string]any{"allegato_id": pdf, "motivo": SvincoloUtente}), Origine: "utente"},
		{Azione: AzFilePreassociato, ChiaveBozza: "man-1", Dopo: mustJSON(map[string]any{"allegato_id": pdf, "chiave_bozza": "man-1"}), Origine: "utente"},
	}); err != nil {
		t.Fatalf("PB legacy spostata: %v", err)
	}
	_, raw := snapshotDellaVersione(t, p, c.BozzaID, 2)
	if pb := pbDi(t, raw, pdf); pb.Provenienza != ProvenienzaManuale || pb.ChiaveBozza != "man-1" {
		t.Fatalf("PB legacy spostata: %+v", pb)
	}
	ee = eventiDB(t, p, c.BozzaID, 2)
	for _, e := range ee {
		if e.Azione == AzFileSvincolato && campoJSON(t, e.Prima, "provenienza") != `"legacy"` {
			t.Fatalf("lo svincolo della legacy: %s", e.Prima)
		}
	}
}

// Nei comandi espliciti (riallinea_proposta, con la sorgente confermata da S8): la proposta univoca nuova del registro B
// diventa una PB motore con il registro B (evento file_preassociato del server prima di proposta_riallineata, non
// annullabile), la PB rimasta tiene il registro A; il reinvio ritrova gli elenchi dall'evento.
func TestV1D39PBNeiComandiEspliciti(t *testing.T) {
	p, s := poolPulito(t)
	id := idSintetici3()
	regA := componiProva3(t, id)
	k7, k9 := ChiaveDellaVoce(id.allegato, id.rif("#7")), ChiaveDellaVoce(id.allegato, id.rif("#9"))
	confermaS8DB(t, p, s, s.allegato, strings.Repeat("b", 64))
	c, err := creaStepDB(t, p, s, uuid.New(), regA, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	nuovoPDF := nuovoAllegato(t, p, idDi("msg-0"), 30, "ACM-1109.pdf", strings.Repeat("7", 64))
	cb := registroProva3(t, id)
	for i := range cb.Proposta.Nodi {
		if cb.Proposta.Nodi[i].Origine.Rif == id.rif("#9") {
			cb.Proposta.Nodi[i].AssociazioniProposte = rawDi(t, []any{map[string]any{"allegato_id": nuovoPDF.String(), "motivo": "nome", "livello": "alto"}})
		}
	}
	cb.AllegatiCitati = append(cb.AllegatiCitati, nuovoPDF.String())
	sort.Strings(cb.AllegatiCitati)
	regB := componi(t, cb)
	chiave := uuid.New()
	v, err := riallineaDB(t, p, s, c.BozzaID, 1, chiave, regB, regB.Contenuto.ImprontaContenuto, nil)
	if err != nil {
		t.Fatalf("riallinea: %v", err)
	}
	if pb := pbDi(t, v.Snapshot, nuovoPDF); pb != (Preassociazione3{AllegatoID: nuovoPDF, ChiaveBozza: k9, Provenienza: ProvenienzaMotore, Importazione: regB.Impronta}) {
		t.Fatalf("PB nuova del comando: %+v", pb)
	}
	if pb := pbDi(t, v.Snapshot, id.pdf); pb.Importazione != regA.Impronta || pb.Provenienza != ProvenienzaMotore || pb.ChiaveBozza != k7 {
		t.Fatalf("PB rimasta: %+v", pb)
	}
	if len(v.Importazione.PreassociazioniIniziali) != 1 || v.Importazione.PreassociazioniIniziali[0].AllegatoID != nuovoPDF {
		t.Fatalf("risposta: %+v", v.Importazione)
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	var pre, fin *EventoLetto
	for i := range ee {
		switch ee[i].Azione {
		case AzFilePreassociato:
			pre = &ee[i]
		case AzPropostaRiallineata:
			fin = &ee[i]
		}
	}
	if pre == nil || fin == nil || pre.Origine != "server" || pre.N > fin.N || campoJSON(t, pre.Dopo, "importazione") != `"`+regB.Impronta+`"` ||
		!strings.Contains(campoJSON(t, fin.Dopo, "preassociazioni_iniziali"), nuovoPDF.String()) {
		t.Fatalf("eventi: %+v %+v", pre, fin)
	}
	// il reinvio: stessa versione, gli elenchi dall'evento
	v2, err := riallineaDB(t, p, s, c.BozzaID, 1, chiave, regB, regB.Contenuto.ImprontaContenuto, nil)
	if err != nil || !v2.Riproposto || len(v2.Importazione.PreassociazioniIniziali) != 1 {
		t.Fatalf("reinvio: %v %+v", err, v2.Importazione)
	}
	// l'annullamento dell'evento del server non e' ammesso (la pila si ferma all'ultima versione del server)
	sc := scena3DB{scenaDB: s, p: p}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 2)
	_, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 2, uuid.New(), m, []Azione{{Azione: AzAnnulla, Dopo: mustJSON(map[string]any{"n": pre.N}), Origine: "utente"}})
	if err == nil {
		t.Fatal("annullare una PB del server: atteso un rifiuto")
	}
}

var _ = importazione.FormatoRegistro
