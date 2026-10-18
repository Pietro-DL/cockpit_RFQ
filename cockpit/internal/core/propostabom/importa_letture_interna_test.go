package propostabom

// L1 — le due proiezioni importazione-1 dell'identità proposta (emendamento di V1 §6.1, «Il codice di partenza», e §3.8,
// rev_proposta; V1-Q09 A) e i due campi di una lettura del registro (DECA PE-08): prove A-24 e A-25 di PM23 §10, la parte
// pura. Le letture sono scritte a mano, con gli intervalli in byte sul testo dell'unità; i codici sono inventati.

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

func iv(a, b int) evidenze.Intervallo { return evidenze.Intervallo{Inizio: a, Fine: b} }

// formaDi: una forma completa con la base [ib, fb) nel testo e il resto dato da chi chiama.
func formaDi(testo string, ib, fb int) motorea.LetturaForma {
	return motorea.LetturaForma{Originale: testo, Intervallo: iv(0, len(testo)),
		Base: motorea.BaseLetta{Originale: testo[ib:fb], Normalizzata: testo[ib:fb], Completa: true,
			Segmenti: []motorea.SegmentoLetto{{Nome: "codice", Originale: testo[ib:fb], Normalizzato: testo[ib:fb], Intervallo: iv(ib, fb)}}}}
}

// TestA24LaRevProposta (A-24): «…_rev_b» letto come b dà B; una revisione con spazi o di 11 caratteri dà la rev vuota
// con revisione_non_ammissibile; un token sospeso, revisione_non_letta; senza revisione, niente e nessun motivo.
func TestA24LaRevProposta(t *testing.T) {
	f := formaDi("7120100A_rev_b", 0, 7)
	rev := func(stato, norm string) *motorea.RevisioneLetta {
		return &motorea.RevisioneLetta{Originale: "_rev_" + norm, Normalizzata: norm, Stato: stato, Intervallo: iv(8, 14)}
	}
	for _, c := range []struct {
		nome        string
		r           *motorea.RevisioneLetta
		rev, motivo string
	}{
		{"la b minuscola", rev(motorea.StatoRevisioneLetta, "b"), "B", ""},
		{"già maiuscola, con una cifra", rev(motorea.StatoRevisioneLetta, "c1"), "C1", ""},
		{"con uno spazio", rev(motorea.StatoRevisioneLetta, "b 1"), "", MotivoRevNonAmmissibile},
		{"di 11 caratteri", rev(motorea.StatoRevisioneLetta, "abcdefghijk"), "", MotivoRevNonAmmissibile},
		{"un token sospeso", rev(motorea.StatoRevisioneNonInterpretabile, ""), "", MotivoRevNonLetta},
		{"senza revisione", nil, "", ""},
	} {
		f.Revisione = c.r
		if r, m := ProiezioneRev(f); r != c.rev || m != c.motivo {
			t.Errorf("%s: rev %q motivo %q, attesi %q %q", c.nome, r, m, c.rev, c.motivo)
		}
	}
	// la revisione resta fuori dal codice di partenza
	f.Revisione = rev(motorea.StatoRevisioneLetta, "b")
	f.Marcatore = &motorea.ParteLetta{Valore: "A", Originale: "A", Intervallo: iv(7, 8)}
	if c, m := ProiezioneCodice(f, "7120100A_rev_b"); c != "7120100A" || m != "" {
		t.Errorf("il codice con la revisione: %q %q", c, m)
	}
}

