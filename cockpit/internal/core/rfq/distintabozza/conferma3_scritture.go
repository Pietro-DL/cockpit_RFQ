package distintabozza

// conferma3_scritture.go: le scritture di R5 formato 3 (conferma3.go), nell'ordine di EM App. I.6 (direttiva 39 §2, §7
// [U + T-39]), tutte nella transazione T dopo i controlli e i lucchetti: working (con la promozione della sorgente) →
// sostituzione documentale di A dopo B → testata → righe e archi certificati e non inclusi → AR e copie dal servizio unico →
// segni legacy → chiusura della bozza, intento e f8_stato. I controlli differiti al COMMIT li fa il database.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/rfq/traccia"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// WidgetMonoPezzo: il gesto della pagina che la traccia della dichiarazione mono_pezzo nomina (messaggio 46 [U]).
const WidgetMonoPezzo = "Dichiara mono_pezzo"

// MotivoMonoPezzoDelGesto: per la prima istanza di R4, il motivo di una dichiarazione mono_pezzo della bozza senza il
// motivo dell'utente (messaggio 46 [U]): la dichiarazione e' il gesto, e alla conferma R5 scrive la traccia.
const MotivoMonoPezzoDelGesto = "dichiarazione mono_pezzo della bozza (gesto «" + WidgetMonoPezzo + "»)"

// NotaNonInclusoR5: la nota fissa delle righe che R5 chiude per una non_includere (EM §7.5 punto 1).
const NotaNonInclusoR5 = "non incluso nella BOM (conferma formato 3)"

// PrefissoNotaAssenzaR5: l'inizio della nota fissa delle righe che R5 chiude per una voce della proposta assente dalla
// bozza (EM §7.5 con U2 §6.1: «esclusa con la conferma cumulativa n, senza motivo scritto»; il motivo che manca si
// dichiara, non si inventa). Le esclusioni legacy di R4 non contano queste righe.
const PrefissoNotaAssenzaR5 = "esclusa con la conferma cumulativa"

// notaAssenzaR5: la nota fissa dell'assenza con il numero della conferma.
func notaAssenzaR5(numero int) string {
	return fmt.Sprintf("%s n. %d, senza motivo scritto", PrefissoNotaAssenzaR5, numero)
}

// Le chiavi di origine della conferma per la correzione della sorgente (G39-03 [U]; forma [T-39], nessuna colonna nuova):
// correzione_sorgente: true e revisione: interna. R7 le rilegge (ConfermaR7.CorrezioneSorgente, Revisione).
const (
	OrigineCorrezioneSorgente = "correzione_sorgente"
	OrigineRevisione          = "revisione"
	RevisioneInterna          = "interna"
)

// MotivoArchiviazioneR5: il motivo dell'archiviazione di un componente escluso senza motivo proprio (op esclude).
const MotivoArchiviazioneR5 = "escluso dalla BOM con la conferma formato 3"

type scrittureR5 struct {
	ctx       context.Context
	tx        pgx.Tx
	q         *db.Queries
	thread    uuid.UUID
	utente    uuid.UUID
	chiave    uuid.UUID
	snap      Snapshot3
	impronta  string
	versione  db.GetDistintaBozzaVersioneRow
	rp        RiepilogoR4
	p         *pianoR5
	st        *confermabom.StatoRFQ
	guasto    func(string) error
	richiesta RichiestaR5
	// invertiTestata: solo per le prove dell'ordine (OpzioniR5.invertiTestata)
	invertiTestata bool

	nodi       map[string]Nodo3
	mappa      map[string]uuid.UUID
	creati     map[string]bool
	scritti    map[[2]uuid.UUID]bool // archi nati o cambiati da R5
	numero     int
	precedente uuid.NullUUID
	conferma   uuid.UUID
	il         string
	out        ConfermaR5
}

func (sc *scrittureR5) passo(nome string) error {
	if sc.guasto == nil {
		return nil
	}
	return sc.guasto(nome)
}

func (sc *scrittureR5) esegui() (ConfermaR5, error) {
	sc.nodi = map[string]Nodo3{}
	for _, n := range sc.snap.Nodi {
		sc.nodi[n.ChiaveBozza] = n
	}
	sc.mappa, sc.creati, sc.scritti = map[string]uuid.UUID{}, map[string]bool{}, map[[2]uuid.UUID]bool{}
	sc.il = time.Now().UTC().Format(time.RFC3339)
	sc.out = ConfermaR5{Esito: "confermata", ChiaveIdempotenza: sc.chiave, Formato: FormatoSnapshot3, ProdottoID: sc.snap.ProdottoID,
		BozzaID: sc.versione.BozzaID, BozzaVersione: int(sc.versione.Versione), ArchiAggiunti: []ArcoScritto{}, ArchiTolti: []ArcoScritto{},
		ArchiModificati: []ArcoScritto{}, IdentitaCreate: []uuid.UUID{}, IdentitaRiusate: []uuid.UUID{}, IdentitaArchiviate: []uuid.UUID{},
		IdentitaAccettate: []string{}, IdentitaCorrette: []string{}, EsclusiCondivisi: []uuid.UUID{}, IdentitaRicodificate: []uuid.UUID{},
		IdentitaRipristinate: []uuid.UUID{}, TipiCambiati: []uuid.UUID{}}
	ultima, ok := sc.st.Ultima(sc.snap.ProdottoID)
	sc.numero = 1
	if ok {
		sc.numero = int(ultima.Numero) + 1
		sc.precedente = uuid.NullUUID{UUID: ultima.ID, Valid: true}
	}
	var base uuid.NullUUID
	if b := sc.snap.VersioneBomBase; b != nil && b.ConfermaID != nil {
		base = uuid.NullUUID{UUID: *b.ConfermaID, Valid: true}
	}
	if base != sc.precedente {
		return ConfermaR5{}, &Errore{Codice: CodiceBaseSuperata, Motivi: []string{MotivoConfermaCambiata}, VersioneCorrente: &sc.rp.VersioneCorrente,
			Messaggio: "la base della bozza non è l'ultima conferma del prodotto: rileggi la base e riconferma"}
	}
	for _, k := range sc.p.inclusi {
		if id, ok := sc.p.esistenti[k]; ok {
			sc.mappa[k] = id
			if !contieneUUID(sc.out.IdentitaRiusate, id) {
				sc.out.IdentitaRiusate = append(sc.out.IdentitaRiusate, id)
			}
		}
	}
	passi := []func() error{sc.working, sc.sostituzione, sc.testata, sc.certificati, sc.associazioni, sc.segni, sc.chiusura}
	if sc.invertiTestata {
		passi = []func() error{sc.testata, sc.working, sc.sostituzione, sc.certificati, sc.associazioni, sc.segni, sc.chiusura}
	}
	for _, f := range passi {
		if err := f(); err != nil {
			return ConfermaR5{}, erroreAlCommit(err)
		}
	}
	return sc.out, nil
}

// ------------------------------------------------------------------ 1. la working, con la promozione della sorgente

