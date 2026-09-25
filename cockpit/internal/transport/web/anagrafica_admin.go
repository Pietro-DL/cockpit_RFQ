package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

// Admin → Anagrafica, la parte amministrabile (checkpoint 3R §7).
//
// La schermata del blocco 3 mostrava tutto in una colonna alta due schermi e, in fondo, una tabella
// del fabbisogno con scritto «si modificano dal database». Una tabella che sembra configurabile e non
// lo è è peggio di una tabella assente: promette una cosa che non c'è, e chi ci prova perde tempo
// prima di scoprirlo. Qui quella promessa viene mantenuta.

// ---------------------------------------------------------------- form strutturato delle regole

// salvaRegoleDalForm costruisce il JSON delle regole dai campi del form e lo salva passando dalla
// STESSA porta in scrittura del riquadro JSON (`regole.ValidaRegole`).
//
// Perché un form e non il JSON. Il JSON era l'interfaccia primaria, e un'interfaccia primaria fatta
// di parentesi graffe ha due difetti: chi la usa deve conoscere lo schema a memoria, e ogni errore di
// battitura diventa un salvataggio rifiutato invece di un campo sbagliato. Il riquadro resta sotto
// «Avanzato», perché per una regex complicata scrivere il JSON è ancora il modo più rapido, e perché
// è l'unico modo di vedere che cosa c'è davvero in database.
//
// Le due strade si incontrano nello stesso convalidatore. Se divergessero, il form potrebbe salvare
// una regola che il riquadro rifiuta, e nessuno saprebbe quale delle due dice la verità.
func (s *Server) salvaRegoleDalForm(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	reg := regole.Regole{
		CanaleAtteso:           strings.TrimSpace(r.FormValue("canale_atteso")),
		LinguaRisposta:         strings.ToLower(strings.TrimSpace(r.FormValue("lingua_risposta"))),
		RichiedeCBD:            r.FormValue("richiede_cbd") == "1",
		NumeroOrdineAnticipato: r.FormValue("numero_ordine_anticipato") == "1",
	}
	if n, e := strconv.Atoi(strings.TrimSpace(r.FormValue("finestra_aggancio_gg"))); e == nil {
		reg.FinestraAggancioGG = n
	}
	if n, e := strconv.Atoi(strings.TrimSpace(r.FormValue("risposta_entro_gg"))); e == nil {
		reg.RispostaEntroGG = n
	}
	for _, d := range strings.Split(r.FormValue("dati_richiesti"), "\n") {
		if d = strings.TrimSpace(d); d != "" {
			reg.DatiRichiesti = append(reg.DatiRichiesti, d)
		}
	}
	for _, f := range strings.Split(r.FormValue("frasi_portale"), "\n") {
		if f = strings.TrimSpace(f); f != "" {
			reg.FrasiPortale = append(reg.FrasiPortale, f)
		}
	}
	for _, x := range strings.Split(r.FormValue("suffissi_decorativi"), "\n") {
		if x = strings.TrimSpace(x); x != "" {
			reg.SuffissiDecorativi = append(reg.SuffissiDecorativi, x)
		}
	}
	if rex := strings.TrimSpace(r.FormValue("rif_regex")); rex != "" {
		reg.RiferimentoRFQ = &regole.Riferimento{
			Regex: rex, Descrizione: strings.TrimSpace(r.FormValue("rif_descrizione")),
			Esempio: strings.TrimSpace(r.FormValue("rif_esempio")),
		}
	}
	// Le famiglie arrivano come array paralleli. Una riga con la regex vuota è una riga cancellata:
	// è così che si toglie una famiglia, senza un bottone «elimina» per ognuna.
	//
	// «Rev nel codice» NO: è una casella di spunta, e il browser manda solo quelle spuntate. Letta
	// come array parallelo, con F1 senza e F2 con la revisione arrivava un solo valore, che finiva su
	// F1: i flag si scambiavano, o il salvataggio si rifiutava su una famiglia che nessuno aveva
	// toccato. Ogni casella porta il numero della sua riga, e qui si legge l'insieme delle righe.
	regex, desc, esempi := r.Form["fam_regex"], r.Form["fam_descrizione"], r.Form["fam_esempio"]
	ruoli := r.Form["fam_ruolo"]
	conRev := map[int]bool{}
	for _, v := range r.Form["fam_rev"] {
		if n, e := strconv.Atoi(strings.TrimSpace(v)); e == nil {
			conRev[n] = true
		}
	}
	for i := range regex {
		if strings.TrimSpace(regex[i]) == "" {
			continue
		}
		f := regole.FamigliaCodice{Regex: strings.TrimSpace(regex[i]), RevNelCodice: conRev[i]}
		if i < len(desc) {
			f.Descrizione = strings.TrimSpace(desc[i])
		}
		if i < len(esempi) {
			f.Esempio = strings.TrimSpace(esempi[i])
		}
		if i < len(ruoli) && ruoli[i] == "parte" {
			f.Ruolo = "parte"
		}
		reg.FamiglieCodice = append(reg.FamiglieCodice, f)
	}
	raw, err := json.Marshal(reg)
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Sez: "riconoscimento", Errore: err.Error()})
		return
	}
	s.scriviRegole(w, r, id, raw, "riconoscimento")
}

