// L1 — la fonte strutturale STEP di un prodotto (contratto di A1c, §1.0 riga 2, §1.2 con le note; R65 A, R68 A,
// R76 b A; T-B0-07, T-B0-08, T-B0-10, T-12; T-E1-09; PO-34): ogni riga della tabella di R65 sugli esiti della vista,
// il gesto 3 con la marcatura e nelle forme rare, la delega, il candidato del motore A solo con la radice e il
// prodotto solo come nodo interno, nessun candidato che cambi una fonte confermata, l'ordine dei motivi di una fonte
// assente coppia per coppia, l'estrazione fallita diversa dai dati insufficienti, il riferimento superato, un PDF che
// non è mai fonte, la fonte che non si calcola.
package valutazione_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// La scena della fonte: un finito manuale «7120100» (quindi target), con lo STEP «7120100.stp» fra gli allegati di
// m1 e come documento del finito, i fatti con la radice «#1» = 7120100 e il figlio «#2» = 7120200.
var (
	cFinito   = uid(0x501)
	docStep   = uid(0x511)
	aStep     = uid(0x521)
	rigaRad   = uid(0x531)
	rigaFig   = uid(0x532)
	propSTEP  = uid(0x541)
	shaStep   = sha("1")
	shaAltro  = sha("2")
	rifFinito = "componente:" + uid(0x501).String()
)

// scenaFinito: il thread con il finito, lo STEP (allegato, fatti, documento) e la riga della vista con l'esito dato.
// Il gesto 3 (step_strutturale_id) c'è per gli esiti «presente_*» e «riferimento_superato».
func scenaFinito(t *testing.T, esito string) fotorfq.Thread {
	t.Helper()
	th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
	c := componente(cFinito, "7120100", "finito", "manuale")
	th.Componenti = []fotorfq.Componente{c}
	f := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7120100"}, {"#2", "7120200"}}, [][2]string{{"#1", "#2"}}, "")
	conAllegato(&th, allegato(aStep, 1, "7120100.stp", "stp", shaStep), &f)
	cf := cFinito
	th.Documenti = []fotorfq.DocumentoConfermato{{ID: docStep, ThreadID: threadACME, ComponenteID: &cf, Tipo: "cad_3d", NomeFile: "7120100.stp",
		Estensione: "stp", Sha256: shaStep, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME, Allegati: []uuid.UUID{aStep}}}
	riga := fotorfq.RigaStepProdotto{ComponenteID: cFinito, Codice: "7120100", NStepCorrenti: 1, Esito: esito}
	switch esito {
	case "presente_analizzato", "presente_parziale", "presente_non_analizzato", "riferimento_superato":
		d := docStep
		th.Componenti[0].StepStrutturaleID = &d
		riga.StepStrutturaleID = &d
	}
	th.StepProdotto = []fotorfq.RigaStepProdotto{riga}
	th.RigheComponenteProposta = []fotorfq.RigaComponenteProposta{
		rigaProposta(rigaRad, "#1", "7120100"), rigaProposta(rigaFig, "#2", "7120200"),
	}
	th.RigheRelazioneProposta = []fotorfq.RigaRelazioneProposta{{AllegatoID: aStep, NomeFile: "7120100.stp", PadreChiave: "#1", FiglioChiave: "#2", Qta: 1, Stato: "aperta"}}
	return th
}

func rigaProposta(id uuid.UUID, chiave, codice string) fotorfq.RigaComponenteProposta {
	return fotorfq.RigaComponenteProposta{ID: id, AllegatoID: aStep, NomeFile: "7120100.stp", Sha256: shaStep, Chiave: chiave, IDGrezzo: codice,
		NomeGrezzo: codice, Famiglia: "acme", Fonte: "step", Stato: "aperta"}
}

// marca mette la marcatura del gesto 3 sulla riga della radice (ruolo radice, chi e quando).
func marca(th *fotorfq.Thread, ruolo string, componente uuid.UUID, sospesa bool) {
	op, d, c := operatore, docStep, componente
	th.RigheComponenteProposta[0].Marcatura = &fotorfq.MarcaturaStrutturale{Versione: 1, Ruolo: ruolo, ComponenteID: &c, DocumentoID: &d,
		DichiaratoDa: &op, DichiaratoIl: "2026-10-01T08:00:00Z", Sospesa: sospesa}
}

