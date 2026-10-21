package distintabozza

// Gli attrezzi comuni a riancoraggio-1 (riallinea_proposta, riallinea3.go) e cambio-fonte-1 (cambia_sorgente,
// cambiafonte3.go): le versioni n+1 che il server costruisce da una bozza aperta e da un registro nuovo (EM §6.5, §6.6;
// direttiva 35, blocco C-3). Puri: niente database.
//   - la scelta recepisci / mantieni per un nodo proposta la cui identita' proposta cambia (U2 §5.2, [U] Q10): recepisci
//     prende l'identita' nuova; mantieni scrive la decisione identita alternativa sulla lettura di prima, con importazione =
//     il registro vecchio, autore e motivo del gesto (la regola gia' scritta per «Tieni come nodo a mano»); senza scelta
//     422 riallineamento_incompleto. Un nodo da_decidere prende da solo l'identita' della voce nuova (un valore derivato di
//     un nodo non toccato, A3-Q10 [T]: un nodo da_decidere su una voce con il codice proposto non passerebbe V-1);
//   - gli eventi che il server scrive nella versione (origine server, non annullabili): nodo_tolto, arco_tolto,
//     file_svincolato, decisione_tolta [T: le decisioni che citano un nodo tolto cadono con lui];
//   - i da_rivedere, le identita' aggiornate, le decisioni scritte.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// I motivi di da_rivedere[] (EM §6.5, §6.6; gli stessi di propostabom).
const (
	RivederePropostaCambiata   = "proposta_cambiata"
	RivedereDecisioneEsistente = "decisione_esistente"
	RivedereLetturaAssente     = "lettura_assente"
	RivedereLetturaCambiata    = "lettura_cambiata"
	RivedereAltraOccorrenza    = "altra_occorrenza"
	RivedereFonteCambiata      = "fonte_cambiata"
)

// eventoServer: un evento del giornale scritto dal server dentro una forma costruita dal server.
type eventoServer struct {
	Azione      string
	ChiaveBozza string
	Prima       json.RawMessage
	Dopo        json.RawMessage
	Motivo      string
}

// cantiere: lo stato di una versione n+1 in costruzione: lo snapshot, gli eventi del server, gli elenchi della risposta.
type cantiere struct {
	s          Snapshot3
	eventi     []eventoServer
	rivedere   map[DaRivedere3]bool
	aggiornate []IdentitaAggiornata3
	scritte    []string
	vassoio    []string // gli allegati delle PB che vanno nel vassoio
	scelte     map[string]string
	usate      map[string]bool
	mancanti   []string
	motivo     string
}

func nuovoCantiere(s Snapshot3, scelte []SceltaR2, motivo string) *cantiere {
	c := &cantiere{s: s.copia(), rivedere: map[DaRivedere3]bool{}, scelte: map[string]string{}, usate: map[string]bool{}, motivo: motivo}
	for _, x := range scelte {
		k := "n:" + x.ChiaveBozza
		if x.DecisioneID != "" {
			k = "d:" + x.DecisioneID
		}
		c.scelte[k] = x.Scelta
	}
	return c
}

func (c *cantiere) rivedi(chiave, decisione, motivo string) {
	c.rivedere[DaRivedere3{ChiaveBozza: chiave, DecisioneID: decisione, Motivo: motivo}] = true
}

// scelta: la scelta del corpo per un nodo o una decisione, segnata come usata ("" = nessuna).
func (c *cantiere) scelta(chiave, decisione string) string {
	k := "n:" + chiave
	if decisione != "" {
		k = "d:" + decisione
	}
	s, ok := c.scelte[k]
	if ok {
		c.usate[k] = true
	}
	return s
}

func (c *cantiere) manca(cosa string) { c.mancanti = append(c.mancanti, cosa) }

