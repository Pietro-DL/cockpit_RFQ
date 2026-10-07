// L1 — lo stato del prodotto, l'asse 7, e il composto (B6, V3; R79, R94 A precisata [R]; contratto §0.3, §1.0 riga 7,
// §1.7; T-E1-01, T-E1-13, T-E1-14, T-E1R-04; PO-18, PO-19 nella parte di B6, PO-22 nella parte di B6, PO-24 e PO-25
// nella parte dello stato, PO-35; R111 A, precisata dall'utente il 07/10): la regola sui prodotti sintetici, condizione
// per condizione, poi dalla fotografia, attraverso Calcola, con la fonte superata e lo STEP di prima nelle due
// direzioni di R111.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_prodotti_test.go.

// Gli ID delle prove dello stato: lo STEP nuovo che sostituisce quello confermato, il suo documento, i file nuovi.
var (
	aStepNuovo = uid(0xf01)
	dStepNuovo = uid(0xf02)
	fTIFF      = uid(0xf03)
	dTIFFNuovo = uid(0xf04)
	fPDFNuovo  = uid(0xf05)
	dStepTerzo = uid(0xf06)
	cTerzoSt   = uid(0xf07)
)

// prodottoSintetico: un prodotto con tutti gli assi verificati, scritto a mano: il target confermato, la fonte confermata,
// nomenclatura e gerarchia verificate, lo smistamento verificato e calcolato, la completezza completa.
func prodottoSintetico() valutazione.ProdottoValutato {
	c := cProdotto
	verificata := valutazione.VerificaAsse{Stato: valutazione.StatoAsseVerificata}
	return valutazione.ProdottoValutato{Rif: rifProdB, Autorita: ancoraggio.AutoritaConfermata, ComponenteID: &c, CodiceRichiesto: "7120100A",
		Identita:    valutazione.IdentitaConfermata,
		Fonte:       valutazione.FonteProdotto{Stato: valutazione.FonteConfermata, Motivo: valutazione.MotivoFonteEstrazioneRiuscita, Calcolata: true},
		BOM:         valutazione.VerificaBOM{Nomenclatura: verificata, Gerarchia: verificata, Verificata: true},
		Smistamento: valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoVerificato, Calcolata: true},
		Documenti:   valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiCompleta, PerimetroChiuso: true}}
}

// motiviProdotto: i motivi come testo, per i confronti.
func motiviProdotto(m []valutazione.MotivoProdotto) string {
	var out []string
	for _, x := range m {
		out = append(out, string(x))
	}
	return strings.Join(out, " ")
}

// statoProdotto: lo stato del prodotto come testo «verificato/stato/motivi», per i confronti.
func statoProdotto(pv valutazione.ProdottoValutato) string {
	v := "no"
	if pv.Verificato {
		v = "sì"
	}
	return v + "/" + string(pv.Stato) + "/" + motiviProdotto(pv.Motivi)
}

// voce: una voce certa della completezza con l'esito e il motivo dati (il disegno 2D del prodotto), per la regola.
func voceCerta(e valutazione.EsitoFabbisogno, m valutazione.MotivoFabbisogno) valutazione.VoceFabbisogno {
	c := cProdotto
	return valutazione.VoceFabbisogno{ComponenteID: c, TipoDocumento: "disegno_2d", Esito: e, Motivo: m}
}

// perimetroAperto: la completezza non_calcolabile per il perimetro aperto, con il motivo del perimetro e le voci certe
// date, com'è nell'uscita di Completezza (StatoDellaCompletezza: nessuna voce manca).
func perimetroAperto(motivo string, voci ...valutazione.VoceFabbisogno) valutazione.CompletezzaDocumentale {
	return valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiPerimetroAperto,
		MotivoPerimetro: motivo, Voci: voci}
}

