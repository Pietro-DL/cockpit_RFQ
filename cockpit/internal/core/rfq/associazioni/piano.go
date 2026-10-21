package associazioni

// piano.go: il Piano del servizio unico (EM App. H.7; piano delle dipendenze della direttiva 35, §3.6 [T]; direttiva 39 §4,
// §5, §6): che cosa farebbe R5 di ogni preassociazione (PB) e di ogni svincolo della bozza, e delle AR attive dei
// componenti che escono dalla working. Una funzione pura sopra una lettura (LeggiFattiPiano): R4 la mostra, R5 la
// ricalcolera' nella sua connessione S e la applichera' con Registra, Sostituisci e Scollega. Mai una sovrascrittura
// silenziosa di una AR registrata verso un altro componente ([U] dir. 35 §8 C-5): la sostituzione c'e' solo con lo
// svincolo nella stessa bozza.
//
// Gli esiti di una PB (EM §3.7, H.7; MAT §4.2), nell'ordine dei controlli:
//   - non_richiesta: nodo commerciale, escluso o non incluso ([U] N-ESENZIONE): la PB non si applica, nessun blocco;
//   - in_conflitto, con il motivo (blocco PREASSOCIAZIONE_NON_APPLICABILE in R4, rifiuto in R5):
//     natura_non_preassociabile (punto 1 di trg_fra_nasce); legato_come_sorgente (lo STEP sorgente del finito, D39 §5
//     [U], D-2 (c)); step_candidato_del_finito (uno STEP candidato sorgente di un finito, non ancora confermato da S8:
//     G39-02 [U], lo STEP del finito non e' mai una PB; il fatto lo mette chi chiama, distintabozza.StepCandidatiDelFinito);
//     file_non_attivo (punto 4); legacy_da_verificare (una PB legacy non riconosciuta: R39-15, base [T-39]
//     dell'orchestratore); gia_associata_altrove con la variante stesso_prodotto | altro_prodotto | copia_legacy (Prova 7,
//     PD35 INC-G18; il punto 5 di trg_fra_nasce per la copia legacy); documento_senza_componente (D-AR06 A [T-39]);
//     documento_confermato_altrove (D-AR07 A [T-39]); codice_non_letto (D-1 A [T-39]); codice_diverso (il codice, o la
//     revisione se il file e il nodo ne hanno una, del file diverso da quello deciso per il nodo: MAT §4.2);
//   - in_attesa_codice: il nodo non ha ancora un codice (insieme a CODICE_DA_ASSEGNARE);
//   - gia_applicata: la stessa coppia ha gia' una AR attiva (nessuna riga nuova: la AR tiene la sua conferma d'origine);
//   - sostituzione: la bozza porta lo svincolo della AR attiva dello stesso allegato verso un componente di questo prodotto;
//   - applicabile.
// Gli esiti di uno svincolo (scollega_documento {allegato_id, componente_id}, D-4 A [T-39]): applicabile; svincolo_gia_chiuso
// (la AR l'ha gia' chiusa la corsia file dopo lo svincolo R2: D-ASS-03 [U]); in_conflitto con svincolo_su_documento (il
// file e' un documento confermato di quel componente: si scollega in V3), svincolo_componente_diverso (la AR attiva del
// file e' su un altro componente) o svincolo_senza_associazione (nessuna AR, ne' attiva ne' chiusa, per la coppia).
// Le AR attive dei registrati che escono (EM H.7 nodo_uscente_con_ar; D-AR09 [T-39], dialogo E-UX3 E4 [U]): un escluso
// senza lo svincolo tiene la AR («Mantieni»: avviso ESCLUSO_CON_DOCUMENTI, mai una cancellazione); un registrato tolto
// senza decisione e senza svincolo blocca (nodo_uscente_con_ar), oltre ad ASSENZA_NON_DECISA.

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Gli esiti del Piano.
const (
	EsitoApplicabile    = "applicabile"
	EsitoGiaApplicata   = "gia_applicata"
	EsitoSostituzione   = "sostituzione"
	EsitoInConflitto    = "in_conflitto"
	EsitoInAttesaCodice = "in_attesa_codice"
	EsitoNonRichiesta   = "non_richiesta"
	// svincolo_gia_chiuso: EsitoSvincoloGiaChiuso di Scollega (lo stesso valore)
	// EsitoMantenuta: la AR attiva di un escluso resta (D-AR09, «Mantieni»)
	EsitoMantenuta = "mantenuta"
)

