package web

// Il Fascicolo, B8.3: la conferma di un allegato non decide piu' la struttura tecnica.
//
// Fino al B8.2 confermare un disegno con un codice creava il componente di quel codice, e lo faceva
// «finito» o «sciolto» a seconda che il codice fosse fra quelli della richiesta: la gerarchia la
// decideva un clic dato per tutt'altro motivo. Adesso un documento, o una proposta non ancora
// confermata, si aggancia a un componente GIA' ESISTENTE della stessa RFQ con un gesto suo, e il
// codice del documento segue quello del componente (addendum A1.4, decisione D18).
//
// Chi garantisce le regole e' il database, non questo file: la FK (thread, componente, codice)
// rifiuta un componente di un'altra RFQ e un codice diverso da quello del componente, e il CHECK
// sui tipi tecnici rifiuta un disegno senza codice. Qui si decide CHE COSA scrivere, e si dice
// all'operatore perche' qualcosa non si puo' fare prima che ci sbatta contro il vincolo.
//
// La schermata del Fascicolo arriva con B8.7: fino ad allora queste rotte rispondono con la pagina
// della RFQ e un avviso.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// rifiuto e' un «no» per l'operatore: la transazione si annulla per intero e l'avviso dice il motivo.
type rifiuto string

func (r rifiuto) Error() string { return string(r) }

// stessoCodice confronta due codici come li confronta l'identita' del componente, upper(codice).
func stessoCodice(a, b string) bool {
	return strings.ToUpper(strings.TrimSpace(a)) == strings.ToUpper(strings.TrimSpace(b))
}

// codiceDaComponente e' il codice che un documento o una proposta prende quando si aggancia a c.
// attuale e' quello che hanno adesso ("" = nessuno).
//
// Lo stesso codice con altre maiuscole non e' una correzione: si copia la stringa del componente
// (A1.4, regola 8), perche' la FK la vuole identica. Un codice DIVERSO lo diventa solo con il gesto
// esplicito «assegna e correggi il codice» (regola 1): un disegno che dice 52920517 sotto il
// componente 52922757 e' un errore di qualcuno, e non si ripara in silenzio.
func codiceDaComponente(attuale string, c db.Componente, correggi bool) (string, error) {
	if strings.TrimSpace(attuale) != "" && !stessoCodice(attuale, c.Codice) && !correggi {
		return "", rifiuto(fmt.Sprintf("codice diverso: %s contro %s. Per assegnarlo comunque va corretto il codice del file (correggi_codice)",
			strings.TrimSpace(attuale), c.Codice))
	}
	return c.Codice, nil
}

// percorsi calcola il percorso di un documento come lo calcola la conferma: la sottocartella del
// tipo e la regola del cliente sulla cartella per codice. PathDocumento resta l'unico posto che
// decide un path_relativo.
type percorsi struct {
	perCodice bool
	layout    map[db.TipoDocumento]db.CartellaDocumento
}

func nuoviPercorsi(ctx context.Context, q *db.Queries, thread uuid.UUID) (*percorsi, error) {
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	cl, err := q.GetCliente(ctx, t.ClienteID)
	if err != nil {
		return nil, err
	}
	return &percorsi{perCodice: regolaBool(cl.Regole, "cartella_per_codice", true), layout: map[db.TipoDocumento]db.CartellaDocumento{}}, nil
}

// di e' il percorso che il documento d avrebbe con il codice dato: la cartella che la conferma gli
// avrebbe dato con quel codice, e il NOME CHE HA GIA'. Una correzione di codice sposta, non rinomina.
func (p *percorsi) di(ctx context.Context, q *db.Queries, d db.Documento, codice string) (string, error) {
	l, ok := p.layout[d.Tipo]
	if !ok {
		var err error
		if l, err = q.GetCartellaDocumento(ctx, d.Tipo); err != nil {
			return "", fmt.Errorf("layout per %s: %w", d.Tipo, err)
		}
		p.layout[d.Tipo] = l
	}
	cartella, err := documenti.CartellaDocumento(documenti.LayoutDocumento{Sottocartella: l.Sottocartella, PerCodice: l.PerCodice}, p.perCodice, codice)
	if err != nil {
		return "", err
	}
	return documenti.NellaCartella(cartella, documenti.NomeNelPercorso(d.PathRelativo)), nil
}