// TestStatoDelProdottoSullaRegola (R79, T-E1-01, T-E1-14, T-E1-13, T-E1R-04; PO-24 nella parte dello stato): la regola
// condizione per condizione. Il prodotto con tutti gli assi è verificato e pronto, senza motivi; ogni condizione che manca
// dà il suo motivo; mancano solo condizioni nuove → da_riesaminare, altrimenti non_pronto.
func TestStatoDelProdottoSullaRegola(t *testing.T) {
	file := uid(0xf10)
	altro := uid(0xf11)
	nessuna := valutazione.CondizioniNuove{}
	casi := []struct {
		nome   string
		cambia func(*valutazione.ProdottoValutato)
		nuove  valutazione.CondizioniNuove
		atteso string
	}{
		{"tutti gli assi: verificato e pronto", func(*valutazione.ProdottoValutato) {}, nessuna, "sì/pronto_fattibilita/"},
		{"tutti gli assi, con le condizioni nuove che non servono", func(*valutazione.ProdottoValutato) {}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true},
			"sì/pronto_fattibilita/"},
		{"il target dello scenario non è confermato (R75 A)", func(p *valutazione.ProdottoValutato) {
			p.Autorita, p.Identita = ancoraggio.AutoritaScenario, valutazione.IdentitaDaConfermare
		}, nessuna, "no/non_pronto/target_non_confermato"},
		{"l'identità confermata con l'autorità proposta non è TargetConfermato", func(p *valutazione.ProdottoValutato) { p.Autorita = ancoraggio.AutoritaProposta },
			nessuna, "no/non_pronto/target_non_confermato"},
		{"la fonte assente", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteAssente, Motivo: valutazione.MotivoFonteNessunaFonte, Calcolata: true}
		}, nessuna, "no/non_pronto/fonte_strutturale_step_mancante_o_non_confermata"},
		{"la fonte non calcolata (T-12): mai confermata", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteConfermata, Motivo: valutazione.MotivoFonteNonDeterminabile}
		}, nessuna, "no/non_pronto/fonte_strutturale_step_mancante_o_non_confermata"},
		{"la fonte superata è una condizione nuova (T-E1-14)", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteRiferimentoSuperato, Calcolata: true}
		}, nessuna, "no/da_riesaminare/fonte_superata"},
		{"la fonte in attesa per un altro motivo non lo è", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteDaScegliere, Calcolata: true}
		}, nessuna, "no/non_pronto/fonte_strutturale_step_mancante_o_non_confermata"},
		{"la fonte superata non calcolata non conta", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Motivo: valutazione.MotivoFonteRiferimentoSuperato}
		}, nessuna, "no/non_pronto/fonte_strutturale_step_mancante_o_non_confermata"},
		{"la nomenclatura da verificare, per condizioni vecchie", func(p *valutazione.ProdottoValutato) {
			p.BOM.Nomenclatura = valutazione.VerificaAsse{Stato: valutazione.StatoAsseDaVerificare, Motivo: valutazione.MotivoAsseDaDecidere, DaDecidere: 1}
		}, nessuna, "no/non_pronto/nomenclatura_non_verificata"},
		{"la nomenclatura ferma solo per condizioni nuove", func(p *valutazione.ProdottoValutato) {
			p.BOM.Nomenclatura = valutazione.VerificaAsse{Stato: valutazione.StatoAsseDaVerificare, Motivo: valutazione.MotivoAsseDaDecidere, DaDecidere: 1}
		}, valutazione.CondizioniNuove{Nomenclatura: true}, "no/da_riesaminare/nomenclatura_non_verificata"},
		{"la nomenclatura in conflitto, ferma solo per il conflitto", func(p *valutazione.ProdottoValutato) {
			p.BOM.Nomenclatura = valutazione.VerificaAsse{Stato: valutazione.StatoAsseConflitto, Motivo: valutazione.MotivoAsseConflitto, Conflitti: []string{"componente:x"}}
		}, valutazione.CondizioniNuove{Nomenclatura: true}, "no/da_riesaminare/nomenclatura_non_verificata conflitto"},
		{"la gerarchia ferma per condizioni vecchie", func(p *valutazione.ProdottoValutato) {
			p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseDaVerificare, Motivo: valutazione.MotivoAsseNessunGesto}
		}, valutazione.CondizioniNuove{Nomenclatura: true}, "no/non_pronto/gerarchia_non_verificata"},
		{"la gerarchia ferma solo per condizioni nuove", func(p *valutazione.ProdottoValutato) {
			p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseConflitto, Motivo: valutazione.MotivoAsseConflitto, Conflitti: []string{"arco:x"}}
		}, valutazione.CondizioniNuove{Gerarchia: true}, "no/da_riesaminare/gerarchia_non_verificata conflitto"},
		{"un file pertinente non terminale (PO-18)", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true, FileNonTerminali: []uuid.UUID{file},
				Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoFileDaSmistare, valutazione.MotivoSmistamentoAssociazioneNonConfermata}}
		}, nessuna, "no/da_riesaminare/smistamento_non_verificato"},
		{"lo smistamento in conflitto (PO-25)", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoConflitto, Calcolata: true, Conflitti: []string{"componente:x"},
				Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoNuovoFile}}
		}, nessuna, "no/da_riesaminare/smistamento_non_verificato conflitto"},
		{"PO-24: un percorso aperto e il resto verificato", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoInRevisione, Calcolata: true, Percorsi: []string{"percorso-1"},
				Motivi: []valutazione.MotivoSmistamento{valutazione.MotivoSmistamentoRevisioneTecnicaAperta}}
		}, nessuna, "no/da_riesaminare/smistamento_non_verificato revisione_tecnica_aperta"},
		{"PO-24: un percorso aperto e una mancanza certa", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoInRevisione, Calcolata: true, Percorsi: []string{"percorso-1"}}
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceMancante}
		}, nessuna, "no/non_pronto/smistamento_non_verificato documentazione_incompleta revisione_tecnica_aperta"},
		{"lo smistamento non calcolato non è mai fermo per condizioni nuove (T-B6-61)", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, FileNonTerminali: []uuid.UUID{file}}
		}, nessuna, "no/non_pronto/smistamento_non_verificato"},
		{"lo smistamento da verificare senza file né conflitti né percorsi", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true}
		}, nessuna, "no/non_pronto/smistamento_non_verificato"},
		// R-81 della revisione di V3 (T-E1-14, R94 A): con il perimetro aperto da una condizione nuova la completezza è
		// non_calcolabile, e il prodotto è da_riesaminare solo se a perimetro chiuso sarebbe completa, cioè con ogni voce
		// certa presente. Una voce non presente, di qualunque motivo e con qualunque file, lo tiene non_pronto, come a
		// perimetro chiuso (incompleta): lo smistamento non guarda le voci, e i motivi sono trattati tutti allo stesso modo
		// (niente asimmetria: T-B6-82). Gli ingressi sono quelli che StatoDellaCompletezza produce.
		{"il perimetro aperto da una rimozione, con le voci certe presenti", func(p *valutazione.ProdottoValutato) {
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiPerimetroAperto,
				MotivoPerimetro: valutazione.MotivoPerimetroRimozioniAperte, Voci: []valutazione.VoceFabbisogno{voceCerta(valutazione.EsitoPresente, ""),
					voceCerta(valutazione.EsitoPresente, "")}}
		}, nessuna, "no/da_riesaminare/documentazione_non_calcolabile"},
		{"il perimetro aperto da una rimozione e il 2D confermato mai analizzato (R-81)", func(p *valutazione.ProdottoValutato) {
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoContenutoNonAnalizzato))
		}, nessuna, "no/non_pronto/documentazione_non_calcolabile"},
		{"il perimetro aperto da una rimozione e il formato non configurato (R-81)", func(p *valutazione.ProdottoValutato) {
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, voceCerta(valutazione.EsitoPresente, ""),
				voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoFormatoNonConfigurato))
		}, nessuna, "no/non_pronto/documentazione_non_calcolabile"},
		{"RV3-01b: il perimetro aperto da una rimozione con la gerarchia in conflitto e una voce vecchia", func(p *valutazione.ProdottoValutato) {
			p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseConflitto, Motivo: valutazione.MotivoAsseConflitto, Conflitti: []string{"arco:x"}}
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoContenutoNonAnalizzato))
		}, valutazione.CondizioniNuove{Gerarchia: true}, "no/non_pronto/gerarchia_non_verificata documentazione_non_calcolabile conflitto"},
		{"una voce certa con il documento non confermato su un file pertinente non terminale, dietro il perimetro aperto", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true, FileNonTerminali: []uuid.UUID{file}}
			v := voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoAssociazioneNonConfermata)
			v.FileCandidato = &file
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, v)
		}, nessuna, "no/non_pronto/smistamento_non_verificato documentazione_non_calcolabile"},
		{"la stessa voce con un file che non è pertinente (fuori perimetro: T-B6-73)", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true, FileNonTerminali: []uuid.UUID{file}}
			v := voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoAssociazioneNonConfermata)
			v.FileCandidato = &altro
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, v)
		}, nessuna, "no/non_pronto/smistamento_non_verificato documentazione_non_calcolabile"},
		{"la stessa voce senza il file", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true, FileNonTerminali: []uuid.UUID{file}}
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoAssociazioneNonConfermata))
		}, nessuna, "no/non_pronto/smistamento_non_verificato documentazione_non_calcolabile"},
		{"la stessa voce a perimetro chiuso: incompleta", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true, FileNonTerminali: []uuid.UUID{file}}
			v := voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoAssociazioneNonConfermata)
			v.FileCandidato = &file
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceNonPresente, PerimetroChiuso: true,
				Voci: []valutazione.VoceFabbisogno{v}}
		}, nessuna, "no/non_pronto/smistamento_non_verificato documentazione_incompleta"},
		{"un file pertinente non terminale con le voci certe presenti, dietro il perimetro aperto (PO-18)", func(p *valutazione.ProdottoValutato) {
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoDaVerificare, Calcolata: true, FileNonTerminali: []uuid.UUID{file}}
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroRimozioniAperte, voceCerta(valutazione.EsitoPresente, ""))
		}, nessuna, "no/da_riesaminare/smistamento_non_verificato documentazione_non_calcolabile"},
		{"la documentazione incompleta", func(p *valutazione.ProdottoValutato) {
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta, Motivo: valutazione.MotivoDocumentiVoceNonPresente, PerimetroChiuso: true}
		}, nessuna, "no/non_pronto/documentazione_incompleta"},
		{"il perimetro aperto da una rimozione proposta (condizione nuova)", func(p *valutazione.ProdottoValutato) {
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiPerimetroAperto,
				MotivoPerimetro: valutazione.MotivoPerimetroRimozioniAperte}
		}, nessuna, "no/da_riesaminare/documentazione_non_calcolabile"},
		{"il perimetro aperto da un nodo da decidere (condizione vecchia)", func(p *valutazione.ProdottoValutato) {
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiPerimetroAperto,
				MotivoPerimetro: valutazione.MotivoPerimetroNodiDaDecidere}
		}, nessuna, "no/non_pronto/documentazione_non_calcolabile"},
		{"il perimetro aperto per la fonte non confermata, senza la fonte superata", func(p *valutazione.ProdottoValutato) {
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiPerimetroAperto,
				MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
		}, nessuna, "no/non_pronto/documentazione_non_calcolabile"},
		{"la completezza non determinabile (T-12)", func(p *valutazione.ProdottoValutato) {
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiNonDeterminabile,
				MotivoPerimetro: valutazione.MotivoPerimetroRimozioniAperte}
		}, nessuna, "no/non_pronto/documentazione_non_calcolabile"},
		{"la completezza senza stato vale non calcolabile", func(p *valutazione.ProdottoValutato) { p.Documenti = valutazione.CompletezzaDocumentale{} },
			nessuna, "no/non_pronto/documentazione_non_calcolabile"},
		{"T-E1-14 con K-02: la fonte superata, la gerarchia ferma per la fonte, il perimetro aperto per la fonte", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteRiferimentoSuperato, Calcolata: true}
			p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseDaVerificare, Motivo: valutazione.MotivoAsseFonteNonConfermata}
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiNonCalcolabile, Motivo: valutazione.MotivoDocumentiPerimetroAperto,
				MotivoPerimetro: valutazione.MotivoPerimetroFonteNonConfermata}
		}, valutazione.CondizioniNuove{Gerarchia: true}, "no/da_riesaminare/fonte_superata gerarchia_non_verificata documentazione_non_calcolabile"},
		{"RV3-02: lo stesso con una voce certa da verificare per una ragione vecchia (R-81)", func(p *valutazione.ProdottoValutato) {
			p.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteRiferimentoSuperato, Calcolata: true}
			p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseDaVerificare, Motivo: valutazione.MotivoAsseFonteNonConfermata}
			p.Documenti = perimetroAperto(valutazione.MotivoPerimetroFonteNonConfermata, voceCerta(valutazione.EsitoPresente, ""),
				voceCerta(valutazione.EsitoDaVerificare, valutazione.MotivoFabbisognoContenutoNonAnalizzato))
		}, valutazione.CondizioniNuove{Gerarchia: true}, "no/non_pronto/fonte_superata gerarchia_non_verificata documentazione_non_calcolabile"},
		{"tutto manca: un motivo per ogni condizione, nell'ordine dei valori", func(p *valutazione.ProdottoValutato) {
			p.Autorita, p.Identita = ancoraggio.AutoritaScenario, valutazione.IdentitaDaConfermare
			p.Fonte = valutazione.FonteProdotto{Motivo: valutazione.MotivoFonteNonDeterminabile}
			p.BOM.Nomenclatura = valutazione.VerificaAsse{Stato: valutazione.StatoAsseNonVerificabile}
			p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseNonVerificabile, Conflitti: []string{"arco:x"}}
			p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoInRevisione, Calcolata: true, Percorsi: []string{"p"}}
			p.Documenti = valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiIncompleta}
		}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true},
			"no/non_pronto/target_non_confermato fonte_strutturale_step_mancante_o_non_confermata nomenclatura_non_verificata gerarchia_non_verificata " +
				"smistamento_non_verificato documentazione_incompleta conflitto revisione_tecnica_aperta"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			pv := prodottoSintetico()
			c.cambia(&pv)
			prima := canonicoDi(t, pv)
			pv.Verificato, pv.Stato, pv.Motivi = valutazione.StatoDelProdotto(pv, c.nuove)
			if got := statoProdotto(pv); got != c.atteso {
				t.Errorf("stato %q, atteso %q", got, c.atteso)
			}
			pv.Verificato, pv.Stato, pv.Motivi = false, "", nil
			if canonicoDi(t, pv) != prima {
				t.Error("StatoDelProdotto ha cambiato il prodotto di chi chiama")
			}
		})
	}
	t.Run("i conflitti dei due assi della BOM e dello smistamento contano anche senza lo stato conflitto", func(t *testing.T) {
		for nome, cambia := range map[string]func(*valutazione.ProdottoValutato){
			"nomenclatura": func(p *valutazione.ProdottoValutato) {
				p.BOM.Nomenclatura = valutazione.VerificaAsse{Stato: valutazione.StatoAsseNonVerificabile, Conflitti: []string{"x"}}
			},
			"gerarchia": func(p *valutazione.ProdottoValutato) {
				p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseNonVerificabile, Conflitti: []string{"x"}}
			},
			"gerarchia in conflitto": func(p *valutazione.ProdottoValutato) {
				p.BOM.Gerarchia = valutazione.VerificaAsse{Stato: valutazione.StatoAsseConflitto}
			},
			"nomenclatura in conflitto": func(p *valutazione.ProdottoValutato) {
				p.BOM.Nomenclatura = valutazione.VerificaAsse{Stato: valutazione.StatoAsseConflitto}
			},
			"smistamento": func(p *valutazione.ProdottoValutato) {
				p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoInRevisione, Calcolata: true, Conflitti: []string{"x"}}
			},
			"smistamento in conflitto": func(p *valutazione.ProdottoValutato) {
				p.Smistamento = valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoConflitto, Calcolata: true}
			},
		} {
			pv := prodottoSintetico()
			cambia(&pv)
			_, _, motivi := valutazione.StatoDelProdotto(pv, valutazione.CondizioniNuove{})
			if !strings.Contains(motiviProdotto(motivi), "conflitto") {
				t.Errorf("%s: motivi %v", nome, motivi)
			}
		}
	})
}

