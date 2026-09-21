// Package rete prepara il materiale TLS del listener (voce 2.4).
//
// Il problema che risolve non è «cifrare per principio». Dal blocco 2 il server sta su una VM e i
// worker sui PC degli operatori: fra loro passano la posta dell'azienda, il token di ogni worker e il
// cookie di sessione dell'operatore. In chiaro su una LAN aziendale li legge chiunque sia attaccato
// allo stesso switch, e non c'è niente che lo segnali.
//
// Non c'è una CA interna, e aspettarne una vorrebbe dire restare in chiaro. Quindi: certificato
// AUTOFIRMATO, generato dal server al primo avvio, e i worker lo riconoscono dall'IMPRONTA scritta
// nel loro worker.toml. È il modello di SSH, e su una rete senza CA è più forte di una catena che
// nessuno verifica: un impostore dovrebbe presentare lo stesso certificato, non un certificato
// qualsiasi firmato da qualcuno di cui il client si fida.
package rete

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Materiale è il certificato del listener e ciò che serve a raccontarlo.
type Materiale struct {
	Certificato tls.Certificate
	Impronta    string   // sha256 del certificato (DER) in esadecimale minuscolo
	Nomi        []string // i nomi e gli indirizzi per cui vale
	Scadenza    time.Time
	Generato    bool // true = l'ha appena creato questo avvio
	// PEM e' il solo certificato pubblico, com'e' nel file: finisce nel pacchetto della postazione
	// (cert.pem) perche' il browser, a differenza del worker, non verifica un'impronta ma una
	// catena, e la catena di un autofirmato e' lui stesso nello store «Autorita' radice» dell'utente.
	// La chiave privata non esce mai di qui.
	PEM []byte
}

// Copre dice se il certificato vale per quel nome o indirizzo (7C.1, P1): serve a dire, all'avvio,
// che l'host di [server].url_pubblico non e' fra i nomi di un certificato generato prima che
// quell'URL esistesse — e che il browser lo fara' notare.
func (m *Materiale) Copre(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "" {
		return true
	}
	for _, n := range m.Nomi {
		if strings.ToLower(n) == host {
			return true
		}
	}
	return false
}

// DurataCertificato: dieci anni. Un certificato autofirmato la cui impronta è scritta a mano in ogni
// worker.toml non si rinnova da solo, e la scadenza non protegge da niente che il pinning non copra
// già. Una data breve qui non aumenta la sicurezza: ferma i worker in una notte qualsiasi, e li
// rimette in moto qualcuno che smette di verificare l'impronta per far ripartire il lavoro.
const DurataCertificato = 10 * 365 * 24 * time.Hour

// Prepara carica cert/key se ci sono già, altrimenti li genera e li scrive.
//
// Generare invece di fallire è deliberato: un server che chiede di procurarsi un certificato prima di
// partire, su una macchina senza openssl, resta in chiaro. Così il percorso di minor resistenza — non
// scrivere niente nel file e riavviare — è quello cifrato.
func Prepara(fileCert, fileKey string, nomi []string) (*Materiale, error) {
	if fileCert == "" || fileKey == "" {
		return nil, fmt.Errorf("rete: servono sia il certificato sia la chiave")
	}
	_, errC := os.Stat(fileCert)
	_, errK := os.Stat(fileKey)
	switch {
	case errC == nil && errK == nil:
		return carica(fileCert, fileKey)
	case errC == nil || errK == nil:
		// Uno dei due c'è e l'altro no: rigenerare sovrascriverebbe il file esistente, e se quello
		// esistente è la chiave privata non lo si può rifare. Meglio fermarsi e dire quale manca.
		mancante := fileKey
		if errK == nil {
			mancante = fileCert
		}
		return nil, fmt.Errorf("rete: manca %s ma l'altro file c'è: cancellare tutti e due per rigenerarli, o ripristinare quello mancante", mancante)
	default:
		return genera(fileCert, fileKey, nomi)
	}
}

func carica(fileCert, fileKey string) (*Materiale, error) {
	c, err := tls.LoadX509KeyPair(fileCert, fileKey)
	if err != nil {
		return nil, fmt.Errorf("rete: %s / %s: %w", fileCert, fileKey, err)
	}
	m := &Materiale{Certificato: c}
	if len(c.Certificate) == 0 {
		return nil, fmt.Errorf("rete: %s non contiene nessun certificato", fileCert)
	}
	m.Impronta = ImprontaDi(c.Certificate[0])
	foglia, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("rete: %s: %w", fileCert, err)
	}
	m.Scadenza = foglia.NotAfter
	m.Nomi = append(append([]string{}, foglia.DNSNames...), indirizziTesto(foglia.IPAddresses)...)
	m.PEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Certificate[0]})
	return m, nil
}

