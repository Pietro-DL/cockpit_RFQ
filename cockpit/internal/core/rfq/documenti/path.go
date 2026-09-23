// Package documenti e' il fascicolo di una RFQ sul NAS: come si chiamano le cartelle e i file, che
// cosa significa copiare un documento e riprenderne il contenuto, e se quello che il database
// promette scritto ci sia davvero.
//
// Non sa niente di code e di job: chi esegue e quando e' `app/runtime`, che chiama queste funzioni.
package documenti

import (
	"errors"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// Convenzione NAS (RFQ_plan §0.2.1.4): <Cliente.cartella_nas>\WIP\<aaaa mm gg> <Cognome buyer> <Oggetto>
// e sotto, per tipo documento, la sottocartella di cartella_documento (es. ELENCO DISEGNI\<codice>\).
// Tutti i percorsi restituiti sono RELATIVI alla radice NAS e usano '\' come separatore.

var reVietati = regexp.MustCompile(`[<>:"/\|?*\x00-\x1f]`)
var reSpazi = regexp.MustCompile(`\s+`)

// NomeSicuro rende una stringa utilizzabile come nome di cartella/file Windows: rimuove i caratteri vietati,
// comprime gli spazi, toglie punti e spazi finali (vietati da NTFS) e tronca a max caratteri.
func NomeSicuro(s string, max int) string {
	s = reVietati.ReplaceAllString(s, " ")
	s = reSpazi.ReplaceAllString(strings.TrimSpace(s), " ")
	s = strings.TrimRightFunc(s, func(r rune) bool { return r == '.' || unicode.IsSpace(r) })
	if max > 0 && len([]rune(s)) > max {
		s = strings.TrimSpace(string([]rune(s)[:max]))
	}
	if s == "" {
		s = "senza nome"
	}
	return s
}

// CartellaThread costruisce il percorso relativo della cartella RFQ.
// Es.: "LANDINI ARGO\WIP\2026 09 08 Rossi Supporto cofano"
func CartellaThread(cartellaCliente string, data time.Time, cognomeBuyer, oggetto string) string {
	parti := []string{data.Format("2006 01 02")}
	if c := NomeSicuro(cognomeBuyer, 30); cognomeBuyer != "" && c != "senza nome" {
		parti = append(parti, c)
	}
	parti = append(parti, NomeSicuro(classificazione.OggettoPulito(oggetto), 60))
	return strings.Join([]string{NomeSicuro(cartellaCliente, 80), "WIP", strings.Join(parti, " ")}, `\`)
}

// LayoutDocumento è la riga di cartella_documento per il tipo.
type LayoutDocumento struct {
	Sottocartella string
	PerCodice     bool
}

// ErrCodiceMancante: il layout del tipo vuole la cartella del codice, e il codice non c'e'.
var ErrCodiceMancante = errors.New("questo tipo di documento va nella cartella del suo codice, e il codice manca")

// PathDocumento restituisce il percorso del file RELATIVO alla cartella del thread.
// cartellaPerCodice è Cliente.regole.cartella_per_codice (default true, SPEC §8: "proposta: sempre").
//
// Se il layout vuole la cartella del codice e il codice manca, restituisce ErrCodiceMancante invece
// di mettere il file nella sottocartella comune (addendum A2.2): un disegno senza codice non ha un
// posto nel fascicolo, e resta fra le proposte finche' qualcuno non glielo da'.
func PathDocumento(l LayoutDocumento, cartellaPerCodice bool, codice, nomeFile string) (string, error) {
	cartella, err := CartellaDocumento(l, cartellaPerCodice, codice)
	if err != nil {
		return "", err
	}
	return NellaCartella(cartella, NomeFileSicuro(nomeFile)), nil
}

// CartellaDocumento e' la parte di PathDocumento che dipende dal tipo e dal codice: la cartella, senza
// il nome del file ("" = il file sta direttamente nella cartella della RFQ).
//
// La correzione di un codice (B8.3, addendum A1.4) cambia questa e non il nome: un file non si
// rinomina perche' il suo codice era sbagliato. Ricalcolare tutto il percorso dal nome gia' salvato
// non e' la stessa cosa: NomeFileSicuro non restituisce sempre lo stesso nome se applicata due volte
// (un nome senza estensione riceve «senza nome» in coda a ogni passata).
func CartellaDocumento(l LayoutDocumento, cartellaPerCodice bool, codice string) (string, error) {
	var parti []string
	if l.Sottocartella != "" {
		parti = append(parti, NomeSicuro(l.Sottocartella, 80))
	}
	if l.PerCodice && cartellaPerCodice {
		if strings.TrimSpace(codice) == "" {
			return "", ErrCodiceMancante
		}
		parti = append(parti, NomeSicuro(strings.ToUpper(codice), 60))
	}
	return strings.Join(parti, `\`), nil
}

// NellaCartella mette un nome di file in una cartella relativa ("" = nessuna cartella).
func NellaCartella(cartella, nome string) string {
	if cartella == "" {
		return nome
	}
	return cartella + `\` + nome
}

// NomeNelPercorso e' il nome del file in un path_relativo: quello che c'e' dopo l'ultima `\`.
func NomeNelPercorso(pathRelativo string) string {
	return pathRelativo[strings.LastIndex(pathRelativo, `\`)+1:]
}

// NomeFileSicuro conserva l'estensione e sanifica il resto.
func NomeFileSicuro(nome string) string {
	ext := path.Ext(nome)
	base := strings.TrimSuffix(nome, ext)
	return NomeSicuro(base, 150) + strings.ToLower(NomeSicuro(ext, 10))
}
