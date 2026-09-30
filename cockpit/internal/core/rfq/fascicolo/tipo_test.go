package fascicolo

// L1 — Smistamento, fase T (decisioni del 27/09 ter, «Tipo componente / commerciale», Domanda 5 = B;
// precisazione dell'utente del 27/09 sera): il tipo del componente lo decide una persona. Le regole pure del
// cambio (MotivoTipoSpento, Opzioni), le simulazioni con cui l'anteprima dice che cosa varrebbe dopo (ConTipo,
// SenzaSospensione, CatenaSenzaSospensioni) e la guardia che nessun automatismo del codice scriva il tipo di
// COMPONENTE commerciale. Le prove L4 degli stessi casi stanno in tipo_db_test.go.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// fattiTipo: un componente del tipo dato, radice, senza figli, fuori dalla richiesta, senza autorizzazioni.
func fattiTipo(codice string, tipo db.TipoComponente) FattiTipo {
	return FattiTipo{Componente: componente(codice, tipo, false)}
}

// Fase T, le regole del tipo una per caso: finito solo per un codice della richiesta, radice, senza delega e
// con l'autorizzazione propria valida (che diventa lo STEP strutturale); da finito ad altro non con lo STEP
// strutturale (il CHECK di 0020:214), tranne verso commerciale quando lo STEP strutturale e' anche
// un'autorizzazione con la marcatura (sospende, e il riferimento si svuota); sciolto non con dei figli («ha 2
// figli: è un assieme»); commerciale nemmeno, perche' il particolare commerciale e' sempre una foglia; senza
// figli commerciale si fa anche con l'autorizzazione (sospende, non si rifiuta), salvo il finito nella forma di
// prima; la BOM congelata, un archiviato e lo stesso tipo spengono tutto.
//
// Giro di correzione: il caso «da finito con lo STEP strutturale» prima fissava il rifiuto del commerciale per
// ogni finito con lo STEP strutturale; adesso vale solo per la forma di prima (senza marcatura), e i casi del
// finito con la marcatura dicono che commerciale si fa e che assieme resta spento, con il consiglio.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che un assieme con un figlio e con l'autorizzazione
// diventasse commerciale (la sospensione, non un rifiuto: precisazione del 27/09 sera). Adesso vale la regola
// della PR #7, confermata dall'utente il 29/09 sera (domanda 6a: «il commerciale è SEMPRE una foglia e mai un
// ramo»): con un figlio il commerciale si spegne con «7120010 ha 1 figlio: è un assieme». Il caso senza figli con
// l'autorizzazione, che prima era compreso in quello, ha una riga sua: si fa, e la sospensione resta.
// Asserzioni: prima 20 casi controllati, dopo 21 (le chiamate t.Error/t.Fatal sono le stesse 3).
func TestLeRegoleDelTipoDelComponente(t *testing.T) {
	sha := shaDi("a")
	valida := func(c db.Componente) Dichiarazione {
		d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rigaDich(c, sha, "#1", OrigineSmistamento)})
		x, ok := d.Di(c.ComponenteID)
		if !ok {
			t.Fatalf("la dichiarazione di prova non vale: %+v", d)
		}
		return x
	}
	casi := []struct {
		nome   string
		f      func() FattiTipo
		tipo   db.TipoComponente
		spenta string // "" = si puo'
	}{
		{"tipo che non esiste", func() FattiTipo { return fattiTipo("7120010", db.TipoComponenteSottoassieme) }, "boh", "tipo di componente non valido"},
		{"lo stesso tipo", func() FattiTipo { return fattiTipo("7120010", db.TipoComponenteSottoassieme) }, db.TipoComponenteSottoassieme, "7120010 è già un assieme"},
		{"BOM congelata", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.Bloccata = 2
			return f
		}, db.TipoComponenteCommerciale, "la BOM è congelata nella V2"},
		{"archiviato", func() FattiTipo {
			return FattiTipo{Componente: componente("7120010", db.TipoComponenteSottoassieme, true)}
		},
			db.TipoComponenteSciolto, "7120010 è archiviato: prima lo si ripristina"},
		{"sciolto con due figli", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.Figli = []string{"7120011", "7120012"}
			return f
		}, db.TipoComponenteSciolto, "7120010 ha 2 figli: è un assieme"},
		{"sciolto con un figlio", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteCommerciale)
			f.Figli = []string{"7120011"}
			return f
		}, db.TipoComponenteSciolto, "7120010 ha 1 figlio: è un assieme"},
		{"sciolto senza figli", func() FattiTipo { return fattiTipo("7120010", db.TipoComponenteSottoassieme) }, db.TipoComponenteSciolto, ""},
		{"commerciale con figli e con l'autorizzazione", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.Figli = []string{"7120011"}
			f.Dichiarazioni = []Dichiarazione{valida(f.Componente)}
			return f
		}, db.TipoComponenteCommerciale, "7120010 ha 1 figlio: è un assieme"}, // sotto un particolare commerciale non si mette niente
		{"commerciale senza figli e con l'autorizzazione", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.Dichiarazioni = []Dichiarazione{valida(f.Componente)}
			return f
		}, db.TipoComponenteCommerciale, ""},
		{"finito fuori dalla richiesta", func() FattiTipo { return fattiTipo("7120010", db.TipoComponenteSottoassieme) },
			db.TipoComponenteFinito, "7120010 non è un codice della richiesta"},
		{"finito sotto un padre", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.DellaRichiesta, f.Padri = true, 1
			return f
		}, db.TipoComponenteFinito, "7120010 sta sotto 1 padre nella BOM: un prodotto finito è una radice"},
		{"finito con una delega", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.DellaRichiesta = true
			p := componente("7120001", db.TipoComponenteFinito, false)
			padre := rigaDich(p, sha, "#1", OrigineSmistamento)
			d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{padre, rigaDelega(padre, f.Componente, "#2")})
			f.Dichiarazioni = d.DelComponente(f.Componente.ComponenteID)
			return f
		}, db.TipoComponenteFinito, "ha una delega nello STEP aaaa.stp: un prodotto finito ha uno STEP suo"},
		{"finito con l'autorizzazione sospesa", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.DellaRichiesta = true
			d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{marcaSospesa(rigaDich(f.Componente, sha, "#1", OrigineSmistamento), f.Componente)})
			f.Dichiarazioni = d.DelComponente(f.Componente.ComponenteID)
			return f
		}, db.TipoComponenteFinito, "l'autorizzazione di aaaa.stp per 7120010 non vale (sospesa: 7120010: è diventato commerciale"},
		{"finito con due autorizzazioni", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.DellaRichiesta = true
			d := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{rigaDich(f.Componente, sha, "#1", OrigineSmistamento),
				rigaDich(f.Componente, shaDi("b"), "#1", OrigineSmistamento)})
			f.Dichiarazioni = d.DelComponente(f.Componente.ComponenteID)
			return f
		}, db.TipoComponenteFinito, "7120010 ha 2 STEP autorizzati: se ne revoca uno prima"},
		{"finito con l'autorizzazione valida", func() FattiTipo {
			f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
			f.DellaRichiesta = true
			f.Dichiarazioni = []Dichiarazione{valida(f.Componente)}
			return f
		}, db.TipoComponenteFinito, ""},
		{"finito senza STEP", func() FattiTipo {
			f := fattiTipo("7120001", db.TipoComponenteCommerciale)
			f.DellaRichiesta = true
			return f
		}, db.TipoComponenteFinito, ""},
		{"da finito con lo STEP strutturale nella forma di prima", func() FattiTipo {
			f := fattiTipo("7120001", db.TipoComponenteFinito)
			f.StepStrutturale = "7120001A_1.stp"
			return f
		}, db.TipoComponenteCommerciale, "7120001 ha uno STEP strutturale (7120001A_1.stp): resta un prodotto finito finché quel riferimento c'è; si revoca prima l'autorizzazione dello STEP: è nella forma di prima"},
		{"da finito con lo STEP strutturale marcato, a commerciale", func() FattiTipo { return finitoMarcato(valida) }, db.TipoComponenteCommerciale, ""},
		{"da finito con lo STEP strutturale marcato, ad assieme", func() FattiTipo { return finitoMarcato(valida) }, db.TipoComponenteSottoassieme,
			"7120001 ha uno STEP strutturale (7120001A_1.stp): resta un prodotto finito finché quel riferimento c'è; si revoca prima l'autorizzazione dello STEP, oppure diventa commerciale"},
		{"da finito con lo STEP strutturale marcato su un altro documento", func() FattiTipo {
			f := finitoMarcato(valida)
			f.Componente.StepStrutturaleID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			return f
		}, db.TipoComponenteCommerciale, "è nella forma di prima"},
		{"da finito senza STEP strutturale", func() FattiTipo { return fattiTipo("7120001", db.TipoComponenteFinito) }, db.TipoComponenteSottoassieme, ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			got := MotivoTipoSpento(c.f(), c.tipo)
			switch {
			case c.spenta == "" && got != "":
				t.Errorf("doveva potersi fare: %q", got)
			case c.spenta != "" && !strings.Contains(got, c.spenta):
				t.Errorf("spenta: %q, attesa %q", got, c.spenta)
			}
		})
	}
}

