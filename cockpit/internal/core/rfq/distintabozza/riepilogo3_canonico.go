package distintabozza

// riepilogo3_canonico.go: i blocchi del codice canonico del cliente in R4 formato 3 (EM §5.2 W5, §8.1 righe 11, 12 e 15; C3
// §4.4 n. 11, 12, 15), nella parte pura del riepilogo: CANONICO_COLLIDE, CANONICO_AMBIGUO, la parte «canonico» di
// RICODIFICA_COLLIDE e il riuso per canonico che non torna. Il canonico e' quello delle regole del cliente della RFQ
// (classificazione.Motore.Canonico: il suffisso decorativo tolto, come lo confronta la pagina di prima), letto da R4 con
// fascicolo.MotoreDellaRfq nella stessa transazione e quindi anche nella S di R5.
//
// Le regole [T-39 della corsia persistenza]:
//   - W5 vale solo fra identita' del cliente (EM §5.2): il nodo con l'emittente del cliente e il componente registrato con
//     l'identita' registrata del cliente (presunta o certificata). Un partner non ha il canonico del cliente;
//   - un cliente senza suffissi decorativi ha il canonico uguale al codice in maiuscolo: W1-W4 coprono gia' tutto e qui non
//     nasce niente (la regola del Fascicolo: «per chi non li dichiara non cambia niente»);
//   - CANONICO_COLLIDE: un nodo da creare il cui codice ha lo stesso canonico di un solo componente registrato con un
//     codice diverso (in maiuscolo), senza riusa_per_canonico verso quel componente; il rimedio e' riusa_per_canonico o un
//     altro codice. Gli archiviati contano: occupano la loro chiave come gli altri (M18:280);
//   - CANONICO_AMBIGUO: lo stesso caso con due o piu' componenti registrati diversi con quel canonico (anche con un
//     riusa_per_canonico che ne sceglie uno); il rimedio del C3 e' ricodificare o archiviare prima. «Toccati dalla bozza»
//     vuol dire toccati da una risoluzione per canonico (un nodo da creare, un riusa_per_canonico): due registrati con lo
//     stesso canonico che la bozza porta per UUID restano due componenti, come vuole Q04 [U] (due codifiche dello stesso
//     oggetto non si fondono), e non bloccano;
//   - un riusa_per_canonico verso un componente il cui canonico non e' quello del nodo e' RIUSO_INCOMPATIBILE con la
//     differenza «canonico»;
//   - RICODIFICA_COLLIDE anche per canonico: la ricodifica verso un codice che ha il canonico di un altro componente del
//     cliente, con un codice diverso (motivo «canonico»; il caso dello stesso codice resta quello di prima).

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/confermabom"
)

// MotivoCanonico: il motivo di RICODIFICA_COLLIDE e di RIUSO_INCOMPATIBILE quando la collisione e' solo per il canonico.
const MotivoCanonico = "canonico"

// canonicoDi: il canonico del cliente di un codice (le regole della RFQ), in maiuscolo; "" per un codice vuoto.
func (a *analisiR4) canonicoDi(codice string) string {
	c := strings.TrimSpace(codice)
	if c == "" {
		return ""
	}
	if a.in.motore != nil {
		c, _ = a.in.motore.Canonico(c, "")
	}
	return strings.ToUpper(strings.TrimSpace(c))
}

// delCliente: il componente registrato ha l'identita' registrata del cliente.
func (a *analisiR4) delCliente(c confermabom.Componente) bool {
	return IdentitaRegistrataDi(c, a.in.cert, a.in.cliente).Emittente == a.in.cliente
}

