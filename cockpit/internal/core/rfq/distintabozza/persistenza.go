package distintabozza

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Gli stati della bozza (distinta_bozza.stato) e quello di R1 senza bozza (contratto rev. 3 §6.4).
const (
	StatoAperta       = "aperta"
	StatoConfermata   = "confermata"
	StatoAbbandonata  = "abbandonata"
	StatoNessunaBozza = "nessuna_bozza"
)

// MaxEventiPagina: quanti eventi del giornale restituisce al piu' una lettura (R1 con ?eventi=1, a pagine: RC-09).
const MaxEventiPagina = 1000

// Avviatore apre una transazione: lo soddisfano *pgxpool.Pool e *pgx.Conn.
type Avviatore interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Base e' la base di una bozza: l'ultima conferma letta, il contatore struttura letto con lei e la firma del
// perimetro letta dal server. Nello snapshot sta in versione_bom_base e firma_base (contratto rev. 3 Δ06); nel
// rebase si scrive cosi' (§6.7). B1 la registra com'e': la firma la ricalcola e la confronta la conferma (B2, §5.1,
// 409 BASE_SUPERATA).
type Base struct {
	ConfermaID *uuid.UUID `json:"conferma_id"`
	Numero     int        `json:"numero"`
	Contatore  int64      `json:"contatore"`
	Firma      string     `json:"firma"`
}

func (b Base) versione() *VersioneBomBase {
	return &VersioneBomBase{ConfermaID: b.ConfermaID, Numero: b.Numero, Contatore: b.Contatore}
}

func (b Base) valida(campo string) *Errore {
	if e := b.versione().valida(campo); e != nil {
		return e
	}
	if !reSha256.MatchString(b.Firma) {
		return corpo(campo+".firma", "la firma della base e' di 64 cifre esadecimali minuscole")
	}
	return nil
}

func baseDelloSnapshot(s Snapshot) Base {
	return Base{ConfermaID: s.VersioneBomBase.ConfermaID, Numero: s.VersioneBomBase.Numero, Contatore: s.VersioneBomBase.Contatore, Firma: s.FirmaBase}
}

// Rebase e' il campo «rebase» della richiesta di R2 (contratto rev. 3 §6.7): la base e la fonte nuove, con il
// motivo. Deve dire la stessa cosa dell'azione rebase.
type Rebase struct {
	Base     Base      `json:"base"`
	Sorgente *Sorgente `json:"sorgente"`
	Motivo   string    `json:"motivo"`
}

// Salvataggio e' una richiesta di R2 gia' decodificata (contratto rev. 3 §6.2): crea (BozzaID nil, VersioneAttesa 0)
// o salva una bozza. Base e fonte stanno nello snapshot (versione_bom_base, firma_base, sorgente): non ci sono campi
// a parte.
type Salvataggio struct {
	ThreadID       uuid.UUID
	ProdottoID     uuid.UUID
	UtenteID       uuid.UUID
	BozzaID        *uuid.UUID
	VersioneAttesa int
	Vocabolario    int
	Snapshot       json.RawMessage
	Azioni         []Azione
	Rebase         *Rebase
}

// Salvato e' l'esito di un salvataggio (contratto rev. 3 §6.2). NuovaVersione falso: lo snapshot era quello della
// versione corrente e non c'erano azioni, quindi nessuna versione nuova.
type Salvato struct {
	BozzaID       uuid.UUID `json:"bozza_id"`
	Versione      int       `json:"versione"`
	NuovaVersione bool      `json:"nuova_versione"`
	Eventi        int       `json:"eventi"`
	Impronta      string    `json:"impronta"`
	SalvataIl     time.Time `json:"salvata_il"`
}

