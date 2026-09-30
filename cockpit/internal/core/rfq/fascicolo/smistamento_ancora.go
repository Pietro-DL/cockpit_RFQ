package fascicolo

// Il flusso ancorato al prodotto, passi 1 e 2 (Smistamento F8, addendum A5.13.2-A5.13.3; decisioni
// dell'utente del 27/09, «bis» e «ter»): i prodotti della RFQ e il loro portatore di struttura.
//
// Il flusso parte SOLO da cio' che una persona ha deciso: un codice della richiesta confermato al triage, o
// un prodotto finito creato da una persona. Mai da un prodotto nato da un'evidenza (scelta 3: quelli che la
// Struttura BOM mostra «da rivedere»). Per ogni prodotto cerca l'ancora: lo STEP autorizzato per lui, poi uno
// STEP la cui struttura lo dice (la radice). Il nome di un file TROVA e ORDINA i candidati, ma non ancora mai
// (P32): l'ancora vuole il contenuto. Senza ancora, per quel prodotto l'automatismo delle destinazioni
// tecniche si ferma (Domanda 4 = B); i file restano analizzati e si decidono a mano.
//
// Senza uno STEP, il disegno del prodotto con il codice del prodotto scritto dentro (il cartiglio, i metadati,
// il testo) da' un'ancora «piatta» (giro 4, fase 4.2; A5.13.3, A-P1…A-P3): lega quel PDF al prodotto, non i
// figli, e non arriva mai gia' scelta (scelta 2 della lista delle domande del giro 4). Un PDF senza testo, con il
// testo non letto o illeggibile non ancora, e il motivo lo dice.
//
// Tutto qui e' puro: legge lo StatoFlusso (le evidenze che il core ha gia' normalizzato) e non scrive
// niente. Il flusso non legge mai il JSON del worker: la struttura di uno STEP la conosce dalle righe di
// componente_proposta e relazione_proposta che ApplicaStruttura ne ha tratto, con le correzioni
// dell'operatore; il codice e il tipo di un file dalla sua valutazione (A5.14); il contenuto di un PDF dalle
// evidenze di EvidenzeContenutoPDF.

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// AlgoritmoFlusso e' la versione dell'algoritmo: entra nella firma dell'indice, e cambiarlo rifa' tutte le
// destinazioni.
const AlgoritmoFlusso = "flusso-ancorato-1"

// I livelli di un'ancora (A5.13.3). L'ancora «piatta» e' il codice del prodotto scritto nel suo PDF (A-P1,
// A-P2, A-P3): da' il prodotto, non i figli, e non e' un riferimento strutturale.
const (
	LivelloAutorizzata  = "autorizzata"
	LivelloPiena        = "piena"
	LivelloDaConfermare = "da_confermare"
	LivelloPiatta       = "piatta"
	LivelloInAttesa     = "in_attesa"
	LivelloAssente      = "assente"
)

// Lo stato dell'analisi di un file, per il flusso: se i fatti correnti ci sono, se arriveranno, o se non
// arriveranno da soli.
const (
	AnalisiCorrente = "corrente" // i fatti dell'analizzatore corrente ci sono
	AnalisiInCorso  = "in_corso" // download, estrazione o analisi pendente
	AnalisiFallita  = "fallita"  // l'ultima analisi con la chiave di oggi e' fallita: non riparte da sola
	AnalisiAssente  = "assente"  // niente fatti e niente lavoro in coda
)

// StatoFlusso e' tutto cio' che il flusso legge di una RFQ (LeggiStatoFlusso), gia' normalizzato: nessun
// campo e' un fatto grezzo del worker. Le destinazioni sono una funzione pura di questo stato (FP7).
type StatoFlusso struct {
	Motore *classificazione.Motore
	// ImprontaRegole e' lo sha256 delle regole del cliente: cambiano le regole, cambia la firma dell'indice.
	ImprontaRegole string
	Identificativi []db.IdentificativoThread
	Componenti     []db.Componente          // tutti, anche archiviati
	Relazioni      []db.ComponenteRelazione // gli archi attivi della working
	Nodi           []db.ComponenteProposta  // le righe della struttura degli STEP della RFQ
	Archi          []db.RelazioneProposta
	Dichiarazioni  Dichiarazioni
	File           []FileFlusso
	// BomCongelata: la working e' congelata; i candidati verso un componente non sono mai preselezionabili.
	BomCongelata bool

	der *derivati // calcolati una volta, alla prima domanda
}

// FileFlusso e' un file della RFQ come il flusso lo vede.
type FileFlusso struct {
	AllegatoID  uuid.UUID
	Sha         string
	Nome        string
	Estensione  string
	PathInterno string
	Zip         string // il nome dell'archivio che lo contiene, se e' una voce
	RicevutoIl  time.Time
	Contenitore bool // e' un archivio con delle voci: le voci si smistano una per una
	DelCliente  bool // fonte del cliente o del progetto (P27)
	Proposta    *PropostaFile
	Analisi     string // AnalisiCorrente | AnalisiInCorso | AnalisiFallita | AnalisiAssente
	// TestoPDF ed EvidenzePDF: lo stato del testo di un PDF (con la frase e l'OCR) e i codici letti nel suo
	// contenuto, normalizzati dal core dai fatti correnti (EvidenzeContenutoPDF). Vuoti per gli altri file.
	TestoPDF    TestoDelPDF
	EvidenzePDF []EvidenzaContenutoPDF
}

