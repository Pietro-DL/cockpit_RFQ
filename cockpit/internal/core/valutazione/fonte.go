package valutazione

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// StatoFonte: l'asse 2 di un prodotto, la fonte strutturale STEP (M1, R57, R65; contratto §1.0 riga 2, §1.2).
// «Candidato» è il singolo file, mai un quarto stato. confermata viene solo dal gesto 3 salvato (nessuna conferma
// implicita, contratto §0 n.6).
type StatoFonte string

const (
	FonteAssente            StatoFonte = "assente"
	FonteInAttesaDiConferma StatoFonte = "in_attesa_di_conferma"
	FonteConfermata         StatoFonte = "confermata"
)

// MotivoFonte: il motivo tecnico, separato dallo stato (6.0.3; R65 A, la tabella di corrispondenza; contratto §2.3,
// con step_presente_non_analizzato di T-E1-09).
type MotivoFonte string

const (
	MotivoFonteEstrazioneRiuscita        MotivoFonte = "estrazione_riuscita"          // presente_analizzato
	MotivoFonteDatiInsufficienti         MotivoFonte = "dati_insufficienti"           // presente_parziale: il grafo non è completo
	MotivoFonteAnalisiInCorso            MotivoFonte = "analisi_in_corso"             // lavoro pendente per lo STEP
	MotivoFonteNonAnalizzata             MotivoFonte = "non_analizzata"               // presente_non_analizzato senza lavoro pendente
	MotivoFonteRiferimentoSuperato       MotivoFonte = "riferimento_superato"         // lo STEP confermato è stato sostituito
	MotivoFonteDaScegliere               MotivoFonte = "da_scegliere"                 // STEP correnti sul prodotto, nessuno scelto
	MotivoFonteProposta3DAperta          MotivoFonte = "proposta_3d_aperta"           // da_confermare: una proposta aperta di 3D
	MotivoFonteIndicataSulPortale        MotivoFonte = "indicata_sul_portale"         // sul_portale
	MotivoFonteSoloAltro3D               MotivoFonte = "solo_altro_3d"                // un 3D che non è uno STEP non è fonte
	MotivoFonteNessunaFonte              MotivoFonte = "nessuna_fonte"                // mancante
	MotivoFonteDocumentoCandidato        MotivoFonte = "documento_candidato"          // un candidato del motore A, con la radice
	MotivoFonteEstrazioneFallita         MotivoFonte = "estrazione_fallita"           // fatti in errore
	MotivoFonteSoloNodoInterno           MotivoFonte = "solo_nodo_interno"            // il prodotto è solo un nodo interno (R76 b A)
	MotivoFonteProdottoSenzaComponente   MotivoFonte = "prodotto_senza_componente"    // target senza riga componente (T-B0-07)
	MotivoFonteNonDeterminabile          MotivoFonte = "non_determinabile"            // non si calcola (T-12)
	MotivoFonteStepPresenteNonAnalizzato MotivoFonte = "step_presente_non_analizzato" // uno STEP del thread senza fatti (T-E1-09)
)

// FonteProdotto: la fonte strutturale di un prodotto (contratto §1.2, §2.3).
//   - Stato e Motivo: la tabella di R65 sugli esiti di v_step_prodotto, il gesto 3, i candidati del motore A
//     (T-B0-07, R76 b), l'ordine dei motivi di T-E1-09. Con Calcolata falsa lo stato non c'è (T-12: non è un
//     quarto stato) e il motivo è non_determinabile.
//   - EsitoVista e MotivoSQL: l'esito e il motivo_parziale della vista, sempre visibili accanto.
//   - Riferimento: il gesto 3 (step_strutturale_id, con la marcatura se c'è: T-B0-08).
//   - Candidati: i documenti candidati (M1): gli STEP correnti del prodotto non scelti, la proposta aperta di uno
//     STEP, gli STEP del thread la cui radice ha la base del prodotto (R76 b A). Un candidato non cambia mai una
//     fonte confermata (T-B0-07).
//   - Estrazione: l'estrazione dei fatti del riferimento; vuota senza riferimento (ogni candidato porta la sua).
//   - DerogaStruttura: la deroga strutturale della vista, se c'è.
type FonteProdotto struct {
	Stato           StatoFonte                      `json:"stato,omitempty"`
	Motivo          MotivoFonte                     `json:"motivo"`
	MotivoSQL       string                          `json:"motivo_sql,omitempty"`
	EsitoVista      string                          `json:"esito_vista,omitempty"`
	Riferimento     *ancoraggio.RiferimentoFonte    `json:"riferimento,omitempty"`
	Candidati       []ancoraggio.DocumentoCandidato `json:"candidati,omitempty"`
	Estrazione      ancoraggio.EsitoEstrazione      `json:"estrazione,omitempty"`
	DerogaStruttura *uuid.UUID                      `json:"deroga_struttura,omitempty"`
	Calcolata       bool                            `json:"calcolata"`
}

