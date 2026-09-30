package fascicolo

// Il tipo di un componente lo decide una persona (Smistamento, fase T; decisioni del 27/09 ter, «Tipo
// componente / commerciale», Domanda 5 = B; precisazione dell'utente del 27/09 sera).
//
// Lo STEP descrive la struttura del CAD, non la decisione make-or-buy di Promatec. Una lettura, una
// preparazione, un'accettazione possono suggerire sottoassieme (un nodo con figli) o sciolto (una foglia),
// mai commerciale: che un pezzo si compra lo dice solo una persona, con questo gesto, dalla scheda del
// componente o dall'editor della Struttura BOM. Il gesto ha le due meta' dell'autorizzazione: l'anteprima
// (EffettoCambioTipo, in GET, non scrive, con la firma di quello che si vede) e la scrittura
// (CambiaTipoComponente), sotto il lucchetto della RFQ, con la BOM congelata ferma (D26).
//
// Le regole:
//   - finito solo dove ha senso con i vincoli di oggi: il codice e' un codice della richiesta confermato da una
//     persona (un prodotto nasce da «Aggiungi un codice della richiesta»), il componente non sta sotto un padre
//     (un prodotto e' una radice della BOM), e se ha un file autorizzato suo, valido, quel file diventa anche
//     il suo STEP strutturale nella stessa transazione (il CHECK di step_strutturale_id, BOM04 e
//     v_step_prodotto lo vogliono cosi'); una delega, o un'autorizzazione che non vale, lo spengono;
//   - da finito ad altro: con uno STEP strutturale, che il CHECK vuole solo per un finito, si revoca prima;
//     tranne verso commerciale, quando lo STEP strutturale e' anche un'autorizzazione nella forma dello
//     Smistamento (la marcatura, dove la sospensione si registra): allora il passaggio si fa come per ogni
//     altro componente autorizzato (precisazione dell'utente), e il riferimento si svuota nella stessa
//     transazione, come alla revoca. Il ritorno e' esplicito: commerciale → assieme → «Riattiva» → prodotto
//     finito, che lo rimette. Nella forma di prima (la sola colonna) non c'e' dove registrare la
//     sospensione: resta spento, con il motivo;
//   - sciolto: non con dei figli nella working («ha 2 figli: è un assieme»);
//   - commerciale: il nodo resta nella BOM Promatec; i suoi discendenti negli STEP restano guida (la sua
//     autorizzazione non vale: ValutaDichiarazioni) e non entrano nella working; un commerciale e' una foglia (PR
//     #7, domanda 6a), quindi non con dei figli nella working, come il particolare (giro 4, fase 4.4a.1b: tolto
//     FigliRestano, «i figli già nella BOM restano», che la regola rendeva irraggiungibile). Se ha un file autorizzato (sorgente o
//     delega) il passaggio non si rifiuta: si fa, e l'autorita' si SOSPENDE. La sospensione si registra nella
//     marcatura (chi, quando, «è diventato commerciale»: la forma che ValutaDichiarazioni legge); le rimozioni
//     aperte della dichiarazione si chiudono con la nota (dopoLaDecisione); le deleghe che ne dipendono restano
//     senza la loro catena e si sospendono con lei. L'anteprima annuncia tutto;
//   - uscire da commerciale non riattiva niente: l'autorizzazione resta sospesa (se non era ancora registrata,
//     la sospensione si registra adesso) finche' una persona non sceglie «Riattiva» (RiattivaStrutturale, qui
//     sotto, con la sua anteprima e la sua firma) o «Revoca». Un commerciale non si riattiva.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// TipiComponente sono i tipi che una persona sceglie per un componente, nell'ordine della tendina.
var TipiComponente = []db.TipoComponente{db.TipoComponenteFinito, db.TipoComponenteSottoassieme, db.TipoComponenteSciolto,
	db.TipoComponenteCommerciale}

// I motivi di una sospensione registrata dal cambio di tipo.
const (
	MotivoDiventatoCommerciale = "è diventato commerciale"
	motivoEraCommerciale       = "era commerciale: il suo STEP era guida"
)

// FattiTipo sono quello che le regole del cambio di tipo guardano, letti una volta (leggiFattiTipo).
type FattiTipo struct {
	Componente db.Componente
	// Bloccata: la versione congelata che blocca la working (D26); 0 = libera.
	Bloccata int32
	// Figli: i codici dei figli del componente nella working; Padri: quanti padri ha.
	Figli []string
	Padri int
	// DellaRichiesta: il codice e' un codice della richiesta confermato da una persona.
	DellaRichiesta bool
	// Dichiarazioni: le autorizzazioni del componente, valide o no (sorgenti e deleghe).
	Dichiarazioni []Dichiarazione
	// StepStrutturale: il nome del file dello STEP strutturale del componente, se c'e'.
	StepStrutturale string
}

// OpzioneTipo e' una voce della tendina: il tipo, con il motivo per cui e' spenta.
type OpzioneTipo struct {
	Tipo    db.TipoComponente
	Nome    string
	Spenta  string
	Attuale bool
}

