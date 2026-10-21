package sorgente

// L1 — la traccia del Motore A nel motivo della riga di S8 (giro G39-bis, integrazione, [T-39 da ratificare]): il testo
// con le evidenze, il limite di 500 caratteri, la causa sorgente_errata in testa con la correzione della sorgente, e il
// riconoscimento della traccia nel reinvio dello stesso corpo. Codici inventati (ACME).

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1G39bisLaTracciaDelMotoreNelMotivo(t *testing.T) {
	ev := []propostabom.EvidenzaCandidatura{
		{Selettore: "nome_file", CodiceLetto: "ACM-100", EsitoBase: "uguale", EsitoMarcatore: "compatibile"},
		{Selettore: propostabom.SelettoreStrutturaCandidata, CodiceLetto: "ACM-1000", EsitoBase: "uguale"},
		{},
	}
	got := tracciaCodiceDalMotore(" ACM-100 ", "ACM-1000", ev)
	want := "codice dal Motore A: letto «ACM-100», prodotto «ACM-1000»; candidatura univoca dello STEP del finito; evidenze: " +
		"nome_file ACM-100 (base uguale, marcatore compatibile); struttura_candidata ACM-1000 (base uguale)"
	if got != want {
		t.Fatalf("traccia:\navuto  %q\natteso %q", got, want)
	}
	if got := tracciaCodiceDalMotore("ACM-100", "ACM-1000", nil); got != "codice dal Motore A: letto «ACM-100», prodotto «ACM-1000»; candidatura univoca dello STEP del finito" {
		t.Fatalf("traccia senza evidenze: %q", got)
	}
	lunghe := make([]propostabom.EvidenzaCandidatura, 40)
	for i := range lunghe {
		lunghe[i] = propostabom.EvidenzaCandidatura{Selettore: "nome_file", CodiceLetto: strings.Repeat("X", 20), EsitoBase: "uguale"}
	}
	if n := len([]rune(tracciaCodiceDalMotore("ACM-100", "ACM-1000", lunghe))); n != 500 {
		t.Fatalf("la traccia lunga si ferma a 500 caratteri: %d", n)
	}

	traccia := tracciaCodiceDalMotore("ACM-100", "ACM-1000", ev)
	semplice := RichiestaS8{}
	if m := semplice.motivoConTraccia(traccia); m != traccia {
		t.Fatalf("senza il motivo dell'utente il motivo e' la traccia: %q", m)
	}
	if !semplice.stessoMotivoDelPasso2(traccia) || !semplice.stessoMotivoDelPasso2("") {
		t.Fatal("il reinvio senza motivo riconosce la riga con la traccia e quella senza motivo")
	}
	if semplice.stessoMotivoDelPasso2("un motivo dell'utente") {
		t.Fatal("una riga con il motivo di un altro corpo non e' lo stesso corpo")
	}
	conMotivo := RichiestaS8{Motivo: "scritto dall'utente"}
	if m := conMotivo.motivoConTraccia(traccia); m != "scritto dall'utente" {
		t.Fatalf("il motivo dell'utente vince sulla traccia: %q", m)
	}
	if conMotivo.stessoMotivoDelPasso2(traccia) {
		t.Fatal("un corpo con il motivo non riconosce la riga con la traccia")
	}
	correzione := RichiestaS8{CorrezioneSorgente: true}
	m := correzione.motivoConTraccia(traccia)
	if m != "sorgente_errata: "+traccia {
		t.Fatalf("con la correzione della sorgente la causa resta in testa: %q", m)
	}
	if !correzione.stessoMotivoDelPasso2(m) || !correzione.stessoMotivoDelPasso2("sorgente_errata") || correzione.stessoMotivoDelPasso2(traccia) {
		t.Fatal("il reinvio della correzione della sorgente riconosce solo le righe con la causa")
	}
}

func TestV1G39bisLEsenzioneSoloPerLaCandidaturaUnivoca(t *testing.T) {
	a, p, altro := uuid.New(), uuid.New(), uuid.New()
	ev := []propostabom.EvidenzaCandidatura{{Selettore: "nome_file", CodiceLetto: "ACM-100"}}
	prep := func(sc *distintabozza.StepCandidati) Preparazione {
		return Preparazione{Proposta: &distintabozza.PropostaCalcolata{StepCandidati: sc}}
	}
	casi := []struct {
		nome    string
		prep    Preparazione
		univoca bool
	}{
		{"univoca del prodotto", prep(&distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{a: {p}},
			Univoche: map[uuid.UUID]uuid.UUID{a: p}, Evidenze: map[uuid.UUID][]propostabom.EvidenzaCandidatura{a: ev}}), true},
		{"non univoca del solo prodotto", prep(&distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{a: {p}}}), false},
		{"univoca di un altro prodotto", prep(&distintabozza.StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{a: {altro}},
			Univoche: map[uuid.UUID]uuid.UUID{a: altro}}), false},
		{"ripiego", prep(&distintabozza.StepCandidati{Prodotti: map[uuid.UUID][]uuid.UUID{a: {p}}, Univoche: map[uuid.UUID]uuid.UUID{a: p}}), false},
		{"nessun calcolo", prep(nil), false},
		{"nessuna proposta", Preparazione{}, false},
	}
	for _, c := range casi {
		got := candidatoDelMotore(c.prep, a, p)
		if got.univoca != c.univoca {
			t.Fatalf("%s: univoca %v, attesa %v", c.nome, got.univoca, c.univoca)
		}
		if c.univoca && (len(got.evidenze) != 1 || got.evidenze[0].CodiceLetto != "ACM-100") {
			t.Fatalf("%s: le evidenze della candidatura: %+v", c.nome, got.evidenze)
		}
	}
}
