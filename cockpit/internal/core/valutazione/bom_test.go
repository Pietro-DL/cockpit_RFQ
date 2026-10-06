// L1 — la verifica della BOM, nomenclatura e gerarchia (R80; contratto §1.0 righe 3 e 4, §1.3, §2.3, §2.6; T-B0-07,
// T-B0-22, T-B0-24; R61 A, R95 A; PO-03 e PO-15 nelle parti di B5): la regola su gesti e strutture sintetici (il
// prodotto senza figli del modello nuovo), l'adattatore di «Conferma l'albero» sulle scene ACME (il segno sugli archi
// del prodotto, la riga della radice esclusa, le righe decise da un automatismo, le relazioni che il segno non copre,
// la fonte non registrata), la lettura A di K-02 nella sua funzione, il target senza componente, i conflitti di
// gerarchia (la quantità decisa contro la BOM di lavoro, sommata per coppia di componenti; la rimozione aperta; il loro
// ordine). Le correzioni dopo la revisione della fase 1: lo STEP arrivato due volte e il nodo mai proposto dal legacy
// (T-B5-15), la somma parziale (T-B5-16), la gerarchia legacy senza la BOM di lavoro (T-B5-17), il conflitto che
// prevale anche per il target senza componente (T-B5-09), la gerarchia non vuota del modello nuovo.
package valutazione_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// La struttura sintetica di un prodotto senza figli: la sola radice; con la fonte confermata è la BOM di lavoro.
var radiceSintetica = ancoraggio.RifNodo(sha("9"), "#1")

func senzaFigliSintetica(fonte bool) valutazione.StrutturaDaVerificare {
	return valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: fonte, BOMDiLavoro: fonte, Radici: []string{radiceSintetica},
		Nodi: []string{radiceSintetica}}
}

func gestoSintetico(nodi, archi []string) *valutazione.GestoVerifica {
	da := operatore
	return &valutazione.GestoVerifica{Da: &da, Il: "2026-10-03T10:00:00Z", Nodi: nodi, Archi: archi}
}

// TestPO03IlProdottoSenzaFigli (PO-03, parte senza K-02; R80; T-B0-22): sulla regola, con la fonte confermata, la
// nomenclatura della radice e la gerarchia vuota confermate da gesti sintetici del modello nuovo danno la BOM
// verificata; la gerarchia vuota non si conferma prima della nomenclatura della radice. Con l'adattatore legacy, sulla
// scena con lo STEP di un pezzo solo, niente da leggere: non_verificabile, con il motivo.
func TestPO03IlProdottoSenzaFigli(t *testing.T) {
	t.Run("sulla regola: radice e gerarchia vuota confermate", func(t *testing.T) {
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil),
			Gerarchia: gestoSintetico(nil, nil)}, senzaFigliSintetica(true))
		if !v.Verificata || asse(v.Nomenclatura) != "verificata/" || asse(v.Gerarchia) != "verificata/" || !v.SenzaFigli || !v.FonteRegistrata ||
			v.Nomenclatura.Legacy || v.Nomenclatura.DaDecidere != 0 || v.Gerarchia.DaDecidere != 0 {
			t.Errorf("BOM %+v", v)
		}
	})
	t.Run("sulla regola: la gerarchia vuota aspetta la nomenclatura della radice", func(t *testing.T) {
		for nome, g := range map[string]valutazione.GestiVerificaBOM{
			"senza il gesto della nomenclatura": {Gerarchia: gestoSintetico(nil, nil)},
			"il gesto non copre la radice":      {Nomenclatura: gestoSintetico([]string{ancoraggio.RifNodo(sha("9"), "#7")}, nil), Gerarchia: gestoSintetico(nil, nil)},
		} {
			v := valutazione.VerificaDellaBOM(g, senzaFigliSintetica(true))
			if v.Verificata || asse(v.Gerarchia) != "da_verificare/nomenclatura_radice_non_verificata" || v.Nomenclatura.Stato != valutazione.StatoAsseDaVerificare {
				t.Errorf("%s: nomenclatura %s, gerarchia %s", nome, asse(v.Nomenclatura), asse(v.Gerarchia))
			}
		}
	})
	t.Run("con l'adattatore legacy e i figli solo proposti: da_verificare (R80, R71 A)", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 1}})
		b := prodotto(t, valuta(t, th, motoreCatena(t), nil), rifProdB).BOM
		if b.SenzaFigli || asse(b.Nomenclatura) != "da_verificare/nessun_gesto" || asse(b.Gerarchia) != "da_verificare/nessun_gesto" ||
			b.Nomenclatura.DaDecidere != 1 || b.Gerarchia.DaDecidere != 2 {
			t.Errorf("BOM %+v", b)
		}
	})
	t.Run("con l'adattatore legacy: non_verificabile", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}}, nil)
		v := valuta(t, th, motoreCatena(t), nil)
		p := prodotto(t, v, rifProdB)
		if p.Struttura != ancoraggio.StatoBOMDiLavoroProposta || p.Fonte.Stato != valutazione.FonteConfermata {
			t.Fatalf("struttura %s, fonte %s", p.Struttura, statoMotivo(p.Fonte))
		}
		b := p.BOM
		if b.Verificata || !b.SenzaFigli || b.FonteRegistrata || asse(b.Nomenclatura) != "non_verificabile/senza_figli_nessun_gesto" ||
			asse(b.Gerarchia) != "non_verificabile/senza_figli_nessun_gesto" || !b.Nomenclatura.Legacy || !b.Gerarchia.Legacy {
			t.Errorf("BOM legacy %+v", b)
		}
		if len(conCodice(v.Diagnostiche, valutazione.CodiceBOMFonteNonRegistrata)) != 0 {
			t.Error("nessun gesto legacy: niente bom.fonte_non_registrata")
		}
	})
}

// TestK02LetturaA (K-02, lettura A, confermata dall'utente il 06/10 [U]; E1R §3.2; T-E1R-04; PO-03, la variante senza
// fonte): senza la fonte confermata la gerarchia non è mai verificata (da_verificare,
// fonte_non_confermata), la nomenclatura non cambia, non_verificabile prevale; fonte_non_confermata vale solo quando è
// l'unica cosa che manca. «Conferma l'albero» con la fonte non confermata: FonteRegistrata falso (R61 A).
func TestK02LetturaA(t *testing.T) {
	t.Run("sulla regola, modello nuovo: la gerarchia vuota confermata senza fonte", func(t *testing.T) {
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil),
			Gerarchia: gestoSintetico(nil, nil)}, senzaFigliSintetica(false))
		if v.Verificata || asse(v.Nomenclatura) != "verificata/" || asse(v.Gerarchia) != "da_verificare/fonte_non_confermata" || v.FonteRegistrata {
			t.Errorf("BOM %+v", v)
		}
	})
	t.Run("sulla regola, legacy senza figli e senza fonte: non_verificabile prevale", func(t *testing.T) {
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Legacy: true}, senzaFigliSintetica(false))
		if asse(v.Gerarchia) != "non_verificabile/senza_figli_nessun_gesto" || asse(v.Nomenclatura) != "non_verificabile/senza_figli_nessun_gesto" {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(v.Nomenclatura), asse(v.Gerarchia))
		}
	})
	t.Run("sulla regola: un altro motivo resta il motivo", func(t *testing.T) {
		s := senzaFigliSintetica(false)
		s.Nodi = append(s.Nodi, ancoraggio.RifNodo(sha("9"), "#2"))
		s.Archi = []string{valutazione.RifArco(radiceSintetica, ancoraggio.RifNodo(sha("9"), "#2"))}
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico(s.Nodi, nil), Gerarchia: gestoSintetico(nil, nil)}, s)
		if asse(v.Gerarchia) != "da_verificare/da_decidere" || v.Gerarchia.DaDecidere != 1 || asse(v.Nomenclatura) != "verificata/" {
			t.Errorf("nomenclatura %s, gerarchia %s (%d da decidere)", asse(v.Nomenclatura), asse(v.Gerarchia), v.Gerarchia.DaDecidere)
		}
	})
	t.Run("«Conferma l'albero» con la fonte non confermata", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
		senzaFonte(&th)
		confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
		v := valuta(t, th, motoreCatena(t), nil)
		p := prodotto(t, v, rifProdB)
		b := p.BOM
		if p.Struttura != ancoraggio.StatoStrutturaCandidata || b.Verificata || b.FonteRegistrata || asse(b.Nomenclatura) != "verificata/" ||
			asse(b.Gerarchia) != "da_verificare/fonte_non_confermata" || !b.Gerarchia.Legacy || b.Gerarchia.DaDecidere != 0 {
			t.Errorf("struttura %s, BOM %+v", p.Struttura, b)
		}
		if len(conCodice(v.Diagnostiche, valutazione.CodiceBOMFonteNonRegistrata)) != 1 {
			t.Errorf("diagnostiche %+v: il gesto legacy non registra la fonte (R61 A)", v.Diagnostiche)
		}
	})
}

