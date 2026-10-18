package distintabozza

// R2, forma «salvataggio», nel formato 3 (EM §9.2; PM23 §14 passo 6, prima parte): la versione n+1 di una bozza formato 3
// aperta, con il vocabolario 2. L'ordine dei controlli e' quello di EM §9.2, uguale per le cinque forme: BloccaThread →
// la chiave del salvataggio (globale: stessa chiave e stessa impronta della richiesta = riproposto, senza una versione
// nuova; impronta diversa = 409 CHIAVE_RIUSATA) → versione attesa e bozza aperta → la forma dello snapshot, la
// riconciliazione delle azioni (riconcilia3.go), le verifiche contro il database e i registri → la scrittura della
// versione (snapshot dalla chiusura dello scrittore, Snapshot3.Canonico) e dei suoi eventi. Mai componente,
// componente_relazione, proposte, associazione_evento, correzione_documento, NAS o F8 (C3 §6.2).
//
// Le verifiche contro il database e i registri che fa il salvataggio (EM §6.2, §6.4, §5.6, §3.4.2), sui riferimenti nuovi
// o cambiati rispetto alla versione precedente (un riferimento gia' salvato e scaduto dopo passa: lo dicono R1 e R4):
//   - appartenenza: ogni UUID citato e' della RFQ del percorso (componenti, allegati delle caselle e delle non_includere,
//     proposte delle preassociazioni e degli svincoli, l'allegato di una chiave_lettura, le fonti viste) → 422 FUORI_RFQ;
//   - occorrenze: le caselle, le non_includere e le preassociazioni nuove o cambiate stanno su occorrenze attive →
//     422 FILE_NON_ATTIVO; il rif ha lo sha256 dell'allegato;
//   - i registri citati esistono, della RFQ e del prodotto, e una decisione nuova cita il registro della versione (V-2) →
//     422 CORPO_NON_VALIDO importazione_sconosciuta;
//   - V-1: un nodo in modo proposta porta l'identita' proposta del registro della versione (rev_proposta vuota = rev
//     assente, PE-11); un nodo da_decidere ha una voce senza codice proposto (EM §3.4.2); un nodo che un nodo_aggiunto
//     mette nella bozza con una casella ha il modo delle regole P-2...P-6 (EM §6.1, l'importazione a mano); un arco step
//     nuovo o cambiato e' un arco del registro della versione, o dei fatti correnti dello STEP senza registro (EM §3.5);
//     la lettura copiata da una decisione identita e' quella del suo registro; V-3: ogni casella cita una voce intera del
//     registro; V-5: il segnale visto di una non_includere e' quello del registro; per una non_includere nuova senza
//     importazione, V-3 e V-5 contro il registro della versione (EM §7.1: per una decisione nuova e' quello) (PE-17);
//   - gli UUID e i registri che compaiono solo nelle azioni (un nodo aggiunto e tolto nello stesso salvataggio): della
//     RFQ anche loro (EM §9, «ovunque sia annidato»);
//   - la regola unica dell'emittente (§5.6) → emittente_non_ammesso; l'identita' registrata dei nodi in modo registrato
//     (codice di oggi, certificazione valida o cliente presunto) → registrato_non_corrispondente; i nodi formato_2 come
//     nella versione 1 della bozza convertita → formato_2_non_corrispondente;
//   - il rebase della sola base: la base nuova e' quella corrente (409 BASE_SUPERATA) e dopo il rebase non restano rif
//     scaduti (422 CORPO_NON_VALIDO rif_scaduti).
//
// Le altre quattro forme di R2 (crea, cambia_sorgente, riallinea_proposta, converti_da) arrivano con l'ondata C: le prime
// tre vogliono l'adattatore propostabom (passo 3), converti_da e' pronta come servizio (Converti, conversione.go).

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
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I codici e i motivi nuovi del salvataggio formato 3 (EM §9.2).
const (
	// CodiceChiaveRiusata: la chiave del salvataggio (o d'intento) e' gia' usata con un altro corpo (409).
	CodiceChiaveRiusata = "CHIAVE_RIUSATA"
	// CodiceBozzaAltroProdotto: la bozza del corpo e' di un altro prodotto (422).
	CodiceBozzaAltroProdotto = "BOZZA_ALTRO_PRODOTTO"
	// CodiceFileNonAttivo: un'occorrenza in Altri file o scartata, nuova o cambiata nel salvataggio (422, EM §10.8).
	CodiceFileNonAttivo = "FILE_NON_ATTIVO"

	MotivoFormatoDaConvertire       = "formato_da_convertire"
	MotivoFormato2NonCorrispondente = "formato_2_non_corrispondente"
	MotivoFormaNonDisponibile       = "forma_non_disponibile"
)

// RebaseRichiesta3 e' il campo «rebase» della richiesta di R2 nel formato 3 (EM §9.2): la base nuova e il motivo, gli
// stessi dell'azione rebase.
type RebaseRichiesta3 struct {
	Base   RebaseV2 `json:"base"`
	Motivo string   `json:"motivo"`
}

// Salvataggio3 e' una richiesta di R2 nella forma salvataggio, gia' letta (richieste3.go).
type Salvataggio3 struct {
	ThreadID, ProdottoID, UtenteID uuid.UUID
	BozzaID                        uuid.UUID
	VersioneAttesa                 int
	ChiaveSalvataggio              uuid.UUID
	// ImprontaRichiesta: lo sha256 della forma canonica del corpo della richiesta (il reinvio con lo stesso corpo e' lo
	// stesso salvataggio).
	ImprontaRichiesta string
	Snapshot          json.RawMessage
	Azioni            []Azione
	Rebase            *RebaseRichiesta3
}

// Salvato3 e' la risposta di R2 (EM §9.2): {bozza_id, formato, versione, nuova_versione, riproposto, eventi, impronta,
// salvata_il}.
type Salvato3 struct {
	BozzaID       uuid.UUID `json:"bozza_id"`
	Formato       int       `json:"formato"`
	Versione      int       `json:"versione"`
	NuovaVersione bool      `json:"nuova_versione"`
	Riproposto    bool      `json:"riproposto"`
	Eventi        int       `json:"eventi"`
	Impronta      string    `json:"impronta"`
	SalvataIl     time.Time `json:"salvata_il"`
}

