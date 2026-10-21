package distintabozza

// Le parti comuni delle forme di R2 che importano (crea con partenza step, cambia_sorgente, riallinea_proposta; EM §6.1,
// §6.2, §6.3, §6.4.1; U2 §2.8; direttiva 35, blocco C-3): la scelta del gruppo fra quelli che il server calcola adesso
// (prima della transazione, da chi chiama, sul calcolo del motore in sola lettura), V-7, e sotto il lucchetto le
// occorrenze del registro, la fonte confermata, il motivo della scelta della fonte, il registro trovato o scritto.
//
// Il calcolo del motore lo fa chi chiama (il web), prima della transazione di scrittura e fuori da ogni lucchetto (EM §6.3
// passo 1): qui arrivano il registro del gruppo scelto (PropostaCalcolata) e, per la scelta, i gruppi del prodotto con le
// occorrenze non attive e una funzione che importa un gruppo (per ritrovare la stessa struttura sotto un'altra chiave).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// PropostaCalcolata: il registro del gruppo scelto, calcolato e composto da chi chiama prima della transazione (EM §6.3
// passi 1-2), gia' confrontato con l'impronta dichiarata (V-7).
type PropostaCalcolata struct {
	Registro importazione.Registro
	// StepCandidati: gli STEP candidati sorgente dei finiti nello stesso calcolo del motore (StepCandidatiDalMotore,
	// G39-02 [U]); nil = il ripiego per le PB iniziali.
	StepCandidati *StepCandidati
}

// step: gli STEP candidati del calcolo (nil senza la proposta o senza il riconoscimento).
func (p *PropostaCalcolata) step() *StepCandidati {
	if p == nil {
		return nil
	}
	return p.StepCandidati
}

// GruppoRitrovato: il gruppo che porta la stessa struttura sotto un'altra chiave (il 409 ANTEPRIMA_SUPERATA del §6.1), con
// la sua impronta del contenuto.
type GruppoRitrovato struct {
	Chiave            string `json:"chiave"`
	Come              string `json:"come"`
	ImprontaContenuto string `json:"impronta_contenuto"`
}

// ImportazioneR2: il registro della versione costruita dal server nella risposta di R2 (EM §9.2): l'id, le impronte, se e'
// un registro gia' scritto che si riusa (stessa impronta), e gli elenchi delle forme che cambiano la fonte o riallineano.
type ImportazioneR2 struct {
	ImportazioneID    uuid.UUID       `json:"importazione_id"`
	Impronta          string          `json:"impronta"`
	ImprontaContenuto string          `json:"impronta_contenuto"`
	ImprontaEvidenze  string          `json:"impronta_evidenze"`
	Riusata           bool            `json:"riusata"`
	Differenze        json.RawMessage `json:"differenze,omitempty"`
	// gli elenchi di cambia_sorgente e riallinea_proposta (EM §6.5, §6.6, §9.2): assenti se vuoti
	NuoviNodiProposti   []string                    `json:"nuovi_nodi_proposti,omitempty"`
	FuoriPerCambioFonte []string                    `json:"fuori_per_cambio_fonte,omitempty"`
	DecisioniCadute     []string                    `json:"decisioni_cadute,omitempty"`
	DaRivedere          []DaRivedere3               `json:"da_rivedere,omitempty"`
	IdentitaAggiornate  []IdentitaAggiornata3       `json:"identita_aggiornate,omitempty"`
	CaselleRiscritte    []CasellaRiscritta3         `json:"caselle_riscritte,omitempty"`
	ArchiRiscritti      []propostabom.ArcoRiscritto `json:"archi_riscritti,omitempty"`
	DecisioniScritte    []string                    `json:"decisioni_scritte,omitempty"`
	// cambio-fonte-2 (riconciliazione3.go): i nodi senza casella con candidati non certi (fuori da da_rivedere[])
	AbbinamentiProposti []AbbinamentoProposto3 `json:"abbinamenti_proposti,omitempty"`
	// la partenza step: le voci fuori per decisione del registro che non entrano (P-8)
	FuoriPerDecisione []string `json:"fuori_per_decisione,omitempty"`
	// direttiva 39 §4 (pbiniziali3.go): le PB iniziali del server e i suggerimenti di cambia_sorgente e
	// riallinea_proposta (preassociazioni_proposte[], mai in da_rivedere[])
	PreassociazioniIniziali []Preassociazione3         `json:"preassociazioni_iniziali,omitempty"`
	PreassociazioniProposte []PreassociazioneProposta3 `json:"preassociazioni_proposte,omitempty"`
}

