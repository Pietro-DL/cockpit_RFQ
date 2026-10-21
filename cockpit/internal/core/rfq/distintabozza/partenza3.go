package distintabozza

// I costruttori della versione 1 di una bozza formato 3 (EM §9.2 «Versione 1 sempre dal server»; §6.1, §6.12; direttiva
// 35, blocco C-3): puri, senza database, sui dati che chi chiama ha letto nella transazione di R2.
//   - vuota: il solo nodo del prodotto, registrato;
//   - registrato: il perimetro della working del prodotto (il prodotto e cio' che gli sta sotto, con i suoi archi), tutto
//     in modo registrato;
//   - da-conferma-1: nodi, archi e caselle dell'ultima conferma valida del prodotto, in modo registrato, con le sole
//     decisioni mono_pezzo e conferma_categoria (riportate con da_conferma), la sorgente della conferma senza registro;
//   - importazione-1 (P-1...P-8): partenzastep3.go.
//
// Le identita' dei nodi registrati sono l'identita' registrata del §5.1 (IdentitaRegistrataDi): la certificazione formato 3
// valida, altrimenti il cliente presunto senza namespace; il codice e' quello di oggi, la rev quella della working. Nessuna
// preassociazione nasce in una versione 1 (P-7; [U] dir. 35 §8 C-3: nessuna PA diventa PB senza il gesto).
//
// Le chiavi dei nodi che il server inventa [T]: «k-c-<componente_id>» per un componente della working (vuota, registrato,
// un nodo della conferma che la conferma non nomina) e «k-v-<24 cifre esadecimali>» per una voce del registro (lo sha256 di
// allegato e rif): deterministiche, nello spazio di reChiave, e diverse dalle chiavi della pagina («k-u…»). da-conferma-1
// tiene le chiavi della conferma: lo stesso nodo ha la stessa chiave da una revisione all'altra.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

// ChiaveDelComponente: la chiave che il server da' al nodo di un componente della working (commento di testa).
func ChiaveDelComponente(c uuid.UUID) string { return "k-c-" + c.String() }

// ChiaveDellaVoce: la chiave che il server da' al nodo di una voce del registro, dall'occorrenza e dal rif (commento di
// testa).
func ChiaveDellaVoce(allegato uuid.UUID, rif string) string {
	h := sha256.Sum256([]byte(allegato.String() + "\x00" + rif))
	return "k-v-" + hex.EncodeToString(h[:12])
}

// Certificazione: l'identita' certificata di un componente dall'ultima conferma formato 3 che lo contiene (EM §5.1), con
// la sua validita' (codice di oggi, componente non archiviato).
type Certificazione struct {
	Emittente         string
	EmittentePresunto bool
	Namespace         string
	Valida            bool
}

// CertificazioniDa: le righe di ListIdentitaRegistrate per componente.
func CertificazioniDa(rr []db.ListIdentitaRegistrateRow) map[uuid.UUID]Certificazione {
	out := make(map[uuid.UUID]Certificazione, len(rr))
	for _, r := range rr {
		if !r.Emittente.Valid {
			continue
		}
		out[r.ComponenteID] = Certificazione{Emittente: r.Emittente.String, EmittentePresunto: r.EmittentePresunto.Bool,
			Namespace: r.Namespace.String, Valida: r.Valida}
	}
	return out
}

// IdentitaRegistrataDi: l'identita' registrata di un componente per un nodo in modo registrato (EM §3.4.2, §5.1): quella
// della certificazione formato 3 se vale ancora, altrimenti il cliente presunto senza namespace; il codice e' sempre quello
// di oggi, la rev quella della working. E' la regola che R2 verifica sui nodi registrati (verifica3.identitaRegistrata).
func IdentitaRegistrataDi(c confermabom.Componente, cert map[uuid.UUID]Certificazione, cliente string) Identita {
	id := Identita{Emittente: cliente, EmittentePresunto: true, Codice: c.CodiceLetterale, Rev: c.Rev, Modo: ModoRegistrato}
	if x, ok := cert[c.ID]; ok && x.Valida && x.Emittente != "" {
		id.Emittente, id.EmittentePresunto, id.Namespace = x.Emittente, x.EmittentePresunto, x.Namespace
	}
	return id
}

// TracciaPartenza: quello che un costruttore della versione 1 non ha potuto copiare com'era, per l'evento bozza_aperta e la
// risposta (mai una troncatura in silenzio): le descrizioni oltre i 200 caratteri (il nodo nasce senza), le posizioni
// oltre i 20 (l'arco nasce senza), i nodi della conferma che non sono piu' nella working.
type TracciaPartenza struct {
	DescrizioniOltreLimite []string `json:"descrizioni_oltre_limite,omitempty"`
	PosizioniOltreLimite   []string `json:"posizioni_oltre_limite,omitempty"`
	NodiFuoriWorking       []string `json:"nodi_fuori_working,omitempty"`
}