// MotivoTipoSpento dice perche' il componente non puo' diventare t; "" = puo'. Pura: sono le regole di
// A5.4.7/U4 e delle decisioni «ter» sul tipo, una per caso.
func MotivoTipoSpento(f FattiTipo, t db.TipoComponente) string {
	c := f.Componente
	switch {
	case !t.Valid():
		return "tipo di componente non valido: " + string(t)
	case f.Bloccata > 0:
		return fmt.Sprintf("la BOM è congelata nella V%d: il tipo si cambia aprendo una revisione", f.Bloccata)
	case c.ArchiviatoIl != nil:
		return c.Codice + " è archiviato: prima lo si ripristina"
	case t == c.Tipo:
		return c.Codice + " è già " + NomeTipoFrase(t)
	case c.Tipo == db.TipoComponenteFinito && f.StepStrutturale != "" && (t != db.TipoComponenteCommerciale || !f.StepMarcato()):
		// il CHECK di step_strutturale_id lo vuole solo per un finito (0020:214). Verso commerciale, con la
		// marcatura, il cambio svuota il riferimento e sospende l'autorizzazione (la precisazione dell'utente:
		// il passaggio a commerciale di un componente autorizzato non si rifiuta)
		s := fmt.Sprintf("%s ha uno STEP strutturale (%s): resta un prodotto finito finché quel riferimento c'è; si revoca prima l'autorizzazione dello STEP",
			c.Codice, f.StepStrutturale)
		switch {
		case f.StepMarcato():
			s += ", oppure diventa commerciale (l'autorizzazione si sospende e il riferimento si svuota)"
		case t == db.TipoComponenteCommerciale:
			s += ": è nella forma di prima, senza la marcatura dove si registra la sospensione"
		}
		return s
	}
	switch t {
	case db.TipoComponenteFinito:
		if !f.DellaRichiesta {
			return c.Codice + " non è un codice della richiesta: un prodotto finito nasce da «Aggiungi un codice della richiesta»"
		}
		if f.Padri > 0 {
			return fmt.Sprintf("%s sta sotto %s nella BOM: un prodotto finito è una radice, si toglie prima dai padri", c.Codice, quanti(f.Padri, "padre", "padri"))
		}
		var proprie []Dichiarazione
		for _, d := range f.Dichiarazioni {
			if d.Delega() {
				return fmt.Sprintf("%s ha una delega nello STEP %s: un prodotto finito ha uno STEP suo; si revoca prima la delega", c.Codice, d.NomeFile)
			}
			proprie = append(proprie, d)
		}
		switch {
		case len(proprie) > 1:
			return fmt.Sprintf("%s ha %d STEP autorizzati: se ne revoca uno prima", c.Codice, len(proprie))
		case len(proprie) == 1 && !proprie[0].Valida():
			return fmt.Sprintf("l'autorizzazione di %s per %s non vale (%s): un prodotto finito la vuole valida, perché il file diventa il suo STEP strutturale; si sistema o si revoca prima",
				proprie[0].NomeFile, c.Codice, proprie[0].Problema)
		}
	case db.TipoComponenteSciolto, db.TipoComponenteCommerciale:
		// sotto un particolare e sotto un particolare commerciale non si mette niente (Contenitore)
		if n := len(f.Figli); n > 0 {
			return fmt.Sprintf("%s ha %s: è un assieme; per farlo diventare %s si spostano prima i suoi pezzi", c.Codice, quanti(n, "figlio", "figli"), NomeTipoFrase(t))
		}
	}
	return ""
}

// StepMarcato dice se lo STEP strutturale del componente e' anche una sua autorizzazione nella forma dello
// Smistamento (la marcatura, non la sola colonna): allora una sospensione ha dove registrarsi, e il passaggio
// a commerciale di un finito si fa. Pura.
func (f FattiTipo) StepMarcato() bool {
	c := f.Componente
	if !c.StepStrutturaleID.Valid {
		return false
	}
	for _, d := range f.Dichiarazioni {
		if d.Origine == OrigineSmistamento && !d.Delega() && d.Documento.Valid && d.Documento.UUID == c.StepStrutturaleID.UUID {
			return true
		}
	}
	return false
}

// Opzioni sono le voci della tendina del tipo, ognuna con il suo motivo se spenta. Pura.
func (f FattiTipo) Opzioni() []OpzioneTipo {
	out := make([]OpzioneTipo, 0, len(TipiComponente))
	for _, t := range TipiComponente {
		o := OpzioneTipo{Tipo: t, Nome: NomeTipo(t), Attuale: t == f.Componente.Tipo}
		o.Spenta = MotivoTipoSpento(f, t)
		out = append(out, o)
	}
	return out
}

// NomeTipoFrase e' il tipo dentro una frase: «un prodotto finito», «un assieme», «un particolare», «un
// particolare commerciale».
func NomeTipoFrase(t db.TipoComponente) string {
	switch t {
	case db.TipoComponenteFinito:
		return "un prodotto finito"
	case db.TipoComponenteSottoassieme:
		return "un assieme"
	case db.TipoComponenteSciolto:
		return "un particolare"
	case db.TipoComponenteCommerciale:
		return "un particolare commerciale"
	}
	return string(t)
}

