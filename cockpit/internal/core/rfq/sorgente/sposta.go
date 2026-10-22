package sorgente

// sposta.go: slegare uno STEP dal prodotto sbagliato e riassociarlo a quello giusto (G39-03 [U], 20/10: «dobbiamo impedire
// che l'utente confermi uno step strutturale per due prodotti diversi, ma dobbiamo dare l'occasione di correggere eventuali
// sviste [...] bisogna dare la possibilita' all'utente di slegare lo step e riassociarlo al prodotto giusto»; RV3-32 [U]).
// La forma e' [T-39] della corsia persistenza (parte B del giro G39), sullo schema della variante 0023-v1-formato3-file-20261019d,
// senza DDL.
//
// Il caso (g) di S8 (lo stesso contenuto e' gia' un documento di un altro prodotto finito) non e' piu' sempre un rifiuto. Il
// passo 0 (Controlla) guarda l'altro prodotto ‹A›:
//   - lo STEP e' gia' entrato in una conferma della BOM di ‹A› (componente.step_strutturale_id, una conferma strutturale con
//     quella fonte, una baseline congelata): rifiuto senza scritture, STEP_SORGENTE_DI_ALTRO_PRODOTTO motivo bom_confermata,
//     con il rimedio «apri la revisione di ‹A›» (dopo R5 il cambio di sorgente e' una revisione, mai uno spostamento diretto);
//   - lo STEP e' la sorgente S8 valida di ‹A›, non usata da una R5: serve il gesto «Sposta la sorgente su questo prodotto»
//     (sposta_da = ‹A› nel corpo di S8); senza, STEP_SORGENTE_DI_ALTRO_PRODOTTO motivo sorgente_spostabile con altro_prodotto;
//   - lo STEP e' solo un documento di ‹A› (legacy, legame storico, o sorgente S8 gia' revocata), senza S8 valida: si
//     riassocia con il gesto esplicito «Riassocia a ‹B›» (sposta_da = ‹A› nel corpo; RV3-32); senza, 422
//     STEP_SORGENTE_DI_ALTRO_PRODOTTO motivo riassociazione con altro_prodotto. Dal messaggio 46 [U] il motivo scritto
//     dall'utente non serve piu': il gesto e' sposta_da, e la traccia del server lo dice (TracciaS8, «lo STEP lascia ‹A›»).
//     Un client di prima che manda il motivo senza sposta_da riassocia ancora, come prima;
//   - lo STEP non e' un documento ma ha la copia P1 (documento_proposta.componente_id di una proposta aperta, senza AR) su un
//     altro componente: come il documento, si riassocia con «Riassocia a ‹B›» (prima S8 lasciava la copia dov'era).
// Restano rifiuti prudenti (GIA_DOCUMENTO): il documento che e' la revisione di un altro documento di ‹A› (la catena
// sostituito_da resta del suo componente), lo STEP autorizzato dallo Smistamento a proporre i figli di ‹A› e il documento
// gia' sostituito nella storia di ‹A› (motivo sostituito_in_altro_prodotto, s8.go): dopo la revisione di ‹A› lo STEP
// sbagliato resta nella sua storia (documento e' UNIQUE (thread_id, sha256)) e non arriva al prodotto giusto senza un cambio
// di schema (aperto, domanda all'utente).
//
// Lo spostamento si scrive nel passo 2 di S8, nella sua transazione e sotto il lucchetto della RFQ, con la stessa chiave k2
// (atomico con la riga conferma_documento di S8 su ‹B›: o tutto o niente; il reinvio trova k2 e dice «gia' fatto»):
//  1. la revoca motivata su ‹A› (fonte_step_evento revoca_sorgente, causa sorgente_errata, documento, chiave k2s): chiude la
//     riga conferma_documento ancora aperta per quel contenuto (anche legacy) e, se non ce n'e', resta comunque nella storia
//     della fonte di ‹A› come traccia che il documento e' passato a un altro prodotto (storia conservata);
//  2. la bozza aperta di ‹A› nata da quella sorgente (fonte_sha256) si annulla: abbandonata con lo stato e il registro che ci
//     sono gia', storia conservata, nessuna cancellazione;
//  3. il documento passa a ‹B›: componente e codice (la FK a tre campi lo vuole uguale a quello del prodotto); il percorso sul
//     NAS si ricalcola solo se il file non e' ancora sul NAS (in coda o in errore, senza una copia in corso: la regola di
//     «Assegna» del Fascicolo). Se e' gia' scritto resta dov'e' (percorso_nas: invariato): lo spostamento sul NAS e'
//     sposta_nas di B8.8, che non c'e' ancora [T-39, aperto];
//  4. la copia P1 si toglie dal servizio unico (associazioni.Scollega: evento «sgancia» senza AR, origine ponte, con il
//     motivo nella nota): nessuna AR si scrive fuori da R5.
// Poi il passo 2 scrive la riga di S8 su ‹B› (con il motivo scritto, senza la causa: per ‹B› non e' una correzione della sua
// BOM, confermabom.StatoSorgente.Correzione) e il passo 3 apre la bozza di ‹B›. Niente
// tocca step_strutturale_id, sostituito_da, la working o le AR (lo fa solo R5).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/confermadoc"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// I casi dello spostamento (EsitoSpostamento.Caso).
const (
	CasoSorgenteS8 = "sorgente_s8"
	CasoDocumento  = "documento"
	CasoCopiaP1    = "copia_p1"
)

