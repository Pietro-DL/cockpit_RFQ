// Package config legge cockpit.toml (accanto all'exe o passato con -config).
package config

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server     Server       `toml:"server"`
	DB         DB           `toml:"db"`
	NAS        NAS          `toml:"nas"`
	Outlook    Outlook      `toml:"outlook"`
	Utenti     []Utente     `toml:"utenti"`
	Caselle    []Casella    `toml:"casella"`    // [[casella]] — fase 0, voce 0.6
	Postazioni []Postazione `toml:"postazione"` // [[postazione]]
	Worker     []Worker     `toml:"worker"`     // [[worker]] — credenziali individuali
	Retention  Retention    `toml:"retention"`  // [retention] — fase 1, voce 1.6
	Analisi    Analisi      `toml:"analisi"`    // [analisi] — fase 1, voce 1.12
}

// Retention: per quanto si tengono i job chiusi. Una coda che non si svuota mai diventa illeggibile e
// rallenta le interrogazioni di amministrazione; 0 = non cancellare nulla.
type Retention struct {
	GiorniJob int `toml:"giorni_job"`
}

// Analisi identifica CON CHE COSA un file è stato analizzato (voce 1.12, A15).
//
// I fatti estratti da un file dipendono da tre cose e solo da tre: il contenuto del file, la versione
// dell'analizzatore e la configurazione che gli è stata data. A parità di tutte e tre, rifare
// l'analisi è lavoro sprecato e il risultato è per definizione lo stesso; se cambia una delle tre, il
// risultato precedente non vale più. Senza queste due informazioni il server non può distinguere i
// due casi, e finisce per fare sempre la scelta sbagliata: o rianalizza tutto ogni volta, o riusa
// fatti calcolati con un dizionario che non è più quello.
//
// Parametri è il blocco che viene mandato al worker; il suo sha256, insieme alla versione, è la
// chiave con cui i fatti vengono conservati e ritrovati. Cambiare un termine qui basta a far
// rianalizzare tutto, senza toccare il codice.
type Analisi struct {
	Versione  int            `toml:"versione"`
	Parametri map[string]any `toml:"parametri"`
}

