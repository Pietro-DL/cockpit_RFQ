// L1 — Interpreta su documenti costruiti in Go (piano A, 5.4.6; 5.7.1: A-C02, A-C04, A-C11, A1b-21, A1b-22): la
// firma senza target, la porta del contratto senza ripieghi, il router dentro l'interpretazione (celle comprese),
// gli attributi legati all'entità e mai al codice più vicino, la deduplica solo della stessa occorrenza, le
// ambiguità conservate con la loro diagnostica, i limiti che danno un risultato parziale e lo stato che conta solo
// le capacità di lettura. Ogni sottoprova «nessun legame» è un tentativo di far nascere un collegamento senza
// provenance, o con la provenance ma senza la regola che lo giustifica (5.0, principio dell'utente del 04/10).
package motorea

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in
// cui un cliente cambia convenzione. Le famiglie (acme-codice, acme-a, acme-b, acme-annidata, acme-seconda) hanno
// basi inventate e attivano un meccanismo, mai il significato di un cliente; i documenti sono D-sint (5.7.2),
// costruiti qui senza adattatore. Le prove citano i requisiti (A-C02, A-C11, A1b-21, R28 a), mai i casi degli
// attesi.

// ---- i documenti D-sint ----

const (
	bundleDSint = "bundle-dsint-acme-1"
	fonteDSint  = "testo:dsint-acme"
	corpoDSint  = "messaggio.corpo_testo" // il testo su cui si misura PosTabella.Esatto
	oggettoDSnt = "messaggio.oggetto"
)

// docDSint: un documento D-sint con la fonte «testo», valido per evidenze.ValidaDocumento.
func docDSint(testi []evidenze.TestoOriginale, segmenti []evidenze.Segmento, entita []evidenze.EntitaLocale,
	unita []evidenze.UnitaEvidenza, capacita ...evidenze.Capacita) evidenze.DocumentoEvidenze {
	return evidenze.DocumentoEvidenze{
		BundleID: bundleDSint, VersioneSchema: evidenze.VersioneSchemaDocumento, VersioneAdattatore: "adattatori-dsint",
		Fonte: evidenze.Fonte{ID: fonteDSint, Tipo: "testo", RiferimentoFatti: evidenze.RiferimentoFatti{Tipo: "nessuno"}},
		Testi: testi, Segmenti: segmenti, Entita: entita, Unita: unita,
		Qualita: evidenze.QualitaFonte{Stato: "disponibile", Capacita: capacita},
	}
}

func testoOrig(id, testo string) evidenze.TestoOriginale {
	return evidenze.TestoOriginale{ID: id, Testo: testo, Origine: id}
}

func posTesto(testoID string, inizio, fine int) evidenze.Localizzatore {
	return evidenze.Localizzatore{Tipo: "testo", Testo: &evidenze.PosTesto{TestoID: testoID, Intervallo: evidenze.Intervallo{Inizio: inizio, Fine: fine}}}
}

func segDSint(id, tipo, logico, testoID string, inizio, fine int) evidenze.Segmento {
	return evidenze.Segmento{ID: id, Tipo: tipo, MessaggioLogicoID: logico, Origine: "ignota", Posizione: posTesto(testoID, inizio, fine)}
}

func entDisegno(id string) evidenze.EntitaLocale {
	return evidenze.EntitaLocale{ID: id, Tipo: "disegno", ChiaveOriginale: id, Posizione: evidenze.Localizzatore{Tipo: "pdf", PDF: &evidenze.PosPDF{Pagina: 1}}}
}

func entIsolata(id string) evidenze.EntitaLocale {
	e := entDisegno(id)
	e.Tipo = "unita_isolata"
	return e
}

func entNodo(id, chiave string) evidenze.EntitaLocale {
	return evidenze.EntitaLocale{ID: id, Tipo: "nodo_step", ChiaveOriginale: chiave,
		Posizione: evidenze.Localizzatore{Tipo: "step", STEP: &evidenze.PosSTEP{Chiave: chiave, Attributo: "id"}}}
}

func entRiga(id, segmento string, tabella, riga int, righe [2]int) evidenze.EntitaLocale {
	r := righe
	return evidenze.EntitaLocale{ID: id, Tipo: "riga", SegmentoID: segmento, ChiaveOriginale: id,
		Posizione: evidenze.Localizzatore{Tipo: "tabella", Tabella: &evidenze.PosTabella{Tabella: tabella, Riga: riga, RigheTesto: &r}}}
}

// uTesto: un'unità di oggetto, corpo o storia, esatta su un testo originale.
func uTesto(t *testing.T, id, segmento, s, testoID, testo string, inizio int) evidenze.UnitaEvidenza {
	return evidenze.UnitaEvidenza{ID: id, FonteID: fonteDSint, SegmentoID: segmento, Selettore: selettore(t, s), Testo: testo,
		Posizione: posTesto(testoID, inizio, inizio+len(testo)), Qualita: evidenze.QualitaUnita{Localizzazione: "esatta"}}
}

// uCampo: un campo del cartiglio, un frammento o un metadato del PDF, con la sua entità ("" = nessuna).
func uCampo(t *testing.T, id, entita, s, testo string) evidenze.UnitaEvidenza {
	return evidenze.UnitaEvidenza{ID: id, FonteID: fonteDSint, EntitaID: entita, Selettore: selettore(t, s), Testo: testo,
		Posizione: evidenze.Localizzatore{Tipo: "pdf", PDF: &evidenze.PosPDF{Pagina: 1, Zona: "basso_destra", Fonte: "nativo"}},
		Qualita:   evidenze.QualitaUnita{Localizzazione: "esatta", Metodo: "nativo"}}
}

// uStep: un attributo di un nodo STEP.
func uStep(t *testing.T, id, entita, chiave, s, attributo, testo string) evidenze.UnitaEvidenza {
	return evidenze.UnitaEvidenza{ID: id, FonteID: fonteDSint, EntitaID: entita, Selettore: selettore(t, s), Testo: testo,
		Posizione: evidenze.Localizzatore{Tipo: "step", STEP: &evidenze.PosSTEP{Chiave: chiave, Attributo: attributo}},
		Qualita:   evidenze.QualitaUnita{Localizzazione: "esatta", Metodo: "parser"}}
}

// uCella: una cella di una tabella della mail, con la sua riga, la colonna, le righe del testo e, se coincide con
// le sue righe, l'intervallo esatto sul corpo.
func uCella(t *testing.T, id, entita, s, testo string, tabella, riga, colonna int, righe [2]int, esatto *evidenze.Intervallo) evidenze.UnitaEvidenza {
	r := righe
	loc := "parziale"
	if esatto != nil {
		loc = "esatta"
	}
	return evidenze.UnitaEvidenza{ID: id, FonteID: fonteDSint, EntitaID: entita, Selettore: selettore(t, s), Testo: testo,
		Posizione: evidenze.Localizzatore{Tipo: "tabella", Tabella: &evidenze.PosTabella{Tabella: tabella, Riga: riga,
			Cella: colonna, Colonna: colonna, RigheTesto: &r, Esatto: esatto}},
		Qualita: evidenze.QualitaUnita{Localizzazione: loc, Metodo: "parser"}}
}

func capacita(nome, stato string) evidenze.Capacita {
	return evidenze.Capacita{Nome: nome, Stato: stato}
}

// usoValutato: un uso «valutato» del documento D-sint con le selezioni date.
func usoValutato(selezioni ...evidenze.SelezioneSegmento) evidenze.UsoSegmenti {
	return evidenze.UsoSegmenti{BundleID: bundleDSint, Versione: 1, Stato: "valutato", Selezioni: selezioni}
}

func selezione(segmento, uso, origine string) evidenze.SelezioneSegmento {
	return evidenze.SelezioneSegmento{SegmentoID: segmento, Uso: uso, Origine: origine}
}

// ---- le grammatiche ----

// famCodice: acme-codice, {prodotto, componente}, base «ACME» più quattro cifre, una forma su ogni selettore attivo
// che le prove usano, la revisione in campo separato a due cifre sul cartiglio.
func famCodice() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-codice", Namespace: "acme-codice",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segLetterale("prefisso", "ACME", ""), segPattern("numero", "[0-9]{4}", "")),
		Forme: []grammatica.FormaCodice{
			forma("ovunque", sel("oggetto", "corpo", "storia", "nome_file", "cartiglio.codice", "radice_step.id", "nodo_step.id", "testo_pdf"), pBase()),
		},
		Revisioni: []grammatica.RegolaRevisione{revCampo("rev-campo")},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-corpo", "corpo", "ACME1111", grammatica.LetturaAttesa{Forma: "ovunque", Base: "ACME1111"}),
			positivo("e-revisione", "cartiglio.revisione", "01", grammatica.LetturaAttesa{Revisione: "01"}),
		},
	}
}

// revCampo: una revisione in campo separato sul cartiglio, due cifre, senza forte e debole (D9).
func revCampo(id string) grammatica.RegolaRevisione {
	return grammatica.RegolaRevisione{
		ID: id, Selettori: sel("cartiglio.revisione"), Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteCampoSeparato,
		Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]{2}", Significato: grammatica.SignificatoNessuno}},
	}
}

func motoreCodice(t *testing.T) *Motore {
	t.Helper()
	m, _ := compilaBene(t, grammaticaACME(famCodice()))
	return m
}

// ---- l'interpretazione e i suoi invarianti ----

// interpreta chiama Interpreta e controlla gli invarianti che valgono per ogni risultato (5.4.6): ogni lettura è
// sul testo della sua unità e, se ha l'assoluto, sull'originale; ogni trasformazione ricostruisce la sua parte;
// i ruoli stanno dentro quelli ammessi dalla funzione; ogni attributo è della sua entità, e le letture compatibili
// sono della stessa entità; ogni elenco è in ordine di ID; l'impronta è quella del canonico.
func interpreta(t *testing.T, m *Motore, doc evidenze.DocumentoEvidenze, uso evidenze.UsoSegmenti) Interpretazione {
	t.Helper()
	r, err := m.Interpreta(doc, uso)
	if err != nil {
		t.Fatalf("Interpreta: %v\n%s", err, elenco(diagnosticheDi(err)))
	}
	controllaInterpretazione(t, doc, r)
	return r
}

