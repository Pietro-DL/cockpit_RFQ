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
	if strings.Contains(pagina, "import(") {
		t.Error("la pagina ha un import dinamico")
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
	if !strings.Contains(duFile(t, duPagina), "await P.riprovaSospeso(s.risorsa)") {
		t.Error("la pagina non usa riprovaSospeso per «Riprova» dei gesti dei file")
	}
}

// I casi di contratto condivisi (e2e/distinta_unica_casi.json): li esegue la prova nel browser con le funzioni della
// pagina (DU-B30) e li può leggere la L1 del server per l'altra metà di T-16. Qui la loro forma: ogni caso ha un nome,
// gli eventi o le azioni e l'atteso; i verbi sono quelli del vocabolario 2 (EM §9.2); le azioni del giornale hanno n.
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
		"rebase": true, "annulla": true, "rifai": true}
	for _, c := range casi.Vassoio {
		if c.Nome == "" || len(c.Eventi) == 0 || c.Atteso == nil {
			t.Errorf("caso del vassoio incompleto: %q", c.Nome)
		}
		for _, e := range c.Eventi {
			if _, ok := e["n"].(float64); !ok || !verbi[fmt.Sprint(e["azione"])] {
				t.Errorf("%q: evento senza n o con un verbo fuori dal vocabolario 2: %v", c.Nome, e)
			}
		}
	}
	for _, c := range casi.Applica {
		if c.Nome == "" || c.Snapshot == nil || len(c.Azioni) == 0 || c.Atteso == nil {
			t.Errorf("caso delle azioni incompleto: %q", c.Nome)
		}
		for _, a := range c.Azioni {
			if !verbi[fmt.Sprint(a["azione"])] {
				t.Errorf("%q: verbo fuori dal vocabolario 2: %v", c.Nome, a["azione"])
			}
		}
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
		"abbandonaBozza":  {"FUORI_RFQ", "CORPO_NON_VALIDO"},
		"riepilogoBom":    {"CORPO_NON_VALIDO", "FUORI_RFQ"},
		"confermaBom":     {"BLOCCHI", "CONGELATA", "CORPO_NON_VALIDO", "FUORI_RFQ"},
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
