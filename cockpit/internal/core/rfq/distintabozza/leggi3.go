package distintabozza

// R1 nel formato 3 (EM §9.1; PE-02, decisione [T] dell'integratore del 18/10): la bozza aperta del prodotto e il suo
// formato effettivo, senza ricalcolare il motore e senza scrivere niente (D-18). Chi chiama apre la transazione (in sola
// lettura, REPEATABLE READ: una fotografia coerente) e la chiude con ROLLBACK.
//
// La risposta: testata e snapshot (i byte canonici salvati), formato e vocabolario espliciti, la chiave dell'ultimo
// salvataggio, il registro della versione (con ?importazione=1 il contenuto), i registri citati, n_eventi sempre (il
// numero degli eventi fino alla versione corrente) ed eventi[] con ?eventi=1, la versione corrente del prodotto, lo stato
// della base, i rif scaduti, il vassoio «Da ricollocare» e, con ?eventi=1, i da_rivedere[] dell'ultima versione costruita
// dal server. Una bozza formato 2 aperta torna com'era, con conversione {necessaria, forma: converti_da}.

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// OpzioniR1 sono i parametri di R1 (EM §9.1): ?eventi=1, ?importazione=1, ?registro=<impronta>; con
// ?suggerimenti_motore=1 l'adattatore passa il calcolo del motore di adesso (Adesso, adesso3.go: G39-01, G39-02 [U]).
type OpzioniR1 struct {
	Eventi       bool
	Importazione bool
	Registro     string
	Adesso       *CalcoloAdesso
}

// EsitoMotoreR1: il calcolo del motore di R1 con ?suggerimenti_motore=1 (G39-01 [T-39]): calcolati vero con il calcolo;
// altrimenti errore dice perche' (CALCOLO_OCCUPATO, CALCOLO_SCADUTO, ERRORE_CALCOLO: lo mette l'adattatore) e i suggerimenti
// sono solo quelli del registro della versione. Mai un 5xx per il parametro, come gli avvisi del motore di R4.
type EsitoMotoreR1 struct {
	Calcolati bool   `json:"calcolati"`
	Errore    string `json:"errore,omitempty"`
}

// StepCandidatoR1: uno STEP candidato sorgente del prodotto, non ancora confermato da S8 (G39-02 [U]): la pagina avvisa
// quando si apre o si lavora una bozza manuale o «solo prodotto» con uno STEP candidato disponibile («molto sconsigliata»),
// e propone la conferma S8. DalMotore: il riconoscimento e' del Motore A (sempre, in R1: senza il calcolo l'elenco non c'e').
type StepCandidatoR1 struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	DalMotore  bool      `json:"dal_motore"`
}

// Conversione dice che una bozza formato 2 aperta va convertita (EM §9.1, §6.7).
type Conversione struct {
	Necessaria bool   `json:"necessaria"`
	Forma      string `json:"forma"`
}

// LetturaR1 e' la risposta di R1 (EM §9.1). Con stato nessuna_bozza ci sono solo stato e versione_corrente.
type LetturaR1 struct {
	Stato                   string                        `json:"stato"`
	Formato                 int                           `json:"formato,omitempty"`
	Vocabolario             int                           `json:"vocabolario,omitempty"`
	BozzaID                 *uuid.UUID                    `json:"bozza_id,omitempty"`
	Versione                int                           `json:"versione,omitempty"`
	Snapshot                json.RawMessage               `json:"snapshot,omitempty"`
	Impronta                string                        `json:"impronta,omitempty"`
	ChiaveUltimoSalvataggio *uuid.UUID                    `json:"chiave_ultimo_salvataggio,omitempty"`
	Importazione            *ImportazioneLetta            `json:"importazione,omitempty"`
	ImportazioniCitate      []RegistroCitato              `json:"importazioni_citate,omitempty"`
	Registro                *ImportazioneLetta            `json:"registro,omitempty"`
	CreataDa                *uuid.UUID                    `json:"creata_da,omitempty"`
	CreataIl                *time.Time                    `json:"creata_il,omitempty"`
	AggiornataDa            *uuid.UUID                    `json:"aggiornata_da,omitempty"`
	AggiornataIl            *time.Time                    `json:"aggiornata_il,omitempty"`
	NEventi                 *int                          `json:"n_eventi,omitempty"`
	Eventi                  []EventoR1                    `json:"eventi,omitempty"`
	VersioneCorrente        *confermabom.VersioneCorrente `json:"versione_corrente,omitempty"`
	Base                    *StatoBase                    `json:"base,omitempty"`
	// RifScaduti e Vassoio ci sono sempre per una bozza formato 3, anche vuoti ([]); mancano per nessuna_bozza e formato 2.
	RifScaduti  *[]RifScaduto   `json:"rif_scaduti,omitempty"`
	Vassoio     *[]VoceVassoio  `json:"vassoio,omitempty"`
	DaRivedere  json.RawMessage `json:"da_rivedere,omitempty"`
	Conversione *Conversione    `json:"conversione,omitempty"`
	// PreassociazioniProposte: i suggerimenti dopo l'apertura dal registro della versione (direttiva 39 §4, D-PB02 A [U];
	// pbiniziali3.go), sempre per una bozza formato 3, anche vuoti; mai in da_rivedere[]. Con ?suggerimenti_motore=1 anche
	// quelli del calcolo di adesso per i file che il registro della versione non propone (G39-01, adesso3.go)
	PreassociazioniProposte *[]PreassociazioneProposta3 `json:"preassociazioni_proposte,omitempty"`
	// SuggerimentiMotore e StepCandidati: solo con ?suggerimenti_motore=1 (G39-01, G39-02), anche per nessuna_bozza
	SuggerimentiMotore *EsitoMotoreR1     `json:"suggerimenti_motore,omitempty"`
	StepCandidati      *[]StepCandidatoR1 `json:"step_candidati,omitempty"`
}