func fonteDi(t *testing.T, v valutazione.ValutazioneProdotti, rif string) valutazione.FonteProdotto {
	t.Helper()
	return prodotto(t, v, rif).Fonte
}

func statoMotivo(f valutazione.FonteProdotto) string { return string(f.Stato) + "/" + string(f.Motivo) }

// TestR65LaTabellaDellaVista (R65 A, la tabella di corrispondenza; R65 b A; 6.0.3; T-E1-09): ogni esito di
// v_step_prodotto dà lo stato e il motivo della tabella, con l'esito e il motivo SQL accanto.
func TestR65LaTabellaDellaVista(t *testing.T) {
	m := motoreACME(t)
	for _, c := range []struct {
		nome, esito string
		prepara     func(th *fotorfq.Thread)
		atteso      string
	}{
		{"presente_analizzato", "presente_analizzato", func(th *fotorfq.Thread) { marca(th, "radice", cFinito, false) }, "confermata/estrazione_riuscita"},
		{"presente_parziale", "presente_parziale", func(th *fotorfq.Thread) {
			marca(th, "radice", cFinito, false)
			th.StepProdotto[0].MotivoParziale = testo("grafo incompleto: solo parti")
		}, "confermata/dati_insufficienti"},
		{"presente_non_analizzato con il lavoro pendente", "presente_non_analizzato", func(th *fotorfq.Thread) {
			marca(th, "radice", cFinito, false)
			th.InAttesa = []uuid.UUID{aStep}
		}, "confermata/analisi_in_corso"},
		{"presente_non_analizzato senza lavoro pendente", "presente_non_analizzato", func(th *fotorfq.Thread) { marca(th, "radice", cFinito, false) },
			"confermata/non_analizzata"},
		{"riferimento_superato", "riferimento_superato", func(th *fotorfq.Thread) {
			marca(th, "radice", cFinito, false)
			nuovo := uid(0x512)
			th.Documenti[0].SostituitoDa = &nuovo
		}, "in_attesa_di_conferma/riferimento_superato"},
		{"da_scegliere", "da_scegliere", nil, "in_attesa_di_conferma/da_scegliere"},
		{"da_confermare, una proposta aperta di STEP", "da_confermare", func(th *fotorfq.Thread) {
			th.Documenti = nil
			p := propSTEP
			th.StepProdotto[0].PropostaAperta = &p
			th.Proposte = []fotorfq.PropostaAttuale{{ID: propSTEP, AllegatoID: aStep, Tipo: "cad_3d", Fonte: "estensione", Stato: "aperta"}}
		}, "in_attesa_di_conferma/proposta_3d_aperta"},
		{"da_confermare, una proposta aperta di un altro 3D (T-E1-09)", "da_confermare", func(th *fotorfq.Thread) {
			th.Documenti, th.Allegati, th.Fatti = nil, nil, nil
			conAllegato(th, allegato(uid(0x522), 2, "7120100.igs", "igs", sha("3")), nil)
			p := propSTEP
			th.StepProdotto[0].PropostaAperta = &p
			th.Proposte = []fotorfq.PropostaAttuale{{ID: propSTEP, AllegatoID: uid(0x522), Tipo: "cad_3d", Fonte: "estensione", Stato: "aperta"}}
		}, "assente/solo_altro_3d"},
		{"sul_portale", "sul_portale", func(th *fotorfq.Thread) { th.Documenti, th.Allegati, th.Fatti = nil, nil, nil }, "assente/indicata_sul_portale"},
		{"solo_altro_3d", "solo_altro_3d", func(th *fotorfq.Thread) { th.Documenti, th.Allegati, th.Fatti = nil, nil, nil }, "assente/solo_altro_3d"},
		{"mancante", "mancante", func(th *fotorfq.Thread) { th.Documenti, th.Allegati, th.Fatti = nil, nil, nil }, "assente/nessuna_fonte"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := scenaFinito(t, c.esito)
			if c.prepara != nil {
				c.prepara(&th)
			}
			f := fonteDi(t, valuta(t, th, m, nil), rifFinito)
			if statoMotivo(f) != c.atteso || f.EsitoVista != c.esito || !f.Calcolata {
				t.Errorf("fonte %s, esito della vista %q: attesa %s", statoMotivo(f), f.EsitoVista, c.atteso)
			}
			if c.esito == "presente_parziale" && f.MotivoSQL != "grafo incompleto: solo parti" {
				t.Errorf("motivo SQL %q: il motivo della vista resta accanto", f.MotivoSQL)
			}
			if f.Stato == valutazione.FonteConfermata && (f.Riferimento == nil || f.Riferimento.Forma != ancoraggio.FormaRiferimentoSmistamento ||
				f.Riferimento.Radice != "#1" || f.Riferimento.Ruolo != "radice" || f.Riferimento.ConfermatoDa == nil || f.Riferimento.Sha256 != shaStep ||
				f.Riferimento.AllegatoID == nil || *f.Riferimento.AllegatoID != aStep || f.Estrazione != ancoraggio.EstrazioneRiuscita) {
				t.Errorf("riferimento %+v, estrazione %q", f.Riferimento, f.Estrazione)
			}
			if c.esito == "riferimento_superato" && (f.Riferimento == nil || !f.Riferimento.Superato) {
				t.Errorf("riferimento %+v: atteso superato", f.Riferimento)
			}
			if c.esito == "da_scegliere" && (len(f.Candidati) != 1 || f.Candidati[0].Origine != ancoraggio.CandidatoDaDocumentoDelProdotto ||
				f.Candidati[0].RadiceCompatibile != "#1" || f.Candidati[0].DocumentoID == nil || *f.Candidati[0].DocumentoID != docStep) {
				t.Errorf("candidati %+v: atteso lo STEP corrente del prodotto, non scelto", f.Candidati)
			}
			if c.nome == "da_confermare, una proposta aperta di STEP" && (len(f.Candidati) != 1 || f.Candidati[0].Origine != ancoraggio.CandidatoDaPropostaAperta) {
				t.Errorf("candidati %+v: attesa la proposta aperta", f.Candidati)
			}
		})
	}

	t.Run("radice_senza_qualifica: il componente del target non è un finito", func(t *testing.T) {
		th := threadBase("Richiesta.")
		cs := uid(0x502)
		th.Componenti = []fotorfq.Componente{componente(cs, "7120100", "sottoassieme", "step")}
		th.Identificativi = []fotorfq.Identificativo{confermato("7120100")}
		th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cs, Codice: "7120100", Esito: "radice_senza_qualifica"}}
		f := fonteDi(t, valuta(t, th, m, nil), "componente:"+cs.String())
		if statoMotivo(f) != "assente/nessuna_fonte" || f.EsitoVista != "radice_senza_qualifica" {
			t.Errorf("fonte %s, esito %q", statoMotivo(f), f.EsitoVista)
		}
	})
}

