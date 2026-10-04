// Package evidenze è il vocabolario comune del motore A e il contratto del suo ingresso:
//   - dove si è letto: Contesto, CampoFonte, Selettore;
//   - che cosa si è letto: DocumentoEvidenze;
//   - quali segmenti valgono come richiesta: UsoSegmenti;
//   - come si segnala un problema: Diagnostica. I codici li dichiara ogni pacchetto che li produce.
//
// È una foglia: non importa niente del progetto, perché lo importano grammatiche, adattatori, motore,
// proposte, valutazione e confronto. Le grammatiche ne usano solo il vocabolario dei selettori e le
// diagnostiche (R41 a, G8). Un documento non contiene regole dei clienti (parte 1 §7.2): qui niente
// interpretazione.
package evidenze

import (
	"fmt"
	"strings"
)

// ---- vocabolario ----

// Contesto: dove è stata letta un'osservazione. È un insieme chiuso di undici valori (parte 1 §4.1): un
// valore fuori elenco è un errore di contratto, mai un ripiego. «figlio_step» non è un contesto: lo scrivono
// gli attesi, e lo traduce in nodo_step solo il loro runner (R19 a).
type Contesto string

const (
	ContestoOggetto      Contesto = "oggetto"
	ContestoCorpo        Contesto = "corpo"
	ContestoStoria       Contesto = "storia"
	ContestoNomeFile     Contesto = "nome_file"
	ContestoVoceArchivio Contesto = "voce_archivio"
	ContestoCartiglio    Contesto = "cartiglio"
	ContestoElencoPDF    Contesto = "elenco_pdf" // nel vocabolario, ma in A1 nessun adattatore lo produce: mancano i fatti tabellari (v3 §2)
	ContestoTestoPDF     Contesto = "testo_pdf"
	ContestoMetadatiPDF  Contesto = "metadati_pdf"
	ContestoRadiceSTEP   Contesto = "radice_step"
	ContestoNodoSTEP     Contesto = "nodo_step" // gli attesi lo scrivono «figlio_step»: lo traduce il runner (R19 a)
)

// contesti: l'elenco chiuso, nell'ordine della parte 1 §4.1.
var contesti = []Contesto{
	ContestoOggetto, ContestoCorpo, ContestoStoria, ContestoNomeFile, ContestoVoceArchivio,
	ContestoCartiglio, ContestoElencoPDF, ContestoTestoPDF, ContestoMetadatiPDF,
	ContestoRadiceSTEP, ContestoNodoSTEP,
}

// VarianteCampo: la famiglia di campi che un contesto ammette (parte 1 §4.2).
type VarianteCampo string

const (
	VarianteNessuno   VarianteCampo = "nessuno"
	VarianteCartiglio VarianteCampo = "cartiglio"
	VarianteStep      VarianteCampo = "step"
	VarianteElenco    VarianteCampo = "elenco"
)

// CampoFonte: che tipo di dato è stato letto. Sono due stringhe confrontabili, quindi una chiave di mappa
// (v3 §4.3). Con la variante "nessuno" il valore è vuoto: "nessuno" non è un jolly che accetta ogni campo.
// I valori ammessi:
//   - cartiglio: codice, numero_disegno, revisione, titolo, scala, materiale, particolare_simile, sconosciuto;
//   - step: id, nome, descrizione, revisione. Per lo STEP «revisione» è la formazione grezza, un dato non
//     confrontabile senza una politica (v3 D1);
//   - elenco: codice, descrizione, revisione, quantita, posizione, sconosciuto.
type CampoFonte struct {
	Variante VarianteCampo
	Valore   string
}

// valoriPerVariante: la tabella della parte 1 §4.2, valori nell'ordine in cui la parte 1 li elenca.
var valoriPerVariante = map[VarianteCampo][]string{
	VarianteNessuno:   nil,
	VarianteCartiglio: {"codice", "numero_disegno", "revisione", "titolo", "scala", "materiale", "particolare_simile", "sconosciuto"},
	VarianteStep:      {"id", "nome", "descrizione", "revisione"},
	VarianteElenco:    {"codice", "descrizione", "revisione", "quantita", "posizione", "sconosciuto"},
}

