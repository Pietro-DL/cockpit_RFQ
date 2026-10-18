package staging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// LO STAGING E' UNA CACHE, NON UNA CODA (Pre-7, D31).
//
// Con «un contenuto, un file» (blocco 4A) `_contenuti\<ab>\<sha256>.<ext>` e' una cache indirizzata
// dal contenuto, e dopo la copia sul NAS serve ancora a tre cose: non riscaricare da Outlook lo
// stesso file ricevuto un'altra volta, rianalizzarlo quando cambia il dizionario, ricopiarlo sul NAS
// se un giorno il file sparisce (il riconciliatore del blocco 5B). Quindi un contenuto NON si toglie
// al successo della prima copia. Si toglie quando nessuno ne ha bisogno adesso — i pin — e da tanto
// non lo tocca nessuno, oppure quando la cache ha superato la capienza che le e' stata data.
//
// Prima del blocco 7 la pulizia toglieva solo i contenuti che NESSUN allegato nominava piu'. Gli
// allegati non si cancellano mai, quindi in pratica non toglieva niente: lo staging cresceva per
// sempre, e sembrava una coda bloccata mentre era una cache senza politica.
//
// L'unita' e' il CONTENUTO, mai il singolo allegato: un file e' di tutti gli allegati che lo hanno.
// Alla rimozione, tutti i loro `path_staging` vanno a NULL nella stessa transazione; lo stato resta
// com'e', perche' il file e' ricostruibile — da Outlook, o dall'archivio da cui era stato estratto.

// Cache e' il custode di `_contenuti`.
type Cache struct {
	Pool    *pgxpool.Pool
	Staging string
	Log     *slog.Logger
	// Retention: da quanto un contenuto deve essere fermo — ne' toccato sul disco ne' nominato di
	// recente in database — perche' si possa togliere. Zero = non togliere niente per eta'.
	Retention time.Duration
	// MaxByte: oltre quanto la cache toglie i contenuti meno usati anche se non sono ancora
	// vecchi, partendo dal meno usato, finche' non torna sotto. Zero = nessun limite.
	MaxByte int64
	// EtaMinima: un file piu' giovane di cosi' non si tocca MAI, nemmeno oltre la capienza. Fra la
	// rinomina di un contenuto e il COMMIT che scrive l'allegato passa un istante in cui quel
	// contenuto non e' nominato da nessuno: senza questa attesa lo si cancellerebbe mentre nasce.
	// Zero = dieci minuti.
	EtaMinima time.Duration
	// Ogni: intervallo fra una passata e l'altra. Zero = sei ore. E' una pulizia, non una reazione.
	Ogni time.Duration
	// Adesso: l'orologio. Nil = time.Now. E' un campo perche' la prova deve poter far invecchiare un
	// contenuto di quaranta giorni senza aspettarli.
	Adesso func() time.Time
	// CopiaNelDeposito (V1, EM §10.5): dice se il file di una copia di conservazione verificata c'e' nel deposito degli
	// «Altri file» con la dimensione attesa. Una proposta aperta di un'occorrenza in Altri file smette di tenere il
	// contenuto nello staging solo quando la sua copia c'e' davvero. Nil = deposito non configurato: la proposta resta
	// un pin (il lato che non cancella).
	CopiaNelDeposito func(percorsoRelativo string, depositoID uuid.UUID, bytes int64) bool
	// PrimaDiRimuovere: solo per le prove (PM23 CO-07, CO-11, CO-15): si chiama fra la passata che decide e la
	// transazione che cancella, cioe' nel punto in cui un caricamento o una «Riattiva» possono arrivare. Nil in
	// produzione.
	PrimaDiRimuovere func(sha string)
}

// MotivoPropostaAltriFile e' il pin «proposta aperta» di un'occorrenza in Altri file (ListContenutiPinnati, ordine 6):
// vale solo finche' il file della sua copia verificata non e' nel deposito (EM §10.5).
const MotivoPropostaAltriFile = "proposta aperta in Altri file"

