package web

// motore_a_sorgente_v1.go: S8 e la revoca della sorgente (direttiva 39 §2, §3 [U + T-39]; U2 §2.2 ed EM §10.9 «S8» per
// la forma, superati dove la direttiva li supera), corsia persistenza. In un file a parte, montato da registraBozzaMotoreA:
//   - S8     POST /thread/{id}/sorgente/step/{allegato}/conferma-sorgente   «Conferma questo STEP come sorgente di ‹prodotto›»
//   - revoca POST /thread/{id}/sorgente/step/{allegato}/revoca-sorgente     la revoca motivata di uno STEP scelto per errore
// La catena e' quella di R1-R3 (motore_a_bozza_v1.go): conMotoreA (flag spento = il 404 del router, prima della sessione)
// → conNoStore → soloAdmin → gestore; per gli altri metodi una riga gemella (405 con Allow). CSRF globale. Corpo JSON
// stretto, al piu' 2 MiB + 4 KiB.
//
// S8, l'ordine: gli id del percorso (400) → la RFQ (404) → il corpo (413, 422) → l'intento gia' scritto con la stessa
// chiave (200 riproposto; un altro corpo 409 CHIAVE_RIUSATA), senza calcolare → il passo 0 senza il motore
// (sorgentev1.Controlla: 422/409, niente scritto) → il calcolo del Motore A in sola lettura con il semaforo (503/504, niente
// scritto): il registro del gruppo dichiarato con V-7 (partenza step), oppure lo stato dell'analisi dello STEP e la
// presenza di una struttura importabile (partenza solo_prodotto) → la procedura del core (sorgentev1.ConfermaS8), che
// risponde 200 completata o parziale. Un guasto dopo il passo 0 e' un 5xx {esito: da_verificare, chiave_idempotenza}: la
// chiave si rilegge con R6.
//
// [T-39] S8 non passa dal rismistamento dopo il gesto (dopoIlGesto): la direttiva vuole che S8 non tocchi la working ne'
// le copie; dopo S8 la pagina rilegge S9.

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// CodiciServerS8: i codici propri di S8 e della revoca, con il loro stato HTTP (gli altri sono quelli di
// statoDelCodiceV1). I cataloghi del ponte non li hanno ancora: richiesta alla corsia interfaccia.
var CodiciServerS8 = []CodiceRotta{
	{"S8", sorgentev1.CodiceStepSorgenteDiAltroProdotto, http.StatusUnprocessableEntity},
	{"S8", sorgentev1.CodiceGiaDocumento, http.StatusUnprocessableEntity},
	{"S8", sorgentev1.CodiceNonStep, http.StatusUnprocessableEntity},
	{"S8", sorgentev1.CodiceRiconciliazioneNonDisponibile, http.StatusConflict},
}

// preparatoreS8: il calcolo del motore del passo 0 di S8. La rotta usa preparaS8 (il motore vero); preparaS8Prova, se
// non e' nil, lo sostituisce: solo le prove del package (il banco web non carica le regole del motore).
type preparatoreS8 func(ctx context.Context, thread, allegato uuid.UUID, r sorgentev1.RichiestaS8) (sorgentev1.Preparazione, error)

var preparaS8Prova preparatoreS8

// registraSorgenteV1 monta S8 e la revoca con le loro righe gemelle.
func (s *Server) registraSorgenteV1(mux *http.ServeMux) {
	catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }
	gemella := func(ammessi string) http.HandlerFunc {
		return s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1(ammessi)))
	}
	mux.HandleFunc("POST /thread/{id}/sorgente/step/{allegato}/conferma-sorgente", catena(s.sorgenteV1Conferma))
	mux.HandleFunc("/thread/{id}/sorgente/step/{allegato}/conferma-sorgente", gemella("POST"))
	mux.HandleFunc("POST /thread/{id}/sorgente/step/{allegato}/revoca-sorgente", catena(s.sorgenteV1Revoca))
	mux.HandleFunc("/thread/{id}/sorgente/step/{allegato}/revoca-sorgente", gemella("POST"))
}

// rifiutoS8Guasto: il 5xx di S8 e della revoca dopo il passo 0 (U2 §2.2): l'esito si rilegge con R6.
type rifiutoS8Guasto struct {
	Esito             string    `json:"esito"`
	Codice            string    `json:"codice"`
	ChiaveIdempotenza uuid.UUID `json:"chiave_idempotenza"`
	Testo             string    `json:"testo"`
}

// rispondiErroreSorgenteV1: i rifiuti come le rotte della bozza; un guasto con la chiave e «da_verificare».
func (s *Server) rispondiErroreSorgenteV1(w http.ResponseWriter, r *http.Request, thread, chiave uuid.UUID, err error) {
	stato, _ := rispostaErroreBozzaV1(err)
	if stato < 500 || chiave == uuid.Nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	if s.Log != nil {
		s.Log.Error("sorgente V1: richiesta non riuscita", "percorso", r.URL.Path, "thread", thread, "err", err)
	}
	s.scriviJSONBozzaV1(w, r, http.StatusInternalServerError, rifiutoS8Guasto{Esito: "da_verificare", Codice: codiceErroreV1, ChiaveIdempotenza: chiave,
		Testo: "La richiesta non è riuscita: l'esito è da verificare con la stessa chiave (R6). Il dettaglio è nel log del server."})
}