func (t *TracciaPartenza) ordina() {
	sort.Strings(t.DescrizioniOltreLimite)
	sort.Strings(t.PosizioniOltreLimite)
	sort.Strings(t.NodiFuoriWorking)
}

// IngressoPartenza: cio' che i costruttori della versione 1 leggono, gia' letto da chi chiama sotto il lucchetto della RFQ.
type IngressoPartenza struct {
	Prodotto       uuid.UUID
	Stato          *confermabom.StatoRFQ
	Certificazioni map[uuid.UUID]Certificazione
	Cliente        string // l'emittente del cliente della RFQ (cliente:<uuid>)
}

// base: la versione corrente del prodotto come base della bozza (EM §6.4): l'ultima conferma, il contatore, la firma.
func (in IngressoPartenza) base() (VersioneBomBase, string, error) {
	vc, err := in.Stato.VersioneCorrente(in.Prodotto)
	if err != nil {
		return VersioneBomBase{}, "", err
	}
	return VersioneBomBase{ConfermaID: vc.ConfermaID, Numero: int(vc.Numero), Contatore: vc.Contatore}, vc.FirmaStruttura, nil
}

// nodoRegistrato: il nodo di un componente della working in modo registrato (riusa, con l'identita' registrata, tipo e
// descrizione della working; una descrizione oltre il limite non si tronca: il nodo nasce senza, con la traccia).
func (in IngressoPartenza) nodoRegistrato(c confermabom.Componente, chiave, origine string, tr *TracciaPartenza) Nodo3 {
	id := c.ID
	n := Nodo3{ChiaveBozza: chiave, ComponenteID: &id, Op: OpRiusa, Tipo: c.Tipo, Origine: origine,
		Identita: IdentitaRegistrataDi(c, in.Certificazioni, in.Cliente)}
	if d := nfc(c.Descrizione); runeOltre(d, MaxDescrizione) {
		tr.DescrizioniOltreLimite = append(tr.DescrizioniOltreLimite, chiave)
	} else {
		n.Descrizione = d
	}
	return n
}

// PartenzaRegistrata: la versione 1 delle partenze vuota (il solo prodotto) e registrato (il perimetro della working del
// prodotto con i suoi archi), con la base corrente.
func PartenzaRegistrata(partenza string, in IngressoPartenza) (Snapshot3, TracciaPartenza, error) {
	var tr TracciaPartenza
	base, firma, err := in.base()
	if err != nil {
		return Snapshot3{}, tr, err
	}
	s := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: in.Prodotto, VersioneBomBase: &base, FirmaBase: firma, Nodi: []Nodo3{}, Archi: []Arco3{},
		Decisioni: []Decisione3{}, DocumentiPreassociati: []Preassociazione3{}}
	if partenza == PartenzaVuota {
		c, ok := in.Stato.Componente(in.Prodotto)
		if !ok {
			return Snapshot3{}, tr, fuori("prodotto_id", "il prodotto %s non e' della RFQ", in.Prodotto)
		}
		s.Nodi = append(s.Nodi, in.nodoRegistrato(c, ChiaveDelComponente(c.ID), OrigineRegistrato, &tr))
		tr.ordina()
		return s, tr, nil
	}
	per, err := in.Stato.Perimetro(in.Prodotto)
	if err != nil {
		return Snapshot3{}, tr, err
	}
	for _, c := range per.Componenti {
		s.Nodi = append(s.Nodi, in.nodoRegistrato(c, ChiaveDelComponente(c.ID), OrigineRegistrato, &tr))
	}
	for _, a := range per.Archi {
		x := Arco3{Padre: ChiaveDelComponente(a.Padre), Figlio: ChiaveDelComponente(a.Figlio), Qta: int(a.Qta), Origine: OrigineRegistrato}
		if p := nfc(a.Posizione); runeOltre(p, MaxPosizione) {
			tr.PosizioniOltreLimite = append(tr.PosizioniOltreLimite, x.Padre+">"+x.Figlio)
		} else {
			x.Posizione = p
		}
		s.Archi = append(s.Archi, x)
	}
	tr.ordina()
	return s, tr, nil
}

// ConfermaDiPartenza: l'ultima conferma valida del prodotto, come la legge da-conferma-1: id e numero, lo snapshot gia'
// nel formato 3 (per una conferma formato 2 convertito con le regole della conversione; per una formato 1 nil: si parte
// come registrato), la mappa chiave della bozza → componente, la fonte della conferma.
type ConfermaDiPartenza struct {
	ConfermaID uuid.UUID
	Numero     int
	Formato    int
	Snapshot   *Snapshot3
	Mappa      map[string]uuid.UUID
	Sorgente   *Sorgente3
	// ImportazioneID: il registro della conferma formato 3 (le PB iniziali di da-conferma-1, direttiva 39 §4)
	ImportazioneID uuid.NullUUID
}