// EventoR1 e' un evento del giornale come lo da' R1 ?eventi=1 (EM §9.1 e §9.2: «nel giornale la chiave si chiama
// chiave», come nel C3, C3:752-753; RIC-P6 della corsia contratto): i campi di EventoLetto, con la chiave dell'evento nel
// campo chiave e l'autore in autore. La pagina li legge cosi' (distinta_unica.mjs: eventoAzione, da_rivedere, autoreDi).
type EventoR1 struct {
	N           int             `json:"n"`
	Versione    int             `json:"versione"`
	Vocabolario int             `json:"vocabolario"`
	Azione      string          `json:"azione"`
	Chiave      string          `json:"chiave,omitempty"`
	Prima       json.RawMessage `json:"prima,omitempty"`
	Dopo        json.RawMessage `json:"dopo,omitempty"`
	Origine     string          `json:"origine"`
	Motivo      string          `json:"motivo,omitempty"`
	Autore      uuid.UUID       `json:"autore"`
	Quando      time.Time       `json:"quando"`
}

// EventiR1 porta gli eventi letti nella forma di R1.
func EventiR1(ee []EventoLetto) []EventoR1 {
	out := make([]EventoR1, 0, len(ee))
	for _, e := range ee {
		out = append(out, EventoR1{N: e.N, Versione: e.Versione, Vocabolario: e.Vocabolario, Azione: e.Azione, Chiave: e.ChiaveBozza,
			Prima: e.Prima, Dopo: e.Dopo, Origine: e.Origine, Motivo: e.Motivo, Autore: e.UtenteID, Quando: e.Quando})
	}
	return out
}

