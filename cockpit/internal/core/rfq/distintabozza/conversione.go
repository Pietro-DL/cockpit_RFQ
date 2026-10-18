package distintabozza

// La conversione di una bozza formato 2 aperta nel formato 3, algoritmo conversione-1 (EM §6.7; PM23 U-05). Serve un
// comando: il trigger del ciclo vieta di cambiare il formato di una bozza aperta, e D-18 vieta le conversioni in lettura.
// In una transazione:
//  1. la bozza formato 2 si abbandona, con motivo_chiusura «convertita nel formato 3 nella bozza <id nuovo>», senza il
//     motivo libero (sta nei 1000 caratteri di ck_distinta_bozza_chiusura); il motivo intero va nell'evento bozza_aperta
//     della bozza nuova; la riga abbandonata del registro delle bozze la scrive il trigger;
//  2. la bozza formato 3 nasce alla versione 1, costruita dal server: nodi, archi, decisioni e preassociazioni copiati;
//     identita = {cliente, presunto, codice, rev, modo formato_2}; le caselle completate con l'allegato della sorgente o,
//     senza, con l'unico allegato attivo della RFQ con quello sha256 (piu' d'uno: conversione_ambigua); caselle di piu'
//     gruppi: conversione_piu_gruppi; righe_proposta lasciate cadere con la traccia (righe_cadute); un arco dello STEP
//     che nel formato 3 non unisce due caselle (il nodo del prodotto formato 2 non ne ha) resta come arco dell'ingegnere,
//     con la traccia (archi_senza_step: EM §6.10, nessuna riscrittura silenziosa; proposta [T] della corsia); nessun
//     registro;
//  3. l'evento bozza_aperta della bozza nuova: partenza formato_2, il motivo, da_bozza {bozza_id, versione, impronta,
//     righe_cadute[], archi_senza_step[]};
//  4. le fonti secondarie con una destinazione temporanea sulla bozza formato 2, non ancora risolte: un evento scelta con
//     la stessa chiave e la stessa fonte sulla bozza nuova, dopo la versione 1.
//
// Gli snapshot formato 2 restano com'erano. In V1, ondata B, il servizio c'e' e lo provano le L4 (U-05); la forma
// converti_da di R2 che lo chiama arriva con l'ondata C, con le altre forme costruite dal server.

import (
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

	"promatec/cockpit/internal/platform/db"
)

// AlgoritmoConversione e' l'algoritmo che costruisce la versione 1 di una bozza convertita (EM §2.5, §6.7).
const AlgoritmoConversione = "conversione-1"

// PartenzaFormato2: la partenza dell'evento bozza_aperta di una bozza convertita.
const PartenzaFormato2 = "formato_2"

// I motivi della conversione (EM §9.2) e il codice del motivo mancante.
const (
	MotivoConversioneAmbigua   = "conversione_ambigua"
	MotivoConversionePiuGruppi = "conversione_piu_gruppi"
	CodiceMotivoObbligatorio   = "MOTIVO_OBBLIGATORIO"
)

// Conversione3 e' una richiesta di conversione (la forma converti_da di R2): la bozza formato 2, la sua versione, il
// motivo, la chiave del salvataggio e l'impronta della richiesta.
type Conversione3 struct {
	ThreadID, ProdottoID, UtenteID uuid.UUID
	BozzaID                        uuid.UUID
	VersioneAttesa                 int
	Motivo                         string
	ChiaveSalvataggio              uuid.UUID
	ImprontaRichiesta              string
}

// RigheCadute: le righe legacy di un nodo che la conversione lascia cadere (EM §6.7: R5 non le decidera').
type RigheCadute struct {
	ChiaveBozza string      `json:"chiave_bozza"`
	Righe       []uuid.UUID `json:"righe"`
}

// ArcoSenzaStep: un arco con origine step nel formato 2 che nel formato 3 non si lega a due caselle (EM §3.3: lo step di un
// arco e' (allegato, rif del padre, rif del figlio); il nodo del prodotto formato 2 non ha caselle, §6.7) e resta come arco
// dell'ingegnere, origine utente, con la stessa qta.
type ArcoSenzaStep struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
}

