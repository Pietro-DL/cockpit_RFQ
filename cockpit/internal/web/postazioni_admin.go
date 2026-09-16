package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/fondazioni"
	"promatec/cockpit/internal/rete"
)

// Pagina *Postazioni* (voce 2.4 + D22): da qui si mette in piedi un PC nuovo senza copiare segreti a
// mano da un file all'altro.
//
// Il problema che risolve. Fino al blocco 1 aggiungere una postazione voleva dire: scrivere il token
// condiviso nel worker.toml del PC nuovo (lo stesso di tutti gli altri), copiare i file del worker da
// una cartella di rete, e sperare che l'indirizzo del server fosse quello giusto. Tre passaggi a
// mano, ognuno dei quali fallisce in silenzio: un token copiato male dà 401 al primo claim, un
// indirizzo vecchio dà un worker che gira e non prende niente.
//
// Qui il pacchetto lo costruisce il server, che è l'unico a sapere tutte e tre le cose: il proprio
// indirizzo, l'impronta del proprio certificato e il segreto che sta assegnando in quel momento.
//
// Che cosa questa pagina NON fa, di proposito: non crea postazioni né worker. Le autorizzazioni —
// quale PC esiste, quale worker serve quali caselle — restano in cockpit.toml, che è versionato,
// leggibile e uguale a ogni avvio. Una schermata che crea autorizzazioni è una schermata da cui si
// può dare accesso alla posta di un collega con due clic e nessuna traccia nel file.

// credenzialeUI è un worker di una postazione come si vede nella pagina.
type credenzialeUI struct {
	Nome       string
	Tipo       string
	Attivo     bool
	Aggiornato time.Time
	Caselle    []string
	// UltimoClaim e IP vengono dalla presenza: dicono se quel worker si è mai fatto vivo, e da dove.
	UltimoClaim *time.Time
	IP          string
	OutlookOk   bool
	// Stato e Problema: se questa credenziale può far entrare qualcuno, e se no perché (voce 2.4).
	// Senza, l'unico modo di scoprire che un token è condiviso o non è mai stato generato è il primo
	// 401 del worker — che parla del token e non dice che cosa fare.
	Stato    fondazioni.StatoCredenziale
	Problema string
}

func (c credenzialeUI) Guasta() bool { return c.Stato != "" && c.Stato != fondazioni.CredenzialeOk }

type postazioneUI struct {
	NomeHost    string
	Descrizione string
	Attiva      bool
	Worker      []credenzialeUI
	// DaSistemare: almeno un worker di questo PC non può collegarsi. È ciò che trasforma il pulsante
	// da «genera e scarica il pacchetto» in «rigenera le credenziali della postazione».
	DaSistemare bool
}

type postazioniDati struct {
	Postazioni []postazioneUI
	URL        string // l'indirizzo che il worker deve chiamare
	TLS        bool
	Impronta   string
	Scadenza   time.Time
	// Errore è ciò che non si è potuto fare e perché: una pagina che torna identica dopo un pulsante
	// lascia l'operatore a chiedersi se ha premuto.
	Errore string
	// Admin: solo un amministratore può generare le credenziali. Agli altri la pagina resta leggibile
	// — sapere quale PC è acceso serve a tutti — senza il pulsante.
	Admin bool
	// DaSistemare: i PC con almeno una credenziale che non fa entrare nessuno. Sta in cima alla
	// pagina perché è l'unica cosa che, se c'è, va fatta prima di tutto il resto.
	DaSistemare []string
}

// URLServer è l'indirizzo che un worker deve chiamare per arrivare qui.
//
// `[server].indirizzo` è un indirizzo di ASCOLTO e può essere 0.0.0.0 o vuoto, che vuol dire «tutte
// le interfacce» e non è un posto dove telefonare. In quel caso si usa il nome host della macchina:
// è ciò che un worker può risolvere, ed è anche uno dei nomi del certificato.
func (s *Server) URLServer() string {
	host, porta, err := net.SplitHostPort(strings.TrimSpace(s.Indirizzo))
	if err != nil {
		host, porta = strings.TrimSpace(s.Indirizzo), ""
	}
	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		if h, err := os.Hostname(); err == nil && h != "" {
			host = h
		} else {
			host = "127.0.0.1"
		}
	}
	schema := "http"
	if s.TLS != nil {
		schema = "https"
	}
	if porta == "" {
		return schema + "://" + host
	}
	return schema + "://" + net.JoinHostPort(host, porta)
}