// TestPO35IlProdottoNonHaLaFattibilitaAvviata (PO-35; T-E1-01, LD-10): un solo booleano, Verificato con il JSON
// prodotto_verificato; nessun campo per prodotto che dica la fattibilità avviata o «pronto» a parte (la fase è del thread:
// StatoFascicolo.FaseThread). Che Verificato valga se e solo se lo stato è pronto_fattibilita lo controllano gli aiuti
// valuta e calcola su tutti i casi delle prove (invariantiDelProdotto).
func TestPO35IlProdottoNonHaLaFattibilitaAvviata(t *testing.T) {
	tipo := reflect.TypeOf(valutazione.ProdottoValutato{})
	booleani := 0
	for i := 0; i < tipo.NumField(); i++ {
		f := tipo.Field(i)
		nome := strings.ToLower(f.Name + " " + f.Tag.Get("json"))
		if f.Type.Kind() == reflect.Bool {
			booleani++
		}
		if strings.Contains(nome, "avviat") || strings.Contains(nome, "pronto") || strings.Contains(nome, "fase") {
			t.Errorf("il campo %s del prodotto dice la fase o il «pronto» a parte (T-E1-01)", f.Name)
		}
	}
	if f, ok := tipo.FieldByName("Verificato"); !ok || booleani != 1 || f.Tag.Get("json") != "prodotto_verificato" {
		t.Errorf("Verificato %+v, %d booleani: uno solo (T-E1-01)", f, booleani)
	}
}

