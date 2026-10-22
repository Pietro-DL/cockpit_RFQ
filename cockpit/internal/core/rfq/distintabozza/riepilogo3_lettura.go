package distintabozza

// riepilogo3_lettura.go: le letture di R4 formato 3 (riepilogo3.go), tutte nella transazione di chi chiama, solo SELECT, e
// la chiamata della parte pura (riepilogo3_regole.go). R5 le rifara' nella connessione S dopo il lucchetto della RFQ.

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// ingressiR4: tutto quello che la parte pura del riepilogo guarda, letto da Riepilogo3 (le prove L1 lo costruiscono a mano).
type ingressiR4 struct {
	thread, prodotto uuid.UUID
	utente           uuid.UUID
	avvisiMotore     bool
	richiesta        RichiestaR4
	// snap: lo snapshot del corpo con la provenienza delle PB decisa dal server
	snap     Snapshot3
	canonico []byte
	// versione salvata usata per la provenienza e il giornale (nil senza bozza)
	versione *int
	eventi   []EventoLetto

	stato     *confermabom.StatoRFQ
	cert      map[uuid.UUID]Certificazione
	cliente   string
	vc        confermabom.VersioneCorrente
	base      StatoBase
	scaduti   []RifScaduto
	registro  *importazione.Registro
	sorgente  SorgenteR4
	nomeFonte string
	congelata bool

	fatti      associazioni.FattiPiano
	altreBozze []BozzaDellaRfq
	scrittiNAS map[uuid.UUID]int
	// confermati: i documenti confermati (in ogni stato del NAS) dei componenti ricodificati (RICODIFICA_FILE_NAS, D-ASS-01)
	confermati map[uuid.UUID]int
	// dichiarati: i componenti con un'autorizzazione STEP del Fascicolo (valida, sospesa o da sistemare): il cambio di tipo
	// da o verso commerciale e il ripristino di un archiviato la toccano (blocco «R5 completo»)
	dichiarati  map[uuid.UUID]bool
	effettiTipo map[string]string // componente|tipo → firma dell'effetto di adesso
	righeNI     map[string][]db.ListRighePerShaChiaveRow
	stessoStep  map[string][]ProdottoStessoStep
	struttura   *db.GetStrutturaDellaSorgenteRow
	stepProd    []db.ListStepDelProdottoRow
	shaS8       map[string]bool
	confBase    *confermaBaseR4
	inAnalisi   []FileInAnalisi
	// motore: le regole del cliente della RFQ (il canonico, CANONICO_*; riepilogo3_canonico.go); nil = nessuna regola
	motore *classificazione.Motore

	// il completamento (riepilogo3_completamento.go)
	righeSha       map[string][]db.ListRigheStrutturaR4Row  // sha → righe legacy (lo STEP sorgente, gli STEP secondari)
	archiNI        map[string][]db.ListArchiPerShaChiaveRow // «sha|chiave» → archi che entrano nei non inclusi
	rimozioni      []db.RimozioneProposta                   // le rimozioni proposte aperte della RFQ
	secondari      []stepSecondario                         // gli STEP secondari autorizzati (dichiarazioni valide)
	nonInclusiBase []db.ListNonInclusiConfermaRow           // i non inclusi della conferma di base (esclusioni ereditate)
	ingressi       *Ingressi                                // fuori firma, solo con OpzioniR4.ConIngressi
	esitoMotore    *EsitoMotoreR4                           // il calcolo del motore (solo con avvisiMotore)
}

// confermaBaseR4: la conferma di base della bozza, per contenuto-atto-1 (uguale_alla_base).
type confermaBaseR4 struct {
	formato   int
	snap      *Snapshot3
	mappa     map[string]uuid.UUID
	criterio  string
	radiceSha string
	monoPezzo bool
}

// Riepilogo3 calcola R4 formato 3 (commento di testa di riepilogo3.go). Errori: RFQ_INESISTENTE (404), FUORI_RFQ (il
// prodotto non e' un finito della RFQ, la bozza o un riferimento non sono della RFQ), CORPO_NON_VALIDO (la versione indicata
// non c'e', la bozza e' formato 2).
func Riepilogo3(ctx context.Context, q *db.Queries, thread uuid.UUID, r RichiestaR4, op OpzioniR4) (RiepilogoR4, error) {
	in, err := leggiIngressiR4(ctx, q, thread, r, op)
	if err != nil {
		return RiepilogoR4{}, err
	}
	return calcolaR4(in)
}

