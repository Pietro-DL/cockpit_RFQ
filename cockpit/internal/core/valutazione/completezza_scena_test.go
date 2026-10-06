// L1 — la completezza documentale dalla fotografia (B5, fase 3; l'adattatore: R62 d C, R62 e A, R82, R99 A, R102 A,
// R103 C; T-B0-07, T-12; T-B5-90…93; PO-28, PO-33 e PO-36 sulla fotografia costruita in Go): il perimetro dalla verifica
// della BOM, le righe certe di qualunque origine, fuori dal perimetro e archiviate no, i fabbisogni dalla vista e dalle
// regole del cliente, il commerciale, gli esiti della vista, la voce del 2D (la deroga con la sua diagnostica, «assegna»,
// lo scarto, il da_determinare, il DWG, il PDF che non si apre), i previsti dei nodi con il tipo proposto e i loro 2D
// candidati, il nodo deciso fuori dal perimetro, la classificazione con la famiglia della minuteria, le sezioni assenti,
// il target senza componente, il determinismo.
package valutazione_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// sceneCompletezza: la scena dell'albero (fonte confermata, BOM di lavoro, «Conferma l'albero» sul figlio) con i
// fabbisogni di default, e i 2D validi del finito e dello sciolto se con2D.
func scenaCompletezza(t *testing.T, con2D bool) fotorfq.Thread {
	t.Helper()
	th := scenaAlbero(t)
	th.Fabbisogni = fabbisogniDefault()
	if con2D {
		conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "assieme-acme.pdf", sha1, scansione(t, sha1))
		conDocumento2D(&th, dPDF2, cSciolto, aPDF2, "disegno-acme.pdf", sha2, scansione(t, sha2))
	}
	return th
}

// scenaTreNodi: la scena con un terzo nodo #3 (7120300A, figlio della radice) e, sotto, #4 (7120310A): «Conferma
// l'albero» solo su #2; le righe di #3 e #4 restano aperte (proposte aperte).
func scenaTreNodi(t *testing.T, con2D bool) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120310A", ""}},
		[]arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}, {"#3", "#4", 1}})
	confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
	th.Fabbisogni = fabbisogniDefault()
	if con2D {
		conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "assieme-acme.pdf", sha1, scansione(t, sha1))
		conDocumento2D(&th, dPDF2, cSciolto, aPDF2, "disegno-acme.pdf", sha2, scansione(t, sha2))
	}
	return th
}

// valutaConVista: la vista calcolata come la calcola v_fascicolo, poi ValutaProdotti.
func valutaConVista(t *testing.T, th fotorfq.Thread, m *motorea.Motore) valutazione.ValutazioneProdotti {
	t.Helper()
	vistaCome(&th)
	return valuta(t, th, m, nil)
}

// TestPO36SullaFotografia (PO-36 dalla fotografia; R72 D [R], R102 A, T-E1-10; T-B5-90, T-B5-92): con la fonte
// confermata, la BOM di lavoro e le righe decise da una persona il perimetro è chiuso: con il 2D che manca incompleta,
// con tutto presente completa, anche con la nomenclatura ancora da verificare; con un nodo da decidere, senza la fonte
// confermata, con una rimozione aperta, senza la BOM di lavoro mai completa (con una mancanza certa incompleta).
func TestPO36SullaFotografia(t *testing.T) {
	m := motoreCatena(t)
	t.Run("il perimetro chiuso e il 2D che manca", func(t *testing.T) {
		d := documentiDi(t, valutaConVista(t, scenaCompletezza(t, false), m), rifProdB)
		if statoDi(d) != "incompleta/voce_mancante chiuso/" {
			t.Fatalf("stato %s (%s)", statoDi(d), vociDi(d))
		}
		if v := voceDi(t, d, cProdotto, "disegno_2d"); esitoDi(v) != "manca/nessun_documento" || !v.DelProdotto || !v.Invariante || v.CalcolataDa != valutazione.CalcolataDaGo {
			t.Errorf("il 2D del finito %+v", v)
		}
		if v := voceDi(t, d, cProdotto, "cad_3d"); esitoDi(v) != "presente/" || v.CalcolataDa != valutazione.CalcolataDaVista || v.DocumentoID == nil || *v.DocumentoID != dStepB {
			t.Errorf("il cad_3d del finito, lo STEP strutturale: %+v", v)
		}
		if v := voceDi(t, d, cSciolto, "disegno_2d"); esitoDi(v) != "manca/nessun_documento" || v.Categoria != valutazione.CategoriaFabbricato {
			t.Errorf("il 2D dello sciolto %+v", v)
		}
		if n, ok := nonBloccanteDi(d, cSciolto, "sviluppo_dxf"); !ok || n.EsitoVista != "manca" || len(d.Voci) != 3 || len(d.NonBloccanti) != 2 {
			t.Errorf("voci %s, non bloccanti %+v", vociDi(d), d.NonBloccanti)
		}
	})
	t.Run("il perimetro chiuso e tutto presente", func(t *testing.T) {
		v := valutaConVista(t, scenaCompletezza(t, true), m)
		p := prodotto(t, v, rifProdB)
		if statoDi(p.Documenti) != "completa/ chiuso/" || !p.BOM.Verificata {
			t.Errorf("stato %s (%s), BOM verificata %v", statoDi(p.Documenti), vociDi(p.Documenti), p.BOM.Verificata)
		}
		if g := voceDi(t, p.Documenti, cSciolto, "disegno_2d").Disegni; g == nil || g.Primario == nil || *g.Primario.DocumentoID != dPDF2 {
			t.Errorf("il gruppo dei 2D della voce %+v", g)
		}
		if len(p.Documenti.Previsti) != 0 {
			t.Errorf("previsti %+v: la radice è il prodotto, e il nodo deciso nel perimetro è una riga certa", p.Documenti.Previsti)
		}
	})
	t.Run("completa con la nomenclatura ancora da verificare", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		arcoDi(t, &th, "#1", "#2").Albero = nil
		p := prodotto(t, valutaConVista(t, th, m), rifProdB)
		if statoDi(p.Documenti) != "completa/ chiuso/" || p.BOM.Nomenclatura.Stato == valutazione.StatoAsseVerificata {
			t.Errorf("stato %s, nomenclatura %s", statoDi(p.Documenti), asse(p.BOM.Nomenclatura))
		}
	})
	t.Run("un nodo da decidere: mai completa", func(t *testing.T) {
		d := documentiDi(t, valutaConVista(t, scenaTreNodi(t, true), m), rifProdB)
		if statoDi(d) != "non_calcolabile/perimetro_aperto aperto/nodi_da_decidere" {
			t.Errorf("stato %s (%s)", statoDi(d), vociDi(d))
		}
		if p := previstoDi(t, d, nodoB("#3"), "disegno_2d"); p.Motivo != valutazione.MotivoPrevistoNodoProposto {
			t.Errorf("previsto %+v", p)
		}
		th := scenaTreNodi(t, false)
		if d := documentiDi(t, valutaConVista(t, th, m), rifProdB); statoDi(d) != "incompleta/voce_mancante aperto/nodi_da_decidere" {
			t.Errorf("con il 2D del finito che manca: %s", statoDi(d))
		}
	})
	t.Run("senza la fonte confermata: mai completa", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		senzaFonte(&th)
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if statoDi(d) != "non_calcolabile/perimetro_aperto aperto/fonte_non_confermata" {
			t.Errorf("stato %s (%s)", statoDi(d), vociDi(d))
		}
		th = scenaCompletezza(t, false)
		senzaFonte(&th)
		if d := documentiDi(t, valutaConVista(t, th, m), rifProdB); statoDi(d) != "incompleta/voce_mancante aperto/fonte_non_confermata" {
			t.Errorf("con il 2D del finito che manca: %s", statoDi(d))
		}
	})
	t.Run("una rimozione aperta: lo sciolto fra i previsti", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		confermaComponente(&th, cManuale, "7120500A", cSciolto, 1)
		th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cSciolto, QtaWorking: 2}}
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if haVoce(d, cManuale, "") || previstoDi(t, d, ancoraggio.RifComponente(cManuale), "disegno_2d").Motivo != valutazione.MotivoPrevistoRimozioneAperta {
			t.Errorf("il figlio della riga con la rimozione, raggiungibile solo da lei: voci %s", vociDi(d))
		}
		if statoDi(d) != "non_calcolabile/perimetro_aperto aperto/rimozioni_aperte" || haVoce(d, cSciolto, "") {
			t.Errorf("stato %s (%s)", statoDi(d), vociDi(d))
		}
		p := previstoDi(t, d, ancoraggio.RifComponente(cSciolto), "disegno_2d")
		if p.Motivo != valutazione.MotivoPrevistoRimozioneAperta || p.CodiceProposto != "7120200A" || p.Disegni == nil || *p.Disegni.Primario.DocumentoID != dPDF2 {
			t.Errorf("previsto %+v", p)
		}
		if _, ok := nonBloccanteDi(d, cSciolto, "cad_3d"); ok {
			t.Error("i non bloccanti della riga con la rimozione aperta non sono certi")
		}
	})
	t.Run("la fonte confermata senza la BOM di lavoro: mai completa (T-B5-92)", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		th.RigheComponenteProposta[0].Marcatura = nil
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x73f), AllegatoID: aStepB,
			NomeFile: "7120100A_1.stp", Sha256: shaStepB, Chiave: "#9", IDGrezzo: "7120900A", Famiglia: "acme-catena", Fonte: "step", Stato: "aperta"})
		p := prodotto(t, valutaConVista(t, th, m), rifProdB)
		if p.Fonte.Stato != valutazione.FonteConfermata || p.Struttura != ancoraggio.StatoStrutturaCandidata {
			t.Fatalf("fonte %s, struttura %s", statoMotivo(p.Fonte), p.Struttura)
		}
		if statoDi(p.Documenti) != "non_calcolabile/perimetro_aperto aperto/bom_di_lavoro_assente" {
			t.Errorf("stato %s (%s)", statoDi(p.Documenti), vociDi(p.Documenti))
		}
	})
}

