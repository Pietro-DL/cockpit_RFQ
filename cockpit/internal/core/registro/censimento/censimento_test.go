package censimento

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// L1 — Smistamento, giro 4, fase 4.17a: la raccolta delle forme, con righe ACME inventate. Le prove L4 (sola
// lettura sul database vero) sono in censimento_db_test.go.

var (
	acme  = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
	beta  = uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	altro = uuid.MustParse("00000000-0000-0000-0000-0000000000c1") // un cliente che l'ingresso non ha
	t1    = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	t2    = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	t3    = uuid.MustParse("00000000-0000-0000-0000-000000000003")
)

// regoleACME: la famiglia 712 e il riferimento «RDO» di nove cifre, come le scriverebbe l'Anagrafica.
const regoleACME = `{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}],
	"riferimento_rfq": {"regex": "RDO \\d{9}", "descrizione": "RDO ACME", "esempio": "RDO 400012345"}}`

// ingressoACME e' il caso di prova: tre RFQ di ACME, un messaggio orfano, un cliente senza regole (BETA) e una
// riga di un cliente che non c'e'.
//   - t1: pezzi 7120001 (prodotto), 7120010 (commerciale, proposto e confermato), 7120011 (particolare,
//     proposto commerciale e scartato); i file 7120001_PRT.stp (due volte), 7120001A_1.pdf (deciso 7120001 rev 1),
//     7120011.pdf (deciso 7120011), image001.png; i nodi 7120010 e 7120011_PRT, e un cordone senza id; due
//     mail della stessa RFQ con il riferimento RDO.
//   - t2: pezzi 7120002 (prodotto) e 7120012X1 (commerciale, senza proposta); il prodotto confermato 7120002
//     anche fra i codici decisi; i file 7120002_PRT.stp e 7120002_XYZ.stp; il cartiglio 7120012X1, due volte
//     dallo stesso file e una volta con l'OCR.
//   - t3: il pezzo 7120003, il prodotto confermato 7120004, il file P7120003.pdf, una mail con il corpo tagliato.
//   - orfano: una mail «Anfrage_» con 7120005 da un indirizzo del dominio non censito come buyer, e il file
//     7120099_1.stp.
//   - all'arrivo l'ingest ha letto la prima mail di t1 (salvando 7120001, 7120001_PRT dal nome dell'allegato e 7120099), l'orfana (con
//     un'estrazione vuota: le regole di allora non trovavano niente) e quella di t3; 7120001A_1.pdf ha la proposta di file 7120001A rev 1,
//     7120011.pdf la proposta 7120011 rev A.
//   - ogni file ha il suo contenuto; 7120001_PRT.stp e' lo stesso contenuto arrivato due volte.
func ingressoACME() Ingresso {
	comm, part, prod := db.TipoComponenteCommerciale, db.TipoComponenteSciolto, db.TipoComponenteFinito
	return Ingresso{
		SolaLettura: true,
		Clienti: []Cliente{
			{ID: acme, Nome: "ACME", Ragione: "Acme S.p.A.", Attivo: true, Regole: json.RawMessage(regoleACME)},
			{ID: beta, Nome: "BETA", Ragione: "Beta Esempio S.r.l.", Attivo: false},
		},
		Messaggi: []Messaggio{
			{Cliente: acme, Thread: t1, Oggetto: "RDO 400012345 - Richiesta offerta 7120001", Corpo: "Buongiorno,\nin allegato il disegno 7120001.\nSaluti",
				Allegati: []string{"7120001_PRT.stp"}, Mittente: "mario.rossi@acme.example", Interpretato: true, Estratto: true,
				Salvati: []string{"7120001", "7120001_PRT", "7120099"}},
			{Cliente: acme, Thread: t1, Oggetto: "R: RDO 400012345 - Richiesta offerta 7120001", Corpo: "Sollecito per 7120001",
				Mittente: "mario.rossi@acme.example"},
			{Cliente: acme, Oggetto: "Anfrage_400012348 7120005", Corpo: "", Mittente: "ufficio.acquisti@acme.example", ViaDominio: true, Interpretato: true,
				Estratto: true},
			// il corpo tagliato: il codice del corpo non entra nel confronto con l'arrivo (la query dei salvati
			// tiene solo oggetto e allegati), ma e' un codice della mail come gli altri
			{Cliente: acme, Thread: t3, Oggetto: "Disegni", Corpo: "vedi 7120007", Troncato: true, Interpretato: true, Estratto: true},
			{Cliente: beta, Oggetto: "Richiesta prezzi", Corpo: "Buongiorno"},
			{Cliente: altro, Thread: t1, Oggetto: "RDO 400012399 7120001"},
		},
		File: []File{
			{Cliente: acme, Thread: t1, Nome: "7120001_PRT.stp", Contenuto: "sha-prt1"},
			{Cliente: acme, Thread: t1, Nome: "7120001_PRT.stp", Contenuto: "sha-prt1"},
			{Cliente: acme, Thread: t1, Nome: "7120001A_1.pdf", Contenuto: "sha-a1", Deciso: true, Codice: "7120001", Rev: "1", Proposto: true,
				CodiceProposto: "7120001A", RevProposta: "1"},
			{Cliente: acme, Thread: t1, Nome: "7120011.pdf", Contenuto: "sha-11", Deciso: true, Codice: "7120011", Proposto: true,
				CodiceProposto: "7120011", RevProposta: "A"},
			{Cliente: acme, Thread: t1, Nome: "image001.png", Contenuto: "sha-img"},
			{Cliente: acme, Thread: t2, Nome: "7120002_PRT.stp", Contenuto: "sha-prt2"},
			{Cliente: acme, Thread: t2, Nome: "7120002_XYZ.stp", Contenuto: "sha-xyz"},
			{Cliente: acme, Thread: t3, Nome: "P7120003.pdf", Contenuto: "sha-p3"},
			{Cliente: acme, Nome: "7120099_1.stp", Contenuto: "sha-99"},
			{Cliente: altro, Thread: t1, Nome: "7120001_PRT.stp", Contenuto: "sha-prt1"},
		},
		Nodi: []Nodo{
			{Cliente: acme, Thread: t1, ID: "7120010", Nome: "7120010"},
			{Cliente: acme, Thread: t1, ID: "7120011_PRT", Nome: "7120011_PRT"},
			{Cliente: acme, Thread: t1, Nome: "FILLET_WELD_1"},
		},
		Campi: []Campo{
			{Cliente: acme, Thread: t2, Contenuto: "sha-1", Etichetta: "codice", Valore: "7120012X1"},
			{Cliente: acme, Thread: t2, Contenuto: "sha-1", Etichetta: "codice", Valore: "7120012X1"},
			{Cliente: acme, Thread: t2, Contenuto: "sha-1", Etichetta: "numero_disegno", Valore: "7l20012", OCR: true},
		},
		Pezzi: []Pezzo{
			{Cliente: acme, Thread: t1, Codice: "7120001", Nomi: []string{"SUPPORTO"}, Deciso: prod},
			{Cliente: acme, Thread: t1, Codice: "7120010", Nomi: []string{"VITE TE M8X20 UNI 5739 8.8"}, Deciso: comm, Proposto: comm, Motivo: "normato: UNI 5739"},
			{Cliente: acme, Thread: t1, Codice: "7120011", Nomi: []string{"PIASTRA SP.6"}, Deciso: part, Proposto: comm, Motivo: "probabile minuteria"},
			{Cliente: acme, Thread: t2, Codice: "7120002", Deciso: prod},
			{Cliente: acme, Thread: t2, Codice: "7120012X1", Nomi: []string{"DADO M8 UNI 5588", "DADO M8"}, Deciso: comm},
			{Cliente: acme, Thread: t3, Codice: "7120003", Deciso: part},
		},
		Decisi: []Deciso{
			{Cliente: acme, Thread: t2, Codice: "7120002"},
			{Cliente: acme, Thread: t3, Codice: "7120004"},
		},
		DominiNonCensiti: []Voce{{Testo: "fornitore-esempio.example", N: 3}},
	}
}

