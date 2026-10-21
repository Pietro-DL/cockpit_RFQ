package distintabozza

// R5 nel formato 3: la conferma atomica (EM §9.5, §7.5, §7.6, App. I.2, I.3, I.6; C3 §5.3 per quanto vale; direttiva 39 §1,
// §2, §7 [U + T-39]). R5 chiude una versione salvata con R2 della bozza formato 3 del prodotto e la rende la BOM confermata:
// nella stessa transazione scrive la working, promuove la sorgente STEP nella working, registra la conferma e le sue righe,
// applica le PB come AR attraverso il servizio unico (con provenienza_pb e importazione_id copiati dallo snapshot
// approvato), scrive i segni legacy come copia derivata, chiude la bozza e registra l'intento. Tutto o niente.
//
// L'invariante della direttiva 39 §1 [U]: una bozza senza una sorgente S8 valida (EM App. I.2) si rifiuta con 422 BLOCCHI
// (blocco STEP_SORGENTE_NON_CONFERMATA) senza scrivere niente: ne' la working, ne' conferme, ne' AR, ne' il documento, ne'
// il NAS. Nessuna scorciatoia: non per il criterio nessuno, non per il legacy, non per la sola presenza del file fra gli
// allegati, non per una chiamata HTTP diretta. Il gate si ricalcola nella connessione S (la funzione condivisa di R4) e si
// rilegge in T sotto il lucchetto della RFQ, che S8 e la revoca prendono anche loro.
//
// R5 e' l'unico scrittore di componente.step_strutturale_id nel percorso formato 3, in prima istanza e nelle revisioni
// (D39 §2 [U + T-39]). L'ordine (App. I.6), ognuno dopo il precedente:
//  0. controlli senza scrivere: RFQ e prodotto, intento (stessa chiave: riproposto; altro corpo: CHIAVE_RIUSATA), bozza e
//     versione (BOZZA_VERSIONE), lo snapshot del corpo uguale a quello della versione salvata a meno dei campi del server,
//     poi R4 ricostruito nella connessione S (REPEATABLE READ READ ONLY aperta dopo il lucchetto): base (BASE_SUPERATA),
//     firma (ANTEPRIMA_SUPERATA con il riepilogo nuovo), blocchi (BLOCCHI, compreso il gate), presa d'atto dei documenti
//     in analisi, NESSUNA_MODIFICA, la difesa R5_NON_APPLICABILE ([T-39]: dopo un R4 confermabile non nasce);
//  1. i lucchetti, nell'ordine documentato nel README (RFQ → bozza → documenti A e B → componente del prodotto →
//     documento_proposta → componente_proposta → relazione_proposta), poi il gate riletto in T e il piano riletto in T
//     confrontato con quello di S;
//  2. la working: componenti nuovi, revisioni e descrizioni, archi tolti, cambiati e nuovi, componenti esclusi, poi la
//     promozione della sorgente B in componente.step_strutturale_id con l'evento autorizzazione di fonte_step_evento;
//  3. la sostituzione documentale A.sostituito_da = B, solo se la working aveva uno STEP A diverso da B e non sostituito;
//  4. la testata conferma_strutturale (dopo la working: working_conta, P4X01);
//  5. le righe e gli archi certificati e le righe dei nodi non inclusi;
//  6. le AR e le copie derivate attraverso il servizio unico (Registra, Sostituisci, Scollega);
//  7. i segni legacy (copia derivata): le righe dei nodi e degli archi tenuti, la chiusura dei non inclusi, la chiusura
//     delle righe aperte dello STEP A sostituito;
//  8. la bozza chiusa come confermata, l'intento conferma_bom_v3 con la risposta, f8_stato in attesa;
//  9. H_T (parti proposta e analisi, con gli stessi elenchi di H_S) e il COMMIT, con i controlli differiti (0020, 0022,
//     0023). H_S diverso da H_T: un secondo controllo, poi 409 ingressi_in_movimento.
//
// Il motore (EM §9.5 con U2 §6.3.5, RV3-25 A [U]): R5 ricalcola il riepilogo con il motore dentro la connessione S
// (OpzioniR5.Motore, dato dall'adattatore web con il semaforo dei calcoli; riepilogo3_motore.go), cosi' gli avvisi e i
// conflitti del motore stanno nella firma confrontata. Il posto del semaforo si prende per il calcolo in S, non da prima di
// BloccaThread fino al COMMIT: il semaforo e' del servizio del motore [T-39]. Motore occupato o non pronto: 503
// CALCOLO_OCCUPATO; scaduto: 504 CALCOLO_SCADUTO; un altro errore o un riepilogo senza gli avvisi del motore: 500
// ERRORE_CALCOLO; tutti non registrati, niente scritto. Il resto della strategia A (S dopo il lucchetto, H_S/H_T, il secondo
// controllo, 40P01 e 55P03 come 409 riprova) c'e'.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// GestoConfermaBomV3: il gesto di esito_intento di R5 formato 3 (0023, trg_esito_intento_formato).
const GestoConfermaBomV3 = "conferma_bom_v3"

// RottaR5: l'origine della conferma (ck_conferma_json: origine.rotta obbligatoria).
const RottaR5 = "motore-a/bom/conferma"

// I codici di R5 che R2 e R4 non hanno (EM §9.5, §8.4).
const (
	CodiceBlocchi         = "BLOCCHI"
	CodiceNessunaModifica = "NESSUNA_MODIFICA"
	CodiceControlloFinale = "CONTROLLO_FINALE"
	CodiceCalcoloOccupato = "CALCOLO_OCCUPATO"
)