// PropostaFile e' la riga di documento_proposta del file, nelle parti che il flusso legge.
type PropostaFile struct {
	PropostaID uuid.UUID
	Aperta     bool // stato aperta: la sola su cui il flusso scrive
	Esclusa    bool // scartata da una persona
	// ComponenteID: una pre-assegnazione di prima (un gesto senza autore registrato, A5.14.3).
	ComponenteID uuid.NullUUID
	Valutazione  classificazione.Valutazione
	// IndiceFirma e Firma: quelle della destinazione scritta l'ultima volta ("" se non ce n'e' una).
	IndiceFirma, Firma string
}

// estensione del file, minuscola e senza punto.
func (f FileFlusso) ext() string {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(f.Estensione), "."))
	if e == "" {
		e = strings.ToLower(strings.TrimPrefix(path.Ext(f.Nome), "."))
	}
	return e
}

func (f FileFlusso) step() bool { e := f.ext(); return e == "stp" || e == "step" }
func (f FileFlusso) pdf() bool  { return f.ext() == "pdf" }

// nelFlusso: il file puo' portare struttura o ricevere una destinazione tecnica. Non un archivio, non un
// file escluso da una persona, non una fonte che non e' del cliente.
func (f FileFlusso) nelFlusso() bool {
	return f.DelCliente && !f.Contenitore && (f.Proposta == nil || !f.Proposta.Esclusa)
}

// ------------------------------------------------------------------ passo 1: i prodotti

// Prodotto e' un prodotto della RFQ da cui il flusso parte: il codice canonico (con le regole del cliente) e,
// se c'e', il componente finito che lo rappresenta.
type Prodotto struct {
	Codice         string        `json:"codice"`
	ComponenteID   uuid.NullUUID `json:"componente_id"`
	Identificativo bool          `json:"identificativo"` // un codice della richiesta confermato al triage
}