func controllaInterpretazione(t *testing.T, doc evidenze.DocumentoEvidenze, r Interpretazione) {
	t.Helper()
	unita := map[string]evidenze.UnitaEvidenza{}
	for _, u := range doc.Unita {
		unita[u.ID] = u
	}
	testi := map[string]string{}
	for _, x := range doc.Testi {
		testi[x.ID] = x.Testo
	}
	letture := map[string]LetturaCodice{}
	for i, l := range r.Letture {
		if i > 0 && r.Letture[i-1].ID >= l.ID {
			t.Errorf("letture fuori ordine o ripetute: %s, %s", r.Letture[i-1].ID, l.ID)
		}
		letture[l.ID] = l
		u, ok := unita[l.UnitaID]
		if !ok {
			t.Errorf("lettura %s su un'unità che non c'è", l.ID)
			continue
		}
		o := l.Occorrenza
		if o.Inizio < 0 || o.Fine > len(u.Testo) || o.Inizio >= o.Fine || u.Testo[o.Inizio:o.Fine] != l.Forma.Originale {
			t.Errorf("lettura %s: l'occorrenza %v non ricostruisce %q sul testo dell'unità", l.ID, o, l.Forma.Originale)
		}
		if a := l.Assoluto; a != nil {
			orig, ok := testi[a.TestoID]
			if !ok || a.Intervallo.Fine > len(orig) || a.Intervallo.Inizio < 0 || orig[a.Intervallo.Inizio:a.Intervallo.Fine] != l.Forma.Originale {
				t.Errorf("lettura %s: l'assoluto %+v non ricostruisce l'originale (A-C03)", l.ID, *a)
			}
		}
		for _, tr := range l.Trasformazioni {
			if len(tr.Intervalli) == 0 {
				t.Errorf("lettura %s: trasformazione %s senza intervalli", l.ID, tr.Operazione)
				continue
			}
			iv := tr.Intervalli[0]
			if iv.Inizio < 0 || iv.Fine > len(u.Testo) || u.Testo[iv.Inizio:iv.Fine] != tr.Prima {
				t.Errorf("lettura %s: la trasformazione %s non ricostruisce %q", l.ID, tr.Operazione, tr.Prima)
			}
		}
		for _, ruolo := range l.RuoliCandidati {
			if !inRuoli(ruolo, RuoliAmmessi(l.Funzione)) {
				t.Errorf("lettura %s: ruolo %s fuori da quelli ammessi da %s", l.ID, ruolo, l.Funzione)
			}
		}
		if l.Uso == "" || l.MotivoRuoli == "" || len(l.Motivi) == 0 {
			t.Errorf("lettura %s senza uso, motivo dei ruoli o motivi: %+v", l.ID, l)
		}
		for _, a := range l.AltreUnita {
			if _, ok := unita[a]; !ok || a == l.UnitaID {
				t.Errorf("lettura %s: AltreUnita %q non valida", l.ID, a)
			}
		}
	}
	for i, a := range r.Attributi {
		if i > 0 && r.Attributi[i-1].ID >= a.ID {
			t.Errorf("attributi fuori ordine o ripetuti: %s, %s", r.Attributi[i-1].ID, a.ID)
		}
		u, ok := unita[a.UnitaID]
		if !ok {
			t.Errorf("attributo %s su un'unità che non c'è", a.ID)
			continue
		}
		// A-C02: l'attributo è dell'entità dell'unità che lo porta, e un'unità senza entità non ne dà.
		if a.EntitaID == "" || a.EntitaID != u.EntitaID {
			t.Errorf("attributo %s legato a %q, l'unità %s è dell'entità %q", a.ID, a.EntitaID, u.ID, u.EntitaID)
		}
		if a.Grezzo != u.Testo {
			t.Errorf("attributo %s: grezzo %q, il testo dell'unità è %q: l'originale si conserva", a.ID, a.Grezzo, u.Testo)
		}
		for _, lid := range a.LettureCompatibili {
			l, ok := letture[lid]
			if !ok {
				t.Errorf("attributo %s: lettura compatibile %s che non c'è", a.ID, lid)
				continue
			}
			if e := unita[l.UnitaID].EntitaID; e != a.EntitaID {
				t.Errorf("attributo %s dell'entità %s legato alla lettura %s dell'entità %q: nessuna provenance", a.ID, a.EntitaID, lid, e)
			}
		}
		for _, ev := range a.Evidenze {
			if _, ok := unita[ev]; !ok {
				t.Errorf("attributo %s: evidenza %s che non c'è", a.ID, ev)
			}
		}
	}
	for _, rel := range r.Relazioni {
		for _, lid := range rel.LettureBersaglio {
			if l, ok := letture[lid]; !ok || l.Funzione != FunzRelazione {
				t.Errorf("relazione %s da una lettura che non ha la funzione relazione: %s", rel.ID, lid)
			}
		}
	}
	for _, d := range r.Diagnostiche {
		if d.Codice == "" || d.Gravita == "" || d.Natura == "" {
			t.Errorf("diagnostica incompleta: %+v", d)
		}
		if d.Gravita == evidenze.GravitaErrore {
			t.Errorf("un errore dentro l'Interpretazione: l'errore di Interpreta è solo di contratto: %+v", d)
		}
	}
	c := r
	c.Impronta = ""
	if imp, err := jsoncanonico.ImprontaDi(c); err != nil || imp != r.Impronta || len(r.Impronta) != 64 {
		t.Errorf("Impronta %q, il canonico del risultato dà %q (%v)", r.Impronta, imp, err)
	}
	if r.ID == "" || r.ID == r.Impronta || r.BundleID != doc.BundleID {
		t.Errorf("identità dell'interpretazione: ID %q, bundle %q", r.ID, r.BundleID)
	}
	if r.VersioneRouter != VersioneRouter || r.VersioneRisultato != VersioneRisultato || r.VersioneAlgoritmo != VersioneAlgoritmo {
		t.Errorf("versioni nell'interpretazione: %s %d %s", r.VersioneRouter, r.VersioneRisultato, r.VersioneAlgoritmo)
	}
}

func lettureDellUnita(r Interpretazione, unita string) []LetturaCodice {
	var out []LetturaCodice
	for _, l := range r.Letture {
		if l.UnitaID == unita {
			out = append(out, l)
		}
	}
	return out
}

func attributiDi(r Interpretazione, tipo string) []AttributoLetto {
	var out []AttributoLetto
	for _, a := range r.Attributi {
		if a.Tipo == tipo {
			out = append(out, a)
		}
	}
	return out
}

func attributoDellUnita(t *testing.T, r Interpretazione, tipo, unita string) AttributoLetto {
	t.Helper()
	var trovati []AttributoLetto
	for _, a := range r.Attributi {
		if a.Tipo == tipo && a.UnitaID == unita {
			trovati = append(trovati, a)
		}
	}
	if len(trovati) != 1 {
		t.Fatalf("attesi un attributo %s dell'unità %s, trovati %d: %+v", tipo, unita, len(trovati), r.Attributi)
	}
	return trovati[0]
}

func idLetture(l []LetturaCodice) []string {
	var out []string
	for _, x := range l {
		out = append(out, x.ID)
	}
	return out
}

// ---- A-C11 ----

// TestAC11CambiareITargetNonCambiaLInterpretazione (A-C11; P1 §7.3, §12): la firma di Interpreta ha solo
// documento e uso, con reflect; due chiamate danno lo stesso ID, la stessa impronta e gli stessi byte; l'identità
// comprende uso, router e limiti (R41 c, R43 B); motorea non importa ancoraggio né confronto (G1).
func TestAC11CambiareITargetNonCambiaLInterpretazione(t *testing.T) {
	t.Run("la firma ha solo documento e uso", func(t *testing.T) {
		meth, ok := reflect.TypeOf((*Motore)(nil)).MethodByName("Interpreta")
		if !ok {
			t.Fatal("Motore non ha il metodo Interpreta")
		}
		tipo := meth.Type
		if tipo.NumIn() != 3 || tipo.In(1) != reflect.TypeOf(evidenze.DocumentoEvidenze{}) || tipo.In(2) != reflect.TypeOf(evidenze.UsoSegmenti{}) {
			t.Fatalf("firma di Interpreta: %v; attesi solo il documento e l'uso dei segmenti", tipo)
		}
		if tipo.NumOut() != 2 || tipo.Out(0) != reflect.TypeOf(Interpretazione{}) || tipo.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			t.Fatalf("risultati di Interpreta: %v", tipo)
		}
		if tipo.IsVariadic() {
			t.Fatal("Interpreta è variadica: un target potrebbe entrare dalla porta di servizio")
		}
		imp := reflect.TypeOf(ImprontaUso)
		if imp.NumIn() != 1 || imp.In(0) != reflect.TypeOf(evidenze.UsoSegmenti{}) || imp.NumOut() != 1 || imp.Out(0).Kind() != reflect.String {
			t.Fatalf("firma di ImprontaUso: %v", imp)
		}
	})

	m := motoreCodice(t)
	doc := docDSint(
		[]evidenze.TestoOriginale{testoOrig(oggettoDSnt, "Richiesta ACME1111"), testoOrig(corpoDSint, "Serve ACME2222.\n> ACME3333")},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, 16), segDSint("s:storia:1", evidenze.SegmentoCitazione, "m1", corpoDSint, 16, 26)},
		nil,
		[]evidenze.UnitaEvidenza{
			uTesto(t, "u:oggetto", "s:corrente", "oggetto", oggettoDSnt, "Richiesta ACME1111", 0),
			uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, "Serve ACME2222.", 0),
			uTesto(t, "u:storia:s:storia:1", "s:storia:1", "storia", corpoDSint, "\n> ACME3333", 15),
		})
	// Il segmento corrente va da 0 a 16 e la storia da 16: le unità stanno dentro (la foglia non lo pretende).
	doc.Segmenti[0].Posizione = posTesto(corpoDSint, 0, 15)
	doc.Segmenti[1].Posizione = posTesto(corpoDSint, 15, 26)

	t.Run("due chiamate danno gli stessi byte", func(t *testing.T) {
		uso := evidenze.UsoSconosciuto(bundleDSint)
		a := interpreta(t, m, doc, uso)
		b := interpreta(t, m, doc, uso)
		if a.ID != b.ID || a.Impronta != b.Impronta || !bytes.Equal(canonico(t, a), canonico(t, b)) {
			t.Fatalf("due chiamate diverse: %s/%s, %s/%s", a.ID, a.Impronta, b.ID, b.Impronta)
		}
		if len(a.Letture) != 3 {
			t.Fatalf("attese tre letture, una per unità: %v", idLetture(a.Letture))
		}
		if a.ImprontaUso == "" || a.ImprontaUso != ImprontaUso(uso) {
			t.Fatalf("l'uso sconosciuto ha la sua impronta, mai vuota: %q, %q", a.ImprontaUso, ImprontaUso(uso))
		}
		if a.VersioneLimiti != "limiti-acme-1" || a.ImprontaLimiti == "" || a.HashSnapshot != m.Snapshot().Hash || a.ClienteID != clienteACME {
			t.Fatalf("identità: limiti %q/%q, snapshot %q, cliente %v", a.VersioneLimiti, a.ImprontaLimiti, a.HashSnapshot, a.ClienteID)
		}
	})

	t.Run("l'uso entra nell'identità; l'ordine delle selezioni no", func(t *testing.T) {
		sconosciuto := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
		u1 := usoValutato(selezione("s:corrente", UsoPertinente, "operatore"), selezione("s:storia:1", UsoEscluso, "operatore"))
		u2 := usoValutato(selezione("s:storia:1", UsoEscluso, "operatore"), selezione("s:corrente", UsoPertinente, "operatore"))
		a, b := interpreta(t, m, doc, u1), interpreta(t, m, doc, u2)
		if ImprontaUso(u1) != ImprontaUso(u2) || a.ID != b.ID || a.Impronta != b.Impronta {
			t.Fatal("le stesse selezioni in un altro ordine cambiano l'interpretazione")
		}
		if a.ID == sconosciuto.ID || a.ImprontaUso == sconosciuto.ImprontaUso {
			t.Fatal("un uso valutato e l'uso sconosciuto hanno la stessa identità")
		}
		u3 := usoValutato(selezione("s:corrente", UsoPertinente, "scenario"), selezione("s:storia:1", UsoEscluso, "operatore"))
		if ImprontaUso(u3) == ImprontaUso(u1) {
			t.Fatal("l'origine della selezione non entra nell'impronta dell'uso")
		}
	})

	t.Run("i limiti entrano nell'identità, valori e versione", func(t *testing.T) {
		uso := evidenze.UsoSconosciuto(bundleDSint)
		base := interpreta(t, m, doc, uso)
		lim := limitiACME()
		lim.Riconoscimento.MaxLettureDocumento = 19999
		m2, _, err := compila(t, grammaticaACME(famCodice()), lim)
		if err != nil {
			t.Fatal(err)
		}
		altro := interpreta(t, m2, doc, uso)
		if altro.ImprontaLimiti == base.ImprontaLimiti || altro.ID == base.ID {
			t.Fatal("un valore dei limiti diverso non cambia l'identità dell'interpretazione (R43 B)")
		}
		lim = limitiACME()
		lim.Versione = "limiti-acme-2"
		m3, _, err := compila(t, grammaticaACME(famCodice()), lim)
		if err != nil {
			t.Fatal(err)
		}
		v2 := interpreta(t, m3, doc, uso)
		if v2.VersioneLimiti != "limiti-acme-2" || v2.ID == base.ID {
			t.Fatal("la versione dei limiti non entra nell'identità (R43 B)")
		}
		// Lo snapshot è lo stesso: i limiti stanno fuori dal suo hash (R41 c riguarda il router, R43 B i limiti).
		if v2.HashSnapshot != base.HashSnapshot {
			t.Fatal("i limiti cambiano l'hash dello snapshot")
		}
	})

	t.Run("motorea non importa ancoraggio, confronto né il motore legacy", func(t *testing.T) {
		voci, err := os.ReadDir(".")
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		for _, v := range voci {
			if v.IsDir() || !strings.HasSuffix(v.Name(), ".go") || strings.HasSuffix(v.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(".", v.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if strings.Contains(p, "ancoraggio") || strings.Contains(p, "confronto") || strings.HasSuffix(p, "/inbox/classificazione") ||
					p == "promatec/cockpit/internal/core/estrazione" {
					t.Errorf("%s importa %s (G1; A-C11)", v.Name(), p)
				}
			}
		}
	})
}

// ---- la porta del contratto ----

