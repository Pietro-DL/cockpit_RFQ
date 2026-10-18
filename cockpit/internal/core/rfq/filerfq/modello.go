package filerfq

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// I testi degli stati della rimozione (EM §10.4, «Gli stati della rimozione e i testi esatti»). «File eliminato»
// compare solo con eseguita / rimosso (D23:222).
var (
	testoInAttesa = "Scarto registrato; rimozione dallo staging ancora da completare."
	motiviAttesa  = map[string]string{
		"da_eseguire":         "",
		"lavoro_in_corso":     " (analisi in corso su questo file)",
		"copia_nas_in_attesa": " (copia sul NAS in attesa)",
		"anomalia_nas":        " (segnalazione aperta sul NAS)",
		"file_giovane":        " (file appena scritto: si riprova fra qualche minuto)",
		"pin_transitorio":     " (un lavoro in corso usa ancora questo contenuto)",
	}
)

// TestoRimozione e' il testo di uno stato della rimozione, con il motivo.
func TestoRimozione(stato, motivo string) string {
	switch stato {
	case "in_attesa":
		return strings.TrimSuffix(testoInAttesa, ".") + motiviAttesa[motivo] + "."
	case "errore":
		if motivo == "fuori_cache" || motivo == "percorso_non_valido" {
			return "Scarto registrato; il file non è stato eliminato: sta fuori dalla cache dello staging. Serve una bonifica dell'amministratore."
		}
		return "Scarto registrato; rimozione dallo staging ancora da completare (il file è aperto da un altro programma: si riprova da solo)."
	case "eseguita":
		switch motivo {
		case "rimosso":
			return "File scartato ed eliminato dallo staging."
		case "bonificato":
			return "File scartato. La copia fuori dalla cache dello staging non c'è più: l'ha tolta la bonifica dell'amministratore."
		}
		return "File scartato. Nello staging non c'era una copia da eliminare."
	case "non_necessaria_condiviso":
		if motivo == "rfq_cambiata" {
			return "File scartato. Il file ora appartiene a un'altra RFQ: la sua copia nello staging resta."
		}
		return "File scartato. La copia nello staging resta perché la usano altri file con lo stesso contenuto."
	}
	return "Scarto registrato."
}

// ---------------------------------------------------------------- il modello di S1

// ModelloSorgente e' la risposta di S1 (EM §10.9; PE-06): i file della RFQ con il loro stato, i contatori, gli STEP e,
// con ?file=, il dettaglio di un file con le dipendenze di ogni occorrenza del gesto.
type ModelloSorgente struct {
	ThreadID       uuid.UUID      `json:"thread_id"`
	Firma          string         `json:"firma"`
	Configurazione Configurazione `json:"configurazione"`
	Contatori      Contatori      `json:"contatori"`
	File           []FileSorgente `json:"file"`
	Step           []StepSorgente `json:"step"`
	Dettaglio      *Dettaglio     `json:"dettaglio"`
}

type Configurazione struct {
	Conservazione bool `json:"conservazione"`
}

type Contatori struct {
	Attivi    int `json:"attivi"`
	AltriFile int `json:"altri_file"`
	Scartati  int `json:"scartati"`
	InAnalisi int `json:"in_analisi"`
}

// FileSorgente e' un file della RFQ in S1. In «byte» ogni campo e' uno stato (presente, assente, non_verificato),
// mai un percorso; path_interno e' il percorso dentro l'archivio o il segno «NAS: …» dell'allegato, con le barre «/».
type FileSorgente struct {
	AllegatoID     uuid.UUID          `json:"allegato_id"`
	PropostaID     *uuid.UUID         `json:"proposta_id"`
	NomeFile       string             `json:"nome_file"`
	Estensione     string             `json:"estensione"`
	Sha256         string             `json:"sha256"`
	Bytes          *int64             `json:"bytes"`
	Origine        string             `json:"origine"`
	Canale         string             `json:"canale"`
	MessaggioID    uuid.UUID          `json:"messaggio_id"`
	RicevutoIl     time.Time          `json:"ricevuto_il"`
	ContenitoreID  *uuid.UUID         `json:"contenitore_id"`
	PathInterno    string             `json:"path_interno"`
	Voci           []uuid.UUID        `json:"voci"`
	Stato          string             `json:"stato"`
	Versione       int32              `json:"versione"`
	ScartatoLegacy bool               `json:"scartato_legacy"`
	Proposta       *PropostaFile      `json:"proposta"`
	Documento      *DocumentoFile     `json:"documento"`
	Analisi        AnalisiFile        `json:"analisi"`
	Byte           ByteFile           `json:"byte"`
	Rimozione      *RimozioneFile     `json:"rimozione"`
	Conservazione  *ConservazioneFile `json:"conservazione"`
	Scarto         *TracciaGesto      `json:"scarto,omitempty"`
	AltriFile      *TracciaGesto      `json:"altri_file,omitempty"`
	Azioni         AzioniFile         `json:"azioni"`
}