// I motivi di R5 (EM §9.5; C3 §5.3).
const (
	MotivoBozzaObbligatoria          = "bozza_obbligatoria"
	MotivoSnapshotDiverso            = "snapshot_diverso"
	MotivoCodiceOccupato             = "codice_occupato"
	MotivoDocumentiInAnalisiCambiati = "documenti_in_analisi_cambiati"
	MotivoIngressiInMovimento        = "ingressi_in_movimento"
	MotivoRiprova                    = "riprova"
	MotivoPresaDAttoObbligatoria     = "presa_d_atto_obbligatoria"
	AnteprimaRiepilogoBom            = "riepilogo_bom"
)

// BloccoR5NonApplicabile: una parte della bozza che R5 non saprebbe applicare alla working senza rischiare una scrittura
// sbagliata. Dal blocco «R5 completo» resta solo come difesa: ogni caso che lo farebbe nascere e' un blocco del catalogo in
// R4 (identita' di un registrato diversa dal suo codice: IDENTITA_INCOMPATIBILE o RIUSO_INCOMPATIBILE «codice»; nodi dello
// stesso componente con dati diversi: COLLISIONE_EMITTENTI, IDENTITA_INCOMPATIBILE, DUE_CREA_STESSO_CODICE,
// RIUSO_INCOMPATIBILE «stesso_componente»; un nodo senza componente; il componente assente: BASE_SUPERATA o FUORI_RFQ),
// quindi dopo un R4 confermabile non nasce. [T-39] della corsia persistenza: la conferma si rifiuta, niente si scrive.
const BloccoR5NonApplicabile = "R5_NON_APPLICABILE"

// AttesaConnessioneS: quanto R5 aspetta la seconda connessione (S) dopo il lucchetto (EM §9.5 passo 4, base [T] 5 s).
const AttesaConnessioneS = 5 * time.Second

// AttesaLucchettiR5: il lock_timeout locale dei lucchetti sulle righe (EM §9.5 passo 7, base [T] 5 s).
const AttesaLucchettiR5 = "5s"

// RichiestaR5 e' il corpo di R5 letto: {firma_anteprima, bozza, presa_d_atto_in_analisi?} (EM §9.5; la presa d'atto al
// primo livello come la manda il ponte, scelta [T] della corsia interfaccia).
type RichiestaR5 struct {
	FirmaAnteprima string
	Bozza          RichiestaR4
	// PresaDAtto: gli allegati in analisi di cui chi conferma ha preso atto; PresaDAttoPresente dice se il campo c'era
	PresaDAtto         []uuid.UUID
	PresaDAttoPresente bool
	// ImprontaCorpo: sha256 del JSON canonico di {firma_anteprima, bozza senza bozza_versione, presa_d_atto_in_analisi
	// in ordine} (C3 §5.2; come impronteCorpo del ponte)
	ImprontaCorpo string
}

// LeggiRichiestaR5 legge il corpo di R5 (commento di RichiestaR5): lettura stretta del primo livello, la bozza con il
// lettore di R4, poi gli obblighi di R5: bozza_id (motivo bozza_obbligatoria), bozza_versione e chiave_idempotenza.
func LeggiRichiestaR5(raw []byte) (RichiestaR5, error) {
	var out RichiestaR5
	var m map[string]json.RawMessage
	if e := decodificaStretta("richiesta", raw, &m); e != nil {
		return out, e
	}
	if m == nil {
		return out, corpo("richiesta", "il corpo e' un oggetto JSON {firma_anteprima, bozza}")
	}
	for k := range m {
		switch k {
		case "firma_anteprima", "bozza", "presa_d_atto_in_analisi":
		default:
			return out, corpo(k, "campo non previsto in R5: %s", k)
		}
	}
	if json.Unmarshal(m["firma_anteprima"], &out.FirmaAnteprima) != nil || !reSha256.MatchString(out.FirmaAnteprima) {
		return out, corpo("firma_anteprima", "firma_anteprima e' la firma del riepilogo letto (64 esadecimali)")
	}
	rb, ok := m["bozza"]
	if !ok || !presenteValore(rb) {
		return out, corpoMotivo("bozza", MotivoBozzaObbligatoria, "la conferma formato 3 chiude una versione salvata della bozza: serve la bozza")
	}
	var bm map[string]json.RawMessage
	if json.Unmarshal(rb, &bm) != nil || bm == nil {
		return out, corpo("bozza", "bozza e' un oggetto JSON")
	}
	if id, ok := bm["bozza_id"]; !ok || !presenteValore(id) {
		return out, corpoMotivo("bozza.bozza_id", MotivoBozzaObbligatoria,
			"la conferma formato 3 chiude sempre una versione salvata con R2: senza bozza_id non ci sono autore e motivo delle decisioni")
	}
	b, err := LeggiRichiestaR4(rb)
	if err != nil {
		return out, err
	}
	if b.BozzaID == nil {
		return out, corpoMotivo("bozza.bozza_id", MotivoBozzaObbligatoria,
			"la conferma formato 3 chiude sempre una versione salvata con R2: senza bozza_id non ci sono autore e motivo delle decisioni")
	}
	if b.BozzaVersione == 0 {
		return out, corpo("bozza.bozza_versione", "bozza_versione e' la versione salvata che la conferma chiude")
	}
	if b.ChiaveIdempotenza == nil {
		return out, corpo("bozza.chiave_idempotenza", "la conferma vuole la chiave d'idempotenza (un UUID)")
	}
	out.Bozza = b
	var presa []string
	if p, ok := m["presa_d_atto_in_analisi"]; ok && strings.TrimSpace(string(p)) != "null" {
		out.PresaDAttoPresente = true
		if json.Unmarshal(p, &presa) != nil {
			return out, corpo("presa_d_atto_in_analisi", "presa_d_atto_in_analisi e' un elenco di allegato_id")
		}
		visti := map[uuid.UUID]bool{}
		for _, s := range presa {
			id, err := uuid.Parse(s)
			if err != nil || id == uuid.Nil {
				return out, corpo("presa_d_atto_in_analisi", "presa_d_atto_in_analisi e' un elenco di allegato_id")
			}
			if !visti[id] {
				visti[id] = true
				out.PresaDAtto = append(out.PresaDAtto, id)
			}
		}
		sort.Slice(out.PresaDAtto, func(i, j int) bool { return out.PresaDAtto[i].String() < out.PresaDAtto[j].String() })
	}
	// l'impronta del corpo: la bozza senza bozza_versione
	delete(bm, "bozza_versione")
	imp := map[string]any{"firma_anteprima": out.FirmaAnteprima, "bozza": bm}
	if out.PresaDAttoPresente {
		ss := make([]string, 0, len(out.PresaDAtto))
		for _, id := range out.PresaDAtto {
			ss = append(ss, id.String())
		}
		imp["presa_d_atto_in_analisi"] = ss
	}
	can, err := jsoncanonico.Codifica(imp)
	if err != nil {
		return out, corpo("richiesta", "il corpo non si rilegge: %v", err)
	}
	out.ImprontaCorpo = jsoncanonico.Impronta(can)
	return out, nil
}

