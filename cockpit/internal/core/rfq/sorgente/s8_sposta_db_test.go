//go:build integrazione

// L4 — giro G39, G39-03 [U] (20/10: «mai la stessa sorgente su due prodotti, ma slegare e riassociare»), corsia
// persistenza parte B, sul database di prova con registri di prova al posto del calcolo del motore. Dati sintetici: la
// scena di s8_db_test.go (ACM-1000 e ACM-6000 sono due prodotti finiti della stessa RFQ). Che cosa si prova, con i conteggi
// prima e dopo:
//   - «Sposta la sorgente su questo prodotto»: lo STEP sorgente S8 di un altro prodotto, non usato da una R5, passa al
//     prodotto giusto in un gesto (revoca motivata la', bozza la' annullata, documento qui, S8 qui), idempotente con la
//     chiave; senza il gesto il rifiuto dice sorgente_spostabile e non scrive niente;
//   - lo STEP che e' solo un documento di un altro prodotto (legacy, senza S8) e la copia P1 su un altro prodotto si
//     riassociano con il motivo, senza lasciare il file attivo su due prodotti e senza AR;
//   - lo STEP gia' nella BOM confermata dell'altro prodotto: rifiuto senza scritture con il rimedio «apri la revisione»;
//   - la gara fra «sposta la sorgente» e R5 sull'altro prodotto: vince uno solo, mai la stessa sorgente valida su due
//     prodotti, step_strutturale_id solo da R5;
//   - dopo R5 il cambio di sorgente e' una revisione segnata «correzione della sorgente» in R4, R5 e R7 (con la revoca
//     sorgente_errata seguita da S8, e con correzione_sorgente nel corpo di S8); la revoca dopo R5 non annulla niente.
// In tutti i casi: nessuna AR fuori da R5, sostituito_da e step_strutturale_id scritti solo da R5.