// Salva crea o salva una bozza in una transazione sola: lucchetto della RFQ, lucchetto della testata, controllo
// della versione attesa, riconciliazione delle azioni con lo snapshot precedente (o, per la prima versione, con lo
// stato di partenza ricostruito dal server), controllo delle fonti e delle basi citate, poi la versione nuova
// (snapshot canonico + impronta) e i suoi eventi nella stessa transazione. Salvare non tocca componente,
// componente_relazione, proposte, NAS, F8 (contratto §6.2 «Regole»).
//
// Peso: chi chiama limita il corpo (B2: 2 MiB + 4 KiB, contratto §1.2); qui lo snapshot e' al piu' MaxSnapshot e la
// somma delle azioni al piu' MaxPesoAzioni (RC-18).
func Salva(ctx context.Context, pool Avviatore, s Salvataggio) (Salvato, error) {
	var out Salvato
	if s.Vocabolario != Vocabolario {
		return out, corpo("vocabolario", "vocabolario %d: questo server conosce il vocabolario %d", s.Vocabolario, Vocabolario)
	}
	nuovo, err := LeggiSnapshot(s.Snapshot)
	if err != nil {
		return out, err
	}
	if nuovo.ProdottoID != s.ProdottoID {
		return out, corpo("snapshot.prodotto_id", "lo snapshot e' del prodotto %s, la richiesta del %s", nuovo.ProdottoID, s.ProdottoID)
	}
	if nuovo.VersioneBomBase == nil || nuovo.FirmaBase == "" {
		return out, corpo("snapshot.versione_bom_base", "lo snapshot salvato porta la sua base: versione_bom_base e firma_base")
	}
	if s.VersioneAttesa < 0 || (s.BozzaID == nil && s.VersioneAttesa != 0) {
		return out, corpo("versione_attesa", "una bozza nuova si crea con versione_attesa 0 e senza bozza_id")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)

	if _, err := q.BloccaThread(ctx, s.ThreadID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	prodotto, err := prodottoDellaRfq(ctx, q, s.ThreadID, s.ProdottoID, true)
	if err != nil {
		return out, err
	}

	var (
		bozzaID   uuid.UUID
		corrente  int
		prima     *Snapshot
		improntaP string
		salvataP  time.Time
		n0        int
	)
	if s.BozzaID == nil {
		aperta, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: s.ThreadID, ProdottoID: s.ProdottoID})
		switch {
		case err == nil:
			return out, giaAperta(aperta)
		case !errors.Is(err, pgx.ErrNoRows):
			return out, err
		}
	} else {
		b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: *s.BozzaID, ThreadID: s.ThreadID})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, fuori("bozza_id", "la bozza %s non e' di questa RFQ", *s.BozzaID)
		}
		if err != nil {
			return out, err
		}
		if b.ProdottoID != s.ProdottoID {
			return out, fuori("prodotto_id", "la bozza %s e' di un altro prodotto", b.BozzaID)
		}
		if b.Stato != StatoAperta || int(b.Versione) != s.VersioneAttesa {
			return out, conflittoVersione(b, s.VersioneAttesa)
		}
		v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: s.ThreadID, Versione: b.Versione})
		if err != nil {
			return out, err
		}
		p, imp, err := rileggiSnapshot(v.Snapshot, v.Impronta)
		if err != nil {
			return out, err
		}
		u, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: b.BozzaID, ThreadID: s.ThreadID})
		if err != nil {
			return out, err
		}
		bozzaID, corrente, prima, improntaP, salvataP, n0 = b.BozzaID, int(b.Versione), &p, imp, v.SalvataIl, int(u)
	}

	precedente := func(n int) (Evento, bool, error) {
		if s.BozzaID == nil {
			return Evento{}, false, nil
		}
		e, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: bozzaID, ThreadID: s.ThreadID, N: int32(n)})
		if errors.Is(err, pgx.ErrNoRows) {
			return Evento{}, false, nil
		}
		if err != nil {
			return Evento{}, false, err
		}
		l := eventoLetto(e)
		return Evento{Azione: l.Azione, ChiaveBozza: l.ChiaveBozza, Prima: l.Prima, Dopo: l.Dopo, Origine: l.Origine, Motivo: l.Motivo}, true, nil
	}
	ric, err := Riconcilia(prima, nuovo, s.Azioni, Contesto{N0: n0, Precedente: precedente})
	if err != nil {
		return out, err
	}
	if prima == nil {
		if err := partenzaDalDatabase(ctx, q, s.ThreadID, prodotto, ric); err != nil {
			return out, err
		}
	}
	if e := rebaseCoerente(s.Rebase, ric.Rebase); e != nil {
		return out, e
	}
	canon, err := nuovo.Canonico()
	if err != nil {
		return out, corpo("snapshot", "lo snapshot non ha una forma canonica: %v", err)
	}
	impronta := jsoncanonico.Impronta(canon)
	if prima != nil && impronta == improntaP && len(s.Azioni) == 0 {
		return Salvato{BozzaID: bozzaID, Versione: corrente, Impronta: impronta, SalvataIl: salvataP}, nil
	}

	// le fonti e le basi: quelle dello snapshot e tutte quelle che le azioni nominano, anche di passaggio (RC-05)
	fonti := ric.FontiCitate
	if nuovo.Sorgente != nil {
		fonti = append(fonti, *nuovo.Sorgente)
	}
	for _, f := range fonti {
		if err := fonteDellaRfq(ctx, q, s.ThreadID, f); err != nil {
			return out, err
		}
	}
	base := baseDelloSnapshot(nuovo)
	for _, b := range append([]Base{base}, ric.BasiCitate...) {
		if err := baseDellaRfq(ctx, q, s.ThreadID, s.ProdottoID, b); err != nil {
			return out, err
		}
	}
	if ric.Rebase != nil {
		if err := baseCorrente(ctx, q, s.ThreadID, s.ProdottoID, ric.Rebase.Base); err != nil {
			return out, err
		}
	}

	fonteAll, fonteSha, fonteGruppo := colonneFonte(nuovo.Sorgente)
	if s.BozzaID == nil {
		r, err := q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{
			ThreadID: s.ThreadID, ProdottoID: s.ProdottoID, BaseConfermaID: nullUUID(base.ConfermaID),
			BaseNumero: int32(base.Numero), BaseContatore: base.Contatore, FirmaBase: base.Firma,
			FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, UtenteID: s.UtenteID,
		})
		if err != nil {
			if vincolo(err) == "ux_distinta_bozza_aperta" {
				// sotto il lucchetto della RFQ e' quasi irraggiungibile; la transazione e' annullata e non si rilegge la
				// bozza che l'ha fermata (RC-15): la pagina rilegge con R1
				return out, &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoAltraBozzaAperta, Messaggio: "il prodotto ha gia' una bozza aperta: rileggila"}
			}
			return out, erroreDiScrittura(err, "snapshot")
		}
		bozzaID = r.BozzaID
	}
	versione := corrente + 1
	salvataIl, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{
		BozzaID: bozzaID, Versione: int32(versione), Snapshot: canon, Impronta: impronta,
		BaseConfermaID: nullUUID(base.ConfermaID), BaseNumero: int32(base.Numero), BaseContatore: base.Contatore,
		FirmaBase: base.Firma, FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, SalvataDa: s.UtenteID,
	})
	if err != nil {
		if vincolo(err) == "distinta_bozza_versione_pkey" {
			// come sopra: sotto i due lucchetti non dovrebbe succedere; RC-15
			return out, &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoVersioneDiversa, Messaggio: "un altro salvataggio e' arrivato prima: rileggi la bozza"}
		}
		return out, erroreDiScrittura(err, "snapshot")
	}
	for i, ev := range ric.Eventi {
		if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{
			BozzaID: bozzaID, N: int32(n0 + i + 1), Versione: int32(versione), Vocabolario: Vocabolario,
			Azione: ev.Azione, ChiaveBozza: testo(ev.ChiaveBozza), Prima: rawONull(ev.Prima), Dopo: rawONull(ev.Dopo),
			Origine: ev.Origine, Motivo: testo(ev.Motivo), UtenteID: s.UtenteID,
		}); err != nil {
			return out, erroreDiScrittura(err, fmt.Sprintf("azioni[%d]", i))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if sqlstate(err) == "P4X04" {
			// il trigger differito della fonte: un cambio di fonte senza evento motivato. Il Go lo previene (motivo
			// obbligatorio su sorgente_cambiata e rebase); se scatta lo stesso, una frase fissa (RC-15)
			return out, nonRiconciliate("azioni", "il database rifiuta la versione: la fonte cambia senza un evento motivato")
		}
		return out, err
	}
	return Salvato{BozzaID: bozzaID, Versione: versione, NuovaVersione: true, Eventi: len(ric.Eventi), Impronta: impronta, SalvataIl: salvataIl}, nil
}

