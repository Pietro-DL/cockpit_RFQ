package confermabom

// La validita' dell'ultima conferma e il controllo della base (contratto rev. 3 §5.1, «Parole» e «Controllo sotto
// lucchetto»; R-18). Funzioni pure: le conferme, il contatore e la firma ricalcolata li legge chi chiama
// (GetUltimaConfermaStrutturale e GetWorkingContatore di db/queries/p4_conferme.sql; FirmaPerimetro qui).

import (
	"regexp"

	"github.com/google/uuid"
)

var reFirma = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Conferma e' una riga di conferma_strutturale di un prodotto, con le colonne che decidono la validita'.
type Conferma struct {
	ID             uuid.UUID
	Numero         int32  // progressivo per prodotto: l'ultima e' quella col numero piu' alto
	Contatore      int64  // working_contatore.struttura letto dopo le scritture della conferma
	FirmaStruttura string // firma del perimetro dopo le scritture (FirmaPerimetro)
}

// Working e' lo stato corrente della RFQ per un prodotto: il contatore struttura della RFQ (una riga mancante vale
// 0) e la firma del perimetro del prodotto ricalcolata adesso (FirmaPerimetro). Firma vuota = non calcolata: in una
// lettura (ConfermaValida, UltimaValida) allora decide solo il contatore; nel controllo sotto lucchetto
// (ControllaBase) e' ErrIngresso, perche' il §5.1 vuole la firma ricalcolata.
type Working struct {
	Contatore int64
	Firma     string
}

// Validita dice se una conferma vale ancora, e per quale ragione.
type Validita struct {
	Valida       bool // Ultima e (PerContatore o PerFirma)
	Ultima       bool // e' la conferma col numero piu' alto del prodotto
	PerContatore bool // il contatore registrato e' quello corrente
	PerFirma     bool // la firma registrata e' quella del perimetro di adesso
}

// Ultima e' la conferma col numero piu' alto fra quelle di un prodotto; ok = false senza conferme. Due conferme
// con lo stesso numero, o con lo stesso ID, sono un ingresso impossibile (la 0022 ha un indice unico per prodotto
// e numero): ErrIngresso. Pura.
func Ultima(conferme []Conferma) (Conferma, bool, error) {
	var u Conferma
	numeri, id := map[int32]bool{}, map[uuid.UUID]bool{}
	for i, c := range conferme {
		if err := controllaConferma(c); err != nil {
			return Conferma{}, false, err
		}
		if numeri[c.Numero] {
			return Conferma{}, false, ingresso("due conferme con il numero %d", c.Numero)
		}
		if id[c.ID] {
			return Conferma{}, false, ingresso("la conferma %s c'e' due volte", c.ID)
		}
		numeri[c.Numero], id[c.ID] = true, true
		if i == 0 || c.Numero > u.Numero {
			u = c
		}
	}
	return u, len(conferme) > 0, nil
}

func controllaConferma(c Conferma) error {
	if c.Numero < 1 {
		return ingresso("la conferma %s ha il numero %d", c.ID, c.Numero)
	}
	if c.Contatore < 0 {
		return ingresso("la conferma %s ha il contatore %d", c.ID, c.Contatore)
	}
	if !reFirma.MatchString(c.FirmaStruttura) {
		return ingresso("la conferma %s non ha una firma di 64 cifre esadecimali minuscole", c.ID)
	}
	return nil
}

func controllaWorking(w Working) error {
	if w.Contatore < 0 {
		return ingresso("contatore della working %d", w.Contatore)
	}
	if w.Firma != "" && !reFirma.MatchString(w.Firma) {
		return ingresso("la firma della working non e' di 64 cifre esadecimali minuscole")
	}
	return nil
}

