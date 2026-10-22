package web

// distinta_unica_banco_r45_test.go: R4 e R5 del banco finto nella forma del server formato 3 (corsia persistenza:
// distintabozza.RiepilogoR4 e ConfermaR5, motore_a_bozza_v1_r4.go e _r5.go; direttiva 39 §1, §4, §5, §7 [U + T-39]). Il
// banco calcola solo quanto basta perché i flussi della pagina si vedano nel browser: il gate della sorgente
// STEP_SORGENTE_NON_CONFERMATA (acceso come nel server; l'interruttore «gate-sorgente» resta per le prove che lo spengono),
// un Piano semplificato delle preassociazioni (esiti e motivi del server: in_conflitto con codice_diverso, codice_non_letto,
// legacy_da_verificare, gia_associata_altrove nelle sue varianti, file_non_attivo; gia_applicata; sostituzione con lo
// svincolo nella stessa bozza; non_richiesta; in_attesa_codice), i gruppi D-PB-04 dalla provenienza delle PB (quella che il
// server decide: il banco la legge dallo snapshot, o la deduce dal registro dichiarato, o «manuale»), gli spostamenti di AR,
// gli esclusi (non_includere, escludi, assenza), la riga «legato come sorgente», condivisione[] (con l'interruttore
// «condivisione»). Le regole vere le provano le L4 della persistenza.
//
// Campi del banco che il server non dà ancora (anticipati e dichiarati nel resoconto della corsia, prova
// TestBancoDistintaUnicaR45InPari): effetti_categoria (U2 §6.4.2) e identita[].decisa.identificativo_cliente (U2 §4.2).
// Dal blocco «R4 completo» della persistenza ingressi è del server, nella forma di R1 ?ingressi=1 (prodotto_id,
// algoritmo, impronta_ingressi, parti), e il riepilogo è confermabile solo con gli avvisi del motore calcolati (la firma li
// comprende: il riepilogo leggero non si conferma); R5 ricalcola il riepilogo con il motore e risponde 503 se il motore è
// occupato, dopo il gate.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// bdCampiR4SoloBanco: i campi del riepilogo che il banco anticipa e il server non dà ancora.
var bdCampiR4SoloBanco = []string{"effetti_categoria"}

// bdEffettiEscluso: gli effetti di un'esclusione nella forma del server (EffettiEsclusoR4, blocco «R4 completo»): le
// occorrenze toccate, le righe dello STEP che R5 chiuderà (una per casella nel banco), nessun arco legacy, nessun figlio
// (niente cascata), nessun fabbisogno tolto.
func bdEffettiEscluso(c m) m {
	righe := []any{}
	occ := []any{}
	if c != nil {
		occ = append(occ, c["allegato_id"])
		righe = append(righe, m{"proposta_id": "00000000-0000-4000-8000-" + bdSha("riga|" + bdStr(c["allegato_id"]) + "|" + bdStr(c["rif"]))[:12], "allegato_id": c["allegato_id"]})
	}
	return m{"occorrenze_toccate": occ, "prodotti_con_lo_stesso_step": []any{}, "righe_da_chiudere": righe, "archi_da_chiudere": []any{}, "figli": []any{},
		"fabbisogni_tolti_dopo_conferma": []any{}}
}

// bdGruppoPB: il gruppo D-PB-04 di una PB e l'elenco di effetti_documentali che la porta (distintabozza.gruppoDi; R39-15:
// la legacy riconosciuta sta in legacy_verificate).
func bdGruppoPB(prov string, verificata bool) (gruppo, elenco string) {
	switch prov {
	case "motore":
		return "motore_intatta", "motore_intatta"
	case "corretta":
		return "corretta", "corretta"
	case "legacy":
		if verificata {
			return "legacy_verificata", "legacy_verificate"
		}
		return "legacy_da_verificare", "legacy_da_verificare"
	}
	return "manuale", "manuale"
}

// testi del gate (distintabozza.testiGate)
var bdTestiGate = map[string]string{
	"senza_sorgente": "La bozza non ha uno STEP confermato come sorgente: confermalo dalla Sorgente («Conferma questo STEP come sorgente»), poi conferma la BOM.",
	"non_confermata": "Lo STEP della bozza non è confermato come sorgente da S8: confermalo dalla Sorgente, poi conferma la BOM.",
	"revocata":       "La sorgente della bozza è stata revocata: conferma di nuovo uno STEP come sorgente dalla Sorgente.",
	"rifiutata":      "La sorgente della bozza è stata rifiutata: conferma uno STEP come sorgente dalla Sorgente.",
	"superata":       "La conferma della sorgente è superata da quella di un altro STEP: riconferma lo STEP della bozza o cambia la sorgente.",
	"non_coerente":   "Lo STEP della bozza non corrisponde più al documento confermato: riconfermalo dalla Sorgente.",
}

// gateSorgenteR4: lo stato della sorgente della versione (SorgenteR4) e il blocco firmato, se scatta.
func (b *bancoDU) gateSorgenteR4(prod string, snap m) (m, m) {
	s, _ := snap["sorgente"].(m)
	sorg := m{"valida": false}
	if s != nil {
		sorg["allegato_id"], sorg["sha256"], sorg["gruppo"] = s["allegato_id"], s["sha256"], s["gruppo"]
	}
	valida := b.s8Valida(prod)
	motivo := ""
	switch {
	case s == nil:
		motivo = "senza_sorgente"
	case valida == "" && b.s8[prod] != nil && bdStr(b.s8[prod]["allegato_id"]) == bdStr(s["allegato_id"]):
		motivo = bdStr(b.s8[prod]["motivo"])
		if motivo == "" {
			motivo = "non_confermata"
		}
	case valida != bdStr(s["allegato_id"]):
		motivo = "non_confermata"
	}
	if motivo == "" {
		sorg["valida"] = true
		sorg["documento_id"] = b.s8[prod]["documento_id"]
		return sorg, nil
	}
	sorg["motivo"] = motivo
	if !b.gateSorgente {
		return sorg, nil
	}
	o := m{"codice": "STEP_SORGENTE_NON_CONFERMATA", "motivo": motivo, "testo": bdTestiGate[motivo], "rimedi": []any{"«Conferma questo STEP come sorgente» del prodotto (S8)"}}
	if s != nil {
		o["sorgente"] = m{"allegato_id": s["allegato_id"], "sha256": s["sha256"]}
	}
	return sorg, o
}

