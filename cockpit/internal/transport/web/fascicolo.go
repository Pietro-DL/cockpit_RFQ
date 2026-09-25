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
// Le rotte rispondono come la schermata da cui arriva il gesto (threadFrammento): dal Fascicolo con
// l'avviso e i suoi pannelli, da altrove con la pagina della RFQ.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// rifiuto e' un «no» per l'operatore: la transazione si annulla per intero e l'avviso dice il motivo.
// E' lo stesso tipo del core (fascicolo.Rifiuto): un «no» detto qui e uno detto da fascicolo si
// riconoscono con un solo errors.As, e spiegaErrore li tratta allo stesso modo.
type rifiuto = fascicolo.Rifiuto

// stessoCodice confronta due codici come li confronta l'identita' del componente, upper(codice).
func stessoCodice(a, b string) bool {
	return strings.ToUpper(strings.TrimSpace(a)) == strings.ToUpper(strings.TrimSpace(b))
}

// codiceDaComponente e' il codice che un documento o una proposta prende quando si aggancia a c.
// attuale e' quello che hanno adesso ("" = nessuno).
//
// Lo stesso codice con altre maiuscole non e' una correzione: si copia la stringa del componente
// (A1.4, regola 8), perche' la FK la vuole identica. Un codice DIVERSO lo diventa solo con il gesto
// esplicito «assegna e correggi il codice» (regola 1): un disegno che dice 77720517 sotto il
// componente 77722757 e' un errore di qualcuno, e non si ripara in silenzio.
func codiceDaComponente(attuale string, c db.Componente, correggi bool) (string, error) {
	if strings.TrimSpace(attuale) != "" && !stessoCodice(attuale, c.Codice) && !correggi {
		return "", rifiuto(fmt.Sprintf("codice diverso: %s contro %s. Per assegnarlo comunque va corretto il codice del file (correggi_codice)",
			strings.TrimSpace(attuale), c.Codice))
	}
	return c.Codice, nil
}

// percorsi calcola il percorso di un documento come lo calcola la conferma: la sottocartella del
// tipo e la regola del cliente sulla cartella per codice (CartellaDocumento), e il nome sul NAS.
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

// cartellaDi e' la cartella che la conferma darebbe al documento d con il codice dato.
func (p *percorsi) cartellaDi(ctx context.Context, q *db.Queries, d db.Documento, codice string) (string, error) {
	l, ok := p.layout[d.Tipo]
	if !ok {
		var err error
		if l, err = q.GetCartellaDocumento(ctx, d.Tipo); err != nil {
			return "", fmt.Errorf("layout per %s: %w", d.Tipo, err)
		}
		p.layout[d.Tipo] = l
	}
	return documenti.CartellaDocumento(documenti.LayoutDocumento{Sottocartella: l.Sottocartella, PerCodice: l.PerCodice}, p.perCodice, codice)
}

// nomeDi e' il nome che il documento d avrebbe sul NAS con il codice dato. Un documento tecnico si
// chiama come il pezzo (D21): con il codice cambia anche il nome (addendum A4.1, «Conseguenza su
// B8.3»). Gli altri tengono il NOME CHE HANNO GIA': la correzione li sposta di cartella e basta.
func (p *percorsi) nomeDi(d db.Documento, codice string) string {
	if documenti.Tecnico(d.Tipo) {
		return documenti.NomeTecnico(codice, d.Rev.String, d.Estensione)
	}
	return documenti.NomeNelPercorso(d.PathRelativo)
}

// di e' il percorso che il documento d avrebbe con il codice dato: la cartella, il nome, e il
// progressivo se quel nome e' gia' preso da un altro. Il lucchetto delle cartelle lo ha preso chi
// chiama (bloccaCartelle), prima di scegliere.
func (p *percorsi) di(ctx context.Context, q *db.Queries, d db.Documento, codice string) (string, error) {
	cartella, err := p.cartellaDi(ctx, q, d, codice)
	if err != nil {
		return "", err
	}
	return documenti.RiscegliPercorso(ctx, q, d.ThreadID, cartella, p.nomeDi(d, codice), d.PathRelativo)
}

// perConferma mette i documenti nell'ordine in cui sono stati confermati: quando piu' file prendono
// lo stesso nome (due disegni dello stesso pezzo, dopo una correzione), il nome senza progressivo va al
// primo arrivato, e la risposta non dipende dall'ordine degli id. I lucchetti restano presi per id.
func perConferma(docs []db.Documento) []db.Documento {
	out := append([]db.Documento(nil), docs...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ConfermatoIl.Equal(out[j].ConfermatoIl) {
			return out[i].ConfermatoIl.Before(out[j].ConfermatoIl)
		}
		return out[i].DocumentoID.String() < out[j].DocumentoID.String()
	})
	return out
}

