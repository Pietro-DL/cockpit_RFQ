//go:build integrazione

// L4 — D-48-01 A [T] (preparata in attesa della risposta), sul database di prova: la PB iniziale del motore su un file con
// il nome «da partner» (la lettura generica del nome, documento_proposta, non e' il codice del pezzo) resta intatta, R4 la
// dice applicabile e R5 ne scrive la AR con provenienza_pb motore. I rami di rifiuto della stessa regola: la PB del motore
// spostata dall'ingegnere su un altro nodo (corretta) e la PB del motore sul componente ricodificato (D-ASS-01 [U]) tornano
// al confronto del codice letto (codice_diverso), in R4 e in R5 (422 BLOCCHI, nessuna scrittura). Dati sintetici (ACME,
// codici ACM-xxxx, nome di file inventato).

package distintabozza

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// bozzaC48 e' la bozza dal registro dell'altro finito con la PB iniziale del motore sul PDF dal nome «da partner».
type bozzaC48 struct {
	p   *pgxpool.Pool
	s   scenaDB
	sc  scena3DB
	c   Creata
	reg importazione.Registro
	pdf uuid.UUID
	sha string
	stp uuid.UUID
	k   string // il nodo della PB iniziale del motore
}

// bozzaC48DB: la bozza dal registro dell'altro finito (registroAltroDB: il PDF proposto sul telaio #7 o, con radice, sulla
// voce della radice), il PDF «BOM_9120001_1010_20260101_E.pdf» con la lettura generica del nome; R2 toglie #11 (senza
// codice proposto) perche' la bozza sia confermabile (versione 2); la PB del motore resta com'e'.
func bozzaC48DB(t *testing.T, radice bool) bozzaC48 {
	t.Helper()
	p, s := poolPulito(t)
	b := bozzaC48{p: p, s: s, sc: scena3DB{scenaDB: s, p: p}, sha: shaProva('d')}
	b.stp = stepDB(t, p, s.thread, "ACM-6000_R.stp", 'd')
	b.pdf = allegatoFileDB(t, p, s.thread, "BOM_9120001_1010_20260101_E.pdf", "BOM_9120001_1010_20260101", '5')
	esegui3(t, p, `UPDATE documento_proposta SET rev = 'E' WHERE allegato_id = $1`, b.pdf)
	s8DB(t, p, s, s.altroFinito, b.stp, b.sha, "ACM-6000", true)
	b.reg = registroAltroDB(t, s, b.stp, b.sha, b.pdf, shaProva('5'))
	if radice {
		c := b.reg.Contenuto
		var proposte json.RawMessage
		for _, v := range c.Proposta.Nodi {
			if v.Origine.Chiave == "#7" {
				proposte = v.AssociazioniProposte
			}
		}
		if len(proposte) == 0 {
			t.Fatal("la voce #7 del registro non propone il PDF")
		}
		for i := range c.Proposta.Nodi {
			switch c.Proposta.Nodi[i].Origine.Chiave {
			case "#1":
				c.Proposta.Nodi[i].AssociazioniProposte = proposte
			case "#7":
				c.Proposta.Nodi[i].AssociazioniProposte = nil
			}
		}
		b.reg = componi(t, c)
	}
	b.c = creaStepAltroDB(t, p, s, b.stp, b.reg)
	k11 := ChiaveDellaVoce(b.stp, "nodo:"+b.sha+":#11")
	pb := pbDi(t, b.c.Snapshot, b.pdf)
	if pb.Provenienza != ProvenienzaMotore || pb.Importazione != b.reg.Impronta {
		t.Fatalf("PB iniziale del motore: %+v", pb)
	}
	b.k = pb.ChiaveBozza
	m, _ := snapshotDellaVersione(t, p, b.c.BozzaID, 1)
	var nodi, archi []any
	var tolto, arcoTolto map[string]any
	for _, x := range m["nodi"].([]any) {
		if x.(map[string]any)["chiave_bozza"] == k11 {
			tolto = x.(map[string]any)
			continue
		}
		nodi = append(nodi, x)
	}
	for _, x := range m["archi"].([]any) {
		if x.(map[string]any)["figlio"] == k11 {
			arcoTolto = x.(map[string]any)
			continue
		}
		archi = append(archi, x)
	}
	if tolto == nil || arcoTolto == nil {
		t.Fatalf("il nodo #11 non c'e' nella bozza nata dal registro: %v", m["nodi"])
	}
	m["nodi"], m["archi"] = nodi, archi
	if _, err := b.sc.salva3Su(t, b.c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{azione3(t, AzArcoTolto, k11, arcoTolto, nil),
		azione3(t, AzNodoTolto, k11, tolto, nil)}); err != nil {
		t.Fatalf("R2 #11 tolto: %v", err)
	}
	return b
}

