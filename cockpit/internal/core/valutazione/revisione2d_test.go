// L1 — la revisione del documento (B5, fase 2; R104, E1R §5; T-E1R-05…T-E1R-09; PO-38 per intero): la proposta
// (cartiglio leggibile → STEP dell'entità → nome del file → assente), sulla regola e dalla fotografia; il 2D di un figlio
// che non prende la revisione della radice; entita_non_univoca; il cartiglio leggibile senza revisione; documento.rev
// registrata con la provenienza, fuori dalla proposta e mai un conflitto; la decisione sul documento con le evidenze
// viste e il conflitto identita_documento contro un'evidenza nuova; le discordanze sempre calcolate; il primario con la
// revisione confermata o le evidenze proprie, mai la proposta dallo STEP.
package valutazione_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// scenaRevisioni: la scena dell'albero con la famiglia che legge la revisione negli id dello STEP: la radice 7120100A1
// (il prodotto, revisione 1) e il figlio 7120200A3 (lo sciolto, revisione 3, con la formazione «B»), confermati con
// «Conferma l'albero»; la fonte è confermata.
func scenaRevisioni(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", "7120100A1", ""}, {"#2", "7120200A3", "B"}}, []arcoF{{"#1", "#2", 2}})
	confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
	return th
}

func motoreCatenaRev(t *testing.T) *motorea.Motore {
	t.Helper()
	return motoreConFamiglia(t, famigliaCatenaRev())
}

// conDocumento2D: un documento disegno_2d confermato sul componente, con il suo allegato e i fatti dati.
func conDocumento2D(th *fotorfq.Thread, doc, comp, all uuid.UUID, nome, s string, f *fotorfq.Fatti) {
	d := documento2D(doc, comp, all, nome, "pdf", s)
	conFile2D(th, allegato(all, int16(20+len(th.Allegati)), nome, "pdf", s), f, &d, uuid.Nil)
}

// scansione: i fatti di un PDF scansionato senza OCR (nessun cartiglio).
func scansione(t *testing.T, s string) *fotorfq.Fatti {
	t.Helper()
	f := fatti(t, s, payloadPDF(t, false), nil)
	return &f
}

// conCartiglio: i fatti di un PDF con il testo e i campi del cartiglio dati.
func conCartiglio(t *testing.T, s string, campi ...[2]string) *fotorfq.Fatti {
	t.Helper()
	f := fattiPDFCampi(t, s, campi...)
	return &f
}

// proposta: la revisione proposta come testo «valore/fonte/entità/motivo», per i confronti.
func proposta(p valutazione.RevisioneProposta) string {
	return val(p.Valore) + "/" + p.Fonte + "/" + p.Entita + "/" + p.Motivo
}

// ---- la proposta ----

// TestPO38LaPropostaSullaRegola (PO-38, E1R §5.2; T-E1R-05, T-E1R-07): la prima fonte con un valore interpretabile, con
// il motivo della fonte prima; assente con il motivo del nome (letto senza una revisione interpretabile:
// nome_non_interpretabile; non letto: nessuna_fonte); documento.rev non entra mai; una fonte che manca vale come una
// fonte senza valore.
func TestPO38LaPropostaSullaRegola(t *testing.T) {
	ev := func(fonte, valore string, interpretabile bool, entita, motivo string) valutazione.EvidenzaRevisione {
		e := valutazione.EvidenzaRevisione{Fonte: fonte, Interpretabile: interpretabile, Entita: entita, Motivo: motivo}
		if valore != "" {
			e.Valore = testo(valore)
		}
		return e
	}
	cart := ev(valutazione.FonteEvidenzaCartiglio, "2", true, "", "")
	senzaCart := ev(valutazione.FonteEvidenzaCartiglio, "", false, "", valutazione.MotivoEvidenzaCartiglioNonLeggibile)
	cartNonInterpretabile := ev(valutazione.FonteEvidenzaCartiglio, "X", false, "", valutazione.MotivoEvidenzaRevisioneNonInterpretabile)
	step := ev(valutazione.FonteEvidenzaStepEntita, "3", true, nodoB("#2"), "")
	stepNonUnivoca := ev(valutazione.FonteEvidenzaStepEntita, "", false, "", valutazione.MotivoEvidenzaEntitaNonUnivoca)
	stepSenzaRev := ev(valutazione.FonteEvidenzaStepEntita, "", false, nodoB("#2"), valutazione.MotivoEvidenzaRevisioneNonDeterminata)
	stepAssente := ev(valutazione.FonteEvidenzaStepEntita, "", false, "", valutazione.MotivoEvidenzaEntitaAssente)
	nome := ev(valutazione.FonteEvidenzaNomeFile, "4", true, "", "")
	senzaNome := ev(valutazione.FonteEvidenzaNomeFile, "", false, "", valutazione.MotivoEvidenzaNomeNonLetto)
	nomeSenzaRev := ev(valutazione.FonteEvidenzaNomeFile, "", false, "", valutazione.MotivoEvidenzaRevisioneAssenteNome)
	nomeNonInterpretabile := ev(valutazione.FonteEvidenzaNomeFile, "X", false, "", valutazione.MotivoEvidenzaRevisioneNonInterpretabile)
	doc := ev(valutazione.FonteEvidenzaDocumento, "5", true, "", "")
	for _, c := range []struct {
		nome   string
		ev     []valutazione.EvidenzaRevisione
		attesa string
	}{
		{"il cartiglio leggibile", []valutazione.EvidenzaRevisione{cart, step, nome, doc}, "2/cartiglio//"},
		{"senza cartiglio, l'unico nodo", []valutazione.EvidenzaRevisione{senzaCart, step, nome, doc}, "3/step_entita/" + nodoB("#2") + "/cartiglio_non_leggibile"},
		{"un cartiglio che non si interpreta non inventa niente", []valutazione.EvidenzaRevisione{cartNonInterpretabile, step}, "3/step_entita/" + nodoB("#2") + "/cartiglio_non_leggibile"},
		{"entita_non_univoca, poi il nome", []valutazione.EvidenzaRevisione{senzaCart, stepNonUnivoca, nome}, "4/nome_file//entita_non_univoca"},
		{"il nodo senza revisione, poi il nome", []valutazione.EvidenzaRevisione{senzaCart, stepSenzaRev, nome}, "4/nome_file//revisione_non_determinata"},
		{"nessun nodo, poi il nome", []valutazione.EvidenzaRevisione{senzaCart, stepAssente, nome}, "4/nome_file//revisione_non_determinata"},
		{"documento.rev non è una quarta fonte", []valutazione.EvidenzaRevisione{senzaCart, stepAssente, senzaNome, doc}, "<nil>/assente//nessuna_fonte"},
		{"il nome letto senza revisione", []valutazione.EvidenzaRevisione{senzaCart, stepSenzaRev, nomeSenzaRev}, "<nil>/assente//nome_non_interpretabile"},
		{"il nome con una revisione che non si interpreta", []valutazione.EvidenzaRevisione{senzaCart, stepAssente, nomeNonInterpretabile}, "<nil>/assente//nome_non_interpretabile"},
		{"nessuna evidenza", nil, "<nil>/assente//nessuna_fonte"},
		{"solo il nome, le altre fonti mancano", []valutazione.EvidenzaRevisione{nome}, "4/nome_file//revisione_non_determinata"},
		{"la prima evidenza di una fonte", []valutazione.EvidenzaRevisione{cart, ev(valutazione.FonteEvidenzaCartiglio, "9", true, "", "")}, "2/cartiglio//"},
	} {
		if p := valutazione.PropostaDiRevisione(c.ev); proposta(p) != c.attesa {
			t.Errorf("%s: %s, attesa %s", c.nome, proposta(p), c.attesa)
		}
	}
}