// scriviRegole è la porta unica: convalida e salva. La usano il form e il riquadro JSON.
func (s *Server) scriviRegole(w http.ResponseWriter, r *http.Request, id uuid.UUID, raw []byte, sez string) {
	ctx := r.Context()
	q := db.New(s.Pool)
	c, err := q.GetCliente(ctx, id)
	if err != nil {
		http.Error(w, "cliente non trovato", 404)
		return
	}
	if _, err := regole.ValidaRegole(raw); err != nil {
		// il testo rifiutato torna nel riquadro: riscriverlo da capo dopo un errore è il modo più
		// sicuro per farne un secondo
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: sez, RegoleJSON: string(raw), Errore: err.Error()})
		return
	}
	nuovo, err := q.SetRegoleCliente(ctx, db.SetRegoleClienteParams{ClienteID: id, Regole: raw})
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: sez, RegoleJSON: string(raw), Errore: err.Error()})
		return
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &nuovo, Sez: sez,
		Fatto: "Regole salvate. Valgono dal prossimo lotto di posta."})
}

// ---------------------------------------------------------------- persone (buyer)

func (s *Server) nuovoBuyerCliente(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	cognome := strings.TrimSpace(r.FormValue("cognome"))
	if cognome == "" {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti",
			Errore: "il cognome è obbligatorio: è il modo in cui la persona compare nel nome della cartella della richiesta"})
		return
	}
	_, err := q.InsertBuyer(ctx, db.InsertBuyerParams{
		ClienteID: id, Cognome: cognome, Nome: txtN(strings.TrimSpace(r.FormValue("nome")), 60),
		Email:    txtN(strings.ToLower(strings.TrimSpace(r.FormValue("email"))), 120),
		Telefono: txtN(strings.TrimSpace(r.FormValue("telefono")), 40),
		Ruolo:    txtN(strings.TrimSpace(r.FormValue("ruolo")), 60), Tipo: tipoBuyer(r.FormValue("tipo")),
		Origine: db.OrigineAnagraficaManuale,
	})
	if err != nil {
		if _, dup := vincoloViolato(err); dup {
			err = fmt.Errorf("esiste già una persona con questo cognome o questa e-mail (anche presso un altro cliente: l'indirizzo è unico)")
		}
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti", Errore: err.Error()})
		return
	}
	fatto := "Persona aggiunta."
	if email := strings.ToLower(strings.TrimSpace(r.FormValue("email"))); email != "" {
		fatto += s.ritriagePer(ctx, email, "")
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti", Fatto: fatto})
}

func (s *Server) eliminaBuyerCliente(w http.ResponseWriter, r *http.Request) {
	_, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	q := db.New(s.Pool)
	bid, err := uuid.Parse(r.FormValue("buyer_id"))
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti", Errore: "persona non valida"})
		return
	}
	n, err := q.EliminaBuyer(r.Context(), db.EliminaBuyerParams{BuyerID: bid, ClienteID: c.ClienteID})
	switch {
	case err != nil:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti", Errore: err.Error()})
	case n == 0:
		// non è un errore tecnico: è una persona che ha scritto davvero, e cancellarla toglierebbe il
		// nome a messaggi e richieste che esistono
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti",
			Errore: "questa persona è citata da messaggi, richieste o proposte (oppure non è di questo cliente) e non si cancella: toglierla vorrebbe dire non sapere più chi aveva scritto"})
	default:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "contatti", Fatto: "Persona rimossa."})
	}
}

func tipoBuyer(v string) db.TipoBuyer {
	if t := db.TipoBuyer(v); t.Valid() {
		return t
	}
	return db.TipoBuyerBuyer
}

// ---------------------------------------------------------------- fabbisogno documentale

