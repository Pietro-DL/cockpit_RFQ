package web

// Le rotte dell'ondata C della V1 nel banco finto della Distinta unica (distinta_unica_banco_test.go): S9 (la variante
// ?sorgente=1 di /motore-a/dati), S8 (la conferma dello STEP come sorgente di un prodotto, una procedura a passi) e la
// variante ?ingressi=1 di R1. Le forme sono quelle dell'emendamento U2 (§2.2, §2.3, §6.3.3); le corsie persistenza e
// motore scrivono le rotte vere. Come il resto del banco: nessuna regola di dominio, solo quanto basta perché i flussi
// del tab Sorgente (mockup v5, direzione A) e del wizard si vedano nel browser.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// s9Azione: {disponibile, motivo} di un'azione di una riga di S9 (U2 §2.3; DEC-A PE-06).
func s9Azione(ok bool, motivo string) m {
	if ok {
		return m{"disponibile": true, "motivo": ""}
	}
	return m{"disponibile": false, "motivo": motivo}
}

// fonteS9: la fonte di un prodotto come la dà S9 (e, dall'ondata C, /dati): con data e autore della conferma e
// dell'autorizzazione (U2 §2.4), e l'allegato dello STEP confermato.
func (b *bancoDU) fonteS9(prod string) m {
	if prod != bdP1 {
		o := m{"confermata": false, "autorizzata": false, "motivo": "il prodotto non ha uno STEP", "conferma_ammessa": false}
		if s := b.sorgenteS8Di(prod); s != nil {
			o["confermata"], o["allegato_id"], o["motivo"], o["conferma_ammessa"] = true, s["allegato_id"], "", s["valida"] != true
		}
		b.campiFonteD39(prod, o)
		return o
	}
	o := m{"confermata": b.fonte["confermata"], "autorizzata": b.fonte["autorizzata"]}
	// direttiva 39: la sorgente confermata da S8 (sorgente_s8), «Conferma la fonte» = S8 (conferma_ammessa, gesto)
	b.campiFonteD39(prod, o)
	o["conferma_ammessa"] = b.s8Valida(prod) == ""
	if c, _ := b.fonte["confermata"].(bool); c {
		o["allegato_id"] = b.fonteAllegato()
		o["confermata_il"] = b.fonte["confermata_il"]
		o["confermata_da"] = m{"utente_id": duUUID(77).String(), "nome": "Amministratore di prova"}
	}
	if a, _ := b.fonte["autorizzata"].(bool); a {
		o["autorizzata_il"] = b.fonte["autorizzata_il"]
		o["autorizzata_da"] = m{"utente_id": duUUID(77).String(), "nome": "Amministratore di prova"}
	}
	return o
}

// fonteAllegato: l'allegato dello STEP della fonte del primo prodotto (quello con lo sha della fonte).
func (b *bancoDU) fonteAllegato() string {
	if a := bdStr(b.fonte["allegato_id"]); a != "" {
		return a
	}
	for _, f := range b.file {
		if f.Sha == bdStr(b.fonte["sha256"]) && (f.Est == "stp" || f.Est == "step") {
			return f.ID
		}
	}
	return ""
}

