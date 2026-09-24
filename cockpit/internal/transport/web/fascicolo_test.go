package web

// L1 — B8.7: la schermata del Fascicolo con dati sintetici, senza database. Gli stati che contano: la
// working libera con le proposte tratteggiate (nodi, archi, rimozioni) e i loro gesti; la BOM congelata
// che si legge e non offre gesti sulla working; D25c senza preselezione; l'anteprima di un PDF (il viewer
// del browser) e di uno STEP (quello che l'analisi ha letto, senza viewer 3D); la risposta fuori banda,
// che non tocca il corpo dell'anteprima.

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

type sinteticoFascicolo struct {
	d                              *fascicoloDati
	prodotto, assieme, particolare db.Componente
	pdf, step                      uuid.UUID // allegati
	nodo, nodoSenzaCodice          db.ComponenteProposta
}

func fascicoloSintetico() *sinteticoFascicolo {
	tid := uuid.New()
	s := &sinteticoFascicolo{}
	mk := func(codice string, tipo db.TipoComponente) db.Componente {
		return db.Componente{ComponenteID: uuid.New(), ThreadID: tid, Codice: codice, Tipo: tipo, Qta: 1}
	}
	s.prodotto, s.assieme, s.particolare = mk("52922757", db.TipoComponenteFinito), mk("52920517", db.TipoComponenteSottoassieme), mk("53017189", db.TipoComponenteSciolto)
	comp := []db.Componente{s.prodotto, s.assieme, s.particolare}
	rel := []db.ComponenteRelazione{{PadreID: s.prodotto.ComponenteID, FiglioID: s.assieme.ComponenteID, Qta: 2},
		{PadreID: s.assieme.ComponenteID, FiglioID: s.particolare.ComponenteID, Qta: 4}}
	d := &fascicoloDati{T: db.ThreadOfferta{ThreadID: tid, Oggetto: txtT("RFQ 52922757")}, Riga: db.VCruscotto{Cliente: "ACME"},
		Base: "/thread/" + tid.String() + "/fascicolo", Scrive: true, Caricamento: true,
		Fase: &db.FaseLog{NomeFase: db.FaseFATTIBILITA}, PuoCongelare: true,
		Albero: fascicolo.NuovoAlbero(comp, rel), Componenti: map[uuid.UUID]db.Componente{},
		Completezza: map[uuid.UUID][]cella{
			s.prodotto.ComponenteID: {cellaDa(db.VFascicolo{TipoDocumento: db.TipoDocumentoCad3d, Bloccante: true, Esito: "ok"}),
				cellaDa(db.VFascicolo{TipoDocumento: db.TipoDocumentoDisegno2d, Bloccante: true, Esito: "ok_in_coda"})},
			s.assieme.ComponenteID: {cellaDa(db.VFascicolo{TipoDocumento: db.TipoDocumentoDisegno2d, Bloccante: true, Esito: "manca"}),
				cellaDa(db.VFascicolo{TipoDocumento: db.TipoDocumentoSviluppoDxf, Esito: "manca"})},
			s.particolare.ComponenteID: {cellaDa(db.VFascicolo{TipoDocumento: db.TipoDocumentoDisegno2d, Bloccante: true, Esito: "derogato"})},
		},
		StepProdotto: map[uuid.UUID]db.VStepProdotto{s.prodotto.ComponenteID: {ComponenteID: s.prodotto.ComponenteID, Codice: "52922757", Esito: fascicolo.StepDaScegliere}},
		Inline:       map[uuid.UUID][]figlioProposto{}, Rimozioni: map[uuid.UUID][]rimozione{},
		Conteggi: map[string]int{}, DocumentiDi: map[uuid.UUID][]db.Documento{},
		Gate:       fascicolo.Gate{Problemi: []string{"2 requisiti bloccanti del fascicolo non soddisfatti né derogati"}},
		NBloccanti: 1}
	for _, c := range comp {
		d.Componenti[c.ComponenteID] = c
	}
	s.step, s.pdf = uuid.New(), uuid.New()
	s.nodo = db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: s.step, Chiave: "#3", Codice: txtT("53011111"), NomeGrezzo: "53011111",
		Stato: db.StatoPropostaAperta, TipoProposto: db.NullTipoComponente{TipoComponente: db.TipoComponenteSciolto, Valid: true}}
	s.nodoSenzaCodice = db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: s.step, Chiave: "#4", NomeGrezzo: "Part1", Stato: db.StatoPropostaAperta}
	padre := db.ComponenteProposta{AllegatoID: s.step, Chiave: "#2", Codice: txtT("52920517"), Stato: db.StatoPropostaDuplicato,
		ComponenteID: uuid.NullUUID{UUID: s.assieme.ComponenteID, Valid: true}}
	d.Inline[s.assieme.ComponenteID] = []figlioProposto{{R: db.RelazioneProposta{AllegatoID: s.step, PadreChiave: "#2", FiglioChiave: "#3", Qta: 2,
		Stato: db.StatoPropostaAperta}, File: "assieme.stp", Padre: padre, Figlio: s.nodo}}
	d.Rimozioni[s.prodotto.ComponenteID] = []rimozione{{R: db.RimozioneProposta{StepDocumentoID: uuid.New(), PadreID: s.prodotto.ComponenteID,
		FiglioID: s.assieme.ComponenteID, QtaWorking: 2, Stato: db.StatoPropostaAperta}, Figlio: s.assieme, Step: "52922757.stp"}}
	d.Blocchi = []bloccoFile{{Allegato: s.step, Nome: "assieme.stp", Aperte: 1, Nodi: []nodoProposto{{P: s.nodoSenzaCodice, Sotto: "53011111"}}}}
	d.NProposte = 3
	prop := db.DocumentoProposta{PropostaID: uuid.New(), AllegatoID: s.pdf, TipoProposto: db.TipoDocumentoDisegno2d, Codice: txtT("52920517"),
		Confidenza: 85, Fonte: db.FontePropostaCartiglio, Stato: db.StatoPropostaAperta, Dettagli: json.RawMessage(`{}`)}
	file := rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: s.pdf, NomeFile: "52920517.pdf", Estensione: txtT("pdf"), Stato: db.StatoAllegatoAnalizzato,
		PathStaging: txtT(`C:\staging\x.pdf`), DataEvento: time.Now(), MittenteNome: txtT("Mario Rossi")}, Proposta: &prop,
		Tipo: "disegno_2d", Codice: "52920517"}
	file.Stato, file.ClasseStato = statoFile(file)
	stepDoc := db.Documento{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.prodotto.ComponenteID, Valid: true}, Tipo: db.TipoDocumentoCad3d,
		Codice: txtT("52922757"), NomeFile: "52922757.stp", Estensione: "stp", StatoNas: db.StatoNasScritto, PathRelativo: `CAD\52922757\52922757_REV_ND.stp`}
	fs := rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: s.step, NomeFile: "assieme.stp", Estensione: txtT("stp"), Stato: db.StatoAllegatoAnalizzato,
		Sha256: txtT(strings.Repeat("c", 64)), DataEvento: time.Now()}, Doc: &stepDoc, Comp: &s.prodotto, Tipo: "cad_3d", Codice: "52922757"}
	fs.Stato, fs.ClasseStato = statoFile(fs)
	d.File = []rigaFile{file, fs}
	d.DocumentiDi[s.prodotto.ComponenteID] = []db.Documento{stepDoc}
	s.d = d
	return s
}

