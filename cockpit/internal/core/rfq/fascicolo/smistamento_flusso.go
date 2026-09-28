package fascicolo

// Il flusso ancorato al prodotto contro il database (Smistamento F8, addendum A5.13.6-A5.13.7): le letture
// dello stato, la scrittura delle destinazioni, le priorita' delle analisi, gli inneschi.
//
// Il flusso scrive UNA cosa: documento_proposta.dettagli.destinazione delle righe esistenti e aperte (FP3),
// con ScriviDestinazione, che non riscrive una destinazione uguale e non tocca una proposta decisa (la guardia
// sta nel WHERE, P34). Nessuna INSERT: il perimetro dei file non cambia e v_thread_da_riesaminare non si
// accende. Mai tipo, codice, rev, confidenza, fonte o componente_id della proposta (li scrive la lettura, o la
// decisione); mai componente, componente_relazione, documento, identificativo_thread, una marcatura: la T non
// diventa L senza un confine umano (decisioni del 27/09 «bis»). E abbassa il numero di priorita' delle analisi
// pronte degli STEP e dei PDF compatibili per nome con un prodotto, perche' passino prima.
//
// Gli inneschi sono solo eventi (IN1-IN9): ognuno gira DOPO il commit dell'evento, in una transazione sua con
// la RFQ bloccata, e un suo errore finisce nel log senza disfare l'evento (FP6, P31). Nessuna GET scrive: la
// GET sa soltanto dire che le destinazioni non sono aggiornate (DestinazioniDaAggiornare), e il POST
// «Aggiorna le proposte» le rifa'.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// LeggiStatoFlusso legge tutto cio' che il flusso vuole sapere della RFQ, e niente di piu': letture soltanto.
// Nessun fatto grezzo del worker: della struttura di uno STEP le righe di proposta, del file la valutazione
// (ValutazioneDellaRiga, che per le righe di prima la ricostruisce), del contenuto di un PDF le evidenze
// normalizzate (EvidenzeContenutoPDF); dell'analisi, solo se i fatti correnti ci sono e com'e' finito l'ultimo
// job.
func LeggiStatoFlusso(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore) (StatoFlusso, error) {
	var s StatoFlusso
	tid := uuid.NullUUID{UUID: thread, Valid: true}
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return s, err
	}
	cl, err := q.GetCliente(ctx, t.ClienteID)
	if err != nil {
		return s, err
	}
	lette, _ := regole.LeggiRegole(cl.Regole)
	s.Motore = classificazione.Compila(cl.RagioneSociale, lette)
	h := sha256.Sum256(cl.Regole)
	s.ImprontaRegole = hex.EncodeToString(h[:])
	if s.Identificativi, err = q.ListIdentificativi(ctx, thread); err != nil {
		return s, err
	}
	if s.Componenti, err = q.ListComponentiThread(ctx, thread); err != nil {
		return s, err
	}
	if s.Relazioni, err = q.ListRelazioniAttive(ctx, thread); err != nil {
		return s, err
	}
	nodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return s, err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return s, err
	}
	s.Nodi, s.Archi = soloNodi(nodi), soloArchi(archi)
	if s.Dichiarazioni, err = LeggiDichiarazioni(ctx, q, thread); err != nil {
		return s, err
	}
	if _, s.BomCongelata, err = WorkingBloccata(ctx, q, thread); err != nil {
		return s, err
	}

	file, err := q.ListFileDelFlusso(ctx, tid)
	if err != nil {
		return s, err
	}
	proposte, err := q.ListProposteDocumentoThread(ctx, tid)
	if err != nil {
		return s, err
	}
	perAllegato := map[uuid.UUID]db.DocumentoProposta{}
	for _, p := range proposte {
		perAllegato[p.AllegatoID] = p
	}
	correnti := map[string]bool{}
	shas, err := q.ListShaConFattiCorrenti(ctx, tid)
	if err != nil {
		return s, err
	}
	for _, sha := range shas {
		correnti[sha] = true
	}
	lavoro, err := q.ListLavoroPendenteRfq(ctx, tid)
	if err != nil {
		return s, err
	}
	inLavoro := map[uuid.UUID]bool{}
	for _, l := range lavoro {
		inLavoro[l.AllegatoID] = true
	}
	ultimi := map[string]db.StatoJob{}
	if an.Versione != 0 {
		var chiavi []string
		for _, f := range file {
			if f.Sha256.Valid && f.Sha256.String != "" {
				chiavi = append(chiavi, coda.ChiaveAnalisi(f.Sha256.String, an))
			}
		}
		if len(chiavi) > 0 {
			jj, err := q.ListUltimiJobPerChiavi(ctx, chiavi)
			if err != nil {
				return s, err
			}
			for _, j := range jj {
				ultimi[j.Chiave] = j.Stato
			}
		}
	}
	for _, r := range file {
		f := FileFlusso{AllegatoID: r.AllegatoID, Sha: r.Sha256.String, Nome: r.NomeFile, Estensione: r.Estensione.String,
			PathInterno: r.PathInterno.String, Zip: r.NomeContenitore.String, RicevutoIl: r.RicevutoIl, Contenitore: r.Contenitore,
			DelCliente: r.DelCliente}
		switch {
		case f.Sha != "" && correnti[f.Sha]:
			f.Analisi = AnalisiCorrente
		case inLavoro[f.AllegatoID]:
			f.Analisi = AnalisiInCorso
		case f.Sha != "" && an.Versione != 0 && ultimi[coda.ChiaveAnalisi(f.Sha, an)] == db.StatoJobFallito:
			f.Analisi = AnalisiFallita
		default:
			f.Analisi = AnalisiAssente
		}
		if p, ok := perAllegato[r.AllegatoID]; ok {
			f.Proposta = propostaFile(p, r.NomeFile, r.Estensione.String)
		}
		if f.pdf() && f.Sha != "" {
			if f.EvidenzePDF, err = EvidenzeContenutoPDF(ctx, q, f.Sha, s.Motore); err != nil {
				return s, err
			}
		}
		s.File = append(s.File, f)
	}
	return s, nil
}

