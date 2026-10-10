//go:build integrazione && privato

// L4 — le prove sulla copia intatta del dump (A1c-L4D-02…16; piano A, 6.7.4; R10, R44, R54 A, P-11, E-18; scelte
// tecniche T-Q10-01…09 ratificate): il banco -dsn -tutti -attesi -gate gira una volta sola per binario di prova
// (T-Q10-09), con la foto del database prima e dopo (RO-DOMINIO), e ogni prova legge il suo requisito nel Rapporto; le
// prove che il rapporto non mostra rifanno il percorso del banco con gli stessi ingressi (il caricatore, Calcola,
// Confronta), dopo aver controllato che l'impronta sia quella del banco.
//
// Senza COCKPIT_DATASET_A o COCKPIT_DUMP_DSN, con una copia diversa da quella del manifest, con PGPASSWORD impostata
// (T-Q10-07) o con il banco che non legge la copia, ogni prova è NON ESEGUITA, mai saltata e mai verde (R44). Una parte
// che il rapporto dichiara non verificata è «NON ESEGUITA: …» con t.Errorf (T-Q10-06): la prova non è mai verde. Dove
// il rapporto non mostra la chiave del caso, la prova lo scrive nel log, perché il revisore A6.4 lo accerti sul rapporto
// (T-Q10-02): l'assenza di errori aggregati non vale come «quel caso è verificato».
//
// Nessun valore reale nel codice (E-18): gli esiti vengono dagli attesi attraverso il runner, i conteggi dal manifest o
// da un conteggio sulla stessa copia (T-Q10-01), e i numeri si scrivono solo nel log; i sottotest nascono a runtime dal
// Rapporto, e i loro nomi stanno solo nei log privati. Gli attesi li legge solo il runner (P-11): qui mai.

package bancoa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit"
	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	valut "promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/dataset"
	"promatec/cockpit/internal/platform/testutil"
)

// q10VariabileDataset: la variabile del manifest privato (piano A, 3.7.3). La legge testutil.DatasetA; il banco vuole il
// percorso, come cmd/bancoa con -dataset.
const q10VariabileDataset = "COCKPIT_DATASET_A"

// q10OrigineScenario: l'origine dei segmenti che il caso dichiara (ancoraggio.OrigineScenario; il banco non importa
// ancoraggio), come la scrive il rapporto in SintesiThread.Segmenti: «<segmento> (scenario)».
const q10OrigineScenario = "scenario"

// q10Corsa: la corsa condivisa del banco sulla copia intatta (T-Q10-09) e il percorso della prova con gli stessi
// ingressi. fermata ed errori sono ciò che la corsa ha detto al primo che l'ha chiesta: le prove dopo li ripetono, così
// nessuna è verde su una corsa che non è andata.
type q10Corsa struct {
	primo   testing.TB
	fermata string
	errori  []string
	finita  bool

	m           dataset.Manifest
	r           RapportoBanco
	prima, dopo map[string]string
	runner      *banco             // gli ingressi del runner, letti senza attesi
	foto        fotorfq.Fotografia // la fotografia del percorso della prova (Tutti, con i messaggi fuori RFQ dei casi)
	esito       valut.Esito        // Calcola su quella fotografia, con gli ingressi del runner
}

var (
	q10Una   sync.Once
	q10Stato *q10Corsa
)

// q10TB: il testing.TB della corsa condivisa. Prende nota di ciò che la corsa dice (la fermata, gli errori) e tiene le
// pulizie per farle alla fine della corsa, non alla fine della prima prova: il ricontrollo delle sentinelle e delle
// impronte di PoolDump (R117 b) dice così il suo esito a tutte le prove.
type q10TB struct {
	testing.TB
	s       *q10Corsa
	pulizie []func()
}

func (c *q10TB) Fatal(a ...any) {
	c.TB.Helper()
	c.s.fermata = fmt.Sprint(a...)
	c.TB.Fatal(a...)
}

func (c *q10TB) Fatalf(f string, a ...any) {
	c.TB.Helper()
	c.s.fermata = fmt.Sprintf(f, a...)
	c.TB.Fatalf(f, a...)
}

func (c *q10TB) FailNow() {
	c.TB.Helper()
	if c.s.fermata == "" {
		c.s.fermata = "la corsa condivisa si è fermata"
	}
	c.TB.FailNow()
}

func (c *q10TB) Error(a ...any) {
	c.TB.Helper()
	c.s.errori = append(c.s.errori, fmt.Sprint(a...))
	c.TB.Error(a...)
}

func (c *q10TB) Errorf(f string, a ...any) {
	c.TB.Helper()
	c.s.errori = append(c.s.errori, fmt.Sprintf(f, a...))
	c.TB.Errorf(f, a...)
}

func (c *q10TB) Cleanup(f func()) { c.pulizie = append(c.pulizie, f) }

// pulisci: le pulizie in ordine inverso, come t.Cleanup.
func (c *q10TB) pulisci() {
	for i := len(c.pulizie) - 1; i >= 0; i-- {
		c.pulizie[i]()
	}
	c.pulizie = nil
}

// q10SenzaPGPASSWORD: sulla copia vera la password sta solo in pgpass.conf; con PGPASSWORD impostata la prova è NON
// ESEGUITA, e la variabile non si tocca (T-Q10-07; R32 c).
func q10SenzaPGPASSWORD(t testing.TB) {
	t.Helper()
	if _, ok := os.LookupEnv("PGPASSWORD"); ok {
		testutil.NonEseguita(t, "PGPASSWORD è impostata: sulla copia vera la password sta solo in pgpass.conf, e la prova non la toglie (T-Q10-07; R32 c)")
	}
}

// q10IngressiDelRunner: gli ingressi del banco senza gli attesi (nuovoBanco e leggiDataset con Attesi falso: gli attesi
// non si aprono, P-11): il manifest, l'indice delle regole, le grammatiche compilate, il file dei casi. Un controllo non
// eseguito, l'insieme delle regole che non c'è o il file dei casi che non si legge: NON ESEGUITA.
func q10IngressiDelRunner(t testing.TB, uscita string) *banco {
	t.Helper()
	b := nuovoBanco(Opzioni{Modalita: ModalitaDSN, Dataset: os.Getenv(q10VariabileDataset), Uscita: uscita}, io.Discard)
	letto := b.leggiDataset()
	for _, c := range b.r.Controlli {
		if c.Stato == ControlloNonEseguito {
			testutil.NonEseguita(t, fmt.Sprintf("gli ingressi del runner: il controllo %s non è eseguito: %s", c.Nome, c.Motivo))
		}
	}
	switch {
	case !letto:
		testutil.NonEseguita(t, "gli ingressi del runner: il manifest non si legge")
	case b.ins == nil:
		testutil.NonEseguita(t, "gli ingressi del runner: l'insieme delle regole non compila (vedi il controllo grammatiche)")
	case !b.casiLetti:
		testutil.NonEseguita(t, "gli ingressi del runner: il file dei casi non si legge (vedi il controllo casi)")
	}
	return b
}

// q10CorsaDSN: la corsa condivisa (T-Q10-09), fatta dalla prima prova che la chiede:
//   - il manifest, la copia intatta (PoolDump: copia, ruolo, schema, sentinelle, impronte) e la foto del database;
//   - EseguiBanco con -dsn -tutti -attesi -gate, il rapporto nella cartella dei rapporti;
//   - il percorso della prova: gli ingressi del runner, Carica con Tutti e i messaggi fuori RFQ dei casi (come il banco),
//     Calcola;
//   - la foto del database dopo tutto.
//
// Le prove dopo la prima ripetono la fermata e gli errori della corsa: mai verdi su una corsa che non è andata.
func q10CorsaDSN(t *testing.T) *q10Corsa {
	t.Helper()
	q10Una.Do(func() {
		s := &q10Corsa{primo: t}
		q10Stato = s
		tb := &q10TB{TB: t, s: s}
		defer tb.pulisci()
		q10Esegui(tb, s)
		s.finita = true
	})
	s := q10Stato
	if s.primo == t {
		return s
	}
	for _, e := range s.errori {
		t.Errorf("%s (dalla corsa condivisa del banco)", e)
	}
	if s.fermata != "" {
		if motivo, ok := strings.CutPrefix(s.fermata, "NON ESEGUITA: "); ok {
			testutil.NonEseguita(t, motivo+" (dalla corsa condivisa del banco)")
		}
		t.Fatalf("la corsa condivisa del banco si è fermata: %s", s.fermata)
	}
	if !s.finita {
		testutil.NonEseguita(t, "la corsa condivisa del banco non è finita (vedi la prima prova che l'ha chiesta)")
	}
	return s
}

