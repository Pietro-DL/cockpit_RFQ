// L1 — le equivalenze del cartiglio (K4, opzione A dell'utente; equivalenze.go): un cartiglio che scrive il codice della
// parte (il materiale ACME) e, nella stessa entità, il documento 3D della stessa parte (cartiglio.riferimento_3d, il
// documento ACME che è l'id del nodo dello STEP) propone il disegno sul nodo del documento 3D, mai lo conferma, con il
// motivo equivalenza_cartiglio e le prove. Il gemello (un PDF letto solo dal nome, un DXF con il cartiglio) prende il
// candidato solo se il nome dice lo stesso codice; le contese non propongono niente e lo dicono; l'ordine degli
// ingressi non conta; senza una forma su cartiglio.riferimento_3d non cambia niente.
package ancoraggio_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati: la scena è quella delle radici di riserva (riserve_test.go), con il
// materiale «acme» 21xxxxxxx e il documento «acme-doc» 101xxxxxx_NN.

// famDocumentoEq: il documento ACME, che si legge anche sul riferimento 3D del cartiglio (la forma «riferimento», solo
// la base: la revisione del 3D sta in un campo a sé).
func famDocumentoEq() grammatica.FamigliaCodice {
	f := famDocumentoRis()
	f.Forme = append(f.Forme, grammatica.FormaCodice{ID: "riferimento", Selettori: []string{"cartiglio.riferimento_3d"}, Stato: grammatica.StatoAttiva,
		Completa: true, Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}})
	f.Esempi = append(append([]grammatica.EsempioCodice(nil), f.Esempi...),
		esempio("e-riferimento", "cartiglio.riferimento_3d", "101000002_01", false, grammatica.LetturaAttesa{Forma: "riferimento", Base: "101000002_01"}))
	return f
}

func motoreEq(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, famMaterialeRis(), famDocumentoEq())
}

// dxfEq: un DXF con il testo del worker (testo_dxf v1) e i campi del cartiglio dati.
func dxfEq(t *testing.T, m *motorea.Motore, n int, nome string, campi []campoP) ancoraggio.FileInterpretato {
	t.Helper()
	cs := []map[string]any{}
	for _, c := range campi {
		cs = append(cs, map[string]any{"etichetta": c.etichetta, "letta": c.etichetta, "valore": c.valore, "pagina": 1, "zona": "pagina",
			"fonte": "nativo", "riquadro": []float64{400, 0, 430, 3.5}, "confidenza": nil})
	}
	raw, err := json.Marshal(map[string]any{"esito": map[string]any{"tipo_proposto": "sviluppo_dxf"}, "testo_dxf": map[string]any{
		"versione": 1, "esito": "letto", "motivo": "", "versione_dxf": "AC1014", "codifica": "cp1252", "testi": 40, "annidati": 0, "troncato": false,
		"cartiglio": cs, "limiti": map[string]any{"byte_max": 67108864, "testi_max": 5000, "testo_max": 512, "campi_max": 24},
	}})
	if err != nil {
		t.Fatal(err)
	}
	sha := fmt.Sprintf("%064x", n)
	return conDisponibilita(fileS(t, m, uidS(n), nome, "dxf", sha, fattiGrezzi(t, sha, raw, nil)), ancoraggio.DisponibilitaDisponibile)
}

// pdfEq: un PDF con il nome e i campi del cartiglio dati (senza campi una scansione senza testo).
func pdfEq(t *testing.T, m *motorea.Motore, n int, nome string, campi ...campoP) ancoraggio.FileInterpretato {
	t.Helper()
	return pdfF(t, m, uidS(n), nome, fmt.Sprintf("%064x", n), campi)
}

// scenaEq: lo STEP della radice di riserva («210000001_101000001_01.stp»: la radice #1 è il documento 101000001_01, il
// figlio #2 il documento 101000002_01) e il target 210000001, letto come materiale.
func scenaEq(t *testing.T, m *motorea.Motore) (ancoraggio.FileInterpretato, ancoraggio.ContestoStrutturale, ancoraggio.ProdottoRichiesto) {
	t.Helper()
	step, s := stepRis(t, m, idStepR, "210000001_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
	return step, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}}, targetRis(t, m, cProdottoR)
}