package sorgente

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// registroPer: lo stesso registro di prova composto per un altro prodotto della RFQ.
func registroPer(t *testing.T, reg importazione.Registro, prodotto uuid.UUID) importazione.Registro {
	t.Helper()
	c := reg.Contenuto
	c.ProdottoID = prodotto.String()
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

// corpoPer: il corpo di S8 per il prodotto dato (codice atteso compreso), con i campi in piu'.
func corpoPer(t *testing.T, s scenaS8, prodotto uuid.UUID, codice string, chiave uuid.UUID, reg importazione.Registro, extra map[string]any) []byte {
	t.Helper()
	x := map[string]any{"prodotto_id": prodotto, "codice_atteso": codice}
	for k, v := range extra {
		x[k] = v
	}
	return corpoStep(t, s, chiave, reg, x)
}

// sorgentiValide: quanti prodotti finiti della scena hanno lo sha256 come sorgente valida per R5.
func sorgentiValide(t *testing.T, p *pgxpool.Pool, s scenaS8, sha string) int {
	t.Helper()
	n := 0
	for _, prod := range []uuid.UUID{s.prodotto, s.altroFinito} {
		st, err := confermabom.SorgenteValida(context.Background(), db.New(p), s.thread, prod, sha)
		if err != nil {
			t.Fatal(err)
		}
		if st.Valida {
			n++
		}
	}
	return n
}

// nuovoStep: uno STEP in piu' della RFQ (file in staging, allegato, proposta di documento cad_3d), con lo sha256.
func nuovoStep(t *testing.T, p *pgxpool.Pool, s scenaS8, nome string, indice int) (uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	percorso := filepath.Join(s.dir, fmt.Sprintf("%d_%s", indice, nome))
	contenuto := []byte("contenuto sintetico " + nome + " " + id.String())
	if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(contenuto)
	sha := hex.EncodeToString(h[:])
	esegui(t, p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'stp', 'file', 'outlook', $5, $6, $7, 'analizzato', now())`, id, s.msg, indice, nome, len(contenuto), sha, percorso)
	return id, sha
}

// contiG39: le righe che lo spostamento non deve toccare (fotoVietate) piu' le AR e i documenti.
func contiG39(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	return fotoVietate(t, p) + "|doc:" + fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`)) +
		"|fse:" + fmt.Sprint(conta(t, p, `SELECT count(*) FROM fonte_step_evento`)) +
		"|bozze:" + fmt.Sprint(conta(t, p, `SELECT count(*) FROM distinta_bozza`)) +
		"|aperte:" + fmt.Sprint(conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE stato = 'aperta'`))
}

func TestV1G39SpostaLaSorgenteSuQuestoProdotto(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	// lo STEP A confermato per errore come sorgente di ACM-1000 (la BOM non e' confermata): la bozza di ACM-1000 nasce da li'
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	esP, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esP.Esito != EsitoCompletata || esP.Bozza == nil {
		t.Fatalf("S8 su ACM-1000: %+v %v", esP, err)
	}
	var docA uuid.UUID
	if err := p.QueryRow(ctx, `SELECT documento_id FROM documento WHERE sha256 = $1`, s.shaA).Scan(&docA); err != nil {
		t.Fatal(err)
	}
	regQ := registroPer(t, regA, s.altroFinito)
	prepQ := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}}
	motivo := map[string]any{"motivo": "lo STEP e' di ACM-6000"}
	prima := contiG39(t, p)
	// senza il gesto: sorgente_spostabile con l'altro prodotto, niente scritto
	_, err = s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, motivo), prepQ)
	if e := codiceDi(t, err, CodiceStepSorgenteDiAltroProdotto, MotivoSorgenteSpostabile); e.AltroProdotto == nil || *e.AltroProdotto != s.prodotto {
		t.Fatalf("il rifiuto senza il gesto: %+v", e)
	}
	// il gesto che nomina un altro prodotto; il gesto che nomina se stesso
	_, err = s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "x", "sposta_da": s.figlio}), prepQ)
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, "")
	_, err = s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "x", "sposta_da": s.altroFinito}), prepQ)
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, "")
	if dopo := contiG39(t, p); dopo != prima {
		t.Fatalf("i rifiuti hanno scritto: %s -> %s", prima, dopo)
	}
	if n := sorgentiValide(t, p, s, s.shaA); n != 1 {
		t.Fatalf("sorgenti valide prima dello spostamento: %d", n)
	}
	// «Sposta la sorgente su ACM-6000»
	chiave := uuid.New()
	corpo := corpoPer(t, s, s.altroFinito, "ACM-6000", chiave, regQ, map[string]any{"motivo": "lo STEP e' di ACM-6000", "sposta_da": s.prodotto})
	es, err := s8(t, p, s, s.stepA, corpo, prepQ)
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil || es.Spostamento == nil {
		t.Fatalf("lo spostamento: %+v %v", es, err)
	}
	sp := es.Spostamento
	if sp.Caso != CasoSorgenteS8 || sp.DaProdottoID != s.prodotto || sp.RevocaEvento == nil || sp.BozzaAbbandonata == nil ||
		*sp.BozzaAbbandonata != esP.Bozza.BozzaID || sp.DocumentoID == nil || *sp.DocumentoID != docA || sp.PercorsoNas != PercorsoAggiornato ||
		confermabom.CausaDelMotivo(sp.Motivo) != confermabom.CausaSorgenteErrata {
		t.Fatalf("l'esito dello spostamento: %+v", sp)
	}
	// mai la stessa sorgente valida su due prodotti: adesso e' di ACM-6000
	if n := sorgentiValide(t, p, s, s.shaA); n != 1 {
		t.Fatalf("sorgenti valide dopo lo spostamento: %d", n)
	}
	// la sorgente spostata non e' una correzione della BOM di ACM-6000 (rilievo bloccante della revisione G39)
	if st, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.altroFinito, s.shaA); !st.Valida || st.Correzione || st.CorrezioneDichiarata {
		t.Fatalf("la sorgente di ACM-6000: %+v", st)
	}
	if st, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); st.Valida || st.Motivo != confermabom.SorgenteRevocata {
		t.Fatalf("la sorgente di ACM-1000 vale ancora: %+v", st)
	}
	// il documento e' di ACM-6000 con il suo codice e il percorso ricalcolato (il file non e' ancora sul NAS); uno solo
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE documento_id = $1 AND componente_id = $2 AND codice = 'ACM-6000'
		AND path_relativo ILIKE '%ACM-6000%' AND sostituito_da IS NULL`, docA, s.altroFinito); n != 1 {
		t.Fatal("il documento non e' passato ad ACM-6000")
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1`, s.shaA); n != 1 {
		t.Fatalf("documenti dello STEP: %d", n)
	}
	// la storia: revoca motivata su ACM-1000 (con la bozza e la causa), S8 su ACM-6000 con il motivo scritto (la causa sta
	// sulla revoca, non sulla riga di S8 di chi riceve lo STEP); la bozza di ACM-1000 abbandonata
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'revoca_sorgente' AND documento_id = $2
		AND bozza_id = $3 AND motivo LIKE 'sorgente_errata: lo STEP passa a ACM-6000%' AND chiave_idempotenza = $4`,
		s.prodotto, docA, esP.Bozza.BozzaID, ChiavePasso(chiave, "2s")); n != 1 {
		t.Fatalf("la revoca su ACM-1000: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'conferma_documento' AND documento_id = $2
		AND motivo = 'lo STEP e'' di ACM-6000' AND chiave_idempotenza = $3`, s.altroFinito, docA, ChiavePasso(chiave, "2")); n != 1 {
		t.Fatalf("la riga di S8 su ACM-6000: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'abbandonata' AND motivo_chiusura LIKE 'sorgente revocata: sorgente_errata%'`,
		esP.Bozza.BozzaID); n != 1 {
		t.Fatal("la bozza di ACM-1000 non e' annullata")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_registro WHERE bozza_id = $1 AND passo = 'abbandonata'`, esP.Bozza.BozzaID); n != 1 {
		t.Fatal("il registro della bozza annullata")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND prodotto_id = $2 AND stato = 'aperta' AND fonte_sha256 = $3`,
		es.Bozza.BozzaID, s.altroFinito, s.shaA); n != 1 {
		t.Fatal("la bozza di ACM-6000 non nasce dallo STEP")
	}
	// niente working, niente AR, niente sostituito_da, niente step_strutturale_id
	if a, b := strings.SplitN(prima, "|doc:", 2)[0], strings.SplitN(contiG39(t, p), "|doc:", 2)[0]; a != b {
		t.Fatalf("lo spostamento ha toccato la working o le AR: %s -> %s", a, b)
	}
	var legato bool
	if err := p.QueryRow(ctx, `SELECT allegato_sorgente_del_finito($1, $2)`, s.thread, s.stepA).Scan(&legato); err != nil || !legato {
		t.Fatalf("lo STEP resta legato come sorgente (di ACM-6000): %v %v", legato, err)
	}
	// il reinvio: riproposto, niente di nuovo
	c0 := contiG39(t, p)
	if es2, err := s8(t, p, s, s.stepA, corpo, prepQ); err != nil || !es2.Riproposto || es2.Spostamento == nil {
		t.Fatalf("reinvio: %+v %v", es2, err)
	}
	if c1 := contiG39(t, p); c1 != c0 {
		t.Fatalf("il reinvio ha scritto: %s -> %s", c0, c1)
	}
	// e il ritorno, se lo sbaglio era il secondo: lo STEP torna ad ACM-1000 con lo stesso gesto
	esR, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"sposta_da": s.altroFinito}), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esR.Esito != EsitoCompletata || esR.Spostamento == nil || esR.Spostamento.BozzaAbbandonata == nil || *esR.Spostamento.BozzaAbbandonata != es.Bozza.BozzaID {
		t.Fatalf("il ritorno: %+v %v", esR, err)
	}
	if n := sorgentiValide(t, p, s, s.shaA); n != 1 {
		t.Fatalf("sorgenti valide dopo il ritorno: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE documento_id = $1 AND componente_id = $2 AND codice = 'ACM-1000'`, docA, s.prodotto); n != 1 {
		t.Fatal("il documento non e' tornato ad ACM-1000")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE stato = 'aperta'`); n != 1 {
		t.Fatalf("bozze aperte: %d", n)
	}
}

func TestV1G39RiassociaDocumentoECopiaP1(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	// (1) lo STEP B e' un documento legacy di ACM-6000 (con la riga conferma_documento delle pagine di prima, senza S8)
	var docB uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-6000', 'ACM-1000_revB.stp', 'stp', $3, '3D/ACM-6000/ACM-6000.stp', $4) RETURNING documento_id`,
		s.thread, s.altroFinito, s.shaB, s.utente).Scan(&docB); err != nil {
		t.Fatal(err)
	}
	esegui(t, p, `UPDATE documento_proposta SET stato = 'confermata' WHERE allegato_id = $1`, s.stepB)
	esegui(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, $6)`, s.thread, s.altroFinito, s.stepB, s.shaB, docB, s.utente)
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	prepB := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}}
	prima := contiG39(t, p)
	// messaggio 46 [U]: senza il gesto «Riassocia a ‹P›» (sposta_da) la riassociazione non parte, con il rimedio; il motivo
	// scritto non serve piu'
	_, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, nil), prepB)
	if e := codiceDi(t, err, CodiceStepSorgenteDiAltroProdotto, MotivoRiassociazione); e.AltroProdotto == nil || *e.AltroProdotto != s.altroFinito {
		t.Fatalf("senza «Riassocia»: %+v", e)
	}
	if dopo := contiG39(t, p); dopo != prima {
		t.Fatalf("il rifiuto ha scritto: %s -> %s", prima, dopo)
	}
	// con sposta_da, senza motivo: la riassociazione, e la riga di S8 di ACM-1000 ha la traccia del gesto
	es, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sposta_da": s.altroFinito}), prepB)
	if err != nil || es.Esito != EsitoCompletata || es.Spostamento == nil || es.Spostamento.Caso != CasoDocumento || es.Spostamento.RevocaEvento == nil ||
		es.Spostamento.DocumentoID == nil || *es.Spostamento.DocumentoID != docB {
		t.Fatalf("la riassociazione del documento: %+v %v", es, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE documento_id = $1 AND componente_id = $2 AND codice = 'ACM-1000'`, docB, s.prodotto); n != 1 {
		t.Fatal("il documento non e' passato ad ACM-1000")
	}
	// il legame legacy con ACM-6000 e' chiuso dalla revoca motivata; ACM-1000 ha la riga di S8; mai su due prodotti
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'revoca_sorgente' AND motivo LIKE 'sorgente_errata:%'`,
		s.altroFinito); n != 1 {
		t.Fatal("la revoca del legame legacy")
	}
	if n := sorgentiValide(t, p, s, s.shaB); n != 1 {
		t.Fatalf("sorgenti valide: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'conferma_documento' AND motivo = $2`,
		s.prodotto, "gesto di S8P da «Conferma la sorgente»: lo STEP «ACM-1000_revB.stp» è la sorgente di ACM-1000; lo STEP lascia ACM-6000"); n != 1 {
		t.Fatal("la riga di S8 di ACM-1000 (la traccia della riassociazione, senza causa: non e' il gesto «sposta»)")
	}

	// (2) la copia P1: uno STEP nuovo pre-assegnato ad ACM-6000 dalle pagine di prima (proposta aperta con la copia, senza AR)
	abbandona(t, p, s)
	stepP1, shaP1 := nuovoStep(t, p, s, "ACM-1000_p1.stp", 9)
	esegui(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, componente_id)
		VALUES ($1, $2, 'cad_3d', 'ACM-6000', 80, 'nome_file', $3)`, stepP1, s.thread, s.altroFinito)
	regP1 := registroRadice(t, s, stepP1, shaP1, "ACM-1000")
	prepP1 := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regP1}}
	ar0, ev0 := conta(t, p, `SELECT count(*) FROM file_rfq_associazione`), conta(t, p, `SELECT count(*) FROM associazione_evento`)
	_, err = s8(t, p, s, stepP1, corpoStep(t, s, uuid.New(), regP1, nil), prepP1)
	if e := codiceDi(t, err, CodiceStepSorgenteDiAltroProdotto, MotivoRiassociazione); e.AltroProdotto == nil || *e.AltroProdotto != s.altroFinito {
		t.Fatalf("la copia P1 senza «Riassocia»: %+v", e)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id = $2 AND stato = 'aperta'`, stepP1, s.altroFinito); n != 1 {
		t.Fatal("il rifiuto ha toccato la copia")
	}
	// un client di prima: il motivo scritto, senza sposta_da, riassocia ancora
	es, err = s8(t, p, s, stepP1, corpoStep(t, s, uuid.New(), regP1, map[string]any{"motivo": "pre-assegnato al prodotto sbagliato"}), prepP1)
	if err != nil || es.Esito != EsitoCompletata || es.Spostamento == nil || es.Spostamento.Caso != CasoCopiaP1 || !es.Spostamento.CopiaTolta {
		t.Fatalf("la riassociazione della copia P1: %+v %v", es, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id IS NULL AND stato = 'confermata'`, stepP1); n != 1 {
		t.Fatal("la copia P1 non e' tolta")
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1 AND componente_id = $2`, shaP1, s.prodotto); n != 1 {
		t.Fatal("il documento della copia P1 non e' di ACM-1000")
	}
	if n := conta(t, p, `SELECT count(*) FROM associazione_evento WHERE allegato_id = $1 AND azione = 'sgancia' AND origine = 'ponte' AND componente_prima = $2
		AND associazione_id IS NULL AND conferma_id IS NULL AND autore_id = $3 AND motivo LIKE 'sorgente_errata:%'`, stepP1, s.altroFinito, s.utente); n != 1 {
		t.Fatal("l'evento dello sgancio della copia")
	}
	if ar, ev := conta(t, p, `SELECT count(*) FROM file_rfq_associazione`), conta(t, p, `SELECT count(*) FROM associazione_evento`); ar != ar0 || ev != ev0+1 {
		t.Fatalf("AR %d -> %d (nessuna fuori da R5), eventi %d -> %d", ar0, ar, ev0, ev)
	}
	if n := conta(t, p, `SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL`); n != 0 {
		t.Fatalf("step_strutturale_id fuori da R5: %d", n)
	}
}