// EffettoTipo e' l'anteprima del cambio di tipo. Solo lettura. Con Tipo vuoto e' solo la tendina.
type EffettoTipo struct {
	Componente db.Componente
	Tipo       db.TipoComponente
	Opzioni    []OpzioneTipo
	// Spento: perche' il cambio non si fa. Il modulo e' spento e lo dice.
	Spento string
	// Sospende: le autorizzazioni del componente che si sospendono, nominate («l'autorizzazione di 7120010.stp
	// per 7120010»); ConLei: i componenti le cui deleghe dipendono da quelle e si sospendono con loro;
	// Rimozioni: le rimozioni aperte che si chiudono.
	Sospende  []string
	ConLei    []string
	Rimozioni int
	// (giro 4, fase 4.4a.1b) FigliRestano non c'e' piu': i figli nella working di un componente che diventava
	// commerciale, che restavano. Con la regola della PR #7 (6a: il commerciale e' una foglia) il passaggio con dei
	// figli e' spento (MotivoTipoSpento), e la lista era sempre vuota.
	// RestaSospesa: uscendo da commerciale, le autorizzazioni che restano sospese finche' una persona non le
	// riattiva; Autorizzabile: uscendo da commerciale senza autorizzazioni, il suo STEP si potra' autorizzare.
	RestaSospesa  []string
	Autorizzabile bool
	// StepStrutturale: diventando finito, il file autorizzato che diventa anche il suo STEP strutturale.
	StepStrutturale string
	// SvuotaStep: un finito che diventa commerciale, il file che non e' piu' il suo STEP strutturale (il
	// riferimento si svuota; l'autorizzazione resta, sospesa).
	SvuotaStep string
	Avvisi     []string
	Firma      string

	fatti      FattiTipo
	sospendere []Dichiarazione
	motivo     string            // il motivo della sospensione che si registra
	causa      db.TipoComponente // il tipo che la causa: quello nuovo, o quello da cui si esce
	stepDoc    uuid.NullUUID
}

// Bottone e' la frase del bottone, che nomina l'effetto (P6).
func (e EffettoTipo) Bottone() string {
	s := fmt.Sprintf("%s diventa %s", e.Componente.Codice, NomeTipoFrase(e.Tipo))
	if len(e.Sospende) > 0 {
		s += ": " + quanti(len(e.Sospende), "autorizzazione si sospende", "autorizzazioni si sospendono")
	}
	return s
}

// frasi dice, con le parole dell'anteprima della scheda, che cosa il cambio fa oltre al tipo: il riepilogo della
// conferma dell'albero le mostra per ogni tipo che cambia su un componente che c'e' (giro 4, fase 4.4a.1b).
func (e EffettoTipo) frasi() []string {
	var out []string
	for _, x := range e.Sospende {
		out = append(out, "si sospende "+x+": il file resta guida e non propone più i figli diretti, finché una persona non la riattiva")
	}
	if len(e.ConLei) > 0 {
		out = append(out, "si sospendono con lei le deleghe di "+strings.Join(e.ConLei, ", "))
	}
	if e.Rimozioni > 0 {
		out = append(out, quanti(e.Rimozioni, "rimozione proposta si chiude", "rimozioni proposte si chiudono")+", con la nota della sospensione")
	}
	if e.SvuotaStep != "" {
		out = append(out, e.SvuotaStep+" non è più il suo STEP strutturale: il riferimento si svuota")
	}
	for _, x := range e.RestaSospesa {
		out = append(out, "resta sospesa "+x+": non si riattiva da sola")
	}
	if e.StepStrutturale != "" {
		out = append(out, e.StepStrutturale+" diventa anche il suo STEP strutturale")
	}
	return append(out, e.Avvisi...)
}

// leggiFattiTipo legge quello che le regole guardano. Non scrive niente.
func leggiFattiTipo(ctx context.Context, q *db.Queries, thread uuid.UUID, c db.Componente, dich Dichiarazioni) (FattiTipo, error) {
	f := FattiTipo{Componente: c, Dichiarazioni: dich.DelComponente(c.ComponenteID)}
	n, bloccata, err := WorkingBloccata(ctx, q, thread)
	if err != nil {
		return f, err
	}
	if bloccata {
		f.Bloccata = n
	}
	comp, err := codiciDeiComponenti(ctx, q, thread)
	if err != nil {
		return f, err
	}
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return f, err
	}
	for _, r := range rel {
		switch c.ComponenteID {
		case r.PadreID:
			f.Figli = append(f.Figli, comp[r.FiglioID].Codice)
		case r.FiglioID:
			f.Padri++
		}
	}
	sort.Strings(f.Figli)
	ids, err := q.ListIdentificativi(ctx, thread)
	if err != nil {
		return f, err
	}
	// un'abilitazione, non un'associazione: l'uguaglianza del codice accende soltanto l'opzione «prodotto», che poi
	// sceglie una persona (identificativo_thread non ha un componente da legare). Il confronto e' quello con cui
	// la preparazione ritrova il prodotto di un codice della richiesta (preparazione.go, GetComponentePerCodice:
	// spazi tolti, maiuscole)
	codice := strings.ToUpper(strings.TrimSpace(c.Codice))
	for _, i := range ids {
		if i.ConfermatoDa.Valid && strings.ToUpper(strings.TrimSpace(i.Codice)) == codice {
			f.DellaRichiesta = true
		}
	}
	if c.StepStrutturaleID.Valid {
		d, err := q.GetDocumento(ctx, c.StepStrutturaleID.UUID)
		switch {
		case err == nil:
			f.StepStrutturale = d.NomeFile
		case errors.Is(err, pgx.ErrNoRows):
			f.StepStrutturale = c.StepStrutturaleID.UUID.String()
		default:
			return f, err
		}
	}
	return f, nil
}

// EffettoCambioTipo calcola l'anteprima del cambio di tipo. Non scrive niente: si chiama in GET, e di nuovo
// sotto il lucchetto nella scrittura. tipo vuoto: solo la tendina (le opzioni con i loro motivi). Un errore e'
// del database o un componente che non e' della RFQ; un cambio che non si puo' fare e' EffettoTipo.Spento.
func EffettoCambioTipo(ctx context.Context, q *db.Queries, thread, comp uuid.UUID, tipo db.TipoComponente) (EffettoTipo, error) {
	return effettoCambioTipo(ctx, q, thread, comp, tipo, nil)
}

