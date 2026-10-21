package web

// La direttiva 39 nel banco finto della Distinta unica (distinta_unica_banco_test.go): S8 conferma la fonte della BOZZA
// (nessuna associazione, nessun «Autorizza», la working che cambia solo con R5), la partenza solo_prodotto con la presa
// d'atto dello stato dell'analisi, la riconciliazione della bozza senza STEP (cambio-fonte-2: gli abbinamenti certi, gli
// abbinamenti_proposti[] incerti, nessun doppione), la revoca della sorgente, la partenza vuota di R2, struttura_step di
// S6 e di /dati, e il gate di R4/R5 STEP_SORGENTE_NON_CONFERMATA (acceso sempre, come il server dal blocco R4/R5 della
// persistenza; l'interruttore «gate-sorgente» resta solo per le prove L1 che lo spengono). Le forme sono
// quelle delle corsie persistenza (core/rfq/sorgente, distintabozza: riconciliazione3.go, soloprodotto3.go, crea3.go) e
// motore (propostabom.StrutturaSTEP, /dati.fonte, S9). Come il resto del banco: nessuna regola di dominio, solo quanto
// basta perché i flussi della pagina si vedano nel browser.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// i testi del motore per i casi della struttura (propostabom/struttura_step.go), copiati com'erano
const (
	bdTestoSolaRadice    = "Il motore non ha estratto figli da questo STEP: la bozza parte dal solo prodotto, senza dedurre un mono-pezzo"
	bdTestoNessuna       = "Il motore non ha trovato una struttura importabile in questo STEP per il prodotto: puoi confermarlo come sorgente e partire dal solo prodotto"
	bdTestoAnalisiCorso  = "Lo STEP è ancora in analisi: la struttura non è determinabile adesso"
	bdTestoAnalisiErrore = "L'analisi dello STEP non è riuscita: la struttura non è determinabile"
)

// statoAnalisiStep: lo stato dell'analisi di uno STEP come lo dice il motore (propostabom.StatoAnalisiDelFile): letto =
// pronta, errore = errore, altrimenti in analisi
func bdStatoAnalisiStep(f *fileBanco) string {
	switch f.Analisi {
	case "letto":
		return "pronta"
	case "errore":
		return "errore"
	}
	return "in_analisi"
}

// strutturaStep: struttura_step di S6 e di /dati.fonte (propostabom.ClassificaStruttura) per lo STEP e il prodotto. Nel
// banco solo il primo prodotto ha una struttura (i due STEP ACM-1000); il secondo non ne ha nessuna importabile.
func (b *bancoDU) strutturaStep(f *fileBanco, prod string) m {
	st := bdStatoAnalisiStep(f)
	switch st {
	case "in_analisi":
		return m{"stato_analisi": st, "struttura": "non_determinabile", "figli_estratti": 0, "partenza": "solo_prodotto", "presa_d_atto_richiesta": true, "testo": bdTestoAnalisiCorso}
	case "errore":
		return m{"stato_analisi": st, "struttura": "non_determinabile", "figli_estratti": 0, "partenza": "solo_prodotto", "presa_d_atto_richiesta": true, "testo": bdTestoAnalisiErrore}
	}
	if prod != bdP1 {
		return m{"stato_analisi": st, "struttura": "nessuna_struttura", "figli_estratti": 0, "partenza": "solo_prodotto", "presa_d_atto_richiesta": false, "testo": bdTestoNessuna}
	}
	reg := b.registro(f.ID, f.Sha, b.versioneProposta)
	return m{"stato_analisi": st, "struttura": "con_struttura", "gruppo": reg["gruppo"].(m)["chiave"], "impronta_contenuto": reg["impronta_contenuto"],
		"figli_estratti": len(reg["proposta"].(m)["nodi"].([]any)) - 1, "partenza": "step", "presa_d_atto_richiesta": false}
}

// sorgenteS8Di: l'ultima conferma di S8 del prodotto (fonte.sorgente_s8), o nil
func (b *bancoDU) sorgenteS8Di(prod string) m {
	s := b.s8[prod]
	if s == nil {
		return nil
	}
	o := m{}
	for k, v := range s {
		o[k] = v
	}
	return o
}

// s8Valida: lo STEP confermato da S8 e valido del prodotto (confermabom.SorgenteValida), o ""
func (b *bancoDU) s8Valida(prod string) string {
	if s := b.s8[prod]; s != nil && s["valida"] == true {
		return bdStr(s["allegato_id"])
	}
	return ""
}

// campiFonteD39: i campi della direttiva 39 nella fonte di /dati e di S9 (motore_a/dati_v1.go)
func (b *bancoDU) campiFonteD39(prod string, o m) {
	o["gesto"] = "conferma_sorgente"
	o["sorgente_confermata"] = b.s8Valida(prod) != ""
	// giro G39 (G39-02 [U]): le proposte di sorgente del Motore A (motore_a/proposta_sorgente.go), sempre un elenco
	o["proposte_sorgente"] = b.proposteSorgenteBanco(prod)
	if s := b.sorgenteS8Di(prod); s != nil {
		o["sorgente_s8"] = s
	}
	// legame storico (D-AR04 (iii)): solo con la sonda, per un prodotto senza conferme di S8
	if l, _ := b.fonte["legame_storico_"+prod].(m); l != nil && b.s8[prod] == nil {
		o["legame_storico"] = l
	}
}

