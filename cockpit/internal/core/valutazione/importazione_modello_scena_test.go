// L1 — le aggiunte di V1 al modello della pagina e il contenuto di S6, sulle scene della valutazione (V1: emendamento di
// V1 §9.9 e §10.9 S6; PE-01, PE-10, PE-15, PE-16, PE-28 accolte nelle decisioni dell'ondata A; prove A-26, A-27 e A-28 di
// PM23 §10, la parte che si calcola dall'esito: i nomi JSON li controlla la prova di motore_a). propostabom.Modello e
// propostabom.AnteprimaDelloSTEP usano lo stesso adattatore di R2: l'impronta_contenuto di un gruppo è quella di
// ImportaGruppo sullo stesso esito. Tutto è inventato, come nelle scene.
package valutazione_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

// modelloDi: il modello del prodotto della scena, con la working e le certificazioni date.
func modelloDi(t *testing.T, c calcoloPerIlRegistro, componenti map[uuid.UUID]propostabom.ComponenteDellaWorking,
	cert map[uuid.UUID]propostabom.Certificazione) *propostabom.ModelloDelProdotto {
	t.Helper()
	e, et := c.e, c.et
	m := propostabom.Modello(propostabom.IngressoModello{Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, Componenti: componenti,
		Certificazioni: cert})
	if m == nil {
		t.Fatal("il modello del prodotto non c'è")
	}
	return m
}

func gruppoDelModello(t *testing.T, m *propostabom.ModelloDelProdotto, chiave string) propostabom.GruppoDelModello {
	t.Helper()
	for _, g := range m.Gruppi {
		if g.Chiave == chiave {
			return g
		}
	}
	t.Fatalf("il modello non ha il gruppo %s: %+v", chiave, m.Gruppi)
	return propostabom.GruppoDelModello{}
}

func nodoDelModello(t *testing.T, g propostabom.GruppoDelModello, rif string) propostabom.NodoDelModello {
	t.Helper()
	for _, n := range g.Nodi {
		if n.Rif == rif {
			return n
		}
	}
	t.Fatalf("il gruppo %s non ha il nodo %s", g.Chiave, rif)
	return propostabom.NodoDelModello{}
}

// TestA28IGruppiDelModello (A-28, la parte dei valori; PE-01; EM §9.9): il gruppo «bom» della scena del collegamento con
// la chiave dal core, l'allegato, la radice, il nome del file, l'impronta_contenuto di ImportaGruppo sullo stesso esito,
// il letterale della radice, la compatibilità, la struttura e i nodi segnalati; i cordoni contano fra i segnalati.
func TestA28IGruppiDelModello(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	m := modelloDi(t, c, nil, nil)
	g := gruppoDelModello(t, m, propostabom.GruppoBOM)
	reg := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	if g.Tipo != propostabom.GruppoBOM || g.AllegatoID == nil || *g.AllegatoID != aStepB || g.Radice != nodoB("#1") || g.NomeFile != "7120100A_1.stp" ||
		g.ImprontaContenuto != reg.Contenuto.ImprontaContenuto || g.RadiceLetterale != "7120100A" || g.Compatibilita != propostabom.CompatibilitaCompatibile ||
		g.Struttura == nil || !g.Struttura.Completa || g.Segnalati != 0 || g.Importazione != nil {
		t.Fatalf("il gruppo bom: %+v (struttura %+v)", g, g.Struttura)
	}
	if len(g.Nodi) != len(reg.Contenuto.Proposta.Nodi) || m.MotivoSenzaGruppi != nil {
		t.Errorf("nodi %d (voci %d), motivo senza gruppi %+v", len(g.Nodi), len(reg.Contenuto.Proposta.Nodi), m.MotivoSenzaGruppi)
	}
	// i cordoni segnalati
	cc := calcolaPerIlRegistro(t, scenaCordoni(t, "Saldatura"), insiemeACME(t))
	if g := gruppoDelModello(t, modelloDi(t, cc, nil, nil), propostabom.GruppoBOM); g.Segnalati != 3 {
		t.Errorf("i nodi segnalati: %d, attesi i tre cordoni", g.Segnalati)
	}
}

