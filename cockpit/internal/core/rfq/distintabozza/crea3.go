package distintabozza

// R2, forma crea, nel formato 3 (EM §9.2, §6.3, §6.8, §6.12; U2 §2.8; DEC-B PD-02; direttiva 35, blocco C-3, R2 passo 6b).
// La versione 1 la costruisce sempre il server (partenza3.go, partenzastep3.go): il browser manda solo la partenza.
//
// L'ordine, uguale per le cinque forme (EM §9.2): BloccaThread → la chiave del salvataggio (globale: stessa chiave e stessa
// impronta della richiesta = riproposto, con la versione e lo snapshot gia' scritti; impronta diversa = 409
// CHIAVE_RIUSATA) → il prodotto finito attivo della RFQ → con abbandona la bozza da abbandonare (della RFQ, del prodotto,
// aperta, alla versione dichiarata: altrimenti BOZZA_VERSIONE), senza nessun'altra bozza aperta del prodotto
// (altra_bozza_aperta) → i controlli della partenza → la scrittura: con abbandona la chiusura della bozza di prima
// («ripartita … nella bozza <id>», il motivo intero nell'evento bozza_aperta), la testata nuova, la versione 1 e gli
// eventi del server; per la partenza step anche il registro e fonte_step_evento.
//
// Le partenze:
//   - vuota: il solo prodotto (PartenzaRegistrata), senza sorgente, registro ne' fonte_step_evento (direttiva 39 §1);
//     422 partenza_non_ammessa con una conferma formato 3 valida o con uno STEP confermato da S8 e valido (vuotaAmmessa);
//   - registrato: il perimetro della working; su un prodotto con una conferma formato 3 valida 422 CORPO_NON_VALIDO,
//     motivo partenza_non_ammessa, campo crea.partenza (DEC-B PD-02 (b));
//   - conferma: da-conferma-1 sull'ultima conferma valida del prodotto (PartenzaDaConferma); senza una conferma valida
//     422 partenza_non_ammessa;
//   - step: importazione-1 (VersioneDaRegistro, partenzastep3.go) sul registro che chi chiama ha calcolato e confrontato
//     con l'impronta dichiarata (V-7) prima della transazione (PropostaCalcolata); sotto il lucchetto: lo sha256
//     dichiarato, di nuovo V-7, la fonte confermata (422 FONTE_NON_CONFERMATA, U2 §2.8; la ripartenza dalla stessa
//     sorgente passa [T]), il motivo della scelta della fonte (MOTIVO_OBBLIGATORIO), le occorrenze del registro (409
//     ANTEPRIMA_SUPERATA), il registro trovato o scritto; gli eventi bozza_aperta e sorgente_scelta (motivo crea.motivo),
//     fonte_step_evento scelta.
//
// Mai componente, componente_relazione, file_rfq_associazione, correzione_documento, NAS o F8 (EM §9.2, «Effetti
// persistenti»). Direttiva 39 §4 (presainc3.go), che supera per le proposte univoche «nessuna proposta del motore diventa
// PB» (P-7; [U] dir. 35 §8 C-3; D-PB01 A [U]): alla nascita le pre-assegnazioni legacy aperte dei nodi diventano PB
// legacy, con l'evento conversione_legacy e la copia tolta dal servizio unico (D-AR14 A [T-39]: l'unico punto in cui crea
// scrive associazione_evento e documento_proposta); con la partenza step e da-conferma-1 le proposte univoche del registro
// diventano PB motore, solo con la sorgente confermata da S8 (D-AR11 B [U]).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// Creazione3: una richiesta della forma crea, gia' letta (RichiestaR2), con la RFQ del percorso e l'utente della sessione.
// Proposta: per la partenza step, il registro calcolato da chi chiama prima della transazione (nil per le altre partenze e
// per un reinvio riconosciuto dalla chiave prima del calcolo).
type Creazione3 struct {
	ThreadID, UtenteID uuid.UUID
	Comune             ComuneR2
	Crea               CreaR2
	Proposta           *PropostaCalcolata
	// PrimaDelCommit: chiamata nella stessa transazione dopo tutte le scritture e prima del COMMIT (S8 vi scrive l'esito
	// del suo intento, U2 §2.2: «nella transazione dell'ultimo passo»); un errore annulla tutto. Mai sul reinvio.
	PrimaDelCommit func(ctx context.Context, q *db.Queries, bozza uuid.UUID, versione int, imp *ImportazioneR2) error
	// Chiusura: con Abbandona, la causa della chiusura della bozza di prima, scritta dal server davanti a «ripartita …»
	// nel motivo_chiusura (S8 che sostituisce una bozza nata da un'altra sorgente S8, G39-03 [U]); mai dal browser.
	Chiusura string
}