// rebaseCoerente: il campo «rebase» della richiesta e l'azione rebase dicono la stessa cosa, o non ci sono tutti e
// due (contratto rev. 3 §6.7).
func rebaseCoerente(campo *Rebase, azione *RebaseDichiarato) *Errore {
	switch {
	case campo == nil && azione == nil:
		return nil
	case campo == nil:
		return corpo("rebase", "l'azione rebase vuole il campo rebase della richiesta, con la stessa base, fonte e motivo")
	case azione == nil:
		return corpo("rebase", "il campo rebase vuole l'azione rebase fra le azioni")
	}
	if !uguali(campo.Base, azione.Base) || !uguali(sorgenteNFC(campo.Sorgente), azione.Sorgente) || strings.TrimSpace(nfc(campo.Motivo)) != azione.Motivo {
		return corpo("rebase", "il campo rebase e l'azione rebase non dicono la stessa base, fonte e motivo")
	}
	return nil
}

// partenzaDalDatabase: per la prima versione, lo stato di partenza ricostruito dal server (contratto rev. 3 §6.6) e
// il confronto con quello che dicono le azioni (RC-02). In B1 si ricostruiscono «vuota» (la sola radice) e
// «registrato» (il perimetro della working); «conferma» e «step» vogliono lo snapshot dell'ultima conferma f.2 e la
// proposta del gruppo del Motore A, che arrivano con B2: fino ad allora 422 partenza_non_ricostruibile, mai una
// prima versione accettata senza confronto.
func partenzaDalDatabase(ctx context.Context, q *db.Queries, thread uuid.UUID, prodotto db.GetProdottoDellaRfqRow, ric Riconciliazione) error {
	if ric.StatoDiPartenza == nil {
		return corpo("azioni", "la prima versione comincia con bozza_aperta")
	}
	reg := Registrato{Prodotto: prodotto.ComponenteID, Componenti: []ComponenteRegistrato{{
		ComponenteID: prodotto.ComponenteID, Codice: prodotto.Codice, Rev: prodotto.Rev.String,
		Descrizione: prodotto.Descrizione.String, Tipo: string(prodotto.Tipo)}}}
	switch ric.Partenza {
	case PartenzaVuota:
	case PartenzaRegistrato:
		cc, err := q.ListPerimetroRegistrato(ctx, db.ListPerimetroRegistratoParams{ThreadID: thread, ProdottoID: prodotto.ComponenteID})
		if err != nil {
			return err
		}
		aa, err := q.ListArchiPerimetroRegistrato(ctx, db.ListArchiPerimetroRegistratoParams{ThreadID: thread, ProdottoID: prodotto.ComponenteID})
		if err != nil {
			return err
		}
		reg.Componenti = reg.Componenti[:0]
		for _, c := range cc {
			reg.Componenti = append(reg.Componenti, ComponenteRegistrato{ComponenteID: c.ComponenteID, Codice: c.Codice,
				Rev: c.Rev.String, Descrizione: c.Descrizione.String, Tipo: string(c.Tipo)})
		}
		for _, a := range aa {
			reg.Archi = append(reg.Archi, ArcoRegistrato{Padre: a.PadreID, Figlio: a.FiglioID, Qta: int(a.Qta), Posizione: a.Posizione.String})
		}
	default:
		return &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoPartenzaNonRicostruibile, Campo: "azioni[0].dopo.partenza",
			Messaggio: fmt.Sprintf("la partenza %q non si ricostruisce ancora sul server (B2: ultima conferma formato 2, proposta del Motore A): si parte da «vuota» o «registrato»", ric.Partenza)}
	}
	if e := ControllaPartenza(ric.Partenza, *ric.StatoDiPartenza, reg); e != nil {
		return e
	}
	return nil
}