// riepilogo: R4 formato 3 (commento di testa).
func (b *bancoDU) riepilogo(prod string, snap m, versione int, motore bool) m {
	bz := b.bozze[prod]
	var reg m
	if bz != nil {
		reg = b.registri[bz.Registro]
	}
	voci := map[string]m{}
	if reg != nil {
		for _, x := range reg["proposta"].(m)["nodi"].([]any) {
			v := x.(m)
			voci[bdStr(v["voce"])] = v
		}
	}
	nodi := bdElenco(snap["nodi"])
	decisioni := bdElenco(snap["decisioni"])
	archiSnap := bdElenco(snap["archi"])
	preSnap := bdElenco(snap["documenti_preassociati"])
	codiceDi, revDi, tipoDi, compDi, opDi := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	figli, padri := map[string]int{}, map[string]int{}
	for _, x := range archiSnap {
		a := x.(m)
		figli[bdStr(a["padre"])]++
		padri[bdStr(a["figlio"])]++
	}
	nodoDelComp := map[string]string{}
	// l'identita' e le voci del registro delle caselle sulla fonte del registro, per la PB del motore intatta (D-48-01 A [T])
	fonteReg := ""
	if reg != nil {
		if fo, _ := reg["proposta"].(m)["fonte"].(m); fo != nil {
			fonteReg = bdStr(fo["allegato_id"])
		}
	}
	idNodo, vociDi := map[string]m{}, map[string][]m{}
	for _, x := range nodi {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		idNodo[k], _ = n["identita"].(m)
		for _, y := range bdElenco(n["caselle"]) {
			c := y.(m)
			if v := voci["n:"+bdStr(c["allegato_id"])+":"+bdStr(c["rif"])]; v != nil && fonteReg != "" && strings.EqualFold(bdStr(c["allegato_id"]), fonteReg) {
				vociDi[k] = append(vociDi[k], v)
			}
		}
	}
	for _, x := range nodi {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		id, _ := n["identita"].(m)
		codiceDi[k], revDi[k], tipoDi[k], compDi[k], opDi[k] = bdStr(id["codice"]), bdStr(id["rev"]), bdStr(n["tipo"]), bdStr(n["componente_id"]), bdStr(n["op"])
		if c := bdStr(n["componente_id"]); c != "" {
			nodoDelComp[c] = k
		}
	}
	// i file con l'associazione registrata (AR) attiva sul loro componente (f.Componente), e quanti per componente
	arDi := map[string]int{}
	for _, f := range b.file {
		if f.Componente != "" && f.Documento == nil {
			arDi[f.Componente]++
		}
	}
	nPre := map[string]int{}
	for _, x := range preSnap {
		nPre[bdStr(x.(m)["chiave_bozza"])]++
	}
	// CREA_SU_ESISTENTE (EM §5.2 W3; C3:514) e il suo rimedio riusa_esistente (EM §3.6 riga 8): il banco conosce un solo
	// registrato fuori dalla radice, ACM-1400 di ACM-1000 (bdC40). Un nodo da creare con quel codice senza il riuso blocca,
	// con o senza un nodo del componente nella bozza (W3/W4 e il caso a coppie del server); con il riuso il nodo va nel
	// componente (componente_id dell'identità, come componenteDi del server). Regola semplificata: niente emittenti né
	// identità (W3 vera: persistenza)
	riusa := map[string]string{}
	for _, x := range decisioni {
		if d := x.(m); bdStr(d["tipo"]) == "riusa_esistente" {
			riusa[bdStr(d["chiave_bozza"])] = strings.ToLower(bdStr(d["componente_id"]))
		}
	}
	registrati := map[string]string{}
	if prod == bdP1 {
		registrati["ACM-1400"] = bdC40
	}
	identita := []any{}
	blocchi := []any{}
	presenti := map[string]bool{}
	segnalatiInclusi := []any{}
	for _, x := range nodi {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		id, _ := n["identita"].(m)
		codice := codiceDi[k]
		var voce m
		if cs, ok := n["caselle"].([]any); ok && len(cs) > 0 {
			c := cs[0].(m)
			chiave := "n:" + bdStr(c["allegato_id"]) + ":" + bdStr(c["rif"])
			presenti[chiave] = true
			voce = voci[chiave]
			if voce != nil && len(voce["segnali"].([]any)) > 0 && opDi[k] == "crea" {
				segnalatiInclusi = append(segnalatiInclusi, k)
			}
		}
		esito := map[string]string{"proposta": "accettata", "registrato": "registrata", "manuale": "manuale", "da_decidere": "da_assegnare", "decisione": "corretta"}[bdStr(id["modo"])]
		// l'identificativo del cliente (U2 §4.2 punto 4): il banco usa la regola della fixture ACME, «codice/revisione»
		// (campo anticipato dal banco: il server di oggi non lo dà)
		idc := ""
		if codice != "" && strings.HasPrefix(bdStr(id["emittente"]), "cliente:") {
			idc = codice
			if rev := revDi[k]; rev != "" {
				idc += "/" + rev
			}
		}
		var cid any
		if compDi[k] != "" {
			cid = compDi[k]
		} else if riusa[k] != "" {
			cid = riusa[k]
		}
		decisa := m{"emittente": id["emittente"], "codice": codice, "modo": id["modo"], "identificativo_cliente": idc}
		if id["emittente_presunto"] == true {
			decisa["emittente_presunto"] = true
		}
		for _, c := range []string{"namespace", "rev", "decisione"} {
			if v := bdStr(id[c]); v != "" {
				decisa[c] = v
			}
		}
		voceId := m{"chiave_bozza": k, "componente_id": cid, "azione": n["op"], "origine": n["origine"], "codice": m{"prima": nil, "dopo": codice},
			"rev": m{"prima": nil, "dopo": revDi[k]}, "descrizione": m{"prima": nil, "dopo": bdStr(n["descrizione"])}, "tipo": m{"prima": nil, "dopo": n["tipo"]},
			"decisa": decisa, "esito_identita": esito, "collisioni": []any{}, "documenti": m{"preassociati": nPre[k], "ar_attive": arDi[compDi[k]]},
			"non_ancora_confermato": opDi[k] == "crea", "differenze_da_ultima_conferma": []any{}, "padri": padri[k]}
		if voce != nil && voce["identita_proposta"] != nil {
			ip := voce["identita_proposta"].(m)
			voceId["proposta"] = m{"letterale": ip["letterale"], "emittente": ip["emittente"], "namespace": ip["namespace"], "codice_proposto": voce["codice_proposto"]}
		}
		identita = append(identita, voceId)
		if opDi[k] == "crea" && codice == "" {
			blocchi = append(blocchi, m{"codice": "CODICE_DA_ASSEGNARE", "chiave_bozza": k, "testo": "Il pezzo «" + bdStr(n["descrizione"]) + "» non ha un codice: assegnalo prima di confermare.",
				"rimedi": []any{"correggi il codice del pezzo (con il motivo, se è importato)", "non includerlo, se non è un articolo"}})
		}
		if c := registrati[strings.ToUpper(codice)]; c != "" && opDi[k] == "crea" && riusa[k] != c {
			blocchi = append(blocchi, m{"codice": "CREA_SU_ESISTENTE", "chiave_bozza": k, "componente_id": c, "chiave_working": strings.ToUpper(codice),
				"testo":  "Il codice di " + codice + " è già di un componente registrato (" + codice + "): riusalo, oppure cambia il codice.",
				"rimedi": []any{"«Riusa i componenti con lo stesso codice» (riusa_esistente)", "cambia il codice del nodo, con il motivo"}})
		}
	}
	// gli esclusi (EM §7.4 con U2 §6.1): le non_includere, gli escludi e le voci della proposta assenti dalla bozza
	esclusi := []any{}
	for _, x := range decisioni {
		d := x.(m)
		switch bdStr(d["tipo"]) {
		case "non_includere":
			c := d["casella"].(m)
			presenti["n:"+bdStr(c["allegato_id"])+":"+bdStr(c["rif"])] = true
			e := m{"origine": "non_includere", "id": d["id"], "casella": c, "motivo_codice": d["motivo_codice"], "motivo": d["motivo"], "azione_id": d["azione_id"],
				"effetti": bdEffettiEscluso(c)}
			if d["segnale_visto"] != nil {
				e["segnale_visto"] = d["segnale_visto"]
			}
			esclusi = append(esclusi, e)
		case "escludi":
			esclusi = append(esclusi, m{"origine": "escludi", "id": d["id"], "componente_id": d["componente_id"], "codice": codiceDi[nodoDelComp[bdStr(d["componente_id"])]], "motivo": d["motivo"]})
		}
	}
	if s, _ := snap["sorgente"].(m); s != nil && reg != nil {
		for chiave, v := range voci {
			if sm, _ := v["stato_motore"].(m); presenti[chiave] || sm["tolto"] == true {
				continue
			}
			o := v["origine"].(m)
			c := m{"allegato_id": o["allegato_id"], "gruppo": s["gruppo"], "rif": o["rif"]}
			esclusi = append(esclusi, m{"origine": "assenza", "casella": c, "motivo_mancante": true, "effetti": bdEffettiEscluso(c)})
		}
	}
	// interruttore «r4-completo»: le esclusioni legacy (scartata nelle pagine di prima, con l'autore) ed ereditata (una
	// non_includere della conferma di base che la bozza non ripete), solo traccia
	if b.r4Completo {
		esclusi = append(esclusi, m{"origine": "legacy", "casella": m{"allegato_id": bdA1, "gruppo": "legacy", "rif": "#90"}, "motivo": "scartata a mano nella pagina di prima", "autore_id": duUUID(78).String()},
			m{"origine": "ereditata", "id": "ni-ereditata", "casella": m{"allegato_id": bdA1, "gruppo": "legacy", "rif": "#91"}, "motivo_codice": "elemento_cad", "motivo": "esclusa dalla conferma di base"})
	}
	avvisi := []any{m{"codice": "VERIFICA_DOPO_COMMIT", "testo": "La verifica della BOM si calcola sulla struttura confermata, dopo il commit."}}
	// le regole minime del banco per D1, mono_pezzo ed esclusi con documenti (EM §8.1, §8.2)
	d1 := map[string]string{}
	monoPezzo := false
	svincolati := map[string]string{} // allegato → componente dello svincolo
	svincoloDec := map[string]string{}
	for _, x := range decisioni {
		d := x.(m)
		switch bdStr(d["tipo"]) {
		case "d1_a_particolare", "d1_resta_sottoassieme", "d1_a_sottoassieme":
			d1[bdStr(d["chiave_bozza"])] = bdStr(d["tipo"])
		case "mono_pezzo":
			monoPezzo = true
		case "scollega_documento":
			svincolati[bdStr(d["allegato_id"])] = bdStr(d["componente_id"])
			svincoloDec[bdStr(d["allegato_id"])] = bdStr(d["id"])
		}
	}
	effetti := m{"motore_intatta": []any{}, "corretta": []any{}, "manuale": []any{}, "legacy_da_verificare": []any{}, "legacy_verificate": []any{}, "proposte_rimosse": []any{},
		"spostamenti_ar": []any{}, "ar_invariate": []any{}}
	aggiungi := func(g string, v m) { effetti[g] = append(effetti[g].([]any), v) }
	for _, x := range nodi {
		n := x.(m)
		k := bdStr(n["chiave_bozza"])
		if compDi[k] == prod {
			if figli[k] == 0 && !monoPezzo {
				avvisi = append(avvisi, m{"codice": "PRODOTTO_SENZA_FIGLI", "chiave_bozza": k, "testo": "Il prodotto non ha figli nella bozza: se è un pezzo unico, dichiara mono_pezzo con il motivo."})
			}
			continue
		}
		var voce m
		if cs, ok := n["caselle"].([]any); ok && len(cs) > 0 {
			c := cs[0].(m)
			voce = voci["n:"+bdStr(c["allegato_id"])+":"+bdStr(c["rif"])]
		}
		tipo, op := tipoDi[k], opDi[k]
		if op == "esclude" {
			mantenute := 0
			for _, f := range b.file {
				if f.Componente != "" && f.Componente == compDi[k] && f.Documento == nil && svincolati[f.ID] == "" {
					mantenute++
					aggiungi("ar_invariate", m{"allegato_id": f.ID, "componente_id": f.Componente, "associazione_id": bdProposta(f.ID), "motivo": "escluso_mantieni",
						"testo": "Il componente esce dalla BOM e l'associazione resta storica («Mantieni»)."})
				}
			}
			if mantenute > 0 {
				avvisi = append(avvisi, m{"codice": "ESCLUSO_CON_DOCUMENTI", "chiave_bozza": k, "testo": fmt.Sprintf("%s esce dalla BOM con %d associazioni di file registrate: restano storiche.", codiceDi[k], mantenute)})
			}
			continue
		}
		if tipo == "sottoassieme" && figli[k] == 0 && voce != nil && voce["foglia"] == false && d1[k] != "d1_a_particolare" && d1[k] != "d1_resta_sottoassieme" {
			blocchi = append(blocchi, m{"codice": "D1_ULTIMO_FIGLIO", "chiave_bozza": k, "testo": codiceDi[k] + " è un assieme rimasto senza figli: diventa un particolare, oppure resta assieme.",
				"rimedi": []any{"Diventa particolare", "Lascia assieme"}})
		}
		if (tipo == "sciolto" || tipo == "commerciale") && figli[k] > 0 && d1[k] != "d1_a_sottoassieme" {
			blocchi = append(blocchi, m{"codice": "D1_PARTICOLARE_CON_FIGLI", "chiave_bozza": k, "testo": codiceDi[k] + " è un particolare con figli: diventa un assieme.", "rimedi": []any{"Diventa assieme"}})
		}
	}
	if len(segnalatiInclusi) > 0 {
		avvisi = append(avvisi, m{"codice": "SEGNALATI_INCLUSI", "testo": fmt.Sprintf("%d nodi segnalati come possibili elementi CAD stanno per diventare componenti.", len(segnalatiInclusi)), "chiavi": segnalatiInclusi})
	}
	archi := []any{}
	for _, x := range archiSnap {
		a := x.(m)
		p, f := bdStr(a["padre"]), bdStr(a["figlio"])
		estremo := func(k string) m {
			e := m{"chiave_bozza": k, "codice": codiceDi[k], "tipo": tipoDi[k]}
			if compDi[k] != "" {
				e["componente_id"] = compDi[k]
			}
			return e
		}
		archi = append(archi, m{"padre": estremo(p), "figlio": estremo(f), "qta_precedente": nil, "qta_voluta": a["qta"], "azione": "aggiunto", "origine": a["origine"]})
	}
	// la PB del motore intatta (D-48-01 A [T], distintabozza.pbDalMotore): provenienza motore, componente non ricodificato, la
	// voce di una casella del nodo propone il file ed e' l'unica del registro a proporlo, e il nodo ha l'identita' di quella
	// voce (proposta: emittente, namespace, codice e rev proposti; registrato: il componente della decisione_esistente, il
	// prodotto per la voce della radice)
	ricodificati := map[string]bool{}
	for _, x := range decisioni {
		if d := x.(m); bdStr(d["tipo"]) == "ricodifica" {
			ricodificati[strings.ToLower(bdStr(d["componente_id"]))] = true
		}
	}
	radice := ""
	if reg != nil {
		if g, _ := reg["gruppo"].(m); g != nil {
			radice = bdStr(g["radice"])
		}
	}
	propone := func(v m, aid string) bool {
		for _, y := range bdElenco(v["associazioni_proposte"]) {
			if strings.EqualFold(bdStr(y.(m)["allegato_id"]), aid) {
				return true
			}
		}
		return false
	}
	destinazioni := func(aid string) int {
		rif := map[string]bool{}
		for _, v := range voci {
			if propone(v, aid) {
				o, _ := v["origine"].(m)
				rif[bdStr(o["rif"])] = true
			}
		}
		return len(rif)
	}
	dalMotore := func(prov, k, aid string) bool {
		if prov != "motore" || ricodificati[strings.ToLower(compDi[k])] || destinazioni(aid) != 1 {
			return false
		}
		id := idNodo[k]
		for _, v := range vociDi[k] {
			if !propone(v, aid) {
				continue
			}
			switch bdStr(id["modo"]) {
			case "proposta":
				ip, _ := v["identita_proposta"].(m)
				return ip != nil && bdStr(v["codice_proposto"]) != "" && bdStr(id["emittente"]) == bdStr(ip["emittente"]) &&
					bdStr(id["namespace"]) == bdStr(ip["namespace"]) && bdStr(id["codice"]) == bdStr(v["codice_proposto"]) &&
					bdStr(id["rev"]) == bdStr(v["rev_proposta"])
			case "registrato":
				atteso := ""
				if de, _ := v["decisione_esistente"].(m); de != nil {
					atteso = bdStr(de["componente_id"])
				}
				if o, _ := v["origine"].(m); o != nil && radice != "" && bdStr(o["rif"]) == radice {
					atteso = prod
				}
				return atteso != "" && strings.EqualFold(atteso, compDi[k])
			}
			return false
		}
		return false
	}
	// il Piano semplificato delle PB (associazioni.Piano)
	pre := []any{}
	for _, x := range preSnap {
		d := x.(m)
		aid, k := bdStr(d["allegato_id"]), bdStr(d["chiave_bozza"])
		f := b.fileDi(aid)
		if f == nil {
			continue
		}
		// la provenienza la decide il server (D-PB03 A [U]): quella della versione salvata per la stessa PB, poi il registro
		// dichiarato con un suggerimento accettato (motore), altrimenti manuale; il corpo non è un'autorità
		prov := ""
		verificata := false
		if bz != nil {
			for _, y := range bdElenco(bz.Snapshot["documenti_preassociati"]) {
				if s := y.(m); bdStr(s["allegato_id"]) == aid && bdStr(s["chiave_bozza"]) == k {
					prov = bdStr(s["provenienza"])
					// R39-15: la verifica di una PB legacy la decide il server con il riconoscimento (file_riconosciuto)
					verificata = prov == "legacy" && s["verificata"] == true
					if prov != "" && bdStr(d["importazione"]) == "" && bdStr(s["importazione"]) != "" {
						d = m{"allegato_id": aid, "chiave_bozza": k, "importazione": s["importazione"]}
					}
				}
			}
		}
		if prov == "" {
			prov = "manuale"
			// il registro dichiarato con la PB accettata e non salvata: motore sul nodo proposto, corretta su un altro (R4 con
			// il motore, come R2: provenienzaDelRegistro); un registro che non la propone resta manuale
			if imp := bdStr(d["importazione"]); imp != "" {
				if p := b.provenienzaDelRegistro(imp, aid, k, snap); p != "" {
					prov = p
				}
			}
		}
		gruppo, elenco := bdGruppoPB(prov, verificata)
		e := m{"allegato_id": aid, "chiave_bozza": k, "provenienza": prov, "gruppo": gruppo, "nome_file": f.Nome}
		if verificata {
			e["verificata"] = true
		}
		if imp := bdStr(d["importazione"]); imp != "" {
			e["importazione"] = imp
		}
		if compDi[k] != "" {
			e["componente_id"] = compDi[k]
		}
		if f.Codice != "" {
			e["codice_letto"] = f.Codice
		}
		if f.Rev != "" {
			e["rev_letta"] = f.Rev
		}
		esito, motivo, variante := "applicabile", "", ""
		norm := func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
		switch {
		case opDi[k] == "esclude" || tipoDi[k] == "commerciale":
			esito = "non_richiesta"
		case f.Stato != "attivo":
			esito, motivo = "in_conflitto", "file_non_attivo"
		case b.s8Valida(prod) == aid:
			esito, motivo = "in_conflitto", "legato_come_sorgente"
		case prov == "legacy" && !verificata:
			esito, motivo = "in_conflitto", "legacy_da_verificare"
		case f.Componente != "" && compDi[k] != "" && f.Componente == compDi[k]:
			esito = "gia_applicata"
			e["associazione_id"] = bdProposta(aid)
		case f.Componente != "" && f.Documento == nil && svincolati[aid] == f.Componente && nodoDelComp[f.Componente] != "":
			esito = "sostituzione"
			e["associazione_id"], e["ar_componente_id"] = bdProposta(aid), f.Componente
			aggiungi("spostamenti_ar", m{"tipo": "sostituzione", "allegato_id": aid, "associazione_id": bdProposta(aid), "da_componente_id": f.Componente, "a_chiave_bozza": k, "esito": "sostituzione"})
		case f.Componente != "" && f.Documento == nil:
			esito, motivo, variante = "in_conflitto", "gia_associata_altrove", "altro_prodotto"
			if nodoDelComp[f.Componente] != "" || f.Componente == bdC40 {
				variante = "stesso_prodotto"
			}
			e["associazione_id"], e["ar_componente_id"], e["ar_prodotti"] = bdProposta(aid), f.Componente, []any{prod}
		case f.Documento != nil:
			esito, motivo = "in_conflitto", "documento_confermato_altrove"
		case codiceDi[k] == "":
			esito = "in_attesa_codice"
		case dalMotore(prov, k, aid):
			// D-48-01 A [T] (associazioni.VocePB.DalMotore): la PB del motore intatta non si riconfronta con il codice letto
		case f.Codice == "":
			esito, motivo = "in_conflitto", "codice_non_letto"
		case norm(f.Codice) != norm(codiceDi[k]) || (f.Rev != "" && revDi[k] != "" && norm(f.Rev) != norm(revDi[k])):
			esito, motivo = "in_conflitto", "codice_diverso"
		}
		e["esito"] = esito
		if motivo != "" {
			e["motivo"] = motivo
		}
		if variante != "" {
			e["variante"] = variante
		}
		e["effetto"] = map[string]string{"applicabile": "associazione registrata con la conferma (non è la conferma del documento)", "gia_applicata": "già registrata: nessuna riga nuova",
			"sostituzione": "l'associazione di prima si chiude e la nuova si registra con la conferma", "non_richiesta": "non si applica: il documento non è richiesto per questo nodo",
			"in_attesa_codice": "in attesa del codice del nodo", "in_conflitto": "non si applica finché il conflitto non è risolto"}[esito]
		pre = append(pre, e)
		g := m{"allegato_id": aid, "chiave_bozza": k, "esito": esito}
		if motivo != "" {
			g["motivo"] = motivo
		}
		if imp := bdStr(d["importazione"]); imp != "" {
			g["importazione"] = imp
		}
		aggiungi(elenco, g)
		if esito == "in_conflitto" {
			testi := map[string]string{"file_non_attivo": "non è più un file attivo della RFQ (Altri file o scartato)", "legato_come_sorgente": "è lo STEP legato come sorgente di un prodotto finito: non è un documento da associare",
				"legacy_da_verificare": "viene da una pre-assegnazione delle pagine di prima: va verificata (riconoscila, toglila o rifalla a mano)", "gia_associata_altrove": "è già associato a un altro componente",
				"documento_confermato_altrove": "è già un documento confermato di un altro componente", "codice_non_letto": "non ha un codice letto",
				"codice_diverso": "ha un codice o una revisione diversi da quelli del pezzo"}
			rimedi := []any{"togli la preassociazione"}
			switch motivo {
			case "gia_associata_altrove":
				if variante == "stesso_prodotto" {
					rimedi = append(rimedi, "nella stessa bozza scollega il file dal componente di prima e preassocialo qui (sostituzione)")
				} else {
					rimedi = append(rimedi, "scollega il file nella bozza dell'altro prodotto e conferma quella, poi questa")
				}
			case "codice_non_letto":
				rimedi = append(rimedi, "riclassifica il file da «Da verificare»")
			case "codice_diverso":
				rimedi = append(rimedi, "correggi il codice del pezzo o del file")
			case "legacy_da_verificare":
				rimedi = []any{"riconoscila: resta legacy e diventa verificata (file_riconosciuto, R2)", "togli la preassociazione", "rifalla a mano (diventa manuale)"}
			}
			bl := m{"codice": "PREASSOCIAZIONE_NON_APPLICABILE", "motivo": motivo, "chiave_bozza": k, "allegato_id": aid, "testo": "Il file «" + f.Nome + "» " + testi[motivo] + ".", "rimedi": rimedi}
			if variante != "" {
				bl["variante"], bl["componente_id"], bl["prodotti"] = variante, f.Componente, []any{prod}
			}
			blocchi = append(blocchi, bl)
		}
	}
	// gli svincoli (associazioni.EsitoSvincolo)
	svincoli := []any{}
	for _, x := range decisioni {
		d := x.(m)
		if bdStr(d["tipo"]) != "scollega_documento" {
			continue
		}
		aid := bdStr(d["allegato_id"])
		f := b.fileDi(aid)
		o := m{"allegato_id": aid, "componente_id": d["componente_id"], "esito": "applicabile"}
		if dd := bdStr(d["da_decisione"]); dd != "" {
			o["da_decisione"] = dd
		}
		switch {
		case f != nil && f.Documento != nil:
			o["esito"], o["motivo"] = "in_conflitto", "svincolo_su_documento"
			blocchi = append(blocchi, m{"codice": "PREASSOCIAZIONE_NON_APPLICABILE", "motivo": "svincolo_su_documento", "allegato_id": aid, "componente_id": d["componente_id"],
				"testo": "«" + f.Nome + "» è un documento confermato: non si scollega da qui (serve una revisione, non in V1)."})
		case f != nil && f.Componente == "":
			o["esito"] = "svincolo_gia_chiuso"
		case f != nil && f.Componente != bdStr(d["componente_id"]):
			o["esito"], o["motivo"] = "in_conflitto", "svincolo_componente_diverso"
		default:
			o["associazione_id"] = bdProposta(aid)
		}
		svincoli = append(svincoli, o)
		// lo svincolo di una sostituzione sta già fra gli spostamenti come «sostituzione»
		sostituita := false
		for _, y := range effetti["spostamenti_ar"].([]any) {
			if y.(m)["allegato_id"] == aid {
				sostituita = true
			}
		}
		if !sostituita {
			sp := m{"tipo": "svincolo", "allegato_id": aid, "da_componente_id": d["componente_id"], "esito": o["esito"], "da_decisione": svincoloDec[aid]}
			if o["motivo"] != nil {
				sp["motivo"] = o["motivo"]
			}
			aggiungi("spostamenti_ar", sp)
		}
	}
	// le proposte del motore entrate nella bozza (PB iniziali, file_preassociato del server) e non più nello snapshot
	if bz != nil {
		nello := map[string]bool{}
		for _, x := range preSnap {
			nello[bdStr(x.(m)["allegato_id"])] = true
		}
		for _, ev := range bz.Eventi {
			if bdStr(ev["azione"]) != "file_preassociato" || bdStr(ev["origine"]) != "server" {
				continue
			}
			if d, _ := ev["dopo"].(m); d != nil && !nello[bdStr(d["allegato_id"])] {
				aggiungi("proposte_rimosse", m{"allegato_id": d["allegato_id"], "chiave_bozza": d["chiave_bozza"], "importazione": d["importazione"]})
			}
		}
	}
	// interruttore «r4-completo»: un avviso del registro con il valore (EQUIVALENTE_NON_ADOTTATO) e un blocco del catalogo
	// con il motivo nuovo del blocco «R5 completo» (RICODIFICA_FILE_NAS documento_confermato), sul componente ACM-1400
	if b.r4Completo {
		if k := nodoDelComp[bdC40]; k != "" {
			avvisi = append(avvisi, m{"codice": "EQUIVALENTE_NON_ADOTTATO", "chiave_bozza": k, "valore": "K4-0099",
				"testo": "La voce dello STEP ha un equivalente (K4-0099) diverso dal codice che va nella BOM: adottalo o rifiutalo con il motivo."})
		}
		blocchi = append(blocchi, m{"codice": "RICODIFICA_FILE_NAS", "motivo": "documento_confermato", "componente_id": bdC40,
			"testo":  "ACM-1400 ha 1 documenti già confermati: la ricodifica ne cambierebbe il codice e la cartella (non è in V1).",
			"rimedi": []any{"togli la ricodifica dalla bozza", "correggi il codice dalla scheda del componente, che porta anche i documenti"}})
	}
	// la sorgente, la riga «legato come sorgente» e l'effetto sulla fonte (direttiva 39 §2, §5)
	sorg, gate := b.gateSorgenteR4(prod, snap)
	if gate != nil {
		if monoPezzo {
			gate["mono_pezzo"] = true
		}
		blocchi = append(blocchi, gate)
	}
	o := m{"formato": 3, "prodotto_id": prod, "versione_corrente": bdVersioneCorrente(b.conferme[prod]), "base": m{"stato": "valida", "motivi": []any{}}, "rif_scaduti": []any{},
		"sorgente": sorg, "legami_storici": []any{}, "identita": identita, "archi": archi, "fuori": []any{}, "da_ricollocare": []any{}, "esclusi": esclusi,
		"documenti_preassociati": pre, "svincoli": svincoli, "effetti_documentali": effetti, "condivisione": []any{}, "registro_decisioni": []any{},
		"criterio_previsto": m{"criterio": "nessuno", "motivo": "serve la dichiarazione mono_pezzo o la gerarchia dello STEP (mai dedotta)"},
		"verifica_prevista": m{"stato": "non_calcolata", "regola": "prima-istanza-1", "criterio": "nessuno", "motivi": []any{"avvisi_motore_non_calcolati"}, "conflitti_motore_calcolati": false,
			"origine_gerarchia": "", "gerarchia_step": false},
		"differenze_dalla_base": []any{}, "blocchi": blocchi, "avvisi": avvisi, "avvisi_motore": []any{}, "avvisi_motore_calcolati": false, "conflitti_motore": []any{}, "file_in_analisi": []any{}}
	if bz != nil {
		o["bozza_id"] = bz.ID
		if versione > 0 {
			o["bozza_versione"] = versione
		}
	}
	if s, _ := snap["sorgente"].(m); s != nil {
		aid := bdStr(s["allegato_id"])
		nome := ""
		if f := b.fileDi(aid); f != nil {
			nome = f.Nome
		}
		l := m{"allegato_id": aid, "sha256": s["sha256"], "nome_file": nome, "confermata": sorg["valida"] == true}
		if sorg["valida"] == true {
			l["documento_id"] = sorg["documento_id"]
			l["testo"] = "Legato come sorgente del prodotto (confermato da S8): non è un documento da associare, e la conferma non ne registra l'associazione."
		} else {
			l["motivo"] = sorg["motivo"]
			l["testo"] = "Sorgente della bozza, non ancora confermata validamente da S8: non è un documento da associare."
		}
		o["sorgente_legata"] = l
	}
	eff := m{"promozione": false, "testo": "Nessuna promozione: la bozza non ha una sorgente confermata validamente da S8."}
	if sorg["valida"] == true {
		eff = m{"promozione": true, "step_dopo": sorg["documento_id"], "sha256_dopo": sorg["sha256"], "testo": "Con la conferma, lo STEP della bozza diventa lo STEP strutturale del prodotto."}
		if aut, _ := b.fonte["autorizzata"].(bool); aut && bdStr(b.fonte["sha256"]) == bdStr(sorg["sha256"]) {
			eff = m{"promozione": false, "step_prima": b.fonte["documento_id"], "sha256_prima": b.fonte["sha256"], "testo": "Lo STEP della bozza è già lo STEP strutturale del prodotto."}
		} else if aut {
			eff["step_prima"], eff["sha256_prima"], eff["sostituzione_di"] = b.fonte["documento_id"], b.fonte["sha256"], b.fonte["documento_id"]
			eff["testo"] = "Con la conferma, lo STEP della bozza diventa lo STEP strutturale del prodotto e quello di prima resta, sostituito, con la sua storia."
		} else {
			avvisi = append(avvisi, m{"codice": "SORGENTE_NON_AUTORIZZATA", "testo": "Con la conferma, lo STEP della bozza diventa lo STEP strutturale del prodotto."})
			o["avvisi"] = avvisi
		}
	}
	// G39-03 [U]: la sorgente valida corregge una sorgente sbagliata (dichiarata in S8 o dopo la revoca «sorgente errata» di
	// uno STEP entrato in una conferma): la conferma sarà una revisione interna. Nel banco basta il segno della sorgente S8
	// (il server vuole anche la bozza partita da una conferma: riepilogo3_regole.go)
	if sorg["valida"] == true && b.s8[prod] != nil && b.s8[prod]["correzione"] == true {
		eff["correzione_sorgente"] = true
		eff["testo"] = bdStr(eff["testo"]) + " È una correzione della sorgente: una revisione interna, non una revisione del cliente."
	}
	o["effetto_sulla_fonte"] = eff
	if b.condivisione {
		// interruttore «condivisione»: un altro prodotto con un componente della bozza (D-TAB-03 A [T-39])
		for _, x := range nodi {
			n := x.(m)
			if c := bdStr(n["componente_id"]); c != "" && c != prod {
				o["condivisione"] = []any{m{"prodotto_id": bdP2, "codice": "ACM-2000", "conferma_valida_prima": true,
					"elementi":    []any{m{"tipo": "componente", "componente_id": c, "codice": codiceDi[bdStr(n["chiave_bozza"])], "effetto": "le sue associazioni e i suoi archi cambiano con questa conferma"}},
					"conseguenza": "Le variazioni vengono dalla conferma di ACM-1000 e sono attribuite a chi la conferma; ACM-2000 non è confermato per implicazione. La sua ultima conferma può non valere più per la working cambiata."}}
				avvisi = append(avvisi, m{"codice": "PRODOTTI_TOCCATI", "testo": "La conferma tocca anche ACM-2000.", "prodotti": []any{bdP2}})
				o["avvisi"] = avvisi
				break
			}
		}
	}
	if bz != nil && bz.Registro != "" {
		o["importazione"] = m{"impronta": bz.Registro, "impronta_contenuto": reg["impronta_contenuto"]}
	}
	// PE-23: uguale_alla_base sempre (il server lo dà sempre); vero solo contro una conferma di base formato 3
	u, _ := b.ugualeAllaBase(prod, snap)
	o["uguale_alla_base"] = u
	// gli avvisi e i conflitti del motore, firmati (blocco «R4 completo» della persistenza): solo con ?avvisi_motore=1 e un
	// calcolo riuscito; il riepilogo è confermabile solo così (il leggero non si conferma, PA-C01)
	calcolati := false
	if motore {
		if b.modo("motore") == "ko" {
			o["avvisi_motore_errore"] = m{"codice": "CALCOLO_OCCUPATO", "testo": "Il motore è occupato o non pronto: gli avvisi del motore non sono calcolati. Riprova il riepilogo."}
		} else {
			calcolati = true
			o["avvisi_motore_calcolati"] = true
			if b.r4Completo {
				o["avvisi_motore"] = []any{m{"codice": "PROPOSTA_CAMBIATA", "motivo": "contenuto", "differenze": m{"nodi": 2, "archi": 1},
					"testo": "La proposta del motore sullo STEP della bozza è cambiata dopo l'apertura: 2 nodi e 1 legame diversi. «Aggiorna proposta» dalla Sorgente per vederla."}}
				o["conflitti_motore"] = []any{"nomenclatura"}
			}
		}
	}
	o["confermabile"] = len(blocchi) == 0 && calcolati
	// l'effetto delle categorie confermate nella bozza (U2 §6.4.2, effetti_categoria[], campo anticipato dal banco)
	effCat := []any{}
	for _, x := range decisioni {
		d, _ := x.(m)
		if d == nil || d["tipo"] != "conferma_categoria" {
			continue
		}
		sog, _ := d["soggetto"].(m)
		dopo := "2D richiesto"
		if d["esito"] == "conferma" && d["categoria"] == "minuteria" {
			dopo = "2D non richiesto"
		}
		effCat = append(effCat, m{"chiave_bozza": sog["chiave_bozza"], "prima": "2D richiesto", "dopo": dopo})
	}
	o["effetti_categoria"] = effCat
	// gli ingressi su cui poggia il riepilogo (campo anticipato dal banco) e i documenti ancora in analisi, fuori firma
	parti, tutto := b.partiIngressi(prod)
	o["ingressi"] = m{"prodotto_id": prod, "algoritmo": "impronta-ingressi-2", "impronta_ingressi": tutto, "parti": parti}
	o["file_in_analisi"] = parti["analisi"].(m)["in_analisi"]
	// la firma sul JSON canonico dello snapshot (EM §8.3, §3.10), con lo stato della sorgente (la revoca cambia la firma) e
	// con gli avvisi del motore (il riepilogo leggero ha un'altra firma: R5 con quella risponde 409)
	o["firma_anteprima"] = bdSha(fmt.Sprintf("anteprima-3|%d|%v|%v|%v|%s", versione, sorg["valida"], calcolati, b.r4Completo, bdCanonico(bdFormaCanonica(snap, ""))))
	if b.firmaConProposta {
		o["firma_anteprima"] = bdSha(fmt.Sprintf("anteprima-3|%d|%v|%v|%v|%d|%s", versione, sorg["valida"], calcolati, b.r4Completo, b.versioneProposta, bdCanonico(bdFormaCanonica(snap, ""))))
	}
	return o
}

