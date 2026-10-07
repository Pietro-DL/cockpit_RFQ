package valutazione

import (
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// impronta.go: le impronte di valutazione (fase 0 di B6, IM.1–IM.3; contratto §1.7, §2.3, T-B0-29, T-E1-19, T-E1R-12;
// R90 [U], R100 A; piano 6.4.6 passo 11): l'impronta dell'esito (V1) e l'impronta di ogni prodotto, la regola
// (ImprontaProdotto) sull'ingresso astratto dei dati decisi (DatiDecisiProdotto) e l'adattatore che lo prepara dalla
// fotografia (datiDecisiDelProdotto). A1c la calcola soltanto: la salverà A2 (R100 A, T-E1R-12).

// VersioneImprontaProdotto: la versione del canonico dell'impronta del prodotto (T-B0-29). Entra in Esito e nel canonico
// di ogni ProdottoValutato.Impronta (DatiDecisiProdotto.Versione); una versione nuova del motore non la tocca. Cambia con
// un commit che lo dichiara, e la prova che la fissa si riscrive.
const VersioneImprontaProdotto = 1

// improntaEsito: lo sha256 del canonico dell'esito con Impronta vuota (IM.1; lo stesso schema di EsitoAncoraggi.Impronta).
// Copre tutto l'esito: le quattro versioni, l'impronta della fotografia, quella dell'indice (con i limiti: R43 B) e la
// versione dei limiti, ogni thread per intero (con le impronte annidate dei prodotti della mail, degli ancoraggi e delle
// interpretazioni, con le loro versioni, e da V3 l'impronta di ogni prodotto), i messaggi fuori RFQ e le diagnostiche.
// Restano fuori per costruzione PresaIl e Sorgente della fotografia (che non entrano nell'esito), qualunque orologio,
// l'atteso (R2) e l'esito di confronto, che ha la sua. Per lo stesso thread l'impronta del banco e quella dell'anteprima
// coincidono solo se il banco gira su quel thread solo (F0-14): l'esito è di tutta la fotografia.
func improntaEsito(e Esito) (string, error) {
	e.Impronta = ""
	return jsoncanonico.ImprontaDi(e)
}

// ---- l'impronta del prodotto (IM.3) ----

// DatiDecisiProdotto: l'ingresso astratto dell'impronta del prodotto (IM.3; T-B0-29: copre solo dati decisi). Lo prepara
// l'adattatore dalla fotografia; le prove della regola lo scrivono a mano.
//   - Versione: VersioneImprontaProdotto. Rif, Codice: il prodotto (PV.Rif, PV.CodiceRichiesto, com'è nel DB).
//   - Fonte: la fonte STEP del gesto 3 (PV.Fonte.Riferimento), se c'è.
//   - Componenti, Relazioni, Documenti: il perimetro confermato del prodotto, le sue relazioni confermate con la quantità,
//     i documenti confermati correnti dei suoi componenti.
type DatiDecisiProdotto struct {
	Versione   int                  `json:"versione"`
	Rif        string               `json:"rif"`
	Codice     string               `json:"codice"`
	Fonte      *ImprontaFonte       `json:"fonte,omitempty"`
	Componenti []ImprontaComponente `json:"componenti,omitempty"`
	Relazioni  []ImprontaRelazione  `json:"relazioni,omitempty"`
	Documenti  []ImprontaDocumento  `json:"documenti,omitempty"`
}

// ImprontaFonte: la fonte STEP del gesto 3 nell'impronta: il documento, il suo sha256, la radice scelta, se è superato.
// Mai l'allegato, che cambia con le copie dello stesso contenuto (T-B5-15); chi e quando del gesto restano fuori.
type ImprontaFonte struct {
	DocumentoID uuid.UUID `json:"documento_id"`
	Sha256      string    `json:"sha256"`
	Radice      string    `json:"radice"`
	Superato    bool      `json:"superato"`
}

// ImprontaComponente: un componente deciso del perimetro, con il codice e la revisione registrata com'è (T-B0-34).
type ImprontaComponente struct {
	ID     uuid.UUID `json:"id"`
	Codice string    `json:"codice"`
	Rev    *string   `json:"rev,omitempty"`
}

// ImprontaRelazione: una relazione confermata fra due componenti del perimetro, con la quantità.
type ImprontaRelazione struct {
	Padre  uuid.UUID `json:"padre"`
	Figlio uuid.UUID `json:"figlio"`
	Qta    int       `json:"qta"`
}

// ImprontaDocumento: un documento confermato corrente di un componente del perimetro, con il tipo, lo sha256 e la
// revisione registrata; Confermata e CodiceConfermato vengono dalla decisione sul documento (DecisioneIdentita), vuoti
// in A1c (LD-27).
type ImprontaDocumento struct {
	ComponenteID     uuid.UUID `json:"componente_id"`
	ID               uuid.UUID `json:"id"`
	Tipo             string    `json:"tipo"`
	Sha256           string    `json:"sha256"`
	Rev              *string   `json:"rev,omitempty"`
	Confermata       *string   `json:"confermata,omitempty"`
	CodiceConfermato *string   `json:"codice_confermato,omitempty"`
}

// ImprontaProdotto: l'impronta del prodotto (R90 [U]; T-B0-29, T-E1-19; IM.3): lo sha256 del canonico dei dati decisi,
// con ogni elenco ordinato (i componenti e i documenti per ID, le relazioni per padre, figlio e quantità),
// così l'ordine degli ingressi non conta. È pura e non cambia l'ingresso. Si calcola per ogni prodotto, anche non
// verificato, perché il fascicolo la confronta con il gesto di congelamento (impronta_cambiata). Una versione nuova del
// motore non la tocca: cambia solo con VersioneImprontaProdotto o con un dato deciso.
func ImprontaProdotto(d DatiDecisiProdotto) (string, error) {
	c := d
	c.Componenti = append([]ImprontaComponente(nil), d.Componenti...)
	sort.SliceStable(c.Componenti, func(i, j int) bool { return c.Componenti[i].ID.String() < c.Componenti[j].ID.String() })
	c.Relazioni = append([]ImprontaRelazione(nil), d.Relazioni...)
	sort.SliceStable(c.Relazioni, func(i, j int) bool {
		a, b := c.Relazioni[i], c.Relazioni[j]
		if a.Padre != b.Padre {
			return a.Padre.String() < b.Padre.String()
		}
		if a.Figlio != b.Figlio {
			return a.Figlio.String() < b.Figlio.String()
		}
		return a.Qta < b.Qta
	})
	c.Documenti = append([]ImprontaDocumento(nil), d.Documenti...)
	sort.SliceStable(c.Documenti, func(i, j int) bool { return c.Documenti[i].ID.String() < c.Documenti[j].ID.String() })
	return jsoncanonico.ImprontaDi(c)
}

// sezioniDellImprontaProdotto: le sezioni della fotografia da cui l'impronta del prodotto dipende (T-12, IM.3): senza una di
// loro non si calcola, e PV.Impronta resta vuota (nessun campo nuovo).
var sezioniDellImprontaProdotto = []string{fotorfq.SezioneComponenti, fotorfq.SezioneRelazioni, fotorfq.SezioneDocumenti,
	fotorfq.SezioneProvenienze, fotorfq.SezioneStepProdotto}

// improntaNonDeterminabile: una sezione dell'impronta del prodotto è assente (T-12): una di IM.3, oppure una delle
// sezioni della fonte (sezioniDellaFonte). L'impronta comprende la fonte del gesto 3 (PV.Fonte.Riferimento), e senza una
// sezione della fonte il riferimento non si calcola: un'impronta calcolata senza la fonte sembrerebbe quella dei dati
// decisi e non lo sarebbe, quindi resta vuota (R-82 della revisione di V3; scostamento [T] dall'elenco di IM.3, dubbio
// T-B6-151). Senza grammatica (T-B1-11) la fonte e il suo riferimento si calcolano, e l'impronta c'è.
func improntaNonDeterminabile(sezioni map[string]fotorfq.StatoSezione) bool {
	return unaSezioneAssente(sezioni, sezioniDellImprontaProdotto, sezioniDellaFonte)
}

// unaSezioneAssente: almeno una sezione di uno degli elenchi è assente (T-12; sezioneAssente).
func unaSezioneAssente(sezioni map[string]fotorfq.StatoSezione, elenchi ...[]string) bool {
	for _, elenco := range elenchi {
		for _, k := range elenco {
			if sezioneAssente(sezioni, k) {
				return true
			}
		}
	}
	return false
}

// datiDecisiDelProdotto: l'adattatore dell'impronta del prodotto (IM.3), dalla fotografia; mai ciò che non è deciso:
//   - Fonte: documento, sha256, radice e Superato del riferimento del gesto 3 (PV.Fonte.Riferimento), se c'è: una fonte
//     sostituita cambia l'impronta;
//   - il perimetro: il componente del prodotto e i componenti attivi raggiungibili per le relazioni confermate
//     (bomDelThread.perimetro: le righe certe della completezza, R62 e A), con il codice e la revisione registrata
//     (T-B0-34: com'è; RevProvenienza si calcola dai fatti e resta fuori). Una decisione tracciata (DecisioneIdentita) in
//     A1c non c'è (LD-27). Un componente condiviso entra nell'impronta di ogni prodotto che lo contiene (T-E1-19, PO-26);
//   - le relazioni confermate percorse nel perimetro, fra componenti attivi, con la quantità (la posizione non arriva:
//     registro §17.38);
//   - i documenti confermati correnti (SostituitoDa nullo) di un componente del perimetro, con tipo, sha256 e revisione
//     registrata.
//
// Restano fuori, per l'elenco chiuso di T-B0-29: i documenti generali (senza componente), le proposte, l'«assegna», la
// destinazione F8 e i candidati, le deroghe, le conferme della categoria, i gesti di verifica della BOM, i percorsi, le
// versioni del motore e il codice manuale di una riga aperta (F0-08: la riga aperta blocca comunque la nomenclatura). Un
// prodotto dello scenario, senza componente né riferimento, ha solo Rif e Codice: non è mai verificato.
func datiDecisiDelProdotto(pv ProdottoValutato, t fotorfq.Thread, b *bomDelThread) DatiDecisiProdotto {
	d := DatiDecisiProdotto{Versione: VersioneImprontaProdotto, Rif: pv.Rif, Codice: pv.CodiceRichiesto}
	if r := pv.Fonte.Riferimento; r != nil {
		d.Fonte = &ImprontaFonte{DocumentoID: r.DocumentoID, Sha256: r.Sha256, Radice: r.Radice, Superato: r.Superato}
	}
	perimetro, relazioni := b.perimetro(pv.ComponenteID)
	for _, c := range t.Componenti {
		if perimetro[c.ID] {
			d.Componenti = append(d.Componenti, ImprontaComponente{ID: c.ID, Codice: c.Codice, Rev: copiaTesto(c.Rev)})
		}
	}
	for _, r := range relazioni {
		d.Relazioni = append(d.Relazioni, ImprontaRelazione{Padre: r.PadreID, Figlio: r.FiglioID, Qta: r.Qta})
	}
	for _, doc := range t.Documenti {
		if doc.ComponenteID != nil && perimetro[*doc.ComponenteID] && doc.SostituitoDa == nil {
			d.Documenti = append(d.Documenti, ImprontaDocumento{ComponenteID: *doc.ComponenteID, ID: doc.ID, Tipo: doc.Tipo, Sha256: doc.Sha256,
				Rev: copiaTesto(doc.Rev)})
		}
	}
	return d
}