func leggiIngressiR4(ctx context.Context, q *db.Queries, thread uuid.UUID, r RichiestaR4, op OpzioniR4) (*ingressiR4, error) {
	s := r.Snapshot
	in := &ingressiR4{thread: thread, prodotto: s.ProdottoID, utente: op.UtenteID, avvisiMotore: op.AvvisiMotore, esitoMotore: op.Motore, richiesta: r,
		cert: map[uuid.UUID]Certificazione{}, scrittiNAS: map[uuid.UUID]int{}, confermati: map[uuid.UUID]int{}, dichiarati: map[uuid.UUID]bool{},
		effettiTipo: map[string]string{},
		righeNI:     map[string][]db.ListRighePerShaChiaveRow{}, stessoStep: map[string][]ProdottoStessoStep{}, shaS8: map[string]bool{},
		righeSha: map[string][]db.ListRigheStrutturaR4Row{}, archiNI: map[string][]db.ListArchiPerShaChiaveRow{}}
	if _, err := q.GetThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'è"}
		}
		return nil, err
	}
	if _, err := prodottoDellaRfq(ctx, q, thread, s.ProdottoID, false); err != nil {
		return nil, err
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	in.stato = st
	if in.vc, err = st.VersioneCorrente(s.ProdottoID); err != nil {
		return nil, err
	}
	if err := riferimentiDellaRfq(ctx, q, thread, s, st); err != nil {
		return nil, err
	}
	// la versione salvata (provenienza e giornale)
	var prima *Snapshot3
	if r.BozzaID != nil {
		b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: *r.BozzaID, ThreadID: thread})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fuori("bozza_id", "la bozza %s non è di questa RFQ", *r.BozzaID)
		}
		if err != nil {
			return nil, err
		}
		if b.ProdottoID != s.ProdottoID {
			return nil, fuori("bozza_id", "la bozza %s non è del prodotto %s", *r.BozzaID, s.ProdottoID)
		}
		if b.Formato != FormatoSnapshot3 {
			return nil, corpoMotivo("bozza_id", MotivoFormatoDaConvertire, "la bozza %s è formato %d: si converte con converti_da (R2)", *r.BozzaID, b.Formato)
		}
		ver := int(b.Versione)
		if r.BozzaVersione > 0 {
			ver = r.BozzaVersione
		}
		v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: thread, Versione: int32(ver)})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, corpo("bozza_versione", "la bozza %s non ha la versione %d", b.BozzaID, ver)
		}
		if err != nil {
			return nil, err
		}
		letto, err := RileggiVersione(int(b.Formato), v.Snapshot, v.Impronta)
		if err != nil {
			return nil, err
		}
		if letto.Formato3 != nil {
			x := *letto.Formato3
			prima = &x
		}
		ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: b.BozzaID, ThreadID: thread,
			VersioneMax: int32(ver), DopoN: 0, Limite: math.MaxInt32})
		if err != nil {
			return nil, err
		}
		for _, e := range ee {
			in.eventi = append(in.eventi, eventoLetto(e))
		}
		in.versione = &ver
	}
	// la provenienza delle PB, con la regola di R2 (con il registro del calcolo di adesso se R4 ha il motore: adesso3.go)
	var adesso func() *importazione.Registro
	if op.Motore != nil && op.Motore.Ingresso != nil && r.BozzaID != nil {
		bozza, fatto := *r.BozzaID, false
		var reg *importazione.Registro
		adesso = func() *importazione.Registro {
			if !fatto {
				fatto = true
				// la sorgente della bozza salvata, come R2 (calcoloAdessoV1); un errore qui non e' un errore di R4: senza il
				// registro di adesso la PB si vede come senza il calcolo
				if src, ok, err := SorgenteDellaBozza(ctx, q, thread, s.ProdottoID, bozza); err == nil && ok {
					if c, err := CalcoloAdessoDa(op.Motore.Ingresso, &src); err == nil {
						reg = c.registro()
					}
				}
			}
			return reg
		}
	}
	if s, err = provenienzeR4(ctx, q, thread, s, prima, adesso); err != nil {
		return nil, err
	}
	in.snap = s
	if in.canonico, err = s.Canonico(); err != nil {
		return nil, err
	}
	cid, err := q.GetClienteDellaRfq(ctx, thread)
	if err != nil {
		return nil, err
	}
	in.cliente = EmittenteCliente(cid)
	if in.motore, err = fascicolo.MotoreDellaRfq(ctx, q, thread); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(st.Componenti))
	for _, c := range st.Componenti {
		ids = append(ids, c.ID)
	}
	if len(ids) > 0 {
		rr, err := q.ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: thread, Componenti: ids})
		if err != nil {
			return nil, err
		}
		in.cert = CertificazioniDa(rr)
	}
	if in.scaduti, err = RifScaduti(ctx, q, thread, s); err != nil {
		return nil, err
	}
	if in.base, err = BaseDellaBozza(ctx, q, thread, s, st, in.scaduti); err != nil {
		return nil, err
	}
	// il registro della versione
	if s.Sorgente != nil && s.Sorgente.Importazione != "" {
		rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: thread, ProdottoID: s.ProdottoID,
			Impronte: []string{s.Sorgente.Importazione}})
		if err != nil {
			return nil, err
		}
		if len(rr) == 0 {
			return nil, fuori("sorgente.importazione", "il registro %s non è del prodotto in questa RFQ", s.Sorgente.Importazione)
		}
		reg, _, err := RileggiImportazione(db.GetDistintaImportazioneRow(rr[0]))
		if err != nil {
			return nil, err
		}
		in.registro = &reg
	}
	// la sorgente e il gate (EM App. I.2)
	if err := in.leggiSorgente(ctx, q); err != nil {
		return nil, err
	}
	// la BOM congelata (D-AR08 A [T-39]; C3 §4.4 n. 20): l'ultima versione della RFQ e' congelata (bom_working_bloccata
	// senza il FOR KEY SHARE, che una transazione in sola lettura non prende)
	if v, err := q.GetUltimaVersione(ctx, thread); err == nil {
		in.congelata = v.Stato == db.StatoBomCongelata
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// i fatti del Piano
	allegati := []uuid.UUID{}
	for _, p := range s.DocumentiPreassociati {
		allegati = append(allegati, p.AllegatoID)
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneScollegaDocumento && d.AllegatoID != nil {
			allegati = append(allegati, *d.AllegatoID)
		}
	}
	if in.fatti, err = associazioni.LeggiFattiPiano(ctx, q, thread, allegati); err != nil {
		return nil, err
	}
	in.completaFattiPiano()
	if err := in.stepCandidatiNellePB(ctx, q); err != nil {
		return nil, err
	}
	// le bozze aperte degli altri prodotti (FILE_CONTESO_FRA_PRODOTTI)
	tutte, err := LeggiBozzeDellaRfq(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	for _, b := range tutte.Bozze {
		if b.ProdottoID != s.ProdottoID {
			in.altreBozze = append(in.altreBozze, b)
		}
	}
	// le decisioni che vogliono il database: ricodifiche (RICODIFICA_FILE_NAS), cambi di tipo (TIPO_EFFETTO_CAMBIATO), non
	// inclusi (NON_INCLUSO_NON_APPLICABILE, prodotti con lo stesso STEP)
	var ricodificati []uuid.UUID
	var shas, chiavi []string
	for _, d := range s.Decisioni {
		switch d.Tipo {
		case DecisioneRicodifica:
			ricodificati = append(ricodificati, *d.ComponenteID)
		case DecisioneCambiaTipo:
			k := d.ComponenteID.String() + "|" + d.TipoNuovo
			if _, ok := in.effettiTipo[k]; ok {
				continue
			}
			t := db.TipoComponente(d.TipoNuovo)
			if !t.Valid() {
				continue
			}
			e, err := fascicolo.EffettoCambioTipo(ctx, q, thread, *d.ComponenteID, t)
			if err != nil {
				var rf fascicolo.Rifiuto
				if errors.As(err, &rf) {
					in.effettiTipo[k] = ""
					continue
				}
				return nil, err
			}
			in.effettiTipo[k] = e.Firma
		case DecisioneNonIncludere:
			sha, ch := shaEChiave(d.Casella.Rif)
			if sha != "" {
				shas, chiavi = append(shas, sha), append(chiavi, ch)
			}
		}
	}
	// le voci assenti dalla bozza: R5 ne chiude le righe come per i non inclusi (EM §7.8 con U2 §6.1), e R4 le dichiara
	for _, c := range caselleAssenti(s, in.registro) {
		if sha, ch := shaEChiave(c.Rif); sha != "" {
			shas, chiavi = append(shas, sha), append(chiavi, ch)
		}
	}
	if len(ricodificati) > 0 {
		rr, err := q.ListDocumentiDeiComponentiR4(ctx, db.ListDocumentiDeiComponentiR4Params{ThreadID: thread, Componenti: ricodificati})
		if err != nil {
			return nil, err
		}
		for _, x := range rr {
			in.confermati[x.ComponenteID]++
			if x.StatoNas == "scritto" {
				in.scrittiNAS[x.ComponenteID]++
			}
		}
	}
	if len(shas) > 0 {
		rr, err := q.ListRighePerShaChiave(ctx, db.ListRighePerShaChiaveParams{ThreadID: thread, Shas: shas, Chiavi: chiavi})
		if err != nil {
			return nil, err
		}
		for _, x := range rr {
			k := strings.TrimSpace(x.Sha256) + "|" + x.Chiave
			in.righeNI[k] = append(in.righeNI[k], x)
		}
		visti := map[string]bool{}
		for _, sha := range shas {
			if visti[sha] {
				continue
			}
			visti[sha] = true
			pp, err := q.ListProdottiConLoStessoStep(ctx, db.ListProdottiConLoStessoStepParams{ThreadID: thread, Sha256: sha})
			if err != nil {
				return nil, err
			}
			out := []ProdottoStessoStep{}
			for _, p := range pp {
				if p.ProdottoID != s.ProdottoID {
					out = append(out, ProdottoStessoStep{ProdottoID: p.ProdottoID, Come: p.Come})
				}
			}
			in.stessoStep[sha] = out
		}
	}
	// la conferma di base (contenuto-atto-1)
	if s.VersioneBomBase != nil && s.VersioneBomBase.ConfermaID != nil {
		if in.confBase, err = leggiConfermaBase(ctx, q, thread, *s.VersioneBomBase.ConfermaID); err != nil {
			return nil, err
		}
	}
	an, err := ParteAnalisiDi(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	in.inAnalisi = an.InAnalisi
	if err := in.leggiCompletamento(ctx, q, shas, chiavi); err != nil {
		return nil, err
	}
	if op.ConIngressi {
		ing, err := LeggiIngressi(ctx, q, thread, s.ProdottoID, op.ImprontaRegole)
		if err != nil {
			return nil, err
		}
		in.ingressi = &ing
	}
	return in, nil
}

// leggiCompletamento: le letture del completamento (riepilogo3_completamento.go): gli archi che entrano nei non inclusi
// (shas, chiavi), le righe legacy dello STEP sorgente e degli STEP secondari autorizzati, le rimozioni aperte, i non
// inclusi della conferma di base formato 3.
func (in *ingressiR4) leggiCompletamento(ctx context.Context, q *db.Queries, shas, chiavi []string) error {
	if len(shas) > 0 {
		aa, err := q.ListArchiPerShaChiave(ctx, db.ListArchiPerShaChiaveParams{ThreadID: in.thread, Shas: shas, Chiavi: chiavi})
		if err != nil {
			return err
		}
		for _, x := range aa {
			k := strings.TrimSpace(x.Sha256) + "|" + x.FiglioChiave
			in.archiNI[k] = append(in.archiNI[k], x)
		}
	}
	dich, err := fascicolo.LeggiDichiarazioni(ctx, q, in.thread)
	if err != nil {
		return err
	}
	for _, c := range in.stato.Componenti {
		if len(dich.DelComponente(c.ID)) > 0 {
			in.dichiarati[c.ID] = true
		}
	}
	var leggere []string
	if in.sorgente.Sha256 != "" {
		leggere = append(leggere, in.sorgente.Sha256)
	}
	for _, d := range dich.Valide() {
		if d.Componente.ComponenteID == in.prodotto || d.Componente.Tipo == db.TipoComponenteFinito || strings.TrimSpace(d.Sha256) == "" {
			continue
		}
		in.secondari = append(in.secondari, stepSecondario{componente: d.Componente.ComponenteID, sha256: strings.TrimSpace(d.Sha256)})
		leggere = append(leggere, strings.TrimSpace(d.Sha256))
	}
	if leggere = shaOrdinati(leggere); len(leggere) > 0 {
		rr, err := q.ListRigheStrutturaR4(ctx, db.ListRigheStrutturaR4Params{ThreadID: in.thread, Shas: leggere})
		if err != nil {
			return err
		}
		for _, r := range rr {
			k := strings.TrimSpace(r.Sha256)
			in.righeSha[k] = append(in.righeSha[k], r)
		}
	}
	if in.rimozioni, err = q.ListRimozioniAperte(ctx, in.thread); err != nil {
		return err
	}
	if cb := in.confBase; cb != nil && cb.formato == FormatoSnapshot3 && in.snap.VersioneBomBase != nil && in.snap.VersioneBomBase.ConfermaID != nil {
		if in.nonInclusiBase, err = q.ListNonInclusiConferma(ctx, db.ListNonInclusiConfermaParams{ConfermaID: *in.snap.VersioneBomBase.ConfermaID,
			ThreadID: in.thread}); err != nil {
			return err
		}
	}
	return nil
}

// riferimentiDellaRfq: i componenti che nodi e decisioni citano per UUID sono della RFQ, e gli allegati delle PB, degli
// svincoli e della sorgente sono della RFQ (FUORI_RFQ, come R2).
func riferimentiDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot3, st *confermabom.StatoRFQ) error {
	comp := func(campo string, id *uuid.UUID) error {
		if id == nil {
			return nil
		}
		if _, ok := st.Componente(*id); !ok {
			return fuori(campo, "il componente %s non è di questa RFQ", *id)
		}
		return nil
	}
	for _, n := range s.Nodi {
		if err := comp("nodi.componente_id", n.ComponenteID); err != nil {
			return err
		}
	}
	for _, d := range s.Decisioni {
		for _, id := range []*uuid.UUID{d.ComponenteID, d.PadreID, d.FiglioID} {
			if err := comp("decisioni."+d.ID, id); err != nil {
				return err
			}
		}
	}
	var ids []uuid.UUID
	for _, p := range s.DocumentiPreassociati {
		ids = append(ids, p.AllegatoID)
	}
	for _, d := range s.Decisioni {
		if d.AllegatoID != nil {
			ids = append(ids, *d.AllegatoID)
		}
	}
	if s.Sorgente != nil && s.Sorgente.AllegatoID != nil {
		ids = append(ids, *s.Sorgente.AllegatoID)
	}
	citati, err := LeggiAllegatiCitati(ctx, q, thread, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !citati[id].DellaRfq {
			return fuori("allegato_id", "l'allegato %s non è di questa RFQ", id)
		}
	}
	return nil
}

// provenienzeR4: la provenienza delle PB del corpo con la regola di R2 (provenienza3.go, commento di testa), contro la
// versione salvata (prima; nil = nessuna), senza i 422: un registro dichiarato che non propone il file sul nodo non conta, e
// vale la regola di una PB nuova o spostata [T-39]. adesso (nil = nessun calcolo: R4 senza ?avvisi_motore=1) da' il
// registro del calcolo di adesso, come R2 (G39-01; rilievo minore 6 della revisione G39 parte A): un suggerimento
// dopo_apertura accettato nel corpo e non ancora salvato si vede «motore» o «corretta» come lo salverebbe R2, non «manuale».
func provenienzeR4(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot3, prima *Snapshot3, adesso func() *importazione.Registro) (Snapshot3, error) {
	pp := map[uuid.UUID]Preassociazione3{}
	if prima != nil {
		for _, p := range prima.DocumentiPreassociati {
			pp[p.AllegatoID] = p
		}
	}
	nodi := map[string]Nodo3{}
	for _, n := range s.Nodi {
		nodi[n.ChiaveBozza] = n
	}
	registri := map[string]*importazione.Registro{}
	out := s.copia()
	for i, x := range s.DocumentiPreassociati {
		p := decisa(pp, x.AllegatoID, x.ChiaveBozza)
		if x.Importazione != "" && !rimasta(pp, x.AllegatoID, x.ChiaveBozza) && reSha256.MatchString(x.Importazione) {
			reg, ok := registri[x.Importazione]
			if !ok {
				rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: thread, ProdottoID: s.ProdottoID,
					Impronte: []string{x.Importazione}})
				if err != nil {
					return s, err
				}
				if len(rr) > 0 {
					r, _, err := RileggiImportazione(db.GetDistintaImportazioneRow(rr[0]))
					if err != nil {
						return s, err
					}
					reg = &r
				}
				registri[x.Importazione] = reg
			}
			impronta := x.Importazione
			if reg == nil && adesso != nil {
				if r, _ := registroDiAdessoPer(adesso(), thread, s.ProdottoID, x.Importazione, x.AllegatoID, nodi[x.ChiaveBozza]); r != nil {
					reg, impronta = r, r.Impronta
				}
			}
			switch {
			case reg != nil && PropostaNelRegistro(reg.Contenuto, nodi[x.ChiaveBozza], x.AllegatoID) && destinazioneUnica(reg.Contenuto, x.AllegatoID) != "":
				p.Provenienza, p.Importazione = ProvenienzaMotore, impronta
			case reg != nil && p.Provenienza == ProvenienzaManuale && destinazioneUnica(reg.Contenuto, x.AllegatoID) != "":
				// G39-01 [U]: la proposta del motore spostata su un altro componente e' corretta (la regola di R2)
				p.Provenienza, p.Importazione = ProvenienzaCorretta, impronta
			}
		}
		out.DocumentiPreassociati[i] = p
	}
	return out, nil
}

