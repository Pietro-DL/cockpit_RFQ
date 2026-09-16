package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
)

// Admin → Anagrafica (voci 6.6, 6.11, 8.8)
//
// L'anagrafica esisteva nel modello dalla 0001 e non si poteva guardare: i clienti nascevano di
// sbieco dal triage («cliente nuovo» nel form di una RFQ) e le loro `regole` restavano `{}` per
// sempre, perché non c'era nessun posto in cui scriverle. Questa è quella schermata.
//
// È amministrativa (D29): una regex cambiata qui cambia il riconoscimento di tutta la posta, non
// una riga di una richiesta.
//
// # Non distruttiva (6.6)
//
// Le due scritture pericolose erano `UpsertCliente` e `UpsertDominioCliente`. Un `ON CONFLICT DO
// UPDATE` su una chiave che identifica QUALCUN ALTRO non è un aggiornamento: è una
// sovrascrittura, e per giunta silenziosa. Adesso sono INSERT che falliscono, e il fallimento
// diventa una frase che dice di chi è la cartella o il dominio che hai appena scritto.

// ---------------------------------------------------------------- scritture non distruttive

// vincoloViolato riconosce il 23505 di PostgreSQL e restituisce il nome del vincolo.
func vincoloViolato(err error) (string, bool) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return pg.ConstraintName, true
	}
	return "", false
}

// CreaCliente crea un cliente NUOVO. Non riusa, non rinomina, non sposta niente: se la cartella
// NAS è già di un altro cliente l'operazione fallisce e dice di chi è (T7).
//
// Perché non riusare in silenzio. Due clienti nella stessa cartella NAS vuol dire due clienti che
// si sovrascrivono i disegni; ma il caso che faceva danno era l'altro — chi scriveva il nome di una
// cartella gia' presa, credendo di creare un cliente nuovo, si portava via ragione sociale, domini,
// buyer e RFQ del cliente che quella cartella ce l'aveva gia', e nessuna schermata glielo diceva.
func CreaCliente(ctx context.Context, q *db.Queries, arg db.InsertClienteParams) (db.Cliente, error) {
	if len(arg.Regole) == 0 {
		arg.Regole = json.RawMessage("{}")
	}
	c, err := q.InsertCliente(ctx, arg)
	if nome, ok := vincoloViolato(err); ok {
		switch {
		case strings.Contains(nome, "cartella_nas"):
			if altro, e := q.GetClientePerCartella(ctx, arg.CartellaNas); e == nil {
				return c, fmt.Errorf("la cartella NAS %q è già di %s: scegline un'altra, oppure usa quel cliente invece di crearne uno nuovo", arg.CartellaNas, altro.RagioneSociale)
			}
			return c, fmt.Errorf("la cartella NAS %q è già di un altro cliente", arg.CartellaNas)
		case strings.Contains(nome, "profilo"):
			return c, fmt.Errorf("il profilo %q è già di un altro cliente", arg.Profilo.String)
		}
	}
	return c, err
}

// AggiungiDominio assegna un dominio a un cliente. Un dominio appartiene a UN cliente: se è già
// di un altro, l'assegnazione fallisce e dice di chi è (T8).
//
// La vecchia `UpsertDominioCliente` lo spostava. Spostare un dominio non cambia solo il futuro:
// `v_inbox` risolve il cliente dal dominio del mittente, quindi novecento messaggi già arrivati
// cambiano cliente insieme a lui, e il triage di tutti quelli ancora orfani cambia proposta.
func AggiungiDominio(ctx context.Context, q *db.Queries, dominio string, clienteID uuid.UUID) error {
	dominio = strings.ToLower(strings.TrimSpace(dominio))
	if dominio == "" {
		return errors.New("il dominio è vuoto")
	}
	if strings.Contains(dominio, "@") {
		return fmt.Errorf("%q è un indirizzo, non un dominio: qui va la parte dopo la chiocciola", dominio)
	}
	err := q.InsertDominioCliente(ctx, db.InsertDominioClienteParams{Lower: dominio, ClienteID: clienteID})
	if _, ok := vincoloViolato(err); ok {
		altro, e := q.GetClientePerDominio(ctx, dominio)
		if e == nil && altro.ClienteID == clienteID {
			return nil // già suo: non è un errore, è una richiesta già soddisfatta
		}
		if e == nil {
			return fmt.Errorf("il dominio %s è già censito per %s: un dominio appartiene a un cliente solo, e spostarlo cambierebbe il cliente di tutti i messaggi già arrivati", dominio, altro.RagioneSociale)
		}
		return fmt.Errorf("il dominio %s è già censito per un altro cliente", dominio)
	}
	return err
}

