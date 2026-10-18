package distintabozza

// La riconciliazione di un salvataggio del formato 3 (R2, forma salvataggio; EM §9.2, vocabolario 2; C3 §6.6): il server
// rifa' le azioni dichiarate dalla pagina sullo snapshot della versione precedente, controllando che ognuna trovi quello
// che dice e che ogni stato intermedio sia una bozza possibile; il risultato deve essere esattamente lo snapshot nuovo.
// Prima legge ogni azione con la forma del suo verbo (vocabolario2.go) e scrive l'evento con prima e dopo nella forma
// canonica, scritti dal server (non copiati dal corpo); poi applica l'azione con la regola condivisa con la pagina
// (applica2.go).
//
// annulla e rifai valgono solo per gli eventi dell'ingegnere scritti dopo l'ultima versione costruita dal server (EM §9.2:
// la pila d'annullamento non attraversa bozza_aperta, sorgente_scelta, sorgente_cambiata, proposta_riallineata), con il
// prima null oppure {azione_id} uguale a quello dell'evento (PE-05); il server scrive nel prima dell'annullamento
// l'azione_id dell'evento, quando c'e'.
//
// Pura: niente database (gli eventi gia' salvati li da' Contesto3.Precedente), niente orologio. Le verifiche contro i
// registri e il database (V-1...V-6, appartenenza, occorrenze attive) le fa chi salva (salva3.go).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Contesto3: quello che Riconcilia3 deve sapere oltre agli snapshot.
type Contesto3 struct {
	// N0: il numero dell'ultimo evento gia' salvato; gli eventi del salvataggio sono N0+1, N0+2, ...
	N0 int
	// Precedente: un evento gia' salvato, per numero (annulla e rifai di una versione precedente).
	Precedente func(n int) (EventoLetto, bool, error)
	// LimiteServer: il numero dell'ultimo evento con cui comincia una versione costruita dal server (0 se nessuno): un
	// annulla o un rifai di un evento con un numero fino a questo e' annullamento_non_ammesso.
	LimiteServer int
}

// RebaseV2 e' il rebase della sola base del formato 3 (EM §9.2): la base nuova e il motivo, che la richiesta di R2 ripete
// nel suo campo «rebase».
type RebaseV2 struct {
	ConfermaID     *uuid.UUID `json:"conferma_id"`
	Numero         int        `json:"numero"`
	Contatore      int64      `json:"contatore"`
	FirmaStruttura string     `json:"firma_struttura"`
}

// RebaseDichiarato3: l'effetto dell'azione rebase del vocabolario 2.
type RebaseDichiarato3 struct {
	Base   RebaseV2
	Motivo string
}

// Riconciliazione3 e' l'esito di Riconcilia3.
type Riconciliazione3 struct {
	Eventi []Evento
	// Rebase: il rebase dichiarato (al piu' uno per salvataggio).
	Rebase *RebaseDichiarato3
	// FontiCitate: gli allegati che sorgente_vista e sorgente_rifiutata nominano (chi salva li controlla nella RFQ).
	FontiCitate []Sorgente3
	// Strutturali: quante azioni hanno cambiato lo snapshot.
	Strutturali int
	// ProposteCitate: le proposte che file_preassociato, file_svincolato e file_lasciato nominano.
	ProposteCitate []uuid.UUID
}

var originiDelBrowser3 = map[string]bool{OrigineStep: true, OrigineUtente: true}

// Riconcilia3 confronta lo snapshot precedente con quello nuovo e valida le azioni del vocabolario 2 (commento di testa).
func Riconcilia3(prima, dopo Snapshot3, azioni []Azione, cx Contesto3) (Riconciliazione3, error) {
	var r Riconciliazione3
	if len(azioni) > MaxAzioni {
		return r, corpo("azioni", "%d azioni, oltre il limite di %d", len(azioni), MaxAzioni)
	}
	peso := 0
	for _, a := range azioni {
		peso += len(a.Prima) + len(a.Dopo) + len(a.Motivo)
	}
	if peso > MaxPesoAzioni {
		return r, corpo("azioni", "le azioni pesano %d byte, oltre il limite di %d", peso, MaxPesoAzioni)
	}
	if prima.ProdottoID != dopo.ProdottoID {
		return r, fuori("prodotto_id", "la bozza e' del prodotto %s, lo snapshot del %s", prima.ProdottoID, dopo.ProdottoID)
	}
	canPrima, err := prima.codifica()
	if err != nil {
		return r, fmt.Errorf("%w: %v", ErrSnapshotIncoerente, err)
	}
	canDopo, err := dopo.codifica()
	if err != nil {
		return r, corpo("snapshot", "lo snapshot non ha una forma canonica: %v", err)
	}
	st, err := statoV2Da(canPrima)
	if err != nil {
		return r, fmt.Errorf("%w: %v", ErrSnapshotIncoerente, err)
	}
	rc := &riconciliatore3{st: st, sorgente: prima.Sorgente, prodotto: prima.ProdottoID, cx: cx, r: &r}
	for i, a := range azioni {
		campo := fmt.Sprintf("azioni[%d]", i)
		if e := formaAzione3(campo, a); e != nil {
			return r, e
		}
		ev, err := rc.applica(campo, a)
		if err != nil {
			return r, err
		}
		if a.Azione != AzSorgenteVista && a.Azione != AzSorgenteRifiutata && a.Azione != AzFileLasciato {
			r.Strutturali++
		}
		r.Eventi = append(r.Eventi, ev)
	}
	rifatto, err := st.snapshotJSON()
	if err != nil {
		return r, corpo("snapshot", "lo stato rifatto non ha una forma canonica: %v", err)
	}
	if !bytes.Equal(rifatto, canDopo) {
		return r, primaDifferenza3(st, canDopo)
	}
	return r, nil
}