// q10Esegui: i passi di q10CorsaDSN.
func q10Esegui(t testing.TB, s *q10Corsa) {
	t.Helper()
	ctx := context.Background()
	q10SenzaPGPASSWORD(t)
	s.m = testutil.DatasetA(t)
	p := testutil.PoolDump(t, s.m.Copia)
	dsn := testutil.DumpDSN(t)
	uscita := filepath.Join(testutil.CartellaRapporti(t), "q10-dsn")
	s.prima = testutil.FotoDelDatabase(t, p)

	var video bytes.Buffer
	o := Opzioni{Modalita: ModalitaDSN, Dataset: os.Getenv(q10VariabileDataset), Uscita: uscita, DSN: dsn, Tutti: true,
		Attesi: true, Gate: true, Migrazioni: cockpit.FS}
	r, err := EseguiBanco(ctx, o, &video)
	var uso *ErroreUso
	switch {
	case errors.As(err, &uso):
		testutil.NonEseguita(t, "il banco rifiuta le opzioni: "+uso.Motivo)
	case err != nil:
		testutil.NonEseguita(t, "il rapporto del banco non si scrive: "+err.Error())
	}
	s.r = r
	for _, nome := range []string{"manifest", ControlloCopia, "sola_lettura", "fotografia"} {
		if c, ok := controlloBanco(r, nome); ok && c.Stato == ControlloNonEseguito {
			testutil.NonEseguita(t, fmt.Sprintf("il banco non ha letto la copia: il controllo %s non è eseguito: %s", nome, c.Motivo))
		}
	}
	if r.Fotografia == nil {
		testutil.NonEseguita(t, "il banco non ha la fotografia: "+r.Motivo)
	}
	t.Logf("corsa condivisa del banco: %s", r.PrimaRiga())

	s.runner = q10IngressiDelRunner(t, uscita)
	f, mancanti, err := caricaSenzaIMancanti(func(messaggi []uuid.UUID) (fotorfq.Fotografia, error) {
		return caricatore.Carica(ctx, p, caricatore.Richiesta{Tutti: true, Messaggi: messaggi})
	}, s.runner.messaggiFuori())
	if err != nil {
		testutil.NonEseguita(t, "la fotografia del percorso della prova non si legge: "+err.Error())
	}
	if len(mancanti) > 0 {
		t.Logf("percorso della prova: %d messaggi dei casi che la copia non ha (come nel banco)", len(mancanti))
	}
	s.foto = f
	if s.esito, err = valut.Calcola(f, s.runner.ins, s.runner.ingressi); err != nil {
		t.Fatalf("percorso della prova: Calcola: %v", err)
	}
	s.dopo = testutil.FotoDelDatabase(t, p)
}

// ---- gli aiuti di lettura del rapporto ----

// q10Voce: un esito contro gli attesi, con il suo file e il suo thread.
type q10Voce struct {
	thread uuid.UUID
	file   FileBanco
	e      EsitoContro
}

// q10EsitiDellaSezione: gli esiti contro gli attesi di tutti i file, per sezione (confronto.Sezione*), nell'ordine del
// rapporto.
func q10EsitiDellaSezione(r RapportoBanco, sezione string) []q10Voce {
	var out []q10Voce
	for _, th := range r.Thread {
		for _, f := range th.File {
			for _, e := range f.Esiti {
				if e.Sezione == sezione {
					out = append(out, q10Voce{thread: th.ThreadID, file: f, e: e})
				}
			}
		}
	}
	return out
}

// q10Dettaglio: il dettaglio di un controllo (le differenze e le parti non verificate, una per riga); vuoto se non c'è.
func q10Dettaglio(r RapportoBanco, nome string) DettaglioControllo {
	for _, d := range r.Dettagli {
		if d.Nome == nome {
			return d
		}
	}
	return DettaglioControllo{}
}

// q10ControlloPulito: il controllo c'è, è eseguito dal runner, senza differenze e senza parti non verificate. Un
// controllo non eseguito, o una parte non verificata, è NON ESEGUITA (T-Q10-06): mai passata.
func q10ControlloPulito(t *testing.T, r RapportoBanco, nome string) {
	t.Helper()
	c, ok := controlloBanco(r, nome)
	if !ok {
		t.Errorf("il rapporto non ha il controllo %s", nome)
		return
	}
	d := q10Dettaglio(r, nome)
	switch c.Stato {
	case ControlloEseguito:
	case ControlloNonEseguito:
		t.Errorf("NON ESEGUITA: il controllo %s non è eseguito: %s", nome, c.Motivo)
	default:
		t.Errorf("il controllo %s è %q: non lo ha eseguito il runner", nome, c.Stato)
	}
	if c.Differenze > 0 {
		t.Errorf("il controllo %s ha %d differenze: %q", nome, c.Differenze, d.Differenze)
	}
	for _, x := range d.NonVerificate {
		t.Errorf("NON ESEGUITA: %s, parte non verificata: %s", nome, x)
	}
}

// q10Gate: il gate del rapporto; senza (gli attesi non letti) la prova è NON ESEGUITA.
func q10Gate(t *testing.T, r RapportoBanco) *Gate {
	t.Helper()
	if r.Gate == nil {
		c, _ := controlloBanco(r, VoceAttesi)
		testutil.NonEseguita(t, "il rapporto non ha il gate: gli attesi non sono stati letti ("+c.Motivo+")")
	}
	return r.Gate
}

// q10VoceGate: una voce del gate per nome.
func q10VoceGate(g *Gate, nome string) (VoceGate, bool) {
	for _, v := range g.Voci {
		if v.Nome == nome {
			return v, true
		}
	}
	return VoceGate{}, false
}

// q10VoceSuperata: la voce del gate è superata. Non eseguita è NON ESEGUITA (T-Q10-06), non superata è un fallimento.
func q10VoceSuperata(t *testing.T, g *Gate, nome string) {
	t.Helper()
	v, ok := q10VoceGate(g, nome)
	switch {
	case !ok:
		t.Errorf("il gate non ha la voce %s", nome)
	case v.Stato == VoceNonEseguita:
		t.Errorf("NON ESEGUITA: la voce %s del gate non è eseguita: %s", nome, v.Motivo)
	case v.Stato != VoceSuperata:
		t.Errorf("la voce %s del gate è %s: %s", nome, v.Stato, v.Motivo)
	}
}

// q10RiguardaPercorso: una riga di un dettaglio parla della voce con quel percorso negli attesi (il percorso, poi la
// fine della riga o un separatore: «x[n]» non riguarda «x[nm]»).
func q10RiguardaPercorso(riga, percorso string) bool {
	if percorso == "" || !strings.HasPrefix(riga, percorso) {
		return false
	}
	return len(riga) == len(percorso) || strings.ContainsRune(":.[ (", rune(riga[len(percorso)]))
}

// q10ThreadDelloScenario: il thread che ha gli esiti della sezione scenario (lo dicono gli attesi attraverso il runner:
// nessun ID nel codice). Nessuno: NON ESEGUITA; più di uno: un fallimento.
func q10ThreadDelloScenario(t *testing.T, r RapportoBanco) ThreadBanco {
	t.Helper()
	var trovati []int
	for i, th := range r.Thread {
		for _, f := range th.File {
			if slices.ContainsFunc(f.Esiti, func(e EsitoContro) bool { return e.Sezione == confronto.SezioneScenario }) {
				trovati = append(trovati, i)
				break
			}
		}
	}
	switch len(trovati) {
	case 0:
		testutil.NonEseguita(t, "nessun file ha un esito della sezione scenario: lo scenario non è stato verificato")
	case 1:
	default:
		t.Fatalf("%d thread hanno esiti della sezione scenario: lo scenario è uno solo", len(trovati))
	}
	return r.Thread[trovati[0]]
}

// q10SenzaCaso: la voce senza_caso di un thread, nil se non c'è.
func q10SenzaCaso(r RapportoBanco, thread uuid.UUID) *ThreadSenzaCaso {
	for i := range r.SenzaCaso {
		if r.SenzaCaso[i].ThreadID == thread {
			return &r.SenzaCaso[i]
		}
	}
	return nil
}

// q10EsitoDellaProva: l'esito del percorso della prova, dopo il controllo che sia quello del banco (stessa impronta):
// altrimenti ciò che vi si cerca non dice niente del banco, e la prova è NON ESEGUITA. A1c-L4D-15 lo asserisce.
func q10EsitoDellaProva(t *testing.T, s *q10Corsa) valut.Esito {
	t.Helper()
	if s.r.Valutazione == nil || s.r.Valutazione.Impronta != s.esito.Impronta {
		testutil.NonEseguita(t, "l'esito del percorso della prova non ha l'impronta di quello del banco (vedi A1c-L4D-15): le diagnostiche dei file non si leggono")
	}
	return s.esito
}

// q10FileInterpretati: i record dei file dell'esito per allegato, dei thread e dei messaggi fuori RFQ.
func q10FileInterpretati(e valut.Esito) map[uuid.UUID]valut.FileInterpretato {
	out := map[uuid.UUID]valut.FileInterpretato{}
	for _, th := range e.Thread {
		for _, f := range th.File {
			out[f.AllegatoID] = f
		}
	}
	for _, m := range e.FuoriRFQ {
		for _, f := range m.File {
			out[f.AllegatoID] = f
		}
	}
	return out
}

// q10HaCodice: il record del file porta una diagnostica con il codice, nel documento (la qualità della fonte) o
// nell'interpretazione: le due sedi che il rapporto non mostra.
func q10HaCodice(f valut.FileInterpretato, codice string) bool {
	for _, d := range f.Documento.Qualita.Diagnostiche {
		if d.Codice == codice {
			return true
		}
	}
	for _, d := range f.Interpretazione.Diagnostiche {
		if d.Codice == codice {
			return true
		}
	}
	return false
}