// leggiSorgente: lo stato della sorgente della versione rispetto al gate di R5 (EM App. I.2; confermabom.SorgenteValida),
// gli STEP del prodotto (legame storico di D-AR04 (iii)) e, per una sorgente valida, la sua gerarchia (prima-istanza-1 (d)).
func (in *ingressiR4) leggiSorgente(ctx context.Context, q *db.Queries) error {
	s := in.snap
	ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: in.thread, ProdottoID: in.prodotto})
	if err != nil {
		return err
	}
	for _, e := range ee {
		if e.Ruolo == "prodotto" && e.Azione == "conferma_documento" && e.ChiaveIdempotenza.Valid {
			in.shaS8[strings.TrimSpace(e.Sha256)] = true
		}
	}
	if in.stepProd, err = q.ListStepDelProdotto(ctx, db.ListStepDelProdottoParams{ThreadID: in.thread, ProdottoID: in.prodotto}); err != nil {
		return err
	}
	if s.Sorgente == nil || s.Sorgente.AllegatoID == nil {
		in.sorgente = SorgenteR4{Motivo: SorgenteMotivoSenzaSorgente}
		if s.Sorgente != nil {
			in.sorgente.Gruppo = s.Sorgente.Gruppo
		}
		return nil
	}
	a := *s.Sorgente.AllegatoID
	in.sorgente = SorgenteR4{AllegatoID: &a, Sha256: s.Sorgente.Sha256, Gruppo: s.Sorgente.Gruppo}
	citati, err := LeggiAllegatiCitati(ctx, q, in.thread, []uuid.UUID{a})
	if err != nil {
		return err
	}
	if c := citati[a]; !c.DellaRfq || !c.Attivo || c.Sha256 != s.Sorgente.Sha256 {
		in.sorgente.Motivo = SorgenteMotivoNonCoerente
		return nil
	}
	occ, err := q.ListOccorrenzePiano(ctx, db.ListOccorrenzePianoParams{ThreadID: in.thread, Allegati: []uuid.UUID{a}})
	if err != nil {
		return err
	}
	if len(occ) == 1 {
		in.nomeFonte = occ[0].NomeFile
	}
	sv, err := confermabom.SorgenteValida(ctx, q, in.thread, in.prodotto, s.Sorgente.Sha256)
	if err != nil {
		return err
	}
	in.sorgente.Valida, in.sorgente.Motivo, in.sorgente.LegameStorico = sv.Valida, sv.Motivo, sv.Storica && !sv.Valida
	in.sorgente.Correzione = sv.Valida && sv.Correzione
	if sv.Documento != uuid.Nil {
		d := sv.Documento
		in.sorgente.DocumentoID = &d
	}
	if !sv.Valida {
		return nil
	}
	row, err := q.GetStrutturaDellaSorgente(ctx, db.GetStrutturaDellaSorgenteParams{Sha256: s.Sorgente.Sha256, ThreadID: in.thread,
		ComponenteID: in.prodotto, DocumentoID: sv.Documento})
	if err != nil {
		return err
	}
	in.struttura = &row
	return nil
}

