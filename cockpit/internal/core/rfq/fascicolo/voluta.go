package fascicolo

// L'editor della struttura (Fascicolo v3): sotto un prodotto finito l'operatore disegna la struttura che
// vuole — sposta le carte, prende o lascia quello che lo STEP propone, mette un pezzo anche sotto un secondo
// padre, scrive a mano il codice di un pezzo quando lo STEP non c'e' — e la conferma con un gesto solo. Qui la
// si applica in UNA transazione (quella di chi chiama), con le primitive di sempre: accettaNodo per i nodi
// proposti, scollega e collega per gli archi, accettaRelazione quando l'arco voluto e' proprio quello che lo
// STEP propone. Il grafo finale si controlla prima di scrivere, e un rifiuto annulla la transazione intera:
// se il ventesimo arco chiudesse un ciclo non resterebbero diciannove archi scritti.
//
// Il perimetro. La struttura voluta dice, per ogni componente che sta nell'albero del prodotto, quali sono i
// suoi figli. Si tolgono solo gli archi che l'editor ha MOSTRATO (Visti) e che l'operatore ha tolto: un arco
// che l'editor non conosceva (scritto da un collega nel frattempo, o di un pezzo ritrovato per codice) non si
// tocca, e se parte da un componente dell'albero la struttura si rifiuta, perche' e' stata disegnata su dati
// vecchi. Un componente che esce dall'albero si porta dietro i suoi figli (si sposta la carta con il suo
// sottoalbero): i suoi archi non si toccano, e se non ha altri padri torna fra le radici da sistemare (D29).
// Gli archi fuori dall'albero, per esempio quelli di un altro prodotto, restano come sono.
//
// Un secondo padre non nasce da un trascinamento: e' un arco voluto in piu', che l'editor mette solo con
// «Condividi anche sotto…». Il database e il Go ammettono piu' padri (sottoassieme condiviso, A1.1);
// vietati restano i cicli.
//
// Le proposte. Un arco proposto dallo STEP che l'editor ha mostrato (RelazioniViste) e che la struttura
// confermata non ha, o ha con un'altra quantita', si chiude con la nota che dice perche': e' una decisione
// presa. Una proposta che l'editor non ha mostrato resta aperta, a meno che dica esattamente un arco voluto
// (allora e' un duplicato). Un nodo che l'operatore scarta si porta dietro, esplicitamente, gli archi che lo
// toccano.
//
// Smistamento F2 (R1, R5). Dall'editor non nasce piu' un componente da un'evidenza: le carte «k:» (i codici
// trovati nella RFQ) sono tolte, e un riferimento «k:» si rifiuta. Un pezzo che lo STEP non propone lo si
// scrive: la carta «n:<codice>» (StrutturaVoluta.Nuovi, U4) ha il codice scritto dall'operatore, il tipo
// assieme o particolare, origine `manuale`, e la guardia «quasi uguale» (P4) che vuole la conferma che e' un
// pezzo diverso. Un codice che la RFQ ha gia' ritrova quel componente: mai una seconda riga. Tolto anche
// «E' il prodotto» (RadiciProposte, che si rifiuta): la radice di uno STEP qualunque diventava il prodotto
// senza che il file fosse associato ne' scelto come STEP strutturale. La radice la fissa lo STEP
// strutturale.
//
// Le rimozioni proposte dallo STEP strutturale su archi che l'operatore ha appena confermato si chiudono:
// «tenuto nella struttura confermata». Una rimozione scartata non si ripropone.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// MaxArchiVoluti e' quanti archi puo' avere una struttura confermata con un gesto: oltre, e' un errore
// del programma che la manda, non una distinta.
const MaxArchiVoluti = 5000

// ArcoVoluto e' un arco della struttura voluta. Padre e figlio sono riferimenti: "c:<componente_id>" per un
// componente della working, "p:<proposta_id>" per un nodo proposto da uno STEP (componente_proposta),
// "n:<codice>" per un componente che l'operatore scrive (StrutturaVoluta.Nuovi). "k:<codice>", il codice
// trovato nella RFQ, non c'e' piu' (Smistamento F2) e si rifiuta.
type ArcoVoluto struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
	Qta    int32  `json:"qta"`
}

// CodiceScritto e' il codice che l'operatore scrive nell'editor per un nodo proposto che non ne ha uno.
type CodiceScritto struct {
	Codice string `json:"codice"`
	Rev    string `json:"rev"`
}

// RelazioneVista e' un arco proposto dallo STEP che l'editor ha mostrato.
type RelazioneVista struct {
	Allegato uuid.UUID `json:"allegato"`
	Padre    string    `json:"padre"`
	Figlio   string    `json:"figlio"`
}

// StrutturaVoluta e' quello che l'editor manda con «Conferma struttura».
type StrutturaVoluta struct {
	Radice uuid.UUID    `json:"radice"`
	Archi  []ArcoVoluto `json:"archi"`
	// Visti sono gli archi della working (c: → c:) che l'editor conosceva quando si e' aperto.
	Visti []ArcoVoluto `json:"visti"`
	// RelazioniViste sono gli archi proposti che l'editor ha mostrato.
	RelazioniViste []RelazioneVista `json:"relazioni_viste"`
	// RadiciProposte erano i nodi proposti che l'operatore diceva essere il prodotto («E' il prodotto»).
	// Il gesto non c'e' piu' (Smistamento F2): il campo resta solo perche' una struttura che lo usa si
	// rifiuti invece di perdere la scelta in silenzio.
	RadiciProposte []uuid.UUID `json:"radici_proposte"`
	// Scarta: nodi proposti che non sono pezzi della distinta, con gli archi che li toccano.
	Scarta []uuid.UUID `json:"scarta"`
	// Codici: proposta_id → il codice scritto dall'operatore.
	Codici map[string]CodiceScritto `json:"codici"`
	// Nuovi: i componenti scritti con «+ Componente con codice» (U4), le carte "n:<codice>".
	Nuovi []ComponenteNuovo `json:"nuovi"`
}

// ComponenteNuovo e' un pezzo che l'operatore scrive nell'editor: il codice e' suo, mai precompilato.
type ComponenteNuovo struct {
	Codice string            `json:"codice"`
	Tipo   db.TipoComponente `json:"tipo"` // sottoassieme o sciolto (TipiNuovo)
	Rev    string            `json:"rev"`
	// Diverso: l'operatore ha detto che e' un pezzo diverso dai codici quasi uguali della RFQ (P4). Senza,
	// un codice quasi uguale si rifiuta.
	Diverso bool `json:"diverso"`
}

// ------------------------------------------------------------------ il piano (puro)

// chiaveNodo e' un nodo della struttura voluta: "c:<componente_id>" per un componente che c'e', "n:<CODICE>"
// per un codice nuovo, che nasce accettando le proposte aperte con quel codice o dal codice scritto
// dall'operatore (una carta «n:»).
type chiaveNodo string

func chiaveComponente(id uuid.UUID) chiaveNodo { return chiaveNodo("c:" + id.String()) }