// Creata: la risposta della forma crea (EM §9.2): la versione 1 con il suo snapshot, la partenza, la conferma di
// da-conferma-1, il registro della partenza step, la bozza abbandonata dalla ripartenza e le tracce del costruttore.
type Creata struct {
	Salvato3
	Partenza         string          `json:"partenza"`
	Snapshot         json.RawMessage `json:"snapshot"`
	Importazione     *ImportazioneR2 `json:"importazione,omitempty"`
	DaConferma       *DaConfermaR2   `json:"da_conferma,omitempty"`
	BozzaAbbandonata *uuid.UUID      `json:"bozza_abbandonata,omitempty"`
	Traccia          TracciaPartenza `json:"traccia"`
	// direttiva 39 §4 (presainc3.go): i suggerimenti (preassociazioni_proposte[], mai in da_rivedere[]), le
	// pre-assegnazioni legacy convertite e quelle rimaste; le PB iniziali stanno nello snapshot
	PreassociazioniProposte      []PreassociazioneProposta3      `json:"preassociazioni_proposte,omitempty"`
	ConversioniLegacy            []ConversioneLegacy3            `json:"conversioni_legacy,omitempty"`
	PreassegnazioniNonConvertite []PreassegnazioneNonConvertita3 `json:"preassegnazioni_non_convertite,omitempty"`
}

// CreaDaRichiesta: la Creazione3 di un corpo di R2 nella forma crea.
func (r RichiestaR2) CreaDaRichiesta(thread, utente uuid.UUID, p *PropostaCalcolata) Creazione3 {
	c := Creazione3{ThreadID: thread, UtenteID: utente, Comune: r.Comune, Proposta: p}
	if r.Crea != nil {
		c.Crea = *r.Crea
	}
	return c
}

