package propostabom

import (
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// Le letture del registro e le due proiezioni dell'identità proposta (V1: emendamento di V1 §3.8, «Una lettura», e
// §6.1, «Il codice di partenza»; domanda V1-Q09, base di lavoro A). Una lettura si scrive sempre con la stessa funzione,
// senza campi che dipendono da dove sta (la voce, un'alternativa, un riferimento, le letture di un file): la stessa
// chiave_lettura porta così gli stessi byte in ogni punto del registro, come il DTO vuole (importazione, validaLetture).
// Niente NFC: intervalli in byte, letterale = testo_unita[inizio:fine].

// I motivi di un codice proposto vuoto (codice_proposto, motivo_proposto; emendamento di V1 §3.4.2, «da_decidere»):
//   - nessuna_lettura, letture_discordanti: il motivo della catena del codice (ancoraggio.MotivoLettura*), senza
//     un'identità proposta;
//   - base_non_completa: la lettura è una forma parziale (vincoli, non un articolo: A-C07);
//   - tratto_non_determinabile: la base non si ritrova una volta sola nel letterale, la lettura ha una ripetizione
//     discordante, oppure dentro il tratto dalla base al marcatore cade una parte che il codice non porta (revisione,
//     etichetta, token, decorazione);
//   - codice_non_ammissibile: il tratto non rispetta la regola del codice della working (importazione.CodiceAmmissibile).
const (
	MotivoPropostoBaseNonCompleta        = "base_non_completa"
	MotivoPropostoTrattoNonDeterminabile = "tratto_non_determinabile"
	MotivoPropostoCodiceNonAmmissibile   = "codice_non_ammissibile"
)

// I motivi di una rev proposta vuota con una revisione letta (rev_proposta, motivo_rev; emendamento di V1 §3.8):
// revisione_non_letta (la revisione c'è, ma non è «letta»: un token sospeso, D5) e revisione_non_ammissibile (le
// maiuscole della normalizzata non rispettano importazione.RevAmmissibile: spazi, oltre 10 byte, controlli). Senza
// revisione nella lettura non c'è niente da spiegare, e il motivo non c'è.
const (
	MotivoRevNonLetta        = "revisione_non_letta"
	MotivoRevNonAmmissibile  = "revisione_non_ammissibile"
	statoRevisioneLettaMotor = motorea.StatoRevisioneLetta
)

// lettore scrive le letture di un'occorrenza (un allegato e il suo documento).
type lettore struct {
	allegato    uuid.UUID
	unita       map[string]evidenze.UnitaEvidenza // per ID, del documento dell'allegato
	cliente     string                            // «cliente:<uuid>»: l'emittente di una lettura senza emittente
	hashCliente string                            // l'hash della grammatica del cliente (Interpretazione.HashSnapshot)
}

func nuovoLettore(allegato uuid.UUID, d evidenze.DocumentoEvidenze, cliente uuid.UUID, hashCliente string) *lettore {
	x := &lettore{allegato: allegato, unita: make(map[string]evidenze.UnitaEvidenza, len(d.Unita)), cliente: "cliente:" + cliente.String(),
		hashCliente: hashCliente}
	for _, u := range d.Unita {
		if _, ok := x.unita[u.ID]; !ok {
			x.unita[u.ID] = u
		}
	}
	return x
}

// chiave: la chiave di una lettura nel registro, «<allegato_id>/<LetturaCodice.ID>».
func (x *lettore) chiave(id string) string { return x.allegato.String() + "/" + id }

// emittente: l'emittente esplicito di una forma e l'hash della sua grammatica: il vuoto è il cliente (MA/letture.go,
// «vuoto = il cliente»), un partner è «partner:<id>».
func (x *lettore) emittente(f motorea.LetturaForma) (string, string) {
	if f.Emittente == "" {
		return x.cliente, x.hashCliente
	}
	return "partner:" + f.Emittente, f.HashEmittente
}

// lettura: la lettura del registro (emendamento di V1 §3.8, «Una lettura»), come oggetto JSON. I vuoti non contano: la
// forma canonica del registro li toglie. codice_proposto e rev_proposta (DECA PE-08) sono le due proiezioni
// importazione-1 della lettura (ProiezioneCodice, ProiezioneRev: la stessa funzione dell'identità proposta di una voce):
// ci sono solo se la proiezione del codice è ammissibile, la rev solo insieme al codice («il codice proposto, e la sua
// rev se c'è»: senza un codice di partenza la lettura non ha una proiezione, e i due campi sono omessi; la voce tiene la
// sua regola, P-4). Dipendono solo dalla lettura e dal testo della sua unità, quindi la stessa chiave_lettura ha gli
// stessi byte in ogni punto del registro.
func (x *lettore) lettura(l motorea.LetturaCodice) map[string]any {
	f := l.Forma
	em, hash := x.emittente(f)
	codice, _ := ProiezioneCodice(f, x.unita[l.UnitaID].Testo)
	rev := ""
	if codice != "" {
		rev, _ = ProiezioneRev(f)
	}
	m := map[string]any{
		"chiave_lettura":   x.chiave(l.ID),
		"unita_id":         l.UnitaID,
		"altre_unita":      stringhe(l.AltreUnita),
		"selettore":        f.Selettore.String(),
		"testo_unita":      x.unita[l.UnitaID].Testo,
		"intervallo":       intervallo(l.Occorrenza),
		"uso":              l.Uso,
		"origine_uso":      l.OrigineUso,
		"ruoli_candidati":  ruoli(l.RuoliCandidati),
		"motivo_ruoli":     l.MotivoRuoli,
		"letterale":        f.Originale,
		"emittente":        em,
		"hash_grammatica":  hash,
		"namespace":        f.Namespace,
		"famiglia":         f.Famiglia,
		"forma":            f.Forma,
		"base":             baseLetta(f.Base),
		"qualificatori":    qualificatori(f),
		"codice_richiesto": f.CodiceRichiesto,
		"categorie":        categorie(f.Categorie),
		"stato_forma":      f.Stato,
		"funzione":         string(l.Funzione),
		"ruolo":            l.Ruolo,
		"qualita":          l.Qualita,
		"motivi":           stringhe(l.Motivi),
		"trasformazioni":   trasformazioni(l.Trasformazioni),
		"codice_proposto":  codice,
		"rev_proposta":     rev,
	}
	if l.Assoluto != nil {
		m["assoluto"] = map[string]any{"testo_id": l.Assoluto.TestoID, "intervallo": intervallo(l.Assoluto.Intervallo)}
	}
	if f.Revisione != nil {
		m["revisione"] = revisione(f.Revisione)
	}
	if c := f.Classe; c != nil {
		m["classe"] = map[string]any{"segmento": c.Segmento, "valore": c.Valore, "categoria": string(c.Categoria),
			"descrizione": c.Descrizione, "fonte": c.Fonte}
	}
	return m
}

func intervallo(iv evidenze.Intervallo) map[string]any {
	return map[string]any{"inizio": iv.Inizio, "fine": iv.Fine}
}

func stringhe(s []string) []any {
	out := make([]any, 0, len(s))
	for _, x := range s {
		out = append(out, x)
	}
	return out
}

func ruoli[T ~string](s []T) []any {
	out := make([]any, 0, len(s))
	for _, x := range s {
		out = append(out, string(x))
	}
	return out
}

func categorie[T ~string](s []T) []any { return ruoli(s) }

func segmenti(s []motorea.SegmentoLetto) []any {
	out := make([]any, 0, len(s))
	for _, x := range s {
		out = append(out, map[string]any{"nome": x.Nome, "originale": x.Originale, "normalizzato": x.Normalizzato,
			"intervallo": intervallo(x.Intervallo)})
	}
	return out
}

func baseLetta(b motorea.BaseLetta) map[string]any {
	return map[string]any{"originale": b.Originale, "normalizzata": b.Normalizzata, "segmenti": segmenti(b.Segmenti),
		"mancanti": stringhe(b.Mancanti), "completa": b.Completa}
}

func parte(p *motorea.ParteLetta) any {
	if p == nil {
		return nil
	}
	return map[string]any{"valore": p.Valore, "originale": p.Originale, "intervallo": intervallo(p.Intervallo)}
}

// qualificatori: marcatore, affissi (con fase e destinazione dell'attribuzione), token, etichetta, decorazioni e
// ripetizioni, nell'ordine del testo come li dà il motore.
func qualificatori(f motorea.LetturaForma) map[string]any {
	affissi := make([]any, 0, len(f.Affissi))
	for _, a := range f.Affissi {
		m := map[string]any{"regola": a.Regola, "originale": a.Originale, "posizione": a.Posizione, "intervallo": intervallo(a.Intervallo),
			"attribuito": a.Attribuito}
		if a.Valore != nil {
			m["fase"], m["destinazione"] = a.Valore.Fase, a.Valore.Destinazione
		}
		affissi = append(affissi, m)
	}
	decorazioni := make([]any, 0, len(f.Decorazioni))
	for _, d := range f.Decorazioni {
		decorazioni = append(decorazioni, map[string]any{"regola": d.Regola, "tipo": d.Tipo, "valore": d.Valore, "originale": d.Originale,
			"riserva": d.Riserva, "intervallo": intervallo(d.Intervallo)})
	}
	ripetizioni := make([]any, 0, len(f.Ripetizioni))
	for _, r := range f.Ripetizioni {
		ripetizioni = append(ripetizioni, map[string]any{"base": baseLetta(r.Base), "concorda": r.Concorda})
	}
	return map[string]any{"marcatore": parte(f.Marcatore), "affissi": affissi, "token": parte(f.Token), "etichetta": parte(f.Etichetta),
		"decorazioni": decorazioni, "ripetizioni": ripetizioni}
}

func revisione(r *motorea.RevisioneLetta) map[string]any {
	return map[string]any{"regola": r.Regola, "originale": r.Originale, "normalizzata": r.Normalizzata, "stato": r.Stato,
		"sorgente": r.Sorgente, "segmenti": segmenti(r.Segmenti), "token": parte(r.Token), "richiede_verifica": r.RichiedeVerifica,
		"intervallo": intervallo(r.Intervallo)}
}

func trasformazioni(tt []motorea.Trasformazione) []any {
	out := make([]any, 0, len(tt))
	for _, t := range tt {
		iv := make([]any, 0, len(t.Intervalli))
		for _, x := range t.Intervalli {
			iv = append(iv, intervallo(x))
		}
		out = append(out, map[string]any{"regola": t.Regola, "operazione": t.Operazione, "prima": t.Prima, "dopo": t.Dopo,
			"motivo": t.Motivo, "intervalli": iv})
	}
	return out
}

// ---- le proiezioni dell'identità proposta (importazione-1) ----

// ProiezioneCodice: il codice di partenza di una lettura per la working (emendamento di V1 §6.1; V1-Q09 A): il tratto
// contiguo del letterale dalla base al marcatore, affissi compresi, senza revisione, etichetta, decorazioni e token, così
// com'è nel testo dell'unità (byte per byte: niente maiuscole, niente NFC). Vuoto, con il motivo, se il tratto non si
// determina o non è ammissibile; mai una stringa inventata, e le varianti per marcatore restano distinte. testo è il
// testo dell'unità della lettura, su cui la forma misura i suoi intervalli. Una regola diversa vuole importazione-2.
func ProiezioneCodice(f motorea.LetturaForma, testo string) (codice, motivo string) {
	if !f.Base.Completa {
		return "", MotivoPropostoBaseNonCompleta
	}
	if f.Stato == motorea.StatoDiscordante {
		return "", MotivoPropostoTrattoNonDeterminabile
	}
	base, ok := intervalloDellaBase(f, testo)
	if !ok {
		return "", MotivoPropostoTrattoNonDeterminabile
	}
	inizio, fine := base.Inizio, base.Fine
	estendi := func(iv evidenze.Intervallo) {
		inizio, fine = min(inizio, iv.Inizio), max(fine, iv.Fine)
	}
	for _, a := range f.Affissi {
		estendi(a.Intervallo)
	}
	if f.Marcatore != nil {
		estendi(f.Marcatore.Intervallo)
	}
	if inizio < f.Intervallo.Inizio || fine > f.Intervallo.Fine || fine > len(testo) || inizio >= fine {
		return "", MotivoPropostoTrattoNonDeterminabile
	}
	for _, iv := range partiFuoriDalCodice(f) {
		if iv.Inizio < fine && inizio < iv.Fine {
			return "", MotivoPropostoTrattoNonDeterminabile
		}
	}
	c := testo[inizio:fine]
	if !importazione.CodiceAmmissibile(c) {
		return "", MotivoPropostoCodiceNonAmmissibile
	}
	return c, ""
}

// partiFuoriDalCodice: gli intervalli delle parti che il codice di partenza non porta: revisione (con il suo token),
// etichetta, token, decorazioni e i segmenti delle ripetizioni della base.
func partiFuoriDalCodice(f motorea.LetturaForma) []evidenze.Intervallo {
	var out []evidenze.Intervallo
	if r := f.Revisione; r != nil {
		out = append(out, r.Intervallo)
		if r.Token != nil {
			out = append(out, r.Token.Intervallo)
		}
	}
	for _, p := range []*motorea.ParteLetta{f.Etichetta, f.Token} {
		if p != nil {
			out = append(out, p.Intervallo)
		}
	}
	for _, d := range f.Decorazioni {
		out = append(out, d.Intervallo)
	}
	for _, r := range f.Ripetizioni {
		for _, s := range r.Base.Segmenti {
			out = append(out, s.Intervallo)
		}
	}
	return out
}

// intervalloDellaBase: dove sta la base nel letterale. La forma non porta l'intervallo della base, solo il suo testo
// (BaseLetta.Originale) e quelli dei segmenti: la base è l'unica occorrenza del suo testo dentro il letterale che
// contiene tutti i suoi segmenti e non tocca le altre parti (affissi, marcatore e le parti fuori dal codice). Con
// nessuna o più di una occorrenza così il tratto non si determina: non si sceglie.
func intervalloDellaBase(f motorea.LetturaForma, testo string) (evidenze.Intervallo, bool) {
	b := f.Base.Originale
	lo, hi := f.Intervallo.Inizio, f.Intervallo.Fine
	if b == "" || lo < 0 || hi > len(testo) || lo > hi {
		return evidenze.Intervallo{}, false
	}
	occupati := partiFuoriDalCodice(f)
	for _, a := range f.Affissi {
		occupati = append(occupati, a.Intervallo)
	}
	if f.Marcatore != nil {
		occupati = append(occupati, f.Marcatore.Intervallo)
	}
	var trovati []evidenze.Intervallo
	for i := lo; i+len(b) <= hi; i++ {
		if testo[i:i+len(b)] != b {
			continue
		}
		iv := evidenze.Intervallo{Inizio: i, Fine: i + len(b)}
		ok := true
		for _, s := range f.Base.Segmenti {
			ok = ok && s.Intervallo.Inizio >= iv.Inizio && s.Intervallo.Fine <= iv.Fine
		}
		for _, o := range occupati {
			ok = ok && !(o.Inizio < iv.Fine && iv.Inizio < o.Fine)
		}
		if ok {
			trovati = append(trovati, iv)
		}
	}
	if len(trovati) != 1 {
		return evidenze.Intervallo{}, false
	}
	return trovati[0], true
}

// ProiezioneRev: la rev di partenza di una lettura per la working (emendamento di V1 §3.8, rev_proposta): le maiuscole
// ASCII della revisione normalizzata se la revisione è «letta» e il risultato rispetta importazione.RevAmmissibile;
// altrimenti vuota, con il motivo (MotivoRev*). Senza revisione: vuota e senza motivo. La revisione letta resta intatta
// nella lettura del registro.
func ProiezioneRev(f motorea.LetturaForma) (rev, motivo string) {
	r := f.Revisione
	if r == nil {
		return "", ""
	}
	if r.Stato != statoRevisioneLettaMotor {
		return "", MotivoRevNonLetta
	}
	v := maiuscoleASCII(r.Normalizzata)
	if !importazione.RevAmmissibile(v) {
		return "", MotivoRevNonAmmissibile
	}
	return v, ""
}

// maiuscoleASCII: le sole lettere ASCII in maiuscolo; il resto com'è (la lunghezza in byte non cambia).
func maiuscoleASCII(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}, s)
}

// ---- la regressione della struttura (LS-01) ----

// reGrafoDaRileggere: il motivo di struttura_motivo_parziale (0020) per una struttura di una versione diversa dalla 3:
// i fatti sono di un worker non aggiornato, e la struttura va riletta (emendamento di V1 §11.2).
var reGrafoDaRileggere = regexp.MustCompile(`^struttura v[^:]*: gli scarti non sono numerati, serve la rianalisi v3$`)

// TestoGrafoDaRileggere: il testo della diagnostica importazione.grafo_da_rileggere (emendamento di V1 §11.2).
const TestoGrafoDaRileggere = `struttura letta da un worker non aggiornato: dopo l'aggiornamento serve "Rifai l'analisi"`

// ordinaLetture ordina le letture per chiave_lettura (l'ordine del motore, per ID, dentro un'occorrenza).
func ordinaLetture(l []map[string]any) {
	sort.SliceStable(l, func(i, j int) bool { return testoDi(l[i]["chiave_lettura"]) < testoDi(l[j]["chiave_lettura"]) })
}

func testoDi(v any) string {
	s, _ := v.(string)
	return s
}