func genera(fileCert, fileKey string, nomi []string) (*Materiale, error) {
	chiave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serie, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	dns, ip := separaNomi(nomi)
	modello := x509.Certificate{
		SerialNumber:          serie,
		Subject:               pkix.Name{CommonName: "cockpit", Organization: []string{"Cockpit RFQ"}},
		NotBefore:             time.Now().Add(-time.Hour), // un orologio indietro di qualche minuto non deve invalidarlo
		NotAfter:              time.Now().Add(DurataCertificato),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true, // autofirmato: è radice di se stesso
		DNSNames:              dns,
		IPAddresses:           ip,
	}
	der, err := x509.CreateCertificate(rand.Reader, &modello, &modello, &chiave.PublicKey, chiave)
	if err != nil {
		return nil, err
	}
	pemCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	grezzaChiave, err := x509.MarshalPKCS8PrivateKey(chiave)
	if err != nil {
		return nil, err
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: grezzaChiave})

	if err := os.MkdirAll(filepath.Dir(fileCert), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(fileKey), 0o755); err != nil {
		return nil, err
	}
	// La chiave privata a 0600: su Linux è la differenza fra un segreto e un file. Su Windows i
	// permessi POSIX non si applicano e restano le ACL della cartella — è un limite dichiarato, non
	// una svista: il posto della chiave è la VM.
	if err := os.WriteFile(fileKey, pemKey, 0o600); err != nil {
		return nil, fmt.Errorf("rete: scrittura di %s: %w", fileKey, err)
	}
	if err := os.WriteFile(fileCert, pemCert, 0o644); err != nil {
		return nil, fmt.Errorf("rete: scrittura di %s: %w", fileCert, err)
	}
	c, err := tls.X509KeyPair(pemCert, pemKey)
	if err != nil {
		return nil, err
	}
	return &Materiale{
		Certificato: c, Impronta: ImprontaDi(der), Nomi: append(dns, indirizziTesto(ip)...),
		Scadenza: modello.NotAfter, Generato: true, PEM: pemCert,
	}, nil
}

// ImprontaDi è lo sha256 del certificato in forma DER, esadecimale minuscolo: la stessa stringa che
// il worker calcola sul certificato che riceve e confronta con quella del suo worker.toml.
func ImprontaDi(der []byte) string {
	somma := sha256.Sum256(der)
	return hex.EncodeToString(somma[:])
}

// NomiPredefiniti sono i nomi per cui vale il certificato quando non se ne dichiara nessuno: il nome
// host della macchina, l'indirizzo su cui ascolta se è un IP, e loopback.
//
// Con il pinning per impronta il nome non decide niente — il worker non verifica l'hostname, verifica
// l'impronta — ma un browser sì, e un certificato che non nomina la macchina dà un avviso in più da
// ignorare ogni volta. Gli avvisi che si ignorano sempre sono quelli che poi non si leggono.
func NomiPredefiniti(indirizzo string) []string {
	nomi := []string{"localhost", "127.0.0.1", "::1"}
	if h, err := os.Hostname(); err == nil && h != "" {
		nomi = append(nomi, h)
	}
	host := indirizzo
	if h, _, err := net.SplitHostPort(indirizzo); err == nil {
		host = h
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host != "" && host != "0.0.0.0" && host != "::" {
		nomi = append(nomi, host)
	}
	return unici(nomi)
}

func separaNomi(nomi []string) ([]string, []net.IP) {
	var dns []string
	var ip []net.IP
	for _, n := range unici(nomi) {
		n = strings.Trim(strings.TrimSpace(n), "[]")
		if n == "" {
			continue
		}
		if a := net.ParseIP(n); a != nil {
			ip = append(ip, a)
			continue
		}
		dns = append(dns, n)
	}
	return dns, ip
}

func indirizziTesto(ip []net.IP) []string {
	out := make([]string, 0, len(ip))
	for _, a := range ip {
		out = append(out, a.String())
	}
	return out
}

func unici(in []string) []string {
	visti := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || visti[strings.ToLower(s)] {
			continue
		}
		visti[strings.ToLower(s)] = true
		out = append(out, s)
	}
	return out
}

// ---------------------------------------------------------------- credenziali dei worker

// ImprontaToken è lo sha256 esadecimale di un token di worker: la forma in cui i segreti stanno in
// database (`worker_credenziale.token_hash`). Il token vero non ci arriva mai.
//
// Sta accanto all'impronta del certificato perché è la stessa idea applicata due volte: le due
// macchine si riconoscono confrontando un'impronta, non scambiandosi un segreto. E sta in un posto
// solo perché chi semina la credenziale e chi la verifica DEVONO calcolarla allo stesso modo — due
// copie che divergono su uno spazio in fondo darebbero un 401 che nessuno spiega.
func ImprontaToken(token string) string {
	somma := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(somma[:])
}

// ImprontaNonGenerata è l'impronta che marca una credenziale ESISTENTE ma SENZA SEGRETO: il worker è
// censito in cockpit.toml con `token` vuoto, dice quali caselle serve, e non autentica finché
// qualcuno non genera il pacchetto dalla pagina Postazioni.
//
// È lo sha256 della stringa vuota, e la scelta non è estetica: `auth` scarta l'header vuoto PRIMA di
// calcolare l'impronta, quindi questo valore non può essere prodotto da nessuna richiesta: è un
// segnaposto che nessuno può presentare. Prima qui ci finiva un token casuale, che aveva la stessa
// proprietà ma era indistinguibile da un segreto vero: la pagina Postazioni non poteva dire «questa
// credenziale è ancora da generare», e lo si scopriva soltanto dal primo 401 del worker.
//
// Resta comunque un hash valido per il vincolo della colonna (64 esadecimali minuscoli), così una
// credenziale senza segreto è una riga normale e non un caso speciale dello schema.
var ImprontaNonGenerata = ImprontaToken("")

// TokenNuovo genera un segreto per un worker: 32 byte casuali in base64url, senza caratteri che si
// rompano dentro un TOML o in un copia-incolla.
func TokenNuovo() (string, error) {
	grezzo := make([]byte, 32)
	if _, err := rand.Read(grezzo); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(grezzo), nil
}