// I motivi di in_conflitto (motivo di PREASSOCIAZIONE_NON_APPLICABILE, EM §8.1, App. I.3, I.8).
const (
	MotivoNaturaNonPreassociabile   = "natura_non_preassociabile"
	MotivoLegatoComeSorgente        = "legato_come_sorgente"
	MotivoStepCandidatoDelFinito    = "step_candidato_del_finito"
	MotivoFileNonAttivo             = "file_non_attivo"
	MotivoLegacyDaVerificare        = "legacy_da_verificare"
	MotivoGiaAssociataAltrove       = "gia_associata_altrove"
	MotivoDocumentoSenzaComponente  = "documento_senza_componente"
	MotivoDocumentoConfermatoAltrui = "documento_confermato_altrove"
	MotivoCodiceNonLetto            = "codice_non_letto"
	MotivoCodiceDiverso             = "codice_diverso"
	MotivoFuoriRFQ                  = "fuori_rfq"
	MotivoSvincoloSuDocumento       = "svincolo_su_documento"
	MotivoSvincoloComponenteDiverso = "svincolo_componente_diverso"
	MotivoSvincoloSenzaAssociazione = "svincolo_senza_associazione"
	MotivoNodoUscenteConAR          = "nodo_uscente_con_ar"
)

// Le varianti di gia_associata_altrove (MAT §4.2).
const (
	VarianteStessoProdotto = "stesso_prodotto"
	VarianteAltroProdotto  = "altro_prodotto"
	VarianteCopiaLegacy    = "copia_legacy"
)

// TipoDaDeterminare: il tipo di una proposta di documento che il worker non ha classificato (D-1 A [T-39]).
const TipoDaDeterminare = "da_determinare"

// OccorrenzaPiano: un'occorrenza della RFQ come la guarda il Piano (ListOccorrenzePiano).
type OccorrenzaPiano struct {
	DellaRfq           bool
	Attivo             bool
	Preassociabile     bool
	LegatoComeSorgente bool
	// StepCandidatoDelFinito: lo STEP e' candidato sorgente di un finito (G39-02 [U]); lo mette chi chiama, non
	// LeggiFattiPiano (il riconoscimento e' del Motore A, distintabozza.StepCandidatiDelFinito)
	StepCandidatoDelFinito bool
	Sha256                 string
	NomeFile               string
	ConProposta            bool
	TipoProposto           string
	CodiceLetto            string
	RevLetta               string
}

// DocumentoPiano: un documento confermato della RFQ con lo stesso contenuto di un'occorrenza.
type DocumentoPiano struct {
	Documento  uuid.UUID
	Componente *uuid.UUID
	Tipo       string
	StatoNas   string
	Sostituito bool
}

// FattiPiano sono i fatti del database che il Piano guarda (LeggiFattiPiano); il perimetro del prodotto della bozza e i
// prodotti dei componenti li mette chi chiama, che ha gia' lo stato della RFQ.
type FattiPiano struct {
	Occorrenze   map[uuid.UUID]OccorrenzaPiano
	Attive       map[uuid.UUID]Associazione // per allegato (D-ASS-04 A [T-39]: al piu' una)
	Chiuse       map[[2]uuid.UUID]bool      // (allegato, componente) con almeno una AR chiusa
	CopieSenzaAR map[uuid.UUID]uuid.UUID    // allegato → componente della copia di una pre-assegnazione legacy aperta
	Documenti    map[uuid.UUID][]DocumentoPiano
	// DelProdotto: i componenti della working nel perimetro del prodotto della bozza, e quelli che la bozza riusa
	DelProdotto map[uuid.UUID]bool
	// ProdottiDi: per un componente, i prodotti finiti della RFQ nel cui perimetro sta (in ordine)
	ProdottiDi map[uuid.UUID][]uuid.UUID
}

// VocePB: una PB della bozza come la vede il Piano.
type VocePB struct {
	Allegato    uuid.UUID
	ChiaveBozza string
	Provenienza string
	// Verificata: una PB legacy riconosciuta con file_riconosciuto (R39-15 [T-39]): non e' piu' legacy_da_verificare
	Verificata bool
	// Componente: il componente registrato del nodo (nil per un nodo da creare)
	Componente *uuid.UUID
	// Codice e Rev: l'identita' decisa del nodo (Codice vuoto: il nodo non ha ancora un codice)
	Codice string
	Rev    string
	// NonRichiesta: nodo commerciale, escluso o non incluso
	NonRichiesta bool
	// Svincolata: la bozza porta lo svincolo della AR attiva di questo allegato
	Svincolata bool
}

// VoceSvincolo: uno svincolo della bozza (scollega_documento).
type VoceSvincolo struct {
	Allegato    uuid.UUID
	Componente  uuid.UUID
	DaDecisione string
}