// ripercorri scrive il percorso che il documento d (bloccato dal chiamante) deve avere con il codice
// nuovo, se e' diverso da quello che ha. Sono le regole 2–4 di A1.4:
//
//   - in coda, o in errore: il file sul NAS non c'e' ancora, e il percorso nuovo si scrive PRIMA
//     della copia. La copia in attesa si blocca (SELECT … FOR UPDATE sul suo job): finche' questa
//     transazione e' aperta nessuno la prende, e quando parte legge il percorso nuovo. Se e' gia'
//     partita non si fa una corsa con lei: si rinuncia, e si riprova quando ha finito.
//   - scritto: il file e' gia' sul NAS in un'altra cartella. Spostarlo e' B8.8 (sposta_nas): fino ad
//     allora si rifiuta, e DB e NAS continuano a dire la stessa cosa.
func ripercorri(ctx context.Context, q *db.Queries, perc *percorsi, d db.Documento, codice string) error {
	nuovo, err := perc.di(ctx, q, d, codice)
	if err != nil {
		return rifiuto(d.NomeFile + ": " + err.Error())
	}
	if nuovo == d.PathRelativo {
		return nil
	}
	switch d.StatoNas {
	case db.StatoNasScritto:
		return rifiuto(fmt.Sprintf("%s: il file è già sul NAS in un'altra cartella (%s): la correzione arriva con lo spostamento",
			d.NomeFile, d.PathRelativo))
	case db.StatoNasInCoda, db.StatoNasErrore:
		j, err := q.BloccaCopiaPendente(ctx, pgtype.Text{String: coda.ChiaveCopia(d.DocumentoID), Valid: true})
		switch {
		case err == nil && j.Stato == db.StatoJobInCorso:
			return rifiuto(d.NomeFile + ": copia in corso, riprova fra poco")
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		n, err := q.SetPathDocumentoInCoda(ctx, db.SetPathDocumentoInCodaParams{DocumentoID: d.DocumentoID, PathRelativo: nuovo})
		if err != nil {
			return err
		}
		if n == 0 {
			return rifiuto(d.NomeFile + ": lo stato sul NAS è cambiato nel frattempo, riprova")
		}
		return nil
	}
	return fmt.Errorf("%s: stato NAS sconosciuto %q", d.NomeFile, d.StatoNas)
}

// assegnaDocumento aggancia il documento d (bloccato dal chiamante) al componente c, oppure lo
// sgancia (c nil). Sganciare non tocca ne' il codice ne' il percorso (regola 6): il documento torna
// «della RFQ» con il suo codice. Agganciare a un componente dello STESSO codice non tocca il NAS: il
// percorso dipende dal codice, e il codice non cambia.
func assegnaDocumento(ctx context.Context, q *db.Queries, perc *percorsi, d db.Documento, c *db.Componente, correggi bool) error {
	arg := db.SetComponenteDocumentoParams{DocumentoID: d.DocumentoID, Codice: d.Codice}
	if c != nil {
		codice, err := codiceDaComponente(d.Codice.String, *c, correggi)
		if err != nil {
			return rifiuto(d.NomeFile + ": " + err.Error())
		}
		if !stessoCodice(d.Codice.String, codice) {
			if err := ripercorri(ctx, q, perc, d, codice); err != nil {
				return err
			}
		}
		arg.ComponenteID = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
		arg.Codice = pgtype.Text{String: codice, Valid: true} // la stringa del componente, identica
	}
	_, err := q.SetComponenteDocumento(ctx, arg)
	return err
}

// assegnaProposta e' la stessa cosa per un file non ancora confermato: la proposta prende il codice del
// componente (A2.2) e alla conferma lo passa al documento. Niente NAS: una proposta non ha percorso.
func assegnaProposta(ctx context.Context, q *db.Queries, p db.DocumentoProposta, nome string, c *db.Componente, correggi bool) error {
	if p.Stato != db.StatoPropostaAperta {
		return rifiuto(nome + ": la proposta è già decisa: si assegna il documento, non la proposta")
	}
	arg := db.SetComponentePropostaParams{PropostaID: p.PropostaID, Codice: p.Codice}
	if c != nil {
		codice, err := codiceDaComponente(p.Codice.String, *c, correggi)
		if err != nil {
			return rifiuto(nome + ": " + err.Error())
		}
		arg.ComponenteID = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
		arg.Codice = pgtype.Text{String: codice, Valid: true}
	}
	n, err := q.SetComponenteProposta(ctx, arg)
	if err != nil {
		return err
	}
	if n == 0 {
		return rifiuto(nome + ": la proposta è stata decisa nel frattempo")
	}
	return nil
}

// assegnaAlComponente e' il gesto «assegna»: documenti e proposte della RFQ thread sotto il componente
// comp, o sganciati se comp non e' valido. Tutto o niente: se uno solo dei file non si puo', non cambia
// niente e il rifiuto dice quale e perche'. E' una decisione su un gruppo di file per un pezzo, e fatta
// a meta' non vorrebbe dire niente.
func assegnaAlComponente(ctx context.Context, q *db.Queries, thread uuid.UUID, comp uuid.NullUUID, docs, props []uuid.UUID, correggi bool) (string, error) {
	var c *db.Componente
	if comp.Valid {
		x, err := q.GetComponente(ctx, comp.UUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", rifiuto("il componente scelto non esiste")
		}
		if err != nil {
			return "", err
		}
		// Il controllo che dice all'operatore che cosa non va. Chi IMPEDISCE l'aggancio e' la FK
		// (thread, componente, codice): senza questa riga la UPDATE fallirebbe lo stesso.
		if x.ThreadID != thread {
			return "", rifiuto(fmt.Sprintf("il componente %s non è di questa RFQ", x.Codice))
		}
		c = &x
	}
	perc, err := nuoviPercorsi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	for _, id := range docs {
		d, err := q.BloccaDocumento(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.ThreadID != thread) {
			return "", rifiuto("un documento scelto non è di questa RFQ")
		}
		if err != nil {
			return "", err
		}
		if err := assegnaDocumento(ctx, q, perc, d, c, correggi); err != nil {
			return "", err
		}
	}
	for _, id := range props {
		p, err := q.BloccaProposta(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!p.ThreadID.Valid || p.ThreadID.UUID != thread)) {
			return "", rifiuto("una proposta scelta non è di questa RFQ")
		}
		if err != nil {
			return "", err
		}
		nome := "file"
		if a, err := q.GetAllegato(ctx, p.AllegatoID); err == nil {
			nome = a.NomeFile
		}
		if err := assegnaProposta(ctx, q, p, nome, c, correggi); err != nil {
			return "", err
		}
	}
	n := len(docs) + len(props)
	file := fmt.Sprintf("%d file", n)
	if c == nil {
		return file + " senza componente: codice e percorso invariati.", nil
	}
	return file + " assegnat" + plurale(n, "o", "i") + " al componente " + c.Codice + ".", nil
}

