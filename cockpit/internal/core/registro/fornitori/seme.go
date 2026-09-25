// Package fornitori importa fornitori, domini, contatti, lavorazioni e qualifiche da un file JSON,
// con ANTEPRIMA e CONFERMA (blocco 7A.4).
//
// # Perché un'anteprima e non un seme che scrive
//
// Il foglio dei clienti e dei verniciatori è una fotografia scritta a mano: dice che cosa qualcuno
// credeva vero un giorno, e contiene nomi che nell'anagrafica non ci sono, lavorazioni che nessuno
// ha dichiarato, celle di cui non si sa il significato. Applicarlo in silenzio vorrebbe dire
// scrivere in database le sue incertezze. Qui il file si legge, si confronta con ciò che c'è, e si
// mostra: che cosa verrebbe creato, che cosa c'è già, che cosa NON si riesce a risolvere. Si scrive
// solo alla conferma, e si scrive solo ciò che l'anteprima ha detto.
//
// # Che cosa non fa
//
// Non sovrascrive un fornitore che c'è già: nemmeno un campo. Non sposta un dominio da un fornitore
// a un altro. Non inventa una qualifica su una lavorazione che il fornitore non fa: la chiave
// esterna composta la rifiuterebbe, e qui la si vede prima come «non risolta». Non interpreta il
// foglio: le celle verdi non hanno un campo, e finché non se ne conosce il significato non lo
// avranno.
package fornitori

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// Seme è il file: un oggetto con una sola chiave, come il seme dei clienti.
type Seme struct {
	Fornitori []FornitoreSeme `json:"fornitori"`
}

type FornitoreSeme struct {
	RagioneSociale string          `json:"ragione_sociale"`
	Tipo           string          `json:"tipo"` // materie_prime | processi | verniciatore
	Lingua         string          `json:"lingua,omitempty"`
	Note           string          `json:"note,omitempty"`
	Domini         []string        `json:"domini,omitempty"`
	Contatti       []ContattoSeme  `json:"contatti,omitempty"`
	Lavorazioni    []string        `json:"lavorazioni,omitempty"`
	Qualifiche     []QualificaSeme `json:"qualifiche,omitempty"`
}

type ContattoSeme struct {
	Nome  string `json:"nome,omitempty"`
	Email string `json:"email"`
	Ruolo string `json:"ruolo,omitempty"`
}

// QualificaSeme: «questo fornitore è qualificato per questo cliente su questa lavorazione». Il
// cliente si nomina con la cartella NAS, che è la sua chiave stabile.
type QualificaSeme struct {
	Cliente     string `json:"cliente"`
	Lavorazione string `json:"lavorazione"`
}

