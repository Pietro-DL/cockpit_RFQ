package filerfq

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// gesto: un gesto con chiave della Sorgente (S2, S3, S4), con il suo intento.
type gesto struct {
	nome     string
	thread   uuid.UUID
	allegato uuid.UUID
	utente   uuid.UUID
	corpo    CorpoGesto
	it       intento
}

func nuovoGesto(nome string, thread, allegato, utente uuid.UUID, c CorpoGesto) (gesto, error) {
	imp, err := improntaCorpo(nome, thread, allegato, c)
	if err != nil {
		return gesto{}, err
	}
	return gesto{nome: nome, thread: thread, allegato: allegato, utente: utente, corpo: c,
		it: intento{chiave: *c.ChiaveIdempotenza, gesto: nome, utente: utente, impronta: imp}}, nil
}

// preparato: quello che il gesto ha controllato sotto BloccaThread.
type preparato struct {
	occ   Occorrenza
	per   Perimetro
	tutte []Occorrenza // l'occorrenza e le voci, in ordine di allegato_id
	el    ElencoDipendenze
}

// prepara fa i controlli comuni dei gesti sotto il lucchetto della RFQ, nell'ordine fisso del contratto (EM §10.4 tempo
// 1 passi 1-4; §10.5 fase A; §10.6): BloccaThread, l'intento, la RFQ unita, l'occorrenza della RFQ e la sua natura, le
// proposte dell'occorrenza e delle sue voci FOR UPDATE, lo stato e la versione attesa (riletti dopo quel lucchetto), il
// perimetro di un archivio, le dipendenze per ogni occorrenza del gesto. Un esito gia' registrato con la stessa chiave
// torna in registrato.
//
// Le proposte si prendono PRIMA di leggere gli stati e di calcolare le dipendenze, non dopo: un gesto legacy che tiene
// una proposta senza la RFQ (il vecchio «Scarta il file», «Documentazione generale») o che la tiene mentre il gesto
// aspetta la RFQ finisce prima, e il gesto legge quello che ha scritto (una provenienza nuova che porta D-1, una
// proposta scartata che fa lo scarto col gesto precedente). Controllo e azione stanno cosi' sotto gli stessi lucchetti
// (rilievo della revisione); le prese successive, in tempo1 e nella fase C, sono gia' di questa transazione. L'ordine
// resta quello di EM §10.6: la RFQ, le righe (proposte in ordine di proposta_id, come il risultato dello stage), per
// ultimo il contenuto.
func (s *Servizio) prepara(ctx context.Context, q *db.Queries, g gesto) (*preparato, *EsitoGesto, error) {
	t, err := q.BloccaThread(ctx, g.thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, rfqNonTrovata()
	}
	if err != nil {
		return nil, nil, err
	}
	reg, err := cercaIntento(ctx, q, g.thread, g.it)
	if err != nil || reg != nil {
		return nil, reg, err
	}
	if t.UnitoIn.Valid {
		return nil, nil, rfqUnita()
	}
	o, err := leggiOccorrenza(ctx, q, g.thread, g.allegato)
	if err != nil {
		return nil, nil, err
	}
	if o == nil {
		return nil, nil, fuoriRFQ("")
	}
	if !o.gestibile() {
		return nil, nil, nonGestito(string(o.Natura))
	}
	discendenti, err := q.ListDiscendentiArchivio(ctx, o.AllegatoID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := q.BloccaPropostaDiAllegato(ctx, append([]uuid.UUID{o.AllegatoID}, discendenti...)); err != nil {
		return nil, nil, err
	}
	// riletta dopo il lucchetto delle proposte: lo stato della proposta (e con lui lo scarto col gesto precedente) e'
	// quello di adesso; lo stato del ciclo dei file non poteva cambiare (lo scrive solo chi tiene la RFQ)
	if o, err = leggiOccorrenza(ctx, q, g.thread, g.allegato); err != nil {
		return nil, nil, err
	}
	if o == nil {
		return nil, nil, fuoriRFQ("")
	}
	vista := *g.corpo.VersioneAttesa
	switch g.nome {
	case GestoScarta:
		if o.Stato == StatoScartato || o.Versione != vista {
			return nil, nil, fileVersione(o.StatoCiclo(), o.Versione)
		}
	case GestoAltriFile:
		if o.StatoCiclo() == StatoScartato {
			return nil, nil, fileScartato()
		}
		if o.Versione != vista || o.Stato != StatoAttivo {
			return nil, nil, fileVersione(o.StatoCiclo(), o.Versione)
		}
	case GestoRiattiva:
		if o.StatoCiclo() == StatoScartato {
			return nil, nil, fileScartato()
		}
		if o.Versione != vista || o.Stato != StatoAltriFile {
			return nil, nil, fileVersione(o.StatoCiclo(), o.Versione)
		}
	}
	p, err := perimetroDa(ctx, q, *o, discendenti)
	if err != nil {
		return nil, nil, err
	}
	if rf := controllaPerimetro(p, g.corpo.NVoci, g.corpo.ImprontaVoci); rf != nil {
		return nil, nil, rf
	}
	pr := &preparato{occ: *o, per: p, tutte: ordinate(append([]Occorrenza{*o}, p.Voci...))}
	if g.nome == GestoRiattiva {
		return pr, nil, nil
	}
	nVoci := map[uuid.UUID]int{}
	if p.Archivio {
		nVoci[o.AllegatoID] = p.NVoci()
	}
	el, err := s.calcolaDipendenze(ctx, q, g.thread, pr.tutte, nVoci)
	if err != nil {
		return nil, nil, err
	}
	if rf := controllaDipendenze(el, *g.corpo.ImprontaDipendenze, g.corpo.scollega(), g.corpo.unicaCopia(), g.nome == GestoScarta); rf != nil {
		return nil, nil, rf
	}
	pr.el = el
	return pr, nil, nil
}

// fotografia dell'occorrenza per l'evento (EM §10.1): letta dopo BloccaAllegatiPerFotografia, con lo stato della
// proposta. Nessun percorso del disco: path_interno e' il percorso dentro l'archivio.
func fotografia(o Occorrenza) json.RawMessage {
	f := map[string]any{
		"messaggio_id": o.MessaggioID, "canale": o.Canale, "indice": o.Indice, "nome_file": o.NomeFile,
		"origine": o.Origine, "ricevuto_il": o.RicevutoIl, "natura": o.Natura,
	}
	if o.ContenitoreID.Valid {
		f["contenitore_id"] = o.ContenitoreID.UUID
	}
	if o.PathInterno.Valid {
		f["path_interno"] = o.PathInterno.String
	}
	if o.Estensione.Valid {
		f["estensione"] = o.Estensione.String
	}
	if o.Sha() != "" {
		f["sha256"] = o.Sha()
	}
	if o.Bytes.Valid {
		f["bytes"] = o.Bytes.Int64
	}
	if o.CaricatoDa.Valid {
		f["caricato_da"] = o.CaricatoDa.UUID
	}
	if o.PropostaID.Valid {
		f["proposta"] = map[string]any{"proposta_id": o.PropostaID.UUID, "stato": o.StatoProposta.StatoProposta}
	}
	b, _ := json.Marshal(f)
	return b
}

// dipendenzeDellOccorrenza: per l'evento, l'impronta dell'elenco intero e le voci di quell'occorrenza (EM §10.4:
// l'evento dell'archivio porta solo il conteggio e l'impronta delle voci, ogni voce ha la sua riga).
func dipendenzeDellOccorrenza(el ElencoDipendenze, id uuid.UUID, p *Perimetro) json.RawMessage {
	var sue []Dipendenza
	for _, d := range el.Voci {
		if d.AllegatoID == id {
			sue = append(sue, d)
		}
	}
	if sue == nil {
		sue = []Dipendenza{}
	}
	m := map[string]any{"impronta_dipendenze": el.Impronta, "dipendenze": sue}
	if p != nil && p.Archivio {
		m["n_voci"] = p.NVoci()
		m["impronta_voci"] = p.Impronta()
	}
	b, _ := json.Marshal(m)
	return b
}

// scriviStato porta l'occorrenza allo stato nuovo: la riga nasce al primo gesto (versione 1) o cresce di uno dalla
// versione letta sotto il lucchetto. Lo scrive solo questo pacchetto (CO-12). Restituisce la versione nuova.
func scriviStato(ctx context.Context, q *db.Queries, o Occorrenza, stato string, utente uuid.UUID) (int32, error) {
	if o.Versione == 0 {
		r, err := q.InsertFileRfq(ctx, db.InsertFileRfqParams{Stato: stato, UtenteID: utente, AllegatoID: o.AllegatoID, ThreadID: o.Thread()})
		if err != nil {
			return 0, err
		}
		return r.Versione, nil
	}
	n, err := q.UpdateFileRfqStato(ctx, db.UpdateFileRfqStatoParams{Stato: stato, UtenteID: utente, ThreadID: o.Thread(),
		AllegatoID: o.AllegatoID, VersioneAttesa: o.Versione})
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fileVersione(o.StatoCiclo(), o.Versione)
	}
	return o.Versione + 1, nil
}

