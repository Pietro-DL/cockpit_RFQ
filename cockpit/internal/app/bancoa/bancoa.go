// Package bancoa è il runner del banco del motore A. È l'unico importatore della libreria YAML: gli attesi entrano
// solo qui, tradotti nei DTO del runner; il motore non li legge mai (v3 §10.5). In A1a ha due modalità
// senza DB:
//   - «regole»: compila le grammatiche del manifest e ne verifica gli esempi («profili attivi validi»);
//   - «casi»: esegue i casi_contratto degli attesi sul riconoscimento per forma (R24 a); da A1b.11 con
//     estrazione.DaTesto più motorea.Interpreta e l'uso sconosciuto (5.4.5, R52 A).
//
// Da A1c ha anche le modalità «dsn» ed «exports» (piano 6.4.9; T-B6-03 e F0-01): la fotografia del DB in sola
// lettura, attraverso il caricatore e platform/migrazioni, o quella degli export; poi lo stesso percorso puro del
// prodotto (valutazione.Calcola, poi confronto.Confronta con la copia di passaggio.go) e, con gli attesi, i controlli
// del runner, gli esiti e il gate. Hanno un punto d'ingresso loro, EseguiBanco, e un rapporto loro (versione 3):
// Esegui, il rapporto di A1a e le due modalità senza DB non cambiano.
//
// Legge tutto da un percorso esterno al repository, attraverso il manifest del dataset privato
// (platform/dataset): attesi, indice delle regole e grammatiche. Nel codice nessun nome di cliente, nessun
// profilo, nessun percorso reale: i profili degli attesi si legano ai clienti nel manifest (D-09), e la
// tabella delle chiavi conosce solo i nomi neutri (R47 a). Usa lo stesso motore del prodotto
// (motorea.CompilaInsieme, Riconosci, Interpreta): non c'è un secondo motore.
package bancoa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/dataset"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// VersioneRapporto: la versione della forma del rapporto. Cambiarla vuol dire riscrivere la prova che la fissa.
// 2 da A1b.11: nei casi le letture hanno la funzione del router e l'esito porta gli attributi di Interpreta.
const VersioneRapporto = 2

// Le modalità di A1a (R24 a), senza DB, e quelle di A1c (6.4.9), che hanno il loro punto d'ingresso (EseguiBanco).
const (
	ModalitaRegole  = "regole"
	ModalitaCasi    = "casi"
	ModalitaDSN     = "dsn"
	ModalitaExports = "exports"
)

// I nomi logici delle voci del manifest che il banco legge: in A1a gli attesi e l'indice; da A1c anche il file dei
// casi (6.6.2), che deve essere quello che l'indice dichiara per l'anteprima (controllo del runner n.4).
const (
	VoceAttesi = "attesi"
	VoceIndice = "regole.indice"
	VoceCasi   = "casi"
)

// Opzioni: che cosa eseguire. Dataset e Uscita sono obbligatori, e stanno fuori dal modulo.
//
// I campi dopo i tre di A1a li leggono solo le modalità dsn ed exports (6.4.9; T-B6-03, F0-01, F0-16): Esegui li
// rifiuta, e nessun campo di A1a cambia.
//   - DSN: la copia del dump, senza password (pgx la legge da pgpass.conf: R32 c); Exports: la cartella degli export;
//     si escludono a vicenda.
//   - Thread: le RFQ da fotografare; Tutti: tutte, lette nella stessa transazione (solo con dsn).
//   - Attesi: gli esiti contro gli attesi e il gate; Gate: il gate decide l'uscita (0 superato, 1 non superato, 3
//     incompleto: R44).
//   - Migrazioni: le migrazioni incorporate del binario (cockpit.FS), per il controllo dello schema di
//     migrazioni.ApriInLettura. Le dà il comando, perché questo pacchetto non importa la radice del modulo (F0-16).
type Opzioni struct {
	Modalita string // regole | casi (A1a); dsn | exports (A1c)
	Dataset  string // il manifest: lo stesso file di COCKPIT_DATASET_A, che legge anche il controllo prima del push
	Uscita   string // la cartella dei rapporti

	DSN        string
	Exports    string
	Thread     []uuid.UUID
	Tutti      bool
	Attesi     bool
	Gate       bool
	Migrazioni fs.FS
}

// Esito: come è andata un'esecuzione, scritto nella prima riga del rapporto (R44).
//   - eseguito_conforme: tutti i controlli eseguiti, nessun esito fuori dall'atteso (uscita 0);
//   - eseguito_con_differenze: almeno un caso fallito, un errore di grammatica o un controllo del runner
//     fallito (uscita 1);
//   - non_eseguito: il manifest manca o non si legge, oppure una sua voce manca o ha un'impronta diversa
//     (uscita 3). Mai «conforme» se un ingresso non è stato letto: senza dataset non esiste un rapporto verde.
//
// Un errore d'uso (flag sbagliati, uscita dentro il modulo) non produce un rapporto: uscita 2, causa su stderr.
type Esito string

