package filerfq

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/storage/staging"
)

// Occorrenza e' un file nella RFQ come la vede il ciclo dei file: l'allegato, la RFQ del suo messaggio, lo stato
// (nessuna riga = attivo, versione 0) e la proposta di documento.
type Occorrenza struct {
	db.ListOccorrenzeSorgenteRow
}

// Thread e' la RFQ dell'occorrenza (uuid zero se il messaggio non e' agganciato a una RFQ).
func (o Occorrenza) Thread() uuid.UUID { return o.ThreadID.UUID }

// Sha: lo sha256 in minuscolo, o "" (un allegato di posta mai scaricato non ce l'ha).
func (o Occorrenza) Sha() string {
	if !o.Sha256.Valid {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(o.Sha256.String))
}

// StatoCiclo e' lo stato per il ciclo dei file: uno scarto fatto col vecchio «Scarta il file» vale scartato (EM §10.2;
// giro 1, TR-17), anche se per il predicato SQL resta attivo.
func (o Occorrenza) StatoCiclo() string {
	if o.ScartatoLegacy {
		return StatoScartato
	}
	return o.Stato
}

// gestibile: la natura che il ciclo dei file gestisce (EM §10.9: file ed elementi di Outlook).
func (o Occorrenza) gestibile() bool {
	return o.Natura == db.NaturaAllegatoFile || o.Natura == db.NaturaAllegatoElementoOutlook
}

// leggiOccorrenze legge le occorrenze per id, nell'ordine degli id (allegato_id crescente).
func leggiOccorrenze(ctx context.Context, q *db.Queries, ids []uuid.UUID) ([]Occorrenza, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	righe, err := q.ListOccorrenzeSorgente(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Occorrenza, 0, len(righe))
	for _, r := range righe {
		out = append(out, Occorrenza{r})
	}
	return out, nil
}

// leggiOccorrenza: un'occorrenza della RFQ; nil se l'allegato non c'e' o non e' della RFQ (per chi chiama: 422
// FUORI_RFQ, mai 404).
func leggiOccorrenza(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) (*Occorrenza, error) {
	occ, err := leggiOccorrenze(ctx, q, []uuid.UUID{allegato})
	if err != nil {
		return nil, err
	}
	if len(occ) != 1 || !occ[0].ThreadID.Valid || occ[0].Thread() != thread {
		return nil, nil
	}
	return &occ[0], nil
}

// Perimetro e' il perimetro di un archivio (EM §10.4, «Archivi e voci»; base della domanda V1-Q21): le voci,
// ricorsivamente, che non sono scartate (anche col gesto precedente). Archivio dice se l'allegato ha voci (anche
// tutte scartate): solo allora il corpo dei gesti porta n_voci e impronta_voci.
//
// [T] della corsia file: il perimetro e' lo stesso per «Scarta», «Metti in Altri file» e «Riattiva» (le voci non
// scartate), cosi' il dettaglio di S1 ne mostra uno solo. «Scarta» le scarta tutte; «Metti in Altri file» mette in
// Altri file quelle attive (quelle gia' in Altri file hanno gia' la loro copia); «Riattiva» riattiva quelle in Altri
// file. Le dipendenze si calcolano su tutte.
type Perimetro struct {
	Archivio bool
	Voci     []Occorrenza
}

// NVoci e Impronta: il conteggio e lo sha256 del JSON canonico dell'elenco ordinato degli allegato_id delle voci, lo
// stesso schema di impronta_dipendenze.
func (p Perimetro) NVoci() int { return len(p.Voci) }

func (p Perimetro) Impronta() string {
	ids := make([]string, 0, len(p.Voci))
	for _, v := range p.Voci {
		ids = append(ids, v.AllegatoID.String())
	}
	sort.Strings(ids)
	h, err := jsoncanonico.ImprontaDi(ids)
	if err != nil {
		return ""
	}
	return h
}

