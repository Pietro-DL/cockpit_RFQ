package domain

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// CP6 — il resolver puro: contatto > dominio > sconosciuto, interno, ambiguo (blocco 7A, D33).

var (
	idEuroforesi = uuid.MustParse("11111111-0000-0000-0000-000000000001")
	idPolver     = uuid.MustParse("11111111-0000-0000-0000-000000000002")
	idLandini    = uuid.MustParse("22222222-0000-0000-0000-000000000001")
	idSame       = uuid.MustParse("22222222-0000-0000-0000-000000000002")
)

func rubricaDiProva() RubricaFissa {
	return RubricaFissa{
		Contatti: map[string][]Voce{
			"mario@gmail.com":          {{ID: idEuroforesi, Nome: "Euroforesi"}},
			"doppio@gruppo.example":    {{ID: idEuroforesi, Nome: "Euroforesi"}},
			"due@fornitori.example":    {{ID: idEuroforesi, Nome: "Euroforesi"}, {ID: idPolver, Nome: "Polver"}},
			"buyer@euroforesi.example": {{ID: idEuroforesi, Nome: "Euroforesi"}},
		},
		Buyer: map[string]Voce{
			"acquisti@landini.example": {ID: idLandini, Nome: "LANDINI ARGO"},
			"doppio@gruppo.example":    {ID: idLandini, Nome: "LANDINI ARGO"},
			"buyer@euroforesi.example": {ID: idSame, Nome: "SAME"},
		},
		DominiFornitore: map[string]Voce{
			"euroforesi.example": {ID: idEuroforesi, Nome: "Euroforesi"},
			"gruppo.example":     {ID: idPolver, Nome: "Polver"},
		},
		DominiCliente: map[string]Voce{
			"landini.example": {ID: idLandini, Nome: "LANDINI ARGO"},
			"gruppo.example":  {ID: idSame, Nome: "SAME"},
		},
	}
}

func nostro(indirizzo string) bool {
	return indirizzo == "commerciale@azienda.example" || indirizzo == "francesco@azienda.example"
}

