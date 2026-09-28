package fascicolo

// I gesti che cambiano la BOM working senza distruggerne la storia: archiviare un componente (A4.9),
// derogare a una lettura parziale dello STEP strutturale di un prodotto finito (A4.5, D33, D36), sostituire
// un documento con una revisione nuova (A4.1). Lo STEP strutturale non si sceglie piu' da qui: e'
// l'autorizzazione dello Smistamento (F5b, autorizzazione.go), che per un finito lo scrive anche nella colonna.
//
// Dopo il congelamento il database li rifiuta (D26); qui lo si dice prima, con le parole giuste.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// ------------------------------------------------------------------ archiviazione (A4.9)

// ArchiviaComponente toglie il componente dalla BOM working: i suoi archi working se ne vanno, in
// tutte e due le direzioni; documenti, proposte, deroghe e storia restano agganciati. I figli non si
// archiviano da soli (D29): se restano senza padre compaiono come radici da sistemare.
//
// Un componente con un file autorizzato (lo STEP strutturale di un prodotto, o la marcatura dello
// Smistamento) chiude le rimozioni ancora aperte che venivano da quel file: l'autorizzazione di un
// componente archiviato e' sospesa (ValutaDichiarazioni), il ricalcolo guarda solo quelle valide, quindi
// nessuno le avrebbe piu' chiuse, e il gate le avrebbe contate per sempre come proposte da decidere. La
// marcatura resta: con il ripristino l'autorizzazione torna valida (F5, A5.4.7). Quelle degli altri
// componenti su archi tolti qui le chiude il loro ricalcolo, come prima.
func ArchiviaComponente(ctx context.Context, q *db.Queries, thread, comp, utente uuid.UUID, motivo string) (string, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return "", Rifiuto("si archivia con un motivo")
	}
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
	}
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	if c.ArchiviatoIl != nil {
		return "", Rifiuto(c.Codice + " è già archiviato")
	}
	if err := SeBloccata(ctx, q, thread, "si archivia un componente"); err != nil {
		return "", err
	}
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return "", err
	}
	archi, err := q.DeleteRelazioniComponente(ctx, comp)
	if err != nil {
		return "", err
	}
	if _, err := q.ArchiviaComponente(ctx, db.ArchiviaComponenteParams{ComponenteID: comp,
		ArchiviatoDa: uuid.NullUUID{UUID: utente, Valid: true}, Motivo: pgtype.Text{String: motivo, Valid: true}}); err != nil {
		return "", err
	}
	nota := "componente archiviato"
	if c.Tipo == db.TipoComponenteFinito {
		nota = "prodotto archiviato"
	}
	chiusi := map[uuid.UUID]bool{}
	chiudi := func(doc uuid.UUID) error {
		if chiusi[doc] {
			return nil
		}
		chiusi[doc] = true
		_, err := q.ChiudiRimozioniDiUnoStep(ctx, db.ChiudiRimozioniDiUnoStepParams{ThreadID: thread,
			StepDocumentoID: doc, Nota: pgtype.Text{String: nota, Valid: true}})
		return err
	}
	if c.StepStrutturaleID.Valid {
		if err := chiudi(c.StepStrutturaleID.UUID); err != nil {
			return "", err
		}
	}
	for _, d := range dich.DelComponente(comp) {
		if d.Documento.Valid {
			if err := chiudi(d.Documento.UUID); err != nil {
				return "", err
			}
		}
	}
	return fmt.Sprintf("%s archiviato: tolti %d archi della working. Documenti, proposte e storia restano.", c.Codice, archi), nil
}

// RipristinaComponente rimette nella working un componente archiviato: stesso componente_id, stessa
// storia. E' quello che si fa quando lo stesso codice torna (A4.9; dai codici della RFQ, B8.6). Gli
// archi non tornano da soli. Le proposte aperte con il suo codice restano proposte (F5): prima lo
// ritrovavano da sole (RiconciliaProposteNodo), ed era un'identita' decisa dal codice; adesso lo ritrova
// chi le accetta. La sua autorizzazione, sospesa, torna valida, e le rimozioni si ricalcolano.
func RipristinaComponente(ctx context.Context, q *db.Queries, thread, comp uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si ripristina un componente"); err != nil {
		return "", err
	}
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	n, err := q.RipristinaComponente(ctx, comp)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", Rifiuto(c.Codice + " non è archiviato")
	}
	return dopoLaDecisione(ctx, q, thread, c.Codice+" ripristinato nella BOM working.", nil)
}

