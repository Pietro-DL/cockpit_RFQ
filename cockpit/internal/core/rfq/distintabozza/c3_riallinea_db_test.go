//go:build integrazione

// L4 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: riallinea_proposta (riancoraggio-1) sul database di
// prova, dal corpo letto da LeggiRichiestaR2, con i registri di prova al posto del calcolo del motore: le scelte che
// mancano, V-7, la versione superata (niente scritto); la versione n+1 con il registro nuovo e gli eventi del server (per
// ultimo proposta_riallineata: la pila d'annullamento si ferma li'), la PB del nodo tolto nel vassoio con il motivo
// nodo_tolto (nessun ricollegamento per codice), nessuna scrittura fuori dalla bozza, il reinvio con gli elenchi dei dati
// salvati. Dati sintetici.

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

// bozzaStepConPB: la scena con la fonte confermata, la bozza della partenza step (versione 1) e la versione 2 con la PB di un
// PDF della RFQ sul nodo #31. Restituisce la bozza, il PDF e la chiave del nodo.
func bozzaStepConPB(t *testing.T, p *pgxpool.Pool, s scenaDB, reg importazione.Registro) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	esegui3(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-1000.stp', 'stp', repeat('b', 64), '3D/ACM-1000.stp', $3)`, s.thread, s.prodotto, s.utente)
	c, err := creaStepDB(t, p, s, uuid.New(), reg, map[string]any{"allegato_id": s.allegato, "gruppo": "bom"}, "", nil)
	if err != nil {
		t.Fatalf("crea step: %v", err)
	}
	pdf := nuovoAllegato(t, p, idDi("msg-0"), 9, "ACM-1131.pdf", strings.Repeat("a", 64))
	id := idSintetici3()
	k31 := ChiaveDellaVoce(id.allegato, id.rif("#31"))
	var m map[string]any
	if err := json.Unmarshal(c.Snapshot, &m); err != nil {
		t.Fatal(err)
	}
	pb := map[string]any{"allegato_id": pdf, "chiave_bozza": k31}
	m["documenti_preassociati"] = []any{pb}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 1, uuid.New(), m, []Azione{{Azione: AzFilePreassociato, ChiaveBozza: k31, Dopo: mustJSON(pb),
		Origine: "utente"}}); err != nil {
		t.Fatalf("la PB: %v", err)
	}
	return c.BozzaID, pdf, k31
}

// riallineaDB e cambiaDB: le due forme dal corpo di R2, con la proposta data.
func riallineaDB(t *testing.T, p *pgxpool.Pool, s scenaDB, bozza uuid.UUID, versione int, chiave uuid.UUID, reg importazione.Registro, impronta string,
	scelte []SceltaR2) (VersioneDelServer, error) {
	t.Helper()
	sc := []any{}
	for _, x := range scelte {
		sc = append(sc, x)
	}
	r := formaR2DB(t, s.prodotto, &bozza, versione, chiave, FormaRiallineaProposta, map[string]any{"impronta_contenuto": impronta,
		"motivo": "proposta di oggi", "scelte": sc})
	return RiallineaFormato3(context.Background(), p, r.RiallineaDaRichiesta(s.thread, s.utente, &PropostaCalcolata{Registro: reg}))
}

