package bancoa

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// Questo file è la traduzione fra gli attesi e il motore (C-30; par.4.7.5 del piano A): i contesti (R19) e la
// tabella «chiave degli attesi → campo o predicato della lettura». Contiene solo nomi di chiavi, e solo nei
// nomi neutri (R47 a, M-21): nessun nome di cliente, nessun frammento di codice reale. Il runner è il giudice,
// non un secondo motore (R25): da A1b.11 legge l'Interpretazione del caso (estrazione.DaTesto più
// motorea.Interpreta con l'uso sconosciuto, 5.4.5) e la confronta con l'atteso, senza riconoscere niente da sé.
//
// Le letture si dividono con la funzione che il router ha dato (R25 a): l'insieme delle letture d'identità
// (identita_file, struttura, richiesta, contate per occorrenza) è quello che leggono base, basi,
// letture_identita e le chiavi dei campi; le menzioni stanno fuori, e le leggono le loro chiavi
// (base_menzionata…); gli attributi della revisione in campo separato li leggono revisione, stato,
// originale_conservato e cifre. Sui selettori d'identità di A1a (nome del file, codice del cartiglio, id dello
// STEP) e su oggetto e corpo con l'uso sconosciuto (router-1 riga 2) le letture d'identità sono tutte le
// letture di forma: le chiavi di A1a non cambiano significato.
//
// Confronto (R25 c, d): esatto per base, basi, marcatore, affisso e revisione; per le altre chiavi il valore
// atteso deve stare fra quelli letti. Una chiave che l'atteso non nomina non si controlla. Una chiave che il
// runner non sa controllare è «rimandata», con la sessione e il motivo: mai passata.

// TraduciContesto legge il contesto di un caso degli attesi come selettore del motore. Gli attesi scrivono
// «figlio_step.<campo>», che nel vocabolario è nodo_step (R19 a); il resto è la notazione puntata di
// evidenze.LeggiSelettore (R19 b), senza altri ripieghi.
func TraduciContesto(s string) (evidenze.Selettore, error) {
	if campo, ok := strings.CutPrefix(s, "figlio_step."); ok {
		s = string(evidenze.ContestoNodoSTEP) + "." + campo
	}
	return evidenze.LeggiSelettore(s)
}

// Le sessioni di una chiave.
const (
	SessioneA1a      = "A1a"
	SessioneA1b      = "A1b"
	SessioneDecaduta = "decaduta" // chiavi che una modifica degli attesi ha tolto (M-01): note, mai controllate
)

// Gli stati di una chiave nel rapporto.
const (
	ChiavePassata   = "passata"
	ChiaveFallita   = "fallita"
	ChiaveRimandata = "rimandata"
	ChiaveRiservata = "riservata"
)

// regolaChiave: in quale sessione una chiave diventa controllabile e come si controlla.
type regolaChiave struct {
	sessione string
	motivo   string // perché non si controlla (le chiavi decadute)
	valuta   func(c *scena, v ValoreAtteso) valutazione
}

// valutazione: l'esito di una chiave su una scena.
type valutazione struct {
	stato, ottenuto, motivo string
}

// scena: ciò che il runner sa di un caso quando ne valuta le chiavi: l'Interpretazione del documento di
// DaTesto sul motore del profilo, con l'uso sconosciuto (5.4.5), divisa per funzione (R25 a). Solo famiglie
// del profilo (par.4.7.4).
type scena struct {
	sel   evidenze.Selettore
	testo string
	// letture: le letture d'identità (funzione identita_file, struttura o richiesta), una per occorrenza.
	letture []motorea.LetturaForma
	// menzioni: le letture con funzione menzione (testo libero del PDF, storia senza selezione, …), fuori
	// dall'insieme d'identità (R25 a).
	menzioni []motorea.LetturaForma
	// codici: tutte le letture di Interpreta, con la funzione e le altre unità della stessa occorrenza.
	codici []motorea.LetturaCodice
	// attributi: gli attributi «revisione» (in campo separato), legati all'entità (5.4.6 punto 12).
	attributi []motorea.AttributoLetto
	// interp: l'interpretazione intera, per la sua identità (cliente e snapshot).
	interp  motorea.Interpretazione
	cliente uuid.UUID // il cliente a cui il manifest lega il profilo del caso (D-09)
	motore  *motorea.Motore
	atteso  map[string]ValoreAtteso
	// formeAltri: per ogni altro caso dello stesso profilo sullo stesso selettore, le forme delle sue letture
	// d'identità (per forma_distinta).
	formeAltri [][]string
}

// nuovaScena divide l'interpretazione di un caso per funzione (R25 a). La funzione è quella che Interpreta ha
// scritto nella lettura (router-1): il runner non la ricalcola.
func nuovaScena(sel evidenze.Selettore, testo string, in motorea.Interpretazione, m *motorea.Motore, cliente uuid.UUID) *scena {
	sc := &scena{sel: sel, testo: testo, codici: in.Letture, interp: in, motore: m, cliente: cliente, atteso: map[string]ValoreAtteso{}}
	sc.letture, sc.menzioni = dividiLetture(in.Letture)
	for _, a := range in.Attributi {
		if a.Tipo == motorea.AttributoRevisione {
			sc.attributi = append(sc.attributi, a)
		}
	}
	return sc
}