// RimuoviComponente prova la cancellazione fisica: riesce solo per un componente che non e' mai
// entrato in una decisione o in una baseline. Se qualcosa lo tiene, lo dice la FK, e il rifiuto dice
// che cosa e propone l'archiviazione. Chi chiama annulla la transazione.
func RimuoviComponente(ctx context.Context, q *db.Queries, thread, comp uuid.UUID) (string, error) {
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	if err := SeBloccata(ctx, q, thread, "si toglie un componente"); err != nil {
		return "", err
	}
	if _, err := q.DeleteComponente(ctx, comp); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23503" {
			return "", Rifiuto(fmt.Sprintf("%s non si cancella: %s. Si archivia", c.Codice, cheCosaLoTiene(pe.TableName)))
		}
		return "", err
	}
	return c.Codice + " tolto: non aveva storia.", nil
}

func cheCosaLoTiene(tabella string) string {
	switch tabella {
	case "documento":
		return "ha dei documenti"
	case "documento_proposta":
		return "ha delle proposte di documento"
	case "componente_proposta", "relazione_proposta":
		return "ha delle proposte strutturali"
	case "rimozione_proposta":
		return "ha delle proposte di rimozione"
	case "deroga_fabbisogno":
		return "ha delle deroghe del fabbisogno"
	case "deroga_struttura":
		return "ha una deroga strutturale"
	case "bom_versione_componente":
		return "è in una baseline congelata"
	case "annotazione_pdf":
		return "ha delle note sui disegni"
	}
	return "è ancora usato (" + tabella + ")"
}

// ------------------------------------------------------------------ STEP strutturale (A4.4, D31)

// Step dice se un documento e' uno STEP: un 3D corrente con estensione stp o step.
//
// ScegliStepStrutturale non c'e' piu' (Smistamento F5b): fissava lo STEP strutturale di un finito con un
// clic, gia' presentato scelto quando era uno solo. Adesso e' l'autorizzazione (DichiaraStrutturale), per
// qualunque componente non commerciale, con l'anteprima, la casella mai spuntata e la firma dell'effetto; per
// un finito scrive anche la colonna.
func Step(d db.Documento) bool {
	e := strings.ToLower(d.Estensione)
	return d.Tipo == db.TipoDocumentoCad3d && (e == "stp" || e == "step")
}

// ------------------------------------------------------------------ deroga strutturale (A4.5, D33, D36)