// LeggiR1 legge la bozza aperta di un prodotto della RFQ nel formato effettivo (commento di testa). Errori: la RFQ che
// non c'e' (RFQ_INESISTENTE, 404), un prodotto che non e' un finito della RFQ (FUORI_RFQ), un registro che la bozza non
// cita (FUORI_RFQ, EM §9.1 ?registro).
func LeggiR1(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, op OpzioniR1) (LetturaR1, error) {
	if _, err := q.GetThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LetturaR1{}, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return LetturaR1{}, err
	}
	if _, err := prodottoDellaRfq(ctx, q, thread, prodotto, false); err != nil {
		return LetturaR1{}, err
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return LetturaR1{}, err
	}
	vc, err := st.VersioneCorrente(prodotto)
	if err != nil {
		return LetturaR1{}, err
	}
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		if op.Registro != "" {
			return LetturaR1{}, fuori("registro", "il prodotto non ha una bozza aperta che citi il registro %s", op.Registro)
		}
		out := LetturaR1{Stato: StatoNessunaBozza, VersioneCorrente: &vc}
		if err := conIlCalcolo(ctx, q, thread, prodotto, op, &out); err != nil {
			return LetturaR1{}, err
		}
		return out, nil
	}
	if err != nil {
		return LetturaR1{}, err
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: thread, Versione: b.Versione})
	if err != nil {
		return LetturaR1{}, err
	}
	letto, err := RileggiVersione(int(b.Formato), v.Snapshot, v.Impronta)
	if err != nil {
		return LetturaR1{}, err
	}
	n, err := q.ContaDistintaBozzaEventi(ctx, db.ContaDistintaBozzaEventiParams{BozzaID: b.BozzaID, ThreadID: thread, VersioneMax: b.Versione})
	if err != nil {
		return LetturaR1{}, err
	}
	nEventi := int(n)
	id, creataDa, creataIl, aggDa, aggIl := b.BozzaID, b.CreataDa, b.CreataIl, v.SalvataDa, v.SalvataIl
	out := LetturaR1{Stato: StatoAperta, Formato: int(b.Formato), Vocabolario: VocabolarioDelFormato(int(b.Formato)), BozzaID: &id,
		Versione: int(b.Versione), Snapshot: letto.Canonico, Impronta: letto.Impronta, CreataDa: &creataDa, CreataIl: &creataIl,
		AggiornataDa: &aggDa, AggiornataIl: &aggIl, NEventi: &nEventi, VersioneCorrente: &vc}
	if v.ChiaveSalvataggio.Valid {
		k := v.ChiaveSalvataggio.UUID
		out.ChiaveUltimoSalvataggio = &k
	}
	// il giornale: sempre per il vassoio; con ?eventi=1 anche nella risposta
	ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: b.BozzaID, ThreadID: thread,
		VersioneMax: b.Versione, DopoN: 0, Limite: math.MaxInt32})
	if err != nil {
		return LetturaR1{}, err
	}
	eventi := make([]EventoLetto, 0, len(ee))
	for _, e := range ee {
		eventi = append(eventi, eventoLetto(e))
	}
	if op.Eventi {
		out.Eventi = EventiR1(eventi)
		out.DaRivedere = daRivedereDa(eventi)
	}
	if err := conIlCalcolo(ctx, q, thread, prodotto, op, &out); err != nil {
		return LetturaR1{}, err
	}
	if letto.Formato3 == nil {
		// una bozza formato 2: com'era, da convertire (la si lavora solo dopo converti_da)
		out.Conversione = &Conversione{Necessaria: true, Forma: FormaConvertiDa}
		if op.Registro != "" {
			return LetturaR1{}, fuori("registro", "una bozza formato 2 non cita registri")
		}
		return out, nil
	}
	s := *letto.Formato3
	vassoio := VassoioDa(GiornaleDa(eventi))
	out.Vassoio = &vassoio
	scaduti, err := RifScaduti(ctx, q, thread, s)
	if err != nil {
		return LetturaR1{}, err
	}
	out.RifScaduti = &scaduti
	base, err := BaseDellaBozza(ctx, q, thread, s, st, scaduti)
	if err != nil {
		return LetturaR1{}, err
	}
	out.Base = &base
	// il registro della versione e quelli citati dalle decisioni
	citati := s.RegistriCitati()
	var registri []db.ListImportazioniPerImpronteRow
	if len(citati) > 0 {
		if registri, err = q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: thread,
			ProdottoID: prodotto, Impronte: citati}); err != nil {
			return LetturaR1{}, err
		}
	}
	perImpronta := map[string]db.ListImportazioniPerImpronteRow{}
	for _, r := range registri {
		perImpronta[strings.TrimSpace(r.Impronta)] = r
		out.ImportazioniCitate = append(out.ImportazioniCitate, RegistroCitato{Impronta: strings.TrimSpace(r.Impronta), CreataIl: r.CreataIl})
	}
	proposte := []PreassociazioneProposta3{}
	var regVersione *importazione.Registro
	if s.Sorgente != nil && s.Sorgente.Importazione != "" {
		r, ok := perImpronta[s.Sorgente.Importazione]
		if !ok {
			return LetturaR1{}, ErrSnapshotIncoerente
		}
		il, err := importazioneLettaDaRiga(rigaRegistro(r), op.Importazione)
		if err != nil {
			return LetturaR1{}, err
		}
		out.Importazione = &il
		reg, _, err := RileggiImportazione(rigaRegistro(r))
		if err != nil {
			return LetturaR1{}, err
		}
		regVersione = &reg
		if proposte, err = suggerimentiDopoAperturaCon(ctx, q, thread, prodotto, s, regVersione, b.BozzaID, int(b.Versione), op.Adesso.step()); err != nil {
			return LetturaR1{}, err
		}
	}
	if op.Adesso != nil {
		dalCalcolo, err := suggerimentiDelCalcolo(ctx, q, thread, prodotto, s, regVersione, op.Adesso, b.BozzaID, int(b.Versione))
		if err != nil {
			return LetturaR1{}, err
		}
		proposte = unisciSuggerimenti(proposte, dalCalcolo)
	}
	out.PreassociazioniProposte = &proposte
	if op.Registro != "" {
		r, ok := perImpronta[op.Registro]
		if !ok {
			return LetturaR1{}, fuori("registro", "la bozza non cita il registro %s", op.Registro)
		}
		il, err := importazioneLettaDaRiga(rigaRegistro(r), true)
		if err != nil {
			return LetturaR1{}, err
		}
		out.Registro = &il
	}
	return out, nil
}