// q10ContoDelGate: rifà dal rapporto il conto del gate, un esito per file (D7): per ogni allegato il primo esito con
// la voce risolta, nell'ordine scenario, baseline, reali, da_rivedere; contano solo lo scenario e la baseline,
// verificabili, con il peso del gate. Restituisce i file nel gate e quelli bloccanti.
func q10ContoDelGate(r RapportoBanco) (nelGate, bloccanti int) {
	sezioni := []string{confronto.SezioneScenario, confronto.SezioneBaseline, confronto.SezioneReali, confronto.SezioneDaRivedere}
	ordine := func(s string) int { return max(slices.Index(sezioni, s), 0) } // come il gate: una sezione fuori elenco va per prima
	type primo struct {
		sezione int
		e       EsitoContro
	}
	per := map[uuid.UUID]*primo{}
	for _, th := range r.Thread {
		for _, f := range th.File {
			for _, e := range f.Esiti {
				if e.Percorso == "" {
					continue
				}
				if p := per[f.AllegatoID]; p == nil || ordine(e.Sezione) < p.sezione {
					per[f.AllegatoID] = &primo{sezione: ordine(e.Sezione), e: e}
				}
			}
		}
	}
	for _, p := range per {
		if (p.e.Sezione != confronto.SezioneScenario && p.e.Sezione != confronto.SezioneBaseline) || p.e.NonVerificabile != "" {
			continue
		}
		switch confronto.PesoNelGate(p.e.Esito) {
		case confronto.PesoCopertura, confronto.PesoAstensione:
			nelGate++
		case confronto.PesoBloccante:
			nelGate++
			bloccanti++
		}
	}
	return nelGate, bloccanti
}

// q10CaricaRegistrata: Carica sul pool con il registro SQL, poi i controlli del registro (3.5): l'ultimo testo è il
// ROLLBACK, una connessione sola, nessun testo vietato. L'errore di Carica torna a chi chiama.
func q10CaricaRegistrata(t *testing.T, nome string, pr *pgxpool.Pool, reg *testutil.RegistroSQL, rq caricatore.Richiesta) (fotorfq.Fotografia, error) {
	t.Helper()
	reg.Azzera()
	f, err := caricatore.Carica(context.Background(), pr, rq)
	if err != nil {
		return f, err
	}
	voci := reg.Voci()
	if len(voci) == 0 || strings.ToLower(strings.TrimSpace(voci[len(voci)-1].SQL)) != "rollback" {
		t.Errorf("%s: il registro non finisce con il ROLLBACK", nome)
	}
	for _, x := range voci {
		if x.Connessione != voci[0].Connessione {
			t.Errorf("%s: un testo su un'altra connessione: %q", nome, x.SQL)
		}
	}
	if v := reg.Vietati(); len(v) > 0 {
		t.Errorf("%s: testi vietati nel registro: %q", nome, v)
	}
	return f, nil
}

// q10CartellaExport: la cartella degli export, ricavata dal manifest (T-Q10-03): la cartella comune dei file delle voci
// export.*. Senza la sezione export, con le voci che non si leggono o in cartelle diverse: NON ESEGUITA.
func q10CartellaExport(t testing.TB, m dataset.Manifest) string {
	t.Helper()
	if m.Export == nil {
		testutil.NonEseguita(t, "il manifest non dichiara la sezione export (terna e schema)")
	}
	voci, err := VociExport(m)
	if err != nil {
		testutil.NonEseguita(t, "le voci degli export del manifest: "+err.Error())
	}
	sezioni := make([]string, 0, len(voci))
	for s := range voci {
		sezioni = append(sezioni, s)
	}
	sort.Strings(sezioni)
	cartella := ""
	for _, s := range sezioni {
		d := filepath.Dir(m.PercorsoDi(voci[s]))
		switch {
		case cartella == "":
			cartella = d
		case d != cartella:
			testutil.NonEseguita(t, "le voci export.* del manifest non stanno nella stessa cartella: la cartella degli export non si ricava (T-Q10-03)")
		}
	}
	return cartella
}

// q10FotografiaDegliExport: la fotografia degli export del manifest, con sha256 e byte controllati. Un file che manca o
// è cambiato: NON ESEGUITA; un export che non ha la forma attesa: un fallimento.
func q10FotografiaDegliExport(t testing.TB, m dataset.Manifest) (fotorfq.Fotografia, string) {
	t.Helper()
	cartella := q10CartellaExport(t, m)
	f, err := FotografiaDaExport(os.DirFS(cartella), m)
	switch {
	case errors.Is(err, dataset.ErrFileMancante), errors.Is(err, dataset.ErrImprontaDiversa):
		testutil.NonEseguita(t, "gli export del manifest: "+err.Error())
	case err != nil:
		t.Fatalf("gli export non si leggono: %v", err)
	}
	return f, cartella
}

// q10ConteggioSQL: un conteggio sulla stessa copia (T-Q10-01).
// q10ChiaviLibereDelleIntegrazioni: le chiavi libere della sezione casi_integrazione, che il runner riporta e non
// giudica (R-109). Le chiavi dedicate di alcuni casi (lo speculare, la formazione dello STEP) stanno lì: per T-Q10-02 la
// prova non può darle per verificate (revisione A6.4 del 10/10). I percorsi vanno solo nel log.
func q10ChiaviLibereDelleIntegrazioni(r RapportoBanco) []string {
	var out []string
	if r.Attesi != nil {
		for _, k := range r.Attesi.ChiaviLibere {
			if strings.HasPrefix(k, "casi_integrazione") {
				out = append(out, k)
			}
		}
	}
	return out
}

// q10NonEseguitaSeLibere: NON ESEGUITA per le chiavi del caso che stanno fra le libere di casi_integrazione; il nucleo del
// caso coperto da altri controlli lo dice il chiamante nel log.
func q10NonEseguitaSeLibere(t *testing.T, r RapportoBanco, caso string) {
	libere := q10ChiaviLibereDelleIntegrazioni(r)
	for _, k := range libere {
		t.Logf("chiave libera di casi_integrazione, non giudicata dal runner: %s", k)
	}
	if len(libere) > 0 {
		t.Errorf("NON ESEGUITA: %s: %d chiavi di casi_integrazione sono chiavi libere, che il runner riporta e non giudica (R-109): il caso specifico non è verificato (T-Q10-02)", caso, len(libere))
	}
}

func q10ConteggioSQL(t *testing.T, p *pgxpool.Pool, sql string) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatalf("conteggio sulla copia: %v", err)
	}
	return n
}

// ---- le prove ----

// A1c-L4D-02: Carica con Tutti, due volte, dà la stessa impronta; la transazione è REPEATABLE READ READ ONLY, una
// connessione, ROLLBACK in fondo, nessun testo vietato; la terna è quella del manifest e lo schema quello della copia. I
// numeri del piano non stanno nel codice (T-Q10-01): i thread, i clienti e gli allegati della fotografia si confrontano
// con un conteggio sulla stessa copia, e vanno nel log per il revisore A6.4.
func TestL4DumpCaricaTutti(t *testing.T) {
	q10SenzaPGPASSWORD(t)
	m := testutil.DatasetA(t)
	p := testutil.PoolDump(t, m.Copia)
	if m.Export == nil {
		testutil.NonEseguita(t, "il manifest non dichiara la terna (sezione export): l'analizzatore della fotografia non ha con che cosa confrontarsi")
	}
	pr, reg := testutil.PoolConRegistro(t, testutil.DumpDSN(t)) // dopo PoolDump, che ha fatto i controlli

	var foto [2]fotorfq.Fotografia
	var impronte [2]string
	for i := range foto {
		f, err := q10CaricaRegistrata(t, fmt.Sprintf("corsa %d", i+1), pr, reg, caricatore.Richiesta{Tutti: true})
		if err != nil {
			t.Fatalf("Carica con Tutti: %v", err)
		}
		h, err := fotorfq.ImprontaFotografia(f)
		if err != nil {
			t.Fatalf("impronta della fotografia: %v", err)
		}
		foto[i], impronte[i] = f, h
	}
	if impronte[0] != impronte[1] {
		t.Errorf("due Carica con Tutti danno due impronte diverse")
	}
	f := foto[0]
	if !f.Coerente || f.SolaLettura != "on" || f.Isolamento != "repeatable read" {
		t.Errorf("la transazione: coerente %v, sola lettura %q, isolamento %q", f.Coerente, f.SolaLettura, f.Isolamento)
	}
	terna := fotorfq.Terna{Versione: m.Export.Versione, HashConfigurazione: m.Export.HashConfigurazione}
	if f.Analizzatore == nil || *f.Analizzatore != terna {
		t.Errorf("l'analizzatore della fotografia non è la terna del manifest")
	}
	if f.SchemaDB != m.Copia.Schema {
		t.Errorf("lo schema della fotografia (%d) non è quello del manifest (%d)", f.SchemaDB, m.Copia.Schema)
	}

	clienti := map[uuid.UUID]bool{}
	allegati, conFatti := 0, 0
	for _, th := range f.Thread {
		clienti[th.ClienteID] = true
		allegati += len(th.Allegati)
		for _, a := range th.Allegati {
			if a.Sha256 == nil {
				continue
			}
			if _, ok := th.Fatti[*a.Sha256]; ok {
				conFatti++
			}
		}
	}
	nThread := q10ConteggioSQL(t, p, `SELECT count(*) FROM thread_offerta`)
	nClienti := q10ConteggioSQL(t, p, `SELECT count(DISTINCT cliente_id) FROM thread_offerta`)
	nAllegati := q10ConteggioSQL(t, p, `SELECT count(*) FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE m.thread_id IS NOT NULL`)
	if len(f.Thread) != nThread || len(clienti) != nClienti {
		t.Errorf("la fotografia ha %d thread di %d clienti, la copia %d di %d", len(f.Thread), len(clienti), nThread, nClienti)
	}
	if allegati != nAllegati {
		t.Errorf("la fotografia ha %d allegati dei thread, la copia %d", allegati, nAllegati)
	}
	if conFatti > allegati {
		t.Errorf("%d allegati con i fatti correnti su %d", conFatti, allegati)
	}
	t.Logf("A6.4 (T-Q10-01): thread %d, clienti %d, allegati %d, con i fatti correnti %d: da confrontare con il registro (A.10d)",
		len(f.Thread), len(clienti), allegati, conFatti)
}

