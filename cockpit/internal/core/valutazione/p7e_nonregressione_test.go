// L1 — la non regressione di P7e (EB7-1 B, EB7-2 A, EB7-4 A, ratificati dall'utente l'08/10; precisazioni D1–D6 del
// 10/10): su un catalogo di scene ACME di valutazione (i costruttori scena*, le fotografie con più thread e i messaggi
// fuori RFQ di scena_calcola_test.go e valutazione_test.go, con quattro insiemi di regole, con la vista calcolata e
// senza), l'esito di Calcola, tolti i soli campi che P7e aggiunge, è uguale a quello del codice di prima di P7e. Il
// «prima» è il file testdata/p7e_prima_acme.json, generato con questo stesso file sul commit di prima (b6a2f3b, dove
// valutazione è identico a 1cc6409), via -overlay: niente è scritto a memoria. Il comando è in testa al file.
//
// La normalizzazione è al livello del JSON, così il file si compila anche sul codice di prima:
//   - la versione e l'impronta dell'esito (Esito.VersioneValutazione, Esito.Impronta): cambiano per costruzione, con i
//     campi nuovi (valutazione-2);
//   - in ogni prodotto valutato strutture_candidate e confermati_senza_step (EB7-1 B);
//   - in ogni nodo di PV.Nodi tipo e origine_tipo (EB7-2 A);
//   - nella voce del 2D documento_id e stato_nas (EB7-4 A): prima di P7e la voce del 2D non li aveva mai;
//   - nei candidati dei record piatti (confrontabili[].nuovo.candidati[]) chiavi_f8: il gemello di confronto-4 (notte
//     14/10, WP-C), un campo in più che dichiara le righe legacy degli stessi nodi, senza cambiare nient'altro.
//
// Tutto il resto deve restare uguale: stato, assi, motivi, autorità, l'impronta del prodotto (R90), i nodi senza il tipo,
// i previsti e le altre voci della completezza, gli ancoraggi, lo smistamento, il fascicolo. Una differenza è una
// regressione da riportare, non da «aggiornare»: il file si rigenera solo con un commit che dichiara un cambiamento di
// regola (e allora sul commit di prima di quel cambiamento).
package valutazione_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_test.go.

// fileP7ePrima: il «prima» della non regressione, in testdata.
const fileP7ePrima = "p7e_prima_acme.json"

// primaP7e: il file del «prima». Scene: per ogni voce del catalogo, l'impronta (sha256 del JSON normalizzato) di ogni
// parte dell'esito (partiP7e). Le mappe si scrivono con le chiavi in ordine.
type primaP7e struct {
	Commento []string                     `json:"commento"`
	Commit   string                       `json:"commit"`
	Comando  []string                     `json:"comando"`
	Scene    map[string]map[string]string `json:"scene"`
}

// voceP7e: una voce del catalogo: la fotografia, l'insieme delle regole e gli ingressi di Calcola.
type voceP7e struct {
	nome     string
	calcolaP func(t *testing.T) (fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi)
}

// insiemeP7e: un insieme delle regole del catalogo, con il suo nome.
type insiemeP7e struct {
	nome    string
	insieme func(t *testing.T) *motorea.InsiemeRegole
}

// insiemeFamigliaACME: le regole ACME con la famiglia di scena_test.go (quella del motore motoreACME), per le scene
// costruite su threadBase.
func insiemeFamigliaACME(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	return insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", famigliaACME())})
}