// prodottoDellaRfq: il prodotto e' un componente della RFQ, di tipo finito e (per scrivere) non archiviato. Solo
// «non trovato» e' un FUORI_RFQ; ogni altro errore del database torna com'e' (RC-10).
func prodottoDellaRfq(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, attivo bool) (db.GetProdottoDellaRfqRow, error) {
	p, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: thread, ComponenteID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		return p, fuori("prodotto_id", "il prodotto %s non e' un componente di questa RFQ", prodotto)
	}
	if err != nil {
		return p, err
	}
	if p.Tipo != db.TipoComponenteFinito || (attivo && p.ArchiviatoIl != nil) {
		return p, fuori("prodotto_id", "%s non e' un prodotto finito attivo della RFQ", p.Codice)
	}
	return p, nil
}

// fonteDellaRfq: l'allegato citato e' un file della RFQ, e lo sha256, quando la citazione lo porta, e' il suo.
func fonteDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID, f Sorgente) error {
	if f.AllegatoID == nil {
		return nil
	}
	a, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{ThreadID: thread, AllegatoID: *f.AllegatoID})
	if errors.Is(err, pgx.ErrNoRows) {
		return fuori("sorgente.allegato_id", "l'allegato %s non e' un file di questa RFQ", *f.AllegatoID)
	}
	if err != nil {
		return err
	}
	if f.Sha256 != "" && (!a.Sha256.Valid || strings.TrimSpace(a.Sha256.String) != f.Sha256) {
		return corpo("sorgente.sha256", "lo sha256 dichiarato non e' quello del file %s", a.NomeFile)
	}
	return nil
}

// baseDellaRfq: la conferma di una base e' una conferma di questo prodotto della RFQ, con quel numero.
func baseDellaRfq(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, b Base) error {
	if b.ConfermaID == nil {
		return nil
	}
	c, err := q.GetConfermaStrutturaleBase(ctx, db.GetConfermaStrutturaleBaseParams{ThreadID: thread, ConfermaID: *b.ConfermaID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ProdottoID != prodotto) {
		return fuori("snapshot.versione_bom_base.conferma_id", "la conferma %s non e' di questo prodotto della RFQ", *b.ConfermaID)
	}
	if err != nil {
		return err
	}
	if int(c.Numero) != b.Numero {
		return corpo("snapshot.versione_bom_base.numero", "la conferma di base e' la n. %d, non la %d", c.Numero, b.Numero)
	}
	return nil
}

// baseCorrente: la base di un rebase e' quella corrente del prodotto: l'ultima conferma e il contatore struttura
// della working (contratto rev. 3 §6.7, 409 BASE_SUPERATA). La firma del perimetro la ricalcola B2.
func baseCorrente(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, b Base) error {
	var motivi []string
	ultima, err := q.GetUltimaConfermaStrutturale(ctx, db.GetUltimaConfermaStrutturaleParams{ThreadID: thread, ProdottoID: prodotto})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if b.ConfermaID != nil {
			motivi = append(motivi, MotivoConfermaCambiata)
		}
	case err != nil:
		return err
	default:
		if b.ConfermaID == nil || *b.ConfermaID != ultima.ConfermaID || int32(b.Numero) != ultima.Numero {
			motivi = append(motivi, MotivoConfermaCambiata)
		}
	}
	wc, err := q.GetWorkingContatore(ctx, thread)
	if err != nil {
		return err
	}
	if wc.Struttura != b.Contatore {
		motivi = append(motivi, MotivoStrutturaCambiata)
	}
	if len(motivi) > 0 {
		return &Errore{Codice: CodiceBaseSuperata, Campo: "rebase.base", Motivi: motivi,
			Messaggio: "la base del rebase non e' quella corrente del prodotto: rileggi la base e rifai il rebase"}
	}
	return nil
}