// TestLaPortaDiInterpretaNonHaRipieghi (P1 §7.3, §9.3; 5.4.6 punto 1): un uso inventato (un segmento che non c'è,
// un'origine o un uso fuori elenco, un altro bundle, uno «sconosciuto» con selezioni) e un documento non valido
// sono errori di contratto, con le diagnostiche documento.*, e nessuna interpretazione. Un uso «scenario» non si
// inventa: senza un segmento del documento non c'è niente da selezionare.
func TestLaPortaDiInterpretaNonHaRipieghi(t *testing.T) {
	m := motoreCodice(t)
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, "> ACME1111")},
		[]evidenze.Segmento{segDSint("s:storia:1", evidenze.SegmentoInoltro, "m1", corpoDSint, 0, 10)}, nil,
		[]evidenze.UnitaEvidenza{uTesto(t, "u:storia:s:storia:1", "s:storia:1", "storia", corpoDSint, "> ACME1111", 0)})

	casi := []struct {
		nome   string
		doc    evidenze.DocumentoEvidenze
		uso    evidenze.UsoSegmenti
		codice string
	}{
		{"scenario su un segmento che il documento non ha", doc, usoValutato(selezione("s:storia:2", UsoPertinente, "scenario")), evidenze.CodiceDocumentoUsoNonValido},
		{"origine inventata", doc, usoValutato(selezione("s:storia:1", UsoPertinente, "intuito")), evidenze.CodiceDocumentoUsoNonValido},
		{"uso inventato", doc, usoValutato(selezione("s:storia:1", "probabile", "scenario")), evidenze.CodiceDocumentoUsoNonValido},
		{"uso di un altro bundle", doc, func() evidenze.UsoSegmenti {
			u := usoValutato(selezione("s:storia:1", UsoPertinente, "scenario"))
			u.BundleID = "bundle-dsint-acme-2"
			return u
		}(), evidenze.CodiceDocumentoUsoNonValido},
		{"sconosciuto con una selezione", doc, func() evidenze.UsoSegmenti {
			u := evidenze.UsoSconosciuto(bundleDSint)
			u.Selezioni = []evidenze.SelezioneSegmento{selezione("s:storia:1", UsoPertinente, "scenario")}
			return u
		}(), evidenze.CodiceDocumentoUsoNonValido},
		{"stesso segmento selezionato due volte", doc, usoValutato(selezione("s:storia:1", UsoEscluso, "operatore"), selezione("s:storia:1", UsoPertinente, "scenario")),
			evidenze.CodiceDocumentoUsoNonValido},
		{"uso senza versione", doc, evidenze.UsoSegmenti{BundleID: bundleDSint, Stato: "sconosciuto"}, evidenze.CodiceDocumentoUsoNonValido},
		{"documento con un segmento pendente", func() evidenze.DocumentoEvidenze {
			d := doc
			d.Unita = []evidenze.UnitaEvidenza{uTesto(t, "u:storia:s:storia:9", "s:storia:9", "storia", corpoDSint, "> ACME1111", 0)}
			return d
		}(), evidenze.UsoSconosciuto(bundleDSint), evidenze.CodiceDocumentoRiferimentoPendente},
		{"unità con un testo diverso dall'originale", func() evidenze.DocumentoEvidenze {
			d := doc
			d.Unita = []evidenze.UnitaEvidenza{uTesto(t, "u:storia:s:storia:1", "s:storia:1", "storia", corpoDSint, "> ACME2222", 0)}
			return d
		}(), evidenze.UsoSconosciuto(bundleDSint), evidenze.CodiceDocumentoTestoDiversoDaOriginale},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			r, err := m.Interpreta(c.doc, c.uso)
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) {
				t.Fatalf("errore %T %v, atteso un *evidenze.ErroreContratto", err, err)
			}
			if len(conCodice(ec.Diagnostiche, c.codice)) == 0 {
				t.Fatalf("l'errore non porta %s:\n%s", c.codice, elenco(ec.Diagnostiche))
			}
			if !reflect.DeepEqual(r, Interpretazione{}) {
				t.Fatalf("con un errore di contratto nasce un'interpretazione: %+v", r)
			}
		})
	}
	// Lo stesso documento con l'uso giusto si interpreta: le prove di sopra rompono una cosa sola.
	interpreta(t, m, doc, usoValutato(selezione("s:storia:1", UsoPertinente, "scenario")))
}

// TestLoSnapshotCambiatoFermaInterpreta (par.3.3.4 e 3.4.2; R41 c): i ruoli, le revisioni riservate e la quantità
// Interpreta li legge dallo snapshot del motore, che ha le slice del chiamante. Se la grammatica cambia dopo la
// compilazione, il suo hash non è più HashSnapshot: Interpreta si ferma con un errore, mai un'interpretazione di
// regole diverse sotto lo stesso ID. Così anche uno snapshot che non è nato da NuovoSnapshot (senza hash).
func TestLoSnapshotCambiatoFermaInterpreta(t *testing.T) {
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, "ACME1111")},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, 8)}, nil,
		[]evidenze.UnitaEvidenza{uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, "ACME1111", 0)})
	uso := usoValutato(selezione("s:corrente", UsoPertinente, "operatore"))

	g := grammaticaACME(famCodice())
	s, err := grammatica.NuovoSnapshot(fileJSON(t, g), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := CompilaVerificato(s, limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	prima := interpreta(t, m, doc, uso)
	if len(prima.Letture) != 1 || len(prima.Letture[0].RuoliCandidati) != 1 {
		t.Fatalf("il documento non esercita i ruoli: %+v", prima.Letture)
	}

	// La slice dei ruoli è quella dello snapshot del chiamante: cambiarla cambia la grammatica del motore.
	ruoli := s.Grammatica.Famiglie[0].Ruoli // {componente, prodotto}, in ordine di byte
	ruoli[len(ruoli)-1] = grammatica.RuoloComponente
	if r, err := m.Interpreta(doc, uso); err == nil || !reflect.DeepEqual(r, Interpretazione{}) {
		t.Fatalf("grammatica cambiata dopo la compilazione: errore %v, risultato %+v", err, r)
	}

	senzaHash, _, err := CompilaVerificato(grammatica.SnapshotRegole{ClienteID: clienteACME, Grammatica: g}, limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := senzaHash.Interpreta(doc, uso); err == nil {
		t.Fatal("uno snapshot senza hash dà un'interpretazione senza identità")
	}
}

// ---- A-C04 dentro Interpreta ----

// TestAC04IlRouterDentroInterpreta (A-C04, riga 6; R48 A): le celle passano dal router come il segmento in cui la
// loro riga si aggancia (l'entità «riga», legami-1); l'uso del segmento e la sua origine restano nella lettura; le
// unità fuori da un messaggio hanno l'uso «non_applicabile» e la loro riga, qualunque sia l'uso dei segmenti; le
// categorie non toccano l'intersezione. Tentativi: un segmento pertinente non porta a «richiesta» un selettore che
// il router non ci manda; s:corrente pertinente non promuove una cella della storia.
func TestAC04IlRouterDentroInterpreta(t *testing.T) {
	m, _ := compilaBene(t, grammaticaRouter())
	corpo := "Serve ACMEPC1001.\nACMEC1002\n> ACMEP1003"
	//         0          1           2   (righe: 0 «Serve ACMEPC1001.», 1 «ACMEC1002», 2 «> ACMEP1003»)
	inizioStoria := strings.Index(corpo, "ACMEC1002")
	doc := docDSint(
		[]evidenze.TestoOriginale{testoOrig(oggettoDSnt, "I: "+testoRouter), testoOrig(corpoDSint, corpo)},
		[]evidenze.Segmento{
			segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, inizioStoria),
			segDSint("s:storia:1", evidenze.SegmentoInoltro, "m1", corpoDSint, inizioStoria, len(corpo)),
		},
		[]evidenze.EntitaLocale{
			entRiga("e:tab:1:r1", "s:storia:1", 1, 1, [2]int{1, 2}),
			entDisegno("e:pdf:p1"), entNodo("e:step:#10", "#10"), entNodo("e:step:#20", "#20"), entIsolata("e:pdf:frammento:1"),
		},
		[]evidenze.UnitaEvidenza{
			uTesto(t, "u:oggetto", "s:corrente", "oggetto", oggettoDSnt, "I: "+testoRouter, 0),
			uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo[:inizioStoria], 0),
			uTesto(t, "u:storia:s:storia:1", "s:storia:1", "storia", corpoDSint, corpo[inizioStoria:], inizioStoria),
			uCella(t, "u:tab:1:r1:c1", "e:tab:1:r1", "storia", "ACMEC1002", 1, 1, 1, [2]int{1, 2},
				&evidenze.Intervallo{Inizio: inizioStoria, Fine: inizioStoria + len("ACMEC1002")}),
			{ID: "u:nome", FonteID: fonteDSint, Selettore: selettore(t, "nome_file"), Testo: testoRouter + ".pdf",
				Posizione: evidenze.Localizzatore{Tipo: "nome_file", NomeFile: &evidenze.PosNomeFile{Campo: "allegato.nome_file",
					Intervallo: evidenze.Intervallo{Inizio: 0, Fine: len(testoRouter) + 4}}},
				Qualita: evidenze.QualitaUnita{Localizzazione: "esatta"}},
			uCampo(t, "u:pdf:cartiglio:codice:1", "e:pdf:p1", "cartiglio.codice", testoRouter),
			uStep(t, "u:step:#10:id", "e:step:#10", "#10", "radice_step.id", "id", testoRouter),
			uStep(t, "u:step:#20:id", "e:step:#20", "#20", "nodo_step.id", "id", testoRouter),
			uCampo(t, "u:pdf:frammento:1", "e:pdf:frammento:1", "testo_pdf", testoRouter),
		})
	doc.Testi = append(doc.Testi, testoOrig("allegato.nome_file", testoRouter+".pdf"))

	ruoliFamiglia := map[string][]grammatica.Ruolo{"acme-pc": ruoliPC, "acme-c": ruoliC, "acme-p": ruoliP}
	// controlla: per ogni lettura dell'unità, funzione, uso, origine e ruoli = famiglia ∩ RuoliAmmessi(funzione).
	controlla := func(t *testing.T, r Interpretazione, unita string, funz Funzione, uso, origine string) {
		t.Helper()
		ls := lettureDellUnita(r, unita)
		if len(ls) == 0 {
			t.Fatalf("%s: nessuna lettura", unita)
		}
		for _, l := range ls {
			if l.Funzione != funz || l.Uso != uso || l.OrigineUso != origine {
				t.Errorf("%s %s: funzione %s, uso %q, origine %q; attesi %s, %q, %q", unita, l.Forma.Famiglia, l.Funzione, l.Uso, l.OrigineUso, funz, uso, origine)
			}
			atteso, _ := intersecaRuoli(ruoliFamiglia[l.Forma.Famiglia], funz)
			if !stessiRuoli(l.RuoliCandidati, atteso) {
				t.Errorf("%s %s: ruoli %v, attesi %v", unita, l.Forma.Famiglia, l.RuoliCandidati, atteso)
			}
			if l.Forma.Famiglia == "acme-c" && (len(l.Categorie) != 1 || l.Categorie[0] != grammatica.CategoriaMinuteria) {
				t.Errorf("%s: la minuteria si copia dalla famiglia come annotazione: %v", unita, l.Categorie)
			}
		}
	}
	fuoriDalMessaggio := func(t *testing.T, r Interpretazione) {
		t.Helper()
		controlla(t, r, "u:nome", FunzIdentitaFile, UsoNonApplicabile, "")
		controlla(t, r, "u:pdf:cartiglio:codice:1", FunzIdentitaFile, UsoNonApplicabile, "")
		controlla(t, r, "u:step:#10:id", FunzIdentitaFile, UsoNonApplicabile, "")
		controlla(t, r, "u:step:#20:id", FunzStruttura, UsoNonApplicabile, "")
		controlla(t, r, "u:pdf:frammento:1", FunzMenzione, UsoNonApplicabile, "")
	}

	t.Run("uso sconosciuto", func(t *testing.T) {
		r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
		controlla(t, r, "u:oggetto", FunzRichiesta, UsoSconosciuto, "")
		controlla(t, r, "u:corpo:s:corrente", FunzRichiesta, UsoSconosciuto, "")
		controlla(t, r, "u:tab:1:r1:c1", FunzMenzione, UsoSconosciuto, "")
		controlla(t, r, "u:storia:s:storia:1", FunzMenzione, UsoSconosciuto, "")
		fuoriDalMessaggio(t, r)
		// Riga 2: la richiesta da un segmento di pertinenza ignota porta la nota, mai la storia.
		note := conCodice(r.Diagnostiche, CodiceMotorePertinenzaIgnota)
		if len(note) == 0 {
			t.Fatalf("richieste con l'uso sconosciuto senza motore.pertinenza_ignota:\n%s", elenco(r.Diagnostiche))
		}
		for _, n := range note {
			if strings.Contains(strings.Join(n.Rif, " "), "storia") {
				t.Errorf("pertinenza ignota su un segmento di storia: %+v", n)
			}
		}
	})

	t.Run("corrente pertinente da operatore, storia pertinente da scenario", func(t *testing.T) {
		r := interpreta(t, m, doc, usoValutato(selezione("s:corrente", UsoPertinente, "operatore"), selezione("s:storia:1", UsoPertinente, "scenario")))
		controlla(t, r, "u:oggetto", FunzRichiesta, UsoPertinente, "operatore")
		controlla(t, r, "u:corpo:s:corrente", FunzRichiesta, UsoPertinente, "operatore")
		controlla(t, r, "u:storia:s:storia:1", FunzRichiesta, UsoPertinente, "scenario")
		controlla(t, r, "u:tab:1:r1:c1", FunzRichiesta, UsoPertinente, "scenario")
		fuoriDalMessaggio(t, r)
		// La richiesta dalla storia resta nel contesto storia (riga 4).
		for _, l := range append(lettureDellUnita(r, "u:storia:s:storia:1"), lettureDellUnita(r, "u:tab:1:r1:c1")...) {
			if l.Forma.Selettore.Contesto != evidenze.ContestoStoria {
				t.Errorf("%s: la richiesta dalla storia ha il contesto %s", l.ID, l.Forma.Selettore.Contesto)
			}
		}
		if n := conCodice(r.Diagnostiche, CodiceMotorePertinenzaIgnota); len(n) != 0 {
			t.Errorf("pertinenza ignota con tutti i segmenti valutati:\n%s", elenco(n))
		}
	})

	t.Run("corrente pertinente: nessuna promozione della storia né dei file", func(t *testing.T) {
		r := interpreta(t, m, doc, usoValutato(selezione("s:corrente", UsoPertinente, "operatore")))
		controlla(t, r, "u:corpo:s:corrente", FunzRichiesta, UsoPertinente, "operatore")
		// La cella e il segmento di storia non hanno una selezione: sconosciuto, menzione (riga 5).
		controlla(t, r, "u:tab:1:r1:c1", FunzMenzione, UsoSconosciuto, "")
		controlla(t, r, "u:storia:s:storia:1", FunzMenzione, UsoSconosciuto, "")
		fuoriDalMessaggio(t, r)
	})

	t.Run("corrente esclusa, storia da valutare", func(t *testing.T) {
		r := interpreta(t, m, doc, usoValutato(selezione("s:corrente", UsoEscluso, "operatore"), selezione("s:storia:1", UsoDaValutare, "riconoscimento")))
		controlla(t, r, "u:oggetto", FunzMenzione, UsoEscluso, "operatore")
		controlla(t, r, "u:corpo:s:corrente", FunzMenzione, UsoEscluso, "operatore")
		controlla(t, r, "u:storia:s:storia:1", FunzMenzione, UsoDaValutare, "riconoscimento")
		controlla(t, r, "u:tab:1:r1:c1", FunzMenzione, UsoDaValutare, "riconoscimento")
		fuoriDalMessaggio(t, r)
	})

	t.Run("corrente da valutare: richiesta con l'incertezza nella lettura", func(t *testing.T) {
		r := interpreta(t, m, doc, usoValutato(selezione("s:corrente", UsoDaValutare, "riconoscimento"), selezione("s:storia:1", UsoEscluso, "operatore")))
		controlla(t, r, "u:corpo:s:corrente", FunzRichiesta, UsoDaValutare, "riconoscimento")
		controlla(t, r, "u:storia:s:storia:1", FunzMenzione, UsoEscluso, "operatore")
		if len(conCodice(r.Diagnostiche, CodiceMotorePertinenzaIgnota)) == 0 {
			t.Fatal("una richiesta da valutare senza motore.pertinenza_ignota")
		}
	})

	t.Run("una cella senza la sua riga non prende l'uso di nessun segmento", func(t *testing.T) {
		// Tentativo: la cella non ha entità (quindi nessuna riga e nessun segmento) ed è agganciata solo per
		// righe, quindi non è la stessa occorrenza della lettura del segmento. Il segmento di storia è pertinente,
		// ma la cella non può prenderne l'uso per vicinanza: resta senza selezione, menzione.
		d := doc
		d.Unita = append([]evidenze.UnitaEvidenza(nil), doc.Unita...)
		for i := range d.Unita {
			if d.Unita[i].ID == "u:tab:1:r1:c1" {
				pt := *d.Unita[i].Posizione.Tabella
				pt.Esatto = nil
				d.Unita[i].Posizione.Tabella = &pt
				d.Unita[i].Qualita.Localizzazione = "parziale"
				d.Unita[i].EntitaID = ""
			}
		}
		r := interpreta(t, m, d, usoValutato(selezione("s:storia:1", UsoPertinente, "scenario")))
		controlla(t, r, "u:tab:1:r1:c1", FunzMenzione, UsoSconosciuto, "")
	})
}