// comeDopoLaConferma e' come la conferma dell'albero trova un componente che c'e' quando ne cambia il tipo (giro 4,
// fase 4.4a.1b; ConfermaAlbero, passo 4): ripristinato se era archiviato (passo 3), con i figli che ha nell'albero
// confermato (i legami che la bozza toglie sono gia' andati via, passo 2), e con le rimozioni dei legami dell'albero
// gia' decise (passo 2). Il riepilogo la usa per dire prima di scrivere che cosa il cambio fara', con le regole e le
// parole dell'anteprima della scheda.
type comeDopoLaConferma struct {
	figli []string
	// rimozioni: step, padre, figlio
	rimozioni map[[3]uuid.UUID]bool
}

// effettoCambioTipo e' EffettoCambioTipo; con conferma, i fatti sono quelli del componente quando la conferma
// dell'albero ne cambia il tipo (comeDopoLaConferma). Non scrive niente.
func effettoCambioTipo(ctx context.Context, q *db.Queries, thread, comp uuid.UUID, tipo db.TipoComponente, conferma *comeDopoLaConferma) (EffettoTipo, error) {
	var e EffettoTipo
	c, err := q.GetComponente(ctx, comp)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ThreadID != thread) {
		return e, Rifiuto("il componente non è di questa RFQ")
	}
	if err != nil {
		return e, err
	}
	e.Componente, e.Tipo = c, tipo
	righe, err := q.ListDichiarazioniRfq(ctx, thread)
	if err != nil {
		return e, fmt.Errorf("autorizzazioni della RFQ: %w", err)
	}
	prima := ValutaDichiarazioni(righe)
	if e.fatti, err = leggiFattiTipo(ctx, q, thread, c, prima); err != nil {
		return e, err
	}
	if conferma != nil {
		e.fatti.Componente.ArchiviatoIl = nil
		e.fatti.Figli = conferma.figli
	}
	e.Opzioni = e.fatti.Opzioni()
	if tipo == "" {
		return e.firmata(), nil
	}
	if e.Spento = MotivoTipoSpento(e.fatti, tipo); e.Spento != "" {
		return e.firmata(), nil
	}

	proprie := e.fatti.Dichiarazioni
	switch {
	case tipo == db.TipoComponenteCommerciale:
		// l'autorita' si sospende: ogni autorizzazione del componente (sorgente o delega), e con lei le deleghe
		// che, nel file, stanno sotto: restano senza catena
		e.motivo, e.causa = MotivoDiventatoCommerciale, tipo
		for _, d := range proprie {
			switch {
			case d.Origine != OrigineSmistamento:
				// la forma di prima c'e' solo per un finito con lo STEP strutturale, che il cambio lo spegne
			case d.Sospensione != nil:
				e.Avvisi = append(e.Avvisi, fmt.Sprintf("%s resta sospesa (%s)", fraseDichiarazione(d), d.Sospensione.Motivo))
			default:
				e.sospendere = append(e.sospendere, d)
				e.Sospende = append(e.Sospende, fraseDichiarazione(d))
			}
		}
		dopo := ValutaDichiarazioni(ConTipo(righe, c.ComponenteID, tipo))
		e.ConLei = delegheCheCambiano(prima, dopo, c.ComponenteID, false)
		aperte, err := q.ListRimozioniAperte(ctx, thread)
		if err != nil {
			return e, err
		}
		for _, r := range aperte {
			if conferma != nil && conferma.rimozioni[[3]uuid.UUID{r.StepDocumentoID, r.PadreID, r.FiglioID}] {
				continue
			}
			if prima.RimozioneValida(r.StepDocumentoID, r.PadreID) && !dopo.RimozioneValida(r.StepDocumentoID, r.PadreID) {
				e.Rimozioni++
			}
		}
		if c.StepStrutturaleID.Valid {
			// un finito con lo STEP strutturale marcato (MotivoTipoSpento lascia passare solo quello): il CHECK
			// lo vuole solo per un finito, e il riferimento si svuota
			e.SvuotaStep = e.fatti.StepStrutturale
		}
		e.Avvisi = append(e.Avvisi, c.Codice+" resta nella BOM Promatec come pezzo comprato: i suoi discendenti negli STEP restano guida e non entrano nella working")
	case c.Tipo == db.TipoComponenteCommerciale:
		// uscire da commerciale non riattiva niente: le autorizzazioni restano sospese finche' una persona non
		// sceglie. Una non ancora registrata (sospesa solo perche' era commerciale) si registra adesso
		e.motivo, e.causa = motivoEraCommerciale, c.Tipo
		for _, d := range proprie {
			if d.Origine != OrigineSmistamento {
				continue
			}
			if d.Sospensione == nil {
				e.sospendere = append(e.sospendere, d)
			}
			e.RestaSospesa = append(e.RestaSospesa, fraseDichiarazione(d))
		}
		e.Autorizzabile = len(proprie) == 0
	}
	if tipo == db.TipoComponenteFinito {
		for _, d := range proprie {
			if !d.Delega() && d.Valida() && d.Documento.Valid {
				e.StepStrutturale, e.stepDoc = d.NomeFile, d.Documento
			}
		}
		if !e.stepDoc.Valid {
			e.Avvisi = append(e.Avvisi, "da prodotto finito, il gate chiede il suo STEP strutturale (o la deroga del fabbisogno cad_3d)")
		}
	}
	if c.Tipo == db.TipoComponenteFinito && e.fatti.DellaRichiesta {
		e.Avvisi = append(e.Avvisi, c.Codice+" resta fra i codici della richiesta, ma nella BOM non è più un prodotto finito")
	}
	return e.firmata(), nil
}

