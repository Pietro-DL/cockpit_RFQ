package motore_a

// dati_v1.go: le aggiunte di V1 al modello della pagina (/motore-a/dati; emendamento di V1 §9.9, §6.4.1; PE-01, PE-10,
// PE-15, PE-16, PE-28 dell'interfaccia, accolte nelle decisioni dell'ondata A; piano della 0023 §14 passo 3).
//
// Le letture nuove stanno nello stesso snapshot REPEATABLE READ READ ONLY della fotografia (leggiV1): le occorrenze non
// attive (ListOccorrenzeNonAttiveRfq, la lettura unica del ciclo dei file, la stessa di F8: decisioni dell'ondata B
// §5.1), le bozze aperte formato 3 con le colonne del loro registro (ListSorgentiBozzeAperte, mai
// il contenuto), lo stato di F8 (GetStatoF8DelThread) e l'identità certificata dei componenti (ListIdentitaRegistrate).
// La composizione (componiV1) è pura: i gruppi del prodotto con l'impronta_contenuto di un'importazione fatta adesso, i
// nodi con la completezza e le diagnostiche correnti e l'identità registrata (propostabom.Modello, lo stesso adattatore
// di R2 e di S6), la fonte del prodotto, lo STEP secondario, il motivo della mancanza di gruppi, la bozza del prodotto con
// il gruppo corrente della sorgente (propostabom.GruppoCorrenteDelProdotto, sui gruppi senza le occorrenze non attive).
// Nessuna scrittura: è una GET (D-18).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// DatiV1: ciò che il modello di V1 legge dal database, nello snapshot della fotografia.
//   - NonAttivi: le occorrenze in «Altri file» o scartate (il ciclo dei file), per allegato con lo stato.
//   - Bozze: la bozza aperta formato 3 di ogni prodotto, con le colonne del registro della versione.
//   - F8: lo stato del rismistamento della RFQ (F8Errore, F8InAttesa, F8Eseguito, o "" senza righe).
//   - Certificazioni: l'identità certificata dall'ultima conferma formato 3 di ogni componente che ne ha una.
//   - EventiFonte, NomiUtenti: gli eventi della fonte dei prodotti (conferma, sostituzione, autorizzazione, revoca) e i
//     nomi di chi ha confermato i documenti, per la data e l'autore della fonte (U2 §2.4).
//   - Conferme: l'ultima conferma di ogni prodotto con la sua validità; Registri: i registri delle ultime conferme
//     formato 3, riletti con le loro colonne (la proposta aggiornata, U2 §5.3).
//   - LetturaBozze (direttiva 35, blocco C-4): le bozze aperte di tutti i prodotti con le PB {allegato_id, chiave_bozza},
//     gli svincoli pendenti e il vassoio «Da ricollocare», e le AR attive della RFQ dal servizio unico
//     (distintabozza.LeggiBozzeDellaRfq, la lettura della persistenza). Mai documento_proposta.componente_id come fonte
//     delle AR: le AR sono anche la fonte «manuale» del Motore A (registrateDelThread, valutazione.Ingressi.Registrate).
//   - ErroreBozze: uno snapshot di bozza che non si rilegge (distintabozza.ErrSnapshotIncoerente); allora LetturaBozze ha
//     solo le AR attive (associazioni.AttiveDellaRfq) e S9 risponde con l'errore. nil di solito.
type DatiV1 struct {
	NonAttivi      propostabom.NonAttivi
	Bozze          map[uuid.UUID]BozzaAperta
	F8             string
	Certificazioni map[uuid.UUID]propostabom.Certificazione
	EventiFonte    []db.ListEventiFonteDelThreadRow
	NomiUtenti     map[uuid.UUID]string
	Conferme       map[uuid.UUID]ConfermaUltima
	Registri       map[uuid.UUID]importazione.Contenuto
	LetturaBozze   distintabozza.BozzeDellaRfq
	ErroreBozze    error
	// SorgentiS8 (direttiva 39 §2): per prodotto, l'ultima conferma di S8 (conferma_documento con la chiave del passo 2,
	// R39-09) con la sua validità per R5, letta con confermabom.SorgenteValida (la stessa funzione del gate di R4 e R5).
	// Solo i prodotti con almeno una conferma di S8: una conferma di S8 più recente di un altro STEP supera le precedenti,
	// quindi solo l'ultima può essere valida.
	SorgentiS8 map[uuid.UUID]SorgenteS8
	// Suggerimenti (direttiva 39 §4, D-PB02 A [U]; giro G39, G39-01 [U]): per prodotto con la bozza aperta formato 3, i
	// suggerimenti dopo l'apertura di R1 con ?suggerimenti_motore=1 (distintabozza.SuggerimentiDellaBozzaApertaCon con il
	// calcolo di adesso, la stessa funzione), che S9 mostra in preassociazioni_proposte[]: una sola sorgente e una sola
	// forma per S9 e R1 [T-39]. Non li scrive il caricatore: li compone S9 dopo il calcolo (Servizio.suggerimentiDiAdesso,
	// suggerimenti_adesso.go), perché il calcolo di adesso è l'esito del motore.
	Suggerimenti map[uuid.UUID][]distintabozza.PreassociazioneProposta3
}

