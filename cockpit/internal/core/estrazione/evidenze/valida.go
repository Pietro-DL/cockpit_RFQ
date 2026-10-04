package evidenze

import (
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
)

// I valori chiusi del documento (parte 1 §3.1-§3.4). Un valore vuoto in un campo obbligatorio è fuori elenco
// come un valore sbagliato; i campi facoltativi (omitempty) si controllano solo quando ci sono.
var (
	tipiFonte         = []string{fonteMessaggio, fonteAllegato, fonteTesto}
	tipiRiferimento   = []string{riferimentoAnalisiFile, riferimentoMessaggio, riferimentoNessuno}
	tipiSegmento      = []string{SegmentoCorrente, SegmentoCitazione, SegmentoInoltro, SegmentoFirma, SegmentoSezioneTecnica}
	originiSegmento   = []string{"mittente_del_messaggio", "ignota"}
	tipiEntita        = []string{"disegno", "nodo_step", "riga", "sezione", "unita_isolata"}
	tipiLocalizzatore = []string{"testo", "pdf", "step", "tabella", "nome_file"}
	localizzazioni    = []string{"esatta", "parziale", "assente"}
	metodi            = []string{"nativo", "ocr", "parser", "adattatore"}
	zonePDF           = []string{"basso_destra", "pagina"}
	fontiPDF          = []string{"nativo", "ocr"}
	attributiSTEP     = []string{"id", "nome", "descrizione", "formazione"}
	campiNomeFile     = []string{"allegato.nome_file", "allegato.path_interno"}
	tipiLegame        = []string{"contiene_segmento", "campo_di_entita", "cella_di_riga", "intestazione_di_cella", "padre_figlio_step", "occorrenza_step"}
	statiQualita      = []string{"disponibile", "parziale", "non_disponibile", "errore"}
	mappature         = []string{"verificata", "plausibile", "sconosciuta"}
	statiCapacita     = []string{"disponibile", "parziale", "non_disponibile"}
)

const (
	legameConQuantita    = "padre_figlio_step"     // il solo legame che porta una quantità
	testoDelCorpo        = "messaggio.corpo_testo" // il testo su cui si misura PosTabella.Esatto
	localizzazioneEsatta = "esatta"
)

// ValidaDocumento controlla un documento (parte 1 §3.1-§3.4, §7.2):
//   - i riferimenti si risolvono: fonte, segmenti, entità, testi, estremi dei legami;
//   - gli intervalli stanno dentro il testo e su confini di runa;
//   - i testi sono UTF-8 valido;
//   - enum e coppie sono ammessi (anche il selettore di ogni unità e la variante di ogni localizzatore);
//   - gli ID locali sono unici;
//   - quando la localizzazione è dichiarata esatta, Testo == originale[inizio:fine] (A-C03).
//
// Ogni violazione ha il suo codice documento.* (codici_diagnostica.go). Le diagnostiche seguono l'ordine del
// documento, quindi due chiamate sullo stesso documento danno lo stesso elenco; nil vuol dire valido. Non
// interpreta niente: nessuna regola cliente è applicata ai testi (parte 1 §7.2).
func ValidaDocumento(d DocumentoEvidenze) []Diagnostica {
	v := &validatore{d: d, testi: map[string]*TestoOriginale{}, segmenti: map[string]bool{}, entita: map[string]bool{}, locali: map[string]bool{}}
	v.versione()
	v.fonte()
	v.indici()
	for i := range d.Segmenti {
		v.segmento(i)
	}
	for i := range d.Entita {
		v.entitaLocale(i)
	}
	for i := range d.Unita {
		v.unita(i)
	}
	for i := range d.Legami {
		v.legame(i)
	}
	v.qualita()
	return v.out
}

type validatore struct {
	d        DocumentoEvidenze
	out      []Diagnostica
	testi    map[string]*TestoOriginale // ID → testo (il primo, se ripetuto)
	segmenti map[string]bool
	entita   map[string]bool
	locali   map[string]bool // tutti gli ID di segmenti, entità, unità e legami: un solo spazio di nomi
}

