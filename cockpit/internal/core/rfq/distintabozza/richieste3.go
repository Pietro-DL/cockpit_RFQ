package distintabozza

// I corpi delle richieste di R2 e R3 nel formato 3 (EM §9.2, §9.3): la lettura stretta che il transport fa prima di
// chiamare il servizio. Le regole di tutto il formato 3: UTF-8 valido, nessuna chiave ripetuta, nomi in minuscolo, numeri
// interi senza esponente, nessun campo sconosciuto, niente dopo il valore. Ogni violazione e' un CORPO_NON_VALIDO con il
// campo (422, mai 5xx).
//
// R2 ha una e una sola di cinque forme (salvataggio, crea, cambia_sorgente, riallinea_proposta, converti_da), con
// prodotto_id, versione_attesa, vocabolario 2 e chiave_salvataggio (UUID v4) sempre. In questa tappa (V1, ondata B) il
// server fa la forma salvataggio; le altre quattro rispondono 422 CORPO_NON_VALIDO, motivo forma_non_disponibile, finche'
// non arrivano con l'ondata C (crea, cambia_sorgente e riallinea_proposta vogliono l'adattatore del Motore A).

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Le cinque forme di R2 (EM §9.2).
const (
	FormaSalvataggio       = "salvataggio"
	FormaCrea              = "crea"
	FormaCambiaSorgente    = "cambia_sorgente"
	FormaRiallineaProposta = "riallinea_proposta"
)

// RichiestaR2 e' un corpo di R2 letto: la forma e, per il salvataggio, il Salvataggio3 senza RFQ e utente (li mette chi
// chiama dal percorso e dalla sessione).
type RichiestaR2 struct {
	Forma       string
	Salvataggio Salvataggio3
}

type richiestaR2In struct {
	BozzaID           json.RawMessage `json:"bozza_id"`
	VersioneAttesa    *int            `json:"versione_attesa"`
	ProdottoID        *uuid.UUID      `json:"prodotto_id"`
	Vocabolario       *int            `json:"vocabolario"`
	ChiaveSalvataggio *uuid.UUID      `json:"chiave_salvataggio"`
	Snapshot          json.RawMessage `json:"snapshot"`
	Azioni            []Azione        `json:"azioni"`
	Rebase            json.RawMessage `json:"rebase"`
	Crea              json.RawMessage `json:"crea"`
	CambiaSorgente    json.RawMessage `json:"cambia_sorgente"`
	RiallineaProposta json.RawMessage `json:"riallinea_proposta"`
	ConvertiDa        json.RawMessage `json:"converti_da"`
}

// LeggiRichiestaR2 legge il corpo di R2 (commento di testa). L'impronta della richiesta e' lo sha256 della sua forma
// canonica: il reinvio dello stesso corpo con la stessa chiave e' lo stesso salvataggio (EM §9.2).
func LeggiRichiestaR2(raw []byte) (RichiestaR2, error) {
	var out RichiestaR2
	var in richiestaR2In
	if e := decodificaStretta("richiesta", raw, &in); e != nil {
		return out, e
	}
	can, err := jsoncanonico.Codifica(json.RawMessage(raw))
	if err != nil {
		return out, corpo("richiesta", "il corpo non ha una forma canonica: %v", err)
	}
	forme := map[string]bool{FormaSalvataggio: !nullo(in.Snapshot), FormaCrea: !nullo(in.Crea), FormaCambiaSorgente: !nullo(in.CambiaSorgente),
		FormaRiallineaProposta: !nullo(in.RiallineaProposta), FormaConvertiDa: !nullo(in.ConvertiDa)}
	n := 0
	for f, c := range forme {
		if c {
			n++
			out.Forma = f
		}
	}
	if n != 1 {
		return out, corpo("richiesta", "il corpo ha una e una sola forma fra snapshot (salvataggio), crea, cambia_sorgente, riallinea_proposta e converti_da: ne ha %d", n)
	}
	if in.ProdottoID == nil || *in.ProdottoID == uuid.Nil {
		return out, corpo("prodotto_id", "manca il prodotto")
	}
	if in.VersioneAttesa == nil || *in.VersioneAttesa < 0 {
		return out, corpo("versione_attesa", "manca versione_attesa (0 per una bozza nuova)")
	}
	if in.Vocabolario == nil || *in.Vocabolario != VocabolarioFormato3 {
		return out, corpo("vocabolario", "il formato 3 si salva con il vocabolario %d", VocabolarioFormato3)
	}
	if in.ChiaveSalvataggio == nil || in.ChiaveSalvataggio.Version() != 4 {
		return out, corpo("chiave_salvataggio", "manca la chiave del salvataggio, un UUID v4")
	}
	if out.Forma != FormaSalvataggio {
		return out, corpoMotivo(out.Forma, MotivoFormaNonDisponibile,
			"la forma %s di R2 arriva con la tappa successiva di V1 (ondata C): qui si salva una bozza aperta (snapshot e azioni)", out.Forma)
	}
	if !presenteValore(in.BozzaID) {
		return out, corpo("bozza_id", "il salvataggio e' di una bozza aperta: manca bozza_id")
	}
	var bozza uuid.UUID
	if err := json.Unmarshal(in.BozzaID, &bozza); err != nil || bozza == uuid.Nil {
		return out, corpo("bozza_id", "bozza_id e' un UUID")
	}
	s := Salvataggio3{ProdottoID: *in.ProdottoID, BozzaID: bozza, VersioneAttesa: *in.VersioneAttesa,
		ChiaveSalvataggio: *in.ChiaveSalvataggio, ImprontaRichiesta: jsoncanonico.Impronta(can), Snapshot: in.Snapshot, Azioni: in.Azioni}
	if !nullo(in.Rebase) {
		var r RebaseRichiesta3
		if e := decodificaStretta("rebase", in.Rebase, &r); e != nil {
			return out, e
		}
		if e := r.Base.valida("rebase.base"); e != nil {
			return out, e
		}
		s.Rebase = &r
	}
	for i, a := range s.Azioni {
		if a.Azione == "" {
			return out, corpo(fmt.Sprintf("azioni[%d].azione", i), "manca l'azione")
		}
	}
	out.Salvataggio = s
	return out, nil
}

type richiestaR3In struct {
	VersioneAttesa *int    `json:"versione_attesa"`
	Motivo         *string `json:"motivo"`
}

// LeggiRichiestaR3 legge il corpo di R3: {versione_attesa, motivo?} (EM §9.3). Restituisce l'Abbandono senza RFQ, bozza e
// utente.
func LeggiRichiestaR3(raw []byte) (Abbandono, error) {
	var out Abbandono
	var in richiestaR3In
	if e := decodificaStretta("richiesta", raw, &in); e != nil {
		return out, e
	}
	if in.VersioneAttesa == nil || *in.VersioneAttesa < 1 {
		return out, corpo("versione_attesa", "manca versione_attesa (la versione corrente della bozza)")
	}
	out.VersioneAttesa = *in.VersioneAttesa
	if in.Motivo != nil {
		out.Motivo = *in.Motivo
	}
	return out, nil
}
