// L1 — dall'adattatore a Interpreta, sulle fixture sintetiche del QA (piano A, 5.7.1 e 5.7.2: A-C02, A-C03 e UTF8,
// A-C05 e MAIL-INOLTRO, A-C09, A-C10, A-C12, D1, HASH-CONFLITTO, A1b-19, A1b-23, A1b-25 nella parte di Interpreta).
// Ogni sottoprova «nessun legame» è un tentativo di far nascere un collegamento senza provenance, o con la
// provenance ma senza la regola che lo giustifica (5.0, principio dell'utente del 04/10): se il prodotto collega,
// è un difetto.
package estrazione_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in
// cui un cliente cambia convenzione. Le fixture sono quelle sintetiche di testdata/ingressi (ACME, acme.example,
// fornitore.example, codici di fantasia come ACME7000100, PB07XX0001, 9999999A, CORDONE_ID_0001, l'intestazione
// inventata «Q.TA»); le grammatiche G-ACME sono scritte qui in linea, ripetute di proposito rispetto alle prove di
// motorea (5.7.2). Le prove citano i requisiti (A-C02, A1b-19, R48 A), mai i casi degli attesi.

// ---- gli ingressi (letti qui: la prova vive fuori dal pacchetto, e gli aiuti interni non si vedono) ----

type ingressoFile struct {
	Allegato    fotorfq.Allegato  `json:"allegato"`
	Contenitore *fotorfq.Allegato `json:"contenitore"`
	Fatti       *fotorfq.Fatti    `json:"fatti"`
}

type ingressoMail struct {
	Messaggio fotorfq.Messaggio `json:"messaggio"`
}

func leggiStretto(t *testing.T, tipo, nome string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "ingressi", tipo, nome+".json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%s: ingresso non valido: %v", nome, err)
	}
}

// ingressoAllegato: l'ingresso di un file, con il digest dei fatti calcolato come lo calcola chi riempie un Fatti.
func ingressoAllegato(t *testing.T, tipo, nome string) ingressoFile {
	t.Helper()
	var in ingressoFile
	leggiStretto(t, tipo, nome, &in)
	if in.Fatti != nil {
		d, err := fotorfq.ImprontaPayload(in.Fatti.Payload)
		if err != nil {
			t.Fatal(err)
		}
		in.Fatti.Digest = d
	}
	return in
}

func docFile(t *testing.T, tipo, nome string) evidenze.DocumentoEvidenze {
	t.Helper()
	in := ingressoAllegato(t, tipo, nome)
	d, err := estrazione.DaAllegato(in.Allegato, in.Fatti, in.Contenitore)
	if err != nil {
		t.Fatalf("%s: %v", nome, err)
	}
	return d
}

func messaggio(t *testing.T, nome string) fotorfq.Messaggio {
	t.Helper()
	var in ingressoMail
	leggiStretto(t, "email", nome, &in)
	return in.Messaggio
}

func docMail(t *testing.T, m fotorfq.Messaggio) evidenze.DocumentoEvidenze {
	t.Helper()
	d, err := estrazione.DaMessaggio(m)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// variante: il messaggio con delle sostituzioni a coppie nel corpo di testo e nell'HTML; ogni vecchio deve
// comparire una volta sola nel suo campo, così la variante cambia proprio ciò che dice.
func variante(t *testing.T, m fotorfq.Messaggio, testo, html []string) fotorfq.Messaggio {
	t.Helper()
	sostituisci := func(campo string, s *string, coppie []string) *string {
		v := *s
		for i := 0; i+1 < len(coppie); i += 2 {
			if n := strings.Count(v, coppie[i]); n != 1 {
				t.Fatalf("variante del %s: %q compare %d volte", campo, coppie[i], n)
			}
			v = strings.Replace(v, coppie[i], coppie[i+1], 1)
		}
		return &v
	}
	m.CorpoTesto = sostituisci("testo", m.CorpoTesto, testo)
	m.CorpoHTML = sostituisci("HTML", m.CorpoHTML, html)
	return m
}

// ---- le grammatiche G-ACME ----

func limitiGACME() grammatica.Limiti {
	return grammatica.Limiti{
		Versione: "limiti-acme-1",
		Grammatica: grammatica.LimitiGrammatica{
			MaxFamiglie: 20, MaxFormePerFamiglia: 16, MaxPartiPerForma: 12, MaxEsempi: 64,
			MaxLunghezzaPattern: 64, MaxRipetizione: 40, MaxLunghezzaLetterale: 40,
		},
		Riconoscimento: grammatica.LimitiRiconoscimento{
			MaxByteUnita: 1048576, MaxUnitaDocumento: 10000, MaxLetturePerUnita: 1000, MaxLettureDocumento: 20000,
		},
		Anteprima: grammatica.LimitiAnteprima{TempoMassimoMs: 20000},
	}
}

func parteBase() grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}
}

func baseACME(prefisso, numero string) grammatica.Base {
	return grammatica.Base{
		Segmenti: []grammatica.SegmentoBase{
			{Nome: "prefisso", Letterale: prefisso, Identitario: true},
			{Nome: "numero", Pattern: numero, Identitario: true},
		},
		Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
}

func formaACME(id string, selettori []string, parti ...grammatica.Parte) grammatica.FormaCodice {
	return grammatica.FormaCodice{ID: id, Selettori: selettori, Stato: grammatica.StatoAttiva, Completa: true, Parti: parti}
}

func esempioACME(id, selettore, testo string, letture ...grammatica.LetturaAttesa) grammatica.EsempioCodice {
	return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: selettore, Testo: testo,
		Atteso: grammatica.AttesoEsempio{Letture: letture}}
}

var (
	ruoliPC = []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente}
	ruoliP  = []grammatica.Ruolo{grammatica.RuoloProdotto}
)

// famMail: i codici «ACME» con quattro cifre, su oggetto, corpo e storia (R48 A: anche sulla storia).
func famMail() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-mail", Namespace: "acme-mail", Ruoli: ruoliPC, Base: baseACME("ACME", "[0-9]{4}"),
		Forme:  []grammatica.FormaCodice{formaACME("mail", []string{"oggetto", "corpo", "storia"}, parteBase())},
		Esempi: []grammatica.EsempioCodice{esempioACME("e-mail", "corpo", "ACME1111", grammatica.LetturaAttesa{Forma: "mail", Base: "ACME1111"})},
	}
}

// famDisegno: i codici «ACME7» con sei cifre, sui file (nome, cartiglio, STEP, frammenti), con la revisione in
// campo separato a due cifre sul cartiglio.
func famDisegno() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-disegno", Namespace: "acme-disegno", Ruoli: ruoliPC, Base: baseACME("ACME7", "[0-9]{6}"),
		Forme: []grammatica.FormaCodice{formaACME("file",
			[]string{"nome_file", "cartiglio.codice", "radice_step.id", "nodo_step.id", "testo_pdf"}, parteBase())},
		Revisioni: []grammatica.RegolaRevisione{{
			ID: "rev-campo", Selettori: []string{"cartiglio.revisione"}, Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteCampoSeparato,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]{2}", Significato: grammatica.SignificatoNessuno}},
		}},
		Esempi: []grammatica.EsempioCodice{
			esempioACME("e-nome", "nome_file", "ACME7000100.pdf", grammatica.LetturaAttesa{Forma: "file", Base: "ACME7000100"}),
			esempioACME("e-revisione", "cartiglio.revisione", "02", grammatica.LetturaAttesa{Revisione: "02"}),
		},
	}
}

// famPB: i codici «PB07XX» con quattro cifre della tabella della mail, ruoli {prodotto}.
func famPB() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-pb", Namespace: "acme-pb", Ruoli: ruoliP, Base: baseACME("PB07XX", "[0-9]{4}"),
		Forme:  []grammatica.FormaCodice{formaACME("tabella", []string{"corpo", "storia"}, parteBase())},
		Esempi: []grammatica.EsempioCodice{esempioACME("e-pb", "corpo", "PB07XX0001", grammatica.LetturaAttesa{Forma: "tabella", Base: "PB07XX0001"})},
	}
}