// ---------------------------------------------------------------- la schermata

type anagraficaDati struct {
	Tab        string // "clienti" | "articoli"
	Clienti    []db.ListClientiTuttiRow
	Scelto     *db.Cliente
	Domini     []db.DominioCliente
	Buyer      []db.Buyer
	RegoleJSON string
	Diagnosi   []domain.Diagnostica
	Fabbisogno []db.ListFabbisognoEffettivoRow
	Prova      *provaDati
	Errore     string
	Fatto      string
}

// provaDati è il banco di prova. `Motore` è lo stesso oggetto che userebbe l'ingest su un
// messaggio vero di questo cliente.
type provaDati struct {
	Testo   string
	Esito   domain.Riconoscimento
	Cliente string
}

func (s *Server) adminAnagrafica(w http.ResponseWriter, r *http.Request) {
	s.rendiAnagrafica(w, r, anagraficaDati{})
}

// rendiAnagrafica carica tutto ciò che la schermata mostra e la disegna. Passa dai `dati` già
// riempiti da chi l'ha chiamata (errore, messaggio, risultato della prova) e completa il resto:
// così ogni POST finisce sulla stessa pagina, con il suo esito sopra, invece che su un redirect
// che perde quello che aveva da dire.
func (s *Server) rendiAnagrafica(w http.ResponseWriter, r *http.Request, dati anagraficaDati) {
	ctx := r.Context()
	q := db.New(s.Pool)
	if dati.Tab == "" {
		dati.Tab = "clienti"
	}
	var err error
	if dati.Clienti, err = q.ListClientiTutti(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if dati.Scelto == nil {
		if id, e := uuid.Parse(r.FormValue("cliente")); e == nil {
			if c, e := q.GetCliente(ctx, id); e == nil {
				dati.Scelto = &c
			}
		}
	}
	if c := dati.Scelto; c != nil {
		dati.Domini, _ = q.ListDominiCliente(ctx, c.ClienteID)
		dati.Buyer, _ = q.ListBuyerCliente(ctx, c.ClienteID)
		dati.Fabbisogno, _ = q.ListFabbisognoEffettivo(ctx, uuid.NullUUID{UUID: c.ClienteID, Valid: true})
		var regole domain.Regole
		regole, dati.Diagnosi = domain.LeggiRegole(c.Regole)
		if dati.RegoleJSON == "" {
			dati.RegoleJSON = indenta(c.Regole, regole)
		}
	}
	s.rendi(w, r, "anagrafica.html", "anagrafica_corpo", "Anagrafica", dati)
}

// indenta mostra il JSON nel riquadro in forma leggibile. Se il testo in database non è
// rileggibile lo si mostra COM'È: riscriverlo «pulito» cancellerebbe l'unica copia di ciò che
// qualcuno deve ancora correggere.
func indenta(raw []byte, r domain.Regole) string {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return "{}"
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil || !json.Valid(raw) {
		return string(raw)
	}
	return string(b)
}

// articoli è lo SCHELLETRO della seconda scheda e nient'altro. Il modello `articolo` è il blocco
// 5, e una schermata che mostrasse qualcosa adesso mostrerebbe `componente`, cioè l'istanza in
// una RFQ — che è proprio la confusione che il blocco 5 deve togliere di mezzo.
func (s *Server) adminAnagraficaArticoli(w http.ResponseWriter, r *http.Request) {
	s.rendiAnagrafica(w, r, anagraficaDati{Tab: "articoli"})
}

// ---------------------------------------------------------------- scritture

func (s *Server) nuovoCliente(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	nome := strings.TrimSpace(r.FormValue("ragione_sociale"))
	cartella := domain.NomeSicuro(strings.ToUpper(strings.TrimSpace(r.FormValue("cartella_nas"))), 80)
	if nome == "" || cartella == "" || cartella == "senza nome" {
		s.rendiAnagrafica(w, r, anagraficaDati{Errore: "servono la ragione sociale e il nome della cartella NAS"})
		return
	}
	c, err := CreaCliente(ctx, q, db.InsertClienteParams{
		CartellaNas: cartella, RagioneSociale: nome, Lingua: txtN(strings.ToLower(r.FormValue("lingua")), 2),
	})
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Errore: err.Error()})
		return
	}
	if dom := r.FormValue("dominio"); strings.TrimSpace(dom) != "" {
		if err := AggiungiDominio(ctx, q, dom, c.ClienteID); err != nil {
			s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Errore: "cliente creato, ma il dominio no: " + err.Error()})
			return
		}
	}
	s.Log.Info("cliente creato", "cliente", c.RagioneSociale, "cartella", c.CartellaNas, "utente", siglaDa(r))
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Fatto: "Cliente creato."})
}