type PropostaFile struct {
	Stato        string     `json:"stato"`
	TipoProposto string     `json:"tipo_proposto"`
	Codice       string     `json:"codice"`
	Rev          string     `json:"rev"`
	ComponenteID *uuid.UUID `json:"componente_id"`
}

// DocumentoFile: il documento confermato di cui il file e' provenienza (non nullo = confermato; PE-06).
type DocumentoFile struct {
	DocumentoID uuid.UUID `json:"documento_id"`
	Stato       string    `json:"stato"`
	Codice      string    `json:"codice"`
	Rev         string    `json:"rev"`
	Tipo        string    `json:"tipo"`
}

// AnalisiFile: lo stato dell'analisi (EM §10.9): non_scaricato, download_in_coda, in_staging, estrazione_in_coda,
// in_analisi, letto, errore (con il motivo). La pagina mostra «in analisi» per i quattro intermedi.
type AnalisiFile struct {
	Stato  string `json:"stato"`
	Motivo string `json:"motivo"`
}

// ByteFile: dove stanno i byte, in stati.
type ByteFile struct {
	Staging       string `json:"staging"`
	Conservazione string `json:"conservazione"`
	NAS           string `json:"nas"`
}

type RimozioneFile struct {
	Stato     string `json:"stato"`
	Motivo    string `json:"motivo"`
	Testo     string `json:"testo"`
	Tentativi int32  `json:"tentativi"`
}

// ConservazioneFile: l'ultima copia di conservazione. In errore (errore_copia, o un rilascio che non riesce) porta
// «stato: errore» con la causa o il motivo (PE-06).
type ConservazioneFile struct {
	Stato        string     `json:"stato"`
	Causa        string     `json:"causa,omitempty"`
	Motivo       string     `json:"motivo,omitempty"`
	VerificatoIl *time.Time `json:"verificato_il,omitempty"`
	Bytes        *int64     `json:"bytes,omitempty"`
}

// TracciaGesto: chi, quando e perche' (lo scarto, il passaggio in Altri file; PE-06, MAT FL-08/FL-09).
type TracciaGesto struct {
	Autore string    `json:"autore"`
	Quando time.Time `json:"quando"`
	Motivo string    `json:"motivo"`
}

// Azione: un'azione della Sorgente su un file, sempre visibile, spenta con il motivo (PE-06; MAT §6.1).
type Azione struct {
	Disponibile bool     `json:"disponibile"`
	Motivo      string   `json:"motivo"`
	Richiede    []string `json:"richiede,omitempty"`
}

type AzioniFile struct {
	Scarta    Azione `json:"scarta"`
	AltriFile Azione `json:"altri_file"`
	Riattiva  Azione `json:"riattiva"`
	Contenuto Azione `json:"contenuto"`
}

// StepSorgente: uno STEP attivo della RFQ (MAT SG-01): le proposte del motore per STEP le da' /dati.
type StepSorgente struct {
	AllegatoID uuid.UUID   `json:"allegato_id"`
	NomeFile   string      `json:"nome_file"`
	Analisi    AnalisiFile `json:"analisi"`
}