// SorgenteS8: l'ultima conferma di S8 di un prodotto: l'allegato e lo sha256 confermati e la validità per R5.
type SorgenteS8 struct {
	AllegatoID uuid.UUID
	Sha256     string
	Stato      confermabom.StatoSorgente
}

// BozzaAperta: la testata di una bozza aperta formato 3 e le colonne del registro della sua versione.
type BozzaAperta struct {
	BozzaID        uuid.UUID
	Versione       int32
	FonteAllegato  *uuid.UUID
	FonteSha256    string
	FonteGruppo    string
	ConRegistro    bool
	RegistroGruppo string
	FonteRadice    string
}

// Gli stati di F8 della RFQ nel modello (f8_stato, 0022): errore se una riga è in errore, in_attesa se una è in attesa,
// eseguito se tutte sono eseguite; "" senza righe (F8 mai previsto).
const (
	F8Errore   = "errore"
	F8InAttesa = "in_attesa"
	F8Eseguito = "eseguito"
)

// leggiV1: le letture di V1 sulla transazione dello snapshot, dopo la fotografia (che dà i componenti del thread).
func leggiV1(ctx context.Context, q *db.Queries, thread uuid.UUID, f fotorfq.Fotografia) (DatiV1, error) {
	out := DatiV1{NonAttivi: propostabom.NonAttivi{}, Bozze: map[uuid.UUID]BozzaAperta{}, Certificazioni: map[uuid.UUID]propostabom.Certificazione{},
		NomiUtenti: map[uuid.UUID]string{}, Conferme: map[uuid.UUID]ConfermaUltima{}, Registri: map[uuid.UUID]importazione.Contenuto{},
		SorgentiS8: map[uuid.UUID]SorgenteS8{}, Suggerimenti: map[uuid.UUID][]distintabozza.PreassociazioneProposta3{}}
	na, err := q.ListOccorrenzeNonAttiveRfq(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("%w: le occorrenze non attive non si leggono: %w", ErrLettura, err)
	}
	for _, r := range na {
		out.NonAttivi[r.AllegatoID] = r.Stato
	}
	bb, err := q.ListSorgentiBozzeAperte(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("%w: le bozze aperte non si leggono: %w", ErrLettura, err)
	}
	for _, r := range bb {
		if _, ok := out.Bozze[r.ProdottoID]; ok {
			continue // una bozza aperta per prodotto (il trigger della 0022): la prima, in ordine di bozza
		}
		b := BozzaAperta{BozzaID: r.BozzaID, Versione: r.Versione, FonteSha256: r.FonteSha256.String, FonteGruppo: r.FonteGruppo.String,
			ConRegistro: r.ImportazioneID.Valid, RegistroGruppo: r.RegistroGruppo.String, FonteRadice: r.RegistroFonteRadice.String}
		if r.FonteAllegatoID.Valid {
			a := r.FonteAllegatoID.UUID
			b.FonteAllegato = &a
		}
		out.Bozze[r.ProdottoID] = b
	}
	f8, err := q.GetStatoF8DelThread(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("%w: lo stato di F8 non si legge: %w", ErrLettura, err)
	}
	switch {
	case f8.InErrore > 0:
		out.F8 = F8Errore
	case f8.InAttesa > 0:
		out.F8 = F8InAttesa
	case f8.Totale > 0:
		out.F8 = F8Eseguito
	}
	var componenti []uuid.UUID
	for _, t := range f.Thread {
		if t.ID != thread {
			continue
		}
		for _, c := range t.Componenti {
			componenti = append(componenti, c.ID)
		}
	}
	if len(componenti) > 0 {
		cc, err := q.ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: thread, Componenti: componenti})
		if err != nil {
			return out, fmt.Errorf("%w: le identità certificate non si leggono: %w", ErrLettura, err)
		}
		for _, r := range cc {
			out.Certificazioni[r.ComponenteID] = propostabom.Certificazione{Codice: r.Codice, Emittente: r.Emittente.String,
				EmittentePresunto: r.EmittentePresunto.Bool, Namespace: r.Namespace.String}
		}
	}
	if err := leggiFonteEConferme(ctx, q, thread, f, &out); err != nil {
		return out, err
	}
	// direttiva 35, blocco C-4: le bozze di tutti i prodotti e le AR attive (la lettura della persistenza, solo SELECT, sulla
	// stessa transazione). Uno snapshot di bozza illeggibile (ErrSnapshotIncoerente, come in R1) non ferma /dati né il
	// calcolo: le AR si rileggono dal servizio unico (la fonte «manuale» del Motore A non si perde in silenzio) e S9, che
	// delle bozze ha bisogno, risponde con l'errore (ErroreBozze). Un errore del database ferma la lettura.
	lb, err := distintabozza.LeggiBozzeDellaRfq(ctx, q, thread)
	if errors.Is(err, distintabozza.ErrSnapshotIncoerente) {
		out.ErroreBozze = err
		ar, errAR := associazioni.AttiveDellaRfq(ctx, q, thread)
		if errAR != nil {
			return out, fmt.Errorf("%w: le associazioni registrate non si leggono: %w", ErrLettura, errAR)
		}
		lb, err = distintabozza.BozzeDellaRfq{Bozze: []distintabozza.BozzaDellaRfq{}, Associazioni: ar}, nil
	}
	if err != nil {
		return out, fmt.Errorf("%w: le bozze dei prodotti e le associazioni registrate non si leggono: %w", ErrLettura, err)
	}
	out.LetturaBozze = lb
	// direttiva 39 §4 e giro G39 (G39-01 [U]): i suggerimenti dopo l'apertura li compone S9 dopo il calcolo, con il calcolo
	// di adesso (Servizio.suggerimentiDiAdesso, campo Suggerimenti)
	return out, nil
}

