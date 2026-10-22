package distintabozza

// L1 — D-48-01 A [T] (preparata in attesa della risposta) nella parte di R4 (documenti, pbDalMotore): una PB del motore
// rimasta intatta su un file con il nome «da partner» (la lettura generica del nome non e' il codice del pezzo) e' applicabile
// sul nodo proposto e sul registrato della voce; torna al confronto del codice letto (codice_diverso) sul componente
// ricodificato (D-ASS-01 [U]), su un nodo in modo manuale, sul registrato con un altro componente da quello della voce, e
// per una PB manuale o corretta (D-48-02 aperta); e su un nodo dove il registro della versione non propone il file (lo
// propone altrove, in modo ambiguo o non lo propone: la PB porta il registro d'origine di prima, D-PB03 A). Dati sintetici: il registro e lo snapshot del formato 3
// (formato3_aiuti_test.go).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// pbMotoreR4: gli ingressi di prova con la sola PB del PDF sul nodo k, con la provenienza data e il registro della
// versione, e il PDF con un nome «da partner» (lettura generica: il nome intero, rev E).
func pbMotoreR4(t *testing.T, modC func(id idProva3, c *importazione.Contenuto), k, prov string) (*ingressiR4, idProva3) {
	t.Helper()
	in, id := ingressiProva3R4(t, modC)
	p := Preassociazione3{AllegatoID: id.pdf, ChiaveBozza: k, Provenienza: prov}
	if prov == ProvenienzaMotore || prov == ProvenienzaCorretta {
		p.Importazione = in.registro.Impronta
	}
	in.snap.DocumentiPreassociati = []Preassociazione3{p}
	in.fatti.Occorrenze[id.pdf] = associazioni.OccorrenzaPiano{DellaRfq: true, Attivo: true, Preassociabile: true, Sha256: id.shaPdf,
		NomeFile: "BOM_9120001_1010_20260101_E.pdf", ConProposta: true, TipoProposto: "disegno_2d", CodiceLetto: "BOM_9120001_1010_20260101", RevLetta: "E"}
	return in, id
}

// pdfSu: il registro propone il PDF sulle voci date (e su nessun'altra).
func pdfSu(t *testing.T, chiavi ...string) func(id idProva3, c *importazione.Contenuto) {
	return func(id idProva3, c *importazione.Contenuto) {
		for i := range c.Proposta.Nodi {
			c.Proposta.Nodi[i].AssociazioniProposte = nil
			for _, k := range chiavi {
				if c.Proposta.Nodi[i].Origine.Rif == id.rif(k) {
					c.Proposta.Nodi[i].AssociazioniProposte = rawDi(t, []any{map[string]any{"allegato_id": id.pdf.String(), "motivo": "nome",
						"livello": "alto"}})
				}
			}
		}
	}
}

// esitoPBdi: l'esito della PB del PDF e i blocchi PREASSOCIAZIONE_NON_APPLICABILE che la nominano.
func esitoPBdi(t *testing.T, in *ingressiR4, id idProva3) (associazioni.EsitoPB, []Rilievo) {
	t.Helper()
	r := calcolaIn(t, in)
	var e associazioni.EsitoPB
	trovata := false
	for _, p := range r.DocumentiPreassociati {
		if p.Allegato == id.pdf {
			e, trovata = p.EsitoPB, true
		}
	}
	if !trovata {
		t.Fatalf("la PB del PDF non e' nel riepilogo: %+v", r.DocumentiPreassociati)
	}
	var bb []Rilievo
	for _, b := range trova(r.Blocchi, BloccoPreassociazioneNonApplicab) {
		if b.AllegatoID != nil && *b.AllegatoID == id.pdf {
			bb = append(bb, b)
		}
	}
	return e, bb
}

