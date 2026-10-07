package bancoa

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	valut "promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/dataset"
	"promatec/cockpit/internal/platform/migrazioni"
)

// controlli.go: i controlli del runner (piano 6.4.9, «Controlli del runner», n.1–5; 6.7.2: A1c-L1P-01 e -02 sono i
// n.5, n.1 e n.4 nelle corse del banco). Girano prima degli esiti. Un controllo fallito è una differenza (uscita 1),
// mai un «mancante»; un controllo che non si può fare perché un ingresso manca è NON ESEGUITO (uscita 3). Ogni
// funzione è pura e restituisce l'esito del controllo, che il runner scrive nel rapporto.

// Le voci dei controlli del runner nel rapporto, con i nomi del piano.
const (
	ControlloScenario      = "runner_1_scenario_e_casi"
	ControlloRisoluzione   = "runner_2_id_e_sha_degli_attesi"
	ControlloFonti         = "runner_3_export_e_fonti"
	ControlloCasiIndice    = "runner_4_casi_e_indice"
	ControlloAttesiInteri  = "runner_5_attesi_interi"
	ControlloTraduzione    = "traduzione_degli_attesi"
	ControlloInvariantiC5  = "invarianti_da_rivedere"
	ControlloBaselineRighe = "baseline_righe_e_basi"
	// ControlloLetture: le letture attese dei file della baseline (oltre a base, che è il controllo (2)) e dei file reali,
	// contro l'interpretazione del file; le chiavi booleane delle voci che nessun altro controllo verifica (R-101; la
	// regola delle chiavi accettate).
	ControlloLetture = "letture_e_invarianti_dei_file"
	// ControlloCasiFoto: i thread e i messaggi del file dei casi contro la fotografia (T-B6-74, T-B6-112; R-103).
	ControlloCasiFoto = "casi_e_fotografia"
	// ControlloClientiExport: i clienti dei thread esportati con la ragione sociale negli export (T-B6-104).
	ControlloClientiExport = "clienti_degli_export"
	// ControlloCopia, ControlloSentinelle: la copia del manifest con -dsn (6.4.9, passo 3; R-106; T-B6-105; le
	// sentinelle delegate ad A1c-L4D-01: R117 b).
	ControlloCopia      = "copia"
	ControlloSentinelle = "sentinelle"
)

// La regola delle chiavi accettate (T-B6-102, precisata dall'orchestratore dopo la revisione di P9): una chiave degli
// attesi che il runner accetta è verificata, oppure è dichiarata non verificata, fra i nonFatti del controllo che la
// riguarda, con il percorso e senza valori. Mai accettata e buttata in silenzio. Le chiavi sconosciute restano
// differenze al n.5; le chiavi delle sezioni a forma libera che non sono ID vanno in un elenco informativo (R-109).

// esitoControllo: come è andato un controllo, prima di diventare un Controllo del rapporto.
type esitoControllo struct {
	nome       string
	eseguito   bool
	differenze []string // ognuna conta 1
	nonFatti   []string // le parti che non si sono potute fare (rendono il controllo NON ESEGUITO se non ci sono differenze)
	// nonApplicabili: le parti che in questa corsa non si applicano, perché il piano le assegna all'altra modalità (la
	// riga della C5 dell'altra modalità: R-117). Solo informazione: non cambiano lo stato del controllo.
	nonApplicabili []string
	// ambitiDedotti: le chiavi giudicate su un ambito dedotto dalle chiavi sorelle della voce, con l'unità (le chiavi
	// dei token: T-B6-220, precisata dall'orchestratore con R-140). Solo informazione: non cambiano lo stato del
	// controllo, ma il rapporto dice su quale unità la chiave è stata giudicata.
	ambitiDedotti []string
	// delegato: il controllo lo verifica una prova fuori dal runner, nominata nel motivo (R117 b: le sentinelle). Il
	// controllo non è eseguito dal runner, ma non è NON ESEGUITO: è «delegato», e non decide l'uscita.
	delegato bool
	motivo   string
}

// contestoCorsa: ciò che i controlli devono sapere della corsa per non dare per verificato ciò che non hanno visto.
//   - parziale, scelti: la fotografia ha solo i thread scelti (-thread);
//   - export: la fotografia viene dagli export (le righe decise si confrontano con quelle dell'export, senza deciso_da);
//   - componentiAssenti: la sezione dei componenti è assente (gli export): i finiti non si vedono (R70 A);
//   - nonValutabili: i thread che non si possono valutare per un limite degli ingressi, con il motivo (T-B6-104: il
//     cliente senza la ragione sociale negli export). Ciò che dipende dalla loro valutazione è una parte non
//     verificabile, mai una differenza.
type contestoCorsa struct {
	parziale          bool
	scelti            map[uuid.UUID]bool
	export            bool
	componentiAssenti bool
	nonValutabili     map[uuid.UUID]string
}

// controllo: l'esito come voce del rapporto. Le differenze prevalgono su ciò che non si è potuto fare (R44). Un
// controllo delegato, senza differenze né parti non fatte, è «delegato» (R117 b).
func (e esitoControllo) controllo() Controllo {
	c := Controllo{Nome: e.nome, Stato: ControlloEseguito, Differenze: len(e.differenze), Motivo: e.motivo}
	switch {
	case !e.eseguito && e.delegato && len(e.differenze) == 0 && len(e.nonFatti) == 0:
		c.Stato = ControlloDelegato
	case !e.eseguito:
		c.Stato = ControlloNonEseguito
	case len(e.differenze) == 0 && len(e.nonFatti) > 0:
		c.Stato = ControlloNonEseguito
		c.Motivo = unisciMotivi(c.Motivo, fmt.Sprintf("%d parti non verificabili", len(e.nonFatti)))
	}
	return c
}

