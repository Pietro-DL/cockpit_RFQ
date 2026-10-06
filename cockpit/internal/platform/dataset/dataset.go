// Package dataset legge il manifest del dataset privato della consegna A, un JSON che sta fuori dai due rami
// del repository, e controlla sha256 e byte di ogni file prima di darlo a chi lo chiede (par.3.7 del piano A).
// Non conosce il formato dei file che elenca: attesi, indice delle regole ed elenchi del controllo prima del
// push li legge chi li usa.
//
// Nessun valore del dataset sta qui: percorsi, impronte, profili e UUID dei clienti sono nel manifest, che
// resta privato (R2). Il pacchetto non importa niente del progetto, perché platform non importa core: qui non
// c'è nessuna evidenze.Diagnostica, solo errori. I due motivi per cui un file non si dà sono errori tipizzati,
// che chi li riceve mostra come NON ESEGUITO e non salta mai (R44).
package dataset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// VersioneManifest: la versione del manifest che questo codice sa leggere. Un manifest con un'altra versione,
// o senza, non si legge: chi lo usa risulta NON ESEGUITO.
const VersioneManifest = 1

// Manifest: il manifest del dataset privato della consegna A, un JSON fuori dal repository. I percorsi delle
// voci sono relativi alla sua cartella. Nessun valore del dataset sta nel codice.
type Manifest struct {
	Versione int               `json:"versione_manifest"`
	Cartella string            `json:"-"`
	Voci     []Voce            `json:"voci"`              // ruoli in A1a: attesi, regole, controllo (gli elenchi del controllo prima del push)
	Profili  map[string]string `json:"profili,omitempty"` // profilo degli attesi → cliente_id: solo per il runner (D-09)
	Copia    CopiaAttesa       `json:"copia"`             // la copia intatta del dump: la usa A1c, in A1a è vuota
	CopiaRun CopiaAttesa       `json:"copia_run"`         // la copia _run, scrivibile: nome, ruolo, schema, sentinelle (A1c, R33 d)
	Export   *ExportDichiarato `json:"export,omitempty"`  // la terna e lo schema degli export (A1c)
	Storico  []RigaStorico     `json:"storico,omitempty"`
}

// ExportDichiarato: ciò che gli export non dicono da soli, e che il manifest privato dichiara (piano A, 6.4.8): la
// terna dei fatti (analizzatore_corrente non è esportato) e lo schema del DB da cui vengono. I valori stanno solo
// nel manifest. Se la sezione c'è, ha tutte e tre le chiavi: una terna a metà non si inventa.
type ExportDichiarato struct {
	Versione           int16  `json:"versione"`            // la versione dell'analizzatore
	HashConfigurazione string `json:"hash_configurazione"` // lo sha256 della configurazione, 64 cifre esadecimali
	Schema             int    `json:"schema"`              // la versione dello schema del DB degli export
}

// Voce: un file del dataset, per nome logico. Ruolo: attesi | regole | casi | dump | export | fixture |
// storico | controllo. «controllo» sono gli elenchi privati del controllo prima del push (par.3.7.6).
// Provenienza vale per le fixture fornite dall'utente (R15).
type Voce struct {
	Nome        string `json:"nome"`
	Percorso    string `json:"percorso"` // relativo alla cartella del manifest, con «/»
	Ruolo       string `json:"ruolo"`
	Sha256      string `json:"sha256"` // del file com'è sul disco, in esadecimale
	Byte        int64  `json:"byte"`
	Provenienza string `json:"provenienza,omitempty"`
}

// CopiaAttesa: che cosa deve essere la copia intatta del dump (A1c). Nessun valore sta nel codice: nome del
// DB, ruolo, schema, tabelle escluse e sentinelle vengono tutti dal manifest.
type CopiaAttesa struct {
	Database   string           `json:"database,omitempty"`
	Ruolo      string           `json:"ruolo,omitempty"`
	Schema     int              `json:"schema,omitempty"`
	Escluse    []string         `json:"escluse,omitempty"`
	Sentinelle map[string]int64 `json:"sentinelle,omitempty"`
}

// RigaStorico: un cambio di un file già elencato, con lo sha256 di prima e il motivo (par.3.7.2).
type RigaStorico struct {
	Data        string `json:"data"`
	Voce        string `json:"voce"`
	Sha256Prima string `json:"sha256_prima"`
	Motivo      string `json:"motivo"`
}