// formaAzione3: i controlli che non dipendono dallo stato.
func formaAzione3(campo string, a Azione) *Errore {
	if azioniDelServer2[a.Azione] {
		return corpo(campo+".azione", "%s lo scrive solo il server", a.Azione)
	}
	if !azioniNote2[a.Azione] {
		return corpo(campo+".azione", "azione sconosciuta nel vocabolario %d: %q", VocabolarioFormato3, a.Azione)
	}
	if !originiDelBrowser3[a.Origine] {
		return corpo(campo+".origine", "origine %q non ammessa qui (step o utente)", a.Origine)
	}
	if a.ChiaveBozza != "" && !reChiave.MatchString(a.ChiaveBozza) {
		return corpo(campo+".chiave_bozza", "chiave della bozza non valida")
	}
	if a.Motivo != "" && !testoLibero(nfc(a.Motivo), MaxMotivo) {
		return corpo(campo+".motivo", "il motivo non e' vuoto, sta in %d caratteri e non ha caratteri di controllo", MaxMotivo)
	}
	if len(a.Prima) > MaxPrimaDopo || len(a.Dopo) > MaxPrimaDopo {
		return corpo(campo, "prima o dopo oltre %d byte", MaxPrimaDopo)
	}
	return nil
}

type riconciliatore3 struct {
	st       *statoV2
	sorgente *Sorgente3
	prodotto uuid.UUID
	cx       Contesto3
	r        *Riconciliazione3
}

// nodo3Di: il nodo della bozza con quella chiave, tipizzato (gli oggetti dello stato sono nella forma canonica di Nodo3).
func (rc *riconciliatore3) nodo3Di(k string) (Nodo3, bool) {
	o, ok := rc.st.nodi[k]
	if !ok {
		return Nodo3{}, false
	}
	var n Nodo3
	if json.Unmarshal(canonico(o), &n) != nil {
		return Nodo3{}, false
	}
	return n, true
}

func (rc *riconciliatore3) eRadice(n Nodo3) bool {
	return n.ComponenteID != nil && *n.ComponenteID == rc.prodotto
}

// rifNonInclusi: i rif delle decisioni non_includere dello stato.
func (rc *riconciliatore3) rifNonInclusi() map[string]string {
	out := map[string]string{}
	for id, d := range rc.st.decisioni {
		if campoTesto(d, "tipo") != DecisioneNonIncludere {
			continue
		}
		var c struct {
			Casella struct {
				Rif string `json:"rif"`
			} `json:"casella"`
		}
		_ = json.Unmarshal(canonico(d), &c)
		out[c.Casella.Rif] = id
	}
	return out
}

// controllaNodo: i controlli locali su un nodo che entra o cambia: finito solo sulla radice, una radice sola, i rif delle
// caselle in un nodo solo e mai in una non_includere (contraddizione, EM §7.2).
func (rc *riconciliatore3) controllaNodo(campo string, n Nodo3, prima *Nodo3) *Errore {
	if n.Tipo == "finito" && !rc.eRadice(n) {
		return nonRiconciliate(campo, "il nodo %s sarebbe finito senza essere la radice del prodotto", n.ChiaveBozza)
	}
	if prima != nil && rc.eRadice(*prima) != rc.eRadice(n) {
		return nonRiconciliate(campo, "il nodo %s diventerebbe (o smetterebbe di essere) la radice del prodotto", n.ChiaveBozza)
	}
	if prima == nil && rc.eRadice(n) {
		for k := range rc.st.nodi {
			if x, ok := rc.nodo3Di(k); ok && k != n.ChiaveBozza && rc.eRadice(x) {
				return nonRiconciliate(campo, "il nodo %s avrebbe il componente del prodotto, che ha gia' la sua radice (%s)", n.ChiaveBozza, k)
			}
		}
	}
	nonInclusi := rc.rifNonInclusi()
	for _, c := range n.Caselle {
		if id, ok := nonInclusi[c.Rif]; ok {
			return corpo(campo, "il rif %s ha la decisione non_includere %s: non incluso e incluso insieme (prima si toglie la decisione)", c.Rif, id)
		}
		for k := range rc.st.nodi {
			if k == n.ChiaveBozza {
				continue
			}
			if x, ok := rc.nodo3Di(k); ok {
				for _, y := range x.Caselle {
					if y.Rif == c.Rif {
						return nonRiconciliate(campo, "il rif %s sta gia' nel nodo %s", c.Rif, k)
					}
				}
			}
		}
	}
	return nil
}

// leggiNodoV2: un nodo formato 3 intero da un prima o da un dopo, con le regole di forma del nodo dello snapshot.
func (rc *riconciliatore3) leggiNodoV2(campo string, raw json.RawMessage) (Nodo3, *Errore) {
	if nullo(raw) {
		return Nodo3{}, corpo(campo, "manca il nodo")
	}
	var in nodo3In
	if e := decodificaStretta(campo, raw, &in); e != nil {
		return Nodo3{}, e
	}
	n, e := in.converti(campo)
	if e != nil {
		return n, e
	}
	if e := validaNodo3(campo, n, rc.sorgente); e != nil {
		return n, e
	}
	normalizzaNodo3(&n)
	return n, nil
}

func normalizzaNodo3(n *Nodo3) {
	if len(n.Caselle) == 0 {
		n.Caselle = nil
	}
	sort.Slice(n.Caselle, func(a, b int) bool {
		x, y := n.Caselle[a], n.Caselle[b]
		if x.Gruppo != y.Gruppo {
			return x.Gruppo < y.Gruppo
		}
		if x.Rif != y.Rif {
			return x.Rif < y.Rif
		}
		return x.AllegatoID.String() < y.AllegatoID.String()
	})
}

