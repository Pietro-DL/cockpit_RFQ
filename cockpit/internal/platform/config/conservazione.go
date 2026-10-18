package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Conservazione e' [conservazione] (V1, EM §10.5; base di lavoro della domanda V1-Q06 A): il deposito degli «Altri
// file» della Sorgente. «Metti in Altri file» copia i byte del file qui, verificati con lo sha256, PRIMA di cambiare lo
// stato: la cache dello staging si svuota da sola, e un file messo da parte deve restare apribile dopo un riavvio e
// dopo la pulizia dello staging. Non e' la cartella ALTRO del NAS e non sta sul NAS: lo scrive solo il server.
//
// Una voce sola. Assente (o vuota): il server parte, e «Metti in Altri file» risponde 503 NON_CONFIGURATO con il
// motivo «conservazione»; il resto della Sorgente funziona. La cartella, il suo segnaposto e il controllo delle copie
// gia' registrate li fa l'avvio (core/rfq/filerfq, Deposito.Prepara), non questo pacchetto: qui solo i percorsi.
type Conservazione struct {
	Radice string `toml:"radice"`
}

// Configurata: la sezione c'e', con una radice.
func (c Conservazione) Configurata() bool { return strings.TrimSpace(c.Radice) != "" }

// unitaDiReteFn dice se la lettera di un percorso e' una condivisione di rete mappata (conservazione_windows.go). E'
// una variabile per le prove: una lettera di rete vera non c'e' su ogni macchina.
var unitaDiReteFn = unitaDiRete

// normalizzaConservazione controlla la radice del deposito (EM §10.5, «sovrapposizioni vietate»; giro 1, SIC-16):
//   - il deposito sta su un disco locale: un percorso UNC o una lettera di rete si rifiutano, perche' il confronto con
//     il NAS non vedrebbe una lettera mappata proprio sul NAS;
//   - non coincide con lo staging, con la radice del NAS o con una delle radici di produzione, e non sta ne' sotto ne'
//     sopra una di loro: un deposito dentro `_contenuti` lo svuoterebbe la cache, uno sul NAS scriverebbe sul NAS, e
//     uno che contiene lo staging lo conterrebbe nelle sue copie di sicurezza.
//
// I percorsi si confrontano risolti (assoluti, con i collegamenti seguiti fin dove esistono, il volume compreso),
// senza maiuscole, con le barre uniformi e senza la barra finale. Una violazione e' un errore di configurazione: il
// server non parte, e il messaggio dice quale voce si sovrappone.
func (c *Config) normalizzaConservazione() error {
	r := strings.TrimSpace(c.Conservazione.Radice)
	if r == "" {
		c.Conservazione.Radice = ""
		return nil
	}
	if percorsoUNC(r) || unitaDiReteFn(r) {
		return fmt.Errorf("config: [conservazione].radice = %q: il deposito degli Altri file sta su un disco locale di questo "+
			"server, e una cartella di rete (UNC o lettera mappata) non si accetta: il controllo che non stia sul NAS non la "+
			"riconoscerebbe", r)
	}
	if !filepath.IsAbs(r) {
		return fmt.Errorf("config: [conservazione].radice = %q non e' un percorso assoluto", r)
	}
	risolta, err := percorsoRisolto(r)
	if err != nil {
		return fmt.Errorf("config: [conservazione].radice = %q non si risolve: %w", r, err)
	}
	if percorsoUNC(risolta) || unitaDiReteFn(risolta) {
		return fmt.Errorf("config: [conservazione].radice = %q porta, seguendo i collegamenti, a una cartella di rete (%s): "+
			"il deposito sta su un disco locale", r, risolta)
	}
	altre := []struct{ voce, percorso string }{
		{"[nas].staging", c.NAS.Staging},
		{"[nas].radice", c.NAS.Radice},
	}
	for _, p := range c.RadiciProduzione() {
		altre = append(altre, struct{ voce, percorso string }{"[nas].radici_produzione", p})
	}
	for _, a := range altre {
		if strings.TrimSpace(a.percorso) == "" {
			continue
		}
		ra, err := percorsoRisolto(a.percorso)
		if err != nil {
			return fmt.Errorf("config: %s = %q non si risolve per il controllo di [conservazione].radice: %w", a.voce, a.percorso, err)
		}
		if sovrapposti(confrontabile(risolta), confrontabile(ra)) {
			return fmt.Errorf("config: [conservazione].radice = %q si sovrappone a %s = %q: il deposito degli Altri file deve "+
				"stare fuori dallo staging, dal NAS e dalle radici di produzione (ne' la stessa cartella, ne' sotto, ne' sopra)",
				r, a.voce, a.percorso)
		}
	}
	c.Conservazione.Radice = filepath.Clean(r)
	return nil
}

// percorsoUNC: \\server\condivisione o //server/condivisione.
func percorsoUNC(p string) bool {
	p = strings.TrimSpace(p)
	return strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//")
}

// percorsoRisolto rende un percorso confrontabile con un altro: assoluto e con i collegamenti seguiti fin dove la
// cartella esiste (il deposito puo' non esistere ancora: lo crea l'avvio), sul volume finale. Su Windows la parte che
// esiste si risolve attraverso l'handle (percorsoFinale: GetFinalPathNameByHandleW), che segue anche le giunzioni (i
// mount point, che filepath.EvalSymlinks non segue piu' con winsymlink=1) e le unita' SUBST, e dice il volume vero
// (rilievo della revisione); fuori da Windows filepath.EvalSymlinks. Un percorso UNC non si tocca: non si apre la rete
// per controllare un file di configurazione.
func percorsoRisolto(p string) (string, error) {
	p = strings.TrimSpace(p)
	if percorsoUNC(p) {
		return p, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	resto := ""
	cur := abs
	for {
		if _, err := os.Lstat(cur); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		padre := filepath.Dir(cur)
		if padre == cur {
			// nemmeno la radice del volume esiste: si confronta il percorso com'e'
			return abs, nil
		}
		resto = filepath.Join(filepath.Base(cur), resto)
		cur = padre
	}
	reale, err := percorsoFinale(cur)
	if err != nil {
		return "", err
	}
	if resto != "" {
		reale = filepath.Join(reale, resto)
	}
	return filepath.Clean(reale), nil
}

// confrontabile: senza maiuscole, con le barre uniformi e senza la barra finale (NTFS non distingue le maiuscole).
func confrontabile(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	for len(p) > 3 && strings.HasSuffix(p, `\`) {
		p = strings.TrimSuffix(p, `\`)
	}
	return strings.ToLower(p)
}

// sovrapposti: la stessa cartella, oppure una sotto l'altra.
func sovrapposti(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.HasPrefix(a, strings.TrimSuffix(b, `\`)+`\`) || strings.HasPrefix(b, strings.TrimSuffix(a, `\`)+`\`)
}
