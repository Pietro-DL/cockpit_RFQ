package valutazione

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// Il collegamento con ancoraggio (B5, fase 1; piano A, 6.4.6 passi 7-9 e 7a; contratto §1.3, §1.4, §1.5, §2.2, §2.5,
// §2.6; le NOTE per B5 dei tre resoconti di B4): ancoraggio non importa la fotografia (T-B0-04), quindi valutazione le
// converte i file del thread (FileInterpretato), i target (ProdottoRichiesto) e il contesto (ContestoStrutturale), e
// chiama ProponiAncoraggi una volta per thread. Le strutture, gli ancoraggi dei file, la catena del codice e la
// riconciliazione escono in ValutazioneProdotti.Ancoraggi. Le regole sono di ancoraggio: qui si sceglie solo che cosa
// della fotografia entra, e niente del legacy che il contratto esclude (il codice delle righe decise, il codice del motore
// legacy sulle righe aperte, le decisioni senza un gesto).

// I valori del DB che il collegamento guarda, ripetuti qui perché i pacchetti che li dichiarano non li esportano:
// gli stati di stato_proposta, l'origine «operatore» di origine_codice (la correzione manuale: T-B0-33), il tipo
// documentale del requisito 2D (R82) e la natura «file» di un allegato (0001). Se cambiano, lo dicono le prove.
const (
	statoPropostaAperta     = "aperta"
	statoPropostaConfermata = "confermata"
	statoPropostaDuplicato  = "duplicato"
	statoPropostaScartata   = "scartata"
	origineCodiceOperatore  = "operatore"
	tipoDocumentoDisegno2D  = "disegno_2d"
	naturaFile              = "file"
	statoQualitaParziale    = "parziale"
)

// collegamento: un thread con ciò che serve agli ingressi di ancoraggio, calcolato una volta. Le mappe servono solo a
// cercare, mai a scorrere: si scorrono gli elenchi della fotografia copiati e messi in ordine.
type collegamento struct {
	t            fotorfq.Thread
	m            *motorea.Motore
	allegati     map[uuid.UUID]*fotorfq.Allegato
	inAttesa     map[uuid.UUID]bool
	tipi         map[uuid.UUID]string // il tipo documentale di ogni allegato: dal documento, se c'è, altrimenti dalla proposta
	file         []ancoraggio.FileInterpretato
	sonoFile     map[uuid.UUID]bool
	strutture    []ancoraggio.StrutturaFile
	shaStruttura map[uuid.UUID]string // lo sha256 della struttura di ogni allegato
}

// ancoraggiDelThread: gli ancoraggi del thread (6.4.6, passo 9), con i target di B1 e la loro fonte già calcolata.
//  1. I file (fileDelThread): gli allegati di natura «file» che non sono contenitori, ognuno con il suo documento
//     (estrazione.DaAllegato, con i fatti alla terna della fotografia e il contenitore), la sua interpretazione con
//     l'uso sconosciuto (un file non ha segmenti), la disponibilità (6.4.6 passo 8, T-B0-31) e il segno del 2D (T-B4-31).
//     Un errore di un adattatore lascia fuori quel file, non il thread (6.4.6 passo 3).
//  2. I target (prodottiRichiesti): Rif, autorità, componente, la lettura del codice con il marcatore e la revisione
//     (T-B4-20, accolta con T-B4-06 rivisto), la fonte confermata solo con la fonte confermata (R76 A).
//  3. Il contesto (contestoStrutturale): le strutture STEP dei file, i componenti confermati non archiviati con la
//     revisione e la lettura, le relazioni confermate, le righe aperte (il codice manuale solo dall'operatore), le righe
//     decise (mai codice né revisione), i codici composti dal compositore (T-08), i codici dei messaggi (T-E1-06), le
//     associazioni decise (T-B4-33). LettureDecise e DecisioniIdentita restano vuote: in A1c nessun adattatore produce
//     una DecisioneIdentita (LD-27).
//
// m nil: nessuna grammatica A; i file hanno un'interpretazione vuota del loro documento, i target non hanno la lettura,
// e nessun codice si compone: le strutture ci sono, senza identità. L'errore è quello di contratto di ProponiAncoraggi.
// Il collegamento torna indietro con l'esito: i 2D (fase 2) usano gli stessi file, già letti e interpretati.
func ancoraggiDelThread(t fotorfq.Thread, m *motorea.Motore, targets []target, interpretati []ancoraggio.MessaggioInterpretato) (ancoraggio.EsitoAncoraggi, *collegamento, error) {
	c := nuovoCollegamento(t, m)
	e, err := ancoraggio.ProponiAncoraggi(c.file, c.prodottiRichiesti(targets), c.contestoStrutturale(interpretati))
	return e, c, err
}

