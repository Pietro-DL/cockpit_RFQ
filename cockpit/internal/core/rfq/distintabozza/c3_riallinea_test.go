package distintabozza

// Le prove L1 di riancoraggio-1 (riallinea_proposta; direttiva 35, blocco C-3; EM §6.5; U2 §5.2, [U] Q10; MAT §4.3 riga 13),
// sulla versione 1 costruita dal registro di prova: la chiave del gruppo cambiata riscrive caselle, archi e sorgente senza
// niente da rivedere; un nodo proposta con l'identita' cambiata vuole recepisci o mantieni (riallineamento_incompleto
// senza; mantieni scrive la decisione identita alternativa sulla lettura di prima, che V-1 accetta contro il registro
// vecchio); un da_decidere prende da solo il codice proposto nuovo; una voce sparita vuole togli (il nodo esce, la sua PB va
// nel vassoio con il motivo nodo_tolto, nessun ricollegamento) o tieni_senza_casella (manuale senza codice, archi utente);
// una non_includere su un rif sparito vuole togli; una voce nuova sta fra i nuovi nodi proposti; un nodo registrato con la
// voce cambiata va da rivedere; una scelta che non serve e' un rifiuto. Ogni versione costruita passa la forma del formato
// 3 e nessuna PA diventa PB. Gli aiuti (spostaRegistro, versioneProva) servono anche alle prove del cambio della fonte.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// spostaRegistro: il contenuto del registro di prova con l'occorrenza (e, se dato, lo sha256) sostituiti dappertutto: la
// stessa struttura su un'altra copia, o un altro STEP.
func spostaRegistro(t *testing.T, c importazione.Contenuto, da, a uuid.UUID, shaDa, shaA string) importazione.Contenuto {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), da.String(), a.String())
	if shaDa != "" {
		s = strings.ReplaceAll(s, shaDa, shaA)
	}
	var out importazione.Contenuto
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out.AllegatiCitati) // in ordine, come li scrive l'adattatore
	return out
}

func componi(t *testing.T, c importazione.Contenuto) importazione.Registro {
	t.Helper()
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

// versioneProva: la versione 1 della partenza step sul registro di prova, con una PB sul nodo #31.
func versioneProva(t *testing.T, id idProva3, reg importazione.Registro) Snapshot3 {
	t.Helper()
	in, _ := ingressoStepProva(id)
	s, _, err := VersioneDaRegistro(reg, in)
	if err != nil {
		t.Fatal(err)
	}
	s.DocumentiPreassociati = []Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: ChiaveDellaVoce(id.allegato, id.rif("#31"))}}
	return s
}

func nodoC3(s Snapshot3, k string) (Nodo3, bool) {
	for _, n := range s.Nodi {
		if n.ChiaveBozza == k {
			return n, true
		}
	}
	return Nodo3{}, false
}