// bloccaCartelle prende i lucchetti delle cartelle in cui i documenti finiranno con il codice nuovo,
// in ordine di percorso (A4.3): chi tocca piu' cartelle le prende sempre nello stesso ordine, e due
// gesti che si incrociano si mettono in fila invece di aspettarsi a vicenda.
func (p *percorsi) bloccaCartelle(ctx context.Context, q *db.Queries, thread uuid.UUID, docs []db.Documento, codice string) error {
	viste := map[string]bool{}
	var cartelle []string
	for _, d := range docs {
		c, err := p.cartellaDi(ctx, q, d, codice)
		if err != nil {
			return rifiuto(d.NomeFile + ": " + err.Error())
		}
		if k := strings.ToLower(c); !viste[k] {
			viste[k] = true
			cartelle = append(cartelle, c)
		}
	}
	sort.Slice(cartelle, func(i, j int) bool { return strings.ToLower(cartelle[i]) < strings.ToLower(cartelle[j]) })
	for _, c := range cartelle {
		if err := documenti.BloccaCartella(ctx, q, thread, c); err != nil {
			return err
		}
	}
	return nil
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
	if cambia := d.ComponenteID.Valid && (c == nil || c.ComponenteID != d.ComponenteID.UUID); cambia {
		if err := restaAlSuoComponente(ctx, q, d); err != nil {
			return err
		}
	}
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

// restaAlSuoComponente dice perche' un documento non puo' lasciare il suo componente, prima che lo
// dica una FK: lo STEP strutturale registrato in una baseline congelata (R2.5, prova 69), lo STEP
// strutturale attuale, un documento dentro una catena di revisioni (D32).
func restaAlSuoComponente(ctx context.Context, q *db.Queries, d db.Documento) error {
	versioni, err := q.VersioniConLoStep(ctx, uuid.NullUUID{UUID: d.DocumentoID, Valid: true})
	if err != nil {
		return err
	}
	if len(versioni) > 0 {
		return rifiuto(fmt.Sprintf("%s è lo STEP strutturale registrato nella baseline V%d: non cambia più componente", d.NomeFile, versioni[0]))
	}
	c, err := q.GetComponente(ctx, d.ComponenteID.UUID)
	if err != nil {
		return err
	}
	if c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == d.DocumentoID {
		return rifiuto(fmt.Sprintf("%s è lo STEP strutturale di %s: prima se ne sceglie un altro", d.NomeFile, c.Codice))
	}
	if d.SostituitoDa.Valid {
		return rifiuto(d.NomeFile + " è stato sostituito: una revisione vecchia resta del suo componente")
	}
	return nil
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

// sceltaRevisione e' la risposta alla domanda che si fa quando un file entra in un componente che ha gia'
// un documento corrente dello stesso tipo (decisione del 24/09/2026 sulla condizione aperta di A4.10):
// il file nuovo si AGGIUNGE, e restano correnti tutti e due (il secondo foglio di un 2D), oppure
// SOSTITUISCE un predecessore preciso. Senza risposta non si procede: cosi' una proposta di
// sostituzione pendente non esiste, e non serve uno schema per ricordarla.
type sceltaRevisione struct {
	data        bool      // la domanda ha avuto una risposta
	sostituisce uuid.UUID // il predecessore; zero = «aggiungi»
	// riferimento: se il predecessore era lo STEP strutturale del componente, il nuovo ne prende il
	// posto (A4.4: la schermata lo chiede, con il si' preselezionato). rispostaRiferimento dice che la
	// domanda ha avuto una risposta: sostituire lo STEP strutturale senza dirlo si rifiuta (B8.7).
	riferimento, rispostaRiferimento bool
	// motivo: perche' si sostituisce. Una versione interna (un file caricato a mano, B8.7) sostituisce
	// solo con un motivo, e il motivo resta nella nota del documento nuovo.
	motivo  string
	interno bool
}

// leggiScelta legge `aggiungi` oppure l'id del documento da sostituire (vuoto = nessuna risposta), la
// risposta alla domanda sullo STEP strutturale ("1" si', "0" no, "" nessuna) e il motivo.
func leggiScelta(v, riferimento, motivo string) (sceltaRevisione, error) {
	v = strings.TrimSpace(v)
	sc := sceltaRevisione{motivo: strings.TrimSpace(motivo)}
	switch strings.TrimSpace(riferimento) {
	case "1":
		sc.riferimento, sc.rispostaRiferimento = true, true
	case "0":
		sc.rispostaRiferimento = true
	}
	switch v {
	case "":
		return sc, nil
	case "aggiungi":
		sc.data = true
		return sc, nil
	}
	id, err := uuid.Parse(strings.TrimPrefix(v, "sostituisci:"))
	if err != nil {
		return sceltaRevisione{}, rifiuto("scelta non valida: si risponde «aggiungi» oppure con il documento da sostituire")
	}
	sc.data, sc.sostituisce = true, id
	return sc, nil
}

// correntiDelloStessoTipo sono i documenti correnti del componente con quel tipo, escluso uno.
// ListDocumentiComponente li blocca: due assegnazioni allo stesso pezzo si mettono in fila, e la
// seconda vede il file della prima.
func correntiDelloStessoTipo(ctx context.Context, q *db.Queries, comp uuid.UUID, tipo db.TipoDocumento, escluso uuid.UUID) ([]db.Documento, error) {
	tutti, err := q.ListDocumentiComponente(ctx, uuid.NullUUID{UUID: comp, Valid: true})
	if err != nil {
		return nil, err
	}
	var out []db.Documento
	for _, d := range tutti {
		if d.Tipo == tipo && !d.SostituitoDa.Valid && d.DocumentoID != escluso {
			out = append(out, d)
		}
	}
	return out, nil
}

// verificaScelta applica la regola: con almeno un corrente dello stesso tipo serve la risposta, e un
// «sostituisce» deve nominare uno di quei correnti. nome e' il file che entra. Restituisce la scelta da
// applicare: un «aggiungi» dato dove non c'era niente da chiedere non dice niente, e si lascia cadere.
func verificaScelta(nome string, c db.Componente, tipo db.TipoDocumento, correnti []db.Documento, s sceltaRevisione) (sceltaRevisione, error) {
	if len(correnti) == 0 && s.sostituisce == uuid.Nil {
		return sceltaRevisione{}, nil
	}
	if len(correnti) > 0 && !s.data {
		nomi := make([]string, len(correnti))
		for i, d := range correnti {
			nomi[i] = d.NomeFile + " (" + d.DocumentoID.String() + ")"
		}
		return s, rifiuto(fmt.Sprintf("%s: %s ha già %d document%s %s corrent%s (%s). Si sceglie: «aggiungi» (restano correnti tutti) oppure il documento che questo sostituisce",
			nome, c.Codice, len(correnti), plurale(len(correnti), "o", "i"), tipo, plurale(len(correnti), "e", "i"), strings.Join(nomi, ", ")))
	}
	if s.sostituisce == uuid.Nil {
		return s, nil
	}
	trovato := false
	for _, d := range correnti {
		trovato = trovato || d.DocumentoID == s.sostituisce
	}
	if !trovato {
		return s, rifiuto(fmt.Sprintf("%s: il documento da sostituire deve essere un %s corrente di %s", nome, tipo, c.Codice))
	}
	return s, verificaSostituzione(nome, c, s)
}

// verificaSostituzione sono le due domande in piu' di una sostituzione (B8.7): se il predecessore e' lo
// STEP strutturale del componente, si dice esplicitamente se il nuovo diventa il riferimento; se il file
// nuovo e' una versione interna, si dice perche'.
func verificaSostituzione(nome string, c db.Componente, s sceltaRevisione) error {
	if c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == s.sostituisce && !s.rispostaRiferimento {
		return rifiuto(fmt.Sprintf("%s sostituisce lo STEP strutturale di %s: si dice se il nuovo diventa il riferimento (sì o no)", nome, c.Codice))
	}
	if s.interno && s.motivo == "" {
		return rifiuto(fmt.Sprintf("%s è una versione interna: sostituisce un documento solo con un motivo", nome))
	}
	return nil
}

// applicaScelta registra la risposta dopo che il file nuovo e' entrato nel componente: la
// sostituzione e' la stessa di fascicolo.Sostituisci, con la catena tenuta dal database (D32).
func applicaScelta(ctx context.Context, q *db.Queries, thread, nuovo uuid.UUID, s sceltaRevisione) (string, error) {
	if !s.data {
		return "", nil
	}
	if s.sostituisce == uuid.Nil {
		return " Si aggiunge: i documenti dello stesso tipo restano correnti.", nil
	}
	msg, err := fascicolo.Sostituisci(ctx, q, thread, s.sostituisce, nuovo, s.riferimento)
	if err != nil {
		return "", err
	}
	if err := notaSostituzione(ctx, q, s.sostituisce, nuovo, s.motivo); err != nil {
		return "", err
	}
	return " " + msg, nil
}

// notaSostituzione scrive nella nota del documento nuovo che cosa ha sostituito e perche'. Senza motivo
// non scrive niente: la catena dice gia' chi ha sostituito chi.
func notaSostituzione(ctx context.Context, q *db.Queries, vecchio, nuovo uuid.UUID, motivo string) error {
	if motivo == "" {
		return nil
	}
	v, err := q.GetDocumento(ctx, vecchio)
	if err != nil {
		return err
	}
	n, err := q.GetDocumento(ctx, nuovo)
	if err != nil {
		return err
	}
	nota := "sostituisce " + v.NomeFile + ": " + motivo
	if n.Nota.Valid && strings.TrimSpace(n.Nota.String) != "" {
		nota = n.Nota.String + " · " + nota
	}
	return q.SetNotaDocumento(ctx, db.SetNotaDocumentoParams{DocumentoID: nuovo, Nota: pgtype.Text{String: nota, Valid: true}})
}

// assegnaAlComponente e' il gesto «assegna»: documenti e proposte della RFQ thread sotto il componente
// comp, o sganciati se comp non e' valido. Tutto o niente: se uno solo dei file non si puo', non cambia
// niente e il rifiuto dice quale e perche'. E' una decisione su un gruppo di file per un pezzo, e fatta
// a meta' non vorrebbe dire niente.
//
// Un documento che entra in un componente con un documento corrente dello stesso tipo vuole la sua
// scelta in scelte (aggiungi / sostituisce); una proposta no: la domanda gliela fa la conferma, quando
// diventa documento.
func assegnaAlComponente(ctx context.Context, q *db.Queries, thread uuid.UUID, comp uuid.NullUUID, docs, props []uuid.UUID, correggi bool,
	scelte map[uuid.UUID]sceltaRevisione) (string, error) {
	// La RFQ prima di componenti, documenti e proposte: e' l'ordine del congelamento e degli altri gesti.
	// Rovesciato (i documenti bloccati qui, poi il trigger della working che chiede la RFQ) basta un
	// congelamento nello stesso istante perche' le due transazioni si aspettino a vicenda (40P01).
	if err := preparaGesto(ctx, q, thread); err != nil {
		return "", err
	}
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
	// Con la BOM congelata (D26) nessun documento cambia componente, e nessun file si aggancia a un
	// componente: il database lo rifiuterebbe per i documenti, e una proposta agganciata non si
	// potrebbe poi confermare. Sganciare una proposta resta libero: non tocca la BOM.
	if len(docs) > 0 || c != nil {
		if n, bloccata, err := fascicolo.WorkingBloccata(ctx, q, thread); err != nil {
			return "", err
		} else if bloccata {
			return "", rifiuto(fmt.Sprintf("la BOM è congelata nella V%d: il file entra senza componente, e lo si assegna aprendo una revisione", n))
		}
	}
	perc, err := nuoviPercorsi(ctx, q, thread)
	if err != nil {
		return "", err
	}
	bloccati := make([]db.Documento, 0, len(docs))
	for _, id := range docs {
		d, err := q.BloccaDocumento(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.ThreadID != thread) {
			return "", rifiuto("un documento scelto non è di questa RFQ")
		}
		if err != nil {
			return "", err
		}
		bloccati = append(bloccati, d)
	}
	if c != nil {
		if err := perc.bloccaCartelle(ctx, q, thread, bloccati, c.Codice); err != nil {
			return "", err
		}
	}
	revisioni := ""
	for _, d := range perConferma(bloccati) {
		var scelta sceltaRevisione
		entra := c != nil && (!d.ComponenteID.Valid || d.ComponenteID.UUID != c.ComponenteID)
		if entra {
			correnti, err := correntiDelloStessoTipo(ctx, q, c.ComponenteID, d.Tipo, d.DocumentoID)
			if err != nil {
				return "", err
			}
			sc := scelte[d.DocumentoID]
			if sc.sostituisce != uuid.Nil {
				if sc.interno, err = q.DocumentoInterno(ctx, d.DocumentoID); err != nil {
					return "", err
				}
			}
			if scelta, err = verificaScelta(d.NomeFile, *c, d.Tipo, correnti, sc); err != nil {
				return "", err
			}
		}
		if err := assegnaDocumento(ctx, q, perc, d, c, correggi); err != nil {
			return "", err
		}
		msg, err := applicaScelta(ctx, q, thread, d.DocumentoID, scelta)
		if err != nil {
			return "", err
		}
		revisioni += msg
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
	return file + " assegnat" + plurale(n, "o", "i") + " al componente " + c.Codice + "." + revisioni, nil
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
	// la stessa guardia di ogni altro codice che entra in una colonna (i nodi, l'editor, il worker)
	if !classificazione.CodiceAmmissibile(nuovo) {
		return "", rifiuto(fmt.Sprintf("il codice ha più di %d caratteri o caratteri non ammessi", classificazione.MaxCodice))
	}
	// La RFQ prima del componente e dei documenti, come nel congelamento e negli altri gesti: con
	// l'ordine rovesciato due transazioni si aspettano a vicenda e PostgreSQL ne uccide una (40P01).
	if err := preparaGesto(ctx, q, thread); err != nil {
		return "", err
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
	if n, bloccata, err := fascicolo.WorkingBloccata(ctx, q, thread); err != nil {
		return "", err
	} else if bloccata {
		return "", rifiuto(fmt.Sprintf("la BOM è congelata nella V%d: il codice si corregge aprendo una revisione", n))
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
	if err := perc.bloccaCartelle(ctx, q, thread, docs, nuovo); err != nil {
		return "", err
	}
	for _, d := range perConferma(docs) {
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
		// i trigger della 0020 hanno i loro codici (A4): li si riconosce senza leggere il testo
		switch pg.Code {
		case "40P01":
			// Due gesti sulla stessa RFQ che si sono aspettati a vicenda: PostgreSQL ne ha fermato uno,
			// questo. Niente e' cambiato, e rifarlo adesso di solito riesce.
			return "un'altra operazione sulla stessa RFQ era in corso nello stesso momento: riprova"
		case "BOM01":
			return "la BOM è congelata: si modifica solo aprendo una revisione"
		case "BOM02":
			return "una versione congelata della BOM, e le sue istantanee, non si modificano"
		case "BOM03":
			return "revisioni del documento: " + pg.Message
		case "BOM04":
			return "STEP strutturale: " + pg.Message
		case "BOM05":
			return "una deroga strutturale non si modifica: se ne concede una nuova"
		}
		switch pg.ConstraintName {
		case "documento_thread_id_sha256_key":
			return "lo stesso file è stato confermato in questo momento da un'altra sessione: ricarica la pagina"
		case "ux_documento_percorso":
			return "un altro documento ha preso quel nome sul NAS in questo momento: riprova"
		case "fk_bvc_step":
			return "il documento è lo STEP strutturale di una baseline congelata: non cambia più componente"
		case "fk_componente_step_strutturale":
			return "il documento è lo STEP strutturale del suo componente: prima se ne sceglie un altro"
		case "fk_documento_sostituito_stesso_tipo", "fk_documento_sostituito_stessa_rfq":
			return "il documento fa parte di una catena di revisioni: componente e tipo non cambiano"
		case "fk_deroga_struttura_step":
			return "sul documento c'è una deroga strutturale: non cambia componente"
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
//	scelta_<doc>     per un documento che entra in un componente con un corrente dello stesso tipo:
//	                 "aggiungi" oppure l'id del documento che sostituisce (con un solo documento basta
//	                 `scelta`)
//	nuovo_riferimento_<doc>  se il sostituito e' lo STEP strutturale: "1" il nuovo prende il suo posto,
//	                 "0" no; senza risposta la sostituzione si rifiuta (B8.7)
//	motivo_<doc>     perche' si sostituisce: obbligatorio per una versione interna (B8.7)
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
	scelte := map[uuid.UUID]sceltaRevisione{}
	for _, d := range docs {
		v, rif, mot := r.FormValue("scelta_"+d.String()), r.FormValue("nuovo_riferimento_"+d.String()), r.FormValue("motivo_"+d.String())
		if len(docs) == 1 && v == "" {
			v = r.FormValue("scelta")
		}
		if len(docs) == 1 && rif == "" {
			rif = r.FormValue("nuovo_riferimento")
		}
		if len(docs) == 1 && mot == "" {
			mot = r.FormValue("motivo")
		}
		sc, err := leggiScelta(v, rif, mot)
		if err != nil {
			s.threadFrammento(w, r, thread, "Assegnazione non riuscita, nessun file cambiato: "+spiegaErrore(err))
			return
		}
		scelte[d] = sc
	}
	msg, err := assegnaAlComponente(ctx, db.New(tx), thread, comp, docs, props, r.FormValue("correggi_codice") == "1", scelte)
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