// Gli esiti di v_step_prodotto (0020).
const (
	esitoPresenteAnalizzato    = "presente_analizzato"
	esitoPresenteParziale      = "presente_parziale"
	esitoPresenteNonAnalizzato = "presente_non_analizzato"
	esitoRiferimentoSuperato   = "riferimento_superato"
	esitoDaScegliere           = "da_scegliere"
	esitoDaConfermare          = "da_confermare"
	esitoSulPortale            = "sul_portale"
	esitoSoloAltro3D           = "solo_altro_3d"
	esitoMancante              = "mancante"
	esitoRadiceSenzaQualifica  = "radice_senza_qualifica"
)

// I valori del DB e degli adattatori che la fonte guarda, ripetuti qui perché i pacchetti che li dichiarano non li
// esportano: il tipo del documento STEP (0001, tipo_documento), la natura «inline» (natura_allegato), il ruolo
// «delega» della marcatura (fascicolo.Marcatura), le capacità «struttura» e «grafo_completo» con i loro stati e lo
// stato «errore» della qualità (estrazione, mappatura-step-1). Se cambiano, lo dicono le prove del pacchetto.
const (
	tipoDocumentoCad3D  = "cad_3d"
	naturaInline        = "inline"
	ruoloDelega         = "delega"
	capStruttura        = "struttura"
	capGrafoCompleto    = "grafo_completo"
	statoDisponibile    = "disponibile"
	statoNonDisponibile = "non_disponibile"
	statoErrore         = "errore"
)

// estensioniSTEP: solo uno STEP è fonte (T-E1-09; R68 A: un PDF mai).
var estensioniSTEP = map[string]bool{"stp": true, "step": true}

// estensioniAltro3D: gli altri 3D, che tengono il loro tipo documentale e non sono mai fonte (T-E1-09). È l'elenco
// dei cad_3d del legacy (classificazione.TipoDaEstensione) senza lo STEP, ripetuto perché valutazione non importa il
// legacy.
var estensioniAltro3D = map[string]bool{"sldprt": true, "sldasm": true, "igs": true, "iges": true, "x_t": true, "x_b": true,
	"prt": true, "par": true, "asm": true, "psm": true}

// Le sezioni della fotografia da cui dipende la fonte (T-12). Senza i fatti la parte del motore A non si calcola,
// ma lo stato dato dalla vista sì.
var sezioniDellaFonte = []string{fotorfq.SezioneStepProdotto, fotorfq.SezioneComponenti, fotorfq.SezioneDocumenti,
	fotorfq.SezioneAllegati, fotorfq.SezioneRigheComponenteProposta, fotorfq.SezioneRigheRelazioneProposta,
	fotorfq.SezioneLavoroPendente, fotorfq.SezioneProposteDocumento}

// ---- i file STEP del thread ----

// nodoLetto: un nodo STEP con la sua chiave e le letture d'identità della grammatica.
type nodoLetto struct {
	chiave  string
	letture []motorea.LetturaForma
}

// fileStep: uno STEP del thread (un allegato, o un documento senza allegato nel thread), con l'esito
// dell'estrazione e, se i fatti si leggono, le radici e i nodi interni con le letture.
type fileStep struct {
	allegatoID    *uuid.UUID
	documentoID   *uuid.UUID
	sha           string
	estrazione    ancoraggio.EsitoEstrazione
	grafoCompleto bool
	motivoGrafo   string
	radici        []nodoLetto
	interni       []nodoLetto
	letto         bool // l'interpretazione c'è: con il motore del cliente e i fatti letti
}

// contesto: un thread con ciò che serve alla fonte di ogni suo prodotto, calcolato una volta.
type contesto struct {
	t            fotorfq.Thread
	m            *motorea.Motore
	sezioni      map[string]fotorfq.StatoSezione
	inAttesa     map[uuid.UUID]bool
	allegati     map[uuid.UUID]*fotorfq.Allegato
	documenti    map[uuid.UUID]*fotorfq.DocumentoConfermato
	stepAllegati []*fileStep // gli STEP fra gli allegati del thread, in ordine di ID
	altri3D      int         // gli altri 3D fra gli allegati
	perSha       map[string]*fileStep
	perAllegato  map[uuid.UUID]*fileStep
	senzaFatti   bool // la sezione dei fatti è assente: il motore A non legge niente
}

func nuovoContesto(f fotorfq.Fotografia, t fotorfq.Thread, m *motorea.Motore) *contesto {
	c := &contesto{t: t, m: m, sezioni: f.Sezioni, inAttesa: map[uuid.UUID]bool{}, allegati: map[uuid.UUID]*fotorfq.Allegato{},
		documenti: map[uuid.UUID]*fotorfq.DocumentoConfermato{}, perSha: map[string]*fileStep{}, perAllegato: map[uuid.UUID]*fileStep{}}
	c.senzaFatti = sezioneAssente(f.Sezioni, fotorfq.SezioneFatti)
	for _, id := range t.InAttesa {
		c.inAttesa[id] = true
	}
	allegati := append([]fotorfq.Allegato(nil), t.Allegati...)
	sort.SliceStable(allegati, func(i, j int) bool { return allegati[i].ID.String() < allegati[j].ID.String() })
	for i := range allegati {
		c.allegati[allegati[i].ID] = &allegati[i]
	}
	for i := range t.Documenti {
		d := t.Documenti[i]
		c.documenti[d.ID] = &d
	}
	for i := range allegati {
		a := &allegati[i]
		if a.Natura == naturaInline {
			continue
		}
		est := estensioneDi(a)
		switch {
		case estensioniSTEP[est]:
			c.stepAllegati = append(c.stepAllegati, c.analisiAllegato(a))
		case estensioniAltro3D[est]:
			c.altri3D++
		}
	}
	return c
}

