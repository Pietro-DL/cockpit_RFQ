// L1 — l'effetto di un cambio della fonte (V1: emendamento di V1 §6.6, cambio-fonte-1, e §10.9 S6, effetto_sulla_bozza;
// la parte in sola lettura di R-05b e RA-08 (d1) di PM23 §10, riga 3), sui registri veri dell'adattatore: una bozza
// formato 3 costruita sul registro della BOM di lavoro e i registri di un'altra occorrenza dello stesso STEP e di un
// altro STEP. La bozza è una VistaBozza scritta qui, con le chiavi bozza inventate. Tutto è inventato, come nelle scene.
package valutazione_test

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

var (
	aStepAltro   = uid(0xe8a3) // un altro STEP del prodotto, con un'altra struttura
	shaStepAltro = sha("f")
	pVassoio     = uid(0xe8a4) // la preassociazione di un nodo della bozza
)

// scenaCambioFonte: la scena del legame con la copia dello stesso STEP (fuori dal perimetro) e un altro STEP del
// prodotto (radice 7120100A, i figli #2 e #9), con il calcolo e i registri dei tre gruppi.
func scenaCambioFonte(t *testing.T) (bom, copia, altro importazione.Contenuto) {
	t.Helper()
	_, hash := insiemeConLegami(t, false)
	r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
	th := scenaLegame(t)
	conAllegato(&th, allegato(aStepGemello, 6, "copia-assieme.stp", "stp", shaStepB), nil)
	fa := fattiStepF(t, shaStepAltro, []nodoF{{"#1", "7120100A", ""}, {"#2", "654302", ""}, {"#9", "654309", ""}}, []arcoF{{"#1", "#2", 1}, {"#1", "#9", 1}})
	conAllegato(&th, allegato(aStepAltro, 7, "altro-assieme.stp", "stp", shaStepAltro), &fa)
	c := calcolaPerIlRegistro(t, th, r)
	pv := prodottoDi(t, c.et, rifProdB)
	chiavi := map[uuid.UUID]string{}
	for _, g := range propostabom.GruppiDelProdotto(pv, c.et.Ancoraggi.Strutture) {
		if g.Tipo == propostabom.GruppoCandidata {
			chiavi[*g.AllegatoID] = g.Chiave
		}
	}
	if chiavi[aStepGemello] == "" || chiavi[aStepAltro] == "" {
		t.Fatalf("i gruppi candidati: %v", chiavi)
	}
	return c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto, c.importa(t, rifProdB, chiavi[aStepGemello], nil).Contenuto,
		c.importa(t, rifProdB, chiavi[aStepAltro], nil).Contenuto
}

