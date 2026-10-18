package filerfq

import (
	"context"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Scarta e' S2, «Scarta» (EM §10.4; base di lavoro della domanda V1-Q05 A per il nome): terminale in V1 (V1-Q20 A),
// in due tempi. Il tempo 1 e' la decisione, in una transazione: per l'occorrenza e per ogni voce del perimetro lo stato
// scartato con il suo evento, lo scollegamento chiesto (D-7), la chiusura della proposta aperta, l'annullamento dei
// download e delle estrazioni pendenti, la riga della rimozione in attesa, ogni copia di conservazione viva da
// rilasciare, l'esito dell'intento. Il path_staging non si tocca nel tempo 1. Dopo il COMMIT, un tentativo del tempo 2
// e del rilascio, con un tempo massimo; il resto lo fa il custode.
func (s *Servizio) Scarta(ctx context.Context, thread, allegato, utente uuid.UUID, c CorpoGesto) (*EsitoGesto, error) {
	g, err := nuovoGesto(GestoScarta, thread, allegato, utente, c)
	if err != nil {
		return nil, err
	}
	var esito *EsitoGesto
	err = s.inTx(ctx, func(q *db.Queries) error {
		pr, reg, err := s.prepara(ctx, q, g)
		if err != nil {
			return err
		}
		if reg != nil {
			esito = reg
			return nil
		}
		esito, err = s.tempo1(ctx, q, g, pr)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.dopoIlGesto(ctx, thread, esito.Occorrenze)
	return s.ricomponi(ctx, thread, esito)
}

// tempo1: le scritture della decisione, dopo i controlli di prepara (EM §10.4, tempo 1 passo 5). L'ordine dei lucchetti
// e' fisso: le proposte (gia' prese da prepara, prima dei controlli: qui la presa e' di questa transazione), i job (SKIP
// LOCKED: non si aspetta mai), gli allegati FOR SHARE, poi le righe del ciclo dei file.
func (s *Servizio) tempo1(ctx context.Context, q *db.Queries, g gesto, pr *preparato) (*EsitoGesto, error) {
	ids := idDi(pr.tutte)
	chiavi := make([]string, 0, 2*len(ids))
	for _, id := range ids {
		chiavi = append(chiavi, "stage:"+id.String(), "estrai:"+id.String())
	}
	if _, err := q.AnnullaJobPendentiPerChiave(ctx, chiavi); err != nil {
		return nil, err
	}
	if _, err := q.BloccaPropostaDiAllegato(ctx, ids); err != nil {
		return nil, err
	}
	for _, o := range pr.tutte {
		if g.corpo.scollega() && haDipendenza(pr.el, o.AllegatoID, "D-7") {
			if err := scollega(ctx, q, g.thread, o, g); err != nil {
				return nil, err
			}
		}
		if o.PropostaID.Valid && o.StatoProposta.StatoProposta == db.StatoPropostaAperta {
			n, err := q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: o.PropostaID.UUID, Stato: db.StatoPropostaScartata,
				DecisoDa: uid(g.utente)})
			if err != nil {
				return nil, err
			}
			if n > 0 {
				if _, err := q.InsertAssociazioneEvento(ctx, db.InsertAssociazioneEventoParams{ThreadID: g.thread,
					PropostaID: uid(o.PropostaID.UUID), Azione: "scarta", Origine: OriginePonte, ChiaveIdempotenza: uid(g.it.chiave),
					AutoreID: g.utente, Motivo: testo(g.corpo.motivo())}); err != nil {
					return nil, err
				}
			}
		}
	}
	if _, err := q.BloccaAllegatiPerFotografia(ctx, ids); err != nil {
		return nil, err
	}
	// la fotografia si rilegge dopo FOR SHARE: un download arrivato prima del lucchetto ha gia' scritto sha e byte
	fresche, err := leggiOccorrenze(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	esito := &EsitoGesto{Esito: "scartato", ChiaveIdempotenza: g.it.chiave, AllegatoID: g.allegato, Stato: StatoScartato,
		Occorrenze: ids}
	for _, o := range fresche {
		per := (*Perimetro)(nil)
		if o.AllegatoID == g.allegato {
			per = &pr.per
		}
		ver, err := scriviStato(ctx, q, o, StatoScartato, g.utente)
		if err != nil {
			return nil, err
		}
		if o.AllegatoID == g.allegato {
			esito.Versione = ver
		}
		prima := o.Stato
		if prima != StatoAltriFile {
			prima = StatoAttivo
		}
		evID, err := scriviEvento(ctx, q, g.thread, evento{o: o, azione: "scarta", prima: prima, dopo: StatoScartato,
			motivo: g.corpo.motivo(), fotografia: fotografia(o), dipendenze: dipendenzeDellOccorrenza(pr.el, o.AllegatoID, per),
			origine: OriginePonte, autore: uid(g.utente), chiave: uid(g.it.chiave)})
		if err != nil {
			return nil, err
		}
		if _, err := q.InsertRimozione(ctx, db.InsertRimozioneParams{ThreadID: g.thread, AllegatoID: o.AllegatoID, EventoID: evID,
			Sha256: o.Sha256}); err != nil {
			return nil, err
		}
		vive, err := q.ListConservazioniVive(ctx, db.ListConservazioniViveParams{ThreadID: g.thread, AllegatoID: o.AllegatoID})
		if err != nil {
			return nil, err
		}
		for _, c := range vive {
			if _, err := q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "da_rilasciare", Causa: testo("scartato"),
				Tentativi: c.Tentativi, ConservazioneID: c.ConservazioneID, StatoAtteso: c.Stato}); err != nil {
				return nil, err
			}
		}
	}
	rim, cons, err := statiCorrenti(ctx, q, g.thread, ids)
	if err != nil {
		return nil, err
	}
	esito.Rimozioni, esito.Conservazioni = rim, cons
	esito.Testo = testoScarto(rim, g.allegato, pr.occ.Stato == StatoAltriFile, cons)
	if err := registraIntento(ctx, q, g.thread, g.it, esito); err != nil {
		return nil, err
	}
	return esito, nil
}