// ConTipo sono le righe delle autorizzazioni come sarebbero con il componente comp di tipo t: l'anteprima
// guarda che cosa varrebbe dopo il cambio con lo stesso predicato. Pura; le righe date non cambiano.
func ConTipo(righe []db.ListDichiarazioniRfqRow, comp uuid.UUID, t db.TipoComponente) []db.ListDichiarazioniRfqRow {
	out := make([]db.ListDichiarazioniRfqRow, len(righe))
	for i, r := range righe {
		if r.Componente.ComponenteID == comp {
			r.Componente.Tipo = t
		}
		out[i] = r
	}
	return out
}

// SenzaSospensione sono le righe con la sospensione registrata tolta dalle marcature delle proposte indicate:
// l'anteprima della riattivazione. Pura; le righe date non cambiano.
func SenzaSospensione(righe []db.ListDichiarazioniRfqRow, proposte map[uuid.UUID]bool) []db.ListDichiarazioniRfqRow {
	out := make([]db.ListDichiarazioniRfqRow, len(righe))
	for i, r := range righe {
		if proposte[r.PropostaID] && len(r.Marcatura) > 0 {
			var m map[string]json.RawMessage
			if json.Unmarshal(r.Marcatura, &m) == nil {
				delete(m, "sospesa")
				if b, err := json.Marshal(m); err == nil {
					r.Marcatura = b
				}
			}
		}
		out[i] = r
	}
	return out
}

// delegheCheCambiano sono i componenti (tranne comp) con una delega valida prima e non dopo (tornano: non valida
// prima e valida dopo), in ordine di codice.
func delegheCheCambiano(prima, dopo Dichiarazioni, comp uuid.UUID, tornano bool) []string {
	var out []string
	da, a := prima, dopo
	if tornano {
		da, a = dopo, prima
	}
	for _, x := range da.Valide() {
		if !x.Delega() || x.Componente.ComponenteID == comp {
			continue
		}
		if y, ok := a.Di(x.Componente.ComponenteID); !ok || y.Sha256 != x.Sha256 || !y.Delega() {
			out = append(out, x.Componente.Codice)
		}
	}
	sort.Strings(out)
	return out
}

// fraseDichiarazione nomina un'autorizzazione: «l'autorizzazione di 7120010.stp per 7120010», «la delega di
// 7120001A_1.stp per 7120010».
func fraseDichiarazione(d Dichiarazione) string {
	if d.Delega() {
		return fmt.Sprintf("la delega di %s per %s", d.NomeFile, d.Componente.Codice)
	}
	return fmt.Sprintf("l'autorizzazione di %s per %s", d.NomeFile, d.Componente.Codice)
}

// firmata calcola la firma dell'anteprima: il componente com'e' adesso, il tipo chiesto, e tutto quello che
// l'anteprima dice.
func (e EffettoTipo) firmata() EffettoTipo {
	x := struct {
		Componente, Da, A, Spento, Step, Svuota string
		Sospende, ConLei, Resta                 []string
		Avvisi                                  []string
		Rimozioni                               int
		Autorizzabile                           bool
		Opzioni                                 []OpzioneTipo
	}{e.Componente.ComponenteID.String(), string(e.Componente.Tipo), string(e.Tipo), e.Spento, e.StepStrutturale, e.SvuotaStep,
		e.Sospende, e.ConLei, e.RestaSospesa, e.Avvisi, e.Rimozioni, e.Autorizzabile, e.Opzioni}
	b, _ := json.Marshal(x)
	h := sha256.Sum256(b)
	e.Firma = hex.EncodeToString(h[:12])
	return e
}

// ------------------------------------------------------------------ la scrittura

// CambiaTipoComponente cambia il tipo del componente, con le regole dell'anteprima (ricalcolata qui, sotto il
// lucchetto della RFQ). Una sola transazione, quella di chi chiama: il tipo (confermato_da = chi cambia), per
// un finito lo STEP strutturale, le sospensioni registrate nelle marcature, la rilettura dei file toccati e il
// ricalcolo delle rimozioni, che chiude con la nota quelle di un'autorizzazione che non vale piu'.
func CambiaTipoComponente(ctx context.Context, q *db.Queries, thread, comp uuid.UUID, tipo db.TipoComponente, utente uuid.UUID) (string, error) {
	return cambiaTipoVisto(ctx, q, thread, comp, tipo, utente, "", false)
}

// CambiaTipoComponenteVisto e' CambiaTipoComponente dal modulo dell'anteprima: la firma di quello che la persona
// ha visto si ricontrolla sotto il lucchetto, e se e' cambiata (una decisione di un collega, un'autorizzazione
// nel frattempo) si rifiuta, come per DichiaraStrutturale.
func CambiaTipoComponenteVisto(ctx context.Context, q *db.Queries, thread, comp uuid.UUID, tipo db.TipoComponente, utente uuid.UUID, firma string) (string, error) {
	return cambiaTipoVisto(ctx, q, thread, comp, tipo, utente, firma, true)
}