// s8D39: POST /thread/{id}/sorgente/step/{a}/conferma-sorgente (U2 §2.2 con la direttiva 39; la forma del server di
// core/rfq/sorgente). Passo 0 senza scrivere; passo 1 «non_serve» (lo STEP sorgente non è una AR); passo 2 la conferma
// del documento (mai l'autorizzazione, mai la working); passo 3 la bozza: nuova (partenza step o solo_prodotto), il
// cambio della fonte di una bozza aperta, la riconciliazione di una bozza senza STEP (cambio-fonte-2), la sola sorgente
// riservata su una bozza senza STEP (solo_prodotto), «già fatto» con la stessa sorgente. Gli esiti da provocare (sonda
// «modo», rotta S8): «parziale» (il passo 3 rifiuta con ANTEPRIMA_SUPERATA), «occupato» (il passo 3 risponde 503:
// parziale ritentabile, l'intento non si scrive), «errore» (un 500 prima di ogni passo: esito da verificare).
func (b *bancoDU) s8D39(w http.ResponseWriter, r *http.Request) {
	a := r.PathValue("a")
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return
	}
	for k := range corpo {
		kl := strings.ToLower(k)
		if strings.Contains(kl, "percorso") || strings.Contains(kl, "path") || strings.Contains(kl, "autorizz") || k == "componente_atteso" {
			bdKo(w, 422, "CORPO_NON_VALIDO", "campo non ammesso: "+k, m{"campo": k})
			return
		}
	}
	b.corpiFile = append(b.corpiFile, m{"gesto": "conferma_sorgente", "allegato": a, "corpo": corpo})
	chiave := bdStr(corpo["chiave_idempotenza"])
	senza := m{}
	for k, v := range corpo {
		if k != "chiave_idempotenza" {
			senza[k] = v
		}
	}
	impronta := bdSha(bdCanonico(m{"gesto": "conferma_sorgente", "thread": bdThread, "allegato": a, "corpo": senza}))
	if it, ok := b.intenti[chiave]; ok {
		if it.Impronta != impronta {
			bdKo(w, 409, "CHIAVE_RIUSATA", "la chiave ha già un esito diverso", nil)
			return
		}
		rr := m{}
		for k, v := range it.Risposta {
			rr[k] = v
		}
		rr["riproposto"] = true
		bdJSON(w, http.StatusOK, rr)
		return
	}
	modo := b.modo("S8")
	if modo == "errore" {
		bdJSON(w, http.StatusInternalServerError, m{"esito": "da_verificare", "codice": "ERRORE", "chiave_idempotenza": chiave, "testo": "errore del server"})
		return
	}
	// il corpo (sorgente.LeggiRichiestaS8)
	partenza := bdStr(corpo["partenza"])
	if partenza == "" {
		partenza = "step"
	}
	prod := bdStr(corpo["prodotto_id"])
	switch partenza {
	case "step":
		if bdStr(corpo["gruppo"]) == "" || bdStr(corpo["impronta_contenuto"]) == "" || corpo["stato_analisi"] != nil || corpo["presa_d_atto"] != nil {
			bdKo(w, 422, "CORPO_NON_VALIDO", "la partenza step vuole gruppo e impronta, senza stato_analisi né presa_d_atto", m{"campo": "gruppo"})
			return
		}
	case "solo_prodotto":
		if corpo["gruppo"] != nil || corpo["impronta_contenuto"] != nil || corpo["scelte"] != nil {
			bdKo(w, 422, "CORPO_NON_VALIDO", "la partenza solo_prodotto non ha gruppo, impronta né scelte", m{"campo": "gruppo"})
			return
		}
		if corpo["presa_d_atto"] != true {
			bdKo(w, 422, "CORPO_NON_VALIDO", "serve la presa d'atto dello stato dell'analisi", m{"campo": "presa_d_atto", "motivo": "presa_d_atto_mancante"})
			return
		}
	default:
		bdKo(w, 422, "CORPO_NON_VALIDO", "la partenza è step oppure solo_prodotto", m{"campo": "partenza"})
		return
	}
	if s, ok := corpo["sostituisci"].(m); ok && s["conferma"] != true {
		bdKo(w, 422, "CORPO_NON_VALIDO", "sostituisci, se c'è, è {conferma: true}", m{"campo": "sostituisci"})
		return
	}
	// passo 0
	f := b.fileDi(a)
	if f == nil {
		bdKo(w, 422, "FUORI_RFQ", "lo STEP non è di questa RFQ", nil)
		return
	}
	if f.Est != "stp" && f.Est != "step" {
		bdKo(w, 422, "NON_STEP", "il file non è uno STEP", nil)
		return
	}
	if f.Stato != "attivo" {
		bdKo(w, 422, "FILE_NON_ATTIVO", "lo STEP non è fra i file attivi", nil)
		return
	}
	if prod != bdP1 && prod != bdP2 {
		bdKo(w, 422, "FUORI_RFQ", "il prodotto non è di questa RFQ", m{"campo": "prodotto_id"})
		return
	}
	cod := map[string]string{bdP1: "ACM-1000", bdP2: "ACM-2000"}[prod]
	if ca := bdStr(corpo["codice_atteso"]); ca != "" && !strings.EqualFold(ca, cod) {
		bdKo(w, 409, "ANTEPRIMA_SUPERATA", "il codice del prodotto è cambiato dopo l'anteprima: rileggi la pagina", m{"anteprima": "prodotto", "campo": "codice_atteso"})
		return
	}
	if sh := bdStr(corpo["sha256"]); sh != "" && sh != f.Sha {
		bdKo(w, 422, "CORPO_NON_VALIDO", "lo sha256 dichiarato non è quello dello STEP", m{"campo": "sha256", "motivo": "sha_diverso"})
		return
	}
	motivo := bdStr(corpo["motivo"])
	// uno STEP è la sorgente di un solo prodotto; G39-03 [U] (core/rfq/sorgente/sposta.go): lo STEP legato a un altro
	// prodotto si sposta (la sua sorgente S8 valida, non usata da una R5: sposta_da), si riassocia con il motivo (un
	// documento dell'altro senza S8 valida: qui la sua sorgente S8 già revocata) o si rifiuta con il rimedio della revisione
	// (la BOM dell'altro confermata con lui: interruttore «bom-confermata-altro»)
	spostaDa := bdStr(corpo["sposta_da"])
	if spostaDa != "" && (spostaDa == prod || (spostaDa != bdP1 && spostaDa != bdP2)) {
		bdKo(w, 422, "CORPO_NON_VALIDO", "sposta_da è l'altro prodotto da cui la sorgente passa a questo", m{"campo": "sposta_da"})
		return
	}
	if v, ok := corpo["correzione_sorgente"]; ok && v != true {
		bdKo(w, 422, "CORPO_NON_VALIDO", "correzione_sorgente, se c'è, è true", m{"campo": "correzione_sorgente"})
		return
	}
	var spostamento m
	for _, p := range []string{bdP1, bdP2} {
		s := b.s8[p]
		if p == prod || s == nil || bdStr(s["allegato_id"]) != a || s["spostato_a"] != nil {
			continue
		}
		codA := map[string]string{bdP1: "ACM-1000", bdP2: "ACM-2000"}[p]
		if spostaDa != "" && spostaDa != p {
			bdKo(w, 422, "CORPO_NON_VALIDO", "lo STEP è un documento di "+codA+", non del prodotto indicato in sposta_da: rileggi la pagina", m{"campo": "sposta_da"})
			return
		}
		if b.usataDaConferma(p, a) {
			bdKo(w, 422, "STEP_SORGENTE_DI_ALTRO_PRODOTTO", "Questo STEP è nella BOM confermata di "+codA+": non si sposta su "+cod+". Apri la revisione di "+codA+
				" e conferma la sua sorgente giusta, se lo STEP è sbagliato per "+codA+" (una correzione della sorgente). Niente è cambiato.",
				m{"campo": "allegato", "motivo": "bom_confermata", "altro_prodotto": p})
			return
		}
		if s["valida"] == true {
			if spostaDa == "" {
				bdKo(w, 422, "STEP_SORGENTE_DI_ALTRO_PRODOTTO", "Questo STEP è la sorgente confermata di "+codA+" (la BOM non è ancora confermata). Se l'associazione è sbagliata, «Sposta la sorgente su "+cod+
					"»: la sorgente di "+codA+" si revoca con il motivo «sorgente errata» e la sua bozza si annulla. In V1 uno STEP è la sorgente di un solo prodotto.",
					m{"campo": "allegato", "motivo": "sorgente_spostabile", "altro_prodotto": p})
				return
			}
			spostamento = m{"caso": "sorgente_s8", "da_prodotto_id": p, "da_codice": codA}
		} else {
			if motivo == "" {
				bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Questo STEP è un documento di "+codA+", non confermato come sua sorgente: per riassociarlo a "+cod+" scrivi il motivo. Lascerà "+codA+".",
					m{"campo": "motivo", "motivo": "riassociazione", "altro_prodotto": p})
				return
			}
			spostamento = m{"caso": "documento", "da_prodotto_id": p, "da_codice": codA}
		}
	}
	var reg m
	if partenza == "step" {
		if prod != bdP1 {
			bdKo(w, 409, "ANTEPRIMA_SUPERATA", "Il gruppo non è più fra quelli dello STEP per il prodotto: rivedi l'anteprima.", m{"anteprima": "proposta_step"})
			return
		}
		reg = b.registro(a, f.Sha, b.versioneProposta)
		if bdStr(corpo["gruppo"]) != reg["gruppo"].(m)["chiave"] || bdStr(corpo["impronta_contenuto"]) != reg["impronta_contenuto"] {
			bdKo(w, 409, "ANTEPRIMA_SUPERATA", "La proposta è cambiata mentre guardavi: rivedila.", m{"anteprima": "proposta_step", "impronta_contenuto": reg["impronta_contenuto"]})
			return
		}
		if a == bdA10 && motivo == "" {
			bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Il codice letto dalla radice dello STEP non è quello del prodotto: serve un motivo.", m{"campo": "motivo"})
			return
		}
	} else {
		ss := b.strutturaStep(f, prod)
		if bdStr(corpo["stato_analisi"]) != bdStr(ss["stato_analisi"]) || ss["partenza"] != "solo_prodotto" {
			bdKo(w, 409, "ANTEPRIMA_SUPERATA", "Lo stato dell'analisi dello STEP è cambiato, o una struttura importabile c'è: rivedi l'anteprima.", m{"anteprima": "stato_analisi"})
			return
		}
		if motivo == "" {
			bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Senza struttura importabile la partenza dal solo prodotto vuole il motivo.", m{"campo": "motivo"})
			return
		}
	}
	// la bozza (Controlla)
	bz := b.bozze[prod]
	aperta := bz != nil && bz.Stato == "aperta"
	var sb m
	if aperta {
		sb, _ = bz.Snapshot["sorgente"].(m)
	}
	gruppoFonte := "solo_prodotto"
	if reg != nil {
		gruppoFonte = bdStr(reg["gruppo"].(m)["chiave"])
	}
	stessa := aperta && sb != nil && bdStr(sb["allegato_id"]) == a && bdStr(sb["gruppo"]) == gruppoFonte
	manuale := aperta && sb == nil
	if aperta && !stessa {
		cb, _ := corpo["bozza"].(m)
		switch {
		case bz.Formato2:
			bdKo(w, 409, "RICONCILIAZIONE_NON_DISPONIBILE", "la bozza aperta del prodotto è nel formato 2: prima si converte", m{"motivo": "bozza_da_convertire", "campo": "bozza"})
			return
		case partenza == "solo_prodotto" && !manuale:
			bdKo(w, 409, "RICONCILIAZIONE_NON_DISPONIBILE", "la bozza aperta ha un'altra sorgente STEP", m{"motivo": "solo_prodotto_su_bozza", "campo": "partenza"})
			return
		case cb == nil:
			bdKo(w, 409, "BOZZA_VERSIONE", "il prodotto ha una bozza aperta: il corpo la dichiara", m{"motivo": "altra_bozza_aperta", "campo": "bozza",
				"conflitto": m{"bozza_id": bz.ID, "stato": "aperta", "versione": bz.Versione}})
			return
		case bdStr(cb["bozza_id"]) != bz.ID || bdNum(cb["versione_attesa"]) != bz.Versione:
			bdKo(w, 409, "BOZZA_VERSIONE", fmt.Sprintf("la bozza aperta del prodotto è alla versione %d: rileggila", bz.Versione), m{"motivo": "versione_diversa", "campo": "bozza",
				"conflitto": m{"bozza_id": bz.ID, "stato": "aperta", "versione": bz.Versione}})
			return
		}
		if motivo == "" {
			bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "Il cambio della sorgente di una bozza aperta vuole il motivo.", m{"campo": "motivo"})
			return
		}
	}
	if !aperta && partenza == "solo_prodotto" {
		if c := b.conferme[prod]; len(c) > 0 && c[len(c)-1]["valida"] == true {
			bdKo(w, 409, "RICONCILIAZIONE_NON_DISPONIBILE", "il prodotto ha una conferma valida", m{"motivo": "solo_prodotto_su_conferma", "campo": "partenza"})
			return
		}
	}
	b.conti["S8"]++
	// lo spostamento (sposta.go, nella transazione del passo 2): la revoca «sorgente errata» e la bozza annullata sull'altro
	if spostamento != nil {
		p, codA := bdStr(spostamento["da_prodotto_id"]), bdStr(spostamento["da_codice"])
		nota := "sorgente_errata: lo STEP passa a " + cod + " (S8)"
		if motivo != "" {
			nota += ": " + motivo
		}
		spostamento["motivo"] = nota
		parti := []string{}
		s := b.s8[p]
		if s["valida"] == true {
			parti = append(parti, "sorgente di "+codA+" revocata (sorgente errata)")
		}
		s["valida"], s["motivo"], s["causa"], s["spostato_a"] = false, "revocata", "sorgente_errata", prod
		b.conti["revoche_s8"]++
		if bz := b.bozze[p]; bz != nil && bz.Stato == "aperta" {
			if sb, _ := bz.Snapshot["sorgente"].(m); sb != nil && bdStr(sb["allegato_id"]) == a {
				bz.Stato = "abbandonata"
				spostamento["bozza_abbandonata"] = bz.ID
				parti = append(parti, "bozza di "+codA+" annullata (la sua storia resta)")
			}
		}
		parti = append(parti, "documento passato a "+cod)
		spostamento["testo"] = "Spostamento: " + strings.Join(parti, "; ") + "."
	}
	passi := []any{m{"passo": "associazione", "esito": "non_serve", "testo": "Lo STEP sorgente non è un'associazione registrata: niente da fare."}}
	// passo 2: la conferma del documento come sorgente della bozza (mai l'autorizzazione, mai la working)
	if b.s8Valida(prod) == a {
		passi = append(passi, m{"passo": "conferma_documento", "esito": "gia_fatto", "testo": "Lo STEP era già la sorgente confermata."})
	} else {
		ora := time.Now().UTC().Format(time.RFC3339)
		prima := b.s8[prod]
		b.s8[prod] = m{"allegato_id": a, "sha256": f.Sha, "documento_id": "00000000-0000-4000-8000-0000000f0d0" + map[string]string{bdP1: "1", bdP2: "2"}[prod], "valida": true,
			"confermata_il": ora, "confermata_da": duUUID(77).String()}
		// G39-03: la correzione della sorgente, dichiarata (correzione_sorgente) o dopo la revoca «sorgente errata» di un altro
		// STEP entrato in una conferma della BOM (confermabom.StatoSorgente.Correzione); mai per lo spostamento qui
		if corpo["correzione_sorgente"] == true || (prima != nil && prima["causa"] == "sorgente_errata" && prima["usata"] == true && bdStr(prima["allegato_id"]) != a) {
			b.s8[prod]["correzione"] = true
		}
		// il passo 2 conferma il documento cad_3d del prodotto (core/rfq/confermadoc): lo STEP resta un documento confermato
		// anche quando un altro STEP lo supera come sorgente (S8 non sostituisce, direttiva 39 §2)
		if f.Documento == nil {
			f.Documento = m{"documento_id": b.s8[prod]["documento_id"], "stato": "confermato", "confermato_il": ora}
		}
		if prod == bdP1 {
			b.fonte["confermata"] = true
			b.fonte["allegato_id"] = a
			b.fonte["sha256"] = f.Sha
			b.fonte["voce"] = f.Proposta
			b.fonte["confermata_il"] = ora
		}
		passi = append(passi, m{"passo": "conferma_documento", "esito": "fatto", "testo": "STEP confermato come sorgente di " + cod + "."})
	}
	risposta := m{"riproposto": false, "chiave_idempotenza": chiave, "copia_nas": "in_coda"}
	if spostamento != nil {
		risposta["spostamento"] = spostamento
	}
	// passo 3: la bozza
	switch modo {
	case "parziale":
		passi = append(passi, m{"passo": "bozza", "esito": "rifiutato", "codice": "ANTEPRIMA_SUPERATA", "testo": "La proposta è cambiata dopo l'anteprima: la bozza non è nata."})
		b.versioneProposta++
		risposta["esito"] = "parziale"
		risposta["passi"] = passi
		risposta["testo"] = "Sorgente confermata; la bozza non è nata: la proposta è cambiata. Rivedi l'anteprima."
		b.intenti[chiave] = intentoBanco{Gesto: "conferma_sorgente", Impronta: impronta, Risorsa: "sorgente:" + prod, Risposta: risposta, Creato: time.Now().UTC()}
		bdJSON(w, http.StatusOK, risposta)
		return
	case "occupato":
		passi = append(passi, m{"passo": "bozza", "esito": "rifiutato", "codice": "CALCOLO_OCCUPATO", "testo": "Il motore è occupato: la bozza non è nata, riprova."})
		risposta["esito"] = "parziale"
		risposta["ritentabile"] = true
		risposta["passi"] = passi
		risposta["testo"] = "Sorgente confermata; la bozza non è ancora nata: il motore è occupato. Riprova con lo stesso gesto."
		bdJSON(w, http.StatusOK, risposta)
		return
	}
	var imp string
	if reg != nil {
		imp = bdImpronta(reg)
		b.registri[imp] = reg
	}
	riservata := m{"allegato_id": a, "sha256": f.Sha, "gruppo": "solo_prodotto"}
	switch {
	case stessa:
		passi = append(passi, m{"passo": "bozza", "esito": "gia_fatto", "testo": "La bozza è già su questa sorgente."})
	case manuale && partenza == "step":
		ab := b.riconcilia(bz, reg, imp)
		passi = append(passi, m{"passo": "bozza", "esito": "fatto", "testo": fmt.Sprintf("La bozza senza STEP è riconciliata con lo STEP (versione %d): %d abbinamenti da decidere.", bz.Versione, ab)})
	case manuale:
		// SoloProdottoSuBozzaFormato3: la sorgente reale con il gruppo riservato, niente registro, nient'altro cambia
		bz.Versione++
		s := m{}
		for k, v := range bz.Snapshot {
			s[k] = v
		}
		s["sorgente"] = riservata
		bz.Snapshot = bdSnapshotCanonico(s)
		b.eventoServer(bz, "sorgente_cambiata", m{"sorgente": riservata, "algoritmo": "solo-prodotto-1", "presa_d_atto_analisi": corpo["stato_analisi"], "da_rivedere": []any{}})
		passi = append(passi, m{"passo": "bozza", "esito": "fatto", "testo": fmt.Sprintf("La bozza ha la sorgente STEP senza struttura (versione %d).", bz.Versione)})
	case aperta:
		bz.Registro = imp
		bz.Versione++
		bz.Snapshot = bdSnapshotCanonico(b.statoDiPartenza(reg, imp))
		b.eventoServer(bz, "sorgente_cambiata", m{"sorgente": bz.Snapshot["sorgente"], "algoritmo": "cambio-fonte-1", "da_rivedere": []any{}})
		passi = append(passi, m{"passo": "bozza", "esito": "fatto", "testo": "La sorgente della bozza è cambiata (versione " + fmt.Sprint(bz.Versione) + ")."})
	default:
		b.nBozze++
		bz = &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 1, Creata: time.Now().UTC(), Registro: imp}
		if reg != nil {
			bz.Snapshot = bdSnapshotCanonico(b.statoDiPartenza(reg, imp))
			b.bozze[prod] = bz
			b.eventoServer(bz, "bozza_aperta", m{"partenza": "step", "importazione": imp})
			b.eventoServer(bz, "sorgente_scelta", m{"allegato_id": a, "gruppo": reg["gruppo"].(m)["chiave"]})
		} else {
			b.costruisciDaRegistrato(bz, prod, "registrato")
			bz.Base = nil
			s := m{}
			for k, v := range bz.Snapshot {
				s[k] = v
			}
			s["sorgente"] = riservata
			bz.Snapshot = bdSnapshotCanonico(s)
			b.bozze[prod] = bz
			b.eventoServer(bz, "bozza_aperta", m{"partenza": "solo_prodotto", "presa_d_atto_analisi": corpo["stato_analisi"]})
		}
		passi = append(passi, m{"passo": "bozza", "esito": "fatto", "testo": "Bozza creata (versione 1)."})
	}
	risposta["esito"] = "completata"
	risposta["passi"] = passi
	var impR any
	if imp != "" {
		impR = "00000000-0000-4000-8000-0000000b1001"
	}
	risposta["bozza"] = m{"bozza_id": bz.ID, "versione": bz.Versione, "importazione": impR}
	risposta["testo"] = "Sorgente confermata: la bozza di " + cod + " è pronta da rivedere. La BOM non è confermata."
	b.intenti[chiave] = intentoBanco{Gesto: "conferma_sorgente", Impronta: impronta, Risorsa: "sorgente:" + prod, Risposta: risposta, Creato: time.Now().UTC()}
	bdJSON(w, http.StatusOK, risposta)
}

