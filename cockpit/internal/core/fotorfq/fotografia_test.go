// L1 — la fotografia di A1c (A1c-L1-03; piano A, 6.4.1 e 3.4; contratto di A1c, §3 e §3.5): Ordina dà un ordine
// totale; ImprontaFotografia è la stessa dopo qualunque permutazione degli elenchi e senza PresaIl e Sorgente, cambia
// con un dato e non tocca la fotografia di chi chiama; ValidaFotografia tace su una fotografia giusta e dice ogni
// riferimento che non si risolve, un candidato dell'agente, un tempo non UTC al millisecondo, fatti a un'altra
// terna; ImprontaPayload è la stessa per i byte del jsonb e per la stringa dell'export; i tag JSON dei record sono
// snake_case.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID di prova): il repository è pubblico.

package fotorfq

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// id: un UUID di prova, 00000000-0000-4000-8000-0000000000nn.
func id(n int) uuid.UUID {
	u := uuid.MustParse("00000000-0000-4000-8000-000000000000")
	u[14], u[15] = byte(n>>8), byte(n)
	return u
}

func pid(n int) *uuid.UUID { u := id(n); return &u }
func ps(s string) *string  { return &s }

// ora: un tempo UTC al millisecondo, a minuti dall'inizio della scena.
func ora(minuti int) time.Time {
	return time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC).Add(time.Duration(minuti) * time.Minute)
}

func pora(minuti int) *time.Time { t := ora(minuti); return &t }

const (
	shaStep = "aaaa000000000000000000000000000000000000000000000000000000000001"
	shaPDF  = "bbbb000000000000000000000000000000000000000000000000000000000002"
	hashCfg = "cccc000000000000000000000000000000000000000000000000000000000003"
)

