package fascicolo

// Il piano di riconciliazione del Fascicolo (B8.7b).
//
// Il server sa gia' quasi tutto dei file di una RFQ: il tipo (dall'analisi), il codice (dal cartiglio, dallo
// STEP, dal nome), il componente (quello con lo stesso codice, o quello che nasce dallo STEP nella stessa
// conferma). Chiederlo di nuovo file per file e' mostrare il modello dei dati invece del lavoro. Il piano
// mette ogni file aperto della RFQ in una di tre condizioni:
//
//	pronto     tipo, codice e componente determinati senza conflitti: entra con «Conferma Fascicolo», una
//	           decisione per tutti, in una transazione
//	decidere   un'ambiguita' vera, con la sua domanda: tipo da determinare, codice mancante o diverso da
//	           quello del componente, revisione discordante, aggiungi o sostituisce, componente incerto
//	attesa     il file non e' ancora sceso o il suo lavoro (download, estrazione, analisi) e' in corso: il
//	           piano lo riprende quando arriva
//
// Lo stesso per la struttura di ogni STEP (i suoi nodi e archi aperti, accettati insieme come «Accetta
// tutto il file») e per lo STEP strutturale di un prodotto che non l'ha ancora (A4.4, D31: lo sceglie una
// persona; qui glielo si presenta gia' scelto quando e' uno solo, e la conferma lo fissa).
//
// Il piano e' una regola pura: non scrive niente, e con gli stessi dati da' lo stesso piano con la stessa
// firma. «Conferma Fascicolo» lo ricalcola nella sua transazione e conferma solo se la firma e' quella che
// l'operatore aveva davanti. Solo allora nascono i documenti e parte la copia sul NAS: prima di sapere
// tipo, componente e revisione un file sta nello staging, non sul NAS (A4.2, A4.3).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// StatoVoce e' la condizione di una voce del piano.
type StatoVoce string

const (
	VocePronta   StatoVoce = "pronto"
	VoceDecidere StatoVoce = "decidere"
	VoceAttesa   StatoVoce = "attesa"
)

// Le domande di una voce da decidere: la chiave dice quale gesto la risolve.
const (
	DomandaFile          = "file"           // il file non c'e' (download fallito, sparito dalla cache)
	DomandaTipo          = "tipo"           // che cos'e' il file: il contenuto non l'ha detto
	DomandaCodice        = "codice"         // un documento tecnico senza codice
	DomandaCodiceDiverso = "codice_diverso" // il file dice un codice, il componente a cui e' assegnato un altro
	DomandaComponente    = "componente"     // il codice non e' nella BOM, o e' di un componente archiviato
	DomandaRevisione     = "revisione"      // revisione discordante con il componente o fra i file arrivati insieme
	DomandaSostituzione  = "sostituzione"   // il componente ha gia' un documento corrente dello stesso tipo
	DomandaStruttura     = "struttura"      // una proposta dello STEP che non si accetta senza una persona
	// DomandaStrutturaEditor: la struttura proposta da uno STEP si conferma dall'editor della BOM (Fascicolo
	// v3), dopo averla vista; non entra con «Conferma Fascicolo».
	DomandaStrutturaEditor = "struttura_editor"
	DomandaStrutturale     = "strutturale" // piu' STEP candidati a STEP strutturale del prodotto
	DomandaCongelata       = "congelata"   // BOM congelata e file gia' assegnato: si sgancia o si apre una revisione
)

// Domanda e' una cosa che una persona deve decidere, con le parole della schermata.
type Domanda struct {
	Chiave string
	Testo  string
}

// NodoInArrivo e' un componente che non c'e' ancora e nasce dalla struttura di uno STEP nella stessa
// conferma: il file con quel codice va li'.
type NodoInArrivo struct {
	Allegato uuid.UUID // lo STEP
	File     string
	Proposta uuid.UUID // il nodo proposto
	Codice   string
}