// riconcilia: cambio-fonte-2 sulla bozza senza STEP, come lo fa la persistenza (distintabozza/riconciliazione3.go), nella
// misura del banco: la radice acquisisce la casella della radice; un nodo senza casella con lo stesso codice e la stessa
// rev di una voce la acquisisce (lo stesso nodo, la stessa chiave: niente doppioni; un manuale passa al modo proposta con
// la stessa identità); lo stesso codice con una rev diversa (o con altre maiuscole) è un abbinamento proposto
// «incompatibile», e la voce non entra; le altre voci entrano come nella partenza step. Gli archi: entrano quelli verso
// una voce che entra o verso un abbinato senza padre; quelli dell'ingegnere restano con le sue quantità. Le PB, le
// decisioni e il vassoio restano. Torna il numero di abbinamenti proposti.
func (b *bancoDU) riconcilia(bz *bozzaBanco, reg m, imp string) int {
	pieno := b.statoDiPartenza(reg, imp)
	perCodice := map[string]m{}
	for _, x := range pieno["nodi"].([]any) {
		n := x.(m)
		if id, _ := n["identita"].(m); id != nil && bdStr(id["codice"]) != "" && bdStr(n["chiave_bozza"]) != "k-root" {
			perCodice[strings.ToUpper(bdStr(id["codice"]))] = n
		}
	}
	voceDi := func(n m) string {
		c := n["caselle"].([]any)[0].(m)
		return "n:" + bdStr(c["allegato_id"]) + ":" + bdStr(c["rif"])
	}
	nodi := []any{}
	presi := map[string]string{} // chiave della voce nel pieno → chiave del nodo della bozza
	proposti := []any{}
	acquisite := []any{}
	usate := map[string]bool{}
	for _, x := range bdElenco(bz.Snapshot["nodi"]) {
		n := m{}
		for k, v := range x.(m) {
			n[k] = v
		}
		k := bdStr(n["chiave_bozza"])
		usate[k] = true
		if n["caselle"] == nil {
			if k == "k-root" {
				for _, y := range pieno["nodi"].([]any) {
					if yy := y.(m); bdStr(yy["chiave_bozza"]) == "k-root" {
						n["caselle"] = yy["caselle"]
						presi["k-root"] = "k-root"
						acquisite = append(acquisite, k)
					}
				}
			} else if id, _ := n["identita"].(m); id != nil && bdStr(id["codice"]) != "" {
				if v := perCodice[strings.ToUpper(bdStr(id["codice"]))]; v != nil {
					vid := v["identita"].(m)
					if bdStr(vid["codice"]) == bdStr(id["codice"]) && bdStr(vid["rev"]) == bdStr(id["rev"]) && n["evidenze_identita"] == nil {
						n["caselle"] = v["caselle"]
						if id["modo"] == "manuale" {
							nid := m{}
							for kk, vv := range id {
								nid[kk] = vv
							}
							nid["modo"] = "proposta"
							n["identita"] = nid
						}
						presi[bdStr(v["chiave_bozza"])] = k
						acquisite = append(acquisite, k)
					} else {
						proposti = append(proposti, m{"chiave_bozza": k, "voci": []any{voceDi(v)}, "motivo": "incompatibile"})
						presi[bdStr(v["chiave_bozza"])] = ""
					}
				}
			}
		}
		nodi = append(nodi, n)
	}
	entrati := map[string]string{}
	for _, x := range pieno["nodi"].([]any) {
		v := x.(m)
		kv := bdStr(v["chiave_bozza"])
		if _, ok := presi[kv]; ok {
			continue
		}
		k := kv
		for usate[k] {
			k += "s"
		}
		usate[k] = true
		nv := m{}
		for kk, vv := range v {
			nv[kk] = vv
		}
		nv["chiave_bozza"] = k
		entrati[kv] = k
		nodi = append(nodi, nv)
	}
	archi := bdElenco(bz.Snapshot["archi"])
	conPadre := map[string]bool{}
	for _, x := range archi {
		conPadre[bdStr(x.(m)["figlio"])] = true
	}
	mappa := func(k string) string {
		if e, ok := entrati[k]; ok {
			return e
		}
		return presi[k]
	}
	for _, x := range pieno["archi"].([]any) {
		a := x.(m)
		p, fi := mappa(bdStr(a["padre"])), mappa(bdStr(a["figlio"]))
		if p == "" || fi == "" {
			continue
		}
		_, entra := entrati[bdStr(a["figlio"])]
		if !entra && conPadre[fi] {
			continue
		}
		na := m{}
		for kk, vv := range a {
			na[kk] = vv
		}
		na["padre"], na["figlio"] = p, fi
		archi = append(archi, na)
		conPadre[fi] = true
	}
	s := m{}
	for k, v := range bz.Snapshot {
		s[k] = v
	}
	s["sorgente"] = pieno["sorgente"]
	s["nodi"], s["archi"] = nodi, archi
	bz.Registro = imp
	bz.Versione++
	bz.Snapshot = bdSnapshotCanonico(s)
	b.eventoServer(bz, "sorgente_cambiata", m{"sorgente": pieno["sorgente"], "algoritmo": "cambio-fonte-2", "da_rivedere": []any{}, "caselle_riscritte": acquisite,
		"abbinamenti_proposti": proposti, "conteggi": m{"abbinati": len(acquisite), "abbinamenti_proposti": len(proposti), "entrati": len(entrati)}})
	return len(proposti)
}

