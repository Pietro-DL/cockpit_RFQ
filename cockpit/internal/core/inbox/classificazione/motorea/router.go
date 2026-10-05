package motorea

import (
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// VersioneRouter: la versione della tabella del router (5.4.6). Sta nell'identità dell'interpretazione, non
// nello snapshot delle regole (R41 c): cambiare una riga vuol dire cambiare questa versione, con un commit che
// lo dichiara. router-2 (correzione di A1b.10, E2): un uso «pertinente» conferma solo con origine «operatore» o
// «scenario»; con origine «riconoscimento» (o un'origine non dichiarata) è un candidato e vale come
// «da_valutare». router-1 non guardava l'origine.
const VersioneRouter = "router-2"

// Funzione: l'uso semantico di una lettura, deciso dal router (parte 1 §4.3, con FunzAttributo del §13).
type Funzione string

const (
	FunzRichiesta    Funzione = "richiesta"
	FunzIdentitaFile Funzione = "identita_file"
	FunzStruttura    Funzione = "struttura"
	FunzRelazione    Funzione = "relazione"
	FunzAttributo    Funzione = "attributo"
	FunzMenzione     Funzione = "menzione"
)

// Gli usi che il router guarda (Instradamento.Uso). I primi tre sono quelli di una selezione
// (evidenze.SelezioneSegmento); «sconosciuto» vale per un segmento senza selezione o per l'uso sconosciuto del
// documento; «non_applicabile» per le unità che non stanno in un messaggio (file, cartiglio, STEP, PDF).
const (
	UsoPertinente     = "pertinente"
	UsoEscluso        = "escluso"
	UsoDaValutare     = "da_valutare"
	UsoSconosciuto    = "sconosciuto"
	UsoNonApplicabile = "non_applicabile"
)

// Le origini di una selezione (evidenze.SelezioneSegmento) che confermano un uso pertinente: la scelta
// dell'operatore e lo scenario del banco, che simula esplicitamente il caso atteso. Un riconoscimento automatico
// propone un candidato, la conferma reale è dell'operatore (E2). È una lista bianca: un'origine vuota o che il
// vocabolario aggiungesse non conferma niente finché il router non la nomina.
const (
	origineOperatore = "operatore"
	origineScenario  = "scenario"
)

// Instradamento: ciò che il router guarda. Il selettore dice dove si è letto; l'uso e la sua origine contano
// solo per oggetto, corpo e storia (per i file valgono «non_applicabile» e nessuna origine). Le celle di una
// tabella della mail hanno il selettore del segmento in cui la tabella si aggancia, e l'uso di quel segmento:
// chi riempie l'Instradamento lo legge dall'entità «riga» della cella (legami-1), così le celle passano dalle
// righe 1-5 come il loro segmento (riga 6).
type Instradamento struct {
	Selettore evidenze.Selettore
	Uso       string // pertinente | escluso | da_valutare | sconosciuto | non_applicabile
	Origine   string // operatore | riconoscimento | scenario | "" (nessuna selezione)
}

// rigaRouter: una riga della tabella del router. I selettori sono nella forma testuale degli attesi
// («cartiglio.codice»); usi vuoto vuol dire «qualunque uso».
type rigaRouter struct {
	numero    int
	selettori []string
	usi       []string
	funzione  Funzione
	motivo    string
}

// tabellaRouter: router-2 (5.4.6; P1 §4.3, riletto sui soli ruoli con il v3 §2). È fissa, e non nomina nessun
// cliente: tutto ciò che è specifico sta nelle grammatiche (par.12). La riga 6 (celle) non è una riga a sé: la
// cella porta il selettore e l'uso del suo segmento, e passa dalle righe 1-5. Le righe dei selettori riservati
// in A1 (voce_archivio, elenco_pdf, metadati_pdf, cartiglio.particolare_simile, …) ci sono lo stesso: la tabella
// è il router, e una grammatica che le attiverà non cambierà il codice.
//
// L'ordine conta solo per oggetto e corpo (righe 1-3) e per la storia (righe 4-5): la prima riga che vale vince,
// e la riga 5 prende ogni uso che la 4 lascia. Le righe guardano l'uso per il router (usoPerIlRouter): un uso
// pertinente senza un'origine che conferma passa dalla riga 2 o dalla 5, mai dalla 1 o dalla 4.
var tabellaRouter = []rigaRouter{
	{1, []string{"oggetto", "corpo"}, []string{UsoPertinente}, FunzRichiesta,
		"router-2 riga 1: oggetto o corpo di un segmento pertinente: richiesta"},
	{2, []string{"oggetto", "corpo"}, []string{UsoSconosciuto, UsoDaValutare}, FunzRichiesta,
		"router-2 riga 2: oggetto o corpo con la pertinenza del segmento non nota o non confermata: richiesta da confermare, l'incertezza resta nella lettura"},
	{3, []string{"oggetto", "corpo"}, []string{UsoEscluso}, FunzMenzione,
		"router-2 riga 3: oggetto o corpo di un segmento escluso: menzione"},
	{4, []string{"storia"}, []string{UsoPertinente}, FunzRichiesta,
		"router-2 riga 4: segmento di storia selezionato pertinente: richiesta, nel contesto storia"},
	{5, []string{"storia"}, nil, FunzMenzione,
		"router-2 riga 5: storia senza una selezione pertinente confermata: menzione"},
	{7, []string{"nome_file", "voce_archivio"}, nil, FunzIdentitaFile,
		"router-2 riga 7: nome del file o voce d'archivio: identità del file"},
	{8, []string{"cartiglio.codice", "cartiglio.numero_disegno"}, nil, FunzIdentitaFile,
		"router-2 riga 8: codice o numero di disegno del cartiglio: identità del file"},
	{9, []string{"radice_step.id", "radice_step.nome"}, nil, FunzIdentitaFile,
		"router-2 riga 9: id o nome di una radice STEP: identità del file"},
	{10, []string{"nodo_step.id", "nodo_step.nome"}, nil, FunzStruttura,
		"router-2 riga 10: id o nome di un nodo STEP: struttura"},
	{11, []string{"elenco_pdf.codice"}, nil, FunzStruttura,
		"router-2 riga 11: codice dell'elenco particolari del PDF: struttura"},
	{12, []string{"cartiglio.particolare_simile"}, nil, FunzRelazione,
		"router-2 riga 12: particolare simile del cartiglio, letto da una forma attiva: relazione «simile»"},
	{13, []string{"cartiglio.revisione", "cartiglio.materiale", "cartiglio.scala", "cartiglio.titolo",
		"radice_step.revisione", "nodo_step.revisione"}, nil, FunzAttributo,
		"router-2 riga 13: campo di un attributo del cartiglio o dello STEP: attributo"},
	{14, []string{"radice_step.descrizione", "nodo_step.descrizione"}, nil, FunzMenzione,
		"router-2 riga 14: descrizione STEP: menzione, nessun attributo «descrizione» in A1"},
	{15, []string{"elenco_pdf.descrizione", "elenco_pdf.revisione", "elenco_pdf.quantita", "elenco_pdf.posizione"}, nil, FunzAttributo,
		"router-2 riga 15: colonna di un attributo dell'elenco particolari: attributo"},
	{16, []string{"testo_pdf", "metadati_pdf", "cartiglio.sconosciuto", "elenco_pdf.sconosciuto"}, nil, FunzMenzione,
		"router-2 riga 16: testo libero del PDF, metadati o campo sconosciuto: menzione"},
}

// motivoFuoriTabella: un selettore che nessuna riga nomina, o un uso fuori elenco per oggetto e corpo. Con il
// vocabolario chiuso della foglia non succede; se succede, la lettura resta una menzione, mai una richiesta: in
// dubbio niente promozione.
const motivoFuoriTabella = "router-2: selettore o uso fuori tabella: menzione, in dubbio nessuna promozione"

// Instrada è la tabella fissa e versionata del router (VersioneRouter, «router-2»): stessi ingressi, stessa
// funzione. Il motivo è una frase stabile che finisce nella lettura. L'origine conta in un solo caso: un uso
// pertinente conferma solo con origine «operatore» o «scenario», altrimenti vale come «da_valutare»
// (usoPerIlRouter; E2), e il motivo lo dice. Lo scenario è l'ingresso del banco che simula il caso atteso, e
// nell'anteprima o nel prodotto non vale come una decisione dell'operatore. L'origine resta visibile nella
// lettura (LetturaCodice.OrigineUso; v3 §10.2).
func Instrada(i Instradamento) (f Funzione, motivo string) {
	sel := i.Selettore.String()
	// Solo oggetto, corpo e storia guardano l'uso (righe 1-5): per gli altri selettori l'origine non conta.
	uso, candidato := i.Uso, false
	switch i.Selettore.Contesto {
	case evidenze.ContestoOggetto, evidenze.ContestoCorpo, evidenze.ContestoStoria:
		uso = usoPerIlRouter(i.Uso, i.Origine)
		candidato = uso != i.Uso
	}
	for _, r := range tabellaRouter {
		if !contiene(r.selettori, sel) {
			continue
		}
		if len(r.usi) > 0 && !contiene(r.usi, uso) {
			continue
		}
		if candidato {
			return r.funzione, r.motivo + motivoCandidato
		}
		return r.funzione, r.motivo
	}
	return FunzMenzione, motivoFuoriTabella
}

// motivoCandidato: la parte del motivo che dice perché un uso pertinente non ha dato la riga della scelta
// esplicita (E2).
const motivoCandidato = "; uso pertinente senza una scelta dell'operatore o dello scenario (un riconoscimento automatico): un candidato, non una conferma, vale come da valutare"

// usoPerIlRouter: l'uso con cui il router sceglie la riga. Un uso «pertinente» conferma solo con origine
// «operatore» o «scenario». Con origine «riconoscimento», o con un'origine che il router non nomina, è un
// candidato, non una conferma (E2: la conferma reale è dell'operatore): vale come «da_valutare», cioè la riga 2
// per oggetto e corpo (richiesta da confermare, con l'incertezza nella lettura e motore.pertinenza_ignota) e la
// riga 5 per la storia (menzione: la storia non si promuove in automatico, 5.1; R48 A). La lettura conserva
// l'uso e l'origine veri (LetturaCodice.Uso, OrigineUso). Nessun punteggio e nessun intento nuovo: la
// distinzione fra candidato e confermato usa gli usi che ci sono già.
func usoPerIlRouter(uso, origine string) string {
	if uso == UsoPertinente && origine != origineOperatore && origine != origineScenario {
		return UsoDaValutare
	}
	return uso
}

// daConfermare: una richiesta con questo uso è da confermare (riga 2): uso sconosciuto, da valutare, o
// pertinente senza un'origine che conferma. Il servizio di proposta non la tratta come confermata (P1 §4.3).
func daConfermare(uso, origine string) bool {
	u := usoPerIlRouter(uso, origine)
	return u == UsoSconosciuto || u == UsoDaValutare
}

// RuoliAmmessi: i ruoli che una funzione lascia passare nell'intersezione (parte 1 §6, riletta sui ruoli dal v3
// §2). richiesta → {prodotto}; identita_file → {prodotto, componente}; struttura → {componente}; relazione,
// attributo, menzione → nessuno. L'elenco è nuovo a ogni chiamata, nell'ordine del vocabolario.
func RuoliAmmessi(f Funzione) []grammatica.Ruolo {
	switch f {
	case FunzRichiesta:
		return []grammatica.Ruolo{grammatica.RuoloProdotto}
	case FunzIdentitaFile:
		return []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente}
	case FunzStruttura:
		return []grammatica.Ruolo{grammatica.RuoloComponente}
	}
	return nil
}

