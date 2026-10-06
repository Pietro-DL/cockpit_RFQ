// L1 — ProponiProdotti, i prodotti candidati della mail (piano A, 6.4.4 e 6.7.1: A1c-L1-05 e A1c-L1-06, riscritta
// con il contratto di B0 §7: i target sono quelli di R60, R70 e R75, e i candidati della mail non sono mai target).
// Dall'adattatore della mail (estrazione.DaMessaggio) a Interpreta e poi a ProponiProdotti, sulle fixture sintetiche:
// codice richiesto separato dalla base, quantità dalla colonna dichiarata con l'evidenza della cella (R28), nessun
// doppione fra testo e HTML, il riferimento della RFQ che non è un prodotto, la storia mai promossa senza una scelta
// (R48 A, E2), il triage solo evidenza (R29 c), ogni esclusione col suo motivo, le regole 1-7 del 6.4.4.
package ancoraggio_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example), i codici sono di fantasia (P712xxxx con la P di
// fase prototipo, ACME7000100, il riferimento ACME26-030), l'intestazione della colonna quantità è inventata («Q.TA»).
// Le grammatiche sono scritte qui in linea, ripetute di proposito rispetto alle prove di motorea ed estrazione. Le
// prove citano i requisiti (A1c-L1-05, R48 A, R60 A), mai i casi degli attesi.

var clienteACME = uuid.MustParse("00000000-0000-4000-8000-00000000ac01")

var (
	idMail      = uuid.MustParse("00000000-0000-4000-8000-000000000061")
	idInoltro   = uuid.MustParse("00000000-0000-4000-8000-000000000062")
	idAltraMail = uuid.MustParse("00000000-0000-4000-8000-000000000063")
)

// ---- le grammatiche ACME ----

func limitiACME() grammatica.Limiti {
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

func base(segmenti ...grammatica.SegmentoBase) grammatica.Base {
	return grammatica.Base{
		Segmenti: segmenti, Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
}

func esempio(id, selettore, testo string, altre bool, letture ...grammatica.LetturaAttesa) grammatica.EsempioCodice {
	return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: selettore, Testo: testo,
		Atteso: grammatica.AttesoEsempio{Letture: letture, AltreAmmesse: altre}}
}

var selMail = []string{"oggetto", "corpo", "storia"}

// famPrefisso: base 712 più quattro cifre, con la P di fase prototipo davanti, riconosciuta e attribuita su
// oggetto, corpo e storia (D2; R48 A: anche sulla storia). Ruoli {prodotto, componente}.
func famPrefisso(altre bool) grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-prefisso", Namespace: "acme-prefisso",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{
				{Tipo: grammatica.TipoParteAffisso, Rif: "P", Min: 1, Max: 1},
				{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1},
			}}},
		Affissi: []grammatica.Affisso{{
			ID: "P", Letterali: []string{"P"}, Posizione: grammatica.PosizionePrefisso,
			Riconoscimento: selMail, Attribuzione: selMail,
			Valore: &grammatica.ValoreQualificatore{Fase: grammatica.FasePrototipo},
		}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-mail", "corpo", "P7120100", altre, grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Affissi: []string{"P"}}),
		},
	}
}

// famDisegno: i codici «ACME7» con sei cifre, con il solo ruolo {componente}: nella mail sono richieste, ma
// l'intersezione con i ruoli della richiesta è vuota (R17 b).
func famDisegno() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-disegno", Namespace: "acme-disegno",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base: base(grammatica.SegmentoBase{Nome: "prefisso", Letterale: "ACME7", Identitario: true},
			grammatica.SegmentoBase{Nome: "numero", Pattern: "[0-9]{6}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-mail", "corpo", "ACME7000100", false, grammatica.LetturaAttesa{Forma: "mail", Base: "ACME7000100"}),
		},
	}
}

// famQuattroCifre: un codice di quattro cifre con il ruolo {prodotto}: serve a provare che la cella della quantità,
// anche quando una famiglia la legge, non diventa un prodotto (6.4.4, regola 5).
func famQuattroCifre() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-quattro", Namespace: "acme-quattro",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto},
		Base:  base(grammatica.SegmentoBase{Nome: "numero", Pattern: "[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-mail", "corpo", "1000", false, grammatica.LetturaAttesa{Forma: "mail", Base: "1000"}),
		},
	}
}

// famAlternativa: un'altra famiglia che legge la stessa occorrenza «P7120100» con la P dentro la base, in un altro
// spazio di codici (6.4.4, regola 6).
func famAlternativa() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-alternativa", Namespace: "acme-alternativa",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto},
		Base: base(grammatica.SegmentoBase{Nome: "lettera", Letterale: "P", Identitario: true},
			grammatica.SegmentoBase{Nome: "numero", Pattern: "712[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-mail", "corpo", "P7120100", true, grammatica.LetturaAttesa{Forma: "mail", Base: "P7120100"}),
		},
	}
}

// quantitaACME: la colonna della quantità con il letterale inventato «Q.TA», dichiarata su corpo e storia (R28 a).
func quantitaACME() []grammatica.QuantitaTabellare {
	return []grammatica.QuantitaTabellare{{ID: "q-acme", Intestazioni: []string{"Q.TA"},
		Regola: grammatica.RegolaQuantitaPrimaRigaSopra, Selettori: []string{"corpo", "storia"}, Stato: grammatica.StatoAttiva}}
}

// motore: la grammatica passa dalla porta del prodotto (file v1, NuovoSnapshot, CompilaVerificato).
func motore(t *testing.T, famiglie ...grammatica.FamigliaCodice) *motorea.Motore {
	t.Helper()
	g := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       famiglie,
		Quantita:       quantitaACME(),
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	s, err := grammatica.NuovoSnapshot(raw, limitiACME())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	m, d, err := motorea.CompilaVerificato(s, limitiACME())
	if err != nil || m == nil {
		t.Fatalf("la grammatica non compila: %v %+v", err, d)
	}
	return m
}

func motoreACME(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, famPrefisso(false), famDisegno())
}

// ---- le mail ACME ----

// riga: una riga della tabella, con codice, descrizione e quantità.
type riga struct{ codice, descrizione, quantita string }

func righeACME() []riga {
	return []riga{{"P7120100", "Staffa", "5"}, {"P7120200", "Piastra", "5"}, {"P7120300", "Perno", "5"}, {"P7120400", "Boccola", "5"}}
}