// revocaS8: POST /thread/{id}/sorgente/step/{a}/revoca-sorgente {chiave_idempotenza, prodotto_id, motivo} (direttiva 39
// §2; core/rfq/sorgente/revoca.go): la sorgente S8 valida dello STEP non vale più; working, documento, bozza e NAS non
// cambiano.
func (b *bancoDU) revocaS8(w http.ResponseWriter, r *http.Request) {
	a := r.PathValue("a")
	corpo, _, err := bdCorpo(r)
	if err != nil {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il corpo non è JSON", nil)
		return
	}
	b.corpiFile = append(b.corpiFile, m{"gesto": "revoca_sorgente", "allegato": a, "corpo": corpo})
	chiave := bdStr(corpo["chiave_idempotenza"])
	senza := m{}
	for k, v := range corpo {
		if k != "chiave_idempotenza" {
			senza[k] = v
		}
	}
	impronta := bdSha(bdCanonico(m{"gesto": "revoca_sorgente", "thread": bdThread, "allegato": a, "corpo": senza}))
	if it, ok := b.intenti[chiave]; ok {
		if it.Impronta != impronta {
			bdKo(w, 409, "CHIAVE_RIUSATA", "la chiave ha già un esito diverso", nil)
			return
		}
		rr := m{}
		for k, v := range it.Risposta {
			rr[k] = v
		}
		rr["riproposto"] = true
		bdJSON(w, http.StatusOK, rr)
		return
	}
	if strings.TrimSpace(bdStr(corpo["motivo"])) == "" {
		bdKo(w, 422, "MOTIVO_OBBLIGATORIO", "la revoca della sorgente vuole il motivo", m{"campo": "motivo"})
		return
	}
	prod := bdStr(corpo["prodotto_id"])
	if prod != bdP1 && prod != bdP2 {
		bdKo(w, 422, "FUORI_RFQ", "il prodotto non è di questa RFQ", m{"campo": "prodotto_id"})
		return
	}
	if v, ok := corpo["sorgente_errata"]; ok && v != true {
		bdKo(w, 422, "CORPO_NON_VALIDO", "sorgente_errata, se c'è, è true", m{"campo": "sorgente_errata"})
		return
	}
	if b.s8Valida(prod) != a {
		bdKo(w, 422, "CORPO_NON_VALIDO", "lo STEP non è una sorgente confermata da S8 e valida: non c'è niente da revocare", m{"campo": "allegato", "motivo": "nessuna_sorgente_da_revocare"})
		return
	}
	// G39-03 (revoca.go): la bozza dichiarata dalla pagina deve essere ancora quella, alla stessa versione
	bz := b.bozze[prod]
	aperta := bz != nil && bz.Stato == "aperta"
	if cb, ok := corpo["bozza"].(m); ok {
		switch {
		case !aperta:
			bdKo(w, 409, "BOZZA_VERSIONE", "la bozza dichiarata non è più aperta: rileggi la pagina", m{"campo": "bozza", "motivo": "bozza_chiusa"})
			return
		case bdStr(cb["bozza_id"]) != bz.ID || bdNum(cb["versione_attesa"]) != bz.Versione:
			bdKo(w, 409, "BOZZA_VERSIONE", fmt.Sprintf("la bozza aperta del prodotto è alla versione %d: rileggila", bz.Versione), m{"campo": "bozza", "motivo": "versione_diversa",
				"conflitto": m{"bozza_id": bz.ID, "stato": "aperta", "versione": bz.Versione}})
			return
		}
	}
	motivoReg := bdStr(corpo["motivo"])
	if corpo["sorgente_errata"] == true {
		motivoReg = "sorgente_errata: " + motivoReg
		b.s8[prod]["causa"] = "sorgente_errata"
	}
	usata := b.usataDaConferma(prod, a)
	b.s8[prod]["valida"] = false
	b.s8[prod]["motivo"] = "revocata"
	b.s8[prod]["usata"] = usata
	b.conti["revoche_s8"]++
	cod := map[string]string{bdP1: "ACM-1000", bdP2: "ACM-2000"}[prod]
	risposta := m{"esito": "revocata", "riproposto": false, "chiave_idempotenza": chiave, "prodotto_id": prod, "allegato_id": a, "documento_id": b.s8[prod]["documento_id"], "motivo": motivoReg,
		"testo": "Sorgente revocata per " + cod + ": la working non cambia; la conferma della BOM aspetta una sorgente confermata di nuovo (S8)."}
	if aperta {
		if sb, _ := bz.Snapshot["sorgente"].(m); sb != nil && bdStr(sb["allegato_id"]) == a {
			risposta["bozza_id"] = bz.ID
			switch {
			case usata:
				risposta["usata_da_conferma"] = true
				risposta["testo"] = "Sorgente revocata per " + cod + ": lo STEP è nella BOM confermata, quindi la working e la bozza non cambiano. Il cambio di sorgente è una revisione interna: conferma la sorgente giusta (S8), poi rivedi e conferma la BOM."
			default:
				bz.Stato = "abbandonata"
				risposta["bozza_abbandonata"] = bz.ID
				risposta["testo"] = "Sorgente revocata per " + cod + ": la bozza nata da questo STEP è annullata (la sua storia resta); la working non cambia. Conferma la sorgente giusta (S8): la bozza nuova nasce da lì."
			}
		}
	}
	b.intenti[chiave] = intentoBanco{Gesto: "revoca_sorgente", Impronta: impronta, Risorsa: "sorgente:" + prod, Risposta: risposta, Creato: time.Now().UTC()}
	bdJSON(w, http.StatusOK, risposta)
}