func (s *Server) salvaCliente(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	c, ok := s.clienteDaURL(w, r, q)
	if !ok {
		return
	}
	peso, err := strconv.Atoi(strings.TrimSpace(primo(r.FormValue("peso"), "0")))
	if err != nil || peso < 0 || peso > 15 {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Errore: "il peso va da 0 a 15 (0 = nessuna priorità dichiarata)"})
		return
	}
	agg, err := q.UpdateCliente(ctx, db.UpdateClienteParams{
		ClienteID: c.ClienteID, RagioneSociale: primo(strings.TrimSpace(r.FormValue("ragione_sociale")), c.RagioneSociale),
		Profilo: c.Profilo, Lingua: txtN(strings.ToLower(strings.TrimSpace(r.FormValue("lingua"))), 2),
		PortaleUrl: txtN(strings.TrimSpace(r.FormValue("portale_url")), 300), PortaleNote: txtN(r.FormValue("portale_note"), 4000),
		Peso: int16(peso), Attivo: r.FormValue("attivo") != "", Note: txtN(r.FormValue("note"), 4000),
	})
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Errore: err.Error()})
		return
	}
	s.Log.Info("cliente aggiornato", "cliente", agg.RagioneSociale, "peso", agg.Peso, "utente", siglaDa(r))
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &agg, Fatto: "Salvato."})
}

// salvaRegole è la porta in scrittura di D17: quello che non rispetta lo schema NON entra. Il
// testo rifiutato torna nel riquadro così com'era scritto, perché il primo effetto di un
// salvataggio rifiutato che perde il testo è che la volta dopo non si prova più a correggerlo.
func (s *Server) salvaRegole(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	c, ok := s.clienteDaURL(w, r, q)
	if !ok {
		return
	}
	testo := strings.TrimSpace(r.FormValue("regole"))
	if testo == "" {
		testo = "{}"
	}
	regole, err := domain.ValidaRegole([]byte(testo))
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, RegoleJSON: testo, Errore: err.Error(),
			Diagnosi: diagnosiDiUnTestoRifiutato(testo)})
		return
	}
	pulito, _ := json.Marshal(regole)
	agg, err := q.SetRegoleCliente(r.Context(), db.SetRegoleClienteParams{ClienteID: c.ClienteID, Regole: pulito})
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, RegoleJSON: testo, Errore: err.Error()})
		return
	}
	s.Log.Info("regole cliente salvate", "cliente", agg.RagioneSociale, "famiglie", len(regole.FamiglieCodice), "utente", siglaDa(r))
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &agg, Fatto: "Regole salvate: tutte le regole hanno un esempio che corrisponde."})
}

// diagnosiDiUnTestoRifiutato mostra il ✓/✗ anche quando il salvataggio è stato rifiutato: serve a
// vedere QUALE riga è rotta, non solo che qualcosa lo è.
func diagnosiDiUnTestoRifiutato(testo string) []domain.Diagnostica {
	_, d := domain.LeggiRegole([]byte(testo))
	return d
}

func (s *Server) aggiungiDominioCliente(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	c, ok := s.clienteDaURL(w, r, q)
	if !ok {
		return
	}
	if err := AggiungiDominio(r.Context(), q, r.FormValue("dominio"), c.ClienteID); err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Errore: err.Error()})
		return
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Fatto: "Dominio aggiunto."})
}

