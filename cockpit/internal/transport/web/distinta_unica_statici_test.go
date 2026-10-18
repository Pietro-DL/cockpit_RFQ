package web

// L1 — giro 5, A1d, V1: i controlli statici dei file della Distinta unica (direttiva 23 §6.3; MAT §1.3, §1.4, §7; EM §9,
// §10.9, §10.10). Sono la forma di T-06 per il ponte nuovo (il mondo chiuso del ponte riscritto di proposito) e la parte
// di T-05 che si può fare senza l'elenco unico dei codici del server, che la corsia persistenza non ha ancora
// pubblicato: i cataloghi del ponte confrontati con i codici scritti in EM, rotta per rotta.
//   - il ponte (distinta_unica_ponte.mjs): una funzione per rotta, con i percorsi di EM alla lettera; i sei atti della
//     lista chiusa ridotta e nessun altro; due sole fetch (il trasporto JSON e gli atti); la chiave del tentativo in
//     sospeso nella memoria di sessione della scheda, con l'utente nella chiave; nessun gesto legacy;
//   - la pagina (distinta_unica.mjs): solo presentazione; nessuna richiesta al server fuori dal ponte; un solo import, il
//     ponte; nessun HTML inserito; niente localStorage per lo stato della bozza (C3 §6.2);
//   - il foglio di stile vale solo dentro la pagina; nessun indirizzo esterno; nessun gesto legacy in nessun file.
// I file si leggono dall'embed (risorse.FS): è quello che il server serve.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	duPonte    = "web/static/distinta_unica_ponte.mjs"
	duPagina   = "web/static/distinta_unica.mjs"
	duCSS      = "web/static/distinta_unica.css"
	duTemplate = "web/templates/distinta_unica.html"
)

// duBlocco: il testo fra «// NOME:inizio» e «// NOME:fine».
func duBlocco(t *testing.T, src, nome string) string {
	t.Helper()
	i := strings.Index(src, "// "+nome+":inizio")
	j := strings.Index(src, "// "+nome+":fine")
	if i < 0 || j < i {
		t.Fatalf("il ponte non ha il blocco %s", nome)
	}
	return src[i:j]
}

// Le rotte del ponte di V1: una per funzione, con i percorsi del C3 §1.2 e di EM §9 e §10.9 alla lettera, e ogni
// funzione esportata con lo stesso nome (MAT §1.3).
func TestPonteDistintaUnicaRotte(t *testing.T) {
	src := duFile(t, duPonte)
	attese := map[string]string{
		"leggiModello":            "GET /thread/{id}/motore-a/dati",                       // modello della pagina (EM §9.9)
		"anteprimaAutorizzazione": "GET /thread/{id}/motore-a/anteprima/step-strutturale", // MAT SG-09
		"leggiBozza":              "GET /thread/{id}/motore-a/bozza",                      // R1
		"salvaBozza":              "POST /thread/{id}/motore-a/bozza",                     // R2, salvataggio
		"creaBozza":               "POST /thread/{id}/motore-a/bozza",                     // R2, creazione
		"cambiaSorgente":          "POST /thread/{id}/motore-a/bozza",                     // R2, cambia_sorgente
		"riallineaProposta":       "POST /thread/{id}/motore-a/bozza",                     // R2, riallinea_proposta
		"convertiBozza":           "POST /thread/{id}/motore-a/bozza",                     // R2, converti_da
		"abbandonaBozza":          "POST /thread/{id}/motore-a/bozza/{u}/abbandona",       // R3
		"riepilogoBom":            "POST /thread/{id}/motore-a/bom/riepilogo",             // R4
		"confermaBom":             "POST /thread/{id}/motore-a/bom/conferma",              // R5
		"esitoIntento":            "GET /thread/{id}/motore-a/intento/{u}",                // R6
		"storicoConferme":         "GET /thread/{id}/motore-a/conferme",                   // R7
		"riallineaDocumenti":      "POST /thread/{id}/motore-a/riallinea",                 // R11
		"carica":                  "POST /thread/{id}/carica",                             // E1′
		"leggiSorgente":           "GET /thread/{id}/sorgente",                            // S1
		"scartaFile":              "POST /thread/{id}/sorgente/file/{u}/scarta",           // S2
		"altriFile":               "POST /thread/{id}/sorgente/file/{u}/altri-file",       // S3
		"riattivaFile":            "POST /thread/{id}/sorgente/file/{u}/riattiva",         // S4
		"urlContenuto":            "GET /thread/{id}/sorgente/file/{u}/contenuto",         // S5
		"anteprimaStep":           "GET /thread/{id}/sorgente/step/{u}/anteprima",         // S6
		"associaStep":             "POST /thread/{id}/sorgente/step/{u}/associa",          // S7
	}
	trovate := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(\w+): "([A-Z]+ [^"]+)",`).FindAllStringSubmatch(duBlocco(t, src, "DEDICATE"), -1) {
		trovate[m[1]] = m[2]
	}
	if len(trovate) != len(attese) {
		t.Errorf("il ponte ha %d rotte dedicate, attese %d: %v", len(trovate), len(attese), trovate)
	}
	for nome, rotta := range attese {
		if trovate[nome] != rotta {
			t.Errorf("rotta di %s: %q, attesa %q", nome, trovate[nome], rotta)
		}
		if !regexp.MustCompile(`(?m)^export (async )?function ` + nome + `\(`).MatchString(src) {
			t.Errorf("il ponte non esporta la funzione %s", nome)
		}
	}
	// le cinque forme di R2 passano tutte da scriviR2 con il loro nome; il riallineamento della proposta non si chiama
	// mai «riallinea» da solo, e R11 è riallineaDocumenti (EM §9)
	for _, nome := range []string{"salvaBozza", "creaBozza", "cambiaSorgente", "riallineaProposta", "convertiBozza"} {
		if !strings.Contains(src, `return scriviR2("`+nome+`", corpo);`) {
			t.Errorf("%s non passa da scriviR2 con il suo nome", nome)
		}
	}
	if regexp.MustCompile(`export (async )?function riallinea\(`).MatchString(src) {
		t.Error("il ponte esporta «riallinea»: il riallineamento della proposta è riallineaProposta, quello di F8 riallineaDocumenti")
	}
	// le scritture con la chiave: R5, S2, S3, S4, S7 (EM §8.4, §10.9)
	for _, nome := range []string{"confermaBom", "scartaFile", "altriFile", "riattivaFile", "associaStep"} {
		if !strings.Contains(src, `scriviConChiave("`+nome+`"`) {
			t.Errorf("%s non passa da scriviConChiave", nome)
		}
	}
}

