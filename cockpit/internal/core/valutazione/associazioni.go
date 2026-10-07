package valutazione

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// associazioni.go: l'adattatore dello smistamento di un thread (fase V2 di B6; contratto §1.0 riga 5, §1.5, §1.7 righe
// «Conflitti con le decisioni» e «Percorso di revisione tecnica», §2.3, §2.5, §2.6; E1R §6; R81, R85, R91, R93, R95 A,
// R105; T-B0-12, T-B0-25, T-B0-29, T-B0-32; T-E1-03, T-E1-05, T-E1-11, T-E1-12, T-E1-13, T-E1-15; T-E1R-08, T-E1R-10,
// T-E1R-11). Dalla fotografia e dagli ancoraggi del thread (B5):
//   - per ogni allegato, l'associazione (AssociazioneFile): la proposta del motore A, la destinazione F8, l'«assegna», il
//     documento confermato, l'esclusione con il gesto, lo stato terminale, il perimetro di R93 (b), la pertinenza;
//   - «da smistare» (FileDaSmistare), con gli orfani;
//   - i conflitti dell'asse smistamento (associazione, nuovo_file, identita_documento), come pezzi per prodotto: Calcola
//     li compone con quelli di nomenclatura e di gerarchia (componiConflitti);
//   - per ogni prodotto target, l'ingresso della regola dello smistamento e la regola (Smistamento), senza percorsi di
//     revisione: in A1c nessun adattatore li produce (LD-18).
//
// Qui si sceglie solo che cosa della fotografia entra; le regole sono in smistamento.go e pertinenza.go. Lo scarto è solo
// un gesto (T-E1R-10): una proposta scartata con deciso_da, nessun calcolo. Niente si scrive, niente si cancella (LD-26).

// I valori del DB che l'adattatore guarda, ripetuti qui perché i pacchetti che li dichiarano non li esportano: le altre
// nature di un allegato che non sono un file (0001, natura_allegato; «inline» è in fonte.go).
const (
	naturaElementoOutlook = "elemento_outlook"
	naturaCollegamento    = "collegamento"
)

// I motivi di un conflitto di associazione (Conflitto.Motivo; T-B0-25, T-E1-05; dubbio T-B6-66, deciso
// dall'orchestratore: i due valori restano [T], il secondo con la parola di ancoraggio):
//   - componente_diverso: l'identità letta dal file, nella fotografia di adesso, porta ad altri candidati, nessuno sul
//     componente deciso;
//   - codice_confermato: un candidato sta sul componente deciso, ma il codice del file non è più compatibile con il codice
//     confermato del componente (la dimensione codice_confermato di ancoraggio, discordante: dopo una rinomina, PO-21).
const (
	MotivoConflittoComponenteDiverso = "componente_diverso"
	MotivoConflittoCodiceConfermato  = ancoraggio.DimensioneCodiceConfermato
)

// rifAllegato: il riferimento di un allegato del thread, «allegato:<uuid>», com'è in evidenze.DocumentoEvidenze.ID e in
// ancoraggio (che non lo esporta): l'aiuto solo del pacchetto (T-B6-66). Serve al file senza documento in un conflitto
// dell'asse smistamento (l'«assegna» in conflitto; il file nuovo di nuovo_file) e alla chiave di contenuto di un 2D senza
// sha256 (disegni_thread.go).
func rifAllegato(id uuid.UUID) string { return "allegato:" + id.String() }

// sezioniDelloSmistamento: le sezioni della fotografia da cui lo smistamento dipende (T-12): senza una di loro lo
// smistamento dei prodotti non si calcola (Calcolata falso, da_verificare).
var sezioniDelloSmistamento = []string{fotorfq.SezioneAllegati, fotorfq.SezioneMessaggi, fotorfq.SezioneProposteDocumento,
	fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze, fotorfq.SezioneComponenti, fotorfq.SezioneRelazioni,
	fotorfq.SezioneRigheComponenteProposta}

// smistamentoThread: ciò che serve allo smistamento di un thread, calcolato una volta. Le mappe servono solo a cercare,
// mai a scorrere: si scorrono gli elenchi della fotografia copiati e messi in ordine.
type smistamentoThread struct {
	t            fotorfq.Thread
	m            *motorea.Motore
	col          *collegamento
	esito        ancoraggio.EsitoAncoraggi
	targets      []target           // nell'ordine dei prodotti: la lettura del codice di ognuno
	prodotti     []ProdottoValutato // i prodotti target con gli assi di B1 e B5
	target       map[string]bool    // i Rif dei target
	interpretati map[uuid.UUID]ancoraggio.MessaggioInterpretato
	dis          *disegniThread
	vecchio      *vecchioDelThread
	messaggi     map[uuid.UUID]*fotorfq.Messaggio
	contenitori  map[uuid.UUID]bool
	ancoraggi    map[uuid.UUID]*ancoraggio.AncoraggioFile
	letti        map[uuid.UUID]*ancoraggio.FileInterpretato
	perimetri    []map[uuid.UUID]bool // per prodotto: la BOM confermata (R62 e A)
	nodi         []map[string]bool    // per prodotto: i nodi di tutte le sue strutture
	contano      []map[string]bool    // per prodotto: i nodi delle strutture che contano (struttureDellaVerifica)
	righe        map[uuid.UUID]*fotorfq.RigaComponenteProposta
	correnti     map[uuid.UUID][]fotorfq.DocumentoConfermato // per componente: i documenti correnti, in ordine di ID
	t12          bool
}