// catalogoP7e: le voci della non regressione. Ogni scena con l'insieme ACME della catena, senza la vista calcolata e con
// la vista calcolata come la calcola v_fascicolo (vistaCome); con la vista anche con gli insiemi della minuteria, della
// storia e della famiglia di scena_test.go. Poi le fotografie con più thread (i quattro clienti, senza insieme e con, i
// messaggi fuori RFQ con i casi).
func catalogoP7e() []voceP7e {
	scene := []struct {
		nome string
		th   func(t *testing.T) fotorfq.Thread
	}{
		{"collegamento", scenaCollegamento},
		{"contesto", scenaContesto},
		{"albero", scenaAlbero},
		{"con-la-copia", scenaConLaCopia},
		{"completezza-senza-2d", func(t *testing.T) fotorfq.Thread { return scenaCompletezza(t, false) }},
		{"completezza-con-2d", func(t *testing.T) fotorfq.Thread { return scenaCompletezza(t, true) }},
		{"tre-nodi-senza-2d", func(t *testing.T) fotorfq.Thread { return scenaTreNodi(t, false) }},
		{"tre-nodi-con-2d", func(t *testing.T) fotorfq.Thread { return scenaTreNodi(t, true) }},
		{"due-disegni-cartiglio-rev-1", func(t *testing.T) fotorfq.Thread { return scenaDueDisegni(t, "2", "7120200A1", "7120200A_2.tif") }},
		{"due-disegni-cartiglio-rev-2", func(t *testing.T) fotorfq.Thread { return scenaDueDisegni(t, "2", "7120200A2", "7120200A_2.tif") }},
		{"finito-mancante", func(t *testing.T) fotorfq.Thread { return scenaFinito(t, "mancante") }},
		{"finito-presente-analizzato", func(t *testing.T) fotorfq.Thread { return scenaFinito(t, "presente_analizzato") }},
		{"finito-presente-parziale", func(t *testing.T) fotorfq.Thread { return scenaFinito(t, "presente_parziale") }},
		{"senza-componente", func(t *testing.T) fotorfq.Thread { return scenaSenzaComponente() }},
		{"revisioni", scenaRevisioni},
		{"radice-illeggibile", func(t *testing.T) fotorfq.Thread { th, _ := scenaRadiceIllegibile(t); return th }},
		{"fuori-perimetro-finito", func(t *testing.T) fotorfq.Thread { return scenaFuoriPerimetro(t, "7120100A_1.pdf") }},
		{"fuori-perimetro-sciolto", func(t *testing.T) fotorfq.Thread { return scenaFuoriPerimetro(t, "7120200A_1.pdf") }},
		{"smistamento", scenaSmistamento},
		{"vecchio-codice-corto", func(t *testing.T) fotorfq.Thread { return scenaVecchio(t, "7120200A", `{}`) }},
		{"vecchio-codice-01", func(t *testing.T) fotorfq.Thread { return scenaVecchio(t, "7120200A01", `{}`) }},
		{"vecchio-con-valutazione", func(t *testing.T) fotorfq.Thread {
			return scenaVecchio(t, "7120200A1", `{"valutazione": {"v": 2, "codice": {"valore": "7120299A1", "regola": "cartiglio"}}}`)
		}},
		{"candidata", scenaCandidata},
		{"due-candidate", func(t *testing.T) fotorfq.Thread { th := scenaCandidata(t); conSecondoSTEP(t, &th); return th }},
		{"albero-e-secondo-step", func(t *testing.T) fotorfq.Thread { th := scenaCompletezza(t, true); conSecondoSTEP(t, &th); return th }},
		{"senza-step", scenaSenzaSTEP},
		{"accanto", scenaAccanto},
		{"due-prodotti", scenaDueProdotti},
		// le varianti scritte dentro le prove di EB7 e dei previsti (emendamenti_test.go, completezza_scena_test.go)
		{"tre-nodi-riga-4-sottoassieme", scenaTreNodiRiga4Sottoassieme},
		{"tre-nodi-senza-riga-3", scenaTreNodiSenzaRiga3},
		{"tre-nodi-arco-3-4-tolto", scenaTreNodiArcoTolto},
		{"tre-nodi-riga-3-sciolto-senza-riga-4", scenaTreNodiRiga3Sciolto},
		{"completezza-commerciale", scenaCompletezzaCommerciale},
		{"completezza-dwg-e-assegna", scenaDWGEAssegna},
		{"completezza-dwg-e-deroga", scenaDWGEDeroga},
		{"completezza-pdf-e-dwg-piu-recente", scenaPDFEDWGPiuRecente},
		// le scene di D3 (precisazione dell'utente del 10/10): le loro differenze sono autorizzate (autorizzateP7e)
		{"d3-riga-scartata-non-da-persona", scenaD3RigaScartata},
		{"d3-riga-fuori-enum", scenaD3FuoriEnum},
		{"d3-riga-confermata-componente-archiviato", scenaD3RigaDecisaArchiviato},
		// la divergenza dichiarata da D1 (T-P7e-05), con i fabbisogni che distinguono sottoassieme e sciolto: deve restare
		{"d1-arco-tolto-con-i-fabbisogni-del-sottoassieme", func(t *testing.T) fotorfq.Thread {
			th := scenaTreNodiArcoTolto(t)
			th.Fabbisogni = fabbisogniD3()
			return th
		}},
	}
	for _, corpo := range []string{
		"Buongiorno,\r\nRichiesta di offerta.",
		"Buongiorno,\r\nper 7120777A vi mandiamo i file.\r\nGrazie",
		"Buongiorno,\r\nrichiesta per P7120100.",
		"Buongiorno,\r\nvi chiediamo l'offerta per 7120100A.\r\nGrazie",
		"Buongiorno,\r\nvi chiediamo l'offerta per 7120100A2 e 7120200A1.\r\nGrazie",
		"Buongiorno,\r\nvi chiediamo l'offerta per 7120100A2.\r\nGrazie",
		"Buongiorno,\r\nvi chiediamo l'offerta per 7120100B.\r\nGrazie",
		"Buongiorno,\r\nvi chiediamo l'offerta per P7120100 e P7120200.",
		"Buongiorno,\r\nvi chiediamo l'offerta per P7120100, P7120200 e P7120300.\r\nGrazie",
		"Buongiorno,\r\nvi chiediamo l'offerta per P7120100.\r\nGrazie",
		"Richiesta.",
		mailInoltrata(),
	} {
		c := corpo
		scene = append(scene, struct {
			nome string
			th   func(t *testing.T) fotorfq.Thread
		}{"base:" + c, func(t *testing.T) fotorfq.Thread { return threadBase(c) }})
	}
	insiemi := []insiemeP7e{{"catena", insiemeACME}, {"minuteria", insiemeConMinuteria}, {"storia", insiemeConStoria}, {"famiglia-acme", insiemeFamigliaACME}}
	var out []voceP7e
	for _, s := range scene {
		s := s
		for i, r := range insiemi {
			r := r
			viste := []bool{true}
			if i == 0 {
				viste = []bool{false, true}
			}
			for _, vista := range viste {
				vista := vista
				nome := s.nome + " | " + r.nome
				if vista {
					nome += " | vista"
				}
				out = append(out, voceP7e{nome: nome, calcolaP: func(t *testing.T) (fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) {
					th := s.th(t)
					if vista {
						vistaCome(&th)
					}
					return fotografiaDi(th), r.insieme(t), valutazione.Ingressi{Versione: valutazione.VersioneCasi}
				}})
			}
		}
	}
	out = append(out,
		voceP7e{"quattro-thread | catena", func(t *testing.T) (fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) {
			return fotografiaDi(quattroThread(t)...), insiemeACME(t), valutazione.Ingressi{Versione: valutazione.VersioneCasi}
		}},
		voceP7e{"quattro-thread | senza insieme", func(t *testing.T) (fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) {
			return fotografiaDi(quattroThread(t)...), nil, valutazione.Ingressi{}
		}},
		voceP7e{"quattro-thread con fuori rfq e casi | catena", func(t *testing.T) (fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) {
			f := fotografiaDi(quattroThread(t)...)
			mUno := uid(0x6f1)
			f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{messaggioFuori(mUno, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil)}
			return f, insiemeACME(t), valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{
				{ID: "censimento-acme", ClienteID: clienteACME, Messaggi: []uuid.UUID{mUno}},
				{ID: "caso-tre", ThreadID: &threadSenzaRegole, ClienteID: clienteSenzaRegole},
			}}
		}},
	)
	return out
}