// completaFattiPiano: il perimetro del prodotto (con i componenti che la bozza riusa) e, per ogni componente, i prodotti
// finiti della RFQ nel cui perimetro sta; lo STEP sorgente della bozza conta come legato come sorgente anche quando la bozza
// non e' salvata (la funzione del database guarda solo le bozze salvate).
func (in *ingressiR4) completaFattiPiano() {
	if per, err := in.stato.Perimetro(in.prodotto); err == nil {
		for _, c := range per.Componenti {
			in.fatti.DelProdotto[c.ID] = true
		}
	}
	for _, n := range in.snap.Nodi {
		if n.ComponenteID != nil {
			in.fatti.DelProdotto[*n.ComponenteID] = true
		}
	}
	for _, p := range in.finiti() {
		per, err := in.stato.Perimetro(p)
		if err != nil {
			continue
		}
		for _, c := range per.Componenti {
			in.fatti.ProdottiDi[c.ID] = append(in.fatti.ProdottiDi[c.ID], p)
		}
	}
	if s := in.snap.Sorgente; s != nil && s.Sha256 != "" {
		for a, o := range in.fatti.Occorrenze {
			if o.Sha256 == s.Sha256 {
				o.LegatoComeSorgente = true
				in.fatti.Occorrenze[a] = o
			}
		}
	}
}