// correggiCodiceComponente corregge il codice del componente cid e lo porta su tutto quello che vi e'
// agganciato (A1.4, regola 5): i documenti, con il loro percorso finche' non sono sul NAS, e le
// proposte. Un solo documento gia' scritto sul NAS che dovrebbe cambiare cartella ferma tutto.
//
// La FK (thread, componente, codice) e' DEFERRABLE INITIALLY IMMEDIATE, e qui si rimanda al COMMIT:
// fra la UPDATE del componente e quelle dei documenti la FK sarebbe violata per costruzione. Al
// COMMIT PostgreSQL la ricontrolla per intero, e un documento dimenticato fa fallire la transazione.
func correggiCodiceComponente(ctx context.Context, tx pgx.Tx, q *db.Queries, thread, cid uuid.UUID, nuovo string) (string, error) {
	nuovo = strings.TrimSpace(nuovo)
	if nuovo == "" {
		return "", rifiuto("il codice non può essere vuoto")
	}
	if utf8.RuneCountInString(nuovo) > 60 {
		return "", rifiuto("il codice ha più di 60 caratteri")
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS fk_documento_componente, fk_proposta_componente DEFERRED`); err != nil {
		return "", err
	}
	c, err := q.BloccaComponente(ctx, cid)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ThreadID != thread) {
		return "", rifiuto("il componente non è di questa RFQ")
	}
	if err != nil {
		return "", err
	}
	if nuovo == c.Codice {
		return "Nessun cambiamento: il codice è già " + nuovo + ".", nil
	}
	if altro, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: thread, Upper: nuovo}); err == nil && altro.ComponenteID != c.ComponenteID {
		return "", rifiuto("in questa RFQ c'è già il componente " + altro.Codice)
	}
	if err := q.SetCodiceComponente(ctx, db.SetCodiceComponenteParams{ComponenteID: cid, Codice: nuovo}); err != nil {
		return "", err
	}
	perc, err := nuoviPercorsi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	docs, err := q.ListDocumentiComponente(ctx, uuid.NullUUID{UUID: cid, Valid: true})
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		if !stessoCodice(d.Codice.String, nuovo) {
			if err := ripercorri(ctx, q, perc, d, nuovo); err != nil {
				return "", err
			}
		}
		if _, err := q.SetComponenteDocumento(ctx, db.SetComponenteDocumentoParams{DocumentoID: d.DocumentoID,
			ComponenteID: uuid.NullUUID{UUID: cid, Valid: true}, Codice: pgtype.Text{String: nuovo, Valid: true}}); err != nil {
			return "", err
		}
	}
	np, err := q.SetCodiceProposteComponente(ctx, db.SetCodiceProposteComponenteParams{
		ComponenteID: uuid.NullUUID{UUID: cid, Valid: true}, Codice: pgtype.Text{String: nuovo, Valid: true}})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Codice corretto: %s → %s (%d document%s, %d propost%s).", c.Codice, nuovo,
		len(docs), plurale(len(docs), "o", "i"), np, plurale(int(np), "a", "e")), nil
}

// spiegaErrore traduce i vincoli del Fascicolo in parole per l'operatore. Arrivarci vuol dire che il
// controllo in Go non l'ha visto (una modifica concorrente, di solito): il vincolo ha fatto il suo
// lavoro, e l'avviso non deve essere un SQLSTATE.
func spiegaErrore(err error) string {
	var r rifiuto
	if errors.As(err, &r) {
		return string(r)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.ConstraintName {
		case "fk_documento_componente", "fk_proposta_componente":
			return "il componente è cambiato nel frattempo, o non è di questa RFQ: ricarica e riprova"
		case "ux_componente_thread_codice":
			return "in questa RFQ c'è già un componente con quel codice"
		case "ck_documento_tecnico_ha_codice", "ck_documento_agganciato_ha_codice", "ck_proposta_agganciata_ha_codice":
			return "un file agganciato a un componente, o un CAD/disegno/DXF, non può restare senza codice"
		}
	}
	return err.Error()
}

// uuidDalForm legge una lista di id dal form; uno non valido e' un errore del chiamante, non un «no».
func uuidDalForm(valori []string) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, v := range valori {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		id, err := uuid.Parse(v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// assegna: POST /thread/{id}/fascicolo/assegna
//
//	componente       il componente di destinazione; vuoto = sgancia
//	documento        uno o piu' documenti della RFQ
//	proposta         una o piu' proposte aperte della RFQ
//	correggi_codice  "1" = il codice dei file diventa quello del componente anche se era diverso
func (s *Server) assegna(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form non valido", 400)
		return
	}
	var comp uuid.NullUUID
	if v := strings.TrimSpace(r.FormValue("componente")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			http.Error(w, "componente non valido", 400)
			return
		}
		comp = uuid.NullUUID{UUID: id, Valid: true}
	}
	docs, err1 := uuidDalForm(r.Form["documento"])
	props, err2 := uuidDalForm(r.Form["proposta"])
	if err1 != nil || err2 != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if len(docs)+len(props) == 0 {
		s.threadFrammento(w, r, thread, "Nessun file scelto: niente da assegnare.")
		return
	}
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	msg, err := assegnaAlComponente(ctx, db.New(tx), thread, comp, docs, props, r.FormValue("correggi_codice") == "1")
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.threadFrammento(w, r, thread, "Assegnazione non riuscita, nessun file cambiato: "+spiegaErrore(err))
		return
	}
	s.threadFrammento(w, r, thread, msg)
}

// correggiCodice: POST /thread/{id}/fascicolo/componente/{cid}/codice, campo `codice`.
func (s *Server) correggiCodice(w http.ResponseWriter, r *http.Request) {
	thread, err1 := uuid.Parse(r.PathValue("id"))
	cid, err2 := uuid.Parse(r.PathValue("cid"))
	if err1 != nil || err2 != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	msg, err := correggiCodiceComponente(ctx, tx, db.New(tx), thread, cid, r.FormValue("codice"))
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.threadFrammento(w, r, thread, "Correzione non riuscita, niente è cambiato: "+spiegaErrore(err))
		return
	}
	s.threadFrammento(w, r, thread, msg)
}