// TestTB008IlGesto3NelleFormeRare (T-B0-08; contratto §1.2, nota): step_strutturale_id senza marcatura vale
// confermata, con chi e quando non registrati e la radice dalle righe del file senza arco entrante se è una sola,
// altrimenti «non registrata» con la diagnosi; una marcatura sospesa non conta; una marcatura di un altro componente
// è incoerente; la vista con un altro STEP è incoerente; una delega è fonte solo con la radice del prodotto.
func TestTB008IlGesto3NelleFormeRare(t *testing.T) {
	m := motoreACME(t)

	t.Run("senza marcatura, la radice dalle righe", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		v := valuta(t, th, m, nil)
		f := fonteDi(t, v, rifFinito)
		if statoMotivo(f) != "confermata/estrazione_riuscita" || f.Riferimento.Forma != ancoraggio.FormaRiferimentoStepStrutturale ||
			f.Riferimento.Radice != "#1" || f.Riferimento.ConfermatoDa != nil || f.Riferimento.ConfermatoIl != "" || f.Riferimento.Ruolo != "" {
			t.Errorf("fonte %s, riferimento %+v", statoMotivo(f), f.Riferimento)
		}
		if len(conCodice(v.Diagnostiche, valutazione.CodiceRadiceNonRegistrata)) != 0 {
			t.Errorf("diagnostiche %+v", v.Diagnostiche)
		}
	})

	t.Run("senza marcatura e con due radici: non registrata", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		th.RigheRelazioneProposta = nil
		v := valuta(t, th, m, nil)
		f := fonteDi(t, v, rifFinito)
		if f.Stato != valutazione.FonteConfermata || f.Riferimento.Radice != "" || len(conCodice(v.Diagnostiche, valutazione.CodiceRadiceNonRegistrata)) != 1 {
			t.Errorf("fonte %s, riferimento %+v, diagnostiche %+v", statoMotivo(f), f.Riferimento, v.Diagnostiche)
		}
	})

	t.Run("una marcatura sospesa non conta", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		marca(&th, "radice", cFinito, true)
		f := fonteDi(t, valuta(t, th, m, nil), rifFinito)
		if f.Riferimento.Forma != ancoraggio.FormaRiferimentoStepStrutturale || f.Riferimento.ConfermatoDa != nil {
			t.Errorf("riferimento %+v", f.Riferimento)
		}
	})

	t.Run("una marcatura di un altro componente è incoerente", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		marca(&th, "radice", uid(0x5ff), false)
		v := valuta(t, th, m, nil)
		if len(conCodice(v.Diagnostiche, valutazione.CodiceRiferimentoIncoerente)) == 0 || fonteDi(t, v, rifFinito).Riferimento.Forma != ancoraggio.FormaRiferimentoStepStrutturale {
			t.Errorf("diagnostiche %+v", v.Diagnostiche)
		}
	})

	t.Run("la vista con un altro STEP è incoerente", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		marca(&th, "radice", cFinito, false)
		altro := uid(0x5fe)
		th.StepProdotto[0].StepStrutturaleID = &altro
		if v := valuta(t, th, m, nil); len(conCodice(v.Diagnostiche, valutazione.CodiceRiferimentoIncoerente)) != 1 {
			t.Errorf("diagnostiche %+v", v.Diagnostiche)
		}
	})

	t.Run("una delega con la radice del prodotto è fonte", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		marca(&th, "delega", cFinito, false)
		if f := fonteDi(t, valuta(t, th, m, nil), rifFinito); statoMotivo(f) != "confermata/estrazione_riuscita" || f.Riferimento.Ruolo != "delega" {
			t.Errorf("fonte %s", statoMotivo(f))
		}
	})

	t.Run("una delega con la radice non confrontabile non si calcola (T-B1-11)", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		marca(&th, "delega", cFinito, false)
		if fo := fonteDi(t, valuta(t, th, nil, nil), rifFinito); fo.Calcolata || fo.Stato != "" || fo.Motivo != valutazione.MotivoFonteNonDeterminabile ||
			fo.EsitoVista != "presente_analizzato" || fo.Riferimento == nil {
			t.Errorf("senza grammatica: fonte %+v", fo)
		}
		th.Componenti[0].Codice = "ACME-SENZA-FORMA"
		v := valuta(t, th, m, nil)
		if fo := fonteDi(t, v, rifFinito); fo.Calcolata || fo.Motivo != valutazione.MotivoFonteNonDeterminabile {
			t.Errorf("con il codice non letto: fonte %+v", fo)
		}
		if len(conCodice(v.Diagnostiche, valutazione.CodiceRiferimentoIncoerente)) != 0 {
			t.Errorf("diagnostiche %+v: una radice che non si confronta non è incoerente", v.Diagnostiche)
		}
	})

	t.Run("una delega senza la radice del prodotto non è fonte (R76 b)", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		f := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7129999"}, {"#2", "7120100"}}, [][2]string{{"#1", "#2"}}, "")
		th.Fatti[shaStep] = f
		marca(&th, "delega", cFinito, false)
		v := valuta(t, th, m, nil)
		fo := fonteDi(t, v, rifFinito)
		if statoMotivo(fo) != "assente/solo_nodo_interno" || fo.Riferimento == nil || len(conCodice(v.Diagnostiche, valutazione.CodiceRiferimentoIncoerente)) != 1 {
			t.Errorf("fonte %s, riferimento %+v, diagnostiche %+v", statoMotivo(fo), fo.Riferimento, v.Diagnostiche)
		}
	})
}

