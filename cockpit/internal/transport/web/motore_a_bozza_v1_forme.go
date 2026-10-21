package web

// motore_a_bozza_v1_forme.go: le forme di R2 che importano (crea con partenza step, cambia_sorgente, riallinea_proposta;
// EM §6.3, §6.5, §6.6, §9.2; direttiva 35, blocco C-3, corsia persistenza). Prima della transazione di scrittura e fuori da
// ogni lucchetto (EM §6.3 passi 0-2):
//  0. la chiave del salvataggio in sola lettura: un reinvio con la stessa chiave e lo stesso corpo risponde dal servizio
//     senza calcolare il motore (riproposto); un'altra impronta e' 409 CHIAVE_RIUSATA;
//  1. il calcolo del Motore A della RFQ in sola lettura, con il semaforo dei calcoli (Servizio.Calcola, come /dati e R15);
//     il gruppo del prodotto scelto fra quelli calcolati adesso (distintabozza.GruppoDaImportare per un gruppo dichiarato,
//     il gruppo corrente della sorgente per il riallineamento); il registro del gruppo (propostabom.ImportaGruppo);
//  2. V-7: l'impronta del contenuto dichiarata e' quella del registro (409 ANTEPRIMA_SUPERATA, proposta_step).
// Poi il servizio del core, con il lucchetto della RFQ, scrive. Gli errori del calcolo sono 503/504 (niente scritto).

import (
	"context"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// calcoloR2: il calcolo del motore di una forma che importa, per il prodotto: i gruppi del prodotto, le occorrenze non
// attive e la funzione che importa un gruppo. pv nil = il prodotto non e' fra i prodotti valutati (nessun gruppo).
type calcoloR2 struct {
	ris *motorea_web.Risultato
	pv  *valutazione.ProdottoValutato
}

func (c calcoloR2) gruppi() []propostabom.Gruppo {
	if c.pv == nil {
		return nil
	}
	return propostabom.GruppiDelProdotto(*c.pv, c.ris.EsitoThread.Ancoraggi.Strutture)
}

func (c calcoloR2) nonAttivi() propostabom.NonAttivi { return c.ris.V1.NonAttivi }

// importa: il registro di un gruppo del prodotto, composto (propostabom.ImportaGruppo con la fotografia del calcolo).
func (c calcoloR2) importa(gruppo string) (importazione.Registro, error) {
	if c.pv == nil {
		return importazione.Registro{}, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Motivo: distintabozza.MotivoGruppoNonImportabile,
			Messaggio: "il prodotto non e' fra i prodotti che il motore valuta: nessun gruppo da importare"}
	}
	return propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &c.ris.Esito, Thread: c.ris.EsitoThread, Regole: c.ris.Regole, Prodotto: c.pv.Rif,
		Gruppo: gruppo, NonAttivi: c.ris.V1.NonAttivi, Foto: &c.ris.Foto})
}

// improntaDi: l'impronta del contenuto di un gruppo importato adesso (per ritrovare la stessa struttura sotto un'altra chiave).
func (c calcoloR2) improntaDi(gruppo string) (string, error) {
	r, err := c.importa(gruppo)
	if err != nil {
		return "", err
	}
	return r.Contenuto.ImprontaContenuto, nil
}

// proposta: la proposta calcolata di una forma che importa, con gli STEP candidati sorgente dei finiti dello stesso calcolo
// (G39-02 [U]: lo STEP del finito si riconosce con il Motore A; le PB iniziali non lo toccano).
func (c calcoloR2) proposta(reg importazione.Registro) *distintabozza.PropostaCalcolata {
	return &distintabozza.PropostaCalcolata{Registro: reg, StepCandidati: distintabozza.StepCandidatiDalMotore(c.ris.EsitoThread, c.ris.Thread)}
}

// calcolaR2: il calcolo del motore della RFQ (sola lettura, semaforo) e il prodotto valutato.
func (s *Server) calcolaR2(ctx context.Context, thread, prodotto uuid.UUID) (calcoloR2, error) {
	ris, err := s.servizioMotoreA().Calcola(ctx, thread)
	if err != nil {
		return calcoloR2{}, err
	}
	out := calcoloR2{ris: ris}
	for i := range ris.EsitoThread.ProdottiValutati {
		if pv := &ris.EsitoThread.ProdottiValutati[i]; pv.ComponenteID != nil && *pv.ComponenteID == prodotto {
			out.pv = pv
		}
	}
	return out, nil
}

// riproponiR2: il passo 0 delle forme che importano (commento di testa): ok vero se la chiave ha gia' salvato questo corpo
// (il servizio, chiamato senza il calcolo, risponde riproposto); un'altra impronta e' l'errore CHIAVE_RIUSATA.
func (s *Server) riproponiR2(ctx context.Context, thread uuid.UUID, c distintabozza.ComuneR2) (bool, error) {
	var ok bool
	err := s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		_, trovata, err := distintabozza.RiproponiFormaDelServer(ctx, q, thread, c)
		ok = trovata
		return err
	})
	return ok, err
}

