package classificazione

import (
	"strings"
	"unicode"
)

// IL TAGLIO CON LE POSIZIONI (giro 5, A1b.2; piano A, 5.4.3)
//
// TagliaCatena restituisce due stringhe ripulite: vanno bene per dire «di che cosa parla questo messaggio»,
// non per dire DOVE, nel corpo memorizzato, sta ciò che è stato scritto adesso. Il motore A ha bisogno del
// dove: ogni evidenza punta a un intervallo di byte del corpo_testo com'è nel DB (P1 §3.3), e ogni segmento
// della mail deve poter citare la riga e la regola che l'ha separato dal resto (principio della provenance,
// 5.0).
//
// Qui il taglio si rifà accanto a TagliaCatena, che non cambia di un byte:
//   - le regole sono le stesse e si chiamano nello stesso ordine, sulle stesse righe normalizzate (gli aiuti
//     di catena.go: separatoreEsplicito, aperturaDiCitazione, intestazioneCitata, testoMarcato, poi arretra).
//     Si duplica il ciclo di dieci righe, non le regole;
//   - in più si danno le posizioni in byte sul corpo ORIGINALE (normalizza cambia le lunghezze: \r\n diventa
//     \n, gli spazi unificatori diventano uno spazio di un byte) e la regola che ha tagliato;
//   - al posto del ripiego di catena.go:58-61 («il taglio non ha lasciato niente: si tiene tutto») c'è lo stato
//     esplicito storia_non_separabile, con i confini. La storia resta storia: nessuna promozione (R48 A, E-22).
//
// Il rischio è la deriva: una regola nuova in catena.go arriva qui da sola, ma un cambio del ciclo di
// TagliaCatena no. Lo fermano l'equivalenza in tabella e il fuzz (A1b-05, A1b-06; 5.10 n.1).
//
// Taglio usa [2]int e non evidenze.Intervallo: così classificazione non importa niente di nuovo (G9), e la
// conversione la fa l'adattatore di core/estrazione.

// StatoTaglio: che cosa il taglio ha trovato nel corpo.
type StatoTaglio string

const (
	// StatoNessunaStoria: nessuna riga di confine riconosciuta; Corrente è tutto il corpo. Non è «non so
	// separare»: è un limite dichiarato dei riconoscitori (italiano, inglese, tedesco, francese in parte), che
	// l'adattatore scrive nella qualità della fonte. L'inoltro senza nessun confine nel corpo lo riconosce
	// l'adattatore con EInoltro: questa funzione guarda solo il corpo (R48 A, P-08; 5.10 n.15).
	StatoNessunaStoria StatoTaglio = "nessuna_storia"
	// StatoTagliato: una riga riconosciuta, e la parte corrente non è vuota.
	StatoTagliato StatoTaglio = "tagliato"
	// StatoStoriaNonSeparabile: una riga riconosciuta e la parte corrente vuota, dove TagliaCatena ripiega sul
	// corpo intero (catena.go:58-61). I confini ci sono lo stesso.
	StatoStoriaNonSeparabile StatoTaglio = "storia_non_separabile"
)

// RegolaTaglio: il riconoscitore di catena.go che ha visto il confine.
type RegolaTaglio string

const (
	RegolaNessuna             RegolaTaglio = ""                     // nessun confine
	RegolaSeparatoreEsplicito RegolaTaglio = "separatore_esplicito" // separatoreEsplicito
	RegolaAperturaCitazione   RegolaTaglio = "apertura_citazione"   // aperturaDiCitazione
	RegolaIntestazioneCitata  RegolaTaglio = "intestazione_citata"  // intestazioneCitata
	RegolaTestoMarcato        RegolaTaglio = "testo_marcato"        // testoMarcato
)

