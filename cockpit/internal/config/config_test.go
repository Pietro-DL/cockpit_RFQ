package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scrivi(t *testing.T, corpo string) string {
	t.Helper()
	d := t.TempDir()
	p := filepath.Join(d, "cockpit.toml")
	base := "[db]\ndsn = \"postgres://x@localhost/y\"\n[server]\ntoken_worker = \"t\"\n[nas]\nstaging = " +
		"'" + filepath.Join(d, "staging") + "'\n"
	if err := os.WriteFile(p, []byte(base+corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Indirizzi e nomi host arrivano dal file scritti come capita: in DB devono entrare in una forma sola,
// altrimenti «Commerciale@…» e «commerciale@…» diventano due caselle diverse.
func TestNormalizzazioneDiCaselleEPostazioni(t *testing.T) {
	c, err := Carica(scrivi(t, `
[outlook]
casella_default = "Commerciale@Azienda.IT"

[[casella]]
indirizzo = "  Commerciale@Azienda.IT "
condivisa = true

[[postazione]]
nome_host = "pc-francesco"
utente = "fp"

[[worker]]
nome = "outlook@PC-FRANCESCO"
tipo = "outlook"
token = "segreto"
postazione = "pc-francesco"
caselle = ["COMMERCIALE@azienda.it"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Caselle[0].Indirizzo != "commerciale@azienda.it" {
		t.Errorf("indirizzo %q non normalizzato", c.Caselle[0].Indirizzo)
	}
	if c.Caselle[0].Nome != "commerciale" {
		t.Errorf("nome predefinito %q: atteso il pezzo prima della chiocciola", c.Caselle[0].Nome)
	}
	if c.Postazioni[0].NomeHost != "PC-FRANCESCO" || c.Postazioni[0].Utente != "FP" {
		t.Errorf("postazione non normalizzata: %+v", c.Postazioni[0])
	}
	if c.Worker[0].Postazione != "PC-FRANCESCO" || c.Worker[0].Caselle[0] != "commerciale@azienda.it" {
		t.Errorf("worker non normalizzato: %+v", c.Worker[0])
	}
	if c.Outlook.CasellaDefault != "commerciale@azienda.it" {
		t.Errorf("casella_default %q non normalizzata", c.Outlook.CasellaDefault)
	}
}

// Con una sola casella non ha senso chiedere di dichiarare quale sia la predefinita.
func TestCasellaDefaultDedottaConUnaSolaCasella(t *testing.T) {
	c, err := Carica(scrivi(t, "[[casella]]\nindirizzo = \"francesco@azienda.it\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Outlook.CasellaDefault != "francesco@azienda.it" {
		t.Errorf("casella_default %q: attesa l'unica casella dichiarata", c.Outlook.CasellaDefault)
	}
}

// Ogni errore qui è un errore di configurazione che deve fermare l'avvio: partire con un routing
// sbagliato significa mandare job alla postazione di un altro o attribuire messaggi alla casella
// sbagliata.
func TestConfigurazioniRifiutate(t *testing.T) {
	casi := []struct {
		nome, corpo, atteso string
	}{
		{
			"casella senza indirizzo",
			"[[casella]]\nnome = \"Senza\"\n",
			"senza indirizzo",
		},
		{
			"stessa casella due volte",
			"[[casella]]\nindirizzo = \"a@b.it\"\n[[casella]]\nindirizzo = \"A@B.it\"\n",
			"dichiarata due volte",
		},
		{
			"condivisa con proprietario",
			"[[casella]]\nindirizzo = \"c@b.it\"\ncondivisa = true\nutente = \"FP\"\n",
			"non può avere un proprietario",
		},
		{
			"più caselle senza predefinita",
			"[[casella]]\nindirizzo = \"a@b.it\"\n[[casella]]\nindirizzo = \"c@b.it\"\n",
			"serve [outlook].casella_default",
		},
		{
			"predefinita non censita",
			"[outlook]\ncasella_default = \"z@b.it\"\n[[casella]]\nindirizzo = \"a@b.it\"\n",
			"non è fra le [[casella]] dichiarate",
		},
		{
			"worker su postazione inesistente",
			"[[casella]]\nindirizzo = \"a@b.it\"\n[[worker]]\nnome = \"w\"\ntipo = \"outlook\"\ntoken = \"t\"\npostazione = \"PC-IGNOTO\"\n",
			"non dichiarata in [[postazione]]",
		},
		{
			"worker su casella non censita",
			"[[casella]]\nindirizzo = \"a@b.it\"\n[[worker]]\nnome = \"w\"\ntipo = \"outlook\"\ntoken = \"t\"\ncaselle = [\"z@b.it\"]\n",
			"non dichiarata in [[casella]]",
		},
		{
			"worker senza token",
			"[[worker]]\nnome = \"w\"\ntipo = \"outlook\"\n",
			"senza token",
		},
		{
			"tipo di worker inventato",
			"[[worker]]\nnome = \"w\"\ntipo = \"server\"\ntoken = \"t\"\n",
			"non valido",
		},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Carica(scrivi(t, c.corpo))
			if err == nil {
				t.Fatalf("configurazione accettata: doveva essere rifiutata (%s)", c.atteso)
			}
			if !strings.Contains(err.Error(), c.atteso) {
				t.Fatalf("atteso un errore che contenga %q, ottenuto: %v", c.atteso, err)
			}
		})
	}
}

// Senza sezioni nuove la configurazione di oggi deve continuare a funzionare tale e quale.
func TestConfigurazioneSenzaFondazioni(t *testing.T) {
	c, err := Carica(scrivi(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Caselle) != 0 || len(c.Postazioni) != 0 || len(c.Worker) != 0 || c.Outlook.CasellaDefault != "" {
		t.Errorf("sezioni assenti non devono inventare righe: %+v", c)
	}
}
