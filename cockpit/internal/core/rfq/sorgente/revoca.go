package sorgente

// revoca.go: la revoca della sorgente confermata da S8 (direttiva 39 §2, «Revoca» [U + T-39]): il gesto esplicito per
// correggere uno STEP scelto per errore, con un evento motivato in fonte_step_evento (azione revoca_sorgente, ruolo
// prodotto, con il documento, la chiave d'intento e, se c'e', la bozza aperta su quella sorgente). Non e' la
// sostituzione per revisione: non tocca la working, il documento, la copia sul NAS, le AR. Una sorgente revocata non e'
// piu' valida per R5 (confermabom.SorgenteValida) finche' S8 non ne conferma una valida. L'esito va in esito_intento con il
// gesto revoca_sorgente: lo stesso reinvio e' «riproposto», un altro corpo con la stessa chiave e' CHIAVE_RIUSATA. Azione
// e gesto propri dalla variante 20261019c della 0023, vigente la 20261019d (R39-10, R39-11 decisi dall'orchestratore
// [T-39]): l'azione revoca resta la revoca dell'autorizzazione legacy, che i lettori di autorizzata_il leggono.
//
// G39-03 [U] (20/10): «la bozza appena creata deve essere annullata siccome la sorgente step cambia». Prima di R5 (lo STEP
// non e' entrato in una conferma della BOM del prodotto: usataDaConferma) la bozza aperta nata da quella sorgente
// (fonte_sha256) si annulla nella stessa transazione: abbandonata con lo stato e il registro che ci sono gia', la storia
// resta, niente si cancella (bozza_abbandonata nella risposta). Dopo R5 la revoca resta un evento e basta: il cambio di
// sorgente e' una revisione (S8 della sorgente giusta, R4, R5), segnata come correzione della sorgente. Il corpo puo' dire
// la causa sorgente_errata (sorgente_errata: true, oppure un motivo che comincia con «sorgente_errata»): sta in testa al
// motivo registrato (confermabom.MotivoConCausa) e, se lo STEP revocato era entrato in una conferma della BOM del prodotto
// (dopo R5), fa della S8 successiva di un altro STEP una correzione (confermabom.SorgenteValida, Correzione); prima di R5 no. Il corpo puo' dichiarare la bozza che la pagina mostrava (bozza: bozza_id,
// versione_attesa): se non e' piu' quella, 409 BOZZA_VERSIONE e niente scritto [T-39].
//
// Messaggio 46 [U] (21/10): il motivo scritto dall'utente si toglie da ogni widget. Il corpo non lo chiede piu': se manca,
// il motivo della riga (ck_fonte_motivo lo vuole per la revoca) e' la traccia del gesto scritta dal server (core/rfq/traccia,
// TracciaRevoca), con la causa sorgente_errata in testa se il corpo la dice. Se un client di prima manda un motivo, vale
// il suo. L'impronta resta quella del corpo mandato: la traccia non la cambia.

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
	"promatec/cockpit/internal/core/rfq/traccia"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// GestoRevocaSorgente: il gesto della revoca in esito_intento (ck_esito_intento_gesto, dalla variante 20261019c; vigente la
// 20261019d).
const GestoRevocaSorgente = "revoca_sorgente"

// AzioneRevocaSorgente: l'azione della revoca in fonte_step_evento (ck_fonte_azione, ck_fonte_revoca_sorgente; dalla
// variante 20261019c, vigente la 20261019d), distinta dalla revoca dell'autorizzazione legacy (azione revoca).
const AzioneRevocaSorgente = confermabom.AzioneRevocaSorgente

// RichiestaRevoca: il corpo della revoca [T-39].
type RichiestaRevoca struct {
	ChiaveIdempotenza uuid.UUID `json:"chiave_idempotenza"`
	ProdottoID        uuid.UUID `json:"prodotto_id"`
	// Motivo: facoltativo dal messaggio 46 [U] (i client di prima lo mandano ancora); senza, la traccia del server
	Motivo string `json:"motivo,omitempty"`
	// SorgenteErrata: la causa sorgente_errata in testa al motivo registrato (G39-03; [T-39])
	SorgenteErrata bool `json:"sorgente_errata,omitempty"`
	// Bozza: la bozza aperta che la pagina mostrava (facoltativa); se c'e' deve essere ancora quella, alla stessa versione
	Bozza *BozzaS8 `json:"bozza,omitempty"`
	// traccia: il motivo scritto dal server quando il corpo non ne porta uno (messaggio 46 [U]); non e' del corpo
	traccia string
}

// WidgetRevoca: il gesto della pagina che la traccia della revoca nomina.
const WidgetRevoca = "Revoca come sorgente"