// mailTabella: la mail con la forma della tabella del caso reale (A1c-L1-05): nel testo una cella per riga,
// l'intestazione «Q.TA» sopra le righe, il riferimento ACME26-030 nel corpo; nell'HTML la stessa tabella.
func mailTabella(id uuid.UUID, oggetto, testoPrima string, righe []riga) fotorfq.Messaggio {
	var testo, html strings.Builder
	testo.WriteString("Buongiorno,\r\n" + testoPrima + "\r\n\r\nCodice\r\nDescrizione\r\nQ.TA\r\n")
	html.WriteString("<html><body><div class=WordSection1>\n<p class=MsoNormal>Buongiorno,</p>\n<p class=MsoNormal>" + testoPrima +
		"</p>\n<table class=MsoNormalTable>\n<tr><td>Codice</td><td>Descrizione</td><td>Q.TA</td></tr>\n")
	for _, r := range righe {
		testo.WriteString(r.codice + "\r\n" + r.descrizione + "\r\n" + r.quantita + "\r\n")
		html.WriteString("<tr><td>" + r.codice + "</td><td>" + r.descrizione + "</td><td>" + r.quantita + "</td></tr>\n")
	}
	testo.WriteString("\r\nGrazie")
	html.WriteString("</table>\n<p class=MsoNormal>Grazie</p>\n</div></body></html>")
	return messaggioACME(id, oggetto, testo.String(), html.String())
}

const testoRiferimento = "vi chiediamo l'offerta per la richiesta ACME26-030."

func mailL105(id uuid.UUID) fotorfq.Messaggio {
	return mailTabella(id, "Richiesta di offerta ACME26-030", testoRiferimento, righeACME())
}

// mailInoltroConStoria: un inoltro senza commento, con la firma dentro il messaggio inoltrato e una citazione
// obsoleta sotto (A1c-L1-06; MAIL-INOLTRO): la parte corrente è vuota, s:storia:1 è l'inoltro, s:storia:2 la
// citazione.
func mailInoltroConStoria(id uuid.UUID) fotorfq.Messaggio {
	corpo := "---------- Messaggio inoltrato ----------\r\n" +
		"Da: Ufficio acquisti ACME <acquisti@acme.example>\r\n" +
		"Data: 30 set 2026, 09:12\r\n" +
		"Oggetto: Richiesta ACME26-030\r\n" +
		"A: vendite@fornitore.example\r\n\r\n" +
		"Buongiorno,\r\nci servirebbe l'offerta per P7120100 e P7120200, disegno di riferimento ACME7000100.\r\n\r\n" +
		"Ufficio acquisti ACME\r\nTel. 0123 456789\r\n\r\n" +
		"Il giorno 29 set 2026, alle ore 17:40, Ufficio tecnico ACME <tecnico@acme.example> ha scritto:\r\n" +
		"Vi giro il codice P7120900 della revisione precedente.\r\nSaluti"
	m := messaggioACME(id, "I: Richiesta ACME26-030", corpo, "")
	m.CorpoHTML = nil
	return m
}

func messaggioACME(id uuid.UUID, oggetto, testo, html string) fotorfq.Messaggio {
	return fotorfq.Messaggio{
		ID:                id,
		ConversazioneID:   uuid.MustParse("00000000-0000-4000-8000-0000000000c6"),
		Canale:            "outlook",
		Direzione:         "entrata",
		DataEvento:        time.Date(2026, 10, 1, 7, 30, 0, 0, time.UTC),
		MittenteNome:      "Ufficio acquisti ACME",
		MittenteIndirizzo: "acquisti@acme.example",
		Oggetto:           &oggetto,
		CorpoTesto:        &testo,
		CorpoHTML:         &html,
	}
}

// ---- dall'adattatore a ProponiProdotti ----

type usoDi func(d evidenze.DocumentoEvidenze) evidenze.UsoSegmenti

func sconosciuto(d evidenze.DocumentoEvidenze) evidenze.UsoSegmenti {
	return evidenze.UsoSconosciuto(d.BundleID)
}

func scelto(segmento, uso, origine string) usoDi {
	return func(d evidenze.DocumentoEvidenze) evidenze.UsoSegmenti {
		return evidenze.UsoSegmenti{BundleID: d.BundleID, Versione: 1, Stato: "valutato",
			Selezioni: []evidenze.SelezioneSegmento{{SegmentoID: segmento, Uso: uso, Origine: origine}}}
	}
}

func interpretato(t *testing.T, m *motorea.Motore, msg fotorfq.Messaggio, uso usoDi) ancoraggio.MessaggioInterpretato {
	t.Helper()
	d, err := estrazione.DaMessaggio(msg)
	if err != nil {
		t.Fatalf("DaMessaggio: %v", err)
	}
	r, err := m.Interpreta(d, uso(d))
	if err != nil {
		t.Fatalf("Interpreta: %v", err)
	}
	return ancoraggio.MessaggioInterpretato{MessaggioID: msg.ID, Documento: d, Interpretazione: r}
}

func richiesta(stato ancoraggio.StatoRichiesta, messaggi ...uuid.UUID) ancoraggio.RichiestaValutata {
	return ancoraggio.RichiestaValutata{ClienteID: clienteACME, Messaggi: messaggi, Stato: stato}
}

