package bancoa

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/dataset"
)

// export.go: la fotografia dagli export del DB (piano 6.4.9, «Sequenza -exports»; A1c.md §3.4; R32 a A, la R3 del
// 03/10: «legge da export o dal dump read-only», P-13). Gli export sono file JSON presi in momenti diversi, ognuno con
// la forma {testo della query: [righe]}: le colonne jsonb sono stringhe, i tempi sono ISO UTC al millisecondo, il NULL
// è null. Riempiono la stessa fotografia del caricatore, con Coerente falso, e con le sezioni filtrate, parziali o
// assenti dichiarate (T-12: dove una sezione manca, ciò che ne dipende non si calcola, mai «assente» per difetto).
//
// Nessun database si apre, nessun toml si legge, nessuna cartella si crea. Nessun nome di file degli export sta nel
// codice: il manifest privato elenca un file per sezione, con la voce «export.<sezione>» (ruolo export), lo sha256 e i
// byte; il file si legge nella cartella degli export con il nome del suo percorso nel manifest.

// Le sezioni degli export, cioè i nomi delle voci «export.<sezione>» del manifest [T]. Le prime tre sono obbligatorie:
// senza thread, messaggi e allegati non c'è una fotografia.
const (
	ExportThread         = "thread"         // thread_offerta (select *)
	ExportMessaggi       = "messaggi"       // messaggio, solo in entrata, senza alcune colonne
	ExportHTML           = "html"           // i corpi HTML con una tabella
	ExportAllegati       = "allegati"       // allegato (select *)
	ExportFatti          = "fatti"          // analisi_fatti, l'ultima riga per contenuto
	ExportProposte       = "proposte"       // documento_proposta, senza dettagli, deciso_da, creato_il
	ExportDocumenti      = "documenti"      // documento con la provenienza, senza confermato_da
	ExportIdentificativi = "identificativi" // identificativo_thread (select *)
	ExportRelazioni      = "relazioni"      // componente_relazione (select *)
	ExportTriage         = "triage"         // proposta_triage, solo deterministico
	ExportClienti        = "clienti"        // cliente: le ragioni sociali, per UUID
)

// sezioniExport: tutte le sezioni degli export, nell'ordine in cui si leggono.
var sezioniExport = []string{ExportThread, ExportMessaggi, ExportHTML, ExportAllegati, ExportFatti, ExportProposte,
	ExportDocumenti, ExportIdentificativi, ExportRelazioni, ExportTriage, ExportClienti}

// prefissoExport: il prefisso dei nomi delle voci degli export nel manifest.
const prefissoExport = "export."

// ErrExportNonValido: un export presente, con l'impronta giusta, che non ha la forma attesa (una colonna che manca, un
// valore del tipo sbagliato). Per il banco è una differenza, non un NON ESEGUITO: il file c'è ed è quello del manifest.
var ErrExportNonValido = errors.New("export non valido")

// FotografiaDaExport: gli export elencati dal manifest → fotorfq.Fotografia, con Coerente falso, la terna e lo schema
// di Manifest.Export, e le sezioni filtrate o assenti dichiarate (A1c.md §3.4). La cartella è quella degli export
// (-exports). Un file che manca o non è quello del manifest (sha256, byte) è dataset.ErrFileMancante o
// dataset.ErrImprontaDiversa; un manifest senza la sezione export o senza le voci obbligatorie è ErrFileMancante; un
// export che non ha la forma attesa è ErrExportNonValido.
func FotografiaDaExport(cartella fs.FS, m dataset.Manifest) (fotorfq.Fotografia, error) {
	f, _, err := fotografiaDaExport(cartella, m, nil)
	return f, err
}

// VociExport: le voci degli export del manifest, per sezione, e gli sha256 dei loro file (per il controllo n.3).
func VociExport(m dataset.Manifest) (map[string]dataset.Voce, error) {
	out := map[string]dataset.Voce{}
	for _, v := range m.Voci {
		if v.Ruolo != dataset.RuoloExport {
			continue
		}
		sez, ok := strings.CutPrefix(v.Nome, prefissoExport)
		if !ok || !dentro(sez, sezioniExport) {
			return nil, fmt.Errorf("%w: la voce %q ha il ruolo export ma non è «export.<sezione>» con una sezione nota (%s)",
				dataset.ErrFileMancante, v.Nome, strings.Join(sezioniExport, ", "))
		}
		out[sez] = v
	}
	for _, sez := range []string{ExportThread, ExportMessaggi, ExportAllegati} {
		if _, ok := out[sez]; !ok {
			return nil, fmt.Errorf("%w: il manifest non ha la voce %s%s", dataset.ErrFileMancante, prefissoExport, sez)
		}
	}
	return out, nil
}