// dividiLetture: le letture di forma delle letture d'identità e delle menzioni, nell'ordine di Interpreta. Le
// letture con funzione relazione o attributo non entrano in nessuna delle due.
func dividiLetture(ls []motorea.LetturaCodice) (identita, menzioni []motorea.LetturaForma) {
	for _, l := range ls {
		switch {
		case funzioneDIdentita(l.Funzione):
			identita = append(identita, l.Forma)
		case l.Funzione == motorea.FunzMenzione:
			menzioni = append(menzioni, l.Forma)
		}
	}
	return identita, menzioni
}

// funzioneDIdentita: le funzioni delle letture d'identità di R25 (a).
func funzioneDIdentita(f motorea.Funzione) bool {
	return f == motorea.FunzIdentitaFile || f == motorea.FunzStruttura || f == motorea.FunzRichiesta
}

// funzioneNelBanco: la funzione che il router dà a una lettura sul selettore, con l'uso dei segmenti del modo
// casi (5.4.5): l'uso sconosciuto, che per oggetto, corpo e storia è «sconosciuto» e per le altre unità «non
// applicabile» (motorea, Instradamento.Uso). Serve dove il runner non ha un'interpretazione da leggere: la
// coerenza fra esempi e casi (regole.go). La prova del pacchetto la confronta con la funzione che Interpreta
// scrive nelle letture dei casi: una sola fonte, il router.
func funzioneNelBanco(sel evidenze.Selettore) motorea.Funzione {
	uso := motorea.UsoNonApplicabile
	switch sel.Contesto {
	case evidenze.ContestoOggetto, evidenze.ContestoCorpo, evidenze.ContestoStoria:
		uso = motorea.UsoSconosciuto
	}
	f, _ := motorea.Instrada(motorea.Instradamento{Selettore: sel, Uso: uso})
	return f
}

func a1a(f func(c *scena, v ValoreAtteso) valutazione) regolaChiave {
	return regolaChiave{sessione: SessioneA1a, valuta: f}
}

// a1b: una chiave che diventa controllabile in A1b, con il router e gli attributi di Interpreta.
func a1b(f func(c *scena, v ValoreAtteso) valutazione) regolaChiave {
	return regolaChiave{sessione: SessioneA1b, valuta: f}
}

func decaduta() regolaChiave {
	return regolaChiave{sessione: SessioneDecaduta, motivo: "chiave decaduta con la modifica degli attesi (M-01): non si controlla"}
}

const (
	formaLavagna    = "lavagna" // la forma che match_forma_lavagna nomina; se è riservata il caso è riservato
	senzaLetture    = "nessuna lettura"
	senzaMenzioni   = "nessuna menzione"
	senzaRevisioni  = "nessuna revisione letta o attribuita"
	tipoNonPrevisto = "tipo del valore atteso non previsto per questa chiave"
	richiedeLetture = true
	ammetteZero     = false
)

// tabellaChiavi: la tabella del par.4.7.5, con le chiavi che il router e gli attributi rendono controllabili
// in A1b (5.1 riga 10; A1b-24). Ogni chiave degli attesi sta qui: una chiave che non c'è è sconosciuta, e la
// lettura degli attesi la rifiuta. Si scorre solo per cercare: l'ordine delle chiavi di un caso è quello
// alfabetico.
var tabellaChiavi = map[string]regolaChiave{
	"base":                             a1a(esatto(basi, richiedeLetture)),
	"basi":                             a1a(esatto(basiConRipetizioni, richiedeLetture)),
	"numero_codici":                    a1a(numeroCodici),
	"secondo_codice_non_perso":         a1a(secondoCodice),
	"marcatore":                        a1a(esatto(marcatori, richiedeLetture)),
	"etichetta":                        a1a(fra(etichette)),
	"codice_richiesto":                 a1a(fra(codiciRichiesti)),
	"revisione":                        a1a(revisione),
	"revisione_da_questo_campo":        a1a(revisione),
	"revisione_dal_nome":               a1a(revisione),
	"revisione_dal_token":              a1a(revisione),
	"revisione_da_token_base":          a1a(revisione),
	"forte":                            a1a(segmentoRevisione(grammatica.SignificatoForte)),
	"debole":                           a1a(segmentoRevisione(grammatica.SignificatoDebole)),
	"token_revisione":                  a1a(fra(tokenRevisione)),
	"revisione_numerica":               a1a(revisioneNumerica),
	"richiede_verifica":                a1a(booleano(richiedeVerifica, richiedeLetture)),
	"non_equivalente_a":                a1a(nonEquivalente),
	"equivalenza_slash_attiva":         a1a(booleano(equivalenzeDichiarate, richiedeLetture)),
	"base_candidata":                   a1a(fra(basiCandidate)),
	"affisso":                          a1a(esatto(affissi, richiedeLetture)),
	"fase":                             a1a(valoreAffisso(func(v grammatica.ValoreQualificatore) string { return v.Fase })),
	"destinazione":                     a1a(valoreAffisso(func(v grammatica.ValoreQualificatore) string { return v.Destinazione })),
	"semantica_S":                      a1a(semanticaS),
	"involucro":                        a1a(decorazione(grammatica.TipoDecorazioneInvolucro)),
	"stato_pdm":                        a1a(decorazione(grammatica.TipoDecorazioneStatoPDM)),
	"decorazione_nome_file":            a1a(decorazione(grammatica.TipoDecorazioneLivelloNomeFile)),
	"token_conservato":                 a1a(tokenConservato),
	"identita_include_stato_pdm":       a1a(booleano(statoPDMNellaBase, richiedeLetture)),
	"identita_include_token":           a1a(booleano(tokenNellaBase, richiedeLetture)),
	"segmenti_mancanti":                a1a(fra(mancanti)),
	"completa":                         a1a(booleano(tutteComplete, richiedeLetture)),
	"completamento_inventato":          a1a(booleano(completamentoInventato, richiedeLetture)),
	"originale_conservato":             a1a(originaleConservato),
	"plus_conservato":                  a1a(booleano(plusConservato, richiedeLetture)),
	"tripla_finale_parte_base":         a1a(booleano(triplaFinale, richiedeLetture)),
	"ripetizioni_concordanti":          a1a(booleano(ripetizioniConcordanti, richiedeLetture)),
	"stato":                            a1a(stato),
	"target_unico":                     a1a(booleano(targetUnico, richiedeLetture)),
	"categorie":                        a1a(fra(categorie)),
	"forma_distinta":                   a1a(formaDistinta),
	"letture_identita":                 a1a(lettureIdentita),
	"identita_da_famiglia_etichettata": a1a(booleano(qualcheLettura, ammetteZero)),
	"match_forma_lavagna":              a1a(matchForma(formaLavagna)),
	"match_forma_osservata":            a1a(booleano(qualcheLettura, ammetteZero)),

	"fallback_generico_non_promuove":     a1b(booleano(nessunaLetturaSenzaFamiglia, ammetteZero)),
	"identita_file_da_nota":              a1b(booleano(qualcheLettura, ammetteZero)),
	"base_menzionata":                    a1b(fraMenzioni(basiMenzionate)),
	"nessuna_fusione":                    a1b(booleano(nessunaFusione, ammetteZero)),
	"cifre":                              a1b(cifre),
	"nessuna_inferenza_da_altro_profilo": a1b(booleano(soloIlProfilo, ammetteZero)),

	"letture_identita_famiglia":           decaduta(),
	"diagnostica_codice_non_riconosciuto": decaduta(),
	"confronto_legacy":                    decaduta(),
}