// scenaAlbero: la scena della BOM con la radice 7120100A e il figlio 7120200A (qta 2), la fonte confermata e «Conferma
// l'albero» sul figlio; la riga della radice resta aperta.
func scenaAlbero(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
	confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
	return th
}

// TestPO15LAdattatoreDiConfermaLAlbero (PO-15, parte B5; R80, R61 A, R71 A come adattatore; T-B4-24): il segno sugli
// archi del prodotto e nessuna riga da decidere della sua struttura, esclusa la riga della radice, che resta aperta e
// non blocca: nomenclatura e gerarchia verificate, la BOM verificata, la fonte non registrata con la sua diagnostica.
// Poi le varianti che la fermano: una riga aperta di un altro nodo, una riga decisa da un automatismo, una relazione
// confermata che il segno non copre, un arco aperto; e una che non la ferma: un nodo tolto dalla conferma (righe
// scartate da una persona, con il segno).
func TestPO15LAdattatoreDiConfermaLAlbero(t *testing.T) {
	m := motoreCatena(t)
	t.Run("la riga della radice aperta non blocca", func(t *testing.T) {
		th := scenaAlbero(t)
		v := valuta(t, th, m, nil)
		p := prodotto(t, v, rifProdB)
		bom := strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
		if bom.RigaRadice != ancoraggio.RifRigaProposta(rigaRadB) || len(bom.RigheDaDecidere) != 0 {
			t.Fatalf("riga della radice %q, righe da decidere %v", bom.RigaRadice, bom.RigheDaDecidere)
		}
		b := p.BOM
		if !b.Verificata || asse(b.Nomenclatura) != "verificata/" || asse(b.Gerarchia) != "verificata/" || !b.Nomenclatura.Legacy ||
			b.FonteRegistrata || b.SenzaFigli || b.RimozioniAperte != 0 || b.Nomenclatura.DaDecidere != 0 || b.Gerarchia.DaDecidere != 0 {
			t.Errorf("BOM %+v", b)
		}
		d := conCodice(v.Diagnostiche, valutazione.CodiceBOMFonteNonRegistrata)
		if len(d) != 1 || d[0].Gravita != evidenze.GravitaAvviso || !reflect.DeepEqual(d[0].Rif, []string{rifProdB}) || d[0].Percorso != "prodotti["+rifProdB+"].bom" {
			t.Errorf("diagnostiche %+v", d)
		}
		if len(v.Conflitti) != 0 {
			t.Errorf("conflitti %+v", v.Conflitti)
		}
	})

	for _, c := range []struct {
		nome         string
		modifica     func(th *fotorfq.Thread)
		nom, ger     string
		daDecidereNG [2]int
	}{
		{"una riga aperta di un altro nodo", func(th *fotorfq.Thread) {
			aggiungiNodo(t, th, "#3", "7120300A")
		}, "da_verificare/da_decidere", "da_verificare/da_decidere", [2]int{1, 2}},
		{"una riga decisa da un automatismo, senza chi l'ha decisa (T-B4-24)", func(th *fotorfq.Thread) {
			rigaDi(t, th, "#2").DecisoDa = nil
		}, "da_verificare/da_decidere", "da_verificare/da_decidere", [2]int{1, 1}},
		{"una relazione confermata che il segno non copre (le decisioni una a una non sono l'adattatore)", func(th *fotorfq.Thread) {
			confermaComponente(th, cTerzo, "7120300A", cProdotto, 1)
		}, "da_verificare/da_decidere", "da_verificare/da_decidere", [2]int{1, 1}},
		{"un arco aperto fra due nodi della struttura", func(th *fotorfq.Thread) {
			aggiungiNodo(t, th, "#3", "7120300A")
			op := operatore
			r := rigaDi(t, th, "#3")
			r.Stato, r.DecisoDa = "scartata", &op
		}, "verificata/", "da_verificare/da_decidere", [2]int{0, 1}},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := scenaAlbero(t)
			c.modifica(&th)
			b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
			if asse(b.Nomenclatura) != c.nom || asse(b.Gerarchia) != c.ger || b.Verificata ||
				b.Nomenclatura.DaDecidere != c.daDecidereNG[0] || b.Gerarchia.DaDecidere != c.daDecidereNG[1] {
				t.Errorf("nomenclatura %s (%d), gerarchia %s (%d); attese %s (%d), %s (%d)", asse(b.Nomenclatura), b.Nomenclatura.DaDecidere,
					asse(b.Gerarchia), b.Gerarchia.DaDecidere, c.nom, c.daDecidereNG[0], c.ger, c.daDecidereNG[1])
			}
		})
	}

	t.Run("il segno conta solo su una riga tenuta da una persona (T-B4-24)", func(t *testing.T) {
		th := scenaAlbero(t)
		aggiungiNodo(t, &th, "#3", "7120300A")
		confermaLAlbero(t, &th, "#3", cTerzo, "7120300A", 1)
		arcoDi(t, &th, "#1", "#3").DecisoDa = nil
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if asse(b.Nomenclatura) != "da_verificare/da_decidere" || b.Nomenclatura.DaDecidere != 1 ||
			asse(b.Gerarchia) != "da_verificare/da_decidere" || b.Gerarchia.DaDecidere != 2 {
			t.Errorf("nomenclatura %s (%d), gerarchia %s (%d): la relazione verso il terzo non è coperta, e la sua riga d'arco è da decidere",
				asse(b.Nomenclatura), b.Nomenclatura.DaDecidere, asse(b.Gerarchia), b.Gerarchia.DaDecidere)
		}
	})
	t.Run("un segno su una riga d'arco scartata non è un arco del gesto", func(t *testing.T) {
		th := scenaAlbero(t)
		arcoDi(t, &th, "#1", "#2").Stato = "scartata"
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if asse(b.Nomenclatura) != "da_verificare/nessun_gesto" {
			t.Errorf("nomenclatura %s", asse(b.Nomenclatura))
		}
	})
	t.Run("dopo la conferma della fonte contano solo le righe della BOM di lavoro (R71 A, R76 A)", func(t *testing.T) {
		th := scenaAlbero(t)
		altro := uid(0x717)
		f := fattiStepF(t, sha("6"), []nodoF{{"#1", "7120100A", ""}, {"#5", "7120500A", ""}}, []arcoF{{"#1", "#5", 1}})
		conAllegato(&th, allegato(altro, 7, "vecchio-7120100A.stp", "stp", sha("6")), &f)
		th.RigheComponenteProposta = append(th.RigheComponenteProposta,
			fotorfq.RigaComponenteProposta{ID: uid(0x73a), AllegatoID: altro, Sha256: sha("6"), Chiave: "#1", IDGrezzo: "7120100A", Fonte: "step", Stato: "aperta"},
			fotorfq.RigaComponenteProposta{ID: uid(0x73b), AllegatoID: altro, Sha256: sha("6"), Chiave: "#5", IDGrezzo: "7120500A", Fonte: "step", Stato: "aperta"})
		th.RigheRelazioneProposta = append(th.RigheRelazioneProposta,
			fotorfq.RigaRelazioneProposta{AllegatoID: altro, PadreChiave: "#1", FiglioChiave: "#5", Qta: 1, Stato: "aperta"})
		v := valuta(t, th, m, nil)
		if b := prodotto(t, v, rifProdB).BOM; !b.Verificata {
			t.Errorf("con la BOM di lavoro: nomenclatura %s, gerarchia %s", asse(b.Nomenclatura), asse(b.Gerarchia))
		}
		strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoStrutturaCandidata) // l'altro STEP resta una struttura candidata
		senzaFonte(&th)
		if b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM; asse(b.Nomenclatura) != "da_verificare/da_decidere" {
			t.Errorf("senza la fonte contano tutte le strutture candidate: nomenclatura %s", asse(b.Nomenclatura))
		}
	})

	t.Run("un nodo tolto dalla conferma, scartato da una persona con il segno, è deciso", func(t *testing.T) {
		th := scenaAlbero(t)
		aggiungiNodo(t, &th, "#3", "7120300A")
		op := operatore
		r := rigaDi(t, &th, "#3")
		r.Stato, r.DecisoDa, r.Albero = "scartata", &op, segnoAlbero(nil, nil)
		a := arcoDi(t, &th, "#1", "#3")
		a.Stato, a.DecisoDa, a.Albero = "scartata", &op, segnoAlbero(nil, nil)
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if !b.Verificata {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(b.Nomenclatura), asse(b.Gerarchia))
		}
	})
}