// proponi chiama ProponiProdotti e controlla gli invarianti di ogni esito: un candidato è sempre proposto e da
// confermare (R60 A), la sua lettura principale sta fra le evidenze, ogni evidenza (e ogni alternativa) è una lettura
// ammessa del suo messaggio con il suo segmento, uso e origine, ogni evidenza ha la chiave del candidato (namespace,
// base, qualificatori), il motivo dice se un'evidenza viene da un messaggio valutato, le esclusioni puntano a letture
// del loro messaggio, i codici sono quelli del pacchetto, l'impronta è quella del canonico dell'esito (par.3.4.2).
func proponi(t *testing.T, r ancoraggio.RichiestaValutata, msgs ...ancoraggio.MessaggioInterpretato) ancoraggio.EsitoProdotti {
	t.Helper()
	e, err := ancoraggio.ProponiProdotti(msgs, r)
	if err != nil {
		t.Fatalf("ProponiProdotti: %v", err)
	}
	letture := map[uuid.UUID]map[string]motorea.LetturaCodice{}
	for _, m := range msgs {
		letture[m.MessaggioID] = map[string]motorea.LetturaCodice{}
		for _, l := range m.Interpretazione.Letture {
			letture[m.MessaggioID][l.ID] = l
		}
	}
	for _, c := range e.Candidati {
		if c.Origine != ancoraggio.OrigineProposto {
			t.Errorf("candidato %s: origine %q, un candidato della mail è sempre proposto (R60 A)", c.Lettura, c.Origine)
		}
		if c.Motivo != ancoraggio.MotivoCandidatoDaConfermare && c.Motivo != ancoraggio.MotivoCandidatoRichiestaNonValutata {
			t.Errorf("candidato %s: motivo %q fuori elenco", c.Lettura, c.Motivo)
		}
		principale := false
		for _, ev := range c.Evidenze {
			principale = principale || (ev.MessaggioID == c.MessaggioID && ev.Lettura == c.Lettura)
			l, ok := letture[ev.MessaggioID][ev.Lettura]
			if !ok || l.Funzione != motorea.FunzRichiesta || l.Uso != ev.Uso || l.OrigineUso != ev.OrigineUso || ev.Segmento == "" ||
				l.Forma.Namespace != c.Namespace || l.Forma.Base.Normalizzata != c.Base.Normalizzata {
				t.Errorf("candidato %s: l'evidenza %+v non è una sua lettura ammessa", c.Lettura, ev)
			}
		}
		if !principale {
			t.Errorf("candidato %s: la lettura principale non sta fra le evidenze %v", c.Lettura, c.Evidenze)
		}
		for _, ev := range c.Alternative {
			if _, ok := letture[ev.MessaggioID][ev.Lettura]; !ok {
				t.Errorf("candidato %s: l'alternativa %+v non è una lettura del suo messaggio", c.Lettura, ev)
			}
		}
		l := letture[c.MessaggioID][c.Lettura]
		if l.Funzione != motorea.FunzRichiesta || c.CodiceRichiesto != l.Forma.CodiceRichiesto || c.Base.Normalizzata != l.Forma.Base.Normalizzata ||
			c.Namespace != l.Forma.Namespace || c.Uso != l.Uso || c.OrigineUso != l.OrigineUso {
			t.Errorf("candidato %s non riporta la sua lettura: %+v", c.Lettura, c)
		}
	}
	for _, x := range e.Esclusioni {
		if _, ok := letture[x.MessaggioID][x.Lettura]; !ok || x.Motivo == "" {
			t.Errorf("esclusione %+v senza lettura del messaggio o senza motivo", x)
		}
	}
	codici := []string{ancoraggio.CodiceRichiestaNonValutata, ancoraggio.CodiceRigheStessaBase,
		ancoraggio.CodiceQuantitaNonIntera, ancoraggio.CodiceAlternativeConservate}
	for _, d := range e.Diagnostiche {
		if !contiene(codici, d.Codice) {
			t.Errorf("diagnostica con un codice non di ProponiProdotti: %+v", d)
		}
	}
	senza := e
	senza.Impronta = ""
	if imp, err := jsoncanonico.ImprontaDi(senza); err != nil || imp != e.Impronta || e.VersioneServizio != ancoraggio.VersioneServizio || len(e.HashIngresso) != 64 {
		t.Errorf("impronte dell'esito: %v, %q contro %q, versione %q, hash %q", err, imp, e.Impronta, e.VersioneServizio, e.HashIngresso)
	}
	return e
}