// ChiaveNota: la chiave sta nella tabella di traduzione. Una chiave fuori tabella rende gli attesi illeggibili.
func ChiaveNota(chiave string) bool {
	_, ok := tabellaChiavi[chiave]
	return ok
}

// SessioneChiave: la sessione in cui la chiave si controlla (A1a, A1b o decaduta); vuota se la chiave non è
// nota.
func SessioneChiave(chiave string) string { return tabellaChiavi[chiave].sessione }

// ChiaviNote: tutte le chiavi della tabella, in ordine alfabetico.
func ChiaviNote() []string {
	out := make([]string, 0, len(tabellaChiavi))
	for k := range tabellaChiavi {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- gli aiuti per i valori ----

func passata(ottenuto string) valutazione {
	return valutazione{stato: ChiavePassata, ottenuto: ottenuto}
}

func fallita(ottenuto, motivo string) valutazione {
	return valutazione{stato: ChiaveFallita, ottenuto: ottenuto, motivo: motivo}
}

func esito(ok bool, ottenuto string) valutazione {
	if ok {
		return passata(ottenuto)
	}
	return fallita(ottenuto, "")
}

// testiAttesi: i testi di un valore semplice o di una lista; ok falso per null, booleani e mappe.
func testiAttesi(v ValoreAtteso) ([]string, bool) {
	switch v.Tipo {
	case TipoStringa, TipoIntero:
		return []string{v.Testo}, true
	case TipoLista:
		return v.Elementi, true
	}
	return nil, false
}

func booleanoAtteso(v ValoreAtteso) (bool, bool) {
	if v.Tipo != TipoBooleano {
		return false, false
	}
	return v.Testo == "true", true
}

func interoAtteso(v ValoreAtteso) (int, bool) {
	if v.Tipo != TipoIntero {
		return 0, false
	}
	n, err := strconv.Atoi(v.Testo)
	return n, err == nil
}

// unici: l'elenco senza vuoti e senza doppioni, in ordine di byte.
func unici(s []string) []string {
	visti := map[string]bool{}
	var out []string
	for _, x := range s {
		if x != "" && !visti[x] {
			visti[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func elenco(s []string) string {
	if len(s) == 0 {
		return "nessuno"
	}
	return strings.Join(s, ", ")
}

func dentro(x string, s []string) bool {
	for _, y := range s {
		if x == y {
			return true
		}
	}
	return false
}

// ---- le chiavi su un insieme di valori letti ----

type estrattore func(c *scena) []string

// esatto: l'insieme dei valori letti coincide con quello atteso (R25 d).
func esatto(e estrattore, conLetture bool) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		attesi, ok := testiAttesi(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		if conLetture && len(c.letture) == 0 {
			return fallita(senzaLetture, "")
		}
		letti := unici(e(c))
		return esito(strings.Join(letti, "\x00") == strings.Join(unici(attesi), "\x00"), elenco(letti))
	}
}

// fra: ogni valore atteso sta fra quelli letti (R25 c). Una lista vuota scritta negli attesi vuol dire
// «nessuno» (per esempio nessun segmento mancante, nessuna categoria): passa solo se non si legge niente, mai
// per vuoto.
func fra(e estrattore) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		attesi, ok := testiAttesi(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		if len(c.letture) == 0 {
			return fallita(senzaLetture, "")
		}
		return contiene(unici(e(c)), attesi, v)
	}
}

// contiene: i valori attesi stanno fra quelli letti; una lista attesa vuota vuole zero valori letti.
func contiene(letti, attesi []string, v ValoreAtteso) valutazione {
	if v.Tipo == TipoLista && len(attesi) == 0 {
		return esito(len(letti) == 0, elenco(letti))
	}
	for _, a := range attesi {
		if !dentro(a, letti) {
			return fallita(elenco(letti), "")
		}
	}
	return passata(elenco(letti))
}

// booleano: un predicato sulle letture, confrontato con il booleano atteso.
func booleano(p func(c *scena) bool, conLetture bool) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		atteso, ok := booleanoAtteso(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		if conLetture && len(c.letture) == 0 {
			return fallita(senzaLetture, "")
		}
		got := p(c)
		return esito(got == atteso, strconv.FormatBool(got))
	}
}

func basi(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		out = append(out, l.Base.Normalizzata)
	}
	return out
}

func basiConRipetizioni(c *scena) []string {
	out := basi(c)
	for _, l := range c.letture {
		for _, r := range l.Ripetizioni {
			out = append(out, r.Base.Normalizzata)
		}
	}
	return out
}

func marcatori(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		if l.Marcatore != nil {
			out = append(out, l.Marcatore.Valore)
		}
	}
	return out
}

func etichette(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		if l.Etichetta != nil {
			out = append(out, l.Etichetta.Valore)
		}
	}
	return out
}