// TestA28IlProdottoSenzaGruppi (A-28, PE-16; MAT SG-13): un prodotto senza uno STEP fra i file non ha gruppi con uno STEP
// (resta al più il gruppo «senza» dei confermati senza STEP) e ha motivo_senza_gruppi con il codice della fonte e un
// testo; con uno STEP il motivo non c'è.
func TestA28IlProdottoSenzaGruppi(t *testing.T) {
	th := threadBase("Buongiorno, vi chiediamo l'offerta per 7120100A. Grazie")
	th.Componenti = []fotorfq.Componente{componente(cProdotto, "7120100A", "finito", "manuale")}
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	m := modelloDi(t, c, nil, nil)
	for _, g := range m.Gruppi {
		if g.Tipo != propostabom.GruppoSenzaSTEP || g.AllegatoID != nil {
			t.Errorf("un gruppo con uno STEP senza STEP: %+v", g)
		}
	}
	if m.MotivoSenzaGruppi == nil || m.MotivoSenzaGruppi.Codice != string(valutazione.MotivoFonteNessunaFonte) || m.MotivoSenzaGruppi.Testo == "" {
		t.Fatalf("il motivo senza gruppi: %+v", m.MotivoSenzaGruppi)
	}
	b, _ := json.Marshal(m)
	if !strings.Contains(string(b), `"motivo_senza_gruppi":{"codice":`) {
		t.Errorf("il nome del motivo: %s", b)
	}
}

// TestA26LIdentitaRegistrata (A-26; PE-10; EM §5.1, §9.9): per il nodo #2, deciso con il componente sciolto, il modello dà
// l'identità registrata: con la certificazione valida l'emittente, la presunzione e il namespace della certificazione;
// con una certificazione superata (il codice di oggi non è quello certificato, oppure il componente è archiviato) il
// cliente presunto, senza namespace, con la certificazione superata; senza certificazione il cliente presunto. Codice e
// rev sono quelli della working (la rev vuota assente).
func TestA26LIdentitaRegistrata(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	working := map[uuid.UUID]propostabom.ComponenteDellaWorking{cSciolto: {Codice: "7120200A", Rev: "1"}}
	cliente := "cliente:" + clienteACME.String()
	identita := func(t *testing.T, w map[uuid.UUID]propostabom.ComponenteDellaWorking, cert map[uuid.UUID]propostabom.Certificazione) *propostabom.DecisioneEsistenteDelModello {
		t.Helper()
		n := nodoDelModello(t, gruppoDelModello(t, modelloDi(t, c, w, cert), propostabom.GruppoBOM), nodoB("#2"))
		if n.DecisioneEsistente == nil || n.DecisioneEsistente.ComponenteID != cSciolto {
			t.Fatalf("#2 senza decisione esistente: %+v", n)
		}
		return n.DecisioneEsistente
	}
	valida := map[uuid.UUID]propostabom.Certificazione{cSciolto: {Codice: "7120200A", Emittente: cliente, Namespace: "acme-catena"}}
	d := identita(t, working, valida)
	if d.IdentitaRegistrata != (propostabom.IdentitaRegistrata{Emittente: cliente, Namespace: "acme-catena", Codice: "7120200A", Rev: "1",
		Modo: propostabom.ModoRegistrato}) || d.CertificazioneSuperata {
		t.Errorf("con la certificazione valida: %+v", d)
	}
	superata := map[uuid.UUID]propostabom.Certificazione{cSciolto: {Codice: "7120299A", Emittente: "partner:beta", Namespace: "beta-dis"}}
	d = identita(t, working, superata)
	if d.IdentitaRegistrata != (propostabom.IdentitaRegistrata{Emittente: cliente, EmittentePresunto: true, Codice: "7120200A", Rev: "1",
		Modo: propostabom.ModoRegistrato}) || !d.CertificazioneSuperata {
		t.Errorf("con la certificazione superata: %+v", d)
	}
	archiviato := map[uuid.UUID]propostabom.ComponenteDellaWorking{cSciolto: {Codice: "7120200A", Rev: "1", Archiviato: true}}
	if d = identita(t, archiviato, valida); !d.CertificazioneSuperata || d.IdentitaRegistrata.Namespace != "" || !d.IdentitaRegistrata.EmittentePresunto {
		t.Errorf("il componente archiviato: %+v", d)
	}
	d = identita(t, map[uuid.UUID]propostabom.ComponenteDellaWorking{cSciolto: {Codice: "7120200A"}}, nil)
	if d.IdentitaRegistrata.Emittente != cliente || !d.IdentitaRegistrata.EmittentePresunto || d.CertificazioneSuperata || d.IdentitaRegistrata.Rev != "" {
		t.Errorf("senza certificazione e senza rev: %+v", d)
	}
	if b, _ := json.Marshal(d.IdentitaRegistrata); strings.Contains(string(b), `"rev"`) {
		t.Errorf("la rev vuota non è assente: %s", b)
	}
	// un componente che la working non dà: niente identità registrata (non si inventa)
	n := nodoDelModello(t, gruppoDelModello(t, modelloDi(t, c, nil, nil), propostabom.GruppoBOM), nodoB("#2"))
	if n.DecisioneEsistente != nil {
		t.Errorf("senza la working l'identità registrata non si inventa: %+v", n.DecisioneEsistente)
	}
}

