package confermabom

// Lo stato della RFQ e la base di una bozza (giro 5, A1d, P4, passo B2.3 parte con il database; piano esecutivo B2-B5
// §1.1; contratto rev. 3 §5.1, §6.4, Emendamento 3 E3.1-E3.2). Qui si leggono, in una volta sola, i componenti, gli
// archi, le conferme e le fonti autorizzate di tutta la RFQ; poi le regole pure di firma.go e validita.go
// (firma-perimetro-2, ConfermaValida, ControllaBase) lavorano su questi dati. Nessuna regola nuova: le funzioni di questo
// file mettono insieme quelle che c'erano.
//
// V1, ondata B (passo 6 del piano della 0023): recuperato dalla tappa del 13/10, che la revisione dava «riusabile, da
// adattare» (V0 §2.3), con le query rigenerate con sqlc in db/queries/v1_bozza_rotte.sql. Lo usano R1 (versione corrente
// e stato della base della bozza formato 3, EM §9.1) e R2 (il rebase della sola base, EM §9.2).
//
// Chi legge sceglie la transazione: R1 una REPEATABLE READ READ ONLY (una fotografia coerente), R2 quella di
// scrittura dopo BloccaThread (il controllo della base del rebase sotto lucchetto, §5.1 «Controllo sotto lucchetto»).

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// TestataConferma e' una conferma strutturale come la legge lo stato della RFQ: le colonne della validita' (Conferma)
// piu' il prodotto, il formato e il criterio.
type TestataConferma struct {
	Conferma
	Prodotto  uuid.UUID
	Formato   int16
	MonoPezzo bool
	Criterio  string
}

// FonteAutorizzata e' la fonte STEP autorizzata di un finito (componente.step_strutturale_id): il documento, il suo
// sha256 e il primo allegato da cui e' arrivato (contratto rev. 3 §4.3, versione_corrente.fonte_autorizzata).
type FonteAutorizzata struct {
	DocumentoID uuid.UUID  `json:"documento_id"`
	AllegatoID  *uuid.UUID `json:"allegato_id"`
	Sha256      string     `json:"sha256"`
	// Superata: il documento e' stato sostituito (documento.sostituito_da). Non e' nel JSON di R1/R4 (contratto §4.3:
	// {documento_id, allegato_id, sha256}); il riepilogo la usa per K-02: una fonte superata non regge il criterio step
	// ne' la dichiarazione mono_pezzo (secondo tempo della tappa A, B2.4) [T].
	Superata bool `json:"-"`
}

// VersioneCorrente e' la versione corrente di un prodotto, come la dicono R1 e R4 (contratto rev. 3 §4.2, §6.4):
// l'ultima conferma (nessuna: numero 0, conferma_id null), il contatore struttura della RFQ, la firma del perimetro
// ricalcolata adesso (firma-perimetro-2) e la fonte autorizzata. E' la base che una bozza nuova dichiara.
type VersioneCorrente struct {
	ConfermaID       *uuid.UUID        `json:"conferma_id"`
	Numero           int32             `json:"numero"`
	Contatore        int64             `json:"contatore"`
	FirmaStruttura   string            `json:"firma_struttura"`
	FonteAutorizzata *FonteAutorizzata `json:"fonte_autorizzata"`
}

// StatoRFQ e' lo stato della RFQ che firma, validita' e risoluzione delle identita' leggono: tutti i componenti
// (archiviati compresi), tutti gli archi della working, le testate delle conferme di ogni prodotto, il contatore e le
// fonti autorizzate. Si legge con LeggiStatoRFQ; i metodi sono puri.
type StatoRFQ struct {
	Thread        uuid.UUID
	Contatore     int64 // working_contatore.struttura (riga mancante = 0)
	ContatoreDati int64 // working_contatore.dati
	Componenti    []Componente
	Archi         []Arco

	perID   map[uuid.UUID]Componente
	testate map[uuid.UUID][]TestataConferma // per prodotto, per numero crescente
	fonti   map[uuid.UUID]FonteAutorizzata
}

