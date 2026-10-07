package bancoa

import (
	"fmt"
	"strings"
)

// classi.go: la classe e l'esito di ogni controllo e di ogni voce del gate, e il riepilogo della chiusura (R116 B,
// precisata dall'utente il 07/10, domande-a1c.md: «Per ogni controllo indica se è obbligatorio, informativo o esterno
// al perimetro approvato, e se è passato, fallito o non eseguito»). Solo il rapporto dei modi dsn ed exports (versione
// 3) li porta; il rapporto di A1a non cambia.
//
// Le classi non cambiano le uscite: una differenza resta 1, un controllo non eseguito resta 3, e il 3 non diventa mai 0
// (R116 B: «Non convertire genericamente il codice 3 in 0»). L'unica riga che non decide l'uscita è quella delle
// sentinelle, delegata ad A1c-L4D-01 (R117 b, ratificata dall'utente il 07/10): un'esclusione specifica e motivata,
// non una conversione generica.
//
// La classificazione è quella della revisione d'impatto delle risposte (R116 §3), con due regole: le tre parti «da
// classificare» (le righe della «Conferma l'albero», le letture dei file parziali fuori dal nome, il n.1 senza i finiti
// con gli export) restano obbligatorie, con la nota «classe da decidere dall'utente (D-R116)»; clienti_degli_export
// resta obbligatorio, con la nota «classe da decidere dall'utente (D-R115)». Le sigle sono quelle delle decisioni aperte
// in domande-a1c.md. Una voce che la tabella non conosce è obbligatoria per difetto: la risposta non autorizza
// un'esenzione generica.

// ClasseControllo: la classe di un controllo o di una voce del gate (R116 B).
type ClasseControllo string

const (
	// ClasseObbligatorio: lo verifica il runner, e senza di lui la parte obbligatoria non è conclusa.
	ClasseObbligatorio ClasseControllo = "obbligatorio"
	// ClasseObbligatorioEsterno: obbligatorio, ma verificato fuori dal runner, con la prova che lo chiude (una L4, G1,
	// G5, le etichette degli attesi); si chiude nel registro.
	ClasseObbligatorioEsterno ClasseControllo = "obbligatorio_esterno"
	// ClasseInformativo: solo informazione; incompleto non impedisce di concludere la parte obbligatoria.
	ClasseInformativo ClasseControllo = "informativo"
	// ClasseFuoriPerimetro: esterno al perimetro approvato, con la motivazione e la sua fonte.
	ClasseFuoriPerimetro ClasseControllo = "fuori_perimetro"
)

// EsitoTriStato: l'esito di un controllo o di una voce del gate, derivato dallo stato (R116 B). Non eseguito vuol dire
// «non eseguito dal runner»: per un obbligatorio esterno la verifica è la prova che la tabella nomina.
type EsitoTriStato string

const (
	TriStatoPassato     EsitoTriStato = "passato"
	TriStatoFallito     EsitoTriStato = "fallito"
	TriStatoNonEseguito EsitoTriStato = "non_eseguito"
)

// ControlloDelegato: lo stato di un controllo verificato fuori dal runner, con la prova nel motivo (R117 b: le
// sentinelle della copia, che verifica A1c-L4D-01). Non è NON ESEGUITO e non decide l'uscita; il suo esito è
// non_eseguito (dal runner) e la chiusura lo mette fra gli obbligatori esterni. Solo nel rapporto della versione 3.
const ControlloDelegato = "delegato"

// Le sedi delle voci della tabella: un controllo del rapporto, una voce del gate, una sezione del rapporto, un
// controllo che non è una corsa del runner.
const (
	SedeControllo = "controllo"
	SedeGate      = "gate"
	SedeRapporto  = "rapporto"
	SedeFuori     = "fuori_dal_runner"
)

// I nomi delle sezioni informative del rapporto nella tabella delle classi.
const (
	SezioneInformativaProdotti       = "prodotti"
	SezioneInformativaCorrezioni     = "correzioni_manuali"
	SezioneInformativaProfilo        = "profilo_limiti"
	SezioneInformativaCensimento     = "censimento"
	SezioneInformativaNonApplicabili = "non_applicabili"
	SezioneInformativaChiaviLibere   = "chiavi_libere"
)

