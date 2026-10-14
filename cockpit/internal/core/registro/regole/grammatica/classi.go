package grammatica

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// La tabella delle classi di una famiglia (decisione dell'utente del 14/10, provenienza [U]): un codice della famiglia
// si classifica appena si legge, guardando il valore di un suo segmento identitario. È una mappa dichiarata nel file
// regole del cliente: valore del segmento → (categoria del pezzo, descrizione, fonte). Per esempio, in una famiglia
// ACME inventata 9.SSSS.AAA.T con il segmento «serie», la voce «4019» dice «commerciale · Molla a compressione ·
// norma ACME 94019». Nessun valore sta nel codice: la tabella è un dato del cliente, come le forme (P-18).
//
// Sostituisce, quando c'è, la strada di prima (una famiglia per serie con le sue Categorie): una famiglia con la
// tabella non dichiara anche Categorie (sarebbe ambiguo quale valga: contratto.classi_con_categorie). Il motore la
// applica in Riconosci e Interpreta (motorea: LetturaForma.Classe e l'unione delle categorie); la valutazione ne fa
// una proposta, mai un'esenzione dal 2D (R103 C, R99 A).
//
// Il campo è facoltativo e inerte quando manca: una grammatica senza tabella ha il canonico e l'hash di prima, e
// nessuna versione cambia (VersioneSchema resta 1, VersioneCapacita resta quella di prima).

// MaxByteDescrizioneClasse e MaxByteFonteClasse: le lunghezze massime, in byte, della descrizione e della fonte di
// una voce. Non sono una taratura del riconoscimento (quelle stanno nell'indice, R43 B): sono la misura del campo,
// come la colonna di una tabella, perché la descrizione finisce in una casella della pagina e nell'esito. Si cambiano
// con un commit che lo dichiara.
const (
	MaxByteDescrizioneClasse = 200
	MaxByteFonteClasse       = 200
)

// TabellaClassi: la tabella delle classi di una famiglia (chiave «classi»).
//   - Segmento: il nome di un segmento identitario della base della famiglia, quello che la tabella guarda (per
//     esempio «serie»);
//   - Voci: una per valore del segmento, senza doppioni. L'ordine nel file non conta: nel canonico le voci stanno in
//     ordine di valore.
type TabellaClassi struct {
	Segmento string       `json:"segmento"`
	Voci     []VoceClasse `json:"voci"`
}

// VoceClasse: una riga della tabella.
//   - Valore: il valore del segmento, scritto come lo dà la lettura normalizzata (SegmentoLetto.Normalizzato): deve
//     essere riconosciuto per intero dal segmento, come il motore lo compila (con le maiuscole della base; con
//     normalizza «maiuscolo» piegato nelle due maiuscole, perché la chiave è la normalizzata: piegaPerLeVoci), e, con
//     normalizza «maiuscolo», non ha lettere ASCII minuscole;
//   - Categoria: minuteria | commerciale (l'enum delle categorie);
//   - Descrizione: che cosa è il pezzo («Molla a compressione»), obbligatoria;
//   - Fonte: da dove viene la voce (la normativa del cliente, «norma ACME 94019»), facoltativa.
type VoceClasse struct {
	Valore      string    `json:"valore"`
	Categoria   Categoria `json:"categoria"`
	Descrizione string    `json:"descrizione"`
	Fonte       string    `json:"fonte,omitempty"`
}

