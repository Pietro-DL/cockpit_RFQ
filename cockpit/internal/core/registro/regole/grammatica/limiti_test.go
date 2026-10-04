package grammatica

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — i limiti, la parte della validazione e dei tetti (A1a-LIM; R43 B; parte 1 §7.5; v3 §2): i limiti
// vengono dall'indice, il codice ha solo i tetti e nessun valore predefinito; limiti assenti, nulli o oltre i
// tetti non attivano niente; un limite di validazione superato è un errore della grammatica. La parte
// dell'indice sta in indice_test.go, quella del riconoscimento con motorea.
//
// I clienti di questi test sono inventati: la grammatica è quella ACME di valida_test.go.

// TestA1aLIMTettiDelCodice fissa i tetti: alzarne uno vuol dire riscrivere questa prova con il titolo
// «Riscritta per …», in un commit che lo dichiara (par.3.4.1).
func TestA1aLIMTettiDelCodice(t *testing.T) {
	atteso := Limiti{
		Grammatica: LimitiGrammatica{
			MaxFamiglie: 64, MaxFormePerFamiglia: 64, MaxPartiPerForma: 32, MaxEsempi: 256,
			MaxLunghezzaPattern: 256, MaxRipetizione: 100, MaxLunghezzaLetterale: 128,
		},
		Riconoscimento: LimitiRiconoscimento{
			MaxByteUnita: 8388608, MaxUnitaDocumento: 100000, MaxLetturePerUnita: 10000, MaxLettureDocumento: 200000,
		},
		Anteprima: LimitiAnteprima{TempoMassimoMs: 60000},
	}
	if TettiLimiti() != atteso {
		t.Fatalf("TettiLimiti() = %+v, attesi %+v", TettiLimiti(), atteso)
	}
	// I tetti stessi, con una versione, sono limiti validi: il tetto è compreso.
	lim := TettiLimiti()
	lim.Versione = "limiti-acme-tetti"
	if ds := lim.Valida(); ds != nil {
		t.Fatalf("limiti uguali ai tetti rifiutati:%s", elenca(ds))
	}
	if ds := limitiACME().Valida(); ds != nil {
		t.Fatalf("i limiti ACME (i valori iniziali del piano) rifiutati:%s", elenca(ds))
	}
}