// ------------------------------------------------------------------ la risposta

// ArcoScritto: un arco della working che R5 ha scritto (aggiunto, tolto, cambiato).
type ArcoScritto struct {
	PadreID       uuid.UUID `json:"padre_id"`
	FiglioID      uuid.UUID `json:"figlio_id"`
	Qta           int       `json:"qta"`
	QtaPrecedente *int      `json:"qta_precedente,omitempty"`
}

// FonteR5: che cosa R5 ha fatto della fonte della working (App. I.6, passi 1 e 2).
type FonteR5 struct {
	Promossa       bool       `json:"promossa"`
	StepPrima      *uuid.UUID `json:"step_prima,omitempty"`
	StepDopo       *uuid.UUID `json:"step_dopo,omitempty"`
	SostituzioneDi *uuid.UUID `json:"sostituzione_di,omitempty"`
	// CorrezioneSorgente: la conferma e' una revisione interna, correzione della sorgente (G39-03 [U]; forma [T-39])
	CorrezioneSorgente bool `json:"correzione_sorgente,omitempty"`
}

// ConfermaR5 e' la risposta 200 di R5 (C3 §5.5 con le aggiunte di EM §9.5 e U2: niente verifica_motore_a).
type ConfermaR5 struct {
	Esito                    string               `json:"esito"`
	Riproposto               bool                 `json:"riproposto"`
	StatoIntento             string               `json:"stato_intento,omitempty"`
	ChiaveIdempotenza        uuid.UUID            `json:"chiave_idempotenza"`
	Formato                  int                  `json:"formato"`
	ConfermaID               uuid.UUID            `json:"conferma_id"`
	NumeroConferma           int                  `json:"numero_conferma"`
	ProdottoID               uuid.UUID            `json:"prodotto_id"`
	BozzaID                  uuid.UUID            `json:"bozza_id"`
	BozzaVersione            int                  `json:"bozza_versione"`
	FirmaStruttura           string               `json:"firma_struttura,omitempty"`
	Contatore                int64                `json:"contatore"`
	Mappa                    map[string]uuid.UUID `json:"mappa,omitempty"`
	ArchiAggiunti            []ArcoScritto        `json:"archi_aggiunti"`
	ArchiTolti               []ArcoScritto        `json:"archi_tolti"`
	ArchiModificati          []ArcoScritto        `json:"archi_modificati"`
	IdentitaCreate           []uuid.UUID          `json:"identita_create"`
	IdentitaRiusate          []uuid.UUID          `json:"identita_riusate"`
	IdentitaArchiviate       []uuid.UUID          `json:"identita_archiviate"`
	IdentitaRicodificate     []uuid.UUID          `json:"identita_ricodificate"`
	IdentitaRipristinate     []uuid.UUID          `json:"identita_ripristinate"`
	TipiCambiati             []uuid.UUID          `json:"tipi_cambiati"`
	IdentitaAccettate        []string             `json:"identita_accettate"`
	IdentitaCorrette         []string             `json:"identita_corrette"`
	EsclusiCondivisi         []uuid.UUID          `json:"esclusi_condivisi"`
	NonInclusiApplicati      int                  `json:"non_inclusi_applicati"`
	AssenzeApplicate         int                  `json:"assenze_applicate"`
	CorrezioniRicodifica     int                  `json:"correzioni_ricodifica"`
	AssociazioniApplicate    int                  `json:"associazioni_applicate"`
	AssociazioniGiaApplicate int                  `json:"associazioni_gia_applicate"`
	SvincoliApplicati        int                  `json:"svincoli_applicati"`
	SvincoliGiaChiusi        int                  `json:"svincoli_gia_chiusi"`
	Fonte                    FonteR5              `json:"fonte"`
	CriterioVerifica         string               `json:"criterio_verifica,omitempty"`
	MonoPezzo                bool                 `json:"mono_pezzo"`
	F8Stato                  string               `json:"f8_stato,omitempty"`
	Testo                    string               `json:"testo"`
}

// RifiutoR5 e' un rifiuto di R5 con un corpo in piu' rispetto a Errore: i blocchi (422 BLOCCHI), il riepilogo nuovo (409
// ANTEPRIMA_SUPERATA), l'esito non_registrato (500 CONTROLLO_FINALE, 503 CALCOLO_OCCUPATO). Errors.As verso *Errore lo
// trova (Unwrap), cosi' chi traduce i codici comuni lo tratta come gli altri.
type RifiutoR5 struct {
	Err           *Errore
	Blocchi       []Rilievo
	Riepilogo     *RiepilogoR4
	NonRegistrato bool
	// Causa: l'errore del database dietro un CONTROLLO_FINALE (per il log)
	Causa error
}