// RigaTesto: una riga del corpo originale, in byte. Le righe si contano con la semantica di normalizza più
// Split (catena.go:50, :80-90): CRLF è un terminatore, CR o LF da soli sono un terminatore, «\r\r\n» sono due
// terminatori. Le righe coprono il corpo senza buchi: la prima comincia a 0, ognuna comincia dove finisce il
// terminatore della precedente, e l'ultima finisce alla fine del corpo, senza terminatore.
type RigaTesto struct {
	Inizio          int // il primo byte della riga
	Fine            int // la fine del contenuto, escluso il terminatore
	FineTerminatore int // dopo il terminatore; uguale a Fine sull'ultima riga
}

// Taglio: il risultato di TagliaCatenaConPosizioni. Gli intervalli sono [inizio, fine) in byte sul corpo dato.
//
//   - Corrente e Storia dividono il corpo in due, senza buchi: Corrente comincia a 0, Storia finisce alla fine
//     del corpo, e la storia comincia alla riga RigaTaglio (dopo arretra: le righe vuote e di separazione
//     sopra il confine sono già della storia, come in TagliaCatena). Senza storia, Storia è vuoto alla fine
//     del corpo.
//   - CorrenteUtile e StoriaUtile sono gli stessi intervalli senza gli spazi ai bordi (gli spazi di
//     strings.TrimSpace): sul corpo originale ricostruiscono, dopo normalizza, le due stringhe di TagliaCatena.
//     Vuoti, hanno inizio uguale alla fine.
//   - Regola è il riconoscitore che ha visto il confine; Inoltro dice se la riga riconosciuta è una frase
//     d'inoltro, letta come la legge EInoltro (rigaDInoltro, qui sotto).
type Taglio struct {
	Stato         StatoTaglio
	Regola        RegolaTaglio
	Inoltro       bool
	Righe         []RigaTesto
	RigaTaglio    int // la prima riga della storia; -1 = nessun taglio
	Corrente      [2]int
	Storia        [2]int
	CorrenteUtile [2]int
	StoriaUtile   [2]int
}

// TagliaCatenaConPosizioni applica le stesse regole di TagliaCatena (catena.go:49-63), nello stesso ordine e
// sulle stesse righe normalizzate. In più dà le posizioni in byte sul corpo ORIGINALE e la regola che ha
// tagliato, e al posto del ripiego legacy (catena.go:58-61) dà lo stato esplicito storia_non_separabile.
//
// L'equivalenza con TagliaCatena (5.4.3 punto 5): con Stato tagliato, l'utile di TagliaCatena è
// TrimSpace(normalizza(corpo[Corrente])) e la storia è TrimSpace(normalizza(corpo[Storia])); negli altri due
// stati TagliaCatena restituisce (TrimSpace(corpo), "").
func TagliaCatenaConPosizioni(corpo string) Taglio {
	righe, pos := righeDelCorpo(corpo)
	t := Taglio{
		Stato:      StatoNessunaStoria,
		Righe:      pos,
		RigaTaglio: -1,
		Corrente:   [2]int{0, len(corpo)},
		Storia:     [2]int{len(corpo), len(corpo)},
	}
	i, regola := primaRigaConRegola(righe, 0)
	if i < 0 {
		t.CorrenteUtile = senzaSpaziAiBordi(corpo, t.Corrente)
		t.StoriaUtile = t.Storia
		return t
	}
	t.Regola = regola
	t.Inoltro = rigaDInoltro(righe[i])
	i = arretra(righe, i)
	t.RigaTaglio = i
	t.Corrente = [2]int{0, pos[i].Inizio}
	t.Storia = [2]int{pos[i].Inizio, len(corpo)}
	t.CorrenteUtile = senzaSpaziAiBordi(corpo, t.Corrente)
	t.StoriaUtile = senzaSpaziAiBordi(corpo, t.Storia)
	// Lo stesso controllo di catena.go:56-58, sulle stesse righe: dove TagliaCatena ripiega sul corpo intero,
	// qui lo stato lo dice e i confini restano.
	if strings.TrimSpace(strings.Join(righe[:i], "\n")) == "" {
		t.Stato = StatoStoriaNonSeparabile
	} else {
		t.Stato = StatoTagliato
	}
	return t
}