// ---- le varianti del catalogo (si compilano anche con il codice di prima di P7e) ----

// scenaTreNodiRiga4Sottoassieme: la scena dei tre nodi con la riga aperta di #4 (senza figli) che propone sottoassieme
// (TestEB72IlTipoDelNodo).
func scenaTreNodiRiga4Sottoassieme(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	rigaDi(t, &th, "#4").TipoProposto = testo("sottoassieme")
	return th
}

// scenaTreNodiSenzaRiga3: la scena dei tre nodi senza la riga di #3 (TestEB72IlTipoDelNodo).
func scenaTreNodiSenzaRiga3(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	var righe []fotorfq.RigaComponenteProposta
	for _, r := range th.RigheComponenteProposta {
		if r.Chiave != "#3" {
			righe = append(righe, r)
		}
	}
	th.RigheComponenteProposta = righe
	return th
}

// scenaTreNodiArcoTolto: la scena dei tre nodi con l'arco #3 → #4 tolto da una persona (TestEB72IlTipoDelNodo; dubbio
// T-P7e-05: #3 nei nodi non ha più figli, nei previsti sì).
func scenaTreNodiArcoTolto(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	op := operatore
	a := arcoDi(t, &th, "#3", "#4")
	a.Stato, a.DecisoDa = "scartata", &op
	return th
}

