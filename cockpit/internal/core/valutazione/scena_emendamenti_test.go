// L1 — gli aiuti delle prove degli emendamenti EB7 (P7e; ratificati dall'utente l'08/10): le scene ACME della struttura
// candidata, dei due STEP candidati, della BOM confermata senza STEP e del componente aggiunto a mano accanto alla BOM di
// lavoro, e l'impronta degli assi del prodotto, per provare che i campi nuovi non cambiano stato, assi e autorità. Gli
// aiuti non usano i campi nuovi: si compilano anche con il codice di prima di P7e, e così gli assi «di prima» si
// calcolano con quel codice (resoconto di P7e).
package valutazione_test

import (
	"testing"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example), con le famiglie acme-catena (712xxxx con il marcatore A
// o B) e acme-minuteria (712xxxx senza marcatore); i nomi dei file sono di fantasia, gli UUID sono
// 00000000-0000-4000-8000-0000000000nn. Le prove citano i requisiti (EB7-1, EB7-2, EB7-4; R90, T-B4-38, T-B6-11), mai
// i casi degli attesi.

// Gli ID delle scene degli emendamenti: il PDF candidato del figlio e la sua proposta, il secondo STEP del prodotto, il
// componente aggiunto a mano e quello sotto lo sciolto, il secondo prodotto senza STEP.
var (
	aPDFCand    = uid(0xe701)
	pPDFCand    = uid(0xe702)
	aStepB2     = uid(0xe711)
	cSottoSciol = uid(0xe721)
	cAMano      = uid(0xe722)
	cP2SenzaSTP = uid(0xe731)
	shaPDFCand  = shaN(702)
	shaStepB2   = shaN(711)
	rifP2Senza  = "componente:" + uid(0xe731).String()
)

// insiemeConMinuteria: le regole ACME con la famiglia della catena e quella della minuteria (la categoria minuteria della
// grammatica, che va in Classificazione.Proposta e non esenta mai).
func insiemeConMinuteria(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	return insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", famigliaCatena(), famigliaMinuteria())})
}

// scenaCandidata: lo STEP dell'assieme (la radice #1 7120100A, il figlio #2 7120200A, la minuteria #3 7129001) senza il
// gesto 3 (senzaFonte): la struttura è candidata, e la fonte in attesa di conferma. Il finito ha il suo 2D confermato e
// una descrizione; il PDF del figlio, con il cartiglio, è proposto come 2D e nessuno l'ha deciso.
func scenaCandidata(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7129001", ""}}, []arcoF{{"#1", "#2", 2}, {"#1", "#3", 4}})
	senzaFonte(&th)
	th.Fabbisogni = fabbisogniDefault()
	th.Componenti[0].Descrizione = testo("Assieme ACME")
	conDocumento2D(&th, dPDF1, cProdotto, aPDF1, "assieme-acme.pdf", sha1, scansione(t, sha1))
	f := fattiPDF(t, shaPDFCand, "7120200A1")
	fileNelMessaggio(&th, aPDFCand, idM1, "7120200A_1.pdf", "pdf", shaPDFCand, &f)
	conProposta(&th, pPDFCand, aPDFCand, "disegno_2d", "nome_file", "aperta", nil, nil)
	return th
}

// conSecondoSTEP: un secondo STEP dello stesso prodotto (la radice 7120100A con il figlio 7120200A), fra gli allegati, non
// scelto: un'altra struttura candidata.
func conSecondoSTEP(t *testing.T, th *fotorfq.Thread) {
	t.Helper()
	f := fattiStepF(t, shaStepB2, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 1}})
	conAllegato(th, allegato(aStepB2, 8, "7120100A_2.stp", "stp", shaStepB2), &f)
}