// suggerimentiDopoApertura: i suggerimenti di R1 (direttiva 39 §4, D-PB02 A [U]): le proposte univoche del registro
// della versione che la bozza non applica, con la regola delle PB iniziali (ClassificaProposte) sui dati salvati di
// adesso; quelle che alla nascita sarebbero state PB sono dopo_apertura (dopo l'apertura niente modifiche silenziose:
// si accettano con un salvataggio R2 esplicito). Senza la sorgente confermata da S8 nessuno (D-AR11 B [U]).
func suggerimentiDopoApertura(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, s Snapshot3, riga db.GetDistintaImportazioneRow,
	bozza uuid.UUID, versione int) ([]PreassociazioneProposta3, error) {
	reg, _, err := RileggiImportazione(riga)
	if err != nil {
		return nil, err
	}
	return suggerimentiDopoAperturaCon(ctx, q, thread, prodotto, s, &reg, bozza, versione, nil)
}

// suggerimentiDopoAperturaCon e' suggerimentiDopoApertura sul registro gia' riletto, con gli STEP candidati del finito
// del Motore A se ci sono (step; nil = il ripiego, G39-02).
func suggerimentiDopoAperturaCon(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, s Snapshot3, reg *importazione.Registro,
	bozza uuid.UUID, versione int, step *StepCandidati) ([]PreassociazioneProposta3, error) {
	f, err := FattiDellaRfqCon(ctx, q, thread, prodotto, s.Sorgente.Sha256, reg, bozza, versione, step)
	if err != nil {
		return nil, err
	}
	applicabili, proposte := ClassificaProposte(s, reg, f)
	for _, p := range applicabili {
		proposte = append(proposte, PreassociazioneProposta3{AllegatoID: p.AllegatoID, ChiaveBozza: p.ChiaveBozza, Provenienza: ProvenienzaMotore,
			Importazione: p.Importazione, Motivo: PropostaDopoApertura})
	}
	sort.SliceStable(proposte, func(i, j int) bool { return proposte[i].AllegatoID.String() < proposte[j].AllegatoID.String() })
	return proposte, nil
}

// suggerimentiDelCalcolo (G39-01 [U], adesso3.go): i suggerimenti del registro del calcolo di adesso per i file che il
// registro della versione non propone in modo univoco, con la regola delle PB iniziali (ClassificaProposte: solo le
// proposte univoche, con la sorgente S8 valida, niente STEP candidato del finito): quelle che alla nascita sarebbero PB
// sono dopo_apertura, con importazione = l'impronta del registro di adesso (non ancora salvato: lo scrive R2 quando il
// suggerimento si accetta). Il registro di adesso deve avere come fonte la sorgente della bozza; lo stesso registro della
// versione non da' niente di nuovo.
func suggerimentiDelCalcolo(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, s Snapshot3, versione *importazione.Registro,
	adesso *CalcoloAdesso, bozza uuid.UUID, nver int) ([]PreassociazioneProposta3, error) {
	reg := adesso.registro()
	if reg == nil || s.Sorgente == nil || s.Sorgente.AllegatoID == nil || strings.TrimSpace(s.Sorgente.Sha256) == "" ||
		reg.Contenuto.Proposta.Fonte.AllegatoID != s.Sorgente.AllegatoID.String() {
		return nil, nil
	}
	if versione != nil && versione.Impronta == reg.Impronta {
		return nil, nil
	}
	gia := map[uuid.UUID]bool{}
	if versione != nil {
		for a, rif := range ProposteDelRegistro(versione.Contenuto) {
			gia[a] = len(rif) == 1
		}
	}
	f, err := FattiDellaRfqCon(ctx, q, thread, prodotto, s.Sorgente.Sha256, reg, bozza, nver, adesso.step())
	if err != nil {
		return nil, err
	}
	applicabili, proposte := ClassificaProposte(s, reg, f)
	out := []PreassociazioneProposta3{}
	for _, p := range applicabili {
		if !gia[p.AllegatoID] {
			out = append(out, PreassociazioneProposta3{AllegatoID: p.AllegatoID, ChiaveBozza: p.ChiaveBozza, Provenienza: ProvenienzaMotore,
				Importazione: p.Importazione, Motivo: PropostaDopoApertura})
		}
	}
	for _, p := range proposte {
		if !gia[p.AllegatoID] {
			out = append(out, p)
		}
	}
	return out, nil
}