// Convertita e' l'esito della conversione: la versione 1 della bozza nuova con il suo snapshot, le righe cadute e gli archi
// rimasti senza step.
type Convertita struct {
	Salvato3
	Snapshot       json.RawMessage `json:"snapshot"`
	RigheCadute    []RigheCadute   `json:"righe_cadute"`
	ArchiSenzaStep []ArcoSenzaStep `json:"archi_senza_step"`
}

// Converti converte una bozza formato 2 aperta (commento di testa).
func Converti(ctx context.Context, pool Avviatore, c Conversione3) (Convertita, error) {
	var out Convertita
	motivo := strings.TrimSpace(nfc(c.Motivo))
	if motivo == "" {
		return out, &Errore{Codice: CodiceMotivoObbligatorio, Campo: "converti_da.motivo", Messaggio: "la conversione vuole il motivo"}
	}
	if !testoLibero(motivo, MaxMotivo) {
		return out, corpo("converti_da.motivo", "il motivo sta in %d caratteri, senza caratteri di controllo", MaxMotivo)
	}
	if c.ChiaveSalvataggio.Version() != 4 || !reSha256.MatchString(c.ImprontaRichiesta) {
		return out, corpo("chiave_salvataggio", "la chiave del salvataggio e' un UUID v4, con l'impronta della richiesta")
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
	if gia, err := q.GetVersionePerChiaveGlobale(ctx, uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true}); err == nil {
		if gia.ThreadID != c.ThreadID || strings.TrimSpace(gia.ImprontaRichiesta.String) != c.ImprontaRichiesta {
			return out, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio", Messaggio: "questa chiave di salvataggio ha gia' salvato un altro corpo"}
		}
		return riproponiConversione(ctx, q, c.ThreadID, gia.BozzaID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if _, err := prodottoDellaRfq(ctx, q, c.ThreadID, c.ProdottoID, true); err != nil {
		return out, err
	}
	b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: c.BozzaID, ThreadID: c.ThreadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("converti_da.bozza_id", "la bozza %s non e' di questa RFQ", c.BozzaID)
	}
	if err != nil {
		return out, err
	}
	if b.ProdottoID != c.ProdottoID {
		return out, &Errore{Codice: CodiceBozzaAltroProdotto, Campo: "converti_da.bozza_id", Messaggio: "la bozza e' di un altro prodotto"}
	}
	if b.Stato != StatoAperta || int(b.Versione) != c.VersioneAttesa {
		return out, conflittoVersione(b, c.VersioneAttesa)
	}
	if int(b.Formato) != FormatoSnapshot {
		return out, corpo("converti_da.bozza_id", "la bozza e' gia' nel formato %d: si converte solo una bozza formato 2", b.Formato)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: c.ThreadID, Versione: b.Versione})
	if err != nil {
		return out, err
	}
	vecchio, improntaVecchia, err := rileggiSnapshot(v.Snapshot, v.Impronta)
	if err != nil {
		return out, err
	}
	cliente, err := q.GetClienteDellaRfq(ctx, c.ThreadID)
	if err != nil {
		return out, err
	}
	nuovo, cadute, senzaStep, err := convertiSnapshot(ctx, q, c.ThreadID, vecchio, EmittenteCliente(cliente))
	if err != nil {
		return out, err
	}
	canon, err := nuovo.Canonico()
	if err != nil {
		var e *Errore
		if errors.As(err, &e) {
			e.Messaggio = "la bozza formato 2 non si converte: " + e.Messaggio
		}
		return out, err
	}
	impronta := jsonImpronta(canon)

	// 1. la bozza formato 2 si chiude
	nuovoID := uuid.New()
	n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: c.UtenteID, Valid: true},
		Motivo: testo("convertita nel formato 3 nella bozza " + nuovoID.String()), BozzaID: b.BozzaID, ThreadID: c.ThreadID, Versione: b.Versione})
	if err != nil {
		return out, err
	}
	if n != 1 {
		return out, conflittoVersione(b, c.VersioneAttesa)
	}
	// 2. la bozza formato 3, versione 1
	base := baseDelloSnapshot3(nuovo)
	fonteAll, fonteSha, fonteGruppo := colonneFonte3(nuovo.Sorgente)
	if _, err := q.InsertDistintaBozzaConID(ctx, db.InsertDistintaBozzaConIDParams{BozzaID: nuovoID, ThreadID: c.ThreadID, ProdottoID: c.ProdottoID,
		Formato: FormatoSnapshot3, BaseConfermaID: nullUUID(base.ConfermaID), BaseNumero: int32(base.Numero), BaseContatore: base.Contatore,
		FirmaBase: base.Firma, FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, UtenteID: c.UtenteID}); err != nil {
		return out, erroreDiScrittura(err, "converti_da")
	}
	salvataIl, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{
		BozzaID: nuovoID, Versione: 1, Snapshot: canon, Impronta: impronta, BaseConfermaID: nullUUID(base.ConfermaID),
		BaseNumero: int32(base.Numero), BaseContatore: base.Contatore, FirmaBase: base.Firma, FonteAllegatoID: fonteAll,
		FonteSha256: fonteSha, FonteGruppo: fonteGruppo, SalvataDa: c.UtenteID,
		ChiaveSalvataggio: uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true}, ImprontaRichiesta: testo(c.ImprontaRichiesta),
	})
	if err != nil {
		return out, erroreDiScrittura(err, "converti_da")
	}
	// 3. l'evento bozza_aperta (origine server), con il motivo intero e la traccia delle righe cadute e degli archi senza step
	dopo := canonico(map[string]any{"partenza": PartenzaFormato2, "algoritmo": AlgoritmoConversione, "impronta": impronta,
		"nodi": len(nuovo.Nodi), "archi": len(nuovo.Archi),
		"da_bozza": map[string]any{"bozza_id": b.BozzaID, "versione": b.Versione, "impronta": improntaVecchia, "righe_cadute": cadute,
			"archi_senza_step": senzaStep}})
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: nuovoID, N: 1, Versione: 1,
		Vocabolario: VocabolarioFormato3, Azione: AzBozzaAperta, Dopo: rawONull(dopo), Origine: "server", Motivo: testo(motivo),
		UtenteID: c.UtenteID}); err != nil {
		return out, erroreDiScrittura(err, "converti_da")
	}
	// 4. le fonti secondarie con la destinazione temporanea sulla bozza di prima, non ancora risolte
	fonti, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: c.ThreadID, ProdottoID: c.ProdottoID})
	if err != nil {
		return out, err
	}
	risolti := map[int64]bool{}
	for _, f := range fonti {
		if f.RisolveEventoID.Valid {
			risolti[f.RisolveEventoID.Int64] = true
		}
	}
	for _, f := range fonti {
		if f.Ruolo != "sottoassieme" || f.Azione != "scelta" || !f.ChiaveBozza.Valid || !f.BozzaID.Valid || f.BozzaID.UUID != b.BozzaID ||
			risolti[f.EventoID] {
			continue
		}
		if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: c.ThreadID, ProdottoID: c.ProdottoID,
			Ruolo: "sottoassieme", ChiaveBozza: f.ChiaveBozza, Azione: "scelta", AllegatoID: f.AllegatoID, Sha256: f.Sha256,
			Gruppo: f.Gruppo, PropostaID: f.PropostaID, DocumentoID: f.DocumentoID, BozzaID: uuid.NullUUID{UUID: nuovoID, Valid: true},
			Motivo: f.Motivo, AutoreID: c.UtenteID}); err != nil {
			return out, erroreDiScrittura(err, "converti_da")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if s := sqlstate(err); s == "P4X04" || s == "P4X07" {
			return out, corpo("converti_da", "il database rifiuta la conversione (%s): la bozza formato 2 resta aperta", s)
		}
		return out, err
	}
	out = Convertita{Salvato3: Salvato3{BozzaID: nuovoID, Formato: FormatoSnapshot3, Versione: 1, NuovaVersione: true, Eventi: 1,
		Impronta: impronta, SalvataIl: salvataIl}, Snapshot: canon, RigheCadute: cadute, ArchiSenzaStep: senzaStep}
	return out, nil
}