// TestPO38LaPropostaDallaFotografia (PO-38; E1R §5.2; T-E1R-05, T-B5-95): dalla fotografia, sullo STEP con la radice
// alla revisione 1 e il figlio alla 3:
//   - il cartiglio leggibile dà la proposta;
//   - senza cartiglio il 2D dello sciolto prende la revisione interpretata del suo nodo (3), mai quella della radice (1),
//     che va solo al 2D del prodotto;
//   - lo sciolto su due nodi di revisione diversa: entita_non_univoca, poi il nome del file;
//   - un candidato unico prende il nodo del suo candidato; uno con identità discordanti, entita_non_univoca; un candidato
//     di un file con un'associazione decisa prende i nodi di quella;
//   - un cartiglio leggibile senza revisione è un'evidenza senza valore, e la proposta passa allo STEP dell'entità;
//   - nessuna fonte interpretabile: assente, nessuna_fonte.
func TestPO38LaPropostaDallaFotografia(t *testing.T) {
	m := motoreCatenaRev(t)
	unico := func(t *testing.T, th fotorfq.Thread, comp uuid.UUID) valutazione.Disegno2D {
		t.Helper()
		g := gruppoDi(t, valuta(t, th, m, nil), comp)
		if len(tuttiDi(g)) != 1 {
			t.Fatalf("gruppo %+v", tuttiDi(g))
		}
		return *g.Primario
	}
	t.Run("il cartiglio leggibile", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", "7120200A2"}))
		d := unico(t, th, cSciolto)
		if proposta(d.RevisioneProposta) != "2/cartiglio//" || d.StatoRevisione != valutazione.StatoRevisione2DProposta {
			t.Errorf("proposta %s, stato %s", proposta(d.RevisioneProposta), d.StatoRevisione)
		}
		want := []valutazione.EvidenzaRevisione{
			{Fonte: valutazione.FonteEvidenzaCartiglio, Valore: testo("2"), Interpretabile: true},
			{Fonte: valutazione.FonteEvidenzaStepEntita, Valore: testo("3"), Entita: nodoB("#2"), Interpretabile: true},
			{Fonte: valutazione.FonteEvidenzaNomeFile, Motivo: valutazione.MotivoEvidenzaNomeNonLetto},
			{Fonte: valutazione.FonteEvidenzaDocumento, Motivo: valutazione.MotivoEvidenzaRevisioneNonRegistrata},
		}
		if !reflect.DeepEqual(d.EvidenzeRevisione, want) {
			t.Errorf("evidenze %+v", d.EvidenzeRevisione)
		}
		if d.Codice != "7120200A2" || d.FonteIdentita != valutazione.FonteIdentitaCartiglio || val(d.Revisione) != "2" || d.CompatibilitaCodice != motorea.CompatibilitaUguale {
			t.Errorf("identità %q %s %s, codice %s", d.Codice, d.FonteIdentita, val(d.Revisione), d.CompatibilitaCodice)
		}
	})
	t.Run("il 2D del figlio non prende la revisione della radice", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		conDocumento2D(&th, dPDF2, cProdotto, aPDF2, "assieme-acme.pdf", sha2, scansione(t, sha2))
		v := valuta(t, th, m, nil)
		figlio, padre := *gruppoDi(t, v, cSciolto).Primario, *gruppoDi(t, v, cProdotto).Primario
		if proposta(figlio.RevisioneProposta) != "3/step_entita/"+nodoB("#2")+"/cartiglio_non_leggibile" {
			t.Errorf("il 2D dello sciolto: %s", proposta(figlio.RevisioneProposta))
		}
		if proposta(padre.RevisioneProposta) != "1/step_entita/"+nodoB("#1")+"/cartiglio_non_leggibile" {
			t.Errorf("il 2D del prodotto: %s", proposta(padre.RevisioneProposta))
		}
		if figlio.Revisione != nil || figlio.FonteIdentita != "" || figlio.CompatibilitaRevisione != motorea.CompatibilitaNonDeterminabile {
			t.Errorf("la proposta dallo STEP non è la revisione del 2D (T-E1R-06): %s %q %s", val(figlio.Revisione), figlio.FonteIdentita, figlio.CompatibilitaRevisione)
		}
	})
	t.Run("lo sciolto su due nodi di revisione diversa", func(t *testing.T) {
		th := scenaRevisioni(t)
		aggiungiNodo(t, &th, "#3", "7120200A4")
		op := operatore
		decidi(t, &th, "#3", "confermata", cSciolto, &op)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "7120200A_4.pdf", sha1, scansione(t, sha1))
		d := unico(t, th, cSciolto)
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaStepEntita); e.Motivo != valutazione.MotivoEvidenzaEntitaNonUnivoca || e.Valore != nil || e.Entita != "" {
			t.Errorf("lo STEP %+v", e)
		}
		if proposta(d.RevisioneProposta) != "4/nome_file//entita_non_univoca" || d.ConfrontatoCon != valutazione.ConfrontatoConNessuno {
			t.Errorf("proposta %s, confrontato con %s", proposta(d.RevisioneProposta), d.ConfrontatoCon)
		}
	})
	t.Run("il candidato unico", func(t *testing.T) {
		th := scenaRevisioni(t)
		conFile2D(&th, allegato(aPDF1, 3, "7120200A_3.pdf", "pdf", sha1), scansione(t, sha1), nil, uuid.Nil)
		d := unico(t, th, cSciolto)
		if d.Provenienza != ancoraggio.OrigineProposto || proposta(d.RevisioneProposta) != "3/step_entita/"+nodoB("#2")+"/cartiglio_non_leggibile" {
			t.Errorf("provenienza %s, proposta %s", d.Provenienza, proposta(d.RevisioneProposta))
		}
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaNomeFile); val(e.Valore) != "3" || !e.Interpretabile {
			t.Errorf("il nome %+v", e)
		}
	})
	t.Run("il candidato con identità discordanti", func(t *testing.T) {
		th := scenaRevisioni(t)
		conFile2D(&th, allegato(aPDF1, 3, "7120200A_3.pdf", "pdf", sha1), conCartiglio(t, sha1, [2]string{"codice", "7120300A1"}), nil, uuid.Nil)
		v := valuta(t, th, m, nil)
		if a := ancoraggioDel(t, v, aPDF1); a.Associazione != ancoraggio.AssociazioneDiscordante {
			t.Fatalf("associazione %s", a.Associazione)
		}
		d := *gruppoDi(t, v, cSciolto).Primario
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaStepEntita); e.Motivo != valutazione.MotivoEvidenzaEntitaNonUnivoca {
			t.Errorf("lo STEP %+v", e)
		}
		if proposta(d.RevisioneProposta) != "1/cartiglio//" {
			t.Errorf("proposta %s", proposta(d.RevisioneProposta))
		}
	})
	t.Run("il candidato di un file deciso su un altro componente", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "7120200A_3.pdf", sha1, scansione(t, sha1))
		d := unico(t, th, cSciolto)
		if d.Provenienza != ancoraggio.OrigineProposto || proposta(d.RevisioneProposta) != "1/step_entita/"+nodoB("#1")+"/cartiglio_non_leggibile" {
			t.Errorf("provenienza %s, proposta %s: il nodo dell'associazione decisa", d.Provenienza, proposta(d.RevisioneProposta))
		}
	})
	t.Run("il cartiglio leggibile senza revisione", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", "7120200A"}, [2]string{"titolo", "STAFFA"}))
		d := unico(t, th, cSciolto)
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaCartiglio); d.Cartiglio != valutazione.CartiglioLetto || e.Valore != nil || e.Interpretabile ||
			e.Motivo != valutazione.MotivoEvidenzaRevisioneAssenteCartiglio {
			t.Errorf("cartiglio %s, evidenza %+v", d.Cartiglio, e)
		}
		if proposta(d.RevisioneProposta) != "3/step_entita/"+nodoB("#2")+"/cartiglio_non_leggibile" {
			t.Errorf("proposta %s", proposta(d.RevisioneProposta))
		}
	})
	t.Run("la revisione nel campo a sé del cartiglio", func(t *testing.T) {
		for _, c := range []struct {
			codice, rev, valore string
			interpretabile      bool
			motivo              string
		}{
			{"7120200A", "5", "5", false, valutazione.MotivoEvidenzaRevisioneNonInterpretabile},
			{"7120200A2", "2", "2", true, ""},
			{"7120200A2", "C", "<nil>", false, valutazione.MotivoEvidenzaRevisioneNonInterpretabile},
		} {
			th := scenaRevisioni(t)
			conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", c.codice}, [2]string{"revisione", c.rev}))
			e := evidenzaDi(t, unico(t, th, cSciolto), valutazione.FonteEvidenzaCartiglio)
			if val(e.Valore) != c.valore || e.Interpretabile != c.interpretabile || e.Motivo != c.motivo {
				t.Errorf("codice %s, revisione %s: %+v", c.codice, c.rev, e)
			}
		}
	})
	t.Run("nessuna fonte interpretabile", func(t *testing.T) {
		th := scenaAlbero(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		d := gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto).Primario
		if proposta(d.RevisioneProposta) != "<nil>/assente//nessuna_fonte" || d.StatoRevisione != valutazione.StatoRevisione2DAssente {
			t.Errorf("proposta %s, stato %s", proposta(d.RevisioneProposta), d.StatoRevisione)
		}
		if e := evidenzaDi(t, *d, valutazione.FonteEvidenzaStepEntita); e.Motivo != valutazione.MotivoEvidenzaRevisioneNonDeterminata || e.Entita != nodoB("#2") {
			t.Errorf("lo STEP %+v", e)
		}
	})
}