const (
	EsitoConforme      Esito = "eseguito_conforme"
	EsitoConDifferenze Esito = "eseguito_con_differenze"
	EsitoNonEseguito   Esito = "non_eseguito"
)

// I codici d'uscita, gli stessi del riepilogo delle prove e del controllo prima del push (R44).
const (
	UscitaConforme      = 0
	UscitaConDifferenze = 1
	UscitaUso           = 2
	UscitaNonEseguito   = 3
)

// CodiceUscita: 0 conforme, 1 con differenze, 3 non eseguito. Un esito sconosciuto non è mai 0.
func (e Esito) CodiceUscita() int {
	switch e {
	case EsitoConforme:
		return UscitaConforme
	case EsitoConDifferenze:
		return UscitaConDifferenze
	}
	return UscitaNonEseguito
}

// ErroreUso: un errore d'uso o di configurazione. Il rapporto non si scrive, e chi lancia il banco esce con 2.
type ErroreUso struct{ Motivo string }

func (e *ErroreUso) Error() string { return "bancoa: " + e.Motivo }

// Esegui legge il manifest, controlla gli sha256, fa girare la modalità e scrive il rapporto. La prima riga è
// «ESITO: …»; le successive dicono modalità, manifest, sha256 di attesi e indice, versione_limiti dell'indice
// (R43 B). Un file del manifest mancante o cambiato non è un errore d'uso: il rapporto si scrive con esito
// non_eseguito e il motivo, e l'elenco dei controlli dice quali non sono stati eseguiti. Una differenza
// prevale su un non eseguito: si vede sempre. Il riepilogo di testo va anche su w.
//
// L'errore è un *ErroreUso per le opzioni sbagliate (modalità, cartelle vuote, uscita o dataset dentro il
// modulo); è un altro errore se il rapporto non si riesce a scrivere. In tutti e due i casi il Rapporto
// restituito è vuoto (Esito vuoto): non c'è un rapporto valido. Se fallisce solo la scrittura su w, il
// rapporto c'è ed è restituito insieme all'errore.
func Esegui(ctx context.Context, o Opzioni, w io.Writer) (Rapporto, error) {
	if err := o.valida(); err != nil {
		return Rapporto{}, err
	}
	r := esegui(ctx, o)
	if err := ScriviRapporto(o.Uscita, r); err != nil {
		return Rapporto{}, err
	}
	if w != nil {
		if _, err := io.WriteString(w, r.Testo()); err != nil {
			return r, err
		}
	}
	return r, nil
}

func (o Opzioni) valida() error {
	switch {
	case o.Modalita == ModalitaDSN || o.Modalita == ModalitaExports:
		return &ErroreUso{Motivo: fmt.Sprintf("modalità %q: si esegue con EseguiBanco, non con Esegui (F0-01)", o.Modalita)}
	case o.Modalita != ModalitaRegole && o.Modalita != ModalitaCasi:
		return &ErroreUso{Motivo: fmt.Sprintf("modalità %q: ci sono %s, %s, %s ed %s", o.Modalita, ModalitaRegole, ModalitaCasi, ModalitaDSN, ModalitaExports)}
	case o.campiDiA1c() != "":
		return &ErroreUso{Motivo: fmt.Sprintf("%s vale solo con le modalità %s ed %s, non con %s (F0-01)", o.campiDiA1c(), ModalitaDSN, ModalitaExports, o.Modalita)}
	case strings.TrimSpace(o.Dataset) == "":
		return &ErroreUso{Motivo: "manca il manifest del dataset (-dataset)"}
	case strings.TrimSpace(o.Uscita) == "":
		return &ErroreUso{Motivo: "manca la cartella dei rapporti (-uscita)"}
	}
	if err := dataset.FuoriDalModulo(o.Uscita); err != nil {
		return &ErroreUso{Motivo: "cartella dei rapporti: " + err.Error()}
	}
	if err := dataset.FuoriDalModulo(o.Dataset); err != nil {
		return &ErroreUso{Motivo: "manifest: " + err.Error()}
	}
	return nil
}

// campiDiA1c: il primo campo delle modalità di A1c impostato, come flag; "" se nessuno. Con regole e casi è un errore
// d'uso (F0-01, condizione c).
func (o Opzioni) campiDiA1c() string {
	switch {
	case o.DSN != "":
		return "-dsn"
	case o.Exports != "":
		return "-exports"
	case len(o.Thread) > 0:
		return "-thread"
	case o.Tutti:
		return "-tutti"
	case o.Attesi:
		return "-attesi"
	case o.Gate:
		return "-gate"
	}
	return ""
}