// fotografiaACME: una RFQ inventata, con tutti i record di A1c e i riferimenti che si risolvono.
func fotografiaACME(t *testing.T) Fotografia {
	t.Helper()
	terna := Terna{Versione: 4, HashConfigurazione: hashCfg}
	fatti := func(sha, payload string, motivo *string) Fatti {
		d, err := ImprontaPayload(json.RawMessage(payload))
		if err != nil {
			t.Fatal(err)
		}
		return Fatti{Sha256: sha, Terna: terna, CalcolatoIl: ora(5), Payload: json.RawMessage(payload), Digest: d, MotivoParziale: motivo}
	}
	thread, cliente, operatore := id(1), id(2), id(3)
	m1, m2 := id(10), id(11)
	aStep, aPDF, aZip, aVoce := id(20), id(21), id(22), id(23)
	finito, sciolto := id(30), id(31)
	docStep, docPDF := id(40), id(41)
	sezioni := map[string]StatoSezione{}
	for _, k := range ChiaviSezioni {
		sezioni[k] = StatoSezione{Stato: StatoSezioneCompleta}
	}
	return Fotografia{
		VersioneSchema: VersioneSchema, Origine: OrigineDSN, Sorgente: "127.0.0.1:5432/acme_prova_test come prove_acme",
		SchemaDB: 21, PresaIl: ora(100), Coerente: true, SolaLettura: "on", Isolamento: "repeatable read",
		Analizzatore: &terna, Sezioni: sezioni,
		Clienti: []Cliente{{ID: cliente, RagioneSociale: "ACME S.p.A."}},
		Utenti:  []Utente{{ID: operatore, Sigla: "AC"}, {ID: id(4), Sigla: "AB"}},
		Thread: []Thread{{
			ID: thread, ClienteID: cliente, Stato: "APERTA", Oggetto: ps("RFQ ACME26-030"), CreatoDa: &operatore, CreatoIl: ora(1),
			Messaggi: []Messaggio{
				{ID: m1, ConversazioneID: id(12), ThreadID: &thread, Canale: "outlook", Direzione: "entrata", DataEvento: ora(2)},
				{ID: m2, ConversazioneID: id(12), ThreadID: &thread, Canale: "outlook", Direzione: "entrata", DataEvento: ora(3)},
			},
			Agganci: []AggancioMessaggio{
				{MessaggioID: m1, Aggancio: "operatore", AgganciatoDa: &operatore, AgganciatoIl: pora(4)},
				{MessaggioID: m2, Aggancio: "auto_conversazione"},
			},
			Allegati: []Allegato{
				{ID: aStep, MessaggioID: m1, Indice: 1, NomeFile: "ACME-030P7120001.stp", Natura: "file", Sha256: ps(shaStep), RicevutoIl: ora(2)},
				{ID: aPDF, MessaggioID: m1, Indice: 2, NomeFile: "7120002.pdf", Natura: "file", Sha256: ps(shaPDF), RicevutoIl: ora(2)},
				{ID: aZip, MessaggioID: m2, Indice: 1, NomeFile: "pacco_acme.zip", Natura: "file", RicevutoIl: ora(3)},
				{ID: aVoce, MessaggioID: m2, ContenitoreID: &aZip, Indice: 1, NomeFile: "7120003.pdf", Natura: "file", RicevutoIl: ora(3)},
			},
			Fatti: map[string]Fatti{
				shaStep: fatti(shaStep, `{"struttura": {"radici": ["#1"]}}`, ps("")),
				shaPDF:  fatti(shaPDF, `{"testo_pdf": {"pagine": 1}}`, nil),
			},
			Proposte: []PropostaAttuale{
				{ID: id(50), AllegatoID: aPDF, ThreadID: &thread, Tipo: "disegno_2d", Codice: ps("7120002"), Fonte: "nome_file", Stato: "confermata",
					DecisoDa: &operatore, DecisoIl: pora(6), Dettagli: json.RawMessage(`{"destinazione": {"esito": "figlio"}}`)},
				{ID: id(51), AllegatoID: aVoce, ThreadID: &thread, Tipo: "disegno_2d", Fonte: "estensione", Stato: "aperta", ComponenteID: &sciolto},
			},
			Documenti: []DocumentoConfermato{
				{ID: docStep, ThreadID: thread, ComponenteID: &finito, Tipo: "cad_3d", NomeFile: "ACME-030P7120001.stp", Estensione: "stp",
					Sha256: shaStep, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: ora(7), Allegati: []uuid.UUID{aStep}},
				{ID: docPDF, ThreadID: thread, ComponenteID: &sciolto, Tipo: "disegno_2d", Codice: ps("7120002"), NomeFile: "7120002.pdf",
					Estensione: "pdf", Sha256: shaPDF, StatoNas: "in_coda", ConfermatoDa: operatore, ConfermatoIl: ora(8), Allegati: []uuid.UUID{aPDF}},
			},
			Identificativi: []Identificativo{
				{Codice: "7120001", Origine: "proposta_oggetto", ConfermatoDa: &operatore, CreatoIl: ora(1)},
				{Codice: "7120009", Origine: "proposta_corpo", CreatoIl: ora(1)},
			},
			Componenti: []Componente{
				{ID: finito, Codice: "7120001", Tipo: "finito", Origine: "codice_rilevato", ConfermatoDa: operatore, CreatoIl: ora(9), StepStrutturaleID: &docStep},
				{ID: sciolto, Codice: "7120002", Rev: ps("1"), Tipo: "sciolto", Origine: "step", ConfermatoDa: operatore, CreatoIl: ora(10)},
			},
			Relazioni: []Relazione{{PadreID: finito, FiglioID: sciolto, Qta: 2, Origine: "step", ConfermatoDa: operatore, CreatoIl: ora(11)}},
			RigheComponenteProposta: []RigaComponenteProposta{
				{ID: id(60), AllegatoID: aStep, NomeFile: "ACME-030P7120001.stp", Sha256: shaStep, Chiave: "#1", IDGrezzo: "ACME-030P7120001",
					NomeGrezzo: "7120001", Famiglia: "acme", Fonte: "step", Stato: "confermata", ComponenteID: &finito,
					DecisoDa: &operatore, DecisoIl: pora(12),
					Marcatura: &MarcaturaStrutturale{Versione: 1, Ruolo: "radice", ComponenteID: &finito, DocumentoID: &docStep,
						DichiaratoDa: &operatore, DichiaratoIl: "2026-10-06T08:12:00Z"}},
				{ID: id(61), AllegatoID: aStep, NomeFile: "ACME-030P7120001.stp", Sha256: shaStep, Chiave: "#2", IDGrezzo: "7120002",
					NomeGrezzo: "7120002", Famiglia: "acme", Fonte: "step", Stato: "aperta", Codice: ps("7120002"),
					OrigineCodice: ps("operatore"), Albero: &SegnoAlbero{Da: operatore, Il: "2026-10-06T08:13:00Z", Firma: "f1", Nodo: "cod:7120002"}},
			},
			RigheRelazioneProposta: []RigaRelazioneProposta{{AllegatoID: aStep, NomeFile: "ACME-030P7120001.stp", PadreChiave: "#1", FiglioChiave: "#2",
				Qta: 2, Stato: "confermata", Albero: &SegnoAlbero{Da: operatore, Il: "2026-10-06T08:13:00Z", Firma: "f1", Padre: &finito, Figlio: &sciolto}}},
			RimozioniAperte: []RimozioneAperta{{StepDocumentoID: docStep, PadreID: finito, FiglioID: sciolto, QtaWorking: 2}},
			StepProdotto: []RigaStepProdotto{{ComponenteID: finito, Codice: "7120001", StepStrutturaleID: &docStep, NStepCorrenti: 1,
				AnalisiCompleta: true, MotivoParziale: ps(""), Esito: "confermato"}},
			Fascicolo: []RigaFascicolo{
				{ComponenteID: finito, Codice: "7120001", TipoComponente: "finito", TipoDocumento: "cad_3d", Bloccante: true, DocumentoID: &docStep, Esito: "ok"},
				{ComponenteID: sciolto, Codice: "7120002", TipoComponente: "sciolto", TipoDocumento: "disegno_2d", Bloccante: true, DocumentoID: &docPDF, Esito: "ok"},
			},
			Fabbisogni: []RigaFabbisogno{
				{TipoComponente: "finito", TipoDocumento: "cad_3d", Bloccante: true},
				{TipoComponente: "finito", TipoDocumento: "disegno_2d", Bloccante: true},
				{TipoComponente: "sciolto", TipoDocumento: "disegno_2d", Bloccante: true},
			},
			Deroghe:         []DerogaFabbisogno{{ID: id(70), ComponenteID: sciolto, Tipo: "sviluppo_dxf", Motivo: "lo facciamo noi", UtenteID: operatore, CreataIl: ora(13)}},
			Triage:          []Triage{{ID: id(80), MessaggioID: m1, Esito: "nuova_rfq", Stato: "accettata", Identificativi: []string{"7120001"}, CreatoIl: ora(2), DecisoIl: pora(4)}},
			CandidatiCodice: []CandidatoCodice{{MessaggioID: m1, Codice: "7120001", Ruolo: "prodotto", Origine: "famiglia", Punteggio: 80}},
			InAttesa:        []uuid.UUID{aVoce},
			VersioneBOM:     &VersioneBOM{ID: id(90), Numero: 1, Stato: "congelata", Contesto: "preventivo", CongelataDa: &operatore, CongelataIl: pora(14)},
			UltimaCongelata: &VersioneBOM{ID: id(90), Numero: 1, Stato: "congelata", Contesto: "preventivo", CongelataDa: &operatore, CongelataIl: pora(14)},
		}},
		FuoriRFQ: []MessaggioFuoriRFQ{{
			Messaggio: Messaggio{ID: id(100), ConversazioneID: id(101), Canale: "outlook", Direzione: "entrata", DataEvento: ora(20)},
			Aggancio:  AggancioMessaggio{MessaggioID: id(100), Aggancio: "nessuno"},
			Allegati:  []Allegato{{ID: id(102), MessaggioID: id(100), Indice: 1, NomeFile: "9999999A_2.pdf", Natura: "file", Sha256: ps(shaPDF), RicevutoIl: ora(20)}},
			Fatti:     map[string]Fatti{shaPDF: fatti(shaPDF, `{"testo_pdf": {"pagine": 1}}`, nil)},
		}},
		Diagnostiche: []evidenze.Diagnostica{
			{Codice: CodiceSezioneParziale, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati, Percorso: "sezioni.b", Messaggio: "inventata"},
			{Codice: CodiceSezioneParziale, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati, Percorso: "sezioni.a", Messaggio: "inventata"},
		},
	}
}