// La lista chiusa ridotta: i sei atti di EM §9.10, nient'altro (MAT §1.4); due sole fetch nel ponte, nessuna nella pagina.
func TestPonteDistintaUnicaListaChiusa(t *testing.T) {
	src := duFile(t, duPonte)
	attesi := map[string]string{
		"autorizza":     "/thread/{id}/fascicolo/componente/{u}/step-strutturale",
		"revoca":        "/thread/{id}/fascicolo/componente/{u}/step-strutturale/revoca",
		"confermaFonte": "/thread/{id}/fascicolo/conferma",
		"prepara":       "/thread/{id}/fascicolo/prepara",
		"rianalizza":    "/thread/{id}/fascicolo/rianalizza",
		"riprovaCopie":  "/thread/{id}/riprova-copie",
	}
	trovati := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(\w+): "(/[^"]+)",`).FindAllStringSubmatch(duBlocco(t, src, "ATTI"), -1) {
		trovati[m[1]] = m[2]
	}
	if len(trovati) != len(attesi) {
		t.Errorf("atti del ponte: %v, attesi i sei di EM §9.10", trovati)
	}
	for k, v := range attesi {
		if trovati[k] != v {
			t.Errorf("atto %s: %q, atteso %q", k, trovati[k], v)
		}
	}
	if n := strings.Count(src, "fetch("); n != 2 {
		t.Errorf("%d fetch( nel ponte, attese due (chiama ed esegui)", n)
	}
	pagina := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(duFile(t, duPagina), "")
	for _, v := range []*regexp.Regexp{regexp.MustCompile(`\bfetch\s*\(`), regexp.MustCompile(`XMLHttpRequest`), regexp.MustCompile(`sendBeacon`),
		regexp.MustCompile(`WebSocket`), regexp.MustCompile(`EventSource`), regexp.MustCompile(`\bhtmx\b`), regexp.MustCompile(`\.submit\s*\(`)} {
		if m := v.FindString(pagina); m != "" {
			t.Errorf("la pagina contiene %q: ogni richiesta al server passa dal ponte (D23:284)", m)
		}
	}
}

// Nessun gesto legacy (MAT §7) nel ponte, nella pagina e nel guscio: né «Conferma l'albero», né i gesti strutturali
// immediati, né «Scarta il file», né le letture /distinta/*.
func TestDistintaUnicaNessunGestoLegacy(t *testing.T) {
	vietati := []*regexp.Regexp{
		regexp.MustCompile(`/distinta/(albero|tipo)`),
		regexp.MustCompile(`/fascicolo/nodo/`),
		regexp.MustCompile(`/fascicolo/rimozione/`),
		regexp.MustCompile(`/fascicolo/componente/\{u\}/(codice|modifica|tipo|collega|sposta|scollega|archivia|rimuovi|ripristina|deroga)`),
		regexp.MustCompile(`/fascicolo/(documento|deroga)/`),
		regexp.MustCompile(`/fascicolo/assegna`),
		regexp.MustCompile(`/proposta/`),
		regexp.MustCompile(`/smistamento/aggiorna`),
		regexp.MustCompile(`(?i)confermaAlbero|Conferma l'albero`),
		regexp.MustCompile(`Scarta il file`),
	}
	for _, f := range []string{duPonte, duPagina, duTemplate} {
		src := duFile(t, f)
		for _, v := range vietati {
			// il ponte nomina «Scarta il file» e «Conferma l'albero» nei commenti di testa per dire che non ci sono
			codice := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(src, "")
			codice = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`).ReplaceAllString(codice, "")
			if m := v.FindString(codice); m != "" {
				t.Errorf("%s contiene il gesto legacy %q (MAT §7)", f, m)
			}
		}
	}
}

// La pagina fa solo presentazione (D23:280): nessuna API che inserisca HTML, nessun codice dinamico, un solo import (il
// ponte), nessun indirizzo di una rotta scritto fuori dal ponte, niente localStorage per la bozza (C3 §6.2).
func TestDistintaUnicaJSSoloPresentazione(t *testing.T) {
	for _, f := range []string{duPagina, duPonte} {
		src := duFile(t, f)
		for _, v := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "<iframe", "localStorage"} {
			if strings.Contains(src, v) {
				t.Errorf("%s contiene %q", f, v)
			}
		}
	}
	pagina := duFile(t, duPagina)
	importazioni := regexp.MustCompile(`(?m)^\s*import\b.*$`).FindAllString(pagina, -1)
	if len(importazioni) != 1 || strings.TrimSpace(importazioni[0]) != `import * as P from "./distinta_unica_ponte.mjs";` {
		t.Errorf("import della pagina: %v; atteso solo il ponte", importazioni)
	}
	// un solo import dinamico: pdf.js per il visore dei PDF, dagli statici che il guscio dice (data-du-pdfjs), come nella
	// pagina vecchia (MAT FL-03, PN-04)
	if n := strings.Count(pagina, "import("); n != 1 || !strings.Contains(pagina, "pdfjsPromessa = import(src)") || !strings.Contains(pagina, "const src = m && m.dataset.duPdfjs;") {
		t.Errorf("la pagina ha %d import dinamici: atteso solo quello di pdf.js dagli statici del guscio", n)
	}
	codice := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(pagina, "")
	if m := regexp.MustCompile("[\"'`]/(thread|proposta|allegato|fascicolo|sorgente|motore-a)/").FindString(codice); m != "" {
		t.Errorf("la pagina scrive l'indirizzo di una rotta (%s): gli indirizzi li costruisce solo il ponte", m)
	}
	if strings.Contains(codice, "sessionStorage") {
		t.Error("la pagina usa sessionStorage: la memoria della scheda per i tentativi in sospeso è del ponte")
	}
	ponte := duFile(t, duPonte)
	if !strings.Contains(ponte, `const PREFISSO = "cockpit.du.sospeso.";`) || !strings.Contains(ponte, `return PREFISSO + UTENTE + "." + RFQ + "." + risorsa;`) {
		t.Error("la chiave del tentativo in sospeso non porta prefisso, utente e RFQ (EM §10.9; MAT §5.5)")
	}
	for _, v := range []string{"import ", "await "} {
		for _, riga := range strings.Split(ponte, "\n") {
			if strings.HasPrefix(riga, v) {
				t.Errorf("il ponte ha %q al primo livello: %s", v, riga)
			}
		}
	}
}