func giaAperta(b db.DistintaBozza) *Errore {
	return &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoAltraBozzaAperta,
		Messaggio: fmt.Sprintf("il prodotto ha gia' una bozza aperta (versione %d)", b.Versione),
		Conflitto: &Conflitto{BozzaID: b.BozzaID, Stato: b.Stato, Versione: int(b.Versione), AggiornataDa: b.AggiornataDa, AggiornataIl: b.AggiornataIl}}
}

func conflittoVersione(b db.DistintaBozza, attesa int) *Errore {
	motivo, msg := MotivoVersioneDiversa, fmt.Sprintf("la bozza e' alla versione %d, non alla %d: rileggila", b.Versione, attesa)
	if b.Stato != StatoAperta {
		motivo, msg = MotivoBozzaChiusa, fmt.Sprintf("la bozza e' %s: non si salva piu'", b.Stato)
	}
	return &Errore{Codice: CodiceBozzaVersione, Motivo: motivo, Messaggio: msg,
		Conflitto: &Conflitto{BozzaID: b.BozzaID, Stato: b.Stato, Versione: int(b.Versione), AggiornataDa: b.AggiornataDa, AggiornataIl: b.AggiornataIl}}
}

// rileggiSnapshot rilegge uno snapshot salvato (jsonb: PostgreSQL ne cambia l'ordine delle chiavi e gli spazi) e ne
// ricontrolla l'impronta: la forma canonica rifatta deve dare l'impronta registrata. Una differenza e' un guasto dei
// dati salvati (ErrSnapshotIncoerente), non un corpo sbagliato (RC-01).
func rileggiSnapshot(raw json.RawMessage, impronta string) (Snapshot, string, error) {
	s, err := leggiSnapshot(raw, 0)
	if err != nil {
		return s, "", fmt.Errorf("%w: non si rilegge (%v)", ErrSnapshotIncoerente, err)
	}
	imp, err := s.Impronta()
	if err != nil {
		return s, "", fmt.Errorf("%w: %v", ErrSnapshotIncoerente, err)
	}
	if imp != strings.TrimSpace(impronta) {
		return s, "", fmt.Errorf("%w: impronta %s, registrata %s", ErrSnapshotIncoerente, imp, impronta)
	}
	return s, imp, nil
}

// erroreDiScrittura traduce i rifiuti del database che dipendono dal corpo (e che il Go non puo' prevedere del
// tutto: il peso del jsonb) in CORPO_NON_VALIDO con il campo; ogni altro errore torna com'e' (RC-07).
func erroreDiScrittura(err error, campo string) error {
	switch sqlstate(err) {
	case "22P05", "22021": // un carattere che il database non accetta (U+0000 in jsonb o in un testo)
		return corpo(campo, "il database non accetta un carattere del testo (U+0000)")
	case "54000": // valore oltre i limiti di jsonb
		return corpo(campo, "il valore e' oltre i limiti del database")
	case "23514":
		switch vincolo(err) {
		case "ck_dbv_snapshot":
			return corpo("snapshot", "lo snapshot pesa troppo per il database (oltre 4 MiB come jsonb)")
		case "ck_dbe_prima":
			return corpo(campo+".prima", "prima pesa troppo per il database (oltre 256 KiB come jsonb)")
		case "ck_dbe_dopo":
			return corpo(campo+".dopo", "dopo pesa troppo per il database (oltre 256 KiB come jsonb)")
		}
	}
	return err
}

func colonneFonte(s *Sorgente) (uuid.NullUUID, pgtype.Text, pgtype.Text) {
	if s == nil {
		return uuid.NullUUID{}, pgtype.Text{}, pgtype.Text{}
	}
	var a uuid.NullUUID
	if s.AllegatoID != nil {
		a = uuid.NullUUID{UUID: *s.AllegatoID, Valid: true}
	}
	return a, testo(s.Sha256), testo(s.Gruppo)
}

func nullUUID(u *uuid.UUID) uuid.NullUUID {
	if u == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *u, Valid: true}
}

func testo(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func rawONull(r json.RawMessage) *json.RawMessage {
	if nullo(r) {
		return nil
	}
	return &r
}

func vincolo(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.ConstraintName
	}
	return ""
}

