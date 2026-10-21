package importazione

// Le impronte del registro (EM §3.8, §3.8.1) per importazione-1:
//   - impronta: lo sha256 del contenuto canonico intero, calcolo compreso: la chiave del registro;
//   - impronta_contenuto: la parte confrontata, struttura e identita' (la striscia «proposta cambiata», l'avviso
//     PROPOSTA_CAMBIATA, il 409 di V-7);
//   - impronta_evidenze: il resto della proposta, solo informativa;
//   - impronta_regole: lo sha256 canonico di regole.
//
// Dentro le letture della parte confrontata (identita_proposta, alternative[], riferimenti[].lettura) l'hash della
// grammatica non entra in impronta_contenuto (B-1 strada (a), U2-P15: CampoFuoriDalConfronto).
//
// L'elenco chiuso sta nelle tabelle di questo file, e la proiezione le legge: un campo che non e' in una tabella non
// entra in nessuna impronta parziale, e la prova L1 dell'elenco chiuso (TestElencoChiusoDelRegistro) fallisce se un
// campo tipizzato del contenuto non ha la sua classe. Le proiezioni sono una scelta tecnica [T] di questo file, perche'
// il §3.8.1 dice quali campi entrano e non con che forma: un oggetto per impronta, con le chiavi dei livelli del
// contenuto (fonte, gruppo, nodi, archi, relazioni, fuori_per_decisione, file, diagnostiche), le voci nell'ordine del
// registro, ognuna con la sua chiave «voce». Le impronte si calcolano sulla forma canonica del registro (FormaCanonica):
// un campo null, "", [] o {} vale come assente a ogni livello (EM §3.10), nodi e archi stanno nel loro ordine (§3.8).

