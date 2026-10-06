// L1 — gli aiuti delle prove di valutazione (A1c-L1-17, A1c-L1-21 e le prove di target e fonte): la grammatica ACME,
// i messaggi, gli allegati con i fatti STEP del worker, le righe della vista e del gesto 3, costruiti in Go come li
// darebbe il caricatore.
package valutazione_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example), i codici sono di fantasia (712xxxx, con la P di
// fase prototipo nella mail), gli UUID sono 00000000-0000-4000-8000-0000000000nn. Le prove citano i requisiti (R70 A,
// R65 A, T-E1-09, PO-19, PO-34), mai i casi degli attesi.

var (
	clienteACME  = uuid.MustParse("00000000-0000-4000-8000-00000000ac01")
	altroCliente = uuid.MustParse("00000000-0000-4000-8000-00000000ac02")
	operatore    = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	threadACME   = uuid.MustParse("00000000-0000-4000-8000-000000000071")
	idM1         = uuid.MustParse("00000000-0000-4000-8000-000000000081")
	idM2         = uuid.MustParse("00000000-0000-4000-8000-000000000082")
	idM3         = uuid.MustParse("00000000-0000-4000-8000-000000000083")
)

// uid: un UUID di prova dal numero, nella forma 00000000-0000-4000-8000-0000000000nn (nn esadecimale a più cifre).
func uid(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

var dataACME = time.Date(2026, 10, 1, 7, 30, 0, 0, time.UTC)

// ---- la grammatica ACME ----

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

var (
	selMail   = []string{"oggetto", "corpo", "storia"}
	selCodice = []string{"nome_file", "cartiglio.codice", "radice_step.id", "radice_step.nome", "nodo_step.id", "nodo_step.nome"}
)

// famigliaACME: base 712 più quattro cifre. Nella mail con la P di fase prototipo davanti (D2; R48 A: anche sulla
// storia); nei file, nel cartiglio e nello STEP senza la P. Ruoli {prodotto, componente}.
func famigliaACME() grammatica.FamigliaCodice {
	base := grammatica.Base{
		Segmenti:  []grammatica.SegmentoBase{{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}},
		Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
	pBase := grammatica.Parte{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}
	esempio := func(id, sel, testo, forma string, affissi ...string) grammatica.EsempioCodice {
		return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: sel, Testo: testo,
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: forma, Base: "7120100", Affissi: affissi}}}}
	}
	return grammatica.FamigliaCodice{
		ID: "acme-prefisso", Namespace: "acme-prefisso",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base,
		Forme: []grammatica.FormaCodice{
			{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteAffisso, Rif: "P", Min: 1, Max: 1}, pBase}},
			{ID: "codice", Selettori: selCodice, Stato: grammatica.StatoAttiva, Completa: true, Parti: []grammatica.Parte{pBase}},
		},
		Affissi: []grammatica.Affisso{{
			ID: "P", Letterali: []string{"P"}, Posizione: grammatica.PosizionePrefisso,
			Riconoscimento: selMail, Attribuzione: selMail,
			Valore: &grammatica.ValoreQualificatore{Fase: grammatica.FasePrototipo},
		}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-mail", "corpo", "P7120100", "mail", "P"),
			esempio("e-step", "radice_step.id", "7120100", "codice"),
		},
	}
}

func quantitaACME() []grammatica.QuantitaTabellare {
	return []grammatica.QuantitaTabellare{{ID: "q-acme", Intestazioni: []string{"Q.TA"},
		Regola: grammatica.RegolaQuantitaPrimaRigaSopra, Selettori: []string{"corpo", "storia"}, Stato: grammatica.StatoAttiva}}
}