func nuovoCollegamento(t fotorfq.Thread, m *motorea.Motore) *collegamento {
	c := &collegamento{t: t, m: m, allegati: map[uuid.UUID]*fotorfq.Allegato{}, inAttesa: map[uuid.UUID]bool{},
		tipi: map[uuid.UUID]string{}, sonoFile: map[uuid.UUID]bool{}, shaStruttura: map[uuid.UUID]string{}}
	allegati := append([]fotorfq.Allegato(nil), t.Allegati...)
	sort.SliceStable(allegati, func(i, j int) bool { return allegati[i].ID.String() < allegati[j].ID.String() })
	contenitori := map[uuid.UUID]bool{}
	for i := range allegati {
		c.allegati[allegati[i].ID] = &allegati[i]
		if allegati[i].ContenitoreID != nil {
			contenitori[*allegati[i].ContenitoreID] = true
		}
	}
	for _, id := range t.InAttesa {
		c.inAttesa[id] = true
	}
	c.tipiDocumentali()
	for i := range allegati {
		a := allegati[i]
		if (a.Natura != "" && a.Natura != naturaFile) || contenitori[a.ID] {
			continue
		}
		if f, ok := c.fileInterpretato(a); ok {
			c.file = append(c.file, f)
			c.sonoFile[a.ID] = true
			if s, ok := ancoraggio.StrutturaDa(f); ok {
				c.strutture = append(c.strutture, s)
				c.shaStruttura[a.ID] = s.Sha256
			}
		}
	}
	return c
}

// tipiDocumentali: il tipo documentale di ogni allegato, come lo dice il DB (T-B4-31): il tipo del documento
// confermato che lo porta in documento_provenienza (prima i documenti correnti, poi gli altri, in ordine di ID),
// altrimenti il tipo proposto della sua proposta (in ordine di ID). Un allegato senza documento né proposta non ha tipo.
func (c *collegamento) tipiDocumentali() {
	docs := append([]fotorfq.DocumentoConfermato(nil), c.t.Documenti...)
	sort.SliceStable(docs, func(i, j int) bool {
		if (docs[i].SostituitoDa == nil) != (docs[j].SostituitoDa == nil) {
			return docs[i].SostituitoDa == nil
		}
		return docs[i].ID.String() < docs[j].ID.String()
	})
	for _, d := range docs {
		for _, a := range d.Allegati {
			if _, ok := c.tipi[a]; !ok {
				c.tipi[a] = d.Tipo
			}
		}
	}
	proposte := append([]fotorfq.PropostaAttuale(nil), c.t.Proposte...)
	sort.SliceStable(proposte, func(i, j int) bool { return proposte[i].ID.String() < proposte[j].ID.String() })
	for _, p := range proposte {
		if _, ok := c.tipi[p.AllegatoID]; !ok {
			c.tipi[p.AllegatoID] = p.Tipo
		}
	}
}