// LeggiStatoRFQ legge lo stato della RFQ con le query di p4_bozza_v2.sql e GetWorkingContatore (p4_conferme.sql).
// Non controlla che la RFQ esista: chi chiama lo ha gia' fatto (BloccaThread, GetThread).
func LeggiStatoRFQ(ctx context.Context, q *db.Queries, thread uuid.UUID) (*StatoRFQ, error) {
	comp, err := q.ListComponentiStatoRfq(ctx, thread)
	if err != nil {
		return nil, fmt.Errorf("confermabom: componenti della RFQ: %w", err)
	}
	archi, err := q.ListArchiStatoRfq(ctx, thread)
	if err != nil {
		return nil, fmt.Errorf("confermabom: archi della RFQ: %w", err)
	}
	conf, err := q.ListConfermeStatoRfq(ctx, thread)
	if err != nil {
		return nil, fmt.Errorf("confermabom: conferme della RFQ: %w", err)
	}
	fonti, err := q.ListFontiAutorizzateRfq(ctx, thread)
	if err != nil {
		return nil, fmt.Errorf("confermabom: fonti autorizzate della RFQ: %w", err)
	}
	wc, err := q.GetWorkingContatore(ctx, thread)
	if err != nil {
		return nil, fmt.Errorf("confermabom: contatore della RFQ: %w", err)
	}
	s := &StatoRFQ{Thread: thread, Contatore: wc.Struttura, ContatoreDati: wc.Dati}
	for _, c := range comp {
		s.Componenti = append(s.Componenti, Componente{ID: c.ComponenteID, CodiceLetterale: c.Codice, Tipo: string(c.Tipo),
			Archiviato: c.ArchiviatoIl != nil, StepStrutturale: c.StepStrutturaleID, Rev: c.Rev.String, Descrizione: c.Descrizione.String})
	}
	for _, a := range archi {
		s.Archi = append(s.Archi, Arco{Padre: a.PadreID, Figlio: a.FiglioID, Qta: a.Qta, Posizione: a.Posizione.String})
	}
	s.indicizza()
	for _, c := range conf {
		s.testate[c.ProdottoID] = append(s.testate[c.ProdottoID], TestataConferma{
			Conferma: Conferma{ID: c.ConfermaID, Numero: c.Numero, Contatore: c.Contatore, FirmaStruttura: strings.TrimSpace(c.FirmaStruttura)},
			Prodotto: c.ProdottoID, Formato: c.Formato, MonoPezzo: c.MonoPezzo, Criterio: c.CriterioVerifica})
	}
	for _, f := range fonti {
		x := FonteAutorizzata{DocumentoID: f.DocumentoID, Sha256: strings.TrimSpace(f.Sha256), Superata: f.Superata}
		if f.AllegatoID.Valid {
			a := f.AllegatoID.UUID
			x.AllegatoID = &a
		}
		s.fonti[f.ComponenteID] = x
	}
	return s, nil
}

// NuovoStatoRFQ costruisce uno stato della RFQ in memoria (le prove L1 e chi ha gia' le righe): conferme in qualunque
// ordine, fonti per finito.
func NuovoStatoRFQ(thread uuid.UUID, contatore int64, componenti []Componente, archi []Arco, conferme []TestataConferma,
	fonti map[uuid.UUID]FonteAutorizzata) *StatoRFQ {
	s := &StatoRFQ{Thread: thread, Contatore: contatore, Componenti: componenti, Archi: archi}
	s.indicizza()
	for _, c := range conferme {
		s.testate[c.Prodotto] = append(s.testate[c.Prodotto], c)
	}
	for p := range s.testate {
		t := s.testate[p]
		for i := 1; i < len(t); i++ {
			for j := i; j > 0 && t[j].Numero < t[j-1].Numero; j-- {
				t[j], t[j-1] = t[j-1], t[j]
			}
		}
	}
	for k, v := range fonti {
		s.fonti[k] = v
	}
	return s
}

func (s *StatoRFQ) indicizza() {
	s.perID = make(map[uuid.UUID]Componente, len(s.Componenti))
	for _, c := range s.Componenti {
		s.perID[c.ID] = c
	}
	s.testate = map[uuid.UUID][]TestataConferma{}
	s.fonti = map[uuid.UUID]FonteAutorizzata{}
}