// esecuzione: lo stato di un'esecuzione mentre si riempie il rapporto.
type esecuzione struct {
	r        Rapporto
	manifest dataset.Manifest
	profili  map[string]uuid.UUID
	indice   *grammatica.IndiceRegole
	cartIdx  string
	attesi   *Attesi
}

func esegui(ctx context.Context, o Opzioni) Rapporto {
	e := &esecuzione{r: Rapporto{
		VersioneRapporto: VersioneRapporto,
		Modalita:         o.Modalita,
		Manifest:         o.Dataset,
		Versioni: Versioni{
			Rapporto: VersioneRapporto, Manifest: dataset.VersioneManifest, Canonicalizzazione: jsoncanonico.Versione,
			SchemaGrammatiche: grammatica.VersioneSchema, Indice: grammatica.VersioneIndice,
			Capacita: grammatica.VersioneCapacita, Algoritmo: motorea.VersioneAlgoritmo,
		},
	}}
	if e.leggiManifest(o.Dataset) {
		e.leggiIndice()
		e.leggiAttesi()
		if ctx.Err() != nil {
			e.controllo("modalità", ControlloNonEseguito, 0, "interrotto: "+ctx.Err().Error())
		} else {
			switch o.Modalita {
			case ModalitaRegole:
				e.regole()
			case ModalitaCasi:
				e.casi()
			}
		}
	}
	e.chiudi()
	return e.r
}

func (e *esecuzione) controllo(nome, stato string, differenze int, motivo string) {
	e.r.Controlli = append(e.r.Controlli, Controllo{Nome: nome, Stato: stato, Differenze: differenze, Motivo: motivo})
}

// chiudi: l'esito dai controlli. Una differenza prevale su un non eseguito (R44).
func (e *esecuzione) chiudi() {
	var motivi []string
	for _, c := range e.r.Controlli {
		e.r.Differenze += c.Differenze
		if c.Stato == ControlloNonEseguito {
			motivi = append(motivi, c.Nome+": "+c.Motivo)
		}
	}
	switch {
	case e.r.Differenze > 0:
		e.r.Esito = EsitoConDifferenze
	case len(motivi) > 0:
		e.r.Esito = EsitoNonEseguito
		e.r.Motivo = strings.Join(motivi, "; ")
	default:
		e.r.Esito = EsitoConforme
	}
}

// leggiManifest: il manifest, la sua impronta e i profili. Senza manifest non si esegue niente.
func (e *esecuzione) leggiManifest(percorso string) bool {
	abs, err := filepath.Abs(percorso)
	if err != nil {
		e.controllo("manifest", ControlloNonEseguito, 0, err.Error())
		return false
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		e.controllo("manifest", ControlloNonEseguito, 0, "il manifest non si legge: "+err.Error())
		return false
	}
	e.r.Sha256Manifest = impronta(raw)
	m, err := dataset.Leggi(raw, filepath.Dir(abs))
	if err != nil {
		e.controllo("manifest", ControlloNonEseguito, 0, err.Error())
		return false
	}
	e.manifest = m
	e.profili = map[string]uuid.UUID{}
	nomi := make([]string, 0, len(m.Profili))
	for nome := range m.Profili {
		nomi = append(nomi, nome)
	}
	sort.Strings(nomi)
	for _, nome := range nomi {
		u, err := uuid.Parse(m.Profili[nome])
		if err != nil {
			e.controllo("manifest", ControlloNonEseguito, 0, fmt.Sprintf("il profilo %q non ha un UUID valido", nome))
			return false
		}
		e.profili[nome] = u
	}
	e.controllo("manifest", ControlloEseguito, 0, "")
	return true
}