// registrateDelThread: le AR attive del thread per valutazione.Ingressi.Registrate (la fonte «manuale» del Motore A, con
// il ripiego per le copie senza AR in valutazione/registrate.go): allegato → componente. nil senza AR, così l'esito
// senza AR è quello di prima.
func registrateDelThread(thread uuid.UUID, v DatiV1) map[uuid.UUID]map[uuid.UUID]uuid.UUID {
	if len(v.LetturaBozze.Associazioni) == 0 {
		return nil
	}
	ar := make(map[uuid.UUID]uuid.UUID, len(v.LetturaBozze.Associazioni))
	for _, a := range v.LetturaBozze.Associazioni {
		if a.Attiva() && a.Thread == thread {
			ar[a.Allegato] = a.Componente
		}
	}
	if len(ar) == 0 {
		return nil
	}
	return map[uuid.UUID]map[uuid.UUID]uuid.UUID{thread: ar}
}

// FonteDelProdotto: la fonte del prodotto nel modello (PE-01; EM §9.9 «per la fonte: conferma_ammessa con il motivo, e
// lo STEP secondario»): lo STEP che la pagina guarda per gli atti della fonte, il documento, se la conferma è ammessa e
// perché no, se il documento è confermato e autorizzato, e lo STEP secondario (PE-15).
//
// Direttiva 39 §2, §3 [U + T-39]: «Conferma la fonte» è S8 (gesto conferma_sorgente, «Conferma questo STEP come
// sorgente di ‹prodotto›»), conferma la fonte della BOZZA e non dipende più dal legame della proposta dello STEP con il
// prodotto (documento_proposta.componente_id, la copia derivata): conferma_ammessa dice se S8 si può chiedere su questo
// STEP. sorgente_confermata dice se lo STEP è la sorgente valida per R5 (l'ultima conferma di S8, non revocata, non
// rifiutata, non superata: confermabom.SorgenteValida); sorgente_s8 è l'ultima conferma di S8 del prodotto con la sua
// validità e il motivo; legame_storico è lo STEP storico univoco di D-AR04 (iii) (un documento cad_3d STEP del prodotto,
// confermato e non sostituito, l'unico, per un prodotto senza conferme di S8): si mostra distinto e non soddisfa il gate,
// l'ingegnere lo riconferma da S8. confermata resta il fatto del documento (un cad_3d corrente del prodotto con questo
// contenuto, anche confermato dalle pagine legacy) e autorizzata lo STEP della working (step_strutturale_id), che solo R5
// cambia. struttura_step dice che cosa il motore sa della struttura di questo STEP per il prodotto e la partenza che S8
// offre (propostabom.StrutturaSTEP: con_struttura, sola_radice, nessuna_struttura, non_determinabile con lo stato
// dell'analisi).
//
// Lo STEP è, in quest'ordine: la sorgente della bozza aperta del prodotto; la sorgente valida di S8; lo STEP autorizzato
// (il documento di step_strutturale_id); lo STEP del legame storico; la candidata compatibile del prodotto, se è una
// sola; il primo gruppo con uno STEP. Senza STEP la conferma non è ammessa, con il motivo. Nel JSON i soli nomi di PE-01
// (A-28) e quelli della direttiva 39: l'allegato resta per chi chiama.
type FonteDelProdotto struct {
	Voce            *uuid.UUID                  `json:"voce,omitempty"`
	AllegatoID      *uuid.UUID                  `json:"-"`
	DocumentoID     *uuid.UUID                  `json:"documento_id,omitempty"`
	Sha256          string                      `json:"sha256,omitempty"`
	ConfermaAmmessa bool                        `json:"conferma_ammessa"`
	Motivo          string                      `json:"motivo,omitempty"`
	Confermata      bool                        `json:"confermata"`
	Autorizzata     bool                        `json:"autorizzata"`
	Sorgente        string                      `json:"sorgente,omitempty"`
	StepSecondario  *propostabom.StepSecondario `json:"step_secondario,omitempty"`
	// Data e autore della conferma e dell'autorizzazione (U2 §2.4; datiDellaFonte): confermata_il e confermata_da
	// dall'ultimo evento conferma_documento del documento (prima di V1: dal documento); autorizzata_il e autorizzata_da
	// dall'ultima autorizzazione non seguita da una revoca. Un'autorizzazione senza evento dice
	// autorizzata_motivo = non_registrato: si dichiara che manca, non si ricostruisce ([UR] Q13).
	ConfermataIl      *time.Time `json:"confermata_il,omitempty"`
	ConfermataDa      *Persona   `json:"confermata_da,omitempty"`
	AutorizzataIl     *time.Time `json:"autorizzata_il,omitempty"`
	AutorizzataDa     *Persona   `json:"autorizzata_da,omitempty"`
	AutorizzataMotivo string     `json:"autorizzata_motivo,omitempty"`
	// direttiva 39 (commento del tipo)
	Gesto              string                     `json:"gesto,omitempty"`
	SorgenteConfermata bool                       `json:"sorgente_confermata"`
	SorgenteS8         *SorgenteS8DellaFonte      `json:"sorgente_s8,omitempty"`
	LegameStorico      *LegameStorico             `json:"legame_storico,omitempty"`
	StrutturaSTEP      *propostabom.StrutturaSTEP `json:"struttura_step,omitempty"`
	// ProposteSorgente (giro G39, G39-02 [U]): gli STEP che il Motore A propone come sorgente del prodotto, prima di S8
	// (proposta_sorgente.go); vuoto se nessuno.
	ProposteSorgente []PropostaSorgente `json:"proposte_sorgente,omitempty"`
}