// CreaFormato3 crea la bozza formato 3 di un prodotto, versione 1 costruita dal server (commento di testa).
func CreaFormato3(ctx context.Context, pool Avviatore, c Creazione3) (Creata, error) {
	var out Creata
	if !chiaveDelServer(c.Comune) {
		return out, corpo("chiave_salvataggio", "la chiave del salvataggio e' un UUID v4, con l'impronta della richiesta")
	}
	if !partenze3[c.Crea.Partenza] {
		return out, corpo("crea.partenza", "la partenza e' una fra vuota, registrato, conferma e step")
	}
	if (c.Crea.Partenza == PartenzaStep || c.Crea.Partenza == PartenzaSoloProdotto) && c.Crea.Sorgente == nil {
		return out, corpo("crea.sorgente", "la partenza %s vuole la sorgente", c.Crea.Partenza)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, c.ThreadID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	if gia, trovata, err := chiaveGiaUsata(ctx, q, c.ThreadID, c.Comune); err != nil || trovata {
		if err != nil {
			return out, err
		}
		return riproponiCreata(ctx, q, c.ThreadID, gia)
	}
	if c.Crea.Partenza == PartenzaStep && c.Proposta == nil {
		return out, corpo("crea", "la partenza step vuole la proposta calcolata dal server")
	}
	if _, err := prodottoDellaRfq(ctx, q, c.ThreadID, c.Comune.ProdottoID, true); err != nil {
		return out, err
	}
	var vecchia *db.DistintaBozza
	if a := c.Crea.Abbandona; a != nil {
		b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: a.BozzaID, ThreadID: c.ThreadID})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, fuori("crea.abbandona.bozza_id", "la bozza %s non e' di questa RFQ", a.BozzaID)
		}
		if err != nil {
			return out, err
		}
		if b.ProdottoID != c.Comune.ProdottoID {
			return out, &Errore{Codice: CodiceBozzaAltroProdotto, Campo: "crea.abbandona.bozza_id", Messaggio: "la bozza da abbandonare e' di un altro prodotto"}
		}
		if b.Stato != StatoAperta || int(b.Versione) != a.VersioneAttesa {
			return out, conflittoVersione(b, a.VersioneAttesa)
		}
		vecchia = &b
	} else {
		aperta, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: c.ThreadID, ProdottoID: c.Comune.ProdottoID})
		switch {
		case err == nil:
			return out, giaAperta(aperta)
		case !errors.Is(err, pgx.ErrNoRows):
			return out, err
		}
	}
	in, err := ingressoPartenza(ctx, q, c.ThreadID, c.Comune.ProdottoID)
	if err != nil {
		return out, err
	}
	var (
		snap  Snapshot3
		tr    TracciaPartenza
		da    *DaConfermaR2
		cp    ConfermaDiPartenza
		passo *passoStep
	)
	switch c.Crea.Partenza {
	case PartenzaVuota:
		if err := vuotaAmmessa(ctx, q, c.ThreadID, c.Comune.ProdottoID, in); err != nil {
			return out, err
		}
		snap, tr, err = PartenzaRegistrata(PartenzaVuota, in)
	case PartenzaRegistrato:
		if u, v, ok, err2 := in.Stato.ValiditaUltima(c.Comune.ProdottoID); err2 != nil {
			return out, err2
		} else if ok && v.Valida && u.Formato == FormatoSnapshot3 {
			return out, corpoMotivo("crea.partenza", MotivoPartenzaNonAmmessa,
				"il prodotto ha una conferma formato 3 valida (n. %d): la strada e' «Modifica la BOM confermata» (partenza conferma)", u.Numero)
		}
		snap, tr, err = PartenzaRegistrata(PartenzaRegistrato, in)
	case PartenzaConferma:
		cp, err = confermaDiPartenza(ctx, q, c.ThreadID, in)
		if err != nil {
			return out, err
		}
		da = &DaConfermaR2{ConfermaID: cp.ConfermaID, Numero: cp.Numero}
		snap, tr, err = PartenzaDaConferma(cp, in)
	case PartenzaStep:
		passo, err = preparaPartenzaStep(ctx, q, c, in, vecchia)
		if err != nil {
			return out, err
		}
		snap, tr = passo.snapshot, passo.traccia
	case PartenzaSoloProdotto:
		snap, tr, err = preparaSoloProdotto(ctx, q, c, in)
	}
	if err != nil {
		return out, err
	}
	// la presa in carico (direttiva 39 §4): le pre-assegnazioni legacy aperte convertite in PB legacy, poi le PB iniziali
	// del motore dal registro della versione (step, da-conferma-1) solo con la sorgente confermata da S8
	presa, err := presaInCarico(ctx, q, c, in, &snap, passo, cp)
	if err != nil {
		return out, err
	}
	canon, err := snap.Canonico()
	if err != nil {
		return out, erroreDiCostruzione(err, c.Crea.Partenza)
	}
	impronta := jsonImpronta(canon)
	nuovoID := uuid.New()
	if vecchia != nil {
		chiusura := fmt.Sprintf("ripartita (partenza %s) nella bozza %s", c.Crea.Partenza, nuovoID)
		if c.Crea.Partenza == PartenzaStep {
			chiusura = "ripartita dalla proposta nella bozza " + nuovoID.String()
		}
		if x := strings.TrimSpace(c.Chiusura); x != "" {
			chiusura = troncaMotivo(x, MaxMotivo-len([]rune(chiusura))-3) + " — " + chiusura
		}
		n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: c.UtenteID, Valid: true},
			Motivo: testo(chiusura), BozzaID: vecchia.BozzaID, ThreadID: c.ThreadID, Versione: vecchia.Versione})
		if err != nil {
			return out, err
		}
		if n != 1 {
			return out, conflittoVersione(*vecchia, c.Crea.Abbandona.VersioneAttesa)
		}
	}
	var importazioneID uuid.NullUUID
	if passo != nil {
		imp, err := registroDellaVersione(ctx, q, c.ThreadID, c.Comune.ProdottoID, c.UtenteID, passo.registro, "crea")
		if err != nil {
			return out, err
		}
		imp.FuoriPerDecisione = FuoriDelRegistro(passo.registro.Contenuto)
		passo.importazione = &imp
		importazioneID = uuid.NullUUID{UUID: imp.ImportazioneID, Valid: true}
	}
	base := baseDelloSnapshot3(snap)
	fonteAll, fonteSha, fonteGruppo := colonneFonte3(snap.Sorgente)
	if _, err := q.InsertDistintaBozzaConID(ctx, db.InsertDistintaBozzaConIDParams{BozzaID: nuovoID, ThreadID: c.ThreadID, ProdottoID: c.Comune.ProdottoID,
		Formato: FormatoSnapshot3, BaseConfermaID: nullUUID(base.ConfermaID), BaseNumero: int32(base.Numero), BaseContatore: base.Contatore,
		FirmaBase: base.Firma, FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, UtenteID: c.UtenteID}); err != nil {
		if vincolo(err) == "ux_distinta_bozza_aperta" {
			return out, &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoAltraBozzaAperta, Messaggio: "il prodotto ha gia' una bozza aperta"}
		}
		return out, erroreDiScrittura(err, "crea")
	}
	salvataIl, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{
		BozzaID: nuovoID, Versione: 1, Snapshot: canon, Impronta: impronta, BaseConfermaID: nullUUID(base.ConfermaID),
		BaseNumero: int32(base.Numero), BaseContatore: base.Contatore, FirmaBase: base.Firma, FonteAllegatoID: fonteAll,
		FonteSha256: fonteSha, FonteGruppo: fonteGruppo, SalvataDa: c.UtenteID, ImportazioneID: importazioneID,
		ChiaveSalvataggio: uuid.NullUUID{UUID: c.Comune.ChiaveSalvataggio, Valid: true}, ImprontaRichiesta: testo(c.Comune.ImprontaRichiesta),
	})
	if err != nil {
		if vincolo(err) == "ux_dbv_chiave_salvataggio" {
			return out, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio", Messaggio: "questa chiave di salvataggio e' gia' usata"}
		}
		return out, erroreDiScrittura(err, "crea")
	}
	algoritmo := map[string]string{PartenzaVuota: AlgoritmoVuota, PartenzaRegistrato: AlgoritmoRegistrato, PartenzaConferma: AlgoritmoDaConferma,
		PartenzaStep: AlgoritmoImportazione, PartenzaSoloProdotto: AlgoritmoSoloProdotto}[c.Crea.Partenza]
	dopo := map[string]any{"partenza": c.Crea.Partenza, "algoritmo": algoritmo, "impronta": impronta, "nodi": len(snap.Nodi), "archi": len(snap.Archi),
		"traccia": tr}
	if da != nil {
		dopo["da_conferma"] = da
	}
	if c.Crea.Partenza == PartenzaSoloProdotto && c.Crea.PresaDAttoAnalisi != "" {
		dopo["presa_d_atto_analisi"] = c.Crea.PresaDAttoAnalisi
	}
	if passo != nil {
		dopo["importazione"] = passo.registro.Impronta
	}
	if vecchia != nil {
		dopo["da_bozza"] = map[string]any{"bozza_id": vecchia.BozzaID, "versione": vecchia.Versione}
	}
	presa.nelDopo(dopo)
	dopoNeiLimiti(dopo, elenchiDellaPresa...)
	motivoApertura := ""
	if a := c.Crea.Abbandona; a != nil {
		motivoApertura = a.Motivo
	}
	eventi := 1
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: nuovoID, N: 1, Versione: 1,
		Vocabolario: VocabolarioFormato3, Azione: AzBozzaAperta, Dopo: rawONull(canonico(dopo)), Origine: "server", Motivo: testo(motivoApertura),
		UtenteID: c.UtenteID}); err != nil {
		return out, erroreDiScrittura(err, "crea")
	}
	if passo != nil || c.Crea.Partenza == PartenzaSoloProdotto {
		if err := scriviFonteScelta(ctx, q, c, nuovoID, snap); err != nil {
			return out, err
		}
		eventi++
	}
	if c.PrimaDelCommit != nil {
		var imp *ImportazioneR2
		if passo != nil {
			imp = passo.importazione
		}
		if err := c.PrimaDelCommit(ctx, q, nuovoID, 1, imp); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if s := sqlstate(err); s == "P4X04" || s == "P4X07" || s == "P4X08" {
			return out, corpo("crea", "il database rifiuta la bozza nuova (%s)", s)
		}
		return out, err
	}
	out = Creata{Salvato3: Salvato3{BozzaID: nuovoID, Formato: FormatoSnapshot3, Versione: 1, NuovaVersione: true, Eventi: eventi,
		Impronta: impronta, SalvataIl: salvataIl}, Partenza: c.Crea.Partenza, Snapshot: canon, DaConferma: da, Traccia: tr}
	if vecchia != nil {
		id := vecchia.BozzaID
		out.BozzaAbbandonata = &id
	}
	if passo != nil {
		out.Importazione = passo.importazione
	}
	out.PreassociazioniProposte, out.ConversioniLegacy, out.PreassegnazioniNonConvertite = presa.Proposte, presa.Conversioni, presa.NonConvertite
	return out, nil
}