// aggiungiFabbisogno mette una riga DI QUESTO cliente.
//
// Attenzione alla risoluzione IN BLOCCO, che è la regola di `v_fascicolo` e da qui non si vedrebbe:
// per un dato `tipo_componente`, se il cliente ha anche una sola riga valgono le sue e i predefiniti
// spariscono. Aggiungere «capitolato» per i finiti non aggiunge una riga alle due predefinite: le
// sostituisce tutte. La schermata lo dice prima, non dopo.
func (s *Server) aggiungiFabbisogno(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	comp, tipo := r.FormValue("tipo_componente"), r.FormValue("tipo")
	if !db.TipoComponente(comp).Valid() || !db.TipoDocumento(tipo).Valid() {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Errore: "tipo di componente o di documento non valido"})
		return
	}
	if tipo == string(db.TipoDocumentoDaDeterminare) {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno",
			Errore: "«da determinare» non è un documento che si possa pretendere: è ciò che si sa di un file prima di averlo aperto"})
		return
	}
	// La riga e la sua fonte attesa sono una cosa sola: una transazione, e la fonte che non si scrive
	// non lascia in anagrafica una riga diversa da quella chiesta.
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		f, err := q.InsertFabbisogno(ctx, db.InsertFabbisognoParams{
			ClienteID: uuid.NullUUID{UUID: id, Valid: true}, TipoComponente: db.TipoComponente(comp), Tipo: db.TipoDocumento(tipo),
			Bloccante: r.FormValue("bloccante") == "1"})
		if err != nil {
			return err
		}
		if fonte := r.FormValue("fonte_attesa"); fonte != "" && db.FonteFabbisogno(fonte).Valid() {
			return q.SetFonteFabbisogno(ctx, db.SetFonteFabbisognoParams{
				FabbisognoID: f.FabbisognoID, ClienteID: uuid.NullUUID{UUID: id, Valid: true},
				Fonte: db.NullFonteFabbisogno{FonteFabbisogno: db.FonteFabbisogno(fonte), Valid: true}})
		}
		return nil
	})
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Errore: err.Error()})
		return
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno",
		Fatto: "Riga aggiunta. Per «" + comp + "» valgono ora SOLO le righe di questo cliente: i predefiniti non si sommano."})
}

func (s *Server) eliminaFabbisogno(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	q := db.New(s.Pool)
	fid, err := uuid.Parse(r.FormValue("fabbisogno_id"))
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Errore: "riga non valida"})
		return
	}
	n, err := q.EliminaFabbisogno(r.Context(), db.EliminaFabbisognoParams{
		FabbisognoID: fid, ClienteID: uuid.NullUUID{UUID: id, Valid: true}})
	switch {
	case err != nil:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Errore: err.Error()})
	case n == 0:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno",
			Errore: "quella riga è un predefinito, valido per tutti i clienti: non si toglie da qui"})
	default:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Fatto: "Riga tolta."})
	}
}

// clienteDaRotta legge l'id dalla rotta e carica il cliente, o risponde e restituisce false.
func (s *Server) clienteDaRotta(w http.ResponseWriter, r *http.Request) (uuid.UUID, db.Cliente, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return uuid.Nil, db.Cliente{}, false
	}
	c, err := db.New(s.Pool).GetCliente(r.Context(), id)
	if err != nil {
		http.Error(w, "cliente non trovato", 404)
		return uuid.Nil, db.Cliente{}, false
	}
	return id, c, true
}

// ---------------------------------------------------------------- tendine del form

// TipiComponente, TipiDocumento e FontiAttesa alimentano le tendine. Vengono dagli enum generati da
// sqlc, quindi un valore aggiunto da una migrazione compare nella schermata senza che nessuno debba
// ricordarsi di aggiornare anche l'HTML.
func (d anagraficaDati) TipiComponente() []db.TipoComponente { return db.AllTipoComponenteValues() }

func (d anagraficaDati) TipiDocumento() []db.TipoDocumento {
	var out []db.TipoDocumento
	for _, t := range db.AllTipoDocumentoValues() {
		switch t {
		// non sono documenti che si possano pretendere da un cliente: «da determinare» è ciò che si
		// sa di un file prima di aprirlo, e gli altri tre sono categorie di scarto
		case db.TipoDocumentoDaDeterminare, db.TipoDocumentoRumore, db.TipoDocumentoAltro, db.TipoDocumentoCorrispondenza:
			continue
		}
		out = append(out, t)
	}
	return out
}

func (d anagraficaDati) FontiAttesa() []db.FonteFabbisogno { return db.AllFonteFabbisognoValues() }