// TestLeRigheDelPerimetro (R62 e A; T-B5-91): le righe certe sono il componente del prodotto e i componenti attivi
// raggiungibili per le relazioni confermate, di qualunque origine (anche manuale); un componente senza la relazione e uno
// archiviato non hanno voci; la classificazione è di ogni componente attivo.
func TestLeRigheDelPerimetro(t *testing.T) {
	th := scenaCompletezza(t, true)
	confermaComponente(&th, cManuale, "7120500A", cSciolto, 1)
	th.Componenti[len(th.Componenti)-1].Origine = "manuale"
	th.Componenti = append(th.Componenti, componente(cFuori, "7120600A", "sciolto", "step"))
	confermaComponente(&th, cArchiviato, "7120700A", cProdotto, 1)
	arch := dataACME.Add(4 * time.Hour)
	th.Componenti[len(th.Componenti)-1].ArchiviatoIl = &arch
	v := valutaConVista(t, th, motoreCatena(t))
	d := documentiDi(t, v, rifProdB)
	if x := voceDi(t, d, cManuale, "disegno_2d"); esitoDi(x) != "manca/nessun_documento" {
		t.Errorf("il componente manuale del perimetro %+v", x)
	}
	if haVoce(d, cFuori, "") || haVoce(d, cArchiviato, "") {
		t.Errorf("voci %s: fuori dal perimetro e archiviato non hanno voci", vociDi(d))
	}
	if d.Stato != valutazione.DocumentiIncompleta {
		t.Errorf("stato %s", statoDi(d))
	}
	var classificati []uuid.UUID
	for _, c := range v.Classificazioni {
		classificati = append(classificati, c.ComponenteID)
	}
	if len(classificati) != 4 || classificazioneDi(t, v, cFuori).Categoria != valutazione.CategoriaFabbricato {
		t.Errorf("classificazioni %v: i quattro componenti attivi, l'archiviato no", classificati)
	}
}

