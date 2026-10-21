package web

// motore_a_anteprima_s6.go: S6 (EM §10.9 «S6»; direttiva 39 §3 [U + T-39]), corsia motore:
//   - S6 GET /thread/{id}/sorgente/step/{allegato}/anteprima?prodotto=…[&gruppo=…][&bozza=…]
//     «che cosa propone il motore se scelgo questo STEP», prima di S8.
// Montata da registraPropostaV1 (motore_a_proposta_v1.go), con la catena delle rotte della bozza: conMotoreA (flag spento =
// il 404 del router, prima della sessione) → conNoStore → soloAdmin → gestore; per gli altri metodi una riga gemella (405
// con Allow: GET, HEAD). Nessuna scrittura (TestNessunaGetScrive): niente bozza, fonte_step_evento, job o registro.
//
// L'ordine: gli id del percorso e della query (400) → la RFQ (404) → in una transazione REPEATABLE READ READ ONLY: lo STEP
// della RFQ (422 FUORI_RFQ), uno STEP (422 NON_STEP), un'occorrenza attiva (422 FILE_NON_ATTIVO), il prodotto finito non
// archiviato della RFQ (422 FUORI_RFQ), la bozza aperta formato 3 del prodotto se chiesta (422 FUORI_RFQ), con il suo
// snapshot e il registro della versione → il calcolo del Motore A in sola lettura con il semaforo (503/504 come le rotte
// della bozza) → il contenuto (propostabom.AnteprimaDelloSTEP, lo stesso adattatore della partenza step: la stessa
// impronta_contenuto che R2 e S8 ricalcoleranno).
//
// Direttiva 39 §3: struttura_step distingue lo STEP con struttura, la sola radice riconosciuta (il motore non ha
// ricostruito figli: mai mono_pezzo dedotto), nessuna struttura importabile (la partenza offerta è solo_prodotto) e
// l'analisi in corso o fallita (non_determinabile con lo stato dell'analisi, mai una prova geometrica, presa d'atto
// richiesta per procedere). Lo stato è in_analisi con l'analisi in corso e non_scaricato per uno STEP non ancora
// scaricato. Con ?bozza= l'effetto del cambio di fonte si calcola come lo calcola R2 cambia_sorgente (e S8 passo 3, che
// lo chiama) sulla stessa bozza, in sola lettura: la vista della bozza di distintabozza.VistaDellaBozza con la radice
// della sorgente della versione (il registro, altrimenti la casella del nodo del prodotto) e
// propostabom.EffettoDelCambioFonte (cambio-fonte-1). Per una bozza senza struttura (distintabozza.SenzaStruttura:
// nessuna sorgente, vuota o manuale, o la riservata solo_prodotto, che per l'effetto vale come assenza: la vista senza
// sorgente) l'effetto è quello di cambio-fonte-2 (direttiva 39 §3 [U + T-39]; rilievo della revisione): dopo
// EffettoDelCambioFonte, distintabozza.RiconciliaEffetto, la stessa funzione di R2, che abbina i nodi senza casella alle
// voci con identità sostenuta (nessun doppione manuale + STEP), trattiene i casi non certi in abbinamenti_proposti[]
// (fuori da da_rivedere[]) e mette fra le differenze archi e quantità discordanti. effetto_sulla_bozza porta allora
// l'algoritmo, gli abbinati, gli abbinamenti_proposti[], le caselle acquisite in caselle_riscritte[] con allegato_prima
// null e le identità dei nodi manuali abbinati che passano al modo proposta, come la risposta di R2.
//
// La revisione con un secondo STEP (giro G39, aperto (a) del verificatore del 20/10 [T-39]): una bozza con una base
// confermata (da-conferma-1, versione_bom_base almeno 1) che guarda un altro STEP si prepara con
// distintabozza.PreparaRevisione, la stessa funzione che Cambia (R2 cambia_sorgente, e S8 passo 3 che lo chiama) usa
// prima di propostabom.EffettoDelCambioFonte: i nodi registrati della fonte di prima che il registro nuovo non ha per
// rif perdono la casella e si riconciliano per identità con distintabozza.RiconciliaEffetto (cambio-fonte-2). Senza,
// l'anteprima mostrava uscite e doppioni che S8 non scrive. Le caselle acquisite da quei nodi dicono l'occorrenza di
// prima (allegato_prima), come dopoLaRevisione nella versione scritta; ricollegati[] elenca le loro chiavi (il conteggio
// registrati_della_fonte_precedente dell'evento di R2).
//
// [T-39] Limiti dichiarati: proposta.stato_di_partenza non c'è (P-1…P-8 della corsia persistenza, non esposti); le PB
// iniziali e i suggerimenti che R2 scriverebbe (preassociazioni_iniziali[], preassociazioni_proposte[]) non sono
// nell'anteprima (vogliono la versione scritta); un errore del calcolo diverso da occupato, scaduto e regole non pronte è
// un 5xx come nelle rotte della bozza (non il 200 con «errore» di D-18).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/confermadoc"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/rfq/filerfq"
	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
	"promatec/cockpit/internal/platform/db"
)