// fileInterpretato: il file con il suo documento, l'interpretazione, la disponibilità e il segno del 2D. Falso se
// l'adattatore o l'interpretazione danno un errore: il file resta fuori (6.4.6 passo 3, «non valutato»).
func (c *collegamento) fileInterpretato(a fotorfq.Allegato) (ancoraggio.FileInterpretato, bool) {
	var fatti *fotorfq.Fatti
	if a.Sha256 != nil {
		if f, ok := c.t.Fatti[*a.Sha256]; ok {
			fatti = &f
		}
	}
	var contenitore *fotorfq.Allegato
	if a.ContenitoreID != nil {
		contenitore = c.allegati[*a.ContenitoreID]
	}
	doc, err := estrazione.DaAllegato(a, fatti, contenitore)
	if err != nil {
		return ancoraggio.FileInterpretato{}, false
	}
	interp := motorea.Interpretazione{BundleID: doc.BundleID}
	if c.m != nil {
		if interp, err = c.m.Interpreta(doc, evidenze.UsoSconosciuto(doc.BundleID)); err != nil {
			return ancoraggio.FileInterpretato{}, false
		}
	}
	return ancoraggio.FileInterpretato{AllegatoID: a.ID, Documento: doc, Interpretazione: interp,
		Disponibilita: disponibilitaDi(a, doc, c.inAttesa[a.ID]), Disegno: c.tipi[a.ID] == tipoDocumentoDisegno2D}, true
}

// disponibilitaDi: la disponibilità di un file (6.4.6 passo 8, con T-B0-31 e la lettura T-B4-09), il primo che vale:
//   - mancante: l'allegato senza sha256 (nessun contenuto);
//   - pendente: un lavoro di analisi in attesa;
//   - illeggibile: un PDF che il worker non ha aperto (pdf.illeggibile, errore_pdf): un contenuto che non si apre (LD-05);
//   - errore: gli altri fatti in errore (la qualità «errore» del documento: uno STEP che il worker non ha letto);
//   - senza_testo: una scansione, un PDF senza testo nativo e senza OCR (pdf.senza_testo; T-B0-31: non «illeggibile»);
//   - illeggibile: la qualità «non_disponibile» del documento, che il piano dava alle scansioni (6.4.6) e che resta per
//     ciò che non si legge per niente;
//   - parziale: la qualità «parziale» (anche un file senza fatti, di cui si legge solo il nome);
//   - disponibile: altrimenti.
func disponibilitaDi(a fotorfq.Allegato, d evidenze.DocumentoEvidenze, inAttesa bool) ancoraggio.Disponibilita {
	switch {
	case a.Sha256 == nil || *a.Sha256 == "":
		return ancoraggio.DisponibilitaMancante
	case inAttesa:
		return ancoraggio.DisponibilitaPendente
	case haDiagnostica(d, estrazione.CodicePDFIlleggibile):
		return ancoraggio.DisponibilitaIlleggibile
	case d.Qualita.Stato == statoErrore:
		return ancoraggio.DisponibilitaErrore
	case haDiagnostica(d, estrazione.CodicePDFSenzaTesto):
		return ancoraggio.DisponibilitaSenzaTesto
	case d.Qualita.Stato == statoNonDisponibile:
		return ancoraggio.DisponibilitaIlleggibile
	case d.Qualita.Stato == statoQualitaParziale:
		return ancoraggio.DisponibilitaParziale
	}
	return ancoraggio.DisponibilitaDisponibile
}

// haDiagnostica: il documento ha una diagnostica con quel codice.
func haDiagnostica(d evidenze.DocumentoEvidenze, codice string) bool {
	for _, x := range d.Qualita.Diagnostiche {
		if x.Codice == codice {
			return true
		}
	}
	return false
}

