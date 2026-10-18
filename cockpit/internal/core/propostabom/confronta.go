package propostabom

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Il confronto fra due registri (V1: emendamento di V1 §6.4, §6.5 «Su che cosa si confronta un nodo», §9.1
// ?confronto=1; A3 §5.2; PE-18 dell'interfaccia, accolta nelle decisioni dell'ondata A). Confronta dice se la proposta
// di adesso è cambiata rispetto a quella importata, e dove: solo sui campi che entrano in impronta_contenuto, con
// l'elenco chiuso del §3.8.1 e l'esclusione della voce della radice. Non ricopia l'elenco: le differenze si calcolano
// con la proiezione della foglia del registro (importazione.ImpronteParziali) su un contenuto ridotto alla voce, o al
// solo campo, che si confronta. Così una differenza c'è se e solo se il campo cambia impronta_contenuto, e con l'esito
// «uguale» le differenze sono vuote per costruzione.

// Gli esiti del confronto fra due registri (EM §9.1). Gli altri esiti di ?confronto=1 (altra_occorrenza,
// gruppo_non_proposto, senza_registro) vengono dal gruppo corrente della sorgente, non da qui.
const (
	EsitoUguale           = "uguale"
	EsitoRegoleCambiate   = "regole_cambiate"
	EsitoPropostaCambiata = "proposta_cambiata"
)

// I tipi di una differenza (DifferenzaNodo.Tipo, DifferenzaArco.Tipo).
const (
	DifferenzaAggiunto = "aggiunto"
	DifferenzaTolto    = "tolto"
	DifferenzaCambiato = "cambiato"
)

// Confronto: l'esito del confronto, l'impronta_contenuto del registro di adesso (quella che riallinea_proposta dichiara,
// V-7), le differenze sui soli campi del contenuto e se sono cambiate le evidenze (solo informazione: mai un blocco).
type Confronto struct {
	Esito                     string     `json:"esito"`
	ImprontaContenutoCorrente string     `json:"impronta_contenuto_corrente"`
	EvidenzeCambiate          bool       `json:"evidenze_cambiate"`
	Differenze                Differenze `json:"differenze"`
}

// Differenze: la forma di PE-18, {nodi[], archi[]}, più i campi del gruppo che entrano nel confronto (gruppo: radice,
// completezza del grafo, la fonte), che PE-18 non nomina e che una rianalisi di LS-01 cambia (RA-04).
type Differenze struct {
	Nodi   []DifferenzaNodo `json:"nodi,omitempty"`
	Archi  []DifferenzaArco `json:"archi,omitempty"`
	Gruppo []string         `json:"gruppo,omitempty"`
}

// Vuote: nessuna differenza.
func (d Differenze) Vuote() bool { return len(d.Nodi) == 0 && len(d.Archi) == 0 && len(d.Gruppo) == 0 }

// IsZero: nessuna differenza; serve a «omitzero» di encoding/json (l'effetto del cambio della fonte, EffettoCambioFonte).
func (d Differenze) IsZero() bool { return d.Vuote() }

// DifferenzaNodo: un nodo che cambia (PE-18): la voce del registro, il tipo e i campi del contenuto che cambiano.
// ChiaveBozza e DecisioneID li mette chi conosce la bozza (R1 ?confronto=1, la risposta di R2); il confronto fra registri
// dà la voce. Un nodo che passa fra le voci e i nodi fuori per decisione cambia stato_motore.raggiunto (fuori come
// non raggiunto) o stato_motore.tolto (fuori come tolto); un cambio delle relazioni che partono dal nodo è il campo
// «relazioni».
type DifferenzaNodo struct {
	ChiaveBozza string   `json:"chiave_bozza,omitempty"`
	Voce        string   `json:"voce,omitempty"`
	DecisioneID string   `json:"decisione_id,omitempty"`
	Tipo        string   `json:"tipo"`
	Campi       []string `json:"campi,omitempty"`
}

// DifferenzaArco: un arco del registro che cambia: l'occorrenza, i due rif, il tipo e, per un arco cambiato, i campi
// (quantita, occorrenze, altre).
type DifferenzaArco struct {
	AllegatoID string   `json:"allegato_id"`
	Padre      string   `json:"padre"`
	Figlio     string   `json:"figlio"`
	Tipo       string   `json:"tipo"`
	Campi      []string `json:"campi,omitempty"`
}

// Confronta: il confronto fra il registro importato (vecchio) e quello di adesso (nuovo) dello stesso prodotto, sulla
// stessa occorrenza: voci abbinate per voce (allegato e rif), archi per (allegato, padre, figlio). È puro e
// deterministico, e non cambia gli ingressi.
func Confronta(vecchio, nuovo importazione.Contenuto) Confronto {
	out := Confronto{ImprontaContenutoCorrente: nuovo.ImprontaContenuto, EvidenzeCambiate: vecchio.ImprontaEvidenze != nuovo.ImprontaEvidenze}
	out.Differenze = differenze(vecchio, nuovo)
	switch {
	case vecchio.ImprontaContenuto != nuovo.ImprontaContenuto || !out.Differenze.Vuote():
		out.Esito = EsitoPropostaCambiata
	case vecchio.ImprontaRegole != nuovo.ImprontaRegole:
		out.Esito = EsitoRegoleCambiate
	default:
		out.Esito = EsitoUguale
	}
	return out
}

// statoVoce: una voce come la vede il confronto: fra i nodi (con il suo JSON e, se è la voce della radice del suo
// registro, il rif della radice), fuori per decisione (con il segno) o assente.
type statoVoce struct {
	nodo   json.RawMessage
	radice string
	segno  string
}

func vociDi(c importazione.Contenuto) map[string]statoVoce {
	out := map[string]statoVoce{}
	for _, v := range c.Proposta.Nodi {
		b, err := jsoncanonico.Codifica(v)
		if err != nil {
			continue
		}
		s := statoVoce{nodo: b}
		if c.EVoceDellaRadice(v) {
			s.radice = v.Origine.Rif
		}
		out[v.Voce] = s
	}
	for _, x := range c.Proposta.FuoriPerDecisione {
		if _, ok := out[x.Voce]; !ok {
			out[x.Voce] = statoVoce{segno: x.Segno}
		}
	}
	return out
}

func differenze(vecchio, nuovo importazione.Contenuto) Differenze {
	var d Differenze
	vv, nv := vociDi(vecchio), vociDi(nuovo)
	rv, rn := relazioniPerVoce(vecchio), relazioniPerVoce(nuovo)
	chiavi := map[string]bool{}
	for _, m := range []map[string]statoVoce{vv, nv} {
		for k := range m {
			chiavi[k] = true
		}
	}
	for k := range rv {
		chiavi[k] = true
	}
	for k := range rn {
		chiavi[k] = true
	}
	for _, k := range ordinate(chiavi) {
		a, inA := vv[k]
		b, inB := nv[k]
		var campi []string
		switch {
		case !inA && !inB:
		case !inA:
			d.Nodi = append(d.Nodi, DifferenzaNodo{Voce: k, Tipo: DifferenzaAggiunto})
			continue
		case !inB:
			d.Nodi = append(d.Nodi, DifferenzaNodo{Voce: k, Tipo: DifferenzaTolto})
			continue
		case a.nodo != nil && b.nodo != nil:
			campi = campiDiversi(a.radice, a.nodo, b.radice, b.nodo)
		default:
			campi = campiDelloStato(a, b)
		}
		if rv[k] != rn[k] {
			campi = append(campi, "relazioni")
		}
		if len(campi) > 0 {
			sort.Strings(campi)
			d.Nodi = append(d.Nodi, DifferenzaNodo{Voce: k, Tipo: DifferenzaCambiato, Campi: campi})
		}
	}
	d.Archi = differenzeArchi(vecchio.Proposta.Archi, nuovo.Proposta.Archi)
	d.Gruppo = differenzeGruppo(vecchio, nuovo)
	return d
}

// campiDelloStato: una voce fuori per decisione da una parte e fra i nodi (o fuori con un altro segno) dall'altra: i
// campi di stato_motore che il passaggio cambia (fuori come non raggiunto: raggiunto falso; fuori come tolto: tolto).
func campiDelloStato(a, b statoVoce) []string {
	stato := func(s statoVoce) (raggiunto, tolto bool) {
		switch s.segno {
		case importazione.SegnoNonRaggiunto:
			return false, false
		case importazione.SegnoTolto:
			return true, true
		}
		var v struct {
			StatoMotore importazione.StatoMotore `json:"stato_motore"`
		}
		_ = json.Unmarshal(s.nodo, &v)
		return v.StatoMotore.Raggiunto == nil || *v.StatoMotore.Raggiunto, v.StatoMotore.Tolto != nil && *v.StatoMotore.Tolto
	}
	ra, ta := stato(a)
	rb, tb := stato(b)
	var out []string
	if ra != rb {
		out = append(out, "stato_motore.raggiunto")
	}
	if ta != tb {
		out = append(out, "stato_motore.tolto")
	}
	return out
}

// campiDiversi: i campi della voce che entrano in impronta_contenuto e cambiano fra le due voci. Prima l'impronta del
// contenuto della voce intera; solo se cambia, campo per campo, ognuno con la voce ridotta a voce, origine e quel campo.
// La radice serve solo a classificare la voce: radiceA e radiceB sono il rif della voce quando è la voce della radice
// del suo registro (così porta fra le evidenze i campi dello stato della RFQ, EM §3.8.1), "" altrimenti. La radice del
// gruppo non entra nel confronto dei campi: due registri con radici diverse non fanno «cambiata» una voce uguale che
// non è la radice in nessuno dei due. Una voce che è la radice in un registro e non nell'altro (registri con radici
// diverse: mai sulla stessa occorrenza, EM §6.4.1) si confronta campo per campo come una voce qualunque, cioè sui
// valori: il cambio della radice sta in differenze.gruppo.
func campiDiversi(radiceA string, a json.RawMessage, radiceB string, b json.RawMessage) []string {
	if improntaVoce(radiceA, a, "") == improntaVoce(radiceB, b, "") {
		return nil
	}
	radice := radiceA
	if radiceA != radiceB {
		radice = ""
	}
	var out []string
	for _, campo := range campiDelContenutoDellaVoce() {
		if improntaVoce(radice, a, campo) != improntaVoce(radice, b, campo) {
			out = append(out, campo)
		}
	}
	return out
}

// campiDelContenutoDellaVoce: i campi della voce della classe InContenuto dell'elenco chiuso della foglia, salvo voce e
// origine (la chiave dell'abbinamento), con i due campi di stato_motore che ci entrano; in ordine.
func campiDelContenutoDellaVoce() []string {
	const voce, stato = "proposta.nodi[].", "proposta.nodi[].stato_motore."
	var out []string
	for p, c := range importazione.Classificazione() {
		if c != importazione.InContenuto {
			continue
		}
		switch {
		case strings.HasPrefix(p, stato):
			out = append(out, "stato_motore."+strings.TrimPrefix(p, stato))
		case strings.HasPrefix(p, voce) && !strings.Contains(strings.TrimPrefix(p, voce), "."):
			if f := strings.TrimPrefix(p, voce); f != "voce" && f != "origine" {
				out = append(out, f)
			}
		}
	}
	sort.Strings(out)
	return out
}

// improntaVoce: l'impronta_contenuto di un contenuto con la sola voce (campo "") o con la voce ridotta a voce, origine e
// quel campo («stato_motore.x» per un campo di stato_motore), con la radice data: il rif della voce stessa se è la voce
// della radice, "" altrimenti (campiDiversi).
func improntaVoce(radice string, voce json.RawMessage, campo string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(voce, &m); err != nil {
		return ""
	}
	if campo != "" {
		r := map[string]json.RawMessage{"voce": m["voce"], "origine": m["origine"]}
		if sotto, ok := strings.CutPrefix(campo, "stato_motore."); ok {
			var sm map[string]json.RawMessage
			_ = json.Unmarshal(m["stato_motore"], &sm)
			if x, ok := sm[sotto]; ok {
				r["stato_motore"] = raw(map[string]json.RawMessage{sotto: x})
			}
		} else if x, ok := m[campo]; ok {
			r[campo] = x
		}
		m = r
	}
	contenuto := map[string]any{"gruppo": map[string]any{"radice": radice}, "proposta": map[string]any{"nodi": []any{m}}}
	b, err := json.Marshal(contenuto)
	if err != nil {
		return ""
	}
	imp, err := importazione.ImpronteParziali(b)
	if err != nil {
		return ""
	}
	return imp.Contenuto
}

// relazioniPerVoce: le relazioni con la sorgente su un nodo (quelle che entrano nel confronto), per voce della sorgente,
// come testo canonico in ordine.
func relazioniPerVoce(c importazione.Contenuto) map[string]string {
	per := map[string][]string{}
	for _, r := range c.Proposta.Relazioni {
		if r.Sorgente.Tipo != "nodo" {
			continue
		}
		var voce string
		_ = json.Unmarshal(r.Sorgente.Voce, &voce)
		b, err := jsoncanonico.Codifica(r)
		if err != nil {
			continue
		}
		per[voce] = append(per[voce], string(b))
	}
	out := map[string]string{}
	for k, v := range per {
		sort.Strings(v)
		out[k] = strings.Join(v, "\n")
	}
	return out
}

func differenzeArchi(vecchi, nuovi []importazione.ArcoRegistro) []DifferenzaArco {
	chiave := func(a importazione.ArcoRegistro) [3]string { return [3]string{a.Padre, a.Figlio, a.AllegatoID} }
	va, na := map[[3]string]importazione.ArcoRegistro{}, map[[3]string]importazione.ArcoRegistro{}
	for _, a := range vecchi {
		va[chiave(a)] = a
	}
	for _, a := range nuovi {
		na[chiave(a)] = a
	}
	tutte := map[[3]string]bool{}
	for k := range va {
		tutte[k] = true
	}
	for k := range na {
		tutte[k] = true
	}
	ordine := make([][3]string, 0, len(tutte))
	for k := range tutte {
		ordine = append(ordine, k)
	}
	sort.Slice(ordine, func(i, j int) bool {
		for x := 0; x < 3; x++ {
			if ordine[i][x] != ordine[j][x] {
				return ordine[i][x] < ordine[j][x]
			}
		}
		return false
	})
	var out []DifferenzaArco
	for _, k := range ordine {
		a, inA := va[k]
		b, inB := na[k]
		d := DifferenzaArco{AllegatoID: k[2], Padre: k[0], Figlio: k[1]}
		switch {
		case !inA:
			d.Tipo = DifferenzaAggiunto
		case !inB:
			d.Tipo = DifferenzaTolto
		default:
			if (a.Quantita == nil) != (b.Quantita == nil) || (a.Quantita != nil && *a.Quantita != *b.Quantita) {
				d.Campi = append(d.Campi, "quantita")
			}
			if !stessoJSON(a.Occorrenze, b.Occorrenze) {
				d.Campi = append(d.Campi, "occorrenze")
			}
			if !stessoJSON(a.Altre, b.Altre) {
				d.Campi = append(d.Campi, "altre")
			}
			if len(d.Campi) == 0 {
				continue
			}
			d.Tipo = DifferenzaCambiato
		}
		out = append(out, d)
	}
	return out
}

// differenzeGruppo: i campi del gruppo e della fonte che entrano nel confronto e cambiano.
func differenzeGruppo(vecchio, nuovo importazione.Contenuto) []string {
	var out []string
	g, h := vecchio.Gruppo, nuovo.Gruppo
	if g.Radice != h.Radice {
		out = append(out, "radice")
	}
	if !stessoJSON(g.GrafoCompleto, h.GrafoCompleto) {
		out = append(out, "grafo_completo")
	}
	if !stessoJSON(g.MotivoGrafo, h.MotivoGrafo) {
		out = append(out, "motivo_grafo")
	}
	if vecchio.Proposta.Fonte.AllegatoID != nuovo.Proposta.Fonte.AllegatoID || vecchio.Proposta.Fonte.Sha256 != nuovo.Proposta.Fonte.Sha256 {
		out = append(out, "fonte")
	}
	return out
}

// stessoJSON: due valori JSON hanno lo stesso testo canonico (assente, null e vuoto valgono uguale, come nella forma
// canonica del registro).
func stessoJSON(a, b json.RawMessage) bool {
	return bytes.Equal(canonicoOVuoto(a), canonicoOVuoto(b))
}

func canonicoOVuoto(r json.RawMessage) []byte {
	if len(r) == 0 {
		return nil
	}
	c, err := importazione.SenzaVuoti(r)
	if err != nil {
		return r
	}
	switch string(c) {
	case "null", `""`, "[]", "{}":
		return nil
	}
	return c
}

// ConfrontaCaselle: le differenze di una bozza senza registro (da-conferma-1, conversione) con il registro di adesso,
// sullo stesso gruppo corrente (EM §9.1: «le caselle assenti, da scegliere nel riallineamento, e le voci nuove»): una
// casella (la voce n:<allegato_id>:<rif>) che il registro non ha né fra i nodi né fra i fuori per decisione è un nodo
// «tolto»; una voce di proposta.nodi[] che nessuna casella rappresenta è un nodo «aggiunto», cioè una voce di
// nuovi_nodi_proposti (EM §6.5). La voce della radice non è mai «aggiunta»: la rappresenta il nodo del prodotto (P-2),
// anche quando, in una bozza venuta dal formato 2, il nodo del prodotto non ha caselle (EM §6.4.1, §6.7); importarla
// sarebbe un doppione del prodotto (la stessa regola di EffettoDelCambioFonte). Le voci fuori per decisione non sono
// mai «aggiunte»: sono nodi che la proposta lascia fuori. voci sono le voci delle caselle della bozza; l'ordine non
// conta.
func ConfrontaCaselle(voci []string, nuovo importazione.Contenuto) Differenze {
	nella := map[string]bool{}
	for _, v := range voci {
		nella[v] = true
	}
	registro, aggiungibili := map[string]bool{}, map[string]bool{}
	for _, v := range nuovo.Proposta.Nodi {
		registro[v.Voce] = true
		aggiungibili[v.Voce] = !nuovo.EVoceDellaRadice(v)
	}
	for _, x := range nuovo.Proposta.FuoriPerDecisione {
		registro[x.Voce] = true
	}
	var d Differenze
	tutte := map[string]bool{}
	for k := range nella {
		tutte[k] = true
	}
	for _, v := range nuovo.Proposta.Nodi {
		tutte[v.Voce] = true
	}
	for _, k := range ordinate(tutte) {
		switch {
		case nella[k] && !registro[k]:
			d.Nodi = append(d.Nodi, DifferenzaNodo{Voce: k, Tipo: DifferenzaTolto})
		case !nella[k] && aggiungibili[k]:
			d.Nodi = append(d.Nodi, DifferenzaNodo{Voce: k, Tipo: DifferenzaAggiunto})
		}
	}
	return d
}