func (sc *scrittureR5) working() error {
	ctx, q, p := sc.ctx, sc.q, sc.p
	// le ricodifiche dei registrati, prima dei componenti nuovi (un pezzo nuovo puo' prendere il codice che una ricodifica
	// lascia): il codice nella working solo se e' ancora quello della decisione; la FK a tre campi delle copie e' differita
	// fino al COMMIT, le copie si allineano al passo delle AR (AllineaCodiceCopia). Un documento confermato del componente e'
	// gia' un blocco di R4 (RICODIFICA_FILE_NAS): R5 non scrive documento (D-ASS-01 [U]; direttiva 35 §7).
	if len(p.ricodifiche) > 0 {
		if _, err := sc.tx.Exec(ctx, `SET CONSTRAINTS fk_proposta_componente DEFERRED`); err != nil {
			return err
		}
	}
	for _, d := range p.ricodifiche {
		n, err := q.RicodificaComponenteR5(ctx, db.RicodificaComponenteR5Params{CodiceDopo: d.CodiceDopo, ConfermatoDa: sc.utente,
			ComponenteID: *d.ComponenteID, ThreadID: sc.thread, CodicePrima: d.CodicePrima})
		if err != nil {
			if sqlstate(err) == "23505" {
				return &Errore{Codice: CodiceBaseSuperata, Motivi: []string{MotivoCodiceOccupato}, VersioneCorrente: &sc.rp.VersioneCorrente,
					Messaggio: fmt.Sprintf("il codice %s è stato occupato nella RFQ dopo la tua lettura: rileggi la base e riconferma", d.CodiceDopo)}
			}
			return err
		}
		if n != 1 {
			return rifiutoRiprova("il codice di un componente è cambiato mentre confermavi: riprova")
		}
		sc.out.IdentitaRicodificate = append(sc.out.IdentitaRicodificate, *d.ComponenteID)
	}
	// i componenti archiviati che la bozza riusa (EM §5.2 W3): di nuovo nella working, stesso componente e stessa storia;
	// gli archi li scrive R5 dalla bozza. Un archiviato con un'autorizzazione STEP e' un blocco di R4 (RIUSO_INCOMPATIBILE
	// archiviato_con_autorizzazione): il ripristino non tocca le autorizzazioni ne' le rimozioni [T-39]
	for _, id := range p.ripristinare {
		n, err := q.RipristinaComponenteR5(ctx, db.RipristinaComponenteR5Params{ConfermatoDa: sc.utente, ComponenteID: id, ThreadID: sc.thread})
		if err != nil {
			return err
		}
		if n != 1 {
			return rifiutoRiprova("un componente archiviato è cambiato mentre confermavi: riprova")
		}
		sc.out.IdentitaRipristinate = append(sc.out.IdentitaRipristinate, id)
	}
	// i componenti nuovi: una INSERT senza ON CONFLICT (BL V1-Q04 A): un 23505 e' uno scrittore legacy senza il lucchetto
	for _, k := range p.daCreare {
		n := sc.nodi[k]
		origine := db.OrigineComponenteManuale
		if n.Origine == "step" {
			origine = db.OrigineComponenteStep
		}
		id, err := q.InsertComponenteSenzaFusione(ctx, db.InsertComponenteSenzaFusioneParams{ThreadID: sc.thread, Codice: n.Identita.Codice,
			Rev: testoNullo(n.Identita.Rev), Descrizione: testoNullo(n.Descrizione), Qta: 1, Tipo: db.TipoComponente(p.tipo[k]), Origine: origine,
			ConfermatoDa: sc.utente})
		if err != nil {
			if sqlstate(err) == "23505" {
				return &Errore{Codice: CodiceBaseSuperata, Motivi: []string{MotivoCodiceOccupato}, VersioneCorrente: &sc.rp.VersioneCorrente,
					Messaggio: fmt.Sprintf("il codice %s è stato occupato nella RFQ dopo la tua lettura: rileggi la base e riconferma", n.Identita.Codice)}
			}
			return err
		}
		sc.mappa[k] = id
		sc.creati[k] = true
		sc.out.IdentitaCreate = append(sc.out.IdentitaCreate, id)
	}
	for k, prima := range p.alias {
		sc.mappa[k] = sc.mappa[prima]
		sc.creati[k] = true
	}
	// tipo, revisione e descrizione dei componenti che c'erano (l'identita' decisa porta la rev; una descrizione vuota lascia
	// quella della working [T-39]; il tipo dopo cambia_tipo e D1 del nodo del componente, con l'effetto che R4 ha visto
	// applicabile: TIPO_EFFETTO_CAMBIATO effetto_non_applicabile altrimenti), una volta per componente
	scritto := map[uuid.UUID]bool{}
	for _, k := range p.inclusi {
		id, ok := p.esistenti[k]
		if !ok || scritto[id] {
			continue
		}
		scritto[id] = true
		c, _ := sc.st.Componente(id)
		n := sc.nodi[k]
		rev, descr, tipo := n.Identita.Rev, c.Descrizione, p.tipo[k]
		if n.Descrizione != "" {
			descr = n.Descrizione
		}
		if rev == c.Rev && descr == c.Descrizione && tipo == c.Tipo {
			continue
		}
		if _, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{Tipo: db.TipoComponente(tipo), Rev: testoNullo(rev),
			Descrizione: testoNullo(descr), ConfermatoDa: sc.utente, ComponenteID: id}); err != nil {
			return err
		}
		if tipo != c.Tipo {
			sc.out.TipiCambiati = append(sc.out.TipiCambiati, id)
		}
	}
	// gli archi: tolti (con la loro decisione), cambiati, nuovi
	voluti := map[[2]uuid.UUID]arcoR5{}
	entranti := map[uuid.UUID][]uuid.UUID{}
	for _, x := range p.voluti {
		k := [2]uuid.UUID{sc.mappa[x.padre], sc.mappa[x.figlio]}
		voluti[k] = x
		entranti[k[1]] = append(entranti[k[1]], k[0])
	}
	esistenti := map[uuid.UUID]bool{}
	for _, id := range p.esistenti {
		esistenti[id] = true
	}
	inBozza := map[uuid.UUID]bool{}
	for _, id := range sc.mappa {
		inBozza[id] = true
	}
	working := map[[2]uuid.UUID]confermabom.Arco{}
	var tolti []confermabom.Arco
	for _, a := range sc.st.Archi {
		if !esistenti[a.Padre] {
			continue
		}
		if f, ok := sc.st.Componente(a.Figlio); ok && f.Archiviato {
			continue
		}
		k := [2]uuid.UUID{a.Padre, a.Figlio}
		if _, ok := voluti[k]; ok {
			working[k] = a
			continue
		}
		spostato := false
		for _, pp := range entranti[a.Figlio] {
			if pp != a.Padre {
				spostato = true
			}
		}
		if !p.togli[k] && !p.escluso[a.Figlio] && !(inBozza[a.Figlio] && spostato) {
			return rifiutoBlocchi([]Rilievo{{Codice: BloccoAssenzaNonDecisa, Motivo: "arco", ComponenteID: uid(a.Figlio),
				Testo:  "Un arco della BOM registrata non c'è nella bozza e non ha una decisione: scollegalo esplicitamente o rimettilo.",
				Rimedi: []string{"«Scollega da un padre…» (togli_arco)", "rimetti l'arco"}}})
		}
		tolti = append(tolti, a)
	}
	sort.Slice(tolti, func(i, j int) bool {
		if tolti[i].Padre != tolti[j].Padre {
			return tolti[i].Padre.String() < tolti[j].Padre.String()
		}
		return tolti[i].Figlio.String() < tolti[j].Figlio.String()
	})
	for _, a := range tolti {
		if n, err := q.DeleteRelazione(ctx, db.DeleteRelazioneParams{PadreID: a.Padre, FiglioID: a.Figlio}); err != nil {
			return err
		} else if n != 1 {
			return rifiutoRiprova("un arco della working è cambiato mentre confermavi: riprova")
		}
		qp := int(a.Qta)
		sc.out.ArchiTolti = append(sc.out.ArchiTolti, ArcoScritto{PadreID: a.Padre, FiglioID: a.Figlio, Qta: qp})
	}
	chiaviVoluti := make([][2]uuid.UUID, 0, len(voluti))
	for k := range voluti {
		chiaviVoluti = append(chiaviVoluti, k)
	}
	sort.Slice(chiaviVoluti, func(i, j int) bool {
		if chiaviVoluti[i][0] != chiaviVoluti[j][0] {
			return chiaviVoluti[i][0].String() < chiaviVoluti[j][0].String()
		}
		return chiaviVoluti[i][1].String() < chiaviVoluti[j][1].String()
	})
	for _, k := range chiaviVoluti {
		x := voluti[k]
		w, c := working[k]
		if c {
			if int(w.Qta) != x.qta {
				if n, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{Qta: int32(x.qta), ConfermatoDa: sc.utente, PadreID: k[0], FiglioID: k[1]}); err != nil {
					return err
				} else if n != 1 {
					return rifiutoRiprova("un arco della working è cambiato mentre confermavi: riprova")
				}
				qp := int(w.Qta)
				sc.out.ArchiModificati = append(sc.out.ArchiModificati, ArcoScritto{PadreID: k[0], FiglioID: k[1], Qta: x.qta, QtaPrecedente: &qp})
				sc.scritti[k] = true
			}
			if w.Posizione != x.posizione {
				if _, err := q.SetPosizioneRelazioneR5(ctx, db.SetPosizioneRelazioneR5Params{Posizione: testoNullo(x.posizione), ConfermatoDa: sc.utente,
					ThreadID: sc.thread, PadreID: k[0], FiglioID: k[1]}); err != nil {
					return err
				}
			}
			continue
		}
		origine := db.OrigineComponenteManuale
		if x.origine == "step" {
			origine = db.OrigineComponenteStep
		}
		if err := q.InsertRelazioneR5(ctx, db.InsertRelazioneR5Params{ThreadID: sc.thread, PadreID: k[0], FiglioID: k[1], Qta: int32(x.qta),
			Posizione: testoNullo(x.posizione), Origine: origine, ConfermatoDa: sc.utente}); err != nil {
			if sqlstate(err) == "23505" {
				return rifiutoRiprova("un arco della working è nato mentre confermavi: riprova")
			}
			return err
		}
		sc.out.ArchiAggiunti = append(sc.out.ArchiAggiunti, ArcoScritto{PadreID: k[0], FiglioID: k[1], Qta: x.qta})
		sc.scritti[k] = true
	}
	// i componenti esclusi: archiviati se restano senza padri, altrimenti condivisi con un altro prodotto [T-39]
	motivi := map[uuid.UUID]string{}
	for _, d := range sc.snap.Decisioni {
		if d.Tipo == DecisioneEscludi && strings.TrimSpace(d.Motivo) != "" {
			motivi[*d.ComponenteID] = d.Motivo
		}
	}
	for _, id := range p.archiviare {
		m := motivi[id]
		if m == "" {
			m = MotivoArchiviazioneR5
		}
		if _, err := fascicolo.ArchiviaComponente(ctx, q, sc.thread, id, sc.utente, m); err != nil {
			return err
		}
		sc.out.IdentitaArchiviate = append(sc.out.IdentitaArchiviate, id)
	}
	sc.out.EsclusiCondivisi = append(sc.out.EsclusiCondivisi, p.condivisi...)
	// la promozione della sorgente B in componente.step_strutturale_id: R5 e' l'unico scrittore nel formato 3 (D39 §2)
	if p.stepA.Valid {
		a := p.stepA.UUID
		sc.out.Fonte.StepPrima = &a
	}
	if p.stepB != uuid.Nil {
		b := p.stepB
		sc.out.Fonte.StepDopo = &b
	}
	if p.promozione {
		if n, err := q.SetStepStrutturale(ctx, db.SetStepStrutturaleParams{StepStrutturaleID: uuid.NullUUID{UUID: p.stepB, Valid: true},
			ComponenteID: sc.snap.ProdottoID}); err != nil {
			return err
		} else if n != 1 {
			return rifiutoRiprova("il prodotto è cambiato mentre confermavi: riprova")
		}
		// R39-07 [T-39 corsia contratto, adottata dall'orchestratore]: l'evento autorizzazione, perche' /dati e R7 restino veri
		src := sc.snap.Sorgente
		if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: sc.thread, ProdottoID: sc.snap.ProdottoID, Ruolo: "prodotto",
			Azione: "autorizzazione", AllegatoID: *src.AllegatoID, Sha256: src.Sha256, Gruppo: testoNullo(src.Gruppo),
			DocumentoID: uuid.NullUUID{UUID: p.stepB, Valid: true}, BozzaID: uuid.NullUUID{UUID: sc.versione.BozzaID, Valid: true},
			Motivo:            pgtype.Text{String: fmt.Sprintf("promossa con la conferma n. %d", sc.numero), Valid: true},
			ChiaveIdempotenza: uuid.NullUUID{UUID: sc.chiave, Valid: true}, AutoreID: sc.utente}); err != nil {
			return err
		}
		sc.out.Fonte.Promossa = true
	}
	return sc.passo("promozione")
}