func impronta(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// voce: una voce del manifest per nome.
func (e *esecuzione) voce(nome string) (dataset.Voce, bool) {
	for _, v := range e.manifest.Voci {
		if v.Nome == nome {
			return v, true
		}
	}
	return dataset.Voce{}, false
}

// leggiIndice: la voce regole.indice, con sha256 e byte controllati, poi la lettura stretta. Una voce che manca
// o è cambiata: non eseguito; un indice presente ma non valido: una differenza (par.3.6.5).
func (e *esecuzione) leggiIndice() {
	raw, err := e.manifest.LeggiFile(VoceIndice)
	if err != nil {
		e.controllo(VoceIndice, ControlloNonEseguito, 0, err.Error())
		return
	}
	e.r.Sha256Indice = impronta(raw)
	ind, err := grammatica.LeggiIndice(raw)
	if err != nil {
		e.controllo(VoceIndice, ControlloEseguito, 1, "indice non valido: "+testoDiagnostiche(err))
		return
	}
	v, _ := e.voce(VoceIndice)
	e.indice = &ind
	e.cartIdx = filepath.Dir(e.manifest.PercorsoDi(v))
	e.r.VersioneLimiti = ind.Limiti.Versione
	e.controllo(VoceIndice, ControlloEseguito, 0, "")
}

// leggiAttesi: la voce attesi, con sha256 e byte controllati, poi LeggiAttesi.
func (e *esecuzione) leggiAttesi() {
	raw, err := e.manifest.LeggiFile(VoceAttesi)
	if err != nil {
		e.controllo(VoceAttesi, ControlloNonEseguito, 0, err.Error())
		return
	}
	e.r.Sha256Attesi = impronta(raw)
	a, err := LeggiAttesi(raw)
	if err != nil {
		e.controllo(VoceAttesi, ControlloEseguito, 1, err.Error())
		return
	}
	e.attesi = &a
	e.r.VersioneAttesi = a.Testata.VersioneAttesi
	e.controllo(VoceAttesi, ControlloEseguito, 0, "")
}

// contenuti: i byte delle grammatiche dell'indice, letti dalla cartella dell'indice. Un file che non si legge
// non entra: CompilaInsieme lo dà come regole.file_assente, e il cliente è scartato.
func (e *esecuzione) contenuti() map[string][]byte {
	out := map[string][]byte{}
	for _, v := range e.indice.Grammatiche {
		p := filepath.Join(e.cartIdx, filepath.FromSlash(strings.ReplaceAll(v.File, `\`, "/")))
		if b, err := os.ReadFile(p); err == nil {
			out[v.File] = b
		}
	}
	return out
}

func (e *esecuzione) regole() {
	if e.indice == nil {
		e.controllo("grammatiche", ControlloNonEseguito, 0, "senza un indice valido le grammatiche non si compilano")
		e.controllo("coerenza_esempi_attesi", ControlloNonEseguito, 0, "senza grammatiche non c'è niente da confrontare")
		return
	}
	var a Attesi
	if e.attesi != nil {
		a = *e.attesi
	}
	rr := VerificaRegole(e.manifest, *e.indice, e.contenuti(), a)
	e.r.ImprontaIndice = rr.ImprontaIndice
	e.r.Regole = &rr
	diffGrammatiche := rr.ClientiScartati
	if len(rr.IndiceNonValido) > 0 {
		diffGrammatiche++
	}
	e.controllo("grammatiche", ControlloEseguito, diffGrammatiche, "")
	if rr.CoerenzaEseguita {
		e.controllo("coerenza_esempi_attesi", ControlloEseguito, rr.Incoerenze, "")
	} else {
		e.controllo("coerenza_esempi_attesi", ControlloNonEseguito, 0, "gli attesi non sono stati letti")
	}
	e.controllo("copertura", ControlloEseguito, 0, fmt.Sprintf("%d lacune, informative (R20 b)", rr.Lacune))
}

func (e *esecuzione) casi() {
	if e.indice == nil {
		e.controllo("casi_contratto", ControlloNonEseguito, 0, "senza un indice valido i casi non si eseguono")
		return
	}
	ins, diag, err := motorea.CompilaInsieme(*e.indice, e.contenuti())
	if err != nil {
		e.controllo("grammatiche", ControlloEseguito, 1, "indice non valido: "+testoDiagnostiche(err))
		e.controllo("casi_contratto", ControlloNonEseguito, 0, "senza grammatiche i casi non si eseguono")
		return
	}
	e.r.ImprontaIndice = ins.ImprontaIndice
	rc := RapportoCasi{ClientiAttivi: len(ins.Motori), ClientiScartati: len(ins.Scartati)}
	for _, d := range diag {
		if d.Gravita == evidenze.GravitaErrore {
			rc.Scarti = append(rc.Scarti, d)
		}
	}
	e.controllo("grammatiche", ControlloEseguito, len(ins.Scartati), "")
	if e.attesi == nil {
		e.r.Casi = &rc
		e.controllo("casi_contratto", ControlloNonEseguito, 0, "gli attesi non sono stati letti")
		return
	}
	rc.Casi = EseguiCasiContratto(*e.attesi, ins, e.profili)
	rc.Conteggi = Conta(rc.Casi)
	e.r.Casi = &rc
	e.controllo("casi_contratto", ControlloEseguito, rc.Conteggi.Falliti, "")
}

// testoDiagnostiche: codice, percorso e messaggio delle diagnostiche di un errore di contratto.
func testoDiagnostiche(err error) string {
	var ec *evidenze.ErroreContratto
	if !errors.As(err, &ec) {
		return err.Error()
	}
	var parti []string
	for _, d := range ec.Diagnostiche {
		parti = append(parti, strings.TrimSpace(d.Codice+" "+d.Percorso+" "+d.Messaggio))
	}
	return strings.Join(parti, "; ")
}