// SorgenteS8DellaFonte: l'ultima conferma di S8 del prodotto nella fonte: lo STEP, il documento, se vale per R5 e, se no,
// perché (revocata, rifiutata, superata, non_coerente), con data e autore della conferma.
type SorgenteS8DellaFonte struct {
	AllegatoID   uuid.UUID  `json:"allegato_id"`
	Sha256       string     `json:"sha256"`
	DocumentoID  *uuid.UUID `json:"documento_id,omitempty"`
	Valida       bool       `json:"valida"`
	Motivo       string     `json:"motivo,omitempty"`
	ConfermataIl *time.Time `json:"confermata_il,omitempty"`
	ConfermataDa *uuid.UUID `json:"confermata_da,omitempty"`
}

// LegameStorico: lo STEP storico univoco di D-AR04 (iii) [U]: il documento, il suo contenuto e il primo allegato da cui è
// nato. Niente date o autori inventati (non c'è un evento di S8).
type LegameStorico struct {
	DocumentoID uuid.UUID  `json:"documento_id"`
	Sha256      string     `json:"sha256"`
	AllegatoID  *uuid.UUID `json:"allegato_id,omitempty"`
}

// I motivi di una conferma della fonte non ammessa. MotivoFonteSenzaComponente: il prodotto non ha un componente della
// RFQ (lo stesso testo del motivo della fonte prodotto_senza_componente). Per lo STEP del prodotto in «Altri file» o
// scartato (il modello lo lascia fuori dai gruppi, EM §9.9) il motivo è propostabom.TestoStepNonAttivo con lo stato:
// «Riattiva» per «Altri file», «ricaricalo» per uno scartato (DEC-B B-3).
//
// Direttiva 39: i motivi legati alla proposta dello STEP («senza proposta aperta», «non è un 3D», «non è associato al
// prodotto») non ci sono più: S8 non parte dalla proposta né dal suo legame. MotivoFonteGiaConfermata vale per la
// sorgente valida di S8; MotivoFonteDocumentoAltrove per lo STEP che è già il documento di un altro componente (S8
// risponderebbe STEP_SORGENTE_DI_ALTRO_PRODOTTO o GIA_DOCUMENTO); MotivoFonteSostituito per un contenuto già sostituito.
const (
	MotivoFonteSenzaSTEP        = "il prodotto non ha uno STEP fra i gruppi del motore"
	MotivoFonteSenzaComponente  = "il prodotto non ha un componente della RFQ"
	MotivoFonteGiaConfermata    = "lo STEP è già la sorgente confermata del prodotto"
	MotivoFonteDocumentoAltrove = "lo STEP è già un documento di un altro componente: non diventa la sorgente di questo prodotto"
	MotivoFonteSostituito       = "lo STEP è un documento già sostituito da una revisione più recente"
	GestoConfermaSorgente       = "conferma_sorgente"
	tipoDocumentoCad3D          = "cad_3d"
)