// DaRivedere3: un nodo o una decisione da rivedere, con un motivo (un elemento per motivo; EM §6.5, §6.6).
type DaRivedere3 struct {
	ChiaveBozza string `json:"chiave_bozza,omitempty"`
	DecisioneID string `json:"decisione_id,omitempty"`
	Motivo      string `json:"motivo"`
}

// IdentitaAggiornata3: un nodo proposta (o da_decidere) che prende l'identita' della proposta nuova, prima → dopo.
type IdentitaAggiornata3 struct {
	ChiaveBozza string   `json:"chiave_bozza"`
	Prima       Identita `json:"prima"`
	Dopo        Identita `json:"dopo"`
}

// anteprimaSuperata: il 409 ANTEPRIMA_SUPERATA delle forme che importano (EM §6.1, V-7), anteprima proposta_step.
func anteprimaSuperata(campo, impronta string, g *GruppoRitrovato, formato string, a ...any) *Errore {
	return &Errore{Codice: CodiceAnteprimaSuperata, Motivo: MotivoPropostaStep, Anteprima: MotivoPropostaStep, Campo: campo,
		ImprontaContenuto: impronta, GruppoCorrente: g, Messaggio: fmt.Sprintf(formato, a...)}
}

// ErroreDellAdattatore: un rifiuto dell'adattatore del registro (propostabom.ImportaGruppo, importazione.Componi) come
// errore del package, con il campo della forma: gruppo_non_importabile, FILE_NON_ATTIVO, PROPOSTA_NON_DISPONIBILE,
// IMPORTAZIONE_TROPPO_GRANDE (EM §6.1). Un errore senza codice e' un difetto dell'adattatore e torna com'e' (5xx).
func ErroreDellAdattatore(err error, campo string) error {
	var e *importazione.Errore
	if errors.As(err, &e) && e.Codice != "" {
		return &Errore{Codice: e.Codice, Motivo: e.Motivo, Campo: campo, Messaggio: e.Messaggio}
	}
	return err
}

