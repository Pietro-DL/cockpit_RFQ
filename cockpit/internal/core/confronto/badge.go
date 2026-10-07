package confronto

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Badge: il confronto del vecchio e del nuovo di un file (v3 §5 A1d), dallo stato del DB e dalle basi lette con la
// grammatica, mai dall'atteso: nel banco e nell'anteprima è lo stesso (R31 a A, ristretta dalla R2: P-14). Le righe C5
// non hanno un badge apposito: il loro badge esce dalla tavola contro la decisione confermata, e «da rivedere» compare
// solo nel rapporto del banco.
type Badge string

const (
	BadgeUguale                  Badge = "uguale"
	BadgeNuovoAncoraggio         Badge = "nuovo_ancoraggio"
	BadgeDiverso                 Badge = "diverso"
	BadgeRegressioneSuConfermata Badge = "regressione_su_confermata"
	BadgeFuoriRichiesta          Badge = "fuori_richiesta"
)

// I badge in un ordine fisso: le chiavi di Esito.Conteggi, tutte presenti.
var tuttiIBadge = []Badge{BadgeUguale, BadgeNuovoAncoraggio, BadgeDiverso, BadgeRegressioneSuConfermata, BadgeFuoriRichiesta}

// I motivi di EsitoFile.Motivo: perché un file deciso ha regressione_su_confermata, o perché un file non deciso ha
// diverso. Per gli altri badge il motivo è vuoto: un'associazione ambigua resta visibile nel nuovo del file, e per un
// file deciso si conta nella misura (CorrezioniManuali.Ambiguita, R114), mai nel badge.
const (
	MotivoNonValutato                  = "non_valutato"                    // il motore non risponde: thread non valutato, contenitore, natura non «file»
	MotivoDocumentoSenzaComponente     = "documento_senza_componente"      // il documento deciso non ha componente: il motore non può proporlo
	MotivoFuoriRichiesta               = "fuori_richiesta"                 // il motore dice il file deciso fuori richiesta
	MotivoNessunCandidato              = "nessun_candidato"                // il motore non ha candidati
	MotivoComponenteNonProposto        = "componente_non_proposto"         // il componente deciso non è fra i candidati, e nessuno ha la sua base
	MotivoStessaBaseAltroTarget        = "stessa_base_altro_target"        // il componente deciso non è fra i candidati, ma uno ha la sua base
	MotivoCodiceRegistratoNonLeggibile = "codice_registrato_non_leggibile" // il codice registrato non si legge con la grammatica: la base vecchia non c'è
	MotivoBaseDiversa                  = "base_diversa"                    // la base del candidato non è la base vecchia
	MotivoPropostaScartata             = "proposta_scartata"               // la proposta è scartata e il motore dà un candidato
	MotivoAncoraggioAssente            = "ancoraggio_assente"              // il vecchio ha una destinazione, il nuovo nessun candidato
	MotivoTargetDiverso                = "target_diverso"                  // nessun candidato ha il target del vecchio
)

// I valori di IndicatoreRevisione.Valore.
const (
	RevisioneUguale           = "uguale"
	RevisioneDiversa          = "diversa"
	RevisioneNonDeterminabile = "non_determinabile"
)

// IndicatoreRevisione: uguale | diversa | non_determinabile, con le due revisioni che valutazione ha confrontato, come le
// ha lette (Vecchia: la revisione vecchia interpretata, Vecchio.Revisione; Nuova: quella letta nel file, Nuovo.Revisione),
// e il motivo che valutazione ha dato. ProvenienzaVecchia dice da dove viene Vecchia (Vecchio.RevisioneDa: «codice»,
// «colonna» o "", R113 B ratificata; E2 §2.6): si copia, non si interpreta, e non cambia né il valore né il badge. La
// colonna rev registrata resta nel vecchio della riga (Vecchio.Rev). Mai un badge: la sola revisione non è una
// regressione (v3 §6; R31 b; E-19), e l'indicatore resta separato dalla correttezza dell'associazione (R113). Il valore
// lo traduce da Nuovo.Revisioni, che decide valutazione con la grammatica: le equivalenze sono solo quelle dichiarate
// dalle regole, mai una normalizzazione fatta qui.
type IndicatoreRevisione struct {
	Valore             string `json:"valore"`
	Vecchia            string `json:"vecchia"`
	ProvenienzaVecchia string `json:"provenienza_vecchia"`
	Nuova              string `json:"nuova"`
	Motivo             string `json:"motivo,omitempty"`
}

