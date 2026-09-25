package web

// L1 — B8.7b: la BOM visuale, il piano in fondo, «Da verificare», «Rivedi», l'avanzamento con il suo poll,
// lo stato nell'indirizzo, i percorsi del NAS. Senza database: dati sintetici, template veri.

import (
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// Un prodotto senza STEP e' una card radice con la struttura da definire; lo STEP, quando arriva, disegna
// la sua proposta sotto quella card, tratteggiata, con le quantita' sugli archi; un nodo del file che e' gia'
// un componente della BOM e' un rimando con l'arco proposto; una radice del file che non pende da nessun
// componente sta in cima, tratteggiata.
func TestLaBomVisualeDisegnaIlProdottoELeProposteSottoDiLui(t *testing.T) {
	tid := uuid.New()
	prodotto := db.Componente{ComponenteID: uuid.New(), ThreadID: tid, Codice: "52922757", Tipo: db.TipoComponenteFinito, Qta: 1}
	altro := db.Componente{ComponenteID: uuid.New(), ThreadID: tid, Codice: "53017189", Tipo: db.TipoComponenteSciolto, Qta: 1}
	d := &fascicoloDati{T: db.ThreadOfferta{ThreadID: tid}, Base: "/thread/" + tid.String() + "/fascicolo",
		Albero: fascicolo.NuovoAlbero([]db.Componente{prodotto, altro}, nil), Componenti: map[uuid.UUID]db.Componente{prodotto.ComponenteID: prodotto, altro.ComponenteID: altro},
		Completezza:  map[uuid.UUID][]cella{prodotto.ComponenteID: {cellaDa(db.VFascicolo{TipoDocumento: db.TipoDocumentoCad3d, Bloccante: true, Esito: "manca"})}},
		StepProdotto: map[uuid.UUID]db.VStepProdotto{prodotto.ComponenteID: {ComponenteID: prodotto.ComponenteID, Esito: fascicolo.StepMancante}},
		DocumentiDi:  map[uuid.UUID][]db.Documento{}, Rimozioni: map[uuid.UUID][]rimozione{}}

	carte := costruisciBom(d)
	if len(carte) != 2 || carte[0].Comp == nil || carte[0].Comp.Codice != "52922757" {
		t.Fatalf("in cima il prodotto (i finiti per primi), poi l'altra radice: %+v", carte)
	}
	if carte[0].Struttura != "struttura da definire: STEP non ancora disponibile" || carte[0].Classe != "urg" {
		t.Errorf("il prodotto senza STEP: %q, classe %q", carte[0].Struttura, carte[0].Classe)
	}

	// arriva lo STEP: la radice ritrova il prodotto, 52920517 e' nuovo, 53017189 c'e' gia' nella BOM
	step := uuid.New()
	nodo := func(chiave, codice string, stato db.StatoProposta, comp *db.Componente) db.ListComponenteProposteThreadRow {
		n := db.ComponenteProposta{PropostaID: uuid.New(), AllegatoID: step, Chiave: chiave, Codice: txtT(codice), NomeGrezzo: codice, Stato: stato,
			TipoProposto: db.NullTipoComponente{TipoComponente: db.TipoComponenteSottoassieme, Valid: true}}
		if comp != nil {
			n.ComponenteID = uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}
		}
		return db.ListComponenteProposteThreadRow{ComponenteProposta: n, NomeFile: "52922757.stp"}
	}
	rel := func(padre, figlio string, qta int32) db.ListRelazioneProposteThreadRow {
		return db.ListRelazioneProposteThreadRow{RelazioneProposta: db.RelazioneProposta{AllegatoID: step, PadreChiave: padre, FiglioChiave: figlio,
			Qta: qta, Stato: db.StatoPropostaAperta}, NomeFile: "52922757.stp"}
	}
	d.NodiProposti = []db.ListComponenteProposteThreadRow{nodo("#1", "52922757", db.StatoPropostaDuplicato, &prodotto),
		nodo("#2", "52920517", db.StatoPropostaAperta, nil), nodo("#3", "53017189", db.StatoPropostaDuplicato, &altro),
		nodo("#9", "99000001", db.StatoPropostaAperta, nil)}
	d.RelazioniProposte = []db.ListRelazioneProposteThreadRow{rel("#1", "#2", 2), rel("#2", "#3", 4)}
	d.StepProdotto[prodotto.ComponenteID] = db.VStepProdotto{ComponenteID: prodotto.ComponenteID, Esito: fascicolo.StepDaConfermare}
	d.Piano = fascicolo.PianoFascicolo{File: []fascicolo.VoceFile{{Nome: "52920517.pdf", Stato: fascicolo.VocePronta,
		DaStep: &fascicolo.NodoInArrivo{Proposta: d.NodiProposti[1].ComponenteProposta.PropostaID, Codice: "52920517"}}}}

	carte = costruisciBom(d)
	p := carte[0]
	if p.Struttura != "1 modifica proposta dallo STEP 52922757.stp" || len(p.Figli) != 1 {
		t.Fatalf("lo STEP disegna la sua proposta sotto il prodotto: %q, figli %d", p.Struttura, len(p.Figli))
	}
	a := p.Figli[0]
	if !a.Proposta() || a.Qta != 2 || a.Codice() != "52920517" || a.InArrivo != 1 || a.ID != "proposta-"+step.String()+"-#2" {
		t.Errorf("il nodo nuovo: %+v", a)
	}
	if len(a.Figli) != 1 || a.Figli[0].Comp == nil || a.Figli[0].Comp.Codice != "53017189" || a.Figli[0].ArcoProposto == nil ||
		!a.Figli[0].Ripetuto || a.Figli[0].Qta != 4 {
		t.Errorf("sotto il nodo nuovo, il componente che c'e' gia', con l'arco proposto: %+v", a.Figli)
	}
	ultima := carte[len(carte)-1]
	if !ultima.Proposta() || !ultima.Radice || ultima.Codice() != "99000001" {
		t.Errorf("la radice del file che non pende da nessun componente sta in cima: %+v", ultima)
	}

	d.Carte = carte
	html := rendiParte(t, "fasc_tela", d)
	haTesto(t, "tela", html, `class="bom-li li-proposta"`, `class="bom-li li-arco-proposto"`, `class="arco"`, "×2", "×4",
		"+ proposto · assieme?", "1 file lo aspetta", "+ arco proposto dallo STEP 52922757.stp", "prodotto finito")
}