// nomeControlloGate: il controllo del gate con -gate (esitoUscitaGate). Nella chiusura il gate si conta voce per voce,
// ognuna con la sua classe: il controllo che le riassume non entra una seconda volta.
const nomeControlloGate = "gate"

// notaDR116, notaDR115: le note delle classi che l'utente deve ancora decidere, con le sigle delle decisioni aperte in
// domande-a1c.md: D-R116 per le tre parti «da classificare», D-R115 per i clienti degli export.
const (
	notaDR116 = "classe da decidere dall'utente (D-R116)"
	notaDR115 = "classe da decidere dall'utente (D-R115)"
)

// voceClassificata: una riga della tabella statica delle classi: la sede e il nome, la classe, la motivazione, la
// fonte (contratto, decisione, prova); per un obbligatorio esterno la prova che lo chiude; la nota di una classe che
// l'utente deve ancora decidere.
type voceClassificata struct {
	Sede        string
	Nome        string
	Classe      ClasseControllo
	Motivazione string
	Fonte       string
	Prova       string
	Nota        string
}

// tabellaClassi: la classe di ogni voce, una volta sola, nell'ordine del rapporto. È la fonte unica: il rapporto e la
// chiusura la leggono da qui, e il README la riassume.
var tabellaClassi = []voceClassificata{
	// I controlli che leggono il dataset e la copia.
	{SedeControllo, "manifest", ClasseObbligatorio, "il manifest del dataset: senza, nessun ingresso si verifica", "par.3.6.5; 6.4.9; R44", "", ""},
	{SedeControllo, VoceIndice, ClasseObbligatorio, "l'indice delle regole, con gli sha256 delle grammatiche", "6.4.9; R43 B", "", ""},
	{SedeControllo, VoceAttesi, ClasseObbligatorio, "gli attesi del manifest, con lo sha256 (con -attesi)", "6.4.9; P-11, R2", "", ""},
	{SedeControllo, VoceCasi, ClasseObbligatorio, "il file dei casi del manifest, letto prima della fotografia", "6.6.3; T-B6-04", "", ""},
	{SedeControllo, "grammatiche", ClasseObbligatorio, "le grammatiche dell'indice compilate dal motore del prodotto", "6.4.9; R43 B", "", ""},
	{SedeControllo, "sola_lettura", ClasseObbligatorio, "il collegamento alla copia non può scrivere", "6.4.9, passo 2; R92; R32 c", "", ""},
	{SedeControllo, ControlloCopia, ClasseObbligatorio, "la copia raggiunta è quella del manifest: ruolo, database, schema, tabelle escluse", "6.4.9, passo 3; R-106; F0-16", "", ""},
	{SedeControllo, ControlloSentinelle, ClasseObbligatorioEsterno,
		"le sentinelle della copia le conta testutil.PoolDump nella L4 sul dump; il runner non ha una query (T-B0-36: nessun SQL nuovo). La delega vale solo per l'ambiente della copia intatta, il database della sezione copia del manifest, e la riga non decide l'uscita",
		"R117 b, precisata e ampliata dall'utente il 07/10 (domande-a1c.md); T-B6-105", "A1c-L4D-01", ""},
	{SedeControllo, "fotografia", ClasseObbligatorio, "la fotografia letta dal caricatore, in una transazione REPEATABLE READ READ ONLY", "6.4.9, passi 4–5", "", ""},
	{SedeControllo, "export", ClasseObbligatorio, "gli export del manifest, con sha256 e byte, nella fotografia", "6.4.9; A1c-B-02; T-12", "", ""},
	{SedeControllo, "thread", ClasseObbligatorio, "i thread scelti con -thread sono fra quelli esportati", "6.4.9; R-104", "", ""},
	{SedeControllo, "interruzione", ClasseObbligatorio, "una corsa interrotta non è conclusa", "R44", "", ""},
	// Il percorso puro e i controlli del runner.
	{SedeControllo, "valutazione", ClasseObbligatorio, "valutazione.Calcola senza errori di contratto, con l'impronta", "6.4.9, passo 6; F0-14", "", ""},
	{SedeControllo, ControlloCasiFoto, ClasseObbligatorio, "il file dei casi contro la fotografia", "R-103; T-B6-74, T-B6-112", "", ""},
	{SedeControllo, ControlloClientiExport, ClasseObbligatorio,
		"con gli export, i clienti dei thread con la ragione sociale: senza, il thread non si valuta",
		"T-B6-104; R115 A, ratificata dall'utente il 07/10 con un vincolo (domande-a1c.md); D-R115 aperta", "",
		notaDR115},
	{SedeControllo, ControlloScenario, ClasseObbligatorio, "il caso dello scenario coincide con gli attesi; il predicato dei target confermati",
		"6.4.9, n.1; T-B0-15; R29 a; R48 A; R70 A; D-R116 aperta", "",
		"contiene una parte da classificare: con gli export, il n.1 senza i finiti (la sezione dei componenti è assente); " + notaDR116},
	{SedeControllo, ControlloRisoluzione, ClasseObbligatorio, "ogni ID e ogni sha256 degli attesi si risolve nella fotografia", "6.4.9, n.2; D7", "", ""},
	{SedeControllo, ControlloFonti, ClasseObbligatorio, "con gli export, gli export del manifest e le fonti degli attesi coincidono", "6.4.9, n.3", "", ""},
	{SedeControllo, ControlloCasiIndice, ClasseObbligatorio, "il file dei casi è quello che l'indice dichiara per l'anteprima", "6.4.9, n.4; R29 b C", "", ""},
	{SedeControllo, ControlloAttesiInteri, ClasseObbligatorio, "gli attesi si leggono per intero, senza chiavi non tradotte", "6.4.9, n.5; A1c-L1P-01", "", ""},
	{SedeControllo, ControlloTraduzione, ClasseObbligatorio, "le sezioni degli attesi si traducono senza contraddizioni", "6.4.6; 6.4.9; D8, R-51", "", ""},
	{SedeControllo, "confronto", ClasseObbligatorio, "le impronte dei confronti calcolate", "6.4.9; R-48", "", ""},
	{SedeControllo, ControlloBaselineRighe, ClasseObbligatorio, "le righe e le basi della baseline, i controlli (1) e (2)", "6.4.9; R-102", "", ""},
	{SedeControllo, ControlloInvariantiC5, ClasseObbligatorio, "gli invarianti dei file della C5",
		"6.4.9, la C5 nel runner; R-109; R-117; D-R116 aperta", "",
		"contiene una parte da classificare: le righe della «Conferma l'albero», di cui il runner risolve solo gli ID; " + notaDR116},
	{SedeControllo, ControlloLetture, ClasseObbligatorio, "le letture attese e le chiavi booleane dei file della baseline e dei reali",
		"R-101; R-116; A1b-22; D-R116 aperta", "",
		"contiene una parte da classificare: le letture dei file con l'interpretazione parziale, fuori dal nome; " + notaDR116},
	{SedeControllo, "casi_contratto", ClasseObbligatorio, "i casi di contratto degli attesi sul motore del prodotto", "6.4.9; R52 A", "", ""},
	{SedeControllo, nomeControlloGate, ClasseObbligatorio, "il gate con -gate, che decide l'uscita; nella chiusura si conta voce per voce", "6.4.9, il gate nel rapporto; R44; R58 B", "", ""},
	// Le voci del gate.
	{SedeGate, VoceGateFalseAssociazioni, ClasseObbligatorio, "zero false associazioni sui file dello scenario e della baseline", "v3 §6; 6.4.9, voce 1", "", ""},
	{SedeGate, VoceGateConteggiScenario, ClasseObbligatorio, "i conteggi dichiarati dallo scenario contro gli esiti", "v3 §6; 6.4.9, voce 2; R-104, R-108", "", ""},
	{SedeGate, VoceGateDecisioniPreservate, ClasseObbligatorio, "le decisioni della baseline preservate", "v3 §6; 6.4.9, voce 3; R-102", "", ""},
	{SedeGate, VoceGateZeroScritture, ClasseObbligatorioEsterno,
		"la parte del runner è il collegamento in sola lettura; il resto sono gli esiti delle L4, nel registro",
		"6.4.9, voce 4; R92; R58 B; R-118", "A1c-L4D-12, A1c-L4S-09", ""},
	{SedeGate, VoceGateRiservati, ClasseObbligatorio, "nessun riservato contato come passato", "v3 §6; 6.4.9, voce 5; R21 c A; R-110", "", ""},
	{SedeGate, VoceGateMotoreSenzaLLM, ClasseObbligatorioEsterno, "il motore senza LLM lo verificano le L4 e le guardie G1 e G5, nel registro",
		"6.4.9, voce 6; piano 3.5 (MOTORE-SENZA-LLM); R58 B", "A1c-L4S-08, A1c-L4T-01 (con la copia _run), G1, G5", ""},
	{SedeGate, VoceGateAltriProfili, ClasseObbligatorioEsterno, "le forme dei due profili ancora senza etichette: oggi c'è solo il censimento",
		"6.4.9, voce 7; R34 c; M-22; R58 B", "le etichette dei due profili negli attesi (M-22), con l'esito nel registro (R58 B)", ""},
	// Le sezioni informative del rapporto.
	{SedeRapporto, SezioneInformativaProdotti, ClasseInformativo,
		"gli assi, lo stato e il fascicolo sui dati veri sono informazione, mai voci del gate; la fonte attesa dei prodotti dello scenario (PO-29) accanto a quella calcolata",
		"T-B0-16; R69 A; PO-29; R109, precisata dall'utente il 07/10 (domande-a1c.md)", "", ""},
	{SedeRapporto, SezioneInformativaCorrezioni, ClasseInformativo,
		"un indicatore ricostruito delle correzioni necessarie, non il tempo risparmiato; dichiara denominatore, copertura ed esclusi",
		"R30 f A; R114, precisata dall'utente il 07/10 (domande-a1c.md); D-R114 aperta", "", ""},
	{SedeRapporto, SezioneInformativaProfilo, ClasseInformativo, "i massimi osservati contro i limiti dell'indice e i tetti del codice", "R43 B; C-23", "", ""},
	{SedeRapporto, SezioneInformativaCensimento, ClasseInformativo, "i messaggi fuori RFQ dei casi di censimento, forma per forma", "R34 c", "", ""},
	{SedeRapporto, SezioneInformativaNonApplicabili, ClasseInformativo, "le parti che il piano assegna all'altra modalità", "R-117", "", ""},
	{SedeRapporto, SezioneInformativaChiaviLibere, ClasseInformativo, "le chiavi delle sezioni a forma libera che non sono ID", "R-109", "", ""},
	// Fuori dal perimetro approvato.
	{SedeFuori, "PO-30", ClasseFuoriPerimetro,
		"la copia preparata con i gesti dell'operatore non è una corsa del runner: con i target confermati il n.1 fallirebbe per costruzione; PO-29 resta sul dump intatto",
		"E1 §11 («mai nel gate»); R110, precisata e ampliata dall'utente il 07/10 (domande-a1c.md)", "", ""},
}

