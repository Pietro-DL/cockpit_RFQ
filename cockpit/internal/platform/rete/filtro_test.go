package rete

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// L1 — il filtro delle reti consentite sul listener (scripts/avvio-rete).

func TestAmmessaSoloDalleRetiEDaQuestoPC(t *testing.T) {
	reti := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24"), netip.MustParsePrefix("fd12:3456:789a:1::/64")}
	locale4, locale6 := netip.MustParseAddr("10.0.0.7"), netip.MustParseAddr("fd12:3456:789a:1::7")
	casi := []struct {
		da, a  string
		attesa bool
	}{
		{"10.0.0.15", "10.0.0.7", true},
		{"::ffff:10.0.0.15", "10.0.0.7", true}, // IPv4 su un listener doppio
		{"10.0.1.15", "10.0.0.7", false},       // la rete accanto
		{"192.168.1.10", "10.0.0.7", false},
		{"8.8.8.8", "10.0.0.7", false},
		{"127.0.0.1", "10.0.0.7", true}, // questo PC
		{"10.0.0.7", "10.0.0.7", true},  // questo PC dal suo indirizzo di rete
		{"fd12:3456:789a:1::15", "fd12:3456:789a:1::7", true},
		{"fd12:3456:789a:2::15", "fd12:3456:789a:1::7", false},
		{"2001:db8::1", "fd12:3456:789a:1::7", false},
		{"fe80::1%23", "fd12:3456:789a:1::7", false},
		{"::1", "fd12:3456:789a:1::7", true},
		{"fd12:3456:789a:1::7", "fd12:3456:789a:1::7", true},
	}
	for _, c := range casi {
		if got := Ammessa(netip.MustParseAddr(c.da), netip.MustParseAddr(c.a), reti); got != c.attesa {
			t.Errorf("Ammessa(%s → %s) = %v, attesa %v", c.da, c.a, got, c.attesa)
		}
	}
	// un indirizzo che non si e' potuto leggere non passa
	if Ammessa(netip.Addr{}, locale4, reti) {
		t.Error("indirizzo vuoto ammesso")
	}
	// una zona sulla rete ammessa non cambia la risposta
	reti = append(reti, netip.MustParsePrefix("fe80::/64"))
	if !Ammessa(netip.MustParseAddr("fe80::1%23"), locale6, reti) {
		t.Error("fe80::1%23 con fe80::/64 fra le reti: rifiutato per la zona")
	}
}

// Sul listener vero: una connessione rifiutata si chiude prima del TLS (il client non vede nessun
// certificato), la successiva ammessa arriva a HTTP, e il rifiuto si annota una volta al minuto.
func TestIlFiltroChiudePrimaDelTLS(t *testing.T) {
	d := t.TempDir()
	m, err := Prepara(filepath.Join(d, "cert.pem"), filepath.Join(d, "key.pem"), []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	grezzo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Tutto passa da loopback, che Ammessa lascia sempre entrare: qui la decisione e' finta (rifiuta
	// le prime due connessioni), il resto e' il listener vero.
	var mu sync.Mutex
	viste, annotati := 0, 0
	f := &filtro{Listener: grezzo, visti: map[netip.Addr]time.Time{},
		ammessa: func(da, a netip.Addr) bool {
			mu.Lock()
			defer mu.Unlock()
			viste++
			return viste > 2
		},
		rifiutata: func(netip.Addr) { mu.Lock(); annotati++; mu.Unlock() },
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{m.Certificato}}}
	go srv.ServeTLS(f, "", "")
	t.Cleanup(func() { srv.Close() })

	indirizzo := grezzo.Addr().String()
	for i := 0; i < 2; i++ {
		c, err := tls.Dial("tcp", indirizzo, &tls.Config{InsecureSkipVerify: true})
		if err == nil {
			c.Close()
			t.Fatalf("tentativo %d: handshake TLS riuscito su una connessione da rifiutare", i+1)
		}
	}
	cliente := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 5 * time.Second}
	r, err := cliente.Get("https://" + indirizzo + "/")
	if err != nil {
		t.Fatalf("la connessione ammessa non e' arrivata a HTTP: %v", err)
	}
	corpo, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if string(corpo) != "ok" {
		t.Errorf("risposta: %q", corpo)
	}
	mu.Lock()
	defer mu.Unlock()
	if annotati != 1 {
		t.Errorf("rifiuti annotati: %d, atteso 1 (due tentativi dallo stesso indirizzo nello stesso minuto)", annotati)
	}
}