// canonici: CANONICO_COLLIDE, CANONICO_AMBIGUO, il riuso per canonico che non torna, RICODIFICA_COLLIDE per canonico.
func (a *analisiR4) canonici() {
	if !a.in.motore.HaSuffissi() {
		return
	}
	perCanonico := map[string][]confermabom.Componente{}
	for _, c := range a.in.stato.Componenti {
		if !a.delCliente(c) {
			continue
		}
		k := a.canonicoDi(c.CodiceLetterale)
		perCanonico[k] = append(perCanonico[k], c)
	}
	for k := range perCanonico {
		sort.Slice(perCanonico[k], func(i, j int) bool { return perCanonico[k][i].ID.String() < perCanonico[k][j].ID.String() })
	}
	perCanonicoRiuso := map[string]uuid.UUID{}
	for _, d := range a.dec[DecisioneRiusaPerCanonico] {
		perCanonicoRiuso[d.ChiaveBozza] = *d.ComponenteID
	}
	for _, k := range a.chiavi {
		n := a.nodi[k]
		if !a.incluso(k) || n.ComponenteID != nil || n.Op != OpCrea || n.Identita.Emittente != a.in.cliente {
			continue
		}
		w := strings.ToUpper(strings.TrimSpace(n.Identita.Codice))
		if w == "" {
			continue
		}
		ck := a.canonicoDi(n.Identita.Codice)
		var altri []confermabom.Componente
		for _, c := range perCanonico[ck] {
			if strings.ToUpper(c.CodiceLetterale) != w {
				altri = append(altri, c)
			}
		}
		scelto, conRiuso := perCanonicoRiuso[k]
		switch {
		case len(altri) >= 2:
			parti := []ParteIdentita{parteNodo(k, n)}
			for _, c := range altri {
				parti = append(parti, parteRegistrata(c, IdentitaRegistrataDi(c, a.in.cert, a.in.cliente)))
			}
			a.blocca(Rilievo{Codice: BloccoCanonicoAmbiguo, ChiaveBozza: k, ChiaveWorking: ck, Parti: parti,
				Testo:  "Il codice " + n.Identita.Codice + " ha lo stesso canonico del cliente (" + ck + ") di più componenti registrati: la conferma non sa a quale corrisponde.",
				Rimedi: []string{"ricodifica o archivia prima i componenti registrati con lo stesso canonico", "cambia il codice del nodo, con il motivo"}})
		case len(altri) == 1 && (!conRiuso || scelto != altri[0].ID):
			c := altri[0]
			a.blocca(Rilievo{Codice: BloccoCanonicoCollide, ChiaveBozza: k, ComponenteID: uid(c.ID), ChiaveWorking: ck,
				Parti: []ParteIdentita{parteNodo(k, n), parteRegistrata(c, IdentitaRegistrataDi(c, a.in.cert, a.in.cliente))},
				Testo: "Il codice " + n.Identita.Codice + " è lo stesso pezzo di " + c.CodiceLetterale + " per le regole del cliente (canonico " + ck +
					"): riusalo, oppure cambia il codice.",
				Rimedi: []string{"«Riusa per canonico» il componente registrato (riusa_per_canonico)", "cambia il codice del nodo, con il motivo"}})
		}
	}
	// riusa_per_canonico verso un componente con un altro canonico
	for _, d := range a.dec[DecisioneRiusaPerCanonico] {
		n, ok := a.nodi[d.ChiaveBozza]
		if !ok || !a.incluso(d.ChiaveBozza) {
			continue
		}
		c, ok := a.in.stato.Componente(*d.ComponenteID)
		if !ok || a.canonicoDi(n.Identita.Codice) == a.canonicoDi(c.CodiceLetterale) {
			continue
		}
		a.blocca(Rilievo{Codice: BloccoRiusoIncompatibile, Motivo: MotivoCanonico, ChiaveBozza: d.ChiaveBozza, ComponenteID: uid(c.ID),
			Differenze: []string{MotivoCanonico},
			Testo:      "Il riuso per canonico di " + c.CodiceLetterale + " non torna: il codice del nodo (" + n.Identita.Codice + ") ha un altro canonico del cliente."})
	}
	// RICODIFICA_COLLIDE per canonico
	for _, d := range a.dec[DecisioneRicodifica] {
		w := strings.ToUpper(strings.TrimSpace(d.CodiceDopo))
		ck := a.canonicoDi(d.CodiceDopo)
		for _, c := range perCanonico[ck] {
			if c.ID == *d.ComponenteID || strings.ToUpper(c.CodiceLetterale) == w {
				continue // lo stesso codice e' la regola di prima (decisioniConIlDatabase)
			}
			a.blocca(Rilievo{Codice: BloccoRicodificaCollide, Motivo: MotivoCanonico, ComponenteID: d.ComponenteID, ChiaveWorking: ck,
				Testo: "La ricodifica di " + a.codiceDi(*d.ComponenteID) + " in " + d.CodiceDopo + " va sul canonico del cliente di " + c.CodiceLetterale +
					", un altro componente della RFQ."})
		}
	}
}
