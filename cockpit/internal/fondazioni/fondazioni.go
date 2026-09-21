// Package fondazioni porta in DB ciò che cockpit.toml dichiara nelle sezioni [[casella]],
// [[postazione]] e [[worker]] (piano di correzione, voce 0.6).
//
// Il seed è NON distruttivo: aggiorna per chiave naturale (indirizzo, nome host, nome worker) e non
// disattiva mai una riga che non compare più nel file — la segnala soltanto, perché disattivare una
// casella significa fermarne il sync e non deve succedere per una svista di configurazione.
//
// Il token di un worker resta nel file: in DB va il suo sha256 (`worker_credenziale.token_hash`).
package fondazioni

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/config"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/rete"
)

// Esito riassume cosa ha fatto il seed; il chiamante lo logga e i test lo verificano.
type Esito struct {
	Caselle        int
	Postazioni     int
	Worker         int
	CasellaDefault uuid.NullUUID
	NonPiuNelFile  []string // righe presenti in DB e assenti dal file: mai disattivate d'ufficio
	UtentiMancanti []string // sigle citate dal file e non presenti in `utente`
	// CredenzialiDaGenerare: worker che NON potranno collegarsi finché non si rigenera il pacchetto
	// della loro postazione — token mai generato, oppure condiviso con un altro worker (voce 2.4).
	CredenzialiDaGenerare []string
}

// HashToken calcola l'impronta con cui il server riconosce il token di un worker.
func HashToken(token string) string { return rete.ImprontaToken(token) }