// usataDaConferma: lo STEP è entrato in una conferma della BOM del prodotto (confermabom.UsataDaConferma): una conferma con
// quella fonte, o l'interruttore «bom-confermata-altro» per il secondo prodotto (G39-03 [U])
func (b *bancoDU) usataDaConferma(prod, a string) bool {
	if b.bomConfermataAltro && prod == bdP2 {
		return true
	}
	for _, c := range b.conferme[prod] {
		if s, _ := c["fonte"].(m); s != nil && bdStr(s["allegato_id"]) == a {
			return true
		}
	}
	return false
}

// bloccoSorgente: il blocco firmato STEP_SORGENTE_NON_CONFERMATA di R4/R5 (direttiva 39 §1; EM App. I.3), con
// l'interruttore «gate-sorgente»: la sorgente della bozza (anche la riservata solo_prodotto) dev'essere lo STEP confermato
// da S8 e valido del prodotto. Nil se non scatta.
func (b *bancoDU) bloccoSorgente(prod string, snap m) m {
	if !b.gateSorgente {
		return nil
	}
	s, _ := snap["sorgente"].(m)
	valida := b.s8Valida(prod)
	motivo := ""
	switch {
	case s == nil:
		motivo = "senza_sorgente"
	case valida == "" && b.s8[prod] != nil && bdStr(b.s8[prod]["allegato_id"]) == bdStr(s["allegato_id"]):
		motivo = bdStr(b.s8[prod]["motivo"])
	case valida != bdStr(s["allegato_id"]):
		motivo = "non_confermata"
	}
	if motivo == "" {
		return nil
	}
	o := m{"codice": "STEP_SORGENTE_NON_CONFERMATA", "motivo": motivo,
		"testo": "La bozza non ha uno STEP confermato come sorgente: confermalo dalla Sorgente («Conferma questo STEP come sorgente»), poi conferma la BOM."}
	if s != nil {
		o["sorgente"] = m{"allegato_id": s["allegato_id"], "sha256": s["sha256"]}
	}
	return o
}