// DaConfermaR2: la conferma da cui parte una bozza da-conferma-1 (l'evento bozza_aperta e la risposta).
type DaConfermaR2 struct {
	ConfermaID uuid.UUID `json:"conferma_id"`
	Numero     int       `json:"numero"`
}

// PartenzaDaConferma: la versione 1 di da-conferma-1 (EM §6.12, U2 §5.3, §6.4; DEC-B PD-01):
//  1. nodi, archi e caselle dello snapshot della conferma; un nodo escluso (op esclude) o il cui componente non e' piu'
//     nella working non entra (traccia nodi_fuori_working), e gli archi entrano solo fra nodi che entrano;
//  2. ogni nodo e' il componente della mappa della conferma, in modo registrato (riusa, identita' registrata, tipo e
//     descrizione della working), con la chiave e le caselle della conferma; origine registrato per il prodotto, step per
//     un nodo con le caselle, conferma per gli altri [T];
//  3. fra le decisioni solo mono_pezzo (com'e') e conferma_categoria (su un nodo che entra), questa con da_conferma
//     {conferma, numero, decisione}, se non ce l'ha gia' (una decisione riportata resta della conferma che l'ha
//     introdotta);
//  4. la sorgente della conferma, senza registro; nessuna preassociazione.
//
// Una conferma formato 1 (senza nodi) parte come registrato (EM §6.12, ultimo paragrafo).
func PartenzaDaConferma(c ConfermaDiPartenza, in IngressoPartenza) (Snapshot3, TracciaPartenza, error) {
	if c.Snapshot == nil {
		return PartenzaRegistrata(PartenzaRegistrato, in)
	}
	var tr TracciaPartenza
	base, firma, err := in.base()
	if err != nil {
		return Snapshot3{}, tr, err
	}
	per, err := in.Stato.Perimetro(in.Prodotto)
	if err != nil {
		return Snapshot3{}, tr, err
	}
	nelPerimetro := map[uuid.UUID]confermabom.Componente{}
	for _, x := range per.Componenti {
		nelPerimetro[x.ID] = x
	}
	s := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: in.Prodotto, VersioneBomBase: &base, FirmaBase: firma, Nodi: []Nodo3{}, Archi: []Arco3{},
		Decisioni: []Decisione3{}, DocumentiPreassociati: []Preassociazione3{}}
	if c.Sorgente != nil {
		x := *c.Sorgente
		x.Importazione = ""
		s.Sorgente = &x
	}
	dentro := map[string]bool{}
	for _, n := range c.Snapshot.Nodi {
		comp := c.Mappa[n.ChiaveBozza]
		if n.ComponenteID != nil {
			comp = *n.ComponenteID
		}
		w, ok := nelPerimetro[comp]
		if n.Op == OpEsclude || comp == uuid.Nil || !ok {
			tr.NodiFuoriWorking = append(tr.NodiFuoriWorking, n.ChiaveBozza)
			continue
		}
		origine := OrigineConferma
		switch {
		case comp == in.Prodotto:
			origine = OrigineRegistrato
		case len(n.Caselle) > 0:
			origine = OrigineStep
		}
		x := in.nodoRegistrato(w, n.ChiaveBozza, origine, &tr)
		if s.Sorgente != nil {
			x.Caselle = append([]Casella3(nil), n.Caselle...)
		}
		s.Nodi = append(s.Nodi, x)
		dentro[n.ChiaveBozza] = true
	}
	for _, a := range c.Snapshot.Archi {
		if !dentro[a.Padre] || !dentro[a.Figlio] {
			continue
		}
		x := a
		x.EvidenzeArco = append([]EvidenzaDocumentale(nil), a.EvidenzeArco...)
		if x.Step != nil {
			st := *x.Step
			x.Step = &st
		}
		if x.Origine == OrigineStep && (s.Sorgente == nil || x.Step == nil) {
			x.Origine, x.Step = OrigineConferma, nil
		}
		s.Archi = append(s.Archi, x)
	}
	da := &DaConferma{ConfermaID: c.ConfermaID, Numero: c.Numero}
	for _, d := range c.Snapshot.Decisioni {
		switch d.Tipo {
		case DecisioneMonoPezzo:
		case DecisioneConfermaCategoria:
			if d.Soggetto == nil || !dentro[d.Soggetto.ChiaveBozza] {
				continue
			}
		default:
			continue
		}
		x := d
		// mono_pezzo ha la forma chiusa {motivo}: passa com'e' (scelta [T]: da_conferma e' un campo della sola
		// conferma_categoria, EM §3.6.3)
		if x.Tipo == DecisioneConfermaCategoria && x.DaConferma == nil {
			y := *da
			y.DecisioneID = d.ID
			x.DaConferma = &y
		}
		s.Decisioni = append(s.Decisioni, x)
	}
	tr.ordina()
	return s, tr, nil
}
