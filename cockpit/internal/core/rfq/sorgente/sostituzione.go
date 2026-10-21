package sorgente

// sostituzione.go: S8 di un altro STEP su un prodotto la cui bozza aperta e' nata da una sorgente S8, prima di R5 (giro
// G39-ter). Le decisioni dell'utente:
//   - G39-03 [U] (20/10): «la bozza appena creata deve essere annullata siccome la sorgente step cambia»; dopo R5 il cambio
//     di sorgente e' una revisione interna;
//   - messaggio 43, caso 1 [U] (21/10): prima di R5 «si puo' tranquillamente cancellare la bozza sbagliata per crearne una
//     giusta sulla fonte strutturale giusta [...] bisogna registrare in "fonte_step" questo cambiamento di scelta, ovvero
//     la revoca [...] e la successiva conferma [...] (quindi anche i registri delle bozze devono annotare la revoca ed il
//     successivo riassegnamento)»; «la storia non deve essere cancellata».
// Prima (s8.go, caso (d)) il passo 3 faceva il cambio della fonte sulla stessa bozza: la bozza sbagliata restava, con le
// sue decisioni, e sulla sorgente vecchia non c'era una revoca.
//
// La forma, [T-39] della corsia sorgente, senza DDL (la variante 20261019d della 0023 resta com'e'):
//   - quando: la bozza aperta del prodotto e' nel formato 3, e' nata da S8 con la partenza step (l'evento bozza_aperta della
//     versione 1 dice partenza step), la sua sorgente di adesso e' uno STEP con struttura (non solo_prodotto) confermato da
//     S8 per il prodotto (una riga conferma_documento con la chiave e il documento), il prodotto non ha nessuna conferma
//     della BOM (prima di qualunque R5) e S8 conferma, con la partenza step, un contenuto diverso. Restano come prima: la
//     bozza nata da una conferma (da-conferma-1 e cambio della fonte, la revisione del messaggio 41), la bozza manuale o
//     solo_prodotto (la riconciliazione della direttiva 39 §3), lo stesso contenuto con un altro gruppo (il cambio della
//     fonte), «Sposta la sorgente su questo prodotto» (sposta.go) e la revoca dopo R5 (revoca.go);
//   - la presa d'atto: il corpo dichiara la bozza (bozza_id, versione_attesa) come per il cambio della fonte; se la bozza
//     vecchia ha modifiche dell'utente (eventi del giornale con origine utente o step) serve anche «sostituisci»
//     ({conferma: true}), altrimenti CORPO_NON_VALIDO presa_d_atto_mancante, campo sostituisci, prima di ogni scrittura;
//   - passo 2 (k2, sotto il lucchetto, prima della riga di S8): la revoca della sorgente vecchia in fonte_step_evento
//     (azione revoca_sorgente, ruolo prodotto, il documento della sua conferma S8, la bozza vecchia, la chiave k2r derivata
//     dalla chiave d'intento, il motivo del server «sorgente_errata: sostituita dalla conferma S8 di <sha abbreviato>»,
//     con il motivo dell'utente in coda se c'e'), cosi' la storia della fonte e' revoca, poi conferma (messaggio 43); se la
//     sorgente vecchia e' gia' revocata (una S8 precedente fermata al passo 3) la revoca non si riscrive;
//   - passo 3 (k3): la bozza vecchia passa ad abbandonata (il registro lo scrive il trigger) e la nuova nasce dalla proposta
//     del motore sulla sorgente nuova, con le PB iniziali (CreaFormato3 con Abbandona, partenza step), nella stessa
//     transazione dell'evento riassegnazione_sorgente sulla versione 1 della bozza nuova (distintabozza.ScriviRiassegnazione)
//     e dell'intento; la chiusura della bozza vecchia dice la causa e la bozza nuova;
//   - la risposta porta «sostituzione» (la bozza abbandonata, la sorgente di prima, la revoca, le modifiche dell'utente) e
//     il testo lo dice.
// Niente tocca la working, step_strutturale_id, sostituito_da o le AR (lo fa solo R5).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// Sostituzione: il piano del passo 0 quando la bozza aperta va sostituita (commento di testa).
type Sostituzione struct {
	Vecchia db.DistintaBozza
	// la sorgente della bozza vecchia e il documento della sua ultima conferma S8
	Allegato  uuid.UUID
	NomeFile  string
	Sha256    string
	Gruppo    string
	Documento uuid.UUID
	// RevocaEvento: la revoca della sorgente vecchia gia' scritta dopo quella conferma (nil: da scrivere nel passo 2)
	RevocaEvento *int64
	// ModificheUtente: il giornale della bozza vecchia ha eventi con origine utente o step
	ModificheUtente bool
}

// EsitoSostituzione: che cosa S8 ha fatto della bozza vecchia (nella risposta di S8, campo sostituzione).
type EsitoSostituzione struct {
	BozzaAbbandonata    uuid.UUID `json:"bozza_abbandonata"`
	VersioneAbbandonata int       `json:"versione_abbandonata"`
	AllegatoPrecedente  uuid.UUID `json:"allegato_precedente"`
	ShaPrecedente       string    `json:"sha256_precedente"`
	RevocaEvento        int64     `json:"revoca_evento"`
	ModificheUtente     bool      `json:"modifiche_utente,omitempty"`
	Motivo              string    `json:"motivo"`
	Testo               string    `json:"testo"`
}

