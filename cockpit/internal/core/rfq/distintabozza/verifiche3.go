package distintabozza

// Le verifiche del formato 3 che vogliono un registro d'importazione ma non il database (EM §6.2, V-1...V-5): il registro
// lo carica chi chiama (R2 dal database; le prove con importazione.Componi da un contenuto scritto a mano), qui si
// confronta. Nessuna di queste funzioni chiama il motore.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I motivi di CORPO_NON_VALIDO delle verifiche contro il registro (EM §6.2, §9.2).
const (
	MotivoPropostaNonCorrispondente   = "proposta_non_corrispondente"
	MotivoSegnaleNonCorrisponde       = "segnale_non_corrisponde"
	MotivoImportazioneSconosciuta     = "importazione_sconosciuta"
	MotivoEmittenteNonAmmesso         = "emittente_non_ammesso"
	MotivoRegistratoNonCorrispondente = "registrato_non_corrispondente"
)

// SegnaliAttesi: i segnali che il registro dice per la voce n:<allegato_id>:<rif> di una casella, nella forma del segnale
// visto. versione_servizio e' regole.versioni.ancoraggio del registro (PE-17, decisione [T] dell'integratore del 18/10):
// importazione.SegnaliDellaVoce la copia da li', mai dal motore di oggi.
func SegnaliAttesi(c *importazione.Contenuto, casella Casella3) ([]importazione.SegnaleVisto, error) {
	voce := "n:" + casella.AllegatoID.String() + ":" + casella.Rif
	return c.SegnaliDellaVoce(voce)
}

// VerificaSegnaleVisto e' V-5 (EM §6.2, §7.1, §7.2): il segnale_visto di una non_includere aggiunta o cambiata nel
// salvataggio e' uno dei segnali che il registro della decisione dice per quel rif, byte per byte (codice, regola e
// versione_servizio = regole.versioni.ancoraggio, PE-17); se il registro non segnala il rif, la decisione non porta un
// segnale. Un difetto e' 422 CORPO_NON_VALIDO, motivo segnale_non_corrisponde.
func VerificaSegnaleVisto(campo string, c *importazione.Contenuto, d Decisione3) *Errore {
	if d.Tipo != DecisioneNonIncludere || d.Casella == nil {
		return nil
	}
	attesi, err := SegnaliAttesi(c, *d.Casella)
	if err != nil {
		return corpoMotivo(campo+".segnale_visto", MotivoSegnaleNonCorrisponde, "i segnali del registro per %s non si leggono: %v", d.Casella.Rif, err)
	}
	if d.SegnaleVisto == nil {
		if len(attesi) > 0 {
			return corpoMotivo(campo+".segnale_visto", MotivoSegnaleNonCorrisponde,
				"il registro segnala %s (%s): la decisione porta il segnale visto", d.Casella.Rif, attesi[0].Codice)
		}
		return nil
	}
	visto, err := jsoncanonico.Codifica(d.SegnaleVisto)
	if err != nil {
		return corpoMotivo(campo+".segnale_visto", MotivoSegnaleNonCorrisponde, "il segnale visto non si legge: %v", err)
	}
	for _, a := range attesi {
		if b, err := jsoncanonico.Codifica(a); err == nil && bytes.Equal(b, visto) {
			return nil
		}
	}
	if len(attesi) == 0 {
		return corpoMotivo(campo+".segnale_visto", MotivoSegnaleNonCorrisponde, "il registro non segnala %s: la decisione non porta un segnale", d.Casella.Rif)
	}
	return corpoMotivo(campo+".segnale_visto", MotivoSegnaleNonCorrisponde,
		"il segnale visto non e' quello del registro per %s (codice, regola, versione_servizio = %s)", d.Casella.Rif, c.Regole.Versioni.Ancoraggio)
}

// VociDelRegistro: una casella cita una voce intera del registro, n:<allegato_id>:<rif>, fra le voci o fra quelle fuori per
// decisione (V-3, EM §6.2): lo stesso rif con l'allegato di un'altra occorrenza non basta.
func VoceDelRegistro(c *importazione.Contenuto, allegato uuid.UUID, rif string) bool {
	if _, ok := c.VocePerRif(allegato, rif); ok {
		return true
	}
	_, ok := c.FuoriPerRif(allegato, rif)
	return ok
}

// letturaDellaVoce dice se la lettura di chiave k e' legata alla voce v (EM §3.6.1, chiave_lettura): l'identita' proposta,
// una delle alternative, la lettura di un riferimento, la lettura del codice della parte di una prova di un equivalente K4
// (<allegato della prova>/<lettura_codice>), oppure una lettura di un file che ha la voce fra i suoi candidati.
func letturaDellaVoce(c *importazione.Contenuto, v importazione.Voce, k string) bool {
	chiaveDi := func(raw json.RawMessage) string {
		var l struct {
			ChiaveLettura string `json:"chiave_lettura"`
		}
		if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &l) != nil {
			return ""
		}
		return l.ChiaveLettura
	}
	if chiaveDi(v.IdentitaProposta) == k {
		return true
	}
	var alt []json.RawMessage
	if len(v.Alternative) > 0 && json.Unmarshal(v.Alternative, &alt) == nil {
		for _, a := range alt {
			if chiaveDi(a) == k {
				return true
			}
		}
	}
	var rr []struct {
		Lettura     json.RawMessage `json:"lettura"`
		Equivalente *struct {
			Prove []struct {
				AllegatoID    string `json:"allegato_id"`
				LetturaCodice string `json:"lettura_codice"`
			} `json:"prove"`
		} `json:"equivalente"`
	}
	if len(v.Riferimenti) > 0 && json.Unmarshal(v.Riferimenti, &rr) == nil {
		for _, r := range rr {
			if chiaveDi(r.Lettura) == k {
				return true
			}
			if r.Equivalente != nil {
				for _, p := range r.Equivalente.Prove {
					if p.AllegatoID+"/"+p.LetturaCodice == k {
						return true
					}
				}
			}
		}
	}
	for _, f := range c.Proposta.File {
		var cand []string
		if len(f.Candidati) == 0 || json.Unmarshal(f.Candidati, &cand) != nil {
			continue
		}
		perVoce := false
		for _, x := range cand {
			if x == v.Voce {
				perVoce = true
			}
		}
		if !perVoce {
			continue
		}
		var ll []json.RawMessage
		if len(f.Letture) > 0 && json.Unmarshal(f.Letture, &ll) == nil {
			for _, l := range ll {
				if chiaveDi(l) == k {
					return true
				}
			}
		}
	}
	return false
}

