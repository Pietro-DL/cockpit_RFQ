package valutazione

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// I conflitti con le decisioni (contratto §1.7, §2.3, §2.5, §2.6; T-B0-12, T-B0-24, T-E1-15; R95 A). Una decisione
// che una proposta nuova contraddice resta il valore corrente; il conflitto la porta accanto, con l'asse che blocca e
// le evidenze dei due lati. In B5 escono come pezzi, per prodotto: quelli di nomenclatura (dalle discordanze della
// riconciliazione di ancoraggio) e di gerarchia (le quantità decise che la BOM di lavoro contraddice, le rimozioni
// aperte); il conflitto identita_documento arriva con i 2D (fase 2). Li compone B6, nell'esito del thread, insieme a
// quelli dello smistamento.

// TipoConflitto: l'oggetto del conflitto (contratto §2.3, §2.6; T-B0-12, T-B0-29, E1R).
type TipoConflitto string

const (
	ConflittoCodice            TipoConflitto = "codice"
	ConflittoAssociazione      TipoConflitto = "associazione"
	ConflittoArco              TipoConflitto = "arco"
	ConflittoNuovoFile         TipoConflitto = "nuovo_file"
	ConflittoIdentitaDocumento TipoConflitto = "identita_documento"
)

// AsseConflitto: l'asse che il conflitto blocca (contratto §2.5: nomenclatura, gerarchia, smistamento).
type AsseConflitto string

const (
	AsseNomenclatura AsseConflitto = "nomenclatura"
	AsseGerarchia    AsseConflitto = "gerarchia"
	AsseSmistamento  AsseConflitto = "smistamento"
)

// I motivi di un conflitto (Conflitto.Motivo): per un codice la parte contraddetta, codice o revisione (le parole di
// ancoraggio.DiscordanzaDecisione.Parte, come per identita_documento: contratto §2.6); per un arco la quantità decisa
// che la BOM di lavoro contraddice, o la rimozione proposta aperta (T-B0-24; dubbio T-B5-12).
const (
	MotivoConflittoQuantita  = "quantita"
	MotivoConflittoRimozione = "rimozione"
)

// Conflitto: una decisione contraddetta da una proposta nuova, come pezzo per prodotto (contratto §2.3, §2.5, §2.6;
// T-E1-15). I tipi dei campi che il contratto non fissa sono della fase 1 di B5 (dubbio T-B5-12).
//   - Tipo, Asse: l'oggetto e l'asse che il conflitto blocca.
//   - Rif: l'oggetto: il componente deciso (RifComponente) o, per il codice manuale di una riga aperta, il nodo
//     (RifNodo); per un arco la relazione confermata (RifArco fra due RifComponente).
//   - Prodotto: il Rif del prodotto target sulla cui struttura cade.
//   - Decisione, OrigineDecisione: il valore corrente, com'è («codice» o «codice rev»; «qta N» per un arco), con la sua
//     origine (confermato o manuale).
//   - Proposta: il valore proposto, com'è (il testo del campo del cartiglio; «qta N» per un arco; «rimozione»).
//   - Documentale: per un conflitto di nomenclatura, l'esito della riconciliazione del codice documentale che porta la
//     proposta.
//   - Motivo: MotivoConflitto* o la parte di ancoraggio.DiscordanzaDecisione.
//   - EvidenzaDecisione, EvidenzaProposta: le evidenze dei due lati (T-E1-15).
type Conflitto struct {
	Tipo              TipoConflitto                   `json:"tipo"`
	Asse              AsseConflitto                   `json:"asse"`
	Rif               string                          `json:"rif"`
	Prodotto          string                          `json:"prodotto"`
	Decisione         string                          `json:"decisione"`
	OrigineDecisione  ancoraggio.OrigineDato          `json:"origine_decisione"`
	Proposta          string                          `json:"proposta"`
	Documentale       ancoraggio.EsitoRiconciliazione `json:"documentale,omitempty"`
	Motivo            string                          `json:"motivo"`
	EvidenzaDecisione EvidenzaDecisione               `json:"evidenza_decisione"`
	EvidenzaProposta  EvidenzaProposta                `json:"evidenza_proposta"`
}