func schedaDi(t *testing.T, c Censimento, nome string) Scheda {
	t.Helper()
	for _, s := range c.Schede {
		if s.Cliente.Nome == nome {
			return s
		}
	}
	t.Fatalf("nessuna scheda per %s", nome)
	return Scheda{}
}

func formaDi(s Scheda, f string) (FormaVista, bool) {
	for _, x := range s.Forme {
		if x.Forma == f {
			return x, true
		}
	}
	return FormaVista{}, false
}

func aggiuntaDi(v []Aggiunta, testo string) (Aggiunta, bool) {
	for _, a := range v {
		if a.Testo == testo {
			return a, true
		}
	}
	return Aggiunta{}, false
}

func voceIn(v []Voce, testo string) (Voce, bool) {
	for _, x := range v {
		if x.Testo == testo {
			return x, true
		}
	}
	return Voce{}, false
}

// Le forme per fonte: ogni stringa una volta per RFQ (le due mail di t1 citano 7120001 una volta sola, il file
// arrivato due volte si conta una volta), le fonti nelle loro colonne, le righe di un cliente che non c'e'
// ignorate, il cartiglio letto con l'OCR contato a parte e non fra le forme.
func TestLeFormeSiContanoPerFonteUnaVoltaPerRfq(t *testing.T) {
	c := Aggrega(ingressoACME())
	if len(c.Schede) != 2 || c.Schede[0].Cliente.Nome != "ACME" || c.Schede[1].Cliente.Nome != "BETA" {
		t.Fatalf("schede: %+v", c.Schede)
	}
	if !c.SolaLettura || len(c.DominiNonCensiti) != 1 {
		t.Errorf("sola lettura e domini non censiti passano com'erano: %v %+v", c.SolaLettura, c.DominiNonCensiti)
	}
	s := schedaDi(t, c, "ACME")
	if s.NMessaggi != 4 || s.NFile != 7 || s.NNodi != 3 || s.NCampi != 1 || s.NCampiOCR != 1 || s.NPezzi != 6 {
		t.Errorf("righe lette: mail %d, file %d, nodi %d, campi %d (+%d OCR), pezzi %d", s.NMessaggi, s.NFile, s.NNodi, s.NCampi, s.NCampiOCR, s.NPezzi)
	}
	casi := []struct {
		forma  string
		per    map[string]int
		esempi []string
	}{
		{"9999999", map[string]int{FonteMail: 3, FonteNome: 1, FonteStep: 1, FonteDistinta: 6},
			[]string{"7120001", "7120002", "7120003"}},
		{"9999999_AAA", map[string]int{FonteNome: 3, FonteStep: 1}, []string{"7120001_PRT", "7120002_PRT", "7120002_XYZ"}},
		{"9999999A9", map[string]int{FonteCartiglio: 1, FonteDistinta: 1}, []string{"7120012X1"}},
		{"9999999A_9", map[string]int{FonteNome: 1}, []string{"7120001A_1"}},
		{"A9999999", map[string]int{FonteNome: 1}, []string{"P7120003"}},
		{"9999999_9", map[string]int{FonteNome: 1}, []string{"7120099_1"}},
		{"AAAAAA_AAAA_9", map[string]int{FonteStep: 1}, []string{"FILLET_WELD_1"}},
	}
	for _, x := range casi {
		f, ok := formaDi(s, x.forma)
		if !ok {
			t.Errorf("manca la forma %s", x.forma)
			continue
		}
		tot := 0
		for _, fo := range Fonti {
			if f.Per[fo] != x.per[fo] {
				t.Errorf("%s, %s: %d, atteso %d", x.forma, fo, f.Per[fo], x.per[fo])
			}
			tot += x.per[fo]
		}
		if f.Totale != tot || !slices.Equal(f.Esempi, x.esempi) {
			t.Errorf("%s: totale %d esempi %v, attesi %d %v", x.forma, f.Totale, f.Esempi, tot, x.esempi)
		}
	}
	if s.Forme[0].Forma != "9999999" {
		t.Errorf("prima la forma piu' vista: %s", s.Forme[0].Forma)
	}
	if _, ok := formaDi(s, "9A99999"); ok {
		t.Error("il cartiglio letto con l'OCR non entra fra le forme")
	}
	if b := schedaDi(t, c, "BETA"); b.NMessaggi != 1 || len(b.Forme) != 0 || b.Cliente.Attivo {
		t.Errorf("BETA: %+v", b)
	}
}

