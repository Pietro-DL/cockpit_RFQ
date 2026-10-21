package distintabozza

// L1 — contenuto-atto-1 (U2 §6.2; PM23 CA-01…CA-07): i casi (i)-(vi) di IMP §2.18 e (vii) una categoria confermata nuova.
// La base e' lo snapshot di prova (la conferma di base) con la mappa dei nodi da creare; l'atto e' lo stesso snapshot
// cambiato come dice il caso.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// baseEAtto: il contenuto della conferma di base (lo snapshot di prova, con i nodi da creare mappati su componenti) e
// quello dell'atto, costruito dallo snapshot dato con la stessa mappa e le stesse parti del database.
func baseEAtto(t *testing.T, id idProva3, base, atto map[string]any, completa func(*ContenutoAtto)) (ContenutoAtto, ContenutoAtto) {
	t.Helper()
	mappa := map[string]uuid.UUID{"k-0007": idDi("c-0007"), "k-0009": idDi("c-0009"), "k-0011": idDi("c-0011"), "k-man": idDi("c-man")}
	b := ContenutoDaSnapshot(leggiProva3(t, base), mappa)
	a := ContenutoDaSnapshot(leggiProva3(t, atto), mappa)
	for _, c := range []*ContenutoAtto{&b, &a} {
		c.Criterio = "step"
	}
	if completa != nil {
		completa(&a)
	}
	return b, a
}

func puntiDi(dd []DifferenzaAtto) []int {
	var out []int
	for _, d := range dd {
		out = append(out, d.Punto)
	}
	return out
}

// CA-01, caso (i): «Rifai l'analisi» con gli stessi fatti, poi riallineamento: registro nuovo, stesso contenuto → uguale.
func TestCA01RegistroNuovoStessoContenuto(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	b, a := baseEAtto(t, id, base, copiaProfonda(t, base), nil)
	// l'impronta del registro non entra: la si cambia solo nell'atto costruito
	a.FonteSha256 = b.FonteSha256
	if uguale, dd := ContenutoAtto1(b, true, a); !uguale {
		t.Fatalf("stesso contenuto con un registro nuovo: %+v", dd)
	}
	// gli id delle decisioni, azione_id e la versione del servizio del segnale non entrano
	atto := copiaProfonda(t, base)
	decisioneIn3(atto, "ni-0001")["azione_id"] = "00000000-0000-4000-8000-0000000e0999"
	b, a = baseEAtto(t, id, base, atto, nil)
	if uguale, dd := ContenutoAtto1(b, true, a); !uguale {
		t.Fatalf("azione_id diversa: %+v", dd)
	}
}

// CA-02, caso (ii): «Autorizza» dopo la conferma n (criterio nessuno), poi rebase: il criterio e la radice autorizzata
// cambiano → non uguale (punti 7 e 1).
func TestCA02AutorizzaDopoLaConferma(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	b, a := baseEAtto(t, id, base, copiaProfonda(t, base), func(a *ContenutoAtto) {
		a.Criterio = "step"
		for i := range a.Componenti {
			if a.Componenti[i].Radice {
				a.Componenti[i].RadiceStepSha = id.sha
			}
		}
	})
	b.Criterio = "nessuno"
	uguale, dd := ContenutoAtto1(b, true, a)
	if uguale || !contieneInt(puntiDi(dd), PuntoFonte) || !contieneInt(puntiDi(dd), PuntoPerimetro) {
		t.Fatalf("autorizza dopo la conferma: %v %+v", uguale, dd)
	}
}

// CA-03, caso (iii): l'evidenza che conferma un'identita' con lo stesso codice, con una decisione che la cita → fondamento
// nuovo (punto 4).
func TestCA03EvidenzaRecepita(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	atto := copiaProfonda(t, base)
	nodoIn3(atto, "k-man")["evidenze_identita"] = []any{map[string]any{"allegato_id": id.pdf.String(), "sha256": id.shaPdf, "pagine": []any{2}}}
	b, a := baseEAtto(t, id, base, atto, nil)
	uguale, dd := ContenutoAtto1(b, true, a)
	if uguale || len(dd) != 1 || dd[0].Punto != PuntoFondamento {
		t.Fatalf("evidenza recepita: %v %+v", uguale, dd)
	}
	// il solo motivo di una decisione cambiato e' un contenuto nuovo (U2-P54)
	atto = copiaProfonda(t, base)
	decisioneIn3(atto, "di-0001")["motivo"] = "il disegno del fornitore conferma la lettura, rivisto"
	b, a = baseEAtto(t, id, base, atto, nil)
	if uguale, dd := ContenutoAtto1(b, true, a); uguale || dd[0].Punto != PuntoFondamento {
		t.Fatalf("motivo cambiato: %v %+v", uguale, dd)
	}
	// la stessa evidenza su un'altra occorrenza con lo stesso contenuto: niente di nuovo
	baseEv := copiaProfonda(t, base)
	nodoIn3(baseEv, "k-man")["evidenze_identita"] = []any{map[string]any{"allegato_id": id.pdf.String(), "sha256": id.shaPdf}}
	attoEv := copiaProfonda(t, base)
	nodoIn3(attoEv, "k-man")["evidenze_identita"] = []any{map[string]any{"allegato_id": idDi("altra-copia").String(), "sha256": id.shaPdf}}
	b, a = baseEAtto(t, id, baseEv, attoEv, nil)
	if uguale, dd := ContenutoAtto1(b, true, a); !uguale {
		t.Fatalf("stessa evidenza su un'altra occorrenza: %+v", dd)
	}
}