// fileDelThread: un allegato del thread con ciò che lo smistamento ne sa.
type fileDelThread struct {
	a            fotorfq.Allegato
	af           *ancoraggio.AncoraggioFile   // l'ancoraggio di B5; nil per un allegato che l'adattatore non legge
	letto        *ancoraggio.FileInterpretato // il file letto e interpretato; nil come sopra
	perimetro    string
	proposta     *fotorfq.PropostaAttuale
	documento    *fotorfq.DocumentoConfermato
	tipo         string // il tipo documentale: dal documento, se c'è, altrimenti dalla proposta (T-B4-31)
	esclusa      bool
	terminale    bool
	manuale      *uuid.UUID
	destinazione []string
	messaggio    *ContestoMessaggio // il contesto del messaggio, con la provenienza, per ogni file che conta (R106, R107)
	contesto     []string           // il prodotto della pertinenza per il contesto del messaggio (uno)
	nominati     []string           // i prodotti nominati dal messaggio, quando sono più d'uno (l'orfano con ProdottiContesto)
	collegati    []string           // i prodotti delle evidenze o della decisione, per contesto_discorde (prodottiCollegati)
	pertinente   []string
	orfano       bool
	conflitto    bool
}

// conta: il file conta nel controllo (il perimetro di R93 b).
func (x *fileDelThread) conta() bool { return x.perimetro == PerimetroDentro }

// esitoSmistamento: lo smistamento di un thread: un'associazione per allegato, i file da smistare, i conflitti dell'asse
// (pezzi per prodotto), lo smistamento di ogni prodotto nell'ordine dei prodotti, le diagnostiche dello smistamento
// (valutazione.contesto_discorde, in ordine di allegato: R106 B, precisata il 07/10).
type esitoSmistamento struct {
	associazioni []AssociazioneFile
	daSmistare   []FileDaSmistare
	conflitti    []Conflitto
	smistamenti  []VerificaSmistamento
	diagnostiche []evidenze.Diagnostica
}

func nuovoSmistamentoThread(f fotorfq.Fotografia, t fotorfq.Thread, m *motorea.Motore, col *collegamento, esito ancoraggio.EsitoAncoraggi,
	targets []target, prodotti []ProdottoValutato, interpretati []ancoraggio.MessaggioInterpretato, b *bomDelThread, dis *disegniThread) *smistamentoThread {
	s := &smistamentoThread{t: t, m: m, col: col, esito: esito, targets: targets, prodotti: prodotti, target: map[string]bool{},
		interpretati: map[uuid.UUID]ancoraggio.MessaggioInterpretato{}, dis: dis, vecchio: nuovoVecchioDelThread(t, m),
		messaggi: map[uuid.UUID]*fotorfq.Messaggio{}, contenitori: contenitoriDi(t.Allegati), ancoraggi: map[uuid.UUID]*ancoraggio.AncoraggioFile{},
		letti: map[uuid.UUID]*ancoraggio.FileInterpretato{}, righe: map[uuid.UUID]*fotorfq.RigaComponenteProposta{},
		correnti: map[uuid.UUID][]fotorfq.DocumentoConfermato{}}
	for _, pv := range prodotti {
		s.target[pv.Rif] = true
	}
	for _, mi := range interpretati {
		s.interpretati[mi.MessaggioID] = mi
	}
	for i := range t.Messaggi {
		s.messaggi[t.Messaggi[i].ID] = &t.Messaggi[i]
	}
	for i := range esito.File {
		s.ancoraggi[esito.File[i].AllegatoID] = &esito.File[i]
	}
	for i := range col.file {
		s.letti[col.file[i].AllegatoID] = &col.file[i]
	}
	for i := range t.RigheComponenteProposta {
		s.righe[t.RigheComponenteProposta[i].ID] = &t.RigheComponenteProposta[i]
	}
	docs := append([]fotorfq.DocumentoConfermato(nil), t.Documenti...)
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].ID.String() < docs[j].ID.String() })
	for _, d := range docs {
		if d.ComponenteID != nil && d.SostituitoDa == nil {
			s.correnti[*d.ComponenteID] = append(s.correnti[*d.ComponenteID], d)
		}
	}
	for _, pv := range prodotti {
		perimetro, _ := b.perimetro(pv.ComponenteID)
		s.perimetri = append(s.perimetri, perimetro)
		tutti, contano := map[string]bool{}, map[string]bool{}
		for _, st := range esito.Strutture {
			if st.Target != pv.Rif {
				continue
			}
			for _, n := range st.Nodi {
				tutti[n.Rif] = true
			}
		}
		for _, st := range struttureDellaVerifica(esito.Strutture, pv.Rif) {
			for _, n := range st.Nodi {
				contano[n.Rif] = true
			}
		}
		s.nodi, s.contano = append(s.nodi, tutti), append(s.contano, contano)
	}
	for _, k := range sezioniDelloSmistamento {
		s.t12 = s.t12 || sezioneAssente(f.Sezioni, k)
	}
	return s
}