// Semina applica le sezioni di cockpit.toml al DB. Va chiamata dopo le migrazioni e dopo il seed
// degli utenti, perché casella.utente_id e postazione.utente_id li referenziano per sigla.
func Semina(ctx context.Context, q *db.Queries, cfg *config.Config, log *slog.Logger) (Esito, error) {
	var e Esito

	utenti, err := q.ListUtenti(ctx)
	if err != nil {
		return e, fmt.Errorf("fondazioni: utenti: %w", err)
	}
	perSigla := make(map[string]uuid.UUID, len(utenti))
	for _, u := range utenti {
		perSigla[strings.ToUpper(u.Sigla)] = u.UtenteID
	}
	risolviUtente := func(sigla, dove string) uuid.NullUUID {
		if sigla == "" {
			return uuid.NullUUID{}
		}
		id, ok := perSigla[sigla]
		if !ok {
			e.UtentiMancanti = append(e.UtentiMancanti, fmt.Sprintf("%s (%s)", sigla, dove))
			return uuid.NullUUID{}
		}
		return uuid.NullUUID{UUID: id, Valid: true}
	}

	// ---------------------------------------------------------------- caselle
	idCasella := map[string]uuid.UUID{}
	for _, k := range cfg.Caselle {
		canale := db.Canale(k.Canale)
		if !canale.Valid() {
			return e, fmt.Errorf("fondazioni: casella %s: canale %q non valido", k.Indirizzo, k.Canale)
		}
		riga, err := q.UpsertCasella(ctx, db.UpsertCasellaParams{
			Canale:    canale,
			Indirizzo: k.Indirizzo,
			Nome:      k.Nome,
			Condivisa: k.Condivisa,
			UtenteID:  risolviUtente(k.Utente, "casella "+k.Indirizzo),
			Attiva:    pgtype.Bool{Bool: k.EAttiva(), Valid: true},
		})
		if err != nil {
			return e, fmt.Errorf("fondazioni: casella %s: %w", k.Indirizzo, err)
		}
		idCasella[k.Indirizzo] = riga.CasellaID
		e.Caselle++
	}

	// ---------------------------------------------------------------- postazioni
	idPostazione := map[string]uuid.UUID{}
	for _, p := range cfg.Postazioni {
		riga, err := q.UpsertPostazione(ctx, db.UpsertPostazioneParams{
			NomeHost:    p.NomeHost,
			Descrizione: p.Descrizione,
			UtenteID:    risolviUtente(p.Utente, "postazione "+p.NomeHost),
		})
		if err != nil {
			return e, fmt.Errorf("fondazioni: postazione %s: %w", p.NomeHost, err)
		}
		idPostazione[p.NomeHost] = riga.PostazioneID
		e.Postazioni++
	}

	// ---------------------------------------------------------------- worker
	for _, w := range cfg.Worker {
		tipo := db.WorkerTipo(w.Tipo)
		if !tipo.Valid() || tipo == db.WorkerTipoServer {
			return e, fmt.Errorf("fondazioni: worker %s: tipo %q non valido", w.Nome, w.Tipo)
		}
		var post uuid.NullUUID
		if w.Postazione != "" {
			id, ok := idPostazione[w.Postazione]
			if !ok {
				return e, fmt.Errorf("fondazioni: worker %s: postazione %s non censita", w.Nome, w.Postazione)
			}
			post = uuid.NullUUID{UUID: id, Valid: true}
		}
		caselle := make([]uuid.UUID, 0, len(w.Caselle))
		for _, ind := range w.Caselle {
			id, ok := idCasella[ind]
			if !ok {
				return e, fmt.Errorf("fondazioni: worker %s: casella %s non censita", w.Nome, ind)
			}
			caselle = append(caselle, id)
		}
		// Un token scritto nel file vince a ogni avvio: la configurazione è la verità. Un token VUOTO
		// significa «il segreto lo genera la pagina Postazioni» (voce 2.4, D22), e allora il seed non
		// deve sovrascriverlo al riavvio successivo — altrimenti il pacchetto scaricato ieri
		// smetterebbe di funzionare stanotte, senza che nessuno abbia toccato niente.
		if strings.TrimSpace(w.Token) != "" {
			if _, err := q.UpsertWorkerCredenziale(ctx, db.UpsertWorkerCredenzialeParams{
				WorkerNome:   w.Nome,
				WorkerTipo:   tipo,
				TokenHash:    HashToken(w.Token),
				PostazioneID: post,
				Caselle:      caselle,
			}); err != nil {
				return e, fmt.Errorf("fondazioni: worker %s: %w", w.Nome, err)
			}
		} else {
			// Alla prima creazione serve comunque un hash, e dev'essere uno che nessuno può presentare:
			// la credenziale esiste e dice quali caselle serve, ma non autentica finché il token non
			// viene generato dalla pagina. Il segnaposto è riconoscibile di proposito (rete.ImprontaNonGenerata):
			// è così che la pagina Postazioni può dire «questa credenziale è da generare» invece di
			// lasciarlo scoprire al primo 401 del worker.
			if _, err := q.UpsertWorkerCredenzialeMantieniToken(ctx, db.UpsertWorkerCredenzialeMantieniTokenParams{
				WorkerNome:   w.Nome,
				WorkerTipo:   tipo,
				TokenHash:    rete.ImprontaNonGenerata,
				PostazioneID: post,
				Caselle:      caselle,
			}); err != nil {
				return e, fmt.Errorf("fondazioni: worker %s: %w", w.Nome, err)
			}
			if log != nil {
				log.Info("worker senza token in cockpit.toml: il segreto si genera dalla pagina Postazioni", "worker", w.Nome)
			}
		}
		e.Worker++
	}

	// ---------------------------------------------------------------- casella predefinita
	if cfg.Outlook.CasellaDefault != "" {
		id, ok := idCasella[cfg.Outlook.CasellaDefault]
		if !ok {
			return e, fmt.Errorf("fondazioni: [outlook].casella_default %s non censita", cfg.Outlook.CasellaDefault)
		}
		e.CasellaDefault = uuid.NullUUID{UUID: id, Valid: true}
	}

	// ---------------------------------------------------------------- righe orfane: solo avviso
	inDB, err := q.ListCaselle(ctx)
	if err != nil {
		return e, err
	}
	for _, k := range inDB {
		if _, presente := idCasella[k.Indirizzo]; !presente && k.Attiva {
			e.NonPiuNelFile = append(e.NonPiuNelFile, "casella "+k.Indirizzo)
		}
	}
	credenziali, err := q.ListWorkerCredenziali(ctx)
	if err != nil {
		return e, err
	}
	nelFile := map[string]bool{}
	for _, w := range cfg.Worker {
		nelFile[w.Nome] = true
	}
	for _, w := range credenziali {
		if !nelFile[w.WorkerNome] && w.Attivo {
			e.NonPiuNelFile = append(e.NonPiuNelFile, "worker "+w.WorkerNome)
		}
	}

	// Chi non potrà entrare, detto adesso e non al primo 401. Il caso che conta è la migrazione dal
	// token condiviso: svuotare i `token` in cockpit.toml è la mossa giusta, ma da sola non cambia
	// niente in database — le impronte duplicate restano, e i worker continuano a ricevere 401.
	stato := StatoCredenziali(credenziali)
	for _, w := range credenziali {
		if !w.Attivo {
			continue
		}
		if d := stato[w.WorkerNome]; !d.Utilizzabile() {
			e.CredenzialiDaGenerare = append(e.CredenzialiDaGenerare, w.WorkerNome)
			if log != nil {
				switch d.Stato {
				case CredenzialeCondivisa:
					log.Warn("credenziale NON individuale: questo worker riceverà 401 finché non si rigenera il pacchetto dalla pagina Postazioni",
						"worker", w.WorkerNome, "stesso_token_di", strings.Join(d.ConChi, ", "))
				default:
					log.Warn("credenziale senza segreto: il worker non può collegarsi finché non si genera il pacchetto dalla pagina Postazioni",
						"worker", w.WorkerNome)
				}
			}
		}
	}

	if log != nil {
		log.Info("fondazioni", "caselle", e.Caselle, "postazioni", e.Postazioni, "worker", e.Worker,
			"casella_default", cfg.Outlook.CasellaDefault)
		for _, r := range e.NonPiuNelFile {
			log.Warn("in DB ma non più in cockpit.toml: lasciata attiva, disattivare a mano se voluto", "riga", r)
		}
		for _, u := range e.UtentiMancanti {
			log.Warn("sigla utente citata in cockpit.toml e assente in [[utenti]]", "utente", u)
		}
	}
	return e, nil
}

