//go:build integrazione

// L4 — V1, ondata B, corsia persistenza: T-10 nella parte che non vuole l'adattatore (VerificaGiornale3): le versioni
// salvate dalla pagina si rifanno dai loro eventi, la versione 1 di una bozza convertita si rifa' con conversione-1, un
// evento cambiato a mano si vede; le versioni costruite con l'adattatore restano fra le non verificate. In piu' R1 su
// una bozza formato 2 aperta (da convertire). Dati sintetici: la scena di ondatab_db_test.go.

package distintabozza

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

func TestV1T10IlGiornaleDelFormato3SiRifa(t *testing.T) {
	sc := preparaBozza3(t)
	ctx := context.Background()
	nuovo := nodoManuale3(sc.id, "k-u1", "ACM-2000")
	arco := map[string]any{"padre": "k-root", "figlio": "k-u1", "qta": 2, "origine": "utente"}
	v2 := copiaProfonda(t, sc.base)
	v2["nodi"] = append(v2["nodi"].([]any), nuovo)
	v2["archi"] = append(v2["archi"].([]any), arco)
	passi := []struct {
		snap   map[string]any
		azioni []Azione
	}{
		{v2, []Azione{azione3(t, AzNodoAggiunto, "k-u1", nil, conCampo(t, nuovo, "azione_id", azioneA)),
			azione3(t, AzArcoAggiunto, "k-u1", nil, conCampo(t, arco, "azione_id", azioneA))}},
		{sc.base, []Azione{azione3(t, AzAnnulla, "", nil, map[string]any{"n": 3}), azione3(t, AzAnnulla, "", nil, map[string]any{"n": 2})}},
	}
	senzaPre := copiaProfonda(t, sc.base)
	senzaPre["documenti_preassociati"] = []any{}
	passi = append(passi, struct {
		snap   map[string]any
		azioni []Azione
	}{senzaPre, []Azione{azione3(t, AzFileSvincolato, "k-0007", map[string]any{"proposta_id": sc.proposta.String(), "chiave_bozza": "k-0007"},
		map[string]any{"proposta_id": sc.proposta.String(), "motivo": SvincoloNodoEscluso}),
		azione3(t, AzFileLasciato, "", nil, map[string]any{"proposta_id": sc.proposta.String(), "motivo": "non serve"})}})
	for i, p := range passi {
		if _, err := sc.salva3(t, i+1, uuid.New(), p.snap, p.azioni); err != nil {
			t.Fatalf("passo %d: %v", i+1, err)
		}
	}
	es, err := VerificaGiornale3(ctx, db.New(sc.p), sc.thread, sc.bozza)
	if err != nil {
		t.Fatalf("il giornale non si rifa': %v", err)
	}
	if len(es.Verificate) != 3 || es.Verificate[0] != 2 || len(es.NonVerificate) != 1 || es.NonVerificate[0].Versione != 1 {
		t.Fatalf("esito: %+v", es)
	}
	// un evento cambiato a mano (il trigger che lo vieta spento nella transazione) non rifa' piu' la sua versione
	tx, err := sc.p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, sql := range []string{
		`ALTER TABLE distinta_bozza_evento DISABLE TRIGGER USER`,
		`UPDATE distinta_bozza_evento SET dopo = jsonb_set(dopo, '{qta}', '7') WHERE bozza_id = '` + sc.bozza.String() + `' AND n = 3`,
	} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	if _, err := VerificaGiornale3(ctx, db.New(tx), sc.thread, sc.bozza); !errors.Is(err, ErrGiornaleIncoerente) {
		t.Fatalf("evento cambiato: %v", err)
	}
	_ = tx.Rollback(ctx)
	// il formato 2 non e' di VerificaGiornale3; una bozza di un'altra RFQ e' FUORI_RFQ
	_, err = VerificaGiornale3(ctx, db.New(sc.p), sc.thread, uuid.New())
	codice3(t, err, CodiceFuoriRFQ, "")
}

func TestV1T10LaConversioneSiRifaER1SuUnaBozzaFormato2(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	shaB := strings.Repeat("b", 64)
	vecchio := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{AllegatoID: pu(s.allegato), Sha256: shaB, Gruppo: "bom"},
		Nodi: []Nodo{
			{ChiaveBozza: "k-root", ComponenteID: pu(s.prodotto), Op: OpRiusa, Codice: "ACM-1000", Rev: "A", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-7", Op: OpCrea, Codice: "ACM-1100", Tipo: "sottoassieme", Origine: "step", Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + shaB + ":#7"}}},
			{ChiaveBozza: "k-9", Op: OpCrea, Codice: "ACM-1109", Tipo: "sciolto", Origine: "step", Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + shaB + ":#9"}}},
		},
		Archi: []Arco{{Padre: "k-root", Figlio: "k-7", Qta: 1, Origine: "step"}, {Padre: "k-7", Figlio: "k-9", Qta: 2, Origine: "step"}},
	}
	b2 := bozza2DB(t, p, s.thread, s.prodotto, s.utente, vecchio)
	// R1 su una bozza formato 2 aperta: com'era, con la conversione da fare, senza i campi del formato 3
	l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{Eventi: true})
	if err != nil {
		t.Fatal(err)
	}
	if l.Formato != FormatoSnapshot || l.Vocabolario != Vocabolario || l.Conversione == nil || !l.Conversione.Necessaria ||
		l.Conversione.Forma != FormaConvertiDa || l.RifScaduti != nil || l.Vassoio != nil || l.NEventi == nil || *l.NEventi != 1 {
		t.Fatalf("R1 formato 2: %+v", l)
	}
	if _, err := VerificaGiornale3(ctx, db.New(p), s.thread, b2); err == nil {
		t.Fatal("VerificaGiornale3 su una bozza formato 2")
	}
	c, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: b2, VersioneAttesa: 1,
		Motivo: "si passa al formato 3", ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("7", 64)})
	if err != nil {
		t.Fatal(err)
	}
	es, err := VerificaGiornale3(ctx, db.New(p), s.thread, c.BozzaID)
	if err != nil || len(es.Verificate) != 1 || es.Verificate[0] != 1 || len(es.NonVerificate) != 0 {
		t.Fatalf("la versione 1 della conversione: %+v %v", es, err)
	}
}