// calcola: lo smistamento del thread.
//  1. Per ogni allegato, in ordine di ID: il perimetro, la proposta, il documento, l'esclusione, lo stato terminale,
//     l'«assegna», la destinazione F8 (fileDelThread); poi la pertinenza, con il contesto del messaggio (pertinenza.go), e
//     la contraddizione fra il contesto e le evidenze o la decisione (diagnosticaContestoDiscorde: R106 B).
//  2. I conflitti dell'asse smistamento: di associazione (conflittoDiAssociazione: ai prodotti con il componente deciso
//     nel perimetro e a quelli a cui il file è pertinente), nuovo_file (conflittiNuovoFile: ai prodotti con il componente
//     nel perimetro), identita_documento dai pezzi dei 2D di B5, con i prodotti a cui il documento è pertinente
//     (conflittiIdentita).
//  3. Le associazioni, uno per allegato, e «da smistare» (voceDaSmistare: R108).
//  4. Lo smistamento di ogni prodotto: l'ingresso (ingressoDelProdotto) e la regola, senza percorsi.
func (s *smistamentoThread) calcola() esitoSmistamento {
	allegati := append([]fotorfq.Allegato(nil), s.t.Allegati...)
	sort.SliceStable(allegati, func(i, j int) bool { return allegati[i].ID.String() < allegati[j].ID.String() })
	var file []*fileDelThread
	perAllegato := map[uuid.UUID]*fileDelThread{}
	for _, a := range allegati {
		if perAllegato[a.ID] != nil {
			continue
		}
		x := s.fileDelThread(a)
		s.pertinenza(x)
		file = append(file, x)
		perAllegato[a.ID] = x
	}

	var out esitoSmistamento
	for _, x := range file {
		if d, ok := s.diagnosticaContestoDiscorde(x); ok {
			out.diagnostiche = append(out.diagnostiche, d)
		}
	}

	var pezzi []Conflitto
	for _, x := range file {
		pezzi = append(pezzi, s.conflittoDiAssociazione(x)...)
		pezzi = append(pezzi, s.conflittiNuovoFile(x)...)
	}
	if s.dis != nil {
		pezzi = append(pezzi, s.conflittiIdentita(s.dis.conflitti, perAllegato)...)
	}
	out.conflitti = componiConflitti(pezzi)

	for _, x := range file {
		out.associazioni = append(out.associazioni, s.associazione(x))
		if motivo, ok := voceDaSmistare(x); ok {
			d := FileDaSmistare{AllegatoID: x.a.ID, NomeFile: x.a.NomeFile, Motivo: motivo, Prodotti: append([]string(nil), x.pertinente...),
				Orfano: x.orfano, ProdottiContesto: append([]string(nil), x.nominati...)}
			if x.af != nil && x.af.Associazione == ancoraggio.AssociazioneCandidatoUnico && len(x.af.Candidati) == 1 {
				d.Proposta = chiaveCandidato(x.af.Candidati[0])
			}
			if len(d.Prodotti) == 0 {
				d.Prodotti = nil
			}
			if len(d.ProdottiContesto) == 0 {
				d.ProdottiContesto = nil
			}
			out.daSmistare = append(out.daSmistare, d)
		}
	}
	for i := range s.prodotti {
		out.smistamenti = append(out.smistamenti, Smistamento(s.ingressoDelProdotto(i, file, out.conflitti), nil))
	}
	return out
}

// fileDelThread: un allegato con il perimetro (perimetroDi), la proposta e il documento scelti come per il vecchio
// (vecchioDelThread: la proposta dell'allegato, il documento che lo porta in documento_provenienza, prima i correnti), il
// tipo documentale e:
//   - Esclusa (T-E1R-10, R105): solo la proposta scartata con deciso_da, il gesto «scarta» dell'utente. Una proposta
//     scartata senza deciso_da non è uno scarto, e niente altro lo è: un orfano, un ambiguo, un file senza candidati, un
//     fuori_richiesta proposto, un rumore proposto o riproposto da hash_rumore, un PDF che non si apre;
//   - Terminale (T-B0-32): un documento confermato che lo porta (su un componente, o il documento generale, senza
//     componente), la proposta duplicato con deciso_da, la proposta scartata con deciso_da;
//   - l'«assegna»: il componente di una proposta aperta (manuale, senza chi né quando: LD-16);
//   - la destinazione F8: le chiavi di dettagli.destinazione della proposta (leggiDettagli, come per il vecchio), nil negli
//     export.
func (s *smistamentoThread) fileDelThread(a fotorfq.Allegato) *fileDelThread {
	x := &fileDelThread{a: a, af: s.ancoraggi[a.ID], letto: s.letti[a.ID], tipo: s.col.tipi[a.ID]}
	x.perimetro = s.perimetroDi(a)
	if p, ok := s.vecchio.proposte[a.ID]; ok {
		x.proposta = &p
		x.esclusa = p.Stato == statoPropostaScartata && p.DecisoDa != nil
		if p.Stato == statoPropostaAperta && p.ComponenteID != nil {
			x.manuale = copiaUUID(p.ComponenteID)
		}
		_, x.destinazione = leggiDettagli(p.Dettagli)
	}
	if d, ok := s.vecchio.documenti[a.ID]; ok {
		x.documento = &d
	}
	duplicato := x.proposta != nil && x.proposta.Stato == statoPropostaDuplicato && x.proposta.DecisoDa != nil
	x.terminale = x.documento != nil || duplicato || x.esclusa
	return x
}

