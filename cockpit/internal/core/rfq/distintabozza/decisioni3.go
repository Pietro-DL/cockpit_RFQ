package distintabozza

// Le 16 decisioni del formato 3 (EM §3.6, §3.6.1, §3.6.2, §7.1, §7.2), con una forma chiusa per tipo: i campi ammessi e
// quelli obbligatori di ogni tipo, letti campo per campo. L'NFC vale solo per i motivi (testi umani, prima dei limiti);
// l'identita' (codici, rev, emittenti, namespace) e le copie dal registro (la lettura compatta, il segnale visto) restano
// byte per byte (EM §3.10, prova N-01). Le regole che vogliono il database o i registri (V-1...V-6, la regola unica
// dell'emittente, l'appartenenza degli UUID alla RFQ) le controlla chi chiama.
//
// Messaggio 46 [U] (21/10): il motivo scritto dall'utente si toglie da ogni widget. Nessuna decisione lo vuole piu':
// motivo e' facoltativo in mono_pezzo, escludi, ricodifica, nella correzione dell'identita', nell'adozione del codice
// manuale e nella conferma della categoria; se la decisione lo porta (i client di prima) vale il suo, e dove lo schema
// vuole un motivo lo scrive R5 (conferma_strutturale.mono_pezzo_motivo: la traccia del gesto). non_includere e'
// l'eccezione [T]: lo snapshot confermato deve portare motivo_codice e motivo uguali alle righe di
// conferma_strutturale_non_incluso (0023, controllo (iii)), quindi restano obbligatori nella decisione e li scrive la
// pagina come traccia del gesto (motivo_codice «altro», la scelta da un vocabolario chiuso che la pagina presentava come
// «motivo»), mai come testo dell'utente.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I 16 tipi delle decisioni del formato 3: i 13 del contratto rev. 3 e i tre nuovi (EM §3.6).
const (
	DecisioneD1AParticolare      = "d1_a_particolare"
	DecisioneD1RestaSottoassieme = "d1_resta_sottoassieme"
	DecisioneD1ASottoassieme     = "d1_a_sottoassieme"
	DecisioneMonoPezzo           = "mono_pezzo"
	DecisioneTogliArco           = "togli_arco"
	DecisioneEscludi             = "escludi"
	DecisioneSenzaPadre          = "senza_padre"
	DecisioneRiusaEsistente      = "riusa_esistente"
	DecisioneRiusaPerCanonico    = "riusa_per_canonico"
	DecisioneRicodifica          = "ricodifica"
	DecisioneArcoCondiviso       = "arco_condiviso"
	DecisioneDaValutare          = "da_valutare"
	DecisioneCambiaTipo          = "cambia_tipo"
	DecisioneIdentita            = "identita"
	DecisioneNonIncludere        = "non_includere"
	DecisioneScollegaDocumento   = "scollega_documento"
	// DecisioneConfermaCategoria: il diciassettesimo tipo (U2 §6.4.2; evidenze3.go)
	DecisioneConfermaCategoria = "conferma_categoria"
)

// I modi della decisione identita e il riferimento al codice corretto a mano nella pagina vecchia (EM §3.6.1).
const (
	ModoAlternativa          = "alternativa"
	ModoCorrezione           = "correzione"
	RiferimentoManualeLegacy = "manuale_legacy"
)

// motiviNonIncludere: l'insieme chiuso di motivo_codice (EM §7.1).
var motiviNonIncludere = map[string]bool{"elemento_cad": true, "duplicato_cad": true, "non_articolo": true, "altro": true}

// formaDecisione: i campi obbligatori e facoltativi di un tipo, oltre a id e tipo (EM §3.6; contratto rev. 3 §3.3).
// cambia_tipo porta il tipo nuovo in tipo_nuovo: il contratto lo chiama «tipo», che e' gia' la chiave del tipo della
// decisione, e un oggetto JSON non ha due chiavi uguali (T-A1-02 del 13/10; proposta di emendamento del resoconto).
type formaDecisione struct{ obbligatori, facoltativi []string }

