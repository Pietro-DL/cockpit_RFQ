package grammatica

import (
	"fmt"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// stessa_parte_di (KN-01, decisione dell'utente del 22/10 sulla domanda RK-02: «integrare il nome del file nelle
// equivalenze e proporlo in corrispondenza al codice»): un campo facoltativo di una forma, che dice una relazione fra
// due letture dello STESSO nome di file. La lettura di questa forma è un documento della stessa parte che il nome dice
// con una lettura d'identità di una delle famiglie elencate (per esempio il documento 3D o 2D accanto al codice della
// parte). Il motore ne propone l'equivalenza fra le due letture (ancoraggio, equivalenze.go), come per il cartiglio
// che scrive il codice della parte e il riferimento 3D: mai un'identità del file, mai un namespace che si confronta.
//
// Il motore non sa niente di un cliente: quali nomi dicono la relazione (le parti della forma: etichette, separatori,
// token) e quali famiglie danno il codice della parte stanno tutti nella grammatica. Senza il campo, il canonico e
// l'hash di una grammatica sono quelli di prima (omitempty, come i campi di GP-1).
//
// Le regole, controllate da Valida:
//   - solo su una forma che legge soltanto nome_file: la relazione è fra due letture dello stesso nome;
//   - un elenco di ID di famiglie della stessa grammatica, senza vuoti né ripetizioni;
//   - mai la famiglia della forma, mai una famiglia dello stesso namespace: due letture dello stesso namespace sono
//     due identità confrontabili, non un'equivalenza.

// stessaParte controlla il campo stessa_parte_di di una forma della famiglia f (KN-01).
func (v *validatore) stessaParte(fo FormaCodice, f FamigliaCodice, p string) {
	if len(fo.StessaParteDi) == 0 {
		return
	}
	ps := p + ".stessa_parte_di"
	for _, s := range fo.Selettori {
		if s != string(evidenze.ContestoNomeFile) {
			v.nonAmmesso(ps, fmt.Sprintf("la relazione della stessa parte vale solo fra due letture di un nome di file, non su %q", s))
			break
		}
	}
	v.duplicati(ps, fo.StessaParteDi)
	for i, id := range fo.StessaParteDi {
		pi := fmt.Sprintf("%s[%d]", ps, i)
		if strings.TrimSpace(id) == "" {
			v.obbligatorio(pi, "l'ID della famiglia del codice della parte")
			continue
		}
		if id == f.ID {
			v.nonAmmesso(pi, fmt.Sprintf("la famiglia %q è quella della forma: una lettura non è la stessa parte di sé stessa", id))
			continue
		}
		g, ok := v.famiglie[id]
		if !ok {
			v.errore(evidenze.Diagnostica{Codice: CodiceRiferimentoPendente, Percorso: pi,
				Messaggio: fmt.Sprintf("la famiglia %q non c'è nella grammatica", id), Rif: []string{id}})
			continue
		}
		if g.Namespace == f.Namespace {
			v.nonAmmesso(pi, fmt.Sprintf("la famiglia %q ha lo stesso namespace %q della forma: due letture dello stesso namespace si confrontano, non sono un'equivalenza", id, g.Namespace))
		}
	}
}