func TestR4PBDelMotoreIntatta(t *testing.T) {
	// il nodo proposto (k-0007, modo proposta, ACM-1100 rev A): la PB del motore e' applicabile, anche col tipo da determinare
	in, id := pbMotoreR4(t, nil, "k-0007", ProvenienzaMotore)
	if e, bb := esitoPBdi(t, in, id); e.Esito != associazioni.EsitoApplicabile || len(bb) != 0 {
		t.Fatalf("PB del motore intatta sul nodo proposto: %+v %+v", e, bb)
	}
	in, id = pbMotoreR4(t, nil, "k-0007", ProvenienzaMotore)
	o := in.fatti.Occorrenze[id.pdf]
	o.TipoProposto, o.CodiceLetto, o.RevLetta = associazioni.TipoDaDeterminare, "0_912_0001_0_0_T1", ""
	in.fatti.Occorrenze[id.pdf] = o
	if e, bb := esitoPBdi(t, in, id); e.Esito != associazioni.EsitoApplicabile || len(bb) != 0 {
		t.Fatalf("PB del motore intatta, tipo da determinare: %+v %+v", e, bb)
	}
	// il registrato della voce della radice (k-root, il prodotto), con il registro che propone il PDF sulla radice
	in, id = pbMotoreR4(t, pdfSu(t, "#1"), "k-root", ProvenienzaMotore)
	if e, bb := esitoPBdi(t, in, id); e.Esito != associazioni.EsitoApplicabile || len(bb) != 0 {
		t.Fatalf("PB del motore intatta sul registrato della radice: %+v %+v", e, bb)
	}
	// le PB dell'ingegnere restano confrontate (D-48-02 aperta)
	for _, prov := range []string{ProvenienzaManuale, ProvenienzaCorretta} {
		in, id = pbMotoreR4(t, nil, "k-0007", prov)
		if e, bb := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso || len(bb) != 1 {
			t.Fatalf("PB %s con il nome da partner: %+v %+v", prov, e, bb)
		}
	}
	// i controlli di prima restano anche per la PB del motore (file non attivo)
	in, id = pbMotoreR4(t, nil, "k-0007", ProvenienzaMotore)
	o = in.fatti.Occorrenze[id.pdf]
	o.Attivo = false
	in.fatti.Occorrenze[id.pdf] = o
	if e, _ := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoFileNonAttivo {
		t.Fatalf("PB del motore su un file non attivo: %+v", e)
	}
	// senza il registro della versione la PB non si dice intatta: torna al confronto
	in, id = pbMotoreR4(t, nil, "k-0007", ProvenienzaMotore)
	in.registro = nil
	if e, _ := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso {
		t.Fatalf("PB del motore senza il registro della versione: %+v", e)
	}
}

// Il registro della versione non propone il file sul nodo della PB del motore: la PB non e' intatta e torna al confronto
// (rilievo bloccante della revisione di D-48-01 A). Il nodo ha l'identita' della sua voce (proposta o registrata).
func TestR4PBDelMotoreNonPropostaSulNodo(t *testing.T) {
	for _, caso := range []struct {
		nome, k string
		modC    func(id idProva3, c *importazione.Contenuto)
		altro   bool // la PB porta un registro d'origine diverso da quello della versione
	}{
		{"radice, il registro propone il PDF sul telaio", "k-root", nil, false},
		{"telaio, il registro propone il PDF sulla radice", "k-0007", pdfSu(t, "#1"), false},
		{"telaio, il registro non propone il PDF, registro d'origine di prima", "k-0007", pdfSu(t), true},
		{"telaio, il registro propone il PDF sulla radice, registro d'origine di prima", "k-0007", pdfSu(t, "#1"), true},
		{"telaio, proposta ambigua (telaio e radice)", "k-0007", pdfSu(t, "#1", "#7"), false},
	} {
		in, id := pbMotoreR4(t, caso.modC, caso.k, ProvenienzaMotore)
		if caso.altro {
			in.snap.DocumentiPreassociati[0].Importazione = "sha256:" + strings.Repeat("ab", 32)
		}
		if e, bb := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso || len(bb) != 1 {
			t.Fatalf("PB del motore, %s: %+v %+v", caso.nome, e, bb)
		}
	}
}

// D-ASS-01 [U]: la PB del motore sul componente ricodificato si confronta con l'identita' nuova.
func TestR4PBDelMotoreSulRicodificato(t *testing.T) {
	ricodifica := func(in *ingressiR4, id idProva3, dopo string) {
		p := id.prodotto
		in.snap.Decisioni = append(in.snap.Decisioni, Decisione3{ID: "dr-0001", Tipo: DecisioneRicodifica, ComponenteID: &p,
			CodicePrima: "ACM-1000", CodiceDopo: dopo, EmittenteDopo: id.clienteE(), Motivo: "codice sbagliato"})
	}
	in, id := pbMotoreR4(t, pdfSu(t, "#1"), "k-root", ProvenienzaMotore)
	ricodifica(in, id, "ACM-1001")
	if e, bb := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso || len(bb) != 1 {
		t.Fatalf("PB del motore sul componente ricodificato: %+v %+v", e, bb)
	}
	// il file che dice il codice nuovo non e' in conflitto («nessun conflitto se la modifica lo elimina»)
	in, id = pbMotoreR4(t, pdfSu(t, "#1"), "k-root", ProvenienzaMotore)
	ricodifica(in, id, "ACM-1001")
	o := in.fatti.Occorrenze[id.pdf]
	o.CodiceLetto, o.RevLetta = "ACM-1001", ""
	in.fatti.Occorrenze[id.pdf] = o
	if e, bb := esitoPBdi(t, in, id); e.Esito != associazioni.EsitoApplicabile || len(bb) != 0 {
		t.Fatalf("PB del motore sul ricodificato con il codice nuovo nel nome: %+v %+v", e, bb)
	}
}