// s9: GET /thread/{id}/motore-a/dati?sorgente=1 (U2 §2.3). Gli STEP attivi e letti con le loro candidate, lo
// smistamento di ogni prodotto con le azioni calcolate dal «server», i file da associare con il motivo del motore, gli
// Altri file. Il banco ha un solo prodotto con candidate (ACM-1000): ACM-2000 resta senza sorgente e senza righe.
func (b *bancoDU) s9(w http.ResponseWriter) {
	switch b.modo("S9") {
	case "occupato":
		bdKo(w, 503, "CALCOLO_OCCUPATO", "Il motore è occupato: riprova.", nil)
		return
	case "errore":
		bdJSON(w, http.StatusInternalServerError, m{"esito": "errore", "codice": "ERRORE", "testo": "errore del server"})
		return
	}
	b.conti["S9_letture"]++
	inAnalisi := []any{}
	for _, f := range b.file {
		if f.Stato == "attivo" && f.Analisi == "in_analisi" {
			inAnalisi = append(inAnalisi, m{"allegato_id": f.ID, "nome_file": f.Nome, "stato": "in_analisi"})
		}
	}
	// gli STEP attivi e letti con le candidate per ACM-1000
	step := []any{}
	for _, a := range []string{bdA1, bdA10} {
		f := b.fileDi(a)
		if f == nil || f.Stato != "attivo" || f.Analisi != "letto" {
			continue
		}
		reg := b.registro(a, f.Sha, b.versioneProposta)
		letta, comp := "ACM-1000", "compatibile"
		if a == bdA10 {
			letta = "ACM-1000-R2"
		}
		if b.s6Incerta && a == bdA1 {
			comp = "incerta"
		}
		c := m{"prodotto_id": bdP1, "gruppo": reg["gruppo"].(m)["chiave"], "compatibilita": comp, "radice_letterale": letta,
			"struttura": m{"versione": 3, "completa": true, "motivo": ""}, "segnalati": 3, "impronta_contenuto": reg["impronta_contenuto"]}
		step = append(step, m{"allegato_id": a, "nome_file": f.Nome, "analisi": m{"stato": f.Analisi}, "candidate": []any{c}})
	}
	// i file proposti dal motore per il primo prodotto (le associazioni proposte del registro di oggi dello STEP della
	// fonte, o del primo STEP): voce, codice, chiave del nodo
	sorg := b.fonteAllegato()
	if sorg == "" {
		sorg = bdA1
	}
	proposti := []m{}
	if f := b.fileDi(sorg); f != nil {
		reg := b.registro(sorg, f.Sha, b.versioneProposta)
		for _, x := range reg["proposta"].(m)["nodi"].([]any) {
			v := x.(m)
			for _, y := range v["associazioni_proposte"].([]any) {
				a := y.(m)
				rif := bdStr(v["origine"].(m)["rif"])
				proposti = append(proposti, m{"allegato_id": a["allegato_id"], "voce": v["voce"], "rif": rif, "codice": v["codice_proposto"],
					"rev": v["rev_proposta"], "chiave_bozza": "k-" + strings.TrimPrefix(rif[strings.LastIndex(rif, ":")+1:], "#")})
			}
		}
	}
	occupati := map[string]bool{}
	prodotti := []any{}
	for _, prod := range []string{bdP1, bdP2} {
		cod := map[string]string{bdP1: "ACM-1000", bdP2: "ACM-2000"}[prod]
		p := m{"prodotto_id": prod, "codice": cod, "fonte": b.fonteS9(prod), "calcolo": m{"stato": "pronto", "file_in_analisi": inAnalisi},
			"righe": []any{}, "in_attesa_sorgente": []any{}, "conti": m{"proposti": 0, "salvati": 0, "registrati": 0, "in_conflitto": 0, "nodi_senza_file": 0}}
		if prod == bdP1 {
			p["identificativo_cliente"] = "ACM-1000/C"
		}
		bz := b.bozze[prod]
		aperta := bz != nil && bz.Stato == "aperta"
		// D-AR11 B [U] (direttiva 39 §4): senza una sorgente confermata da S8 e valida nessuna riga del motore, anche con una
		// bozza senza STEP aperta: i file restano nella chip «attende la sorgente» (G-6) con il motivo
		valida := b.s8Valida(prod) != ""
		p["preassociazioni_proposte"] = []any{}
		p["conti"].(m)["legati_come_sorgente"], p["conti"].(m)["legami_storici"] = 0, 0
		if !valida && prod == bdP1 {
			for _, x := range proposti {
				ff := b.fileDi(bdStr(x["allegato_id"]))
				if ff == nil || ff.Stato != "attivo" {
					continue
				}
				p["in_attesa_sorgente"] = append(p["in_attesa_sorgente"].([]any), m{"allegato_id": x["allegato_id"], "voce": x["voce"], "codice_mostrato": x["codice"], "motivo": "sorgente_non_confermata"})
				occupati[bdStr(x["allegato_id"])] = true
			}
		}
		if !aperta {
			prodotti = append(prodotti, p)
			continue
		}
		s, _ := bz.Snapshot["sorgente"].(m)
		pbz := m{"bozza_id": bz.ID, "versione": bz.Versione}
		if s != nil {
			pbz["sorgente"] = s["allegato_id"]
		}
		p["bozza"] = pbz
		nodi := map[string]m{}
		perComp := map[string]string{}
		for _, x := range bdElenco(bz.Snapshot["nodi"]) {
			n := x.(m)
			nodi[bdStr(n["chiave_bozza"])] = n
			if c := bdStr(n["componente_id"]); c != "" {
				perComp[c] = bdStr(n["chiave_bozza"])
			}
		}
		// allegato_id → chiave_bozza delle preassociazioni salvate (direttiva 35, C-2: la PB cita l'occorrenza)
		pre := map[string]string{}
		for _, x := range bdElenco(bz.Snapshot["documenti_preassociati"]) {
			d := x.(m)
			pre[bdStr(d["allegato_id"])] = bdStr(d["chiave_bozza"])
		}
		svincoli := map[string]bool{}
		for _, x := range bdElenco(bz.Snapshot["decisioni"]) {
			d := x.(m)
			if d["tipo"] == "scollega_documento" {
				svincoli[bdStr(d["allegato_id"])] = true
			}
		}
		righe := []any{}
		conti := p["conti"].(m)
		riga := func(f *fileBanco, k, comp, codice, rev, stato, origine string) {
			dest := m{}
			if k != "" {
				dest["chiave_bozza"] = k
			}
			if comp != "" {
				dest["componente_id"] = comp
			}
			cid := ""
			if codice != "" {
				cid = codice
				if rev != "" {
					cid += "/" + rev
				}
			}
			az := m{
				"salva_nella_bozza":      s9Azione(stato == "proposto", "solo una riga proposta si salva nella bozza"),
				"togli_dalla_bozza":      s9Azione(stato == "salvato_nella_bozza" || (stato == "in_conflitto" && origine == "bozza"), "solo un collegamento salvato nella bozza si toglie"),
				"sposta":                 s9Azione(stato != "gia_registrato" && stato != "da_scollegare", "un collegamento registrato si scollega alla conferma, non si sposta"),
				"scollega_alla_conferma": s9Azione(stato == "gia_registrato" && f.Documento == nil, map[bool]string{true: "è un documento confermato: si sostituisce o si revisiona (non in V1)", false: "solo un collegamento registrato si scollega alla conferma"}[f.Documento != nil]),
			}
			o := m{"allegato_id": f.ID, "proposta_id": f.Proposta, "destinazione": dest, "codice_mostrato": codice, "rev_mostrata": rev, "stato": stato, "origine": origine,
				"condiviso_con": []any{}, "azioni": az}
			if cid != "" {
				o["identificativo_cliente"] = cid
			}
			if f.Proposta == "" {
				// come S9 vero: proposta_id c'è solo se il file ha una proposta di documento aperta (omitempty)
				delete(o, "proposta_id")
			}
			righe = append(righe, o)
			occupati[f.ID] = true
			switch stato {
			case "proposto":
				conti["proposti"] = bdNum(conti["proposti"]) + 1
			case "salvato_nella_bozza":
				conti["salvati"] = bdNum(conti["salvati"]) + 1
			case "gia_registrato", "da_scollegare":
				conti["registrati"] = bdNum(conti["registrati"]) + 1
			case "in_conflitto":
				conti["in_conflitto"] = bdNum(conti["in_conflitto"]) + 1
			}
		}
		visti := map[string]bool{}
		for _, x := range proposti {
			f := b.fileDi(bdStr(x["allegato_id"]))
			if f == nil || f.Stato != "attivo" || visti[f.ID] {
				continue
			}
			k := bdStr(x["chiave_bozza"])
			if _, ok := nodi[k]; !ok {
				k = ""
			}
			visti[f.ID] = true
			if kk, ok := pre[f.ID]; ok {
				stato := "salvato_nella_bozza"
				// la rev del file (B) non è quella del nodo (A): il riepilogo la dice in conflitto
				if f.ID == bdA3 && kk == "k-11" {
					stato = "in_conflitto"
				}
				riga(f, kk, "", bdStr(x["codice"]), f.Rev, stato, "bozza")
				continue
			}
			// come S9 vero (motore_a/sorgente.go, passi 4 e 6): una AR attiva di un componente del prodotto viene prima
			// delle proposte del motore; la sua riga gia_registrato la scrive il giro delle AR qui sotto
			if f.Componente != "" && f.Componente != bdP1 && (perComp[f.Componente] != "" || b.compR5[prod][f.Componente]) {
				delete(visti, f.ID)
				continue
			}
			if !valida {
				// D-AR11 B: senza sorgente S8 il motore non propone; il file resta in attesa (chip)
				continue
			}
			riga(f, k, "", bdStr(x["codice"]), f.Rev, "proposto", "motore")
			// D-PB02 A [U]: con la bozza già aperta la proposta del motore è un suggerimento, mai applicato da solo
			sug := m{"allegato_id": f.ID, "destinazione": m{}, "provenienza": "motore", "motivo": "dopo_apertura"}
			if k != "" {
				sug["chiave_bozza"] = k
				sug["destinazione"] = m{"chiave_bozza": k}
			}
			p["preassociazioni_proposte"] = append(p["preassociazioni_proposte"].([]any), sug)
		}
		// lo STEP sorgente del finito in sola lettura (direttiva 39 §5; S9 legato_come_sorgente, motivo conferma_s8)
		if sv := b.s8Valida(prod); sv != "" {
			if f := b.fileDi(sv); f != nil && f.Stato == "attivo" {
				no := "lo STEP sorgente del prodotto si cambia con «Conferma questo STEP come sorgente» (S8) o con la revoca, non da qui"
				righe = append(righe, m{"allegato_id": f.ID, "destinazione": m{"componente_id": prod}, "codice_mostrato": map[string]string{bdP1: "ACM-1000", bdP2: "ACM-2000"}[prod],
					"stato": "legato_come_sorgente", "motivo": "conferma_s8", "origine": "sorgente", "priorita_anteprima": 1, "condiviso_con": []any{}, "condivisione": []any{},
					"azioni": m{"salva_nella_bozza": s9Azione(false, no), "togli_dalla_bozza": s9Azione(false, no), "sposta": s9Azione(false, no), "scollega_alla_conferma": s9Azione(false, no)}})
				conti["legati_come_sorgente"] = 1
			}
		}
		// le preassociazioni salvate dall'utente per file che il motore non propone
		ids := []string{}
		for aid := range pre {
			ids = append(ids, aid)
		}
		sort.Strings(ids)
		for _, aid := range ids {
			f := b.fileDi(aid)
			if f == nil || visti[f.ID] || f.Stato != "attivo" {
				continue
			}
			visti[f.ID] = true
			k := pre[aid]
			cod := ""
			if n := nodi[k]; n != nil {
				if id, _ := n["identita"].(m); id != nil {
					cod = bdStr(id["codice"])
				}
			}
			riga(f, k, "", cod, f.Rev, "salvato_nella_bozza", "bozza")
		}
		// le associazioni registrate dei componenti della bozza (AR): già registrate, o da scollegare alla conferma; e quelle
		// che R5 ha registrato su componenti del prodotto che la bozza non ha (la destinazione è il componente)
		for _, f := range b.file {
			if f.Componente == "" || f.Componente == bdP1 || visti[f.ID] || f.Stato != "attivo" {
				continue
			}
			k := perComp[f.Componente]
			if k == "" && !b.compR5[prod][f.Componente] {
				continue
			}
			visti[f.ID] = true
			stato := "gia_registrato"
			if svincoli[f.ID] {
				stato = "da_scollegare"
			}
			riga(f, k, f.Componente, f.Codice, f.Rev, stato, "working")
		}
		p["righe"] = righe
		prodotti = append(prodotti, p)
	}
	// i file da associare: attivi, letti, non STEP, senza una riga né un'attesa, fuori dagli archivi
	motivi := map[string]string{bdA4: "nessun_candidato", bdA5: "associazione_ambigua", bdA6: "nessun_candidato", bdA8: "collocazione_non_determinabile",
		bdA9: "nessun_candidato", bdA14: "nessun_candidato", bdA15: "fuori_richiesta_non_confermato"}
	daAssociare := []any{}
	altri := []any{}
	for _, f := range b.file {
		if f.Stato == "altri_file" {
			stato := "copiata"
			if f.Conservazione != nil {
				stato = bdStr(f.Conservazione["stato"])
			}
			altri = append(altri, m{"allegato_id": f.ID, "nome_sul_nas": f.Nome, "stato_copia": stato})
			continue
		}
		if f.Stato != "attivo" || f.Analisi != "letto" || f.Est == "stp" || f.Est == "step" || f.Est == "zip" || occupati[f.ID] {
			continue
		}
		if f.Componente != "" {
			continue
		}
		mot := motivi[f.ID]
		if mot == "" {
			mot = "nessun_candidato"
		}
		daAssociare = append(daAssociare, m{"allegato_id": f.ID, "motivo": mot})
	}
	var firma strings.Builder
	for _, f := range b.file {
		fmt.Fprintf(&firma, "%s|%s|%d|%s|%s;", f.ID, f.Stato, f.Versione, f.Analisi, f.Componente)
	}
	for _, prod := range []string{bdP1, bdP2} {
		if bz := b.bozze[prod]; bz != nil {
			fmt.Fprintf(&firma, "%s|%s|%d;", bz.ID, bz.Stato, bz.Versione)
		}
	}
	fmt.Fprintf(&firma, "%v|%v|%d", b.fonte["confermata"], b.fonte["autorizzata"], b.versioneProposta)
	bdJSON(w, http.StatusOK, m{"firma": bdSha(firma.String()), "step": step, "prodotti": prodotti, "da_associare": daAssociare, "altri_file": altri})
}