func codiciRichiesti(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		out = append(out, l.CodiceRichiesto)
	}
	return out
}

func tokenRevisione(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		if l.Revisione != nil && l.Revisione.Token != nil {
			out = append(out, l.Revisione.Token.Valore)
		}
	}
	return out
}

func basiCandidate(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		if l.Stato == motorea.StatoDaVerificare {
			out = append(out, l.Base.Normalizzata)
		}
	}
	return out
}

func affissi(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		for _, a := range l.Affissi {
			out = append(out, a.Originale)
		}
	}
	return out
}

func mancanti(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		out = append(out, l.Base.Mancanti...)
	}
	return out
}

// fraMenzioni: come fra, sulle menzioni (R25 a: fuori dall'insieme d'identità, le leggono le loro chiavi).
// Senza menzioni la chiave fallisce: mai un passato per vuoto.
func fraMenzioni(e estrattore) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		attesi, ok := testiAttesi(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		if len(c.menzioni) == 0 {
			return fallita(senzaMenzioni, "")
		}
		return contiene(unici(e(c)), attesi, v)
	}
}

// basiMenzionate: le basi delle menzioni.
func basiMenzionate(c *scena) []string {
	var out []string
	for _, l := range c.menzioni {
		out = append(out, l.Base.Normalizzata)
	}
	return out
}

// stato: gli stati delle letture d'identità (LetturaForma.Stato) e degli attributi della revisione
// (AttributoLetto.Stato: attribuito, ambiguo, non_attribuito, non_interpretabile). Il valore atteso sta fra
// quelli letti (R25 c); senza letture e senza attributi la chiave fallisce.
func stato(c *scena, v ValoreAtteso) valutazione {
	attesi, ok := testiAttesi(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	if len(c.letture) == 0 && len(c.attributi) == 0 {
		return fallita(senzaLetture, "")
	}
	var letti []string
	for _, l := range c.letture {
		letti = append(letti, l.Stato)
	}
	for _, a := range c.attributi {
		letti = append(letti, a.Stato)
	}
	return contiene(unici(letti), attesi, v)
}

func categorie(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		for _, k := range l.Categorie {
			out = append(out, string(k))
		}
	}
	return out
}

// ---- le chiavi con una regola propria ----