// propostaFile legge della riga di documento_proposta cio' che il flusso usa: lo stato, la pre-assegnazione
// di prima, la valutazione e la firma della destinazione scritta.
func propostaFile(p db.DocumentoProposta, nome, ext string) *PropostaFile {
	pf := &PropostaFile{PropostaID: p.PropostaID, Aperta: p.Stato == db.StatoPropostaAperta,
		Esclusa: p.Stato == db.StatoPropostaScartata && p.DecisoDa.Valid, ComponenteID: p.ComponenteID,
		Valutazione: classificazione.ValutazioneDellaRiga(string(p.TipoProposto), p.Codice.String, p.Rev.String, string(p.Fonte),
			int(p.Confidenza), p.Dettagli, nome, ext)}
	var dd struct {
		Destinazione *struct {
			IndiceFirma string `json:"indice_firma"`
			Firma       string `json:"firma"`
		} `json:"destinazione"`
	}
	if json.Unmarshal(p.Dettagli, &dd) == nil && dd.Destinazione != nil {
		pf.IndiceFirma, pf.Firma = dd.Destinazione.IndiceFirma, dd.Destinazione.Firma
	}
	return pf
}

// EsitoRismista dice che cosa ha fatto un giro del flusso su una RFQ.
type EsitoRismista struct {
	File        int    // destinazioni calcolate
	Scritte     int64  // destinazioni riscritte (le altre erano gia' cosi', o la proposta e' decisa)
	Priorita    int64  // analisi passate avanti
	TuttaLaRfq  bool   // il giro ha rifatto tutta la RFQ (anche quando l'evento era di un file solo)
	Saltata     string // perche' la RFQ non si rismista ("" = rismistata)
	IndiceFirma string
}

// RismistaRfq e' un giro del flusso sulla RFQ, nella transazione di chi chiama, con la RFQ bloccata (le
// decisioni, il congelamento e i giri concorrenti si mettono in fila qui). Una RFQ chiusa o unita a un'altra
// non si rismista (A5.13.7). Con l'ambito di alcuni file si rifa' tutta la RFQ se l'indice e' cambiato rispetto
// a quello su cui sono state calcolate le destinazioni degli altri file (IN5).
func RismistaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID, ambito Ambito, an coda.Analizzatore) (EsitoRismista, error) {
	var es EsitoRismista
	if err := bloccaThread(ctx, q, thread); err != nil {
		return es, err
	}
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return es, err
	}
	switch {
	case t.Stato != db.StatoThreadAPERTA:
		es.Saltata = EsclusaChiusa
		return es, nil
	case t.UnitoIn.Valid:
		es.Saltata = EsclusaUnita
		return es, nil
	}
	s, err := LeggiStatoFlusso(ctx, q, thread, an)
	if err != nil {
		return es, fmt.Errorf("stato del flusso: %w", err)
	}
	p := ProdottiDellaRfq(&s)
	a := Ancore(p, &s)
	ix := IndiceCodici(p, a, &s)
	es.IndiceFirma = ix.Firma
	if !ambito.Rfq {
		for _, f := range s.File {
			if f.Proposta != nil && f.Proposta.Aperta && f.Proposta.IndiceFirma != "" && f.Proposta.IndiceFirma != ix.Firma &&
				!ambito.contiene(f.AllegatoID) {
				ambito = AmbitoRfq()
				break
			}
		}
	}
	es.TuttaLaRfq = ambito.Rfq
	dest := SecondoGiro(ix, &s, ambito)
	for _, f := range s.File {
		x, ok := dest[f.AllegatoID]
		if !ok || !f.Proposta.Aperta {
			continue
		}
		es.File++
		raw, err := json.Marshal(x)
		if err != nil {
			return es, err
		}
		n, err := q.ScriviDestinazione(ctx, db.ScriviDestinazioneParams{PropostaID: f.Proposta.PropostaID, Destinazione: raw, Firma: x.Firma})
		if err != nil {
			return es, fmt.Errorf("destinazione di %s: %w", f.Nome, err)
		}
		es.Scritte += n
	}
	if es.Priorita, err = alzaPrioritaCompatibili(ctx, q, &s, p, an); err != nil {
		return es, err
	}
	return es, nil
}