// A1c-L4D-03 (i prodotti della mail): con il caso, i prodotti della mail dello scenario sono quelli attesi (base, fase,
// quantità con l'evidenza di cella) e nessuno in più; senza il caso nessun prodotto (R48 A). I prodotti attesi vengono
// dagli attesi attraverso il runner; il thread dello scenario lo dicono gli esiti della sezione scenario.
func TestL4DumpProdottiDallaMail(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	th := q10ThreadDelloScenario(t, r)
	if th.Caso == "" {
		t.Errorf("il thread dello scenario non ha il caso del file dei casi")
	}
	if len(th.ProdottiControAtteso) == 0 {
		t.Errorf("il thread dello scenario non ha prodotti contro l'atteso")
	}
	for i, pa := range th.ProdottiControAtteso {
		if pa.Esito != confronto.EsitoCorretto {
			t.Errorf("prodotto %d contro l'atteso: %s %q", i, pa.Esito, pa.Differenze)
		}
	}
	// Nessuno in più: ConfrontaProdotti mette dopo gli attesi i candidati senza voce, con un esito che non è corretto
	// (confronto/atteso.go). Le righe sono tante quanti i prodotti attesi letti dal runner, e tutte corrette (sopra).
	// Un conteggio «prodotti» nel gate c'è solo se gli attesi lo dichiarano: allora vale anche quello.
	attesi := -1
	if r.Attesi != nil {
		for _, v := range r.Attesi.Voci {
			if v.Sezione == "scenario_prodotti" {
				attesi = v.Voci
			}
		}
	}
	switch {
	case attesi < 0:
		t.Errorf("NON ESEGUITA: il rapporto non dice quanti prodotti attesi ha letto il runner")
	case attesi == 0 || len(th.ProdottiControAtteso) != attesi:
		t.Errorf("prodotti contro l'atteso %d, prodotti attesi letti dal runner %d: un candidato in più o uno mancante", len(th.ProdottiControAtteso), attesi)
	}
	for _, c := range g.ConteggiScenario {
		if c.Nome == "prodotti" && (c.Dichiarato != c.Ottenuto || c.InPiu != 0) {
			t.Errorf("prodotti dello scenario: dichiarati %d, ottenuti %d, in più %d", c.Dichiarato, c.Ottenuto, c.InPiu)
		}
	}
	q10ControlloPulito(t, r, ControlloScenario)

	sc := q10SenzaCaso(r, th.ThreadID)
	if sc == nil {
		t.Fatalf("il rapporto non ha la sezione senza_caso per il thread dello scenario")
	}
	if len(sc.SenzaCaso.Prodotti) != 0 || sc.Uguale {
		t.Errorf("senza il caso: %d prodotti, uguale %v (R48 A: nessun prodotto)", len(sc.SenzaCaso.Prodotti), sc.Uguale)
	}
	if !slices.ContainsFunc(sc.ConCaso.Segmenti, func(x string) bool { return strings.HasSuffix(x, " ("+q10OrigineScenario+")") }) {
		t.Errorf("con il caso nessun segmento ha l'origine dello scenario: %q", sc.ConCaso.Segmenti)
	}
	t.Logf("prodotti dello scenario contro l'atteso: %d; nuovi del thread: %d", len(th.ProdottiControAtteso), len(th.ProdottiNuovi))
}

// A1c-L4D-04 (gli ancoraggi dello scenario e il figlio condiviso): ogni file dello scenario ha l'esito corretto o fuori
// richiesta, con l'autorità dello scenario; i conteggi dello scenario (radici, figli, file condivisi, fuori richiesta)
// sono quelli dichiarati dagli attesi; zero false associazioni. I numeri vengono dagli attesi attraverso il runner.
func TestL4DumpAncoraggiDelloScenario(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	voci := q10EsitiDellaSezione(r, confronto.SezioneScenario)
	if len(voci) == 0 {
		testutil.NonEseguita(t, "nessun esito della sezione scenario nel rapporto")
	}
	conti := map[confronto.EsitoAtteso]int{}
	for _, v := range voci {
		conti[v.e.Esito]++
		t.Run(v.e.ID+"@"+v.e.Percorso, func(t *testing.T) {
			if v.e.NonVerificabile != "" {
				t.Errorf("NON ESEGUITA: la voce non si verifica in questa corsa: %s", v.e.NonVerificabile)
				return
			}
			switch v.e.Esito {
			case confronto.EsitoCorretto, confronto.EsitoFuoriRichiesta:
			default:
				t.Errorf("esito %s, motivi %q", v.e.Esito, v.e.Motivi)
			}
		})
	}
	if len(g.ConteggiScenario) == 0 {
		t.Errorf("NON ESEGUITA: il gate non ha i conteggi dello scenario")
	}
	for _, c := range g.ConteggiScenario {
		if c.Dichiarato != c.Ottenuto || c.InPiu != 0 {
			t.Errorf("conteggio %s: dichiarato %d, ottenuto %d, in più %d", c.Nome, c.Dichiarato, c.Ottenuto, c.InPiu)
		}
		t.Logf("conteggio %s: %d", c.Nome, c.Ottenuto)
	}
	for _, x := range g.NonVerificabili {
		t.Errorf("NON ESEGUITA: voce non verificabile: %s", x)
	}
	q10VoceSuperata(t, g, VoceGateConteggiScenario)
	q10VoceSuperata(t, g, VoceGateFalseAssociazioni)
	t.Logf("esiti dello scenario: %v", conti)
}

// A1c-L4D-05 (lo speculare): le letture dei file e lo scenario senza differenze né parti non
// verificate, e nessuna chiave degli attesi non tradotta. Il rapporto non elenca le chiavi verificate (T-Q10-02): la
// prova mostra che niente è fallito o rimasto non verificato, non che il caso dello speculare c'era; lo dice il log.
func TestL4DumpSpeculare(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	q10ControlloPulito(t, r, ControlloLetture)
	q10ControlloPulito(t, r, ControlloScenario)
	if r.Attesi == nil {
		testutil.NonEseguita(t, "il rapporto non ha la sintesi degli attesi")
	}
	for _, k := range r.Attesi.NonTradotte {
		t.Errorf("chiave degli attesi non tradotta: %s", k)
	}
	// Il nucleo (il target del file è il suo codice, nessun candidato dell'altro) lo giudicano le voci dello scenario
	// (A1c-L4D-04); le chiavi dedicate del caso (il cartiglio, la nota che non diventa identità) sono libere.
	q10NonEseguitaSeLibere(t, r, "lo speculare")
	t.Logf("A6.4 (T-Q10-02): il nucleo dello speculare è coperto solo dalle voci dello scenario; le chiavi dedicate del caso non le giudica nessun controllo")
}

// A1c-L4D-06 (lo STEP con le formazioni): almeno un file della baseline porta la diagnostica della
// formazione non confrontabile, e il suo esito non è errato; le righe e le basi della baseline e le letture dei file
// senza differenze. Le letture negative (nessuna lettura sui nodi di saldatura) le verifica il runner con gli attesi
// (T-Q10-04): il prefisso dei nodi non sta nel codice.
func TestL4DumpBaselineConFormazioneSTEP(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	file := q10FileInterpretati(q10EsitoDellaProva(t, s))
	voci := q10EsitiDellaSezione(r, confronto.SezioneBaseline)
	if len(voci) == 0 {
		testutil.NonEseguita(t, "nessun esito della sezione baseline nel rapporto")
	}
	con := 0
	for _, v := range voci {
		fi, ok := file[v.file.AllegatoID]
		if !ok || !q10HaCodice(fi, motorea.CodiceRevisioneFormazioneNonConfrontabile) {
			continue
		}
		con++
		if confronto.PesoNelGate(v.e.Esito) == confronto.PesoBloccante {
			t.Errorf("un file della baseline con le formazioni STEP ha l'esito %s", v.e.Esito)
		}
	}
	if con == 0 {
		t.Errorf("nessun file della baseline porta la diagnostica %s", motorea.CodiceRevisioneFormazioneNonConfrontabile)
	}
	q10ControlloPulito(t, r, ControlloBaselineRighe)
	q10ControlloPulito(t, r, ControlloLetture)
	t.Logf("file della baseline con la formazione non confrontabile: %d; revisioni diagnosticate nella baseline: %d (indicatore, R113 B: non asserito)",
		con, g.RevisioniBaseline.Diagnosticate)
	// Le chiavi dedicate della formazione (grezza, nessuna equivalenza con la revisione) sono libere di casi_integrazione:
	// la diagnostica sopra è l'unica parte giudicata del caso.
	q10NonEseguitaSeLibere(t, r, "la formazione dello STEP")
	t.Logf("A6.4 (T-Q10-02): il rapporto non dice quale file sia quello con le formazioni; le chiavi dedicate del caso non le giudica nessun controllo")
}