// primaConferma: S8 dello STEP A su ACM-1000 (con il figlio della working e una staffa), R4 e R5: la conferma n. 1.
func primaConferma(t *testing.T, p *pgxpool.Pool, s scenaS8) (importazione.Registro, distintabozza.ConfermaR5, uuid.UUID) {
	t.Helper()
	figlio := s.figlio
	esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
	regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#2", codice: "ACM-1100", componente: &figlio, qta: 1},
		{chiave: "#3", codice: "ACM-1150", qta: 4}})
	fattiRevisione(t, p, s.shaA, "#1", "#2", "#3")
	esA, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"motivo": "prima sorgente"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esA.Esito != EsitoCompletata || esA.Bozza == nil {
		t.Fatalf("S8 su A: %+v %v", esA, err)
	}
	rp := riepilogoRev(t, p, s, esA.Bozza.BozzaID, 1)
	if len(rp.Blocchi) != 0 || !rp.Confermabile || rp.EffettoSullaFonte.CorrezioneSorgente {
		t.Fatalf("R4 della prima conferma: %v %+v", codiciR4(rp), rp.EffettoSullaFonte)
	}
	c1, err := confermaRev(t, p, s, esA.Bozza.BozzaID, 1, rp.FirmaAnteprima)
	if err != nil || c1.NumeroConferma != 1 || c1.Fonte.CorrezioneSorgente {
		t.Fatalf("R5 della prima conferma: %+v %v", c1, err)
	}
	var docA uuid.UUID
	if err := p.QueryRow(context.Background(), `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, s.prodotto).Scan(&docA); err != nil {
		t.Fatal(err)
	}
	return regA, c1, docA
}

func TestV1G39StepNellaBomConfermataNonSiSposta(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA, _, docA := primaConferma(t, p, s)
	regQ := registroPer(t, regA, s.altroFinito)
	prepQ := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}}
	prima := fotoRev(t, p)
	for _, extra := range []map[string]any{{"motivo": "lo STEP e' di ACM-6000"}, {"motivo": "lo STEP e' di ACM-6000", "sposta_da": s.prodotto}} {
		_, err := s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, extra), prepQ)
		e := codiceDi(t, err, CodiceStepSorgenteDiAltroProdotto, MotivoBomConfermata)
		if e.AltroProdotto == nil || *e.AltroProdotto != s.prodotto || !strings.Contains(e.Messaggio, "Apri la revisione di ACM-1000") {
			t.Fatalf("il rifiuto con il rimedio: %+v", e)
		}
	}
	dopo := fotoRev(t, p)
	for _, tab := range tabelleRev {
		if dopo[tab] != prima[tab] {
			t.Fatalf("il rifiuto ha scritto in %s", tab)
		}
	}
	// la revoca dopo R5 non annulla niente: e' una revisione
	raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": "STEP sbagliato", "sorgente_errata": true})
	r, imp, err := LeggiRichiestaRevoca(raw)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := Revoca(ctx, p, s.thread, s.stepA, s.utente, r, imp)
	if err != nil || !rv.UsataDaConferma || rv.BozzaAbbandonata != nil || rv.Motivo != "sorgente_errata: STEP sbagliato" {
		t.Fatalf("la revoca dopo R5: %+v %v", rv, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND step_strutturale_id = $2`, s.prodotto, docA); n != 1 {
		t.Fatal("la revoca ha toccato la working")
	}
}