// passoStep: la partenza step preparata sotto il lucchetto (commento di testa): il registro, la versione 1, la traccia, e
// dopo la scrittura del registro la sua testata per la risposta.
type passoStep struct {
	registro     importazione.Registro
	snapshot     Snapshot3
	traccia      TracciaPartenza
	importazione *ImportazioneR2
}

// preparaPartenzaStep: i controlli della partenza step sotto il lucchetto e la versione 1 (commento di testa). vecchia e'
// la bozza della ripartenza, se c'e'.
func preparaPartenzaStep(ctx context.Context, q *db.Queries, c Creazione3, in IngressoPartenza, vecchia *db.DistintaBozza) (*passoStep, error) {
	reg := c.Proposta.Registro
	f := reg.Contenuto.Proposta.Fonte
	if f.AllegatoID != c.Crea.Sorgente.AllegatoID.String() {
		a, _ := uuid.Parse(f.AllegatoID)
		return nil, &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoGruppoSuAltraOccorrenza, Campo: "crea.sorgente.gruppo", AllegatoID: &a,
			Messaggio: "il gruppo calcolato sta su un'altra occorrenza dello STEP dichiarato"}
	}
	if c.Crea.Sorgente.Sha256 != "" && c.Crea.Sorgente.Sha256 != f.Sha256 {
		return nil, corpo("crea.sorgente.sha256", "lo sha256 dichiarato non e' quello dello STEP")
	}
	if err := VerificaImprontaDichiarata("crea.impronta_contenuto", reg, c.Crea.ImprontaContenuto); err != nil {
		return nil, err
	}
	// la ripartenza dalla stessa sorgente passa (U2 §2.8: «la fonte e' gia' confermata»; [T]: la stessa sorgente = lo
	// stesso sha256 della bozza abbandonata)
	ripartenza := vecchia != nil && vecchia.FonteSha256.Valid && strings.TrimSpace(vecchia.FonteSha256.String) == f.Sha256
	if !ripartenza {
		if err := fonteConfermata(ctx, q, c.ThreadID, c.Comune.ProdottoID, f.Sha256, "crea.sorgente"); err != nil {
			return nil, err
		}
	}
	prodotto, _ := in.Stato.Componente(c.Comune.ProdottoID)
	if err := motivoDellaFonte(reg, prodotto.CodiceLetterale, c.Crea.Motivo, "crea.motivo"); err != nil {
		return nil, err
	}
	if err := occorrenzeDelRegistro(ctx, q, c.ThreadID, reg, "crea"); err != nil {
		return nil, err
	}
	snap, tr, err := VersioneDaRegistro(reg, in)
	if err != nil {
		return nil, err
	}
	return &passoStep{registro: reg, snapshot: snap, traccia: tr}, nil
}

