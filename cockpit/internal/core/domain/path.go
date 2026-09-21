package domain

import (
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Convenzione NAS (RFQ_plan §0.2.1.4): <Cliente.cartella_nas>\WIP\<aaaa mm gg> <Cognome buyer> <Oggetto>
// e sotto, per tipo documento, la sottocartella di cartella_documento (es. ELENCO DISEGNI\<codice>\).
// Tutti i percorsi restituiti sono RELATIVI alla radice NAS e usano '\' come separatore.

var reVietati = regexp.MustCompile(`[<>:"/\|?*\x00-\x1f]`)
var rePrefissi = regexp.MustCompile(`(?i)^((re|r|fw|fwd|i|tr|aw|wg)\s*:\s*)+`)
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

// OggettoPulito toglie i prefissi RE:/FW:/I: ripetuti dall'oggetto della mail.
func OggettoPulito(oggetto string) string {
	return strings.TrimSpace(rePrefissi.ReplaceAllString(strings.TrimSpace(oggetto), ""))
}

// CartellaThread costruisce il percorso relativo della cartella RFQ.
// Es.: "LANDINI ARGO\WIP\2026 09 08 Rossi Supporto cofano"
func CartellaThread(cartellaCliente string, data time.Time, cognomeBuyer, oggetto string) string {
	parti := []string{data.Format("2006 01 02")}
	if c := NomeSicuro(cognomeBuyer, 30); cognomeBuyer != "" && c != "senza nome" {
		parti = append(parti, c)
	}
	parti = append(parti, NomeSicuro(OggettoPulito(oggetto), 60))
	return strings.Join([]string{NomeSicuro(cartellaCliente, 80), "WIP", strings.Join(parti, " ")}, `\`)
}

// LayoutDocumento è la riga di cartella_documento per il tipo.
type LayoutDocumento struct {
	Sottocartella string
	PerCodice     bool
}

// PathDocumento restituisce il percorso del file RELATIVO alla cartella del thread.
// cartellaPerCodice è Cliente.regole.cartella_per_codice (default true, SPEC §8: "proposta: sempre").
func PathDocumento(l LayoutDocumento, cartellaPerCodice bool, codice, nomeFile string) string {
	var parti []string
	if l.Sottocartella != "" {
		parti = append(parti, NomeSicuro(l.Sottocartella, 80))
	}
	if l.PerCodice && cartellaPerCodice && strings.TrimSpace(codice) != "" {
		parti = append(parti, NomeSicuro(strings.ToUpper(codice), 60))
	}
	parti = append(parti, NomeFileSicuro(nomeFile))
	return strings.Join(parti, `\`)
}

// NomeFileSicuro conserva l'estensione e sanifica il resto.
func NomeFileSicuro(nome string) string {
	ext := path.Ext(nome)
	base := strings.TrimSuffix(nome, ext)
	return NomeSicuro(base, 150) + strings.ToLower(NomeSicuro(ext, 10))
}
