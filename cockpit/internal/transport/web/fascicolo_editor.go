package web

// I dati dell'editor della struttura (Fascicolo v3): GET /thread/{id}/fascicolo/bom/dati?prodotto=<id>.
//
// L'editor li chiede quando si apre, freschi: la working intera (tutti gli archi attivi, perche' la conferma
// dice al server quali archi l'editor conosceva), i nodi e gli archi proposti dagli STEP con i nodi dello
// stesso codice gia' uniti in una carta sola, le rimozioni proposte dallo STEP strutturale e lo stato dello
// STEP del prodotto. La conferma torna a POST .../bom/applica (fascicolo_gesti_v3.go).
//
// Smistamento F2 (R1, U4): i codici trovati nella RFQ non sono piu' carte dell'editor (le carte «k:»):
// erano evidenze che diventavano componenti trascinandole. Un pezzo che lo STEP non propone lo si scrive con
// «+ Componente con codice» (la carta «n:»), e prima di metterla l'editor chiede a GET .../bom/codice che
// cosa la RFQ sa di quel codice: se c'e' gia', se e' un codice della richiesta, se e' quasi uguale a uno che
// c'e' (P4).
//
// Smistamento F5 (A5.3.11, P13): le proposte che l'editor puo' prendere sono solo quelle nell'AUTORITA' di un
// file autorizzato (i figli diretti della sorgente e gli archi dalla sorgente). Un figlio diretto con il codice
// di un componente e' disegnato come quel componente e detto «ritrovato per codice» (Ritrovati); la conferma lo
// accetta solo se l'editor l'ha mostrato. La GUIDA (il resto dei file autorizzati e i file non autorizzati) si
// vede, distinta, per aiutare l'operatore (precisazione dell'utente del 27/09): nessun gesto la porta nella BOM,
// e il codice uguale a quello di un componente vi compare solo come suggerimento.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

type nodoEditor struct {
	Codice      string `json:"codice"`
	Rev         string `json:"rev,omitempty"`
	Desc        string `json:"desc,omitempty"`
	Tipo        string `json:"tipo,omitempty"`
	Finito      bool   `json:"finito,omitempty"`
	Proposto    bool   `json:"proposto,omitempty"`
	SenzaCodice bool   `json:"senza_codice,omitempty"`
	Nome        string `json:"nome,omitempty"` // il nome nel file, per un nodo senza codice
	File        string `json:"file,omitempty"`
	Nota        string `json:"nota,omitempty"`
}

type arcoEditor struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
	Qta    int32  `json:"qta"`
}

type propostoEditor struct {
	Padre    string    `json:"padre"`
	Figlio   string    `json:"figlio"`
	Qta      int32     `json:"qta"`
	Allegato uuid.UUID `json:"allegato"`
	PK       string    `json:"pk"`
	FK       string    `json:"fk"`
	File     string    `json:"file"`
}

type rimozioneEditor struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
	Step   string `json:"step"`
}

// ritrovatoEditor e' un nodo nell'autorita' che l'editor disegna come il componente con lo stesso codice
// (Smistamento F5, P13): lo si dice, e la conferma lo accetta come quel componente solo se l'editor l'ha
// mostrato. Agganciato: un aggancio per codice di prima dello Smistamento, da confermare.
type ritrovatoEditor struct {
	Proposta   uuid.UUID `json:"proposta"`
	Ref        string    `json:"ref"`
	Codice     string    `json:"codice"`
	File       string    `json:"file"`
	Agganciato bool      `json:"agganciato,omitempty"`
}

// guidaEditor e' un arco di GUIDA (Smistamento F5, precisazione dell'utente del 27/09): si vede nell'editor
// per aiutare l'operatore, distinto dall'autorita', e nessun gesto lo porta nella working BOM. Suggerito e'
// il componente con lo stesso codice del figlio: un suggerimento calcolato qui, mai un'identita'.
type guidaEditor struct {
	File      string `json:"file"`
	Padre     string `json:"padre"`
	Figlio    string `json:"figlio"`
	Qta       int32  `json:"qta"`
	Suggerito string `json:"suggerito,omitempty"`
}

type prodottoEditor struct {
	Ref    string `json:"ref"`
	Codice string `json:"codice"`
	Desc   string `json:"desc,omitempty"`
}

