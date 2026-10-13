package distintabozza

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Gli aiuti comuni delle prove L1 e L4 del package. Tutti i dati sono sintetici (codici ACM-…, nessun cliente vero).

// idDi da' un UUID fisso e leggibile per un nome: la stessa prova rifatta usa sempre gli stessi id.
func idDi(nome string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("distintabozza-prova:"+nome))
}

func pu(u uuid.UUID) *uuid.UUID { return &u }

const firmaProva = "1111111111111111111111111111111111111111111111111111111111111111"

func shaProva(c byte) string { return strings.Repeat(string(c), 64) }

// fonteScena: la fonte STEP della scena (nella L4 e' l'allegato «allegato» della RFQ, con sha256 b…b).
func fonteScena() Sorgente {
	return Sorgente{AllegatoID: pu(idDi("allegato")), Sha256: shaProva('b'), Gruppo: "cand:allegato:ACM-1000"}
}

// scena e' una bozza di prova con il suo prodotto e i componenti esistenti (chiave → componente_id).
type scena struct {
	prodotto   uuid.UUID
	componenti map[string]uuid.UUID
	snap       Snapshot
}

// scena35 costruisce la bozza dei 35 nodi come la working registrata: la radice (il finito), quattro sottoassiemi e
// 30 particolari (sciolti e commerciali), tutti registrati (op riusa, origine registrato) e legati in un albero:
// radice → s1..s4 (qta 1), s_i → p_i_j (qta j). E' lo stato di partenza «registrato» della prima versione. I
// componenti esistenti li da' chi chiama (nelle L4 sono righe vere di componente), o idDi.
func scena35(prodotto uuid.UUID, comp func(chiave string) uuid.UUID) scena {
	sc := scena{prodotto: prodotto, componenti: map[string]uuid.UUID{"k-root": prodotto}}
	s := Snapshot{Formato: FormatoSnapshot, ProdottoID: prodotto,
		VersioneBomBase: &VersioneBomBase{Numero: 0, Contatore: 0}, FirmaBase: firmaProva}
	s.Nodi = append(s.Nodi, Nodo{ChiaveBozza: "k-root", ComponenteID: pu(prodotto), Op: OpRiusa, Codice: "ACM-1000",
		Rev: "A", Descrizione: "Gruppo base di prova", Tipo: "finito", Origine: "registrato"})
	quanti := []int{8, 8, 7, 7} // 30 particolari
	for i := 1; i <= 4; i++ {
		ks := fmt.Sprintf("k-s%d", i)
		sc.componenti[ks] = comp(ks)
		s.Nodi = append(s.Nodi, Nodo{ChiaveBozza: ks, ComponenteID: pu(sc.componenti[ks]), Op: OpRiusa,
			Codice: fmt.Sprintf("ACM-1%d00", i), Rev: "A", Descrizione: fmt.Sprintf("Sottoassieme %d", i), Tipo: "sottoassieme",
			Origine: "registrato"})
		s.Archi = append(s.Archi, Arco{Padre: "k-root", Figlio: ks, Qta: 1, Origine: "registrato"})
		for j := 1; j <= quanti[i-1]; j++ {
			kp := fmt.Sprintf("k-p%d-%d", i, j)
			sc.componenti[kp] = comp(kp)
			tipo := "sciolto"
			if j%3 == 0 {
				tipo = "commerciale"
			}
			s.Nodi = append(s.Nodi, Nodo{ChiaveBozza: kp, ComponenteID: pu(sc.componenti[kp]), Op: OpRiusa,
				Codice: fmt.Sprintf("ACM-1%d%02d", i, j), Descrizione: fmt.Sprintf("Particolare %d.%d", i, j),
				Tipo: tipo, Origine: "registrato"})
			s.Archi = append(s.Archi, Arco{Padre: ks, Figlio: kp, Qta: j, Origine: "registrato"})
		}
	}
	s.normalizza()
	sc.snap = s
	return sc
}

// registratoDi: la working della scena come la ricostruisce il server (ListPerimetroRegistrato e
// ListArchiPerimetroRegistrato), per ControllaPartenza nelle L1.
func registratoDi(sc scena) Registrato {
	reg := Registrato{Prodotto: sc.prodotto}
	for _, n := range sc.snap.Nodi {
		reg.Componenti = append(reg.Componenti, ComponenteRegistrato{ComponenteID: *n.ComponenteID, Codice: n.Codice, Rev: n.Rev,
			Descrizione: n.Descrizione, Tipo: n.Tipo})
	}
	for _, a := range sc.snap.Archi {
		reg.Archi = append(reg.Archi, ArcoRegistrato{Padre: sc.componenti[a.Padre], Figlio: sc.componenti[a.Figlio], Qta: a.Qta, Posizione: a.Posizione})
	}
	return reg
}