func rendiFascicolo(t *testing.T, nome string, d *fascicoloDati) string {
	t.Helper()
	var buf bytes.Buffer
	if err := serverTest(t).pagine["fascicolo.html"].ExecuteTemplate(&buf, nome, vista{Dati: d, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// rendiParte esegue un pezzo interno della schermata, che riceve i dati e non la vista.
func rendiParte(t *testing.T, nome string, d *fascicoloDati) string {
	t.Helper()
	var buf bytes.Buffer
	if err := serverTest(t).pagine["fascicolo.html"].ExecuteTemplate(&buf, nome, d); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func haTesto(t *testing.T, cosa, html string, attesi ...string) {
	t.Helper()
	for _, a := range attesi {
		if !strings.Contains(html, a) {
			t.Errorf("%s: manca %q", cosa, a)
		}
	}
}

func senzaTesto(t *testing.T, cosa, html string, vietati ...string) {
	t.Helper()
	for _, v := range vietati {
		if strings.Contains(html, v) {
			t.Errorf("%s: non doveva esserci %q", cosa, v)
		}
	}
}

// Working libera: l'albero pieno, le proposte tratteggiate con i loro gesti, la griglia, la barra.
func TestLaSchermataMostraLaBomConLeProposteTratteggiate(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	html := rendiFascicolo(t, "fasc_corpo", s.d)
	tid := s.d.T.ThreadID.String()
	haTesto(t, "struttura", html,
		`id="nodo-`+s.prodotto.ComponenteID.String()+`"`, `class="node proposal"`, "53011111", "STEP assieme.stp",
		"/fascicolo/nodo/"+s.nodo.PropostaID.String()+"/accetta", "Accetta il nodo", "con il sottoalbero",
		`class="node removal"`, "non è più nello STEP strutturale 52922757.stp", "Togli dalla BOM", "Tieni",
		"Dallo STEP «", "assieme.stp", "Accetta tutto il file", "/fascicolo/nodo/"+s.nodoSenzaCodice.PropostaID.String()+"/codice", "Scrivi il codice",
		`title="STEP prodotto finito: DA SCEGLIERE — quale STEP è la distinta"`)
	haTesto(t, "completezza", html, `class="cell ok"`, "3D ✓", "2D ✓°", "2D ✗", "DXF ○", "2D ≈", "Bloccanti: <b class=\"urg-t\">1</b>")
	haTesto(t, "testata", html, "Congela…", "Non si congela ancora", "2 requisiti bloccanti", "BOM working, mai congelata", "/thread/"+tid+"/fascicolo/rianalizza")
	haTesto(t, "documenti", html, "52920517.pdf", `name="proposta" value="`, "Scegli un nodo nella struttura", "Carica nuova versione interna", `hx-encoding="multipart/form-data"`)
	if !regexp.MustCompile(`<button type="submit" class="btn small primary" disabled>Congela la V1</button>`).MatchString(html) {
		t.Error("con il gate rosso il bottone del congelamento e' spento")
	}

	s.d.Stato.Vista = "griglia"
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "griglia", html, `<table class="griglia">`, "STEP prodotto", `title="2D: mancante, obbligatorio"`)
}

// BOM congelata: le righe e le proposte si leggono, i gesti sulla working no; il caricamento e le
// decisioni sui file restano. In ACCETTATA la revisione vuole la scelta, senza preselezione (D25c).
func TestConLaBomCongelataLaSchermataSiLeggeENonCambia(t *testing.T) {
	s := fascicoloSintetico()
	s.d.Bloccata, s.d.Gate = 1, fascicolo.Gate{}
	adesso := time.Now()
	s.d.Versioni = []db.VBomVersioni{{Numero: 1, Stato: db.StatoBomCongelata, Contesto: db.ContestoBomPreventivo, CongelataIl: &adesso, Motivo: "prima baseline"}}
	s.d.Ultima = &s.d.Versioni[0]
	s.d.Fase = &db.FaseLog{NomeFase: db.FaseACCETTATA}
	s.d.SceltaContesto, s.d.PuoAprire, s.d.PuoCongelare = true, true, false
	s.d.Stato.Nodo = s.particolare.ComponenteID
	s.d.Nodo = &schedaNodo{C: s.particolare}
	s.d.filtraFile()
	html := rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "congelata", html, "BOM congelata nella V1", "Apri una revisione…", "In ACCETTATA il tipo di revisione lo sceglie chi la apre",
		`name="contesto" value="preventivo" required>`, `name="contesto" value="tecnica" required>`, "Carica nuova versione interna",
		"53011111", "si decide aprendo una revisione", "BOM congelata: un file si assegna aprendo una revisione")
	if regexp.MustCompile(`name="contesto"[^>]*checked`).MatchString(html) {
		t.Error("D25c: nessuna preselezione")
	}
	senzaTesto(t, "congelata", html, "Accetta il nodo", "Togli dalla BOM", "Accetta tutto il file", "Congela…", "Assegna i selezionati",
		"/componente/"+s.particolare.ComponenteID.String()+"/modifica", "/componente/"+s.particolare.ComponenteID.String()+"/archivia")

	// in SCHEDA_COSTO il tipo lo dice la fase
	s.d.Fase, s.d.SceltaContesto, s.d.ContestoFisso = &db.FaseLog{NomeFase: db.FaseSCHEDACOSTO}, false, "preventivo"
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "SCHEDA_COSTO", html, "Sarà una revisione <b>preventivo</b>")
	senzaTesto(t, "SCHEDA_COSTO", html, `name="contesto"`)
}

// L'anteprima: il PDF nel viewer del browser; uno STEP con quello che l'analisi ha letto, niente viewer.
func TestLAnteprimaDiUnPdfEDiUnoStep(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	s.d.Anteprima = &anteprimaDati{F: s.d.File[0]}
	html := rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "pdf", html, `<iframe class="ant-pdf" id="anteprima-pdf" src="/allegato/`+s.pdf.String()+`/anteprima"`, "Conferma…",
		`name="ritorna_thread" value="`+s.d.T.ThreadID.String()+`"`, "/proposta/"+s.d.File[0].Proposta.PropostaID.String()+"/conferma")

	st := &worker.StrutturaSTEP{Versione: 3, Schema: "AP214", Radici: []string{"#1"},
		Nodi:      []worker.NodoSTEP{{Chiave: "#1", IDGrezzo: "52922757"}, {Chiave: "#2", IDGrezzo: "52920517"}},
		Relazioni: []worker.RelazioneSTEP{{Padre: "#1", Figlio: "#2", Qta: 2}}, Avvisi: []string{"PRODUCT #9 senza definizione"},
		Scarti: &worker.ScartiSTEP{ProdottiSenzaDefinizione: 1}}
	s.d.Anteprima = &anteprimaDati{F: s.d.File[1], Analisi: &riepilogoAnalisi{Versione: 3, Struttura: st, MotivoParziale: "1 PRODUCT senza definizione",
		Nomi: map[string]string{"#1": "52922757", "#2": "52920517"}}, Proposte: []db.ComponenteProposta{s.nodo}}
	html = rendiFascicolo(t, "fasc_corpo", s.d)
	haTesto(t, "step", html, "versione 3", "schema AP214", "2 nodi", "1 relazioni", "letta in parte", "1 PRODUCT senza definizione",
		"<td class=\"mono\">52922757</td><td class=\"mono\">52920517</td><td>2</td>", "PRODUCT #9 senza definizione", "Nodi proposti da questo file",
		"Nessun viewer 3D", "Usa come STEP strutturale di 52922757")
	senzaTesto(t, "step", html, "<iframe")
}