type Server struct {
	Indirizzo string `toml:"indirizzo"` // es. "127.0.0.1:8080" in sviluppo, "0.0.0.0:8443" in LAN
	// TokenWorker era il token CONDIVISO da tutti i worker. Dalla voce 2.4 non autentica più niente:
	// la credenziale è individuale ([[worker]].token, sha256 in `worker_credenziale`). Resta letto
	// per un motivo solo — dirlo a chi ce l'ha ancora nel file, invece di lasciarlo credere che
	// protegga qualcosa.
	TokenWorker     string `toml:"token_worker"`
	SegretoSessione string `toml:"segreto_sessione"` // riservato a usi futuri (firma cookie); le sessioni vivono nel DB
	LogLivello      string `toml:"log_livello"`      // debug | info | warn
	// TLSCert e TLSKey: i due file PEM del listener (voce 2.4). Vuoti = niente TLS, e allora il
	// server accetta solo di ascoltare su loopback. Percorsi relativi al file di configurazione.
	//
	// Se i due file non esistono il server ne genera uno **autofirmato** e li scrive: su una LAN
	// aziendale senza CA interna un certificato autofirmato con l'impronta dichiarata nel worker.toml
	// è più forte di una catena che nessuno verifica, e infinitamente più forte del testo in chiaro.
	// L'impronta sha256 viene scritta nel log all'avvio e mostrata nella pagina *Postazioni*.
	TLSCert string `toml:"tls_cert"`
	TLSKey  string `toml:"tls_key"`
	// TLSNomi sono i nomi e gli indirizzi per cui vale il certificato generato (SAN): il nome host
	// della VM, il suo IP, gli alias con cui i worker la chiamano. Vuoto = nome host del server e
	// l'indirizzo su cui ascolta, se è un IP.
	TLSNomi []string `toml:"tls_nomi"`
	// ConsentiLanInChiaro è la via d'uscita dichiarata, non il default: senza di lei il server si
	// rifiuta di ascoltare fuori da loopback senza TLS. Un server in chiaro sulla LAN espone la posta
	// dell'azienda e i token dei worker a chiunque sia attaccato allo stesso switch, e non se ne
	// accorge nessuno: è esattamente il tipo di difetto che non dà errore.
	ConsentiLanInChiaro bool `toml:"consenti_lan_in_chiaro"`
	// LogFile: dove il server scrive il proprio log, oltre che sullo stdout della finestra.
	// Vuoto = <nas.staging>\log\cockpit.log; "-" = solo stdout, nessun file.
	LogFile string `toml:"log_file"`
	// Modalita è come gira il server: "shadow" oppure "produzione" (§2.7, D16, voce 9.5).
	//
	// In shadow il Cockpit LEGGE il mondo e non lo tocca: niente bozze, niente «segna letto», niente
	// spostamenti di cartella, niente scritture sul NAS. Serve a far girare il sistema sulla posta
	// vera senza che un difetto si trasformi in una mail modificata o in un file scritto dove non
	// doveva. `dry_run` viene forzato a true e l'unica azione Outlook che resta è «Apri».
	//
	// Vuoto = shadow. È il default sicuro: un cockpit.toml scritto prima che questa opzione
	// esistesse non contiene la riga, e fra le due letture possibili di un silenzio — «non scrivere
	// niente» e «scrivi pure sul NAS di produzione» — solo una si può correggere dopo.
	Modalita string `toml:"modalita"`
	// MaxUploadMB è il limite di un singolo allegato caricato dal worker con
	// PUT /api/v1/allegati/{id}/file (voce 2.3). Oltre, il server risponde 413 prima di leggere il
	// corpo e l'allegato va in errore con il motivo visibile; senza un limite un allegato da qualche
	// gigabyte riempirebbe lo staging del server in silenzio. Zero = il default (64).
	MaxUploadMB int `toml:"max_upload_mb"`
}

type DB struct {
	DSN string `toml:"dsn"`
}

// PercorsoLog dice dove va il log del server; "" significa «solo sullo stdout».
//
// Il default non è una cartella qualsiasi: è la stessa <staging>\log dove scrivono i worker, perché
// la diagnosi di un job si fa mettendo le due metà una accanto all'altra — chi ha chiesto che cosa e
// che cosa ha risposto il server — e cercarle in due posti diversi è metà del lavoro.
func (c *Config) PercorsoLog() string {
	switch {
	case c.Server.LogFile == "-":
		return ""
	case c.Server.LogFile != "":
		return c.Server.LogFile
	case c.NAS.Staging != "":
		return filepath.Join(c.NAS.Staging, "log", "cockpit.log")
	default:
		return ""
	}
}

type NAS struct {
	Radice  string `toml:"radice"`  // \nas01\TECNICO - PREVENTIVI\PREVENTIVI DA FARE  (in sviluppo: cartella locale)
	DryRun  bool   `toml:"dry_run"` // true = nessuna scrittura reale, solo log
	Staging string `toml:"staging"` // cartella locale dove il worker-outlook salva gli allegati
	// RadiciProduzione sono le radici VERE, dichiarate una volta (elenco di percorsi UNC).
	//
	// Servono a una cosa sola: impedire che una prova in shadow parta puntata sul NAS di produzione
	// (§2.7). Un `dry_run` dimenticato a false e una radice copia-incollata dal file di produzione
	// sono due errori di battitura che insieme scrivono nel fascicolo di un cliente; dichiarare qui
	// le radici vere trasforma quella combinazione in un errore all'avvio, che si legge.
	RadiciProduzione []string `toml:"radici_produzione"`
}

// Modalità del server (§2.7). Non sono stringhe libere: un valore scritto male non deve poter
// significare «produzione» per distrazione.
const (
	ModalitaShadow     = "shadow"
	ModalitaProduzione = "produzione"
)