// GruppoDaImportare: il gruppo che crea (partenza step) e cambia_sorgente importano, fra i gruppi del prodotto che il
// server calcola adesso (EM §6.1, §6.3 passo 1, «Errori dell'importazione»), senza scomporre la chiave:
//   - il gruppo dichiarato c'e': deve stare sull'occorrenza dichiarata (altrimenti 422 gruppo_su_altra_occorrenza con
//     l'allegato del gruppo; un gruppo senza struttura, gruppo_non_importabile); in cambia_sorgente non deve essere la
//     struttura della sorgente sulla stessa occorrenza (stesso allegato, sha256 e radice: 422 stessa_sorgente; senza la
//     radice della sorgente il controllo non si fa);
//   - non c'e': se l'allegato dichiarato non ha gruppi del prodotto, 422 gruppo_non_importabile; se un suo gruppo ha
//     l'impronta del contenuto dichiarata, 409 ANTEPRIMA_SUPERATA con il gruppo ritrovato; altrimenti lo stesso 409 senza.
//
// improntaDi importa un gruppo e ne da' l'impronta del contenuto (la usa solo il secondo caso). Le candidate su
// un'occorrenza non attiva non ci sono, come in /dati (EM §6.4.1, §9.9).
func GruppoDaImportare(campo string, gruppi []propostabom.Gruppo, nonAttivi propostabom.NonAttivi, d SorgenteDichiarata, impronta string,
	sorgente *propostabom.SorgenteBozza, improntaDi func(chiave string) (string, error)) (propostabom.Gruppo, error) {
	var visibili []propostabom.Gruppo
	for _, g := range gruppi {
		if g.AllegatoID != nil && g.Tipo == propostabom.GruppoCandidata {
			if _, na := nonAttivi[*g.AllegatoID]; na {
				continue
			}
		}
		visibili = append(visibili, g)
	}
	for _, g := range visibili {
		if g.Chiave != d.Gruppo {
			continue
		}
		if g.AllegatoID == nil || g.Tipo == propostabom.GruppoSenzaSTEP {
			return g, corpoMotivo(campo+".gruppo", MotivoGruppoNonImportabile, "il gruppo %s non ha una struttura da importare", g.Chiave)
		}
		if *g.AllegatoID != d.AllegatoID {
			a := *g.AllegatoID
			return g, &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoGruppoSuAltraOccorrenza, Campo: campo + ".gruppo", AllegatoID: &a,
				Messaggio: fmt.Sprintf("il gruppo %s sta su un'altra occorrenza dello stesso STEP: si rifa' il gesto dichiarando quella copia", g.Chiave)}
		}
		if s := sorgente; s != nil && s.Radice != "" && s.AllegatoID == *g.AllegatoID && s.Sha256 == g.Sha256 && s.Radice == g.Radice {
			return g, corpoMotivo(campo+".gruppo", MotivoStessaSorgente,
				"il gruppo %s e' la stessa struttura della sorgente della bozza, sulla stessa occorrenza: e' un riallineamento («Aggiorna alla proposta di oggi»)", g.Chiave)
		}
		return g, nil
	}
	var sullAllegato []propostabom.Gruppo
	for _, g := range visibili {
		if g.AllegatoID != nil && *g.AllegatoID == d.AllegatoID && g.Tipo != propostabom.GruppoSenzaSTEP {
			sullAllegato = append(sullAllegato, g)
		}
	}
	if len(sullAllegato) == 0 {
		return propostabom.Gruppo{}, corpoMotivo(campo+".gruppo", MotivoGruppoNonImportabile,
			"il server non calcola piu' il gruppo %s per il prodotto, e lo STEP dichiarato non ha altri gruppi del prodotto", d.Gruppo)
	}
	for _, g := range sullAllegato {
		imp, err := improntaDi(g.Chiave)
		if err != nil {
			continue
		}
		if imp == impronta {
			return propostabom.Gruppo{}, anteprimaSuperata(campo+".gruppo", imp,
				&GruppoRitrovato{Chiave: g.Chiave, Come: propostabom.ComeStessaOccorrenza, ImprontaContenuto: imp},
				"la stessa struttura ora sta nel gruppo %s: rileggi l'anteprima dello STEP", g.Chiave)
		}
	}
	return propostabom.Gruppo{}, anteprimaSuperata(campo+".gruppo", "", nil,
		"il gruppo %s non c'e' piu' e la proposta di quello STEP e' cambiata: rileggi l'anteprima dello STEP", d.Gruppo)
}

// VerificaImprontaDichiarata: V-7 (EM §6.2): l'impronta del contenuto che il browser ha visto e' quella del registro calcolato
// adesso; altrimenti 409 ANTEPRIMA_SUPERATA (proposta_step) con l'impronta nuova, niente scritto.
func VerificaImprontaDichiarata(campo string, reg importazione.Registro, dichiarata string) error {
	if reg.Contenuto.ImprontaContenuto != dichiarata {
		return anteprimaSuperata(campo, reg.Contenuto.ImprontaContenuto, nil,
			"la proposta del Motore A e' cambiata dopo l'anteprima: rileggi le differenze prima di importare")
	}
	return nil
}

// occorrenzeDelRegistro: sotto il lucchetto, la fonte e ogni allegato citato dal registro sono ancora della RFQ e attivi
// (EM §6.3 passo 4): altrimenti 409 ANTEPRIMA_SUPERATA (proposta_step), niente scritto.
func occorrenzeDelRegistro(ctx context.Context, q *db.Queries, thread uuid.UUID, reg importazione.Registro, campo string) error {
	t := reg.Colonne()
	ids := append([]uuid.UUID(nil), t.AllegatiCitati...)
	if t.FonteAllegatoID != nil {
		ids = append(ids, *t.FonteAllegatoID)
	}
	citati, err := LeggiAllegatiCitati(ctx, q, thread, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if a, ok := citati[id]; !ok || !a.DellaRfq || !a.Attivo {
			return anteprimaSuperata(campo, reg.Contenuto.ImprontaContenuto, nil,
				"il file %s del registro non e' piu' della RFQ o non e' attivo (Altri file o scartato): rileggi l'anteprima", id)
		}
	}
	return nil
}