// classeDi: la riga della tabella per una voce. Una voce che la tabella non conosce è obbligatoria per difetto, con il
// motivo: mai un'esenzione per una voce dimenticata (R116 B).
func classeDi(sede, nome string) voceClassificata {
	for _, v := range tabellaClassi {
		if v.Sede == sede && v.Nome == nome {
			return v
		}
	}
	return voceClassificata{Sede: sede, Nome: nome, Classe: ClasseObbligatorio,
		Motivazione: "voce che la tabella delle classi non conosce: obbligatoria per difetto", Fonte: "R116 B, precisata dall'utente il 07/10"}
}

// esitoDelControllo: l'esito tri-stato di un controllo, dal suo stato: con differenze fallito; eseguito senza differenze
// passato; altrimenti (non eseguito, delegato) non eseguito dal runner.
func esitoDelControllo(c Controllo) EsitoTriStato {
	switch {
	case c.Differenze > 0:
		return TriStatoFallito
	case c.Stato == ControlloEseguito:
		return TriStatoPassato
	}
	return TriStatoNonEseguito
}

// esitoDellaVoce: l'esito tri-stato di una voce del gate: superata passato, non superata fallito, altrimenti non
// eseguito.
func esitoDellaVoce(v VoceGate) EsitoTriStato {
	switch v.Stato {
	case VoceSuperata:
		return TriStatoPassato
	case VoceNonSuperata:
		return TriStatoFallito
	}
	return TriStatoNonEseguito
}