func (r *RifiutoR5) Error() string {
	if r.Causa != nil {
		return r.Err.Error() + ": " + r.Causa.Error()
	}
	return r.Err.Error()
}

// Unwrap da' l'Errore con il codice.
func (r *RifiutoR5) Unwrap() error { return r.Err }

func rifiutoBlocchi(bb []Rilievo) *RifiutoR5 {
	return &RifiutoR5{Err: &Errore{Codice: CodiceBlocchi, Messaggio: fmt.Sprintf("La BOM non si conferma: %d blocchi.", len(bb))}, Blocchi: bb}
}

func rifiutoRiprova(formato string, a ...any) *RifiutoR5 {
	return &RifiutoR5{Err: &Errore{Codice: CodiceAnteprimaSuperata, Motivo: MotivoRiprova, Messaggio: fmt.Sprintf(formato, a...)}}
}

// OpzioniR5 sono i parametri di R5 che non stanno nel corpo.
type OpzioniR5 struct {
	UtenteID uuid.UUID
	// ImprontaRegole: l'impronta delle regole caricate all'avvio (parte proposta di impronta-ingressi-2)
	ImprontaRegole string
	// AttesaS: quanto si aspetta la connessione S (0 = AttesaConnessioneS)
	AttesaS time.Duration
	// Guasto: solo per le prove (rollback iniettato): chiamato dopo ogni passo delle scritture con il nome del passo
	// («promozione», «sostituzione», «testata», «certificati», «associazioni», «segni», «chiusura»); un errore annulla
	Guasto func(passo string) error
	// DopoS: solo per le prove (concorrenza): chiamato dopo che S e' chiusa e prima dei lucchetti delle righe
	DopoS func()
	// Motore: il calcolo del motore nella connessione S (obbligatorio: senza, 500 ERRORE_CALCOLO non registrato)
	Motore CalcoloMotoreR5
	// invertiTestata: solo per le prove dell'ordine (D39 §7): la testata prima della working, che il database ferma (P4X01)
	invertiTestata bool
}

// AvviatoreR5 apre le due transazioni di R5 (T in scrittura, S in sola lettura): lo soddisfa *pgxpool.Pool.
type AvviatoreR5 interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, opz pgx.TxOptions) (pgx.Tx, error)
}

// Conferma3 e' R5 formato 3 (commento di testa). Un secondo controllo solo se H_S e H_T non coincidono (EM §9.5 passo 10).
func Conferma3(ctx context.Context, pool AvviatoreR5, thread uuid.UUID, r RichiestaR5, op OpzioniR5) (ConfermaR5, error) {
	if op.AttesaS <= 0 {
		op.AttesaS = AttesaConnessioneS
	}
	for giro := 1; ; giro++ {
		out, movimento, err := tentativoR5(ctx, pool, thread, r, op)
		if err != nil {
			return ConfermaR5{}, traduciR5(err)
		}
		if !movimento {
			return out, nil
		}
		if giro >= 2 {
			return ConfermaR5{}, &RifiutoR5{Err: &Errore{Codice: CodiceAnteprimaSuperata, Motivo: MotivoIngressiInMovimento,
				Messaggio: "le proposte o le analisi della RFQ stanno cambiando mentre confermi: riprova fra poco con la stessa chiave"}}
		}
	}
}

// traduciR5: 40P01, 55P03 e 40001 in qualunque passo sono un 409 riprova, mai un 5xx (EM §9.5 passo 11).
func traduciR5(err error) error {
	switch sqlstate(err) {
	case "40P01", "55P03", "40001":
		return rifiutoRiprova("un'altra operazione sulla stessa RFQ era in corso: niente e' stato scritto, riprova con la stessa chiave")
	}
	return err
}