// prodottiRichiesti: i target di B1 come target delle proposte (contratto §2.2; 6.4.5).
//   - Rif, Autorita, ComponenteID, CodiceRichiesto, Base: quelli del prodotto valutato (R60 A: confermata o scenario).
//   - Namespace, Marcatore, Revisione, Qualificatori: dalla lettura del codice (il codice registrato letto con la
//     grammatica per un target confermato; la lettura principale della mail per lo scenario). Il marcatore serve alla
//     regola unica dell'identità (T-B4-20, T-B4-06 rivisto, T-B4-30); la revisione è l'indizio del codice del target
//     confermato (T-E1-06), che ancoraggio usa solo con l'autorità confermata.
//   - Origine: l'origine del target (confermato o scenario). VersioneDecisione resta vuota: l'impronta della decisione
//     che fa il target non è un dato della fotografia (dubbio T-B5-06).
//   - FonteConfermata: il riferimento della fonte solo quando la fonte del prodotto è confermata (lo stato dalla vista,
//     R65 A): dice quale struttura è la BOM di lavoro (R76 A). Una fonte in attesa (anche il riferimento superato) o
//     non calcolata non lo passa. Il suo AllegatoID è l'allegato portatore di quel contenuto (allegatoPortatore, dubbio
//     T-B5-15), che può non essere quello del riferimento di B1: ProdottoValutato.Fonte.Riferimento resta com'è.
func (c *collegamento) prodottiRichiesti(targets []target) []ancoraggio.ProdottoRichiesto {
	out := make([]ancoraggio.ProdottoRichiesto, 0, len(targets))
	for _, tg := range targets {
		pv := tg.pv
		origine := ancoraggio.OrigineConfermato
		if pv.Autorita == ancoraggio.AutoritaScenario {
			origine = ancoraggio.OrigineScenario
		}
		p := ancoraggio.ProdottoRichiesto{Rif: pv.Rif, Autorita: pv.Autorita, ClienteID: c.t.ClienteID, CodiceRichiesto: pv.CodiceRichiesto,
			Base: copiaBase(pv.Base), Origine: string(origine), ComponenteID: copiaUUID(pv.ComponenteID)}
		if l := tg.lettura; l != nil {
			p.Namespace, p.Marcatore = l.Namespace, marcatoreDi(*l)
			p.Revisione = copiaRevisione(l.Revisione)
			p.Qualificatori = copiaAffissi(l.Affissi)
		}
		if pv.Fonte.Calcolata && pv.Fonte.Stato == FonteConfermata && pv.Fonte.Riferimento != nil {
			r := *pv.Fonte.Riferimento
			r.AllegatoID, r.ConfermatoDa = c.allegatoPortatore(r), copiaUUID(r.ConfermatoDa)
			p.FonteConfermata = &r
		}
		out = append(out, p)
	}
	return out
}

// allegatoPortatore: l'allegato su cui ancoraggio costruisce la BOM di lavoro di una fonte confermata (dubbio T-B5-15;
// R80, T-B0-24, R76 A). Lo stesso STEP arrivato due volte nella RFQ ha le righe legacy (componente_proposta e
// relazione_proposta) su un allegato solo, il portatore del legacy (PortatoreDelFile: lo stesso contenuto propone una
// volta sola), mentre il riferimento di B1 prende il primo allegato del documento confermato, che può essere la copia.
// Sulla copia nessun nodo troverebbe la sua riga, e la verifica perderebbe le righe da decidere e i conflitti. Quindi:
// l'allegato del riferimento se porta righe di componente_proposta di quel contenuto; altrimenti, fra gli allegati
// con la struttura di quel contenuto, il primo in ordine di ID che le porta (la fotografia non ha creato_il delle righe,
// che il legacy guarda per primo: il portatore è comunque uno solo); se nessuno le porta, l'allegato del riferimento.
// È sempre lo stesso contenuto (lo stesso sha256), quindi la stessa struttura.
func (c *collegamento) allegatoPortatore(r ancoraggio.RiferimentoFonte) *uuid.UUID {
	if r.Sha256 == "" {
		return copiaUUID(r.AllegatoID)
	}
	porta := map[uuid.UUID]bool{}
	for _, riga := range c.t.RigheComponenteProposta {
		if riga.Sha256 == r.Sha256 {
			porta[riga.AllegatoID] = true
		}
	}
	if r.AllegatoID != nil && porta[*r.AllegatoID] {
		return copiaUUID(r.AllegatoID)
	}
	for _, s := range c.strutture { // in ordine di allegato (nuovoCollegamento)
		if s.Sha256 == r.Sha256 && porta[s.AllegatoID] {
			id := s.AllegatoID
			return &id
		}
	}
	return copiaUUID(r.AllegatoID)
}