// variantePerContesto: quale variante ammette ogni contesto (parte 1 §4.2). Un contesto ne ammette una sola.
var variantePerContesto = map[Contesto]VarianteCampo{
	ContestoOggetto:      VarianteNessuno,
	ContestoCorpo:        VarianteNessuno,
	ContestoStoria:       VarianteNessuno,
	ContestoNomeFile:     VarianteNessuno,
	ContestoVoceArchivio: VarianteNessuno,
	ContestoCartiglio:    VarianteCartiglio,
	ContestoElencoPDF:    VarianteElenco,
	ContestoTestoPDF:     VarianteNessuno,
	ContestoMetadatiPDF:  VarianteNessuno,
	ContestoRadiceSTEP:   VarianteStep,
	ContestoNodoSTEP:     VarianteStep,
}

// Selettore: una coppia valida contesto/campo. Abilita le forme di una grammatica e indirizza le unità di un
// documento. La forma testuale è quella degli attesi: «cartiglio.codice», «radice_step.id», «nome_file».
type Selettore struct {
	Contesto Contesto
	Campo    CampoFonte
}

// LeggiSelettore legge la forma testuale degli attesi: «cartiglio.codice», «radice_step.id», «nome_file».
// Una coppia non ammessa, un contesto fuori elenco o «nessuno» usato come jolly sono un *ErroreContratto con
// il codice contratto.selettore_non_ammesso. Un selettore generico («cartiglio.*») non è una coppia:
// *ErroreContratto con il codice contratto.selettore_generico. Se in A1 un generico sia riservato non lo
// decide questo pacchetto: lo dice la grammatica (R20 c), che li riconosce prima di chiamare questa funzione
// (D-12).
//
// Il testo si legge com'è: niente spazi tolti, niente maiuscole abbassate. Nessun ripiego.
func LeggiSelettore(s string) (Selettore, error) {
	ctx, valore, conCampo := strings.Cut(s, ".")
	c := Contesto(ctx)
	if conCampo && valore == "*" {
		if _, ok := variantePerContesto[c]; ok {
			return Selettore{}, &ErroreContratto{Diagnostiche: []Diagnostica{{
				Codice:    CodiceSelettoreGenerico,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  "campo",
				Messaggio: fmt.Sprintf("il selettore generico %q non è una coppia contesto/campo: si espande in compilazione, sulle coppie ammesse", s),
			}}}
		}
	}
	v, ok := variantePerContesto[c]
	if !ok {
		return Selettore{}, &ErroreContratto{Diagnostiche: []Diagnostica{{
			Codice:    CodiceSelettoreNonAmmesso,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "contesto",
			Messaggio: fmt.Sprintf("selettore %q: il contesto %q non è nell'elenco chiuso (parte 1 §4.1)", s, ctx),
		}}}
	}
	sel := Selettore{Contesto: c, Campo: CampoFonte{Variante: v, Valore: valore}}
	if !conCampo {
		sel.Campo.Valore = ""
	} else if v == VarianteNessuno {
		// «nome_file.codice», «corpo.nessuno», «storia.»: la variante «nessuno» non accetta un campo.
		return Selettore{}, &ErroreContratto{Diagnostiche: []Diagnostica{{
			Codice:    CodiceSelettoreNonAmmesso,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "campo",
			Messaggio: fmt.Sprintf("selettore %q: il contesto %q non ha campi (variante «nessuno», che non è un jolly)", s, ctx),
		}}}
	}
	if err := sel.Valida(); err != nil {
		return Selettore{}, err
	}
	return sel, nil
}