func TestA1aLIMLimitiNonValidi(t *testing.T) {
	t.Run("assenti", func(t *testing.T) {
		ds := Limiti{}.Valida()
		if len(ds) != 1 {
			t.Fatalf("attesa una diagnostica sola:%s", elenca(ds))
		}
		d := richiedi(t, ds, CodiceCampoObbligatorio, "limiti")
		if d.Gravita != evidenze.GravitaErrore || d.Natura != evidenze.NaturaContratto {
			t.Fatalf("limiti assenti: %s %s", d.Gravita, d.Natura)
		}
	})
	t.Run("senza versione", func(t *testing.T) {
		lim := limitiACME()
		lim.Versione = ""
		richiedi(t, lim.Valida(), CodiceCampoObbligatorio, "limiti.versione_limiti")
	})
	cambi := []struct {
		nome     string
		cambia   func(*Limiti)
		codice   string
		percorso string
	}{
		{"famiglie zero", func(l *Limiti) { l.Grammatica.MaxFamiglie = 0 }, CodiceCampoObbligatorio, "limiti.grammatica.max_famiglie"},
		{"ripetizione negativa", func(l *Limiti) { l.Grammatica.MaxRipetizione = -1 }, CodiceCampoObbligatorio, "limiti.grammatica.max_ripetizione"},
		{"letture per documento assenti", func(l *Limiti) { l.Riconoscimento.MaxLettureDocumento = 0 }, CodiceCampoObbligatorio, "limiti.riconoscimento.max_letture_documento"},
		{"tempo dell'anteprima assente", func(l *Limiti) { l.Anteprima.TempoMassimoMs = 0 }, CodiceCampoObbligatorio, "limiti.anteprima.tempo_massimo_ms"},
		{"famiglie oltre il tetto", func(l *Limiti) { l.Grammatica.MaxFamiglie = 65 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_famiglie"},
		{"forme oltre il tetto", func(l *Limiti) { l.Grammatica.MaxFormePerFamiglia = 65 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_forme_per_famiglia"},
		{"parti oltre il tetto", func(l *Limiti) { l.Grammatica.MaxPartiPerForma = 33 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_parti_per_forma"},
		{"esempi oltre il tetto", func(l *Limiti) { l.Grammatica.MaxEsempi = 257 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_esempi"},
		{"pattern oltre il tetto", func(l *Limiti) { l.Grammatica.MaxLunghezzaPattern = 257 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_lunghezza_pattern"},
		{"ripetizione oltre il tetto", func(l *Limiti) { l.Grammatica.MaxRipetizione = 101 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_ripetizione"},
		{"letterale oltre il tetto", func(l *Limiti) { l.Grammatica.MaxLunghezzaLetterale = 129 }, CodiceLimiteOltreTetto, "limiti.grammatica.max_lunghezza_letterale"},
		{"byte per unità oltre il tetto", func(l *Limiti) { l.Riconoscimento.MaxByteUnita = 8388609 }, CodiceLimiteOltreTetto, "limiti.riconoscimento.max_byte_unita"},
		{"unità per documento oltre il tetto", func(l *Limiti) { l.Riconoscimento.MaxUnitaDocumento = 100001 }, CodiceLimiteOltreTetto, "limiti.riconoscimento.max_unita_documento"},
		{"letture per unità oltre il tetto", func(l *Limiti) { l.Riconoscimento.MaxLetturePerUnita = 10001 }, CodiceLimiteOltreTetto, "limiti.riconoscimento.max_letture_per_unita"},
		{"letture per documento oltre il tetto", func(l *Limiti) { l.Riconoscimento.MaxLettureDocumento = 200001 }, CodiceLimiteOltreTetto, "limiti.riconoscimento.max_letture_documento"},
		{"tempo dell'anteprima oltre il tetto", func(l *Limiti) { l.Anteprima.TempoMassimoMs = 60001 }, CodiceLimiteOltreTetto, "limiti.anteprima.tempo_massimo_ms"},
	}
	for _, c := range cambi {
		t.Run(c.nome, func(t *testing.T) {
			lim := limitiACME()
			c.cambia(&lim)
			ds := lim.Valida()
			if len(ds) != 1 {
				t.Fatalf("attesa una diagnostica sola:%s", elenca(ds))
			}
			d := richiedi(t, ds, c.codice, c.percorso)
			if d.Gravita != evidenze.GravitaErrore || d.Natura != evidenze.NaturaContratto {
				t.Fatalf("%s: %s %s, atteso errore di contratto", c.codice, d.Gravita, d.Natura)
			}
		})
	}
}

// TestA1aLIMSenzaLimitiNessunaGrammatica: con limiti non validi Valida non guarda la grammatica e non la
// lascia passare: restituisce solo le diagnostiche dei limiti, che sono errori.
func TestA1aLIMSenzaLimitiNessunaGrammatica(t *testing.T) {
	g, err := Decodifica([]byte(grammaticaACME))
	if err != nil {
		t.Fatal(err)
	}
	for _, lim := range []Limiti{{}, func() Limiti { l := limitiACME(); l.Grammatica.MaxFamiglie = 1000; return l }()} {
		ds := g.Valida(lim)
		if len(ds) == 0 || len(errori(ds)) != len(ds) {
			t.Fatalf("limiti non validi: attesi solo errori:%s", elenca(ds))
		}
		for _, d := range ds {
			if !strings.HasPrefix(d.Percorso, "limiti") {
				t.Fatalf("con limiti non validi Valida ha guardato la grammatica:%s", elenca(ds))
			}
		}
	}
}

// TestA1aLIMValidazioneSuperata: ogni limite di validazione dell'indice, superato, è un errore della
// grammatica (limite.superato, natura limite), con il percorso dell'elemento.
func TestA1aLIMValidazioneSuperata(t *testing.T) {
	casi := []struct {
		nome     string
		cambia   func(*LimitiGrammatica)
		raw      []byte
		percorso string
	}{
		{"famiglie", func(l *LimitiGrammatica) { l.MaxFamiglie = 3 }, nil, "famiglie_codice"},
		{"forme della famiglia", func(l *LimitiGrammatica) { l.MaxFormePerFamiglia = 2 }, nil, "famiglie_codice[acme-prefisso].forme"},
		{"parti della forma", func(l *LimitiGrammatica) { l.MaxPartiPerForma = 3 }, nil, "famiglie_codice[acme-documento].forme[documento].parti"},
		{"parti della sequenza interna", func(l *LimitiGrammatica) { l.MaxPartiPerForma = 3 },
			variante(t, `{"tipo": "ripetizione_base", "min": 1, "max": 1}]},`, `{"tipo": "ripetizione_base", "min": 1, "max": 1}, {"tipo": "token", "letterali": ["A"], "rimando": "Q1", "min": 0, "max": 1}, {"tipo": "token", "letterali": ["B"], "rimando": "Q1", "min": 0, "max": 1}]},`),
			"famiglie_codice[acme-documento].decorazioni[suffisso].parti"},
		{"esempi della famiglia", func(l *LimitiGrammatica) { l.MaxEsempi = 3 }, nil, "famiglie_codice[acme-prefisso].esempi"},
		{"lunghezza di un pattern", func(l *LimitiGrammatica) { l.MaxLunghezzaPattern = 8 }, nil, "famiglie_codice[acme-prefisso].base.segmenti[numero].pattern"},
		{"lunghezza di un letterale", func(l *LimitiGrammatica) { l.MaxLunghezzaLetterale = 7 }, nil, "famiglie_codice[acme-prefisso].decorazioni[pacchetto].letterali[0]"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			lim := limitiACME()
			c.cambia(&lim.Grammatica)
			raw := c.raw
			if raw == nil {
				raw = []byte(grammaticaACME)
			}
			d := richiedi(t, diagnosiDi(t, raw, lim), CodiceLimiteSuperato, c.percorso)
			if d.Gravita != evidenze.GravitaErrore || d.Natura != evidenze.NaturaLimite {
				t.Fatalf("limite superato in validazione: %s %s, atteso errore di limite", d.Gravita, d.Natura)
			}
		})
	}
	// La ripetizione di un pattern: il codice proprio del pattern, con il limite ricevuto.
	lim := limitiACME()
	lim.Grammatica.MaxRipetizione = 5
	richiedi(t, diagnosiDi(t, []byte(grammaticaACME), lim), CodicePatternRipetizioneOltreLimite, "famiglie_codice[acme-prefisso].base.segmenti[numero].pattern")
}
