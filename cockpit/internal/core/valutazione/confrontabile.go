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

// confrontabile.go: i record piatti per confronto (piano A, 6.4.6, «Il passaggio»; R42 B, R53 B; fase 0 di B6, CP.1–CP.3,
// F0-09). Sono i gemelli dei DTO di confronto (File, Vecchio, Nuovo, Candidato, ProdottoNuovo): stessi nomi dei campi,
// stesso ordine, stessi tipi Go, stessi tag JSON, e solo tipi delle foglie (string, bool, int, uuid.UUID, time.Time, i
// loro puntatori, gli slice e le struct gemelle), mai un tipo con nome del motore. I due pacchetti non si importano: chi
// chiama copia campo per campo (in A1c app/bancoa, passaggio.go), e una prova strutturale lo controlla (A1c-L1-31, Q9). Un
// campo cambiato qui si annuncia allo scrittore di confronto prima del commit.

// FileConfrontabile: il vecchio e il nuovo di un allegato, già scelti e piatti (confronto.File).
type FileConfrontabile struct {
	AllegatoID uuid.UUID     `json:"allegato_id"`
	Vecchio    VecchioPiatto `json:"vecchio"`
	Nuovo      NuovoPiatto   `json:"nuovo"`
}

// VecchioPiatto: il vecchio di un allegato (confronto.Vecchio; vecchioLetto). La base vecchia è sempre Base, la lettura
// della grammatica, mai la stringa del codice (R31 c). CodiceLettoBase: la base della lettura del vecchio motore
// (CodiceLetto) letta con la stessa grammatica, LeggiCodiceRegistrato(m, CodiceLetto).Base; vuota se CodiceLetto è
// vuoto o non si legge (F0-19, emendamento di CP.2). CodiceLettoMarcatore: il marcatore della stessa lettura,
// LeggiCodiceRegistrato(m, CodiceLetto).Marcatore (LetturaForma.Marcatore); vuoto se il marcatore non c'è, se
// CodiceLetto è vuoto o se non si legge (il gemello per R114, precisata dall'utente il 07/10). Servono al
// «prima» delle correzioni manuali di confronto (R30 f): la base per i cambi di base, il marcatore per contare a parte
// la stessa base con due marcatori scritti e diversi, senza sommarla ai cambi di base; quale dei due conteggi faccia da
// titolo resta una scelta dell'utente (D-R114, aperta in domande-a1c.md; non è la D1 della revisione di P8).
//
// Revisione e RevisioneDa (R113 B ratificata; il gemello di E2 §2.6, subito dopo CodiceLettoMarcatore: 19 campi):
// Revisione è la revisione vecchia interpretata e RevisioneDa da dove viene: «codice» se è quella letta nel codice con la
// grammatica (la stessa di prima di R113), «colonna» se il codice si legge senza nessuna revisione e la colonna Rev si
// legge con la regola in campo separato della famiglia del codice (motorea.LeggiRevisioneRegistrata), "" se non ce n'è
// una interpretata. Revisione è vuota se e solo se RevisioneDa è vuoto. L'originale resta in Codice e in Rev. Quando le
// due revisioni lette discordano, Revisione resta quella del codice e il confronto non si fa
// (revisione_vecchia_discorde): RevisioneDa dice la provenienza del valore, mai una scelta fra codice e colonna
// (T-B6-201).
type VecchioPiatto struct {
	Stato                string     `json:"stato"`
	Fonte                string     `json:"fonte"`
	Codice               string     `json:"codice"`
	Rev                  string     `json:"rev"`
	Base                 string     `json:"base"`
	Marcatore            string     `json:"marcatore"`
	Revisione            string     `json:"revisione"`
	Leggibile            bool       `json:"leggibile"`
	MotivoLettura        string     `json:"motivo_lettura"`
	CodiceLetto          string     `json:"codice_letto"`
	CodiceLettoBase      string     `json:"codice_letto_base"`
	CodiceLettoMarcatore string     `json:"codice_letto_marcatore"`
	RevisioneDa          string     `json:"revisione_da"`
	Componente           *uuid.UUID `json:"componente,omitempty"`
	Documento            *uuid.UUID `json:"documento,omitempty"`
	ComponenteProposta   *uuid.UUID `json:"componente_proposta,omitempty"`
	SostituitoDa         *uuid.UUID `json:"sostituito_da,omitempty"`
	DecisoIl             *time.Time `json:"deciso_il,omitempty"`
	Destinazione         []string   `json:"destinazione,omitempty"`
}