// r2CreaVuota: R2 crea con la partenza vuota (direttiva 39 §1; distintabozza/crea3.go): il solo prodotto finito, senza
// sorgente, registro né storia della fonte; 422 partenza_non_ammessa con una conferma formato 3 valida o con una sorgente
// S8 valida (R39-19).
func (b *bancoDU) r2CreaVuota(w http.ResponseWriter, prod string) m {
	if c := b.conferme[prod]; len(c) > 0 && bdNum(c[len(c)-1]["formato"]) == 3 && c[len(c)-1]["valida"] == true {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il prodotto ha una conferma formato 3 valida: si parte da quella", m{"motivo": "partenza_non_ammessa", "campo": "crea.partenza"})
		return nil
	}
	if b.s8Valida(prod) != "" {
		bdKo(w, 422, "CORPO_NON_VALIDO", "il prodotto ha uno STEP confermato da S8: la bozza nasce da lì", m{"motivo": "partenza_non_ammessa", "campo": "crea.partenza"})
		return nil
	}
	b.nBozze++
	bz := &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 1, Creata: time.Now().UTC()}
	b.costruisciDaRegistrato(bz, prod, "registrato")
	bz.Base = nil
	b.bozze[prod] = bz
	b.eventoServer(bz, "bozza_aperta", m{"partenza": "vuota"})
	return m{"bozza_id": bz.ID, "formato": 3, "versione": 1, "nuova_versione": true, "riproposto": false, "eventi": 1, "impronta": bdSha(bdCanonico(bz.Snapshot)), "salvata_il": time.Now().UTC(), "snapshot": bz.Snapshot}
}