// alzaPrioritaCompatibili porta avanti le analisi ancora pronte degli STEP e dei PDF il cui nome e'
// compatibile con un prodotto della RFQ (A5.13.6): sono i file da cui l'ancora puo' arrivare.
func alzaPrioritaCompatibili(ctx context.Context, q *db.Queries, s *StatoFlusso, prodotti []Prodotto, an coda.Analizzatore) (int64, error) {
	if an.Versione == 0 {
		return 0, nil
	}
	var chiavi []string
	for _, f := range s.File {
		if f.Analisi != AnalisiInCorso || f.Sha == "" || !f.nelFlusso() || !(f.step() || f.pdf()) {
			continue
		}
		for _, p := range prodotti {
			if CompatibilitaNome(f.Nome, f.PathInterno, f.Zip, p.Codice, s.Motore).Compatibile() {
				chiavi = append(chiavi, coda.ChiaveAnalisi(f.Sha, an))
				break
			}
		}
	}
	return coda.AlzaPriorita(ctx, q, chiavi, coda.PrioritaAnalisiCompatibile)
}

// Compatibili dice quali nomi di file sono compatibili con un prodotto della RFQ: all'estrazione di un
// archivio (IN2) le analisi di quegli STEP e PDF si accodano con la priorita' che le fa passare prima. Il
// valore zero non trova niente compatibile.
type Compatibili struct {
	prodotti []Prodotto
	m        *classificazione.Motore
}

// CompatibiliConIProdotti legge i prodotti confermati della RFQ, con le regole del suo cliente.
func CompatibiliConIProdotti(ctx context.Context, q *db.Queries, thread uuid.UUID) (Compatibili, error) {
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return Compatibili{}, err
	}
	s := StatoFlusso{Motore: m}
	if s.Identificativi, err = q.ListIdentificativi(ctx, thread); err != nil {
		return Compatibili{}, err
	}
	if s.Componenti, err = q.ListComponentiThread(ctx, thread); err != nil {
		return Compatibili{}, err
	}
	return Compatibili{prodotti: ProdottiDellaRfq(&s), m: m}, nil
}

// Compatibile dice se il file (uno STEP o un PDF) ha un nome, una cartella o un archivio compatibili con un
// prodotto.
func (c Compatibili) Compatibile(nome, pathInterno, zip string) bool {
	f := FileFlusso{Nome: nome}
	if !f.step() && !f.pdf() {
		return false
	}
	for _, p := range c.prodotti {
		if CompatibilitaNome(nome, pathInterno, zip, p.Codice, c.m).Compatibile() {
			return true
		}
	}
	return false
}

// Rismista e' RismistaRfq in una transazione sua: la strada degli inneschi, dopo il commit dell'evento.
func Rismista(ctx context.Context, pool *pgxpool.Pool, thread uuid.UUID, ambito Ambito, an coda.Analizzatore) (EsitoRismista, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return EsitoRismista{}, err
	}
	defer tx.Rollback(ctx)
	es, err := RismistaRfq(ctx, db.New(tx), thread, ambito, an)
	if err != nil {
		return es, err
	}
	return es, tx.Commit(ctx)
}

// DestinazioniDaAggiornare dice se le destinazioni scritte non sono calcolate sull'indice di adesso (le regole
// del cliente cambiate, un evento perso fra il commit e il flusso, un file che non ne ha ancora una): e' cio'
// che la GET dello Smistamento sa senza scrivere niente (A5.13.6), per offrire il POST «Aggiorna le proposte».
// Letture soltanto.
func DestinazioniDaAggiornare(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore) (bool, error) {
	s, err := LeggiStatoFlusso(ctx, q, thread, an)
	if err != nil {
		return false, err
	}
	p := ProdottiDellaRfq(&s)
	ix := IndiceCodici(p, Ancore(p, &s), &s)
	for _, f := range s.File {
		if f.Proposta != nil && f.Proposta.Aperta && f.Proposta.IndiceFirma != ix.Firma {
			return true, nil
		}
	}
	return false, nil
}