// perimetroDi: il perimetro di R93 (b) A [R] (T-E1-12), dai campi della fotografia, il primo che vale:
//   - inline, elemento_outlook (un .msg come contenitore), collegamento: la natura dell'allegato;
//   - contenitore_estratto: un archivio, o un .msg, di cui il worker ha estratto le voci (contano le voci);
//   - messaggio_in_uscita: il messaggio dell'allegato esterno è in uscita e non è interno (un'offerta di Promatec). Un
//     messaggio interno (un inoltro fra colleghi) conta: porta i file del cliente (R48);
//   - altra_controparte: la controparte del messaggio è un fornitore, o un cliente diverso da quello del thread;
//   - dentro: tutto il resto, anche le controparti sconosciuto, ambiguo, interno e altro (per prudenza: dubbio T-B6-65), e
//     i file che il sistema propone come rumore, corrispondenza o altro, finché una persona non li esclude (R93 b A).
//
// Un file fuori perimetro si mostra a parte, con il motivo, e non entra né in «da smistare» né fra gli orfani; non è
// pertinente a nessun prodotto e non toglie nessun fabbisogno. Il suo documento confermato contraddetto dall'identità
// letta adesso dà lo stesso il conflitto di associazione (R-72: conflittoDiAssociazione).
func (s *smistamentoThread) perimetroDi(a fotorfq.Allegato) string {
	switch {
	case a.Natura == naturaInline:
		return PerimetroInline
	case a.Natura == naturaElementoOutlook:
		return PerimetroElementoOutlook
	case a.Natura == naturaCollegamento:
		return PerimetroCollegamento
	case s.contenitori[a.ID]:
		return PerimetroContenitoreEstratto
	}
	m := s.messaggi[s.allegatoEsterno(a).MessaggioID]
	switch {
	case m == nil, m.Interno:
		return PerimetroDentro
	case m.Direzione == direzioneUscita:
		return PerimetroMessaggioInUscita
	case m.ControparteTipo == controparteFornitore:
		return PerimetroAltraControparte
	case m.ControparteTipo == controparteCliente && m.ControparteClienteID != nil && *m.ControparteClienteID != s.t.ClienteID:
		return PerimetroAltraControparte
	}
	return PerimetroDentro
}

// associazione: il record dell'allegato (contratto §1.5, §2.3, §2.5, §2.6):
//   - Associazione, Collocazione, Proposta: dall'ancoraggio di B5 (non_valutata, senza collocazione, per un allegato che
//     l'adattatore non legge: un contenitore, una natura che non è un file, un documento che non si legge);
//   - Confermata, DocumentoID: il documento confermato che porta il file; Manuale: l'«assegna»;
//   - Origine: la più forte, confermato (un documento), poi manuale (l'«assegna»), poi proposto (un candidato);
//   - DestinazioneCoincide (destinazioneCoincide), Conflitto (il file è in un conflitto dell'asse smistamento), Esclusa,
//     Terminale, Pertinente, Perimetro, PertinenzaContesto;
//   - Contesto: il contesto del messaggio con la sua provenienza (prodottiNominati), per ogni file che conta, copiato.
func (s *smistamentoThread) associazione(x *fileDelThread) AssociazioneFile {
	out := AssociazioneFile{AllegatoID: x.a.ID, Associazione: ancoraggio.AssociazioneNonValutata, DestinazioneF8: append([]string(nil), x.destinazione...),
		Manuale: copiaUUID(x.manuale), Conflitto: x.conflitto, Esclusa: x.esclusa, Terminale: x.terminale,
		Pertinente: append([]string(nil), x.pertinente...), Perimetro: x.perimetro, PertinenzaContesto: append([]string(nil), x.contesto...)}
	if x.af != nil {
		out.Associazione, out.Collocazione = x.af.Associazione, x.af.Collocazione
		for _, c := range x.af.Candidati {
			out.Proposta = append(out.Proposta, chiaveCandidato(c))
		}
	}
	if d := x.documento; d != nil {
		id := d.ID
		out.Confermata, out.DocumentoID = copiaUUID(d.ComponenteID), &id
	}
	switch {
	case x.documento != nil:
		out.Origine = ancoraggio.OrigineConfermato
	case x.manuale != nil:
		out.Origine = ancoraggio.OrigineManuale
	case x.af != nil && len(x.af.Candidati) > 0:
		out.Origine = ancoraggio.OrigineProposto
	}
	out.DestinazioneCoincide = s.destinazioneCoincide(x)
	if len(out.DestinazioneF8) == 0 {
		out.DestinazioneF8 = nil
	}
	if len(out.Pertinente) == 0 {
		out.Pertinente = nil
	}
	if len(out.PertinenzaContesto) == 0 {
		out.PertinenzaContesto = nil
	}
	if c := x.messaggio; c != nil {
		out.Contesto = &ContestoMessaggio{MessaggioID: c.MessaggioID, Letture: append([]string(nil), c.Letture...),
			Prodotti: append([]string(nil), c.Prodotti...)}
	}
	return out
}