func grezzo(t testing.TB, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// nodoDi restituisce il nodo con quella chiave (copia).
func nodoDi(s Snapshot, k string) Nodo {
	for _, n := range s.Nodi {
		if n.ChiaveBozza == k {
			return n
		}
	}
	panic("nodo assente: " + k)
}

func arcoDi(s Snapshot, padre, figlio string) Arco {
	for _, a := range s.Archi {
		if a.Padre == padre && a.Figlio == figlio {
			return a
		}
	}
	panic("arco assente: " + padre + " → " + figlio)
}

func cambiaNodo(s *Snapshot, k string, f func(n *Nodo)) {
	for i := range s.Nodi {
		if s.Nodi[i].ChiaveBozza == k {
			f(&s.Nodi[i])
			return
		}
	}
	panic("nodo assente: " + k)
}

func apertura(t testing.TB, partenza string) Azione {
	return Azione{Azione: AzBozzaAperta, Origine: "utente", Dopo: grezzo(t, map[string]string{"partenza": partenza})}
}

// passo e' un salvataggio della scena: le azioni dichiarate e lo snapshot che ne risulta, costruito a mano (non con
// Riconcilia), cosi' la prova confronta due strade indipendenti. n0: l'ultimo evento salvato prima di questo passo.
type passo struct {
	nome   string
	azioni []Azione
	snap   Snapshot
	n0     int
}

// passiScena: la prima versione (bozza_aperta dal registrato, la fonte scelta, le caselle STEP dei sottoassiemi),
// tre esclusioni, due aggiunte (una senza codice, E2.3), due ricodifiche, gli spostamenti (con un cambio di qta) con
// annulla e rifai nella stessa versione, e annulla e rifai di un evento della versione prima (stesso snapshot,
// versione nuova).
func passiScena(t testing.TB, sc scena) []passo {
	t.Helper()
	var out []passo
	n := 0 // eventi gia' salvati
	aggiungi := func(nome string, az []Azione, s Snapshot) {
		out = append(out, passo{nome: nome, azioni: az, snap: s.copia(), n0: n})
		n += len(az)
	}
	cur := sc.snap.copia()

	// apertura: partenza dal registrato, la fonte, le caselle dei quattro sottoassiemi (origine step)
	f := fonteScena()
	az := []Azione{apertura(t, PartenzaRegistrato), {Azione: AzSorgenteScelta, Origine: "utente", Dopo: grezzo(t, f)}}
	cur.Sorgente = &f
	for i := 1; i <= 4; i++ {
		k := fmt.Sprintf("k-s%d", i)
		caselle := []Casella{{Gruppo: f.Gruppo, Rif: fmt.Sprintf("nodo:%s:PRODUCT#%d", shaProva('b'), i)}}
		az = append(az, Azione{Azione: AzNodoModificato, ChiaveBozza: k, Origine: "step",
			Prima: grezzo(t, map[string]any{"caselle": nil}), Dopo: grezzo(t, map[string]any{"caselle": caselle})})
		cambiaNodo(&cur, k, func(x *Nodo) { x.Caselle = caselle })
	}
	cur.normalizza()
	aggiungi("apertura", az, cur)

	// tre esclusioni: l'arco tolto, op esclude, la decisione «escludi» con il motivo
	az = nil
	for _, k := range []string{"k-p1-8", "k-p2-8", "k-p3-7"} {
		nd := nodoDi(cur, k)
		padre := "k-s" + k[3:4]
		a := arcoDi(cur, padre, k)
		dec := map[string]any{"id": "d-escludi-" + k, "tipo": "escludi", "componente_id": nd.ComponenteID, "motivo": "fuori fornitura"}
		az = append(az,
			Azione{Azione: AzArcoTolto, Origine: "utente", Prima: grezzo(t, a)},
			Azione{Azione: AzNodoModificato, ChiaveBozza: k, Origine: "utente",
				Prima: grezzo(t, map[string]string{"op": OpRiusa}), Dopo: grezzo(t, map[string]string{"op": OpEsclude})},
			Azione{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: grezzo(t, dec)})
		cur.Archi = rimuoviArco(cur.Archi, padre, k)
		cambiaNodo(&cur, k, func(x *Nodo) { x.Op = OpEsclude })
		cur.Decisioni = append(cur.Decisioni, canonico(dec))
	}
	cur.normalizza()
	aggiungi("tre esclusioni", az, cur)

	// due aggiunte: una vite con il codice, una staffa senza codice (E2.3) con la posizione del cartiglio sull'arco
	az = nil
	nuovi := []Nodo{
		{ChiaveBozza: "k-new1", Op: OpCrea, Codice: "ACM-9001", Descrizione: "Vite M8x20", Tipo: "commerciale", Origine: "utente"},
		{ChiaveBozza: "k-new2", Op: OpCrea, Rev: "A", Descrizione: "Staffa di rinforzo", Tipo: "sciolto", Origine: "utente"},
	}
	archiNuovi := []Arco{{Padre: "k-s1", Figlio: "k-new1", Qta: 8, Origine: "utente"}, {Padre: "k-s2", Figlio: "k-new2", Qta: 2, Origine: "utente", Posizione: "40"}}
	for i := range nuovi {
		az = append(az, Azione{Azione: AzNodoAggiunto, ChiaveBozza: nuovi[i].ChiaveBozza, Origine: "utente", Dopo: grezzo(t, nuovi[i])},
			Azione{Azione: AzArcoAggiunto, Origine: "utente", Dopo: grezzo(t, archiNuovi[i])})
		cur.Nodi = append(cur.Nodi, nuovi[i])
		cur.Archi = append(cur.Archi, archiNuovi[i])
	}
	cur.normalizza()
	aggiungi("due aggiunte", az, cur)

	// due ricodifiche: il nodo registrato passa a «modifica», poi cambia codice con il motivo, e la decisione
	az = nil
	for _, r := range []struct{ k, dopo string }{{"k-p1-1", "ACM-1101-B"}, {"k-p4-2", "ACM-1402-X"}} {
		nd := nodoDi(cur, r.k)
		dec := map[string]any{"id": "d-ricodifica-" + r.k, "tipo": "ricodifica", "componente_id": nd.ComponenteID,
			"codice_prima": nd.Codice, "codice_dopo": r.dopo, "motivo": "codice letto male dal cartiglio"}
		az = append(az,
			Azione{Azione: AzNodoModificato, ChiaveBozza: r.k, Origine: "utente",
				Prima: grezzo(t, map[string]string{"op": OpRiusa}), Dopo: grezzo(t, map[string]string{"op": OpModifica})},
			Azione{Azione: AzNodoModificato, ChiaveBozza: r.k, Origine: "utente", Motivo: "codice letto male dal cartiglio",
				Prima: grezzo(t, map[string]string{"codice": nd.Codice}), Dopo: grezzo(t, map[string]string{"codice": r.dopo})},
			Azione{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: grezzo(t, dec)})
		cambiaNodo(&cur, r.k, func(x *Nodo) { x.Op, x.Codice = OpModifica, r.dopo })
		cur.Decisioni = append(cur.Decisioni, canonico(dec))
	}
	cur.normalizza()
	aggiungi("due ricodifiche", az, cur)

	// spostamenti: due particolari da s3 a s4 (uno con la qta nuova), una qta cambiata, e un nodo aggiunto, annullato,
	// rifatto e annullato di nuovo (nel giornale restano tutte e quattro le azioni, lo snapshot non lo vede)
	az = nil
	for _, m := range []struct {
		k   string
		qta int
	}{{"k-p3-1", 1}, {"k-p3-2", 5}} {
		a := arcoDi(cur, "k-s3", m.k)
		b := a
		b.Padre, b.Qta = "k-s4", m.qta
		az = append(az, Azione{Azione: AzArcoSpostato, Origine: "utente", Prima: grezzo(t, a), Dopo: grezzo(t, b)})
		cur.Archi = rimuoviArco(cur.Archi, "k-s3", m.k)
		cur.Archi = append(cur.Archi, b)
	}
	a := arcoDi(cur, "k-s2", "k-p2-3")
	az = append(az, Azione{Azione: AzQtaModificata, Origine: "utente",
		Prima: grezzo(t, map[string]any{"padre": "k-s2", "figlio": "k-p2-3", "qta": a.Qta}),
		Dopo:  grezzo(t, map[string]any{"padre": "k-s2", "figlio": "k-p2-3", "qta": 12})})
	nQta := n + len(az) // il numero dell'evento qta_modificata
	cur.Archi = rimuoviArco(cur.Archi, "k-s2", "k-p2-3")
	a.Qta = 12
	cur.Archi = append(cur.Archi, a)
	tmp := Nodo{ChiaveBozza: "k-tmp", Op: OpCrea, Codice: "ACM-TMP", Tipo: "sciolto", Origine: "utente"}
	az = append(az, Azione{Azione: AzNodoAggiunto, ChiaveBozza: "k-tmp", Origine: "utente", Dopo: grezzo(t, tmp)})
	nTmp := n + len(az)
	az = append(az,
		Azione{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": nTmp})},
		Azione{Azione: AzRifai, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": nTmp})},
		Azione{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": nTmp})})
	cur.normalizza()
	aggiungi("spostamenti", az, cur)

	// annulla e rifai di un evento della versione prima: lo snapshot torna uguale, ma le azioni ci sono e la versione
	// nuova si scrive (contratto §6.2: «stessa impronta con azioni non vuote → versione nuova»)
	az = []Azione{
		{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": nQta})},
		{Azione: AzRifai, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": nQta})},
	}
	aggiungi("annulla e rifai fra versioni", az, cur)
	return out
}

func rimuoviArco(archi []Arco, padre, figlio string) []Arco {
	out := archi[:0:0]
	for _, a := range archi {
		if a.Padre != padre || a.Figlio != figlio {
			out = append(out, a)
		}
	}
	return out
}

// precedentiDa: gli eventi gia' riconciliati come li darebbe il database (Contesto.Precedente).
func precedentiDa(eventi []Evento) Precedente {
	return func(n int) (Evento, bool, error) {
		if n < 1 || n > len(eventi) {
			return Evento{}, false, nil
		}
		return eventi[n-1], true, nil
	}
}