// finitoMarcato: il prodotto 7120001 con il suo STEP autorizzato nella forma dello Smistamento (la marcatura), che
// e' anche il suo STEP strutturale.
func finitoMarcato(valida func(db.Componente) Dichiarazione) FattiTipo {
	f := fattiTipo("7120001", db.TipoComponenteFinito)
	d := valida(f.Componente)
	f.Componente.StepStrutturaleID = d.Documento
	f.StepStrutturale = "7120001A_1.stp"
	f.Dichiarazioni = []Dichiarazione{d}
	return f
}

// La tendina: i quattro tipi nell'ordine, quello di adesso segnato e spento («è già»), gli altri col loro
// motivo; particolare e particolare commerciale sono spenti per un pezzo con dei figli (sotto non ci va niente),
// e accesi quando i figli non ci sono (per il commerciale la sospensione non e' un rifiuto).
//
// Riscritta per lo Smistamento (Distinta): prima fissava il commerciale sempre acceso per un assieme con un figlio
// (o[3] senza motivo) e il nome «commerciale». Adesso, con la regola della PR #7 confermata il 29/09 sera
// (domanda 6a), o[3] e' spento con «ha 1 figlio: è un assieme» e si chiama «particolare commerciale»; lo stesso
// pezzo senza figli ha particolare e commerciale accesi. Asserzioni (chiamate t.Error/t.Fatal): prima 4, dopo 5.
func TestLaTendinaDelTipoDiceIMotivi(t *testing.T) {
	f := fattiTipo("7120010", db.TipoComponenteSottoassieme)
	f.Figli = []string{"7120011"}
	o := f.Opzioni()
	var tipi []string
	for _, x := range o {
		tipi = append(tipi, string(x.Tipo))
	}
	if strings.Join(tipi, " ") != "finito sottoassieme sciolto commerciale" {
		t.Fatalf("ordine: %v", tipi)
	}
	if !o[1].Attuale || !strings.Contains(o[1].Spenta, "è già un assieme") || o[0].Attuale {
		t.Errorf("il tipo di adesso: %+v", o[1])
	}
	if !strings.Contains(o[0].Spenta, "non è un codice della richiesta") || !strings.Contains(o[2].Spenta, "ha 1 figlio: è un assieme") || !strings.Contains(o[3].Spenta, "ha 1 figlio: è un assieme") {
		t.Errorf("i motivi: %+v", o)
	}
	if o[0].Nome != "prodotto" || o[3].Nome != "particolare commerciale" {
		t.Errorf("i nomi della schermata: %+v", o)
	}
	// lo stesso assieme senza figli: particolare e particolare commerciale si possono scegliere
	if o := fattiTipo("7120010", db.TipoComponenteSottoassieme).Opzioni(); o[2].Spenta != "" || o[3].Spenta != "" {
		t.Errorf("senza figli, particolare e commerciale: %+v", o)
	}
}