// chiaveCandidato: la chiave di un candidato, «<Livello>/<Target>» (F0-12; T-B4-28: mai il solo Target).
func chiaveCandidato(c ancoraggio.CandidatoAncoraggio) string { return c.Livello + "/" + c.Target }

// destinazioneCoincide: «corretta a mano» o «proposta e accettata» (contratto §1.5): il DB non lo sa, e si confrontano
// la destinazione F8 e il documento confermato. Si guarda la prima chiave della destinazione, quella che lo Smistamento
// legacy mette in testa (dubbio T-B6-64): coincide se dice il componente del documento (o «generale» per un documento
// senza componente). nil = non determinabile: nessun documento, nessuna destinazione (sempre negli export), una chiave
// che non porta a un componente deciso.
func (s *smistamentoThread) destinazioneCoincide(x *fileDelThread) *bool {
	if x.documento == nil || len(x.destinazione) == 0 {
		return nil
	}
	k, generale, ok := s.componenteDellaChiave(x.destinazione[0])
	if !ok {
		return nil
	}
	coincide := x.documento.ComponenteID == nil
	if !generale {
		coincide = x.documento.ComponenteID != nil && *x.documento.ComponenteID == k
	}
	return &coincide
}

// componenteDellaChiave: il componente deciso di una chiave della destinazione F8: «componente:<uuid>»; «nodo:<id>» con
// la riga decisa (confermata o duplicato) su un componente; «identificativo:<CODICE>» del target con quel codice e il suo
// componente. «generale» è il documento senza componente. ok falso altrimenti.
func (s *smistamentoThread) componenteDellaChiave(chiave string) (uuid.UUID, bool, bool) {
	switch {
	case chiave == chiaveF8Generale:
		return uuid.Nil, true, true
	case strings.HasPrefix(chiave, chiaveF8Componente):
		k, err := uuid.Parse(strings.TrimPrefix(chiave, chiaveF8Componente))
		return k, false, err == nil
	case strings.HasPrefix(chiave, chiaveF8Nodo):
		id, err := uuid.Parse(strings.TrimPrefix(chiave, chiaveF8Nodo))
		if err != nil {
			return uuid.Nil, false, false
		}
		if r := s.righe[id]; r != nil && r.ComponenteID != nil && (r.Stato == statoPropostaConfermata || r.Stato == statoPropostaDuplicato) {
			return *r.ComponenteID, false, true
		}
	case strings.HasPrefix(chiave, chiaveF8Identificativo):
		codice := chiaveCodice(strings.TrimPrefix(chiave, chiaveF8Identificativo))
		for _, pv := range s.prodotti {
			if codice != "" && chiaveCodice(pv.CodiceRichiesto) == codice && pv.ComponenteID != nil {
				return *pv.ComponenteID, false, true
			}
		}
	}
	return uuid.Nil, false, false
}

// ---- i conflitti dell'asse smistamento ----