// fotografiaDaExport: come FotografiaDaExport, con i messaggi fuori RFQ dei casi di censimento (R34), che si cercano
// fra i messaggi esportati. Un messaggio di un caso che gli export non hanno (gli export hanno solo i messaggi in
// entrata) non ferma la fotografia: torna fra i mancanti, e il banco lo dice come parte non eseguita.
func fotografiaDaExport(cartella fs.FS, m dataset.Manifest, fuori []uuid.UUID) (fotorfq.Fotografia, []uuid.UUID, error) {
	if m.Export == nil {
		return fotorfq.Fotografia{}, nil, fmt.Errorf("%w: il manifest non dichiara la sezione export (terna e schema)", dataset.ErrFileMancante)
	}
	voci, err := VociExport(m)
	if err != nil {
		return fotorfq.Fotografia{}, nil, err
	}
	righe := map[string][]rigaExport{}
	for _, sez := range sezioniExport {
		v, ok := voci[sez]
		if !ok {
			continue
		}
		r, err := leggiExport(cartella, v)
		if err != nil {
			return fotorfq.Fotografia{}, nil, err
		}
		righe[sez] = r
	}
	c := &compositore{m: m, righe: righe, presenti: voci}
	f, err := c.componi(fuori)
	return f, c.mancanti, err
}