// I codici propri di S6 (gli altri sono quelli delle rotte della bozza).
const codiceNonStepS6 = "NON_STEP"

// CodiciServerS6: i codici di S6 con il loro stato HTTP (EM §10.9 «S6», Errori).
var CodiciServerS6 = []CodiceRotta{
	{"S6", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"S6", codiceNonStepS6, http.StatusUnprocessableEntity},
	{"S6", distintabozza.CodiceFileNonAttivo, http.StatusUnprocessableEntity},
	{"S6", codiceCalcoloOccupato, http.StatusServiceUnavailable},
	{"S6", codiceNonConfigurato, http.StatusServiceUnavailable},
	{"S6", codiceCalcoloScaduto, http.StatusGatewayTimeout},
}

// RispostaS6: il contenuto dell'anteprima e il testo per la pagina. EffettoSullaBozza (nella forma di S6, con la
// riconciliazione) prende il posto di quello di propostabom.Anteprima nel JSON.
type RispostaS6 struct {
	propostabom.Anteprima
	EffettoSullaBozza *EffettoS6 `json:"effetto_sulla_bozza,omitempty"`
	Testo             string     `json:"testo"`
}

// EffettoS6: effetto_sulla_bozza di S6, gli elenchi di R2 cambia_sorgente (commento di testa). Algoritmo: cambio-fonte-1
// per una bozza con struttura, cambio-fonte-2 per una senza. CaselleRiscritte prende il posto di quelle di
// cambio-fonte-1 nel JSON, con allegato_prima null per le caselle acquisite dai nodi abbinati e dal nodo del prodotto.
type EffettoS6 struct {
	propostabom.EffettoCambioFonte
	Algoritmo           string                               `json:"algoritmo"`
	CaselleRiscritte    []distintabozza.CasellaRiscritta3    `json:"caselle_riscritte,omitempty"`
	Abbinati            []distintabozza.Abbinamento3         `json:"abbinati,omitempty"`
	AbbinamentiProposti []distintabozza.AbbinamentoProposto3 `json:"abbinamenti_proposti,omitempty"`
	// Ricollegati: nella revisione con un secondo STEP, i nodi registrati della fonte di prima che si riconciliano per
	// identità (distintabozza.PreparaRevisione; EsitoCambioFonte.Ricollegati di Cambia)
	Ricollegati []string `json:"ricollegati,omitempty"`
	// Sostituzione (giro G39-ter, G39-03 [U]): la bozza aperta è nata da un'altra sorgente S8 prima di R5, e S8 di questo
	// STEP la sostituisce invece di cambiarne la fonte (sorgente.SostituzioneDellaBozza, la stessa regola di S8): gli elenchi
	// qui sopra non sono quello che S8 scriverà
	Sostituzione *sorgentev1.AnteprimaSostituzione `json:"sostituzione,omitempty"`
}

// preparazioneS6: la vista della bozza da cui si calcola l'effetto e, nella revisione con un secondo STEP, la versione
// preparata da distintabozza.PreparaRevisione con le chiavi dei nodi ricollegati.
type preparazioneS6 struct {
	vista       propostabom.VistaBozza
	prima       *distintabozza.Snapshot3
	revisione   bool
	ricollegati []string
}

// preparaS6: come Cambia prima di propostabom.EffettoDelCambioFonte (commento di testa).
func preparaS6(let letturaS6, c importazione.Contenuto) preparazioneS6 {
	out := preparazioneS6{vista: *let.bozza, prima: let.snapshot}
	if let.snapshot == nil || let.senzaStruttura {
		return out
	}
	prep, ok := distintabozza.PreparaRevisione(*let.snapshot, c)
	if !ok {
		return out
	}
	conCasella := map[string]bool{}
	for _, n := range let.snapshot.Nodi {
		conCasella[n.ChiaveBozza] = len(n.Caselle) > 0
	}
	for _, n := range prep.Nodi {
		if conCasella[n.ChiaveBozza] && len(n.Caselle) == 0 {
			out.ricollegati = append(out.ricollegati, n.ChiaveBozza)
		}
	}
	sort.Strings(out.ricollegati)
	out.vista, out.prima, out.revisione = distintabozza.VistaDellaBozza(prep, let.radice), &prep, true
	return out
}