func (k chiaveNodo) componente() (uuid.UUID, bool) {
	s, ok := strings.CutPrefix(string(k), "c:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	return id, err == nil
}

// contestoVoluta e' la working, le proposte e i codici della richiesta, letti sotto il lucchetto della RFQ.
type contestoVoluta struct {
	Componenti map[uuid.UUID]db.Componente
	PerCodice  map[string]db.Componente // upper(codice) → componente, archiviati compresi (e il codice senza suffisso)
	Proposte   map[uuid.UUID]db.ComponenteProposta
	// Relazioni: le proposte di arco, per dire se una che l'editor ha mostrato e' stata decisa nel frattempo
	Relazioni map[ChiaveRelazione]db.RelazioneProposta
	Attivi    []db.ComponenteRelazione
	// Richiesta: i codici della richiesta (identificativi). Un componente scritto nell'editor non ne prende
	// uno: diventano prodotti con la creazione della RFQ (triage) o aprendo una revisione della BOM congelata.
	Richiesta []string
	// Motore: le regole del cliente, per la guardia «quasi uguale» (i suffissi decorativi). Puo' essere nil.
	Motore *classificazione.Motore
}

func nuovoContestoVoluta(comp []db.Componente, rel []db.ComponenteRelazione, nodi []db.ComponenteProposta, richiesta []string) contestoVoluta {
	cx := contestoVoluta{Componenti: map[uuid.UUID]db.Componente{}, PerCodice: map[string]db.Componente{},
		Proposte: map[uuid.UUID]db.ComponenteProposta{}, Attivi: rel, Richiesta: richiesta}
	for _, c := range comp {
		cx.Componenti[c.ComponenteID] = c
		cx.PerCodice[strings.ToUpper(strings.TrimSpace(c.Codice))] = c
	}
	for _, p := range nodi {
		cx.Proposte[p.PropostaID] = p
	}
	return cx
}

// componentiOrdinati sono tutti i componenti del contesto, archiviati compresi, in un ordine fisso.
func (cx contestoVoluta) componentiOrdinati() []db.Componente {
	out := make([]db.Componente, 0, len(cx.Componenti))
	for _, c := range cx.Componenti {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Codice < out[j].Codice })
	return out
}

// conAlias aggiunge, per un cliente con suffissi decorativi, ogni componente anche sotto il suo codice senza
// suffisso (come Working.conCanonici): un nodo «X» dello STEP ritrova il pezzo nato come «X_PRT».
func (cx contestoVoluta) conAlias(m *classificazione.Motore) {
	if !m.HaSuffissi() {
		return
	}
	tutti := make([]db.Componente, 0, len(cx.Componenti))
	for _, c := range cx.Componenti {
		tutti = append(tutti, c)
	}
	sort.Slice(tutti, func(i, j int) bool { return tutti[i].Codice < tutti[j].Codice })
	for _, c := range tutti {
		can, _ := m.Canonico(c.Codice, "")
		if k := strings.ToUpper(strings.TrimSpace(can)); k != "" {
			if _, gia := cx.PerCodice[k]; !gia {
				cx.PerCodice[k] = c
			}
		}
	}
}

// stessoCodice sono le proposte aperte con lo stesso codice di questa (lei compresa, per prima): nell'editor
// sono una carta sola, e un gesto sulla carta vale per tutte.
func (cx contestoVoluta) stessoCodice(id uuid.UUID) []uuid.UUID {
	out := []uuid.UUID{id}
	codice := strings.ToUpper(strings.TrimSpace(cx.Proposte[id].Codice.String))
	if codice == "" {
		return out
	}
	var altre []uuid.UUID
	for _, p := range cx.Proposte {
		if p.PropostaID != id && p.Stato == db.StatoPropostaAperta && strings.ToUpper(strings.TrimSpace(p.Codice.String)) == codice {
			altre = append(altre, p.PropostaID)
		}
	}
	sort.Slice(altre, func(i, j int) bool { return altre[i].String() < altre[j].String() })
	return append(out, altre...)
}

type arcoPiano struct {
	Padre, Figlio chiaveNodo
	Qta           int32
}

// pianoVoluta e' la struttura voluta risolta: ogni riferimento e' un nodo, ogni codice nuovo sa da che cosa
// nasce, e l'albero (i nodi raggiungibili dalla radice) e' il perimetro.
type pianoVoluta struct {
	Radice    uuid.UUID
	Archi     []arcoPiano
	Albero    map[chiaveNodo]bool
	Figli     map[chiaveNodo]int
	Nuovi     map[chiaveNodo][]uuid.UUID // codice nuovo → le proposte aperte che lo portano (vuoto = carta «n:» scritta)
	Scritti   map[chiaveNodo]ComponenteNuovo
	Ritrovati map[uuid.UUID]uuid.UUID // proposta aperta → il componente che ha gia' il suo codice
	Codici    map[uuid.UUID]CodiceScritto
	Scarta    []uuid.UUID
	// Esistenti: le carte «n:» il cui codice la RFQ ha gia'. Sono quel componente (identita' invariata).
	Esistenti map[uuid.UUID]bool
	Visti     map[Arco]int32 // gli archi della working che l'editor mostrava, con la quantita' di allora (0 = non detta)
	Tenuti    int            // archi di componenti ritrovati per codice, che l'editor non mostrava: restano
	// Disegnati: la radice e i componenti che la struttura nomina come c: (l'editor ne mostrava i figli).
	// Carte: le proposte nominate come p: (l'editor mostrava i loro archi proposti). Un componente entrato per
	// codice non e' fra i disegnati: le proposte e le rimozioni sotto di lui non si decidono da qui.
	Disegnati map[uuid.UUID]bool
	Carte     map[uuid.UUID]bool
	Tenute    map[Arco]bool // gli archi tenuti: restano, non sono confermati
}

// nome dice come si chiama un nodo per chi legge un rifiuto.
func (cx contestoVoluta) nome(k chiaveNodo) string {
	if id, ok := k.componente(); ok {
		if c, ok := cx.Componenti[id]; ok {
			return c.Codice
		}
		return id.String()
	}
	return strings.TrimPrefix(string(k), "n:")
}