// VoceFile e' un file aperto della RFQ nel piano.
type VoceFile struct {
	Allegato   uuid.UUID
	Proposta   uuid.UUID
	Nome       string
	Estensione string
	Sha256     string
	Tipo       db.TipoDocumento
	Codice     string
	Rev        string
	Interno    bool           // caricato a mano (una versione interna)
	Componente *db.Componente // il componente a cui va; nil = senza componente, o nasce dallo STEP (DaStep)
	// Alias: il componente nato con il suffisso decorativo del cliente («X_PRT», prima della regola), che il file
	// «X» non trova per codice: e' lo stesso pezzo, e il gesto e' assegnarlo a quello
	Alias *db.Componente
	// DaStep: il nodo dello STEP da cui nasce il componente a cui va. Con la struttura pronta la voce e' pronta
	// ed entra con lei; con la struttura da confermare nell'editor (Fascicolo v3) la voce aspetta, ma sa gia'
	// a quale nodo va (il pannello del nodo la mostra fra i file in arrivo).
	DaStep *NodoInArrivo
	// Aggiunge: entra accanto ad altri file dello stesso tipo e dello stesso componente arrivati insieme e
	// con la stessa revisione (i fogli di un disegno): alla conferma si aggiunge, nessuno sostituisce l'altro.
	Aggiunge bool
	// Duplicato: il nome del documento che ha gia' lo stesso contenuto. La conferma registra solo la
	// provenienza (A1.3): e' lo stesso file arrivato due volte.
	Duplicato string
	// Correnti: i documenti correnti dello stesso tipo nel componente, quando la domanda e' aggiungi o
	// sostituisce (A4.10).
	Correnti []db.Documento
	// Suggerito: il codice che il nome del file contiene, per la domanda «codice».
	Suggerito string
	Stato     StatoVoce
	Domande   []Domanda
}

// Destinazione e' il codice del componente a cui la voce va: quello del componente, quello che nasce dallo
// STEP, o "" (un documento della RFQ senza componente).
func (v VoceFile) Destinazione() string {
	switch {
	case v.Componente != nil:
		return v.Componente.Codice
	case v.DaStep != nil:
		return v.DaStep.Codice
	}
	return ""
}

// VoceStruttura sono le proposte aperte di uno STEP.
type VoceStruttura struct {
	Allegato uuid.UUID
	Nome     string
	Nodi     int      // nodi aperti con un codice: nascono (o si ritrovano) come componenti
	Archi    int      // archi aperti fra nodi vivi
	Morti    int      // archi aperti verso un nodo scartato: si decidono uno per uno (A1.1), non bloccano il resto
	Nuovi    []string // i codici che nascerebbero
	Stato    StatoVoce
	Domande  []Domanda
}

// VoceStrutturale e' lo STEP strutturale suggerito per un prodotto finito che non l'ha ancora.
type VoceStrutturale struct {
	Prodotto  db.Componente
	Allegato  uuid.UUID     // lo STEP fra i file del piano; zero se e' gia' un documento
	Documento uuid.NullUUID // lo STEP gia' confermato, se e' quello
	Nome      string
	Stato     StatoVoce
	Domande   []Domanda
}

// PianoFascicolo e' il piano di riconciliazione di una RFQ.
type PianoFascicolo struct {
	File        []VoceFile
	Strutture   []VoceStruttura
	Strutturali []VoceStrutturale
	Bloccata    int32 // la BOM e' congelata in questa versione: i file entrano senza componente
}

// Pronte conta le voci che entrano con «Conferma Fascicolo».
func (p PianoFascicolo) Pronte() int {
	n := 0
	for _, v := range p.File {
		if v.Stato == VocePronta {
			n++
		}
	}
	for _, v := range p.Strutture {
		if v.Stato == VocePronta {
			n++
		}
	}
	for _, v := range p.Strutturali {
		if v.Stato == VocePronta {
			n++
		}
	}
	return n
}

// FilePronti conta i file pronti.
func (p PianoFascicolo) FilePronti() int {
	n := 0
	for _, v := range p.File {
		if v.Stato == VocePronta {
			n++
		}
	}
	return n
}

// Decisioni conta le voci che aspettano una persona.
func (p PianoFascicolo) Decisioni() int {
	n := 0
	for _, v := range p.File {
		if v.Stato == VoceDecidere {
			n++
		}
	}
	for _, v := range p.Strutture {
		if v.Stato == VoceDecidere || v.Morti > 0 {
			n++
		}
	}
	for _, v := range p.Strutturali {
		if v.Stato == VoceDecidere {
			n++
		}
	}
	return n
}

// InAttesa conta i file il cui lavoro non e' finito.
func (p PianoFascicolo) InAttesa() int {
	n := 0
	for _, v := range p.File {
		if v.Stato == VoceAttesa {
			n++
		}
	}
	return n
}