// EvidenzaDecisione: il lato della decisione (T-E1-15): l'origine, la provenienza della revisione (per una decisione
// tracciata, decisione_tracciata: vale R95 A, non R97 B) e chi e quando, solo se il DB li ha (la decisione tracciata; la
// relazione confermata, con confermato_da e creato_il). Il codice deciso del legacy non ha chi né quando (LD-13).
type EvidenzaDecisione struct {
	Origine        ancoraggio.OrigineDato `json:"origine"`
	RevProvenienza string                 `json:"rev_provenienza,omitempty"`
	Da             *uuid.UUID             `json:"da,omitempty"`
	Il             *time.Time             `json:"il,omitempty"`
}

// EvidenzaProposta: il lato della proposta (T-E1-15): il file, il documento, l'unità e il localizzatore da cui viene,
// la coppia fonte-valore (ancoraggio.EvidenzaDa, T-B4-22) e i riferimenti degli elementi della struttura che la portano
// (il nodo; gli archi dello STEP per una quantità).
type EvidenzaProposta struct {
	AllegatoID  *uuid.UUID               `json:"allegato_id,omitempty"`
	DocumentoID *uuid.UUID               `json:"documento_id,omitempty"`
	UnitaID     string                   `json:"unita_id,omitempty"`
	Posizione   evidenze.Localizzatore   `json:"posizione,omitzero"`
	Evidenza    ancoraggio.EvidenzaVista `json:"evidenza,omitzero"`
	Riferimenti []string                 `json:"riferimenti,omitempty"`
}

// ConflittiDellaNomenclatura: i conflitti di nomenclatura di un prodotto (T-B0-24, R95 A; T-E1-20, T-E1R-08): le
// discordanze con le decisioni dei nodi che la riconciliazione di ancoraggio dà con l'effetto conflitto (un codice
// confermato o manuale che il cartiglio di un 2D associato contraddice; una decisione tracciata contro un'evidenza
// nuova), sulle strutture del prodotto che contano (la BOM di lavoro, se c'è, altrimenti le candidate: R71 A). Gli
// indicatori (l'evidenza già vista, componente.rev del legacy: R97 B; un 2D solo proposto e incerto: T-B4-39) non sono
// conflitti e non bloccano. Lo stesso nodo può stare in più strutture, e la stessa discordanza arriva più volte: i
// doppioni (lo stesso oggetto, lo stesso 2D, la stessa decisione, la stessa parte e la stessa proposta) si tolgono. È
// pura; in ordine di (Rif, Motivo, origine, decisione, proposta, allegato).
func ConflittiDellaNomenclatura(prodotto string, strutture []ancoraggio.StrutturaProdotto) []Conflitto {
	var out []Conflitto
	visti := map[string]bool{}
	for _, s := range struttureDellaVerifica(strutture, prodotto) {
		for _, n := range s.Nodi {
			for _, cd := range n.Codice.Documentale {
				for _, d := range cd.Discordanze {
					if d.Effetto != ancoraggio.EffettoDiscordanzaConflitto {
						continue
					}
					rif := n.Rif
					if d.Decisione.ComponenteID != nil {
						rif = ancoraggio.RifComponente(*d.Decisione.ComponenteID)
					}
					aid := cd.AllegatoID
					c := Conflitto{Tipo: ConflittoCodice, Asse: AsseNomenclatura, Rif: rif, Prodotto: prodotto,
						Decisione: testoDeciso(d.Decisione), OrigineDecisione: d.Decisione.Origine, Proposta: d.Evidenza.Valore,
						Documentale: cd.Esito, Motivo: d.Parte,
						EvidenzaDecisione: EvidenzaDecisione{Origine: d.Decisione.Origine, RevProvenienza: d.Decisione.RevProvenienza,
							Da: copiaUUID(d.DecisaDa), Il: copiaTempo(d.DecisaIl)},
						EvidenzaProposta: EvidenzaProposta{AllegatoID: &aid, DocumentoID: copiaUUID(cd.DocumentoID), UnitaID: cd.UnitaID,
							Posizione: cd.Posizione, Evidenza: d.Evidenza, Riferimenti: []string{n.Rif}}}
					k := strings.Join([]string{c.Rif, aid.String(), string(c.OrigineDecisione), c.Motivo, c.Decisione, c.Proposta}, "\x00")
					if !visti[k] {
						visti[k] = true
						out = append(out, c)
					}
				}
			}
		}
	}
	ordinaConflitti(out)
	return out
}