// evento scrive un evento della vita di un file.
type evento struct {
	o          Occorrenza
	azione     string
	prima      string
	dopo       string
	motivo     string
	fotografia json.RawMessage
	dipendenze json.RawMessage
	esito      any
	altro      uuid.NullUUID
	origine    string
	autore     uuid.NullUUID
	chiave     uuid.NullUUID
}

func scriviEvento(ctx context.Context, q *db.Queries, thread uuid.UUID, e evento) (int64, error) {
	p := db.InsertFileRfqEventoParams{ThreadID: thread, AllegatoID: e.o.AllegatoID, Azione: e.azione, Origine: e.origine,
		AltroAllegatoID: e.altro, AutoreID: e.autore, ChiaveIdempotenza: e.chiave}
	if e.prima != "" {
		p.StatoPrima = pgtype.Text{String: e.prima, Valid: true}
	}
	if e.dopo != "" {
		p.StatoDopo = pgtype.Text{String: e.dopo, Valid: true}
	}
	if e.motivo != "" {
		p.Motivo = pgtype.Text{String: e.motivo, Valid: true}
	}
	if len(e.fotografia) > 0 {
		f := e.fotografia
		p.Fotografia = &f
	}
	if len(e.dipendenze) > 0 {
		d := e.dipendenze
		p.Dipendenze = &d
	}
	if e.esito != nil {
		b, err := json.Marshal(e.esito)
		if err != nil {
			return 0, err
		}
		r := json.RawMessage(b)
		p.Esito = &r
	}
	r, err := q.InsertFileRfqEvento(ctx, p)
	if err != nil {
		return 0, err
	}
	return r.EventoID, nil
}

