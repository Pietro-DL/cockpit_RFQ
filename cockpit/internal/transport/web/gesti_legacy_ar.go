package web

// C-6b, corsia file: i gesti atomici delle pagine legacy e le associazioni registrate (AR) file → componente.
//
// Le regole (direttiva 39 §6 [U]; D-AR03 D, messaggio 38 [U]; NOMI (f) e (i) [T-39 dell'orchestratore]):
//   - «RFQ formato 3» e' una sola funzione del core (associazioni.RFQFormato3: almeno una bozza o una conferma formato 3
//     nella storia della RFQ; l'abbandono della bozza non la riporta a legacy);
//   - nelle RFQ interamente legacy un gesto atomico passa dal servizio unico con l'origine gesto_legacy
//     (associazioni.RegistraLegacy, SostituisciLegacy; la chiusura con sgancio_legacy, scarto_legacy, file_generale):
//     mai piu' una scrittura diretta della copia documento_proposta.componente_id;
//   - nelle RFQ formato 3 un gesto atomico NON scrive AR ne' la copia: con una bozza aperta diventa un salvataggio R2
//     costruito dal server sulla versione corrente, con il lucchetto e la versione attesa di R2 (distintabozza.SalvaFormato3,
//     la stessa entrata della pagina nuova, che al salvataggio dopo riceve 409 BOZZA_VERSIONE e si riallinea); senza bozza
//     aperta rifiuta e invita al percorso di revisione, senza aprire la bozza da solo (D-AR03 D punto 5; lettura d1 del
//     messaggio 38);
//   - nessuno STEP sorgente del finito e' trattato da gesto_legacy (direttiva 39 §6 [U]): uno STEP assegnato a un
//     prodotto finito e' il legame della sorgente (D-AR02 A [U]); nella RFQ legacy passa da associazioni.LegaSorgente
//     (fonte_step_evento «caricamento» e la copia, nessuna AR), nella RFQ formato 3 rimanda a S8.
//
// Scelte [T-39 corsia file], scritte nel resoconto:
//   - nel formato 3 il gesto lavora una proposta alla volta e non si mescola con i documenti nella stessa richiesta (due
//     transazioni diverse lascerebbero uno stato a meta');
//   - la bozza e' quella aperta che ha il componente fra i suoi nodi (componente_id, non escluso); in nessuna o in piu' di
//     una il gesto rifiuta e rimanda alla Distinta, che sceglie il nodo;
//   - «Togli» nel formato 3: la PB della bozza esce (file_svincolato, motivo utente); senza PB e con una AR attiva lo
//     svincolo si prepara nella bozza che ha il componente (scollega_documento {allegato_id, componente_id}) e si applica
//     alla conferma; una copia senza AR (pre-assegnazione legacy) non si tocca: si converte alla presa in carico
//     (D-AR14 A [T-39]);
//   - nel formato 3 la PB non porta il codice del componente: correggi_codice non serve e non si chiede (i controlli
//     d'identita' sono di R4/R5);
//   - la provenienza della PB nuova non la mette questo gesto: la decide il server di R2 (R39-24, provenienza_dal_server).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermadoc"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// I testi dei rifiuti nel formato 3.
const (
	testoFormato3        = "la RFQ è gestita con la Distinta nuova (formato 3)"
	testoSenzaBozza      = testoFormato3 + ": un'associazione si prepara nella bozza del prodotto e si registra con la conferma della BOM. Nessuna bozza è aperta: apri la revisione del prodotto dalla Distinta e ripeti il gesto. Niente è cambiato"
	testoStepAS8         = testoFormato3 + ": lo STEP di un prodotto finito è la sua sorgente, non un'associazione. Si conferma dalla Sorgente («Conferma la sorgente», S8). Niente è cambiato"
	testoUnaAllaVolta    = testoFormato3 + ": il gesto cambia la bozza un file alla volta, senza documenti nella stessa richiesta. Niente è cambiato"
	testoPreassegnazione = testoFormato3 + ": il file ha una pre-assegnazione del vecchio «Assegna» senza associazione registrata; si converte alla presa in carico del prodotto, non da qui. Niente è cambiato"
)