// TestPO18IlFileNuovoSulProdottoVerificato (PO-18, PO-02 nella parte L1, PO-22 nella parte di B6; T-E1-14, T-E1-08,
// T-E1-13): il prodotto altrimenti verificato è verificato e pronto, e il fascicolo del suo thread, con un target solo, è
// congelabile ma non congelato (gesto_non_registrato). Arriva un file nuovo pertinente, un secondo 2D dello sciolto (un
// TIFF della stessa identità): mai verificato in silenzio, da_riesaminare, con lo smistamento non verificato e nessun
// percorso di revisione; il fascicolo non è congelabile. Il 2D nuovo si vede nel nodo dello sciolto, con la relazione da
// confermare, e la revisione del componente non cambia. Poi il file è confermato sul componente (terminale): l'ultimo
// prodotto torna verificato, il fascicolo congelabile.
func TestPO18IlFileNuovoSulProdottoVerificato(t *testing.T) {
	th := scenaSmistamento(t)
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if statoProdotto(p) != "sì/pronto_fattibilita/" {
		t.Fatalf("il prodotto altrimenti verificato: %s (smistamento %+v, documenti %s, BOM %v)", statoProdotto(p), p.Smistamento, p.Documenti.Stato, p.BOM.Verificata)
	}
	if fa := et.Fascicolo; !fa.Congelabile || fa.Congelato || fa.MotivoNonCongelato != valutazione.NonCongelatoGestoNonRegistrato || fa.NumeroTarget != 1 ||
		!reflect.DeepEqual(fa.Pronti, []string{rifProdB}) || fa.MotivoNonCongelabile != "" || fa.Legacy != nil {
		t.Errorf("fascicolo %+v (PO-02: congelabile, non congelato senza il gesto)", fa)
	}

	conTIFF := scenaSmistamento(t)
	fileNelMessaggio(&conTIFF, fTIFF, idM1, "7120200A_2.tif", "tif", shaN(301), nil)
	conProposta(&conTIFF, uid(0xf21), fTIFF, "disegno_2d", "estensione", "aperta", nil, nil)
	et = esitoSmistamento(t, conTIFF)
	p = prodottoDi(t, et, rifProdB)
	if statoProdotto(p) != "no/da_riesaminare/smistamento_non_verificato" || !nonTerminale(p.Smistamento, fTIFF) ||
		p.Smistamento.Stato == valutazione.SmistamentoInRevisione || len(p.Smistamento.Percorsi) != 0 {
		t.Fatalf("con il TIFF nuovo: %s, smistamento %+v", statoProdotto(p), p.Smistamento)
	}
	if fa := et.Fascicolo; fa.Congelabile || fa.MotivoNonCongelabile != valutazione.NonCongelabileProdottiNonPronti || len(fa.Bloccati) != 1 ||
		motiviProdotto(fa.Bloccati[0].Motivi) != "smistamento_non_verificato" || fa.NumeroVerificati != 0 {
		t.Errorf("fascicolo %+v", fa)
	}
	n := nodoBOMDi(t, p, nodoB("#2"))
	if n.Disegni.Primario == nil || len(n.Disegni.Alternativi) != 1 || len(n.Disegni.RelazioniDaConfermare) != 1 ||
		n.Disegni.RelazioniDaConfermare[0].Stato != valutazione.StatoRelazioneDaConfermare {
		t.Errorf("i 2D del nodo dello sciolto %+v: il TIFF accanto, con la relazione da confermare (T-E1-08)", n.Disegni)
	}
	if c := n.Nodo.Codice.Confermato; c == nil || c.Rev != nil || c.Codice != "7120200A" {
		t.Errorf("il codice deciso dello sciolto %+v: nessuna revisione cambiata", c)
	}

	d := documento2D(dTIFFNuovo, cSciolto, fTIFF, "7120200A_2.tif", "tif", shaN(301))
	conTIFF.Documenti = append(conTIFF.Documenti, d)
	conTIFF.Proposte[len(conTIFF.Proposte)-1].Stato = "confermata"
	et = esitoSmistamento(t, conTIFF)
	if p := prodottoDi(t, et, rifProdB); statoProdotto(p) != "sì/pronto_fattibilita/" || !et.Fascicolo.Congelabile {
		t.Errorf("il TIFF confermato: %s, fascicolo %+v", statoProdotto(p), et.Fascicolo)
	}
}