func sqlstate(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// ------------------------------------------------------------------ l'abbandono (R3)

// Abbandono e' una richiesta di R3 gia' decodificata.
type Abbandono struct {
	ThreadID       uuid.UUID
	BozzaID        uuid.UUID
	UtenteID       uuid.UUID
	VersioneAttesa int
	Motivo         string
}

// Abbandonata e' l'esito dell'abbandono.
type Abbandonata struct {
	Stato    string    `json:"stato"`
	BozzaID  uuid.UUID `json:"bozza_id"`
	Versione int       `json:"versione"`
	ChiusaIl time.Time `json:"chiusa_il"`
}

// Abbandona chiude una bozza aperta alla versione attesa come «abbandonata» (contratto rev. 3 §6.5). Le versioni e
// il giornale restano (la conservazione di 90 giorni la applica il database; il registro della bozza resta per
// sempre); i file caricati restano proposte.
func Abbandona(ctx context.Context, pool Avviatore, a Abbandono) (Abbandonata, error) {
	var out Abbandonata
	motivo := strings.TrimSpace(nfc(a.Motivo))
	if motivo != "" && !testoLibero(motivo, MaxMotivo) {
		return out, corpo("motivo", "il motivo sta in %d caratteri, senza caratteri di controllo", MaxMotivo)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, a.ThreadID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: a.BozzaID, ThreadID: a.ThreadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("bozza_id", "la bozza %s non e' di questa RFQ", a.BozzaID)
	}
	if err != nil {
		return out, err
	}
	if b.Stato != StatoAperta || int(b.Versione) != a.VersioneAttesa {
		return out, conflittoVersione(b, a.VersioneAttesa)
	}
	n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{
		ChiusaDa: uuid.NullUUID{UUID: a.UtenteID, Valid: true}, Motivo: testo(motivo), BozzaID: a.BozzaID, ThreadID: a.ThreadID, Versione: b.Versione,
	})
	if err != nil {
		return out, err
	}
	if n != 1 {
		return out, conflittoVersione(b, a.VersioneAttesa)
	}
	letta, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: a.BozzaID, ThreadID: a.ThreadID})
	if err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	out = Abbandonata{Stato: StatoAbbandonata, BozzaID: a.BozzaID, Versione: int(b.Versione)}
	if letta.ChiusaIl != nil {
		out.ChiusaIl = *letta.ChiusaIl
	}
	return out, nil
}

// ------------------------------------------------------------------ le letture (R1)

// Lettura e' la risposta di R1 (contratto rev. 3 §6.4): la bozza aperta del prodotto, o nessuna. In JSON i campi
// della bozza stanno al primo livello accanto a «stato». versione_corrente, base e rif_scaduti di §6.4 vogliono la
// firma del perimetro e la fonte corrente: li aggiunge B2.
type Lettura struct {
	Stato string `json:"stato"`
	*BozzaLetta
}

// BozzaLetta e' una bozza con la versione corrente (snapshot canonico, ricontrollato con l'impronta) e il numero
// dei suoi eventi; l'elenco degli eventi si legge a pagine con LeggiEventi (R1 ?eventi=1, RC-09).
type BozzaLetta struct {
	BozzaID        uuid.UUID       `json:"bozza_id"`
	ProdottoID     uuid.UUID       `json:"prodotto_id"`
	Stato          string          `json:"stato"`
	Versione       int             `json:"versione"`
	Snapshot       json.RawMessage `json:"snapshot"`
	Impronta       string          `json:"impronta"`
	Base           Base            `json:"base_salvata"`
	Fonte          *Sorgente       `json:"fonte,omitempty"`
	CreataDa       uuid.UUID       `json:"creata_da"`
	CreataIl       time.Time       `json:"creata_il"`
	AggiornataDa   uuid.UUID       `json:"aggiornata_da"`
	AggiornataIl   time.Time       `json:"aggiornata_il"`
	ChiusaDa       *uuid.UUID      `json:"chiusa_da,omitempty"`
	ChiusaIl       *time.Time      `json:"chiusa_il,omitempty"`
	MotivoChiusura string          `json:"motivo_chiusura,omitempty"`
	Eventi         int             `json:"eventi"`
}

// EventoLetto e' un evento del giornale come si rilegge.
type EventoLetto struct {
	N           int             `json:"n"`
	Versione    int             `json:"versione"`
	Vocabolario int             `json:"vocabolario"`
	Azione      string          `json:"azione"`
	ChiaveBozza string          `json:"chiave_bozza,omitempty"`
	Prima       json.RawMessage `json:"prima,omitempty"`
	Dopo        json.RawMessage `json:"dopo,omitempty"`
	Origine     string          `json:"origine"`
	Motivo      string          `json:"motivo,omitempty"`
	UtenteID    uuid.UUID       `json:"autore"`
	Quando      time.Time       `json:"quando"`
}

// Leggi restituisce la bozza aperta di un prodotto della RFQ (R1), o lo stato nessuna_bozza. Prima il perimetro,
// con gli stessi codici di Salva: RFQ che non c'e' → RFQ_INESISTENTE; prodotto che non e' un finito della RFQ →
// FUORI_RFQ (un finito archiviato si legge ancora). La lettura e' coerente senza una transazione: testata, poi la
// versione che la testata nomina e gli eventi fino a quella versione, che non si riscrivono (RC-09).
func Leggi(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID) (Lettura, error) {
	if _, err := q.GetThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Lettura{}, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return Lettura{}, err
	}
	if _, err := prodottoDellaRfq(ctx, q, thread, prodotto, false); err != nil {
		return Lettura{}, err
	}
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lettura{Stato: StatoNessunaBozza}, nil
	}
	if err != nil {
		return Lettura{}, err
	}
	bl, err := leggiBozza(ctx, q, b)
	if err != nil {
		return Lettura{}, err
	}
	return Lettura{Stato: StatoAperta, BozzaLetta: &bl}, nil
}

