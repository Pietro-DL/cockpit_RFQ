// L1 — gli aiuti delle prove di B5 (il collegamento con ancoraggio, la verifica della BOM, i conflitti di nomenclatura
// e di gerarchia): la famiglia ACME con il marcatore e la revisione, i fatti STEP con le formazioni, i PDF con il
// cartiglio, le righe legacy con il segno di «Conferma l'albero», costruiti in Go come li darebbe il caricatore.
package valutazione_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example); i codici sono di fantasia (712xxxx con il marcatore
// A o B e la revisione di una cifra: «7120100A_1» nel nome del file, «7120200A1» nel cartiglio, «7120100A» nei nodi
// STEP), gli UUID sono 00000000-0000-4000-8000-0000000000nn. La famiglia acme-catena attiva un meccanismo (un nodo che
// legge base e marcatore senza revisione, mentre la forma del cartiglio la chiede: R87), mai il significato di un
// cliente. Le prove citano i requisiti (R80, T-B0-24, K-02, PO-03, PO-15, PO-23, PO-37), mai i casi degli attesi.

// ---- la grammatica ACME della catena ----

func parteC(tipo string, letterali ...string) grammatica.Parte {
	return grammatica.Parte{Tipo: tipo, Letterali: letterali, Min: 1, Max: 1}
}

func formaCatena(id string, selettori []string, parti ...grammatica.Parte) grammatica.FormaCodice {
	return grammatica.FormaCodice{ID: id, Selettori: selettori, Stato: grammatica.StatoAttiva, Completa: true, Parti: parti}
}

// famigliaCatena: base 712 più quattro cifre, marcatore A o B (fa parte dell'identità: R86), revisione di una cifra.
//   - cartiglio [base][A][rev] su cartiglio.codice: la forma documentale, con la revisione obbligatoria (R63 B);
//   - mail [base][A][rev facoltativa] su oggetto e corpo;
//   - nome [base][A]_[rev] sul nome del file;
//   - step [base][A] sugli id delle radici e dei nodi: base e marcatore, nessuna revisione (il caso di R87).
func famigliaCatena() grammatica.FamigliaCodice {
	base := grammatica.Base{
		Segmenti:  []grammatica.SegmentoBase{{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}},
		Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
	pBase := parteC(grammatica.TipoParteBase)
	pMarc := parteC(grammatica.TipoParteMarcatore, "A", "B")
	pSotto := parteC(grammatica.TipoParteSeparatore, "_")
	rev := func(min int) grammatica.Parte {
		return grammatica.Parte{Tipo: grammatica.TipoParteRevisione, Rif: "rev-c", Min: min, Max: 1}
	}
	step := formaCatena("step", []string{"radice_step.id", "nodo_step.id"}, pBase, pMarc)
	parola := grammatica.ConfineParolaASCII
	step.ConfineDopo = &parola
	esempio := func(id, sel, testo string, l grammatica.LetturaAttesa) grammatica.EsempioCodice {
		return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: sel, Testo: testo,
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{l}}}
	}
	return grammatica.FamigliaCodice{
		ID: "acme-catena", Namespace: "acme-catena",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base,
		Forme: []grammatica.FormaCodice{
			formaCatena("cartiglio", []string{"cartiglio.codice"}, pBase, pMarc, rev(1)),
			formaCatena("mail", []string{"oggetto", "corpo"}, pBase, pMarc, rev(0)),
			formaCatena("nome", []string{"nome_file"}, pBase, pMarc, pSotto, rev(1)),
			step,
		},
		Revisioni: []grammatica.RegolaRevisione{{
			ID: "rev-c", Selettori: []string{"cartiglio.codice", "nome_file", "corpo", "oggetto"},
			Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]", Significato: grammatica.SignificatoNessuno}},
		}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-cartiglio", "cartiglio.codice", "7120100A2", grammatica.LetturaAttesa{Forma: "cartiglio", Base: "7120100", Marcatore: "A", Revisione: "2"}),
			esempio("e-mail", "corpo", "7120100A2", grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Marcatore: "A", Revisione: "2"}),
			esempio("e-nome", "nome_file", "7120100A_2.stp", grammatica.LetturaAttesa{Forma: "nome", Base: "7120100", Marcatore: "A", Revisione: "2"}),
			esempio("e-step", "nodo_step.id", "7120100A", grammatica.LetturaAttesa{Forma: "step", Base: "7120100", Marcatore: "A"}),
		},
	}
}