// Le code e le teste intorno a un codice deciso nella stessa RFQ: «_PRT» dopo tre pezzi diversi in due RFQ e'
// un alias candidato; «A_1» e «_XYZ», viste una volta, no (niente refusi); «P» e' una testa. Le stringhe
// identiche a un codice deciso sono «uguali», e l'orfano non ha codici decisi intorno.
func TestLeCodeDopoUnCodiceDecisoSonoAliasSoloConDuePezziEDueRfq(t *testing.T) {
	s := schedaDi(t, Aggrega(ingressoACME()), "ACME")
	prt, ok := aggiuntaDi(s.Code, "_PRT")
	if !ok || !prt.Alias || prt.N != 3 || prt.Pezzi != 3 || prt.RFQ != 2 || prt.Forma != "_AAA" ||
		!slices.Equal(prt.Fonti, []string{FonteNome, FonteStep}) || !slices.Equal(prt.Esempi, []string{"7120001_PRT", "7120002_PRT", "7120011_PRT"}) {
		t.Errorf("_PRT: %+v", prt)
	}
	for _, testo := range []string{"A_1", "_XYZ"} {
		a, ok := aggiuntaDi(s.Code, testo)
		if !ok || a.Alias || a.N != 1 || a.Pezzi != 1 || a.RFQ != 1 {
			t.Errorf("%s: vista una volta, non e' un alias: %+v", testo, a)
		}
	}
	if len(s.Code) != 3 || s.Code[0].Testo != "_PRT" {
		t.Errorf("prima gli alias candidati, e nessun'altra coda: %+v", s.Code)
	}
	if p, ok := aggiuntaDi(s.Teste, "P"); !ok || len(s.Teste) != 1 || p.Alias || p.Forma != "A" {
		t.Errorf("teste: %+v", s.Teste)
	}
	if s.Uguali != 3 {
		t.Errorf("uguali a un codice deciso (7120011.pdf, il nodo 7120010, il cartiglio 7120012X1): %d", s.Uguali)
	}
}