var formeDecisione = map[string]formaDecisione{
	DecisioneD1AParticolare:      {[]string{"chiave_bozza"}, []string{"motivo"}},
	DecisioneD1RestaSottoassieme: {[]string{"chiave_bozza"}, []string{"motivo"}},
	DecisioneD1ASottoassieme:     {[]string{"chiave_bozza"}, nil},
	DecisioneMonoPezzo:           {nil, []string{"motivo"}},
	DecisioneTogliArco:           {[]string{"padre_id", "figlio_id"}, nil},
	DecisioneEscludi:             {[]string{"componente_id"}, []string{"motivo"}},
	DecisioneSenzaPadre:          {[]string{"chiave_bozza"}, nil},
	DecisioneRiusaEsistente:      {[]string{"chiave_bozza", "componente_id"}, nil},
	DecisioneRiusaPerCanonico:    {[]string{"chiave_bozza", "componente_id"}, nil},
	DecisioneRicodifica:          {[]string{"componente_id", "codice_prima", "codice_dopo", "emittente_dopo"}, []string{"namespace_dopo", "motivo"}},
	DecisioneArcoCondiviso:       {[]string{"padre_id", "figlio_id", "qta", "prodotti"}, nil},
	DecisioneDaValutare:          {[]string{"chiave_bozza"}, nil},
	DecisioneCambiaTipo:          {[]string{"componente_id", "tipo_nuovo", "firma_effetto"}, nil},
	// evidenza_documentale: solo nella correzione (U2 §7.7, U2-P20)
	DecisioneIdentita: {[]string{"chiave_bozza", "modo", "identita"}, []string{"importazione", "chiave_lettura", "riferimento", "lettura", "motivo",
		"evidenza_documentale"}},
	// non_includere tiene motivo_codice e motivo obbligatori: il controllo (iii) di conferma_strutturale_contenuto (0023)
	// vuole le righe di conferma_strutturale_non_incluso uguali alle decisioni dello snapshot, e ck_csni_origine le vuole
	// non vuote; dal messaggio 46 [U] li scrive la pagina, non l'utente: motivo_codice «altro» e la traccia del gesto
	// («gesto di <sigla> da «Non includere nella BOM»: …») [T]
	DecisioneNonIncludere: {[]string{"casella", "motivo_codice", "motivo", "azione_id"}, []string{"importazione", "segnale_visto"}},
	// da_decisione: l'id della decisione escludi da cui nasce lo svincolo (PE-27 dell'ondata A, decisione [T]
	// dell'integratore, 18/10): «↺ Ripristina» toglie l'esclusione e gli svincoli che la citano nello stesso gesto
	// scollega_documento {allegato_id, componente_id}: lo svincolo di una AR attiva cita l'occorrenza e il componente,
	// stabili anche dopo una riconferma (direttiva 35 §1.1; provvisorio, base [T] D-4 A; blocco C-2). Con la risposta B
	// citerebbe l'associazione_id della riga di file_rfq_associazione
	DecisioneScollegaDocumento: {[]string{"allegato_id", "componente_id"}, []string{"motivo", "da_decisione"}},
	// conferma_categoria (U2 §6.4.2): ingresso e' un campo della decisione (strada (a) della revisione tecnica di U2)
	DecisioneConfermaCategoria: {[]string{"soggetto", "categoria", "esito", "origine_categoria", "ingresso"},
		[]string{"classe_proposta", "importazione", "da_conferma", "motivo"}},
}

var (
	reFirmaEffetto = regexp.MustCompile(`^[0-9a-f]{24,64}$`)
)