// Le richieste R-2 e R-3 della corsia, fatte nell'ondata B sui due template autorizzati. R-3, come corretta nel secondo
// passaggio: le chiavi cockpit.du. della memoria di sessione della scheda si tolgono solo a uscita avvenuta, sulla
// pagina senza il form «esci» (la pagina d'accesso) e solo con la marca appena lasciata da «esci»; il clic su «esci»
// toglie subito solo le chiavi dell'editor, come prima, e un gesto sulla pagina (si è restati) toglie la marca (MAT §5.5
// n. 7). Il prefisso del ponte è cockpit.du.; il menu della pagina vecchia non dice più che «Prepara i file» rilegge gli
// STEP già analizzati (lo fa «Rifai l'analisi»).
func TestDistintaUnicaUscitaETestoDellaPaginaVecchia(t *testing.T) {
	layout := duFile(t, "web/templates/layout.html")
	i := strings.Index(layout, `var MARCA = "cockpit.uscita";`)
	if i < 0 || !strings.Contains(layout[i:], `getElementById("esci")`) {
		t.Fatal("layout.html non ha più l'uscita con la marca e il form «esci»")
	}
	uscita := layout[i:]
	if j := strings.Index(uscita, "})();"); j > 0 {
		uscita = uscita[:j]
	}
	tratto := func(da, a string) string {
		k := strings.Index(uscita, da)
		if k < 0 {
			t.Fatalf("l'uscita del layout non ha %s", da)
		}
		s := uscita[k:]
		if e := strings.Index(s, a); e > 0 {
			s = s[:e]
		}
		return s
	}
	for _, v := range []string{`document.addEventListener("pointerdown", resta, true);`, `document.addEventListener("keydown", resta, true);`} {
		if !strings.Contains(uscita, v) {
			t.Errorf("l'uscita del layout non ha %s", v)
		}
	}
	accesso := tratto("if (!f) {", "return;")
	for _, v := range []string{`d.indexOf("cockpit.du.") === 0`, "sessionStorage.removeItem(d)", "Date.now() - quando) < 60000", "sessionStorage.removeItem(MARCA)"} {
		if !strings.Contains(accesso, v) {
			t.Errorf("la pagina senza il form «esci» non toglie le chiavi cockpit.du. con la sola marca appena scritta (R-3): manca %s", v)
		}
	}
	invio := tratto(`f.addEventListener("submit"`, "});")
	if !strings.Contains(invio, `k.indexOf("cockpit.editor.") === 0`) || !strings.Contains(invio, "sessionStorage.removeItem(k)") ||
		!strings.Contains(invio, "sessionStorage.setItem(MARCA, String(Date.now()))") {
		t.Error("il clic su «esci» non toglie le chiavi dell'editor o non lascia la marca")
	}
	if strings.Contains(invio, "cockpit.du.") {
		t.Error("il clic su «esci» toglie le chiavi cockpit.du. prima che l'uscita sia avvenuta (MAT §5.5 n. 7)")
	}
	if !strings.Contains(duFile(t, duPonte), `const PREFISSO = "cockpit.du.`) {
		t.Error("il prefisso dei tentativi in sospeso del ponte non comincia con cockpit.du.: l'uscita non lo toglierebbe")
	}
	vecchia := duFile(t, "web/templates/motore_a.html")
	if strings.Contains(vecchia, "rilegge quelli già analizzati") || !strings.Contains(vecchia, "non rilegge gli STEP già analizzati") {
		t.Error("il menu della pagina vecchia dice ancora che «Prepara i file» rilegge gli STEP già analizzati")
	}
}

// «Riprova» dei gesti dei file (EM §10.9, «Reinvio e risposta persa»; MAT §5.5 n. 4): il tentativo in sospeso porta il
// corpo solo per S2, S3, S4 e S7, piccoli (la conferma della BOM si riprova dal wizard, che rilegge il riepilogo), e
// riprovaSospeso rimanda il corpo di allora passando da scriviConChiave, che riusa la chiave per lo stesso corpo.
func TestPonteDistintaUnicaRiprovaDeiFile(t *testing.T) {
	src := duFile(t, duPonte)
	if !strings.Contains(src, `const GESTI_FILE = ["scartaFile", "altriFile", "riattivaFile", "associaStep"];`) {
		t.Error("l'elenco dei gesti dei file che portano il corpo nel tentativo in sospeso non è quello di S2, S3, S4 e S7")
	}
	if !regexp.MustCompile(`(?m)^export async function riprovaSospeso\(`).MatchString(src) ||
		!strings.Contains(src, "return scriviConChiave(s.nome, risorsa, s.u, s.corpo, s.etichetta);") {
		t.Error("riprovaSospeso non rimanda il corpo di allora con scriviConChiave")
	}
	// la pagina lo chiama dalla coda della risorsa del file (MAT §1.3)
	if !strings.Contains(duFile(t, duPagina), "inSerie(s.risorsa, () => P.riprovaSospeso(s.risorsa))") {
		t.Error("la pagina non usa riprovaSospeso, dalla coda della risorsa, per «Riprova» dei gesti dei file")
	}
}