// sezioneAssente: con le sezioni dichiarate (gli export), quella sezione è «assente». Senza sezioni (una fotografia
// costruita in memoria) niente è assente.
func sezioneAssente(sezioni map[string]fotorfq.StatoSezione, k string) bool {
	s, ok := sezioni[k]
	return sezioni != nil && (!ok || s.Stato == fotorfq.StatoSezioneAssente)
}

// estensioneDi: l'estensione dichiarata dell'allegato, o quella del nome, senza maiuscole.
func estensioneDi(a *fotorfq.Allegato) string {
	if a.Estensione != nil && *a.Estensione != "" {
		return strings.ToLower(strings.TrimPrefix(*a.Estensione, "."))
	}
	return strings.ToLower(strings.TrimPrefix(path.Ext(a.NomeFile), "."))
}

// analisiAllegato: l'analisi di uno STEP fra gli allegati, una volta per allegato.
func (c *contesto) analisiAllegato(a *fotorfq.Allegato) *fileStep {
	if fs, ok := c.perAllegato[a.ID]; ok {
		return fs
	}
	id := a.ID
	fs := &fileStep{allegatoID: &id}
	if a.Sha256 != nil {
		fs.sha = *a.Sha256
	}
	for _, d := range c.t.Documenti {
		if fs.sha != "" && d.Sha256 == fs.sha {
			did := d.ID
			fs.documentoID = &did
		}
	}
	var contenitore *fotorfq.Allegato
	if a.ContenitoreID != nil {
		contenitore = c.allegati[*a.ContenitoreID]
	}
	c.leggi(fs, *a, contenitore, c.inAttesa[a.ID])
	c.perAllegato[a.ID] = fs
	if fs.sha != "" {
		if _, ok := c.perSha[fs.sha]; !ok {
			c.perSha[fs.sha] = fs
		}
	}
	return fs
}

// analisiDocumento: l'analisi dello STEP di un documento confermato: quella del suo contenuto fra gli allegati, se
// c'è; altrimenti dai fatti del documento (T-04), con un record d'allegato che serve solo all'adattatore per leggere
// i fatti e non esce da qui.
func (c *contesto) analisiDocumento(d *fotorfq.DocumentoConfermato) *fileStep {
	if fs, ok := c.perSha[d.Sha256]; ok && d.Sha256 != "" {
		return fs
	}
	did := d.ID
	fs := &fileStep{documentoID: &did, sha: d.Sha256}
	inAttesa := false
	for _, a := range d.Allegati {
		inAttesa = inAttesa || c.inAttesa[a]
	}
	est, sha := d.Estensione, d.Sha256
	c.leggi(fs, fotorfq.Allegato{ID: d.ID, NomeFile: d.NomeFile, Estensione: &est, Sha256: &sha, Natura: "file"}, nil, inAttesa)
	if d.Sha256 != "" {
		c.perSha[d.Sha256] = fs
	}
	return fs
}

// leggi: l'esito dell'estrazione dai fatti alla terna corrente (T-B0-07), e le radici e i nodi con le letture.
//   - nessun fatto: in_corso con il lavoro pendente, altrimenti non_analizzata;
//   - fatti che l'adattatore non legge, o con lo stato «errore» (il worker non ha letto il file): fallita;
//   - fatti senza una struttura che si legge (assente, o di una versione che va rianalizzata): non_analizzata;
//   - altrimenti riuscita, con la completezza del grafo (la capacità grafo_completo: «dati insufficienti» se manca).
func (c *contesto) leggi(fs *fileStep, a fotorfq.Allegato, contenitore *fotorfq.Allegato, inAttesa bool) {
	fatti, ok := c.t.Fatti[fs.sha]
	if fs.sha == "" || !ok || c.senzaFatti {
		fs.estrazione = ancoraggio.EstrazioneNonAnalizzata
		if inAttesa {
			fs.estrazione = ancoraggio.EstrazioneInCorso
		}
		return
	}
	doc, err := estrazione.DaAllegato(a, &fatti, contenitore)
	if err != nil || doc.Qualita.Stato == statoErrore {
		fs.estrazione = ancoraggio.EstrazioneFallita
		return
	}
	if capacita(doc, capStruttura).Stato == statoNonDisponibile || capacita(doc, capStruttura).Stato == "" {
		fs.estrazione = ancoraggio.EstrazioneNonAnalizzata
		return
	}
	fs.estrazione = ancoraggio.EstrazioneRiuscita
	g := capacita(doc, capGrafoCompleto)
	fs.grafoCompleto = g.Stato == statoDisponibile
	if !fs.grafoCompleto {
		fs.motivoGrafo = g.Motivo
	}

	// Le radici e i nodi: le entità nodo_step; una è radice se le sue unità hanno il contesto radice_step (le radici
	// vengono da struttura.radici dei fatti, A1b).
	radice := map[string]bool{}
	chiave := map[string]string{}
	for _, e := range doc.Entita {
		chiave[e.ID] = e.ChiaveOriginale
	}
	entitaDi := map[string]string{}
	for _, u := range doc.Unita {
		if u.EntitaID == "" {
			continue
		}
		entitaDi[u.ID] = u.EntitaID
		if u.Selettore.Contesto == evidenze.ContestoRadiceSTEP {
			radice[u.EntitaID] = true
		}
	}
	letture := map[string][]motorea.LetturaForma{}
	if c.m != nil {
		if r, err := c.m.Interpreta(doc, evidenze.UsoSconosciuto(doc.BundleID)); err == nil {
			fs.letto = true
			for _, l := range r.Letture {
				campo := l.Forma.Selettore.Campo.Valore
				if e := entitaDi[l.UnitaID]; e != "" && (campo == "id" || campo == "nome") {
					letture[e] = append(letture[e], l.Forma)
				}
			}
		}
	}
	var entita []string
	for _, e := range doc.Entita {
		if e.Tipo == "nodo_step" {
			entita = append(entita, e.ID)
		}
	}
	sort.Strings(entita)
	for _, e := range entita {
		n := nodoLetto{chiave: chiave[e], letture: letture[e]}
		if radice[e] {
			fs.radici = append(fs.radici, n)
		} else {
			fs.interni = append(fs.interni, n)
		}
	}
}

