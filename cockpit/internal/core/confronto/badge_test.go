// L1 — il badge e l'indicatore di revisione di confronto, riga per riga della tavola del 6.4.6 (A1c-L1-18; R31 a-d A;
// v3 §6: la sola revisione non è mai una regressione; E-19): i file decisi danno regressione_su_confermata o uguale, i
// non decisi, nell'ordine, fuori_richiesta, nuovo_ancoraggio, uguale o diverso con il motivo. Le righe «come C5» hanno il
// badge calcolato contro la decisione confermata, senza «da rivedere». La lettura dei codici registrati con la
// grammatica è di valutazione (A1c-L1-30): qui il vecchio arriva già letto.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn): il repository
// è pubblico.

package confronto_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
)

// id: gli UUID inventati delle prove.
func id(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", n)) }

func pid(n int) *uuid.UUID { u := id(n); return &u }

func rifComp(n int) string { return "componente:" + id(n).String() }

// decisoSu: il vecchio di un file deciso sul componente n, con il codice letto dalla grammatica.
func decisoSu(n int, codice, base string) confronto.Vecchio {
	return confronto.Vecchio{Stato: "confermata", Fonte: "nome_file", Codice: codice, Rev: "01", Base: base,
		Leggibile: base != "", Componente: pid(n), Documento: pid(900 + n)}
}

// candidato: un candidato di livello componente sul componente n, con la base data e l'autorità proposta.
func candidato(n int, base string) confronto.Candidato {
	return confronto.Candidato{Target: rifComp(n), Livello: "componente", Base: base, Autorita: "proposta"}
}

// valutato: il nuovo di un file valutato, con i candidati dati.
func valutato(coll, assoc string, basi []string, c ...confronto.Candidato) confronto.Nuovo {
	return confronto.Nuovo{Valutato: true, Basi: basi, Candidati: c, Collocazione: coll, Associazione: assoc,
		Disponibilita: "disponibile"}
}

// unoSolo: Confronta su un solo file, senza atteso; la riga del file.
func unoSolo(t *testing.T, f confronto.File) confronto.EsitoFile {
	t.Helper()
	e := confronto.Confronta([]confronto.File{f}, nil, nil)
	if len(e.File) != 1 {
		t.Fatalf("righe: %d", len(e.File))
	}
	return e.File[0]
}

func TestBadgeRigaPerRiga(t *testing.T) {
	uguali := []string{"7120100"}
	casi := []struct {
		nome   string
		v      confronto.Vecchio
		n      confronto.Nuovo
		badge  confronto.Badge
		motivo string
	}{
		// Riga 2: file deciso, il componente del documento fra i candidati con la stessa base.
		{"deciso, componente proposto con la stessa base", decisoSu(1, "7120100", "7120100"),
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeUguale, ""},
		{"deciso, ambiguo ma con il componente fra i candidati", decisoSu(1, "7120100", "7120100"),
			valutato("figlio", "ambiguo", uguali, candidato(2, "7120100"), candidato(1, "7120100")), confronto.BadgeUguale, ""},
		{"deciso come duplicato", func() confronto.Vecchio { v := decisoSu(1, "7120100", "7120100"); v.Stato = "duplicato"; return v }(),
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeUguale, ""},
		{"deciso con il marcatore nel codice: conta la base (R31 c)", decisoSu(1, "7120100A", "7120100"),
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeUguale, ""},
		// Riga 1: file deciso, regressione con il motivo.
		{"deciso, il motore non risponde", decisoSu(1, "7120100", "7120100"),
			confronto.Nuovo{Valutato: false, Motivo: "thread_non_valutato", Associazione: "non_valutata"},
			confronto.BadgeRegressioneSuConfermata, confronto.MotivoNonValutato},
		{"deciso, documento senza componente", func() confronto.Vecchio { v := decisoSu(1, "7120100", "7120100"); v.Componente = nil; return v }(),
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoDocumentoSenzaComponente},
		{"deciso, detto fuori richiesta anche con il candidato", decisoSu(1, "7120100", "7120100"),
			valutato("fuori_richiesta", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoFuoriRichiesta},
		{"deciso, nessun candidato", decisoSu(1, "7120100", "7120100"),
			valutato("non_determinabile", "nessun_candidato", uguali), confronto.BadgeRegressioneSuConfermata, confronto.MotivoNessunCandidato},
		{"deciso, un altro target con la stessa base (T-B6-51)", decisoSu(1, "7120100", "7120100"),
			valutato("figlio", "candidato_unico", uguali, candidato(2, "7120100")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoStessaBaseAltroTarget},
		{"deciso, il componente non è fra i candidati, nessuno con la sua base", decisoSu(1, "7120100", "7120100"),
			valutato("figlio", "candidato_unico", []string{"7120199"}, candidato(2, "7120199")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoComponenteNonProposto},
		{"deciso, il componente con un'altra base", decisoSu(1, "7120100", "7120100"),
			valutato("figlio", "candidato_unico", []string{"7120199"}, candidato(1, "7120199")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoBaseDiversa},
		{"deciso, codice registrato non leggibile", decisoSu(1, "ACME-??", ""),
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoCodiceRegistratoNonLeggibile},
		// R-45 (MU1): una base vuota non è mai uguale a una base vuota.
		{"deciso, codice non leggibile e il componente proposto senza base", decisoSu(1, "ACME-??", ""),
			valutato("figlio", "candidato_unico", nil, candidato(1, "")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoCodiceRegistratoNonLeggibile},
		// R-43: deciso vuol dire che un documento confermato porta il file, come in valutazione; lo stato della proposta
		// non conta.
		{"documento con la proposta aperta: deciso", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, Componente: pid(1), Documento: pid(901)},
			valutato("figlio", "candidato_unico", []string{"7120199"}, candidato(2, "7120199")), confronto.BadgeRegressioneSuConfermata, confronto.MotivoComponenteNonProposto},
		{"documento senza proposta: deciso", confronto.Vecchio{Codice: "7120100", Base: "7120100", Leggibile: true, Componente: pid(1), Documento: pid(901)},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeUguale, ""},
		// Una proposta confermata senza documento non è un file deciso: vale la parte dei non decisi.
		{"confermata senza documento", confronto.Vecchio{Stato: "confermata", Codice: "7120100", Base: "7120100", Leggibile: true},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeNuovoAncoraggio, ""},
		// Riga 3.
		{"non deciso, fuori richiesta", confronto.Vecchio{Stato: "aperta", Codice: "7120500", Base: "7120500", Leggibile: true, ComponenteProposta: pid(5)},
			valutato("fuori_richiesta", "nessun_candidato", []string{"7120500"}), confronto.BadgeFuoriRichiesta, ""},
		// Riga 4.
		{"non deciso, vecchio senza destinazione, nuovo con un candidato", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeNuovoAncoraggio, ""},
		{"nessuna proposta, nuovo con un candidato", confronto.Vecchio{},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeNuovoAncoraggio, ""},
		// Riga 6, prima della 4: la proposta scartata a cui il motore dà un candidato.
		{"proposta scartata con un candidato del motore", confronto.Vecchio{Stato: "scartata", Codice: "7120100", Base: "7120100", Leggibile: true},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeDiverso, confronto.MotivoPropostaScartata},
		// R-45 (MU7): la scartata non ha destinazioni, nemmeno l'«assegna» rimasto sulla riga.
		{"proposta scartata con un «assegna» proposto dal motore", confronto.Vecchio{Stato: "scartata", Codice: "7120100", Base: "7120100", Leggibile: true, ComponenteProposta: pid(1)},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeDiverso, confronto.MotivoPropostaScartata},
		// Riga 5.
		{"stessa base e stesso target (assegna)", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, ComponenteProposta: pid(1)},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeUguale, ""},
		// R-43 e R-45 (MU14): il componente del vecchio è una destinazione per la riga 4 e per la riga 5 (un ingresso con
		// il componente e senza il documento, fuori dal contratto di valutazione, non diventa un nuovo ancoraggio).
		{"stessa base e stesso target (il componente)", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, Componente: pid(1)},
			valutato("figlio", "candidato_unico", uguali, candidato(1, "7120100")), confronto.BadgeUguale, ""},
		{"il componente del vecchio non è proposto", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, Componente: pid(1)},
			valutato("figlio", "candidato_unico", uguali, candidato(2, "7120100")), confronto.BadgeDiverso, confronto.MotivoTargetDiverso},
		{"stessa base e stesso target (candidato F8)", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, Destinazione: []string{rifComp(1)}},
			valutato("figlio", "ambiguo", uguali, candidato(2, "7120100"), candidato(1, "7120100")), confronto.BadgeUguale, ""},
		{"tutti e due senza ancoraggio, stessa base", confronto.Vecchio{Stato: "aperta", Codice: "7120300A", Base: "7120300", Leggibile: true},
			valutato("non_determinabile", "nessun_candidato", []string{"7120300"}), confronto.BadgeUguale, ""},
		{"tutti e due senza codice né base", confronto.Vecchio{},
			valutato("non_determinabile", "nessun_candidato", nil), confronto.BadgeUguale, ""},
		{"proposta scartata, il motore non ancora: stessa base", confronto.Vecchio{Stato: "scartata", Codice: "7120300", Base: "7120300", Leggibile: true},
			valutato("non_determinabile", "nessun_candidato", []string{"7120300"}), confronto.BadgeUguale, ""},
		// Riga 6: diverso, con il motivo.
		{"nessun codice, file non valutato", confronto.Vecchio{},
			confronto.Nuovo{Valutato: false, Motivo: "contenitore", Associazione: "non_valutata"}, confronto.BadgeDiverso, confronto.MotivoNonValutato},
		{"file non valutato con la stessa base: mai uguale", confronto.Vecchio{Stato: "aperta", Codice: "7120300", Base: "7120300", Leggibile: true},
			confronto.Nuovo{Valutato: false, Motivo: "thread_non_valutato", Basi: []string{"7120300"}, Associazione: "non_valutata"},
			confronto.BadgeDiverso, confronto.MotivoNonValutato},
		{"codice registrato non leggibile", confronto.Vecchio{Stato: "aperta", Codice: "ACME-??", Leggibile: false, MotivoLettura: "nessuna_lettura"},
			valutato("non_determinabile", "nessun_candidato", []string{"7120300"}), confronto.BadgeDiverso, confronto.MotivoCodiceRegistratoNonLeggibile},
		{"il vecchio ancora, il nuovo no", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, ComponenteProposta: pid(1)},
			valutato("non_determinabile", "nessun_candidato", uguali), confronto.BadgeDiverso, confronto.MotivoAncoraggioAssente},
		{"stesso target, base diversa", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, ComponenteProposta: pid(1)},
			valutato("figlio", "candidato_unico", []string{"7120199"}, candidato(1, "7120199")), confronto.BadgeDiverso, confronto.MotivoBaseDiversa},
		{"target diverso", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, Destinazione: []string{rifComp(1)}},
			valutato("figlio", "candidato_unico", uguali, candidato(2, "7120100")), confronto.BadgeDiverso, confronto.MotivoTargetDiverso},
		{"target in un'altra forma: nessuna inferenza", confronto.Vecchio{Stato: "aperta", Codice: "7120100", Base: "7120100", Leggibile: true, Destinazione: []string{"identificativo:P7120100"}},
			valutato("radice", "candidato_unico", uguali, confronto.Candidato{Target: rifComp(1), Livello: "prodotto", Base: "7120100", Autorita: "confermata"}),
			confronto.BadgeDiverso, confronto.MotivoTargetDiverso},
		{"tutti e due senza ancoraggio, basi diverse", confronto.Vecchio{Stato: "aperta", Codice: "7120300", Base: "7120300", Leggibile: true},
			valutato("non_determinabile", "nessun_candidato", []string{"7120399"}), confronto.BadgeDiverso, confronto.MotivoBaseDiversa},
		{"senza ancoraggio, due basi lette: non è la stessa base", confronto.Vecchio{Stato: "aperta", Codice: "7120300", Base: "7120300", Leggibile: true},
			valutato("non_determinabile", "nessun_candidato", []string{"7120300", "7120399"}), confronto.BadgeDiverso, confronto.MotivoBaseDiversa},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			r := unoSolo(t, confronto.File{AllegatoID: id(1), Vecchio: c.v, Nuovo: c.n})
			if r.Badge != c.badge || r.Motivo != c.motivo {
				t.Errorf("badge %s (%q), attesi %s (%q)", r.Badge, r.Motivo, c.badge, c.motivo)
			}
			if r.Badge == confronto.BadgeUguale || r.Badge == confronto.BadgeNuovoAncoraggio || r.Badge == confronto.BadgeFuoriRichiesta {
				if r.Motivo != "" {
					t.Errorf("il badge %s non ha motivo: %q", r.Badge, r.Motivo)
				}
			}
		})
	}
}