// NuovoPiatto: il nuovo di un allegato (confronto.Nuovo): le letture d'identità del file (R25 a) e l'ancoraggio del
// motore A.
//   - Valutato, Motivo: falso con il motivo per un contenitore, una natura che non è un file, un documento che
//     l'adattatore non legge, un thread non valutato (MotivoFile*). Allora Associazione è non_valutata (A1c-L1-14), e
//     niente del nuovo si mostra, salvo la disponibilità del file, se l'adattatore l'ha letto.
//   - Basi: le basi normalizzate delle letture d'identità del file, in ordine di byte, senza doppioni.
//   - Candidati: i candidati di ancoraggio, nell'ordine del sostegno.
//   - Disponibilita: senza_testo per una scansione (T-B0-31, T-B6-02).
//   - Revisione: la revisione letta dalle letture d'identità del file, quando tutte quelle che ne leggono una dicono la
//     stessa; Revisioni e MotivoRevisioni: il confronto con la revisione del codice vecchio, fatto qui con la grammatica,
//     perché confronto non ha il motore (6.4.6 passo 10; Revisioni*, MotivoRevisioni*).
type NuovoPiatto struct {
	Valutato        bool              `json:"valutato"`
	Motivo          string            `json:"motivo"`
	Basi            []string          `json:"basi,omitempty"`
	Candidati       []CandidatoPiatto `json:"candidati,omitempty"`
	Collocazione    string            `json:"collocazione"`
	Associazione    string            `json:"associazione"`
	Disponibilita   string            `json:"disponibilita"`
	Revisione       string            `json:"revisione"`
	Revisioni       string            `json:"revisioni"`
	MotivoRevisioni string            `json:"motivo_revisioni"`
}

// CandidatoPiatto: un candidato di ancoraggio, piatto (confronto.Candidato; F0-09, il quinto tipo).
//   - Target, Livello, Autorita: quelli del candidato (la chiave è (Livello, Target): T-B4-28).
//   - Base: la base del target per il livello prodotto; per il livello componente quella del codice del componente
//     deciso, letto con la grammatica (per «componente:<uuid>» è la base del codice del componente, non quella letta nel
//     file: D1 della revisione di P8, chiusa), oppure quella della forma letta del nodo (o dei nodi della stessa
//     identità su cui il candidato sta); "" se non si legge: la base non si inventa.
//   - Radici: le basi dei target da cui il candidato si raggiunge, in ordine di byte, senza doppioni. Una o più radici la
//     cui base non si legge restano un solo "" (R-62 della revisione di V1): una radice in più non sparisce, e confronto
//     conta "" come una radice che non coincide con nessuna base attesa.
type CandidatoPiatto struct {
	Target   string   `json:"target"`
	Livello  string   `json:"livello"`
	Base     string   `json:"base"`
	Autorita string   `json:"autorita"`
	Radici   []string `json:"radici,omitempty"`
}

// ProdottoConfrontabile: un candidato prodotto della mail, piatto (confronto.ProdottoNuovo; da EsitoProdotti.Candidati).
//   - Base: la base normalizzata. Fase: il qualificatore fase attribuito (più valori diversi, in ordine e uniti da «|»).
//   - QuantitaDaCella: la quantità viene da una cella sotto l'intestazione dichiarata (R28: EvidenzaQuantita non vuota).
type ProdottoConfrontabile struct {
	CodiceRichiesto string `json:"codice_richiesto"`
	Base            string `json:"base"`
	Fase            string `json:"fase"`
	Quantita        *int   `json:"quantita,omitempty"`
	QuantitaDaCella bool   `json:"quantita_da_cella"`
}