// leggiArcoV2: un arco formato 3 intero, con le regole di forma dell'arco dello snapshot.
func (rc *riconciliatore3) leggiArcoV2(campo string, raw json.RawMessage) (Arco3, *Errore) {
	if nullo(raw) {
		return Arco3{}, corpo(campo, "manca l'arco")
	}
	var in arco3In
	if e := decodificaStretta(campo, raw, &in); e != nil {
		return Arco3{}, e
	}
	a, e := in.converti(campo)
	if e != nil {
		return a, e
	}
	return a, validaArco3(campo, a, rc.sorgente)
}

func (rc *riconciliatore3) estremiPresenti(campo string, a Arco3) *Errore {
	for _, k := range []string{a.Padre, a.Figlio} {
		if _, ok := rc.st.nodi[k]; !ok {
			return nonRiconciliate(campo, "l'arco %s → %s tocca il nodo %s, che non c'e' (si aggiunge prima il nodo)", a.Padre, a.Figlio, k)
		}
	}
	return nil
}

// campiNodoModificato3 e campiNodoRiusato3: i campi di un nodo che nodo_modificato e nodo_riusato del vocabolario 2
// possono toccare (EM §9.2: nodo_modificato tocca identita, descrizione, tipo, op, caselle; componente_id serve a
// «Fissa l'emittente» sul crea che collide, EM §3.6.1 caso P-4). La chiave e l'origine non cambiano.
var (
	campiNodoModificato3 = map[string]bool{"identita": true, "descrizione": true, "tipo": true, "op": true, "caselle": true, "componente_id": true}
	campiNodoRiusato3    = map[string]bool{"op": true, "componente_id": true, "identita": true}
)