// SalvaFormato3 salva una versione nuova di una bozza formato 3 aperta (commento di testa).
func SalvaFormato3(ctx context.Context, pool Avviatore, s Salvataggio3) (Salvato3, error) {
	var out Salvato3
	if s.ChiaveSalvataggio == uuid.Nil || s.ChiaveSalvataggio.Version() != 4 {
		return out, corpo("chiave_salvataggio", "la chiave del salvataggio e' un UUID v4")
	}
	if !reSha256.MatchString(s.ImprontaRichiesta) {
		return out, corpo("richiesta", "manca l'impronta della richiesta")
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
	// la chiave, prima di ogni controllo di bozza e di versione (EM §9.2): un reinvio trova la sua versione anche se la
	// bozza nel frattempo e' avanzata o si e' chiusa
	if gia, err := q.GetVersionePerChiaveGlobale(ctx, uuid.NullUUID{UUID: s.ChiaveSalvataggio, Valid: true}); err == nil {
		if gia.ThreadID != s.ThreadID || strings.TrimSpace(gia.ImprontaRichiesta.String) != s.ImprontaRichiesta {
			return out, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio",
				Messaggio: "questa chiave di salvataggio ha gia' salvato un altro corpo: rileggi la bozza"}
		}
		v, err := q.GetVersionePerChiaveSalvataggio(ctx, db.GetVersionePerChiaveSalvataggioParams{ChiaveSalvataggio: uuid.NullUUID{UUID: s.ChiaveSalvataggio, Valid: true}, ThreadID: s.ThreadID})
		if err != nil {
			return out, err
		}
		return Salvato3{BozzaID: v.BozzaID, Formato: FormatoSnapshot3, Versione: int(v.Versione), Riproposto: true,
			Impronta: strings.TrimSpace(v.Impronta), SalvataIl: v.SalvataIl}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	prodotto, err := prodottoDellaRfq(ctx, q, s.ThreadID, s.ProdottoID, true)
	if err != nil {
		return out, err
	}
	b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: s.BozzaID, ThreadID: s.ThreadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("bozza_id", "la bozza %s non e' di questa RFQ", s.BozzaID)
	}
	if err != nil {
		return out, err
	}
	if b.ProdottoID != s.ProdottoID {
		return out, &Errore{Codice: CodiceBozzaAltroProdotto, Campo: "bozza_id", Messaggio: fmt.Sprintf("la bozza %s e' di un altro prodotto", b.BozzaID)}
	}
	if b.Stato != StatoAperta || int(b.Versione) != s.VersioneAttesa {
		return out, conflittoVersione(b, s.VersioneAttesa)
	}
	if int(b.Formato) != FormatoSnapshot3 {
		return out, corpoMotivo("bozza_id", MotivoFormatoDaConvertire, "la bozza e' nel formato %d: per lavorarla va convertita (converti_da)", b.Formato)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: s.ThreadID, Versione: b.Versione})
	if err != nil {
		return out, err
	}
	prima, improntaP, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return out, err
	}
	nuovo, err := LeggiSnapshot3(s.Snapshot)
	if err != nil {
		return out, err
	}
	if nuovo.ProdottoID != s.ProdottoID {
		return out, corpo("snapshot.prodotto_id", "lo snapshot e' del prodotto %s, la richiesta del %s", nuovo.ProdottoID, s.ProdottoID)
	}
	n0, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: b.BozzaID, ThreadID: s.ThreadID})
	if err != nil {
		return out, err
	}
	limite, err := limiteServer(ctx, q, s.ThreadID, b.BozzaID, int(n0))
	if err != nil {
		return out, err
	}
	precedente := func(n int) (EventoLetto, bool, error) {
		e, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: b.BozzaID, ThreadID: s.ThreadID, N: int32(n)})
		if errors.Is(err, pgx.ErrNoRows) {
			return EventoLetto{}, false, nil
		}
		if err != nil {
			return EventoLetto{}, false, err
		}
		return eventoLetto(e), true, nil
	}
	ric, err := Riconcilia3(prima, nuovo, s.Azioni, Contesto3{N0: int(n0), Precedente: precedente, LimiteServer: limite})
	if err != nil {
		return out, err
	}
	if e := rebaseCoerente3(s.Rebase, ric.Rebase); e != nil {
		return out, e
	}
	canon, err := nuovo.Canonico()
	if err != nil {
		return out, err
	}
	impronta := jsonImpronta(canon)
	if impronta == improntaP && len(s.Azioni) == 0 {
		return Salvato3{BozzaID: b.BozzaID, Formato: FormatoSnapshot3, Versione: int(b.Versione), Impronta: impronta, SalvataIl: v.SalvataIl}, nil
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, s.ThreadID)
	if err != nil {
		return out, err
	}
	vf := &verifica3{ctx: ctx, q: q, thread: s.ThreadID, prodotto: prodotto, prima: prima, nuovo: nuovo, st: st, bozza: b.BozzaID}
	if err := vf.tutto(ric); err != nil {
		return out, err
	}
	if ric.Rebase != nil {
		if err := vf.rebase(ric.Rebase.Base); err != nil {
			return out, err
		}
	}
	base := baseDelloSnapshot3(nuovo)
	fonteAll, fonteSha, fonteGruppo := colonneFonte3(nuovo.Sorgente)
	versione := int(b.Versione) + 1
	salvataIl, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{
		BozzaID: b.BozzaID, Versione: int32(versione), Snapshot: canon, Impronta: impronta,
		BaseConfermaID: nullUUID(base.ConfermaID), BaseNumero: int32(base.Numero), BaseContatore: base.Contatore,
		FirmaBase: base.Firma, FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, SalvataDa: s.UtenteID,
		ImportazioneID: v.ImportazioneID, ChiaveSalvataggio: uuid.NullUUID{UUID: s.ChiaveSalvataggio, Valid: true},
		ImprontaRichiesta: testo(s.ImprontaRichiesta),
	})
	if err != nil {
		switch vincolo(err) {
		case "distinta_bozza_versione_pkey":
			return out, &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoVersioneDiversa, Messaggio: "un altro salvataggio e' arrivato prima: rileggi la bozza"}
		case "ux_dbv_chiave_salvataggio":
			return out, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio", Messaggio: "questa chiave di salvataggio e' gia' usata"}
		}
		return out, erroreDiScrittura(err, "snapshot")
	}
	for i, ev := range ric.Eventi {
		if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{
			BozzaID: b.BozzaID, N: n0 + int32(i) + 1, Versione: int32(versione), Vocabolario: VocabolarioFormato3,
			Azione: ev.Azione, ChiaveBozza: testo(ev.ChiaveBozza), Prima: rawONull(ev.Prima), Dopo: rawONull(ev.Dopo),
			Origine: ev.Origine, Motivo: testo(ev.Motivo), UtenteID: s.UtenteID,
		}); err != nil {
			return out, erroreDiScrittura(err, fmt.Sprintf("azioni[%d]", i))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if sqlstate(err) == "P4X04" {
			return out, nonRiconciliate("azioni", "il database rifiuta la versione (ciclo della bozza o legame con il registro)")
		}
		return out, err
	}
	return Salvato3{BozzaID: b.BozzaID, Formato: FormatoSnapshot3, Versione: versione, NuovaVersione: true, Eventi: len(ric.Eventi),
		Impronta: impronta, SalvataIl: salvataIl}, nil
}