// pianifica controlla la struttura voluta contro la working, senza scrivere niente: riferimenti, codici,
// quantita', archi ripetuti, la radice che non sta sotto nessuno, ogni arco appeso all'albero, gli archi
// della working che l'editor non conosceva, il grafo finale senza cicli. Il primo problema e' il rifiuto.
func pianifica(cx contestoVoluta, v StrutturaVoluta) (pianoVoluta, error) {
	pv := pianoVoluta{Radice: v.Radice, Albero: map[chiaveNodo]bool{}, Figli: map[chiaveNodo]int{},
		Nuovi: map[chiaveNodo][]uuid.UUID{}, Scritti: map[chiaveNodo]ComponenteNuovo{}, Ritrovati: map[uuid.UUID]uuid.UUID{},
		Codici: map[uuid.UUID]CodiceScritto{}, Esistenti: map[uuid.UUID]bool{}, Visti: map[Arco]int32{}}
	radice, ok := cx.Componenti[v.Radice]
	if !ok {
		return pv, Rifiuto("la radice della struttura non è un componente di questa RFQ")
	}
	if radice.ArchiviatoIl != nil {
		return pv, Rifiuto(radice.Codice + " è archiviato: prima lo si ripristina")
	}
	if radice.Tipo != db.TipoComponenteFinito {
		return pv, Rifiuto(radice.Codice + " non è un prodotto finito: la struttura si disegna sotto un prodotto")
	}
	if len(v.Archi) > MaxArchiVoluti {
		return pv, Rifiuto(fmt.Sprintf("la struttura ha %d archi: il limite è %d", len(v.Archi), MaxArchiVoluti))
	}
	if len(v.Visti) > 4*MaxArchiVoluti {
		return pv, Rifiuto(fmt.Sprintf("l'editor ha mandato %d archi della BOM mostrata: il limite è %d", len(v.Visti), 4*MaxArchiVoluti))
	}
	for _, a := range v.Visti {
		p, errP := uuid.Parse(strings.TrimPrefix(a.Padre, "c:"))
		f, errF := uuid.Parse(strings.TrimPrefix(a.Figlio, "c:"))
		if errP != nil || errF != nil {
			return pv, Rifiuto("un arco della BOM mostrata non è valido: riapri l'editor")
		}
		pv.Visti[Arco{Padre: p, Figlio: f}] = a.Qta
	}
	// i codici scritti dall'operatore, controllati prima di tutto: cambiano che nodo e' una proposta
	for k, c := range v.Codici {
		id, err := uuid.Parse(k)
		if err != nil {
			return pv, Rifiuto("un codice scritto si riferisce a un nodo non valido")
		}
		p, ok := cx.Proposte[id]
		if !ok {
			return pv, Rifiuto("un nodo proposto non è di questa RFQ")
		}
		codice, rev := strings.TrimSpace(c.Codice), strings.ToUpper(strings.TrimSpace(c.Rev))
		if codice == "" {
			continue
		}
		if p.Stato != db.StatoPropostaAperta {
			return pv, Rifiuto(nomeNodo(p) + ": la proposta è già decisa, il codice si corregge sul componente")
		}
		if !classificazione.CodiceAmmissibile(codice) {
			return pv, Rifiuto(fmt.Sprintf("«%s»: il codice ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxCodice))
		}
		if rev != "" && !classificazione.RevAmmissibile(rev) {
			return pv, Rifiuto(fmt.Sprintf("«%s»: la revisione ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxRev))
		}
		pv.Codici[id] = CodiceScritto{Codice: codice, Rev: rev}
	}
	// «E' il prodotto» non c'e' piu' (Smistamento F2, R5): la radice di uno STEP qualunque diventava il
	// prodotto senza una decisione sul file. Chi lo manda ancora (una bozza di prima, un altro programma)
	// riceve un rifiuto, non una struttura confermata senza quella parte
	if len(v.RadiciProposte) > 0 {
		return pv, Rifiuto("«È il prodotto» non c'è più: la radice di uno STEP la fissa lo STEP strutturale del prodotto. Riapri l'editor")
	}
	// i componenti scritti dall'operatore (U4): codice scritto, tipo assieme o particolare, una carta per
	// codice. Se la RFQ ha gia' quel codice la carta e' quel componente; se ne ha uno quasi uguale (P4)
	// serve la conferma che e' un pezzo diverso. Controllati quando un arco li usa
	scritti := map[string]ComponenteNuovo{}
	for _, n := range v.Nuovi {
		codice := strings.TrimSpace(n.Codice)
		if !classificazione.CodiceAmmissibile(codice) {
			return pv, Rifiuto(fmt.Sprintf("«%s»: il codice ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxCodice))
		}
		rev := strings.ToUpper(strings.TrimSpace(n.Rev))
		if rev != "" && !classificazione.RevAmmissibile(rev) {
			return pv, Rifiuto(fmt.Sprintf("«%s»: la revisione ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxRev))
		}
		if n.Tipo != db.TipoComponenteSottoassieme && n.Tipo != db.TipoComponenteSciolto {
			return pv, Rifiuto(codice + ": un componente scritto nell'editor è un assieme o un particolare")
		}
		up := strings.ToUpper(codice)
		if _, gia := scritti[up]; gia {
			return pv, Rifiuto(codice + " è scritto due volte: un pezzo è una carta sola")
		}
		scritti[up] = ComponenteNuovo{Codice: codice, Tipo: n.Tipo, Rev: rev, Diverso: n.Diverso}
	}
	chiaveRadice := chiaveComponente(v.Radice)
	risolvi := func(ref string) (chiaveNodo, error) {
		tipo, resto, _ := strings.Cut(ref, ":")
		switch tipo {
		case "c":
			id, err := uuid.Parse(resto)
			if err != nil {
				break
			}
			c, ok := cx.Componenti[id]
			if !ok {
				return "", Rifiuto("un componente della struttura non è di questa RFQ")
			}
			if c.ArchiviatoIl != nil {
				return "", Rifiuto(c.Codice + " è archiviato: prima lo si ripristina")
			}
			return chiaveComponente(id), nil
		case "k":
			// un codice trovato non diventa un componente (R1): il codice lo scrive l'operatore
			return "", Rifiuto(strings.TrimSpace(resto) + ": un codice trovato nella RFQ non diventa un componente dall'editor. " +
				"Se è un pezzo della distinta, scrivine il codice con «+ Componente con codice»")
		case "n":
			n, ok := scritti[strings.ToUpper(strings.TrimSpace(resto))]
			if !ok {
				return "", Rifiuto(strings.TrimSpace(resto) + ": il componente nuovo non ha il suo codice scritto: riapri l'editor")
			}
			e := ValutaCodiceNuovo(n.Codice, cx.componentiOrdinati(), cx.Richiesta, cx.Motore)
			switch {
			case e.Esistente != nil:
				// lo stesso pezzo: nessuna riga nuova, la carta e' quel componente (U4)
				if e.Esistente.ArchiviatoIl != nil {
					return "", Rifiuto(e.Esistente.Codice + " c'è già ed è archiviato: si ripristina dalla Struttura BOM, non se ne crea un altro")
				}
				// un prodotto della RFQ non entra nella struttura di un altro da un codice scritto: l'editor lo
				// ferma prima di mettere la carta, e il core non si fida del client
				if e.Esistente.Tipo == db.TipoComponenteFinito {
					return "", Rifiuto(e.Esistente.Codice + " è un prodotto della RFQ: si sceglie in alto, non si mette sotto un altro prodotto")
				}
				pv.Esistenti[e.Esistente.ComponenteID] = true
				return chiaveComponente(e.Esistente.ComponenteID), nil
			case e.Richiesta:
				return "", Rifiuto(n.Codice + " è un codice della richiesta: il prodotto nasce con la creazione della RFQ dal triage, " +
					"o aprendo una revisione della BOM congelata; non è un componente dall'editor")
			case len(e.Vicini) > 0 && !n.Diverso:
				// la carta scritta prima che il vicino ci fosse non ha la conferma, e l'editor non la chiede su una
				// carta gia' posata: la si toglie e la si riscrive, e la domanda sui vicini arriva
				return "", Rifiuto(fmt.Sprintf("%s è quasi uguale a %s: se è lo stesso pezzo si usa quello; se è un pezzo diverso "+
					"togli la carta di %s e riscrivila con «+ Componente con codice», confermando che è un pezzo diverso",
					n.Codice, codiciVicini(e.Vicini), n.Codice))
			}
			k := chiaveNodo("n:" + strings.ToUpper(n.Codice))
			if _, ok := pv.Nuovi[k]; !ok {
				pv.Nuovi[k] = nil
			}
			pv.Scritti[k] = n
			return k, nil
		case "p":
			id, err := uuid.Parse(resto)
			if err != nil {
				break
			}
			p, ok := cx.Proposte[id]
			if !ok {
				return "", Rifiuto("un nodo proposto non è di questa RFQ")
			}
			switch p.Stato {
			case db.StatoPropostaAperta:
				codice := strings.TrimSpace(p.Codice.String)
				if c, ok := pv.Codici[id]; ok {
					codice = c.Codice
				}
				if codice == "" {
					return "", Rifiuto(nomeNodo(p) + " non ha un codice: lo si scrive prima di metterlo nella struttura")
				}
				if c, ok := cx.PerCodice[strings.ToUpper(codice)]; ok {
					pv.Ritrovati[id] = c.ComponenteID
					return chiaveComponente(c.ComponenteID), nil
				}
				k := chiaveNodo("n:" + strings.ToUpper(codice))
				if !contieneID(pv.Nuovi[k], id) {
					pv.Nuovi[k] = append(pv.Nuovi[k], id)
				}
				return k, nil
			case db.StatoPropostaConfermata, db.StatoPropostaDuplicato:
				c, ok := cx.Componenti[p.ComponenteID.UUID]
				if !p.ComponenteID.Valid || !ok {
					return "", Rifiuto(nomeNodo(p) + ": la proposta non porta a un componente")
				}
				if c.ArchiviatoIl != nil {
					return "", Rifiuto(c.Codice + " è archiviato: prima lo si ripristina")
				}
				return chiaveComponente(c.ComponenteID), nil
			}
			return "", Rifiuto(nomeNodo(p) + " è stato scartato: non entra nella struttura")
		}
		return "", Rifiuto("un nodo della struttura non è valido: " + ref)
	}
	visti := map[[2]chiaveNodo]bool{}
	figliDi := map[chiaveNodo][]chiaveNodo{}
	for _, a := range v.Archi {
		p, err := risolvi(a.Padre)
		if err != nil {
			return pv, err
		}
		f, err := risolvi(a.Figlio)
		if err != nil {
			return pv, err
		}
		if p == f {
			// due PRODUCT con lo stesso codice, uno dentro l'altro (STEP veri, A1.1): dopo i nodi sono lo stesso
			// componente, e un pezzo non contiene se stesso. Fra nodi proposti non e' un errore dell'operatore.
			if strings.HasPrefix(a.Padre, "p:") && strings.HasPrefix(a.Figlio, "p:") {
				continue
			}
			return pv, Rifiuto(cx.nome(f) + ": un componente non sta sotto se stesso")
		}
		if a.Qta < 1 || a.Qta > MaxQtaArco {
			return pv, Rifiuto(fmt.Sprintf("%s sotto %s: la quantità va da 1 a %d", cx.nome(f), cx.nome(p), MaxQtaArco))
		}
		if f == chiaveRadice {
			return pv, Rifiuto(radice.Codice + " è la radice della struttura: nell'editor non va sotto un altro componente")
		}
		if visti[[2]chiaveNodo{p, f}] {
			return pv, Rifiuto(fmt.Sprintf("%s è due volte sotto %s: la quantità è una sola", cx.nome(f), cx.nome(p)))
		}
		visti[[2]chiaveNodo{p, f}] = true
		pv.Archi = append(pv.Archi, arcoPiano{Padre: p, Figlio: f, Qta: a.Qta})
		figliDi[p] = append(figliDi[p], f)
		pv.Figli[p]++
	}
	// l'albero: quello che la radice raggiunge; un arco che parte da fuori non e' appeso a niente
	calcolaAlbero := func() {
		pv.Albero = map[chiaveNodo]bool{chiaveRadice: true}
		coda := []chiaveNodo{chiaveRadice}
		for len(coda) > 0 {
			n := coda[0]
			coda = coda[1:]
			for _, f := range figliDi[n] {
				if !pv.Albero[f] {
					pv.Albero[f] = true
					coda = append(coda, f)
				}
			}
		}
	}
	calcolaAlbero()
	for _, a := range pv.Archi {
		if !pv.Albero[a.Padre] {
			return pv, Rifiuto(fmt.Sprintf("%s → %s non è appeso alla struttura di %s", cx.nome(a.Padre), cx.nome(a.Figlio), radice.Codice))
		}
	}
	// un componente che entra nell'albero senza la sua carta (un nodo proposto ritrovato per codice, un codice
	// trovato che c'e' gia'): l'editor non mostrava i suoi figli, e i suoi archi restano com'erano. Solo i
	// componenti che l'editor ha disegnato (c: nella struttura) hanno i figli che la struttura dice
	disegnati := map[chiaveNodo]bool{chiaveRadice: true}
	pv.Disegnati, pv.Carte, pv.Tenute = map[uuid.UUID]bool{v.Radice: true}, map[uuid.UUID]bool{}, map[Arco]bool{}
	for _, a := range v.Archi {
		for _, ref := range []string{a.Padre, a.Figlio} {
			if id, err := uuid.Parse(strings.TrimPrefix(ref, "c:")); err == nil && strings.HasPrefix(ref, "c:") {
				disegnati[chiaveComponente(id)] = true
				pv.Disegnati[id] = true
			}
			if id, err := uuid.Parse(strings.TrimPrefix(ref, "p:")); err == nil && strings.HasPrefix(ref, "p:") {
				pv.Carte[id] = true
			}
		}
	}
	for {
		tenuti := 0
		for _, r := range cx.Attivi {
			p, f := chiaveComponente(r.PadreID), chiaveComponente(r.FiglioID)
			if !pv.Albero[p] || disegnati[p] || visti[[2]chiaveNodo{p, f}] {
				continue
			}
			visti[[2]chiaveNodo{p, f}] = true
			pv.Archi = append(pv.Archi, arcoPiano{Padre: p, Figlio: f, Qta: r.Qta})
			pv.Tenute[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = true
			figliDi[p] = append(figliDi[p], f)
			pv.Figli[p]++
			tenuti++
		}
		if tenuti == 0 {
			break
		}
		pv.Tenuti += tenuti
		calcolaAlbero()
	}
	// gli archi della working che partono dall'albero devono essere voluti o visti: uno che l'editor non
	// conosceva vuol dire che la struttura e' stata disegnata su dati vecchi
	voluto := map[[2]chiaveNodo]bool{}
	for _, a := range pv.Archi {
		voluto[[2]chiaveNodo{a.Padre, a.Figlio}] = true
	}
	attivi := map[Arco]int32{}
	for _, r := range cx.Attivi {
		p, f := chiaveComponente(r.PadreID), chiaveComponente(r.FiglioID)
		attivi[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r.Qta
		if _, visto := pv.Visti[Arco{Padre: r.PadreID, Figlio: r.FiglioID}]; pv.Albero[p] && !voluto[[2]chiaveNodo{p, f}] && !visto {
			return pv, Rifiuto(fmt.Sprintf("%s ha sotto %s, che l'editor non mostrava (un altro gesto nel frattempo): riapri l'editor",
				cx.nome(p), cx.nome(f)))
		}
	}
	// e quelli che l'editor mostrava e che nel frattempo sono cambiati: tolti (e la struttura li vuole) o con
	// un'altra quantita'. Confermare riscriverebbe la decisione di un altro senza dirlo
	vistiOrdinati := make([]Arco, 0, len(pv.Visti))
	for a := range pv.Visti {
		vistiOrdinati = append(vistiOrdinati, a)
	}
	sort.Slice(vistiOrdinati, func(i, j int) bool {
		if vistiOrdinati[i].Padre != vistiOrdinati[j].Padre {
			return vistiOrdinati[i].Padre.String() < vistiOrdinati[j].Padre.String()
		}
		return vistiOrdinati[i].Figlio.String() < vistiOrdinati[j].Figlio.String()
	})
	for _, a := range vistiOrdinati {
		qv := pv.Visti[a]
		p, f := chiaveComponente(a.Padre), chiaveComponente(a.Figlio)
		if !pv.Albero[p] {
			continue
		}
		q, presente := attivi[a]
		switch {
		case !presente && voluto[[2]chiaveNodo{p, f}]:
			return pv, Rifiuto(fmt.Sprintf("%s sotto %s nel frattempo è stato tolto (un altro gesto): riapri l'editor", cx.nome(f), cx.nome(p)))
		case presente && qv > 0 && q != qv:
			return pv, Rifiuto(fmt.Sprintf("%s sotto %s nel frattempo è diventato ×%d (l'editor mostrava ×%d): riapri l'editor", cx.nome(f), cx.nome(p), q, qv))
		}
	}
	for _, r := range v.RelazioniViste {
		if x, ok := cx.Relazioni[ChiaveRelazione{Allegato: r.Allegato, Padre: r.Padre, Figlio: r.Figlio}]; ok && x.Stato != db.StatoPropostaAperta {
			return pv, Rifiuto(fmt.Sprintf("una proposta dello STEP (%s → %s) è stata decisa nel frattempo: riapri l'editor", r.Padre, r.Figlio))
		}
	}
	// il grafo finale: gli archi della working fuori dall'albero, e quelli dell'albero che restano (i voluti).
	// Un ciclo lo si dice prima di scrivere.
	id := func(k chiaveNodo) uuid.UUID {
		if u, ok := k.componente(); ok {
			return u
		}
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(k))
	}
	nomi := map[uuid.UUID]chiaveNodo{}
	var finale []Arco
	for _, r := range cx.Attivi {
		if !pv.Albero[chiaveComponente(r.PadreID)] {
			finale = append(finale, Arco{Padre: r.PadreID, Figlio: r.FiglioID})
		}
	}
	for _, a := range pv.Archi {
		p, f := id(a.Padre), id(a.Figlio)
		nomi[p], nomi[f] = a.Padre, a.Figlio
		finale = append(finale, Arco{Padre: p, Figlio: f})
	}
	if giro := Ciclo(finale); giro != nil {
		parti := make([]string, len(giro))
		for i, g := range giro {
			if k, ok := nomi[g]; ok {
				parti[i] = cx.nome(k)
			} else {
				parti[i] = cx.nome(chiaveComponente(g))
			}
		}
		return pv, Rifiuto("la struttura chiuderebbe un ciclo (" + strings.Join(parti, " → ") + ")")
	}
	var scarta []uuid.UUID
	for _, s := range v.Scarta {
		p, ok := cx.Proposte[s]
		if !ok {
			return pv, Rifiuto("un nodo da scartare non è di questa RFQ")
		}
		if p.Stato != db.StatoPropostaAperta {
			return pv, Rifiuto(nomeNodo(p) + ": la proposta è già decisa")
		}
		for _, x := range cx.stessoCodice(s) {
			if !contieneID(scarta, x) {
				scarta = append(scarta, x)
			}
		}
	}
	for _, s := range scarta {
		p := cx.Proposte[s]
		if contieneID(pv.Scarta, s) {
			continue
		}
		for k, ids := range pv.Nuovi {
			if contieneID(ids, s) && pv.Albero[k] {
				return pv, Rifiuto(nomeNodo(p) + " è nella struttura e fra gli scartati: o l'uno o l'altro")
			}
		}
		if c, ok := pv.Ritrovati[s]; ok && pv.Albero[chiaveComponente(c)] {
			return pv, Rifiuto(nomeNodo(p) + " è nella struttura e fra gli scartati: o l'uno o l'altro")
		}
		pv.Scarta = append(pv.Scarta, s)
	}
	// un arco proposto che l'editor ha mostrato, verso un nodo ancora aperto con il codice di un componente
	// che c'e' (l'editor lo disegna come quel componente): il nodo si ritrova, e l'arco si decide per quello
	// che e', non si chiude come «tolto»
	perChiave := map[ChiaveRelazione]uuid.UUID{}
	for _, p := range cx.Proposte {
		perChiave[ChiaveRelazione{Allegato: p.AllegatoID, Padre: p.Chiave}] = p.PropostaID
	}
	for _, r := range v.RelazioniViste {
		for _, k := range []string{r.Padre, r.Figlio} {
			id, ok := perChiave[ChiaveRelazione{Allegato: r.Allegato, Padre: k}]
			if !ok || contieneID(pv.Scarta, id) {
				continue
			}
			p := cx.Proposte[id]
			if p.Stato != db.StatoPropostaAperta {
				continue
			}
			if _, scritto := pv.Codici[id]; scritto {
				continue
			}
			if c, ok := cx.PerCodice[strings.ToUpper(strings.TrimSpace(p.Codice.String))]; ok && p.Codice.Valid && c.ArchiviatoIl == nil {
				if _, gia := pv.Ritrovati[id]; !gia {
					pv.Ritrovati[id] = c.ComponenteID
				}
			}
		}
	}
	return pv, nil
}

func contieneID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ l'applicazione

// esitoVoluta conta quello che e' cambiato, per la frase all'operatore.
type esitoVoluta struct {
	nuovi, ritrovati, aggiunti, tolti, quantita, scartati, chiuse, assiemi, codici, tenute, esistenti int
	senzaPadre                                                                                        []string
}

// ApplicaStrutturaVoluta porta la working alla struttura voluta sotto il prodotto v.Radice: nodi proposti
// accettati, componenti scritti dall'operatore che nascono, archi tolti, cambiati e aggiunti, proposte viste
// e non volute chiuse, in una transazione sola.
func ApplicaStrutturaVoluta(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, v StrutturaVoluta) (string, error) {
	if err := prepara(ctx, q, thread, "si cambia la struttura"); err != nil {
		return "", err
	}
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return "", err
	}
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return "", err
	}
	righeNodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return "", err
	}
	nodi := make([]db.ComponenteProposta, len(righeNodi))
	for i, n := range righeNodi {
		nodi[i] = n.ComponenteProposta
	}
	richiesta, err := codiciDellaRichiesta(ctx, q, thread)
	if err != nil {
		return "", err
	}
	cx := nuovoContestoVoluta(comp, rel, nodi, richiesta)
	righeRel, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return "", err
	}
	cx.Relazioni = map[ChiaveRelazione]db.RelazioneProposta{}
	for _, r := range righeRel {
		x := r.RelazioneProposta
		cx.Relazioni[ChiaveRelazione{Allegato: x.AllegatoID, Padre: x.PadreChiave, Figlio: x.FiglioChiave}] = x
	}
	// Le regole del cliente si leggono o si fallisce: senza gli alias dei suffissi un nodo «X» non
	// ritroverebbe il pezzo nato come «X_PRT» e ne nascerebbe un secondo. Un errore qui e' del database,
	// su una transazione che non arriverebbe comunque in fondo.
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return "", err
	}
	cx.conAlias(m)
	cx.Motore = m
	pv, err := pianifica(cx, v)
	if err != nil {
		return "", err
	}
	// le proposte di arco che l'editor ha mostrato, fissate prima di scrivere: quelle che un'accettazione
	// aggancia per effetto collaterale (un nodo con lo stesso codice in un altro file) non sono state viste
	viste := map[ChiaveRelazione]bool{}
	for _, r := range v.RelazioniViste {
		viste[ChiaveRelazione{Allegato: r.Allegato, Padre: r.Padre, Figlio: r.Figlio}] = true
	}
	var es esitoVoluta

	// 1. i codici scritti dall'operatore sui nodi proposti, gli scarti (con gli archi che li toccano)
	idCodici := make([]uuid.UUID, 0, len(pv.Codici))
	for id := range pv.Codici {
		idCodici = append(idCodici, id)
	}
	sort.Slice(idCodici, func(i, j int) bool { return idCodici[i].String() < idCodici[j].String() })
	for _, id := range idCodici {
		c := pv.Codici[id]
		n, err := q.SetCodiceComponenteProposta(ctx, db.SetCodiceComponentePropostaParams{PropostaID: id,
			Codice: pgtype.Text{String: c.Codice, Valid: true}, Rev: testo(c.Rev)})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", Rifiuto(nomeNodo(cx.Proposte[id]) + ": la proposta è stata decisa nel frattempo")
		}
		es.codici++
	}
	for _, id := range pv.Scarta {
		n, err := scartaNodoConArchi(ctx, q, thread, id, utente)
		if err != nil {
			return "", err
		}
		es.scartati++
		es.chiuse += n
	}

	// 2. i nodi che entrano: i codici nuovi nascono (da una proposta, o dal codice scritto su una carta
	// «n:»), quelli gia' noti si ritrovano
	risolto := map[chiaveNodo]uuid.UUID{}
	chiavi := make([]chiaveNodo, 0, len(pv.Nuovi))
	for k := range pv.Nuovi {
		chiavi = append(chiavi, k)
	}
	sort.Slice(chiavi, func(i, j int) bool { return chiavi[i] < chiavi[j] })
	for _, k := range chiavi {
		if !pv.Albero[k] {
			continue
		}
		codice := strings.TrimPrefix(string(k), "n:")
		if ids := pv.Nuovi[k]; len(ids) > 0 {
			// la proposta con il codice scritto dall'operatore prima, poi le altre in un ordine fisso: e' la
			// sua revisione e la sua descrizione che il componente prende
			sort.Slice(ids, func(i, j int) bool {
				_, si := pv.Codici[ids[i]]
				_, sj := pv.Codici[ids[j]]
				if si != sj {
					return si
				}
				return ids[i].String() < ids[j].String()
			})
			fatto := false
			for _, id := range ids {
				p, err := q.BloccaComponenteProposta(ctx, id)
				if err != nil {
					return "", err
				}
				if p.Stato != db.StatoPropostaAperta {
					continue
				}
				if _, err := accettaNodo(ctx, q, p, utente, tipoVoluto(p, pv.Figli[k])); err != nil {
					return "", err
				}
				fatto = true
				break
			}
			if !fatto {
				return "", Rifiuto(codice + ": le proposte sono state decise nel frattempo: riapri l'editor")
			}
		} else {
			// la carta «n:» (U4): il codice scritto dall'operatore, il tipo che ha scelto (assieme se nella
			// struttura ha dei figli, E37), origine manuale. Nessuna riconciliazione: un nodo aperto con lo
			// stesso codice in uno STEP resta una proposta, che si decide dove la si vede. Il controllo che il
			// codice non ci sia gia' l'ha fatto pianifica, sotto il lucchetto della RFQ
			n := pv.Scritti[k]
			tipo := n.Tipo
			if pv.Figli[k] > 0 {
				tipo = db.TipoComponenteSottoassieme
			}
			if _, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: thread, Codice: n.Codice, Rev: testo(n.Rev), Qta: 1,
				Tipo: tipo, Origine: db.OrigineComponenteManuale, ConfermatoDa: utente}); err != nil {
				return "", err
			}
		}
		c, err := componenteNato(ctx, q, thread, codice)
		if err != nil {
			return "", err
		}
		risolto[k] = c.ComponenteID
		es.nuovi++
	}
	ritrovati := make([]uuid.UUID, 0, len(pv.Ritrovati))
	for id := range pv.Ritrovati {
		ritrovati = append(ritrovati, id)
	}
	sort.Slice(ritrovati, func(i, j int) bool { return ritrovati[i].String() < ritrovati[j].String() })
	for _, id := range ritrovati {
		if !pv.Albero[chiaveComponente(pv.Ritrovati[id])] {
			continue
		}
		p, err := q.BloccaComponenteProposta(ctx, id)
		if err != nil {
			return "", err
		}
		if p.Stato != db.StatoPropostaAperta {
			continue // riconciliata da un'accettazione di poco fa
		}
		if _, err := accettaNodo(ctx, q, p, utente, ""); err != nil {
			return "", err
		}
		es.ritrovati++
	}
	for id := range pv.Esistenti {
		if pv.Albero[chiaveComponente(id)] {
			es.esistenti++
		}
	}
	compDi := func(k chiaveNodo) uuid.UUID {
		if id, ok := k.componente(); ok {
			return id
		}
		return risolto[k]
	}

	// 3. gli archi: prima si tolgono, poi le quantita', poi si aggiungono. Il grafo finale e' senza cicli, e
	// ogni grafo di mezzo ne e' un pezzo: nessun arco nuovo si scontra con uno che sta per andarsene.
	nelAlbero := map[uuid.UUID]bool{}
	for k := range pv.Albero {
		nelAlbero[compDi(k)] = true
	}
	voluti := map[Arco]int32{}
	for _, a := range pv.Archi {
		voluti[Arco{Padre: compDi(a.Padre), Figlio: compDi(a.Figlio)}] = a.Qta
	}
	attuali := map[Arco]int32{}
	for _, r := range rel {
		if nelAlbero[r.PadreID] {
			attuali[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r.Qta
		}
	}
	var tolti []Arco
	for _, a := range ordinaArchi(attuali) {
		if _, visto := pv.Visti[a]; !visto {
			continue
		}
		if _, ok := voluti[a]; ok {
			continue
		}
		if _, err := scollega(ctx, q, thread, a.Padre, a.Figlio); err != nil {
			return "", err
		}
		tolti = append(tolti, a)
		es.tolti++
	}
	for _, a := range ordinaArchi(voluti) {
		if qa, ok := attuali[a]; ok && qa != voluti[a] {
			if _, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{PadreID: a.Padre, FiglioID: a.Figlio, Qta: voluti[a], ConfermatoDa: utente}); err != nil {
				return "", err
			}
			es.quantita++
		}
	}
	aperte, err := proposteArcoAperte(ctx, q, thread)
	if err != nil {
		return "", err
	}
	proposti := proposteSullaCoppia(aperte)
	archi, err := archiAttivi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	for _, a := range ordinaArchi(voluti) {
		if _, ok := attuali[a]; ok {
			continue
		}
		fatto := false
		for _, r := range proposti[a] {
			if r.Qta != voluti[a] {
				continue
			}
			k := ChiaveRelazione{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave}
			ora, err := q.BloccaRelazioneProposta(ctx, db.BloccaRelazionePropostaParams{ThreadID: thread, AllegatoID: k.Allegato,
				PadreChiave: k.Padre, FiglioChiave: k.Figlio})
			if err != nil {
				return "", err
			}
			if ora.Stato != db.StatoPropostaAperta {
				continue
			}
			if _, err := accettaRelazione(ctx, q, thread, k, utente, &archi); err != nil {
				return "", err
			}
			fatto = true
			break
		}
		if !fatto {
			if _, _, err := collega(ctx, q, thread, a.Padre, a.Figlio, utente, voluti[a]); err != nil {
				return "", err
			}
			archi = append(archi, a)
		}
		es.aggiunti++
	}

	// 4. le proposte di arco: quelle uguali a un arco voluto sono duplicati; quelle che l'editor ha mostrato e
	// la struttura non vuole (o vuole con un'altra quantita') si chiudono con il motivo. Le altre restano.
	aperte, err = proposteArcoAperte(ctx, q, thread)
	if err != nil {
		return "", err
	}
	// la proposta di un nodo: il suo id, per sapere se l'editor ne mostrava la carta
	nodoDi := map[ChiaveRelazione]uuid.UUID{}
	for _, n := range nodi {
		nodoDi[ChiaveRelazione{Allegato: n.AllegatoID, Padre: n.Chiave}] = n.PropostaID
	}
	for _, x := range aperte {
		if !nelAlbero[x.Padre] {
			continue
		}
		// sotto un componente entrato per codice (non disegnato, e non dalla carta del suo nodo) l'editor non
		// mostrava le proposte: se ne prende solo quella uguale a un arco voluto, le altre restano a chi le vede
		mostrata := pv.Disegnati[x.Padre] || pv.Carte[nodoDi[ChiaveRelazione{Allegato: x.R.AllegatoID, Padre: x.R.PadreChiave}]]
		k := ChiaveRelazione{Allegato: x.R.AllegatoID, Padre: x.R.PadreChiave, Figlio: x.R.FiglioChiave}
		if x.Stesso {
			// due nodi con lo stesso codice uno dentro l'altro (STEP veri, A1.1): adesso sono lo stesso componente
			if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaScartata, "padre e figlio sono lo stesso componente: un pezzo non contiene se stesso", utente); err != nil {
				return "", err
			}
			es.chiuse++
			continue
		}
		qv, voluto := int32(0), false
		if x.Figlio.Valid {
			qv, voluto = voluti[Arco{Padre: x.Padre, Figlio: x.Figlio.UUID}]
		}
		if !(voluto && qv == x.R.Qta) && (!viste[k] || !mostrata) {
			continue
		}
		ora, err := q.BloccaRelazioneProposta(ctx, db.BloccaRelazionePropostaParams{ThreadID: thread, AllegatoID: k.Allegato,
			PadreChiave: k.Padre, FiglioChiave: k.Figlio})
		if err != nil {
			return "", err
		}
		if ora.Stato != db.StatoPropostaAperta {
			continue
		}
		switch {
		case voluto && qv == ora.Qta:
			if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaDuplicato, "", utente); err != nil {
				return "", err
			}
		case voluto:
			if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaScartata,
				fmt.Sprintf("quantità decisa nella struttura: ×%d invece di ×%d", qv, ora.Qta), utente); err != nil {
				return "", err
			}
			es.chiuse++
		default:
			if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaScartata, "tolta nella struttura confermata", utente); err != nil {
				return "", err
			}
			es.chiuse++
		}
	}

	// 5. un particolare che adesso ha dei figli e' un assieme
	for k := range pv.Albero {
		id := compDi(k)
		c, ok := cx.Componenti[id]
		if !ok || pv.Figli[k] == 0 || c.Tipo != db.TipoComponenteSciolto {
			continue
		}
		if err := q.SetTipoComponente(ctx, db.SetTipoComponenteParams{ComponenteID: id, Tipo: db.TipoComponenteSottoassieme, ConfermatoDa: utente}); err != nil {
			return "", err
		}
		es.assiemi++
	}

	// chi e' uscito dall'albero senza altri padri torna fra le radici, da sistemare
	dopoArchi, err := archiAttivi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	haPadre := map[uuid.UUID]bool{}
	for _, a := range dopoArchi {
		haPadre[a.Figlio] = true
	}
	for _, a := range tolti {
		if nome := cx.nome(chiaveComponente(a.Figlio)); !haPadre[a.Figlio] && !contiene(es.senzaPadre, nome) {
			es.senzaPadre = append(es.senzaPadre, nome)
		}
	}
	sort.Strings(es.senzaPadre)
	if _, err := dopoLaDecisione(ctx, q, thread, "", nil); err != nil {
		return "", err
	}
	// 6. le rimozioni che lo STEP strutturale propone su archi appena confermati: l'operatore li ha tenuti
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return "", err
	}
	for _, r := range rim {
		a := Arco{Padre: r.PadreID, Figlio: r.FiglioID}
		// un arco tenuto non e' stato confermato da nessuno: la sua rimozione proposta resta a chi la vede
		if _, voluto := voluti[a]; !voluto || !nelAlbero[r.PadreID] || pv.Tenute[a] || !pv.Disegnati[r.PadreID] {
			continue
		}
		n, err := q.TieniArcoDellaRimozione(ctx, db.TieniArcoDellaRimozioneParams{ThreadID: thread, StepDocumentoID: r.StepDocumentoID,
			PadreID: r.PadreID, FiglioID: r.FiglioID, DecisoDa: uid(utente)})
		if err != nil {
			return "", err
		}
		es.tenute += int(n)
	}
	return fraseVoluta(cx.Componenti[v.Radice].Codice, es), nil
}

