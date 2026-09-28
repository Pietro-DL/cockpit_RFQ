package fascicolo

// Autorita' e guida delle proposte di struttura (Smistamento F5, addendum A5.4.1, A5.4.5).
//
// Le righe di componente_proposta e relazione_proposta sono tutte fatti letti dagli STEP, e restano tutte:
// F5 cambia che cosa significano, non le perde. Una riga e' nell'AUTORITA' se sta su un arco che parte da
// una sorgente valida (Dichiarazioni): l'arco stesso e il suo figlio diretto. La sorgente e' il componente C
// per cui il file e' autorizzato. Tutto il resto e' GUIDA: si vede (anche nell'editor, distinta, per aiutare
// l'operatore), si corregge come evidenza, indica dove vanno i disegni, ma non entra nella working BOM, non
// si accetta, non entra nel gate, nei conteggi delle cose da decidere, nei banner.
//
// Qui le regole sono pure (CalcolaAutorita e le letture che ne derivano); LeggiAutorita legge le righe.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// NodoFile e' un nodo di un file: l'allegato che porta le righe e la chiave del nodo.
type NodoFile struct {
	Allegato uuid.UUID
	Chiave   string
}

// Autorita' e' la divisione fra autorita' e guida delle proposte di una RFQ.
type Autorita struct {
	Dichiarazioni Dichiarazioni
	sorgente      map[NodoFile]Dichiarazione // la riga di una sorgente valida → la sua dichiarazione
	figlio        map[NodoFile]uuid.UUID     // figlio diretto di una sorgente → C
	arco          map[ChiaveRelazione]uuid.UUID
}

// CalcolaAutorita divide le proposte in autorita' e guida. Pura. nodi e archi possono essere quelli di tutta
// la RFQ o di un file solo: l'autorita' di un arco dipende solo dal suo padre.
func CalcolaAutorita(d Dichiarazioni, nodi []db.ComponenteProposta, archi []db.RelazioneProposta) Autorita {
	a := Autorita{Dichiarazioni: d, sorgente: map[NodoFile]Dichiarazione{}, figlio: map[NodoFile]uuid.UUID{},
		arco: map[ChiaveRelazione]uuid.UUID{}}
	for _, n := range nodi {
		for _, x := range d.PerSha[n.Sha256] {
			if _, ok := x.Sorgenti[n.Chiave]; ok {
				a.sorgente[NodoFile{n.AllegatoID, n.Chiave}] = x
			}
		}
	}
	for _, r := range archi {
		s, ok := a.sorgente[NodoFile{r.AllegatoID, r.PadreChiave}]
		if !ok {
			continue
		}
		a.arco[ChiaveRelazione{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave}] = s.Componente.ComponenteID
		f := NodoFile{r.AllegatoID, r.FiglioChiave}
		// un raggruppamento e' C stesso, non un suo figlio; una delega invece e' un altro componente, figlio
		// diretto del padre in questo file e sorgente per i propri figli (Domanda 1 = B, A5.4.6)
		if x, sorg := a.sorgente[f]; !sorg || x.Delega() {
			a.figlio[f] = s.Componente.ComponenteID
		}
	}
	return a
}

// Sorgente dice se il nodo e' la sorgente di un file autorizzato, e per quale dichiarazione.
func (a Autorita) Sorgente(allegato uuid.UUID, chiave string) (Dichiarazione, bool) {
	d, ok := a.sorgente[NodoFile{allegato, chiave}]
	return d, ok
}

// FiglioDiretto dice se il nodo e' un figlio diretto di una sorgente valida, e di quale componente.
func (a Autorita) FiglioDiretto(allegato uuid.UUID, chiave string) (uuid.UUID, bool) {
	c, ok := a.figlio[NodoFile{allegato, chiave}]
	return c, ok
}

// ArcoAutorizzato dice se l'arco parte da una sorgente valida, e di quale componente e' la sorgente.
func (a Autorita) ArcoAutorizzato(k ChiaveRelazione) (uuid.UUID, bool) {
	c, ok := a.arco[k]
	return c, ok
}