// tentativoR5: un giro della transazione T. movimento: H_S diverso da H_T (rollback fatto, niente scritto).
func tentativoR5(ctx context.Context, pool AvviatoreR5, thread uuid.UUID, r RichiestaR5, op OpzioniR5) (ConfermaR5, bool, error) {
	var zero ConfermaR5
	s := r.Bozza
	chiave := *s.ChiaveIdempotenza
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, false, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return zero, false, err
	}
	// l'intento, prima di ogni altro controllo: un reinvio trova la sua conferma anche se la bozza e' chiusa
	if gia, ok, err := intentoR5(ctx, q, thread, chiave, r.ImprontaCorpo, op.UtenteID); err != nil || ok {
		return gia, false, err
	}
	prodotto := s.Snapshot.ProdottoID
	if _, err := prodottoDellaRfq(ctx, q, thread, prodotto, true); err != nil {
		return zero, false, err
	}
	b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: *s.BozzaID, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, false, fuori("bozza.bozza_id", "la bozza %s non e' di questa RFQ", *s.BozzaID)
	}
	if err != nil {
		return zero, false, err
	}
	if b.ProdottoID != prodotto {
		// R5 ha FUORI_RFQ, non BOZZA_ALTRO_PRODOTTO (EM §9.5, catalogo confermaBom del ponte) [T-39]
		return zero, false, fuori("bozza.bozza_id", "la bozza %s e' di un altro prodotto", b.BozzaID)
	}
	if int(b.Formato) != FormatoSnapshot3 {
		return zero, false, corpoMotivo("bozza.bozza_id", MotivoFormatoDaConvertire, "la bozza e' nel formato %d: si converte con converti_da (R2)", b.Formato)
	}
	if b.Stato != StatoAperta || int(b.Versione) != s.BozzaVersione {
		return zero, false, conflittoVersione(b, s.BozzaVersione)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: thread, Versione: b.Versione})
	if err != nil {
		return zero, false, err
	}
	salvato, impronta, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return zero, false, err
	}
	if !uguali3(senzaProvenienza(salvato), senzaProvenienza(s.Snapshot)) {
		c := conflittoVersione(b, s.BozzaVersione)
		c.Motivo, c.Messaggio = MotivoSnapshotDiverso, "la bozza del corpo non e' la versione salvata che dichiara: salvala (R2) e rileggi il riepilogo prima di confermare"
		return zero, false, c
	}
	// S: la funzione condivisa di R4 sulla versione salvata, il piano di R5 e H_S, in sola lettura dopo il lucchetto
	rich := RichiestaR4{Snapshot: salvato, BozzaID: s.BozzaID, BozzaVersione: s.BozzaVersione, ChiaveIdempotenza: s.ChiaveIdempotenza}
	rp, pS, hS, err := connessioneS(ctx, pool, thread, rich, op)
	if err != nil {
		return zero, false, err
	}
	if op.DopoS != nil {
		op.DopoS()
	}
	if err := controlliR5(r, rp, pS); err != nil {
		return zero, false, err
	}
	// i lucchetti, nell'ordine documentato (README), con il lock_timeout locale
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '"+AttesaLucchettiR5+"'"); err != nil {
		return zero, false, err
	}
	if err := bloccaR5(ctx, q, thread, prodotto, pS); err != nil {
		return zero, false, err
	}
	// il gate riletto in T sotto il lucchetto (I.3): S8 e la revoca prendono lo stesso lucchetto della RFQ
	if g, err := gateInT(ctx, q, thread, salvato); err != nil {
		return zero, false, err
	} else if !g.Valida {
		return zero, false, rifiutoBlocchi([]Rilievo{rilievoGate(g, salvato)})
	} else if rp.Sorgente.DocumentoID == nil || g.DocumentoID == nil || *g.DocumentoID != *rp.Sorgente.DocumentoID {
		return zero, false, rifiutoRiprova("la sorgente della bozza e' cambiata mentre confermavi: rileggi il riepilogo")
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return zero, false, err
	}
	pT, err := pianificaR5(ctx, q, thread, salvato, rp, st)
	if err != nil {
		return zero, false, err
	}
	if !uguali(pS.confronto(), pT.confronto()) {
		return zero, false, rifiutoRiprova("lo stato della RFQ letto sotto il lucchetto non e' quello del riepilogo: riprova")
	}
	// le scritture
	sc := &scrittureR5{ctx: ctx, tx: tx, q: q, thread: thread, utente: op.UtenteID, chiave: chiave, snap: salvato, impronta: impronta,
		versione: v, rp: rp, p: pT, st: st, guasto: op.Guasto, richiesta: r, invertiTestata: op.invertiTestata}
	out, err := sc.esegui()
	if err != nil {
		return zero, false, err
	}
	hT, err := improntaR5(ctx, q, thread, prodotto, op.ImprontaRegole, pT)
	if err != nil {
		return zero, false, err
	}
	if hT != hS {
		return zero, true, nil // ROLLBACK (defer): niente e' stato scritto
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, false, erroreAlCommit(err)
	}
	return out, false, nil
}

// trova3: i rilievi con il codice.
func trova3(rr []Rilievo, codice string) []Rilievo {
	var out []Rilievo
	for _, r := range rr {
		if r.Codice == codice {
			out = append(out, r)
		}
	}
	return out
}

// uguali3: due snapshot con la stessa forma canonica.
func uguali3(a, b Snapshot3) bool {
	ca, err1 := a.Canonico()
	cb, err2 := b.Canonico()
	return err1 == nil && err2 == nil && bytes.Equal(ca, cb)
}

