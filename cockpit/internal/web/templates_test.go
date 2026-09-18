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
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/jobs"
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
		MittenteNome: txtT("Mario Rossi"), MittenteIndirizzo: txtT("mario.rossi@acme.example"), Oggetto: txtT("RFQ 6674611A"), CorpoTesto: txtT("testo"),
		ThreadID: uuid.NullUUID{UUID: tid, Valid: true}, Aggancio: db.AggancioOperatore}
	prop := db.DocumentoProposta{PropostaID: pid, AllegatoID: aid, TipoProposto: db.TipoDocumentoDisegno2d, Codice: txtT("6674611A"), Rev: txtT("4"),
		Confidenza: 70, Fonte: db.FontePropostaNomeFile, Stato: db.StatoPropostaAperta, Dettagli: json.RawMessage(`{"pre_spunta":true}`)}
	figlio := AllegatoUI{Allegato: db.Allegato{AllegatoID: uuid.New(), MessaggioID: mid, ContenitoreID: uuid.NullUUID{UUID: aid, Valid: true}, Indice: 1,
		NomeFile: "6674611A.stp", Estensione: txtT("stp"), Natura: db.NaturaAllegatoFile, Stato: db.StatoAllegatoInStaging, PathStaging: txtT("C:\\x"), Sha256: txtT(strings.Repeat("a", 64))},
		Proposta: &db.DocumentoProposta{PropostaID: uuid.New(), TipoProposto: db.TipoDocumentoCad3d, Stato: db.StatoPropostaAperta, Confidenza: 70, Fonte: db.FontePropostaEstensione}, FileMancante: true}
	allegati := []AllegatoUI{
		{Allegato: db.Allegato{AllegatoID: aid, MessaggioID: mid, Indice: 1, NomeFile: "6674611A_4.pdf", Estensione: txtT("pdf"), Natura: db.NaturaAllegatoFile,
			Stato: db.StatoAllegatoAnalizzato, PathStaging: txtT("C:\\x"), Sha256: txtT(strings.Repeat("b", 64)), Bytes: pgtype.Int8{Int64: 1000, Valid: true}},
			Proposta: &prop, PreSpunta: true, Figli: []AllegatoUI{figlio},
			Documento: &db.Documento{DocumentoID: uuid.New(), StatoNas: db.StatoNasScritto, PathRelativo: `ELENCO DISEGNI\6674611A\6674611A_4.pdf`}},
		{Allegato: db.Allegato{AllegatoID: uuid.New(), MessaggioID: mid, Indice: 2, NomeFile: "logo.png", Natura: db.NaturaAllegatoInline}},
		{Allegato: db.Allegato{AllegatoID: uuid.New(), MessaggioID: mid, Indice: 3, NomeFile: "disegni.zip", Natura: db.NaturaAllegatoFile, Stato: db.StatoAllegatoGrezzo, Errore: txtT("in coda")},
			Proposta: &db.DocumentoProposta{PropostaID: uuid.New(), TipoProposto: db.TipoDocumentoAltro, Stato: db.StatoPropostaAperta, Confidenza: 20, Fonte: db.FontePropostaEstensione}, PreSpunta: true},
	}
	th := db.ThreadOfferta{ThreadID: tid, CartellaRelativa: txtT(`ACME\WIP\2026 09 08 Rossi RFQ 6674611A`), Oggetto: txtT("RFQ 6674611A"), DataInizio: time.Now(), Stato: db.StatoThreadAPERTA}
	copia := jobs.Copia{CasellaID: uuid.New(), EntryID: "E1", CasellaNome: "Commerciale", NonLetto: true, WorkerNome: "outlook@PC-FRANCESCO"}
	md := &messaggioDati{M: m, Riga: db.VInbox{MessaggioID: mid, TriageEsito: txtT("nuova_rfq"), TriageConfidenza: pgtype.Int2{Int16: 80, Valid: true}}, Copia: &copia,
		Presenze: []db.ListPresenzeRow{{MessaggioID: mid, EntryID: "E1", CasellaNome: "Commerciale", Cartella: txtT("Posta in arrivo"), NonLetto: true},
			{MessaggioID: mid, EntryID: "E2", CasellaNome: "Francesco", Cartella: txtT("Posta in arrivo")}},
		Allegati: allegati, Thread: &th, Avviso: "ok"}
	td := &triageDati{M: m, Riga: md.Riga, Azione: "nuova", Clienti: []db.Cliente{{ClienteID: uuid.New(), CartellaNas: "ACME", RagioneSociale: "Acme"}},
		Buyers: []db.Buyer{{BuyerID: uuid.New(), Cognome: "Rossi", Nome: txtT("Mario"), Email: txtT("mario.rossi@acme.example")}},
		Nome:   "Mario", Cognome: "Rossi", Email: "mario.rossi@acme.example", Dominio: "acme.example", Oggetto: "RFQ 6674611A", Allegati: allegati, Anteprima: `ACME\WIP\x`,
		Riferimento: "RDO 490020618",
		Proponibili: []db.CandidatoCodice{{MessaggioID: mid, Codice: "6674611A", Ruolo: db.RuoloCodiceProdotto,
			Origine: db.OrigineCodiceFamiglia, Famiglia: "7 cifre + lettera", Punteggio: 80, Evidenza: "oggetto"}},
		Altri: []db.CandidatoCodice{{MessaggioID: mid, Codice: "20260908", Ruolo: db.RuoloCodiceNonClassificato,
			Origine: db.OrigineCodiceGenerico, Punteggio: 30, Evidenza: "corpo"}},
		Candidati: []db.ListCandidatiAggancioRow{
			{MessaggioID: mid, ThreadID: tid, Regola: db.RegolaAggancioR0Reply, Punteggio: 98,
				Evidenza: "In-Reply-To punta a un messaggio agganciato", ThreadStato: db.StatoThreadAPERTA,
				Oggetto: txtT("RFQ 6674611A"), DataInizio: time.Now(), Cliente: "Acme"},
			{MessaggioID: mid, ThreadID: uuid.New(), Regola: db.RegolaAggancioR2Oggetto, Punteggio: 55,
				Evidenza: "stesso oggetto", ThreadStato: db.StatoThreadCHIUSA,
				Oggetto: txtT("RFQ 6674611A"), DataInizio: time.Now(), Cliente: "Acme"}}}
	thd := &threadDati{T: th, Riga: db.VCruscotto{Cliente: "ACME", NomeFase: db.NullFase{Fase: db.FaseRICEVUTA, Valid: true}, Semaforo: txtT("verde")},
		Identificativi: []db.IdentificativoThread{{Codice: "6674611A"}},
		Messaggi:       []messaggioThread{{M: m, Copia: &copia, Allegati: allegati}, {M: m, Motivo: "nessuna postazione associata alla sessione"}},
		Documenti:      []db.Documento{{Tipo: db.TipoDocumentoDisegno2d, Codice: txtT("6674611A"), PathRelativo: "x", StatoNas: db.StatoNasInCoda}},
		Fascicolo:      []db.VFascicolo{{Codice: "6674611A", TipoComponente: db.TipoComponenteSciolto, TipoDocumento: db.TipoDocumentoDisegno2d, Bloccante: true, Esito: "manca"}},
		Avviso:         "ok"}
	return md, td, thd
}

// datiIntegritaSintetici: una riga per ogni problema che ha un'azione diversa dalle altre.
func datiIntegritaSintetici() *integritaDati {
	riga := func(p db.ProblemaNas, stato db.StatoNas, trovato string) rigaIntegrita {
		return rigaIntegrita{db.ListAnomalieNasRow{
			NasAnomalium: db.NasAnomalium{
				DocumentoID: uuid.New(), ThreadID: uuid.New(), Problema: p, StatoDb: stato,
				Percorso:  `ACME\WIP\2026 09 17 prova\ELENCO DISEGNI\6674611A_4.pdf`,
				ShaAtteso: strings.Repeat("a", 64), ShaTrovato: txtT(trovato),
				Dettaglio: "dettaglio di prova", RilevataIl: time.Now(), VistaIl: time.Now(),
			},
			NomeFile: "6674611A_4.pdf", Codice: txtT("6674611A"), Rev: txtT("4"),
			Tipo: db.TipoDocumentoDisegno2d, Oggetto: txtT("RFQ 6674611A"), RagioneSociale: "Acme S.p.A.",
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
		{"inbox.html", "triage_form", td, []string{"Crea RFQ", "nuovo cliente", "6674611A_4.pdf", "checked"}},
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
