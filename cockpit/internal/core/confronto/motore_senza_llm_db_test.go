//go:build integrazione

// L4 — MOTORE-SENZA-LLM sul database di prova (A1c-L4S-08; par.3.5; T-B6-01): caricatore.Carica, valutazione.Calcola e
// confronto.Confronta sulla scena ACME, prima senza nessun ingresso dell'agente (la tabella dei suggerimenti vuota),
// poi con un suggerimento discordante, un candidato di codice di origine agente e un triage di fonte agente. Le impronte
// delle interpretazioni, dei prodotti, degli ancoraggi, della valutazione e del confronto restano le stesse; il registro
// SQL non ha testi vietati; il codice ignoto, le letture discordanti e le diagnostiche ci sono tutte e due le volte.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package confronto_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/testutil"
)

// impronte: le impronte di un giro, per livello: interpretazioni (per allegato), prodotti, ancoraggi, valutazione,
// confronto.
type impronte struct {
	fotografia, prodotti, ancoraggi, valutazione, confronto string
	interpretazioni                                         []string
}

func improntaDi(t *testing.T, p percorso) impronte {
	t.Helper()
	if len(p.valutato.Thread) != 1 || len(p.confronti) != 1 {
		t.Fatalf("thread %d, confronti %d", len(p.valutato.Thread), len(p.confronti))
	}
	th := p.valutato.Thread[0]
	out := impronte{fotografia: p.impronta, prodotti: th.Prodotti.Impronta, ancoraggi: th.Ancoraggi.Impronta,
		valutazione: p.valutato.Impronta, confronto: p.confronti[0].Impronta}
	for _, f := range th.File {
		out.interpretazioni = append(out.interpretazioni, f.AllegatoID.String()+"="+f.Interpretazione.ID)
	}
	return out
}

