package web

// «Conferma Fascicolo» e le decisioni di «Da verificare» (B8.7b).
//
// Il piano (core/rfq/fascicolo/piano.go) dice che cosa e' pronto: file con tipo, codice e componente senza
// conflitti, strutture di STEP senza nodi da decidere. Una conferma sola li porta nel fascicolo, in una
// transazione: prima la struttura (nascono i componenti), poi i documenti (ciascuno con la conferma di sempre,
// che sceglie il posto sul NAS e accoda la copia). Un solo rifiuto annulla tutto, e l'avviso dice quale file e
// perche'. Lo STEP strutturale non c'e' (Smistamento F5b, P26, U7): e' un'autorizzazione, che si da' con
// l'anteprima e la casella mai spuntata dalla scheda del componente o dal cassetto, anche quando lo STEP e' uno
// solo. La conferma del piano non autorizza niente.
//
// La conferma lavora sul piano di adesso, ricalcolato nella sua transazione con la RFQ bloccata. Giro 4, fase
// 4.1b (domanda 9b = A): «Conferma Fascicolo» prende una conferma scritta. Il bottone in fondo apre il riepilogo
// (il cassetto «Rivedi», una GET che non scrive): ogni file pronto con il componente, il tipo di documento e il
// percorso che ricevera' sul NAS, e la firma di tutto questo. La POST vuole quella firma e la conferma esplicita
// («Conferma e copia sul NAS»); ricalcola il riepilogo, e se non e' piu' quello visto non scrive niente e lo
// ridisegna. Porta nel fascicolo le voci lasciate spuntate, e ognuna deve essere ancora pronta; ogni documento
// deve prendere sul NAS il percorso che il riepilogo mostrava. Prima il bottone portava gia' dentro la firma del
// piano, e un clic confermava e copiava.
//
// Le ambiguita' vere si decidono una per una, con gesti piccoli: il tipo o il codice di un file
// (DecidiPropostaDocumento: fonte operatore, che una lettura dopo non riscrive), aggiungere il componente
// che manca, assegnare, confermare con la scelta aggiungi/sostituisce. Una voce decisa torna nel piano, e
// se e' pronta entra con la conferma successiva.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraConferma(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/fascicolo/avanzamento", s.autenticato(s.fascicoloAvanzamento))
	mux.HandleFunc("GET /thread/{id}/fascicolo/nas", s.autenticato(s.fascicoloNas))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nas/importa", s.autenticato(s.dopoICaricamenti(s.importaDalNas)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/conferma", s.autenticato(s.dopoIlGesto(s.confermaFascicolo)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/proposta/{pid}/decidi", s.autenticato(s.dopoIlGesto(s.decidiProposta)))
}

// selezione sono le voci del piano che la conferma deve portare nel fascicolo.
type selezione struct {
	file      map[uuid.UUID]bool // proposte dei documenti
	strutture map[uuid.UUID]bool // allegati STEP
}

func (s selezione) vuota() bool { return len(s.file)+len(s.strutture) == 0 }

// leggiSelezione legge le voci scelte dal form (voce, struttura). Il campo `strutturale` (la casella dello STEP
// strutturale di prima, nata spuntata) si rifiuta: lo STEP si autorizza con l'anteprima (Smistamento F5b).
func leggiSelezione(r *http.Request) (selezione, error) {
	sel := selezione{file: map[uuid.UUID]bool{}, strutture: map[uuid.UUID]bool{}}
	if len(r.Form["strutturale"]) > 0 {
		return sel, rifiuto("lo STEP strutturale non si conferma con il piano: si autorizza dalla scheda del componente, con l'anteprima")
	}
	for campo, m := range map[string]map[uuid.UUID]bool{"voce": sel.file, "struttura": sel.strutture} {
		ids, err := uuidDalForm(r.Form[campo])
		if err != nil {
			return sel, rifiuto("voce del piano non valida")
		}
		for _, id := range ids {
			m[id] = true
		}
	}
	return sel, nil
}