// numeroCodici: il numero delle letture.
func numeroCodici(c *scena, v ValoreAtteso) valutazione {
	n, ok := interoAtteso(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	return esito(len(c.letture) == n, strconv.Itoa(len(c.letture)))
}

// secondoCodice: la seconda base dell'elenco «basi» dello stesso caso è fra quelle lette.
func secondoCodice(c *scena, v ValoreAtteso) valutazione {
	atteso, ok := booleanoAtteso(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	b, ok := c.atteso["basi"]
	if !ok || b.Tipo != TipoLista || len(b.Elementi) < 2 {
		return fallita("", "la chiave si legge con «basi», che nel caso manca o ha meno di due basi")
	}
	got := dentro(b.Elementi[1], unici(basiConRipetizioni(c)))
	return esito(got == atteso, strconv.FormatBool(got))
}

// revisioniLette: le revisioni in linea delle letture d'identità con lo stato «letta», più quelle in campo
// separato attribuite all'entità da una regola (5.4.6 punto 12), normalizzate. Una revisione
// non_interpretabile, ambigua o non attribuita non ha valore: non entra.
func revisioniLette(c *scena) []string {
	var out []string
	for _, l := range c.letture {
		if l.Revisione != nil && l.Revisione.Stato == motorea.StatoRevisioneLetta {
			out = append(out, l.Revisione.Normalizzata)
		}
	}
	for _, a := range c.attributi {
		if a.Stato == motorea.StatoAttribuito {
			out = append(out, a.Normalizzato)
		}
	}
	return out
}

// revisione: null ⇔ nessuna revisione letta né attribuita; una stringa: l'insieme delle revisioni lette e
// attribuite coincide con lei (R25 d). Vale anche per revisione_da_questo_campo, revisione_dal_nome,
// revisione_dal_token e revisione_da_token_base. Sul campo della revisione del cartiglio il valore viene
// dall'attributo in campo separato (A-C02, A-C09): con l'attributo non_interpretabile la revisione è null, e
// l'originale resta (originale_conservato, stato). Senza letture d'identità e senza attributi la chiave
// fallisce.
func revisione(c *scena, v ValoreAtteso) valutazione {
	if len(c.letture) == 0 && len(c.attributi) == 0 {
		return fallita(senzaLetture, "")
	}
	lette := unici(revisioniLette(c))
	if v.Tipo == TipoNullo {
		return esito(len(lette) == 0, elencoONull(lette))
	}
	attesi, ok := testiAttesi(v)
	if !ok || v.Tipo == TipoLista {
		return fallita("", tipoNonPrevisto)
	}
	return esito(len(lette) == 1 && lette[0] == attesi[0], elencoONull(lette))
}

func elencoONull(s []string) string {
	if len(s) == 0 {
		return "null"
	}
	return strings.Join(s, ", ")
}

// revisioneNumerica: null ⇔ nessun segmento della revisione letto; altrimenti come revisione.
func revisioneNumerica(c *scena, v ValoreAtteso) valutazione {
	if len(c.letture) == 0 {
		return fallita(senzaLetture, "")
	}
	if v.Tipo == TipoNullo {
		n := 0
		for _, l := range c.letture {
			if l.Revisione != nil {
				n += len(l.Revisione.Segmenti)
			}
		}
		if n == 0 {
			return passata("null")
		}
		return fallita(fmt.Sprintf("%d segmenti letti", n), "")
	}
	return revisione(c, v)
}

// regolaRevisione: la regola di revisione di una lettura, nella grammatica del motore.
func regolaRevisione(m *motorea.Motore, l motorea.LetturaForma) *grammatica.RegolaRevisione {
	f := famiglia(m, l.Famiglia)
	if f == nil || l.Revisione == nil {
		return nil
	}
	for i := range f.Revisioni {
		if f.Revisioni[i].ID == l.Revisione.Regola {
			return &f.Revisioni[i]
		}
	}
	return nil
}

func famiglia(m *motorea.Motore, id string) *grammatica.FamigliaCodice {
	if m == nil {
		return nil
	}
	g := m.Snapshot().Grammatica
	for i := range g.Famiglie {
		if g.Famiglie[i].ID == id {
			return &g.Famiglie[i]
		}
	}
	return nil
}

// segmentoRevisione: forte e debole sono i segmenti della revisione con quel significato dichiarato (D4),
// come interi. Il nome del segmento è libero: conta il significato scritto nella regola.
func segmentoRevisione(significato string) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		n, ok := interoAtteso(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		if len(c.letture) == 0 {
			return fallita(senzaLetture, "")
		}
		var letti []string
		trovato := false
		for _, l := range c.letture {
			r := regolaRevisione(c.motore, l)
			if r == nil || l.Revisione.Stato != motorea.StatoRevisioneLetta {
				continue
			}
			for _, s := range l.Revisione.Segmenti {
				for _, def := range r.Segmenti {
					if def.Nome != s.Nome || def.Significato != significato {
						continue
					}
					letti = append(letti, s.Normalizzato)
					if x, err := strconv.Atoi(s.Normalizzato); err == nil && x == n {
						trovato = true
					}
				}
			}
		}
		return esito(trovato, elenco(unici(letti)))
	}
}

func richiedeVerifica(c *scena) bool {
	for _, l := range c.letture {
		if l.Revisione != nil && l.Revisione.RichiedeVerifica {
			return true
		}
	}
	return false
}

// nonEquivalente: per ogni lettura, ConfrontaRevisioni con il valore atteso non dà né uguale né equivalente.
// Una lettura senza revisione, o con un token sospeso, non è confrontabile: quindi non è equivalente.
func nonEquivalente(c *scena, v ValoreAtteso) valutazione {
	attesi, ok := testiAttesi(v)
	if !ok || v.Tipo == TipoLista {
		return fallita("", tipoNonPrevisto)
	}
	if len(c.letture) == 0 {
		return fallita(senzaLetture, "")
	}
	altra := motorea.RevisioneLetta{Stato: motorea.StatoRevisioneLetta, Normalizzata: attesi[0]}
	var esiti []string
	ok = true
	for _, l := range c.letture {
		cmp := motorea.CompatibilitaNonDeterminabile
		if l.Revisione != nil {
			cmp = motorea.ConfrontaRevisioni(*l.Revisione, altra)
		}
		esiti = append(esiti, string(cmp))
		if cmp == motorea.CompatibilitaUguale || cmp == motorea.CompatibilitaEquivalente {
			ok = false
		}
	}
	return esito(ok, elenco(unici(esiti)))
}

// equivalenzeDichiarate: la regola della revisione letta dichiara equivalenze (parte 1 §5.2: solo dichiarate).
func equivalenzeDichiarate(c *scena) bool {
	for _, l := range c.letture {
		if r := regolaRevisione(c.motore, l); r != nil && len(r.Equivalenze) > 0 {
			return true
		}
	}
	return false
}

// valoreAffisso: il valore degli affissi attribuiti sul selettore della lettura, su un asse (fase o
// destinazione); null = nessuno.
func valoreAffisso(asse func(grammatica.ValoreQualificatore) string) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		if len(c.letture) == 0 {
			return fallita(senzaLetture, "")
		}
		var letti []string
		for _, l := range c.letture {
			for _, a := range l.Affissi {
				if a.Attribuito && a.Valore != nil {
					letti = append(letti, asse(*a.Valore))
				}
			}
		}
		letti = unici(letti)
		if v.Tipo == TipoNullo {
			return esito(len(letti) == 0, elencoONull(letti))
		}
		attesi, ok := testiAttesi(v)
		if !ok || v.Tipo == TipoLista {
			return fallita("", tipoNonPrevisto)
		}
		return esito(dentro(attesi[0], letti), elencoONull(letti))
	}
}