// stepCandidatiNellePB (G39-02 [U]): le PB della bozza su uno STEP candidato sorgente di un finito sono in conflitto
// (associazioni.MotivoStepCandidatoDelFinito, blocco PREASSOCIAZIONE_NON_APPLICABILE), anche quando vengono da una versione
// salvata prima: lo STEP del finito non e' mai una PB ne' una AR ordinaria. Il riconoscimento e' del Motore A quando R4
// lo calcola (?avvisi_motore=1, e sempre nella S di R5), altrimenti il ripiego [T-39] (StepCandidatiDelFinito). Lo STEP
// gia' legato come sorgente resta con il suo motivo. Come R2, la PB su uno STEP candidato non univoco verso una parte o
// un sottoassieme non e' in conflitto (StepCandidati.BloccaPB; G39-06 [U], giro G39-ter).
func (in *ingressiR4) stepCandidatiNellePB(ctx context.Context, q *db.Queries) error {
	if len(in.snap.DocumentiPreassociati) == 0 {
		return nil
	}
	var motore *StepCandidati
	if in.esitoMotore != nil && in.esitoMotore.Ingresso != nil {
		motore = in.esitoMotore.Ingresso.StepCandidati
	}
	ids := make([]uuid.UUID, 0, len(in.snap.DocumentiPreassociati))
	for _, p := range in.snap.DocumentiPreassociati {
		ids = append(ids, p.AllegatoID)
	}
	cand, err := StepCandidatiDelFinito(ctx, q, in.thread, ids, motore)
	if err != nil {
		return err
	}
	for _, p := range in.snap.DocumentiPreassociati {
		if !cand.BloccaPB(p.AllegatoID, NodoVersoUnFinito(in.snap, p.ChiaveBozza, in.stato)) {
			continue
		}
		if o, ok := in.fatti.Occorrenze[p.AllegatoID]; ok && !o.LegatoComeSorgente {
			o.StepCandidatoDelFinito = true
			in.fatti.Occorrenze[p.AllegatoID] = o
		}
	}
	return nil
}

