package distintabozza

// I corpi delle richieste di R2 e R3 nel formato 3 (EM §9.2, §9.3): la lettura stretta che il transport fa prima di
// chiamare il servizio. Le regole di tutto il formato 3: UTF-8 valido, nessuna chiave ripetuta, nomi in minuscolo, numeri
// interi senza esponente, nessun campo sconosciuto, niente dopo il valore. Ogni violazione e' un CORPO_NON_VALIDO con il
// campo (422, mai 5xx).
//
// R2 ha una e una sola di cinque forme (salvataggio, crea, cambia_sorgente, riallinea_proposta, converti_da), con
// prodotto_id, versione_attesa, vocabolario 2 e chiave_salvataggio (UUID v4) sempre (EM §9.2). La versione attesa, forma
// per forma: salvataggio, cambia_sorgente e riallinea_proposta portano la versione della bozza (bozza_id obbligatorio);
// crea e converti_da portano 0 e bozza_id null (la versione annidata e' quella della bozza da abbandonare o da
// convertire). Direttiva 35, blocco C-3 (R2 passo 6b): le quattro forme costruite dal server si leggono qui per intero;
// i motivi obbligatori che mancano sono 422 MOTIVO_OBBLIGATORIO.

import (
	"encoding/json"
	"fmt"
	"strings"

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

// Le scelte del corpo di riallinea_proposta e cambia_sorgente (EM §6.5, §6.6; U2 §5.2): togli e tieni_senza_casella per un
// nodo la cui casella non c'e' piu' nel registro nuovo (togli anche per una non_includere su un rif sparito, con
// decisione_id), recepisci e mantieni per un nodo proposta la cui identita' proposta cambia.
const (
	SceltaTogli             = "togli"
	SceltaTieniSenzaCasella = "tieni_senza_casella"
	SceltaRecepisci         = "recepisci"
	SceltaMantieni          = "mantieni"
)

var scelteAmmesse = map[string]bool{SceltaTogli: true, SceltaTieniSenzaCasella: true, SceltaRecepisci: true, SceltaMantieni: true}

// RichiestaR2 e' un corpo di R2 letto: la forma e, per il salvataggio, il Salvataggio3 senza RFQ e utente (li mette chi
// chiama dal percorso e dalla sessione); per le forme costruite dal server i campi comuni (Comune) e il corpo della forma.
type RichiestaR2 struct {
	Forma       string
	Salvataggio Salvataggio3

	Comune         ComuneR2
	Crea           *CreaR2
	CambiaSorgente *CambiaSorgenteR2
	Riallinea      *RiallineaR2
	Converti       *ConvertiR2
}

// ComuneR2: i campi di primo livello di una forma costruita dal server. BozzaID e' uuid.Nil per crea e converti_da.
type ComuneR2 struct {
	ProdottoID        uuid.UUID
	BozzaID           uuid.UUID
	VersioneAttesa    int
	ChiaveSalvataggio uuid.UUID
	// ImprontaRichiesta: lo sha256 della forma canonica del corpo (il reinvio con lo stesso corpo e' lo stesso atto).
	ImprontaRichiesta string
}

// SorgenteDichiarata: la sorgente che il browser dichiara in crea (partenza step) e cambia_sorgente: l'occorrenza, il suo
// sha256 (facoltativo: se c'e' si confronta con quello dell'allegato) e il gruppo opaco di /dati o di S6, mai scomposto.
type SorgenteDichiarata struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Sha256     string    `json:"sha256,omitempty"`
	Gruppo     string    `json:"gruppo"`
}

// CreaR2: il corpo della forma crea (EM §9.2): la partenza, la sorgente e l'impronta del contenuto per la partenza step
// (V-7), il motivo della scelta della fonte, l'abbandono della bozza aperta (la ripartenza, EM §6.8).
type CreaR2 struct {
	Partenza          string
	Sorgente          *SorgenteDichiarata
	ImprontaContenuto string
	Motivo            string
	Abbandona         *AbbandonaR2
	// PresaDAttoAnalisi: per la partenza solo_prodotto (solo da S8), lo stato dell'analisi dello STEP di cui l'ingegnere
	// ha preso atto; va nell'evento bozza_aperta (direttiva 39 §3).
	PresaDAttoAnalisi string
}

