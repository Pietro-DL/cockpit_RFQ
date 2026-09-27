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
	Proposti  []propostoEditor      `json:"proposti"`
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
	nodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return nil, err
	}
	d := &datiEditor{Nodi: map[string]nodoEditor{}, Scrive: scrive, Archi: []arcoEditor{}, Proposti: []propostoEditor{},
		Rimozioni: []rimozioneEditor{}, Prodotti: []prodottoEditor{}}
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

	// i nodi proposti: un nodo gia' deciso e' il suo componente; uno aperto con il codice di un componente e'
	// quel componente; quelli aperti con lo stesso codice sono una carta sola (la prima proposta)
	type chiave struct {
		a uuid.UUID
		k string
	}
	refDi := map[chiave]string{}
	canonico := map[string]string{}
	for _, n := range nodi {
		p := n.ComponenteProposta
		k := chiave{p.AllegatoID, p.Chiave}
		switch p.Stato {
		case db.StatoPropostaConfermata, db.StatoPropostaDuplicato:
			if c, ok := attivo[p.ComponenteID.UUID]; ok && p.ComponenteID.Valid {
				refDi[k] = "c:" + c.ComponenteID.String()
			}
		case db.StatoPropostaAperta:
			codice := strings.TrimSpace(p.Codice.String)
			up := strings.ToUpper(codice)
			switch {
			case codice == "":
				ref := "p:" + p.PropostaID.String()
				refDi[k] = ref
				d.Nodi[ref] = nodoEditor{Nome: p.NomeGrezzo, Desc: p.Descrizione.String, Tipo: tipoProposto(p), Proposto: true,
					SenzaCodice: true, File: n.NomeFile, Nota: p.Nota.String}
			case perCodice[up].ComponenteID != uuid.Nil:
				refDi[k] = "c:" + perCodice[up].ComponenteID.String()
			case canonico[up] != "":
				refDi[k] = canonico[up]
			default:
				ref := "p:" + p.PropostaID.String()
				canonico[up], refDi[k] = ref, ref
				d.Nodi[ref] = nodoEditor{Codice: codice, Rev: p.Rev.String, Desc: p.Descrizione.String, Tipo: tipoProposto(p),
					Proposto: true, Nome: p.NomeGrezzo, File: n.NomeFile, Nota: p.Nota.String}
			}
		}
	}
	for _, r := range archi {
		x := r.RelazioneProposta
		if x.Stato != db.StatoPropostaAperta {
			continue
		}
		pr, fr := refDi[chiave{x.AllegatoID, x.PadreChiave}], refDi[chiave{x.AllegatoID, x.FiglioChiave}]
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
	// Richiesta: e' un codice della richiesta, che diventa prodotto con la creazione della RFQ (triage) o
	// aprendo una revisione della BOM congelata.
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