// ---- la revisione registrata (T-E1R-07) ----

// TestPO38LaRevisioneRegistrata (PO-38; E1R §5.5; T-E1R-07, T-E1-22): documento.rev diverso dal cartiglio è registrata,
// con la discordanza proposta_registrata come indicatore, e nessun conflitto; la provenienza coincide_con_formazione_step
// quando è la formazione del nodo dello sciolto; documento.rev non entra nella proposta, ed è l'ultima risorsa della
// compatibilità; la rev_diversa della vista è una discordanza con CalcolataDa = vista.
func TestPO38LaRevisioneRegistrata(t *testing.T) {
	m := motoreCatenaRev(t)
	t.Run("diversa dal cartiglio: un indicatore", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, conCartiglio(t, sha1, [2]string{"codice", "7120200A2"}))
		th.Documenti[len(th.Documenti)-1].Rev = testo(" 5 ")
		v := valuta(t, th, m, nil)
		d := *gruppoDi(t, v, cSciolto).Primario
		if val(d.Registrata) != "5" || d.RevProvenienza != ancoraggio.RevProvenienzaNonRegistrata || d.StatoRevisione != valutazione.StatoRevisione2DRegistrata ||
			d.FonteIdentita != valutazione.FonteIdentitaCartiglio || proposta(d.RevisioneProposta) != "2/cartiglio//" {
			t.Errorf("registrata %s %s, stato %s, fonte %s, proposta %s", val(d.Registrata), d.RevProvenienza, d.StatoRevisione, d.FonteIdentita, proposta(d.RevisioneProposta))
		}
		reg := valutazione.DiscordanzaRevisione{Tra: valutazione.TraPropostaRegistrata, A: "2", B: "5", FonteA: valutazione.FonteEvidenzaCartiglio,
			FonteB: valutazione.FonteEvidenzaDocumento, Effetto: valutazione.EffettoIndicatore, CalcolataDa: valutazione.CalcolataDaGo}
		comp := valutazione.DiscordanzaRevisione{Tra: valutazione.TraDocumentoComponente, A: "2", B: "3", FonteA: valutazione.FonteIdentitaCartiglio,
			FonteB: valutazione.FonteComponenteStepEntita, Effetto: valutazione.EffettoIndicatore, CalcolataDa: valutazione.CalcolataDaGo}
		if !reflect.DeepEqual(d.Discordanze, []valutazione.DiscordanzaRevisione{comp, reg}) {
			t.Errorf("discordanze %+v", d.Discordanze)
		}
		for _, c := range v.Conflitti {
			if c.Tipo == valutazione.ConflittoIdentitaDocumento {
				t.Errorf("documento.rev non è mai un conflitto: %+v", c)
			}
		}
	})
	t.Run("la formazione del nodo dello sciolto", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		th.Documenti[len(th.Documenti)-1].Rev = testo("B")
		d := *gruppoDi(t, valuta(t, th, m, nil), cSciolto).Primario
		if d.RevProvenienza != ancoraggio.RevProvenienzaFormazioneSTEP {
			t.Errorf("provenienza %s", d.RevProvenienza)
		}
	})
	t.Run("fuori dalla proposta, ultima risorsa della compatibilità", func(t *testing.T) {
		th := scenaAlbero(t)
		th.Componenti[1].Rev = testo("5")
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		th.Documenti[len(th.Documenti)-1].Rev, th.Documenti[len(th.Documenti)-1].Codice = testo("5"), testo("7120200A")
		d := *gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto).Primario
		if proposta(d.RevisioneProposta) != "<nil>/assente//nessuna_fonte" || d.StatoRevisione != valutazione.StatoRevisione2DRegistrata ||
			d.FonteIdentita != valutazione.FonteIdentitaDocumento || val(d.Revisione) != "5" || d.CompatibilitaRevisione != motorea.CompatibilitaUguale ||
			d.ConfrontatoCon != valutazione.ConfrontatoConDeciso || d.Codice != "7120200A" || d.CompatibilitaCodice != motorea.CompatibilitaUguale {
			t.Errorf("%+v", d)
		}
		if e := evidenzaDi(t, d, valutazione.FonteEvidenzaDocumento); val(e.Valore) != "5" || !e.Interpretabile {
			t.Errorf("l'evidenza del documento %+v", e)
		}
	})
	t.Run("la rev_diversa della vista", func(t *testing.T) {
		th := scenaAlbero(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		dd, altro := dPDF1, dPDF2
		th.Fascicolo = []fotorfq.RigaFascicolo{
			{ComponenteID: cSciolto, TipoComponente: "sciolto", TipoDocumento: "disegno_2d", DocumentoID: &altro, RevDiversa: true, DocumentoRev: testo("7"), Rev: testo("4")},
			{ComponenteID: cSciolto, TipoComponente: "sciolto", TipoDocumento: "cad_3d", DocumentoID: &dd, RevDiversa: true, DocumentoRev: testo("6"), Rev: testo("4")},
			{ComponenteID: cSciolto, TipoComponente: "sciolto", TipoDocumento: "disegno_2d", DocumentoID: &dd, RevDiversa: true, DocumentoRev: testo(" 5"), Rev: testo("4 ")},
		}
		d := *gruppoDi(t, valuta(t, th, motoreCatena(t), nil), cSciolto).Primario
		want := []valutazione.DiscordanzaRevisione{{Tra: valutazione.TraDocumentoComponente, A: "5", B: "4", FonteA: valutazione.FonteEvidenzaDocumento,
			FonteB: valutazione.FonteComponenteRegistrata, Effetto: valutazione.EffettoIndicatore, CalcolataDa: valutazione.CalcolataDaVista}}
		if !reflect.DeepEqual(d.Discordanze, want) {
			t.Errorf("discordanze %+v", d.Discordanze)
		}
	})
}