// valutaSostituzione: la bozza aperta b del prodotto va sostituita dalla S8 di sha (commento di testa)? nil se no.
func valutaSostituzione(ctx context.Context, q *db.Queries, thread uuid.UUID, r RichiestaS8, b db.DistintaBozza, sha string) (*Sostituzione, error) {
	if r.Partenza != PartenzaStep || int(b.Formato) != distintabozza.FormatoSnapshot3 || !b.FonteAllegatoID.Valid || !b.FonteSha256.Valid ||
		b.FonteGruppo.String == distintabozza.GruppoSoloProdotto {
		return nil, nil
	}
	vecchio := strings.TrimSpace(b.FonteSha256.String)
	if vecchio == sha {
		return nil, nil
	}
	// nata da S8 con la partenza step (non da una conferma, non manuale)
	e1, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: b.BozzaID, ThreadID: thread, N: 1})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var aperta struct {
		Partenza string `json:"partenza"`
	}
	if e1.Azione != distintabozza.AzBozzaAperta || e1.Dopo == nil || json.Unmarshal(*e1.Dopo, &aperta) != nil || aperta.Partenza != PartenzaStep {
		return nil, nil
	}
	// prima di qualunque R5 del prodotto
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	if _, ok := st.Ultima(b.ProdottoID); ok {
		return nil, nil
	}
	// la sorgente vecchia confermata da S8 per il prodotto, e la revoca gia' scritta dopo, se c'e'
	ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: thread, ProdottoID: b.ProdottoID})
	if err != nil {
		return nil, err
	}
	out := &Sostituzione{Vecchia: b, Allegato: b.FonteAllegatoID.UUID, Sha256: vecchio, Gruppo: b.FonteGruppo.String}
	confermata := false
	for _, e := range ee {
		if e.Ruolo != "prodotto" || strings.TrimSpace(e.Sha256) != vecchio {
			continue
		}
		switch e.Azione {
		case "conferma_documento":
			if e.ChiaveIdempotenza.Valid && e.DocumentoID.Valid {
				confermata, out.Documento, out.RevocaEvento = true, e.DocumentoID.UUID, nil
			}
		case "revoca", AzioneRevocaSorgente:
			if confermata {
				id := e.EventoID
				out.RevocaEvento = &id
			}
		}
	}
	if !confermata {
		return nil, nil
	}
	if out.ModificheUtente, err = modificheDellUtente(ctx, q, thread, b); err != nil {
		return nil, err
	}
	if a, err := q.GetAllegato(ctx, out.Allegato); err == nil {
		out.NomeFile = a.NomeFile
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return out, nil
}

// modificheDellUtente: il giornale della bozza ha eventi dichiarati dalla pagina (origine utente o step).
func modificheDellUtente(ctx context.Context, q *db.Queries, thread uuid.UUID, b db.DistintaBozza) (bool, error) {
	dopo := int32(0)
	for {
		ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: b.BozzaID, ThreadID: thread, VersioneMax: b.Versione,
			DopoN: dopo, Limite: 500})
		if err != nil {
			return false, err
		}
		for _, e := range ee {
			if e.Origine == "utente" || e.Origine == "step" {
				return true, nil
			}
			dopo = e.N
		}
		if len(ee) < 500 {
			return false, nil
		}
	}
}

// controllaSostituzione: la presa d'atto della sostituzione (commento di testa); la bozza dichiarata l'ha gia' controllata
// Controlla.
func controllaSostituzione(r RichiestaS8, p db.GetProdottoDellaRfqRow, s *Sostituzione) error {
	if s.ModificheUtente && r.Sostituisci == nil {
		return errCorpo("sostituisci", MotivoPresaDAttoMancante,
			"la bozza aperta di %s (versione %d) ha modifiche tue ed e' nata da un altro STEP: la conferma di questo STEP la sostituisce con una bozza nuova "+
				"dalla proposta (la bozza di adesso resta nella storia); serve la presa d'atto (sostituisci)", p.Codice, s.Vecchia.Versione)
	}
	return nil
}

// motivoSostituzione: il motivo del server della revoca, della chiusura della bozza vecchia e dell'evento.
func motivoSostituzione(r RichiestaS8, shaNuovo string) string {
	t := "sostituita dalla conferma S8 di " + abbreviato(shaNuovo)
	if r.Motivo != "" {
		t += ": " + r.Motivo
	}
	return confermabom.MotivoConCausa(confermabom.CausaSorgenteErrata, t)
}