// TestIlCommercialeELeRegoleDelClienteSullaFotografia (R103 C, R99 A; PO-33 sulla fotografia; K-01 con la lettura A; T-E1-17;
// decisioni sull'analista, punto 8): il commerciale nel perimetro senza righe chiede il 2D (schema_senza_2d, calcolata in
// Go); con le regole del cliente il 2D del finito resta bloccante (regola_cliente_senza_2d, regola_cliente_2d_non_bloccante),
// il cad_3d del cliente sullo sciolto si aggiunge, la riga esplicita non bloccante sullo sciolto si rispetta; RegolaCliente
// dai fabbisogni effettivi.
func TestIlCommercialeELeRegoleDelClienteSullaFotografia(t *testing.T) {
	m := motoreCatena(t)
	conCommerciale := func(th *fotorfq.Thread) {
		confermaComponente(th, cCommerciale, "7120400A", cProdotto, 4)
		th.Componenti[len(th.Componenti)-1].Tipo = "commerciale"
	}
	t.Run("il commerciale con le regole di default", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		conCommerciale(&th)
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		v := voceDi(t, d, cCommerciale, "disegno_2d")
		if esitoDi(v) != "manca/nessun_documento" || v.NotaRegola != valutazione.NotaSchemaSenza2D || v.CalcolataDa != valutazione.CalcolataDaGo ||
			!v.Invariante || v.RegolaCliente || v.Categoria != valutazione.CategoriaCommerciale {
			t.Errorf("il 2D del commerciale %+v", v)
		}
		if statoDi(d) != "incompleta/voce_mancante chiuso/" {
			t.Errorf("stato %s", statoDi(d))
		}
	})
	t.Run("le regole del cliente", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		conCommerciale(&th)
		th.Fabbisogni = conRegole(regolaCliente("finito", "sviluppo_dxf", false), regolaCliente("sciolto", "disegno_2d", false),
			regolaCliente("sciolto", "cad_3d", true), regolaCliente("commerciale", "cad_3d", false))
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if v := voceDi(t, d, cProdotto, "disegno_2d"); esitoDi(v) != "presente/" || !v.Bloccante || v.NotaRegola != valutazione.NotaRegolaClienteSenza2D ||
			!v.RegolaCliente || v.CalcolataDa != valutazione.CalcolataDaGo {
			t.Errorf("il 2D del finito %+v", v)
		}
		if haVoce(d, cProdotto, "cad_3d") {
			t.Error("il finito senza il cad_3d del cliente non ha la voce: lo STEP strutturale è l'asse della fonte")
		}
		if v := voceDi(t, d, cSciolto, "cad_3d"); esitoDi(v) != "manca/nessun_documento" || !v.RegolaCliente || v.CalcolataDa != valutazione.CalcolataDaVista {
			t.Errorf("il cad_3d dello sciolto %+v", v)
		}
		if haVoce(d, cSciolto, "disegno_2d") {
			t.Error("il 2D dello sciolto reso non bloccante dal cliente non è fra le voci (K-01 A)")
		}
		if n, ok := nonBloccanteDi(d, cSciolto, "disegno_2d"); !ok || n.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante || n.EsitoVista != "ok" {
			t.Errorf("il 2D dello sciolto fra i non bloccanti %+v %v", n, ok)
		}
		if v := voceDi(t, d, cCommerciale, "disegno_2d"); v.NotaRegola != valutazione.NotaRegolaClienteSenza2D || !v.RegolaCliente {
			t.Errorf("il 2D del commerciale %+v", v)
		}
		if _, ok := nonBloccanteDi(d, cCommerciale, "cad_3d"); !ok {
			t.Error("il cad_3d del commerciale fra i non bloccanti")
		}
		if d.Stato != valutazione.DocumentiIncompleta {
			t.Errorf("stato %s", statoDi(d))
		}
	})
	t.Run("la riga esplicita sul finito con il 2D non bloccante", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		th.Fabbisogni = conRegole(regolaCliente("finito", "disegno_2d", false), regolaCliente("finito", "cad_3d", true))
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if v := voceDi(t, d, cProdotto, "disegno_2d"); !v.Bloccante || v.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante || esitoDi(v) != "presente/" {
			t.Errorf("il 2D del finito %+v", v)
		}
		if statoDi(d) != "completa/ chiuso/" {
			t.Errorf("stato %s", statoDi(d))
		}
	})
}

// TestGliEsitiDellaVistaSullaFotografia (contratto §1.6, «per gli altri tipi bloccanti: come nella vista»; S3; R62 b A;
// rischio 6, CalcolataDa): il cad_3d del finito con gli esiti della vista; il 2D che la vista dà «ok» per il documento più
// recente in un formato non configurato, che A1c dà «manca» (CalcolataDa = go).
func TestGliEsitiDellaVistaSullaFotografia(t *testing.T) {
	m := motoreCatena(t)
	pa, aPA := uid(0x9a1), uid(0x9a2)
	for _, c := range []struct {
		esito  string
		atteso string
	}{
		{"ok_in_coda", "presente/"}, {"ok_errore_nas", "presente/"}, {"derogato", "presente/"}, {"sul_portale", "da_verificare/sul_portale"},
		{"da_confermare", "da_verificare/associazione_non_confermata"}, {"manca", "manca/nessun_documento"},
	} {
		th := scenaCompletezza(t, true)
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pa, AllegatoID: aPA, Tipo: "cad_3d", Fonte: "estensione", Stato: "aperta"})
		vistaCome(&th)
		for i := range th.Fascicolo {
			if r := &th.Fascicolo[i]; r.ComponenteID == cProdotto && r.TipoDocumento == "cad_3d" {
				r.Esito, r.DocumentoID, r.PropostaAperta, r.DerogaID = c.esito, nil, nil, nil
				switch c.esito {
				case "da_confermare":
					r.PropostaAperta = &pa
				case "derogato":
					r.DerogaID = &derogaB
				}
			}
		}
		v := voceDi(t, documentiDi(t, valuta(t, th, m, nil), rifProdB), cProdotto, "cad_3d")
		if esitoDi(v) != c.atteso || v.CalcolataDa != valutazione.CalcolataDaVista {
			t.Errorf("%s: %+v", c.esito, v)
		}
		if c.esito == "da_confermare" && (v.FileCandidato == nil || *v.FileCandidato != aPA || *v.PropostaAperta != pa) {
			t.Errorf("%s: il file candidato %+v", c.esito, v)
		}
		if c.esito == "derogato" && (v.DerogaID == nil || *v.DerogaID != derogaB) {
			t.Errorf("%s: la deroga %+v", c.esito, v)
		}
	}
	t.Run("la vista prende il documento più recente, qualunque sia il formato", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		dwg := documento2D(dDWG, cSciolto, aDWG, "disegno-acme.dwg", "dwg", shaDWG)
		conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, &dwg, uuid.Nil)
		vistaCome(&th)
		for _, r := range th.Fascicolo {
			if r.ComponenteID == cSciolto && r.TipoDocumento == "disegno_2d" && r.Esito != "ok" {
				t.Fatalf("la vista %s", r.Esito)
			}
		}
		v := voceDi(t, documentiDi(t, valuta(t, th, m, nil), rifProdB), cSciolto, "disegno_2d")
		if esitoDi(v) != "manca/formato_non_configurato" || v.CalcolataDa != valutazione.CalcolataDaGo {
			t.Errorf("il DWG non soddisfa il 2D: %+v", v)
		}
	})
}