// Firma riassume le voci pronte: chi conferma dice quale piano ha visto, e se nel frattempo e' cambiato (un
// file analizzato, una decisione di un collega) la conferma si ferma invece di confermare altro.
func (p PianoFascicolo) Firma() string {
	var righe []string
	for _, v := range p.File {
		if v.Stato != VocePronta {
			continue
		}
		dest := ""
		switch {
		case v.Componente != nil:
			dest = "c:" + v.Componente.ComponenteID.String()
		case v.DaStep != nil:
			dest = "s:" + v.DaStep.Proposta.String()
		}
		righe = append(righe, fmt.Sprintf("F|%s|%s|%s|%s|%s|%t|%s", v.Proposta, v.Tipo, v.Codice, v.Rev, dest, v.Aggiunge, v.Duplicato))
	}
	for _, v := range p.Strutture {
		if v.Stato == VocePronta {
			righe = append(righe, fmt.Sprintf("S|%s|%d|%d", v.Allegato, v.Nodi, v.Archi))
		}
	}
	for _, v := range p.Strutturali {
		if v.Stato == VocePronta {
			righe = append(righe, fmt.Sprintf("T|%s|%s|%s", v.Prodotto.ComponenteID, v.Allegato, v.Documento.UUID))
		}
	}
	sort.Strings(righe)
	h := sha256.Sum256([]byte(strings.Join(righe, "\n")))
	return hex.EncodeToString(h[:8])
}

// DaVerificare conta, per ogni componente, le decisioni che lo riguardano: i file che vanno a lui (o che
// hanno il suo codice) e aspettano una persona, e lo STEP strutturale da scegliere. La card lo mostra.
func (p PianoFascicolo) DaVerificare() map[uuid.UUID]int {
	out := map[uuid.UUID]int{}
	for _, v := range p.File {
		if v.Stato == VoceDecidere && v.Componente != nil {
			out[v.Componente.ComponenteID]++
		}
	}
	for _, v := range p.Strutturali {
		if v.Stato == VoceDecidere {
			out[v.Prodotto.ComponenteID]++
		}
	}
	return out
}

// FileAperto e' un file della RFQ con la sua proposta aperta, come lo legge il piano.
type FileAperto struct {
	Allegato db.ListAllegatiFascicoloRow
	Proposta db.DocumentoProposta
	Presente bool // il contenuto e' nello staging (verificato sul disco da chi legge)
	InLavoro bool // un download, un'estrazione o un'analisi di questo file e' in corso
}

// IngressoPiano e' tutto quello che il piano legge.
type IngressoPiano struct {
	File       []FileAperto
	Componenti []db.Componente
	Documenti  []db.Documento
	Nodi       []db.ComponenteProposta
	Relazioni  []db.RelazioneProposta
	NomiFile   map[uuid.UUID]string // allegato → nome, per gli STEP
	Bloccata   int32
	Motore     *classificazione.Motore // le regole del cliente della RFQ: i suffissi decorativi (nil = nessuna)
}

// dettagliProposta sono le chiavi dei dettagli che il piano usa.
type dettagliProposta struct {
	CodiceLetto   string   `json:"codice_letto"`
	CodiciNelNome []string `json:"codici_nel_nome"`
}