func jsonImpronta(b []byte) string { return jsoncanonico.Impronta(b) }

// limiteServer: il numero dell'ultimo evento con cui comincia una versione costruita dal server (EM §9.2: la pila
// d'annullamento si ferma li').
func limiteServer(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID, n0 int) (int, error) {
	if n0 == 0 {
		return 0, nil
	}
	ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: bozza, ThreadID: thread, VersioneMax: 1 << 30,
		DopoN: 0, Limite: int32(n0)})
	if err != nil {
		return 0, err
	}
	limite := 0
	for _, e := range ee {
		if azioniDelServer2[e.Azione] && int(e.N) > limite {
			limite = int(e.N)
		}
	}
	return limite, nil
}

// rebaseCoerente3: il campo «rebase» della richiesta e l'azione rebase dicono la stessa base e lo stesso motivo, o non ci
// sono tutti e due (EM §9.2).
func rebaseCoerente3(campo *RebaseRichiesta3, azione *RebaseDichiarato3) *Errore {
	switch {
	case campo == nil && azione == nil:
		return nil
	case campo == nil:
		return corpo("rebase", "l'azione rebase vuole il campo rebase della richiesta, con la stessa base e lo stesso motivo")
	case azione == nil:
		return corpo("rebase", "il campo rebase vuole l'azione rebase fra le azioni")
	}
	if !uguali(campo.Base, azione.Base) || strings.TrimSpace(nfc(campo.Motivo)) != azione.Motivo {
		return corpo("rebase", "il campo rebase e l'azione rebase non dicono la stessa base e lo stesso motivo")
	}
	return nil
}

func baseDelloSnapshot3(s Snapshot3) Base {
	b := Base{Firma: s.FirmaBase}
	if s.VersioneBomBase != nil {
		b.ConfermaID, b.Numero, b.Contatore = s.VersioneBomBase.ConfermaID, s.VersioneBomBase.Numero, s.VersioneBomBase.Contatore
	}
	return b
}

func colonneFonte3(s *Sorgente3) (uuid.NullUUID, pgtype.Text, pgtype.Text) {
	if s == nil {
		return uuid.NullUUID{}, testo(""), testo("")
	}
	return nullUUID(s.AllegatoID), testo(s.Sha256), testo(s.Gruppo)
}

// ------------------------------------------------------------------ le verifiche contro il database e i registri

type verifica3 struct {
	ctx         context.Context
	q           *db.Queries
	thread      uuid.UUID
	prodotto    db.GetProdottoDellaRfqRow
	prima       Snapshot3
	nuovo       Snapshot3
	st          *confermabom.StatoRFQ
	bozza       uuid.UUID
	registri    map[string]*importazione.Registro
	cliente     string
	certificate map[uuid.UUID]db.ListIdentitaRegistrateRow
	versione1   *Snapshot3
}

// nodiCambiati: i nodi nuovi o cambiati rispetto alla versione precedente.
func (vf *verifica3) nodiCambiati() []Nodo3 {
	prima := map[string][]byte{}
	for _, n := range vf.prima.Nodi {
		prima[n.ChiaveBozza] = canonico(n)
	}
	var out []Nodo3
	for _, n := range vf.nuovo.Nodi {
		if p, ok := prima[n.ChiaveBozza]; !ok || !bytes.Equal(p, canonico(n)) {
			out = append(out, n)
		}
	}
	return out
}

// decisioniCambiate: le decisioni nuove o cambiate (con la loro posizione nello snapshot).
func (vf *verifica3) decisioniCambiate() map[int]Decisione3 {
	prima := map[string][]byte{}
	for _, d := range vf.prima.Decisioni {
		b, _ := d.Canonica()
		prima[d.ID] = b
	}
	out := map[int]Decisione3{}
	for i, d := range vf.nuovo.Decisioni {
		b, _ := d.Canonica()
		if p, ok := prima[d.ID]; !ok || !bytes.Equal(p, b) {
			out[i] = d
		}
	}
	return out
}

func (vf *verifica3) preassociazioniCambiate() []Preassociazione {
	prima := map[uuid.UUID]string{}
	for _, p := range vf.prima.DocumentiPreassociati {
		prima[p.PropostaID] = p.ChiaveBozza
	}
	var out []Preassociazione
	for _, p := range vf.nuovo.DocumentiPreassociati {
		if k, ok := prima[p.PropostaID]; !ok || k != p.ChiaveBozza {
			out = append(out, p)
		}
	}
	return out
}

func campoNodo(s Snapshot3, chiave string) string {
	for i, n := range s.Nodi {
		if n.ChiaveBozza == chiave {
			return fmt.Sprintf("snapshot.nodi[%d]", i)
		}
	}
	return "snapshot.nodi"
}

func (vf *verifica3) registro(imp string) (*importazione.Registro, error) {
	if r, ok := vf.registri[imp]; ok {
		return r, nil
	}
	return nil, nil
}

// caricaRegistri carica i registri citati dallo snapshot nuovo (V-2: esistono, della RFQ e del prodotto).
func (vf *verifica3) caricaRegistri() error {
	vf.registri = map[string]*importazione.Registro{}
	citati := vf.nuovo.RegistriCitati()
	if len(citati) == 0 {
		return nil
	}
	rr, err := vf.q.ListImportazioniPerImpronte(vf.ctx, db.ListImportazioniPerImpronteParams{ThreadID: vf.thread,
		ProdottoID: vf.prodotto.ComponenteID, Impronte: citati})
	if err != nil {
		return err
	}
	for _, r := range rr {
		reg, _, err := RileggiImportazione(db.GetDistintaImportazioneRow(r))
		if err != nil {
			return err
		}
		x := reg
		vf.registri[strings.TrimSpace(r.Impronta)] = &x
	}
	for _, imp := range citati {
		if _, ok := vf.registri[imp]; !ok {
			return corpoMotivo("snapshot", MotivoImportazioneSconosciuta, "il registro %s non c'e' fra quelli del prodotto in questa RFQ", imp)
		}
	}
	return nil
}