// TestLaVoceDelDisegnoSullaFotografia (contratto §1.6; R62 b A, R62 D.4; LD-17; T-E1R-10; T-B0-31): la deroga sul 2D non
// lo sostituisce, con documenti.deroga_non_sostituisce_2d; «assegna» da verificare, con il file e la proposta della
// vista; un candidato del motore A da verificare, e scartato da una persona non soddisfa la voce; un da_determinare
// candidato del motore A da verificare (la vista non lo vede), e non entra nel gruppo dei 2D; un PDF confermato che non si
// apre da verificare.
func TestLaVoceDelDisegnoSullaFotografia(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	voceSciolto := func(t *testing.T, th fotorfq.Thread) (valutazione.VoceFabbisogno, valutazione.ValutazioneProdotti) {
		t.Helper()
		v := valutaConVista(t, th, m)
		return voceDi(t, documentiDi(t, v, rifProdB), cSciolto, "disegno_2d"), v
	}
	t.Run("la deroga sul 2D", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaA, ComponenteID: cSciolto, Tipo: "disegno_2d", Motivo: "lo facciamo noi", UtenteID: operatore, CreataIl: dataACME}}
		v, tutto := voceSciolto(t, th)
		if esitoDi(v) != "manca/derogato_non_sostituisce_2d" || v.DerogaID == nil || *v.DerogaID != derogaA {
			t.Errorf("voce %+v", v)
		}
		d := conCodice(tutto.Diagnostiche, valutazione.CodiceDocumentiDerogaNonSostituisce2D)
		if len(d) != 1 || d[0].Percorso != "prodotti["+rifProdB+"].documenti" || d[0].Rif[1] != ancoraggio.RifComponente(cSciolto) {
			t.Errorf("diagnostiche %+v", d)
		}
	})
	t.Run("«assegna»", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		conFile2D(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), scansione(t, sha2), nil, cSciolto)
		v, _ := voceSciolto(t, th)
		if esitoDi(v) != "da_verificare/associazione_non_confermata" || v.FileCandidato == nil || *v.FileCandidato != aPDF2 || v.PropostaAperta == nil {
			t.Errorf("voce %+v", v)
		}
	})
	t.Run("«assegna» su un DWG non è un'associazione da verificare", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, nil, cSciolto)
		if v, _ := voceSciolto(t, th); esitoDi(v) != "manca/nessun_documento" || v.PropostaAperta == nil {
			t.Errorf("voce %+v", v)
		}
	})
	t.Run("il candidato del motore A e lo scarto", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		conFile2D(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), conCartiglio(t, sha2, [2]string{"codice", "7120200A2"}), nil, uuid.Nil)
		v, _ := voceSciolto(t, th)
		if esitoDi(v) != "da_verificare/associazione_non_confermata" || v.FileCandidato == nil || *v.FileCandidato != aPDF2 {
			t.Fatalf("il candidato: %+v", v)
		}
		il := dataACME.Add(5 * time.Hour)
		th.Proposte[len(th.Proposte)-1].Stato, th.Proposte[len(th.Proposte)-1].DecisoDa, th.Proposte[len(th.Proposte)-1].DecisoIl = "scartata", &op, &il
		v, tutto := voceSciolto(t, th)
		if esitoDi(v) != "manca/nessun_documento" || v.FileCandidato != nil {
			t.Errorf("lo scarto non soddisfa la voce: %+v", v)
		}
		if v.Disegni == nil || v.Disegni.Primario == nil || len(gruppoDi(t, tutto, cSciolto).Alternativi) != 0 {
			t.Errorf("il gruppo dei 2D resta quello della fase 2: %+v", v.Disegni)
		}
	})
	t.Run("il da_determinare candidato del motore A (LD-17)", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		f := conCartiglio(t, shaDaDet, [2]string{"codice", "7120200A2"})
		conAllegato(&th, allegato(aDaDet, 4, "scansione-acme.pdf", "pdf", shaDaDet), f)
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pDaDet, AllegatoID: aDaDet, Tipo: "da_determinare", Fonte: "estensione", Stato: "aperta"})
		v, tutto := voceSciolto(t, th)
		if esitoDi(v) != "da_verificare/associazione_non_confermata" || v.FileCandidato == nil || *v.FileCandidato != aDaDet {
			t.Errorf("voce %+v", v)
		}
		for _, x := range tutto.Disegni {
			if x.ComponenteID == cSciolto {
				t.Errorf("il da_determinare non è un 2D del gruppo: %+v", x.Gruppo)
			}
		}
		il := dataACME.Add(5 * time.Hour)
		th.Proposte[len(th.Proposte)-1].Stato, th.Proposte[len(th.Proposte)-1].DecisoDa, th.Proposte[len(th.Proposte)-1].DecisoIl = "scartata", &op, &il
		if v, _ := voceSciolto(t, th); esitoDi(v) != "manca/nessun_documento" {
			t.Errorf("scartato: %+v", v)
		}
	})
	t.Run("un PDF confermato che non si apre", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		f := fattiPDFErrore(t, sha2)
		conDocumento2D(&th, dPDF2, cSciolto, aPDF2, "disegno-acme.pdf", sha2, &f)
		if v, _ := voceSciolto(t, th); esitoDi(v) != "da_verificare/contenuto_non_aperto" {
			t.Errorf("voce %+v", v)
		}
	})
}