// La posta con il motore di oggi: i codici proponibili della famiglia, il riferimento della regola del cliente,
// gli oggetti ridotti a forma (senza «R:»), i numeri accanto alle parole di riferimento, i mittenti del dominio
// che non sono buyer.
func TestLaPostaConIlMotoreDiOggi(t *testing.T) {
	s := schedaDi(t, Aggrega(ingressoACME()), "ACME")
	m := s.Mail
	if m.Messaggi != 4 || m.ConCodice != 4 || m.ConRiferimento != 2 {
		t.Errorf("mail: %+v", m)
	}
	if m.Proponibili < 3 {
		t.Errorf("codici proponibili: almeno 7120001 due volte e 7120005: %d", m.Proponibili)
	}
	if v, ok := voceIn(m.FormeRiferimento, "AAA 999999999"); !ok || v.N != 2 || !slices.Equal(v.Esempi, []string{"RDO 400012345"}) {
		t.Errorf("forme del riferimento: %+v", m.FormeRiferimento)
	}
	if v, ok := voceIn(s.Oggetti, "RDO 999999999 - RICHIESTA OFFERTA 9999999"); !ok || v.N != 2 {
		t.Errorf("oggetti: %+v", s.Oggetti)
	}
	if _, ok := voceIn(s.Oggetti, "ANFRAGE_999999999 9999999"); !ok {
		t.Errorf("oggetto dell'orfano: %+v", s.Oggetti)
	}
	if v, ok := voceIn(s.Riferimenti, "RDO 999999999"); !ok || v.N != 2 || !slices.Equal(v.Esempi, []string{"400012345"}) {
		t.Errorf("riferimenti RDO: %+v", s.Riferimenti)
	}
	if v, ok := voceIn(s.Riferimenti, "ANFRAGE 999999999"); !ok || v.N != 1 || !slices.Equal(v.Esempi, []string{"400012348"}) {
		t.Errorf("riferimenti Anfrage: %+v", s.Riferimenti)
	}
	if len(s.Mittenti) != 1 || s.Mittenti[0].Testo != "ufficio.acquisti@acme.example" || s.Mittenti[0].N != 1 {
		t.Errorf("mittenti del dominio non censiti come buyer: %+v", s.Mittenti)
	}
}

// I nomi dei file con il motore di oggi (solo dal nome, come l'ingest), accanto alla decisione della persona:
// 7120001A_1.pdf si legge 7120001A rev 1 e la persona ha deciso 7120001 rev 1, 7120011.pdf si legge com'e'
// stato deciso. L'immagine della firma non e' un file tecnico.
func TestINomiDeiFileLettiDalMotoreAccantoAllaDecisione(t *testing.T) {
	n := schedaDi(t, Aggrega(ingressoACME()), "ACME").Nome
	if n.File != 7 || n.Confrontati != 2 || n.CodiceUguale != 1 || n.RevUguale != 2 {
		t.Errorf("letture del nome: %+v", n)
	}
	if len(n.Diversi) != 1 || n.Diversi[0] != (Differenza{Nome: "7120001A_1.pdf", Letto: "7120001A rev 1", Deciso: "7120001 rev 1"}) {
		t.Errorf("differenze: %+v", n.Diversi)
	}
	tot := 0
	for _, l := range n.Letture {
		tot += l.N
	}
	if tot != 7 {
		t.Errorf("ogni file tecnico ha una lettura: %d", tot)
	}
	if v, ok := voceIn(n.Letture, "9999999A · rev 9"); !ok || !slices.Contains(v.Esempi, "7120001A_1.pdf") {
		t.Errorf("la lettura di 7120001A_1.pdf: %+v", n.Letture)
	}
	if Tecnico("image001.png") || !Tecnico("7120001A_1.STP") || !Tecnico(" 7120001.pdf ") {
		t.Error("Tecnico: le immagini no, disegni e modelli si', senza distinguere maiuscole")
	}
}

// Le firme dei commerciali decisi: la forma con le lettere (la X di 7120012X1) e le parole dei nomi, contate
// per pezzo, accanto agli altri pezzi decisi. Solo le parole che un commerciale porta.
func TestLeFirmeDeiCommercialiDecisi(t *testing.T) {
	f := schedaDi(t, Aggrega(ingressoACME()), "ACME").Firme
	if f.Commerciali != 2 || f.Altri != 4 {
		t.Errorf("pezzi: %d commerciali, %d altri", f.Commerciali, f.Altri)
	}
	attese := []Firma{
		{Testo: "9999999X9", Commerciali: 1, Altri: 0, Esempi: []string{"7120012X1"}},
		{Testo: "9999999", Commerciali: 1, Altri: 4, Esempi: []string{"7120010"}},
	}
	if !reflect.DeepEqual(f.Forme, attese) {
		t.Errorf("forme: %+v", f.Forme)
	}
	var parole []string
	for _, p := range f.Parole {
		parole = append(parole, p.Testo)
	}
	if !slices.Equal(parole, []string{"UNI", "DADO", "TE", "VITE"}) || f.Parole[0].Commerciali != 2 || f.Parole[0].Altri != 0 {
		t.Errorf("parole: %+v", f.Parole)
	}
	if got := Parole("Vite TE M8x20 UNI 5739 8.8 - vite"); !slices.Equal(got, []string{"TE", "UNI", "VITE"}) {
		t.Errorf("Parole: %v", got)
	}
}

