package caricatore

// conversioni.go: dalle righe sqlc ai tipi puri di fotorfq (piano A, A1c, 6.4.2; contratto di A1c, §3.3). Si fa a
// transazione chiusa. Le regole, uguali per ogni record: un NULL del DB diventa un puntatore nil, mai un valore
// vuoto; i tempi sono UTC, troncati al millisecondo (come gli adattatori di A1b); i valori di enum diventano il loro
// testo; di evidenza entrano solo strutturale e albero, in record tipizzati (T-03). Nessuna interpretazione: le
// righe dicono che cosa c'è nel DB, non che cosa significa.

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/db"
)

// componi: la fotografia dalle letture, ordinata. La transazione è già chiusa.
func componi(l letture, sorgente string) (fotorfq.Fotografia, error) {
	f := fotorfq.Fotografia{
		VersioneSchema: fotorfq.VersioneSchema,
		Origine:        fotorfq.OrigineDSN,
		Sorgente:       sorgente,
		SchemaDB:       l.schema,
		PresaIl:        alMillisecondo(l.presaIl),
		Coerente:       true,
		SolaLettura:    solaLetturaAttesa, // la risposta di SHOW, controllata da apriInSolaLettura
		Isolamento:     isolamentoAtteso,
		Sezioni:        map[string]fotorfq.StatoSezione{},
	}
	for _, k := range fotorfq.ChiaviSezioni {
		f.Sezioni[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneCompleta}
	}
	if l.analizzatore != nil {
		f.Analizzatore = &fotorfq.Terna{Versione: l.analizzatore.VersioneAnalizzatore, HashConfigurazione: l.analizzatore.HashConfigurazione}
	} else {
		f.Sezioni[fotorfq.SezioneFatti] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente, Motivo: "nessun analizzatore corrente"}
		f.Diagnostiche = append(f.Diagnostiche, evidenze.Diagnostica{
			Codice: fotorfq.CodiceNessunAnalizzatore, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
			Percorso: "analizzatore", Messaggio: "analizzatore_corrente è vuota: nessuna terna corrente, nessun fatto caricato",
		})
	}
	for _, lt := range l.thread {
		t, err := thread(lt)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		f.Thread = append(f.Thread, t)
	}
	for _, c := range l.clienti {
		f.Clienti = append(f.Clienti, fotorfq.Cliente{ID: c.ClienteID, RagioneSociale: c.RagioneSociale})
	}
	for _, lf := range l.fuori {
		m, err := fuoriRFQ(lf)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		f.FuoriRFQ = append(f.FuoriRFQ, m)
	}
	for _, u := range l.utenti {
		f.Utenti = append(f.Utenti, fotorfq.Utente{ID: u.UtenteID, Sigla: u.Sigla})
	}
	f.Ordina()
	return f, nil
}