// TestIPrevistiSullaFotografia (contratto §1.6, «Fabbisogni previsti»; R62 e A; T-B5-93; tabella dell'analista, T29): i
// nodi delle strutture che contano senza decisione, con il tipo proposto della riga legacy o, senza riga, quello che il
// legacy scriverebbe (sottoassieme con figli, sciolto senza), il codice proposto della catena e i 2D candidati del nodo;
// un nodo deciso fuori dal perimetro, con il tipo e il codice del componente; un nodo scartato da una persona no.
func TestIPrevistiSullaFotografia(t *testing.T) {
	m := motoreCatena(t)
	t.Run("i nodi senza decisione", func(t *testing.T) {
		th := scenaTreNodi(t, true)
		rigaDi(t, &th, "#3").TipoProposto = testo("sciolto")
		var tenute []fotorfq.RigaComponenteProposta
		for _, r := range th.RigheComponenteProposta {
			if r.Chiave != "#4" {
				tenute = append(tenute, r)
			}
		}
		th.RigheComponenteProposta = tenute
		conFile2D(&th, allegato(aNodo3, 5, "disegno3-acme.pdf", "pdf", shaNodo3), conCartiglio(t, shaNodo3, [2]string{"codice", "7120310A1"}), nil, uuid.Nil)
		th.Fabbisogni = conRegole(regolaCliente("sciolto", "cad_3d", true), regolaCliente("sciolto", "disegno_2d", true))
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		p3 := previstoDi(t, d, nodoB("#3"), "cad_3d")
		if p3.Motivo != valutazione.MotivoPrevistoNodoProposto || p3.Disegni != nil {
			t.Errorf("il cad_3d del cliente sul nodo sciolto #3: %+v", p3)
		}
		previstoDi(t, d, nodoB("#3"), "disegno_2d")
		p4 := previstoDi(t, d, nodoB("#4"), "disegno_2d")
		if p4.Disegni == nil || p4.Disegni.Primario == nil || *p4.Disegni.Primario.AllegatoID != aNodo3 ||
			p4.Disegni.Primario.Provenienza != ancoraggio.OrigineProposto {
			t.Errorf("i 2D candidati del nodo #4: %+v", p4.Disegni)
		}
		if p := previstoDi(t, d, nodoB("#4"), "cad_3d"); p.Motivo != valutazione.MotivoPrevistoNodoProposto {
			t.Errorf("#4, senza riga e senza figli, è uno sciolto proposto, con la regola del cliente: %+v", p)
		}
	})
	t.Run("i 2D candidati di un nodo contro la sua identità", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A1", ""}, {"#2", "7120200A3", ""}, {"#3", "7120300A1", ""}, {"#4", "7120310A2", ""}},
			[]arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}, {"#3", "#4", 1}})
		confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
		th.Fabbisogni = fabbisogniDefault()
		conFile2D(&th, allegato(aNodo3, 5, "disegno3-acme.pdf", "pdf", shaNodo3), conCartiglio(t, shaNodo3, [2]string{"codice", "7120310A2"}), nil, uuid.Nil)
		p := previstoDi(t, documentiDi(t, valutaConVista(t, th, motoreCatenaRev(t)), rifProdB), nodoB("#4"), "disegno_2d")
		if p.Disegni == nil || p.Disegni.Primario == nil || p.Disegni.Primario.ConfrontatoCon != valutazione.ConfrontatoConProposto ||
			p.Disegni.Primario.CompatibilitaRevisione != motorea.CompatibilitaUguale || p.Disegni.Primario.CompatibilitaCodice != motorea.CompatibilitaUguale {
			t.Errorf("il 2D del nodo #4 confrontato con la sua identità (7120310A, revisione 2): %+v", p.Disegni)
		}
	})
	t.Run("il tipo proposto senza riga dalla struttura", func(t *testing.T) {
		th := scenaTreNodi(t, true)
		var tenute []fotorfq.RigaComponenteProposta
		for _, r := range th.RigheComponenteProposta {
			if r.Chiave != "#3" {
				tenute = append(tenute, r)
			}
		}
		th.RigheComponenteProposta = tenute
		th.Fabbisogni = conRegole(regolaCliente("sottoassieme", "sviluppo_dxf", true))
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if p := previstoDi(t, d, nodoB("#3"), "sviluppo_dxf"); p.Motivo != valutazione.MotivoPrevistoNodoProposto {
			t.Errorf("#3 ha un figlio: sottoassieme proposto, con la regola del cliente %+v", p)
		}
		previstoDi(t, d, nodoB("#3"), "disegno_2d")
	})
	t.Run("un nodo deciso fuori dal perimetro e un nodo scartato", func(t *testing.T) {
		th := scenaTreNodi(t, true)
		op := operatore
		th.Componenti = append(th.Componenti, componente(cFuori, "7120300A", "sottoassieme", "step"))
		decidi(t, &th, "#3", "confermata", cFuori, &op)
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		p := previstoDi(t, d, nodoB("#3"), "disegno_2d")
		if p.Motivo != valutazione.MotivoPrevistoNodoDecisoFuoriPerimetro || p.CodiceProposto != "7120300A" || haVoce(d, cFuori, "") {
			t.Errorf("previsto %+v, voci %s", p, vociDi(d))
		}
		th = scenaTreNodi(t, true)
		il := dataACME.Add(2 * time.Hour)
		r := rigaDi(t, &th, "#4")
		r.Stato, r.DecisoDa, r.DecisoIl = "scartata", &op, &il
		d = documentiDi(t, valutaConVista(t, th, m), rifProdB)
		for _, p := range d.Previsti {
			if p.Nodo == nodoB("#4") {
				t.Errorf("il nodo scartato da una persona non è previsto: %+v", p)
			}
		}
		previstoDi(t, d, nodoB("#3"), "disegno_2d")
	})
}

// TestPO28LaClassificazioneSullaFotografia (PO-28 sulla fotografia; T-E1-18, T-E1R-01; LD-19): uno sciolto e un
// commerciale della famiglia minuteria nella grammatica ACME: Proposta = minuteria, la categoria dal tipo, il 2D
// richiesto; nessuna ConfermaCategoria sui dati (nessun adattatore), quindi nessuna esenzione.
func TestPO28LaClassificazioneSullaFotografia(t *testing.T) {
	m := motoreConFamiglie(t, famigliaCatena(), famigliaMinuteria())
	th := scenaCompletezza(t, true)
	confermaComponente(&th, cMinuteria, "7129001", cProdotto, 8)
	confermaComponente(&th, cComMinut, "7129002", cProdotto, 4)
	th.Componenti[len(th.Componenti)-1].Tipo = "commerciale"
	v := valutaConVista(t, th, m)
	for _, c := range []struct {
		comp                     uuid.UUID
		ruolo, categoria, propos string
	}{
		{cProdotto, "prodotto", "fabbricato", ""},
		{cSciolto, "componente", "fabbricato", ""},
		{cMinuteria, "componente", "fabbricato", "minuteria"},
		{cComMinut, "componente", "commerciale", "minuteria"},
	} {
		if x := classificazioneDi(t, v, c.comp); x.Ruolo != c.ruolo || x.Categoria != c.categoria || x.Proposta != c.propos {
			t.Errorf("%s: %+v", c.comp, x)
		}
	}
	d := documentiDi(t, v, rifProdB)
	if x := voceDi(t, d, cMinuteria, "disegno_2d"); esitoDi(x) != "manca/nessun_documento" || x.Categoria != valutazione.CategoriaFabbricato {
		t.Errorf("la minuteria solo proposta chiede il 2D: %+v", x)
	}
	if x := voceDi(t, d, cComMinut, "disegno_2d"); x.NotaRegola != valutazione.NotaSchemaSenza2D || x.Categoria != valutazione.CategoriaCommerciale {
		t.Errorf("il commerciale della famiglia minuteria chiede il 2D: %+v", x)
	}
}