// componenteNato e' il componente di un codice nuovo della struttura voluta, appena nato (o ritrovato)
// accettando la sua proposta. Il codice viene dal piano: se fra la lettura e la scrittura la proposta ha
// cambiato codice (un altro gesto nel frattempo), con quel codice non e' nato niente. E' un rifiuto che
// dice di riaprire l'editor, non un «no rows in result set» grezzo.
func componenteNato(ctx context.Context, q *db.Queries, thread uuid.UUID, codice string) (db.Componente, error) {
	c, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: thread, Upper: codice})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, Rifiuto(codice + ": nel frattempo la proposta ha cambiato codice, e con questo non è nato nessun componente: riapri l'editor")
	}
	return c, err
}

// tipoVoluto e' il tipo con cui nasce un nodo proposto: assieme se nella struttura ha dei figli, altrimenti
// quello che lo STEP suggerisce (mai «prodotto»: sta sotto un padre), altrimenti particolare.
func tipoVoluto(p db.ComponenteProposta, figli int) db.TipoComponente {
	if figli > 0 {
		return db.TipoComponenteSottoassieme
	}
	if p.TipoProposto.Valid && p.TipoProposto.TipoComponente != db.TipoComponenteFinito {
		return p.TipoProposto.TipoComponente
	}
	return db.TipoComponenteSciolto
}