// classificaControlli: i controlli con la classe della tabella e l'esito derivato, in una copia. Lo stato, le
// differenze e il motivo non cambiano.
func classificaControlli(cc []Controllo) []Controllo {
	if cc == nil {
		return nil
	}
	out := make([]Controllo, len(cc))
	for i, c := range cc {
		c.Classe = classeDi(SedeControllo, c.Nome).Classe
		c.Esito = esitoDelControllo(c)
		out[i] = c
	}
	return out
}

// classificaVoci: le voci del gate con la classe della tabella e l'esito derivato. Lo stato e il motivo non cambiano.
func classificaVoci(vv []VoceGate) {
	for i := range vv {
		vv[i].Classe = classeDi(SedeGate, vv[i].Nome).Classe
		vv[i].Esito = esitoDellaVoce(vv[i])
	}
}

// Le conclusioni della parte obbligatoria del runner, distinte dalla completezza del rapporto (R116 B: «distingui la
// conclusione della parte obbligatoria dall'incompletezza del rapporto complessivo»).
const (
	ConclusioneNonConclusa    = "parte obbligatoria del runner non conclusa"
	ConclusioneEsternoFallito = "parte obbligatoria del runner conclusa; un obbligatorio esterno è fallito"
	ConclusioneConEsterni     = "parte obbligatoria del runner conclusa; esterni da chiudere nel registro"
	ConclusioneConclusa       = "parte obbligatoria del runner conclusa"
)

