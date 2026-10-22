package distintabozza

// Le evidenze documentali del formato 3 e la decisione conferma_categoria (ondata C di V1; U2 §6.4, §7.7; EM §3.4,
// §3.6.1, §3.6.3).
//
// L'evidenza documentale e' un file attivo della RFQ scelto a mano dall'ingegnere: {allegato_id, sha256, pagine?, riga?,
// nota?}. Sta, facoltativa, nella decisione identita in modo correzione (evidenza_documentale), nei nodi manuali
// (evidenze_identita[]) e negli archi con origine utente (evidenze_arco[]). Lo sha lo manda il browser (lo ha da S1) e il
// server lo verifica contro l'allegato attivo (sha_diverso): il server non riscrive mai lo snapshot. Qui la sola forma;
// l'appartenenza alla RFQ, l'occorrenza attiva e lo sha li controlla chi salva (salva3.go).
//
// conferma_categoria (il diciassettesimo tipo, U2 §6.4.2): la conferma, o il rifiuto, di una categoria del nodo
// (fabbricato, commerciale, minuteria), con il punto d'ingresso (editor, wizard), l'origine della categoria e, per una
// decisione riportata da da-conferma-1, la conferma che l'ha introdotta. Mai sul finito.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// I limiti delle evidenze documentali (U2 §7.7). MaxPagineEvidenza e MaxPagina sono tetti di forma [T].
const (
	MaxEvidenze       = 20
	MaxRigaEvidenza   = 200
	MaxNotaEvidenza   = 500
	MaxPagineEvidenza = 1000
	MaxPagina         = 100000
)

// I motivi di CORPO_NON_VALIDO delle evidenze e delle categorie (U2 §6.4.2, §7.7).
const (
	MotivoShaDiverso          = "sha_diverso"
	MotivoCategoriaNonAmmessa = "categoria_non_ammessa"
	MotivoCategoriaSulFinito  = "categoria_sul_finito"
)

// Il vocabolario chiuso della categoria, lo stesso che il motore accetta come ConfermaCategoria (U2 §6.4.2).
var categorieAmmesse = map[string]bool{"fabbricato": true, "commerciale": true, "minuteria": true}

// CategoriaAmmessa dice se la categoria e' nel vocabolario chiuso.
func CategoriaAmmessa(c string) bool { return categorieAmmesse[c] }

// Esiti, origini e punti d'ingresso di conferma_categoria (U2 §6.4.2).
const (
	EsitoCategoriaConferma = "conferma"
	EsitoCategoriaRifiuto  = "rifiuto"

	OrigineCategoriaClasse    = "classe_grammatica"
	OrigineCategoriaNormativa = "normativa"
	OrigineCategoriaManuale   = "manuale"

	IngressoEditor = "editor"
	IngressoWizard = "wizard"
)

// EvidenzaDocumentale: un file attivo della RFQ citato dall'ingegnere come evidenza (U2 §7.7). Le pagine sono ordinate e
// senza doppioni (altrimenti [3,1] e [1,3] darebbero byte diversi e un falso fondamento nuovo in X-02); la riga resta
// byte per byte; la nota segue la regola NFC dei motivi, ma senza normalizzarla: una nota che non e' NFC si rifiuta.
type EvidenzaDocumentale struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Sha256     string    `json:"sha256"`
	Pagine     []int     `json:"pagine,omitempty"`
	Riga       string    `json:"riga,omitempty"`
	Nota       string    `json:"nota,omitempty"`
}

// chiaveOrdine: l'ordine canonico delle evidenze di un elenco (allegato, sha, pagine, riga, nota).
func (e EvidenzaDocumentale) chiaveOrdine() string {
	pp := make([]string, len(e.Pagine))
	for i, p := range e.Pagine {
		pp[i] = fmt.Sprintf("%08d", p)
	}
	return e.AllegatoID.String() + "\x00" + e.Sha256 + "\x00" + strings.Join(pp, ",") + "\x00" + e.Riga + "\x00" + e.Nota
}

type evidenzaIn struct {
	AllegatoID *uuid.UUID `json:"allegato_id"`
	Sha256     *string    `json:"sha256"`
	Pagine     []int      `json:"pagine"`
	Riga       *string    `json:"riga"`
	Nota       *string    `json:"nota"`
}