// TestBadgeAmbiguoRestaVisibile: per un file deciso con il componente fra i candidati il badge è uguale, e
// l'associazione ambigua resta nel nuovo della riga (6.4.6, riga 2).
func TestBadgeAmbiguoRestaVisibile(t *testing.T) {
	r := unoSolo(t, confronto.File{AllegatoID: id(1), Vecchio: decisoSu(1, "7120100", "7120100"),
		Nuovo: valutato("figlio", "ambiguo", []string{"7120100"}, candidato(2, "7120100"), candidato(1, "7120100"))})
	if r.Badge != confronto.BadgeUguale || r.File.Nuovo.Associazione != "ambiguo" || len(r.File.Nuovo.Candidati) != 2 {
		t.Errorf("riga %+v", r)
	}
}

// TestBadgeRigheComeC5 (R31 a A, P-14): le righe «come C5» hanno il badge della tavola contro la decisione confermata;
// nessun badge «da rivedere» esiste, e una regressione su una riga così è un dato, con il suo motivo.
func TestBadgeRigheComeC5(t *testing.T) {
	// Confermata con il codice con la A; il motore propone lo stesso componente con la stessa base.
	uguale := unoSolo(t, confronto.File{AllegatoID: id(1), Vecchio: decisoSu(1, "7120100A", "7120100"),
		Nuovo: valutato("figlio", "candidato_unico", []string{"7120100"}, candidato(1, "7120100"))})
	// Confermata su un componente, mentre il nome del file dà un'altra base: il motore propone un altro componente.
	regressione := unoSolo(t, confronto.File{AllegatoID: id(2), Vecchio: decisoSu(3, "7120300", "7120300"),
		Nuovo: valutato("figlio", "candidato_unico", []string{"7120200"}, candidato(2, "7120200"))})
	if uguale.Badge != confronto.BadgeUguale || regressione.Badge != confronto.BadgeRegressioneSuConfermata ||
		regressione.Motivo != confronto.MotivoComponenteNonProposto {
		t.Errorf("righe come C5: %s, %s (%s)", uguale.Badge, regressione.Badge, regressione.Motivo)
	}
	e := confronto.Confronta(nil, nil, nil)
	if len(e.Conteggi) != 5 {
		t.Fatalf("conteggi %v: i badge sono cinque", e.Conteggi)
	}
	for b := range e.Conteggi {
		if b == "da_rivedere" {
			t.Errorf("un badge «da rivedere»: compare solo nel rapporto del banco")
		}
	}
}