// conflittoDiAssociazione: il conflitto di associazione di un file (T-B0-25, T-E1-05, T-B0-12; R95 A, R62 g A; PO-04,
// PO-21). La decisione è il documento confermato corrente su un componente K (confermato, con chi e quando del
// documento) o, senza, l'«assegna» su K (manuale, senza chi né quando: LD-16; T-B0-12: un indicatore di Conflitto, la
// tavola dei badge non cambia). È «sbagliata» quando l'identità letta dal file, nella fotografia di adesso, porta a un
// altro componente: il file ha candidati e nessuno sta su K (componente_diverso), oppure un candidato sta su K ma il suo
// codice non è più compatibile con il codice confermato di K (codice_confermato: T-E1-05, dopo una rinomina). Un file
// senza candidati non porta a un altro componente: nessun conflitto (un cartiglio letto male non annulla una decisione:
// R62 g A). La decisione resta il valore corrente; nella completezza il documento resta presente (R62 g A).
//
// Il perimetro di R93 (b) vale per la seconda e la terza condizione dello smistamento, non per il conflitto di un
// documento confermato (R-72 della revisione di V2, decisione [T] dell'orchestratore): il documento soddisfa la voce
// della completezza anche fuori perimetro, e una decisione contraddetta da un'evidenza nuova non resta in silenzio
// (R95 A). L'«assegna», che non soddisfa nessuna voce (associazione_non_confermata), dà il conflitto solo per un file che
// conta (dubbio T-B6-95). Uno per prodotto (F0-13): i prodotti con K nel perimetro (prodottiDelComponente) e quelli a cui
// il file è pertinente; senza prodotti, uno con il prodotto vuoto.
func (s *smistamentoThread) conflittoDiAssociazione(x *fileDelThread) []Conflitto {
	if x.af == nil || len(x.af.Candidati) == 0 {
		return nil
	}
	var k uuid.UUID
	var rif string
	var dec EvidenzaDecisione
	var doc *uuid.UUID
	switch d := x.documento; {
	case d != nil && d.ComponenteID != nil && d.SostituitoDa == nil:
		k, rif = *d.ComponenteID, RifDocumento(d.ID)
		da, il := d.ConfermatoDa, d.ConfermatoIl.UTC().Truncate(time.Millisecond)
		dec = EvidenzaDecisione{Origine: ancoraggio.OrigineConfermato, Da: &da, Il: &il}
		doc = copiaUUID(&d.ID)
	case x.manuale != nil && x.conta():
		k, rif = *x.manuale, rifAllegato(x.a.ID)
		dec = EvidenzaDecisione{Origine: ancoraggio.OrigineManuale}
	default:
		return nil
	}
	su, discorde := false, false
	for _, c := range x.af.Candidati {
		if s.candidatoSu(c, k) {
			su = true
			discorde = discorde || codiceConfermatoDiscordante(c, k)
		}
	}
	motivo := MotivoConflittoCodiceConfermato
	switch {
	case !su:
		motivo = MotivoConflittoComponenteDiverso
	case !discorde:
		return nil
	}
	var proposte []string
	for _, c := range x.af.Candidati {
		proposte = append(proposte, chiaveCandidato(c))
	}
	unita, posizione := s.primaLettura(x)
	var out []Conflitto
	for _, p := range oVuoto(ordinatiUnici(append(s.prodottiDelComponente(k), x.pertinente...))) {
		aid := x.a.ID
		d := dec
		d.Da, d.Il = copiaUUID(dec.Da), copiaTempo(dec.Il)
		out = append(out, Conflitto{Tipo: ConflittoAssociazione, Asse: AsseSmistamento, Rif: rif, Prodotto: p,
			Decisione: ancoraggio.RifComponente(k), OrigineDecisione: dec.Origine, Proposta: strings.Join(proposte, "|"), Motivo: motivo,
			EvidenzaDecisione: d,
			EvidenzaProposta: EvidenzaProposta{AllegatoID: &aid, DocumentoID: copiaUUID(doc), UnitaID: unita, Posizione: posizione,
				Riferimenti: append([]string(nil), proposte...)}})
	}
	x.conflitto = true
	return out
}

// conflittiNuovoFile: i conflitti nuovo_file di un file che conta e non è terminale (T-B0-29, T-E1-14; PO-25): il file è
// candidato per un componente K (candidatoSu: il componente deciso del candidato, il componente del target per il livello
// prodotto, i nodi decisi delle posizioni) che ha già un documento confermato corrente dello stesso tipo documentale.
// Salvo per il 2D: un secondo 2D è un file pertinente, con la relazione da confermare (T-E1-08), mai un nuovo_file. Il
// tipo è quello della proposta del file (senza tipo, niente: il tipo non si inventa). Uno per (K, documento) e per
// prodotto che ha K nel perimetro (F0-13: i prodotti che condividono il componente; R-73 della revisione di V2), non per
// i prodotti a cui il file è pertinente per altre evidenze; senza prodotti, uno con il prodotto vuoto. Rif: il
// componente; Decisione: il documento di K; Proposta: il file nuovo; Motivo: il tipo documentale. Un nuovo file non apre
// mai un percorso di revisione (T-E1-13). Il perimetro di R93 (b) vale: un file che non conta non dà nuovo_file (PO-32).
func (s *smistamentoThread) conflittiNuovoFile(x *fileDelThread) []Conflitto {
	if !x.conta() || x.terminale || x.af == nil || x.tipo == "" || x.tipo == tipoDocumentoDisegno2D {
		return nil
	}
	componenti := map[uuid.UUID]bool{}
	for _, c := range x.af.Candidati {
		for _, k := range s.componentiDelCandidato(c) {
			componenti[k] = true
		}
	}
	var ordinati []uuid.UUID
	for k := range componenti {
		ordinati = append(ordinati, k)
	}
	sort.Slice(ordinati, func(i, j int) bool { return ordinati[i].String() < ordinati[j].String() })
	var proposte []string
	for _, c := range x.af.Candidati {
		proposte = append(proposte, chiaveCandidato(c))
	}
	var out []Conflitto
	for _, k := range ordinati {
		for _, d := range s.correnti[k] {
			if d.Tipo != x.tipo || portaAllegato(d, x.a.ID) {
				continue
			}
			for _, p := range oVuoto(s.prodottiDelComponente(k)) {
				aid, da, il := x.a.ID, d.ConfermatoDa, d.ConfermatoIl.UTC().Truncate(time.Millisecond)
				out = append(out, Conflitto{Tipo: ConflittoNuovoFile, Asse: AsseSmistamento, Rif: ancoraggio.RifComponente(k), Prodotto: p,
					Decisione: RifDocumento(d.ID), OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: rifAllegato(x.a.ID), Motivo: x.tipo,
					EvidenzaDecisione: EvidenzaDecisione{Origine: ancoraggio.OrigineConfermato, Da: &da, Il: &il},
					EvidenzaProposta:  EvidenzaProposta{AllegatoID: &aid, Riferimenti: append([]string(nil), proposte...)}})
			}
			x.conflitto = true
		}
	}
	return out
}