// NodoNellAutorita dice se il nodo e' nell'autorita' di un file: sorgente o figlio diretto.
func (a Autorita) NodoNellAutorita(allegato uuid.UUID, chiave string) bool {
	_, s := a.sorgente[NodoFile{allegato, chiave}]
	_, f := a.figlio[NodoFile{allegato, chiave}]
	return s || f
}

// ComponenteDi e' il componente che un nodo E', per le decisioni: C per una sorgente valida, il componente
// deciso da una PERSONA per un nodo accettato. Mai quello con lo stesso codice, e mai quello di un aggancio
// automatico di prima dello Smistamento (duplicato senza chi l'ha deciso: E08): non sono decisioni.
func (a Autorita) ComponenteDi(p db.ComponenteProposta) (uuid.UUID, bool) {
	if d, ok := a.sorgente[NodoFile{p.AllegatoID, p.Chiave}]; ok {
		return d.Componente.ComponenteID, true
	}
	return DecisoDaUnaPersona(p)
}

// DecisoDaUnaPersona e' il componente di un nodo accettato da una persona.
func DecisoDaUnaPersona(p db.ComponenteProposta) (uuid.UUID, bool) {
	if (p.Stato == db.StatoPropostaConfermata || p.Stato == db.StatoPropostaDuplicato) && p.ComponenteID.Valid && p.DecisoDa.Valid {
		return p.ComponenteID.UUID, true
	}
	return uuid.Nil, false
}

// AgganciatoPerCodice dice se il nodo e' un aggancio automatico di prima dello Smistamento (duplicato con
// un componente, senza chi l'ha deciso: forma F-A). Non e' una decisione: nell'autorita' si conferma o si
// corregge (il gate lo conta da decidere), fuori e' guida inerte; il comando U5 lo riapre (F7).
func AgganciatoPerCodice(p db.ComponenteProposta) bool {
	return p.Stato == db.StatoPropostaDuplicato && p.ComponenteID.Valid && !p.DecisoDa.Valid
}

// LeggiAutorita legge le dichiarazioni e le proposte della RFQ e le divide. Non scrive niente.
func LeggiAutorita(ctx context.Context, q *db.Queries, thread uuid.UUID) (Autorita, []db.ListComponenteProposteThreadRow, []db.ListRelazioneProposteThreadRow, error) {
	d, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return Autorita{}, nil, nil, err
	}
	nodi, err := q.ListComponenteProposteThread(ctx, thread)
	if err != nil {
		return Autorita{}, nil, nil, err
	}
	archi, err := q.ListRelazioneProposteThread(ctx, thread)
	if err != nil {
		return Autorita{}, nil, nil, err
	}
	return CalcolaAutorita(d, soloNodi(nodi), soloArchi(archi)), nodi, archi, nil
}

// autoritaDelFile e' l'autorita' di un file solo: le dichiarazioni della RFQ e le righe del portatore.
func autoritaDelFile(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) (Autorita, []db.ComponenteProposta, []db.RelazioneProposta, error) {
	d, err := LeggiDichiarazioni(ctx, q, thread)
	if err != nil {
		return Autorita{}, nil, nil, err
	}
	nodi, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return Autorita{}, nil, nil, err
	}
	archi, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return Autorita{}, nil, nil, err
	}
	return CalcolaAutorita(d, nodi, archi), nodi, archi, nil
}

func soloNodi(righe []db.ListComponenteProposteThreadRow) []db.ComponenteProposta {
	out := make([]db.ComponenteProposta, len(righe))
	for i, r := range righe {
		out[i] = r.ComponenteProposta
	}
	return out
}

func soloArchi(righe []db.ListRelazioneProposteThreadRow) []db.RelazioneProposta {
	out := make([]db.RelazioneProposta, len(righe))
	for i, r := range righe {
		out[i] = r.RelazioneProposta
	}
	return out
}

// ------------------------------------------------------------------ le viste: solo l'autorita'