// Uscita: un registrato che esce dalla working con la conferma (escluso) o che la bozza non porta piu' (tolto).
type Uscita struct {
	Componente uuid.UUID
	Escluso    bool
}

// EsitoPB: l'esito di una PB.
type EsitoPB struct {
	Allegato    uuid.UUID `json:"allegato_id"`
	ChiaveBozza string    `json:"chiave_bozza"`
	Esito       string    `json:"esito"`
	Motivo      string    `json:"motivo,omitempty"`
	Variante    string    `json:"variante,omitempty"`
	// la AR attiva dell'allegato, se c'e' (per gia_applicata, sostituzione, gia_associata_altrove)
	Associazione *uuid.UUID  `json:"associazione_id,omitempty"`
	ARComponente *uuid.UUID  `json:"ar_componente_id,omitempty"`
	ARProdotti   []uuid.UUID `json:"ar_prodotti,omitempty"`
	// la copia legacy o il documento che fermano la PB
	CopiaComponente *uuid.UUID `json:"copia_componente_id,omitempty"`
	Documento       *uuid.UUID `json:"documento_id,omitempty"`
	CodiceLetto     string     `json:"codice_letto,omitempty"`
	RevLetta        string     `json:"rev_letta,omitempty"`
}

// Blocca dice se l'esito ferma R5.
func (e EsitoPB) Blocca() bool { return e.Esito == EsitoInConflitto || e.Esito == EsitoInAttesaCodice }

// Applica dice se R5 scriverebbe una AR (Registra o Sostituisci).
func (e EsitoPB) Applica() bool { return e.Esito == EsitoApplicabile || e.Esito == EsitoSostituzione }

// EsitoSvincolo: l'esito di uno svincolo.
type EsitoSvincolo struct {
	Allegato     uuid.UUID  `json:"allegato_id"`
	Componente   uuid.UUID  `json:"componente_id"`
	DaDecisione  string     `json:"da_decisione,omitempty"`
	Esito        string     `json:"esito"`
	Motivo       string     `json:"motivo,omitempty"`
	Associazione *uuid.UUID `json:"associazione_id,omitempty"`
	Documento    *uuid.UUID `json:"documento_id,omitempty"`
}

// Blocca dice se l'esito ferma R5.
func (e EsitoSvincolo) Blocca() bool { return e.Esito == EsitoInConflitto }

// EsitoUscita: la AR attiva di un registrato che esce.
type EsitoUscita struct {
	Allegato     uuid.UUID `json:"allegato_id"`
	Componente   uuid.UUID `json:"componente_id"`
	Associazione uuid.UUID `json:"associazione_id"`
	Escluso      bool      `json:"escluso"`
	Esito        string    `json:"esito"`
	Motivo       string    `json:"motivo,omitempty"`
}

// Blocca dice se l'esito ferma R5.
func (e EsitoUscita) Blocca() bool { return e.Esito == EsitoInConflitto }

// EsitiPiano: gli esiti del Piano, in ordine di allegato.
type EsitiPiano struct {
	Preassociazioni []EsitoPB
	Svincoli        []EsitoSvincolo
	Uscite          []EsitoUscita
}

// Piano calcola gli esiti (commento di testa). Pura e deterministica.
func Piano(pb []VocePB, svincoli []VoceSvincolo, uscite []Uscita, f FattiPiano) EsitiPiano {
	out := EsitiPiano{Preassociazioni: []EsitoPB{}, Svincoli: []EsitoSvincolo{}, Uscite: []EsitoUscita{}}
	for _, v := range pb {
		out.Preassociazioni = append(out.Preassociazioni, esitoPB(v, f))
	}
	sort.SliceStable(out.Preassociazioni, func(i, j int) bool {
		return out.Preassociazioni[i].Allegato.String() < out.Preassociazioni[j].Allegato.String()
	})
	svincolate := map[[2]uuid.UUID]bool{}
	for _, s := range svincoli {
		svincolate[[2]uuid.UUID{s.Allegato, s.Componente}] = true
		out.Svincoli = append(out.Svincoli, esitoSvincolo(s, f))
	}
	sort.SliceStable(out.Svincoli, func(i, j int) bool {
		a, b := out.Svincoli[i], out.Svincoli[j]
		if a.Allegato != b.Allegato {
			return a.Allegato.String() < b.Allegato.String()
		}
		return a.Componente.String() < b.Componente.String()
	})
	esce := map[uuid.UUID]Uscita{}
	for _, u := range uscite {
		esce[u.Componente] = u
	}
	allegati := make([]uuid.UUID, 0, len(f.Attive))
	for a := range f.Attive {
		allegati = append(allegati, a)
	}
	sort.Slice(allegati, func(i, j int) bool { return allegati[i].String() < allegati[j].String() })
	for _, a := range allegati {
		ar := f.Attive[a]
		u, ok := esce[ar.Componente]
		if !ok || svincolate[[2]uuid.UUID{a, ar.Componente}] {
			continue
		}
		e := EsitoUscita{Allegato: a, Componente: ar.Componente, Associazione: ar.ID, Escluso: u.Escluso, Esito: EsitoMantenuta}
		if !u.Escluso {
			e.Esito, e.Motivo = EsitoInConflitto, MotivoNodoUscenteConAR
		}
		out.Uscite = append(out.Uscite, e)
	}
	return out
}