// Decisione3 e' una decisione formato 3. I campi sono quelli di tutti i tipi: la forma chiusa di ogni tipo dice quali
// ci sono (formeDecisione), e i vuoti non si scrivono.
type Decisione3 struct {
	ID            string                        `json:"id"`
	Tipo          string                        `json:"tipo"`
	ChiaveBozza   string                        `json:"chiave_bozza,omitempty"`
	ComponenteID  *uuid.UUID                    `json:"componente_id,omitempty"`
	PadreID       *uuid.UUID                    `json:"padre_id,omitempty"`
	FiglioID      *uuid.UUID                    `json:"figlio_id,omitempty"`
	Qta           int                           `json:"qta,omitempty"`
	Prodotti      []uuid.UUID                   `json:"prodotti,omitempty"`
	CodicePrima   string                        `json:"codice_prima,omitempty"`
	CodiceDopo    string                        `json:"codice_dopo,omitempty"`
	EmittenteDopo string                        `json:"emittente_dopo,omitempty"`
	NamespaceDopo string                        `json:"namespace_dopo,omitempty"`
	TipoNuovo     string                        `json:"tipo_nuovo,omitempty"`
	FirmaEffetto  string                        `json:"firma_effetto,omitempty"`
	Modo          string                        `json:"modo,omitempty"`
	Importazione  string                        `json:"importazione,omitempty"`
	ChiaveLettura string                        `json:"chiave_lettura,omitempty"`
	Riferimento   string                        `json:"riferimento,omitempty"`
	Lettura       *importazione.LetturaCompatta `json:"lettura,omitempty"`
	Identita      *IdentitaScelta               `json:"identita,omitempty"`
	Casella       *Casella3                     `json:"casella,omitempty"`
	MotivoCodice  string                        `json:"motivo_codice,omitempty"`
	AzioneID      *uuid.UUID                    `json:"azione_id,omitempty"`
	SegnaleVisto  *importazione.SegnaleVisto    `json:"segnale_visto,omitempty"`
	AllegatoID    *uuid.UUID                    `json:"allegato_id,omitempty"`
	DaDecisione   string                        `json:"da_decisione,omitempty"`
	Motivo        string                        `json:"motivo,omitempty"`
	// U2 §7.7: l'evidenza scelta a mano nella correzione
	EvidenzaDocumentale *EvidenzaDocumentale `json:"evidenza_documentale,omitempty"`
	// conferma_categoria (U2 §6.4.2)
	Soggetto         *SoggettoCategoria `json:"soggetto,omitempty"`
	Categoria        string             `json:"categoria,omitempty"`
	Esito            string             `json:"esito,omitempty"`
	OrigineCategoria string             `json:"origine_categoria,omitempty"`
	ClasseProposta   *ClasseProposta    `json:"classe_proposta,omitempty"`
	Ingresso         string             `json:"ingresso,omitempty"`
	DaConferma       *DaConferma        `json:"da_conferma,omitempty"`
}

// IdentitaScelta e' l'identita' fissata da una decisione identita (EM §3.6.1): byte per byte, mai NFC.
type IdentitaScelta struct {
	Emittente string `json:"emittente"`
	Namespace string `json:"namespace,omitempty"`
	Codice    string `json:"codice"`
	Rev       string `json:"rev,omitempty"`
}

// Canonica: i byte canonici della decisione.
func (d Decisione3) Canonica() ([]byte, error) { return jsoncanonico.Codifica(d) }