// Componente restituisce il componente della RFQ con quell'id.
func (s *StatoRFQ) Componente(id uuid.UUID) (Componente, bool) {
	c, ok := s.perID[id]
	return c, ok
}

// Perimetro e' il perimetro del prodotto sulla working di adesso (CalcolaPerimetro).
func (s *StatoRFQ) Perimetro(prodotto uuid.UUID) (Perimetro, error) {
	return CalcolaPerimetro(prodotto, s.Componenti, s.Archi)
}

// Firma e' la firma del perimetro del prodotto adesso (firma-perimetro-2, codice letterale: E3.1).
func (s *StatoRFQ) Firma(prodotto uuid.UUID) (string, error) {
	return FirmaPerimetro(prodotto, s.Componenti, s.Archi)
}

// Testate sono le conferme del prodotto, dalla prima.
func (s *StatoRFQ) Testate(prodotto uuid.UUID) []TestataConferma {
	return s.testate[prodotto]
}

// Conferme sono le conferme del prodotto nella forma di validita.go.
func (s *StatoRFQ) Conferme(prodotto uuid.UUID) []Conferma {
	t := s.testate[prodotto]
	out := make([]Conferma, 0, len(t))
	for _, c := range t {
		out = append(out, c.Conferma)
	}
	return out
}

// Ultima e' l'ultima conferma del prodotto (numero piu' alto); ok falso senza conferme.
func (s *StatoRFQ) Ultima(prodotto uuid.UUID) (TestataConferma, bool) {
	t := s.testate[prodotto]
	if len(t) == 0 {
		return TestataConferma{}, false
	}
	return t[len(t)-1], true
}

// Working e' lo stato corrente per il prodotto: il contatore della RFQ e la firma del perimetro ricalcolata.
func (s *StatoRFQ) Working(prodotto uuid.UUID) (Working, error) {
	f, err := s.Firma(prodotto)
	if err != nil {
		return Working{}, err
	}
	return Working{Contatore: s.Contatore, Firma: f}, nil
}

// ValiditaUltima e' l'ultima conferma del prodotto con la sua validita' (ultima e, contatore uguale oppure firma
// uguale: §5.1); ok falso senza conferme.
func (s *StatoRFQ) ValiditaUltima(prodotto uuid.UUID) (TestataConferma, Validita, bool, error) {
	u, ok := s.Ultima(prodotto)
	if !ok {
		return TestataConferma{}, Validita{}, false, nil
	}
	w, err := s.Working(prodotto)
	if err != nil {
		return TestataConferma{}, Validita{}, false, err
	}
	v, err := ConfermaValida(u.Conferma, s.Conferme(prodotto), w)
	if err != nil {
		return TestataConferma{}, Validita{}, false, err
	}
	return u, v, true, nil
}

// FonteAutorizzata e' la fonte autorizzata del prodotto, o nil.
func (s *StatoRFQ) FonteAutorizzata(prodotto uuid.UUID) *FonteAutorizzata {
	f, ok := s.fonti[prodotto]
	if !ok {
		return nil
	}
	return &f
}

// VersioneCorrente e' la versione corrente del prodotto (contratto rev. 3 §4.2, §6.4).
func (s *StatoRFQ) VersioneCorrente(prodotto uuid.UUID) (VersioneCorrente, error) {
	f, err := s.Firma(prodotto)
	if err != nil {
		return VersioneCorrente{}, err
	}
	v := VersioneCorrente{Contatore: s.Contatore, FirmaStruttura: f, FonteAutorizzata: s.FonteAutorizzata(prodotto)}
	if u, ok := s.Ultima(prodotto); ok {
		id := u.ID
		v.ConfermaID, v.Numero = &id, u.Numero
	}
	return v, nil
}

// ControllaBase e' il controllo della base di una bozza del prodotto (ControllaBase di validita.go, con la firma del
// perimetro ricalcolata adesso): nessun motivo = la base vale; altrimenti conferma_cambiata e/o struttura_cambiata.
func (s *StatoRFQ) ControllaBase(prodotto uuid.UUID, b Base) ([]string, error) {
	w, err := s.Working(prodotto)
	if err != nil {
		return nil, err
	}
	return ControllaBase(b, s.Conferme(prodotto), w)
}