// perimetro calcola il perimetro di un'occorrenza: le voci non scartate dell'archivio, della stessa RFQ.
func perimetro(ctx context.Context, q *db.Queries, o Occorrenza) (Perimetro, error) {
	ids, err := q.ListDiscendentiArchivio(ctx, o.AllegatoID)
	if err != nil {
		return Perimetro{}, err
	}
	if len(ids) == 0 {
		return Perimetro{}, nil
	}
	tutte, err := leggiOccorrenze(ctx, q, ids)
	if err != nil {
		return Perimetro{}, err
	}
	p := Perimetro{Archivio: true}
	for _, v := range tutte {
		if v.ThreadID != o.ThreadID || v.StatoCiclo() == StatoScartato {
			continue
		}
		p.Voci = append(p.Voci, v)
	}
	return p, nil
}

// controllaPerimetro confronta il perimetro dichiarato dal corpo con quello che il server ricalcola: diverso = 409
// ANTEPRIMA_SUPERATA con anteprima voci_archivio e l'elenco giusto (EM §10.4; giro 1, SIC-11). Un file che non e' un
// archivio accetta solo un perimetro assente o vuoto.
func controllaPerimetro(p Perimetro, nVoci *int, impronta *string) *Rifiuto {
	dichN, dichI := 0, ""
	if nVoci != nil {
		dichN = *nVoci
	}
	if impronta != nil {
		dichI = *impronta
	}
	if !p.Archivio {
		if dichN == 0 && (dichI == "" || dichI == (Perimetro{}).Impronta()) {
			return nil
		}
		return perimetroCambiato(p)
	}
	if nVoci == nil || impronta == nil || dichN != p.NVoci() || dichI != p.Impronta() {
		return perimetroCambiato(p)
	}
	return nil
}

func perimetroCambiato(p Perimetro) *Rifiuto {
	voci := make([]map[string]any, 0, len(p.Voci))
	for _, v := range p.Voci {
		voci = append(voci, map[string]any{"allegato_id": v.AllegatoID, "nome_file": v.NomeFile, "stato": v.StatoCiclo()})
	}
	return rifiuto(409, "ANTEPRIMA_SUPERATA", "Il perimetro delle voci dell'archivio è cambiato: rivedi l'elenco e riprova.",
		extra("anteprima", "voci_archivio", "n_voci", p.NVoci(), "impronta_voci", p.Impronta(), "voci", voci))
}

// presenteNelloStaging: il file dello staging dell'occorrenza e' il contenuto con quello sha (EContenuto: il nome e'
// l'hash, e sotto _contenuti un file arriva solo verificato) e c'e' sul disco. Non ricalcola l'hash: per quello c'e'
// staging.ContenutoGiaPresente, dove serve davvero (sotto BloccaContenuto, prima di adottare o di rilasciare).
func presenteNelloStaging(radice, percorso, sha string) bool {
	if strings.TrimSpace(radice) == "" || percorso == "" || sha == "" {
		return false
	}
	if !staging.EContenuto(radice, percorso, sha) {
		return false
	}
	st, err := os.Stat(percorso)
	return err == nil && !st.IsDir()
}

// idDi: gli allegato_id delle occorrenze.
func idDi(occ []Occorrenza) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(occ))
	for _, o := range occ {
		out = append(out, o.AllegatoID)
	}
	return out
}

// ordinate: le occorrenze in ordine di allegato_id (l'ordine fisso dei lucchetti e delle scritture).
func ordinate(occ []Occorrenza) []Occorrenza {
	out := append([]Occorrenza(nil), occ...)
	sort.Slice(out, func(i, j int) bool { return out[i].AllegatoID.String() < out[j].AllegatoID.String() })
	return out
}

// improntaDi: lo sha256 del JSON canonico di un valore (jsoncanonico, canonico-1).
func improntaDi(v any) (string, error) { return jsoncanonico.ImprontaDi(v) }