// ---- A-C02 ----

// TestAC02UnAttributoStaConLaSuaEntita (A-C02; P1 §10.5; 5.4.6 punto 12): due disegni con le revisioni in campo
// separato «vicine» al codice dell'altro, nell'ordine delle unità (il testo appiattito): ogni revisione va solo
// alla sua entità, e le letture compatibili sono solo quelle della stessa entità. Tentativi di legame che devono
// fallire: una revisione senza entità, un titolo senza entità, un frammento con «REV», un disegno senza codice,
// una regola di un'altra famiglia, nessuna regola, una regola riservata, un valore fuori regola, una revisione in
// linea di un'altra entità.
func TestAC02UnAttributoStaConLaSuaEntita(t *testing.T) {
	docDisegni := func(t *testing.T, codiceP1, codiceP2 string) evidenze.DocumentoEvidenze {
		// Gli ID sono scelti perché, in ordine, la revisione di p1 (u:04) stia accanto al codice di p2 (u:03) e la
		// revisione di p2 (u:02) accanto al codice di p1 (u:01). Il frammento ripete i due codici con le
		// revisioni scambiate: nel testo appiattito «REV 01» segue il codice di p2.
		return docDSint(nil, nil,
			[]evidenze.EntitaLocale{entDisegno("e:pdf:p1"), entDisegno("e:pdf:p2"), entDisegno("e:pdf:p3"), entIsolata("e:pdf:frammento:1")},
			[]evidenze.UnitaEvidenza{
				uCampo(t, "u:01", "e:pdf:p1", "cartiglio.codice", codiceP1),
				uCampo(t, "u:02", "e:pdf:p2", "cartiglio.revisione", "02"),
				uCampo(t, "u:03", "e:pdf:p2", "cartiglio.codice", codiceP2),
				uCampo(t, "u:04", "e:pdf:p1", "cartiglio.revisione", "01"),
				uCampo(t, "u:05", "e:pdf:p3", "cartiglio.revisione", "05"),
				uCampo(t, "u:06", "", "cartiglio.revisione", "06"),
				uCampo(t, "u:07", "", "cartiglio.titolo", "TITOLO SENZA DISEGNO"),
				uCampo(t, "u:08", "e:pdf:p1", "cartiglio.titolo", "STAFFA ACME"),
				uCampo(t, "u:09", "e:pdf:frammento:1", "testo_pdf", "ACME7002 REV 01 ACME7001 REV 02"),
			},
			capacita("testo", "disponibile"), capacita("cartiglio", "disponibile"))
	}

	t.Run("ogni revisione va alla sua entità", func(t *testing.T) {
		m := motoreCodice(t)
		doc := docDisegni(t, "ACME7001", "ACME7002")
		r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
		l01, l03 := lettureDellUnita(r, "u:01"), lettureDellUnita(r, "u:03")
		if len(l01) != 1 || len(l03) != 1 {
			t.Fatalf("attesa una lettura per codice: %v", idLetture(r.Letture))
		}
		a04 := attributoDellUnita(t, r, AttributoRevisione, "u:04")
		if a04.EntitaID != "e:pdf:p1" || a04.Stato != StatoAttribuito || a04.Normalizzato != "01" || !reflect.DeepEqual(a04.LettureCompatibili, []string{l01[0].ID}) {
			t.Errorf("revisione di p1: %+v; attesa attribuita a p1, compatibile solo con %s", a04, l01[0].ID)
		}
		a02 := attributoDellUnita(t, r, AttributoRevisione, "u:02")
		if a02.EntitaID != "e:pdf:p2" || a02.Stato != StatoAttribuito || a02.Normalizzato != "02" || !reflect.DeepEqual(a02.LettureCompatibili, []string{l03[0].ID}) {
			t.Errorf("revisione di p2: %+v; attesa attribuita a p2, compatibile solo con %s", a02, l03[0].ID)
		}
		// Un disegno senza codici: la sola regola attiva sul selettore (5.4.6 punto 12), nessun legame con il
		// codice di un altro disegno.
		a05 := attributoDellUnita(t, r, AttributoRevisione, "u:05")
		if a05.EntitaID != "e:pdf:p3" || len(a05.LettureCompatibili) != 0 {
			t.Errorf("revisione del disegno senza codici legata a un codice: %+v", a05)
		}
		// Nessun attributo da un'unità senza entità, né dal frammento.
		for _, a := range r.Attributi {
			if a.UnitaID == "u:06" || a.UnitaID == "u:07" || a.UnitaID == "u:09" {
				t.Errorf("attributo da un'unità senza entità o da un frammento: %+v", a)
			}
		}
		if n := len(attributiDi(r, AttributoRevisione)); n != 3 {
			t.Errorf("attese tre revisioni (p1, p2, p3), trovate %d: %+v", n, attributiDi(r, AttributoRevisione))
		}
		tit := attributoDellUnita(t, r, AttributoTitolo, "u:08")
		if tit.EntitaID != "e:pdf:p1" || tit.Normalizzato != "" || len(tit.LettureCompatibili) != 0 {
			t.Errorf("titolo: %+v; atteso il solo grezzo, legato al disegno, senza codici", tit)
		}
		// Le letture del frammento sono menzioni, e nessuna diventa compatibile con una revisione.
		for _, l := range lettureDellUnita(r, "u:09") {
			if l.Funzione != FunzMenzione {
				t.Errorf("lettura del frammento con funzione %s", l.Funzione)
			}
		}
	})

	t.Run("revisione con una regola di un'altra famiglia: nessuna attribuzione", func(t *testing.T) {
		// acme-codice non ha la regola in campo separato; acme-seconda sì, ma nel disegno p1 non c'è nessuna lettura
		// di acme-seconda: la stessa entità permette il legame, nessuna regola lo fa nascere.
		fc := famCodice()
		fc.Revisioni = nil
		fc.Esempi = fc.Esempi[:1]
		seconda := grammatica.FamigliaCodice{
			ID: "acme-seconda", Namespace: "acme-seconda", Ruoli: ruoliPC,
			Base:      base(segLetterale("prefisso", "ACMEB", ""), segPattern("numero", "[0-9]{4}", "")),
			Forme:     []grammatica.FormaCodice{forma("cartiglio", sel("cartiglio.codice"), pBase())},
			Revisioni: []grammatica.RegolaRevisione{revCampo("rev-seconda")},
			Esempi: []grammatica.EsempioCodice{
				positivo("e-seconda", "cartiglio.codice", "ACMEB1234", grammatica.LetturaAttesa{Forma: "cartiglio", Base: "ACMEB1234"}),
				positivo("e-seconda-rev", "cartiglio.revisione", "07", grammatica.LetturaAttesa{Revisione: "07"}),
			},
		}
		m, _ := compilaBene(t, grammaticaACME(fc, seconda))
		r := interpreta(t, m, docDisegni(t, "ACME7001", "ACME7002"), evidenze.UsoSconosciuto(bundleDSint))
		for _, unita := range []string{"u:04", "u:02"} {
			for _, a := range r.Attributi {
				if a.UnitaID != unita {
					continue
				}
				if a.Stato == StatoAttribuito || a.Normalizzato != "" || len(a.LettureCompatibili) != 0 {
					t.Errorf("revisione di %s attribuita senza la regola della famiglia letta: %+v", unita, a)
				}
			}
		}
	})

	// Riscritta per la correzione di A1b.10 (C-34; R21 e = A): senza nessuna regola sul selettore la revisione
	// del cartiglio non è assente, è non_interpretabile con l'originale conservato. Prima la prova chiedeva
	// nessun attributo, contro C-34 e il 5.4.6 punto 12.
	t.Run("nessuna regola in campo separato: non interpretabile con l'originale, e il campo si dice non letto", func(t *testing.T) {
		fc := famCodice()
		fc.Revisioni = nil
		fc.Esempi = fc.Esempi[:1]
		m, _ := compilaBene(t, grammaticaACME(fc))
		r := interpreta(t, m, docDisegni(t, "ACME7001", "ACME7002"), evidenze.UsoSconosciuto(bundleDSint))
		// u:02, u:04 e u:05 hanno un'entità; u:06 no, e senza entità non c'è attributo (A-C02).
		if a := attributiDi(r, AttributoRevisione); len(a) != 3 {
			t.Fatalf("attese 3 revisioni non interpretabili, una per campo con un'entità: %+v", a)
		}
		for u, grezzo := range map[string]string{"u:02": "02", "u:04": "01", "u:05": "05"} {
			a := attributoDellUnita(t, r, AttributoRevisione, u)
			if a.Stato != StatoNonInterpretabile || a.Normalizzato != "" || a.Grezzo != grezzo || len(a.LettureCompatibili) != 0 || a.ID != "a:"+u+":revisione" {
				t.Errorf("senza regola, %s: %+v; atteso non_interpretabile con l'originale, senza valore né legami", u, a)
			}
		}
		if n := len(conCodice(r.Diagnostiche, CodiceRevisioneNonInterpretabile)); n != 3 {
			t.Errorf("revisione.non_interpretabile: %d, attese 3:\n%s", n, elenco(r.Diagnostiche))
		}
		// Il titolo non è una revisione: resta il grezzo, e nessun default prudente lo tocca.
		if a := attributoDellUnita(t, r, AttributoTitolo, "u:08"); a.Stato != StatoAttribuito {
			t.Errorf("il titolo: %+v", a)
		}
		trovata := false
		for _, d := range conCodice(r.Diagnostiche, grammatica.CodiceCapacitaNonSupportata) {
			if strings.Contains(d.Percorso, "cartiglio.revisione") {
				trovata = true
				if strings.Join(d.Rif, ",") != "u:02,u:04,u:05,u:06" {
					t.Errorf("la nota del campo non letto ha le unità %v", d.Rif)
				}
			}
		}
		if !trovata {
			t.Fatalf("il campo revisione ricevuto e non letto non si dice:\n%s", elenco(r.Diagnostiche))
		}
	})

	t.Run("regola riservata o valore fuori regola: non interpretabile, originale conservato", func(t *testing.T) {
		fc := famCodice()
		fc.Revisioni[0].Stato = grammatica.StatoRiservata
		m, _, err := compila(t, grammaticaACME(fc), limitiACME())
		if err != nil || m == nil {
			t.Fatalf("la grammatica con la revisione riservata non compila: %v", err)
		}
		r := interpreta(t, m, docDisegni(t, "ACME7001", "ACME7002"), evidenze.UsoSconosciuto(bundleDSint))
		a := attributoDellUnita(t, r, AttributoRevisione, "u:04")
		if a.Stato != StatoNonInterpretabile || a.Normalizzato != "" || a.Grezzo != "01" || len(a.LettureCompatibili) != 0 {
			t.Errorf("con la regola riservata (Q1): %+v; atteso non_interpretabile, senza valore né legami", a)
		}
		if len(conCodice(r.Diagnostiche, CodiceRevisioneNonInterpretabile)) == 0 {
			t.Errorf("manca revisione.non_interpretabile:\n%s", elenco(r.Diagnostiche))
		}

		m2 := motoreCodice(t)
		doc := docDisegni(t, "ACME7001", "ACME7002")
		doc.Unita[3].Testo = "Rev. 01 del 2019"
		r2 := interpreta(t, m2, doc, evidenze.UsoSconosciuto(bundleDSint))
		a2 := attributoDellUnita(t, r2, AttributoRevisione, "u:04")
		if a2.Stato != StatoNonInterpretabile || a2.Normalizzato != "" || a2.Grezzo != "Rev. 01 del 2019" || len(a2.LettureCompatibili) != 0 {
			t.Errorf("valore che la regola non legge per intero: %+v", a2)
		}
		if len(conCodice(r2.Diagnostiche, CodiceRevisioneNonInterpretabile)) == 0 {
			t.Errorf("manca revisione.non_interpretabile:\n%s", elenco(r2.Diagnostiche))
		}
	})

	t.Run("regola attiva e riservata della stessa famiglia: vale l'attiva, un esito solo", func(t *testing.T) {
		// Q1 dà non_interpretabile con la regola riservata che si applica; con un'attiva della stessa famiglia
		// sullo stesso selettore si applica l'attiva (5.4.6 punto 12, primo trattino).
		fc := famCodice()
		riservata := revCampo("rev-campo-riservata")
		riservata.Stato = grammatica.StatoRiservata
		fc.Revisioni = append(fc.Revisioni, riservata)
		m, _, err := compila(t, grammaticaACME(fc), limitiACME())
		if err != nil || m == nil {
			t.Fatalf("la grammatica con un'attiva e una riservata non compila: %v", err)
		}
		r := interpreta(t, m, docDisegni(t, "ACME7001", "ACME7002"), evidenze.UsoSconosciuto(bundleDSint))
		var di04 []AttributoLetto
		for _, a := range attributiDi(r, AttributoRevisione) {
			if a.UnitaID == "u:04" {
				di04 = append(di04, a)
			}
		}
		if len(di04) != 1 || di04[0].Stato != StatoAttribuito || di04[0].Normalizzato != "01" {
			t.Errorf("revisione di p1 con un'attiva e una riservata: %+v; atteso un solo attributo, attribuito", di04)
		}
		if d := conCodice(r.Diagnostiche, CodiceRevisioneNonInterpretabile); len(d) != 0 {
			t.Errorf("revisione.non_interpretabile accanto all'attiva:\n%s", elenco(d))
		}
	})

	t.Run("la discordanza si cerca solo nella stessa entità", func(t *testing.T) {
		fc := famCodice()
		fc.Forme = append(fc.Forme, forma("con-revisione", sel("testo_pdf"), pBase(), pRif(grammatica.TipoParteRevisione, "rev-linea", 1)))
		fc.Forme[0].Selettori = sel("corpo", "cartiglio.codice")
		fc.Revisioni = append(fc.Revisioni, grammatica.RegolaRevisione{
			ID: "rev-linea", Selettori: sel("testo_pdf"), Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
			Separatori: []string{"/"},
			Segmenti:   []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]{2}", Significato: grammatica.SignificatoNessuno}},
		})
		fc.Esempi = append(fc.Esempi, positivo("e-linea", "testo_pdf", "ACME1111/03", grammatica.LetturaAttesa{Forma: "con-revisione", Base: "ACME1111", Revisione: "03"}))
		m, _ := compilaBene(t, grammaticaACME(fc))
		// p1: codice «ACME7001» nel cartiglio, campo «01». p2: codice e revisione in linea «/03» in un'unità
		// testo_pdf della STESSA entità p2, campo «03». Nessuna discordanza: la revisione in linea di p2 non si
		// confronta con il campo di p1.
		doc := docDSint(nil, nil, []evidenze.EntitaLocale{entDisegno("e:pdf:p1"), entDisegno("e:pdf:p2")},
			[]evidenze.UnitaEvidenza{
				uCampo(t, "u:01", "e:pdf:p1", "cartiglio.codice", "ACME7001"),
				uCampo(t, "u:02", "e:pdf:p1", "cartiglio.revisione", "01"),
				uCampo(t, "u:03", "e:pdf:p2", "testo_pdf", "ACME7002/03"),
				uCampo(t, "u:04", "e:pdf:p2", "cartiglio.revisione", "03"),
			})
		r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
		if d := conCodice(r.Diagnostiche, CodiceRevisioneDiscordante); len(d) != 0 {
			t.Fatalf("discordanza fra due entità diverse:\n%s", elenco(d))
		}
		// p2 con il campo «04» e la revisione in linea «03» della stessa entità: discordante, e si conservano tutte
		// e due (P1 §5.2).
		doc.Unita[3].Testo = "04"
		r = interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
		d := conCodice(r.Diagnostiche, CodiceRevisioneDiscordante)
		if len(d) != 1 {
			t.Fatalf("attesa una revisione.discordante per p2:\n%s", elenco(r.Diagnostiche))
		}
		a := attributoDellUnita(t, r, AttributoRevisione, "u:04")
		l := lettureDellUnita(r, "u:03")
		if len(l) != 1 || l[0].Forma.Revisione == nil || l[0].Forma.Revisione.Normalizzata != "03" || a.Normalizzato != "04" || a.Grezzo != "04" {
			t.Fatalf("le due revisioni non si conservano: attributo %+v, letture %v", a, idLetture(l))
		}
		rif := strings.Join(d[0].Rif, " ")
		if !strings.Contains(rif, a.ID) || !strings.Contains(rif, l[0].ID) || strings.Contains(rif, "u:01") {
			t.Errorf("la discordanza cita %v", d[0].Rif)
		}
		for _, x := range attributiDi(r, AttributoRevisione) {
			if x.EntitaID == "e:pdf:p1" && x.Normalizzato != "01" {
				t.Errorf("la revisione di p1 cambia per p2: %+v", x)
			}
		}
	})
}

