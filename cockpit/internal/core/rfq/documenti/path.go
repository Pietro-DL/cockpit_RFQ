// Package documenti e' il fascicolo di una RFQ sul NAS: come si chiamano le cartelle e i file, che
// cosa significa copiare un documento e riprenderne il contenuto, e se quello che il database
// promette scritto ci sia davvero.
//
// Non decide QUANDO si esegue un lavoro: il giro dei job, il claim e i tentativi sono di `app/runtime`,
// che chiama queste funzioni. Della coda usa l'accodamento, dove un gesto sul fascicolo ha bisogno di un
// lavoro dopo di se' — la ripresa di un contenuto sparito (riestrazione o download da Outlook,
// ripresa.go), lo spostamento di un file (sposta_nas, nomi_nas.go) — e legge i job di copia pendenti,
// perche' il ricognitore non segnali come anomalia un lavoro gia' in corso (integrita.go).
package documenti

import (
	"errors"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// Convenzione NAS (RFQ_plan §0.2.1.4): <Cliente.cartella_nas>\WIP\<aaaa mm gg> <Cognome buyer> <Oggetto>
// e sotto, per tipo documento, la sottocartella di cartella_documento (es. ELENCO DISEGNI\<codice>\).
// Tutti i percorsi restituiti sono RELATIVI alla radice NAS e usano '\' come separatore.

var reVietati = regexp.MustCompile(`[<>:"/\|?*\x00-\x1f]`)
var reSpazi = regexp.MustCompile(`\s+`)

// NomeSicuro rende una stringa utilizzabile come nome di cartella/file Windows: rimuove i caratteri vietati,
// comprime gli spazi, toglie punti e spazi finali (vietati da NTFS) e tronca a max caratteri.
//
// Punti e spazi finali si tolgono anche DOPO il taglio: «Offerta rev. 2» tagliato a 12 finisce con il
// punto di «rev.». Windows quel punto lo toglie da solo, e la cartella creata si chiama in un modo mentre
// il database la ricorda in un altro; con il prefisso \\?\ invece il punto resta, e nasce una cartella che
// Esplora risorse non apre. E' quello che fa gia' NomeFileSicuro: taglia, poi ripassa senzaCoda.
func NomeSicuro(s string, max int) string {
	s = pulisci(s)
	if max > 0 {
		s = senzaCoda(taglia(s, max))
	}
	if s == "" {
		s = "senza nome"
	}
	return s
}

// pulisci e' la parte di NomeSicuro che non taglia: via i caratteri vietati, spazi compressi, via
// punti e spazi finali.
func pulisci(s string) string {
	s = reVietati.ReplaceAllString(s, " ")
	s = reSpazi.ReplaceAllString(strings.TrimSpace(s), " ")
	return senzaCoda(s)
}

// senzaCoda toglie i punti e gli spazi finali, che NTFS non accetta.
func senzaCoda(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool { return r == '.' || unicode.IsSpace(r) })
}

// CartellaThread costruisce il percorso relativo della cartella RFQ.
// Es.: "ACME\WIP\2026 09 08 Rossi Supporto cofano"
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
// non e' la stessa cosa: un nome salvato prima di B8.A4-0, o scritto a mano, puo' non essere quello
// che NomeFileSicuro ne farebbe oggi (un'estensione maiuscola, «senza nome» in coda).
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

const (
	maxNomeFile   = 150 // il nome senza l'estensione
	maxEstensione = 10  // l'estensione, punto compreso
)

// NomeFileSicuro conserva l'estensione, in minuscolo, e sanifica il resto.
//
// Ripassata sul nome che ha prodotto, lo restituisce uguale (addendum A4.1, R1.7): chi ricalcola un
// percorso da un nome gia' sanificato non lo cambia. Per questo:
//   - un nome senza estensione resta senza: prima riceveva «senza nome» in coda a ogni passata;
//   - l'estensione si cerca nel nome gia' pulito: pulire toglie i punti finali e le barre, e cosi'
//     «a.B.» e «a.B/c» hanno un'estensione dopo e non prima;
//   - un'estensione e' corta e senza spazi: il punto di «Offerta 12.03.2026 rev finale» non ne apre
//     una, e il nome resta intero invece di perdere «finale»;
//   - se il taglio di un nome lungo lascia in coda un punto e poche lettere, quella e' l'estensione
//     della passata dopo, e quindi e' gia' l'estensione di questa.
func NomeFileSicuro(nome string) string {
	s := pulisci(nome)
	ext := estensione(s)
	if ext == "" {
		s = senzaCoda(taglia(s, maxNomeFile))
		ext = estensione(s)
	}
	base := senzaCoda(taglia(strings.TrimSuffix(s, ext), maxNomeFile))
	if base == "" {
		base = "senza nome"
	}
	return base + strings.ToLower(ext)
}

// estensione e' la parte di un nome pulito dall'ultimo punto in poi, se non supera maxEstensione
// caratteri e non ha spazi; altrimenti "". Nel nome pulito non ci sono barre ne' un punto finale:
// l'estensione, se c'e', ha almeno un carattere dopo il punto.
func estensione(s string) string {
	ext := path.Ext(s)
	if utf8.RuneCountInString(ext) > maxEstensione || strings.IndexFunc(ext, unicode.IsSpace) >= 0 {
		return ""
	}
	return ext
}

// taglia tiene i primi max caratteri.
func taglia(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}
