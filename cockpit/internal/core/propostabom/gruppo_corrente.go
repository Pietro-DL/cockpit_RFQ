package propostabom

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/valutazione"
)

// Il gruppo corrente della sorgente (V1: emendamento di V1 §6.4.1, la regola di O-01, con il secondo, il terzo e il
// quarto giro, che non cambia la regola): la stessa struttura di uno STEP può cambiare gruppo senza cambiare (alla prima
// «Autorizza» passa da una candidata al gruppo «bom»; «Revoca» e il cambio di tipo la riportano fra le candidate, la
// sostituzione dello STEP autorizzato solo con una radice compatibile: con una radice di riserva di solito la struttura non
// è più proposta, e il gruppo corrente è assente). Una bozza la ritrova per occorrenza, sha256 e radice, mai per la sola
// chiave. La stessa funzione serve alla striscia della pagina (/dati, bozza_del_prodotto), a R1 ?confronto=1, all'avviso
// PROPOSTA_CAMBIATA di R4 e a riallinea_proposta.

// I modi del gruppo corrente (GruppoCorrente.Come).
const (
	ComeStessaOccorrenza = "stessa_occorrenza"
	ComeAltraOccorrenza  = "altra_occorrenza"
	ComeAssente          = "assente"
)

// I motivi di un gruppo corrente assente (GruppoCorrente.Motivo; terzo giro su O-01, OT-08):
//   - nessun_gruppo: con la radice della sorgente nessun gruppo delle occorrenze attive ha la struttura, e nessuna BOM
//     di lavoro con la stessa struttura sta su un'occorrenza non attiva: il motore non propone più questa struttura;
//   - occorrenza_non_attiva: la BOM di lavoro con la stessa struttura sta su un'occorrenza in «Altri file» o scartata,
//     oppure la candidata della propria occorrenza sta su quell'occorrenza non attiva (decisione P-4 (a) e (b)
//     dell'ondata A di V1); GruppoCorrente.AllegatoID è il suo allegato: prima di seguirla serve «Riattiva»;
//   - senza_radice: la bozza non ha la radice della sorgente e il ripiego per chiave non la dà (la chiave «bom», o una
//     chiave «cand:…» che non c'è più): la regola non si applica. Solo per una bozza senza registro venuta dal formato 2.
const (
	MotivoNessunGruppo        = "nessun_gruppo"
	MotivoOccorrenzaNonAttiva = "occorrenza_non_attiva"
	MotivoSenzaRadice         = "senza_radice"
)

// NonAttivi: le occorrenze della RFQ in «Altri file» o scartate, con il loro stato, come le dà il ciclo dei file
// (emendamento di V1 §10.8); vuota o nil = tutte attive.
type NonAttivi map[uuid.UUID]string

// SorgenteBozza: i tre dati della bozza con cui si ritrova il gruppo corrente.
//   - AllegatoID, Sha256: l'occorrenza e il contenuto della sorgente (sorgente.allegato_id e sorgente.sha256; con un
//     registro sono anche fonte_allegato_id e fonte_sha256).
//   - Radice: fonte_radice del registro della versione (gruppo.radice, il RifNodo della radice); senza registro il rif
//     della casella del nodo del prodotto; "" quando il nodo del prodotto non ha caselle (una bozza venuta dal formato
//     2): allora vale il ripiego per chiave.
//   - Gruppo: sorgente.gruppo della versione. Serve solo al ripiego per chiave, e non si scompone mai.
type SorgenteBozza struct {
	AllegatoID uuid.UUID
	Sha256     string
	Radice     string
	Gruppo     string
}

// GruppoCorrente: il gruppo corrente della sorgente, {chiave?, come, allegato_id?, motivo?}: Chiave con
// stessa_occorrenza e altra_occorrenza; AllegatoID con altra_occorrenza (l'allegato della BOM di lavoro) e con il motivo
// occorrenza_non_attiva; Motivo solo con assente.
type GruppoCorrente struct {
	Chiave     string     `json:"chiave,omitempty"`
	Come       string     `json:"come"`
	AllegatoID *uuid.UUID `json:"allegato_id,omitempty"`
	Motivo     string     `json:"motivo,omitempty"`
}

// GruppoCorrenteDelProdotto: il gruppo corrente della sorgente fra i gruppi del prodotto della bozza
// (GruppiDelProdotto): un gruppo di un altro prodotto non vale mai, anche con la stessa chiave.
func GruppoCorrenteDelProdotto(pv valutazione.ProdottoValutato, strutture []ancoraggio.StrutturaProdotto, nonAttivi NonAttivi,
	s SorgenteBozza) GruppoCorrente {
	return GruppoCorrenteDellaSorgente(GruppiDelProdotto(pv, strutture), nonAttivi, s)
}