func contiene(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

func codiciRichiesti(e ancoraggio.EsitoProdotti) []string {
	var out []string
	for _, c := range e.Candidati {
		out = append(out, c.CodiceRichiesto)
	}
	return out
}

func motiviEsclusione(e ancoraggio.EsitoProdotti, l map[string]motorea.LetturaCodice) map[string]string {
	out := map[string]string{}
	for _, x := range e.Esclusioni {
		out[l[x.Lettura].Forma.Originale] = x.Motivo
	}
	return out
}

func lettureDi(m ancoraggio.MessaggioInterpretato) map[string]motorea.LetturaCodice {
	out := map[string]motorea.LetturaCodice{}
	for _, l := range m.Interpretazione.Letture {
		out[l.ID] = l
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

// ---- A1c-L1-05 ----

// TestL105LaTabellaDellaMailDaQuattroCandidati (A1c-L1-05; P1 §7.4; R28; C-31): la mail con la tabella a quattro righe
// e il riferimento ACME26-030 nel corpo dà quattro candidati, in ordine di riga: il codice richiesto con la P
// separato dalla base, la fase prototipo attribuita, la quantità 5 con l'evidenza della cella della sua riga, una
// lettura sola per riga anche se il codice sta nel testo e nell'HTML; il riferimento non è un prodotto.
func TestL105LaTabellaDellaMailDaQuattroCandidati(t *testing.T) {
	m := motoreACME(t)
	mi := interpretato(t, m, mailL105(idMail), sconosciuto)
	e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)

	if got := strings.Join(codiciRichiesti(e), ","); got != "P7120100,P7120200,P7120300,P7120400" {
		t.Fatalf("candidati %s, attesi i quattro codici in ordine di riga", got)
	}
	l := lettureDi(mi)
	for i, c := range e.Candidati {
		n := i + 2 // la riga 1 è l'intestazione
		rigaID := "e:tab:1:r" + string(rune('0'+n))
		cella := "u:tab:1:r" + string(rune('0'+n)) + ":c3"
		if c.Riga != rigaID || c.MessaggioID != idMail || c.Segmento != "s:corrente" {
			t.Errorf("candidato %d: riga %q, messaggio %s, segmento %q", i, c.Riga, c.MessaggioID, c.Segmento)
		}
		if c.Base.Normalizzata != strings.TrimPrefix(c.CodiceRichiesto, "P") || !c.Base.Completa || c.Namespace != "acme-prefisso" {
			t.Errorf("candidato %d: base %+v, namespace %q: il codice richiesto e la base sono due campi", i, c.Base, c.Namespace)
		}
		if len(c.Qualificatori) != 1 || c.Qualificatori[0].Valore == nil || c.Qualificatori[0].Valore.Fase != grammatica.FasePrototipo || c.Qualificatori[0].Originale != "P" {
			t.Errorf("candidato %d: qualificatori %+v, attesa la fase prototipo dalla P", i, c.Qualificatori)
		}
		if c.Quantita == nil || *c.Quantita != 5 {
			t.Errorf("candidato %d: quantità %v, attesa 5", i, c.Quantita)
		}
		if !reflect.DeepEqual(c.EvidenzaQuantita, []string{"a:" + cella + ":quantita", cella, "u:tab:1:r1:c3"}) {
			t.Errorf("candidato %d: evidenza della quantità %v, attese l'attributo, la cella della sua riga e l'intestazione", i, c.EvidenzaQuantita)
		}
		// Nessun doppione fra testo e HTML (R28 b): la cella e il testo sono la stessa occorrenza, una lettura sola.
		if len(c.Evidenze) != 1 || c.Evidenze[0].Lettura != c.Lettura || !contiene(l[c.Lettura].AltreUnita, "u:corpo:s:corrente") || !strings.HasPrefix(l[c.Lettura].UnitaID, "u:tab:1:r") {
			t.Errorf("candidato %d: evidenze %v, lettura %+v: attesa la sola lettura della cella, con il testo fra le altre unità", i, c.Evidenze, l[c.Lettura])
		}
		if c.Motivo != ancoraggio.MotivoCandidatoDaConfermare || c.Uso != motorea.UsoSconosciuto || c.OrigineUso != "" {
			t.Errorf("candidato %d: motivo %q, uso %q, origine %q", i, c.Motivo, c.Uso, c.OrigineUso)
		}
	}
	// Il riferimento della RFQ non è un prodotto: nessuna lettura lo prende, nessun candidato lo nomina (C-31).
	for _, x := range mi.Interpretazione.Letture {
		if strings.Contains(x.Forma.Originale, "26-030") || strings.Contains(x.Forma.Originale, "ACME26") {
			t.Errorf("il riferimento è letto come un codice: %s", x.ID)
		}
	}
	if len(e.Esclusioni) != 0 || len(e.Diagnostiche) != 0 {
		t.Errorf("esclusioni %+v, diagnostiche %+v: attese nessuna", e.Esclusioni, e.Diagnostiche)
	}

	t.Run("la quantità segue la sua riga", func(t *testing.T) {
		righe := righeACME()
		righe[0].quantita, righe[1].quantita, righe[2].quantita, righe[3].quantita = "12", "3", "750", "1"
		mi := interpretato(t, m, mailTabella(idMail, "Richiesta di offerta ACME26-030", testoRiferimento, righe), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		var q []int
		for _, c := range e.Candidati {
			if c.Quantita == nil {
				t.Fatalf("%s senza quantità", c.CodiceRichiesto)
			}
			q = append(q, *c.Quantita)
		}
		if !reflect.DeepEqual(q, []int{12, 3, 750, 1}) {
			t.Errorf("quantità %v, attese 12, 3, 750, 1 nell'ordine delle righe", q)
		}
	})

	t.Run("il codice nell'oggetto è evidenza della sua riga", func(t *testing.T) {
		mi := interpretato(t, motore(t, famPrefisso(false), famDisegno()),
			mailTabella(idMail, "Richiesta P7120300 ACME26-030", testoRiferimento, righeACME()), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		if len(e.Candidati) != 4 {
			t.Fatalf("candidati %v: la lettura dell'oggetto non è un candidato in più", codiciRichiesti(e))
		}
		c := e.Candidati[2]
		if c.CodiceRichiesto != "P7120300" || len(c.Evidenze) != 2 || !strings.HasPrefix(c.Evidenze[1].Lettura, "l:u:tab:1:r4:c1:") ||
			!strings.HasPrefix(c.Evidenze[0].Lettura, "l:u:oggetto:") || c.Lettura != c.Evidenze[1].Lettura {
			t.Errorf("candidato %+v: attese la cella come lettura principale e l'oggetto fra le evidenze", c)
		}
	})

	t.Run("la stessa base in due righe: due candidati, ognuno con la sua quantità", func(t *testing.T) {
		righe := righeACME()
		righe[2] = riga{"P7120100", "Staffa lunga", "3"}
		mi := interpretato(t, m, mailTabella(idMail, "Richiesta P7120100", testoRiferimento, righe), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		var stessa []ancoraggio.CandidatoProdotto
		for _, c := range e.Candidati {
			if c.CodiceRichiesto == "P7120100" {
				stessa = append(stessa, c)
			}
		}
		if len(stessa) != 3 || stessa[0].Riga != "" || stessa[1].Riga != "e:tab:1:r2" || stessa[2].Riga != "e:tab:1:r4" ||
			*stessa[1].Quantita != 5 || *stessa[2].Quantita != 3 || stessa[0].Quantita != nil {
			t.Fatalf("candidati della stessa base %+v: attesi l'oggetto in testo libero (due righe: nessuna scelta) e le due righe", stessa)
		}
		d := conCodice(e.Diagnostiche, ancoraggio.CodiceRigheStessaBase)
		fonte := "messaggio:" + idMail.String()
		if len(d) != 1 || !reflect.DeepEqual(d[0].Rif, []string{fonte, "e:tab:1:r2", fonte, "e:tab:1:r4"}) {
			t.Errorf("diagnostiche %+v: attesa ancoraggio.righe_stessa_base con le due righe", e.Diagnostiche)
		}
	})

	t.Run("una quantità non intera resta nil, con la diagnosi", func(t *testing.T) {
		righe := righeACME()
		righe[1].quantita = "cinque"
		mi := interpretato(t, m, mailTabella(idMail, "Richiesta di offerta ACME26-030", testoRiferimento, righe), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		c := e.Candidati[1]
		if c.CodiceRichiesto != "P7120200" || c.Quantita != nil || len(c.EvidenzaQuantita) != 0 {
			t.Errorf("candidato %+v: attesa la quantità nil", c)
		}
		d := conCodice(e.Diagnostiche, ancoraggio.CodiceQuantitaNonIntera)
		if len(d) != 1 || d[0].Rif[0] != c.Lettura || d[0].Rif[1] != "a:u:tab:1:r3:c3:quantita" {
			t.Errorf("diagnostiche %+v: attesa ancoraggio.quantita_non_intera sulla riga", e.Diagnostiche)
		}
	})
}

// ---- A1c-L1-06 ----

// TestL106RichiestaEStoria (A1c-L1-06, riscritta: R60 A, R70 A, R75 A; A-C05 nella parte di A1c; MAIL-INOLTRO; P1
// §4.3; R17 b; R48 A; E2): con l'uso sconosciuto nessun prodotto dalla storia; con la scelta dell'operatore o dello
// scenario prodotti solo dal segmento scelto, con l'origine conservata; il riconoscimento non promuove la storia;
// ogni esclusione col suo motivo. Nessun candidato è mai un target: tutti proposti, da confermare.
func TestL106RichiestaEStoria(t *testing.T) {
	m := motoreACME(t)
	msg := mailInoltroConStoria(idInoltro)
	r := richiesta(ancoraggio.StatoRichiestaValutata, idInoltro)

	d, err := estrazione.DaMessaggio(msg)
	if err != nil {
		t.Fatal(err)
	}
	var segmenti []string
	for _, s := range d.Segmenti {
		segmenti = append(segmenti, s.ID+"="+s.Tipo)
	}
	if strings.Join(segmenti, ",") != "s:corrente=corrente,s:storia:1=inoltro,s:storia:2=citazione" {
		t.Fatalf("segmenti della fixture: %v", segmenti)
	}

	t.Run("uso sconosciuto: nessun prodotto dalla storia", func(t *testing.T) {
		mi := interpretato(t, m, msg, sconosciuto)
		e := proponi(t, r, mi)
		if len(e.Candidati) != 0 {
			t.Fatalf("candidati %v dalla storia senza una scelta", codiciRichiesti(e))
		}
		motivi := motiviEsclusione(e, lettureDi(mi))
		for _, codice := range []string{"P7120100", "P7120200", "P7120900", "ACME7000100"} {
			if motivi[codice] != ancoraggio.MotivoEsclusioneMenzione {
				t.Errorf("%s: motivo %q, attesa la menzione", codice, motivi[codice])
			}
		}
	})

	for _, origine := range []string{"operatore", "scenario"} {
		t.Run("s:storia:1 scelto da "+origine, func(t *testing.T) {
			mi := interpretato(t, m, msg, scelto("s:storia:1", "pertinente", origine))
			e := proponi(t, r, mi)
			if got := strings.Join(codiciRichiesti(e), ","); got != "P7120100,P7120200" {
				t.Fatalf("candidati %s, attesi i due codici del messaggio inoltrato", got)
			}
			for _, c := range e.Candidati {
				if c.Segmento != "s:storia:1" || c.Uso != motorea.UsoPertinente || c.OrigineUso != origine || c.Riga != "" || c.Quantita != nil {
					t.Errorf("candidato %+v: atteso il segmento scelto con l'origine %q conservata", c, origine)
				}
				if c.Motivo != ancoraggio.MotivoCandidatoDaConfermare || c.Origine != ancoraggio.OrigineProposto {
					t.Errorf("candidato %s: %q, %q: anche scelto da %s resta proposto e da confermare (R60 A)", c.CodiceRichiesto, c.Motivo, c.Origine, origine)
				}
			}
			motivi := motiviEsclusione(e, lettureDi(mi))
			if motivi["P7120900"] != ancoraggio.MotivoEsclusioneMenzione || motivi["ACME7000100"] != ancoraggio.MotivoEsclusioneSenzaRuoloProdotto {
				t.Errorf("esclusioni %v: attese la citazione obsoleta come menzione e la famiglia solo componente", motivi)
			}
		})
	}

	t.Run("solo il segmento scelto: la citazione obsoleta scelta dall'operatore", func(t *testing.T) {
		mi := interpretato(t, m, msg, scelto("s:storia:2", "pertinente", "operatore"))
		e := proponi(t, r, mi)
		if got := strings.Join(codiciRichiesti(e), ","); got != "P7120900" || e.Candidati[0].Segmento != "s:storia:2" {
			t.Fatalf("candidati %s: atteso il solo codice della citazione", got)
		}
		if motivi := motiviEsclusione(e, lettureDi(mi)); motivi["P7120100"] != ancoraggio.MotivoEsclusioneMenzione {
			t.Errorf("esclusioni %v: il livello sopra non eredita la scelta", motivi)
		}
	})

	// La chiave è (namespace, base, qualificatori, riga) (6.4.4, regola 3; T-B1-02): le
	// menzioni ammesse dello stesso codice in testo libero fanno un candidato solo, anche da segmenti diversi, e ogni
	// evidenza conserva il suo segmento, il suo uso e la sua origine. La principale è quella con la scelta più forte.
	// Una menzione della storia senza una scelta resta un'esclusione e non entra nel candidato.
	t.Run("lo stesso codice in due segmenti: un candidato con due evidenze", func(t *testing.T) {
		conOggetto := mailInoltroConStoria(idInoltro)
		oggetto := "I: Richiesta P7120100"
		conOggetto.Oggetto = &oggetto
		e := proponi(t, r, interpretato(t, m, conOggetto, scelto("s:storia:1", "pertinente", "operatore")))
		if got := strings.Join(codiciRichiesti(e), ","); got != "P7120100,P7120200" {
			t.Fatalf("candidati %s", got)
		}
		c := e.Candidati[0]
		var dove []string
		for _, ev := range c.Evidenze {
			dove = append(dove, ev.Segmento+"/"+ev.Uso+"/"+ev.OrigineUso)
		}
		if strings.Join(dove, ",") != "s:corrente/sconosciuto/,s:storia:1/pertinente/operatore" {
			t.Errorf("evidenze %v: attese l'oggetto e la storia, ognuna con il suo uso", dove)
		}
		if c.Segmento != "s:storia:1" || c.OrigineUso != "operatore" || !strings.HasPrefix(c.Lettura, "l:u:storia:") {
			t.Errorf("candidato %+v: attesa come principale la lettura con la scelta dell'operatore", c)
		}

		e = proponi(t, r, interpretato(t, m, conOggetto, sconosciuto))
		if len(e.Candidati) != 1 || len(e.Candidati[0].Evidenze) != 1 || e.Candidati[0].Segmento != "s:corrente" {
			t.Errorf("candidati %+v: con l'uso sconosciuto la storia resta menzione, e il candidato ha la sola evidenza dell'oggetto", e.Candidati)
		}
	})

	t.Run("lo stesso codice in due messaggi: un candidato con le due evidenze", func(t *testing.T) {
		primo := interpretato(t, m, mailTabella(idMail, "Richiesta P7120900", testoRiferimento, righeACME()), sconosciuto)
		corpo := "Buongiorno,\r\nconfermate anche P7120900?\r\nGrazie"
		secondo := interpretato(t, m, messaggioACME(idAltraMail, "Re: Richiesta", corpo, ""), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail, idAltraMail), primo, secondo)
		var c *ancoraggio.CandidatoProdotto
		for i := range e.Candidati {
			if e.Candidati[i].CodiceRichiesto == "P7120900" {
				c = &e.Candidati[i]
			}
		}
		if c == nil || len(e.Candidati) != 5 || len(c.Evidenze) != 2 || c.Evidenze[0].MessaggioID == c.Evidenze[1].MessaggioID || c.Riga != "" {
			t.Fatalf("candidati %v, %+v: atteso un candidato in testo libero con un'evidenza per messaggio", codiciRichiesti(e), c)
		}
	})

	t.Run("il codice della riga in un altro messaggio è evidenza della riga", func(t *testing.T) {
		primo := interpretato(t, m, mailL105(idMail), sconosciuto)
		corpo := "Buongiorno,\r\nper P7120200 serve anche il disegno.\r\nGrazie"
		secondo := interpretato(t, m, messaggioACME(idAltraMail, "Re: Richiesta", corpo, ""), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail, idAltraMail), primo, secondo)
		if len(e.Candidati) != 4 {
			t.Fatalf("candidati %v: la menzione nel secondo messaggio non è un candidato in più", codiciRichiesti(e))
		}
		c := e.Candidati[1]
		if c.CodiceRichiesto != "P7120200" || c.Riga != "e:tab:1:r3" || c.MessaggioID != idMail || len(c.Evidenze) != 2 ||
			c.Quantita == nil || *c.Quantita != 5 {
			t.Errorf("candidato %+v: attese la riga del primo messaggio come principale e la menzione del secondo fra le evidenze", c)
		}
	})

	t.Run("il riconoscimento, una scelta da valutare o esclusa non promuovono la storia", func(t *testing.T) {
		for _, u := range []usoDi{
			scelto("s:storia:1", "pertinente", "riconoscimento"),
			scelto("s:storia:1", "da_valutare", "operatore"),
			scelto("s:storia:1", "escluso", "operatore"),
		} {
			e := proponi(t, r, interpretato(t, m, msg, u))
			if len(e.Candidati) != 0 {
				t.Errorf("candidati %v dalla storia senza una scelta pertinente dell'operatore o dello scenario", codiciRichiesti(e))
			}
		}
	})

	t.Run("la parte corrente: riconoscimento e operatore conservati, esclusa è menzione", func(t *testing.T) {
		for _, origine := range []string{"riconoscimento", "operatore"} {
			e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail),
				interpretato(t, m, mailL105(idMail), scelto("s:corrente", "pertinente", origine)))
			if len(e.Candidati) != 4 {
				t.Fatalf("%s: candidati %v", origine, codiciRichiesti(e))
			}
			for _, c := range e.Candidati {
				if c.Uso != motorea.UsoPertinente || c.OrigineUso != origine || c.Motivo != ancoraggio.MotivoCandidatoDaConfermare {
					t.Errorf("%s: candidato %+v", origine, c)
				}
			}
		}
		mi := interpretato(t, m, mailL105(idMail), scelto("s:corrente", "escluso", "operatore"))
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		if len(e.Candidati) != 0 || len(e.Esclusioni) != 4 {
			t.Errorf("segmento escluso: candidati %v, esclusioni %+v", codiciRichiesti(e), e.Esclusioni)
		}
	})

	t.Run("la famiglia solo componente nella parte corrente", func(t *testing.T) {
		mi := interpretato(t, m, mailTabella(idMail, "Richiesta di offerta ACME26-030",
			"vi chiediamo l'offerta, disegno di riferimento ACME7000100.", righeACME()), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		if len(e.Candidati) != 4 || motiviEsclusione(e, lettureDi(mi))["ACME7000100"] != ancoraggio.MotivoEsclusioneSenzaRuoloProdotto {
			t.Errorf("candidati %v, esclusioni %+v: attesa ACME7000100 esclusa, famiglia senza il ruolo prodotto (R17 b)", codiciRichiesti(e), e.Esclusioni)
		}
	})

	t.Run("la cella della quantità non è un prodotto, anche se una famiglia la legge", func(t *testing.T) {
		mq := motore(t, famPrefisso(false), famDisegno(), famQuattroCifre())
		righe := righeACME()
		righe[0].quantita = "1000"
		mi := interpretato(t, mq, mailTabella(idMail, "Richiesta di offerta ACME26-030", testoRiferimento, righe), sconosciuto)
		e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
		if got := strings.Join(codiciRichiesti(e), ","); got != "P7120100,P7120200,P7120300,P7120400" {
			t.Fatalf("candidati %s: la quantità è diventata un prodotto", got)
		}
		if q := e.Candidati[0].Quantita; q == nil || *q != 1000 {
			t.Errorf("quantità %v, attesa 1000", q)
		}
		if len(e.Esclusioni) != 1 || e.Esclusioni[0].Motivo != ancoraggio.MotivoEsclusioneAttributoDiRiga ||
			!strings.HasPrefix(e.Esclusioni[0].Lettura, "l:u:tab:1:r2:c3:acme-quattro/") {
			t.Errorf("esclusioni %+v: attesa la lettura della cella della quantità, attributo di riga", e.Esclusioni)
		}
	})
}

// TestL106InoltroSenzaConfine (A1c-L1-06; R48 A; A1b-25): la mail di A1c-L1-05 con l'oggetto «I: …» ha la storia non
// separabile, tutta in s:storia:1. Senza la scelta di s:storia:1 nessun prodotto; con la scelta dello scenario i
// quattro prodotti della tabella, con la quantità; il riconoscimento non la promuove.
func TestL106InoltroSenzaConfine(t *testing.T) {
	m := motoreACME(t)
	msg := mailTabella(idInoltro, "I: Richiesta di offerta ACME26-030", testoRiferimento, righeACME())
	d, err := estrazione.DaMessaggio(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Segmenti) != 2 || d.Segmenti[1].ID != "s:storia:1" || d.Segmenti[1].Tipo != evidenze.SegmentoInoltro {
		t.Fatalf("la fixture non ha la storia non separabile: %+v", d.Segmenti)
	}
	r := richiesta(ancoraggio.StatoRichiestaValutata, idInoltro)

	for _, u := range []usoDi{sconosciuto, scelto("s:storia:1", "pertinente", "riconoscimento")} {
		mi := interpretato(t, m, msg, u)
		e := proponi(t, r, mi)
		if len(e.Candidati) != 0 || len(e.Esclusioni) != 4 {
			t.Fatalf("candidati %v, esclusioni %+v: nessun prodotto senza la scelta di s:storia:1", codiciRichiesti(e), e.Esclusioni)
		}
		for _, x := range e.Esclusioni {
			if x.Motivo != ancoraggio.MotivoEsclusioneMenzione {
				t.Errorf("esclusione %+v: attesa la menzione", x)
			}
		}
	}

	e := proponi(t, r, interpretato(t, m, msg, scelto("s:storia:1", "pertinente", "scenario")))
	if got := strings.Join(codiciRichiesti(e), ","); got != "P7120100,P7120200,P7120300,P7120400" {
		t.Fatalf("candidati %s con s:storia:1 scelto dallo scenario", got)
	}
	for _, c := range e.Candidati {
		if c.Segmento != "s:storia:1" || c.OrigineUso != "scenario" || c.Quantita == nil || *c.Quantita != 5 || c.Riga == "" ||
			c.Motivo != ancoraggio.MotivoCandidatoDaConfermare || c.Origine != ancoraggio.OrigineProposto {
			t.Errorf("candidato %+v: attesi la riga, la quantità, l'origine scenario conservata e il candidato da confermare", c)
		}
	}
}

// TestL106RichiestaNonValutata (A1c-L1-06; 6.4.4 regola 1; R29 c; T-B0-06): senza il gesto 1 i candidati restano
// elencati con il motivo richiesta_non_valutata e la diagnosi; il triage è solo evidenza e non cambia niente; il
// gesto del messaggio vale sullo stato del thread; si considerano solo i messaggi della richiesta.
func TestL106RichiestaNonValutata(t *testing.T) {
	m := motoreACME(t)
	mi := interpretato(t, m, mailL105(idMail), sconosciuto)

	for _, stato := range []ancoraggio.StatoRichiesta{ancoraggio.StatoRichiestaDaValutare, ancoraggio.StatoRichiestaNonValutabile, ""} {
		r := richiesta(stato, idMail)
		r.Evento = "nuova_rfq/richiesta_offerta" // il triage deterministico: un'evidenza, mai il gesto
		e := proponi(t, r, mi)
		if len(e.Candidati) != 4 {
			t.Fatalf("stato %q: candidati %v: i candidati si elencano lo stesso", stato, codiciRichiesti(e))
		}
		for _, c := range e.Candidati {
			if c.Motivo != ancoraggio.MotivoCandidatoRichiestaNonValutata {
				t.Errorf("stato %q: candidato %s con il motivo %q", stato, c.CodiceRichiesto, c.Motivo)
			}
		}
		d := conCodice(e.Diagnostiche, ancoraggio.CodiceRichiestaNonValutata)
		if len(d) != 1 || d[0].Rif[0] != "messaggio:"+idMail.String() || len(d[0].Rif) != 5 {
			t.Errorf("stato %q: diagnostiche %+v", stato, e.Diagnostiche)
		}
	}

	t.Run("il gesto del messaggio vale sullo stato del thread", func(t *testing.T) {
		r := richiesta(ancoraggio.StatoRichiestaValutata, idMail)
		r.Gesti = []ancoraggio.GestoMessaggio{{MessaggioID: idMail, Stato: ancoraggio.StatoRichiestaDaValutare, Aggancio: "auto_conversazione"}}
		if e := proponi(t, r, mi); e.Candidati[0].Motivo != ancoraggio.MotivoCandidatoRichiestaNonValutata {
			t.Errorf("gesto da valutare sotto un thread valutato: motivo %q", e.Candidati[0].Motivo)
		}
		da := uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
		il := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
		r = richiesta(ancoraggio.StatoRichiestaDaValutare, idMail)
		r.Gesti = []ancoraggio.GestoMessaggio{{MessaggioID: idMail, Stato: ancoraggio.StatoRichiestaValutata, Aggancio: "operatore", AgganciatoDa: &da, AgganciatoIl: &il}}
		e := proponi(t, r, mi)
		if e.Candidati[0].Motivo != ancoraggio.MotivoCandidatoDaConfermare || len(e.Diagnostiche) != 0 {
			t.Errorf("gesto dell'operatore: motivo %q, diagnostiche %+v", e.Candidati[0].Motivo, e.Diagnostiche)
		}
	})

	t.Run("solo i messaggi della richiesta", func(t *testing.T) {
		altra := interpretato(t, m, mailTabella(idAltraMail, "Richiesta di offerta ACME26-030", testoRiferimento,
			[]riga{{"P7120500", "Flangia", "2"}}), sconosciuto)
		r := richiesta(ancoraggio.StatoRichiestaValutata, idMail)
		solo := proponi(t, r, mi)
		conAltra := proponi(t, r, altra, mi)
		if solo.Impronta != conAltra.Impronta || solo.HashIngresso != conAltra.HashIngresso {
			t.Errorf("un messaggio fuori dalla richiesta cambia l'esito: %v", codiciRichiesti(conAltra))
		}
		entrambi := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idAltraMail, idMail), mi, altra)
		if len(entrambi.Candidati) != 5 {
			t.Errorf("candidati %v, attesi quelli dei due messaggi", codiciRichiesti(entrambi))
		}
	})
}

// TestProponiProdottiAlternative (6.4.4, regola 6): due famiglie leggono la stessa occorrenza con basi diverse;
// restano tutte e due, ognuna con l'altra fra le alternative, con ancoraggio.alternative_conservate; nessuna scelta.
func TestProponiProdottiAlternative(t *testing.T) {
	m := motore(t, famPrefisso(true), famDisegno(), famAlternativa())
	mi := interpretato(t, m, mailTabella(idMail, "Richiesta di offerta ACME26-030", testoRiferimento, righeACME()[:1]), sconosciuto)
	e := proponi(t, richiesta(ancoraggio.StatoRichiestaValutata, idMail), mi)
	if len(e.Candidati) != 2 {
		t.Fatalf("candidati %+v: attese le due letture della stessa occorrenza", e.Candidati)
	}
	a, b := e.Candidati[0], e.Candidati[1]
	if a.Riga != b.Riga || a.Namespace == b.Namespace || len(a.Alternative) != 1 || a.Alternative[0].Lettura != b.Lettura ||
		len(b.Alternative) != 1 || b.Alternative[0].Lettura != a.Lettura {
		t.Errorf("candidati %+v e %+v: attese le alternative incrociate", a, b)
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceAlternativeConservate); len(d) != 1 {
		t.Errorf("diagnostiche %+v: attesa ancoraggio.alternative_conservate", e.Diagnostiche)
	}
}

// TestProponiProdottiDeterministico (par.3.4.2, 3.4.5; MOTORE-SENZA-LLM): messaggi, richiesta e gesti in ordine
// diverso danno gli stessi byte canonici e la stessa impronta; una richiesta diversa cambia HashIngresso.
func TestProponiProdottiDeterministico(t *testing.T) {
	m := motoreACME(t)
	a := interpretato(t, m, mailL105(idMail), sconosciuto)
	b := interpretato(t, m, mailInoltroConStoria(idInoltro), scelto("s:storia:1", "pertinente", "operatore"))
	da := uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	il := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	ilRoma := il.In(time.FixedZone("ACME", 2*3600))
	r1 := richiesta(ancoraggio.StatoRichiestaValutata, idMail, idInoltro)
	r1.Gesti = []ancoraggio.GestoMessaggio{
		{MessaggioID: idMail, Stato: ancoraggio.StatoRichiestaValutata, Aggancio: "operatore", AgganciatoDa: &da, AgganciatoIl: &il},
		{MessaggioID: idInoltro, Stato: ancoraggio.StatoRichiestaDaValutare},
	}
	r2 := richiesta(ancoraggio.StatoRichiestaValutata, idInoltro, idMail)
	r2.Gesti = []ancoraggio.GestoMessaggio{r1.Gesti[1], r1.Gesti[0]}
	r2.Gesti[1].AgganciatoIl = &ilRoma

	e1 := proponi(t, r1, a, b)
	e2 := proponi(t, r2, b, a)
	c1, err1 := jsoncanonico.Codifica(e1)
	c2, err2 := jsoncanonico.Codifica(e2)
	if err1 != nil || err2 != nil || string(c1) != string(c2) || e1.Impronta != e2.Impronta {
		t.Fatalf("stessi ingressi in ordine diverso, esiti diversi:\n%s\n%s", c1, c2)
	}
	// I due codici dell'inoltro sono le righe della tabella dell'altro messaggio: evidenze di quelle righe (T-B1-02).
	if len(e1.Candidati) != 4 || e1.Candidati[0].MessaggioID != idMail || len(e1.Candidati[0].Evidenze) != 2 ||
		e1.Candidati[0].Evidenze[1].MessaggioID != idInoltro {
		t.Errorf("candidati %v", codiciRichiesti(e1))
	}
	r3 := r1
	r3.Evento = "nuova_rfq/richiesta_offerta"
	if e3 := proponi(t, r3, a, b); e3.HashIngresso == e1.HashIngresso || e3.Impronta == e1.Impronta {
		t.Error("una richiesta diversa non cambia HashIngresso")
	}
}

// TestProponiProdottiErroriDiContratto: un messaggio ripetuto, un documento di un altro messaggio, un'interpretazione
// di un altro documento, una lettura su un'unità che il documento non ha, un gesto ripetuto: errore di contratto,
// con il codice della foglia, mai un esito.
func TestProponiProdottiErroriDiContratto(t *testing.T) {
	m := motoreACME(t)
	a := interpretato(t, m, mailL105(idMail), sconosciuto)
	b := interpretato(t, m, mailInoltroConStoria(idInoltro), sconosciuto)
	r := richiesta(ancoraggio.StatoRichiestaValutata, idMail, idInoltro)

	scambiato := a
	scambiato.Documento = b.Documento
	altroBundle := a
	altroBundle.Interpretazione.BundleID = b.Documento.BundleID
	unitaPerse := a
	unitaPerse.Documento.Unita = nil
	gestoDoppio := r
	gestoDoppio.Gesti = []ancoraggio.GestoMessaggio{{MessaggioID: idMail, Stato: ancoraggio.StatoRichiestaValutata}, {MessaggioID: idMail}}

	for _, c := range []struct {
		nome   string
		msgs   []ancoraggio.MessaggioInterpretato
		r      ancoraggio.RichiestaValutata
		codice string
	}{
		{"messaggio ripetuto", []ancoraggio.MessaggioInterpretato{a, a}, r, evidenze.CodiceDocumentoIDRipetuto},
		{"documento di un altro messaggio", []ancoraggio.MessaggioInterpretato{scambiato}, r, evidenze.CodiceDocumentoRiferimentoPendente},
		{"interpretazione di un altro documento", []ancoraggio.MessaggioInterpretato{altroBundle}, r, evidenze.CodiceDocumentoRiferimentoPendente},
		{"letture su unità che non ci sono", []ancoraggio.MessaggioInterpretato{unitaPerse}, r, evidenze.CodiceDocumentoRiferimentoPendente},
		{"gesto ripetuto", []ancoraggio.MessaggioInterpretato{a}, gestoDoppio, evidenze.CodiceDocumentoIDRipetuto},
	} {
		t.Run(c.nome, func(t *testing.T) {
			e, err := ancoraggio.ProponiProdotti(c.msgs, c.r)
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) || len(conCodice(ec.Diagnostiche, c.codice)) == 0 {
				t.Fatalf("errore %v, atteso un errore di contratto con %s", err, c.codice)
			}
			if !reflect.DeepEqual(e, ancoraggio.EsitoProdotti{}) {
				t.Errorf("con l'errore un esito non vuoto: %+v", e)
			}
		})
	}
}