// LivelloStoria: un livello della storia citata. Il primo comincia alla riga di taglio; gli altri dove le
// stesse regole riconoscono un confine più in basso.
type LivelloStoria struct {
	Righe   [2]int       // righe [da, a) del corpo: dalla riga di confine fino al livello dopo
	Byte    [2]int       // gli stessi confini in byte sul corpo originale
	Regola  RegolaTaglio // la regola che ha aperto il livello
	Inoltro bool         // la riga che lo apre è una frase d'inoltro (conversazione.go:82-88)
}

// LivelliDellaStoria riapplica dentro la storia le quattro regole di TagliaCatena, per separare l'inoltro dalle
// citazioni più vecchie (R27 b, nella forma minimale decisa il 04/10: nessuna regola nuova, solo gli aiuti di
// oggi). La ricerca di un livello riparte dopo il blocco di confine del livello prima: la riga riconosciuta,
// le intestazioni che la seguono (chiaveIntestazione), le righe vuote o di separazione; per il testo marcato,
// tutte le righe che cominciano con «>» (5.10 n.2). Senza quel blocco la seconda ricerca si fermerebbe subito
// sulle intestazioni sotto il separatore.
//
// I livelli coprono la storia senza buchi: il primo comincia a t.RigaTaglio, ognuno finisce dove comincia il
// successivo (anche questo dopo arretra, ma mai prima della fine del blocco di confine di sopra), l'ultimo
// alla fine del corpo. Restituisce al più massimo livelli, in ordine: oltre, il resto resta nell'ultimo (il
// limite e la sua diagnostica li tiene l'adattatore, 5.4.3 punto 6). Senza storia, o con un Taglio che non è
// di questo corpo, nessuno [+3].
func LivelliDellaStoria(corpo string, t Taglio, massimo int) []LivelloStoria {
	if massimo <= 0 || t.Stato == StatoNessunaStoria || t.RigaTaglio < 0 {
		return nil
	}
	righe, pos := righeDelCorpo(corpo)
	if t.RigaTaglio >= len(righe) || !stesseRighe(pos, t.Righe) {
		return nil
	}
	var livelli []LivelloStoria
	inizio, da := t.RigaTaglio, t.RigaTaglio
	for len(livelli) < massimo {
		i, regola := primaRigaConRegola(righe, da)
		if i < 0 {
			break
		}
		if n := len(livelli); n > 0 {
			inizio = max(arretra(righe, i), da)
			livelli[n-1].Righe[1] = inizio
			livelli[n-1].Byte[1] = pos[inizio].Inizio
		}
		livelli = append(livelli, LivelloStoria{
			Righe:   [2]int{inizio, len(righe)},
			Byte:    [2]int{pos[inizio].Inizio, len(corpo)},
			Regola:  regola,
			Inoltro: rigaDInoltro(righe[i]),
		})
		da = fineDelConfine(righe, i, regola)
	}
	return livelli
}

// righeDelCorpo divide il corpo come strings.Split(normalizza(corpo), "\n"), ma tiene le posizioni delle righe
// sul corpo originale. Le righe restituite sono normalizzate riga per riga: normalizza lavora carattere per
// carattere, e né CR né LF possono stare dentro un carattere di più byte, quindi è lo stesso che
// normalizzare il corpo intero e poi dividerlo.
func righeDelCorpo(corpo string) ([]string, []RigaTesto) {
	var pos []RigaTesto
	inizio := 0
	for i := 0; i < len(corpo); i++ {
		switch corpo[i] {
		case '\r':
			fine := i
			if i+1 < len(corpo) && corpo[i+1] == '\n' {
				i++
			}
			pos = append(pos, RigaTesto{Inizio: inizio, Fine: fine, FineTerminatore: i + 1})
			inizio = i + 1
		case '\n':
			pos = append(pos, RigaTesto{Inizio: inizio, Fine: i, FineTerminatore: i + 1})
			inizio = i + 1
		}
	}
	pos = append(pos, RigaTesto{Inizio: inizio, Fine: len(corpo), FineTerminatore: len(corpo)})
	righe := make([]string, len(pos))
	for k, r := range pos {
		righe[k] = normalizza(corpo[r.Inizio:r.Fine])
	}
	return righe, pos
}