// AbbandonaR2: la bozza aperta da abbandonare nella stessa transazione della creazione, con la sua versione e il motivo.
type AbbandonaR2 struct {
	BozzaID        uuid.UUID
	VersioneAttesa int
	Motivo         string
}

// CambiaSorgenteR2: il corpo di cambia_sorgente (EM §6.6): la sorgente nuova, l'impronta del contenuto che il browser ha
// visto (V-7), il motivo (obbligatorio) e, fra due copie dello stesso STEP, le scelte recepisci / mantieni (base B di
// V1-Q30).
type CambiaSorgenteR2 struct {
	Sorgente          SorgenteDichiarata
	ImprontaContenuto string
	Motivo            string
	Scelte            []SceltaR2
}

// RiallineaR2: il corpo di riallinea_proposta (EM §6.5): l'impronta del contenuto del gruppo corrente (V-7), il motivo e
// le scelte dell'ingegnere.
type RiallineaR2 struct {
	ImprontaContenuto string
	Motivo            string
	Scelte            []SceltaR2
}

// SceltaR2: una scelta del corpo, su un nodo (chiave_bozza) o su una decisione (decisione_id), mai tutti e due.
type SceltaR2 struct {
	ChiaveBozza string `json:"chiave_bozza,omitempty"`
	DecisioneID string `json:"decisione_id,omitempty"`
	Scelta      string `json:"scelta"`
}