// contestoStrutturale: il contesto di ProponiAncoraggi dalla fotografia (contratto §1.3, §1.4, §1.5; T-B0-33, T-E1-22,
// T-08, T-E1-06, T-B4-33).
//   - Strutture: le strutture STEP dei file (StrutturaDa).
//   - Confermato: i componenti non archiviati (un componente archiviato è uscito dalla BOM con una decisione), con
//     componente.rev e il codice letto con la grammatica (R31 c; nil se non si legge).
//   - ArchiConfermati: le relazioni confermate fra due di quei componenti, con la quantità.
//   - Proposto: le righe aperte di componente_proposta; il codice manuale solo con origine_codice = operatore, letto con
//     la grammatica (T-B0-33): il codice del motore legacy di ogni altra riga non entra mai.
//   - Decise: le righe confermate, duplicato o scartate, con l'allegato, lo sha256, la chiave, lo stato, il componente e
//     chi ha deciso: mai codice né revisione (contratto §3, T-E1-22).
//   - ArchiProposti: le righe aperte di relazione_proposta, fra i nodi del contenuto del loro allegato.
//   - CodiciProposti: il compositore (motorea.ComponiCodiceDocumentale) su ogni lettura d'identità dei nodi delle
//     strutture, con la chiave ChiaveCodiceProposto (T-08).
//   - CodiciMessaggi: le letture del motore A nel testo dei messaggi del thread, con la posizione dell'occorrenza
//     (Assoluto, o la posizione dell'unità: T-B4-04).
//   - AssociazioniDecise: i documenti confermati correnti su un componente, per ogni loro allegato fra i file
//     (documento_provenienza; confermato, con il documento), e le proposte aperte con «assegna» (manuale).
//
// Una riga di un altro contenuto della struttura del suo allegato, o senza allegato, sha256 o chiave, non può essere un
// nodo di nessuna struttura e resta fuori, come una riga confermata o duplicato senza componente e una riga con uno
// stato fuori elenco: il DB non le ammette, e così un dato incoerente non ferma il thread (dubbio T-B5-07).
func (c *collegamento) contestoStrutturale(interpretati []ancoraggio.MessaggioInterpretato) ancoraggio.ContestoStrutturale {
	ctx := ancoraggio.ContestoStrutturale{Strutture: c.strutture}

	componenti := append([]fotorfq.Componente(nil), c.t.Componenti...)
	sort.SliceStable(componenti, func(i, j int) bool { return componenti[i].ID.String() < componenti[j].ID.String() })
	attivi := map[uuid.UUID]bool{}
	for _, k := range componenti {
		if k.ArchiviatoIl != nil {
			continue
		}
		attivi[k.ID] = true
		d := ancoraggio.ComponenteDeciso{ComponenteID: k.ID, Codice: k.Codice, Autorita: ancoraggio.AutoritaConfermata,
			Origine: ancoraggio.OrigineConfermato, Rev: copiaTesto(k.Rev)}
		if l, ok := leggiCodiceRegistrato(c.m, k.Codice); ok {
			d.Lettura = &l
		}
		ctx.Confermato = append(ctx.Confermato, d)
	}
	for _, r := range relazioniAttive(c.t.Relazioni, attivi) {
		q := r.Qta
		ctx.ArchiConfermati = append(ctx.ArchiConfermati, ancoraggio.ArcoPercorso{Padre: ancoraggio.RifComponente(r.PadreID),
			Figlio: ancoraggio.RifComponente(r.FiglioID), Quantita: &q, Origine: ancoraggio.OrigineArcoConfermato})
	}

	righe := append([]fotorfq.RigaComponenteProposta(nil), c.t.RigheComponenteProposta...)
	sort.SliceStable(righe, func(i, j int) bool { return righe[i].ID.String() < righe[j].ID.String() })
	for _, r := range righe {
		if r.AllegatoID == uuid.Nil || r.Sha256 == "" || r.Chiave == "" || !c.stessoContenuto(r.AllegatoID, r.Sha256) {
			continue
		}
		switch r.Stato {
		case statoPropostaAperta:
			p := ancoraggio.RigaPropostaLegacy{ID: r.ID, AllegatoID: r.AllegatoID, Sha256: r.Sha256, Chiave: r.Chiave,
				Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}
			if r.OrigineCodice != nil && *r.OrigineCodice == origineCodiceOperatore && r.Codice != nil && strings.TrimSpace(*r.Codice) != "" {
				p.CodiceManuale, p.RevManuale = copiaTesto(r.Codice), copiaTesto(r.Rev)
				if l, ok := leggiCodiceRegistrato(c.m, *r.Codice); ok {
					p.LetturaManuale = &l
				}
			}
			ctx.Proposto = append(ctx.Proposto, p)
		case statoPropostaConfermata, statoPropostaDuplicato, statoPropostaScartata:
			if r.Stato != statoPropostaScartata && r.ComponenteID == nil {
				continue
			}
			ctx.Decise = append(ctx.Decise, ancoraggio.RigaDecisaLegacy{ID: r.ID, AllegatoID: r.AllegatoID, Sha256: r.Sha256, Chiave: r.Chiave,
				Stato: r.Stato, ComponenteID: copiaUUID(r.ComponenteID), DecisoDa: copiaUUID(r.DecisoDa)})
		}
	}
	for _, r := range c.t.RigheRelazioneProposta {
		sha := c.shaDellAllegato(r.AllegatoID)
		if r.Stato != statoPropostaAperta || sha == "" || r.PadreChiave == "" || r.FiglioChiave == "" {
			continue
		}
		q := r.Qta
		ctx.ArchiProposti = append(ctx.ArchiProposti, ancoraggio.ArcoPercorso{Padre: ancoraggio.RifNodo(sha, r.PadreChiave),
			Figlio: ancoraggio.RifNodo(sha, r.FiglioChiave), Quantita: &q, Origine: ancoraggio.OrigineArcoProposto})
	}

	if c.m != nil {
		ctx.CodiciProposti = map[string]motorea.CodiceComposto{}
		for _, s := range c.strutture {
			for _, n := range s.Nodi {
				for _, l := range n.Letture {
					ctx.CodiciProposti[ancoraggio.ChiaveCodiceProposto(s.BundleID, l.ID)] = c.m.ComponiCodiceDocumentale(l.Forma)
				}
			}
		}
	}
	for _, mi := range interpretati {
		pos := make(map[string]evidenze.Localizzatore, len(mi.Documento.Unita))
		for _, u := range mi.Documento.Unita {
			pos[u.ID] = u.Posizione
		}
		for _, l := range mi.Interpretazione.Letture {
			p := pos[l.UnitaID]
			if l.Assoluto != nil {
				a := *l.Assoluto
				p = evidenze.Localizzatore{Tipo: tipoLocalizzatoreTesto, Testo: &a}
			}
			ctx.CodiciMessaggi = append(ctx.CodiciMessaggi, ancoraggio.CodiceDiMessaggio{MessaggioID: mi.MessaggioID, Lettura: l, Posizione: p})
		}
	}
	ctx.AssociazioniDecise = c.associazioniDecise()
	return ctx
}