// collocazioneEq: «collocazione/motivi» di un allegato.
func collocazioneEq(t *testing.T, e ancoraggio.EsitoAncoraggi, id uuid.UUID) string {
	t.Helper()
	a := ancoraggioDi(t, e, id)
	return fmt.Sprintf("%s %v", a.Collocazione, a.Motivi)
}

// candidatoEq controlla che il file sia un figlio con un solo candidato, proposto, per equivalenza del cartiglio, sul
// nodo #2, con le dimensioni misurate sul riferimento, e restituisce il candidato.
func candidatoEq(t *testing.T, e ancoraggio.EsitoAncoraggi, id uuid.UUID) ancoraggio.CandidatoAncoraggio {
	t.Helper()
	a := ancoraggioDi(t, e, id)
	figlio := ancoraggio.RifNodo(shaStepR, "#2")
	if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneFiglio || len(a.Motivi) != 0 {
		t.Fatalf("%s: atteso un figlio con un candidato unico: %+v", id, a)
	}
	c := a.Candidati[0]
	if c.Motivo != ancoraggio.MotivoCandidatoEquivalenzaCartiglio || c.Origine != ancoraggio.OrigineProposto || c.Livello != ancoraggio.LivelloComponente ||
		c.Target != figlio || len(c.Posizioni) != 1 || c.Posizioni[0].Nodo != figlio || len(c.Conflitti) != 0 || len(c.Equivalenze) == 0 {
		t.Errorf("%s: il candidato per equivalenza sul nodo #2: %+v", id, c)
	}
	for _, d := range []struct {
		nome   string
		atteso motorea.Compatibilita
	}{
		{ancoraggio.DimensioneNamespace, motorea.CompatibilitaNonDeterminabile},
		{ancoraggio.DimensioneBase, motorea.CompatibilitaUguale},
		{ancoraggio.DimensioneMarcatore, motorea.CompatibilitaUguale},
		{ancoraggio.DimensioneRevisione, motorea.CompatibilitaNonDeterminabile},
	} {
		if got := dimensione(c, d.nome, figlio); got != d.atteso {
			t.Errorf("%s: la dimensione %s sul nodo è %q, attesa %q", id, d.nome, got, d.atteso)
		}
	}
	return c
}