func thread(lt letturaThread) (fotorfq.Thread, error) {
	th := lt.thread
	t := fotorfq.Thread{
		ID:          th.ThreadID,
		ClienteID:   th.ClienteID,
		Stato:       string(th.Stato),
		UnitoIn:     uuidPtr(th.UnitoIn),
		Oggetto:     testo(th.Oggetto),
		Riferimento: testo(th.RiferimentoCliente),
		CreatoDa:    uuidPtr(th.CreatoDa),
		CreatoIl:    alMillisecondo(th.CreatoIl),
	}
	for _, m := range lt.messaggi {
		t.Messaggi = append(t.Messaggi, messaggio(m))
		t.Agganci = append(t.Agganci, aggancio(m))
	}
	for _, a := range lt.allegati {
		t.Allegati = append(t.Allegati, allegato(a))
	}
	for _, r := range lt.fatti {
		x, err := fattiDi(r.Sha256, r.VersioneAnalizzatore, r.HashConfigurazione, r.Fatti, r.CalcolatoIl, r.ConStruttura, r.MotivoParziale)
		if err != nil {
			return t, fmt.Errorf("caricatore: RFQ %s: %w", t.ID, err)
		}
		if t.Fatti == nil {
			t.Fatti = map[string]fotorfq.Fatti{}
		}
		t.Fatti[x.Sha256] = x
	}
	for _, p := range lt.proposte {
		t.Proposte = append(t.Proposte, proposta(p))
	}
	provenienze := map[uuid.UUID][]uuid.UUID{}
	for _, p := range lt.provenienze {
		if p.AllegatoID.Valid {
			provenienze[p.DocumentoID] = append(provenienze[p.DocumentoID], p.AllegatoID.UUID)
		}
	}
	for _, d := range lt.documenti {
		t.Documenti = append(t.Documenti, documento(d, provenienze[d.DocumentoID]))
	}
	for _, x := range lt.identificativi {
		t.Identificativi = append(t.Identificativi, identificativo(x))
	}
	for _, c := range lt.componenti {
		t.Componenti = append(t.Componenti, componente(c))
	}
	for _, r := range lt.relazioni {
		t.Relazioni = append(t.Relazioni, relazione(r))
	}
	for _, r := range lt.righeNodi {
		riga, err := rigaComponente(r)
		if err != nil {
			return t, fmt.Errorf("caricatore: RFQ %s: %w", t.ID, err)
		}
		t.RigheComponenteProposta = append(t.RigheComponenteProposta, riga)
	}
	for _, r := range lt.righeArchi {
		riga, err := rigaRelazione(r)
		if err != nil {
			return t, fmt.Errorf("caricatore: RFQ %s: %w", t.ID, err)
		}
		t.RigheRelazioneProposta = append(t.RigheRelazioneProposta, riga)
	}
	for _, r := range lt.rimozioni {
		t.RimozioniAperte = append(t.RimozioniAperte, fotorfq.RimozioneAperta{
			StepDocumentoID: r.StepDocumentoID, PadreID: r.PadreID, FiglioID: r.FiglioID, QtaWorking: int(r.QtaWorking),
		})
	}
	for _, r := range lt.stepProdotto {
		t.StepProdotto = append(t.StepProdotto, rigaStepProdotto(r))
	}
	for _, r := range lt.fascicolo {
		t.Fascicolo = append(t.Fascicolo, rigaFascicolo(r))
	}
	for _, r := range lt.fabbisogni {
		var fonte *string
		if r.FonteAttesa.Valid {
			s := string(r.FonteAttesa.FonteFabbisogno)
			fonte = &s
		}
		t.Fabbisogni = append(t.Fabbisogni, fotorfq.RigaFabbisogno{
			TipoComponente: string(r.TipoComponente), TipoDocumento: string(r.Tipo), Bloccante: r.Bloccante,
			FonteAttesa: fonte, Proprio: r.Proprio,
		})
	}
	for _, d := range lt.deroghe {
		t.Deroghe = append(t.Deroghe, fotorfq.DerogaFabbisogno{
			ID: d.DerogaID, ComponenteID: d.ComponenteID, Tipo: string(d.Tipo), Motivo: d.Motivo, UtenteID: d.UtenteID,
			CreataIl: alMillisecondo(d.CreataIl),
		})
	}
	for _, r := range lt.triage {
		t.Triage = append(t.Triage, triage(r))
	}
	for _, c := range lt.candidati {
		t.CandidatiCodice = append(t.CandidatiCodice, fotorfq.CandidatoCodice{
			MessaggioID: c.MessaggioID, Codice: c.Codice, Ruolo: string(c.Ruolo), Rev: c.Rev, Origine: string(c.Origine),
			Famiglia: c.Famiglia, Punteggio: c.Punteggio,
		})
	}
	visti := map[uuid.UUID]bool{}
	for _, p := range lt.pendenti {
		if !visti[p.AllegatoID] {
			visti[p.AllegatoID] = true
			t.InAttesa = append(t.InAttesa, p.AllegatoID)
		}
	}
	t.VersioneBOM = versioneBOM(lt.versione)
	t.UltimaCongelata = versioneBOM(lt.congelata)
	return t, nil
}

func fuoriRFQ(lf letturaFuori) (fotorfq.MessaggioFuoriRFQ, error) {
	m := fotorfq.MessaggioFuoriRFQ{Messaggio: messaggio(lf.messaggio), Aggancio: aggancio(lf.messaggio)}
	for _, a := range lf.allegati {
		m.Allegati = append(m.Allegati, allegato(a))
	}
	for _, r := range lf.fatti {
		x, err := fattiDi(r.Sha256, r.VersioneAnalizzatore, r.HashConfigurazione, r.Fatti, r.CalcolatoIl, r.ConStruttura, r.MotivoParziale)
		if err != nil {
			return m, fmt.Errorf("caricatore: messaggio %s: %w", m.Messaggio.ID, err)
		}
		if m.Fatti == nil {
			m.Fatti = map[string]fotorfq.Fatti{}
		}
		m.Fatti[x.Sha256] = x
	}
	return m, nil
}