// La risposta di un gesto: l'avviso e i pannelli fuori banda, mai il corpo dell'anteprima.
func TestLaRispostaFuoriBandaNonToccaLAnteprima(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	s.d.Anteprima = &anteprimaDati{F: s.d.File[0]}
	s.d.Avviso = "Niente è cambiato: prova"
	html := rendiFascicolo(t, "fasc_parti", s.d)
	for _, id := range []string{"fasc-testata", "struttura", "documenti", "completezza", "cassetto", "anteprima-testa"} {
		haTesto(t, "fuori banda", html, `id="`+id+`" hx-swap-oob="innerHTML"`)
	}
	senzaTesto(t, "fuori banda", html, "anteprima-corpo", "<iframe")
	if !strings.HasPrefix(strings.TrimSpace(html), `<div class="avviso-f no" role="status">Niente è cambiato: prova</div>`) {
		t.Errorf("l'avviso va per primo, nel suo posto, con il colore del rifiuto:\n%.200s", html)
	}
}

// Il cassetto: i codici della richiesta (i frammenti di B8.6, con il bersaglio della schermata) e gli avvisi.
func TestIlCassettoDeiCodiciEDegliAvvisi(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	s.d.Stato.Cassetto = "avvisi"
	s.d.Avvisi = []avvisoFascicolo{{"urg", "Per congelare: 2 requisiti bloccanti"}}
	html := rendiParte(t, "fasc_cassetto", s.d)
	haTesto(t, "avvisi", html, `class="cassetto-dentro"`, `class="avviso-riga urg"`, "Per congelare: 2 requisiti bloccanti")

	s.d.Stato.Cassetto = "codici"
	s.d.Codici = fascicolo.Unisci([]db.ListCodiciCandidatiThreadRow{{Codice: "20260908", Sorgente: "messaggio", Origine: "generico", Evidenza: "corpo",
		Punteggio: 30, MessaggioID: uuid.New()}}, fascicolo.ContestoCodici{})
	html = rendiParte(t, "fasc_cassetto", s.d)
	haTesto(t, "codici", html, "Codici della richiesta", `id="codice-20260908"`, `hx-target="#fasc-avviso"`, "+ Prodotto")
	senzaTesto(t, "codici", html, `hx-target="#thread"`)
	_ = pgtype.Text{}
}