// TestPO25LoStatoConIlNuovoCAD (PO-25, la parte dello stato; T-B0-29, T-E1-14): P altrimenti verificato, con la fonte
// confermata e il suo cad_3d; arriva uno STEP nuovo con la radice di P: il conflitto nuovo_file porta lo smistamento in
// conflitto, e P è da_riesaminare, con i motivi dello smistamento e del conflitto. La sua impronta non cambia (lo STEP
// nuovo non è un dato deciso); il secondo prodotto non cambia, né lo stato né l'impronta.
func TestPO25LoStatoConIlNuovoCAD(t *testing.T) {
	base := scenaSmistamento(t)
	secondoProdotto(t, &base)
	prima := esitoSmistamento(t, base)
	th := base
	th.Allegati = append([]fotorfq.Allegato(nil), base.Allegati...)
	th.Fatti = map[string]fotorfq.Fatti{}
	for k, v := range base.Fatti {
		th.Fatti[k] = v
	}
	f := fattiStepF(t, shaN(302), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
	fileNelMessaggio(&th, uid(0xf22), idM1, "7120100A_2.stp", "stp", shaN(302), &f)
	conProposta(&th, uid(0xf23), uid(0xf22), "cad_3d", "step", "aperta", nil, nil)
	dopo := esitoSmistamento(t, th)
	p, p0 := prodottoDi(t, dopo, rifProdB), prodottoDi(t, prima, rifProdB)
	if statoProdotto(p0) != "sì/pronto_fattibilita/" || statoProdotto(p) != "no/da_riesaminare/smistamento_non_verificato conflitto" {
		t.Fatalf("prima %s, dopo %s", statoProdotto(p0), statoProdotto(p))
	}
	if p.Impronta != p0.Impronta {
		t.Errorf("l'impronta di P cambia con uno STEP che non è deciso: %s → %s (T-B0-29)", p0.Impronta, p.Impronta)
	}
	if n := nodoBOMDi(t, p, nodoB("#1")); motiviNodo(n) != "associazione_non_confermata nuovo_file_su_componente_deciso" {
		t.Errorf("il nodo della radice, il componente del conflitto nuovo_file: motivi %q", motiviNodo(n))
	}
	q, q0 := prodottoDi(t, dopo, rifP2), prodottoDi(t, prima, rifP2)
	if statoProdotto(q) != statoProdotto(q0) || q.Impronta != q0.Impronta || q.Stato != valutazione.ProdottoNonPronto {
		t.Errorf("il secondo prodotto cambia: %s %s → %s %s", statoProdotto(q0), q0.Impronta, statoProdotto(q), q.Impronta)
	}
}

// TestPO19IlTargetConfermatoNonVerificato (PO-19, la parte di B6; R78): il codice estratto dalla mail e accettato alla
// creazione della RFQ è un target confermato, ma non è verificato: non_pronto, con i motivi degli assi che mancano; il
// candidato non confermato della stessa mail non è un target (B1).
func TestPO19IlTargetConfermatoNonVerificato(t *testing.T) {
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A2 e 7120200A1.\r\nGrazie")
	th.Identificativi = []fotorfq.Identificativo{confermato("7120100A"), proposto("7120200A")}
	th.Fabbisogni = fabbisogniDefault()
	et := esitoSmistamento(t, th)
	if len(et.ProdottiValutati) != 1 {
		t.Fatalf("prodotti %d: solo il confermato è un target (R60 A)", len(et.ProdottiValutati))
	}
	p := et.ProdottiValutati[0]
	if !valutazione.TargetConfermato(p) || statoProdotto(p) != "no/non_pronto/fonte_strutturale_step_mancante_o_non_confermata nomenclatura_non_verificata "+
		"gerarchia_non_verificata smistamento_non_verificato documentazione_non_calcolabile" {
		t.Errorf("il target confermato senza componente: %s", statoProdotto(p))
	}
	if fa := et.Fascicolo; fa.Congelabile || fa.MotivoNonCongelabile != valutazione.NonCongelabileProdottiNonPronti {
		t.Errorf("fascicolo %+v", fa)
	}
}

// TestR81UnaVoceVecchiaDietroIlPerimetroAperto (R-81 della revisione di V3; T-E1-14, R94 A, R72 D, T-E1-10): il 2D del
// finito confermato e mai analizzato (i fatti non ci sono: una voce certa da_verificare, contenuto_non_analizzato) è una
// condizione vecchia. A perimetro chiuso dà incompleta, quindi non_pronto; una condizione nuova che apre il perimetro
// (una rimozione proposta aperta, la fonte superata con K-02) non la nasconde: il prodotto resta non_pronto. Con il 2D
// analizzato, la stessa condizione nuova dà da_riesaminare (il controllo).
func TestR81UnaVoceVecchiaDietroIlPerimetroAperto(t *testing.T) {
	finitoNonAnalizzato := func(t *testing.T) fotorfq.Thread {
		th := scenaCompletezza(t, false)
		conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "assieme-acme.pdf", sha1, nil)
		conDocumento2D(&th, dPDF2, cSciolto, aPDF2, "disegno-acme.pdf", sha2, scansione(t, sha2))
		return th
	}
	rimozione := func(th *fotorfq.Thread) {
		th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cSciolto, QtaWorking: 2}}
	}
	k02 := func(t *testing.T, th *fotorfq.Thread) {
		conFonteSuperata(t, th, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}}, false)
	}
	vocePresente := func(t *testing.T, p valutazione.ProdottoValutato, atteso string) {
		t.Helper()
		if v := voceDi(t, p.Documenti, cProdotto, "disegno_2d"); esitoDi(v) != atteso {
			t.Fatalf("la voce del 2D del finito %s, attesa %s", esitoDi(v), atteso)
		}
	}
	casi := []struct {
		nome      string
		scena     func(*testing.T) fotorfq.Thread
		voce      string
		documenti string
		atteso    string
	}{
		{"a perimetro chiuso: incompleta", finitoNonAnalizzato, "da_verificare/contenuto_non_analizzato",
			"incompleta/voce_non_presente chiuso/", "no/non_pronto/documentazione_incompleta"},
		{"RV3-01: la rimozione aperta non nasconde la voce vecchia", func(t *testing.T) fotorfq.Thread {
			th := finitoNonAnalizzato(t)
			rimozione(&th)
			return th
		}, "da_verificare/contenuto_non_analizzato", "non_calcolabile/perimetro_aperto aperto/rimozioni_aperte",
			"no/non_pronto/gerarchia_non_verificata documentazione_non_calcolabile conflitto"},
		{"il controllo: la rimozione aperta con il 2D analizzato", func(t *testing.T) fotorfq.Thread {
			th := scenaSmistamento(t)
			rimozione(&th)
			return th
		}, "presente/", "non_calcolabile/perimetro_aperto aperto/rimozioni_aperte",
			"no/da_riesaminare/gerarchia_non_verificata documentazione_non_calcolabile conflitto"},
		{"RV3-02: la fonte superata (K-02) non nasconde la voce vecchia", func(t *testing.T) fotorfq.Thread {
			th := finitoNonAnalizzato(t)
			k02(t, &th)
			return th
		}, "da_verificare/contenuto_non_analizzato", "non_calcolabile/perimetro_aperto aperto/fonte_non_confermata",
			"no/non_pronto/fonte_superata gerarchia_non_verificata documentazione_non_calcolabile"},
		{"il controllo: la fonte superata (K-02) con il 2D analizzato", func(t *testing.T) fotorfq.Thread {
			th := scenaSmistamento(t)
			k02(t, &th)
			return th
		}, "presente/", "non_calcolabile/perimetro_aperto aperto/fonte_non_confermata",
			"no/da_riesaminare/fonte_superata gerarchia_non_verificata documentazione_non_calcolabile"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			p := prodottoDi(t, esitoSmistamento(t, c.scena(t)), rifProdB)
			vocePresente(t, p, c.voce)
			if statoDi(p.Documenti) != c.documenti || statoProdotto(p) != c.atteso {
				t.Errorf("documenti %s, prodotto %s; attesi %s, %s", statoDi(p.Documenti), statoProdotto(p), c.documenti, c.atteso)
			}
		})
	}
}

// ---- la fonte superata e R111 ----

// conFonteSuperata: lo STEP confermato del prodotto (dStepB) sostituito da uno nuovo (dStepNuovo, corrente, cad_3d sul
// finito), con la vista riferimento_superato (R65 A): la fonte è in attesa di conferma. Lo STEP nuovo ha i nodi dati; i
// suoi fatti ci sono se conFatti. Lo STEP di prima resta fra gli allegati (con i suoi fatti, salvo che la prova li tolga).
func conFonteSuperata(t *testing.T, th *fotorfq.Thread, nodi []nodoF, archi []arcoF, conFatti bool) {
	t.Helper()
	var f *fotorfq.Fatti
	if conFatti {
		x := fattiStepF(t, shaN(310), nodi, archi)
		f = &x
	}
	conAllegato(th, allegato(aStepNuovo, 30, "7120100A_2.stp", "stp", shaN(310)), f)
	cp, nuovo := cProdotto, dStepNuovo
	th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dStepNuovo, ThreadID: threadACME, ComponenteID: &cp, Tipo: "cad_3d",
		NomeFile: "7120100A_2.stp", Estensione: "stp", Sha256: shaN(310), StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME.Add(4 * time.Hour),
		Allegati: []uuid.UUID{aStepNuovo}})
	for i := range th.Documenti {
		if th.Documenti[i].ID == dStepB {
			th.Documenti[i].SostituitoDa = &nuovo
		}
	}
	th.StepProdotto[0].Esito, th.StepProdotto[0].NStepCorrenti = "riferimento_superato", 1
}