// TestLaCompletezzaCheNonSiCalcola (T-12; T-B0-07): con una sezione della completezza assente lo stato è non_calcolabile
// con non_determinabile, senza voci; il target senza componente è non_calcolabile.
func TestLaCompletezzaCheNonSiCalcola(t *testing.T) {
	m := motoreCatena(t)
	for _, s := range []string{fotorfq.SezioneFascicolo, fotorfq.SezioneFabbisogni, fotorfq.SezioneDeroghe, fotorfq.SezioneRimozioniAperte} {
		th := scenaCompletezza(t, true)
		vistaCome(&th)
		f := fotografia(th)
		f.Sezioni[s] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente, Motivo: "non letta"}
		v, err := valutazione.ValutaProdotti(f, th, m, nil)
		if err != nil {
			t.Fatal(err)
		}
		if d := documentiDi(t, v, rifProdB); statoDi(d) != "non_calcolabile/non_determinabile aperto/non_determinabile" || len(d.Voci) != 0 {
			t.Errorf("%s assente: %s (%s)", s, statoDi(d), vociDi(d))
		}
	}
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A.\r\nGrazie")
	th.Identificativi = []fotorfq.Identificativo{confermato("7120100A")}
	th.Fabbisogni = fabbisogniDefault()
	if d := documentiDi(t, valuta(t, th, motoreCatena(t), nil), "identificativo:7120100A"); statoDi(d) != "non_calcolabile/target_senza_componente aperto/target_senza_componente" ||
		len(d.Voci) != 0 {
		t.Errorf("il target senza componente: %s", statoDi(d))
	}
}

// TestLaCompletezzaSullaFotografiaEDeterministica (il determinismo del motore): gli elenchi della fotografia in un altro
// ordine danno gli stessi byte canonici della completezza e delle classificazioni, anche con i previsti, le deroghe e le
// proposte.
func TestLaCompletezzaSullaFotografiaEDeterministica(t *testing.T) {
	m := motoreCatena(t)
	th := scenaTreNodi(t, true)
	confermaComponente(&th, cManuale, "7120500A", cSciolto, 1)
	conFile2D(&th, allegato(aNodo3, 5, "disegno3-acme.pdf", "pdf", shaNodo3), conCartiglio(t, shaNodo3, [2]string{"codice", "7120310A1"}), nil, uuid.Nil)
	th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaA, ComponenteID: cManuale, Tipo: "disegno_2d", Motivo: "prova", UtenteID: operatore, CreataIl: dataACME}}
	vistaCome(&th)
	a := valuta(t, th, m, nil)
	r := th
	r.Componenti, r.Relazioni, r.Fascicolo, r.Fabbisogni = rovescia(th.Componenti), rovescia(th.Relazioni), rovescia(th.Fascicolo), rovescia(th.Fabbisogni)
	r.Documenti, r.Proposte, r.Deroghe, r.Allegati = rovescia(th.Documenti), rovescia(th.Proposte), rovescia(th.Deroghe), rovescia(th.Allegati)
	r.RigheComponenteProposta, r.RigheRelazioneProposta = rovescia(th.RigheComponenteProposta), rovescia(th.RigheRelazioneProposta)
	b := valuta(t, r, m, nil)
	if canonicoDi(t, documentiDi(t, a, rifProdB)) != canonicoDi(t, documentiDi(t, b, rifProdB)) || canonicoDi(t, a.Classificazioni) != canonicoDi(t, b.Classificazioni) ||
		canonicoDi(t, a) != canonicoDi(t, b) {
		t.Error("l'ordine degli elenchi della fotografia cambia la completezza")
	}
	if d := documentiDi(t, a, rifProdB); len(d.Previsti) == 0 || len(conCodice(a.Diagnostiche, valutazione.CodiceDocumentiDerogaNonSostituisce2D)) != 1 {
		t.Errorf("la scena ha i previsti e la deroga: %+v", d.Previsti)
	}
}

// motoreConFamiglie: la grammatica ACME con le famiglie date, dalla porta del prodotto.
func motoreConFamiglie(t *testing.T, f ...grammatica.FamigliaCodice) *motorea.Motore {
	t.Helper()
	g := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       f,
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	s, err := grammatica.NuovoSnapshot(raw, limitiACME())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	mm, d, err := motorea.CompilaVerificato(s, limitiACME())
	if err != nil || mm == nil {
		t.Fatalf("la grammatica non compila: %v %+v", err, d)
	}
	return mm
}

// TestLEsitoCalcolatoComeLaVista (R62 d C; contratto §1.6, «per i componenti senza riga nella vista»): senza le righe di
// v_fascicolo i fabbisogni vengono dalle regole effettive e l'esito si calcola come lo calcola la vista, con
// CalcolataDa = go: il documento corrente più recente (anche con lo stato del NAS in errore), la deroga, la proposta
// aperta assegnata o con lo stesso codice, altrimenti manca; un documento sostituito o una proposta chiusa non contano.
func TestLEsitoCalcolatoComeLaVista(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	cad := func(t *testing.T, th fotorfq.Thread) valutazione.VoceFabbisogno {
		t.Helper()
		th.Fascicolo = nil
		return voceDi(t, documentiDi(t, valuta(t, th, m, nil), rifProdB), cProdotto, "cad_3d")
	}
	pa := uid(0x9b1)
	propostaCad := func(th *fotorfq.Thread, stato string) {
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pa, AllegatoID: aStepB, Tipo: "cad_3d", Codice: testo("7120100a"), Fonte: "step", Stato: stato})
	}
	for _, c := range []struct {
		nome   string
		cambia func(*fotorfq.Thread)
		atteso string
	}{
		{"il documento corrente", func(*fotorfq.Thread) {}, "presente/"},
		{"lo stato del NAS in errore non toglie la presenza", func(th *fotorfq.Thread) { th.Documenti[0].StatoNas = "errore" }, "presente/"},
		{"il documento sostituito non conta", func(th *fotorfq.Thread) { th.Documenti[0].SostituitoDa = ptr(uid(0x9b2)) }, "manca/nessun_documento"},
		{"la deroga", func(th *fotorfq.Thread) {
			th.Documenti[0].SostituitoDa = ptr(uid(0x9b2))
			th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaB, ComponenteID: cProdotto, Tipo: "cad_3d", Motivo: "prova", UtenteID: op, CreataIl: dataACME}}
		}, "presente/"},
		{"la deroga di un altro tipo non conta", func(th *fotorfq.Thread) {
			th.Documenti[0].SostituitoDa = ptr(uid(0x9b2))
			th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaB, ComponenteID: cProdotto, Tipo: "sviluppo_dxf", Motivo: "prova", UtenteID: op, CreataIl: dataACME}}
		}, "manca/nessun_documento"},
		{"la proposta aperta con lo stesso codice", func(th *fotorfq.Thread) {
			th.Documenti[0].SostituitoDa = ptr(uid(0x9b2))
			propostaCad(th, "aperta")
		}, "da_verificare/associazione_non_confermata"},
		{"la proposta chiusa non conta", func(th *fotorfq.Thread) {
			th.Documenti[0].SostituitoDa = ptr(uid(0x9b2))
			propostaCad(th, "confermata")
		}, "manca/nessun_documento"},
	} {
		th := scenaCompletezza(t, true)
		c.cambia(&th)
		v := cad(t, th)
		if esitoDi(v) != c.atteso || v.CalcolataDa != valutazione.CalcolataDaGo {
			t.Errorf("%s: %+v", c.nome, v)
		}
		if c.atteso == "da_verificare/associazione_non_confermata" && (v.FileCandidato == nil || *v.FileCandidato != aStepB || v.PropostaAperta == nil || *v.PropostaAperta != pa) {
			t.Errorf("%s: il file candidato %+v", c.nome, v)
		}
		if c.nome == "la deroga" && (v.DerogaID == nil || *v.DerogaID != derogaB) {
			t.Errorf("%s: %+v", c.nome, v)
		}
	}
	t.Run("il 2D senza la riga della vista: la deroga e la proposta calcolate", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaA, ComponenteID: cSciolto, Tipo: "disegno_2d", Motivo: "prova", UtenteID: op, CreataIl: dataACME}}
		th.Fascicolo = nil
		v := voceDi(t, documentiDi(t, valuta(t, th, m, nil), rifProdB), cSciolto, "disegno_2d")
		if esitoDi(v) != "manca/derogato_non_sostituisce_2d" || v.DerogaID == nil || *v.DerogaID != derogaA {
			t.Errorf("voce %+v", v)
		}
		th = scenaCompletezza(t, false)
		conAllegato(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), scansione(t, sha2))
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: uid(0x9b3), AllegatoID: aPDF2, Tipo: "disegno_2d", Codice: testo("7120200A"), Fonte: "nome_file", Stato: "aperta"})
		th.Fascicolo = nil
		v = voceDi(t, documentiDi(t, valuta(t, th, m, nil), rifProdB), cSciolto, "disegno_2d")
		if esitoDi(v) != "da_verificare/associazione_non_confermata" || v.FileCandidato == nil || *v.FileCandidato != aPDF2 || v.PropostaAperta == nil {
			t.Errorf("la proposta aperta con il codice dello sciolto: %+v", v)
		}
	})
}