// ---- la deduplica ----

// TestLaDeduplicaSoloDellaStessaOccorrenza (A-C10; P1 §7.3; 5.4.6 punto 9): una cella esatta e il segmento che la
// contiene danno una lettura sola, con l'altra unità in AltreUnita; una cella agganciata per righe si fonde solo
// con la lettura del suo segmento che cade dentro le sue righe. Tentativi che devono restare separati: una lettura
// del segmento fuori dalle righe della cella, una lettura di un altro segmento sulle stesse righe, due celle con lo
// stesso codice sulla stessa riga (scegliere sarebbe indovinare per ordine), lo stesso intervallo su un altro
// testo, lo stesso codice in due unità.
func TestLaDeduplicaSoloDellaStessaOccorrenza(t *testing.T) {
	m := motoreCodice(t)
	corpo := "Elenco:\nACME1111\tStaffa\t5\nNota ACME1111 fuori\nACME2222\tACME2222\t3"
	riga1 := strings.Index(corpo, "ACME1111")
	doc := docDSint(
		[]evidenze.TestoOriginale{testoOrig(oggettoDSnt, "ACME3333"), testoOrig(corpoDSint, corpo)},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, len(corpo))},
		[]evidenze.EntitaLocale{
			entRiga("e:tab:1:r1", "s:corrente", 1, 1, [2]int{1, 2}),
			entRiga("e:tab:1:r2", "s:corrente", 1, 2, [2]int{3, 4}),
		},
		[]evidenze.UnitaEvidenza{
			uTesto(t, "u:oggetto", "s:corrente", "oggetto", oggettoDSnt, "ACME3333", 0),
			uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo, 0),
			uCella(t, "u:tab:1:r1:c1", "e:tab:1:r1", "corpo", "ACME1111", 1, 1, 1, [2]int{1, 2}, nil),
			uCella(t, "u:tab:1:r1:c2", "e:tab:1:r1", "corpo", "Staffa", 1, 1, 2, [2]int{1, 2}, nil),
			uCella(t, "u:tab:1:r2:c1", "e:tab:1:r2", "corpo", "ACME2222", 1, 2, 1, [2]int{3, 4}, nil),
			uCella(t, "u:tab:1:r2:c2", "e:tab:1:r2", "corpo", "ACME2222", 1, 2, 2, [2]int{3, 4}, nil),
		})

	t.Run("per righe: solo dentro le righe, solo con una candidata", func(t *testing.T) {
		r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
		c1 := lettureDellUnita(r, "u:tab:1:r1:c1")
		if len(c1) != 1 || !reflect.DeepEqual(c1[0].AltreUnita, []string{"u:corpo:s:corrente"}) {
			t.Fatalf("la cella per righe e la lettura del segmento dentro la sua riga sono una lettura sola: %+v", c1)
		}
		// La lettura del segmento sulla riga 2 («Nota ACME1111 fuori») è un'altra occorrenza: resta, da sola.
		seg := lettureDellUnita(r, "u:corpo:s:corrente")
		var fuori, riga3 []LetturaCodice
		for _, l := range seg {
			switch {
			case l.Forma.Originale == "ACME1111":
				fuori = append(fuori, l)
			case l.Forma.Originale == "ACME2222":
				riga3 = append(riga3, l)
			}
		}
		if len(fuori) != 1 || fuori[0].Occorrenza.Inizio <= riga1 || len(fuori[0].AltreUnita) != 0 {
			t.Fatalf("la lettura del segmento fuori dalle righe della cella si è fusa o persa: %+v", fuori)
		}
		// Due celle con lo stesso codice sulla stessa riga e due letture del segmento: ogni cella ha due
		// candidate, e la coppia si sceglierebbe per ordine. Restano quattro letture, nessuna fusa.
		if len(riga3) != 2 {
			t.Fatalf("le due letture del segmento sulla riga con due celle uguali: %+v", riga3)
		}
		for _, l := range append(riga3, append(lettureDellUnita(r, "u:tab:1:r2:c1"), lettureDellUnita(r, "u:tab:1:r2:c2")...)...) {
			if len(l.AltreUnita) != 0 {
				t.Errorf("fusione indovinata per ordine: %s con %v", l.ID, l.AltreUnita)
			}
		}
		if n := len(r.Letture); n != 7 {
			t.Errorf("attese 7 letture (oggetto, cella r1 fusa, segmento fuori riga, 2 del segmento e 2 celle su r2), trovate %d: %v", n, idLetture(r.Letture))
		}
		// Nessuna fusione con l'oggetto, che ha un altro testo.
		if o := lettureDellUnita(r, "u:oggetto"); len(o) != 1 || len(o[0].AltreUnita) != 0 {
			t.Errorf("oggetto: %+v", o)
		}
	})

	t.Run("per righe: un altro segmento sulle stesse righe non si fonde", func(t *testing.T) {
		d := doc
		d.Segmenti = append(append([]evidenze.Segmento(nil), doc.Segmenti...), segDSint("s:storia:1", evidenze.SegmentoCitazione, "m1", corpoDSint, len(corpo), len(corpo)))
		d.Entita = append([]evidenze.EntitaLocale(nil), doc.Entita...)
		d.Entita[0].SegmentoID = "s:storia:1" // la riga dice di stare in un altro segmento
		r := interpreta(t, m, d, evidenze.UsoSconosciuto(bundleDSint))
		for _, l := range r.Letture {
			if len(l.AltreUnita) != 0 && l.UnitaID == "u:tab:1:r1:c1" {
				t.Fatalf("una cella della storia si fonde con una lettura del segmento corrente: %+v", l)
			}
		}
	})

	t.Run("esatta: cella e segmento sono una lettura; lo stesso intervallo su un altro testo no", func(t *testing.T) {
		corpo2 := "ACME3333\r\nACME3333"
		d := docDSint(
			[]evidenze.TestoOriginale{testoOrig(oggettoDSnt, "ACME3333"), testoOrig(corpoDSint, corpo2)},
			[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, len(corpo2))},
			[]evidenze.EntitaLocale{entRiga("e:tab:1:r1", "s:corrente", 1, 1, [2]int{0, 1}), entRiga("e:tab:1:r2", "s:corrente", 1, 2, [2]int{1, 2})},
			[]evidenze.UnitaEvidenza{
				uTesto(t, "u:oggetto", "s:corrente", "oggetto", oggettoDSnt, "ACME3333", 0),
				uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo2, 0),
				uCella(t, "u:tab:1:r1:c1", "e:tab:1:r1", "corpo", "ACME3333", 1, 1, 1, [2]int{0, 1}, &evidenze.Intervallo{Inizio: 0, Fine: 8}),
				uCella(t, "u:tab:1:r2:c1", "e:tab:1:r2", "corpo", "ACME3333", 1, 2, 1, [2]int{1, 2}, &evidenze.Intervallo{Inizio: 10, Fine: 18}),
			})
		r := interpreta(t, m, d, evidenze.UsoSconosciuto(bundleDSint))
		// Due righe con lo stesso codice: due letture, ciascuna con il segmento in AltreUnita; l'oggetto (stesso
		// intervallo [0,8), ma su un altro testo) resta da solo.
		for _, c := range []string{"u:tab:1:r1:c1", "u:tab:1:r2:c1"} {
			l := lettureDellUnita(r, c)
			if len(l) != 1 || !reflect.DeepEqual(l[0].AltreUnita, []string{"u:corpo:s:corrente"}) || l[0].Assoluto == nil {
				t.Fatalf("%s: %+v", c, l)
			}
		}
		if s := lettureDellUnita(r, "u:corpo:s:corrente"); len(s) != 0 {
			t.Fatalf("le letture del segmento esatte con le celle restano doppie: %v", idLetture(s))
		}
		if o := lettureDellUnita(r, "u:oggetto"); len(o) != 1 || len(o[0].AltreUnita) != 0 {
			t.Fatalf("lo stesso intervallo su un altro testo si è fuso: %+v", o)
		}
		if len(r.Letture) != 3 {
			t.Fatalf("attese tre letture: %v", idLetture(r.Letture))
		}
	})

	t.Run("lo stesso codice in due unità diverse resta due letture", func(t *testing.T) {
		d := docDSint(nil, nil, []evidenze.EntitaLocale{entDisegno("e:pdf:p1"), entNodo("e:step:#10", "#10"), entIsolata("e:pdf:frammento:1")},
			[]evidenze.UnitaEvidenza{
				uCampo(t, "u:pdf:cartiglio:codice:1", "e:pdf:p1", "cartiglio.codice", "ACME4444"),
				uCampo(t, "u:pdf:frammento:1", "e:pdf:frammento:1", "testo_pdf", "ACME4444"),
				uStep(t, "u:step:#10:id", "e:step:#10", "#10", "radice_step.id", "id", "ACME4444"),
			})
		r := interpreta(t, m, d, evidenze.UsoSconosciuto(bundleDSint))
		if len(r.Letture) != 3 {
			t.Fatalf("deduplica per sola stringa: %v", idLetture(r.Letture))
		}
		for _, l := range r.Letture {
			if len(l.AltreUnita) != 0 {
				t.Errorf("%s fusa con %v", l.ID, l.AltreUnita)
			}
		}
	})
}