// classi controlla la tabella delle classi di una famiglia (regola 17 di Valida), con i codici che dice:
//   - una famiglia con la tabella non dichiara anche categorie (contratto.classi_con_categorie);
//   - il segmento c'è (contratto.campo_obbligatorio) ed è un segmento identitario della base
//     (contratto.classi_segmento_non_identitario);
//   - almeno una voce (contratto.campo_obbligatorio), non più di max_voci_classi dell'indice (limite.superato: un
//     indice che non dichiara il limite non attiva nessuna tabella, R43 B);
//   - ogni valore c'è (contratto.campo_obbligatorio), è riconosciuto per intero dal segmento compilato come lo
//     compila il motore, ed è già normalizzato (contratto.classi_valore_non_conforme); nessun valore ripetuto
//     (contratto.insieme_con_duplicati);
//   - la categoria è nell'enum (contratto.enum_ignoto);
//   - descrizione obbligatoria (contratto.campo_obbligatorio), descrizione e fonte senza caratteri di controllo o di
//     formato (grammatica.carattere_di_controllo) ed entro le loro lunghezze (limite.superato).
func (v *validatore) classi(f FamigliaCodice, p string) {
	t := f.Classi
	if t == nil {
		return
	}
	pc := p + ".classi"
	if len(f.Categorie) > 0 {
		v.errore(evidenze.Diagnostica{
			Codice:    CodiceClassiConCategorie,
			Percorso:  p + ".categorie",
			Messaggio: "la famiglia ha la tabella delle classi e anche delle categorie: quale valga per un codice non si decide; con la tabella la categoria la dà la voce",
		})
	}

	// Il segmento, e la sua espressione come la compila il motore, per controllare i valori.
	var re *regexp.Regexp
	var seg *SegmentoBase
	if strings.TrimSpace(t.Segmento) == "" {
		v.obbligatorio(pc+".segmento", "il segmento identitario della base che la tabella guarda")
	} else {
		for i := range f.Base.Segmenti {
			if f.Base.Segmenti[i].Nome == t.Segmento {
				seg = &f.Base.Segmenti[i]
				break
			}
		}
		switch {
		case seg == nil:
			v.errore(evidenze.Diagnostica{Codice: CodiceClassiSegmentoNonIdentitario, Percorso: pc + ".segmento",
				Messaggio: fmt.Sprintf("il segmento %q non è della base della famiglia", t.Segmento)})
		case !seg.Identitario:
			v.errore(evidenze.Diagnostica{Codice: CodiceClassiSegmentoNonIdentitario, Percorso: pc + ".segmento",
				Messaggio: fmt.Sprintf("il segmento %q non è identitario: la classe di un codice si legge solo da un segmento della sua identità", t.Segmento)})
		default:
			re = espressioneDelSegmento(*seg, piegaPerLeVoci(f.Base))
		}
	}

	if len(t.Voci) == 0 {
		v.obbligatorio(pc+".voci", "almeno una voce della tabella delle classi")
	}
	if n, massimo := len(t.Voci), v.lim.Grammatica.MaxVociClassi; n > massimo {
		if massimo == 0 {
			v.errore(evidenze.Diagnostica{
				Codice:    CodiceLimiteSuperato,
				Natura:    evidenze.NaturaLimite,
				Percorso:  pc + ".voci",
				Messaggio: fmt.Sprintf("voci della tabella delle classi: %d, e l'indice non dichiara limiti.grammatica.max_voci_classi: senza il limite nessuna tabella si attiva (R43 B)", n),
			})
		} else {
			v.superato(pc+".voci", "voci della tabella delle classi", n, massimo)
		}
	}
	valori := make([]string, len(t.Voci))
	for i, x := range t.Voci {
		valori[i] = x.Valore
		pv := elemento(pc+".voci", x.Valore, i)
		switch {
		case x.Valore == "":
			v.obbligatorio(pv+".valore", "il valore del segmento")
		case re != nil && !re.MatchString(x.Valore):
			v.errore(evidenze.Diagnostica{Codice: CodiceClassiValoreNonConforme, Percorso: pv + ".valore",
				Messaggio: fmt.Sprintf("il valore %q non è riconosciuto per intero dal segmento %q: nessuna lettura lo darebbe", x.Valore, t.Segmento)})
		case re != nil && f.Base.Normalizza == NormalizzaMaiuscolo && strings.ContainsFunc(x.Valore, func(r rune) bool { return 'a' <= r && r <= 'z' }):
			v.errore(evidenze.Diagnostica{Codice: CodiceClassiValoreNonConforme, Percorso: pv + ".valore",
				Messaggio: fmt.Sprintf("il valore %q ha lettere minuscole, ma la base si normalizza in maiuscolo: la voce si scrive come il valore normalizzato", x.Valore)})
		}
		v.enum(pv+".categoria", "categoria della voce", string(x.Categoria), categorie)
		if strings.TrimSpace(x.Descrizione) == "" {
			v.obbligatorio(pv+".descrizione", "la descrizione della voce: che cosa è il pezzo")
		} else {
			v.testoClasse(pv+".descrizione", x.Descrizione, MaxByteDescrizioneClasse)
		}
		if x.Fonte != "" {
			v.testoClasse(pv+".fonte", x.Fonte, MaxByteFonteClasse)
		}
	}
	v.duplicati(pc+".voci", valori)
}