// I motivi propri dello spostamento.
const (
	// MotivoBomConfermata: lo STEP e' gia' nella BOM confermata dell'altro prodotto: si apre la revisione di quello.
	MotivoBomConfermata = "bom_confermata"
	// MotivoSorgenteSpostabile: lo STEP e' la sorgente S8 valida dell'altro prodotto, non usata da una R5: il gesto e'
	// «Sposta la sorgente su questo prodotto» (sposta_da).
	MotivoSorgenteSpostabile = "sorgente_spostabile"
	// MotivoRiassociazione: lo STEP e' solo un documento (o una copia P1) di un altro componente: si riassocia con il motivo
	// scritto (anche con sposta_da: la causa da sola non dice perche').
	MotivoRiassociazione = "riassociazione"
	// MotivoSostituitoInAltroProdotto: lo STEP e' un documento sostituito nella storia della BOM di un altro prodotto (dopo la
	// sua revisione): in V1 non diventa la sorgente di questo, senza un cambio di schema (aperto, G39-03 dopo R5).
	MotivoSostituitoInAltroProdotto = "sostituito_in_altro_prodotto"
)

// Gli esiti del percorso sul NAS del documento spostato.
const (
	PercorsoAggiornato = "aggiornato"
	PercorsoInvariato  = "invariato"
)

// Spostamento: il piano del passo 0 quando lo STEP e' di un altro componente (commento di testa).
type Spostamento struct {
	Caso      string
	Da        db.Componente
	Documento *db.Documento
	Proposta  *db.DocumentoProposta
	// RigaDaRevocare: l'altro prodotto ha una riga conferma_documento non revocata per quel contenuto (la revoca la chiude;
	// senza, la revoca resta come traccia)
	RigaDaRevocare bool
	Gruppo         string
}

// EsitoSpostamento: che cosa il passo 2 ha fatto sull'altro prodotto (nella risposta di S8, campo spostamento).
type EsitoSpostamento struct {
	Caso             string     `json:"caso"`
	DaProdottoID     uuid.UUID  `json:"da_prodotto_id"`
	DaCodice         string     `json:"da_codice"`
	DocumentoID      *uuid.UUID `json:"documento_id,omitempty"`
	RevocaEvento     *int64     `json:"revoca_evento,omitempty"`
	BozzaAbbandonata *uuid.UUID `json:"bozza_abbandonata,omitempty"`
	CopiaTolta       bool       `json:"copia_tolta,omitempty"`
	PercorsoNas      string     `json:"percorso_nas,omitempty"`
	Motivo           string     `json:"motivo"`
	Testo            string     `json:"testo"`
}

