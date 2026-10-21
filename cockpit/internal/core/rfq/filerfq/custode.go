package filerfq

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// Custode e' il custode del ciclo dei file (EM §10.4, «Chi esegue il tempo 2»; §10.5): accanto a quello della cache,
// prima passata 2 minuti dopo l'avvio, poi ogni 15 minuti. Riprende le rimozioni e i rilasci rimasti a meta' (con le
// attese crescenti scritte nelle righe), porta in rilascio le copie in corso con il battito fermo da oltre 15 minuti
// (mai una copia viva), toglie i parziali delle copie piu' vecchi di 24 ore (non sono di nessuna copia viva) e i parziali
// dello staging del ciclo dei file rimasti da un processo morto, e una volta al giorno fa il controllo della bonifica
// degli errori non ritentabili. Riparte dopo un riavvio: lo stato sta nel database.
//
// Una passata prende TUTTE le righe scadute, una pagina di Limite righe alla volta, per chiave: dopo lo «Scarta» di un
// archivio di 1.801 voci (PM23 SC-18) il tempo 2 e i rilasci finiscono nella passata dopo, non in nove ore (rilievo
// della revisione). Una riga che resta da riprendere non torna nella stessa passata: niente giri senza fine.
type Custode struct {
	S *Servizio
	// Ogni: fra una passata e l'altra (zero = 15 minuti). Prima: la prima passata dopo l'avvio (zero = 2 minuti).
	Ogni  time.Duration
	Prima time.Duration
	// Limite: quante righe per pagina (zero = 50). La passata scorre tutte le pagine.
	Limite int32

	mu             sync.Mutex
	ultimaBonifica time.Time
}

// EsitoCustode: che cosa ha fatto una passata.
type EsitoCustode struct {
	Rimozioni           int
	Rilasci             int
	TempiScaduti        int
	Bonificate          int
	PartiRimosse        int // parziali delle copie degli Altri file
	PartiStagingRimosse int // parziali dello staging del ciclo dei file (PrefissoParteCiclo)
	BonificaFatta       bool
	ErroriRighe         int
	ConservazioneX      bool // nessun conservatore: niente parziali (i rilasci aspettano)
}

func (c *Custode) limite() int32 {
	if c.Limite > 0 {
		return c.Limite
	}
	return 50
}