func TestV1C3RiancoraggioLaChiaveDelGruppo(t *testing.T) {
	id := idSintetici3()
	vecchio := componiProva3(t, id)
	prima := versioneProva(t, id, vecchio)
	c := registroProva3(t, id)
	c.Gruppo.Chiave = "cand:" + id.allegato.String() + ":" + id.rif("#1")
	nuovo := componi(t, c)
	es, err := Riancora(prima, &vecchio, nuovo, nil, "proposta di oggi", id.clienteE())
	if err != nil {
		t.Fatal(err)
	}
	s := es.Snapshot
	if s.Sorgente.Gruppo != c.Gruppo.Chiave || s.Sorgente.Importazione != nuovo.Impronta || len(es.DaRivedere) != 0 || len(es.IdentitaAggiornate) != 0 ||
		len(es.eventi) != 0 || len(s.Nodi) != len(prima.Nodi) {
		t.Fatalf("riancoraggio: %+v %+v", s.Sorgente, es)
	}
	for _, n := range s.Nodi {
		for _, x := range n.Caselle {
			if x.Gruppo != c.Gruppo.Chiave {
				t.Fatalf("casella non riscritta: %+v", x)
			}
		}
	}
	for _, a := range s.Archi {
		if a.Step != nil && a.Step.Gruppo != c.Gruppo.Chiave {
			t.Fatalf("arco non riscritto: %+v", a)
		}
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
}

func TestV1C3RiancoraggioLeScelte(t *testing.T) {
	id := idSintetici3()
	vecchio := componiProva3(t, id)
	prima := versioneProva(t, id, vecchio)
	k7, k9, k11, k31 := ChiaveDellaVoce(id.allegato, id.rif("#7")), ChiaveDellaVoce(id.allegato, id.rif("#9")),
		ChiaveDellaVoce(id.allegato, id.rif("#11")), ChiaveDellaVoce(id.allegato, id.rif("#31"))
	c := registroProva3(t, id)
	var tenute []importazione.Voce
	for _, v := range c.Proposta.Nodi {
		switch v.Origine.Rif {
		case id.rif("#7"):
			v.CodiceProposto = "ACM-1101"
		case id.rif("#9"):
			v.CodiceProposto, v.IdentitaProposta = "ACM-1109", rawDi(t, letturaProva3(id.allegato, "l:u:step:#9:id:acme-num/sei:0-8", id.clienteE(), "acme",
				"ACM-1109", map[string]any{}, nil))
		case id.rif("#11"), id.rif("#31"):
			continue // le voci sparite
		}
		tenute = append(tenute, v)
	}
	nuova := c.Proposta.Nodi[1]
	nuova.Voce, nuova.Origine.Rif, nuova.Origine.Chiave = id.voce("#40"), id.rif("#40"), "#40"
	nuova.CodiceProposto = "ACM-1140"
	c.Proposta.Nodi = append(tenute, nuova)
	var archi []importazione.ArcoRegistro
	for _, a := range c.Proposta.Archi {
		if a.Figlio != id.rif("#11") && a.Figlio != id.rif("#31") {
			archi = append(archi, a)
		}
	}
	c.Proposta.Archi = archi
	nuovo := componi(t, c)
	// senza scelte: riallineamento_incompleto con l'elenco
	_, err := Riancora(prima, &vecchio, nuovo, nil, "proposta di oggi", id.clienteE())
	if e := errore3(t, err); e.Motivo != MotivoRiallineamentoIncompleto || !strings.Contains(e.Messaggio, k7) || !strings.Contains(e.Messaggio, k11) ||
		!strings.Contains(e.Messaggio, k31) {
		t.Fatalf("senza scelte: %+v", e)
	}
	scelte := []SceltaR2{{ChiaveBozza: k7, Scelta: SceltaMantieni}, {ChiaveBozza: k11, Scelta: SceltaTieniSenzaCasella}, {ChiaveBozza: k31, Scelta: SceltaTogli}}
	es, err := Riancora(prima, &vecchio, nuovo, scelte, "proposta di oggi", id.clienteE())
	if err != nil {
		t.Fatal(err)
	}
	s := es.Snapshot
	// mantieni: il nodo passa al modo decisione con la decisione identita alternativa sulla lettura di prima
	n7, _ := nodoC3(s, k7)
	if n7.Identita.Modo != ModoDecisione || n7.Identita.Codice != "ACM-1100" || len(es.DecisioniScritte) != 1 || n7.Identita.Decisione != es.DecisioniScritte[0] {
		t.Fatalf("mantieni: %+v %v", n7.Identita, es.DecisioniScritte)
	}
	var dm Decisione3
	for _, d := range s.Decisioni {
		if d.ID == n7.Identita.Decisione {
			dm = d
		}
	}
	if dm.Tipo != DecisioneIdentita || dm.Modo != ModoAlternativa || dm.Importazione != vecchio.Impronta || dm.Motivo != "proposta di oggi" {
		t.Fatalf("decisione scritta: %+v", dm)
	}
	cas := n7.Caselle[0]
	if e := VerificaLetturaDecisione("decisione", &vecchio.Contenuto, dm, &Casella3{AllegatoID: cas.AllegatoID, Gruppo: "bom", Rif: cas.Rif}); e != nil {
		t.Fatalf("V-1 della decisione scritta: %+v", e)
	}
	// il da_decidere prende da solo il codice proposto nuovo
	n9, _ := nodoC3(s, k9)
	if n9.Identita.Modo != ModoProposta || n9.Identita.Codice != "ACM-1109" || len(es.IdentitaAggiornate) != 1 || es.IdentitaAggiornate[0].ChiaveBozza != k9 {
		t.Fatalf("da_decidere: %+v %+v", n9.Identita, es.IdentitaAggiornate)
	}
	// tieni_senza_casella: manuale senza codice, origine utente, gli archi step verso di lui diventano utente
	n11, _ := nodoC3(s, k11)
	if n11.Identita.Modo != ModoManuale || n11.Identita.Codice != "" || len(n11.Caselle) != 0 || n11.Origine != OrigineUtente {
		t.Fatalf("tieni: %+v", n11)
	}
	for _, a := range s.Archi {
		if a.Figlio == k11 && (a.Origine != OrigineUtente || a.Step != nil || a.Qta != 3) {
			t.Fatalf("arco del nodo tenuto: %+v", a)
		}
	}
	// togli: il nodo esce con il suo arco; la sua PB va nel vassoio (nodo_tolto), nessun ricollegamento
	if _, ok := nodoC3(s, k31); ok || len(s.DocumentiPreassociati) != 0 || len(es.Vassoio) != 1 || es.Vassoio[0] != id.pdf.String() {
		t.Fatalf("togli: %+v %v", s.DocumentiPreassociati, es.Vassoio)
	}
	azioni := []string{}
	for _, e := range es.eventi {
		azioni = append(azioni, e.Azione)
		if e.Azione == AzFileSvincolato && !strings.Contains(string(e.Dopo), `"motivo":"nodo_tolto"`) {
			t.Fatalf("file_svincolato: %s", e.Dopo)
		}
	}
	if strings.Join(azioni, ",") != "arco_tolto,file_svincolato,nodo_tolto" {
		t.Fatalf("eventi del server: %v", azioni)
	}
	// la voce nuova non entra: e' fra i nuovi nodi proposti
	if len(es.NuoviNodiProposti) != 1 || es.NuoviNodiProposti[0] != id.voce("#40") {
		t.Fatalf("nuovi nodi proposti: %v", es.NuoviNodiProposti)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
	// recepisci al posto di mantieni: l'identita' nuova, elencata
	scelte[0].Scelta = SceltaRecepisci
	es, err = Riancora(prima, &vecchio, nuovo, scelte, "proposta di oggi", id.clienteE())
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := nodoC3(es.Snapshot, k7); n.Identita.Modo != ModoProposta || n.Identita.Codice != "ACM-1101" || len(es.IdentitaAggiornate) != 2 {
		t.Fatalf("recepisci: %+v %+v", n.Identita, es.IdentitaAggiornate)
	}
	// una scelta che non serve e' un rifiuto
	_, err = Riancora(prima, &vecchio, nuovo, append(scelte, SceltaR2{ChiaveBozza: ChiaveDelComponente(id.prodotto), Scelta: SceltaTogli}), "x", id.clienteE())
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || !strings.HasPrefix(e.Campo, "riallinea_proposta.scelte[") {
		t.Fatalf("scelta che non serve: %+v", e)
	}
}

func TestV1C3RiancoraggioNonIncludereERegistrati(t *testing.T) {
	id := idSintetici3()
	in, comp := ingressoStepProva(id)
	cv := registroProva3(t, id)
	for i := range cv.Proposta.Nodi {
		if cv.Proposta.Nodi[i].Origine.Rif == id.rif("#9") {
			cv.Proposta.Nodi[i].DecisioneEsistente = rawDi(t, map[string]any{"componente_id": comp["r"].String(), "codice": "ACM-1900"})
		}
	}
	vecchio := componi(t, cv)
	prima, _, err := VersioneDaRegistro(vecchio, in)
	if err != nil {
		t.Fatal(err)
	}
	// il nodo #31 lo rappresenta una non_includere (il nodo e' tolto dalla bozza)
	k31 := ChiaveDellaVoce(id.allegato, id.rif("#31"))
	var nodi []Nodo3
	for _, n := range prima.Nodi {
		if n.ChiaveBozza != k31 {
			nodi = append(nodi, n)
		}
	}
	prima.Nodi = nodi
	var archi []Arco3
	for _, a := range prima.Archi {
		if a.Figlio != k31 {
			archi = append(archi, a)
		}
	}
	prima.Archi = archi
	az := uuid.MustParse("00000000-0000-4000-8000-0000000e0101")
	prima.Decisioni = []Decisione3{{ID: "ni-1", Tipo: DecisioneNonIncludere, Casella: &Casella3{AllegatoID: id.allegato, Gruppo: "bom", Rif: id.rif("#31")},
		MotivoCodice: "elemento_cad", Motivo: "cordone", AzioneID: &az}}
	if _, err := prima.Canonico(); err != nil {
		t.Fatalf("bozza di partenza: %v", err)
	}
	cn := registroProva3(t, id)
	var tenute []importazione.Voce
	for _, v := range cn.Proposta.Nodi {
		if v.Origine.Rif == id.rif("#31") {
			continue
		}
		if v.Origine.Rif == id.rif("#9") {
			v.DecisioneEsistente = rawDi(t, map[string]any{"componente_id": comp["r"].String(), "codice": "ACM-1900"})
			v.TipoProposto = rawDi(t, map[string]any{"valore": "commerciale", "origine": "regola"})
		}
		tenute = append(tenute, v)
	}
	cn.Proposta.Nodi = tenute
	var aa []importazione.ArcoRegistro
	for _, a := range cn.Proposta.Archi {
		if a.Figlio != id.rif("#31") {
			aa = append(aa, a)
		}
	}
	cn.Proposta.Archi = aa
	nuovo := componi(t, cn)
	_, err = Riancora(prima, &vecchio, nuovo, nil, "x", id.clienteE())
	if e := errore3(t, err); e.Motivo != MotivoRiallineamentoIncompleto || !strings.Contains(e.Messaggio, "ni-1") {
		t.Fatalf("non_includere senza scelta: %+v", e)
	}
	es, err := Riancora(prima, &vecchio, nuovo, []SceltaR2{{DecisioneID: "ni-1", Scelta: SceltaTogli}}, "x", id.clienteE())
	if err != nil {
		t.Fatal(err)
	}
	if len(es.Snapshot.Decisioni) != 0 {
		t.Fatalf("la non_includere non e' tolta: %+v", es.Snapshot.Decisioni)
	}
	k9 := ChiaveDellaVoce(id.allegato, id.rif("#9"))
	trovato := false
	for _, r := range es.DaRivedere {
		if r.ChiaveBozza == k9 && r.Motivo == RivederePropostaCambiata {
			trovato = true
		}
	}
	if !trovato {
		t.Fatalf("il registrato con la voce cambiata non e' da rivedere: %+v", es.DaRivedere)
	}
	if n, _ := nodoC3(es.Snapshot, k9); n.Identita.Modo != ModoRegistrato || n.Op != OpRiusa {
		t.Fatalf("il registrato e' stato riscritto: %+v", n)
	}
}