// scenaTreNodiRiga3Sciolto: la scena di TestIPrevistiSullaFotografia, «i nodi senza decisione»: la riga di #3 propone
// sciolto, #4 senza riga, il PDF candidato di #3, le regole del cliente sullo sciolto.
func scenaTreNodiRiga3Sciolto(t *testing.T) fotorfq.Thread {
	t.Helper()
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
	return th
}

// scenaCompletezzaCommerciale: la scena dell'albero con lo sciolto commerciale (TestEB72IlCommerciale).
func scenaCompletezzaCommerciale(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, true)
	th.Componenti[1].Tipo = "commerciale"
	return th
}

// scenaDWGEAssegna: lo sciolto con un DWG confermato e corrente e un PDF con «assegna» (la voce del 2D da verificare per
// l'associazione; TestEB74LaVoceDecisaDallAssociazioneConUnDWG).
func scenaDWGEAssegna(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, false)
	dwg := documento2D(dDWG, cSciolto, aDWG, "disegno-acme.dwg", "dwg", shaDWG)
	conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, &dwg, uuid.Nil)
	conFile2D(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), scansione(t, sha2), nil, cSciolto)
	return th
}

// scenaDWGEDeroga: lo sciolto con un DWG confermato e corrente e una deroga sul 2D (la voce manca, derogata: la deroga
// non sostituisce il 2D, R62 D.4).
func scenaDWGEDeroga(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, false)
	dwg := documento2D(dDWG, cSciolto, aDWG, "disegno-acme.dwg", "dwg", shaDWG)
	conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, &dwg, uuid.Nil)
	th.Deroghe = []fotorfq.DerogaFabbisogno{{ID: derogaA, ComponenteID: cSciolto, Tipo: "disegno_2d", Motivo: "lo facciamo noi", UtenteID: operatore, CreataIl: dataACME}}
	return th
}

// scenaPDFEDWGPiuRecente: il PDF valido dello sciolto confermato prima e un DWG confermato dopo, in coda sul NAS
// (TestEB74LoStatoDelNASDelDisegno).
func scenaPDFEDWGPiuRecente(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, true)
	dwg := documento2D(dDWG, cSciolto, aDWG, "disegno-acme.dwg", "dwg", shaDWG)
	dwg.ConfermatoIl, dwg.StatoNas = dataACME.Add(4*time.Hour), "in_coda"
	conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, &dwg, uuid.Nil)
	return th
}

// ---- le scene di D3 (precisazione dell'utente del 10/10, «Precedenza delle fonti del tipo») ----

// cArchiviatoD3: il componente archiviato che una riga confermata porta (scenaD3RigaDecisaArchiviato).
var cArchiviatoD3 = uid(0xe7d1)

// fabbisogniD3: i fabbisogni con le righe del cliente sul sottoassieme (il cad_3d bloccante accanto al 2D): un
// sottoassieme e uno sciolto chiedono così cose diverse, e un tipo proposto diverso si vede nei previsti.
func fabbisogniD3() []fotorfq.RigaFabbisogno {
	return conRegole(regolaCliente("sottoassieme", "cad_3d", true), regolaCliente("sottoassieme", "disegno_2d", true))
}

// scenaD3RigaScartata: la riga di #4 (senza figli) scartata senza una persona, con tipo_proposto sottoassieme: il nodo non
// è uscito dalla BOM (nessuno l'ha deciso), quindi è previsto. Prima di D3 la riga scartata dava il tipo; con D3 no, e #4 è
// uno sciolto della regola.
func scenaD3RigaScartata(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	th.Fabbisogni = fabbisogniD3()
	r := rigaDi(t, &th, "#4")
	r.Stato, r.DecisoDa, r.TipoProposto = "scartata", nil, testo("sottoassieme")
	return th
}

// scenaD3FuoriEnum: la riga aperta di #3 (con il figlio #4) propone un valore fuori dall'enum tipo_componente. Prima di D3
// il valore era il tipo; con D3 non è una proposta, e #3 è un sottoassieme della regola.
func scenaD3FuoriEnum(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	th.Fabbisogni = fabbisogniD3()
	rigaDi(t, &th, "#3").TipoProposto = testo("assieme")
	return th
}