// aggiungiNodo: un figlio in più della radice nei fatti dello STEP della scena (qta 1), con la sua riga aperta e la
// riga aperta dell'arco.
func aggiungiNodo(t *testing.T, th *fotorfq.Thread, chiave, codice string) {
	t.Helper()
	var nodi []nodoF
	var archi []arcoF
	for _, r := range th.RigheComponenteProposta {
		nodi = append(nodi, nodoF{r.Chiave, r.IDGrezzo, ""})
	}
	for _, r := range th.RigheRelazioneProposta {
		archi = append(archi, arcoF{r.PadreChiave, r.FiglioChiave, r.Qta})
	}
	nodi = append(nodi, nodoF{chiave, codice, ""})
	archi = append(archi, arcoF{"#1", chiave, 1})
	th.Fatti[shaStepB] = fattiStepF(t, shaStepB, nodi, archi)
	th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x739), AllegatoID: aStepB,
		NomeFile: "7120100A_1.stp", Sha256: shaStepB, Chiave: chiave, IDGrezzo: codice, Famiglia: "acme-catena", Fonte: "step", Stato: "aperta"})
	th.RigheRelazioneProposta = append(th.RigheRelazioneProposta, fotorfq.RigaRelazioneProposta{AllegatoID: aStepB, NomeFile: "7120100A_1.stp",
		PadreChiave: "#1", FiglioChiave: chiave, Qta: 1, Stato: "aperta"})
}

// TestLAdattatoreNeiCasiDiConfine (R80, R61 A, R62 e A; T-B4-24; i casi che le mutazioni non vedevano): la riga della
// radice decisa da un automatismo resta esclusa; il segno vale anche su una riga d'arco duplicato; una riga d'arco aperta
// resta da decidere anche se porta chi l'ha toccata; le righe d'arco di una copia dello STEP non sono della BOM di
// lavoro; un segno con il padre fuori dal perimetro non è il gesto del prodotto; un componente archiviato non è più
// nella BOM né una decisione del nodo; i figli solo confermati non fanno un prodotto senza figli.
func TestLAdattatoreNeiCasiDiConfine(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	verificata := func(t *testing.T, th fotorfq.Thread) valutazione.ValutazioneProdotti {
		t.Helper()
		v := valuta(t, th, m, nil)
		p := prodotto(t, v, rifProdB)
		if p.Struttura != ancoraggio.StatoBOMDiLavoroProposta {
			t.Fatalf("struttura %s: la scena chiede la BOM di lavoro", p.Struttura)
		}
		if b := p.BOM; !b.Verificata || b.Nomenclatura.DaDecidere != 0 || b.Gerarchia.DaDecidere != 0 {
			t.Errorf("nomenclatura %s (%d), gerarchia %s (%d): attesa la BOM verificata", asse(b.Nomenclatura), b.Nomenclatura.DaDecidere,
				asse(b.Gerarchia), b.Gerarchia.DaDecidere)
		}
		return v
	}

	t.Run("la riga della radice decisa da un automatismo resta esclusa (R80, T-B4-24)", func(t *testing.T) {
		th := scenaAlbero(t)
		decidi(t, &th, "#1", "duplicato", cProdotto, nil)
		verificata(t, th)
	})
	t.Run("il segno su una riga d'arco duplicato vale", func(t *testing.T) {
		th := scenaAlbero(t)
		arcoDi(t, &th, "#1", "#2").Stato = "duplicato"
		verificata(t, th)
	})
	t.Run("una riga d'arco aperta resta da decidere anche con chi l'ha toccata", func(t *testing.T) {
		th := scenaAlbero(t)
		aggiungiNodo(t, &th, "#3", "7120300A")
		r := rigaDi(t, &th, "#3")
		r.Stato, r.DecisoDa = "scartata", &op
		arcoDi(t, &th, "#1", "#3").DecisoDa = &op
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if asse(b.Nomenclatura) != "verificata/" || asse(b.Gerarchia) != "da_verificare/da_decidere" || b.Gerarchia.DaDecidere != 1 {
			t.Errorf("nomenclatura %s, gerarchia %s (%d)", asse(b.Nomenclatura), asse(b.Gerarchia), b.Gerarchia.DaDecidere)
		}
	})
	t.Run("le righe d'arco di una copia dello STEP non sono della BOM di lavoro", func(t *testing.T) {
		th := scenaAlbero(t)
		copia := uid(0x716)
		conAllegato(&th, allegato(copia, 6, "copia-7120100A_1.stp", "stp", shaStepB), nil)
		th.RigheRelazioneProposta = append(th.RigheRelazioneProposta,
			fotorfq.RigaRelazioneProposta{AllegatoID: copia, PadreChiave: "#1", FiglioChiave: "#2", Qta: 2, Stato: "aperta"})
		verificata(t, th)
	})
	t.Run("un segno con il padre fuori dal perimetro non è il gesto del prodotto", func(t *testing.T) {
		th := scenaAlbero(t)
		estraneo := uid(0x7ff)
		arcoDi(t, &th, "#1", "#2").Albero.Padre = &estraneo
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if asse(b.Nomenclatura) != "da_verificare/nessun_gesto" || asse(b.Gerarchia) != "da_verificare/nessun_gesto" {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(b.Nomenclatura), asse(b.Gerarchia))
		}
	})
	t.Run("un componente archiviato non è più nella BOM né una decisione del nodo (R62 e A)", func(t *testing.T) {
		th := scenaAlbero(t)
		aggiungiNodo(t, &th, "#3", "7120300A")
		confermaComponente(&th, cTerzo, "7120300A", cProdotto, 1)
		archiviato := dataACME
		th.Componenti[len(th.Componenti)-1].ArchiviatoIl = &archiviato
		decidi(t, &th, "#3", "confermata", cTerzo, &op)
		a := arcoDi(t, &th, "#1", "#3")
		a.Stato, a.DecisoDa = "scartata", &op
		v := verificata(t, th)
		if n := nodoIn(t, strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta), nodoB("#3")); n.Decisione != nil {
			t.Errorf("nodo #3: decisione %+v sul componente archiviato", n.Decisione)
		}
	})
	t.Run("i figli solo confermati: non è un prodotto senza figli (R80)", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}}, nil)
		confermaComponente(&th, cSciolto, "7120200A", cProdotto, 1)
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if b.SenzaFigli || asse(b.Nomenclatura) != "da_verificare/nessun_gesto" || asse(b.Gerarchia) != "da_verificare/nessun_gesto" ||
			b.Nomenclatura.DaDecidere != 1 || b.Gerarchia.DaDecidere != 1 {
			t.Errorf("BOM %+v", b)
		}
	})
}