// EShadow dice se il server gira in sola lettura verso il mondo.
func (c *Config) EShadow() bool { return c.Server.Modalita == ModalitaShadow }

// RadiceDiProduzione restituisce la radice dichiarata di produzione che contiene (o coincide con)
// `nas.radice`, oppure "" se non ce n'è nessuna. Il confronto ignora maiuscole, separatori e barre
// finali, perché sono le tre cose che cambiano fra un file e l'altro senza cambiare la cartella.
func (c *Config) RadiceDiProduzione() string {
	r := normalizzaPercorso(c.NAS.Radice)
	if r == "" {
		return ""
	}
	for _, grezza := range c.RadiciProduzione() {
		p := normalizzaPercorso(grezza)
		if p == "" {
			continue
		}
		if r == p || strings.HasPrefix(r, p+`\`) {
			return grezza
		}
	}
	return ""
}

// RadiciProduzione è l'elenco dichiarato, senza voci vuote.
func (c *Config) RadiciProduzione() []string {
	var out []string
	for _, r := range c.NAS.RadiciProduzione {
		if strings.TrimSpace(r) != "" {
			out = append(out, strings.TrimSpace(r))
		}
	}
	return out
}

func normalizzaPercorso(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "/", `\`)
	for len(p) > 1 && strings.HasSuffix(p, `\`) {
		p = strings.TrimSuffix(p, `\`)
	}
	return strings.ToLower(p)
}

type Outlook struct {
	Cartelle        []string `toml:"cartelle"`          // es. ["Inbox", "Sent Items"]
	IntervalloSyncS int      `toml:"intervallo_sync_s"` // ogni quanti secondi accodare sync_outlook
	Dal             string   `toml:"dal"`               // "2026-09-01": data minima al primo avvio (cursore vuoto)
	Lotto           int      `toml:"lotto"`             // messaggi per POST ingest
	ConsentiInvio   bool     `toml:"consenti_invio"`    // false = solo bozze (regola aziendale)
	CasellaDefault  string   `toml:"casella_default"`   // indirizzo della casella attribuita ai messaggi che non la dichiarano (fase 1)
}

// Casella è una voce [[casella]]: una casella di posta censita, personale o condivisa.
type Casella struct {
	Indirizzo string `toml:"indirizzo"` // 'nome.cognome@azienda.it' (normalizzato in minuscolo)
	Nome      string `toml:"nome"`      // etichetta in UI: 'Francesco'
	Canale    string `toml:"canale"`    // default 'outlook'
	Condivisa bool   `toml:"condivisa"` // true = cassetta condivisa Exchange (nessun proprietario)
	Utente    string `toml:"utente"`    // sigla del proprietario; vuoto o ignorato se condivisa
	// Attiva: assente = true. Serve a dichiarare nel file quale casella è in uso, invece di doverlo
	// fare a mano in database a ogni avvio. Finché lo schema è alla 0003 ne può essere attiva una sola.
	Attiva *bool `toml:"attiva"`
}

// EAttiva risolve il default: una casella senza `attiva` nel file è attiva.
func (c Casella) EAttiva() bool { return c.Attiva == nil || *c.Attiva }

// Postazione è una voce [[postazione]]: un PC con Outlook e un worker.
type Postazione struct {
	NomeHost    string `toml:"nome_host"` // 'PC-FRANCESCO' (normalizzato in maiuscolo)
	Descrizione string `toml:"descrizione"`
	Utente      string `toml:"utente"` // sigla di chi ci lavora
}

// Worker è una voce [[worker]]: credenziale individuale e caselle autorizzate.
// Il token sta solo qui: nel DB finisce il suo sha256.
type Worker struct {
	Nome       string   `toml:"nome"`       // 'outlook@PC-FRANCESCO'
	Tipo       string   `toml:"tipo"`       // outlook | analisi
	Token      string   `toml:"token"`      // segreto individuale
	Postazione string   `toml:"postazione"` // nome host
	Caselle    []string `toml:"caselle"`    // indirizzi autorizzati
}

type Utente struct {
	Sigla    string `toml:"sigla"`
	Nome     string `toml:"nome"`
	Ufficio  string `toml:"ufficio"`
	Ruolo    string `toml:"ruolo"`
	Password string `toml:"password"` // solo per il seed iniziale; nel DB va l'hash bcrypt
}

func Carica(percorso string) (*Config, error) {
	c := &Config{}
	c.Server.Indirizzo = "127.0.0.1:8080"
	c.Server.LogLivello = "info"
	c.Server.MaxUploadMB = 64
	c.Server.Modalita = ModalitaShadow
	c.Outlook.Cartelle = []string{"Inbox", "Sent Items"}
	c.Outlook.IntervalloSyncS = 60
	c.Outlook.Lotto = 50
	c.Retention.GiorniJob = 30
	c.Analisi.Versione = 1
	if _, err := toml.DecodeFile(percorso, c); err != nil {
		return nil, fmt.Errorf("config %s: %w", percorso, err)
	}
	if c.DB.DSN == "" {
		return nil, fmt.Errorf("config: [db].dsn mancante")
	}
	if err := c.normalizzaRete(filepath.Dir(percorso)); err != nil {
		return nil, err
	}
	if c.Server.MaxUploadMB < 1 {
		return nil, fmt.Errorf("config: [server].max_upload_mb = %d non valido (almeno 1)", c.Server.MaxUploadMB)
	}
	if err := c.normalizzaModalita(); err != nil {
		return nil, err
	}
	if c.NAS.Staging == "" {
		c.NAS.Staging = filepath.Join(filepath.Dir(percorso), "staging")
	}
	if err := os.MkdirAll(c.NAS.Staging, 0o755); err != nil {
		return nil, fmt.Errorf("staging %s: %w", c.NAS.Staging, err)
	}
	if err := c.normalizzaFondazioni(); err != nil {
		return nil, err
	}
	return c, nil
}

// normalizzaModalita valida [server].modalita e ne applica le conseguenze (§2.7, voce 9.5).
//
// Due conseguenze, e sono entrambe rifiuti: in shadow il server non parte se la radice del NAS è una
// radice di produzione dichiarata, e `dry_run` viene forzato a true qualunque cosa dica il file. Un
// avvio in shadow che scrive sul NAS vero non è una shadow: è una prova sulla produzione con un
// badge rassicurante in testata.
func (c *Config) normalizzaModalita() error {
	c.Server.Modalita = strings.ToLower(strings.TrimSpace(c.Server.Modalita))
	if c.Server.Modalita == "" {
		c.Server.Modalita = ModalitaShadow
	}
	if c.Server.Modalita != ModalitaShadow && c.Server.Modalita != ModalitaProduzione {
		return fmt.Errorf("config: [server].modalita = %q non valida (%s | %s)", c.Server.Modalita, ModalitaShadow, ModalitaProduzione)
	}
	if !c.EShadow() {
		return nil
	}
	if r := c.RadiceDiProduzione(); r != "" {
		return fmt.Errorf("config: [server].modalita = shadow ma [nas].radice (%s) è sotto la radice di produzione %q dichiarata in [nas].radici_produzione: "+
			"una prova in shadow non si fa sul NAS vero. Cambiare radice, oppure passare a modalita = \"produzione\" se è ciò che si vuole davvero",
			c.NAS.Radice, r)
	}
	c.NAS.DryRun = true
	return nil
}

// ETLS dice se il listener parla TLS: servono tutti e due i file, perché mezzo TLS non esiste.
func (c *Config) ETLS() bool {
	return strings.TrimSpace(c.Server.TLSCert) != "" && strings.TrimSpace(c.Server.TLSKey) != ""
}

// Schema è "https" o "http": serve a scrivere gli indirizzi (log, pagina Postazioni, worker.toml
// generato) con lo schema che il server sta davvero usando, invece di uno scritto a mano.
func (c *Config) Schema() string {
	if c.ETLS() {
		return "https"
	}
	return "http"
}

// SuLoopback dice se [server].indirizzo ascolta solo su questo PC. Un indirizzo senza host
// ("":8080) ascolta su tutte le interfacce: è LAN a tutti gli effetti.
func SuLoopback(indirizzo string) bool {
	host := strings.TrimSpace(indirizzo)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "":
		return false
	case "localhost":
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		// Un nome che non è un IP: non si può dire che sia loopback, e nel dubbio non lo è.
		return false
	}
	return ip.IsLoopback()
}

// normalizzaRete applica la voce 2.4: TLS sul listener e credenziali individuali.
//
// Due conseguenze, e sono di natura diversa.
//
//  1. Un RIFIUTO: fuori da loopback senza TLS il server non parte. Il traffico che passa di qui è la
//     posta dell'azienda, i token dei worker e il cookie di sessione dell'operatore; in chiaro su una
//     LAN li legge chiunque sia attaccato allo stesso switch, e non se ne accorge nessuno mai. Chi ha
//     davvero bisogno del chiaro (una prova, un tunnel che cifra già lui) lo dichiara con
//     `consenti_lan_in_chiaro = true`, che si legge nel file e nel log.
//  2. Un AVVISO che non ferma niente: `[server].token_worker` non autentica più. Toglierlo dal file è
//     una riga in meno; lasciarlo credendo che serva è un segreto condiviso che resta in giro.
func (c *Config) normalizzaRete(dirConfig string) error {
	c.Server.TLSCert = assoluto(dirConfig, c.Server.TLSCert)
	c.Server.TLSKey = assoluto(dirConfig, c.Server.TLSKey)
	if strings.TrimSpace(c.Server.TLSCert) != "" && strings.TrimSpace(c.Server.TLSKey) == "" {
		return fmt.Errorf("config: [server].tls_cert senza [server].tls_key (servono tutti e due)")
	}
	if strings.TrimSpace(c.Server.TLSKey) != "" && strings.TrimSpace(c.Server.TLSCert) == "" {
		return fmt.Errorf("config: [server].tls_key senza [server].tls_cert (servono tutti e due)")
	}
	if !c.ETLS() && !SuLoopback(c.Server.Indirizzo) && !c.Server.ConsentiLanInChiaro {
		return fmt.Errorf("config: [server].indirizzo = %q ascolta fuori da questo PC e [server].tls_cert non c'è. "+
			"In chiaro sulla LAN viaggiano la posta, i token dei worker e il cookie di sessione. "+
			"Indicare tls_cert/tls_key (se i file non esistono il server ne genera uno autofirmato e ne scrive l'impronta), "+
			"oppure dichiarare consenti_lan_in_chiaro = true se il collegamento è già cifrato da qualcos'altro",
			c.Server.Indirizzo)
	}
	return nil
}

// assoluto risolve un percorso relativo rispetto alla cartella del file di configurazione: i
// percorsi in un file si leggono da dove sta il file, non da dove è stato lanciato l'eseguibile.
func assoluto(dir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// normalizzaFondazioni mette in forma canonica [[casella]], [[postazione]], [[worker]] e
// [outlook].casella_default, e rifiuta i riferimenti incrociati che non esistono. Un errore qui
// blocca l'avvio: meglio non partire che partire con un routing sbagliato.
func (c *Config) normalizzaFondazioni() error {
	indirizzi := map[string]bool{}
	for i := range c.Caselle {
		k := &c.Caselle[i]
		k.Indirizzo = strings.ToLower(strings.TrimSpace(k.Indirizzo))
		if k.Indirizzo == "" {
			return fmt.Errorf("config: [[casella]] #%d senza indirizzo", i+1)
		}
		if indirizzi[k.Indirizzo] {
			return fmt.Errorf("config: casella %q dichiarata due volte", k.Indirizzo)
		}
		indirizzi[k.Indirizzo] = true
		if k.Canale == "" {
			k.Canale = "outlook"
		}
		if k.Nome == "" {
			k.Nome, _, _ = strings.Cut(k.Indirizzo, "@")
		}
		k.Utente = strings.ToUpper(strings.TrimSpace(k.Utente))
		if k.Condivisa && k.Utente != "" {
			return fmt.Errorf("config: casella %q è condivisa e non può avere un proprietario (utente = %q)", k.Indirizzo, k.Utente)
		}
	}
	host := map[string]bool{}
	for i := range c.Postazioni {
		p := &c.Postazioni[i]
		p.NomeHost = strings.ToUpper(strings.TrimSpace(p.NomeHost))
		if p.NomeHost == "" {
			return fmt.Errorf("config: [[postazione]] #%d senza nome_host", i+1)
		}
		if host[p.NomeHost] {
			return fmt.Errorf("config: postazione %q dichiarata due volte", p.NomeHost)
		}
		host[p.NomeHost] = true
		p.Utente = strings.ToUpper(strings.TrimSpace(p.Utente))
	}
	nomi := map[string]bool{}
	tokenDi := map[string]string{} // token → il primo worker che l'ha dichiarato
	for i := range c.Worker {
		w := &c.Worker[i]
		w.Nome = strings.TrimSpace(w.Nome)
		if w.Nome == "" {
			return fmt.Errorf("config: [[worker]] #%d senza nome", i+1)
		}
		if nomi[w.Nome] {
			return fmt.Errorf("config: worker %q dichiarato due volte", w.Nome)
		}
		nomi[w.Nome] = true
		if w.Tipo != "outlook" && w.Tipo != "analisi" {
			return fmt.Errorf("config: worker %q: tipo %q non valido (outlook | analisi)", w.Nome, w.Tipo)
		}
		// Un worker senza `token` non è più un errore (voce 2.4): significa «il segreto lo genera la
		// pagina Postazioni», che è la via consigliata in azienda — il pacchetto scaricato da lì
		// contiene indirizzo, impronta e token, e nessuno li copia a mano. Un token scritto qui invece
		// vince a ogni avvio. Quello che NON si può fare è darne uno solo a due worker: in quel caso
		// il server non sa chi sta chiamando, e se ne accorge al primo claim con un 401.
		if w.Token != "" {
			if altro, gia := tokenDi[w.Token]; gia {
				return fmt.Errorf("config: i worker %q e %q hanno lo stesso token: un segreto che è di due non identifica nessuno, "+
					"e il server risponderà 401 a tutti e due. Darne uno diverso a ciascuno, oppure lasciarli vuoti e generare il "+
					"pacchetto dalla pagina Postazioni", altro, w.Nome)
			}
			tokenDi[w.Token] = w.Nome
		}
		w.Postazione = strings.ToUpper(strings.TrimSpace(w.Postazione))
		if w.Postazione != "" && !host[w.Postazione] {
			return fmt.Errorf("config: worker %q: postazione %q non dichiarata in [[postazione]]", w.Nome, w.Postazione)
		}
		for j, ind := range w.Caselle {
			w.Caselle[j] = strings.ToLower(strings.TrimSpace(ind))
			if !indirizzi[w.Caselle[j]] {
				return fmt.Errorf("config: worker %q: casella %q non dichiarata in [[casella]]", w.Nome, w.Caselle[j])
			}
		}
	}
	c.Outlook.CasellaDefault = strings.ToLower(strings.TrimSpace(c.Outlook.CasellaDefault))
	switch {
	case c.Outlook.CasellaDefault != "":
		if !indirizzi[c.Outlook.CasellaDefault] {
			return fmt.Errorf("config: [outlook].casella_default = %q non è fra le [[casella]] dichiarate", c.Outlook.CasellaDefault)
		}
	case len(c.Caselle) == 1:
		c.Outlook.CasellaDefault = c.Caselle[0].Indirizzo
	case len(c.Caselle) > 1:
		return fmt.Errorf("config: con più [[casella]] serve [outlook].casella_default")
	}
	return nil
}