// Le regole del ponte rifatte dopo la revisione della corsia: l'impronta di R5 sulla forma canonica di EM §3.10 (lo
// stesso contenuto in un altro ordine riusa la chiave), il testo vero di PONTE_IN_VOLO (la scrittura non è partita, la
// coda sta nella pagina), il tempo massimo delle chiamate JSON e degli atti (una richiesta appesa non tiene la risorsa
// per sempre) e, per il caricamento, il tempo d'inattività al posto del tempo massimo (ondata B).
func TestPonteDistintaUnicaFormaCanonicaETempi(t *testing.T) {
	src := duFile(t, duPonte)
	if !regexp.MustCompile(`(?m)^export function formaCanonica\(`).MatchString(src) ||
		!strings.Contains(src, `return formaCanonica({ firma_anteprima: corpo.firma_anteprima, bozza: b }, "");`) {
		t.Error("l'impronta di R5 non si calcola sulla forma canonica di EM §3.10")
	}
	for _, c := range []string{"nodi: (x) => [x.chiave_bozza]", "archi: (x) => [x.padre, x.figlio]", "decisioni: (x) => [x.id]", "documenti_preassociati: (x) => [x.proposta_id]",
		"caselle: (x) => [x.gruppo, x.rif, x.allegato_id]"} {
		if !strings.Contains(src, c) {
			t.Errorf("la forma canonica non ordina come EM §3.10: manca %q", c)
		}
	}
	if strings.Contains(src, "resta in coda") || !strings.Contains(src, "questa non è partita") {
		t.Error("il testo di PONTE_IN_VOLO non dice che la scrittura non è partita")
	}
	if n := strings.Count(src, "signal: segnale(),"); n != 2 {
		t.Errorf("%d fetch con il tempo massimo, attese due (chiama ed esegui)", n)
	}
	// E1′: un tempo d'inattività, non un tempo massimo totale (decisioni dell'ondata A, §5 n. 3). Il caricamento passa da
	// inviaConInattivita (XMLHttpRequest, che dice i byte inviati), che riarma il tempo a ogni byte inviato o ricevuto; non
	// passa da chiama né da AbortSignal.timeout. XMLHttpRequest e setTimeout stanno solo lì
	carica := duFunzione(t, src, "export async function carica(")
	if !strings.Contains(carica, "inviaConInattivita(") || strings.Contains(carica, "chiama(") || strings.Contains(carica, "segnale(") {
		t.Error("carica (E1′) non passa da inviaConInattivita, o ha ancora il tempo massimo delle chiamate JSON")
	}
	invio := duFunzione(t, src, "function inviaConInattivita(")
	for _, v := range []string{"new XMLHttpRequest()", `x.upload.addEventListener("progress"`, `x.addEventListener("progress", riarma)`, "setTimeout(", "INATTIVITA", `"PONTE_INATTIVITA"`} {
		if !strings.Contains(invio, v) {
			t.Errorf("inviaConInattivita non ha %s", v)
		}
	}
	if strings.Contains(invio, "AbortSignal") {
		t.Error("inviaConInattivita ha un tempo massimo totale (AbortSignal)")
	}
	codice := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(src, "")
	if n := strings.Count(codice, "XMLHttpRequest"); n != 1 {
		t.Errorf("%d XMLHttpRequest nel ponte, atteso uno solo (il caricamento)", n)
	}
	if n := strings.Count(codice, "setTimeout("); n != 1 {
		t.Errorf("%d setTimeout nel ponte, atteso uno solo (il tempo d'inattività del caricamento)", n)
	}
	// il tempo d'inattività non è più stretto del tempo massimo che sostituisce (rilievo del secondo passaggio, RQ-F1): un
	// server che, ricevuto il corpo, risponde entro TEMPO_MASSIMO deve arrivare in fondo come prima
	inatt := regexp.MustCompile(`(?m)^const INATTIVITA = (\d+);`).FindStringSubmatch(src)
	massimo := regexp.MustCompile(`(?m)^const TEMPO_MASSIMO = (\d+);`).FindStringSubmatch(src)
	if inatt == nil || massimo == nil {
		t.Fatal("il ponte non dichiara INATTIVITA e TEMPO_MASSIMO")
	}
	msInatt, _ := strconv.Atoi(inatt[1])
	msMassimo, _ := strconv.Atoi(massimo[1])
	if msInatt < msMassimo {
		t.Errorf("INATTIVITA = %d ms, più corto del tempo massimo delle chiamate JSON (%d ms) che prima valeva anche per il caricamento", msInatt, msMassimo)
	}
}

// duFunzione: il testo di una funzione di primo livello del ponte, dalla sua riga alla graffa che la chiude a capo riga
// («\n}»).
func duFunzione(t *testing.T, src, inizio string) string {
	t.Helper()
	i := strings.Index(src, "\n"+inizio)
	if i < 0 {
		t.Fatalf("la funzione %q non c'è", inizio)
	}
	j := strings.Index(src[i+1:], "\n}")
	if j < 0 {
		t.Fatalf("la funzione %q non si chiude", inizio)
	}
	return src[i+1 : i+1+j+2]
}

// Le azioni che V1 non ha restano visibili e spente con il motivo scritto accanto (MAT §6.1; D23:294), e la pagina ha
// le code per risorsa per gli atti, i gesti dei file e R3 (MAT §1.3).
func TestDistintaUnicaAzioniNonDisponibiliECode(t *testing.T) {
	src := duFile(t, duPagina)
	for _, v := range []string{"Conferma il documento", "Conferma 2D", "Correggi il codice di un file", `"Deroga"`, `"Revoca la deroga"`, "Sostituisci il principale con questo",
		"Chiedi una revisione autorizzata", "Rivedi e conferma fascicolo", "Rifiuta la candidata", "Accetta o rifiuta"} {
		if !strings.Contains(src, v) {
			t.Errorf("la pagina non mostra l'azione non disponibile %s (MAT §6.1)", v)
		}
	}
	for _, v := range []string{`inSerie("atto"`, `inSerie("file:" + f.allegato_id`, `inSerie("bozza:" + prodotto`, `inSerie("f8"`, `inSerie("carica"`} {
		if !strings.Contains(src, v) {
			t.Errorf("la pagina non passa dalla coda della risorsa: manca %s (MAT §1.3)", v)
		}
	}
}

