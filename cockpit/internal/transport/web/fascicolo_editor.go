package web

// I dati dell'editor della struttura (Fascicolo v3): GET /thread/{id}/fascicolo/bom/dati?prodotto=<id>.
//
// L'editor li chiede quando si apre, freschi: la working intera (tutti gli archi attivi, perche' la conferma
// dice al server quali archi l'editor conosceva), i nodi e gli archi proposti dagli STEP con i nodi dello
// stesso codice gia' uniti in una carta sola, i codici trovati nella RFQ che possono diventare componenti
// (per costruire la struttura a mano quando lo STEP non c'e'), le rimozioni proposte dallo STEP strutturale
// e lo stato dello STEP del prodotto. La conferma torna a POST .../bom/applica (fascicolo_gesti_v3.go).

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
	Trovato     bool   `json:"trovato,omitempty"`
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
	Trovati   []string              `json:"trovati"`
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
		Rimozioni: []rimozioneEditor{}, Trovati: []string{}, Prodotti: []prodottoEditor{}}
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

	// i codici trovati nella RFQ che possono nascere come componenti
	if cand, err := fascicolo.CandidatiDellaRfq(ctx, q, thread); err == nil {
		for _, lista := range [][]fascicolo.CodiceCandidato{cand.Prodotto, cand.Altri} {
			for _, k := range lista {
				if k.Stato.Situazione != fascicolo.SituazioneNuovo {
					continue
				}
				ref := "k:" + k.Codice
				if _, gia := d.Nodi[ref]; gia {
					continue
				}
				rev := ""
				if len(k.Revisioni) == 1 {
					rev = k.Revisioni[0].Rev
				}
				d.Nodi[ref] = nodoEditor{Codice: k.Codice, Rev: rev, Trovato: true, Desc: k.Motivo}
				d.Trovati = append(d.Trovati, ref)
			}
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
