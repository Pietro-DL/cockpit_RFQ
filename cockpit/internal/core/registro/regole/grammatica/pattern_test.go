package grammatica

import (
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — i vincoli RE2 dei pattern (A1a-PAT; v3 §4.3; parte 1 §5.1; T1-T12, T16, T20): ogni divieto di
// VerificaPattern dà il suo codice grammatica.* e solo quello; la lunghezza e le ripetizioni seguono i
// limiti ricevuti dall'indice (R43 B); il «\b» scritto nel JSON è un backspace, e il messaggio lo dice.
//
// I clienti di questi test sono inventati: qui non ce n'è nessuno, i pattern sono pezzi qualunque.

func codiciDi(ds []evidenze.Diagnostica) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Codice)
	}
	sort.Strings(out)
	return out
}

func TestA1aPATDivieti(t *testing.T) {
	casi := []struct {
		nome, pattern string
		codici        []string
	}{
		{"lookahead", `(?=5)[0-9]`, []string{CodicePatternOperatoreNonRE2}},
		{"lookahead negativo", `(?!5)[0-9]`, []string{CodicePatternOperatoreNonRE2}},
		{"lookbehind", `(?<=5)[0-9]`, []string{CodicePatternOperatoreNonRE2}},
		{"lookbehind negativo", `(?<!5)[0-9]`, []string{CodicePatternOperatoreNonRE2}},
		{"backreference", `([0-9])\1`, []string{CodicePatternOperatoreNonRE2}},
		{"backreference con nome", `(?P<n>[0-9])\k<n>`, []string{CodicePatternOperatoreNonRE2}},
		{"gruppo atomico", `(?>[0-9])`, []string{CodicePatternOperatoreNonRE2}},
		{"possessivo", `[0-9]++`, []string{CodicePatternOperatoreNonRE2}},
		{"possessivo su una ripetizione limitata", `[0-9]{2}+`, []string{CodicePatternOperatoreNonRE2}},
		{"confine \\b", `[0-9]{3}\b`, []string{CodicePatternConfine}},
		{"confine \\B", `\B[0-9]`, []string{CodicePatternConfine}},
		{"ancora ^", `^[0-9]`, []string{CodicePatternAncora}},
		{"ancora $", `[0-9]$`, []string{CodicePatternAncora}},
		{"ancora \\A", `\A[0-9]`, []string{CodicePatternAncora}},
		{"ancora \\z", `[0-9]\z`, []string{CodicePatternAncora}},
		{"gruppo", `([0-9])`, []string{CodicePatternGruppo}},
		{"gruppo non catturante", `(?:[0-9])`, []string{CodicePatternGruppo}},
		{"gruppo con nome", `(?P<n>[0-9])`, []string{CodicePatternGruppo}},
		{"flag in linea", `(?i)a[0-9]`, []string{CodicePatternFlag}},
		{"gruppo con flag", `(?i:a)[0-9]`, []string{CodicePatternFlag, CodicePatternGruppo}},
		{"punto", `.[0-9]`, []string{CodicePatternPunto}},
		{"classe negata", `[^0-9]`, []string{CodicePatternPunto}},
		{"classe negata \\D", `\D`, []string{CodicePatternPunto}},
		{"classe Unicode", `\pL[0-9]`, []string{CodicePatternClasseNonASCII}},
		{"classe con lettere accentate", `[à-ù]`, []string{CodicePatternClasseNonASCII}},
		{"alternanza", `A|B`, []string{CodicePatternAlternanza}},
		{"alternanza di parole", `AB|CD`, []string{CodicePatternAlternanza}},
		{"stella", `A[0-9]*`, []string{CodicePatternRipetizioneIllimitata}},
		{"più", `A[0-9]+`, []string{CodicePatternRipetizioneIllimitata}},
		{"{n,}", `A[0-9]{2,}`, []string{CodicePatternRipetizioneIllimitata}},
		{"{n,m} oltre il limite", `[0-9]{1,41}`, []string{CodicePatternRipetizioneOltreLimite}},
		{"{n} oltre il limite", `[0-9]{41}`, []string{CodicePatternRipetizioneOltreLimite}},
		{"oltre il massimo di regexp/syntax", `[0-9]{1001}`, []string{CodicePatternRipetizioneOltreLimite}},
		{"facoltativo da solo", `[0-9]?`, []string{CodicePatternVuoto}},
		{"{0,n} da solo", `[0-9]{0,3}`, []string{CodicePatternVuoto}},
		{"pattern vuoto", ``, []string{CodicePatternVuoto}},
		{"non compila", `[0-9`, []string{CodicePatternNonCompila}},
		{"spazio", `[0-9] [0-9]`, []string{CodiceCarattereDiControllo}},
		{"spazio unificatore", "[0-9] ", []string{CodiceCarattereDiControllo}},
		{"carattere di formato", "[0-9]​", []string{CodiceCarattereDiControllo}},
		{"tabulazione", "[0-9]\t", []string{CodiceCarattereDiControllo}},
		{"backspace", "[0-9]{3}\b", []string{CodiceCarattereDiControllo}},
		{"tabulazione scritta con l'escape", `[0-9]\t`, []string{CodiceCarattereDiControllo}},
		{"spazio scritto con l'escape", `\x20[0-9]`, []string{CodiceCarattereDiControllo}},
		{"spazi con \\s", `\s[0-9]`, []string{CodiceCarattereDiControllo}},
		{"controlli in una classe", `[\x00-\x1f0-9]`, []string{CodiceCarattereDiControllo}},
		{"spazio unificatore scritto con l'escape", `[0-9]\x{a0}`, []string{CodiceCarattereDiControllo}},
		{"carattere di formato scritto con l'escape", `[0-9]\x{feff}`, []string{CodiceCarattereDiControllo}},
		{"troppo lungo", strings.Repeat("A", 65), []string{CodiceLimiteSuperato}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ds := VerificaPattern("p", c.pattern, limitiACME())
			got := codiciDi(ds)
			want := append([]string(nil), c.codici...)
			sort.Strings(want)
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Fatalf("%q: codici %v, attesi %v%s", c.pattern, got, want, elenca(ds))
			}
			for _, d := range ds {
				if d.Percorso != "p" || d.Gravita != evidenze.GravitaErrore || d.Messaggio == "" {
					t.Fatalf("diagnostica incompleta:%s", elenca(ds))
				}
				natura := evidenze.NaturaContratto
				if d.Codice == CodiceLimiteSuperato {
					natura = evidenze.NaturaLimite
				}
				if d.Natura != natura {
					t.Fatalf("%s con natura %s, attesa %s", d.Codice, d.Natura, natura)
				}
			}
		})
	}
}