// esitoPBC48: R4 della versione v e la riga della PB del PDF.
func (b bozzaC48) esitoPBC48(t *testing.T, v int) (RiepilogoR4, PreassociazioneR4) {
	t.Helper()
	rp := mustRiepilogoDB(t, b.p, b.s.thread, corpoDellaVersione(t, b.p, b.c.BozzaID, v), b.s.utente)
	for _, x := range rp.DocumentiPreassociati {
		if x.Allegato == b.pdf {
			return rp, x
		}
	}
	t.Fatalf("la PB del PDF non e' nel riepilogo: %+v", rp.DocumentiPreassociati)
	return rp, PreassociazioneR4{}
}

// rifiutoC48: R4 della versione v da' la PB del PDF in conflitto codice_diverso (il blocco la nomina) e R5 la rifiuta
// (422 BLOCCHI con lo stesso blocco) senza scrivere.
func (b bozzaC48) rifiutoC48(t *testing.T, v int, caso string) {
	t.Helper()
	rp, x := b.esitoPBC48(t, v)
	nomina := func(bb []Rilievo) bool {
		for _, r := range trova(bb, BloccoPreassociazioneNonApplicab) {
			if r.AllegatoID != nil && *r.AllegatoID == b.pdf && r.Motivo == "codice_diverso" {
				return true
			}
		}
		return false
	}
	if x.Esito != "in_conflitto" || x.Motivo != "codice_diverso" || !nomina(rp.Blocchi) || rp.Confermabile {
		t.Fatalf("R4, %s: %+v %v %+v", caso, x, codici(rp.Blocchi), rp.Blocchi)
	}
	prima := fotoR5(t, b.p)
	_, err := confermaR5DB(t, b.p, b.s, richiestaR5(t, b.p, b.c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if r := rifiutoR5Di(t, err, CodiceBlocchi); !nomina(r.Blocchi) {
		t.Fatalf("R5, %s: %v %+v", caso, codici(r.Blocchi), r.Blocchi)
	}
	if d := diffFoto(prima, fotoR5(t, b.p)); len(d) > 0 {
		t.Fatalf("R5 rifiutata (%s) ha scritto in %v", caso, d)
	}
}

func TestV1D48R5PBDelMotoreConNomeDaPartnerDB(t *testing.T) {
	b := bozzaC48DB(t, false)
	k7 := ChiaveDellaVoce(b.stp, "nodo:"+b.sha+":#7")
	if b.k != k7 {
		t.Fatalf("PB iniziale del motore su %s, attesa su %s", b.k, k7)
	}
	rp, x := b.esitoPBC48(t, 2)
	if x.Esito != "applicabile" || x.Provenienza != ProvenienzaMotore || x.CodiceLetto != "BOM_9120001_1010_20260101" || x.RevLetta != "E" {
		t.Fatalf("R4, la PB del motore con il nome da partner: %+v", x)
	}
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4 non confermabile: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	out, err := confermaR5DB(t, b.p, b.s, richiestaR5(t, b.p, b.c.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	if out.AssociazioniApplicate != 1 {
		t.Fatalf("risposta: %+v", out)
	}
	if n := conta3(t, b.p, `SELECT count(*) FROM file_rfq_associazione f JOIN distinta_importazione i ON i.importazione_id = f.importazione_id
		WHERE f.allegato_id = $1 AND f.componente_id = $2 AND f.stato = 'attiva' AND f.origine = 'conferma_bom' AND f.conferma_id = $3
		AND f.provenienza_pb = 'motore' AND i.impronta = $4`, b.pdf, out.Mappa[k7], out.ConfermaID, b.reg.Impronta); n != 1 {
		t.Fatal("la AR della PB del motore con il nome da partner")
	}
}

// La PB del motore spostata dall'ingegnere su un altro nodo (#9, ACM-6611) e' corretta (provenienzeDecise): torna al
// confronto del codice letto, in R4 e in R5.
func TestV1D48R5PBDelMotoreSpostataDB(t *testing.T) {
	b := bozzaC48DB(t, false)
	k9 := ChiaveDellaVoce(b.stp, "nodo:"+b.sha+":#9")
	m, _ := snapshotDellaVersione(t, b.p, b.c.BozzaID, 2)
	var tenute []any
	for _, x := range m["documenti_preassociati"].([]any) {
		if x.(map[string]any)["allegato_id"] != b.pdf.String() {
			tenute = append(tenute, x)
		}
	}
	m["documenti_preassociati"] = append(tenute, map[string]any{"allegato_id": b.pdf, "chiave_bozza": k9})
	if _, err := b.sc.salva3Su(t, b.c.BozzaID, b.s.altroFinito, 2, uuid.New(), m, []Azione{
		{Azione: AzFileSvincolato, ChiaveBozza: b.k, Prima: mustJSON(map[string]any{"allegato_id": b.pdf, "chiave_bozza": b.k}),
			Dopo: mustJSON(map[string]any{"allegato_id": b.pdf, "motivo": SvincoloUtente}), Origine: "utente"},
		{Azione: AzFilePreassociato, ChiaveBozza: k9, Dopo: mustJSON(map[string]any{"allegato_id": b.pdf, "chiave_bozza": k9}), Origine: "utente"},
	}); err != nil {
		t.Fatalf("R2 PB spostata: %v", err)
	}
	_, raw := snapshotDellaVersione(t, b.p, b.c.BozzaID, 3)
	if pb := pbDi(t, raw, b.pdf); pb.Provenienza != ProvenienzaCorretta || pb.ChiaveBozza != k9 {
		t.Fatalf("PB spostata: %+v", pb)
	}
	b.rifiutoC48(t, 3, "PB del motore spostata (corretta)")
}

// D-ASS-01 [U]: il registro propone il PDF sulla voce della radice (il prodotto registrato); la PB del motore sulla radice e'
// intatta e applicabile; con la ricodifica del prodotto nella bozza la PB si confronta con il codice nuovo: codice_diverso in
// R4 e in R5.
func TestV1D48R5PBDelMotoreSulRicodificatoDB(t *testing.T) {
	b := bozzaC48DB(t, true)
	m, _ := snapshotDellaVersione(t, b.p, b.c.BozzaID, 2)
	var radice map[string]any
	for _, x := range m["nodi"].([]any) {
		n := x.(map[string]any)
		if id, _ := n["componente_id"].(string); id == b.s.altroFinito.String() {
			radice = n
		}
	}
	if radice == nil || radice["chiave_bozza"] != b.k {
		t.Fatalf("la PB iniziale del motore non e' sulla radice (%s): %v", b.k, radice)
	}
	if _, x := b.esitoPBC48(t, 2); x.Esito != "applicabile" || x.Provenienza != ProvenienzaMotore {
		t.Fatalf("R4, la PB del motore intatta sulla radice: %+v", x)
	}
	em := EmittenteCliente(idDi("cliente"))
	d := map[string]any{"id": "d-ric", "tipo": "ricodifica", "componente_id": b.s.altroFinito, "codice_prima": "ACM-6000", "codice_dopo": "ACM-6009",
		"motivo": "il codice giusto del prodotto e' un altro", "emittente_dopo": em}
	primaNodo := map[string]any{"op": radice["op"], "identita": radice["identita"]}
	radice["op"] = OpModifica
	radice["identita"] = map[string]any{"emittente": em, "emittente_presunto": false, "codice": "ACM-6009", "modo": ModoDecisione, "decisione": "d-ric"}
	dopoNodo := map[string]any{"op": radice["op"], "identita": radice["identita"]}
	m["decisioni"] = append(m["decisioni"].([]any), d)
	if _, err := b.sc.salva3Su(t, b.c.BozzaID, b.s.altroFinito, 2, uuid.New(), m, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, d),
		azione3(t, AzNodoModificato, b.k, primaNodo, dopoNodo)}); err != nil {
		t.Fatalf("R2 della ricodifica del prodotto: %v", err)
	}
	_, raw := snapshotDellaVersione(t, b.p, b.c.BozzaID, 3)
	if pb := pbDi(t, raw, b.pdf); pb.Provenienza != ProvenienzaMotore || pb.ChiaveBozza != b.k {
		t.Fatalf("la PB del motore resta sulla radice: %+v", pb)
	}
	b.rifiutoC48(t, 3, "PB del motore sul prodotto ricodificato")
}
