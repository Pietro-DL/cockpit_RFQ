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

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
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
		(a.Stato == db.StatoAllegatoInStaging || a.Stato == db.StatoAllegatoAnalizzato) && strings.ToLower(a.Estensione.String) != "zip"
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

// fraseSeCe e' la frase, preceduta da uno spazio, solo se qualcosa e' stato chiesto: dopo la creazione di una
// RFQ i file utili li prepara il sistema (B8.7b), e «nessun allegato da scaricare» direbbe il contrario.
func (c contiDownload) fraseSeCe() string {
	if c.accodati+c.gia+c.riusati == 0 {
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
	// Il componente: quello scelto nel form, altrimenti quello a cui la proposta era gia' stata
	// assegnata. Nessuno dei due = il documento entra nel fascicolo senza componente (B8.3).
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

// confermaProposta crea il documento. Non crea componenti e non ne cerca uno per codice (B8.3): il
// documento si aggancia solo al componente comp, se c'e', e ne prende il codice lettera per lettera
// (A1.4, A2.2). Un codice diverso da quello del componente passa solo con correggi. Se il componente ha
// gia' un documento corrente dello stesso tipo serve la scelta: aggiungi, o sostituisce quale.
func (s *Server) confermaProposta(ctx context.Context, q *db.Queries, u *db.Utente, p db.DocumentoProposta, a db.Allegato, m db.Messaggio,
	tipo db.TipoDocumento, codice, rev, nota string, comp uuid.NullUUID, correggi bool, scelta sceltaRevisione) (string, error) {
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

	// stesso file già confermato nel thread → solo una provenienza in più; ma se quel documento è stato
	// sostituito, tornare ai suoi byte non è «già presente» (addendum A4.1, R2.6)
	if d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: t.ThreadID, Sha256: a.Sha256.String}); err == nil {
		if d.SostituitoDa.Valid {
			return "", identicoAUnaRevisioneSostituita(ctx, q, d)
		}
		_ = q.InsertProvenienza(ctx, db.InsertProvenienzaParams{DocumentoID: d.DocumentoID, AllegatoID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true},
			MessaggioID: uuid.NullUUID{UUID: m.MessaggioID, Valid: true}, RicevutoIl: a.RicevutoIl})
		_, _ = q.DecidiProposta(ctx, db.DecidiPropostaParams{PropostaID: p.PropostaID, Stato: db.StatoPropostaDuplicato, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}})
		return "File già presente nel fascicolo (" + d.PathRelativo + "): registrata la nuova provenienza.", nil
	}

	// Il componente scelto. Fino al B8.2 qui se ne creava uno per ogni codice nuovo, e lo si faceva
	// «finito» o «sciolto» guardando gli identificativi della richiesta: la struttura la decideva la
	// conferma di un allegato. Adesso la decide chi assegna, con un gesto suo.
	codiceTxt := ptxt(codice)
	assegnato := ""
	if comp.Valid {
		c, err := q.GetComponente(ctx, comp.UUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("il componente scelto non esiste")
		}
		if err != nil {
			return "", err
		}
		// il messaggio per l'operatore; chi impedisce l'aggancio e' la FK (thread, componente, codice)
		if c.ThreadID != t.ThreadID {
			return "", fmt.Errorf("il componente %s non è di questa RFQ", c.Codice)
		}
		if codice, err = codiceDaComponente(codice, c, correggi); err != nil {
			return "", err
		}
		codiceTxt = pgtype.Text{String: codice, Valid: true} // la stringa del componente, identica
		assegnato = c.Codice
		// Con la BOM congelata un file non si conferma gia' assegnato (D26): il database lo
		// rifiuterebbe comunque, qui lo si dice con le parole giuste e la proposta resta aperta.
		if n, bloccata, err := fascicolo.WorkingBloccata(ctx, q, t.ThreadID); err != nil {
			return "", err
		} else if bloccata {
			return "", rifiuto(fmt.Sprintf("la BOM è congelata nella V%d: il file entra senza componente, e lo si assegna aprendo una revisione", n))
		}
		correnti, err := correntiDelloStessoTipo(ctx, q, c.ComponenteID, tipo, uuid.Nil)
		if err != nil {
			return "", err
		}
		scelta.interno = a.Origine == db.OrigineAllegatoManuale
		if scelta, err = verificaScelta(a.NomeFile, c, tipo, correnti, scelta); err != nil {
			return "", err
		}
	}

	// Un file tecnico senza codice non diventa documento (addendum A2.2): il database lo rifiuterebbe
	// comunque (ck_documento_tecnico_ha_codice), ma con un errore che all'operatore non dice niente.
	// Qui gli si dice che cosa manca, e la proposta resta aperta.
	if documentoTecnico(tipo) && codice == "" {
		return "", errors.New("un CAD 3D, un disegno 2D o uno sviluppo DXF entra nel fascicolo solo con il codice del pezzo: scrivi il codice, il file resta fra le proposte")
	}
	// Il posto sul NAS (A4.1, D21, D22): la cartella del tipo e del codice, e un nome che per i tipi
	// tecnici e' <CODICE>_REV_<REV>, per gli altri l'originale sanificato. Il nome si sceglie sotto il
	// lucchetto della cartella, con il progressivo se e' gia' preso da un documento, da uno
	// spostamento o da un file rimasto sul NAS.
	cartella, err := documenti.CartellaDocumento(documenti.LayoutDocumento{Sottocartella: layout.Sottocartella, PerCodice: layout.PerCodice}, perCodice, codice)
	if err != nil {
		return "", fmt.Errorf("%s: %w", tipo, err)
	}
	pathRel, err := documenti.ScegliPercorso(ctx, q, t.ThreadID, cartella,
		documenti.NomeSulNas(tipo, codice, rev, a.Estensione.String, a.NomeFile))
	if err != nil {
		return "", err
	}
	// nome_file e' il nome ORIGINALE, com'e' arrivato (R1.6): il nome sul NAS vive solo in path_relativo.
	d, err := q.InsertDocumento(ctx, db.InsertDocumentoParams{
		ThreadID: t.ThreadID, ComponenteID: comp, Tipo: tipo, Codice: codiceTxt, Rev: ptxt(rev), NomeFile: a.NomeFile,
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
	componente := " Senza componente: codice e percorso restano questi finché non lo si assegna."
	if assegnato != "" {
		revisione, err := applicaScelta(ctx, q, t.ThreadID, d.DocumentoID, scelta)
		if err != nil {
			return "", err
		}
		componente = " Assegnato al componente " + assegnato + "." + revisione
	}
	if _, err := coda.AccodaCopia(ctx, q, d.DocumentoID); err != nil {
		// Con `nas_scrittura` spenta il documento si conferma lo stesso e resta `in_coda`: la decisione
		// dell'operatore è registrata, è la SCRITTURA sul NAS che aspetta (SH1). Dirgli «errore»
		// gliela farebbe rifare domani, e sarebbe due volte la stessa decisione.
		if errors.Is(err, coda.ErrCapacitaSpenta) {
			return "Confermato: " + pathRel + " — copia sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta." + componente, nil
		}
		return "", err
	}
	return "Confermato: " + pathRel + " (copia sul NAS in coda)." + componente, nil
}

// identicoAUnaRevisioneSostituita e' il rifiuto di A4.1 (R2.6): il file e' identico a un documento
// che e' stato sostituito. Tornare ai suoi byte non puo' essere una riga nuova (UNIQUE (thread,
// sha256)), e dire «gia' presente» farebbe credere che il fascicolo sia a posto mentre resta sulla
// revisione piu' nuova. Il ritorno al predecessore diretto si fa annullando la sostituzione; il ritorno
// piu' indietro non e' ancora gestito, e arriva come rifiuto, non come errore.
func identicoAUnaRevisioneSostituita(ctx context.Context, q *db.Queries, d db.Documento) error {
	storia, err := q.ListStoriaDocumento(ctx, d.DocumentoID)
	if err != nil {
		return err
	}
	var dopo []db.VDocumentoStoria
	trovato := false
	for _, s := range storia {
		if trovato {
			dopo = append(dopo, s)
		}
		if s.DocumentoID == d.DocumentoID {
			trovato = true
		}
	}
	switch len(dopo) {
	case 0:
		return rifiuto(fmt.Sprintf("il file è identico a %s, che risulta sostituito: ricarica la pagina", d.NomeFile))
	case 1:
		return rifiuto(fmt.Sprintf("il file è identico a %s, che %s ha sostituito: per tornare a %s si annulla quella sostituzione",
			d.NomeFile, dopo[0].NomeFile, d.NomeFile))
	}
	return rifiuto(fmt.Sprintf("il file è identico a %s, sostituito da %s e poi da %s: tornare a una revisione più vecchia della precedente non è ancora gestito",
		d.NomeFile, dopo[0].NomeFile, dopo[len(dopo)-1].NomeFile))
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

// documentoTecnico: i tipi che descrivono un pezzo, e che quindi esistono solo con il suo codice.
func documentoTecnico(tipo db.TipoDocumento) bool {
	return tipo == db.TipoDocumentoCad3d || tipo == db.TipoDocumentoDisegno2d || tipo == db.TipoDocumentoSviluppoDxf
}