// EsitoCache e' che cosa ha fatto una passata.
type EsitoCache struct {
	Contenuti int   // file trovati sotto _contenuti
	Byte      int64 // quanto occupavano prima della passata
	Pinnati   int   // contenuti che qualcuno sta ancora usando
	Rimossi   int
	Liberati  int64
	// Saltato: se non e' vuoto, la passata NON e' stata fatta, e questo dice perche'.
	Saltato string
}

func (c *Cache) adesso() time.Time {
	if c.Adesso != nil {
		return c.Adesso()
	}
	return time.Now()
}

func (c *Cache) etaMinima() time.Duration {
	if c.EtaMinima > 0 {
		return c.EtaMinima
	}
	return 10 * time.Minute
}

// Avvia fa la prima passata dopo due minuti — non subito: all'avvio il database e i worker hanno
// cose piu' urgenti — e poi una ogni Ogni.
func (c *Cache) Avvia(ctx context.Context) {
	if strings.TrimSpace(c.Staging) == "" {
		c.Log.Info("cache dei contenuti: nessuno staging, custode non avviato")
		return
	}
	ogni := c.Ogni
	if ogni <= 0 {
		ogni = 6 * time.Hour
	}
	c.Log.Info("cache dei contenuti: custode avviato", "retention", durataCache(c.Retention), "capienza", capienza(c.MaxByte), "ogni", ogni)
	go func() {
		primo := time.NewTimer(2 * time.Minute)
		defer primo.Stop()
		select {
		case <-ctx.Done():
			return
		case <-primo.C:
		}
		c.giroELog(ctx)
		t := time.NewTicker(ogni)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.giroELog(ctx)
			}
		}
	}()
}

func (c *Cache) giroELog(ctx context.Context) {
	e, err := c.Giro(ctx)
	switch {
	case err != nil:
		c.Log.Error("cache dei contenuti: passata fallita", "err", err)
	case e.Saltato != "":
		c.Log.Info("cache dei contenuti: passata saltata", "perche'", e.Saltato)
	case e.Rimossi > 0:
		c.Log.Info("cache dei contenuti: passata conclusa", "contenuti", e.Contenuti, "MB", e.Byte>>20,
			"pinnati", e.Pinnati, "rimossi", e.Rimossi, "MB_liberati", e.Liberati>>20)
	default:
		c.Log.Info("cache dei contenuti: niente da togliere", "contenuti", e.Contenuti, "MB", e.Byte>>20, "pinnati", e.Pinnati)
	}
}

// contenuto e' un file sotto _contenuti visto dalla passata.
type contenuto struct {
	percorso  string
	sha       string
	bytes     int64
	ultimoUso time.Time // la piu' recente fra l'orario del file e l'ultimo uso in database
	pin       string    // perche' non si puo' togliere; vuoto = libero
}