// effettoS6: l'effetto sulla bozza verso il registro c della proposta (nil senza proposta o senza bozza), calcolato
// come Cambia: la vista della bozza (preparata con distintabozza.PreparaRevisione nella revisione con un secondo STEP),
// propostabom.EffettoDelCambioFonte (cambio-fonte-1) e, per una bozza senza struttura o nella revisione,
// distintabozza.RiconciliaEffetto (cambio-fonte-2).
func effettoS6(let letturaS6, c *importazione.Contenuto) (*EffettoS6, error) {
	if let.bozza == nil || c == nil {
		return nil, nil
	}
	p := preparaS6(let, *c)
	return effettoS6Da(let, p, propostabom.EffettoDelCambioFonte(p.vista, let.registroBozza, *c), c)
}

// effettoS6Da: l'effetto della rotta dall'effetto di cambio-fonte-1: com'è per una bozza con struttura; riconciliato con
// distintabozza.RiconciliaEffetto per una bozza senza o nella revisione (cambio-fonte-2, la stessa funzione di R2).
func effettoS6Da(let letturaS6, p preparazioneS6, eff propostabom.EffettoCambioFonte, c *importazione.Contenuto) (*EffettoS6, error) {
	out := &EffettoS6{EffettoCambioFonte: eff, Algoritmo: distintabozza.AlgoritmoCambioFonte, CaselleRiscritte: caselleS6(eff.CaselleRiscritte)}
	if (!let.senzaStruttura && !p.revisione) || p.prima == nil || c == nil {
		return out, nil
	}
	prima := p.prima
	ric, err := distintabozza.RiconciliaEffetto(*prima, *c, eff)
	if err != nil {
		return nil, err
	}
	out.EffettoCambioFonte, out.Algoritmo = ric.Effetto, distintabozza.AlgoritmoCambioFonte2
	acquisite := ric.Acquisite
	if p.revisione {
		// come dopoLaRevisione: la casella acquisita da un nodo ricollegato dice l'occorrenza di prima
		ricollegato := map[string]bool{}
		for _, k := range p.ricollegati {
			ricollegato[k] = true
		}
		acquisite = append([]distintabozza.CasellaRiscritta3(nil), acquisite...)
		for i := range acquisite {
			if ricollegato[acquisite[i].ChiaveBozza] {
				src := *let.snapshot.Sorgente.AllegatoID
				acquisite[i].Prima = &src
			}
		}
		out.Ricollegati = p.ricollegati
	}
	out.CaselleRiscritte = append(caselleS6(ric.Effetto.CaselleRiscritte), acquisite...)
	out.Abbinati, out.AbbinamentiProposti = ric.Abbinati, ric.Proposti
	// il nodo manuale abbinato passa al modo proposta con l'identità della voce (Cambia: identita_aggiornate[])
	nodi := map[string]distintabozza.Nodo3{}
	for _, n := range prima.Nodi {
		nodi[n.ChiaveBozza] = n
	}
	for _, x := range ric.Abbinati {
		n := nodi[x.ChiaveBozza]
		if n.Identita.Modo != distintabozza.ModoManuale {
			continue
		}
		for _, v := range c.Proposta.Nodi {
			if v.Voce != x.Voce {
				continue
			}
			idv, ok, err := distintabozza.IdentitaDellaProposta(v)
			if err != nil {
				return nil, err
			}
			if ok {
				out.IdentitaAggiornate = append(out.IdentitaAggiornate, propostabom.IdentitaAggiornata{ChiaveBozza: x.ChiaveBozza,
					Prima: identitaBozzaS6(n.Identita), Dopo: identitaBozzaS6(idv)})
			}
		}
	}
	return out, nil
}

// caselleS6: le caselle riscritte di cambio-fonte-1 nella forma di R2 (allegato_prima valorizzato).
func caselleS6(xx []propostabom.CasellaRiscritta) []distintabozza.CasellaRiscritta3 {
	var out []distintabozza.CasellaRiscritta3
	for _, x := range xx {
		p := x.Prima
		out = append(out, distintabozza.CasellaRiscritta3{ChiaveBozza: x.ChiaveBozza, DecisioneID: x.DecisioneID, Rif: x.Rif, Prima: &p, Dopo: x.Dopo})
	}
	return out
}

