package grammatica

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// VersioneIndice: la versione del file indice delle regole (par.3.6 del piano A). Un indice con un'altra
// versione, o senza, non attiva niente.
const VersioneIndice = 1

// IndiceRegole: il file indicato dal manifest del banco e, da A1d, da [motore_a].regole. Dice quale
// grammatica vale per quale cliente: il legame è per UUID, con lo sha256 di ogni file (R29 d), perché un file
// estraneo in una cartella verrebbe caricato in silenzio e il dataset non ha Git. Dichiara anche i limiti,
// con la loro versione (R43 B), e il file dei casi, che può mancare: il dato arriva in A1c, il formato
// dell'indice non cambia.
type IndiceRegole struct {
	VersioneIndice int          `json:"versione_indice"`
	Limiti         Limiti       `json:"limiti"` // obbligatorio: LeggiIndice lo controlla con Limiti.Valida (R43 B)
	Grammatiche    []VoceIndice `json:"grammatiche"`
	Casi           *FileIndice  `json:"casi,omitempty"` // il file degli ingressi dei casi (A1c; R48 A)
}

// VoceIndice: il legame di un cliente con il suo file di regole. Il file dichiara a sua volta il suo
// cliente: i due UUID devono coincidere (lo controlla chi compila l'insieme delle regole).
type VoceIndice struct {
	ClienteID uuid.UUID `json:"cliente_id"`
	File      string    `json:"file"`   // relativo alla cartella dell'indice
	Sha256    string    `json:"sha256"` // del file com'è sul disco
}

// FileIndice: un file citato dall'indice, con il suo sha256.
type FileIndice struct {
	File   string `json:"file"` // relativo alla cartella dell'indice
	Sha256 string `json:"sha256"`
}

// LeggiIndice: la decodifica stretta dell'indice, con le stesse regole di Decodifica (versione_indice al
// posto di versione_schema). Poi controlla:
//   - i limiti, con Limiti.Valida: assenti, nulli o oltre i tetti (contratto.campo_obbligatorio,
//     limite.oltre_tetto);
//   - le voci: UUID, file e sha256 presenti (contratto.campo_obbligatorio), nessun cliente ripetuto
//     (regole.cliente_ripetuto);
//   - il file dei casi, se c'è: file e sha256 presenti.
//
// Ogni problema è un *evidenze.ErroreContratto, con tutte le diagnostiche: con un indice così non si attiva
// nessuna grammatica (R43 B). Non legge file: gli sha256 li confronta chi legge le grammatiche.
func LeggiIndice(raw []byte) (IndiceRegole, error) {
	var ix IndiceRegole
	if d := decodificaStretta(raw, &ix, "versione_indice", VersioneIndice); len(d) > 0 {
		return IndiceRegole{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	out := ix.Limiti.Valida()
	var clienti []uuid.UUID
	for i, v := range ix.Grammatiche {
		p := fmt.Sprintf("grammatiche[%d]", i)
		if v.ClienteID == uuid.Nil {
			out = append(out, evidenze.Diagnostica{
				Codice:    CodiceCampoObbligatorio,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  p + ".cliente_id",
				Messaggio: "manca l'UUID del cliente: il legame è per UUID, mai per nome",
			})
		} else {
			for _, c := range clienti {
				if c == v.ClienteID {
					out = append(out, evidenze.Diagnostica{
						Codice:    CodiceRegoleClienteRipetuto,
						Gravita:   evidenze.GravitaErrore,
						Natura:    evidenze.NaturaContratto,
						Percorso:  p + ".cliente_id",
						Messaggio: "il cliente compare due volte nell'indice: quale grammatica valga non si decide",
						Rif:       []string{v.ClienteID.String()},
					})
					break
				}
			}
			clienti = append(clienti, v.ClienteID)
		}
		out = append(out, fileObbligatorio(p, v.File, v.Sha256)...)
	}
	if ix.Casi != nil {
		out = append(out, fileObbligatorio("casi", ix.Casi.File, ix.Casi.Sha256)...)
	}
	if len(out) > 0 {
		return IndiceRegole{}, &evidenze.ErroreContratto{Diagnostiche: out}
	}
	return ix, nil
}

func fileObbligatorio(p, file, sha string) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	if strings.TrimSpace(file) == "" {
		out = append(out, evidenze.Diagnostica{
			Codice:    CodiceCampoObbligatorio,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p + ".file",
			Messaggio: "manca il nome del file, relativo alla cartella dell'indice",
		})
	}
	if strings.TrimSpace(sha) == "" {
		out = append(out, evidenze.Diagnostica{
			Codice:    CodiceCampoObbligatorio,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p + ".sha256",
			Messaggio: "manca lo sha256 del file: senza, un file cambiato passerebbe in silenzio",
		})
	}
	return out
}

// ControllaRagioneSociale confronta la ragione sociale della grammatica con quella del DB: spazi ai bordi e
// maiuscole ASCII non contano, tutto il resto sì. Se non coincidono restituisce una diagnosi
// (regole.ragione_sociale_discorde, avviso, dati): il thread non si valuta (R29 d; par.3.6.3). È un
// controllo, non una chiave: il cliente si cerca per UUID. Prende un UUID copiato sbagliato e un DB ricreato
// con UUID nuovi. Il messaggio non riporta le ragioni sociali: può finire nel log.
func ControllaRagioneSociale(g Grammatica, delDB string) *evidenze.Diagnostica {
	if ugualiASCII(strings.TrimSpace(g.Cliente.RagioneSociale), strings.TrimSpace(delDB)) {
		return nil
	}
	return &evidenze.Diagnostica{
		Codice:    CodiceRegoleRagioneSocialeDiscorde,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  "cliente.ragione_sociale",
		Messaggio: "la ragione sociale della grammatica non coincide con quella del DB (spazi ai bordi e maiuscole ASCII non contano): il thread non si valuta",
		Rif:       []string{g.Cliente.ID.String()},
	}
}

// ugualiASCII: le due stringhe coincidono a meno delle maiuscole ASCII. Le lettere fuori dall'ASCII devono
// coincidere: strings.EqualFold piegherebbe anche quelle.
func ugualiASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