// ---------------------------------------------------------------- stato delle credenziali (voce 2.4)

// StatoCredenziale dice se una credenziale può identificare qualcuno.
//
// Serve perché fra «c'è una riga in worker_credenziale» e «quel worker può lavorare» ci sono due
// stati intermedi, e finora si scoprivano tutti e due allo stesso modo: un 401 al primo claim, con un
// motivo che parla del token e non di ciò che va fatto.
type StatoCredenziale string

const (
	// CredenzialeOk: un segreto suo, diverso da quello di tutti gli altri.
	CredenzialeOk StatoCredenziale = "ok"
	// CredenzialeDaGenerare: censita in cockpit.toml con `token` vuoto e mai generata dalla pagina
	// Postazioni. Esiste, dice quali caselle serve, e non autentica nessuno.
	CredenzialeDaGenerare StatoCredenziale = "da_generare"
	// CredenzialeCondivisa: lo stesso segreto di un altro worker. È lo stato in cui si trova chi
	// arriva dal token condiviso di prima della voce 2.4: il seed non riscrive i token che non sono
	// nel file, quindi le due impronte uguali restano in database anche dopo aver svuotato i `token`,
	// e tutti e due i worker prendono 401 — «questo token è di più worker».
	CredenzialeCondivisa StatoCredenziale = "condivisa"
)

// DiagnosiCredenziale è lo stato di una credenziale con, quando serve, chi le sta rubando l'identità.
type DiagnosiCredenziale struct {
	Stato  StatoCredenziale
	ConChi []string // gli altri worker che hanno lo stesso segreto (solo per CredenzialeCondivisa)
}

// Utilizzabile: questa credenziale può far entrare qualcuno.
func (d DiagnosiCredenziale) Utilizzabile() bool { return d.Stato == CredenzialeOk }