// BozzaDelProdotto: la bozza aperta formato 3 del prodotto con un registro e il gruppo corrente della sorgente (EM §6.4.1,
// §9.9): con assente il motivo è nessun_gruppo o occorrenza_non_attiva, perché una bozza con un registro ha sempre la
// radice. Per una bozza senza registro il gruppo corrente lo dicono R1 ?confronto=1 e riallinea_proposta: qui non c'è.
type BozzaDelProdotto struct {
	BozzaID        uuid.UUID                  `json:"bozza_id"`
	Versione       int32                      `json:"versione"`
	GruppoCorrente propostabom.GruppoCorrente `json:"gruppo_corrente"`
}

// componiV1: le aggiunte di V1 alla pagina del prodotto scelto.
func componiV1(p *Pagina, r *Risultato) {
	et := r.EsitoThread
	var pid *uuid.UUID
	for i := range et.ProdottiValutati {
		if et.ProdottiValutati[i].Rif == p.Prodotto {
			pid = et.ProdottiValutati[i].ComponenteID
		}
	}
	componenti := map[uuid.UUID]propostabom.ComponenteDellaWorking{}
	for _, c := range r.Thread.Componenti {
		k := propostabom.ComponenteDellaWorking{Codice: c.Codice, Archiviato: c.ArchiviatoIl != nil}
		if c.Rev != nil {
			k.Rev = *c.Rev
		}
		componenti[c.ID] = k
	}
	m := propostabom.Modello(propostabom.IngressoModello{Esito: &r.Esito, Thread: et, Regole: r.Regole, Prodotto: p.Prodotto,
		NonAttivi: r.V1.NonAttivi, Componenti: componenti, Certificazioni: r.V1.Certificazioni, Foto: &r.Foto, Insieme: r.Insieme})
	p.F8Stato, p.Modello = r.V1.F8, m
	if m == nil {
		p.Gruppi = []propostabom.GruppoDelModello{}
		return
	}
	p.Gruppi, p.MotivoSenzaGruppi = m.Gruppi, m.MotivoSenzaGruppi
	if pid != nil {
		if b, ok := r.V1.Bozze[*pid]; ok && b.ConRegistro && b.FonteAllegato != nil {
			for i := range et.ProdottiValutati {
				if pv := et.ProdottiValutati[i]; pv.Rif == p.Prodotto {
					gc := propostabom.GruppoCorrenteDelProdotto(pv, et.Ancoraggi.Strutture, r.V1.NonAttivi,
						propostabom.SorgenteBozza{AllegatoID: *b.FonteAllegato, Sha256: b.FonteSha256, Radice: b.FonteRadice, Gruppo: b.FonteGruppo})
					p.BozzaDelProdotto = &BozzaDelProdotto{BozzaID: b.BozzaID, Versione: b.Versione, GruppoCorrente: gc}
				}
			}
		}
	}
	p.Fonte = fonteDelProdotto(r, p, pid, m)
	if pid != nil {
		// giro G39 (G39-02 [U]): lo STEP candidato del prodotto finito, letto con il Motore A, come proposta di sorgente
		p.Fonte.ProposteSorgente = proposteSorgente(r, *pid, r.CandidatureSorgente())
		datiDellaFonte(r, *pid, p.Fonte)
		p.ConfermaCorrente = confermaCorrente(r, p, *pid, m)
	}
}