func (in evidenzaIn) converti() EvidenzaDocumentale {
	e := EvidenzaDocumentale{Sha256: valore(in.Sha256), Pagine: in.Pagine, Riga: valore(in.Riga), Nota: valore(in.Nota)}
	if in.AllegatoID != nil {
		e.AllegatoID = *in.AllegatoID
	}
	if len(e.Pagine) == 0 {
		e.Pagine = nil
	}
	return e
}

func convertiEvidenze(in []evidenzaIn) []EvidenzaDocumentale {
	if len(in) == 0 {
		return nil
	}
	out := make([]EvidenzaDocumentale, len(in))
	for i, x := range in {
		out[i] = x.converti()
	}
	return out
}

// validaEvidenza: la forma di un'evidenza documentale (U2 §7.7), senza il database.
func validaEvidenza(campo string, e EvidenzaDocumentale) *Errore {
	if e.AllegatoID == uuid.Nil {
		return corpo(campo+".allegato_id", "l'evidenza cita un file della RFQ: allegato_id")
	}
	if !reSha256.MatchString(e.Sha256) {
		return corpo(campo+".sha256", "lo sha256 dell'evidenza e' di 64 cifre esadecimali minuscole (lo da' S1)")
	}
	if len(e.Pagine) > MaxPagineEvidenza {
		return corpo(campo+".pagine", "al piu' %d pagine", MaxPagineEvidenza)
	}
	for i, p := range e.Pagine {
		if p < 1 || p > MaxPagina {
			return corpo(fmt.Sprintf("%s.pagine[%d]", campo, i), "pagina %d fuori da 1…%d", p, MaxPagina)
		}
		if i > 0 && p <= e.Pagine[i-1] {
			return corpo(campo+".pagine", "le pagine sono in ordine crescente e senza doppioni")
		}
	}
	if e.Riga != "" && (runeOltre(e.Riga, MaxRigaEvidenza) || haControlli(e.Riga) || strings.TrimSpace(e.Riga) == "") {
		return corpo(campo+".riga", "la riga e' un testo non vuoto di al piu' %d caratteri, senza caratteri di controllo", MaxRigaEvidenza)
	}
	if e.Nota != "" {
		if !norm.NFC.IsNormalString(e.Nota) {
			return corpo(campo+".nota", "la nota non e' in NFC: il server non la normalizza da solo, la manda gia' normalizzata la pagina")
		}
		if !testoLibero(e.Nota, MaxNotaEvidenza) {
			return corpo(campo+".nota", "la nota e' un testo di al piu' %d caratteri, senza caratteri di controllo", MaxNotaEvidenza)
		}
	}
	return nil
}

// validaEvidenze: un elenco di evidenze (evidenze_identita, evidenze_arco): al piu' MaxEvidenze, ognuna valida, nessuna
// ripetuta.
func validaEvidenze(campo string, ee []EvidenzaDocumentale) *Errore {
	if len(ee) > MaxEvidenze {
		return corpo(campo, "al piu' %d evidenze", MaxEvidenze)
	}
	viste := map[string]bool{}
	for i, e := range ee {
		c := campo + "[" + strconv.Itoa(i) + "]"
		if x := validaEvidenza(c, e); x != nil {
			return x
		}
		k := e.chiaveOrdine()
		if viste[k] {
			return corpo(c, "evidenza ripetuta")
		}
		viste[k] = true
	}
	return nil
}

// ordinaEvidenze mette un elenco di evidenze nell'ordine canonico (come le caselle di un nodo).
func ordinaEvidenze(ee []EvidenzaDocumentale) []EvidenzaDocumentale {
	if len(ee) == 0 {
		return nil
	}
	out := append([]EvidenzaDocumentale(nil), ee...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].chiaveOrdine() < out[j].chiaveOrdine() })
	return out
}

// SoggettoCategoria: il nodo della bozza di una conferma_categoria (U2 §6.4.2).
type SoggettoCategoria struct {
	ChiaveBozza string `json:"chiave_bozza"`
}

// ClasseProposta: la copia di tipo_proposto.classe della voce nel registro citato (U2 §6.4.2), verificata dal server.
type ClasseProposta struct {
	Categoria string `json:"categoria"`
	Fonte     string `json:"fonte"`
	Valore    string `json:"valore"`
}

