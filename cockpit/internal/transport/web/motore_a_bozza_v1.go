package web

// motore_a_bozza_v1.go: le rotte della bozza della Distinta unica di V1 che la corsia persistenza scrive al passo 6 del
// piano della 0023 (prima parte, ondata B; emendamento di V1 = EM §9). In un file a parte, per non urtare motore_a.go,
// motore_a_pagina.go e distinta_unica_pagina.go delle altre corsie:
//   - R1  GET  /thread/{id}/motore-a/bozza?prodotto=[&eventi=1][&importazione=1][&registro=<impronta>][&confronto=1]
//         e la variante ?effetto_cambio_tipo=<componente_id>&tipo_nuovo=<tipo> (PE-09); la variante ?ingressi=1 (ondata C,
//         U2 §6.3.3: lo stato degli ingressi del riepilogo, solo database);
//   - R2  POST /thread/{id}/motore-a/bozza                       le cinque forme (le quattro costruite dal server: direttiva 35, C-3);
//   - R3  POST /thread/{id}/motore-a/bozza/{bozza}/abbandona     abbandona una bozza (formato 2 o 3);
//   - R6  GET  /thread/{id}/motore-a/intento/{chiave}           l'esito di un intento;
//   - R7  GET  /thread/{id}/motore-a/conferme?prodotto=[&conferma=][&importazione=1]   lo storico nei formati 1, 2, 3;
//   - R11 POST /thread/{id}/motore-a/riallinea                  rifa' F8 e scrive f8_stato.
// S8 e la revoca della sorgente (direttiva 39) sono in motore_a_sorgente_v1.go, R4 formato 3 in motore_a_bozza_v1_r4.go,
// montate da registraBozzaMotoreA. R5 (la conferma formato 3), S6 e S7 arrivano dopo.
//
// Il montaggio: registraBozzaMotoreA, chiamata da registraMotoreA (motore_a.go; DEC-B RQ-1, autorizzata alla corsia
// persistenza dell'ondata C per questa sola chiamata); le prove la montano da se' solo se Registra non l'ha gia' montata.
//
// La catena di ogni rotta e' quella di motore_a.go (EM §9, §12): conMotoreA (flag spento = il 404 del router, prima della
// sessione) → conNoStore → soloAdmin → gestore; per gli altri metodi una riga gemella senza metodo (405 con Allow a flag
// acceso, 404 a flag spento). CSRF: http.CrossOriginProtection, globale. Il corpo JSON e' al piu' 2 MiB + 4 KiB, letto in
// modo stretto (DisallowUnknownFields, niente dopo il valore); oltre, 413.
//
// L'ordine dei controlli (EM §9, «prima l'esistenza della RFQ, poi la forma», REV RS-11): gli id del percorso (400), poi
// l'esistenza della RFQ (404, con una lettura), poi il corpo (413) e la forma del corpo e della query (422); la
// transazione del core ricontrolla la RFQ sotto il suo lucchetto.
//
// I codici (EM §9): 400 un id del percorso non valido; 404 la RFQ non c'e' (o il flag e' spento); 409 BOZZA_VERSIONE,
// BASE_SUPERATA, ANTEPRIMA_SUPERATA, CHIAVE_RIUSATA (esito «conflitto»); 413; 422 CORPO_NON_VALIDO (con campo e motivo),
// FUORI_RFQ, BOZZA_ALTRO_PRODOTTO, FILE_NON_ATTIVO, MOTIVO_OBBLIGATORIO; 500 ogni altro errore, con un testo generico e il
// dettaglio nel log (5xx = «esito da verificare» per il ponte). Le letture (R1, R6, R7) non scrivono niente: R1 e R7 in una
// transazione REPEATABLE READ READ ONLY chiusa con ROLLBACK, R6 in una transazione che prende la riga della RFQ FOR KEY
// SHARE e non scrive (TestNessunaGetScriveDistintaV1).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// maxCorpoBozzaV1: il corpo JSON di una rotta della bozza (EM §3.10: 2 MiB + 4 KiB).
const maxCorpoBozzaV1 = 2<<20 + 4<<10

// I codici d'errore delle rotte della bozza che non vengono da distintabozza.
const (
	codiceTroppoGrandeV1 = "TROPPO_GRANDE"
	codiceErroreV1       = "ERRORE"
)