// propostaArco e' una proposta di arco aperta il cui padre e' gia' un componente; il figlio puo' essere
// ancora un nodo aperto (Figlio non valido). Stesso: padre e figlio sono diventati lo stesso componente.
type propostaArco struct {
	R      db.RelazioneProposta
	Padre  uuid.UUID
	Figlio uuid.NullUUID
	Stesso bool
}

// proposteArcoAperte sono le proposte di arco aperte con il padre gia' deciso, in un ordine fisso.
func proposteArcoAperte(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]propostaArco, error) {
	nodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	type chiave struct {
		a uuid.UUID
		k string
	}
	comp := map[chiave]uuid.NullUUID{}
	for _, n := range nodi {
		p := n.ComponenteProposta
		if p.Stato == db.StatoPropostaConfermata || p.Stato == db.StatoPropostaDuplicato {
			comp[chiave{p.AllegatoID, p.Chiave}] = p.ComponenteID
		}
	}
	var out []propostaArco
	for _, r := range archi {
		x := r.RelazioneProposta
		if x.Stato != db.StatoPropostaAperta {
			continue
		}
		p, f := comp[chiave{x.AllegatoID, x.PadreChiave}], comp[chiave{x.AllegatoID, x.FiglioChiave}]
		if !p.Valid {
			continue
		}
		out = append(out, propostaArco{R: x, Padre: p.UUID, Figlio: f, Stesso: f.Valid && f.UUID == p.UUID})
	}
	return out, nil
}