// ConcediDerogaStruttura dichiara «si congela con QUESTO STEP letto in parte». Vale per lo STEP
// strutturale corrente e per la sua analisi corrente, letti qui, nella stessa transazione: una
// rianalisi, una sostituzione o la prima analisi di uno STEP derogato prima di essere analizzato la
// fanno decadere, e v_step_prodotto smette di mostrarla.
func ConcediDerogaStruttura(ctx context.Context, q *db.Queries, thread, comp, utente uuid.UUID, motivo string) (db.DerogaStruttura, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return db.DerogaStruttura{}, Rifiuto("una deroga si concede con il suo motivo")
	}
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return db.DerogaStruttura{}, err
	}
	if err := SeBloccata(ctx, q, thread, "si concede una deroga"); err != nil {
		return db.DerogaStruttura{}, err
	}
	sp, err := q.GetStepProdotto(ctx, comp)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.DerogaStruttura{}, Rifiuto(c.Codice + " non è un prodotto finito della BOM working")
	}
	if err != nil {
		return db.DerogaStruttura{}, err
	}
	switch sp.Esito {
	case StepParziale, StepNonAnalizzato:
	case StepAnalizzato:
		return db.DerogaStruttura{}, Rifiuto(c.Codice + ": lo STEP strutturale è letto per intero, la deroga non serve")
	case StepDaScegliere, StepRiferimentoSuperato:
		return db.DerogaStruttura{}, Rifiuto(c.Codice + ": prima si sceglie lo STEP strutturale, la deroga vale per quel file")
	default:
		return db.DerogaStruttura{}, Rifiuto(c.Codice + ": non c'è uno STEP da derogare; per congelare senza STEP serve la deroga del fabbisogno cad_3d")
	}
	d, err := q.GetDocumento(ctx, sp.StepStrutturaleID.UUID)
	if err != nil {
		return db.DerogaStruttura{}, err
	}
	arg := db.InsertDerogaStrutturaParams{ThreadID: thread, ComponenteID: comp, StepDocumentoID: d.DocumentoID, StepSha256: d.Sha256,
		MotivoParziale: sp.MotivoParziale.String, Motivo: motivo, ConcessaDa: utente}
	ac, err := q.GetAnalizzatoreCorrente(ctx)
	switch {
	case err == nil:
		af, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: d.Sha256, VersioneAnalizzatore: ac.VersioneAnalizzatore,
			HashConfigurazione: ac.HashConfigurazione})
		switch {
		case err == nil:
			calcolato := af.CalcolatoIl
			arg.VersioneAnalizzatore = pgtype.Int2{Int16: af.VersioneAnalizzatore, Valid: true}
			arg.HashConfigurazione = pgtype.Text{String: af.HashConfigurazione, Valid: true}
			arg.AnalisiCalcolataIl = &calcolato
		case !errors.Is(err, pgx.ErrNoRows):
			return db.DerogaStruttura{}, err
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return db.DerogaStruttura{}, err
	}
	return q.InsertDerogaStruttura(ctx, arg)
}

// RevocaDerogaStruttura toglie una deroga strutturale di questa RFQ: l'id da solo non basta, arriva da
// un indirizzo e potrebbe essere di un'altra (come per RevocaDeroga). Se una baseline la usa, la FK lo
// rifiuta: una deroga che ha fatto congelare una versione fa parte di quella versione.
func RevocaDerogaStruttura(ctx context.Context, q *db.Queries, thread, deroga uuid.UUID) error {
	if err := prepara(ctx, q, thread, "si revoca una deroga"); err != nil {
		return err
	}
	n, err := q.DeleteDerogaStruttura(ctx, db.DeleteDerogaStrutturaParams{DerogaStrutturaID: deroga, ThreadID: thread})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23503" {
		return Rifiuto("la deroga strutturale è in una baseline congelata: non si toglie, decade da sola")
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return Rifiuto("deroga strutturale non trovata in questa RFQ")
	}
	return nil
}

// ------------------------------------------------------------------ revisioni dei documenti (A4.1)