// scenaSenzaComponente: un identificativo confermato senza componente (T-B0-07): la fonte viene dai soli candidati
// del motore A, fra gli allegati del thread.
func scenaSenzaComponente() fotorfq.Thread {
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per P7120100.")
	th.Identificativi = []fotorfq.Identificativo{confermato("P7120100")}
	return th
}

const rifSenzaComponente = "identificativo:P7120100"

// TestR76bIlCandidatoDelMotoreASoloConLaRadice (R76 b A; R65 A; T-B0-07; M1): uno STEP del thread la cui radice ha la
// base del prodotto è un documento candidato e porta la fonte da assente a in attesa di conferma; se il prodotto è
// solo un nodo interno, la fonte resta assente con solo_nodo_interno; un grafo incompleto si vede nel candidato.
func TestR76bIlCandidatoDelMotoreASoloConLaRadice(t *testing.T) {
	m := motoreACME(t)

	th := scenaSenzaComponente()
	f := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7120100"}, {"#2", "7120200"}}, [][2]string{{"#1", "#2"}}, "")
	conAllegato(&th, allegato(aStep, 1, "assieme_acme.stp", "stp", shaStep), &f)
	fo := fonteDi(t, valuta(t, th, m, nil), rifSenzaComponente)
	if statoMotivo(fo) != "in_attesa_di_conferma/documento_candidato" || len(fo.Candidati) != 1 || fo.Riferimento != nil {
		t.Fatalf("fonte %s, candidati %+v", statoMotivo(fo), fo.Candidati)
	}
	c := fo.Candidati[0]
	if c.Origine != ancoraggio.CandidatoDaMotoreA || c.AllegatoID == nil || *c.AllegatoID != aStep || c.RadiceCompatibile != "#1" ||
		c.Compatibilita != motorea.CompatibilitaUguale || c.Estrazione != ancoraggio.EstrazioneRiuscita || !c.GrafoCompleto ||
		strings.Join(c.Radici, ",") != "#1" || c.Sha256 != shaStep {
		t.Errorf("candidato %+v", c)
	}

	t.Run("il prodotto solo come nodo interno", func(t *testing.T) {
		th := scenaSenzaComponente()
		f := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7129999"}, {"#2", "7120100"}}, [][2]string{{"#1", "#2"}}, "")
		conAllegato(&th, allegato(aStep, 1, "assieme_acme.stp", "stp", shaStep), &f)
		fo := fonteDi(t, valuta(t, th, m, nil), rifSenzaComponente)
		if statoMotivo(fo) != "assente/solo_nodo_interno" || len(fo.Candidati) != 0 {
			t.Errorf("fonte %s, candidati %+v", statoMotivo(fo), fo.Candidati)
		}
	})

	t.Run("un grafo incompleto: dati insufficienti nel candidato", func(t *testing.T) {
		th := scenaSenzaComponente()
		f := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7120100"}}, nil, "solo parti, nessun arco")
		conAllegato(&th, allegato(aStep, 1, "assieme_acme.stp", "stp", shaStep), &f)
		fo := fonteDi(t, valuta(t, th, m, nil), rifSenzaComponente)
		if fo.Stato != valutazione.FonteInAttesaDiConferma || fo.Candidati[0].GrafoCompleto || fo.Candidati[0].MotivoGrafo != "solo parti, nessun arco" ||
			fo.Candidati[0].Estrazione != ancoraggio.EstrazioneRiuscita {
			t.Errorf("fonte %s, candidato %+v", statoMotivo(fo), fo.Candidati)
		}
	})
}