// Leggi legge e CONVALIDA il file. Ogni errore è un errore del file, e dice dove.
func Leggi(r io.Reader) (Seme, error) {
	var s Seme
	dec := json.NewDecoder(senzaBOM(r))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("il file non è un seme leggibile: %w", err)
	}
	if len(s.Fornitori) == 0 {
		return s, errors.New("il file non contiene nessun fornitore («fornitori» vuoto o assente)")
	}
	visti := map[string]bool{}
	for i := range s.Fornitori {
		f := &s.Fornitori[i]
		f.RagioneSociale = strings.TrimSpace(f.RagioneSociale)
		dove := fmt.Sprintf("fornitore %d (%s)", i+1, f.RagioneSociale)
		if f.RagioneSociale == "" {
			return s, fmt.Errorf("fornitore %d: manca la ragione sociale", i+1)
		}
		if chiave := strings.ToLower(f.RagioneSociale); visti[chiave] {
			return s, fmt.Errorf("%s: compare due volte nel file", dove)
		} else {
			visti[chiave] = true
		}
		if !db.TipoFornitore(f.Tipo).Valid() {
			return s, fmt.Errorf("%s: tipo «%s» non previsto (materie_prime | processi | verniciatore)", dove, f.Tipo)
		}
		if f.Lingua != "" && (len(f.Lingua) != 2 || strings.ToLower(f.Lingua) != f.Lingua) {
			return s, fmt.Errorf("%s: la lingua va scritta con due lettere minuscole", dove)
		}
		for j, d := range f.Domini {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" || strings.Contains(d, "@") || !strings.Contains(d, ".") {
				return s, fmt.Errorf("%s: «%s» non è un dominio (serve la parte dopo la chiocciola, con un punto)", dove, f.Domini[j])
			}
			f.Domini[j] = d
		}
		for j, c := range f.Contatti {
			e := strings.ToLower(strings.TrimSpace(c.Email))
			if !strings.Contains(e, "@") {
				return s, fmt.Errorf("%s: il contatto «%s» non ha un'email valida", dove, c.Email)
			}
			f.Contatti[j].Email = e
		}
		for j, l := range f.Lavorazioni {
			l = strings.ToLower(strings.TrimSpace(l))
			if l == "" {
				return s, fmt.Errorf("%s: una lavorazione è vuota", dove)
			}
			f.Lavorazioni[j] = l
		}
		for j, qs := range f.Qualifiche {
			qs.Cliente, qs.Lavorazione = strings.TrimSpace(qs.Cliente), strings.ToLower(strings.TrimSpace(qs.Lavorazione))
			if qs.Cliente == "" || qs.Lavorazione == "" {
				return s, fmt.Errorf("%s: una qualifica senza cliente o senza lavorazione", dove)
			}
			f.Qualifiche[j] = qs
		}
	}
	return s, nil
}

// LeggiFile è Leggi su un percorso.
func LeggiFile(percorso string) (Seme, error) {
	f, err := os.Open(percorso)
	if err != nil {
		return Seme{}, err
	}
	defer f.Close()
	return Leggi(f)
}

// Riga è una voce dell'anteprima: a quale fornitore si riferisce, che cosa, e il dettaglio.
type Riga struct {
	Fornitore string
	Cosa      string
	Dettaglio string
}

func (r Riga) String() string {
	if r.Dettaglio == "" {
		return r.Fornitore + ": " + r.Cosa
	}
	return r.Fornitore + ": " + r.Cosa + " — " + r.Dettaglio
}

// Anteprima è ciò che l'import FAREBBE (prima della conferma) o ciò che HA FATTO (dopo): le stesse
// voci, così chi conferma può confrontare.
type Anteprima struct {
	FornitoriDaCreare []string
	FornitoriPresenti []string // c'erano già: nemmeno un campo toccato
	DaAggiungere      []Riga   // domini, contatti, lavorazioni, qualifiche che verranno scritti
	Presenti          []Riga   // già in database: niente da fare
	NonRisolti        []Riga   // ciò che il file dice e l'anagrafica non sa risolvere: NON si scrive
	Avvisi            []Riga   // si scrive, ma vale la pena saperlo
	// DominiScritti e IndirizziScritti sono le sole voci che cambiano la RISPOSTA alla domanda «di
	// chi è questa mail»: dopo averle scritte, i messaggi già arrivati da quei domini o da quegli
	// indirizzi vanno ricalcolati, o l'Inbox continuerà a chiamarli «sconosciuti» (7B.5). Prima
	// della conferma dicono che cosa verrebbe scritto; dopo, che cosa è stato scritto.
	DominiScritti    []string
	IndirizziScritti []string
}

// Vuota: niente da scrivere.
func (a Anteprima) Vuota() bool { return len(a.FornitoriDaCreare) == 0 && len(a.DaAggiungere) == 0 }

// piano è l'anteprima più le operazioni tipizzate che la realizzano.
type piano struct {
	Anteprima
	crea        []FornitoreSeme
	domini      []opDominio
	contatti    []opContatto
	lavorazioni []opLavorazione
	qualifiche  []opQualifica
}