func (s *Server) adminPostazioni(w http.ResponseWriter, r *http.Request) {
	s.rendiPostazioni(w, r, postazioniDati{})
}

func (s *Server) rendiPostazioni(w http.ResponseWriter, r *http.Request, d postazioniDati) {
	q := db.New(s.Pool)
	ctx := r.Context()
	poste, err := q.ListPostazioni(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	presenze := map[string]db.ListWorkerPresenzaRow{}
	if righe, err := q.ListWorkerPresenza(ctx); err == nil {
		for _, p := range righe {
			presenze[p.WorkerNome] = p
		}
	}
	// Lo stato si calcola su TUTTE le credenziali, non su quelle di una postazione alla volta: un
	// token condiviso può esserlo anche fra due PC diversi, ed è proprio il caso che si vede peggio.
	tutte, err := q.ListWorkerCredenziali(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	stato := fondazioni.StatoCredenziali(tutte)
	d.URL = s.URLServer()
	d.TLS = s.TLS != nil
	if u := utenteDa(ctx); u != nil {
		d.Admin = u.Ruolo == db.RuoloUtenteAdmin
	}
	if s.TLS != nil {
		d.Impronta, d.Scadenza = s.TLS.Impronta, s.TLS.Scadenza
	}
	for _, p := range poste {
		u := postazioneUI{NomeHost: p.NomeHost, Descrizione: p.Descrizione, Attiva: p.Attiva}
		cred, err := q.ListCredenzialiDiPostazione(ctx, uuid.NullUUID{UUID: p.PostazioneID, Valid: true})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		for _, c := range cred {
			cu := credenzialeUI{Nome: c.WorkerNome, Tipo: string(c.WorkerTipo), Attivo: c.Attivo, Aggiornato: c.AggiornatoIl}
			sc := stato[c.WorkerNome]
			cu.Stato = sc.Stato
			switch sc.Stato {
			case fondazioni.CredenzialeDaGenerare:
				cu.Problema = "credenziale mai generata: questo worker non può collegarsi"
			case fondazioni.CredenzialeCondivisa:
				cu.Problema = "stesso token di " + strings.Join(sc.ConChi, ", ") +
					": un segreto che è di due non identifica nessuno, e il server risponde 401 a tutti e due"
			}
			if cu.Guasta() && c.Attivo {
				u.DaSistemare = true
			}
			if caselle, err := q.ListCaselleAutorizzate(ctx, c.WorkerNome); err == nil {
				for _, x := range caselle {
					cu.Caselle = append(cu.Caselle, x.Nome)
				}
			}
			if pr, ok := presenze[c.WorkerNome]; ok {
				t := pr.UltimoClaim
				cu.UltimoClaim, cu.OutlookOk = &t, pr.OutlookOk
				if pr.IndirizzoIp != nil {
					cu.IP = pr.IndirizzoIp.String()
				}
			}
			u.Worker = append(u.Worker, cu)
		}
		if u.DaSistemare {
			d.DaSistemare = append(d.DaSistemare, u.NomeHost)
		}
		d.Postazioni = append(d.Postazioni, u)
	}
	s.rendi(w, r, "postazioni.html", "postazioni_elenco", "Postazioni", d)
}

// pacchettoWorker genera i segreti dei worker di una postazione e restituisce il pacchetto da
// copiare su quel PC (D22).
//
// Rigenerare INVALIDA i token precedenti: è il motivo per cui il pulsante chiede conferma. Non c'è
// un modo di «rivedere» un token esistente, e non deve esserci: in database c'è solo lo sha256, e
// una schermata che sapesse rileggere i segreti sarebbe il posto da cui rubarli tutti insieme.
func (s *Server) pacchettoWorker(w http.ResponseWriter, r *http.Request) {
	u := utenteDa(r.Context())
	if u == nil || u.Ruolo != db.RuoloUtenteAdmin {
		http.Error(w, "solo un amministratore può generare le credenziali di una postazione", http.StatusForbidden)
		return
	}
	host := strings.ToUpper(strings.TrimSpace(r.PathValue("host")))
	q := db.New(s.Pool)
	p, err := q.GetPostazionePerHost(r.Context(), host)
	if err != nil {
		http.Error(w, "postazione sconosciuta: "+host, 404)
		return
	}
	cred, err := q.ListCredenzialiDiPostazione(r.Context(), uuid.NullUUID{UUID: p.PostazioneID, Valid: true})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(cred) == 0 {
		s.rendiPostazioni(w, r, postazioniDati{Errore: "nessun worker censito su " + host +
			": aggiungerlo a [[worker]] in cockpit.toml (con `token` vuoto: il segreto lo genera questa pagina) e riavviare il server"})
		return
	}
	segreti := map[string]string{}
	for _, c := range cred {
		t, err := rete.TokenNuovo()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := q.ImpostaTokenWorker(r.Context(), db.ImpostaTokenWorkerParams{
			WorkerNome: c.WorkerNome, TokenHash: rete.ImprontaToken(t),
		}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		segreti[c.WorkerNome] = t
	}
	s.Log.Warn("credenziali dei worker rigenerate dalla pagina Postazioni: i token precedenti non valgono più",
		"postazione", host, "utente", u.Sigla, "worker", len(cred))

	toml := s.workerTOML(host, cred, segreti)
	zipBytes, err := s.zipPacchetto(host, toml)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "cockpit-worker-"+strings.ToLower(host)+".zip"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(zipBytes)
}

// workerTOML scrive la configurazione del PC: un file, con dentro tutto ciò che il worker non può
// indovinare. L'impronta del certificato ci finisce da sola — è la sola forma in cui un'impronta
// viene davvero verificata, perché nessuno la trascrive a mano da un certificato.
func (s *Server) workerTOML(host string, cred []db.WorkerCredenziale, segreti map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Configurazione del worker Cockpit RFQ per %s\n", host)
	fmt.Fprintf(&b, "# Generata dal server il %s. I token qui dentro sono segreti: questo file non va in nessun repository.\n#\n",
		time.Now().Format("02/01/2006 15:04"))
	b.WriteString("# Rigenerare il pacchetto dalla pagina Postazioni invalida i token di questo file.\n\n")
	fmt.Fprintf(&b, "server_url = %q\n", s.URLServer())
	if s.TLS != nil {
		b.WriteString("\n# Impronta sha256 del certificato del server: il worker rifiuta di parlare con chiunque\n")
		b.WriteString("# altro, anche se presenta un certificato valido. Cambia solo se il certificato viene rifatto.\n")
		fmt.Fprintf(&b, "impronta = %q\n", s.TLS.Impronta)
	} else {
		b.WriteString("\n# Il server gira IN CHIARO: non c'è nessuna impronta da verificare, e il token di questo\n")
		b.WriteString("# file viaggia leggibile sulla rete. Vale solo per il banco di prova.\n")
	}
	b.WriteString("\nstaging = 'C:\\cockpit\\staging'   # cartella locale del worker: file temporanei e log\n")
	b.WriteString("consenti_invio = false\n")
	for _, c := range cred {
		sezione := string(c.WorkerTipo)
		fmt.Fprintf(&b, "\n[%s]\n", sezione)
		fmt.Fprintf(&b, "worker_id = %q\n", c.WorkerNome)
		fmt.Fprintf(&b, "token     = %q\n", segreti[c.WorkerNome])
	}
	return b.String()
}

// istruzioni è il foglio che accompagna il pacchetto. Sta nello zip e non in una pagina web perché
// chi lo legge è davanti al PC nuovo, che di solito non è quello da cui si è scaricato il pacchetto.
func istruzioni(host, url string, conTLS bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Worker Cockpit RFQ per %s\n", host)
	b.WriteString(strings.Repeat("=", 40) + "\n\n")
	fmt.Fprintf(&b, "Server: %s\n\n", url)
	b.WriteString("1. Copiare questa cartella sul PC, per esempio in C:\\cockpit\\worker\n")
	b.WriteString("2. Installare le dipendenze:   python -m pip install -r requirements.txt\n")
	b.WriteString("3. Controllare che [staging] in worker.toml punti a una cartella che esiste\n")
	b.WriteString("4. Prova senza prendere job:   python worker_outlook.py --caselle\n")
	b.WriteString("   (deve elencare SOLO le caselle assegnate a questo PC dal server)\n")
	b.WriteString("5. Avvio normale:              python worker_outlook.py\n")
	b.WriteString("   Worker di analisi (se serve su questo PC): python worker_analisi.py\n\n")
	if conTLS {
		b.WriteString("Il collegamento è cifrato e il worker verifica l'IMPRONTA del certificato scritta in\n")
		b.WriteString("worker.toml. Se il certificato del server viene rifatto, il worker si ferma con un errore\n")
		b.WriteString("esplicito: va scaricato un pacchetto nuovo. Non togliere l'impronta per farlo ripartire.\n\n")
	}
	b.WriteString("I token in worker.toml sono segreti e valgono solo per questo PC. Se il file viene perso,\n")
	b.WriteString("non si recupera: si rigenera il pacchetto dalla pagina Postazioni (e i token vecchi muoiono).\n")
	return b.String()
}

// servePostazione dice se un file di `workers/` ha senso su un PC di produzione.
//
// Fuori restano i test e gli strumenti di sviluppo: il server finto, il generatore dei contratti, il
// banco end-to-end. Non sono pericolosi — nessuno li avvia — ma un pacchetto che porta anche un finto
// server è un pacchetto in cui, il giorno che qualcosa non va, si comincia a provare cose. Ciò che
// sta su una postazione deve essere solo ciò che le serve.
func servePostazione(base string) bool {
	switch {
	case strings.HasPrefix(base, "test_"):
		return false
	case base == "server_finto.py", base == "prova_e2e.py", base == "genera_contratti.py":
		return false
	case base == "worker.toml", base == "worker.toml.example":
		return false // il worker.toml lo scriviamo noi, con i token di questa postazione
	}
	return true
}

// zipPacchetto mette insieme la configurazione, le istruzioni e i file del worker.
//
// I file del worker sono nel binario del server (embed): così la versione del worker e quella del
// server non possono allontanarsi, che è il modo in cui una postazione comincia a comportarsi in modo
// diverso dalle altre senza che nessuno se ne accorga. I test (`test_*.py`) restano fuori: non
// servono su un PC di produzione e porterebbero con sé dipendenze che lì non ci sono.
func (s *Server) zipPacchetto(host, workerToml string) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	scrivi := func(nome, contenuto string) error {
		f, err := z.Create(nome)
		if err != nil {
			return err
		}
		_, err = f.Write([]byte(contenuto))
		return err
	}
	if err := scrivi("worker.toml", workerToml); err != nil {
		return nil, err
	}
	if err := scrivi("ISTRUZIONI.txt", istruzioni(host, s.URLServer(), s.TLS != nil)); err != nil {
		return nil, err
	}
	if s.Workers != nil {
		nomi, err := fs.Glob(s.Workers, "workers/*")
		if err != nil {
			return nil, err
		}
		sort.Strings(nomi)
		for _, n := range nomi {
			if !servePostazione(path.Base(n)) {
				continue
			}
			base := path.Base(n)
			dati, err := fs.ReadFile(s.Workers, n)
			if err != nil {
				return nil, err
			}
			if err := scrivi(base, string(dati)); err != nil {
				return nil, err
			}
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