// semanticaS: «non_attribuita» ⇔ c'è un affisso riconosciuto e nessuno è attribuito (D5).
func semanticaS(c *scena, v ValoreAtteso) valutazione {
	if v.Tipo != TipoStringa {
		return fallita("", tipoNonPrevisto)
	}
	if len(c.letture) == 0 {
		return fallita(senzaLetture, "")
	}
	riconosciuti, attribuiti := 0, 0
	for _, l := range c.letture {
		for _, a := range l.Affissi {
			riconosciuti++
			if a.Attribuito {
				attribuiti++
			}
		}
	}
	got := "nessun_affisso"
	switch {
	case attribuiti > 0:
		got = "attribuita"
	case riconosciuti > 0:
		got = "non_attribuita"
	}
	return esito(got == v.Testo, got)
}

// decorazione: le decorazioni lette di un tipo. Una stringa o una lista: i valori stanno fra quelli letti;
// un booleano: c'è (o non c'è) una decorazione di quel tipo.
func decorazione(tipo string) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		if len(c.letture) == 0 {
			return fallita(senzaLetture, "")
		}
		var letti []string
		for _, l := range c.letture {
			for _, d := range l.Decorazioni {
				if d.Tipo == tipo {
					letti = append(letti, d.Valore)
				}
			}
		}
		letti = unici(letti)
		if b, ok := booleanoAtteso(v); ok {
			return esito((len(letti) > 0) == b, elenco(letti))
		}
		attesi, ok := testiAttesi(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		return contiene(letti, attesi, v)
	}
}

// tokenConservato: la parte token della lettura (R16), conservata e non attribuita. Una stringa: il suo
// valore; un booleano: c'è o non c'è.
func tokenConservato(c *scena, v ValoreAtteso) valutazione {
	if len(c.letture) == 0 {
		return fallita(senzaLetture, "")
	}
	var letti []string
	for _, l := range c.letture {
		if l.Token != nil {
			letti = append(letti, l.Token.Valore)
		}
	}
	letti = unici(letti)
	if b, ok := booleanoAtteso(v); ok {
		return esito((len(letti) > 0) == b, elenco(letti))
	}
	attesi, ok := testiAttesi(v)
	if !ok || v.Tipo == TipoLista {
		return fallita("", tipoNonPrevisto)
	}
	return esito(dentro(attesi[0], letti), elenco(letti))
}

// intervalloBase: dal primo all'ultimo segmento letto della base.
func intervalloBase(b motorea.BaseLetta) (evidenze.Intervallo, bool) {
	if len(b.Segmenti) == 0 {
		return evidenze.Intervallo{}, false
	}
	iv := b.Segmenti[0].Intervallo
	for _, s := range b.Segmenti[1:] {
		if s.Intervallo.Inizio < iv.Inizio {
			iv.Inizio = s.Intervallo.Inizio
		}
		if s.Intervallo.Fine > iv.Fine {
			iv.Fine = s.Intervallo.Fine
		}
	}
	return iv, true
}

func nellaBase(b motorea.BaseLetta, iv evidenze.Intervallo) bool {
	ib, ok := intervalloBase(b)
	return ok && iv.Inizio < ib.Fine && ib.Inizio < iv.Fine
}

// statoPDMNellaBase, tokenNellaBase: vero se l'intervallo della parte tocca quello della base, cioè se
// l'identità la comprende. L'atteso è di solito falso: lo stato PDM e il token stanno fuori dall'identità.
func statoPDMNellaBase(c *scena) bool {
	for _, l := range c.letture {
		for _, d := range l.Decorazioni {
			if d.Tipo == grammatica.TipoDecorazioneStatoPDM && nellaBase(l.Base, d.Intervallo) {
				return true
			}
		}
	}
	return false
}

func tokenNellaBase(c *scena) bool {
	for _, l := range c.letture {
		if l.Token != nil && nellaBase(l.Base, l.Token.Intervallo) {
			return true
		}
	}
	return false
}

func tutteComplete(c *scena) bool {
	for _, l := range c.letture {
		if !l.Base.Completa {
			return false
		}
	}
	return true
}

// completamentoInventato: un segmento letto fra quelli che la forma dichiara mancanti (A-C07: mai).
func completamentoInventato(c *scena) bool {
	for _, l := range c.letture {
		for _, s := range l.Base.Segmenti {
			if dentro(s.Nome, l.Base.Mancanti) {
				return true
			}
		}
	}
	return false
}