// motoreCatena: la grammatica della catena passa dalla porta del prodotto (file v1, NuovoSnapshot, CompilaVerificato).
func motoreCatena(t *testing.T) *motorea.Motore {
	t.Helper()
	return motoreConFamiglia(t, famigliaCatena())
}

// motoreConFamiglia: la grammatica ACME con quella famiglia sola, dalla porta del prodotto.
func motoreConFamiglia(t *testing.T, f grammatica.FamigliaCodice) *motorea.Motore {
	t.Helper()
	g := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       []grammatica.FamigliaCodice{f},
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	s, err := grammatica.NuovoSnapshot(raw, limitiACME())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	m, d, err := motorea.CompilaVerificato(s, limitiACME())
	if err != nil || m == nil {
		t.Fatalf("la grammatica non compila: %v %+v", err, d)
	}
	return m
}

// ---- i fatti ----

// nodoF: un PRODUCT dei fatti con la formazione grezza (rev_grezza, PRODUCT_DEFINITION_FORMATION.id).
type nodoF struct{ chiave, id, rev string }

// arcoF: una relazione dei fatti, con la quantità della coppia.
type arcoF struct {
	padre, figlio string
	qta           int
}

// fattiStepF: i fatti di uno STEP completo alla terna corrente (struttura v3), con la radice nel primo nodo.
func fattiStepF(t *testing.T, sha string, nodi []nodoF, archi []arcoF) fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": "", "descrizione_grezza": "",
			"rev_grezza": x.rev, "evidenza": map[string]any{}})
	}
	r := []map[string]any{}
	for _, a := range archi {
		r = append(r, map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": map[string]any{}})
	}
	raw, err := json.Marshal(map[string]any{"struttura": map[string]any{
		"versione": 3, "schema": "AP214", "radici": []string{nodi[0].chiave}, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fatti(t, sha, string(raw), testo(""))
}

// fattiPDF: i fatti di un PDF con il testo v2 del worker e il codice del cartiglio dato; senza codice è una scansione
// senza testo nativo e senza OCR.
func fattiPDF(t *testing.T, sha, codice string) fotorfq.Fatti {
	t.Helper()
	cs := []map[string]any{}
	estraibile, ocr := true, "non_necessario"
	if codice != "" {
		cs = append(cs, map[string]any{"etichetta": "codice", "letta": "CODICE", "valore": codice, "pagina": 1, "zona": "basso_destra",
			"fonte": "nativo", "riquadro": []float64{800, 700, 900, 711}, "confidenza": nil})
	} else {
		estraibile, ocr = false, "non_disponibile"
	}
	raw, err := json.Marshal(map[string]any{"testo_pdf": map[string]any{
		"versione": 2, "estraibile": estraibile, "pagine": 1, "pagine_lette": 1, "caratteri": 10, "troncato": false, "formato_pagina1": []float64{1191, 842},
		"frammenti": []map[string]any{}, "cartiglio": cs, "metadati": map[string]any{"titolo": "", "soggetto": "", "parole_chiave": "", "creatore": "", "produttore": ""},
		"ocr": map[string]any{"stato": ocr, "motivo": "", "motore": "", "tentativi": []string{}},
		"limiti": map[string]any{"pagine_max": 11, "frammenti_max": 200, "frammento_max": 512, "caratteri_max": 16384, "campi_max": 24,
			"ocr_soglia_pagina": 20, "ocr_soglia_cartiglio": 8, "ocr_pagine_max": 2, "ocr_tempo_max_s": 60, "ocr_dpi": 300},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fatti(t, sha, string(raw), nil)
}

// ---- la scena della BOM ----

// Gli ID della scena: il finito manuale 7120100A (target), lo sciolto 7120200A, lo STEP dell'assieme con la radice #1 e
// i figli, il suo documento, il 2D dello sciolto con il suo documento, le righe legacy.
var (
	cProdotto  = uid(0x701)
	cSciolto   = uid(0x702)
	cTerzo     = uid(0x703)
	aStepB     = uid(0x711)
	aDisegno   = uid(0x712)
	dStepB     = uid(0x721)
	dDisegno   = uid(0x722)
	rigaRadB   = uid(0x731)
	rigaB2     = uid(0x732)
	rigaB3     = uid(0x733)
	rigaB4     = uid(0x734)
	shaStepB   = sha("a")
	shaDisegno = sha("b")
	rifProdB   = "componente:" + uid(0x701).String()
	ilAlbero   = "2026-10-02T09:00:00Z"
)

// nodoB: il Rif del nodo dello STEP della scena.
func nodoB(chiave string) string { return ancoraggio.RifNodo(shaStepB, chiave) }

// scenaBOM: la RFQ ACME con il finito manuale 7120100A e il suo STEP strutturale autorizzato (documento,
// step_strutturale_id, riga della vista presente_analizzato, marcatura della radice: la fonte è confermata), con i
// nodi dati (la radice è il primo), gli archi dei fatti e una riga aperta per ogni nodo. Le prove aggiungono i figli
// confermati, il segno, le righe decise.
func scenaBOM(t *testing.T, nodi []nodoF, archi []arcoF) fotorfq.Thread {
	t.Helper()
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per 7120100A.\r\nGrazie")
	th.Componenti = []fotorfq.Componente{componente(cProdotto, "7120100A", "finito", "manuale")}
	f := fattiStepF(t, shaStepB, nodi, archi)
	conAllegato(&th, allegato(aStepB, 1, "7120100A_1.stp", "stp", shaStepB), &f)
	cp, d := cProdotto, dStepB
	th.Documenti = []fotorfq.DocumentoConfermato{{ID: dStepB, ThreadID: threadACME, ComponenteID: &cp, Tipo: "cad_3d", NomeFile: "7120100A_1.stp",
		Estensione: "stp", Sha256: shaStepB, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME, Allegati: []uuid.UUID{aStepB}}}
	th.Componenti[0].StepStrutturaleID = &d
	th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cProdotto, Codice: "7120100A", StepStrutturaleID: &d, NStepCorrenti: 1, AnalisiCompleta: true,
		Esito: "presente_analizzato"}}
	for i, n := range nodi {
		id := uid(0x731 + i)
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: id, AllegatoID: aStepB,
			NomeFile: "7120100A_1.stp", Sha256: shaStepB, Chiave: n.chiave, IDGrezzo: n.id, Famiglia: "acme-catena", Fonte: "step", Stato: "aperta"})
	}
	for _, a := range archi {
		th.RigheRelazioneProposta = append(th.RigheRelazioneProposta, fotorfq.RigaRelazioneProposta{AllegatoID: aStepB, NomeFile: "7120100A_1.stp",
			PadreChiave: a.padre, FiglioChiave: a.figlio, Qta: a.qta, Stato: "aperta"})
	}
	op, cpp, dd := operatore, cProdotto, dStepB
	th.RigheComponenteProposta[0].Marcatura = &fotorfq.MarcaturaStrutturale{Versione: 1, Ruolo: "radice", ComponenteID: &cpp, DocumentoID: &dd,
		DichiaratoDa: &op, DichiaratoIl: "2026-10-01T08:00:00Z"}
	return th
}

// senzaFonte: la stessa scena senza il gesto 3 (nessuno step_strutturale_id, nessuna marcatura, la vista da_scegliere):
// la fonte non è confermata, e le strutture restano candidate.
func senzaFonte(th *fotorfq.Thread) {
	th.Componenti[0].StepStrutturaleID = nil
	th.StepProdotto[0].StepStrutturaleID, th.StepProdotto[0].Esito = nil, "da_scegliere"
	th.RigheComponenteProposta[0].Marcatura = nil
}

// rigaDi: il puntatore alla riga di componente_proposta del nodo con quella chiave.
func rigaDi(t *testing.T, th *fotorfq.Thread, chiave string) *fotorfq.RigaComponenteProposta {
	t.Helper()
	for i := range th.RigheComponenteProposta {
		if th.RigheComponenteProposta[i].Chiave == chiave {
			return &th.RigheComponenteProposta[i]
		}
	}
	t.Fatalf("nessuna riga per il nodo %s", chiave)
	return nil
}

// arcoDi: il puntatore alla riga di relazione_proposta dell'arco (padre, figlio).
func arcoDi(t *testing.T, th *fotorfq.Thread, padre, figlio string) *fotorfq.RigaRelazioneProposta {
	t.Helper()
	for i := range th.RigheRelazioneProposta {
		if r := &th.RigheRelazioneProposta[i]; r.PadreChiave == padre && r.FiglioChiave == figlio {
			return r
		}
	}
	t.Fatalf("nessuna riga per l'arco %s→%s", padre, figlio)
	return nil
}

// decidi: la riga del nodo decisa come il componente dato (confermata o duplicato), da una persona se da non è nil.
func decidi(t *testing.T, th *fotorfq.Thread, chiave, stato string, comp uuid.UUID, da *uuid.UUID) {
	t.Helper()
	r := rigaDi(t, th, chiave)
	c := comp
	il := dataACME.Add(2 * time.Hour)
	r.Stato, r.ComponenteID, r.DecisoDa, r.DecisoIl = stato, &c, da, &il
}

// confermaComponente: un componente confermato (sciolto, origine step) con la relazione dal padre e la quantità date.
func confermaComponente(th *fotorfq.Thread, id uuid.UUID, codice string, padre uuid.UUID, qta int) {
	th.Componenti = append(th.Componenti, componente(id, codice, "sciolto", "step"))
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: padre, FiglioID: id, Qta: qta, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
}

// segnoAlbero: il segno di «Conferma l'albero» (evidenza.albero): sulla riga di un nodo deciso dalla conferma, e sulla
// riga di un arco tenuto, con i due componenti del legame.
func segnoAlbero(padre, figlio *uuid.UUID) *fotorfq.SegnoAlbero {
	return &fotorfq.SegnoAlbero{Da: operatore, Il: ilAlbero, Firma: strings.Repeat("f", 64), Padre: padre, Figlio: figlio}
}

// confermaLAlbero: «Conferma l'albero» come la scrive il legacy sul figlio di un arco della radice: il componente e la
// relazione confermati, la riga del nodo decisa da una persona con il segno (confermata), la riga dell'arco tenuta con il
// segno e i due componenti. La riga della radice resta com'è (aperta: la radice del prodotto non è una riga decisa).
func confermaLAlbero(t *testing.T, th *fotorfq.Thread, chiave string, comp uuid.UUID, codice string, qta int) {
	t.Helper()
	confermaComponente(th, comp, codice, cProdotto, qta)
	op := operatore
	decidi(t, th, chiave, "confermata", comp, &op)
	rigaDi(t, th, chiave).Albero = segnoAlbero(nil, nil)
	a := arcoDi(t, th, "#1", chiave)
	p, f, il := cProdotto, comp, dataACME.Add(2*time.Hour)
	a.Stato, a.DecisoDa, a.DecisoIl, a.Albero = "confermata", &op, &il, segnoAlbero(&p, &f)
}

// conDisegno: il 2D di un componente: l'allegato PDF con il cartiglio dato e il documento disegno_2d confermato sul
// componente.
func conDisegno(t *testing.T, th *fotorfq.Thread, comp uuid.UUID, cartiglio string, rev *string) {
	t.Helper()
	f := fattiPDF(t, shaDisegno, cartiglio)
	conAllegato(th, allegato(aDisegno, 2, "disegno-acme.pdf", "pdf", shaDisegno), &f)
	c := comp
	th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dDisegno, ThreadID: threadACME, ComponenteID: &c, Tipo: "disegno_2d",
		NomeFile: "disegno-acme.pdf", Estensione: "pdf", Sha256: shaDisegno, Rev: rev, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME,
		Allegati: []uuid.UUID{aDisegno}})
}