func cambiaTipoVisto(ctx context.Context, q *db.Queries, thread, comp uuid.UUID, tipo db.TipoComponente, utente uuid.UUID, firma string, controlla bool) (string, error) {
	if err := prepara(ctx, q, thread, "si cambia il tipo di un componente"); err != nil {
		return "", err
	}
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	if !tipo.Valid() {
		return "", Rifiuto("tipo di componente non valido")
	}
	e, err := EffettoCambioTipo(ctx, q, thread, c.ComponenteID, tipo)
	if err != nil {
		return "", err
	}
	if e.Spento != "" {
		return "", Rifiuto(e.Spento)
	}
	if controlla && (firma == "" || firma != e.Firma) {
		return "", Rifiuto("quello che il cambio di tipo farebbe è cambiato mentre lo guardavi (un'autorizzazione, una decisione di un collega): riapri l'anteprima e guarda di nuovo")
	}
	parti, err := cambiaTipo(ctx, q, thread, e, utente)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s: tipo %s → %s.", c.Codice, NomeTipo(c.Tipo), NomeTipo(tipo))
	if len(parti) > 0 {
		msg += " " + strings.Join(parti, " ")
	}
	return dopoLaDecisione(ctx, q, thread, msg, nil)
}

// cambiaTipo scrive il cambio che l'anteprima e ha descritto, e restituisce le frasi dell'esito. Il lucchetto e
// le regole li ha gia' visti chi chiama.
func cambiaTipo(ctx context.Context, q *db.Queries, thread uuid.UUID, e EffettoTipo, utente uuid.UUID) ([]string, error) {
	c := e.Componente
	var parti []string
	if e.SvuotaStep != "" {
		// il CHECK di step_strutturale_id lo vuole solo per un finito: prima il riferimento, poi il tipo. Si
		// svuota come alla revoca (revocaDichiarazione); la marcatura resta, con la sospensione, e il ritorno a
		// prodotto finito dopo «Riattiva» lo rimette (e.stepDoc)
		if _, err := q.SetStepStrutturale(ctx, db.SetStepStrutturaleParams{ComponenteID: c.ComponenteID}); err != nil {
			return nil, err
		}
	}
	if err := q.SetTipoComponente(ctx, db.SetTipoComponenteParams{ComponenteID: c.ComponenteID, Tipo: e.Tipo, ConfermatoDa: utente}); err != nil {
		return nil, err
	}
	if e.stepDoc.Valid {
		// la materializzazione per un finito (A5.4.2): BOM04, la FK e il CHECK controllano ancora
		if _, err := q.SetStepStrutturale(ctx, db.SetStepStrutturaleParams{ComponenteID: c.ComponenteID, StepStrutturaleID: e.stepDoc}); err != nil {
			return nil, err
		}
		parti = append(parti, e.StepStrutturale+" diventa anche il suo STEP strutturale.")
	}
	toccati := map[string]bool{}
	sosp := Sospensione{Motivo: e.motivo, Tipo: string(e.causa), Da: uid(utente), Il: time.Now().UTC().Format(time.RFC3339)}
	for _, d := range e.sospendere {
		if err := registraSospensione(ctx, q, thread, d, sosp); err != nil {
			return nil, err
		}
		toccati[d.Sha256] = true
	}
	if e.Tipo == db.TipoComponenteCommerciale && len(e.Sospende) > 0 {
		// come alla revoca (E33): gli archi che un automatismo aveva chiuso nell'autorita' tornano aperti, e la
		// rilettura richiude solo quelli che hanno ancora un'autorita' valida (le altre sorgenti dello stesso file)
		for _, d := range e.fatti.Dichiarazioni {
			if d.Origine == OrigineSmistamento {
				toccati[d.Sha256] = true
			}
		}
		for sha := range toccati {
			if _, err := q.RiapriArchiAutomaticiDiUnFile(ctx, db.RiapriArchiAutomaticiDiUnFileParams{ThreadID: thread, Sha256: testo(sha)}); err != nil {
				return nil, err
			}
		}
		parti = append(parti, "Sospesa "+strings.Join(e.Sospende, "; ")+": si riattiva solo con una scelta esplicita, dopo un altro cambio di tipo.")
		if len(e.ConLei) > 0 {
			parti = append(parti, "Si sospendono con lei le deleghe di "+strings.Join(e.ConLei, ", ")+".")
		}
		if e.SvuotaStep != "" {
			parti = append(parti, e.SvuotaStep+" non è più il suo STEP strutturale: torna con «Riattiva» e il ritorno a prodotto finito.")
		}
	}
	if len(e.RestaSospesa) > 0 {
		parti = append(parti, "Resta sospesa "+strings.Join(e.RestaSospesa, "; ")+": si riattiva o si revoca dalla scheda.")
	}
	if e.Autorizzabile {
		parti = append(parti, "Il suo STEP si può autorizzare dalla scheda: niente si autorizza da solo.")
	}
	if err := rileggiIContenuti(ctx, q, thread, toccati); err != nil {
		return nil, err
	}
	return parti, nil
}

// registraSospensione scrive la sospensione nelle marcature delle sorgenti della dichiarazione.
func registraSospensione(ctx context.Context, q *db.Queries, thread uuid.UUID, d Dichiarazione, s Sospensione) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	for _, k := range d.ChiaviSorgenti() {
		p, err := q.GetComponentePropostaPerChiave(ctx, db.GetComponentePropostaPerChiaveParams{ThreadID: thread, AllegatoID: d.Allegato, Chiave: k})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := q.SospendiDichiarazione(ctx, db.SospendiDichiarazioneParams{Sospensione: b, PropostaID: p.PropostaID}); err != nil {
			return err
		}
	}
	return nil
}

// ------------------------------------------------------------------ la riattivazione