// tutto: le verifiche del commento di testa, nell'ordine.
func (vf *verifica3) tutto(ric Riconciliazione3) error {
	cliente, err := vf.q.GetClienteDellaRfq(vf.ctx, vf.thread)
	if err != nil {
		return err
	}
	vf.cliente = EmittenteCliente(cliente)
	nodi := vf.nodiCambiati()
	decisioni := vf.decisioniCambiate()
	pre := vf.preassociazioniCambiate()

	// i riferimenti che la versione precedente aveva gia': quelli degli eventi che non ci sono, sono nuovi
	prec := riferimentiDello(vf.prima)

	// appartenenza dei componenti
	componenti := map[uuid.UUID]string{}
	for u, campo := range ric.Riferimenti.Componenti {
		if !prec.componenti[u] {
			componenti[u] = campo
		}
	}
	for _, n := range nodi {
		if n.ComponenteID != nil {
			componenti[*n.ComponenteID] = campoNodo(vf.nuovo, n.ChiaveBozza) + ".componente_id"
		}
	}
	for i, d := range decisioni {
		c := fmt.Sprintf("snapshot.decisioni[%d]", i)
		for nome, u := range map[string]*uuid.UUID{"componente_id": d.ComponenteID, "padre_id": d.PadreID, "figlio_id": d.FiglioID} {
			if u != nil {
				componenti[*u] = c + "." + nome
			}
		}
		for j, p := range d.Prodotti {
			componenti[p] = fmt.Sprintf("%s.prodotti[%d]", c, j)
		}
	}
	for _, u := range ordinati(componenti) {
		if _, ok := vf.st.Componente(u); !ok {
			return fuori(componenti[u], "il componente %s non e' di questa RFQ", u)
		}
	}

	// occorrenze delle caselle e delle non_includere nuove o cambiate, e allegati citati
	type occ struct {
		campo string
		c     Casella3
	}
	var occorrenze []occ
	var ids []uuid.UUID
	// un riferimento gia' salvato (lo stesso allegato e lo stesso rif, in una casella o in una non_includere della
	// versione precedente) non e' nuovo: se e' scaduto dopo, passa e lo dicono R1 e R4 (EM §6.4, «una regola sola per R2»)
	salvate := map[[2]string]bool{}
	for _, n := range vf.prima.Nodi {
		for _, c := range n.Caselle {
			salvate[[2]string{c.AllegatoID.String(), c.Rif}] = true
		}
	}
	for _, d := range vf.prima.Decisioni {
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			salvate[[2]string{d.Casella.AllegatoID.String(), d.Casella.Rif}] = true
		}
	}
	nuova := func(c Casella3) bool { return !salvate[[2]string{c.AllegatoID.String(), c.Rif}] }
	for _, n := range nodi {
		for j, c := range n.Caselle {
			if nuova(c) {
				occorrenze = append(occorrenze, occ{fmt.Sprintf("%s.caselle[%d]", campoNodo(vf.nuovo, n.ChiaveBozza), j), c})
				ids = append(ids, c.AllegatoID)
			}
		}
	}
	for i, d := range decisioni {
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil && nuova(*d.Casella) {
			occorrenze = append(occorrenze, occ{fmt.Sprintf("snapshot.decisioni[%d].casella", i), *d.Casella})
			ids = append(ids, d.Casella.AllegatoID)
		}
	}
	letture := map[uuid.UUID]string{}
	for i, d := range decisioni {
		if d.ChiaveLettura != "" {
			if u, err := uuid.Parse(strings.SplitN(d.ChiaveLettura, "/", 2)[0]); err == nil {
				letture[u] = fmt.Sprintf("snapshot.decisioni[%d].chiave_lettura", i)
				ids = append(ids, u)
			}
		}
	}
	for _, f := range ric.FontiCitate {
		ids = append(ids, *f.AllegatoID)
	}
	// gli allegati che solo le azioni nominano (caselle, step e letture di oggetti aggiunti e tolti nello stesso
	// salvataggio): della RFQ; non serve che siano attivi, perche' lo snapshot salvato non li porta
	transitori := map[uuid.UUID]string{}
	for u, campo := range ric.Riferimenti.Allegati {
		if !prec.allegati[u] {
			transitori[u] = campo
			ids = append(ids, u)
		}
	}
	allegati, err := LeggiAllegatiCitati(vf.ctx, vf.q, vf.thread, ids)
	if err != nil {
		return err
	}
	for _, o := range occorrenze {
		a := allegati[o.c.AllegatoID]
		sha, _ := shaEChiave(o.c.Rif)
		switch {
		case !a.DellaRfq:
			return fuori(o.campo+".allegato_id", "l'allegato %s non e' un file di questa RFQ", o.c.AllegatoID)
		case !a.Attivo:
			return &Errore{Codice: CodiceFileNonAttivo, Campo: o.campo + ".allegato_id",
				Messaggio: fmt.Sprintf("l'allegato %s e' in Altri file o scartato: prima si riattiva", o.c.AllegatoID)}
		case a.Sha256 != sha:
			return corpoMotivo(o.campo+".rif", MotivoRifFuoriImportazione, "il rif non e' dello sha256 dell'allegato %s", o.c.AllegatoID)
		}
	}
	for u, campo := range letture {
		if !allegati[u].DellaRfq {
			return fuori(campo, "l'allegato della lettura %s non e' un file di questa RFQ", u)
		}
	}
	for i, f := range ric.FontiCitate {
		a := allegati[*f.AllegatoID]
		if !a.DellaRfq {
			return fuori(fmt.Sprintf("azioni.sorgente_vista[%d].allegato_id", i), "l'allegato %s non e' un file di questa RFQ", *f.AllegatoID)
		}
		if f.Sha256 != "" && a.Sha256 != f.Sha256 {
			return corpo(fmt.Sprintf("azioni.sorgente_vista[%d].sha256", i), "lo sha256 dichiarato non e' quello del file")
		}
	}
	for _, u := range ordinati(transitori) {
		if !allegati[u].DellaRfq {
			return fuori(transitori[u], "l'allegato %s non e' un file di questa RFQ", u)
		}
	}

	// le proposte: preassociazioni nuove o cambiate, svincoli nuovi o cambiati, quelle nominate dalle azioni
	proposte := map[uuid.UUID]string{}
	for i, p := range pre {
		proposte[p.PropostaID] = fmt.Sprintf("snapshot.documenti_preassociati[%d].proposta_id", i)
	}
	for i, d := range decisioni {
		if d.Tipo == DecisioneScollegaDocumento && d.PropostaID != nil {
			proposte[*d.PropostaID] = fmt.Sprintf("snapshot.decisioni[%d].proposta_id", i)
		}
	}
	// le proposte che le azioni nominano e che la versione precedente non aveva (uno svincolo o un «Lascia» di una
	// preassociazione gia' salvata non e' un riferimento nuovo)
	giaSalvate := map[uuid.UUID]bool{}
	for _, p := range vf.prima.DocumentiPreassociati {
		giaSalvate[p.PropostaID] = true
	}
	for _, d := range vf.prima.Decisioni {
		if d.Tipo == DecisioneScollegaDocumento && d.PropostaID != nil {
			giaSalvate[*d.PropostaID] = true
		}
	}
	for _, p := range ric.ProposteCitate {
		if _, ok := proposte[p]; !ok && !giaSalvate[p] {
			proposte[p] = "azioni"
		}
	}
	for p, campo := range ric.Riferimenti.Proposte {
		if _, ok := proposte[p]; !ok && !giaSalvate[p] {
			proposte[p] = campo
		}
	}
	if len(proposte) > 0 {
		ids := make([]uuid.UUID, 0, len(proposte))
		for u := range proposte {
			ids = append(ids, u)
		}
		rr, err := vf.q.ListProposteDellaRfq(vf.ctx, db.ListProposteDellaRfqParams{ThreadID: uuid.NullUUID{UUID: vf.thread, Valid: true}, Proposte: ids})
		if err != nil {
			return err
		}
		trovate := map[uuid.UUID]db.ListProposteDellaRfqRow{}
		var allegatiPre []uuid.UUID
		for _, r := range rr {
			trovate[r.PropostaID] = r
			allegatiPre = append(allegatiPre, r.AllegatoID)
		}
		for u, campo := range proposte {
			if _, ok := trovate[u]; !ok {
				return fuori(campo, "la proposta %s non e' di questa RFQ", u)
			}
		}
		att, err := LeggiAllegatiCitati(vf.ctx, vf.q, vf.thread, allegatiPre)
		if err != nil {
			return err
		}
		for i, p := range pre {
			r := trovate[p.PropostaID]
			if !att[r.AllegatoID].Attivo {
				return &Errore{Codice: CodiceFileNonAttivo, Campo: fmt.Sprintf("snapshot.documenti_preassociati[%d].proposta_id", i),
					Messaggio: fmt.Sprintf("il file della proposta %s e' in Altri file o scartato: prima si riattiva", p.PropostaID)}
			}
		}
	}

	// i registri (V-2) e le verifiche contro di loro (V-1, V-3, V-5)
	if err := vf.caricaRegistri(); err != nil {
		return err
	}
	if err := vf.registriTransitori(ric.Riferimenti.Registri, prec.registri); err != nil {
		return err
	}
	versione := ""
	if vf.nuovo.Sorgente != nil {
		versione = vf.nuovo.Sorgente.Importazione
	}
	regVersione, _ := vf.registro(versione)
	aggiunti := map[string]bool{}
	for _, k := range ric.NodiAggiunti {
		aggiunti[k] = true
	}
	for _, n := range nodi {
		campo := campoNodo(vf.nuovo, n.ChiaveBozza)
		if regVersione != nil {
			for j, c := range n.Caselle {
				if !VoceDelRegistro(&regVersione.Contenuto, c.AllegatoID, c.Rif) {
					return corpoMotivo(fmt.Sprintf("%s.caselle[%d]", campo, j), MotivoRifFuoriImportazione,
						"la casella non cita una voce intera del registro della versione (n:<allegato>:<rif>)")
				}
			}
			if e := vf.modoDelNodo(campo, n, &regVersione.Contenuto, aggiunti[n.ChiaveBozza]); e != nil {
				return e
			}
		}
		if n.Identita.Modo == ModoProposta {
			if regVersione == nil {
				return corpoMotivo(campo+".identita", MotivoPropostaNonCorrispondente, "il modo proposta vuole il registro della versione")
			}
			v, _ := regVersione.Contenuto.VocePerRif(n.Caselle[0].AllegatoID, n.Caselle[0].Rif)
			atteso, ok, err := IdentitaDellaProposta(v)
			if err != nil || !ok {
				return corpoMotivo(campo+".identita", MotivoPropostaNonCorrispondente, "la voce del registro non ha un codice proposto: il nodo e' da decidere")
			}
			id := n.Identita
			if id.Emittente != atteso.Emittente || id.Namespace != atteso.Namespace || id.Codice != atteso.Codice || id.Rev != atteso.Rev {
				return corpoMotivo(campo+".identita", MotivoPropostaNonCorrispondente,
					"l'identita' non e' quella proposta dal registro (emittente, namespace, codice proposto, rev proposta)")
			}
		}
	}
	// gli archi step nuovi o cambiati: rimandi al registro (V-1, «relazioni»; EM §3.5)
	if err := vf.archiStep(regVersione); err != nil {
		return err
	}
	casellaDi := map[string]*Casella3{}
	for _, n := range vf.nuovo.Nodi {
		if len(n.Caselle) > 0 {
			c := n.Caselle[0]
			casellaDi[n.ChiaveBozza] = &c
		}
	}
	for i, d := range decisioni {
		campo := fmt.Sprintf("snapshot.decisioni[%d]", i)
		if d.Importazione != "" && d.Importazione != versione {
			return corpoMotivo(campo+".importazione", MotivoImportazioneSconosciuta,
				"una decisione aggiunta o cambiata nel salvataggio cita il registro della versione")
		}
		reg, _ := vf.registro(d.Importazione)
		switch d.Tipo {
		case DecisioneIdentita:
			if d.Modo == ModoAlternativa && reg != nil {
				if e := VerificaLetturaDecisione(campo, &reg.Contenuto, d, casellaDi[d.ChiaveBozza]); e != nil {
					return e
				}
			}
		case DecisioneNonIncludere:
			// il registro della decisione; per una decisione nuova o cambiata senza importazione, quello della versione
			// (EM §7.1: «per una decisione nuova, quello della versione»; §3.4.1: con un registro citato ogni rif sta nel
			// registro; V-5): cosi' una non_includere non porta un rif che il registro della bozza non ha
			if reg == nil && d.Importazione == "" {
				reg = regVersione
			}
			if reg != nil {
				if !VoceDelRegistro(&reg.Contenuto, d.Casella.AllegatoID, d.Casella.Rif) {
					return corpoMotivo(campo+".casella", MotivoRifFuoriImportazione, "la casella non cita una voce intera del registro della decisione (o della versione)")
				}
				if e := VerificaSegnaleVisto(campo, &reg.Contenuto, d); e != nil {
					return e
				}
			} else if d.SegnaleVisto != nil {
				return corpoMotivo(campo+".segnale_visto", MotivoSegnaleNonCorrisponde, "un segnale visto viene da un registro, e la decisione non ne cita uno")
			}
		}
	}

	// la regola unica dell'emittente, l'identita' registrata, la presunzione, i nodi formato_2
	return vf.identita(nodi, decisioni, regVersione)
}