// bozzaSulRegistro: la bozza formato 3 nata dalla BOM di lavoro: il nodo del prodotto k1 (registrato), k2 «proposta»
// con l'identità proposta di #2, k3 con una decisione identita (d1) che cita la lettura dell'identità proposta di #3, una
// non_includere (d2) sulla casella di #3, la preassociazione di k3; gli archi «step» k1→k2 e k1→k3.
func bozzaSulRegistro(t *testing.T, bom importazione.Contenuto) propostabom.VistaBozza {
	t.Helper()
	casella := func(chiave string) []propostabom.CasellaBozza {
		return []propostabom.CasellaBozza{{AllegatoID: aStepB, Gruppo: propostabom.GruppoBOM, Rif: nodoB(chiave)}}
	}
	v2, v3 := voceDelRegistro(t, bom, nodoB("#2")), voceDelRegistro(t, bom, nodoB("#3"))
	l3 := letturaDa(t, v3.IdentitaProposta)
	lr, ok := bom.TrovaLettura(l3.ChiaveLettura)
	if !ok {
		t.Fatalf("la lettura di #3 non si trova nel registro: %s", l3.ChiaveLettura)
	}
	cp, err := lr.Compatta()
	if err != nil {
		t.Fatal(err)
	}
	passo := &propostabom.PassoStep{AllegatoID: aStepB, Gruppo: propostabom.GruppoBOM}
	c3 := casella("#3")[0]
	return propostabom.VistaBozza{
		Sorgente:     propostabom.SorgenteBozza{AllegatoID: aStepB, Sha256: shaStepB, Radice: nodoB("#1"), Gruppo: propostabom.GruppoBOM},
		NodoProdotto: "k1",
		Nodi: []propostabom.NodoBozza{
			{ChiaveBozza: "k1", Caselle: casella("#1"), Identita: propostabom.IdentitaBozza{Modo: propostabom.ModoRegistrato, Codice: "7120100A"}},
			{ChiaveBozza: "k2", Caselle: casella("#2"), Identita: propostabom.IdentitaBozza{Modo: propostabom.ModoProposta, Emittente: "partner:beta",
				Namespace: "beta-dis", Codice: v2.CodiceProposto}},
			{ChiaveBozza: "k3", Caselle: casella("#3"), Identita: propostabom.IdentitaBozza{Modo: propostabom.ModoDecisione, Decisione: "d1", Codice: "654303"}},
		},
		Archi: []propostabom.ArcoBozza{{Padre: "k1", Figlio: "k2", Origine: "step", Step: passo}, {Padre: "k1", Figlio: "k3", Origine: "step", Step: passo}},
		Decisioni: []propostabom.DecisioneBozza{
			{ID: "d1", Tipo: propostabom.TipoDecisioneIdentita, CitaNodi: []string{"k3"}, ChiaveLettura: l3.ChiaveLettura, Lettura: &cp},
			{ID: "d2", Tipo: propostabom.TipoDecisioneNonIncludere, Casella: &c3},
		},
		Preassociazioni: []propostabom.PreassociazioneBozza{{PropostaID: pVassoio, ChiaveBozza: "k3"}},
	}
}

func daRivedereCon(e propostabom.EffettoCambioFonte, chiave, decisione, motivo string) bool {
	for _, x := range e.DaRivedere {
		if x.ChiaveBozza == chiave && x.DecisioneID == decisione && x.Motivo == motivo {
			return true
		}
	}
	return false
}

// TestCambioFonteLaStessaStruttura (cambio-fonte-1 passo 1; OV-05): verso lo stesso gruppo della stessa struttura sulla
// stessa occorrenza il gesto è un riallineamento: solo StessaSorgente, nessun elenco.
func TestCambioFonteLaStessaStruttura(t *testing.T) {
	bom, _, _ := scenaCambioFonte(t)
	e := propostabom.EffettoDelCambioFonte(bozzaSulRegistro(t, bom), &bom, bom)
	if !e.StessaSorgente || len(e.NodiCheRestano) != 0 || len(e.CaselleRiscritte) != 0 || len(e.DaRivedere) != 0 {
		t.Errorf("la stessa sorgente: %+v", e)
	}
}