// String è il contrario di LeggiSelettore, ed è stabile: la usano il JSON canonico e i rapporti.
func (s Selettore) String() string {
	if s.Campo.Valore == "" {
		return string(s.Contesto)
	}
	return string(s.Contesto) + "." + s.Campo.Valore
}

// MarshalText dà al JSON (anche a quello canonico) la forma testuale di String: nei documenti un selettore
// si scrive «cartiglio.codice», come negli attesi.
func (s Selettore) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText rilegge la forma testuale con LeggiSelettore: le stesse regole, nessun ripiego.
func (s *Selettore) UnmarshalText(b []byte) error {
	sel, err := LeggiSelettore(string(b))
	if err != nil {
		return err
	}
	*s = sel
	return nil
}

// Valida controlla la variante, il valore e l'elenco delle coppie ammesse (parte 1 §4.2). Nessun ripiego:
// un campo STEP con il contesto cartiglio è un errore, non un campo sconosciuto.
func (s Selettore) Valida() error {
	v, ok := variantePerContesto[s.Contesto]
	if !ok {
		return &ErroreContratto{Diagnostiche: []Diagnostica{{
			Codice:    CodiceSelettoreNonAmmesso,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "contesto",
			Messaggio: fmt.Sprintf("il contesto %q non è nell'elenco chiuso (parte 1 §4.1)", string(s.Contesto)),
		}}}
	}
	if s.Campo.Variante != v {
		return &ErroreContratto{Diagnostiche: []Diagnostica{{
			Codice:    CodiceSelettoreNonAmmesso,
			Gravita:   GravitaErrore,
			Natura:    NaturaContratto,
			Percorso:  "campo",
			Messaggio: fmt.Sprintf("il contesto %q ammette la variante %q, non %q", string(s.Contesto), string(v), string(s.Campo.Variante)),
		}}}
	}
	if v == VarianteNessuno {
		if s.Campo.Valore != "" {
			return &ErroreContratto{Diagnostiche: []Diagnostica{{
				Codice:    CodiceSelettoreNonAmmesso,
				Gravita:   GravitaErrore,
				Natura:    NaturaContratto,
				Percorso:  "campo",
				Messaggio: fmt.Sprintf("il contesto %q non ha campi (variante «nessuno», che non è un jolly)", string(s.Contesto)),
			}}}
		}
		return nil
	}
	for _, x := range valoriPerVariante[v] {
		if x == s.Campo.Valore {
			return nil
		}
	}
	return &ErroreContratto{Diagnostiche: []Diagnostica{{
		Codice:    CodiceSelettoreNonAmmesso,
		Gravita:   GravitaErrore,
		Natura:    NaturaContratto,
		Percorso:  "campo",
		Messaggio: fmt.Sprintf("la coppia %q non è ammessa: il campo %q non è della variante %q (parte 1 §4.2)", s.String(), s.Campo.Valore, string(v)),
	}}}
}

// CampiAmmessi: la tabella della parte 1 §4.2 per un contesto: la variante e i suoi valori (nessuno per la
// variante «nessuno»). Lo STEP ha anche «revisione»: è la formazione grezza, mai una revisione confrontabile
// senza una politica (v3 D1). Per un contesto fuori elenco la variante è vuota. L'elenco restituito è una
// copia: chi lo riceve può cambiarlo senza toccare la tabella.
func CampiAmmessi(c Contesto) (VarianteCampo, []string) {
	v, ok := variantePerContesto[c]
	if !ok {
		return "", nil
	}
	valori := valoriPerVariante[v]
	if len(valori) == 0 {
		return v, nil
	}
	return v, append([]string(nil), valori...)
}

// Intervallo: [Inizio, Fine) in byte UTF-8, 0-based, sul testo indicato. Inizio e Fine cadono su un confine di
// runa (piano A, par.3.4.4). Lo usano le letture di forma del motore e i localizzatori del documento.
type Intervallo struct {
	Inizio int `json:"inizio"`
	Fine   int `json:"fine"`
}