// controllaScelte: le scelte che mancano (422 riallineamento_incompleto, con l'elenco) e quelle che non servono (422 sul
// campo della scelta: una scelta che non si applica non si ignora in silenzio [T]).
func (c *cantiere) controllaScelte(campo string, scelte []SceltaR2) *Errore {
	if len(c.mancanti) > 0 {
		sort.Strings(c.mancanti)
		return corpoMotivo(campo, MotivoRiallineamentoIncompleto, "mancano le scelte per: %s", strings.Join(c.mancanti, ", "))
	}
	for i, x := range scelte {
		k := "n:" + x.ChiaveBozza
		if x.DecisioneID != "" {
			k = "d:" + x.DecisioneID
		}
		if !c.usate[k] {
			return corpo(fmt.Sprintf("%s[%d]", campo, i), "la scelta %s per %s non si applica: quel nodo o quella decisione non la chiede", x.Scelta,
				strings.TrimPrefix(strings.TrimPrefix(k, "n:"), "d:"))
		}
	}
	return nil
}

func (c *cantiere) evento(azione, chiave string, prima, dopo any, motivo string) {
	e := eventoServer{Azione: azione, ChiaveBozza: chiave, Motivo: motivo}
	if prima != nil {
		e.Prima = canonico(prima)
	}
	if dopo != nil {
		e.Dopo = canonico(dopo)
	}
	c.eventi = append(c.eventi, e)
}

// togliNodo: il nodo esce dalla bozza con i suoi archi (arco_tolto), le sue preassociazioni (file_svincolato con il motivo:
// vanno nel vassoio) e le decisioni che lo citano (decisione_tolta); tutti eventi del server.
func (c *cantiere) togliNodo(chiave, motivoPB string) {
	var nodo *Nodo3
	nodi := c.s.Nodi[:0]
	for i := range c.s.Nodi {
		if c.s.Nodi[i].ChiaveBozza == chiave {
			n := c.s.Nodi[i]
			nodo = &n
			continue
		}
		nodi = append(nodi, c.s.Nodi[i])
	}
	c.s.Nodi = nodi
	if nodo == nil {
		return
	}
	archi := c.s.Archi[:0]
	for _, a := range c.s.Archi {
		if a.Padre == chiave || a.Figlio == chiave {
			c.evento(AzArcoTolto, a.Figlio, a, nil, "")
			continue
		}
		archi = append(archi, a)
	}
	c.s.Archi = archi
	pb := c.s.DocumentiPreassociati[:0]
	for _, p := range c.s.DocumentiPreassociati {
		if p.ChiaveBozza == chiave {
			c.evento(AzFileSvincolato, chiave, Preassociazione3{AllegatoID: p.AllegatoID, ChiaveBozza: chiave},
				map[string]any{"allegato_id": p.AllegatoID, "motivo": motivoPB}, "")
			c.vassoio = append(c.vassoio, p.AllegatoID.String())
			continue
		}
		pb = append(pb, p)
	}
	c.s.DocumentiPreassociati = pb
	c.togliDecisioniDi(chiave)
	c.evento(AzNodoTolto, chiave, *nodo, nil, "")
}

// citaNodo: la decisione cita il nodo (chiave_bozza, il soggetto di una conferma_categoria).
func citaNodo(d Decisione3, chiave string) bool {
	return d.ChiaveBozza == chiave || (d.Soggetto != nil && d.Soggetto.ChiaveBozza == chiave)
}

func (c *cantiere) togliDecisioniDi(chiave string) {
	dd := c.s.Decisioni[:0]
	for _, d := range c.s.Decisioni {
		if citaNodo(d, chiave) {
			c.evento(AzDecisioneTolta, chiaveEventoDecisione(d), d, nil, "")
			continue
		}
		dd = append(dd, d)
	}
	c.s.Decisioni = dd
}

func (c *cantiere) togliDecisione(id string) {
	dd := c.s.Decisioni[:0]
	for _, d := range c.s.Decisioni {
		if d.ID == id {
			c.evento(AzDecisioneTolta, chiaveEventoDecisione(d), d, nil, "")
			continue
		}
		dd = append(dd, d)
	}
	c.s.Decisioni = dd
}

// chiaveEventoDecisione: la chiave dell'evento di una decisione, quella della decisione se il tipo ha il campo (DEC-B PD-08 (a)).
func chiaveEventoDecisione(d Decisione3) string { return d.ChiaveBozza }