import (
	"bytes"
	"encoding/json"
	"fmt"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Le classi dell'elenco chiuso.
const (
	// InContenuto: il campo entra in impronta_contenuto.
	InContenuto = "contenuto"
	// InEvidenze: il campo entra in impronta_evidenze.
	InEvidenze = "evidenze"
	// InRegole: il campo entra in impronta_regole.
	InRegole = "regole"
	// SoloImprontaIntera: fuori dalle tre impronte parziali (calcolo, contesto, la testata ripetuta, allegati_citati, le
	// tre impronte stesse): entra solo nell'impronta intera.
	SoloImprontaIntera = "solo_impronta_intera"
	// PerSorgente: una relazione entra in impronta_contenuto con sorgente.tipo = nodo, in impronta_evidenze con file.
	PerSorgente = "per_sorgente"
	// Parti: un oggetto i cui campi hanno ognuno la sua classe.
	Parti = "parti"
)

// Le tabelle dell'elenco chiuso, livello per livello. Le classi Parti rimandano alla tabella del livello sotto.
var (
	campiContenuto = map[string]string{
		"formato_registro": SoloImprontaIntera, "versione_importazione": SoloImprontaIntera, "thread_id": SoloImprontaIntera,
		"prodotto_id": SoloImprontaIntera, "gruppo": Parti, "regole": InRegole, "calcolo": SoloImprontaIntera,
		"contesto": SoloImprontaIntera, "proposta": Parti, "allegati_citati": SoloImprontaIntera,
		"impronta_contenuto": SoloImprontaIntera, "impronta_evidenze": SoloImprontaIntera, "impronta_regole": SoloImprontaIntera,
	}
	// O-01: del gruppo, nel confronto solo la radice e la completezza del grafo.
	campiGruppo = map[string]string{
		"chiave": InEvidenze, "tipo": InEvidenze, "allegato_id": InEvidenze, "sha256": InEvidenze, "radice": InContenuto,
		"nome_file": InEvidenze, "stato_struttura": InEvidenze, "radice_del_file": InEvidenze, "compatibilita": InEvidenze,
		"origine": InEvidenze, "grafo_completo": InContenuto, "motivo_grafo": InContenuto,
	}
	campiProposta = map[string]string{
		"fonte": Parti, "nodi": Parti, "archi": InContenuto, "file": InEvidenze, "relazioni": PerSorgente,
		"fuori_per_decisione": Parti, "diagnostiche": InEvidenze,
	}
	campiFonte = map[string]string{
		"allegato_id": InContenuto, "sha256": InContenuto, "nome_ricevuto": InEvidenze, "contenitore_id": InEvidenze,
		"percorso_ricevuto": InEvidenze,
	}
	// La voce: «voce» sta nel confronto, e la proiezione delle evidenze la ripete come chiave della voce.
	campiVoce = map[string]string{
		"voce": InContenuto, "origine": InContenuto, "entita_id": InContenuto, "padri": InContenuto, "foglia": InContenuto,
		"radice_del_file": InContenuto, "leggibile": InContenuto, "stato_motore": Parti, "grezzi": InContenuto,
		"identita_proposta": InContenuto, "codice_proposto": InContenuto, "motivo_proposto": InContenuto,
		"rev_proposta": InContenuto, "motivo_rev": InContenuto, "codice_composto": InContenuto, "motivo_lettura": InContenuto,
		"alternative": InContenuto, "riferimenti": InContenuto, "attributi": InContenuto, "tipo_proposto": InContenuto,
		"segnali": InContenuto, "diagnostiche": InContenuto, "decisione_esistente": InContenuto,
		"abbinamento_proposto": InContenuto, "codice_manuale_legacy": InContenuto, "associazioni_proposte": InEvidenze,
		"fabbisogni_previsti_all_importazione": InEvidenze, "riga_legacy": InEvidenze,
	}
	campiStatoMotore = map[string]string{"raggiunto": InContenuto, "tolto": InContenuto, "deciso": InEvidenze}
	// fuori_per_decisione: voce e segno nel confronto; la traccia (Q13, U2 §4.4) e l'identità proposta con le due
	// proiezioni (U2-P22) fra le evidenze.
	campiFuori = map[string]string{"voce": InContenuto, "segno": InContenuto, "chi": InEvidenze, "quando": InEvidenze, "motivo": InEvidenze,
		"origine_legacy": InEvidenze, "segno_albero": InEvidenze, "archi_tolti": InEvidenze, "mancanti": InEvidenze,
		"identita_proposta": InEvidenze, "codice_proposto": InEvidenze, "motivo_proposto": InEvidenze, "rev_proposta": InEvidenze,
		"motivo_rev": InEvidenze}

	// CampiDellaRadiceFraLeEvidenze: per la voce della radice (origine.rif = gruppo.radice) questi campi, che vengono
	// dallo stato della RFQ, passano dal confronto alle evidenze; con loro stato_motore.raggiunto e tolto e, dei
	// riferimenti documentali, le discordanze e la revisione_inferiore della correzione (EM §3.8.1, O-01).
	CampiDellaRadiceFraLeEvidenze = []string{"decisione_esistente", "abbinamento_proposto", "tipo_proposto",
		"codice_manuale_legacy", "diagnostiche"}
)

// Classificazione restituisce l'elenco chiuso come percorso → classe, con «[]» per gli elementi di un elenco: e'
// quello che la prova L1 confronta con i campi tipizzati del contenuto.
func Classificazione() map[string]string {
	out := map[string]string{}
	livello := func(prefisso string, t map[string]string) {
		for k, c := range t {
			out[prefisso+k] = c
		}
	}
	livello("", campiContenuto)
	livello("gruppo.", campiGruppo)
	livello("proposta.", campiProposta)
	livello("proposta.fonte.", campiFonte)
	livello("proposta.nodi[].", campiVoce)
	livello("proposta.nodi[].stato_motore.", campiStatoMotore)
	livello("proposta.fuori_per_decisione[].", campiFuori)
	return out
}

// Impronte: le tre impronte parziali.
type Impronte struct {
	Contenuto string
	Evidenze  string
	Regole    string
}

// ImpronteParziali calcola le tre impronte parziali di un contenuto (canonico o no: si rilegge e si porta nella forma
// canonica del registro, FormaCanonica, cosi' un vuoto o un ordine diverso non cambiano le impronte) con l'elenco chiuso
// di importazione-1. I campi impronta_* del contenuto non entrano: si possono calcolare prima di scriverli.
func ImpronteParziali(contenuto []byte) (Impronte, error) {
	cont, ev, regole, err := proiezioni(contenuto)
	if err != nil {
		return Impronte{}, err
	}
	var out Impronte
	if out.Contenuto, err = jsoncanonico.ImprontaDi(cont); err != nil {
		return Impronte{}, forma("contenuto", "la parte confrontata non ha una forma canonica: %v", err)
	}
	if out.Evidenze, err = jsoncanonico.ImprontaDi(ev); err != nil {
		return Impronte{}, forma("contenuto", "le evidenze non hanno una forma canonica: %v", err)
	}
	if out.Regole, err = jsoncanonico.ImprontaDi(regole); err != nil {
		return Impronte{}, forma("regole", "le regole non hanno una forma canonica: %v", err)
	}
	return out, nil
}

// ProiezioneEvidenze: la proiezione di impronta_evidenze di un contenuto (l'oggetto di cui impronta_evidenze è lo
// sha256 canonico), con le chiavi dei livelli (fonte, gruppo, nodi, fuori_per_decisione, file, relazioni,
// diagnostiche) e le voci con la loro chiave «voce». Serve a dire dove le evidenze cambiano (differenze.evidenze[] del
// confronto, U2 §5.2) con lo stesso elenco chiuso delle impronte, mai una copia.
func ProiezioneEvidenze(contenuto []byte) (map[string]any, error) {
	_, ev, _, err := proiezioni(contenuto)
	return ev, err
}

// proiezioni: le tre proiezioni di un contenuto (la parte confrontata, le evidenze, le regole) sulla forma canonica del
// registro.
func proiezioni(contenuto []byte) (cont, ev map[string]any, regole any, err error) {
	contenuto, err = FormaCanonica(contenuto)
	if err != nil {
		return nil, nil, nil, forma("contenuto", "il contenuto non si legge: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(contenuto))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		return nil, nil, nil, forma("contenuto", "il contenuto non si legge: %v", err)
	}
	gruppo := oggettoDi(v["gruppo"])
	proposta := oggettoDi(v["proposta"])
	radice, _ := gruppo["radice"].(string)
	fonte := oggettoDi(proposta["fonte"])

	cont = map[string]any{
		"fonte":  scegli(fonte, campiFonte, InContenuto),
		"gruppo": scegli(gruppo, campiGruppo, InContenuto),
		"archi":  elencoONiente(proposta["archi"]),
	}
	ev = map[string]any{
		"fonte":        scegli(fonte, campiFonte, InEvidenze),
		"gruppo":       scegli(gruppo, campiGruppo, InEvidenze),
		"file":         elencoONiente(proposta["file"]),
		"diagnostiche": elencoONiente(proposta["diagnostiche"]),
	}

	nodiC, nodiE := []any{}, []any{}
	for _, x := range elencoDi(proposta["nodi"]) {
		vc, ve := proiettaVoce(oggettoDi(x), radice)
		nodiC = append(nodiC, vc)
		nodiE = append(nodiE, ve)
	}
	cont["nodi"], ev["nodi"] = nodiC, nodiE

	relC, relE := []any{}, []any{}
	for i, x := range elencoDi(proposta["relazioni"]) {
		switch oggettoDi(oggettoDi(x)["sorgente"])["tipo"] {
		case "nodo":
			relC = append(relC, x)
		case "file":
			relE = append(relE, x)
		default:
			return nil, nil, nil, forma(fmt.Sprintf("proposta.relazioni[%d].sorgente.tipo", i), "la sorgente di una relazione e' nodo o file")
		}
	}
	cont["relazioni"], ev["relazioni"] = relC, relE

	fuoriC, fuoriE := []any{}, []any{}
	for _, x := range elencoDi(proposta["fuori_per_decisione"]) {
		m := oggettoDi(x)
		fc, fe := scegli(m, campiFuori, InContenuto), scegli(m, campiFuori, InEvidenze)
		if voce, ok := m["voce"]; ok && voce != nil {
			fe["voce"] = voce
		}
		fuoriC, fuoriE = append(fuoriC, fc), append(fuoriE, fe)
	}
	cont["fuori_per_decisione"], ev["fuori_per_decisione"] = fuoriC, fuoriE
	return cont, ev, v["regole"], nil
}

// proiettaVoce divide una voce fra confronto ed evidenze; la voce della radice passa alle evidenze i campi che vengono
// dallo stato della RFQ.
func proiettaVoce(voce map[string]any, radice string) (map[string]any, map[string]any) {
	vc := scegli(voce, campiVoce, InContenuto)
	ve := scegli(voce, campiVoce, InEvidenze)
	if k, ok := voce["voce"]; ok && k != nil {
		ve["voce"] = k
	}
	sm := oggettoDi(voce["stato_motore"])
	smc, sme := scegli(sm, campiStatoMotore, InContenuto), scegli(sm, campiStatoMotore, InEvidenze)
	rif, _ := oggettoDi(voce["origine"])["rif"].(string)
	if radice != "" && rif == radice {
		for _, k := range CampiDellaRadiceFraLeEvidenze {
			if x, ok := vc[k]; ok {
				ve[k] = x
				delete(vc, k)
			}
		}
		for k, x := range smc {
			sme[k] = x
		}
		smc = map[string]any{}
		if rr, ok := vc["riferimenti"].([]any); ok {
			nuovi, documentali := make([]any, 0, len(rr)), []any{}
			for i, r := range rr {
				m, ok := r.(map[string]any)
				if !ok || m["tipo"] != "documentale" {
					nuovi = append(nuovi, r)
					continue
				}
				copia := copiaOggetto(m)
				parte := map[string]any{}
				if d, ok := copia["discordanze"]; ok {
					if d != nil {
						parte["discordanze"] = d
					}
					delete(copia, "discordanze")
				}
				if corr, ok := copia["correzione"].(map[string]any); ok {
					if ri, ok := corr["revisione_inferiore"]; ok {
						cc := copiaOggetto(corr)
						delete(cc, "revisione_inferiore")
						copia["correzione"] = cc
						if ri != nil {
							parte["revisione_inferiore"] = ri
						}
					}
				}
				if len(parte) > 0 {
					parte["indice"] = i
					documentali = append(documentali, parte)
				}
				nuovi = append(nuovi, copia)
			}
			vc["riferimenti"] = nuovi
			if len(documentali) > 0 {
				ve["riferimenti_documentali"] = documentali
			}
		}
	}
	senzaHashGrammatica(vc)
	vc["stato_motore"], ve["stato_motore"] = smc, sme
	return vc, ve
}

// CampoFuoriDalConfronto: il campo di una lettura che resta nel registro ma non entra nella parte confrontata (B-1,
// strada (a); U2-P15): l'hash della grammatica del suo emittente. Una grammatica nuova che non cambia nessuna lettura
// cambia impronta_regole («regole cambiate»), mai impronta_contenuto; l'hash resta nella lettura del registro, in
// regole e nell'impronta intera.
const CampoFuoriDalConfronto = "hash_grammatica"

// senzaHashGrammatica toglie l'hash della grammatica dalle letture della parte confrontata di una voce: l'identità
// proposta, le alternative e la lettura di ogni riferimento. Lavora su copie: la voce del registro non cambia.
func senzaHashGrammatica(vc map[string]any) {
	togli := func(v any) any {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		if _, c := m[CampoFuoriDalConfronto]; !c {
			return m
		}
		cp := copiaOggetto(m)
		delete(cp, CampoFuoriDalConfronto)
		return cp
	}
	if x, ok := vc["identita_proposta"]; ok {
		vc["identita_proposta"] = togli(x)
	}
	if l, ok := vc["alternative"].([]any); ok {
		n := make([]any, len(l))
		for i, x := range l {
			n[i] = togli(x)
		}
		vc["alternative"] = n
	}
	if l, ok := vc["riferimenti"].([]any); ok {
		n := make([]any, len(l))
		for i, r := range l {
			m, ok := r.(map[string]any)
			if !ok {
				n[i] = r
				continue
			}
			if lt, ok := m["lettura"]; ok {
				cp := copiaOggetto(m)
				cp["lettura"] = togli(lt)
				n[i] = cp
				continue
			}
			n[i] = r
		}
		vc["riferimenti"] = n
	}
}

func oggettoDi(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func elencoDi(v any) []any {
	l, _ := v.([]any)
	return l
}

func elencoONiente(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{}
}

// scegli prende i campi di m della classe data nella tabella; un valore null vale come assente.
func scegli(m map[string]any, tabella map[string]string, classe string) map[string]any {
	out := map[string]any{}
	for k, x := range m {
		if tabella[k] == classe && x != nil {
			out[k] = x
		}
	}
	return out
}

func copiaOggetto(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, x := range m {
		c[k] = x
	}
	return c
}