// A1c-L4D-07 (i PDF senza testo): nessun esito della baseline errato, nessuna differenza di riga o di
// lettura sulle voci della baseline, «componente_id_proposta_vuoto_preservato» senza differenze. Il gruppo dei PDF
// senza testo non si separa dal resto della baseline senza l'ID del cliente (T-Q10-02): la prova copre tutta la
// baseline, e il log dice quanti file hanno l'interpretazione non disponibile.
func TestL4DumpBaselineSenzaTestoPDF(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	file := q10FileInterpretati(q10EsitoDellaProva(t, s))
	voci := q10EsitiDellaSezione(r, confronto.SezioneBaseline)
	if len(voci) == 0 {
		testutil.NonEseguita(t, "nessun esito della sezione baseline nel rapporto")
	}
	dettagli := []DettaglioControllo{q10Dettaglio(r, ControlloBaselineRighe), q10Dettaglio(r, ControlloLetture)}
	senzaTesto := 0
	for _, v := range voci {
		if confronto.PesoNelGate(v.e.Esito) == confronto.PesoBloccante {
			t.Errorf("una voce della baseline ha l'esito %s", v.e.Esito)
		}
		if fi, ok := file[v.file.AllegatoID]; ok && fi.Interpretazione.Stato == motorea.StatoInterpretazioneNonDisponibile {
			senzaTesto++
		}
		for _, d := range dettagli {
			for _, x := range d.Differenze {
				if q10RiguardaPercorso(x, v.e.Percorso) {
					t.Errorf("%s, differenza su una voce della baseline: %s", d.Nome, x)
				}
			}
			for _, x := range d.NonVerificate {
				if q10RiguardaPercorso(x, v.e.Percorso) {
					t.Errorf("NON ESEGUITA: %s, parte non verificata di una voce della baseline: %s", d.Nome, x)
				}
			}
		}
	}
	for _, d := range r.Dettagli {
		for _, x := range d.Differenze {
			if strings.Contains(x, "componente_id_proposta_vuoto_preservato") {
				t.Errorf("%s: %s", d.Nome, x)
			}
		}
	}
	if senzaTesto == 0 {
		t.Errorf("NON ESEGUITA: nessun file della baseline ha l'interpretazione non disponibile: il gruppo dei PDF senza testo non si trova, e «illeggibile» e «base dal nome» non hanno chiavi giudicate (T-Q10-02)")
	}
	astensioni := map[confronto.EsitoAtteso]int{}
	for _, v := range voci {
		if confronto.PesoNelGate(v.e.Esito) != confronto.PesoBloccante && v.e.Esito != confronto.EsitoCorretto {
			astensioni[v.e.Esito]++
		}
	}
	t.Logf("file della baseline con l'interpretazione non disponibile: %d; esiti della baseline diversi da corretto, non bloccanti: %v", senzaTesto, astensioni)
	t.Logf("A6.4 (T-Q10-02): accertare che i file senza testo del profilo abbiano la base letta dal nome, le righe invariate e " +
		"la lettura illeggibile dove il testo manca: il rapporto non li separa dal resto della baseline")
}

// A1c-L4D-08: le decisioni della baseline sono tutte preservate; la revisione è solo un indicatore. Il numero delle
// decisioni lo controlla il runner dagli attesi (runner_5_attesi_interi), non la prova.
func TestL4DumpDecisioniPreservate(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	if g.DecisioniBaseline == 0 {
		t.Errorf("NON ESEGUITA: nessuna decisione della baseline nel gate")
	}
	if g.DecisioniPreservate != g.DecisioniBaseline {
		t.Errorf("decisioni preservate %d su %d", g.DecisioniPreservate, g.DecisioniBaseline)
	}
	q10VoceSuperata(t, g, VoceGateDecisioniPreservate)
	q10ControlloPulito(t, r, ControlloBaselineRighe)
	t.Logf("decisioni preservate %d su %d; revisioni diagnosticate %d (indicatore)", g.DecisioniPreservate, g.DecisioniBaseline,
		g.RevisioniBaseline.Diagnosticate)
}

// A1c-L4D-09 (C5): i file da rivedere hanno il badge, gli invarianti della C5 non hanno differenze, e stanno fuori dal
// gate: nessuno conta fra le false associazioni o nella copertura per cliente (il conto del gate rifatto dal rapporto).
// Le righe della «Conferma l'albero» stanno nella sezione informativa (D-R116 I), non fra le parti non verificate.
func TestL4DumpDaRivedereFuoriDalGate(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	n := 0
	for _, v := range q10EsitiDellaSezione(r, confronto.SezioneDaRivedere) {
		if v.e.Percorso == "" {
			continue
		}
		n++
		if v.file.Riga.Badge == "" {
			t.Errorf("un file da rivedere senza badge (voce %s)", v.e.Percorso)
		}
	}
	if n == 0 {
		t.Errorf("NON ESEGUITA: nessun esito da rivedere nel rapporto")
	}
	if g.DaRivedere != n {
		t.Errorf("il gate conta %d da rivedere, il rapporto ne ha %d", g.DaRivedere, n)
	}
	nelGate, bloccanti := q10ContoDelGate(r)
	somma := 0
	for _, c := range g.PerCliente {
		somma += c.Copertura + c.Astensioni + c.Bloccanti
	}
	if somma != nelGate || g.FalseAssociazioni != bloccanti {
		t.Errorf("il gate conta %d file e %d false associazioni; dai soli scenario e baseline ne vengono %d e %d",
			somma, g.FalseAssociazioni, nelGate, bloccanti)
	}
	q10ControlloPulito(t, r, ControlloInvariantiC5)
	if a := r.RigheConfermaAlbero; a != nil {
		d := q10Dettaglio(r, ControlloInvariantiC5)
		for _, p := range a.Percorsi {
			for _, x := range d.NonVerificate {
				if q10RiguardaPercorso(x, p) {
					t.Errorf("una riga della «Conferma l'albero» è fra le parti non verificate (D-R116 I): %s", x)
				}
			}
		}
		t.Logf("righe della «Conferma l'albero», solo informazione: %d", a.Righe)
	}
	t.Logf("da rivedere: %d", n)
}

// A1c-L4D-10 (le voci reali degli archivi): esito riservato, mai contato come passato (R21 c); gli archivi che
// l'acquisizione non estrae portano la diagnostica; le letture dei file delle voci reali senza differenze. Una lettura
// parziale non giudicata resta NON ESEGUITA (D-R116 O): mai verde.
func TestL4DumpArchiviReali(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	voci := q10EsitiDellaSezione(r, confronto.SezioneReali)
	if len(voci) == 0 {
		testutil.NonEseguita(t, "nessun esito della sezione reali nel rapporto")
	}
	thread := map[uuid.UUID]bool{}
	for _, v := range voci {
		thread[v.thread] = true
		if v.e.Esito != confronto.EsitoRiservato {
			t.Errorf("una voce reale ha l'esito %s: deve essere riservato (R21 c)", v.e.Esito)
		}
	}
	q10VoceSuperata(t, g, VoceGateRiservati)

	e := q10EsitoDellaProva(t, s)
	archivi := 0
	for _, th := range e.Thread {
		if !thread[th.ThreadID] {
			continue
		}
		for _, f := range th.File {
			if q10HaCodice(f, estrazione.CodiceArchivioNonEstraibile) {
				archivi++
			}
		}
	}
	if archivi == 0 {
		t.Errorf("nessun file dei thread delle voci reali porta la diagnostica %s", estrazione.CodiceArchivioNonEstraibile)
	}
	d := q10Dettaglio(r, ControlloLetture)
	for _, v := range voci {
		for _, x := range d.Differenze {
			if q10RiguardaPercorso(x, v.e.Percorso) {
				t.Errorf("%s, differenza su una voce reale: %s", ControlloLetture, x)
			}
		}
		for _, x := range d.NonVerificate {
			if q10RiguardaPercorso(x, v.e.Percorso) {
				t.Errorf("NON ESEGUITA: lettura parziale o non verificata di una voce reale (D-R116 O): %s", x)
			}
		}
	}
	t.Logf("voci reali %d; file con l'archivio non estraibile %d", len(voci), archivi)
	t.Logf("A6.4 (T-Q10-02): accertare sul rapporto il token conservato e nessuna revisione presa dal nome: il rapporto " +
		"non elenca le chiavi verificate")
}