// messaggio: le colonne del messaggio nel record di A1b (invariato). Il gesto 1 va in aggancio.
func messaggio(m db.Messaggio) fotorfq.Messaggio {
	return fotorfq.Messaggio{
		ID:                   m.MessaggioID,
		ConversazioneID:      m.ConversazioneID,
		ThreadID:             uuidPtr(m.ThreadID),
		ParentID:             uuidPtr(m.ParentMessaggioID),
		Canale:               string(m.Canale),
		Direzione:            string(m.Direzione),
		DataEvento:           alMillisecondo(m.DataEvento),
		MittenteNome:         testoVuoto(m.MittenteNome),
		MittenteIndirizzo:    testoVuoto(m.MittenteIndirizzo),
		Oggetto:              testo(m.Oggetto),
		CorpoTesto:           testo(m.CorpoTesto),
		CorpoHTML:            testo(m.CorpoHtml),
		ControparteTipo:      string(m.ControparteTipo),
		ControparteClienteID: uuidPtr(m.ControparteClienteID),
		Interno:              m.Interno,
	}
}

// aggancio: il gesto 1 di un messaggio (I-2, T-B0-01). AgganciatoDa nil = automatico.
func aggancio(m db.Messaggio) fotorfq.AggancioMessaggio {
	return fotorfq.AggancioMessaggio{
		MessaggioID:  m.MessaggioID,
		Aggancio:     string(m.Aggancio),
		AgganciatoDa: uuidPtr(m.AgganciatoDa),
		AgganciatoIl: alMillisecondoPtr(m.AgganciatoIl),
	}
}

// allegato: le colonne dell'allegato nel record di A1b (invariato).
func allegato(a db.Allegato) fotorfq.Allegato {
	return fotorfq.Allegato{
		ID:            a.AllegatoID,
		MessaggioID:   a.MessaggioID,
		ContenitoreID: uuidPtr(a.ContenitoreID),
		Indice:        a.Indice,
		NomeFile:      a.NomeFile,
		PathInterno:   testo(a.PathInterno),
		Estensione:    testo(a.Estensione),
		ContentType:   testo(a.ContentType),
		Natura:        string(a.Natura),
		Origine:       string(a.Origine),
		Stato:         string(a.Stato),
		Bytes:         int8Ptr(a.Bytes),
		Sha256:        testo(a.Sha256),
		RicevutoIl:    alMillisecondo(a.RicevutoIl),
	}
}

// fattiDi: i fatti di un contenuto, con l'impronta del payload (ImprontaPayload: l'unico modo, 5.4.2) e il motivo di
// completezza della 0020 solo per i fatti con la struttura STEP ("" = completa; nil = nessuna struttura).
func fattiDi(sha string, versione int16, hash string, payload json.RawMessage, calcolato time.Time, conStruttura bool, motivo string) (fotorfq.Fatti, error) {
	p := append(json.RawMessage(nil), payload...)
	digest, err := fotorfq.ImprontaPayload(p)
	if err != nil {
		return fotorfq.Fatti{}, fmt.Errorf("fatti di %s: %w", sha, err)
	}
	x := fotorfq.Fatti{
		Sha256:      sha,
		Terna:       fotorfq.Terna{Versione: versione, HashConfigurazione: hash},
		CalcolatoIl: alMillisecondo(calcolato),
		Payload:     p,
		Digest:      digest,
	}
	if conStruttura {
		m := motivo
		x.MotivoParziale = &m
	}
	return x, nil
}