// motoreACME: la grammatica passa dalla porta del prodotto (file v1, NuovoSnapshot, CompilaVerificato).
func motoreACME(t *testing.T) *motorea.Motore {
	t.Helper()
	g := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       []grammatica.FamigliaCodice{famigliaACME()},
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

// ---- i messaggi ----

func testo(s string) *string { return &s }

// messaggio: un messaggio in entrata dal cliente ACME, con l'oggetto e il corpo dati.
func messaggio(id uuid.UUID, minuti int, oggetto, corpo string) fotorfq.Messaggio {
	c := clienteACME
	th := threadACME
	return fotorfq.Messaggio{ID: id, ConversazioneID: uid(0xc6), ThreadID: &th, Canale: "outlook", Direzione: "entrata",
		DataEvento: dataACME.Add(time.Duration(minuti) * time.Minute), MittenteNome: "Ufficio acquisti ACME",
		MittenteIndirizzo: "acquisti@acme.example", Oggetto: testo(oggetto), CorpoTesto: testo(corpo),
		ControparteTipo: "cliente", ControparteClienteID: &c}
}

// gesto: il gesto 1 dell'operatore sul messaggio (aggancio «operatore», con chi e quando).
func gesto(id uuid.UUID) fotorfq.AggancioMessaggio {
	da, il := operatore, dataACME.Add(time.Hour)
	return fotorfq.AggancioMessaggio{MessaggioID: id, Aggancio: "operatore", AgganciatoDa: &da, AgganciatoIl: &il}
}

// threadBase: la RFQ ACME con un messaggio con il gesto 1, senza decisioni.
func threadBase(corpo string) fotorfq.Thread {
	m := messaggio(idM1, 0, "Richiesta di offerta ACME26-030", corpo)
	return fotorfq.Thread{ID: threadACME, ClienteID: clienteACME, Stato: "APERTA", CreatoIl: dataACME,
		Messaggi: []fotorfq.Messaggio{m}, Agganci: []fotorfq.AggancioMessaggio{gesto(idM1)}}
}

// ---- gli allegati e i fatti STEP ----

const versioneAnalizzatore = 4

var hashConfigurazione = strings.Repeat("c", 64)

// nodoSTEP: un PRODUCT dei fatti, con la chiave e l'id grezzo.
type nodoSTEP struct{ chiave, id string }

// fattiSTEP: i fatti di uno STEP alla terna corrente (struttura v3, la forma del worker), con le radici, i nodi e
// gli archi dati (padre, figlio). Un grafo completo ha il motivo parziale ""; incompleto, il motivo dato.
func fattiSTEP(t *testing.T, sha string, radici []string, nodi []nodoSTEP, archi [][2]string, motivoParziale string) fotorfq.Fatti {
	t.Helper()
	var n, r []string
	for _, x := range nodi {
		n = append(n, fmt.Sprintf(`{"chiave": %q, "id_grezzo": %q, "nome_grezzo": "", "evidenza": {}}`, x.chiave, x.id))
	}
	for _, a := range archi {
		r = append(r, fmt.Sprintf(`{"padre": %q, "figlio": %q, "qta": 1, "evidenza": {}}`, a[0], a[1]))
	}
	rad, _ := json.Marshal(radici)
	payload := fmt.Sprintf(`{"struttura": {"versione": 3, "schema": "AP214", "radici": %s, "avvisi": [], "nodi": [%s], "relazioni": [%s], `+
		`"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`,
		rad, strings.Join(n, ", "), strings.Join(r, ", "))
	return fatti(t, sha, payload, &motivoParziale)
}

// fattiSTEPIllegibili: i fatti di uno STEP che il worker non ha letto (nessun nodo, l'avviso del worker): l'estrazione
// è fallita.
func fattiSTEPIllegibili(t *testing.T, sha string) fotorfq.Fatti {
	t.Helper()
	payload := `{"struttura": {"versione": 3, "schema": "", "radici": [], "avvisi": ["non e' un file STEP Part 21"], "nodi": [], "relazioni": [], ` +
		`"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
	return fatti(t, sha, payload, nil)
}

func fatti(t *testing.T, sha, payload string, motivo *string) fotorfq.Fatti {
	t.Helper()
	f := fotorfq.Fatti{Sha256: sha, Terna: fotorfq.Terna{Versione: versioneAnalizzatore, HashConfigurazione: hashConfigurazione},
		CalcolatoIl: dataACME, Payload: json.RawMessage(payload), MotivoParziale: motivo}
	d, err := fotorfq.ImprontaPayload(f.Payload)
	if err != nil {
		t.Fatal(err)
	}
	f.Digest = d
	return f
}

// allegato: un file del messaggio m1, con nome, estensione e sha.
func allegato(id uuid.UUID, indice int16, nome, est, sha string) fotorfq.Allegato {
	return fotorfq.Allegato{ID: id, MessaggioID: idM1, Indice: indice, NomeFile: nome, Estensione: testo(est), Natura: "file",
		Origine: "outlook", Stato: "analizzato", Sha256: testo(sha), RicevutoIl: dataACME}
}

// conAllegato aggiunge al thread un allegato e, se ci sono, i suoi fatti.
func conAllegato(t *fotorfq.Thread, a fotorfq.Allegato, f *fotorfq.Fatti) {
	t.Allegati = append(t.Allegati, a)
	if f != nil {
		if t.Fatti == nil {
			t.Fatti = map[string]fotorfq.Fatti{}
		}
		t.Fatti[f.Sha256] = *f
	}
}

// sha: uno sha256 di prova, ripetendo una cifra esadecimale.
func sha(c string) string { return strings.Repeat(c, 64) }

// fotografia: la fotografia con un thread solo, letta come dal caricatore (tutte le sezioni complete).
func fotografia(t fotorfq.Thread) fotorfq.Fotografia {
	sez := map[string]fotorfq.StatoSezione{}
	for _, k := range fotorfq.ChiaviSezioni {
		sez[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneCompleta}
	}
	return fotorfq.Fotografia{VersioneSchema: fotorfq.VersioneSchema, Origine: fotorfq.OrigineDSN, Coerente: true,
		Analizzatore: &fotorfq.Terna{Versione: versioneAnalizzatore, HashConfigurazione: hashConfigurazione},
		Sezioni:      sez, Clienti: []fotorfq.Cliente{{ID: clienteACME, RagioneSociale: "ACME S.p.A."}}, Thread: []fotorfq.Thread{t}}
}
