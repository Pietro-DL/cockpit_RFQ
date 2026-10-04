package evidenze

import "fmt"

// versioneUso: la versione della struttura di UsoSegmenti. Un uso di un'altra versione non si legge.
const versioneUso = 1

// UsoSegmenti: quali segmenti di un documento valgono come richiesta (parte 1 §4.3, con l'origine
// «scenario» del v3 §10.2). È un ingresso dell'interpretazione: cambiarlo cambia funzioni e ruoli, non il
// riconoscimento. «sconosciuto» ha una forma canonica sua, mai NULL.
type UsoSegmenti struct {
	BundleID  string              `json:"bundle_id"`
	Versione  int                 `json:"versione"`
	Stato     string              `json:"stato"` // sconosciuto | valutato
	Selezioni []SelezioneSegmento `json:"selezioni,omitempty"`
}

// SelezioneSegmento: l'uso di un segmento, con l'origine. Un riconoscimento automatico conserva la sua
// provenienza e non finge una decisione umana.
type SelezioneSegmento struct {
	SegmentoID string `json:"segmento_id"`
	Uso        string `json:"uso"`     // pertinente | escluso | da_valutare
	Origine    string `json:"origine"` // operatore | riconoscimento | scenario
	Motivo     string `json:"motivo,omitempty"`
	Rif        string `json:"rif,omitempty"` // il riferimento della decisione o del caso; mai inventato
}

var (
	statiUso   = []string{"sconosciuto", "valutato"}
	usiAmmessi = []string{"pertinente", "escluso", "da_valutare"}
	originiUso = []string{"operatore", "riconoscimento", "scenario"}
)

// UsoSconosciuto: la forma canonica di «nessuna selezione» per un documento: stato «sconosciuto», nessuna
// selezione. Ha la sua impronta, mai NULL (parte 1 §9.3).
func UsoSconosciuto(bundleID string) UsoSegmenti {
	return UsoSegmenti{BundleID: bundleID, Versione: versioneUso, Stato: "sconosciuto"}
}

// ValidaUso controlla che un uso dei segmenti appartenga al documento: stesso bundle, versione 1, segmenti
// esistenti, enum ammessi (uso: pertinente | escluso | da_valutare; origine: operatore | riconoscimento |
// scenario). Un uso «sconosciuto» non ha selezioni, e un segmento si seleziona una volta sola. Un uso
// sbagliato è un errore di contratto, non un uso sconosciuto (parte 1 §9.3): nessun ripiego. Tutte le
// diagnostiche hanno il codice documento.uso_non_valido; nil vuol dire valido.
func ValidaUso(u UsoSegmenti, d DocumentoEvidenze) []Diagnostica {
	var out []Diagnostica
	if u.BundleID != d.BundleID {
		out = append(out, Diagnostica{
			Codice:    CodiceDocumentoUsoNonValido,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "uso.bundle_id",
			Messaggio: fmt.Sprintf("l'uso è del bundle %q, il documento è il bundle %q", u.BundleID, d.BundleID),
		})
	}
	if u.Versione != versioneUso {
		out = append(out, Diagnostica{
			Codice:    CodiceDocumentoUsoNonValido,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "uso.versione",
			Messaggio: fmt.Sprintf("versione dell'uso %d: è ammessa solo la %d", u.Versione, versioneUso),
		})
	}
	if !in(u.Stato, statiUso) {
		out = append(out, Diagnostica{
			Codice:    CodiceDocumentoUsoNonValido,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "uso.stato",
			Messaggio: fmt.Sprintf("stato dell'uso %q fuori elenco (sconosciuto | valutato)", u.Stato),
		})
	}
	if u.Stato == "sconosciuto" && len(u.Selezioni) > 0 {
		out = append(out, Diagnostica{
			Codice:    CodiceDocumentoUsoNonValido,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "uso.selezioni",
			Messaggio: "un uso «sconosciuto» non ha selezioni: la sua forma canonica è quella di UsoSconosciuto",
		})
	}
	segmenti := make(map[string]bool, len(d.Segmenti))
	for _, s := range d.Segmenti {
		segmenti[s.ID] = true
	}
	visti := make(map[string]bool, len(u.Selezioni))
	for i, s := range u.Selezioni {
		percorso := fmt.Sprintf("uso.selezioni[%d]", i)
		if !segmenti[s.SegmentoID] {
			out = append(out, Diagnostica{
				Codice:    CodiceDocumentoUsoNonValido,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso + ".segmento_id",
				Messaggio: fmt.Sprintf("il segmento %q non è nel documento", s.SegmentoID),
				Rif:       []string{s.SegmentoID},
			})
		} else if visti[s.SegmentoID] {
			out = append(out, Diagnostica{
				Codice:    CodiceDocumentoUsoNonValido,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso + ".segmento_id",
				Messaggio: fmt.Sprintf("il segmento %q è selezionato due volte", s.SegmentoID),
				Rif:       []string{s.SegmentoID},
			})
		}
		visti[s.SegmentoID] = true
		if !in(s.Uso, usiAmmessi) {
			out = append(out, Diagnostica{
				Codice:    CodiceDocumentoUsoNonValido,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso + ".uso",
				Messaggio: fmt.Sprintf("uso %q fuori elenco (pertinente | escluso | da_valutare)", s.Uso),
			})
		}
		if !in(s.Origine, originiUso) {
			out = append(out, Diagnostica{
				Codice:    CodiceDocumentoUsoNonValido,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso + ".origine",
				Messaggio: fmt.Sprintf("origine %q fuori elenco (operatore | riconoscimento | scenario)", s.Origine),
			})
		}
	}
	return out
}

// in dice se s è uno dei valori ammessi.
func in(s string, ammessi []string) bool {
	for _, a := range ammessi {
		if a == s {
			return true
		}
	}
	return false
}