// verificaSelezione controlla le voci scelte sul piano di adesso: ognuna deve essere ancora pronta, e nel piano.
// Senza voci scelte la selezione e' tutto il pronto: la firma del riepilogo (o, prima della fase 4.1b, quella
// del piano) ha gia' detto che e' quello che l'operatore ha visto.
func verificaSelezione(p fascicolo.PianoFascicolo, sel selezione) (selezione, error) {
	if !sel.vuota() {
		for _, v := range p.File {
			if sel.file[v.Proposta] && v.Stato != fascicolo.VocePronta {
				return sel, rifiuto(v.Nome + ": non è più pronto (" + statoVoce(v.Stato, v.Domande) + "): ricontrolla il piano")
			}
		}
		for _, v := range p.Strutture {
			if sel.strutture[v.Allegato] && v.Stato != fascicolo.VocePronta {
				return sel, rifiuto("la struttura di " + v.Nome + " non è più pronta: ricontrolla il piano")
			}
		}
		if n := len(sel.file) + len(sel.strutture); n != contaSelezionabili(p, sel) {
			return sel, rifiuto("una voce scelta non è più nel piano: ricontrolla e conferma di nuovo")
		}
		return sel, nil
	}
	for _, v := range p.File {
		if v.Stato == fascicolo.VocePronta {
			sel.file[v.Proposta] = true
		}
	}
	for _, v := range p.Strutture {
		if v.Stato == fascicolo.VocePronta {
			sel.strutture[v.Allegato] = true
		}
	}
	return sel, nil
}

// contaSelezionabili conta quante voci scelte esistono davvero nel piano.
func contaSelezionabili(p fascicolo.PianoFascicolo, sel selezione) int {
	n := 0
	for _, v := range p.File {
		if sel.file[v.Proposta] {
			n++
		}
	}
	for _, v := range p.Strutture {
		if sel.strutture[v.Allegato] {
			n++
		}
	}
	return n
}

func statoVoce(s fascicolo.StatoVoce, dd []fascicolo.Domanda) string {
	switch s {
	case fascicolo.VoceAttesa:
		return "in preparazione"
	case fascicolo.VoceDecidere:
		if len(dd) > 0 {
			return dd[0].Testo
		}
		return "da decidere"
	}
	return string(s)
}

// ------------------------------------------------------------------ il riepilogo (giro 4, fase 4.1b)

// rigaRiepilogo e' un file nel riepilogo di «Conferma Fascicolo»: che cosa diventa e dove va sul NAS.
type rigaRiepilogo struct {
	Proposta   uuid.UUID
	Nome       string // il nome del file com'e' arrivato
	Tipo       db.TipoDocumento
	Codice     string // il codice del documento: quello del componente, quando va a un componente
	Rev        string
	Componente string // il codice del componente a cui va; "" = un documento della RFQ, senza componente
	DaStep     string // lo STEP dalla cui struttura il componente nasce nella stessa conferma
	Aggiunge   bool
	// Provenienza: il file con lo stesso contenuto, gia' nel fascicolo o prima di lui nel riepilogo. La conferma
	// registra solo la provenienza: nessun documento nuovo, nessuna copia; il percorso e' quello del primo.
	Provenienza string
	Percorso    string // il percorso sul NAS, dentro la cartella della RFQ
	Errore      string // perche' il file non ha un percorso: la conferma si rifiuterebbe
}

// riepilogoConferma e' quello che l'operatore ha davanti prima di «Conferma e copia sul NAS»: i file pronti, ognuno
// con il componente, il tipo di documento e il percorso che ricevera' sul NAS, e la firma di tutto questo. La GET
// che lo mostra e la POST che lo ricontrolla lo calcolano allo stesso modo (riepilogoDelPiano).
type riepilogoConferma struct {
	Cartella  string // la cartella della RFQ sul NAS
	File      []rigaRiepilogo
	Strutture []fascicolo.VoceStruttura // le strutture pronte (dal Fascicolo v3 nessuna: si confermano nell'editor)
	Firma     string
}

// percorsoNelRiepilogo e' il percorso sul NAS di un file con il codice che il documento avra'. presi sono i
// percorsi (in minuscolo) che il riepilogo ha gia' dato ai file prima di lui. Legge soltanto. Un rifiuto (o la
// cartella senza il codice) resta sulla riga del file; un altro errore ferma il riepilogo.
type percorsoNelRiepilogo func(v fascicolo.VoceFile, codice string, presi map[string]bool) (string, error)