// scriviFonteScelta: dopo bozza_aperta, l'evento sorgente_scelta del server (dopo = la sorgente della versione 1, il motivo
// della scelta) e la storia della fonte (fonte_step_evento scelta, ruolo prodotto, con la bozza; per la partenza
// solo_prodotto il gruppo riservato solo_prodotto).
func scriviFonteScelta(ctx context.Context, q *db.Queries, c Creazione3, bozza uuid.UUID, snap Snapshot3) error {
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: bozza, N: 2, Versione: 1, Vocabolario: VocabolarioFormato3,
		Azione: AzSorgenteScelta, Dopo: rawONull(canonico(snap.Sorgente)), Origine: "server", Motivo: testo(c.Crea.Motivo), UtenteID: c.UtenteID}); err != nil {
		return erroreDiScrittura(err, "crea")
	}
	s := snap.Sorgente
	if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: c.ThreadID, ProdottoID: c.Comune.ProdottoID, Ruolo: "prodotto",
		Azione: "scelta", AllegatoID: *s.AllegatoID, Sha256: s.Sha256, Gruppo: testo(s.Gruppo), BozzaID: uuid.NullUUID{UUID: bozza, Valid: true},
		Motivo: testo(c.Crea.Motivo), AutoreID: c.UtenteID}); err != nil {
		return erroreDiScrittura(err, "crea")
	}
	return nil
}

// chiaveGiaUsata: la chiave del salvataggio cercata prima di ogni controllo di bozza e di versione (EM §9.2): con la stessa
// impronta della richiesta trovata = la versione gia' scritta; con un'altra impronta (o in un'altra RFQ) 409 CHIAVE_RIUSATA.
func chiaveGiaUsata(ctx context.Context, q *db.Queries, thread uuid.UUID, c ComuneR2) (db.GetVersionePerChiaveSalvataggioRow, bool, error) {
	var zero db.GetVersionePerChiaveSalvataggioRow
	gia, err := q.GetVersionePerChiaveGlobale(ctx, uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	if gia.ThreadID != thread || strings.TrimSpace(gia.ImprontaRichiesta.String) != c.ImprontaRichiesta {
		return zero, false, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio",
			Messaggio: "questa chiave di salvataggio ha gia' salvato un altro corpo: rileggi la bozza"}
	}
	v, err := q.GetVersionePerChiaveSalvataggio(ctx, db.GetVersionePerChiaveSalvataggioParams{ChiaveSalvataggio: uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true},
		ThreadID: thread})
	if err != nil {
		return zero, false, err
	}
	return v, true, nil
}

// RiproponiFormaDelServer: la ricerca della chiave in sola lettura, prima del calcolo del motore (EM §6.3 passo 0): con la
// stessa impronta della richiesta la versione gia' scritta (ok vero), con un'altra 409 CHIAVE_RIUSATA, senza la chiave ok
// falso. Chi chiama la usa nelle forme che importano per non calcolare il motore su un reinvio.
func RiproponiFormaDelServer(ctx context.Context, q *db.Queries, thread uuid.UUID, c ComuneR2) (Salvato3, bool, error) {
	v, ok, err := chiaveGiaUsata(ctx, q, thread, c)
	if err != nil || !ok {
		return Salvato3{}, false, err
	}
	return Salvato3{BozzaID: v.BozzaID, Formato: FormatoSnapshot3, Versione: int(v.Versione), Riproposto: true,
		Impronta: strings.TrimSpace(v.Impronta), SalvataIl: v.SalvataIl}, true, nil
}

