package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermadoc"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/archivio"
	"promatec/cockpit/internal/platform/storage/staging"
)

// AllegatoUI è la riga della tabella allegati: FATTO (allegato) + INTERPRETAZIONE (proposta) + DECISIONE (documento).
type AllegatoUI struct {
	db.Allegato
	FileMancante bool                  // path_staging valorizzato ma file sparito dal disco
	Proposta     *db.DocumentoProposta // nil solo per inline/collegamenti
	PreSpunta    bool                  // suggerimento di download (dettagli.pre_spunta)
	Documento    *db.Documento         // confermato → sul NAS (o in coda)
	Figli        []AllegatoUI          // voci estratte da uno zip
}

// Scaricabile: allegato diretto di un messaggio Outlook, non ancora su disco (o file sparito).
func (a AllegatoUI) Scaricabile() bool {
	if a.Natura != db.NaturaAllegatoFile && a.Natura != db.NaturaAllegatoElementoOutlook {
		return false
	}
	if a.ContenitoreID.Valid {
		return false
	}
	return a.Stato == db.StatoAllegatoGrezzo || a.Stato == db.StatoAllegatoErrore || a.FileMancante
}

// Confermabile: file presente in staging con una proposta ancora aperta.
func (a AllegatoUI) Confermabile() bool {
	return a.Proposta != nil && a.Proposta.Stato == db.StatoPropostaAperta && a.PathStaging.Valid && !a.FileMancante &&
		(a.Stato == db.StatoAllegatoInStaging || a.Stato == db.StatoAllegatoAnalizzato) && !archivio.Estraibile(a.Estensione.String)
}

// Anteprimabile: un PDF che sta su questo server — in staging, oppure come documento scritto sul
// NAS (blocco 8, B8.1). Non promette che l'anteprima si aprira': il file puo' essere sparito fra
// questa riga e il clic, e chi risponde e' la rotta, che guarda i byte veri. Promette soltanto che
// vale la pena di offrire il pulsante, perche' nascondere un pulsante che funzionerebbe e mostrarne
// uno che non puo' funzionare sono lo stesso difetto visto da due lati.
func (a AllegatoUI) Anteprimabile() bool {
	if a.Natura == db.NaturaAllegatoInline || !strings.EqualFold(strings.TrimPrefix(a.Estensione.String, "."), "pdf") {
		return false
	}
	if a.PathStaging.Valid && a.PathStaging.String != "" && !a.FileMancante {
		return true
	}
	return a.Documento != nil && a.Documento.StatoNas == db.StatoNasScritto
}

// InCoda: download richiesto, in attesa del worker.
func (a AllegatoUI) InCoda() bool {
	return a.Stato == db.StatoAllegatoGrezzo && a.Errore.Valid && a.Errore.String == "in coda"
}

// allegatiUI costruisce l'albero degli allegati di un messaggio con proposte, documenti e stato del file.
func (s *Server) allegatiUI(ctx context.Context, q *db.Queries, messaggioID uuid.UUID) ([]AllegatoUI, error) {
	allegati, err := q.ListAllegatiMessaggio(ctx, messaggioID)
	if err != nil {
		return nil, err
	}
	proposte := map[uuid.UUID]db.DocumentoProposta{}
	if ps, err := q.ListProposteMessaggio(ctx, messaggioID); err == nil {
		for _, p := range ps {
			proposte[p.AllegatoID] = p
		}
	}
	docPerAllegato := map[uuid.UUID]db.Documento{}
	if ds, err := q.ListDocumentiMessaggio(ctx, messaggioID); err == nil {
		for _, d := range ds {
			if d.AllegatoID.Valid {
				docPerAllegato[d.AllegatoID.UUID] = d.Documento
			}
		}
	}
	costruisci := func(a db.Allegato) AllegatoUI {
		u := AllegatoUI{Allegato: a}
		if a.PathStaging.Valid && a.PathStaging.String != "" {
			if _, err := os.Stat(a.PathStaging.String); err != nil {
				u.FileMancante = true
			}
		}
		if p, ok := proposte[a.AllegatoID]; ok {
			p := p
			u.Proposta = &p
			var dett struct {
				PreSpunta bool `json:"pre_spunta"`
			}
			_ = json.Unmarshal(p.Dettagli, &dett)
			u.PreSpunta = dett.PreSpunta
		}
		if d, ok := docPerAllegato[a.AllegatoID]; ok {
			d := d
			u.Documento = &d
		}
		return u
	}
	var radici []AllegatoUI
	figli := map[uuid.UUID][]AllegatoUI{}
	for _, a := range allegati {
		if a.ContenitoreID.Valid {
			figli[a.ContenitoreID.UUID] = append(figli[a.ContenitoreID.UUID], costruisci(a))
		} else {
			radici = append(radici, costruisci(a))
		}
	}
	for i := range radici {
		radici[i].Figli = figli[radici[i].AllegatoID]
	}
	return radici, nil
}