// TracciaRevoca: la traccia della revoca senza il motivo dell'utente (messaggio 46 [U]), per esempio «gesto di PS da
// «Revoca come sorgente»: lo STEP «ACME-1.stp» non e' piu' la sorgente confermata di ACME-1».
func TracciaRevoca(sigla, nomeFile, codiceProdotto string) string {
	return traccia.Gesto(distintabozza.MaxMotivo, sigla, WidgetRevoca,
		"lo STEP «"+strings.TrimSpace(nomeFile)+"» non è più la sorgente confermata di "+strings.TrimSpace(codiceProdotto))
}

// motivoRegistrato: il motivo della riga della revoca (quello dell'utente o la traccia), con la causa sorgente_errata in
// testa se il corpo la dice.
func (r RichiestaRevoca) motivoRegistrato() string {
	m := traccia.DellUtente(r.Motivo, r.traccia)
	if r.SorgenteErrata {
		return confermabom.MotivoConCausa(confermabom.CausaSorgenteErrata, m)
	}
	return m
}

// EsitoRevoca: la risposta 200 della revoca.
type EsitoRevoca struct {
	Esito             string     `json:"esito"` // revocata
	Riproposto        bool       `json:"riproposto"`
	ChiaveIdempotenza uuid.UUID  `json:"chiave_idempotenza"`
	ProdottoID        uuid.UUID  `json:"prodotto_id"`
	AllegatoID        uuid.UUID  `json:"allegato_id"`
	DocumentoID       uuid.UUID  `json:"documento_id"`
	BozzaID           *uuid.UUID `json:"bozza_id,omitempty"`
	// BozzaAbbandonata: la bozza nata da quella sorgente, annullata con la revoca prima di R5 (G39-03 [U])
	BozzaAbbandonata *uuid.UUID `json:"bozza_abbandonata,omitempty"`
	// UsataDaConferma: lo STEP e' gia' nella BOM confermata del prodotto: la revoca non annulla la bozza, il cambio di
	// sorgente e' una revisione
	UsataDaConferma bool   `json:"usata_da_conferma,omitempty"`
	Motivo          string `json:"motivo"`
	Testo           string `json:"testo"`
}