// badgeDi: il badge di un file e il suo motivo, con la tavola del 6.4.6 (R31 d A), nell'ordine:
//  1. file deciso (un documento confermato lo porta: vedi deciso) e il motore non propone il componente del documento
//     fra i candidati, oppure gli dà un'altra base, oppure dice il file fuori richiesta, oppure non risponde:
//     regressione_su_confermata;
//  2. file deciso e il componente del documento fra i candidati con la stessa base: uguale;
//  3. file non deciso e collocazione fuori_richiesta: fuori_richiesta;
//  4. vecchio senza destinazione (nessun componente, nessuna proposta «assegna», nessun candidato F8, proposta non
//     scartata) e nuovo con almeno un candidato: nuovo_ancoraggio;
//  5. stessa base e stesso target (le destinazioni della riga 4: il componente, l'«assegna», i candidati F8), oppure
//     tutti e due senza ancoraggio con la stessa base: uguale;
//  6. tutto il resto, compresa una proposta scartata a cui il motore dà un candidato e un codice registrato non
//     leggibile: diverso, con il motivo.
//
// La base vecchia è sempre Vecchio.Base, letta da valutazione con la grammatica, mai la stringa; una base vuota non è
// mai uguale a niente. I target si confrontano solo come riferimenti scritti nella stessa forma (componente:<uuid> per
// un componente): nessuna inferenza fra forme diverse.
func badgeDi(f File) (Badge, string) {
	v, n := f.Vecchio, f.Nuovo
	if deciso(v) {
		if m := regressione(v, n); m != "" {
			return BadgeRegressioneSuConfermata, m
		}
		return BadgeUguale, ""
	}
	if n.Collocazione == collocazioneFuoriRichiesta {
		return BadgeFuoriRichiesta, ""
	}
	if senzaDestinazione(v) && len(n.Candidati) > 0 {
		return BadgeNuovoAncoraggio, ""
	}
	if stessaBaseEStessoTarget(v, n) || senzaAncoraggioStessaBase(v, n) {
		return BadgeUguale, ""
	}
	return BadgeDiverso, motivoDiverso(v, n)
}

// deciso: un documento confermato porta il file (Vecchio.Documento, dalla provenienza). È la stessa definizione di
// valutazione, che da lì prende il codice e il componente del vecchio (registro §10.2; R-43). Lo stato della proposta
// non conta: il documento nasce solo da un gesto di conferma (confermato_da), anche quando la proposta è rimasta
// aperta o non c'è; una proposta confermata o duplicata senza il documento in provenienza non è decisa. Un documento
// poi sostituito (SostituitoDa) resta la decisione su questo file.
func deciso(v Vecchio) bool {
	return v.Documento != nil
}

// regressione: il motivo per cui un file deciso è una regressione, "" se il motore propone il componente del
// documento con la stessa base (riga 2). L'ordine dei motivi va dal più grave: il motore che non risponde prima del
// componente che non c'è, poi la base.
func regressione(v Vecchio, n Nuovo) string {
	switch {
	case !n.Valutato:
		return MotivoNonValutato
	case v.Componente == nil:
		return MotivoDocumentoSenzaComponente
	case n.Collocazione == collocazioneFuoriRichiesta:
		return MotivoFuoriRichiesta
	case len(n.Candidati) == 0:
		return MotivoNessunCandidato
	}
	target := rifComponente(*v.Componente)
	proposto := false
	for _, c := range n.Candidati {
		if c.Target != target {
			continue
		}
		proposto = true
		if v.Base != "" && c.Base == v.Base {
			return ""
		}
	}
	switch {
	case !proposto && stessaBaseAltrove(v, n):
		return MotivoStessaBaseAltroTarget
	case !proposto:
		return MotivoComponenteNonProposto
	case v.Base == "":
		return MotivoCodiceRegistratoNonLeggibile
	default:
		return MotivoBaseDiversa
	}
}

// stessaBaseAltrove: un candidato del nuovo ha la base vecchia, ma non è il componente deciso (T-B6-51). Il badge resta
// una regressione: contare uguale un altro target con la stessa base sarebbe un legame che nessuno ha deciso (R31). Il
// motivo serve solo a leggere «dopo» nel rapporto.
func stessaBaseAltrove(v Vecchio, n Nuovo) bool {
	if v.Base == "" {
		return false
	}
	for _, c := range n.Candidati {
		if c.Base == v.Base {
			return true
		}
	}
	return false
}

// targetVecchi: le destinazioni del vecchio, nella forma dei riferimenti: il componente del documento, il componente
// della proposta («assegna») e le chiavi F8, le stesse che guarda la riga 4 (R-43). Una proposta scartata non ha
// destinazioni: lo scarto è la decisione.
func targetVecchi(v Vecchio) map[string]bool {
	out := map[string]bool{}
	if v.Stato == statoScartata {
		return out
	}
	if v.Componente != nil {
		out[rifComponente(*v.Componente)] = true
	}
	if v.ComponenteProposta != nil {
		out[rifComponente(*v.ComponenteProposta)] = true
	}
	for _, d := range v.Destinazione {
		out[d] = true
	}
	return out
}

// senzaDestinazione (riga 4): nessun componente, né del documento né della proposta, nessun candidato F8, e la
// proposta non è scartata (la riga 6 nomina la proposta scartata a cui il motore dà un candidato: è diverso).
func senzaDestinazione(v Vecchio) bool {
	return v.Stato != statoScartata && v.Componente == nil && v.ComponenteProposta == nil && len(v.Destinazione) == 0
}