// Le simulazioni dell'anteprima: con il componente commerciale (ConTipo) la sua autorizzazione e' sospesa, e la
// delega che ne dipende resta senza catena; tolta la sospensione registrata (SenzaSospensione) torna valida, e la
// delega con lei. Le righe date non cambiano: l'anteprima non tocca quello che ha letto.
func TestLeSimulazioniDellAnteprimaNonToccanoLeRighe(t *testing.T) {
	p := componente("7120001", db.TipoComponenteFinito, false)
	s := componente("7120010", db.TipoComponenteSottoassieme, false)
	n := componente("7120011", db.TipoComponenteSottoassieme, false)
	sha := shaDi("c")
	padre := rigaDich(p, sha, "#1", OrigineSmistamento)
	d10 := rigaDelega(padre, s, "#2")
	d11 := rigaDelega(padre, n, "#4")
	d11.Padri = []string{"#2"}
	righe := []db.ListDichiarazioniRfqRow{padre, d10, d11}
	if d := ValutaDichiarazioni(righe); len(d.PerComponente) != 3 {
		t.Fatalf("prima: tre dichiarazioni valide: %+v", d)
	}

	com := ValutaDichiarazioni(ConTipo(righe, s.ComponenteID, db.TipoComponenteCommerciale))
	if _, ok := com.Di(s.ComponenteID); ok {
		t.Error("con 7120010 commerciale la sua delega non vale")
	}
	if _, ok := com.Di(n.ComponenteID); ok {
		t.Error("la delega di 7120011, sotto quella di 7120010, resta senza catena")
	}
	if got := delegheCheCambiano(ValutaDichiarazioni(righe), com, s.ComponenteID, false); strings.Join(got, ",") != "7120011" {
		t.Errorf("le deleghe che si sospendono con lei: %v", got)
	}
	if righe[1].Componente.Tipo != db.TipoComponenteSottoassieme {
		t.Error("ConTipo ha cambiato le righe date")
	}

	// la sospensione registrata sulla delega di 7120010: sospesa anche da sottoassieme, e la catena si ferma
	m := Marcatura{V: 1, Ruolo: RuoloDelega, ComponenteID: uuid.NullUUID{UUID: s.ComponenteID, Valid: true},
		Sospesa: &Sospensione{Motivo: MotivoDiventatoCommerciale, Tipo: "commerciale", Da: uuid.NullUUID{UUID: uuid.New(), Valid: true}}}
	sosp := append([]db.ListDichiarazioniRfqRow{}, righe...)
	sosp[1].Marcatura, _ = json.Marshal(m)
	prima := ValutaDichiarazioni(sosp)
	if x := prima.DelComponente(s.ComponenteID); len(x) != 1 || !strings.Contains(x[0].Problema, "è diventato commerciale") || x[0].Sospensione == nil {
		t.Fatalf("la delega sospesa: %+v", x)
	}
	dopo := ValutaDichiarazioni(SenzaSospensione(sosp, map[uuid.UUID]bool{sosp[1].PropostaID: true}))
	if _, ok := dopo.Di(s.ComponenteID); !ok {
		t.Error("tolta la sospensione, la delega di 7120010 torna valida")
	}
	if got := delegheCheCambiano(prima, dopo, s.ComponenteID, true); strings.Join(got, ",") != "7120011" {
		t.Errorf("le deleghe che tornano con lei: %v", got)
	}
	if !strings.Contains(string(sosp[1].Marcatura), "sospesa") {
		t.Error("SenzaSospensione ha cambiato le righe date")
	}

	// senza nessuna sospensione la catena c'e' (7120010 sospeso, non revocato); senza la riga di 7120010, no
	st := ValutaDichiarazioni(CatenaSenzaSospensioni(sosp))
	x11 := prima.DelComponente(n.ComponenteID)
	if len(x11) != 1 || senzaCatenaStrutturale(st, x11[0]) {
		t.Errorf("con 7120010 sospeso la catena di 7120011 c'e' ancora: %+v", st.DelComponente(n.ComponenteID))
	}
	senza := ValutaDichiarazioni(CatenaSenzaSospensioni([]db.ListDichiarazioniRfqRow{padre, sosp[2]}))
	if !senzaCatenaStrutturale(senza, x11[0]) {
		t.Error("revocata la delega di 7120010, quella di 7120011 non ha piu' la catena")
	}
}