// capacita: la capacità del documento con quel nome; vuota se non c'è.
func capacita(d evidenze.DocumentoEvidenze, nome string) evidenze.Capacita {
	for _, x := range d.Qualita.Capacita {
		if x.Nome == nome {
			return x
		}
	}
	return evidenze.Capacita{}
}

// compatibilita: il confronto della base del prodotto con le letture di un nodo (motorea.ConfrontaBasi), nello
// stesso namespace: il migliore fra uguale, compatibile_parziale, discordante, non_determinabile. Senza letture
// del nodo nello stesso namespace: non_determinabile.
//
// Dalla fase 1 di B5 vale anche la regola unica del marcatore di ancoraggio (T-B4-30, nelle radici candidate di
// ProponiStrutture: compatibilitaDelNodo): con il marcatore del prodotto e quello del nodo scritti tutti e due e
// diversi, una base compatibile è discordante (R86: un altro marcatore è un'altra identità); un marcatore scritto da
// una parte sola non cambia niente. Così il candidato del motore A, il nodo interno e la radice di una delega dicono la
// stessa cosa delle strutture di ancoraggio (StrutturaProdotto.Fonti, T-B2-02).
func compatibilita(p motorea.LetturaForma, letture []motorea.LetturaForma) motorea.Compatibilita {
	rango := map[motorea.Compatibilita]int{motorea.CompatibilitaNonDeterminabile: 0, motorea.CompatibilitaDiscordante: 1,
		motorea.CompatibilitaParziale: 2, motorea.CompatibilitaEquivalente: 3, motorea.CompatibilitaUguale: 4}
	migliore := motorea.CompatibilitaNonDeterminabile
	for _, l := range letture {
		if l.Namespace != p.Namespace {
			continue
		}
		if c := conMarcatore(motorea.ConfrontaBasi(p.Base, l.Base), marcatoreDi(p), marcatoreDi(l)); rango[c] > rango[migliore] {
			migliore = c
		}
	}
	return migliore
}

// conMarcatore: la compatibilità delle basi corretta con la regola unica del marcatore (T-B4-30, la stessa di
// ancoraggio, che non la esporta): due basi compatibili con due marcatori scritti e diversi sono discordanti; un altro
// esito delle basi resta com'è.
func conMarcatore(basi motorea.Compatibilita, a, b string) motorea.Compatibilita {
	if compatibile(basi) && a != "" && b != "" && a != b {
		return motorea.CompatibilitaDiscordante
	}
	return basi
}

// marcatoreDi: il valore del marcatore di una lettura; "" senza marcatore.
func marcatoreDi(f motorea.LetturaForma) string {
	if f.Marcatore == nil {
		return ""
	}
	return f.Marcatore.Valore
}

// compatibile: la base del nodo è quella del prodotto (uguale, o compatibile con una forma parziale: A-C07).
func compatibile(c motorea.Compatibilita) bool {
	return c == motorea.CompatibilitaUguale || c == motorea.CompatibilitaEquivalente || c == motorea.CompatibilitaParziale
}

// ---- la fonte di un prodotto ----