// conflittiDellaGerarchia: i conflitti di gerarchia di un prodotto (T-B0-24, R95 A):
//   - una quantità decisa che la BOM di lavoro dello STEP confermato contraddice: sulla struttura bom_di_lavoro_proposta,
//     gli archi con l'arco confermato accanto (ArcoProposto.Decisione) si sommano per coppia di componenti (la relazione
//     confermata) sotto lo stesso nodo padre, perché due nodi decisi come lo stesso componente sotto lo stesso padre hanno
//     ognuno la stessa relazione accanto (il limite di ancoraggio.QuantitaDiscorde, che è per arco: R-08 di B4); la
//     somma diversa dalla quantità confermata è il conflitto. Due nodi padre decisi come lo stesso componente sono due
//     esemplari del padre: ognuno porta i suoi figli, e la sua somma si confronta da sola con la relazione, mai la somma
//     dei due (dubbio T-B5-14); le somme discordi uguali della stessa relazione fanno un conflitto solo, con gli archi
//     di tutte. Una quantità che i fatti non danno non è un'informazione: quella coppia non si confronta. Una somma
//     parziale non contraddice (dubbio T-B5-16): sotto un nodo padre con un figlio non ancora deciso (senza la decisione
//     accanto e non scartato da una persona), una somma minore della quantità decisa non si confronta, perché il figlio
//     aperto può essere proprio il pezzo che manca (come la sospensione di fascicolo.AggiornaRimozioni); una somma
//     maggiore resta un conflitto, perché nessun figlio da decidere la può ridurre. Sulle strutture candidate niente:
//     senza la fonte confermata nessuna BOM di lavoro contraddice (R85);
//   - una rimozione proposta aperta sul perimetro del prodotto (rimozione_proposta, lo STEP strutturale che non contiene
//     più l'arco).
//
// Un arco deciso che lo STEP confermato non contiene più, senza una rimozione proposta, non dà un conflitto: il legacy
// lo dice con la rimozione proposta, calcolata con le sue condizioni (lettura completa, figli diretti decisi da una
// persona, profondità 1), e qui non si ricalcola con condizioni diverse (dubbio T-B5-13). In ordine di Rif e motivo.
func (b *bomDelThread) conflittiDellaGerarchia(prodotto string, scelte []ancoraggio.StrutturaProdotto, rimozioni []fotorfq.RimozioneAperta) []Conflitto {
	var out []Conflitto
	for _, s := range scelte {
		if s.Stato != ancoraggio.StatoBOMDiLavoroProposta {
			continue
		}
		// I nodi padre con un figlio non ancora deciso (T-B5-16): deciso vuol dire con la decisione accanto, o scartato da
		// una persona; un figlio deciso come un altro componente non sospende niente (non è un pezzo di quella coppia).
		decisi := map[string]bool{}
		for _, n := range s.Nodi {
			if n.Decisione != nil || (n.RigaDecisa != nil && n.RigaDecisa.Stato == ancoraggio.StatoRigaScartata && n.DecisoDaPersona) {
				decisi[n.Rif] = true
			}
		}
		conFiglioAperto := map[string]bool{}
		for _, a := range s.Archi {
			if !decisi[a.Figlio] {
				conFiglioAperto[a.Padre] = true
			}
		}
		// Le coppie: per nodo padre e relazione confermata, nell'ordine degli archi di ancoraggio.
		type coppia struct {
			padre  string
			decisa ancoraggio.ArcoPercorso
			somma  int
			ignota bool
			archi  []string
		}
		per := map[string]*coppia{}
		var ordine []string
		for _, a := range s.Archi {
			if a.Decisione == nil {
				continue
			}
			k := a.Padre + "\x00" + a.Decisione.Padre + "\x00" + a.Decisione.Figlio
			c := per[k]
			if c == nil {
				c = &coppia{padre: a.Padre, decisa: *a.Decisione}
				per[k] = c
				ordine = append(ordine, k)
			}
			c.archi = append(c.archi, RifArco(a.Padre, a.Figlio))
			if a.Quantita == nil {
				c.ignota = true
			} else {
				c.somma += *a.Quantita
			}
		}
		// I conflitti: uno per relazione e somma discorde, con gli archi di tutti i nodi padre che la danno.
		perConflitto := map[string]int{}
		for _, k := range ordine {
			c := per[k]
			if c.ignota || c.decisa.Quantita == nil || c.somma == *c.decisa.Quantita || (c.somma < *c.decisa.Quantita && conFiglioAperto[c.padre]) {
				continue
			}
			rif := RifArco(c.decisa.Padre, c.decisa.Figlio)
			kc := rif + "\x00" + strconv.Itoa(c.somma)
			if i, ok := perConflitto[kc]; ok {
				out[i].EvidenzaProposta.Riferimenti = ordinatiUnici(append(out[i].EvidenzaProposta.Riferimenti, c.archi...))
				continue
			}
			perConflitto[kc] = len(out)
			aid := s.AllegatoID
			out = append(out, Conflitto{Tipo: ConflittoArco, Asse: AsseGerarchia, Rif: rif, Prodotto: prodotto,
				Decisione: testoQuantita(*c.decisa.Quantita), OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: testoQuantita(c.somma),
				Motivo: MotivoConflittoQuantita, EvidenzaDecisione: b.evidenzaDellaRelazione(c.decisa.Padre, c.decisa.Figlio),
				EvidenzaProposta: EvidenzaProposta{AllegatoID: &aid, Riferimenti: ordinatiUnici(c.archi)}})
		}
	}
	for _, r := range rimozioni {
		padre, figlio := ancoraggio.RifComponente(r.PadreID), ancoraggio.RifComponente(r.FiglioID)
		decisione := ""
		if rel, ok := b.relazioni[r.PadreID.String()+"\x00"+r.FiglioID.String()]; ok {
			decisione = testoQuantita(rel.Qta)
		}
		doc := r.StepDocumentoID
		out = append(out, Conflitto{Tipo: ConflittoArco, Asse: AsseGerarchia, Rif: RifArco(padre, figlio), Prodotto: prodotto,
			Decisione: decisione, OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: MotivoConflittoRimozione, Motivo: MotivoConflittoRimozione,
			EvidenzaDecisione: b.evidenzaDellaRelazione(padre, figlio), EvidenzaProposta: EvidenzaProposta{DocumentoID: &doc}})
	}
	ordinaConflitti(out)
	return out
}