// LeggiRichiestaRevoca legge il corpo in modo stretto, con l'impronta della forma canonica.
func LeggiRichiestaRevoca(raw []byte) (RichiestaRevoca, string, error) {
	var r RichiestaRevoca
	can, err := jsoncanonico.Codifica(json.RawMessage(raw))
	if err != nil {
		return r, "", errCorpo("richiesta", "", "il JSON non si legge: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, "", errCorpo("richiesta", "", "il JSON non ha la forma attesa: %v", err)
	}
	if dec.More() {
		return r, "", errCorpo("richiesta", "", "dopo il JSON c'e' altro")
	}
	if r.ChiaveIdempotenza.Version() != 4 {
		return r, "", errCorpo("chiave_idempotenza", "", "manca la chiave d'intento, un UUID v4")
	}
	if r.ProdottoID == uuid.Nil {
		return r, "", errCorpo("prodotto_id", "", "manca il prodotto")
	}
	// il motivo non e' piu' obbligatorio (messaggio 46 [U]): senza, la traccia del server (Revoca)
	r.Motivo = strings.TrimSpace(r.Motivo)
	if len([]rune(r.motivoRegistrato())) > 1000 {
		return r, "", errCorpo("motivo", "", "il motivo sta in 1000 caratteri")
	}
	if r.Bozza != nil && (r.Bozza.BozzaID == uuid.Nil || r.Bozza.VersioneAttesa < 1) {
		return r, "", errCorpo("bozza", "", "bozza vuole bozza_id e versione_attesa (almeno 1)")
	}
	return r, jsoncanonico.Impronta(can), nil
}

// Revoca scrive la revoca (commento di testa), in una transazione con il lucchetto della RFQ.
func Revoca(ctx context.Context, pool distintabozza.Avviatore, thread, allegato, utente uuid.UUID, r RichiestaRevoca, impronta string) (EsitoRevoca, error) {
	out := EsitoRevoca{Esito: "revocata", ChiaveIdempotenza: r.ChiaveIdempotenza, ProdottoID: r.ProdottoID, AllegatoID: allegato}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, errCodice(distintabozza.CodiceRFQInesistente, "", "la RFQ non c'e'")
		}
		return out, err
	}
	if gia, err := IntentoRegistrato(ctx, q, thread, r.ChiaveIdempotenza, impronta, GestoRevocaSorgente); err != nil {
		return out, err
	} else if gia != nil {
		var x EsitoRevoca
		if err := json.Unmarshal(gia, &x); err != nil {
			return out, err
		}
		x.Riproposto = true
		return x, nil
	}
	a, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{AllegatoID: allegato, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errCodice(distintabozza.CodiceFuoriRFQ, "allegato", "il file non e' di questa RFQ")
	}
	if err != nil {
		return out, err
	}
	p, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: thread, ComponenteID: r.ProdottoID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errCodice(distintabozza.CodiceFuoriRFQ, "prodotto_id", "il prodotto non e' di questa RFQ")
	}
	if err != nil {
		return out, err
	}
	if p.Tipo != db.TipoComponenteFinito {
		return out, errCorpo("prodotto_id", MotivoProdottoNonAmmesso, "%s non e' un prodotto finito", p.Codice)
	}
	sha := strings.TrimSpace(a.Sha256.String)
	st, err := confermabom.SorgenteValida(ctx, q, thread, r.ProdottoID, sha)
	if err != nil {
		return out, err
	}
	if !a.Sha256.Valid || !st.Valida {
		return out, errCorpo("allegato", MotivoNessunaSorgente, "lo STEP non e' una sorgente confermata da S8 e valida per %s: non c'e' niente da revocare", p.Codice)
	}
	out.DocumentoID = st.Documento
	if r.Motivo == "" {
		sigla, err := traccia.Sigla(ctx, q, utente)
		if err != nil {
			return out, err
		}
		r.traccia = TracciaRevoca(sigla, a.NomeFile, p.Codice)
	}
	out.Motivo = r.motivoRegistrato()
	ins := db.InsertFonteStepEventoParams{ThreadID: thread, ProdottoID: r.ProdottoID, Ruolo: "prodotto", Azione: AzioneRevocaSorgente, AllegatoID: allegato,
		Sha256: sha, DocumentoID: uuid.NullUUID{UUID: st.Documento, Valid: true}, Motivo: pgtype.Text{String: out.Motivo, Valid: true},
		ChiaveIdempotenza: uuid.NullUUID{UUID: r.ChiaveIdempotenza, Valid: true}, AutoreID: utente}
	if st.Gruppo != "" {
		ins.Gruppo = pgtype.Text{String: st.Gruppo, Valid: true}
	}
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: r.ProdottoID})
	aperta := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	// la bozza dichiarata dalla pagina deve essere ancora quella (niente scritto altrimenti)
	if r.Bozza != nil && (!aperta || b.BozzaID != r.Bozza.BozzaID || int(b.Versione) != r.Bozza.VersioneAttesa) {
		e := &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Motivo: distintabozza.MotivoBozzaChiusa, Campo: "bozza",
			Messaggio: "la bozza dichiarata non e' piu' aperta: rileggi la pagina"}
		if aperta {
			e.Motivo, e.Messaggio = distintabozza.MotivoVersioneDiversa, fmt.Sprintf("la bozza aperta del prodotto e' alla versione %d: rileggila", b.Versione)
			e.Conflitto = &distintabozza.Conflitto{BozzaID: b.BozzaID, Stato: b.Stato, Versione: int(b.Versione), AggiornataDa: b.AggiornataDa, AggiornataIl: b.AggiornataIl}
		}
		return out, e
	}
	if aperta && b.FonteSha256.Valid && strings.TrimSpace(b.FonteSha256.String) == sha {
		id := b.BozzaID
		out.BozzaID = &id
		ins.BozzaID = uuid.NullUUID{UUID: id, Valid: true}
	}
	if _, err := q.InsertFonteStepEvento(ctx, ins); err != nil {
		return out, err
	}
	// prima di R5 la bozza nata da quella sorgente si annulla; dopo, il cambio di sorgente e' una revisione (commento di testa)
	if out.UsataDaConferma, err = usataDaConferma(ctx, q, thread, r.ProdottoID, sha, st.Documento); err != nil {
		return out, err
	}
	// i testi parlano all'ingegnere, senza i nomi delle rotte né «working» (messaggio 46 dell'utente, difetto D46 [T])
	out.Testo = "Sorgente revocata per " + p.Codice + ". Per confermare la BOM serve di nuovo uno STEP confermato come sorgente."
	switch {
	case out.UsataDaConferma:
		out.Testo = "Sorgente revocata per " + p.Codice + ". Lo STEP è nella BOM confermata: la BOM e la bozza restano come sono. " +
			"Il cambio di sorgente è una revisione interna: conferma lo STEP giusto come sorgente, poi rivedi e conferma la BOM."
	case out.BozzaID != nil:
		if out.BozzaAbbandonata, err = abbandonaBozzaDellaSorgente(ctx, q, thread, r.ProdottoID, utente, sha, "sorgente revocata: "+out.Motivo); err != nil {
			return out, err
		}
		if out.BozzaAbbandonata != nil {
			out.Testo = "Sorgente revocata per " + p.Codice + ". La bozza nata da questo STEP è annullata (resta nella storia). " +
				"Conferma lo STEP giusto come sorgente: la bozza nuova nasce da lì."
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if _, err := q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: r.ChiaveIdempotenza, ThreadID: thread, Gesto: GestoRevocaSorgente,
		UtenteID: utente, ImprontaCorpo: impronta, Esito: raw}); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