// Chiusura: il riepilogo della chiusura della corsa (R116 B, precisata dall'utente il 07/10). Non decide l'uscita, che
// resta quella dei controlli e, con -gate, del gate: il 3 resta 3.
//   - Conclusione: una delle quattro frasi qui sopra; RapportoCompleto: la parte obbligatoria conclusa, nessun
//     obbligatorio esterno da chiudere, nessun informativo incompleto, e il gate nel rapporto.
//   - Gate: l'esito del gate, o «assente» senza -attesi.
//   - Obbligatori: gli obbligatori del runner (i controlli e le voci del gate), conclusi o no; quelli non passati uno per
//     uno, con le note delle classi da decidere.
//   - Esterni: gli obbligatori esterni, ognuno con la sua prova e il suo esito.
//   - Informativi: gli informativi incompleti, con il motivo.
//   - FuoriPerimetro: le voci esterne al perimetro approvato, con la motivazione.
type Chiusura struct {
	Conclusione      string               `json:"conclusione"`
	RapportoCompleto bool                 `json:"rapporto_completo"`
	Gate             string               `json:"gate"`
	Obbligatori      ObbligatoriDelRunner `json:"obbligatori_del_runner"`
	Esterni          []VoceDiChiusura     `json:"obbligatori_esterni,omitempty"`
	Informativi      []VoceDiChiusura     `json:"informativi_incompleti,omitempty"`
	FuoriPerimetro   []VoceDiChiusura     `json:"fuori_perimetro,omitempty"`
}

// ObbligatoriDelRunner: quanti sono, quanti sono passati, se sono conclusi (tutti passati), e quelli non passati.
type ObbligatoriDelRunner struct {
	Totale     int              `json:"totale"`
	Passati    int              `json:"passati"`
	Conclusi   bool             `json:"conclusi"`
	NonPassati []VoceDiChiusura `json:"non_passati,omitempty"`
}

// VoceDiChiusura: una voce nel riepilogo della chiusura, con la sede, l'esito, il motivo della corsa, la motivazione e
// la fonte della tabella, la prova (per gli esterni) e la nota.
type VoceDiChiusura struct {
	Sede        string        `json:"sede"`
	Nome        string        `json:"nome"`
	Esito       EsitoTriStato `json:"esito,omitempty"`
	Motivo      string        `json:"motivo,omitempty"`
	Motivazione string        `json:"motivazione"`
	Fonte       string        `json:"fonte"`
	Prova       string        `json:"prova,omitempty"`
	Nota        string        `json:"nota,omitempty"`
}