// unisciSuggerimenti: i suggerimenti del registro della versione e, per gli allegati che non ci sono, quelli del calcolo di
// adesso, in ordine di allegato.
func unisciSuggerimenti(versione, adesso []PreassociazioneProposta3) []PreassociazioneProposta3 {
	visti := map[uuid.UUID]bool{}
	out := append([]PreassociazioneProposta3{}, versione...)
	for _, p := range versione {
		visti[p.AllegatoID] = true
	}
	for _, p := range adesso {
		if !visti[p.AllegatoID] {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AllegatoID.String() < out[j].AllegatoID.String() })
	return out
}

// conIlCalcolo: con ?suggerimenti_motore=1 (op.Adesso) l'esito del calcolo e gli STEP candidati sorgente del prodotto non
// ancora confermati da S8 (G39-02 [U]), per l'avviso della pagina sulla bozza manuale o «solo prodotto».
func conIlCalcolo(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, op OpzioniR1, out *LetturaR1) error {
	if op.Adesso == nil {
		return nil
	}
	out.SuggerimentiMotore = &EsitoMotoreR1{Calcolati: true}
	elenco := []StepCandidatoR1{}
	out.StepCandidati = &elenco
	step := op.Adesso.step()
	if step == nil || !step.DalMotore {
		return nil
	}
	cand, err := StepCandidatiDelFinito(ctx, q, thread, step.DelProdotto(prodotto), step)
	if err != nil {
		return err
	}
	ids := cand.DelProdotto(prodotto)
	if len(ids) == 0 {
		return nil
	}
	rr, err := q.ListAllegatiPreassociabili(ctx, db.ListAllegatiPreassociabiliParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return err
	}
	for _, r := range rr {
		if r.DellaRfq && r.Attivo && !r.LegatoComeSorgente {
			elenco = append(elenco, StepCandidatoR1{AllegatoID: r.AllegatoID, DalMotore: true})
		}
	}
	out.StepCandidati = &elenco
	return nil
}

// FormaConvertiDa: la forma di R2 che converte una bozza formato 2 aperta (EM §6.7).
const FormaConvertiDa = "converti_da"

// rigaRegistro: una riga di ListImportazioniPerImpronte nella forma di GetDistintaImportazione (le stesse colonne).
func rigaRegistro(r db.ListImportazioniPerImpronteRow) db.GetDistintaImportazioneRow {
	return db.GetDistintaImportazioneRow(r)
}

func importazioneLettaDaRiga(r db.GetDistintaImportazioneRow, conContenuto bool) (ImportazioneLetta, error) {
	reg, t, err := RileggiImportazione(r)
	if err != nil {
		return ImportazioneLetta{}, err
	}
	return ImportazioneLettaDa(reg, t, conContenuto), nil
}

// daRivedereDa: i da_rivedere[] dell'ultima versione costruita dal server (EM §9.1; MAT §5.1): l'elenco che porta il dopo
// di proposta_riallineata o sorgente_cambiata. Nessuna di queste versioni: niente. Se l'evento non lo porta intero (il
// limite di 128 KiB dell'evento), l'elenco si ricalcola dai dati salvati con la funzione delle forme costruite dal server,
// che arrivano con l'ondata C (adattatore): fino ad allora resta quello dell'evento.
func daRivedereDa(eventi []EventoLetto) json.RawMessage {
	for i := len(eventi) - 1; i >= 0; i-- {
		e := eventi[i]
		switch e.Azione {
		case AzBozzaAperta:
			return nil
		case AzPropostaRiallineata, AzSorgenteCambiata:
			var d struct {
				DaRivedere json.RawMessage `json:"da_rivedere"`
			}
			if json.Unmarshal(e.Dopo, &d) != nil || nullo(d.DaRivedere) {
				return json.RawMessage("[]")
			}
			return canonico(d.DaRivedere)
		}
	}
	return nil
}

// assicura che importazione resti importato anche se le funzioni di lettura del registro cambiano file.
var _ = importazione.FormatoRegistro