// ---------------------------------------------------------------- download su richiesta

// scarica accoda lo staging degli allegati selezionati. Consentito solo dentro una RFQ: il file finirà
// nella cartella del thread e non deve mai esistere un download "senza destinazione".
func (s *Server) scarica(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	if !m.ThreadID.Valid {
		s.pannelloConAvviso(w, r, id, "Prima crea la RFQ o aggancia il messaggio a una RFQ esistente: i file scaricati vanno nella sua cartella.")
		return
	}
	c, err := s.copiaDownload(ctx, q, id, sessioneDa(ctx))
	if err != nil {
		http.Error(w, "messaggio non presente in nessuna casella attiva", 404)
		return
	}
	esiti, err := s.accodaDownload(ctx, q, m, c, r.Form["allegato_id"])
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, id, esiti.frase())
}

// contiDownload tiene separato ciò che è stato chiesto al worker da ciò che c'era già. Dire «3 download
// richiesti» quando i file erano già sul disco è una bugia piccola che però fa aspettare l'operatore.
type contiDownload struct{ accodati, gia, riusati, nonAttivi int }

func (c contiDownload) frase() string {
	var parti []string
	if c.accodati > 0 {
		parti = append(parti, fmt.Sprintf("%d download richiesti al worker Outlook", c.accodati))
	}
	if c.gia > 0 {
		parti = append(parti, fmt.Sprintf("%d già in staging", c.gia))
	}
	if c.riusati > 0 {
		parti = append(parti, fmt.Sprintf("%d riusati da un allegato con lo stesso contenuto", c.riusati))
	}
	// V1 (EM §10.7, G-8): i file in Altri file o scartati nella Distinta nuova non si scaricano, e si dice
	if c.nonAttivi > 0 {
		parti = append(parti, fmt.Sprintf("%d saltati perché in Altri file o scartati nella Distinta nuova", c.nonAttivi))
	}
	if len(parti) == 0 {
		return "Nessun allegato da scaricare."
	}
	return strings.Join(parti, ", ") + "."
}

// fraseSeCe e' la frase, preceduta da uno spazio, solo se qualcosa e' stato chiesto: dopo la creazione di una
// RFQ i file utili li prepara il sistema (B8.7b), e «nessun allegato da scaricare» direbbe il contrario.
func (c contiDownload) fraseSeCe() string {
	if c.accodati+c.gia+c.riusati+c.nonAttivi == 0 {
		return ""
	}
	return " " + c.frase()
}

// copiaDownload sceglie da quale copia scaricare: quella servita dalla postazione della sessione se
// c'è, altrimenti la copia di riferimento. Un download non apre finestre e non è legato al PC del
// richiedente: lo esegue qualunque worker autorizzato sulla casella (voce 2.6).
func (s *Server) copiaDownload(ctx context.Context, q *db.Queries, id uuid.UUID, sess sessioneUI) (coda.Copia, error) {
	var richiedente uuid.UUID
	if sess.Utente != nil {
		richiedente = sess.Utente.UtenteID
	}
	return coda.CopiaPerDownload(ctx, q, id, sess.Postazione, richiedente)
}