// fonte calcola la fonte strutturale di un prodotto target (contratto §1.2, con le note; R65 A, R76 b A, T-B0-07,
// T-B0-08, T-B0-10, T-E1-09, T-12).
//
//  1. Con il componente, la riga di v_step_prodotto dà lo stato e il motivo con la tabella di R65 A: è l'unica
//     definizione. presente_non_analizzato è analisi_in_corso con un lavoro pendente per lo STEP, altrimenti
//     non_analizzata. Una proposta aperta di 3D che non è uno STEP non è fonte (T-E1-09): vale come solo_altro_3d.
//  2. Il gesto 3 dà il riferimento (T-B0-08): step_strutturale_id, con la marcatura evidenza.strutturale valida
//     (forma «smistamento», con radice, ruolo, chi e quando) o senza (forma «step_strutturale_id», chi e quando non
//     registrati, la radice dalle righe del file senza arco entrante se è una sola). Una delega è fonte solo se la
//     radice del file ha la base del prodotto (contratto §1.2, nota; R76 b).
//  3. I candidati: gli STEP correnti del prodotto non scelti, la proposta aperta di uno STEP, gli STEP fra gli
//     allegati del thread con una radice compatibile con la base del prodotto (R76 b A). Un candidato porta la fonte
//     a in_attesa_di_conferma solo da assente, con documento_candidato, mai da confermata né da in attesa (T-B0-07).
//  4. Una fonte assente prende il primo motivo che vale, nell'ordine di T-E1-09: estrazione_fallita,
//     analisi_in_corso, step_presente_non_analizzato (fra gli STEP del thread), poi solo_nodo_interno (il prodotto è
//     un nodo interno di uno STEP analizzato: R76 b A), indicata_sul_portale, solo_altro_3d (dalla vista, o dal
//     motore A con un altro 3D e nessuno STEP fra gli allegati), prodotto_senza_componente per un target senza
//     componente (T-B0-07), nessuna_fonte.
//  5. T-12: con una sezione della fonte assente la fonte non si calcola: niente stato, motivo non_determinabile. Lo
//     stesso (la lettura dell'orchestratore T-B1-11) senza poter confrontare le basi (nessuna grammatica, codice non
//     letto, fatti assenti) mentre uno STEP analizzato del thread potrebbe essere un candidato, o la radice di una
//     delega; uno stato dato dalla vista (confermata, in attesa) resta, salvo la delega con la radice non
//     confrontabile, e l'esito della vista sta sempre accanto.
func (c *contesto) fonte(tg target) (FonteProdotto, []evidenze.Diagnostica) {
	f := FonteProdotto{Calcolata: true}
	for _, s := range sezioniDellaFonte {
		if sezioneAssente(c.sezioni, s) {
			return FonteProdotto{Motivo: MotivoFonteNonDeterminabile}, nil
		}
	}
	var diag []evidenze.Diagnostica
	incoerente := func(messaggio string, rif ...string) {
		diag = append(diag, evidenze.Diagnostica{Codice: CodiceRiferimentoIncoerente, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
			Percorso: "prodotti[" + tg.pv.Rif + "].fonte", Messaggio: messaggio, Rif: append([]string{tg.pv.Rif}, rif...)})
	}

	comp := tg.componente
	var vista *fotorfq.RigaStepProdotto
	if comp != nil {
		for i := range c.t.StepProdotto {
			if c.t.StepProdotto[i].ComponenteID == comp.ID {
				vista = &c.t.StepProdotto[i]
			}
		}
	}
	if vista != nil {
		f.EsitoVista, f.DerogaStruttura = vista.Esito, vista.DerogaStrutturaID
		if vista.MotivoParziale != nil {
			f.MotivoSQL = *vista.MotivoParziale
		}
		if !stessoUUID(vista.StepStrutturaleID, comp.StepStrutturaleID) {
			incoerente("lo STEP strutturale della vista non è quello del componente")
		}
	}

	// 2. Il gesto 3.
	var rifFile *fileStep
	if comp != nil && comp.StepStrutturaleID != nil {
		var d []evidenze.Diagnostica
		f.Riferimento, rifFile, d = c.riferimento(tg, comp)
		diag = append(diag, d...)
		if rifFile != nil {
			f.Estrazione = rifFile.estrazione
		}
		if vista == nil {
			incoerente("il componente ha lo STEP strutturale, ma la vista non ha la sua riga: lo stato non si legge dal gesto da solo")
		}
	} else if comp != nil {
		for _, r := range c.t.RigheComponenteProposta {
			if m := r.Marcatura; m != nil && !m.Sospesa && m.ComponenteID != nil && *m.ComponenteID == comp.ID && m.Ruolo != ruoloDelega {
				incoerente("una marcatura strutturale valida per il componente, che non ha lo STEP strutturale (step_strutturale_id vuoto)", "componente_proposta:"+r.ID.String())
			}
		}
	}

	// 3. I candidati.
	cand, interno, determinabile := c.candidati(tg, f.Riferimento, vista)
	f.Candidati = cand

	// 1. Lo stato dalla vista.
	var stato StatoFonte
	var motivo MotivoFonte
	soloAltro3DVista, portale := false, false
	if vista != nil {
		switch vista.Esito {
		case esitoPresenteAnalizzato:
			stato, motivo = FonteConfermata, MotivoFonteEstrazioneRiuscita
		case esitoPresenteParziale:
			stato, motivo = FonteConfermata, MotivoFonteDatiInsufficienti
		case esitoPresenteNonAnalizzato:
			stato, motivo = FonteConfermata, MotivoFonteNonAnalizzata
			if f.Riferimento != nil && c.riferimentoInAttesa(f.Riferimento.DocumentoID) {
				motivo = MotivoFonteAnalisiInCorso
			}
		case esitoRiferimentoSuperato:
			stato, motivo = FonteInAttesaDiConferma, MotivoFonteRiferimentoSuperato
		case esitoDaScegliere:
			stato, motivo = FonteInAttesaDiConferma, MotivoFonteDaScegliere
		case esitoDaConfermare:
			if c.propostaNonStep(vista.PropostaAperta) {
				soloAltro3DVista = true
			} else {
				stato, motivo = FonteInAttesaDiConferma, MotivoFonteProposta3DAperta
			}
		case esitoSulPortale:
			portale = true
		case esitoSoloAltro3D:
			soloAltro3DVista = true
		}
	}

	// La delega (contratto §1.2, nota): fonte solo se la radice del file ha la base del prodotto. Se la radice non si
	// può confrontare (il file senza i fatti letti, nessuna grammatica, il codice del target non letto), la fonte non
	// si calcola, come per T-12 (la lettura dell'orchestratore T-B1-11); con la radice confrontabile e diversa, lo STEP
	// non è la sua fonte.
	if stato == FonteConfermata && f.Riferimento != nil && f.Riferimento.Ruolo == ruoloDelega {
		switch {
		case rifFile == nil || tg.lettura == nil || !rifFile.letto:
			return FonteProdotto{Motivo: MotivoFonteNonDeterminabile, MotivoSQL: f.MotivoSQL, EsitoVista: f.EsitoVista,
				Riferimento: f.Riferimento, Candidati: f.Candidati, Estrazione: f.Estrazione, DerogaStruttura: f.DerogaStruttura}, diag
		case !radiceCompatibile(rifFile, *tg.lettura):
			incoerente("una delega su un finito: la radice del file non ha la base del prodotto, quindi lo STEP non è la sua fonte (R76 b)")
			stato, motivo = "", ""
			for _, n := range rifFile.interni {
				interno = interno || compatibile(compatibilita(*tg.lettura, n.letture))
			}
		}
	}

	switch {
	case stato != "":
		f.Stato, f.Motivo = stato, motivo
	case haCandidatiDelMotoreA(cand):
		f.Stato, f.Motivo = FonteInAttesaDiConferma, MotivoFonteDocumentoCandidato
	case !determinabile:
		return FonteProdotto{Motivo: MotivoFonteNonDeterminabile, MotivoSQL: f.MotivoSQL, EsitoVista: f.EsitoVista,
			Riferimento: f.Riferimento, Candidati: f.Candidati, Estrazione: f.Estrazione, DerogaStruttura: f.DerogaStruttura}, diag
	default:
		f.Stato, f.Motivo = FonteAssente, c.motivoDellAssenza(comp == nil, interno, portale, soloAltro3DVista)
	}
	return f, diag
}