// Il piano in fondo: pronti, da verificare, in preparazione; la conferma porta la firma del piano e si spegne
// senza niente di pronto. Chi consulta vede i conti e non i comandi.
func TestIlPianoInFondoPortaLaFirmaDelPiano(t *testing.T) {
	s := fascicoloSintetico()
	comp := s.prodotto
	s.d.Piano = fascicolo.PianoFascicolo{File: []fascicolo.VoceFile{
		{Proposta: uuid.New(), Nome: "52922757.pdf", Tipo: db.TipoDocumentoDisegno2d, Codice: "52922757", Componente: &comp, Stato: fascicolo.VocePronta},
		{Proposta: uuid.New(), Nome: "anonimo.pdf", Tipo: db.TipoDocumentoDaDeterminare, Stato: fascicolo.VoceDecidere, Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaTipo, Testo: "che cos'è?"}}},
		{Proposta: uuid.New(), Nome: "in arrivo.stp", Stato: fascicolo.VoceAttesa}}}
	html := rendiParte(t, "fasc_piano", s.d)
	haTesto(t, "piano", html, "<b>1</b> pronto", "<b>2</b> da verificare", "<b>1</b> in preparazione",
		`name="firma" value="`+s.d.Piano.Firma()+`"`, `hx-post="/thread/`+s.d.T.ThreadID.String()+`/fascicolo/conferma"`, "Rivedi")
	senzaTesto(t, "piano", html, `id="conferma-fascicolo" disabled`)

	s.d.Piano.File = s.d.Piano.File[1:]
	html = rendiParte(t, "fasc_piano", s.d)
	if !strings.Contains(html, `id="conferma-fascicolo" disabled`) {
		t.Error("senza niente di pronto la conferma e' spenta")
	}
	s.d.Scrive = false
	senzaTesto(t, "consultazione", rendiParte(t, "fasc_piano", s.d), "Conferma Fascicolo", "Rivedi")
}

