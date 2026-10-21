package distintabozza

// R15 (U2 §5.7; [U] C-04, [UR] Q19, vincolo [U] di DEC:125): «Rifiuta la proposta aggiornata…» per un prodotto confermato
// in formato 3 e senza bozza. Il rifiuto ha un nome diverso dallo scarto di un file e dall'abbandono di una bozza; registra
// che l'ingegnere mantiene la BOM confermata per quella proposta (impronta_contenuto del gruppo corrente) e nient'altro:
// nessuna riga legacy, nessun componente, nessuna conferma, nessuna fonte, nessun cambio della verifica.
//
// L'ordine (come R2 nelle forme che importano, EM §6.3): il calcolo del motore prima, in sola lettura, da chi chiama
// (PropostaDiAdesso); poi la transazione: BloccaThread → la chiave d'intento (riproposto, CHIAVE_RIUSATA) → il prodotto
// finito della RFQ → la conferma e' l'ultima valida del prodotto, formato 3 con il suo registro → l'impronta dichiarata e'
// quella di adesso (409 ANTEPRIMA_SUPERATA, proposta_step) e diversa da quella della conferma (422: niente da rifiutare) →
// proposta_aggiornata_rifiuto ed esito_intento (gesto rifiuta_proposta) → COMMIT.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// GestoRifiutaProposta: il gesto di esito_intento di R15 (0023, U2 §3.14 riga 6).
const GestoRifiutaProposta = "rifiuta_proposta"

// I codici e i motivi di R15 (U2 §5.7; i motivi sono [T] della corsia persistenza).
const (
	CodiceAnteprimaSuperata         = "ANTEPRIMA_SUPERATA"
	MotivoPropostaStep              = "proposta_step"
	MotivoConfermaNonCorrente       = "conferma_non_corrente"
	MotivoConfermaNonFormato3       = "conferma_non_formato3"
	MotivoNessunaPropostaAggiornata = "nessuna_proposta_aggiornata"
	MotivoBozzaAperta               = "bozza_aperta"
)

// RichiestaR15: il corpo di R15, letto in modo stretto.
type RichiestaR15 struct {
	ChiaveIdempotenza uuid.UUID `json:"chiave_idempotenza"`
	ProdottoID        uuid.UUID `json:"prodotto_id"`
	ConfermaID        uuid.UUID `json:"conferma_id"`
	Gruppo            string    `json:"gruppo"`
	ImprontaContenuto string    `json:"impronta_contenuto"`
	Motivo            string    `json:"motivo"`
}

// LeggiRichiestaR15 legge il corpo di R15 (DisallowUnknownFields, niente dopo il valore) e ne calcola l'impronta sulla
// forma canonica (la chiave d'intento con un altro corpo e' CHIAVE_RIUSATA).
func LeggiRichiestaR15(raw []byte) (RichiestaR15, string, error) {
	var in struct {
		ChiaveIdempotenza *uuid.UUID `json:"chiave_idempotenza"`
		ProdottoID        *uuid.UUID `json:"prodotto_id"`
		ConfermaID        *uuid.UUID `json:"conferma_id"`
		Gruppo            *string    `json:"gruppo"`
		ImprontaContenuto *string    `json:"impronta_contenuto"`
		Motivo            *string    `json:"motivo"`
	}
	var r RichiestaR15
	if e := valoriSenzaNFC("corpo", raw); e != nil {
		return r, "", e
	}
	if e := decodificaStretta("corpo", raw, &in); e != nil {
		return r, "", e
	}
	switch {
	case in.ChiaveIdempotenza == nil || in.ChiaveIdempotenza.Version() != 4:
		return r, "", corpo("chiave_idempotenza", "la chiave d'intento e' un UUID v4")
	case in.ProdottoID == nil || *in.ProdottoID == uuid.Nil:
		return r, "", corpo("prodotto_id", "manca il prodotto")
	case in.ConfermaID == nil || *in.ConfermaID == uuid.Nil:
		return r, "", corpo("conferma_id", "manca la conferma")
	case in.Gruppo == nil || !testoLibero(*in.Gruppo, MaxGruppo):
		return r, "", corpo("gruppo", "il gruppo e' obbligatorio, al piu' %d caratteri", MaxGruppo)
	case in.ImprontaContenuto == nil || !reSha256.MatchString(*in.ImprontaContenuto):
		return r, "", corpo("impronta_contenuto", "l'impronta del contenuto e' di 64 cifre esadecimali minuscole")
	case in.Motivo == nil || strings.TrimSpace(nfc(*in.Motivo)) == "":
		return r, "", &Errore{Codice: CodiceMotivoObbligatorio, Campo: "motivo", Messaggio: "il rifiuto della proposta aggiornata vuole un motivo"}
	}
	motivo := strings.TrimSpace(nfc(*in.Motivo))
	if !testoLibero(motivo, MaxMotivo) {
		return r, "", corpo("motivo", "il motivo sta in %d caratteri, senza caratteri di controllo", MaxMotivo)
	}
	r = RichiestaR15{ChiaveIdempotenza: *in.ChiaveIdempotenza, ProdottoID: *in.ProdottoID, ConfermaID: *in.ConfermaID, Gruppo: *in.Gruppo,
		ImprontaContenuto: *in.ImprontaContenuto, Motivo: motivo}
	can, err := jsoncanonico.Codifica(r)
	if err != nil {
		return r, "", corpo("corpo", "il corpo non ha una forma canonica: %v", err)
	}
	return r, jsoncanonico.Impronta(can), nil
}