func identitaBozzaS6(i distintabozza.Identita) propostabom.IdentitaBozza {
	return propostabom.IdentitaBozza{Emittente: i.Emittente, Namespace: i.Namespace, Codice: i.Codice, Rev: i.Rev, Modo: i.Modo, Decisione: i.Decisione}
}

// registraAnteprimaS6 monta S6 e la sua riga gemella.
func (s *Server) registraAnteprimaS6(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/sorgente/step/{allegato}/anteprima", s.conMotoreA(conNoStore(s.soloAdmin(s.anteprimaS6))))
	mux.HandleFunc("/thread/{id}/sorgente/step/{allegato}/anteprima", s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1("GET, HEAD"))))
}

// letturaS6: ciò che S6 legge prima del calcolo, nella transazione in sola lettura.
type letturaS6 struct {
	codiceProdotto string
	scaricato      bool
	bozza          *propostabom.VistaBozza
	registroBozza  *importazione.Contenuto
	// snapshot: la versione corrente della bozza; senzaStruttura: distintabozza.SenzaStruttura (cambio-fonte-2)
	snapshot       *distintabozza.Snapshot3
	senzaStruttura bool
	// radice: la radice della sorgente della versione (il registro, altrimenti la casella del nodo del prodotto)
	radice string
	// sostituzione: S8 di questo STEP sostituirebbe la bozza (sorgente.SostituzioneDellaBozza)
	sostituzione *sorgentev1.AnteprimaSostituzione
}

// anteprimaS6: S6 (commento di testa).
func (s *Server) anteprimaS6(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok {
		return
	}
	allegato, ok := s.idDelPercorsoV1(w, r, "allegato")
	if !ok {
		return
	}
	q := r.URL.Query()
	prodotto, err := uuid.Parse(q.Get("prodotto"))
	if err != nil {
		s.scriviJSONBozzaV1(w, r, http.StatusBadRequest, rifiutoBozzaV1{Esito: "rifiutato", Codice: distintabozza.CodiceCorpoNonValido, Campo: "prodotto",
			Testo: "prodotto è obbligatorio ed è un UUID"})
		return
	}
	var bozza *uuid.UUID
	if x := q.Get("bozza"); x != "" {
		b, err := uuid.Parse(x)
		if err != nil {
			s.scriviJSONBozzaV1(w, r, http.StatusBadRequest, rifiutoBozzaV1{Esito: "rifiutato", Codice: distintabozza.CodiceCorpoNonValido, Campo: "bozza",
				Testo: "bozza è un UUID"})
			return
		}
		bozza = &b
	}
	if !s.rfqEsisteV1(w, r, thread) {
		return
	}
	ctx := r.Context()
	var let letturaS6
	if err := s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		var err error
		let, err = leggiS6(ctx, q, thread, allegato, prodotto, bozza)
		return err
	}); err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		return
	}
	ris, err := s.servizioMotoreA().Calcola(ctx, thread)
	if err != nil {
		s.rispondiErroreCalcoloV1(w, r, thread, err)
		return
	}
	rif := ""
	for _, pv := range ris.EsitoThread.ProdottiValutati {
		if pv.ComponenteID != nil && *pv.ComponenteID == prodotto {
			rif = pv.Rif
		}
	}
	a := propostabom.AnteprimaDelloSTEP(propostabom.IngressoAnteprima{Esito: &ris.Esito, Thread: ris.EsitoThread, Regole: ris.Regole, Prodotto: rif,
		CodiceProdotto: let.codiceProdotto, Allegato: allegato, Gruppo: q.Get("gruppo"), NonAttivi: ris.V1.NonAttivi, Bozza: let.bozza,
		RegistroBozza: let.registroBozza, Foto: &ris.Foto})
	if a.ProdottoID == "" {
		a.ProdottoID = prodotto.String()
	}
	if !let.scaricato {
		a.Stato = propostabom.StatoAnteprimaNonScaricato
	}
	eff, err := effettoS6(let, a.RegistroDellaProposta())
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	if eff != nil {
		eff.Sostituzione = let.sostituzione
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, RispostaS6{Anteprima: a, EffettoSullaBozza: eff, Testo: testoS6(a)})
}

// testoS6: una frase per la pagina, dal caso della struttura e dallo stato.
func testoS6(a propostabom.Anteprima) string {
	switch {
	case a.Stato == propostabom.StatoAnteprimaNonScaricato:
		return "Lo STEP non è ancora scaricato: l'anteprima non è disponibile."
	case a.StrutturaSTEP.Testo != "":
		return a.StrutturaSTEP.Testo + "."
	case a.StrutturaSTEP.Struttura == propostabom.StrutturaConFigli:
		return fmt.Sprintf("Il motore propone una struttura con %d figli da questo STEP.", a.StrutturaSTEP.FigliEstratti)
	}
	return ""
}