// scenaSenzaSTEP: il finito manuale 7120100A con la sua BOM confermata senza nessuno STEP: lo sciolto 7120200A sotto il
// finito (la relazione confermata con la quantità 3), con il suo 2D confermato; la vista non ha nessuno STEP del
// prodotto (mancante).
func scenaSenzaSTEP(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A.\r\nGrazie")
	th.Componenti = []fotorfq.Componente{componente(cProdotto, "7120100A", "finito", "manuale")}
	confermaComponente(&th, cSciolto, "7120200A", cProdotto, 3)
	th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cProdotto, Codice: "7120100A", Esito: "mancante"}}
	th.Fabbisogni = fabbisogniDefault()
	conDocumento2D(&th, dPDF2, cSciolto, aPDF2, "disegno-acme.pdf", sha2, scansione(t, sha2))
	return th
}

// scenaAccanto: la BOM di lavoro della scena dell'albero (la radice #1 con il figlio #2 deciso come lo sciolto) e, accanto,
// due componenti confermati senza un nodo: uno aggiunto a mano sotto il finito (7120300A, quantità 4) e uno sotto lo
// sciolto (7120310A, quantità 2).
func scenaAccanto(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, true)
	th.Componenti = append(th.Componenti, componente(cAMano, "7120300A", "sciolto", "manuale"))
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cProdotto, FiglioID: cAMano, Qta: 4, Origine: "manuale", ConfermatoDa: operatore, CreatoIl: dataACME})
	confermaComponente(&th, cSottoSciol, "7120310A", cSciolto, 2)
	return th
}

// scenaDueProdotti: la scena dell'albero con i 2D, lo sciolto con la revisione registrata «2», e il finito manuale
// 7120900A, senza STEP, con lo sciolto nella sua BOM confermata (la relazione con la quantità 5): lo sciolto è un nodo
// della BOM di lavoro del primo prodotto e un confermato senza STEP del secondo.
func scenaDueProdotti(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, true)
	for i := range th.Componenti {
		if th.Componenti[i].ID == cSciolto {
			th.Componenti[i].Rev = testo("2")
		}
	}
	th.Componenti = append(th.Componenti, componente(cP2SenzaSTP, "7120900A", "finito", "manuale"))
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cP2SenzaSTP, FiglioID: cSciolto, Qta: 5, Origine: "manuale", ConfermatoDa: operatore, CreatoIl: dataACME})
	th.StepProdotto = append(th.StepProdotto, fotorfq.RigaStepProdotto{ComponenteID: cP2SenzaSTP, Codice: "7120900A", Esito: "mancante"})
	return th
}

// assiDelProdotto: l'impronta di ciò che i campi nuovi non devono cambiare (vincolo dell'utente dell'08/10: «l'esposizione
// di più dati non deve cambiare da sola stato, assi e autorità»): l'autorità e l'identità, la fonte, lo stato della
// struttura, la verifica della BOM, lo smistamento, lo stato e il motivo della completezza con il perimetro, il composto,
// lo stato, i motivi e l'impronta del prodotto (R90). Le voci della completezza restano fuori, perché EB7-4 aggiunge il
// documento e lo stato del NAS alla voce del 2D: le prova la loro prova.
func assiDelProdotto(t *testing.T, pv valutazione.ProdottoValutato) string {
	t.Helper()
	h, err := jsoncanonico.ImprontaDi(struct {
		Autorita        string                          `json:"autorita"`
		Identita        string                          `json:"identita"`
		Fonte           valutazione.FonteProdotto       `json:"fonte"`
		Struttura       string                          `json:"struttura"`
		BOM             valutazione.VerificaBOM         `json:"bom"`
		Smistamento     valutazione.VerificaSmistamento `json:"smistamento"`
		Documenti       string                          `json:"documenti"`
		MotivoDocumenti string                          `json:"motivo_documenti"`
		Perimetro       string                          `json:"perimetro"`
		Verificato      bool                            `json:"prodotto_verificato"`
		Stato           string                          `json:"stato"`
		Motivi          []valutazione.MotivoProdotto    `json:"motivi"`
		Impronta        string                          `json:"impronta"`
	}{string(pv.Autorita), string(pv.Identita), pv.Fonte, string(pv.Struttura), pv.BOM, pv.Smistamento, string(pv.Documenti.Stato),
		pv.Documenti.Motivo, statoDi(pv.Documenti), pv.Verificato, string(pv.Stato), pv.Motivi, pv.Impronta})
	if err != nil {
		t.Fatal(err)
	}
	return h
}