// codiciDiagnostiche: i codici delle diagnostiche del thread (valutazione, prodotti, ancoraggi) e del confronto, in
// ordine, senza doppioni.
func codiciDiagnostiche(p percorso) []string {
	visti := map[string]bool{}
	th := p.valutato.Thread[0]
	for _, gruppo := range [][]string{codici(th.Diagnostiche), codici(th.Prodotti.Diagnostiche), codici(th.Ancoraggi.Diagnostiche),
		codici(p.confronti[0].Diagnostiche), codici(p.valutato.Diagnostiche)} {
		for _, c := range gruppo {
			visti[c] = true
		}
	}
	out := make([]string, 0, len(visti))
	for c := range visti {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func codici(d []evidenze.Diagnostica) []string {
	out := make([]string, 0, len(d))
	for _, x := range d {
		out = append(out, x.Codice)
	}
	return out
}

// rigaDi: la riga di confronto di un allegato.
func rigaDi(t *testing.T, e confronto.Esito, allegato uuid.UUID) confronto.EsitoFile {
	t.Helper()
	for _, r := range e.File {
		if r.File.AllegatoID == allegato {
			return r
		}
	}
	t.Fatalf("l'allegato %s non ha una riga di confronto", allegato)
	return confronto.EsitoFile{}
}

func TestL4MotoreSenzaLLM(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaDB(t, p)
	if n := testutil.Conta(t, p, "analisi"+"_messaggio"); n != 0 { // il nome spezzato, come nella guardia
		t.Fatalf("la tabella dei suggerimenti dell'agente ha %d righe prima della prova", n)
	}
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	r := insiemeACME(t, s.cliente)

	senza := giro(t, "senza l'agente", pr, reg, s.thread, r)
	conIngressiDellAgente(t, p, s)
	con := giro(t, "con l'agente", pr, reg, s.thread, r)

	for nome, x := range map[string]percorso{"senza l'agente": senza, "con l'agente": con} {
		if len(x.sqlVietato) > 0 {
			t.Errorf("%s: testi vietati nel registro: %q", nome, x.sqlVietato)
		}
		if !x.valutato.Thread[0].Valutato {
			t.Fatalf("%s: il thread non è valutato: %s", nome, x.valutato.Thread[0].Motivo)
		}
	}
	i1, i2 := improntaDi(t, senza), improntaDi(t, con)
	if !reflect.DeepEqual(i1, i2) {
		t.Errorf("gli ingressi dell'agente cambiano le impronte:\nsenza %+v\ncon   %+v", i1, i2)
	}
	if i1.valutazione == "" || i1.confronto == "" || i1.prodotti == "" || i1.ancoraggi == "" || len(i1.interpretazioni) != 5 {
		t.Errorf("impronte vuote o incomplete: %+v", i1)
	}

	// Il codice ignoto, le letture discordanti e le diagnostiche, tutte e due le volte, con gli stessi codici.
	for nome, x := range map[string]percorso{"senza l'agente": senza, "con l'agente": con} {
		c := x.confronti[0]
		ignoto := rigaDi(t, c, s.aIgnoto)
		if len(ignoto.File.Nuovo.Candidati) != 0 || ignoto.Badge == confronto.BadgeUguale || ignoto.Badge == confronto.BadgeNuovoAncoraggio {
			t.Errorf("%s: il codice ignoto ha candidati o un badge che lo dà per buono: %+v", nome, ignoto)
		}
		ambiguo := rigaDi(t, c, s.aAmbiguo)
		if len(ambiguo.File.Nuovo.Basi) != 2 || (ambiguo.File.Nuovo.Associazione != "ambiguo" && ambiguo.File.Nuovo.Associazione != "discordante") {
			t.Errorf("%s: le letture discordanti non restano visibili: %+v", nome, ambiguo.File.Nuovo)
		}
		if len(codiciDiagnostiche(x)) == 0 {
			t.Errorf("%s: nessuna diagnostica", nome)
		}
		for _, v := range x.valutato.Thread[0].VecchiProdotti {
			if v == "candidato:P7129999" || v == "identificativo:P7129999" {
				t.Errorf("%s: il candidato dell'agente è nel vecchio: %v", nome, x.valutato.Thread[0].VecchiProdotti)
			}
		}
	}
	if d1, d2 := codiciDiagnostiche(senza), codiciDiagnostiche(con); !reflect.DeepEqual(d1, d2) {
		t.Errorf("diagnostiche diverse: %v e %v", d1, d2)
	}
	// Il 2D deciso: il vecchio viene dal documento, e il motore A propone il componente deciso con la stessa base.
	figlio := rigaDi(t, senza.confronti[0], s.aFiglio)
	if figlio.File.Vecchio.Componente == nil || *figlio.File.Vecchio.Componente != s.figlio ||
		figlio.File.Vecchio.Documento == nil || *figlio.File.Vecchio.Documento != s.docFiglio || figlio.Badge != confronto.BadgeUguale {
		t.Errorf("il 2D deciso: badge %s (%s), vecchio %+v", figlio.Badge, figlio.Motivo, figlio.File.Vecchio)
	}
	// La misura delle correzioni (R114, precisata dall'utente il 07/10): il 2D deciso porta la lettura del vecchio motore
	// fino al DTO di confronto (la stessa base del codice deciso, nessun marcatore nella grammatica ACME), entra nel
	// denominatore e non ha correzioni, né prima né dopo (un candidato solo, il componente deciso). Con l'agente la misura
	// è la stessa.
	if v := figlio.File.Vecchio; v.CodiceLetto != "7120101" || v.CodiceLettoBase != "7120101" || v.Base != "7120101" || v.CodiceLettoMarcatore != "" {
		t.Errorf("il 2D deciso: la lettura del vecchio motore non arriva al confronto: %+v", v)
	}
	attese := confronto.CorrezioniManuali{Decisi: 1, Valutabili: 1}
	for nome, x := range map[string]percorso{"senza l'agente": senza, "con l'agente": con} {
		if c := x.confronti[0].Correzioni; c != attese {
			t.Errorf("%s: correzioni %+v, attese %+v (un deciso valutabile, nessuna correzione)", nome, c, attese)
		}
	}
}