// EffettoRiattivazione e' l'anteprima della riattivazione di un'autorizzazione sospesa. Solo lettura.
type EffettoRiattivazione struct {
	Componente  db.Componente
	Sha256      string
	NomeFile    string
	Delega      bool
	Sospensione *Sospensione
	// Spento: perche' non si riattiva (un commerciale, un archiviato, la BOM congelata, niente di sospeso, o
	// un'autorizzazione che riattivata non varrebbe).
	Spento string
	// FigliDiretti: i figli diretti delle sorgenti nel file, che tornano nell'autorita'.
	FigliDiretti int
	// ConLei: i componenti le cui deleghe tornano valide con lei.
	ConLei []string
	Avvisi []string
	Firma  string

	proposte map[uuid.UUID]bool // le righe marcate da riattivare
	toccati  map[string]bool
}

// Bottone e' la frase del bottone: «Riattiva l'autorizzazione dello STEP 7120010.stp per 7120010».
func (e EffettoRiattivazione) Bottone() string {
	if e.Delega {
		return fmt.Sprintf("Riattiva la delega dello STEP %s per %s", e.NomeFile, e.Componente.Codice)
	}
	return fmt.Sprintf("Riattiva l'autorizzazione dello STEP %s per %s", e.NomeFile, e.Componente.Codice)
}

// EffettoRiattivazioneDi calcola l'anteprima della riattivazione delle autorizzazioni sospese (registrate) del
// componente: quelle del file sha, o tutte se sha e' vuoto. Non scrive niente.
func EffettoRiattivazioneDi(ctx context.Context, q *db.Queries, thread, comp uuid.UUID, sha string) (EffettoRiattivazione, error) {
	var e EffettoRiattivazione
	c, err := q.GetComponente(ctx, comp)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ThreadID != thread) {
		return e, Rifiuto("il componente non è di questa RFQ")
	}
	if err != nil {
		return e, err
	}
	e.Componente, e.Sha256 = c, sha
	righe, err := q.ListDichiarazioniRfq(ctx, thread)
	if err != nil {
		return e, fmt.Errorf("autorizzazioni della RFQ: %w", err)
	}
	prima := ValutaDichiarazioni(righe)
	var sospese []Dichiarazione
	for _, d := range prima.DelComponente(comp) {
		if d.Sospensione != nil && (sha == "" || d.Sha256 == sha) {
			sospese = append(sospese, d)
		}
	}
	if len(sospese) > 0 {
		d := sospese[0]
		e.Sha256, e.NomeFile, e.Delega, e.Sospensione = d.Sha256, d.NomeFile, d.Delega(), d.Sospensione
	}
	switch {
	case c.ArchiviatoIl != nil:
		e.Spento = c.Codice + " è archiviato: si ripristina prima"
	case c.Tipo == db.TipoComponenteCommerciale:
		e.Spento = c.Codice + " è un commerciale: il suo STEP resta guida; per riattivare l'autorizzazione cambia prima il tipo"
	case len(sospese) == 0:
		e.Spento = c.Codice + " non ha un'autorizzazione sospesa da riattivare"
	}
	if e.Spento == "" {
		if n, bloccata, err := WorkingBloccata(ctx, q, thread); err != nil {
			return e, err
		} else if bloccata {
			e.Spento = fmt.Sprintf("la BOM è congelata nella V%d: niente autorizzazioni né revoche finché non si apre una revisione", n)
		}
	}
	if e.Spento != "" {
		return e.firmata(), nil
	}
	e.proposte, e.toccati = map[uuid.UUID]bool{}, map[string]bool{}
	for _, r := range righe {
		if r.Componente.ComponenteID != comp || r.Origine != OrigineSmistamento || (sha != "" && r.Sha256 != sha) {
			continue
		}
		for _, d := range sospese {
			if d.Sha256 == r.Sha256 && d.Allegato == r.AllegatoID {
				if _, sorgente := d.Sorgenti[r.Chiave]; sorgente {
					e.proposte[r.PropostaID] = true
					e.toccati[r.Sha256] = true
				}
			}
		}
	}
	dopo := ValutaDichiarazioni(SenzaSospensione(righe, e.proposte))
	for _, d := range sospese {
		var x *Dichiarazione
		for _, y := range dopo.DelComponente(comp) {
			if y.Sha256 == d.Sha256 && y.Delega() == d.Delega() {
				y := y
				x = &y
			}
		}
		if x == nil || !x.Valida() {
			perche, consiglio := "non si legge più", "si revoca, e si autorizza di nuovo dall'anteprima"
			if x != nil {
				perche = x.Problema
				// una delega che resterebbe senza la catena perche' chi la tiene e' sospeso a sua volta: si
				// riattiva prima quello (revocarla la perderebbe senza motivo)
				if p, ok := sospesaSopra(dopo, *x); ok {
					consiglio = "si riattiva prima " + fraseDichiarazione(p)
				}
			}
			e.Spento = fmt.Sprintf("riattivata, %s non varrebbe (%s): %s", fraseDichiarazione(d), perche, consiglio)
			return e.firmata(), nil
		}
		// i figli diretti delle sorgenti nel file
		archi, err := q.ListRelazioneProposteFile(ctx, db.ListRelazioneProposteFileParams{ThreadID: thread, AllegatoID: x.Allegato})
		if err != nil {
			return e, err
		}
		visti := map[string]bool{}
		for _, a := range archi {
			if _, sorgente := x.Sorgenti[a.PadreChiave]; sorgente && !visti[a.FiglioChiave] {
				if _, anche := x.Sorgenti[a.FiglioChiave]; !anche {
					visti[a.FiglioChiave] = true
					e.FigliDiretti++
				}
			}
		}
	}
	e.ConLei = delegheCheCambiano(prima, dopo, comp, true)
	if len(sospese) > 1 {
		e.Avvisi = append(e.Avvisi, fmt.Sprintf("%s ha %d autorizzazioni sospese: si riattivano tutte", c.Codice, len(sospese)))
	}
	e.Avvisi = append(e.Avvisi, "le rimozioni chiuse dalla sospensione si ricalcolano, e il gate conta di nuovo le decisioni aperte del file")
	return e.firmata(), nil
}

