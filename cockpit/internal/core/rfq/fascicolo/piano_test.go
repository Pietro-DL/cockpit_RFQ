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
	p.in.NomiFile = map[uuid.UUID]string{p.step: "52922757.stp"}
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
func TestIlPianoDelloZipMetteProntoQuelloCheIlServerSaGia(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("52922757", db.TipoComponenteFinito)
	p.nodo("#100", "52922757", db.StatoPropostaDuplicato, &prodotto)
	p.nodo("#110", "52920517", db.StatoPropostaAperta, nil)
	p.nodo("#120", "53011111", db.StatoPropostaAperta, nil)
	p.relazione("#100", "#110", 2)
	p.relazione("#110", "#120", 2)
	p.file("52922757.stp", db.TipoDocumentoCad3d, "52922757", "")
	p.file("52922757.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
	p.file("52920517.pdf", db.TipoDocumentoDisegno2d, "52920517", "")
	p.file("Capitolato fornitura.pdf", db.TipoDocumentoCapitolato, "", "")
	p.file("53017189 foglio 2.pdf", db.TipoDocumentoDisegno2d, "", "")
	zip := p.file("RFQ ACME.zip", db.TipoDocumentoAltro, "", "")
	zip.Proposta.Fonte = db.FontePropostaEstensione

	pf := PianoDelFascicolo(p.in)
	if len(pf.Strutture) != 1 || pf.Strutture[0].Stato != VoceDecidere || pf.Strutture[0].Nodi != 2 || pf.Strutture[0].Archi != 2 ||
		len(pf.Strutture[0].Domande) != 1 || pf.Strutture[0].Domande[0].Chiave != DomandaStrutturaEditor {
		t.Fatalf("struttura dello STEP: %+v", pf.Strutture)
	}
	stp := vocePer(t, pf, "52922757.stp")
	deveEssere(t, stp, VocePronta, "")
	if stp.Componente == nil || stp.Componente.Codice != "52922757" {
		t.Errorf("lo STEP va al prodotto: %+v", stp.Componente)
	}
	deveEssere(t, vocePer(t, pf, "52922757.pdf"), VocePronta, "")
	assieme := vocePer(t, pf, "52920517.pdf")
	deveEssere(t, assieme, VoceDecidere, DomandaComponente)
	if assieme.DaStep == nil || assieme.DaStep.Codice != "52920517" || assieme.Componente != nil || !strings.Contains(assieme.Domande[0].Testo, "editor") {
		t.Errorf("52920517.pdf aspetta la struttura che si conferma nell'editor, e sa a quale nodo va: %+v %+v", assieme.DaStep, assieme.Domande)
	}
	cap := vocePer(t, pf, "Capitolato fornitura.pdf")
	deveEssere(t, cap, VocePronta, "")
	if cap.Destinazione() != "" {
		t.Errorf("il capitolato e' della RFQ, non di un componente: %s", cap.Destinazione())
	}
	foglio := vocePer(t, pf, "53017189 foglio 2.pdf")
	deveEssere(t, foglio, VoceDecidere, DomandaCodice)
	if foglio.Suggerito != "53017189" {
		t.Errorf("il codice suggerito viene dal nome: %q", foglio.Suggerito)
	}
	for _, v := range pf.File {
		if v.Nome == "RFQ ACME.zip" {
			t.Error("lo zip e' un contenitore: non entra nel piano")
		}
	}
	if len(pf.Strutturali) != 1 || pf.Strutturali[0].Stato != VocePronta || pf.Strutturali[0].Nome != "52922757.stp" {
		t.Fatalf("lo STEP strutturale suggerito: %+v", pf.Strutturali)
	}
	if pf.Pronte() != 4 || pf.FilePronti() != 3 || pf.Decisioni() != 3 || pf.InAttesa() != 0 {
		t.Errorf("conti del piano: pronte %d, file %d, decisioni %d, attesa %d", pf.Pronte(), pf.FilePronti(), pf.Decisioni(), pf.InAttesa())
	}
	if a, b := pf.Firma(), PianoDelFascicolo(p.in).Firma(); a != b || a == "" {
		t.Errorf("stessi dati, stessa firma: %q %q", a, b)
	}
}

// Un file che non e' ancora sceso, o il cui lavoro e' in corso, aspetta; uno il cui download e' fallito, o
// sparito dalla cache, chiede di riscaricarlo.
func TestIlLavoroInCorsoAspettaIGuastiSiDicono(t *testing.T) {
	p := nuovoPiano()
	p.componente("52922757", db.TipoComponenteFinito)
	p.file("in lavoro.pdf", db.TipoDocumentoDaDeterminare, "", "").InLavoro = true
	g := p.file("grezzo.pdf", db.TipoDocumentoDaDeterminare, "", "")
	g.Allegato.Stato, g.Presente = db.StatoAllegatoGrezzo, false
	e := p.file("errore.pdf", db.TipoDocumentoDaDeterminare, "", "")
	e.Allegato.Stato, e.Allegato.Errore, e.Presente = db.StatoAllegatoErrore, testoP("elemento non trovato"), false
	p.file("sparito.pdf", db.TipoDocumentoDisegno2d, "52922757", "").Presente = false

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
	prodotto := p.componente("52922757", db.TipoComponenteFinito)
	assieme := p.componente("52920517", db.TipoComponenteSottoassieme)
	assieme.Rev = testoP("A")
	p.in.Componenti[1] = assieme
	archiviato := p.componente("53000001", db.TipoComponenteSciolto)
	adesso := time.Now()
	archiviato.ArchiviatoIl = &adesso
	p.in.Componenti[2] = archiviato
	p.documento(prodotto, "52922757.pdf", db.TipoDocumentoDisegno2d, strings.Repeat("f", 64))

	p.file("anonimo.pdf", db.TipoDocumentoDaDeterminare, "", "")
	altro := p.file("boh.xyz", db.TipoDocumentoAltro, "", "")
	altro.Proposta.Fonte = db.FontePropostaEstensione
	p.file("53999999.pdf", db.TipoDocumentoDisegno2d, "53999999", "")
	p.file("53000001.pdf", db.TipoDocumentoDisegno2d, "53000001", "")
	p.file("52920517_B.pdf", db.TipoDocumentoDisegno2d, "52920517", "B")
	p.file("52922757_nuovo.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
	diverso := p.file("52999999.dxf", db.TipoDocumentoSviluppoDxf, "52922757", "")
	diverso.Proposta.ComponenteID = uuid.NullUUID{UUID: prodotto.ComponenteID, Valid: true}
	diverso.Proposta.Dettagli = json.RawMessage(`{"codice_letto": "52999999"}`)

	pf := PianoDelFascicolo(p.in)
	deveEssere(t, vocePer(t, pf, "anonimo.pdf"), VoceDecidere, DomandaTipo)
	deveEssere(t, vocePer(t, pf, "boh.xyz"), VoceDecidere, DomandaTipo)
	deveEssere(t, vocePer(t, pf, "53999999.pdf"), VoceDecidere, DomandaComponente)
	deveEssere(t, vocePer(t, pf, "53000001.pdf"), VoceDecidere, DomandaComponente)
	deveEssere(t, vocePer(t, pf, "52920517_B.pdf"), VoceDecidere, DomandaRevisione)
	nuovo := vocePer(t, pf, "52922757_nuovo.pdf")
	deveEssere(t, nuovo, VoceDecidere, DomandaSostituzione)
	if len(nuovo.Correnti) != 1 || nuovo.Correnti[0].NomeFile != "52922757.pdf" {
		t.Errorf("la domanda aggiungi/sostituisce nomina il corrente: %+v", nuovo.Correnti)
	}
	deveEssere(t, vocePer(t, pf, "52999999.dxf"), VoceDecidere, DomandaCodiceDiverso)
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
	p.componente("52922757", db.TipoComponenteFinito)
	p.componente("53017189", db.TipoComponenteSciolto)
	p.file("52922757 foglio 1.pdf", db.TipoDocumentoDisegno2d, "52922757", "B")
	p.file("52922757 foglio 2.pdf", db.TipoDocumentoDisegno2d, "52922757", "B")
	p.file("53017189.pdf", db.TipoDocumentoDisegno2d, "53017189", "")
	p.file("53017189_B.pdf", db.TipoDocumentoDisegno2d, "53017189", "B")

	pf := PianoDelFascicolo(p.in)
	for _, n := range []string{"52922757 foglio 1.pdf", "52922757 foglio 2.pdf"} {
		v := vocePer(t, pf, n)
		deveEssere(t, v, VocePronta, "")
		if !v.Aggiunge {
			t.Errorf("%s: i fogli si aggiungono", n)
		}
	}
	for _, n := range []string{"53017189.pdf", "53017189_B.pdf"} {
		deveEssere(t, vocePer(t, pf, n), VoceDecidere, DomandaRevisione)
	}
}

// Lo stesso contenuto gia' documento e' una provenienza; identico a una revisione sostituita e' un rifiuto
// da spiegare; due file uguali nello stesso piano: il secondo e' una provenienza del primo.
func TestLoStessoContenutoNonDiventaUnSecondoDocumento(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("52922757", db.TipoComponenteFinito)
	gia := p.file("52922757 copia.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
	p.documento(prodotto, "52922757.pdf", db.TipoDocumentoDisegno2d, gia.Allegato.Sha256.String)
	vecchio := p.file("52922757 vecchio.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
	d := p.documento(prodotto, "52922757_A.pdf", db.TipoDocumentoDisegno2d, vecchio.Allegato.Sha256.String)
	p.in.Documenti[len(p.in.Documenti)-1].SostituitoDa = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	_ = d
	uno := p.file("capitolato.pdf", db.TipoDocumentoCapitolato, "", "")
	due := p.file("capitolato (1).pdf", db.TipoDocumentoCapitolato, "", "")
	due.Allegato.Sha256 = uno.Allegato.Sha256

	pf := PianoDelFascicolo(p.in)
	v := vocePer(t, pf, "52922757 copia.pdf")
	deveEssere(t, v, VocePronta, "")
	if v.Duplicato != "52922757.pdf" {
		t.Errorf("provenienza di 52922757.pdf: %q", v.Duplicato)
	}
	deveEssere(t, vocePer(t, pf, "52922757 vecchio.pdf"), VoceDecidere, DomandaSostituzione)
	if v := vocePer(t, pf, "capitolato (1).pdf"); v.Duplicato != "capitolato.pdf" {
		t.Errorf("il secondo file uguale e' una provenienza del primo: %q", v.Duplicato)
	}
}

// La struttura di uno STEP non e' pronta se ha un nodo senza codice, un componente archiviato da ripristinare
// o una quantita' diversa; un file che va a un nodo di quella struttura aspetta con lei. Gli archi verso un
// nodo scartato si decidono uno per uno, e non fermano il resto.
func TestLaStrutturaConUnNodoSenzaCodiceAspettaUnaPersona(t *testing.T) {
	p := nuovoPiano()
	prodotto := p.componente("52922757", db.TipoComponenteFinito)
	p.nodo("#100", "52922757", db.StatoPropostaDuplicato, &prodotto)
	p.nodo("#110", "52920517", db.StatoPropostaAperta, nil)
	p.nodo("#120", "", db.StatoPropostaAperta, nil)
	p.relazione("#100", "#110", 2)
	p.relazione("#110", "#120", 1)
	p.file("52920517.pdf", db.TipoDocumentoDisegno2d, "52920517", "")

	pf := PianoDelFascicolo(p.in)
	if pf.Strutture[0].Stato != VoceDecidere || !strings.Contains(pf.Strutture[0].Domande[0].Testo, "senza codice") {
		t.Fatalf("struttura con un nodo senza codice: %+v", pf.Strutture[0])
	}
	deveEssere(t, vocePer(t, pf, "52920517.pdf"), VoceDecidere, DomandaComponente)

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
	deveEssere(t, vocePer(t, pf, "52920517.pdf"), VoceDecidere, DomandaComponente)
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
	prodotto := p.componente("52922757", db.TipoComponenteFinito)
	p.nodo("#100", "52922757", db.StatoPropostaDuplicato, &prodotto)
	p.nodo("#110", "52920517", db.StatoPropostaAperta, nil)
	p.relazione("#100", "#110", 2)
	p.file("52922757.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
	assegnato := p.file("52922757 foglio 2.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
	assegnato.Proposta.ComponenteID = uuid.NullUUID{UUID: prodotto.ComponenteID, Valid: true}
	p.in.Bloccata = 1

	pf := PianoDelFascicolo(p.in)
	if len(pf.Strutture) != 0 || len(pf.Strutturali) != 0 {
		t.Errorf("con la BOM congelata niente struttura e niente STEP strutturale: %+v %+v", pf.Strutture, pf.Strutturali)
	}
	v := vocePer(t, pf, "52922757.pdf")
	deveEssere(t, v, VocePronta, "")
	if v.Componente != nil || v.DaStep != nil {
		t.Errorf("entra senza componente: %+v", v)
	}
	deveEssere(t, vocePer(t, pf, "52922757 foglio 2.pdf"), VoceDecidere, DomandaCongelata)
}

// Lo STEP strutturale si presenta gia' scelto quando il prodotto ne avra' uno solo; con due si sceglie; con
// quello gia' fissato non si propone niente.
func TestLoStepStrutturaleSiSuggerisceSoloSeEUnoSolo(t *testing.T) {
	p := nuovoPiano()
	uno := p.componente("52922757", db.TipoComponenteFinito)
	due := p.componente("52922758", db.TipoComponenteFinito)
	fissato := p.componente("52922759", db.TipoComponenteFinito)
	fissato.StepStrutturaleID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	p.in.Componenti[2] = fissato
	p.documento(uno, "52922757.stp", db.TipoDocumentoCad3d, strings.Repeat("1", 64))
	p.documento(due, "52922758.stp", db.TipoDocumentoCad3d, strings.Repeat("2", 64))
	p.file("52922758_B.stp", db.TipoDocumentoCad3d, "52922758", "B")

	pf := PianoDelFascicolo(p.in)
	per := map[string]VoceStrutturale{}
	for _, v := range pf.Strutturali {
		per[v.Prodotto.Codice] = v
	}
	if v := per["52922757"]; v.Stato != VocePronta || !v.Documento.Valid || v.Nome != "52922757.stp" {
		t.Errorf("uno STEP solo, gia' confermato: %+v", v)
	}
	// 52922758 ha gia' uno STEP corrente: il file nuovo chiede aggiungi o sostituisce, e finche' non si
	// decide non e' fra i candidati. Lo STEP strutturale suggerito resta quello corrente.
	deveEssere(t, vocePer(t, pf, "52922758_B.stp"), VoceDecidere, DomandaSostituzione)
	if v := per["52922758"]; v.Stato != VocePronta || v.Nome != "52922758.stp" {
		t.Errorf("il corrente resta il candidato: %+v", v)
	}
	if _, c := per["52922759"]; c {
		t.Error("un prodotto con lo STEP strutturale fissato non ne riceve un altro")
	}

	// due STEP correnti: si sceglie
	p.documento(uno, "52922757 bis.stp", db.TipoDocumentoCad3d, strings.Repeat("3", 64))
	pf = PianoDelFascicolo(p.in)
	for _, v := range pf.Strutturali {
		if v.Prodotto.Codice == "52922757" && (v.Stato != VoceDecidere || v.Domande[0].Chiave != DomandaStrutturale) {
			t.Errorf("due STEP: si sceglie: %+v", v)
		}
	}
}

// La firma cambia quando cambia una voce pronta: chi conferma deve aver visto il piano che conferma.
func TestLaFirmaCambiaConIlPiano(t *testing.T) {
	p := nuovoPiano()
	p.componente("52922757", db.TipoComponenteFinito)
	p.file("52922757.pdf", db.TipoDocumentoDisegno2d, "52922757", "")
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
	vecchio := p.componente("52920000_PRT", db.TipoComponenteSciolto)
	nuovo := p.componente("52930000", db.TipoComponenteSciolto)
	p.file("52920000.pdf", db.TipoDocumentoDisegno2d, "52920000", "")
	p.file("52930000_PRT.pdf", db.TipoDocumentoDisegno2d, "52930000_PRT", "")
	assegnato := p.file("52920000 foglio 2.pdf", db.TipoDocumentoDisegno2d, "52920000_PRT", "")
	assegnato.Proposta.ComponenteID = uuid.NullUUID{UUID: vecchio.ComponenteID, Valid: true}
	assegnato.Proposta.Dettagli = json.RawMessage(`{"codice_letto": "52920000"}`)
	anonimo := p.file("scansione.pdf", db.TipoDocumentoDaDeterminare, "", "")
	anonimo.Proposta.Dettagli = json.RawMessage(`{"codici_nel_nome": ["52940000_PRT_B"]}`)
	pf := PianoDelFascicolo(p.in)

	v := vocePer(t, pf, "52920000.pdf")
	deveEssere(t, v, VoceDecidere, DomandaComponente)
	if v.Alias == nil || v.Alias.ComponenteID != vecchio.ComponenteID || v.Componente != nil {
		t.Errorf("il file «52920000» e il pezzo «52920000_PRT»: alias %+v", v.Alias)
	}
	if v := vocePer(t, pf, "52930000_PRT.pdf"); v.Stato != VoceDecidere || v.Alias == nil || v.Alias.ComponenteID != nuovo.ComponenteID || v.Componente != nil {
		t.Errorf("la proposta «52930000_PRT» trova il pezzo 52930000, e chiede di assegnarlo (il codice e' del componente): %+v %v", v.Alias, v.Domande)
	}
	if v := vocePer(t, pf, "52920000 foglio 2.pdf"); v.Stato != VocePronta {
		t.Errorf("assegnato al pezzo con il suffisso, il codice letto 52920000 non e' diverso: %s %v", v.Stato, v.Domande)
	}
	if v := vocePer(t, pf, "scansione.pdf"); v.Suggerito != "52940000" {
		t.Errorf("il codice suggerito dal nome: %q", v.Suggerito)
	}

	// senza la regola: nessun alias, e il codice letto diverso e' una domanda
	p.in.Motore = nil
	pf = PianoDelFascicolo(p.in)
	if v := vocePer(t, pf, "52920000.pdf"); v.Alias != nil || !strings.Contains(v.Domande[0].Testo, "non è nella BOM") {
		t.Errorf("senza regola: %+v %v", v.Alias, v.Domande)
	}
	deveEssere(t, vocePer(t, pf, "52920000 foglio 2.pdf"), VoceDecidere, DomandaCodiceDiverso)
}