// TestLEquivalenzaDelCartiglioProponeIlNodo (i): il 2D del figlio letto come materiale, con il riferimento 3D del
// documento del figlio nello stesso cartiglio, è un candidato unico sul nodo #2, con il motivo equivalenza_cartiglio e
// la prova (l'allegato, l'entità del cartiglio, le due letture, la fonte cartiglio). Il 2D della radice, con il codice
// del target e il riferimento della radice, resta il prodotto (base_del_target): la radice non è un nodo da componente.
// Nessuna diagnostica nuova.
func TestLEquivalenzaDelCartiglioProponeIlNodo(t *testing.T) {
	m := motoreEq(t)
	step, ctx, tg := scenaEq(t, m)
	figlio := pdfEq(t, m, 0xa01, "disegno-figlio.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"})
	radice := pdfEq(t, m, 0xa02, "disegno-radice.pdf", campoP{"codice", "210000001"}, campoP{"riferimento_3d", "101000001_01"})
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, figlio, radice}, []ancoraggio.ProdottoRichiesto{tg}, ctx)

	c := candidatoEq(t, e, figlio.AllegatoID)
	var codice, riferimento string
	for _, l := range figlio.Interpretazione.Letture {
		switch l.Forma.Selettore.Campo.Valore {
		case "codice":
			codice = l.ID
		case "riferimento_3d":
			riferimento = l.ID
			if l.Funzione != motorea.FunzRelazione || l.Forma.Namespace != "acme-doc" {
				t.Errorf("il riferimento 3D è una relazione nel namespace del documento: %+v", l)
			}
		}
	}
	if !reflect.DeepEqual(c.Equivalenze, []ancoraggio.ProvaEquivalenza{{AllegatoID: figlio.AllegatoID, Entita: "e:pdf:p1", LetturaCodice: codice,
		LetturaRiferimento: riferimento, Fonte: ancoraggio.FonteEquivalenzaCartiglio}}) || !reflect.DeepEqual(c.Letture, []string{codice}) {
		t.Errorf("la prova dell'equivalenza: %+v, letture %v", c.Equivalenze, c.Letture)
	}
	a := ancoraggioDi(t, e, radice.AllegatoID)
	if a.Collocazione != ancoraggio.CollocazioneRadice || len(a.Candidati) != 1 || a.Candidati[0].Motivo != ancoraggio.MotivoCandidatoBaseDelTarget ||
		len(a.Candidati[0].Equivalenze) != 0 {
		t.Errorf("il 2D della radice resta il prodotto: %+v", a)
	}
	if len(conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa)) != 0 {
		t.Errorf("nessuna contesa: %+v", e.Diagnostiche)
	}
	// Il nodo #2 ha un file: non è più senza file. E porta il codice della parte del cartiglio (CatenaCodice.Equivalente:
	// il P/N accanto al Ref.3D, collaudo del 14/10), con la prova; l'identità del nodo resta quella dello STEP.
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			if n.Rif != ancoraggio.RifNodo(shaStepR, "#2") {
				continue
			}
			if n.SenzaFile {
				t.Errorf("il nodo #2 con il 2D per equivalenza è senza file")
			}
			if eq := n.Codice.Equivalente; eq == nil || eq.Namespace != "acme" || eq.Base != "210000002" || !reflect.DeepEqual(eq.Prove, c.Equivalenze) ||
				n.Codice.Identita.Base != "101000002_01" {
				t.Errorf("il nodo #2: il codice della parte dal cartiglio %+v, identità %+v", eq, n.Codice.Identita)
			}
		}
	}
}

// nodiConEquivalente: i nodi delle strutture con il codice della parte dal cartiglio, come «rif=base».
func nodiConEquivalente(e ancoraggio.EsitoAncoraggi) []string {
	var out []string
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			if n.Codice.Equivalente != nil {
				out = append(out, n.Rif+"="+n.Codice.Equivalente.Base)
			}
		}
	}
	return out
}

// TestLEquivalenzaSoloConLaForma: senza una forma della grammatica su cartiglio.riferimento_3d il campo non dà letture,
// e il 2D del figlio letto solo come materiale è quello di prima (non determinabile, namespace_non_confrontabile): la
// regola si accende solo con la grammatica.
func TestLEquivalenzaSoloConLaForma(t *testing.T) {
	m := motoreRis(t)
	step, ctx, tg := scenaEq(t, m)
	figlio := pdfEq(t, m, 0xa01, "disegno-figlio.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"})
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, figlio}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if got := collocazioneEq(t, e, figlio.AllegatoID); got != "non_determinabile [namespace_non_confrontabile]" {
		t.Errorf("senza la forma, il 2D del figlio: %s", got)
	}
	if got := nodiConEquivalente(e); len(got) != 0 {
		t.Errorf("senza la forma nessun nodo ha il codice della parte dal cartiglio: %v", got)
	}
}