// testoScarto: il testo della risposta, dallo stato della rimozione dell'occorrenza del percorso; da Altri file aggiunge
// l'esito della copia di conservazione (EM §10.4).
func testoScarto(rim []StatoRimozione, allegato uuid.UUID, daAltriFile bool, cons []StatoConservazione) string {
	t := "Scarto registrato."
	for _, r := range rim {
		if r.AllegatoID == allegato {
			t = r.Testo
		}
	}
	if daAltriFile {
		for _, c := range cons {
			if c.AllegatoID == allegato {
				if c.Stato == "rilasciata" {
					t += " La copia di conservazione è stata eliminata."
				} else {
					t += " La copia di conservazione si elimina appena possibile."
				}
			}
		}
	}
	return t
}

// dopoIlGesto: subito dopo il COMMIT (o al reinvio), un tentativo del tempo 2 e del rilascio delle copie delle
// occorrenze del gesto, con il tempo massimo (EM §10.4: «nella stessa richiesta, con un tempo massimo»). Quello che non
// finisce lo riprende il custode; un errore qui non cambia l'esito del gesto, che e' gia' registrato.
func (s *Servizio) dopoIlGesto(ctx context.Context, thread uuid.UUID, ids []uuid.UUID) {
	if s.Agganci != nil && s.Agganci.SaltaTentativi {
		return
	}
	cctx, fine := context.WithTimeout(context.WithoutCancel(ctx), s.tempoTentativo())
	defer fine()
	q := db.New(s.Pool)
	rr, err := q.ListRimozioniOccorrenze(cctx, db.ListRimozioniOccorrenzeParams{ThreadID: thread, Allegati: ids})
	if err == nil {
		for _, r := range rr {
			if cctx.Err() != nil {
				return
			}
			if r.Stato == "in_attesa" || (r.Stato == "errore" && r.Ritentabile) {
				if _, err := s.EseguiRimozione(cctx, r.RimozioneID, false); err != nil {
					s.log().Warn("ciclo dei file: tentativo della rimozione dopo il gesto non riuscito", "rimozione", r.RimozioneID, "err", err)
				}
			}
		}
	}
	for _, id := range ids {
		if cctx.Err() != nil {
			return
		}
		s.rilasciaCopieDi(cctx, thread, id)
	}
}

// rilasciaCopieDi: un tentativo di rilascio delle copie da rilasciare di un'occorrenza.
func (s *Servizio) rilasciaCopieDi(ctx context.Context, thread, allegato uuid.UUID) {
	q := db.New(s.Pool)
	cc, err := q.ListCopieConservazioneAllegato(ctx, allegato)
	if err != nil {
		return
	}
	for _, c := range cc {
		if c.ThreadID != thread || c.Stato != "da_rilasciare" {
			continue
		}
		if _, err := s.RilasciaCopia(ctx, c.ConservazioneID, false); err != nil {
			s.log().Warn("ciclo dei file: tentativo di rilascio dopo il gesto non riuscito", "conservazione", c.ConservazioneID, "err", err)
		}
	}
}

// ricomponi: la risposta con gli stati letti adesso (dopo i tentativi, o al reinvio).
func (s *Servizio) ricomponi(ctx context.Context, thread uuid.UUID, e *EsitoGesto) (*EsitoGesto, error) {
	q := db.New(s.Pool)
	rim, cons, err := statiCorrenti(ctx, q, thread, e.Occorrenze)
	if err != nil {
		return nil, err
	}
	out := *e
	switch e.Esito {
	case "scartato":
		out.Rimozioni, out.Conservazioni = rim, cons
		daAltri := false
		for _, c := range cons {
			if c.AllegatoID == e.AllegatoID {
				daAltri = true
			}
		}
		out.Testo = testoScarto(rim, e.AllegatoID, daAltri, cons)
	case "in_altri_file":
		out.Conservazioni = cons
	case "riattivato":
		for _, c := range cons {
			if c.AllegatoID == e.AllegatoID {
				cc := c
				out.Conservazione = &cc
			}
		}
	}
	return &out, nil
}

// adessoNel: l'orologio del servizio come puntatore (prossimo_tentativo_il).
func (s *Servizio) traDopo(d time.Duration) *time.Time {
	t := s.adesso().Add(d)
	return &t
}