func uid(u uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: u, Valid: true} }

// scollega toglie l'associazione della proposta aperta di un'occorrenza con D-7 (V1-Q22 A: la risoluzione nello stesso
// gesto, registrata come scollegamento con chi e quando): il codice resta quello della proposta, il componente va a
// NULL, e associazione_evento «sgancia» porta il componente di prima.
func scollega(ctx context.Context, q *db.Queries, thread uuid.UUID, o Occorrenza, g gesto) error {
	if !o.PropostaID.Valid || !o.PropostaComponenteID.Valid || o.StatoProposta.StatoProposta != db.StatoPropostaAperta {
		return nil
	}
	p, err := q.GetPropostaDocumentoDiAllegato(ctx, o.AllegatoID)
	if err != nil {
		return err
	}
	if p.Stato != db.StatoPropostaAperta || !p.ComponenteID.Valid {
		return nil
	}
	if _, err := q.SetComponenteProposta(ctx, db.SetComponentePropostaParams{PropostaID: p.PropostaID, Codice: p.Codice}); err != nil {
		return err
	}
	_, err = q.InsertAssociazioneEvento(ctx, db.InsertAssociazioneEventoParams{ThreadID: thread, PropostaID: uid(p.PropostaID),
		Azione: "sgancia", ComponentePrima: p.ComponenteID, Origine: OriginePonte, ChiaveIdempotenza: uid(g.it.chiave),
		AutoreID: g.utente, Motivo: testo(g.corpo.motivo())})
	return err
}

func testo(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// haRisoluzione: l'occorrenza ha la dipendenza con quel codice nell'elenco.
func haDipendenza(el ElencoDipendenze, id uuid.UUID, codice string) bool {
	for _, d := range el.Voci {
		if d.AllegatoID == id && d.Codice == codice {
			return true
		}
	}
	return false
}