// accodaDownload accoda stage_allegato per gli id passati (solo allegati del messaggio, diretti, scaricabili).
// Non tutti diventano un job: la guardia della voce 1.11 sta dentro coda.AccodaStage, non qui, così vale
// per qualunque punto del server chieda un download.
func (s *Server) accodaDownload(ctx context.Context, q *db.Queries, m db.Messaggio, copia coda.Copia, ids []string) (contiDownload, error) {
	var c contiDownload
	for _, raw := range ids {
		aid, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		a, err := q.GetAllegato(ctx, aid)
		if err != nil || a.MessaggioID != m.MessaggioID || a.ContenitoreID.Valid || a.Natura == db.NaturaAllegatoInline {
			continue
		}
		// V1 (EM §10.7, G-8): un file in Altri file o scartato nella Distinta nuova non si scarica
		if stato, err := statoOccorrenza(ctx, q, a.AllegatoID); err != nil {
			return c, err
		} else if !filerfq.Attivo(stato) {
			c.nonAttivi++
			continue
		}
		esito, _, err := coda.AccodaStage(ctx, q, staging.FileStaging{}, a, m, copia, 1)
		if err != nil {
			return c, err
		}
		switch esito {
		case coda.StageGiaPresente:
			c.gia++
		case coda.StageRiusato:
			c.riusati++
		default: // accodato o già in coda: per l'operatore è la stessa attesa
			c.accodati++
		}
	}
	return c, nil
}

// riscarica rimette in coda un singolo allegato (file sparito dallo staging, errore del worker).
func (s *Server) riscarica(w http.ResponseWriter, r *http.Request) {
	aid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, aid)
	if err != nil {
		http.Error(w, "allegato non trovato", 404)
		return
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if !m.ThreadID.Valid {
		s.pannelloConAvviso(w, r, m.MessaggioID, "Prima crea o aggancia la RFQ.")
		return
	}
	// V1 (EM §10.7, G-9): un file scartato si ricarica, uno in Altri file si riattiva prima
	switch stato, err := statoOccorrenza(ctx, q, a.AllegatoID); {
	case err != nil:
		http.Error(w, err.Error(), 500)
		return
	case stato == filerfq.StatoScartato:
		s.pannelloConAvviso(w, r, m.MessaggioID, "Niente è cambiato: il file è stato scartato in questa RFQ. Per riaverlo, ricaricalo.")
		return
	case stato == filerfq.StatoAltriFile:
		s.pannelloConAvviso(w, r, m.MessaggioID, "Niente è cambiato: il file è in Altri file. Riattivalo prima.")
		return
	}
	c, err := s.copiaDownload(ctx, q, a.MessaggioID, sessioneDa(ctx))
	if err != nil {
		http.Error(w, "messaggio non presente in nessuna casella attiva", 404)
		return
	}
	esiti, err := s.accodaDownload(ctx, q, m, c, []string{aid.String()})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, m.MessaggioID, esiti.frase())
}

// ---------------------------------------------------------------- DECISIONE: conferma / scarta