func (s *Server) eliminaDominioCliente(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	c, ok := s.clienteDaURL(w, r, q)
	if !ok {
		return
	}
	dom := strings.ToLower(strings.TrimSpace(r.FormValue("dominio")))
	if err := q.EliminaDominioCliente(r.Context(), db.EliminaDominioClienteParams{Lower: dom, ClienteID: c.ClienteID}); err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Errore: err.Error()})
		return
	}
	s.Log.Info("dominio rimosso", "dominio", dom, "cliente", c.RagioneSociale, "utente", siglaDa(r))
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Fatto: "Dominio rimosso da questo cliente."})
}

// bancoProva fa passare un testo incollato per lo STESSO riconoscimento dei messaggi veri
// (`domain.Riconosci`), con le regole di questo cliente compilate come le compilerebbe l'ingest.
// Non c'è un secondo motore: se questa schermata e l'Inbox dicessero cose diverse, sarebbe questa
// a mentire, ed è quella su cui si tarano le regex.
func (s *Server) bancoProva(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	c, ok := s.clienteDaURL(w, r, q)
	if !ok {
		return
	}
	testo := r.FormValue("testo")
	oggetto, corpo := primaRigaEResto(testo)
	regole, _ := domain.LeggiRegole(c.Regole)
	in := domain.IngressoTriage{
		Oggetto: oggetto, Corpo: corpo, Direzione: string(db.DirezioneEntrata),
		ClienteNoto: true, Motore: domain.Compila(c.RagioneSociale, regole),
	}
	if n := strings.TrimSpace(r.FormValue("allegati")); n != "" {
		in.NomiAllegati = strings.Fields(n)
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c,
		Prova: &provaDati{Testo: testo, Cliente: c.RagioneSociale, Esito: domain.Riconosci(in, time.Now())}})
}

// primaRigaEResto: nel banco si incolla una mail intera. La prima riga fa da oggetto — è come
// arriva quando si copia da Outlook — e il resto è il corpo. Un "Oggetto:" iniziale si toglie.
func primaRigaEResto(t string) (string, string) {
	t = strings.ReplaceAll(t, "\r\n", "\n")
	riga, resto, _ := strings.Cut(t, "\n")
	riga = strings.TrimSpace(riga)
	for _, p := range []string{"oggetto:", "subject:", "betreff:"} {
		if strings.HasPrefix(strings.ToLower(riga), p) {
			riga = strings.TrimSpace(riga[len(p):])
			break
		}
	}
	return riga, resto
}

// ---------------------------------------------------------------- Richieste

// richieste è la lista di lavoro: le RFQ aperte, quelle dei clienti con il peso più alto in cima.
//
// L'ordinamento è `cliente.peso` e poi la scadenza, e si ferma lì. Il PUNTEGGIO di priorità
// dell'addendum 2 — quello con i pesi 40/25/20/15 e le soglie — non esiste ancora, e le sue
// regole vanno rese esplicite prima di essere codificate: inventarne una versione provvisoria qui
// vorrebbe dire che l'ordine di lavoro di tutti dipende da una formula che nessuno ha approvato.
func (s *Server) richieste(w http.ResponseWriter, r *http.Request) {
	righe, err := db.New(s.Pool).ListRichieste(r.Context(), 200)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.rendi(w, r, "richieste.html", "richieste_tabella", "Richieste", righe)
}

// ---------------------------------------------------------------- minuterie

func (s *Server) clienteDaURL(w http.ResponseWriter, r *http.Request, q *db.Queries) (db.Cliente, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "cliente non valido", http.StatusBadRequest)
		return db.Cliente{}, false
	}
	c, err := q.GetCliente(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "cliente non trovato", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), 500)
		}
		return db.Cliente{}, false
	}
	return c, true
}

func siglaDa(r *http.Request) string {
	if u := utenteDa(r.Context()); u != nil {
		return u.Sigla
	}
	return "?"
}

// txtN è `ptxt` con un tetto: le colonne hanno un varchar, e un testo più lungo deve arrivare
// tagliato invece di far fallire il salvataggio con un errore del database in faccia a chi sta
// solo scrivendo una nota.
func txtN(s string, max int) pgtype.Text {
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return ptxt(s)
}

func primo(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