// riepilogoDa e' il riepilogo del piano p, nell'ordine in cui applicaPiano porta i file nel fascicolo: il secondo
// file con lo stesso nome nella stessa cartella prende il progressivo, lo stesso contenuto arrivato due volte e'
// una provenienza del primo. Il percorso di ogni file lo chiede a percorso; il resto e' puro.
func riepilogoDa(p fascicolo.PianoFascicolo, cartella string, percorso percorsoNelRiepilogo) (riepilogoConferma, error) {
	r := riepilogoConferma{Cartella: cartella}
	for _, v := range p.Strutture {
		if v.Stato == fascicolo.VocePronta {
			r.Strutture = append(r.Strutture, v)
		}
	}
	presi := map[string]bool{}
	perContenuto := map[string]string{} // sha256 → il percorso che quel contenuto ha nel riepilogo
	for _, v := range p.File {
		if v.Stato != fascicolo.VocePronta {
			continue
		}
		f := rigaRiepilogo{Proposta: v.Proposta, Nome: v.Nome, Tipo: v.Tipo, Codice: v.Codice, Rev: v.Rev, Aggiunge: v.Aggiunge, Provenienza: v.Duplicato}
		// il codice del documento e' quello del componente, lettera per lettera (codiceDaComponente, come la conferma)
		switch {
		case v.Componente != nil:
			f.Componente = v.Componente.Codice
			if c, err := codiceDaComponente(v.Codice, *v.Componente, false); err != nil {
				f.Errore = spiegaErrore(err)
			} else {
				f.Codice = c
			}
		case v.DaStep != nil:
			f.Componente, f.DaStep, f.Codice = v.DaStep.Codice, v.DaStep.File, v.DaStep.Codice
		}
		if f.Errore == "" && f.Codice != "" && !classificazione.CodiceAmmissibile(f.Codice) {
			f.Errore = fmt.Sprintf("%s: il codice ha più di %d caratteri o caratteri non ammessi", f.Codice, classificazione.MaxCodice)
		}
		if f.Errore == "" {
			if primo, ok := perContenuto[v.Sha256]; ok && v.Duplicato != "" && v.Sha256 != "" {
				f.Percorso = primo
			} else if pp, err := percorso(v, f.Codice, presi); err != nil {
				var rf rifiuto
				if !errors.As(err, &rf) && !errors.Is(err, documenti.ErrCodiceMancante) {
					return r, err
				}
				f.Errore = spiegaErrore(err)
			} else {
				f.Percorso = pp
				if v.Duplicato == "" {
					presi[strings.ToLower(pp)] = true
				}
			}
			if _, ok := perContenuto[v.Sha256]; !ok && f.Percorso != "" && v.Sha256 != "" {
				perContenuto[v.Sha256] = f.Percorso
			}
		}
		r.File = append(r.File, f)
	}
	r.Firma = r.firmaCon(p.Firma())
	return r, nil
}