type opDominio struct{ fornitore, dominio string }
type opContatto struct {
	fornitore string
	c         ContattoSeme
}
type opLavorazione struct{ fornitore, codice string }
type opQualifica struct {
	fornitore   string
	clienteID   uuid.UUID
	cartella    string
	lavorazione string
}

// Calcola confronta il seme con l'anagrafica e non scrive niente.
func Calcola(ctx context.Context, q *db.Queries, s Seme) (Anteprima, error) {
	p, err := calcola(ctx, q, s)
	if err != nil {
		return Anteprima{}, err
	}
	return p.Anteprima, nil
}

func calcola(ctx context.Context, q *db.Queries, s Seme) (*piano, error) {
	p := &piano{}
	lavorazioniNote := map[string]bool{}
	tutte, err := q.ListLavorazioni(ctx)
	if err != nil {
		return nil, err
	}
	for _, l := range tutte {
		lavorazioniNote[l.Codice] = true
	}
	// Ciò che il piano ha già deciso di scrivere, per tutto il file. Un foglio compilato a mano ripete
	// le cose (lo stesso dominio sotto due fornitori di un gruppo, la stessa email due volte): senza
	// questo l'anteprima diceva «da aggiungere» due volte, il pulsante era attivo, e Applica si fermava
	// sulla chiave primaria annullando tutto l'import. Il database non si interroga una seconda volta:
	// la seconda occorrenza si dice qui, con il motivo.
	dominiDelPiano := map[string]string{} // dominio → fornitore che lo riceve
	for _, f := range s.Fornitori {
		nome := f.RagioneSociale
		esistente, err := q.GetFornitorePerRagioneSociale(ctx, nome)
		var id uuid.NullUUID
		switch {
		case err == nil:
			id = uuid.NullUUID{UUID: esistente.FornitoreID, Valid: true}
			p.FornitoriPresenti = append(p.FornitoriPresenti, nome)
			// il seme non cambia nemmeno un campo di chi c'e' gia': se dice un tipo diverso, lo si
			// legge qui e si decide a mano
			if string(esistente.Tipo) != f.Tipo {
				p.Avvisi = append(p.Avvisi, Riga{nome, "tipo", "il file dice «" + f.Tipo + "», in anagrafica e' «" + string(esistente.Tipo) + "»: non si cambia dall'import"})
			}
		case errors.Is(err, pgx.ErrNoRows):
			p.FornitoriDaCreare = append(p.FornitoriDaCreare, nome)
			p.crea = append(p.crea, f)
		default:
			return nil, err
		}
		// capacità: quelle che ha più quelle del file, per decidere le qualifiche
		capacita := map[string]bool{}
		if id.Valid {
			proprie, err := q.ListLavorazioniFornitore(ctx, id.UUID)
			if err != nil {
				return nil, err
			}
			for _, l := range proprie {
				capacita[l.Codice] = true
			}
		}
		for _, d := range f.Domini {
			if chi, gia := dominiDelPiano[d]; gia {
				if chi == nome {
					p.Avvisi = append(p.Avvisi, Riga{nome, "dominio " + d, "ripetuto nel file: si scrive una volta"})
				} else {
					p.NonRisolti = append(p.NonRisolti, Riga{nome, "dominio " + d, "nel file è anche del fornitore " + chi + ": un dominio è di un fornitore solo"})
				}
				continue
			}
			altro, err := q.GetFornitorePerDominio(ctx, d)
			switch {
			case err == nil && id.Valid && altro.FornitoreID == id.UUID:
				p.Presenti = append(p.Presenti, Riga{nome, "dominio " + d, ""})
				continue
			case err == nil:
				p.NonRisolti = append(p.NonRisolti, Riga{nome, "dominio " + d, "è già del fornitore " + altro.RagioneSociale + ": un dominio non si sposta"})
				continue
			case !errors.Is(err, pgx.ErrNoRows):
				return nil, err
			}
			if c, err := q.GetClientePerDominio(ctx, d); err == nil {
				p.Avvisi = append(p.Avvisi, Riga{nome, "dominio " + d, "è censito anche per il cliente " + c.RagioneSociale + ": la posta da quel dominio sarà «ambigua»"})
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			p.DaAggiungere = append(p.DaAggiungere, Riga{nome, "dominio " + d, ""})
			p.DominiScritti = append(p.DominiScritti, d)
			p.domini = append(p.domini, opDominio{nome, d})
			dominiDelPiano[d] = nome
		}
		esistenti := map[string]bool{}
		if id.Valid {
			cs, err := q.ListContattiFornitore(ctx, id.UUID)
			if err != nil {
				return nil, err
			}
			for _, c := range cs {
				esistenti[c.Email] = true
			}
		}
		nelPiano := map[string]bool{}
		for _, c := range f.Contatti {
			if esistenti[c.Email] {
				p.Presenti = append(p.Presenti, Riga{nome, "contatto " + c.Email, ""})
				continue
			}
			if nelPiano[c.Email] {
				p.Avvisi = append(p.Avvisi, Riga{nome, "contatto " + c.Email, "ripetuto nel file: si scrive una volta"})
				continue
			}
			nelPiano[c.Email] = true
			p.DaAggiungere = append(p.DaAggiungere, Riga{nome, "contatto " + c.Email, c.Nome})
			p.IndirizziScritti = append(p.IndirizziScritti, c.Email)
			p.contatti = append(p.contatti, opContatto{nome, c})
		}
		for _, l := range f.Lavorazioni {
			switch {
			case !lavorazioniNote[l]:
				p.NonRisolti = append(p.NonRisolti, Riga{nome, "lavorazione " + l, "non esiste nella tabella delle lavorazioni"})
			case capacita[l]:
				p.Presenti = append(p.Presenti, Riga{nome, "lavorazione " + l, ""})
			default:
				p.DaAggiungere = append(p.DaAggiungere, Riga{nome, "lavorazione " + l, ""})
				p.lavorazioni = append(p.lavorazioni, opLavorazione{nome, l})
				capacita[l] = true
			}
		}
		qualifiche := map[string]bool{}
		if id.Valid {
			qs, err := q.ListQualificheFornitore(ctx, id.UUID)
			if err != nil {
				return nil, err
			}
			for _, x := range qs {
				qualifiche[x.CartellaNas+"|"+x.Lavorazione] = true
			}
		}
		for _, qs := range f.Qualifiche {
			cosa := "qualifica " + qs.Cliente + " / " + qs.Lavorazione
			cliente, err := q.GetClientePerCartella(ctx, qs.Cliente)
			if errors.Is(err, pgx.ErrNoRows) {
				p.NonRisolti = append(p.NonRisolti, Riga{nome, cosa, "il cliente «" + qs.Cliente + "» non è in anagrafica (si cerca per cartella NAS)"})
				continue
			}
			if err != nil {
				return nil, err
			}
			if !lavorazioniNote[qs.Lavorazione] {
				p.NonRisolti = append(p.NonRisolti, Riga{nome, cosa, "la lavorazione non esiste"})
				continue
			}
			if !capacita[qs.Lavorazione] {
				p.NonRisolti = append(p.NonRisolti, Riga{nome, cosa, "il fornitore non dichiara questa lavorazione: prima la capacità, poi la qualifica"})
				continue
			}
			if qualifiche[cliente.CartellaNas+"|"+qs.Lavorazione] {
				p.Presenti = append(p.Presenti, Riga{nome, cosa, ""})
				continue
			}
			qualifiche[cliente.CartellaNas+"|"+qs.Lavorazione] = true // una seconda riga uguale nel file è «presente»
			p.DaAggiungere = append(p.DaAggiungere, Riga{nome, cosa, ""})
			p.qualifiche = append(p.qualifiche, opQualifica{nome, cliente.ClienteID, cliente.CartellaNas, qs.Lavorazione})
		}
	}
	sort.Strings(p.FornitoriDaCreare)
	sort.Strings(p.FornitoriPresenti)
	return p, nil
}

// Applica scrive ciò che l'anteprima ha detto, in una transazione, e restituisce l'anteprima
// applicata. Un secondo import dello stesso file non scrive niente: tutto risulta «presente».
func Applica(ctx context.Context, pool *pgxpool.Pool, s Seme) (Anteprima, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Anteprima{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	p, err := calcola(ctx, q, s)
	if err != nil {
		return Anteprima{}, err
	}
	id := map[string]uuid.UUID{}
	for _, f := range p.crea {
		row, err := q.InsertFornitore(ctx, db.InsertFornitoreParams{RagioneSociale: f.RagioneSociale, Tipo: db.TipoFornitore(f.Tipo),
			Lingua: txt(f.Lingua), Note: txt(f.Note)})
		if err != nil {
			return Anteprima{}, fmt.Errorf("fornitore %s: %w", f.RagioneSociale, err)
		}
		id[f.RagioneSociale] = row.FornitoreID
	}
	idDi := func(nome string) (uuid.UUID, error) {
		if x, ok := id[nome]; ok {
			return x, nil
		}
		f, err := q.GetFornitorePerRagioneSociale(ctx, nome)
		if err != nil {
			return uuid.Nil, fmt.Errorf("fornitore %s: %w", nome, err)
		}
		id[nome] = f.FornitoreID
		return f.FornitoreID, nil
	}
	for _, op := range p.domini {
		fid, err := idDi(op.fornitore)
		if err != nil {
			return Anteprima{}, err
		}
		if err := q.InsertDominioFornitore(ctx, db.InsertDominioFornitoreParams{Lower: op.dominio, FornitoreID: fid}); err != nil {
			return Anteprima{}, fmt.Errorf("dominio %s: %w", op.dominio, err)
		}
	}
	for _, op := range p.contatti {
		fid, err := idDi(op.fornitore)
		if err != nil {
			return Anteprima{}, err
		}
		if _, err := q.InsertContattoFornitore(ctx, db.InsertContattoFornitoreParams{FornitoreID: fid, Nome: txt(op.c.Nome), Lower: op.c.Email, Ruolo: txt(op.c.Ruolo)}); err != nil {
			return Anteprima{}, fmt.Errorf("contatto %s: %w", op.c.Email, err)
		}
	}
	for _, op := range p.lavorazioni {
		fid, err := idDi(op.fornitore)
		if err != nil {
			return Anteprima{}, err
		}
		if _, err := q.InsertLavorazioneFornitore(ctx, db.InsertLavorazioneFornitoreParams{FornitoreID: fid, Lavorazione: op.codice}); err != nil {
			return Anteprima{}, fmt.Errorf("lavorazione %s di %s: %w", op.codice, op.fornitore, err)
		}
	}
	for _, op := range p.qualifiche {
		fid, err := idDi(op.fornitore)
		if err != nil {
			return Anteprima{}, err
		}
		if _, err := q.InsertQualifica(ctx, db.InsertQualificaParams{ClienteID: op.clienteID, FornitoreID: fid, Lavorazione: op.lavorazione}); err != nil {
			return Anteprima{}, fmt.Errorf("qualifica %s / %s di %s: %w", op.cartella, op.lavorazione, op.fornitore, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Anteprima{}, err
	}
	return p.Anteprima, nil
}

func txt(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

// senzaBOM toglie la firma UTF-8 che Windows mette in testa a un file salvato con Blocco note o con
// `Out-File`. Non è un dettaglio da puristi: senza, il file viene rifiutato con «invalid character
// '\ufeff' looking for beginning of value», che non dice a nessuno che cosa fare. Il contenuto è
// giusto, il problema sono tre byte invisibili.
func senzaBOM(r io.Reader) io.Reader {
	b := bufio.NewReader(r)
	if primi, err := b.Peek(3); err == nil && primi[0] == 0xEF && primi[1] == 0xBB && primi[2] == 0xBF {
		_, _ = b.Discard(3)
	}
	return b
}