// Dettaglio: il file chiesto con ?file=, con le dipendenze di ogni occorrenza del gesto, l'impronta, il perimetro delle
// voci (solo per un archivio) e la storia.
type Dettaglio struct {
	AllegatoID         uuid.UUID       `json:"allegato_id"`
	Dipendenze         []Dipendenza    `json:"dipendenze"`
	ImprontaDipendenze string          `json:"impronta_dipendenze"`
	NVoci              *int            `json:"n_voci,omitempty"`
	ImprontaVoci       string          `json:"impronta_voci,omitempty"`
	Voci               []VoceDettaglio `json:"voci,omitempty"`
	Eventi             []EventoFile    `json:"eventi"`
}

type VoceDettaglio struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	NomeFile   string    `json:"nome_file"`
	Stato      string    `json:"stato"`
}

type EventoFile struct {
	Azione          string     `json:"azione"`
	StatoPrima      string     `json:"stato_prima,omitempty"`
	StatoDopo       string     `json:"stato_dopo,omitempty"`
	Motivo          string     `json:"motivo,omitempty"`
	Esito           any        `json:"esito,omitempty"`
	AltroAllegatoID *uuid.UUID `json:"altro_allegato_id,omitempty"`
	Origine         string     `json:"origine"`
	Autore          string     `json:"autore,omitempty"`
	Quando          time.Time  `json:"quando"`
}

// OpzioniModello: i parametri di S1.
type OpzioniModello struct {
	File     *uuid.UUID // ?file=
	Scartati bool       // ?scartati=1
}

// ErrRFQInesistente: la RFQ del percorso non c'e' (404).
var ErrRFQInesistente = errors.New("RFQ inesistente")

// Modello legge S1 in una transazione REPEATABLE READ READ ONLY, senza lucchetti (EM §10.9; giro 1, TR-08): le
// dipendenze del dettaglio si calcolano in questa fotografia, e il gesto le ricalcola sotto BloccaThread.
func (s *Servizio) Modello(ctx context.Context, thread uuid.UUID, op OpzioniModello) (*ModelloSorgente, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	t, err := q.GetThread(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRFQInesistente
	}
	if err != nil {
		return nil, err
	}
	unita := t.UnitoIn.Valid
	righe, err := q.ListFileSorgente(ctx, thread)
	if err != nil {
		return nil, err
	}
	lavori, err := q.ListLavoriPendentiSorgente(ctx, thread)
	if err != nil {
		return nil, err
	}
	pendenti := map[uuid.UUID]map[string]bool{}
	for _, l := range lavori {
		if pendenti[l.AllegatoID] == nil {
			pendenti[l.AllegatoID] = map[string]bool{}
		}
		pendenti[l.AllegatoID][l.Tipo] = true
	}
	figli := map[uuid.UUID][]uuid.UUID{}
	for _, r := range righe {
		if r.ContenitoreID.Valid {
			figli[r.ContenitoreID.UUID] = append(figli[r.ContenitoreID.UUID], r.AllegatoID)
		}
	}
	m := &ModelloSorgente{ThreadID: thread, Configurazione: Configurazione{Conservazione: s.Deposito != nil},
		File: []FileSorgente{}, Step: []StepSorgente{}}
	for _, r := range righe {
		f := s.fileDaRiga(r, pendenti[r.AllegatoID], figli[r.AllegatoID], unita)
		switch {
		case f.Stato == StatoScartato:
			m.Contatori.Scartati++
		case f.Stato == StatoAltriFile:
			m.Contatori.AltriFile++
		default:
			m.Contatori.Attivi++
			if inAnalisi(f.Analisi.Stato) {
				m.Contatori.InAnalisi++
			}
			if est := strings.ToLower(f.Estensione); est == "stp" || est == "step" {
				m.Step = append(m.Step, StepSorgente{AllegatoID: f.AllegatoID, NomeFile: f.NomeFile, Analisi: f.Analisi})
			}
		}
		if f.Stato == StatoScartato && !op.Scartati {
			continue
		}
		m.File = append(m.File, f)
	}
	firma, err := improntaDi(struct {
		C Contatori      `json:"c"`
		F []FileSorgente `json:"f"`
		S []StepSorgente `json:"s"`
	}{m.Contatori, m.File, m.Step})
	if err != nil {
		return nil, err
	}
	m.Firma = firma
	if op.File != nil {
		d, err := s.dettaglio(ctx, q, thread, *op.File)
		if err != nil {
			return nil, err
		}
		m.Dettaglio = d
	}
	return m, nil
}