// TestCambioFonteFraDueOccorrenze (R-05b, la parte in sola lettura; RA-08 (d1); EM §6.6 passi 2-7): verso la copia dello
// stesso STEP (un altro allegato, lo stesso sha, fuori dal perimetro) nessun nodo esce, le caselle e gli archi «step» si
// riscrivono verso l'altra occorrenza ed entrano negli elenchi, niente si eredita in silenzio: il nodo «proposta» la cui
// identità è diversa nell'altra occorrenza (lì BETA non legge) è fra le identità aggiornate, il nodo deciso e la
// non_includere vanno da rivedere per altra_occorrenza, la decisione identita anche per la lettura che il registro nuovo
// non ha; nessuna voce entra da sola.
func TestCambioFonteFraDueOccorrenze(t *testing.T) {
	bom, copia, _ := scenaCambioFonte(t)
	e := propostabom.EffettoDelCambioFonte(bozzaSulRegistro(t, bom), &bom, copia)
	if e.StessaSorgente || len(e.FuoriPerCambioFonte) != 0 || len(e.NodiCheEntrano) != 0 || len(e.NuoviNodiProposti) != 0 || len(e.DecisioniCadute) != 0 ||
		len(e.PreassociazioniNelVassoio) != 0 {
		t.Fatalf("fra due occorrenze: %+v", e)
	}
	if len(e.NodiCheRestano) != 3 {
		t.Errorf("nodi che restano: %v", e.NodiCheRestano)
	}
	riscritte := map[string]propostabom.CasellaRiscritta{}
	for _, c := range e.CaselleRiscritte {
		riscritte[c.ChiaveBozza+"/"+c.DecisioneID] = c
	}
	for _, k := range []string{"k1/", "k2/", "k3/", "/d2"} {
		if c, ok := riscritte[k]; !ok || c.Prima != aStepB || c.Dopo != aStepGemello {
			t.Errorf("la casella %s non è riscritta verso la copia: %+v", k, c)
		}
	}
	if len(e.ArchiRiscritti) != 2 || e.ArchiRiscritti[0].Dopo.AllegatoID != aStepGemello || e.ArchiRiscritti[0].Dopo.Gruppo != copia.Gruppo.Chiave {
		t.Errorf("gli archi riscritti: %+v", e.ArchiRiscritti)
	}
	if len(e.IdentitaAggiornate) != 1 || e.IdentitaAggiornate[0].ChiaveBozza != "k2" || e.IdentitaAggiornate[0].Dopo.Modo != propostabom.ModoDaDecidere {
		t.Errorf("le identità aggiornate: %+v", e.IdentitaAggiornate)
	}
	if !daRivedereCon(e, "k3", "", propostabom.MotivoRivedereAltraOccorrenza) || !daRivedereCon(e, "", "d2", propostabom.MotivoRivedereAltraOccorrenza) ||
		!daRivedereCon(e, "k3", "", propostabom.MotivoRivedereLetturaAssente) {
		t.Errorf("da rivedere: %+v", e.DaRivedere)
	}
	if daRivedereCon(e, "k2", "", propostabom.MotivoRivedereAltraOccorrenza) {
		t.Errorf("un nodo «proposta» non va da rivedere per l'occorrenza: %+v", e.DaRivedere)
	}
	if !e.Differenze.Vuote() {
		t.Errorf("fra due occorrenze della stessa struttura nessun arco nuovo: %+v", e.Differenze)
	}
}

// TestCambioFonteVersoUnAltroSTEP (EM §6.6 passi 2-7, con un'altra struttura): i nodi della bozza sulle caselle dello STEP
// di prima escono (fuori_per_cambio_fonte), le decisioni che li citano cadono (anche la non_includere sulla casella che
// non c'è più), la preassociazione va nel vassoio; il nodo del prodotto resta, con la casella sulla radice nuova; le voci
// nuove (salvo la radice) entrano.
func TestCambioFonteVersoUnAltroSTEP(t *testing.T) {
	bom, _, altro := scenaCambioFonte(t)
	e := propostabom.EffettoDelCambioFonte(bozzaSulRegistro(t, bom), &bom, altro)
	if e.StessaSorgente || len(e.NodiCheRestano) != 1 || e.NodiCheRestano[0] != "k1" {
		t.Fatalf("nodi che restano: %+v", e)
	}
	if len(e.FuoriPerCambioFonte) != 2 || e.FuoriPerCambioFonte[0] != "k2" || e.FuoriPerCambioFonte[1] != "k3" {
		t.Errorf("fuori per cambio fonte: %v", e.FuoriPerCambioFonte)
	}
	if len(e.DecisioniCadute) != 2 || e.DecisioniCadute[0] != "d1" || e.DecisioniCadute[1] != "d2" {
		t.Errorf("decisioni cadute: %v", e.DecisioniCadute)
	}
	if len(e.PreassociazioniNelVassoio) != 1 || e.PreassociazioniNelVassoio[0] != pVassoio.String() {
		t.Errorf("il vassoio: %v", e.PreassociazioniNelVassoio)
	}
	if len(e.NodiCheEntrano) != 2 {
		t.Errorf("le voci che entrano: %v", e.NodiCheEntrano)
	}
	if len(e.CaselleRiscritte) != 1 || e.CaselleRiscritte[0].ChiaveBozza != "k1" || e.CaselleRiscritte[0].Rif != ancoraggio.RifNodo(shaStepAltro, "#1") ||
		e.CaselleRiscritte[0].Dopo != aStepAltro {
		t.Errorf("la casella del prodotto: %+v", e.CaselleRiscritte)
	}
	if len(e.ArchiRiscritti) != 0 {
		t.Errorf("nessun arco «step» resta: %+v", e.ArchiRiscritti)
	}
}