// TestIndicatoreDiRevisione (R31 b A; E-19): l'indicatore traduce il confronto delle revisioni di valutazione, con le
// due revisioni confrontate (quella letta nel codice vecchio e quella letta nel file) e il motivo; la sola revisione
// diversa lascia il badge uguale; non_confrontabili dà non_determinabile, con la diagnostica solo se c'era qualcosa da
// confrontare (file valutato, vecchio con un codice o una rev, almeno un lato con una revisione o le revisioni del file
// discordi: R-46); un valore fuori vocabolario sempre con la diagnostica; nessun confronto fatto, non_determinabile senza.
func TestIndicatoreDiRevisione(t *testing.T) {
	base := func(rev, revisioni, motivo string) confronto.File {
		v := decisoSu(1, "7120100_02", "7120100")
		v.Rev, v.Revisione = "00.00", "02"
		n := valutato("figlio", "candidato_unico", []string{"7120100"}, candidato(1, "7120100"))
		n.Revisione, n.Revisioni, n.MotivoRevisioni = rev, revisioni, motivo
		return confronto.File{AllegatoID: id(7), Vecchio: v, Nuovo: n}
	}
	senzaVecchio := base("02", "non_confrontabili", "revisione_vecchia_non_letta")
	senzaVecchio.Vecchio = confronto.Vecchio{}
	nonValutato := base("", "non_confrontabili", "nuovo_non_valutato")
	nonValutato.Nuovo.Valutato = false
	senzaRevisioni := base("", "non_confrontabili", "revisione_vecchia_non_letta")
	senzaRevisioni.Vecchio.Rev, senzaRevisioni.Vecchio.Revisione = "", ""
	discordi := base("", "non_confrontabili", "revisioni_nuove_discordi")
	discordi.Vecchio.Rev, discordi.Vecchio.Revisione = "", ""
	casi := []struct {
		nome        string
		f           confronto.File
		valore      string
		vecchia     string
		badge       confronto.Badge
		diagnostica bool
	}{
		{"uguali", base("02", "uguali", ""), confronto.RevisioneUguale, "02", confronto.BadgeUguale, false},
		{"diverse: solo revisione, mai regressione", base("03", "diverse", ""), confronto.RevisioneDiversa, "02", confronto.BadgeUguale, false},
		{"non confrontabili", base("00", "non_confrontabili", "token_non_attribuito"), confronto.RevisioneNonDeterminabile, "02", confronto.BadgeUguale, true},
		{"non confrontabili senza nessun vecchio", senzaVecchio, confronto.RevisioneNonDeterminabile, "", confronto.BadgeNuovoAncoraggio, false},
		{"non confrontabili per il file non valutato", nonValutato, confronto.RevisioneNonDeterminabile, "02", confronto.BadgeRegressioneSuConfermata, false},
		{"non confrontabili senza revisioni da nessuna parte (R-46)", senzaRevisioni, confronto.RevisioneNonDeterminabile, "", confronto.BadgeUguale, false},
		{"revisioni del file discordi (R-46)", discordi, confronto.RevisioneNonDeterminabile, "", confronto.BadgeUguale, true},
		{"nessun confronto", base("", "", ""), confronto.RevisioneNonDeterminabile, "02", confronto.BadgeUguale, false},
		{"fuori vocabolario", base("02", "forse", ""), confronto.RevisioneNonDeterminabile, "02", confronto.BadgeUguale, true},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := confronto.Confronta([]confronto.File{c.f}, nil, nil)
			r := e.File[0]
			if r.Badge != c.badge {
				t.Errorf("badge %s (%s), atteso %s: la revisione non lo cambia", r.Badge, r.Motivo, c.badge)
			}
			if r.Revisione.Valore != c.valore || r.Revisione.Vecchia != c.vecchia || r.Revisione.Nuova != c.f.Nuovo.Revisione ||
				r.Revisione.Motivo != c.f.Nuovo.MotivoRevisioni {
				t.Errorf("indicatore %+v", r.Revisione)
			}
			if got := len(e.Diagnostiche) == 1; got != c.diagnostica {
				t.Fatalf("diagnostiche %+v", e.Diagnostiche)
			}
			if c.diagnostica {
				d := e.Diagnostiche[0]
				if d.Codice != confronto.CodiceRevisioneNonConfrontabile || d.Gravita != "nota" || d.Natura != "dati" ||
					len(d.Rif) != 1 || d.Rif[0] != id(7).String() || d.Messaggio == "" {
					t.Errorf("diagnostica %+v", d)
				}
			}
		})
	}
}