// firmaCon riassume il riepilogo: il piano (le voci pronte e le loro destinazioni), la cartella della RFQ e, file per
// file, il componente, il tipo e il percorso sul NAS. Chi conferma dice quale riepilogo ha visto: se nel frattempo
// cambia anche solo un percorso (un collega ha confermato un file con lo stesso nome), la conferma si ferma.
func (r riepilogoConferma) firmaCon(piano string) string {
	h := sha256.New()
	fmt.Fprintf(h, "piano:%s\ncartella:%s\n", piano, r.Cartella)
	for _, f := range r.File {
		fmt.Fprintf(h, "F|%s|%s|%s|%s|%s|%s|%t|%s|%s|%s\n", f.Proposta, f.Tipo, f.Codice, f.Rev, f.Componente, f.DaStep, f.Aggiunge,
			f.Provenienza, f.Percorso, f.Errore)
	}
	for _, s := range r.Strutture {
		fmt.Fprintf(h, "S|%s|%d|%d\n", s.Allegato, s.Nodi, s.Archi)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// riga e' la riga del riepilogo di una proposta; nil se il file non c'e'.
func (r riepilogoConferma) riga(proposta uuid.UUID) *rigaRiepilogo {
	for i := range r.File {
		if r.File[i].Proposta == proposta {
			return &r.File[i]
		}
	}
	return nil
}

// riepilogoDelPiano calcola il riepilogo di «Conferma Fascicolo» sul piano p con le regole della conferma: la
// cartella del tipo e del codice (con la regola del cliente), il nome sul NAS, il primo nome libero. Legge
// soltanto: nessun lucchetto, niente di riservato (documenti.PercorsoPrevisto).
func riepilogoDelPiano(ctx context.Context, q *db.Queries, thread uuid.UUID, p fascicolo.PianoFascicolo) (riepilogoConferma, error) {
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return riepilogoConferma{}, err
	}
	pp, err := nuoviPercorsi(ctx, q, thread)
	if err != nil {
		return riepilogoConferma{}, err
	}
	return riepilogoDa(p, t.CartellaRelativa.String, func(v fascicolo.VoceFile, codice string, presi map[string]bool) (string, error) {
		if !t.CartellaRelativa.Valid {
			return "", rifiuto("la RFQ non ha una cartella sul NAS")
		}
		if v.Duplicato != "" {
			// lo stesso contenuto e' gia' un documento: la conferma registra la provenienza, il file resta dov'e'
			d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: v.Sha256})
			if errors.Is(err, pgx.ErrNoRows) {
				return "", rifiuto("il documento con lo stesso contenuto (" + v.Duplicato + ") non è nel fascicolo")
			}
			if err != nil {
				return "", err
			}
			return d.PathRelativo, nil
		}
		cartella, err := pp.cartellaPer(ctx, q, v.Tipo, codice)
		if err != nil {
			return "", err
		}
		return documenti.PercorsoPrevisto(ctx, q, thread, cartella, documenti.NomeSulNas(v.Tipo, codice, v.Rev, v.Estensione, v.Nome), presi)
	})
}

// ------------------------------------------------------------------ la conferma

// fileSolo dice se la conferma e' il «✓ Conferma» di un file solo (il pannello della vista Documenti): una voce e
// nient'altro, niente firma, conferma, elenco del riepilogo o strutture. Tutto il resto e' «Conferma Fascicolo»,
// e vuole il riepilogo.
func fileSolo(f url.Values) bool {
	return len(f["voce"]) == 1 && len(f["struttura"]) == 0 && len(f["strutturale"]) == 0 && len(f["firma"]) == 0 &&
		len(f["conferma"]) == 0 && len(f["selezione"]) == 0
}

// confermaFascicolo: POST /thread/{id}/fascicolo/conferma. Due gesti sulla stessa rotta.
//
// «Conferma Fascicolo» (giro 4, fase 4.1b; domanda 9b = A): arriva dal riepilogo con `firma` (quella del riepilogo:
// i file, i componenti, i tipi e i percorsi sul NAS) e `conferma=1` (il bottone «Conferma e copia sul NAS»), piu' le
// voci lasciate spuntate (`voce`, `struttura`, con `selezione`; senza elenco, tutto il pronto). Il riepilogo si ricalcola qui, con la RFQ
// bloccata: senza la firma, senza la conferma o con la firma di un riepilogo che nel frattempo e' cambiato non si
// scrive niente, e la risposta ridisegna il riepilogo di adesso. Prima bastava la firma del piano, e il bottone in
// fondo alla pagina la portava gia' dentro.
//
// «✓ Conferma» di un file solo (fileSolo): un altro gesto, su un file che l'operatore ha davanti, e resta com'era.
func (s *Server) confermaFascicolo(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form non valido", 400)
		return
	}
	solo := fileSolo(r.Form)
	ctx := r.Context()
	u := utenteDa(ctx)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	msg, err := s.confermaDalRiepilogo(ctx, db.New(tx), thread, u, r, solo)
	if err == nil {
		err = tx.Commit(ctx)
	}
	stato := http.StatusOK
	if err != nil {
		_ = tx.Rollback(ctx)
		msg = "Niente è cambiato: " + spiegaErrore(err)
		stato = http.StatusInternalServerError
		var rf rifiuto
		if errors.As(err, &rf) {
			stato = http.StatusUnprocessableEntity
		}
	}
	s.rispostaConferma(w, r, thread, msg, stato, err != nil && !solo)
}