func proposta(p db.DocumentoProposta) fotorfq.PropostaAttuale {
	var dettagli json.RawMessage
	if len(p.Dettagli) > 0 {
		dettagli = append(json.RawMessage(nil), p.Dettagli...)
	}
	return fotorfq.PropostaAttuale{
		ID:           p.PropostaID,
		AllegatoID:   p.AllegatoID,
		ThreadID:     uuidPtr(p.ThreadID),
		Tipo:         string(p.TipoProposto),
		Codice:       testo(p.Codice),
		Rev:          testo(p.Rev),
		ComponenteID: uuidPtr(p.ComponenteID),
		Fonte:        string(p.Fonte),
		Stato:        string(p.Stato),
		DecisoDa:     uuidPtr(p.DecisoDa),
		DecisoIl:     alMillisecondoPtr(p.DecisoIl),
		Dettagli:     dettagli,
	}
}

func documento(d db.Documento, provenienze []uuid.UUID) fotorfq.DocumentoConfermato {
	return fotorfq.DocumentoConfermato{
		ID:           d.DocumentoID,
		ThreadID:     d.ThreadID,
		ComponenteID: uuidPtr(d.ComponenteID),
		Tipo:         string(d.Tipo),
		Codice:       testo(d.Codice),
		Rev:          testo(d.Rev),
		NomeFile:     d.NomeFile,
		Estensione:   d.Estensione,
		Sha256:       d.Sha256,
		StatoNas:     string(d.StatoNas),
		ConfermatoDa: d.ConfermatoDa,
		ConfermatoIl: alMillisecondo(d.ConfermatoIl),
		SostituitoDa: uuidPtr(d.SostituitoDa),
		Allegati:     append([]uuid.UUID(nil), provenienze...),
	}
}

func identificativo(x db.IdentificativoThread) fotorfq.Identificativo {
	var conf *int16
	if x.Confidenza.Valid {
		c := x.Confidenza.Int16
		conf = &c
	}
	return fotorfq.Identificativo{
		Codice:       x.Codice,
		Origine:      string(x.Origine),
		Confidenza:   conf,
		ConfermatoDa: uuidPtr(x.ConfermatoDa),
		CreatoIl:     alMillisecondo(x.CreatoIl),
	}
}

func componente(c db.Componente) fotorfq.Componente {
	return fotorfq.Componente{
		ID:                c.ComponenteID,
		Codice:            c.Codice,
		Rev:               testo(c.Rev),
		Descrizione:       testo(c.Descrizione),
		Tipo:              string(c.Tipo),
		Origine:           string(c.Origine),
		ConfermatoDa:      c.ConfermatoDa,
		CreatoIl:          alMillisecondo(c.CreatoIl),
		ArchiviatoIl:      alMillisecondoPtr(c.ArchiviatoIl),
		StepStrutturaleID: uuidPtr(c.StepStrutturaleID),
	}
}

func relazione(r db.ComponenteRelazione) fotorfq.Relazione {
	return fotorfq.Relazione{
		PadreID:      r.PadreID,
		FiglioID:     r.FiglioID,
		Qta:          int(r.Qta),
		Posizione:    testo(r.Posizione),
		Origine:      string(r.Origine),
		ConfermatoDa: r.ConfermatoDa,
		CreatoIl:     alMillisecondo(r.CreatoIl),
	}
}

func rigaComponente(r db.ListComponenteProposteThreadRow) (fotorfq.RigaComponenteProposta, error) {
	cp := r.ComponenteProposta
	riga := fotorfq.RigaComponenteProposta{
		ID:           cp.PropostaID,
		AllegatoID:   cp.AllegatoID,
		NomeFile:     r.NomeFile,
		Sha256:       cp.Sha256,
		Chiave:       cp.Chiave,
		IDGrezzo:     cp.IDGrezzo,
		NomeGrezzo:   cp.NomeGrezzo,
		Descrizione:  testo(cp.Descrizione),
		Codice:       testo(cp.Codice),
		Rev:          testo(cp.Rev),
		Famiglia:     cp.Famiglia,
		Fonte:        string(cp.Fonte),
		Stato:        string(cp.Stato),
		ComponenteID: uuidPtr(cp.ComponenteID),
		DecisoDa:     uuidPtr(cp.DecisoDa),
		DecisoIl:     alMillisecondoPtr(cp.DecisoIl),
		Nota:         testo(cp.Nota),
	}
	if cp.OrigineCodice.Valid {
		s := string(cp.OrigineCodice.OrigineCodice)
		riga.OrigineCodice = &s
	}
	if cp.TipoProposto.Valid {
		s := string(cp.TipoProposto.TipoComponente)
		riga.TipoProposto = &s
	}
	var err error
	if riga.Marcatura, riga.Albero, err = evidenzaDellaRiga(cp.Evidenza); err != nil {
		return riga, fmt.Errorf("riga %s di componente_proposta: %w", cp.PropostaID, err)
	}
	return riga, nil
}