// Le decisioni dell'ondata A nella pagina (ondata B; il comportamento lo provano le DU-B nel browser, qui che i pezzi ci
// siano): PE-21 (il nome «Accetta le proposte (N)»), PE-22 (nodo_escluso per le preassociazioni di un registrato
// escluso), PE-27 (da_decisione negli svincoli dell'esclusione, niente convenzione .svN nei gesti nuovi), PE-23 (il segno
// uguale_alla_base di R4 prima del giornale, la partenza «registrato» non offerta con una conferma formato 3), P-7
// (cambia_tipo con tipo_nuovo e la firma dalla variante di R1), le modifiche tenute da parte con prodotto e bozza (§5
// n. 2: la scelta prima del cambio di prodotto, beforeunload).
func TestDistintaUnicaDecisioniOndataA(t *testing.T) {
	src := duFile(t, duPagina)
	codice := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(src, "")
	for _, v := range []string{
		`"Accetta le proposte (" + n + ")"`,
		`motivo: "nodo_escluso"`,
		`da_decisione: dec.id`,
		`typeof r.uguale_alla_base === "boolean"`,
		`["conferma", "registrato"].includes(ultimo.dopo.partenza)`,
		`haConfermaFormato3() ? PARTENZA_REGISTRATO_NO`,
		`tipo: "cambia_tipo", componente_id: v.nodo.componente_id,`,
		`tipo_nuovo: t, firma_effetto: st.effetto.firma_effetto`,
		`P.leggiBozza(S.prodotto, { effetto_cambio_tipo: v.nodo.componente_id, tipo_nuovo: t })`,
		`if (S.daParte && S.daParte.prodotto === S.prodotto) return chiediPrimaDiCambiare(id);`,
		`if (S.coda.length || S.invio || S.daParte) {`,
		`if (dp.prodotto !== S.prodotto)`,
	} {
		if !strings.Contains(codice, v) {
			t.Errorf("la pagina non ha %s", v)
		}
	}
	for _, v := range []string{"proposte valide", `".sv" + (`} {
		if strings.Contains(codice, v) {
			t.Errorf("la pagina ha ancora %q", v)
		}
	}
	if escludi := duFunzione(t, src, "function apriEscludi("); strings.Contains(escludi, `"nodo_tolto"`) || !strings.Contains(escludi, `"nodo_escluso"`) {
		t.Error("«Escludi» manda le preassociazioni nel vassoio con un motivo diverso da nodo_escluso (PE-22)")
	}
	// cambia_tipo non porta il tipo nuovo in «tipo», che è la chiave del tipo della decisione (P-7)
	if regexp.MustCompile(`tipo: "cambia_tipo"[^}]*\btipo: t\b`).MatchString(codice) {
		t.Error("cambia_tipo porta il tipo nuovo in «tipo» invece che in tipo_nuovo (P-7)")
	}
	// la chiave_bozza degli eventi delle decisioni è quella della decisione, o null per i tipi senza il campo (EM §9.2;
	// RPC PD-08): mai la chiave di un nodo scritta a mano accanto all'evento
	if !strings.Contains(codice, "function chiaveEvento(dec) {") {
		t.Error("la pagina non ha chiaveEvento")
	}
	if m := regexp.MustCompile(`azione: "decisione_(aggiunta|tolta)", chiave_bozza: (v\.k|k|dec\.chiave_bozza \|\| null)[,\s]`).FindString(codice); m != "" {
		t.Errorf("un evento di una decisione con la chiave del nodo scritta a mano: %q (chiaveEvento)", m)
	}
	// PE-05 (EM §7.1): annulla e rifai di un evento di un gesto cumulativo portano l'azione_id del gesto nel «prima»
	for _, f := range []string{"function annulla(", "function ripeti("} {
		if corpo := duFunzione(t, src, f); !strings.Contains(corpo, "const id = idGesto(voce);") || !strings.Contains(corpo, "prima: id ? { azione_id: id } : null") {
			t.Errorf("%s non porta l'azione_id del gesto cumulativo nel «prima» (PE-05)", f)
		}
	}
	// secondo passaggio, rilievo 1 (DECA §5 n. 1): la regola del giornale di WZ-01, con lo storico (R7) che non si legge,
	// spegne il CTA come le due partenze (PE-B03), dopo il segno di R4 e prima di guardare l'ultima conferma
	nm := duFunzione(t, src, "function nessunaModifica(")
	iR4, iR7, iUltima := strings.Index(nm, "typeof r.uguale_alla_base"), strings.Index(nm, "if (storicoNonLetto()) return STORICO_NON_LETTO;"), strings.Index(nm, "const u = ultimaConferma();")
	if iR4 < 0 || iR7 < 0 || iUltima < 0 || !(iR4 < iR7 && iR7 < iUltima) {
		t.Error("nessunaModifica non spegne il CTA con lo storico non letto, dopo R4 e prima dell'ultima conferma (DECA §5 n. 1)")
	}
	if !strings.Contains(duFunzione(t, src, "function perchéNoPartenza("), "if (storicoNonLetto()) return STORICO_NON_LETTO;") {
		t.Error("le partenze non usano la stessa regola dello storico non letto (PE-B03)")
	}
	// rilievo 4 (P-18): «Correggi» e «Fissa l'emittente» guardano la rev del nodo come sarà dopo il gesto
	for _, f := range []string{"function apriCorreggi(", "function apriFissaEmittente("} {
		if !strings.Contains(duFunzione(t, src, f), "revMinuscoleAmmesse(") {
			t.Errorf("%s non controlla le minuscole della rev con revMinuscoleAmmesse (P-18)", f)
		}
	}
	// rilievo 5 (PE-B05): il ponte legge l'effetto della variante di R1 solo nella forma attesa
	ponte := duFile(t, duPonte)
	if el := duFunzione(t, ponte, "function effettoLeggibile("); !strings.Contains(el, `typeof e.spento !== "string"`) ||
		!strings.Contains(el, `["sospende", "con_lei", "resta_sospesa", "avvisi"].every((k) => Array.isArray(e[k]))`) {
		t.Error("effettoLeggibile non richiede spento stringa e gli elenchi dell'effetto")
	}
	if !strings.Contains(duFunzione(t, ponte, "export async function leggiBozza("), "&& effettoLeggibile(j.effetto)") {
		t.Error("la variante di R1 non controlla la forma dell'effetto (effettoLeggibile)")
	}
}