// tipoLocalizzatoreTesto: il tipo del localizzatore di un intervallo su un testo originale (evidenze.Localizzatore).
const tipoLocalizzatoreTesto = "testo"

// stessoContenuto: la riga è dello stesso contenuto della struttura del suo allegato, o l'allegato non ha una struttura.
func (c *collegamento) stessoContenuto(allegato uuid.UUID, sha string) bool {
	s, ok := c.shaStruttura[allegato]
	return !ok || s == sha
}

// shaDellAllegato: lo sha256 del contenuto di un allegato del thread: quello della sua struttura, se c'è, altrimenti
// quello dell'allegato; "" se non si sa.
func (c *collegamento) shaDellAllegato(allegato uuid.UUID) string {
	if s, ok := c.shaStruttura[allegato]; ok {
		return s
	}
	if a := c.allegati[allegato]; a != nil && a.Sha256 != nil {
		return *a.Sha256
	}
	return ""
}

// associazioniDecise: le associazioni dei file ai componenti decise nel DB (contratto §1.5; T-B4-33), per gli allegati
// che sono fra i file: i documenti confermati correnti (sostituito_da nullo) su un componente, uno per allegato di
// documento_provenienza, con il documento; le proposte aperte con il componente («assegna», manuale, senza chi né quando:
// LD-16). Un documento sostituito non è più la decisione corrente (dubbio T-B5-08). In ordine, senza doppioni.
func (c *collegamento) associazioniDecise() []ancoraggio.AssociazioneDecisa {
	var out []ancoraggio.AssociazioneDecisa
	visti := map[string]bool{}
	aggiungi := func(a ancoraggio.AssociazioneDecisa) {
		k := a.AllegatoID.String() + "\x00" + a.ComponenteID.String() + "\x00" + string(a.Origine) + "\x00" + testoUUID(a.DocumentoID)
		if !visti[k] {
			visti[k] = true
			out = append(out, a)
		}
	}
	docs := append([]fotorfq.DocumentoConfermato(nil), c.t.Documenti...)
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].ID.String() < docs[j].ID.String() })
	for _, d := range docs {
		if d.ComponenteID == nil || d.SostituitoDa != nil {
			continue
		}
		for _, a := range d.Allegati {
			if c.sonoFile[a] {
				aggiungi(ancoraggio.AssociazioneDecisa{AllegatoID: a, DocumentoID: copiaUUID(&d.ID), ComponenteID: *d.ComponenteID, Origine: ancoraggio.OrigineConfermato})
			}
		}
	}
	proposte := append([]fotorfq.PropostaAttuale(nil), c.t.Proposte...)
	sort.SliceStable(proposte, func(i, j int) bool { return proposte[i].ID.String() < proposte[j].ID.String() })
	for _, p := range proposte {
		if p.Stato == statoPropostaAperta && p.ComponenteID != nil && c.sonoFile[p.AllegatoID] {
			aggiungi(ancoraggio.AssociazioneDecisa{AllegatoID: p.AllegatoID, ComponenteID: *p.ComponenteID, Origine: ancoraggio.OrigineManuale})
		}
	}
	return out
}