// conferma trasforma una proposta in documento: calcola il percorso NAS dalla convenzione e accoda copia_nas.
func (s *Server) conferma(w http.ResponseWriter, r *http.Request) {
	pid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	// Prima la RFQ, poi la proposta: l'ordine dei lucchetti del congelamento e degli altri gesti sul
	// Fascicolo. Con la proposta bloccata prima e la RFQ chiesta dopo (dalla scrittura del documento,
	// che la prende FOR KEY SHARE), un congelamento nello stesso istante basta perche' le due
	// transazioni si aspettino a vicenda (40P01). La RFQ e' quella del messaggio dell'allegato: un
	// messaggio non agganciato non ne ha, e la conferma lo rifiuta piu' sotto.
	//
	// FOR KEY SHARE e non FOR UPDATE: e' il lucchetto che la scrittura prenderebbe comunque, preso prima.
	// Aspetta un congelamento o un gesto sulla struttura (FOR UPDATE), non un'altra conferma: due
	// conferme dello stesso file restano una corsa che decide il vincolo (thread, sha256).
	if prima, err := q.GetProposta(ctx, pid); err == nil {
		if a, err := q.GetAllegato(ctx, prima.AllegatoID); err == nil {
			if m, err := q.GetMessaggio(ctx, a.MessaggioID); err == nil && m.ThreadID.Valid {
				if _, err := tx.Exec(ctx, `SELECT 1 FROM thread_offerta WHERE thread_id = $1 FOR KEY SHARE`, m.ThreadID.UUID); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			}
		}
	}
	// BloccaProposta, non GetProposta: due conferme concorrenti sullo stesso allegato leggerebbero
	// entrambe stato='aperta' e creerebbero due documenti nel fascicolo, con lo stesso file copiato
	// due volte sul NAS. La seconda si ferma qui, poi rilegge e trova la proposta già decisa (T14).
	p, err := q.BloccaProposta(ctx, pid)
	if err != nil {
		http.Error(w, "proposta non trovata", 404)
		return
	}
	a, err := q.GetAllegato(ctx, p.AllegatoID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tipo := db.TipoDocumento(r.FormValue("tipo"))
	if !tipo.Valid() {
		tipo = p.TipoProposto
	}
	codice := strings.ToUpper(strings.TrimSpace(r.FormValue("codice")))
	rev := strings.ToUpper(strings.TrimSpace(r.FormValue("rev")))
	if r.Form.Has("codice") == false {
		codice, rev = p.Codice.String, p.Rev.String
	}
	// Il componente: quello scelto nel form, altrimenti quello a cui la proposta era gia' stata
	// assegnata. Nessuno dei due = il documento entra nel fascicolo senza componente (B8.3). Con una associazione
	// registrata attiva dell'allegato il componente deve essere il suo (INC-G9, in confermadoc.Conferma).
	comp := p.ComponenteID
	if v := strings.TrimSpace(r.FormValue("componente_id")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			http.Error(w, "componente non valido", 400)
			return
		}
		comp = uuid.NullUUID{UUID: id, Valid: true}
	}
	scelta, err := leggiScelta(r.FormValue("scelta"), r.FormValue("nuovo_riferimento"), r.FormValue("motivo"))
	if err != nil {
		s.pannelloConAvviso(w, r, m.MessaggioID, "Conferma non riuscita: "+spiegaErrore(err))
		return
	}
	msg, err := s.confermaProposta(ctx, q, u, p, a, m, tipo, codice, rev, strings.TrimSpace(r.FormValue("nota")),
		comp, r.FormValue("correggi_codice") == "1", scelta)
	if err != nil {
		s.pannelloConAvviso(w, r, m.MessaggioID, "Conferma non riuscita: "+spiegaErrore(err))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, m.MessaggioID, msg)
}

// confermaProposta crea il documento: e' confermadoc.Conferma (direttiva 35, blocco C-3, INC-G9: la logica e la guardia
// della associazione registrata sono nel core, che chiama anche S8), con la scelta della revisione delle pagine legacy.
// La chiamano «Conferma il documento», il lotto del Fascicolo e «Documentazione generale»; la frase e' quella di sempre.
func (s *Server) confermaProposta(ctx context.Context, q *db.Queries, u *db.Utente, p db.DocumentoProposta, a db.Allegato, m db.Messaggio,
	tipo db.TipoDocumento, codice, rev, nota string, comp uuid.NullUUID, correggi bool, scelta sceltaRevisione) (string, error) {
	es, err := confermadoc.Conferma(ctx, q, confermadoc.Richiesta{Utente: u.UtenteID, Proposta: p, Allegato: a, Messaggio: m, Tipo: tipo,
		Codice: codice, Rev: rev, Nota: nota, Componente: comp, Correggi: correggi, Scelta: scelta.core()})
	if err != nil {
		return "", err
	}
	return es.Messaggio, nil
}

