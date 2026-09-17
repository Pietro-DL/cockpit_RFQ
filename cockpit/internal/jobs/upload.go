package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/nas"
)

// Upload degli allegati legato al tentativo (voce 2.3, D8, N45).
//
// Fino alla fase 1 il worker salvava l'allegato direttamente nella cartella di staging del server e
// nel result dichiarava il percorso. Funzionava per un solo motivo: worker e server erano sullo
// stesso PC. Con il worker su un'altra postazione quel percorso è su un altro disco, e il server non
// ha niente in mano.
//
// Ora il file viaggia con PUT /api/v1/allegati/{id}/file, e viaggia DENTRO il tentativo: la richiesta
// porta job_id e lease_token, il server lo scrive in <staging>\_parti, e solo il result valido dello
// stesso tentativo lo promuove a contenuto definitivo, dopo aver verificato lo sha256.
// Il token nel nome del file non è un dettaglio: è ciò che impedisce a un tentativo scaduto, che
// finisce di caricare in ritardo, di consegnare il file al posto del tentativo che gli è subentrato
// (M13). Due tentativi scrivono due file diversi; a diventare definitivo è quello del result che
// passa il predicato di validità, e l'altro viene rimosso.

// LO STAGING E' ORGANIZZATO PER CONTENUTO, NON PER MESSAGGIO (blocco 4A).
//
// Due cartelle sole, e la differenza fra le due è la differenza fra «sta arrivando» e «c'è»:
//
//	<staging>\_parti\<allegato>.parte.<lease_token>   un trasferimento in corso. Nessuno lo legge,
//	                                                   e se il tentativo non lo conclude sparisce;
//	<staging>\_contenuti\<ab>\<sha256>.<ext>          un contenuto verificato. Il nome E' l'hash.
//
// Prima ogni messaggio aveva la sua cartella — l'hash breve del Message-ID — e dentro ci finivano i
// suoi allegati. Con quel disegno lo stesso disegno allegato a cinque richieste stava sul disco
// cinque volte, e uno zip da 6 MB con dentro 30 MB di file ne occupava 36 per ogni messaggio in cui
// compariva: bastavano due mail con lo stesso allegato per quadruplicare l'occupazione.
//
// La deduplica per hash in AccodaStage c'era già, ma arriva tardi per costruzione: prima di scaricare
// da Outlook lo sha256 non lo conosce nessuno, quindi due copie dello stesso file possono essere
// accodate entrambe prima che la prima finisca. Con il nome uguale al contenuto quella corsa non ha
// più un perdente: chi arriva secondo trova il file già al suo posto, con l'hash giusto, e non
// scrive niente.
//
// Un contenuto è quindi di tutti gli allegati che lo hanno: cancellarlo li lascia senza tutti
// insieme, ed è la stessa condizione — con «Riscarica» — in cui si trovava già un allegato solo.
const (
	CartellaParti     = "_parti"
	CartellaContenuti = "_contenuti"
)

// reSha256: un hash, in minuscolo, e nient'altro. Il percorso di un contenuto finisce in una
// os.Create, e questo è il punto in cui si garantisce che non possa uscire dallo staging.
var reSha256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

var reEstensione = regexp.MustCompile(`^[a-z0-9]{1,10}$`)

// PercorsoParte è il file in cui un tentativo carica. Il token nel nome non è un dettaglio: è ciò che
// impedisce a un tentativo scaduto, che finisce di caricare in ritardo, di consegnare il file al
// posto del tentativo che gli è subentrato (M13). Due tentativi scrivono due file diversi.
func PercorsoParte(staging string, allegatoID, token uuid.UUID) string {
	return filepath.Join(staging, CartellaParti, allegatoID.String()+".parte."+token.String())
}

// PercorsoContenuto è dove sta un contenuto verificato: lo dice il suo sha256, e nient'altro.
//
// I primi due caratteri fanno da sottocartella. Non è estetica: qualche decina di migliaia di file
// in una sola cartella su NTFS rende lenta ogni enumerazione, compresa quella della pulizia.
//
// L'estensione viene dal NOME dell'allegato e serve solo a chi guarda la cartella con Esplora
// risorse: l'analizzatore decide il tipo dal nome del file che riceve nel payload, mai dal percorso.
// Un'estensione strana diventa .bin, perché qui dentro non deve poter comparire niente che non sia
// stato scelto da noi.
func PercorsoContenuto(staging, sha256, nomeFile string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(sha256))
	if !reSha256.MatchString(h) {
		return "", fmt.Errorf("%w: %q", ErrSha256, sha256)
	}
	return filepath.Join(staging, CartellaContenuti, h[:2], h+estensioneDi(nomeFile)), nil
}

// ErrSha256: si è chiesto il percorso di un contenuto senza avere un hash. Non può succedere per un
// file verificato; se succede, il posto giusto per accorgersene è prima di scrivere sul disco.
var ErrSha256 = errors.New("sha256 non valido")

