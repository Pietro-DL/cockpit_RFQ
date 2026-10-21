package importazione

// Le letture del registro (EM §3.8, «Una lettura») e le due copie che una decisione ne conserva: la copia compatta
// della lettura scelta (decisione identita, §3.6.1) e il segnale visto (non_includere, §7.1). Le copie sono byte per
// byte quelle del registro che la decisione cita (V-1, V-5): mai NFC, e la stessa forma canonica da tutte e due le parti.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// LetturaRegistro: i campi di una lettura che servono fuori dal motore. Gli altri campi (unita', intervallo, ruolo, ...)
// restano nel contenuto e si leggono dal JSON. CodiceProposto e RevProposta (facoltativi; DECA PE-08, EM §3.8): il
// codice e la rev di partenza della lettura per la working, con le stesse proiezioni (importazione-1) dell'identita'
// proposta di una voce; li scrive l'adattatore su ogni lettura che ha una proiezione ammissibile, e servono ad «Adotta
// come codice» su un'alternativa o su un riferimento. Non entrano nella copia compatta (V-1 non cambia).
type LetturaRegistro struct {
	ChiaveLettura  string          `json:"chiave_lettura"`
	Emittente      string          `json:"emittente"`
	HashGrammatica string          `json:"hash_grammatica"`
	Namespace      string          `json:"namespace"`
	Letterale      string          `json:"letterale"`
	Base           json.RawMessage `json:"base"`
	Qualificatori  json.RawMessage `json:"qualificatori"`
	Revisione      json.RawMessage `json:"revisione"`
	CodiceProposto string          `json:"codice_proposto,omitempty"`
	RevProposta    string          `json:"rev_proposta,omitempty"`
}

// LetturaCompatta e' la copia compatta di una lettura (EM §3.6.1): {emittente, hash_grammatica, namespace, letterale,
// base: {originale, normalizzata}, qualificatori, revisione}. Nella forma canonica i facoltativi vuoti o null non si
// scrivono, nemmeno dentro qualificatori e revisione (la regola della forma canonica del registro, SenzaVuoti), e
// qualificatori e revisione vuoti non si scrivono affatto: la stessa regola per la copia della decisione e per quella
// ricavata dal registro, cosi' il confronto di V-1 e' fra byte canonici.
type LetturaCompatta struct {
	Emittente      string          `json:"emittente"`
	HashGrammatica string          `json:"hash_grammatica,omitempty"`
	Namespace      string          `json:"namespace,omitempty"`
	Letterale      string          `json:"letterale"`
	Base           BaseCompatta    `json:"base"`
	Qualificatori  json.RawMessage `json:"qualificatori,omitempty"`
	Revisione      json.RawMessage `json:"revisione,omitempty"`
}

// BaseCompatta: la base letta, solo originale e normalizzata.
type BaseCompatta struct {
	Originale    string `json:"originale,omitempty"`
	Normalizzata string `json:"normalizzata,omitempty"`
}

// Normalizzata: la copia con qualificatori e revisione canonici, e vuoti al posto di {} e null.
func (l LetturaCompatta) Normalizzata() (LetturaCompatta, error) {
	var err error
	if l.Qualificatori, err = opzionale(l.Qualificatori, "qualificatori"); err != nil {
		return l, err
	}
	if l.Revisione, err = opzionale(l.Revisione, "revisione"); err != nil {
		return l, err
	}
	return l, nil
}

// Canonica: i byte canonici della copia normalizzata.
func (l LetturaCompatta) Canonica() ([]byte, error) {
	n, err := l.Normalizzata()
	if err != nil {
		return nil, err
	}
	return jsoncanonico.Codifica(n)
}

// StessaLettura: le due copie hanno gli stessi byte canonici (V-1).
func StessaLettura(a, b LetturaCompatta) bool {
	x, err := a.Canonica()
	if err != nil {
		return false
	}
	y, err := b.Canonica()
	return err == nil && bytes.Equal(x, y)
}

// opzionale: un oggetto JSON canonico senza i membri vuoti (la forma canonica del registro, SenzaVuoti), oppure niente
// per assente, null, {} o un oggetto di soli vuoti.
func opzionale(r json.RawMessage, campo string) (json.RawMessage, error) {
	if len(r) == 0 {
		return nil, nil
	}
	if _, err := jsoncanonico.Codifica(r); err != nil {
		return nil, fmt.Errorf("%s: %v", campo, err)
	}
	c, err := SenzaVuoti(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", campo, err)
	}
	switch s := string(c); {
	case s == "null" || s == "{}":
		return nil, nil
	case !strings.HasPrefix(s, "{"):
		return nil, fmt.Errorf("%s: atteso un oggetto", campo)
	}
	return c, nil
}