// Il motore di oggi accanto a quello dell'arrivo. Sulle mail che l'ingest ha letto: 7120099, salvato all'arrivo,
// oggi non si trova piu' (perso); 7120005 e il numero generico dell'orfana oggi si trovano e allora no (nuovi);
// la mail di seconda mano non letta dall'ingest non conta, e il codice del corpo tagliato non entra nel
// confronto. Sui file: la proposta salvata uguale alla lettura del nome e quella diversa.
func TestIlMotoreDiOggiAccantoAQuelloDellArrivo(t *testing.T) {
	s := schedaDi(t, Aggrega(ingressoACME()), "ACME")
	m := s.Mail
	if m.Interpretati != 3 || m.Salvati != 3 || m.Nuovi != 2 || m.Persi != 1 || m.SenzaEstrazione != 0 {
		t.Errorf("accanto all'arrivo: %+v", m)
	}
	if !slices.Equal(m.EsempiNuovi, []string{"7120005", "ANFRAGE_400012348"}) || !slices.Equal(m.EsempiPersi, []string{"7120099"}) {
		t.Errorf("esempi: nuovi %v, persi %v", m.EsempiNuovi, m.EsempiPersi)
	}
	n := s.Nome
	if n.ConProposta != 2 || n.PropostaUguale != 1 || len(n.ProposteDiverse) != 1 ||
		n.ProposteDiverse[0] != (Differenza{Nome: "7120011.pdf", Letto: "7120011", Deciso: "7120011 rev A"}) {
		t.Errorf("proposte di file: %+v", n)
	}
}

// La minuteria, proposto e deciso. Con le proposte (la 4.4a.1, qui inventate) si contano le confermate, le
// scartate dall'ingegnere (i «mancati», con il motivo) e i commerciali decisi senza proposta; senza proposte
// (oggi: Leggi non ne porta) tutti i commerciali decisi sono «non proposti» e Proposte resta falso.
func TestLaMinuteriaPropostaEDecisa(t *testing.T) {
	m := schedaDi(t, Aggrega(ingressoACME()), "ACME").Minuteria
	if !m.Proposte || m.Confermate != 1 || m.Scartate != 1 || m.NonProposte != 1 || m.Altri != 3 {
		t.Errorf("con le proposte: %+v", m)
	}
	if len(m.Scarti) != 1 || m.Scarti[0] != (RigaMinuteria{Codice: "7120011", Nome: "PIASTRA SP.6", Deciso: db.TipoComponenteSciolto,
		Proposto: db.TipoComponenteCommerciale, Motivo: "probabile minuteria"}) {
		t.Errorf("scarti: %+v", m.Scarti)
	}
	if m.Scarti[0].NomeDeciso() != "particolare" || m.Scarti[0].NomeProposto() != "particolare commerciale" {
		t.Errorf("i nomi dei tipi: %q %q", m.Scarti[0].NomeDeciso(), m.Scarti[0].NomeProposto())
	}
	if len(m.NonViste) != 1 || m.NonViste[0].Codice != "7120012X1" || m.NonViste[0].Nome != "DADO M8 UNI 5588" {
		t.Errorf("non proposti: %+v", m.NonViste)
	}

	in := ingressoACME()
	for i := range in.Pezzi {
		in.Pezzi[i].Proposto, in.Pezzi[i].Motivo = "", ""
	}
	m = schedaDi(t, Aggrega(in), "ACME").Minuteria
	if m.Proposte || m.Confermate != 0 || m.Scartate != 0 || m.NonProposte != 2 || m.Altri != 4 || len(m.Scarti) != 0 {
		t.Errorf("senza proposte: %+v", m)
	}
}

// ingressoConRevisione e' ingressoACME con i casi in cui l'ordine delle righe potrebbe scegliere: in t1 il file
// 7120012.pdf arriva due volte con due contenuti (la seconda e' una revisione, decisa e proposta), e la
// revisione arriva ancora una volta uguale; in t1 lo STEP 7120001_PRT.stp torna con il nome in minuscolo
// (7120001_prt.stp, lo stesso contenuto); in t2 il campo 7120012X1 del cartiglio si legge anche con l'OCR;
// due pezzi con lo stesso codice e lo stesso nome in due RFQ, scartati con due motivi diversi.
func ingressoConRevisione() Ingresso {
	in := ingressoACME()
	rev := File{Cliente: acme, Thread: t1, Nome: "7120012.pdf", Contenuto: "sha-12b", Deciso: true, Codice: "7120012", Rev: "B",
		Proposto: true, CodiceProposto: "7120012"}
	in.File = append(in.File, File{Cliente: acme, Thread: t1, Nome: "7120012.pdf", Contenuto: "sha-12a"}, rev, rev,
		File{Cliente: acme, Thread: t1, Nome: "7120001_prt.stp", Contenuto: "sha-prt1"})
	in.Campi = append(in.Campi, Campo{Cliente: acme, Thread: t2, Contenuto: "sha-1", Etichetta: "codice", Valore: "7120012X1", OCR: true})
	in.Pezzi = append(in.Pezzi,
		Pezzo{Cliente: acme, Thread: t2, Codice: "7120013", Nomi: []string{"SPINA"}, Deciso: db.TipoComponenteSciolto,
			Proposto: db.TipoComponenteCommerciale, Motivo: "normato"},
		Pezzo{Cliente: acme, Thread: t3, Codice: "7120013", Nomi: []string{"SPINA"}, Deciso: db.TipoComponenteSciolto,
			Proposto: db.TipoComponenteCommerciale, Motivo: "probabile minuteria"})
	return in
}

