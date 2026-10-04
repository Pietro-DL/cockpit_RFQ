// L1 — il router router-1 e l'intersezione con i ruoli (A-C04; piano A, 5.4.6; P1 §4.3, §6; R23, R20 a): ogni
// riga della tabella dà la sua funzione e i suoi ruoli ammessi, con le famiglie {prodotto, componente},
// {componente} e {prodotto}, insiemi vuoti compresi; l'origine dell'uso e le categorie della famiglia non
// cambiano niente; le righe dei selettori riservati in A1 si provano su Instrada e RuoliAmmessi.
package motorea

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in
// cui un cliente cambia convenzione. Le famiglie qui sotto (acme-pc, acme-c, acme-p) hanno basi inventate e
// attivano un meccanismo, mai il significato di un cliente. Le prove citano i requisiti (A-C04, R20 a, R23), mai i
// casi degli attesi.

// Le famiglie dei ruoli del 5.7.1 (A-C04).
var (
	ruoliPC = []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente}
	ruoliC  = []grammatica.Ruolo{grammatica.RuoloComponente}
	ruoliP  = []grammatica.Ruolo{grammatica.RuoloProdotto}
)

// rigaAttesa: una riga di router-1 come la scrive il piano (5.4.6).
type rigaAttesa struct {
	numero    int
	selettori []string
	usi       []string // gli usi della riga; nil = la riga vale per ogni uso
	funzione  Funzione
	ruoli     []grammatica.Ruolo
}

// usiDelMessaggio: tutti gli usi che un'unità di oggetto, corpo o storia può avere (5.4.6, Instradamento).
var usiDelMessaggio = []string{UsoPertinente, UsoEscluso, UsoDaValutare, UsoSconosciuto}

// tuttiGliUsi: gli usi di Instradamento più l'uso delle unità fuori da un messaggio.
var tuttiGliUsi = []string{UsoPertinente, UsoEscluso, UsoDaValutare, UsoSconosciuto, UsoNonApplicabile}

// origini: le origini di una selezione, più «nessuna selezione».
var origini = []string{"operatore", "riconoscimento", "scenario", ""}

// tabellaDelPiano: le righe 1-5 e 7-16 del 5.4.6. La riga 6 (celle) non ha un selettore suo: la cella ha il
// selettore e l'uso del segmento in cui la tabella si aggancia, e passa dalle righe 1-5 (la prova con le celle
// vere sta in interpreta_test.go).
var tabellaDelPiano = []rigaAttesa{
	{1, []string{"oggetto", "corpo"}, []string{UsoPertinente}, FunzRichiesta, ruoliP},
	{2, []string{"oggetto", "corpo"}, []string{UsoSconosciuto, UsoDaValutare}, FunzRichiesta, ruoliP},
	{3, []string{"oggetto", "corpo"}, []string{UsoEscluso}, FunzMenzione, nil},
	{4, []string{"storia"}, []string{UsoPertinente}, FunzRichiesta, ruoliP},
	{5, []string{"storia"}, []string{UsoEscluso, UsoDaValutare, UsoSconosciuto, UsoNonApplicabile}, FunzMenzione, nil},
	{7, []string{"nome_file", "voce_archivio"}, nil, FunzIdentitaFile, ruoliPC},
	{8, []string{"cartiglio.codice", "cartiglio.numero_disegno"}, nil, FunzIdentitaFile, ruoliPC},
	{9, []string{"radice_step.id", "radice_step.nome"}, nil, FunzIdentitaFile, ruoliPC},
	{10, []string{"nodo_step.id", "nodo_step.nome"}, nil, FunzStruttura, ruoliC},
	{11, []string{"elenco_pdf.codice"}, nil, FunzStruttura, ruoliC},
	{12, []string{"cartiglio.particolare_simile"}, nil, FunzRelazione, nil},
	{13, []string{"cartiglio.revisione", "cartiglio.materiale", "cartiglio.scala", "cartiglio.titolo",
		"radice_step.revisione", "nodo_step.revisione"}, nil, FunzAttributo, nil},
	{14, []string{"radice_step.descrizione", "nodo_step.descrizione"}, nil, FunzMenzione, nil},
	{15, []string{"elenco_pdf.descrizione", "elenco_pdf.revisione", "elenco_pdf.quantita", "elenco_pdf.posizione"}, nil, FunzAttributo, nil},
	{16, []string{"testo_pdf", "metadati_pdf", "cartiglio.sconosciuto", "elenco_pdf.sconosciuto"}, nil, FunzMenzione, nil},
}