// TestProiezioneCodiceITratti (emendamento di V1 §6.1, V1-Q09 A): il tratto dalla base al marcatore con gli affissi,
// byte per byte (niente maiuscole, niente NFC), e i tre motivi del codice vuoto.
func TestProiezioneCodiceITratti(t *testing.T) {
	// un affisso davanti, il marcatore dopo
	f := formaDi("P7120100A", 1, 8)
	f.Affissi = []motorea.AffissoLetto{{Regola: "fase", Originale: "P", Posizione: "prima", Intervallo: iv(0, 1)}}
	f.Marcatore = &motorea.ParteLetta{Valore: "A", Originale: "A", Intervallo: iv(8, 9)}
	if c, m := ProiezioneCodice(f, "P7120100A"); c != "P7120100A" || m != "" {
		t.Errorf("affisso e marcatore: %q %q", c, m)
	}
	// le minuscole e un letterale NFD restano come sono
	nfd := "élement-7"
	g := formaDi(nfd, 0, len(nfd))
	if c, m := ProiezioneCodice(g, nfd); c != nfd || m != "" {
		t.Errorf("il letterale NFD: %q %q", c, m)
	}
	// una base parziale
	h := formaDi("7120100", 0, 7)
	h.Base.Completa = false
	if c, m := ProiezioneCodice(h, "7120100"); c != "" || m != MotivoPropostoBaseNonCompleta {
		t.Errorf("la base parziale: %q %q", c, m)
	}
	// un token fra la base e il marcatore
	k := formaDi("7120100XXA", 0, 7)
	k.Token = &motorea.ParteLetta{Valore: "XX", Originale: "XX", Intervallo: iv(7, 9)}
	k.Marcatore = &motorea.ParteLetta{Valore: "A", Originale: "A", Intervallo: iv(9, 10)}
	if c, m := ProiezioneCodice(k, "7120100XXA"); c != "" || m != MotivoPropostoTrattoNonDeterminabile {
		t.Errorf("il token nel tratto: %q %q", c, m)
	}
	// una ripetizione discordante
	d := formaDi("7120100", 0, 7)
	d.Stato = motorea.StatoDiscordante
	if c, m := ProiezioneCodice(d, "7120100"); c != "" || m != MotivoPropostoTrattoNonDeterminabile {
		t.Errorf("la lettura discordante: %q %q", c, m)
	}
	// un tratto con uno spazio
	s := formaDi("7120 100", 0, 8)
	if c, m := ProiezioneCodice(s, "7120 100"); c != "" || m != MotivoPropostoCodiceNonAmmissibile {
		t.Errorf("il tratto con lo spazio: %q %q", c, m)
	}
	// la base due volte nel letterale, senza segmenti che la collochino
	r := formaDi("7120100-7120100", 0, 7)
	r.Base.Segmenti = nil
	if c, m := ProiezioneCodice(r, "7120100-7120100"); c != "" || m != MotivoPropostoTrattoNonDeterminabile {
		t.Errorf("la base ripetuta: %q %q", c, m)
	}
}

// TestA25ICampiDellaLettura (A-25; DECA PE-08): la lettura del registro porta codice_proposto e rev_proposta quando la
// proiezione è ammissibile, con i valori delle proiezioni dell'identità proposta; senza proiezione ammissibile i due
// campi sono vuoti, e la forma canonica del registro li toglie. Una lettura senza un codice di partenza (la base
// parziale) ma con una revisione letta non porta nemmeno la rev: «il codice, e la sua rev se c'è».
func TestA25ICampiDellaLettura(t *testing.T) {
	a := uuid.MustParse("00000000-0000-4000-8000-0000000e8a10")
	testo := "7120100A_rev_b"
	d := evidenze.DocumentoEvidenze{Unita: []evidenze.UnitaEvidenza{{ID: "u:1", Testo: testo}}}
	x := nuovoLettore(a, d, uuid.MustParse("00000000-0000-4000-8000-00000000ac01"), "h")
	f := formaDi(testo, 0, 7)
	f.Marcatore = &motorea.ParteLetta{Valore: "A", Originale: "A", Intervallo: iv(7, 8)}
	f.Revisione = &motorea.RevisioneLetta{Originale: "_rev_b", Normalizzata: "b", Stato: motorea.StatoRevisioneLetta, Intervallo: iv(8, 14)}
	l := x.lettura(motorea.LetturaCodice{ID: "l:1", UnitaID: "u:1", Forma: f})
	if l["codice_proposto"] != "7120100A" || l["rev_proposta"] != "B" || l["chiave_lettura"] != a.String()+"/l:1" {
		t.Errorf("la lettura con le proiezioni: codice %v, rev %v", l["codice_proposto"], l["rev_proposta"])
	}
	// la base parziale con una revisione letta e ammissibile: niente codice, quindi niente rev (la voce invece la porta)
	f.Base.Completa = false
	l = x.lettura(motorea.LetturaCodice{ID: "l:3", UnitaID: "u:1", Forma: f})
	if l["codice_proposto"] != "" || l["rev_proposta"] != "" {
		t.Errorf("la lettura con la rev e senza il codice di partenza: codice %v, rev %v", l["codice_proposto"], l["rev_proposta"])
	}
	if r, m := ProiezioneRev(f); r != "B" || m != "" {
		t.Errorf("la rev della voce resta la sua proiezione: %q %q", r, m)
	}
	f.Revisione.Normalizzata = "b 1"
	l = x.lettura(motorea.LetturaCodice{ID: "l:2", UnitaID: "u:1", Forma: f})
	if l["codice_proposto"] != "" || l["rev_proposta"] != "" {
		t.Errorf("la lettura senza proiezioni ammissibili: codice %v, rev %v", l["codice_proposto"], l["rev_proposta"])
	}
}