// confermaDalRiepilogo e' la conferma nella sua transazione: il piano di adesso e, per «Conferma Fascicolo», il
// riepilogo di adesso, con la firma e la conferma esplicita.
func (s *Server) confermaDalRiepilogo(ctx context.Context, q *db.Queries, thread uuid.UUID, u *db.Utente, r *http.Request, solo bool) (string, error) {
	if err := preparaGesto(ctx, q, thread); err != nil {
		return "", err
	}
	p, err := fascicolo.LeggiPianoFascicolo(ctx, q, thread)
	if err != nil {
		return "", err
	}
	sel, err := leggiSelezione(r)
	if err != nil {
		return "", err
	}
	var visto *riepilogoConferma
	if !solo {
		rp, err := riepilogoDelPiano(ctx, q, thread, p)
		if err != nil {
			return "", err
		}
		switch f := strings.TrimSpace(r.FormValue("firma")); {
		case f == "":
			return "", rifiuto("«Conferma Fascicolo» si conferma dal riepilogo, che dice per ogni file il componente, il tipo e il percorso sul NAS: eccolo. Si scrive solo con «Conferma e copia sul NAS»")
		case f != rp.Firma:
			return "", rifiuto("il riepilogo è cambiato mentre lo guardavi (un file analizzato, una decisione di un collega, un percorso sul NAS già preso): ricontrolla i file e i percorsi e conferma di nuovo")
		}
		if strings.TrimSpace(r.FormValue("conferma")) != "1" {
			return "", rifiuto("manca la conferma: i file entrano nel fascicolo, e la copia sul NAS parte, solo con «Conferma e copia sul NAS» nel riepilogo")
		}
		// il modulo del riepilogo dice che l'elenco e' quello spuntato: senza spunte non c'e' niente da confermare
		// (senza `selezione` la conferma e' tutto il pronto, che e' il riepilogo intero, firmato)
		if len(r.Form["selezione"]) > 0 && sel.vuota() {
			return "", rifiuto("nel riepilogo non è rimasto spuntato niente: non c'è niente da confermare")
		}
		visto = &rp
	}
	if sel, err = verificaSelezione(p, sel); err != nil {
		return "", err
	}
	if sel.vuota() {
		return "", rifiuto("nel piano non c'è niente di pronto da confermare")
	}
	return s.applicaPiano(ctx, q, thread, u, p, sel, visto)
}

// rispostaConferma risponde alla conferma. Con htmx dalla schermata del Fascicolo, come gli altri gesti: l'avviso e i
// pannelli fuori banda; un rifiuto di «Conferma Fascicolo» apre il cassetto del riepilogo (quello di adesso, con la
// firma nuova) e ne scrive lo stato nell'indirizzo. Senza htmx (il modulo mandato da un browser senza JavaScript) la
// pagina intera del Fascicolo, con l'esito nell'avviso e, dopo un rifiuto, il riepilogo aperto: 200 fatto, 422 un
// rifiuto, 500 un errore.
func (s *Server) rispostaConferma(w http.ResponseWriter, r *http.Request, thread uuid.UUID, msg string, stato int, riepilogo bool) {
	if r.Header.Get("HX-Request") == "true" {
		st, ok := dalFascicolo(r.Header, thread)
		if !ok || !riepilogo {
			s.threadFrammento(w, r, thread, msg)
			return
		}
		st.Cassetto = "piano"
		w.Header().Set("HX-Push-Url", "/thread/"+thread.String()+"/fascicolo"+st.Query())
		s.rispondiFascicolo(w, r, thread, st, msg)
		return
	}
	st := statoFascicolo{}
	if riepilogo {
		st.Cassetto = "piano"
	}
	d, err := s.caricaFascicolo(r.Context(), thread, st, utenteDa(r.Context()))
	if err != nil {
		s.Log.Warn("pagina della conferma senza htmx non letta", "rfq", thread, "err", err)
		http.Error(w, msg, stato)
		return
	}
	d.Avviso = msg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(stato)
	s.rendi(w, r, "fascicolo.html", "fasc_corpo", "Fascicolo", d)
}