func TestV1C3RiallineaProposta(t *testing.T) {
	p, s := poolPulito(t)
	id := idSintetici3()
	vecchio := componiProva3(t, id)
	bozza, pdf, k31 := bozzaStepConPB(t, p, s, vecchio)
	k7 := ChiaveDellaVoce(id.allegato, id.rif("#7"))
	// la proposta di oggi: il telaio con un altro codice proposto, il cordone #31 sparito
	c := registroProva3(t, id)
	var tenute []importazione.Voce
	for _, v := range c.Proposta.Nodi {
		if v.Origine.Rif == id.rif("#31") {
			continue
		}
		if v.Origine.Rif == id.rif("#7") {
			v.CodiceProposto = "ACM-1101"
		}
		tenute = append(tenute, v)
	}
	c.Proposta.Nodi = tenute
	var archi []importazione.ArcoRegistro
	for _, a := range c.Proposta.Archi {
		if a.Figlio != id.rif("#31") {
			archi = append(archi, a)
		}
	}
	c.Proposta.Archi = archi
	nuovo := componi(t, c)
	prima := contiTabelleVietate(t, p)
	// le scelte che mancano, V-7, la versione superata: niente scritto
	_, err := riallineaDB(t, p, s, bozza, 2, uuid.New(), nuovo, nuovo.Contenuto.ImprontaContenuto, []SceltaR2{{ChiaveBozza: k7, Scelta: SceltaRecepisci}})
	codice3(t, err, CodiceCorpoNonValido, MotivoRiallineamentoIncompleto)
	_, err = riallineaDB(t, p, s, bozza, 2, uuid.New(), nuovo, strings.Repeat("0", 64), nil)
	codice3(t, err, CodiceAnteprimaSuperata, MotivoPropostaStep)
	_, err = riallineaDB(t, p, s, bozza, 1, uuid.New(), nuovo, nuovo.Contenuto.ImprontaContenuto, nil)
	codice3(t, err, CodiceBozzaVersione, MotivoVersioneDiversa)
	if n := conta3(t, p, `SELECT count(*) FROM distinta_importazione`); n != 1 {
		t.Fatalf("i rifiuti hanno scritto un registro: %d", n)
	}
	// il riallineamento
	chiave := uuid.New()
	scelte := []SceltaR2{{ChiaveBozza: k7, Scelta: SceltaRecepisci}, {ChiaveBozza: k31, Scelta: SceltaTogli}}
	v, err := riallineaDB(t, p, s, bozza, 2, chiave, nuovo, nuovo.Contenuto.ImprontaContenuto, scelte)
	if err != nil {
		t.Fatalf("riallineamento: %v", err)
	}
	if v.Versione != 3 || v.Importazione == nil || v.Importazione.Riusata || v.Importazione.Impronta != nuovo.Impronta ||
		len(v.Importazione.IdentitaAggiornate) != 1 || v.Importazione.IdentitaAggiornate[0].ChiaveBozza != k7 {
		t.Fatalf("esito: %+v %+v", v.Salvato3, v.Importazione)
	}
	snap, err := LeggiSnapshot3(v.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Sorgente.Importazione != nuovo.Impronta || len(snap.DocumentiPreassociati) != 0 {
		t.Fatalf("snapshot: %+v %+v", snap.Sorgente, snap.DocumentiPreassociati)
	}
	if n, _ := nodoC3(snap, k7); n.Identita.Codice != "ACM-1101" {
		t.Fatalf("recepisci: %+v", n.Identita)
	}
	ee := eventiDB(t, p, bozza, 1)
	ultimo := ee[len(ee)-1]
	if ultimo.Azione != AzPropostaRiallineata || ultimo.Origine != "server" || ultimo.Motivo != "proposta di oggi" ||
		campoJSON(t, ultimo.Dopo, "algoritmo") != `"`+AlgoritmoRiancoraggio+`"` || campoJSON(t, ultimo.Prima, "importazione") != `"`+vecchio.Impronta+`"` {
		t.Fatalf("proposta_riallineata: %+v", ultimo)
	}
	svincolo := 0
	for _, e := range ee {
		if e.Versione == 3 && e.Azione == AzFileSvincolato {
			svincolo = e.N
			if e.Origine != "server" || !strings.Contains(string(e.Dopo), `"motivo":"nodo_tolto"`) {
				t.Fatalf("file_svincolato: %+v", e)
			}
		}
	}
	if svincolo == 0 {
		t.Fatal("manca il file_svincolato del server")
	}
	l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(l.Vassoio)
	if !strings.Contains(string(b), pdf.String()) || !strings.Contains(string(b), `"nodo_tolto"`) {
		t.Fatalf("il vassoio dopo il riallineamento: %s", b)
	}
	if dopo := contiTabelleVietate(t, p); !mappeUguali(prima, dopo) {
		t.Fatalf("il riallineamento ha scritto fuori dalla bozza: %v → %v", prima, dopo)
	}
	// il reinvio: riproposto, con il registro e gli elenchi dei dati salvati
	r2, err := riallineaDB(t, p, s, bozza, 2, chiave, nuovo, nuovo.Contenuto.ImprontaContenuto, scelte)
	if err != nil || !r2.Riproposto || r2.Versione != 3 || r2.Importazione == nil || len(r2.Importazione.IdentitaAggiornate) != 1 {
		t.Fatalf("reinvio: %v %+v", err, r2)
	}
	// la pila d'annullamento si ferma alla versione del server: annullare lo svincolo del server e' rifiutato
	var m map[string]any
	if err := json.Unmarshal(v.Snapshot, &m); err != nil {
		t.Fatal(err)
	}
	sc := scena3DB{scenaDB: s, p: p}
	_, err = sc.salva3Su(t, bozza, s.prodotto, 3, uuid.New(), m, []Azione{{Azione: AzAnnulla, Dopo: mustJSON(map[string]int{"n": svincolo}), Origine: "utente"}})
	codice3(t, err, CodiceCorpoNonValido, "annullamento_non_ammesso")
}