// DaConferma: la conferma che ha introdotto una decisione riportata da da-conferma-1 (U2 §6.4.2): autore e momento si
// risolvono risalendo al registro di quella conferma, mai dall'evento bozza_aperta.
type DaConferma struct {
	ConfermaID  uuid.UUID `json:"conferma_id"`
	Numero      int       `json:"numero"`
	DecisioneID string    `json:"decisione_id"`
}

type soggettoIn struct {
	ChiaveBozza *string `json:"chiave_bozza"`
}

type classePropostaIn struct {
	Categoria *string `json:"categoria"`
	Fonte     *string `json:"fonte"`
	Valore    *string `json:"valore"`
}

type daConfermaIn struct {
	ConfermaID  *uuid.UUID `json:"conferma_id"`
	Numero      *int       `json:"numero"`
	DecisioneID *string    `json:"decisione_id"`
}

// validaCategoria: la forma di una conferma_categoria (U2 §6.4.2), senza il database e senza la bozza.
func (d *Decisione3) validaCategoria(campo string) *Errore {
	if d.Soggetto == nil || !reChiave.MatchString(d.Soggetto.ChiaveBozza) {
		return corpo(campo+".soggetto.chiave_bozza", "il soggetto e' un nodo della bozza: {chiave_bozza}")
	}
	if !CategoriaAmmessa(d.Categoria) {
		return corpoMotivo(campo+".categoria", MotivoCategoriaNonAmmessa, "categoria %q: fabbricato, commerciale o minuteria", d.Categoria)
	}
	if d.Esito != EsitoCategoriaConferma && d.Esito != EsitoCategoriaRifiuto {
		return corpo(campo+".esito", "esito %q: conferma o rifiuto", d.Esito)
	}
	if d.Ingresso != IngressoEditor && d.Ingresso != IngressoWizard {
		return corpo(campo+".ingresso", "ingresso %q: editor o wizard", d.Ingresso)
	}
	switch d.OrigineCategoria {
	case OrigineCategoriaClasse:
		if d.ClasseProposta == nil {
			return corpo(campo+".classe_proposta", "con classe_grammatica la decisione copia la classe proposta dal registro")
		}
		if d.Importazione == "" {
			return corpo(campo+".importazione", "con classe_grammatica la decisione cita il registro su cui la classe e' letta")
		}
	case OrigineCategoriaNormativa, OrigineCategoriaManuale:
		if d.ClasseProposta != nil {
			return corpo(campo+".classe_proposta", "la classe proposta c'e' solo con classe_grammatica")
		}
	default:
		return corpo(campo+".origine_categoria", "origine %q: classe_grammatica, normativa o manuale", d.OrigineCategoria)
	}
	// il motivo non e' piu' obbligatorio con l'origine normativa o manuale o con il rifiuto (messaggio 46 [U]): la
	// decisione e' il gesto, con il suo autore nel giornale della bozza
	if c := d.ClasseProposta; c != nil {
		if !CategoriaAmmessa(c.Categoria) {
			return corpoMotivo(campo+".classe_proposta.categoria", MotivoCategoriaNonAmmessa, "categoria proposta %q fuori dal vocabolario", c.Categoria)
		}
		for _, x := range []struct{ nome, v string }{{"fonte", c.Fonte}, {"valore", c.Valore}} {
			if strings.TrimSpace(x.v) == "" || runeOltre(x.v, MaxCodiceSegnale) || haControlli(x.v) {
				return corpo(campo+".classe_proposta."+x.nome, "%s della classe proposta: non vuoto, al piu' %d caratteri", x.nome, MaxCodiceSegnale)
			}
		}
	}
	if d.Importazione != "" && !reSha256.MatchString(d.Importazione) {
		return corpo(campo+".importazione", "l'impronta del registro e' di 64 cifre esadecimali minuscole")
	}
	if x := d.DaConferma; x != nil {
		if x.ConfermaID == uuid.Nil || x.Numero < 1 || !reChiave.MatchString(x.DecisioneID) {
			return corpo(campo+".da_conferma", "da_conferma vuole conferma_id, numero (>= 1) e decisione_id della conferma che l'ha introdotta")
		}
	}
	return nil
}