// ordinati: le chiavi di una mappa di UUID in ordine (un errore dice sempre lo stesso campo per lo stesso corpo).
func ordinati(m map[uuid.UUID]string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m))
	for u := range m {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// riferimentiPrecedenti: gli UUID e i registri che uno snapshot salvato porta (componenti dei nodi e delle decisioni,
// allegati della sorgente, delle caselle, degli step, delle non_includere e delle letture, proposte delle preassociazioni
// e degli svincoli, registri della sorgente e delle decisioni).
type riferimentiPrecedenti struct {
	componenti, allegati, proposte map[uuid.UUID]bool
	registri                       map[string]bool
}

func riferimentiDello(s Snapshot3) riferimentiPrecedenti {
	r := riferimentiPrecedenti{componenti: map[uuid.UUID]bool{}, allegati: map[uuid.UUID]bool{}, proposte: map[uuid.UUID]bool{},
		registri: map[string]bool{}}
	if s.Sorgente != nil && s.Sorgente.AllegatoID != nil {
		r.allegati[*s.Sorgente.AllegatoID] = true
	}
	for _, imp := range s.RegistriCitati() {
		r.registri[imp] = true
	}
	for _, n := range s.Nodi {
		if n.ComponenteID != nil {
			r.componenti[*n.ComponenteID] = true
		}
		for _, c := range n.Caselle {
			r.allegati[c.AllegatoID] = true
		}
	}
	for _, a := range s.Archi {
		if a.Step != nil {
			r.allegati[a.Step.AllegatoID] = true
		}
	}
	for _, d := range s.Decisioni {
		for _, u := range []*uuid.UUID{d.ComponenteID, d.PadreID, d.FiglioID} {
			if u != nil {
				r.componenti[*u] = true
			}
		}
		if d.PropostaID != nil {
			r.proposte[*d.PropostaID] = true
		}
		for _, p := range d.Prodotti {
			r.componenti[p] = true
		}
		if d.Casella != nil {
			r.allegati[d.Casella.AllegatoID] = true
		}
		if d.ChiaveLettura != "" {
			if u, err := uuid.Parse(strings.SplitN(d.ChiaveLettura, "/", 2)[0]); err == nil {
				r.allegati[u] = true
			}
		}
	}
	for _, p := range s.DocumentiPreassociati {
		r.proposte[p.PropostaID] = true
	}
	return r
}

// registriTransitori: i registri che solo le azioni citano (una decisione aggiunta e tolta nello stesso salvataggio)
// esistono e sono della RFQ e del prodotto, come quelli dello snapshot (V-2).
func (vf *verifica3) registriTransitori(citati map[string]string, prec map[string]bool) error {
	var cercare []string
	campi := map[string]string{}
	for imp, campo := range citati {
		if _, gia := vf.registri[imp]; gia || prec[imp] {
			continue
		}
		cercare = append(cercare, imp)
		campi[imp] = campo
	}
	if len(cercare) == 0 {
		return nil
	}
	sort.Strings(cercare)
	rr, err := vf.q.ListImportazioniPerImpronte(vf.ctx, db.ListImportazioniPerImpronteParams{ThreadID: vf.thread,
		ProdottoID: vf.prodotto.ComponenteID, Impronte: cercare})
	if err != nil {
		return err
	}
	trovati := map[string]bool{}
	for _, r := range rr {
		trovati[strings.TrimSpace(r.Impronta)] = true
	}
	for _, imp := range cercare {
		if !trovati[imp] {
			return corpoMotivo(campi[imp], MotivoImportazioneSconosciuta, "il registro %s non c'e' fra quelli del prodotto in questa RFQ", imp)
		}
	}
	return nil
}

// modoAtteso: il modo che le regole dello stato di partenza danno a un nodo con la casella di quella voce (EM §6.1):
// P-2 la voce della radice e P-3 una voce con decisione_esistente danno registrato (salvo il raggruppamento di P-3, la
// voce diversa dalla radice la cui decisione_esistente e' il prodotto del registro, che segue P-4); P-4 da'
// proposta con codice_proposto, altrimenti da_decidere.
func modoAtteso(c *importazione.Contenuto, v importazione.Voce) string {
	if c.EVoceDellaRadice(v) {
		return ModoRegistrato
	}
	if len(v.DecisioneEsistente) > 0 && string(v.DecisioneEsistente) != "null" {
		var de struct {
			ComponenteID string `json:"componente_id"`
		}
		_ = json.Unmarshal(v.DecisioneEsistente, &de)
		if de.ComponenteID == "" || de.ComponenteID != c.ProdottoID {
			return ModoRegistrato
		}
	}
	if v.CodiceProposto != "" {
		return ModoProposta
	}
	return ModoDaDecidere
}

// modoDelNodo: il modo di un nodo nuovo o cambiato con la casella, contro la voce del registro della versione (EM §3.4.2,
// §6.1; PM23 R-03). Un nodo da_decidere ha una voce senza codice proposto (altrimenti si toglierebbe il codice proposto
// senza la decisione identita motivata, §3.6.1); un nodo messo nella bozza da un nodo_aggiunto di questo salvataggio
// («Importa dalla proposta…», «Includi di nuovo»), che non sia in modo decisione, ha il modo che darebbero P-2...P-6.
// Un altro modo e' 422 proposta_non_corrispondente. I nodi gia' salvati con il modo di allora, e quelli che un annulla o
// un rifai riporta, non si rivedono qui.
func (vf *verifica3) modoDelNodo(campo string, n Nodo3, c *importazione.Contenuto, aggiunto bool) *Errore {
	if len(n.Caselle) == 0 || n.Identita.Modo == ModoDecisione {
		return nil
	}
	v, ok := c.VocePerRif(n.Caselle[0].AllegatoID, n.Caselle[0].Rif)
	if !ok {
		return nil // una voce fuori per decisione: il nodo non e' dello stato di partenza, e V-3 l'ha gia' guardata
	}
	if n.Identita.Modo == ModoDaDecidere && v.CodiceProposto != "" {
		return corpoMotivo(campo+".identita.modo", MotivoPropostaNonCorrispondente,
			"la voce del nodo ha il codice proposto %s: un nodo da decidere e' senza codice proposto; per toglierlo serve una decisione identita motivata", v.CodiceProposto)
	}
	if aggiunto {
		if atteso := modoAtteso(c, v); n.Identita.Modo != atteso {
			return corpoMotivo(campo+".identita.modo", MotivoPropostaNonCorrispondente,
				"un nodo importato dalla proposta ha il modo che danno le regole dello stato di partenza (%s), non %s", atteso, n.Identita.Modo)
		}
	}
	return nil
}

// archiStep: gli archi con origine step nuovi o cambiati (l'arco o le caselle dei suoi estremi) sono archi della
// struttura del motore (EM §3.5: «l'arco STEP accettato e' (allegato_id, rif della casella del padre, rif della casella
// del figlio). Il server verifica che stia nel registro, o nei fatti correnti per una bozza senza importazione»; V-1,
// «relazioni»). Con il registro della versione: un arco di proposta.archi[] con quell'allegato e quei due rif; senza
// registro: una relazione della struttura dei fatti correnti dello STEP. Altrimenti 422 CORPO_NON_VALIDO,
// rif_fuori_importazione (scelta [T] della corsia persistenza: l'arco e' un rimando per rif, come una casella).
func (vf *verifica3) archiStep(reg *importazione.Registro) error {
	caselle := func(s Snapshot3) map[string][]Casella3 {
		out := map[string][]Casella3{}
		for _, n := range s.Nodi {
			out[n.ChiaveBozza] = n.Caselle
		}
		return out
	}
	cPrima, cNuovo := caselle(vf.prima), caselle(vf.nuovo)
	archiPrima := map[[2]string][]byte{}
	for _, a := range vf.prima.Archi {
		archiPrima[[2]string{a.Padre, a.Figlio}] = canonico(a)
	}
	var nelRegistro map[[3]string]bool
	if reg != nil {
		nelRegistro = map[[3]string]bool{}
		for _, x := range reg.Contenuto.Proposta.Archi {
			nelRegistro[[3]string{x.AllegatoID, x.Padre, x.Figlio}] = true
		}
	}
	fatti := map[string]map[[2]string]bool{}
	for i, a := range vf.nuovo.Archi {
		if a.Origine != OrigineStep || a.Step == nil {
			continue
		}
		k := [2]string{a.Padre, a.Figlio}
		if p, ok := archiPrima[k]; ok && bytes.Equal(p, canonico(a)) && bytes.Equal(canonico(cPrima[a.Padre]), canonico(cNuovo[a.Padre])) &&
			bytes.Equal(canonico(cPrima[a.Figlio]), canonico(cNuovo[a.Figlio])) {
			continue // l'arco e i suoi estremi sono quelli della versione precedente
		}
		campo := fmt.Sprintf("snapshot.archi[%d].step", i)
		trovato := false
		for _, cp := range cNuovo[a.Padre] {
			for _, cf := range cNuovo[a.Figlio] {
				if reg != nil {
					if nelRegistro[[3]string{a.Step.AllegatoID.String(), cp.Rif, cf.Rif}] {
						trovato = true
					}
					continue
				}
				shaP, chiaveP := shaEChiave(cp.Rif)
				shaF, chiaveF := shaEChiave(cf.Rif)
				if shaP == "" || shaP != shaF {
					continue
				}
				rr, ok := fatti[shaP]
				if !ok {
					var err error
					if rr, err = relazioniDeiFatti(vf.ctx, vf.q, shaP); err != nil {
						return err
					}
					fatti[shaP] = rr
				}
				if rr[[2]string{chiaveP, chiaveF}] {
					trovato = true
				}
			}
		}
		if !trovato {
			if reg != nil {
				return corpoMotivo(campo, MotivoRifFuoriImportazione,
					"l'arco step %s → %s non e' un arco del registro della versione (allegato, rif della casella del padre, rif della casella del figlio)", a.Padre, a.Figlio)
			}
			return corpoMotivo(campo, MotivoRifFuoriImportazione,
				"l'arco step %s → %s non e' una relazione dei fatti correnti dello STEP (la bozza non ha un registro)", a.Padre, a.Figlio)
		}
	}
	return nil
}

// relazioniDeiFatti: le relazioni (chiave del padre, chiave del figlio) della struttura dei fatti correnti di un
// contenuto; vuote senza fatti correnti o senza una struttura leggibile.
func relazioniDeiFatti(ctx context.Context, q *db.Queries, sha string) (map[[2]string]bool, error) {
	out := map[[2]string]bool{}
	af, err := q.GetAnalisiCorrente(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	st, ok := worker.DecodificaStruttura(af.Fatti)
	if !ok {
		return out, nil
	}
	for _, r := range st.Relazioni {
		out[[2]string{r.Padre, r.Figlio}] = true
	}
	return out, nil
}

// emittenteAmmesso: la regola unica dell'emittente (EM §5.6): il cliente della RFQ, un partner attivo nel registro citato
// (o in quello della versione), quello della certificazione valida del componente.
func (vf *verifica3) emittenteAmmesso(e string, regs []*importazione.Registro, comp *uuid.UUID) bool {
	if e == vf.cliente {
		return true
	}
	for _, r := range regs {
		if r == nil {
			continue
		}
		for _, p := range r.Contenuto.PartnerAttivi() {
			if p == e {
				return true
			}
		}
	}
	if comp != nil {
		if c, ok := vf.certificate[*comp]; ok && c.Valida && c.Emittente.String == e {
			return true
		}
	}
	return false
}

func (vf *verifica3) caricaCertificazioni(nodi []Nodo3, decisioni map[int]Decisione3) error {
	vf.certificate = map[uuid.UUID]db.ListIdentitaRegistrateRow{}
	var ids []uuid.UUID
	for _, n := range nodi {
		if n.ComponenteID != nil {
			ids = append(ids, *n.ComponenteID)
		}
	}
	for _, d := range decisioni {
		if d.ComponenteID != nil {
			ids = append(ids, *d.ComponenteID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rr, err := vf.q.ListIdentitaRegistrate(vf.ctx, db.ListIdentitaRegistrateParams{ThreadID: vf.thread, Componenti: ids})
	if err != nil {
		return err
	}
	for _, r := range rr {
		vf.certificate[r.ComponenteID] = r
	}
	return nil
}

// identitaRegistrata: l'identita' registrata di un componente per un nodo in modo registrato (EM §3.4.2, §5.1): quella
// della certificazione formato 3 se vale ancora, altrimenti il cliente presunto senza namespace; il codice e' sempre
// quello di oggi.
func (vf *verifica3) identitaRegistrata(comp uuid.UUID) (Identita, bool) {
	c, ok := vf.st.Componente(comp)
	if !ok {
		return Identita{}, false
	}
	id := Identita{Emittente: vf.cliente, EmittentePresunto: true, Codice: c.CodiceLetterale, Modo: ModoRegistrato}
	if x, ok := vf.certificate[comp]; ok && x.Valida && x.Emittente.Valid {
		id.Emittente = x.Emittente.String
		id.EmittentePresunto = x.EmittentePresunto.Bool
		id.Namespace = x.Namespace.String
	}
	return id, true
}

func (vf *verifica3) identita(nodi []Nodo3, decisioni map[int]Decisione3, regVersione *importazione.Registro) error {
	if err := vf.caricaCertificazioni(nodi, decisioni); err != nil {
		return err
	}
	perID := map[string]Decisione3{}
	for _, d := range vf.nuovo.Decisioni {
		perID[d.ID] = d
	}
	for _, n := range nodi {
		campo := campoNodo(vf.nuovo, n.ChiaveBozza) + ".identita"
		id := n.Identita
		regs := []*importazione.Registro{regVersione}
		if id.Modo == ModoDecisione {
			if d, ok := perID[id.Decisione]; ok {
				if r, _ := vf.registro(d.Importazione); r != nil {
					regs = append(regs, r)
				}
			}
		}
		if !vf.emittenteAmmesso(id.Emittente, regs, n.ComponenteID) {
			return corpoMotivo(campo+".emittente", MotivoEmittenteNonAmmesso,
				"l'emittente %s non e' il cliente della RFQ, un partner attivo nel registro citato o quello certificato del componente", id.Emittente)
		}
		switch id.Modo {
		case ModoRegistrato:
			atteso, ok := vf.identitaRegistrata(*n.ComponenteID)
			if !ok || id.Codice != atteso.Codice || id.Emittente != atteso.Emittente || id.EmittentePresunto != atteso.EmittentePresunto ||
				id.Namespace != atteso.Namespace {
				return corpoMotivo(campo, MotivoRegistratoNonCorrispondente,
					"l'identita' registrata del componente e' %s %s (presunto %v, namespace %q): il server la verifica sulla certificazione valida",
					atteso.Codice, atteso.Emittente, atteso.EmittentePresunto, atteso.Namespace)
			}
		case ModoDaDecidere:
			// un nodo con la casella e senza codice: nessuno ha scelto un emittente, e vale la regola del cliente, presunto
			// (scelta [T] della corsia persistenza, ondata B: EM §3.4.2 non lo fissa)
			if id.Emittente != vf.cliente || !id.EmittentePresunto || id.Namespace != "" {
				return corpoMotivo(campo+".emittente_presunto", MotivoEmittentePresuntoDiverso,
					"un nodo da decidere ha l'emittente del cliente, presunto, senza namespace")
			}
		case ModoManuale:
			if id.Emittente != vf.cliente {
				return corpoMotivo(campo+".emittente", MotivoEmittenteNonAmmesso, "un nodo scritto a mano ha l'emittente del cliente della RFQ (EM §5.6)")
			}
		case ModoFormato2:
			if id.Emittente != vf.cliente {
				return corpoMotivo(campo+".emittente", MotivoEmittenteNonAmmesso, "un nodo venuto dal formato 2 ha l'emittente del cliente della RFQ")
			}
			if err := vf.formato2(campo, n); err != nil {
				return err
			}
		}
	}
	for i, d := range decisioni {
		campo := fmt.Sprintf("snapshot.decisioni[%d]", i)
		var regs []*importazione.Registro
		if r, _ := vf.registro(d.Importazione); r != nil {
			regs = append(regs, r)
		}
		switch d.Tipo {
		case DecisioneIdentita:
			if !vf.emittenteAmmesso(d.Identita.Emittente, append(regs, regVersione), d.ComponenteID) && !vf.emittenteDelNodo(d) {
				return corpoMotivo(campo+".identita.emittente", MotivoEmittenteNonAmmesso,
					"l'emittente %s non e' il cliente della RFQ, un partner attivo nel registro citato o quello certificato del componente", d.Identita.Emittente)
			}
		case DecisioneRicodifica:
			if !vf.emittenteAmmesso(d.EmittenteDopo, []*importazione.Registro{regVersione}, d.ComponenteID) {
				return corpoMotivo(campo+".emittente_dopo", MotivoEmittenteNonAmmesso,
					"l'emittente %s non e' il cliente della RFQ, un partner attivo nel registro della versione o quello certificato del componente", d.EmittenteDopo)
			}
		}
	}
	return nil
}

// emittenteDelNodo: per una decisione identita su un nodo del componente (riusa o modifica), l'emittente certificato del
// suo componente vale (EM §5.6, terzo caso).
func (vf *verifica3) emittenteDelNodo(d Decisione3) bool {
	for _, n := range vf.nuovo.Nodi {
		if n.ChiaveBozza == d.ChiaveBozza && n.ComponenteID != nil {
			return vf.emittenteAmmesso(d.Identita.Emittente, nil, n.ComponenteID)
		}
	}
	return false
}

// formato2: un nodo formato_2 ha codice e rev della versione 1 della bozza formato 3 che la conversione ha costruito
// (EM §3.4.2), che non si cancella.
func (vf *verifica3) formato2(campo string, n Nodo3) error {
	if vf.versione1 == nil {
		v, err := vf.q.GetDistintaBozzaVersione(vf.ctx, db.GetDistintaBozzaVersioneParams{BozzaID: vf.bozza, ThreadID: vf.thread, Versione: 1})
		if err != nil {
			return err
		}
		s, _, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
		if err != nil {
			return err
		}
		vf.versione1 = &s
	}
	for _, x := range vf.versione1.Nodi {
		if x.ChiaveBozza == n.ChiaveBozza {
			if x.Identita.Modo != ModoFormato2 || x.Identita.Codice != n.Identita.Codice || x.Identita.Rev != n.Identita.Rev {
				return corpoMotivo(campo, MotivoFormato2NonCorrispondente,
					"un nodo formato_2 ha codice e rev della versione 1 della bozza convertita (%q %q)", x.Identita.Codice, x.Identita.Rev)
			}
			return nil
		}
	}
	return corpoMotivo(campo, MotivoFormato2NonCorrispondente, "il nodo %s non e' fra quelli della conversione: non puo' essere formato_2", n.ChiaveBozza)
}

// rebase: la base nuova e' quella corrente del prodotto (409 BASE_SUPERATA), e dopo il rebase non restano rif scaduti
// (422; EM §9.2, MAT BZ-23).
func (vf *verifica3) rebase(b RebaseV2) error {
	vc, err := vf.st.VersioneCorrente(vf.prodotto.ComponenteID)
	if err != nil {
		return err
	}
	var motivi []string
	if (vc.ConfermaID == nil) != (b.ConfermaID == nil) || (vc.ConfermaID != nil && *vc.ConfermaID != *b.ConfermaID) || int(vc.Numero) != b.Numero {
		motivi = append(motivi, MotivoConfermaCambiata)
	}
	if vc.Contatore != b.Contatore || vc.FirmaStruttura != b.FirmaStruttura {
		motivi = append(motivi, MotivoStrutturaCambiata)
	}
	if len(motivi) > 0 {
		return &Errore{Codice: CodiceBaseSuperata, Campo: "rebase.base", Motivi: motivi, VersioneCorrente: &vc,
			Messaggio: "la base del rebase non e' quella corrente del prodotto: rileggi la versione corrente e rifai il rebase"}
	}
	scaduti, err := RifScaduti(vf.ctx, vf.q, vf.thread, vf.nuovo)
	if err != nil {
		return err
	}
	if len(scaduti) > 0 {
		return corpoMotivo("rebase", MotivoRifScaduti, "dopo il rebase restano %d rif scaduti (%s %s): prima si tolgono o si rifanno", len(scaduti),
			scaduti[0].Casella.Rif, scaduti[0].Motivo)
	}
	return nil
}