// originaleConservato: ogni lettura d'identità ha l'originale non vuoto, uguale al testo del suo intervallo, e
// ogni attributo della revisione ha il grezzo non vuoto, uguale al testo del caso (DaTesto fa una sola unità
// con tutto il testo: il campo intero è l'originale, anche quando non è interpretabile, C-34). Senza letture e
// senza attributi la chiave fallisce.
func originaleConservato(c *scena, v ValoreAtteso) valutazione {
	atteso, ok := booleanoAtteso(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	if len(c.letture) == 0 && len(c.attributi) == 0 {
		return fallita(senzaLetture, "")
	}
	got := true
	for _, l := range c.letture {
		iv := l.Intervallo
		if l.Originale == "" || iv.Inizio < 0 || iv.Fine > len(c.testo) || iv.Inizio > iv.Fine ||
			c.testo[iv.Inizio:iv.Fine] != l.Originale {
			got = false
		}
	}
	for _, a := range c.attributi {
		if a.Grezzo == "" || a.Grezzo != c.testo {
			got = false
		}
	}
	return esito(got == atteso, strconv.FormatBool(got))
}

func plusConservato(c *scena) bool {
	for _, l := range c.letture {
		if strings.Contains(l.Base.Originale, "+") && strings.Contains(l.Base.Normalizzata, "+") {
			return true
		}
	}
	return false
}

// triplaFinale: l'ultimo segmento della base dichiarata è letto, la base è completa e nessuna revisione è
// letta al suo posto.
func triplaFinale(c *scena) bool {
	for _, l := range c.letture {
		f := famiglia(c.motore, l.Famiglia)
		if f == nil || len(f.Base.Segmenti) == 0 || len(l.Base.Segmenti) == 0 {
			continue
		}
		ultimo := f.Base.Segmenti[len(f.Base.Segmenti)-1].Nome
		if l.Base.Completa && len(l.Base.Mancanti) == 0 && l.Revisione == nil &&
			l.Base.Segmenti[len(l.Base.Segmenti)-1].Nome == ultimo {
			return true
		}
	}
	return false
}

func ripetizioniConcordanti(c *scena) bool {
	for _, l := range c.letture {
		if len(l.Ripetizioni) == 0 {
			continue
		}
		tutte := true
		for _, r := range l.Ripetizioni {
			tutte = tutte && r.Concorda
		}
		if tutte {
			return true
		}
	}
	return false
}

// targetUnico: falso ⇔ una lettura è discordante (le ripetizioni non concordano: nessuna scelta della prima,
// D2).
func targetUnico(c *scena) bool {
	for _, l := range c.letture {
		if l.Stato == motorea.StatoDiscordante {
			return false
		}
	}
	return true
}

// qualcheLettura: c'è almeno una lettura d'identità. Per identita_da_famiglia_etichettata e
// match_forma_osservata (falso ⇔ nessuna lettura delle famiglie del profilo) e per identita_file_da_nota
// (falso ⇔ la nota non dà nessuna identità: le sue letture, se ci sono, sono menzioni, R25 a).
func qualcheLettura(c *scena) bool { return len(c.letture) > 0 }

// formeDi: le forme delle letture, come «famiglia/forma».
func formeDi(ls []motorea.LetturaForma) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Famiglia+"/"+l.Forma)
	}
	return unici(out)
}