// TestA1aPATAmmessi: i pezzi che una grammatica scrive davvero passano; «(» e «|» dentro una classe o dentro
// \Q…\E sono caratteri qualunque, non un gruppo o un'alternanza.
func TestA1aPATAmmessi(t *testing.T) {
	for _, p := range []string{
		`[0-9]{7}`, `X[0-9A-Z]{3}`, `[A-Z]?[0-9]`, `\d{2}`, `[0-9]{1,40}`, `[0-9]{40}`,
		`[(|][0-9]`, `\Q(|\E[0-9]`, `[[:digit:]]{2}`, `[\]][0-9]`, `è[0-9]`, strings.Repeat("A", 64),
	} {
		if ds := VerificaPattern("p", p, limitiACME()); ds != nil {
			t.Errorf("%q rifiutato:%s", p, elenca(ds))
		}
	}
}

// TestA1aPATLimitiDallIndice: lunghezza e ripetizione seguono i limiti ricevuti, non una costante del codice
// (R43 B).
func TestA1aPATLimitiDallIndice(t *testing.T) {
	lim := limitiACME()
	lim.Grammatica.MaxRipetizione = 5
	lim.Grammatica.MaxLunghezzaPattern = 8
	richiedi(t, VerificaPattern("p", `[0-9]{6}`, lim), CodicePatternRipetizioneOltreLimite, "p")
	richiedi(t, VerificaPattern("p", `[0-9]{2}AAAA`, lim), CodiceLimiteSuperato, "p")
	if ds := VerificaPattern("p", `[0-9]{5}`, lim); ds != nil {
		t.Fatalf("[0-9]{5} con max_ripetizione 5:%s", elenca(ds))
	}
}