// inAnalisi: i quattro stati intermedi che la pagina mostra come «in analisi».
func inAnalisi(stato string) bool {
	switch stato {
	case "download_in_coda", "in_staging", "estrazione_in_coda", "in_analisi":
		return true
	}
	return false
}

// statoAnalisi ricava lo stato dell'analisi dai fatti dell'allegato e dai lavori pendenti (EM §10.9).
func statoAnalisi(stato db.StatoAllegato, errore string, pendenti map[string]bool) AnalisiFile {
	switch {
	case pendenti[string(db.TipoJobStageAllegato)]:
		return AnalisiFile{Stato: "download_in_coda"}
	case pendenti[string(db.TipoJobEstraiArchivio)]:
		return AnalisiFile{Stato: "estrazione_in_coda"}
	case pendenti[string(db.TipoJobAnalizzaAllegato)]:
		return AnalisiFile{Stato: "in_analisi"}
	}
	switch stato {
	case db.StatoAllegatoErrore:
		return AnalisiFile{Stato: "errore", Motivo: errore}
	case db.StatoAllegatoAnalizzato, db.StatoAllegatoIgnorato:
		return AnalisiFile{Stato: "letto"}
	case db.StatoAllegatoInStaging:
		return AnalisiFile{Stato: "in_staging"}
	}
	if strings.TrimSpace(errore) == "in coda" {
		return AnalisiFile{Stato: "download_in_coda"}
	}
	return AnalisiFile{Stato: "non_scaricato"}
}