// ---- la decisione sul documento (T-E1R-08) ----

// fonteSintetica: una fonte d'identità per le prove della regola: la coppia dal testo grezzo (vuoto: nessuna), la
// revisione interpretabile (vuota: nessuna), la lettura del codice e l'entità date.
func fonteSintetica(fonte, grezzo, rev string, lettura *motorea.LetturaForma, entita string) valutazione.FonteDelDisegno {
	aa := aPDF1
	f := valutazione.FonteDelDisegno{Revisione: valutazione.EvidenzaRevisione{Fonte: fonte, Entita: entita, Motivo: "senza_valore"}, Lettura: lettura}
	if grezzo != "" {
		f.Coppia = ancoraggio.EvidenzaDa(fonte, grezzo)
	}
	if rev != "" {
		f.Revisione.Valore, f.Revisione.Interpretabile, f.Revisione.Motivo = testo(rev), true, ""
	}
	if lettura != nil {
		f.Codice = grezzo
	}
	if fonte == valutazione.FonteEvidenzaCartiglio {
		f.AllegatoID, f.UnitaID = &aa, "u:pdf:cartiglio:codice:1"
	}
	return f
}

// pdfDeciso: l'ingresso astratto del PDF confermato sullo sciolto con le fonti date. È la forma che l'adattatore
// prepara dalla fotografia; la decisione sul documento non ha un adattatore in A1c (LD-27), quindi la prova la aggiunge
// a mano.
func pdfDeciso(fonti ...valutazione.FonteDelDisegno) valutazione.DisegnoDaValutare {
	dd, aa := dPDF1, aPDF1
	return valutazione.DisegnoDaValutare{Fonti: fonti, Disegno: valutazione.Disegno2D{DocumentoID: &dd, AllegatoID: &aa, Sha256: sha1, NomeFile: "disegno-acme.pdf",
		Formato: valutazione.Formato2DPDF, Estensione: "pdf", Validita: valutazione.ValiditaValido, Cartiglio: valutazione.CartiglioLetto,
		Provenienza: ancoraggio.OrigineConfermato, Corrente: true}}
}