// ConfermaValida dice se la conferma c di un prodotto vale ancora: e' l'ultima fra le conferme del prodotto, e il
// contatore struttura registrato e' quello corrente oppure la firma registrata e' quella del perimetro ricalcolato
// adesso (contratto rev. 3 §5.1). Il contatore e' per RFQ: una scrittura in un altro prodotto lo muove, e allora
// decide la firma. conferme sono tutte le conferme del prodotto, c compresa. Pura.
func ConfermaValida(c Conferma, conferme []Conferma, w Working) (Validita, error) {
	if err := controllaConferma(c); err != nil {
		return Validita{}, err
	}
	if err := controllaWorking(w); err != nil {
		return Validita{}, err
	}
	u, ok, err := Ultima(conferme)
	if err != nil {
		return Validita{}, err
	}
	if !ok {
		return Validita{}, ingresso("la conferma %s non e' fra le conferme del prodotto", c.ID)
	}
	trovata := false
	for _, x := range conferme {
		if x.ID == c.ID {
			trovata = x == c
		}
	}
	if !trovata {
		return Validita{}, ingresso("la conferma %s non e' fra le conferme del prodotto, o vi e' diversa", c.ID)
	}
	v := Validita{Ultima: u.ID == c.ID, PerContatore: c.Contatore == w.Contatore, PerFirma: w.Firma != "" && c.FirmaStruttura == w.Firma}
	v.Valida = v.Ultima && (v.PerContatore || v.PerFirma)
	return v, nil
}

// UltimaValida e' l'ultima conferma del prodotto e la sua validita'; ok = false se il prodotto non ha conferme.
// Pura.
func UltimaValida(conferme []Conferma, w Working) (Conferma, Validita, bool, error) {
	u, ok, err := Ultima(conferme)
	if err != nil || !ok {
		return Conferma{}, Validita{}, false, err
	}
	v, err := ConfermaValida(u, conferme, w)
	return u, v, err == nil, err
}

// I motivi di BASE_SUPERATA che dipendono da conferme, contatore e firma (contratto rev. 3 §4.4 n. 1). Gli altri
// due (fonte_rifiutata, rif_scaduti) dipendono dalla fonte STEP, e li aggiunge chi chiama.
const (
	MotivoConfermaCambiata  = "conferma_cambiata"
	MotivoStrutturaCambiata = "struttura_cambiata"
)

// Base e' la base dichiarata da una bozza (versione_bom_base e firma_base): l'ultima conferma letta (nessuna:
// ConfermaID nil e Numero 0), il contatore struttura letto con lei, la firma del perimetro letta dal server.
type Base struct {
	ConfermaID *uuid.UUID
	Numero     int32
	Contatore  int64
	Firma      string
}

// ControllaBase e' il controllo della base sotto lucchetto (contratto rev. 3 §5.1): l'ultima conferma del prodotto
// deve essere quella della base (altrimenti conferma_cambiata), e il contatore struttura corrente deve essere
// quello della base oppure la firma del perimetro di adesso quella della base (altrimenti struttura_cambiata). I
// motivi in quest'ordine; nessun motivo = la base vale. Pura.
//
// La firma della working e' obbligatoria (ErrIngresso se vuota): senza, il controllo scenderebbe al solo contatore
// senza dirlo, e ogni scrittura in un altro prodotto della RFQ darebbe un BASE_SUPERATA falso (revisione della
// corsia D, B3).
func ControllaBase(b Base, conferme []Conferma, w Working) ([]string, error) {
	if b.Contatore < 0 || b.Numero < 0 || (b.ConfermaID == nil) != (b.Numero == 0) {
		return nil, ingresso("base non coerente (conferma %v, numero %d, contatore %d)", b.ConfermaID, b.Numero, b.Contatore)
	}
	if !reFirma.MatchString(b.Firma) {
		return nil, ingresso("la firma della base non e' di 64 cifre esadecimali minuscole")
	}
	if err := controllaWorking(w); err != nil {
		return nil, err
	}
	if w.Firma == "" {
		return nil, ingresso("il controllo della base vuole la firma del perimetro ricalcolata adesso")
	}
	u, ok, err := Ultima(conferme)
	if err != nil {
		return nil, err
	}
	var motivi []string
	switch {
	case !ok && b.ConfermaID != nil, ok && (b.ConfermaID == nil || *b.ConfermaID != u.ID || b.Numero != u.Numero):
		motivi = append(motivi, MotivoConfermaCambiata)
	}
	if b.Contatore != w.Contatore && b.Firma != w.Firma {
		motivi = append(motivi, MotivoStrutturaCambiata)
	}
	return motivi, nil
}
