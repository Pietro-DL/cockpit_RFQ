package rete

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// SoloDalleReti avvolge il listener con [server].reti_consentite: una connessione che non viene da
// una di quelle reti si chiude appena accettata, prima del TLS e prima di HTTP. Un PC fuori elenco
// non riceve nemmeno il certificato, e nessun handler deve ricordarsi di controllare niente.
//
// Il controllo e' sull'indirizzo della connessione TCP, non su un header: X-Forwarded-For lo scrive
// chiunque, l'indirizzo da cui arriva un handshake TCP no (voce 2.7, P7.1).
//
// `rifiutata` riceve l'indirizzo respinto, una volta al minuto per indirizzo: un PC che riprova ogni
// secondo non deve riempire il log.
func SoloDalleReti(ln net.Listener, reti []netip.Prefix, rifiutata func(da netip.Addr)) net.Listener {
	return &filtro{Listener: ln, ammessa: func(da, a netip.Addr) bool { return Ammessa(da, a, reti) }, rifiutata: rifiutata,
		visti: map[netip.Addr]time.Time{}}
}

// Ammessa dice se una connessione da `da` verso `a` (l'indirizzo locale su cui e' arrivata) passa il
// filtro. Passa sempre questo PC: loopback, e `da` uguale ad `a`, che e' il browser del server che
// chiama il proprio indirizzo di rete. Senza, chi avvia il server con un elenco di due colleghi non
// potrebbe aprirlo dalla propria scrivania.
func Ammessa(da, a netip.Addr, reti []netip.Prefix) bool {
	// Contains non ammette mai un indirizzo con la zona (fe80::1%23), e un IPv4 arriva in forma 4in6
	// su un listener doppio: si confronta l'indirizzo e basta.
	da, a = da.Unmap().WithZone(""), a.Unmap().WithZone("")
	if !da.IsValid() {
		return false
	}
	if da.IsLoopback() || da == a {
		return true
	}
	for _, p := range reti {
		if p.Contains(da) {
			return true
		}
	}
	return false
}

type filtro struct {
	net.Listener
	ammessa   func(da, a netip.Addr) bool
	rifiutata func(da netip.Addr)

	mu    sync.Mutex
	visti map[netip.Addr]time.Time
}

func (f *filtro) Accept() (net.Conn, error) {
	for {
		c, err := f.Listener.Accept()
		if err != nil {
			return nil, err
		}
		da, a := indirizzo(c.RemoteAddr()), indirizzo(c.LocalAddr())
		if f.ammessa(da, a) {
			return c, nil
		}
		_ = c.Close()
		f.annota(da)
	}
}

// annota passa il rifiuto a chi scrive il log, ma non piu' di una volta al minuto per indirizzo.
// L'elenco si svuota oltre mille voci: e' un freno al rumore, non un registro.
func (f *filtro) annota(da netip.Addr) {
	if f.rifiutata == nil {
		return
	}
	adesso := time.Now()
	f.mu.Lock()
	if len(f.visti) > 1000 {
		clear(f.visti)
	}
	ultima, gia := f.visti[da]
	scrivi := !gia || adesso.Sub(ultima) >= time.Minute
	if scrivi {
		f.visti[da] = adesso
	}
	f.mu.Unlock()
	if scrivi {
		f.rifiutata(da)
	}
}

func indirizzo(a net.Addr) netip.Addr {
	if t, ok := a.(*net.TCPAddr); ok {
		return t.AddrPort().Addr()
	}
	if a == nil {
		return netip.Addr{}
	}
	p, err := netip.ParseAddrPort(a.String())
	if err != nil {
		return netip.Addr{}
	}
	return p.Addr()
}