// ProdottiDellaRfq sono i prodotti CONFERMATI (A5.13.3, passo 1; scelta 3): i codici della richiesta che una
// persona ha confermato (identificativo_thread.confermato_da), con il loro componente se c'e', e i finiti
// attivi creati da una persona (origine manuale). Un finito nato da un'evidenza (un codice trovato, un nodo di
// uno STEP) il cui codice nessuno ha confermato non e' un prodotto per il flusso: la Struttura BOM lo mostra
// «da rivedere». Un codice della richiesta il cui componente e' stato archiviato resta fuori: toglierlo dalla
// BOM e' stata una decisione, e l'automatismo non la disfa. In ordine di codice.
func ProdottiDellaRfq(s *StatoFlusso) []Prodotto {
	d := s.derivati()
	visti := map[string]bool{}
	var out []Prodotto
	for _, i := range s.Identificativi {
		if !i.ConfermatoDa.Valid || strings.TrimSpace(i.Codice) == "" {
			continue
		}
		p := Prodotto{Codice: d.canonico(i.Codice), Identificativo: true}
		if c, ok := d.componentePerCodice(i.Codice); ok {
			if c.ArchiviatoIl != nil {
				continue
			}
			p.ComponenteID = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
		}
		if p.Codice == "" || visti[p.Codice] {
			continue
		}
		visti[p.Codice] = true
		out = append(out, p)
	}
	for _, c := range s.Componenti {
		if c.Tipo != db.TipoComponenteFinito || c.ArchiviatoIl != nil || c.Origine != db.OrigineComponenteManuale {
			continue
		}
		k := d.canonico(c.Codice)
		if k == "" || visti[k] {
			continue
		}
		visti[k] = true
		out = append(out, Prodotto{Codice: k, ComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Codice < out[j].Codice })
	return out
}

// ------------------------------------------------------------------ passo 2a: la compatibilita' per nome

// I pesi della compatibilita' per nome (A5.13.3, A-N1…A-N5): ordinano i candidati e decidono quali analisi
// passano prima; non sono uno score di una dimensione e non ancorano.
const (
	pesoNomeUguale   = 60 // A-N1
	pesoNomeVicino   = 40 // A-N2
	pesoNomeContiene = 25 // A-N3
	pesoCartellaZip  = 15 // A-N4
	pesoZipProdotto  = 10 // A-N5
)

// Compatibilita' e' quanto il nome di un file dice di un prodotto: il peso d'ordine, le regole, i motivi e le
// discordanze (lettera_finale, suffisso per un nome vicino).
type Compatibilita struct {
	Peso        int      `json:"peso"`
	Regole      []string `json:"regole"`
	Motivi      []string `json:"motivi"`
	Discordanze []string `json:"discordanze,omitempty"`
}

// Compatibile dice se il nome dice qualcosa del prodotto.
func (c Compatibilita) Compatibile() bool { return c.Peso > 0 }

// codiciDelNome sono le letture del nome di un file: il nome intero, tolta la rev e il suffisso decorativo del
// cliente, quando e' un codice; altrimenti i codici che il nome cita. Solo il nome del file (P15): ogni copia
// rilegge il suo.
func codiciDelNome(nome string, m *classificazione.Motore) (codice string, citati []string) {
	p := classificazione.PropostaDaNome(nome, 0, "")
	if p.Codice != "" {
		k, _ := m.Canonico(p.Codice, p.Rev)
		return strings.ToUpper(strings.TrimSpace(k)), nil
	}
	for _, c := range p.CodiciNelNome {
		if k := strings.ToUpper(strings.TrimSpace(m.CanonicoNome(c))); k != "" {
			citati = append(citati, k)
		}
	}
	return "", citati
}

// CompatibilitaNome dice quanto il nome di un file (e la cartella e l'archivio da cui viene) e' compatibile con
// il prodotto p. Pura. «7120010» non e' compatibile con «7120001», ne' «7120011» con «7120012» (P4).
func CompatibilitaNome(nome, pathInterno, zip, p string, m *classificazione.Motore) Compatibilita {
	var c Compatibilita
	p = strings.ToUpper(strings.TrimSpace(p))
	if p == "" {
		return c
	}
	cod, citati := codiciDelNome(nome, m)
	switch {
	case cod == p:
		c.Peso, c.Regole, c.Motivi = pesoNomeUguale, []string{"A-N1"}, []string{"il nome dice " + p}
	case cod != "" && vicino(cod, p, m) != "":
		motivo := vicino(cod, p, m)
		c.Peso, c.Regole = pesoNomeVicino, []string{"A-N2"}
		c.Motivi = []string{fmt.Sprintf("il nome dice %s, vicino a %s (%s)", cod, p, motivo)}
		c.Discordanze = discordanzeVicino(motivo)
	case nomeContiene(cod, citati, p):
		c.Peso, c.Regole, c.Motivi = pesoNomeContiene, []string{"A-N3"}, []string{"il nome contiene " + p}
	}
	if pathInterno != "" {
		dir := path.Dir(strings.ReplaceAll(pathInterno, "\\", "/"))
		for _, seg := range strings.Split(dir, "/") {
			if strings.EqualFold(strings.TrimSpace(seg), p) {
				c.Peso += pesoCartellaZip
				c.Regole = append(c.Regole, "A-N4")
				c.Motivi = append(c.Motivi, "nello ZIP sta nella cartella "+p)
				break
			}
		}
	}
	if zip != "" {
		if z := CompatibilitaNome(zip, "", "", p, m); z.Peso == pesoNomeUguale || z.Peso == pesoNomeVicino {
			c.Peso += pesoZipProdotto
			c.Regole = append(c.Regole, "A-N5")
			c.Motivi = append(c.Motivi, "viene dallo ZIP "+zip)
		}
	}
	return c
}

// nomeContiene: p e' fra i codici citati nel nome, o il codice del nome e' p seguito da un separatore e da altro
// («7120001_STAFFA_SX»).
func nomeContiene(cod string, citati []string, p string) bool {
	for _, c := range citati {
		if c == p {
			return true
		}
	}
	if len(cod) > len(p)+1 && strings.HasPrefix(cod, p) {
		return separatore(rune(cod[len(p)]))
	}
	return false
}

// vicino e' il motivo per cui due codici diversi sono quasi uguali (P4), o "".
func vicino(a, b string, m *classificazione.Motore) string {
	if ok, motivo := QuasiUguale(a, b, m); ok {
		return motivo
	}
	return ""
}

// discordanzeVicino sono le chiavi del catalogo (A5.5) che un «quasi uguale» porta con se'.
func discordanzeVicino(motivo string) []string {
	switch motivo {
	case VicinoLetteraFinale:
		return []string{DiscLetteraFinale}
	case VicinoSuffisso, VicinoSuffissoCliente:
		return []string{DiscSuffisso}
	}
	return nil
}

// ------------------------------------------------------------------ passo 2b: l'ancora

// I pesi dell'ancora per contenuto (A5.13.3, A-S1…A-S5): ordinano i file che aspettano un'autorizzazione e
// non si mostrano come confidenza di una dimensione del file.
const (
	pesoRadiceUguale   = 100 // A-S1
	pesoRadiceFamiglia = 90  // A-S2
	pesoRadiceVicina   = 60  // A-S3
	pesoRadiceNomina   = 40  // A-S4
	pesoPiuRadici      = 40  // A-S5
	pesoCoperturaMax   = 20
)

// Ancora e' il portatore di struttura di un prodotto (o, nell'indice, di un sottoassieme).
type Ancora struct {
	Prodotto    string      `json:"prodotto"`
	Livello     string      `json:"livello"`
	Portatori   []Portatore `json:"portatori,omitempty"`
	Motivi      []string    `json:"motivi,omitempty"`
	Discordanze []string    `json:"discordanze,omitempty"`
	// SoloGuida: l'ancora di un commerciale nell'indice. Il suo STEP si legge e si mostra, ma non e' una
	// sorgente strutturale finche' una persona non cambia il tipo (Domanda 5 = B): i suoi nodi sono guida.
	SoloGuida bool `json:"solo_guida,omitempty"`
	// testiPdf: i PDF che il passo 4 ha guardato per il prodotto, con lo stato del loro testo («sha:stato»).
	// Entrano nella firma dell'indice: quando un PDF prende il testo i motivi dell'ancora cambiano anche se
	// l'ancora no, e le destinazioni che li riportano si rifanno.
	testiPdf []string
}

// Ancorato dice se l'ancora da' una struttura da cui partire (anche da confermare: allora i candidati che ne
// dipendono portano la discordanza e non si preselezionano).
func (a Ancora) Ancorato() bool {
	return a.Livello == LivelloAutorizzata || a.Livello == LivelloPiena || a.Livello == LivelloDaConfermare
}

// Portatore e' un file che porta (o potrebbe portare) la struttura di un pezzo.
type Portatore struct {
	AllegatoID  uuid.UUID `json:"allegato_id"`
	Sha         string    `json:"-"`
	Nome        string    `json:"file"`
	Peso        int       `json:"peso"`
	Livello     string    `json:"livello"`
	Regole      []string  `json:"regole"`
	Discordanze []string  `json:"discordanze,omitempty"`
	// Sorgenti: le chiavi dei nodi che SONO il pezzo nel file (la radice, o le sorgenti autorizzate).
	Sorgenti   []string  `json:"-"`
	ricevutoIl time.Time // ordine deterministico: peso, poi ricevuto_il, poi allegato
}

// Ancore trova l'ancora di ogni prodotto (A5.13.3, passo 2b), nell'ordine:
//  1. autorizzata: il prodotto ha uno STEP autorizzato valido (A5.4); gli altri candidati restano alternative;
//  2. STEP per contenuto: un file STEP della RFQ, nel flusso, con la struttura letta (le righe di proposta), la
//     cui radice e' il prodotto (A-S1, A-S2: piena), gli e' vicina (A-S3), lo nomina (A-S4), o e' una di piu'
//     radici (A-S5): da confermare. Esclusa una radice che e' un nodo di un altro STEP (e' lo STEP di un
//     sottoassieme) e un file che il nome dice del prodotto ma la cui radice non c'entra (fonti_diverse);
//  3. in attesa: uno STEP compatibile per nome sta ancora scendendo o si sta analizzando. Uno la cui analisi
//     e' fallita non e' attesa: lo si dice, e si passa oltre;
//  4. piatta: un PDF del prodotto (un disegno, o un PDF di cui il tipo non si sa) il cui testo dice il
//     prodotto in modo indipendente dal nome del file: nel cartiglio (A-P1), solo nei metadati (A-P2,
//     solo_metadati), altrove nel testo (A-P3, da confermare). Da' il prodotto, non i figli: gli altri file
//     tecnici restano «manca un riferimento strutturale», e niente si preseleziona (ancorePdf);
//  5. in attesa: un PDF compatibile per nome si sta ancora analizzando, e il suo testo puo' dare l'ancora
//     piatta;
//  6. assente: per il prodotto l'automatismo delle destinazioni tecniche si ferma. Perche' i PDF compatibili
//     per nome non ancorano lo dicono i motivi (perchePdfNonAncora).
//
// Pura.
func Ancore(prodotti []Prodotto, s *StatoFlusso) map[string]Ancora {
	d := s.derivati()
	out := map[string]Ancora{}
	for _, p := range prodotti {
		a := Ancora{Prodotto: p.Codice}
		candidati, motivi := d.ancorePerContenuto(p.Codice, nil)
		a.Motivi = append(a.Motivi, motivi...)
		if p.ComponenteID.Valid {
			if x, ok := s.Dichiarazioni.Di(p.ComponenteID.UUID); ok {
				a.Livello = LivelloAutorizzata
				aut := Portatore{Sha: x.Sha256, Nome: x.NomeFile, Livello: LivelloAutorizzata, Regole: []string{"autorizzata"},
					Sorgenti: x.ChiaviSorgenti()}
				if f, ok := d.primoFile(x.Sha256); ok {
					aut.AllegatoID, aut.Nome, aut.ricevutoIl = f.AllegatoID, f.Nome, f.RicevutoIl
				}
				a.Portatori = append([]Portatore{aut}, alternative(candidati, x.Sha256)...)
				a.Motivi = append([]string{fmt.Sprintf("%s è autorizzato per %s", aut.Nome, p.Codice)}, a.Motivi...)
				out[p.Codice] = a
				continue
			}
		}
		if piene := conLivello(candidati, LivelloPiena); len(piene) > 0 {
			a.Livello, a.Portatori = LivelloPiena, piene
			if shaDiversi(piene) > 1 {
				a.Discordanze = append(a.Discordanze, DiscAncoreConcorrenti)
				a.Motivi = append(a.Motivi, fmt.Sprintf("%d STEP hanno la radice %s: sceglie l'autorizzazione", shaDiversi(piene), p.Codice))
			}
			out[p.Codice] = a
			continue
		}
		if dc := conLivello(candidati, LivelloDaConfermare); len(dc) > 0 {
			a.Livello, a.Portatori = LivelloDaConfermare, dc
			a.Discordanze = append(a.Discordanze, DiscAncoraDaConfermare)
			out[p.Codice] = a
			continue
		}
		// nessuno STEP la cui struttura dica il prodotto: si guarda se uno compatibile per nome deve ancora
		// arrivare (A5.13.3, punto 3)
		attesa, perso := d.stepInAttesa(p.Codice)
		a.Motivi = append(a.Motivi, perso...)
		if len(attesa) > 0 {
			a.Livello, a.Portatori = LivelloInAttesa, attesa
			for _, x := range attesa {
				a.Motivi = append(a.Motivi, fmt.Sprintf("lo STEP %s, compatibile per nome, è ancora in analisi", x.Nome))
			}
			out[p.Codice] = a
			continue
		}
		// il disegno del prodotto: l'ancora piatta (A5.13.3, passo 4)
		piatte, motiviPdf := d.ancorePdf(p.Codice)
		a.testiPdf = d.testiPdf(p.Codice)
		if len(piatte) > 0 {
			a.Livello, a.Portatori = LivelloPiatta, piatte
			a.Discordanze = append(a.Discordanze, piatte[0].Discordanze...)
			a.Motivi = append(a.Motivi, motiviPdf...)
			a.Motivi = append(a.Motivi, d.perchePdfNonAncora(p.Codice, piatte)...)
			a.Motivi = append(a.Motivi, fmt.Sprintf("nessuno STEP compatibile con %s: il disegno lega il file al prodotto, non ai suoi figli; "+
				"l'automatismo delle destinazioni tecniche si ferma", p.Codice))
			out[p.Codice] = a
			continue
		}
		// un PDF compatibile per nome si sta ancora leggendo: il suo testo puo' dare l'ancora (passo 5)
		if attesa := d.pdfInAttesa(p.Codice); len(attesa) > 0 {
			a.Livello, a.Portatori = LivelloInAttesa, attesa
			for _, x := range attesa {
				a.Motivi = append(a.Motivi, fmt.Sprintf("il PDF %s, compatibile per nome, è ancora in analisi", x.Nome))
			}
			out[p.Codice] = a
			continue
		}
		a.Livello = LivelloAssente
		a.Motivi = append(a.Motivi, d.perchePdfNonAncora(p.Codice, nil)...)
		a.Motivi = append(a.Motivi, fmt.Sprintf("nessuno STEP compatibile con %s: l'automatismo delle destinazioni tecniche si ferma", p.Codice))
		out[p.Codice] = a
	}
	return out
}

// ancorePerContenuto sono gli STEP della RFQ (nel flusso, con la struttura letta, non ancora usati) la cui
// struttura dice il pezzo k, con la regola e il peso; e i motivi di chi il nome direbbe e la radice no.
func (d *derivati) ancorePerContenuto(k string, usati map[string]bool) ([]Portatore, []string) {
	var out []Portatore
	var motivi []string
	for _, sha := range d.shaStep {
		if usati[sha] {
			continue
		}
		st := d.strutture[sha]
		f, _ := d.primoFile(sha)
		p, rc, ok := d.ancoraDelFile(st, k)
		if ok && usati == nil && d.radiceAltrove(st, rc) {
			// la radice e' un nodo di un altro STEP della RFQ: e' lo STEP di un sottoassieme, e non ancora il
			// prodotto (A5.13.3); l'indice lo usa come ancora di quel sottoassieme
			continue
		}
		if !ok {
			// il nome dice k (A-N1, A-N2) ma la radice non c'entra: nessuna ancora, e lo si dice (157)
			for _, ff := range d.fileStep[sha] {
				if c := CompatibilitaNome(ff.Nome, ff.PathInterno, ff.Zip, k, d.m); c.Peso >= pesoNomeVicino && len(st.Radici) > 0 {
					motivi = append(motivi, fmt.Sprintf("il nome di %s dice %s, ma la radice dello STEP è %s: non ancora (%s)",
						ff.Nome, k, strings.Join(d.codiciRadici(st), ", "), DiscFontiDiverse))
					break
				}
			}
			continue
		}
		p.AllegatoID, p.Nome, p.ricevutoIl = f.AllegatoID, f.Nome, f.RicevutoIl
		p.Peso += d.copertura(st)
		out = append(out, p)
	}
	ordinaPortatori(out)
	return out, motivi
}

// ancoraDelFile dice se la struttura st e' un'ancora del pezzo k, con quale regola (A-S1…A-S5), e il codice
// della radice che la regola ha guardato.
func (d *derivati) ancoraDelFile(st *strutturaStep, k string) (Portatore, string, bool) {
	p := Portatore{Sha: st.Sha}
	if len(st.Radici) == 0 {
		return p, "", false
	}
	if len(st.Radici) > 1 {
		for _, r := range st.Radici {
			if d.codiceNodo(st.Nodi[r]) == k {
				p.Peso, p.Livello, p.Regole, p.Sorgenti = pesoPiuRadici, LivelloDaConfermare, []string{"A-S5"}, []string{r}
				p.Discordanze = []string{DiscPiuRadici}
				return p, k, true
			}
		}
		return p, "", false
	}
	r := st.Radici[0]
	n := st.Nodi[r]
	rc := d.codiceNodo(n)
	p.Sorgenti = []string{r}
	switch {
	case rc == k && (d.canonico(n.IDGrezzo) == k || d.canonico(n.NomeGrezzo) == k):
		p.Peso, p.Livello, p.Regole = pesoRadiceUguale, LivelloPiena, []string{"A-S1"}
	case rc == k:
		// il codice l'ha separato una famiglia del cliente dal testo del PRODUCT («7120001-01»)
		p.Peso, p.Livello, p.Regole = pesoRadiceFamiglia, LivelloPiena, []string{"A-S2"}
		if d.revDelNomeDiversa(st, n) {
			p.Discordanze = []string{DiscRevFonti}
		}
	case rc != "" && vicino(rc, k, d.m) != "":
		p.Peso, p.Livello, p.Regole = pesoRadiceVicina, LivelloDaConfermare, []string{"A-S3"}
		p.Discordanze = append([]string{DiscRadiceDiversa}, discordanzeVicino(vicino(rc, k, d.m))...)
	case nomina(n, k):
		p.Peso, p.Livello, p.Regole = pesoRadiceNomina, LivelloDaConfermare, []string{"A-S4"}
		p.Discordanze = []string{DiscFontiDiverse}
	default:
		return p, rc, false
	}
	return p, rc, true
}

// radiceAltrove: il codice della radice e' un nodo (non la radice) di un altro STEP della RFQ. Allora il file
// e' lo STEP di un sottoassieme, e non ancora il prodotto (A5.13.3).
func (d *derivati) radiceAltrove(st *strutturaStep, codice string) bool {
	if codice == "" {
		return false
	}
	for _, sha := range d.shaStep {
		if sha == st.Sha {
			continue
		}
		o := d.strutture[sha]
		for k, n := range o.Nodi {
			if len(o.Padri[k]) > 0 && d.codiceNodo(n) == codice {
				return true
			}
		}
	}
	return false
}

// nomina: il prodotto sta nell'id, nel nome o nella descrizione della radice, come parola, ma non e' il suo
// codice (A-S4).
func nomina(n db.ComponenteProposta, k string) bool {
	for _, t := range []string{n.IDGrezzo, n.NomeGrezzo, n.Descrizione.String} {
		t = strings.ToUpper(t)
		for i := strings.Index(t, k); i >= 0; {
			prima := i == 0 || !alfanumerico(rune(t[i-1]))
			fine := i+len(k) >= len(t) || !alfanumerico(rune(t[i+len(k)]))
			if prima && fine {
				return true
			}
			j := strings.Index(t[i+1:], k)
			if j < 0 {
				break
			}
			i += 1 + j
		}
	}
	return false
}

func alfanumerico(r rune) bool { return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') }

// revDelNomeDiversa: la rev della radice e quella che il nome del file separa sono diverse (rev_fonti).
func (d *derivati) revDelNomeDiversa(st *strutturaStep, n db.ComponenteProposta) bool {
	rv := strings.TrimSpace(n.Rev.String)
	if rv == "" {
		return false
	}
	for _, f := range d.fileStep[st.Sha] {
		p := classificazione.PropostaDaNome(f.Nome, 0, "")
		if p.Rev != "" && !strings.EqualFold(p.Rev, rv) {
			return true
		}
	}
	return false
}

// copertura: quanta parte dei codici dei file della RFQ (dai loro nomi) sono nodi della struttura, fino a
// pesoCoperturaMax. Ordina due STEP concorrenti: quello che spiega piu' file viene prima.
func (d *derivati) copertura(st *strutturaStep) int {
	if len(d.codiciDeiFile) == 0 {
		return 0
	}
	nodi := map[string]bool{}
	for _, n := range st.Nodi {
		if c := d.codiceNodo(n); c != "" {
			nodi[c] = true
		}
	}
	dentro := 0
	for c := range d.codiciDeiFile {
		if nodi[c] {
			dentro++
		}
	}
	return pesoCoperturaMax * dentro / len(d.codiciDeiFile)
}

// stepInAttesa sono gli STEP compatibili per nome con k che non hanno ancora una struttura letta e che
// stanno scendendo o si stanno analizzando; perso dice quelli la cui analisi e' fallita.
func (d *derivati) stepInAttesa(k string) (attesa []Portatore, perso []string) {
	for _, f := range d.s.File {
		if !f.step() || !f.nelFlusso() || d.strutture[f.Sha] != nil {
			continue
		}
		c := CompatibilitaNome(f.Nome, f.PathInterno, f.Zip, k, d.m)
		if !c.Compatibile() {
			continue
		}
		switch f.Analisi {
		case AnalisiInCorso:
			attesa = append(attesa, Portatore{AllegatoID: f.AllegatoID, Sha: f.Sha, Nome: f.Nome, Peso: c.Peso, Livello: LivelloInAttesa,
				Regole: c.Regole, ricevutoIl: f.RicevutoIl})
		case AnalisiFallita:
			perso = append(perso, fmt.Sprintf("lo STEP candidato %s non si è potuto leggere", f.Nome))
		}
	}
	ordinaPortatori(attesa)
	return attesa, perso
}

// I pesi dell'ancora piatta (A5.13.3, A-P1…A-P3): ordinano i PDF del prodotto, e non si mostrano come
// confidenza di una dimensione del file.
const (
	pesoPdfCartiglio = 80 // A-P1
	pesoPdfNome      = 10 // A-P1 con il nome compatibile (A-N1, A-N2)
	pesoPdfMetadati  = 50 // A-P2
	pesoPdfTesto     = 30 // A-P3
)

// pdfDelProdotto: il PDF puo' essere il disegno del prodotto, cioe' un disegno o un PDF di cui il tipo non si
// sa; mai un capitolato, un'offerta o una distinta che nominano il prodotto (A5.13.3, passo 4).
func pdfDelProdotto(f FileFlusso) bool {
	if f.Proposta == nil {
		return true
	}
	switch f.Proposta.Valutazione.Tipo.Valore {
	case "", "disegno_2d", "da_determinare":
		return true
	}
	return false
}

// dicePdf dice dove il testo del PDF f porta il pezzo k in modo indipendente dal nome del file (non un indizio
// dell'OCR, non una lettura che ripete il nome: P32): nel cartiglio, nei metadati, altrove nel testo. Il cartiglio
// di un testo letto con il worker di prima conta come il resto del testo (DaRileggere, fase 4.6r): A-P3, da
// confermare, finche' non si rianalizza.
func (d *derivati) dicePdf(f FileFlusso, k string) (cartiglio, metadati, testo bool) {
	for _, e := range f.EvidenzePDF {
		if e.Indizio || e.DipendeDaNome || d.canonico(e.Codice) != k {
			continue
		}
		switch {
		case e.DalCartiglio():
			cartiglio = true
		case e.Fonte == FontePDFMetadati:
			metadati = true
		case e.Fonte == FontePDFTesto, e.DaRileggere:
			testo = true
		}
	}
	return
}

// ancorePdf sono i PDF della RFQ che danno l'ancora piatta del prodotto k (A5.13.3, passo 4), con i motivi: un
// PDF del prodotto (pdfDelProdotto), nel flusso, con il testo letto, che dice k nel cartiglio (A-P1, 80, +10 con
// il nome compatibile), solo nei metadati (A-P2, 50, solo_metadati) o altrove nel testo (A-P3, 30,
// ancora_da_confermare), sempre in modo indipendente dal nome (dicePdf). Il cartiglio vale anche con un codice
// generico: l'ancora piatta dice soltanto «questo e' il disegno del prodotto», e non si preseleziona mai.
// Ordinati per peso, poi ricevuto_il, poi allegato.
func (d *derivati) ancorePdf(k string) ([]Portatore, []string) {
	var out []Portatore
	motivo := map[uuid.UUID]string{}
	for _, f := range d.s.File {
		if !f.pdf() || !f.nelFlusso() || !pdfDelProdotto(f) || !f.TestoPDF.Letto() {
			continue
		}
		p := Portatore{AllegatoID: f.AllegatoID, Sha: f.Sha, Nome: f.Nome, Livello: LivelloPiatta, ricevutoIl: f.RicevutoIl}
		switch cartiglio, metadati, testo := d.dicePdf(f, k); {
		case cartiglio:
			p.Peso, p.Regole = pesoPdfCartiglio, []string{"A-P1"}
			c := CompatibilitaNome(f.Nome, f.PathInterno, f.Zip, k, d.m)
			for _, r := range []string{"A-N1", "A-N2"} {
				if contiene(c.Regole, r) {
					p.Peso += pesoPdfNome
					p.Regole = append(p.Regole, r)
					break
				}
			}
			motivo[f.AllegatoID] = fmt.Sprintf("il cartiglio di %s dice %s (A-P1): è il disegno del prodotto", f.Nome, k)
		case metadati:
			p.Peso, p.Regole, p.Discordanze = pesoPdfMetadati, []string{"A-P2"}, []string{DiscSoloMetadati}
			motivo[f.AllegatoID] = fmt.Sprintf("%s dice %s solo nei metadati (A-P2): un'ancora debole (%s)", f.Nome, k, DiscSoloMetadati)
		case testo:
			p.Peso, p.Regole, p.Discordanze = pesoPdfTesto, []string{"A-P3"}, []string{DiscAncoraDaConfermare}
			motivo[f.AllegatoID] = fmt.Sprintf("il testo di %s cita %s fuori dal cartiglio (A-P3): da confermare", f.Nome, k)
			if f.TestoPDF.DaRileggere {
				// il cartiglio di un testo di prima conta come il resto del testo (dicePdf, fase 4.6r)
				motivo[f.AllegatoID] = fmt.Sprintf("il testo di %s cita %s, ma è stato letto con il worker di prima (A-P3): da confermare, "+
					"o da rianalizzare", f.Nome, k)
			}
		default:
			continue
		}
		out = append(out, p)
	}
	ordinaPortatori(out)
	var motivi []string
	for _, p := range out {
		motivi = append(motivi, motivo[p.AllegatoID])
	}
	return out, motivi
}

// pdfInAttesa sono i PDF compatibili per nome con k il cui testo non c'e' ancora e che si stanno scaricando o
// analizzando (A5.13.3, passo 5): quando arrivano possono dare l'ancora piatta.
func (d *derivati) pdfInAttesa(k string) []Portatore {
	var out []Portatore
	for _, f := range d.s.File {
		if !f.pdf() || !f.nelFlusso() || !pdfDelProdotto(f) || f.TestoPDF.Letto() || f.Analisi != AnalisiInCorso {
			continue
		}
		if c := CompatibilitaNome(f.Nome, f.PathInterno, f.Zip, k, d.m); c.Compatibile() {
			out = append(out, Portatore{AllegatoID: f.AllegatoID, Sha: f.Sha, Nome: f.Nome, Peso: c.Peso, Livello: LivelloInAttesa,
				Regole: c.Regole, ricevutoIl: f.RicevutoIl})
		}
	}
	ordinaPortatori(out)
	return out
}

// pdfDaGuardare: il PDF f conta per l'ancora del prodotto k: e' nel flusso e il suo nome e' compatibile con k,
// o il suo testo lo dice (anche solo con un indizio dell'OCR, o ripetendo il nome).
func (d *derivati) pdfDaGuardare(f FileFlusso, k string) bool {
	if !f.pdf() || !f.nelFlusso() {
		return false
	}
	if CompatibilitaNome(f.Nome, f.PathInterno, f.Zip, k, d.m).Compatibile() {
		return true
	}
	for _, e := range f.EvidenzePDF {
		if d.canonico(e.Codice) == k {
			return true
		}
	}
	return false
}

// testiPdf sono i PDF che contano per l'ancora di k, con lo stato del loro testo (Ancora.testiPdf) e, per un testo
// letto con il worker di prima, il segno «da rileggere» (fase 4.6r): la rianalisi lascia lo stato «letto», ma
// cambia quello che le destinazioni dicono del PDF, e la firma deve cambiare.
func (d *derivati) testiPdf(k string) []string {
	var out []string
	for _, f := range d.s.File {
		if d.pdfDaGuardare(f, k) {
			s := f.Sha + ":" + f.TestoPDF.Stato
			if f.TestoPDF.DaRileggere {
				s += ":da_rileggere"
			}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// perchePdfNonAncora dice perche' i PDF che contano per il prodotto k (pdfDaGuardare), fuori da quelli che lo
// ancorano, non ancorano: il testo che non c'e' (la frase del suo stato: senza testo, non letto, illeggibile, da
// analizzare, la stessa della valutazione e della schermata), il tipo che non e' un disegno, un indizio
// dell'OCR, una lettura che ripete il nome, o un testo che non dice k (con la frase del testo letto con il
// worker di prima, fase 4.6r). Il nome da solo non ancora mai (P32).
func (d *derivati) perchePdfNonAncora(k string, ancorano []Portatore) []string {
	usato := map[uuid.UUID]bool{}
	for _, p := range ancorano {
		usato[p.AllegatoID] = true
	}
	var out []string
	for _, f := range d.s.File {
		if !d.pdfDaGuardare(f, k) || usato[f.AllegatoID] {
			continue
		}
		ocr, ripete := false, false
		for _, e := range f.EvidenzePDF {
			if d.canonico(e.Codice) == k {
				ocr = ocr || e.Indizio
				ripete = ripete || (!e.Indizio && e.DipendeDaNome)
			}
		}
		switch {
		case f.TestoPDF.Stato == "":
			out = append(out, fmt.Sprintf("%s: il nome da solo non ancora", f.Nome))
		case !f.TestoPDF.Letto():
			out = append(out, fmt.Sprintf("%s: %s; non ancora %s", f.Nome, f.TestoPDF.Frase, k))
		case !pdfDelProdotto(f):
			out = append(out, fmt.Sprintf("%s è un documento (%s), non il disegno del prodotto: non ancora %s", f.Nome,
				f.Proposta.Valutazione.Tipo.Valore, k))
		case ocr:
			out = append(out, fmt.Sprintf("%s: l'OCR legge %s, un indizio: non ancora", f.Nome, k))
		case ripete:
			out = append(out, fmt.Sprintf("%s: il testo dice %s solo ripetendo il nome del file, e il nome da solo non ancora", f.Nome, k))
		case f.TestoPDF.DaRileggere:
			// il testo di prima (fase 4.6r) puo' non vedere il campo del codice: lo si dice
			out = append(out, fmt.Sprintf("il testo di %s non dice %s (%s): il nome da solo non ancora", f.Nome, k, f.TestoPDF.Frase))
		default:
			out = append(out, fmt.Sprintf("il testo di %s non dice %s: il nome da solo non ancora", f.Nome, k))
		}
	}
	return out
}

func alternative(candidati []Portatore, sha string) []Portatore {
	var out []Portatore
	for _, c := range candidati {
		if c.Sha != sha {
			out = append(out, c)
		}
	}
	return out
}

func conLivello(pp []Portatore, livello string) []Portatore {
	var out []Portatore
	for _, p := range pp {
		if p.Livello == livello {
			out = append(out, p)
		}
	}
	return out
}

func shaDiversi(pp []Portatore) int {
	visti := map[string]bool{}
	for _, p := range pp {
		visti[p.Sha] = true
	}
	return len(visti)
}

// ordinaPortatori: peso, poi ricevuto_il, poi allegato (A5.13.3).
func ordinaPortatori(pp []Portatore) {
	sort.SliceStable(pp, func(i, j int) bool {
		if pp[i].Peso != pp[j].Peso {
			return pp[i].Peso > pp[j].Peso
		}
		if !pp[i].ricevutoIl.Equal(pp[j].ricevutoIl) {
			return pp[i].ricevutoIl.Before(pp[j].ricevutoIl)
		}
		return pp[i].AllegatoID.String() < pp[j].AllegatoID.String()
	})
}