// TestLaRegolaNeiCasiDiConfine (R80; T-B0-22): nel modello nuovo un gesto senza struttura da coprire non verifica
// niente; nel legacy la gerarchia non aspetta la nomenclatura, che un conflitto può fermare da sola.
func TestLaRegolaNeiCasiDiConfine(t *testing.T) {
	t.Run("modello nuovo senza struttura", func(t *testing.T) {
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico(nil, nil), Gerarchia: gestoSintetico(nil, nil)},
			valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true})
		if v.Verificata || asse(v.Nomenclatura) != "da_verificare/nessuna_struttura" || asse(v.Gerarchia) != "da_verificare/nessuna_struttura" {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(v.Nomenclatura), asse(v.Gerarchia))
		}
	})
	t.Run("modello nuovo: la gerarchia non vuota non aspetta la nomenclatura (R80)", func(t *testing.T) {
		figlio := ancoraggio.RifNodo(sha("9"), "#2")
		arco := valutazione.RifArco(radiceSintetica, figlio)
		s := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true, Radici: []string{radiceSintetica},
			Nodi: []string{radiceSintetica, figlio}, Archi: []string{arco}}
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil),
			Gerarchia: gestoSintetico(nil, []string{arco})}, s)
		if asse(v.Nomenclatura) != "da_verificare/da_decidere" || v.Nomenclatura.DaDecidere != 1 || asse(v.Gerarchia) != "verificata/" || v.Verificata {
			t.Errorf("nomenclatura %s (%d), gerarchia %s: solo la gerarchia vuota aspetta la nomenclatura della radice", asse(v.Nomenclatura),
				v.Nomenclatura.DaDecidere, asse(v.Gerarchia))
		}
	})
	t.Run("legacy: la gerarchia senza archi nello STEP non aspetta la nomenclatura", func(t *testing.T) {
		arco := valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())
		s := valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: true, BOMDiLavoro: true, Radici: []string{radiceSintetica},
			Nodi: []string{radiceSintetica}, ArchiConfermati: []string{arco},
			Conflitti: []valutazione.Conflitto{{Tipo: valutazione.ConflittoCodice, Asse: valutazione.AsseNomenclatura, Rif: "componente:" + cSciolto.String()}}}
		v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Legacy: true, Nomenclatura: gestoSintetico(nil, []string{arco}),
			Gerarchia: gestoSintetico(nil, []string{arco})}, s)
		if asse(v.Nomenclatura) != "conflitto/conflitto" || asse(v.Gerarchia) != "verificata/" || v.SenzaFigli {
			t.Errorf("nomenclatura %s, gerarchia %s, senza figli %v", asse(v.Nomenclatura), asse(v.Gerarchia), v.SenzaFigli)
		}
	})
}

// TestLaBOMDiLavoroSoloConLaFonteConfermata (R76 A, R65 A): il riferimento del gesto 3 c'è, ma la vista dice che la
// fonte è in attesa (una riga incoerente con il gesto): ancoraggio non riceve la fonte confermata, e lo STEP resta una
// struttura candidata.
func TestLaBOMDiLavoroSoloConLaFonteConfermata(t *testing.T) {
	th := scenaAlbero(t)
	th.StepProdotto[0].Esito = "da_scegliere"
	p := prodotto(t, valuta(t, th, motoreCatena(t), nil), rifProdB)
	if p.Fonte.Stato != valutazione.FonteInAttesaDiConferma || p.Fonte.Riferimento == nil || p.Struttura != ancoraggio.StatoStrutturaCandidata {
		t.Errorf("fonte %s con il riferimento %v, struttura %s", statoMotivo(p.Fonte), p.Fonte.Riferimento != nil, p.Struttura)
	}
}

// TestIlTargetSenzaComponente (T-B0-07): un identificativo confermato senza componente ha la struttura candidata, ma
// nomenclatura e gerarchia sono non_verificabile, con il motivo: non c'è una BOM da leggere.
func TestIlTargetSenzaComponente(t *testing.T) {
	th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
	th.Identificativi = []fotorfq.Identificativo{confermato("7120100A")}
	f := fattiStepF(t, sha("e"), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 1}})
	conAllegato(&th, allegato(uid(0x781), 1, "assieme-acme.stp", "stp", sha("e")), &f)
	p := prodotto(t, valuta(t, th, motoreCatena(t), nil), "identificativo:7120100A")
	if p.Struttura != ancoraggio.StatoStrutturaCandidata || asse(p.BOM.Nomenclatura) != "non_verificabile/target_senza_componente" ||
		asse(p.BOM.Gerarchia) != "non_verificabile/target_senza_componente" || p.BOM.Verificata {
		t.Errorf("struttura %s, BOM %+v", p.Struttura, p.BOM)
	}
}

// TestIConflittiDellaGerarchia (T-B0-24, R95 A; T-B4 R-08): sulla BOM di lavoro dello STEP confermato, la quantità di
// una relazione confermata che lo STEP contraddice, con la somma per coppia di componenti (due nodi decisi come lo
// stesso componente sotto lo stesso padre); una rimozione proposta aperta sul perimetro. Il conflitto blocca la
// gerarchia, con le evidenze dei due lati, e la decisione resta; sulle strutture candidate niente.
func TestIConflittiDellaGerarchia(t *testing.T) {
	m := motoreCatena(t)
	rifRelazione := valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())

	t.Run("la quantità decisa contro la BOM di lavoro", func(t *testing.T) {
		th := scenaAlbero(t)
		th.Relazioni[0].Qta = 3
		v := valuta(t, th, m, nil)
		b := prodotto(t, v, rifProdB).BOM
		if asse(b.Gerarchia) != "conflitto/conflitto" || !reflect.DeepEqual(b.Gerarchia.Conflitti, []string{rifRelazione}) ||
			asse(b.Nomenclatura) != "verificata/" || b.Verificata {
			t.Fatalf("BOM %+v", b)
		}
		if len(v.Conflitti) != 1 {
			t.Fatalf("conflitti %+v", v.Conflitti)
		}
		c := v.Conflitti[0]
		if c.Tipo != valutazione.ConflittoArco || c.Asse != valutazione.AsseGerarchia || c.Rif != rifRelazione || c.Prodotto != rifProdB ||
			c.Decisione != "qta 3" || c.Proposta != "qta 2" || c.Motivo != valutazione.MotivoConflittoQuantita || c.OrigineDecisione != ancoraggio.OrigineConfermato ||
			c.EvidenzaDecisione.Da == nil || *c.EvidenzaDecisione.Da != operatore || c.EvidenzaDecisione.Il == nil ||
			c.EvidenzaProposta.AllegatoID == nil || *c.EvidenzaProposta.AllegatoID != aStepB ||
			!reflect.DeepEqual(c.EvidenzaProposta.Riferimenti, []string{valutazione.RifArco(nodoB("#1"), nodoB("#2"))}) {
			t.Errorf("conflitto %+v", c)
		}
		arco := strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta).Archi[0]
		if arco.Decisione == nil || arco.Decisione.Quantita == nil || *arco.Decisione.Quantita != 3 {
			t.Errorf("arco %+v: la decisione resta il valore corrente (R95 A)", arco)
		}
	})
	t.Run("due nodi decisi come lo stesso componente: la somma per coppia", func(t *testing.T) {
		for _, c := range []struct {
			qta       int
			conflitto bool
		}{{2, false}, {1, true}} {
			th := scenaAlbero(t)
			aggiungiNodo(t, &th, "#3", "7120200A")
			th.Relazioni[0].Qta = c.qta
			th.Fatti[shaStepB] = fattiStepF(t, shaStepB, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120200A", ""}},
				[]arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}})
			op := operatore
			decidi(t, &th, "#3", "duplicato", cSciolto, &op)
			a := arcoDi(t, &th, "#1", "#3")
			a.Stato, a.DecisoDa = "duplicato", &op
			v := valuta(t, th, m, nil)
			ger := prodotto(t, v, rifProdB).BOM.Gerarchia
			if (ger.Stato == valutazione.StatoAsseConflitto) != c.conflitto {
				t.Errorf("relazione qta %d contro 1+1 nello STEP: gerarchia %s, conflitti %+v", c.qta, asse(ger), v.Conflitti)
			}
			if c.conflitto && (len(v.Conflitti) != 1 || v.Conflitti[0].Proposta != "qta 2" || len(v.Conflitti[0].EvidenzaProposta.Riferimenti) != 2) {
				t.Errorf("conflitti %+v", v.Conflitti)
			}
		}
	})
	t.Run("due nodi padre decisi come lo stesso componente: ogni esemplare da solo (T-B5-14)", func(t *testing.T) {
		rifSotto := valutazione.RifArco("componente:"+cSciolto.String(), "componente:"+cTerzo.String())
		for _, c := range []struct {
			qta       int
			conflitto bool
		}{{1, false}, {2, true}} {
			// #2 e #3 sono due esemplari dello stesso sottoassieme, ognuno con un figlio deciso come il terzo (qta 1).
			th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120200A", ""}, {"#4", "7120300A", ""}, {"#5", "7120300A", ""}},
				[]arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#2", "#4", 1}, {"#3", "#5", 1}})
			confermaComponente(&th, cSciolto, "7120200A", cProdotto, 2)
			confermaComponente(&th, cTerzo, "7120300A", cSciolto, c.qta)
			op := operatore
			decidi(t, &th, "#2", "confermata", cSciolto, &op)
			decidi(t, &th, "#3", "duplicato", cSciolto, &op)
			decidi(t, &th, "#4", "confermata", cTerzo, &op)
			decidi(t, &th, "#5", "duplicato", cTerzo, &op)
			v := valuta(t, th, m, nil)
			bom := strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
			decisi := 0
			for _, a := range bom.Archi {
				if a.Decisione != nil {
					decisi++
				}
			}
			if decisi != 4 {
				t.Fatalf("archi con la decisione accanto %d: attesi 4", decisi)
			}
			ger := prodotto(t, v, rifProdB).BOM.Gerarchia
			if (ger.Stato == valutazione.StatoAsseConflitto) != c.conflitto {
				t.Errorf("relazione qta %d contro 1 per esemplare: gerarchia %s, conflitti %+v", c.qta, asse(ger), v.Conflitti)
			}
			if !c.conflitto {
				continue
			}
			if len(v.Conflitti) != 1 || v.Conflitti[0].Rif != rifSotto || v.Conflitti[0].Decisione != "qta 2" || v.Conflitti[0].Proposta != "qta 1" ||
				!reflect.DeepEqual(v.Conflitti[0].EvidenzaProposta.Riferimenti, []string{valutazione.RifArco(nodoB("#2"), nodoB("#4")), valutazione.RifArco(nodoB("#3"), nodoB("#5"))}) {
				t.Errorf("conflitti %+v: uno solo, con gli archi dei due esemplari", v.Conflitti)
			}
		}
	})
	t.Run("una rimozione proposta aperta sul perimetro", func(t *testing.T) {
		th := scenaAlbero(t)
		th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cSciolto, QtaWorking: 2},
			{StepDocumentoID: dStepB, PadreID: uid(0x7ff), FiglioID: uid(0x7fe), QtaWorking: 1}}
		v := valuta(t, th, m, nil)
		b := prodotto(t, v, rifProdB).BOM
		if asse(b.Gerarchia) != "conflitto/conflitto" || b.RimozioniAperte != 1 || asse(b.Nomenclatura) != "verificata/" {
			t.Fatalf("BOM %+v", b)
		}
		if len(v.Conflitti) != 1 || v.Conflitti[0].Motivo != valutazione.MotivoConflittoRimozione || v.Conflitti[0].Rif != rifRelazione ||
			v.Conflitti[0].Decisione != "qta 2" || v.Conflitti[0].EvidenzaProposta.DocumentoID == nil || *v.Conflitti[0].EvidenzaProposta.DocumentoID != dStepB {
			t.Errorf("conflitti %+v", v.Conflitti)
		}
	})
	t.Run("i conflitti di gerarchia escono in ordine di Rif e motivo, non della fotografia", func(t *testing.T) {
		th := scenaAlbero(t)
		th.Relazioni[0].Qta = 3
		confermaComponente(&th, cTerzo, "7120300A", cProdotto, 1)
		th.RimozioniAperte = []fotorfq.RimozioneAperta{{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cTerzo, QtaWorking: 1},
			{StepDocumentoID: dStepB, PadreID: cProdotto, FiglioID: cSciolto, QtaWorking: 2}}
		rifTerzo := valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cTerzo.String())
		var got []string
		for _, c := range valuta(t, th, m, nil).Conflitti {
			got = append(got, c.Rif+" "+c.Motivo)
		}
		want := []string{rifRelazione + " quantita", rifRelazione + " rimozione", rifTerzo + " rimozione"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("conflitti %v, attesi %v", got, want)
		}
	})
	t.Run("sulla struttura candidata nessuna BOM di lavoro contraddice", func(t *testing.T) {
		th := scenaAlbero(t)
		senzaFonte(&th)
		op := operatore
		decidi(t, &th, "#1", "duplicato", cProdotto, &op)
		th.Relazioni[0].Qta = 3
		v := valuta(t, th, m, nil)
		s := strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoStrutturaCandidata)
		if len(s.Archi) != 1 || !ancoraggio.QuantitaDiscorde(s.Archi[0]) {
			t.Fatalf("archi %+v: la quantità discorde c'è anche sulla struttura candidata", s.Archi)
		}
		if len(v.Conflitti) != 0 || prodotto(t, v, rifProdB).BOM.Gerarchia.Stato == valutazione.StatoAsseConflitto {
			t.Errorf("conflitti %+v", v.Conflitti)
		}
	})
}