// TestIValoriDelContratto (contratto §2.2; par.3.4.1): la versione del servizio e i valori delle costanti del
// vocabolario e della fonte sono quelli del contratto. Se uno cambia, questa prova si riscrive con «Riscritta per …».
func TestIValoriDelContratto(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{ancoraggio.VersioneServizio, "ancoraggio-1"},
		{string(ancoraggio.AutoritaConfermata), "confermata"},
		{string(ancoraggio.AutoritaProposta), "proposta"},
		{string(ancoraggio.AutoritaScenario), "scenario"},
		{string(ancoraggio.OrigineGrezzo), "grezzo"},
		{string(ancoraggio.OrigineProposto), "proposto"},
		{string(ancoraggio.OrigineManuale), "manuale"},
		{string(ancoraggio.OrigineConfermato), "confermato"},
		{string(ancoraggio.OrigineScenario), "scenario"},
		{string(ancoraggio.StatoRichiestaValutata), "valutata"},
		{string(ancoraggio.StatoRichiestaDaValutare), "da_valutare"},
		{string(ancoraggio.StatoRichiestaNonValutabile), "non_valutabile"},
		{ancoraggio.TipoRiferimentoStep, "step"},
		{ancoraggio.FormaRiferimentoSmistamento, "smistamento"},
		{ancoraggio.FormaRiferimentoStepStrutturale, "step_strutturale_id"},
		{string(ancoraggio.EstrazioneRiuscita), "riuscita"},
		{string(ancoraggio.EstrazioneFallita), "fallita"},
		{string(ancoraggio.EstrazioneInCorso), "in_corso"},
		{string(ancoraggio.EstrazioneNonAnalizzata), "non_analizzata"},
		{string(ancoraggio.CandidatoDaDocumentoDelProdotto), "documento_del_prodotto"},
		{string(ancoraggio.CandidatoDaMotoreA), "motore_a"},
		{string(ancoraggio.CandidatoDaPropostaAperta), "proposta_aperta"},
	} {
		if c.got != c.want {
			t.Errorf("valore %q, il contratto dice %q", c.got, c.want)
		}
	}

	// I campi del contratto, con i tag JSON snake_case (contratto §2.2; README di fotorfq, «tag JSON snake_case»).
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{ancoraggio.GestoMessaggio{}, "MessaggioID:messaggio_id Stato:stato Aggancio:aggancio AgganciatoDa:agganciato_da AgganciatoIl:agganciato_il Evento:evento Motivo:motivo"},
		{ancoraggio.RiferimentoFonte{}, "Tipo:tipo DocumentoID:documento_id Sha256:sha256 AllegatoID:allegato_id Radice:radice Ruolo:ruolo Forma:forma ConfermatoDa:confermato_da ConfermatoIl:confermato_il Superato:superato"},
		{ancoraggio.DocumentoCandidato{}, "Origine:origine AllegatoID:allegato_id DocumentoID:documento_id Sha256:sha256 Radici:radici RadiceCompatibile:radice_compatibile Compatibilita:compatibilita Estrazione:estrazione GrafoCompleto:grafo_completo MotivoGrafo:motivo_grafo"},
	} {
		tp := reflect.TypeOf(c.tipo)
		var campi []string
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			campi = append(campi, f.Name+":"+tag)
		}
		if got := strings.Join(campi, " "); got != c.campi {
			t.Errorf("%s: campi %s, il contratto dice %s", tp.Name(), got, c.campi)
		}
	}
}