// fonteDelProdotto: la fonte del prodotto (FonteDelProdotto). Uno STEP in «Altri file» o scartato non si conferma: con lo
// STEP scelto non attivo, o senza STEP perché il modello ha lasciato fuori quelli non attivi, il motivo lo dice.
func fonteDelProdotto(r *Risultato, p *Pagina, pid *uuid.UUID, m *propostabom.ModelloDelProdotto) *FonteDelProdotto {
	out := &FonteDelProdotto{StepSecondario: m.StepSecondario}
	if pid == nil {
		out.Motivo = MotivoFonteSenzaComponente
		return out
	}
	t := r.Thread
	var comp *fotorfq.Componente
	for i := range t.Componenti {
		if t.Componenti[i].ID == *pid {
			comp = &t.Componenti[i]
		}
	}
	var allegato *uuid.UUID
	if b, ok := r.V1.Bozze[*pid]; ok && b.FonteAllegato != nil {
		allegato = b.FonteAllegato
	}
	s8, conS8 := r.V1.SorgentiS8[*pid]
	if allegato == nil && conS8 && s8.Stato.Valida {
		a := s8.AllegatoID
		allegato = &a
	}
	if allegato == nil && comp != nil && comp.StepStrutturaleID != nil {
		for i := range t.Documenti {
			if d := &t.Documenti[i]; d.ID == *comp.StepStrutturaleID && len(d.Allegati) > 0 {
				a := d.Allegati[0]
				allegato = &a
			}
		}
	}
	if storico := legameStorico(r, *pid); !conS8 && storico != nil {
		out.LegameStorico = storico
		if allegato == nil {
			allegato = storico.AllegatoID
		}
	}
	if conS8 {
		x := &SorgenteS8DellaFonte{AllegatoID: s8.AllegatoID, Sha256: s8.Sha256, Valida: s8.Stato.Valida, Motivo: s8.Stato.Motivo}
		if s8.Stato.Documento != uuid.Nil {
			d, il, da := s8.Stato.Documento, s8.Stato.ConfermataIl, s8.Stato.ConfermataDa
			x.DocumentoID, x.ConfermataIl, x.ConfermataDa = &d, &il, &da
		}
		out.SorgenteS8 = x
	}
	if allegato == nil {
		compatibili := 0
		var primo, compatibile *uuid.UUID
		for _, g := range m.Gruppi {
			if g.AllegatoID == nil || g.Tipo == propostabom.GruppoSenzaSTEP {
				continue
			}
			if primo == nil {
				primo = g.AllegatoID
			}
			if g.Tipo == propostabom.GruppoCandidata && g.Compatibilita == propostabom.CompatibilitaCompatibile {
				compatibili++
				compatibile = g.AllegatoID
			}
		}
		allegato = primo
		if compatibili == 1 {
			allegato = compatibile
		}
	}
	if allegato == nil {
		out.Motivo = MotivoFonteSenzaSTEP
		if len(m.StepNonAttivi) > 0 {
			var stati []string
			for _, a := range m.StepNonAttivi {
				stati = append(stati, r.V1.NonAttivi[a])
			}
			out.Motivo = propostabom.TestoStepNonAttivo(stati...)
		}
		return out
	}
	a := *allegato
	out.AllegatoID = &a
	statoNonAttivo, nonAttivo := r.V1.NonAttivi[a]
	for _, f := range r.EsitoThread.File {
		if f.AllegatoID == a {
			out.Sha256 = f.Documento.Fonte.RiferimentoFatti.Sha256
		}
	}
	if out.Sha256 == "" && conS8 && s8.AllegatoID == a {
		out.Sha256 = s8.Sha256
	}
	// il documento con questo contenuto (o nato da questo allegato): un cad_3d corrente del prodotto (confermata), un
	// documento di un altro componente o del thread (S8 lo rifiuterebbe), o un documento già sostituito
	altrove, sostituito := false, false
	for i := range t.Documenti {
		d := &t.Documenti[i]
		dentro := out.Sha256 != "" && d.Sha256 == out.Sha256
		for _, x := range d.Allegati {
			dentro = dentro || x == a
		}
		switch {
		case !dentro:
		case d.SostituitoDa != nil:
			sostituito = true
		case d.ComponenteID == nil || *d.ComponenteID != *pid:
			altrove = true
		case d.Tipo == tipoDocumentoCad3D && out.DocumentoID == nil:
			id := d.ID
			out.DocumentoID, out.Confermata = &id, true
			out.Autorizzata = comp != nil && comp.StepStrutturaleID != nil && *comp.StepStrutturaleID == d.ID
		}
	}
	out.SorgenteConfermata = conS8 && s8.Stato.Valida && out.Sha256 != "" && s8.Sha256 == out.Sha256
	if p.Indice != nil {
		if au := p.Indice.AutorizzazioneDi(p.Prodotto, a); au != nil {
			out.Sorgente = au.Sorgente
		}
		out.Voce = p.Indice.PropostaAperta(a)
	}
	st := propostabom.StrutturaDelloSTEPNelModello(r.EsitoThread, m, a)
	out.StrutturaSTEP = &st
	// la conferma ammessa è quella di S8: non guarda la proposta dello STEP né il suo legame con il prodotto (direttiva 39)
	switch {
	case out.SorgenteConfermata:
		out.Motivo = MotivoFonteGiaConfermata
	case nonAttivo:
		out.Motivo = propostabom.TestoStepNonAttivo(statoNonAttivo)
	case altrove && out.DocumentoID == nil:
		out.Motivo = MotivoFonteDocumentoAltrove
	case sostituito && out.DocumentoID == nil:
		out.Motivo = MotivoFonteSostituito
	default:
		out.ConfermaAmmessa, out.Gesto = true, GestoConfermaSorgente
	}
	return out
}