// TestA1aPATBackspaceDalJSON: un «\b» scritto nel JSON della grammatica arriva come backspace; la
// validazione lo rifiuta nel pattern e nel letterale, e il messaggio suggerisce «\\b» (T20).
func TestA1aPATBackspaceDalJSON(t *testing.T) {
	ds := diagnosiDi(t, variante(t, `"pattern": "7[0-9]{6}"`, `"pattern": "7[0-9]{6}\b"`), limitiACME())
	d := richiedi(t, ds, CodiceCarattereDiControllo, "famiglie_codice[acme-etichetta].base.segmenti[numero].pattern")
	if !strings.Contains(d.Messaggio, `"\\b"`) {
		t.Fatalf("il messaggio non suggerisce \\\\b: %s", d.Messaggio)
	}
	ds = diagnosiDi(t, variante(t, `"letterali": ["PN"]`, `"letterali": ["PN\b"]`), limitiACME())
	d = richiedi(t, ds, CodiceCarattereDiControllo, "famiglie_codice[acme-etichetta].etichette[pn].letterali[0]")
	if !strings.Contains(d.Messaggio, "backspace") {
		t.Fatalf("il messaggio non dice backspace: %s", d.Messaggio)
	}
}

// TestA1aPATSpazioNeiLetterali: lo spazio U+0020 è ammesso in un letterale (i separatori lo usano), mai nel
// pattern.
func TestA1aPATSpazioNeiLetterali(t *testing.T) {
	ds := diagnosiDi(t, variante(t, `"letterali": ["PN"]`, `"letterali": ["PN ", "P N"]`), limitiACME())
	if e := errori(ds); len(e) > 0 {
		t.Fatalf("lo spazio ASCII in un letterale è ammesso:%s", elenca(e))
	}
}

// TestA1aPATTuttiIPatternDellaGrammatica: Valida verifica i pattern dei segmenti della base, delle revisioni,
// dei token sospesi e delle decorazioni, ciascuno con il suo percorso.
func TestA1aPATTuttiIPatternDellaGrammatica(t *testing.T) {
	casi := []struct{ da, a, percorso string }{
		{`"pattern": "5[0-9]{6}"`, `"pattern": "5[0-9]+"`, "famiglie_codice[acme-prefisso].base.segmenti[numero].pattern"},
		{`{"nome": "forte", "pattern": "[0-9]"`, `{"nome": "forte", "pattern": "[0-9]*"`, "famiglie_codice[acme-punti].revisioni[rev].segmenti[forte].pattern"},
		{`"pattern": "xx"`, `"pattern": "x+"`, "famiglie_codice[acme-punti].revisioni[rev].token_sospesi[xx].pattern"},
		{`{"id": "stato", "tipo": "stato_pdm", "letterali": ["IN_WORK"]`, `{"id": "stato", "tipo": "stato_pdm", "pattern": "IN_[A-Z]+"`, "famiglie_codice[acme-documento].decorazioni[stato].pattern"},
	}
	for _, c := range casi {
		ds := diagnosiDi(t, variante(t, c.da, c.a), limitiACME())
		richiedi(t, ds, CodicePatternRipetizioneIllimitata, c.percorso)
	}
}
