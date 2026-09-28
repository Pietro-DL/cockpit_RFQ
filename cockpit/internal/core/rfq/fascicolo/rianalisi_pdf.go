package fascicolo

// La rianalisi dei PDF letti da un analizzatore precedente (Smistamento F9, analizzatore 3 → 4).
//
// Dalla 4 il worker riporta il testo dei PDF (`dettagli.testo_pdf`). I PDF gia' analizzati prima hanno soltanto
// i fatti della versione precedente, sotto la loro chiave (contenuto, versione, configurazione), e la chiave
// corrente non ce l'hanno: la preparazione non li guarda (non sono fermi nello staging) e la rilettura degli
// STEP nemmeno. Senza questo file non riprenderebbero mai il testo.
//
// Come si riconoscono: un PDF della RFQ con il contenuto scaricato (sha256), gia' `analizzato`, senza i fatti
// con la chiave dell'analizzatore corrente, oppure con quei fatti ma senza il testo (un worker non aggiornato
// che ha risposto a un job della 4: classificazione.StatoDelTestoPDF dice TestoNonLetto). Che cosa vale finche'
// non e' rianalizzato: «testo non letto» (classificazione.TestoNonLetto, «da rianalizzare»), mai «senza
// testo»; la proposta resta quella di prima, e chi legge il testo corrente lo sa da TestoCorrenteDelPDF. Come
// si accoda: solo con il gesto «Rianalizza» (RianalizzaRfq), pochi alla volta, con la stessa chiave idempotente
// di ogni analisi; nessuna rianalisi in massa parte da sola (la preparazione della pagina non li accoda). Che cosa succede quando i fatti nuovi
// arrivano: la strada normale del risultato dell'analisi (workerapi), con le sue regole di oggi. Le proposte
// DECISE non si riscrivono (UpsertProposta: una riga non piu' aperta resta com'e', una decisa da una persona
// riceve la lettura nuova solo in `lettura_dopo`); quelle aperte prendono la valutazione nuova; nessuna
// identita' tecnica nasce (niente componente, relazione, documento). I fatti vecchi restano sotto la loro
// chiave. Ripetere il gesto non raddoppia niente: gia' in coda si conta, i fatti con il testo chiudono il caso.
// Un fatto corrente senza testo si riscrive (UpsertAnalisiFatti, stessa chiave) quando il worker aggiornato
// risponde; se risponde di nuovo un worker vecchio resta «non letto», e il gesto lo riaccoda di nuovo solo
// quando una persona lo ripreme.

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// pdfGiaAnalizzato dice se l'allegato e' un PDF della RFQ che un'analisi ha gia' letto (con un analizzatore
// qualunque): il candidato a una rianalisi.
func pdfGiaAnalizzato(a db.Allegato) bool {
	return strings.EqualFold(strings.TrimSpace(a.Estensione.String), "pdf") && a.Natura == db.NaturaAllegatoFile &&
		a.Sha256.Valid && a.Sha256.String != "" && a.Stato == db.StatoAllegatoAnalizzato
}

// AccodaPdfDaRileggere accoda l'analisi dei PDF della RFQ che hanno soltanto fatti di un analizzatore
// precedente (pdfGiaAnalizzato, senza i fatti con la chiave corrente) o fatti correnti senza il testo (worker
// non aggiornato: coda.RiaccodaAnalisi, che non si ferma ai fatti gia' presenti), al massimo maxAccodati. E'
// una meta' del gesto «Rianalizza» (RianalizzaRfq), e solo di quello: una persona l'ha chiesto, quindi riprova
// anche un'analisi il cui ultimo tentativo e' fallito. Lo stesso contenuto in piu' allegati si accoda una volta: al
// risultato i fatti vanno a tutte le copie con la proposta aperta. Un contenuto non piu' in staging non si
// accoda (il worker fallirebbe con «usa Riscarica»): si conta.
func AccodaPdfDaRileggere(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int) (Rianalisi, error) {
	var r Rianalisi
	allegati, err := q.ListAllegatiThread(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return r, err
	}
	hash := an.Hash()
	accodato := map[string]bool{}
	for _, a := range allegati {
		if !pdfGiaAnalizzato(a) || accodato[a.Sha256.String] {
			continue
		}
		f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String,
			VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: hash})
		switch {
		case err == nil && classificazione.StatoDelTestoPDF(f.Fatti) != classificazione.TestoNonLetto:
			continue // letto con l'analizzatore corrente (con il testo, senza, o illeggibile): niente da rianalizzare
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return r, err
		}
		if !a.PathStaging.Valid || !inStaging(a.PathStaging.String) {
			r.PdfSenzaStaging++
			continue
		}
		if r.PdfAccodati >= maxAccodati {
			r.PdfRimandati++
			continue
		}
		// RiaccodaAnalisi e non AccodaAnalisi: quella si ferma ai fatti correnti, anche a quelli senza testo
		j, err := coda.RiaccodaAnalisi(ctx, q, a, uuid.NullUUID{UUID: thread, Valid: true}, an)
		if err != nil {
			return r, err
		}
		accodato[a.Sha256.String] = true
		if j == nil {
			r.PdfGiaInCoda++ // un'analisi con la stessa chiave e' gia' pendente
			continue
		}
		r.PdfAccodati++
	}
	return r, nil
}

// TestoCorrenteDelPDF e' il testo di un PDF come lo vede adesso chi viene dopo (il flusso, F8): le letture
// normalizzate dei fatti dell'analizzatore corrente (classificazione.LettureDelPDF), per le regole del cliente
// della RFQ (m) e il nome di QUESTO allegato. Senza i fatti correnti non si indovina: un PDF gia' analizzato da
// un analizzatore precedente e' «testo non letto, da rianalizzare» (come uno con i fatti correnti ma senza il
// testo, che dice LettureDelPDF), uno non ancora analizzato e' «da analizzare». Non scrive niente e non accoda
// niente: si puo' chiamare anche aprendo una pagina.
func TestoCorrenteDelPDF(ctx context.Context, q *db.Queries, a db.Allegato, an coda.Analizzatore, m *classificazione.Motore) (classificazione.LettureTestoPDF, error) {
	daAnalizzare := classificazione.LettureTestoPDF{Stato: classificazione.TestoDaAnalizzare,
		Frase: classificazione.FraseTestoPDF(classificazione.TestoDaAnalizzare, "")}
	if !a.Sha256.Valid || a.Sha256.String == "" {
		return daAnalizzare, nil
	}
	f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String,
		VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: an.Hash()})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if pdfGiaAnalizzato(a) {
			return classificazione.LettureTestoPDF{Stato: classificazione.TestoNonLetto,
				Frase: classificazione.FraseTestoPDF(classificazione.TestoNonLetto, "")}, nil
		}
		return daAnalizzare, nil
	case err != nil:
		return classificazione.LettureTestoPDF{}, err
	}
	return classificazione.LettureDelPDF(m, f.Fatti, a.NomeFile), nil
}