// bdElenco: un elenco JSON del banco ([]any), vuoto se assente.
func bdElenco(v any) []any {
	l, _ := v.([]any)
	return l
}

// s8: la conferma della sorgente sta in distinta_unica_banco_d39_test.go (s8D39), con le regole della direttiva 39.

// partiIngressi: le parti della variante ?ingressi=1 di R1 (U2 §6.3.3), con le impronte per parti degli ingressi del
// riepilogo, solo «database», anche senza bozza. Il banco le calcola dal suo stato: le conferme e la fonte (bom), la
// testata della bozza (bozza), la proposta di oggi e lo stato dei file (proposta), i file in analisi (analisi). Un
// salvataggio di un collega (sonda bozza-altrove) porta un altro autore nella parte «bozza». R4 dà le stesse parti.
func (b *bancoDU) partiIngressi(prod string) (m, string) {
	bom := fmt.Sprintf("bom|%d|%v|%v|%s", len(b.conferme[prod]), b.fonte["confermata"], b.fonte["autorizzata"], bdStr(b.fonte["sha256"]))
	if b.baseSuperata {
		bom += "|superata"
	}
	pb := m{}
	bozza := "bozza|nessuna"
	if bz := b.bozze[prod]; bz != nil && bz.Stato == "aperta" {
		bozza = fmt.Sprintf("bozza|%s|%d", bz.ID, bz.Versione)
		// aggiornata_da è l'utente_id, senza il nome, come nel server (distintabozza.ParteBozza, EM §9.1)
		pb = m{"bozza_id": bz.ID, "versione": bz.Versione, "aggiornata_da": duUUID(77).String(), "aggiornata_il": time.Now().UTC()}
		if b.altrove[prod] == bz.Versione {
			pb["aggiornata_da"] = duUUID(78).String()
		}
	}
	pb["impronta"] = bdSha(bozza)
	var prop strings.Builder
	fmt.Fprintf(&prop, "proposta|%d;", b.versioneProposta)
	inAnalisi := []any{}
	var an strings.Builder
	for _, f := range b.file {
		if f.Analisi == "in_analisi" && f.Stato == "attivo" {
			inAnalisi = append(inAnalisi, m{"allegato_id": f.ID, "nome_file": f.Nome, "stato": "in_analisi"})
			fmt.Fprintf(&an, "%s;", f.ID)
			continue
		}
		fmt.Fprintf(&prop, "%s|%s|%d|%s;", f.ID, f.Stato, f.Versione, f.Componente)
	}
	bm := m{"impronta": bdSha(bom), "contatore_struttura": 12 + len(b.conferme[prod]), "contatore_dati": 12, "ultima_conferma_id": nil}
	if c := b.conferme[prod]; len(c) > 0 {
		bm["ultima_conferma_id"] = c[len(c)-1]["conferma_id"]
	}
	parti := m{"bom": bm, "bozza": pb, "proposta": m{"impronta": bdSha(prop.String())}, "analisi": m{"impronta": bdSha("analisi|" + an.String()), "in_analisi": inAnalisi}}
	tutto := bdSha(bdCanonico(m{"bom": bm["impronta"], "bozza": pb["impronta"], "proposta": parti["proposta"].(m)["impronta"], "analisi": parti["analisi"].(m)["impronta"]}))
	return parti, tutto
}