// sospesaSopra cerca, per una delega x che non vale per la catena (SenzaCatena, o il file che non vale per il suo
// componente), l'autorizzazione dello stesso file che la tiene ed e' ferma con una sospensione registrata. Sale
// nel file dai nodi sorgente di x, livello per livello, fino alla prima sorgente di un'altra dichiarazione dello
// stesso file: sospesa con la registrazione, e' quella; senza valere per la catena a sua volta, si sale ancora;
// valida, la catena da quella parte c'e'. Poi guarda il titolare del file (il componente del suo documento).
// Pura.
func sospesaSopra(tutte Dichiarazioni, x Dichiarazione) (Dichiarazione, bool) {
	if !x.Delega() || !(x.SenzaCatena() || x.SenzaTitolare()) {
		return Dichiarazione{}, false
	}
	perChiave := map[string]Dichiarazione{}
	var titolare *Dichiarazione
	for _, ds := range tutte.tutte {
		for _, d := range ds {
			if d.Sha256 != x.Sha256 || d.Componente.ComponenteID == x.Componente.ComponenteID {
				continue
			}
			for k := range d.Sorgenti {
				perChiave[k] = d
			}
			if !d.Delega() && x.titolare.Valid && d.Componente.ComponenteID == x.titolare.UUID {
				d := d
				titolare = &d
			}
		}
	}
	var livello []string
	for k := range x.Sorgenti {
		livello = append(livello, x.padri[k]...)
	}
	visti := map[string]bool{}
	for len(livello) > 0 {
		sort.Strings(livello)
		var sopra []string
		for _, k := range livello {
			if visti[k] {
				continue
			}
			visti[k] = true
			d, ok := perChiave[k]
			switch {
			case !ok, d.Valida():
			case d.Sospensione != nil:
				return d, true
			case d.Delega() && (d.SenzaCatena() || d.SenzaTitolare()):
				sopra = append(sopra, d.padri[k]...)
			}
		}
		livello = sopra
	}
	if titolare != nil && titolare.Sospensione != nil {
		return *titolare, true
	}
	return Dichiarazione{}, false
}

func (e EffettoRiattivazione) firmata() EffettoRiattivazione {
	x := struct {
		Componente, Tipo, Sha, Spento string
		Delega, Archiviato            bool
		FigliDiretti                  int
		ConLei, Avvisi                []string
		Sospensione                   *Sospensione
	}{e.Componente.ComponenteID.String(), string(e.Componente.Tipo), e.Sha256, e.Spento, e.Delega, e.Componente.ArchiviatoIl != nil,
		e.FigliDiretti, e.ConLei, e.Avvisi, e.Sospensione}
	b, _ := json.Marshal(x)
	h := sha256.Sum256(b)
	e.Firma = hex.EncodeToString(h[:12])
	return e
}

// RiattivaStrutturale e' la scelta esplicita di una persona: l'autorizzazione sospesa del componente (del file
// sha, o tutte) torna valida. Sotto il lucchetto della RFQ l'anteprima si ricalcola e la firma vista si
// ricontrolla; poi la sospensione va nella storia delle marcature, il file si rilegge (gli archi dell'autorita'
// tengono di nuovo i conti con la working) e le rimozioni si ricalcolano (quelle chiuse dalla sospensione, da un
// automatismo, si riaprono: E33). Un commerciale non si riattiva.
func RiattivaStrutturale(ctx context.Context, q *db.Queries, thread, utente, comp uuid.UUID, sha, firma string) (string, error) {
	if err := prepara(ctx, q, thread, "si riattiva un'autorizzazione"); err != nil {
		return "", err
	}
	if _, err := componenteDellaRfq(ctx, q, thread, comp); err != nil {
		return "", err
	}
	e, err := EffettoRiattivazioneDi(ctx, q, thread, comp, sha)
	if err != nil {
		return "", err
	}
	switch {
	case e.Spento != "":
		return "", Rifiuto(e.Spento)
	case firma == "" || firma != e.Firma:
		return "", Rifiuto("quello che la riattivazione farebbe è cambiato mentre lo guardavi: riapri l'anteprima e guarda di nuovo")
	}
	ids := make([]uuid.UUID, 0, len(e.proposte))
	for id := range e.proposte {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		n, err := q.RiattivaDichiarazione(ctx, db.RiattivaDichiarazioneParams{RiattivataDa: utente, PropostaID: id})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", Rifiuto("l'autorizzazione è cambiata nel frattempo: riapri l'anteprima")
		}
	}
	if err := rileggiIContenuti(ctx, q, thread, e.toccati); err != nil {
		return "", err
	}
	cosa := "l'autorizzazione"
	if e.Delega {
		cosa = "la delega"
	}
	msg := fmt.Sprintf("Riattivata %s di %s per %s: %s nell'autorità.", cosa, e.NomeFile, e.Componente.Codice,
		quanti(e.FigliDiretti, "figlio diretto torna", "figli diretti tornano"))
	if len(e.ConLei) > 0 {
		msg += " Tornano valide con lei le deleghe di " + strings.Join(e.ConLei, ", ") + "."
	}
	return dopoLaDecisione(ctx, q, thread, msg, nil)
}