// conflittiIdentita: i conflitti identita_documento dai pezzi dei 2D di B5 (T-E1R-08; ValutaDisegno con i prodotti vuoti:
// il prodotto lo attribuisce chi compone), sullo smistamento dei prodotti a cui il documento è pertinente: i prodotti
// della pertinenza degli allegati del documento (documento_provenienza) e dell'allegato dell'evidenza. Uno per prodotto
// (F0-13); un pezzo che ha già il prodotto resta com'è; senza prodotti resta con il prodotto vuoto (mai silenzio). Gli
// allegati del documento sono in conflitto. In A1c nessun adattatore produce una DecisioneIdentita (LD-27), quindi sui
// dati veri non ce ne sono.
func (s *smistamentoThread) conflittiIdentita(pezzi []Conflitto, file map[uuid.UUID]*fileDelThread) []Conflitto {
	var out []Conflitto
	for _, c := range pezzi {
		var allegati []uuid.UUID
		if c.EvidenzaProposta.AllegatoID != nil {
			allegati = append(allegati, *c.EvidenzaProposta.AllegatoID)
		}
		if c.EvidenzaProposta.DocumentoID != nil {
			for _, d := range s.t.Documenti {
				if d.ID == *c.EvidenzaProposta.DocumentoID {
					allegati = append(allegati, d.Allegati...)
				}
			}
		}
		var prodotti []string
		for _, a := range allegati {
			if x := file[a]; x != nil {
				x.conflitto = true
				prodotti = append(prodotti, x.pertinente...)
			}
		}
		prodotti = ordinatiUnici(prodotti)
		if c.Prodotto != "" || len(prodotti) == 0 {
			out = append(out, copiaConflitto(c))
			continue
		}
		for _, p := range prodotti {
			cc := copiaConflitto(c)
			cc.Prodotto = p
			out = append(out, cc)
		}
	}
	return out
}

// candidatoSu: il candidato sta sul componente k: il suo Target è il componente deciso (livello componente), il target del
// livello prodotto ha k come componente, o una sua posizione è un nodo deciso come k (o la radice che rappresenta il
// prodotto di k: disegniThread.posizioni, la stessa raccolta dei 2D).
func (s *smistamentoThread) candidatoSu(c ancoraggio.CandidatoAncoraggio, k uuid.UUID) bool {
	for _, x := range s.componentiDelCandidato(c) {
		if x == k {
			return true
		}
	}
	return false
}