// ordineRuoli: l'ordine in cui si scrivono i ruoli, quello del vocabolario della grammatica.
var ordineRuoli = []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente}

// intersecaRuoli: i ruoli candidati di una lettura, Ruoli(famiglia) ∩ RuoliAmmessi(funzione) (parte 1 §6 r.237),
// con il motivo per esteso, per esempio «famiglia {prodotto, componente} ∩ struttura {componente} =
// {componente}». Le categorie della famiglia non entrano: sono un'annotazione (v3 §2; R7). Il risultato è
// nell'ordine del vocabolario; nil se l'intersezione è vuota.
func intersecaRuoli(famiglia []grammatica.Ruolo, f Funzione) ([]grammatica.Ruolo, string) {
	ammessi := RuoliAmmessi(f)
	var out []grammatica.Ruolo
	for _, r := range ordineRuoli {
		if inRuoli(r, famiglia) && inRuoli(r, ammessi) {
			out = append(out, r)
		}
	}
	return out, "famiglia " + insiemeRuoli(famiglia) + " ∩ " + string(f) + " " + insiemeRuoli(ammessi) + " = " + insiemeRuoli(out)
}

// insiemeRuoli: un insieme di ruoli scritto nell'ordine del vocabolario, «∅» se vuoto. Un ruolo fuori
// vocabolario (la validazione della grammatica lo rifiuta prima) si scrive in fondo, così non sparisce dal
// motivo.
func insiemeRuoli(rs []grammatica.Ruolo) string {
	var parti []string
	for _, r := range ordineRuoli {
		if inRuoli(r, rs) {
			parti = append(parti, string(r))
		}
	}
	for _, r := range rs {
		if !inRuoli(r, ordineRuoli) && !contiene(parti, string(r)) {
			parti = append(parti, string(r))
		}
	}
	if len(parti) == 0 {
		return "∅"
	}
	return "{" + strings.Join(parti, ", ") + "}"
}

func inRuoli(r grammatica.Ruolo, elenco []grammatica.Ruolo) bool {
	for _, x := range elenco {
		if x == r {
			return true
		}
	}
	return false
}