// I valori di NuovoPiatto.Revisioni (6.4.6 passo 10): il confronto della revisione vecchia (letta nel codice, oppure
// nella colonna: VecchioPiatto.RevisioneDa) con quella del file, con motorea.ConfrontaRevisioni (solo le equivalenze
// dichiarate, nessun ordinamento).
const (
	RevisioniUguali           = "uguali"
	RevisioniDiverse          = "diverse"
	RevisioniNonConfrontabili = "non_confrontabili"
)

// I valori di VecchioPiatto.RevisioneDa (R113 B ratificata; E2 §2.6): la provenienza della revisione vecchia
// interpretata; "" vuol dire che non ce n'è una.
const (
	RevisioneDaCodice  = "codice"
	RevisioneDaColonna = "colonna"
)

// I motivi di NuovoPiatto.MotivoRevisioni [T]: "" per uguali e diverse, equivalente per una coppia che la regola della
// revisione dichiara equivalente; per non_confrontabili il primo che vale, nell'ordine (il file, poi il lato vecchio,
// poi il lato nuovo):
//   - nuovo_non_valutato: il file non è valutato;
//   - revisione_vecchia_non_letta: il codice vecchio non si legge (senza la sua famiglia la colonna non si legge: R113,
//     non determinabile), oppure ne legge una non «letta» (un token sospeso), oppure si legge senza nessuna revisione e
//     la colonna rev è vuota;
//   - revisione_vecchia_discorde: il codice legge una revisione e la colonna, letta con la regola della famiglia, un'altra
//     che non concorda (motorea.ConfrontaRevisioni discordante): due revisioni vecchie, e nessuna si sceglie (R113 B; E2
//     §2.6). Una colonna che non si legge accanto alla revisione del codice non la contraddice: vale quella del codice;
//   - il codice si legge senza nessuna revisione e la colonna non è vuota ma non si legge, con un motivo per ogni stato
//     di motorea.LeggiRevisioneRegistrata, così chi conta la colonna la divide per stato (T-B6-202):
//     revisione_solo_in_colonna per nessuna_regola (la famiglia non ha una regola in campo separato: la revisione
//     vecchia c'è solo nella colonna, che la grammatica non legge, e non si confronta per stringa; R-65 della revisione
//     di V1); revisione_colonna_non_interpretabile per non_interpretabile (una regola riservata, una colonna che la
//     regola non legge per intero, un token sospeso); revisione_colonna_ambigua per ambigua (più regole, nessuna si
//     sceglie);
//   - revisione_nuova_non_letta: nessuna lettura d'identità del file legge una revisione;
//   - revisioni_nuove_discordi: le letture d'identità del file leggono revisioni diverse.
//
// Con la colonna letta (RevisioneDa «colonna») il confronto si fa con la revisione della colonna, come con quella del
// codice: solo le equivalenze dichiarate dalle due regole, nessuna rappresentazione diversa resa uguale.
const (
	MotivoRevisioniEquivalente              = "equivalente"
	MotivoRevisioniNuovoNonValutato         = "nuovo_non_valutato"
	MotivoRevisioniSoloInColonna            = "revisione_solo_in_colonna"
	MotivoRevisioniColonnaNonInterpretabile = "revisione_colonna_non_interpretabile"
	MotivoRevisioniColonnaAmbigua           = "revisione_colonna_ambigua"
	MotivoRevisioniVecchiaDiscorde          = "revisione_vecchia_discorde"
	MotivoRevisioniVecchiaNonLetta          = "revisione_vecchia_non_letta"
	MotivoRevisioniNuovaNonLetta            = "revisione_nuova_non_letta"
	MotivoRevisioniNuoveDiscordi            = "revisioni_nuove_discordi"
	// motivoRevisioniNonDeterminabile: ConfrontaRevisioni non decide anche con due revisioni lette; oggi non succede, ma
	// la regola è sua, e qui non si sceglie al suo posto.
	motivoRevisioniNonDeterminabile = "non_determinabile"
)

// separatoreFasi: il separatore di più fasi in ProdottoConfrontabile.Fase.
const separatoreFasi = "|"