// ---- A1b-21: le ambiguità ----

// TestLeAmbiguitaRestanoConLaDiagnostica (A1b-21; 5.4.6 punti 10 e 16; R25 e, f; R32 b): due famiglie sullo stesso
// tratto danno tutte e due le letture con motore.letture_alternative; una lettura contenuta in un'altra della
// stessa famiglia resta, con motore.letture_annidate; un token senza famiglia non dà letture, né menzioni generiche;
// un grafo di completezza non determinabile non inventa completamenti né rende parziale lo stato.
func TestLeAmbiguitaRestanoConLaDiagnostica(t *testing.T) {
	famA := grammatica.FamigliaCodice{
		ID: "acme-a", Namespace: "acme-a", Ruoli: ruoliPC,
		Base:   base(segLetterale("prefisso", "ACME", ""), segPattern("numero", "[0-9]{4}", "")),
		Forme:  []grammatica.FormaCodice{forma("a", sel("corpo", "oggetto"), pBase())},
		Esempi: []grammatica.EsempioCodice{positivo("e-a", "oggetto", "ACME2222", grammatica.LetturaAttesa{Forma: "a", Base: "ACME2222"})},
	}
	famB := grammatica.FamigliaCodice{
		ID: "acme-b", Namespace: "acme-b", Ruoli: ruoliC,
		Base:   base(segLetterale("prefisso", "ACME1", ""), segPattern("numero", "[0-9]{3}", "")),
		Forme:  []grammatica.FormaCodice{forma("b", sel("corpo", "storia"), pBase())},
		Esempi: []grammatica.EsempioCodice{positivo("e-b", "storia", "ACME1234", grammatica.LetturaAttesa{Forma: "b", Base: "ACME1234"})},
	}
	famN := grammatica.FamigliaCodice{
		ID: "acme-annidata", Namespace: "acme-annidata", Ruoli: ruoliPC,
		Base: base(segLetterale("prefisso", "ACMEZ", ""), segPattern("numero", "[0-9]{4}", "")),
		Forme: []grammatica.FormaCodice{
			forma("corta", sel("corpo", "oggetto"), pBase()),
			forma("lunga", sel("corpo", "storia"), pSep("X-"), pBase()),
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-corta", "oggetto", "ACMEZ2222", grammatica.LetturaAttesa{Forma: "corta", Base: "ACMEZ2222"}),
			positivo("e-lunga", "storia", "X-ACMEZ2222", grammatica.LetturaAttesa{Forma: "lunga", Base: "ACMEZ2222"}),
		},
	}
	m, _ := compilaBene(t, grammaticaACME(famA, famB, famN))
	corpo := "ACME1111 X-ACMEZ3333 zz-9999 QX12345 acme1111 ACME 1111"
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, corpo)},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, len(corpo))}, nil,
		[]evidenze.UnitaEvidenza{uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo, 0)},
		capacita("firma", "non_disponibile"))
	r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))

	var a, b, corta, lunga []LetturaCodice
	for _, l := range r.Letture {
		switch l.Forma.Famiglia + "/" + l.Forma.Forma {
		case "acme-a/a":
			a = append(a, l)
		case "acme-b/b":
			b = append(b, l)
		case "acme-annidata/corta":
			corta = append(corta, l)
		case "acme-annidata/lunga":
			lunga = append(lunga, l)
		default:
			t.Errorf("lettura inattesa: %s", l.ID)
		}
	}
	if len(a) != 1 || len(b) != 1 || a[0].Occorrenza != b[0].Occorrenza {
		t.Fatalf("due famiglie sullo stesso tratto: attese tutte e due le letture: %v", idLetture(r.Letture))
	}
	alt := conCodice(r.Diagnostiche, CodiceMotoreLettureAlternative)
	if len(alt) != 1 || !reflect.DeepEqual(sortCopia(alt[0].Rif), sortCopia([]string{a[0].ID, b[0].ID})) {
		t.Fatalf("motore.letture_alternative: %+v", alt)
	}
	if alt[0].Gravita != evidenze.GravitaAvviso || alt[0].Natura != evidenze.NaturaDati {
		t.Errorf("letture alternative: gravità %s, natura %s; attesi avviso, dati (5.4.9)", alt[0].Gravita, alt[0].Natura)
	}
	// I ruoli restano quelli di ciascuna famiglia: nessuna scelta fra le due.
	if !stessiRuoli(a[0].RuoliCandidati, ruoliP) || len(b[0].RuoliCandidati) != 0 {
		t.Errorf("ruoli delle alternative: %v, %v", a[0].RuoliCandidati, b[0].RuoliCandidati)
	}
	if len(corta) != 1 || len(lunga) != 1 {
		t.Fatalf("le letture annidate della stessa famiglia restano tutte e due (R25 f): %v", idLetture(r.Letture))
	}
	ann := conCodice(r.Diagnostiche, CodiceMotoreLettureAnnidate)
	if len(ann) != 1 || !reflect.DeepEqual(sortCopia(ann[0].Rif), sortCopia([]string{corta[0].ID, lunga[0].ID})) {
		t.Fatalf("motore.letture_annidate: %+v", ann)
	}
	// Un token senza famiglia («zz-9999», «QX12345», «acme1111» in minuscolo, «ACME 1111» con lo spazio) non dà
	// letture (R25 e = A), e non nasce nessun codice «non riconosciuto».
	if len(r.Letture) != 4 {
		t.Errorf("attese 4 letture, trovate %d: %v", len(r.Letture), idLetture(r.Letture))
	}
	for _, d := range r.Diagnostiche {
		if strings.HasPrefix(d.Codice, "codice.") || strings.Contains(d.Codice, "non_riconosciuto") {
			t.Errorf("diagnostica di un riconoscitore generico: %+v", d)
		}
	}
	can := canonico(t, r)
	if bytes.Contains(can, []byte(`"menzioni"`)) || bytes.Contains(can, []byte(`"riferimenti"`)) {
		t.Errorf("l'interpretazione ha Menzioni o Riferimenti: in A1 non nascono (R25 e, R20 c)")
	}
	// La firma non disponibile non rende parziale una mail (5.4.6 punto 16).
	if r.Stato != StatoInterpretazioneCompleta {
		t.Errorf("stato %s: la capacità firma non conta", r.Stato)
	}

	t.Run("completezza del grafo non determinabile", func(t *testing.T) {
		mc := motoreCodice(t)
		d := docDSint(nil, nil, []evidenze.EntitaLocale{entNodo("e:step:#10", "#10"), entNodo("e:step:#20", "#20")},
			[]evidenze.UnitaEvidenza{
				uStep(t, "u:step:#10:id", "e:step:#10", "#10", "radice_step.id", "id", "ACME5555"),
				uStep(t, "u:step:#20:id", "e:step:#20", "#20", "nodo_step.id", "id", "ACME6666"),
			},
			capacita("struttura", "disponibile"), evidenze.Capacita{Nome: "grafo_completo", Stato: "non_disponibile", Motivo: "non determinabile"})
		d.Legami = []evidenze.LegameFonte{{ID: "l:step:#10>#20", Tipo: "padre_figlio_step", Da: "e:step:#10", A: "e:step:#20", Quantita: func() *int { n := 2; return &n }()}}
		r := interpreta(t, mc, d, evidenze.UsoSconosciuto(bundleDSint))
		if r.Stato != StatoInterpretazioneCompleta {
			t.Errorf("stato %s: grafo_completo è qualità della fonte, non cambia lo stato (R32 b)", r.Stato)
		}
		if len(r.Relazioni) != 0 || len(r.Attributi) != 0 {
			t.Errorf("completamenti inventati: relazioni %+v, attributi %+v", r.Relazioni, r.Attributi)
		}
	})
}