// TestLaRegolaSuiConflitti (R95 A; T-B0-24; T-B5-09): il conflitto blocca l'asse e prevale su ogni altro stato,
// anche su non_verificabile; un conflitto di un asse non blocca l'altro.
func TestLaRegolaSuiConflitti(t *testing.T) {
	s := senzaFigliSintetica(true)
	s.Conflitti = []valutazione.Conflitto{{Tipo: valutazione.ConflittoCodice, Asse: valutazione.AsseNomenclatura, Rif: "componente:" + uuid.Nil.String()}}
	for nome, g := range map[string]valutazione.GestiVerificaBOM{
		"legacy senza figli": {Legacy: true},
		"modello nuovo":      {Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil), Gerarchia: gestoSintetico(nil, nil)},
	} {
		v := valutazione.VerificaDellaBOM(g, s)
		if v.Nomenclatura.Stato != valutazione.StatoAsseConflitto || v.Verificata || len(v.Nomenclatura.Conflitti) != 1 || len(v.Gerarchia.Conflitti) != 0 ||
			v.Gerarchia.Stato == valutazione.StatoAsseConflitto {
			t.Errorf("%s: nomenclatura %+v, gerarchia %+v", nome, v.Nomenclatura, v.Gerarchia)
		}
	}
	// T-B5-09 [T], confermata dall'orchestratore (da ratificare dall'utente): anche per un target senza componente il
	// conflitto prevale su non_verificabile (T-B0-07 resta per l'asse senza conflitti).
	s.ConComponente = false
	v := valutazione.VerificaDellaBOM(valutazione.GestiVerificaBOM{Legacy: true}, s)
	if asse(v.Nomenclatura) != "conflitto/conflitto" || asse(v.Gerarchia) != "non_verificabile/target_senza_componente" || v.Verificata {
		t.Errorf("target senza componente con un conflitto: nomenclatura %s, gerarchia %s", asse(v.Nomenclatura), asse(v.Gerarchia))
	}
}

// ---- le correzioni dopo la revisione della fase 1 ----

// aCopia: lo stesso STEP della scena arrivato una seconda volta, con un ID maggiore del portatore.
var aCopia = uid(0x7f0)

// scenaConLaCopia: la scena dell'albero con la copia dello STEP, e il documento confermato che porta solo la copia: il
// riferimento di B1 dice la copia, le righe legacy (componente_proposta e relazione_proposta) stanno sul portatore,
// come le scrive il legacy (lo stesso contenuto propone una volta sola).
func scenaConLaCopia(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaAlbero(t)
	conAllegato(&th, allegato(aCopia, 9, "copia-7120100A_1.stp", "stp", shaStepB), nil)
	th.Documenti[0].Allegati = []uuid.UUID{aCopia}
	return th
}

