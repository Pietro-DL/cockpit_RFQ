package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/jobs"
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
		(a.Stato == db.StatoAllegatoInStaging || a.Stato == db.StatoAllegatoAnalizzato) && strings.ToLower(a.Estensione.String) != "zip"
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
	documenti := map[uuid.UUID]db.Documento{}
	if ds, err := q.ListDocumentiMessaggio(ctx, messaggioID); err == nil {
		for _, d := range ds {
			if d.AllegatoID.Valid {
				documenti[d.AllegatoID.UUID] = d.Documento
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
		if d, ok := documenti[a.AllegatoID]; ok {
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
type contiDownload struct{ accodati, gia, riusati int }

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
	if len(parti) == 0 {
		return "Nessun allegato da scaricare."
	}
	return strings.Join(parti, ", ") + "."
}

// copiaDownload sceglie da quale copia scaricare: quella servita dalla postazione della sessione se
// c'è, altrimenti la copia di riferimento. Un download non apre finestre e non è legato al PC del
// richiedente: lo esegue qualunque worker autorizzato sulla casella (voce 2.6).
func (s *Server) copiaDownload(ctx context.Context, q *db.Queries, id uuid.UUID, sess sessioneUI) (jobs.Copia, error) {
	var richiedente uuid.UUID
	if sess.Utente != nil {
		richiedente = sess.Utente.UtenteID
	}
	return jobs.CopiaPerDownload(ctx, q, id, sess.Postazione, richiedente)
}

// accodaDownload accoda stage_allegato per gli id passati (solo allegati del messaggio, diretti, scaricabili).
// Non tutti diventano un job: la guardia della voce 1.11 sta dentro jobs.AccodaStage, non qui, così vale
// per qualunque punto del server chieda un download.
func (s *Server) accodaDownload(ctx context.Context, q *db.Queries, m db.Messaggio, copia jobs.Copia, ids []string) (contiDownload, error) {
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
		esito, _, err := jobs.AccodaStage(ctx, q, jobs.FileStaging{}, a, m, copia, 1)
		if err != nil {
			return c, err
		}
		switch esito {
		case jobs.StageGiaPresente:
			c.gia++
		case jobs.StageRiusato:
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
	msg, err := s.confermaProposta(ctx, q, u, p, a, m, tipo, codice, rev, strings.TrimSpace(r.FormValue("nota")))
	if err != nil {
		s.pannelloConAvviso(w, r, m.MessaggioID, "Conferma non riuscita: "+err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, m.MessaggioID, msg)
}

func (s *Server) confermaProposta(ctx context.Context, q *db.Queries, u *db.Utente, p db.DocumentoProposta, a db.Allegato, m db.Messaggio,
	tipo db.TipoDocumento, codice, rev, nota string) (string, error) {
	if p.Stato != db.StatoPropostaAperta {
		return "", errors.New("proposta già decisa")
	}
	// `da_determinare` non è una destinazione (checkpoint 3R §5): non ha una sottocartella sul NAS,
	// perché «non so che cosa sia» non è una cartella. Il messaggio dice che cosa fare invece di
	// lasciare fallire una query su `cartella_documento` con un errore che non significa niente.
	if tipo == db.TipoDocumentoDaDeterminare {
		return "", errors.New("il tipo di questo file non è ancora stato determinato: scegli il tipo, oppure lascia che il worker-analisi lo legga")
	}
	if !m.ThreadID.Valid {
		return "", errors.New("il messaggio non è agganciato a una RFQ")
	}
	if !a.Sha256.Valid || !a.PathStaging.Valid {
		return "", errors.New("file non ancora scaricato")
	}
	if _, err := os.Stat(a.PathStaging.String); err != nil {
		return "", errors.New("file non presente in staging: usa Riscarica")
	}
	t, err := q.GetThread(ctx, m.ThreadID.UUID)
	if err != nil {
		return "", err
	}
	if !t.CartellaRelativa.Valid {
		return "", errors.New("thread senza cartella")
	}
	cl, err := q.GetCliente(ctx, t.ClienteID)
	if err != nil {
		return "", err
	}
	layout, err := q.GetCartellaDocumento(ctx, tipo)
	if err != nil {
		return "", fmt.Errorf("layout per %s: %w", tipo, err)
	}
	perCodice := regolaBool(cl.Regole, "cartella_per_codice", true)
	pathRel := domain.PathDocumento(domain.LayoutDocumento{Sottocartella: layout.Sottocartella, PerCodice: layout.PerCodice}, perCodice, codice, a.NomeFile)

	// stesso file già confermato nel thread → solo una provenienza in più
	if d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: t.ThreadID, Sha256: a.Sha256.String}); err == nil {
		_ = q.InsertProvenienza(ctx, db.InsertProvenienzaParams{DocumentoID: d.DocumentoID, AllegatoID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true},
			MessaggioID: uuid.NullUUID{UUID: m.MessaggioID, Valid: true}, RicevutoIl: a.RicevutoIl})
		_, _ = q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: p.PropostaID, Stato: db.StatoPropostaDuplicato, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}})
		return "File già presente nel fascicolo (" + d.PathRelativo + "): registrata la nuova provenienza.", nil
	}

	// componente: i file tecnici con codice si agganciano all'albero prodotto (creato se manca)
	var compID uuid.NullUUID
	if codice != "" && (tipo == db.TipoDocumentoCad3d || tipo == db.TipoDocumentoDisegno2d || tipo == db.TipoDocumentoSviluppoDxf) {
		c, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: t.ThreadID, Upper: codice})
		if errors.Is(err, pgx.ErrNoRows) {
			tipoComp := db.TipoComponenteSciolto
			if idents, _ := q.ListIdentificativi(ctx, t.ThreadID); contieneCodice(idents, codice) {
				tipoComp = db.TipoComponenteFinito
			}
			c, err = q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: t.ThreadID, Codice: codice, Qta: 1, Tipo: tipoComp,
				Origine: db.OrigineComponenteCodiceRilevato, ConfermatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}})
		}
		if err != nil {
			return "", fmt.Errorf("componente: %w", err)
		}
		compID = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
	}
	d, err := q.InsertDocumento(ctx, db.InsertDocumentoParams{
		ThreadID: t.ThreadID, ComponenteID: compID, Tipo: tipo, Codice: ptxt(codice), Rev: ptxt(rev), NomeFile: domain.NomeFileSicuro(a.NomeFile),
		Estensione: strings.ToLower(a.Estensione.String), Sha256: a.Sha256.String, Bytes: a.Bytes, PathRelativo: pathRel, ConfermatoDa: u.UtenteID, Nota: ptxt(nota),
	})
	if err != nil {
		return "", fmt.Errorf("documento: %w", err)
	}
	if err := q.InsertProvenienza(ctx, db.InsertProvenienzaParams{DocumentoID: d.DocumentoID, AllegatoID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true},
		MessaggioID: uuid.NullUUID{UUID: m.MessaggioID, Valid: true}, RicevutoIl: a.RicevutoIl}); err != nil {
		return "", err
	}
	if _, err := q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: p.PropostaID, Stato: db.StatoPropostaConfermata, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
		return "", err
	}
	if _, err := jobs.AccodaCopia(ctx, q, d.DocumentoID); err != nil {
		// Con `nas_scrittura` spenta il documento si conferma lo stesso e resta `in_coda`: la decisione
		// dell'operatore è registrata, è la SCRITTURA sul NAS che aspetta (SH1). Dirgli «errore»
		// gliela farebbe rifare domani, e sarebbe due volte la stessa decisione.
		if errors.Is(err, jobs.ErrCapacitaSpenta) {
			return "Confermato: " + pathRel + " — copia sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta.", nil
		}
		return "", err
	}
	return "Confermato: " + pathRel + " (copia sul NAS in coda).", nil
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

func contieneCodice(idents []db.IdentificativoThread, codice string) bool {
	for _, i := range idents {
		if strings.EqualFold(i.Codice, codice) {
			return true
		}
	}
	return false
}