// Il consiglio della riattivazione (giro di correzione): la delega di 7120011, con la sospensione tolta per
// l'anteprima, resterebbe senza la catena perche' la delega di 7120010, che la tiene nel file, e' sospesa con
// una registrazione: sospesaSopra la trova (si riattiva prima quella). Senza la registrazione sopra (7120010
// riattivato) la catena c'e' e non c'e' niente da consigliare; con la delega di 7120010 revocata (la riga non c'e')
// non c'e' niente da riattivare prima. Lo stesso per il titolare del file sospeso con una registrazione.
func TestIlConsiglioDellaRiattivazioneTrovaChiLaTiene(t *testing.T) {
	p := componente("7120001", db.TipoComponenteFinito, false)
	s := componente("7120010", db.TipoComponenteSottoassieme, false)
	n := componente("7120011", db.TipoComponenteSottoassieme, false)
	sha := shaDi("e")
	padre := rigaDich(p, sha, "#1", OrigineSmistamento)
	sospendi := func(r db.ListDichiarazioniRfqRow, c db.Componente) db.ListDichiarazioniRfqRow {
		m := Marcatura{V: 1, Ruolo: RuoloDelega, ComponenteID: uuid.NullUUID{UUID: c.ComponenteID, Valid: true},
			Sospesa: &Sospensione{Motivo: MotivoDiventatoCommerciale, Tipo: "commerciale"}}
		r.Marcatura, _ = json.Marshal(m)
		return r
	}
	d10 := sospendi(rigaDelega(padre, s, "#2"), s)
	d11 := sospendi(rigaDelega(padre, n, "#4"), n)
	d11.Padri = []string{"#2"}
	righe := []db.ListDichiarazioniRfqRow{padre, d10, d11}
	solo11 := map[uuid.UUID]bool{d11.PropostaID: true}

	dopo := ValutaDichiarazioni(SenzaSospensione(righe, solo11))
	x := dopo.DelComponente(n.ComponenteID)
	if len(x) != 1 || !x[0].SenzaCatena() {
		t.Fatalf("senza la sua sospensione, la delega di 7120011 resta senza catena: %+v", x)
	}
	if d, ok := sospesaSopra(dopo, x[0]); !ok || d.Componente.ComponenteID != s.ComponenteID || fraseDichiarazione(d) != "la delega di eeee.stp per 7120010" {
		t.Errorf("chi la tiene, sospeso: %+v %v", d, ok)
	}

	tutte := ValutaDichiarazioni(SenzaSospensione(righe, map[uuid.UUID]bool{d10.PropostaID: true, d11.PropostaID: true}))
	if y := tutte.DelComponente(n.ComponenteID); len(y) != 1 || !y[0].Valida() {
		t.Fatalf("senza le due sospensioni la delega di 7120011 vale: %+v", y)
	}
	if _, ok := sospesaSopra(tutte, tutte.DelComponente(n.ComponenteID)[0]); ok {
		t.Error("una delega valida non ha un consiglio")
	}

	senzaPadre := ValutaDichiarazioni(SenzaSospensione([]db.ListDichiarazioniRfqRow{padre, d11}, solo11))
	if y := senzaPadre.DelComponente(n.ComponenteID); len(y) != 1 || !y[0].SenzaCatena() {
		t.Fatalf("revocata la delega di 7120010, quella di 7120011 e' senza catena: %+v", y)
	} else if _, ok := sospesaSopra(senzaPadre, y[0]); ok {
		t.Error("senza la riga di chi la teneva non c'e' niente da riattivare prima")
	}

	// il titolare del file sospeso con una registrazione: la delega di 7120010 dice lui
	pSosp := marcaSospesa(padre, p)
	d10v := rigaDelega(padre, s, "#2")
	tit := ValutaDichiarazioni([]db.ListDichiarazioniRfqRow{pSosp, d10v})
	y := tit.DelComponente(s.ComponenteID)
	if len(y) != 1 || y[0].Valida() {
		t.Fatalf("con il titolare sospeso la delega non vale: %+v", y)
	}
	if d, ok := sospesaSopra(tit, y[0]); !ok || d.Componente.ComponenteID != p.ComponenteID {
		t.Errorf("il titolare sospeso: %+v %v", d, ok)
	}
}