func sortCopia(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// ---- A1b-22: i limiti ----

// TestOltreIlLimiteIlRisultatoEParziale (A1b-22; P1 §7.5; R43 B): oltre max_letture_documento, max_byte_unita o
// max_unita_documento si ha limite.superato, stato parziale, e le letture già fatte restano: mai un successo
// vuoto. I limiti arrivano come li darebbe l'indice, sotto i tetti del codice.
func TestOltreIlLimiteIlRisultatoEParziale(t *testing.T) {
	corpo := "ACME1111 ACME2222 ACME3333"
	altro := "ACME4444"
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(oggettoDSnt, altro), testoOrig(corpoDSint, corpo)},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, len(corpo))}, nil,
		[]evidenze.UnitaEvidenza{
			uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo, 0),
			uTesto(t, "u:oggetto", "s:corrente", "oggetto", oggettoDSnt, altro, 0),
		})
	tetti := grammatica.TettiLimiti().Riconoscimento
	casi := []struct {
		nome   string
		cambia func(*grammatica.LimitiRiconoscimento)
		minimo int // letture che devono restare
	}{
		{"max_letture_documento", func(l *grammatica.LimitiRiconoscimento) { l.MaxLettureDocumento = 2 }, 2},
		{"max_byte_unita", func(l *grammatica.LimitiRiconoscimento) { l.MaxByteUnita = 16 }, 1},
		{"max_unita_documento", func(l *grammatica.LimitiRiconoscimento) { l.MaxUnitaDocumento = 1 }, 1},
		{"max_letture_per_unita", func(l *grammatica.LimitiRiconoscimento) { l.MaxLetturePerUnita = 1 }, 1},
	}
	pieno := interpreta(t, motoreCodice(t), doc, evidenze.UsoSconosciuto(bundleDSint))
	if pieno.Stato != StatoInterpretazioneCompleta || len(pieno.Letture) != 4 || len(conCodice(pieno.Diagnostiche, grammatica.CodiceLimiteSuperato)) != 0 {
		t.Fatalf("senza limiti stretti: stato %s, %d letture", pieno.Stato, len(pieno.Letture))
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			lim := limitiACME()
			c.cambia(&lim.Riconoscimento)
			if lim.Riconoscimento.MaxByteUnita > tetti.MaxByteUnita || lim.Riconoscimento.MaxLettureDocumento > tetti.MaxLettureDocumento {
				t.Fatal("i limiti della prova devono stare sotto i tetti del codice")
			}
			m, d, err := compila(t, grammaticaACME(famCodice()), lim)
			if err != nil {
				t.Fatalf("la grammatica non compila con i limiti dati: %v\n%s", err, elenco(d))
			}
			r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
			sup := conCodice(r.Diagnostiche, grammatica.CodiceLimiteSuperato)
			if len(sup) == 0 {
				t.Fatalf("nessun limite.superato:\n%s", elenco(r.Diagnostiche))
			}
			for _, x := range sup {
				if x.Natura != evidenze.NaturaLimite || x.Gravita == evidenze.GravitaErrore {
					t.Errorf("limite.superato con natura %s e gravità %s", x.Natura, x.Gravita)
				}
			}
			if r.Stato != StatoInterpretazioneParziale {
				t.Errorf("stato %s oltre un limite, atteso parziale", r.Stato)
			}
			if len(r.Letture) < c.minimo || len(r.Letture) >= len(pieno.Letture) {
				t.Errorf("%d letture oltre il limite: le già fatte restano, e non sono tutte", len(r.Letture))
			}
			// Ogni lettura del risultato parziale è una lettura del risultato pieno: niente inventato dal taglio.
			for _, l := range r.Letture {
				trovata := false
				for _, p := range pieno.Letture {
					if p.ID == l.ID {
						trovata = true
					}
				}
				if !trovata {
					t.Errorf("lettura inventata dal limite: %s", l.ID)
				}
			}
			if r.ImprontaLimiti == pieno.ImprontaLimiti || r.ID == pieno.ID {
				t.Error("limiti diversi, stessa identità (R43 B)")
			}
		})
	}
}

// TestOltreIlLimiteNessunAttributoSuLettureTagliate (A1b-22; 5.4.6 punti 2 e 12; 5.0): la revisione in campo
// separato si attribuisce con la regola della famiglia letta nella stessa entità. Se il limite taglia proprio la
// lettura del codice del disegno, l'entità non è letta per intero: la regola dell'altra famiglia («senza letture
// nell'entità, l'unica attiva») non deve attribuire il campo. A lettura completa il campo è non_attribuito; oltre il
// limite non nasce nessun attributo, e limite.superato nomina il disegno.
func TestOltreIlLimiteNessunAttributoSuLettureTagliate(t *testing.T) {
	senzaRev := grammatica.FamigliaCodice{
		ID: "acme-senza-rev", Namespace: "acme-senza-rev", Ruoli: ruoliPC,
		Base:   base(segLetterale("prefisso", "ACMEB", ""), segPattern("numero", "[0-9]{4}", "")),
		Forme:  []grammatica.FormaCodice{forma("cartiglio", sel("cartiglio.codice"), pBase())},
		Esempi: []grammatica.EsempioCodice{positivo("e-senza-rev", "cartiglio.codice", "ACMEB1234", grammatica.LetturaAttesa{Forma: "cartiglio", Base: "ACMEB1234"})},
	}
	corpo := "ACME1111 ACME2222 ACME3333"
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, corpo)},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, len(corpo))},
		[]evidenze.EntitaLocale{entDisegno("e:pdf:p1")},
		[]evidenze.UnitaEvidenza{
			uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo, 0),
			uCampo(t, "u:pdf:cartiglio:codice:1", "e:pdf:p1", "cartiglio.codice", "ACMEB1234"),
			uCampo(t, "u:pdf:cartiglio:revisione:1", "e:pdf:p1", "cartiglio.revisione", "04"),
		})
	g := grammaticaACME(famCodice(), senzaRev)

	m, _ := compilaBene(t, g)
	pieno := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
	rev := attributiDi(pieno, AttributoRevisione)
	if len(rev) != 1 || rev[0].Stato != StatoNonAttribuito {
		t.Fatalf("a lettura completa la famiglia del disegno non ha regola: atteso non_attribuito, %+v", rev)
	}

	lim := limitiACME()
	lim.Riconoscimento.MaxLettureDocumento = 2
	mt, d, err := compila(t, g, lim)
	if err != nil {
		t.Fatalf("la grammatica non compila con i limiti dati: %v\n%s", err, elenco(d))
	}
	r := interpreta(t, mt, doc, evidenze.UsoSconosciuto(bundleDSint))
	if len(lettureDellUnita(r, "u:pdf:cartiglio:codice:1")) != 0 {
		t.Fatalf("la prova vuole il codice del disegno tagliato dal limite: %v", idLetture(r.Letture))
	}
	if a := attributiDi(r, AttributoRevisione); len(a) != 0 {
		t.Errorf("revisione attribuita su un'entità non letta per intero: %+v", a)
	}
	nominato := false
	for _, x := range conCodice(r.Diagnostiche, grammatica.CodiceLimiteSuperato) {
		nominato = nominato || (x.Percorso == "attributi" && reflect.DeepEqual(x.Rif, []string{"e:pdf:p1"}))
	}
	if !nominato || r.Stato != StatoInterpretazioneParziale {
		t.Errorf("stato %s; limite.superato deve nominare il disegno:\n%s", r.Stato, elenco(r.Diagnostiche))
	}
}

// ---- lo stato ----

// TestLoStatoContaSoloLeCapacitaDiLettura (5.4.6 punto 16; A-C09; R32 b): completa con le capacità di lettura
// disponibili, anche con zero letture e con firma, segmentazione, storia annidata, tabelle, elenco PDF e grafo
// completo assenti; parziale con una capacità di lettura parziale o assente, o con un'unità troncata dal worker;
// non disponibile senza unità leggibili.
func TestLoStatoContaSoloLeCapacitaDiLettura(t *testing.T) {
	m := motoreCodice(t)
	unita := func(t *testing.T) []evidenze.UnitaEvidenza {
		return []evidenze.UnitaEvidenza{uCampo(t, "u:pdf:frammento:1", "e:pdf:frammento:1", "testo_pdf", "nessun codice qui")}
	}
	ent := []evidenze.EntitaLocale{entIsolata("e:pdf:frammento:1")}
	nonContano := []evidenze.Capacita{
		capacita("firma", "non_disponibile"), capacita("sezione_tecnica", "non_disponibile"), capacita("storia_annidata", "non_disponibile"),
		capacita("segmentazione", "parziale"), capacita("tabelle", "parziale"), capacita("elenco_pdf", "non_disponibile"),
		capacita("grafo_completo", "non_disponibile"),
	}
	casi := []struct {
		nome     string
		capacita []evidenze.Capacita
		troncata bool
		vuota    bool
		stato    string
	}{
		{"tutto disponibile, zero letture", append([]evidenze.Capacita{capacita("testo", "disponibile"), capacita("cartiglio", "disponibile")}, nonContano...), false, false, StatoInterpretazioneCompleta},
		{"testo assente", append([]evidenze.Capacita{capacita("testo", "non_disponibile"), capacita("cartiglio", "disponibile")}, nonContano...), false, false, StatoInterpretazioneParziale},
		{"cartiglio parziale", []evidenze.Capacita{capacita("testo", "disponibile"), capacita("cartiglio", "parziale")}, false, false, StatoInterpretazioneParziale},
		{"struttura assente", []evidenze.Capacita{capacita("struttura", "non_disponibile")}, false, false, StatoInterpretazioneParziale},
		{"contenuto assente", []evidenze.Capacita{capacita("contenuto", "non_disponibile")}, false, false, StatoInterpretazioneParziale},
		{"unità troncata dal worker", []evidenze.Capacita{capacita("testo", "disponibile")}, true, false, StatoInterpretazioneParziale},
		{"nessuna unità leggibile", []evidenze.Capacita{capacita("testo", "non_disponibile")}, false, true, StatoInterpretazioneNonDisponibile},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			u := unita(t)
			u[0].Qualita.Troncata = c.troncata
			if c.vuota {
				u = nil
			}
			r := interpreta(t, m, docDSint(nil, nil, ent, u, c.capacita...), evidenze.UsoSconosciuto(bundleDSint))
			if r.Stato != c.stato {
				t.Errorf("stato %s, atteso %s", r.Stato, c.stato)
			}
			if len(r.Letture) != 0 {
				t.Errorf("letture inventate: %v", idLetture(r.Letture))
			}
		})
	}
}

// ---- relazioni e copertura ----

// TestNessunaRelazioneSenzaUnaFormaAttiva (5.4.6 punti 13 e 15; P1 §10.1; R20 c): «SPECCHIATO DI» o «SIMILE A» nel
// testo libero resta una menzione e non dà relazioni; il particolare simile del cartiglio, riservato in A1, non
// ha forme, quindi nessuna relazione e una nota «campo ricevuto, non letto».
func TestNessunaRelazioneSenzaUnaFormaAttiva(t *testing.T) {
	m := motoreCodice(t)
	doc := docDSint(nil, nil, []evidenze.EntitaLocale{entDisegno("e:pdf:p1"), entIsolata("e:pdf:frammento:1"), entNodo("e:step:#10", "#10")},
		[]evidenze.UnitaEvidenza{
			uCampo(t, "u:pdf:cartiglio:codice:1", "e:pdf:p1", "cartiglio.codice", "ACME7000"),
			uCampo(t, "u:pdf:cartiglio:particolare_simile:1", "e:pdf:p1", "cartiglio.particolare_simile", "ACME7001"),
			uCampo(t, "u:pdf:frammento:1", "e:pdf:frammento:1", "testo_pdf", "SPECCHIATO DI ACME7002 SIMILE A ACME7003"),
			uStep(t, "u:step:#10:descrizione", "e:step:#10", "#10", "radice_step.descrizione", "descrizione", "STAFFA SIMILE A ACME7004"),
		})
	r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
	if len(r.Relazioni) != 0 {
		t.Fatalf("relazioni senza una forma attiva sul particolare simile: %+v", r.Relazioni)
	}
	for _, l := range r.Letture {
		if l.UnitaID == "u:pdf:frammento:1" && l.Funzione != FunzMenzione {
			t.Errorf("%s: %s, attesa menzione (testo libero)", l.ID, l.Funzione)
		}
		if l.UnitaID == "u:pdf:cartiglio:particolare_simile:1" || l.UnitaID == "u:step:#10:descrizione" {
			t.Errorf("lettura su un selettore riservato: %s", l.ID)
		}
	}
	perSelettore := map[string][]string{}
	for _, d := range conCodice(r.Diagnostiche, grammatica.CodiceCapacitaNonSupportata) {
		perSelettore[d.Percorso] = d.Rif
	}
	trovate := 0
	for p, rif := range perSelettore {
		switch {
		case strings.Contains(p, "cartiglio.particolare_simile"):
			trovate++
			if strings.Join(rif, ",") != "u:pdf:cartiglio:particolare_simile:1" {
				t.Errorf("nota del particolare simile con %v", rif)
			}
		case strings.Contains(p, "radice_step.descrizione"):
			trovate++
		case strings.Contains(p, "cartiglio.codice"), strings.Contains(p, "testo_pdf"):
			t.Errorf("nota di campo non letto su un selettore con forme: %s", p)
		}
	}
	if trovate != 2 {
		t.Errorf("attese le note dei due selettori senza forme:\n%s", elenco(r.Diagnostiche))
	}
}