// rovescia: tutti gli elenchi al contrario, per provare che l'ordine non viene dall'ingresso.
func rovescia(f *Fotografia) {
	slices.Reverse(f.Clienti)
	slices.Reverse(f.Utenti)
	slices.Reverse(f.Thread)
	slices.Reverse(f.FuoriRFQ)
	slices.Reverse(f.Diagnostiche)
	for i := range f.Thread {
		t := &f.Thread[i]
		slices.Reverse(t.Messaggi)
		slices.Reverse(t.Agganci)
		slices.Reverse(t.Allegati)
		slices.Reverse(t.Proposte)
		slices.Reverse(t.Documenti)
		slices.Reverse(t.Identificativi)
		slices.Reverse(t.Componenti)
		slices.Reverse(t.Relazioni)
		slices.Reverse(t.RigheComponenteProposta)
		slices.Reverse(t.RigheRelazioneProposta)
		slices.Reverse(t.RimozioniAperte)
		slices.Reverse(t.StepProdotto)
		slices.Reverse(t.Fascicolo)
		slices.Reverse(t.Fabbisogni)
		slices.Reverse(t.Deroghe)
		slices.Reverse(t.Triage)
		slices.Reverse(t.CandidatiCodice)
		slices.Reverse(t.InAttesa)
	}
}