type datiEditor struct {
	Prodotto  string                `json:"prodotto"`
	Prodotti  []prodottoEditor      `json:"prodotti"`
	Nodi      map[string]nodoEditor `json:"nodi"`
	Archi     []arcoEditor          `json:"archi"`
	Proposti  []propostoEditor      `json:"proposti"`  // solo l'autorita' dei file autorizzati
	Ritrovati []ritrovatoEditor     `json:"ritrovati"` // nell'autorita', con il codice di un componente
	Guida     []guidaEditor         `json:"guida"`     // da vedere, non entra
	Rimozioni []rimozioneEditor     `json:"rimozioni"`
	Step      map[string]string     `json:"step,omitempty"`
	Analisi   int64                 `json:"analisi"`  // analisi ancora in corso sui file della RFQ
	Bloccata  int32                 `json:"bloccata"` // la BOM e' congelata in questa versione
	Scrive    bool                  `json:"scrive"`
}

// fascicoloDatiEditor: GET .../bom/dati?prodotto=<componente>&step=<allegato>. JSON. Senza prodotto l'editor
// si apre sul prodotto da cui si raggiunge la struttura proposta da quello STEP (o da uno STEP qualunque).
func (s *Server) fascicoloDatiEditor(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	prodotto, _ := uuid.Parse(r.URL.Query().Get("prodotto"))
	step, _ := uuid.Parse(r.URL.Query().Get("step"))
	d, err := s.datiEditor(r.Context(), db.New(s.Pool), thread, prodotto, step, almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore))
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(d)
}

