package web

// Il pannello di destra del Fascicolo (B8.7b): cambia con quello che si sceglie.
//
//   - un componente della BOM: la sua card, le schede 3D / 2D / DXF / Altri / Storico con i documenti
//     (correnti e sostituiti) e i file del piano che lo aspettano, e sotto l'anteprima del documento aperto;
//     i gesti sul componente stanno in un riquadro che si apre (Modifica, Posizione, STEP strutturale,
//     Deroghe, Archivia: quelli di B8.7);
//   - un nodo proposto da uno STEP: da che file viene, dove starebbe, che file lo aspettano, e i gesti della
//     proposta (accetta, con il sottoalbero, scrivi il codice, scarta); sotto, la struttura letta dallo STEP;
//   - un file (dalla tabella dei documenti, o dal piano): l'anteprima di B8.7, con i suoi gesti.
//
// Il corpo (l'iframe del PDF, la struttura dello STEP) sta in #anteprima-corpo, e un gesto non lo rifa':
// il PDF aperto resta aperto.

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// schedaDettaglio e' una delle schede del dettaglio di un componente.
type schedaDettaglio struct {
	Chiave, Nome string
	N            int  // documenti correnti di quel tipo
	Attesi       int  // file del piano che vanno li'
	Richiesto    bool // il fascicolo lo chiede (c'e' una cella di completezza)
	Cella        *cella
}

// docDettaglio e' un documento del componente nella scheda aperta.
type docDettaglio struct {
	D        db.Documento
	Allegato uuid.UUID // l'allegato da cui e' nato: e' quello che l'anteprima apre
	Corrente bool
	Aperto   bool
}

// dettaglioNodo e' il componente scelto, nel pannello di destra.
type dettaglioNodo struct {
	Carta     *carta
	Scheda    string
	Schede    []schedaDettaglio
	Documenti []docDettaglio
	InArrivo  []fascicolo.VoceFile // i file del piano che vanno a questo componente
	Aperto    *docDettaglio
	Anteprima string // il file in arrivo che l'anteprima mostra, quando la scheda non ha ancora documenti
}

// NomeScheda e' il nome della scheda aperta: 3D, 2D, DXF.
func (n *dettaglioNodo) NomeScheda() string {
	for _, s := range n.Schede {
		if s.Chiave == n.Scheda {
			return s.Nome
		}
	}
	return n.Scheda
}

// propostaScelta e' il nodo proposto scelto, nel pannello di destra.
type propostaScelta struct {
	P        db.ComponenteProposta
	File     string
	Sotto    []string // «sotto 77720517 ×2»
	Figli    []string // «77811111 ×2»
	InArrivo []fascicolo.VoceFile
}

// tipiScheda sono i tipi di documento di ciascuna scheda; «altri» e' tutto il resto.
var tipiScheda = map[string]db.TipoDocumento{"3d": db.TipoDocumentoCad3d, "2d": db.TipoDocumentoDisegno2d, "dxf": db.TipoDocumentoSviluppoDxf}

func schedaDi(t db.TipoDocumento) string {
	for k, x := range tipiScheda {
		if x == t {
			return k
		}
	}
	return "altri"
}

// pannelloDestro prepara il pannello di destra per lo stato della pagina.
func (s *Server) pannelloDestro(ctx context.Context, q *db.Queries, d *fascicoloDati) error {
	var err error
	switch {
	case d.Stato.File != uuid.Nil:
		d.Anteprima, err = s.anteprimaFascicolo(ctx, q, d, d.Stato.File)
	case d.Stato.Nodo != uuid.Nil:
		if c, ok := d.Componenti[d.Stato.Nodo]; ok {
			err = s.dettaglioNelPannello(ctx, q, d, c)
		}
	case d.Stato.Prop != uuid.Nil:
		d.PropScelta = propostaSceltaDi(d, d.Stato.Prop)
		if d.PropScelta == nil {
			break
		}
		// Una proposta gia' decisa e' diventata un componente (accettata, o ritrovata): si mostra quello.
		if p := d.PropScelta.P; p.Stato != db.StatoPropostaAperta && p.ComponenteID.Valid {
			if c, ok := d.Componenti[p.ComponenteID.UUID]; ok && c.ArchiviatoIl == nil {
				d.PropScelta = nil
				return s.dettaglioNelPannello(ctx, q, d, c)
			}
		}
		d.Anteprima, err = s.anteprimaFascicolo(ctx, q, d, d.PropScelta.P.AllegatoID)
	}
	return err
}

// dettaglioNelPannello mette il componente nel pannello di destra, con l'anteprima del documento aperto o, se
// nella scheda non c'e' ancora un documento, del primo file che arriva per lei: il 2D di un pezzo si guarda
// anche prima della conferma.
func (s *Server) dettaglioNelPannello(ctx context.Context, q *db.Queries, d *fascicoloDati, c db.Componente) error {
	d.Dettaglio = dettaglioDi(d, c)
	allegato := uuid.Nil
	switch {
	case d.Dettaglio.Aperto != nil:
		allegato = d.Dettaglio.Aperto.Allegato
	default:
		for _, v := range d.Dettaglio.InArrivo {
			if d.Dettaglio.Scheda == schedaDi(v.Tipo) {
				allegato = v.Allegato
				d.Dettaglio.Anteprima = v.Nome
				break
			}
		}
	}
	if allegato == uuid.Nil {
		return nil
	}
	var err error
	d.Anteprima, err = s.anteprimaFascicolo(ctx, q, d, allegato)
	return err
}