// usataDaConferma: confermabom.UsataDaConferma (la regola sta li', con la correzione della sorgente che la usa).
func usataDaConferma(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha string, documento uuid.UUID) (bool, error) {
	return confermabom.UsataDaConferma(ctx, q, thread, prodotto, sha, documento)
}

// rigaFonteAperta: il prodotto ha una riga conferma_documento (di S8 o delle pagine di prima) per quel contenuto, non
// revocata ne' rifiutata dopo; con il gruppo dell'ultima.
func rigaFonteAperta(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha string) (bool, string, error) {
	ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return false, "", err
	}
	aperta, gruppo := false, ""
	for _, e := range ee {
		if e.Ruolo != "prodotto" || strings.TrimSpace(e.Sha256) != strings.TrimSpace(sha) {
			continue
		}
		switch e.Azione {
		case "conferma_documento":
			aperta, gruppo = true, e.Gruppo.String
		case "revoca", AzioneRevocaSorgente, "rifiuto":
			aperta = false
		}
	}
	return aperta, gruppo, nil
}

// valutaAltroProdotto: il caso (g) con il documento d dell'altro finito a (commento di testa). Errore = rifiuto senza
// scritture; altrimenti il piano.
func valutaAltroProdotto(ctx context.Context, q *db.Queries, thread uuid.UUID, r RichiestaS8, p db.GetProdottoDellaRfqRow, d db.Documento,
	a db.Componente, sha string) (*Spostamento, error) {
	altro := a.ComponenteID
	if r.SpostaDa != nil && *r.SpostaDa != altro {
		return nil, errCorpo("sposta_da", "", "lo STEP e' un documento di %s, non del prodotto indicato in sposta_da: rileggi la pagina", a.Codice)
	}
	usata, err := usataDaConferma(ctx, q, thread, altro, sha, d.DocumentoID)
	if err != nil {
		return nil, err
	}
	if usata {
		return nil, &distintabozza.Errore{Codice: CodiceStepSorgenteDiAltroProdotto, Motivo: MotivoBomConfermata, Campo: "allegato", AltroProdotto: &altro,
			Messaggio: fmt.Sprintf("Questo STEP è nella BOM confermata di %s: non si sposta su %s. Apri la revisione di %s e conferma la sua sorgente giusta, se lo STEP è sbagliato "+
				"per %s (una correzione della sorgente). In V1, dopo quella revisione, questo file resta nella storia di %s e non può diventare la sorgente di %s. Niente è cambiato.",
				a.Codice, p.Codice, a.Codice, a.Codice, a.Codice, p.Codice)}
	}
	dd, err := q.ListDocumentiThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	for _, x := range dd {
		if x.SostituitoDa.Valid && x.SostituitoDa.UUID == d.DocumentoID {
			return nil, errCodice(CodiceGiaDocumento, "allegato", "questo STEP è la revisione di un altro documento di %s (%s): la catena delle revisioni resta del suo componente", a.Codice, x.NomeFile)
		}
	}
	dich, err := fascicolo.LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	if x, ok := dich.DelDocumento(d.DocumentoID); ok && x.Origine == fascicolo.OrigineSmistamento {
		return nil, errCodice(CodiceGiaDocumento, "allegato", "questo STEP è autorizzato a proporre i figli diretti di %s: prima si revoca l'autorizzazione", x.Componente.Codice)
	}
	st, err := confermabom.SorgenteValida(ctx, q, thread, altro, sha)
	if err != nil {
		return nil, err
	}
	dc := d
	sp := &Spostamento{Caso: CasoDocumento, Da: a, Documento: &dc}
	if sp.RigaDaRevocare, sp.Gruppo, err = rigaFonteAperta(ctx, q, thread, altro, sha); err != nil {
		return nil, err
	}
	if st.Valida {
		sp.Caso = CasoSorgenteS8
		if r.SpostaDa == nil {
			return nil, &distintabozza.Errore{Codice: CodiceStepSorgenteDiAltroProdotto, Motivo: MotivoSorgenteSpostabile, Campo: "allegato", AltroProdotto: &altro,
				Messaggio: fmt.Sprintf("Questo STEP è la sorgente confermata di %s (la BOM non è ancora confermata). Se l'associazione è sbagliata, «Sposta la sorgente su %s»: "+
					"la sorgente di %s si revoca con il motivo «sorgente errata» e la sua bozza si annulla. In V1 uno STEP è la sorgente di un solo prodotto.", a.Codice, p.Codice, a.Codice)}
		}
		return sp, nil
	}
	if !riassociazioneEsplicita(r, altro) {
		return nil, &distintabozza.Errore{Codice: CodiceStepSorgenteDiAltroProdotto, Motivo: MotivoRiassociazione, Campo: "sposta_da", AltroProdotto: &altro,
			Messaggio: fmt.Sprintf("Questo STEP è un documento di %s, non confermato come sua sorgente: per riassociarlo a %s usa «Riassocia a %s». Lascerà %s.", a.Codice, p.Codice, p.Codice, a.Codice)}
	}
	return sp, nil
}