// «Da verificare» mostra solo le decisioni, ciascuna con il suo gesto; «Rivedi» le voci pronte, spuntate.
func TestDaVerificareERivediMostranoCiascunoLeSueVoci(t *testing.T) {
	s := fascicoloSintetico()
	comp := s.prodotto
	corrente := db.Documento{DocumentoID: uuid.New(), NomeFile: "52922757.pdf", Rev: txtT("A")}
	pTipo, pComp, pSost, pPronto := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	struttura := uuid.New()
	s.d.Piano = fascicolo.PianoFascicolo{
		File: []fascicolo.VoceFile{
			{Proposta: pTipo, Allegato: uuid.New(), Nome: "anonimo.pdf", Tipo: db.TipoDocumentoDaDeterminare, Suggerito: "53017189", Stato: fascicolo.VoceDecidere,
				Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaTipo, Testo: "che cos'è questo file?"}}},
			{Proposta: pComp, Allegato: uuid.New(), Nome: "53999999.pdf", Tipo: db.TipoDocumentoDisegno2d, Codice: "53999999", Stato: fascicolo.VoceDecidere,
				Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaComponente, Testo: "53999999 non è nella BOM"}}},
			{Proposta: pSost, Allegato: uuid.New(), Nome: "52922757_B.pdf", Tipo: db.TipoDocumentoDisegno2d, Codice: "52922757", Rev: "B", Componente: &comp,
				Correnti: []db.Documento{corrente}, Stato: fascicolo.VoceDecidere, Domande: []fascicolo.Domanda{{Chiave: fascicolo.DomandaSostituzione, Testo: "si aggiunge, o sostituisce quale?"}}},
			{Proposta: pPronto, Allegato: uuid.New(), Nome: "52920517.pdf", Tipo: db.TipoDocumentoDisegno2d, Codice: "52920517", Stato: fascicolo.VocePronta,
				DaStep: &fascicolo.NodoInArrivo{Allegato: struttura, File: "assieme.stp", Codice: "52920517"}},
		},
		Strutture: []fascicolo.VoceStruttura{{Allegato: struttura, Nome: "assieme.stp", Nodi: 2, Archi: 3, Nuovi: []string{"52920517", "53011111"}, Stato: fascicolo.VocePronta}},
	}
	s.d.Stato.Cassetto = "verifica"
	base := s.d.Base
	html := rendiParte(t, "fasc_cassetto", s.d)
	haTesto(t, "verifica", html, "anonimo.pdf", "che cos&#39;è questo file?", base+"/proposta/"+pTipo.String()+"/decidi", `value="53017189"`,
		"53999999.pdf", base+"/codice/aggiungi", "+ Particolare", base+"/assegna", "Conferma senza componente",
		"52922757_B.pdf", `name="scelta" required`, `value="`+corrente.DocumentoID.String()+`"`, "sostituisce 52922757.pdf",
		"Non si congela ancora"[:0])
	senzaTesto(t, "verifica", html, "52920517.pdf")

	s.d.Stato.Cassetto = "piano"
	html = rendiParte(t, "fasc_cassetto", s.d)
	haTesto(t, "rivedi", html, `name="struttura" value="`+struttura.String()+`" checked`, "nascono 52920517, 53011111",
		`name="voce" value="`+pPronto.String()+`" checked`, "nasce dallo STEP assieme.stp", "Conferma i selezionati")
	senzaTesto(t, "rivedi", html, `value="`+pTipo.String()+`"`, `value="`+pSost.String()+`"`)
}

