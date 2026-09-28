package documenti

import (
	"fmt"
	"path/filepath"
	"strings"

	"promatec/cockpit/internal/platform/storage/nas"
)

// DOVE STA IL FILE DI UN DOCUMENTO, E PERCHE' LO SI RICAVA UNA VOLTA SOLA.
//
// Il percorso di un documento non e' un dato: e' il risultato di una composizione — la radice del
// NAS di QUESTO server, la cartella della RFQ, il percorso relativo del documento. Nessuno dei tre
// pezzi arriva mai da una richiesta HTTP, e nella URL dell'anteprima non c'e' nessun parametro di
// percorso da ripulire, perche' non c'e' nessun parametro di percorso.
//
// Il controllo qui sotto non serve quindi a fermare un utente — che non ha modo di scrivere in questi
// campi — ma a fermare un BUG o una riga cambiata a mano nel database: `..\..\..\Windows\win.ini` in
// `path_relativo` e' una riga che nessuna rotta puo' scrivere, e proprio per questo nessuno la
// cercherebbe mai. Un percorso che esce dalla radice non e' un caso d'uso: chi lo riceve risponde 500
// e lo scrive nel log.
//
// Il prefisso long-path (`\\?\`) lo mette `nas.UNC` DOPO il controllo: aggiunto prima, il confronto
// fra il percorso e la radice avverrebbe fra due stringhe di forma diversa.

// Percorso sono i due modi di nominare lo stesso file, e servono tutti e due: il relativo si mostra
// e si scrive nelle anomalie (una riga letta da un'altra postazione non deve contenere la radice di
// questa), l'assoluto si apre.
type Percorso struct {
	Relativo string
	Assoluto string
}

// PercorsoSulNas compone il percorso e verifica che stia dentro la radice.
func PercorsoSulNas(radice, cartellaThread, pathRelativo string) (Percorso, error) {
	rel := percorsoDocumento(cartellaThread, pathRelativo)
	if strings.TrimSpace(rel) == "" {
		return Percorso{}, fmt.Errorf("documento senza percorso relativo")
	}
	if err := relativoSicuro(rel); err != nil {
		return Percorso{}, err
	}
	base := filepath.Clean(strings.TrimRight(radice, `\/`))
	assoluto := filepath.Clean(filepath.Join(base, filepath.FromSlash(rel)))
	if !dentroLaRadice(assoluto, base) {
		return Percorso{}, fmt.Errorf("percorso fuori radice: %q non sta sotto %q", assoluto, base)
	}
	return Percorso{Relativo: rel, Assoluto: nas.UNC(radice, rel)}, nil
}

// relativoSicuro rifiuta cio' che non puo' essere un percorso relativo dentro la radice. Sono le
// forme che `NomeSicuro` gia' esclude quando il percorso viene SCRITTO: il controllo c'e' per non
// presumerlo, perche' fra la scrittura e questa lettura possono passare anni e una migrazione.
//
// Si guarda PEZZO PER PEZZO, e non la stringa intera, per due ragioni imparate da una prova:
//
//   - `..` dev'essere un pezzo del percorso, non una sottostringa. `rev..vecchia.pdf` e' un nome di
//     file legittimo, e rifiutarlo vorrebbe dire non mostrare un disegno che c'e';
//   - un pezzo VUOTO e' cio' che resta di un percorso che comincia dalla radice del disco dopo che
//     gli e' stata incollata davanti la cartella della RFQ. `\Windows\win.ini` in `path_relativo`
//     diventa `ACME\WIP\...\\Windows\win.ini`: non comincia piu' per barra, `filepath.Join`
//     ricompatta le due barre, e il risultato sta dentro la radice per puro caso. Non e' un percorso
//     che qualcuno ha scritto: e' un errore, e va fermato dove si vede.
func relativoSicuro(rel string) error {
	if strings.ContainsRune(rel, 0) {
		return fmt.Errorf("percorso fuori radice: contiene un byte zero")
	}
	if strings.Contains(rel, ":") {
		return fmt.Errorf("percorso fuori radice: contiene una lettera di unita' o un flusso (%q)", rel)
	}
	for _, pezzo := range strings.Split(strings.ReplaceAll(rel, "/", `\`), `\`) {
		switch {
		case strings.TrimSpace(pezzo) == "":
			return fmt.Errorf("percorso fuori radice: un pezzo del percorso e' vuoto — comincia dalla "+
				"radice del disco, oppure ha due separatori di fila (%q)", rel)
		case pezzo == "..":
			return fmt.Errorf("percorso fuori radice: contiene un pezzo «..» (%q)", rel)
		case pezzo == ".":
			return fmt.Errorf("percorso fuori radice: contiene un pezzo «.» (%q)", rel)
		}
	}
	return nil
}

// dentroLaRadice confronta senza distinguere maiuscole e minuscole, perche' NTFS non le distingue: un
// confronto sensibile direbbe che `C:\NAS\...` non sta dentro `C:\nas`, e sul disco invece ci sta.
func dentroLaRadice(assoluto, base string) bool {
	if strings.EqualFold(assoluto, base) {
		return false // la radice stessa non e' un documento
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(strings.ToLower(assoluto), strings.ToLower(base)+sep)
}

// DentroLaRadice e' la stessa domanda che si fa `PercorsoSulNas` — «questo percorso assoluto sta
// davvero sotto quella cartella?» — posta da chi ha gia' il percorso in mano.
//
// Esiste perche' il NAS non e' l'unica radice del Cockpit: anche `allegato.path_staging` e' un
// percorso assoluto letto dal database, e l'anteprima lo apre. Vale parola per parola la ragione
// scritta in cima a questo file: non lo puo' comporre nessuno da fuori, e proprio per questo nessuno
// andrebbe a guardarlo. Una regola sola per le due radici, perche' due regole diventano due
// comportamenti, e il secondo se ne accorge qualcuno solo il giorno in cui serve.
//
// `assoluto` e `base` si passano gia' puliti (`filepath.Clean`, senza barra finale): qui si confronta
// e basta, non si normalizza — normalizzare in due posti vuol dire normalizzare in due modi.
func DentroLaRadice(assoluto, base string) bool { return dentroLaRadice(assoluto, base) }