// TestTB007UnCandidatoNonCambiaUnaFonteConfermata (T-B0-07; PO-25 nella parte della fonte): uno STEP nuovo con la
// radice del prodotto diventa candidato, e la fonte confermata resta confermata; una fonte in attesa resta con il suo
// motivo.
func TestTB007UnCandidatoNonCambiaUnaFonteConfermata(t *testing.T) {
	m := motoreACME(t)
	for _, c := range []struct{ esito, atteso string }{
		{"presente_analizzato", "confermata/estrazione_riuscita"},
		{"da_scegliere", "in_attesa_di_conferma/da_scegliere"},
	} {
		th := scenaFinito(t, c.esito)
		f := fattiSTEP(t, shaAltro, []string{"#1"}, []nodoSTEP{{"#1", "7120100"}}, nil, "")
		conAllegato(&th, allegato(uid(0x523), 3, "7120100_nuovo.stp", "stp", shaAltro), &f)
		fo := fonteDi(t, valuta(t, th, m, nil), rifFinito)
		motoreA := 0
		for _, d := range fo.Candidati {
			if d.Origine == ancoraggio.CandidatoDaMotoreA && d.Sha256 == shaAltro {
				motoreA++
			}
		}
		if statoMotivo(fo) != c.atteso || motoreA != 1 {
			t.Errorf("%s: fonte %s, candidati %+v", c.esito, statoMotivo(fo), fo.Candidati)
		}
	}
}