// registraBozzaMotoreA monta R1, R2, R3, R6, R7 e R11 con le loro righe gemelle (commento di testa).
func (s *Server) registraBozzaMotoreA(mux *http.ServeMux) {
	catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }
	gemella := func(ammessi string) http.HandlerFunc {
		return s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1(ammessi)))
	}
	mux.HandleFunc("GET /thread/{id}/motore-a/bozza", catena(s.bozzaV1Leggi))
	mux.HandleFunc("POST /thread/{id}/motore-a/bozza", catena(s.bozzaV1Salva))
	mux.HandleFunc("/thread/{id}/motore-a/bozza", gemella("GET, HEAD, POST"))
	mux.HandleFunc("POST /thread/{id}/motore-a/bozza/{bozza}/abbandona", catena(s.bozzaV1Abbandona))
	mux.HandleFunc("/thread/{id}/motore-a/bozza/{bozza}/abbandona", gemella("POST"))
	mux.HandleFunc("GET /thread/{id}/motore-a/intento/{chiave}", catena(s.bozzaV1Intento))
	mux.HandleFunc("/thread/{id}/motore-a/intento/{chiave}", gemella("GET, HEAD"))
	mux.HandleFunc("GET /thread/{id}/motore-a/conferme", catena(s.bozzaV1Storico))
	mux.HandleFunc("/thread/{id}/motore-a/conferme", gemella("GET, HEAD"))
	mux.HandleFunc("POST /thread/{id}/motore-a/riallinea", catena(s.bozzaV1Riallinea))
	mux.HandleFunc("/thread/{id}/motore-a/riallinea", gemella("POST"))
	// ondata C: R15, il rifiuto della proposta aggiornata (motore_a_proposta_v1.go)
	s.registraPropostaV1(mux)
	// direttiva 39: S8 e la revoca della sorgente (motore_a_sorgente_v1.go)
	s.registraSorgenteV1(mux)
	// direttiva 39: R4 formato 3, il riepilogo senza scritture (motore_a_bozza_v1_r4.go)
	s.registraRiepilogoV1(mux)
	// direttiva 39: R5 formato 3, la conferma atomica (motore_a_bozza_v1_r5.go)
	s.registraConfermaV1(mux)
}