// sondaR45: POST /__banco/provenienza {allegato_id, provenienza, importazione?, chiave_bozza?}: la provenienza che il server
// avrebbe deciso per una PB della bozza aperta di ACM-1000 (D-PB03 A [U]: motore, corretta, manuale, legacy), scritta nella
// versione salvata come la scrive il server; con chiave_bozza, se la PB non c'è, la PB scritta dal server (una PB iniziale
// del motore, D-PB-01 A, o la conversione di una pre-assegnazione P1, D-AR14 A). R1 la rilegge con lo snapshot e R4 la
// raggruppa (D-PB-04).
// POST /__banco/ar {nome, componente_id}: rimette l'associazione registrata di un file del banco (dati di prova: una prova
// che ha chiuso la AR con un gesto composto lascia il banco com'era per le prove che seguono).
func (b *bancoDU) sondaR45(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost && r.URL.Path == "/__banco/ar" {
		var o struct {
			Nome         string `json:"nome"`
			ComponenteID string `json:"componente_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&o)
		for _, f := range b.file {
			if f.Nome == o.Nome {
				f.Componente = o.ComponenteID
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	if r.Method != http.MethodPost || r.URL.Path != "/__banco/provenienza" {
		return false
	}
	var o struct {
		AllegatoID   string `json:"allegato_id"`
		Provenienza  string `json:"provenienza"`
		Importazione string `json:"importazione"`
		ChiaveBozza  string `json:"chiave_bozza"`
	}
	_ = json.NewDecoder(r.Body).Decode(&o)
	if bz := b.bozze[bdP1]; bz != nil {
		c := false
		for _, x := range bdElenco(bz.Snapshot["documenti_preassociati"]) {
			c = c || bdStr(x.(m)["allegato_id"]) == o.AllegatoID
		}
		if !c && o.ChiaveBozza != "" {
			bz.Snapshot["documenti_preassociati"] = append(bdElenco(bz.Snapshot["documenti_preassociati"]), m{"allegato_id": o.AllegatoID, "chiave_bozza": o.ChiaveBozza})
		}
		if o.Importazione == "registro" {
			o.Importazione = bz.Registro
		}
		for _, x := range bdElenco(bz.Snapshot["documenti_preassociati"]) {
			if d := x.(m); bdStr(d["allegato_id"]) == o.AllegatoID {
				d["provenienza"] = o.Provenienza
				delete(d, "importazione")
				delete(d, "verificata")
				if o.Importazione != "" {
					d["importazione"] = o.Importazione
				}
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}