// applicaPiano porta nel fascicolo le voci scelte: la struttura, poi i documenti. Con il riepilogo visto (visto,
// «Conferma Fascicolo» dalla fase 4.1b) ogni documento deve prendere sul NAS il percorso che il riepilogo
// mostrava; senza (il «✓ Conferma» di un file solo) il percorso lo dice l'avviso, come prima.
func (s *Server) applicaPiano(ctx context.Context, q *db.Queries, thread uuid.UUID, u *db.Utente, p fascicolo.PianoFascicolo, sel selezione, visto *riepilogoConferma) (string, error) {
	// le dipendenze: un file che va a un componente che nasce da uno STEP si conferma con quella struttura
	for _, v := range p.File {
		if sel.file[v.Proposta] && v.DaStep != nil && !sel.strutture[v.DaStep.Allegato] {
			return "", rifiuto(fmt.Sprintf("%s va a %s, che nasce dalla struttura di %s: si confermano insieme", v.Nome, v.DaStep.Codice, v.DaStep.File))
		}
	}

	var parti []string
	for _, v := range p.Strutture {
		if !sel.strutture[v.Allegato] {
			continue
		}
		msg, err := fascicolo.AccettaFile(ctx, q, thread, v.Allegato, u.UtenteID)
		if err != nil {
			return "", rifiuto("la struttura di " + v.Nome + ": " + spiegaErrore(err))
		}
		parti = append(parti, "struttura di "+v.Nome+": "+strings.TrimSuffix(strings.ToLower(msg[:1])+msg[1:], "."))
	}

	nDoc, nProv, attesa := 0, 0, false
	for _, v := range p.File {
		if !sel.file[v.Proposta] {
			continue
		}
		pr, err := q.BloccaProposta(ctx, v.Proposta)
		if err != nil {
			return "", err
		}
		a, err := q.GetAllegato(ctx, v.Allegato)
		if err != nil {
			return "", err
		}
		m, err := q.GetMessaggio(ctx, a.MessaggioID)
		if err != nil {
			return "", err
		}
		var comp uuid.NullUUID
		switch {
		case v.Componente != nil:
			comp = uuid.NullUUID{UUID: v.Componente.ComponenteID, Valid: true}
		case v.DaStep != nil:
			c, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: thread, Upper: v.DaStep.Codice})
			if errors.Is(err, pgx.ErrNoRows) {
				return "", rifiuto(fmt.Sprintf("%s: il componente %s non è nato dalla struttura di %s", v.Nome, v.DaStep.Codice, v.DaStep.File))
			}
			if err != nil {
				return "", err
			}
			comp = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
		}
		scelta := sceltaRevisione{}
		if v.Aggiunge {
			scelta.data = true // fogli arrivati insieme: si aggiungono l'uno all'altro
		}
		msg, err := s.confermaProposta(ctx, q, u, pr, a, m, v.Tipo, v.Codice, v.Rev, "", comp, false, scelta)
		if err != nil {
			return "", rifiuto(v.Nome + ": " + spiegaErrore(err))
		}
		if visto != nil {
			if err := percorsoComeVisto(ctx, q, thread, *visto, v.Proposta, a); err != nil {
				return "", err
			}
		}
		switch {
		case strings.HasPrefix(msg, "File già presente"):
			nProv++
		default:
			nDoc++
			attesa = attesa || strings.Contains(msg, "IN ATTESA")
		}
	}
	if nDoc > 0 {
		copia := "copie sul NAS in coda"
		if attesa {
			copia = "copie sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta"
		}
		parti = append(parti, fmt.Sprintf("%s (%s)", conta(nDoc, "documento", "documenti"), copia))
	}
	if nProv > 0 {
		parti = append(parti, conta(nProv, "file già nel fascicolo: registrata la provenienza", "file già nel fascicolo: registrate le provenienze"))
	}

	if len(parti) == 0 {
		return "", rifiuto("niente da confermare")
	}
	return "Fascicolo confermato: " + strings.Join(parti, "; ") + ".", nil
}