func TestCP6LaControparteSiRisolveConLaPrecedenzaContattoDominioSconosciuto(t *testing.T) {
	r := rubricaDiProva()
	casi := []struct {
		nome        string
		mittente    string
		destinatari []string
		tipo, via   string
		cliente     uuid.UUID
		fornitore   uuid.UUID
		indirizzo   string
	}{
		{"1 contatto fornitore su dominio generico", "mario@gmail.com", nil, ControparteFornitore, ViaContatto, uuid.Nil, idEuroforesi, "mario@gmail.com"},
		{"2 buyer esatto", "acquisti@landini.example", nil, ControparteCliente, ViaContatto, idLandini, uuid.Nil, "acquisti@landini.example"},
		{"3 email in entrambe le anagrafiche", "doppio@gruppo.example", nil, ControparteAmbiguo, ViaContatto, uuid.Nil, uuid.Nil, "doppio@gruppo.example"},
		{"4 dominio fornitore", "chiunque@euroforesi.example", nil, ControparteFornitore, ViaDominio, uuid.Nil, idEuroforesi, "chiunque@euroforesi.example"},
		{"5 dominio cliente", "chiunque@landini.example", nil, ControparteCliente, ViaDominio, idLandini, uuid.Nil, "chiunque@landini.example"},
		{"6 dominio in entrambe", "x@gruppo.example", nil, ControparteAmbiguo, ViaDominio, uuid.Nil, uuid.Nil, "x@gruppo.example"},
		{"7 niente di censito", "nessuno@altrove.example", nil, ControparteSconosciuto, "", uuid.Nil, uuid.Nil, "nessuno@altrove.example"},
		{"8 il contatto vince sul dominio", "buyer@euroforesi.example", nil, ControparteAmbiguo, ViaContatto, uuid.Nil, uuid.Nil, "buyer@euroforesi.example"},
		{"9 mittente nostro, destinatari nostri", "commerciale@azienda.example", []string{"francesco@azienda.example"}, ControparteInterno, ViaCasella, uuid.Nil, uuid.Nil, "commerciale@azienda.example"},
		{"10 mittente nostro, primo destinatario esterno fornitore", "commerciale@azienda.example", []string{"ordini@euroforesi.example", "acquisti@landini.example"}, ControparteFornitore, ViaDominio, uuid.Nil, idEuroforesi, "ordini@euroforesi.example"},
		{"11 mittente nostro, salta i nostri e prende il cliente", "francesco@azienda.example", []string{"commerciale@azienda.example", "acquisti@landini.example"}, ControparteCliente, ViaContatto, idLandini, uuid.Nil, "acquisti@landini.example"},
		{"12 mittente senza chiocciola", "/O=EXCHANGE/OU=PRIMA", nil, ControparteSconosciuto, "", uuid.Nil, uuid.Nil, "/o=exchange/ou=prima"},
		{"13 maiuscole e spazi si normalizzano", "  MARIO@GMAIL.COM ", nil, ControparteFornitore, ViaContatto, uuid.Nil, idEuroforesi, "mario@gmail.com"},
		{"14 lo stesso contatto in due fornitori", "due@fornitori.example", nil, ControparteAmbiguo, ViaContatto, uuid.Nil, uuid.Nil, "due@fornitori.example"},
		{"15 mittente nostro senza destinatari", "commerciale@azienda.example", nil, ControparteInterno, ViaCasella, uuid.Nil, uuid.Nil, "commerciale@azienda.example"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			got, err := RisolviControparte(context.Background(), IngressoControparte{Mittente: c.mittente, Destinatari: c.destinatari, Nostro: nostro}, r)
			if err != nil {
				t.Fatal(err)
			}
			if got.Tipo != c.tipo || got.Via != c.via {
				t.Fatalf("tipo/via = %s/%q, atteso %s/%q (%s)", got.Tipo, got.Via, c.tipo, c.via, got.Motivo)
			}
			if got.ClienteID != c.cliente || got.FornitoreID != c.fornitore {
				t.Fatalf("id = cliente %s / fornitore %s, attesi %s / %s", got.ClienteID, got.FornitoreID, c.cliente, c.fornitore)
			}
			if got.Indirizzo != c.indirizzo {
				t.Fatalf("indirizzo deciso = %q, atteso %q", got.Indirizzo, c.indirizzo)
			}
			if got.Motivo == "" {
				t.Fatal("ogni risposta porta un motivo")
			}
			if (got.Tipo == ControparteCliente || got.Tipo == ControparteFornitore) && got.Nome == "" {
				t.Fatal("cliente e fornitore riconosciuti portano il nome")
			}
		})
	}
}

// Senza `Nostro` il mittente si risolve e basta: e' cio' che fa chi non sa quali sono le caselle.
func TestSenzaLElencoDeiNostriSiGuardaSoloIlMittente(t *testing.T) {
	got, err := RisolviControparte(context.Background(), IngressoControparte{Mittente: "commerciale@azienda.example",
		Destinatari: []string{"ordini@euroforesi.example"}}, rubricaDiProva())
	if err != nil {
		t.Fatal(err)
	}
	if got.Tipo != ControparteSconosciuto {
		t.Fatalf("senza i nostri il mittente nostro e' sconosciuto, non %s", got.Tipo)
	}
}

// Un errore della rubrica ferma la risoluzione: non diventa «sconosciuto» in silenzio.
type rubricaRotta struct{ RubricaFissa }

func (rubricaRotta) BuyerCliente(context.Context, string) (Voce, bool, error) {
	return Voce{}, false, errRubrica
}

var errRubrica = &erroreDiProva{"database non raggiungibile"}

type erroreDiProva struct{ s string }

func (e *erroreDiProva) Error() string { return e.s }

func TestUnErroreDellaRubricaNonDiventaSconosciuto(t *testing.T) {
	_, err := RisolviControparte(context.Background(), IngressoControparte{Mittente: "x@landini.example"}, rubricaRotta{rubricaDiProva()})
	if err == nil {
		t.Fatal("l'errore della rubrica deve arrivare a chi chiama")
	}
}