// A1c-L4D-11: lo stesso sha256 con nomi diversi sui dati veri (la coppia si cerca a runtime) dà una riga per allegato,
// senza fusioni; i file non coperti hanno il badge; i thread dei clienti senza grammatica sono «non valutati», a parte,
// fuori dalle correzioni manuali per cliente.
func TestL4DumpStessoShaNonCopertiSenzaGrammatica(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)

	gruppi := map[string][]FileBanco{}
	for _, th := range r.Thread {
		for _, f := range th.File {
			if f.Sha256 != "" {
				gruppi[f.Sha256] = append(gruppi[f.Sha256], f)
			}
		}
	}
	coppie := 0
	for _, ff := range gruppi {
		nomi, ids := map[string]bool{}, map[uuid.UUID]bool{}
		for _, f := range ff {
			nomi[f.NomeFile] = true
		}
		if len(nomi) < 2 {
			continue
		}
		coppie++
		for _, f := range ff {
			if ids[f.AllegatoID] {
				t.Errorf("lo stesso allegato ha due righe nel rapporto")
			}
			ids[f.AllegatoID] = true
			if f.Riga.File.AllegatoID != f.AllegatoID {
				t.Errorf("la riga di confronto di un allegato con lo sha256 condiviso è di un altro allegato")
			}
		}
	}
	if coppie == 0 {
		t.Errorf("NON ESEGUITA: nessun gruppo di allegati con lo stesso sha256 e nomi diversi (T-Q10-01: il piano dice che c'è)")
	}

	if g.NonCoperti == 0 {
		t.Errorf("il gate non conta file non coperti")
	}
	for _, th := range r.Thread {
		for _, f := range th.File {
			for _, e := range f.Esiti {
				if e.Esito == confronto.EsitoNonCoperto && f.Riga.Badge == "" {
					t.Errorf("un file non coperto senza badge")
				}
			}
		}
	}

	valutati, nonValutati := map[uuid.UUID]int{}, map[uuid.UUID]int{}
	senza := 0
	for _, th := range r.Thread {
		if th.Valutato {
			valutati[th.ClienteID]++
			continue
		}
		nonValutati[th.ClienteID]++
		if th.Motivo != string(valut.MotivoThreadSenzaGrammatica) {
			continue
		}
		senza++
		if !th.AParte {
			t.Errorf("un thread senza grammatica non è a parte")
		}
		if !slices.ContainsFunc(g.NonValutati, func(x string) bool { return strings.HasPrefix(x, th.ThreadID.String()) }) {
			t.Errorf("un thread senza grammatica non è fra i non valutati del gate")
		}
	}
	for _, c := range r.CorrezioniManuali {
		if c.Thread != valutati[c.ClienteID] || c.ThreadNonValutati != nonValutati[c.ClienteID] {
			t.Errorf("correzioni manuali: %d thread e %d non valutati, il rapporto ne ha %d e %d", c.Thread, c.ThreadNonValutati,
				valutati[c.ClienteID], nonValutati[c.ClienteID])
		}
	}
	if senza == 0 {
		t.Errorf("NON ESEGUITA: nessun thread non valutato per la grammatica che manca")
	}
	t.Logf("gruppi con lo stesso sha256 e nomi diversi %d; non coperti %d; thread senza grammatica %d", coppie, g.NonCoperti, senza)
}