func unisciMotivi(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// controlloScenario (n.1; T-B0-15; R29 a; R48 A): gli ingressi del caso dello scenario nel file dei casi coincidono
// con la sezione dello scenario degli attesi:
//   - c'è un caso per il thread dello scenario, con un segmento del messaggio dello scenario (il segmento lo controlla
//     ValidaUso dentro valutazione: un segmento che non esiste nel documento rende il thread non valutato, con
//     errore_valutazione, e il predicato qui sotto non si verifica);
//   - l'autorità del caso è quella degli attesi, tradotta (scenario_richiesta_inoltrata → scenario);
//   - prodotti_confermati_db dice se il thread della fotografia ha un target confermato, con il predicato di R70 A
//     (valutazione.TargetConfermato) sui prodotti valutati del thread.
//
// Senza la sezione dello scenario non c'è niente da confrontare (eseguito, senza differenze); senza il file dei casi
// letto il controllo è NON ESEGUITO. Il predicato non si verifica, ed è una parte non verificabile (mai una differenza),
// se il thread dello scenario non è fra quelli scelti (R-104), se non si valuta per un limite degli ingressi
// (T-B6-104), o se la sezione dei componenti è assente e il thread non ha un target confermato dagli identificativi
// (R-105: i finiti di origine manuale non si vedono; un target confermato trovato vale anche senza i componenti).
func controlloScenario(s *ScenarioAtteso, in valut.Ingressi, casiLetti bool, esito *valut.Esito, cc contestoCorsa) esitoControllo {
	e := esitoControllo{nome: ControlloScenario, eseguito: true}
	switch {
	case s == nil:
		e.motivo = "nessuna sezione dello scenario negli attesi"
		return e
	case !casiLetti:
		e.eseguito, e.motivo = false, "il file dei casi non è stato letto"
		return e
	case s.ThreadID == nil:
		e.differenze = append(e.differenze, "lo scenario degli attesi non dice il thread")
		return e
	}
	caso := in.CasoDelThread(*s.ThreadID)
	if caso == nil {
		e.differenze = append(e.differenze, "il file dei casi non ha un caso per il thread dello scenario")
		return e
	}
	if s.MessaggioID == nil {
		e.differenze = append(e.differenze, "lo scenario degli attesi non dice il messaggio")
	} else {
		trovato := false
		for _, sg := range caso.Segmenti {
			if sg.MessaggioID == *s.MessaggioID {
				trovato = true
			}
		}
		if !trovato {
			e.differenze = append(e.differenze, "il caso dello scenario non ha un segmento del messaggio degli attesi")
		}
	}
	if a, ok := traduciAutorita(s.AutoritaTarget); !ok {
		e.differenze = append(e.differenze, fmt.Sprintf("autorita_target %q non si traduce", s.AutoritaTarget))
	} else if a != string(caso.Autorita) {
		e.differenze = append(e.differenze, fmt.Sprintf("l'autorità del caso è %q, gli attesi dicono %q", string(caso.Autorita), a))
	}
	if s.ProdottiConfermatiDB == nil {
		e.differenze = append(e.differenze, "lo scenario degli attesi non dice prodotti_confermati_db")
		return e
	}
	if esito == nil {
		e.differenze = append(e.differenze, "la valutazione non c'è: il predicato dei target confermati non si verifica")
		return e
	}
	var et *valut.EsitoThread
	for i := range esito.Thread {
		if esito.Thread[i].ThreadID == *s.ThreadID {
			et = &esito.Thread[i]
		}
	}
	switch {
	case et == nil && cc.parziale:
		e.nonFatti = append(e.nonFatti, "scenario_inoltro.prodotti_confermati_db: il thread dello scenario non è fra quelli scelti")
	case et == nil:
		e.differenze = append(e.differenze, "il thread dello scenario non è nella fotografia")
	case cc.nonValutabili[*s.ThreadID] != "":
		e.nonFatti = append(e.nonFatti, "scenario_inoltro.prodotti_confermati_db: "+cc.nonValutabili[*s.ThreadID])
	case et.Motivo == valut.MotivoThreadErroreValutazione:
		e.differenze = append(e.differenze, "il thread dello scenario ha un errore della valutazione: il predicato dei target confermati non si verifica")
	default:
		if d, nf := predicatoConfermati(*et, *s.ProdottiConfermatiDB, cc.componentiAssenti, "scenario_inoltro.prodotti_confermati_db"); d != "" {
			e.differenze = append(e.differenze, d)
		} else if nf != "" {
			e.nonFatti = append(e.nonFatti, nf)
		}
	}
	return e
}

// predicatoConfermati: «il thread ha un target confermato» (R70 A, valutazione.TargetConfermato sui prodotti valutati)
// contro il valore atteso. Restituisce la differenza, oppure la parte non verificabile: senza la sezione dei componenti
// un thread senza target confermati dagli identificativi può averne uno da un finito, quindi il predicato non si decide.
func predicatoConfermati(et valut.EsitoThread, atteso, componentiAssenti bool, percorso string) (differenza, nonFatto string) {
	confermati := 0
	for _, pv := range et.ProdottiValutati {
		if valut.TargetConfermato(pv) {
			confermati++
		}
	}
	switch {
	case confermati > 0 && atteso:
	case confermati > 0:
		return fmt.Sprintf("%s falso, ma il thread ha %d target confermati (R70 A)", percorso, confermati), ""
	case componentiAssenti:
		return "", percorso + ": i componenti non sono nella fotografia, e senza i finiti il predicato di R70 A non si decide"
	case atteso:
		return percorso + " vero, ma il thread non ha target confermati (R70 A)", ""
	}
	return "", ""
}

// controlloRisoluzione (n.2; ATT «fixture_reali»; D7): ogni ID e ogni sha256 degli attesi si risolve nella fotografia.
//   - Le voci di file non risolte e i file in due sezioni (che il gate non conta due volte) vengono dalla traduzione.
//   - Il thread e il messaggio dello scenario.
//   - Gli ID e gli sha256 della conferma dell'albero e dei casi d'integrazione, per chiave: allegato, thread, messaggio,
//     componente (anche padre e figlio), documento, sha256. Un componente non si verifica se la sezione dei
//     componenti è assente (gli export).
//
// parziale: la fotografia ha solo i thread scelti (-thread): un allegato o un thread che non c'è può essere di un altro
// thread, quindi non è una differenza ma una parte non verificabile.
func controlloRisoluzione(s SezioniAttesi, tr traduzione, ix indiceFoto, f fotorfq.Fotografia, parziale bool) esitoControllo {
	e := esitoControllo{nome: ControlloRisoluzione, eseguito: true}
	for _, nr := range tr.nonRisolte {
		if parziale && (strings.HasSuffix(nr, "l'allegato non è nella fotografia") || strings.Contains(nr, "nessun allegato della fotografia")) {
			e.nonFatti = append(e.nonFatti, nr)
			continue
		}
		e.differenze = append(e.differenze, nr)
	}
	e.differenze = append(e.differenze, tr.doppie...)
	e.differenze = append(e.differenze, tr.nomi...)
	if sc := s.Scenario; sc != nil && sc.ThreadID != nil {
		if _, ok := ix.thread[*sc.ThreadID]; !ok {
			if parziale {
				e.nonFatti = append(e.nonFatti, "il thread dello scenario non è fra quelli scelti")
			} else {
				e.differenze = append(e.differenze, "il thread dello scenario non è nella fotografia")
			}
		} else if sc.MessaggioID != nil && ix.messaggi[*sc.MessaggioID] != *sc.ThreadID {
			e.differenze = append(e.differenze, "il messaggio dello scenario non è un messaggio del suo thread")
		}
	}
	componentiAssenti := f.Sezioni[fotorfq.SezioneComponenti].Stato == fotorfq.StatoSezioneAssente
	for _, id := range append(append([]IDAtteso(nil), s.Albero...), s.Integrazione...) {
		if id.Chiave == "sha256" {
			switch {
			case ix.sha[strings.ToLower(id.Valore)]:
			case parziale:
				e.nonFatti = append(e.nonFatti, id.Percorso+": non è fra i contenuti dei thread scelti")
			default:
				e.differenze = append(e.differenze, id.Percorso+": nessun contenuto della fotografia ha questo sha256")
			}
			continue
		}
		u, err := uuid.Parse(id.Valore)
		if err != nil {
			e.differenze = append(e.differenze, id.Percorso+": l'UUID non si legge")
			continue
		}
		var trovato bool
		switch id.Chiave {
		case "allegato_id":
			_, trovato = ix.allegati[u]
		case "thread_id":
			_, trovato = ix.thread[u]
		case "messaggio_id":
			_, trovato = ix.messaggi[u]
		case "documento_id":
			trovato = ix.documenti[u]
		case "componente_id", "padre_id", "figlio_id":
			if componentiAssenti {
				e.nonFatti = append(e.nonFatti, id.Percorso+": i componenti non sono nella fotografia")
				continue
			}
			trovato = ix.componenti[u]
		}
		switch {
		case trovato:
		case parziale:
			e.nonFatti = append(e.nonFatti, id.Percorso+": non è fra i thread scelti")
		default:
			e.differenze = append(e.differenze, id.Percorso+": non si risolve nella fotografia")
		}
	}
	return e
}

// controlloFonti (n.3, solo -exports): gli export del manifest e le fonti degli attesi coincidono, nelle due direzioni
// (6.4.9: «coincidono»). Lo sha256 di ogni export del manifest sta fra le fonti; e ogni sha256 delle fonti (la chiave
// sha256, che negli attesi hanno gli export: il dump ha sha256_file e non entra) è un export del manifest, altrimenti
// un export che gli attesi elencano si vedrebbe solo come sezione assente [T].
func controlloFonti(s *SezioniAttesi, export []string) esitoControllo {
	e := esitoControllo{nome: ControlloFonti, eseguito: true}
	if s == nil {
		e.eseguito, e.motivo = false, "gli attesi non sono stati letti"
		return e
	}
	var manifest []string
	for _, sha := range export {
		manifest = append(manifest, strings.ToLower(sha))
		if !dentro(strings.ToLower(sha), s.Fonti) {
			e.differenze = append(e.differenze, "uno sha256 degli export non è fra le fonti degli attesi: "+sha)
		}
	}
	for _, sha := range s.Fonti {
		if !dentro(sha, manifest) {
			e.differenze = append(e.differenze, "uno sha256 delle fonti degli attesi non è fra gli export del manifest: "+sha)
		}
	}
	return e
}

// controlloCasiIndice (n.4; R29 b C): il file dei casi della voce casi del manifest è quello che l'indice delle regole
// dichiara per l'anteprima (stesso sha256). Se no, banco e anteprima leggerebbero ingressi diversi.
func controlloCasiIndice(shaCasi string, indice *grammatica.IndiceRegole) esitoControllo {
	e := esitoControllo{nome: ControlloCasiIndice, eseguito: true}
	switch {
	case shaCasi == "":
		e.eseguito, e.motivo = false, "il file dei casi non è stato letto"
	case indice == nil:
		e.eseguito, e.motivo = false, "l'indice delle regole non è stato letto"
	case indice.Casi == nil:
		e.differenze = append(e.differenze, "l'indice delle regole non dichiara il file dei casi")
	case !strings.EqualFold(indice.Casi.Sha256, shaCasi):
		e.differenze = append(e.differenze, "il file dei casi del manifest non è quello dell'indice delle regole")
	}
	return e
}

// controlloAttesiInteri (n.5; A1c-L1P-01): gli attesi si leggono per intero, con lo sha256 del manifest: nessuna
// chiave sconosciuta (né per LeggiAttesi né per LeggiSezioni), e i conteggi che gli attesi dichiarano coincidono con le
// loro voci. I numeri del piano non stanno nel codice: il rapporto scrive quante voci ha ogni sezione, e il revisore li
// confronta.
func controlloAttesiInteri(letti bool, motivoNonLetti string, s *SezioniAttesi) esitoControllo {
	e := esitoControllo{nome: ControlloAttesiInteri, eseguito: true}
	switch {
	case !letti && s == nil:
		e.eseguito, e.motivo = false, motivoNonLetti
		return e
	case !letti:
		e.differenze = append(e.differenze, "gli attesi non si leggono: "+motivoNonLetti)
	}
	if s == nil {
		return e
	}
	for _, k := range s.NonTradotte {
		e.differenze = append(e.differenze, "chiave non tradotta: "+k)
	}
	if s.Scenario != nil {
		for _, c := range controllaConteggi(s.Scenario) {
			e.differenze = append(e.differenze, c)
		}
	}
	return e
}

// I nomi dei conteggi dello scenario che il runner sa contare dalle voci [T]: i file per collocazione attesa
// (radice, figlio; fuori_scenario, come gli attesi chiamano la collocazione fuori richiesta), tutti i file, i prodotti.
// Un altro nome non si conta in silenzio: è una chiave non tradotta.
var conteggiNoti = []string{"radice", "figlio", "fuori_scenario", "file", "prodotti"}

// controllaConteggi: i conteggi dichiarati dallo scenario contro le sue voci.
func controllaConteggi(s *ScenarioAtteso) []string {
	var out []string
	for _, c := range s.Conteggi {
		n, noto := contaVociScenario(s, c.Nome)
		switch {
		case !noto:
			out = append(out, "chiave non tradotta: scenario_inoltro.conteggi."+c.Nome)
		case n != c.Valore:
			out = append(out, fmt.Sprintf("scenario_inoltro.conteggi.%s dichiara %d, le voci sono %d", c.Nome, c.Valore, n))
		}
	}
	return out
}

// contaVociScenario: le voci dello scenario per un nome di conteggio, e se il nome è noto.
func contaVociScenario(s *ScenarioAtteso, nome string) (int, bool) {
	n := 0
	switch nome {
	case "radice", "figlio":
		for _, v := range s.File {
			if v.Atteso == nome {
				n++
			}
		}
	case "fuori_scenario":
		for _, v := range s.File {
			if v.Atteso == confronto.AttesoFuori || v.AttesoStato == confronto.StatoAttesoFuoriScenario {
				n++
			}
		}
	case "file":
		n = len(s.File)
	case "prodotti":
		n = len(s.Prodotti)
	default:
		return 0, false
	}
	return n, true
}

// controlloTraduzione: gli errori della traduzione delle sezioni (D8, R-51, le radici vuote, i valori fuori elenco) e
// i valori che non si leggono. Ognuno è una differenza del runner: una voce che si contraddice non entra negli esiti.
func controlloTraduzione(s *SezioniAttesi, tr traduzione) esitoControllo {
	e := esitoControllo{nome: ControlloTraduzione, eseguito: s != nil}
	if s == nil {
		e.motivo = "gli attesi non sono stati letti"
		return e
	}
	e.differenze = append(append(e.differenze, s.Errori...), tr.errori...)
	return e
}

// ---- gli invarianti delle righe decise (baseline (1) e C5) ----

// confrontoRiga: una riga attesa contro la riga della fotografia, campo per campo. Un campo che la fotografia non porta
// (gli export: deciso_da) non si confronta, e lo dice nonConfrontabili: chi lo usa ne fa una parte non verificabile.
// Un campo che quella riga non ha (per esempio confermato_il in una proposta) non arriva qui: è un errore di traduzione
// (campiDellaRiga, R-102); il ramo default resta per prudenza e dà una parte non verificabile, mai un silenzio.
type confrontoRiga struct {
	differenze       []string
	nonConfrontabili []string
}

// confrontaProposta: una riga attesa di proposta contro la proposta attuale del file.
func confrontaProposta(riga []CampoAtteso, p *fotorfq.PropostaAttuale, export bool) confrontoRiga {
	var out confrontoRiga
	if len(riga) == 0 {
		return out
	}
	if p == nil {
		out.differenze = append(out.differenze, "la fotografia non ha la proposta del file")
		return out
	}
	for _, c := range riga {
		var ottenuto *string
		switch c.Nome {
		case "codice":
			ottenuto = p.Codice
		case "rev":
			ottenuto = p.Rev
		case "componente_id":
			ottenuto = uuidTesto(p.ComponenteID)
		case "stato":
			ottenuto = &p.Stato
		case "fonte":
			ottenuto = &p.Fonte
		case "tipo", "tipo_proposto":
			ottenuto = &p.Tipo
		case "deciso_il":
			ottenuto = tempoTesto(p.DecisoIl)
		case "deciso_da":
			if export {
				out.nonConfrontabili = append(out.nonConfrontabili, "proposta.deciso_da (non è negli export)")
				continue
			}
			ottenuto = uuidTesto(p.DecisoDa)
		default:
			out.nonConfrontabili = append(out.nonConfrontabili, "proposta."+c.Nome)
			continue
		}
		if d := differenzaCampo("proposta."+c.Nome, c, ottenuto); d != "" {
			out.differenze = append(out.differenze, d)
		}
	}
	return out
}

// confrontaDocumento: una riga attesa di documento contro il documento confermato che porta il file.
func confrontaDocumento(riga []CampoAtteso, d *fotorfq.DocumentoConfermato) confrontoRiga {
	var out confrontoRiga
	if len(riga) == 0 {
		return out
	}
	if d == nil {
		out.differenze = append(out.differenze, "la fotografia non ha un documento confermato che porti il file")
		return out
	}
	for _, c := range riga {
		var ottenuto *string
		switch c.Nome {
		case "codice":
			ottenuto = d.Codice
		case "rev":
			ottenuto = d.Rev
		case "componente_id":
			ottenuto = uuidTesto(d.ComponenteID)
		case "documento_id":
			s := d.ID.String()
			ottenuto = &s
		case "tipo":
			ottenuto = &d.Tipo
		case "sostituito_da":
			ottenuto = uuidTesto(d.SostituitoDa)
		case "confermato_il":
			ottenuto = tempoTesto(&d.ConfermatoIl)
		case "sha256":
			ottenuto = &d.Sha256
		case "nome_file":
			ottenuto = &d.NomeFile
		default:
			out.nonConfrontabili = append(out.nonConfrontabili, "documento."+c.Nome)
			continue
		}
		if x := differenzaCampo("documento."+c.Nome, c, ottenuto); x != "" {
			out.differenze = append(out.differenze, x)
		}
	}
	return out
}

// differenzaCampo: "" se il campo atteso coincide con quello della fotografia. Un null atteso vuole un campo assente; gli
// istanti si confrontano al millisecondo, in UTC; gli UUID e gli sha256 senza le maiuscole.
func differenzaCampo(nome string, c CampoAtteso, ottenuto *string) string {
	switch {
	case c.Nullo && ottenuto == nil:
		return ""
	case c.Nullo:
		return nome + ": atteso null, nella fotografia c'è un valore"
	case ottenuto == nil:
		return nome + ": atteso un valore, nella fotografia è assente"
	}
	atteso, valore := c.Valore, *ottenuto
	if strings.HasSuffix(nome, "_il") {
		ta, err := tempoAtteso(atteso)
		if err != nil {
			return nome + ": " + err.Error()
		}
		atteso = ta.Format("2006-01-02T15:04:05.000Z07:00")
	}
	if strings.HasSuffix(nome, "_id") || strings.HasSuffix(nome, "_da") || strings.HasSuffix(nome, "sha256") {
		atteso, valore = strings.ToLower(atteso), strings.ToLower(valore)
	}
	if atteso != valore {
		return nome + ": diverso dalla fotografia"
	}
	return ""
}

func uuidTesto(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}

func tempoTesto(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	return &s
}

// righeDelFile: la proposta attuale e il documento confermato che porta un allegato, nella fotografia.
func righeDelFile(t *fotorfq.Thread, allegato uuid.UUID) (*fotorfq.PropostaAttuale, *fotorfq.DocumentoConfermato) {
	var p *fotorfq.PropostaAttuale
	var d *fotorfq.DocumentoConfermato
	for i := range t.Proposte {
		if t.Proposte[i].AllegatoID == allegato {
			p = &t.Proposte[i]
		}
	}
	for i := range t.Documenti {
		for _, a := range t.Documenti[i].Allegati {
			if a == allegato {
				d = &t.Documenti[i]
			}
		}
	}
	return p, d
}

// esitoBaseline: i controlli (1) e (2) di una voce della baseline (6.4.9): le righe di proposta e di documento della
// fotografia uguali alla decisione attesa, e la base letta dal motore nel file uguale ad atteso.base (con le letture
// d'identità del file: Nuovo.Basi, esatto come la chiave «base» dei casi, R25 c). Righe e Base dicono che la parte si
// è verificata ed è uguale; Differenze sono gli scarti; NonVerificate le parti che la corsa non può verificare
// (deciso_da con gli export: R-102; il file di un thread che non si valuta per un limite degli ingressi: T-B6-104).
// Una decisione è preservata solo senza differenze e senza parti non verificate.
//
// Revisione: l'indicatore di revisione della riga di confronto del file (valore, le due revisioni, la provenienza della
// vecchia, il motivo), nil se il file non ha una riga. Serve alla voce del gate delle decisioni preservate, che conta le
// differenze di revisione diagnosticate (R113 B ratificata; E2 §3.4); non entra in Righe né in Base, e non toglie la
// preservazione: l'indicatore è separato dalla correttezza dell'associazione (R113).
type esitoBaseline struct {
	Percorso      string                         `json:"percorso"`
	ID            string                         `json:"id,omitempty"`
	AllegatoID    uuid.UUID                      `json:"allegato_id"`
	Righe         bool                           `json:"righe"`
	Base          bool                           `json:"base"`
	Differenze    []string                       `json:"differenze,omitempty"`
	NonVerificate []string                       `json:"non_verificate,omitempty"`
	Revisione     *confronto.IndicatoreRevisione `json:"revisione,omitempty"`
}

// controllaBaseline: (1) e (2) per ogni voce risolta della baseline. Per un file che il motore non ha valutato la
// differenza dice «file non valutato», con il motivo del motore (R-115), non «la base è diversa». revisioni: gli
// indicatori di revisione delle righe di confronto, per allegato (R113 B).
func controllaBaseline(tr traduzione, f fotorfq.Fotografia, ix indiceFoto, nuovi map[uuid.UUID]confronto.File,
	revisioni map[uuid.UUID]confronto.IndicatoreRevisione, cc contestoCorsa) []esitoBaseline {
	var out []esitoBaseline
	for _, vt := range tr.voci {
		if vt.voce.Sezione != confronto.SezioneBaseline || !vt.risolta {
			continue
		}
		t := &f.Thread[ix.thread[vt.thread]]
		p, d := righeDelFile(t, vt.file.AllegatoID)
		eb := esitoBaseline{Percorso: vt.voce.Percorso, ID: vt.voce.ID, AllegatoID: vt.file.AllegatoID}
		if r, ok := revisioni[vt.file.AllegatoID]; ok {
			eb.Revisione = &r
		}
		cp := confrontaProposta(vt.voce.Proposta, p, cc.export)
		cd := confrontaDocumento(vt.voce.Documento, d)
		eb.Differenze = append(append(eb.Differenze, cp.differenze...), cd.differenze...)
		eb.NonVerificate = append(append(eb.NonVerificate, cp.nonConfrontabili...), cd.nonConfrontabili...)
		if atteso, ok := vt.voce.invariante("componente_id_proposta_vuoto_preservato"); ok {
			if p == nil || (p.ComponenteID == nil) != atteso {
				eb.Differenze = append(eb.Differenze, "componente_id_proposta_vuoto_preservato non coincide")
			}
		}
		eb.Righe = len(eb.Differenze) == 0
		n, ok := nuovi[vt.file.AllegatoID]
		base := vt.file.BaseLetta
		switch {
		case cc.nonValutabili[vt.thread] != "":
			eb.NonVerificate = append(eb.NonVerificate, "atteso.base: "+cc.nonValutabili[vt.thread])
		case !ok || !n.Nuovo.Valutato:
			motivo := "file non valutato"
			if ok && n.Nuovo.Motivo != "" {
				motivo += ": " + n.Nuovo.Motivo
			}
			eb.Differenze = append(eb.Differenze, motivo+" (la base letta non c'è)")
		case len(n.Nuovo.Basi) == 1 && n.Nuovo.Basi[0] == base:
			eb.Base = true
		default:
			eb.Differenze = append(eb.Differenze, "la base letta nel file non è atteso.base")
		}
		out = append(out, eb)
	}
	return out
}

// controlloBaselineDi: i controlli (1) e (2) della baseline come voce dei controlli: una voce che non li passa è una
// differenza (il gate la conta anche fra le decisioni preservate); una parte non verificata è fra i nonFatti, quindi il
// controllo senza differenze è NON ESEGUITO, come la C5 per lo stesso caso (R-102).
func controlloBaselineDi(eb []esitoBaseline, letti bool) esitoControllo {
	e := esitoControllo{nome: ControlloBaselineRighe, eseguito: letti}
	if !letti {
		e.motivo = "gli attesi non sono stati letti"
		return e
	}
	for _, x := range eb {
		for _, d := range x.Differenze {
			e.differenze = append(e.differenze, x.Percorso+": "+d)
		}
		for _, d := range x.NonVerificate {
			e.nonFatti = append(e.nonFatti, x.Percorso+": "+d)
		}
	}
	return e
}

// controlloInvariantiC5 (6.4.9, «La sezione C5 nel runner»): per ogni file della C5,
//   - con -dsn le righe della fotografia uguali a decisione_proposta e decisione_documento (sovrascritta_dal_motore
//     falso); con -exports uguali a nell_export_0848 (la riga della proposta dell'export). La riga dell'altra
//     modalità non si applica a questa corsa (il piano assegna ogni riga a una modalità sola, e la verifica la corsa
//     dell'altra): va fra le parti non applicabili, un elenco informativo che non cambia l'uscita (R-117);
//   - componente_id_proposta_vuoto_preservato uguale al valore dichiarato;
//   - in_baseline uguale a «il file è nella baseline degli attesi»; conta_nel_gate falso (la C5 è fuori dal gate);
//   - mostrata_con_badge_di_confronto: il file ha una riga di confronto, e quindi il badge calcolato contro la decisione
//     (R31 a); badge sono i badge del confronto, per allegato.
//
// Le righe della conferma dell'albero (RigheAlbero): il runner ne risolve solo gli ID (n.2), non verifica che siano
// invariate nella fotografia, e lo dice fra i nonFatti (R-109). L'etichetta «da rivedere» e le anomalie restano nel
// rapporto, accanto agli esiti del file.
func controlloInvariantiC5(s *SezioniAttesi, tr traduzione, f fotorfq.Fotografia, ix indiceFoto, cc contestoCorsa, badge map[uuid.UUID]confronto.Badge) esitoControllo {
	e := esitoControllo{nome: ControlloInvariantiC5, eseguito: s != nil}
	if s == nil {
		e.motivo = "gli attesi non sono stati letti"
		return e
	}
	inBaseline := map[uuid.UUID]bool{}
	for _, vt := range tr.voci {
		if vt.voce.Sezione == confronto.SezioneBaseline && vt.risolta {
			inBaseline[vt.file.AllegatoID] = true
		}
	}
	for _, vt := range tr.voci {
		if vt.voce.Sezione != confronto.SezioneDaRivedere || !vt.risolta {
			continue
		}
		t := &f.Thread[ix.thread[vt.thread]]
		p, d := righeDelFile(t, vt.file.AllegatoID)
		var righe []confrontoRiga
		if cc.export {
			righe = append(righe, confrontaProposta(vt.voce.Export, p, true))
			if len(vt.voce.Proposta) > 0 {
				e.nonApplicabili = append(e.nonApplicabili, vt.voce.Percorso+".decisione_proposta: si verifica con -dsn")
			}
			if len(vt.voce.Documento) > 0 {
				e.nonApplicabili = append(e.nonApplicabili, vt.voce.Percorso+".decisione_documento: si verifica con -dsn")
			}
		} else {
			righe = append(righe, confrontaProposta(vt.voce.Proposta, p, false), confrontaDocumento(vt.voce.Documento, d))
			if len(vt.voce.Export) > 0 {
				e.nonApplicabili = append(e.nonApplicabili, vt.voce.Percorso+".nell_export_0848: si verifica con -exports")
			}
		}
		for _, r := range righe {
			for _, x := range r.differenze {
				e.differenze = append(e.differenze, vt.voce.Percorso+": "+x)
			}
			for _, x := range r.nonConfrontabili {
				e.nonFatti = append(e.nonFatti, vt.voce.Percorso+": "+x)
			}
		}
		if v, ok := vt.voce.invariante("mostrata_con_badge_di_confronto"); ok {
			if m := cc.nonValutabili[vt.thread]; m != "" {
				e.nonFatti = append(e.nonFatti, vt.voce.Percorso+".mostrata_con_badge_di_confronto: "+m)
			} else if _, ha := badge[vt.file.AllegatoID]; ha != v {
				e.differenze = append(e.differenze, vt.voce.Percorso+": mostrata_con_badge_di_confronto non coincide con le righe del confronto")
			}
		}
		if v, ok := vt.voce.invariante("sovrascritta_dal_motore"); ok && v {
			e.differenze = append(e.differenze, vt.voce.Percorso+": sovrascritta_dal_motore vero: una decisione sovrascritta non è un invariante")
		}
		if v, ok := vt.voce.invariante("componente_id_proposta_vuoto_preservato"); ok && (p == nil || (p.ComponenteID == nil) != v) {
			e.differenze = append(e.differenze, vt.voce.Percorso+": componente_id_proposta_vuoto_preservato non coincide")
		}
		if v, ok := vt.voce.invariante("in_baseline"); ok && v != inBaseline[vt.file.AllegatoID] {
			e.differenze = append(e.differenze, vt.voce.Percorso+": in_baseline non coincide con la baseline degli attesi")
		}
		if v, ok := vt.voce.invariante("conta_nel_gate"); ok && v {
			e.differenze = append(e.differenze, vt.voce.Percorso+": conta_nel_gate vero, ma la C5 è fuori dal gate")
		}
	}
	for _, r := range s.RigheAlbero {
		e.nonFatti = append(e.nonFatti, r+": riga della conferma dell'albero; il runner ne risolve gli ID (n.2), non verifica che sia invariata nella fotografia")
	}
	sort.Strings(e.nonFatti)
	return e
}

// ---- le letture attese e le chiavi booleane dei file (R-101) ----

// chiaviNonDirette: le chiavi della tabella dei casi che sul file intero non hanno un confronto diretto: vogliono il
// testo del caso, che è una sola unità (originale_conservato), o gli altri casi dello stesso profilo (forma_distinta).
var chiaviNonDirette = map[string]string{
	"originale_conservato": "vuole il testo di un caso di una sola unità, non un file",
	"forma_distinta":       "vuole gli altri casi dello stesso profilo, non un file",
}

// fileDelThread: l'interpretazione di un file nell'esito e se il suo thread è valutato.
type fileDelThread struct {
	fi       *valut.FileInterpretato
	valutato bool
	cliente  uuid.UUID
}

// controlloLetture (R-101; la regola delle chiavi accettate): per ogni voce risolta,
//   - baseline: le letture attese oltre a base (che è il controllo (2)), con la logica dei casi di A1a, cioè la regola
//     della chiave nella tabella dei casi sull'interpretazione del file;
//   - reali: tutte le letture attese, base compresa, e target_presente con il predicato di R70 A sul thread del file.
//
// Una lettura che la regola dà fallita è una differenza; dove il confronto non è diretto la chiave è fra i nonFatti, con
// il percorso (lettureDelFile dice quando). Il file dei reali resta riservato nel gate (R21 c A): qui si verificano le
// letture. scritture_consentite dello scenario non è una lettura: la dichiara la voce del gate «zero scritture» (R-118).
func controlloLetture(s *SezioniAttesi, tr traduzione, file map[uuid.UUID]fileDelThread, esito *valut.Esito, ins *motorea.InsiemeRegole, cc contestoCorsa) esitoControllo {
	e := esitoControllo{nome: ControlloLetture, eseguito: s != nil}
	if s == nil {
		e.motivo = "gli attesi non sono stati letti"
		return e
	}
	threadDi := map[uuid.UUID]*valut.EsitoThread{}
	if esito != nil {
		for i := range esito.Thread {
			threadDi[esito.Thread[i].ThreadID] = &esito.Thread[i]
		}
	}
	for _, vt := range tr.voci {
		if !vt.risolta {
			continue
		}
		v := vt.voce
		if v.Sezione != confronto.SezioneBaseline && v.Sezione != confronto.SezioneReali {
			continue
		}
		var letture []ChiaveAttesa
		for _, k := range v.Letture {
			if v.Sezione == confronto.SezioneBaseline && k.Chiave == "base" {
				continue
			}
			letture = append(letture, k)
		}
		d, nf, dedotti := lettureDelFileConAmbiti(v.Percorso, letture, file[vt.file.AllegatoID], ins, cc.nonValutabili[vt.thread])
		e.differenze = append(e.differenze, d...)
		e.nonFatti = append(e.nonFatti, nf...)
		e.ambitiDedotti = append(e.ambitiDedotti, dedotti...)
		if atteso, ok := v.invariante("target_presente"); ok {
			p := v.Percorso + ".target_presente"
			et := threadDi[vt.thread]
			switch {
			case cc.nonValutabili[vt.thread] != "":
				e.nonFatti = append(e.nonFatti, p+": "+cc.nonValutabili[vt.thread])
			case et == nil || !et.Valutato:
				e.nonFatti = append(e.nonFatti, p+": il thread del file non è valutato")
			default:
				if x, n := predicatoConfermati(*et, atteso, cc.componentiAssenti, p); x != "" {
					e.differenze = append(e.differenze, x)
				} else if n != "" {
					e.nonFatti = append(e.nonFatti, n)
				}
			}
		}
	}
	sort.Strings(e.nonFatti)
	return e
}

// ambitoDellaChiave (R-116, T-B6-131): le chiavi con un ambito, cioè che parlano di un'unità precisa, si giudicano solo
// sulle letture di quell'unità, mai sull'interpretazione del file intero (altrimenti la revisione del cartiglio farebbe
// passare revisione_dal_nome). Il valore è il contesto dell'unità; vuoto per l'ambito «questo campo», che una voce di
// file non dice. Le revisioni dal token e dal token della base sono del nome (i casi del nome li hanno, e i file reali
// li dichiarano sul nome dell'archivio) [T]; la nota è il testo del PDF (il caso che la definisce è su testo_pdf) [T].
var ambitoDellaChiave = map[string]evidenze.Contesto{
	"revisione_dal_nome":        evidenze.ContestoNomeFile,
	"decorazione_nome_file":     evidenze.ContestoNomeFile,
	"revisione_dal_token":       evidenze.ContestoNomeFile,
	"revisione_da_token_base":   evidenze.ContestoNomeFile,
	"identita_file_da_nota":     evidenze.ContestoTestoPDF,
	"revisione_da_questo_campo": "",
}

// chiaviDelToken (T-B6-200; O-1 della controprova delle risposte): le chiavi dei token della revisione non nominano
// un'unità, ma parlano del token di un'unità precisa. Sul file intero il token di un'altra unità (il cartiglio) farebbe
// passare una voce che parla del nome: un passato falso, contro la regola «verificata, oppure dichiarata non
// verificata». Si giudicano sul solo ambito che la voce degli attesi indica (ambitoDelToken); se l'ambito non si
// determina, sono fra i nonFatti con il motivo. Mai sul file intero.
var chiaviDelToken = map[string]bool{"token_revisione": true, "token_conservato": true, "identita_include_token": true}

// ambitoDelToken: l'ambito che la voce indica per le sue chiavi dei token, cioè quello delle altre chiavi della sua
// mappa «atteso» che hanno un ambito (ambitoDellaChiave), se è uno solo e dice un'unità [T]. Restituisce l'ambito, oppure
// il motivo per cui non si determina: nessuna chiave con un ambito, chiavi con ambiti diversi, l'ambito «questo campo»
// (che una voce di file non dice).
func ambitoDelToken(letture []ChiaveAttesa) (evidenze.Contesto, string) {
	visti := map[evidenze.Contesto]bool{}
	var nomi []string
	for _, k := range letture {
		a, ok := ambitoDellaChiave[k.Chiave]
		if !ok || visti[a] {
			continue
		}
		visti[a] = true
		if a == "" {
			nomi = append(nomi, "«questo campo»")
		} else {
			nomi = append(nomi, string(a))
		}
	}
	sort.Strings(nomi)
	switch {
	case len(visti) == 0:
		return "", "la chiave del token non nomina un'unità e la voce non ne indica una (nessuna chiave con un ambito): sul file intero il confronto non è diretto (T-B6-200)"
	case len(visti) > 1:
		return "", "le chiavi con un ambito della voce indicano unità diverse (" + strings.Join(nomi, ", ") + "): l'ambito della chiave del token non si determina (T-B6-200)"
	case visti[""]:
		return "", "la voce indica l'ambito «questo campo», che una voce di file non dice: l'ambito della chiave del token non si determina (T-B6-200)"
	}
	return evidenze.Contesto(nomi[0]), ""
}

// lettureDelFile: le letture attese di un file contro la sua interpretazione, con le regole della tabella dei casi
// (A1a), sul motore del cliente del thread. Restituisce le differenze e le parti non verificabili, con il percorso.
//   - Il file non è valutato, o il thread non si valuta per un limite degli ingressi: ogni chiave è fra i nonFatti.
//   - Una chiave con un ambito (ambitoDellaChiave) si giudica sulle sole letture dell'unità che nomina. Il nome si legge
//     sempre per intero, anche quando il documento è parziale: le chiavi del nome si giudicano comunque. Un'altra unità
//     vuole l'interpretazione completa. Senza letture di quell'unità, o con l'ambito «questo campo», la chiave è fra i
//     nonFatti.
//   - Una chiave dei token (chiaviDelToken) prende l'ambito che la voce indica (ambitoDelToken), e poi si giudica come
//     una chiave con quell'ambito; se l'ambito non si determina, è fra i nonFatti con il motivo (T-B6-200). L'ambito è
//     dedotto, non dichiarato (T-B6-220, precisata con R-140): un'asserzione negativa (asserzioneNegativa) è fra i
//     nonFatti, perché su un'unità più stretta del file passerebbe a vuoto; un'asserzione positiva si giudica, e
//     l'ambito dedotto si scrive nel rapporto (dedotti).
//   - Una chiave senza ambito si giudica sul file intero, e vuole l'interpretazione completa: un risultato tagliato non
//     si giudica (A1b-22).
//   - Le chiavi che vogliono il testo di un caso o gli altri casi (chiaviNonDirette) e quelle che la regola non decide
//     sono fra i nonFatti.
func lettureDelFile(percorso string, letture []ChiaveAttesa, fd fileDelThread, ins *motorea.InsiemeRegole, nonValutabile string) (differenze, nonFatti []string) {
	differenze, nonFatti, _ = lettureDelFileConAmbiti(percorso, letture, fd, ins, nonValutabile)
	return differenze, nonFatti
}

// asserzioneNegativa: il valore atteso di una chiave dei token dice un'assenza (false, null, la lista vuota): su un
// ambito dedotto non si giudica (R-140), perché sull'unità dedotta l'assenza può essere vera e sul file falsa.
func asserzioneNegativa(v ValoreAtteso) bool {
	switch v.Tipo {
	case TipoNullo:
		return true
	case TipoBooleano:
		return v.Testo == "false"
	case TipoLista:
		return len(v.Elementi) == 0
	}
	return false
}

// lettureDelFileConAmbiti: lettureDelFile, con in più le chiavi giudicate su un ambito dedotto (le chiavi dei token),
// una riga per chiave con l'unità, per il rapporto (R-140).
func lettureDelFileConAmbiti(percorso string, letture []ChiaveAttesa, fd fileDelThread, ins *motorea.InsiemeRegole, nonValutabile string) (differenze, nonFatti, dedotti []string) {
	if len(letture) == 0 {
		return nil, nil, nil
	}
	fermo := nonValutabile
	if fermo == "" && (fd.fi == nil || !fd.valutato || fd.fi.Motivo != "") {
		fermo = "il file non è valutato"
		if fd.fi != nil && fd.fi.Motivo != "" {
			fermo += " (" + fd.fi.Motivo + ")"
		}
	}
	if fermo != "" {
		for _, k := range letture {
			nonFatti = append(nonFatti, percorso+".atteso."+k.Chiave+": "+fermo)
		}
		return nil, nonFatti, nil
	}
	stato := fd.fi.Interpretazione.Stato
	completa := stato == motorea.StatoInterpretazioneCompleta
	motore := motoreDi(ins, fd.cliente)
	scene := map[evidenze.Contesto]*scena{}
	scenaDi := func(ambito evidenze.Contesto, intera bool) *scena {
		chiave := ambito
		if intera {
			chiave = "*"
		}
		if sc := scene[chiave]; sc != nil {
			return sc
		}
		in := fd.fi.Interpretazione
		if !intera {
			in = interpretazioneDellUnita(fd.fi, ambito)
		}
		sc := nuovaScena(evidenze.Selettore{Contesto: ambito}, "", in, motore, fd.cliente)
		for _, k := range letture {
			sc.atteso[k.Chiave] = k.Valore
		}
		scene[chiave] = sc
		return sc
	}
	for _, k := range letture {
		p := percorso + ".atteso." + k.Chiave
		regola, nota := tabellaChiavi[k.Chiave]
		switch {
		case !nota || regola.valuta == nil:
			m := regola.motivo
			if m == "" {
				m = "nessuna regola nella tabella dei casi"
			}
			nonFatti = append(nonFatti, p+": la chiave non si controlla ("+m+")")
			continue
		case chiaviNonDirette[k.Chiave] != "":
			nonFatti = append(nonFatti, p+": "+chiaviNonDirette[k.Chiave])
			continue
		}
		var sc *scena
		ambito, conAmbito := ambitoDellaChiave[k.Chiave]
		dedotto := false
		if chiaviDelToken[k.Chiave] {
			a, motivo := ambitoDelToken(letture)
			switch {
			case motivo != "":
				nonFatti = append(nonFatti, p+": "+motivo)
				continue
			case asserzioneNegativa(k.Valore):
				nonFatti = append(nonFatti, p+": ambito dedotto, asserzione negativa: sull'unità "+string(a)+
					", dedotta dalle chiavi sorelle, un'assenza può passare a vuoto (T-B6-220, R-140)")
				continue
			}
			ambito, conAmbito, dedotto = a, true, true
		}
		if conAmbito {
			switch {
			case ambito == "":
				nonFatti = append(nonFatti, p+": la chiave ha l'ambito «questo campo», che una voce di file non dice")
				continue
			case ambito != evidenze.ContestoNomeFile && !completa:
				nonFatti = append(nonFatti, p+": interpretazione "+stato+": l'unità "+string(ambito)+" può essere tagliata (A1b-22)")
				continue
			}
			sc = scenaDi(ambito, false)
			if len(sc.letture) == 0 && len(sc.menzioni) == 0 && len(sc.attributi) == 0 {
				nonFatti = append(nonFatti, p+": nessuna lettura sull'unità "+string(ambito)+": la chiave ha un ambito, e sul file intero il confronto non è diretto")
				continue
			}
		} else {
			if !completa {
				nonFatti = append(nonFatti, p+": interpretazione "+stato+": un risultato tagliato non si giudica (A1b-22)")
				continue
			}
			sc = scenaDi("", true)
		}
		if dedotto {
			dedotti = append(dedotti, p+": giudicata sull'unità "+string(ambito)+", ambito dedotto dalle chiavi sorelle (T-B6-220)")
		}
		switch v := regola.valuta(sc, k.Valore); v.stato {
		case ChiavePassata:
		case ChiaveFallita:
			differenze = append(differenze, fmt.Sprintf("%s: atteso %s, ottenuto %s%s", p, k.Valore.String(), vuotoONo(v.ottenuto), conMotivo(v.motivo)))
		default:
			nonFatti = append(nonFatti, p+": "+v.stato+conMotivo(v.motivo))
		}
	}
	return differenze, nonFatti, dedotti
}

// interpretazioneDellUnita: l'interpretazione di un file ridotta alle letture e agli attributi delle unità con quel
// contesto. L'unità di una lettura è quella del documento del file (UnitaID); se il documento non la porta, il selettore
// della lettura. Un'unità che non si riconosce non entra: meglio una chiave non verificata che un passato falso.
func interpretazioneDellUnita(fi *valut.FileInterpretato, ambito evidenze.Contesto) motorea.Interpretazione {
	unita := map[string]evidenze.Contesto{}
	for _, u := range fi.Documento.Unita {
		unita[u.ID] = u.Selettore.Contesto
	}
	in := fi.Interpretazione
	in.Letture, in.Attributi = nil, nil
	for _, l := range fi.Interpretazione.Letture {
		c, ok := unita[l.UnitaID]
		if !ok {
			c = l.Forma.Selettore.Contesto
		}
		if c == ambito {
			in.Letture = append(in.Letture, l)
		}
	}
	for _, a := range fi.Interpretazione.Attributi {
		if c, ok := unita[a.UnitaID]; ok && c == ambito {
			in.Attributi = append(in.Attributi, a)
		}
	}
	return in
}

func conMotivo(m string) string {
	if m == "" {
		return ""
	}
	return " (" + m + ")"
}

// ---- i casi e la fotografia (R-103; T-B6-74, T-B6-112) ----

// controlloCasiNellaFoto: il file dei casi contro la fotografia, sempre (non solo per lo scenario, non solo con
// -attesi). Un thread di un caso che la fotografia non ha, un messaggio di un segmento che non c'è o è di un altro
// thread, un messaggio fuori RFQ di un caso che la copia non ha (mancantiCopia, da Carica): il file dei casi contraddice
// i dati, ed è una differenza (uscita 1). Con -thread un caso di un thread non scelto non si guarda. Con gli export i
// messaggi che mancano mancano per costruzione (solo i messaggi in entrata: mancantiExport, e i segmenti su un messaggio
// non esportato): sono parti non verificabili (uscita 3).
func controlloCasiNellaFoto(in valut.Ingressi, letti bool, ix indiceFoto, cc contestoCorsa, mancantiCopia, mancantiExport []uuid.UUID) esitoControllo {
	e := esitoControllo{nome: ControlloCasiFoto, eseguito: letti}
	if !letti {
		e.motivo = "il file dei casi non è stato letto"
		return e
	}
	casoDi := map[uuid.UUID]string{}
	for _, c := range in.Casi {
		if c.ThreadID == nil {
			for _, m := range c.Messaggi {
				casoDi[m] = c.ID
			}
			continue
		}
		th := *c.ThreadID
		if _, ok := ix.thread[th]; !ok {
			if cc.parziale && !cc.scelti[th] {
				continue
			}
			e.differenze = append(e.differenze, fmt.Sprintf("caso %s: il thread %s non è nella fotografia", c.ID, th))
			continue
		}
		for i, sg := range c.Segmenti {
			di, ok := ix.messaggi[sg.MessaggioID]
			switch {
			case !ok && cc.export:
				e.nonFatti = append(e.nonFatti, fmt.Sprintf("caso %s, segmenti[%d]: il messaggio non è negli export (solo i messaggi in entrata)", c.ID, i))
			case !ok:
				e.differenze = append(e.differenze, fmt.Sprintf("caso %s, segmenti[%d]: il messaggio %s non è nella fotografia", c.ID, i, sg.MessaggioID))
			case di != th:
				e.differenze = append(e.differenze, fmt.Sprintf("caso %s, segmenti[%d]: il messaggio %s non è del thread del caso", c.ID, i, sg.MessaggioID))
			}
		}
	}
	for _, m := range mancantiCopia {
		e.differenze = append(e.differenze, fmt.Sprintf("caso %s: il messaggio fuori RFQ %s non è nella copia", casoDi[m], m))
	}
	for _, m := range mancantiExport {
		e.nonFatti = append(e.nonFatti, fmt.Sprintf("caso %s: il messaggio fuori RFQ %s non è negli export (solo i messaggi in entrata)", casoDi[m], m))
	}
	return e
}

// ---- i clienti degli export (T-B6-104) ----

// threadSenzaCliente: con gli export, i thread il cui cliente non ha la ragione sociale nell'export dei clienti
// (l'export di prima della ricreazione del DB non ha gli UUID di oggi). Il motore non li valuta (ragione sociale
// discorde): è un limite degli ingressi, non del motore, e ciò che dipende dalla loro valutazione non si verifica.
func threadSenzaCliente(f fotorfq.Fotografia) map[uuid.UUID]string {
	if f.Origine != fotorfq.OrigineExport {
		return nil
	}
	conRS := map[uuid.UUID]bool{}
	for _, c := range f.Clienti {
		conRS[c.ID] = true
	}
	out := map[uuid.UUID]string{}
	for _, t := range f.Thread {
		if !conRS[t.ClienteID] {
			out[t.ID] = "il cliente del thread non ha la ragione sociale negli export: il thread non si valuta (T-B6-104)"
		}
	}
	return out
}

// controlloClientiExport: il controllo dei clienti degli export. NON ESEGUITO se un thread esportato ha il cliente senza
// la ragione sociale: quei thread non si valutano, e i controlli che ne dipendono hanno le loro parti fra i nonFatti
// (T-B6-104). R115 A, ratificata dall'utente il 07/10 con un vincolo (domande-a1c.md): un export nuovo non si mescola
// al campione del 02/10, che resta con questo limite dichiarato; un campione separato ha un manifest suo. Il controllo
// resta obbligatorio finché l'utente non decide D-R115 (la classe è in classi.go).
func controlloClientiExport(f fotorfq.Fotografia, senza map[uuid.UUID]string) esitoControllo {
	e := esitoControllo{nome: ControlloClientiExport, eseguito: true}
	if len(senza) > 0 {
		e.eseguito = false
		e.motivo = fmt.Sprintf("%d thread esportati su %d hanno il cliente senza la ragione sociale negli export: non si valutano (T-B6-104; R115: limite dichiarato del campione)", len(senza), len(f.Thread))
	}
	return e
}

// ---- la copia del manifest (R-106, R-107; T-B6-105) ----

// controllaCopia: la copia raggiunta contro la sezione copia del manifest (6.4.9, passo 3; 6.6.2). Le decisioni sono qui,
// pure, e le prova la L1:
//   - un ruolo, un database o uno schema diversi dal manifest: la copia non è quella del manifest, NON ESEGUITO, e la
//     corsa si ferma prima di Carica (fermo);
//   - un campo che il manifest non dichiara (ruolo, database, schema, tabelle escluse, sentinelle): una parte non
//     eseguita, con il motivo «il manifest non dichiara …»; la corsa continua, ma il controllo è NON ESEGUITO;
//   - le sentinelle dichiarate: il runner non ha una query per contarle (T-B0-36). Con R117 b, ratificata e ampliata
//     dall'utente il 07/10 (domande-a1c.md), le verifica A1c-L4D-01 (testutil.PoolDump) sull'ambiente della copia
//     intatta, con le impronte di contenuto oltre ai conteggi (B6b, lato QA: T-B6-219): la riga è «delegata», un
//     obbligatorio esterno con la prova e l'ambiente nel motivo, non NON ESEGUITO, e non decide l'uscita. La delega vale
//     solo per quell'ambiente: con un database diverso dal manifest la corsa si è già fermata; se il manifest non
//     dichiara il database, l'ambiente non si identifica e la riga resta NON ESEGUITA.
//
// ultimaMigrazione: la versione dell'ultima migrazione incorporata nel binario (-1 se non si legge).
func controllaCopia(attesa dataset.CopiaAttesa, col migrazioni.Collegamento, ultimaMigrazione int) (copia esitoControllo, fermo bool, sentinelle *esitoControllo) {
	copia = esitoControllo{nome: ControlloCopia, eseguito: true}
	var diversi []string
	switch {
	case attesa.Ruolo == "":
		copia.nonFatti = append(copia.nonFatti, "il manifest non dichiara il ruolo della copia")
	case attesa.Ruolo != col.Utente:
		diversi = append(diversi, "il ruolo non è quello del manifest")
	}
	switch {
	case attesa.Database == "":
		copia.nonFatti = append(copia.nonFatti, "il manifest non dichiara il database della copia")
	case attesa.Database != col.Database:
		diversi = append(diversi, "il database non è quello del manifest")
	}
	switch {
	case attesa.Schema == 0:
		copia.nonFatti = append(copia.nonFatti, "il manifest non dichiara lo schema della copia")
	case attesa.Schema != ultimaMigrazione:
		diversi = append(diversi, "lo schema del binario non è quello del manifest")
	}
	if len(attesa.Escluse) == 0 {
		copia.nonFatti = append(copia.nonFatti, "il manifest non dichiara le tabelle escluse: che non si leggano non si controlla")
	}
	switch {
	case len(attesa.Sentinelle) == 0:
		copia.nonFatti = append(copia.nonFatti, "il manifest non dichiara le sentinelle della copia")
	case attesa.Database == "":
		sentinelle = &esitoControllo{nome: ControlloSentinelle,
			motivo: "il manifest non dichiara il database della copia: la delega ad A1c-L4D-01 vale solo per l'ambiente della copia intatta (R117 b)"}
	default:
		sentinelle = &esitoControllo{nome: ControlloSentinelle, delegato: true,
			motivo: fmt.Sprintf("sentinelle e impronte di contenuto verificate fuori dal runner da A1c-L4D-01 (testutil.PoolDump), sull'ambiente della copia intatta: il database %q della sezione copia del manifest; il runner non le conta e non le calcola (T-B0-36: nessun SQL nuovo) e la riga non decide l'uscita (R117 b)", attesa.Database)}
	}
	if len(diversi) > 0 {
		return esitoControllo{nome: ControlloCopia, motivo: "la copia non è quella del manifest: " + strings.Join(diversi, "; ")}, true, nil
	}
	return copia, false, sentinelle
}

// solaLetturaDi: la sezione sola_lettura del rapporto dai controlli del collegamento. Le tabelle escluse non leggibili
// si contano solo se il controllo delle escluse c'è stato (ControllaSolaLettura si ferma al primo controllo che non
// passa, e le escluse sono l'ultimo): altrimenti il campo resta vuoto, «non controllate» (R-107).
func solaLetturaDi(col migrazioni.Collegamento, err error, escluse []string) *SolaLettura {
	s := &SolaLettura{Ruolo: col.Utente, Database: col.Database, ScritturaPossibile: col.Scrive, Escluse: append([]string(nil), escluse...)}
	if err == nil || len(col.EscluseLeggibili) > 0 {
		n := len(escluse) - len(col.EscluseLeggibili)
		s.EscluseNonLeggibili = &n
	}
	return s
}

// controlloConfronto: le impronte dei confronti dei thread; una che non è lunga 64 è una differenza (R-48).
func controlloConfronto(impronte []string) esitoControllo {
	e := esitoControllo{nome: "confronto", eseguito: true}
	for _, h := range impronte {
		if len(h) != 64 {
			e.differenze = append(e.differenze, "impronta del confronto non calcolata (R-48)")
		}
	}
	if len(e.differenze) > 0 {
		e.motivo = "impronta del confronto non calcolata (R-48)"
	}
	return e
}