func (v *validatore) aggiungi(d Diagnostica) { v.out = append(v.out, d) }

func (v *validatore) enum(percorso, cosa, valore string, ammessi []string, rif ...string) {
	if in(valore, ammessi) {
		return
	}
	v.aggiungi(Diagnostica{
		Codice:    CodiceDocumentoEnumIgnoto,
		Gravita:   GravitaErrore,
		Natura:    NaturaContratto,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("%s %q fuori elenco", cosa, valore),
		Rif:       rif,
	})
}

func (v *validatore) enumFacoltativo(percorso, cosa, valore string, ammessi []string, rif ...string) {
	if valore != "" {
		v.enum(percorso, cosa, valore, ammessi, rif...)
	}
}

func (v *validatore) pendente(percorso, cosa, id string, rif ...string) {
	v.aggiungi(Diagnostica{
		Codice:    CodiceDocumentoRiferimentoPendente,
		Gravita:   GravitaErrore,
		Natura:    NaturaContratto,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("%s %q non è nel documento", cosa, id),
		Rif:       append(rif, id),
	})
}

func (v *validatore) versione() {
	if v.d.VersioneSchema != VersioneSchemaDocumento {
		v.aggiungi(Diagnostica{
			Codice:    CodiceDocumentoEnumIgnoto,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "versione_schema",
			Messaggio: fmt.Sprintf("versione dello schema del documento %d: è ammessa solo la %d", v.d.VersioneSchema, VersioneSchemaDocumento),
		})
	}
}

func (v *validatore) fonte() {
	f := v.d.Fonte
	v.enum("fonte.tipo", "tipo di fonte", f.Tipo, tipiFonte, f.ID)
	v.enum("fonte.riferimento_fatti.tipo", "tipo di riferimento ai fatti", f.RiferimentoFatti.Tipo, tipiRiferimento, f.ID)
	if f.Tipo == fonteTesto && (f.OrigineID != uuid.Nil || f.RiferimentoFatti.Tipo != riferimentoNessuno) {
		// «testo» è la fonte senza origine nel DB: con un'origine o con dei fatti sarebbe un'altra fonte.
		v.aggiungi(Diagnostica{
			Codice:    CodiceDocumentoEnumIgnoto,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "fonte",
			Messaggio: "una fonte «testo» non ha origine nel DB: origine_id nullo e riferimento ai fatti «nessuno»",
			Rif:       []string{f.ID},
		})
	}
}

// indici raccoglie gli ID, segnala quelli ripetuti e controlla l'UTF-8 dei testi originali.
func (v *validatore) indici() {
	for i := range v.d.Testi {
		t := &v.d.Testi[i]
		percorso := fmt.Sprintf("testi[%s]", t.ID)
		if _, gia := v.testi[t.ID]; gia {
			v.ripetuto(percorso, t.ID)
		} else {
			v.testi[t.ID] = t
		}
		if !utf8.ValidString(t.Testo) {
			v.aggiungi(Diagnostica{
				Codice:    CodiceDocumentoUTF8NonValido,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso + ".testo",
				Messaggio: fmt.Sprintf("il testo %q non è UTF-8 valido", t.ID),
				Rif:       []string{t.ID},
			})
		}
	}
	locale := func(gruppo, id string) {
		if v.locali[id] {
			v.ripetuto(fmt.Sprintf("%s[%s]", gruppo, id), id)
		}
		v.locali[id] = true
	}
	for _, s := range v.d.Segmenti {
		locale("segmenti", s.ID)
		v.segmenti[s.ID] = true
	}
	for _, e := range v.d.Entita {
		locale("entita", e.ID)
		v.entita[e.ID] = true
	}
	for _, u := range v.d.Unita {
		locale("unita", u.ID)
	}
	for _, l := range v.d.Legami {
		locale("legami", l.ID)
	}
}

func (v *validatore) ripetuto(percorso, id string) {
	v.aggiungi(Diagnostica{
		Codice:    CodiceDocumentoIDRipetuto,
		Gravita:   GravitaErrore,
		Natura:    NaturaContratto,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("l'ID %q compare due volte nel documento", id),
		Rif:       []string{id},
	})
}