// La PB del motore su un nodo che l'ingegnere ha cambiato: un nodo in modo manuale; il registrato con un componente
// diverso da quello della decisione_esistente della voce (il modo resta registrato); l'identita' proposta cambiata.
func TestR4PBDelMotoreSulNodoCambiato(t *testing.T) {
	// un nodo scritto a mano (manuale, senza casella)
	in, id := pbMotoreR4(t, nil, "k-a", ProvenienzaMotore)
	in.snap.Nodi = append(in.snap.Nodi, Nodo3{ChiaveBozza: "k-a", Op: OpCrea, Tipo: "sciolto", Origine: OrigineUtente,
		Identita: Identita{Emittente: id.clienteE(), EmittentePresunto: true, Codice: "9120001", Modo: ModoManuale}})
	in.snap.Archi = append(in.snap.Archi, Arco3{Padre: "k-root", Figlio: "k-a", Qta: 1, Origine: OrigineUtente})
	if e, bb := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso || len(bb) != 1 {
		t.Fatalf("PB del motore su un nodo in modo manuale: %+v %+v", e, bb)
	}
	// il nodo proposto con un'identita' che non e' quella della voce
	in, id = pbMotoreR4(t, nil, "k-0007", ProvenienzaMotore)
	for i := range in.snap.Nodi {
		if in.snap.Nodi[i].ChiaveBozza == "k-0007" {
			in.snap.Nodi[i].Identita.Codice = "ACM-1199"
		}
	}
	if e, _ := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso {
		t.Fatalf("PB del motore su un nodo proposto con un'altra identita': %+v", e)
	}
	// il registrato: la voce del telaio ha la decisione_esistente sul componente X; il nodo riusa X (intatta) o Y (cambiato)
	x, y := idDi("comp-x"), idDi("comp-y")
	conDE := func(id idProva3, c *importazione.Contenuto) {
		for i := range c.Proposta.Nodi {
			if c.Proposta.Nodi[i].Origine.Rif == id.rif("#7") {
				c.Proposta.Nodi[i].DecisioneEsistente = json.RawMessage(`{"componente_id":"` + x.String() + `","codice":"ACM-1100","rev":"A"}`)
			}
		}
	}
	registrato := func(comp uuid.UUID, codice string) (*ingressiR4, idProva3) {
		in, id := pbMotoreR4(t, conDE, "k-0007", ProvenienzaMotore)
		st := confermabom.NuovoStatoRFQ(id.thread, 7, []confermabom.Componente{{ID: id.prodotto, CodiceLetterale: "ACM-1000", Tipo: "finito"},
			{ID: x, CodiceLetterale: "ACM-1100", Tipo: "sottoassieme"}, {ID: y, CodiceLetterale: "ACM-1150", Tipo: "sottoassieme"}}, nil, nil,
			map[uuid.UUID]confermabom.FonteAutorizzata{})
		in.stato = st
		for i := range in.snap.Nodi {
			if in.snap.Nodi[i].ChiaveBozza == "k-0007" {
				c := comp
				in.snap.Nodi[i].ComponenteID, in.snap.Nodi[i].Op = &c, OpRiusa
				in.snap.Nodi[i].Identita = Identita{Emittente: id.clienteE(), EmittentePresunto: true, Codice: codice, Modo: ModoRegistrato}
			}
		}
		return in, id
	}
	in, id = registrato(x, "ACM-1100")
	if e, bb := esitoPBdi(t, in, id); e.Esito != associazioni.EsitoApplicabile || len(bb) != 0 {
		t.Fatalf("PB del motore sul registrato della voce: %+v %+v", e, bb)
	}
	in, id = registrato(y, "ACM-1150")
	if e, bb := esitoPBdi(t, in, id); e.Motivo != associazioni.MotivoCodiceDiverso || len(bb) != 1 {
		t.Fatalf("PB del motore sul registrato con un altro componente: %+v %+v", e, bb)
	}
}