// TestR111LaFonteSuperataNelleDueDirezioni (R111 A, precisata dall'utente il 07/10; T-E1-14, T-E1R-04, K-02): il prodotto
// altrimenti verificato con lo STEP confermato sostituito da uno nuovo, che aggiunge un nodo.
//   - A: la fotografia porta tutte e due le versioni (le due strutture candidate): le voci da decidere vengono solo dal
//     nuovo STEP, quindi la nomenclatura e la gerarchia sono ferme solo per condizioni nuove; con la fonte superata e il
//     perimetro aperto per la fonte, il prodotto è da_riesaminare;
//   - B: lo STEP di prima non si legge più (senza i suoi fatti): le stesse voci non si possono confrontare, e il
//     prodotto è non_pronto (il limite dichiarato);
//   - una voce vecchia (una relazione confermata che il segno di «Conferma l'albero» non copre) basta per non_pronto;
//   - K-02 senza R111: lo STEP nuovo non si legge (nessuna struttura nuova), lo STEP di prima è tutto deciso; la gerarchia è
//     ferma solo per fonte_non_confermata dopo la fonte superata, che è una condizione nuova: da_riesaminare;
//   - la regola effettiva (R-85 della revisione di V3): le chiavi comprendono lo sha256, quindi ogni voce del nuovo STEP è
//     nuova, anche con gli stessi codici e la stessa struttura dello STEP di prima, o con un nodo diverso.
func TestR111LaFonteSuperataNelleDueDirezioni(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}}
	t.Run("A: tutte e due le versioni", func(t *testing.T) {
		th := scenaSmistamento(t)
		conFonteSuperata(t, &th, nodi, archi, true)
		et := esitoSmistamento(t, th)
		p := prodottoDi(t, et, rifProdB)
		if p.Fonte.Motivo != valutazione.MotivoFonteRiferimentoSuperato || p.Fonte.Riferimento == nil || !p.Fonte.Riferimento.Superato {
			t.Fatalf("fonte %+v", p.Fonte)
		}
		if asse(p.BOM.Nomenclatura) != "da_verificare/da_decidere" || asse(p.BOM.Gerarchia) != "da_verificare/da_decidere" ||
			statoDi(p.Documenti) != "non_calcolabile/perimetro_aperto aperto/fonte_non_confermata" {
			t.Fatalf("BOM %s %s, documenti %s", asse(p.BOM.Nomenclatura), asse(p.BOM.Gerarchia), statoDi(p.Documenti))
		}
		if statoProdotto(p) != "no/da_riesaminare/fonte_superata nomenclatura_non_verificata gerarchia_non_verificata documentazione_non_calcolabile" {
			t.Errorf("R111 A: %s (smistamento %+v)", statoProdotto(p), p.Smistamento)
		}
	})
	t.Run("B: lo STEP di prima non si legge", func(t *testing.T) {
		th := scenaSmistamento(t)
		conFonteSuperata(t, &th, nodi, archi, true)
		delete(th.Fatti, shaStepB)
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if asse(p.BOM.Nomenclatura) != "da_verificare/da_decidere" || p.Stato != valutazione.ProdottoNonPronto ||
			!strings.HasPrefix(motiviProdotto(p.Motivi), "fonte_superata nomenclatura_non_verificata") {
			t.Errorf("R111 B: %s, nomenclatura %s", statoProdotto(p), asse(p.BOM.Nomenclatura))
		}
	})
	t.Run("una voce vecchia", func(t *testing.T) {
		th := scenaSmistamento(t)
		conFonteSuperata(t, &th, nodi, archi, true)
		confermaComponente(&th, cTerzoSt, "7120700A", cProdotto, 1)
		conDocumento2D(&th, dStepTerzo, cTerzoSt, uid(0xf08), "terzo-acme.pdf", shaN(311), scansione(t, shaN(311)))
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if p.Stato != valutazione.ProdottoNonPronto || p.Documenti.Stato != valutazione.DocumentiNonCalcolabile ||
			!strings.HasPrefix(motiviProdotto(p.Motivi), "fonte_superata nomenclatura_non_verificata") {
			t.Errorf("con la relazione confermata non coperta dal segno: %s, documenti %s", statoProdotto(p), statoDi(p.Documenti))
		}
	})
	t.Run("A senza il gesto di «Conferma l'albero»: il gesto che manca è vecchio", func(t *testing.T) {
		th := scenaSmistamento(t)
		conFonteSuperata(t, &th, nodi, archi, true)
		for i := range th.RigheRelazioneProposta {
			th.RigheRelazioneProposta[i].Albero = nil
		}
		for i := range th.RigheComponenteProposta {
			th.RigheComponenteProposta[i].Albero = nil
		}
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if asse(p.BOM.Nomenclatura) != "da_verificare/nessun_gesto" || p.Stato != valutazione.ProdottoNonPronto {
			t.Errorf("senza il gesto: %s, nomenclatura %s", statoProdotto(p), asse(p.BOM.Nomenclatura))
		}
	})
	t.Run("K-02: lo STEP nuovo non si legge, lo STEP di prima è deciso", func(t *testing.T) {
		th := scenaSmistamento(t)
		conFonteSuperata(t, &th, nodi, archi, false)
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if asse(p.BOM.Nomenclatura) != "verificata/" || asse(p.BOM.Gerarchia) != "da_verificare/fonte_non_confermata" ||
			statoProdotto(p) != "no/da_riesaminare/fonte_superata gerarchia_non_verificata documentazione_non_calcolabile" {
			t.Errorf("K-02 dopo la fonte superata: %s, BOM %s %s", statoProdotto(p), asse(p.BOM.Nomenclatura), asse(p.BOM.Gerarchia))
		}
	})
	t.Run("R-85: ogni voce del nuovo STEP è nuova, anche con gli stessi codici", func(t *testing.T) {
		for nome, nuovo := range map[string]struct {
			nodi  []nodoF
			archi []arcoF
		}{
			"lo stesso STEP, con un altro contenuto": {[]nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}}},
			"lo STEP con un nodo diverso":            {[]nodoF{{"#1", "7120100A", ""}, {"#9", "7120900A", ""}}, []arcoF{{"#1", "#9", 2}}},
		} {
			th := scenaSmistamento(t)
			conFonteSuperata(t, &th, nuovo.nodi, nuovo.archi, true)
			p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
			if asse(p.BOM.Nomenclatura) != "da_verificare/da_decidere" ||
				statoProdotto(p) != "no/da_riesaminare/fonte_superata nomenclatura_non_verificata gerarchia_non_verificata documentazione_non_calcolabile" {
				t.Errorf("%s: %s, BOM %s (%d) %s (%d)", nome, statoProdotto(p), asse(p.BOM.Nomenclatura), p.BOM.Nomenclatura.DaDecidere,
					asse(p.BOM.Gerarchia), p.BOM.Gerarchia.DaDecidere)
			}
		}
	})
	t.Run("la fonte non superata: le stesse voci da decidere sono vecchie", func(t *testing.T) {
		th := scenaTreNodi(t, true)
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if p.Stato != valutazione.ProdottoNonPronto || asse(p.BOM.Nomenclatura) != "da_verificare/da_decidere" {
			t.Errorf("con la fonte confermata e due nodi da decidere: %s, nomenclatura %s", statoProdotto(p), asse(p.BOM.Nomenclatura))
		}
	})
}