// Sostituisci dice «nuovo sostituisce vecchio»: una revisione nuova e' una riga nuova, e la vecchia
// resta, con sostituito_da. Stesso componente e stesso tipo (D32); la catena la tiene il database.
// Uno STEP sostituito chiude le proposte ancora aperte che venivano da lui.
//
// Se il vecchio era lo STEP autorizzato del componente (Smistamento F5b, A5.4.7), la domanda «il nuovo file
// diventa lo STEP autorizzato?» non ha una risposta preselezionata: il gesto la chiede, e senza risposta la
// sostituzione si rifiuta (P26). «Si'» (nuovoRiferimento) e' l'autorizzazione del file nuovo
// (DichiaraStrutturale), che revoca la vecchia: la radice vecchia non torna aperta, perche' e' davvero il
// componente nella revisione di prima, e la marcatura va nella sua storia con l'evento sostituita_da. Se il
// file nuovo non si autorizza cosi' com'e' (piu' radici, un altro nome della radice, nessuna analisi), si
// rifiuta tutto e lo si dice: si risponde «no», e il file nuovo si autorizza dalla scheda con l'anteprima.
// «No»: l'autorizzazione resta sul file sostituito ed e' «superata», e il gate si ferma finche' qualcuno non
// autorizza il file nuovo (come riferimento_superato per lo STEP strutturale di un finito).
func Sostituisci(ctx context.Context, q *db.Queries, thread, vecchio, nuovo uuid.UUID, nuovoRiferimento bool, utente uuid.UUID) (string, error) {
	if vecchio == nuovo {
		return "", Rifiuto("un documento non sostituisce se stesso")
	}
	docs, err := bloccaDocumenti(ctx, q, thread, vecchio, nuovo)
	if err != nil {
		return "", err
	}
	v, n := docs[vecchio], docs[nuovo]
	switch {
	case !v.ComponenteID.Valid:
		return "", Rifiuto(fmt.Sprintf("si sostituisce un documento di un pezzo: %s non è assegnato a nessun componente", v.NomeFile))
	case !n.ComponenteID.Valid || n.ComponenteID.UUID != v.ComponenteID.UUID:
		return "", Rifiuto(fmt.Sprintf("%s e %s non sono dello stesso componente: prima si assegna il file nuovo", v.NomeFile, n.NomeFile))
	case n.Tipo != v.Tipo:
		return "", Rifiuto(fmt.Sprintf("un %s si sostituisce con un %s, non con un %s", v.Tipo, v.Tipo, n.Tipo))
	case v.SostituitoDa.Valid:
		return "", Rifiuto(v.NomeFile + " è già stato sostituito")
	case n.SostituitoDa.Valid:
		return "", Rifiuto(n.NomeFile + " è a sua volta sostituito: si sceglie un documento corrente")
	}
	if err := SeBloccata(ctx, q, thread, "si sostituisce un documento"); err != nil {
		return "", err
	}
	k, err := q.SetSostituitoDa(ctx, db.SetSostituitoDaParams{Vecchio: vecchio, Nuovo: uuid.NullUUID{UUID: nuovo, Valid: true}})
	if err != nil {
		return "", err
	}
	if k != 1 {
		return "", Rifiuto(v.NomeFile + " è stato sostituito nel frattempo")
	}
	msg := fmt.Sprintf("%s sostituito da %s.", v.NomeFile, n.NomeFile)
	if v.Tipo == db.TipoDocumentoCad3d {
		if err := chiudiProposteDi(ctx, q, thread, v, "superata da "+n.NomeFile); err != nil {
			return "", err
		}
	}
	c, err := q.GetComponente(ctx, v.ComponenteID.UUID)
	if err != nil {
		return "", err
	}
	autorizzato, err := StepAutorizzatoDi(ctx, q, thread, c, vecchio)
	if err != nil || !autorizzato {
		return msg, err
	}
	if !nuovoRiferimento || !Step(n) {
		return msg + " L'autorizzazione dello STEP di " + c.Codice + " resta sul file sostituito ed è superata: " +
			"si autorizza il file nuovo dalla scheda di " + c.Codice + ".", nil
	}
	e, err := EffettoAutorizzazione(ctx, q, thread, RichiestaAutorizzazione{Componente: c.ComponenteID, Documento: nuovo})
	if err != nil {
		return "", err
	}
	perche := e.Spento
	switch {
	case perche != "":
	case e.Sorgente == nil:
		perche = fmt.Sprintf("%s ha %d radici, e quale sia %s lo indica una persona", n.NomeFile, len(e.Radici), c.Codice)
	case e.PresaDAtto != "":
		perche = e.PresaDAtto + ", e serve la presa d'atto"
	}
	if perche != "" {
		// il rifiuto annulla anche la sostituzione
		return "", Rifiuto(fmt.Sprintf("%s non diventa lo STEP autorizzato di %s così com'è (%s): si risponde «no», "+
			"e il file nuovo si autorizza dalla scheda di %s, con l'anteprima", n.NomeFile, c.Codice, perche, c.Codice))
	}
	m, err := DichiaraStrutturale(ctx, q, thread, utente, RichiestaAutorizzazione{Componente: c.ComponenteID, Documento: nuovo,
		Autorizza: true, Firma: e.Firma})
	if err != nil {
		return "", err
	}
	return msg + " " + m, nil
}

// StepAutorizzatoDi dice se il documento e' lo STEP autorizzato del componente (la marcatura o la forma di
// prima, valida o da sistemare; non una delega, il cui documento e' del padre): la domanda della sostituzione
// si fa solo allora.
func StepAutorizzatoDi(ctx context.Context, q *db.Queries, thread uuid.UUID, c db.Componente, doc uuid.UUID) (bool, error) {
	if c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == doc {
		return true, nil
	}
	dich, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return false, err
	}
	for _, d := range dich.DelComponente(c.ComponenteID) {
		if !d.Delega() && d.Documento.Valid && d.Documento.UUID == doc {
			return true, nil
		}
	}
	return false, nil
}