// L'avanzamento: con del lavoro in corso l'elemento porta il poll, e l'attesa si allunga quando non cambia
// niente; senza lavoro niente poll.
func TestIlPollCeSoloFinchéCeLavoro(t *testing.T) {
	a := avanzamento{Base: "/thread/x/fascicolo", Lavoro: fascicolo.Lavoro{Download: 1, Analisi: 4}, Firma: "abc", Attesa: attesaDopo(0)}
	d := &fascicoloDati{Avanzamento: a}
	html := rendiParte(t, "fasc_avanzamento", d)
	haTesto(t, "con lavoro", html, `hx-get="/thread/x/fascicolo/avanzamento?firma=abc&amp;giro=0"`, `hx-trigger="every 2s"`, `hx-swap="outerHTML"`,
		"1 download · 4 analisi", "il Fascicolo si aggiorna da solo")
	d.Avanzamento.Lavoro = fascicolo.Lavoro{}
	senzaTesto(t, "senza lavoro", rendiParte(t, "fasc_avanzamento", d), "hx-get", "hx-trigger")
	if attesaDopo(0) != 2 || attesaDopo(5) != 5 || attesaDopo(100) != 10 {
		t.Errorf("attese: %d %d %d", attesaDopo(0), attesaDopo(5), attesaDopo(100))
	}
}

// Lo stato nell'indirizzo: i valori non validi cadono; scegliere un nodo toglie il nodo proposto, la scheda e
// il documento; scegliere una scheda toglie il documento.
func TestLoStatoNellIndirizzo(t *testing.T) {
	n, p, doc := uuid.New(), uuid.New(), uuid.New()
	st := leggiStatoFascicolo(url.Values{"nodo": {n.String()}, "prop": {p.String()}, "doc": {doc.String()}, "scheda": {"2d"},
		"vista": {"boh"}, "cassetto": {"nas"}, "q": {"  5292  "}, "nas": {"ACME/WIP"}})
	if st.Vista != "" || st.Cassetto != "nas" || st.Scheda != "2d" || st.Cerca != "5292" || st.Nas != "ACME/WIP" {
		t.Errorf("stato letto: %+v", st)
	}
	v, _ := url.ParseQuery(strings.TrimPrefix(st.Con("nodo", uuid.New().String()), "?"))
	if v.Get("prop") != "" || v.Get("doc") != "" || v.Get("scheda") != "" || v.Get("cassetto") != "nas" {
		t.Errorf("un nodo nuovo toglie proposta, documento e scheda: %v", v)
	}
	v, _ = url.ParseQuery(strings.TrimPrefix(st.Con("scheda", "3d"), "?"))
	if v.Get("doc") != "" || v.Get("scheda") != "3d" || v.Get("nodo") != n.String() {
		t.Errorf("una scheda nuova toglie il documento: %v", v)
	}
	if st.HaFile() != (st.File != uuid.Nil) {
		t.Error("HaFile")
	}
	if leggiStatoFascicolo(url.Values{"scheda": {"boh"}, "cassetto": {"boh"}}).Scheda != "" {
		t.Error("una scheda sconosciuta cade")
	}
}

// Il NAS: un percorso si ripulisce, relativo alla radice; «..» che esce, i percorsi assoluti e di unita' si
// rifiutano.
func TestIPercorsiDelNasStannoSottoLaRadice(t *testing.T) {
	for in, atteso := range map[string]string{"": ".", ".": ".", `ACME\WIP\2026`: "ACME/WIP/2026", "ACME/./WIP/": "ACME/WIP", "ACME/../BETA": "BETA"} {
		if got, err := percorsoNas(in); err != nil || got != atteso {
			t.Errorf("%q: %q %v, atteso %q", in, got, err, atteso)
		}
	}
	for _, in := range []string{"..", "../fuori", "ACME/../../fuori", `\\server\share`, "/assoluto", `C:\Windows`, "c:/x"} {
		if got, err := percorsoNas(in); err == nil {
			t.Errorf("%q: accettato come %q", in, got)
		}
	}
}