// propostaPerCrea: la proposta della partenza step (EM §6.3 passi 1-2): il gruppo dichiarato fra quelli del prodotto, il
// suo registro, V-7.
func (s *Server) propostaPerCrea(ctx context.Context, thread uuid.UUID, req distintabozza.RichiestaR2) (*distintabozza.PropostaCalcolata, error) {
	c, err := s.calcolaR2(ctx, thread, req.Comune.ProdottoID)
	if err != nil {
		return nil, err
	}
	g, err := distintabozza.GruppoDaImportare("crea.sorgente", c.gruppi(), c.nonAttivi(), *req.Crea.Sorgente, req.Crea.ImprontaContenuto, nil, c.improntaDi)
	if err != nil {
		return nil, err
	}
	reg, err := c.importa(g.Chiave)
	if err != nil {
		return nil, distintabozza.ErroreDellAdattatore(err, "crea.sorgente.gruppo")
	}
	if err := distintabozza.VerificaImprontaDichiarata("crea.impronta_contenuto", reg, req.Crea.ImprontaContenuto); err != nil {
		return nil, err
	}
	return c.proposta(reg), nil
}

// sorgenteDellaBozzaR2: la sorgente della bozza del corpo, in sola lettura (per il gruppo corrente e stessa_sorgente). ok
// falso: la bozza non e' aperta, non e' del prodotto o non ha una sorgente STEP (lo dira' il servizio sotto il lucchetto).
func (s *Server) sorgenteDellaBozzaR2(ctx context.Context, thread uuid.UUID, c distintabozza.ComuneR2) (propostabom.SorgenteBozza, bool, error) {
	var src propostabom.SorgenteBozza
	var ok bool
	err := s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		var err error
		src, ok, err = distintabozza.SorgenteDellaBozza(ctx, q, thread, c.ProdottoID, c.BozzaID)
		return err
	})
	return src, ok, err
}

// propostaPerRiallinea: la proposta di riallinea_proposta (EM §6.5 passo 1): il gruppo corrente della sorgente fra i gruppi
// di adesso (stessa_occorrenza; altrimenti 422 gruppo_su_altra_occorrenza o gruppo_non_importabile), il suo registro, V-7.
// Una bozza senza sorgente STEP (o non aperta) non calcola il motore: il servizio risponde con il suo rifiuto.
func (s *Server) propostaPerRiallinea(ctx context.Context, thread uuid.UUID, req distintabozza.RichiestaR2) (*distintabozza.PropostaCalcolata, error) {
	src, ok, err := s.sorgenteDellaBozzaR2(ctx, thread, req.Comune)
	if err != nil || !ok {
		return nil, err
	}
	c, err := s.calcolaR2(ctx, thread, req.Comune.ProdottoID)
	if err != nil {
		return nil, err
	}
	chiave, err := distintabozza.GruppoDelRiallineamento(c.gruppi(), c.nonAttivi(), src)
	if err != nil {
		return nil, err
	}
	reg, err := c.importa(chiave)
	if err != nil {
		return nil, distintabozza.ErroreDellAdattatore(err, "riallinea_proposta")
	}
	if err := distintabozza.VerificaImprontaDichiarata("riallinea_proposta.impronta_contenuto", reg, req.Riallinea.ImprontaContenuto); err != nil {
		return nil, err
	}
	return c.proposta(reg), nil
}

// propostaPerCambia: la proposta di cambia_sorgente (EM §6.6 passo 1): il gruppo dichiarato fra quelli del prodotto, con
// stessa_sorgente contro la sorgente della bozza; il suo registro; V-7.
func (s *Server) propostaPerCambia(ctx context.Context, thread uuid.UUID, req distintabozza.RichiestaR2) (*distintabozza.PropostaCalcolata, error) {
	src, ok, err := s.sorgenteDellaBozzaR2(ctx, thread, req.Comune)
	if err != nil {
		return nil, err
	}
	var sorgente *propostabom.SorgenteBozza
	if ok {
		sorgente = &src
	}
	c, err := s.calcolaR2(ctx, thread, req.Comune.ProdottoID)
	if err != nil {
		return nil, err
	}
	d := req.CambiaSorgente
	g, err := distintabozza.GruppoDaImportare("cambia_sorgente.sorgente", c.gruppi(), c.nonAttivi(), d.Sorgente, d.ImprontaContenuto, sorgente, c.improntaDi)
	if err != nil {
		return nil, err
	}
	reg, err := c.importa(g.Chiave)
	if err != nil {
		return nil, distintabozza.ErroreDellAdattatore(err, "cambia_sorgente.sorgente.gruppo")
	}
	if err := distintabozza.VerificaImprontaDichiarata("cambia_sorgente.impronta_contenuto", reg, d.ImprontaContenuto); err != nil {
		return nil, err
	}
	return c.proposta(reg), nil
}