// I casi di contratto condivisi (e2e/distinta_unica_casi.json): li esegue la prova nel browser con le funzioni della
// pagina (DU-B33) e li può leggere la L1 del server per l'altra metà di T-16. Qui la loro forma: ogni caso ha un nome,
// gli eventi o le azioni e l'atteso; i verbi sono quelli del vocabolario 2 (EM §9.2); le azioni del giornale hanno n.
// In più le forme complete della tabella di EM §9.2, che il server rilegge (richieste RIC-I1 e RIC-I2 della corsia
// contratto, ondata B): nodi e archi completi (EM §3.4, §3.5), ogni azione_id UUID v4 (EM §7.2), l'origine di ogni
// azione, il motivo di rebase e di sorgente_rifiutata, la chiave_bozza degli eventi delle decisioni dedotta dalla
// decisione (null per i tipi senza il campo: RPC PD-08, opzione a); e i casi che EM chiede: nodo_escluso nel vassoio
// (PE-22), annulla e rifai di un gesto cumulativo con prima = {azione_id} (PE-05), scollega_documento con da_decisione
// che cita l'esclusione dello stesso componente (PE-27), sorgente_rifiutata.
func TestDistintaUnicaCasiCondivisi(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("e2e", "distinta_unica_casi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var casi struct {
		Vassoio []struct {
			Nome   string           `json:"nome"`
			Eventi []map[string]any `json:"eventi"`
			Atteso []map[string]any `json:"atteso"`
		} `json:"vassoio"`
		Applica []struct {
			Nome     string           `json:"nome"`
			Snapshot map[string]any   `json:"snapshot"`
			Azioni   []map[string]any `json:"azioni"`
			Atteso   map[string]any   `json:"atteso"`
		} `json:"applica"`
	}
	if err := json.Unmarshal(raw, &casi); err != nil {
		t.Fatalf("i casi condivisi non sono JSON: %v", err)
	}
	if len(casi.Vassoio) < 5 || len(casi.Applica) < 5 {
		t.Fatalf("casi: %d del vassoio e %d delle azioni, attesi almeno 5 e 5", len(casi.Vassoio), len(casi.Applica))
	}
	verbi := map[string]bool{"nodo_aggiunto": true, "nodo_tolto": true, "nodo_modificato": true, "arco_aggiunto": true, "arco_tolto": true, "arco_spostato": true,
		"qta_modificata": true, "decisione_aggiunta": true, "decisione_tolta": true, "file_preassociato": true, "file_svincolato": true, "file_lasciato": true,
		"rebase": true, "annulla": true, "rifai": true, "sorgente_rifiutata": true}
	uuid4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	lista := func(v any) []any {
		l, _ := v.([]any)
		return l
	}
	// ogni azione_id, dovunque stia (dopo, prima, decisioni): UUID v4 (EM §7.2)
	var guardaID func(nome string, v any)
	guardaID = func(nome string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				if s, _ := y.(string); k == "azione_id" && !uuid4.MatchString(s) {
					t.Errorf("%q: azione_id che non è un UUID v4: %v", nome, y)
				}
				guardaID(nome, y)
			}
		case []any:
			for _, y := range x {
				guardaID(nome, y)
			}
		}
	}
	annullaGesto, rifaiGesto := false, false
	for _, c := range casi.Vassoio {
		if c.Nome == "" || len(c.Eventi) == 0 || c.Atteso == nil {
			t.Errorf("caso del vassoio incompleto: %q", c.Nome)
		}
		for _, e := range c.Eventi {
			if _, ok := e["n"].(float64); !ok || !verbi[fmt.Sprint(e["azione"])] {
				t.Errorf("%q: evento senza n o con un verbo fuori dal vocabolario 2: %v", c.Nome, e)
			}
			guardaID(c.Nome, e)
			if p, _ := e["prima"].(map[string]any); p != nil && p["azione_id"] != nil {
				annullaGesto = annullaGesto || e["azione"] == "annulla"
				rifaiGesto = rifaiGesto || e["azione"] == "rifai"
			}
		}
	}
	if !annullaGesto || !rifaiGesto {
		t.Error("manca un caso del vassoio con annulla e rifai di un gesto cumulativo, con prima = {azione_id} (PE-05)")
	}
	// T-16 (PM23, recepimento dell'ondata A): lo svincolo con il motivo nodo_escluso entra nel vassoio (DECA PE-22)
	escluso := false
	for _, c := range casi.Vassoio {
		for _, x := range c.Atteso {
			escluso = escluso || x["motivo"] == "nodo_escluso"
		}
	}
	if !escluso {
		t.Error("nessun caso del vassoio con il motivo nodo_escluso (PE-22, T-16)")
	}
	// i nodi e gli archi completi degli snapshot (EM §3.4, §3.5)
	guardaSnapshot := func(nome string, s map[string]any) {
		for _, x := range lista(s["nodi"]) {
			n, _ := x.(map[string]any)
			for _, k := range []string{"chiave_bozza", "op", "tipo", "origine"} {
				if v, _ := n[k].(string); v == "" {
					t.Errorf("%q: un nodo senza %s (EM §3.4): %v", nome, k, n)
				}
			}
			if id, _ := n["identita"].(map[string]any); id == nil || id["modo"] == nil || id["emittente"] == nil {
				t.Errorf("%q: un nodo senza l'identità con emittente e modo (EM §3.4.2): %v", nome, n)
			}
			if _, conComponente := n["componente_id"].(string); (n["op"] == "crea") == conComponente {
				t.Errorf("%q: op e componente_id non coerenti (EM §3.4): %v", nome, n)
			}
			if n["origine"] == "step" && len(lista(n["caselle"])) == 0 {
				t.Errorf("%q: un nodo con origine step senza la sua casella (EM §3.4): %v", nome, n)
			}
		}
		for _, x := range lista(s["archi"]) {
			a, _ := x.(map[string]any)
			q, _ := a["qta"].(float64)
			if a["padre"] == nil || a["figlio"] == nil || q < 1 || a["origine"] == nil {
				t.Errorf("%q: un arco senza padre, figlio, qta o origine (EM §3.5): %v", nome, a)
			}
			if _, conStep := a["step"].(map[string]any); (a["origine"] == "step") != conStep {
				t.Errorf("%q: step c'è se e solo se l'origine è step (EM §3.5): %v", nome, a)
			}
		}
	}
	daDecisione, rifiutata := false, false
	for _, c := range casi.Applica {
		if c.Nome == "" || c.Snapshot == nil || len(c.Azioni) == 0 || c.Atteso == nil {
			t.Errorf("caso delle azioni incompleto: %q", c.Nome)
		}
		guardaSnapshot(c.Nome, c.Snapshot)
		guardaSnapshot(c.Nome, c.Atteso)
		for _, a := range c.Azioni {
			if !verbi[fmt.Sprint(a["azione"])] {
				t.Errorf("%q: verbo fuori dal vocabolario 2: %v", c.Nome, a["azione"])
			}
			if o, _ := a["origine"].(string); o == "" {
				t.Errorf("%q: un'azione senza origine: %v", c.Nome, a)
			}
			guardaID(c.Nome, a)
			switch a["azione"] {
			case "rebase", "sorgente_rifiutata":
				if m, _ := a["motivo"].(string); m == "" {
					t.Errorf("%q: %v senza il motivo, che è obbligatorio (EM §9.2)", c.Nome, a["azione"])
				}
				rifiutata = rifiutata || a["azione"] == "sorgente_rifiutata"
			case "decisione_aggiunta", "decisione_tolta":
				d, _ := a["dopo"].(map[string]any)
				if a["azione"] == "decisione_tolta" {
					d, _ = a["prima"].(map[string]any)
				}
				if dk := d["chiave_bozza"]; a["chiave_bozza"] != nil && a["chiave_bozza"] != dk {
					t.Errorf("%q: la chiave_bozza dell'evento della decisione non è quella della decisione, né null (EM §9.2; RPC PD-08): %v", c.Nome, a)
				}
			}
		}
		// PE-27: uno svincolo con da_decisione cita un'esclusione dello stesso componente, nello stesso snapshot (RPC PD-04)
		esclusioni := map[string]string{}
		for _, x := range lista(c.Atteso["decisioni"]) {
			if d, _ := x.(map[string]any); d["tipo"] == "escludi" {
				esclusioni[fmt.Sprint(d["id"])] = fmt.Sprint(d["componente_id"])
			}
		}
		for _, x := range lista(c.Atteso["decisioni"]) {
			d, _ := x.(map[string]any)
			if d["tipo"] != "scollega_documento" || d["da_decisione"] == nil {
				continue
			}
			if comp, ok := esclusioni[fmt.Sprint(d["da_decisione"])]; !ok || comp != fmt.Sprint(d["componente_id"]) {
				t.Errorf("%q: da_decisione che non cita l'esclusione dello stesso componente (PE-27): %v", c.Nome, d)
			}
			daDecisione = true
		}
	}
	if !daDecisione {
		t.Error("nessun caso delle azioni con scollega_documento e da_decisione (PE-27)")
	}
	if !rifiutata {
		t.Error("nessun caso delle azioni con sorgente_rifiutata")
	}
}