// intentoR5: l'esito registrato con la stessa chiave (EM §9.5, C3 §5.3 passo 2): stessa impronta, gesto, RFQ e utente →
// riproposto con lo stato di F8 di adesso; altrimenti CHIAVE_RIUSATA (una chiave di un'altra RFQ: FUORI_RFQ). Senza
// intento ma con la conferma della chiave (intento oltre la conservazione): riproposto, ricostruito dalla conferma.
func intentoR5(ctx context.Context, q *db.Queries, thread, chiave uuid.UUID, impronta string, utente uuid.UUID) (ConfermaR5, bool, error) {
	ei, err := q.GetEsitoIntentoPerChiave(ctx, chiave)
	switch {
	case err == nil:
		if ei.ThreadID != thread {
			return ConfermaR5{}, false, fuori("bozza.chiave_idempotenza", "la chiave %s e' di un'altra RFQ", chiave)
		}
		if ei.Gesto != GestoConfermaBomV3 || strings.TrimSpace(ei.ImprontaCorpo) != impronta || ei.UtenteID != utente {
			return ConfermaR5{}, false, &Errore{Codice: CodiceChiaveRiusata, Campo: "bozza.chiave_idempotenza",
				Messaggio: "questa chiave d'intento ha gia' registrato un altro corpo: per riprovare serve una chiave nuova"}
		}
		var out ConfermaR5
		if err := json.Unmarshal(ei.Esito, &out); err != nil {
			return ConfermaR5{}, false, fmt.Errorf("esito registrato della conferma non leggibile: %w", err)
		}
		out.Riproposto = true
		if f, err := q.GetF8StatoDellaConferma(ctx, db.GetF8StatoDellaConfermaParams{ThreadID: thread, ConfermaID: uuid.NullUUID{UUID: out.ConfermaID, Valid: true}}); err == nil {
			out.F8Stato = f
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return ConfermaR5{}, false, err
		}
		return out, true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return ConfermaR5{}, false, err
	}
	c, err := q.GetConfermaPerChiave(ctx, db.GetConfermaPerChiaveParams{ChiaveIdempotenza: uuid.NullUUID{UUID: chiave, Valid: true}, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfermaR5{}, false, nil
	}
	if err != nil {
		return ConfermaR5{}, false, err
	}
	out := ConfermaR5{Esito: "confermata", Riproposto: true, StatoIntento: IntentoRegistratoSenzaEsito, ChiaveIdempotenza: chiave,
		Formato: int(c.Formato), ConfermaID: c.ConfermaID, NumeroConferma: int(c.Numero), ProdottoID: c.ProdottoID,
		ArchiAggiunti: []ArcoScritto{}, ArchiTolti: []ArcoScritto{}, ArchiModificati: []ArcoScritto{}, IdentitaCreate: []uuid.UUID{},
		IdentitaRiusate: []uuid.UUID{}, IdentitaArchiviate: []uuid.UUID{}, IdentitaAccettate: []string{}, IdentitaCorrette: []string{},
		EsclusiCondivisi: []uuid.UUID{}, Testo: fmt.Sprintf("Conferma n. %d gia' registrata con questa chiave.", c.Numero)}
	if c.BozzaID.Valid {
		out.BozzaID = c.BozzaID.UUID
	}
	if c.BozzaVersione.Valid {
		out.BozzaVersione = int(c.BozzaVersione.Int32)
	}
	return out, true, nil
}

// connessioneS: la seconda connessione, REPEATABLE READ READ ONLY aperta dopo il lucchetto della RFQ con un tempo breve
// (EM §9.5 passo 4): R4 ricalcolato, il piano di R5 e H_S; si chiude con ROLLBACK. Senza connessione entro l'attesa: 503
// CALCOLO_OCCUPATO, non registrato.
func connessioneS(ctx context.Context, pool AvviatoreR5, thread uuid.UUID, r RichiestaR4, op OpzioniR5) (RiepilogoR4, *pianoR5, string, error) {
	cctx, cancel := context.WithTimeout(ctx, op.AttesaS)
	txS, err := pool.BeginTx(cctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	cancel()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return RiepilogoR4{}, nil, "", &RifiutoR5{Err: &Errore{Codice: CodiceCalcoloOccupato,
				Messaggio: "il server e' occupato: la conferma non e' partita, niente e' stato scritto; riprova con la stessa chiave"}, NonRegistrato: true}
		}
		return RiepilogoR4{}, nil, "", err
	}
	defer txS.Rollback(ctx) //nolint:errcheck // sola lettura
	qS := db.New(txS)
	// il motore nella stessa fotografia (la prima lettura di S fissa l'istantanea)
	var ing *IngressoMotoreR4
	errM := errors.New("motore A: nessun calcolo del motore per R5")
	if op.Motore != nil {
		ing, errM = op.Motore(ctx, txS, thread, r.Snapshot.ProdottoID)
	}
	if errM != nil {
		// occupato o non pronto: il motore non ha letto niente e S e' intatta; il riepilogo leggero dice ancora la base
		// superata (409) e il gate della sorgente (422 BLOCCHI) prima del 503 [T-39]. Un altro errore (scaduto, un guasto)
		// puo' aver lasciato S a meta': il rifiuto del motore subito.
		if !errors.Is(errM, ErrMotoreOccupato) && !errors.Is(errM, ErrMotoreNonPronto) {
			return RiepilogoR4{}, nil, "", rifiutoMotore(errM)
		}
		rl, err := Riepilogo3(ctx, qS, thread, r, OpzioniR4{UtenteID: op.UtenteID})
		if err != nil {
			return RiepilogoR4{}, nil, "", err
		}
		if e := controlliSenzaMotore(rl); e != nil {
			return RiepilogoR4{}, nil, "", e
		}
		return RiepilogoR4{}, nil, "", rifiutoMotore(errM)
	}
	rp, err := Riepilogo3(ctx, qS, thread, r, OpzioniR4{UtenteID: op.UtenteID, AvvisiMotore: true, Motore: &EsitoMotoreR4{Ingresso: ing}})
	if err != nil {
		return RiepilogoR4{}, nil, "", err
	}
	if !rp.AvvisiMotoreCalcolati {
		e := errors.New("motore A: il riepilogo di S non ha gli avvisi del motore")
		if rp.AvvisiMotoreErrore != nil {
			e = fmt.Errorf("motore A: %s", rp.AvvisiMotoreErrore.Testo)
		}
		return RiepilogoR4{}, nil, "", rifiutoMotore(e)
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, qS, thread)
	if err != nil {
		return RiepilogoR4{}, nil, "", err
	}
	p, err := pianificaR5(ctx, qS, thread, r.Snapshot, rp, st)
	if err != nil {
		return RiepilogoR4{}, nil, "", err
	}
	h, err := improntaR5(ctx, qS, thread, r.Snapshot.ProdottoID, op.ImprontaRegole, p)
	if err != nil {
		return RiepilogoR4{}, nil, "", err
	}
	return rp, p, h, nil
}

// rifiutoMotore: il rifiuto non registrato di R5 per il motore (commento di testa): occupato o non pronto 503
// CALCOLO_OCCUPATO, scaduto 504 CALCOLO_SCADUTO, altrimenti 500 ERRORE_CALCOLO; la causa va nel log.
func rifiutoMotore(err error) *RifiutoR5 {
	switch {
	case errors.Is(err, ErrMotoreOccupato), errors.Is(err, ErrMotoreNonPronto):
		return &RifiutoR5{Err: &Errore{Codice: CodiceCalcoloOccupato,
			Messaggio: "il motore e' occupato: la conferma non e' partita, niente e' stato scritto; riprova con la stessa chiave"}, NonRegistrato: true, Causa: err}
	case errors.Is(err, ErrMotoreScaduto):
		return &RifiutoR5{Err: &Errore{Codice: CodiceCalcoloScaduto,
			Messaggio: "il calcolo del motore non e' finito in tempo: niente e' stato scritto; riprova con la stessa chiave"}, NonRegistrato: true, Causa: err}
	default:
		return &RifiutoR5{Err: &Errore{Codice: CodiceErroreCalcolo,
			Messaggio: "il calcolo del motore non e' riuscito: niente e' stato scritto"}, NonRegistrato: true, Causa: err}
	}
}