func esitoPB(v VocePB, f FattiPiano) EsitoPB {
	e := EsitoPB{Allegato: v.Allegato, ChiaveBozza: v.ChiaveBozza}
	conflitto := func(motivo string) EsitoPB {
		e.Esito, e.Motivo = EsitoInConflitto, motivo
		return e
	}
	occ, ok := f.Occorrenze[v.Allegato]
	e.CodiceLetto, e.RevLetta = occ.CodiceLetto, occ.RevLetta
	if v.NonRichiesta {
		e.Esito = EsitoNonRichiesta
		return e
	}
	switch {
	case !ok || !occ.DellaRfq:
		return conflitto(MotivoFuoriRFQ)
	case !occ.Preassociabile:
		return conflitto(MotivoNaturaNonPreassociabile)
	case occ.LegatoComeSorgente:
		return conflitto(MotivoLegatoComeSorgente)
	case occ.StepCandidatoDelFinito:
		return conflitto(MotivoStepCandidatoDelFinito)
	case !occ.Attivo:
		return conflitto(MotivoFileNonAttivo)
	case v.Provenienza == ProvenienzaLegacy && !v.Verificata:
		return conflitto(MotivoLegacyDaVerificare)
	}
	esito := EsitoApplicabile
	if ar, ok := f.Attive[v.Allegato]; ok {
		id, comp := ar.ID, ar.Componente
		e.Associazione, e.ARComponente, e.ARProdotti = &id, &comp, append([]uuid.UUID(nil), f.ProdottiDi[comp]...)
		switch {
		case v.Componente != nil && *v.Componente == comp:
			e.Esito = EsitoGiaApplicata
			return e
		case v.Svincolata && f.DelProdotto[comp]:
			esito = EsitoSostituzione
		default:
			e.Variante = VarianteAltroProdotto
			if f.DelProdotto[comp] {
				e.Variante = VarianteStessoProdotto
			}
			return conflitto(MotivoGiaAssociataAltrove)
		}
	}
	if comp, ok := f.CopieSenzaAR[v.Allegato]; ok && (v.Componente == nil || *v.Componente != comp) {
		c := comp
		e.CopiaComponente, e.Variante = &c, VarianteCopiaLegacy
		return conflitto(MotivoGiaAssociataAltrove)
	}
	for _, d := range f.Documenti[v.Allegato] {
		doc := d.Documento
		switch {
		case d.Componente == nil:
			e.Documento = &doc
			return conflitto(MotivoDocumentoSenzaComponente)
		case v.Componente == nil || *d.Componente != *v.Componente:
			e.Documento = &doc
			return conflitto(MotivoDocumentoConfermatoAltrui)
		}
	}
	if strings.TrimSpace(v.Codice) == "" {
		e.Esito = EsitoInAttesaCodice
		return e
	}
	if !occ.ConProposta || occ.TipoProposto == TipoDaDeterminare || strings.TrimSpace(occ.CodiceLetto) == "" {
		return conflitto(MotivoCodiceNonLetto)
	}
	if !stessoCodice(occ.CodiceLetto, v.Codice) || (strings.TrimSpace(occ.RevLetta) != "" && strings.TrimSpace(v.Rev) != "" &&
		!stessoCodice(occ.RevLetta, v.Rev)) {
		return conflitto(MotivoCodiceDiverso)
	}
	e.Esito = esito
	return e
}

// stessoCodice: due codici (o due revisioni) uguali senza gli spazi ai lati e senza distinguere maiuscole e minuscole
// (la lettura del nome di un file puo' avere un'altra grafia; l'identita' decisa resta byte per byte nello snapshot) [T-39].
func stessoCodice(a, b string) bool {
	return strings.ToUpper(strings.TrimSpace(a)) == strings.ToUpper(strings.TrimSpace(b))
}