func estensioneDi(nomeFile string) string {
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(nomeFile)), "."))
	if !reEstensione.MatchString(e) {
		return ".bin"
	}
	return "." + e
}

// ContenutoGiaPresente dice se quel contenuto è già nello staging, verificandone l'hash.
//
// L'hash si ricalcola invece di fidarsi del nome: un file rimasto a metà da una versione precedente,
// o corrotto sul disco, porterebbe il nome giusto e il contenuto sbagliato, e verrebbe copiato sul
// NAS al posto del disegno vero. Costa una lettura del file, e la si fa fuori dalla transazione.
func ContenutoGiaPresente(percorso, sha256 string) bool {
	if st, err := os.Stat(percorso); err != nil || st.IsDir() {
		return false
	}
	h, _, err := nas.Sha256File(percorso)
	return err == nil && strings.EqualFold(h, sha256)
}

var reParte = regexp.MustCompile(`\.parte\.([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// TokenDiParte riconosce un file .parte.<token> e ne restituisce il token.
func TokenDiParte(nome string) (uuid.UUID, bool) {
	m := reParte.FindStringSubmatch(strings.ToLower(nome))
	if m == nil {
		return uuid.UUID{}, false
	}
	t, err := uuid.Parse(m[1])
	return t, err == nil
}

// Promuovi rende definitivo il file caricato da un tentativo: rinomina atomica di parte su
// definitivo. Se definitivo esiste già viene sostituito, e dal blocco 4A questo non è più nemmeno un
// caso interessante: il nome è l'hash, quindi il file che c'era ha lo stesso contenuto di questo.
//
// Va chiamata DENTRO la transazione del result, dopo BloccaTentativo: la riga del job è bloccata,
// quindi nessun altro tentativo può diventare valido mentre il file cambia nome. Se il commit poi
// fallisce, il file definitivo resta con il contenuto giusto e il tentativo successivo lo ricarica
// e lo ripromuove tale e quale: l'operazione è ripetibile (M7).
func Promuovi(parte, definitivo string) error {
	if err := os.Rename(parte, definitivo); err != nil {
		return fmt.Errorf("promozione di %s: %w", filepath.Base(parte), err)
	}
	return nil
}

// RimuoviParte toglie il file di un tentativo che non vale più. Un file assente non è un errore.
func RimuoviParte(parte string) {
	if parte == "" {
		return
	}
	if err := os.Remove(parte); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("file parziale non rimosso", "file", parte, "err", err)
	}
}

// PulisciParti rimuove dallo staging i .parte.<token> dei tentativi che non sono più in corso (N8,
// voce 2.3). Sono i file di upload interrotti a metà, di tentativi scaduti durante il trasferimento,
// o di result mai arrivati: nessuno li promuoverà più.
//
// Un file il cui token è ancora in corso non si tocca, qualunque età abbia: un upload da centinaia
// di megabyte su una LAN lenta è ancora un upload. E un file orfano più giovane di etaMinima si
// lascia stare per un giro: fra la fine dell'upload e il result c'è una finestra in cui il token è
// ancora in corso, e non vale la pena giocare sul filo dei secondi.
func PulisciParti(ctx context.Context, q *db.Queries, staging string, etaMinima time.Duration, log *slog.Logger) (int, error) {
	if strings.TrimSpace(staging) == "" {
		return 0, nil
	}
	token, err := q.ListLeaseTokenInCorso(ctx)
	if err != nil {
		return 0, err
	}
	vivi := map[uuid.UUID]bool{}
	for _, t := range token {
		if t.Valid {
			vivi[t.UUID] = true
		}
	}
	limite := time.Now().Add(-etaMinima)
	rimossi := 0
	rimossi += pulisciZipInterrotti(staging, vivi, limite, log)
	err = filepath.WalkDir(staging, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // una cartella illeggibile non ferma la pulizia delle altre
		}
		t, ok := TokenDiParte(d.Name())
		if !ok || vivi[t] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(limite) {
			return nil
		}
		if err := os.Remove(p); err != nil {
			log.Warn("file parziale orfano non rimosso", "file", p, "err", err)
			return nil
		}
		rimossi++
		log.Info("file parziale orfano rimosso", "file", p, "token", t)
		return nil
	})
	return rimossi, err
}

// PercorsoEstrazione e' la cartella temporanea in cui un tentativo scompatta un archivio.
//
// Sta sotto _parti, e con il token nel nome, per la stessa ragione del file di un upload: e' roba di
// QUESTO tentativo, non deve confondersi con quella di un altro che sta estraendo lo stesso
// archivio, e se il tentativo non arriva in fondo la pulizia sa di chi era e puo' portarla via.
func PercorsoEstrazione(staging string, token uuid.UUID) string {
	return filepath.Join(staging, CartellaParti, "zip."+token.String())
}

// pulisciZipInterrotti toglie le cartelle di estrazione rimaste da tentativi che non esistono piu'.
//
// Le voci di un archivio si scompattano in una cartella temporanea e da li' si spostano fra i
// contenuti, perche' il nome definitivo di una voce e' il suo sha256 e lo sha256 si conosce dopo
// averla scritta. Se il processo muore in mezzo, quella cartella resta: non e' un file .parte e la
// camminata qui sopra non la vedrebbe mai.
func pulisciZipInterrotti(staging string, vivi map[uuid.UUID]bool, limite time.Time, log *slog.Logger) int {
	voci, err := os.ReadDir(filepath.Join(staging, CartellaParti))
	if err != nil {
		return 0
	}
	rimosse := 0
	for _, d := range voci {
		if !d.IsDir() || !strings.HasPrefix(d.Name(), "zip.") {
			continue
		}
		t, err := uuid.Parse(strings.TrimPrefix(d.Name(), "zip."))
		if err != nil || vivi[t] {
			continue
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(limite) {
			continue
		}
		p := filepath.Join(staging, CartellaParti, d.Name())
		if err := os.RemoveAll(p); err != nil {
			log.Warn("estrazione interrotta non rimossa", "cartella", p, "err", err)
			continue
		}
		rimosse++
		log.Info("estrazione interrotta rimossa dallo staging", "cartella", p, "token", t)
	}
	return rimosse
}

// PulisciContenuti toglie dallo staging i contenuti che non servono piu' a nessuno (blocco 4A).
//
// Un contenuto serve finche' ESISTE UN ALLEGATO CON QUEL SHA256. Non si guarda il percorso: il nome
// del file e' l'hash, quindi l'hash basta, e su Windows due percorsi che differiscono per maiuscole
// o separatori indicano lo stesso file — un confronto per stringa cancellerebbe il disegno che stava
// per essere copiato sul NAS.
//
// Restano fuori DUE cose, di proposito:
//
//   - i file piu' giovani di etaMinima. Fra la rinomina di un contenuto e il COMMIT che scrive
//     l'allegato passa un istante in cui quel contenuto non e' nominato da nessuno: senza questa
//     attesa la pulizia lo cancellerebbe proprio mentre sta nascendo;
//   - tutto cio' che non sta sotto _contenuti. Lo staging di prima del blocco 4A e' organizzato per
//     messaggio, con i nomi veri dei file: li' un nome non dice niente sul contenuto, e la pulizia
//     non ha modo di sapere che cosa sia ancora in uso. Quei file si tolgono a mano.
//
// etaMinima e giorni sono due soglie diverse e servono a due cose diverse: la prima protegge il
// contenuto appena nato, la seconda e' la retention vera («da quanto non lo tocca nessuno»).
// Giorni a zero = non cancellare niente, ed e' il valore predefinito.
func PulisciContenuti(ctx context.Context, q *db.Queries, staging string, giorni int, etaMinima time.Duration, log *slog.Logger) (int, int64, error) {
	if strings.TrimSpace(staging) == "" || giorni <= 0 {
		return 0, 0, nil
	}
	radice := filepath.Join(staging, CartellaContenuti)
	if st, err := os.Stat(radice); err != nil || !st.IsDir() {
		return 0, 0, nil
	}
	limite := time.Now().Add(-etaMinima)
	if scadenza := time.Now().AddDate(0, 0, -giorni); scadenza.Before(limite) {
		limite = scadenza
	}

	// Prima si raccoglie, poi si chiede, poi si cancella. Chiedere al database un file alla volta
	// mentre si cammina sul disco vorrebbe dire una query per contenuto: su qualche decina di
	// migliaia di file e' un quarto d'ora di interrogazioni per non cancellare niente.
	type candidato struct {
		percorso string
		sha      string
		bytes    int64
	}
	var candidati []candidato
	err := filepath.WalkDir(radice, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // una cartella illeggibile non ferma la pulizia delle altre
		}
		sha := strings.ToLower(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())))
		if !reSha256.MatchString(sha) {
			return nil // non l'abbiamo scritto noi: non lo cancelliamo noi
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(limite) {
			return nil
		}
		candidati = append(candidati, candidato{p, sha, info.Size()})
		return nil
	})
	if err != nil || len(candidati) == 0 {
		return 0, 0, err
	}
	sha := make([]string, 0, len(candidati))
	for _, c := range candidati {
		sha = append(sha, c.sha)
	}
	usati, err := q.ShaAncoraUsati(ctx, sha)
	if err != nil {
		return 0, 0, err
	}
	vivi := map[string]bool{}
	for _, u := range usati {
		if u.Valid {
			vivi[strings.ToLower(strings.TrimSpace(u.String))] = true
		}
	}
	rimossi, liberati := 0, int64(0)
	for _, c := range candidati {
		if vivi[c.sha] {
			continue
		}
		if err := os.Remove(c.percorso); err != nil {
			log.Warn("contenuto non rimosso dallo staging", "file", c.percorso, "err", err)
			continue
		}
		rimossi++
		liberati += c.bytes
		log.Info("contenuto rimosso dallo staging: nessun allegato lo nomina piu'", "sha", c.sha[:12], "byte", c.bytes)
	}
	return rimossi, liberati, nil
}