// piatti: ciò che i record piatti di un thread leggono, calcolato una volta. Le mappe servono solo a cercare.
type piatti struct {
	m           *motorea.Motore
	valutato    bool
	vecchio     *vecchioDelThread
	contenitori map[uuid.UUID]bool
	file        map[uuid.UUID]ancoraggio.FileInterpretato
	ancoraggi   map[uuid.UUID]ancoraggio.AncoraggioFile
	basiTarget  map[string]string    // Rif del target → base normalizzata
	basiNodo    map[string]string    // Rif del nodo → base normalizzata della sua forma letta
	componenti  map[uuid.UUID]string // componente → codice
}

func nuoviPiatti(t fotorfq.Thread, m *motorea.Motore, valutato bool, col *collegamento, v ValutazioneProdotti) *piatti {
	p := &piatti{m: m, valutato: valutato, vecchio: nuovoVecchioDelThread(t, m), contenitori: contenitoriDi(t.Allegati),
		file: map[uuid.UUID]ancoraggio.FileInterpretato{}, ancoraggi: map[uuid.UUID]ancoraggio.AncoraggioFile{},
		basiTarget: map[string]string{}, basiNodo: map[string]string{}, componenti: map[uuid.UUID]string{}}
	if col != nil {
		for _, f := range col.file {
			p.file[f.AllegatoID] = f
		}
	}
	for _, a := range v.Ancoraggi.File {
		p.ancoraggi[a.AllegatoID] = a
	}
	for _, pv := range v.ProdottiValutati {
		p.basiTarget[pv.Rif] = pv.Base.Normalizzata
	}
	for _, s := range v.Ancoraggi.Strutture {
		for _, n := range s.Nodi {
			if n.Codice.Forma != nil && n.Codice.Forma.Base.Normalizzata != "" {
				p.basiNodo[n.Rif] = n.Codice.Forma.Base.Normalizzata
			}
		}
	}
	for _, c := range t.Componenti {
		p.componenti[c.ID] = c.Codice
	}
	return p
}

// confrontabili: un record per allegato del thread, in ordine di AllegatoID (CP.2, regole di popolamento): anche i
// contenitori, le nature che non sono file e i file di un thread non valutato, che il runner mette a parte con
// EsitoThread.Valutato. Con la grammatica, il codice vecchio che non si legge dà confronto.codice_registrato_non_leggibile.
func (p *piatti) confrontabili(t fotorfq.Thread) ([]FileConfrontabile, []evidenze.Diagnostica) {
	allegati := append([]fotorfq.Allegato(nil), t.Allegati...)
	sort.SliceStable(allegati, func(i, j int) bool { return allegati[i].ID.String() < allegati[j].ID.String() })
	var out []FileConfrontabile
	var diag []evidenze.Diagnostica
	for _, a := range allegati {
		vl := p.vecchio.delFile(a.ID)
		if p.m != nil && vl.Lettura.Motivo != "" && vl.Lettura.Motivo != MotivoLetturaCodiceVuoto {
			diag = append(diag, diagnosticaCodiceNonLeggibile(a.ID, vl.Lettura))
		}
		out = append(out, FileConfrontabile{AllegatoID: a.ID, Vecchio: vl.piatto(), Nuovo: p.nuovo(a, vl)})
	}
	return out, diag
}

// piatto: il vecchio come record piatto. Revisione e RevisioneDa vengono dalla revisione vecchia interpretata (R113 B):
// per un codice che legge la sua revisione sono quella del codice, come prima.
func (v vecchioLetto) piatto() VecchioPiatto {
	return VecchioPiatto{Stato: v.Stato, Fonte: v.Fonte, Codice: v.Codice, Rev: v.Rev, Base: v.Lettura.Base, Marcatore: v.Lettura.Marcatore,
		Revisione: v.revisione.valore, Leggibile: v.Lettura.Leggibile, MotivoLettura: v.Lettura.Motivo, CodiceLetto: v.CodiceLetto,
		CodiceLettoBase: v.CodiceLettoBase, CodiceLettoMarcatore: v.CodiceLettoMarcatore, RevisioneDa: v.revisione.da, Componente: copiaUUID(v.Componente), Documento: copiaUUID(v.Documento), ComponenteProposta: copiaUUID(v.ComponenteProposta),
		SostituitoDa: copiaUUID(v.SostituitoDa), DecisoIl: copiaTempo(v.DecisoIl), Destinazione: append([]string(nil), v.Destinazione...)}
}