// LeggiBozza restituisce una bozza della RFQ in qualunque stato, con la versione corrente.
func LeggiBozza(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID) (BozzaLetta, error) {
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: bozza, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return BozzaLetta{}, fuori("bozza_id", "la bozza %s non e' di questa RFQ", bozza)
	}
	if err != nil {
		return BozzaLetta{}, err
	}
	return leggiBozza(ctx, q, b)
}

func leggiBozza(ctx context.Context, q *db.Queries, b db.DistintaBozza) (BozzaLetta, error) {
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: b.ThreadID, Versione: b.Versione})
	if err != nil {
		return BozzaLetta{}, err
	}
	s, imp, err := rileggiSnapshot(v.Snapshot, v.Impronta)
	if err != nil {
		return BozzaLetta{}, err
	}
	canon, err := s.Canonico()
	if err != nil {
		return BozzaLetta{}, err
	}
	eventi, err := q.ContaDistintaBozzaEventi(ctx, db.ContaDistintaBozzaEventiParams{BozzaID: b.BozzaID, ThreadID: b.ThreadID, VersioneMax: b.Versione})
	if err != nil {
		return BozzaLetta{}, err
	}
	out := BozzaLetta{
		BozzaID: b.BozzaID, ProdottoID: b.ProdottoID, Stato: b.Stato, Versione: int(b.Versione), Snapshot: canon, Impronta: imp,
		Base:     baseDaColonne(v.BaseConfermaID, v.BaseNumero, v.BaseContatore, v.FirmaBase),
		Fonte:    fonteDaColonne(v.FonteAllegatoID, v.FonteSha256, v.FonteGruppo),
		CreataDa: b.CreataDa, CreataIl: b.CreataIl, AggiornataDa: v.SalvataDa, AggiornataIl: v.SalvataIl,
		ChiusaIl: b.ChiusaIl, MotivoChiusura: b.MotivoChiusura.String, Eventi: int(eventi),
	}
	if b.ChiusaDa.Valid {
		u := b.ChiusaDa.UUID
		out.ChiusaDa = &u
	}
	return out, nil
}

// LeggiEventi restituisce il giornale di una bozza della RFQ a pagine: gli eventi dopo il numero dopoN, al piu'
// limite (0 o oltre MaxEventiPagina: MaxEventiPagina), fino alla versione corrente della testata.
func LeggiEventi(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID, dopoN, limite int) ([]EventoLetto, error) {
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: bozza, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fuori("bozza_id", "la bozza %s non e' di questa RFQ", bozza)
	}
	if err != nil {
		return nil, err
	}
	if limite <= 0 || limite > MaxEventiPagina {
		limite = MaxEventiPagina
	}
	ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: bozza, ThreadID: thread,
		VersioneMax: b.Versione, DopoN: int32(max(dopoN, 0)), Limite: int32(limite)})
	if err != nil {
		return nil, err
	}
	out := make([]EventoLetto, 0, len(ee))
	for _, e := range ee {
		out = append(out, eventoLetto(e))
	}
	return out, nil
}

func eventoLetto(e db.DistintaBozzaEvento) EventoLetto {
	ev := EventoLetto{N: int(e.N), Versione: int(e.Versione), Vocabolario: int(e.Vocabolario), Azione: e.Azione,
		ChiaveBozza: e.ChiaveBozza.String, Origine: e.Origine, Motivo: e.Motivo.String, UtenteID: e.UtenteID, Quando: e.Quando}
	if e.Prima != nil {
		ev.Prima = canonico(*e.Prima)
	}
	if e.Dopo != nil {
		ev.Dopo = canonico(*e.Dopo)
	}
	return ev
}

func baseDaColonne(id uuid.NullUUID, numero int32, contatore int64, firma string) Base {
	b := Base{Numero: int(numero), Contatore: contatore, Firma: strings.TrimSpace(firma)}
	if id.Valid {
		u := id.UUID
		b.ConfermaID = &u
	}
	return b
}

func fonteDaColonne(all uuid.NullUUID, sha, gruppo pgtype.Text) *Sorgente {
	if !all.Valid && !gruppo.Valid {
		return nil
	}
	s := &Sorgente{Gruppo: gruppo.String, Sha256: strings.TrimSpace(sha.String)}
	if all.Valid {
		u := all.UUID
		s.AllegatoID = &u
	}
	return s
}

// ------------------------------------------------------------------ la storia e il rifacimento del giornale

// Storia sono tutte le versioni di una bozza e il suo giornale, per rifare il giornale versione per versione.
type Storia struct {
	BozzaID  uuid.UUID
	Versioni []VersioneLetta
	Eventi   []EventoLetto
}