// riproponiCreata: il reinvio di una creazione gia' fatta (stessa chiave, stesso corpo): la versione 1 con il suo snapshot,
// la partenza e le tracce dell'evento bozza_aperta, senza scrivere.
func riproponiCreata(ctx context.Context, q *db.Queries, thread uuid.UUID, v db.GetVersionePerChiaveSalvataggioRow) (Creata, error) {
	out := Creata{Salvato3: Salvato3{BozzaID: v.BozzaID, Formato: FormatoSnapshot3, Versione: int(v.Versione), Riproposto: true,
		Impronta: strings.TrimSpace(v.Impronta), SalvataIl: v.SalvataIl}}
	r, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: v.BozzaID, ThreadID: thread, Versione: v.Versione})
	if err != nil {
		return out, err
	}
	s, _, err := rileggiSnapshot3(r.Snapshot, r.Impronta)
	if err != nil {
		return out, err
	}
	if out.Snapshot, err = s.codifica(); err != nil {
		return out, err
	}
	e, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: v.BozzaID, ThreadID: thread, N: 1})
	if err != nil || e.Dopo == nil {
		return out, nil
	}
	var d struct {
		Partenza   string          `json:"partenza"`
		DaConferma *DaConfermaR2   `json:"da_conferma"`
		Traccia    TracciaPartenza `json:"traccia"`
		DaBozza    *struct {
			BozzaID uuid.UUID `json:"bozza_id"`
		} `json:"da_bozza"`
		Proposte      []PreassociazioneProposta3      `json:"preassociazioni_proposte"`
		Conversioni   []ConversioneLegacy3            `json:"conversioni_legacy"`
		NonConvertite []PreassegnazioneNonConvertita3 `json:"preassegnazioni_non_convertite"`
	}
	if json.Unmarshal(*e.Dopo, &d) == nil {
		out.Partenza, out.DaConferma, out.Traccia = d.Partenza, d.DaConferma, d.Traccia
		out.PreassociazioniProposte, out.ConversioniLegacy, out.PreassegnazioniNonConvertite = d.Proposte, d.Conversioni, d.NonConvertite
		if d.DaBozza != nil {
			id := d.DaBozza.BozzaID
			out.BozzaAbbandonata = &id
		}
	}
	if r.ImportazioneID.Valid {
		riga, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: r.ImportazioneID.UUID, ThreadID: thread})
		if err != nil {
			return out, err
		}
		reg, t, err := RileggiImportazione(riga)
		if err != nil {
			return out, err
		}
		out.Importazione = &ImportazioneR2{ImportazioneID: t.ImportazioneID, Impronta: t.Impronta, ImprontaContenuto: t.ImprontaContenuto,
			ImprontaEvidenze: t.ImprontaEvidenze, Riusata: true, FuoriPerDecisione: FuoriDelRegistro(reg.Contenuto)}
	}
	return out, nil
}

// ingressoPartenza: lo stato della RFQ, il cliente e le identita' certificate dei componenti, letti sotto il lucchetto.
func ingressoPartenza(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID) (IngressoPartenza, error) {
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return IngressoPartenza{}, err
	}
	cliente, err := q.GetClienteDellaRfq(ctx, thread)
	if err != nil {
		return IngressoPartenza{}, err
	}
	ids := make([]uuid.UUID, 0, len(st.Componenti))
	for _, c := range st.Componenti {
		ids = append(ids, c.ID)
	}
	cert := map[uuid.UUID]Certificazione{}
	if len(ids) > 0 {
		rr, err := q.ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: thread, Componenti: ids})
		if err != nil {
			return IngressoPartenza{}, err
		}
		cert = CertificazioniDa(rr)
	}
	return IngressoPartenza{Prodotto: prodotto, Stato: st, Certificazioni: cert, Cliente: EmittenteCliente(cliente)}, nil
}