// scenaD3RigaDecisaArchiviato: la riga di #4 confermata senza una persona (un aggancio automatico) con un componente
// archiviato e tipo_proposto sottoassieme: il contesto non ha il componente, quindi il nodo non ha una decisione e resta
// previsto. Prima di D3 la riga decisa dava il tipo come se fosse aperta; con D3 vale solo attraverso la decisione, che
// qui non arriva al nodo, e #4 è uno sciolto della regola.
func scenaD3RigaDecisaArchiviato(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaTreNodi(t, true)
	th.Fabbisogni = fabbisogniD3()
	k := componente(cArchiviatoD3, "7120310A", "sciolto", "manuale")
	il := dataACME
	k.ArchiviatoIl = &il
	th.Componenti = append(th.Componenti, k)
	c := cArchiviatoD3
	r := rigaDi(t, &th, "#4")
	r.Stato, r.ComponenteID, r.DecisoDa, r.TipoProposto = "confermata", &c, nil, testo("sottoassieme")
	return th
}

// autorizzateP7e: le differenze autorizzate dall'utente (D3 del 10/10: «una proposta scartata non ha autorità», «una
// proposta già decisa va interpretata attraverso la decisione», «i valori proposti devono appartenere all'enum»). Il tipo
// proposto dei previsti della completezza è la stessa funzione del tipo dei nodi (bomDelThread.tipoProposto), quindi sulle
// scene di D3 cambiano i previsti del prodotto, e solo loro: la parte «previsti» del prodotto (Documenti.Previsti, a parte
// dal resto della completezza). Lo stato, il motivo, il perimetro, le voci e i non bloccanti della completezza (la parte
// «documenti») non dipendono dai previsti (StatoDellaCompletezza), e devono restare uguali come le altre parti. Ogni scena
// qui deve cambiare almeno in una voce del catalogo, altrimenti l'autorizzazione non serve più.
var autorizzateP7e = map[string]bool{
	"d3-riga-scartata-non-da-persona":          true,
	"d3-riga-fuori-enum":                       true,
	"d3-riga-confermata-componente-archiviato": true,
}

// parteAutorizzataP7e: la differenza della voce nella parte è autorizzata da D3 (autorizzateP7e): i soli previsti del
// prodotto della scena ACME, su una scena di D3. Restituisce la scena.
func parteAutorizzataP7e(voce, parte string) (string, bool) {
	scena, _, _ := strings.Cut(voce, " | ")
	return scena, autorizzateP7e[scena] && parte == "thread "+threadACME.String()+" | prodotto "+rifProdB+" | previsti"
}

// normalizzaP7e: toglie dal JSON dell'esito i soli campi che P7e aggiunge o cambia per costruzione (in testa al file).
// Ogni oggetto con prodotti_valutati, a qualunque profondità, ha i suoi prodotti normalizzati; ogni oggetto con
// confrontabili i suoi candidati senza chiavi_f8 (confronto-4).
func normalizzaP7e(v any) {
	switch x := v.(type) {
	case map[string]any:
		if pv, ok := x["prodotti_valutati"].([]any); ok {
			for _, p := range pv {
				if m, ok := p.(map[string]any); ok {
					normalizzaProdottoP7e(m)
				}
			}
		}
		if cf, ok := x["confrontabili"].([]any); ok {
			for _, f := range cf {
				if m, ok := f.(map[string]any); ok {
					normalizzaConfrontabileP7e(m)
				}
			}
		}
		for _, c := range x {
			normalizzaP7e(c)
		}
	case []any:
		for _, c := range x {
			normalizzaP7e(c)
		}
	}
}

// normalizzaProdottoP7e: un prodotto valutato senza i campi di EB7-1 B, il tipo dei nodi (EB7-2 A) e il documento con lo
// stato del NAS della voce del 2D (EB7-4 A).
func normalizzaProdottoP7e(p map[string]any) {
	delete(p, "strutture_candidate")
	delete(p, "confermati_senza_step")
	if nodi, ok := p["nodi"].([]any); ok {
		for _, n := range nodi {
			if m, ok := n.(map[string]any); ok {
				delete(m, "tipo")
				delete(m, "origine_tipo")
			}
		}
	}
	if d, ok := p["documenti"].(map[string]any); ok {
		if voci, ok := d["voci"].([]any); ok {
			for _, v := range voci {
				if m, ok := v.(map[string]any); ok && m["tipo_documento"] == "disegno_2d" {
					delete(m, "documento_id")
					delete(m, "stato_nas")
				}
			}
		}
	}
}

