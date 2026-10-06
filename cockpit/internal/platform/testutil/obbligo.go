package testutil

import (
	"os"
	"strings"
	"testing"
)

// variabileObbligatorie: l'elenco delle risorse d'ambiente che, in una corsa che chiude una sessione o un gate,
// non si possono saltare (per esempio «L4,PYTHON»).
const variabileObbligatorie = "COCKPIT_PROVE_OBBLIGATORIE"

// Richiesto dichiara che una risorsa d'ambiente (il DB di prova, Python, Playwright) manca, e decide che cosa ne
// è della prova (piano A, 3.3.9, 3.7.3 e 6.4.7; R44): nelle corse di sviluppo la salta con
// «SALTATO-AMBIENTE: <risorsa>: <motivo>»; se COCKPIT_PROVE_OBBLIGATORIE la dichiara obbligatoria, la prova è
// NON ESEGUITA, mai saltata e mai verde. Chi la chiama l'ha già trovata mancante.
func Richiesto(t testing.TB, risorsa, motivo string) {
	t.Helper()
	if obbligatoria(risorsa, os.Getenv(variabileObbligatorie)) {
		NonEseguita(t, risorsa+": "+motivo+" ("+variabileObbligatorie+" la dichiara obbligatoria)")
		return
	}
	t.Skipf("SALTATO-AMBIENTE: %s: %s", risorsa, motivo)
}

// obbligatoria: la risorsa sta nell'elenco separato da virgole, senza badare a spazi e maiuscole.
func obbligatoria(risorsa, elenco string) bool {
	r := strings.TrimSpace(risorsa)
	if r == "" {
		return false
	}
	for _, voce := range strings.Split(elenco, ",") {
		if strings.EqualFold(strings.TrimSpace(voce), r) {
			return true
		}
	}
	return false
}