// Un file rimandato con lo stesso nome e un contenuto nuovo (una revisione) e' un altro file, con la sua
// decisione: la forma e la lettura del nome si contano una volta per RFQ, il file e il confronto con la
// decisione e con la proposta una volta per contenuto. Lo stesso contenuto arrivato due volte resta uno; con il
// nome scritto con altre maiuscole e' un altro allegato, ma la sua forma e' la stessa stringa e non si conta
// di nuovo (ne diventa un esempio). La lettura dell'OCR e quella nativa dello stesso valore del cartiglio sono
// due letture.
func TestUnFileRimandatoConLoStessoNomeEUnAltroFile(t *testing.T) {
	s := schedaDi(t, Aggrega(ingressoConRevisione()), "ACME")
	if s.NFile != 10 || s.Nome.File != 9 {
		t.Errorf("file %d (uno per nome e contenuto: 7 + le due versioni di 7120012.pdf + 7120001_prt.stp), nomi letti %d (7 + due)", s.NFile, s.Nome.File)
	}
	if f, ok := formaDi(s, "9999999"); !ok || f.Per[FonteNome] != 2 {
		t.Errorf("la forma del nome 7120012 si conta una volta, accanto a 7120011: %+v", f)
	}
	if f, ok := formaDi(s, "9999999_AAA"); !ok || f.Per[FonteNome] != 3 || !slices.Equal(f.Esempi, []string{"7120001_PRT", "7120001_prt", "7120002_PRT"}) {
		t.Errorf("7120001_prt e 7120001_PRT sono la stessa stringa della RFQ, e tutte e due esempi: %+v", f)
	}
	n := s.Nome
	if n.Confrontati != 3 || n.CodiceUguale != 2 || n.RevUguale != 2 {
		t.Errorf("la revisione decisa entra nel confronto, una volta: %+v", n)
	}
	if len(n.Diversi) != 2 || n.Diversi[1] != (Differenza{Nome: "7120012.pdf", Letto: "7120012", Deciso: "7120012 rev B"}) {
		t.Errorf("differenze: %+v", n.Diversi)
	}
	if n.ConProposta != 3 || n.PropostaUguale != 2 {
		t.Errorf("la proposta della revisione, una volta: %+v", n)
	}
	if s.NCampi != 1 || s.NCampiOCR != 2 {
		t.Errorf("campi del cartiglio: %d nativi, %d con l'OCR (7l20012 e 7120012X1)", s.NCampi, s.NCampiOCR)
	}
	if m := s.Minuteria; m.Scartate != 3 || len(m.Scarti) != 3 || m.Scarti[1].Motivo != "normato" || m.Scarti[2].Motivo != "probabile minuteria" {
		t.Errorf("scarti con lo stesso codice e lo stesso nome, in ordine di motivo: %+v", m.Scarti)
	}
}

// Lo stesso ingresso in un altro ordine da' lo stesso censimento e lo stesso export: due export dello stesso
// database si confrontano con un diff, e le righe cambiate devono essere solo quelle del codice. Dentro ci
// sono i casi in cui l'ordine potrebbe scegliere (ingressoConRevisione): la revisione decisa di un file con lo
// stesso nome di un altro, la stessa lettura del cartiglio nativa e con l'OCR, due scarti uguali tranne il
// motivo. L'ordine inverso e qualche mescolata a seme fisso.
func TestLOrdineDelleRigheNonCambiaIlCensimento(t *testing.T) {
	a := Aggrega(ingressoConRevisione())
	if s := schedaDi(t, a, "ACME"); s.Nome.Confrontati != 3 {
		t.Fatalf("il caso della revisione non c'e': confrontati %d", s.Nome.Confrontati)
	}
	ora := time.Date(2026, 9, 29, 18, 0, 0, 0, time.UTC)
	confronta := func(nome string, in Ingresso) {
		t.Helper()
		b := Aggrega(in)
		if !reflect.DeepEqual(a, b) {
			sa, sb := schedaDi(t, a, "ACME"), schedaDi(t, b, "ACME")
			t.Errorf("%s: l'ordine delle righe ha cambiato il censimento (confrontati %d → %d, campi %d+%d → %d+%d)", nome,
				sa.Nome.Confrontati, sb.Nome.Confrontati, sa.NCampi, sa.NCampiOCR, sb.NCampi, sb.NCampiOCR)
		}
		if Markdown(a, ora) != Markdown(b, ora) {
			t.Errorf("%s: l'ordine delle righe ha cambiato l'export", nome)
		}
	}
	in := ingressoConRevisione()
	slices.Reverse(in.Messaggi)
	slices.Reverse(in.File)
	slices.Reverse(in.Nodi)
	slices.Reverse(in.Campi)
	slices.Reverse(in.Pezzi)
	slices.Reverse(in.Decisi)
	confronta("ordine inverso", in)
	for seme := uint64(1); seme <= 20; seme++ {
		r := rand.New(rand.NewPCG(seme, 4017))
		in := ingressoConRevisione()
		mescola(r, in.Messaggi)
		mescola(r, in.File)
		mescola(r, in.Nodi)
		mescola(r, in.Campi)
		mescola(r, in.Pezzi)
		mescola(r, in.Decisi)
		confronta(fmt.Sprintf("mescolata %d", seme), in)
	}
}