// TestLoSTEPArrivatoDueVolte (T-B5-15; R80, T-B0-24, R95 A, R76 A): la BOM di lavoro sta
// sull'allegato che porta le righe legacy di quel contenuto, non sulla copia che il riferimento di B1 nomina; il
// riferimento di B1 resta com'è. Sulla copia la verifica perderebbe le righe da decidere e i conflitti: una riga aperta
// ferma la BOM, la quantità decisa contraddetta e il cartiglio di PO-23 danno i loro conflitti. Con le righe su due
// allegati vale quello del riferimento; senza un allegato del thread che le porta, resta quello del riferimento.
func TestLoSTEPArrivatoDueVolte(t *testing.T) {
	m := motoreCatena(t)
	bomDi := func(t *testing.T, v valutazione.ValutazioneProdotti) ancoraggio.StrutturaProdotto {
		t.Helper()
		return strutturaDelProdotto(t, v, rifProdB, ancoraggio.StatoBOMDiLavoroProposta)
	}

	t.Run("la BOM di lavoro sta sul portatore, il riferimento di B1 resta la copia", func(t *testing.T) {
		v := valuta(t, scenaConLaCopia(t), m, nil)
		p := prodotto(t, v, rifProdB)
		if r := p.Fonte.Riferimento; r == nil || r.AllegatoID == nil || *r.AllegatoID != aCopia {
			t.Errorf("riferimento %+v: il riferimento di B1 non cambia", p.Fonte.Riferimento)
		}
		bom := bomDi(t, v)
		if bom.AllegatoID != aStepB || nodoIn(t, bom, nodoB("#2")).RigaDecisa == nil || nodoIn(t, bom, nodoB("#2")).Decisione == nil {
			t.Fatalf("BOM di lavoro sull'allegato %s: attesa sul portatore %s, con la riga decisa del nodo #2", bom.AllegatoID, aStepB)
		}
		if !p.BOM.Verificata {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(p.BOM.Nomenclatura), asse(p.BOM.Gerarchia))
		}
	})
	t.Run("una riga aperta sul portatore ferma la BOM", func(t *testing.T) {
		th := scenaConLaCopia(t)
		aggiungiNodo(t, &th, "#3", "7120300A")
		b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
		if b.Verificata || asse(b.Nomenclatura) != "da_verificare/da_decidere" || b.Nomenclatura.DaDecidere != 1 ||
			asse(b.Gerarchia) != "da_verificare/da_decidere" || b.Gerarchia.DaDecidere != 2 {
			t.Errorf("nomenclatura %s (%d), gerarchia %s (%d)", asse(b.Nomenclatura), b.Nomenclatura.DaDecidere, asse(b.Gerarchia), b.Gerarchia.DaDecidere)
		}
	})
	t.Run("la quantità decisa che lo STEP contraddice dà il conflitto", func(t *testing.T) {
		th := scenaConLaCopia(t)
		th.Relazioni[0].Qta = 3
		v := valuta(t, th, m, nil)
		if b := prodotto(t, v, rifProdB).BOM; asse(b.Gerarchia) != "conflitto/conflitto" {
			t.Errorf("gerarchia %s", asse(b.Gerarchia))
		}
		if len(v.Conflitti) != 1 || v.Conflitti[0].Proposta != "qta 2" || v.Conflitti[0].EvidenzaProposta.AllegatoID == nil ||
			*v.Conflitti[0].EvidenzaProposta.AllegatoID != aStepB {
			t.Errorf("conflitti %+v", v.Conflitti)
		}
	})
	t.Run("il cartiglio contro la nomenclatura confermata dà il conflitto (PO-23)", func(t *testing.T) {
		th := scenaConLaCopia(t)
		conDisegno(t, &th, cSciolto, "7120200B1", nil)
		v := valuta(t, th, m, nil)
		if b := prodotto(t, v, rifProdB).BOM; asse(b.Nomenclatura) != "conflitto/conflitto" || len(v.Conflitti) != 1 {
			t.Errorf("nomenclatura %s, conflitti %+v", asse(b.Nomenclatura), v.Conflitti)
		}
	})
	t.Run("con le righe su due allegati vale quello del riferimento", func(t *testing.T) {
		th := scenaAlbero(t)
		bassa := uid(0x710) // prima del portatore in ordine di ID, con tutte le righe dei nodi aperte
		conAllegato(&th, allegato(bassa, 9, "copia-7120100A_1.stp", "stp", shaStepB), nil)
		for i, r := range append([]fotorfq.RigaComponenteProposta(nil), th.RigheComponenteProposta...) {
			th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x760 + i), AllegatoID: bassa,
				Sha256: r.Sha256, Chiave: r.Chiave, IDGrezzo: r.IDGrezzo, Fonte: "step", Stato: "aperta"})
		}
		v := valuta(t, th, m, nil)
		if bom := bomDi(t, v); bom.AllegatoID != aStepB {
			t.Errorf("BOM di lavoro sull'allegato %s: atteso quello del riferimento, %s", bom.AllegatoID, aStepB)
		}
		if b := prodotto(t, v, rifProdB).BOM; !b.Verificata {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(b.Nomenclatura), asse(b.Gerarchia))
		}
	})
	t.Run("una riga di quel contenuto su un allegato di un altro contenuto non fa il portatore (dubbio T-B5-07)", func(t *testing.T) {
		th := scenaConLaCopia(t)
		bassa, altro := uid(0x710), uid(0x70f) // una seconda copia senza righe, prima del portatore; uno STEP di un altro contenuto
		conAllegato(&th, allegato(bassa, 10, "copia-2-7120100A_1.stp", "stp", shaStepB), nil)
		f := fattiStepF(t, sha("6"), []nodoF{{"#1", "7120900A", ""}}, nil)
		conAllegato(&th, allegato(altro, 11, "altro-acme.stp", "stp", sha("6")), &f)
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x73f), AllegatoID: altro,
			Sha256: shaStepB, Chiave: "#7", IDGrezzo: "7120700A", Fonte: "step", Stato: "aperta"})
		if bom := bomDi(t, valuta(t, th, m, nil)); bom.AllegatoID != aStepB {
			t.Errorf("BOM di lavoro sull'allegato %s: atteso il portatore %s", bom.AllegatoID, aStepB)
		}
	})
	t.Run("nessun allegato del thread porta le righe: resta quello del riferimento", func(t *testing.T) {
		th := scenaConLaCopia(t)
		fuori := uid(0x7ee) // un allegato che non è fra quelli del thread
		for i := range th.RigheComponenteProposta {
			th.RigheComponenteProposta[i].AllegatoID = fuori
		}
		if bom := bomDi(t, valuta(t, th, m, nil)); bom.AllegatoID != aCopia {
			t.Errorf("BOM di lavoro sull'allegato %s: atteso quello del riferimento, %s", bom.AllegatoID, aCopia)
		}
	})
}

// TestUnNodoMaiPropostoDalLegacy (T-B5-15, la rete di sicurezza; R80): un nodo della BOM di lavoro che non ha nessuna
// riga legacy del suo contenuto, su nessun allegato, conta fra i da decidere: il segno di «Conferma l'albero» non può
// coprire un nodo che il legacy non ha mai proposto. La radice della struttura resta esclusa. Riscritta per R-21 (la
// rete degli archi): per la gerarchia conta anche l'arco #1→#3, che non ha una riga.
func TestUnNodoMaiPropostoDalLegacy(t *testing.T) {
	m := motoreCatena(t)
	th := scenaAlbero(t)
	th.Fatti[shaStepB] = fattiStepF(t, shaStepB, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}},
		[]arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}})
	b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM
	if b.Verificata || asse(b.Nomenclatura) != "da_verificare/da_decidere" || b.Nomenclatura.DaDecidere != 1 ||
		asse(b.Gerarchia) != "da_verificare/da_decidere" || b.Gerarchia.DaDecidere != 2 {
		t.Errorf("nomenclatura %s (%d), gerarchia %s (%d): il nodo #3 e il suo arco senza righe sono da decidere", asse(b.Nomenclatura), b.Nomenclatura.DaDecidere,
			asse(b.Gerarchia), b.Gerarchia.DaDecidere)
	}
	// La radice di una struttura senza la sua riga non conta: la riga della radice non si decide mai (R80). Senza la fonte
	// confermata (la marcatura sta sulla riga della radice), la struttura è candidata.
	th = scenaAlbero(t)
	senzaFonte(&th)
	th.RigheComponenteProposta = th.RigheComponenteProposta[1:]
	if b := prodotto(t, valuta(t, th, m, nil), rifProdB).BOM; asse(b.Nomenclatura) != "verificata/" || b.Nomenclatura.DaDecidere != 0 {
		t.Errorf("nomenclatura %s (%d): la radice non è un nodo da decidere", asse(b.Nomenclatura), b.Nomenclatura.DaDecidere)
	}
}