// valutaCopiaP1: lo STEP senza documento con la copia P1 su un altro componente (commento di testa); nil se non c'e'.
func valutaCopiaP1(ctx context.Context, q *db.Queries, allegato uuid.UUID, r RichiestaS8, p db.GetProdottoDellaRfqRow) (*Spostamento, error) {
	pr, err := q.GetPropostaDocumentoDiAllegato(ctx, allegato)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pr.Stato != db.StatoPropostaAperta || !pr.ComponenteID.Valid || pr.ComponenteID.UUID == r.ProdottoID {
		return nil, nil
	}
	a, err := q.GetComponente(ctx, pr.ComponenteID.UUID)
	if err != nil {
		return nil, err
	}
	altro := a.ComponenteID
	if r.SpostaDa != nil && *r.SpostaDa != altro {
		return nil, errCorpo("sposta_da", "", "lo STEP e' pre-assegnato a %s, non al prodotto indicato in sposta_da: rileggi la pagina", a.Codice)
	}
	if !riassociazioneEsplicita(r, altro) {
		return nil, &distintabozza.Errore{Codice: CodiceStepSorgenteDiAltroProdotto, Motivo: MotivoRiassociazione, Campo: "sposta_da", AltroProdotto: &altro,
			Messaggio: fmt.Sprintf("Questo STEP è pre-assegnato a %s dalle pagine di prima: per riassociarlo a %s usa «Riassocia a %s». Lascerà %s.", a.Codice, p.Codice, p.Codice, a.Codice)}
	}
	pc := pr
	return &Spostamento{Caso: CasoCopiaP1, Da: a, Proposta: &pc}, nil
}

// riassociazioneEsplicita: il corpo chiede la riassociazione dello STEP dall'altro prodotto: con il gesto «Riassocia a
// ‹B›» (sposta_da = l'altro prodotto; messaggio 46 [U]: il gesto, non un testo, e' la decisione) oppure, per i client di
// prima, con un motivo scritto.
func riassociazioneEsplicita(r RichiestaS8, altro uuid.UUID) bool {
	return (r.SpostaDa != nil && *r.SpostaDa == altro) || r.Motivo != ""
}

// notaSpostamento: il motivo registrato sull'altro prodotto (revoca, abbandono della bozza, nota dello sgancio della copia).
func notaSpostamento(r RichiestaS8, codice string) string {
	t := "lo STEP passa a " + codice + " (S8)"
	if r.Motivo != "" {
		t += ": " + r.Motivo
	}
	return confermabom.MotivoConCausa(confermabom.CausaSorgenteErrata, t)
}