type decisione3In struct {
	ID            *string                       `json:"id"`
	Tipo          *string                       `json:"tipo"`
	ChiaveBozza   *string                       `json:"chiave_bozza"`
	ComponenteID  *uuid.UUID                    `json:"componente_id"`
	PadreID       *uuid.UUID                    `json:"padre_id"`
	FiglioID      *uuid.UUID                    `json:"figlio_id"`
	Qta           *int                          `json:"qta"`
	Prodotti      []uuid.UUID                   `json:"prodotti"`
	CodicePrima   *string                       `json:"codice_prima"`
	CodiceDopo    *string                       `json:"codice_dopo"`
	EmittenteDopo *string                       `json:"emittente_dopo"`
	NamespaceDopo *string                       `json:"namespace_dopo"`
	TipoNuovo     *string                       `json:"tipo_nuovo"`
	FirmaEffetto  *string                       `json:"firma_effetto"`
	Modo          *string                       `json:"modo"`
	Importazione  *string                       `json:"importazione"`
	ChiaveLettura *string                       `json:"chiave_lettura"`
	Riferimento   *string                       `json:"riferimento"`
	Lettura       *importazione.LetturaCompatta `json:"lettura"`
	Identita      *identitaSceltaIn             `json:"identita"`
	Casella       *casella3In                   `json:"casella"`
	MotivoCodice  *string                       `json:"motivo_codice"`
	AzioneID      *uuid.UUID                    `json:"azione_id"`
	SegnaleVisto  *importazione.SegnaleVisto    `json:"segnale_visto"`
	AllegatoID    *uuid.UUID                    `json:"allegato_id"`
	DaDecisione   *string                       `json:"da_decisione"`
	Motivo        *string                       `json:"motivo"`

	EvidenzaDocumentale *evidenzaIn       `json:"evidenza_documentale"`
	Soggetto            *soggettoIn       `json:"soggetto"`
	Categoria           *string           `json:"categoria"`
	Esito               *string           `json:"esito"`
	OrigineCategoria    *string           `json:"origine_categoria"`
	ClasseProposta      *classePropostaIn `json:"classe_proposta"`
	Ingresso            *string           `json:"ingresso"`
	DaConferma          *daConfermaIn     `json:"da_conferma"`
}

type identitaSceltaIn struct {
	Emittente *string `json:"emittente"`
	Namespace *string `json:"namespace"`
	Codice    *string `json:"codice"`
	Rev       *string `json:"rev"`
}