// nuovo: il nuovo di un allegato. Il motivo di un file non valutato è il primo che vale: contenitore, natura che non è un
// file (proprietà del file, con o senza grammatica), thread non valutato, documento che l'adattatore non legge.
func (p *piatti) nuovo(a fotorfq.Allegato, vl vecchioLetto) NuovoPiatto {
	n := NuovoPiatto{Associazione: string(ancoraggio.AssociazioneNonValutata)}
	f, letto := p.file[a.ID]
	if letto {
		n.Disponibilita = string(f.Disponibilita)
	}
	af, ancorato := p.ancoraggi[a.ID]
	switch {
	case p.contenitori[a.ID]:
		n.Motivo = MotivoFileContenitore
	case a.Natura != "" && a.Natura != naturaFile:
		n.Motivo = MotivoFileNaturaNonFile
	case !p.valutato:
		n.Motivo = MotivoFileThreadNonValutato
	case !letto || !ancorato:
		n.Motivo = MotivoFileDocumentoNonLeggibile
	default:
		n.Valutato = true
		n.Disponibilita = string(af.Disponibilita)
		n.Collocazione, n.Associazione = string(af.Collocazione), string(af.Associazione)
		letture := lettureDiIdentita(f, af)
		n.Basi = basiDi(letture)
		for _, c := range af.Candidati {
			n.Candidati = append(n.Candidati, CandidatoPiatto{Target: c.Target, Livello: c.Livello, Base: p.baseDelCandidato(c),
				Autorita: string(c.Autorita), Radici: p.basiDelleRadici(c.RadiciRaggiungibili)})
		}
		rev, motivo := revisioneDelFile(letture)
		if rev != nil {
			n.Revisione = rev.Normalizzata
		}
		n.Revisioni, n.MotivoRevisioni = confrontaLeRevisioni(vl.revisione, rev, motivo)
		return n
	}
	n.Revisioni, n.MotivoRevisioni = RevisioniNonConfrontabili, MotivoRevisioniNuovoNonValutato
	return n
}

// lettureDiIdentita: le letture d'identità del file (AncoraggioFile.Letture), nel loro ordine, cercate
// nell'interpretazione del file.
func lettureDiIdentita(f ancoraggio.FileInterpretato, af ancoraggio.AncoraggioFile) []motorea.LetturaCodice {
	perID := make(map[string]motorea.LetturaCodice, len(f.Interpretazione.Letture))
	for _, l := range f.Interpretazione.Letture {
		perID[l.ID] = l
	}
	var out []motorea.LetturaCodice
	for _, id := range af.Letture {
		if l, ok := perID[id]; ok {
			out = append(out, l)
		}
	}
	return out
}

// basiDi: le basi normalizzate delle letture, in ordine di byte, senza doppioni e senza le basi vuote.
func basiDi(letture []motorea.LetturaCodice) []string {
	var out []string
	for _, l := range letture {
		if b := l.Forma.Base.Normalizzata; b != "" {
			out = append(out, b)
		}
	}
	return ordinatiUnici(out)
}

// baseDelCandidato: la base di un candidato (CandidatoPiatto.Base).
func (p *piatti) baseDelCandidato(c ancoraggio.CandidatoAncoraggio) string {
	if c.Livello == ancoraggio.LivelloProdotto {
		return p.basiTarget[c.Target]
	}
	if strings.HasPrefix(c.Target, rifComponente) {
		id, err := uuid.Parse(strings.TrimPrefix(c.Target, rifComponente))
		if err != nil {
			return ""
		}
		if l, ok := leggiCodiceRegistrato(p.m, p.componenti[id]); ok {
			return l.Base.Normalizzata
		}
		return ""
	}
	if b, ok := p.basiNodo[c.Target]; ok {
		return b
	}
	for _, pos := range c.Posizioni { // un'identità: i nodi su cui il candidato sta hanno tutti quell'identità
		if b, ok := p.basiNodo[pos.Nodo]; ok {
			return b
		}
	}
	return ""
}

