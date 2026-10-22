package sorgente

// L1 — la traccia del gesto nel motivo della riga di S8 (messaggio 46 [U], 21/10, corsia MOTIVO; prima la traccia del Motore
// A del giro G39-bis): il testo «gesto di <sigla> da «Conferma la sorgente»: …» con le ragioni e le evidenze della
// candidatura univoca, il limite di 500 caratteri, la causa sorgente_errata in testa con la correzione della sorgente, e il
// riconoscimento della riga nel reinvio dello stesso corpo per confronto ESATTO con la traccia che il server genera (mai per
// prefisso). Codici inventati (ACME).

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1M46LaTracciaDelGestoNelMotivoDiS8(t *testing.T) {
	ev := []propostabom.EvidenzaCandidatura{
		{Selettore: "nome_file", CodiceLetto: "ACM-100", EsitoBase: "uguale", EsitoMarcatore: "compatibile"},
		{Selettore: propostabom.SelettoreStrutturaCandidata, CodiceLetto: "ACM-1000", EsitoBase: "uguale"},
		{},
	}
	g := RagioniS8{Sigla: "PS", NomeFile: "ACM-1000.stp", CodiceProdotto: "ACM-1000", Letto: " ACM-100 ", Univoca: true, Evidenze: ev}
	if !g.Serve() {
		t.Fatal("il codice letto dal file diverso dal prodotto vuole la traccia")
	}
	got := TracciaS8(g)
	want := "gesto di PS da «Conferma la sorgente»: lo STEP «ACM-1000.stp» è la sorgente di ACM-1000; codice letto dal file «ACM-100», il documento prende quello del prodotto; " +
		"candidatura univoca dello STEP del finito; evidenze: nome_file ACM-100 (base uguale, marcatore compatibile); struttura_candidata ACM-1000 (base uguale)"
	if got != want {
		t.Fatalf("traccia:\navuto  %q\natteso %q", got, want)
	}
	// senza candidatura univoca: le stesse ragioni, senza le evidenze del motore
	g2 := g
	g2.Univoca, g2.Evidenze = false, nil
	if got := TracciaS8(g2); got != "gesto di PS da «Conferma la sorgente»: lo STEP «ACM-1000.stp» è la sorgente di ACM-1000; codice letto dal file «ACM-100», il documento prende quello del prodotto" {
		t.Fatalf("traccia senza la candidatura univoca: %q", got)
	}
	// le altre ragioni, nell'ordine fisso
	tutte := RagioniS8{Sigla: "PS", NomeFile: "ACM-1000.stp", CodiceProdotto: "ACM-1000", SoloProdotto: true, StatoAnalisi: "pronta",
		Radice: propostabom.ConfrontoCodiceNonDeterminabile, LasciaA: "ACM-2000"}
	if got := TracciaS8(tutte); got != "gesto di PS da «Conferma la sorgente»: lo STEP «ACM-1000.stp» è la sorgente di ACM-1000; partenza dal solo prodotto (analisi: pronta); "+
		"codice della radice dello STEP non determinabile; lo STEP lascia ACM-2000" {
		t.Fatalf("traccia con le altre ragioni: %q", got)
	}
	radice := RagioniS8{Sigla: "PS", NomeFile: "ACM-1000.stp", CodiceProdotto: "ACM-1000", Radice: propostabom.ConfrontoCodiceDiverso, CodiceRadice: "ACM-100"}
	if got := TracciaS8(radice); !strings.HasSuffix(got, "codice della radice dello STEP diverso («ACM-100»)") {
		t.Fatalf("traccia del codice della radice: %q", got)
	}
	if (RagioniS8{Sigla: "PS", NomeFile: "x.stp", CodiceProdotto: "ACM-1000"}).Serve() {
		t.Fatal("senza ragioni la traccia non serve nella riga di S8")
	}
	lunghe := make([]propostabom.EvidenzaCandidatura, 40)
	for i := range lunghe {
		lunghe[i] = propostabom.EvidenzaCandidatura{Selettore: "nome_file", CodiceLetto: strings.Repeat("X", 20), EsitoBase: "uguale"}
	}
	g3 := g
	g3.Evidenze = lunghe
	if n := len([]rune(TracciaS8(g3))); n != MaxTracciaS8 {
		t.Fatalf("la traccia lunga si ferma a %d caratteri: %d", MaxTracciaS8, n)
	}

	traccia := TracciaS8(g)
	semplice := RichiestaS8{traccia: traccia}
	if m := semplice.motivoRegistrato(); m != traccia {
		t.Fatalf("senza il motivo dell'utente il motivo e' la traccia: %q", m)
	}
	if !semplice.stessoMotivoDelPasso2(traccia) {
		t.Fatal("il reinvio senza motivo riconosce la riga con la stessa traccia")
	}
	if semplice.stessoMotivoDelPasso2(traccia[:len(traccia)-10]) || semplice.stessoMotivoDelPasso2(traccia+" in piu'") || semplice.stessoMotivoDelPasso2("") {
		t.Fatal("una riga con una traccia diversa (anche solo per un pezzo) non e' lo stesso corpo: il confronto e' esatto, non per prefisso")
	}
	if semplice.stessoMotivoDelPasso2("un motivo dell'utente") {
		t.Fatal("una riga con il motivo di un altro corpo non e' lo stesso corpo")
	}
	senzaRagioni := RichiestaS8{}
	if !senzaRagioni.stessoMotivoDelPasso2("") || senzaRagioni.stessoMotivoDelPasso2(traccia) {
		t.Fatal("senza ragioni la riga resta senza motivo, e una riga con la traccia e' un altro corpo")
	}
	conMotivo := RichiestaS8{Motivo: "scritto dall'utente", traccia: traccia}
	if m := conMotivo.motivoRegistrato(); m != "scritto dall'utente" {
		t.Fatalf("il motivo dell'utente (client di prima) vince sulla traccia: %q", m)
	}
	if conMotivo.stessoMotivoDelPasso2(traccia) {
		t.Fatal("un corpo con il motivo non riconosce la riga con la traccia")
	}
	correzione := RichiestaS8{CorrezioneSorgente: true, traccia: traccia}
	m := correzione.motivoRegistrato()
	if m != "sorgente_errata: "+traccia {
		t.Fatalf("con la correzione della sorgente la causa resta in testa: %q", m)
	}
	if !correzione.stessoMotivoDelPasso2(m) || correzione.stessoMotivoDelPasso2("sorgente_errata") || correzione.stessoMotivoDelPasso2(traccia) {
		t.Fatal("il reinvio della correzione della sorgente riconosce solo la riga con la causa e la stessa traccia")
	}
	// l'evento del cambio della fonte ha sempre un motivo: la traccia base
	base := RichiestaS8{tracciaBase: TracciaS8(RagioniS8{Sigla: "PS", NomeFile: "ACM-1000.stp", CodiceProdotto: "ACM-1000"})}
	if got := motivoCambio(base); got != "gesto di PS da «Conferma la sorgente»: lo STEP «ACM-1000.stp» è la sorgente di ACM-1000" {
		t.Fatalf("il motivo del cambio della fonte senza ragioni: %q", got)
	}
}

func TestV1G39bisLaCandidaturaUnivocaDelMotore(t *testing.T) {
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