// stessaBaseEStessoTarget (riga 5, prima parte): un candidato del nuovo, valutato, ha un target del vecchio e la base
// vecchia.
func stessaBaseEStessoTarget(v Vecchio, n Nuovo) bool {
	if v.Base == "" || !n.Valutato {
		return false
	}
	vecchi := targetVecchi(v)
	for _, c := range n.Candidati {
		if vecchi[c.Target] && c.Base == v.Base {
			return true
		}
	}
	return false
}

// senzaAncoraggioStessaBase (riga 5, seconda parte): né il vecchio né il nuovo, valutato, ancorano il file, e la base è
// la stessa: la base vecchia letta è l'unica base delle letture d'identità del nuovo. Se il vecchio non ha nessun codice
// e il nuovo non legge nessuna base, i due lati dicono la stessa cosa (nessuna base): uguale. Un codice registrato che
// non si legge non è mai uguale (riga 6), e nemmeno un file che il motore non ha valutato.
func senzaAncoraggioStessaBase(v Vecchio, n Nuovo) bool {
	if !n.Valutato || len(targetVecchi(v)) > 0 || len(n.Candidati) > 0 {
		return false
	}
	if v.Base != "" {
		return len(n.Basi) == 1 && n.Basi[0] == v.Base
	}
	return v.Codice == "" && len(n.Basi) == 0
}

// motivoDiverso: il motivo del badge diverso (riga 6), dal più specifico.
func motivoDiverso(v Vecchio, n Nuovo) string {
	vecchi := targetVecchi(v)
	switch {
	case !n.Valutato:
		return MotivoNonValutato
	case v.Stato == statoScartata && len(n.Candidati) > 0:
		return MotivoPropostaScartata
	case v.Codice != "" && !v.Leggibile:
		return MotivoCodiceRegistratoNonLeggibile
	case len(vecchi) > 0 && len(n.Candidati) == 0:
		return MotivoAncoraggioAssente
	}
	for _, c := range n.Candidati {
		if vecchi[c.Target] {
			return MotivoBaseDiversa // il target c'è, la base no
		}
	}
	if len(vecchi) > 0 {
		return MotivoTargetDiverso
	}
	return MotivoBaseDiversa
}

// indicatoreDi: l'indicatore di revisione di un file, e la diagnostica quando le revisioni non si confrontano.
//   - uguali e diverse danno uguale e diversa, senza diagnostica.
//   - non_confrontabili dà non_determinabile; la diagnostica solo se c'era qualcosa da confrontare (R-46): il file è
//     valutato, il vecchio ha un codice o una rev registrata, e almeno un lato porta una revisione (Vecchio.Revisione,
//     Vecchio.Rev, Nuovo.Revisione), oppure le letture del file ne danno di discordi. Senza revisioni da mettere a
//     fianco l'indicatore, con il motivo di valutazione, lo dice già, e una nota su ogni file perderebbe valore.
//   - Nuovo.Revisioni vuoto (valutazione non ha confrontato) dà non_determinabile, senza diagnostica.
//   - Un valore fuori vocabolario dà non_determinabile, sempre con la diagnostica: mai «uguale» per difetto.
func indicatoreDi(f File) (IndicatoreRevisione, *evidenze.Diagnostica) {
	ind := IndicatoreRevisione{Vecchia: f.Vecchio.Revisione, ProvenienzaVecchia: f.Vecchio.RevisioneDa, Nuova: f.Nuovo.Revisione,
		Motivo: f.Nuovo.MotivoRevisioni}
	msg := ""
	switch f.Nuovo.Revisioni {
	case revisioniUguali:
		ind.Valore = RevisioneUguale
		return ind, nil
	case revisioniDiverse:
		ind.Valore = RevisioneDiversa
		return ind, nil
	case "":
		ind.Valore = RevisioneNonDeterminabile
		return ind, nil
	case revisioniNonConfrontabili:
		ind.Valore = RevisioneNonDeterminabile
		v, n := f.Vecchio, f.Nuovo
		unaRevisione := v.Revisione != "" || v.Rev != "" || n.Revisione != "" || n.MotivoRevisioni == motivoRevisioniNuoveDiscordi
		if !n.Valutato || (v.Codice == "" && v.Rev == "") || !unaRevisione {
			return ind, nil
		}
		msg = "revisioni non confrontabili: l'indicatore è non_determinabile, mai una regressione"
	default:
		ind.Valore = RevisioneNonDeterminabile
		msg = "valore delle revisioni fuori vocabolario (" + f.Nuovo.Revisioni + "): l'indicatore è non_determinabile"
	}
	if ind.Motivo != "" {
		msg += " (" + ind.Motivo + ")"
	}
	return ind, &evidenze.Diagnostica{
		Codice:    CodiceRevisioneNonConfrontabile,
		Gravita:   evidenze.GravitaNota,
		Natura:    evidenze.NaturaDati,
		Percorso:  "file[" + f.AllegatoID.String() + "].revisione",
		Messaggio: msg,
		Rif:       []string{f.AllegatoID.String()},
	}
}

// rifComponente: il riferimento di un componente nella forma dei target (componente:<uuid>).
func rifComponente(id uuid.UUID) string { return prefissoRifComponente + id.String() }