// ------------------------------------------------------------------ 2. la sostituzione documentale A → B

func (sc *scrittureR5) sostituzione() error {
	p := sc.p
	if !p.sostituzione {
		return sc.passo("sostituzione")
	}
	n, err := sc.q.SetSostituitoDa(sc.ctx, db.SetSostituitoDaParams{Nuovo: uuid.NullUUID{UUID: p.stepB, Valid: true}, Vecchio: p.stepA.UUID})
	if err != nil {
		return err
	}
	if n != 1 {
		return rifiutoRiprova("lo STEP di prima del prodotto è stato sostituito mentre confermavi: riprova")
	}
	a := p.stepA.UUID
	sc.out.Fonte.SostituzioneDi = &a
	return sc.passo("sostituzione")
}

// ------------------------------------------------------------------ 3. la testata

func (sc *scrittureR5) testata() error {
	ctx, q := sc.ctx, sc.q
	st2, err := confermabom.LeggiStatoRFQ(ctx, q, sc.thread)
	if err != nil {
		return err
	}
	// il perimetro scritto e' quello della bozza (difesa: il database lo ricontrolla al COMMIT)
	per, err := st2.Perimetro(sc.snap.ProdottoID)
	if err != nil {
		return err
	}
	attesi := map[uuid.UUID]bool{}
	for _, k := range sc.p.inclusi {
		attesi[sc.mappa[k]] = true
	}
	if len(per.Componenti) != len(attesi) || len(per.Archi) != len(sc.p.voluti) {
		return &RifiutoR5{Err: &Errore{Codice: CodiceControlloFinale, Motivo: "perimetro",
			Messaggio: fmt.Sprintf("il perimetro scritto (%d componenti, %d archi) non è quello della bozza (%d, %d): niente è stato registrato",
				len(per.Componenti), len(per.Archi), len(attesi), len(sc.p.voluti))}, NonRegistrato: true}
	}
	for _, c := range per.Componenti {
		if !attesi[c.ID] {
			return &RifiutoR5{Err: &Errore{Codice: CodiceControlloFinale, Motivo: "perimetro",
				Messaggio: "il perimetro scritto non è quello della bozza: niente è stato registrato"}, NonRegistrato: true}
		}
	}
	firma, err := per.Firma()
	if err != nil {
		return err
	}
	// i registri citati dallo snapshot (sorgente e decisioni), del prodotto
	impronte := map[string]bool{}
	if sc.snap.Sorgente != nil && sc.snap.Sorgente.Importazione != "" {
		impronte[sc.snap.Sorgente.Importazione] = true
	}
	for _, d := range sc.snap.Decisioni {
		if d.Importazione != "" {
			impronte[d.Importazione] = true
		}
	}
	citate := []uuid.UUID{}
	if len(impronte) > 0 {
		ii := make([]string, 0, len(impronte))
		for k := range impronte {
			ii = append(ii, k)
		}
		sort.Strings(ii)
		rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: sc.thread, ProdottoID: sc.snap.ProdottoID, Impronte: ii})
		if err != nil {
			return err
		}
		if len(rr) != len(ii) {
			return fuori("snapshot", "lo snapshot cita registri d'importazione che non sono del prodotto")
		}
		for _, r := range rr {
			citate = append(citate, r.ImportazioneID)
		}
		citate = uuidUnici(citate)
	}
	mono, motivoMono := false, ""
	for _, d := range sc.snap.Decisioni {
		if d.Tipo == DecisioneMonoPezzo {
			mono, motivoMono = true, d.Motivo
		}
	}
	// messaggio 46 [U]: la dichiarazione mono_pezzo non chiede piu' il motivo all'utente; ck_conferma_mono_pezzo lo vuole,
	// e lo scrive il server come traccia del gesto (la decisione resta la dichiarazione della bozza)
	if mono && strings.TrimSpace(motivoMono) == "" {
		if motivoMono, err = traccia.DelGesto(ctx, q, sc.utente, MaxMotivo, WidgetMonoPezzo,
			"il prodotto è un pezzo unico: dichiarazione mono_pezzo della bozza, registrata con la conferma n. "+strconv.Itoa(sc.numero)); err != nil {
			return err
		}
	}
	mappaJSON := map[string]string{}
	for k, id := range sc.mappa {
		mappaJSON[k] = id.String()
	}
	origine := map[string]any{"rotta": RottaR5, "formato": FormatoSnapshot3}
	// la correzione della sorgente (G39-03 [U]: una revisione interna, non del cliente), dalla R4 calcolata in S e firmata
	if sc.rp.EffettoSullaFonte.CorrezioneSorgente {
		origine[OrigineCorrezioneSorgente] = true
		origine[OrigineRevisione] = RevisioneInterna
		sc.out.Fonte.CorrezioneSorgente = true
	}
	if sc.richiesta.PresaDAttoPresente {
		origine["presa_d_atto_in_analisi"] = sc.richiesta.PresaDAtto
	}
	if len(sc.rp.FileInAnalisi) > 0 {
		var ff []uuid.UUID
		for _, f := range sc.rp.FileInAnalisi {
			ff = append(ff, f.AllegatoID)
		}
		origine["file_in_analisi"] = ff
	}
	delta := map[string]any{"formato": FormatoSnapshot3, "identita": sc.rp.Identita, "archi": sc.rp.Archi, "fuori": sc.rp.Fuori,
		"da_ricollocare": sc.rp.DaRicollocare, "esclusi": sc.rp.Esclusi, "effetti_documentali": sc.rp.EffettiDocumentali,
		"effetto_sulla_fonte": sc.rp.EffettoSullaFonte}
	can, err := sc.snap.Canonico()
	if err != nil {
		return err
	}
	var base uuid.NullUUID
	var baseContatore int64
	if b := sc.snap.VersioneBomBase; b != nil {
		baseContatore = b.Contatore
		if b.ConfermaID != nil {
			base = uuid.NullUUID{UUID: *b.ConfermaID, Valid: true}
		}
	}
	src := sc.snap.Sorgente
	var fonteAll uuid.NullUUID
	var fonteSha, fonteGruppo pgtype.Text
	if src != nil && src.AllegatoID != nil {
		fonteAll, fonteSha = uuid.NullUUID{UUID: *src.AllegatoID, Valid: true}, pgtype.Text{String: src.Sha256, Valid: true}
	}
	if src != nil {
		fonteGruppo = testoNullo(src.Gruppo)
	}
	c, err := q.InsertConfermaStrutturale(ctx, db.InsertConfermaStrutturaleParams{ThreadID: sc.thread, ProdottoID: sc.snap.ProdottoID,
		Numero: int32(sc.numero), PrecedenteID: sc.precedente, Formato: FormatoSnapshot3, SenzaRiepilogoV2: false, FirmaStruttura: firma,
		FirmaAnteprima: pgtype.Text{String: sc.rp.FirmaAnteprima, Valid: true}, BaseConfermaID: base, BaseContatore: baseContatore,
		FirmaBase: pgtype.Text{String: sc.snap.FirmaBase, Valid: sc.snap.FirmaBase != ""}, Contatore: st2.Contatore, ContatoreDati: st2.ContatoreDati,
		BozzaID: uuid.NullUUID{UUID: sc.versione.BozzaID, Valid: true}, BozzaVersione: pgtype.Int4{Int32: sc.versione.Versione, Valid: true},
		FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, Origine: canonico(origine), Snapshot: can,
		ImprontaSnapshot: sc.impronta, Delta: canonico(delta), Mappa: canonico(mappaJSON), Decisioni: canonico(sc.rp.RegistroDecisioni),
		MonoPezzo: mono, MonoPezzoMotivo: pgtype.Text{String: motivoMono, Valid: mono}, CriterioVerifica: sc.rp.CriterioPrevisto.Criterio,
		Verifica: json.RawMessage(`{"regola": "prima-istanza-1"}`), ChiaveIdempotenza: uuid.NullUUID{UUID: sc.chiave, Valid: true},
		ConfermatoDa: sc.utente, ImportazioneID: sc.versione.ImportazioneID, ImportazioniCitate: citate})
	if err != nil {
		return err
	}
	sc.conferma = c.ConfermaID
	m := map[string]uuid.UUID{}
	for k, v := range sc.mappa {
		m[k] = v
	}
	sc.out.ConfermaID, sc.out.NumeroConferma, sc.out.FirmaStruttura, sc.out.Contatore, sc.out.Mappa = c.ConfermaID, sc.numero, firma, st2.Contatore, m
	sc.out.CriterioVerifica, sc.out.MonoPezzo = sc.rp.CriterioPrevisto.Criterio, mono
	return sc.passo("testata")
}

