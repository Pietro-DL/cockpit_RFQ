package fascicolo

// Le correzioni a mano della BOM working che la schermata del Fascicolo offre (Blocco 8, B8.7; piano §7.3):
// tipo, revisione e descrizione di un componente; un arco messo, tolto o spostato; la deroga di un
// requisito del fascicolo. Il codice ha il suo gesto (A1.4), perche' tocca i documenti e il NAS.
//
// Sono decisioni come le altre: la RFQ bloccata, D26 detto prima del muro del database, i cicli rifiutati
// con la stessa funzione delle proposte (CreerebbeCiclo), le rimozioni ricalcolate dopo ogni arco
// (dopoLaDecisione), perche' dicano sempre che cosa la working e lo STEP strutturale dicono adesso.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// MaxQtaArco e' la quantita' piu' alta che un arco scritto a mano puo' avere: oltre e' un errore di
// battitura, non una distinta.
const MaxQtaArco = 100000

// ModificaComponente cambia tipo, revisione e descrizione di un componente. Uno STEP strutturale esiste
// solo per un prodotto finito (A4.4): chi toglie la qualifica di prodotto deve prima togliere quello.
func ModificaComponente(ctx context.Context, q *db.Queries, thread, comp, utente uuid.UUID, tipo db.TipoComponente, rev, descrizione string) (string, error) {
	if err := prepara(ctx, q, thread, "si modifica un componente"); err != nil {
		return "", err
	}
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	if c.ArchiviatoIl != nil {
		return "", Rifiuto(c.Codice + " è archiviato: prima lo si ripristina")
	}
	if !tipo.Valid() {
		return "", Rifiuto("tipo di componente non valido")
	}
	rev, descrizione = strings.ToUpper(strings.TrimSpace(rev)), strings.TrimSpace(descrizione)
	if rev != "" && !classificazione.RevAmmissibile(rev) {
		return "", Rifiuto(fmt.Sprintf("revisione non valida: al massimo %d caratteri, senza spazi", classificazione.MaxRev))
	}
	if utf8.RuneCountInString(descrizione) > 200 {
		return "", Rifiuto("la descrizione ha più di 200 caratteri")
	}
	if tipo != db.TipoComponenteFinito && c.StepStrutturaleID.Valid {
		return "", Rifiuto(fmt.Sprintf("%s ha uno STEP strutturale: resta un prodotto finito finché quel riferimento c'è", c.Codice))
	}
	var cambi []string
	if tipo != c.Tipo {
		cambi = append(cambi, fmt.Sprintf("tipo %s → %s", NomeTipo(c.Tipo), NomeTipo(tipo)))
	}
	if rev != c.Rev.String {
		cambi = append(cambi, fmt.Sprintf("rev %s → %s", vuotoTrattino(c.Rev.String), vuotoTrattino(rev)))
	}
	if descrizione != c.Descrizione.String {
		cambi = append(cambi, "descrizione")
	}
	if len(cambi) == 0 {
		return "Nessun cambiamento: " + c.Codice + " è già così.", nil
	}
	if _, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{ComponenteID: comp, Tipo: tipo, Rev: testo(rev),
		Descrizione: testo(descrizione), ConfermatoDa: utente}); err != nil {
		return "", err
	}
	return dopoLaDecisione(ctx, q, thread, c.Codice+": "+strings.Join(cambi, ", ")+".", nil)
}