func (s *Server) datiEditor(ctx context.Context, q *db.Queries, thread, prodotto, step uuid.UUID, scrive bool) (*datiEditor, error) {
	if _, err := q.GetThread(ctx, thread); err != nil {
		return nil, err
	}
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return nil, err
	}
	aut, nodi, archi, err := fascicolo.LeggiAutorita(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return nil, err
	}
	d := &datiEditor{Nodi: map[string]nodoEditor{}, Scrive: scrive, Archi: []arcoEditor{}, Proposti: []propostoEditor{},
		Ritrovati: []ritrovatoEditor{}, Guida: []guidaEditor{}, Rimozioni: []rimozioneEditor{}, Prodotti: []prodottoEditor{}}
	if n, bloccata, err := fascicolo.WorkingBloccata(ctx, q, thread); err != nil {
		return nil, err
	} else if bloccata {
		d.Bloccata = n
	}
	d.Analisi, _ = q.ContaAnalisiInCorso(ctx, uuid.NullUUID{UUID: thread, Valid: true})

	attivo := map[uuid.UUID]db.Componente{}
	perCodice := map[string]db.Componente{}
	for _, c := range comp {
		if c.ArchiviatoIl != nil {
			continue
		}
		attivo[c.ComponenteID] = c
		perCodice[strings.ToUpper(strings.TrimSpace(c.Codice))] = c
		ref := "c:" + c.ComponenteID.String()
		d.Nodi[ref] = nodoEditor{Codice: c.Codice, Rev: c.Rev.String, Desc: c.Descrizione.String, Tipo: string(c.Tipo),
			Finito: c.Tipo == db.TipoComponenteFinito}
		if c.Tipo == db.TipoComponenteFinito {
			d.Prodotti = append(d.Prodotti, prodottoEditor{Ref: ref, Codice: c.Codice, Desc: c.Descrizione.String})
		}
	}
	for _, p := range d.Prodotti {
		if p.Ref == "c:"+prodotto.String() {
			d.Prodotto = p.Ref
		}
	}
	for _, r := range rel {
		d.Archi = append(d.Archi, arcoEditor{Padre: "c:" + r.PadreID.String(), Figlio: "c:" + r.FiglioID.String(), Qta: r.Qta})
	}

	// I nodi proposti, solo nell'autorita' (Smistamento F5): la sorgente di un file autorizzato e' il suo
	// componente; un nodo deciso da una persona e' il suo componente; un figlio diretto aperto con il codice di
	// un componente e' disegnato come quel componente e detto «ritrovato per codice»; i figli diretti aperti
	// con lo stesso codice nello stesso file sono una carta sola (la prima proposta). Lo stesso codice in un
	// altro file e' un'altra carta: accettarne una non decide l'altra (A5.4.7). La guida non fa carte: si
	// elenca a parte.
	type chiave struct {
		a uuid.UUID
		k string
	}
	refDi := map[chiave]string{}
	etichetta := map[chiave]string{}
	canonico := map[chiave]string{} // (file, codice) → la carta
	for _, n := range nodi {
		p := n.ComponenteProposta
		k := chiave{p.AllegatoID, p.Chiave}
		etichetta[k] = etichettaNodo(p)
		if x, ok := aut.Sorgente(p.AllegatoID, p.Chiave); ok {
			if c, ok := attivo[x.Componente.ComponenteID]; ok {
				refDi[k] = "c:" + c.ComponenteID.String()
				etichetta[k] = c.Codice
			}
			continue
		}
		if id, ok := fascicolo.DecisoDaUnaPersona(p); ok {
			if c, ok := attivo[id]; ok {
				refDi[k] = "c:" + c.ComponenteID.String()
			}
			continue
		}
		if _, figlio := aut.FiglioDiretto(p.AllegatoID, p.Chiave); !figlio {
			continue // guida
		}
		if fascicolo.AgganciatoPerCodice(p) {
			if c, ok := attivo[p.ComponenteID.UUID]; ok {
				ref := "c:" + c.ComponenteID.String()
				refDi[k] = ref
				d.Ritrovati = append(d.Ritrovati, ritrovatoEditor{Proposta: p.PropostaID, Ref: ref, Codice: c.Codice, File: n.NomeFile, Agganciato: true})
			}
			continue
		}
		if p.Stato != db.StatoPropostaAperta {
			continue
		}
		codice := strings.TrimSpace(p.Codice.String)
		up := strings.ToUpper(codice)
		switch {
		case codice == "":
			ref := "p:" + p.PropostaID.String()
			refDi[k] = ref
			d.Nodi[ref] = nodoEditor{Nome: p.NomeGrezzo, Desc: p.Descrizione.String, Tipo: tipoProposto(p), Proposto: true,
				SenzaCodice: true, File: n.NomeFile, Nota: p.Nota.String}
		case perCodice[up].ComponenteID != uuid.Nil:
			ref := "c:" + perCodice[up].ComponenteID.String()
			refDi[k] = ref
			d.Ritrovati = append(d.Ritrovati, ritrovatoEditor{Proposta: p.PropostaID, Ref: ref, Codice: perCodice[up].Codice, File: n.NomeFile})
		case canonico[chiave{p.AllegatoID, up}] != "":
			refDi[k] = canonico[chiave{p.AllegatoID, up}]
		default:
			ref := "p:" + p.PropostaID.String()
			canonico[chiave{p.AllegatoID, up}], refDi[k] = ref, ref
			d.Nodi[ref] = nodoEditor{Codice: codice, Rev: p.Rev.String, Desc: p.Descrizione.String, Tipo: tipoProposto(p),
				Proposto: true, Nome: p.NomeGrezzo, File: n.NomeFile, Nota: p.Nota.String}
		}
	}
	perNodo := map[chiave]db.ComponenteProposta{}
	for _, n := range nodi {
		perNodo[chiave{n.ComponenteProposta.AllegatoID, n.ComponenteProposta.Chiave}] = n.ComponenteProposta
	}
	for _, r := range archi {
		x := r.RelazioneProposta
		if x.Stato != db.StatoPropostaAperta {
			continue
		}
		pk, fk := chiave{x.AllegatoID, x.PadreChiave}, chiave{x.AllegatoID, x.FiglioChiave}
		if _, ok := aut.ArcoAutorizzato(fascicolo.ChiaveRelazione{Allegato: x.AllegatoID, Padre: x.PadreChiave, Figlio: x.FiglioChiave}); !ok {
			// la guida: si elenca, con il suggerimento del codice uguale, e non entra
			if f := perNodo[fk]; f.Stato != db.StatoPropostaScartata && perNodo[pk].Stato != db.StatoPropostaScartata {
				g := guidaEditor{File: r.NomeFile, Padre: etichetta[pk], Figlio: etichetta[fk], Qta: x.Qta}
				if c, ok := perCodice[strings.ToUpper(strings.TrimSpace(f.Codice.String))]; ok && f.Codice.Valid {
					g.Suggerito = c.Codice
				}
				d.Guida = append(d.Guida, g)
			}
			continue
		}
		pr, fr := refDi[pk], refDi[fk]
		if pr == "" || fr == "" || pr == fr {
			continue
		}
		d.Proposti = append(d.Proposti, propostoEditor{Padre: pr, Figlio: fr, Qta: x.Qta, Allegato: x.AllegatoID,
			PK: x.PadreChiave, FK: x.FiglioChiave, File: r.NomeFile})
	}
	// senza un prodotto scelto: quello da cui si raggiunge la struttura proposta dallo STEP (con gli archi della
	// working e quelli proposti), poi quello di una proposta qualunque, poi il primo
	if d.Prodotto == "" {
		for _, solo := range []bool{true, false} {
			if solo && step == uuid.Nil {
				continue
			}
			for _, p := range d.Prodotti {
				if d.Prodotto != "" {
					break
				}
				raggiunti := raggiuntiDa(p.Ref, d.Archi, d.Proposti)
				for _, x := range d.Proposti {
					if (!solo || x.Allegato == step) && raggiunti[x.Padre] {
						d.Prodotto = p.Ref
						break
					}
				}
			}
		}
	}
	if d.Prodotto == "" && len(d.Prodotti) > 0 {
		d.Prodotto = d.Prodotti[0].Ref
	}
	if len(rim) > 0 {
		documenti, err := q.ListDocumentiThread(ctx, thread)
		if err != nil {
			return nil, err
		}
		nomi := map[uuid.UUID]string{}
		for _, x := range documenti {
			nomi[x.DocumentoID] = x.NomeFile
		}
		for _, x := range rim {
			if !aut.Dichiarazioni.RimozioneValida(x.StepDocumentoID, x.PadreID) {
				continue // di un'autorizzazione che non vale (sospesa, in conflitto): il file non ha autorita'
			}
			d.Rimozioni = append(d.Rimozioni, rimozioneEditor{Padre: "c:" + x.PadreID.String(), Figlio: "c:" + x.FiglioID.String(), Step: nomi[x.StepDocumentoID]})
		}
	}

	// lo STEP del prodotto
	if id, err := uuid.Parse(strings.TrimPrefix(d.Prodotto, "c:")); err == nil {
		if sp, err := q.ListStepProdotto(ctx, thread); err == nil {
			for _, x := range sp {
				if x.ComponenteID == id {
					d.Step = map[string]string{"esito": x.Esito, "etichetta": fascicolo.EtichettaStep(x.Esito), "motivo": x.MotivoParziale.String}
				}
			}
		}
	}
	return d, nil
}