// sondaD39: le sonde della direttiva 39. POST /__banco/analisi-step {nome, stato}: lo stato dell'analisi di un file
// (letto, in_analisi, errore); POST /__banco/legame-storico {prodotto}: il legame storico univoco di D-AR04 (iii) sul
// primo STEP, per un prodotto senza conferme di S8. Torna false se la richiesta non è sua.
func (b *bancoDU) sondaD39(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/__banco/analisi-step":
		var o struct{ Nome, Stato string }
		_ = json.NewDecoder(r.Body).Decode(&o)
		for _, f := range b.file {
			if f.Nome == o.Nome {
				// fermo: l'analisi resta com'è finché la sonda non la cambia (S1 non la conclude da sola)
				f.Analisi, f.fermo, f.letture = o.Stato, o.Stato != "letto", 0
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	case "/__banco/legame-storico":
		var o struct{ Prodotto string }
		_ = json.NewDecoder(r.Body).Decode(&o)
		b.fonte["legame_storico_"+o.Prodotto] = m{"documento_id": "00000000-0000-4000-8000-0000000f0d09", "sha256": bdShaStep, "allegato_id": bdA1}
		w.WriteHeader(http.StatusNoContent)
		return true
	case "/__banco/sorgente-s8":
		// {prodotto}: dati di prova per le prove che riguardano altro (PE-23, R6): la bozza aperta del prodotto riceve come
		// sorgente uno STEP confermato da S8 e valido, così il gate della sorgente (acceso come nel server) la lascia
		// confermare. Per il primo prodotto è il suo STEP (ACM-1000.stp); il secondo nel banco non ha STEP fra i file: uno
		// STEP fuori dall'elenco, dichiarato qui. La sorgente va anche nella versione 1 (Base), così uguale_alla_base non cambia.
		// G39-03 [U]: con allegato (un file del banco) quello STEP diventa la sorgente S8 valida del prodotto, e con bozza il
		// prodotto ha una bozza aperta nata da lui (lo STEP della BOM confermato per errore sul secondo prodotto)
		var o struct {
			Prodotto, Allegato string
			Bozza              bool
		}
		_ = json.NewDecoder(r.Body).Decode(&o)
		a, sha, doc := bdA1, bdShaStep, "00000000-0000-4000-8000-0000000f0d01"
		if o.Prodotto == bdP2 {
			a, sha, doc = "00000000-0000-4000-8000-0000000a2051", bdSha("step-acm-2000"), "00000000-0000-4000-8000-0000000f0d02"
		}
		if f := b.fileDi(o.Allegato); f != nil {
			a, sha = f.ID, f.Sha
			b.s8[o.Prodotto] = m{"allegato_id": a, "sha256": sha, "documento_id": doc, "valida": true, "confermata_il": time.Now().UTC().Format(time.RFC3339), "confermata_da": duUUID(77).String()}
			if bz := b.bozze[o.Prodotto]; o.Bozza && (bz == nil || bz.Stato != "aperta") {
				b.nBozze++
				bz = &bozzaBanco{ID: fmt.Sprintf("00000000-0000-4000-8000-0000000b%04d", b.nBozze), Stato: "aperta", Versione: 1, Creata: time.Now().UTC()}
				b.costruisciDaRegistrato(bz, o.Prodotto, "registrato")
				bz.Base = nil
				b.bozze[o.Prodotto] = bz
				b.eventoServer(bz, "bozza_aperta", m{"partenza": "solo_prodotto"})
			}
		}
		// una sorgente S8 già valida resta quella (le prove che seguono la ritrovano com'era)
		if v := b.s8Valida(o.Prodotto); v != "" {
			a, sha = v, bdStr(b.s8[o.Prodotto]["sha256"])
		} else {
			b.s8[o.Prodotto] = m{"allegato_id": a, "sha256": sha, "documento_id": doc, "valida": true, "confermata_il": time.Now().UTC().Format(time.RFC3339), "confermata_da": duUUID(77).String()}
		}
		if bz := b.bozze[o.Prodotto]; bz != nil && bz.Stato == "aperta" {
			bz.Snapshot["sorgente"] = m{"allegato_id": a, "sha256": sha, "gruppo": bdGruppo(a, sha)}
			if bz.Base != nil {
				bz.Base["sorgente"] = m{"allegato_id": a, "sha256": sha, "gruppo": bdGruppo(a, sha)}
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