// Giro fa una passata: guarda tutti i contenuti, chiede al database chi serve ancora e da quanto
// non si usa, e toglie quelli liberi che sono vecchi o che eccedono la capienza.
//
// Prima si raccoglie, poi si chiede, poi si cancella: chiedere al database un file alla volta
// mentre si cammina sul disco vorrebbe dire una query per contenuto, cioe' un quarto d'ora di
// interrogazioni per non cancellare niente.
func (c *Cache) Giro(ctx context.Context) (EsitoCache, error) {
	var e EsitoCache
	if strings.TrimSpace(c.Staging) == "" {
		e.Saltato = "nessuno staging configurato"
		return e, nil
	}
	radice := filepath.Join(c.Staging, CartellaContenuti)
	if st, err := os.Stat(radice); err != nil || !st.IsDir() {
		e.Saltato = "la cartella " + CartellaContenuti + " non esiste ancora"
		return e, nil
	}
	contenuti, err := c.raccogli(radice)
	if err != nil {
		return e, err
	}
	e.Contenuti = len(contenuti)
	for _, k := range contenuti {
		e.Byte += k.bytes
	}
	if len(contenuti) == 0 {
		return e, nil
	}
	q := db.New(c.Pool)
	if err := c.interroga(ctx, q, contenuti); err != nil {
		return e, err
	}

	adesso := c.adesso()
	sort.Slice(contenuti, func(i, j int) bool { return contenuti[i].ultimoUso.Before(contenuti[j].ultimoUso) })
	totale := e.Byte
	for _, k := range contenuti {
		if k.pin != "" {
			e.Pinnati++
			continue
		}
		if adesso.Sub(k.ultimoUso) < c.etaMinima() {
			continue // sta nascendo: non si tocca, qualunque sia la capienza
		}
		var motivo string
		switch {
		case c.Retention > 0 && adesso.Sub(k.ultimoUso) > c.Retention:
			motivo = fmt.Sprintf("fermo da %d giorni (retention %d)", int(adesso.Sub(k.ultimoUso).Hours()/24), int(c.Retention.Hours()/24))
		case c.MaxByte > 0 && totale > c.MaxByte:
			motivo = fmt.Sprintf("cache oltre la capienza (%d MB > %d MB), il meno usato", totale>>20, c.MaxByte>>20)
		default:
			continue
		}
		if c.PrimaDiRimuovere != nil {
			c.PrimaDiRimuovere(k.sha)
		}
		n, err := c.rimuovi(ctx, k, c.Retention > 0 && adesso.Sub(k.ultimoUso) > c.Retention)
		if errors.Is(err, errNonPiuRimovibile) {
			c.Log.Info("contenuto non rimosso dalla cache: ricontrollato sotto il lucchetto, serve ancora", "sha", k.sha[:12], "perche'", err.Error())
			continue
		}
		if err != nil {
			c.Log.Warn("contenuto non rimosso dalla cache", "sha", k.sha[:12], "err", err)
			continue
		}
		totale -= k.bytes
		e.Rimossi++
		e.Liberati += k.bytes
		c.Log.Info("contenuto rimosso dalla cache", "sha", k.sha[:12], "byte", k.bytes, "motivo", motivo,
			"allegati_toccati", n, "ultimo_uso", k.ultimoUso.Format("02/01/2006 15:04"))
	}
	return e, nil
}

// raccogli cammina sotto _contenuti. Un file che non si chiama come un hash non l'abbiamo scritto
// noi, e non lo cancelliamo noi. Lo staging vecchio, per messaggio, sta fuori da _contenuti e non
// viene nemmeno guardato.
func (c *Cache) raccogli(radice string) ([]*contenuto, error) {
	var out []*contenuto
	err := filepath.WalkDir(radice, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // una cartella illeggibile non ferma la passata sulle altre
		}
		sha := strings.ToLower(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())))
		if !reSha256.MatchString(sha) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, &contenuto{percorso: p, sha: sha, bytes: info.Size(), ultimoUso: info.ModTime()})
		return nil
	})
	return out, err
}

// interroga chiede al database, per tutti i contenuti in una volta: da quanto non si usano e chi
// li tiene fermi. La dipendenza archivio → voci si risolve qui: un archivio e' pinnato se lo e' una
// sua voce, anche una voce che sul disco non c'e' piu' — proprio perche' da lui si riestrae.
func (c *Cache) interroga(ctx context.Context, q *db.Queries, contenuti []*contenuto) error {
	perSha := make(map[string]*contenuto, len(contenuti))
	sha := make([]string, 0, len(contenuti))
	for _, k := range contenuti {
		perSha[k.sha] = k
		sha = append(sha, k.sha)
	}
	usi, err := q.UltimoUsoContenuti(ctx, sha)
	if err != nil {
		return fmt.Errorf("ultimo uso dei contenuti: %w", err)
	}
	for _, u := range usi {
		if k := perSha[strings.TrimSpace(u.Sha256)]; k != nil && u.UltimoUso.After(k.ultimoUso) {
			k.ultimoUso = u.UltimoUso
		}
	}
	figliDi, err := VociDiArchivi(ctx, q, sha)
	if err != nil {
		return err
	}
	daChiedere := sha
	for _, figli := range figliDi {
		for _, f := range figli {
			if _, c := perSha[f]; !c {
				daChiedere = append(daChiedere, f)
			}
		}
	}
	motivi, err := PinDiContenuti(ctx, q, daChiedere, c.CopiaNelDeposito)
	if err != nil {
		return err
	}
	for _, k := range contenuti {
		k.pin = motivi[k.sha]
		if k.pin != "" {
			continue
		}
		k.pin = PinDiUnaVoce(figliDi[k.sha], motivi)
	}
	return nil
}