// GruppoCorrenteDellaSorgente: il gruppo corrente della sorgente fra i gruppi del prodotto della bozza, come li calcola
// adesso il motore (GruppiDelProdotto), sulle occorrenze attive: una candidata su un'occorrenza non attiva non c'è, come
// in /dati; la BOM di lavoro resta anche su un portatore non attivo, perché la fotografia del motore non è filtrata. La
// radice è quella della sorgente oppure, senza, quella del ripiego per chiave. Poi, in quest'ordine, al primo che vale:
//  1. la candidata della stessa occorrenza, con lo stesso allegato, sha256 e radice: la sua chiave, stessa_occorrenza;
//  2. la BOM di lavoro con lo stesso allegato, sha256 e radice: «bom», stessa_occorrenza;
//  3. la BOM di lavoro con lo stesso sha256 e la stessa radice su un altro allegato (il portatore): «bom»,
//     altra_occorrenza, con il suo allegato;
//  4. altrimenti assente: occorrenza_non_attiva se la BOM di lavoro con la stessa struttura sta su un'occorrenza non
//     attiva, o se la candidata della propria occorrenza (lo stesso allegato, sha256 e radice) sta su un'occorrenza non
//     attiva (P-4 (b)), con il suo allegato; nessun_gruppo se no.
//
// Una candidata di un'altra occorrenza non vale mai: con la stessa radice candidata sulla propria copia e la BOM di lavoro
// sull'altra vale la candidata. Nessuna chiave si scompone: la chiave del risultato è quella del gruppo, e il ripiego
// confronta la chiave intera. È pura e deterministica, e non cambia gli ingressi.
func GruppoCorrenteDellaSorgente(gruppi []Gruppo, nonAttivi NonAttivi, s SorgenteBozza) GruppoCorrente {
	nonAttivo := func(g Gruppo) bool {
		if g.AllegatoID == nil {
			return false
		}
		_, ok := nonAttivi[*g.AllegatoID]
		return ok
	}
	visibili := make([]Gruppo, 0, len(gruppi))
	for _, g := range gruppi {
		if g.Tipo == GruppoCandidata && nonAttivo(g) {
			continue
		}
		visibili = append(visibili, g)
	}
	radice := s.Radice
	if radice == "" {
		if radice = radiceDalRipiego(visibili, s); radice == "" {
			return GruppoCorrente{Come: ComeAssente, Motivo: MotivoSenzaRadice}
		}
	}
	for _, g := range visibili {
		if g.Tipo == GruppoCandidata && stessaStruttura(g, s.Sha256, radice) && *g.AllegatoID == s.AllegatoID {
			return GruppoCorrente{Chiave: g.Chiave, Come: ComeStessaOccorrenza}
		}
	}
	for _, g := range visibili {
		if g.Tipo != GruppoBOM || !stessaStruttura(g, s.Sha256, radice) {
			continue
		}
		id := *g.AllegatoID
		switch {
		case nonAttivo(g):
			return GruppoCorrente{Come: ComeAssente, AllegatoID: &id, Motivo: MotivoOccorrenzaNonAttiva}
		case id == s.AllegatoID:
			return GruppoCorrente{Chiave: g.Chiave, Come: ComeStessaOccorrenza}
		default:
			return GruppoCorrente{Chiave: g.Chiave, Come: ComeAltraOccorrenza, AllegatoID: &id}
		}
	}
	for _, g := range gruppi {
		if g.Tipo == GruppoCandidata && nonAttivo(g) && stessaStruttura(g, s.Sha256, radice) && *g.AllegatoID == s.AllegatoID {
			id := *g.AllegatoID
			return GruppoCorrente{Come: ComeAssente, AllegatoID: &id, Motivo: MotivoOccorrenzaNonAttiva}
		}
	}
	return GruppoCorrente{Come: ComeAssente, Motivo: MotivoNessunGruppo}
}

// stessaStruttura: il gruppo è su uno STEP con quel contenuto e quella radice.
func stessaStruttura(g Gruppo, sha256, radice string) bool {
	return g.AllegatoID != nil && sha256 != "" && g.Sha256 == sha256 && g.Radice == radice
}

// radiceDalRipiego: il ripiego per chiave (terzo giro su O-01, OT-07). Vale solo per la chiave di una candidata: si cerca
// sorgente.gruppo, uguale come stringa, fra i gruppi visibili del prodotto; se è una candidata con lo sha256 della
// sorgente, la radice è la sua. La chiave «bom» non porta la radice (dopo una nuova autorizzazione dello stesso STEP con
// un'altra radice il gruppo «bom» ha lo stesso sha256 e un'altra struttura), e una chiave che non c'è più non la dà: "".
func radiceDalRipiego(visibili []Gruppo, s SorgenteBozza) string {
	if s.Gruppo == "" || s.Sha256 == "" {
		return ""
	}
	for _, g := range visibili {
		if g.Chiave != s.Gruppo {
			continue
		}
		if g.Tipo == GruppoCandidata && g.Sha256 == s.Sha256 {
			return g.Radice
		}
		return ""
	}
	return ""
}