// La gara fra «sposta la sorgente» su ACM-6000 e R5 su ACM-1000 (che ha lo STEP come sorgente S8, la BOM non confermata):
// vince uno solo, nell'ordine del lucchetto della RFQ. Tre giri dietro il lucchetto tenuto da una terza transazione: R5 in
// coda per prima, S8 in coda per prima (la coda del lucchetto di PostgreSQL e' in ordine d'arrivo), partenza insieme.
func TestV1G39GaraFraSpostaLaSorgenteER5(t *testing.T) {
	esiti := map[string]int{}
	for giro := 0; giro < 3; giro++ {
		p, s := preparaS8(t)
		ctx := context.Background()
		figlio := s.figlio
		esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
		regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#2", codice: "ACM-1100", componente: &figlio, qta: 1}})
		fattiRevisione(t, p, s.shaA, "#1", "#2")
		esA, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
		if err != nil || esA.Bozza == nil {
			t.Fatalf("S8 su A: %+v %v", esA, err)
		}
		rp := riepilogoRev(t, p, s, esA.Bozza.BozzaID, 1)
		if !rp.Confermabile {
			t.Fatalf("R4: %v", codiciR4(rp))
		}
		ar0 := conta(t, p, `SELECT count(*) FROM file_rfq_associazione`)
		regQ := registroPer(t, regA, s.altroFinito)
		corpo := corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "lo STEP e' di ACM-6000", "sposta_da": s.prodotto})
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.New(tx).BloccaThread(ctx, s.thread); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var c5 distintabozza.ConfermaR5
		var e5, e8 error
		var es8 EsitoS8
		parti5, parti8 := make(chan struct{}), make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-parti5
			c5, e5 = confermaRev(t, p, s, esA.Bozza.BozzaID, 1, rp.FirmaAnteprima)
		}()
		go func() {
			defer wg.Done()
			<-parti8
			es8, e8 = s8(t, p, s, s.stepA, corpo, Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}})
		}()
		switch giro {
		case 0:
			close(parti5)
			time.Sleep(time.Second)
			close(parti8)
		case 1:
			close(parti8)
			time.Sleep(time.Second)
			close(parti5)
		default:
			close(parti5)
			close(parti8)
		}
		time.Sleep(time.Second)
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		r5ok := e5 == nil && c5.NumeroConferma == 1
		spostato := e8 == nil && es8.Esito == EsitoCompletata
		if r5ok == spostato {
			t.Fatalf("giro %d: deve vincere uno solo: R5 %+v %v; S8 %+v %v", giro, c5, e5, es8, e8)
		}
		if n := sorgentiValide(t, p, s, s.shaA); n > 1 {
			t.Fatalf("giro %d: la stessa sorgente valida su %d prodotti", giro, n)
		}
		ss := conta(t, p, `SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL`)
		if r5ok {
			esiti["r5"]++
			// S8 rifiutata nel passo 2 sotto il lucchetto: parziale, niente spostato
			if e8 != nil || es8.Esito != EsitoParziale || len(es8.Passi) < 2 || es8.Passi[1].Codice != CodiceStepSorgenteDiAltroProdotto {
				t.Fatalf("giro %d: S8 dopo R5: %+v %v", giro, es8, e8)
			}
			if ss != 1 || conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1 AND componente_id = $2`, s.shaA, s.prodotto) != 1 {
				t.Fatalf("giro %d: dopo R5 lo STEP resta di ACM-1000 ed e' il suo STEP strutturale", giro)
			}
		} else {
			esiti["sposta"]++
			if e5 == nil {
				t.Fatalf("giro %d: R5 dopo lo spostamento non e' rifiutata: %+v", giro, c5)
			}
			if ss != 0 || conta(t, p, `SELECT count(*) FROM conferma_strutturale`) != 0 ||
				conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1 AND componente_id = $2`, s.shaA, s.altroFinito) != 1 {
				t.Fatalf("giro %d: dopo lo spostamento niente conferma, lo STEP e' di ACM-6000", giro)
			}
		}
		if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione`); n != ar0 {
			t.Fatalf("giro %d: AR %d -> %d", giro, ar0, n)
		}
		if n := conta(t, p, `SELECT count(*) FROM documento WHERE sostituito_da IS NOT NULL`); n != 0 {
			t.Fatalf("giro %d: sostituito_da: %d", giro, n)
		}
	}
	t.Logf("esiti della gara: %v", esiti)
	if esiti["r5"] == 0 || esiti["sposta"] == 0 {
		t.Fatalf("i due ordini non sono stati provati tutti e due: %v", esiti)
	}
}

// La correzione della sorgente dopo R5: la revoca con la causa sorgente_errata, poi S8 dello STEP giusto (B): la revisione
// e' segnata in R4 (effetto_sulla_fonte), in R5 (origine e risposta) e in R7; la conferma n. 1 no.
func TestV1G39CorrezioneDellaSorgenteDopoR5(t *testing.T) {
	for _, via := range []string{"revoca", "corpo"} {
		t.Run(via, func(t *testing.T) {
			p, s := preparaS8(t)
			ctx := context.Background()
			_, c1, docA := primaConferma(t, p, s)
			figlio := s.figlio
			var staffa uuid.UUID
			if err := p.QueryRow(ctx, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = 'ACM-1150'`, s.thread).Scan(&staffa); err != nil {
				t.Fatal(err)
			}
			extra := map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "lo STEP giusto"}
			if via == "revoca" {
				raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": "confermato lo STEP sbagliato",
					"sorgente_errata": true})
				r, imp, err := LeggiRichiestaRevoca(raw)
				if err != nil {
					t.Fatal(err)
				}
				if rv, err := Revoca(ctx, p, s.thread, s.stepA, s.utente, r, imp); err != nil || !rv.UsataDaConferma {
					t.Fatalf("revoca: %+v %v", rv, err)
				}
			} else {
				extra["correzione_sorgente"] = true
			}
			regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#5", codice: "ACM-1100", componente: &figlio, qta: 1},
				{chiave: "#6", codice: "ACM-1150", componente: &staffa, qta: 4}})
			fattiRevisione(t, p, s.shaB, "#1", "#5", "#6")
			esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, extra), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
			if err != nil || esB.Esito != EsitoCompletata || esB.Bozza == nil {
				t.Fatalf("S8 su B: %+v %v", esB, err)
			}
			rp := riepilogoRev(t, p, s, esB.Bozza.BozzaID, esB.Bozza.Versione)
			if len(rp.Blocchi) != 0 || !rp.Sorgente.Correzione || !rp.EffettoSullaFonte.CorrezioneSorgente || !rp.EffettoSullaFonte.Promozione ||
				!strings.Contains(rp.EffettoSullaFonte.Testo, "non una revisione del cliente") {
				t.Fatalf("R4 della correzione: %v %+v %+v", codiciR4(rp), rp.Sorgente, rp.EffettoSullaFonte)
			}
			c2, err := confermaRev(t, p, s, esB.Bozza.BozzaID, esB.Bozza.Versione, rp.FirmaAnteprima)
			if err != nil || c2.NumeroConferma != 2 || !c2.Fonte.CorrezioneSorgente || !c2.Fonte.Promossa || c2.Fonte.SostituzioneDi == nil || *c2.Fonte.SostituzioneDi != docA {
				t.Fatalf("R5 della correzione: %+v %v", c2, err)
			}
			if n := conta(t, p, `SELECT count(*) FROM conferma_strutturale WHERE conferma_id = $1 AND origine->>'correzione_sorgente' = 'true'
				AND origine->>'revisione' = 'interna' AND delta->'effetto_sulla_fonte'->>'correzione_sorgente' = 'true'`, c2.ConfermaID); n != 1 {
				t.Fatal("la conferma n. 2 non e' segnata come correzione della sorgente")
			}
			if n := conta(t, p, `SELECT count(*) FROM conferma_strutturale WHERE conferma_id = $1 AND origine ? 'correzione_sorgente'`, c1.ConfermaID); n != 0 {
				t.Fatal("la conferma n. 1 e' segnata")
			}
			st, err := distintabozza.LeggiStorico(ctx, db.New(p), s.thread, s.prodotto, distintabozza.OpzioniR7{})
			if err != nil {
				t.Fatal(err)
			}
			segnate := map[int]bool{}
			for _, c := range st.Conferme {
				segnate[c.Numero] = c.CorrezioneSorgente && c.Revisione == distintabozza.RevisioneInterna
			}
			if !segnate[2] || segnate[1] {
				t.Fatalf("R7: %v", segnate)
			}
			// R5 scrive step_strutturale_id e sostituito_da, S8 no
			var docB uuid.UUID
			if err := p.QueryRow(ctx, `SELECT documento_id FROM documento WHERE sha256 = $1`, s.shaB).Scan(&docB); err != nil {
				t.Fatal(err)
			}
			if n := conta(t, p, `SELECT count(*) FROM documento WHERE documento_id = $1 AND sostituito_da = $2`, docA, docB); n != 1 {
				t.Fatal("A sostituito da B (solo R5)")
			}
		})
	}
}

