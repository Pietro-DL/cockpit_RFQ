package web

// motore_a_bozza_v1_r4.go: R4 formato 3, il riepilogo della conferma senza scritture (EM §9.4; direttiva 39 §1, §4, §5,
// §6 [U + T-39]), corsia persistenza. In un file a parte, montato da registraBozzaMotoreA:
//   - R4 POST /thread/{id}/motore-a/bom/riepilogo[?avvisi_motore=1]   il riepilogo della bozza formato 3 del corpo
// Il contratto ha la sola POST (EM §9.4: «POST di sola lettura», C3 §4.1): nessuna GET [T-39: «GET/POST come da contratto»
// = la POST, piu' la riga gemella che risponde 405 agli altri metodi con Allow: POST].
//
// La catena e' quella di R1-R3 (motore_a_bozza_v1.go): conMotoreA → conNoStore → soloAdmin → gestore. L'ordine: l'id del
// percorso (400) → la RFQ (404) → il corpo (413) → la forma del corpo e della query (422) → il calcolo
// (distintabozza.Riepilogo3) in una transazione REPEATABLE READ READ ONLY chiusa con ROLLBACK: nessun lucchetto, nessuna
// scrittura (dentro TestNessunaGetScriveDistintaV1). La risposta e' 200 anche con dei blocchi (confermabile: false),
// compreso il blocco firmato STEP_SORGENTE_NON_CONFERMATA di una bozza senza STEP (direttiva 39 §1: non e' un errore della
// lettura). Con ?avvisi_motore=1 R4 calcola il Motore A (motore_a_bozza_v1_motore.go: Servizio.Calcola con il semaforo dei
// calcoli, nella sua fotografia, prima della transazione del riepilogo) e il core ne compone gli avvisi e i conflitti, firmati;
// il motore che non risponde da' 200 con avvisi_motore_calcolati false e avvisi_motore_errore (CALCOLO_OCCUPATO,
// CALCOLO_SCADUTO, ERRORE_CALCOLO), mai un 5xx per il parametro (EM §9.4). Senza il parametro e' il riepilogo leggero, non
// confermabile (PA-C01). La risposta porta anche ingressi (le parti di R1 ?ingressi=1, fuori firma).

import (
	"net/http"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// registraRiepilogoV1 monta R4 con la sua riga gemella.
func (s *Server) registraRiepilogoV1(mux *http.ServeMux) {
	catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }
	gemella := func(ammessi string) http.HandlerFunc {
		return s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1(ammessi)))
	}
	mux.HandleFunc("POST /thread/{id}/motore-a/bom/riepilogo", catena(s.bozzaV1Riepilogo))
	mux.HandleFunc("/thread/{id}/motore-a/bom/riepilogo", gemella("POST"))
}

// bozzaV1Riepilogo: R4 (commento di testa).
func (s *Server) bozzaV1Riepilogo(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	for k := range q {
		if k != "avvisi_motore" {
			s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: k,
				Messaggio: "R4 ha un solo parametro: avvisi_motore=1"})
			return
		}
	}
	if q.Has("avvisi_motore") && q.Get("avvisi_motore") != "1" {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "avvisi_motore",
			Messaggio: "avvisi_motore vale 1"})
		return
	}
	richiesta, err := distintabozza.LeggiRichiestaR4(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	op := distintabozza.OpzioniR4{UtenteID: utenteDa(r.Context()).UtenteID, AvvisiMotore: q.Get("avvisi_motore") == "1", ConIngressi: true,
		ImprontaRegole: improntaRegoleV1(s.MotoreA)}
	if op.AvvisiMotore {
		op.Motore = s.motoreR4(r.Context(), thread, richiesta.Snapshot.ProdottoID)
	}
	var out distintabozza.RiepilogoR4
	err = s.inSolaLetturaV1(r.Context(), func(qq *db.Queries) error {
		var e error
		out, e = distintabozza.Riepilogo3(r.Context(), qq, thread, richiesta, op)
		return e
	})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}
