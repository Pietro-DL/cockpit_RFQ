package filerfq

// L1 — V1, direttiva 35, blocco C-6a (corsia file): le regole della cartella ALTRO che non vogliono il database. Lo
// svincolo gia' salvato nella bozza chiede «scollega» invece di bloccare (U2-P47; D-ASS-03 / V1-Q42 [U] A), i testi
// della copia scartata (U2 §3.7: «eliminato» solo con rimosso), il nome di una copia (niente «..»), l'azione «Riprova la
// rimozione» in S1 (U2 §3.5). Nomi sintetici.

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

func TestSvincoloNellaBozzaChiedeScollega(t *testing.T) {
	a := uuid.New()
	d := dipendenzaDaBozza(a, uuid.New(), "svincolo", "ACME-1")
	if d.Codice != "D-6" || d.Esito != EsitoRisoluzione || d.Risoluzione != RisoluzioneScollega {
		t.Fatalf("lo svincolo nella bozza: %+v", d)
	}
	el := ElencoDipendenze{Voci: []Dipendenza{d}}
	el.Impronta = "x"
	if rf := controllaDipendenze(el, "x", false, false, true, nil); rf == nil || rf.Codice != "RISOLUZIONE_RICHIESTA" {
		t.Errorf("senza scollega: %+v", rf)
	}
	if rf := controllaDipendenze(el, "x", true, false, true, nil); rf != nil {
		t.Errorf("con scollega lo svincolo nella bozza blocca ancora: %+v", rf)
	}
	// le altre citazioni della bozza bloccano come prima
	if d := dipendenzaDaBozza(a, uuid.New(), "preassociazione", "ACME-1"); d.Esito != EsitoBlocca {
		t.Errorf("la preassociazione non blocca: %+v", d)
	}
}

func TestTestiDellaCopiaScartata(t *testing.T) {
	casi := map[[2]string]string{
		{"rilasciata", nas.MotivoRimosso}:               "File eliminato anche dalla cartella ALTRO della RFQ.",
		{"rilasciata", nas.MotivoGiaAssente}:            "La copia non è stata trovata al percorso registrato: la cartella può essere stata spostata o rinominata.",
		{"rilasciata", nas.MotivoNonNostro}:             "Il file in ALTRO non l'ha creato il cockpit: resta dov'è.",
		{"rilasciata", nas.MotivoPercorsoDiUnDocumento}: "Il file in ALTRO è un documento confermato: resta dov'è.",
		{"rilasciata", nas.MotivoFileCambiato}:          "Il file in ALTRO è stato cambiato da una persona: il cockpit non lo tocca.",
		{"da_rilasciare", nas.MotivoFileOccupato}:       "La copia in ALTRO sul NAS è aperta da un altro programma: si toglie appena si chiude.",
		{"da_rilasciare", ""}:                           "La copia in ALTRO si toglie appena possibile.",
		{"in_rimozione", ""}:                            "La copia in ALTRO si toglie appena possibile.",
	}
	for k, atteso := range casi {
		got := testoCopiaScartata(StatoConservazione{Stato: k[0], Motivo: k[1]})
		if got != atteso {
			t.Errorf("%s/%s: %q, atteso %q", k[0], k[1], got, atteso)
		}
		if strings.Contains(got, "eliminato") != (k[1] == nas.MotivoRimosso) {
			t.Errorf("%s/%s: «eliminato» fuori da rimosso", k[0], k[1])
		}
	}
}

func TestNomeNellaCartellaAltro(t *testing.T) {
	for ricevuto, atteso := range map[string]string{
		"Offerta ACME.pdf":       "Offerta ACME.pdf",
		"rev..vecchia.pdf":       "rev.vecchia.pdf",
		`cartella\dentro\x.pdf`:  "",
		"ACME....definitivo.PDF": "ACME.definitivo.pdf",
	} {
		got := nomeNellaCartellaAltro(ricevuto)
		if strings.Contains(got, "..") || strings.ContainsAny(got, `\/:`) {
			t.Errorf("%q → %q: il nome non va bene per ALTRO\\<nome>", ricevuto, got)
		}
		if atteso != "" && got != atteso {
			t.Errorf("%q → %q, atteso %q", ricevuto, got, atteso)
		}
	}
}

func TestAzioneRiprovaRimozione(t *testing.T) {
	cons := &nas.Conservatore{Radice: t.TempDir()}
	s := &Servizio{Conservatore: cons}
	scartato := func(stato, causa string) FileSorgente {
		return FileSorgente{Stato: StatoScartato, Conservazione: &ConservazioneFile{Stato: stato, Causa: causa}}
	}
	if a := s.azioni(scartato("da_rilasciare", "scartato"), db.NaturaAllegatoFile, false); !a.RiprovaRimozione.Disponibile {
		t.Errorf("acceso: %+v", a.RiprovaRimozione)
	}
	for _, f := range []FileSorgente{scartato("rilasciata", "scartato"), scartato("trattenuta", "riattivato"), {Stato: StatoAttivo},
		{Stato: StatoScartato}} {
		if a := s.azioni(f, db.NaturaAllegatoFile, false); a.RiprovaRimozione.Disponibile || a.RiprovaRimozione.Motivo == "" {
			t.Errorf("%+v: %+v", f.Conservazione, a.RiprovaRimozione)
		}
	}
	cons.DryRun = true
	if a := s.azioni(scartato("da_rilasciare", "scartato"), db.NaturaAllegatoFile, false); a.RiprovaRimozione.Disponibile ||
		!strings.Contains(a.RiprovaRimozione.Motivo, "scrittura") {
		t.Errorf("con la scrittura spenta: %+v", a.RiprovaRimozione)
	}
}