func vuotoTrattino(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// Collega mette figlio sotto padre con la quantita' qta; se l'arco c'e' gia', ne cambia la quantita'. Un
// arco che chiuderebbe un giro si rifiuta e dice quale.
func Collega(ctx context.Context, q *db.Queries, thread, padre, figlio, utente uuid.UUID, qta int32) (string, error) {
	if err := prepara(ctx, q, thread, "si cambia la struttura"); err != nil {
		return "", err
	}
	msg, nuovo, err := collega(ctx, q, thread, padre, figlio, utente, qta)
	if msg, err = dopoLaDecisione(ctx, q, thread, msg, err); err != nil || !nuovo {
		return msg, err
	}
	return msg, tieniArcoMessoAMano(ctx, q, thread, Arco{Padre: padre, Figlio: figlio}, utente)
}

// collega e' il cuore di Collega; nuovo dice se l'arco l'ha messo adesso (e non ne ha solo cambiato la
// quantita').
func collega(ctx context.Context, q *db.Queries, thread, padre, figlio, utente uuid.UUID, qta int32) (msg string, nuovo bool, err error) {
	if qta < 1 || qta > MaxQtaArco {
		return "", false, Rifiuto(fmt.Sprintf("la quantità va da 1 a %d", MaxQtaArco))
	}
	if padre == figlio {
		return "", false, Rifiuto("un componente non sta sotto se stesso")
	}
	p, err := componenteAttivo(ctx, q, thread, padre)
	if err != nil {
		return "", false, err
	}
	f, err := componenteAttivo(ctx, q, thread, figlio)
	if err != nil {
		return "", false, err
	}
	esistente, err := q.GetRelazione(ctx, db.GetRelazioneParams{PadreID: padre, FiglioID: figlio})
	switch {
	case err == nil:
		if esistente.Qta == qta {
			return fmt.Sprintf("%s è già sotto %s ×%d.", f.Codice, p.Codice, qta), false, nil
		}
		if _, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{PadreID: padre, FiglioID: figlio, Qta: qta, ConfermatoDa: utente}); err != nil {
			return "", false, err
		}
		return fmt.Sprintf("%s sotto %s: quantità %d → %d.", f.Codice, p.Codice, esistente.Qta, qta), false, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return "", false, err
	}
	archi, err := archiAttivi(ctx, q, thread)
	if err != nil {
		return "", false, err
	}
	if giro := CreerebbeCiclo(archi, padre, figlio); giro != nil {
		return "", false, Rifiuto(fmt.Sprintf("%s sotto %s chiuderebbe un ciclo (%s)", f.Codice, p.Codice, ciclo(ctx, q, giro)))
	}
	if _, err := q.InsertRelazione(ctx, db.InsertRelazioneParams{ThreadID: thread, PadreID: padre, FiglioID: figlio, Qta: qta,
		Origine: db.OrigineComponenteManuale, ConfermatoDa: utente}); err != nil {
		return "", false, err
	}
	return fmt.Sprintf("%s sotto %s ×%d.", f.Codice, p.Codice, qta), true, nil
}

// tieniArcoMessoAMano chiude le rimozioni che lo STEP strutturale propone su un arco appena messo a mano.
// Il ricalcolo dopo la decisione le propone subito — lo STEP quell'arco non ce l'ha, ed e' proprio per
// questo che una persona l'ha messo — e il gate le conterebbe fra le proposte da decidere: sarebbe
// chiedere di confermare due volte la stessa decisione. E' quello che fa l'editor della struttura
// (TieniArcoDellaRimozione, in ApplicaStrutturaVoluta); una rimozione scartata non si ripropone.
func tieniArcoMessoAMano(ctx context.Context, q *db.Queries, thread uuid.UUID, a Arco, utente uuid.UUID) error {
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return err
	}
	for _, r := range rim {
		if r.PadreID != a.Padre || r.FiglioID != a.Figlio {
			continue
		}
		if _, err := q.TieniArcoDellaRimozione(ctx, db.TieniArcoDellaRimozioneParams{ThreadID: thread, StepDocumentoID: r.StepDocumentoID,
			PadreID: r.PadreID, FiglioID: r.FiglioID, DecisoDa: uid(utente)}); err != nil {
			return err
		}
	}
	return nil
}

// Scollega toglie l'arco padre → figlio. Il figlio resta nella BOM: se non ha altri padri torna fra le
// radici, da sistemare (D29).
func Scollega(ctx context.Context, q *db.Queries, thread, padre, figlio uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si cambia la struttura"); err != nil {
		return "", err
	}
	msg, err := scollega(ctx, q, thread, padre, figlio)
	return dopoLaDecisione(ctx, q, thread, msg, err)
}

func scollega(ctx context.Context, q *db.Queries, thread, padre, figlio uuid.UUID) (string, error) {
	p, err := componenteDellaRfq(ctx, q, thread, padre)
	if err != nil {
		return "", err
	}
	f, err := componenteDellaRfq(ctx, q, thread, figlio)
	if err != nil {
		return "", err
	}
	n, err := q.DeleteRelazione(ctx, db.DeleteRelazioneParams{PadreID: padre, FiglioID: figlio})
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", Rifiuto(fmt.Sprintf("%s non è sotto %s", f.Codice, p.Codice))
	}
	return fmt.Sprintf("%s non è più sotto %s.", f.Codice, p.Codice), nil
}