// I cataloghi del ponte contro i codici di EM, rotta per rotta (la parte di T-05 che si fa senza l'elenco unico del
// server): i 409 che ogni funzione legge come conflitto e i 422 che legge come rifiuto.
func TestPonteDistintaUnicaCataloghi(t *testing.T) {
	blocco := duBlocco(t, duFile(t, duPonte), "CATALOGHI")
	costanti := map[string][]string{}
	elenco := func(s string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`"([A-Z_]+)"`).FindAllStringSubmatch(s, -1) {
			out = append(out, m[1])
		}
		return out
	}
	for _, m := range regexp.MustCompile(`(?m)^const (RIFIUTI_R2|RIFIUTI_FILE|OCCUPATO) = (\[[^\]]*\]);`).FindAllStringSubmatch(strings.ReplaceAll(blocco, "\n  ", " "), -1) {
		costanti[m[1]] = elenco(m[2])
	}
	leggi := func(nome string) map[string][]string {
		i := strings.Index(blocco, "const "+nome+" = {")
		if i < 0 {
			t.Fatalf("il blocco CATALOGHI non ha %s", nome)
		}
		j := strings.Index(blocco[i:], "\n};")
		out := map[string][]string{}
		for _, m := range regexp.MustCompile(`(?m)^\s*(\w+): (.+?),?$`).FindAllStringSubmatch(blocco[i:i+j], -1) {
			v := m[2]
			switch {
			case strings.HasPrefix(v, "["):
				out[m[1]] = elenco(v)
			case strings.Contains(v, ".concat("):
				base := strings.TrimSpace(v[:strings.Index(v, ".concat(")])
				out[m[1]] = append(append([]string{}, costanti[base]...), elenco(v[strings.Index(v, ".concat("):])...)
			default:
				out[m[1]] = costanti[strings.TrimSpace(v)]
			}
		}
		return out
	}
	conflitti, rifiuti := leggi("CONFLITTI"), leggi("RIFIUTI")
	r2c := []string{"BOZZA_VERSIONE", "BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"}
	r2r := []string{"CORPO_NON_VALIDO", "FUORI_RFQ", "BOZZA_ALTRO_PRODOTTO", "FILE_NON_ATTIVO", "MOTIVO_OBBLIGATORIO", "PROPOSTA_NON_DISPONIBILE", "IMPORTAZIONE_TROPPO_GRANDE"}
	attesiC := map[string][]string{ // EM §9.2, §9.3, §9.5, §10.9
		"salvaBozza": r2c, "creaBozza": r2c, "cambiaSorgente": r2c, "riallineaProposta": r2c, "convertiBozza": r2c,
		"abbandonaBozza": {"BOZZA_VERSIONE"},
		"confermaBom":    {"BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA", "BOZZA_VERSIONE", "COPIA_IN_CORSO"},
		"scartaFile":     {"FILE_VERSIONE", "CHIAVE_RIUSATA", "ANTEPRIMA_SUPERATA"},
		"altriFile":      {"FILE_VERSIONE", "CHIAVE_RIUSATA", "ANTEPRIMA_SUPERATA", "COPIA_IN_CORSO"},
		"riattivaFile":   {"FILE_VERSIONE", "CHIAVE_RIUSATA", "ANTEPRIMA_SUPERATA"},
		"associaStep":    {"ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"},
	}
	file := []string{"CORPO_NON_VALIDO", "FUORI_RFQ", "NON_GESTITO", "RFQ_UNITA", "DIPENDENZE_ATTIVE", "RISOLUZIONE_RICHIESTA"}
	attesiR := map[string][]string{ // EM §9.1-§9.7, §10.9
		"leggiBozza": {"FUORI_RFQ", "CORPO_NON_VALIDO"},
		"salvaBozza": r2r, "creaBozza": r2r, "cambiaSorgente": r2r, "riallineaProposta": r2r, "convertiBozza": r2r,
		"abbandonaBozza": {"FUORI_RFQ", "CORPO_NON_VALIDO"},
		"riepilogoBom":   {"CORPO_NON_VALIDO", "FUORI_RFQ"},
		// NESSUNA_MODIFICA: decisioni dell'ondata A, PE-23 (R5 rifiuta una bozza uguale alla conferma di base formato 3)
		"confermaBom":     {"BLOCCHI", "CONGELATA", "NESSUNA_MODIFICA", "CORPO_NON_VALIDO", "FUORI_RFQ"},
		"esitoIntento":    {"FUORI_RFQ"},
		"storicoConferme": {"FUORI_RFQ", "CORPO_NON_VALIDO"},
		"carica":          {"NESSUN_FILE_CARICATO", "CORPO_NON_VALIDO"},
		"leggiSorgente":   {"FUORI_RFQ", "CORPO_NON_VALIDO"},
		"scartaFile":      append(append([]string{}, file...), "MOTIVO_OBBLIGATORIO"),
		"altriFile":       append(append([]string{}, file...), "FILE_SENZA_BYTE", "FILE_SCARTATO", "CONSERVAZIONE_NON_RIUSCITA"),
		"riattivaFile":    {"FILE_SCARTATO", "FUORI_RFQ", "CORPO_NON_VALIDO", "RFQ_UNITA", "CONSERVAZIONE_MANCANTE"},
		"anteprimaStep":   {"FUORI_RFQ", "NON_STEP", "FILE_NON_ATTIVO", "CORPO_NON_VALIDO"},
		"associaStep":     {"MOTIVO_OBBLIGATORIO", "NON_STEP", "GIA_DOCUMENTO", "FILE_NON_ATTIVO", "FUORI_RFQ", "RFQ_UNITA", "CORPO_NON_VALIDO"},
	}
	ordina := func(x []string) string {
		y := append([]string{}, x...)
		sort.Strings(y)
		return strings.Join(y, ",")
	}
	for nome, atteso := range attesiC {
		if ordina(conflitti[nome]) != ordina(atteso) {
			t.Errorf("CONFLITTI di %s: %v, attesi %v", nome, conflitti[nome], atteso)
		}
	}
	for nome, atteso := range attesiR {
		if ordina(rifiuti[nome]) != ordina(atteso) {
			t.Errorf("RIFIUTI di %s: %v, attesi %v", nome, rifiuti[nome], atteso)
		}
	}
	if ordina(costanti["OCCUPATO"]) != ordina([]string{"CALCOLO_OCCUPATO", "NON_CONFIGURATO", "CALCOLO_SCADUTO"}) {
		t.Errorf("OCCUPATO: %v", costanti["OCCUPATO"])
	}
}