// fonteConfermata: la sorgente di crea (partenza step) e cambia_sorgente e' lo STEP confermato, e non sostituito, del
// prodotto (U2 §2.8, base [T] U2-P38): un documento cad_3d del prodotto con lo sha256 della fonte e senza sostituito_da.
// Il confronto e' per contenuto, mai per allegato ne' per chiave del gruppo: seguire la BOM di lavoro su un'altra copia
// dello stesso file passa. [T] di corsia: la radice non si confronta, perche' il documento confermato non la porta (la
// porta solo la marcatura di «Autorizza», che la conferma della fonte non fa).
func fonteConfermata(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha, campo string) error {
	_, err := q.GetDocumentoFonteConfermata(ctx, db.GetDocumentoFonteConfermataParams{ThreadID: thread, ComponenteID: prodotto, Sha256: sha})
	if errors.Is(err, pgx.ErrNoRows) {
		return &Errore{Codice: CodiceFonteNonConfermata, Campo: campo,
			Messaggio: "lo STEP dichiarato non e' la fonte confermata del prodotto: prima «Conferma questo STEP come sorgente» (S8)"}
	}
	return err
}

// motivoDellaFonte: il motivo della scelta della fonte e' obbligatorio quando il codice letto dalla radice dello STEP non
// coincide con quello del prodotto (EM §6.3 passo 3; DEC-B B-10: una sola funzione con S6 e S8,
// propostabom.MotivoFonteObbligatorio): 422 MOTIVO_OBBLIGATORIO.
func motivoDellaFonte(reg importazione.Registro, codiceProdotto, motivo, campo string) error {
	if strings.TrimSpace(motivo) == "" && propostabom.MotivoFonteObbligatorio(reg.Contenuto, codiceProdotto) {
		return &Errore{Codice: CodiceMotivoObbligatorio, Campo: campo,
			Messaggio: "il codice letto dalla radice dello STEP non coincide con quello del prodotto (" + propostabom.ConfrontoDelCodice(reg.Contenuto, codiceProdotto) +
				"): la scelta della fonte vuole il motivo"}
	}
	return nil
}

// registroDellaVersione: sotto il lucchetto della RFQ il registro con la stessa impronta, se c'e', si riusa; altrimenti si
// scrive (prima si cerca, poi si scrive: niente ON CONFLICT; EM §6.3 passo 4). Un rifiuto del trigger del registro (P4X08:
// la fonte o un allegato citato non attivi) e' un 409 ANTEPRIMA_SUPERATA.
func registroDellaVersione(ctx context.Context, q *db.Queries, thread, prodotto, utente uuid.UUID, reg importazione.Registro, campo string) (ImportazioneR2, error) {
	out := ImportazioneR2{Impronta: reg.Impronta, ImprontaContenuto: reg.Contenuto.ImprontaContenuto, ImprontaEvidenze: reg.Contenuto.ImprontaEvidenze}
	if reg.Contenuto.ThreadID != thread.String() || reg.Contenuto.ProdottoID != prodotto.String() {
		return out, fmt.Errorf("il registro calcolato e' di un altro prodotto o di un'altra RFQ")
	}
	gia, err := q.GetDistintaImportazionePerImpronta(ctx, db.GetDistintaImportazionePerImprontaParams{ThreadID: thread, ProdottoID: prodotto, Impronta: reg.Impronta})
	switch {
	case err == nil:
		out.ImportazioneID, out.Riusata = gia.ImportazioneID, true
		return out, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return out, err
	}
	riga, err := q.InsertDistintaImportazione(ctx, ParametriImportazione(reg, utente))
	if err != nil {
		if sqlstate(err) == "P4X08" {
			return out, anteprimaSuperata(campo, reg.Contenuto.ImprontaContenuto, nil, "il database rifiuta il registro (P4X08): rileggi l'anteprima")
		}
		return out, erroreDiScrittura(err, campo)
	}
	out.ImportazioneID = riga.ImportazioneID
	return out, nil
}