// stesseRighe: le righe di due tagli coincidono, cioè il Taglio è stato calcolato su questo corpo.
func stesseRighe(a, b []RigaTesto) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

// primaRigaConRegola è il ciclo di primaRigaDellaStoria (catena.go:92-104), da una riga data in poi, che dice
// anche quale riconoscitore ha visto il confine. Stessi aiuti, nello stesso ordine: il primo che riconosce la
// riga è la regola.
func primaRigaConRegola(righe []string, da int) (int, RegolaTaglio) {
	for i := da; i < len(righe); i++ {
		t := pulisciCitazione(righe[i])
		if t == "" {
			continue
		}
		switch {
		case separatoreEsplicito(t):
			return i, RegolaSeparatoreEsplicito
		case aperturaDiCitazione(righe, i):
			return i, RegolaAperturaCitazione
		case intestazioneCitata(righe, i):
			return i, RegolaIntestazioneCitata
		case testoMarcato(righe, i):
			return i, RegolaTestoMarcato
		}
	}
	return -1, RegolaNessuna
}

// rigaDInoltro: la riga che apre la storia (o un suo livello) è una frase d'inoltro, ripulita come la ripulisce
// EInoltro (conversazione.go:102-103) e cercata in frasiInoltro. È l'unico punto che decide il tipo «inoltro»
// dei segmenti della mail (R27 c, lettura del 5.4.3 punto 4 e del 5.10 n.3): «-----Messaggio originale-----»
// taglia, ma non è una frase d'inoltro, e dà una citazione. Se l'utente sceglie un'altra lettura, cambia qui.
func rigaDInoltro(riga string) bool {
	t := strings.ToLower(pulisciCitazione(riga))
	return frasiInoltro[strings.TrimSpace(strings.Trim(t, "-_=* \t"))]
}

// fineDelConfine: la prima riga dopo il blocco di confine che comincia alla riga i (5.4.3, 5.10 n.2). Per il
// testo marcato il blocco sono tutte le righe che cominciano con «>» (e quelle vuote in mezzo); per le altre
// regole la riga riconosciuta, poi le intestazioni (chiaveIntestazione), le righe vuote e quelle di
// separazione che la seguono.
func fineDelConfine(righe []string, i int, regola RegolaTaglio) int {
	k := i + 1
	if regola == RegolaTestoMarcato {
		for k < len(righe) {
			s := strings.TrimSpace(righe[k])
			if s != "" && !strings.HasPrefix(s, ">") {
				break
			}
			k++
		}
		return k
	}
	for k < len(righe) {
		s := pulisciCitazione(righe[k])
		if s != "" && !rigaDiSeparazione(s) && chiaveIntestazione(s) == "" {
			break
		}
		k++
	}
	return k
}

// senzaSpaziAiBordi: l'intervallo iv del corpo senza gli spazi ai bordi, con gli stessi spazi di
// strings.TrimSpace (unicode.IsSpace). Un intervallo di soli spazi diventa vuoto alla sua fine.
func senzaSpaziAiBordi(corpo string, iv [2]int) [2]int {
	s := corpo[iv[0]:iv[1]]
	a := iv[0] + len(s) - len(strings.TrimLeftFunc(s, unicode.IsSpace))
	b := iv[0] + len(strings.TrimRightFunc(s, unicode.IsSpace))
	if b < a {
		b = a
	}
	return [2]int{a, b}
}
