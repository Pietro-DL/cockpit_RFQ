package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/registro/fornitori"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// I template vengono compilati in Init, ma gli errori di campo (nome sbagliato, metodo mancante) escono solo
// all'esecuzione: qui si eseguono i frammenti principali con dati sintetici.
func serverTest(t *testing.T) *Server {
	t.Helper()
	templ, _ := fs.Sub(risorse.FS, "web/templates")
	static, _ := fs.Sub(risorse.FS, "web/static")
	s := &Server{Templ: templ, Static: static}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func txtT(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func datiSintetici() (*messaggioDati, *triageDati, *threadDati) {
	mid, tid, aid, pid := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	m := db.Messaggio{MessaggioID: mid, Canale: db.CanaleOutlook, Direzione: db.DirezioneEntrata, DataEvento: time.Now(),
		MittenteNome: txtT("Mario Rossi"), MittenteIndirizzo: txtT("mario.rossi@acme.example"), Oggetto: txtT("RFQ 1234567A"), CorpoTesto: txtT("testo"),
		ThreadID: uuid.NullUUID{UUID: tid, Valid: true}, Aggancio: db.AggancioOperatore}
	prop := db.DocumentoProposta{PropostaID: pid, AllegatoID: aid, TipoProposto: db.TipoDocumentoDisegno2d, Codice: txtT("1234567A"), Rev: txtT("4"),
		Confidenza: 70, Fonte: db.FontePropostaNomeFile, Stato: db.StatoPropostaAperta, Dettagli: json.RawMessage(`{"pre_spunta":true}`)}
	figlio := AllegatoUI{Allegato: db.Allegato{AllegatoID: uuid.New(), MessaggioID: mid, ContenitoreID: uuid.NullUUID{UUID: aid, Valid: true}, Indice: 1,
		NomeFile: "1234567A.stp", Estensione: txtT("stp"), Natura: db.NaturaAllegatoFile, Stato: db.StatoAllegatoInStaging, PathStaging: txtT("C:\\x"), Sha256: txtT(strings.Repeat("a", 64))},
		Proposta: &db.DocumentoProposta{PropostaID: uuid.New(), TipoProposto: db.TipoDocumentoCad3d, Stato: db.StatoPropostaAperta, Confidenza: 70, Fonte: db.FontePropostaEstensione}, FileMancante: true}
	allegati := []AllegatoUI{
		{Allegato: db.Allegato{AllegatoID: aid, MessaggioID: mid, Indice: 1, NomeFile: "1234567A_4.pdf", Estensione: txtT("pdf"), Natura: db.NaturaAllegatoFile,
			Stato: db.StatoAllegatoAnalizzato, PathStaging: txtT("C:\\x"), Sha256: txtT(strings.Repeat("b", 64)), Bytes: pgtype.Int8{Int64: 1000, Valid: true}},
			Proposta: &prop, PreSpunta: true, Figli: []AllegatoUI{figlio},
			Documento: &db.Documento{DocumentoID: uuid.New(), StatoNas: db.StatoNasScritto, PathRelativo: `ELENCO DISEGNI\1234567A\1234567A_4.pdf`}},
		{Allegato: db.Allegato{AllegatoID: uuid.New(), MessaggioID: mid, Indice: 2, NomeFile: "logo.png", Natura: db.NaturaAllegatoInline}},
		{Allegato: db.Allegato{AllegatoID: uuid.New(), MessaggioID: mid, Indice: 3, NomeFile: "disegni.zip", Natura: db.NaturaAllegatoFile, Stato: db.StatoAllegatoGrezzo, Errore: txtT("in coda")},
			Proposta: &db.DocumentoProposta{PropostaID: uuid.New(), TipoProposto: db.TipoDocumentoAltro, Stato: db.StatoPropostaAperta, Confidenza: 20, Fonte: db.FontePropostaEstensione}, PreSpunta: true},
	}
	th := db.ThreadOfferta{ThreadID: tid, CartellaRelativa: txtT(`ACME\WIP\2026 09 08 Rossi RFQ 1234567A`), Oggetto: txtT("RFQ 1234567A"), DataInizio: time.Now(), Stato: db.StatoThreadAPERTA}
	copia := coda.Copia{CasellaID: uuid.New(), EntryID: "E1", CasellaNome: "Commerciale", NonLetto: true, WorkerNome: "outlook@PC-FRANCESCO"}
	md := &messaggioDati{M: m, Riga: db.VInbox{MessaggioID: mid, TriageEsito: txtT("nuova_rfq"), TriageConfidenza: pgtype.Int2{Int16: 80, Valid: true}}, Copia: &copia,
		Presenze: []db.ListPresenzeRow{{MessaggioID: mid, EntryID: "E1", CasellaNome: "Commerciale", Cartella: txtT("Posta in arrivo"), NonLetto: true},
			{MessaggioID: mid, EntryID: "E2", CasellaNome: "Francesco", Cartella: txtT("Posta in arrivo")}},
		Allegati: allegati, Thread: &th, Avviso: "ok"}
	td := &triageDati{M: m, Riga: md.Riga, Azione: "nuova", Clienti: []db.Cliente{{ClienteID: uuid.New(), CartellaNas: "ACME", RagioneSociale: "Acme"}},
		Buyers: []db.Buyer{{BuyerID: uuid.New(), Cognome: "Rossi", Nome: txtT("Mario"), Email: txtT("mario.rossi@acme.example")}},
		Nome:   "Mario", Cognome: "Rossi", Email: "mario.rossi@acme.example", Dominio: "acme.example", Oggetto: "RFQ 1234567A", Allegati: allegati, Anteprima: `ACME\WIP\x`,
		Riferimento: "RDO 400012345",
		Proponibili: []db.CandidatoCodice{{MessaggioID: mid, Codice: "1234567A", Ruolo: db.RuoloCodiceProdotto,
			Origine: db.OrigineCodiceFamiglia, Famiglia: "7 cifre + lettera", Punteggio: 80, Evidenza: "oggetto"}},
		Altri: []db.CandidatoCodice{{MessaggioID: mid, Codice: "20260908", Ruolo: db.RuoloCodiceNonClassificato,
			Origine: db.OrigineCodiceGenerico, Punteggio: 30, Evidenza: "corpo"}},
		Candidati: []db.ListCandidatiAggancioRow{
			{MessaggioID: mid, ThreadID: tid, Regola: db.RegolaAggancioR0Reply, Punteggio: 98,
				Evidenza: "In-Reply-To punta a un messaggio agganciato", ThreadStato: db.StatoThreadAPERTA,
				Oggetto: txtT("RFQ 1234567A"), DataInizio: time.Now(), Cliente: "Acme"},
			{MessaggioID: mid, ThreadID: uuid.New(), Regola: db.RegolaAggancioR2Oggetto, Punteggio: 55,
				Evidenza: "stesso oggetto", ThreadStato: db.StatoThreadCHIUSA,
				Oggetto: txtT("RFQ 1234567A"), DataInizio: time.Now(), Cliente: "Acme"}}}
	thd := &threadDati{T: th, Riga: db.VCruscotto{Cliente: "ACME", NomeFase: db.NullFase{Fase: db.FaseRICEVUTA, Valid: true}, Semaforo: txtT("verde")},
		Identificativi: []db.IdentificativoThread{{Codice: "1234567A"}},
		Messaggi:       []messaggioThread{{M: m, Copia: &copia, Allegati: allegati}, {M: m, Motivo: "nessuna postazione associata alla sessione"}},
		Documenti:      []db.Documento{{Tipo: db.TipoDocumentoDisegno2d, Codice: txtT("1234567A"), PathRelativo: "x", StatoNas: db.StatoNasInCoda}},
		Fascicolo:      []db.VFascicolo{{Codice: "1234567A", TipoComponente: db.TipoComponenteSciolto, TipoDocumento: db.TipoDocumentoDisegno2d, Bloccante: true, Esito: "manca"}},
		Avviso:         "ok"}
	return md, td, thd
}

// datiIntegritaSintetici: una riga per ogni problema che ha un'azione diversa dalle altre.
func datiIntegritaSintetici() *integritaDati {
	riga := func(p db.ProblemaNas, stato db.StatoNas, trovato string) rigaIntegrita {
		return rigaIntegrita{db.ListAnomalieNasRow{
			NasAnomalium: db.NasAnomalium{
				DocumentoID: uuid.New(), ThreadID: uuid.New(), Problema: p, StatoDb: stato,
				Percorso:  `ACME\WIP\2026 09 17 prova\ELENCO DISEGNI\1234567A_4.pdf`,
				ShaAtteso: strings.Repeat("a", 64), ShaTrovato: txtT(trovato),
				Dettaglio: "dettaglio di prova", RilevataIl: time.Now(), VistaIl: time.Now(),
			},
			NomeFile: "1234567A_4.pdf", Codice: txtT("1234567A"), Rev: txtT("4"),
			Tipo: db.TipoDocumentoDisegno2d, Oggetto: txtT("RFQ 1234567A"), RagioneSociale: "Acme S.p.A.",
		}}
	}
	ora := time.Now()
	return &integritaDati{
		Radice: `\\server\PROVA`, Raggiungibile: true, Scrittura: true, Automatico: 15 * time.Minute,
		Documenti: 9, Verificati: 9, UltimoControllo: &ora,
		Conta: []db.ContaAnomalieNasRow{{Problema: db.ProblemaNasMancante, N: 1}, {Problema: db.ProblemaNasConflitto, N: 1}},
		Righe: []rigaIntegrita{
			riga(db.ProblemaNasMancante, db.StatoNasScritto, ""),
			riga(db.ProblemaNasConflitto, db.StatoNasScritto, strings.Repeat("b", 64)),
			riga(db.ProblemaNasGiaPresente, db.StatoNasInCoda, strings.Repeat("a", 64)),
			riga(db.ProblemaNasInAttesa, db.StatoNasInCoda, ""),
			riga(db.ProblemaNasErrore, db.StatoNasErrore, ""),
			riga(db.ProblemaNasIlleggibile, db.StatoNasScritto, ""),
		},
	}
}

func TestFrammentiEseguono(t *testing.T) {
	s := serverTest(t)
	md, td, thd := datiSintetici()
	casi := []struct {
		pagina, nome string
		dati         any
		attesi       []string
	}{
		{"inbox.html", "messaggio_pannello", md, []string{"Scarica selezionati", "Conferma", "NAS: scritto", "file mancante", "in coda", "Apri RFQ"}},
		{"inbox.html", "triage_form", td, []string{"Crea RFQ", "nuovo cliente", "1234567A_4.pdf", "checked"}},
		{"inbox.html", "triage_form", func() *triageDati { x := *td; x.Azione = "aggancia"; return &x }(), []string{"/thread/cerca", "Aggancia"}},
		{"inbox.html", "buyer_select", td, []string{"Rossi Mario"}},
		{"inbox.html", "thread_risultati", []db.VCruscotto{{ThreadID: uuid.New(), Cliente: "ACME", Oggetto: txtT("x"), DataInizio: time.Now()}}, []string{"ACME"}},
		{"thread.html", "thread_corpo", thd, []string{"Documenti sul NAS", "Fascicolo", "RICEVUTA", "Scarica selezionati"}},
		{"inbox.html", "stato_worker", nil, []string{"stato-worker", "Sei su:", "PC-FRANCESCO", "Francesco attiva", "Commerciale OFFLINE", "Luigi non configurata", "analisi mai avviata"}},
		// blocco 5B: la schermata dell'integrita' con una riga per ogni tipo di problema. Un enum nuovo
		// che non finisce ne' in StatoFilesystem ne' in Azione esce qui, e non davanti all'operatore.
		{"integrita.html", "integrita_corpo", datiIntegritaSintetici(), []string{
			"Integrità NAS", "Controlla ora", "mancante", "conflitto", "gia_presente",
			"presente, contenuto DIVERSO", "presente, hash corretto",
			"/riaccoda", "/allinea", "nessuna azione automatica"}},
		{"inbox.html", "messaggio_pannello", func() *messaggioDati {
			x := *md
			x.Copia, x.MotivoAzioni = nil, "nessuna postazione associata a questa sessione"
			return &x
		}(), []string{"azioni Outlook non disponibili", "nessuna postazione associata"}},
		// blocco 7A: la lista a quadranti con una riga per controparte, il pannello di un fornitore
		// (senza «Nuova RFQ»), il form di censimento, la scheda fornitore, l'import e la sezione
		// «Lavorazioni e fornitori» del cliente.
		{"inbox.html", "inbox_lista", inboxDati{Quadrante: "", Filtro: "tutti", Righe: []db.VInbox{
			{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("cliente"), ControparteTipo: "cliente", Controparte: txtT("ACME")},
			{MessaggioID: uuid.New(), Direzione: db.DirezioneUscita, DataEvento: time.Now(), Oggetto: txtT("fornitore"), ControparteTipo: "fornitore", Controparte: txtT("Fresature Esempio")},
			{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("ambiguo"), ControparteTipo: "ambiguo"},
			{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("interno"), ControparteTipo: "interno"},
			{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("ignoto"), ControparteTipo: "sconosciuto"},
		}}, []string{"→ ACME", "chip fornitore", "Fresature Esempio", ">ambiguo<", ">interna<", ">sconosciuto<", "q=tutti", "in uscita"}},
		{"inbox.html", "messaggio_pannello", func() *messaggioDati {
			x := *md
			x.Thread = nil
			x.Riga.ControparteTipo, x.Riga.Controparte = "fornitore", txtT("Fresature Esempio")
			return &x
		}(), []string{"fornitore · Fresature Esempio", "triage?azione=aggancia", "niente «Nuova RFQ»"}},
		{"inbox.html", "censisci_form", &censisciDati{M: md.M, Riga: md.Riga, Come: "fornitore", Indirizzo: "mario.rossi@acme.example", Dominio: "acme.example",
			Lavorazioni: []db.Lavorazione{{Codice: "zincatura", Descrizione: "Zincatura"}}, Tipi: db.AllTipoFornitoreValues(), Tipo: "processi",
			UsaDominio: true, Scelte: map[string]bool{"zincatura": true}, Errore: "prova di errore"},
			[]string{"Censisci come fornitore", `name="usa_dominio" value="1" checked`, `value="zincatura" checked`, "Censisci e ricalcola", "prova di errore"}},
		{"inbox.html", "censisci_form", &censisciDati{M: md.M, Come: "cliente", Indirizzo: "x@gmail.example", Dominio: "gmail.example", DominioPubblico: true,
			Tipi: db.AllTipoFornitoreValues(), Scelte: map[string]bool{}},
			[]string{"Censisci come cliente", "dominio pubblico", "cartella_nas", "un suo buyer"}},
		{"fornitori.html", "fornitori_corpo", fornitoriDati{Sez: "lavorazioni",
			Fornitori:   []db.ListFornitoriRow{{FornitoreID: uuid.New(), RagioneSociale: "Galvanica Esempio", Tipo: db.TipoFornitoreProcessi, Attivo: true}},
			Scelto:      &db.Fornitore{FornitoreID: uuid.New(), RagioneSociale: "Galvanica Esempio", Tipo: db.TipoFornitoreProcessi, Attivo: true},
			Lavorazioni: []db.Lavorazione{{Codice: "zincatura", Descrizione: "Zincatura"}, {Codice: "cataforesi", Descrizione: "Cataforesi"}},
			Sue:         map[string]bool{"zincatura": true}, Tipi: db.AllTipoFornitoreValues(),
			Qualifiche: []db.ListQualificheFornitoreRow{{ClienteID: uuid.New(), Lavorazione: "zincatura", CartellaNas: "ACME", Cliente: "Acme", LavorazioneDescrizione: "Zincatura"}},
			Clienti:    []db.ListClientiTuttiRow{{ClienteID: uuid.New(), CartellaNas: "ACME", RagioneSociale: "Acme"}}},
			[]string{"Lavorazioni e qualifiche", `value="zincatura" checked`, "Salva lavorazioni", "Qualifiche per cliente", "ACME", "Importa da file"}},
		{"fornitori.html", "fornitori_corpo", fornitoriDati{Sez: "posta", Tipi: db.AllTipoFornitoreValues(),
			Scelto:   &db.Fornitore{FornitoreID: uuid.New(), RagioneSociale: "Galvanica Esempio", Tipo: db.TipoFornitoreProcessi, Attivo: true},
			Messaggi: []db.Messaggio{md.M}}, []string{"RFQ 1234567A", "Mario Rossi"}},
		{"importa.html", "importa_corpo", importDati{Testo: "{}", Anteprima: &fornitori.Anteprima{FornitoriDaCreare: []string{"Fresature Esempio"},
			DaAggiungere: []fornitori.Riga{{Fornitore: "Fresature Esempio", Cosa: "dominio", Dettaglio: "fresature-esempio.example"}},
			NonRisolti:   []fornitori.Riga{{Fornitore: "Fresature Esempio", Cosa: "qualifica", Dettaglio: "cliente CLIENTE-IGNOTO non in anagrafica"}}}},
			[]string{"Anteprima: che cosa farebbe", "Fresature Esempio", "fresature-esempio.example", "Non risolti", "CLIENTE-IGNOTO", `value="applica"`}},
		// blocco 7B: il pannello con i candidati verso una richiesta e con la proposta «richiesta
		// mandata a mano». La RFQ senza il box delle richieste e' TestLaRFQNonCreaRichiesteAiFornitori.
		{"inbox.html", "messaggio_pannello", func() *messaggioDati {
			x := *md
			x.Thread = nil
			x.Riga.ControparteTipo, x.Riga.Controparte, x.Riga.TriageAtto, x.Riga.TriageLegame = "fornitore", txtT("Minuterie Esempio"), "offerta", "risposta"
			x.CandidatiRichiesta = []db.ListCandidatiRichiestaRow{
				{RichiestaID: uuid.New(), Regola: db.RegolaRichiestaR0Reply, Punteggio: 95, Evidenza: "In-Reply-To", Fornitore: "Minuterie Esempio", Cliente: "BETA SPORT", OggettoRfq: txtT("RFQ 0X001234AB"), RichiestaStato: db.StatoRichiestaFornitoreInviata},
				{RichiestaID: uuid.New(), Regola: db.RegolaRichiestaR3fCodice, Punteggio: 60, Evidenza: "codice", Fornitore: "Minuterie Esempio", Cliente: "BETA SPORT", OggettoRfq: txtT("RFQ bis"), RichiestaStato: db.StatoRichiestaFornitoreInviata}}
			return &x
		}(), []string{"Risposta a una nostra richiesta?", "risposta-fornitore", ">95<", ">60<", "R3f_codice", `class="chip atto offerta"`, `name="atto"`, `value="offerta" selected`, "legame: risposta"}},
		{"inbox.html", "messaggio_pannello", func() *messaggioDati {
			x := *md
			x.Thread = nil
			x.M.Direzione = db.DirezioneUscita
			x.Riga.ControparteTipo, x.Riga.TriageAtto = "fornitore", "richiesta_offerta"
			x.PropostaThread = &db.ThreadOfferta{ThreadID: uuid.New(), CartellaRelativa: txtT(`BETA SPORT\WIP\x`)}
			x.PropostaFornitore = &db.Fornitore{FornitoreID: uuid.New(), RagioneSociale: "Minuterie Esempio"}
			x.Lavorazioni = []db.Lavorazione{{Codice: "tornitura", Descrizione: "Tornitura"}}
			return &x
		}(), []string{"Richiesta mandata a mano?", "richiesta-fornitore", "Sì, è la richiesta a Minuterie Esempio", "tornitura"}},
		{"inbox.html", "messaggio_pannello", func() *messaggioDati {
			x := *md
			x.Richiesta = &db.RichiestaFornitore{RichiestaID: uuid.New(), Stato: db.StatoRichiestaFornitoreOffertaRicevuta}
			x.RichiestaFornitore = "Minuterie Esempio"
			return &x
		}(), []string{"<b>Richiesta:</b> a Minuterie Esempio", `class="chip richiesta offerta_ricevuta"`}},
		{"anagrafica.html", "anagrafica_corpo", anagraficaDati{Tab: "clienti", Sez: "lavorazioni",
			Clienti: []db.ListClientiTuttiRow{{ClienteID: uuid.New(), CartellaNas: "ACME", RagioneSociale: "Acme", Attivo: true}},
			Scelto:  &db.Cliente{ClienteID: uuid.New(), CartellaNas: "ACME", RagioneSociale: "Acme", Attivo: true},
			Convenzioni: []db.ListConvenzioniClienteRow{
				{ConvenzioneID: uuid.New(), Modo: db.ModoConvenzioneSuffisso, Espressione: "-ZN", Esempio: "AB-ZN", Descrizione: "zincato", Attiva: true, Lavorazioni: []string{"zincatura"}},
				{ConvenzioneID: uuid.New(), Modo: db.ModoConvenzioneRegex, Espressione: "([", Esempio: "X", Descrizione: "rotta", Attiva: true}},
			DiagnosiConvenzioni: []regole.Diagnostica{{Regola: "convenzione 1", Ok: true}, {Regola: "convenzione 2", Ok: false, Motivo: "la regex non compila"}},
			Lavorazioni:         []db.Lavorazione{{Codice: "zincatura", Descrizione: "Zincatura"}},
			Fornitori:           []db.Fornitore{{FornitoreID: uuid.New(), RagioneSociale: "Galvanica Esempio", Tipo: db.TipoFornitoreProcessi}},
			Qualifiche:          []db.ListQualificheClienteRow{{FornitoreID: uuid.New(), Lavorazione: "zincatura", Fornitore: "Galvanica Esempio", Tipo: db.TipoFornitoreProcessi, LavorazioneDescrizione: "Zincatura"}},
			ProvaCodice: &provaCodice{Codice: "AB-ZN", Trovate: []regole.LavorazioneTrovata{{Lavorazione: "zincatura", Descrizione: "zincato", Espressione: "-ZN"}},
				Qualificati: map[string][]db.Fornitore{"zincatura": {{RagioneSociale: "Galvanica Esempio"}}}}},
			[]string{"Convenzioni di codice", `class="spunta si"`, `class="spunta no"`, "la regex non compila", "Fornitori qualificati", "Prova un codice", "<b>zincatura</b>", "Galvanica Esempio", "Lavorazioni e fornitori"}},
	}
	stato := &statoUI{Postazione: "PC-FRANCESCO", PostazioneID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, Origine: "ip",
		Scelte:  []db.Postazione{{PostazioneID: uuid.New(), NomeHost: "PC-FRANCESCO"}},
		Caselle: []statoCasella{{Nome: "Francesco", Stato: "attiva", Classe: "fatto"}, {Nome: "Commerciale", Stato: "offline", Classe: "fallito"}, {Nome: "Luigi", Stato: "non_configurata"}},
		Analisi: statoChip{Etichetta: "analisi mai avviata", Classe: "fallito"}}
	for _, c := range casi {
		var buf bytes.Buffer
		v := vista{Dati: c.dati, Frammento: true, Stato: stato}
		if err := s.pagine[c.pagina].ExecuteTemplate(&buf, c.nome, v); err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		for _, a := range c.attesi {
			if !strings.Contains(buf.String(), a) {
				t.Errorf("%s: manca %q nell'output", c.nome, a)
			}
		}
	}
	// messaggio orfano: niente checkbox attive, bottoni di triage presenti
	md.Thread = nil
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Nuova RFQ") || strings.Contains(buf.String(), "Scarica selezionati") {
		t.Errorf("pannello orfano: triage assente o download consentito")
	}
}

// Correzione prima di B8.2: la pagina della RFQ non mostra piu' il box del blocco 7B, ne' le
// richieste gia' fatte ne' il form per crearne una, e threadDati non ne carica piu' i dati.
// Se il box tornasse nel template con i vecchi campi, l'esecuzione fallirebbe qui; se tornasse
// scritto a mano, lo trovano le frasi vietate.
func TestLaRFQNonCreaRichiesteAiFornitori(t *testing.T) {
	s := serverTest(t)
	_, _, thd := datiSintetici()
	var buf bytes.Buffer
	if err := s.pagine["thread.html"].ExecuteTemplate(&buf, "thread_corpo", vista{Dati: thd, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	pagina := buf.String()
	// la pagina c'e' ancora, con il resto
	for _, atteso := range []string{"Documenti sul NAS", "Fascicolo", "RICEVUTA"} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("manca %q: la pagina della RFQ si e' rotta", atteso)
		}
	}
	for _, vietato := range []string{"Richieste ai fornitori", "Nessuna richiesta ai fornitori", "Nuova richiesta a un fornitore",
		"Crea richiesta", `name="fornitore_id"`, "/richiesta\"", "/richiesta/", "qualificato:", `class="chip richiesta`} {
		if strings.Contains(pagina, vietato) {
			t.Errorf("la pagina della RFQ mostra ancora %q", vietato)
		}
	}
}