// voceDiChiusura: la voce della chiusura dalla riga della tabella e dalla corsa.
func voceDiChiusura(t voceClassificata, esito EsitoTriStato, motivo string) VoceDiChiusura {
	return VoceDiChiusura{Sede: t.Sede, Nome: t.Nome, Esito: esito, Motivo: motivo, Motivazione: t.Motivazione, Fonte: t.Fonte,
		Prova: t.Prova, Nota: t.Nota}
}

// chiusuraDi: il riepilogo della chiusura dal rapporto, con le classi della tabella. I controlli e le voci nell'ordine
// del rapporto; il controllo del gate non entra (le sue voci sì, una per una); poi le sezioni informative e le voci
// fuori dal perimetro.
func chiusuraDi(r RapportoBanco) *Chiusura {
	ch := &Chiusura{Gate: "assente: senza -attesi il gate non c'è"}
	esterniFalliti := false
	conta := func(t voceClassificata, esito EsitoTriStato, motivo string) {
		switch t.Classe {
		case ClasseObbligatorio:
			ch.Obbligatori.Totale++
			if esito == TriStatoPassato {
				ch.Obbligatori.Passati++
			} else {
				ch.Obbligatori.NonPassati = append(ch.Obbligatori.NonPassati, voceDiChiusura(t, esito, motivo))
			}
		case ClasseObbligatorioEsterno:
			if esito == TriStatoFallito {
				esterniFalliti = true
			}
			ch.Esterni = append(ch.Esterni, voceDiChiusura(t, esito, motivo))
		case ClasseInformativo:
			if esito != TriStatoPassato {
				ch.Informativi = append(ch.Informativi, voceDiChiusura(t, esito, motivo))
			}
		case ClasseFuoriPerimetro:
			ch.FuoriPerimetro = append(ch.FuoriPerimetro, voceDiChiusura(t, esito, motivo))
		}
	}
	for _, c := range r.Controlli {
		if c.Nome == nomeControlloGate {
			continue
		}
		conta(classeDi(SedeControllo, c.Nome), esitoDelControllo(c), c.Motivo)
	}
	if g := r.Gate; g != nil {
		ch.Gate = g.Esito
		for _, v := range g.Voci {
			conta(classeDi(SedeGate, v.Nome), esitoDellaVoce(v), v.Motivo)
		}
	}
	for _, s := range informativiIncompleti(r) {
		ch.Informativi = append(ch.Informativi, voceDiChiusura(classeDi(SedeRapporto, s[0]), "", s[1]))
	}
	for _, t := range tabellaClassi {
		if t.Sede == SedeFuori {
			conta(t, "", "")
		}
	}
	ch.Obbligatori.Conclusi = ch.Obbligatori.Totale > 0 && ch.Obbligatori.Passati == ch.Obbligatori.Totale
	daChiudere := false
	for _, e := range ch.Esterni {
		if e.Esito != TriStatoPassato {
			daChiudere = true
		}
	}
	switch {
	case !ch.Obbligatori.Conclusi:
		ch.Conclusione = ConclusioneNonConclusa
	case esterniFalliti:
		ch.Conclusione = ConclusioneEsternoFallito
	case daChiudere:
		ch.Conclusione = ConclusioneConEsterni
	default:
		ch.Conclusione = ConclusioneConclusa
	}
	// Senza il gate (senza -attesi) il rapporto non è mai completo: le voci del gate e i controlli del runner 1–5 non ci
	// sono, anche se gli obbligatori di questa corsa sono passati (T-B6-194).
	ch.RapportoCompleto = ch.Conclusione == ConclusioneConclusa && len(ch.Informativi) == 0 && r.Gate != nil
	return ch
}