// finiti: i prodotti finiti non archiviati della RFQ, in ordine.
func (in *ingressiR4) finiti() []uuid.UUID {
	var out []uuid.UUID
	for _, c := range in.stato.Componenti {
		if c.Tipo == string(db.TipoComponenteFinito) && !c.Archiviato {
			out = append(out, c.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// leggiConfermaBase: la conferma di base (snapshot, mappa, criterio, sha dello STEP certificato della radice).
func leggiConfermaBase(ctx context.Context, q *db.Queries, thread, conferma uuid.UUID) (*confermaBaseR4, error) {
	r, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: thread, ConfermaID: conferma})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &confermaBaseR4{formato: int(r.Formato), criterio: r.CriterioVerifica, monoPezzo: r.MonoPezzo, mappa: map[string]uuid.UUID{}}
	if len(r.Mappa) > 0 {
		_ = json.Unmarshal(r.Mappa, &out.mappa)
	}
	letto := LeggiSnapshotConferma(int(r.Formato), r.Snapshot, r.ImprontaSnapshot)
	if letto.Formato3 != nil {
		x := *letto.Formato3
		out.snap = &x
	}
	cc, err := q.ListConfermaStrutturaleComponenti(ctx, db.ListConfermaStrutturaleComponentiParams{ConfermaID: conferma, ThreadID: thread})
	if err != nil {
		return nil, err
	}
	for _, c := range cc {
		if c.Radice {
			out.radiceSha = strings.TrimSpace(c.StepSha256.String)
		}
	}
	return out, nil
}