// CA-04, caso (iv): un gesto e il suo annullamento (lo stesso snapshot); un altro ingegnere che riconferma → uguale.
func TestCA04GestoEAnnullamento(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	b, a := baseEAtto(t, id, base, copiaProfonda(t, base), nil)
	if uguale, dd := ContenutoAtto1(b, true, a); !uguale || len(dd) != 0 {
		t.Fatalf("gesto e annullamento: %+v", dd)
	}
	// con il gesto e senza annullamento: la quantita' di un arco
	atto := copiaProfonda(t, base)
	for _, x := range atto["archi"].([]any) {
		if m := x.(map[string]any); m["figlio"] == "k-man" {
			m["qta"] = 5
		}
	}
	b, a = baseEAtto(t, id, base, atto, nil)
	if uguale, dd := ContenutoAtto1(b, true, a); uguale || dd[0].Punto != PuntoArchi {
		t.Fatalf("qta cambiata: %v %+v", uguale, dd)
	}
}

// CA-05, caso (v): la partenza registrato senza gesti su un prodotto con una conferma formato 3: nodi registrato con le
// identita' certificate, archi con i valori di trasporto → uguale.
func TestCA05PartenzaRegistratoSenzaGesti(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	mappa := map[string]uuid.UUID{"k-0007": idDi("c-0007"), "k-0009": idDi("c-0009"), "k-0011": idDi("c-0011"), "k-man": idDi("c-man")}
	b := ContenutoDaSnapshot(leggiProva3(t, base), mappa)
	b.Criterio = "step"
	// l'atto: lo stesso perimetro, gli archi con l'origine «conferma» (trasporto), nessuna decisione d'identita' nuova
	a := b
	a.Archi = nil
	for _, x := range b.Archi {
		x.Origine = ""
		a.Archi = append(a.Archi, x)
	}
	a.Fondamento = nil
	a.ProposteAccettate = map[string]string{}
	if uguale, dd := ContenutoAtto1(b, true, a); !uguale {
		t.Fatalf("partenza registrato senza gesti: %+v", dd)
	}
	// la prima conferma formato 3 su una base formato 1 o 2: mai uguale
	if uguale, dd := ContenutoAtto1(b, false, a); uguale || dd != nil {
		t.Fatalf("base non formato 3: %v %+v", uguale, dd)
	}
}

// CA-06, caso (vi): righe riaperte da una pagina vecchia dopo la conferma: l'atto le richiuderebbe (punto 8).
func TestCA06EffettiSulleRighe(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	b, a := baseEAtto(t, id, base, copiaProfonda(t, base), func(a *ContenutoAtto) {
		a.Effetti = []string{"chiude riga " + id.allegato.String() + "/#31", "chiude riga " + id.allegato.String() + "/#31"}
	})
	uguale, dd := ContenutoAtto1(b, true, a)
	if uguale || len(dd) != 1 || dd[0].Punto != PuntoEffetti || !strings.Contains(dd[0].Chiave, "#31") {
		t.Fatalf("effetti sulle righe: %v %+v", uguale, dd)
	}
}

// CA-07, caso (vii): una categoria confermata nuova (punto 9); l'ingresso e da_conferma non contano.
func TestCA07CategoriaNuova(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	cat := func(ingresso string) map[string]any {
		return map[string]any{"id": "cc-1", "tipo": DecisioneConfermaCategoria, "soggetto": map[string]any{"chiave_bozza": "k-man"},
			"categoria": "minuteria", "esito": "conferma", "origine_categoria": "normativa", "ingresso": ingresso, "motivo": "vite ISO"}
	}
	atto := copiaProfonda(t, base)
	atto["decisioni"] = append(atto["decisioni"].([]any), cat("editor"))
	b, a := baseEAtto(t, id, base, atto, nil)
	if uguale, dd := ContenutoAtto1(b, true, a); uguale || len(dd) != 1 || dd[0].Punto != PuntoCategorie {
		t.Fatalf("categoria nuova: %v %+v", uguale, dd)
	}
	// la stessa categoria gia' nella base, con un altro ingresso e riportata con da_conferma: uguale
	baseCat := copiaProfonda(t, base)
	baseCat["decisioni"] = append(baseCat["decisioni"].([]any), cat("editor"))
	attoCat := copiaProfonda(t, base)
	riportata := cat("wizard")
	riportata["da_conferma"] = map[string]any{"conferma_id": idDi("conferma-n").String(), "numero": 1, "decisione_id": "cc-1"}
	attoCat["decisioni"] = append(attoCat["decisioni"].([]any), riportata)
	b, a = baseEAtto(t, id, baseCat, attoCat, nil)
	if uguale, dd := ContenutoAtto1(b, true, a); !uguale {
		t.Fatalf("categoria riportata: %+v", dd)
	}
}

func contieneInt(xx []int, x int) bool {
	for _, y := range xx {
		if y == x {
			return true
		}
	}
	return false
}