// VociDiArchivi: per ogni archivio fra questi contenuti, gli sha delle voci estratte da lui (ListVociDiArchivio), in
// ordine. E' la meta' «archivio → voci» della funzione dei pin: la usano la passata qui sopra e il tempo 2 dello scarto
// (V1, EM §10.4 passo 8: «i pin si calcolano con la stessa funzione della cache, con la regola archivio → voci»).
func VociDiArchivi(ctx context.Context, q *db.Queries, sha []string) (map[string][]string, error) {
	voci, err := q.ListVociDiArchivio(ctx, sha)
	if err != nil {
		return nil, fmt.Errorf("voci degli archivi: %w", err)
	}
	figliDi := map[string][]string{}
	for _, v := range voci {
		z, f := strings.TrimSpace(v.Archivio), strings.TrimSpace(v.Voce)
		figliDi[z] = append(figliDi[z], f)
	}
	for z := range figliDi {
		sort.Strings(figliDi[z])
	}
	return figliDi, nil
}

// PinDiUnaVoce: il pin di un archivio per la regola archivio → voci: la prima voce (in ordine) con un pin, o "". Un
// archivio e' pinnato se lo e' una sua voce, anche una voce che sul disco non c'e' piu': da lui si riestrae.
func PinDiUnaVoce(voci []string, motivi map[string]string) string {
	for _, f := range voci {
		if m := motivi[f]; m != "" {
			return "archivio con una voce ancora in uso (" + m + ")"
		}
	}
	return ""
}

// PinDiContenuti: i pin di questi contenuti con la stessa funzione della passata, cioe' ListContenutiPinnati e il pin
// «proposta aperta in Altri file» che cade quando la copia verificata e' nel deposito (copiaNelDeposito; nil = deposito
// non configurato: il pin resta). Senza la regola archivio → voci, che applica chi chiama (VociDiArchivi,
// PinDiUnaVoce). Un contenuto senza pin non sta nella mappa.
func PinDiContenuti(ctx context.Context, q *db.Queries, sha []string, copiaNelDeposito func(string, uuid.UUID, int64) bool) (map[string]string, error) {
	pin, err := q.ListContenutiPinnati(ctx, sha)
	if err != nil {
		return nil, fmt.Errorf("contenuti pinnati: %w", err)
	}
	motivi := make(map[string]string, len(pin))
	var altriFile []string
	for _, p := range pin {
		s := strings.TrimSpace(p.Sha256)
		motivi[s] = p.Motivo
		if p.Motivo == MotivoPropostaAltriFile {
			altriFile = append(altriFile, s)
		}
	}
	if err := sciogliAltriFile(ctx, q, copiaNelDeposito, altriFile, motivi); err != nil {
		return nil, err
	}
	return motivi, nil
}

// sciogliAltriFile: per i contenuti il cui unico pin e' una proposta aperta di un'occorrenza in Altri file, il pin
// cade se OGNI copia verificata di quelle occorrenze c'e' nel deposito con la dimensione attesa (EM §10.5). Se il file
// del deposito manca, il pin resta: la cache non toglie lo staging di un file in Altri file senza byte nel deposito
// (PM23 AF-15), e la Sorgente segnala l'anomalia.
func sciogliAltriFile(ctx context.Context, q *db.Queries, copiaNelDeposito func(string, uuid.UUID, int64) bool, sha []string,
	motivi map[string]string) error {
	if len(sha) == 0 || copiaNelDeposito == nil {
		return nil
	}
	copie, err := q.ListConservazioniCopiatePerSha(ctx, sha)
	if err != nil {
		return fmt.Errorf("copie di conservazione dei file in Altri file: %w", err)
	}
	trovate := map[string]bool{}
	mancante := map[string]bool{}
	for _, cp := range copie {
		s := strings.TrimSpace(cp.Sha256)
		trovate[s] = true
		if !cp.Bytes.Valid || !copiaNelDeposito(cp.PercorsoRelativo, cp.DepositoID, cp.Bytes.Int64) {
			mancante[s] = true
		}
	}
	for _, s := range sha {
		if trovate[s] && !mancante[s] {
			delete(motivi, s)
		}
	}
	return nil
}