func esitoSvincolo(s VoceSvincolo, f FattiPiano) EsitoSvincolo {
	e := EsitoSvincolo{Allegato: s.Allegato, Componente: s.Componente, DaDecisione: s.DaDecisione}
	for _, d := range f.Documenti[s.Allegato] {
		if d.Componente != nil && *d.Componente == s.Componente {
			doc := d.Documento
			e.Esito, e.Motivo, e.Documento = EsitoInConflitto, MotivoSvincoloSuDocumento, &doc
			return e
		}
	}
	if ar, ok := f.Attive[s.Allegato]; ok {
		id := ar.ID
		e.Associazione = &id
		if ar.Componente == s.Componente {
			e.Esito = EsitoApplicabile
		} else {
			e.Esito, e.Motivo = EsitoInConflitto, MotivoSvincoloComponenteDiverso
		}
		return e
	}
	if f.Chiuse[[2]uuid.UUID{s.Allegato, s.Componente}] {
		e.Esito = EsitoSvincoloGiaChiuso
		return e
	}
	e.Esito, e.Motivo = EsitoInConflitto, MotivoSvincoloSenzaAssociazione
	return e
}

// LeggiFattiPiano legge i fatti del Piano per gli allegati dati, nella transazione di chi chiama (sola lettura): le
// occorrenze con la lettura del codice, tutte le AR attive della RFQ (anche degli allegati che la bozza non cita: le
// uscite), le AR chiuse degli allegati dati, le copie legacy senza AR, i documenti con lo stesso contenuto. DelProdotto e
// ProdottiDi restano vuoti: li riempie chi chiama.
func LeggiFattiPiano(ctx context.Context, q *db.Queries, thread uuid.UUID, allegati []uuid.UUID) (FattiPiano, error) {
	f := FattiPiano{Occorrenze: map[uuid.UUID]OccorrenzaPiano{}, Attive: map[uuid.UUID]Associazione{}, Chiuse: map[[2]uuid.UUID]bool{},
		CopieSenzaAR: map[uuid.UUID]uuid.UUID{}, Documenti: map[uuid.UUID][]DocumentoPiano{}, DelProdotto: map[uuid.UUID]bool{},
		ProdottiDi: map[uuid.UUID][]uuid.UUID{}}
	ids := unici(allegati)
	attive, err := AttiveDellaRfq(ctx, q, thread)
	if err != nil {
		return f, err
	}
	for _, a := range attive {
		f.Attive[a.Allegato] = a
	}
	copie, err := q.ListCopieSenzaARDellaRfq(ctx, thread)
	if err != nil {
		return f, err
	}
	for _, r := range copie {
		f.CopieSenzaAR[r.AllegatoID] = r.ComponenteID
	}
	if len(ids) == 0 {
		return f, nil
	}
	occ, err := q.ListOccorrenzePiano(ctx, db.ListOccorrenzePianoParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return f, err
	}
	for _, r := range occ {
		f.Occorrenze[r.AllegatoID] = OccorrenzaPiano{DellaRfq: r.DellaRfq, Attivo: r.Attivo, Preassociabile: r.Preassociabile,
			LegatoComeSorgente: r.LegatoComeSorgente, Sha256: strings.TrimSpace(r.Sha256), NomeFile: r.NomeFile, ConProposta: r.ConProposta,
			TipoProposto: r.TipoProposto, CodiceLetto: r.CodiceLetto, RevLetta: r.RevLetta}
	}
	tutte, err := q.ListAssociazioniDegliAllegati(ctx, db.ListAssociazioniDegliAllegatiParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return f, err
	}
	for _, r := range tutte {
		if !r.Attiva {
			f.Chiuse[[2]uuid.UUID{r.AllegatoID, r.ComponenteID}] = true
		}
	}
	docs, err := q.ListDocumentiDegliAllegati(ctx, db.ListDocumentiDegliAllegatiParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return f, err
	}
	for _, r := range docs {
		d := DocumentoPiano{Documento: r.DocumentoID, Tipo: r.Tipo, StatoNas: r.StatoNas, Sostituito: r.Sostituito}
		if r.ComponenteID.Valid {
			c := r.ComponenteID.UUID
			d.Componente = &c
		}
		f.Documenti[r.AllegatoID] = append(f.Documenti[r.AllegatoID], d)
	}
	return f, nil
}

func unici(xx []uuid.UUID) []uuid.UUID {
	visti := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(xx))
	for _, x := range xx {
		if x != uuid.Nil && !visti[x] {
			visti[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