// normalizzaConfrontabileP7e: un record piatto con i candidati del nuovo senza chiavi_f8 (il gemello di confronto-4,
// notte 14/10, WP-C).
func normalizzaConfrontabileP7e(f map[string]any) {
	n, ok := f["nuovo"].(map[string]any)
	if !ok {
		return
	}
	if cc, ok := n["candidati"].([]any); ok {
		for _, c := range cc {
			if m, ok := c.(map[string]any); ok {
				delete(m, "chiavi_f8")
			}
		}
	}
}

// jsonP7e: il JSON di un valore (le chiavi delle mappe in ordine, i numeri com'erano).
func jsonP7e(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// improntaP7e: lo sha256 del JSON di un valore.
func improntaP7e(t *testing.T, v any) string {
	t.Helper()
	h := sha256.Sum256(jsonP7e(t, v))
	return hex.EncodeToString(h[:])
}

// esitoNormalizzatoP7e: l'esito come JSON generico (i numeri come json.Number), normalizzato.
func esitoNormalizzatoP7e(t *testing.T, e valutazione.Esito) map[string]any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(jsonP7e(t, e)))
	d.UseNumber()
	var root map[string]any
	if err := d.Decode(&root); err != nil {
		t.Fatal(err)
	}
	delete(root, "versione_valutazione")
	delete(root, "impronta")
	normalizzaP7e(root)
	return root
}

// partiP7e: le impronte delle parti dell'esito normalizzato, per dire dove cade una differenza: l'esito senza i thread
// («esito»), ogni thread senza i prodotti («thread <id>»), e di ogni prodotto la completezza senza i previsti, i previsti,
// i nodi e il resto («thread <id> | prodotto <rif> | documenti», «… | previsti», «… | nodi», «… | resto»). L'esito
// normalizzato non cambia: le parti si fanno su copie.
func partiP7e(t *testing.T, root map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	metti := func(k string, v any) {
		if _, ok := out[k]; ok {
			t.Fatalf("la parte %q è doppia", k)
		}
		out[k] = improntaP7e(t, v)
	}
	thread, _ := root["thread"].([]any)
	senza := map[string]any{}
	for k, v := range root {
		if k != "thread" {
			senza[k] = v
		}
	}
	metti("esito", senza)
	for _, x := range thread {
		th, _ := x.(map[string]any)
		id, _ := th["thread_id"].(string)
		pv, _ := th["prodotti_valutati"].([]any)
		resto := map[string]any{}
		for k, v := range th {
			if k != "prodotti_valutati" {
				resto[k] = v
			}
		}
		metti("thread "+id, resto)
		for _, y := range pv {
			p, _ := y.(map[string]any)
			rif, _ := p["rif"].(string)
			k := "thread " + id + " | prodotto " + rif
			altro := map[string]any{}
			for c, v := range p {
				if c != "documenti" && c != "nodi" {
					altro[c] = v
				}
			}
			var previsti any
			documenti := p["documenti"]
			if d, ok := documenti.(map[string]any); ok {
				senzaPrevisti := map[string]any{}
				for c, v := range d {
					if c != "previsti" {
						senzaPrevisti[c] = v
					}
				}
				documenti, previsti = senzaPrevisti, d["previsti"]
			}
			metti(k+" | documenti", documenti)
			metti(k+" | previsti", previsti)
			metti(k+" | nodi", p["nodi"])
			metti(k+" | resto", altro)
		}
	}
	return out
}

