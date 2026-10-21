//go:build integrazione

// L4 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: cambia_sorgente (cambio-fonte-1) sul database di prova,
// dal corpo letto da LeggiRichiestaR2, con i registri di prova al posto del calcolo del motore: la stessa sorgente rifiutata,
// la copia dello stesso STEP (la fonte confermata vale per contenuto; nessun nodo esce; la storia della fonte con la
// sostituzione; il reinvio), FONTE_NON_CONFERMATA verso un altro STEP, poi il cambio verso quello STEP (i nodi della fonte
// vecchia escono, la PB nel vassoio con fonte_cambiata, nessuna scrittura fuori dalla bozza, un registro nuovo per ogni
// fonte nuova e nessuno per i rifiuti e i reinvii). E' la parte della Prova 6 della direttiva che non passa dal passo 1 di
// S8 (sospeso: INC-S1). Dati sintetici; gli aiuti sono in c3_riallinea_db_test.go e c3_step_db_test.go.

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

func cambiaDB(t *testing.T, p *pgxpool.Pool, s scenaDB, bozza uuid.UUID, versione int, chiave, allegato uuid.UUID, reg importazione.Registro,
	scelte []SceltaR2) (VersioneDelServer, error) {
	t.Helper()
	sc := []any{}
	for _, x := range scelte {
		sc = append(sc, x)
	}
	r := formaR2DB(t, s.prodotto, &bozza, versione, chiave, FormaCambiaSorgente, map[string]any{"sorgente": map[string]any{"allegato_id": allegato,
		"gruppo": reg.Contenuto.Gruppo.Chiave}, "impronta_contenuto": reg.Contenuto.ImprontaContenuto, "motivo": "revisione CAD", "scelte": sc})
	return CambiaSorgenteFormato3(context.Background(), p, r.CambiaSorgenteDaRichiesta(s.thread, s.utente, &PropostaCalcolata{Registro: reg}))
}

func TestV1C3CambiaSorgente(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	id := idSintetici3()
	vecchio := componiProva3(t, id)
	bozza, pdf, k31 := bozzaStepConPB(t, p, s, vecchio)
	// verso la stessa struttura sulla stessa occorrenza: stessa_sorgente
	_, err := cambiaDB(t, p, s, bozza, 2, uuid.New(), s.allegato, vecchio, nil)
	codice3(t, err, CodiceCorpoNonValido, MotivoStessaSorgente)
	// un'altra copia dello stesso STEP (lo stesso contenuto): la fonte confermata vale per contenuto
	copia := nuovoAllegato(t, p, idDi("msg-0"), 10, "ACM-1000_copia.stp", strings.Repeat("b", 64))
	regCopia := componi(t, spostaRegistro(t, registroProva3(t, id), id.allegato, copia, "", ""))
	chiave := uuid.New()
	v, err := cambiaDB(t, p, s, bozza, 2, chiave, copia, regCopia, nil)
	if err != nil {
		t.Fatalf("cambio verso la copia: %v", err)
	}
	snap, err := LeggiSnapshot3(v.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if v.Versione != 3 || *snap.Sorgente.AllegatoID != copia || len(snap.DocumentiPreassociati) != 1 || len(v.Importazione.FuoriPerCambioFonte) != 0 {
		t.Fatalf("copia: %+v %+v", snap.Sorgente, v.Importazione)
	}
	ee := eventiDB(t, p, bozza, 1)
	ultimo := ee[len(ee)-1]
	if ultimo.Azione != AzSorgenteCambiata || ultimo.Motivo != "revisione CAD" || !strings.Contains(string(ultimo.Prima), s.allegato.String()) {
		t.Fatalf("sorgente_cambiata: %+v", ultimo)
	}
	if n := conta3(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'sostituzione' AND allegato_id = $2
		AND precedente_allegato_id = $3 AND bozza_id = $4`, s.prodotto, copia, s.allegato, bozza); n != 1 {
		t.Fatalf("la sostituzione nella storia della fonte: %d", n)
	}
	v2, err := cambiaDB(t, p, s, bozza, 2, chiave, copia, regCopia, nil)
	if err != nil || !v2.Riproposto || v2.Versione != 3 {
		t.Fatalf("reinvio: %v %+v", err, v2)
	}
	// un altro STEP senza la fonte confermata: FONTE_NON_CONFERMATA, niente scritto
	altro := nuovoAllegato(t, p, idDi("msg-0"), 11, "ACM-2000.stp", strings.Repeat("e", 64))
	regAltro := componi(t, spostaRegistro(t, registroProva3(t, id), id.allegato, altro, id.sha, strings.Repeat("e", 64)))
	_, err = cambiaDB(t, p, s, bozza, 3, uuid.New(), altro, regAltro, nil)
	codice3(t, err, CodiceFonteNonConfermata, "")
	esegui3(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-2000.stp', 'stp', repeat('e', 64), '3D/ACM-2000.stp', $3)`, s.thread, s.prodotto, s.utente)
	prima := contiTabelleVietate(t, p)
	v, err = cambiaDB(t, p, s, bozza, 3, uuid.New(), altro, regAltro, nil)
	if err != nil {
		t.Fatalf("cambio verso un altro STEP: %v", err)
	}
	snap, err = LeggiSnapshot3(v.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if *snap.Sorgente.AllegatoID != altro || len(snap.DocumentiPreassociati) != 0 || len(v.Importazione.FuoriPerCambioFonte) == 0 {
		t.Fatalf("altro STEP: %+v %+v", snap.Sorgente, v.Importazione)
	}
	for _, n := range snap.Nodi {
		if n.ChiaveBozza == k31 {
			t.Fatal("il nodo della fonte vecchia e' rimasto")
		}
	}
	l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(l.Vassoio)
	if !strings.Contains(string(b), pdf.String()) || !strings.Contains(string(b), `"fonte_cambiata"`) {
		t.Fatalf("il vassoio dopo il cambio della fonte: %s", b)
	}
	if dopo := contiTabelleVietate(t, p); !mappeUguali(prima, dopo) {
		t.Fatalf("il cambio della fonte ha scritto fuori dalla bozza: %v → %v", prima, dopo)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_importazione`); n != 3 {
		t.Fatalf("registri: %d", n)
	}
	_ = ctx
}