// quantitaACME: la colonna della quantità con il letterale inventato «Q.TA» (R28 a; 5.6.2: nessun letterale di un
// cliente), sui selettori e con lo stato dati.
func quantitaACME(stato string, intestazioni []string, selettori ...string) grammatica.QuantitaTabellare {
	return grammatica.QuantitaTabellare{ID: "q-acme", Intestazioni: intestazioni, Regola: grammatica.RegolaQuantitaPrimaRigaSopra,
		Selettori: selettori, Stato: stato}
}

func grammaticaGACME(quantita []grammatica.QuantitaTabellare, famiglie ...grammatica.FamigliaCodice) grammatica.Grammatica {
	return grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: uuid.MustParse("00000000-0000-4000-8000-00000000ac01"), RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       famiglie,
		Quantita:       quantita,
	}
}

// motore: la grammatica passa dalla porta del prodotto (file v1, NuovoSnapshot, CompilaVerificato).
func motore(t *testing.T, g grammatica.Grammatica) *motorea.Motore {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	s, err := grammatica.NuovoSnapshot(raw, limitiGACME())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	m, d, err := motorea.CompilaVerificato(s, limitiGACME())
	if err != nil || m == nil {
		t.Fatalf("la grammatica non compila: %v %+v", err, d)
	}
	return m
}

func motoreGACME(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, grammaticaGACME([]grammatica.QuantitaTabellare{quantitaACME(grammatica.StatoAttiva, []string{"Q.TA"}, "corpo", "storia")},
		famMail(), famDisegno(), famPB()))
}

// ---- l'interpretazione e i suoi invarianti ----

func usoSconosciuto(d evidenze.DocumentoEvidenze) evidenze.UsoSegmenti {
	return evidenze.UsoSconosciuto(d.BundleID)
}

func usoSelezionato(d evidenze.DocumentoEvidenze, segmento, uso, origine string) evidenze.UsoSegmenti {
	return evidenze.UsoSegmenti{BundleID: d.BundleID, Versione: 1, Stato: "valutato",
		Selezioni: []evidenze.SelezioneSegmento{{SegmentoID: segmento, Uso: uso, Origine: origine}}}
}

// interpretaDoc chiama Interpreta e controlla gli invarianti di ogni risultato: letture ricostruite sull'unità e
// sull'originale (A-C03), trasformazioni che ricostruiscono le parti, attributi della loro entità e mai di
// un'unità senza entità, letture compatibili della stessa entità (A-C02), ruoli dentro quelli della funzione.
func interpretaDoc(t *testing.T, m *motorea.Motore, d evidenze.DocumentoEvidenze, uso evidenze.UsoSegmenti) motorea.Interpretazione {
	t.Helper()
	r, err := m.Interpreta(d, uso)
	if err != nil {
		t.Fatalf("Interpreta: %v", err)
	}
	unita := map[string]evidenze.UnitaEvidenza{}
	for _, u := range d.Unita {
		unita[u.ID] = u
	}
	testi := map[string]string{}
	for _, x := range d.Testi {
		testi[x.ID] = x.Testo
	}
	letture := map[string]motorea.LetturaCodice{}
	for _, l := range r.Letture {
		letture[l.ID] = l
		u, ok := unita[l.UnitaID]
		o := l.Occorrenza
		if !ok || o.Inizio < 0 || o.Fine > len(u.Testo) || u.Testo[o.Inizio:o.Fine] != l.Forma.Originale {
			t.Errorf("lettura %s non ricostruisce %q sulla sua unità", l.ID, l.Forma.Originale)
			continue
		}
		if a := l.Assoluto; a != nil {
			orig := testi[a.TestoID]
			if a.Intervallo.Inizio < 0 || a.Intervallo.Fine > len(orig) || orig[a.Intervallo.Inizio:a.Intervallo.Fine] != l.Forma.Originale {
				t.Errorf("lettura %s: l'assoluto %+v non ricostruisce l'originale (A-C03)", l.ID, *a)
			}
		}
		for _, tr := range l.Trasformazioni {
			iv := tr.Intervalli[0]
			if iv.Inizio < 0 || iv.Fine > len(u.Testo) || u.Testo[iv.Inizio:iv.Fine] != tr.Prima {
				t.Errorf("lettura %s: la trasformazione %s non ricostruisce %q", l.ID, tr.Operazione, tr.Prima)
			}
		}
		ammessi := motorea.RuoliAmmessi(l.Funzione)
		for _, ruolo := range l.RuoliCandidati {
			trovato := false
			for _, a := range ammessi {
				trovato = trovato || a == ruolo
			}
			if !trovato {
				t.Errorf("lettura %s: ruolo %s fuori da quelli di %s", l.ID, ruolo, l.Funzione)
			}
		}
	}
	for _, a := range r.Attributi {
		u, ok := unita[a.UnitaID]
		if !ok || a.EntitaID == "" || a.EntitaID != u.EntitaID || a.Grezzo != u.Testo {
			t.Errorf("attributo %s: entità %q, unità %+v", a.ID, a.EntitaID, u)
			continue
		}
		for _, lid := range a.LettureCompatibili {
			if l, ok := letture[lid]; !ok || unita[l.UnitaID].EntitaID != a.EntitaID {
				t.Errorf("attributo %s dell'entità %s legato alla lettura %s di un'altra entità", a.ID, a.EntitaID, lid)
			}
		}
	}
	for _, x := range r.Diagnostiche {
		if x.Gravita == evidenze.GravitaErrore {
			t.Errorf("errore dentro un'interpretazione: %+v", x)
		}
	}
	return r
}

func lettureDi(r motorea.Interpretazione, unita string) []motorea.LetturaCodice {
	var out []motorea.LetturaCodice
	for _, l := range r.Letture {
		if l.UnitaID == unita {
			out = append(out, l)
		}
	}
	return out
}

func attributi(r motorea.Interpretazione, tipo string) []motorea.AttributoLetto {
	var out []motorea.AttributoLetto
	for _, a := range r.Attributi {
		if a.Tipo == tipo {
			out = append(out, a)
		}
	}
	return out
}

func conCodice(d []evidenze.Diagnostica, codice string) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, x := range d {
		if x.Codice == codice {
			out = append(out, x)
		}
	}
	return out
}

func ids(l []motorea.LetturaCodice) []string {
	var out []string
	for _, x := range l {
		out = append(out, x.ID+"="+x.Forma.Originale)
	}
	return out
}

func entitaDi(d evidenze.DocumentoEvidenze, unita string) string {
	for _, u := range d.Unita {
		if u.ID == unita {
			return u.EntitaID
		}
	}
	return ""
}

// ---- A-C02 ----