// scarta chiude la proposta senza documento; se è rumore, l'hash viene ricordato per il dominio del mittente.
func (s *Server) scarta(w http.ResponseWriter, r *http.Request) {
	pid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	ctx := r.Context()
	// Una transazione sola: la decisione sulla proposta e la memoria del rumore sono la stessa scelta
	// dell'operatore, e non devono poter restare a metà (N19).
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	// C-6b, INC-G25: prima la RFQ (FOR KEY SHARE, come la conferma qui sopra e «Documentazione generale»), poi la
	// proposta: l'ordine RFQ → proposta → AR del servizio unico e di R5. Con la sola proposta bloccata, la chiusura della
	// AR chiesta dopo avrebbe rovesciato l'ordine rispetto a R5 (40P01). La RFQ e' quella del messaggio dell'allegato.
	if prima, err := q.GetProposta(ctx, pid); err == nil {
		if a, err := q.GetAllegato(ctx, prima.AllegatoID); err == nil {
			if m, err := q.GetMessaggio(ctx, a.MessaggioID); err == nil && m.ThreadID.Valid {
				if _, err := q.BloccaThreadCondiviso(ctx, m.ThreadID.UUID); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			}
		}
	}
	p, err := q.BloccaProposta(ctx, pid)
	if err != nil {
		http.Error(w, "proposta non trovata", 404)
		return
	}
	a, err := q.GetAllegato(ctx, p.AllegatoID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if p.Stato != db.StatoPropostaAperta {
		s.pannelloConAvviso(w, r, m.MessaggioID, "La proposta era già stata decisa: nessun cambiamento.")
		return
	}
	// V1 (EM §10.7): il vecchio «Scarta il file» su un file in Altri file nella Distinta nuova rifiuta (la proposta resta
	// aperta, e «Riattiva» non riaprirebbe una decisione)
	if stato, err := statoOccorrenza(ctx, q, a.AllegatoID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	} else if stato == filerfq.StatoAltriFile {
		s.pannelloConAvviso(w, r, m.MessaggioID, "Niente è cambiato: "+testoFileInAltriFile+".")
		return
	}
	// C-6b, INC-G10 (gesti_legacy_ar.go, D-AR03 D [U]): prima della decisione, nella RFQ interamente legacy la AR (o la
	// copia senza AR) si chiude dal servizio unico con scarto_legacy, cosi' la proposta scartata non lascia il componente;
	// nella RFQ formato 3 un file associato, con una copia o nella bozza si scarta dalla Sorgente con il gesto composto
	// «Scollega e scarta» (D-ASS-03 [U]), e qui si rifiuta senza scrivere
	if m.ThreadID.Valid {
		if err := chiusuraLegacy(ctx, q, m.ThreadID.UUID, u.UtenteID, p, a, associazioni.MotivoScartoLegacy,
			"si scarta dalla Sorgente con «Scollega e scarta», che prima salva lo svincolo nella bozza"); err != nil {
			s.pannelloConAvviso(w, r, m.MessaggioID, "Niente è cambiato: "+spiegaErrore(err))
			return
		}
	}
	if _, err := q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: pid, Stato: db.StatoPropostaScartata, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if r.FormValue("rumore") == "1" && a.Sha256.Valid && m.MittenteIndirizzo.Valid {
		if i := strings.LastIndex(m.MittenteIndirizzo.String, "@"); i >= 0 {
			if err := q.UpsertHashRumore(ctx, db.UpsertHashRumoreParams{Sha256: a.Sha256.String, Lower: m.MittenteIndirizzo.String[i+1:], ScartatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, m.MessaggioID, "Proposta scartata.")
}

// ---------------------------------------------------------------- helper

// pannelloConAvviso ri-renderizza il pannello del messaggio (o la pagina thread se richiesto) con un avviso in testa.
func (s *Server) pannelloConAvviso(w http.ResponseWriter, r *http.Request, messaggioID uuid.UUID, avviso string) {
	w.Header().Set("HX-Trigger", "inbox-aggiorna")
	if tid := r.FormValue("ritorna_thread"); tid != "" {
		if id, err := uuid.Parse(tid); err == nil {
			s.threadFrammento(w, r, id, avviso)
			return
		}
	}
	d, err := s.caricaMessaggio(r.Context(), messaggioID, sessioneDa(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	d.Avviso = avviso
	s.frammento(w, "messaggio_pannello", d)
}

func ptxt(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func regolaBool(regole json.RawMessage, chiave string, def bool) bool {
	var m map[string]any
	if json.Unmarshal(regole, &m) != nil {
		return def
	}
	if v, ok := m[chiave].(bool); ok {
		return v
	}
	return def
}