// legameStorico: lo STEP storico univoco di D-AR04 (iii) [U] del prodotto: l'unico documento cad_3d STEP (.stp, .step)
// del prodotto, corrente (non sostituito); nil se non ce n'è o se ce n'è più di uno. Chi chiama lo usa solo per un
// prodotto senza conferme di S8 (R39-06: «univoco» vale per i prodotti senza eventi di S8).
func legameStorico(r *Risultato, prodotto uuid.UUID) *LegameStorico {
	if r.Thread == nil {
		return nil
	}
	var out *LegameStorico
	for i := range r.Thread.Documenti {
		d := &r.Thread.Documenti[i]
		if d.Tipo != tipoDocumentoCad3D || d.ComponenteID == nil || *d.ComponenteID != prodotto || d.SostituitoDa != nil ||
			!estensioneSTEP(d.Estensione, d.NomeFile) {
			continue
		}
		if out != nil {
			return nil // più di uno: non è univoco
		}
		out = &LegameStorico{DocumentoID: d.ID, Sha256: d.Sha256}
		if len(d.Allegati) > 0 {
			a := d.Allegati[0]
			out.AllegatoID = &a
		}
	}
	return out
}

// estensioneSTEP: il formato dichiarato del documento, o il nome del file, è uno STEP (.stp, .step).
func estensioneSTEP(estensione, nome string) bool {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(estensione), "."))
	if e == "stp" || e == "step" {
		return true
	}
	n := strings.ToLower(strings.TrimSpace(nome))
	return strings.HasSuffix(n, ".stp") || strings.HasSuffix(n, ".step")
}