// decisioneDoc: la decisione sul PDF dello sciolto, con il codice 7120200A, la revisione e le evidenze viste date.
func decisioneDoc(rev string, viste ...ancoraggio.EvidenzaVista) *ancoraggio.DecisioneIdentita {
	return &ancoraggio.DecisioneIdentita{Oggetto: ancoraggio.OggettoDecisioneDocumento, ID: dPDF1, Codice: " 7120200A ", Revisione: rev, EvidenzeViste: viste,
		Da: operatore, Il: dataACME.Add(5*time.Hour + 123456789)}
}

// TestPO38LaDecisioneSulDocumento (PO-38; E1R §5.3; T-E1R-08, R95 A): una DecisioneIdentita sul documento, corretta
// contro il cartiglio che l'operatore aveva davanti: nessun conflitto, la revisione confermata, lo stato confermata, la
// discordanza con la proposta come indicatore; poi un cartiglio diverso da quelli visti: il conflitto identita_documento
// sullo smistamento, la parte nel motivo, le evidenze dei due lati, e la decisione resta. Sul codice (un'altra base, un
// altro marcatore), per prodotto, senza revisione decisa, dallo STEP dell'entità e dal nome del file; mai da
// documento.rev né da una fonte non interpretabile; una decisione su un altro oggetto non vale; senza la lettura del
// codice deciso la parte codice non si giudica.
func TestPO38LaDecisioneSulDocumento(t *testing.T) {
	m := motoreCatenaRev(t)
	ld := letturaDi(t, m, "nodo_step.id", "7120200A")
	ls := ld
	comp := valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Lettura: &ls, Revisione: testo("3"), FonteRevisione: valutazione.FonteComponenteStepEntita,
		Confronto: valutazione.ConfrontatoConProposto}
	cart := func(codice string) valutazione.FonteDelDisegno {
		l := letturaDi(t, m, "cartiglio.codice", codice)
		return fonteSintetica(valutazione.FonteEvidenzaCartiglio, codice, l.Revisione.Normalizzata, &l, "")
	}
	nome := func(n string) valutazione.FonteDelDisegno {
		l := letturaDi(t, m, "nome_file", n)
		return fonteSintetica(valutazione.FonteEvidenzaNomeFile, n, l.Revisione.Normalizzata, &l, "")
	}
	step := func(grezzo, rev string) valutazione.FonteDelDisegno {
		return fonteSintetica(valutazione.FonteEvidenzaStepEntita, grezzo, rev, nil, nodoB("#2"))
	}
	vista := ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120200A2")
	valuta2D := func(in valutazione.DisegnoDaValutare, dec *ancoraggio.DecisioneIdentita, prodotti ...string) (valutazione.Disegno2D, []valutazione.Conflitto) {
		in.Decisione, in.LetturaDecisa = dec, &ld
		return valutazione.ValutaDisegno(in, comp, prodotti)
	}
	parti := func(c []valutazione.Conflitto) []string {
		var out []string
		for _, x := range c {
			out = append(out, x.Prodotto+":"+x.Motivo+":"+x.Proposta)
		}
		return out
	}

	t.Run("corretta contro il cartiglio visto: nessun conflitto", func(t *testing.T) {
		d, c := valuta2D(pdfDeciso(cart("7120200A2"), step("7120200A3", "3")),
			decisioneDoc("3", vista, ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaStepEntita, "7120200A3")), rifProdB)
		if len(c) != 0 {
			t.Fatalf("conflitti %+v", c)
		}
		if val(d.Confermata) != "3" || val(d.CodiceConfermato) != "7120200A" || d.StatoRevisione != valutazione.StatoRevisione2DConfermata ||
			d.FonteIdentita != valutazione.FonteIdentitaConfermata || val(d.Revisione) != "3" || d.Codice != "7120200A" ||
			d.CompatibilitaRevisione != motorea.CompatibilitaUguale || d.CompatibilitaCodice != motorea.CompatibilitaUguale || proposta(d.RevisioneProposta) != "2/cartiglio//" {
			t.Errorf("%+v", d)
		}
		want := []valutazione.DiscordanzaRevisione{{Tra: valutazione.TraPropostaConfermata, A: "2", B: "3", FonteA: valutazione.FonteEvidenzaCartiglio,
			FonteB: valutazione.FonteDiscordanzaConfermata, Effetto: valutazione.EffettoIndicatore, CalcolataDa: valutazione.CalcolataDaGo}}
		if !reflect.DeepEqual(d.Discordanze, want) {
			t.Errorf("discordanze %+v", d.Discordanze)
		}
	})
	t.Run("un cartiglio diverso da quelli visti: conflitto, la decisione resta", func(t *testing.T) {
		d, c := valuta2D(pdfDeciso(cart("7120200A4")), decisioneDoc("3", vista), rifProdB)
		dd, aa, da := dPDF1, aPDF1, operatore
		il := dataACME.Add(5*time.Hour + 123*time.Millisecond)
		want := []valutazione.Conflitto{{Tipo: valutazione.ConflittoIdentitaDocumento, Asse: valutazione.AsseSmistamento, Rif: "documento:" + dPDF1.String(),
			Prodotto: rifProdB, Decisione: "7120200A 3", OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: "7120200A4", Motivo: ancoraggio.ParteDiscordanzaRevisione,
			EvidenzaDecisione: valutazione.EvidenzaDecisione{Origine: ancoraggio.OrigineConfermato, RevProvenienza: ancoraggio.RevProvenienzaDecisioneTracciata, Da: &da, Il: &il},
			EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: &aa, DocumentoID: &dd, UnitaID: "u:pdf:cartiglio:codice:1",
				Evidenza: ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120200A4")}}}
		if !reflect.DeepEqual(c, want) {
			t.Errorf("conflitti\n%+v\nattesi\n%+v", c, want)
		}
		if val(d.Confermata) != "3" || val(d.Revisione) != "3" || len(d.Discordanze) != 1 || d.Discordanze[0].Effetto != valutazione.EffettoConflitto ||
			d.Discordanze[0].A != "4" {
			t.Errorf("la decisione resta il valore corrente: %s, discordanze %+v", val(d.Confermata), d.Discordanze)
		}
		if valutazione.RifDocumento(dPDF1) != "documento:"+dPDF1.String() {
			t.Error("RifDocumento")
		}
	})
	t.Run("sul codice", func(t *testing.T) {
		for _, codice := range []string{"7120300A3", "7120200B3"} {
			_, c := valuta2D(pdfDeciso(cart(codice)), decisioneDoc("3", vista))
			if !reflect.DeepEqual(parti(c), []string{":codice:" + codice}) {
				t.Errorf("%s: %v", codice, parti(c))
			}
		}
	})
	t.Run("uno per prodotto, in ordine", func(t *testing.T) {
		_, c := valuta2D(pdfDeciso(cart("7120200A4")), decisioneDoc("3", vista), "componente:p2", "componente:p1")
		if !reflect.DeepEqual(parti(c), []string{"componente:p1:revisione:7120200A4", "componente:p2:revisione:7120200A4"}) {
			t.Errorf("%v", parti(c))
		}
	})
	t.Run("la decisione senza revisione", func(t *testing.T) {
		d, c := valuta2D(pdfDeciso(cart("7120200A2")), decisioneDoc(""))
		if !reflect.DeepEqual(parti(c), []string{":revisione:7120200A2"}) || d.Confermata != nil || d.StatoRevisione != valutazione.StatoRevisione2DConfermata ||
			d.Revisione != nil || d.FonteIdentita != valutazione.FonteIdentitaConfermata || d.CompatibilitaRevisione != motorea.CompatibilitaNonDeterminabile ||
			d.ConfrontatoCon != valutazione.ConfrontatoConProposto {
			t.Errorf("conflitti %v, %+v", parti(c), d)
		}
		if len(d.Discordanze) != 1 || d.Discordanze[0].B != "" || d.Discordanze[0].Effetto != valutazione.EffettoConflitto {
			t.Errorf("discordanze %+v", d.Discordanze)
		}
	})
	t.Run("lo STEP dell'entità nuovo", func(t *testing.T) {
		d, c := valuta2D(pdfDeciso(fonteSintetica(valutazione.FonteEvidenzaCartiglio, "", "", nil, ""), step("7120200A5", "5")), decisioneDoc("3", vista))
		if len(c) != 1 || c[0].Motivo != ancoraggio.ParteDiscordanzaRevisione || c[0].Proposta != "7120200A5" ||
			!reflect.DeepEqual(c[0].EvidenzaProposta.Riferimenti, []string{nodoB("#2")}) {
			t.Errorf("conflitti %+v", c)
		}
		if proposta(d.RevisioneProposta) != "5/step_entita/"+nodoB("#2")+"/cartiglio_non_leggibile" || len(d.Discordanze) != 1 ||
			d.Discordanze[0].Effetto != valutazione.EffettoConflitto {
			t.Errorf("proposta %s, discordanze %+v", proposta(d.RevisioneProposta), d.Discordanze)
		}
	})
	t.Run("il nome del file nuovo", func(t *testing.T) {
		for n, attesa := range map[string]string{"7120200A_4": ":revisione:7120200A_4", "7120300A_3": ":codice:7120300A_3"} {
			_, c := valuta2D(pdfDeciso(nome(n)), decisioneDoc("3", vista))
			if !reflect.DeepEqual(parti(c), []string{attesa}) {
				t.Errorf("%s: %v", n, parti(c))
			}
		}
	})
	t.Run("mai da documento.rev né da una fonte non interpretabile", func(t *testing.T) {
		l := letturaDi(t, m, "nodo_step.id", "7120200A")
		doc := fonteSintetica(valutazione.FonteEvidenzaDocumento, "7120300A 9", "9", &l, "")
		nonInterpretabile := cart("7120200A4")
		nonInterpretabile.Revisione.Interpretabile = false
		in := pdfDeciso(nonInterpretabile, doc)
		in.Disegno.Registrata = testo("9")
		d, c := valuta2D(in, decisioneDoc("3", vista))
		if len(c) != 0 || d.StatoRevisione != valutazione.StatoRevisione2DConfermata {
			t.Errorf("conflitti %+v, stato %s", c, d.StatoRevisione)
		}
	})
	t.Run("una decisione su un altro oggetto non vale", func(t *testing.T) {
		altro := decisioneDoc("3")
		altro.ID = dPDF2
		sulComponente := decisioneDoc("3")
		sulComponente.Oggetto = ancoraggio.OggettoDecisioneComponente
		for _, dec := range []*ancoraggio.DecisioneIdentita{altro, sulComponente} {
			d, c := valuta2D(pdfDeciso(cart("7120200A4")), dec)
			if len(c) != 0 || d.Confermata != nil || d.CodiceConfermato != nil || d.StatoRevisione != valutazione.StatoRevisione2DProposta ||
				d.FonteIdentita != valutazione.FonteIdentitaCartiglio {
				t.Errorf("%s %s: %+v, conflitti %+v", dec.Oggetto, dec.ID, d, c)
			}
		}
		in := pdfDeciso(cart("7120200A4"))
		in.Disegno.DocumentoID = nil
		if d, c := valuta2D(in, decisioneDoc("3")); len(c) != 0 || d.Confermata != nil {
			t.Errorf("un 2D che non è un documento: %+v", c)
		}
	})
	t.Run("senza la lettura del codice deciso la parte codice non si giudica", func(t *testing.T) {
		in := pdfDeciso(cart("7120300A3"))
		in.Decisione = decisioneDoc("3", vista)
		if _, c := valutazione.ValutaDisegno(in, comp, nil); len(c) != 0 {
			t.Errorf("conflitti %+v", c)
		}
	})
}

// ---- il primario e documento_componente (T-E1R-06, T-E1R-09) ----

// TestPO38IlPrimarioELaDiscordanza (PO-38; E1R §5.4, §5.6; T-E1R-06, T-E1R-09): un PDF senza revisione propria non
// diventa corrente per la revisione dello STEP: vince il TIFF della revisione giusta; con la revisione confermata il PDF
// torna primario per il formato. documento_componente si calcola anche quando la proposta viene dallo STEP, con la
// revisione propria del 2D; senza una revisione del 2D non è determinabile.
func TestPO38IlPrimarioELaDiscordanza(t *testing.T) {
	m := motoreCatenaRev(t)
	t.Run("dalla fotografia: vince il TIFF della revisione giusta", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "disegno-acme.pdf", sha1, scansione(t, sha1))
		tif := documento2D(dTIFF, cSciolto, aTIFF, "7120200A_3.tif", "tif", sha3)
		conFile2D(&th, allegato(aTIFF, 30, "7120200A_3.tif", "tif", sha3), nil, &tif, uuid.Nil)
		g := gruppoDi(t, valuta(t, th, m, nil), cSciolto)
		if g.Primario.Sha256 != sha3 || g.RegolaPrimario != valutazione.RegolaPrimarioCompatibilita {
			t.Fatalf("gruppo %+v", g)
		}
		pdf := g.Alternativi[0]
		if val(pdf.RevisioneProposta.Valore) != "3" || pdf.Revisione != nil || pdf.CompatibilitaRevisione != motorea.CompatibilitaNonDeterminabile ||
			pdf.ConfrontatoCon != valutazione.ConfrontatoConProposto || len(pdf.Discordanze) != 0 {
			t.Errorf("il PDF %+v", pdf)
		}
		if val(g.Primario.Revisione) != "3" || g.Primario.CompatibilitaRevisione != motorea.CompatibilitaUguale || len(g.Primario.Discordanze) != 0 {
			t.Errorf("il TIFF %+v", *g.Primario)
		}
	})
	t.Run("sulla regola: con la revisione confermata il PDF torna primario", func(t *testing.T) {
		ld := letturaDi(t, m, "nodo_step.id", "7120200A")
		comp := valutazione.ComponenteDaConfrontare{ComponenteID: cSciolto, Lettura: &ld, Revisione: testo("3"), FonteRevisione: valutazione.FonteComponenteStepEntita,
			Confronto: valutazione.ConfrontatoConProposto}
		in := pdfDeciso(fonteSintetica(valutazione.FonteEvidenzaStepEntita, "7120200A3", "3", nil, nodoB("#2")))
		in.Decisione, in.LetturaDecisa = decisioneDoc("3"), &ld
		pdf, _ := valutazione.ValutaDisegno(in, comp, nil)
		ln := letturaDi(t, m, "nome_file", "7120200A_3")
		tif := valutazione.DisegnoDaValutare{Disegno: valutazione.Disegno2D{Sha256: sha3, Formato: valutazione.Formato2DTIFF, Provenienza: ancoraggio.OrigineConfermato,
			Corrente: true}, Fonti: []valutazione.FonteDelDisegno{fonteSintetica(valutazione.FonteEvidenzaNomeFile, "7120200A_3", "3", &ln, "")}}
		t2, _ := valutazione.ValutaDisegno(tif, comp, nil)
		g := valutazione.Primario([]valutazione.Disegno2D{t2, pdf})
		if g.Primario.Sha256 != sha1 || g.RegolaPrimario != valutazione.RegolaPrimarioFormato || pdf.CompatibilitaRevisione != motorea.CompatibilitaUguale {
			t.Errorf("gruppo %+v", g)
		}
	})
	t.Run("documento_componente con la proposta dallo STEP", func(t *testing.T) {
		th := scenaRevisioni(t)
		conDocumento2D(&th, dPDF1, cSciolto, aPDF1, "7120200A_2.pdf", sha1, scansione(t, sha1))
		d := *gruppoDi(t, valuta(t, th, m, nil), cSciolto).Primario
		want := []valutazione.DiscordanzaRevisione{{Tra: valutazione.TraDocumentoComponente, A: "2", B: "3", FonteA: valutazione.FonteIdentitaNomeFile,
			FonteB: valutazione.FonteComponenteStepEntita, Effetto: valutazione.EffettoIndicatore, CalcolataDa: valutazione.CalcolataDaGo}}
		if proposta(d.RevisioneProposta) != "3/step_entita/"+nodoB("#2")+"/cartiglio_non_leggibile" || !reflect.DeepEqual(d.Discordanze, want) {
			t.Errorf("proposta %s, discordanze %+v", proposta(d.RevisioneProposta), d.Discordanze)
		}
	})
}

