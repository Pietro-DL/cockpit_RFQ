// Package ingest scrive il FATTO (messaggio, messaggio_outlook, allegato), esegue l'aggancio automatico
// deterministico e produce le prime INTERPRETAZIONI (proposta_triage, riferimento_portale).
// Non scrive mai sul NAS e non prende decisioni: ogni proposta è revocabile dall'operatore.
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
)

type Servizio struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// Ingerisci elabora un lotto: una transazione per messaggio, così un elemento anomalo non blocca gli altri.
func (s *Servizio) Ingerisci(ctx context.Context, msgs []api.MessaggioIn) (api.IngestRisposta, error) {
	var out api.IngestRisposta
	for i := range msgs {
		e, err := s.uno(ctx, &msgs[i])
		if err != nil {
			s.Log.Error("ingest messaggio", "message_id", msgs[i].MessageID, "err", err)
			return out, fmt.Errorf("messaggio %s: %w", msgs[i].MessageID, err)
		}
		if e.Inserito {
			out.Inseriti++
		} else {
			out.Aggiornati++
		}
		out.Esiti = append(out.Esiti, e)
	}
	return out, nil
}

func txt(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func txtN(s string, max int) pgtype.Text {
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return txt(s)
}

func (s *Servizio) uno(ctx context.Context, m *api.MessaggioIn) (api.EsitoMessaggio, error) {
	esito := api.EsitoMessaggio{MessageID: m.MessageID, Aggancio: "nessuno"}
	if m.MessageID == "" {
		return esito, errors.New("message_id vuoto")
	}
	if m.Direzione != "entrata" && m.Direzione != "uscita" {
		return esito, fmt.Errorf("direzione non valida: %q", m.Direzione)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return esito, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	chiaveConv := m.ConversationID
	if chiaveConv == "" {
		chiaveConv = "msg:" + m.MessageID
	}
	conv, err := q.UpsertConversazione(ctx, db.UpsertConversazioneParams{Canale: db.CanaleOutlook, ChiaveEsterna: txtN(chiaveConv, 255).String, PrimoMessaggioIl: m.DataEvento})
	if err != nil {
		return esito, fmt.Errorf("conversazione: %w", err)
	}

	// anagrafica: buyer per indirizzo, cliente per dominio (solo in entrata)
	var buyer *db.Buyer
	var clienteID uuid.NullUUID
	indirizzo := strings.ToLower(strings.TrimSpace(m.MittenteIndirizzo))
	if m.Direzione == "entrata" && indirizzo != "" {
		if b, err := q.GetBuyerPerEmail(ctx, indirizzo); err == nil {
			buyer = &b
			clienteID = uuid.NullUUID{UUID: b.ClienteID, Valid: true}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return esito, err
		}
		if !clienteID.Valid {
			if i := strings.LastIndex(indirizzo, "@"); i > 0 {
				if c, err := q.GetClientePerDominio(ctx, indirizzo[i+1:]); err == nil {
					clienteID = uuid.NullUUID{UUID: c.ClienteID, Valid: true}
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return esito, err
				}
			}
		}
	}

	var parent uuid.NullUUID
	if m.ParentMessageID != "" {
		if p, err := q.GetMessaggioPerChiave(ctx, db.GetMessaggioPerChiaveParams{Canale: db.CanaleOutlook, ChiaveEsterna: m.ParentMessageID}); err == nil {
			parent = uuid.NullUUID{UUID: p.MessaggioID, Valid: true}
		}
	}
	dest, _ := json.Marshal(m.Destinatari)
	if m.Destinatari == nil {
		dest = []byte("[]")
	}
	var buyerID uuid.NullUUID
	if buyer != nil {
		buyerID = uuid.NullUUID{UUID: buyer.BuyerID, Valid: true}
	}
	var imp pgtype.Int2
	if m.Importanza >= 0 && m.Importanza <= 2 {
		imp = pgtype.Int2{Int16: int16(m.Importanza), Valid: true}
	}
	row, err := q.UpsertMessaggio(ctx, db.UpsertMessaggioParams{
		Canale: db.CanaleOutlook, ChiaveEsterna: txtN(m.MessageID, 255).String, ConversazioneID: conv.ConversazioneID,
		ParentMessaggioID: parent, Direzione: db.Direzione(m.Direzione), DataEvento: m.DataEvento,
		MittenteNome: txtN(m.MittenteNome, 150), MittenteIndirizzo: txtN(indirizzo, 200), BuyerID: buyerID,
		Destinatari: dest, Oggetto: txtN(m.Oggetto, 500), CorpoTesto: txt(m.CorpoTesto), CorpoHtml: txt(m.CorpoHTML),
		Importanza: imp,
	})
	if err != nil {
		return esito, fmt.Errorf("messaggio: %w", err)
	}
	esito.MessaggioID = row.MessaggioID
	esito.Inserito = row.Inserito

	var flag pgtype.Int2
	if m.FlagStato > 0 {
		flag = pgtype.Int2{Int16: int16(m.FlagStato), Valid: true}
	}
	if err := q.UpsertMessaggioOutlook(ctx, db.UpsertMessaggioOutlookParams{
		MessaggioID: row.MessaggioID, EntryID: m.EntryID, StoreID: m.StoreID, ConversationID: txtN(m.ConversationID, 255),
		ConversationIndex: txtN(m.ConversationIndex, 600), InReplyTo: txtN(m.InReplyTo, 255), Riferimenti: m.Riferimenti,
		Cartella: txtN(m.Cartella, 200), Categorie: m.Categorie, NonLetto: m.NonLetto, FlagStato: flag,
	}); err != nil {
		return esito, fmt.Errorf("messaggio_outlook: %w", err)
	}

	// FATTO: allegati. Nessun download automatico: sul disco vanno solo i file che l'operatore chiede
	// (SPEC: staging su richiesta). Qui si registra l'allegato e la prima proposta dal solo nome file.
	var nomiAllegati []string
	for _, a := range m.Allegati {
		nat := db.NaturaAllegatoFile
		switch a.Natura {
		case "inline":
			nat = db.NaturaAllegatoInline
		case "elemento_outlook":
			nat = db.NaturaAllegatoElementoOutlook
		case "collegamento":
			nat = db.NaturaAllegatoCollegamento
		}
		ext := strings.ToLower(strings.TrimPrefix(a.Estensione, "."))
		if ext == "" {
			if i := strings.LastIndex(a.NomeFile, "."); i >= 0 {
				ext = strings.ToLower(a.NomeFile[i+1:])
			}
		}
		var by pgtype.Int8
		if a.Bytes > 0 {
			by = pgtype.Int8{Int64: a.Bytes, Valid: true}
		}
		al, err := q.UpsertAllegato(ctx, db.UpsertAllegatoParams{
			MessaggioID: row.MessaggioID, Indice: int16(a.Indice), NomeFile: txtN(a.NomeFile, 300).String,
			Estensione: txtN(ext, 10), ContentType: txtN(a.ContentType, 120), Natura: nat, Origine: db.OrigineAllegatoOutlook,
			Bytes: by, RicevutoIl: m.DataEvento,
		})
		if err != nil {
			return esito, fmt.Errorf("allegato %d: %w", a.Indice, err)
		}
		if nat == db.NaturaAllegatoFile || nat == db.NaturaAllegatoElementoOutlook {
			nomiAllegati = append(nomiAllegati, a.NomeFile)
			pr := domain.PropostaDaNome(a.NomeFile, a.Bytes, m.Direzione)
			dett, _ := json.Marshal(map[string]any{"estensione": ext, "bytes": a.Bytes, "pre_spunta": pr.PreSpunta})
			if err := q.InsertPropostaSeAssente(ctx, db.InsertPropostaSeAssenteParams{
				AllegatoID: al.AllegatoID, ThreadID: row.ThreadID, TipoProposto: db.TipoDocumento(pr.Tipo), Codice: txtN(pr.Codice, 60),
				Rev: txtN(pr.Rev, 10), Confidenza: int16(pr.Confidenza), Fonte: db.FonteProposta(pr.Fonte), Dettagli: dett,
			}); err != nil {
				return esito, fmt.Errorf("proposta allegato %d: %w", a.Indice, err)
			}
		}
	}

	// aggancio automatico, solo per messaggi nuovi ancora orfani
	codici := domain.EstraiCodici(append([]string{m.Oggetto, m.CorpoTesto}, senzaEstensione(nomiAllegati)...)...)
	threadID := row.ThreadID
	if row.Inserito && !row.ThreadID.Valid {
		if tid, ok, err := s.threadPerConversazione(ctx, q, conv); err != nil {
			return esito, err
		} else if ok {
			threadID = uuid.NullUUID{UUID: tid, Valid: true}
			esito.Aggancio = string(db.AggancioAutoConversazione)
		} else if clienteID.Valid {
			for _, c := range codici {
				tid, err := q.ThreadPerCodiceCliente(ctx, db.ThreadPerCodiceClienteParams{ClienteID: clienteID.UUID, Upper: c})
				if err == nil {
					threadID = uuid.NullUUID{UUID: tid, Valid: true}
					esito.Aggancio = string(db.AggancioAutoIdentificativo)
					break
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return esito, err
				}
			}
		}
		if threadID.Valid {
			if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{MessaggioID: row.MessaggioID, ThreadID: threadID, Aggancio: db.Aggancio(esito.Aggancio)}); err != nil {
				return esito, err
			}
			if !conv.ThreadID.Valid {
				_ = q.CollegaConversazione(ctx, db.CollegaConversazioneParams{ConversazioneID: conv.ConversazioneID, ThreadID: threadID, CollegataDa: db.Aggancio(esito.Aggancio)})
			}
		}
	} else if row.ThreadID.Valid {
		esito.Aggancio = string(row.Aggancio)
	}
	if threadID.Valid {
		t := threadID.UUID
		esito.ThreadID = &t
	}

	// INTERPRETAZIONE: riferimenti portale e triage deterministico (solo alla prima vista del messaggio)
	if row.Inserito {
		for _, r := range domain.RilevaPortale(m.CorpoTesto) {
			cod := r.Codici
			if len(cod) == 0 {
				cod = []string{""}
			}
			for _, c := range cod {
				if _, err := q.UpsertRiferimentoPortale(ctx, db.UpsertRiferimentoPortaleParams{
					MessaggioID: row.MessaggioID, ThreadID: threadID, Codice: txtN(c, 60), Url: txtN(r.URL, 500), TestoCitato: txtN(r.TestoCitato, 1000).String,
				}); err != nil {
					return esito, fmt.Errorf("riferimento_portale: %w", err)
				}
			}
		}
		if !threadID.Valid && m.Direzione == "entrata" {
			tr := domain.Triage(domain.IngressoTriage{
				Oggetto: m.Oggetto, Corpo: m.CorpoTesto, NomiAllegati: nomiAllegati, Direzione: m.Direzione,
				ClienteNoto: clienteID.Valid, BuyerNoto: buyer != nil, ConversazioneNota: conv.ThreadID.Valid,
			})
			motivi, _ := json.Marshal(tr.Motivi)
			if tr.Codici == nil {
				tr.Codici = []string{}
			}
			var scad *time.Time
			if d, ok := domain.RilevaScadenza(m.CorpoTesto, m.DataEvento); ok {
				scad = &d
			}
			if _, err := q.UpsertTriage(ctx, db.UpsertTriageParams{
				MessaggioID: row.MessaggioID, Esito: db.EsitoTriage(tr.Esito), ClienteProposto: clienteID, BuyerProposto: buyerID,
				Identificativi: tr.Codici, ScadenzaProposta: scad, Confidenza: int16(tr.Confidenza), Motivi: motivi, Fonte: db.FonteTriageDeterministico,
			}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return esito, fmt.Errorf("triage: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return esito, err
	}
	return esito, nil
}

func (s *Servizio) threadPerConversazione(ctx context.Context, q *db.Queries, conv db.Conversazione) (uuid.UUID, bool, error) {
	if conv.ThreadID.Valid {
		return conv.ThreadID.UUID, true, nil
	}
	tid, err := q.ThreadDellaConversazione(ctx, conv.ConversazioneID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return tid.UUID, tid.Valid, nil
}

func senzaEstensione(nomi []string) []string {
	out := make([]string, 0, len(nomi))
	for _, n := range nomi {
		if i := strings.LastIndex(n, "."); i > 0 {
			n = n[:i]
		}
		out = append(out, n)
	}
	return out
}