// TestA27LaCompletezzaEDiagnosticheDelNodo (A-27; PE-28; EM §9.9; MAT PN-06, PN-07): per ogni nodo del gruppo la
// completezza del 2D è quella che la valutazione calcola sulla stessa fotografia (la voce del 2D del componente deciso, o
// il fabbisogno previsto del nodo senza componente), e le diagnostiche correnti sono quelle della voce del registro di
// adesso; un 2D caricato per il componente cambia la completezza del nodo.
func TestA27LaCompletezzaEDiagnosticheDelNodo(t *testing.T) {
	th := scenaCollegamento(t)
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	g := gruppoDelModello(t, modelloDi(t, c, nil, nil), propostabom.GruppoBOM)
	pv := prodottoDi(t, c.et, rifProdB)
	reg := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	for _, n := range g.Nodi {
		v := voceDelRegistro(t, reg, n.Rif)
		if string(n.Diagnostiche) != string(v.Diagnostiche) {
			t.Errorf("%s: le diagnostiche correnti non sono quelle della voce: %s / %s", n.Rif, n.Diagnostiche, v.Diagnostiche)
		}
	}
	// #2 ha il componente sciolto: la voce del 2D del componente
	n2 := nodoDelModello(t, g, nodoB("#2"))
	var attesa *valutazione.VoceFabbisogno
	for i := range pv.Documenti.Voci {
		if x := &pv.Documenti.Voci[i]; x.ComponenteID == cSciolto && x.TipoDocumento == propostabom.RequisitoDisegno2D {
			attesa = x
		}
	}
	if attesa == nil {
		t.Fatal("la scena non ha la voce del 2D del componente sciolto")
	}
	if n2.Completezza == nil || n2.Completezza.Requisito != propostabom.RequisitoDisegno2D || n2.Completezza.Esito != string(attesa.Esito) {
		t.Errorf("la completezza di #2: %+v, attesa %s", n2.Completezza, attesa.Esito)
	}
	// #3 e #4 senza componente: il fabbisogno previsto, se la valutazione lo prevede
	for _, ch := range []string{"#3", "#4"} {
		n := nodoDelModello(t, g, nodoB(ch))
		previsto := false
		for _, p := range pv.Documenti.Previsti {
			previsto = previsto || (p.Nodo == nodoB(ch) && p.TipoDocumento == propostabom.RequisitoDisegno2D)
		}
		switch {
		case previsto && (n.Completezza == nil || n.Completezza.Esito != propostabom.EsitoRequisitoPrevisto):
			t.Errorf("%s: il 2D previsto non c'è: %+v", ch, n.Completezza)
		case !previsto && n.Completezza != nil:
			t.Errorf("%s: una completezza senza un requisito: %+v", ch, n.Completezza)
		}
	}
	// senza il 2D del componente sciolto la completezza di #2 è un'altra
	senza := scenaCollegamento(t)
	var docs []fotorfq.DocumentoConfermato
	for _, d := range senza.Documenti {
		if d.ID != dDisegno {
			docs = append(docs, d)
		}
	}
	senza.Documenti = docs
	c2 := calcolaPerIlRegistro(t, senza, insiemeACME(t))
	n2b := nodoDelModello(t, gruppoDelModello(t, modelloDi(t, c2, nil, nil), propostabom.GruppoBOM), nodoB("#2"))
	if n2b.Completezza == nil || n2b.Completezza.Esito == n2.Completezza.Esito {
		t.Errorf("il 2D del componente non cambia la completezza del nodo: %+v / %+v", n2b.Completezza, n2.Completezza)
	}
}