// gestoNelFormato3: il gesto «Assegna» di una proposta in una RFQ formato 3, riconosciuto sotto il lucchetto della RFQ;
// chi lo riceve annulla la transazione e lo esegue come salvataggio R2 (assegnaNellaBozza).
type gestoNelFormato3 struct {
	allegato uuid.UUID
	nome     string
	comp     uuid.NullUUID
}

func (g *gestoNelFormato3) Error() string { return "gesto legacy in una RFQ formato 3: " + g.nome }

// attoLegacy: l'atto di un gesto delle pagine legacy (che non hanno una chiave d'idempotenza: ogni gesto e' un atto nuovo,
// e lo stesso stato ripetuto e' «invariato» per il servizio).
func attoLegacy(thread, utente uuid.UUID, nota string) associazioni.Atto {
	return associazioni.Atto{Thread: thread, Autore: utente, Chiave: uuid.New(), Nota: nota}
}

// stepDelFinito: uno STEP assegnato a un prodotto finito e' il legame della sorgente, mai una AR (direttiva 39 §5, §6 [U]).
func stepDelFinito(a db.Allegato, c *db.Componente) bool {
	return c != nil && c.Tipo == db.TipoComponenteFinito && confermadoc.FileStep(a.Estensione.String, a.NomeFile)
}

// rifiutoAR: un rifiuto del servizio unico detto all'operatore, con il nome del file; gli altri errori passano.
func rifiutoAR(nome string, err error) error {
	if err == nil {
		return nil
	}
	var r *associazioni.Rifiuto
	if !errors.As(err, &r) {
		return err
	}
	var testo string
	switch r.Codice {
	case associazioni.CodiceGiaAssociataAltrove:
		testo = "è già associato a un altro componente: lo spostamento si fa con un gesto esplicito"
	case associazioni.CodiceDocumentoConfermatoAltrui:
		testo = "lo stesso contenuto è già un documento confermato di un altro componente"
	case associazioni.CodiceComponenteArchiviato:
		testo = "il componente è archiviato: prima lo si ripristina"
	case associazioni.CodiceComponenteNonDellaRfq:
		testo = "il componente non è di questa RFQ"
	case associazioni.CodiceRFQFormato3:
		testo = testoFormato3 + ": il gesto non registra associazioni. Niente è cambiato"
	case associazioni.CodiceBomCongelata:
		testo = "la BOM è congelata: si modifica solo aprendo una revisione"
	case associazioni.CodiceVincolo:
		testo = "il database ha rifiutato l'associazione (" + r.Messaggio + ")"
	default:
		testo = r.Messaggio
	}
	return rifiuto(nome + ": " + testo)
}

// ------------------------------------------------------------------ RFQ interamente legacy: il servizio unico