// ------------------------------------------------------------------ 4. righe e archi certificati, non inclusi

func (sc *scrittureR5) certificati() error {
	ctx, q := sc.ctx, sc.q
	righe, err := q.ListComponentiStatoRfq(ctx, sc.thread)
	if err != nil {
		return err
	}
	perID := map[uuid.UUID]db.ListComponentiStatoRfqRow{}
	for _, r := range righe {
		perID[r.ComponenteID] = r
	}
	certificato := map[uuid.UUID]bool{}
	for _, k := range sc.p.inclusi {
		id := sc.mappa[k]
		c, ok := perID[id]
		if !ok {
			return fmt.Errorf("conferma: il componente %s del nodo %s non c'è", id, k)
		}
		if certificato[id] {
			// lo stesso componente di un nodo prima (W1): una riga certificata sola, con la chiave del primo nodo
			if sc.nodi[k].Identita.Modo == ModoProposta {
				sc.out.IdentitaAccettate = append(sc.out.IdentitaAccettate, k)
			}
			continue
		}
		certificato[id] = true
		n := sc.nodi[k]
		radice := id == sc.snap.ProdottoID
		var step uuid.NullUUID
		var sha pgtype.Text
		if radice && c.StepStrutturaleID.Valid {
			step = c.StepStrutturaleID
			d, err := q.GetDocumento(ctx, c.StepStrutturaleID.UUID)
			if err != nil {
				return err
			}
			sha = pgtype.Text{String: strings.TrimSpace(d.Sha256), Valid: true}
		}
		if err := q.InsertConfermaStrutturaleComponente(ctx, db.InsertConfermaStrutturaleComponenteParams{ConfermaID: sc.conferma, ThreadID: sc.thread,
			ComponenteID: id, Radice: radice, ChiaveBozza: pgtype.Text{String: k, Valid: true}, Codice: c.Codice, Rev: c.Rev, Descrizione: c.Descrizione,
			Tipo: c.Tipo, StepStrutturaleID: step, StepSha256: sha, Emittente: pgtype.Text{String: n.Identita.Emittente, Valid: true},
			EmittentePresunto: pgtype.Bool{Bool: n.Identita.EmittentePresunto, Valid: true}, Namespace: testoNullo(n.Identita.Namespace)}); err != nil {
			return err
		}
		if n.Identita.Modo == ModoProposta {
			sc.out.IdentitaAccettate = append(sc.out.IdentitaAccettate, k)
		}
	}
	for _, x := range sc.p.voluti {
		if err := q.InsertConfermaStrutturaleArco(ctx, db.InsertConfermaStrutturaleArcoParams{ConfermaID: sc.conferma, PadreID: sc.mappa[x.padre],
			FiglioID: sc.mappa[x.figlio], Qta: int32(x.qta), Posizione: testoNullo(x.posizione), Origine: pgtype.Text{String: x.origine, Valid: true}}); err != nil {
			return err
		}
	}
	for _, d := range sc.snap.Decisioni {
		if d.Tipo == DecisioneIdentita && d.Modo == ModoCorrezione {
			sc.out.IdentitaCorrette = append(sc.out.IdentitaCorrette, d.ChiaveBozza)
		}
	}
	sort.Strings(sc.out.IdentitaCorrette)
	// una riga di conferma_strutturale_non_incluso per ogni decisione non_includere (EM §7.5 punto 4), con le righe e gli
	// archi legacy che i segni chiuderanno
	for _, d := range sc.nonIncluse() {
		sha, ch := shaEChiave(d.Casella.Rif)
		righe, archi := sc.daChiudereNI(sha, ch)
		ids := make([]uuid.UUID, 0, len(righe))
		for _, r := range righe {
			ids = append(ids, r.PropostaID)
		}
		aa := make([]map[string]any, 0, len(archi))
		for _, a := range archi {
			aa = append(aa, map[string]any{"allegato_id": a.AllegatoID, "padre_chiave": a.PadreChiave, "figlio_chiave": a.FiglioChiave})
		}
		var segnale *json.RawMessage
		if d.SegnaleVisto != nil {
			b, err := json.Marshal(d.SegnaleVisto)
			if err != nil {
				return err
			}
			r := json.RawMessage(b)
			segnale = &r
		}
		archiJSON, err := json.Marshal(aa)
		if err != nil {
			return err
		}
		if err := q.InsertConfermaNonIncluso(ctx, db.InsertConfermaNonInclusoParams{ConfermaID: sc.conferma, ThreadID: sc.thread, Sha256: sha, Chiave: ch,
			AllegatoID: d.Casella.AllegatoID, Gruppo: d.Casella.Gruppo, Origine: DecisioneNonIncludere, DecisioneID: pgtype.Text{String: d.ID, Valid: true},
			AzioneID: uuid.NullUUID{UUID: derefUUID(d.AzioneID), Valid: d.AzioneID != nil}, MotivoCodice: testoNullo(d.MotivoCodice), Motivo: testoNullo(d.Motivo),
			Segnale: segnale, RigheChiuse: ids, ArchiChiusi: archiJSON}); err != nil {
			return err
		}
		sc.out.NonInclusiApplicati++
	}
	if err := sc.tracce(); err != nil {
		return err
	}
	return sc.passo("certificati")
}