func maiuscolo(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// PianoDelFascicolo calcola il piano. Pura.
func PianoDelFascicolo(in IngressoPiano) PianoFascicolo {
	p := PianoFascicolo{Bloccata: in.Bloccata}
	perCodice := map[string]db.Componente{}
	perID := map[uuid.UUID]db.Componente{}
	alias := map[string]db.Componente{} // codice senza suffisso → il componente nato con il suffisso
	for _, c := range in.Componenti {
		perCodice[maiuscolo(c.Codice)] = c
		perID[c.ComponenteID] = c
	}
	if in.Motore.HaSuffissi() {
		ordinati := append([]db.Componente(nil), in.Componenti...)
		sort.Slice(ordinati, func(i, j int) bool { return ordinati[i].Codice < ordinati[j].Codice })
		for _, c := range ordinati {
			can, _ := in.Motore.Canonico(c.Codice, "")
			k := maiuscolo(can)
			if _, esiste := perCodice[k]; k == maiuscolo(c.Codice) || esiste || c.ArchiviatoIl != nil {
				continue
			}
			if _, gia := alias[k]; !gia {
				alias[k] = c
			}
		}
	}
	correnti := map[uuid.UUID][]db.Documento{}
	perHash := map[string]db.Documento{}
	for _, d := range in.Documenti {
		perHash[d.Sha256] = d
		if d.ComponenteID.Valid && !d.SostituitoDa.Valid {
			correnti[d.ComponenteID.UUID] = append(correnti[d.ComponenteID.UUID], d)
		}
	}

	strutture, inArrivo, inSospeso := struttureDi(in)
	p.Strutture = strutture

	primoPerHash := map[string]string{} // nello stesso piano: il secondo file con lo stesso contenuto e' una provenienza
	for _, f := range in.File {
		v, ok := voceFile(f, in.Bloccata, perCodice, perID, correnti, perHash, inArrivo, inSospeso, alias, in.Motore)
		if !ok {
			continue
		}
		if v.Stato == VocePronta && v.Duplicato == "" && v.Sha256 != "" {
			if primo, c := primoPerHash[v.Sha256]; c {
				v.Duplicato = primo
			} else {
				primoPerHash[v.Sha256] = v.Nome
			}
		}
		p.File = append(p.File, v)
	}
	fratelli(p.File)
	p.Strutturali = strutturaliDi(in, p.File, correnti)
	return p
}

// struttureDi fa una voce per ogni STEP con proposte aperte. inArrivo sono i codici dei nodi che nascono da
// una struttura pronta; inSospeso quelli di una struttura che aspetta una decisione.
func struttureDi(in IngressoPiano) ([]VoceStruttura, map[string]NodoInArrivo, map[string]NodoInArrivo) {
	inArrivo := map[string]NodoInArrivo{}
	inSospeso := map[string]NodoInArrivo{}
	if in.Bloccata > 0 {
		return nil, inArrivo, inSospeso
	}
	type chiave struct {
		a uuid.UUID
		k string
	}
	nodo := map[chiave]db.ComponenteProposta{}
	voci := map[uuid.UUID]*VoceStruttura{}
	var ordine []uuid.UUID
	voce := func(a uuid.UUID) *VoceStruttura {
		if v, ok := voci[a]; ok {
			return v
		}
		v := &VoceStruttura{Allegato: a, Nome: in.NomiFile[a]}
		voci[a] = v
		ordine = append(ordine, a)
		return v
	}
	senzaCodice := map[uuid.UUID][]string{}
	archiviati := map[uuid.UUID][]string{}
	for _, n := range in.Nodi {
		nodo[chiave{n.AllegatoID, n.Chiave}] = n
		if n.Stato != db.StatoPropostaAperta {
			continue
		}
		v := voce(n.AllegatoID)
		codice := strings.TrimSpace(n.Codice.String)
		switch {
		case codice == "":
			senzaCodice[n.AllegatoID] = append(senzaCodice[n.AllegatoID], nomeNodo(n))
		case n.Nota.Valid && strings.Contains(n.Nota.String, "archiviato"):
			archiviati[n.AllegatoID] = append(archiviati[n.AllegatoID], codice)
		default:
			v.Nodi++
			v.Nuovi = append(v.Nuovi, codice)
		}
	}
	quantita := map[uuid.UUID][]string{}
	for _, r := range in.Relazioni {
		if r.Stato != db.StatoPropostaAperta {
			continue
		}
		v := voce(r.AllegatoID)
		pn, fn := nodo[chiave{r.AllegatoID, r.PadreChiave}], nodo[chiave{r.AllegatoID, r.FiglioChiave}]
		if pn.Stato == db.StatoPropostaScartata || fn.Stato == db.StatoPropostaScartata {
			v.Morti++
			continue
		}
		if r.Nota.Valid && strings.HasPrefix(r.Nota.String, "qta diversa") {
			quantita[r.AllegatoID] = append(quantita[r.AllegatoID], nomeNodo(pn)+" → "+nomeNodo(fn)+" ("+r.Nota.String+")")
			continue
		}
		v.Archi++
	}
	var out []VoceStruttura
	for _, a := range ordine {
		v := voci[a]
		if s := senzaCodice[a]; len(s) > 0 {
			v.Domande = append(v.Domande, Domanda{DomandaStruttura, fmt.Sprintf("%s senza codice: si scrive il codice o si scarta il nodo", elencoBreveNomi(s))})
		}
		if s := archiviati[a]; len(s) > 0 {
			v.Domande = append(v.Domande, Domanda{DomandaStruttura, fmt.Sprintf("%s è archiviato: accettarlo lo ripristina", elencoBreveNomi(s))})
		}
		if s := quantita[a]; len(s) > 0 {
			v.Domande = append(v.Domande, Domanda{DomandaStruttura, "quantità diverse dalla BOM: " + strings.Join(s, "; ")})
		}
		if v.Morti > 0 {
			v.Domande = append(v.Domande, Domanda{DomandaStruttura, fmt.Sprintf("%d arc%s verso nodi scartati: si scartano uno per uno", v.Morti, map[bool]string{true: "o", false: "hi"}[v.Morti == 1])})
		}
		bloccanti := len(senzaCodice[a]) + len(archiviati[a]) + len(quantita[a])
		switch {
		case bloccanti > 0:
			v.Stato = VoceDecidere
		case v.Nodi+v.Archi > 0:
			// Fascicolo v3: la struttura proposta da uno STEP si guarda nell'editor prima di entrare (la
			// specifica: «deve essere visualizzata prima della conferma in un editor grafico»). Non entra piu'
			// con «Conferma Fascicolo»: e' una decisione, e i file che vanno ai suoi componenti la aspettano.
			v.Stato = VoceDecidere
			var cosa []string
			if v.Nodi > 0 {
				cosa = append(cosa, quanti(v.Nodi, "nodo", "nodi"))
			}
			if v.Archi > 0 {
				cosa = append(cosa, quanti(v.Archi, "arco", "archi"))
			}
			propost := "proposti"
			if v.Nodi+v.Archi == 1 {
				propost = "proposto"
			}
			v.Domande = append(v.Domande, Domanda{DomandaStrutturaEditor, fmt.Sprintf("%s %s: la struttura si rivede e si conferma nell'editor (Struttura BOM)",
				strings.Join(cosa, " e "), propost)})
		default:
			v.Stato = "" // solo archi morti: non c'e' niente da confermare, c'e' da decidere quelli
		}
		for _, n := range in.Nodi {
			if n.AllegatoID != a || n.Stato != db.StatoPropostaAperta || !n.Codice.Valid || strings.TrimSpace(n.Codice.String) == "" {
				continue
			}
			k := maiuscolo(n.Codice.String)
			nodo := NodoInArrivo{Allegato: a, File: v.Nome, Proposta: n.PropostaID, Codice: strings.TrimSpace(n.Codice.String)}
			if v.Stato == VocePronta {
				if _, gia := inArrivo[k]; !gia {
					inArrivo[k] = nodo
				}
			} else if _, gia := inSospeso[k]; !gia {
				inSospeso[k] = nodo
			}
		}
		out = append(out, *v)
	}
	return out, inArrivo, inSospeso
}

func elencoBreveNomi(nomi []string) string {
	if len(nomi) <= 3 {
		return strings.Join(nomi, ", ")
	}
	return fmt.Sprintf("%s e altri %d", strings.Join(nomi[:3], ", "), len(nomi)-3)
}

// voceFile mette un file nella sua condizione. ok = false: il file non e' materia del piano (un archivio,
// che e' un contenitore e si estrae; il rumore; una proposta gia' decisa).
func voceFile(f FileAperto, bloccata int32, perCodice map[string]db.Componente, perID map[uuid.UUID]db.Componente,
	correnti map[uuid.UUID][]db.Documento, perHash map[string]db.Documento, inArrivo map[string]NodoInArrivo,
	inSospeso map[string]NodoInArrivo, alias map[string]db.Componente, m *classificazione.Motore) (VoceFile, bool) {
	a, pr := f.Allegato, f.Proposta
	ext := strings.ToLower(strings.TrimPrefix(a.Estensione.String, "."))
	v := VoceFile{Allegato: a.AllegatoID, Proposta: pr.PropostaID, Nome: a.NomeFile, Estensione: ext, Sha256: a.Sha256.String,
		Tipo: pr.TipoProposto, Codice: strings.TrimSpace(pr.Codice.String), Rev: strings.TrimSpace(pr.Rev.String),
		Interno: a.Origine == db.OrigineAllegatoManuale}
	if pr.Stato != db.StatoPropostaAperta || ext == "zip" || pr.TipoProposto == db.TipoDocumentoRumore {
		return v, false
	}
	decidi := func(chiave, testo string) (VoceFile, bool) {
		v.Stato = VoceDecidere
		v.Domande = append(v.Domande, Domanda{chiave, testo})
		return v, true
	}
	switch {
	case f.InLavoro:
		v.Stato = VoceAttesa
		return v, true
	case a.Stato == db.StatoAllegatoErrore:
		return decidi(DomandaFile, "il download non è riuscito ("+a.Errore.String+"): si riscarica")
	case a.Stato == db.StatoAllegatoGrezzo:
		v.Stato = VoceAttesa // non ancora sceso: la preparazione lo scarica
		return v, true
	case !f.Presente:
		return decidi(DomandaFile, "il file non è più nello staging: si riscarica")
	}

	var dett dettagliProposta
	_ = json.Unmarshal(pr.Dettagli, &dett)
	if len(dett.CodiciNelNome) == 0 {
		// i dettagli dell'analisi sostituiscono quelli della proposta dal nome: i codici che il nome contiene
		// si rileggono dal nome, che non cambia
		dett.CodiciNelNome = classificazione.PropostaDaNome(a.NomeFile, 0, string(db.DirezioneEntrata)).CodiciNelNome
	}
	// il codice suggerito e' quello del pezzo, senza il suffisso decorativo del cliente
	for i, c := range dett.CodiciNelNome {
		dett.CodiciNelNome[i] = m.CanonicoNome(c)
	}
	// uguale e' uguale a meno del suffisso decorativo: il file «X» assegnato al pezzo «X_PRT» non ha un codice diverso
	uguali := func(a, b string) bool {
		ka, _ := m.Canonico(a, "")
		kb, _ := m.Canonico(b, "")
		return strings.EqualFold(strings.TrimSpace(ka), strings.TrimSpace(kb))
	}
	if pr.TipoProposto == db.TipoDocumentoDaDeterminare || (pr.TipoProposto == db.TipoDocumentoAltro && pr.Fonte == db.FontePropostaEstensione) {
		if len(dett.CodiciNelNome) == 1 {
			v.Suggerito = dett.CodiciNelNome[0]
		}
		return decidi(DomandaTipo, "che cos'è questo file? Il contenuto non lo dice")
	}
	if d, c := perHash[v.Sha256]; c && v.Sha256 != "" {
		if d.SostituitoDa.Valid {
			return decidi(DomandaSostituzione, "è identico a "+d.NomeFile+", che è stato sostituito: si torna a quella revisione annullando la sostituzione")
		}
		v.Duplicato = d.NomeFile
		v.Stato = VocePronta
		return v, true
	}

	tecnicoFile := tecnico(string(pr.TipoProposto))
	if tecnicoFile && v.Codice == "" {
		if len(dett.CodiciNelNome) == 1 {
			v.Suggerito = dett.CodiciNelNome[0]
		}
		return decidi(DomandaCodice, "un documento tecnico entra nel fascicolo con il codice del pezzo: quale?")
	}
	if bloccata > 0 {
		// D26: con la BOM congelata un file entra senza componente, e lo si assegna aprendo una revisione. Uno
		// assegnato prima del congelamento non si conferma in silenzio senza il suo componente: si sgancia (e
		// allora entra senza), o si aspetta la revisione.
		if pr.ComponenteID.Valid {
			if c, trovato := perID[pr.ComponenteID.UUID]; trovato {
				v.Componente = &c
			}
			return decidi(DomandaCongelata, "la BOM è congelata: il file è assegnato a un componente, e con la BOM congelata entra senza. Si sgancia, o si conferma aprendo una revisione")
		}
		v.Stato = VocePronta
		return v, true
	}

	switch {
	case pr.ComponenteID.Valid:
		c, trovato := perID[pr.ComponenteID.UUID]
		if !trovato {
			return decidi(DomandaComponente, "il componente a cui era assegnato non c'è più")
		}
		v.Componente = &c
		if dett.CodiceLetto != "" && !uguali(dett.CodiceLetto, c.Codice) {
			return decidi(DomandaCodiceDiverso, fmt.Sprintf("il file dice %s, ed è assegnato a %s", dett.CodiceLetto, c.Codice))
		}
	case tecnicoFile:
		chiave := maiuscolo(v.Codice)
		if c, trovato := perCodice[chiave]; trovato {
			if c.ArchiviatoIl != nil {
				v.Componente = &c
				return decidi(DomandaComponente, c.Codice+" è archiviato: si ripristina, o il file resta senza componente")
			}
			v.Componente = &c
			break
		}
		if n, trovato := inArrivo[chiave]; trovato {
			n := n
			v.DaStep = &n
			break
		}
		if n, trovato := inSospeso[chiave]; trovato {
			n := n
			v.DaStep = &n
			return decidi(DomandaComponente, fmt.Sprintf("%s nasce dalla struttura dello STEP %s: si conferma prima quella, nell'editor della Struttura BOM", v.Codice, n.File))
		}
		if c, trovato := alias[chiave]; trovato {
			c := c
			v.Alias = &c
			return decidi(DomandaComponente, fmt.Sprintf("%s: nella BOM c'è %s, lo stesso pezzo con il suffisso del cliente. Si assegna a %s (il file prende il suo codice), o si corregge il codice del componente", v.Codice, c.Codice, c.Codice))
		}
		// una proposta scritta prima della regola dice ancora «X_PRT», e il pezzo e' «X»: lo stesso pezzo. Il file
		// si assegna a quello e ne prende il codice (un documento ha il codice del suo componente)
		if can, _ := m.Canonico(v.Codice, ""); maiuscolo(can) != chiave {
			if c, trovato := perCodice[maiuscolo(can)]; trovato && c.ArchiviatoIl == nil {
				c := c
				v.Alias = &c
				return decidi(DomandaComponente, fmt.Sprintf("%s: nella BOM c'è %s, lo stesso pezzo senza il suffisso del cliente. Si assegna a %s (il file prende il suo codice)", v.Codice, c.Codice, c.Codice))
			}
			if n, trovato := inSospeso[maiuscolo(can)]; trovato {
				n := n
				v.DaStep = &n
				return decidi(DomandaComponente, fmt.Sprintf("%s nasce dalla struttura dello STEP %s: si conferma prima quella, nell'editor della Struttura BOM", n.Codice, n.File))
			}
		}
		return decidi(DomandaComponente, v.Codice+" non è nella BOM: si aggiunge, si assegna a un componente o il file resta senza componente")
	}

	if c := v.Componente; c != nil {
		if c.Rev.Valid && strings.TrimSpace(c.Rev.String) != "" && v.Rev != "" && !strings.EqualFold(v.Rev, strings.TrimSpace(c.Rev.String)) {
			return decidi(DomandaRevisione, fmt.Sprintf("il file è rev %s, il componente %s è rev %s", v.Rev, c.Codice, c.Rev.String))
		}
		for _, d := range correnti[c.ComponenteID] {
			if d.Tipo == pr.TipoProposto {
				v.Correnti = append(v.Correnti, d)
			}
		}
		if len(v.Correnti) > 0 {
			return decidi(DomandaSostituzione, fmt.Sprintf("%s ha già %d %s: si aggiunge, o sostituisce quale?", c.Codice, len(v.Correnti), etichettaTipo(pr.TipoProposto)))
		}
	}
	v.Stato = VocePronta
	return v, true
}

// fratelli guarda i file pronti che vanno allo stesso componente con lo stesso tipo. Con la stessa
// revisione sono fogli dello stesso disegno: entrano insieme, e ciascuno si aggiunge agli altri. Con
// revisioni diverse (anche «nessuna» contro «B») uno potrebbe sostituire l'altro: lo decide una persona.
func fratelli(voci []VoceFile) {
	gruppi := map[string][]int{}
	for i, v := range voci {
		if v.Stato != VocePronta || v.Duplicato != "" || v.Destinazione() == "" {
			continue
		}
		k := maiuscolo(v.Destinazione()) + "|" + string(v.Tipo)
		gruppi[k] = append(gruppi[k], i)
	}
	for _, idx := range gruppi {
		if len(idx) < 2 {
			continue
		}
		revs := map[string]bool{}
		for _, i := range idx {
			revs[maiuscolo(voci[i].Rev)] = true
		}
		if len(revs) > 1 {
			var elenco []string
			for r := range revs {
				if r == "" {
					r = "senza revisione"
				}
				elenco = append(elenco, r)
			}
			sort.Strings(elenco)
			for _, i := range idx {
				voci[i].Stato = VoceDecidere
				voci[i].Domande = append(voci[i].Domande, Domanda{DomandaRevisione,
					"revisioni diverse fra i file arrivati per " + voci[i].Destinazione() + " (" + strings.Join(elenco, ", ") + "): quale vale?"})
			}
			continue
		}
		for _, i := range idx {
			voci[i].Aggiunge = true
		}
	}
}

// strutturaliDi suggerisce lo STEP strutturale di ogni prodotto finito che non l'ha: se dopo la conferma
// il prodotto avra' un solo STEP, e' quello (presentato gia' scelto, D31: lo fissa la conferma); se ne avra'
// piu' d'uno, si sceglie.
func strutturaliDi(in IngressoPiano, voci []VoceFile, correnti map[uuid.UUID][]db.Documento) []VoceStrutturale {
	if in.Bloccata > 0 {
		return nil
	}
	var out []VoceStrutturale
	for _, c := range in.Componenti {
		if c.Tipo != db.TipoComponenteFinito || c.ArchiviatoIl != nil || c.StepStrutturaleID.Valid {
			continue
		}
		type candidato struct {
			allegato  uuid.UUID
			documento uuid.NullUUID
			nome      string
		}
		var cand []candidato
		for _, d := range correnti[c.ComponenteID] {
			if Step(d) {
				cand = append(cand, candidato{documento: uuid.NullUUID{UUID: d.DocumentoID, Valid: true}, nome: d.NomeFile})
			}
		}
		for _, v := range voci {
			if v.Stato != VocePronta || v.Duplicato != "" || v.Tipo != db.TipoDocumentoCad3d || (v.Estensione != "stp" && v.Estensione != "step") {
				continue
			}
			if v.Componente != nil && v.Componente.ComponenteID == c.ComponenteID {
				cand = append(cand, candidato{allegato: v.Allegato, nome: v.Nome})
			}
		}
		switch len(cand) {
		case 0:
			continue
		case 1:
			out = append(out, VoceStrutturale{Prodotto: c, Allegato: cand[0].allegato, Documento: cand[0].documento, Nome: cand[0].nome, Stato: VocePronta})
		default:
			nomi := make([]string, len(cand))
			for i, x := range cand {
				nomi[i] = x.nome
			}
			out = append(out, VoceStrutturale{Prodotto: c, Stato: VoceDecidere, Domande: []Domanda{{DomandaStrutturale,
				fmt.Sprintf("lo STEP strutturale di %s si sceglie fra %s", c.Codice, strings.Join(nomi, ", "))}}})
		}
	}
	return out
}

// etichettaTipo e' il nome breve di un tipo di documento per le frasi: 3D, 2D, DXF, gli altri con gli spazi.
func etichettaTipo(t db.TipoDocumento) string {
	switch t {
	case db.TipoDocumentoCad3d:
		return "3D"
	case db.TipoDocumentoDisegno2d:
		return "2D"
	case db.TipoDocumentoSviluppoDxf:
		return "DXF"
	}
	return strings.ReplaceAll(string(t), "_", " ")
}

// ------------------------------------------------------------------ dal database

// LeggiPianoFascicolo legge quello che serve al piano e lo calcola. Non scrive niente. Nella transazione di
// «Conferma Fascicolo» legge quello che la transazione vede: la conferma lavora sul piano di adesso.
func LeggiPianoFascicolo(ctx context.Context, q *db.Queries, thread uuid.UUID) (PianoFascicolo, error) {
	in, err := LeggiIngressoPiano(ctx, q, thread)
	if err != nil {
		return PianoFascicolo{}, err
	}
	return PianoDelFascicolo(in), nil
}

// LeggiIngressoPiano legge l'ingresso del piano.
func LeggiIngressoPiano(ctx context.Context, q *db.Queries, thread uuid.UUID) (IngressoPiano, error) {
	var in IngressoPiano
	tid := uuid.NullUUID{UUID: thread, Valid: true}
	allegati, err := q.ListAllegatiFascicolo(ctx, tid)
	if err != nil {
		return in, err
	}
	proposte, err := q.ListProposteDocumentoThread(ctx, tid)
	if err != nil {
		return in, err
	}
	lavoro, err := q.ListLavoroPendenteRfq(ctx, tid)
	if err != nil {
		return in, err
	}
	inLavoro := map[uuid.UUID]bool{}
	for _, l := range lavoro {
		inLavoro[l.AllegatoID] = true
	}
	perAllegato := map[uuid.UUID]db.DocumentoProposta{}
	for _, p := range proposte {
		perAllegato[p.AllegatoID] = p
	}
	for _, a := range allegati {
		p, c := perAllegato[a.AllegatoID]
		if !c || p.Stato != db.StatoPropostaAperta {
			continue
		}
		in.File = append(in.File, FileAperto{Allegato: a, Proposta: p, InLavoro: inLavoro[a.AllegatoID],
			Presente: a.PathStaging.Valid && a.PathStaging.String != "" && inStaging(a.PathStaging.String)})
	}
	if in.Componenti, err = q.ListComponentiThread(ctx, thread); err != nil {
		return in, err
	}
	if in.Documenti, err = q.ListDocumentiThread(ctx, thread); err != nil {
		return in, err
	}
	nodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return in, err
	}
	in.NomiFile = map[uuid.UUID]string{}
	for _, n := range nodi {
		in.Nodi = append(in.Nodi, n.ComponenteProposta)
		in.NomiFile[n.ComponenteProposta.AllegatoID] = n.NomeFile
	}
	rel, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return in, err
	}
	for _, r := range rel {
		in.Relazioni = append(in.Relazioni, r.RelazioneProposta)
		in.NomiFile[r.RelazioneProposta.AllegatoID] = r.NomeFile
	}
	in.Motore, _ = MotoreDellaRfq(ctx, q, thread) // senza cliente leggibile: nessun suffisso
	n, bloccata, err := WorkingBloccata(ctx, q, thread)
	if err != nil {
		return in, err
	}
	if bloccata {
		in.Bloccata = n
	}
	return in, nil
}
