package fotorfq

// impronta.go: l'impronta della fotografia (piano A, A1c, 6.4.1 e 3.4.2). Due letture della stessa copia danno la
// stessa impronta: per questo restano fuori Sorgente (da dove si è letto) e PresaIl (quando), e gli elenchi si
// mettono nell'ordine totale di Ordina prima di codificare.

import (
	"encoding/json"
	"fmt"
	"time"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// ImprontaFotografia: lo sha256 del JSON canonico (jsoncanonico, con i tag dei tipi) di tutto tranne PresaIl e
// Sorgente, con gli elenchi nell'ordine di Ordina. Non tocca la fotografia di chi chiama: ordina una copia, fatta
// passando per il JSON dei tipi, che porta tutti i campi. Un payload dei fatti che non è JSON valido è un errore:
// un'impronta non si inventa.
func ImprontaFotografia(f Fotografia) (string, error) {
	f.Sorgente = ""
	f.PresaIl = time.Time{}
	raw, err := json.Marshal(f)
	if err != nil {
		return "", fmt.Errorf("fotorfq: impronta della fotografia: %w", err)
	}
	var copia Fotografia
	if err := json.Unmarshal(raw, &copia); err != nil {
		return "", fmt.Errorf("fotorfq: impronta della fotografia: %w", err)
	}
	copia.Ordina()
	h, err := jsoncanonico.ImprontaDi(copia)
	if err != nil {
		return "", fmt.Errorf("fotorfq: impronta della fotografia: %w", err)
	}
	return h, nil
}