// confermaDiPartenza: l'ultima conferma valida del prodotto per da-conferma-1 (EM §6.12): lo snapshot nel formato 3 (una
// formato 2 convertita con le regole della conversione, conversione_ambigua compresa; una formato 1 senza snapshot di nodi),
// la mappa, la fonte. Senza una conferma, o con l'ultima non piu' valida, 422 partenza_non_ammessa.
func confermaDiPartenza(ctx context.Context, q *db.Queries, thread uuid.UUID, in IngressoPartenza) (ConfermaDiPartenza, error) {
	var out ConfermaDiPartenza
	u, v, ok, err := in.Stato.ValiditaUltima(in.Prodotto)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, corpoMotivo("crea.partenza", MotivoPartenzaNonAmmessa, "il prodotto non ha conferme: la partenza conferma non c'e'")
	}
	if !v.Valida {
		return out, corpoMotivo("crea.partenza", MotivoPartenzaNonAmmessa,
			"l'ultima conferma del prodotto (n. %d) non e' piu' valida: la working e' cambiata dopo; si parte dalla BOM registrata", u.Numero)
	}
	r, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: thread, ConfermaID: u.ID})
	if err != nil {
		return out, err
	}
	out = ConfermaDiPartenza{ConfermaID: r.ConfermaID, Numero: int(r.Numero), Formato: int(r.Formato), Mappa: map[string]uuid.UUID{},
		ImportazioneID: r.ImportazioneID}
	if len(r.Mappa) > 0 {
		var m map[string]uuid.UUID
		if err := json.Unmarshal(r.Mappa, &m); err == nil {
			out.Mappa = m
		}
	}
	if r.FonteAllegatoID.Valid && r.FonteGruppo.Valid {
		a := r.FonteAllegatoID.UUID
		out.Sorgente = &Sorgente3{AllegatoID: &a, Sha256: strings.TrimSpace(r.FonteSha256.String), Gruppo: r.FonteGruppo.String}
	}
	letto := LeggiSnapshotConferma(int(r.Formato), r.Snapshot, r.ImprontaSnapshot)
	switch {
	case letto.NonLeggibile:
		return out, fmt.Errorf("lo snapshot della conferma %s non si rilegge: %s: %w", r.ConfermaID, letto.Motivo, ErrSnapshotIncoerente)
	case letto.Formato3 != nil:
		s := *letto.Formato3
		out.Snapshot = &s
	case letto.Formato2 != nil:
		// le caselle formato 2 si completano con l'allegato come nella conversione (EM §6.12, conversione_ambigua compreso);
		// identita', preassociazioni e decisioni le rifa' da-conferma-1
		s2 := *letto.Formato2
		s2.DocumentiPreassociati = nil
		var tenute []json.RawMessage
		for _, d := range s2.Decisioni {
			var t struct {
				Tipo string `json:"tipo"`
			}
			if json.Unmarshal(d, &t) == nil && t.Tipo == DecisioneMonoPezzo {
				tenute = append(tenute, d)
			}
		}
		s2.Decisioni = tenute
		s3, _, err := convertiSnapshot(ctx, q, thread, s2, in.Cliente)
		if err != nil {
			var e *Errore
			if errors.As(err, &e) {
				e.Campo = "crea.partenza"
			}
			return out, err
		}
		out.Snapshot = &s3
		if s3.Sorgente != nil {
			out.Sorgente = s3.Sorgente
		}
	}
	return out, nil
}

// erroreDiCostruzione: uno snapshot costruito dal server che non passa la forma del formato 3 e' un rifiuto della partenza
// (422 sul campo crea, con il motivo della forma), mai un 5xx: succede con dati della working che il formato 3 non ammette
// (per esempio un finito sotto un altro finito).
func erroreDiCostruzione(err error, partenza string) error {
	var e *Errore
	if errors.As(err, &e) {
		x := *e
		x.Messaggio = fmt.Sprintf("la versione 1 della partenza %s non ha la forma del formato 3 (%s): %s", partenza, e.Campo, e.Messaggio)
		x.Campo = "crea.partenza"
		return &x
	}
	return err
}

// preparaSoloProdotto: la partenza solo_prodotto sotto il lucchetto (forme3.go; direttiva 39 §3): lo STEP dichiarato e'
// un allegato della RFQ con lo sha256 dichiarato, attivo, confermato come fonte del prodotto (FONTE_NON_CONFERMATA); il
// motivo e' obbligatorio, perche' senza struttura il codice della radice non e' determinabile (DEC-B B-10). La versione
// 1 e' il solo prodotto con la sorgente reale e il gruppo riservato, senza registro.
func preparaSoloProdotto(ctx context.Context, q *db.Queries, c Creazione3, in IngressoPartenza) (Snapshot3, TracciaPartenza, error) {
	src, err := sorgenteSoloProdotto(ctx, q, c.ThreadID, c.Comune.ProdottoID, *c.Crea.Sorgente, c.Crea.Motivo, "crea")
	if err != nil {
		return Snapshot3{}, TracciaPartenza{}, err
	}
	snap, tr, err := PartenzaRegistrata(PartenzaVuota, in)
	if err != nil {
		return snap, tr, err
	}
	snap.Sorgente = &src
	return snap, tr, nil
}