// NellAutorita sono le proposte che l'editor, la BOM visuale, l'albero e i banner mostrano: i nodi e gli
// archi nell'autorita' di un file autorizzato. Una sorgente si mostra come quello che e', il componente C
// (in memoria: la riga non si scrive, e una radice di prima ancora aperta resta aperta), con la nota
// calcolata in lettura quando il file la chiama in un altro modo. La guida resta fuori.
func (a Autorita) NellAutorita(nodi []db.ListComponenteProposteThreadRow, archi []db.ListRelazioneProposteThreadRow) (
	[]db.ListComponenteProposteThreadRow, []db.ListRelazioneProposteThreadRow) {
	var n []db.ListComponenteProposteThreadRow
	for _, r := range nodi {
		p := r.ComponenteProposta
		if d, ok := a.Sorgente(p.AllegatoID, p.Chiave); ok {
			r.ComponenteProposta = ComeSorgente(p, d)
			n = append(n, r)
			continue
		}
		if _, ok := a.FiglioDiretto(p.AllegatoID, p.Chiave); ok {
			n = append(n, r)
		}
	}
	var x []db.ListRelazioneProposteThreadRow
	for _, r := range archi {
		k := ChiaveRelazione{Allegato: r.RelazioneProposta.AllegatoID, Padre: r.RelazioneProposta.PadreChiave, Figlio: r.RelazioneProposta.FiglioChiave}
		if _, ok := a.ArcoAutorizzato(k); ok {
			x = append(x, r)
		}
	}
	return n, x
}

// ComeSorgente e' la riga di una sorgente come la si mostra: il componente C, deciso. Solo in memoria.
func ComeSorgente(p db.ComponenteProposta, d Dichiarazione) db.ComponenteProposta {
	if p.Stato == db.StatoPropostaAperta {
		p.Stato = db.StatoPropostaDuplicato
	}
	p.ComponenteID = uuid.NullUUID{UUID: d.Componente.ComponenteID, Valid: true}
	if c := strings.TrimSpace(p.Codice.String); c != "" && !strings.EqualFold(c, d.Componente.Codice) && !p.Nota.Valid {
		p.Nota = testo(fmt.Sprintf("radice dello STEP autorizzato di %s: il file la chiama %s", d.Componente.Codice, c))
	}
	return p
}

// Guida e' quello che resta fuori dall'autorita', in numeri: la riga sola che la BOM visuale ne dice
// («2 STEP analizzati non sono autorizzati per nessun componente; 7120010 ha 1 figlio che resta guida»).
type Guida struct {
	FileSenzaAutorizzazione int            // file con proposte e nessuna sorgente valida
	Nodi                    int            // nodi aperti di guida
	FigliDi                 map[string]int // codice di un figlio diretto deciso → nodi sotto di lui che restano guida
}

// Vuota dice se non c'e' guida da dire.
func (g Guida) Vuota() bool { return g.FileSenzaAutorizzazione == 0 && g.Nodi == 0 }

// Frase e' la riga della BOM visuale.
func (g Guida) Frase() string {
	var parti []string
	if g.FileSenzaAutorizzazione > 0 {
		parti = append(parti, fmt.Sprintf("%s non %s autorizzat%s per nessun componente", quanti(g.FileSenzaAutorizzazione, "STEP analizzato", "STEP analizzati"),
			map[bool]string{true: "è", false: "sono"}[g.FileSenzaAutorizzazione == 1], map[bool]string{true: "o", false: "i"}[g.FileSenzaAutorizzazione == 1]))
	}
	codici := make([]string, 0, len(g.FigliDi))
	for c := range g.FigliDi {
		codici = append(codici, c)
	}
	sort.Strings(codici)
	for _, c := range codici {
		n := g.FigliDi[c]
		parti = append(parti, fmt.Sprintf("%s ha %s che rest%s guida", c, quanti(n, "figlio", "figli"), map[bool]string{true: "a", false: "ano"}[n == 1]))
	}
	if len(parti) == 0 {
		return ""
	}
	return strings.Join(parti, "; ") + ": le loro proposte sono guida, non entrano nella BOM finché uno STEP non è autorizzato"
}

