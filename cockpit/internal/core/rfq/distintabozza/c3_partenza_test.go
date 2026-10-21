package distintabozza

// Le prove L1 dei costruttori della versione 1 (direttiva 35, blocco C-3, R2 passo 6b; EM §9.2, §6.12): la partenza vuota
// (il solo prodotto), registrato (il perimetro della working, gli archi, le descrizioni e le posizioni oltre il limite con la
// traccia, mai troncate), l'identita' registrata (la certificazione valida, il cliente presunto), da-conferma-1 (le chiavi e
// le caselle della conferma, il nodo escluso e quello fuori dalla working, le decisioni riportate con da_conferma), le chiavi
// deterministiche del server; ogni versione costruita passa la forma del formato 3 (Canonico) e non porta preassociazioni.

import (
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/confermabom"
)

// statoProvaC3: una working di prova (il prodotto con due figli e un nipote, piu' un componente archiviato appeso al prodotto)
// e una conferma formato 3 del prodotto (numero 1) sul contatore corrente.
func statoProvaC3(t *testing.T) (*confermabom.StatoRFQ, map[string]uuid.UUID) {
	t.Helper()
	id := map[string]uuid.UUID{"p": idDi("c3-p"), "a": idDi("c3-a"), "b": idDi("c3-b"), "c": idDi("c3-c"), "x": idDi("c3-x")}
	comp := []confermabom.Componente{
		{ID: id["p"], CodiceLetterale: "ACM-3000", Tipo: "finito", Rev: "b", Descrizione: "Gruppo " + eAccento},
		{ID: id["a"], CodiceLetterale: "ACM-3100", Tipo: "sottoassieme", Rev: "A", Descrizione: strings.Repeat("d", MaxDescrizione+1)},
		{ID: id["b"], CodiceLetterale: "ACM-3200", Tipo: "sciolto"},
		{ID: id["c"], CodiceLetterale: "ACM-3110", Tipo: "commerciale"},
		{ID: id["x"], CodiceLetterale: "ACM-3900", Tipo: "sciolto", Archiviato: true},
	}
	archi := []confermabom.Arco{{Padre: id["p"], Figlio: id["a"], Qta: 1, Posizione: "P1"}, {Padre: id["p"], Figlio: id["b"], Qta: 2,
		Posizione: strings.Repeat("9", MaxPosizione+1)}, {Padre: id["a"], Figlio: id["c"], Qta: 4}, {Padre: id["p"], Figlio: id["x"], Qta: 1}}
	conf := confermabom.TestataConferma{Conferma: confermabom.Conferma{ID: idDi("c3-conferma"), Numero: 1, Contatore: 7, FirmaStruttura: firmaProva},
		Prodotto: id["p"], Formato: 3}
	return confermabom.NuovoStatoRFQ(idDi("rfq"), 7, comp, archi, []confermabom.TestataConferma{conf}, nil), id
}