// fileDaRiga compone un file di S1 da una riga di ListFileSorgente.
func (s *Servizio) fileDaRiga(r db.ListFileSorgenteRow, pendenti map[string]bool, voci []uuid.UUID, unita bool) FileSorgente {
	sha := ""
	if r.Sha256.Valid {
		sha = strings.ToLower(strings.TrimSpace(r.Sha256.String))
	}
	stato := r.Stato
	if r.ScartatoLegacy {
		stato = StatoScartato
	}
	f := FileSorgente{AllegatoID: r.AllegatoID, NomeFile: r.NomeFile, Estensione: r.Estensione.String, Sha256: sha,
		Origine: string(r.Origine), Canale: string(r.Canale), MessaggioID: r.MessaggioID, RicevutoIl: r.RicevutoIl,
		PathInterno: strings.ReplaceAll(r.PathInterno.String, `\`, "/"), Voci: voci, Stato: stato, Versione: r.Versione,
		ScartatoLegacy: r.ScartatoLegacy}
	if f.Voci == nil {
		f.Voci = []uuid.UUID{}
	}
	if r.Bytes.Valid {
		b := r.Bytes.Int64
		f.Bytes = &b
	}
	if r.ContenitoreID.Valid {
		c := r.ContenitoreID.UUID
		f.ContenitoreID = &c
	}
	if r.PropostaID.Valid {
		pid := r.PropostaID.UUID
		f.PropostaID = &pid
		p := &PropostaFile{Stato: string(r.StatoProposta.StatoProposta), TipoProposto: string(r.TipoProposto.TipoDocumento),
			Codice: r.PropostaCodice.String, Rev: r.PropostaRev.String}
		if r.PropostaComponenteID.Valid {
			c := r.PropostaComponenteID.UUID
			p.ComponenteID = &c
		}
		f.Proposta = p
	}
	if r.DocumentoID.Valid {
		f.Documento = &DocumentoFile{DocumentoID: r.DocumentoID.UUID, Stato: string(r.DocumentoStatoNas.StatoNas),
			Codice: r.DocumentoCodice.String, Rev: r.DocumentoRev.String, Tipo: string(r.DocumentoTipo.TipoDocumento)}
	}
	f.Analisi = statoAnalisi(r.StatoAllegato, r.Errore.String, pendenti)
	f.Analisi.Motivo = SenzaPercorsi(f.Analisi.Motivo) // il messaggio di un worker puo' citare un percorso (T-15)
	// i byte, in stati
	f.Byte = ByteFile{Staging: "assente", Conservazione: "assente", NAS: "assente"}
	if r.PathStaging.Valid && r.PathStaging.String != "" {
		switch {
		case presenteNelloStaging(s.Staging, r.PathStaging.String, sha):
			f.Byte.Staging = "presente"
		case s.Staging != "" && sha != "" && !esiste(r.PathStaging.String):
			f.Byte.Staging = "assente"
		default:
			f.Byte.Staging = "non_verificato"
		}
	}
	if r.ConservazioneID.Valid {
		cf := &ConservazioneFile{Stato: r.ConservazioneStato.String, Causa: r.ConservazioneCausa.String, VerificatoIl: r.ConservazioneVerificatoIl}
		if r.ConservazioneBytes.Valid {
			b := r.ConservazioneBytes.Int64
			cf.Bytes = &b
		}
		switch r.ConservazioneStato.String {
		case "errore_copia":
			cf.Stato = "errore"
		case "da_rilasciare":
			if r.ConservazioneErrore.Valid && r.ConservazioneErrore.String != "" && stato != StatoScartato {
				cf.Motivo = SenzaPercorsi(r.ConservazioneErrore.String)
			}
		}
		if (r.ConservazioneStato.String == "copiata" || r.ConservazioneStato.String == "da_rilasciare") &&
			s.Deposito != nil && r.ConservazioneDeposito.Valid && r.ConservazioneBytes.Valid &&
			s.Deposito.CopiaPresente(r.ConservazionePercorso.String, r.ConservazioneDeposito.UUID, r.ConservazioneBytes.Int64) {
			f.Byte.Conservazione = "presente"
		} else if r.ConservazioneStato.String == "copiata" && s.Deposito == nil {
			f.Byte.Conservazione = "non_verificato"
		}
		// una copia verificata il cui file manca nel deposito e' un'anomalia: la Sorgente la segnala (EM §10.5)
		if r.ConservazioneStato.String == "copiata" && f.Byte.Conservazione == "assente" {
			cf.Stato = "errore"
			cf.Motivo = "il file della copia non è nel deposito"
		}
		f.Conservazione = cf
	}
	if r.NasScritto {
		f.Byte.NAS = "presente"
	}
	if r.RimozioneID.Valid {
		f.Rimozione = &RimozioneFile{Stato: r.RimozioneStato.String, Motivo: r.RimozioneMotivo.String,
			Testo: TestoRimozione(r.RimozioneStato.String, r.RimozioneMotivo.String), Tentativi: r.RimozioneTentativi.Int32}
	}
	if r.ScartoQuando != nil && stato == StatoScartato {
		f.Scarto = &TracciaGesto{Autore: r.ScartoAutore.String, Quando: *r.ScartoQuando, Motivo: r.ScartoMotivo.String}
	}
	if r.AltriFileQuando != nil && stato == StatoAltriFile {
		f.AltriFile = &TracciaGesto{Autore: r.AltriFileAutore.String, Quando: *r.AltriFileQuando, Motivo: r.AltriFileMotivo.String}
	}
	f.Azioni = s.azioni(f, r.Natura, unita)
	return f
}

func esiste(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// azioni: le azioni della Sorgente su un file, sempre presenti, spente con il motivo (PE-06).
func (s *Servizio) azioni(f FileSorgente, natura db.NaturaAllegato, unita bool) AzioniFile {
	no := func(motivo string) Azione { return Azione{Motivo: motivo} }
	si := Azione{Disponibile: true}
	var a AzioniFile
	gestibile := natura == db.NaturaAllegatoFile || natura == db.NaturaAllegatoElementoOutlook
	switch {
	case !gestibile:
		a.Scarta = no("il ciclo dei file gestisce i file e gli elementi di Outlook")
		a.AltriFile, a.Riattiva = a.Scarta, a.Scarta
	case unita:
		a.Scarta = no("la RFQ è stata unita a un'altra: i suoi file si gestiscono in quella")
		a.AltriFile, a.Riattiva = a.Scarta, a.Scarta
	default:
		switch {
		case f.ScartatoLegacy && f.Byte.Staging == "assente":
			a.Scarta = no("scartato con il gesto precedente: nello staging non c'è una copia da eliminare")
		case f.ScartatoLegacy:
			a.Scarta = si
		case f.Stato == StatoScartato:
			a.Scarta = no("il file è già scartato")
		default:
			a.Scarta = si
		}
		switch {
		case f.Stato == StatoScartato:
			a.AltriFile = no("il file è scartato: per riaverlo si ricarica")
		case f.Stato == StatoAltriFile:
			a.AltriFile = no("il file è già in Altri file")
		case s.Deposito == nil:
			a.AltriFile = no("la conservazione degli Altri file non è configurata su questo server")
		default:
			a.AltriFile = si
		}
		switch f.Stato {
		case StatoAltriFile:
			a.Riattiva = si
		case StatoScartato:
			a.Riattiva = no("il file è scartato: per riaverlo si ricarica")
		default:
			a.Riattiva = no("il file è già fra gli attivi")
		}
	}
	switch {
	case f.Stato == StatoScartato:
		a.Contenuto = Azione{Motivo: "il file è scartato in questa RFQ: per riaverlo, ricaricalo", Richiede: []string{"ricarica"}}
	case f.Byte.Staging == "presente" || f.Byte.Conservazione == "presente" || f.Byte.NAS == "presente" ||
		f.Byte.Staging == "non_verificato" || f.Byte.Conservazione == "non_verificato":
		a.Contenuto = Azione{Disponibile: true}
	case f.Origine == string(db.OrigineAllegatoOutlook):
		a.Contenuto = Azione{Motivo: "il file non è nello staging: scaricalo dalla mail con «Scarica»", Richiede: []string{"scarica"}}
	default:
		a.Contenuto = Azione{Motivo: "il file non è più nello staging: ricaricalo", Richiede: []string{"ricarica"}}
	}
	if a.Contenuto.Richiede == nil {
		a.Contenuto.Richiede = []string{}
	}
	return a
}

// dettaglio: il file chiesto con ?file= (dipendenze di ogni occorrenza del gesto, impronta, perimetro, storia).
func (s *Servizio) dettaglio(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) (*Dettaglio, error) {
	o, err := leggiOccorrenza(ctx, q, thread, allegato)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fuoriRFQ("il file chiesto non è di questa RFQ")
	}
	p, err := perimetro(ctx, q, *o)
	if err != nil {
		return nil, err
	}
	tutte := ordinate(append([]Occorrenza{*o}, p.Voci...))
	nVoci := map[uuid.UUID]int{}
	if p.Archivio {
		nVoci[o.AllegatoID] = p.NVoci()
	}
	el, err := s.calcolaDipendenze(ctx, q, thread, tutte, nVoci)
	if err != nil {
		return nil, err
	}
	d := &Dettaglio{AllegatoID: allegato, Dipendenze: el.Voci, ImprontaDipendenze: el.Impronta, Eventi: []EventoFile{}}
	if p.Archivio {
		n := p.NVoci()
		d.NVoci = &n
		d.ImprontaVoci = p.Impronta()
		d.Voci = []VoceDettaglio{}
		for _, v := range p.Voci {
			d.Voci = append(d.Voci, VoceDettaglio{AllegatoID: v.AllegatoID, NomeFile: v.NomeFile, Stato: v.StatoCiclo()})
		}
	}
	ev, err := q.ListEventiFileConAutore(ctx, db.ListEventiFileConAutoreParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return nil, err
	}
	for _, e := range ev {
		x := EventoFile{Azione: e.Azione, StatoPrima: e.StatoPrima.String, StatoDopo: e.StatoDopo.String, Motivo: e.Motivo.String,
			Origine: e.Origine, Autore: e.Autore.String, Quando: e.Quando}
		if e.Esito != nil {
			x.Esito = e.Esito
		}
		if e.AltroAllegatoID.Valid {
			a := e.AltroAllegatoID.UUID
			x.AltroAllegatoID = &a
		}
		d.Eventi = append(d.Eventi, x)
	}
	return d, nil
}

// rfqNonTrovata: la RFQ del percorso non c'e' (404; il gestore controlla l'esistenza prima del corpo).
func rfqNonTrovata() *Rifiuto {
	return rifiuto(http.StatusNotFound, "RFQ_INESISTENTE", "RFQ non trovata", nil)
}