func TestOrdinaDaUnOrdineTotale(t *testing.T) {
	a := fotografiaACME(t)
	a.Ordina()
	b := fotografiaACME(t)
	rovescia(&b)
	b.Ordina()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("due fotografie con gli stessi dati in ordine diverso restano diverse dopo Ordina")
	}
	th := a.Thread[0]
	// allegati: data del messaggio, messaggio, contenitore prima, indice, ID
	var nomi []string
	for _, x := range th.Allegati {
		nomi = append(nomi, x.NomeFile)
	}
	if got := strings.Join(nomi, ","); got != "ACME-030P7120001.stp,7120002.pdf,pacco_acme.zip,7120003.pdf" {
		t.Errorf("ordine degli allegati: %s", got)
	}
	if th.Messaggi[0].ID != id(10) || th.Identificativi[0].Codice != "7120001" || a.Utenti[0].ID != id(3) {
		t.Errorf("ordine per chiave: messaggi %v, identificativi %v, utenti %v", th.Messaggi[0].ID, th.Identificativi[0].Codice, a.Utenti[0].ID)
	}
	if a.Diagnostiche[0].Percorso != "sezioni.a" {
		t.Errorf("ordine delle diagnostiche: %v", a.Diagnostiche)
	}
}

func TestImprontaFotografiaStabile(t *testing.T) {
	base := fotografiaACME(t)
	h, err := ImprontaFotografia(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 64 {
		t.Fatalf("impronta: %q", h)
	}

	permutata := fotografiaACME(t)
	rovescia(&permutata)
	if got, err := ImprontaFotografia(permutata); err != nil || got != h {
		t.Errorf("dopo le permutazioni: %s (%v), attesa %s", got, err, h)
	}
	// la fotografia di chi chiama non è stata riordinata
	if permutata.Thread[0].Allegati[0].NomeFile != "7120003.pdf" {
		t.Error("ImprontaFotografia ha riordinato la fotografia di chi chiama")
	}

	altrove := fotografiaACME(t)
	altrove.Sorgente = "export della cartella inventata"
	altrove.PresaIl = ora(999)
	if got, _ := ImprontaFotografia(altrove); got != h {
		t.Error("PresaIl e Sorgente entrano nell'impronta")
	}
	altrove.PresaIl = time.Time{}
	if got, _ := ImprontaFotografia(altrove); got != h {
		t.Error("PresaIl zero (export) cambia l'impronta")
	}

	for nome, cambia := range map[string]func(*Fotografia){
		"il codice di un componente": func(f *Fotografia) { f.Thread[0].Componenti[1].Codice = "7120008" },
		"il gesto 1":                 func(f *Fotografia) { f.Thread[0].Agganci[1].AgganciatoDa = pid(3) },
		"il gesto 3":                 func(f *Fotografia) { f.Thread[0].Componenti[0].StepStrutturaleID = nil },
		"la versione della BOM":      func(f *Fotografia) { f.Thread[0].VersioneBOM.Stato = "bozza" },
		"un fabbisogno":              func(f *Fotografia) { f.Thread[0].Fabbisogni[0].Bloccante = false },
		"la marcatura sospesa":       func(f *Fotografia) { f.Thread[0].RigheComponenteProposta[0].Marcatura.Sospesa = true },
		"lo schema":                  func(f *Fotografia) { f.SchemaDB = 20 },
		"una sezione":                func(f *Fotografia) { f.Sezioni[SezioneFatti] = StatoSezione{Stato: StatoSezioneAssente} },
	} {
		f := fotografiaACME(t)
		cambia(&f)
		if got, _ := ImprontaFotografia(f); got == h {
			t.Errorf("%s cambiato: l'impronta è la stessa", nome)
		}
	}

	rotta := fotografiaACME(t)
	rotta.Thread[0].Fatti[shaPDF] = Fatti{Sha256: shaPDF, Terna: *rotta.Analizzatore, Payload: json.RawMessage(`{"a":1,"a":2}`)}
	if _, err := ImprontaFotografia(rotta); err == nil {
		t.Error("un payload con una chiave ripetuta dà un'impronta")
	}
}

func TestValidaFotografia(t *testing.T) {
	if d := ValidaFotografia(fotografiaACME(t)); len(d) != 0 {
		t.Fatalf("una fotografia giusta ha diagnostiche: %+v", d)
	}
	estraneo := pid(999)
	casi := []struct {
		nome    string
		rompi   func(*Fotografia)
		codice  string
		percors string
	}{
		{"allegato senza messaggio", func(f *Fotografia) { f.Thread[0].Allegati[0].MessaggioID = *estraneo }, CodiceRiferimentoNonRisolto, ".allegati["},
		{"aggancio senza messaggio", func(f *Fotografia) { f.Thread[0].Agganci[0].MessaggioID = *estraneo }, CodiceRiferimentoNonRisolto, ".agganci["},
		{"proposta senza allegato", func(f *Fotografia) { f.Thread[0].Proposte[0].AllegatoID = *estraneo }, CodiceRiferimentoNonRisolto, ".proposte["},
		{"provenienza senza allegato", func(f *Fotografia) { f.Thread[0].Documenti[0].Allegati = []uuid.UUID{*estraneo} }, CodiceRiferimentoNonRisolto, ".documenti["},
		{"STEP strutturale del componente", func(f *Fotografia) { f.Thread[0].Componenti[0].StepStrutturaleID = estraneo }, CodiceRiferimentoNonRisolto, ".step_strutturale_id"},
		{"riga di v_step_prodotto senza componente", func(f *Fotografia) { f.Thread[0].StepProdotto[0].ComponenteID = *estraneo }, CodiceRiferimentoNonRisolto, ".step_prodotto["},
		{"STEP strutturale della vista", func(f *Fotografia) { f.Thread[0].StepProdotto[0].StepStrutturaleID = estraneo }, CodiceRiferimentoNonRisolto, ".step_prodotto["},
		{"riga di v_fascicolo senza componente", func(f *Fotografia) { f.Thread[0].Fascicolo[0].ComponenteID = *estraneo }, CodiceRiferimentoNonRisolto, ".fascicolo["},
		{"documento di v_fascicolo", func(f *Fotografia) { f.Thread[0].Fascicolo[1].DocumentoID = estraneo }, CodiceRiferimentoNonRisolto, ".documento_id"},
		{"deroga senza componente", func(f *Fotografia) { f.Thread[0].Deroghe[0].ComponenteID = *estraneo }, CodiceRiferimentoNonRisolto, ".deroghe["},
		{"documento della marcatura", func(f *Fotografia) { f.Thread[0].RigheComponenteProposta[0].Marcatura.DocumentoID = estraneo }, CodiceRiferimentoNonRisolto, ".marcatura.documento_id"},
		{"STEP della rimozione", func(f *Fotografia) { f.Thread[0].RimozioniAperte[0].StepDocumentoID = *estraneo }, CodiceRiferimentoNonRisolto, ".rimozioni_aperte["},
		{"candidato dell'agente", func(f *Fotografia) { f.Thread[0].CandidatiCodice[0].Origine = "agente" }, CodiceRiferimentoNonRisolto, ".candidati_codice["},
		{"tempo non UTC", func(f *Fotografia) { f.Thread[0].Messaggi[0].DataEvento = ora(2).In(time.FixedZone("CEST", 7200)) }, CodiceRiferimentoNonRisolto, ".data_evento"},
		{"tempo con i microsecondi", func(f *Fotografia) { f.Thread[0].Componenti[0].CreatoIl = ora(9).Add(time.Microsecond) }, CodiceRiferimentoNonRisolto, ".creato_il"},
		{"fatti a un'altra terna", func(f *Fotografia) {
			x := f.Thread[0].Fatti[shaPDF]
			x.Terna.Versione = 3
			f.Thread[0].Fatti[shaPDF] = x
		}, CodiceTernaDiversa, ".fatti["},
		{"fatti senza terna corrente", func(f *Fotografia) { f.Analizzatore = nil }, CodiceTernaDiversa, ".terna"},
		{"fatti sotto un altro sha256", func(f *Fotografia) { f.Thread[0].Fatti[shaStep] = f.Thread[0].Fatti[shaPDF] }, CodiceRiferimentoNonRisolto, ".sha256"},
		{"allegato fuori RFQ di un altro messaggio", func(f *Fotografia) { f.FuoriRFQ[0].Allegati[0].MessaggioID = *estraneo }, CodiceRiferimentoNonRisolto, "fuori_rfq["},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			f := fotografiaACME(t)
			c.rompi(&f)
			d := ValidaFotografia(f)
			trovata := false
			for _, x := range d {
				if x.Codice == c.codice && strings.Contains(x.Percorso, c.percors) && x.Natura == evidenze.NaturaContratto && x.Gravita == evidenze.GravitaErrore {
					trovata = true
				}
			}
			if !trovata {
				t.Errorf("nessuna diagnosi %s su %q: %+v", c.codice, c.percors, d)
			}
		})
	}
}