// A1c-L4D-12 (RO-DOMINIO): la foto del database prima e dopo tutta la catena è la stessa (job
// compresa); le tabelle che il ruolo non legge sono le escluse del manifest; il collegamento non può scrivere e la
// transazione del caricatore è in sola lettura; e il ruolo, con una transazione READ WRITE esplicita, riceve 42501 su
// una UPDATE che non tocca niente (25006 direbbe solo la transazione, non il ruolo). Chiusa con ROLLBACK. La voce del
// gate «zero scritture» resta non eseguita per costruzione: l'esito di questa prova va nel registro.
func TestL4DumpNessunaScrittura(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	if d := testutil.Differenze(s.prima, s.dopo); len(d) > 0 {
		t.Errorf("la catena ha cambiato il database: %v", d)
	}
	if v, ok := s.prima["job"]; !ok || v != s.dopo["job"] {
		t.Errorf("la tabella job manca nella foto o è cambiata")
	}
	var nonLeggibili []string
	for tab, v := range s.prima {
		if v == testutil.ValoreNonLeggibile {
			nonLeggibili = append(nonLeggibili, tab)
		}
	}
	escluse := append([]string(nil), s.m.Copia.Escluse...)
	sort.Strings(nonLeggibili)
	sort.Strings(escluse)
	if !slices.Equal(nonLeggibili, escluse) {
		t.Errorf("le tabelle che il ruolo non legge %v non sono le escluse del manifest %v", nonLeggibili, escluse)
	}
	sl := r.SolaLettura
	switch {
	case sl == nil:
		t.Errorf("il rapporto non ha i controlli di sola lettura")
	default:
		if sl.ScritturaPossibile {
			t.Errorf("il collegamento del banco può scrivere")
		}
		switch {
		case sl.EscluseNonLeggibili == nil:
			t.Errorf("le tabelle escluse non sono state controllate")
		case *sl.EscluseNonLeggibili != len(s.m.Copia.Escluse):
			t.Errorf("tabelle escluse non leggibili %d, il manifest ne dichiara %d", *sl.EscluseNonLeggibili, len(s.m.Copia.Escluse))
		}
		if sl.TransazioneSolaLettura != "on" || sl.Isolamento != "repeatable read" {
			t.Errorf("la transazione del caricatore: %q, %q", sl.TransazioneSolaLettura, sl.Isolamento)
		}
	}

	p := testutil.PoolDump(t, s.m.Copia)
	ctx := context.Background()
	tx, err := p.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if err != nil {
		t.Fatalf("BEGIN READ WRITE: %v", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE documento_proposta SET stato = stato WHERE false`)
	var pe *pgconn.PgError
	switch {
	case err == nil:
		t.Errorf("il ruolo del banco può scrivere: la UPDATE è passata")
	case !errors.As(err, &pe):
		t.Errorf("la UPDATE non dà un errore del database: %v", err)
	case pe.Code != "42501":
		t.Errorf("la UPDATE dà %s, non 42501: non dimostra il ruolo", pe.Code)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Errorf("ROLLBACK: %v", err)
	}
	t.Logf("registro (gate «zero scritture»): foto invariata, escluse %d, ruolo senza privilegi di scrittura", len(escluse))
}

// q10FileComune: un allegato con la chiave della parità (il nome del file e il digest dei fatti, R32 a), con le sue
// impronte dell'interpretazione e la riga di confronto del nuovo.
type q10FileComune struct {
	impronte []string
	righe    []confronto.File
}

// q10PerChiave: gli allegati della fotografia per «nome del file + digest dei fatti», con l'interpretazione dei thread
// valutati. Un allegato senza sha256 o senza fatti non ha chiave; uno senza interpretazione non ha impronte.
func q10PerChiave(f fotorfq.Fotografia, e valut.Esito) (map[string]*q10FileComune, int) {
	chiave := map[uuid.UUID]string{}
	for _, th := range f.Thread {
		for _, a := range th.Allegati {
			if a.Sha256 == nil {
				continue
			}
			if fa, ok := th.Fatti[*a.Sha256]; ok && fa.Digest != "" {
				chiave[a.ID] = a.NomeFile + "\x00" + fa.Digest
			}
		}
	}
	out := map[string]*q10FileComune{}
	nonValutati := 0
	for _, et := range e.Thread {
		if !et.Valutato {
			nonValutati++
			continue
		}
		righe := map[uuid.UUID]confronto.File{}
		for _, x := range inFiles(et.Confrontabili) {
			righe[x.AllegatoID] = x
		}
		for _, fi := range et.File {
			k, ok := chiave[fi.AllegatoID]
			if !ok || fi.Interpretazione.Impronta == "" {
				continue
			}
			c := out[k]
			if c == nil {
				c = &q10FileComune{}
				out[k] = c
			}
			if !slices.Contains(c.impronte, fi.Interpretazione.Impronta) {
				c.impronte = append(c.impronte, fi.Interpretazione.Impronta)
			}
			if x, ok := righe[fi.AllegatoID]; ok {
				c.righe = append(c.righe, x)
			}
		}
	}
	for _, c := range out {
		sort.Strings(c.impronte)
	}
	return out, nonValutati
}

// q10Radici: le radici dei candidati di una riga, in ordine e senza doppioni.
func q10Radici(f confronto.File) []string {
	var out []string
	for _, c := range f.Nuovo.Candidati {
		for _, x := range c.Radici {
			if !slices.Contains(out, x) {
				out = append(out, x)
			}
		}
	}
	sort.Strings(out)
	return out
}

// A1c-L4D-13: la parità fra -exports e -dsn sui thread esportati (R32 a): per lo stesso nome del file e lo stesso digest
// dei fatti, interpretato da tutte e due le parti, l'impronta dell'interpretazione è la stessa; gli ancoraggi si
// confrontano per collocazione, basi e radici dove le due righe sono valutate, e le differenze si elencano nel log per
// categoria (il contesto degli export è diverso: C5, componenti non esportati, «fuori richiesta» non dimostrabile, il
// cliente degli export, D-R115 A). Il banco -exports gira per controllo: con D-R115 A non si chiede l'uscita 0. Zero
// file confrontabili: NON ESEGUITA, mai verde.
func TestL4DumpPareggioExportDSN(t *testing.T) {
	s := q10CorsaDSN(t)
	ctx := context.Background()
	fe, cartella := q10FotografiaDegliExport(t, s.m)

	var video bytes.Buffer
	o := Opzioni{Modalita: ModalitaExports, Dataset: os.Getenv(q10VariabileDataset), Exports: cartella,
		Uscita: filepath.Join(testutil.CartellaRapporti(t), "q10-exports"), Attesi: true}
	re, err := EseguiBanco(ctx, o, &video)
	var uso *ErroreUso
	switch {
	case errors.As(err, &uso):
		testutil.NonEseguita(t, "il banco -exports rifiuta le opzioni: "+uso.Motivo)
	case err != nil:
		testutil.NonEseguita(t, "il rapporto del banco -exports non si scrive: "+err.Error())
	}
	if c, ok := controlloBanco(re, "export"); !ok || c.Stato == ControlloNonEseguito {
		testutil.NonEseguita(t, "il banco -exports non ha letto gli export: "+c.Motivo)
	}
	cl, _ := controlloBanco(re, ControlloClientiExport)
	t.Logf("banco -exports: %s; uscita %d; %s: %s (%s), fuori perimetro %d (D-R115 A: non si chiede 0)", re.PrimaRiga(),
		re.Esito.CodiceUscita(), ControlloClientiExport, cl.Stato, cl.Esito, len(q10Dettaglio(re, ControlloClientiExport).FuoriPerimetro))

	var ids []uuid.UUID
	for _, th := range fe.Thread {
		ids = append(ids, th.ID)
	}
	if len(ids) == 0 {
		testutil.NonEseguita(t, "gli export non hanno thread")
	}
	p := testutil.PoolDump(t, s.m.Copia)
	fd, err := caricatore.Carica(ctx, p, caricatore.Richiesta{Thread: ids})
	if err != nil {
		testutil.NonEseguita(t, "i thread degli export non si fotografano nella copia: "+err.Error())
	}
	ee, err := valut.Calcola(fe, s.runner.ins, s.runner.ingressi)
	if err != nil {
		t.Fatalf("Calcola sugli export: %v", err)
	}
	ed, err := valut.Calcola(fd, s.runner.ins, s.runner.ingressi)
	if err != nil {
		t.Fatalf("Calcola sulla copia: %v", err)
	}
	daExport, nonValutatiExport := q10PerChiave(fe, ee)
	daCopia, _ := q10PerChiave(fd, ed)
	daRivedere := map[uuid.UUID]bool{}
	for _, v := range q10EsitiDellaSezione(s.r, confronto.SezioneDaRivedere) {
		daRivedere[v.file.AllegatoID] = true
	}

	chiavi := make([]string, 0, len(daCopia))
	for k := range daCopia {
		chiavi = append(chiavi, k)
	}
	sort.Strings(chiavi)
	confrontabili, ancoraggiUguali := 0, 0
	categorie := map[string]int{}
	for _, k := range chiavi {
		d, x := daCopia[k], daExport[k]
		if x == nil {
			continue
		}
		confrontabili++
		if !slices.Equal(d.impronte, x.impronte) {
			t.Errorf("un file con lo stesso nome e lo stesso digest dei fatti ha interpretazioni diverse con -exports e con -dsn")
		}
		if len(d.righe) != 1 || len(x.righe) != 1 {
			categorie["più allegati con la stessa chiave: ancoraggi non confrontati"]++
			continue
		}
		a, b := d.righe[0], x.righe[0]
		if !a.Nuovo.Valutato || !b.Nuovo.Valutato {
			categorie["riga non valutata da una delle due parti"]++
			continue
		}
		if a.Nuovo.Collocazione == b.Nuovo.Collocazione && slices.Equal(a.Nuovo.Basi, b.Nuovo.Basi) && slices.Equal(q10Radici(a), q10Radici(b)) {
			ancoraggiUguali++
			continue
		}
		switch {
		case daRivedere[a.AllegatoID]:
			categorie["ancoraggio diverso su un file da rivedere (C5)"]++
		default:
			categorie["ancoraggio diverso: contesto degli export (componenti non esportati, «fuori richiesta» non dimostrabile, R32 b)"]++
		}
	}
	if confrontabili == 0 {
		testutil.NonEseguita(t, "nessun file con lo stesso nome e lo stesso digest dei fatti interpretato da tutte e due le parti")
	}
	if nonValutatiExport > 0 {
		categorie["thread non valutato negli export (cliente degli export non coerente, D-R115 A)"] += nonValutatiExport
	}
	t.Logf("parità: thread esportati %d, file confrontabili %d, ancoraggi uguali %d", len(ids), confrontabili, ancoraggiUguali)
	nomi := make([]string, 0, len(categorie))
	for c := range categorie {
		nomi = append(nomi, c)
	}
	sort.Strings(nomi)
	for _, c := range nomi {
		t.Logf("differenza, %s: %d", c, categorie[c])
	}
	t.Logf("A6.4: le differenze degli ancoraggi sono elencate e non giudicate dalla prova: accertare che ognuna venga dal contesto degli export")
}

// A1c-L4D-14 (le forme dei due profili senza etichette): finché le etichette mancano, i messaggi fuori RFQ dei casi
// senza thread hanno il censimento delle letture forma per forma, la voce del gate delle forme dei due profili resta non
// eseguita con il motivo delle etichette (D-M22; R34 c = B: fuori dal perimetro obbligatorio di A1c, mai superata), il gate è incompleto
// e l'uscita con -gate è NON ESEGUITO. La prova è verde quando lo stato è coerente con D-M22: non chiude la voce. Le
// forme sono di clienti reali e stanno solo nel log.
func TestL4DumpCensimentoAltriProfili(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	g := q10Gate(t, r)
	type chiave struct {
		caso      string
		messaggio uuid.UUID
	}
	voci := map[chiave]VoceCensimento{}
	for _, v := range r.Censimento {
		voci[chiave{v.Caso, v.MessaggioID}] = v
	}
	casi := 0
	astensioni, conLetture, nonValutati := map[string]int{}, map[string]int{}, map[string]int{}
	discordi, senzaCliente := map[string]int{}, map[string]int{}
	clientiDellaFoto := map[uuid.UUID]bool{}
	for _, cl := range s.foto.Clienti {
		clientiDellaFoto[cl.ID] = true
	}
	for _, c := range s.runner.ingressi.Casi {
		if c.ThreadID != nil {
			continue
		}
		casi++
		if len(c.Messaggi) == 0 {
			t.Errorf("il caso %s non ha messaggi", c.ID)
		}
		for _, m := range c.Messaggi {
			v, ok := voci[chiave{c.ID, m}]
			// D-PERIMETRO-A1C: il riconoscimento mancante per una grammatica non prevista è un'astensione dichiarata,
			// non un errore. Il messaggio deve stare nel censimento; non valutato, con il motivo; valutato senza
			// letture, un'astensione anche questa. Le astensioni si contano nel log: non sono copertura superata.
			switch {
			case !ok:
				t.Errorf("il caso %s: un messaggio non è nel censimento", c.ID)
				continue
			case !v.Valutato && v.Motivo == "":
				t.Errorf("il caso %s: un messaggio del censimento non è valutato e non dice perché", c.ID)
			case !v.Valutato && v.Motivo == string(valut.MotivoThreadSenzaGrammatica):
				astensioni[c.ID+": non valutato, "+v.Motivo]++
			case !v.Valutato && v.Motivo == string(valut.MotivoThreadRagioneSocialeDiscorde):
				// Decisione dell'utente del 10/10: un'astensione dichiarata dentro un censimento non certificato, in una
				// categoria propria. Non è «grammatica mancante» e non è prova della qualità del riconoscimento: può
				// essere una discrepanza fra l'identità del cliente nella fotografia e quella delle regole. Accanto, nel
				// log, i messaggi il cui cliente la fotografia non porta (la causa trovata da A6.4: D-Q10-4).
				discordi[c.ID]++
				if !clientiDellaFoto[v.ClienteID] {
					senzaCliente[c.ID]++
				}
			case !v.Valutato:
				// Un altro motivo non è «una grammatica non ancora prevista» (revisione A6.4 del 10/10): per esempio il
				// cliente di un caso senza thread che la fotografia non porta, e la ragione sociale che risulta discorde.
				nonValutati[c.ID+": "+v.Motivo]++
			case len(v.Letture) == 0:
				astensioni[c.ID+": valutato, nessuna lettura"]++
			default:
				conLetture[c.ID]++
			}
			for _, l := range v.Letture {
				t.Logf("censimento, caso %s: %s / %s, %s: %d", c.ID, l.Famiglia, l.Forma, l.Funzione, l.N)
			}
		}
	}
	if casi == 0 {
		testutil.NonEseguita(t, "il file dei casi non ha casi senza thread: il censimento non ha messaggi")
	}
	for _, k := range slices.Sorted(maps.Keys(conLetture)) {
		t.Logf("censimento, caso %s: messaggi con letture %d", k, conLetture[k])
	}
	for _, k := range slices.Sorted(maps.Keys(astensioni)) {
		t.Logf("censimento, astensione dichiarata (D-PERIMETRO-A1C), caso %s: %d", k, astensioni[k])
	}
	for _, k := range slices.Sorted(maps.Keys(discordi)) {
		t.Logf("censimento, caso %s: %d messaggi non valutati per ragione sociale discorde: astensione dichiarata di un censimento non certificato, NON prova del riconoscimento; di questi, %d con il cliente assente dalla fotografia (D-Q10-4)",
			k, discordi[k], senzaCliente[k])
	}
	for _, k := range slices.Sorted(maps.Keys(nonValutati)) {
		t.Errorf("NON ESEGUITA: censimento, caso %s: %d messaggi non valutati per un motivo che non è la grammatica mancante: non è un'astensione dichiarata", k, nonValutati[k])
	}

	v, ok := q10VoceGate(g, VoceGateAltriProfili)
	switch {
	case !ok:
		t.Errorf("il gate non ha la voce %s", VoceGateAltriProfili)
	case v.Stato != VoceNonEseguita:
		t.Errorf("la voce %s è %s: resta non eseguita finché mancano le etichette (D-M22; R34 c = B), mai superata", v.Nome, v.Stato)
	case !strings.Contains(v.Motivo, "etichette"):
		t.Errorf("la voce %s non dice che mancano le etichette: %s", v.Nome, v.Motivo)
	}
	atteso := GateIncompleto
	if slices.ContainsFunc(g.Voci, func(x VoceGate) bool { return x.Stato == VoceNonSuperata }) {
		atteso = GateNonSuperato
	}
	if g.Esito != atteso {
		t.Errorf("il gate è %s, con le voci di adesso deve essere %s", g.Esito, atteso)
	}
	uscita := UscitaNonEseguito
	if r.Differenze > 0 {
		uscita = UscitaConDifferenze
	}
	if r.Esito.CodiceUscita() != uscita {
		t.Errorf("con -gate l'uscita è %d, deve essere %d", r.Esito.CodiceUscita(), uscita)
	}
	if r.Chiusura != nil {
		t.Logf("chiusura (solo informazione): %s; rapporto completo %v", r.Chiusura.Conclusione, r.Chiusura.RapportoCompleto)
	}
	t.Logf("casi del censimento %d; la voce %s resta non eseguita e fuori dal perimetro obbligatorio di A1c (D-M22, R34 c = B): questa prova non la chiude",
		casi, VoceGateAltriProfili)
}

// A1c-L4D-15: la voce casi del manifest e il file dei casi dell'indice sono lo stesso file (runner_4 e lo sha256);
// l'anteprima, emulata (T-Q10-05: il suo percorso vero è di A1d), prende gli ingressi dal file che l'indice indica e dà
// la stessa impronta di valutazione e, thread per thread, di confronto, con gli stessi segmenti e la stessa origine; il
// banco, con l'atteso, ha la stessa impronta di confronto (l'impronta non contiene le parti contro l'atteso). Senza il
// caso, per lo scenario, nessun prodotto (come A1c-L4D-03).
func TestL4DumpBancoEAnteprimaStessiIngressi(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	b := s.runner
	q10ControlloPulito(t, r, ControlloCasiIndice)
	if b.indice == nil || b.indice.Casi == nil {
		t.Fatalf("l'indice delle regole non dichiara il file dei casi: l'anteprima non ha ingressi")
	}
	if !strings.EqualFold(b.indice.Casi.Sha256, r.Sha256Casi) {
		t.Errorf("il file dei casi del manifest non ha lo sha256 di quello dell'indice")
	}
	raw, err := os.ReadFile(filepath.Join(b.cartIdx, filepath.FromSlash(b.indice.Casi.File)))
	if err != nil {
		testutil.NonEseguita(t, "il file dei casi indicato dall'indice non si legge: "+err.Error())
	}
	h := sha256.Sum256(raw)
	if !strings.EqualFold(hex.EncodeToString(h[:]), b.indice.Casi.Sha256) {
		t.Fatalf("il file dei casi indicato dall'indice non ha lo sha256 dell'indice")
	}
	ingressi, err := valut.LeggiIngressi(raw)
	if err != nil {
		t.Fatalf("il file dei casi indicato dall'indice non si legge come ingressi: %v", err)
	}

	if r.Valutazione == nil || r.Fotografia == nil {
		testutil.NonEseguita(t, "il rapporto non ha la valutazione o la fotografia")
	}
	if fh, err := fotorfq.ImprontaFotografia(s.foto); err != nil || fh != r.Fotografia.Impronta {
		t.Errorf("la fotografia del percorso della prova non ha l'impronta di quella del banco")
	}
	if s.esito.Impronta != r.Valutazione.Impronta {
		t.Errorf("il percorso della prova non dà l'impronta di valutazione del banco")
	}
	anteprima, err := valut.Calcola(s.foto, b.ins, ingressi)
	if err != nil {
		t.Fatalf("Calcola con gli ingressi dell'indice: %v", err)
	}
	if anteprima.Impronta != s.esito.Impronta {
		t.Errorf("l'anteprima emulata e il banco danno due impronte di valutazione")
	}
	perThread := map[uuid.UUID]valut.EsitoThread{}
	for _, et := range anteprima.Thread {
		perThread[et.ThreadID] = et
	}
	delBanco := map[uuid.UUID]string{}
	for _, th := range r.Thread {
		delBanco[th.ThreadID] = th.ImprontaConfronto
	}
	for _, et := range s.esito.Thread {
		at, ok := perThread[et.ThreadID]
		if !ok {
			t.Errorf("un thread del banco manca nell'anteprima emulata")
			continue
		}
		ib := confronto.Confronta(inFiles(et.Confrontabili), inProdotti(et.ProdottiConfrontabili), nil).Impronta
		ia := confronto.Confronta(inFiles(at.Confrontabili), inProdotti(at.ProdottiConfrontabili), nil).Impronta
		if ib != ia {
			t.Errorf("un thread ha due impronte di confronto, dal banco e dall'anteprima emulata")
		}
		if ib != delBanco[et.ThreadID] {
			t.Errorf("l'impronta di confronto del rapporto (con l'atteso) non è quella senza l'atteso")
		}
		if !reflect.DeepEqual(sintesiThread(et).Segmenti, sintesiThread(at).Segmenti) {
			t.Errorf("un thread ha segmenti o origini diversi, dal banco e dall'anteprima emulata")
		}
	}
	t.Logf("T-Q10-05: anteprima emulata (ingressi dall'indice contro ingressi dal manifest), %d thread; la prova sul percorso vero si ripete in A1d",
		len(s.esito.Thread))

	th := q10ThreadDelloScenario(t, r)
	if sc := q10SenzaCaso(r, th.ThreadID); sc == nil || len(sc.SenzaCaso.Prodotti) != 0 {
		t.Errorf("senza il caso il thread dello scenario ha prodotti, o non ha la sezione senza_caso")
	}
}

// A1c-L4D-16: il profilo dei limiti sui dati veri, solo misura (R43 B; C-23): la versione dei limiti è quella del
// rapporto, i tetti sono quelli del codice, i limiti quelli dell'indice; ogni voce oltre l'indice corrisponde a un
// massimo osservato sopra il limite, e viceversa; un «limite superato» nelle diagnostiche è dichiarato (oltre l'indice,
// o un'interpretazione parziale). La prova non fallisce perché i valori superano l'indice: fallisce se sono incoerenti
// o non dichiarati. I valori stanno solo nel log.
func TestL4DumpProfiloDeiLimiti(t *testing.T) {
	s := q10CorsaDSN(t)
	r := s.r
	pl := r.ProfiloLimiti
	if pl == nil {
		testutil.NonEseguita(t, "il rapporto non ha il profilo dei limiti")
	}
	if pl.VersioneLimiti != r.VersioneLimiti {
		t.Errorf("la versione dei limiti del profilo non è quella del rapporto")
	}
	if pl.Documenti == 0 {
		t.Errorf("il profilo dei limiti non ha documenti")
	}
	if pl.Tetti != grammatica.TettiLimiti().Riconoscimento {
		t.Errorf("i tetti del profilo non sono quelli del codice")
	}
	if pl.Indice != s.runner.ins.Indice.Limiti.Riconoscimento {
		t.Errorf("i limiti del profilo non sono quelli dell'indice")
	}
	o, l := pl.Osservati, pl.Indice
	var oltre []string
	for _, x := range []struct {
		nome     string
		oss, lim int
	}{
		{"max_byte_unita", o.MaxByteUnita, l.MaxByteUnita},
		{"max_unita_documento", o.MaxUnitaDocumento, l.MaxUnitaDocumento},
		{"max_letture_per_unita", o.MaxLetturePerUnita, l.MaxLetturePerUnita},
		{"max_letture_documento", o.MaxLettureDocumento, l.MaxLettureDocumento},
	} {
		t.Logf("limite %s: osservato %d, indice %d", x.nome, x.oss, x.lim)
		if x.oss > x.lim {
			oltre = append(oltre, x.nome)
		}
	}
	got := append([]string(nil), pl.OltreIndice...)
	sort.Strings(got)
	sort.Strings(oltre)
	if !slices.Equal(got, oltre) {
		t.Errorf("oltre l'indice: il profilo dice %v, i massimi osservati %v", got, oltre)
	}

	e := q10EsitoDellaProva(t, s)
	superati, nonDichiarati := 0, 0
	for _, f := range q10FileInterpretati(e) {
		if !q10HaCodice(f, grammatica.CodiceLimiteSuperato) {
			continue
		}
		superati++
		if len(pl.OltreIndice) == 0 && f.Interpretazione.Stato != motorea.StatoInterpretazioneParziale {
			nonDichiarati++
		}
	}
	if nonDichiarati > 0 {
		t.Errorf("%d file con un limite superato, nessuna voce oltre l'indice e l'interpretazione non parziale", nonDichiarati)
	}
	t.Logf("documenti %d; file con un limite superato %d; oltre l'indice %v", pl.Documenti, superati, pl.OltreIndice)
}
