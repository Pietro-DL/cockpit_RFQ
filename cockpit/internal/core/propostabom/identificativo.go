package propostabom

import (
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// L'identificativo verso il cliente (V1, emendamento U2 §4.2 punto 4, P-13; prova A-33): una stringa calcolata alla
// lettura, mai salvata, composta da codice e revisione con la regola della grammatica dell'emittente
// (motorea.ComponiIdentificativo sulla forma attiva su cartiglio.codice). Non entra nell'identità decisa né nella
// working; nel registro resta codice_composto come evidenza. Senza regola: «non determinato», con il motivo, e la pagina
// mostra codice e revisione separati. Mai la concatenazione come regola universale ([U] D31 §8).

// IdentificativoCliente: il testo composto, oppure il motivo per cui non c'è (MotivoComposizione*,
// MotivoIdentificativo* di motorea, MotivoIdentificativoGrammaticaAssente).
type IdentificativoCliente struct {
	Testo  string `json:"testo,omitempty"`
	Motivo string `json:"motivo,omitempty"`
}

// MotivoIdentificativoGrammaticaAssente: l'emittente non ha una grammatica attiva nell'insieme delle regole (un cliente
// scartato o senza grammatica, un partner che non c'è, un emittente non scritto).
const MotivoIdentificativoGrammaticaAssente = "grammatica_assente"

// IdentificativoDelCliente: l'identificativo verso il cliente di un'identità (emittente «cliente:<uuid>» o
// «partner:<id>», namespace, codice e revisione), con la grammatica dell'emittente nell'insieme delle regole caricate.
// La stessa funzione per /dati, S9 e, nella persistenza, R1, R4 e R7 (identita[].decisa.identificativo_cliente).
func IdentificativoDelCliente(insieme *motorea.InsiemeRegole, emittente, namespace, codice, rev string) IdentificativoCliente {
	var m *motorea.Motore
	if insieme != nil {
		switch {
		case strings.HasPrefix(emittente, "cliente:"):
			if id, err := uuid.Parse(strings.TrimPrefix(emittente, "cliente:")); err == nil {
				m = insieme.Motori[id]
			}
		case strings.HasPrefix(emittente, "partner:"):
			m = insieme.Emittenti[strings.TrimPrefix(emittente, "partner:")]
		}
	}
	if m == nil {
		return IdentificativoCliente{Motivo: MotivoIdentificativoGrammaticaAssente}
	}
	c := m.ComponiIdentificativo(codice, rev, namespace)
	if c.Testo == "" {
		return IdentificativoCliente{Motivo: c.Motivo}
	}
	return IdentificativoCliente{Testo: c.Testo}
}