// TestPO34SoloLoSTEPEFonteELOrdineDeiMotivi (PO-34; T-E1-09; R68 A): solo IGS e DXF fra gli allegati danno
// solo_altro_3d dal motore A; uno STEP senza fatti step_presente_non_analizzato; un caso per ogni coppia vicina
// dell'ordine dei motivi di una fonte assente (estrazione_fallita, analisi_in_corso, step_presente_non_analizzato,
// solo_nodo_interno, indicata_sul_portale, solo_altro_3d, prodotto_senza_componente o nessuna_fonte); un PDF non è
// mai fonte; l'estrazione fallita non è «dati insufficienti».
func TestPO34SoloLoSTEPEFonteELOrdineDeiMotivi(t *testing.T) {
	m := motoreACME(t)
	stepFallito := func(t *testing.T, th *fotorfq.Thread) {
		f := fattiSTEPIllegibili(t, sha("4"))
		conAllegato(th, allegato(uid(0x601), 1, "illeggibile_acme.stp", "stp", sha("4")), &f)
	}
	stepInCorso := func(t *testing.T, th *fotorfq.Thread) {
		conAllegato(th, allegato(uid(0x602), 2, "in_analisi_acme.stp", "stp", sha("5")), nil)
		th.InAttesa = append(th.InAttesa, uid(0x602))
	}
	stepNonAnalizzato := func(t *testing.T, th *fotorfq.Thread) {
		conAllegato(th, allegato(uid(0x603), 3, "da_analizzare_acme.stp", "stp", sha("6")), nil)
	}
	stepInterno := func(t *testing.T, th *fotorfq.Thread) {
		f := fattiSTEP(t, sha("7"), []string{"#1"}, []nodoSTEP{{"#1", "7129999"}, {"#2", "7120100"}}, [][2]string{{"#1", "#2"}}, "")
		conAllegato(th, allegato(uid(0x604), 4, "assieme_grande_acme.stp", "stp", sha("7")), &f)
	}
	altro3D := func(t *testing.T, th *fotorfq.Thread) {
		conAllegato(th, allegato(uid(0x605), 5, "7120100.igs", "igs", sha("8")), nil)
		conAllegato(th, allegato(uid(0x606), 6, "7120100.dxf", "dxf", sha("9")), nil)
	}
	// portale: il prodotto con il componente e la vista «sul_portale» (la vista vale solo con un componente).
	conPortale := func(th *fotorfq.Thread) string {
		th.Componenti = []fotorfq.Componente{componente(cFinito, "P7120100", "finito", "codice_rilevato")}
		th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cFinito, Codice: "P7120100", Esito: "sul_portale"}}
		return rifFinito
	}

	type passo func(t *testing.T, th *fotorfq.Thread)
	for _, c := range []struct {
		nome    string
		passi   []passo
		portale bool
		atteso  string
	}{
		{"solo IGS e DXF", []passo{altro3D}, false, "assente/solo_altro_3d"},
		{"uno STEP senza fatti", []passo{stepNonAnalizzato}, false, "assente/step_presente_non_analizzato"},
		{"estrazione fallita prima di analisi in corso", []passo{stepInCorso, stepFallito}, false, "assente/estrazione_fallita"},
		{"analisi in corso prima di STEP non analizzato", []passo{stepNonAnalizzato, stepInCorso}, false, "assente/analisi_in_corso"},
		{"STEP non analizzato prima di solo nodo interno", []passo{stepInterno, stepNonAnalizzato}, false, "assente/step_presente_non_analizzato"},
		{"STEP non analizzato prima di indicata sul portale", []passo{stepNonAnalizzato}, true, "assente/step_presente_non_analizzato"},
		{"solo nodo interno prima di indicata sul portale", []passo{stepInterno}, true, "assente/solo_nodo_interno"},
		{"indicata sul portale prima di solo altro 3D", []passo{altro3D}, true, "assente/indicata_sul_portale"},
		{"solo altro 3D prima di prodotto senza componente", []passo{altro3D}, false, "assente/solo_altro_3d"},
		{"prodotto senza componente", nil, false, "assente/prodotto_senza_componente"},
		{"un altro 3D accanto a uno STEP non è solo altro 3D", []passo{altro3D, stepNonAnalizzato}, false, "assente/step_presente_non_analizzato"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := scenaSenzaComponente()
			rif := rifSenzaComponente
			if c.portale {
				rif = conPortale(&th)
			}
			for _, p := range c.passi {
				p(t, &th)
			}
			if f := fonteDi(t, valuta(t, th, m, nil), rif); statoMotivo(f) != c.atteso || len(f.Candidati) != 0 {
				t.Errorf("fonte %s, candidati %+v: atteso %s", statoMotivo(f), f.Candidati, c.atteso)
			}
		})
	}

	t.Run("con il componente e la vista mancante: nessuna fonte", func(t *testing.T) {
		th := scenaFinito(t, "mancante")
		th.Documenti, th.Allegati, th.Fatti = nil, nil, nil
		if f := fonteDi(t, valuta(t, th, m, nil), rifFinito); statoMotivo(f) != "assente/nessuna_fonte" {
			t.Errorf("fonte %s", statoMotivo(f))
		}
	})

	t.Run("un PDF con il codice non è mai fonte", func(t *testing.T) {
		th := scenaSenzaComponente()
		f := fatti(t, sha("a"), `{"testo_pdf": {"pagine": [{"testo": "7120100"}]}}`, nil)
		conAllegato(&th, allegato(uid(0x607), 1, "7120100.pdf", "pdf", sha("a")), &f)
		if fo := fonteDi(t, valuta(t, th, m, nil), rifSenzaComponente); statoMotivo(fo) != "assente/prodotto_senza_componente" || len(fo.Candidati) != 0 {
			t.Errorf("fonte %s, candidati %+v", statoMotivo(fo), fo.Candidati)
		}
	})

	t.Run("uno STEP con i fatti senza struttura: non analizzato", func(t *testing.T) {
		th := scenaSenzaComponente()
		f := fatti(t, sha("b"), `{"esito": {"codice": "7120100"}}`, nil)
		conAllegato(&th, allegato(uid(0x608), 1, "7120100.stp", "stp", sha("b")), &f)
		if fo := fonteDi(t, valuta(t, th, m, nil), rifSenzaComponente); statoMotivo(fo) != "assente/step_presente_non_analizzato" {
			t.Errorf("fonte %s", statoMotivo(fo))
		}
	})

	t.Run("estrazione fallita non è dati insufficienti", func(t *testing.T) {
		th := scenaFinito(t, "presente_parziale")
		marca(&th, "radice", cFinito, false)
		th.StepProdotto[0].MotivoParziale = testo("grafo incompleto")
		if f := fonteDi(t, valuta(t, th, m, nil), rifFinito); statoMotivo(f) != "confermata/dati_insufficienti" {
			t.Errorf("fonte %s", statoMotivo(f))
		}
		th2 := scenaSenzaComponente()
		stepFallito(t, &th2)
		if f := fonteDi(t, valuta(t, th2, m, nil), rifSenzaComponente); statoMotivo(f) != "assente/estrazione_fallita" {
			t.Errorf("fonte %s", statoMotivo(f))
		}
	})
}