// TestAC02UnAttributoStaConLaSuaEntita (A-C02; P1 §10.5): la revisione del cartiglio va al disegno, e la «Rev 03»
// di un frammento non si lega al disegno; le formazioni STEP restano ciascuna al suo nodo, e un nodo senza
// formazione non ne riceve una da un altro.
func TestAC02UnAttributoStaConLaSuaEntita(t *testing.T) {
	m := motoreGACME(t)

	t.Run("PDF: la revisione del cartiglio sì, quella del frammento no", func(t *testing.T) {
		d := docFile(t, "pdf", "pdf_01_cartiglio")
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		rev := attributi(r, motorea.AttributoRevisione)
		if len(rev) != 1 {
			t.Fatalf("attesa una revisione, del cartiglio: %+v", rev)
		}
		a := rev[0]
		if a.UnitaID != "u:pdf:cartiglio:revisione:1" || a.EntitaID != "e:pdf:p1" || a.Stato != motorea.StatoAttribuito || a.Normalizzato != "02" {
			t.Fatalf("revisione del cartiglio: %+v", a)
		}
		var attese []string
		for _, l := range r.Letture {
			if entitaDi(d, l.UnitaID) == "e:pdf:p1" {
				attese = append(attese, l.ID)
			}
		}
		if len(attese) == 0 || !reflect.DeepEqual(a.LettureCompatibili, attese) {
			t.Errorf("letture compatibili %v, attese quelle del disegno %v", a.LettureCompatibili, attese)
		}
		for _, x := range r.Attributi {
			if strings.HasPrefix(x.UnitaID, "u:pdf:frammento") || strings.Contains(x.Grezzo, "03") || x.Normalizzato == "03" {
				t.Errorf("la «Rev» di un frammento è diventata un attributo: %+v", x)
			}
		}
		// Titolo, scala e materiale: il solo grezzo, legato al disegno.
		for _, tipo := range []string{motorea.AttributoTitolo, motorea.AttributoScala, motorea.AttributoMateriale} {
			x := attributi(r, tipo)
			if len(x) != 1 || x[0].EntitaID != "e:pdf:p1" || x[0].Normalizzato != "" || len(x[0].LettureCompatibili) != 0 {
				t.Errorf("%s: %+v", tipo, x)
			}
		}
		// Le letture dei frammenti sono menzioni (riga 16).
		for _, l := range r.Letture {
			if strings.HasPrefix(l.UnitaID, "u:pdf:frammento") && l.Funzione != motorea.FunzMenzione {
				t.Errorf("frammento %s con funzione %s", l.ID, l.Funzione)
			}
		}
	})

	t.Run("STEP: ogni formazione al suo nodo", func(t *testing.T) {
		for _, nome := range []string{"step_01_assieme", "step_09_cordoni"} {
			d := docFile(t, "step", nome)
			r := interpretaDoc(t, m, d, usoSconosciuto(d))
			formazioni := map[string]string{} // entità → grezzo
			for _, u := range d.Unita {
				if u.Selettore.Campo.Valore == "revisione" && u.Selettore.Campo.Variante == evidenze.VarianteStep {
					formazioni[u.EntitaID] = u.Testo
				}
			}
			fa := attributi(r, motorea.AttributoFormazione)
			if len(fa) != len(formazioni) || len(fa) == 0 {
				t.Fatalf("%s: %d formazioni nei fatti, %d attributi: %+v", nome, len(formazioni), len(fa), fa)
			}
			for _, a := range fa {
				if formazioni[a.EntitaID] != a.Grezzo || a.Normalizzato != "" || len(a.LettureCompatibili) != 0 || a.Stato != motorea.StatoAttribuito {
					t.Errorf("%s: formazione %+v; attesa grezza, al suo nodo, senza letture", nome, a)
				}
			}
			if rv := attributi(r, motorea.AttributoRevisione); len(rv) != 0 {
				t.Errorf("%s: una formazione STEP è diventata una revisione: %+v", nome, rv)
			}
			// Un nodo senza formazione (le saldature di F-STEP-9) non ne riceve una.
			for _, a := range fa {
				if strings.HasPrefix(entitaDi(d, a.UnitaID), "e:step:#4") || strings.HasPrefix(entitaDi(d, a.UnitaID), "e:step:#5") {
					t.Errorf("%s: formazione su un nodo che non l'ha: %+v", nome, a)
				}
			}
		}
	})
}

// ---- A-C03 e UTF8 ----

