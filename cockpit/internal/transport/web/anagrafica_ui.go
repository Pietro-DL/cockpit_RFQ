package web

// L'ANAGRAFICA NUOVA (cockpit/_fasi/mockup_anagrafica.html): la scheda completa o da completare, le persone con
// le loro mail, il fabbisogno come tabella pezzo × documento con «Personalizza», la matrice «Chi fa cosa» dei
// fornitori, e il banco di prova accanto alla scheda, che prova anche le regole non ancora salvate.
//
// Niente qui scrive regole: il banco legge il form com'e' (regoleDalForm, la stessa lettura del salvataggio),
// e la regola non entra in anagrafica finche' non si preme «Salva regole».

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// puntoScheda e' una cosa che una scheda cliente completa ha, con la sezione dove si aggiunge.
type puntoScheda struct {
	Cosa, Sez string
	Ok        bool
}

// completezza: dominio, buyer, famiglia di codice che riconosce il suo esempio, riferimento della richiesta,
// giorni per rispondere. Un cliente che manca di qualcosa non e' sbagliato: si riconosce peggio, e lo si dice.
func completezza(regoleRaw []byte, nDomini, nBuyer int64) []puntoScheda {
	r, _ := regole.LeggiRegole(regoleRaw)
	fam := len(r.FamiglieCodice) > 0
	for _, f := range r.FamiglieCodice {
		re, err := regexp.Compile(f.Regex)
		if err != nil || f.Esempio == "" || !re.MatchString(f.Esempio) {
			fam = false
		}
	}
	return []puntoScheda{
		{"un dominio", "contatti", nDomini > 0},
		{"un buyer", "contatti", nBuyer > 0},
		{"una famiglia di codice", "riconoscimento", fam},
		{"il riferimento della richiesta", "riconoscimento", r.RiferimentoRFQ != nil && r.RiferimentoRFQ.Regex != ""},
		{"i giorni per rispondere", "riconoscimento", r.RispostaEntroGG > 0},
	}
}

func contaOk(pp []puntoScheda) int {
	n := 0
	for _, p := range pp {
		if p.Ok {
			n++
		}
	}
	return n
}

// Completezza della riga dell'elenco (template: .Completezza).
type rigaCliente struct{ db.ListClientiTuttiRow }

func (c rigaCliente) Completezza() []puntoScheda { return completezza(c.Regole, c.NDomini, c.NBuyer) }
func (c rigaCliente) Completa() bool             { return contaOk(c.Completezza()) == 5 }

// RigheClienti sono le righe dell'elenco con la loro completezza.
func (d anagraficaDati) RigheClienti() []rigaCliente {
	out := make([]rigaCliente, len(d.Clienti))
	for i, c := range d.Clienti {
		out[i] = rigaCliente{c}
	}
	return out
}

// DaCompletare conta i clienti attivi con la scheda incompleta.
func (d anagraficaDati) DaCompletare() int {
	n := 0
	for _, c := range d.RigheClienti() {
		if c.Attivo && !c.Completa() {
			n++
		}
	}
	return n
}

// Scheda e' la completezza del cliente scelto, con i numeri gia' letti.
func (d anagraficaDati) Scheda() []puntoScheda {
	if d.Scelto == nil {
		return nil
	}
	return completezza(d.Scelto.Regole, int64(len(d.Domini)), int64(len(d.Buyer)))
}
func (d anagraficaDati) SchedaOk() int { return contaOk(d.Scheda()) }

// RfqDelCliente e' quante RFQ ha il cliente scelto (dall'elenco, che le conta gia').
func (d anagraficaDati) RfqDelCliente() int64 {
	for _, c := range d.Clienti {
		if d.Scelto != nil && c.ClienteID == d.Scelto.ClienteID {
			return c.NRichieste
		}
	}
	return 0
}

// mailPersona: quante mail ha scritto una persona, e l'ultima.
type mailPersona struct {
	N      int32
	Ultima time.Time
}

// MailDi e' la riga di una persona (zero se non ha mai scritto).
func (d anagraficaDati) MailDi(id uuid.UUID) mailPersona { return d.Mail[id] }

// FamigliaOk dice se la famiglia riconosce il suo esempio: la spunta della scheda, letta come la legge il motore.
func famigliaOk(f regole.FamigliaCodice) bool {
	re, err := regexp.Compile(f.Regex)
	return err == nil && f.Esempio != "" && re.MatchString(f.Esempio)
}

// Celle del fabbisogno: il pezzo × il documento, con la riga che vale e se e' del cliente.
type cellaFabbisogno struct {
	Comp, Doc string
	Riga      *db.ListFabbisognoEffettivoRow
	ID        string // la riga propria, per toglierla
}

type rigaFabbisogno struct {
	Comp, Nome string
	Proprio    bool
	Celle      []cellaFabbisogno
}

var docFabbisogno = []struct{ Tipo, Nome string }{
	{"cad_3d", "3D"}, {"disegno_2d", "2D"}, {"sviluppo_dxf", "DXF"}, {"capitolato", "Capitolato"}, {"distinta_cliente", "Distinta cliente"},
}

// DocFabbisogno sono le colonne della tabella.
func (d anagraficaDati) DocFabbisogno() []struct{ Tipo, Nome string } { return docFabbisogno }