// VersioneLetta e' una versione salvata: lo snapshot riletto (ricontrollato con l'impronta).
type VersioneLetta struct {
	Versione  int
	Snapshot  Snapshot
	Impronta  string
	SalvataDa uuid.UUID
	SalvataIl time.Time
}

// LeggiStoria legge tutte le versioni e il giornale di una bozza della RFQ. E' una lettura di diagnosi e delle prove
// (VerificaGiornale), senza tetto: nessuna rotta la espone (RC-17).
func LeggiStoria(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID) (Storia, error) {
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: bozza, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return Storia{}, fuori("bozza_id", "la bozza %s non e' di questa RFQ", bozza)
	}
	if err != nil {
		return Storia{}, err
	}
	vv, err := q.ListDistintaBozzaVersioni(ctx, db.ListDistintaBozzaVersioniParams{BozzaID: bozza, ThreadID: thread})
	if err != nil {
		return Storia{}, err
	}
	st := Storia{BozzaID: bozza}
	for _, v := range vv {
		s, imp, err := rileggiSnapshot(v.Snapshot, v.Impronta)
		if err != nil {
			return Storia{}, fmt.Errorf("versione %d: %w", v.Versione, err)
		}
		st.Versioni = append(st.Versioni, VersioneLetta{Versione: int(v.Versione), Snapshot: s, Impronta: imp, SalvataDa: v.SalvataDa, SalvataIl: v.SalvataIl})
	}
	ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: bozza, ThreadID: thread,
		VersioneMax: b.Versione, DopoN: 0, Limite: math.MaxInt32})
	if err != nil {
		return Storia{}, err
	}
	for _, e := range ee {
		st.Eventi = append(st.Eventi, eventoLetto(e))
	}
	return st, nil
}

// VerificaGiornale rifa' il giornale: per ogni versione, gli eventi di quella versione applicati alla versione
// precedente (alla prima: lo stato di partenza che le sue azioni descrivono) devono dare esattamente lo snapshot
// salvato. E' la prova che eventi e snapshot non divergono (ratifica 05 §2, direttiva 06 B1), anche dopo un
// riavvio. Gli eventi sono numerati senza buchi, in ordine di versione, tutti del vocabolario 1 (RC-16): un evento
// di un vocabolario che questo codice non conosce non si rifa' con le regole del vocabolario 1.
func VerificaGiornale(st Storia) error {
	perVersione := map[int][]Azione{}
	perNumero := map[int]Evento{}
	primoDi := map[int]int{} // la versione v comincia dopo l'evento primoDi[v]
	ultimaVersione := 0
	for i, e := range st.Eventi {
		if e.N != i+1 {
			return fmt.Errorf("eventi non numerati in ordine: alla posizione %d l'evento %d", i, e.N)
		}
		if e.Vocabolario != Vocabolario {
			return fmt.Errorf("evento %d del vocabolario %d: questo codice rifa' solo il vocabolario %d", e.N, e.Vocabolario, Vocabolario)
		}
		if e.Versione < ultimaVersione {
			return fmt.Errorf("evento %d della versione %d dopo eventi della versione %d", e.N, e.Versione, ultimaVersione)
		}
		if _, visto := primoDi[e.Versione]; !visto {
			primoDi[e.Versione] = e.N - 1
		}
		ultimaVersione = e.Versione
		perVersione[e.Versione] = append(perVersione[e.Versione], Azione{Azione: e.Azione, ChiaveBozza: e.ChiaveBozza,
			Prima: e.Prima, Dopo: e.Dopo, Origine: e.Origine, Motivo: e.Motivo})
		perNumero[e.N] = Evento{Azione: e.Azione, ChiaveBozza: e.ChiaveBozza, Prima: e.Prima, Dopo: e.Dopo, Origine: e.Origine, Motivo: e.Motivo}
	}
	precedente := func(n int) (Evento, bool, error) {
		ev, ok := perNumero[n]
		return ev, ok, nil
	}
	var prima *Snapshot
	for i, v := range st.Versioni {
		if v.Versione != i+1 {
			return fmt.Errorf("versioni non consecutive: alla posizione %d la versione %d", i, v.Versione)
		}
		n0, ok := primoDi[v.Versione]
		if !ok { // una versione senza eventi: solo la prima non puo' esserlo (bozza_aperta)
			n0 = len(st.Eventi)
			for _, e := range st.Eventi {
				if e.Versione > v.Versione {
					n0 = e.N - 1
					break
				}
			}
		}
		if _, err := riconcilia(prima, v.Snapshot, perVersione[v.Versione], Contesto{N0: n0, Precedente: precedente}, originiTutte); err != nil {
			return fmt.Errorf("versione %d: %w", v.Versione, err)
		}
		s := v.Snapshot
		prima = &s
		delete(perVersione, v.Versione)
	}
	for v := range perVersione {
		return fmt.Errorf("eventi della versione %d, che non c'e'", v)
	}
	return nil
}