// proposteSullaCoppia sono le proposte di arco con i due nodi decisi, per coppia di componenti.
func proposteSullaCoppia(aperte []propostaArco) map[Arco][]db.RelazioneProposta {
	out := map[Arco][]db.RelazioneProposta{}
	for _, x := range aperte {
		if x.Figlio.Valid && !x.Stesso {
			a := Arco{Padre: x.Padre, Figlio: x.Figlio.UUID}
			out[a] = append(out[a], x.R)
		}
	}
	return out
}

func ordinaArchi(m map[Arco]int32) []Arco {
	out := make([]Arco, 0, len(m))
	for a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Padre != out[j].Padre {
			return out[i].Padre.String() < out[j].Padre.String()
		}
		return out[i].Figlio.String() < out[j].Figlio.String()
	})
	return out
}

func contiene(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// fraseVoluta dice all'operatore che cosa e' cambiato.
func fraseVoluta(radice string, es esitoVoluta) string {
	var parti []string
	if es.nuovi > 0 {
		parti = append(parti, quanti(es.nuovi, "componente nuovo", "componenti nuovi"))
	}
	if es.ritrovati > 0 {
		parti = append(parti, quanti(es.ritrovati, "nodo proposto ritrovato nella BOM", "nodi proposti ritrovati nella BOM"))
	}
	if es.esistenti > 0 {
		parti = append(parti, quanti(es.esistenti, "codice scritto già nella BOM, collegato lo stesso componente", "codici scritti già nella BOM, collegati gli stessi componenti"))
	}
	if es.aggiunti > 0 {
		parti = append(parti, quanti(es.aggiunti, "legame aggiunto", "legami aggiunti"))
	}
	if es.tolti > 0 {
		parti = append(parti, quanti(es.tolti, "legame tolto", "legami tolti"))
	}
	if es.quantita > 0 {
		parti = append(parti, quanti(es.quantita, "quantità cambiata", "quantità cambiate"))
	}
	if es.assiemi > 0 {
		parti = append(parti, quanti(es.assiemi, "particolare diventa assieme", "particolari diventano assiemi"))
	}
	if es.scartati > 0 {
		parti = append(parti, quanti(es.scartati, "nodo proposto scartato", "nodi proposti scartati"))
	}
	if es.chiuse > 0 {
		parti = append(parti, quanti(es.chiuse, "proposta di legame chiusa", "proposte di legame chiuse"))
	}
	if es.tenute > 0 {
		parti = append(parti, quanti(es.tenute, "rimozione proposta dallo STEP chiusa: il legame resta", "rimozioni proposte dallo STEP chiuse: i legami restano"))
	}
	if es.codici > 0 {
		parti = append(parti, quanti(es.codici, "codice scritto", "codici scritti"))
	}
	if len(parti) == 0 {
		return "Nessun cambiamento: la struttura di " + radice + " è già così."
	}
	frase := "Struttura di " + radice + " confermata: " + strings.Join(parti, ", ") + "."
	if len(es.senzaPadre) > 0 {
		frase += " Fuori dalla struttura e senza padre, da sistemare: " + strings.Join(es.senzaPadre, ", ") + "."
	}
	return frase
}

// scartaNodoConArchi scarta un nodo proposto e chiude gli archi proposti che lo toccano (sono le righe che
// l'operatore ha visto andare via con lui nell'editor). Restituisce quanti archi ha chiuso.
func scartaNodoConArchi(ctx context.Context, q *db.Queries, thread, proposta, utente uuid.UUID) (int, error) {
	p, err := q.BloccaComponenteProposta(ctx, proposta)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.ThreadID != thread) {
		return 0, Rifiuto("la proposta non è di questa RFQ")
	}
	if err != nil {
		return 0, err
	}
	n, err := q.DecidiComponenteProposta(ctx, db.DecidiComponentePropostaParams{PropostaID: proposta, Stato: db.StatoPropostaScartata, DecisoDa: uid(utente)})
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, Rifiuto(nomeNodo(p) + ": la proposta è già decisa")
	}
	rel, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: p.AllegatoID})
	if err != nil {
		return 0, err
	}
	chiuse := 0
	for _, r := range rel {
		if r.Stato != db.StatoPropostaAperta || (r.PadreChiave != p.Chiave && r.FiglioChiave != p.Chiave) {
			continue
		}
		k := ChiaveRelazione{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave}
		if err := decidiRelazione(ctx, q, thread, k, db.StatoPropostaScartata, "nodo scartato nella struttura: "+nomeNodo(p), utente); err != nil {
			return 0, err
		}
		chiuse++
	}
	return chiuse, nil
}