// tracce: le righe di conferma_strutturale_non_incluso delle esclusioni senza una decisione non_includere propria (EM §7.5
// con U2 §6.1, U2-P28; il controllo (iv) della 0023): assenza (le voci della proposta assenti dalla bozza, con le righe e
// gli archi che i segni chiuderanno), escludi (un registrato escluso, per ogni casella del suo nodo), legacy (tolta nelle
// pagine di prima) ed ereditata (dalla conferma di base), queste due solo come traccia. Una riga per (sha256, chiave): una
// casella gia' scritta (prima le non_includere, poi nell'ordine di esclusi[] di R4) non si ripete. La decisione si cita
// solo se e' dello snapshot (escludi); una ereditata cita la decisione di un'altra conferma, che resta nella sua traccia
// [T-39]. Il motivo che manca resta NULL (si dichiara, non si inventa: [UR] Q13).
func (sc *scrittureR5) tracce() error {
	scritte := map[string]bool{}
	for _, d := range sc.nonIncluse() {
		sha, ch := shaEChiave(d.Casella.Rif)
		scritte[sha+"|"+ch] = true
	}
	scrivi := func(origine string, c Casella3, decisione, motivoCodice, motivo string, segnale *importazione.SegnaleVisto, chiudere bool) error {
		sha, ch := shaEChiave(c.Rif)
		if sha == "" || scritte[sha+"|"+ch] {
			return nil
		}
		scritte[sha+"|"+ch] = true
		ids := []uuid.UUID{}
		aa := []map[string]any{}
		if chiudere {
			righe, archi := sc.daChiudereNI(sha, ch)
			for _, r := range righe {
				ids = append(ids, r.PropostaID)
			}
			for _, a := range archi {
				aa = append(aa, map[string]any{"allegato_id": a.AllegatoID, "padre_chiave": a.PadreChiave, "figlio_chiave": a.FiglioChiave})
			}
		}
		archiJSON, err := json.Marshal(aa)
		if err != nil {
			return err
		}
		var seg *json.RawMessage
		if segnale != nil {
			b, err := json.Marshal(segnale)
			if err != nil {
				return err
			}
			r := json.RawMessage(b)
			seg = &r
		}
		motivo = strings.TrimSpace(motivo)
		if len([]rune(motivo)) > 1000 {
			motivo = string([]rune(motivo)[:1000])
		}
		switch motivoCodice {
		case "elemento_cad", "duplicato_cad", "non_articolo", "altro":
		default:
			motivoCodice = ""
		}
		return sc.q.InsertConfermaNonIncluso(sc.ctx, db.InsertConfermaNonInclusoParams{ConfermaID: sc.conferma, ThreadID: sc.thread, Sha256: sha,
			Chiave: ch, AllegatoID: c.AllegatoID, Gruppo: c.Gruppo, Origine: origine, DecisioneID: testoNullo(decisione),
			MotivoCodice: testoNullo(motivoCodice), Motivo: testoNullo(motivo), Segnale: seg, RigheChiuse: ids, ArchiChiusi: archiJSON})
	}
	for _, e := range sc.rp.Esclusi {
		switch e.Origine {
		case OrigineEsclusoAssenza, OrigineEsclusoLegacy, OrigineEsclusoEreditata:
			if e.Casella == nil {
				continue
			}
			if err := scrivi(e.Origine, *e.Casella, "", e.MotivoCodice, e.Motivo, e.SegnaleRegistro, e.Origine == OrigineEsclusoAssenza); err != nil {
				return err
			}
			if e.Origine == OrigineEsclusoAssenza {
				sc.out.AssenzeApplicate++
			}
		case DecisioneEscludi:
			if e.ComponenteID == nil {
				continue
			}
			for _, k := range chiaviNodi(sc.nodi) {
				n := sc.nodi[k]
				if n.ComponenteID == nil || *n.ComponenteID != *e.ComponenteID {
					continue
				}
				for _, c := range n.Caselle {
					if err := scrivi(DecisioneEscludi, c, e.ID, "", e.Motivo, nil, false); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// chiaviNodi: le chiavi dei nodi in ordine.
func chiaviNodi(m map[string]Nodo3) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func derefUUID(u *uuid.UUID) uuid.UUID {
	if u == nil {
		return uuid.Nil
	}
	return *u
}

// nonIncluse: le decisioni non_includere dello snapshot, per id.
func (sc *scrittureR5) nonIncluse() []Decisione3 {
	var out []Decisione3
	for _, d := range sc.snap.Decisioni {
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// decidibile: una riga di nodo che DecidiNodoNellAlbero tocca senza anche_decise (aperta, o l'aggancio per codice di prima).
func decidibile(r db.ListRighePerShaChiaveRow) bool {
	return r.Stato == db.StatoPropostaAperta || (r.Stato == db.StatoPropostaDuplicato && !r.DecisoDa.Valid && r.ComponenteID.Valid)
}

// daChiudereNI: le righe e gli archi legacy che la non_includere di (sha, chiave) chiude (letti in T sotto il lucchetto).
func (sc *scrittureR5) daChiudereNI(sha, ch string) ([]db.ListRighePerShaChiaveRow, []db.ListArchiPerShaChiaveRow) {
	var righe []db.ListRighePerShaChiaveRow
	for _, r := range sc.p.righe[sha+"|"+ch] {
		if decidibile(r) {
			righe = append(righe, r)
		}
	}
	var archi []db.ListArchiPerShaChiaveRow
	for _, a := range sc.p.archiEntranti[sha+"|"+ch] {
		if a.Stato == db.StatoPropostaAperta {
			archi = append(archi, a)
		}
	}
	return righe, archi
}

// ------------------------------------------------------------------ 5. le AR, dal servizio unico

func (sc *scrittureR5) associazioni() error {
	ctx, q := sc.ctx, sc.q
	atto := associazioni.Atto{Thread: sc.thread, Autore: sc.utente, Chiave: sc.chiave, Conferma: uuid.NullUUID{UUID: sc.conferma, Valid: true},
		Nota: fmt.Sprintf("conferma n. %d", sc.numero)}
	pb := map[uuid.UUID]Preassociazione3{}
	for _, x := range sc.snap.DocumentiPreassociati {
		pb[x.AllegatoID] = x
	}
	registri := map[string]uuid.NullUUID{}
	registro := func(impronta string) (uuid.NullUUID, error) {
		if impronta == "" {
			return uuid.NullUUID{}, nil
		}
		if r, ok := registri[impronta]; ok {
			return r, nil
		}
		rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: sc.thread, ProdottoID: sc.snap.ProdottoID, Impronte: []string{impronta}})
		if err != nil {
			return uuid.NullUUID{}, err
		}
		if len(rr) != 1 {
			return uuid.NullUUID{}, fuori("documenti_preassociati.importazione", "il registro %s della PB non è del prodotto", impronta)
		}
		registri[impronta] = uuid.NullUUID{UUID: rr[0].ImportazioneID, Valid: true}
		return registri[impronta], nil
	}
	rifiuto := func(e associazioni.EsitoPB, err error) error {
		var rf *associazioni.Rifiuto
		switch {
		case errors.As(err, &rf) && rf.Codice == associazioni.CodiceVincolo:
			// il database ha fermato la scrittura della AR: la transazione e' annullata, niente e' registrato
			return &RifiutoR5{Err: &Errore{Codice: CodiceControlloFinale, Motivo: rf.Codice,
				Messaggio: "il controllo delle associazioni non è passato: niente è stato registrato"}, NonRegistrato: true}
		case errors.As(err, &rf):
			a := e.Allegato
			return rifiutoBlocchi([]Rilievo{{Codice: BloccoPreassociazioneNonApplicab, Motivo: rf.Codice, AllegatoID: &a, ChiaveBozza: e.ChiaveBozza,
				Testo: "La preassociazione non si applica più: " + rf.Messaggio}})
		}
		return err
	}
	// le ricodifiche (D-ASS-01 «Mantieni» [U]; EM H.5): la AR resta sulla stessa riga, la copia prende il codice nuovo e
	// correzione_documento (ricodifica_bom) registra codice prima e dopo, il motivo della decisione e chi conferma
	for _, d := range sc.p.ricodifiche {
		if _, err := associazioni.AllineaCodiceCopia(ctx, q, sc.thread, *d.ComponenteID); err != nil {
			return err
		}
		rr, err := q.ListAttiveDelComponenteR5(ctx, db.ListAttiveDelComponenteR5Params{ThreadID: sc.thread, ComponenteID: *d.ComponenteID})
		if err != nil {
			return err
		}
		for _, r := range rr {
			prop := uuid.NullUUID{UUID: r.PropostaID, Valid: r.PropostaID != uuid.Nil}
			if err := q.InsertCorrezioneRicodificaR5(ctx, db.InsertCorrezioneRicodificaR5Params{ThreadID: sc.thread, PropostaID: prop,
				ComponenteID: uuid.NullUUID{UUID: *d.ComponenteID, Valid: true}, AllegatoID: r.AllegatoID, NomeFile: r.NomeFile, Sha256: r.Sha256,
				CodiceLetto: testoNullo(r.CodiceLetto), RevLetta: r.RevLetta, CodicePrima: testoNullo(d.CodicePrima), CodiceDopo: testoNullo(d.CodiceDopo),
				Motivo: testoNullo(d.Motivo), ChiaveIdempotenza: uuid.NullUUID{UUID: sc.chiave, Valid: true}, AutoreID: sc.utente,
				AssociazioneID: uuid.NullUUID{UUID: r.AssociazioneID, Valid: true}}); err != nil {
				return err
			}
			sc.out.CorrezioniRicodifica++
		}
	}
	for _, e := range sc.rp.esitiPiano.Preassociazioni {
		x := pb[e.Allegato]
		switch e.Esito {
		case associazioni.EsitoNonRichiesta:
			continue
		case associazioni.EsitoGiaApplicata:
			sc.out.AssociazioniGiaApplicate++
			continue
		case associazioni.EsitoApplicabile, associazioni.EsitoSostituzione:
		default:
			a := e.Allegato
			return rifiutoBlocchi([]Rilievo{{Codice: BloccoPreassociazioneNonApplicab, Motivo: e.Motivo, AllegatoID: &a, ChiaveBozza: e.ChiaveBozza,
				Testo: "La preassociazione non si applica."}})
		}
		imp, err := registro(x.Importazione)
		if err != nil {
			return err
		}
		reg := associazioni.Registrazione{Allegato: e.Allegato, Componente: sc.mappa[e.ChiaveBozza], ChiaveBozza: e.ChiaveBozza,
			Provenienza: x.Provenienza, Importazione: imp}
		var es associazioni.Esito
		if e.Esito == associazioni.EsitoSostituzione {
			es, err = associazioni.Sostituisci(ctx, q, atto, reg)
		} else {
			es, err = associazioni.Registra(ctx, q, atto, reg)
		}
		if err != nil {
			return rifiuto(e, err)
		}
		switch {
		case es.Invariata:
			sc.out.AssociazioniGiaApplicate++
		default:
			sc.out.AssociazioniApplicate++
		}
	}
	for _, e := range sc.rp.esitiPiano.Svincoli {
		switch e.Esito {
		case associazioni.EsitoApplicabile:
		case associazioni.EsitoSvincoloGiaChiuso:
			sc.out.SvincoliGiaChiusi++
			continue
		default:
			a := e.Allegato
			return rifiutoBlocchi([]Rilievo{{Codice: BloccoPreassociazioneNonApplicab, Motivo: e.Motivo, AllegatoID: &a, Testo: "Lo svincolo non si applica."}})
		}
		ch, err := associazioni.Scollega(ctx, q, atto, associazioni.Scollegamento{Allegato: e.Allegato, Componente: e.Componente,
			Motivo: associazioni.MotivoSvincoloConferma})
		if err != nil {
			return rifiuto(associazioni.EsitoPB{Allegato: e.Allegato}, err)
		}
		switch {
		case ch.Esito == associazioni.EsitoChiusa:
			sc.out.SvincoliApplicati++
		case ch.Esito == associazioni.EsitoSvincoloGiaChiuso && ch.Associazione != nil && ch.Associazione.ChiusaChiave.Valid &&
			ch.Associazione.ChiusaChiave.UUID == sc.chiave:
			sc.out.SvincoliApplicati++ // chiusa da una sostituzione della stessa conferma
		default:
			sc.out.SvincoliGiaChiusi++
		}
	}
	return sc.passo("associazioni")
}

// ------------------------------------------------------------------ 6. i segni legacy (copia derivata)

func (sc *scrittureR5) segno(extra map[string]any) json.RawMessage {
	s := map[string]any{"da": sc.utente, "il": sc.il, "firma": sc.rp.FirmaAnteprima, "formato": FormatoSnapshot3, "gesto": "conferma_bom",
		"prodotto_id": sc.snap.ProdottoID, "conferma_id": sc.conferma, "chiave_idempotenza": sc.chiave}
	for k, v := range extra {
		s[k] = v
	}
	b, _ := jsoncanonico.Codifica(s)
	return b
}

func (sc *scrittureR5) segni() error {
	ctx, q := sc.ctx, sc.q
	casella := map[uuid.UUID]bool{}
	// le righe dei nodi tenuti: la prima riga di un pezzo nuovo e' «confermata», le altre (anche dei suoi alias, e quelle di
	// un componente che c'era) «duplicato», con il componente della mappa e il segno
	confermato := map[uuid.UUID]bool{}
	for _, k := range sc.p.inclusi {
		n := sc.nodi[k]
		stato := db.StatoPropostaDuplicato
		if sc.creati[k] && !confermato[sc.mappa[k]] {
			stato = db.StatoPropostaConfermata
		}
		for _, c := range n.Caselle {
			sha, ch := shaEChiave(c.Rif)
			for _, r := range sc.p.righe[sha+"|"+ch] {
				casella[r.PropostaID] = true
				if !decidibile(r) {
					continue
				}
				m, err := q.DecidiNodoNellAlbero(ctx, db.DecidiNodoNellAlberoParams{Stato: stato, ComponenteID: uuid.NullUUID{UUID: sc.mappa[k], Valid: true},
					DecisoDa: sc.utente, Segno: sc.segno(map[string]any{"decisione": "tieni", "rif": c.Rif, "nodo": k}), PropostaID: r.PropostaID})
				if err != nil {
					return err
				}
				if m > 0 {
					if stato == db.StatoPropostaConfermata {
						confermato[sc.mappa[k]] = true
					}
					stato = db.StatoPropostaDuplicato
				}
			}
		}
	}
	// gli archi STEP tenuti
	archiTenuti := map[string]bool{}
	var chiavi []string
	tenuti := map[string]arcoR5{}
	for _, x := range sc.p.voluti {
		if a, ok := sc.p.archoStep(sc.nodi, x); ok {
			k := a.Allegato.String() + "|" + a.Padre + "|" + a.Figlio
			archiTenuti[k] = true
			chiavi = append(chiavi, k)
			tenuti[k] = x
		}
	}
	if len(chiavi) > 0 {
		rr, err := q.ListRelazioniPropostaR5(ctx, db.ListRelazioniPropostaR5Params{ThreadID: sc.thread, Chiavi: chiavi})
		if err != nil {
			return err
		}
		for _, r := range rr {
			if r.Stato != db.StatoPropostaAperta {
				continue
			}
			x := tenuti[r.AllegatoID.String()+"|"+r.PadreChiave+"|"+r.FiglioChiave]
			pid, fid := sc.mappa[x.padre], sc.mappa[x.figlio]
			stato := db.StatoPropostaDuplicato
			if sc.scritti[[2]uuid.UUID{pid, fid}] {
				stato = db.StatoPropostaConfermata
			}
			if _, err := q.DecidiArcoNellAlbero(ctx, db.DecidiArcoNellAlberoParams{Stato: stato, DecisoDa: sc.utente,
				Segno:    sc.segno(map[string]any{"decisione": "tieni", "padre": pid, "figlio": fid}),
				ThreadID: sc.thread, AllegatoID: r.AllegatoID, PadreChiave: r.PadreChiave, FiglioChiave: r.FiglioChiave}); err != nil {
				return err
			}
		}
	}
	// le non_includere: le righe del nodo e gli archi che entrano, chiusi con la nota fissa (EM §7.5 punti 1 e 2)
	for _, d := range sc.nonIncluse() {
		sha, ch := shaEChiave(d.Casella.Rif)
		righe, archi := sc.daChiudereNI(sha, ch)
		segno := sc.segno(map[string]any{"decisione": DecisioneNonIncludere, "decisione_id": d.ID, "azione_id": d.AzioneID, "rif": d.Casella.Rif,
			"motivo_codice": d.MotivoCodice, "segnale_visto": d.SegnaleVisto})
		for _, r := range righe {
			m, err := q.DecidiNodoNellAlbero(ctx, db.DecidiNodoNellAlberoParams{Stato: db.StatoPropostaScartata, DecisoDa: sc.utente,
				Nota: pgtype.Text{String: NotaNonInclusoR5, Valid: true}, Segno: segno, PropostaID: r.PropostaID})
			if err != nil {
				return err
			}
			if m != 1 {
				return rifiutoRiprova("una riga della proposta è cambiata mentre confermavi: riprova")
			}
		}
		for _, a := range archi {
			m, err := q.DecidiArcoNellAlbero(ctx, db.DecidiArcoNellAlberoParams{Stato: db.StatoPropostaScartata, Nota: pgtype.Text{String: NotaNonInclusoR5, Valid: true},
				DecisoDa: sc.utente, Segno: segno, ThreadID: sc.thread, AllegatoID: a.AllegatoID, PadreChiave: a.PadreChiave, FiglioChiave: a.FiglioChiave})
			if err != nil {
				return err
			}
			if m != 1 {
				return rifiutoRiprova("un arco della proposta è cambiato mentre confermavi: riprova")
			}
		}
	}
	// le voci assenti dalla bozza: le righe e gli archi che entrano, chiusi come per una non_includere, con la nota fissa
	// dell'assenza (EM §7.5 con U2 §6.1; dichiarati in R4, esclusi[].effetti)
	for _, c := range sc.p.assenti {
		sha, ch := shaEChiave(c.Rif)
		righe, archi := sc.daChiudereNI(sha, ch)
		nota := pgtype.Text{String: notaAssenzaR5(sc.numero), Valid: true}
		segno := sc.segno(map[string]any{"decisione": OrigineEsclusoAssenza, "rif": c.Rif})
		for _, r := range righe {
			m, err := q.DecidiNodoNellAlbero(ctx, db.DecidiNodoNellAlberoParams{Stato: db.StatoPropostaScartata, DecisoDa: sc.utente,
				Nota: nota, Segno: segno, PropostaID: r.PropostaID})
			if err != nil {
				return err
			}
			if m != 1 {
				return rifiutoRiprova("una riga della proposta è cambiata mentre confermavi: riprova")
			}
		}
		for _, a := range archi {
			m, err := q.DecidiArcoNellAlbero(ctx, db.DecidiArcoNellAlberoParams{Stato: db.StatoPropostaScartata, Nota: nota,
				DecisoDa: sc.utente, Segno: segno, ThreadID: sc.thread, AllegatoID: a.AllegatoID, PadreChiave: a.PadreChiave, FiglioChiave: a.FiglioChiave})
			if err != nil {
				return err
			}
			if m != 1 {
				return rifiutoRiprova("un arco della proposta è cambiato mentre confermavi: riprova")
			}
		}
	}
	// lo STEP A sostituito da B: le sue righe ancora aperte (tranne quelle della bozza confermata) e le sue rimozioni,
	// chiuse con la nota «superata da ‹B› (conferma n)» (R39-13; come faceva la sostituzione legacy)
	if p := sc.p; p.sostituzione && p.shaA != "" && p.shaA != p.shaB {
		nome := p.shaB
		if d, err := q.GetDocumento(ctx, p.stepB); err == nil {
			nome = d.NomeFile
		}
		nota := pgtype.Text{String: tagliaNotaR5(fmt.Sprintf("superata da %s (conferma n. %d)", nome, sc.numero)), Valid: true}
		escluse := make([]uuid.UUID, 0, len(casella))
		for id := range casella {
			escluse = append(escluse, id)
		}
		var esclusiArchi []string
		for k := range archiTenuti {
			esclusiArchi = append(esclusiArchi, k)
		}
		if _, err := q.ChiudiProposteDelloShaR5(ctx, db.ChiudiProposteDelloShaR5Params{Nota: nota, ThreadID: sc.thread, Sha256: p.shaA, Escluse: escluse}); err != nil {
			return err
		}
		if _, err := q.ChiudiRelazioniDelloShaR5(ctx, db.ChiudiRelazioniDelloShaR5Params{Nota: nota, Sha256: pgtype.Text{String: p.shaA, Valid: true}, ThreadID: sc.thread, Escluse: esclusiArchi}); err != nil {
			return err
		}
		if _, err := q.ChiudiRimozioniDiUnoStep(ctx, db.ChiudiRimozioniDiUnoStepParams{Nota: nota, ThreadID: sc.thread, StepDocumentoID: p.stepA.UUID}); err != nil {
			return err
		}
	}
	return sc.passo("segni")
}

// tagliaNotaR5: la nota entro 500 caratteri.
func tagliaNotaR5(s string) string {
	r := []rune(s)
	if len(r) > 500 {
		return string(r[:500])
	}
	return s
}

// ------------------------------------------------------------------ 7. la chiusura della bozza, l'intento, f8_stato

func (sc *scrittureR5) chiusura() error {
	ctx, q := sc.ctx, sc.q
	n, err := q.ConfermaDistintaBozza(ctx, db.ConfermaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: sc.utente, Valid: true},
		BozzaID: sc.versione.BozzaID, ThreadID: sc.thread, Versione: sc.versione.Versione})
	if err != nil {
		return err
	}
	if n != 1 {
		return rifiutoRiprova("la bozza è cambiata mentre confermavi: riprova")
	}
	codice := sc.snap.ProdottoID.String()
	if c, ok := sc.st.Componente(sc.snap.ProdottoID); ok {
		codice = c.CodiceLetterale
	}
	sc.out.F8Stato = "in_attesa"
	sc.out.Testo = fmt.Sprintf("Conferma n. %d di %s: %d componenti nuovi, %d legami nuovi, %d legami tolti, %d associazioni registrate.",
		sc.numero, codice, len(sc.out.IdentitaCreate), len(sc.out.ArchiAggiunti), len(sc.out.ArchiTolti), sc.out.AssociazioniApplicate)
	if sc.out.Fonte.Promossa {
		sc.out.Testo += " Lo STEP sorgente della bozza è ora lo STEP strutturale del prodotto."
	}
	esito, err := json.Marshal(sc.out)
	if err != nil {
		return err
	}
	if _, err := q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: sc.chiave, ThreadID: sc.thread, Gesto: GestoConfermaBomV3, UtenteID: sc.utente,
		ImprontaCorpo: sc.richiesta.ImprontaCorpo, Esito: esito, ConfermaID: uuid.NullUUID{UUID: sc.conferma, Valid: true}}); err != nil {
		return err
	}
	if _, err := q.InsertF8StatoConferma(ctx, db.InsertF8StatoConfermaParams{ThreadID: sc.thread, ChiaveIntento: uuid.NullUUID{UUID: sc.chiave, Valid: true},
		ConfermaID: uuid.NullUUID{UUID: sc.conferma, Valid: true}}); err != nil {
		return err
	}
	return sc.passo("chiusura")
}

// testoNullo: un testo vuoto e' NULL.
func testoNullo(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