// sorgenteV1Conferma: S8 (commento di testa).
func (s *Server) sorgenteV1Conferma(w http.ResponseWriter, r *http.Request) {
	prepara := s.preparaS8
	if preparaS8Prova != nil {
		prepara = preparaS8Prova
	}
	s.confermaSorgenteCon(w, r, prepara)
}

func (s *Server) confermaSorgenteCon(w http.ResponseWriter, r *http.Request, prepara preparatoreS8) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok {
		return
	}
	allegato, ok := s.idDelPercorsoV1(w, r, "allegato")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	req, impronta, err := sorgentev1.LeggiRichiestaS8(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	ctx := r.Context()
	// l'intento gia' scritto: lo stesso reinvio non calcola niente
	var gia json.RawMessage
	err = s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		var err error
		gia, err = sorgentev1.IntentoRegistrato(ctx, q, thread, req.ChiaveIdempotenza, impronta, sorgentev1.GestoConfermaSorgente)
		return err
	})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	if gia != nil {
		es, err := sorgentev1.Riproposto(gia)
		if err != nil {
			s.rispondiErroreBozzaV1(w, r, thread, err)
			return
		}
		s.scriviJSONBozzaV1(w, r, http.StatusOK, es)
		return
	}
	// il passo 0 senza il motore
	if err := s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		_, err := sorgentev1.Controlla(ctx, q, thread, allegato, req)
		return err
	}); err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	// il passo 0 con il motore (sola lettura, semaforo): niente scritto se non riesce
	prep, err := prepara(ctx, thread, allegato, req)
	if err != nil {
		s.rispondiErroreCalcoloV1(w, r, thread, err)
		return
	}
	es, err := sorgentev1.ConfermaS8(ctx, s.Pool, thread, allegato, utenteDa(ctx).UtenteID, req, impronta, prep)
	if err != nil {
		s.rispondiErroreSorgenteV1(w, r, thread, req.ChiaveIdempotenza, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, es)
}

// preparaS8: il calcolo del Motore A della RFQ per il passo 0 di S8 (come le forme di R2 che importano,
// motore_a_bozza_v1_forme.go): per la partenza step il gruppo dichiarato fra quelli del prodotto, il suo registro, V-7;
// per la partenza solo_prodotto lo stato dell'analisi dello STEP e se una struttura importabile c'e'.
func (s *Server) preparaS8(ctx context.Context, thread, allegato uuid.UUID, r sorgentev1.RichiestaS8) (sorgentev1.Preparazione, error) {
	c, err := s.calcolaR2(ctx, thread, r.ProdottoID)
	if err != nil {
		return sorgentev1.Preparazione{}, err
	}
	if r.Partenza == sorgentev1.PartenzaStep {
		d := distintabozza.SorgenteDichiarata{AllegatoID: allegato, Gruppo: r.Gruppo, Sha256: r.Sha256}
		g, err := distintabozza.GruppoDaImportare("gruppo", c.gruppi(), c.nonAttivi(), d, r.ImprontaContenuto, nil, c.improntaDi)
		if err != nil {
			return sorgentev1.Preparazione{}, err
		}
		reg, err := c.importa(g.Chiave)
		if err != nil {
			return sorgentev1.Preparazione{}, distintabozza.ErroreDellAdattatore(err, "gruppo")
		}
		if err := distintabozza.VerificaImprontaDichiarata("impronta_contenuto", reg, r.ImprontaContenuto); err != nil {
			return sorgentev1.Preparazione{}, err
		}
		return sorgentev1.Preparazione{Proposta: c.proposta(reg)}, nil
	}
	out := sorgentev1.Preparazione{StatoAnalisi: statoAnalisiS8(c.ris, allegato)}
	for _, g := range c.gruppi() {
		if g.AllegatoID != nil && *g.AllegatoID == allegato && g.Tipo != propostabom.GruppoSenzaSTEP {
			if _, err := c.importa(g.Chiave); err == nil {
				out.Importabile = true
				break
			}
		}
	}
	return out, nil
}

// statoAnalisiS8: lo stato dell'analisi dello STEP nel calcolo (direttiva 39 §3: in corso o fallita si dice, mai come prova
// geometrica). Aggancio della corsia motore (direttiva 39, richiesta di questa corsia): la stessa funzione che la pagina
// mostra in /dati.fonte.struttura_step e in S6 (propostabom.StatoAnalisiDelFile): fatti pendenti o mancanti = in_analisi;
// errore o illeggibile = errore; un file senza i fatti di un'analisi = non_valutato; letto = pronta; il file fuori dal
// calcolo = non_valutato. I valori sono quelli di sorgentev1.Analisi* (prova L1 motore_a_anteprima_s6_test.go).
func statoAnalisiS8(ris *motorea_web.Risultato, allegato uuid.UUID) string {
	if ris == nil || ris.EsitoThread == nil {
		return sorgentev1.AnalisiNonValutato
	}
	return propostabom.StatoAnalisiDelFile(ris.EsitoThread, allegato)
}

// sorgenteV1Revoca: la revoca della sorgente (commento di testa).
func (s *Server) sorgenteV1Revoca(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok {
		return
	}
	allegato, ok := s.idDelPercorsoV1(w, r, "allegato")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	req, impronta, err := sorgentev1.LeggiRichiestaRevoca(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	out, err := sorgentev1.Revoca(r.Context(), s.Pool, thread, allegato, utenteDa(r.Context()).UtenteID, req, impronta)
	if err != nil {
		s.rispondiErroreSorgenteV1(w, r, thread, req.ChiaveIdempotenza, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}