// PropostaDiAdesso: la proposta di adesso del gruppo dichiarato, calcolata da chi chiama prima della transazione (il
// motore in sola lettura). Disponibile falso: il gruppo non c'e' piu' o non si importa (la proposta e' cambiata).
// Differenze: quelle fra il registro della conferma e la proposta di adesso, come la pagina le ha mostrate (le calcola
// chi chiama con l'adattatore, propostabom.Confronta), salvate con il rifiuto.
type PropostaDiAdesso struct {
	Disponibile       bool
	ImprontaContenuto string
	Differenze        json.RawMessage
}

// EsitoR15: la risposta 200 di R15, che esito_intento conserva intera.
type EsitoR15 struct {
	Esito             string    `json:"esito"`
	Riproposto        bool      `json:"riproposto"`
	ChiaveIdempotenza uuid.UUID `json:"chiave_idempotenza"`
	RifiutoID         uuid.UUID `json:"rifiuto_id"`
	ProdottoID        uuid.UUID `json:"prodotto_id"`
	ConfermaID        uuid.UUID `json:"conferma_id"`
	ImprontaContenuto string    `json:"impronta_contenuto"`
	Quando            time.Time `json:"quando"`
	Testo             string    `json:"testo"`
}

// RifiutaPropostaAggiornata e' R15 (commento di testa).
func RifiutaPropostaAggiornata(ctx context.Context, pool Avviatore, thread, utente uuid.UUID, r RichiestaR15, improntaCorpo string,
	adesso PropostaDiAdesso) (EsitoR15, error) {
	var out EsitoR15
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	if gia, err := q.GetEsitoIntentoPerChiave(ctx, r.ChiaveIdempotenza); err == nil {
		if gia.ThreadID != thread || gia.Gesto != GestoRifiutaProposta || strings.TrimSpace(gia.ImprontaCorpo) != improntaCorpo {
			return out, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_idempotenza",
				Messaggio: "questa chiave d'intento e' gia' usata con un altro corpo: rileggi la pagina"}
		}
		if err := json.Unmarshal(gia.Esito, &out); err != nil {
			return out, fmt.Errorf("R15: l'esito salvato non si rilegge: %w", err)
		}
		out.Riproposto = true
		return out, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if _, err := prodottoDellaRfq(ctx, q, thread, r.ProdottoID, false); err != nil {
		return out, err
	}
	if b, err := q.IngressiBozza(ctx, db.IngressiBozzaParams{ThreadID: thread, ProdottoID: r.ProdottoID}); err != nil {
		return out, err
	} else if len(b) > 0 {
		// con una bozza aperta il rifiuto e' BZ-17 con «Mantieni» (U2 §5.7): si salva nella bozza
		return out, corpoMotivo("prodotto_id", MotivoBozzaAperta, "il prodotto ha una bozza aperta: la proposta aggiornata si rifiuta nella bozza («Mantieni»)")
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return out, err
	}
	u, val, ok, err := st.ValiditaUltima(r.ProdottoID)
	if err != nil {
		return out, err
	}
	if !ok || !val.Valida || u.ID != r.ConfermaID {
		return out, &Errore{Codice: CodiceAnteprimaSuperata, Motivo: MotivoConfermaNonCorrente, Campo: "conferma_id",
			Messaggio: "la conferma non e' l'ultima valida del prodotto: rileggi la pagina"}
	}
	c, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: thread, ConfermaID: r.ConfermaID})
	if err != nil {
		return out, err
	}
	if c.Formato != FormatoSnapshot3 || !c.ImportazioneID.Valid {
		return out, corpoMotivo("conferma_id", MotivoConfermaNonFormato3, "la proposta aggiornata si confronta con il registro di una conferma formato 3")
	}
	reg, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: c.ImportazioneID.UUID, ThreadID: thread})
	if err != nil {
		return out, err
	}
	if !adesso.Disponibile || adesso.ImprontaContenuto != r.ImprontaContenuto {
		return out, &Errore{Codice: CodiceAnteprimaSuperata, Motivo: MotivoPropostaStep, Campo: "impronta_contenuto",
			Messaggio: "la proposta di adesso non e' quella che hai visto: rileggi le differenze"}
	}
	if strings.TrimSpace(reg.ImprontaContenuto) == r.ImprontaContenuto {
		return out, corpoMotivo("impronta_contenuto", MotivoNessunaPropostaAggiornata, "la proposta di adesso e' quella della conferma: non c'e' niente da rifiutare")
	}
	diff := adesso.Differenze
	if len(diff) == 0 || string(diff) == "null" {
		diff = json.RawMessage(`{}`)
	}
	riga, err := q.InsertPropostaAggiornataRifiuto(ctx, db.InsertPropostaAggiornataRifiutoParams{ThreadID: thread, ProdottoID: r.ProdottoID,
		ConfermaID: r.ConfermaID, Gruppo: r.Gruppo, ImprontaContenuto: r.ImprontaContenuto, Differenze: diff, Motivo: r.Motivo, AutoreID: utente,
		ChiaveIdempotenza: r.ChiaveIdempotenza})
	if err != nil {
		return out, erroreDiScrittura(err, "corpo")
	}
	out = EsitoR15{Esito: "registrato", ChiaveIdempotenza: r.ChiaveIdempotenza, RifiutoID: riga.RifiutoID, ProdottoID: r.ProdottoID,
		ConfermaID: r.ConfermaID, ImprontaContenuto: r.ImprontaContenuto, Quando: riga.Quando,
		Testo: fmt.Sprintf("Proposta aggiornata rifiutata: la BOM confermata (conferma n. %d) resta com'è. La verifica non cambia.", c.Numero)}
	esito, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if _, err := q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: r.ChiaveIdempotenza, ThreadID: thread, Gesto: GestoRifiutaProposta,
		UtenteID: utente, ImprontaCorpo: improntaCorpo, Esito: esito}); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}