// riproponiConversione: il reinvio di una conversione gia' fatta (stessa chiave, stesso corpo): la versione 1 e gli
// elenchi ricalcolati dai dati salvati, senza scrivere.
func riproponiConversione(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID) (Convertita, error) {
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: bozza, ThreadID: thread, Versione: 1})
	if err != nil {
		return Convertita{}, err
	}
	s, imp, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return Convertita{}, err
	}
	can, err := s.codifica()
	if err != nil {
		return Convertita{}, err
	}
	out := Convertita{Salvato3: Salvato3{BozzaID: bozza, Formato: FormatoSnapshot3, Versione: 1, Riproposto: true, Impronta: imp,
		SalvataIl: v.SalvataIl}, Snapshot: can, RigheCadute: []RigheCadute{}, ArchiSenzaStep: []ArcoSenzaStep{}}
	e, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: bozza, ThreadID: thread, N: 1})
	if err == nil && e.Dopo != nil {
		var d struct {
			DaBozza struct {
				RigheCadute    []RigheCadute   `json:"righe_cadute"`
				ArchiSenzaStep []ArcoSenzaStep `json:"archi_senza_step"`
			} `json:"da_bozza"`
		}
		if json.Unmarshal(*e.Dopo, &d) == nil {
			if d.DaBozza.RigheCadute != nil {
				out.RigheCadute = d.DaBozza.RigheCadute
			}
			if d.DaBozza.ArchiSenzaStep != nil {
				out.ArchiSenzaStep = d.DaBozza.ArchiSenzaStep
			}
		}
	}
	return out, nil
}