// Sposta porta figlio da sotto da (se c'e') a sotto a (se c'e'; nessuno = radice), con la quantita' qta:
// e' «scollega» piu' «collega» in una decisione sola, tutto o niente.
func Sposta(ctx context.Context, q *db.Queries, thread, figlio uuid.UUID, da, a uuid.NullUUID, utente uuid.UUID, qta int32) (string, error) {
	if err := prepara(ctx, q, thread, "si cambia la struttura"); err != nil {
		return "", err
	}
	if da == a {
		return "", Rifiuto("il padre di partenza e quello di arrivo sono lo stesso")
	}
	var parti []string
	if da.Valid {
		m, err := scollega(ctx, q, thread, da.UUID, figlio)
		if err != nil {
			return "", err
		}
		parti = append(parti, m)
	}
	nuovo := false
	if a.Valid {
		m, n, err := collega(ctx, q, thread, a.UUID, figlio, utente, qta)
		if err != nil {
			return "", err
		}
		parti, nuovo = append(parti, m), n
	} else {
		c, err := q.GetComponente(ctx, figlio)
		if err != nil {
			return "", err
		}
		altri, err := archiAttivi(ctx, q, thread)
		if err != nil {
			return "", err
		}
		radice := true
		for _, x := range altri {
			radice = radice && x.Figlio != figlio
		}
		if radice {
			parti = append(parti, c.Codice+" è una radice.")
		} else {
			parti = append(parti, c.Codice+" resta sotto gli altri padri.")
		}
	}
	msg, err := dopoLaDecisione(ctx, q, thread, strings.Join(parti, " "), nil)
	if err != nil || !nuovo {
		return msg, err
	}
	return msg, tieniArcoMessoAMano(ctx, q, thread, Arco{Padre: a.UUID, Figlio: figlio}, utente)
}

// componenteAttivo e' componenteDellaRfq per chi entra in un arco: un archiviato prima si ripristina.
func componenteAttivo(ctx context.Context, q *db.Queries, thread, comp uuid.UUID) (db.Componente, error) {
	c, err := componenteDellaRfq(ctx, q, thread, comp)
	if err != nil {
		return c, err
	}
	if c.ArchiviatoIl != nil {
		return c, Rifiuto(c.Codice + " è archiviato: prima lo si ripristina")
	}
	return c, nil
}

// ------------------------------------------------------------------ deroghe del fabbisogno

// ConcediDeroga dice «per questo componente questo documento non serve» (deroga_fabbisogno): vale per
// (componente, tipo), qualunque file arrivi dopo. Si concede solo su un requisito che il fascicolo chiede.
// Una deroga che c'e' gia' cambia solo il motivo. Per uno STEP strutturale letto in parte non basta: quella
// e' la deroga strutturale (D33, D36).
func ConcediDeroga(ctx context.Context, q *db.Queries, thread, comp, utente uuid.UUID, tipo db.TipoDocumento, motivo string) (string, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return "", Rifiuto("una deroga si concede con il suo motivo")
	}
	if err := prepara(ctx, q, thread, "si concede una deroga"); err != nil {
		return "", err
	}
	c, err := componenteAttivo(ctx, q, thread, comp)
	if err != nil {
		return "", err
	}
	righe, err := q.ListFascicolo(ctx, thread)
	if err != nil {
		return "", err
	}
	chiesto := false
	for _, r := range righe {
		chiesto = chiesto || (r.ComponenteID == comp && r.TipoDocumento == tipo)
	}
	if !chiesto {
		return "", Rifiuto(fmt.Sprintf("per %s il fascicolo non chiede un %s: non c'è niente da derogare", c.Codice, tipo))
	}
	if _, err := q.InsertDeroga(ctx, db.InsertDerogaParams{ThreadID: thread, ComponenteID: comp, Tipo: tipo, Motivo: motivo, UtenteID: utente}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s derogato («%s»).", c.Codice, tipo, motivo), nil
}

// RevocaDeroga toglie una deroga del fabbisogno: il requisito torna a contare. Una baseline che l'aveva
// la ricorda nella sua istantanea (R2.9), e non cambia.
func RevocaDeroga(ctx context.Context, q *db.Queries, thread, deroga uuid.UUID) (string, error) {
	if err := prepara(ctx, q, thread, "si revoca una deroga"); err != nil {
		return "", err
	}
	n, err := q.DeleteDeroga(ctx, db.DeleteDerogaParams{DerogaID: deroga, ThreadID: thread})
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", Rifiuto("deroga non trovata in questa RFQ")
	}
	return "Deroga revocata: il requisito torna a contare.", nil
}

// ------------------------------------------------------------------ revisioni di un caricamento interno

// RevisioneProponibile e' la revisione che la proposta di un file puo' portare. Un file caricato a mano
// (origine manuale) non inventa una revisione del cliente: quella letta nel nome o nel cartiglio resta
// nei dettagli (seconda risposta), e la scrive chi decide, se vuole.
func RevisioneProponibile(origine db.OrigineAllegato, rev string) (proponibile, trattenuta string) {
	if origine == db.OrigineAllegatoManuale && strings.TrimSpace(rev) != "" {
		return "", rev
	}
	return rev, ""
}

// ------------------------------------------------------------------ il contenitore dei caricamenti interni

// ChiaveNotaInterna e' la chiave esterna del contenitore dei caricamenti interni di una RFQ.
func ChiaveNotaInterna(thread uuid.UUID) string { return "nota:caricamenti:" + thread.String() }