// formaDistinta: le letture del caso vengono da forme che non leggono un altro nome dello stesso profilo, sullo
// stesso selettore: c'è almeno un altro caso letto, e solo da forme diverse.
func formaDistinta(c *scena, v ValoreAtteso) valutazione {
	atteso, ok := booleanoAtteso(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	if len(c.letture) == 0 {
		return fallita(senzaLetture, "")
	}
	mie := formeDi(c.letture)
	got := false
	for _, altre := range c.formeAltri {
		if len(altre) == 0 {
			continue
		}
		diverse := true
		for _, f := range altre {
			diverse = diverse && !dentro(f, mie)
		}
		if diverse {
			got = true
			break
		}
	}
	return esito(got == atteso, strconv.FormatBool(got)+" ("+elenco(mie)+")")
}

// lettureIdentita: il numero delle letture d'identità di R25 (a), cioè delle letture di famiglia con funzione
// identita_file, struttura o richiesta, contate per occorrenza (dopo la deduplica della stessa occorrenza di
// Interpreta), su qualunque selettore. Menzioni, relazioni e attributi non contano: l'ottenuto le nomina, con
// la funzione, perché un numero diverso si capisca senza rileggere il caso.
func lettureIdentita(c *scena, v ValoreAtteso) valutazione {
	n, ok := interoAtteso(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	ottenuto := strconv.Itoa(len(c.letture))
	var fuori []string
	for _, l := range c.codici {
		if !funzioneDIdentita(l.Funzione) {
			fuori = append(fuori, string(l.Funzione))
		}
	}
	if len(fuori) > 0 {
		sort.Strings(fuori)
		ottenuto += " (fuori dall'insieme: " + strings.Join(fuori, ", ") + ")"
	}
	return esito(len(c.letture) == n, ottenuto)
}

// nessunaLetturaSenzaFamiglia: fallback_generico_non_promuove (R25 e = A): nessuna lettura d'identità (né di
// richiesta) senza una famiglia della grammatica del profilo. Con R25 (e) = A Interpreta non ha un
// riconoscitore generico, quindi ogni lettura ha la sua famiglia; la chiave controlla che sia davvero una
// famiglia dello snapshot del profilo. Senza letture è vero: il generico non ha promosso niente.
func nessunaLetturaSenzaFamiglia(c *scena) bool {
	for _, l := range c.letture {
		if l.Famiglia == "" || famiglia(c.motore, l.Famiglia) == nil {
			return false
		}
	}
	return true
}

// nessunaFusione: nessuna lettura unita a un'altra. L'unica fusione che Interpreta fa è la deduplica della
// stessa occorrenza (5.4.6 punto 9), che porta le unità tolte in AltreUnita; le relazioni non fondono mai
// niente (RelazioneSemantica). Una menzione resta una lettura a sé, senza identità.
func nessunaFusione(c *scena) bool {
	for _, l := range c.codici {
		if len(l.AltreUnita) > 0 {
			return false
		}
	}
	return true
}

// cifre: le revisioni lette o attribuite (revisioniLette) sono tutte fatte di quel numero di cifre decimali,
// zeri compresi (D4, D9: «01» ha due cifre). Senza revisioni la chiave fallisce.
func cifre(c *scena, v ValoreAtteso) valutazione {
	n, ok := interoAtteso(v)
	if !ok {
		return fallita("", tipoNonPrevisto)
	}
	lette := unici(revisioniLette(c))
	if len(lette) == 0 {
		return fallita(senzaRevisioni, "")
	}
	got := true
	var letti []string
	for _, r := range lette {
		k := len(r)
		for i := 0; i < len(r); i++ {
			if r[i] < '0' || r[i] > '9' {
				k = -1
				break
			}
		}
		if k < 0 {
			letti = append(letti, r+": non solo cifre")
		} else {
			letti = append(letti, fmt.Sprintf("%s: %d", r, k))
		}
		got = got && k == n
	}
	return esito(got, strings.Join(letti, ", "))
}

// soloIlProfilo: nessuna inferenza da un altro profilo (D9: le regole di un cliente non passano a un altro).
// L'interpretazione è dello snapshot del profilo del caso (ClienteID e HashSnapshot del motore a cui il
// manifest lega il profilo), e ogni revisione attribuita viene da una regola di una famiglia di quello
// snapshot (l'ID dell'attributo la porta, AttributoLetto.ID).
func soloIlProfilo(c *scena) bool {
	if c.motore == nil || c.interp.ClienteID != c.cliente || c.interp.HashSnapshot != c.motore.Snapshot().Hash {
		return false
	}
	for _, a := range c.attributi {
		if a.Stato != motorea.StatoAttribuito {
			continue
		}
		fam, _, ok := regolaDellAttributo(a)
		if !ok || famiglia(c.motore, fam) == nil {
			return false
		}
	}
	return true
}

// regolaDellAttributo: la famiglia e la regola che hanno dato una revisione in campo separato, dall'ID
// dell'attributo («a:<unità>:revisione:<famiglia>/<regola>», motorea, AttributoLetto.ID). Un attributo senza
// regola (ambiguo, non attribuito) non le ha.
func regolaDellAttributo(a motorea.AttributoLetto) (fam, regola string, ok bool) {
	resto, ok := strings.CutPrefix(a.ID, "a:"+a.UnitaID+":"+motorea.AttributoRevisione+":")
	if !ok {
		return "", "", false
	}
	fam, regola, ok = strings.Cut(resto, "/")
	return fam, regola, ok && fam != "" && regola != ""
}

// matchForma: falso ⇔ nessuna lettura della forma con quell'ID. Se la forma nella grammatica del profilo è
// riservata, o non entra nel motore, la chiave è riservata: un predicato su una forma riservata non si decide
// (par.4.7.5).
func matchForma(id string) func(c *scena, v ValoreAtteso) valutazione {
	return func(c *scena, v ValoreAtteso) valutazione {
		atteso, ok := booleanoAtteso(v)
		if !ok {
			return fallita("", tipoNonPrevisto)
		}
		dichiarata, attiva := formaNelMotore(c.motore, id)
		if !dichiarata {
			return fallita("", fmt.Sprintf("la grammatica del profilo non dichiara la forma %q", id))
		}
		if !attiva {
			return valutazione{stato: ChiaveRiservata, motivo: fmt.Sprintf("la forma %q è riservata nella grammatica del profilo", id)}
		}
		got := false
		for _, l := range c.letture {
			got = got || l.Forma == id
		}
		return esito(got == atteso, strconv.FormatBool(got))
	}
}

// formaNelMotore: la forma è dichiarata da qualche famiglia, ed è attiva (stato attiva, proiezione attiva,
// almeno un selettore attivo).
func formaNelMotore(m *motorea.Motore, id string) (dichiarata, attiva bool) {
	if m == nil {
		return false, false
	}
	for _, f := range m.Snapshot().Grammatica.Famiglie {
		for _, fo := range f.Forme {
			if fo.ID != id {
				continue
			}
			dichiarata = true
			if formaAttiva(fo, f.Base) {
				attiva = true
			}
		}
	}
	return dichiarata, attiva
}

// formaAttiva: lo stato dichiarato, la proiezione e i selettori dicono che la forma può entrare nel motore.
func formaAttiva(fo grammatica.FormaCodice, b grammatica.Base) bool {
	if fo.Stato != grammatica.StatoAttiva || !grammatica.ProiezioneAttiva(fo, b) {
		return false
	}
	for _, s := range fo.Selettori {
		if grammatica.SelettoreAttivo(s) {
			return true
		}
	}
	return false
}