// errNonPiuRimovibile: ricontrollato sotto BloccaContenuto, il contenuto serve ancora (un pin nuovo, un riferimento
// appena scritto, un file rinfrescato). Non e' un errore: la passata va avanti.
var errNonPiuRimovibile = errors.New("contenuto di nuovo in uso")

// rimuovi toglie un contenuto: prima gli allegati che nominano QUEL file perdono il percorso, poi il file sparisce, e
// le due cose stanno nella stessa transazione. Se il file non si lascia cancellare, il database non cambia. Se il commit
// fallisce dopo la cancellazione resta un percorso che non porta a niente: e' la condizione che il blocco 4C gia'
// gestisce («contenuto non piu' in staging»).
//
// V1 (EM §10.6; giro 1, TR-09): chi cancella ricalcola sotto il lucchetto per contenuto (BloccaContenuto), nella
// transazione che cancella, pin, eta' e riferimenti, con la stessa funzione della passata (interroga, con la regola
// archivio → voci), prima di os.Remove. La passata ha deciso con dati di qualche istante prima: un caricamento, un
// riuso del gemello o una «Riattiva» che nel frattempo hanno adottato il contenuto prendono lo stesso lucchetto, e qui
// si vede. perEta: la passata l'ha scelto per la retention (va ricontrollata), non per la capienza.
func (c *Cache) rimuovi(ctx context.Context, k *contenuto, perEta bool) (int64, error) {
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if err := q.BloccaContenuto(ctx, k.sha); err != nil {
		return 0, err
	}
	ora := &contenuto{percorso: k.percorso, sha: k.sha, bytes: k.bytes}
	st, err := os.Stat(k.percorso)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// gia' sparito: restano solo i riferimenti a QUEL file, che non portano piu' a niente
		ora.ultimoUso = time.Time{}
	case err != nil:
		return 0, err
	default:
		ora.ultimoUso = st.ModTime()
	}
	if err := c.interroga(ctx, q, []*contenuto{ora}); err != nil {
		return 0, err
	}
	if st != nil {
		adesso := c.adesso()
		switch {
		case ora.pin != "":
			return 0, fmt.Errorf("%w: %s", errNonPiuRimovibile, ora.pin)
		case adesso.Sub(ora.ultimoUso) < c.etaMinima():
			return 0, fmt.Errorf("%w: usato da meno di %s", errNonPiuRimovibile, c.etaMinima())
		case perEta && c.Retention > 0 && adesso.Sub(ora.ultimoUso) <= c.Retention:
			return 0, fmt.Errorf("%w: usato dentro la retention", errNonPiuRimovibile)
		}
	}
	n, err := q.AzzeraPathStagingPerFile(ctx, db.AzzeraPathStagingPerFileParams{Sha256: k.sha, Percorso: k.percorso})
	if err != nil {
		return 0, err
	}
	if err := os.Remove(k.percorso); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return n, tx.Commit(ctx)
}

// ToccaContenuto rinfresca l'orario di un contenuto appena usato — letto per una copia sul NAS —
// cosi' la cache sa che serve ancora. Best effort: un orario non aggiornato accorcia la vita del
// file di qualche giorno, non lo rompe.
func ToccaContenuto(percorso string) {
	if percorso == "" {
		return
	}
	adesso := time.Now()
	_ = os.Chtimes(percorso, adesso, adesso)
}

func durataCache(d time.Duration) string {
	if d <= 0 {
		return "mai per eta'"
	}
	return fmt.Sprintf("%d giorni", int(d.Hours()/24))
}

func capienza(b int64) string {
	if b <= 0 {
		return "illimitata"
	}
	return fmt.Sprintf("%d MB", b>>20)
}