func (v *validatore) segmento(i int) {
	s := v.d.Segmenti[i]
	percorso := fmt.Sprintf("segmenti[%s]", s.ID)
	v.enum(percorso+".tipo", "tipo di segmento", s.Tipo, tipiSegmento, s.ID)
	v.enum(percorso+".origine", "origine del segmento", s.Origine, originiSegmento, s.ID)
	if s.PadreID != "" && (!v.segmenti[s.PadreID] || s.PadreID == s.ID) {
		v.pendente(percorso+".padre_id", "il segmento padre", s.PadreID, s.ID)
	}
	v.localizzatore(percorso+".posizione", s.Posizione, nil, s.ID)
}

func (v *validatore) entitaLocale(i int) {
	e := v.d.Entita[i]
	percorso := fmt.Sprintf("entita[%s]", e.ID)
	v.enum(percorso+".tipo", "tipo di entità", e.Tipo, tipiEntita, e.ID)
	if e.SegmentoID != "" && !v.segmenti[e.SegmentoID] {
		v.pendente(percorso+".segmento_id", "il segmento", e.SegmentoID, e.ID)
	}
	v.localizzatore(percorso+".posizione", e.Posizione, nil, e.ID)
}

func (v *validatore) unita(i int) {
	u := v.d.Unita[i]
	percorso := fmt.Sprintf("unita[%s]", u.ID)
	if u.FonteID != v.d.Fonte.ID {
		// Tutti i riferimenti si risolvono nel medesimo bundle (parte 1 §3.2): l'unità è della sua fonte.
		v.pendente(percorso+".fonte_id", "la fonte", u.FonteID, u.ID)
	}
	if u.SegmentoID != "" && !v.segmenti[u.SegmentoID] {
		v.pendente(percorso+".segmento_id", "il segmento", u.SegmentoID, u.ID)
	}
	if u.EntitaID != "" && !v.entita[u.EntitaID] {
		v.pendente(percorso+".entita_id", "l'entità", u.EntitaID, u.ID)
	}
	if err := u.Selettore.Valida(); err != nil {
		v.aggiungi(Diagnostica{
			Codice:    CodiceDocumentoEnumIgnoto,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  percorso + ".selettore",
			Messaggio: fmt.Sprintf("selettore %q non ammesso (parte 1 §4.2)", u.Selettore.String()),
			Rif:       []string{u.ID},
		})
	}
	testoValido := utf8.ValidString(u.Testo)
	if !testoValido {
		v.aggiungi(Diagnostica{
			Codice:    CodiceDocumentoUTF8NonValido,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  percorso + ".testo",
			Messaggio: fmt.Sprintf("il testo dell'unità %q non è UTF-8 valido", u.ID),
			Rif:       []string{u.ID},
		})
	}
	v.enum(percorso+".qualita.localizzazione", "localizzazione", u.Qualita.Localizzazione, localizzazioni, u.ID)
	v.enumFacoltativo(percorso+".qualita.metodo", "metodo", u.Qualita.Metodo, metodi, u.ID)

	var relativo *string
	if testoValido {
		relativo = &u.Testo
	}
	v.localizzatore(percorso+".posizione", u.Posizione, relativo, u.ID)

	// A-C03: con la localizzazione esatta, il testo dell'unità è l'originale nell'intervallo dichiarato.
	if u.Qualita.Localizzazione == localizzazioneEsatta && testoValido {
		if orig, ok := v.originale(u.Posizione); ok && orig != u.Testo {
			v.aggiungi(Diagnostica{
				Codice:    CodiceDocumentoTestoDiversoDaOriginale,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso + ".testo",
				Messaggio: fmt.Sprintf("localizzazione esatta, ma il testo dell'unità %q non è quello dell'originale nell'intervallo (A-C03)", u.ID),
				Rif:       []string{u.ID},
			})
		}
	}
}