// TestR111SullaFunzione (R111 A, precisata dall'utente il 07/10): la funzione sola, nelle due direzioni e nei casi di
// confine: la catena delle sostituzioni (il documento corrente in fondo; una catena rotta, un ciclo, un documento non
// sostituito), le due versioni (senza una delle due strutture vale la B), le voci (le righe dei nodi e degli archi del
// nuovo STEP, i loro Rif, una voce dello STEP di prima, una relazione confermata, una voce sconosciuta), lo stesso
// contenuto nei due documenti.
func TestR111SullaFunzione(t *testing.T) {
	shaVecchio, shaNuovo, shaTerzo := shaN(320), shaN(321), shaN(322)
	dVecchio, dNuovo, dTerzo := uid(0xf31), uid(0xf32), uid(0xf33)
	aNuovo := uid(0xf34)
	rigaNuova := uid(0xf35)
	nodo := func(sha, chiave string) string { return ancoraggio.RifNodo(sha, chiave) }
	struttura := func(sha string, nodi ...string) ancoraggio.StrutturaProdotto {
		s := ancoraggio.StrutturaProdotto{Target: rifProdB, Sha256: sha, Radice: nodo(sha, "#1"), Stato: ancoraggio.StatoStrutturaCandidata}
		for _, k := range nodi {
			s.Nodi = append(s.Nodi, ancoraggio.NodoProposto{Rif: nodo(sha, k)})
			if k != "#1" {
				s.Archi = append(s.Archi, ancoraggio.ArcoProposto{Padre: nodo(sha, "#1"), Figlio: nodo(sha, k)})
			}
		}
		return s
	}
	documento := func(id uuid.UUID, sha string, sostituito *uuid.UUID) fotorfq.DocumentoConfermato {
		return fotorfq.DocumentoConfermato{ID: id, Sha256: sha, SostituitoDa: sostituito}
	}
	superata := func() valutazione.ProdottoValutato {
		pv := prodottoSintetico()
		pv.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteRiferimentoSuperato, Calcolata: true,
			Riferimento: &ancoraggio.RiferimentoFonte{DocumentoID: dVecchio, Sha256: shaVecchio, Superato: true}}
		return pv
	}
	thread := func(docs ...fotorfq.DocumentoConfermato) fotorfq.Thread {
		s := shaNuovo
		return fotorfq.Thread{Documenti: docs, Allegati: []fotorfq.Allegato{{ID: aNuovo, Sha256: &s}},
			RigheComponenteProposta: []fotorfq.RigaComponenteProposta{{ID: rigaNuova, AllegatoID: aNuovo, Sha256: shaNuovo, Chiave: "#3"}},
			RigheRelazioneProposta:  []fotorfq.RigaRelazioneProposta{{AllegatoID: aNuovo, PadreChiave: "#1", FiglioChiave: "#3"}}}
	}
	nuovo := dNuovo
	catena := thread(documento(dVecchio, shaVecchio, &nuovo), documento(dNuovo, shaNuovo, nil))
	due := []ancoraggio.StrutturaProdotto{struttura(shaVecchio, "#1", "#2"), struttura(shaNuovo, "#1", "#2", "#3")}
	vociNuove := []string{ancoraggio.RifRigaProposta(rigaNuova), "relazione_proposta:" + aNuovo.String() + ":#1>#3", nodo(shaNuovo, "#2"),
		valutazione.RifArco(nodo(shaNuovo, "#1"), nodo(shaNuovo, "#2"))}
	f := valutazione.VociNuoveDopoLaFonteSuperataR111PerProva
	if !f(vociNuove, superata(), due, catena) {
		t.Error("A: le righe e i Rif del nuovo STEP, che lo STEP di prima non aveva, sono nuovi")
	}
	if !f(nil, superata(), due, catena) {
		t.Error("nessuna voce: niente di vecchio")
	}
	terzo := dTerzo
	casi := []struct {
		nome      string
		voci      []string
		pv        valutazione.ProdottoValutato
		strutture []ancoraggio.StrutturaProdotto
		th        fotorfq.Thread
	}{
		{"una voce dello STEP di prima", []string{nodo(shaVecchio, "#2")}, superata(), due, catena},
		{"una relazione confermata che il segno non copre", []string{valutazione.RifArco("componente:a", "componente:b")}, superata(), due, catena},
		{"una voce che non si riconosce", []string{"qualcosa"}, superata(), due, catena},
		{"una riga di un altro STEP", []string{nodo(shaTerzo, "#2")}, superata(), append(due, struttura(shaTerzo, "#1", "#2")), catena},
		{"la fonte non superata", vociNuove, prodottoSintetico(), due, catena},
		{"la fonte confermata con il riferimento sostituito: R111 vale solo per la fonte superata", vociNuove, func() valutazione.ProdottoValutato {
			p := prodottoSintetico()
			p.Fonte.Riferimento = &ancoraggio.RiferimentoFonte{DocumentoID: dVecchio, Sha256: shaVecchio, Superato: true}
			return p
		}(), due, catena},
		{"la fonte superata senza il riferimento", vociNuove, func() valutazione.ProdottoValutato {
			p := superata()
			p.Fonte.Riferimento = nil
			return p
		}(), due, catena},
		{"B: senza la struttura dello STEP di prima", vociNuove, superata(), due[1:], catena},
		{"B: senza la struttura del nuovo STEP", vociNuove, superata(), due[:1], catena},
		{"il documento del riferimento non è sostituito", vociNuove, superata(), due, thread(documento(dVecchio, shaVecchio, nil), documento(dNuovo, shaNuovo, nil))},
		{"il documento del riferimento non è nella fotografia", vociNuove, superata(), due, thread(documento(dNuovo, shaNuovo, nil))},
		{"la catena rotta", vociNuove, superata(), due, thread(documento(dVecchio, shaVecchio, &nuovo))},
		{"il ciclo", vociNuove, superata(), due, thread(documento(dVecchio, shaVecchio, &nuovo), documento(dNuovo, shaNuovo, func() *uuid.UUID { v := dVecchio; return &v }()))},
		{"il nuovo è il terzo: le voci del secondo non sono del nuovo STEP", vociNuove, superata(), due,
			thread(documento(dVecchio, shaVecchio, &nuovo), documento(dNuovo, shaNuovo, &terzo), documento(dTerzo, shaTerzo, nil))},
		{"lo stesso contenuto nei due documenti: lo STEP di prima ha già i nodi", []string{nodo(shaVecchio, "#2")}, superata(),
			[]ancoraggio.StrutturaProdotto{struttura(shaVecchio, "#1", "#2")}, thread(documento(dVecchio, shaVecchio, &nuovo), documento(dNuovo, shaVecchio, nil))},
	}
	for _, c := range casi {
		if f(c.voci, c.pv, c.strutture, c.th) {
			t.Errorf("%s: le voci valgono come nuove", c.nome)
		}
	}
	if f([]string{nodo(shaNuovo, "#3")}, superata(), append(due, ancoraggio.StrutturaProdotto{Target: rifP2, Sha256: shaVecchio,
		Nodi: []ancoraggio.NodoProposto{{Rif: nodo(shaNuovo, "#3")}}}), catena) == false {
		t.Error("la struttura di un altro prodotto non conta fra le versioni del prodotto")
	}
	cicloSenzaCorrente := thread(documento(dVecchio, shaVecchio, &nuovo), documento(dNuovo, shaNuovo, &terzo), documento(dTerzo, shaTerzo, &nuovo))
	if f([]string{nodo(shaNuovo, "#3")}, superata(), due, cicloSenzaCorrente) {
		t.Error("un ciclo di sostituzioni senza un documento corrente: nessun nuovo STEP")
	}
	terzoInCatena := thread(documento(dVecchio, shaVecchio, &nuovo), documento(dNuovo, shaNuovo, &terzo), documento(dTerzo, shaTerzo, nil))
	if !f([]string{nodo(shaTerzo, "#2")}, superata(), append(due, struttura(shaTerzo, "#1", "#2")), terzoInCatena) {
		t.Error("il documento corrente in fondo alla catena è il nuovo STEP")
	}
}