// TestT12LaFonteCheNonSiCalcola (T-12; M1): con una sezione della fonte assente (gli export) la fonte non si calcola:
// niente stato, motivo non_determinabile. Senza grammatica, o con la sezione dei fatti assente, uno STEP del thread
// potrebbe essere un candidato: una fonte che sarebbe assente non si calcola; lo stato dato dalla vista resta.
func TestT12LaFonteCheNonSiCalcola(t *testing.T) {
	m := motoreACME(t)

	th := scenaFinito(t, "presente_analizzato")
	f := fotografia(th)
	f.Sezioni[fotorfq.SezioneStepProdotto] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente, Motivo: "export mancante"}
	v, err := valutazione.ValutaProdotti(f, th, m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fo := fonteDi(t, v, rifFinito); fo.Calcolata || fo.Stato != "" || fo.Motivo != valutazione.MotivoFonteNonDeterminabile {
		t.Errorf("fonte %+v: attesa non calcolata", fo)
	}

	t.Run("senza grammatica: la vista sì, il confronto delle radici no", func(t *testing.T) {
		th := scenaFinito(t, "presente_analizzato")
		if fo := fonteDi(t, valuta(t, th, nil, nil), rifFinito); statoMotivo(fo) != "confermata/estrazione_riuscita" || !fo.Calcolata {
			t.Errorf("fonte %s", statoMotivo(fo))
		}
		th2 := scenaSenzaComponente()
		ff := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7120100"}}, nil, "")
		conAllegato(&th2, allegato(aStep, 1, "assieme_acme.stp", "stp", shaStep), &ff)
		if fo := fonteDi(t, valuta(t, th2, nil, nil), rifSenzaComponente); fo.Calcolata || fo.Motivo != valutazione.MotivoFonteNonDeterminabile {
			t.Errorf("fonte %+v: senza grammatica la compatibilità non si inventa", fo)
		}
	})

	t.Run("con la sezione dei fatti assente", func(t *testing.T) {
		th := scenaSenzaComponente()
		ff := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "7120100"}}, nil, "")
		conAllegato(&th, allegato(aStep, 1, "assieme_acme.stp", "stp", shaStep), &ff)
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneFatti] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
		v, err := valutazione.ValutaProdotti(f, th, m, nil)
		if err != nil {
			t.Fatal(err)
		}
		if fo := fonteDi(t, v, rifSenzaComponente); fo.Calcolata || fo.Motivo != valutazione.MotivoFonteNonDeterminabile {
			t.Errorf("fonte %+v", fo)
		}
	})
}