// TestAC03GliIntervalliRicostruisconoLOriginale (A-C03, UTF8; P1 §3.3): per ogni lettura esatta
// originale[Assoluto] == testo[Occorrenza]; le trasformazioni ricostruiscono le parti (in interpretaDoc). I codici
// attaccati si leggono tutti; dopo un'emoji byte, code point Python e unità UTF-16 sono tre numeri diversi.
func TestAC03GliIntervalliRicostruisconoLOriginale(t *testing.T) {
	m := motoreGACME(t)

	t.Run("mail con CRLF, accenti, NBSP, zero-width, emoji e codici attaccati", func(t *testing.T) {
		d := docMail(t, messaggio(t, "mail_01_utf8"))
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		var corpo, oggetto []string
		for _, l := range r.Letture {
			if l.Assoluto == nil {
				t.Errorf("lettura %s di un'unità esatta senza assoluto", l.ID)
			}
			switch l.Forma.Selettore.Contesto {
			case evidenze.ContestoCorpo:
				corpo = append(corpo, l.Forma.Originale)
			case evidenze.ContestoOggetto:
				oggetto = append(oggetto, l.Forma.Originale)
			}
		}
		// Le letture sono in ordine di ID (5.4.6 punto 17), non di posizione: si confrontano come insieme.
		sort.Strings(corpo)
		if strings.Join(corpo, ",") != "ACME1111,ACME1111,ACME2222,ACME2222,ACME3333,ACME4444" {
			t.Errorf("letture del corpo: %v", corpo)
		}
		if strings.Join(oggetto, ",") != "ACME3333" {
			t.Fatalf("letture dell'oggetto: %v", oggetto)
		}
		// Dopo l'emoji dell'oggetto: byte, code point (Python) e unità UTF-16 sono diversi. La conversione verso il
		// browser è di A1d (R35); qui l'intervallo è in byte, su un confine di runa.
		for _, l := range r.Letture {
			if l.Forma.Selettore.Contesto != evidenze.ContestoOggetto {
				continue
			}
			ogg := *messaggio(t, "mail_01_utf8").Oggetto
			byteInizio := l.Assoluto.Intervallo.Inizio
			prima := ogg[:byteInizio]
			puntiPython := utf8.RuneCountInString(prima)
			unitaUTF16 := len(utf16.Encode([]rune(prima)))
			if byteInizio == puntiPython || puntiPython == unitaUTF16 || byteInizio == unitaUTF16 {
				t.Errorf("byte %d, code point %d, UTF-16 %d: attesi tre numeri diversi", byteInizio, puntiPython, unitaUTF16)
			}
			if !utf8.RuneStart(ogg[byteInizio]) {
				t.Error("intervallo a metà di una runa")
			}
		}
	})

	t.Run("STEP e PDF con caratteri fuori dall'ASCII", func(t *testing.T) {
		for _, c := range []struct{ tipo, nome string }{{"step", "step_07_caratteri"}, {"pdf", "pdf_09_caratteri"}} {
			d := docFile(t, c.tipo, c.nome)
			interpretaDoc(t, m, d, usoSconosciuto(d))
		}
	})

	t.Run("nome con accenti NFC e NFD ed emoji", func(t *testing.T) {
		nfd := "e" + string(rune(0x301))
		for _, nome := range []string{"🔧ACME7000100 è.pdf", "ACME7000100 " + nfd + ".pdf", "ACME7000100" + string(rune(0x00a0)) + "🔧.stp"} {
			a := fotorfq.Allegato{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000e1"), MessaggioID: uuid.MustParse("00000000-0000-4000-8000-000000000090"),
				Indice: 1, NomeFile: nome, Natura: "file", Origine: "outlook", RicevutoIl: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}
			d, err := estrazione.DaAllegato(a, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			r := interpretaDoc(t, m, d, usoSconosciuto(d))
			if len(r.Letture) != 1 || r.Letture[0].Assoluto == nil || r.Letture[0].Forma.Originale != "ACME7000100" {
				t.Errorf("%q: %v", nome, ids(r.Letture))
			}
			// Un file con il solo nome ha il contenuto non disponibile: parziale, mai «completa» (5.4.6 punto 16).
			if r.Stato != motorea.StatoInterpretazioneParziale {
				t.Errorf("%q: stato %s di un file senza fatti", nome, r.Stato)
			}
		}
	})
}

// ---- A-C05 e MAIL-INOLTRO ----

// TestAC05SoloIlSegmentoSceltoDiventaRichiesta (A-C05, MAIL-INOLTRO; P1 §4.3; R27 a; R48 A): con l'uso sconosciuto
// nessuna lettura della storia è richiesta; con s:storia:1 pertinente il codice dell'inoltro è richiesta e resta in
// contesto storia, quello della citazione più vecchia resta menzione; scegliendo s:storia:2 succede il contrario.
// L'origine della scelta resta nella lettura. La firma è dichiarata assente, e il codice nella firma dell'inoltro
// segue il suo segmento (rischio dichiarato da R27 a).
func TestAC05SoloIlSegmentoSceltoDiventaRichiesta(t *testing.T) {
	m := motoreGACME(t)
	d := docMail(t, messaggio(t, "mail_03_inoltro"))
	firma := false
	for _, c := range d.Qualita.Capacita {
		if c.Nome == "firma" && c.Stato == "non_disponibile" {
			firma = true
		}
	}
	if !firma {
		t.Fatal("la capacità firma non è dichiarata assente (R27 a)")
	}
	perSegmento := func(r motorea.Interpretazione, unita string) (codici []string, funzioni []motorea.Funzione, origini []string) {
		for _, l := range lettureDi(r, unita) {
			codici = append(codici, l.Forma.Originale)
			funzioni = append(funzioni, l.Funzione)
			origini = append(origini, l.OrigineUso)
		}
		return
	}
	tutte := func(f []motorea.Funzione, attesa motorea.Funzione) bool {
		for _, x := range f {
			if x != attesa {
				return false
			}
		}
		return len(f) > 0
	}

	t.Run("uso sconosciuto: la storia è menzione", func(t *testing.T) {
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		for _, l := range r.Letture {
			if l.Forma.Selettore.Contesto == evidenze.ContestoStoria && l.Funzione != motorea.FunzMenzione {
				t.Errorf("%s: una lettura della storia è %s senza una scelta esplicita", l.ID, l.Funzione)
			}
		}
		c1, _, _ := perSegmento(r, "u:storia:s:storia:1")
		c2, _, _ := perSegmento(r, "u:storia:s:storia:2")
		if !contiene(c1, "ACME1111") || !contiene(c1, "ACME9999") || strings.Join(c2, ",") != "ACME2222" {
			t.Fatalf("letture dei livelli: %v | %v", c1, c2)
		}
	})

	for _, c := range []struct {
		scelto, altro, origine string
	}{{"s:storia:1", "s:storia:2", "operatore"}, {"s:storia:2", "s:storia:1", "scenario"}} {
		t.Run("scelto "+c.scelto+" da "+c.origine, func(t *testing.T) {
			r := interpretaDoc(t, m, d, usoSelezionato(d, c.scelto, "pertinente", c.origine))
			_, f, o := perSegmento(r, "u:storia:"+c.scelto)
			if !tutte(f, motorea.FunzRichiesta) {
				t.Errorf("%s pertinente: funzioni %v, attese richiesta", c.scelto, f)
			}
			for _, x := range o {
				if x != c.origine {
					t.Errorf("origine dell'uso %q, attesa %q visibile nella lettura", x, c.origine)
				}
			}
			for _, l := range lettureDi(r, "u:storia:"+c.scelto) {
				if l.Forma.Selettore.Contesto != evidenze.ContestoStoria {
					t.Errorf("%s: la richiesta dalla storia ha cambiato contesto: %s", l.ID, l.Forma.Selettore.Contesto)
				}
			}
			_, fa, _ := perSegmento(r, "u:storia:"+c.altro)
			if !tutte(fa, motorea.FunzMenzione) {
				t.Errorf("%s non scelto: funzioni %v, attese menzione (il padre o il figlio non ereditano)", c.altro, fa)
			}
		})
	}

	t.Run("una scelta da valutare o esclusa non promuove", func(t *testing.T) {
		for _, uso := range []string{"da_valutare", "escluso"} {
			r := interpretaDoc(t, m, d, usoSelezionato(d, "s:storia:1", uso, "operatore"))
			for _, l := range r.Letture {
				if l.Forma.Selettore.Contesto == evidenze.ContestoStoria && l.Funzione != motorea.FunzMenzione {
					t.Errorf("uso %s: %s è %s", uso, l.ID, l.Funzione)
				}
			}
		}
		// Il corrente pertinente non promuove la storia.
		r := interpretaDoc(t, m, d, usoSelezionato(d, "s:corrente", "pertinente", "operatore"))
		for _, l := range r.Letture {
			if l.Forma.Selettore.Contesto == evidenze.ContestoStoria && l.Funzione != motorea.FunzMenzione {
				t.Errorf("s:corrente pertinente promuove %s della storia", l.ID)
			}
		}
	})
}

func contiene(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// ---- A1b-25 nella parte di Interpreta ----

// TestLInoltroSenzaConfineNonDaRichiesteDaSolo (A1b-25; R48 A; 5.10 n.16): con l'uso sconosciuto i codici della
// tabella di F-MAIL-4 sono menzioni; con s:storia:1 pertinente da scenario sono richiesta in contesto storia. Lo
// stesso corpo senza prefisso d'inoltro (F-MAIL-4b) è corpo corrente: richiesta con la pertinenza ignota.
func TestLInoltroSenzaConfineNonDaRichiesteDaSolo(t *testing.T) {
	m := motoreGACME(t)
	d := docMail(t, messaggio(t, "mail_04_inoltro_tabella"))
	r := interpretaDoc(t, m, d, usoSconosciuto(d))
	if len(r.Letture) != 4 {
		t.Fatalf("attese le quattro letture della tabella: %v", ids(r.Letture))
	}
	for _, l := range r.Letture {
		if l.Funzione != motorea.FunzMenzione || len(l.RuoliCandidati) != 0 {
			t.Errorf("%s: %s %v senza una scelta (nessuna promozione automatica)", l.ID, l.Funzione, l.RuoliCandidati)
		}
	}
	r = interpretaDoc(t, m, d, usoSelezionato(d, "s:storia:1", "pertinente", "scenario"))
	for _, l := range r.Letture {
		if l.Funzione != motorea.FunzRichiesta || l.OrigineUso != "scenario" || l.Forma.Selettore.Contesto != evidenze.ContestoStoria {
			t.Errorf("%s con s:storia:1 pertinente da scenario: %s, origine %q, contesto %s", l.ID, l.Funzione, l.OrigineUso, l.Forma.Selettore.Contesto)
		}
	}

	d4b := docMail(t, messaggio(t, "mail_04b_senza_prefisso"))
	r = interpretaDoc(t, m, d4b, usoSconosciuto(d4b))
	if len(r.Letture) != 4 {
		t.Fatalf("F-MAIL-4b: %v", ids(r.Letture))
	}
	for _, l := range r.Letture {
		if l.Funzione != motorea.FunzRichiesta || l.Uso != motorea.UsoSconosciuto || l.Forma.Selettore.Contesto != evidenze.ContestoCorpo {
			t.Errorf("F-MAIL-4b %s: %s, uso %s", l.ID, l.Funzione, l.Uso)
		}
	}
	if len(conCodice(r.Diagnostiche, motorea.CodiceMotorePertinenzaIgnota)) == 0 {
		t.Error("richieste con la pertinenza ignota senza la nota")
	}
}

// ---- A-C09 ----

// TestAC09UnPDFSenzaTestoNonEUnPDFSenzaCodici (A-C09; P1 §3.4; C-34): la revisione illeggibile è non_interpretabile
// con l'originale; un PDF senza testo, senza testo_pdf o con l'errore è parziale, mai «completo con zero letture»;
// le formazioni restano grezze e alternative.
func TestAC09UnPDFSenzaTestoNonEUnPDFSenzaCodici(t *testing.T) {
	m := motoreGACME(t)

	t.Run("revisione con testo legale", func(t *testing.T) {
		d := docFile(t, "pdf", "pdf_02_revisione_legale")
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		rev := attributi(r, motorea.AttributoRevisione)
		if len(rev) != 1 || rev[0].Stato != motorea.StatoNonInterpretabile || rev[0].Normalizzato != "" ||
			!strings.HasPrefix(rev[0].Grezzo, "Questo disegno") || len(rev[0].LettureCompatibili) != 0 {
			t.Fatalf("revisione illeggibile: %+v", rev)
		}
		if len(conCodice(r.Diagnostiche, motorea.CodiceRevisioneNonInterpretabile)) != 1 {
			t.Errorf("manca revisione.non_interpretabile: %+v", r.Diagnostiche)
		}
	})

	t.Run("senza testo è parziale, diverso da completo con zero letture", func(t *testing.T) {
		zero := motore(t, grammaticaGACME(nil, grammatica.FamigliaCodice{
			ID: "acme-altro", Namespace: "acme-altro", Ruoli: ruoliPC, Base: baseACME("ALTRO", "[0-9]{4}"),
			Forme:  []grammatica.FormaCodice{formaACME("file", []string{"nome_file", "cartiglio.codice", "testo_pdf"}, parteBase())},
			Esempi: []grammatica.EsempioCodice{esempioACME("e", "nome_file", "ALTRO1234.pdf", grammatica.LetturaAttesa{Forma: "file", Base: "ALTRO1234"})},
		}))
		d1 := docFile(t, "pdf", "pdf_01_cartiglio")
		completo := interpretaDoc(t, zero, d1, usoSconosciuto(d1))
		if completo.Stato != motorea.StatoInterpretazioneCompleta || len(completo.Letture) != 0 {
			t.Fatalf("PDF leggibile senza codici: stato %s, %d letture", completo.Stato, len(completo.Letture))
		}
		for _, nome := range []string{"pdf_03_senza_testo", "pdf_04a_senza_testo_pdf", "pdf_04b_errore_pdf"} {
			d := docFile(t, "pdf", nome)
			r := interpretaDoc(t, m, d, usoSconosciuto(d))
			if r.Stato != motorea.StatoInterpretazioneParziale {
				t.Errorf("%s: stato %s, atteso parziale (capacità testo assente, nome leggibile)", nome, r.Stato)
			}
			if r.Stato == completo.Stato {
				t.Errorf("%s: si confonde con un PDF completo senza codici", nome)
			}
		}
		// Il nome del PDF senza testo resta letto.
		d := docFile(t, "pdf", "pdf_03_senza_testo")
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		if len(lettureDi(r, "u:nome")) != 1 {
			t.Errorf("il nome del PDF senza testo non si legge: %v", ids(r.Letture))
		}
	})

	t.Run("formazioni alternative e grezze", func(t *testing.T) {
		d := docFile(t, "step", "step_04_formazioni_alternative")
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		var radice []motorea.AttributoLetto
		for _, a := range attributi(r, motorea.AttributoFormazione) {
			if a.EntitaID == "e:step:#10" {
				radice = append(radice, a)
			}
		}
		if len(radice) < 2 {
			t.Fatalf("le formazioni alternative della radice restano tutte: %+v", radice)
		}
		if len(conCodice(d.Qualita.Diagnostiche, "step.formazioni_alternative")) == 0 {
			t.Error("il documento non dichiara le formazioni alternative")
		}
		d8 := docFile(t, "step", "step_08_formazioni")
		r8 := interpretaDoc(t, m, d8, usoSconosciuto(d8))
		var grezzi []string
		for _, a := range attributi(r8, motorea.AttributoFormazione) {
			grezzi = append(grezzi, a.Grezzo)
			if a.Normalizzato != "" {
				t.Errorf("formazione normalizzata: %+v", a)
			}
		}
		for _, g := range []string{"00.00", "1", "ANY"} {
			if !contiene(grezzi, g) {
				t.Errorf("la formazione %q non è conservata com'è: %v", g, grezzi)
			}
		}
	})
}

// TestLoStatoSegueLeCapacitaDiLetturaSuOgniFixture (5.4.6 punto 16; R32 b; A-C09): su ogni fixture degli
// adattatori lo stato è quello che dicono le sole capacità di lettura (testo, cartiglio, struttura, contenuto) e i
// troncamenti del worker: firma, segmentazione, grafo completo e le altre non contano.
func TestLoStatoSegueLeCapacitaDiLetturaSuOgniFixture(t *testing.T) {
	m := motoreGACME(t)
	file := map[string][]string{
		"step": {"step_01_assieme", "step_02_due_radici", "step_03_figlio_con_due_padri", "step_04_formazioni_alternative",
			"step_05_troncata_con_scarti", "step_06_non_step21", "step_06b_struttura_v1", "step_07_caratteri", "step_08_formazioni",
			"step_09_cordoni", "step_10_v2_senza_scarti"},
		"pdf": {"pdf_01_cartiglio", "pdf_02_revisione_legale", "pdf_03_senza_testo", "pdf_04a_senza_testo_pdf", "pdf_04b_errore_pdf",
			"sottoversione_1", "pdf_06_specchiato", "pdf_07_ocr", "pdf_08_troncato", "pdf_09_caratteri", "pdf_10_metadati", "pdf_11_elenco"},
	}
	var docs []evidenze.DocumentoEvidenze
	var nomi []string
	for _, tipo := range []string{"step", "pdf"} {
		for _, n := range file[tipo] {
			docs = append(docs, docFile(t, tipo, n))
			nomi = append(nomi, n)
		}
	}
	for _, n := range []string{"mail_01_utf8", "mail_02_risposta", "mail_03_inoltro", "mail_04_inoltro_tabella", "mail_04b_senza_prefisso",
		"mail_05a_tab_e_non_combacia", "mail_05b_a_cavallo", "mail_05c_solo_html", "mail_05d_marcato_e_separatori", "mail_06_messaggio_originale"} {
		docs = append(docs, docMail(t, messaggio(t, n)))
		nomi = append(nomi, n)
	}
	for i, d := range docs {
		atteso := motorea.StatoInterpretazioneCompleta
		leggibili := false
		for _, u := range d.Unita {
			leggibili = leggibili || u.Testo != ""
			if u.Qualita.Troncata {
				atteso = motorea.StatoInterpretazioneParziale
			}
		}
		for _, c := range d.Qualita.Capacita {
			if contiene([]string{"testo", "cartiglio", "struttura", "contenuto"}, c.Nome) && c.Stato != "disponibile" {
				atteso = motorea.StatoInterpretazioneParziale
			}
		}
		if !leggibili {
			atteso = motorea.StatoInterpretazioneNonDisponibile
		}
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		if r.Stato != atteso {
			t.Errorf("%s: stato %s, atteso %s dalle capacità di lettura %+v", nomi[i], r.Stato, atteso, d.Qualita.Capacita)
		}
	}
}

// ---- A-C10 ----

// TestAC10LoStessoCodiceInPiuPostiNonSiPerde (A-C10; P1 §7.3): lo stesso codice nel nome, nel cartiglio e in un
// frammento dà tre letture; due righe di tabella con lo stesso codice danno due letture; la cella esatta e il suo
// segmento danno una lettura sola con AltreUnita; le celle per righe di una tabella a TAB si fondono solo con la
// lettura della loro riga.
func TestAC10LoStessoCodiceInPiuPostiNonSiPerde(t *testing.T) {
	m := motoreGACME(t)

	t.Run("nome, cartiglio, frammento; nome e radice", func(t *testing.T) {
		d := docFile(t, "pdf", "pdf_10_metadati")
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		n := 0
		for _, l := range r.Letture {
			if l.Forma.Originale == "ACME7000100" {
				n++
				if len(l.AltreUnita) != 0 {
					t.Errorf("%s fusa per sola stringa con %v", l.ID, l.AltreUnita)
				}
			}
		}
		if n != 3 {
			t.Errorf("attese tre letture di ACME7000100 (nome, cartiglio, frammento): %v", ids(r.Letture))
		}
		ds := docFile(t, "step", "step_01_assieme")
		rs := interpretaDoc(t, m, ds, usoSconosciuto(ds))
		if len(lettureDi(rs, "u:nome")) != 1 || len(lettureDi(rs, "u:step:#10:id")) != 1 {
			t.Errorf("nome e radice con lo stesso codice: %v", ids(rs.Letture))
		}
	})

	t.Run("cella esatta e segmento: una lettura; due righe uguali: due", func(t *testing.T) {
		d := docMail(t, messaggio(t, "mail_04_inoltro_tabella"))
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		for _, l := range r.Letture {
			if !strings.HasPrefix(l.UnitaID, "u:tab:1:r") || !reflect.DeepEqual(l.AltreUnita, []string{"u:storia:s:storia:1"}) {
				t.Errorf("%s: unità %s, AltreUnita %v; attesa la cella con il segmento", l.ID, l.UnitaID, l.AltreUnita)
			}
		}
		v := variante(t, messaggio(t, "mail_04_inoltro_tabella"),
			[]string{"PB07XX0002\r\n", "PB07XX0001\r\n"}, []string{"<td>PB07XX0002</td>", "<td>PB07XX0001</td>"})
		dv := docMail(t, v)
		rv := interpretaDoc(t, m, dv, usoSconosciuto(dv))
		var celle []string
		for _, l := range rv.Letture {
			if l.Forma.Originale == "PB07XX0001" {
				celle = append(celle, l.UnitaID)
			}
		}
		if strings.Join(celle, ",") != "u:tab:1:r3:c1,u:tab:1:r4:c1" {
			t.Errorf("due righe con lo stesso codice: %v", ids(rv.Letture))
		}
	})

	t.Run("tabella a TAB: celle per righe fuse solo con la lettura della loro riga", func(t *testing.T) {
		d := docMail(t, messaggio(t, "mail_05a_tab_e_non_combacia"))
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		if len(r.Letture) != 2 {
			t.Fatalf("attese due letture (una per codice): %v", ids(r.Letture))
		}
		for _, l := range r.Letture {
			if !strings.HasPrefix(l.UnitaID, "u:tab:1:r") || !reflect.DeepEqual(l.AltreUnita, []string{"u:corpo:s:corrente"}) || l.Assoluto != nil {
				t.Errorf("%s: %+v", l.ID, l)
			}
		}
	})

	// Le righe di una cella sono righe del corpo: non valgono sul testo dell'oggetto, che pure sta nel segmento
	// s:corrente (5.4.6 punto 9, secondo caso; 5.0). Con la tabella sulla prima riga del corpo e lo stesso codice
	// nell'oggetto, la cella si fonde con la lettura del corpo, e l'oggetto resta una lettura a sé.
	t.Run("nessun legame: le righe della cella non valgono sull'oggetto", func(t *testing.T) {
		msg := messaggio(t, "mail_05a_tab_e_non_combacia")
		oggetto := "Richiesta ACME1111"
		corpo := "ACME1111\tStaffa\t5\r\nACME2222\tPiastra\t3\r\n\r\nGrazie"
		html := "<html><body>\n<table class=MsoNormalTable>\n<tr><td>ACME1111</td><td>Staffa</td><td>5</td></tr>\n" +
			"<tr><td>ACME2222</td><td>Piastra</td><td>3</td></tr>\n</table>\n<p class=MsoNormal>Grazie</p>\n</body></html>"
		msg.Oggetto, msg.CorpoTesto, msg.CorpoHTML = &oggetto, &corpo, &html
		d := docMail(t, msg)
		cella := false
		for _, u := range d.Unita {
			if u.ID == "u:tab:1:r1:c1" {
				pt := u.Posizione.Tabella
				cella = pt != nil && pt.Esatto == nil && pt.RigheTesto != nil && pt.RigheTesto[0] == 0
			}
		}
		if !cella {
			t.Fatalf("la variante non ha la cella agganciata per righe sulla prima riga del corpo")
		}
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		var trovate []string
		for _, l := range r.Letture {
			if l.Forma.Originale != "ACME1111" {
				continue
			}
			trovate = append(trovate, l.UnitaID)
			switch l.UnitaID {
			case "u:tab:1:r1:c1":
				if !reflect.DeepEqual(l.AltreUnita, []string{"u:corpo:s:corrente"}) {
					t.Errorf("la cella e il corpo sono la stessa occorrenza: AltreUnita %v", l.AltreUnita)
				}
			case "u:oggetto":
				if len(l.AltreUnita) != 0 {
					t.Errorf("l'oggetto fuso con %v per le righe di una cella del corpo", l.AltreUnita)
				}
			}
		}
		sort.Strings(trovate)
		if strings.Join(trovate, ",") != "u:oggetto,u:tab:1:r1:c1" {
			t.Errorf("attese una lettura dell'oggetto e una della cella con il corpo: %v", ids(r.Letture))
		}
	})
}

// ---- A-C12 ----

// TestAC12IFattiFotografatiRifannoLoStessoDocumento (A-C12; P1 §12): due payload alla stessa terna (stesso
// sha256, calcolato_il e contenuto diversi) danno due documenti con digest e BundleID diversi; il documento rifatto
// dal primo payload è identico al primo, byte per byte, e così l'impronta dell'interpretazione.
func TestAC12IFattiFotografatiRifannoLoStessoDocumento(t *testing.T) {
	m := motoreGACME(t)
	in := ingressoAllegato(t, "step", "step_01_assieme")
	f2 := *in.Fatti
	f2.CalcolatoIl = in.Fatti.CalcolatoIl.Add(time.Hour)
	if !bytes.Contains(f2.Payload, []byte("TELAIO ACME")) {
		t.Fatal("la fixture non ha la descrizione attesa")
	}
	f2.Payload = bytes.Replace(append(json.RawMessage(nil), in.Fatti.Payload...), []byte("TELAIO ACME"), []byte("TELAIO ACME RIFATTO"), 1)
	d2digest, err := fotorfq.ImprontaPayload(f2.Payload)
	if err != nil {
		t.Fatal(err)
	}
	f2.Digest = d2digest

	d1, err := estrazione.DaAllegato(in.Allegato, in.Fatti, nil)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := estrazione.DaAllegato(in.Allegato, &f2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d1.Fonte.RiferimentoFatti.Sha256 != d2.Fonte.RiferimentoFatti.Sha256 ||
		d1.Fonte.RiferimentoFatti.DigestPayload == d2.Fonte.RiferimentoFatti.DigestPayload || d1.BundleID == d2.BundleID {
		t.Fatalf("stessa terna, payload diversi: digest %s/%s, bundle %s/%s", d1.Fonte.RiferimentoFatti.DigestPayload,
			d2.Fonte.RiferimentoFatti.DigestPayload, d1.BundleID, d2.BundleID)
	}
	rifatto := ingressoAllegato(t, "step", "step_01_assieme")
	d1bis, err := estrazione.DaAllegato(rifatto.Allegato, rifatto.Fatti, nil)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := jsoncanonico.Codifica(d1)
	b1bis, _ := jsoncanonico.Codifica(d1bis)
	if !bytes.Equal(b1, b1bis) {
		t.Fatal("il documento rifatto dai fatti fotografati non è identico")
	}
	r1, r1bis, r2 := interpretaDoc(t, m, d1, usoSconosciuto(d1)), interpretaDoc(t, m, d1bis, usoSconosciuto(d1bis)), interpretaDoc(t, m, d2, usoSconosciuto(d2))
	if r1.Impronta != r1bis.Impronta || r1.ID != r1bis.ID {
		t.Fatal("l'interpretazione del documento rifatto è diversa")
	}
	if r1.ID == r2.ID {
		t.Fatal("due fotografie diverse danno la stessa identità d'interpretazione")
	}
}

// ---- D1 ----

// TestD1LaFormazioneSTEPNonEUnaRevisione (D1, la forma sintetica della prova del conflitto STEP; E-12): file
// «9999999A_2.STP», radice «9999999A» con formazione «1», grammatica con il marcatore: dal nome base 9999999,
// marcatore A, revisione «2»; dalla radice la stessa base, revisione assente; la formazione «1» resta grezza, senza
// equivalenza, con revisione.formazione_non_confrontabile.
func TestD1LaFormazioneSTEPNonEUnaRevisione(t *testing.T) {
	fam := grammatica.FamigliaCodice{
		ID: "acme-marcatore", Namespace: "acme-marcatore", Ruoli: ruoliPC,
		Base: grammatica.Base{Segmenti: []grammatica.SegmentoBase{{Nome: "numero", Pattern: "[0-9]{7}", Identitario: true}},
			Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
			ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII},
		Forme: []grammatica.FormaCodice{
			formaACME("nome", []string{"nome_file"}, parteBase(), grammatica.Parte{Tipo: grammatica.TipoParteMarcatore, Letterali: []string{"A"}, Min: 1, Max: 1},
				grammatica.Parte{Tipo: grammatica.TipoParteSeparatore, Letterali: []string{"_"}, Min: 1, Max: 1},
				grammatica.Parte{Tipo: grammatica.TipoParteRevisione, Rif: "rev-a", Min: 1, Max: 1}),
			formaACME("step", []string{"radice_step.id", "nodo_step.id"}, parteBase(), grammatica.Parte{Tipo: grammatica.TipoParteMarcatore, Letterali: []string{"A"}, Min: 1, Max: 1}),
		},
		Revisioni: []grammatica.RegolaRevisione{{ID: "rev-a", Selettori: []string{"nome_file"}, Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]", Significato: grammatica.SignificatoNessuno}}}},
		Esempi: []grammatica.EsempioCodice{
			esempioACME("e-nome", "nome_file", "9999999A_2.STP", grammatica.LetturaAttesa{Forma: "nome", Base: "9999999", Marcatore: "A", Revisione: "2"}),
			esempioACME("e-step", "radice_step.id", "9999999A", grammatica.LetturaAttesa{Forma: "step", Base: "9999999", Marcatore: "A"}),
		},
	}
	m := motore(t, grammaticaGACME(nil, fam))
	d := docFile(t, "step", "step_09_cordoni")
	r := interpretaDoc(t, m, d, usoSconosciuto(d))
	nome, radice := lettureDi(r, "u:nome"), lettureDi(r, "u:step:#10:id")
	if len(nome) != 1 || len(radice) != 1 {
		t.Fatalf("letture: %v", ids(r.Letture))
	}
	n, rd := nome[0].Forma, radice[0].Forma
	if n.Base.Normalizzata != "9999999" || n.Marcatore == nil || n.Marcatore.Valore != "A" || n.Revisione == nil || n.Revisione.Normalizzata != "2" {
		t.Errorf("dal nome: %+v", n)
	}
	if rd.Base.Normalizzata != "9999999" || rd.Marcatore == nil || rd.Marcatore.Valore != "A" || rd.Revisione != nil {
		t.Errorf("dalla radice, revisione assente: %+v", rd)
	}
	var formazione *motorea.AttributoLetto
	for i, a := range r.Attributi {
		if a.Tipo == motorea.AttributoFormazione && a.EntitaID == "e:step:#10" {
			formazione = &r.Attributi[i]
		}
		if a.Tipo == motorea.AttributoRevisione {
			t.Errorf("una revisione da uno STEP: %+v", a)
		}
	}
	if formazione == nil || formazione.Grezzo != "1" || formazione.Normalizzato != "" || len(formazione.LettureCompatibili) != 0 {
		t.Fatalf("formazione della radice: %+v", formazione)
	}
	note := conCodice(r.Diagnostiche, motorea.CodiceRevisioneFormazioneNonConfrontabile)
	if len(note) != 1 {
		t.Fatalf("attesa una revisione.formazione_non_confrontabile per documento: %+v", note)
	}
	rif := strings.Join(note[0].Rif, " ")
	if !strings.Contains(rif, formazione.ID) || !strings.Contains(rif, nome[0].ID) {
		t.Errorf("la nota cita %v: attesi la formazione e la revisione letta dal nome", note[0].Rif)
	}
	if note[0].Gravita != evidenze.GravitaNota || note[0].Natura != evidenze.NaturaDati {
		t.Errorf("formazione non confrontabile: %s %s, attesi nota e dati (5.4.9)", note[0].Gravita, note[0].Natura)
	}
	// La revisione del nome e la formazione della radice non diventano «compatibili» né «discordanti».
	if len(conCodice(r.Diagnostiche, motorea.CodiceRevisioneDiscordante)) != 0 {
		t.Error("formazione confrontata con una revisione")
	}
}

// ---- HASH-CONFLITTO ----

// TestLoStessoContenutoConDueNomiDaDueFonti (HASH-CONFLITTO; P1 §3.1): lo stesso contenuto con due nomi e due
// identità dà due fonti, due BundleID, lo stesso riferimento ai fatti, due interpretazioni, letture dal nome
// diverse, nessuna fusione.
func TestLoStessoContenutoConDueNomiDaDueFonti(t *testing.T) {
	m := motoreGACME(t)
	in := ingressoAllegato(t, "step", "step_01_assieme")
	a2 := in.Allegato
	a2.ID = uuid.MustParse("00000000-0000-4000-8000-0000000000e2")
	a2.NomeFile = "ACME7000112.stp"
	d1, err := estrazione.DaAllegato(in.Allegato, in.Fatti, nil)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := estrazione.DaAllegato(a2, in.Fatti, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d1.Fonte.ID == d2.Fonte.ID || d1.BundleID == d2.BundleID || !reflect.DeepEqual(d1.Fonte.RiferimentoFatti, d2.Fonte.RiferimentoFatti) {
		t.Fatalf("fonti %s/%s, bundle %s/%s", d1.Fonte.ID, d2.Fonte.ID, d1.BundleID, d2.BundleID)
	}
	r1, r2 := interpretaDoc(t, m, d1, usoSconosciuto(d1)), interpretaDoc(t, m, d2, usoSconosciuto(d2))
	if r1.ID == r2.ID || r1.BundleID == r2.BundleID {
		t.Fatal("due fonti, una sola interpretazione")
	}
	n1, n2 := lettureDi(r1, "u:nome"), lettureDi(r2, "u:nome")
	if len(n1) != 1 || len(n2) != 1 || n1[0].Forma.Originale == n2[0].Forma.Originale {
		t.Fatalf("letture dal nome: %v | %v", ids(n1), ids(n2))
	}
	for _, l := range r2.Letture {
		if l.Forma.Originale == n1[0].Forma.Originale && l.UnitaID == "u:nome" {
			t.Error("la lettura del nome di una fonte è finita nell'altra")
		}
	}
}

// ---- A1b-19: la quantità ----

// TestLaQuantitaVieneDallaColonnaDichiarata (A1b-19; R28 a; R48 A): la colonna «Q.TA» dichiarata dà la quantità di
// ogni riga con codice, legata alla riga, con l'intestazione in Evidenze; «5 pz» è non interpretabile. Tentativi
// che non devono dare quantità: nessuna regola, regola riservata, regola solo su corpo con la tabella nella storia,
// letterale con le maiuscole diverse, intestazione non dichiarata, intestazione che non è la prima riga non vuota
// sopra le righe con codice, riga senza codice.
func TestLaQuantitaVieneDallaColonnaDichiarata(t *testing.T) {
	q := func(stato string, intestazioni []string, selettori ...string) *motorea.Motore {
		return motore(t, grammaticaGACME([]grammatica.QuantitaTabellare{quantitaACME(stato, intestazioni, selettori...)}, famMail(), famDisegno(), famPB()))
	}
	attiva := q(grammatica.StatoAttiva, []string{"Q.TA"}, "corpo", "storia")
	m4 := messaggio(t, "mail_04_inoltro_tabella")

	t.Run("quattro righe con codice, intestazione nella seconda riga", func(t *testing.T) {
		d := docMail(t, m4)
		r := interpretaDoc(t, attiva, d, usoSconosciuto(d))
		qa := attributi(r, motorea.AttributoQuantita)
		if len(qa) != 4 {
			t.Fatalf("attese quattro quantità: %+v", qa)
		}
		for i, a := range qa {
			riga := "r" + string(rune('3'+i))
			if a.UnitaID != "u:tab:1:"+riga+":c3" || a.EntitaID != "e:tab:1:"+riga || a.Stato != motorea.StatoAttribuito ||
				a.Normalizzato != "5" || !reflect.DeepEqual(a.Evidenze, []string{"u:tab:1:r2:c3"}) || len(a.LettureCompatibili) != 0 {
				t.Errorf("quantità %d: %+v", i, a)
			}
		}
		if len(conCodice(r.Diagnostiche, motorea.CodiceQuantitaNonInterpretabile)) != 0 {
			t.Error("quantita.non_interpretabile senza motivo")
		}
	})

	t.Run("5 pz non è interpretabile", func(t *testing.T) {
		v := variante(t, m4, []string{"Staffa\r\n5\r\n", "Staffa\r\n5 pz\r\n"}, []string{"<td>Staffa</td><td>5</td>", "<td>Staffa</td><td>5 pz</td>"})
		d := docMail(t, v)
		r := interpretaDoc(t, attiva, d, usoSconosciuto(d))
		var trovata bool
		for _, a := range attributi(r, motorea.AttributoQuantita) {
			if a.UnitaID == "u:tab:1:r3:c3" {
				trovata = true
				if a.Stato != motorea.StatoNonInterpretabile || a.Grezzo != "5 pz" || a.Normalizzato != "" {
					t.Errorf("«5 pz»: %+v", a)
				}
			} else if a.Stato != motorea.StatoAttribuito {
				t.Errorf("le altre righe restano attribuite: %+v", a)
			}
		}
		if !trovata || len(conCodice(r.Diagnostiche, motorea.CodiceQuantitaNonInterpretabile)) != 1 {
			t.Errorf("manca la quantità non interpretabile con la sua nota: %+v", r.Diagnostiche)
		}
	})

	nessuna := func(t *testing.T, m *motorea.Motore, msg fotorfq.Messaggio, perche string) {
		t.Helper()
		d := docMail(t, msg)
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		if qa := attributi(r, motorea.AttributoQuantita); len(qa) != 0 {
			t.Errorf("%s: quantità senza la regola che la giustifichi: %+v", perche, qa)
		}
	}
	t.Run("nessun legame senza la regola attiva per il selettore", func(t *testing.T) {
		nessuna(t, motore(t, grammaticaGACME(nil, famMail(), famDisegno(), famPB())), m4, "senza quantita_tabellare")
		nessuna(t, q(grammatica.StatoRiservata, []string{"Q.TA"}, "corpo", "storia"), m4, "regola riservata")
		nessuna(t, q(grammatica.StatoAttiva, []string{"Q.TA"}, "corpo"), m4, "regola solo sul corpo, tabella nella storia (R48 A)")
		nessuna(t, q(grammatica.StatoAttiva, []string{"Q.Ta"}, "corpo", "storia"), m4, "letterale con le maiuscole diverse")
		// Con la sola regola sul corpo, lo stesso corpo senza prefisso d'inoltro (corpo corrente) dà le quantità.
		d := docMail(t, messaggio(t, "mail_04b_senza_prefisso"))
		r := interpretaDoc(t, q(grammatica.StatoAttiva, []string{"Q.TA"}, "corpo"), d, usoSconosciuto(d))
		if len(attributi(r, motorea.AttributoQuantita)) != 4 {
			t.Errorf("F-MAIL-4b con la regola sul corpo: %+v", attributi(r, motorea.AttributoQuantita))
		}
	})

	t.Run("nessun legame da una colonna senza l'intestazione dichiarata", func(t *testing.T) {
		v := variante(t, m4, []string{"Q.TA", "QUANTITA"}, []string{"<td>Q.TA</td>", "<td>QUANTITA</td>"})
		nessuna(t, attiva, v, "intestazione non dichiarata")
		// Una riga non vuota fra l'intestazione e le righe con codice: l'intestazione è quella riga (la prima non
		// vuota sopra), che non ha il letterale. Nessuna colonna si sceglie per somiglianza o distanza.
		v = variante(t, m4, []string{"Q.TA\r\n", "Q.TA\r\nGruppo\r\nA\r\n"},
			[]string{"<td>Q.TA</td></tr>\n", "<td>Q.TA</td></tr>\n<tr><td>Gruppo</td><td>A</td><td></td></tr>\n"})
		nessuna(t, attiva, v, "intestazione che non è la prima riga non vuota sopra le righe con codice")
	})

	t.Run("nessuna quantità su una riga senza codice", func(t *testing.T) {
		v := variante(t, m4, []string{"Boccola\r\n5\r\n", "Boccola\r\n5\r\nNota\r\nVarie\r\n7\r\n"},
			[]string{"<td>Boccola</td><td>5</td></tr>\n", "<td>Boccola</td><td>5</td></tr>\n<tr><td>Nota</td><td>Varie</td><td>7</td></tr>\n"})
		d := docMail(t, v)
		r := interpretaDoc(t, attiva, d, usoSconosciuto(d))
		qa := attributi(r, motorea.AttributoQuantita)
		for _, a := range qa {
			if a.Grezzo == "7" || a.EntitaID == "e:tab:1:r7" {
				t.Errorf("quantità su una riga senza codice: %+v", a)
			}
		}
		if len(qa) != 4 {
			t.Errorf("attese le quattro quantità delle righe con codice: %+v", qa)
		}
	})

	t.Run("tabella a TAB con le celle per righe", func(t *testing.T) {
		d := docMail(t, messaggio(t, "mail_05a_tab_e_non_combacia"))
		r := interpretaDoc(t, attiva, d, usoSconosciuto(d))
		qa := attributi(r, motorea.AttributoQuantita)
		if len(qa) != 2 || qa[0].EntitaID != "e:tab:1:r2" || qa[0].Normalizzato != "5" || qa[1].EntitaID != "e:tab:1:r3" || qa[1].Normalizzato != "3" {
			t.Errorf("quantità della tabella a TAB: %+v", qa)
		}
	})
}

// ---- A1b-23 ----

// TestUnCampoRicevutoENonLettoSiDice (A1b-23; v3 §2): con una grammatica senza forme sul cartiglio, ogni selettore
// presente e senza forme ha una sola nota capacita.non_supportata, con tutte le sue unità in Rif; il nome, che ha
// la sua forma, no.
func TestUnCampoRicevutoENonLettoSiDice(t *testing.T) {
	soloNome := famDisegno()
	soloNome.Forme[0].Selettori = []string{"nome_file"}
	soloNome.Revisioni = nil
	soloNome.Esempi = soloNome.Esempi[:1]
	m := motore(t, grammaticaGACME(nil, soloNome))
	d := docFile(t, "pdf", "pdf_01_cartiglio")
	r := interpretaDoc(t, m, d, usoSconosciuto(d))
	perSelettore := map[string][]string{}
	for _, u := range d.Unita {
		if u.Selettore.String() != "nome_file" {
			perSelettore[u.Selettore.String()] = append(perSelettore[u.Selettore.String()], u.ID)
		}
	}
	note := conCodice(r.Diagnostiche, grammatica.CodiceCapacitaNonSupportata)
	if len(note) != len(perSelettore) {
		t.Fatalf("%d note per %d selettori senza forme:\n%+v", len(note), len(perSelettore), note)
	}
	for s, unita := range perSelettore {
		trovate := 0
		for _, n := range note {
			if strings.Contains(n.Percorso, s) {
				trovate++
				if !reflect.DeepEqual(n.Rif, unita) {
					t.Errorf("%s: Rif %v, attese le unità %v", s, n.Rif, unita)
				}
				if n.Gravita != evidenze.GravitaNota {
					t.Errorf("%s: gravità %s, attesa nota", s, n.Gravita)
				}
			}
		}
		if trovate != 1 {
			t.Errorf("%s: %d note, attesa una", s, trovate)
		}
	}
	for _, n := range note {
		if strings.Contains(n.Percorso, "nome_file") {
			t.Errorf("nota su un selettore con la sua forma: %+v", n)
		}
	}
}

// ---- relazioni ----

// TestNessunaRelazioneDalTestoLibero (5.4.6 punto 13; P1 §10.1; C-39): «SPECCHIATO DI» in un frammento e
// «SIMILE A» in una descrizione STEP restano menzioni (o niente, su un selettore riservato): nessuna relazione.
func TestNessunaRelazioneDalTestoLibero(t *testing.T) {
	m := motoreGACME(t)
	for _, c := range []struct{ tipo, nome string }{{"pdf", "pdf_06_specchiato"}, {"step", "step_01_assieme"}, {"pdf", "sottoversione_1"}, {"pdf", "pdf_01_cartiglio"}} {
		d := docFile(t, c.tipo, c.nome)
		r := interpretaDoc(t, m, d, usoSconosciuto(d))
		if len(r.Relazioni) != 0 {
			t.Errorf("%s: relazioni dal testo libero: %+v", c.nome, r.Relazioni)
		}
		for _, l := range r.Letture {
			if l.Forma.Selettore.Contesto == evidenze.ContestoTestoPDF && l.Funzione != motorea.FunzMenzione {
				t.Errorf("%s: %s con funzione %s", c.nome, l.ID, l.Funzione)
			}
		}
	}
	d := docFile(t, "pdf", "pdf_06_specchiato")
	r := interpretaDoc(t, m, d, usoSconosciuto(d))
	var specchiato []motorea.LetturaCodice
	for _, l := range r.Letture {
		if l.Forma.Originale == "ACME7000101" {
			specchiato = append(specchiato, l)
		}
	}
	if len(specchiato) != 1 || specchiato[0].Funzione != motorea.FunzMenzione || len(specchiato[0].RuoliCandidati) != 0 {
		t.Errorf("il codice dopo «SPECCHIATO DI»: %v", ids(specchiato))
	}
}
