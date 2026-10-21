//go:build integrazione

// L4 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: crea con la partenza step sul database di prova, con il
// registro di prova (formato3_aiuti_test.go) al posto del calcolo del motore, che chi chiama fa prima della transazione:
// FONTE_NON_CONFERMATA (U2 §2.8), V-7, l'occorrenza e lo sha256 dichiarati, il motivo della scelta della fonte, la
// versione 1 importazione-1 con il registro scritto e gli eventi del server, la storia della fonte, nessuna scrittura
// nelle tabelle della working, delle proposte e delle associazioni (nessuna PA diventa PB), il reinvio, il salvataggio
// successivo, la ripartenza con il registro riusato, un file del registro messo in Altri file. Dati sintetici.

package distintabozza

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// creaStepDB: crea con la partenza step dal corpo di R2 e la proposta data.
func creaStepDB(t *testing.T, p *pgxpool.Pool, s scenaDB, chiave uuid.UUID, reg importazione.Registro, sorgente map[string]any, motivo string,
	abbandona map[string]any) (Creata, error) {
	t.Helper()
	corpo := map[string]any{"partenza": "step", "sorgente": sorgente, "impronta_contenuto": reg.Contenuto.ImprontaContenuto}
	if motivo != "" {
		corpo["motivo"] = motivo
	}
	if abbandona != nil {
		corpo["abbandona"] = abbandona
	}
	r := formaR2DB(t, s.prodotto, nil, 0, chiave, FormaCrea, corpo)
	return CreaFormato3(context.Background(), p, r.CreaDaRichiesta(s.thread, s.utente, &PropostaCalcolata{Registro: reg}))
}

// contiTabelleVietate: le righe delle tabelle che R2 non scrive mai (EM §9.2, «Effetti persistenti»; direttiva 35 §8 C-3).
func contiTabelleVietate(t *testing.T, p *pgxpool.Pool) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tab := range []string{"componente", "componente_relazione", "componente_proposta", "relazione_proposta", "documento_proposta",
		"associazione_evento", "file_rfq_associazione", "correzione_documento", "documento"} {
		out[tab] = conta3(t, p, `SELECT count(*) FROM `+tab)
	}
	return out
}