// I ruoli di una voce (par.3.3.9). Un ruolo fuori elenco rende il manifest illeggibile, mai un ripiego.
const (
	RuoloAttesi    = "attesi"
	RuoloRegole    = "regole"
	RuoloCasi      = "casi"
	RuoloDump      = "dump"
	RuoloExport    = "export"
	RuoloFixture   = "fixture"
	RuoloStorico   = "storico"
	RuoloControllo = "controllo"
)

var ruoli = []string{RuoloAttesi, RuoloRegole, RuoloCasi, RuoloDump, RuoloExport, RuoloFixture, RuoloStorico, RuoloControllo}

// ErrFileMancante, ErrImprontaDiversa: un file del manifest che non c'è, o che non è quello dichiarato. Chi li
// riceve non salta e non continua: le prove li mostrano come NON ESEGUITA, il banco esce con 3 (R44).
var (
	ErrFileMancante    = errors.New("dataset: file mancante")
	ErrImprontaDiversa = errors.New("dataset: sha256 o byte diversi dal manifest")
)

// sha256Esadecimale: 64 cifre esadecimali. Le maiuscole delle cifre non cambiano il valore.
var sha256Esadecimale = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// Leggi: decodifica stretta del manifest, come le grammatiche (prima i token, poi DisallowUnknownFields).
// Rifiuta la BOM, l'UTF-8 non valido, i surrogati soli scritti in escape, il testo dopo l'oggetto, le chiavi
// sconosciute, ripetute o con le maiuscole diverse dal tag, i null, i numeri non interi, le chiavi
// obbligatorie assenti. Poi controlla la versione e le voci: nome presente e unico, percorso relativo, ruolo
// dell'elenco, sha256 di 64 cifre esadecimali, byte non negativi; profili con nome e cliente. cartella è la
// cartella del manifest: i percorsi delle voci si leggono da lì. Non legge nessun file.
func Leggi(raw []byte, cartella string) (Manifest, error) {
	if err := controllaStretto(raw); err != nil {
		return Manifest{}, fmt.Errorf("dataset: manifest non valido: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("dataset: manifest non valido: %w", err)
	}
	if err := m.valida(); err != nil {
		return Manifest{}, fmt.Errorf("dataset: manifest non valido: %w", err)
	}
	m.Cartella = cartella
	return m, nil
}

func (m Manifest) valida() error {
	if m.Versione != VersioneManifest {
		return fmt.Errorf("versione_manifest %d: questo codice legge la versione %d", m.Versione, VersioneManifest)
	}
	visti := map[string]bool{}
	for i, v := range m.Voci {
		p := fmt.Sprintf("voci[%d]", i)
		switch {
		case strings.TrimSpace(v.Nome) == "":
			return fmt.Errorf("%s.nome: vuoto", p)
		case visti[v.Nome]:
			return fmt.Errorf("%s.nome: la voce %q compare due volte", p, v.Nome)
		case strings.TrimSpace(v.Percorso) == "":
			return fmt.Errorf("%s.percorso: vuoto", p)
		case assoluto(v.Percorso):
			return fmt.Errorf("%s.percorso: deve essere relativo alla cartella del manifest", p)
		case !in(v.Ruolo, ruoli):
			return fmt.Errorf("%s.ruolo: %q non è nell'elenco (%s)", p, v.Ruolo, strings.Join(ruoli, ", "))
		case !sha256Esadecimale.MatchString(v.Sha256):
			return fmt.Errorf("%s.sha256: servono 64 cifre esadecimali", p)
		case v.Byte < 0:
			return fmt.Errorf("%s.byte: negativo", p)
		}
		visti[v.Nome] = true
	}
	nomi := make([]string, 0, len(m.Profili))
	for nome := range m.Profili {
		nomi = append(nomi, nome)
	}
	sort.Strings(nomi) // il primo problema detto è sempre lo stesso
	for _, nome := range nomi {
		if strings.TrimSpace(nome) == "" || strings.TrimSpace(m.Profili[nome]) == "" {
			return errors.New("profili: un profilo senza nome o senza cliente")
		}
	}
	if e := m.Export; e != nil {
		switch {
		case e.Versione <= 0:
			return errors.New("export.versione: serve la versione dell'analizzatore, positiva")
		case !sha256Esadecimale.MatchString(e.HashConfigurazione):
			return errors.New("export.hash_configurazione: servono 64 cifre esadecimali")
		case e.Schema <= 0:
			return errors.New("export.schema: serve la versione dello schema, positiva")
		}
	}
	return nil
}

// assoluto: un percorso con la radice o con l'unità, in una delle due grafie.
func assoluto(p string) bool {
	return filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`)
}

// LeggiFile legge un file per nome logico e ne controlla sha256 e byte. Un file mancante o cambiato è un errore
// tipizzato (ErrFileMancante, ErrImprontaDiversa), mai un salto: chi lo riceve risulta NON ESEGUITO. Una voce
// che il manifest non ha è un file mancante. Il messaggio dice la voce, mai il contenuto.
func (m Manifest) LeggiFile(nome string) ([]byte, error) {
	v, ok := m.voce(nome)
	if !ok {
		return nil, fmt.Errorf("%w: il manifest non ha la voce %q", ErrFileMancante, nome)
	}
	b, err := os.ReadFile(m.PercorsoDi(v))
	if err != nil {
		return nil, fmt.Errorf("%w: voce %q: %v", ErrFileMancante, nome, err)
	}
	if int64(len(b)) != v.Byte {
		return nil, fmt.Errorf("%w: voce %q: %d byte, il manifest ne dichiara %d", ErrImprontaDiversa, nome, len(b), v.Byte)
	}
	s := sha256.Sum256(b)
	if hex.EncodeToString(s[:]) != strings.ToLower(v.Sha256) {
		return nil, fmt.Errorf("%w: voce %q: lo sha256 non è quello del manifest", ErrImprontaDiversa, nome)
	}
	return b, nil
}

// PercorsoDi: il percorso sul disco di una voce, dalla cartella del manifest. Serve a chi deve leggere i file
// che una voce cita a sua volta (le grammatiche, relative alla cartella dell'indice).
func (m Manifest) PercorsoDi(v Voce) string {
	return filepath.Join(m.Cartella, filepath.FromSlash(path.Clean(strings.ReplaceAll(v.Percorso, `\`, "/"))))
}

func (m Manifest) voce(nome string) (Voce, bool) {
	for _, v := range m.Voci {
		if v.Nome == nome {
			return v, true
		}
	}
	return Voce{}, false
}

// FuoriDalModulo rifiuta un percorso dentro l'albero che contiene go.mod: le uscite e il dataset non entrano
// mai nel repository. Risale dal percorso (che può non esistere ancora, come una cartella di uscita) fino alla
// radice del disco, e si ferma al primo go.mod.
func FuoriDalModulo(percorso string) error {
	if strings.TrimSpace(percorso) == "" {
		return errors.New("dataset: percorso vuoto")
	}
	abs, err := filepath.Abs(percorso)
	if err != nil {
		return fmt.Errorf("dataset: %s: %w", percorso, err)
	}
	for d := abs; ; {
		if fi, err := os.Stat(filepath.Join(d, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return fmt.Errorf("dataset: %s sta dentro il modulo di %s: il dataset e le uscite del banco non entrano mai nel repository", percorso, d)
		}
		su := filepath.Dir(d)
		if su == d {
			return nil
		}
		d = su
	}
}

func in(v string, elenco []string) bool {
	for _, x := range elenco {
		if x == v {
			return true
		}
	}
	return false
}

// ---- la lettura a token ----

// schema: le chiavi ammesse in un oggetto del manifest, con le obbligatorie; oppure una mappa con chiavi
// libere, un array o una foglia. Serve solo al passaggio a token: encoding/json accetterebbe in silenzio una
// chiave con le maiuscole sbagliate e lascerebbe vincere un doppione.
type schema struct {
	chiavi       map[string]*schema
	obbligatorie []string
	mappa        *schema // oggetto con chiavi libere (profili, sentinelle)
	elementi     *schema // array
}

var foglia = &schema{}

// schemaCopia: una copia del dump attesa (copia, copia_run), con le stesse chiavi.
var schemaCopia = &schema{chiavi: map[string]*schema{
	"database": foglia, "ruolo": foglia, "schema": foglia,
	"escluse":    {elementi: foglia},
	"sentinelle": {mappa: foglia},
}}

var schemaManifest = &schema{
	chiavi: map[string]*schema{
		"versione_manifest": foglia,
		"voci": {elementi: &schema{
			chiavi: map[string]*schema{
				"nome": foglia, "percorso": foglia, "ruolo": foglia, "sha256": foglia, "byte": foglia, "provenienza": foglia,
			},
			obbligatorie: []string{"nome", "percorso", "ruolo", "sha256", "byte"},
		}},
		"profili":   {mappa: foglia},
		"copia":     schemaCopia,
		"copia_run": schemaCopia,
		"export": {
			chiavi:       map[string]*schema{"versione": foglia, "hash_configurazione": foglia, "schema": foglia},
			obbligatorie: []string{"versione", "hash_configurazione", "schema"},
		},
		"storico": {elementi: &schema{
			chiavi:       map[string]*schema{"data": foglia, "voce": foglia, "sha256_prima": foglia, "motivo": foglia},
			obbligatorie: []string{"data", "voce", "sha256_prima", "motivo"},
		}},
	},
	obbligatorie: []string{"versione_manifest", "voci"},
}

// controllaStretto: BOM, UTF-8, surrogati, poi i token contro lo schema, poi niente dopo l'oggetto.
func controllaStretto(raw []byte) error {
	if bytes.HasPrefix(raw, []byte("\xef\xbb\xbf")) {
		return errors.New("il file comincia con la firma UTF-8 (BOM): va salvato in UTF-8 senza BOM")
	}
	if !utf8.Valid(raw) {
		return errors.New("il file non è UTF-8 valido")
	}
	if err := surrogatiSoli(raw); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return errors.New("il file deve contenere un oggetto JSON")
	}
	if err := leggiOggetto(dec, schemaManifest, ""); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("testo dopo l'oggetto: il manifest contiene un valore solo")
	}
	return nil
}

// leggiOggetto legge le coppie di un oggetto già aperto, fino alla graffa chiusa.
func leggiOggetto(dec *json.Decoder, s *schema, p string) error {
	viste := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		k := t.(string) // dentro un oggetto encoding/json dà sempre una stringa come chiave
		pk := unisci(p, k)
		if viste[k] {
			return fmt.Errorf("%s: chiave ripetuta", pk)
		}
		viste[k] = true
		figlio := s.mappa
		if figlio == nil {
			figlio = s.chiavi[k]
			if figlio == nil {
				for nome := range s.chiavi {
					if strings.EqualFold(nome, k) {
						return fmt.Errorf("%s: chiave con le maiuscole diverse da %q", pk, nome)
					}
				}
				return fmt.Errorf("%s: chiave sconosciuta", pk)
			}
		}
		if err := leggiValore(dec, figlio, pk); err != nil {
			return err
		}
	}
	for _, k := range s.obbligatorie {
		if !viste[k] {
			return fmt.Errorf("%s: chiave obbligatoria assente", unisci(p, k))
		}
	}
	_, err := dec.Token() // la graffa chiusa
	return err
}

func leggiValore(dec *json.Decoder, s *schema, p string) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	switch v := t.(type) {
	case nil:
		return fmt.Errorf("%s: null non ammesso", p)
	case json.Number:
		if strings.ContainsAny(v.String(), ".eE") {
			return fmt.Errorf("%s: numero non intero", p)
		}
		return nil
	case json.Delim:
		switch v {
		case '{':
			if s.chiavi == nil && s.mappa == nil {
				return fmt.Errorf("%s: oggetto dove serve un valore semplice o un array", p)
			}
			return leggiOggetto(dec, s, p)
		case '[':
			if s.elementi == nil {
				return fmt.Errorf("%s: array dove non è previsto", p)
			}
			for i := 0; dec.More(); i++ {
				if err := leggiValore(dec, s.elementi, fmt.Sprintf("%s[%d]", p, i)); err != nil {
					return err
				}
			}
			_, err := dec.Token() // la quadra chiusa
			return err
		}
	}
	return nil
}

func unisci(p, k string) string {
	if p == "" {
		return k
	}
	return p + "." + k
}

// surrogatiSoli: un surrogato scritto in escape senza la sua metà («\ud800»), che encoding/json cambierebbe
// in silenzio in U+FFFD. Scorre le stringhe con i loro escape, così «\\ud800» (una barra e del testo) non
// conta.
func surrogatiSoli(raw []byte) error {
	inStringa := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inStringa {
			if c == '"' {
				inStringa = true
			}
			continue
		}
		switch c {
		case '"':
			inStringa = false
		case '\\':
			if i+1 < len(raw) && raw[i+1] == 'u' && i+6 <= len(raw) {
				u := strings.ToLower(string(raw[i+2 : i+6]))
				if u >= "d800" && u <= "dbff" {
					// la metà alta vuole subito dopo una metà bassa
					if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
						return errors.New("un surrogato solo scritto in escape")
					}
					b := strings.ToLower(string(raw[i+8 : i+12]))
					if b < "dc00" || b > "dfff" {
						return errors.New("un surrogato solo scritto in escape")
					}
					i += 11
					continue
				}
				if u >= "dc00" && u <= "dfff" {
					return errors.New("un surrogato solo scritto in escape")
				}
				i += 5
				continue
			}
			i++ // l'escape di un carattere
		}
	}
	return nil
}