// assegnaPropostaDalServizio e' «Assegna» (o «Togli», c nil) di una proposta aperta in una RFQ interamente legacy: le
// stesse regole di sempre (la proposta aperta, il codice del componente con correggi_codice), ma la scrittura passa dal
// servizio unico (D-AR03 D punto 6 [U]): una AR gesto_legacy con il suo evento e la copia, la sostituzione esplicita di
// una AR su un altro componente, la copia senza AR su un altro componente tolta prima con un evento (esito copia_tolta).
// Lo STEP di un prodotto finito e' il legame della sorgente (LegaSorgente), mai una AR. Chi chiama tiene la RFQ FOR UPDATE
// (preparaGesto) e la proposta (BloccaProposta): l'ordine RFQ → proposta → AR del servizio.
func assegnaPropostaDalServizio(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, p db.DocumentoProposta, a db.Allegato,
	nome string, c *db.Componente, correggi bool) error {
	if p.Stato != db.StatoPropostaAperta {
		return rifiuto(nome + ": la proposta è già decisa: si assegna il documento, non la proposta")
	}
	if c != nil {
		if _, err := codiceDaComponente(p.Codice.String, *c, correggi); err != nil {
			return rifiuto(nome + ": " + err.Error())
		}
	}
	sgancia := func() error {
		_, err := associazioni.Scollega(ctx, q, attoLegacy(thread, utente, ""), associazioni.Scollegamento{Allegato: a.AllegatoID,
			Motivo: associazioni.MotivoSgancioLegacy})
		return rifiutoAR(nome, err)
	}
	if c == nil {
		return sgancia()
	}
	att, err := associazioni.Attiva(ctx, q, thread, a.AllegatoID)
	if err != nil {
		return err
	}
	copiaAltrove := p.ComponenteID.Valid && p.ComponenteID.UUID != c.ComponenteID
	if stepDelFinito(a, c) {
		if att == nil && p.ComponenteID.Valid && !copiaAltrove {
			return nil // gia' legato a questo prodotto
		}
		if att != nil || copiaAltrove {
			if err := sgancia(); err != nil {
				return err
			}
		}
		_, err := associazioni.LegaSorgente(ctx, q, attoLegacy(thread, utente, ""), associazioni.LegameSorgente{Allegato: a.AllegatoID,
			Prodotto: c.ComponenteID})
		return rifiutoAR(nome, err)
	}
	switch {
	case att != nil && att.Componente != c.ComponenteID:
		_, err = associazioni.SostituisciLegacy(ctx, q, attoLegacy(thread, utente, ""), a.AllegatoID, c.ComponenteID)
	case att == nil && copiaAltrove:
		if err := sgancia(); err != nil {
			return err
		}
		_, err = associazioni.RegistraLegacy(ctx, q, attoLegacy(thread, utente, ""), a.AllegatoID, c.ComponenteID)
	default:
		_, err = associazioni.RegistraLegacy(ctx, q, attoLegacy(thread, utente, ""), a.AllegatoID, c.ComponenteID)
	}
	return rifiutoAR(nome, err)
}

// chiusuraLegacy: la chiusura di un gesto legacy che toglie il file dal suo componente prima di deciderlo (il vecchio
// «Scarta il file», motivo scarto_legacy; «Documentazione generale», motivo file_generale). Nella RFQ interamente legacy
// passa dal servizio (la AR chiusa, o la copia senza AR tolta, con l'evento), prima della decisione sulla proposta
// (INC-G10). Nella RFQ formato 3 non chiude niente: un file con una AR, una copia o una PB in una bozza aperta si rifiuta
// (rimando del gesto composto della Sorgente per lo scarto, D-ASS-03 [U]; per «Documentazione generale» D-AR06 A
// [T-39]); senza legami il gesto va avanti senza toccare AR e copia. Chi chiama tiene la RFQ e la proposta.
func chiusuraLegacy(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, p db.DocumentoProposta, a db.Allegato, motivo, rimando string) error {
	f3, err := associazioni.RFQFormato3(ctx, q, thread)
	if err != nil {
		return err
	}
	if !f3 {
		_, err := associazioni.Scollega(ctx, q, attoLegacy(thread, utente, ""), associazioni.Scollegamento{Allegato: a.AllegatoID, Motivo: motivo})
		return rifiutoAR(a.NomeFile, err)
	}
	att, err := associazioni.Attiva(ctx, q, thread, a.AllegatoID)
	if err != nil {
		return err
	}
	if att != nil {
		cod := att.Componente.String()
		if c, err := q.GetComponente(ctx, att.Componente); err == nil {
			cod = c.Codice
		}
		return rifiuto(fmt.Sprintf("%s: %s e il file ha un'associazione registrata con %s: %s. Niente è cambiato", a.NomeFile, testoFormato3, cod, rimando))
	}
	if p.Stato == db.StatoPropostaAperta && p.ComponenteID.Valid {
		return rifiuto(a.NomeFile + ": " + testoPreassegnazione)
	}
	if tipo, err := citatoDaUnaBozza(ctx, q, thread, a.AllegatoID); err != nil {
		return err
	} else if tipo != "" {
		return rifiuto(fmt.Sprintf("%s: %s e il file è nella bozza aperta (%s): %s. Niente è cambiato", a.NomeFile, testoFormato3, tipo, rimando))
	}
	return nil
}

// citatoDaUnaBozza: «preassociazione» o «svincolo» se una bozza aperta della RFQ cita il file cosi', altrimenti "".
func citatoDaUnaBozza(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) (string, error) {
	righe, err := q.ListDipendenzeBozzaPerFile(ctx, thread)
	if err != nil {
		return "", err
	}
	for _, r := range righe {
		if r.AllegatoID == allegato && (r.Tipo == "preassociazione" || r.Tipo == "svincolo") {
			if r.Tipo == "svincolo" {
				return "svincolo di un'associazione", nil
			}
			return "preassociazione a un nodo", nil
		}
	}
	return "", nil
}