// LaGuida conta la guida: i file senza una sorgente valida e, per ogni figlio diretto gia' deciso che nel file
// del padre ha dei figli, quanti ne restano guida (A5.4.5: «avvisa nel gate»). codici: componente → codice.
func (a Autorita) LaGuida(nodi []db.ComponenteProposta, archi []db.RelazioneProposta, codici map[uuid.UUID]string) Guida {
	g := Guida{FigliDi: map[string]int{}}
	conSorgente := map[uuid.UUID]bool{}
	file := map[uuid.UUID]bool{}
	perNodo := map[NodoFile]db.ComponenteProposta{}
	for _, n := range nodi {
		perNodo[NodoFile{n.AllegatoID, n.Chiave}] = n
		file[n.AllegatoID] = true
		if _, ok := a.Sorgente(n.AllegatoID, n.Chiave); ok {
			conSorgente[n.AllegatoID] = true
		}
		if n.Stato == db.StatoPropostaAperta && !a.NodoNellAutorita(n.AllegatoID, n.Chiave) {
			g.Nodi++
		}
	}
	for f := range file {
		if !conSorgente[f] {
			g.FileSenzaAutorizzazione++
		}
	}
	for _, r := range archi {
		p := NodoFile{r.AllegatoID, r.PadreChiave}
		if _, figlio := a.figlio[p]; !figlio || r.Stato != db.StatoPropostaAperta {
			continue
		}
		if c, ok := DecisoDaUnaPersona(perNodo[p]); ok {
			// il padre e' un figlio diretto deciso: i suoi figli nel file del padre sono guida, finche' non ha
			// uno STEP autorizzato suo
			if _, autorizzato := a.Dichiarazioni.Di(c); !autorizzato {
				g.FigliDi[codici[c]]++
			}
		}
	}
	return g
}

// ------------------------------------------------------------------ il riesame (v_thread_da_riesaminare)

// RiesameNellAutorita legge v_thread_da_riesaminare e ne toglie la guida (A5.4.8, E28, X4): la vista conta
// ogni proposta strutturale aperta nata dopo l'ultima baseline, anche quelle di file non autorizzati, e
// senza migrazione non si ridefinisce. Qui le proposte si ricontano solo nell'autorita'; una riga
// «evidenze nuove» che resta senza niente sparisce.
func RiesameNellAutorita(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]db.VThreadDaRiesaminare, error) {
	righe, err := q.ListThreadDaRiesaminare(ctx, thread)
	if err != nil || len(righe) == 0 {
		return righe, err
	}
	aut, nodi, archi, err := LeggiAutorita(ctx, q, thread)
	if err != nil {
		return nil, err
	}
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return nil, err
	}
	var out []db.VThreadDaRiesaminare
	for _, r := range righe {
		if r.TipoMotivo != "evidenze_nuove" {
			out = append(out, r)
			continue
		}
		v, err := q.GetBomVersione(ctx, r.UltimaCongelataID)
		if err != nil {
			return nil, err
		}
		r.NProposte = int64(ProposteNellAutoritaDopo(aut, soloNodi(nodi), soloArchi(archi), rim, v.CongelataIl))
		if r.NFile+r.NProposte == 0 {
			continue
		}
		r.Motivo = fmt.Sprintf("evidenze tecniche nuove dopo V%d: %d file, %d proposte", r.UltimoNumero, r.NFile, r.NProposte)
		out = append(out, r)
	}
	return out, nil
}

// ProposteNellAutoritaDopo conta le proposte strutturali aperte nate dopo un istante, solo nell'autorita':
// i figli diretti, gli archi dalle sorgenti, le rimozioni (che nascono solo da un file autorizzato). Pura.
func ProposteNellAutoritaDopo(a Autorita, nodi []db.ComponenteProposta, archi []db.RelazioneProposta, rim []db.RimozioneProposta, dopo *time.Time) int {
	nuova := func(t time.Time) bool { return dopo == nil || t.After(*dopo) }
	n := 0
	for _, p := range nodi {
		if _, ok := a.FiglioDiretto(p.AllegatoID, p.Chiave); ok && p.Stato == db.StatoPropostaAperta && nuova(p.CreatoIl) {
			n++
		}
	}
	for _, r := range archi {
		if _, ok := a.ArcoAutorizzato(ChiaveRelazione{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave}); ok &&
			r.Stato == db.StatoPropostaAperta && nuova(r.CreatoIl) {
			n++
		}
	}
	for _, r := range rim {
		// solo quelle di un'autorizzazione valida: una sospesa non ha autorita' (A5.4.6)
		if r.Stato == db.StatoPropostaAperta && nuova(r.CreatoIl) && a.Dichiarazioni.RimozioneValida(r.StepDocumentoID, r.PadreID) {
			n++
		}
	}
	return n
}