// relazioniAttive: le relazioni confermate fra due componenti attivi (non archiviati), in ordine di (padre, figlio).
// ListRelazioniDellaRfq porta anche gli archi fra componenti archiviati, che non sono più nella BOM (R62 e A).
func relazioniAttive(relazioni []fotorfq.Relazione, attivi map[uuid.UUID]bool) []fotorfq.Relazione {
	var out []fotorfq.Relazione
	for _, r := range relazioni {
		if attivi[r.PadreID] && attivi[r.FiglioID] {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PadreID != out[j].PadreID {
			return out[i].PadreID.String() < out[j].PadreID.String()
		}
		return out[i].FiglioID.String() < out[j].FiglioID.String()
	})
	return out
}

// ---- le copie: l'uscita non condivide memoria con la fotografia né con le letture ----

func copiaUUID(p *uuid.UUID) *uuid.UUID {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copiaTesto(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copiaRevisione(r *motorea.RevisioneLetta) *motorea.RevisioneLetta {
	if r == nil {
		return nil
	}
	v := *r
	v.Segmenti = append([]motorea.SegmentoLetto(nil), r.Segmenti...)
	if len(v.Segmenti) == 0 {
		v.Segmenti = nil
	}
	if r.Token != nil {
		tok := *r.Token
		v.Token = &tok
	}
	return &v
}

func copiaAffissi(a []motorea.AffissoLetto) []motorea.AffissoLetto {
	if len(a) == 0 {
		return nil
	}
	out := make([]motorea.AffissoLetto, len(a))
	for i, x := range a {
		if x.Valore != nil {
			v := *x.Valore
			x.Valore = &v
		}
		out[i] = x
	}
	return out
}

func testoUUID(p *uuid.UUID) string {
	if p == nil {
		return ""
	}
	return p.String()
}