// ------------------------------------------------------------------ RFQ formato 3: il salvataggio R2 del server

// bozzaAperta: una bozza formato 3 aperta alla versione corrente.
type bozzaAperta struct {
	id, prodotto uuid.UUID
	versione     int
	s            distintabozza.Snapshot3
}

// assegnaNellaBozza esegue «Assegna» (comp valido) o «Togli» (comp nullo) di un file in una RFQ formato 3 come un
// salvataggio R2 della bozza aperta (commento di testa): legge le bozze senza lucchetto, costruisce la versione nuova con
// le azioni del vocabolario 2 e la manda a distintabozza.SalvaFormato3, che prende il lucchetto della RFQ, ricontrolla la
// versione attesa (409 BOZZA_VERSIONE se nel frattempo la bozza e' cambiata) e rifa' tutte le verifiche di R2. Non scrive
// AR, copia, componenti ne' documenti.
func (s *Server) assegnaNellaBozza(ctx context.Context, thread, utente uuid.UUID, g gestoNelFormato3) (string, error) {
	q := db.New(s.Pool)
	bb, err := q.ListBozzeAperteDellaRfq(ctx, thread)
	if err != nil {
		return "", err
	}
	var aperte []bozzaAperta
	for _, b := range bb {
		if int(b.Formato) != distintabozza.FormatoSnapshot3 {
			continue
		}
		letto, err := distintabozza.RileggiVersione(int(b.Formato), b.Snapshot, b.Impronta)
		if err != nil {
			return "", err
		}
		if letto.Formato3 == nil {
			return "", fmt.Errorf("la bozza %s non e' del formato 3", b.BozzaID)
		}
		aperte = append(aperte, bozzaAperta{id: b.BozzaID, prodotto: b.ProdottoID, versione: int(b.Versione), s: *letto.Formato3})
	}
	if len(aperte) == 0 {
		return "", rifiuto(g.nome + ": " + testoSenzaBozza)
	}
	gestoID := uuid.New()
	if g.comp.Valid {
		return s.preassociaNellaBozza(ctx, q, thread, utente, g, aperte, gestoID)
	}
	return s.togliNellaBozza(ctx, q, thread, utente, g, aperte, gestoID)
}

// nodiDelComponente: le bozze aperte (indice) e i nodi (chiave) che hanno il componente, non esclusi.
func nodiDelComponente(aperte []bozzaAperta, comp uuid.UUID) (bozze []int, chiavi []string) {
	for i, b := range aperte {
		var kk []string
		for _, n := range b.s.Nodi {
			if n.ComponenteID != nil && *n.ComponenteID == comp && n.Op != distintabozza.OpEsclude {
				kk = append(kk, n.ChiaveBozza)
			}
		}
		if len(kk) > 0 {
			sort.Strings(kk)
			bozze, chiavi = append(bozze, i), append(chiavi, kk[0])
		}
	}
	return bozze, chiavi
}