// TestICasiDiConfineDellaCompletezza (R62 e A; T-E1R-10; T-B4-24; LD-17; T-B0-30): la riga con una rimozione aperta resta
// fra i previsti anche se il prodotto la raggiunge per un'altra via; uno scarto senza chi l'ha deciso non è uno scarto; un
// nodo scartato da un automatismo resta previsto; la diagnostica della deroga solo sul 2D; un da_determinare in un formato
// non configurato, o un file con un altro tipo documentale, non è un'associazione da verificare, e un da_determinare
// candidato di un componente non tocca la voce di un altro; i 2D candidati di un nodo sono solo i file 2D.
func TestICasiDiConfineDellaCompletezza(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	// Riscritta per R-22 (T-B5-56 deciso dall'orchestratore [T] con la raggiungibilità): il figlio di una rimozione aperta
	// che il prodotto raggiunge anche per un'altra via resta fra le voci certe, e la sua mancanza certa non si perde; il
	// perimetro resta aperto.
	t.Run("la rimozione aperta su un figlio raggiungibile per un'altra via", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		confermaComponente(&th, cManuale, "7120500A", cSciolto, 1)
		th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cProdotto, FiglioID: cManuale, Qta: 1, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
		th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cManuale, QtaWorking: 1}}
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if v := voceDi(t, d, cManuale, "disegno_2d"); esitoDi(v) != "manca/nessun_documento" || len(d.Previsti) != 0 {
			t.Errorf("voce %+v, previsti %+v", v, d.Previsti)
		}
		if statoDi(d) != "incompleta/voce_mancante aperto/rimozioni_aperte" {
			t.Errorf("stato %s: la mancanza certa non si perde, il perimetro resta aperto", statoDi(d))
		}
	})
	t.Run("uno scarto senza chi l'ha deciso non è uno scarto", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		conFile2D(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), conCartiglio(t, sha2, [2]string{"codice", "7120200A2"}), nil, uuid.Nil)
		th.Proposte[len(th.Proposte)-1].Stato = "scartata"
		if v := voceDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), cSciolto, "disegno_2d"); esitoDi(v) != "da_verificare/associazione_non_confermata" {
			t.Errorf("voce %+v", v)
		}
	})
	t.Run("un nodo scartato da un automatismo resta previsto", func(t *testing.T) {
		th := scenaTreNodi(t, true)
		r := rigaDi(t, &th, "#4")
		il := dataACME.Add(2 * time.Hour)
		r.Stato, r.DecisoIl = "scartata", &il
		previstoDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), nodoB("#4"), "disegno_2d")
	})
	t.Run("la diagnostica della deroga solo sul 2D", func(t *testing.T) {
		th := scenaCompletezza(t, true)
		th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaB, ComponenteID: cProdotto, Tipo: "cad_3d", Motivo: "prova", UtenteID: op, CreataIl: dataACME}}
		v := valutaConVista(t, th, m)
		if x := voceDi(t, documentiDi(t, v, rifProdB), cProdotto, "cad_3d"); x.DerogaID == nil ||
			len(conCodice(v.Diagnostiche, valutazione.CodiceDocumentiDerogaNonSostituisce2D)) != 0 {
			t.Errorf("voce %+v, diagnostiche %+v", x, v.Diagnostiche)
		}
	})
	daDeterminare := func(th *fotorfq.Thread, nome, est, tipo string) {
		f := conCartiglio(t, shaDaDet, [2]string{"codice", "7120200A2"})
		conAllegato(th, allegato(aDaDet, 4, nome, est, shaDaDet), f)
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pDaDet, AllegatoID: aDaDet, Tipo: tipo, Fonte: "estensione", Stato: "aperta"})
	}
	t.Run("i da_determinare che non contano", func(t *testing.T) {
		for _, c := range [][3]string{{"scansione-acme.dwg", "dwg", "da_determinare"}, {"scansione-acme.pdf", "pdf", "altro"}} {
			th := scenaCompletezza(t, false)
			daDeterminare(&th, c[0], c[1], c[2])
			if v := voceDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), cSciolto, "disegno_2d"); esitoDi(v) != "manca/nessun_documento" {
				t.Errorf("%s, %s: %+v", c[0], c[2], v)
			}
		}
		th := scenaCompletezza(t, false)
		daDeterminare(&th, "scansione-acme.pdf", "pdf", "da_determinare")
		d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
		if v := voceDi(t, d, cProdotto, "disegno_2d"); esitoDi(v) != "manca/nessun_documento" {
			t.Errorf("il da_determinare dello sciolto non tocca il finito: %+v", v)
		}
	})
	t.Run("i 2D candidati di un nodo sono solo i file 2D", func(t *testing.T) {
		th := scenaTreNodi(t, true)
		f := conCartiglio(t, shaDaDet, [2]string{"codice", "7120310A1"})
		conAllegato(&th, allegato(aDaDet, 4, "scansione-acme.pdf", "pdf", shaDaDet), f)
		th.Proposte = append(th.Proposte, fotorfq.PropostaAttuale{ID: pDaDet, AllegatoID: aDaDet, Tipo: "da_determinare", Fonte: "estensione", Stato: "aperta"})
		if p := previstoDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), nodoB("#4"), "disegno_2d"); p.Disegni != nil {
			t.Errorf("un da_determinare non è un 2D candidato del nodo: %+v", p.Disegni)
		}
	})
}