// TestIValoriDellaRevisione: i valori e i campi del contratto (§2.6, E1R §7.2). Si riscrive con «Riscritta per …».
func TestIValoriDellaRevisione(t *testing.T) {
	for _, c := range [][2]string{
		{valutazione.FonteEvidenzaCartiglio, "cartiglio"}, {valutazione.FonteEvidenzaStepEntita, "step_entita"}, {valutazione.FonteEvidenzaNomeFile, "nome_file"},
		{valutazione.FonteEvidenzaDocumento, "documento"}, {valutazione.FontePropostaAssente, "assente"},
		{valutazione.MotivoPropostaCartiglioNonLeggibile, "cartiglio_non_leggibile"}, {valutazione.MotivoPropostaRevisioneNonDeterminata, "revisione_non_determinata"},
		{valutazione.MotivoPropostaEntitaNonUnivoca, "entita_non_univoca"}, {valutazione.MotivoPropostaNomeNonInterpretabile, "nome_non_interpretabile"},
		{valutazione.MotivoPropostaNessunaFonte, "nessuna_fonte"},
		{valutazione.TraDocumentoComponente, "documento_componente"}, {valutazione.TraPropostaRegistrata, "proposta_registrata"},
		{valutazione.TraPropostaConfermata, "proposta_confermata"}, {valutazione.EffettoIndicatore, "indicatore"}, {valutazione.EffettoConflitto, "conflitto"},
		{valutazione.CalcolataDaGo, "go"}, {valutazione.CalcolataDaVista, "vista"},
		{valutazione.StatoRevisione2DAssente, "assente"}, {valutazione.StatoRevisione2DProposta, "proposta"},
		{valutazione.StatoRevisione2DRegistrata, "registrata"}, {valutazione.StatoRevisione2DConfermata, "confermata"},
		{ancoraggio.RevProvenienzaNonRegistrata, "non_registrata"}, {ancoraggio.RevProvenienzaFormazioneSTEP, "coincide_con_formazione_step"},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, atteso %q", c[0], c[1])
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{valutazione.EvidenzaRevisione{}, "Fonte:fonte Valore:valore Entita:entita Interpretabile:interpretabile Motivo:motivo"},
		{valutazione.RevisioneProposta{}, "Valore:valore Fonte:fonte Entita:entita Motivo:motivo"},
		{valutazione.DiscordanzaRevisione{}, "Tra:tra A:a B:b FonteA:fonte_a FonteB:fonte_b Effetto:effetto CalcolataDa:calcolata_da"},
	} {
		if got := campiJSON(c.tipo); got != c.campi {
			t.Errorf("%T: campi %q, attesi %q", c.tipo, got, c.campi)
		}
	}
}