func TestV1C3CreaStep(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	id := idSintetici3()
	reg := componiProva3(t, id)
	sorgente := map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}
	// senza la fonte confermata del prodotto: FONTE_NON_CONFERMATA, niente scritto
	prima := contiTabelleVietate(t, p)
	_, err := creaStepDB(t, p, s, uuid.New(), reg, sorgente, "", nil)
	codice3(t, err, CodiceFonteNonConfermata, "")
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza`); n != 0 {
		t.Fatalf("il rifiuto ha scritto una bozza: %d", n)
	}
	// la conferma della fonte (il documento STEP del prodotto, come la scrive «Conferma la fonte»)
	esegui3(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-1000.stp', 'stp', repeat('b', 64), '3D/ACM-1000.stp', $3)`, s.thread, s.prodotto, s.utente)
	prima = contiTabelleVietate(t, p)
	// V-7: l'impronta dichiarata non e' quella del registro
	r := formaR2DB(t, s.prodotto, nil, 0, uuid.New(), FormaCrea, map[string]any{"partenza": "step", "sorgente": sorgente, "impronta_contenuto": strings.Repeat("0", 64)})
	_, err = CreaFormato3(ctx, p, r.CreaDaRichiesta(s.thread, s.utente, &PropostaCalcolata{Registro: reg}))
	if e := codice3(t, err, CodiceAnteprimaSuperata, MotivoPropostaStep); e.ImprontaContenuto != reg.Contenuto.ImprontaContenuto || e.Anteprima != MotivoPropostaStep {
		t.Fatalf("V-7: %+v", e)
	}
	// l'occorrenza dichiarata non e' quella del gruppo calcolato; lo sha256 dichiarato non e' quello dello STEP
	_, err = creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato2, "gruppo": "bom"}, "", nil)
	if e := codice3(t, err, CodiceCorpoNonValido, MotivoGruppoSuAltraOccorrenza); e.AllegatoID == nil || *e.AllegatoID != s.allegato {
		t.Fatalf("altra occorrenza: %+v", e)
	}
	_, err = creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato, "gruppo": "bom", "sha256": strings.Repeat("c", 64)}, "", nil)
	if e := codice3(t, err, CodiceCorpoNonValido, ""); e.Campo != "crea.sorgente.sha256" {
		t.Fatalf("sha256: %+v", e)
	}
	// il codice letto dalla radice non coincide con quello del prodotto: il motivo e' obbligatorio
	diverso := registroProva3(t, id)
	diverso.Proposta.Nodi[0].CodiceProposto = "ACM-1001"
	regDiverso, err := importazione.Componi(diverso)
	if err != nil {
		t.Fatal(err)
	}
	_, err = creaStepDB(t, p, s, uuid.New(), regDiverso, sorgente, "", nil)
	if e := codice3(t, err, CodiceMotivoObbligatorio, ""); e.Campo != "crea.motivo" {
		t.Fatalf("motivo: %+v", e)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_importazione`); n != 0 {
		t.Fatalf("i rifiuti hanno scritto un registro: %d", n)
	}
	// la partenza step
	chiave := uuid.New()
	c, err := creaStepDB(t, p, s, chiave, reg, sorgente, "", nil)
	if err != nil {
		t.Fatalf("crea step: %v", err)
	}
	if c.Partenza != PartenzaStep || c.Eventi != 2 || c.Importazione == nil || c.Importazione.Riusata || c.Importazione.Impronta != reg.Impronta ||
		len(c.Importazione.FuoriPerDecisione) != 1 {
		t.Fatalf("esito: %+v %+v", c, c.Importazione)
	}
	snap, err := LeggiSnapshot3(c.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	// la radice, cinque voci meno la radice e la voce fuori, e i 34 confermati senza STEP del gruppo bom (P-6)
	if snap.Sorgente == nil || snap.Sorgente.Importazione != reg.Impronta || len(snap.Nodi) != 1+4+34 || len(snap.DocumentiPreassociati) != 0 ||
		len(snap.Decisioni) != 0 {
		t.Fatalf("snapshot: %d nodi %+v", len(snap.Nodi), snap.Sorgente)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND importazione_id IS NOT NULL`, c.BozzaID); n != 1 {
		t.Fatalf("la versione 1 con il registro: %d", n)
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	if len(ee) != 2 || ee[0].Azione != AzBozzaAperta || campoJSON(t, ee[0].Dopo, "importazione") != `"`+reg.Impronta+`"` ||
		ee[1].Azione != AzSorgenteScelta || ee[1].Origine != "server" || campoJSON(t, ee[1].Dopo, "importazione") != `"`+reg.Impronta+`"` {
		t.Fatalf("eventi: %+v", ee)
	}
	if n := conta3(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'scelta' AND ruolo = 'prodotto' AND bozza_id = $2
		AND allegato_id = $3 AND gruppo = 'bom'`, s.prodotto, c.BozzaID, s.allegato); n != 1 {
		t.Fatalf("la storia della fonte: %d", n)
	}
	if dopo := contiTabelleVietate(t, p); !mappeUguali(prima, dopo) {
		t.Fatalf("R2 crea ha scritto fuori dalla bozza: %v → %v", prima, dopo)
	}
	if _, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{Importazione: true}); err != nil {
		t.Fatalf("R1 dopo crea step: %v", err)
	}
	// il reinvio: riproposto, il registro riusato, niente scritto
	versioni := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`)
	c2, err := creaStepDB(t, p, s, chiave, reg, sorgente, "", nil)
	if err != nil || !c2.Riproposto || c2.BozzaID != c.BozzaID || c2.Partenza != PartenzaStep || c2.Importazione == nil || !c2.Importazione.Riusata {
		t.Fatalf("reinvio: %v %+v", err, c2)
	}
	if conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`) != versioni {
		t.Fatal("il reinvio ha scritto")
	}
	// il salvataggio successivo: la versione del server passa le verifiche di R2 (il nodo proposto cambiato e' V-1)
	var m map[string]any
	if err := json.Unmarshal(c.Snapshot, &m); err != nil {
		t.Fatal(err)
	}
	k7 := ChiaveDellaVoce(id.allegato, id.rif("#7"))
	for _, x := range m["nodi"].([]any) {
		if n := x.(map[string]any); n["chiave_bozza"] == k7 {
			n["descrizione"] = "Telaio saldato"
		}
	}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 1, uuid.New(), m, []Azione{{Azione: AzNodoModificato, ChiaveBozza: k7,
		Prima: json.RawMessage(`{"descrizione":null}`), Dopo: json.RawMessage(`{"descrizione":"Telaio saldato"}`), Origine: "utente"}}); err != nil {
		t.Fatalf("salvataggio dopo crea step: %v", err)
	}
	// la ripartenza dalla stessa sorgente: il registro si riusa, la bozza di prima si chiude «ripartita dalla proposta»
	rp, err := creaStepDB(t, p, s, uuid.New(), reg, sorgente, "", map[string]any{"bozza_id": c.BozzaID, "versione_attesa": 2, "motivo": "si riparte"})
	if err != nil || rp.Importazione == nil || !rp.Importazione.Riusata || rp.BozzaAbbandonata == nil {
		t.Fatalf("ripartenza: %v %+v", err, rp)
	}
	var chiusura string
	if err := p.QueryRow(ctx, `SELECT motivo_chiusura FROM distinta_bozza WHERE bozza_id = $1`, c.BozzaID).Scan(&chiusura); err != nil {
		t.Fatal(err)
	}
	if chiusura != "ripartita dalla proposta nella bozza "+rp.BozzaID.String() || conta3(t, p, `SELECT count(*) FROM distinta_importazione`) != 1 {
		t.Fatalf("ripartenza: %q", chiusura)
	}
	// un file del registro messo in Altri file prima della transazione: 409 ANTEPRIMA_SUPERATA, niente scritto
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: rp.BozzaID, UtenteID: s.utente, VersioneAttesa: 1}); err != nil {
		t.Fatal(err)
	}
	inAltriFile(t, p, s.allegato2, s.utente)
	_, err = creaStepDB(t, p, s, uuid.New(), reg, sorgente, "", nil)
	codice3(t, err, CodiceAnteprimaSuperata, MotivoPropostaStep)
}

func mappeUguali(a, b map[string]int) bool {
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