// localizzatore controlla la variante e i suoi riferimenti e intervalli. relativo è il testo su cui si
// misura l'intervallo di un PosPDF (il testo dell'unità); nil per segmenti ed entità, e per un'unità il cui
// testo non è UTF-8 (già segnalato): allora dell'intervallo si controlla solo la forma.
func (v *validatore) localizzatore(percorso string, l Localizzatore, relativo *string, rif string) {
	v.enum(percorso+".tipo", "tipo di localizzatore", l.Tipo, tipiLocalizzatore, rif)
	varianti := map[string]bool{
		"testo":     l.Testo != nil,
		"pdf":       l.PDF != nil,
		"step":      l.STEP != nil,
		"tabella":   l.Tabella != nil,
		"nome_file": l.NomeFile != nil,
	}
	for _, nome := range tipiLocalizzatore {
		if varianti[nome] != (nome == l.Tipo) {
			v.aggiungi(Diagnostica{
				Codice:    CodiceDocumentoEnumIgnoto,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  percorso,
				Messaggio: fmt.Sprintf("localizzatore di tipo %q: la variante %q deve esserci solo se è quella del tipo", l.Tipo, nome),
				Rif:       []string{rif},
			})
			break
		}
	}
	if l.Testo != nil {
		v.suTesto(percorso+".testo.intervallo", l.Testo.TestoID, l.Testo.Intervallo, rif)
	}
	if p := l.PDF; p != nil {
		v.enumFacoltativo(percorso+".pdf.zona", "zona del PDF", p.Zona, zonePDF, rif)
		v.enumFacoltativo(percorso+".pdf.fonte", "fonte del testo PDF", p.Fonte, fontiPDF, rif)
		if p.Intervallo != nil {
			if relativo != nil {
				v.intervallo(percorso+".pdf.intervallo", *relativo, *p.Intervallo, "il testo dell'unità", rif)
			} else if p.Intervallo.Inizio < 0 || p.Intervallo.Fine < p.Intervallo.Inizio {
				v.intervalloErrato(percorso+".pdf.intervallo", *p.Intervallo, "rovesciato o negativo", rif)
			}
		}
	}
	if s := l.STEP; s != nil {
		v.enum(percorso+".step.attributo", "attributo STEP", s.Attributo, attributiSTEP, rif)
	}
	if t := l.Tabella; t != nil && t.Esatto != nil {
		v.suTesto(percorso+".tabella.esatto", testoDelCorpo, *t.Esatto, rif)
	}
	if n := l.NomeFile; n != nil {
		v.enum(percorso+".nome_file.campo", "campo del nome", n.Campo, campiNomeFile, rif)
		_, haTesto := v.testi[n.Campo]
		switch {
		case !in(n.Campo, campiNomeFile):
			// già segnalato: senza un campo ammesso non c'è un testo su cui misurare
		case !haTesto:
			// un testo mancante si segnala una volta, non per ogni intervallo
			v.pendente(percorso+".nome_file.intervallo", "il testo", n.Campo, rif)
		default:
			v.suTesto(percorso+".nome_file.intervallo", n.Campo, n.Intervallo, rif)
			if n.Stem != nil {
				v.suTesto(percorso+".nome_file.stem", n.Campo, *n.Stem, rif)
			}
			if n.Estensione != nil {
				v.suTesto(percorso+".nome_file.estensione", n.Campo, *n.Estensione, rif)
			}
		}
	}
}

// suTesto: l'intervallo deve puntare a un testo originale che c'è, e starci dentro.
func (v *validatore) suTesto(percorso, testoID string, iv Intervallo, rif string) {
	t, ok := v.testi[testoID]
	if !ok {
		v.pendente(percorso, "il testo", testoID, rif)
		return
	}
	if !utf8.ValidString(t.Testo) {
		return // già segnalato: su un testo non UTF-8 i confini di runa non hanno senso
	}
	v.intervallo(percorso, t.Testo, iv, fmt.Sprintf("il testo %q", testoID), rif)
}

