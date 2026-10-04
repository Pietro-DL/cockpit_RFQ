package evidenze

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// L1 — il tipo delle diagnostiche (A1a-VOC; parte 1 §7.3, R41 b): una forma JSON fissa con i nomi italiani,
// quattro nature e tre gravità chiuse, e un ErroreContratto che porta le diagnostiche e le dice nel testo.
//
// I clienti di questi test sono inventati: il repository è pubblico, e una diagnostica non ha bisogno di un
// cliente vero per dire di che cosa si lamenta.

// TestA1aVOCDiagnosticaInJSON: i nomi dei campi e omitempty su percorso e riferimenti.
func TestA1aVOCDiagnosticaInJSON(t *testing.T) {
	d := Diagnostica{
		Codice:    CodiceSelettoreNonAmmesso,
		Gravita:   GravitaErrore,
		Natura:    NaturaContratto,
		Percorso:  "famiglie_codice[acme].forme[nome].selettori[0]",
		Messaggio: "coppia non ammessa",
		Rif:       []string{"u:nome"},
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	const attesi = `{"codice":"contratto.selettore_non_ammesso","gravita":"errore","natura":"contratto","percorso":"famiglie_codice[acme].forme[nome].selettori[0]","messaggio":"coppia non ammessa","rif":["u:nome"]}`
	if string(b) != attesi {
		t.Fatalf("%s\natteso %s", b, attesi)
	}
	b, _ = json.Marshal(Diagnostica{Codice: CodiceSelettoreGenerico, Gravita: GravitaAvviso, Natura: NaturaCapacita, Messaggio: "m"})
	if got := string(b); got != `{"codice":"contratto.selettore_generico","gravita":"avviso","natura":"capacita","messaggio":"m"}` {
		t.Fatalf("senza percorso e riferimenti: %s", got)
	}
}

// TestA1aVOCGravitaENaturaChiuse: tre gravità e quattro nature, con questi valori. Per ciò che il motore
// non sa non ci sono altri stati: si dice con una natura e un codice.
func TestA1aVOCGravitaENaturaChiuse(t *testing.T) {
	gravita := map[Gravita]string{GravitaErrore: "errore", GravitaAvviso: "avviso", GravitaNota: "nota"}
	for g, s := range gravita {
		if string(g) != s {
			t.Errorf("gravità %q, attesa %q", g, s)
		}
	}
	nature := map[Natura]string{NaturaContratto: "contratto", NaturaDati: "dati", NaturaCapacita: "capacita", NaturaLimite: "limite"}
	for n, s := range nature {
		if string(n) != s {
			t.Errorf("natura %q, attesa %q", n, s)
		}
	}
	if len(gravita) != 3 || len(nature) != 4 {
		t.Fatalf("valori ripetuti: %d gravità, %d nature", len(gravita), len(nature))
	}
}

// TestA1aVOCErroreContratto: è un error, si ritrova con errors.As e il testo dice codici e percorsi.
func TestA1aVOCErroreContratto(t *testing.T) {
	var err error = &ErroreContratto{Diagnostiche: []Diagnostica{
		{Codice: CodiceSelettoreNonAmmesso, Gravita: GravitaErrore, Natura: NaturaContratto, Percorso: "campo", Messaggio: "uno"},
		{Codice: CodiceSelettoreGenerico, Gravita: GravitaErrore, Natura: NaturaContratto, Messaggio: "due"},
	}}
	var ec *ErroreContratto
	if !errors.As(err, &ec) || len(ec.Diagnostiche) != 2 {
		t.Fatalf("errors.As: %v", err)
	}
	testo := err.Error()
	for _, parte := range []string{"contratto.selettore_non_ammesso [campo]: uno", "contratto.selettore_generico: due"} {
		if !strings.Contains(testo, parte) {
			t.Errorf("%q non contiene %q", testo, parte)
		}
	}
	if (&ErroreContratto{}).Error() == "" {
		t.Error("un ErroreContratto vuoto deve comunque dire qualcosa")
	}
}
