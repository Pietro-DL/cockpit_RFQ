package fascicolo

// L1 — B8.7b: il piano di riconciliazione come regola pura. Che cosa e' pronto (tipo, codice e componente
// determinati senza conflitti), che cosa aspetta una persona (con la domanda giusta), che cosa aspetta il
// lavoro dei worker; la struttura degli STEP; lo STEP strutturale; la firma. Le prove con il database e con
// la conferma stanno in piano_db_test.go e nel web.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

type pianoProva struct {
	in     IngressoPiano
	thread uuid.UUID
	comp   map[string]db.Componente
	step   uuid.UUID // lo STEP delle proposte di struttura
}

func testoP(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func nuovoPiano() *pianoProva {
	p := &pianoProva{thread: uuid.New(), comp: map[string]db.Componente{}, step: uuid.New()}
	p.in.NomiFile = map[uuid.UUID]string{p.step: "77722757.stp"}
	return p
}

func (p *pianoProva) componente(codice string, tipo db.TipoComponente) db.Componente {
	c := db.Componente{ComponenteID: uuid.New(), ThreadID: p.thread, Codice: codice, Tipo: tipo, Qta: 1}
	p.comp[codice] = c
	p.in.Componenti = append(p.in.Componenti, c)
	return c
}

// file aggiunge un file aperto, gia' analizzato e nello staging.
func (p *pianoProva) file(nome string, tipo db.TipoDocumento, codice, rev string) *FileAperto {
	a := uuid.New()
	ext := ""
	if i := strings.LastIndex(nome, "."); i >= 0 {
		ext = nome[i+1:]
	}
	f := FileAperto{
		Allegato: db.ListAllegatiFascicoloRow{AllegatoID: a, NomeFile: nome, Estensione: testoP(ext), Stato: db.StatoAllegatoAnalizzato,
			Sha256: testoP(strings.Repeat(string(rune('a'+len(p.in.File)%6)), 60) + nome[:1] + "000"), PathStaging: testoP(`C:\staging\` + nome), Origine: db.OrigineAllegatoOutlook},
		Proposta: db.DocumentoProposta{PropostaID: uuid.New(), AllegatoID: a, TipoProposto: tipo, Codice: testoP(codice), Rev: testoP(rev),
			Confidenza: 95, Fonte: db.FontePropostaCartiglio, Stato: db.StatoPropostaAperta, Dettagli: json.RawMessage(`{}`)},
		Presente: true,
	}
	p.in.File = append(p.in.File, f)
	return &p.in.File[len(p.in.File)-1]
}

// nodo aggiunge un nodo proposto dallo STEP.
func (p *pianoProva) nodo(chiave, codice string, stato db.StatoProposta, comp *db.Componente) db.ComponenteProposta {
	n := db.ComponenteProposta{PropostaID: uuid.New(), ThreadID: p.thread, AllegatoID: p.step, Chiave: chiave, NomeGrezzo: codice,
		Codice: testoP(codice), Stato: stato}
	if comp != nil {
		n.ComponenteID = uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}
	}
	p.in.Nodi = append(p.in.Nodi, n)
	return n
}

func (p *pianoProva) relazione(padre, figlio string, qta int32) {
	p.in.Relazioni = append(p.in.Relazioni, db.RelazioneProposta{ThreadID: p.thread, AllegatoID: p.step, PadreChiave: padre,
		FiglioChiave: figlio, Qta: qta, Stato: db.StatoPropostaAperta})
}

func (p *pianoProva) documento(comp db.Componente, nome string, tipo db.TipoDocumento, sha string) db.Documento {
	d := db.Documento{DocumentoID: uuid.New(), ThreadID: p.thread, ComponenteID: uuid.NullUUID{UUID: comp.ComponenteID, Valid: true},
		Tipo: tipo, Codice: testoP(comp.Codice), NomeFile: nome, Estensione: nome[strings.LastIndex(nome, ".")+1:], Sha256: sha,
		ConfermatoIl: time.Now()}
	p.in.Documenti = append(p.in.Documenti, d)
	return d
}

func vocePer(t *testing.T, pf PianoFascicolo, nome string) VoceFile {
	t.Helper()
	for _, v := range pf.File {
		if v.Nome == nome {
			return v
		}
	}
	t.Fatalf("%s non e' nel piano", nome)
	return VoceFile{}
}

func deveEssere(t *testing.T, v VoceFile, stato StatoVoce, domanda string) {
	t.Helper()
	if v.Stato != stato {
		t.Errorf("%s: stato %s, atteso %s (domande %v)", v.Nome, v.Stato, stato, v.Domande)
		return
	}
	if domanda != "" && (len(v.Domande) == 0 || v.Domande[0].Chiave != domanda) {
		t.Errorf("%s: domanda %v, attesa %s", v.Nome, v.Domande, domanda)
	}
}

// Il caso dello zip: il prodotto nasce dal codice della richiesta, lo STEP propone la struttura sotto di lui,
// i disegni vanno ai loro componenti, il capitolato e' della RFQ, il foglio senza codice e' una domanda. Lo
// STEP strutturale e' uno solo. Fascicolo v3: la struttura proposta non entra con «Conferma Fascicolo», si
// rivede e si conferma nell'editor; il disegno di un componente che nasce da lei la aspetta.
//
// Riscritta per lo Smistamento (F5b, P26, U7): prima fissava lo STEP strutturale unico come voce pronta, fra le
// pronte e nella firma della conferma. Adesso e' una voce da decidere (l'anteprima e la casella mai spuntata),
// non e' fra le pronte e non e' nella firma.
func TestIlPianoDelloZipMetteProntoQuelloCheIlServerSaGia(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("77722757", db.TipoComponenteFinito)
	p.nodo("#100", "77722757", db.StatoPropostaDuplicato, &prodotto)
	p.nodo("#110", "77720517", db.StatoPropostaAperta, nil)
	p.nodo("#120", "77811111", db.StatoPropostaAperta, nil)
	p.relazione("#100", "#110", 2)
	p.relazione("#110", "#120", 2)
	p.file("77722757.stp", db.TipoDocumentoCad3d, "77722757", "")
	p.file("77722757.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	p.file("77720517.pdf", db.TipoDocumentoDisegno2d, "77720517", "")
	p.file("Capitolato fornitura.pdf", db.TipoDocumentoCapitolato, "", "")
	p.file("77817189 foglio 2.pdf", db.TipoDocumentoDisegno2d, "", "")
	zip := p.file("RFQ ACME.zip", db.TipoDocumentoAltro, "", "")
	zip.Proposta.Fonte = db.FontePropostaEstensione

	pf := PianoDelFascicolo(p.in)
	if len(pf.Strutture) != 1 || pf.Strutture[0].Stato != VoceDecidere || pf.Strutture[0].Nodi != 2 || pf.Strutture[0].Archi != 2 ||
		len(pf.Strutture[0].Domande) != 1 || pf.Strutture[0].Domande[0].Chiave != DomandaStrutturaEditor {
		t.Fatalf("struttura dello STEP: %+v", pf.Strutture)
	}
	stp := vocePer(t, pf, "77722757.stp")
	deveEssere(t, stp, VocePronta, "")
	if stp.Componente == nil || stp.Componente.Codice != "77722757" {
		t.Errorf("lo STEP va al prodotto: %+v", stp.Componente)
	}
	deveEssere(t, vocePer(t, pf, "77722757.pdf"), VocePronta, "")
	assieme := vocePer(t, pf, "77720517.pdf")
	deveEssere(t, assieme, VoceDecidere, DomandaComponente)
	if assieme.DaStep == nil || assieme.DaStep.Codice != "77720517" || assieme.Componente != nil || !strings.Contains(assieme.Domande[0].Testo, "editor") {
		t.Errorf("77720517.pdf aspetta la struttura che si conferma nell'editor, e sa a quale nodo va: %+v %+v", assieme.DaStep, assieme.Domande)
	}
	cap := vocePer(t, pf, "Capitolato fornitura.pdf")
	deveEssere(t, cap, VocePronta, "")
	if cap.Destinazione() != "" {
		t.Errorf("il capitolato e' della RFQ, non di un componente: %s", cap.Destinazione())
	}
	foglio := vocePer(t, pf, "77817189 foglio 2.pdf")
	deveEssere(t, foglio, VoceDecidere, DomandaCodice)
	if foglio.Suggerito != "77817189" {
		t.Errorf("il codice suggerito viene dal nome: %q", foglio.Suggerito)
	}
	for _, v := range pf.File {
		if v.Nome == "RFQ ACME.zip" {
			t.Error("lo zip e' un contenitore: non entra nel piano")
		}
	}
	if len(pf.Strutturali) != 1 || pf.Strutturali[0].Stato != VoceDecidere || pf.Strutturali[0].Nome != "77722757.stp" ||
		len(pf.Strutturali[0].Domande) != 1 || pf.Strutturali[0].Domande[0].Chiave != DomandaStrutturale ||
		!strings.Contains(pf.Strutturali[0].Domande[0].Testo, "anteprima") {
		t.Fatalf("lo STEP strutturale unico si decide con l'anteprima, non entra con la conferma: %+v", pf.Strutturali)
	}
	if pf.Pronte() != 3 || pf.FilePronti() != 3 || pf.Decisioni() != 4 || pf.InAttesa() != 0 {
		t.Errorf("conti del piano: pronte %d, file %d, decisioni %d, attesa %d", pf.Pronte(), pf.FilePronti(), pf.Decisioni(), pf.InAttesa())
	}
	if a, b := pf.Firma(), PianoDelFascicolo(p.in).Firma(); a != b || a == "" {
		t.Errorf("stessi dati, stessa firma: %q %q", a, b)
	}
	if n := pf.DaVerificare()[prodotto.ComponenteID]; n != 1 {
		t.Errorf("lo STEP da autorizzare e' una cosa da verificare del prodotto: %d", n)
	}
}

// Un file che non e' ancora sceso, o il cui lavoro e' in corso, aspetta; uno il cui download e' fallito, o
// sparito dalla cache, chiede di riscaricarlo.
func TestIlLavoroInCorsoAspettaIGuastiSiDicono(t *testing.T) {
	p := nuovoPiano()
	p.componente("77722757", db.TipoComponenteFinito)
	p.file("in lavoro.pdf", db.TipoDocumentoDaDeterminare, "", "").InLavoro = true
	g := p.file("grezzo.pdf", db.TipoDocumentoDaDeterminare, "", "")
	g.Allegato.Stato, g.Presente = db.StatoAllegatoGrezzo, false
	e := p.file("errore.pdf", db.TipoDocumentoDaDeterminare, "", "")
	e.Allegato.Stato, e.Allegato.Errore, e.Presente = db.StatoAllegatoErrore, testoP("elemento non trovato"), false
	p.file("sparito.pdf", db.TipoDocumentoDisegno2d, "77722757", "").Presente = false

	pf := PianoDelFascicolo(p.in)
	deveEssere(t, vocePer(t, pf, "in lavoro.pdf"), VoceAttesa, "")
	deveEssere(t, vocePer(t, pf, "grezzo.pdf"), VoceAttesa, "")
	deveEssere(t, vocePer(t, pf, "errore.pdf"), VoceDecidere, DomandaFile)
	deveEssere(t, vocePer(t, pf, "sparito.pdf"), VoceDecidere, DomandaFile)
	if !strings.Contains(vocePer(t, pf, "errore.pdf").Domande[0].Testo, "elemento non trovato") {
		t.Error("il motivo del download fallito si legge")
	}
	if pf.InAttesa() != 2 || pf.Pronte() != 0 {
		t.Errorf("attesa %d, pronte %d", pf.InAttesa(), pf.Pronte())
	}
}

// Le ambiguita' vere: che cos'e' il file, il codice che non e' nella BOM, il componente archiviato, la
// revisione diversa da quella del componente, un documento corrente dello stesso tipo, il codice diverso da
// quello del componente a cui il file e' assegnato.
func TestLeAmbiguitaVereChiedonoUnaPersona(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("77722757", db.TipoComponenteFinito)
	assieme := p.componente("77720517", db.TipoComponenteSottoassieme)
	assieme.Rev = testoP("A")
	p.in.Componenti[1] = assieme
	archiviato := p.componente("77800001", db.TipoComponenteSciolto)
	adesso := time.Now()
	archiviato.ArchiviatoIl = &adesso
	p.in.Componenti[2] = archiviato
	p.documento(prodotto, "77722757.pdf", db.TipoDocumentoDisegno2d, strings.Repeat("f", 64))

	p.file("anonimo.pdf", db.TipoDocumentoDaDeterminare, "", "")
	altro := p.file("boh.xyz", db.TipoDocumentoAltro, "", "")
	altro.Proposta.Fonte = db.FontePropostaEstensione
	p.file("53999999.pdf", db.TipoDocumentoDisegno2d, "53999999", "")
	p.file("77800001.pdf", db.TipoDocumentoDisegno2d, "77800001", "")
	p.file("77720517_B.pdf", db.TipoDocumentoDisegno2d, "77720517", "B")
	p.file("77722757_nuovo.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	diverso := p.file("77799999.dxf", db.TipoDocumentoSviluppoDxf, "77722757", "")
	diverso.Proposta.ComponenteID = uuid.NullUUID{UUID: prodotto.ComponenteID, Valid: true}
	diverso.Proposta.Dettagli = json.RawMessage(`{"codice_letto": "77799999"}`)

	pf := PianoDelFascicolo(p.in)
	deveEssere(t, vocePer(t, pf, "anonimo.pdf"), VoceDecidere, DomandaTipo)
	deveEssere(t, vocePer(t, pf, "boh.xyz"), VoceDecidere, DomandaTipo)
	deveEssere(t, vocePer(t, pf, "53999999.pdf"), VoceDecidere, DomandaComponente)
	deveEssere(t, vocePer(t, pf, "77800001.pdf"), VoceDecidere, DomandaComponente)
	deveEssere(t, vocePer(t, pf, "77720517_B.pdf"), VoceDecidere, DomandaRevisione)
	nuovo := vocePer(t, pf, "77722757_nuovo.pdf")
	deveEssere(t, nuovo, VoceDecidere, DomandaSostituzione)
	if len(nuovo.Correnti) != 1 || nuovo.Correnti[0].NomeFile != "77722757.pdf" {
		t.Errorf("la domanda aggiungi/sostituisce nomina il corrente: %+v", nuovo.Correnti)
	}
	deveEssere(t, vocePer(t, pf, "77799999.dxf"), VoceDecidere, DomandaCodiceDiverso)
	if pf.Pronte() != 0 || pf.Decisioni() != 7 {
		t.Errorf("pronte %d, decisioni %d", pf.Pronte(), pf.Decisioni())
	}
	if n := pf.DaVerificare()[prodotto.ComponenteID]; n != 2 {
		t.Errorf("le decisioni del prodotto sulla sua card: %d, attese 2", n)
	}
}

// I fogli dello stesso disegno arrivati insieme entrano insieme e si aggiungono; con revisioni diverse (anche
// «nessuna» contro «B») uno potrebbe sostituire l'altro, e lo dice una persona.
func TestIFogliArrivatiInsiemeSiAggiungonoLeRevisioniDiverseNo(t *testing.T) {
	p := nuovoPiano()
	p.componente("77722757", db.TipoComponenteFinito)
	p.componente("77817189", db.TipoComponenteSciolto)
	p.file("77722757 foglio 1.pdf", db.TipoDocumentoDisegno2d, "77722757", "B")
	p.file("77722757 foglio 2.pdf", db.TipoDocumentoDisegno2d, "77722757", "B")
	p.file("77817189.pdf", db.TipoDocumentoDisegno2d, "77817189", "")
	p.file("77817189_B.pdf", db.TipoDocumentoDisegno2d, "77817189", "B")

	pf := PianoDelFascicolo(p.in)
	for _, n := range []string{"77722757 foglio 1.pdf", "77722757 foglio 2.pdf"} {
		v := vocePer(t, pf, n)
		deveEssere(t, v, VocePronta, "")
		if !v.Aggiunge {
			t.Errorf("%s: i fogli si aggiungono", n)
		}
	}
	for _, n := range []string{"77817189.pdf", "77817189_B.pdf"} {
		deveEssere(t, vocePer(t, pf, n), VoceDecidere, DomandaRevisione)
	}
}

// Lo stesso contenuto gia' documento e' una provenienza; identico a una revisione sostituita e' un rifiuto
// da spiegare; due file uguali nello stesso piano: il secondo e' una provenienza del primo.
func TestLoStessoContenutoNonDiventaUnSecondoDocumento(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("77722757", db.TipoComponenteFinito)
	gia := p.file("77722757 copia.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	p.documento(prodotto, "77722757.pdf", db.TipoDocumentoDisegno2d, gia.Allegato.Sha256.String)
	vecchio := p.file("77722757 vecchio.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	d := p.documento(prodotto, "77722757_A.pdf", db.TipoDocumentoDisegno2d, vecchio.Allegato.Sha256.String)
	p.in.Documenti[len(p.in.Documenti)-1].SostituitoDa = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	_ = d
	uno := p.file("capitolato.pdf", db.TipoDocumentoCapitolato, "", "")
	due := p.file("capitolato (1).pdf", db.TipoDocumentoCapitolato, "", "")
	due.Allegato.Sha256 = uno.Allegato.Sha256

	pf := PianoDelFascicolo(p.in)
	v := vocePer(t, pf, "77722757 copia.pdf")
	deveEssere(t, v, VocePronta, "")
	if v.Duplicato != "77722757.pdf" {
		t.Errorf("provenienza di 77722757.pdf: %q", v.Duplicato)
	}
	deveEssere(t, vocePer(t, pf, "77722757 vecchio.pdf"), VoceDecidere, DomandaSostituzione)
	if v := vocePer(t, pf, "capitolato (1).pdf"); v.Duplicato != "capitolato.pdf" {
		t.Errorf("il secondo file uguale e' una provenienza del primo: %q", v.Duplicato)
	}
}

// La struttura di uno STEP non e' pronta se ha un nodo senza codice, un componente archiviato da ripristinare
// o una quantita' diversa; un file che va a un nodo di quella struttura aspetta con lei. Gli archi verso un
// nodo scartato si decidono uno per uno, e non fermano il resto.
func TestLaStrutturaConUnNodoSenzaCodiceAspettaUnaPersona(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("77722757", db.TipoComponenteFinito)
	p.nodo("#100", "77722757", db.StatoPropostaDuplicato, &prodotto)
	p.nodo("#110", "77720517", db.StatoPropostaAperta, nil)
	p.nodo("#120", "", db.StatoPropostaAperta, nil)
	p.relazione("#100", "#110", 2)
	p.relazione("#110", "#120", 1)
	p.file("77720517.pdf", db.TipoDocumentoDisegno2d, "77720517", "")

	pf := PianoDelFascicolo(p.in)
	if pf.Strutture[0].Stato != VoceDecidere || !strings.Contains(pf.Strutture[0].Domande[0].Testo, "senza codice") {
		t.Fatalf("struttura con un nodo senza codice: %+v", pf.Strutture[0])
	}
	deveEssere(t, vocePer(t, pf, "77720517.pdf"), VoceDecidere, DomandaComponente)

	// il nodo senza codice si scarta: la struttura non ha piu' domande bloccanti ma si conferma nell'editor
	// (Fascicolo v3), e l'arco verso lo scartato e' un «morto»
	p.in.Nodi[2].Stato = db.StatoPropostaScartata
	pf = PianoDelFascicolo(p.in)
	s := pf.Strutture[0]
	if s.Stato != VoceDecidere || s.Morti != 1 || s.Nodi != 1 || s.Archi != 1 {
		t.Fatalf("dopo lo scarto: %+v", s)
	}
	editor := false
	for _, d := range s.Domande {
		editor = editor || d.Chiave == DomandaStrutturaEditor
		if strings.Contains(d.Testo, "senza codice") {
			t.Errorf("dopo lo scarto non c'e' piu' un nodo senza codice: %+v", s.Domande)
		}
	}
	if !editor {
		t.Errorf("la struttura si conferma nell'editor: %+v", s.Domande)
	}
	deveEssere(t, vocePer(t, pf, "77720517.pdf"), VoceDecidere, DomandaComponente)
	if pf.Decisioni() != 2 {
		t.Errorf("la struttura (con l'arco verso lo scartato) e il file che la aspetta: %d", pf.Decisioni())
	}

	// una quantita' diversa dalla BOM e' una decisione
	p.in.Relazioni[0].Nota = testoP("qta diversa: 3 contro 2")
	if pf = PianoDelFascicolo(p.in); pf.Strutture[0].Stato != VoceDecidere {
		t.Errorf("con una quantita' diversa la struttura aspetta: %+v", pf.Strutture[0])
	}
}

// Con la BOM congelata non c'e' struttura da confermare, i file entrano senza componente, e uno gia'
// assegnato si sgancia prima.
func TestConLaBomCongelataIFileEntranoSenzaComponente(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("77722757", db.TipoComponenteFinito)
	p.nodo("#100", "77722757", db.StatoPropostaDuplicato, &prodotto)
	p.nodo("#110", "77720517", db.StatoPropostaAperta, nil)
	p.relazione("#100", "#110", 2)
	p.file("77722757.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	assegnato := p.file("77722757 foglio 2.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	assegnato.Proposta.ComponenteID = uuid.NullUUID{UUID: prodotto.ComponenteID, Valid: true}
	p.in.Bloccata = 1

	pf := PianoDelFascicolo(p.in)
	if len(pf.Strutture) != 0 || len(pf.Strutturali) != 0 {
		t.Errorf("con la BOM congelata niente struttura e niente STEP strutturale: %+v %+v", pf.Strutture, pf.Strutturali)
	}
	v := vocePer(t, pf, "77722757.pdf")
	deveEssere(t, v, VocePronta, "")
	if v.Componente != nil || v.DaStep != nil {
		t.Errorf("entra senza componente: %+v", v)
	}
	deveEssere(t, vocePer(t, pf, "77722757 foglio 2.pdf"), VoceDecidere, DomandaCongelata)
}

// Lo STEP strutturale del prodotto e' una voce da decidere anche quando e' uno solo; con due la domanda li
// nomina; con quello gia' fissato non si propone niente.
//
// Riscritta per lo Smistamento (F5b, P26, U7; A5.4.7 supera A4.4): prima fissava che con uno STEP solo la voce
// nascesse pronta («gia' scelto», D31) e la conferma del piano lo fissasse. Adesso nessuno STEP e' preselezionato.
func TestLoStepStrutturaleSiSuggerisceSoloSeEUnoSolo(t *testing.T) {
	p := nuovoPiano()
	uno := p.componente("77722757", db.TipoComponenteFinito)
	due := p.componente("77722758", db.TipoComponenteFinito)
	fissato := p.componente("77722759", db.TipoComponenteFinito)
	fissato.StepStrutturaleID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	p.in.Componenti[2] = fissato
	p.documento(uno, "77722757.stp", db.TipoDocumentoCad3d, strings.Repeat("1", 64))
	p.documento(due, "77722758.stp", db.TipoDocumentoCad3d, strings.Repeat("2", 64))
	p.file("77722758_B.stp", db.TipoDocumentoCad3d, "77722758", "B")

	pf := PianoDelFascicolo(p.in)
	per := map[string]VoceStrutturale{}
	for _, v := range pf.Strutturali {
		per[v.Prodotto.Codice] = v
	}
	if v := per["77722757"]; v.Stato != VoceDecidere || !v.Documento.Valid || v.Nome != "77722757.stp" ||
		len(v.Domande) != 1 || v.Domande[0].Chiave != DomandaStrutturale || !strings.Contains(v.Domande[0].Testo, "nessuno STEP è scelto da solo") {
		t.Errorf("uno STEP solo, gia' confermato: si decide con l'anteprima: %+v", v)
	}
	if pf.Pronte() != 0 {
		t.Errorf("nessuno STEP strutturale fra le pronte: %d", pf.Pronte())
	}
	// 77722758 ha gia' uno STEP corrente: il file nuovo chiede aggiungi o sostituisce, e finche' non si
	// decide non e' fra i candidati. Lo STEP strutturale suggerito resta quello corrente.
	deveEssere(t, vocePer(t, pf, "77722758_B.stp"), VoceDecidere, DomandaSostituzione)
	if v := per["77722758"]; v.Stato != VoceDecidere || v.Nome != "77722758.stp" {
		t.Errorf("il corrente resta il candidato, da decidere: %+v", v)
	}
	if _, c := per["77722759"]; c {
		t.Error("un prodotto con lo STEP strutturale fissato non ne riceve un altro")
	}

	// due STEP correnti: si sceglie
	p.documento(uno, "77722757 bis.stp", db.TipoDocumentoCad3d, strings.Repeat("3", 64))
	pf = PianoDelFascicolo(p.in)
	for _, v := range pf.Strutturali {
		if v.Prodotto.Codice == "77722757" && (v.Stato != VoceDecidere || v.Domande[0].Chiave != DomandaStrutturale) {
			t.Errorf("due STEP: si sceglie: %+v", v)
		}
	}
}

// La firma cambia quando cambia una voce pronta: chi conferma deve aver visto il piano che conferma.
func TestLaFirmaCambiaConIlPiano(t *testing.T) {
	p := nuovoPiano()
	p.componente("77722757", db.TipoComponenteFinito)
	p.file("77722757.pdf", db.TipoDocumentoDisegno2d, "77722757", "")
	prima := PianoDelFascicolo(p.in).Firma()
	p.file("capitolato.pdf", db.TipoDocumentoCapitolato, "", "")
	if dopo := PianoDelFascicolo(p.in).Firma(); dopo == prima {
		t.Error("un file pronto in piu' cambia la firma")
	}
	prima = PianoDelFascicolo(p.in).Firma()
	p.file("attesa.pdf", db.TipoDocumentoDaDeterminare, "", "").InLavoro = true
	if dopo := PianoDelFascicolo(p.in).Firma(); dopo != prima {
		t.Error("un file in attesa non cambia la firma: non entra nella conferma")
	}
}

// I suffissi decorativi del cliente nel piano (Fascicolo v3): il file «X» trova il pezzo nato «X_PRT» e chiede di
// assegnarlo a quello; una proposta scritta prima della regola («X_PRT») trova il pezzo «X»; un file assegnato
// al pezzo con il suffisso non ha un «codice diverso» perche' dice X; il codice suggerito dal nome e' senza suffisso.
func TestIlPianoConISuffissiDecorativi(t *testing.T) {
	r, err := regole.ValidaRegole([]byte(`{"suffissi_decorativi": ["_PRT"]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := classificazione.Compila("ACME", r)

	p := nuovoPiano()
	p.in.Motore = m
	vecchio := p.componente("77720000_PRT", db.TipoComponenteSciolto)
	nuovo := p.componente("77730000", db.TipoComponenteSciolto)
	p.file("77720000.pdf", db.TipoDocumentoDisegno2d, "77720000", "")
	p.file("77730000_PRT.pdf", db.TipoDocumentoDisegno2d, "77730000_PRT", "")
	assegnato := p.file("77720000 foglio 2.pdf", db.TipoDocumentoDisegno2d, "77720000_PRT", "")
	assegnato.Proposta.ComponenteID = uuid.NullUUID{UUID: vecchio.ComponenteID, Valid: true}
	assegnato.Proposta.Dettagli = json.RawMessage(`{"codice_letto": "77720000"}`)
	anonimo := p.file("scansione.pdf", db.TipoDocumentoDaDeterminare, "", "")
	anonimo.Proposta.Dettagli = json.RawMessage(`{"codici_nel_nome": ["77740000_PRT_B"]}`)
	pf := PianoDelFascicolo(p.in)

	v := vocePer(t, pf, "77720000.pdf")
	deveEssere(t, v, VoceDecidere, DomandaComponente)
	if v.Alias == nil || v.Alias.ComponenteID != vecchio.ComponenteID || v.Componente != nil {
		t.Errorf("il file «77720000» e il pezzo «77720000_PRT»: alias %+v", v.Alias)
	}
	if v := vocePer(t, pf, "77730000_PRT.pdf"); v.Stato != VoceDecidere || v.Alias == nil || v.Alias.ComponenteID != nuovo.ComponenteID || v.Componente != nil {
		t.Errorf("la proposta «77730000_PRT» trova il pezzo 77730000, e chiede di assegnarlo (il codice e' del componente): %+v %v", v.Alias, v.Domande)
	}
	if v := vocePer(t, pf, "77720000 foglio 2.pdf"); v.Stato != VocePronta {
		t.Errorf("assegnato al pezzo con il suffisso, il codice letto 77720000 non e' diverso: %s %v", v.Stato, v.Domande)
	}
	if v := vocePer(t, pf, "scansione.pdf"); v.Suggerito != "77740000" {
		t.Errorf("il codice suggerito dal nome: %q", v.Suggerito)
	}

	// senza la regola: nessun alias, e il codice letto diverso e' una domanda
	p.in.Motore = nil
	pf = PianoDelFascicolo(p.in)
	if v := vocePer(t, pf, "77720000.pdf"); v.Alias != nil || !strings.Contains(v.Domande[0].Testo, "non è nella BOM") {
		t.Errorf("senza regola: %+v %v", v.Alias, v.Domande)
	}
	deveEssere(t, vocePer(t, pf, "77720000 foglio 2.pdf"), VoceDecidere, DomandaCodiceDiverso)
}

// Che cos'e' un file lo dice la dimensione `tipo` della sua valutazione (Smistamento F4, piano.go:529): il piano
// chiede quando nessuna evidenza dice il tipo, o quando lo dice solo l'estensione («altro»). Prima guardava la
// colonna (`altro` con fonte `estensione`), e con la fonte in colonna che adesso e' quella del codice un «.dft»
// con il codice nel nome saltava la domanda. Una riga dell'operatore e' una decisione: non chiede; un tipo letto
// nel contenuto nemmeno.
func TestLaDomandaSulTipoLaFaLaValutazione(t *testing.T) {
	p := nuovoPiano()
	p.componente("7120001", db.TipoComponenteFinito)
	scrivi := func(f *FileAperto, in classificazione.IngressoFile) {
		dett, rp := classificazione.ConValutazione(nil, classificazione.Valuta(in), time.Now())
		f.Proposta.TipoProposto, f.Proposta.Codice, f.Proposta.Rev = db.TipoDocumento(rp.Tipo), testoP(rp.Codice), testoP(rp.Rev)
		f.Proposta.Confidenza, f.Proposta.Fonte, f.Proposta.Dettagli = int16(rp.Confidenza), db.FonteProposta(rp.Fonte), dett
	}
	dft := p.file("7120001.dft", db.TipoDocumentoAltro, "7120001", "")
	scrivi(dft, classificazione.IngressoFile{NomeFile: "7120001.dft", Bytes: 50_000})
	if dft.Proposta.Fonte != db.FontePropostaNomeFile {
		t.Fatalf("la fonte in colonna e' quella del codice: %s", dft.Proposta.Fonte)
	}
	pdf := p.file("7120001.pdf", db.TipoDocumentoDisegno2d, "7120001", "")
	scrivi(pdf, classificazione.IngressoFile{NomeFile: "7120001.pdf", Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"},
		Fatti: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)})
	nonLetto := p.file("7120001_B.pdf", db.TipoDocumentoDaDeterminare, "7120001", "B")
	scrivi(nonLetto, classificazione.IngressoFile{NomeFile: "7120001_B.pdf"})
	deciso := p.file("7120001.doc", db.TipoDocumentoAltro, "7120001", "")
	deciso.Proposta.Fonte = db.FontePropostaOperatore

	pf := PianoDelFascicolo(p.in)
	deveEssere(t, vocePer(t, pf, "7120001.dft"), VoceDecidere, DomandaTipo)
	deveEssere(t, vocePer(t, pf, "7120001_B.pdf"), VoceDecidere, DomandaTipo)
	for _, nome := range []string{"7120001.pdf", "7120001.doc"} {
		for _, d := range vocePer(t, pf, nome).Domande {
			if d.Chiave == DomandaTipo {
				t.Errorf("%s: il tipo e' detto (dal contenuto, o da una persona), e il piano chiede %q", nome, d.Testo)
			}
		}
	}
}

// Giro 4, fase 4.2 (bug 3 della Distinta, 29/09): un PDF con il codice solo dal testo non e' «pronto» per sola
// uguaglianza con un componente. «tavola.pdf» non ha codici nel nome, e il cartiglio dice 7120010 (un codice di
// famiglia ACME): il componente 7120010 c'e', ma il file e' una domanda («il codice viene dal testo del PDF: e'
// 7120010?»), con il componente accanto; lo stesso per «vista.pdf», che 7120010 lo ha solo nel titolo. Le
// controprove: «7120010.pdf», con lo stesso cartiglio e il nome che lo dice, e' pronto; il codice deciso da una
// persona e' pronto. La controprova a mano: senza codiceSoloDalTesto la tavola torna pronta e la prova fallisce.
//
// Riscritta per lo Smistamento (giro 4, fase 4.6): prima il cartiglio era il solo testo «DISEGNO N. 7120010»;
// adesso nella zona del cartiglio vota soltanto il campo del codice, e il cartiglio lo porta (disegnoN).
func TestIlCodiceLettoNelTestoNonFaPronto(t *testing.T) {
	p := nuovoPiano()
	m := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})`, Descrizione: "ACME 712", Esempio: "7120001"}}})
	p.in.Motore = m
	c := p.componente("7120010", db.TipoComponenteSciolto)
	// giro 4, fase 4.6: il cartiglio porta il campo «DISEGNO N.» (nella zona del cartiglio vota solo il campo del codice)
	cartiglio := fattiPDF(t, disegnoN("7120010", "SCALA 1:2"))
	titolo := fattiPDF(t, testoFinto("SCALA 1:2", "", worker.MetadatiPDF{Titolo: "7120010"}))
	scrivi := func(nome string, fatti json.RawMessage) *FileAperto {
		v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: nome, Direzione: "entrata", Motore: m,
			Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, Fatti: fatti})
		dett, rp := classificazione.ConValutazione(fatti, v, time.Now())
		f := p.file(nome, db.TipoDocumento(rp.Tipo), rp.Codice, rp.Rev)
		f.Proposta.Confidenza, f.Proposta.Fonte, f.Proposta.Dettagli = int16(rp.Confidenza), db.FonteProposta(rp.Fonte), dett
		return f
	}
	tavola := scrivi("tavola.pdf", cartiglio)
	scrivi("vista.pdf", titolo)
	scrivi("7120010.pdf", cartiglio)
	deciso := scrivi("foglio 2.pdf", cartiglio)
	deciso.Proposta.Fonte = db.FontePropostaOperatore
	if tavola.Proposta.Codice.String != "7120010" {
		t.Fatalf("la scena: il codice della tavola viene dal suo cartiglio: %+v", tavola.Proposta)
	}

	pf := PianoDelFascicolo(p.in)
	for _, nome := range []string{"tavola.pdf", "vista.pdf"} {
		v := vocePer(t, pf, nome)
		deveEssere(t, v, VoceDecidere, DomandaCodice)
		if len(v.Domande) == 0 || !strings.Contains(v.Domande[0].Testo, "il codice viene dal testo del PDF, non dal nome: è 7120010?") {
			t.Errorf("%s: la domanda dice da dove viene il codice: %v", nome, v.Domande)
		}
		if v.Componente == nil || v.Componente.ComponenteID != c.ComponenteID {
			t.Errorf("%s: la voce sa a quale componente il testo lo manderebbe: %+v", nome, v.Componente)
		}
	}
	deveEssere(t, vocePer(t, pf, "7120010.pdf"), VocePronta, "")
	deveEssere(t, vocePer(t, pf, "foglio 2.pdf"), VocePronta, "")
	if pf.FilePronti() != 2 {
		t.Errorf("i file pronti: %d, attesi 2 (il nome, la persona)", pf.FilePronti())
	}
}