// ingressi: GET …/motore-a/bozza?prodotto=…&ingressi=1 (U2 §6.3.3).
func (b *bancoDU) ingressi(w http.ResponseWriter, prod string) {
	b.conti["R1_ingressi"]++
	parti, tutto := b.partiIngressi(prod)
	bdJSON(w, http.StatusOK, m{"prodotto_id": prod, "impronta_ingressi": tutto, "parti": parti})
}

// sondaV1C: le sonde dell'ondata C. POST /__banco/documento-nuovo {nome}: un PDF nuovo della RFQ, in analisi finché
// POST /__banco/analisi-conclusa {nome, cambia} non ne conclude l'analisi (con cambia vero la proposta di oggi cambia:
// «analisi conclusa con proposta cambiata», N2; altrimenti N5). Torna false se la richiesta non è sua.
func (b *bancoDU) sondaV1C(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/__banco/documento-nuovo":
		var o struct{ Nome string }
		_ = json.NewDecoder(r.Body).Decode(&o)
		id := fmt.Sprintf("00000000-0000-4000-8000-0000000a%04x", 0x100+len(b.file))
		b.file = append(b.file, &fileBanco{ID: id, Proposta: bdProposta(id), Nome: o.Nome, Est: "pdf", Sha: bdSha(o.Nome), Origine: "portale", Canale: "posta", Stato: "attivo",
			Analisi: "in_analisi", fermo: true})
		bdJSON(w, http.StatusOK, m{"allegato_id": id})
		return true
	case "/__banco/analisi-conclusa":
		var o struct {
			Nome   string
			Cambia bool
		}
		_ = json.NewDecoder(r.Body).Decode(&o)
		for _, f := range b.file {
			if f.Nome == o.Nome {
				f.Analisi, f.fermo = "letto", false
			}
		}
		if o.Cambia {
			b.versioneProposta++
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