// VerificaLetturaDecisione e' V-1 e V-3 di una decisione identita contro il suo registro (EM §3.6.1, §6.2):
//   - con chiave_lettura la lettura sta nel registro, e' legata alla voce del nodo (per un nodo senza casella basta che
//     stia nel registro), e la copia compatta e' byte per byte quella del registro;
//   - con riferimento manuale_legacy la voce del nodo ha codice_manuale_legacy, e l'identita' ne porta codice e rev.
//
// La proiezione importazione-1 della lettura come codice (EM §3.6.1) la controlla l'adattatore quando il registro porta
// codice_proposto per ogni lettura (PE-08, corsia motore): finche' non c'e', qui si controllano emittente e namespace
// (la forma, decisioni3.go) e la copia della lettura.
func VerificaLetturaDecisione(campo string, c *importazione.Contenuto, d Decisione3, casella *Casella3) *Errore {
	if d.Tipo != DecisioneIdentita || d.Modo != ModoAlternativa {
		return nil
	}
	var voce *importazione.Voce
	if casella != nil {
		v, ok := c.VocePerRif(casella.AllegatoID, casella.Rif)
		if !ok {
			return corpoMotivo(campo+".chiave_bozza", MotivoRifFuoriImportazione, "la casella del nodo non e' una voce del registro della decisione")
		}
		voce = &v
	}
	if d.ChiaveLettura != "" {
		l, ok := c.TrovaLettura(d.ChiaveLettura)
		if !ok {
			return corpoMotivo(campo+".chiave_lettura", MotivoRifFuoriImportazione, "la lettura %s non c'e' nel registro della decisione", d.ChiaveLettura)
		}
		if voce != nil && !letturaDellaVoce(c, *voce, d.ChiaveLettura) {
			return corpoMotivo(campo+".chiave_lettura", MotivoRifFuoriImportazione, "la lettura %s non e' legata alla voce del nodo", d.ChiaveLettura)
		}
		comp, err := l.Compatta()
		if err != nil {
			return corpoMotivo(campo+".lettura", MotivoPropostaNonCorrispondente, "la lettura %s del registro non si compatta: %v", d.ChiaveLettura, err)
		}
		if d.Lettura == nil || !importazione.StessaLettura(comp, *d.Lettura) {
			return corpoMotivo(campo+".lettura", MotivoPropostaNonCorrispondente, "la copia della lettura non e' quella del registro (byte per byte)")
		}
		return nil
	}
	// riferimento manuale_legacy
	if voce == nil || len(voce.CodiceManualeLegacy) == 0 || string(voce.CodiceManualeLegacy) == "null" {
		return corpoMotivo(campo+".riferimento", MotivoRifFuoriImportazione, "la voce del nodo non ha un codice corretto a mano nella pagina vecchia")
	}
	var m struct {
		Codice string `json:"codice"`
		Rev    string `json:"rev"`
	}
	if err := json.Unmarshal(voce.CodiceManualeLegacy, &m); err != nil {
		return corpoMotivo(campo+".riferimento", MotivoPropostaNonCorrispondente, "codice_manuale_legacy non si legge: %v", err)
	}
	if d.Identita == nil || d.Identita.Codice != m.Codice || d.Identita.Rev != m.Rev {
		return corpoMotivo(campo+".identita", MotivoPropostaNonCorrispondente,
			"adottare il codice manuale vuole codice e rev di codice_manuale_legacy (%s %s)", m.Codice, m.Rev)
	}
	return nil
}

// IdentitaDellaProposta e' l'identita' che un nodo in modo proposta porta (EM §3.4.2): emittente e namespace della lettura
// primaria della voce, codice = codice_proposto, rev = rev_proposta (anche vuota: una rev vuota vale assente, PE-11).
// ok falso: la voce non ha un codice proposto (allora il nodo e' da_decidere, non proposta).
func IdentitaDellaProposta(v importazione.Voce) (Identita, bool, error) {
	if v.CodiceProposto == "" {
		return Identita{}, false, nil
	}
	var l struct {
		Emittente string `json:"emittente"`
		Namespace string `json:"namespace"`
	}
	if len(v.IdentitaProposta) == 0 || string(v.IdentitaProposta) == "null" {
		return Identita{}, false, fmt.Errorf("la voce %s ha il codice proposto senza l'identita' proposta", v.Voce)
	}
	if err := json.Unmarshal(v.IdentitaProposta, &l); err != nil {
		return Identita{}, false, fmt.Errorf("l'identita' proposta di %s non si legge: %v", v.Voce, err)
	}
	return Identita{Emittente: l.Emittente, Namespace: l.Namespace, Codice: v.CodiceProposto, Rev: v.RevProposta, Modo: ModoProposta}, true, nil
}