func mescola[T any](r *rand.Rand, v []T) {
	r.Shuffle(len(v), func(i, j int) { v[i], v[j] = v[j], v[i] })
}

// Le mail lette all'arrivo con «ignora» e senza codici salvati (i rami del triage che non estraggono: la
// controparte ambigua, la posta di un cliente che non e' di lavoro) non entrano nel confronto con l'arrivo:
// con lo stesso motore, ogni codice che oggi ci si trova sembrerebbe nuovo. Si contano a parte, e restano
// mail del cliente come le altre. Una mail letta con un'estrazione vuota (le regole di allora non trovavano
// niente) resta nel confronto, e i codici di oggi sono nuovi davvero.
func TestLeMailLetteSenzaEstrazioneRestanoFuoriDalConfronto(t *testing.T) {
	in := Ingresso{
		Clienti: []Cliente{{ID: acme, Nome: "ACME", Ragione: "Acme S.p.A.", Attivo: true, Regole: json.RawMessage(regoleACME)}},
		Messaggi: []Messaggio{
			{Cliente: acme, Thread: t1, Oggetto: "Risposta automatica: RDO 400012345 - 7120001", Corpo: "Sono fuori ufficio fino al 12/10.",
				Interpretato: true},
			{Cliente: acme, Thread: t2, Oggetto: "Richiesta offerta 7120002", Interpretato: true, Estratto: true},
			{Cliente: acme, Thread: t3, Oggetto: "Disegni 7120003", Interpretato: true, Estratto: true, Salvati: []string{"7120003"}},
		},
	}
	m := schedaDi(t, Aggrega(in), "ACME").Mail
	if m.Messaggi != 3 || m.Interpretati != 2 || m.SenzaEstrazione != 1 || m.Salvati != 1 || m.Nuovi != 1 || m.Persi != 0 ||
		!slices.Equal(m.EsempiNuovi, []string{"7120002"}) {
		t.Errorf("accanto all'arrivo: %+v", m)
	}
	if m.ConCodice != 3 || m.ConRiferimento != 1 {
		t.Errorf("la mail senza estrazione resta una mail del cliente: %+v", m)
	}
	md := Markdown(Aggrega(in), time.Time{})
	for _, atteso := range []string{
		"sulle 2 mail lette dall'ingest con un'estrazione salvata: codici salvati allora 1; trovati oggi e non allora 1 (`7120002`)",
		"fuori dal confronto (il triage puo' non averle estratte: controparte ambigua, posta non di lavoro): 1.",
	} {
		if !strings.Contains(md, atteso) {
			t.Errorf("manca %q nell'export", atteso)
		}
	}
}