// ------------------------------------------------------------------ gli inneschi

type chiaveRismistamenti struct{}

// Rismistamenti raccoglie le RFQ (e i file) da rismistare durante un evento, per rismistarle DOPO il suo commit
// (FP6). Chi apre la transazione dell'evento mette la raccolta nel contesto (ConRismistamenti); chi dentro
// l'evento tocca un file o una RFQ la segna (SegnaRismistamento); dopo il commit, Esegui. Senza una raccolta
// nel contesto segnare non fa niente: l'evento non ha un dopo.
type Rismistamenti struct {
	mu  sync.Mutex
	per map[uuid.UUID]*Ambito
}

// NuoviRismistamenti e' una raccolta vuota, fuori da un contesto: per chi sa gia' che cosa rismistare.
func NuoviRismistamenti() *Rismistamenti { return &Rismistamenti{per: map[uuid.UUID]*Ambito{}} }

// ConRismistamenti mette nel contesto una raccolta nuova.
func ConRismistamenti(ctx context.Context) (context.Context, *Rismistamenti) {
	r := NuoviRismistamenti()
	return context.WithValue(ctx, chiaveRismistamenti{}, r), r
}

// SegnaRismistamento segna la RFQ da rismistare: tutta, o solo quei file.
func SegnaRismistamento(ctx context.Context, thread uuid.UUID, allegati ...uuid.UUID) {
	r, ok := ctx.Value(chiaveRismistamenti{}).(*Rismistamenti)
	if !ok || r == nil || thread == uuid.Nil {
		return
	}
	r.Segna(thread, allegati...)
}

// Segna e' SegnaRismistamento sulla raccolta: senza file, tutta la RFQ; la RFQ intera assorbe i file.
func (r *Rismistamenti) Segna(thread uuid.UUID, allegati ...uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.per[thread]
	if !ok {
		a = &Ambito{}
		r.per[thread] = a
	}
	if len(allegati) == 0 {
		a.Rfq, a.File = true, nil
		return
	}
	if a.Rfq {
		return
	}
	for _, x := range allegati {
		if !a.contiene(x) {
			a.File = append(a.File, x)
		}
	}
}

// Vuota dice se non c'e' niente da rismistare.
func (r *Rismistamenti) Vuota() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.per) == 0
}

// Smistatore e' chi fa un giro del flusso su una RFQ: di solito Rismista con il pool e l'analizzatore del
// server. Un parametro, non una chiamata fissa: chi prova che un errore del flusso non disfa l'evento (P31) ne
// mette uno che fallisce.
type Smistatore func(ctx context.Context, thread uuid.UUID, ambito Ambito) error

// SmistatoreDi e' lo Smistatore di sempre: Rismista con quel pool e quell'analizzatore.
func SmistatoreDi(pool *pgxpool.Pool, an coda.Analizzatore) Smistatore {
	return func(ctx context.Context, thread uuid.UUID, ambito Ambito) error {
		_, err := Rismista(ctx, pool, thread, ambito, an)
		return err
	}
}

// Esegui rismista le RFQ raccolte, in ordine di thread_id (nel fan-out le RFQ si bloccano sempre nello stesso
// ordine, e due eventi concorrenti non si incrociano), ognuna nella sua transazione. Un errore si scrive nel
// log e non ferma le altre: l'evento e' gia' salvato, e la GET dira' che le proposte non sono aggiornate.
// Restituisce quante RFQ non si sono potute rismistare.
func (r *Rismistamenti) Esegui(ctx context.Context, fai Smistatore, log *slog.Logger) int {
	r.mu.Lock()
	thread := make([]uuid.UUID, 0, len(r.per))
	ambiti := map[uuid.UUID]Ambito{}
	for t, a := range r.per {
		thread = append(thread, t)
		ambiti[t] = *a
	}
	r.per = map[uuid.UUID]*Ambito{}
	r.mu.Unlock()
	sort.Slice(thread, func(i, j int) bool { return strings.Compare(thread[i].String(), thread[j].String()) < 0 })
	falliti := 0
	for _, t := range thread {
		if err := fai(ctx, t, ambiti[t]); err != nil {
			falliti++
			if log != nil {
				log.Warn("flusso ancorato: destinazioni non aggiornate (l'evento resta salvato; «Aggiorna le proposte» le rifa')",
					"rfq", t, "err", err)
			}
		}
	}
	return falliti
}