// TestLEquivalenzaDeiGemelli (ii): il DXF con il cartiglio (il codice e il riferimento 3D) e il PDF dello stesso disegno
// senza testo, letto solo dal nome. La tabella è del thread, per chiave del codice: il PDF prende il candidato solo se
// il suo nome dice lo stesso codice della parte (stesso namespace, base e marcatore), con la prova del DXF; un PDF il
// cui nome dice un altro codice non prende niente (nessuna somiglianza di nomi, E-18). Il ripiego sul nome: un DXF il
// cui cartiglio dice solo il riferimento 3D prende il codice dal nome dello stesso file (fonte nome_file).
func TestLEquivalenzaDeiGemelli(t *testing.T) {
	m := motoreEq(t)
	step, ctx, tg := scenaEq(t, m)
	dxf := dxfEq(t, m, 0xa11, "disegno-gemello.dxf", []campoP{{"codice", "210000002"}, {"riferimento_3d", "101000002_01"}, {"revisione", "00"}})
	gemello := pdfEq(t, m, 0xa12, "210000002.pdf")
	estraneo := pdfEq(t, m, 0xa13, "210000003.pdf")
	if gemello.Disponibilita != ancoraggio.DisponibilitaSenzaTesto {
		t.Fatalf("il PDF gemello è senza testo: %s", gemello.Disponibilita)
	}
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, dxf, gemello, estraneo}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	cd := candidatoEq(t, e, dxf.AllegatoID)
	cp := candidatoEq(t, e, gemello.AllegatoID)
	if len(cd.Equivalenze) != 1 || cd.Equivalenze[0].Entita != "e:dxf:cartiglio" || cd.Equivalenze[0].AllegatoID != dxf.AllegatoID ||
		cd.Equivalenze[0].Fonte != ancoraggio.FonteEquivalenzaCartiglio || !reflect.DeepEqual(cp.Equivalenze, cd.Equivalenze) {
		t.Errorf("la prova del DXF, la stessa per il gemello: %+v %+v", cd.Equivalenze, cp.Equivalenze)
	}
	if got := collocazioneEq(t, e, estraneo.AllegatoID); got != "non_determinabile [namespace_non_confrontabile]" {
		t.Errorf("il PDF con un altro codice nel nome: %s", got)
	}

	soloRif := dxfEq(t, m, 0xa14, "210000002_D42.dxf", []campoP{{"riferimento_3d", "101000002_01"}})
	e = ancoraA(t, []ancoraggio.FileInterpretato{step, soloRif}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	c := candidatoEq(t, e, soloRif.AllegatoID)
	if len(c.Equivalenze) != 1 || c.Equivalenze[0].Fonte != ancoraggio.FonteEquivalenzaNomeFile || c.Equivalenze[0].Entita != "e:dxf:cartiglio" {
		t.Errorf("il ripiego sul nome dello stesso file: %+v", c.Equivalenze)
	}
}