// AnnullaSostituzione annulla l'ultima sostituzione: il successore deve essere ancora corrente. Le
// proposte chiuse dalla sostituzione restano chiuse.
func AnnullaSostituzione(ctx context.Context, q *db.Queries, thread, vecchio uuid.UUID) (string, error) {
	v, err := q.BloccaDocumento(ctx, vecchio)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && v.ThreadID != thread) {
		return "", Rifiuto("il documento non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	if !v.SostituitoDa.Valid {
		return "", Rifiuto(v.NomeFile + " non è sostituito")
	}
	s, err := q.BloccaDocumento(ctx, v.SostituitoDa.UUID)
	if err != nil {
		return "", err
	}
	if s.SostituitoDa.Valid {
		return "", Rifiuto(fmt.Sprintf("%s è stato a sua volta sostituito: si annulla prima quella sostituzione", s.NomeFile))
	}
	if err := SeBloccata(ctx, q, thread, "si annulla una sostituzione"); err != nil {
		return "", err
	}
	k, err := q.AnnullaSostituzione(ctx, db.AnnullaSostituzioneParams{Vecchio: vecchio, Successore: uuid.NullUUID{UUID: s.DocumentoID, Valid: true}})
	if err != nil {
		return "", err
	}
	if k != 1 {
		return "", Rifiuto(v.NomeFile + ": la sostituzione è cambiata nel frattempo")
	}
	return fmt.Sprintf("Sostituzione annullata: %s è di nuovo corrente, accanto a %s.", v.NomeFile, s.NomeFile), nil
}

// bloccaDocumenti blocca i documenti in ordine di id, cosi' due gesti sugli stessi file non si
// aspettano a vicenda, e controlla che siano della RFQ.
func bloccaDocumenti(ctx context.Context, q *db.Queries, thread uuid.UUID, ids ...uuid.UUID) (map[uuid.UUID]db.Documento, error) {
	ordinati := append([]uuid.UUID{}, ids...)
	for i := range ordinati {
		for j := i + 1; j < len(ordinati); j++ {
			if ordinati[j].String() < ordinati[i].String() {
				ordinati[i], ordinati[j] = ordinati[j], ordinati[i]
			}
		}
	}
	out := map[uuid.UUID]db.Documento{}
	for _, id := range ordinati {
		d, err := q.BloccaDocumento(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.ThreadID != thread) {
			return nil, Rifiuto("un documento scelto non è di questa RFQ")
		}
		if err != nil {
			return nil, err
		}
		out[id] = d
	}
	return out, nil
}

// chiudiProposteDi chiude le proposte ancora aperte che venivano dal file d (A4.4): scartate, senza chi
// le ha decise, con la nota. Quelle gia' decise restano come sono. La nota nomina un file («superata da
// <nome>», fino a 300 caratteri) e le colonne ne tengono 200: si taglia qui, non la si rifiuta.
func chiudiProposteDi(ctx context.Context, q *db.Queries, thread uuid.UUID, d db.Documento, nota string) error {
	n := pgtype.Text{String: tagliaNota(nota), Valid: true}
	if _, err := q.ChiudiProposteComponenteDiUnFile(ctx, db.ChiudiProposteComponenteDiUnFileParams{ThreadID: thread, Sha256: d.Sha256, Nota: n}); err != nil {
		return err
	}
	if _, err := q.ChiudiProposteRelazioneDiUnFile(ctx, db.ChiudiProposteRelazioneDiUnFileParams{ThreadID: thread, Sha256: pgtype.Text{String: d.Sha256, Valid: true}, Nota: n}); err != nil {
		return err
	}
	_, err := q.ChiudiRimozioniDiUnoStep(ctx, db.ChiudiRimozioniDiUnoStepParams{ThreadID: thread, StepDocumentoID: d.DocumentoID, Nota: n})
	return err
}