// ConvertiR2: il corpo di converti_da (EM §6.7): la bozza formato 2 aperta, la sua versione, il motivo.
type ConvertiR2 struct {
	BozzaID        uuid.UUID
	VersioneAttesa int
	Motivo         string
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
	impronta := jsoncanonico.Impronta(can)
	if out.Forma != FormaSalvataggio {
		if len(in.Azioni) > 0 || !nullo(in.Rebase) {
			return out, corpo("richiesta", "la forma %s non ha azioni ne' rebase: la versione la costruisce il server", out.Forma)
		}
		out.Comune = ComuneR2{ProdottoID: *in.ProdottoID, VersioneAttesa: *in.VersioneAttesa, ChiaveSalvataggio: *in.ChiaveSalvataggio,
			ImprontaRichiesta: impronta}
		if e := leggiFormaDelServer(&out, in); e != nil {
			return out, e
		}
		return out, nil
	}
	if !presenteValore(in.BozzaID) {
		return out, corpo("bozza_id", "il salvataggio e' di una bozza aperta: manca bozza_id")
	}
	var bozza uuid.UUID
	if err := json.Unmarshal(in.BozzaID, &bozza); err != nil || bozza == uuid.Nil {
		return out, corpo("bozza_id", "bozza_id e' un UUID")
	}
	s := Salvataggio3{ProdottoID: *in.ProdottoID, BozzaID: bozza, VersioneAttesa: *in.VersioneAttesa,
		ChiaveSalvataggio: *in.ChiaveSalvataggio, ImprontaRichiesta: impronta, Snapshot: in.Snapshot, Azioni: in.Azioni}
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

// leggiFormaDelServer: i campi di primo livello e il corpo di una forma costruita dal server.
func leggiFormaDelServer(out *RichiestaR2, in richiestaR2In) *Errore {
	conBozza := out.Forma == FormaCambiaSorgente || out.Forma == FormaRiallineaProposta
	if conBozza {
		var bozza uuid.UUID
		if !presenteValore(in.BozzaID) || json.Unmarshal(in.BozzaID, &bozza) != nil || bozza == uuid.Nil {
			return corpo("bozza_id", "la forma %s e' di una bozza aperta: bozza_id e' il suo UUID", out.Forma)
		}
		if out.Comune.VersioneAttesa < 1 {
			return corpo("versione_attesa", "la forma %s porta la versione corrente della bozza", out.Forma)
		}
		out.Comune.BozzaID = bozza
	} else {
		if !nullo(in.BozzaID) {
			return corpo("bozza_id", "la forma %s crea una bozza nuova: bozza_id e' null", out.Forma)
		}
		if out.Comune.VersioneAttesa != 0 {
			return corpo("versione_attesa", "la forma %s crea una bozza nuova: versione_attesa e' 0 (la versione della bozza di prima e' nel corpo della forma)", out.Forma)
		}
	}
	switch out.Forma {
	case FormaCrea:
		c, e := leggiCrea(in.Crea)
		out.Crea = c
		return e
	case FormaCambiaSorgente:
		c, e := leggiCambiaSorgente(in.CambiaSorgente)
		out.CambiaSorgente = c
		return e
	case FormaRiallineaProposta:
		r, e := leggiRiallinea(in.RiallineaProposta)
		out.Riallinea = r
		return e
	default:
		c, e := leggiConverti(in.ConvertiDa)
		out.Converti = c
		return e
	}
}

type sorgenteDichiarataIn struct {
	AllegatoID *uuid.UUID `json:"allegato_id"`
	Sha256     *string    `json:"sha256"`
	Gruppo     *string    `json:"gruppo"`
}

func (s *sorgenteDichiarataIn) converti(campo string) (SorgenteDichiarata, *Errore) {
	var out SorgenteDichiarata
	if s == nil {
		return out, corpo(campo, "manca la sorgente: {allegato_id, gruppo, sha256?}")
	}
	if s.AllegatoID == nil || *s.AllegatoID == uuid.Nil {
		return out, corpo(campo+".allegato_id", "la sorgente vuole l'allegato dello STEP")
	}
	if s.Gruppo == nil || !testoLibero(*s.Gruppo, MaxGruppo) {
		return out, corpo(campo+".gruppo", "il gruppo e' obbligatorio, al piu' %d caratteri, senza caratteri di controllo", MaxGruppo)
	}
	if *s.Gruppo == "senza" || *s.Gruppo == GruppoSoloProdotto {
		return out, corpoMotivo(campo+".gruppo", MotivoGruppoNonImportabile, "il gruppo «%s» non si importa", *s.Gruppo)
	}
	out.AllegatoID, out.Gruppo = *s.AllegatoID, *s.Gruppo
	if s.Sha256 != nil {
		if !reSha256.MatchString(*s.Sha256) {
			return out, corpo(campo+".sha256", "lo sha256 e' di 64 cifre esadecimali minuscole")
		}
		out.Sha256 = *s.Sha256
	}
	return out, nil
}

// motivoDichiarato: un motivo facoltativo nella forma dei testi umani (NFC, 1-1000 caratteri senza controlli).
func motivoDichiarato(campo string, m *string) (string, *Errore) {
	if m == nil {
		return "", nil
	}
	x := strings.TrimSpace(nfc(*m))
	if x == "" {
		return "", nil
	}
	if !testoLibero(x, MaxMotivo) {
		return "", corpo(campo, "il motivo sta in %d caratteri, senza caratteri di controllo", MaxMotivo)
	}
	return x, nil
}

func motivoObbligatorio(campo string, m *string, cosa string) (string, *Errore) {
	x, e := motivoDichiarato(campo, m)
	if e != nil {
		return "", e
	}
	if x == "" {
		return "", &Errore{Codice: CodiceMotivoObbligatorio, Campo: campo, Messaggio: cosa + " vuole il motivo"}
	}
	return x, nil
}

func improntaDichiarata(campo string, s *string) (string, *Errore) {
	if s == nil || !reSha256.MatchString(*s) {
		return "", corpo(campo, "l'impronta del contenuto che il browser ha visto (V-7) e' di 64 cifre esadecimali minuscole")
	}
	return *s, nil
}

func leggiCrea(raw json.RawMessage) (*CreaR2, *Errore) {
	var in struct {
		Partenza          *string               `json:"partenza"`
		Sorgente          *sorgenteDichiarataIn `json:"sorgente"`
		ImprontaContenuto *string               `json:"impronta_contenuto"`
		Motivo            *string               `json:"motivo"`
		Abbandona         *struct {
			BozzaID        *uuid.UUID `json:"bozza_id"`
			VersioneAttesa *int       `json:"versione_attesa"`
			Motivo         *string    `json:"motivo"`
		} `json:"abbandona"`
	}
	if e := decodificaStretta("crea", raw, &in); e != nil {
		return nil, e
	}
	if in.Partenza != nil && *in.Partenza == PartenzaSoloProdotto {
		return nil, corpoMotivo("crea.partenza", MotivoPartenzaNonAmmessa,
			"la partenza solo_prodotto nasce solo con «Conferma questo STEP come sorgente» (S8), con la presa d'atto dello stato dell'analisi")
	}
	if in.Partenza == nil || !partenze[*in.Partenza] {
		return nil, corpo("crea.partenza", "la partenza e' una fra vuota, registrato, conferma e step")
	}
	c := &CreaR2{Partenza: *in.Partenza}
	var e *Errore
	if c.Partenza == PartenzaStep {
		s, e := in.Sorgente.converti("crea.sorgente")
		if e != nil {
			return nil, e
		}
		c.Sorgente = &s
		if c.ImprontaContenuto, e = improntaDichiarata("crea.impronta_contenuto", in.ImprontaContenuto); e != nil {
			return nil, e
		}
	} else if in.Sorgente != nil || in.ImprontaContenuto != nil {
		return nil, corpo("crea.sorgente", "sorgente e impronta_contenuto sono della sola partenza step: la partenza %s non importa", c.Partenza)
	}
	if c.Motivo, e = motivoDichiarato("crea.motivo", in.Motivo); e != nil {
		return nil, e
	}
	if a := in.Abbandona; a != nil {
		if a.BozzaID == nil || *a.BozzaID == uuid.Nil {
			return nil, corpo("crea.abbandona.bozza_id", "la ripartenza nomina la bozza aperta da abbandonare")
		}
		if a.VersioneAttesa == nil || *a.VersioneAttesa < 1 {
			return nil, corpo("crea.abbandona.versione_attesa", "la ripartenza porta la versione corrente della bozza da abbandonare")
		}
		m, e := motivoObbligatorio("crea.abbandona.motivo", a.Motivo, "l'abbandono della bozza nella ripartenza")
		if e != nil {
			return nil, e
		}
		c.Abbandona = &AbbandonaR2{BozzaID: *a.BozzaID, VersioneAttesa: *a.VersioneAttesa, Motivo: m}
	}
	return c, nil
}

func leggiScelte(campo string, in []SceltaR2) ([]SceltaR2, *Errore) {
	viste := map[string]bool{}
	out := make([]SceltaR2, 0, len(in))
	for i, s := range in {
		c := fmt.Sprintf("%s[%d]", campo, i)
		if !scelteAmmesse[s.Scelta] {
			return nil, corpo(c+".scelta", "scelta %q: togli, tieni_senza_casella, recepisci o mantieni", s.Scelta)
		}
		if (s.ChiaveBozza == "") == (s.DecisioneID == "") {
			return nil, corpo(c, "una scelta nomina un nodo (chiave_bozza) oppure una decisione (decisione_id)")
		}
		if s.ChiaveBozza != "" && !reChiave.MatchString(s.ChiaveBozza) {
			return nil, corpo(c+".chiave_bozza", "la chiave e' 1-64 caratteri fra lettere, cifre e . _ : -")
		}
		if s.DecisioneID != "" && (!reChiave.MatchString(s.DecisioneID) || s.Scelta != SceltaTogli) {
			return nil, corpo(c+".decisione_id", "su una decisione (una non_includere su un rif sparito) la sola scelta e' togli")
		}
		k := "n:" + s.ChiaveBozza
		if s.DecisioneID != "" {
			k = "d:" + s.DecisioneID
		}
		if viste[k] {
			return nil, corpo(c, "due scelte per lo stesso nodo o la stessa decisione")
		}
		viste[k] = true
		out = append(out, s)
	}
	return out, nil
}

func leggiCambiaSorgente(raw json.RawMessage) (*CambiaSorgenteR2, *Errore) {
	var in struct {
		Sorgente          *sorgenteDichiarataIn `json:"sorgente"`
		ImprontaContenuto *string               `json:"impronta_contenuto"`
		Motivo            *string               `json:"motivo"`
		Scelte            []SceltaR2            `json:"scelte"`
	}
	if e := decodificaStretta("cambia_sorgente", raw, &in); e != nil {
		return nil, e
	}
	s, e := in.Sorgente.converti("cambia_sorgente.sorgente")
	if e != nil {
		return nil, e
	}
	c := &CambiaSorgenteR2{Sorgente: s}
	if c.ImprontaContenuto, e = improntaDichiarata("cambia_sorgente.impronta_contenuto", in.ImprontaContenuto); e != nil {
		return nil, e
	}
	if c.Motivo, e = motivoObbligatorio("cambia_sorgente.motivo", in.Motivo, "il cambio della fonte"); e != nil {
		return nil, e
	}
	if c.Scelte, e = leggiScelte("cambia_sorgente.scelte", in.Scelte); e != nil {
		return nil, e
	}
	for i, x := range c.Scelte {
		if x.Scelta != SceltaRecepisci && x.Scelta != SceltaMantieni {
			return nil, corpo(fmt.Sprintf("cambia_sorgente.scelte[%d].scelta", i), "nel cambio della fonte le scelte sono recepisci o mantieni")
		}
	}
	return c, nil
}

func leggiRiallinea(raw json.RawMessage) (*RiallineaR2, *Errore) {
	var in struct {
		ImprontaContenuto *string    `json:"impronta_contenuto"`
		Motivo            *string    `json:"motivo"`
		Scelte            []SceltaR2 `json:"scelte"`
	}
	if e := decodificaStretta("riallinea_proposta", raw, &in); e != nil {
		return nil, e
	}
	r := &RiallineaR2{}
	var e *Errore
	if r.ImprontaContenuto, e = improntaDichiarata("riallinea_proposta.impronta_contenuto", in.ImprontaContenuto); e != nil {
		return nil, e
	}
	if r.Motivo, e = motivoObbligatorio("riallinea_proposta.motivo", in.Motivo, "il riallineamento della proposta"); e != nil {
		return nil, e
	}
	if r.Scelte, e = leggiScelte("riallinea_proposta.scelte", in.Scelte); e != nil {
		return nil, e
	}
	return r, nil
}

func leggiConverti(raw json.RawMessage) (*ConvertiR2, *Errore) {
	var in struct {
		BozzaID        *uuid.UUID `json:"bozza_id"`
		VersioneAttesa *int       `json:"versione_attesa"`
		Motivo         *string    `json:"motivo"`
	}
	if e := decodificaStretta("converti_da", raw, &in); e != nil {
		return nil, e
	}
	if in.BozzaID == nil || *in.BozzaID == uuid.Nil {
		return nil, corpo("converti_da.bozza_id", "la conversione nomina la bozza formato 2 aperta")
	}
	if in.VersioneAttesa == nil || *in.VersioneAttesa < 1 {
		return nil, corpo("converti_da.versione_attesa", "la conversione porta la versione corrente della bozza formato 2")
	}
	m, e := motivoObbligatorio("converti_da.motivo", in.Motivo, "la conversione")
	if e != nil {
		return nil, e
	}
	return &ConvertiR2{BozzaID: *in.BozzaID, VersioneAttesa: *in.VersioneAttesa, Motivo: m}, nil
}

// Conversione3Da: la richiesta del servizio della conversione (Converti) da un corpo di R2 letto, con la RFQ del percorso
// e l'utente della sessione.
func (r RichiestaR2) Conversione3Da(thread, utente uuid.UUID) Conversione3 {
	c := Conversione3{ThreadID: thread, ProdottoID: r.Comune.ProdottoID, UtenteID: utente, ChiaveSalvataggio: r.Comune.ChiaveSalvataggio,
		ImprontaRichiesta: r.Comune.ImprontaRichiesta}
	if r.Converti != nil {
		c.BozzaID, c.VersioneAttesa, c.Motivo = r.Converti.BozzaID, r.Converti.VersioneAttesa, r.Converti.Motivo
	}
	return c
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