// ---- qualità della lettura e codici ----

// TestLaQualitaNonCambiaLaFunzione (5.4.6 punti 7, 8 e nota al router; 5.10 n.6; D2): la fonte OCR dà la qualità
// da verificare e il suo motivo, la zona «pagina» il suo motivo, ma il router non le guarda e la funzione resta;
// una forma parziale dà la qualità parziale; una base ripetuta che non concorda dà da_verificare con
// motore.ripetizioni_discordanti; le trasformazioni conservano l'originale. Ogni codice nuovo ha la natura e la
// gravità del 5.4.9.
func TestLaQualitaNonCambiaLaFunzione(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famCodice(), famDocumento(), famPunti()))
	corpo := "Serve 9.123.4567 e 9.123.4567.3/01"
	ocr := uCampo(t, "u:pdf:cartiglio:codice:1", "e:pdf:p1", "cartiglio.codice", "ACME1111")
	ocr.Posizione.PDF.Fonte = "ocr"
	ocr.Qualita.Metodo = "ocr"
	pagina := uCampo(t, "u:pdf:cartiglio:codice:2", "e:pdf:p1", "cartiglio.codice", "ACME2222")
	pagina.Posizione.PDF.Zona = "pagina"
	nome := evidenze.UnitaEvidenza{ID: "u:nome", FonteID: fonteDSint, Selettore: selettore(t, "nome_file"), Testo: "97123456#1#R98123456#.pdf",
		Posizione: evidenze.Localizzatore{Tipo: "nome_file", NomeFile: &evidenze.PosNomeFile{Campo: "allegato.nome_file",
			Intervallo: evidenze.Intervallo{Inizio: 0, Fine: len("97123456#1#R98123456#.pdf")}}},
		Qualita: evidenze.QualitaUnita{Localizzazione: "esatta"}}
	doc := docDSint(
		[]evidenze.TestoOriginale{testoOrig(corpoDSint, corpo), testoOrig("allegato.nome_file", nome.Testo)},
		[]evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, len(corpo))},
		[]evidenze.EntitaLocale{entDisegno("e:pdf:p1")},
		[]evidenze.UnitaEvidenza{ocr, pagina, nome, uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo, 0)})
	r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))

	lo := lettureDellUnita(r, "u:pdf:cartiglio:codice:1")
	if len(lo) != 1 || lo[0].Funzione != FunzIdentitaFile || lo[0].Qualita != QualitaDaVerificare || !contiene(lo[0].Motivi, "fonte ocr") {
		t.Errorf("lettura da OCR: %+v", lo)
	}
	lp := lettureDellUnita(r, "u:pdf:cartiglio:codice:2")
	if len(lp) != 1 || lp[0].Funzione != FunzIdentitaFile || lp[0].Qualita != QualitaCompleta || len(lp[0].Motivi) < 2 {
		t.Errorf("lettura fuori zona: %+v", lp)
	}
	ln := lettureDellUnita(r, "u:nome")
	if len(ln) != 1 || ln[0].Qualita != QualitaDaVerificare {
		t.Fatalf("base ripetuta discordante: %+v", ln)
	}
	rip := conCodice(r.Diagnostiche, CodiceMotoreRipetizioniDiscordanti)
	if len(rip) != 1 || !contiene(rip[0].Rif, ln[0].ID) {
		t.Errorf("motore.ripetizioni_discordanti: %+v", rip)
	}
	var parziale, completa bool
	for _, l := range lettureDellUnita(r, "u:corpo:s:corrente") {
		switch l.Forma.Forma {
		case "parziale":
			parziale = l.Qualita == QualitaParziale
		case "completa":
			completa = l.Qualita == QualitaCompleta && l.Forma.Revisione != nil
		}
	}
	if !parziale || !completa {
		t.Errorf("forma parziale %v, completa con revisione %v: %v", parziale, completa, idLetture(r.Letture))
	}

	// Natura e gravità dei codici emessi (5.4.9).
	attesi := map[string][2]string{
		CodiceMotoreLettureAlternative:            {string(evidenze.GravitaAvviso), string(evidenze.NaturaDati)},
		CodiceMotoreLettureAnnidate:               {string(evidenze.GravitaAvviso), string(evidenze.NaturaDati)},
		CodiceMotoreIDNomeDiscordi:                {string(evidenze.GravitaAvviso), string(evidenze.NaturaDati)},
		CodiceMotoreRipetizioniDiscordanti:        {string(evidenze.GravitaAvviso), string(evidenze.NaturaDati)},
		CodiceMotorePertinenzaIgnota:              {string(evidenze.GravitaNota), string(evidenze.NaturaDati)},
		CodiceRevisioneFormazioneNonConfrontabile: {string(evidenze.GravitaNota), string(evidenze.NaturaDati)},
		CodiceRevisioneNonInterpretabile:          {string(evidenze.GravitaAvviso), string(evidenze.NaturaDati)},
		CodiceRevisioneDiscordante:                {string(evidenze.GravitaAvviso), string(evidenze.NaturaDati)},
		CodiceQuantitaNonInterpretabile:           {string(evidenze.GravitaNota), string(evidenze.NaturaDati)},
		grammatica.CodiceCapacitaNonSupportata:    {string(evidenze.GravitaNota), string(evidenze.NaturaCapacita)},
	}
	controlla := func(r Interpretazione) {
		for _, d := range r.Diagnostiche {
			if a, ok := attesi[d.Codice]; ok && (string(d.Gravita) != a[0] || string(d.Natura) != a[1]) {
				t.Errorf("%s: %s %s, attesi %s %s (5.4.9)", d.Codice, d.Gravita, d.Natura, a[0], a[1])
			}
		}
	}
	controlla(r)
	// Le altre interpretazioni del pacchetto passano dagli stessi controlli dove emettono i loro codici: qui
	// quelle con la revisione in campo separato riservata e con le formazioni.
	fc := famCodice()
	fc.Revisioni[0].Stato = grammatica.StatoRiservata
	mr, _, err := compila(t, grammaticaACME(fc), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	dr := docDSint(nil, nil, []evidenze.EntitaLocale{entDisegno("e:pdf:p1"), entNodo("e:step:#10", "#10")},
		[]evidenze.UnitaEvidenza{
			uCampo(t, "u:01", "e:pdf:p1", "cartiglio.codice", "ACME1111"),
			uCampo(t, "u:02", "e:pdf:p1", "cartiglio.revisione", "01"),
			uCampo(t, "u:03", "e:pdf:p1", "cartiglio.titolo", "TITOLO"),
			uStep(t, "u:step:#10:id", "e:step:#10", "#10", "radice_step.id", "id", "ACME1111"),
			uStep(t, "u:step:#10:revisione", "e:step:#10", "#10", "radice_step.revisione", "formazione", "1"),
		})
	rr := interpreta(t, mr, dr, evidenze.UsoSconosciuto(bundleDSint))
	controlla(rr)
	if len(conCodice(rr.Diagnostiche, CodiceRevisioneFormazioneNonConfrontabile)) != 1 || len(conCodice(rr.Diagnostiche, CodiceRevisioneNonInterpretabile)) != 1 {
		t.Errorf("formazione e revisione riservata: %s", elenco(rr.Diagnostiche))
	}
	// La formazione resta al suo nodo, mai al disegno, e non è compatibile con nessuna lettura.
	f := attributoDellUnita(t, rr, AttributoFormazione, "u:step:#10:revisione")
	if f.EntitaID != "e:step:#10" || len(f.LettureCompatibili) != 0 || f.Normalizzato != "" {
		t.Errorf("formazione: %+v", f)
	}
}

// TestLaQuantitaNonPassaDaUnaTabellaAllAltra (5.4.6 punto 12; R28 a): la provenance della quantità è la stessa
// tabella, la stessa riga e la colonna della cella d'intestazione dichiarata. Tentativi che non devono dare
// quantità: l'intestazione in un'altra tabella, l'intestazione sotto le righe con codice, una cella di un'altra
// colonna; e la cella giusta dà la quantità della sua riga soltanto.
func TestLaQuantitaNonPassaDaUnaTabellaAllAltra(t *testing.T) {
	g := grammaticaACME(famCodice())
	g.Quantita = []grammatica.QuantitaTabellare{{ID: "q-acme", Intestazioni: []string{"Q.TA"},
		Regola: grammatica.RegolaQuantitaPrimaRigaSopra, Selettori: sel("corpo"), Stato: grammatica.StatoAttiva}}
	m, _ := compilaBene(t, g)
	corpo := "x"
	seg := []evidenze.Segmento{segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, 1)}
	riga := func(tab, r int) evidenze.EntitaLocale {
		return entRiga("e:tab:"+itoa(tab)+":r"+itoa(r), "s:corrente", tab, r, [2]int{0, 1})
	}
	cella := func(tab, r, c int, testo string) evidenze.UnitaEvidenza {
		return uCella(t, "u:tab:"+itoa(tab)+":r"+itoa(r)+":c"+itoa(c), "e:tab:"+itoa(tab)+":r"+itoa(r), "corpo", testo, tab, r, c, [2]int{0, 1}, nil)
	}
	casi := []struct {
		nome   string
		entita []evidenze.EntitaLocale
		unita  []evidenze.UnitaEvidenza
		attese map[string]string // unità della quantità → normalizzato
	}{
		{"l'intestazione sta in un'altra tabella",
			[]evidenze.EntitaLocale{riga(1, 1), riga(2, 1), riga(2, 2)},
			[]evidenze.UnitaEvidenza{cella(1, 1, 1, "Codice"), cella(1, 1, 2, "Q.TA"), cella(2, 1, 1, "ACME1111"), cella(2, 1, 2, "5"),
				cella(2, 2, 1, "ACME2222"), cella(2, 2, 2, "6")},
			nil},
		{"l'intestazione sta sotto le righe con codice",
			[]evidenze.EntitaLocale{riga(1, 1), riga(1, 2)},
			[]evidenze.UnitaEvidenza{cella(1, 1, 1, "ACME1111"), cella(1, 1, 2, "5"), cella(1, 2, 1, "Codice"), cella(1, 2, 2, "Q.TA")},
			nil},
		{"solo la cella della colonna dichiarata, solo nelle righe con codice",
			[]evidenze.EntitaLocale{riga(1, 1), riga(1, 2), riga(1, 3)},
			[]evidenze.UnitaEvidenza{cella(1, 1, 1, "Codice"), cella(1, 1, 2, "Descrizione"), cella(1, 1, 3, " Q.TA "),
				cella(1, 2, 1, "ACME1111"), cella(1, 2, 2, "7"), cella(1, 2, 3, "12"),
				cella(1, 3, 1, "Nota"), cella(1, 3, 2, "8"), cella(1, 3, 3, "9")},
			map[string]string{"u:tab:1:r2:c3": "12"}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, corpo)}, seg, c.entita, c.unita)
			r := interpreta(t, m, d, evidenze.UsoSconosciuto(bundleDSint))
			q := attributiDi(r, AttributoQuantita)
			if len(q) != len(c.attese) {
				t.Fatalf("quantità %+v, attese %v", q, c.attese)
			}
			for _, a := range q {
				if n, ok := c.attese[a.UnitaID]; !ok || a.Normalizzato != n || a.Stato != StatoAttribuito {
					t.Errorf("quantità %+v, attese %v", a, c.attese)
				}
				if a.EntitaID != entitaDellUnita(d, a.UnitaID) || len(a.Evidenze) != 1 || a.Evidenze[0] != "u:tab:1:r1:c3" {
					t.Errorf("quantità senza la sua riga o la sua intestazione: %+v", a)
				}
			}
		})
	}
}

func entitaDellUnita(d evidenze.DocumentoEvidenze, id string) string {
	for _, u := range d.Unita {
		if u.ID == id {
			return u.EntitaID
		}
	}
	return ""
}