// Il foglio della pagina vale solo dentro la pagina: ogni selettore comincia con .du (la pagina) o .du- (le finestre
// fuori dalla pagina), anche dentro @media; nessun indirizzo esterno nei quattro file, nessun carattere o immagine da
// fuori nel foglio.
func TestDistintaUnicaCSSDentroLaPaginaENessunURLEsterno(t *testing.T) {
	css := duFile(t, duCSS)
	sel := duSelettori(t, css)
	re := regexp.MustCompile(`^\.du([\s.:#\[>+~-]|$)`)
	for _, s := range sel {
		if !re.MatchString(s) {
			t.Errorf("selettore %q fuori dalla pagina", s)
		}
	}
	if len(sel) < 80 {
		t.Errorf("%d selettori letti: la lettura del foglio non ha funzionato", len(sel))
	}
	for _, f := range []string{duPonte, duPagina, duCSS, duTemplate} {
		src := duFile(t, f)
		for _, v := range []string{"http://", "https://"} {
			if strings.Contains(src, v) && !(f == duPagina && strings.Count(src, v) == strings.Count(src, `"http://www.w3.org/2000/svg"`)) {
				t.Errorf("%s contiene %q", f, v)
			}
		}
	}
	for _, v := range []string{"url(", "@import", "@font-face"} {
		if strings.Contains(css, v) {
			t.Errorf("il foglio contiene %q", v)
		}
	}
}

// duSelettori: i selettori delle regole di un foglio di stile, anche dentro @media e @supports (commenti tolti, graffe
// contate, virgole fuori dalle parentesi).
func duSelettori(t *testing.T, css string) []string {
	t.Helper()
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	var out []string
	var giro func(s string)
	giro = func(s string) {
		i := 0
		for i < len(s) {
			j := strings.IndexAny(s[i:], "{;}")
			if j < 0 {
				return
			}
			j += i
			prelude := strings.TrimSpace(s[i:j])
			if s[j] != '{' {
				i = j + 1
				continue
			}
			d, k := 0, j
			for ; k < len(s); k++ {
				if s[k] == '{' {
					d++
				} else if s[k] == '}' {
					d--
					if d == 0 {
						break
					}
				}
			}
			if k >= len(s) {
				t.Fatalf("una graffa non chiusa dopo %q", prelude)
			}
			corpo := s[j+1 : k]
			switch {
			case regexp.MustCompile(`^@(media|supports)\b`).MatchString(prelude):
				giro(corpo)
			case strings.HasPrefix(prelude, "@"):
				t.Errorf("at-rule %q: il foglio della pagina non ne ha bisogno", prelude)
			default:
				prof, cur := 0, ""
				for _, ch := range prelude {
					switch ch {
					case '(':
						prof++
					case ')':
						prof--
					}
					if ch == ',' && prof == 0 {
						out = append(out, strings.Join(strings.Fields(cur), " "))
						cur = ""
						continue
					}
					cur += string(ch)
				}
				out = append(out, strings.Join(strings.Fields(cur), " "))
			}
			i = k + 1
		}
	}
	giro(css)
	return out
}