// TestLaSommaParziale (T-B5-16; T-B0-24, R-08 di B4): sotto un nodo padre con un figlio non
// ancora deciso, una somma minore della quantità decisa non contraddice (il figlio aperto può essere il pezzo che
// manca); una somma maggiore sì. Un figlio scartato da una persona, o deciso come un altro componente, non sospende; uno
// scartato da un automatismo sì; un figlio aperto sotto un altro nodo padre non sospende niente.
func TestLaSommaParziale(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	rifRelazione := valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())
	conFratello := func(t *testing.T, qta int) fotorfq.Thread {
		t.Helper()
		th := scenaAlbero(t) // #1→#2 (qta 2), #2 deciso come lo sciolto
		aggiungiNodo(t, &th, "#3", "7120200A")
		th.Relazioni[0].Qta = qta
		return th
	}
	controlla := func(t *testing.T, th fotorfq.Thread, conflitto bool, decisione, proposta string) {
		t.Helper()
		v := valuta(t, th, m, nil)
		ger := prodotto(t, v, rifProdB).BOM.Gerarchia
		var quantita []valutazione.Conflitto
		for _, c := range v.Conflitti {
			if c.Motivo == valutazione.MotivoConflittoQuantita {
				quantita = append(quantita, c)
			}
		}
		if !conflitto {
			if ger.Stato == valutazione.StatoAsseConflitto || len(quantita) != 0 {
				t.Errorf("gerarchia %s, conflitti %+v: la somma parziale non contraddice", asse(ger), quantita)
			}
			return
		}
		if ger.Stato != valutazione.StatoAsseConflitto || len(quantita) != 1 || quantita[0].Rif != rifRelazione || quantita[0].Decisione != decisione ||
			quantita[0].Proposta != proposta {
			t.Errorf("gerarchia %s, conflitti %+v: atteso il conflitto %s contro %s", asse(ger), quantita, decisione, proposta)
		}
	}

	t.Run("un figlio aperto sotto lo stesso nodo padre sospende la somma minore", func(t *testing.T) {
		controlla(t, conFratello(t, 3), false, "", "")
	})
	t.Run("la somma maggiore resta un conflitto", func(t *testing.T) {
		controlla(t, conFratello(t, 1), true, "qta 1", "qta 2")
	})
	t.Run("un figlio scartato da una persona non sospende", func(t *testing.T) {
		th := conFratello(t, 3)
		r := rigaDi(t, &th, "#3")
		r.Stato, r.DecisoDa = "scartata", &op
		a := arcoDi(t, &th, "#1", "#3")
		a.Stato, a.DecisoDa = "scartata", &op
		controlla(t, th, true, "qta 3", "qta 2")
	})
	t.Run("un figlio scartato da un automatismo sospende", func(t *testing.T) {
		th := conFratello(t, 3)
		rigaDi(t, &th, "#3").Stato = "scartata"
		controlla(t, th, false, "", "")
	})
	t.Run("un figlio deciso come un altro componente non sospende", func(t *testing.T) {
		th := conFratello(t, 3)
		confermaComponente(&th, cTerzo, "7120300A", cProdotto, 1)
		decidi(t, &th, "#3", "confermata", cTerzo, &op)
		controlla(t, th, true, "qta 3", "qta 2")
	})
	t.Run("un figlio aperto sotto un altro nodo padre non sospende", func(t *testing.T) {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}, []arcoF{{"#1", "#2", 2}, {"#2", "#3", 1}})
		confermaComponente(&th, cSciolto, "7120200A", cProdotto, 3)
		decidi(t, &th, "#2", "confermata", cSciolto, &op)
		controlla(t, th, true, "qta 3", "qta 2")
	})
}

// TestLaGerarchiaSenzaBOMDiLavoro (T-B5-17 [T], lettura prudente dell'orchestratore, da confermare dall'utente; tabella
// degli assi di E1, E1R §3.2): nel legacy, con la fonte confermata ma senza la BOM di lavoro, la gerarchia che sarebbe
// verificata è da_verificare con il motivo bom_di_lavoro_assente; la nomenclatura non cambia. È fuori da K-02: senza la
// fonte confermata il motivo resta fonte_non_confermata. Nel modello nuovo niente cambia; un conflitto e
// non_verificabile prevalgono. Sulle scene: l'analisi dello STEP confermato in corso, la radice non registrata.
func TestLaGerarchiaSenzaBOMDiLavoro(t *testing.T) {
	arco := valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())
	struttura := func(fonte, bom bool) valutazione.StrutturaDaVerificare {
		return valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: fonte, BOMDiLavoro: bom, Radici: []string{radiceSintetica},
			Nodi: []string{radiceSintetica}, ArchiConfermati: []string{arco}}
	}
	legacy := valutazione.GestiVerificaBOM{Legacy: true, Nomenclatura: gestoSintetico(nil, []string{arco}), Gerarchia: gestoSintetico(nil, []string{arco})}

	t.Run("sulla regola", func(t *testing.T) {
		for _, c := range []struct {
			nome     string
			g        valutazione.GestiVerificaBOM
			s        valutazione.StrutturaDaVerificare
			nom, ger string
		}{
			{"legacy, fonte confermata, senza BOM di lavoro", legacy, struttura(true, false), "verificata/", "da_verificare/bom_di_lavoro_assente"},
			{"legacy, fonte confermata, con la BOM di lavoro", legacy, struttura(true, true), "verificata/", "verificata/"},
			{"legacy, senza fonte confermata: K-02", legacy, struttura(false, false), "verificata/", "da_verificare/fonte_non_confermata"},
			{"modello nuovo, fonte confermata, senza BOM di lavoro", valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil),
				Gerarchia: gestoSintetico(nil, nil)}, func() valutazione.StrutturaDaVerificare {
				s := senzaFigliSintetica(true)
				s.BOMDiLavoro = false
				return s
			}(), "verificata/", "verificata/"},
			{"legacy senza figli: non_verificabile prevale", valutazione.GestiVerificaBOM{Legacy: true}, valutazione.StrutturaDaVerificare{ConComponente: true,
				FonteConfermata: true, Radici: []string{radiceSintetica}, Nodi: []string{radiceSintetica}},
				"non_verificabile/senza_figli_nessun_gesto", "non_verificabile/senza_figli_nessun_gesto"},
			{"un conflitto prevale", legacy, func() valutazione.StrutturaDaVerificare {
				s := struttura(true, false)
				s.Conflitti = []valutazione.Conflitto{{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia, Rif: arco}}
				return s
			}(), "verificata/", "conflitto/conflitto"},
		} {
			v := valutazione.VerificaDellaBOM(c.g, c.s)
			if asse(v.Nomenclatura) != c.nom || asse(v.Gerarchia) != c.ger {
				t.Errorf("%s: nomenclatura %s, gerarchia %s; attese %s, %s", c.nome, asse(v.Nomenclatura), asse(v.Gerarchia), c.nom, c.ger)
			}
		}
	})
	t.Run("l'analisi dello STEP confermato in corso (R65 b A)", func(t *testing.T) {
		th := scenaAlbero(t)
		delete(th.Fatti, shaStepB)
		th.StepProdotto[0].Esito, th.StepProdotto[0].AnalisiCompleta = "presente_non_analizzato", false
		th.InAttesa = []uuid.UUID{aStepB}
		p := prodotto(t, valuta(t, th, motoreCatena(t), nil), rifProdB)
		if p.Fonte.Stato != valutazione.FonteConfermata || p.Struttura != ancoraggio.StatoStrutturaNessuna {
			t.Fatalf("fonte %s, struttura %s", statoMotivo(p.Fonte), p.Struttura)
		}
		if b := p.BOM; b.Verificata || asse(b.Nomenclatura) != "verificata/" || asse(b.Gerarchia) != "da_verificare/bom_di_lavoro_assente" || !b.Gerarchia.Legacy {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(b.Nomenclatura), asse(b.Gerarchia))
		}
	})
	t.Run("la radice non registrata (T-B0-08)", func(t *testing.T) {
		// Nessuna marcatura, e due righe del file senza un arco entrante (la radice e un nodo che il file non ha più):
		// le righe non dicono una radice sola.
		th := scenaAlbero(t)
		th.RigheComponenteProposta[0].Marcatura = nil
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x73e), AllegatoID: aStepB,
			NomeFile: "7120100A_1.stp", Sha256: shaStepB, Chiave: "#9", IDGrezzo: "7120900A", Famiglia: "acme-catena", Fonte: "step", Stato: "aperta"})
		v := valuta(t, th, motoreCatena(t), nil)
		p := prodotto(t, v, rifProdB)
		if p.Fonte.Stato != valutazione.FonteConfermata || p.Struttura != ancoraggio.StatoStrutturaCandidata ||
			len(conCodice(v.Diagnostiche, valutazione.CodiceRadiceNonRegistrata)) != 1 {
			t.Fatalf("fonte %s, struttura %s", statoMotivo(p.Fonte), p.Struttura)
		}
		if b := p.BOM; b.Verificata || asse(b.Nomenclatura) != "verificata/" || asse(b.Gerarchia) != "da_verificare/bom_di_lavoro_assente" {
			t.Errorf("nomenclatura %s, gerarchia %s", asse(b.Nomenclatura), asse(b.Gerarchia))
		}
	})
}