// TestIValoriDellaFonteEDellIdentita (contratto §2.3, §2.5): i valori delle costanti di valutazione sono quelli del
// contratto; i motivi della fonte sono i quindici del §2.3 più step_presente_non_analizzato. Se uno cambia, la prova
// si riscrive con «Riscritta per …».
func TestIValoriDellaFonteEDellIdentita(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{string(valutazione.IdentitaDaConfermare), "da_confermare"},
		{string(valutazione.IdentitaConfermata), "confermata"},
		{string(valutazione.FonteAssente), "assente"},
		{string(valutazione.FonteInAttesaDiConferma), "in_attesa_di_conferma"},
		{string(valutazione.FonteConfermata), "confermata"},
	} {
		if c.got != c.want {
			t.Errorf("valore %q, il contratto dice %q", c.got, c.want)
		}
	}
	var motivi []string
	for _, x := range []valutazione.MotivoFonte{
		valutazione.MotivoFonteEstrazioneRiuscita, valutazione.MotivoFonteDatiInsufficienti, valutazione.MotivoFonteAnalisiInCorso,
		valutazione.MotivoFonteNonAnalizzata, valutazione.MotivoFonteRiferimentoSuperato, valutazione.MotivoFonteDaScegliere,
		valutazione.MotivoFonteProposta3DAperta, valutazione.MotivoFonteIndicataSulPortale, valutazione.MotivoFonteSoloAltro3D,
		valutazione.MotivoFonteNessunaFonte, valutazione.MotivoFonteDocumentoCandidato, valutazione.MotivoFonteEstrazioneFallita,
		valutazione.MotivoFonteSoloNodoInterno, valutazione.MotivoFonteProdottoSenzaComponente, valutazione.MotivoFonteNonDeterminabile,
		valutazione.MotivoFonteStepPresenteNonAnalizzato,
	} {
		motivi = append(motivi, string(x))
	}
	if got := strings.Join(motivi, " "); got != "estrazione_riuscita dati_insufficienti analisi_in_corso non_analizzata riferimento_superato "+
		"da_scegliere proposta_3d_aperta indicata_sul_portale solo_altro_3d nessuna_fonte documento_candidato estrazione_fallita "+
		"solo_nodo_interno prodotto_senza_componente non_determinabile step_presente_non_analizzato" {
		t.Errorf("motivi %s", got)
	}
	for _, c := range []struct{ got, want string }{
		{valutazione.CodiceTargetNonLeggibile, "ancoraggio.target_non_leggibile"},
		{valutazione.CodiceTargetPossibileRinomina, "target.possibile_rinomina"},
		{valutazione.CodiceRiferimentoIncoerente, "fonte_strutturale.riferimento_incoerente"},
		{valutazione.CodiceRadiceNonRegistrata, "fonte_strutturale.radice_non_registrata"},
	} {
		if c.got != c.want {
			t.Errorf("codice %q, il 6.4.10 dice %q", c.got, c.want)
		}
	}
}