func rigaRelazione(r db.ListRelazioneProposteThreadRow) (fotorfq.RigaRelazioneProposta, error) {
	rp := r.RelazioneProposta
	riga := fotorfq.RigaRelazioneProposta{
		AllegatoID:   rp.AllegatoID,
		NomeFile:     r.NomeFile,
		PadreChiave:  rp.PadreChiave,
		FiglioChiave: rp.FiglioChiave,
		Qta:          int(rp.Qta),
		Stato:        string(rp.Stato),
		Nota:         testo(rp.Nota),
		DecisoDa:     uuidPtr(rp.DecisoDa),
		DecisoIl:     alMillisecondoPtr(rp.DecisoIl),
	}
	var err error
	if _, riga.Albero, err = evidenzaDellaRiga(rp.Evidenza); err != nil {
		return riga, fmt.Errorf("riga di relazione_proposta (%s, %s → %s): %w", rp.AllegatoID, rp.PadreChiave, rp.FiglioChiave, err)
	}
	return riga, nil
}

// marcaturaDB: evidenza.strutturale com'è nel JSON (la forma di fascicolo.Marcatura).
type marcaturaDB struct {
	V            int        `json:"v"`
	Ruolo        string     `json:"ruolo"`
	ComponenteID *uuid.UUID `json:"componente_id"`
	DocumentoID  *uuid.UUID `json:"documento_id"`
	DichiaratoDa *uuid.UUID `json:"dichiarato_da"`
	DichiaratoIl string     `json:"dichiarato_il"`
	PresaDAtto   string     `json:"presa_d_atto"`
}

// segnoDB: evidenza.albero com'è nel JSON (la forma di fascicolo.SegnoAlbero; commerciale non si porta).
type segnoDB struct {
	Da     uuid.UUID  `json:"da"`
	Il     string     `json:"il"`
	Firma  string     `json:"firma"`
	Padre  *uuid.UUID `json:"padre"`
	Figlio *uuid.UUID `json:"figlio"`
	Nodo   string     `json:"nodo"`
}

// evidenzaDellaRiga legge di evidenza solo strutturale (il gesto 3) e albero (il segno di «Conferma l'albero»):
// classificato, storia e il resto non entrano (T-03). Una chiave assente o null vuol dire nessun record. Un JSON che
// non ha la forma attesa è un errore: un gesto non si indovina.
func evidenzaDellaRiga(raw json.RawMessage) (*fotorfq.MarcaturaStrutturale, *fotorfq.SegnoAlbero, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal(raw, &chiavi); err != nil {
		return nil, nil, fmt.Errorf("evidenza non è un oggetto JSON: %w", err)
	}
	var marc *fotorfq.MarcaturaStrutturale
	if s, ok := chiavi["strutturale"]; ok && !nullo(s) {
		var m marcaturaDB
		if err := json.Unmarshal(s, &m); err != nil {
			return nil, nil, fmt.Errorf("evidenza.strutturale: %w", err)
		}
		var interne map[string]json.RawMessage
		if err := json.Unmarshal(s, &interne); err != nil {
			return nil, nil, fmt.Errorf("evidenza.strutturale: %w", err)
		}
		_, sospesa := interne["sospesa"]
		marc = &fotorfq.MarcaturaStrutturale{
			Versione: m.V, Ruolo: m.Ruolo, ComponenteID: m.ComponenteID, DocumentoID: m.DocumentoID,
			DichiaratoDa: m.DichiaratoDa, DichiaratoIl: m.DichiaratoIl, PresaDAtto: m.PresaDAtto, Sospesa: sospesa,
		}
	}
	var segno *fotorfq.SegnoAlbero
	if a, ok := chiavi["albero"]; ok && !nullo(a) {
		var s segnoDB
		if err := json.Unmarshal(a, &s); err != nil {
			return nil, nil, fmt.Errorf("evidenza.albero: %w", err)
		}
		if s.Da == uuid.Nil {
			return nil, nil, errors.New("evidenza.albero senza chi l'ha confermato (da)")
		}
		segno = &fotorfq.SegnoAlbero{Da: s.Da, Il: s.Il, Firma: s.Firma, Padre: s.Padre, Figlio: s.Figlio, Nodo: s.Nodo}
	}
	return marc, segno, nil
}