// applica rifa' un'azione sullo stato e restituisce il suo evento.
func (rc *riconciliatore3) applica(campo string, a Azione) (Evento, error) {
	ev := Evento{Azione: a.Azione, Origine: a.Origine, Motivo: strings.TrimSpace(nfc(a.Motivo))}
	chiave := func(dedotta string) *Errore {
		if a.ChiaveBozza != "" && a.ChiaveBozza != dedotta {
			return nonRiconciliate(campo+".chiave_bozza", "l'azione dichiara la chiave %s ma riguarda %s", a.ChiaveBozza, dedotta)
		}
		ev.ChiaveBozza = dedotta
		return nil
	}
	vuoto := func(raw json.RawMessage, nome string) *Errore {
		if !nullo(raw) {
			return corpo(campo+"."+nome, "%s non porta %s", a.Azione, nome)
		}
		return nil
	}
	switch a.Azione {
	case AzNodoAggiunto:
		if e := vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		azione, e := leggiAzioneID(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		n, e := rc.leggiNodoV2(campo+".dopo", senzaAzioneID(a.Dopo))
		if e != nil {
			return ev, e
		}
		if e := chiave(n.ChiaveBozza); e != nil {
			return ev, e
		}
		if e := rc.controllaNodo(campo, n, nil); e != nil {
			return ev, e
		}
		ev.Dopo = conAzioneID(canonico(n), azione)
		if e := rc.st.applicaV2(campo, a.Azione, nil, canonico(n)); e != nil {
			return ev, e
		}
	case AzNodoTolto:
		azione, e := leggiSoloAzioneID(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		n, e := rc.leggiNodoV2(campo+".prima", a.Prima)
		if e != nil {
			return ev, e
		}
		if e := chiave(n.ChiaveBozza); e != nil {
			return ev, e
		}
		cur, ok := rc.nodo3Di(n.ChiaveBozza)
		if ok && rc.eRadice(cur) {
			return ev, nonRiconciliate(campo, "nodo_tolto: il nodo %s e' la radice del prodotto e non si toglie", n.ChiaveBozza)
		}
		if rc.st.grado[n.ChiaveBozza] > 0 || rc.st.preDi[n.ChiaveBozza] > 0 {
			return ev, nonRiconciliate(campo, "nodo_tolto: il nodo %s ha ancora archi o documenti preassociati (prima si tolgono quelli)", n.ChiaveBozza)
		}
		if e := rc.st.applicaV2(campo, a.Azione, canonico(n), nil); e != nil {
			return ev, e
		}
		ev.Prima = canonico(n)
		if azione != nil {
			ev.Dopo = conAzioneID(nil, azione)
		}
	case AzNodoModificato, AzNodoRiusato:
		if a.ChiaveBozza == "" {
			return ev, corpo(campo+".chiave_bozza", "%s vuole la chiave del nodo", a.Azione)
		}
		if e := chiave(a.ChiaveBozza); e != nil {
			return ev, e
		}
		ammessi := campiNodoModificato3
		if a.Azione == AzNodoRiusato {
			ammessi = campiNodoRiusato3
		}
		var p, d oggettoV2
		if e := decodificaStretta(campo+".prima", a.Prima, &p); e != nil {
			return ev, e
		}
		if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
			return ev, e
		}
		for k := range p {
			if !ammessi[k] {
				return ev, corpo(campo, "%s: il campo %q del nodo non si cambia cosi'", a.Azione, k)
			}
		}
		// i valori di prima e dopo nella forma del nodo: si leggono sovrapposti al nodo corrente, che e' gia' normalizzato
		cur, ok := rc.nodo3Di(a.ChiaveBozza)
		if !ok {
			return ev, nonRiconciliate(campo, "%s: il nodo %s non c'e'", a.Azione, a.ChiaveBozza)
		}
		pn, e := rc.sovrapponi3(campo+".prima", rc.st.nodi[a.ChiaveBozza], p)
		if e != nil {
			return ev, e
		}
		if !bytes.Equal(canonico(pn), canonico(cur)) {
			return ev, nonRiconciliate(campo, "%s: il nodo %s non ha i valori che l'azione dice di trovare", a.Azione, a.ChiaveBozza)
		}
		dn, e := rc.sovrapponi3(campo+".dopo", rc.st.nodi[a.ChiaveBozza], d)
		if e != nil {
			return ev, e
		}
		if a.Azione == AzNodoRiusato && (dn.Op != OpRiusa || dn.ComponenteID == nil) {
			return ev, corpo(campo, "nodo_riusato porta op e componente_id (e l'identita'): dopo, op riusa con il componente")
		}
		if e := rc.controllaNodo(campo, dn, &cur); e != nil {
			return ev, e
		}
		if dn.Op == OpEsclude && rc.st.preDi[a.ChiaveBozza] > 0 {
			return ev, nonRiconciliate(campo, "%s: il nodo %s ha documenti preassociati e non si esclude (prima si svincolano)", a.Azione, a.ChiaveBozza)
		}
		pp, dd := sottoinsieme3(cur, p), sottoinsieme3(dn, p)
		if _, _, e := rc.st.modificaV2(campo, a.Azione, a.ChiaveBozza, pp, dd); e != nil {
			return ev, e
		}
		rc.st.nodi[a.ChiaveBozza] = oggettoDiNodo3(dn)
		ev.Prima, ev.Dopo = pp, dd
	case AzArcoAggiunto:
		if e := vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		azione, e := leggiAzioneID(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		x, e := rc.leggiArcoV2(campo+".dopo", senzaAzioneID(a.Dopo))
		if e != nil {
			return ev, e
		}
		if e := chiave(x.Figlio); e != nil {
			return ev, e
		}
		if e := rc.estremiPresenti(campo, x); e != nil {
			return ev, e
		}
		if e := rc.st.applicaV2(campo, a.Azione, nil, canonico(x)); e != nil {
			return ev, e
		}
		ev.Dopo = conAzioneID(canonico(x), azione)
	case AzArcoTolto:
		azione, e := leggiSoloAzioneID(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		x, e := rc.leggiArcoV2(campo+".prima", a.Prima)
		if e != nil {
			return ev, e
		}
		if e := chiave(x.Figlio); e != nil {
			return ev, e
		}
		if e := rc.st.applicaV2(campo, a.Azione, canonico(x), nil); e != nil {
			return ev, e
		}
		ev.Prima = canonico(x)
		if azione != nil {
			ev.Dopo = conAzioneID(nil, azione)
		}
	case AzArcoSpostato:
		azione, e := leggiAzioneID(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		x, e := rc.leggiArcoV2(campo+".prima", a.Prima)
		if e != nil {
			return ev, e
		}
		y, e := rc.leggiArcoV2(campo+".dopo", senzaAzioneID(a.Dopo))
		if e != nil {
			return ev, e
		}
		// l'arco spostato non e' piu' quello dello STEP: origine utente, senza step (EM §6.6 passo 5); la qta e' quella
		// scritta dall'ingegnere
		if x.Figlio != y.Figlio || x.Padre == y.Padre || y.Origine != OrigineUtente || y.Step != nil {
			return ev, corpo(campo, "arco_spostato cambia il padre dello stesso figlio: l'arco spostato ha origine utente, senza step")
		}
		if e := chiave(x.Figlio); e != nil {
			return ev, e
		}
		if e := rc.estremiPresenti(campo, y); e != nil {
			return ev, e
		}
		if e := rc.st.applicaV2(campo, a.Azione, canonico(x), canonico(y)); e != nil {
			return ev, e
		}
		ev.Prima, ev.Dopo = canonico(x), conAzioneID(canonico(y), azione)
	case AzQtaModificata:
		var p, d struct {
			Padre  string `json:"padre"`
			Figlio string `json:"figlio"`
			Qta    *int   `json:"qta"`
		}
		if e := decodificaStretta(campo+".prima", a.Prima, &p); e != nil {
			return ev, e
		}
		if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
			return ev, e
		}
		if p.Qta == nil || d.Qta == nil || p.Padre != d.Padre || p.Figlio != d.Figlio {
			return ev, corpo(campo, "qta_modificata vuole prima e dopo con lo stesso padre e figlio e la qta")
		}
		if *d.Qta < 1 || *d.Qta > MaxQta {
			return ev, corpoMotivo(campo+".dopo.qta", MotivoArcoSenzaQuantita, "quantita' %d fuori da 1…%d", *d.Qta, MaxQta)
		}
		if *d.Qta == *p.Qta {
			return ev, nonRiconciliate(campo, "qta_modificata: la qta non cambia")
		}
		if e := chiave(p.Figlio); e != nil {
			return ev, e
		}
		pp := canonico(map[string]any{"padre": p.Padre, "figlio": p.Figlio, "qta": *p.Qta})
		dd := canonico(map[string]any{"padre": d.Padre, "figlio": d.Figlio, "qta": *d.Qta})
		if e := rc.st.applicaV2(campo, a.Azione, pp, dd); e != nil {
			return ev, e
		}
		ev.Prima, ev.Dopo = pp, dd
	case AzDecisioneAggiunta, AzDecisioneTolta:
		var d Decisione3
		raw, nome := a.Dopo, "dopo"
		if a.Azione == AzDecisioneTolta {
			raw, nome = a.Prima, "prima"
			if e := vuoto(a.Dopo, "dopo"); e != nil {
				return ev, e
			}
		} else if e := vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		if nullo(raw) {
			return ev, corpo(campo+"."+nome, "%s porta la decisione nel %s", a.Azione, nome)
		}
		d, e := LeggiDecisione3(campo+"."+nome, raw)
		if e != nil {
			return ev, e
		}
		can, err := d.Canonica()
		if err != nil {
			return ev, corpo(campo+"."+nome, "la decisione non ha una forma canonica: %v", err)
		}
		// la chiave e' quella del nodo della decisione, se ne nomina uno; una decisione senza chiave (non_includere, il
		// nodo e' tolto) tiene quella dichiarata dalla pagina
		ded := d.ChiaveBozza
		if ded == "" {
			ded = a.ChiaveBozza
		}
		if e := chiave(ded); e != nil {
			return ev, e
		}
		if a.Azione == AzDecisioneAggiunta {
			if d.Tipo == DecisioneNonIncludere {
				for k := range rc.st.nodi {
					if x, ok := rc.nodo3Di(k); ok {
						for _, c := range x.Caselle {
							if c.Rif == d.Casella.Rif {
								return ev, corpo(campo+".dopo.casella.rif", "il rif %s e' nelle caselle del nodo %s: non incluso e incluso insieme (prima si toglie il nodo)", c.Rif, k)
							}
						}
					}
				}
			}
			if e := rc.st.applicaV2(campo, a.Azione, nil, can); e != nil {
				return ev, e
			}
			ev.Dopo = can
		} else {
			if e := rc.st.applicaV2(campo, a.Azione, can, nil); e != nil {
				return ev, e
			}
			ev.Prima = can
		}
	case AzFilePreassociato:
		if e := vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		azione, e := leggiAzioneID(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		var x svincolo
		if e := decodificaStretta(campo+".dopo", senzaAzioneID(a.Dopo), &x); e != nil {
			return ev, e
		}
		if x.PropostaID == uuid.Nil || !reChiave.MatchString(x.ChiaveBozza) {
			return ev, corpo(campo+".dopo", "proposta_id e chiave_bozza")
		}
		if e := chiave(x.ChiaveBozza); e != nil {
			return ev, e
		}
		n, ok := rc.nodo3Di(x.ChiaveBozza)
		if !ok || n.Op == OpEsclude {
			return ev, nonRiconciliate(campo, "file_preassociato: il nodo %s non c'e' o e' escluso", x.ChiaveBozza)
		}
		if e := rc.st.applicaV2(campo, a.Azione, nil, canonico(x)); e != nil {
			return ev, e
		}
		rc.r.ProposteCitate = append(rc.r.ProposteCitate, x.PropostaID)
		ev.Dopo = conAzioneID(canonico(x), azione)
	case AzFileSvincolato:
		var x svincolo
		if e := decodificaStretta(campo+".prima", a.Prima, &x); e != nil {
			return ev, e
		}
		if x.PropostaID == uuid.Nil || !reChiave.MatchString(x.ChiaveBozza) {
			return ev, corpo(campo+".prima", "proposta_id e chiave_bozza")
		}
		d, e := LeggiDopoSvincolo(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		if d.PropostaID != x.PropostaID {
			return ev, corpo(campo+".dopo.proposta_id", "prima e dopo dello svincolo nominano la stessa proposta")
		}
		if e := chiave(x.ChiaveBozza); e != nil {
			return ev, e
		}
		if e := rc.st.applicaV2(campo, a.Azione, canonico(x), nil); e != nil {
			return ev, e
		}
		rc.r.ProposteCitate = append(rc.r.ProposteCitate, x.PropostaID)
		ev.Prima, ev.Dopo = canonico(x), canonico(d)
	case AzFileLasciato:
		if e := vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		d, e := LeggiDopoLasciato(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		if e := chiave(""); e != nil {
			return ev, e
		}
		rc.r.ProposteCitate = append(rc.r.ProposteCitate, d.PropostaID)
		ev.Dopo = canonico(d)
	case AzRebase:
		if rc.r.Rebase != nil {
			return ev, corpo(campo, "un salvataggio porta al piu' un rebase")
		}
		if ev.Motivo == "" {
			return ev, corpo(campo+".motivo", "il rebase vuole il motivo")
		}
		if e := chiave(""); e != nil {
			return ev, e
		}
		p, d, e := leggiRebase3(campo, a, rc.sorgente)
		if e != nil {
			return ev, e
		}
		if e := rc.st.applicaV2(campo, a.Azione, canonico(map[string]any{"base": p.Base}), canonico(map[string]any{"base": d.Base})); e != nil {
			return ev, e
		}
		rc.r.Rebase = &RebaseDichiarato3{Base: d.Base, Motivo: ev.Motivo}
		ev.Prima = canonico(map[string]any{"base": p.Base})
		ev.Dopo = canonico(map[string]any{"base": d.Base, "rif_scaduti": d.RifScaduti})
	case AzSorgenteVista, AzSorgenteRifiutata:
		if e := vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		s, e := leggiCitazione3(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		if a.Azione == AzSorgenteRifiutata && ev.Motivo == "" {
			return ev, corpo(campo+".motivo", "un rifiuto della sorgente vuole il motivo")
		}
		if a.ChiaveBozza != "" {
			n, ok := rc.nodo3Di(a.ChiaveBozza)
			if !ok || rc.eRadice(n) {
				return ev, nonRiconciliate(campo+".chiave_bozza", "%s: la destinazione %s non e' un nodo della bozza diverso dalla radice", a.Azione, a.ChiaveBozza)
			}
		}
		ev.ChiaveBozza = a.ChiaveBozza
		rc.r.FontiCitate = append(rc.r.FontiCitate, s)
		ev.Dopo = canonico(s)
	case AzAnnulla, AzRifai:
		return rc.annullaORifai(campo, a, ev)
	default:
		return ev, corpo(campo+".azione", "azione %q non applicabile", a.Azione)
	}
	return ev, nil
}

// annullaORifai: annulla applica l'inverso dell'evento n, rifai lo riapplica (EM §9.2, §7.6; PE-05).
func (rc *riconciliatore3) annullaORifai(campo string, a Azione, ev Evento) (Evento, error) {
	azione, e := LeggiPrimaAnnullamento(campo+".prima", a.Prima)
	if e != nil {
		return ev, e
	}
	n, e := leggiNumeroEvento(campo, a)
	if e != nil {
		return ev, e
	}
	b, err := rc.evento(n)
	if err != nil {
		return ev, err
	}
	if b == nil {
		return ev, nonRiconciliate(campo+".dopo.n", "%s nomina l'evento %d, che non c'e' prima di lui", a.Azione, n)
	}
	if n <= rc.cx.LimiteServer || b.Origine == "server" {
		return ev, corpoMotivo(campo+".dopo.n", MotivoAnnullamentoNonAmmesso,
			"l'evento %d e' prima dell'ultima versione costruita dal server (o e' del server): si torna indietro con un gesto nuovo", n)
	}
	if !azioniInvertibili2[b.Azione] {
		return ev, nonRiconciliate(campo+".dopo.n", "%s nomina l'evento %d (%s), che non si annulla ne' si rifa'", a.Azione, n, b.Azione)
	}
	idEvento, _ := leggiAzioneID(campo, b.Dopo)
	if idEvento == nil {
		idEvento, _ = leggiAzioneID(campo, b.Prima)
	}
	if azione != nil && (idEvento == nil || *azione != *idEvento) {
		return ev, nonRiconciliate(campo+".prima.azione_id", "l'azione_id del prima non e' quella dell'evento %d", n)
	}
	if a.ChiaveBozza != "" && a.ChiaveBozza != b.ChiaveBozza {
		return ev, nonRiconciliate(campo+".chiave_bozza", "l'azione dichiara la chiave %s ma l'evento %d riguarda %s", a.ChiaveBozza, n, b.ChiaveBozza)
	}
	ev.ChiaveBozza = b.ChiaveBozza
	verbo, prima, dopo := b.Azione, b.Prima, b.Dopo
	if a.Azione == AzAnnulla {
		var ok bool
		if verbo, prima, dopo, ok = inversaV2(b.Azione, b.Prima, b.Dopo); !ok {
			return ev, nonRiconciliate(campo, "l'evento %d non ha un inverso", n)
		}
	}
	if e := rc.applicaInterna(campo, verbo, b.ChiaveBozza, prima, dopo); e != nil {
		return ev, nonRiconciliate(campo, "%s dell'evento %d: %s", a.Azione, n, e.Messaggio)
	}
	ev.Dopo = canonico(map[string]int{"n": n})
	if idEvento != nil {
		ev.Prima = conAzioneID(nil, idEvento)
	}
	return ev, nil
}

// applicaInterna applica un evento gia' riconciliato (annulla, rifai), con i controlli locali di presenza.
func (rc *riconciliatore3) applicaInterna(campo, verbo, chiave string, prima, dopo json.RawMessage) *Errore {
	switch verbo {
	case AzNodoModificato, AzNodoRiusato:
		_, nuovo, e := rc.st.modificaV2(campo, verbo, chiave, prima, dopo)
		if e != nil {
			return e
		}
		var n Nodo3
		if json.Unmarshal(canonico(nuovo), &n) != nil {
			return corpo(campo, "il nodo %s non si rilegge", chiave)
		}
		return nil
	case AzNodoTolto:
		if rc.st.grado[chiave] > 0 || rc.st.preDi[chiave] > 0 {
			return nonRiconciliate(campo, "il nodo %s ha ancora archi o documenti preassociati", chiave)
		}
	case AzNodoAggiunto:
		var n Nodo3
		if json.Unmarshal(senzaAzioneID(dopo), &n) == nil {
			if e := rc.controllaNodo(campo, n, nil); e != nil {
				return e
			}
		}
	case AzArcoAggiunto, AzArcoSpostato:
		var x Arco3
		if json.Unmarshal(senzaAzioneID(dopo), &x) == nil {
			if e := rc.estremiPresenti(campo, x); e != nil {
				return e
			}
		}
	case AzFilePreassociato:
		var x svincolo
		if json.Unmarshal(senzaAzioneID(dopo), &x) == nil {
			if n, ok := rc.nodo3Di(x.ChiaveBozza); !ok || n.Op == OpEsclude {
				return nonRiconciliate(campo, "il nodo %s non c'e' o e' escluso", x.ChiaveBozza)
			}
		}
	case AzDecisioneAggiunta:
		var d struct {
			Tipo    string `json:"tipo"`
			Casella *struct {
				Rif string `json:"rif"`
			} `json:"casella"`
		}
		if json.Unmarshal(dopo, &d) == nil && d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			for k := range rc.st.nodi {
				if x, ok := rc.nodo3Di(k); ok {
					for _, c := range x.Caselle {
						if c.Rif == d.Casella.Rif {
							return corpo(campo, "il rif %s e' nelle caselle del nodo %s: non incluso e incluso insieme", c.Rif, k)
						}
					}
				}
			}
		}
	}
	return rc.st.applicaV2(campo, verbo, prima, dopo)
}

// evento: l'evento numero n, gia' salvato (Contesto3.Precedente) o di questo salvataggio (gia' riconciliato).
func (rc *riconciliatore3) evento(n int) (*Evento, error) {
	if n >= 1 && n <= rc.cx.N0 {
		if rc.cx.Precedente == nil {
			return nil, nil
		}
		e, ok, err := rc.cx.Precedente(n)
		if err != nil || !ok {
			return nil, err
		}
		return &Evento{Azione: e.Azione, ChiaveBozza: e.ChiaveBozza, Prima: e.Prima, Dopo: e.Dopo, Origine: e.Origine, Motivo: e.Motivo}, nil
	}
	if i := n - rc.cx.N0 - 1; i >= 0 && i < len(rc.r.Eventi) {
		e := rc.r.Eventi[i]
		return &e, nil
	}
	return nil, nil
}

// sovrapponi3: il nodo (oggetto canonico) con i campi dati al posto dei suoi, riletto con le regole di forma del nodo.
func (rc *riconciliatore3) sovrapponi3(campo string, cur oggettoV2, campi oggettoV2) (Nodo3, *Errore) {
	m := oggettoV2{}
	for k, v := range cur {
		m[k] = v
	}
	for k, v := range campi {
		if nullo(v) {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	return rc.leggiNodoV2(campo, canonico(m))
}

// sottoinsieme3: i campi nominati del nodo, nella forma canonica (null per un campo assente).
func sottoinsieme3(n Nodo3, campi oggettoV2) json.RawMessage {
	m := oggettoDiNodo3(n)
	out := map[string]json.RawMessage{}
	for k := range campi {
		if v, ok := m[k]; ok {
			out[k] = v
		} else {
			out[k] = json.RawMessage("null")
		}
	}
	return canonico(out)
}

func oggettoDiNodo3(n Nodo3) oggettoV2 {
	var m oggettoV2
	_ = json.Unmarshal(canonico(n), &m)
	return m
}

// rebase3: la forma del rebase del vocabolario 2: prima {base}, dopo {base, rif_scaduti}.
type rebase3Prima struct {
	Base RebaseV2 `json:"base"`
}

type rebase3Dopo struct {
	Base       RebaseV2          `json:"base"`
	RifScaduti []json.RawMessage `json:"rif_scaduti"`
}

func (b RebaseV2) valida(campo string) *Errore {
	if b.Numero < 0 || b.Contatore < 0 || (b.ConfermaID == nil) != (b.Numero == 0) {
		return corpo(campo, "la base vuole conferma_id e numero insieme (nessuna conferma: null e 0) e un contatore")
	}
	if b.ConfermaID != nil && *b.ConfermaID == uuid.Nil {
		return corpo(campo+".conferma_id", "UUID nullo")
	}
	if !reSha256.MatchString(b.FirmaStruttura) {
		return corpo(campo+".firma_struttura", "la firma della base e' di 64 cifre esadecimali minuscole")
	}
	return nil
}

func leggiRebase3(campo string, a Azione, sorgente *Sorgente3) (rebase3Prima, rebase3Dopo, *Errore) {
	var p rebase3Prima
	var d rebase3Dopo
	// il rebase del formato 3 cambia solo la base (EM §9.2): una sorgente nel prima o nel dopo, se c'e', e' quella della
	// bozza; un'altra e' rebase_cambia_sorgente (la fonte cambia solo con cambia_sorgente e riallinea_proposta)
	for _, x := range []struct {
		nome string
		raw  json.RawMessage
	}{{"prima", a.Prima}, {"dopo", a.Dopo}} {
		var m oggettoV2
		if nullo(x.raw) || json.Unmarshal(x.raw, &m) != nil {
			return p, d, corpo(campo+"."+x.nome, "il rebase porta nel %s la base", x.nome)
		}
		s, ok := m["sorgente"]
		if !ok {
			continue
		}
		if !nullo(s) || sorgente != nil {
			var cur json.RawMessage = json.RawMessage("null")
			if sorgente != nil {
				cur = canonico(sorgente)
			}
			if !bytes.Equal(canonico(s), canonico(cur)) {
				return p, d, corpoMotivo(campo+"."+x.nome+".sorgente", MotivoRebaseCambiaSorgente,
					"il rebase del formato 3 cambia solo la base: la fonte e il registro cambiano con cambia_sorgente o riallinea_proposta")
			}
		}
		delete(m, "sorgente")
		raw := canonico(m)
		if x.nome == "prima" {
			if e := decodificaStretta(campo+".prima", raw, &p); e != nil {
				return p, d, e
			}
		} else if e := decodificaStretta(campo+".dopo", raw, &d); e != nil {
			return p, d, e
		}
	}
	if p.Base.FirmaStruttura == "" {
		if e := decodificaStretta(campo+".prima", a.Prima, &p); e != nil {
			return p, d, e
		}
	}
	if d.Base.FirmaStruttura == "" {
		if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
			return p, d, e
		}
	}
	if e := p.Base.valida(campo + ".prima.base"); e != nil {
		return p, d, e
	}
	if e := d.Base.valida(campo + ".dopo.base"); e != nil {
		return p, d, e
	}
	if len(d.RifScaduti) > MaxVoci {
		return p, d, corpo(campo+".dopo.rif_scaduti", "troppi rif scaduti (al piu' %d)", MaxVoci)
	}
	for i, x := range d.RifScaduti {
		var o map[string]json.RawMessage
		if err := json.Unmarshal(x, &o); err != nil {
			return p, d, corpo(fmt.Sprintf("%s.dopo.rif_scaduti[%d]", campo, i), "un rif scaduto e' un oggetto {chiave_bozza | decisione, casella, motivo}")
		}
		for k := range o {
			switch k {
			case "chiave_bozza", "decisione", "casella", "motivo":
			default:
				return p, d, corpo(fmt.Sprintf("%s.dopo.rif_scaduti[%d].%s", campo, i, k), "campo non ammesso in un rif scaduto")
			}
		}
	}
	if d.RifScaduti == nil {
		d.RifScaduti = []json.RawMessage{}
	}
	return p, d, nil
}

// leggiCitazione3: la fonte nominata da sorgente_vista e sorgente_rifiutata: {allegato_id, gruppo, sha256?}.
func leggiCitazione3(campo string, raw json.RawMessage) (Sorgente3, *Errore) {
	var in struct {
		AllegatoID *uuid.UUID `json:"allegato_id"`
		Gruppo     string     `json:"gruppo"`
		Sha256     string     `json:"sha256"`
	}
	if nullo(raw) {
		return Sorgente3{}, corpo(campo, "manca la fonte: allegato_id e gruppo")
	}
	if e := decodificaStretta(campo, raw, &in); e != nil {
		return Sorgente3{}, e
	}
	if in.AllegatoID == nil || *in.AllegatoID == uuid.Nil {
		return Sorgente3{}, corpo(campo+".allegato_id", "la fonte citata e' un allegato")
	}
	if !testoLibero(in.Gruppo, MaxGruppo) {
		return Sorgente3{}, corpo(campo+".gruppo", "il gruppo e' obbligatorio, al piu' %d caratteri", MaxGruppo)
	}
	if in.Sha256 != "" && !reSha256.MatchString(in.Sha256) {
		return Sorgente3{}, corpo(campo+".sha256", "lo sha256 e' di 64 cifre esadecimali minuscole")
	}
	return Sorgente3{AllegatoID: in.AllegatoID, Gruppo: in.Gruppo, Sha256: in.Sha256}, nil
}

// primaDifferenza3 dice, per nome, dove lo stato rifatto e lo snapshot nuovo differiscono.
func primaDifferenza3(st *statoV2, canDopo []byte) *Errore {
	atteso, err := statoV2Da(canDopo)
	if err != nil {
		return nonRiconciliate("snapshot", "le azioni non danno lo snapshot salvato")
	}
	if !bytes.Equal(canonico(st.testa["versione_bom_base"]), canonico(atteso.testa["versione_bom_base"])) ||
		!bytes.Equal(canonico(st.testa["firma_base"]), canonico(atteso.testa["firma_base"])) {
		return &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoAzioniNonRiconciliate, Campo: "rebase",
			Messaggio: "la base (versione_bom_base, firma_base) cambia, ma nessuna azione rebase lo dice"}
	}
	if !bytes.Equal(canonico(st.testa["sorgente"]), canonico(atteso.testa["sorgente"])) {
		return &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoRebaseCambiaSorgente, Campo: "snapshot.sorgente",
			Messaggio: "la fonte e il registro cambiano solo con cambia_sorgente e riallinea_proposta, mai in un salvataggio"}
	}
	for _, k := range unione(chiavi(st.nodi), chiavi(atteso.nodi)) {
		a, okA := st.nodi[k]
		b, okB := atteso.nodi[k]
		switch {
		case okA && !okB:
			return nonRiconciliate("snapshot.nodi", "il nodo %s non c'e' piu', ma nessuna azione lo toglie", k)
		case !okA && okB:
			return nonRiconciliate("snapshot.nodi", "il nodo %s e' nuovo, ma nessuna azione lo aggiunge", k)
		case !stessoOggetto(a, b):
			return nonRiconciliate("snapshot.nodi", "il nodo %s e' cambiato, ma nessuna azione lo dice", k)
		}
	}
	for k, b := range atteso.archi {
		if a, ok := st.archi[k]; !ok || !stessoOggetto(a, b) {
			return nonRiconciliate("snapshot.archi", "l'arco %s → %s e' nuovo o cambiato, ma nessuna azione lo dice", k[0], k[1])
		}
	}
	for k := range st.archi {
		if _, ok := atteso.archi[k]; !ok {
			return nonRiconciliate("snapshot.archi", "l'arco %s → %s non c'e' piu', ma nessuna azione lo toglie", k[0], k[1])
		}
	}
	if len(st.decisioni) != len(atteso.decisioni) {
		return nonRiconciliate("snapshot.decisioni", "le decisioni cambiano, ma le azioni non lo dicono")
	}
	for k, b := range atteso.decisioni {
		if a, ok := st.decisioni[k]; !ok || !stessoOggetto(a, b) {
			return nonRiconciliate("snapshot.decisioni", "la decisione %s e' nuova o cambiata, ma nessuna azione lo dice", k)
		}
	}
	for k, b := range atteso.pre {
		if a, ok := st.pre[k]; !ok || !stessoOggetto(a, b) {
			return nonRiconciliate("snapshot.documenti_preassociati", "la preassociazione di %s e' nuova o cambiata, ma nessuna azione lo dice", k)
		}
	}
	if len(st.pre) != len(atteso.pre) {
		return nonRiconciliate("snapshot.documenti_preassociati", "le preassociazioni cambiano, ma le azioni non lo dicono")
	}
	return nonRiconciliate("snapshot", "le azioni non danno lo snapshot salvato")
}

// MotivoRebaseCambiaSorgente: un salvataggio (anche con un rebase) che cambia la sorgente o il registro (EM §9.2).
const MotivoRebaseCambiaSorgente = "rebase_cambia_sorgente"

func chiavi(m map[string]oggettoV2) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func unione(a, b []string) []string {
	m := map[string]bool{}
	for _, x := range append(append([]string{}, a...), b...) {
		m[x] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