// improntaR5: H = impronta-ingressi sulle sole parti proposta e analisi, con l'elenco delle righe che R5 scrive escluse
// (EM §9.5 passi 4 e 9; IngressiProposta (2), (2-bis)).
func improntaR5(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, regole string, p *pianoR5) (string, error) {
	r5 := &RigheDiR5{AllegatiCitati: p.allegatiCitati, Proposte: p.proposte, Archi: p.archi, Ricodificati: p.ricodificati}
	prop, err := partePropostaR5(ctx, q, thread, prodotto, regole, r5, p.documentiEsclusi, p.rimozioniEscluse)
	if err != nil {
		return "", err
	}
	an, err := ParteAnalisiDi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	return improntaDi("r5-ingressi", prop.Impronta, an.Impronta), nil
}

// partePropostaR5: PartePropostaDi con le esclusioni di R5 (documenti sostituiti, rimozioni chiuse).
func partePropostaR5(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, regole string, r5 *RigheDiR5, documenti, rimozioni []uuid.UUID) (ParteProposta, error) {
	p := db.IngressiPropostaParams{ThreadID: thread, ProdottoID: prodotto, CitatiDallaBozza: false, AllegatiCitati: r5.AllegatiCitati,
		EscludiProposte: r5.Proposte, EscludiDocumenti: documenti, EscludiRimozioni: rimozioni, EscludiCodiceComponenti: r5.Ricodificati}
	for _, a := range r5.Archi {
		p.EscludiArchi = append(p.EscludiArchi, a.Allegato.String()+"|"+a.Padre+"|"+a.Figlio)
	}
	sql, err := q.IngressiProposta(ctx, p)
	if err != nil {
		return ParteProposta{}, err
	}
	return ParteProposta{Impronta: improntaDi("proposta-2", sql, "regole:"+regole)}, nil
}

// controlliSenzaMotore: i primi due controlli di controlliR5 (base superata, gate della sorgente) sul riepilogo leggero,
// quando il motore e' occupato o non pronto: una bozza senza sorgente S8 valida resta 422 BLOCCHI anche senza il motore.
func controlliSenzaMotore(rp RiepilogoR4) error {
	if rp.Base.Stato == BaseSuperata {
		return &Errore{Codice: CodiceBaseSuperata, Motivi: append([]string(nil), rp.Base.Motivi...), VersioneCorrente: &rp.VersioneCorrente,
			Messaggio: "la distinta del prodotto e' cambiata dopo la tua lettura: la bozza resta, rileggi la base e riconferma"}
	}
	if len(trova3(rp.Blocchi, BloccoStepSorgenteNonConfermata)) > 0 {
		return rifiutoBlocchi(rp.Blocchi)
	}
	return nil
}

// controlliR5: i controlli sul riepilogo di S, senza scrivere (EM §9.5; C3 §5.3 passo 4): base, firma, blocchi, presa
// d'atto dei documenti in analisi, NESSUNA_MODIFICA, la difesa R5_NON_APPLICABILE.
func controlliR5(r RichiestaR5, rp RiepilogoR4, p *pianoR5) error {
	if rp.Base.Stato == BaseSuperata {
		return &Errore{Codice: CodiceBaseSuperata, Motivi: append([]string(nil), rp.Base.Motivi...), VersioneCorrente: &rp.VersioneCorrente,
			Messaggio: "la distinta del prodotto e' cambiata dopo la tua lettura: la bozza resta, rileggi la base e riconferma"}
	}
	// il gate prima della firma [T-39]: una bozza senza sorgente S8 valida e' 422 BLOCCHI anche con un riepilogo superato
	// (per esempio la revoca di B dopo R4), con tutti i blocchi di adesso
	if len(trova3(rp.Blocchi, BloccoStepSorgenteNonConfermata)) > 0 {
		return rifiutoBlocchi(rp.Blocchi)
	}
	if rp.FirmaAnteprima != r.FirmaAnteprima {
		x := rp
		return &RifiutoR5{Err: &Errore{Codice: CodiceAnteprimaSuperata, Motivo: AnteprimaRiepilogoBom, Anteprima: AnteprimaRiepilogoBom,
			Messaggio: "quello che la conferma farebbe e' cambiato da quando hai letto il riepilogo: guarda il riepilogo nuovo e conferma di nuovo"}, Riepilogo: &x}
	}
	if len(rp.Blocchi) > 0 {
		return rifiutoBlocchi(rp.Blocchi)
	}
	if len(rp.FileInAnalisi) > 0 {
		if !r.PresaDAttoPresente {
			return corpoMotivo("presa_d_atto_in_analisi", MotivoPresaDAttoObbligatoria,
				"ci sono documenti ancora in analisi: la conferma vuole la presa d'atto dell'elenco che il riepilogo ha mostrato")
		}
		visti := map[uuid.UUID]bool{}
		for _, a := range r.PresaDAtto {
			visti[a] = true
		}
		for _, f := range rp.FileInAnalisi {
			if !visti[f.AllegatoID] {
				x := rp
				return &RifiutoR5{Err: &Errore{Codice: CodiceAnteprimaSuperata, Motivo: MotivoDocumentiInAnalisiCambiati, Anteprima: AnteprimaRiepilogoBom,
					Messaggio: "i documenti in analisi sono cambiati da quando hai letto il riepilogo: guardali e conferma di nuovo"}, Riepilogo: &x}
			}
		}
	}
	if rp.UgualeAllaBase {
		return &Errore{Codice: CodiceNessunaModifica, Messaggio: "la bozza non cambia niente rispetto alla conferma di base: non c'e' niente da confermare"}
	}
	if len(p.nonApplicabili) > 0 {
		return rifiutoBlocchi(p.nonApplicabili)
	}
	return nil
}