// TestLeEquivalenzeConteseNonPropongono (iii): uno stesso riferimento 3D accanto a due codici della parte, uno stesso
// codice accanto a due riferimenti 3D, un cartiglio con due codici: nessun candidato (i 2D restano non determinabili con
// namespace_non_confrontabile, come senza la regola) e l'avviso ancoraggio.equivalenza_cartiglio_contesa, con le
// identità e gli allegati fra i Rif.
func TestLeEquivalenzeConteseNonPropongono(t *testing.T) {
	m := motoreEq(t)
	step, ctx, tg := scenaEq(t, m)
	for _, c := range []struct {
		nome string
		file []ancoraggio.FileInterpretato
		rif  []string // Rif attesi nella diagnostica, oltre agli allegati
	}{
		{"lo stesso riferimento per due codici", []ancoraggio.FileInterpretato{
			pdfEq(t, m, 0xa21, "a.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"}),
			pdfEq(t, m, 0xa22, "b.pdf", campoP{"codice", "210000003"}, campoP{"riferimento_3d", "101000002_01"}),
		}, []string{"identita:acme-doc:101000002_01::", "identita:acme:210000002::", "identita:acme:210000003::"}},
		{"lo stesso codice per due riferimenti", []ancoraggio.FileInterpretato{
			pdfEq(t, m, 0xa23, "c.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000003_01"}),
			dxfEq(t, m, 0xa24, "d.dxf", []campoP{{"codice", "210000002"}, {"riferimento_3d", "101000002_01"}}),
		}, []string{"identita:acme:210000002::", "identita:acme-doc:101000002_01::", "identita:acme-doc:101000003_01::"}},
		{"due codici in un cartiglio", []ancoraggio.FileInterpretato{
			pdfEq(t, m, 0xa25, "e.pdf", campoP{"codice", "210000002"}, campoP{"codice", "210000003"}, campoP{"riferimento_3d", "101000002_01"}),
		}, []string{"identita:acme:210000002::", "identita:acme:210000003::", "identita:acme-doc:101000002_01::"}},
		// revisione del 14/10: il cartiglio conteso in sé toglie anche la coppia di un secondo cartiglio coerente
		{"un cartiglio conteso e uno coerente", []ancoraggio.FileInterpretato{
			pdfEq(t, m, 0xa26, "f.pdf", campoP{"codice", "210000002"}, campoP{"codice", "210000003"}, campoP{"riferimento_3d", "101000002_01"}),
			pdfEq(t, m, 0xa27, "g.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"}),
		}, []string{"identita:acme:210000002::", "identita:acme:210000003::", "identita:acme-doc:101000002_01::"}},
		{"un cartiglio con due riferimenti e uno coerente", []ancoraggio.FileInterpretato{
			pdfEq(t, m, 0xa28, "h.pdf", campoP{"codice", "210000003"}, campoP{"riferimento_3d", "101000002_01"}, campoP{"riferimento_3d", "101000003_01"}),
			pdfEq(t, m, 0xa29, "i.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"}),
		}, []string{"identita:acme:210000003::", "identita:acme-doc:101000002_01::", "identita:acme-doc:101000003_01::"}},
	} {
		t.Run(c.nome, func(t *testing.T) {
			e := ancoraA(t, append([]ancoraggio.FileInterpretato{step}, c.file...), []ancoraggio.ProdottoRichiesto{tg}, ctx)
			// Con due codici nel cartiglio il file ha anche identità discordanti (R64 A): resta così.
			for _, f := range c.file {
				if a := ancoraggioDi(t, e, f.AllegatoID); len(a.Candidati) != 0 || a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
					!contiene(a.Motivi, ancoraggio.MotivoAncoraggioNamespaceNonConfrontabile) {
					t.Errorf("%s: nessun candidato per una contesa: %+v", f.AllegatoID, a)
				}
			}
			for _, x := range nodiConEquivalente(e) {
				if x[:len(ancoraggio.RifNodo(shaStepR, "#2"))] == ancoraggio.RifNodo(shaStepR, "#2") {
					t.Errorf("una contesa non dà il codice della parte al nodo #2: %v", x)
				}
			}
			d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa)
			if len(d) == 0 {
				t.Fatalf("la diagnostica della contesa manca: %+v", e.Diagnostiche)
			}
			var rif []string
			for _, x := range d {
				if x.Gravita != "avviso" || x.Natura != "dati" || x.Messaggio == "" {
					t.Errorf("la diagnostica: %+v", x)
				}
				rif = append(rif, x.Rif...)
			}
			attesi := append([]string(nil), c.rif...)
			for _, f := range c.file {
				attesi = append(attesi, "allegato:"+f.AllegatoID.String())
			}
			for _, r := range attesi {
				if !contiene(rif, r) {
					t.Errorf("la diagnostica non nomina %s: %+v", r, d)
				}
			}
		})
	}
}