// percorsoComeVisto controlla, dopo la conferma di un file, che il suo documento stia sul NAS dove il riepilogo
// diceva. La firma lo garantisce per il riepilogo intero; con una voce tolta dalla conferma il file dopo di lei
// puo' prendere il nome che lei lasciava libero, e un percorso diverso da quello visto non e' la conferma scritta.
func percorsoComeVisto(ctx context.Context, q *db.Queries, thread uuid.UUID, visto riepilogoConferma, proposta uuid.UUID, a db.Allegato) error {
	riga := visto.riga(proposta)
	if riga == nil {
		return rifiuto(a.NomeFile + ": non è nel riepilogo: ricontrolla e conferma di nuovo")
	}
	d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: a.Sha256.String})
	if err != nil {
		return err
	}
	if d.PathRelativo != riga.Percorso {
		return rifiuto(fmt.Sprintf("%s: sul NAS andrebbe in %s, non in %s come diceva il riepilogo (un file tolto dalla conferma gli lascia il nome, o il percorso è appena stato preso): ricontrolla e conferma di nuovo",
			a.NomeFile, d.PathRelativo, riga.Percorso))
	}
	return nil
}

// decidiProposta: POST /thread/{id}/fascicolo/proposta/{pid}/decidi, campi `tipo`, `codice`, `rev`. E' la
// risposta a una domanda del piano su un file non ancora confermato (che cos'e', che codice ha): la proposta
// prende quei valori con fonte operatore, e una lettura che arriva dopo non la riscrive.
func (s *Server) decidiProposta(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		pid, err := idDa(r.PathValue("pid"), "proposta")
		if err != nil {
			return "", err
		}
		p, err := q.BloccaProposta(ctx, pid)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!p.ThreadID.Valid || p.ThreadID.UUID != thread)) {
			return "", rifiuto("la proposta non è di questa RFQ")
		}
		if err != nil {
			return "", err
		}
		a, err := q.GetAllegato(ctx, p.AllegatoID)
		if err != nil {
			return "", err
		}
		if p.Stato != db.StatoPropostaAperta {
			return "", rifiuto(a.NomeFile + ": la proposta è già decisa")
		}
		if p.ComponenteID.Valid {
			return "", rifiuto(a.NomeFile + ": è assegnato a un componente, e ne ha il codice: si sgancia prima di cambiarlo")
		}
		tipo := db.TipoDocumento(strings.TrimSpace(r.FormValue("tipo")))
		if tipo == "" {
			tipo = p.TipoProposto
		}
		if !tipo.Valid() || tipo == db.TipoDocumentoDaDeterminare || tipo == db.TipoDocumentoRumore {
			return "", rifiuto("scegli che cos'è il file (un file che non serve si scarta)")
		}
		codice := strings.ToUpper(strings.TrimSpace(r.FormValue("codice")))
		rev := strings.ToUpper(strings.TrimSpace(r.FormValue("rev")))
		if codice != "" && !classificazione.CodiceAmmissibile(codice) {
			return "", rifiuto(fmt.Sprintf("%s: il codice ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxCodice))
		}
		if rev != "" && !classificazione.RevAmmissibile(rev) {
			return "", rifiuto(fmt.Sprintf("%s: la revisione ha più di %d caratteri o caratteri non ammessi", rev, classificazione.MaxRev))
		}
		if documenti.Tecnico(tipo) && codice == "" {
			return "", rifiuto("un CAD 3D, un disegno 2D o uno sviluppo DXF entra nel fascicolo solo con il codice del pezzo")
		}
		if _, err := q.DecidiPropostaDocumento(ctx, db.DecidiPropostaDocumentoParams{PropostaID: pid, TipoProposto: tipo,
			Codice: ptxt(codice), Rev: ptxt(rev)}); err != nil {
			return "", err
		}
		msg := a.NomeFile + ": " + etichettaTipoDoc(string(tipo))
		if codice != "" {
			msg += ", codice " + codice
		}
		if rev != "" {
			msg += " rev " + rev
		}
		return msg + ". Il piano lo riprende: se è pronto entra con «Conferma Fascicolo».", nil
	})
}