// leggiS6: i controlli e le letture di S6 prima del calcolo (commento di testa), sulla transazione in sola lettura.
func leggiS6(ctx context.Context, q *db.Queries, thread, allegato, prodotto uuid.UUID, bozza *uuid.UUID) (letturaS6, error) {
	var out letturaS6
	fuori := func(campo, formato string, a ...any) error {
		return &distintabozza.Errore{Codice: distintabozza.CodiceFuoriRFQ, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
	}
	ar, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{AllegatoID: allegato, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("allegato", "il file non è di questa RFQ")
	}
	if err != nil {
		return out, err
	}
	a, err := q.GetAllegato(ctx, allegato)
	if err != nil {
		return out, err
	}
	if !confermadoc.FileStep(a.Estensione.String, a.NomeFile) {
		return out, &distintabozza.Errore{Codice: codiceNonStepS6, Campo: "allegato", Messaggio: a.NomeFile + " non è uno STEP (.stp, .step)"}
	}
	out.scaricato = ar.Sha256.Valid && strings.TrimSpace(ar.Sha256.String) != "" && a.PathStaging.Valid
	if st, err := q.StatoOccorrenzaAllegato(ctx, allegato); err == nil {
		if st.Stato != filerfq.StatoAttivo || st.ScartatoLegacy {
			return out, &distintabozza.Errore{Codice: distintabozza.CodiceFileNonAttivo, Campo: "allegato",
				Messaggio: fmt.Sprintf("lo STEP non è un file attivo della RFQ (%s)", st.Stato)}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	p, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: thread, ComponenteID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("prodotto", "il prodotto non è di questa RFQ")
	}
	if err != nil {
		return out, err
	}
	if p.Tipo != db.TipoComponenteFinito || p.ArchiviatoIl != nil {
		return out, fuori("prodotto", "%s non è un prodotto finito attivo della RFQ", p.Codice)
	}
	out.codiceProdotto = p.Codice
	if bozza == nil {
		return out, nil
	}
	bt, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: *bozza, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("bozza", "la bozza %s non è di questa RFQ", *bozza)
	}
	if err != nil {
		return out, err
	}
	if bt.ProdottoID != prodotto || bt.Stato != distintabozza.StatoAperta {
		return out, fuori("bozza", "la bozza %s non è la bozza aperta del prodotto", *bozza)
	}
	if int(bt.Formato) != distintabozza.FormatoSnapshot3 {
		return out, fuori("bozza", "la bozza %s non è nel formato 3: prima si converte", *bozza)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: bt.BozzaID, ThreadID: thread, Versione: bt.Versione})
	if err != nil {
		return out, err
	}
	letto, err := distintabozza.RileggiVersione(int(bt.Formato), v.Snapshot, v.Impronta)
	if err != nil {
		return out, err
	}
	if letto.Formato3 == nil {
		return out, fmt.Errorf("%w: la bozza %s non e' del formato 3", distintabozza.ErrSnapshotIncoerente, bt.BozzaID)
	}
	s3 := *letto.Formato3
	var reg *importazione.Registro
	if v.ImportazioneID.Valid {
		riga, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: v.ImportazioneID.UUID, ThreadID: thread})
		if err != nil {
			return out, err
		}
		x, _, err := distintabozza.RileggiImportazione(riga)
		if err != nil {
			return out, err
		}
		reg = &x
	}
	radice := ""
	if reg != nil {
		radice = reg.Contenuto.Gruppo.Radice
		c := reg.Contenuto
		out.registroBozza = &c
	}
	if radice == "" {
		for _, n := range s3.Nodi {
			if n.ComponenteID != nil && *n.ComponenteID == prodotto && len(n.Caselle) > 0 {
				radice = n.Caselle[0].Rif
			}
		}
	}
	vista := distintabozza.VistaDellaBozza(s3, radice)
	if distintabozza.SenzaStruttura(s3) {
		// come Cambia (R2 cambia_sorgente): la sorgente solo_prodotto non ha struttura e per l'effetto vale come assenza
		vista.Sorgente = propostabom.SorgenteBozza{}
		out.senzaStruttura = true
	}
	out.bozza, out.snapshot, out.radice = &vista, &s3, radice
	if out.scaricato {
		if out.sostituzione, err = sorgentev1.SostituzioneDellaBozza(ctx, q, thread, bt, ar.Sha256.String); err != nil {
			return out, err
		}
	}
	return out, nil
}