// TestLEquivalenzaNonDipendeDallOrdine (iv): la scena dei gemelli e di una contesa con gli ingressi rovesciati (i file,
// le letture, le strutture con i nodi e il nome) dà lo stesso esito canonico.
func TestLEquivalenzaNonDipendeDallOrdine(t *testing.T) {
	m := motoreEq(t)
	step, ctx, tg := scenaEq(t, m)
	file := []ancoraggio.FileInterpretato{step,
		dxfEq(t, m, 0xa31, "disegno-gemello.dxf", []campoP{{"codice", "210000002"}, {"riferimento_3d", "101000002_01"}}),
		pdfEq(t, m, 0xa32, "210000002.pdf"),
		pdfEq(t, m, 0xa33, "disegno.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"}),
		pdfEq(t, m, 0xa34, "x.pdf", campoP{"codice", "210000004"}, campoP{"riferimento_3d", "101000004_01"}),
		pdfEq(t, m, 0xa35, "y.pdf", campoP{"codice", "210000005"}, campoP{"riferimento_3d", "101000004_01"}),
	}
	e := ancoraA(t, file, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if c := candidatoEq(t, e, uidS(0xa33)); len(c.Equivalenze) != 2 {
		t.Errorf("due prove, il DXF e il PDF con il cartiglio: %+v", c.Equivalenze)
	}
	rov := ctx
	rov.Strutture = rovescia(ctx.Strutture)
	for i := range rov.Strutture {
		rov.Strutture[i].Nodi = rovescia(rov.Strutture[i].Nodi)
		rov.Strutture[i].NomeFile = rovescia(rov.Strutture[i].NomeFile)
	}
	fileRov := rovescia(file)
	for i := range fileRov {
		fileRov[i].Interpretazione.Letture = rovescia(fileRov[i].Interpretazione.Letture)
	}
	if b := ancoraA(t, fileRov, []ancoraggio.ProdottoRichiesto{tg}, rov); canonico(t, e) != canonico(t, b) {
		t.Errorf("le equivalenze del cartiglio dipendono dall'ordine degli ingressi")
	}
}

// TestIlRipiegoSulNomeTogliIlNamespaceDelRiferimento: il nome di un 2D vero dice più identità, il materiale e il
// documento 2D («210000002_D42_101000007_01.dxf», come «210000080_D42_100000001_000_00.dxf»). Quando il cartiglio dice
// solo il riferimento 3D, il ripiego sul nome toglie le letture nel namespace del riferimento (il documento 2D): resta
// il materiale, e il DXF è il candidato sul nodo #2, senza l'avviso della contesa. Con due materiali nel nome il
// ripiego non sceglie: nessun candidato per equivalenza, e nessuna contesa (più identità in un nome non lo sono).
func TestIlRipiegoSulNomeTogliIlNamespaceDelRiferimento(t *testing.T) {
	m := motoreEq(t)
	step, ctx, tg := scenaEq(t, m)
	vero := dxfEq(t, m, 0xa41, "210000002_D42_101000007_01.dxf", []campoP{{"riferimento_3d", "101000002_01"}})
	ns := map[string]bool{}
	for _, l := range vero.Interpretazione.Letture {
		if l.Funzione == motorea.FunzIdentitaFile {
			ns[l.Forma.Namespace] = true
		}
	}
	if !ns["acme"] || !ns["acme-doc"] {
		t.Fatalf("il nome dice il materiale e il documento 2D: %+v", vero.Interpretazione.Letture)
	}
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, vero}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	c := candidatoEq(t, e, vero.AllegatoID)
	if len(c.Equivalenze) != 1 || c.Equivalenze[0].Fonte != ancoraggio.FonteEquivalenzaNomeFile {
		t.Errorf("il ripiego sul nome, con il solo materiale: %+v", c.Equivalenze)
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa); len(d) != 0 {
		t.Errorf("il materiale e il documento 2D nel nome non sono una contesa: %+v", d)
	}

	due := dxfEq(t, m, 0xa42, "210000002_210000003_D42.dxf", []campoP{{"riferimento_3d", "101000002_01"}})
	e = ancoraA(t, []ancoraggio.FileInterpretato{step, due}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	for _, x := range ancoraggioDi(t, e, due.AllegatoID).Candidati {
		if x.Motivo == ancoraggio.MotivoCandidatoEquivalenzaCartiglio || len(x.Equivalenze) != 0 {
			t.Errorf("due materiali nel nome: nessun candidato per equivalenza: %+v", x)
		}
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa); len(d) != 0 {
		t.Errorf("due identità nel nome non sono una contesa: %+v", d)
	}
}