// TestCondizioniNuoveDellaBOM (T-E1-14; R111, K-02, R80): l'adattatore delle condizioni nuove sui gesti e sulla struttura
// della verifica: lo stato di base senza i conflitti decide; la gerarchia vuota del modello nuovo segue la nomenclatura.
func TestCondizioniNuoveDellaBOM(t *testing.T) {
	f := valutazione.CondizioniNuoveDellaBOMPerProva
	gesto := &valutazione.GestoVerifica{Nodi: []string{"n:1", "n:2"}, Archi: []string{"arco:n:1>n:2"}}
	legacy := valutazione.GestiVerificaBOM{Nomenclatura: gesto, Gerarchia: gesto, Legacy: true}
	base := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true, Radici: []string{"n:1"},
		Nodi: []string{"n:1", "n:2"}, Archi: []string{"arco:n:1>n:2"}}
	conflitto := []valutazione.Conflitto{{Asse: valutazione.AsseNomenclatura, Rif: "componente:x"}, {Asse: valutazione.AsseGerarchia, Rif: "arco:y"}}
	pv := prodottoSintetico()
	casi := []struct {
		nome   string
		g      valutazione.GestiVerificaBOM
		s      func() valutazione.StrutturaDaVerificare
		atteso valutazione.CondizioniNuove
	}{
		{"verificate: niente da dire, le condizioni valgono", legacy, func() valutazione.StrutturaDaVerificare { return base }, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}},
		{"in conflitto, verificate sotto: ferme solo per i conflitti", legacy, func() valutazione.StrutturaDaVerificare {
			s := base
			s.Conflitti = conflitto
			return s
		}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}},
		{"in conflitto senza il gesto: il gesto che manca è vecchio", valutazione.GestiVerificaBOM{Legacy: true}, func() valutazione.StrutturaDaVerificare {
			s := base
			s.Conflitti = conflitto
			return s
		}, valutazione.CondizioniNuove{}},
		{"una riga da decidere con la fonte confermata: vecchia", legacy, func() valutazione.StrutturaDaVerificare {
			s := base
			s.RigheDaDecidere = []string{"componente_proposta:x"}
			return s
		}, valutazione.CondizioniNuove{}},
		{"senza figli, nel legacy: non_verificabile è vecchio", legacy, func() valutazione.StrutturaDaVerificare {
			s := base
			s.Nodi, s.Archi = []string{"n:1"}, nil
			return s
		}, valutazione.CondizioniNuove{}},
		{"la gerarchia senza la fonte, la fonte non superata (K-02)", legacy, func() valutazione.StrutturaDaVerificare {
			s := base
			s.FonteConfermata, s.BOMDiLavoro = false, false
			return s
		}, valutazione.CondizioniNuove{Nomenclatura: true}},
		{"la gerarchia legacy senza la BOM di lavoro (T-B5-17): vecchia", legacy, func() valutazione.StrutturaDaVerificare {
			s := base
			s.BOMDiLavoro = false
			return s
		}, valutazione.CondizioniNuove{Nomenclatura: true}},
		{"il modello nuovo: la gerarchia vuota aspetta la nomenclatura, che aspetta un nodo (vecchio)", valutazione.GestiVerificaBOM{
			Nomenclatura: &valutazione.GestoVerifica{Nodi: []string{"n:1"}}, Gerarchia: &valutazione.GestoVerifica{}}, func() valutazione.StrutturaDaVerificare {
			s := base
			s.Archi = nil
			return s
		}, valutazione.CondizioniNuove{}},
		{"il modello nuovo: la gerarchia vuota aspetta la nomenclatura in conflitto (nuovo)", valutazione.GestiVerificaBOM{
			Nomenclatura: &valutazione.GestoVerifica{Nodi: []string{"n:1", "n:2"}}, Gerarchia: &valutazione.GestoVerifica{}}, func() valutazione.StrutturaDaVerificare {
			s := base
			s.Archi, s.Conflitti = nil, conflitto[:1]
			return s
		}, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}},
	}
	for _, c := range casi {
		if got := f(pv, c.g, c.s(), nil, fotorfq.Thread{}); got != c.atteso {
			t.Errorf("%s: %+v, attese %+v", c.nome, got, c.atteso)
		}
	}
	sup := pv
	sup.Fonte = valutazione.FonteProdotto{Stato: valutazione.FonteInAttesaDiConferma, Motivo: valutazione.MotivoFonteRiferimentoSuperato, Calcolata: true}

	// Con le due versioni (R111): una riga da decidere del nuovo STEP è nuova per la nomenclatura e per la gerarchia; un arco
	// da decidere dello STEP di prima, che conta solo per la gerarchia, la ferma per una condizione vecchia.
	vecchio, nuovo := shaN(360), shaN(361)
	dV, dN := uid(0xf94), uid(0xf95)
	strutture := []ancoraggio.StrutturaProdotto{
		{Target: rifProdB, Sha256: vecchio, Nodi: []ancoraggio.NodoProposto{{Rif: ancoraggio.RifNodo(vecchio, "#1")}, {Rif: ancoraggio.RifNodo(vecchio, "#2")}},
			Archi: []ancoraggio.ArcoProposto{{Padre: ancoraggio.RifNodo(vecchio, "#1"), Figlio: ancoraggio.RifNodo(vecchio, "#2")}}},
		{Target: rifProdB, Sha256: nuovo, Nodi: []ancoraggio.NodoProposto{{Rif: ancoraggio.RifNodo(nuovo, "#1")}, {Rif: ancoraggio.RifNodo(nuovo, "#3")}}}}
	th := fotorfq.Thread{Documenti: []fotorfq.DocumentoConfermato{{ID: dV, Sha256: vecchio, SostituitoDa: &dN}, {ID: dN, Sha256: nuovo}}}
	conDue := sup
	conDue.Fonte.Riferimento = &ancoraggio.RiferimentoFonte{DocumentoID: dV, Sha256: vecchio, Superato: true}
	due := base
	due.FonteConfermata, due.BOMDiLavoro = false, false
	due.RigheDaDecidere = []string{ancoraggio.RifNodo(nuovo, "#3")}
	due.ArchiDaDecidere = []string{valutazione.RifArco(ancoraggio.RifNodo(vecchio, "#1"), ancoraggio.RifNodo(vecchio, "#2"))}
	if got := f(conDue, legacy, due, strutture, th); got != (valutazione.CondizioniNuove{Nomenclatura: true}) {
		t.Errorf("la riga nuova e l'arco vecchio: %+v", got)
	}
	due.ArchiDaDecidere = nil
	if got := f(conDue, legacy, due, strutture, th); got != (valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}) {
		t.Errorf("la sola riga nuova: %+v", got)
	}
	// Senza il gesto, le stesse voci nuove non bastano: il gesto che manca è una condizione vecchia (nessun_gesto non va a R111).
	if got := f(conDue, valutazione.GestiVerificaBOM{Legacy: true}, due, strutture, th); got != (valutazione.CondizioniNuove{}) {
		t.Errorf("la sola riga nuova, senza il gesto: %+v", got)
	}

	s := base
	s.FonteConfermata, s.BOMDiLavoro = false, false
	if got := f(sup, legacy, s, nil, fotorfq.Thread{}); got != (valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}) {
		t.Errorf("K-02 dopo la fonte superata: %+v (T-E1-14)", got)
	}
	s.RigheDaDecidere = []string{"componente_proposta:x"}
	if got := f(sup, legacy, s, nil, fotorfq.Thread{}); got != (valutazione.CondizioniNuove{}) {
		t.Errorf("una voce da decidere senza le due versioni (R111 B): %+v", got)
	}
}

// TestLoStatoDelProdottoEDeterministico: gli stessi ingressi, anche permutati, danno lo stesso stato e la stessa impronta
// di ogni prodotto, con più prodotti e la fonte superata.
func TestLoStatoDelProdottoEDeterministico(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th, "7120200A")
	conFonteSuperata(t, &th, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}, []arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}}, true)
	vistaCome(&th)
	f, in := fotografia(th), valutazione.Ingressi{Versione: valutazione.VersioneCasi}
	r := insiemeACME(t)
	a := esitoThread(t, calcola(t, f, r, in), threadACME)
	g, gin := permuta(f, in)
	b := esitoThread(t, calcola(t, g, r, gin), threadACME)
	if canonicoDi(t, a.ProdottiValutati) != canonicoDi(t, b.ProdottiValutati) || canonicoDi(t, a.Fascicolo) != canonicoDi(t, b.Fascicolo) {
		t.Error("gli ingressi permutati cambiano i prodotti o il fascicolo")
	}
	if len(a.ProdottiValutati) != 2 || a.ProdottiValutati[0].Impronta == a.ProdottiValutati[1].Impronta {
		t.Errorf("due prodotti con due impronte: %+v", a.ProdottiValutati)
	}
}