// Compatta ricava dalla lettura del registro la sua copia compatta.
func (l LetturaRegistro) Compatta() (LetturaCompatta, error) {
	out := LetturaCompatta{Emittente: l.Emittente, HashGrammatica: l.HashGrammatica, Namespace: l.Namespace,
		Letterale: l.Letterale, Qualificatori: l.Qualificatori, Revisione: l.Revisione}
	if len(l.Base) > 0 && string(l.Base) != "null" {
		var b struct {
			Originale    string `json:"originale"`
			Normalizzata string `json:"normalizzata"`
		}
		if err := json.Unmarshal(l.Base, &b); err != nil {
			return LetturaCompatta{}, fmt.Errorf("base della lettura %s: %v", l.ChiaveLettura, err)
		}
		out.Base = BaseCompatta{Originale: b.Originale, Normalizzata: b.Normalizzata}
	}
	return out.Normalizzata()
}

// TrovaLettura cerca una lettura del registro per chiave_lettura (<allegato_id>/<ID della lettura>, unica dentro il
// registro): nell'identita' proposta, nelle alternative e nei riferimenti delle voci, nell'identita' proposta dei nodi
// fuori per decisione (U2-P22) e nelle letture dei file. Quali
// letture una decisione puo' citare per un nodo (legate alla voce, EM §3.6.1) lo decide chi chiama.
func (c *Contenuto) TrovaLettura(chiave string) (LetturaRegistro, bool) {
	trova := func(raw json.RawMessage) (LetturaRegistro, bool) {
		var l LetturaRegistro
		if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &l) != nil {
			return LetturaRegistro{}, false
		}
		return l, l.ChiaveLettura == chiave
	}
	inElenco := func(raw json.RawMessage) (LetturaRegistro, bool) {
		var ll []json.RawMessage
		if len(raw) == 0 || json.Unmarshal(raw, &ll) != nil {
			return LetturaRegistro{}, false
		}
		for _, x := range ll {
			if l, ok := trova(x); ok {
				return l, true
			}
		}
		return LetturaRegistro{}, false
	}
	for _, v := range c.Proposta.Nodi {
		if l, ok := trova(v.IdentitaProposta); ok {
			return l, true
		}
		if l, ok := inElenco(v.Alternative); ok {
			return l, true
		}
		var rr []struct {
			Lettura json.RawMessage `json:"lettura"`
		}
		if len(v.Riferimenti) > 0 && json.Unmarshal(v.Riferimenti, &rr) == nil {
			for _, r := range rr {
				if l, ok := trova(r.Lettura); ok {
					return l, true
				}
			}
		}
	}
	for _, x := range c.Proposta.FuoriPerDecisione {
		if l, ok := trova(x.IdentitaProposta); ok {
			return l, true
		}
	}
	for _, f := range c.Proposta.File {
		if l, ok := inElenco(f.Letture); ok {
			return l, true
		}
	}
	return LetturaRegistro{}, false
}

// SegnaleVisto e' il segnale LS-02 come stava nel registro della decisione (EM §7.1): codice e regola del segnale della
// voce, versione_servizio dall'ancoraggio delle regole.
type SegnaleVisto struct {
	Codice           string `json:"codice"`
	Regola           string `json:"regola"`
	VersioneServizio string `json:"versione_servizio"`
}

// SegnaliDellaVoce: i segnali della voce n:<allegato_id>:<rif> nella forma del segnale visto.
func (c *Contenuto) SegnaliDellaVoce(voce string) ([]SegnaleVisto, error) {
	for _, v := range c.Proposta.Nodi {
		if v.Voce != voce {
			continue
		}
		if len(v.Segnali) == 0 || string(v.Segnali) == "null" {
			return nil, nil
		}
		var ss []struct {
			Codice string `json:"codice"`
			Regola string `json:"regola"`
		}
		if err := json.Unmarshal(v.Segnali, &ss); err != nil {
			return nil, fmt.Errorf("segnali della voce %s: %v", voce, err)
		}
		out := make([]SegnaleVisto, 0, len(ss))
		for _, s := range ss {
			out = append(out, SegnaleVisto{Codice: s.Codice, Regola: s.Regola, VersioneServizio: c.Regole.Versioni.Ancoraggio})
		}
		return out, nil
	}
	return nil, nil
}