// intervallo: [Inizio, Fine) dentro il testo, con Inizio ≤ Fine, su confini di runa.
func (v *validatore) intervallo(percorso, testo string, iv Intervallo, dove string, rif string) {
	switch {
	case iv.Inizio < 0 || iv.Fine < iv.Inizio:
		v.intervalloErrato(percorso, iv, "rovesciato o negativo", rif)
	case iv.Fine > len(testo):
		v.intervalloErrato(percorso, iv, fmt.Sprintf("fuori da %s (%d byte)", dove, len(testo)), rif)
	case !confineDiRuna(testo, iv.Inizio) || !confineDiRuna(testo, iv.Fine):
		v.intervalloErrato(percorso, iv, fmt.Sprintf("a metà di una runa di %s", dove), rif)
	}
}

func (v *validatore) intervalloErrato(percorso string, iv Intervallo, perche string, rif string) {
	v.aggiungi(Diagnostica{
		Codice:    CodiceDocumentoIntervalloNonValido,
		Gravita:   GravitaErrore,
		Natura:    NaturaContratto,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("intervallo [%d, %d) %s", iv.Inizio, iv.Fine, perche),
		Rif:       []string{rif},
	})
}

func confineDiRuna(s string, i int) bool {
	return i == len(s) || utf8.RuneStart(s[i])
}

// originale: il pezzo di testo originale che il localizzatore indica, quando lo indica in byte su un testo
// del documento (testo, nome del file, cella con aggancio esatto). ok è falso se il localizzatore non
// indica un pezzo di testo originale, o se l'intervallo non è valido (già segnalato).
func (v *validatore) originale(l Localizzatore) (string, bool) {
	var id string
	var iv Intervallo
	switch {
	case l.Tipo == "testo" && l.Testo != nil:
		id, iv = l.Testo.TestoID, l.Testo.Intervallo
	case l.Tipo == "nome_file" && l.NomeFile != nil:
		id, iv = l.NomeFile.Campo, l.NomeFile.Intervallo
	case l.Tipo == "tabella" && l.Tabella != nil && l.Tabella.Esatto != nil:
		id, iv = testoDelCorpo, *l.Tabella.Esatto
	default:
		return "", false
	}
	t, ok := v.testi[id]
	if !ok || !utf8.ValidString(t.Testo) {
		return "", false
	}
	if iv.Inizio < 0 || iv.Fine < iv.Inizio || iv.Fine > len(t.Testo) ||
		!confineDiRuna(t.Testo, iv.Inizio) || !confineDiRuna(t.Testo, iv.Fine) {
		return "", false
	}
	return t.Testo[iv.Inizio:iv.Fine], true
}

func (v *validatore) legame(i int) {
	l := v.d.Legami[i]
	percorso := fmt.Sprintf("legami[%s]", l.ID)
	v.enum(percorso+".tipo", "tipo di legame", l.Tipo, tipiLegame, l.ID)
	if !v.locali[l.Da] || l.Da == l.ID {
		v.pendente(percorso+".da", "l'estremo", l.Da, l.ID)
	}
	if !v.locali[l.A] || l.A == l.ID {
		v.pendente(percorso+".a", "l'estremo", l.A, l.ID)
	}
	if l.Quantita != nil && l.Tipo != legameConQuantita {
		v.aggiungi(Diagnostica{
			Codice:    CodiceDocumentoEnumIgnoto,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  percorso + ".quantita",
			Messaggio: fmt.Sprintf("la quantità appartiene solo al legame %q, non a %q (parte 1 §3.4)", legameConQuantita, l.Tipo),
			Rif:       []string{l.ID},
		})
	}
}

func (v *validatore) qualita() {
	q := v.d.Qualita
	v.enum("qualita.stato", "stato della fonte", q.Stato, statiQualita)
	v.enumFacoltativo("qualita.metodo", "metodo", q.Metodo, metodi)
	v.enumFacoltativo("qualita.mappatura", "affidabilità della mappatura", q.Mappatura, mappature)
	for i, c := range q.Capacita {
		v.enum(fmt.Sprintf("qualita.capacita[%d].stato", i), "stato della capacità", c.Stato, statiCapacita)
	}
}