func (s *Server) preassociaNellaBozza(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, g gestoNelFormato3, aperte []bozzaAperta,
	gestoID uuid.UUID) (string, error) {
	comp, err := q.GetComponente(ctx, g.comp.UUID)
	if err != nil {
		return "", err
	}
	bozze, chiavi := nodiDelComponente(aperte, comp.ComponenteID)
	switch {
	case len(bozze) == 0:
		return "", rifiuto(fmt.Sprintf("%s: %s: il componente %s non è in nessuna bozza aperta; aggiungilo alla bozza del suo prodotto dalla Distinta, poi ripeti il gesto. Niente è cambiato",
			g.nome, testoFormato3, comp.Codice))
	case len(bozze) > 1:
		return "", rifiuto(fmt.Sprintf("%s: %s: il componente %s è in più bozze aperte; scegli la bozza dalla Distinta. Niente è cambiato",
			g.nome, testoFormato3, comp.Codice))
	}
	b, chiave := aperte[bozze[0]], chiavi[0]
	nuovo := b.s
	nuovo.DocumentiPreassociati = nil
	var azioni []distintabozza.Azione
	for _, p := range b.s.DocumentiPreassociati {
		if p.AllegatoID != g.allegato {
			nuovo.DocumentiPreassociati = append(nuovo.DocumentiPreassociati, p)
			continue
		}
		if p.ChiaveBozza == chiave {
			return fmt.Sprintf("Niente è cambiato: %s è già preassociato a %s nella bozza; l'associazione si registra con la conferma della BOM.",
				g.nome, comp.Codice), nil
		}
		azioni = append(azioni, azioneSvincolo(g.allegato, p.ChiaveBozza, gestoID))
	}
	azioni = append(azioni, distintabozza.Azione{Azione: distintabozza.AzFilePreassociato, Origine: "utente",
		Dopo: jsonDi(map[string]any{"allegato_id": g.allegato, "chiave_bozza": chiave, "azione_id": gestoID})})
	nuovo.DocumentiPreassociati = append(nuovo.DocumentiPreassociati, distintabozza.Preassociazione3{AllegatoID: g.allegato, ChiaveBozza: chiave})
	v, err := s.salvaR2DelServer(ctx, thread, utente, b, nuovo, azioni)
	if err != nil {
		return "", erroreR2(g.nome, err)
	}
	return fmt.Sprintf("%s preassociato a %s nella bozza di %s (versione %d): l'associazione si registra con la conferma della BOM.",
		g.nome, comp.Codice, s.codiceDi(ctx, q, b.prodotto), v), nil
}

func (s *Server) togliNellaBozza(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, g gestoNelFormato3, aperte []bozzaAperta,
	gestoID uuid.UUID) (string, error) {
	var conPB []int
	for i, b := range aperte {
		for _, p := range b.s.DocumentiPreassociati {
			if p.AllegatoID == g.allegato {
				conPB = append(conPB, i)
				break
			}
		}
	}
	if len(conPB) > 1 {
		return "", rifiuto(fmt.Sprintf("%s: %s: il file è preassociato in più bozze aperte; toglilo dalla Distinta, bozza per bozza. Niente è cambiato", g.nome, testoFormato3))
	}
	if len(conPB) == 1 {
		b := aperte[conPB[0]]
		nuovo := b.s
		nuovo.DocumentiPreassociati = nil
		var azioni []distintabozza.Azione
		for _, p := range b.s.DocumentiPreassociati {
			if p.AllegatoID == g.allegato {
				azioni = append(azioni, azioneSvincolo(g.allegato, p.ChiaveBozza, gestoID))
				continue
			}
			nuovo.DocumentiPreassociati = append(nuovo.DocumentiPreassociati, p)
		}
		v, err := s.salvaR2DelServer(ctx, thread, utente, b, nuovo, azioni)
		if err != nil {
			return "", erroreR2(g.nome, err)
		}
		return fmt.Sprintf("%s tolto dalla bozza di %s (versione %d): la preassociazione non c'è più.", g.nome, s.codiceDi(ctx, q, b.prodotto), v), nil
	}
	att, err := associazioni.Attiva(ctx, q, thread, g.allegato)
	if err != nil {
		return "", err
	}
	if att == nil {
		if p, err := q.GetPropostaDocumentoDiAllegato(ctx, g.allegato); err == nil && p.Stato == db.StatoPropostaAperta && p.ComponenteID.Valid {
			return "", rifiuto(g.nome + ": " + testoPreassegnazione)
		}
		return fmt.Sprintf("Niente è cambiato: %s non è associato e non è nella bozza.", g.nome), nil
	}
	cod := s.codiceDi(ctx, q, att.Componente)
	bozze, _ := nodiDelComponente(aperte, att.Componente)
	if len(bozze) != 1 {
		return "", rifiuto(fmt.Sprintf("%s: %s e il file ha un'associazione registrata con %s: lo svincolo si prepara nella bozza del prodotto che ha %s, dalla Distinta. Niente è cambiato",
			g.nome, testoFormato3, cod, cod))
	}
	b := aperte[bozze[0]]
	for _, d := range b.s.Decisioni {
		if d.Tipo == distintabozza.DecisioneScollegaDocumento && d.AllegatoID != nil && *d.AllegatoID == g.allegato &&
			d.ComponenteID != nil && *d.ComponenteID == att.Componente {
			return fmt.Sprintf("Niente è cambiato: lo svincolo di %s da %s è già nella bozza e si applica con la conferma della BOM.", g.nome, cod), nil
		}
	}
	dec := distintabozza.Decisione3{ID: "d-svl-" + esadecimale(6), Tipo: distintabozza.DecisioneScollegaDocumento, AllegatoID: &g.allegato,
		ComponenteID: &att.Componente, Motivo: "«Togli» dalla pagina legacy"}
	nuovo := b.s
	nuovo.Decisioni = append(append([]distintabozza.Decisione3(nil), b.s.Decisioni...), dec)
	azioni := []distintabozza.Azione{{Azione: distintabozza.AzDecisioneAggiunta, Origine: "utente", Dopo: jsonDi(dec)}}
	v, err := s.salvaR2DelServer(ctx, thread, utente, b, nuovo, azioni)
	if err != nil {
		return "", erroreR2(g.nome, err)
	}
	return fmt.Sprintf("Svincolo di %s da %s preparato nella bozza di %s (versione %d): l'associazione registrata si chiude con la conferma della BOM.",
		g.nome, cod, s.codiceDi(ctx, q, b.prodotto), v), nil
}