// La firma della schermata cambia quando cambia quello che si vede.
func TestLaFirmaDellaSchermata(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	prima := firmaDi(s.d)
	if prima != firmaDi(s.d) {
		t.Fatal("stessi dati, stessa firma")
	}
	s.d.File[0].A.Stato = db.StatoAllegatoInStaging
	if firmaDi(s.d) == prima {
		t.Error("un file che cambia stato cambia la firma")
	}
	s.d.File[0].A.Stato = db.StatoAllegatoAnalizzato
	s.d.Lavoro = fascicolo.Lavoro{Analisi: 1}
	if firmaDi(s.d) == prima {
		t.Error("il lavoro in corso cambia la firma")
	}
}

// Il corpo del pannello di destra porta la chiave di che cosa mostra. Lo stesso PDF ha la stessa chiave (un
// gesto non lo ricarica); il riepilogo di uno STEP la cambia quando cambiano gli stati dei nodi che propone.
// La risposta di un gesto rifa' il corpo solo con RifaiCorpo, e la pagina manda la chiave con ogni richiesta.
func TestLaChiaveDelCorpoDiceQuandoRifarlo(t *testing.T) {
	s := fascicoloSintetico()
	s.d.filtraFile()
	pdf := &anteprimaDati{F: s.d.File[0]}
	if k := chiaveCorpo(pdf); k != "pdf:"+s.pdf.String() {
		t.Errorf("il PDF in staging: %q", k)
	}
	mancante := &anteprimaDati{F: s.d.File[0]}
	mancante.F.FileMancante = true
	if k := chiaveCorpo(mancante); k != "pdf-mancante:"+s.pdf.String() {
		t.Errorf("il PDF che non e' piu' nello staging e' un altro corpo (niente iframe): %q", k)
	}
	if k := chiaveCorpo(nil); k != "vuoto" {
		t.Errorf("niente nel pannello: %q", k)
	}
	step := &anteprimaDati{F: s.d.File[1], Analisi: &riepilogoAnalisi{Versione: 3}, Proposte: []db.ComponenteProposta{s.nodo}}
	prima := chiaveCorpo(step)
	if !strings.HasPrefix(prima, "modello:"+s.step.String()+":") || chiaveCorpo(step) != prima {
		t.Errorf("il riepilogo dello STEP: %q", prima)
	}
	deciso := s.nodo
	deciso.Stato = db.StatoPropostaConfermata
	step.Proposte = []db.ComponenteProposta{deciso}
	if chiaveCorpo(step) == prima {
		t.Error("lo STEP con il nodo deciso mostra un altro stato: e' un altro corpo")
	}
	if chiaveCorpo(&anteprimaDati{F: s.d.File[1]}) == prima {
		t.Error("lo STEP senza analisi e' un altro corpo")
	}

	s.d.Anteprima, s.d.ChiaveCorpo = pdf, chiaveCorpo(pdf)
	html := rendiFascicolo(t, "fasc_parti", s.d)
	senzaTesto(t, "stesso corpo", html, "anteprima-corpo", "<iframe")
	s.d.RifaiCorpo = true
	html = rendiFascicolo(t, "fasc_parti", s.d)
	haTesto(t, "altro corpo", html, `<div id="anteprima-corpo" hx-swap-oob="innerHTML">`,
		`id="corpo-chiave" name="corpo_chiave" value="pdf:`+s.pdf.String()+`"`, `<iframe class="ant-pdf" id="anteprima-pdf"`)
	s.d.RifaiCorpo = false
	html = rendiFascicolo(t, "contenuto", s.d)
	haTesto(t, "la pagina", html, `hx-include="#corpo-chiave"`, `id="corpo-chiave" name="corpo_chiave" value="pdf:`+s.pdf.String()+`"`)
}