func TestV1C3LaPartenzaVuotaERegistrata(t *testing.T) {
	st, id := statoProvaC3(t)
	cliente := EmittenteCliente(idDi("cliente"))
	cert := map[uuid.UUID]Certificazione{
		id["p"]: {Emittente: cliente, EmittentePresunto: false, Namespace: "acme", Valida: true},
		id["b"]: {Emittente: "partner:beta", Namespace: "beta", Valida: false}, // superata: vale il cliente presunto
	}
	in := IngressoPartenza{Prodotto: id["p"], Stato: st, Certificazioni: cert, Cliente: cliente}
	v, tr, err := PartenzaRegistrata(PartenzaVuota, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Nodi) != 1 || len(v.Archi) != 0 || v.Nodi[0].ChiaveBozza != ChiaveDelComponente(id["p"]) || *v.Nodi[0].ComponenteID != id["p"] ||
		v.Nodi[0].Op != OpRiusa || v.Nodi[0].Origine != OrigineRegistrato || v.Nodi[0].Tipo != "finito" || v.Nodi[0].Descrizione != nfc("Gruppo "+eAccento) {
		t.Fatalf("vuota: %+v", v.Nodi)
	}
	if got := v.Nodi[0].Identita; got != (Identita{Emittente: cliente, Namespace: "acme", Codice: "ACM-3000", Rev: "b", Modo: ModoRegistrato}) {
		t.Fatalf("identita' certificata del prodotto: %+v", got)
	}
	if v.VersioneBomBase == nil || v.VersioneBomBase.Contatore != 7 || v.VersioneBomBase.Numero != 1 || *v.VersioneBomBase.ConfermaID != idDi("c3-conferma") ||
		v.FirmaBase == "" || len(tr.DescrizioniOltreLimite) != 0 {
		t.Fatalf("base o traccia: %+v %+v", v.VersioneBomBase, tr)
	}
	r, tr, err := PartenzaRegistrata(PartenzaRegistrato, in)
	if err != nil {
		t.Fatal(err)
	}
	// il perimetro: il prodotto, a, b, c; l'archiviato x resta fuori con il suo arco
	if len(r.Nodi) != 4 || len(r.Archi) != 3 {
		t.Fatalf("registrato: %d nodi, %d archi", len(r.Nodi), len(r.Archi))
	}
	for _, n := range r.Nodi {
		if *n.ComponenteID == id["x"] || n.ChiaveBozza != ChiaveDelComponente(*n.ComponenteID) || n.Identita.Modo != ModoRegistrato {
			t.Errorf("nodo %+v", n)
		}
		if *n.ComponenteID == id["b"] && (n.Identita.Emittente != cliente || !n.Identita.EmittentePresunto || n.Identita.Namespace != "") {
			t.Errorf("certificazione superata: %+v", n.Identita)
		}
		if *n.ComponenteID == id["a"] && n.Descrizione != "" {
			t.Errorf("la descrizione oltre il limite si tronca: %q", n.Descrizione)
		}
	}
	if len(tr.DescrizioniOltreLimite) != 1 || tr.DescrizioniOltreLimite[0] != ChiaveDelComponente(id["a"]) || len(tr.PosizioniOltreLimite) != 1 {
		t.Fatalf("traccia: %+v", tr)
	}
	for _, a := range r.Archi {
		if a.Origine != OrigineRegistrato || (a.Figlio == ChiaveDelComponente(id["a"]) && a.Posizione != "P1") ||
			(a.Figlio == ChiaveDelComponente(id["b"]) && a.Posizione != "") {
			t.Errorf("arco %+v", a)
		}
	}
	for _, s := range []Snapshot3{v, r} {
		if _, err := s.Canonico(); err != nil {
			t.Fatalf("la versione 1 non ha la forma del formato 3: %v", err)
		}
		if len(s.DocumentiPreassociati) != 0 || len(s.Decisioni) != 0 || s.Sorgente != nil {
			t.Fatalf("la versione 1 porta preassociazioni, decisioni o una fonte: %+v", s)
		}
	}
}