// contestiDelVocabolario: tutti i contesti della foglia (evidenze, vocabolario chiuso).
var contestiDelVocabolario = []evidenze.Contesto{
	evidenze.ContestoOggetto, evidenze.ContestoCorpo, evidenze.ContestoStoria, evidenze.ContestoNomeFile,
	evidenze.ContestoVoceArchivio, evidenze.ContestoCartiglio, evidenze.ContestoElencoPDF, evidenze.ContestoTestoPDF,
	evidenze.ContestoMetadatiPDF, evidenze.ContestoRadiceSTEP, evidenze.ContestoNodoSTEP,
}

// tuttiISelettori: ogni coppia valida contesto/campo della foglia, nella forma testuale.
func tuttiISelettori(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, c := range contestiDelVocabolario {
		v, campi := evidenze.CampiAmmessi(c)
		if v == "" {
			t.Fatalf("contesto %q senza variante", c)
		}
		if len(campi) == 0 {
			out = append(out, string(c))
			continue
		}
		for _, campo := range campi {
			out = append(out, string(c)+"."+campo)
		}
	}
	return out
}

func stessiRuoli(a, b []grammatica.Ruolo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAC04OgniRamoDelRouterDaLaSuaFunzioneEISuoiRuoli(t *testing.T) {
	if VersioneRouter != "router-1" {
		t.Fatalf("VersioneRouter = %q: cambiare la tabella vuol dire cambiare la versione, e riscrivere questa prova", VersioneRouter)
	}

	t.Run("ogni riga, ogni selettore, ogni uso e ogni origine", func(t *testing.T) {
		for _, r := range tabellaDelPiano {
			usi := r.usi
			if usi == nil {
				usi = append(append([]string(nil), tuttiGliUsi...), "")
			}
			for _, s := range r.selettori {
				for _, u := range usi {
					for _, o := range origini {
						f, motivo := Instrada(Instradamento{Selettore: selettore(t, s), Uso: u, Origine: o})
						if f != r.funzione {
							t.Errorf("riga %d, %s, uso %q, origine %q: funzione %q, attesa %q", r.numero, s, u, o, f, r.funzione)
						}
						if !strings.Contains(motivo, "riga "+itoa(r.numero)+":") {
							t.Errorf("riga %d, %s, uso %q: il motivo non dice la riga: %q", r.numero, s, u, motivo)
						}
						// Il motivo è una frase stabile: non cambia con l'origine (v3 §10.2: l'origine resta nella
						// lettura, non nella funzione).
						if _, m2 := Instrada(Instradamento{Selettore: selettore(t, s), Uso: u}); m2 != motivo {
							t.Errorf("riga %d, %s, uso %q: il motivo cambia con l'origine: %q, %q", r.numero, s, u, motivo, m2)
						}
					}
				}
				if got := RuoliAmmessi(r.funzione); !stessiRuoli(got, r.ruoli) {
					t.Errorf("riga %d: RuoliAmmessi(%s) = %v, attesi %v", r.numero, r.funzione, got, r.ruoli)
				}
			}
		}
	})

	t.Run("ogni selettore del vocabolario sta in una sola riga", func(t *testing.T) {
		riga := map[string]int{}
		for _, r := range tabellaDelPiano {
			for _, s := range r.selettori {
				if r.numero <= 5 {
					continue // oggetto, corpo e storia dipendono dall'uso: li prova la sottoprova di sopra
				}
				if n, gia := riga[s]; gia {
					t.Fatalf("la tabella della prova nomina %s nelle righe %d e %d", s, n, r.numero)
				}
				riga[s] = r.numero
			}
		}
		for _, s := range tuttiISelettori(t) {
			switch s {
			case "oggetto", "corpo", "storia":
				continue
			}
			n, ok := riga[s]
			if !ok {
				t.Errorf("il selettore %s non è in nessuna riga del piano: la prova va aggiornata insieme al vocabolario", s)
				continue
			}
			_, motivo := Instrada(Instradamento{Selettore: selettore(t, s), Uso: UsoNonApplicabile})
			if !strings.HasPrefix(motivo, "router-1 riga "+itoa(n)+":") {
				t.Errorf("%s: motivo %q, attesa la riga %d", s, motivo, n)
			}
		}
	})

	t.Run("un uso fuori elenco non promuove a richiesta", func(t *testing.T) {
		// Instradamento.Uso è un elenco chiuso. Un valore che il router non conosce non è una pertinenza: in
		// dubbio nessuna richiesta (principio della provenance, 5.0; R48 A: nessuna promozione automatica).
		for _, s := range []string{"oggetto", "corpo", "storia"} {
			for _, u := range []string{"", "PERTINENTE", "pertinente ", "confermato"} {
				if f, _ := Instrada(Instradamento{Selettore: selettore(t, s), Uso: u, Origine: "scenario"}); f == FunzRichiesta {
					t.Errorf("%s con l'uso %q: richiesta, attesa nessuna promozione", s, u)
				}
			}
		}
		// La storia non diventa richiesta con un uso che non sia «pertinente», nemmeno da un operatore.
		for _, u := range []string{UsoDaValutare, UsoSconosciuto, UsoEscluso, UsoNonApplicabile} {
			if f, _ := Instrada(Instradamento{Selettore: selettore(t, "storia"), Uso: u, Origine: "operatore"}); f != FunzMenzione {
				t.Errorf("storia con l'uso %q: %s, attesa menzione (riga 5)", u, f)
			}
		}
	})

	t.Run("RuoliAmmessi di ogni funzione", func(t *testing.T) {
		attesi := map[Funzione][]grammatica.Ruolo{
			FunzRichiesta: ruoliP, FunzIdentitaFile: ruoliPC, FunzStruttura: ruoliC,
			FunzRelazione: nil, FunzAttributo: nil, FunzMenzione: nil, Funzione("sconosciuta"): nil,
		}
		for _, f := range []Funzione{FunzRichiesta, FunzIdentitaFile, FunzStruttura, FunzRelazione, FunzAttributo, FunzMenzione, "sconosciuta"} {
			if got := RuoliAmmessi(f); !stessiRuoli(got, attesi[f]) {
				t.Errorf("RuoliAmmessi(%q) = %v, attesi %v", f, got, attesi[f])
			}
		}
		// L'elenco è nuovo a ogni chiamata: chi lo cambia non cambia il router.
		r := RuoliAmmessi(FunzIdentitaFile)
		r[0] = grammatica.RuoloComponente
		if got := RuoliAmmessi(FunzIdentitaFile); !stessiRuoli(got, ruoliPC) {
			t.Fatalf("cambiare il risultato di RuoliAmmessi cambia il router: %v", got)
		}
	})

	t.Run("l'intersezione con le tre famiglie, insiemi vuoti compresi", func(t *testing.T) {
		funzioni := []Funzione{FunzRichiesta, FunzIdentitaFile, FunzStruttura, FunzRelazione, FunzAttributo, FunzMenzione}
		attesi := map[string][]grammatica.Ruolo{
			"pc/richiesta": ruoliP, "pc/identita_file": ruoliPC, "pc/struttura": ruoliC,
			"c/richiesta": nil, "c/identita_file": ruoliC, "c/struttura": ruoliC,
			"p/richiesta": ruoliP, "p/identita_file": ruoliP, "p/struttura": nil,
		}
		famiglie := []struct {
			nome  string
			ruoli []grammatica.Ruolo
		}{{"pc", ruoliPC}, {"c", ruoliC}, {"p", ruoliP}}
		for _, fam := range famiglie {
			for _, f := range funzioni {
				got, motivo := intersecaRuoli(fam.ruoli, f)
				atteso := attesi[fam.nome+"/"+string(f)]
				if !stessiRuoli(got, atteso) {
					t.Errorf("famiglia %s ∩ %s = %v, attesi %v", fam.nome, f, got, atteso)
				}
				if !strings.Contains(motivo, string(f)) {
					t.Errorf("famiglia %s ∩ %s: il motivo non nomina la funzione: %q", fam.nome, f, motivo)
				}
				if len(atteso) == 0 && !strings.Contains(motivo, "∅") {
					t.Errorf("famiglia %s ∩ %s: l'insieme vuoto non si vede nel motivo: %q", fam.nome, f, motivo)
				}
				// L'ordine dei ruoli nella famiglia non conta.
				if rov, _ := intersecaRuoli(rovescia(fam.ruoli), f); !stessiRuoli(rov, got) {
					t.Errorf("famiglia %s con i ruoli in un altro ordine ∩ %s = %v, prima %v", fam.nome, f, rov, got)
				}
			}
		}
		// Il motivo per esteso, con l'esempio del piano (5.4.6 punto 5).
		if _, motivo := intersecaRuoli(ruoliPC, FunzStruttura); motivo != "famiglia {prodotto, componente} ∩ struttura {componente} = {componente}" {
			t.Errorf("motivo %q, atteso quello del piano", motivo)
		}
	})

	t.Run("con il riconoscimento: le categorie non toccano l'intersezione", func(t *testing.T) {
		// Tre famiglie ACME con forme su tutti i selettori attivi in A1 che hanno una riga del router. La famiglia
		// {componente} dichiara la minuteria: è un'annotazione, che non cambia i ruoli (v3 §2; R7).
		m, _ := compilaBene(t, grammaticaRouter())
		ruoliDi := map[string][]grammatica.Ruolo{}
		categorieDi := map[string][]grammatica.Categoria{}
		for _, f := range m.Snapshot().Grammatica.Famiglie {
			ruoliDi[f.ID] = f.Ruoli
			categorieDi[f.ID] = f.Categorie
		}
		casi := []struct {
			sel, uso string
			funzione Funzione
		}{
			{"oggetto", UsoPertinente, FunzRichiesta},
			{"corpo", UsoSconosciuto, FunzRichiesta},
			{"corpo", UsoEscluso, FunzMenzione},
			{"storia", UsoPertinente, FunzRichiesta},
			{"storia", UsoSconosciuto, FunzMenzione},
			{"nome_file", UsoNonApplicabile, FunzIdentitaFile},
			{"cartiglio.codice", UsoNonApplicabile, FunzIdentitaFile},
			{"radice_step.id", UsoNonApplicabile, FunzIdentitaFile},
			{"nodo_step.id", UsoNonApplicabile, FunzStruttura},
			{"testo_pdf", UsoNonApplicabile, FunzMenzione},
		}
		for _, c := range casi {
			letture, _ := riconosci(t, m, c.sel, testoRouter)
			if len(letture) != 3 {
				t.Fatalf("%s: %d letture, attese le tre famiglie:\n%s", c.sel, len(letture), riassunto(letture))
			}
			f, _ := Instrada(Instradamento{Selettore: selettore(t, c.sel), Uso: c.uso, Origine: "operatore"})
			if f != c.funzione {
				t.Fatalf("%s con uso %s: funzione %s, attesa %s", c.sel, c.uso, f, c.funzione)
			}
			for _, l := range letture {
				ruoli, _ := intersecaRuoli(ruoliDi[l.Famiglia], f)
				if l.Famiglia == "acme-c" {
					if len(l.Categorie) != 1 || l.Categorie[0] != grammatica.CategoriaMinuteria {
						t.Errorf("%s: la lettura della famiglia con la minuteria ha le categorie %v", c.sel, l.Categorie)
					}
					// minuteria e {componente}: la richiesta non lo ammette, la struttura sì.
					switch f {
					case FunzRichiesta, FunzMenzione:
						if len(ruoli) != 0 {
							t.Errorf("%s: {componente} con la minuteria ∩ %s = %v, atteso ∅", c.sel, f, ruoli)
						}
					case FunzStruttura, FunzIdentitaFile:
						if !stessiRuoli(ruoli, ruoliC) {
							t.Errorf("%s: {componente} con la minuteria ∩ %s = %v, atteso {componente}", c.sel, f, ruoli)
						}
					}
				}
			}
		}
		if len(categorieDi["acme-pc"]) != 0 || len(categorieDi["acme-p"]) != 0 {
			t.Fatalf("solo acme-c dichiara una categoria: %v", categorieDi)
		}
	})

	t.Run("le righe dei selettori riservati, su Instrada e RuoliAmmessi", func(t *testing.T) {
		// R20 (c): su questi selettori nessuna grammatica ha forme attive, nemmeno sintetica; il router le ha lo
		// stesso, e si provano qui (R20 a).
		casi := []struct {
			sel      string
			riga     int
			funzione Funzione
			ruoli    []grammatica.Ruolo
		}{
			{"voce_archivio", 7, FunzIdentitaFile, ruoliPC},
			{"cartiglio.numero_disegno", 8, FunzIdentitaFile, ruoliPC},
			{"radice_step.nome", 9, FunzIdentitaFile, ruoliPC},
			{"nodo_step.nome", 10, FunzStruttura, ruoliC},
			{"elenco_pdf.codice", 11, FunzStruttura, ruoliC},
			{"cartiglio.particolare_simile", 12, FunzRelazione, nil},
			{"cartiglio.titolo", 13, FunzAttributo, nil},
			{"nodo_step.revisione", 13, FunzAttributo, nil},
			{"radice_step.descrizione", 14, FunzMenzione, nil},
			{"elenco_pdf.quantita", 15, FunzAttributo, nil},
			{"metadati_pdf", 16, FunzMenzione, nil},
			{"elenco_pdf.sconosciuto", 16, FunzMenzione, nil},
		}
		for _, c := range casi {
			if grammatica.SelettoreAttivo(c.sel) {
				t.Errorf("%s: la prova lo considera riservato, la grammatica lo dice attivo", c.sel)
			}
			f, motivo := Instrada(Instradamento{Selettore: selettore(t, c.sel), Uso: UsoNonApplicabile})
			if f != c.funzione || !strings.HasPrefix(motivo, "router-1 riga "+itoa(c.riga)+":") {
				t.Errorf("%s: %s %q, attesa %s dalla riga %d", c.sel, f, motivo, c.funzione, c.riga)
			}
			if got := RuoliAmmessi(f); !stessiRuoli(got, c.ruoli) {
				t.Errorf("%s: ruoli ammessi %v, attesi %v", c.sel, got, c.ruoli)
			}
		}
	})

	t.Run("stessi ingressi, stessa funzione", func(t *testing.T) {
		for _, s := range tuttiISelettori(t) {
			for _, u := range tuttiGliUsi {
				i := Instradamento{Selettore: selettore(t, s), Uso: u, Origine: "scenario"}
				f1, m1 := Instrada(i)
				f2, m2 := Instrada(i)
				if f1 != f2 || m1 != m2 || m1 == "" {
					t.Fatalf("%s %s: %s %q, poi %s %q", s, u, f1, m1, f2, m2)
				}
			}
		}
	})
}

// itoa: un intero piccolo in decimale, senza strconv per una cifra o due.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// testoRouter: un testo con un codice di ciascuna delle tre famiglie di grammaticaRouter, separati da spazi.
const testoRouter = "ACMEPC1001 ACMEC1002 ACMEP1003"

// grammaticaRouter: tre famiglie ACME, una per insieme di ruoli del 5.7.1, con una forma su ogni selettore attivo
// in A1 che ha una riga del router. Gli esempi stanno sul corpo, uno per famiglia.
func grammaticaRouter() grammatica.Grammatica {
	selettori := sel("oggetto", "corpo", "storia", "nome_file", "cartiglio.codice", "radice_step.id", "nodo_step.id", "testo_pdf")
	famiglia := func(id, prefisso string, ruoli []grammatica.Ruolo, categorie []grammatica.Categoria, esempio string) grammatica.FamigliaCodice {
		return grammatica.FamigliaCodice{
			ID: id, Namespace: id, Ruoli: ruoli, Categorie: categorie,
			Base:  base(segLetterale("prefisso", prefisso, ""), segPattern("numero", "[0-9]{4}", "")),
			Forme: []grammatica.FormaCodice{forma("ovunque", selettori, pBase())},
			Esempi: []grammatica.EsempioCodice{
				positivo("e-"+id, "corpo", esempio, grammatica.LetturaAttesa{Forma: "ovunque", Base: esempio}),
			},
		}
	}
	return grammaticaACME(
		famiglia("acme-pc", "ACMEPC", ruoliPC, nil, "ACMEPC1001"),
		famiglia("acme-c", "ACMEC", ruoliC, []grammatica.Categoria{grammatica.CategoriaMinuteria}, "ACMEC1002"),
		famiglia("acme-p", "ACMEP", ruoliP, nil, "ACMEP1003"),
	)
}