// componentiDelCandidato: i componenti decisi su cui sta un candidato (candidatoSu), in ordine, senza doppioni.
func (s *smistamentoThread) componentiDelCandidato(c ancoraggio.CandidatoAncoraggio) []uuid.UUID {
	visti := map[uuid.UUID]bool{}
	var out []uuid.UUID
	aggiungi := func(k uuid.UUID) {
		if !visti[k] {
			visti[k] = true
			out = append(out, k)
		}
	}
	if c.Livello == ancoraggio.LivelloComponente && strings.HasPrefix(c.Target, rifComponente) {
		if k, err := uuid.Parse(strings.TrimPrefix(c.Target, rifComponente)); err == nil {
			aggiungi(k)
		}
	}
	if c.Livello == ancoraggio.LivelloProdotto {
		for _, pv := range s.prodotti {
			if pv.Rif == c.Target && pv.ComponenteID != nil {
				aggiungi(*pv.ComponenteID)
			}
		}
	}
	if s.dis != nil {
		for _, p := range c.Posizioni {
			if k, ok := s.dis.posizioni[chiavePosizione(p.Target, p.AllegatoID, p.Radice, p.Nodo)]; ok {
				aggiungi(k)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// codiceConfermatoDiscordante: una dimensione codice_confermato del candidato con il componente k è discordante (T-E1-05:
// il file non è più compatibile con il codice confermato).
func codiceConfermatoDiscordante(c ancoraggio.CandidatoAncoraggio, k uuid.UUID) bool {
	rif := ancoraggio.RifComponente(k)
	for _, d := range c.Dimensioni {
		if d.Dimensione == ancoraggio.DimensioneCodiceConfermato && d.Rif == rif && d.Esito == motorea.CompatibilitaDiscordante {
			return true
		}
	}
	return false
}

// primaLettura: l'unità e il localizzatore della prima lettura d'identità che sostiene un candidato del file, per
// l'evidenza della proposta (T-E1-15); vuoti se non si trova.
func (s *smistamentoThread) primaLettura(x *fileDelThread) (string, evidenze.Localizzatore) {
	if x.letto == nil || x.af == nil {
		return "", evidenze.Localizzatore{}
	}
	letture := map[string]motorea.LetturaCodice{}
	for _, l := range x.letto.Interpretazione.Letture {
		letture[l.ID] = l
	}
	for _, c := range x.af.Candidati {
		for _, id := range c.Letture {
			l, ok := letture[id]
			if !ok {
				continue
			}
			for _, u := range x.letto.Documento.Unita {
				if u.ID == l.UnitaID {
					return u.ID, u.Posizione
				}
			}
			return l.UnitaID, evidenze.Localizzatore{}
		}
	}
	return "", evidenze.Localizzatore{}
}

// oVuoto: i prodotti di un conflitto (F0-13: uno per prodotto); senza prodotti, il prodotto vuoto, perché un conflitto
// non si perde.
func oVuoto(prodotti []string) []string {
	if len(prodotti) == 0 {
		return []string{""}
	}
	return prodotti
}

// portaAllegato: il documento porta l'allegato (documento_provenienza).
func portaAllegato(d fotorfq.DocumentoConfermato, allegato uuid.UUID) bool {
	for _, a := range d.Allegati {
		if a == allegato {
			return true
		}
	}
	return false
}

// copiaConflitto: un conflitto che non condivide memoria con quello di partenza.
func copiaConflitto(c Conflitto) Conflitto {
	c.EvidenzaDecisione.Da, c.EvidenzaDecisione.Il = copiaUUID(c.EvidenzaDecisione.Da), copiaTempo(c.EvidenzaDecisione.Il)
	c.EvidenzaProposta.AllegatoID, c.EvidenzaProposta.DocumentoID = copiaUUID(c.EvidenzaProposta.AllegatoID), copiaUUID(c.EvidenzaProposta.DocumentoID)
	c.EvidenzaProposta.Riferimenti = append([]string(nil), c.EvidenzaProposta.Riferimenti...)
	if len(c.EvidenzaProposta.Riferimenti) == 0 {
		c.EvidenzaProposta.Riferimenti = nil
	}
	if t := c.EvidenzaProposta.Posizione.Testo; t != nil {
		v := *t
		c.EvidenzaProposta.Posizione.Testo = &v
	}
	return c
}

// ---- lo smistamento di un prodotto ----

// ingressoDelProdotto: l'ingresso della regola dello smistamento del prodotto i (IngressoSmistamento):
//   - Elementi: i componenti della BOM confermata (RifComponente) e i nodi delle strutture che contano, in ordine;
//   - File: i file pertinenti al prodotto, con lo stato terminale, i due assi della proposta e l'«assegna»;
//   - Voci: le voci certe della completezza del prodotto (T-B6-08);
//   - Conflitti: i conflitti dell'asse smistamento con il prodotto;
//   - Calcolata: falsa per il target senza componente (T-B6-07, F0-17: anche lo scenario), con una sezione assente (T-12,
//     anche quelle della completezza, perché il primo requisito ne dipende), e senza grammatica (T-B6-61, confermato
//     dalla revisione di V2: le identità dei file e il contesto non si leggono, quindi la pertinenza per evidenza non è
//     completa). Con una sezione dello smistamento assente o senza grammatica nessun file è orfano (pertinenzaCalcolata).
func (s *smistamentoThread) ingressoDelProdotto(i int, file []*fileDelThread, conflitti []Conflitto) IngressoSmistamento {
	pv := s.prodotti[i]
	in := IngressoSmistamento{Prodotto: pv.Rif, Voci: pv.Documenti.Voci,
		Calcolata: pv.ComponenteID != nil && s.pertinenzaCalcolata() && pv.Documenti.Motivo != MotivoDocumentiNonDeterminabile}
	for k := range s.perimetri[i] {
		in.Elementi = append(in.Elementi, ancoraggio.RifComponente(k))
	}
	for n := range s.contano[i] {
		in.Elementi = append(in.Elementi, n)
	}
	in.Elementi = ordinatiUnici(in.Elementi)
	for _, x := range file {
		if !contiene(x.pertinente, pv.Rif) {
			continue
		}
		f := FileDelloSmistamento{AllegatoID: x.a.ID, Terminale: x.terminale, Associazione: ancoraggio.AssociazioneNonValutata, Manuale: x.manuale != nil}
		if x.af != nil {
			f.Associazione, f.Collocazione = x.af.Associazione, x.af.Collocazione
		}
		in.File = append(in.File, f)
	}
	for _, c := range conflitti {
		if c.Asse == AsseSmistamento && c.Prodotto == pv.Rif {
			in.Conflitti = append(in.Conflitti, c)
		}
	}
	return in
}