// Lo stesso payload come lo dà il jsonb (chiavi riordinate da PostgreSQL, spazi dopo i due punti) e come lo
// scrive un export (compatto, altro ordine, l'escape di «è»): la stessa impronta (A1b-02, ripresa in A1c-L1-03).
func TestImprontaPayloadUgualeFraDBEdExport(t *testing.T) {
	daJSONB := json.RawMessage(`{"nome": "pezzo è", "radici": ["#1", "#2"], "quantita": 800}`)
	daExport := json.RawMessage(`{"radici":["#1","#2"],"quantita":800,"nome":"pezzo è"}`)
	a, err := ImprontaPayload(daJSONB)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ImprontaPayload(daExport)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("impronte diverse: jsonb %s, export %s", a, b)
	}
}

// I tag JSON dei record di A1c sono snake_case (contratto §3, regola 3): ogni campo esportato ha un tag, in
// minuscolo, con il trattino basso.
func TestITagDeiRecordSonoSnakeCase(t *testing.T) {
	snake := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for _, v := range []any{Fotografia{}, StatoSezione{}, Cliente{}, Utente{}, MessaggioFuoriRFQ{}, Thread{}, AggancioMessaggio{},
		Identificativo{}, Componente{}, Relazione{}, RigaComponenteProposta{}, MarcaturaStrutturale{}, SegnoAlbero{},
		RigaRelazioneProposta{}, RimozioneAperta{}, DocumentoConfermato{}, PropostaAttuale{}, RigaStepProdotto{}, RigaFascicolo{},
		DerogaFabbisogno{}, VersioneBOM{}, RigaFabbisogno{}, Triage{}, CandidatoCodice{}} {
		ty := reflect.TypeOf(v)
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			nome, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if !snake.MatchString(nome) {
				t.Errorf("%s.%s: tag json %q non è snake_case", ty.Name(), f.Name, nome)
			}
		}
	}
}