// motivoDellAssenza: il primo motivo che vale, nell'ordine di T-E1-09 (con solo_nodo_interno e
// prodotto_senza_componente al loro posto: vedi fonte, passo 4).
func (c *contesto) motivoDellAssenza(senzaComponente, interno, portale, soloAltro3DVista bool) MotivoFonte {
	fallita, inCorso, nonAnalizzata := false, false, false
	for _, fs := range c.stepAllegati {
		switch fs.estrazione {
		case ancoraggio.EstrazioneFallita:
			fallita = true
		case ancoraggio.EstrazioneInCorso:
			inCorso = true
		case ancoraggio.EstrazioneNonAnalizzata:
			nonAnalizzata = true
		}
	}
	switch {
	case fallita:
		return MotivoFonteEstrazioneFallita
	case inCorso:
		return MotivoFonteAnalisiInCorso
	case nonAnalizzata:
		return MotivoFonteStepPresenteNonAnalizzato
	case interno:
		return MotivoFonteSoloNodoInterno
	case portale:
		return MotivoFonteIndicataSulPortale
	case soloAltro3DVista, len(c.stepAllegati) == 0 && c.altri3D > 0:
		return MotivoFonteSoloAltro3D
	case senzaComponente:
		return MotivoFonteProdottoSenzaComponente
	}
	return MotivoFonteNessunaFonte
}