// evidenzaDellaRelazione: il lato della decisione di una relazione confermata, con chi e quando (confermato_da,
// creato_il della relazione: il DB li ha), se la relazione c'è.
func (b *bomDelThread) evidenzaDellaRelazione(padre, figlio string) EvidenzaDecisione {
	e := EvidenzaDecisione{Origine: ancoraggio.OrigineConfermato}
	p, err1 := uuid.Parse(strings.TrimPrefix(padre, rifComponente))
	f, err2 := uuid.Parse(strings.TrimPrefix(figlio, rifComponente))
	if err1 != nil || err2 != nil {
		return e
	}
	if rel, ok := b.relazioni[p.String()+"\x00"+f.String()]; ok {
		da, il := rel.ConfermatoDa, rel.CreatoIl.UTC().Truncate(time.Millisecond)
		e.Da, e.Il = &da, &il
	}
	return e
}

// rifDeiConflitti: i Rif dei conflitti di un asse, in ordine, senza doppioni.
func rifDeiConflitti(conflitti []Conflitto, asse AsseConflitto) []string {
	var out []string
	for _, c := range conflitti {
		if c.Asse == asse {
			out = append(out, c.Rif)
		}
	}
	return ordinatiUnici(out)
}

// ordinaConflitti: l'ordine canonico dei conflitti di un prodotto: (asse, Rif, motivo, origine, decisione, proposta) e
// poi l'evidenza della proposta (allegato, documento, unità), così due conflitti che differiscono solo per il documento
// (due rimozioni aperte dello stesso arco da due STEP) non dipendono dall'ordine degli elenchi della fotografia.
func ordinaConflitti(c []Conflitto) {
	sort.SliceStable(c, func(i, j int) bool { return chiaveOrdineConflitto(c[i]) < chiaveOrdineConflitto(c[j]) })
}