// Fase T, la verifica chiesta: nessun automatismo assegna il tipo di COMPONENTE commerciale (decisioni del 27/09
// ter). Nel codice Go (non le prove) db.TipoComponenteCommerciale si usa solo in un confronto (== o !=, un
// case), o nella tendina dei tipi che una persona sceglie (TipiComponente): nessuna assegnazione, nessun
// argomento di una scrittura. Le query non scrivono 'commerciale' in componente.tipo. Il contratto dello STEP
// del worker non porta un tipo di componente (il «commerciale» del worker e' il tipo di DOCUMENTO di un foglio
// di calcolo, un'altra cosa). Il tipo commerciale lo scrive solo SetTipoComponente con il tipo scelto da una
// persona (CambiaTipoComponente) o InsertComponente con il tipo di un modulo.
//
// Riscritta per lo Smistamento (Distinta): prima fissava come sola lista ammessa la tendina TipiComponente. La PR
// #7 mette il particolare commerciale anche in TipiNuovo (i tipi con cui l'operatore scrive un pezzo nella
// Distinta, che sceglie una persona): la guardia ammette TipiNuovo e basta. In piu', perche' la lista ammessa
// non diventi una strada per aggirarla, nessun codice prende un tipo da TipiNuovo o da TipiComponente per
// posizione (TipiNuovo[2] e' il commerciale scelto da un programma). Asserzioni (chiamate t.Error/t.Fatal):
// prima 7, dopo 8 (gli indici delle liste).
//
// Allargata nel giro 4, fase 4.4a.1b, in modo esplicito e per un percorso solo (domanda 30, seconda risposta: il tipo
// commerciale si propone, e lo scrive la conferma della persona, «la responsabilita' e' di chi conferma»): la conferma
// dell'albero fa nascere il particolare commerciale quando una persona ha dato il ✓ alla proposta (o l'ha scelto con
// la tendina). Il tipo lo nomina una funzione sola, tipoConfermatoNellAlbero in albero_conferma.go, in un return, con
// la risposta della persona come argomento: la guardia la ammette per nome e per file, e vuole che ci sia, una volta
// sola (un'eccezione che nessuno usa piu' si toglie); e come per tipoVoluto, senza la conferma di una persona quella
// funzione non da' mai il commerciale. In piu' (dalla verifica della fase: ammettere la funzione per nome non basta, se
// un altro automatismo la chiama con true) la guardia guarda chi la chiama: una chiamata sola, dentro ConfermaAlbero
// in albero_conferma.go, con la risposta della bozza (g.commercialeConfermato()) come argomento; nessun'altra chiamata
// e nessun uso come valore. Asserzioni: prima 8, dopo 12.
func TestNessunAutomatismoScriveIlTipoCommerciale(t *testing.T) {
	radice := filepath.Join("..", "..", "..") // internal
	fset := token.NewFileSet()
	var usi, indici, chiamateFuori []string
	eccezione := 0 // gli usi dentro tipoConfermatoNellAlbero (albero_conferma.go)
	chiamate := 0  // le chiamate ammesse a tipoConfermatoNellAlbero (in ConfermaAlbero)
	err := filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Base(p) == "db" && strings.Contains(filepath.ToSlash(p), "platform/db") {
				return filepath.SkipDir // il codice generato: la definizione della costante
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		var pila []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				pila = pila[:len(pila)-1]
				return true
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "TipoComponenteCommerciale" {
				padre := pila[len(pila)-1]
				permesso := false
				switch x := padre.(type) {
				case *ast.BinaryExpr:
					permesso = x.Op == token.EQL || x.Op == token.NEQ
				case *ast.CaseClause:
					permesso = true
				case *ast.CompositeLit:
					// la tendina dei tipi (TipiComponente) e i tipi con cui l'operatore scrive un pezzo
					// nella Distinta (TipiNuovo): li sceglie una persona, nessun automatismo
					for i := len(pila) - 1; i >= 0; i-- {
						if vs, ok := pila[i].(*ast.ValueSpec); ok {
							permesso = len(vs.Names) == 1 && (vs.Names[0].Name == "TipiComponente" || vs.Names[0].Name == "TipiNuovo")
							break
						}
					}
				case *ast.ReturnStmt:
					// la conferma dell'albero, con il ✓ di una persona (fase 4.4a.1b): solo in tipoConfermatoNellAlbero
					for i := len(pila) - 1; i >= 0; i-- {
						if fd, ok := pila[i].(*ast.FuncDecl); ok {
							permesso = filepath.Base(p) == "albero_conferma.go" && fd.Recv == nil && fd.Name.Name == "tipoConfermatoNellAlbero"
							break
						}
					}
					if permesso {
						eccezione++
					}
				}
				if !permesso {
					usi = append(usi, fset.Position(sel.Pos()).String())
				}
			}
			// chi usa tipoConfermatoNellAlbero: la sua dichiarazione, e una chiamata sola, in ConfermaAlbero, con la
			// risposta della persona nella bozza come argomento (fase 4.4a.1b)
			if id, ok := n.(*ast.Ident); ok && id.Name == "tipoConfermatoNellAlbero" {
				switch x := pila[len(pila)-1].(type) {
				case *ast.FuncDecl:
					if x.Name != id {
						chiamateFuori = append(chiamateFuori, fset.Position(id.Pos()).String())
					}
				case *ast.CallExpr:
					ammessa := false
					for i := len(pila) - 1; i >= 0; i-- {
						if fd, ok := pila[i].(*ast.FuncDecl); ok {
							ammessa = filepath.Base(p) == "albero_conferma.go" && fd.Recv == nil && fd.Name.Name == "ConfermaAlbero"
							break
						}
					}
					if ammessa && x.Fun == id && len(x.Args) == 2 {
						risposta, _ := x.Args[1].(*ast.CallExpr)
						ammessa = risposta != nil && len(risposta.Args) == 0
						if ammessa {
							sel, _ := risposta.Fun.(*ast.SelectorExpr)
							ammessa = sel != nil && sel.Sel.Name == "commercialeConfermato"
						}
					} else {
						ammessa = false
					}
					if ammessa {
						chiamate++
					} else {
						chiamateFuori = append(chiamateFuori, fset.Position(id.Pos()).String())
					}
				default:
					// presa come valore (assegnata, passata): una strada per chiamarla da un'altra parte
					chiamateFuori = append(chiamateFuori, fset.Position(id.Pos()).String())
				}
			}
			// un tipo preso da una delle due liste per posizione e' un tipo scelto dal programma
			if ix, ok := n.(*ast.IndexExpr); ok {
				nome := ""
				switch x := ix.X.(type) {
				case *ast.Ident:
					nome = x.Name
				case *ast.SelectorExpr:
					nome = x.Sel.Name
				}
				if nome == "TipiNuovo" || nome == "TipiComponente" {
					indici = append(indici, fset.Position(ix.Pos()).String())
				}
			}
			pila = append(pila, n)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usi) > 0 {
		t.Errorf("TipoComponenteCommerciale usato fuori da un confronto (un automatismo che scrive il tipo commerciale?):\n%s", strings.Join(usi, "\n"))
	}
	if len(indici) > 0 {
		t.Errorf("un tipo preso per posizione da TipiNuovo o TipiComponente (un automatismo che sceglie il tipo?):\n%s", strings.Join(indici, "\n"))
	}
	if eccezione != 1 {
		t.Errorf("tipoConfermatoNellAlbero nomina il tipo commerciale %d volte, attesa 1: l'eccezione della guardia e' per quel return solo", eccezione)
	}
	if len(chiamateFuori) > 0 {
		t.Errorf("tipoConfermatoNellAlbero usata fuori da ConfermaAlbero o senza la risposta della persona (un automatismo che scrive il tipo commerciale?):\n%s",
			strings.Join(chiamateFuori, "\n"))
	}
	if chiamate != 1 {
		t.Errorf("tipoConfermatoNellAlbero chiamata %d volte da ConfermaAlbero con la risposta della persona, attesa 1", chiamate)
	}

	// le query: nessuna scrive 'commerciale' in componente.tipo
	query, err := filepath.Glob(filepath.Join(radice, "platform", "db", "queries", "*.sql"))
	if err != nil || len(query) == 0 {
		t.Fatalf("le query non si trovano: %v", err)
	}
	for _, q := range query {
		b, err := os.ReadFile(q)
		if err != nil {
			t.Fatal(err)
		}
		for i, riga := range strings.Split(string(b), "\n") {
			if strings.Contains(riga, "'commerciale'") && !strings.HasPrefix(strings.TrimSpace(riga), "--") {
				t.Errorf("%s:%d scrive o legge 'commerciale': %s", filepath.Base(q), i+1, strings.TrimSpace(riga))
			}
		}
	}

	// il contratto dello STEP: nessun campo porta un tipo di componente
	for _, tipo := range []reflect.Type{reflect.TypeOf(worker.NodoSTEP{}), reflect.TypeOf(worker.RelazioneSTEP{}), reflect.TypeOf(worker.StrutturaSTEP{})} {
		for i := 0; i < tipo.NumField(); i++ {
			if nome := strings.ToLower(tipo.Field(i).Name); strings.Contains(nome, "tipo") {
				t.Errorf("il contratto dello STEP porta un tipo: %s.%s", tipo.Name(), tipo.Field(i).Name)
			}
		}
	}

	// e le letture: il tipo suggerito a un nodo e' sottoassieme o sciolto, mai commerciale (F5, qui ripetuto
	// perche' e' la stessa regola vista da un'altra parte)
	for _, proposto := range []db.TipoComponente{"", db.TipoComponenteCommerciale, db.TipoComponenteFinito, db.TipoComponenteSottoassieme} {
		p := db.ComponenteProposta{TipoProposto: db.NullTipoComponente{TipoComponente: proposto, Valid: proposto != ""}}
		for _, figli := range []int{0, 2} {
			if got := tipoVoluto(p, figli); got == db.TipoComponenteCommerciale || got == db.TipoComponenteFinito {
				t.Errorf("tipoVoluto(%q, %d figli) = %s", proposto, figli, got)
			}
		}
	}
	// e la conferma dell'albero: senza il ✓ di una persona il tipo resta quello proposto, mai commerciale
	for _, proposto := range []db.TipoComponente{db.TipoComponenteSciolto, db.TipoComponenteSottoassieme} {
		if got := tipoConfermatoNellAlbero(proposto, false); got != proposto {
			t.Errorf("tipoConfermatoNellAlbero(%s, senza il ✓) = %s", proposto, got)
		}
	}
}