func abbreviato(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// scriviRevocaSostituzione: la revoca della sorgente vecchia (commento di testa, passo 2), nella transazione di chi chiama
// con il lucchetto della RFQ; se c'e' gia', l'evento di quella.
func scriviRevocaSostituzione(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, r RichiestaS8, s *Sostituzione, shaNuovo string, k uuid.UUID) (int64, error) {
	if s.RevocaEvento != nil {
		return *s.RevocaEvento, nil
	}
	ins := db.InsertFonteStepEventoParams{ThreadID: thread, ProdottoID: r.ProdottoID, Ruolo: "prodotto", Azione: AzioneRevocaSorgente, AllegatoID: s.Allegato,
		Sha256: s.Sha256, DocumentoID: uuid.NullUUID{UUID: s.Documento, Valid: true}, BozzaID: uuid.NullUUID{UUID: s.Vecchia.BozzaID, Valid: true},
		Motivo: pgtype.Text{String: troncato(motivoSostituzione(r, shaNuovo), 1000), Valid: true}, ChiaveIdempotenza: uuid.NullUUID{UUID: k, Valid: true},
		AutoreID: utente}
	if s.Gruppo != "" {
		ins.Gruppo = pgtype.Text{String: s.Gruppo, Valid: true}
	}
	ev, err := q.InsertFonteStepEvento(ctx, ins)
	if err != nil {
		return 0, err
	}
	id := ev.EventoID
	s.RevocaEvento = &id
	return id, nil
}

func troncato(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-3]) + "..."
	}
	return s
}

// esitoSostituzione: il campo della risposta, con il testo per la pagina.
func esitoSostituzione(c Contesto, r RichiestaS8, s *Sostituzione, revoca int64) *EsitoSostituzione {
	nome := s.Allegato.String()
	if s.NomeFile != "" {
		nome = s.NomeFile
	}
	t := fmt.Sprintf("La bozza precedente di %s (versione %d, nata da %s) è sostituita: è annullata con la sua storia, la sua sorgente è revocata "+
		"(sorgente errata) e la bozza nuova nasce dalla proposta di questo STEP.", c.Prodotto.Codice, s.Vecchia.Versione, nome)
	if s.ModificheUtente {
		t += " Le modifiche fatte su quella bozza restano nella sua storia e non passano alla bozza nuova."
	}
	return &EsitoSostituzione{BozzaAbbandonata: s.Vecchia.BozzaID, VersioneAbbandonata: int(s.Vecchia.Versione), AllegatoPrecedente: s.Allegato,
		ShaPrecedente: s.Sha256, RevocaEvento: revoca, ModificheUtente: s.ModificheUtente, Motivo: motivoSostituzione(r, c.Sha256), Testo: t}
}

// testoCompletaSostituzione: il testo della risposta completa con la sostituzione.
func testoCompletaSostituzione(codice string) string {
	return "Sorgente confermata: la bozza precedente di " + codice + " è stata sostituita da una bozza nuova dalla proposta, pronta da rivedere. " +
		"La BOM non è confermata."
}

// AnteprimaSostituzione: per l'anteprima S6 con ?bozza= (effetto_sulla_bozza.sostituzione): la conferma di questo STEP
// sostituirebbe la bozza aperta invece di cambiarne la fonte (commento di testa), con la presa d'atto se ha modifiche
// dell'utente. Sola lettura.
type AnteprimaSostituzione struct {
	BozzaID            uuid.UUID `json:"bozza_id"`
	Versione           int       `json:"versione"`
	AllegatoPrecedente uuid.UUID `json:"allegato_precedente"`
	NomeFile           string    `json:"nome_file,omitempty"`
	ModificheUtente    bool      `json:"modifiche_utente,omitempty"`
	Testo              string    `json:"testo"`
}

// SostituzioneDellaBozza: la bozza aperta b del prodotto sarebbe sostituita dalla S8 (partenza step) dello STEP con quello
// sha256? nil se no (il cambio della fonte o la riconciliazione di sempre). Nella transazione di sola lettura di chi chiama.
func SostituzioneDellaBozza(ctx context.Context, q *db.Queries, thread uuid.UUID, b db.DistintaBozza, sha string) (*AnteprimaSostituzione, error) {
	if b.Stato != distintabozza.StatoAperta {
		return nil, nil
	}
	s, err := valutaSostituzione(ctx, q, thread, RichiestaS8{Partenza: PartenzaStep}, b, strings.TrimSpace(sha))
	if err != nil || s == nil {
		return nil, err
	}
	nome := s.NomeFile
	if nome == "" {
		nome = "un altro STEP"
	}
	t := fmt.Sprintf("La bozza aperta (versione %d) è nata da %s e la BOM non è mai stata confermata: confermando questo STEP la bozza di adesso è annullata "+
		"(la sua storia resta), la sua sorgente è revocata (sorgente errata) e la bozza nuova nasce dalla proposta di questo STEP.", b.Versione, nome)
	if s.ModificheUtente {
		t += " Le modifiche fatte su quella bozza non passano alla bozza nuova: serve la presa d'atto."
	}
	return &AnteprimaSostituzione{BozzaID: b.BozzaID, Versione: int(b.Versione), AllegatoPrecedente: s.Allegato, NomeFile: s.NomeFile,
		ModificheUtente: s.ModificheUtente, Testo: t}, nil
}