// TestIValoriDellaVerifica: i valori e i campi del contratto (§2.3, §2.6). Si riscrive con «Riscritta per …».
func TestIValoriDellaVerifica(t *testing.T) {
	for _, c := range [][2]string{
		{string(valutazione.StatoAsseDaVerificare), "da_verificare"}, {string(valutazione.StatoAsseVerificata), "verificata"},
		{string(valutazione.StatoAsseConflitto), "conflitto"}, {string(valutazione.StatoAsseNonVerificabile), "non_verificabile"},
		{valutazione.MotivoAsseFonteNonConfermata, "fonte_non_confermata"}, {valutazione.MotivoAsseBOMDiLavoroAssente, "bom_di_lavoro_assente"},
		{string(valutazione.ConflittoCodice), "codice"}, {string(valutazione.ConflittoAssociazione), "associazione"},
		{string(valutazione.ConflittoArco), "arco"}, {string(valutazione.ConflittoNuovoFile), "nuovo_file"},
		{string(valutazione.ConflittoIdentitaDocumento), "identita_documento"},
		{string(valutazione.AsseNomenclatura), "nomenclatura"}, {string(valutazione.AsseGerarchia), "gerarchia"},
		{string(valutazione.AsseSmistamento), "smistamento"},
		{valutazione.CodiceBOMFonteNonRegistrata, "bom.fonte_non_registrata"},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, atteso %q", c[0], c[1])
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{valutazione.GestiVerificaBOM{}, "Nomenclatura:nomenclatura Gerarchia:gerarchia Legacy:legacy"},
		{valutazione.GestoVerifica{}, "Da:da Il:il Nodi:nodi Archi:archi"},
		{valutazione.VerificaAsse{}, "Stato:stato Legacy:legacy DaDecidere:da_decidere Conflitti:conflitti Motivo:motivo"},
		{valutazione.VerificaBOM{}, "Nomenclatura:nomenclatura Gerarchia:gerarchia Verificata:verificata FonteRegistrata:fonte_registrata RimozioniAperte:rimozioni_aperte SenzaFigli:senza_figli"},
		{valutazione.Conflitto{}, "Tipo:tipo Asse:asse Rif:rif Prodotto:prodotto Decisione:decisione OrigineDecisione:origine_decisione Proposta:proposta Documentale:documentale Motivo:motivo EvidenzaDecisione:evidenza_decisione EvidenzaProposta:evidenza_proposta"},
	} {
		if got := campiJSON(c.tipo); got != c.campi {
			t.Errorf("%T: campi %q, attesi %q", c.tipo, got, c.campi)
		}
	}
	pv := reflect.TypeOf(valutazione.ProdottoValutato{})
	if f, ok := pv.FieldByName("Struttura"); !ok || f.Tag.Get("json") != "struttura" {
		t.Error("ProdottoValutato.Struttura")
	}
	if f, ok := pv.FieldByName("BOM"); !ok || f.Tag.Get("json") != "bom" {
		t.Error("ProdottoValutato.BOM")
	}
}

// campiJSON: i campi di una struttura con il nome del tag JSON, in ordine.
func campiJSON(v any) string {
	tp := reflect.TypeOf(v)
	var out []string
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		tag := f.Tag.Get("json")
		for j := 0; j < len(tag); j++ {
			if tag[j] == ',' {
				tag = tag[:j]
				break
			}
		}
		out = append(out, f.Name+":"+tag)
	}
	s := ""
	for i, x := range out {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}

// TestUnArcoMaiPropostoDalLegacy (R-21 della revisione della fase 3; contratto §1.6, PerimetroChiuso: «ogni nodo e ogni
// arco della BOM di lavoro … ha una decisione»; R80): nei fatti c'è l'arco #1→#3, il nodo #3 è deciso da una persona
// come un componente confermato, ma la riga dell'arco non c'è su nessun allegato, e la relazione confermata nemmeno.
// L'arco è da decidere: la gerarchia legacy non è verificata, e il perimetro non si chiude (archi_da_decidere), quindi la
// completezza non è completa. Con la riga dell'arco decisa da una persona su un allegato con lo stesso contenuto, l'arco
// è deciso.
func TestUnArcoMaiPropostoDalLegacy(t *testing.T) {
	m := motoreCatena(t)
	op := operatore
	scena := func(t *testing.T) fotorfq.Thread {
		t.Helper()
		th := scenaCompletezza(t, true)
		th.Fatti[shaStepB] = fattiStepF(t, shaStepB, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}},
			[]arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}})
		th.Componenti = append(th.Componenti, componente(cTerzo, "7120300A", "sciolto", "step"))
		ct, il := cTerzo, dataACME.Add(time.Hour)
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x73e), AllegatoID: aStepB,
			NomeFile: "7120100A_1.stp", Sha256: shaStepB, Chiave: "#3", IDGrezzo: "7120300A", Fonte: "step", Stato: "confermata", ComponenteID: &ct,
			DecisoDa: &op, DecisoIl: &il})
		return th
	}
	p := prodotto(t, valutaConVista(t, scena(t), m), rifProdB)
	if p.BOM.Verificata || asse(p.BOM.Gerarchia) != "da_verificare/da_decidere" || p.BOM.Gerarchia.DaDecidere != 1 || asse(p.BOM.Nomenclatura) != "verificata/" {
		t.Errorf("nomenclatura %s, gerarchia %s (%d)", asse(p.BOM.Nomenclatura), asse(p.BOM.Gerarchia), p.BOM.Gerarchia.DaDecidere)
	}
	if d := p.Documenti; d.PerimetroChiuso || d.MotivoPerimetro != valutazione.MotivoPerimetroArchiDaDecidere || d.Stato == valutazione.DocumentiCompleta {
		t.Errorf("completezza %s: l'arco senza riga non è deciso", statoDi(d))
	}
	th := scena(t)
	th.RigheRelazioneProposta = append(th.RigheRelazioneProposta, fotorfq.RigaRelazioneProposta{AllegatoID: aStepB, NomeFile: "7120100A_1.stp",
		PadreChiave: "#1", FiglioChiave: "#3", Qta: 1, Stato: "scartata", DecisoDa: &op})
	p = prodotto(t, valutaConVista(t, th, m), rifProdB)
	if asse(p.BOM.Gerarchia) != "verificata/" || !p.Documenti.PerimetroChiuso || p.Documenti.Stato != valutazione.DocumentiCompleta {
		t.Errorf("con la riga dell'arco tolta da una persona: gerarchia %s, completezza %s", asse(p.BOM.Gerarchia), statoDi(p.Documenti))
	}
}

// TestIlModelloNuovoConLeRelazioniConfermate (T-B5-99; R80: «senza figli né confermati né proposti»): sulla regola, nel
// modello nuovo, le relazioni confermate che il gesto non copre sono da decidere, per la nomenclatura e per la
// gerarchia; con figli confermati a mano un gesto sulla gerarchia vuota dello STEP non la verifica. La gerarchia è vuota
// solo senza archi nelle strutture e senza relazioni confermate: con le relazioni confermate coperte dal gesto della
// gerarchia, non aspetta la nomenclatura.
func TestIlModelloNuovoConLeRelazioniConfermate(t *testing.T) {
	arco := valutazione.RifArco("componente:"+cProdotto.String(), "componente:"+cSciolto.String())
	s := senzaFigliSintetica(true)
	s.ArchiConfermati = []string{arco}
	for _, c := range []struct {
		nome     string
		g        valutazione.GestiVerificaBOM
		nom, ger string
		verif    bool
	}{
		{"il gesto sulla gerarchia vuota dello STEP, con figli confermati a mano",
			valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil), Gerarchia: gestoSintetico(nil, nil)},
			"da_verificare/da_decidere", "da_verificare/da_decidere", false},
		{"i gesti coprono anche le relazioni confermate",
			valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, []string{arco}), Gerarchia: gestoSintetico(nil, []string{arco})},
			"verificata/", "verificata/", true},
		{"la gerarchia con le relazioni confermate non è vuota: non aspetta la nomenclatura",
			valutazione.GestiVerificaBOM{Nomenclatura: gestoSintetico([]string{radiceSintetica}, nil), Gerarchia: gestoSintetico(nil, []string{arco})},
			"da_verificare/da_decidere", "verificata/", false},
	} {
		v := valutazione.VerificaDellaBOM(c.g, s)
		if asse(v.Nomenclatura) != c.nom || asse(v.Gerarchia) != c.ger || v.Verificata != c.verif {
			t.Errorf("%s: nomenclatura %s, gerarchia %s, verificata %v", c.nome, asse(v.Nomenclatura), asse(v.Gerarchia), v.Verificata)
		}
	}
}