// Matrice e' il fabbisogno come tabella: una riga per tipo di pezzo, una colonna per documento.
func (d anagraficaDati) Matrice() []rigaFabbisogno {
	var out []rigaFabbisogno
	for _, t := range fascicolo.TipiComponente {
		r := rigaFabbisogno{Comp: string(t), Nome: nomeTipoPezzo(t)}
		for _, doc := range docFabbisogno {
			c := cellaFabbisogno{Comp: string(t), Doc: doc.Tipo}
			for i := range d.Fabbisogno {
				f := &d.Fabbisogno[i]
				if string(f.TipoComponente) == string(t) && string(f.Tipo) == doc.Tipo {
					c.Riga = f
					r.Proprio = r.Proprio || f.Proprio
				}
			}
			for _, p := range d.Propri {
				if string(p.TipoComponente) == string(t) && string(p.Tipo) == doc.Tipo {
					c.ID = p.FabbisognoID.String()
				}
			}
			r.Celle = append(r.Celle, c)
		}
		for _, f := range d.Fabbisogno {
			if string(f.TipoComponente) == string(t) && f.Proprio {
				r.Proprio = true
			}
		}
		out = append(out, r)
	}
	return out
}

func nomeTipoPezzo(t db.TipoComponente) string {
	switch t {
	case db.TipoComponenteFinito:
		return "Prodotto finito"
	case db.TipoComponenteSottoassieme:
		return "Assieme"
	case db.TipoComponenteSciolto:
		return "Particolare"
	case db.TipoComponenteCommerciale:
		return "Particolare commerciale"
	}
	return string(t)
}

// CapacitaDi sono le lavorazioni di un fornitore, per le tendine delle qualifiche (data-lav).
func (d anagraficaDati) CapacitaDi(id uuid.UUID) string { return strings.Join(d.Capacita[id], " ") }

// ---------------------------------------------------------------- scritture del fabbisogno

// fabbisognoTipo: «Personalizza» (copia i predefiniti di un tipo di pezzo) e «Torna ai predefiniti».
func (s *Server) fabbisognoTipo(copia bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, c, ok := s.clienteDaRotta(w, r)
		if !ok {
			return
		}
		comp := r.FormValue("tipo_componente")
		if !db.TipoComponente(comp).Valid() {
			s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Errore: "tipo di pezzo non valido"})
			return
		}
		q := db.New(s.Pool)
		var err error
		fatto := ""
		if copia {
			_, err = q.CopiaFabbisognoPredefinito(r.Context(), db.CopiaFabbisognoPredefinitoParams{ClienteID: id, TipoComponente: db.TipoComponente(comp)})
			fatto = "Predefiniti copiati: adesso le righe di «" + nomeTipoPezzo(db.TipoComponente(comp)) + "» sono di questo cliente e si cambiano senza perderne nessuna."
		} else {
			_, err = q.TogliFabbisognoTipo(r.Context(), db.TogliFabbisognoTipoParams{ClienteID: id, TipoComponente: db.TipoComponente(comp)})
			fatto = "Per «" + nomeTipoPezzo(db.TipoComponente(comp)) + "» tornano a valere i predefiniti."
		}
		if err != nil {
			s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Errore: err.Error()})
			return
		}
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "fabbisogno", Fatto: fatto})
	}
}

// ---------------------------------------------------------------- il banco di prova, accanto alla scheda

// regoleDellaProva sono le regole con cui provare: quelle del form come sono scritte adesso («dal_form»), oppure
// quelle salvate. Quelle del form passano dalla stessa lettura del salvataggio (LeggiRegole): una famiglia che
// non si legge resta fuori anche qui.
func regoleDellaProva(r *http.Request, c db.Cliente) (regole.Regole, bool) {
	if r.FormValue("dal_form") == "1" {
		if raw, err := json.Marshal(regoleDalForm(r)); err == nil {
			reg, _ := regole.LeggiRegole(raw)
			return reg, true
		}
	}
	reg, _ := regole.LeggiRegole(c.Regole)
	return reg, false
}

// esitoProva e' il frammento del banco: quello che l'Inbox proporrebbe per il testo incollato.
func (s *Server) esitoProva(w http.ResponseWriter, r *http.Request, c db.Cliente) {
	testo := r.FormValue("testo")
	oggetto, corpo := primaRigaEResto(testo)
	reg, dalForm := regoleDellaProva(r, c)
	in := classificazione.IngressoTriage{Oggetto: oggetto, Corpo: corpo, Direzione: string(db.DirezioneEntrata),
		ClienteNoto: true, Motore: classificazione.Compila(c.RagioneSociale, reg)}
	if n := strings.TrimSpace(r.FormValue("allegati")); n != "" {
		in.NomiAllegati = strings.Fields(n)
	}
	p := &provaDati{Testo: testo, Cliente: c.CartellaNas, Esito: classificazione.Riconosci(in, time.Now()), DalForm: dalForm, Regole: reg}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["anagrafica.html"].ExecuteTemplate(w, "ana_prova_esito", vista{Utente: utenteDa(r.Context()), Dati: p, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "ana_prova_esito", "err", err)
	}
}

// ---------------------------------------------------------------- fornitori: «Chi fa cosa»

type chiFaCosa struct {
	Lavorazioni []db.Lavorazione
	Fornitori   []db.ListFornitoriRow
	Fa, Qual    map[string]bool // fornitore|lavorazione
}

func (m *chiFaCosa) Si(f uuid.UUID, l string) bool          { return m.Fa[f.String()+"|"+l] }
func (m *chiFaCosa) Qualificato(f uuid.UUID, l string) bool { return m.Qual[f.String()+"|"+l] }

// NomeBreve e' la prima parola della ragione sociale, per le colonne strette.
func nomeBreve(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}