// basiDelleRadici: le basi dei target da cui un candidato si raggiunge (CandidatoPiatto.Radici), in ordine di byte, senza
// doppioni: una o più radici con la base che non si legge restano un solo "" (R-62).
func (p *piatti) basiDelleRadici(radici []string) []string {
	var out []string
	for _, r := range radici {
		out = append(out, p.basiTarget[r])
	}
	return ordinatiUnici(out)
}

// revisioneDelFile: la revisione letta dalle letture d'identità del file: quella comune a tutte le letture che ne leggono
// una (lo stato «letta»); nil con il motivo se nessuna la legge o se ne leggono di diverse.
func revisioneDelFile(letture []motorea.LetturaCodice) (*motorea.RevisioneLetta, string) {
	var rev *motorea.RevisioneLetta
	for i := range letture {
		r := letture[i].Forma.Revisione
		if r == nil || r.Stato != motorea.StatoRevisioneLetta {
			continue
		}
		switch {
		case rev == nil:
			rev = r
		case rev.Normalizzata != r.Normalizzata:
			return nil, MotivoRevisioniNuoveDiscordi
		}
	}
	if rev == nil {
		return nil, MotivoRevisioniNuovaNonLetta
	}
	return rev, ""
}

// confrontaLeRevisioni: la revisione vecchia interpretata (dal codice o dalla colonna: revisioneVecchia) contro quella
// del file, con la grammatica (ConfrontaRevisioni). Il lato vecchio viene prima del nuovo: un motivo del vecchio vale
// anche se il file non legge nessuna revisione.
func confrontaLeRevisioni(vecchia revisioneVecchia, nuova *motorea.RevisioneLetta, motivoNuova string) (string, string) {
	switch {
	case vecchia.motivo != "":
		return RevisioniNonConfrontabili, vecchia.motivo
	case vecchia.rev == nil: // non succede: senza motivo la revisione c'è; se succede non si sceglie
		return RevisioniNonConfrontabili, MotivoRevisioniVecchiaNonLetta
	case nuova == nil:
		return RevisioniNonConfrontabili, motivoNuova
	}
	switch motorea.ConfrontaRevisioni(*vecchia.rev, *nuova) {
	case motorea.CompatibilitaUguale:
		return RevisioniUguali, ""
	case motorea.CompatibilitaEquivalente:
		return RevisioniUguali, MotivoRevisioniEquivalente
	case motorea.CompatibilitaDiscordante:
		return RevisioniDiverse, ""
	}
	return RevisioniNonConfrontabili, motivoRevisioniNonDeterminabile
}

// prodottiConfrontabili: i candidati prodotto della mail, piatti, nell'ordine dei candidati (ProdottoConfrontabile).
func prodottiConfrontabili(p ancoraggio.EsitoProdotti) []ProdottoConfrontabile {
	var out []ProdottoConfrontabile
	for _, c := range p.Candidati {
		var q *int
		if c.Quantita != nil {
			v := *c.Quantita
			q = &v
		}
		out = append(out, ProdottoConfrontabile{CodiceRichiesto: c.CodiceRichiesto, Base: c.Base.Normalizzata, Fase: faseDi(c.Qualificatori),
			Quantita: q, QuantitaDaCella: len(c.EvidenzaQuantita) > 0})
	}
	return out
}

// faseDi: il qualificatore fase attribuito di un candidato; più valori diversi in ordine, uniti da «|».
func faseDi(q []motorea.AffissoLetto) string {
	var fasi []string
	for _, a := range q {
		if a.Attribuito && a.Valore != nil && a.Valore.Fase != "" {
			fasi = append(fasi, a.Valore.Fase)
		}
	}
	return strings.Join(ordinatiUnici(fasi), separatoreFasi)
}

// contenitoriDi: gli allegati da cui vengono altre voci (ContenitoreID di un altro allegato).
func contenitoriDi(allegati []fotorfq.Allegato) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, a := range allegati {
		if a.ContenitoreID != nil {
			out[*a.ContenitoreID] = true
		}
	}
	return out
}