// eseguiSpostamento: i passi 1-4 del commento di testa, nella transazione del passo 2 (lucchetto della RFQ gia' preso,
// Controlla appena rifatto). k: la chiave dello spostamento (derivata dalla chiave d'intento).
func eseguiSpostamento(ctx context.Context, q *db.Queries, thread, allegato, utente uuid.UUID, r RichiestaS8, c *Contesto, k uuid.UUID) (*EsitoSpostamento, error) {
	sp := c.Spostamento
	nota := notaSpostamento(r, c.Prodotto.Codice)
	out := &EsitoSpostamento{Caso: sp.Caso, DaProdottoID: sp.Da.ComponenteID, DaCodice: sp.Da.Codice, Motivo: nota}
	var parti []string
	// 1. la revoca motivata sull'altro prodotto
	if sp.Documento != nil {
		ins := db.InsertFonteStepEventoParams{ThreadID: thread, ProdottoID: sp.Da.ComponenteID, Ruolo: "prodotto", Azione: AzioneRevocaSorgente,
			AllegatoID: allegato, Sha256: c.Sha256, DocumentoID: uuid.NullUUID{UUID: sp.Documento.DocumentoID, Valid: true},
			Motivo: pgtype.Text{String: nota, Valid: true}, ChiaveIdempotenza: uuid.NullUUID{UUID: k, Valid: true}, AutoreID: utente}
		if sp.Gruppo != "" {
			ins.Gruppo = pgtype.Text{String: sp.Gruppo, Valid: true}
		}
		if b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: sp.Da.ComponenteID}); err == nil {
			if b.FonteSha256.Valid && strings.TrimSpace(b.FonteSha256.String) == c.Sha256 {
				ins.BozzaID = uuid.NullUUID{UUID: b.BozzaID, Valid: true}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		ev, err := q.InsertFonteStepEvento(ctx, ins)
		if err != nil {
			return nil, err
		}
		id := ev.EventoID
		out.RevocaEvento = &id
		if sp.RigaDaRevocare {
			parti = append(parti, "la sorgente di "+sp.Da.Codice+" è revocata")
		} else {
			parti = append(parti, "il legame con "+sp.Da.Codice+" è chiuso nella storia della fonte")
		}
	}
	// 2. la bozza aperta dell'altro prodotto nata da quella sorgente si annulla
	if id, err := abbandonaBozzaDellaSorgente(ctx, q, thread, sp.Da.ComponenteID, utente, c.Sha256, "sorgente revocata: "+nota); err != nil {
		return nil, err
	} else if id != nil {
		out.BozzaAbbandonata = id
		parti = append(parti, "la bozza di "+sp.Da.Codice+" nata da quello STEP è annullata (la sua storia resta)")
	}
	// 3. il documento passa al prodotto
	if sp.Documento != nil {
		percorso, err := documentoAlProdotto(ctx, q, thread, *sp.Documento, c.Prodotto)
		if err != nil {
			return nil, err
		}
		id := sp.Documento.DocumentoID
		out.DocumentoID, out.PercorsoNas = &id, percorso
		parti = append(parti, "il documento passa a "+c.Prodotto.Codice)
		if percorso == PercorsoInvariato {
			parti = append(parti, "il file resta sul NAS dov'è (lo spostamento sul NAS non è ancora disponibile)")
		}
	}
	// 4. la copia P1 si toglie dal servizio unico, senza AR
	if sp.Proposta != nil {
		es, err := associazioni.Scollega(ctx, q, associazioni.Atto{Thread: thread, Autore: utente, Chiave: k, Nota: nota},
			associazioni.Scollegamento{Allegato: allegato, Componente: sp.Da.ComponenteID, Motivo: associazioni.MotivoScollegaFile})
		if err != nil {
			var rf *associazioni.Rifiuto
			if errors.As(err, &rf) {
				return nil, errCorpo("allegato", MotivoRiassociazione, "la pre-assegnazione a %s non si toglie: %s", sp.Da.Codice, rf.Messaggio)
			}
			return nil, err
		}
		out.CopiaTolta = es.Esito == associazioni.EsitoCopiaTolta
		if out.CopiaTolta {
			parti = append(parti, "la pre-assegnazione a "+sp.Da.Codice+" è tolta")
		}
	}
	out.Testo = "Spostamento: " + strings.Join(vuoto(parti), "; ") + "."
	return out, nil
}