// chiaveOrdineConflitto: la chiave dell'ordine canonico di ordinaConflitti.
func chiaveOrdineConflitto(x Conflitto) string {
	return strings.Join([]string{string(x.Asse), x.Rif, x.Motivo, string(x.OrigineDecisione), x.Decisione, x.Proposta,
		testoUUID(x.EvidenzaProposta.AllegatoID), testoUUID(x.EvidenzaProposta.DocumentoID), x.EvidenzaProposta.UnitaID}, "\x00")
}

// componiConflitti: i conflitti dell'esito del thread (EsitoThread.Conflitti; T-B6-10, F0-13), dai pezzi per prodotto: in
// V1 quelli di nomenclatura e di gerarchia di B5 (ValutazioneProdotti.Conflitti), da V2 anche quelli dell'asse
// smistamento. Ogni conflitto porta il suo prodotto: lo stesso conflitto su un componente condiviso da due prodotti resta
// due voci, una per prodotto (F0-13). Senza doppioni, con la chiave di F0-13 (tipo, asse, Rif, prodotto, motivo, origine
// e valore della decisione, proposta, allegato dell'evidenza della proposta), allargata al documento e all'unità
// dell'evidenza della proposta: due rimozioni aperte dello stesso arco da due STEP sono due conflitti, non un doppione
// (ordinaConflitti le distingue già; dubbio T-B6-23). Dei doppioni resta il primo, nell'ordine dei pezzi. Ordine:
// ordinaConflitti, poi il prodotto e il tipo, così l'esito non dipende dall'ordine dei pezzi. È pura.
func componiConflitti(pezzi ...[]Conflitto) []Conflitto {
	var out []Conflitto
	visti := map[string]bool{}
	for _, p := range pezzi {
		for _, c := range p {
			k := strings.Join([]string{string(c.Tipo), string(c.Asse), c.Rif, c.Prodotto, c.Motivo, string(c.OrigineDecisione), c.Decisione,
				c.Proposta, testoUUID(c.EvidenzaProposta.AllegatoID), testoUUID(c.EvidenzaProposta.DocumentoID), c.EvidenzaProposta.UnitaID}, "\x00")
			if !visti[k] {
				visti[k] = true
				out = append(out, c)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ka, kb := chiaveOrdineConflitto(a), chiaveOrdineConflitto(b); ka != kb {
			return ka < kb
		}
		if a.Prodotto != b.Prodotto {
			return a.Prodotto < b.Prodotto
		}
		return a.Tipo < b.Tipo
	})
	return out
}

// testoDeciso: un codice deciso come si mostra, «codice» o «codice rev».
func testoDeciso(d ancoraggio.CodiceDeciso) string {
	if d.Rev != nil && strings.TrimSpace(*d.Rev) != "" {
		return d.Codice + " " + strings.TrimSpace(*d.Rev)
	}
	return d.Codice
}

// testoQuantita: una quantità come si mostra, «qta N».
func testoQuantita(q int) string { return "qta " + strconv.Itoa(q) }

func copiaTempo(p *time.Time) *time.Time {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