// riferimento: il gesto 3 del componente (T-B0-08), con il file STEP del documento se c'è.
func (c *contesto) riferimento(tg target, comp *fotorfq.Componente) (*ancoraggio.RiferimentoFonte, *fileStep, []evidenze.Diagnostica) {
	var diag []evidenze.Diagnostica
	// d porta solo il codice, scritto dal chiamante con la sua costante (A1a-CAT); qui il resto.
	diagnosi := func(d evidenze.Diagnostica, messaggio string, rif ...string) {
		d.Gravita, d.Natura, d.Percorso, d.Messaggio = evidenze.GravitaAvviso, evidenze.NaturaDati, "prodotti["+tg.pv.Rif+"].fonte.riferimento", messaggio
		d.Rif = append([]string{tg.pv.Rif}, rif...)
		diag = append(diag, d)
	}
	stepID := *comp.StepStrutturaleID
	r := &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: stepID, Forma: ancoraggio.FormaRiferimentoStepStrutturale}
	doc := c.documenti[stepID]
	var fs *fileStep
	if doc == nil {
		diagnosi(evidenze.Diagnostica{Codice: CodiceRiferimentoIncoerente}, "lo STEP strutturale del componente non è fra i documenti del thread", "documento:"+stepID.String())
	} else {
		r.Sha256, r.Superato = doc.Sha256, doc.SostituitoDa != nil
		for _, a := range doc.Allegati {
			if c.allegati[a] != nil {
				id := a
				r.AllegatoID = &id
				break
			}
		}
		fs = c.analisiDocumento(doc)
	}

	// La marcatura valida: non sospesa, dello stesso documento, del componente (o senza componente).
	var marcate []fotorfq.RigaComponenteProposta
	for _, riga := range c.t.RigheComponenteProposta {
		m := riga.Marcatura
		if m == nil || m.Sospesa || m.DocumentoID == nil {
			continue
		}
		switch {
		case *m.DocumentoID == stepID && m.ComponenteID != nil && *m.ComponenteID != comp.ID:
			diagnosi(evidenze.Diagnostica{Codice: CodiceRiferimentoIncoerente}, "la marcatura dello STEP strutturale è di un altro componente", "componente_proposta:"+riga.ID.String())
		case *m.DocumentoID == stepID:
			marcate = append(marcate, riga)
		case m.ComponenteID != nil && *m.ComponenteID == comp.ID && m.Ruolo != ruoloDelega:
			diagnosi(evidenze.Diagnostica{Codice: CodiceRiferimentoIncoerente}, "una marcatura valida del componente per un documento che non è il suo STEP strutturale", "componente_proposta:"+riga.ID.String())
		}
	}
	sort.SliceStable(marcate, func(i, j int) bool { return marcate[i].ID.String() < marcate[j].ID.String() })
	switch {
	case len(marcate) == 1:
		m := marcate[0].Marcatura
		r.Forma, r.Radice, r.Ruolo, r.ConfermatoDa, r.ConfermatoIl = ancoraggio.FormaRiferimentoSmistamento, marcate[0].Chiave, m.Ruolo, m.DichiaratoDa, m.DichiaratoIl
		if doc != nil && marcate[0].Sha256 != doc.Sha256 {
			diagnosi(evidenze.Diagnostica{Codice: CodiceRiferimentoIncoerente}, "la riga della marcatura è di un altro contenuto dello STEP strutturale", "componente_proposta:"+marcate[0].ID.String())
		}
		return r, fs, diag
	case len(marcate) > 1:
		var rif []string
		for _, x := range marcate {
			rif = append(rif, "componente_proposta:"+x.ID.String())
		}
		diagnosi(evidenze.Diagnostica{Codice: CodiceRiferimentoIncoerente}, fmt.Sprintf("%d marcature valide per lo stesso STEP strutturale: nessuna si sceglie", len(marcate)), rif...)
	}

	// Senza marcatura (T-B0-08): la radice dalle righe del file senza arco entrante, se è una sola.
	if doc != nil {
		r.Radice = c.radiceDalleRighe(doc.Sha256)
	}
	if r.Radice == "" {
		diagnosi(evidenze.Diagnostica{Codice: CodiceRadiceNonRegistrata}, "lo STEP strutturale non dice la radice: nessuna marcatura, e le righe del file non hanno una sola radice (T-B0-08)",
			"documento:"+stepID.String())
	}
	return r, fs, diag
}

// radiceDalleRighe: la chiave dell'unica riga di componente_proposta del contenuto senza un arco entrante nel suo
// file; "" se non ce n'è una sola (T-B0-08).
func (c *contesto) radiceDalleRighe(sha string) string {
	figli := map[string]bool{}
	for _, r := range c.t.RigheRelazioneProposta {
		figli[r.AllegatoID.String()+"\x00"+r.FiglioChiave] = true
	}
	radici := map[string]bool{}
	for _, r := range c.t.RigheComponenteProposta {
		if r.Sha256 == sha && sha != "" && !figli[r.AllegatoID.String()+"\x00"+r.Chiave] {
			radici[r.Chiave] = true
		}
	}
	if len(radici) != 1 {
		return ""
	}
	for k := range radici {
		return k
	}
	return ""
}