// LeggiDecisione3 legge una decisione formato 3 con la forma chiusa del suo tipo: id e tipo, i soli campi del tipo, gli
// obbligatori presenti, ogni valore nella sua forma. Un campo assente, null o vuoto e' la stessa cosa.
func LeggiDecisione3(campo string, raw json.RawMessage) (Decisione3, *Errore) {
	var d Decisione3
	if _, err := jsoncanonico.Codifica(raw); err != nil {
		return d, corpo(campo, "la decisione non si legge: %v", err)
	}
	if err := nomiInMinuscolo(raw); err != nil {
		return d, corpo(campo, "%v", err)
	}
	if e := valoriSenzaNFC(campo, raw); e != nil {
		return d, e
	}
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal(raw, &chiavi); err != nil {
		return d, corpo(campo, "una decisione e' un oggetto")
	}
	var testa struct {
		ID   *string `json:"id"`
		Tipo *string `json:"tipo"`
	}
	_ = json.Unmarshal(raw, &testa)
	if testa.ID == nil || !reChiave.MatchString(*testa.ID) {
		return d, corpo(campo+".id", "l'id di una decisione e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	f, ok := formeDecisione[valore(testa.Tipo)]
	if !ok {
		return d, corpo(campo+".tipo", "tipo di decisione sconosciuto %q (i 17 del formato 3)", valore(testa.Tipo))
	}
	ammessi := map[string]bool{"id": true, "tipo": true}
	for _, k := range append(append([]string{}, f.obbligatori...), f.facoltativi...) {
		ammessi[k] = true
	}
	nomi := make([]string, 0, len(chiavi))
	for k := range chiavi {
		nomi = append(nomi, k)
	}
	sort.Strings(nomi)
	for _, k := range nomi {
		if !ammessi[k] && presenteValore(chiavi[k]) {
			return d, corpo(campo+"."+k, "campo non ammesso in una decisione %s", *testa.Tipo)
		}
	}
	for _, k := range f.obbligatori {
		if !presenteValore(chiavi[k]) {
			return d, corpo(campo+"."+k, "una decisione %s vuole %s", *testa.Tipo, k)
		}
	}
	var in decisione3In
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return d, corpo(campo, "la decisione non ha la forma attesa: %v", err)
	}
	d = in.converti()
	if e := d.valida(campo); e != nil {
		return Decisione3{}, e
	}
	return d, nil
}

// presenteValore: un campo c'e' se non e' assente, null, "" o un elenco vuoto.
func presenteValore(r json.RawMessage) bool {
	s := strings.TrimSpace(string(r))
	return s != "" && s != "null" && s != `""` && s != "[]"
}

func (in decisione3In) converti() Decisione3 {
	d := Decisione3{ID: valore(in.ID), Tipo: valore(in.Tipo), ChiaveBozza: valore(in.ChiaveBozza), ComponenteID: in.ComponenteID,
		PadreID: in.PadreID, FiglioID: in.FiglioID, Prodotti: in.Prodotti, CodicePrima: valore(in.CodicePrima),
		CodiceDopo: valore(in.CodiceDopo), EmittenteDopo: valore(in.EmittenteDopo), NamespaceDopo: valore(in.NamespaceDopo),
		TipoNuovo: valore(in.TipoNuovo), FirmaEffetto: valore(in.FirmaEffetto), Modo: valore(in.Modo),
		Importazione: valore(in.Importazione), ChiaveLettura: valore(in.ChiaveLettura), Riferimento: valore(in.Riferimento),
		Lettura: in.Lettura, MotivoCodice: valore(in.MotivoCodice), AzioneID: in.AzioneID, SegnaleVisto: in.SegnaleVisto,
		AllegatoID: in.AllegatoID, DaDecisione: valore(in.DaDecisione), Categoria: valore(in.Categoria), Esito: valore(in.Esito),
		OrigineCategoria: valore(in.OrigineCategoria), Ingresso: valore(in.Ingresso)}
	if e := in.EvidenzaDocumentale; e != nil {
		x := e.converti()
		d.EvidenzaDocumentale = &x
	}
	if s := in.Soggetto; s != nil {
		d.Soggetto = &SoggettoCategoria{ChiaveBozza: valore(s.ChiaveBozza)}
	}
	if c := in.ClasseProposta; c != nil {
		d.ClasseProposta = &ClasseProposta{Categoria: valore(c.Categoria), Fonte: valore(c.Fonte), Valore: valore(c.Valore)}
	}
	if x := in.DaConferma; x != nil {
		d.DaConferma = &DaConferma{DecisioneID: valore(x.DecisioneID)}
		if x.ConfermaID != nil {
			d.DaConferma.ConfermaID = *x.ConfermaID
		}
		if x.Numero != nil {
			d.DaConferma.Numero = *x.Numero
		}
	}
	if in.Qta != nil {
		d.Qta = *in.Qta
	}
	if len(d.Prodotti) == 0 {
		d.Prodotti = nil
	}
	if in.Motivo != nil {
		d.Motivo = nfc(*in.Motivo) // testo umano: NFC prima dei limiti
	}
	if id := in.Identita; id != nil {
		d.Identita = &IdentitaScelta{Emittente: valore(id.Emittente), Namespace: valore(id.Namespace), Codice: valore(id.Codice), Rev: valore(id.Rev)}
	}
	if c := in.Casella; c != nil {
		d.Casella = &Casella3{Gruppo: valore(c.Gruppo), Rif: valore(c.Rif)}
		if c.AllegatoID != nil {
			d.Casella.AllegatoID = *c.AllegatoID
		}
	}
	return d
}

// valida: la forma di ogni campo del tipo (senza il database).
func (d *Decisione3) valida(campo string) *Errore {
	uuidValido := func(nome string, u *uuid.UUID) *Errore {
		if u != nil && *u == uuid.Nil {
			return corpo(campo+"."+nome, "UUID nullo")
		}
		return nil
	}
	for _, x := range []struct {
		nome string
		u    *uuid.UUID
	}{{"componente_id", d.ComponenteID}, {"padre_id", d.PadreID}, {"figlio_id", d.FiglioID}, {"azione_id", d.AzioneID},
		{"allegato_id", d.AllegatoID}} {
		if e := uuidValido(x.nome, x.u); e != nil {
			return e
		}
	}
	if d.ChiaveBozza != "" && !reChiave.MatchString(d.ChiaveBozza) {
		return corpo(campo+".chiave_bozza", "la chiave e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	// la decisione escludi da cui nasce lo svincolo: un id della stessa forma; che sia un'esclusione dello stesso
	// snapshot, dello stesso componente, lo controlla lo snapshot (validaDecisioni, PE-27)
	if d.DaDecisione != "" && !reChiave.MatchString(d.DaDecisione) {
		return corpo(campo+".da_decisione", "l'id di una decisione e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	maxMotivo := MaxMotivo
	if d.Tipo == DecisioneRicodifica {
		maxMotivo = MaxMotivoRicodifica
	}
	if d.Motivo != "" && !testoLibero(d.Motivo, maxMotivo) {
		return corpo(campo+".motivo", "il motivo e' un testo di al piu' %d caratteri, senza caratteri di controllo", maxMotivo)
	}
	if d.PadreID != nil && d.FiglioID != nil && *d.PadreID == *d.FiglioID {
		return corpo(campo, "padre e figlio sono lo stesso componente")
	}
	switch d.Tipo {
	case DecisioneArcoCondiviso:
		if d.Qta < 1 || d.Qta > MaxQta {
			return corpo(campo+".qta", "quantita' %d fuori da 1…%d", d.Qta, MaxQta)
		}
		if len(d.Prodotti) > MaxVoci {
			return corpo(campo+".prodotti", "troppi prodotti (al piu' %d)", MaxVoci)
		}
		visti := map[uuid.UUID]bool{}
		for i, p := range d.Prodotti {
			if p == uuid.Nil || visti[p] {
				return corpo(fmt.Sprintf("%s.prodotti[%d]", campo, i), "prodotto nullo o ripetuto")
			}
			visti[p] = true
		}
	case DecisioneRicodifica:
		if !codiceRegistrato(d.CodicePrima) {
			return corpo(campo+".codice_prima", "il codice registrato, al piu' %d caratteri", MaxCodiceRegistrato)
		}
		if !classificazione.CodiceAmmissibile(d.CodiceDopo) {
			return corpo(campo+".codice_dopo", "un codice nuovo e' senza spazi e al piu' %d caratteri", classificazione.MaxCodice)
		}
		if !EmittenteAmmissibile(d.EmittenteDopo) {
			return corpo(campo+".emittente_dopo", "emittente %q: cliente:<uuid> oppure partner:<id>", d.EmittenteDopo)
		}
		if d.NamespaceDopo != "" && !NamespaceAmmissibile(d.NamespaceDopo) {
			return corpo(campo+".namespace_dopo", "il namespace e' un testo non vuoto di al piu' %d caratteri", MaxNamespace)
		}
	case DecisioneCambiaTipo:
		if !db.TipoComponente(d.TipoNuovo).Valid() {
			return corpo(campo+".tipo_nuovo", "tipo sconosciuto %q", d.TipoNuovo)
		}
		if !reFirmaEffetto.MatchString(d.FirmaEffetto) {
			return corpo(campo+".firma_effetto", "la firma dell'effetto e' esadecimale minuscola (24-64 cifre)")
		}
	case DecisioneIdentita:
		return d.validaIdentita(campo)
	case DecisioneConfermaCategoria:
		return d.validaCategoria(campo)
	case DecisioneNonIncludere:
		if !motiviNonIncludere[d.MotivoCodice] {
			return corpo(campo+".motivo_codice", "motivo_codice %q: elemento_cad, duplicato_cad, non_articolo o altro", d.MotivoCodice)
		}
		if d.AzioneID.Version() != 4 {
			return corpo(campo+".azione_id", "l'azione e' un UUID v4, lo stesso per i nodi di una selezione")
		}
		if d.Importazione != "" && !reSha256.MatchString(d.Importazione) {
			return corpo(campo+".importazione", "l'impronta del registro e' di 64 cifre esadecimali minuscole")
		}
		if s := d.SegnaleVisto; s != nil {
			for _, x := range []struct{ nome, v string }{{"codice", s.Codice}, {"regola", s.Regola}, {"versione_servizio", s.VersioneServizio}} {
				if strings.TrimSpace(x.v) == "" || runeOltre(x.v, MaxCodiceSegnale) || haControlli(x.v) {
					return corpo(campo+".segnale_visto."+x.nome, "%s del segnale visto: non vuoto, al piu' %d caratteri", x.nome, MaxCodiceSegnale)
				}
			}
			if d.Importazione == "" {
				return corpo(campo+".importazione", "il segnale visto viene da un registro: la decisione cita il suo registro")
			}
		}
		if e := validaCasellaForma(campo+".casella", *d.Casella); e != nil {
			return e
		}
	}
	return nil
}

// validaIdentita: la forma della decisione identita (EM §3.6.1).
func (d *Decisione3) validaIdentita(campo string) *Errore {
	id := d.Identita
	if !EmittenteAmmissibile(id.Emittente) {
		return corpo(campo+".identita.emittente", "emittente %q: cliente:<uuid> oppure partner:<id>", id.Emittente)
	}
	if id.Namespace != "" && !NamespaceAmmissibile(id.Namespace) {
		return corpo(campo+".identita.namespace", "il namespace e' un testo non vuoto di al piu' %d caratteri", MaxNamespace)
	}
	if !codiceRegistrato(id.Codice) {
		return corpo(campo+".identita.codice", "il codice e' obbligatorio, al piu' %d caratteri, senza spazi ai bordi", MaxCodiceRegistrato)
	}
	// qui la sola forma: le minuscole ASCII dipendono da dove viene la rev (il nodo e la decisione insieme), e le guarda
	// lo snapshot (revDallaFonte, proposta di emendamento P-18)
	if id.Rev != "" && !classificazione.RevAmmissibile(id.Rev) {
		return corpo(campo+".identita.rev", "revisione con spazi o controlli, o oltre %d byte", classificazione.MaxRev)
	}
	if d.Importazione != "" && !reSha256.MatchString(d.Importazione) {
		return corpo(campo+".importazione", "l'impronta del registro e' di 64 cifre esadecimali minuscole")
	}
	if d.ChiaveLettura != "" && (!reChiaveLettura.MatchString(d.ChiaveLettura) || runeOltre(d.ChiaveLettura, MaxChiaveLettura) || haControlli(d.ChiaveLettura)) {
		return corpo(campo+".chiave_lettura", "la chiave di lettura e' <allegato_id>/<id della lettura>, al piu' %d caratteri", MaxChiaveLettura)
	}
	if d.EvidenzaDocumentale != nil && d.Modo != ModoCorrezione {
		return corpo(campo+".evidenza_documentale", "l'evidenza documentale sta solo in una correzione (U2 §7.7)")
	}
	switch d.Modo {
	case ModoAlternativa:
		if d.Importazione == "" {
			return corpo(campo+".importazione", "un'alternativa cita il registro su cui e' scelta")
		}
		if (d.ChiaveLettura == "") == (d.Riferimento == "") {
			return corpo(campo, "un'alternativa cita una lettura (chiave_lettura) oppure il codice manuale (riferimento), uno solo dei due")
		}
		if d.ChiaveLettura != "" {
			if d.Lettura == nil {
				return corpo(campo+".lettura", "con chiave_lettura la decisione porta la copia compatta della lettura")
			}
			l, err := d.Lettura.Normalizzata()
			if err != nil {
				return corpo(campo+".lettura", "%v", err)
			}
			if !EmittenteAmmissibile(l.Emittente) || l.Letterale == "" {
				return corpo(campo+".lettura", "la copia della lettura vuole emittente e letterale")
			}
			if l.Emittente != id.Emittente || l.Namespace != id.Namespace {
				return corpo(campo+".identita", "con un'alternativa da una lettura emittente e namespace sono quelli della lettura")
			}
			d.Lettura = &l
		} else {
			if d.Riferimento != RiferimentoManualeLegacy {
				return corpo(campo+".riferimento", "il riferimento e' %q", RiferimentoManualeLegacy)
			}
			if d.Lettura != nil {
				return corpo(campo+".lettura", "il codice manuale non ha una lettura")
			}
			if !strings.HasPrefix(id.Emittente, "cliente:") || id.Namespace != "" {
				return corpo(campo+".identita.emittente", "il codice manuale e' del cliente, presunto, senza namespace")
			}
		}
	case ModoCorrezione:
		if d.ChiaveLettura != "" || d.Riferimento != "" || d.Lettura != nil {
			return corpo(campo, "una correzione non e' un'evidenza del registro: niente chiave_lettura, riferimento o lettura")
		}
		if d.EvidenzaDocumentale != nil {
			if e := validaEvidenza(campo+".evidenza_documentale", *d.EvidenzaDocumentale); e != nil {
				return e
			}
		}
		if strings.HasPrefix(id.Emittente, "partner:") && d.Importazione == "" {
			return corpo(campo+".importazione", "una correzione verso un partner cita il registro in cui il partner e' attivo")
		}
	default:
		return corpo(campo+".modo", "modo %q: alternativa o correzione", d.Modo)
	}
	return nil
}

// validaCasellaForma: la forma di una casella senza la sorgente (EM §3.4.1, §7.2).
func validaCasellaForma(campo string, c Casella3) *Errore {
	if c.AllegatoID == uuid.Nil {
		return corpo(campo+".allegato_id", "allegato nullo")
	}
	if !testoLibero(c.Gruppo, MaxGruppo) {
		return corpo(campo+".gruppo", "il gruppo e' obbligatorio, al piu' %d caratteri, senza caratteri di controllo", MaxGruppo)
	}
	if !reRif.MatchString(c.Rif) || runeOltre(c.Rif, MaxRif) || haControlli(c.Rif) {
		return corpo(campo+".rif", "rif nella forma nodo:<sha256>:<chiave>, al piu' %d caratteri", MaxRif)
	}
	return nil
}

// valoriSenzaNFC scorre un JSON: numeri interi senza esponente e diversi da -0, al piu' MaxCifre cifre; nessuna stringa
// con U+0000. A differenza di valoreCanonico del formato 2 non cambia niente: l'NFC lo applicano i lettori ai soli campi
// di testo umano.
func valoriSenzaNFC(campo string, raw []byte) *Errore {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return corpo(campo, "il JSON non si legge: %v", err)
		}
		switch x := tok.(type) {
		case json.Number:
			s := string(x)
			if len(strings.TrimPrefix(s, "-")) > MaxCifre {
				return corpo(campo, "un numero ha piu' di %d cifre", MaxCifre)
			}
			if !reIntero.MatchString(s) || s == "-0" {
				return corpo(campo, "il numero %s non e' un intero scritto senza esponente (la forma canonica ha solo interi)", s)
			}
		case string:
			if strings.ContainsRune(x, 0) {
				return corpo(campo, "una stringa contiene U+0000, che il database non accetta")
			}
			if !utf8.ValidString(x) {
				return corpo(campo, "una stringa non e' UTF-8 valido")
			}
		}
	}
}