// nomeFileP7e: un nome di file dal nome della voce, per P7E_DUMP.
func nomeFileP7e(nome string) string {
	var b strings.Builder
	for _, r := range nome {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	h := sha256.Sum256([]byte(nome))
	s := b.String()
	if len(s) > 60 {
		s = s[:60]
	}
	return s + "_" + hex.EncodeToString(h[:4]) + ".json"
}

// TestP7eNonRegressione (P7e; compito punto «non regressione dell'asse»; direttiva del 10/10, Fase 3: «la prova di non
// regressione deve coprire tutte le fixture previste dal compito, non soltanto un campione»): ogni voce del catalogo dà le
// stesse parti del «prima». Con P7E_AGGIORNA=1 riscrive il «prima» (solo sul commit di prima, vedi in testa); con
// P7E_DUMP=<cartella> scrive l'esito normalizzato di ogni voce, per un diff fra due alberi.
func TestP7eNonRegressione(t *testing.T) {
	catalogo := catalogoP7e()
	calcolate := map[string]map[string]string{}
	dump := os.Getenv("P7E_DUMP")
	for _, v := range catalogo {
		if _, ok := calcolate[v.nome]; ok {
			t.Fatalf("la voce %q è doppia", v.nome)
		}
		f, r, in := v.calcolaP(t)
		root := esitoNormalizzatoP7e(t, calcola(t, f, r, in))
		calcolate[v.nome] = partiP7e(t, root)
		if dump != "" {
			b, err := json.MarshalIndent(root, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dump, nomeFileP7e(v.nome)), append(b, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	percorso := filepath.Join("testdata", fileP7ePrima)
	if os.Getenv("P7E_AGGIORNA") == "1" {
		p := primaP7e{
			Commento: []string{
				"Il «prima» della non regressione di P7e (p7e_nonregressione_test.go): per ogni voce del catalogo, lo sha256 del JSON",
				"normalizzato di ogni parte dell'esito di Calcola. Generato con il codice di prima di P7e, sul commit qui sotto",
				"(valutazione identico a 1cc6409), con il file di prova in overlay. Solo scene ACME inventate.",
			},
			Commit: os.Getenv("P7E_COMMIT"),
			Comando: []string{
				"git worktree add --detach <scratch>/pr b6a2f3b",
				"overlay.json: Replace di <pr>/cockpit/internal/core/valutazione/{scena_emendamenti_test.go,p7e_nonregressione_test.go} con le copie di P7e",
				"cd <pr>/cockpit && P7E_AGGIORNA=1 P7E_COMMIT=b6a2f3b GOFLAGS=-mod=readonly GOPROXY=off go test -overlay <overlay.json> ./internal/core/valutazione/ -run TestP7eNonRegressione -count=1",
			},
			Scene: calcolate,
		}
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(p); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(percorso, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("riscritto %s: %d voci", percorso, len(calcolate))
		return
	}
	raw, err := os.ReadFile(percorso)
	if err != nil {
		t.Fatal(err)
	}
	var prima primaP7e
	if err := json.Unmarshal(raw, &prima); err != nil {
		t.Fatal(err)
	}
	if prima.Commit == "" || len(prima.Comando) == 0 {
		t.Errorf("il «prima» non dice il commit e il comando con cui è stato generato")
	}
	var nomi []string
	for k := range prima.Scene {
		nomi = append(nomi, k)
	}
	for k := range calcolate {
		if _, ok := prima.Scene[k]; !ok {
			nomi = append(nomi, k)
		}
	}
	sort.Strings(nomi)
	parti, diverse, autorizzate := 0, 0, 0
	viste := map[string]bool{}
	for _, nome := range nomi {
		p, ok := prima.Scene[nome]
		d, ok2 := calcolate[nome]
		if !ok || !ok2 {
			t.Errorf("la voce %q c'è solo da una parte (prima %v, dopo %v)", nome, ok, ok2)
			diverse++
			continue
		}
		var chiavi []string
		for k := range p {
			chiavi = append(chiavi, k)
		}
		for k := range d {
			if _, ok := p[k]; !ok {
				chiavi = append(chiavi, k)
			}
		}
		sort.Strings(chiavi)
		for _, k := range chiavi {
			parti++
			if p[k] == d[k] {
				continue
			}
			if scena, ok := parteAutorizzataP7e(nome, k); ok {
				autorizzate++
				viste[scena] = true
				t.Logf("%s: la parte %q è cambiata, autorizzata da D3", nome, k)
				continue
			}
			diverse++
			t.Errorf("%s: la parte %q è cambiata (prima %q, dopo %q)", nome, k, p[k], d[k])
		}
	}
	for scena := range autorizzateP7e {
		if !viste[scena] {
			t.Errorf("la scena %q è fra le differenze autorizzate da D3, ma non cambia: l'autorizzazione non serve più", scena)
		}
	}
	if len(catalogo) < 260 || parti < 1300 {
		t.Errorf("il catalogo è troppo piccolo: %d voci, %d parti", len(catalogo), parti)
	}
	t.Logf("non regressione di P7e: %d voci, %d parti, %d diverse, %d autorizzate da D3", len(nomi), parti, diverse, autorizzate)
}