// azioneSvincolo: file_svincolato di una PB (motivo utente: «Togli la preassociazione», non va nel vassoio).
func azioneSvincolo(allegato uuid.UUID, chiave string, gestoID uuid.UUID) distintabozza.Azione {
	return distintabozza.Azione{Azione: distintabozza.AzFileSvincolato, Origine: "utente",
		Prima: jsonDi(map[string]any{"allegato_id": allegato, "chiave_bozza": chiave}),
		Dopo:  jsonDi(map[string]any{"allegato_id": allegato, "motivo": distintabozza.SvincoloUtente, "azione_id": gestoID})}
}

// salvaR2DelServer manda la versione nuova a R2 come la manderebbe la pagina (LeggiRichiestaR2 → SalvaFormato3), con una
// chiave di salvataggio nuova e la versione letta come versione attesa. Restituisce la versione salvata.
func (s *Server) salvaR2DelServer(ctx context.Context, thread, utente uuid.UUID, b bozzaAperta, nuovo distintabozza.Snapshot3,
	azioni []distintabozza.Azione) (int, error) {
	can, err := nuovo.Canonico()
	if err != nil {
		return 0, err
	}
	raw, err := json.Marshal(map[string]any{"bozza_id": b.id, "versione_attesa": b.versione, "prodotto_id": b.prodotto, "vocabolario": distintabozza.VocabolarioFormato3,
		"chiave_salvataggio": uuid.New(), "snapshot": json.RawMessage(can), "azioni": azioni})
	if err != nil {
		return 0, err
	}
	req, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		return 0, err
	}
	sv := req.Salvataggio
	sv.ThreadID, sv.UtenteID = thread, utente
	out, err := distintabozza.SalvaFormato3(ctx, s.Pool, sv)
	if err != nil {
		return 0, err
	}
	return out.Versione, nil
}

// erroreR2: un rifiuto di R2 detto all'operatore.
func erroreR2(nome string, err error) error {
	var e *distintabozza.Errore
	if !errors.As(err, &e) {
		return err
	}
	if e.Codice == distintabozza.CodiceBozzaVersione || e.Codice == distintabozza.CodiceBaseSuperata {
		return rifiuto(nome + ": la bozza è cambiata nel frattempo (un collega o la pagina della Distinta): rileggi e riprova. Niente è cambiato")
	}
	msg := strings.TrimSpace(e.Messaggio)
	if e.Motivo != "" {
		msg += " (" + e.Motivo + ")"
	}
	return rifiuto(nome + ": la bozza non accetta il gesto: " + msg + ". Niente è cambiato")
}

func (s *Server) codiceDi(ctx context.Context, q *db.Queries, comp uuid.UUID) string {
	if c, err := q.GetComponente(ctx, comp); err == nil {
		return c.Codice
	}
	return comp.String()
}

func jsonDi(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func esadecimale(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