// Senza la causa, la revisione con un secondo STEP resta una revisione ordinaria (non segnata).
func TestV1G39RevisioneOrdinariaNonESegnata(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	primaConferma(t, p, s)
	figlio := s.figlio
	var staffa uuid.UUID
	if err := p.QueryRow(ctx, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = 'ACM-1150'`, s.thread).Scan(&staffa); err != nil {
		t.Fatal(err)
	}
	regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#5", codice: "ACM-1100", componente: &figlio, qta: 1},
		{chiave: "#6", codice: "ACM-1150", componente: &staffa, qta: 4}})
	fattiRevisione(t, p, s.shaB, "#1", "#5", "#6")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "revisione 2 del cliente"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Bozza == nil {
		t.Fatalf("S8 su B: %+v %v", esB, err)
	}
	rp := riepilogoRev(t, p, s, esB.Bozza.BozzaID, esB.Bozza.Versione)
	if rp.Sorgente.Correzione || rp.EffettoSullaFonte.CorrezioneSorgente {
		t.Fatalf("una revisione del cliente segnata come correzione: %+v", rp.EffettoSullaFonte)
	}
	c2, err := confermaRev(t, p, s, esB.Bozza.BozzaID, esB.Bozza.Versione, rp.FirmaAnteprima)
	if err != nil || c2.Fonte.CorrezioneSorgente {
		t.Fatalf("R5: %+v %v", c2, err)
	}
}

func TestV1G39RevocaConLaBozzaDichiarata(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	es, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || es.Bozza == nil {
		t.Fatalf("S8: %+v %v", es, err)
	}
	revoca := func(bozza map[string]any) (EsitoRevoca, error) {
		raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": "sorgente_errata", "bozza": bozza})
		r, imp, err := LeggiRichiestaRevoca(raw)
		if err != nil {
			return EsitoRevoca{}, err
		}
		return Revoca(ctx, p, s.thread, s.stepA, s.utente, r, imp)
	}
	prima := contiG39(t, p)
	_, err = revoca(map[string]any{"bozza_id": es.Bozza.BozzaID, "versione_attesa": es.Bozza.Versione + 1})
	if e := codiceDi(t, err, distintabozza.CodiceBozzaVersione, distintabozza.MotivoVersioneDiversa); e.Conflitto == nil || e.Conflitto.BozzaID != es.Bozza.BozzaID {
		t.Fatalf("la bozza dichiarata superata: %+v", e)
	}
	if dopo := contiG39(t, p); dopo != prima {
		t.Fatalf("il rifiuto ha scritto: %s -> %s", prima, dopo)
	}
	rv, err := revoca(map[string]any{"bozza_id": es.Bozza.BozzaID, "versione_attesa": es.Bozza.Versione})
	if err != nil || rv.BozzaAbbandonata == nil || *rv.BozzaAbbandonata != es.Bozza.BozzaID || rv.Motivo != "sorgente_errata" {
		t.Fatalf("la revoca con la bozza dichiarata: %+v %v", rv, err)
	}
	if st, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); st.Valida {
		t.Fatalf("la sorgente vale ancora: %+v", st)
	}
}