// abbandonaBozzaDellaSorgente: la bozza aperta del prodotto la cui sorgente e' quel contenuto si annulla (abbandonata con
// lo stato e il registro esistenti, nella transazione di chi chiama, che ha gia' il lucchetto della RFQ). nil se non c'e'.
func abbandonaBozzaDellaSorgente(ctx context.Context, q *db.Queries, thread, prodotto, utente uuid.UUID, sha, motivo string) (*uuid.UUID, error) {
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !b.FonteSha256.Valid || strings.TrimSpace(b.FonteSha256.String) != strings.TrimSpace(sha) {
		return nil, nil
	}
	bb, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: b.BozzaID, ThreadID: thread})
	if err != nil {
		return nil, err
	}
	if bb.Stato != distintabozza.StatoAperta {
		return nil, nil
	}
	m := []rune(strings.TrimSpace(motivo))
	if len(m) > distintabozza.MaxMotivo {
		m = m[:distintabozza.MaxMotivo]
	}
	n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: utente, Valid: true},
		Motivo: pgtype.Text{String: string(m), Valid: len(m) > 0}, BozzaID: bb.BozzaID, ThreadID: thread, Versione: bb.Versione})
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, fmt.Errorf("la bozza %s non si e' abbandonata (versione cambiata sotto il lucchetto)", bb.BozzaID)
	}
	id := bb.BozzaID
	return &id, nil
}

// documentoAlProdotto: il documento passa al prodotto, con il codice del prodotto (la FK a tre campi) e, se il file non e'
// ancora sul NAS, il percorso ricalcolato con la regola della conferma (commento di testa, passo 3).
func documentoAlProdotto(ctx context.Context, q *db.Queries, thread uuid.UUID, d db.Documento, p db.GetProdottoDellaRfqRow) (string, error) {
	esito := PercorsoInvariato
	if d.StatoNas == db.StatoNasInCoda || d.StatoNas == db.StatoNasErrore {
		inCorso := false
		if j, err := q.BloccaCopiaPendente(ctx, pgtype.Text{String: coda.ChiaveCopia(d.DocumentoID), Valid: true}); err == nil {
			inCorso = j.Stato == db.StatoJobInCorso
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if !inCorso {
			nuovo, err := percorsoPer(ctx, q, thread, d, p.Codice)
			if err != nil {
				return "", err
			}
			if nuovo == d.PathRelativo {
				esito = PercorsoAggiornato
			} else if n, err := q.SetPathDocumentoInCoda(ctx, db.SetPathDocumentoInCodaParams{DocumentoID: d.DocumentoID, PathRelativo: nuovo}); err != nil {
				return "", err
			} else if n == 1 {
				esito = PercorsoAggiornato
			}
		}
	}
	n, err := q.SetComponenteDocumento(ctx, db.SetComponenteDocumentoParams{DocumentoID: d.DocumentoID,
		ComponenteID: uuid.NullUUID{UUID: p.ComponenteID, Valid: true}, Codice: pgtype.Text{String: p.Codice, Valid: true}})
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", fmt.Errorf("il documento %s non e' passato a %s", d.DocumentoID, p.Codice)
	}
	return esito, nil
}

// percorsoPer: il percorso che la conferma darebbe al documento con il codice dato (documenti.CartellaDocumento,
// NomeSulNas, RiscegliPercorso sotto il lucchetto della cartella: la regola di confermadoc.Conferma; il percorso attuale
// conta come libero).
func percorsoPer(ctx context.Context, q *db.Queries, thread uuid.UUID, d db.Documento, codice string) (string, error) {
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return "", err
	}
	cl, err := q.GetCliente(ctx, t.ClienteID)
	if err != nil {
		return "", err
	}
	l, err := q.GetCartellaDocumento(ctx, d.Tipo)
	if err != nil {
		return "", fmt.Errorf("layout per %s: %w", d.Tipo, err)
	}
	cartella, err := documenti.CartellaDocumento(documenti.LayoutDocumento{Sottocartella: l.Sottocartella, PerCodice: l.PerCodice},
		confermadoc.RegolaBool(cl.Regole, "cartella_per_codice", true), codice)
	if err != nil {
		return "", err
	}
	if err := documenti.BloccaCartella(ctx, q, thread, cartella); err != nil {
		return "", err
	}
	return documenti.RiscegliPercorso(ctx, q, thread, cartella, documenti.NomeSulNas(d.Tipo, codice, d.Rev.String, d.Estensione, d.NomeFile), d.PathRelativo)
}