// TestIlProdottoNonFinito (R-23 della revisione della fase 3; R99 A; E1R §4.1): il componente del prodotto target non è
// un finito (un identificativo con il suo componente sottoassieme). Sulla regola, con una ConfermaCategoria minuteria il
// suo 2D resta richiesto (mai l'esenzione per il prodotto), e una riga esplicita non bloccante del cliente non lo toglie
// (resta bloccante, con la nota); dalla fotografia il suo ruolo è prodotto, e un finito che non è target resta prodotto.
func TestIlProdottoNonFinito(t *testing.T) {
	cp := cProdotto
	riga := valutazione.RigaDelPerimetro{ComponenteID: cp, Codice: "7120100A", TipoComponente: "sottoassieme", DelProdotto: true,
		Fabbisogni: []valutazione.FabbisognoDellaRiga{{TipoDocumento: "disegno_2d", Bloccante: true, EsitoVista: "manca", CalcolataDa: valutazione.CalcolataDaVista}}}
	conferme := []valutazione.ConfermaCategoria{{ComponenteID: cp, Categoria: valutazione.CategoriaMinuteria, Da: operatore, Il: dataACME}}
	chiusa := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true}
	d := valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa, Righe: []valutazione.RigaDelPerimetro{riga}, Conferme: conferme})
	if v := voceDi(t, d, cp, "disegno_2d"); !v.Bloccante || !v.Invariante || v.Categoria != valutazione.CategoriaMinuteria || d.Stato == valutazione.DocumentiCompleta {
		t.Errorf("il 2D del prodotto sottoassieme con la minuteria confermata: %+v, stato %s", v, statoDi(d))
	}
	c := valutazione.Classifica(valutazione.ComponenteDaClassificare{ComponenteID: &cp, Tipo: "sottoassieme", DelProdotto: true}, conferme)
	if c.Ruolo != valutazione.RuoloProdotto || c.Motivo != valutazione.MotivoCategoriaFinitoSempre2D || valutazione.EsenteDal2D(c) {
		t.Errorf("classificazione %+v", c)
	}
	riga.RegolaCliente, riga.Fabbisogni[0].Bloccante = true, false
	d = valutazione.Completezza(valutazione.IngressoCompletezza{Struttura: chiusa, Righe: []valutazione.RigaDelPerimetro{riga}})
	if v := voceDi(t, d, cp, "disegno_2d"); !v.Bloccante || v.NotaRegola != valutazione.NotaRegolaCliente2DNonBloccante || len(d.NonBloccanti) != 0 {
		t.Errorf("la riga esplicita non bloccante sul prodotto sottoassieme: %+v, non bloccanti %+v", v, d.NonBloccanti)
	}

	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A.\r\nGrazie")
	th.Identificativi = []fotorfq.Identificativo{confermato("7120100A")}
	th.Componenti = []fotorfq.Componente{componente(cProdotto, "7120100A", "sottoassieme", "step"), componente(cFuori, "7120600A", "finito", "step")}
	v := valuta(t, th, motoreCatena(t), nil)
	if x := classificazioneDi(t, v, cProdotto); x.Ruolo != valutazione.RuoloProdotto || x.Categoria != valutazione.CategoriaFabbricato {
		t.Errorf("il componente sottoassieme del target: %+v", x)
	}
	if x := classificazioneDi(t, v, cFuori); x.Ruolo != valutazione.RuoloProdotto {
		t.Errorf("il finito che non è target resta prodotto (emendamento E1 §6.3): %+v", x)
	}
}

// TestIlNodoConLArcoTolto (dubbio 2 del revisore della fase 3, deciso dall'orchestratore [T]; dubbio T-B5-67): un nodo
// che la radice raggiunge solo attraverso un arco la cui riga è scartata da una persona non è fra i previsti, e nemmeno i
// nodi sotto di lui, anche se il nodo è deciso come un componente fuori dal perimetro; un arco scartato da un
// automatismo, o con una riga tenuta su un'altra copia dello stesso contenuto, non toglie niente.
func TestIlNodoConLArcoTolto(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	scarta := func(t *testing.T, th *fotorfq.Thread, da *uuid.UUID) {
		t.Helper()
		a := arcoDi(t, th, "#1", "#3")
		il := dataACME.Add(2 * time.Hour)
		a.Stato, a.DecisoDa, a.DecisoIl = "scartata", da, &il
	}
	th := scenaTreNodi(t, true)
	scarta(t, &th, &op)
	d := documentiDi(t, valutaConVista(t, th, m), rifProdB)
	for _, p := range d.Previsti {
		if p.Nodo == nodoB("#3") || p.Nodo == nodoB("#4") {
			t.Errorf("un nodo sotto un arco tolto da una persona: %+v", p)
		}
	}
	th = scenaTreNodi(t, true)
	th.Componenti = append(th.Componenti, componente(cFuori, "7120300A", "sottoassieme", "step"))
	decidi(t, &th, "#3", "confermata", cFuori, &op)
	scarta(t, &th, &op)
	for _, p := range documentiDi(t, valutaConVista(t, th, m), rifProdB).Previsti {
		if p.Nodo == nodoB("#3") {
			t.Errorf("il nodo deciso fuori dal perimetro, con l'arco tolto da una persona: %+v", p)
		}
	}
	th = scenaTreNodi(t, true)
	scarta(t, &th, nil)
	d = documentiDi(t, valutaConVista(t, th, m), rifProdB)
	previstoDi(t, d, nodoB("#3"), "disegno_2d")
	previstoDi(t, d, nodoB("#4"), "disegno_2d")
	// Lo stesso arco con una riga tenuta su un'altra copia dello stesso STEP: l'arco non è tolto.
	th = scenaTreNodi(t, true)
	scarta(t, &th, &op)
	copia := uid(0x7ea)
	conAllegato(&th, allegato(copia, 8, "7120100A_1-copia.stp", "stp", shaStepB), nil)
	th.RigheRelazioneProposta = append(th.RigheRelazioneProposta, fotorfq.RigaRelazioneProposta{AllegatoID: copia, NomeFile: "7120100A_1-copia.stp",
		PadreChiave: "#1", FiglioChiave: "#3", Qta: 1, Stato: "aperta"})
	previstoDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), nodoB("#3"), "disegno_2d")
}