// NotaInterna restituisce il messaggio che contiene i file caricati a mano nella RFQ: del canale `nota`,
// uno per RFQ, gia' agganciato. Lo crea al primo caricamento; dopo lo ritrova.
func NotaInterna(ctx context.Context, q *db.Queries, thread uuid.UUID, u db.Utente) (db.Messaggio, error) {
	if _, err := q.GetThread(ctx, thread); errors.Is(err, pgx.ErrNoRows) {
		return db.Messaggio{}, Rifiuto("RFQ non trovata")
	} else if err != nil {
		return db.Messaggio{}, err
	}
	chiave := ChiaveNotaInterna(thread)
	conv, err := q.UpsertConversazione(ctx, db.UpsertConversazioneParams{Canale: db.CanaleNota, ChiaveEsterna: chiave, PrimoMessaggioIl: adesso()})
	if err != nil {
		return db.Messaggio{}, err
	}
	if err := q.CollegaConversazioneNota(ctx, db.CollegaConversazioneNotaParams{ThreadID: uid(thread), ConversazioneID: conv.ConversazioneID}); err != nil {
		return db.Messaggio{}, err
	}
	m, err := q.UpsertNotaInterna(ctx, db.UpsertNotaInternaParams{Chiave: chiave, ConversazioneID: conv.ConversazioneID,
		MittenteNome: testo(u.Nome), Oggetto: testo("Caricamenti interni"),
		Corpo:    testo("File caricati a mano nel Fascicolo di questa RFQ: nuove versioni interne di CAD e disegni. Ognuno fa la strada degli allegati (staging, analisi, proposta) e diventa un documento solo con una decisione."),
		ThreadID: uid(thread), Utente: uid(u.UtenteID)})
	if err != nil {
		return db.Messaggio{}, err
	}
	if !m.ThreadID.Valid || m.ThreadID.UUID != thread {
		return db.Messaggio{}, fmt.Errorf("la nota interna %s non è di questa RFQ", chiave)
	}
	return m, nil
}

// Caricato e' un file caricato a mano, gia' verificato e messo fra i contenuti dello staging.
type Caricato struct {
	Nome     string
	Sha256   string
	Bytes    int64
	Percorso string // il contenuto nello staging (staging.PercorsoContenuto)
	Tipo     string // il content type dichiarato dal browser, solo informativo
	// PathInterno: da dove viene, quando non viene dal PC di chi carica: per un file importato dal NAS
	// (B8.7b) «NAS: <percorso sotto la radice>». Solo informativo, come per le voci di uno zip.
	PathInterno string
}

// RegistraCaricamento aggiunge il file alla nota interna come allegato di origine manuale, gia' in staging.
// Da qui fa la strada degli altri: proposta dal nome, analisi, decisione. Il messaggio si blocca prima di
// scegliere il posto del file, cosi' due caricamenti insieme non prendono lo stesso indice.
func RegistraCaricamento(ctx context.Context, q *db.Queries, nota db.Messaggio, utente uuid.UUID, f Caricato) (db.Allegato, error) {
	if _, err := q.BloccaMessaggio(ctx, nota.MessaggioID); err != nil {
		return db.Allegato{}, err
	}
	indice, err := q.ProssimoIndiceAllegato(ctx, nota.MessaggioID)
	if err != nil {
		return db.Allegato{}, err
	}
	ext := ""
	if i := strings.LastIndex(f.Nome, "."); i >= 0 && i < len(f.Nome)-1 {
		ext = strings.ToLower(f.Nome[i+1:])
	}
	if len(ext) > 10 {
		ext = ""
	}
	a, err := q.UpsertAllegato(ctx, db.UpsertAllegatoParams{MessaggioID: nota.MessaggioID, Indice: int16(indice), NomeFile: f.Nome,
		PathInterno: testo(tronca(f.PathInterno, 500)), Estensione: testo(ext), ContentType: testo(tronca(f.Tipo, 120)), Natura: db.NaturaAllegatoFile, Origine: db.OrigineAllegatoManuale,
		Bytes: pgtype.Int8{Int64: f.Bytes, Valid: true}, Sha256: testo(f.Sha256), RicevutoIl: adesso(), CaricatoDa: uid(utente)})
	if err != nil {
		return db.Allegato{}, err
	}
	if err := q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: a.AllegatoID, PathStaging: testo(f.Percorso),
		Sha256: testo(f.Sha256), Bytes: pgtype.Int8{Int64: f.Bytes, Valid: true}}); err != nil {
		return db.Allegato{}, err
	}
	return q.GetAllegato(ctx, a.AllegatoID)
}

// adesso e' l'istante dei caricamenti: una variabile, perche' le prove lo possano fissare.
var adesso = time.Now

// tronca taglia a n byte senza spezzare un carattere: una stringa UTF-8 tagliata a meta' il database la
// rifiuta.
func tronca(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