// StatoCredenziali guarda le credenziali in blocco e dice quali non funzioneranno, PRIMA che un
// worker ci sbatta contro.
//
// È una funzione pura sulle righe già lette: la stessa risposta la usano l'avvio del server (che la
// scrive nel log) e la pagina Postazioni (che ci mette il pulsante accanto). Le credenziali
// disattivate non contano: non autenticano comunque, e non tolgono l'identità a nessuno.
func StatoCredenziali(righe []db.WorkerCredenziale) map[string]DiagnosiCredenziale {
	perImpronta := map[string][]string{}
	for _, c := range righe {
		if !c.Attivo || c.TokenHash == rete.ImprontaNonGenerata {
			continue
		}
		perImpronta[c.TokenHash] = append(perImpronta[c.TokenHash], c.WorkerNome)
	}
	out := make(map[string]DiagnosiCredenziale, len(righe))
	for _, c := range righe {
		switch {
		case c.TokenHash == rete.ImprontaNonGenerata:
			out[c.WorkerNome] = DiagnosiCredenziale{Stato: CredenzialeDaGenerare}
		case len(perImpronta[c.TokenHash]) > 1:
			altri := make([]string, 0, len(perImpronta[c.TokenHash])-1)
			for _, n := range perImpronta[c.TokenHash] {
				if n != c.WorkerNome {
					altri = append(altri, n)
				}
			}
			out[c.WorkerNome] = DiagnosiCredenziale{Stato: CredenzialeCondivisa, ConChi: altri}
		default:
			out[c.WorkerNome] = DiagnosiCredenziale{Stato: CredenzialeOk}
		}
	}
	return out
}

// CasellaPerIndirizzo risolve un indirizzo (case-insensitive) in casella_id.
func CasellaPerIndirizzo(ctx context.Context, q *db.Queries, canale, indirizzo string) (uuid.UUID, error) {
	c := db.Canale(canale)
	if !c.Valid() {
		return uuid.Nil, fmt.Errorf("canale %q non valido", canale)
	}
	riga, err := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: c, Indirizzo: strings.ToLower(strings.TrimSpace(indirizzo))})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("casella %s non censita", indirizzo)
	}
	if err != nil {
		return uuid.Nil, err
	}
	return riga.CasellaID, nil
}

// VersioneCaselleMultiple è la versione di schema a partire dalla quale il cursore di sincronizzazione
// è per (casella, cartella) e non più per sola cartella: la 0004 della fase 2.
const VersioneCaselleMultiple = 4

// UnaSolaCasellaAttiva rifiuta l'avvio con più di una casella attiva finché lo schema non è arrivato
// alla 0004.
//
// Il motivo è preciso e vale la pena scriverlo per esteso, perché il danno sarebbe silenzioso. Alla
// versione 3 `sync_cursore` ha per chiave la sola CARTELLA. Il lotto porta già `casella_id` e tutto il
// resto è pronto per più caselle, ma il cursore no: due caselle che hanno entrambe una cartella
// «Inbox» scriverebbero sulla stessa riga. Ognuna farebbe avanzare il cursore dell'altra, e ogni
// avanzamento di troppo è una finestra di tempo che la seconda casella non leggerà mai — messaggi mai
// acquisiti, senza nessun errore da nessuna parte, e senza che nessuno se ne accorga finché qualcuno
// non cerca una RFQ che non c'è.
//
// Meglio non partire. È una condizione temporanea, verificata a ogni avvio: applicata la 0004, il
// controllo smette da solo di intervenire.
func UnaSolaCasellaAttiva(ctx context.Context, q *db.Queries, versioneSchema int) error {
	if versioneSchema >= VersioneCaselleMultiple {
		return nil
	}
	attive, err := q.ListCaselleAttive(ctx)
	if err != nil {
		return err
	}
	if len(attive) <= 1 {
		return nil
	}
	nomi := make([]string, 0, len(attive))
	for _, c := range attive {
		nomi = append(nomi, c.Indirizzo)
	}
	return fmt.Errorf(
		"%d caselle attive (%s) ma lo schema è alla versione %d: il cursore di sincronizzazione è ancora "+
			"per sola cartella, quindi due caselle si sovrascriverebbero il cursore a vicenda e perderebbero "+
			"messaggi in silenzio. Disattivarne tutte tranne una, oppure applicare la migrazione %04d (fase 2)",
		len(attive), strings.Join(nomi, ", "), versioneSchema, VersioneCaselleMultiple)
}