// ---- le letture dell'esito ----

func strutturaDelProdotto(t *testing.T, v valutazione.ValutazioneProdotti, rif string, stato ancoraggio.StatoStruttura) ancoraggio.StrutturaProdotto {
	t.Helper()
	for _, s := range v.Ancoraggi.Strutture {
		if s.Target == rif && s.Stato == stato {
			return s
		}
	}
	t.Fatalf("nessuna struttura %s per %s fra %d", stato, rif, len(v.Ancoraggi.Strutture))
	return ancoraggio.StrutturaProdotto{}
}

func nodoIn(t *testing.T, s ancoraggio.StrutturaProdotto, rif string) ancoraggio.NodoProposto {
	t.Helper()
	for _, n := range s.Nodi {
		if n.Rif == rif {
			return n
		}
	}
	t.Fatalf("il nodo %s non c'è nella struttura %s", rif, s.Radice)
	return ancoraggio.NodoProposto{}
}

func ancoraggioDel(t *testing.T, v valutazione.ValutazioneProdotti, allegato uuid.UUID) ancoraggio.AncoraggioFile {
	t.Helper()
	for _, a := range v.Ancoraggi.File {
		if a.AllegatoID == allegato {
			return a
		}
	}
	t.Fatalf("nessun ancoraggio per l'allegato %s", allegato)
	return ancoraggio.AncoraggioFile{}
}

func canonicoDi(t *testing.T, v any) string {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// asse: lo stato e il motivo di un asse, per i confronti.
func asse(a valutazione.VerificaAsse) string { return string(a.Stato) + "/" + a.Motivo }