// sorgenteSoloProdotto: i controlli della sorgente solo_prodotto sotto il lucchetto (preparaSoloProdotto e
// SoloProdottoSuBozzaFormato3): il gruppo riservato; lo STEP e' un allegato della RFQ, scaricato, con lo sha256
// dichiarato, attivo, confermato come fonte del prodotto (FONTE_NON_CONFERMATA); il motivo obbligatorio (DEC-B B-10).
func sorgenteSoloProdotto(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, s SorgenteDichiarata, motivo, forma string) (Sorgente3, error) {
	if s.Gruppo != GruppoSoloProdotto {
		return Sorgente3{}, corpo(forma+".sorgente.gruppo", "la partenza solo_prodotto ha il gruppo riservato %q", GruppoSoloProdotto)
	}
	a, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{AllegatoID: s.AllegatoID, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return Sorgente3{}, fuori(forma+".sorgente.allegato_id", "l'allegato %s non e' di questa RFQ", s.AllegatoID)
	}
	if err != nil {
		return Sorgente3{}, err
	}
	sha := strings.TrimSpace(a.Sha256.String)
	if !a.Sha256.Valid || sha == "" {
		return Sorgente3{}, corpo(forma+".sorgente.allegato_id", "lo STEP non e' ancora scaricato: non ha lo sha256")
	}
	if s.Sha256 != "" && s.Sha256 != sha {
		return Sorgente3{}, corpo(forma+".sorgente.sha256", "lo sha256 dichiarato non e' quello dello STEP")
	}
	st, err := q.StatoOccorrenzaAllegato(ctx, s.AllegatoID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Sorgente3{}, err
	}
	if err == nil && (st.Stato != "attivo" || st.ScartatoLegacy) {
		return Sorgente3{}, &Errore{Codice: CodiceFileNonAttivo, Campo: forma + ".sorgente.allegato_id",
			Messaggio: "lo STEP non e' un file attivo della RFQ (" + st.Stato + ")"}
	}
	if err := fonteConfermata(ctx, q, thread, prodotto, sha, forma+".sorgente"); err != nil {
		return Sorgente3{}, err
	}
	if strings.TrimSpace(motivo) == "" {
		return Sorgente3{}, &Errore{Codice: CodiceMotivoObbligatorio, Campo: forma + ".motivo",
			Messaggio: "senza struttura importabile il codice della radice dello STEP non e' determinabile: la scelta della fonte vuole il motivo"}
	}
	all := s.AllegatoID
	return Sorgente3{AllegatoID: &all, Sha256: sha, Gruppo: GruppoSoloProdotto}, nil
}

// vuotaAmmessa: la partenza vuota (direttiva 39 §1, §3 [U]: bozza senza STEP, sola radice, niente sorgente, registro o
// fonte_step_evento) si rifiuta, 422 CORPO_NON_VALIDO partenza_non_ammessa, solo dove c'e' gia' una strada che non perde
// lavoro (EM App. I.5, rilievo R39-19, base [T-39] adottata dall'orchestratore): con una conferma formato 3 valida la
// strada e' da-conferma-1 (come registrato, DEC-B PD-02 (b)); con uno STEP gia' confermato come sorgente da S8 e ancora
// valido (confermabom.SorgenteValida) la strada e' S8, che da' la partenza step o solo_prodotto. Il legame storico senza
// S8 (D-AR04 (iii)) non basta a rifiutarla: non e' una sorgente valida.
func vuotaAmmessa(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, in IngressoPartenza) error {
	if u, v, ok, err := in.Stato.ValiditaUltima(prodotto); err != nil {
		return err
	} else if ok && v.Valida && u.Formato == FormatoSnapshot3 {
		return corpoMotivo("crea.partenza", MotivoPartenzaNonAmmessa,
			"il prodotto ha una conferma formato 3 valida (n. %d): la strada e' «Modifica la BOM confermata» (partenza conferma)", u.Numero)
	}
	ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return err
	}
	visti := map[string]bool{}
	for _, e := range ee {
		sha := strings.TrimSpace(e.Sha256)
		if e.Ruolo != "prodotto" || e.Azione != "conferma_documento" || !e.ChiaveIdempotenza.Valid || visti[sha] {
			continue
		}
		visti[sha] = true
		st, err := confermabom.SorgenteValida(ctx, q, thread, prodotto, sha)
		if err != nil {
			return err
		}
		if st.Valida {
			return corpoMotivo("crea.partenza", MotivoPartenzaNonAmmessa,
				"il prodotto ha uno STEP confermato come sorgente: la bozza parte dalla conferma della sorgente (S8), non vuota")
		}
	}
	return nil
}