// nullo: il valore JSON è null.
func nullo(raw json.RawMessage) bool {
	return string(raw) == "null"
}

func rigaStepProdotto(r db.VStepProdotto) fotorfq.RigaStepProdotto {
	return fotorfq.RigaStepProdotto{
		ComponenteID:       r.ComponenteID,
		Codice:             r.Codice,
		StepStrutturaleID:  uuidPtr(r.StepStrutturaleID),
		NStepCorrenti:      r.NStepCorrenti,
		AnalisiCompleta:    r.AnalisiCompleta,
		MotivoParziale:     testo(r.MotivoParziale),
		DerogaStrutturaID:  uuidPtr(r.DerogaStrutturaID),
		Altro3DDocumentoID: uuidPtr(r.Altro3dDocumentoID),
		PropostaAperta:     uuidPtr(r.PropostaAperta),
		AttesoDaPortale:    uuidPtr(r.AttesoDaPortale),
		Esito:              r.Esito,
	}
}

func rigaFascicolo(r db.VFascicolo) fotorfq.RigaFascicolo {
	var fonte *string
	if r.FonteAttesa.Valid {
		s := string(r.FonteAttesa.FonteFabbisogno)
		fonte = &s
	}
	return fotorfq.RigaFascicolo{
		ComponenteID:    r.ComponenteID,
		Codice:          r.Codice,
		Rev:             testo(r.Rev),
		TipoComponente:  string(r.TipoComponente),
		TipoDocumento:   string(r.TipoDocumento),
		Bloccante:       r.Bloccante,
		DocumentoID:     uuidPtr(r.DocumentoID),
		StatoNas:        testo(r.StatoNas),
		PropostaAperta:  uuidPtr(r.PropostaAperta),
		NProposteAperte: r.NProposteAperte,
		AttesoDaPortale: uuidPtr(r.AttesoDaPortale),
		DerogaID:        uuidPtr(r.DerogaID),
		Esito:           r.Esito,
		FonteAttesa:     fonte,
		DocumentoRev:    testo(r.DocumentoRev),
		RevDiversa:      r.RevDiversa,
		AnomaliaID:      int8Ptr(r.AnomaliaID),
	}
}

func triage(r db.ListTriageThreadRow) fotorfq.Triage {
	legame := ""
	if r.Legame.Valid {
		legame = string(r.Legame.LegameOperativo)
	}
	return fotorfq.Triage{
		ID:             r.TriageID,
		MessaggioID:    r.MessaggioID,
		Esito:          string(r.Esito),
		Atto:           testoVuoto(r.Atto),
		Legame:         legame,
		Stato:          string(r.Stato),
		Identificativi: append([]string(nil), r.Identificativi...),
		CreatoIl:       alMillisecondo(r.CreatoIl),
		DecisoIl:       alMillisecondoPtr(r.DecisoIl),
	}
}

func versioneBOM(v *db.BomVersione) *fotorfq.VersioneBOM {
	if v == nil {
		return nil
	}
	return &fotorfq.VersioneBOM{
		ID:          v.BomVersioneID,
		Numero:      v.Numero,
		Stato:       string(v.Stato),
		Contesto:    string(v.Contesto),
		CongelataDa: uuidPtr(v.CongelataDa),
		CongelataIl: alMillisecondoPtr(v.CongelataIl),
	}
}

// alMillisecondo: UTC, troncato al millisecondo, come negli adattatori di A1b. Lo zero resta zero.
func alMillisecondo(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Millisecond)
}

func alMillisecondoPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := alMillisecondo(*t)
	return &v
}

// testo: NULL → nil, mai "".
func testo(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

// testoVuoto: per i campi di A1b che il contratto vuole stringhe, dove "" vuol dire «non c'è».
func testoVuoto(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func uuidPtr(u uuid.NullUUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	v := u.UUID
	return &v
}

func int8Ptr(n pgtype.Int8) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