// dettaglioDi costruisce il dettaglio di un componente: la card, le schede, i documenti della scheda aperta.
func dettaglioDi(d *fascicoloDati, c db.Componente) *dettaglioNodo {
	b := &costruttoreBom{d: d, decidere: d.Piano.DaVerificare()}
	for padre, rr := range d.Rimozioni {
		b.decidere[padre] += len(rr)
	}
	k := &carta{Comp: &c, ID: "dettaglio-" + c.ComponenteID.String()}
	b.riempiComponente(k)
	n := &dettaglioNodo{Carta: k}
	for _, v := range d.Piano.File {
		if (v.Componente != nil && v.Componente.ComponenteID == c.ComponenteID) ||
			(v.Componente == nil && v.DaStep == nil && strings.EqualFold(v.Codice, c.Codice)) {
			n.InArrivo = append(n.InArrivo, v)
		}
	}
	docs := d.DocumentiDi[c.ComponenteID]
	conta := map[string]int{}
	for _, x := range docs {
		if !x.SostituitoDa.Valid {
			conta[schedaDi(x.Tipo)]++
		}
	}
	attesi := map[string]int{}
	for _, v := range n.InArrivo {
		attesi[schedaDi(v.Tipo)]++
	}
	celle := map[string]*cella{}
	for i, x := range d.Completezza[c.ComponenteID] {
		if sk := schedaDi(x.Tipo); sk != "altri" {
			celle[sk] = &d.Completezza[c.ComponenteID][i]
		}
	}
	for _, sk := range []struct{ k, nome string }{{"3d", "3D"}, {"2d", "2D"}, {"dxf", "DXF"}, {"altri", "Altri"}, {"storico", "Storico"}} {
		sch := schedaDettaglio{Chiave: sk.k, Nome: sk.nome, N: conta[sk.k], Attesi: attesi[sk.k], Cella: celle[sk.k], Richiesto: celle[sk.k] != nil}
		if sk.k == "storico" {
			sch.N = len(docs) - conta["3d"] - conta["2d"] - conta["dxf"] - conta["altri"] // i sostituiti
		}
		n.Schede = append(n.Schede, sch)
	}
	n.Scheda = d.Stato.Scheda
	if n.Scheda == "" {
		n.Scheda = "2d"
		for _, sk := range []string{"2d", "3d", "dxf", "altri"} {
			if conta[sk] > 0 {
				n.Scheda = sk
				break
			}
		}
	}
	for _, x := range docs {
		corrente := !x.SostituitoDa.Valid
		switch n.Scheda {
		case "storico":
			if corrente {
				continue
			}
		default:
			if !corrente || schedaDi(x.Tipo) != n.Scheda {
				continue
			}
		}
		n.Documenti = append(n.Documenti, docDettaglio{D: x, Allegato: d.AllegatoDi[x.DocumentoID], Corrente: corrente})
	}
	sort.SliceStable(n.Documenti, func(i, j int) bool { return n.Documenti[i].D.NomeFile < n.Documenti[j].D.NomeFile })
	for i := range n.Documenti {
		if (d.Stato.Doc != uuid.Nil && n.Documenti[i].D.DocumentoID == d.Stato.Doc) || (d.Stato.Doc == uuid.Nil && i == 0) {
			n.Documenti[i].Aperto = true
			n.Aperto = &n.Documenti[i]
		}
	}
	return n
}

// propostaSceltaDi costruisce il dettaglio di un nodo proposto; nil se non e' fra le proposte della RFQ.
func propostaSceltaDi(d *fascicoloDati, id uuid.UUID) *propostaScelta {
	var p *propostaScelta
	nomi := map[chiaveNodo]string{}
	for _, n := range d.NodiProposti {
		x := n.ComponenteProposta
		nomi[chiaveNodo{x.AllegatoID, x.Chiave}] = etichettaNodo(x)
		if x.PropostaID == id {
			p = &propostaScelta{P: x, File: n.NomeFile}
		}
	}
	if p == nil {
		return nil
	}
	for _, r := range d.RelazioniProposte {
		x := r.RelazioneProposta
		if x.AllegatoID != p.P.AllegatoID || x.Stato == db.StatoPropostaScartata {
			continue
		}
		switch {
		case x.FiglioChiave == p.P.Chiave:
			p.Sotto = append(p.Sotto, nomi[chiaveNodo{x.AllegatoID, x.PadreChiave}]+" ×"+strconv.Itoa(int(x.Qta)))
		case x.PadreChiave == p.P.Chiave:
			p.Figli = append(p.Figli, nomi[chiaveNodo{x.AllegatoID, x.FiglioChiave}]+" ×"+strconv.Itoa(int(x.Qta)))
		}
	}
	for _, v := range d.Piano.File {
		if v.DaStep != nil && v.DaStep.Proposta == id {
			p.InArrivo = append(p.InArrivo, v)
		}
	}
	return p
}