// leggiExport: un export per la sua voce: il file della cartella con il nome del percorso della voce, sha256 e byte
// controllati, poi la forma {query: [righe]}.
func leggiExport(cartella fs.FS, v dataset.Voce) ([]rigaExport, error) {
	nome := path.Base(strings.ReplaceAll(v.Percorso, `\`, "/"))
	b, err := fs.ReadFile(cartella, nome)
	if err != nil {
		return nil, fmt.Errorf("%w: voce %q: %v", dataset.ErrFileMancante, v.Nome, err)
	}
	if int64(len(b)) != v.Byte {
		return nil, fmt.Errorf("%w: voce %q: %d byte, il manifest ne dichiara %d", dataset.ErrImprontaDiversa, v.Nome, len(b), v.Byte)
	}
	s := sha256.Sum256(b)
	if hex.EncodeToString(s[:]) != strings.ToLower(v.Sha256) {
		return nil, fmt.Errorf("%w: voce %q: lo sha256 non è quello del manifest", dataset.ErrImprontaDiversa, v.Nome)
	}
	var cima map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&cima); err != nil || len(cima) != 1 {
		return nil, fmt.Errorf("%w: voce %q: serve un oggetto con una sola chiave, il testo della query", ErrExportNonValido, v.Nome)
	}
	var righe []rigaExport
	for _, raw := range cima {
		if err := json.Unmarshal(raw, &righe); err != nil {
			return nil, fmt.Errorf("%w: voce %q: le righe non sono un elenco di oggetti", ErrExportNonValido, v.Nome)
		}
	}
	return righe, nil
}

// rigaExport: una riga di un export, colonna per colonna.
type rigaExport map[string]json.RawMessage

// erroreColonna: una colonna che manca o non ha il tipo atteso.
func erroreColonna(sez, col, motivo string) error {
	return fmt.Errorf("%w: %s%s, colonna %q: %s", ErrExportNonValido, prefissoExport, sez, col, motivo)
}

func (r rigaExport) ha(col string) bool { _, ok := r[col]; return ok }

func nulla(raw json.RawMessage) bool { return len(raw) == 0 || string(raw) == "null" }

// testo: una colonna di testo; nil per NULL. Una colonna assente è un errore se obbligatoria.
func (r rigaExport) testo(sez, col string, obbligatoria bool) (*string, error) {
	raw, ok := r[col]
	if !ok {
		if obbligatoria {
			return nil, erroreColonna(sez, col, "manca")
		}
		return nil, nil
	}
	if nulla(raw) {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, erroreColonna(sez, col, "non è una stringa")
	}
	return &s, nil
}

// testoVuoto: una colonna di testo come stringa, "" per NULL o assente.
func (r rigaExport) testoVuoto(sez, col string, obbligatoria bool) (string, error) {
	s, err := r.testo(sez, col, obbligatoria)
	if err != nil || s == nil {
		return "", err
	}
	return *s, nil
}

func (r rigaExport) uuidPtr(sez, col string, obbligatoria bool) (*uuid.UUID, error) {
	s, err := r.testo(sez, col, obbligatoria)
	if err != nil || s == nil {
		return nil, err
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return nil, erroreColonna(sez, col, "l'UUID non si legge")
	}
	return &u, nil
}

// uuidObbligatorio: una colonna UUID NOT NULL.
func (r rigaExport) uuidObbligatorio(sez, col string) (uuid.UUID, error) {
	u, err := r.uuidPtr(sez, col, true)
	if err != nil {
		return uuid.Nil, err
	}
	if u == nil {
		return uuid.Nil, erroreColonna(sez, col, "è NULL")
	}
	return *u, nil
}

// tempo: un istante ISO, in UTC e al millisecondo (6.4.1); nil per NULL o assente.
func (r rigaExport) tempo(sez, col string, obbligatoria bool) (*time.Time, error) {
	s, err := r.testo(sez, col, obbligatoria)
	if err != nil || s == nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return nil, erroreColonna(sez, col, "non è un istante ISO")
	}
	t = t.UTC().Truncate(time.Millisecond)
	return &t, nil
}

func (r rigaExport) tempoObbligatorio(sez, col string) (time.Time, error) {
	t, err := r.tempo(sez, col, true)
	if err != nil {
		return time.Time{}, err
	}
	if t == nil {
		return time.Time{}, erroreColonna(sez, col, "è NULL")
	}
	return *t, nil
}

// intero: una colonna intera; nil per NULL o assente.
func (r rigaExport) intero(sez, col string, obbligatoria bool) (*int64, error) {
	raw, ok := r[col]
	if !ok {
		if obbligatoria {
			return nil, erroreColonna(sez, col, "manca")
		}
		return nil, nil
	}
	if nulla(raw) {
		return nil, nil
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return nil, erroreColonna(sez, col, "non è un intero")
	}
	return &n, nil
}

func (r rigaExport) booleano(sez, col string) (bool, error) {
	raw, ok := r[col]
	if !ok || nulla(raw) {
		return false, nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, erroreColonna(sez, col, "non è un booleano")
	}
	return b, nil
}

// jsonb: una colonna jsonb, che l'export scrive come stringa (il testo di jsonb_out): i byte del JSON. Un oggetto già
// JSON si prende com'è.
func (r rigaExport) jsonb(sez, col string, obbligatoria bool) (json.RawMessage, error) {
	raw, ok := r[col]
	if !ok {
		if obbligatoria {
			return nil, erroreColonna(sez, col, "manca")
		}
		return nil, nil
	}
	if nulla(raw) {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.RawMessage(s), nil
	}
	return append(json.RawMessage(nil), raw...), nil
}

// testi: una colonna text[], che l'export scrive come stringa nella forma di PostgreSQL ({a,b,"c d"}) o come elenco
// JSON di stringhe.
func (r rigaExport) testi(sez, col string) ([]string, error) {
	raw, ok := r[col]
	if !ok || nulla(raw) {
		return nil, nil
	}
	var elenco []string
	if err := json.Unmarshal(raw, &elenco); err == nil {
		return elenco, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, erroreColonna(sez, col, "non è un elenco di testi")
	}
	return arrayPostgres(s)
}

// arrayPostgres: la forma di testo di un text[] di PostgreSQL, senza NULL dentro.
func arrayPostgres(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, fmt.Errorf("%w: un text[] deve stare fra graffe", ErrExportNonValido)
	}
	corpo := s[1 : len(s)-1]
	if corpo == "" {
		return []string{}, nil
	}
	var out []string
	var cur strings.Builder
	inVirgolette, escape := false, false
	for _, c := range corpo {
		switch {
		case escape:
			cur.WriteRune(c)
			escape = false
		case c == '\\':
			escape = true
		case c == '"':
			inVirgolette = !inVirgolette
		case c == ',' && !inVirgolette:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	return append(out, cur.String()), nil
}

// colonneAssenti: le colonne di un elenco che nessuna riga di un export porta.
func colonneAssenti(righe []rigaExport, colonne ...string) []string {
	var out []string
	for _, c := range colonne {
		presente := false
		for _, r := range righe {
			if r.ha(c) {
				presente = true
				break
			}
		}
		if !presente {
			out = append(out, c)
		}
	}
	return out
}

// compositore: lo stato della composizione della fotografia dagli export.
type compositore struct {
	m        dataset.Manifest
	righe    map[string][]rigaExport
	presenti map[string]dataset.Voce
	diag     []evidenze.Diagnostica
	mancanti []uuid.UUID // i messaggi fuori RFQ dei casi che gli export non hanno
}

func (c *compositore) avviso(percorso, messaggio string) {
	c.diag = append(c.diag, evidenze.Diagnostica{Codice: fotorfq.CodiceSezioneParziale, Gravita: evidenze.GravitaAvviso,
		Natura: evidenze.NaturaDati, Percorso: percorso, Messaggio: messaggio})
}

// componi: la fotografia, sezione per sezione.
func (c *compositore) componi(fuori []uuid.UUID) (fotorfq.Fotografia, error) {
	terna := fotorfq.Terna{Versione: c.m.Export.Versione, HashConfigurazione: strings.ToLower(c.m.Export.HashConfigurazione)}
	f := fotorfq.Fotografia{
		VersioneSchema: fotorfq.VersioneSchema,
		Origine:        fotorfq.OrigineExport,
		Sorgente:       "export",
		SchemaDB:       c.m.Export.Schema,
		Coerente:       false,
		Analizzatore:   &terna,
		Sezioni:        map[string]fotorfq.StatoSezione{},
	}
	for _, k := range fotorfq.ChiaviSezioni {
		f.Sezioni[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente, Motivo: "non esportata"}
	}

	// I thread.
	thread := map[uuid.UUID]*fotorfq.Thread{}
	var ordine []uuid.UUID
	for _, r := range c.righe[ExportThread] {
		t, err := threadDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		if _, doppio := thread[t.ID]; doppio {
			return fotorfq.Fotografia{}, fmt.Errorf("%w: %s%s: il thread %s compare due volte", ErrExportNonValido, prefissoExport, ExportThread, t.ID)
		}
		thread[t.ID] = &t
		ordine = append(ordine, t.ID)
	}

	// I messaggi, con il gesto 1 se l'export ne ha le colonne.
	msgs := map[uuid.UUID]fotorfq.Messaggio{}
	agganci := map[uuid.UUID]fotorfq.AggancioMessaggio{}
	conAggancio := len(colonneAssenti(c.righe[ExportMessaggi], "aggancio")) == 0
	for _, r := range c.righe[ExportMessaggi] {
		m, a, err := messaggioDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		msgs[m.ID] = m
		agganci[m.ID] = a
	}
	html := 0
	for _, r := range c.righe[ExportHTML] {
		id, err := r.uuidObbligatorio(ExportHTML, "messaggio_id")
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		h, err := r.testo(ExportHTML, "corpo_html", true)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		if m, ok := msgs[id]; ok {
			m.CorpoHTML = h
			msgs[id] = m
			html++
		}
	}
	motivoMessaggi := "solo i messaggi in entrata, senza alcune colonne (" + strings.Join(colonneAssenti(c.righe[ExportMessaggi],
		"canale", "chiave_esterna", "buyer_id", "lingua", "registrato_il", "conversazione_id"), ", ") + ")"
	if _, ok := c.presenti[ExportHTML]; ok {
		motivoMessaggi += "; l'HTML solo dei corpi con una tabella: per gli altri «non esportato», che non vuol dire «senza HTML»"
	} else {
		motivoMessaggi += "; l'HTML non è esportato: un corpo senza HTML qui non vuol dire «senza HTML»"
	}
	f.Sezioni[fotorfq.SezioneMessaggi] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale, Motivo: motivoMessaggi}
	if conAggancio {
		f.Sezioni[fotorfq.SezioneAgganci] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneCompleta}
	}

	// Gli allegati: di un messaggio esportato; gli altri non sono attribuibili.
	allegati := map[uuid.UUID]fotorfq.Allegato{}
	nonAttribuibili := 0
	for _, r := range c.righe[ExportAllegati] {
		a, err := allegatoDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		if _, ok := msgs[a.MessaggioID]; !ok {
			nonAttribuibili++
			continue
		}
		allegati[a.ID] = a
	}
	f.Sezioni[fotorfq.SezioneAllegati] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale,
		Motivo: fmt.Sprintf("solo gli allegati dei messaggi esportati: %d non attribuibili", nonAttribuibili)}

	// I fatti: l'ultima riga per contenuto. Una riga a un'altra terna non entra: il contenuto resta senza fatti, con la
	// diagnosi (A1c.md §3.4). MotivoParziale resta nil: senza il DB non si determina (R32 b).
	fatti := map[string]fotorfq.Fatti{}
	altraTerna := map[string]bool{}
	for _, r := range c.righe[ExportFatti] {
		x, err := fattiDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		if x.Terna != terna {
			altraTerna[x.Sha256] = true
			continue
		}
		fatti[x.Sha256] = x
	}
	if _, ok := c.presenti[ExportFatti]; ok {
		f.Sezioni[fotorfq.SezioneFatti] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale,
			Motivo: "l'ultima riga per contenuto, non la terna esatta; le righe a un'altra terna sono in diagnosi; motivo parziale non determinabile"}
	}

	// Le proposte, i documenti, gli identificativi, le relazioni, il triage: per thread.
	proposte := map[uuid.UUID][]fotorfq.PropostaAttuale{}
	for _, r := range c.righe[ExportProposte] {
		p, err := propostaDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		proposte[p.AllegatoID] = append(proposte[p.AllegatoID], p)
	}
	if _, ok := c.presenti[ExportProposte]; ok {
		f.Sezioni[fotorfq.SezioneProposteDocumento] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale,
			Motivo: "senza " + strings.Join(colonneAssenti(c.righe[ExportProposte], "dettagli", "deciso_da", "creato_il"), ", ") +
				": il vecchio ancoraggio, la lettura del vecchio motore e chi ha deciso non ci sono"}
	}
	documenti, err := documentiDaExport(c.righe[ExportDocumenti])
	if err != nil {
		return fotorfq.Fotografia{}, err
	}
	if _, ok := c.presenti[ExportDocumenti]; ok {
		f.Sezioni[fotorfq.SezioneDocumenti] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale,
			Motivo: "senza " + strings.Join(colonneAssenti(c.righe[ExportDocumenti], "confermato_da", "stato_nas", "confermato_il"), ", ")}
		f.Sezioni[fotorfq.SezioneProvenienze] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale,
			Motivo: "dalla giunzione dei documenti: solo gli allegati"}
	}
	identificativi := map[uuid.UUID][]fotorfq.Identificativo{}
	for _, r := range c.righe[ExportIdentificativi] {
		th, x, err := identificativoDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		identificativi[th] = append(identificativi[th], x)
	}
	if _, ok := c.presenti[ExportIdentificativi]; ok {
		f.Sezioni[fotorfq.SezioneIdentificativi] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneCompleta}
	}
	relazioni := map[uuid.UUID][]fotorfq.Relazione{}
	for _, r := range c.righe[ExportRelazioni] {
		th, x, err := relazioneDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		relazioni[th] = append(relazioni[th], x)
	}
	if _, ok := c.presenti[ExportRelazioni]; ok {
		f.Sezioni[fotorfq.SezioneRelazioni] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale,
			Motivo: "senza i componenti non si sa se gli estremi sono archiviati"}
	}
	triage := map[uuid.UUID][]fotorfq.Triage{}
	esclusi := 0
	for _, r := range c.righe[ExportTriage] {
		x, deterministico, err := triageDaExport(r)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		if !deterministico {
			esclusi++
			continue
		}
		triage[x.MessaggioID] = append(triage[x.MessaggioID], x)
	}
	if _, ok := c.presenti[ExportTriage]; ok {
		st := fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneCompleta}
		if esclusi > 0 {
			st = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneFiltrata, Motivo: fmt.Sprintf("solo deterministico: %d righe di un'altra fonte escluse", esclusi)}
		}
		f.Sezioni[fotorfq.SezioneTriage] = st
	}

	// La composizione per thread.
	soloAltraTerna := 0
	proposteFuori, provenienzeFuori := 0, 0
	for _, id := range ordine {
		t := thread[id]
		for _, m := range msgs {
			if m.ThreadID != nil && *m.ThreadID == id {
				t.Messaggi = append(t.Messaggi, m)
				if conAggancio {
					t.Agganci = append(t.Agganci, agganci[m.ID])
				}
				t.Triage = append(t.Triage, triage[m.ID]...)
			}
		}
		delThread := map[uuid.UUID]bool{}
		for _, a := range allegati {
			if m := msgs[a.MessaggioID]; m.ThreadID != nil && *m.ThreadID == id {
				t.Allegati = append(t.Allegati, a)
				delThread[a.ID] = true
			}
		}
		for allegato, pp := range proposte {
			if !delThread[allegato] {
				continue
			}
			t.Proposte = append(t.Proposte, pp...)
		}
		for _, d := range documenti {
			if d.ThreadID != id {
				continue
			}
			var dentroThread []uuid.UUID
			for _, a := range d.Allegati {
				if delThread[a] {
					dentroThread = append(dentroThread, a)
				} else {
					provenienzeFuori++
				}
			}
			d.Allegati = dentroThread
			t.Documenti = append(t.Documenti, d)
		}
		t.Identificativi = append(t.Identificativi, identificativi[id]...)
		t.Relazioni = append(t.Relazioni, relazioni[id]...)
		t.Fatti = fattiDi(fatti, t.Allegati, t.Documenti)
		for _, a := range t.Allegati {
			if a.Sha256 != nil && altraTerna[*a.Sha256] && t.Fatti[*a.Sha256].Sha256 == "" {
				soloAltraTerna++
			}
		}
		f.Thread = append(f.Thread, *t)
	}
	for allegato, pp := range proposte {
		if _, ok := allegati[allegato]; !ok {
			proposteFuori += len(pp)
		}
	}
	if proposteFuori > 0 {
		c.avviso("sezioni."+fotorfq.SezioneProposteDocumento, fmt.Sprintf("%d proposte con l'allegato fuori dai messaggi esportati: non attribuibili", proposteFuori))
	}
	if provenienzeFuori > 0 {
		c.avviso("sezioni."+fotorfq.SezioneProvenienze, fmt.Sprintf("%d provenienze con l'allegato fuori dal thread esportato: non attribuibili", provenienzeFuori))
	}
	if soloAltraTerna > 0 {
		c.diag = append(c.diag, evidenze.Diagnostica{Codice: fotorfq.CodiceFattiAssenti, Gravita: evidenze.GravitaAvviso,
			Natura: evidenze.NaturaDati, Percorso: "fatti",
			Messaggio: fmt.Sprintf("%d contenuti hanno i fatti solo a un'altra terna: restano senza fatti", soloAltraTerna)})
	}

	// I clienti dei thread, per UUID: la ragione sociale è un controllo contro la grammatica, mai una chiave.
	clienti := map[uuid.UUID]bool{}
	for _, t := range f.Thread {
		clienti[t.ClienteID] = true
	}
	for _, r := range c.righe[ExportClienti] {
		id, err := r.uuidObbligatorio(ExportClienti, "cliente_id")
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		rs, err := r.testoVuoto(ExportClienti, "ragione_sociale", true)
		if err != nil {
			return fotorfq.Fotografia{}, err
		}
		if clienti[id] {
			f.Clienti = append(f.Clienti, fotorfq.Cliente{ID: id, RagioneSociale: rs})
		}
	}
	if senza := len(clienti) - len(f.Clienti); senza > 0 {
		c.avviso("clienti", fmt.Sprintf("%d clienti dei thread senza ragione sociale negli export: la grammatica non si può controllare, e quei thread non si valutano", senza))
	}

	// I messaggi fuori RFQ dei casi di censimento.
	for _, id := range fuori {
		m, ok := msgs[id]
		if !ok {
			c.mancanti = append(c.mancanti, id)
			continue
		}
		mf := fotorfq.MessaggioFuoriRFQ{Messaggio: m, Aggancio: agganci[id]}
		if !conAggancio {
			mf.Aggancio = fotorfq.AggancioMessaggio{MessaggioID: id}
		}
		for _, a := range allegati {
			if a.MessaggioID == id {
				mf.Allegati = append(mf.Allegati, a)
			}
		}
		mf.Fatti = fattiDi(fatti, mf.Allegati, nil)
		f.FuoriRFQ = append(f.FuoriRFQ, mf)
	}

	for _, k := range fotorfq.ChiaviSezioni {
		if st := f.Sezioni[k]; st.Stato == fotorfq.StatoSezioneParziale || st.Stato == fotorfq.StatoSezioneFiltrata {
			c.avviso("sezioni."+k, st.Motivo)
		}
	}
	f.Diagnostiche = c.diag
	f.Ordina()
	return f, nil
}

// fattiDi: i fatti dei contenuti degli allegati e dei documenti di un thread, per sha256.
func fattiDi(tutti map[string]fotorfq.Fatti, allegati []fotorfq.Allegato, documenti []fotorfq.DocumentoConfermato) map[string]fotorfq.Fatti {
	var out map[string]fotorfq.Fatti
	aggiungi := func(sha string) {
		if x, ok := tutti[sha]; ok {
			if out == nil {
				out = map[string]fotorfq.Fatti{}
			}
			out[sha] = x
		}
	}
	for _, a := range allegati {
		if a.Sha256 != nil {
			aggiungi(*a.Sha256)
		}
	}
	for _, d := range documenti {
		aggiungi(d.Sha256)
	}
	return out
}

func threadDaExport(r rigaExport) (fotorfq.Thread, error) {
	const s = ExportThread
	var t fotorfq.Thread
	var err error
	if t.ID, err = r.uuidObbligatorio(s, "thread_id"); err != nil {
		return t, err
	}
	if t.ClienteID, err = r.uuidObbligatorio(s, "cliente_id"); err != nil {
		return t, err
	}
	if t.Stato, err = r.testoVuoto(s, "stato", true); err != nil {
		return t, err
	}
	if t.UnitoIn, err = r.uuidPtr(s, "unito_in", false); err != nil {
		return t, err
	}
	if t.Oggetto, err = r.testo(s, "oggetto", false); err != nil {
		return t, err
	}
	if t.Riferimento, err = r.testo(s, "riferimento_cliente", false); err != nil {
		return t, err
	}
	if t.CreatoDa, err = r.uuidPtr(s, "creato_da", false); err != nil {
		return t, err
	}
	t.CreatoIl, err = r.tempoObbligatorio(s, "creato_il")
	return t, err
}

func messaggioDaExport(r rigaExport) (fotorfq.Messaggio, fotorfq.AggancioMessaggio, error) {
	const s = ExportMessaggi
	var m fotorfq.Messaggio
	var a fotorfq.AggancioMessaggio
	var err error
	if m.ID, err = r.uuidObbligatorio(s, "messaggio_id"); err != nil {
		return m, a, err
	}
	if !r.ha("thread_id") {
		return m, a, erroreColonna(s, "thread_id", "manca")
	}
	if m.ThreadID, err = r.uuidPtr(s, "thread_id", true); err != nil {
		return m, a, err
	}
	if c, err := r.uuidPtr(s, "conversazione_id", false); err != nil {
		return m, a, err
	} else if c != nil {
		m.ConversazioneID = *c
	}
	if m.ParentID, err = r.uuidPtr(s, "parent_messaggio_id", false); err != nil {
		return m, a, err
	}
	if m.Canale, err = r.testoVuoto(s, "canale", false); err != nil {
		return m, a, err
	}
	if m.Direzione, err = r.testoVuoto(s, "direzione", true); err != nil {
		return m, a, err
	}
	if m.DataEvento, err = r.tempoObbligatorio(s, "data_evento"); err != nil {
		return m, a, err
	}
	if m.MittenteNome, err = r.testoVuoto(s, "mittente_nome", false); err != nil {
		return m, a, err
	}
	if m.MittenteIndirizzo, err = r.testoVuoto(s, "mittente_indirizzo", false); err != nil {
		return m, a, err
	}
	if m.Oggetto, err = r.testo(s, "oggetto", false); err != nil {
		return m, a, err
	}
	if m.CorpoTesto, err = r.testo(s, "corpo_testo", false); err != nil {
		return m, a, err
	}
	if m.ControparteTipo, err = r.testoVuoto(s, "controparte_tipo", false); err != nil {
		return m, a, err
	}
	if m.ControparteClienteID, err = r.uuidPtr(s, "controparte_cliente_id", false); err != nil {
		return m, a, err
	}
	if m.Interno, err = r.booleano(s, "interno"); err != nil {
		return m, a, err
	}
	a.MessaggioID = m.ID
	if a.Aggancio, err = r.testoVuoto(s, "aggancio", false); err != nil {
		return m, a, err
	}
	if a.AgganciatoDa, err = r.uuidPtr(s, "agganciato_da", false); err != nil {
		return m, a, err
	}
	a.AgganciatoIl, err = r.tempo(s, "agganciato_il", false)
	return m, a, err
}

func allegatoDaExport(r rigaExport) (fotorfq.Allegato, error) {
	const s = ExportAllegati
	var a fotorfq.Allegato
	var err error
	if a.ID, err = r.uuidObbligatorio(s, "allegato_id"); err != nil {
		return a, err
	}
	if a.MessaggioID, err = r.uuidObbligatorio(s, "messaggio_id"); err != nil {
		return a, err
	}
	if a.ContenitoreID, err = r.uuidPtr(s, "contenitore_id", false); err != nil {
		return a, err
	}
	i, err := r.intero(s, "indice", true)
	if err != nil {
		return a, err
	}
	if i == nil {
		return a, erroreColonna(s, "indice", "è NULL")
	}
	a.Indice = int16(*i)
	if a.NomeFile, err = r.testoVuoto(s, "nome_file", true); err != nil {
		return a, err
	}
	if a.PathInterno, err = r.testo(s, "path_interno", false); err != nil {
		return a, err
	}
	if a.Estensione, err = r.testo(s, "estensione", false); err != nil {
		return a, err
	}
	if a.ContentType, err = r.testo(s, "content_type", false); err != nil {
		return a, err
	}
	if a.Natura, err = r.testoVuoto(s, "natura", false); err != nil {
		return a, err
	}
	if a.Origine, err = r.testoVuoto(s, "origine", false); err != nil {
		return a, err
	}
	if a.Stato, err = r.testoVuoto(s, "stato", false); err != nil {
		return a, err
	}
	if a.Bytes, err = r.intero(s, "bytes", false); err != nil {
		return a, err
	}
	if a.Sha256, err = r.testo(s, "sha256", false); err != nil {
		return a, err
	}
	a.RicevutoIl, err = r.tempoObbligatorio(s, "ricevuto_il")
	return a, err
}

func fattiDaExport(r rigaExport) (fotorfq.Fatti, error) {
	const s = ExportFatti
	var x fotorfq.Fatti
	sha, err := r.testoVuoto(s, "sha256", true)
	if err != nil {
		return x, err
	}
	v, err := r.intero(s, "versione_analizzatore", true)
	if err != nil {
		return x, err
	}
	if v == nil {
		return x, erroreColonna(s, "versione_analizzatore", "è NULL")
	}
	hash, err := r.testoVuoto(s, "hash_configurazione", true)
	if err != nil {
		return x, err
	}
	payload, err := r.jsonb(s, "fatti", true)
	if err != nil {
		return x, err
	}
	calcolato, err := r.tempoObbligatorio(s, "calcolato_il")
	if err != nil {
		return x, err
	}
	digest, err := fotorfq.ImprontaPayload(payload)
	if err != nil {
		return x, erroreColonna(s, "fatti", "il JSON dei fatti non ha un'impronta: "+err.Error())
	}
	return fotorfq.Fatti{Sha256: sha, Terna: fotorfq.Terna{Versione: int16(*v), HashConfigurazione: strings.ToLower(hash)},
		CalcolatoIl: calcolato, Payload: payload, Digest: digest}, nil
}

func propostaDaExport(r rigaExport) (fotorfq.PropostaAttuale, error) {
	const s = ExportProposte
	var p fotorfq.PropostaAttuale
	var err error
	if p.ID, err = r.uuidObbligatorio(s, "proposta_id"); err != nil {
		return p, err
	}
	if p.AllegatoID, err = r.uuidObbligatorio(s, "allegato_id"); err != nil {
		return p, err
	}
	if p.ThreadID, err = r.uuidPtr(s, "thread_id", false); err != nil {
		return p, err
	}
	if p.Tipo, err = r.testoVuoto(s, "tipo_proposto", true); err != nil {
		return p, err
	}
	if p.Codice, err = r.testo(s, "codice", false); err != nil {
		return p, err
	}
	if p.Rev, err = r.testo(s, "rev", false); err != nil {
		return p, err
	}
	if p.ComponenteID, err = r.uuidPtr(s, "componente_id", false); err != nil {
		return p, err
	}
	if p.Fonte, err = r.testoVuoto(s, "fonte", true); err != nil {
		return p, err
	}
	if p.Stato, err = r.testoVuoto(s, "stato", true); err != nil {
		return p, err
	}
	if p.DecisoDa, err = r.uuidPtr(s, "deciso_da", false); err != nil {
		return p, err
	}
	if p.DecisoIl, err = r.tempo(s, "deciso_il", false); err != nil {
		return p, err
	}
	p.Dettagli, err = r.jsonb(s, "dettagli", false)
	return p, err
}

// documentiDaExport: i documenti, una riga per documento e provenienza (documento LEFT JOIN provenienza), raccolti per
// documento con gli allegati delle provenienze in ordine.
func documentiDaExport(righe []rigaExport) ([]fotorfq.DocumentoConfermato, error) {
	const s = ExportDocumenti
	per := map[uuid.UUID]*fotorfq.DocumentoConfermato{}
	var ordine []uuid.UUID
	for _, r := range righe {
		id, err := r.uuidObbligatorio(s, "documento_id")
		if err != nil {
			return nil, err
		}
		d := per[id]
		if d == nil {
			d = &fotorfq.DocumentoConfermato{ID: id}
			if d.ThreadID, err = r.uuidObbligatorio(s, "thread_id"); err != nil {
				return nil, err
			}
			if d.ComponenteID, err = r.uuidPtr(s, "componente_id", false); err != nil {
				return nil, err
			}
			if d.Tipo, err = r.testoVuoto(s, "tipo", true); err != nil {
				return nil, err
			}
			if d.Codice, err = r.testo(s, "codice", false); err != nil {
				return nil, err
			}
			if d.Rev, err = r.testo(s, "rev", false); err != nil {
				return nil, err
			}
			if d.NomeFile, err = r.testoVuoto(s, "nome_file", false); err != nil {
				return nil, err
			}
			if d.Estensione, err = r.testoVuoto(s, "estensione", false); err != nil {
				return nil, err
			}
			if d.Sha256, err = r.testoVuoto(s, "sha256", true); err != nil {
				return nil, err
			}
			if d.StatoNas, err = r.testoVuoto(s, "stato_nas", false); err != nil {
				return nil, err
			}
			if cd, err := r.uuidPtr(s, "confermato_da", false); err != nil {
				return nil, err
			} else if cd != nil {
				d.ConfermatoDa = *cd
			}
			if ci, err := r.tempo(s, "confermato_il", false); err != nil {
				return nil, err
			} else if ci != nil {
				d.ConfermatoIl = *ci
			}
			if d.SostituitoDa, err = r.uuidPtr(s, "sostituito_da", false); err != nil {
				return nil, err
			}
			per[id] = d
			ordine = append(ordine, id)
		}
		a, err := r.uuidPtr(s, "allegato_id", false)
		if err != nil {
			return nil, err
		}
		if a != nil {
			d.Allegati = append(d.Allegati, *a)
		}
	}
	out := make([]fotorfq.DocumentoConfermato, 0, len(ordine))
	for _, id := range ordine {
		d := per[id]
		sort.Slice(d.Allegati, func(i, j int) bool { return bytes.Compare(d.Allegati[i][:], d.Allegati[j][:]) < 0 })
		out = append(out, *d)
	}
	return out, nil
}

func identificativoDaExport(r rigaExport) (uuid.UUID, fotorfq.Identificativo, error) {
	const s = ExportIdentificativi
	var x fotorfq.Identificativo
	th, err := r.uuidObbligatorio(s, "thread_id")
	if err != nil {
		return th, x, err
	}
	if x.Codice, err = r.testoVuoto(s, "codice", true); err != nil {
		return th, x, err
	}
	if x.Origine, err = r.testoVuoto(s, "origine", true); err != nil {
		return th, x, err
	}
	conf, err := r.intero(s, "confidenza", false)
	if err != nil {
		return th, x, err
	}
	if conf != nil {
		c := int16(*conf)
		x.Confidenza = &c
	}
	if x.ConfermatoDa, err = r.uuidPtr(s, "confermato_da", false); err != nil {
		return th, x, err
	}
	x.CreatoIl, err = r.tempoObbligatorio(s, "creato_il")
	return th, x, err
}

func relazioneDaExport(r rigaExport) (uuid.UUID, fotorfq.Relazione, error) {
	const s = ExportRelazioni
	var x fotorfq.Relazione
	th, err := r.uuidObbligatorio(s, "thread_id")
	if err != nil {
		return th, x, err
	}
	if x.PadreID, err = r.uuidObbligatorio(s, "padre_id"); err != nil {
		return th, x, err
	}
	if x.FiglioID, err = r.uuidObbligatorio(s, "figlio_id"); err != nil {
		return th, x, err
	}
	q, err := r.intero(s, "qta", true)
	if err != nil {
		return th, x, err
	}
	if q == nil {
		return th, x, erroreColonna(s, "qta", "è NULL")
	}
	x.Qta = int(*q)
	if x.Posizione, err = r.testo(s, "posizione", false); err != nil {
		return th, x, err
	}
	if x.Origine, err = r.testoVuoto(s, "origine", true); err != nil {
		return th, x, err
	}
	if cd, err := r.uuidPtr(s, "confermato_da", false); err != nil {
		return th, x, err
	} else if cd != nil {
		x.ConfermatoDa = *cd
	}
	x.CreatoIl, err = r.tempoObbligatorio(s, "creato_il")
	return th, x, err
}

// fonteDeterministica: la fonte del triage che una fotografia porta (6.4.1: mai la fonte dell'agente LLM).
const fonteDeterministica = "deterministico"

func triageDaExport(r rigaExport) (fotorfq.Triage, bool, error) {
	const s = ExportTriage
	var x fotorfq.Triage
	var err error
	if x.ID, err = r.uuidObbligatorio(s, "triage_id"); err != nil {
		return x, false, err
	}
	if x.MessaggioID, err = r.uuidObbligatorio(s, "messaggio_id"); err != nil {
		return x, false, err
	}
	if x.Esito, err = r.testoVuoto(s, "esito", true); err != nil {
		return x, false, err
	}
	if x.Atto, err = r.testoVuoto(s, "atto", false); err != nil {
		return x, false, err
	}
	if x.Legame, err = r.testoVuoto(s, "legame", false); err != nil {
		return x, false, err
	}
	if x.Stato, err = r.testoVuoto(s, "stato", true); err != nil {
		return x, false, err
	}
	if x.Identificativi, err = r.testi(s, "identificativi"); err != nil {
		return x, false, err
	}
	if x.CreatoIl, err = r.tempoObbligatorio(s, "creato_il"); err != nil {
		return x, false, err
	}
	if x.DecisoIl, err = r.tempo(s, "deciso_il", false); err != nil {
		return x, false, err
	}
	fonte, err := r.testoVuoto(s, "fonte", false)
	if err != nil {
		return x, false, err
	}
	return x, fonte == "" || fonte == fonteDeterministica, nil
}