// informativiIncompleti: le sezioni informative del rapporto che la corsa non ha completato, con il motivo, in ordine
// di tabella:
//   - prodotti: assente, o non calcolata dove manca una sezione della fotografia (T-12); con la fonte contro l'atteso dei
//     prodotti dello scenario, la derivazione dagli attesi, esplicita e indipendente dal motore, che arriva in B6b
//     (R109, precisata dall'utente il 07/10);
//   - correzioni_manuali: assenti, o con decisi esclusi dal denominatore (la copertura non è piena) o thread non
//     valutati a parte;
//   - profilo_limiti, censimento: solo se assenti;
//   - non_applicabili e chiavi_libere: se ce ne sono.
func informativiIncompleti(r RapportoBanco) [][2]string {
	var out [][2]string
	letta := r.Fotografia != nil
	if p := r.Prodotti; p == nil {
		if letta {
			out = append(out, [2]string{SezioneInformativaProdotti, "non calcolata"})
		}
	} else {
		var m []string
		if !p.Calcolata {
			m = append(m, "non calcolata dove manca: "+strings.Join(p.SezioniAssenti, ", "))
		}
		fonti := 0
		for _, t := range p.Thread {
			for _, x := range t.Prodotti {
				if x.Fonte != nil {
					fonti++
				}
			}
		}
		if fonti > 0 {
			m = append(m, fmt.Sprintf("%d prodotti dello scenario con la fonte contro l'atteso: la derivazione dagli attesi, esplicita e indipendente dal motore, arriva in B6b, prima di Q10 (R109)", fonti))
		}
		if len(m) > 0 {
			out = append(out, [2]string{SezioneInformativaProdotti, strings.Join(m, "; ")})
		}
	}
	if letta && len(r.Thread) > 0 && len(r.CorrezioniManuali) == 0 {
		out = append(out, [2]string{SezioneInformativaCorrezioni, "non calcolate"})
	}
	decisi, valutabili, aParte := 0, 0, 0
	for _, c := range r.CorrezioniManuali {
		decisi += c.Correzioni.Decisi
		valutabili += c.Correzioni.Valutabili
		aParte += c.ThreadNonValutati
	}
	if decisi > valutabili || aParte > 0 {
		out = append(out, [2]string{SezioneInformativaCorrezioni,
			fmt.Sprintf("copertura %d su %d decisi (gli esclusi per motivo nella sezione); %d thread non valutati a parte", valutabili, decisi, aParte)})
	}
	if letta && r.ProfiloLimiti == nil {
		out = append(out, [2]string{SezioneInformativaProfilo, "non calcolato"})
	}
	na := 0
	for _, d := range r.Dettagli {
		na += len(d.NonApplicabili)
	}
	if na > 0 {
		out = append(out, [2]string{SezioneInformativaNonApplicabili, fmt.Sprintf("%d parti non applicabili in questa corsa: si verificano con l'altra modalità", na)})
	}
	if a := r.Attesi; a != nil && len(a.ChiaviLibere) > 0 {
		out = append(out, [2]string{SezioneInformativaChiaviLibere, fmt.Sprintf("%d chiavi libere non verificate", len(a.ChiaviLibere))})
	}
	return out
}

// testoChiusura: le righe della chiusura nel riepilogo.
func testoChiusura(riga func(string, ...any), ch *Chiusura) {
	completo := "incompleto"
	if ch.RapportoCompleto {
		completo = "completo"
	}
	riga("chiusura: %s; rapporto %s; gate: %s", ch.Conclusione, completo, ch.Gate)
	o := ch.Obbligatori
	riga("  obbligatori del runner: %d passati su %d", o.Passati, o.Totale)
	voce := func(titolo string, v VoceDiChiusura) {
		s := "  " + titolo + ": " + v.Nome
		if v.Esito != "" {
			s += " (" + string(v.Esito) + ")"
		}
		if v.Prova != "" {
			s += ", prova: " + v.Prova
		}
		if v.Motivo != "" {
			s += " — " + v.Motivo
		}
		if v.Nota != "" {
			s += " [" + v.Nota + "]"
		}
		riga("%s", s)
	}
	for _, v := range o.NonPassati {
		voce("obbligatorio non passato", v)
	}
	for _, v := range ch.Esterni {
		voce("obbligatorio esterno", v)
	}
	for _, v := range ch.Informativi {
		voce("informativo incompleto", v)
	}
	for _, v := range ch.FuoriPerimetro {
		s := "  fuori perimetro: " + v.Nome + " — " + v.Motivazione + " (" + v.Fonte + ")"
		riga("%s", s)
	}
}