// metodoNonAmmessoBozzaV1: un metodo non previsto, a flag acceso: 405 con Allow, senza leggere la sessione.
func metodoNonAmmessoBozzaV1(ammessi string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", ammessi)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// rifiutoBozzaV1 e' il corpo di un rifiuto (EM §9, «una sola forma»: {esito, codice, testo, …}).
type rifiutoBozzaV1 struct {
	Esito            string                        `json:"esito"`
	Codice           string                        `json:"codice"`
	Motivo           string                        `json:"motivo,omitempty"`
	Campo            string                        `json:"campo,omitempty"`
	Testo            string                        `json:"testo"`
	Motivi           []string                      `json:"motivi,omitempty"`
	BozzaID          *uuid.UUID                    `json:"bozza_id,omitempty"`
	StatoBozza       string                        `json:"stato_bozza,omitempty"`
	Versione         *int                          `json:"versione,omitempty"`
	AggiornataDa     *uuid.UUID                    `json:"aggiornata_da,omitempty"`
	AggiornataIl     *time.Time                    `json:"aggiornata_il,omitempty"`
	VersioneCorrente *confermabom.VersioneCorrente `json:"versione_corrente,omitempty"`
	AllegatoID       *uuid.UUID                    `json:"allegato_id,omitempty"`
	// il 409 ANTEPRIMA_SUPERATA delle forme che importano (EM §6.1, V-7; direttiva 35, C-3)
	Anteprima         string                         `json:"anteprima,omitempty"`
	ImprontaContenuto string                         `json:"impronta_contenuto,omitempty"`
	GruppoCorrente    *distintabozza.GruppoRitrovato `json:"gruppo_corrente,omitempty"`
	// S8 (G39-03): il prodotto a cui lo STEP e' gia' legato
	AltroProdotto *uuid.UUID `json:"altro_prodotto,omitempty"`
}

// rispostaErroreBozzaV1 traduce un errore di distintabozza nel codice HTTP e nel corpo (commento di testa). Pura: le
// prove L1 la chiamano con ogni codice (CodiciServerBozzaV1 e' l'elenco).
func rispostaErroreBozzaV1(err error) (int, rifiutoBozzaV1) {
	var e *distintabozza.Errore
	if errors.As(err, &e) && e != nil {
		r := rifiutoBozzaV1{Esito: "rifiutato", Codice: e.Codice, Motivo: e.Motivo, Campo: e.Campo, Testo: e.Messaggio, Motivi: e.Motivi,
			VersioneCorrente: e.VersioneCorrente, AllegatoID: e.AllegatoID, Anteprima: e.Anteprima, ImprontaContenuto: e.ImprontaContenuto,
			GruppoCorrente: e.GruppoCorrente, AltroProdotto: e.AltroProdotto}
		if c := e.Conflitto; c != nil {
			id, v, da, il := c.BozzaID, c.Versione, c.AggiornataDa, c.AggiornataIl
			r.BozzaID, r.StatoBozza, r.Versione, r.AggiornataDa, r.AggiornataIl = &id, c.Stato, &v, &da, &il
		}
		if stato, ok := statoDelCodiceV1[e.Codice]; ok {
			if stato == http.StatusConflict {
				r.Esito = "conflitto"
			}
			return stato, r
		}
		for _, c := range CodiciServerR2Forme {
			if c.Codice == e.Codice {
				return c.Stato, r
			}
		}
		for _, c := range CodiciServerS8 {
			if c.Codice == e.Codice {
				if c.Stato == http.StatusConflict {
					r.Esito = "conflitto"
				}
				return c.Stato, r
			}
		}
	}
	var rf fascicolo.Rifiuto
	if errors.As(err, &rf) {
		return http.StatusUnprocessableEntity, rifiutoBozzaV1{Esito: "rifiutato", Codice: distintabozza.CodiceFuoriRFQ, Testo: string(rf)}
	}
	return http.StatusInternalServerError, rifiutoBozzaV1{Esito: "errore", Codice: codiceErroreV1,
		Testo: "La richiesta non è riuscita: il dettaglio è nel log del server. L'esito è da verificare."}
}

// statoDelCodiceV1: lo stato HTTP di ogni codice delle rotte di questo file (EM §9; T-05: l'elenco unico dei codici del
// server per queste rotte e' CodiciServerBozzaV1).
var statoDelCodiceV1 = map[string]int{
	distintabozza.CodiceRFQInesistente:     http.StatusNotFound,
	distintabozza.CodiceBozzaVersione:      http.StatusConflict,
	distintabozza.CodiceBaseSuperata:       http.StatusConflict,
	distintabozza.CodiceChiaveRiusata:      http.StatusConflict,
	distintabozza.CodiceCorpoNonValido:     http.StatusUnprocessableEntity,
	distintabozza.CodiceFuoriRFQ:           http.StatusUnprocessableEntity,
	distintabozza.CodiceBozzaAltroProdotto: http.StatusUnprocessableEntity,
	distintabozza.CodiceFileNonAttivo:      http.StatusUnprocessableEntity,
	distintabozza.CodiceMotivoObbligatorio: http.StatusUnprocessableEntity,
	distintabozza.CodiceAnteprimaSuperata:  http.StatusConflict,
	// direttiva 35, C-3: le forme di R2 che importano (EM §6.1, U2 §2.8)
	distintabozza.CodicePropostaNonDisponibile:   http.StatusUnprocessableEntity,
	distintabozza.CodiceImportazioneTroppoGrande: http.StatusUnprocessableEntity,
	// direttiva 39: R5 formato 3 (EM §9.5): i blocchi e la bozza uguale alla base sono 422 (U2-P55)
	distintabozza.CodiceBlocchi:         http.StatusUnprocessableEntity,
	distintabozza.CodiceNessunaModifica: http.StatusUnprocessableEntity,
}

// CodiceRotta: un codice che una rotta della bozza puo' dare, con il suo stato HTTP.
type CodiceRotta struct {
	Rotta  string
	Codice string
	Stato  int
}

// CodiciServerBozzaV1 e' l'elenco unico dei codici che le rotte di questo file danno (T-05; R-4 della corsia interfaccia):
// la prova L1 lo confronta con i cataloghi del ponte (CONFLITTI per i 409, RIFIUTI per i 422) e con statoDelCodiceV1.
var CodiciServerBozzaV1 = []CodiceRotta{
	{"R1", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R1", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodiceBozzaVersione, http.StatusConflict},
	{"R2", distintabozza.CodiceBaseSuperata, http.StatusConflict},
	{"R2", distintabozza.CodiceChiaveRiusata, http.StatusConflict},
	{"R2", distintabozza.CodiceAnteprimaSuperata, http.StatusConflict},
	{"R2", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodiceBozzaAltroProdotto, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodiceFileNonAttivo, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodiceMotivoObbligatorio, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodicePropostaNonDisponibile, http.StatusUnprocessableEntity},
	{"R2", distintabozza.CodiceImportazioneTroppoGrande, http.StatusUnprocessableEntity},
	{"R3", distintabozza.CodiceBozzaVersione, http.StatusConflict},
	// R4 (EM §9.4: 422 CORPO_NON_VALIDO, FUORI_RFQ; 404 la RFQ): la bozza con i blocchi e' un 200
	{"R4", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	{"R4", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R3", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R3", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	// R5 (EM §9.5; motore_a_bozza_v1_r5.go): 409 e 422; i 500/503 non_registrato stanno in CodiciServerR5Altri
	{"R5", distintabozza.CodiceBaseSuperata, http.StatusConflict},
	{"R5", distintabozza.CodiceAnteprimaSuperata, http.StatusConflict},
	{"R5", distintabozza.CodiceChiaveRiusata, http.StatusConflict},
	{"R5", distintabozza.CodiceBozzaVersione, http.StatusConflict},
	{"R5", distintabozza.CodiceBlocchi, http.StatusUnprocessableEntity},
	{"R5", distintabozza.CodiceNessunaModifica, http.StatusUnprocessableEntity},
	{"R5", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	{"R5", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R6", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R7", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R7", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	// R11: un corpo che non e' vuoto ne' {} (PB-17); il ponte non ha ancora il catalogo di riallineaDocumenti (RQ-3): T-05 lo
	// confronta appena c'e'
	{"R11", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
}

// CodiciServerR2Forme: i codici delle forme di R2 costruite dal server che i cataloghi del ponte non hanno ancora
// (direttiva 35, C-3; U2 §2.8): FONTE_NON_CONFERMATA (422). Non stanno in CodiciServerBozzaV1, che T-05 confronta con
// RIFIUTI_R2 del ponte: richiesta alla corsia interfaccia di aggiungerlo, poi passa nell'elenco unico.
var CodiciServerR2Forme = []CodiceRotta{
	{"R2", distintabozza.CodiceFonteNonConfermata, http.StatusUnprocessableEntity},
	// G39-02 [U]: STEP_CANDIDATO_DEL_FINITO (422), richiesta alla corsia interfaccia di aggiungerlo a RIFIUTI_R2 del ponte
	{"R2", distintabozza.CodiceStepCandidatoDelFinito, http.StatusUnprocessableEntity},
}

// scriviJSONBozzaV1: JSON senza l'escape HTML di encoding/json (lo snapshot di R1 e' byte per byte quello salvato: «&»,
// «<», «>» di una descrizione non diventano \u00XX), con nosniff e no-store.
func (s *Server) scriviJSONBozzaV1(w http.ResponseWriter, r *http.Request, stato int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := enc.Encode(v); err != nil {
		if s.Log != nil {
			s.Log.Error("motore A, bozza V1: la risposta non si codifica in JSON", "percorso", r.URL.Path, "stato", stato, "err", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"esito":"errore","codice":"ERRORE","testo":"La risposta non si codifica: il dettaglio è nel log del server."}`)
		return
	}
	w.WriteHeader(stato)
	_, _ = w.Write(buf.Bytes())
}

// rispondiErroreBozzaV1 scrive la risposta d'errore; un 5xx va nel log con il dettaglio (mai al client).
func (s *Server) rispondiErroreBozzaV1(w http.ResponseWriter, r *http.Request, thread uuid.UUID, err error) {
	stato, corpo := rispostaErroreBozzaV1(err)
	if stato >= 500 && s.Log != nil {
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			s.Log.Info("motore A, bozza V1: la richiesta e' stata chiusa da chi l'aveva fatta", "percorso", r.URL.Path, "thread", thread, "err", err)
		} else {
			s.Log.Error("motore A, bozza V1: richiesta non riuscita", "percorso", r.URL.Path, "thread", thread, "err", err)
		}
	}
	s.scriviJSONBozzaV1(w, r, stato, corpo)
}

// rfqEsisteV1: la RFQ del percorso c'e' (una lettura fuori transazione: la transazione del core la ricontrolla sotto il
// lucchetto); altrimenti 404 RFQ_INESISTENTE, prima di leggere la forma della richiesta (EM §9, REV RS-11). ok falso:
// risposta gia' scritta.
func (s *Server) rfqEsisteV1(w http.ResponseWriter, r *http.Request, thread uuid.UUID) bool {
	if _, err := db.New(s.Pool).GetThread(r.Context(), thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = &distintabozza.Errore{Codice: distintabozza.CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return false
	}
	return true
}

// idDelPercorsoV1 legge un id del percorso: 400 se non e' un UUID (ok falso, risposta gia' scritta).
func (s *Server) idDelPercorsoV1(w http.ResponseWriter, r *http.Request, nome string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(nome))
	if err != nil {
		s.scriviJSONBozzaV1(w, r, http.StatusBadRequest, rifiutoBozzaV1{Esito: "rifiutato", Codice: distintabozza.CodiceCorpoNonValido,
			Campo: nome, Testo: nome + " non è un UUID"})
		return uuid.Nil, false
	}
	return id, true
}

// leggiCorpoV1 legge il corpo entro maxCorpoBozzaV1: oltre, 413 TROPPO_GRANDE (ok falso, risposta gia' scritta).
func (s *Server) leggiCorpoV1(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCorpoBozzaV1))
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			s.scriviJSONBozzaV1(w, r, http.StatusRequestEntityTooLarge, rifiutoBozzaV1{Esito: "rifiutato", Codice: codiceTroppoGrandeV1,
				Testo: "Il corpo supera 2 MiB + 4 KiB."})
			return nil, false
		}
		s.scriviJSONBozzaV1(w, r, http.StatusUnprocessableEntity, rifiutoBozzaV1{Esito: "rifiutato", Codice: distintabozza.CodiceCorpoNonValido,
			Campo: "richiesta", Testo: "Il corpo della richiesta non si legge."})
		return nil, false
	}
	return raw, true
}

// sola lettura: una transazione REPEATABLE READ READ ONLY, chiusa sempre con ROLLBACK.
func (s *Server) inSolaLetturaV1(ctx context.Context, f func(q *db.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sola lettura
	return f(db.New(tx))
}

// confrontoR1 e' la risposta di ?confronto=1 finche' il confronto con la proposta di oggi non c'e': lo calcola Confronta
// dell'adattatore del Motore A (passo 3, corsia motore), con la regola del gruppo corrente della sorgente (EM §6.4.1). Come
// un errore di calcolo (D-18): 200 con confronto.errore, nessuna scrittura.
type confrontoR1 struct {
	Errore string `json:"errore"`
}

// rispostaR1 e' la risposta di R1 (EM §9.1), con il confronto quando e' chiesto. ConfrontoDisponibile dice alla pagina se
// ?confronto=1 calcola il confronto con la proposta di oggi: finche' l'adattatore del Motore A non c'e' e' sempre false, e
// la pagina tiene spenti con il perche' «Aggiorna proposta…» e «Vedi le differenze» invece di offrirli e fallire
// (collaudo UI46, giro 3, F3-I1 [T]). Solo lettura: nessuna regola nuova, nessuna scrittura.
type rispostaR1 struct {
	distintabozza.LetturaR1
	ConfrontoDisponibile bool         `json:"confronto_disponibile"`
	Confronto            *confrontoR1 `json:"confronto,omitempty"`
}

// effettoR1 e' la variante ?effetto_cambio_tipo=<componente_id>&tipo_nuovo=<tipo> di R1 (PE-09): l'effetto del cambio di
// tipo (fascicolo.EffettoCambioTipo, la stessa anteprima della scheda) e la sua firma, che la decisione cambia_tipo
// porta in firma_effetto. Nessuna scrittura.
type effettoR1 struct {
	ComponenteID uuid.UUID     `json:"componente_id"`
	TipoNuovo    string        `json:"tipo_nuovo"`
	Effetto      effettoTipoV1 `json:"effetto"`
	FirmaEffetto string        `json:"firma_effetto"`
}

type opzioneTipoV1 struct {
	Tipo    string `json:"tipo"`
	Nome    string `json:"nome"`
	Spenta  string `json:"spenta,omitempty"`
	Attuale bool   `json:"attuale,omitempty"`
}

type effettoTipoV1 struct {
	Codice          string          `json:"codice"`
	TipoAttuale     string          `json:"tipo_attuale"`
	Spento          string          `json:"spento"` // DEC-B PD-05: sempre presente, vuoto quando il cambio si puo' fare
	Bottone         string          `json:"bottone,omitempty"`
	Opzioni         []opzioneTipoV1 `json:"opzioni"`
	Sospende        []string        `json:"sospende"`
	ConLei          []string        `json:"con_lei"`
	Rimozioni       int             `json:"rimozioni"`
	RestaSospesa    []string        `json:"resta_sospesa"`
	Autorizzabile   bool            `json:"autorizzabile"`
	StepStrutturale string          `json:"step_strutturale,omitempty"`
	SvuotaStep      string          `json:"svuota_step,omitempty"`
	Avvisi          []string        `json:"avvisi"`
}

func vuotoSeNil(xx []string) []string {
	if xx == nil {
		return []string{}
	}
	return xx
}

// bozzaV1Leggi: R1 (EM §9.1).
func (s *Server) bozzaV1Leggi(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	q := r.URL.Query()
	prodotto, err := uuid.Parse(q.Get("prodotto"))
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "prodotto",
			Messaggio: "manca il prodotto, o non è un UUID (?prodotto=…)"})
		return
	}
	if q.Has("ingressi") {
		s.bozzaV1Ingressi(w, r, thread, prodotto, q)
		return
	}
	if comp := q.Get("effetto_cambio_tipo"); comp != "" || q.Has("tipo_nuovo") {
		s.bozzaV1Effetto(w, r, thread, prodotto, comp, q.Get("tipo_nuovo"))
		return
	}
	op := distintabozza.OpzioniR1{Eventi: q.Get("eventi") == "1", Importazione: q.Get("importazione") == "1", Registro: q.Get("registro")}
	if op.Registro != "" && !reImprontaV1(op.Registro) {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "registro",
			Messaggio: "registro è l'impronta di un registro: 64 cifre esadecimali minuscole"})
		return
	}
	// G39-01, G39-02 [U] (distintabozza/adesso3.go): con ?suggerimenti_motore=1 il calcolo del motore di adesso, prima della
	// lettura; un motore che non calcola non e' un errore di R1 (200, suggerimenti_motore.errore)
	erroreMotore := ""
	if q.Has("suggerimenti_motore") {
		if q.Get("suggerimenti_motore") != "1" {
			s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "suggerimenti_motore",
				Messaggio: "suggerimenti_motore vale solo 1"})
			return
		}
		adesso, err := s.calcoloAdessoV1(r.Context(), thread, prodotto, uuid.Nil)
		if err != nil {
			if s.Log != nil && !errors.Is(err, motorea_web.ErrOccupato) && !errors.Is(err, motorea_web.ErrRegole) {
				s.Log.Warn("motore A, R1: suggerimenti del motore non calcolati", "thread", thread, "err", err)
			}
			erroreMotore = codiceErroreMotoreV1(err)
		}
		op.Adesso = adesso
	}
	var out rispostaR1
	err = s.inSolaLetturaV1(r.Context(), func(qq *db.Queries) error {
		l, err := distintabozza.LeggiR1(r.Context(), qq, thread, prodotto, op)
		out.LetturaR1 = l
		return err
	})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	if erroreMotore != "" {
		out.SuggerimentiMotore = &distintabozza.EsitoMotoreR1{Errore: erroreMotore}
	}
	if q.Get("confronto") == "1" && out.Stato == distintabozza.StatoAperta {
		out.Confronto = &confrontoR1{Errore: "Il confronto con la proposta di oggi non è ancora disponibile: arriva con l'adattatore del Motore A."}
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

func reImprontaV1(x string) bool {
	if len(x) != 64 {
		return false
	}
	for _, c := range x {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// bozzaV1Ingressi: la variante di R1 ?ingressi=1 (U2 §6.3.3): lo stato degli ingressi del riepilogo, solo database, senza
// motore e senza lucchetti, in una transazione REPEATABLE READ READ ONLY; risponde anche senza bozza. Una variante a se':
// ingressi vale solo «1» e non si combina con gli altri parametri di R1 (422 sul campo).
func (s *Server) bozzaV1Ingressi(w http.ResponseWriter, r *http.Request, thread, prodotto uuid.UUID, q url.Values) {
	if q.Get("ingressi") != "1" {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "ingressi",
			Messaggio: "ingressi vale 1"})
		return
	}
	for _, k := range []string{"eventi", "importazione", "registro", "confronto", "effetto_cambio_tipo", "tipo_nuovo"} {
		if q.Has(k) {
			s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: k,
				Messaggio: "?ingressi=1 e' una variante a se': non si combina con " + k})
			return
		}
	}
	var out distintabozza.Ingressi
	err := s.inSolaLetturaV1(r.Context(), func(qq *db.Queries) error {
		var err error
		out, err = distintabozza.LeggiIngressi(r.Context(), qq, thread, prodotto, improntaRegoleV1(s.MotoreA))
		return err
	})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// improntaRegoleV1: l'impronta delle regole caricate all'avvio (stato, versione e sha256 dell'indice, versione dei limiti),
// che entra nella parte proposta degli ingressi (U2 §6.3.3). Le regole non cambiano durante la vita del processo: un
// riavvio con altre regole cambia la parte.
func improntaRegoleV1(r motorea_web.Regole) string {
	return fmt.Sprintf("%s|%d|%s|%s", r.Stato, r.VersioneIndice, r.Sha256Indice, r.VersioneLimiti)
}

// bozzaV1Effetto: la variante di R1 con l'effetto del cambio di tipo (PE-09).
func (s *Server) bozzaV1Effetto(w http.ResponseWriter, r *http.Request, thread, prodotto uuid.UUID, comp, tipo string) {
	cid, err := uuid.Parse(comp)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "effetto_cambio_tipo",
			Messaggio: "effetto_cambio_tipo è l'UUID del componente"})
		return
	}
	t := db.TipoComponente(strings.TrimSpace(tipo))
	if !t.Valid() {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "tipo_nuovo",
			Messaggio: "tipo_nuovo è uno fra finito, sottoassieme, sciolto, commerciale"})
		return
	}
	var out effettoR1
	err = s.inSolaLetturaV1(r.Context(), func(qq *db.Queries) error {
		if _, err := qq.GetThread(r.Context(), thread); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &distintabozza.Errore{Codice: distintabozza.CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
			}
			return err
		}
		if p, err := qq.GetProdottoDellaRfq(r.Context(), db.GetProdottoDellaRfqParams{ThreadID: thread, ComponenteID: prodotto}); errors.Is(err, pgx.ErrNoRows) ||
			(err == nil && p.Tipo != db.TipoComponenteFinito) {
			return &distintabozza.Errore{Codice: distintabozza.CodiceFuoriRFQ, Campo: "prodotto", Messaggio: "il prodotto non è un finito di questa RFQ"}
		} else if err != nil {
			return err
		}
		e, err := fascicolo.EffettoCambioTipo(r.Context(), qq, thread, cid, t)
		if err != nil {
			return err
		}
		x := effettoTipoV1{Codice: e.Componente.Codice, TipoAttuale: string(e.Componente.Tipo), Spento: e.Spento, Opzioni: []opzioneTipoV1{},
			Sospende: vuotoSeNil(e.Sospende), ConLei: vuotoSeNil(e.ConLei), Rimozioni: e.Rimozioni, RestaSospesa: vuotoSeNil(e.RestaSospesa),
			Autorizzabile: e.Autorizzabile, StepStrutturale: e.StepStrutturale, SvuotaStep: e.SvuotaStep, Avvisi: vuotoSeNil(e.Avvisi)}
		if e.Spento == "" {
			x.Bottone = e.Bottone()
		}
		for _, o := range e.Opzioni {
			x.Opzioni = append(x.Opzioni, opzioneTipoV1{Tipo: string(o.Tipo), Nome: o.Nome, Spenta: o.Spenta, Attuale: o.Attuale})
		}
		out = effettoR1{ComponenteID: cid, TipoNuovo: string(t), Effetto: x, FirmaEffetto: e.Firma}
		return nil
	})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// bozzaV1Salva: R2 (EM §9.2), le cinque forme. La forma salvataggio: la transazione (lucchetto della RFQ, chiave, versione
// attesa, riconciliazione, verifiche, versione ed eventi) e' tutta in distintabozza.SalvaFormato3. Le forme costruite dal
// server (direttiva 35, C-3, R2 passo 6b): converti_da con distintabozza.Converti (conversione-1, EM §6.7), crea con
// distintabozza.CreaFormato3 (vuota, registrato, conferma; la partenza step con il calcolo del motore prima),
// riallinea_proposta con distintabozza.RiallineaFormato3 e cambia_sorgente con distintabozza.CambiaSorgenteFormato3 (con il
// calcolo del motore prima, motore_a_bozza_v1_forme.go).
func (s *Server) bozzaV1Salva(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	req, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	utente := utenteDa(r.Context()).UtenteID
	var out any
	switch req.Forma {
	case distintabozza.FormaSalvataggio:
		sv := req.Salvataggio
		sv.ThreadID, sv.UtenteID = thread, utente
		// G39-01, G39-02 [U] (distintabozza/adesso3.go): il calcolo del motore di adesso solo quando il salvataggio ne ha
		// bisogno, prima della transazione. Se serve (per il registro di un suggerimento accettato, o per una PB nuova o
		// spostata su uno STEP: i soli STEP candidati, senza importare) e il motore e' occupato, scaduto o in errore:
		// 503/504 (niente scritto, si riprova), perche' R2 non salvi come PB uno STEP che solo il motore riconosce candidato
		// del finito (G39-02 [U]; rilievo minore 4 della revisione G39 parte A). Senza le regole del motore (non configurate:
		// nessun suggerimento del calcolo puo' esserci) si prosegue senza il calcolo: il servizio decide (422
		// importazione_sconosciuta; per gli STEP il ripiego [T-39], la lettura legacy con il marcatore, che R4 con il motore e
		// R5 ricalcolano)
		var registro, step bool
		if err2 := s.inSolaLetturaV1(r.Context(), func(q *db.Queries) error {
			var e error
			registro, step, e = distintabozza.MotoreDelSalvataggio(r.Context(), q, sv)
			return e
		}); err2 != nil {
			s.rispondiErroreBozzaV1(w, r, thread, err2)
			return
		}
		switch {
		case registro:
			adesso, err2 := s.calcoloAdessoV1(r.Context(), thread, sv.ProdottoID, sv.BozzaID)
			if err2 != nil && !errors.Is(err2, motorea_web.ErrRegole) {
				s.rispondiErroreCalcoloV1(w, r, thread, err2)
				return
			}
			sv.Adesso = adesso
		case step:
			st, err2 := s.stepCandidatiV1(r.Context(), thread)
			if err2 != nil && !errors.Is(err2, motorea_web.ErrRegole) {
				s.rispondiErroreCalcoloV1(w, r, thread, err2)
				return
			}
			if st != nil {
				sv.Adesso = &distintabozza.CalcoloAdesso{Step: st}
			}
		}
		out, err = distintabozza.SalvaFormato3(r.Context(), s.Pool, sv)
	case distintabozza.FormaConvertiDa:
		out, err = distintabozza.Converti(r.Context(), s.Pool, req.Conversione3Da(thread, utente))
	case distintabozza.FormaCrea:
		var p *distintabozza.PropostaCalcolata
		if req.Crea.Partenza == distintabozza.PartenzaStep {
			// EM §6.3 passi 0-2: la chiave in sola lettura (un reinvio non calcola il motore), poi il calcolo e V-7
			gia, err2 := s.riproponiR2(r.Context(), thread, req.Comune)
			if err2 != nil {
				s.rispondiErroreBozzaV1(w, r, thread, err2)
				return
			}
			if !gia {
				if p, err2 = s.propostaPerCrea(r.Context(), thread, req); err2 != nil {
					s.rispondiErroreCalcoloV1(w, r, thread, err2)
					return
				}
			}
		}
		if req.Crea.Partenza == distintabozza.PartenzaConferma {
			// da-conferma-1 (G39-02 [U]; rilievo minore 5 della revisione G39 parte A): le PB iniziali della revisione, dove va
			// anche un file arrivato dopo R5 (G39-01), escludono lo STEP candidato del finito con il riconoscimento del motore,
			// come la partenza step; la chiave prima (un reinvio non calcola). Motore occupato, scaduto o in errore: 503/504,
			// niente scritto; regole non configurate: il ripiego [T-39]
			gia, err2 := s.riproponiR2(r.Context(), thread, req.Comune)
			if err2 != nil {
				s.rispondiErroreBozzaV1(w, r, thread, err2)
				return
			}
			if !gia {
				st, err2 := s.stepCandidatiV1(r.Context(), thread)
				if err2 != nil && !errors.Is(err2, motorea_web.ErrRegole) {
					s.rispondiErroreCalcoloV1(w, r, thread, err2)
					return
				}
				if st != nil {
					p = &distintabozza.PropostaCalcolata{StepCandidati: st}
				}
			}
		}
		out, err = distintabozza.CreaFormato3(r.Context(), s.Pool, req.CreaDaRichiesta(thread, utente, p))
	case distintabozza.FormaRiallineaProposta:
		var p *distintabozza.PropostaCalcolata
		gia, err2 := s.riproponiR2(r.Context(), thread, req.Comune)
		if err2 != nil {
			s.rispondiErroreBozzaV1(w, r, thread, err2)
			return
		}
		if !gia {
			if p, err2 = s.propostaPerRiallinea(r.Context(), thread, req); err2 != nil {
				s.rispondiErroreCalcoloV1(w, r, thread, err2)
				return
			}
		}
		out, err = distintabozza.RiallineaFormato3(r.Context(), s.Pool, req.RiallineaDaRichiesta(thread, utente, p))
	case distintabozza.FormaCambiaSorgente:
		var p *distintabozza.PropostaCalcolata
		gia, err2 := s.riproponiR2(r.Context(), thread, req.Comune)
		if err2 != nil {
			s.rispondiErroreBozzaV1(w, r, thread, err2)
			return
		}
		if !gia {
			if p, err2 = s.propostaPerCambia(r.Context(), thread, req); err2 != nil {
				s.rispondiErroreCalcoloV1(w, r, thread, err2)
				return
			}
		}
		out, err = distintabozza.CambiaSorgenteFormato3(r.Context(), s.Pool, req.CambiaSorgenteDaRichiesta(thread, utente, p))
	default:
		err = &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Motivo: distintabozza.MotivoFormaNonDisponibile, Campo: req.Forma,
			Messaggio: "la forma " + req.Forma + " di R2 non c'e'"}
	}
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// bozzaV1Abbandona: R3 (EM §9.3), per le bozze formato 2 e 3.
func (s *Server) bozzaV1Abbandona(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok {
		return
	}
	bozza, ok := s.idDelPercorsoV1(w, r, "bozza")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	a, err := distintabozza.LeggiRichiestaR3(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	a.ThreadID, a.BozzaID, a.UtenteID = thread, bozza, utenteDa(r.Context()).UtenteID
	out, err := distintabozza.Abbandona(r.Context(), s.Pool, a)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// bozzaV1Intento: R6 (EM §9.6).
func (s *Server) bozzaV1Intento(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok {
		return
	}
	chiave, ok := s.idDelPercorsoV1(w, r, "chiave")
	if !ok {
		return
	}
	out, err := distintabozza.LeggiIntento(r.Context(), s.Pool, thread, chiave)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// bozzaV1Storico: R7 (EM §9.7).
func (s *Server) bozzaV1Storico(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	q := r.URL.Query()
	prodotto, err := uuid.Parse(q.Get("prodotto"))
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "prodotto",
			Messaggio: "manca il prodotto, o non è un UUID (?prodotto=…)"})
		return
	}
	op := distintabozza.OpzioniR7{Importazione: q.Get("importazione") == "1"}
	if c := q.Get("conferma"); c != "" {
		id, err := uuid.Parse(c)
		if err != nil {
			s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "conferma",
				Messaggio: "conferma è un UUID"})
			return
		}
		op.Conferma = &id
	}
	var out distintabozza.StoricoR7
	err = s.inSolaLetturaV1(r.Context(), func(qq *db.Queries) error {
		var err error
		out, err = distintabozza.LeggiStorico(r.Context(), qq, thread, prodotto, op)
		return err
	})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// rispostaR11 e' la risposta di R11: lo stato di F8 dopo il riallineamento e quanti stati di F8 sono cambiati. Con F8
// non riuscito e' un 500 nella busta unica degli errori (EM §9, REV RS-08: {esito, codice, testo, …}), con f8_stato e
// aggiornati in piu'.
type rispostaR11 struct {
	Esito      string `json:"esito,omitempty"`
	Codice     string `json:"codice,omitempty"`
	Testo      string `json:"testo,omitempty"`
	F8Stato    string `json:"f8_stato"`
	Aggiornati int    `json:"aggiornati"`
}

// bozzaV1Riallinea: R11 (C3 §1.2; EM §9.8), invariato: rifa' F8 sull'intera RFQ e porta gli stati di F8 non eseguiti,
// creati prima del suo inizio, a eseguito (risolto_da = riallinea) o, se F8 non riesce, a errore. Non tocca la bozza ne'
// il registro. Il corpo, se c'e', e' vuoto ({}).
func (s *Server) bozzaV1Riallinea(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	if t := strings.TrimSpace(string(raw)); t != "" && t != "{}" {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: "richiesta",
			Messaggio: "R11 non ha un corpo (oppure {})"})
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	// l'inizio si legge dall'orologio del database, quello di f8_stato.creato_il: con l'orologio dell'applicazione uno
	// stato creato da una R5 concorrente dopo l'inizio potrebbe passare per «creato prima» (o il contrario)
	inizio, err := q.OraDelDatabase(ctx)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	errF8 := s.smistatore()(context.WithoutCancel(ctx), thread, fascicolo.Ambito{Rfq: true})
	righe, err := q.ListF8DaRiallineare(ctx, db.ListF8DaRiallineareParams{ThreadID: thread, PrimaDi: inizio})
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	out := rispostaR11{F8Stato: "eseguito"}
	for _, x := range righe {
		var n int64
		if errF8 == nil {
			n, err = q.SegnaF8Eseguito(ctx, db.SegnaF8EseguitoParams{F8ID: x.F8ID, ThreadID: thread})
		} else {
			n, err = q.SegnaF8Errore(ctx, db.SegnaF8ErroreParams{F8ID: x.F8ID, ThreadID: thread, Errore: pgtype.Text{String: testoErroreF8(errF8), Valid: true}})
		}
		if err != nil {
			s.rispondiErroreBozzaV1(w, r, thread, err)
			return
		}
		out.Aggiornati += int(n)
	}
	if errF8 != nil {
		if s.Log != nil {
			s.Log.Error("motore A, R11: F8 non riuscito", "thread", thread, "err", errF8)
		}
		s.scriviJSONBozzaV1(w, r, http.StatusInternalServerError, erroreR11(out.Aggiornati))
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// erroreR11: la risposta di R11 con F8 non riuscito (500), nella busta unica degli errori (EM §9, REV RS-08).
func erroreR11(aggiornati int) rispostaR11 {
	return rispostaR11{Esito: "errore", Codice: codiceErroreV1, F8Stato: "errore", Aggiornati: aggiornati,
		Testo: "Riallineamento dei documenti non riuscito: riprova. Il dettaglio è nel log del server."}
}

// testoErroreF8: il testo dell'errore di F8 per f8_stato.errore (ck_f8_errore: non vuoto, al piu' 2000 caratteri).
func testoErroreF8(err error) string {
	t := strings.TrimSpace(err.Error())
	if t == "" {
		t = "F8 non riuscito"
	}
	if r := []rune(t); len(r) > 2000 {
		t = string(r[:2000])
	}
	return t
}

// fileStep dice se un file e' uno STEP (estensione stp o step; senza estensione registrata, il nome): «Conferma la fonte»
// scrive la storia della fonte STEP del prodotto solo per un CAD 3D che e' uno STEP (EM §2.4; PM23 R-24; allegati.go).
func fileStep(estensione, nome string) bool {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(estensione), "."))
	if e == "" {
		if i := strings.LastIndex(nome, "."); i >= 0 {
			e = strings.ToLower(nome[i+1:])
		}
	}
	return e == "stp" || e == "step"
}