// L'export: le sezioni di ogni cliente, le celle che non rompono la tabella (una barra verticale, un a capo),
// i codici fra apici inversi (un «_» non diventa un corsivo), la riga sulla sola lettura.
func TestLExportMarkdown(t *testing.T) {
	in := ingressoACME()
	in.Pezzi[4].Nomi = []string{"DADO | M8\nUNI 5588"}
	in.Pezzi[4].Proposto = db.TipoComponenteCommerciale
	in.Pezzi[4].Deciso = db.TipoComponenteSciolto
	md := Markdown(Aggrega(in), time.Date(2026, 9, 29, 18, 0, 0, 0, time.UTC))
	for _, atteso := range []string{
		"# Forme viste\n",
		"letto il 2026-09-29 18:00",
		"transazione di sola lettura: sì",
		"| fornitore-esempio.example | 3 |",
		"## ACME — Acme S.p.A.\n",
		"## BETA — Beta Esempio S.r.l. (non attivo)\n",
		"| Forma | mail | nome | step | cartiglio | distinta | Totale | Esempi |",
		"| `9999999_AAA` | 0 | 3 | 1 | 0 | 0 | 4 | `7120001_PRT`, `7120002_PRT`, `7120002_XYZ` |",
		"| `_PRT` | `_AAA` | 3 | 3 | 2 | nome, step | **sì** |",
		"| `A_1` | `A_9` | 1 | 1 | 1 | nome | no |",
		"| `P` | `A` | 1 | 1 | 1 | nome | no |",
		"Riferimento del cliente trovato dalla sua regola in 2 mail.",
		"sulle 3 mail lette dall'ingest con un'estrazione salvata: codici salvati allora 3; trovati oggi e non allora 2 (`7120005`, `ANFRAGE_400012348`); salvati allora e non trovati oggi 1 (`7120099`).",
		"fuori dal confronto (il triage puo' non averle estratte: controparte ambigua, posta non di lavoro): 0.",
		"Nomi di file tecnici: 7 (uno per RFQ: la lettura dipende solo dal nome). Con un documento deciso che porta un codice: 2 file",
		"| `7120011.pdf` | `7120011` | `7120011 rev A` |",
		"| `7120001A_1.pdf` | `7120001A rev 1` | `7120001 rev 1` |",
		"| `9999999X9` | 0 | 1 |",
		"| `7120012X1` | DADO \\| M8 UNI 5588 | particolare | particolare commerciale |",
		"| `RDO 999999999` | 2 | `400012345` |",
		"| `ufficio.acquisti@acme.example` | 1 |",
	} {
		if !strings.Contains(md, atteso) {
			t.Errorf("manca %q nell'export", atteso)
		}
	}
	if strings.Contains(md, "7l20012") {
		t.Error("il cartiglio letto con l'OCR e' finito nell'export")
	}
	senza := Aggrega(ingressoACME())
	for i := range senza.Schede {
		senza.Schede[i].Minuteria.Proposte = false
	}
	if !strings.Contains(Markdown(senza, time.Time{}), "arriva con la fase 4.4a.1") {
		t.Error("senza proposte l'export dice che il tipo proposto arriva con la 4.4a.1")
	}
}

// Il riconoscimento dei numeri accanto alle parole di riferimento: le parole attaccate con «_», «n.», «nr:»;
// un numero di meno di tre cifre non e' un riferimento; una parola dentro un'altra parola non conta.
func TestIRiferimentiAccantoAlleParole(t *testing.T) {
	got := Riferimenti("Anfrage_400012348; CTE n. 26999999 e ordine nr: 4500000001. RFQ 12, economia 7120001, rdo-400012345 rdo 400012345")
	atteso := map[string][]string{
		"ANFRAGE 999999999": {"400012348"},
		"CTE 99999999":      {"26999999"},
		"ORDINE 9999999999": {"4500000001"},
		"RDO 999999999":     {"400012345"},
	}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("riferimenti: %v", got)
	}
}

// I campi del cartiglio dai fatti di un'analisi, letti dalla classificazione (CampiIdentificativiDelPDF): solo
// codice e numero di disegno, anche fuori dalla zona in basso a destra, vuoti fuori, la fonte OCR segnata;
// fatti che non si leggono (il solo elenco dei campi, una versione sconosciuta) non fermano niente.
func TestICampiDelCartiglio(t *testing.T) {
	fatti := json.RawMessage(`{"testo_pdf": {"versione": 1, "estraibile": true, "cartiglio": [
		{"etichetta": "codice", "valore": " 7120001A ", "fonte": "nativo", "zona": "basso_destra"},
		{"etichetta": "numero_disegno", "valore": "7120001", "fonte": "ocr", "zona": "basso_destra"},
		{"etichetta": "numero_disegno", "valore": "7120010", "fonte": "nativo", "zona": "pagina"},
		{"etichetta": "revisione", "valore": "1", "fonte": "nativo", "zona": "basso_destra"},
		{"etichetta": "titolo", "valore": "SUPPORTO", "fonte": "nativo", "zona": "basso_destra"},
		{"etichetta": "codice", "valore": "  ", "fonte": "nativo", "zona": "basso_destra"}]}}`)
	got := CampiDelCartiglio(acme, t1, "sha-1", fatti)
	atteso := []Campo{
		{Cliente: acme, Thread: t1, Contenuto: "sha-1", Etichetta: "codice", Valore: "7120001A"},
		{Cliente: acme, Thread: t1, Contenuto: "sha-1", Etichetta: "numero_disegno", Valore: "7120001", OCR: true},
		{Cliente: acme, Thread: t1, Contenuto: "sha-1", Etichetta: "numero_disegno", Valore: "7120010"},
	}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("campi: %+v", got)
	}
	for nome, f := range map[string]string{
		"il solo elenco dei campi": `[{"etichetta": "codice", "valore": "7120001", "fonte": "nativo"}]`,
		"versione sconosciuta":     `{"testo_pdf": {"versione": 0, "cartiglio": [{"etichetta": "codice", "valore": "7120001", "fonte": "nativo"}]}}`,
		"non un elenco":            `{"non": "un elenco"}`,
	} {
		if c := CampiDelCartiglio(acme, t1, "sha-1", json.RawMessage(f)); c != nil {
			t.Errorf("%s: nessun campo, ne ha dati %+v", nome, c)
		}
	}
}