// codiceEditor e' la risposta di GET .../bom/codice?codice=<scritto>: che cosa la RFQ sa del codice che
// l'operatore scrive con «+ Componente con codice» (U4). Non scrive niente; la conferma rifa' lo stesso
// controllo sotto il lucchetto della RFQ (fascicolo.ApplicaStrutturaVoluta).
type codiceEditor struct {
	Codice string `json:"codice"`
	Errore string `json:"errore,omitempty"`
	// Esiste: il componente con questo codice (lo stesso pezzo: la carta e' lui, mai una seconda riga).
	Esiste *codiceEsistente `json:"esiste,omitempty"`
	// Richiesta: e' un codice della richiesta, che diventa prodotto con una decisione del triage o, se
	// confermato con la BOM congelata, aprendo la revisione.
	Richiesta bool `json:"richiesta,omitempty"`
	// Vicini: i codici quasi uguali (P4): la carta nasce solo se l'operatore dice che e' un pezzo diverso.
	Vicini []fascicolo.Vicino `json:"vicini"`
}

type codiceEsistente struct {
	Ref        string `json:"ref"`
	Codice     string `json:"codice"`
	Tipo       string `json:"tipo"`
	Archiviato bool   `json:"archiviato,omitempty"`
}

// fascicoloCodiceEditor: GET .../bom/codice?codice=<scritto>. JSON.
func (s *Server) fascicoloCodiceEditor(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	if _, err := q.GetThread(r.Context(), thread); err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	e, err := fascicolo.ControllaCodiceNuovo(r.Context(), q, thread, r.URL.Query().Get("codice"))
	if err != nil {
		http.Error(w, "il codice non si è potuto controllare: riprova", 500)
		return
	}
	out := codiceEditor{Codice: e.Codice, Errore: e.Errore, Richiesta: e.Richiesta, Vicini: e.Vicini}
	if out.Vicini == nil {
		out.Vicini = []fascicolo.Vicino{}
	}
	if c := e.Esistente; c != nil {
		out.Esiste = &codiceEsistente{Ref: "c:" + c.ComponenteID.String(), Codice: c.Codice, Tipo: string(c.Tipo), Archiviato: c.ArchiviatoIl != nil}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

// raggiuntiDa sono i nodi che si raggiungono da una radice con gli archi della working e quelli proposti.
func raggiuntiDa(radice string, archi []arcoEditor, proposti []propostoEditor) map[string]bool {
	figli := map[string][]string{}
	for _, a := range archi {
		figli[a.Padre] = append(figli[a.Padre], a.Figlio)
	}
	for _, a := range proposti {
		figli[a.Padre] = append(figli[a.Padre], a.Figlio)
	}
	visti := map[string]bool{radice: true}
	coda := []string{radice}
	for len(coda) > 0 {
		n := coda[0]
		coda = coda[1:]
		for _, f := range figli[n] {
			if !visti[f] {
				visti[f] = true
				coda = append(coda, f)
			}
		}
	}
	return visti
}

// tipoProposto e' il tipo che lo STEP suggerisce per un nodo, «» se non ne suggerisce.
func tipoProposto(p db.ComponenteProposta) string {
	if p.TipoProposto.Valid {
		return string(p.TipoProposto.TipoComponente)
	}
	return ""
}
