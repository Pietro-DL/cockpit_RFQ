package propostabom

// L1 — un errore o un panic dell'adattatore restano nel gruppo (revisione della corsia motore, ondata B di V1: «contenere
// un errore o un panic dell'adattatore nel gruppo, senza far cadere la pagina»; EM §9.9). Il modello di /dati e il
// contenuto di S6 chiamano l'adattatore con importaProtetto: il gruppo ha il rifiuto con ERRORE_CALCOLO, gli altri campi
// restano, e la pagina non cade. Esiti sintetici, inventati.

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

func TestModelloIlPanicDellAdattatoreRestaNelGruppo(t *testing.T) {
	prima := importaGruppo
	defer func() { importaGruppo = prima }()
	a := uuid.MustParse("00000000-0000-4000-8000-0000000a0101")
	sha := strings.Repeat("ab", 32)
	pv := valutazione.ProdottoValutato{Rif: "componente:00000000-0000-4000-8000-0000000c0101",
		StruttureCandidate: []valutazione.StrutturaCandidata{{AllegatoID: a, Sha256: sha, Radice: ancoraggio.RifNodo(sha, "#1")}}}
	et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{pv}}
	modello := func(t *testing.T) GruppoDelModello {
		t.Helper()
		m := Modello(IngressoModello{Esito: &valutazione.Esito{}, Thread: et, Prodotto: pv.Rif})
		if m == nil || len(m.Gruppi) != 1 {
			t.Fatalf("il modello: %+v", m)
		}
		return m.Gruppi[0]
	}
	anteprima := func(t *testing.T) GruppoAnteprima {
		t.Helper()
		x := AnteprimaDelloSTEP(IngressoAnteprima{Esito: &valutazione.Esito{}, Thread: et, Prodotto: pv.Rif, Allegato: a})
		if len(x.Gruppi) != 1 {
			t.Fatalf("l'anteprima: %+v", x)
		}
		return x.Gruppi[0]
	}

	importaGruppo = func(Ingresso) (importazione.Registro, error) { panic("prova del panic") }
	if g := modello(t); g.Importazione == nil || g.Importazione.Codice != CodiceErroreCalcolo || !strings.Contains(g.Importazione.Testo, "panic") ||
		g.ImprontaContenuto != "" || len(g.Nodi) != 0 {
		t.Errorf("il panic nel modello: %+v", g)
	}
	if g := anteprima(t); g.Importazione == nil || g.Importazione.Codice != CodiceErroreCalcolo || g.ImprontaContenuto != "" {
		t.Errorf("il panic nell'anteprima: %+v", g)
	}

	// un errore senza codice (un difetto della forma del registro, cioè dell'adattatore): lo stesso ERRORE_CALCOLO
	importaGruppo = func(Ingresso) (importazione.Registro, error) {
		return importazione.Registro{}, &importazione.Errore{Campo: "proposta.nodi", Messaggio: "un difetto di prova"}
	}
	if g := modello(t); g.Importazione == nil || g.Importazione.Codice != CodiceErroreCalcolo || g.Importazione.Testo == "" {
		t.Errorf("l'errore senza codice: %+v", g)
	}

	// un rifiuto con il codice resta il suo
	importaGruppo = func(Ingresso) (importazione.Registro, error) {
		return importazione.Registro{}, &importazione.Errore{Codice: importazione.CodicePropostaNonDisponibile, Motivo: importazione.MotivoRegistroIncompleto,
			Messaggio: "manca il contesto"}
	}
	if g := modello(t); g.Importazione == nil || g.Importazione.Codice != importazione.CodicePropostaNonDisponibile ||
		g.Importazione.Motivo != importazione.MotivoRegistroIncompleto {
		t.Errorf("il rifiuto con il codice: %+v", g)
	}
}