// idDecisioneScritta: l'id di una decisione scritta dal server per un nodo e un registro, deterministico e unico nello
// snapshot (un suffisso se gia' preso).
func (c *cantiere) idDecisioneScritta(prefisso, chiave, registro string) string {
	h := sha256.Sum256([]byte(chiave + "\x00" + registro))
	base := prefisso + "-" + hex.EncodeToString(h[:8])
	id := base
	for i := 2; ; i++ {
		presa := false
		for _, d := range c.s.Decisioni {
			if d.ID == id {
				presa = true
			}
		}
		if !presa {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// mantieni: la decisione identita alternativa sulla lettura che aveva dato l'identita' proposta del nodo, nel registro
// vecchio (EM §6.5 «Tieni come nodo a mano», U2 §5.2): il nodo passa al modo decisione e l'identita' resta la stessa
// (emittente, namespace, codice, rev della lettura di prima).
func (c *cantiere) mantieni(i int, vecchio *importazione.Registro, allegato uuid.UUID, rif string) error {
	n := &c.s.Nodi[i]
	if vecchio == nil {
		return fmt.Errorf("il nodo %s in modo proposta senza il registro della versione", n.ChiaveBozza)
	}
	v, ok := vecchio.Contenuto.VocePerRif(allegato, rif)
	var lr importazione.LetturaRegistro
	if !ok || len(v.IdentitaProposta) == 0 || json.Unmarshal(v.IdentitaProposta, &lr) != nil || lr.ChiaveLettura == "" {
		return fmt.Errorf("la voce di %s nel registro vecchio non ha l'identita' proposta", n.ChiaveBozza)
	}
	comp, err := lr.Compatta()
	if err != nil {
		return fmt.Errorf("la lettura di %s non si compatta: %w", n.ChiaveBozza, err)
	}
	id := c.idDecisioneScritta("dm", n.ChiaveBozza, vecchio.Impronta)
	d := Decisione3{ID: id, Tipo: DecisioneIdentita, ChiaveBozza: n.ChiaveBozza, Modo: ModoAlternativa, Importazione: vecchio.Impronta,
		ChiaveLettura: lr.ChiaveLettura, Lettura: &comp, Motivo: c.motivo,
		Identita: &IdentitaScelta{Emittente: n.Identita.Emittente, Namespace: n.Identita.Namespace, Codice: n.Identita.Codice, Rev: n.Identita.Rev}}
	c.s.Decisioni = append(c.s.Decisioni, d)
	n.Identita = Identita{Emittente: n.Identita.Emittente, Namespace: n.Identita.Namespace, Codice: n.Identita.Codice, Rev: n.Identita.Rev,
		Modo: ModoDecisione, Decisione: id}
	c.scritte = append(c.scritte, id)
	return nil
}

// identitaNuovaDellaVoce: l'identita' che una partenza step darebbe a un nodo da creare sulla voce (P-4): proposta con il
// codice proposto, altrimenti da_decidere (il cliente, presunto).
func identitaNuovaDellaVoce(v importazione.Voce, cliente string) (Identita, error) {
	id, ok, err := IdentitaDellaProposta(v)
	if err != nil {
		return Identita{}, err
	}
	if !ok {
		return Identita{Emittente: cliente, EmittentePresunto: true, Modo: ModoDaDecidere}, nil
	}
	return id, nil
}

// identitaDellaVoceFuori: l'identita' proposta di una voce fuori per decisione (U2-P22), come per una voce.
func identitaDellaVoceFuori(x importazione.FuoriPerDecisione, cliente string) (Identita, error) {
	return identitaNuovaDellaVoce(importazione.Voce{Voce: x.Voce, IdentitaProposta: x.IdentitaProposta, CodiceProposto: x.CodiceProposto,
		RevProposta: x.RevProposta}, cliente)
}

// aggiornaIdentita: la regola di un nodo proposta o da_decidere con una casella che resta, verso l'identita' della voce
// nuova: da_decidere la prende da solo; proposta la prende con recepisci, scrive la decisione con mantieni, senza scelta e'
// una scelta che manca. Restituisce un errore solo per un registro incoerente.
func (c *cantiere) aggiornaIdentita(i int, nuova Identita, vecchio *importazione.Registro, allegatoVecchio uuid.UUID, rif string) error {
	n := &c.s.Nodi[i]
	if stessaIdentita3(n.Identita, nuova) {
		return nil
	}
	switch n.Identita.Modo {
	case ModoDaDecidere:
		c.aggiornate = append(c.aggiornate, IdentitaAggiornata3{ChiaveBozza: n.ChiaveBozza, Prima: n.Identita, Dopo: nuova})
		n.Identita = nuova
	case ModoProposta:
		switch c.scelta(n.ChiaveBozza, "") {
		case SceltaRecepisci:
			c.aggiornate = append(c.aggiornate, IdentitaAggiornata3{ChiaveBozza: n.ChiaveBozza, Prima: n.Identita, Dopo: nuova})
			n.Identita = nuova
		case SceltaMantieni:
			return c.mantieni(i, vecchio, allegatoVecchio, rif)
		case "":
			c.manca(n.ChiaveBozza + " (recepisci o mantieni)")
		default:
			c.manca(n.ChiaveBozza + " (la scelta e' recepisci o mantieni)")
		}
	}
	return nil
}

func stessaIdentita3(a, b Identita) bool {
	return a.Modo == b.Modo && a.Emittente == b.Emittente && a.EmittentePresunto == b.EmittentePresunto && a.Namespace == b.Namespace &&
		a.Codice == b.Codice && a.Rev == b.Rev
}

// motivoLettura: lettura_assente se il registro nuovo non ha la lettura della decisione identita (dovunque stia),
// lettura_cambiata se la copia compatta e' diversa, "" se e' la stessa (EM §6.5, §6.6 passo 7).
func motivoLettura(nuovo importazione.Contenuto, d Decisione3) string {
	if d.ChiaveLettura == "" {
		return ""
	}
	l, ok := nuovo.TrovaLettura(d.ChiaveLettura)
	if !ok {
		return RivedereLetturaAssente
	}
	if d.Lettura == nil {
		return ""
	}
	cp, err := l.Compatta()
	if err != nil || !importazione.StessaLettura(cp, *d.Lettura) {
		return RivedereLetturaCambiata
	}
	return ""
}

// elenchi: gli elenchi della risposta in un ordine che non dipende dagli ingressi.
func (c *cantiere) elencoRivedere() []DaRivedere3 {
	out := make([]DaRivedere3, 0, len(c.rivedere))
	for x := range c.rivedere {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ChiaveBozza != b.ChiaveBozza {
			return a.ChiaveBozza < b.ChiaveBozza
		}
		if a.DecisioneID != b.DecisioneID {
			return a.DecisioneID < b.DecisioneID
		}
		return a.Motivo < b.Motivo
	})
	return out
}

func (c *cantiere) chiude() {
	sort.Slice(c.aggiornate, func(i, j int) bool { return c.aggiornate[i].ChiaveBozza < c.aggiornate[j].ChiaveBozza })
	sort.Strings(c.scritte)
	sort.Strings(c.vassoio)
}

// chiaviAggiornate: le chiavi delle identita' aggiornate (per l'evento, che porta le chiavi).
func chiaviAggiornate(xx []IdentitaAggiornata3) []string {
	out := make([]string, 0, len(xx))
	for _, x := range xx {
		out = append(out, x.ChiaveBozza)
	}
	return out
}

// dopoNeiLimiti: il dopo dell'evento del server nei 128 KiB (EM §3.10): gli elenchi piu' lunghi diventano il loro conteggio
// (<elenco>_ridotto); gli elenchi interi si ricalcolano dai dati salvati.
func dopoNeiLimiti(dopo map[string]any, elenchi ...string) {
	for _, k := range elenchi {
		if len(canonico(dopo)) <= MaxPrimaDopo {
			return
		}
		voci := 0
		if b, err := json.Marshal(dopo[k]); err == nil {
			var xx []json.RawMessage
			if json.Unmarshal(b, &xx) == nil {
				voci = len(xx)
			}
		}
		delete(dopo, k)
		dopo[k+"_ridotto"] = voci
	}
}