// Avvia fa partire il custode in una goroutine.
func (c *Custode) Avvia(ctx context.Context) {
	ogni := c.Ogni
	if ogni <= 0 {
		ogni = 15 * time.Minute
	}
	prima := c.Prima
	if prima <= 0 {
		prima = 2 * time.Minute
	}
	c.S.log().Info("ciclo dei file: custode avviato", "ogni", ogni, "conservatore", c.S.Conservatore != nil)
	go func() {
		t0 := time.NewTimer(prima)
		defer t0.Stop()
		select {
		case <-ctx.Done():
			return
		case <-t0.C:
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

func (c *Custode) giroELog(ctx context.Context) {
	e, err := c.Giro(ctx)
	if err != nil {
		c.S.log().Error("ciclo dei file: passata del custode non riuscita", "err", err)
		return
	}
	if e.Rimozioni+e.Rilasci+e.TempiScaduti+e.Bonificate+e.PartiRimosse+e.PartiStagingRimosse > 0 {
		c.S.log().Info("ciclo dei file: passata del custode", "rimozioni", e.Rimozioni, "rilasci", e.Rilasci,
			"tempi_scaduti", e.TempiScaduti, "bonificate", e.Bonificate, "parziali_tolti", e.PartiRimosse,
			"parziali_staging_tolti", e.PartiStagingRimosse)
	}
}

// Giro fa una passata del custode.
func (c *Custode) Giro(ctx context.Context) (EsitoCustode, error) {
	var e EsitoCustode
	s := c.S
	q := db.New(s.Pool)
	adesso := s.adesso()
	// le rimozioni da riprendere, tutte, una pagina alla volta
	for dopo := int64(0); ; {
		ids, err := q.ListIdRimozioniDaRiprendere(ctx, db.ListIdRimozioniDaRiprendereParams{Adesso: adesso, Dopo: dopo, Limite: c.limite()})
		if err != nil {
			return e, err
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				return e, ctx.Err()
			}
			p, err := s.EseguiRimozione(ctx, id, true)
			if err != nil {
				e.ErroriRighe++
				s.log().Warn("ciclo dei file: passo della rimozione non riuscito", "rimozione", id, "err", err)
				continue
			}
			if !p.Saltato {
				e.Rimozioni++
			}
		}
		if len(ids) < int(c.limite()) {
			break
		}
		dopo = ids[len(ids)-1]
	}
	// le copie da riprendere: le in_copia con il battito fermo vanno in rilascio (tempo_scaduto), poi i rilasci; tutte,
	// una pagina alla volta
	for dopo := int64(0); ; {
		cids, err := q.ListIdConservazioniDaRiprendere(ctx, db.ListIdConservazioniDaRiprendereParams{Adesso: adesso, Dopo: dopo, Limite: c.limite()})
		if err != nil {
			return e, err
		}
		for _, id := range cids {
			if ctx.Err() != nil {
				return e, ctx.Err()
			}
			scaduto, err := c.chiudiTempoScaduto(ctx, id)
			if err != nil {
				e.ErroriRighe++
				s.log().Warn("ciclo dei file: copia con il battito fermo non chiusa", "conservazione", id, "err", err)
				continue
			}
			if scaduto {
				e.TempiScaduti++
			}
			p, err := s.RilasciaCopia(ctx, id, true)
			if err != nil {
				e.ErroriRighe++
				s.log().Warn("ciclo dei file: rilascio non riuscito", "conservazione", id, "err", err)
				continue
			}
			if p.Stato == "rilasciata" {
				e.Rilasci++
			}
		}
		if len(cids) < int(c.limite()) {
			break
		}
		dopo = cids[len(cids)-1]
	}
	// i parziali dello staging scritti dal ciclo dei file (E1′, «Riattiva») e rimasti da un processo morto
	e.PartiStagingRimosse = PulisciPartiCiclo(s.Staging, adesso, s.log())
	// la bonifica, una volta al giorno
	c.mu.Lock()
	daFare := c.ultimaBonifica.IsZero() || adesso.Sub(c.ultimaBonifica) >= 24*time.Hour
	if daFare {
		c.ultimaBonifica = adesso
	}
	c.mu.Unlock()
	if daFare {
		n, err := c.Bonifica(ctx)
		if err != nil {
			return e, err
		}
		e.Bonificate, e.BonificaFatta = n, true
	}
	// i parziali delle copie piu' vecchi di 24 ore
	if s.Conservatore == nil {
		e.ConservazioneX = true
		return e, nil
	}
	e.PartiRimosse = c.pulisciParti(ctx, q, adesso)
	return e, nil
}

// TokenViviAltro: i token dei tentativi vivi che scrivono nella cartella ALTRO di una RFQ (U2 §3.9): il lease dei job
// in corso (i lavori di copia dei documenti, documenti.PulisciPartiScadute) e il token delle copie degli Altri file
// in_copia. Chi pulisce i parziali di quella cartella non tocca un parziale con uno di questi token.
func TokenViviAltro(ctx context.Context, q *db.Queries) (map[uuid.UUID]bool, error) {
	vivi := map[uuid.UUID]bool{}
	lavori, err := q.ListLeaseTokenInCorso(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range lavori {
		if t.Valid {
			vivi[t.UUID] = true
		}
	}
	copie, err := q.ListTokenConservazioniInCopia(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range copie {
		vivi[t] = true
	}
	return vivi, nil
}

// chiudiTempoScaduto porta a da_rilasciare (causa tempo_scaduto) una riga in_copia con il battito fermo da oltre 15
// minuti, ricontrollato con la riga presa: mai una copia viva, anche se iniziata da oltre 15 minuti (PM23 CO-16).
func (c *Custode) chiudiTempoScaduto(ctx context.Context, id int64) (bool, error) {
	s := c.S
	chiusa := false
	err := s.inTx(ctx, func(q *db.Queries) error {
		r, err := q.GetConservazioneSeLibera(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if r.Stato != "in_copia" || s.adesso().Sub(battitoDi(r)) < battitoFermo {
			return nil
		}
		chiusa = true
		return s.passoConservazione(ctx, q, r, "da_rilasciare", pgtype.Text{String: "tempo_scaduto", Valid: true}, pgtype.Text{})
	})
	return chiusa, err
}

// Bonifica e' il controllo giornaliero degli errori non ritentabili (EM §10.4; giro 2, NV-02): per OGNI riga in errore
// non ritentabile ricostruisce il percorso da percorso_trovato e lo controlla sotto la radice dello staging della
// configurazione, con lo stesso contenimento pulito e senza maiuscole di EContenuto. Se il file non esiste piu', la
// riga passa a eseguita / bonificato con l'evento rimozione_eseguita; se c'e' ancora, resta in errore. Un percorso
// assente o fuori dalla radice non si controlla: il custode non legge il disco fuori dalla radice (limite dichiarato).
// Le righe si leggono una pagina alla volta, per chiave, fino all'ultima: le prime che restano in errore (i loro file
// ci sono ancora) non nascondono quelle dopo (rilievo bloccante della revisione; il trigger della 0023 non lascia
// toccare una riga non ritentabile se non per chiuderla, quindi la rotazione non puo' stare nelle righe).
func (c *Custode) Bonifica(ctx context.Context) (int, error) {
	s := c.S
	if strings.TrimSpace(s.Staging) == "" {
		return 0, nil
	}
	n := 0
	for dopo := int64(0); ; {
		ids, err := db.New(s.Pool).ListIdRimozioniNonRitentabili(ctx, db.ListIdRimozioniNonRitentabiliParams{Dopo: dopo, Limite: c.limite()})
		if err != nil {
			return n, err
		}
		for _, id := range ids {
			fatta, err := c.bonificaUna(ctx, id)
			if err != nil {
				return n, err
			}
			if fatta {
				n++
			}
		}
		if len(ids) < int(c.limite()) {
			return n, nil
		}
		dopo = ids[len(ids)-1]
	}
}

// bonificaUna: il controllo della bonifica di una riga, nella sua transazione (la riga SKIP LOCKED).
func (c *Custode) bonificaUna(ctx context.Context, id int64) (bool, error) {
	s := c.S
	fatta := false
	err := s.inTx(ctx, func(q *db.Queries) error {
		r, err := q.GetRimozioneSeLibera(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if r.Stato != "errore" || r.Ritentabile || !r.PercorsoTrovato.Valid {
			return nil
		}
		p := filepath.Clean(r.PercorsoTrovato.String)
		if !sottoRadice(s.Staging, p) {
			return nil
		}
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			return nil // il file c'e' ancora (o non si sa): la riga resta errore
		}
		fatta = true
		return s.passoRimozione(ctx, q, r, "eseguita", "bonificato", pgtype.Text{}, pgtype.Text{})
	})
	return fatta, err
}

// pulisciParti toglie i parziali <nome>.parte.<token> piu' vecchi di 24 ore e non vivi (U2 §3.9), nelle cartelle ALTRO
// delle righe di conservazione toccate negli ultimi 7 giorni. I vivi sono l'unione dei token dei lavori di copia dei
// documenti e di quelli delle copie in_copia (TokenViviAltro): nella cartella ALTRO scrivono tutti e due.
func (c *Custode) pulisciParti(ctx context.Context, q *db.Queries, adesso time.Time) int {
	righe, err := q.ListConservazioniRecenti(ctx, adesso.Add(-7*24*time.Hour))
	if err != nil {
		c.S.log().Warn("ciclo dei file: le cartelle delle copie non si leggono", "err", err)
		return 0
	}
	// i token vivi della cartella ALTRO: l'unione dei tentativi dei lavori di copia dei documenti (il lease dei job in
	// corso) e delle copie degli Altri file in_copia (U2 §3.6 regola 6, §3.9; prova AF-33)
	vivi, err := TokenViviAltro(ctx, q)
	if err != nil {
		c.S.log().Warn("ciclo dei file: i token vivi dei parziali non si leggono", "err", err)
		return 0
	}
	viste := map[string]bool{}
	n := 0
	for _, r := range righe {
		p := postoDi(r.RadiceID, r.CartellaThread.String, r.PercorsoRelativo, r.Sha256, -1)
		cartella := strings.ToLower(r.CartellaThread.String + `\` + filepath.Dir(strings.ReplaceAll(r.PercorsoRelativo, "/", `\`)))
		k := r.RadiceID.String() + "|" + cartella
		if viste[k] || r.RadiceID != c.S.Conservatore.RadiceID() {
			continue
		}
		viste[k] = true
		tolti, err := c.S.Conservatore.PulisciPartiCopie(p, vivi, adesso.Add(-24*time.Hour))
		if err != nil {
			continue
		}
		n += tolti
	}
	return n
}
