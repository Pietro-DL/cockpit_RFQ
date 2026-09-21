package anagrafica

import (
	"strings"
	"testing"
)

// L1 — AN2: il cancello del seme.
//
// D17 dice che una regola il cui esempio non corrisponde non viene salvata. La schermata
// Anagrafica lo fa. Ma un seme è l'altra porta, e senza questo controllo lo stesso identico
// errore, scritto in un file invece che in un riquadro, entrerebbe in database senza che nessuno
// lo guardi — e ci resterebbe, riconoscendo niente, in silenzio.
//
// I clienti qui sono inventati: quelli veri sono un dato dell'azienda e il repository è pubblico.

const semeBuono = `{"clienti":[
 {"ragione_sociale":"ACME S.p.A.","cartella_nas":"ACME","lingua":"it","peso":12,
  "domini":["acme.example"],
  "buyer":[{"cognome":"Rossi","nome":"Mario","email":"MARIO.ROSSI@acme.example","tipo":"buyer"}],
  "regole":{"famiglie_codice":[{"regex":"\\bAC\\d{5}[A-Z]\\b","descrizione":"codici ACME","esempio":"AC12345B"}],
            "richiede_cbd":true}},
 {"ragione_sociale":"Beta S.r.l.","cartella_nas":"BETA","domini":["beta.example"]}
]}`

func leggi(t *testing.T, s string) (Seme, error) {
	t.Helper()
	return Leggi(strings.NewReader(s))
}

// AN2 — il seme non parte se anche una sola regola di un solo cliente ha un esempio che non
// corrisponde. Non «quel cliente viene saltato»: non parte niente, perché un seme a metà è la
// cosa più difficile da capire il giorno dopo.
func TestAN2IlSemeNonPartConUnEsempioCheNonCorrisponde(t *testing.T) {
	rotto := strings.Replace(semeBuono, `"esempio":"AC12345B"`, `"esempio":"XY99"`, 1)
	if rotto == semeBuono {
		t.Fatal("il test non ha sostituito niente: si sta provando due volte lo stesso file")
	}
	_, err := leggi(t, rotto)
	if err == nil {
		t.Fatal("seminata una regola con l'esempio che non corrisponde: sarebbe entrata in database senza spunte rosse e senza errori")
	}
	for _, atteso := range []string{"ACME", "non corrisponde"} {
		if !strings.Contains(err.Error(), atteso) {
			t.Errorf("l'errore non dice %q (non si saprebbe quale cliente correggere): %v", atteso, err)
		}
	}
}

// E le regole buone passano tutte: un cancello che blocca anche il buono non è un cancello.
func TestUnSemeBuonoPassaPerIntero(t *testing.T) {
	s, err := leggi(t, semeBuono)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Clienti) != 2 {
		t.Fatalf("clienti letti: %d", len(s.Clienti))
	}
	if s.Clienti[0].Peso != 12 || len(s.Clienti[0].Buyer) != 1 || len(s.Clienti[0].Domini) != 1 {
		t.Errorf("primo cliente letto male: %+v", s.Clienti[0])
	}
	// un cliente senza regole è legittimo: è come nascono tutti
	if len(s.Clienti[1].Regole) != 0 {
		t.Errorf("il secondo cliente non doveva avere regole: %s", s.Clienti[1].Regole)
	}
}

// Gli altri modi di scrivere un file che farebbe danno. Ognuno di questi, senza controllo, si
// scoprirebbe soltanto guardando il database.
func TestIlFileVieneControllatoPrimaDiScrivere(t *testing.T) {
	casi := []struct{ nome, file, atteso string }{
		{"due clienti nella stessa cartella NAS",
			`{"clienti":[{"ragione_sociale":"A","cartella_nas":"X"},{"ragione_sociale":"B","cartella_nas":"X"}]}`,
			"due volte"},
		{"lo stesso dominio a due clienti",
			`{"clienti":[{"ragione_sociale":"A","cartella_nas":"A","domini":["x.example"]},{"ragione_sociale":"B","cartella_nas":"B","domini":["x.example"]}]}`,
			"un cliente solo"},
		{"un indirizzo al posto di un dominio",
			`{"clienti":[{"ragione_sociale":"A","cartella_nas":"A","domini":["mario@x.example"]}]}`,
			"non un dominio"},
		{"peso fuori dal dominio",
			`{"clienti":[{"ragione_sociale":"A","cartella_nas":"A","peso":99}]}`,
			"0"},
		{"senza cartella NAS",
			`{"clienti":[{"ragione_sociale":"A"}]}`,
			"cartella_nas"},
		{"un campo inventato (di solito un nome scritto male)",
			`{"clienti":[{"ragione_sociale":"A","cartella_nas":"A","peso_cliente":3}]}`,
			"peso_cliente"},
		{"nessun cliente",
			`{"clienti":[]}`, "nessun cliente"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, err := leggi(t, c.file)
			if err == nil {
				t.Fatal("accettato")
			}
			if !strings.Contains(err.Error(), c.atteso) {
				t.Errorf("l'errore non dice %q: %v", c.atteso, err)
			}
		})
	}
}
