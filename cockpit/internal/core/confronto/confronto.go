// Package confronto mette a fianco il «vecchio» (le proposte e le decisioni di adesso, con i codici già letti dalla
// grammatica), il «nuovo» (il motore A) e, solo nel banco, l'«atteso» (l'oracolo privato), per i file di un thread
// (piano A, par.3.3.8 e 6.4.6; R3: «DTO e logica di confronto … Riceve DTO»; R42 B, R53 B). Calcola il badge e
// l'indicatore di revisione di ogni file, i conteggi per badge, le correzioni manuali prima e dopo e, con l'atteso, gli
// esiti per file e per prodotto.
//
// Riceve DTO propri e piatti (File, Vecchio, Nuovo, Candidato, ProdottoNuovo), che chi chiama copia campo per campo dai
// record piatti di valutazione: i due pacchetti non si importano. Non importa i pacchetti del motore, la fotografia, il
// caricatore né una libreria YAML: delle foglie solo la diagnostica (evidenze) e l'impronta (jsoncanonico). L'atteso lo
// traduce il runner (app/bancoa) nei DTO di questo pacchetto; l'anteprima non ce l'ha. È puro: nessun DB, file,
// orologio o rete.
package confronto

import (
	"bytes"
	"sort"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// VersioneConfronto: la versione delle regole di questo pacchetto (badge, indicatore, correzioni, esiti). Entra
// nell'impronta dell'esito: cambiarla cambia l'impronta. confronto-2: la misura delle correzioni manuali con la
// definizione di R114 (denominatore, esclusi, marcatore a parte, «dopo» diviso) e il campo CodiceLettoMarcatore nel
// vecchio.
const VersioneConfronto = "confronto-2"

// Esito: il confronto dei file di un thread. Il thread lo conosce chi chiama, che mette l'esito accanto a quello di
// valutazione; A2 salverà i due (par.8).
//   - File: una riga per allegato, in ordine di AllegatoID, con il badge e l'indicatore di revisione.
//   - Conteggi: i file per badge, con tutti e cinque i badge.
//   - Correzioni: la misura prima e dopo sui file decisi (R30 f), con la definizione di R114: lo stesso campione per
//     prima e dopo, il denominatore e gli esclusi per motivo.
//   - ControAtteso, ProdottiControAtteso: solo con l'atteso, cioè nel banco.
//   - Diagnostiche: quelle dell'indicatore di revisione, in ordine di (codice, percorso, riferimenti, messaggio).
//   - Impronta: sha256 del canonico dell'esito senza le parti contro l'atteso e con Impronta vuota: la stessa nel banco
//     e nell'anteprima, per gli stessi file. Nessun orario dentro. Vuota vuol dire «non calcolata»: l'esito non ha
//     un canonico (una stringa non UTF-8 o un tempo fuori dall'intervallo di JSON negli ingressi). Due impronte vuote
//     non dicono che due esiti coincidono: chi chiama la tratta come un errore, e il banco esce con 1 (R-48).
type Esito struct {
	VersioneConfronto    string                 `json:"versione_confronto"`
	File                 []EsitoFile            `json:"file,omitempty"`
	Conteggi             map[Badge]int          `json:"conteggi"`
	Correzioni           CorrezioniManuali      `json:"correzioni"`
	ControAtteso         []EsitoFileAtteso      `json:"contro_atteso,omitempty"`
	ProdottiControAtteso []EsitoProdottoAtteso  `json:"prodotti_contro_atteso,omitempty"`
	Diagnostiche         []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
	Impronta             string                 `json:"impronta"`
}

// EsitoFile: una riga per allegato: il file com'è arrivato, il badge con il suo motivo, l'indicatore di revisione.
type EsitoFile struct {
	File      File                `json:"file"`
	Badge     Badge               `json:"badge"`
	Motivo    string              `json:"motivo,omitempty"`
	Revisione IndicatoreRevisione `json:"revisione"`
}

// Confronta affianca vecchio e nuovo di ogni file del thread e, se c'è, l'atteso del thread (sequenza del 6.4.6):
//  1. per ogni file, in ordine di AllegatoID, il badge e l'indicatore di revisione, dal vecchio e dal nuovo piatti;
//  2. i conteggi per badge e le correzioni manuali sui file decisi;
//  3. con l'atteso, solo nel banco: ConfrontaConAtteso per i file e ConfrontaProdotti per i prodotti;
//  4. l'impronta.
//
// Badge e indicatore non dipendono dall'atteso: nel banco e nell'anteprima sono gli stessi, e così l'impronta. Con
// atteso nil (l'anteprima) gli esiti contro l'atteso restano vuoti, e prodotti non serve. È pura e non riceve mai la
// fotografia; lo stesso ingresso, comunque permutato, dà lo stesso esito.
func Confronta(file []File, prodotti []ProdottoNuovo, atteso *Atteso) Esito {
	e := Esito{VersioneConfronto: VersioneConfronto, Conteggi: map[Badge]int{}}
	for _, b := range tuttiIBadge {
		e.Conteggi[b] = 0
	}
	for _, f := range ordinaFile(file) {
		badge, motivo := badgeDi(f)
		ind, d := indicatoreDi(f)
		e.File = append(e.File, EsitoFile{File: f, Badge: badge, Motivo: motivo, Revisione: ind})
		e.Conteggi[badge]++
		if d != nil {
			e.Diagnostiche = append(e.Diagnostiche, *d)
		}
	}
	e.Correzioni = correzioniDi(e.File)
	sort.SliceStable(e.Diagnostiche, func(i, j int) bool {
		return chiaveDiagnostica(e.Diagnostiche[i]) < chiaveDiagnostica(e.Diagnostiche[j])
	})
	e.Impronta = impronta(e)
	if atteso != nil {
		e.ControAtteso = ConfrontaConAtteso(file, *atteso)
		e.ProdottiControAtteso = ConfrontaProdotti(prodotti, atteso.Prodotti)
	}
	return e
}

// impronta: lo sha256 del canonico di un valore. Per l'esito, senza le parti contro l'atteso e con Impronta vuota
// (si chiama prima di riempirle). "" se il valore non ha un canonico.
func impronta(v any) string {
	if e, ok := v.(Esito); ok {
		e.Impronta, e.ControAtteso, e.ProdottiControAtteso = "", nil, nil
		v = e
	}
	h, err := jsoncanonico.ImprontaDi(v)
	if err != nil {
		return ""
	}
	return h
}

// ordinaFile: i file in ordine di AllegatoID; due righe con lo stesso allegato (un ingresso fuori contratto) in ordine
// d'impronta, perché l'ordine non dipenda da quello d'ingresso.
func ordinaFile(file []File) []File {
	out := append([]File(nil), file...)
	chiavi := make([]string, len(out))
	indici := make([]int, len(out))
	for i := range out {
		indici[i] = i
		chiavi[i] = impronta(out[i])
	}
	sort.SliceStable(indici, func(x, y int) bool {
		a, b := out[indici[x]], out[indici[y]]
		if c := bytes.Compare(a.AllegatoID[:], b.AllegatoID[:]); c != 0 {
			return c < 0
		}
		return chiavi[indici[x]] < chiavi[indici[y]]
	})
	ordinati := make([]File, len(out))
	for i, j := range indici {
		ordinati[i] = out[j]
	}
	return ordinati
}

// chiaveDiagnostica: l'ordine delle diagnostiche, come negli altri pacchetti del motore A.
func chiaveDiagnostica(d evidenze.Diagnostica) string {
	return d.Codice + "\x00" + d.Percorso + "\x00" + strings.Join(d.Rif, "\x01") + "\x00" + d.Messaggio
}