// candidati: i documenti candidati del prodotto, in ordine di (origine, sha256, radice). Restituisce anche se il
// prodotto è solo un nodo interno di uno STEP analizzato del thread, e se il confronto con le radici si è potuto
// fare per tutti gli STEP analizzati che non sono già il riferimento o un candidato.
func (c *contesto) candidati(tg target, rif *ancoraggio.RiferimentoFonte, vista *fotorfq.RigaStepProdotto) ([]ancoraggio.DocumentoCandidato, bool, bool) {
	var out []ancoraggio.DocumentoCandidato
	usati := map[string]bool{}
	if rif != nil && rif.Sha256 != "" {
		usati[rif.Sha256] = true
	}
	aggiungi := func(origine ancoraggio.OrigineCandidato, allegato, documento *uuid.UUID, fs *fileStep) {
		dc := ancoraggio.DocumentoCandidato{Origine: origine, AllegatoID: allegato, DocumentoID: documento, Sha256: fs.sha,
			Estrazione: fs.estrazione, GrafoCompleto: fs.grafoCompleto, MotivoGrafo: fs.motivoGrafo, Compatibilita: motorea.CompatibilitaNonDeterminabile}
		for _, n := range fs.radici {
			dc.Radici = append(dc.Radici, n.chiave)
		}
		sort.Strings(dc.Radici)
		if tg.lettura != nil && fs.letto {
			for _, n := range fs.radici {
				if comp := compatibilita(*tg.lettura, n.letture); compatibile(comp) {
					dc.RadiceCompatibile, dc.Compatibilita = n.chiave, comp
					break
				}
			}
			if dc.RadiceCompatibile == "" {
				for _, n := range fs.radici {
					if comp := compatibilita(*tg.lettura, n.letture); comp == motorea.CompatibilitaDiscordante {
						dc.Compatibilita = comp
					}
				}
			}
		}
		usati[fs.sha] = true
		out = append(out, dc)
	}

	// Gli STEP correnti del prodotto non scelti (v_step_prodotto.n_step_correnti).
	if tg.componente != nil {
		var docs []*fotorfq.DocumentoConfermato
		for i := range c.t.Documenti {
			d := &c.t.Documenti[i]
			if d.ComponenteID != nil && *d.ComponenteID == tg.componente.ID && d.Tipo == tipoDocumentoCad3D && d.SostituitoDa == nil &&
				estensioniSTEP[strings.ToLower(d.Estensione)] && !usati[d.Sha256] {
				docs = append(docs, d)
			}
		}
		sort.SliceStable(docs, func(i, j int) bool { return docs[i].ID.String() < docs[j].ID.String() })
		for _, d := range docs {
			did := d.ID
			fs := c.analisiDocumento(d)
			var allegato *uuid.UUID
			for _, a := range d.Allegati {
				if c.allegati[a] != nil {
					id := a
					allegato = &id
					break
				}
			}
			aggiungi(ancoraggio.CandidatoDaDocumentoDelProdotto, allegato, &did, fs)
		}
	}
	// La proposta aperta di uno STEP (v_step_prodotto.proposta_aperta).
	if vista != nil && vista.PropostaAperta != nil {
		for _, p := range c.t.Proposte {
			if p.ID != *vista.PropostaAperta {
				continue
			}
			if a := c.allegati[p.AllegatoID]; a != nil && estensioniSTEP[estensioneDi(a)] {
				fs := c.analisiAllegato(a)
				if !usati[fs.sha] {
					aid := a.ID
					aggiungi(ancoraggio.CandidatoDaPropostaAperta, &aid, fs.documentoID, fs)
				}
			}
		}
	}
	// Gli STEP del thread con una radice compatibile (R76 b A).
	interno, determinabile := false, true
	if c.senzaFatti && len(c.stepAllegati) > 0 {
		determinabile = false // senza la sezione dei fatti, uno STEP del thread potrebbe essere un candidato (T-12)
	}
	for _, fs := range c.stepAllegati {
		if fs.estrazione != ancoraggio.EstrazioneRiuscita || usati[fs.sha] && fs.sha != "" {
			continue
		}
		if tg.lettura == nil || !fs.letto {
			determinabile = false
			continue
		}
		radice := false
		for _, n := range fs.radici {
			if compatibile(compatibilita(*tg.lettura, n.letture)) {
				radice = true
			}
		}
		if radice {
			aggiungi(ancoraggio.CandidatoDaMotoreA, fs.allegatoID, fs.documentoID, fs)
			continue
		}
		for _, n := range fs.interni {
			if compatibile(compatibilita(*tg.lettura, n.letture)) {
				interno = true
			}
		}
	}
	ordine := map[ancoraggio.OrigineCandidato]int{ancoraggio.CandidatoDaDocumentoDelProdotto: 0, ancoraggio.CandidatoDaPropostaAperta: 1, ancoraggio.CandidatoDaMotoreA: 2}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Origine != b.Origine {
			return ordine[a.Origine] < ordine[b.Origine]
		}
		if a.Sha256 != b.Sha256 {
			return a.Sha256 < b.Sha256
		}
		return a.RadiceCompatibile < b.RadiceCompatibile
	})
	return out, interno, determinabile
}

// radiceCompatibile: una radice del file ha la base del prodotto.
func radiceCompatibile(fs *fileStep, p motorea.LetturaForma) bool {
	for _, n := range fs.radici {
		if compatibile(compatibilita(p, n.letture)) {
			return true
		}
	}
	return false
}

// haCandidatiDelMotoreA: fra i candidati c'è uno STEP del thread trovato dal motore A (T-B0-07: solo lui porta una
// fonte assente in attesa di conferma; gli altri hanno già lo stato dalla vista).
func haCandidatiDelMotoreA(cand []ancoraggio.DocumentoCandidato) bool {
	for _, d := range cand {
		if d.Origine == ancoraggio.CandidatoDaMotoreA {
			return true
		}
	}
	return false
}

// riferimentoInAttesa: c'è un lavoro pendente per un allegato del documento di riferimento.
func (c *contesto) riferimentoInAttesa(id uuid.UUID) bool {
	d := c.documenti[id]
	if d == nil {
		return false
	}
	for _, a := range d.Allegati {
		if c.inAttesa[a] {
			return true
		}
	}
	return false
}

// propostaNonStep: la proposta aperta della vista è di un file che non è uno STEP (T-E1-09: un altro 3D non è
// fonte, e tiene il suo tipo). Una proposta che non si trova, o il cui file non si sa, resta com'è nella vista.
func (c *contesto) propostaNonStep(id *uuid.UUID) bool {
	if id == nil {
		return false
	}
	for _, p := range c.t.Proposte {
		if p.ID == *id {
			if a := c.allegati[p.AllegatoID]; a != nil {
				est := estensioneDi(a)
				return est != "" && !estensioniSTEP[est]
			}
		}
	}
	return false
}

func stessoUUID(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