// piegaPerLeVoci: se il valore di una voce si controlla con il segmento piegato nelle due maiuscole. Con le maiuscole
// indifferenti il motore compila il segmento così, e una lettura può dare ogni maiuscola. Con normalizza «maiuscolo» la
// chiave della ricerca è la normalizzata (SegmentoLetto.Normalizzato, le maiuscole ASCII dell'originale): anche con le
// maiuscole esatte e un pattern con lettere minuscole, «abcd» si legge e dà «ABCD», che il segmento non piegato non
// riconosce. Il segmento piegato, con il divieto delle minuscole che segue, accetta esattamente le maiuscole dei valori
// del segmento: quelli che una lettura normalizzata può dare (difetto del 14/10, combinazione esatte + maiuscolo).
func piegaPerLeVoci(b Base) bool {
	return b.Maiuscole == MaiuscoleIndifferentiASCII || b.Normalizza == NormalizzaMaiuscolo
}

// testoClasse: un testo libero di una voce (descrizione, fonte): senza caratteri di controllo o di formato (U+0020
// sì), entro la lunghezza del campo.
func (v *validatore) testoClasse(percorso, s string, massimo int) {
	if strings.ContainsFunc(s, vietatoNelLetterale) {
		v.errore(evidenze.Diagnostica{Codice: CodiceCarattereDiControllo, Percorso: percorso, Messaggio: messaggioControllo("testo", s)})
	}
	if len(s) > massimo {
		v.superato(percorso, "testo lungo (byte)", len(s), massimo)
	}
}

// espressioneDelSegmento: la regex di un segmento della base, ancorata ai due capi, come la scrive il compilatore di
// motorea (LetteraleBase, PatternBase: la stessa funzione). nil se il segmento non si compila: l'errore lo dà già la
// validazione del segmento, e i valori non si controllano.
func espressioneDelSegmento(s SegmentoBase, piega bool) *regexp.Regexp {
	var expr string
	switch {
	case s.Letterale != "" && s.Pattern == "":
		expr = LetteraleBase(s.Letterale, piega)
	case s.Pattern != "" && s.Letterale == "":
		p, err := PatternBase(s.Pattern, piega)
		if err != nil {
			return nil
		}
		expr = p
	default:
		return nil
	}
	re, err := regexp.Compile(`\A(?:` + expr + `)\z`)
	if err != nil {
		return nil
	}
	return re
}

// normalizzaClassi: la tabella con le voci copiate e in ordine di valore (l'ordine nel file non ha significato, e un
// doppione è già un errore di Valida); nil senza tabella, così una famiglia senza ha il canonico di prima.
func normalizzaClassi(t *TabellaClassi) *TabellaClassi {
	if t == nil {
		return nil
	}
	c := *t
	c.Voci = nil
	if len(t.Voci) > 0 {
		c.Voci = append([]VoceClasse(nil), t.Voci...)
	}
	sort.SliceStable(c.Voci, func(i, j int) bool { return c.Voci[i].Valore < c.Voci[j].Valore })
	return &c
}