// TestS6LAnteprimaDelloSTEP (EM §10.9 S6, il contenuto che si calcola dall'esito; §6.6 «L'anteprima dell'effetto»): con la
// fonte non confermata lo STEP ha la candidata compatibile del prodotto, in vista, con l'impronta_contenuto di
// ImportaGruppo; la proposta è quella della candidata, con il confronto del codice della radice e le associazioni
// previste; con una bozza sulla stessa struttura e la stessa occorrenza l'effetto è un riallineamento (StessaSorgente).
// Una struttura letta da un worker non aggiornato non si mostra: struttura_non_determinabile, con l'avviso.
func TestS6LAnteprimaDelloSTEP(t *testing.T) {
	th := scenaCollegamento(t)
	senzaFonte(&th)
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	e, et := c.e, c.et
	a := propostabom.AnteprimaDelloSTEP(propostabom.IngressoAnteprima{Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, Allegato: aStepB})
	if a.Stato != propostabom.StatoAnteprimaPronta || a.NomeFile != "7120100A_1.stp" || a.Sha256 != shaStepB || a.CodiceProdotto != "7120100A" ||
		len(a.Gruppi) != 1 || !a.Gruppi[0].InVista || a.Gruppi[0].Compatibilita != propostabom.CompatibilitaCompatibile {
		t.Fatalf("l'anteprima: %+v", a)
	}
	reg := c.importa(t, rifProdB, a.Gruppi[0].Gruppo, nil)
	if a.Gruppi[0].ImprontaContenuto != reg.Contenuto.ImprontaContenuto || a.Gruppi[0].Nodi != len(reg.Contenuto.Proposta.Nodi) ||
		a.Proposta == nil || a.Proposta.Gruppo != a.Gruppi[0].Gruppo || string(a.Proposta.Contenuto) != string(reg.Canonico) {
		t.Fatalf("la proposta della candidata in vista: %+v", a.Proposta)
	}
	if a.Confronto != propostabom.ConfrontoCodiceCoincide || a.Gruppi[0].Radice == nil || len(a.Avvisi) != 0 || a.EffettoSullaBozza != nil {
		t.Errorf("confronto %s, radice %+v, avvisi %+v", a.Confronto, a.Gruppi[0].Radice, a.Avvisi)
	}
	if b, _ := json.Marshal(a); !strings.Contains(string(b), `"associazioni_previste":[`) || !strings.Contains(string(b), `"avvisi":[]`) {
		t.Errorf("gli elenchi di S6 non sono elenchi: %s", b)
	}
	// con una bozza sulla stessa struttura e la stessa occorrenza: un riallineamento
	vb := propostabom.VistaBozza{Sorgente: propostabom.SorgenteBozza{AllegatoID: aStepB, Sha256: shaStepB, Radice: nodoB("#1"), Gruppo: a.Gruppi[0].Gruppo},
		NodoProdotto: "k1"}
	k := reg.Contenuto
	b := propostabom.AnteprimaDelloSTEP(propostabom.IngressoAnteprima{Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, Allegato: aStepB,
		Bozza: &vb, RegistroBozza: &k})
	if b.EffettoSullaBozza == nil || !b.EffettoSullaBozza.StessaSorgente {
		t.Errorf("l'effetto sulla bozza della stessa struttura: %+v", b.EffettoSullaBozza)
	}
	// LS-01: la struttura da rileggere
	f := th.Fatti[shaStepB]
	f.MotivoParziale = testo("struttura v4: gli scarti non sono numerati, serve la rianalisi v3")
	th.Fatti[shaStepB] = f
	c3 := calcolaPerIlRegistro(t, th, insiemeACME(t))
	e3, et3 := c3.e, c3.et
	x := propostabom.AnteprimaDelloSTEP(propostabom.IngressoAnteprima{Esito: &e3, Thread: &et3, Regole: c3.reg, Prodotto: rifProdB, Allegato: aStepB})
	if x.Stato != propostabom.StatoAnteprimaStrutturaNonDeterminabile || x.Proposta != nil || len(x.Avvisi) != 1 ||
		x.Avvisi[0].Codice != propostabom.CodiceImportazioneGrafoDaRileggere {
		t.Errorf("la struttura da rileggere: %+v", x)
	}
}