// convertiSnapshot: lo snapshot formato 3 della versione 1 di una bozza convertita (commento di testa, punto 2).
func convertiSnapshot(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot, cliente string) (Snapshot3, []RigheCadute, []ArcoSenzaStep, error) {
	out := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: s.ProdottoID, VersioneBomBase: s.VersioneBomBase, FirmaBase: s.FirmaBase}
	cadute := []RigheCadute{}
	var sorgente *Sorgente3
	if s.Sorgente != nil {
		sorgente = &Sorgente3{AllegatoID: s.Sorgente.AllegatoID, Sha256: s.Sorgente.Sha256, Gruppo: s.Sorgente.Gruppo}
	}
	// le caselle: un gruppo solo (quello della sorgente) e l'allegato da completare
	var piuGruppi, ambigui []string
	allegatoPerSha := map[string][]uuid.UUID{}
	allegatoDi := func(sha string) ([]uuid.UUID, error) {
		if a, ok := allegatoPerSha[sha]; ok {
			return a, nil
		}
		a, err := q.ListAllegatiAttiviPerSha(ctx, db.ListAllegatiAttiviPerShaParams{ThreadID: thread, Sha256: pgText(sha)})
		if err != nil {
			return nil, err
		}
		allegatoPerSha[sha] = a
		return a, nil
	}
	for _, n := range s.Nodi {
		x := Nodo3{ChiaveBozza: n.ChiaveBozza, ComponenteID: n.ComponenteID, Op: n.Op, Tipo: n.Tipo, Descrizione: n.Descrizione, Origine: n.Origine,
			Identita: Identita{Emittente: cliente, EmittentePresunto: true, Codice: n.Codice, Rev: n.Rev, Modo: ModoFormato2}}
		for _, c := range n.Caselle {
			if sorgente == nil || c.Gruppo != sorgente.Gruppo {
				piuGruppi = append(piuGruppi, n.ChiaveBozza)
				break
			}
			sha, _ := shaEChiave(c.Rif)
			var all uuid.UUID
			switch {
			case sorgente.AllegatoID != nil && sorgente.Sha256 == sha:
				all = *sorgente.AllegatoID
			default:
				aa, err := allegatoDi(sha)
				if err != nil {
					return out, nil, nil, err
				}
				if len(aa) != 1 {
					ambigui = append(ambigui, n.ChiaveBozza)
					continue
				}
				all = aa[0]
			}
			x.Caselle = append(x.Caselle, Casella3{AllegatoID: all, Gruppo: c.Gruppo, Rif: c.Rif})
		}
		if len(n.RigheProposta) > 0 {
			cadute = append(cadute, RigheCadute{ChiaveBozza: n.ChiaveBozza, Righe: append([]uuid.UUID(nil), n.RigheProposta...)})
		}
		out.Nodi = append(out.Nodi, x)
	}
	if len(piuGruppi) > 0 {
		sort.Strings(piuGruppi)
		return out, nil, nil, corpoMotivo("converti_da", MotivoConversionePiuGruppi,
			"nodi con caselle di un gruppo diverso da quello della sorgente (il formato 3 ha un gruppo solo): %s", strings.Join(piuGruppi, ", "))
	}
	if len(ambigui) > 0 {
		sort.Strings(ambigui)
		return out, nil, nil, corpoMotivo("converti_da", MotivoConversioneAmbigua,
			"nodi le cui caselle non hanno un solo allegato attivo con quello sha256: %s", strings.Join(ambigui, ", "))
	}
	// la sorgente senza allegato prende quello delle caselle, se e' uno solo
	if sorgente != nil && sorgente.AllegatoID == nil {
		var all *uuid.UUID
		sha := ""
		for _, n := range out.Nodi {
			for _, c := range n.Caselle {
				a := c.AllegatoID
				if all != nil && *all != a {
					return out, nil, nil, corpoMotivo("converti_da", MotivoConversioneAmbigua, "le caselle stanno su piu' allegati: la sorgente non e' una")
				}
				all = &a
				sha, _ = shaEChiave(c.Rif)
			}
		}
		if all != nil {
			sorgente.AllegatoID, sorgente.Sha256 = all, sha
		}
	}
	out.Sorgente = sorgente
	conCaselle := map[string]bool{}
	for _, n := range out.Nodi {
		if len(n.Caselle) > 0 {
			conCaselle[n.ChiaveBozza] = true
		}
	}
	senzaStep := []ArcoSenzaStep{}
	for _, a := range s.Archi {
		x := Arco3{Padre: a.Padre, Figlio: a.Figlio, Qta: a.Qta, Origine: a.Origine, Posizione: a.Posizione}
		if a.Origine == OrigineStep {
			if sorgente != nil && sorgente.AllegatoID != nil && conCaselle[a.Padre] && conCaselle[a.Figlio] {
				x.Step = &StepArco3{AllegatoID: *sorgente.AllegatoID, Gruppo: sorgente.Gruppo}
			} else {
				x.Origine = OrigineUtente
				senzaStep = append(senzaStep, ArcoSenzaStep{Padre: a.Padre, Figlio: a.Figlio})
			}
		}
		out.Archi = append(out.Archi, x)
	}
	for i, raw := range s.Decisioni {
		d, e := LeggiDecisione3(fmt.Sprintf("decisioni[%d]", i), raw)
		if e != nil {
			e.Messaggio = "la decisione della bozza formato 2 non ha la forma del formato 3: " + e.Messaggio
			return out, nil, nil, e
		}
		out.Decisioni = append(out.Decisioni, d)
	}
	out.DocumentiPreassociati = append([]Preassociazione(nil), s.DocumentiPreassociati...)
	return out, cadute, senzaStep, nil
}

func pgText(s string) pgtype.Text { return testo(s) }

// la data della conversione per le prove (il database la scrive da se').
var _ = time.Now