// bloccaR5: i lucchetti di R5 dopo quello della RFQ (gia' preso con la bozza), nell'ordine documentato nel README:
// documenti A e B per documento_id crescente → componente del prodotto → documento_proposta degli allegati citati →
// componente_proposta → relazione_proposta. Il servizio unico delle AR non prende lucchetti propri.
func bloccaR5(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, p *pianoR5) error {
	docs := append([]uuid.UUID(nil), p.documentiDaBloccare...)
	sort.Slice(docs, func(i, j int) bool { return docs[i].String() < docs[j].String() })
	for _, d := range docs {
		if _, err := q.BloccaDocumento(ctx, d); err != nil {
			return err
		}
	}
	if _, err := q.BloccaComponente(ctx, prodotto); err != nil {
		return err
	}
	if len(p.allegatiCitati) > 0 {
		if _, err := q.BloccaDocumentiPropostaR5(ctx, db.BloccaDocumentiPropostaR5Params{ThreadID: uuid.NullUUID{UUID: thread, Valid: true}, Allegati: p.allegatiCitati}); err != nil {
			return err
		}
	}
	if len(p.proposte) > 0 {
		if _, err := q.BloccaComponentiPropostaR5(ctx, db.BloccaComponentiPropostaR5Params{ThreadID: thread, Proposte: p.proposte}); err != nil {
			return err
		}
	}
	if len(p.archi) > 0 {
		chiavi := make([]string, 0, len(p.archi))
		for _, a := range p.archi {
			chiavi = append(chiavi, a.Allegato.String()+"|"+a.Padre+"|"+a.Figlio)
		}
		if _, err := q.BloccaRelazioniPropostaR5(ctx, db.BloccaRelazioniPropostaR5Params{ThreadID: thread, Chiavi: chiavi}); err != nil {
			return err
		}
	}
	return nil
}

// gateInT: la sorgente della versione rispetto al gate (EM App. I.2), letta in T sotto il lucchetto.
func gateInT(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot3) (SorgenteR4, error) {
	if s.Sorgente == nil || s.Sorgente.AllegatoID == nil {
		return SorgenteR4{Motivo: SorgenteMotivoSenzaSorgente}, nil
	}
	a := *s.Sorgente.AllegatoID
	out := SorgenteR4{AllegatoID: &a, Sha256: s.Sorgente.Sha256, Gruppo: s.Sorgente.Gruppo}
	citati, err := LeggiAllegatiCitati(ctx, q, thread, []uuid.UUID{a})
	if err != nil {
		return out, err
	}
	if c := citati[a]; !c.DellaRfq || !c.Attivo || c.Sha256 != s.Sorgente.Sha256 {
		out.Motivo = SorgenteMotivoNonCoerente
		return out, nil
	}
	sv, err := confermabom.SorgenteValida(ctx, q, thread, s.ProdottoID, s.Sorgente.Sha256)
	if err != nil {
		return out, err
	}
	out.Valida, out.Motivo, out.LegameStorico = sv.Valida, sv.Motivo, sv.Storica && !sv.Valida
	if sv.Documento != uuid.Nil {
		d := sv.Documento
		out.DocumentoID = &d
	}
	return out, nil
}

// rilievoGate: il blocco STEP_SORGENTE_NON_CONFERMATA dalla sorgente letta in T (la stessa forma di R4).
func rilievoGate(g SorgenteR4, s Snapshot3) Rilievo {
	mono := false
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneMonoPezzo {
			mono = true
		}
	}
	r := Rilievo{Codice: BloccoStepSorgenteNonConfermata, Motivo: g.Motivo, DocumentoID: g.DocumentoID, LegameStorico: g.LegameStorico,
		MonoPezzo: mono, Testo: testiGate[g.Motivo], Rimedi: []string{"«Conferma questo STEP come sorgente» del prodotto (S8)"}}
	if r.Testo == "" {
		r.Testo = testiGate[SorgenteMotivoNonConfermata]
	}
	if g.AllegatoID != nil {
		r.Sorgente = &RiferimentoFonte{AllegatoID: *g.AllegatoID, Sha256: g.Sha256}
	}
	return r
}

// erroreAlCommit: un rifiuto dei controlli differiti al COMMIT (0020, 0022, 0023) e' 500 CONTROLLO_FINALE con esito
// non_registrato (EM §8.4): il COMMIT non e' avvenuto. 40P01/55P03/40001 come gli altri passi.
func erroreAlCommit(err error) error {
	switch c := sqlstate(err); {
	case c == "40P01" || c == "55P03" || c == "40001":
		return err
	case strings.HasPrefix(c, "P4X") || strings.HasPrefix(c, "23") || strings.HasPrefix(c, "BOM"):
		// il dettaglio tecnico (vincolo, componente) resta nell'errore avvolto per il log, non nel testo per chi legge
		return &RifiutoR5{Err: &Errore{Codice: CodiceControlloFinale, Motivo: c,
			Messaggio: "il controllo finale della conferma non e' passato: niente e' stato registrato"}, NonRegistrato: true, Causa: err}
	}
	return err
}