func TestV1C3LaPartenzaDaConferma(t *testing.T) {
	st, id := statoProvaC3(t)
	cliente := EmittenteCliente(idDi("cliente"))
	in := IngressoPartenza{Prodotto: id["p"], Stato: st, Certificazioni: map[uuid.UUID]Certificazione{}, Cliente: cliente}
	allegato, sha := idDi("allegato"), shaProva('b')
	casella := func(k string) []Casella3 {
		return []Casella3{{AllegatoID: allegato, Gruppo: "bom", Rif: "nodo:" + sha + ":" + k}}
	}
	step := &StepArco3{AllegatoID: allegato, Gruppo: "bom"}
	cs := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: id["p"], Sorgente: &Sorgente3{AllegatoID: &allegato, Sha256: sha, Gruppo: "bom",
		Importazione: strings.Repeat("6", 64)},
		Nodi: []Nodo3{
			{ChiaveBozza: "k-root", ComponenteID: pu(id["p"]), Op: OpRiusa, Tipo: "finito", Origine: "registrato", Caselle: casella("#1"),
				Identita: Identita{Emittente: cliente, EmittentePresunto: true, Codice: "ACM-3000", Modo: ModoRegistrato}},
			{ChiaveBozza: "k-a", Op: OpCrea, Tipo: "sottoassieme", Origine: "step", Caselle: casella("#7"),
				Identita: Identita{Emittente: cliente, Namespace: "acme", Codice: "ACM-3100", Modo: ModoProposta}},
			{ChiaveBozza: "k-b", Op: OpCrea, Tipo: "sciolto", Origine: "utente", Identita: Identita{Emittente: cliente, EmittentePresunto: true,
				Codice: "ACM-3200", Modo: ModoManuale}},
			{ChiaveBozza: "k-c", Op: OpCrea, Tipo: "commerciale", Origine: "utente", Identita: Identita{Emittente: cliente, EmittentePresunto: true,
				Codice: "ACM-3110", Modo: ModoManuale}},
			// escluso dalla conferma: non entra
			{ChiaveBozza: "k-x", ComponenteID: pu(id["x"]), Op: OpEsclude, Tipo: "sciolto", Origine: "registrato",
				Identita: Identita{Emittente: cliente, EmittentePresunto: true, Codice: "ACM-3900", Modo: ModoRegistrato}},
			// un nodo della conferma che la working non ha piu': non entra
			{ChiaveBozza: "k-y", Op: OpCrea, Tipo: "sciolto", Origine: "utente", Identita: Identita{Emittente: cliente, EmittentePresunto: true,
				Codice: "ACM-3999", Modo: ModoManuale}},
		},
		Archi: []Arco3{{Padre: "k-root", Figlio: "k-a", Qta: 1, Origine: "step", Step: step}, {Padre: "k-root", Figlio: "k-b", Qta: 2, Origine: "utente"},
			{Padre: "k-a", Figlio: "k-c", Qta: 4, Origine: "utente"}, {Padre: "k-root", Figlio: "k-y", Qta: 1, Origine: "utente"}},
		Decisioni: []Decisione3{
			{ID: "mp-1", Tipo: DecisioneMonoPezzo, Motivo: "pezzo unico"},
			{ID: "cc-1", Tipo: DecisioneConfermaCategoria, Soggetto: &SoggettoCategoria{ChiaveBozza: "k-a"}, Categoria: "fabbricato", Esito: EsitoCategoriaConferma, Motivo: "dal disegno",
				OrigineCategoria: OrigineCategoriaManuale, Ingresso: IngressoEditor},
			// gia' riportata da una conferma precedente: resta della conferma che l'ha introdotta
			{ID: "cc-2", Tipo: DecisioneConfermaCategoria, Soggetto: &SoggettoCategoria{ChiaveBozza: "k-c"}, Categoria: "commerciale",
				Esito: EsitoCategoriaConferma, Motivo: "dal disegno", OrigineCategoria: OrigineCategoriaManuale, Ingresso: IngressoEditor,
				DaConferma: &DaConferma{ConfermaID: idDi("c3-conferma-0"), Numero: 1, DecisioneID: "cc-0"}},
			{ID: "cc-3", Tipo: DecisioneConfermaCategoria, Soggetto: &SoggettoCategoria{ChiaveBozza: "k-y"}, Categoria: "fabbricato", Esito: EsitoCategoriaConferma, Motivo: "dal disegno",
				OrigineCategoria: OrigineCategoriaManuale, Ingresso: IngressoEditor},
			{ID: "dv-1", Tipo: DecisioneDaValutare, ChiaveBozza: "k-a"},
		},
		DocumentiPreassociati: []Preassociazione3{{AllegatoID: idDi("pdf"), ChiaveBozza: "k-a"}},
	}
	cp := ConfermaDiPartenza{ConfermaID: idDi("c3-conferma"), Numero: 2, Formato: 3, Snapshot: &cs,
		Mappa:    map[string]uuid.UUID{"k-root": id["p"], "k-a": id["a"], "k-b": id["b"], "k-c": id["c"], "k-x": id["x"], "k-y": uuid.Nil},
		Sorgente: &Sorgente3{AllegatoID: &allegato, Sha256: sha, Gruppo: "bom"}}
	v, tr, err := PartenzaDaConferma(cp, in)
	if err != nil {
		t.Fatal(err)
	}
	if v.Sorgente == nil || v.Sorgente.Importazione != "" || *v.Sorgente.AllegatoID != allegato || v.Sorgente.Gruppo != "bom" {
		t.Fatalf("la sorgente della conferma, senza registro: %+v", v.Sorgente)
	}
	chiavi := []string{}
	for _, n := range v.Nodi {
		chiavi = append(chiavi, n.ChiaveBozza)
		if n.Op != OpRiusa || n.ComponenteID == nil || n.Identita.Modo != ModoRegistrato || n.Identita.EmittentePresunto != true {
			t.Errorf("nodo %+v", n)
		}
	}
	sort.Strings(chiavi)
	if strings.Join(chiavi, ",") != "k-a,k-b,k-c,k-root" {
		t.Fatalf("le chiavi della conferma: %v", chiavi)
	}
	if strings.Join(tr.NodiFuoriWorking, ",") != "k-x,k-y" {
		t.Fatalf("nodi fuori: %+v", tr)
	}
	perChiave := map[string]Nodo3{}
	for _, n := range v.Nodi {
		perChiave[n.ChiaveBozza] = n
	}
	if len(perChiave["k-a"].Caselle) != 1 || perChiave["k-a"].Origine != OrigineStep || perChiave["k-root"].Origine != OrigineRegistrato ||
		perChiave["k-b"].Origine != OrigineConferma || perChiave["k-a"].Identita.Codice != "ACM-3100" || perChiave["k-root"].Identita.Rev != "b" {
		t.Fatalf("nodi: %+v", perChiave)
	}
	if len(v.Archi) != 3 || v.Archi[0].Step == nil {
		t.Fatalf("archi: %+v", v.Archi)
	}
	ids := []string{}
	for _, d := range v.Decisioni {
		ids = append(ids, d.ID)
		switch d.ID {
		case "mp-1":
			if d.DaConferma != nil {
				t.Errorf("mono_pezzo con da_conferma: %+v", d)
			}
		case "cc-1":
			if d.DaConferma == nil || *d.DaConferma != (DaConferma{ConfermaID: idDi("c3-conferma"), Numero: 2, DecisioneID: "cc-1"}) {
				t.Errorf("cc-1: %+v", d.DaConferma)
			}
		case "cc-2":
			if d.DaConferma == nil || d.DaConferma.DecisioneID != "cc-0" {
				t.Errorf("cc-2: %+v", d.DaConferma)
			}
		}
	}
	sort.Strings(ids)
	if strings.Join(ids, ",") != "cc-1,cc-2,mp-1" {
		t.Fatalf("decisioni: %v", ids)
	}
	if len(v.DocumentiPreassociati) != 0 {
		t.Fatal("una preassociazione passa nella bozza nuova")
	}
	if _, err := v.Canonico(); err != nil {
		t.Fatalf("la versione 1 non ha la forma del formato 3: %v", err)
	}
	// una conferma formato 1 (senza snapshot di nodi) parte come registrato
	r, _, err := PartenzaDaConferma(ConfermaDiPartenza{ConfermaID: idDi("c3-conferma"), Numero: 2, Formato: 1}, in)
	if err != nil || len(r.Nodi) != 4 || r.Sorgente != nil {
		t.Fatalf("conferma formato 1: %v %+v", err, r.Nodi)
	}
}

func TestV1C3LeChiaviDelServer(t *testing.T) {
	a, b := idDi("allegato"), idDi("allegato2")
	k := ChiaveDellaVoce(a, "nodo:"+shaProva('b')+":#7")
	if k != ChiaveDellaVoce(a, "nodo:"+shaProva('b')+":#7") || k == ChiaveDellaVoce(b, "nodo:"+shaProva('b')+":#7") ||
		k == ChiaveDellaVoce(a, "nodo:"+shaProva('b')+":#9") || !reChiave.MatchString(k) || !strings.HasPrefix(k, "k-v-") {
		t.Fatalf("chiave della voce: %s", k)
	}
	if c := ChiaveDelComponente(idDi("c3-p")); !reChiave.MatchString(c) || !strings.HasPrefix(c, "k-c-") {
		t.Fatalf("chiave del componente: %s", c)
	}
}
