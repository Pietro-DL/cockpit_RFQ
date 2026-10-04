package motorea

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// InsiemeRegole: i motori di tutti i clienti dell'indice. Un cliente la cui grammatica non è valida non ha un
// motore: resta la sua diagnosi in Scartati. Le mappe servono a cercare per UUID, mai a scorrere in un ordine.
type InsiemeRegole struct {
	Indice         grammatica.IndiceRegole // con i limiti e la loro versione (Indice.Limiti)
	ImprontaIndice string                  // sha256 del canonico dell'indice: copre anche i limiti
	Motori         map[uuid.UUID]*Motore
	Scartati       map[uuid.UUID][]evidenze.Diagnostica
}

// CompilaInsieme compila le grammatiche dell'indice, ciascuna con NuovoSnapshot e CompilaVerificato e con i
// limiti dell'indice stesso: non c'è un parametro di limiti, perché l'unica fonte è l'indice (R43 B). I
// contenuti sono i byte dei file, per nome come l'indice lo scrive, letti da chi chiama: questo pacchetto
// non tocca il disco. Un cliente con il file assente, uno sha256 diverso dall'indice, un cliente.id diverso
// dalla voce o una grammatica non valida è scartato; gli altri restano attivi (par.3.6.3-3.6.5).
//
// Le diagnostiche seguono l'ordine delle voci dell'indice: per ogni cliente scartato il motivo
// (regole.file_assente, regole.sha256_discorde, regole.cliente_discorde, regole.non_valide con le
// diagnostiche della grammatica), per ogni cliente attivo gli avvisi e le note della sua compilazione.
// L'errore è solo per un indice con cui nessuna grammatica si attiva: limiti assenti, nulli o oltre i tetti,
// un cliente ripetuto (lo controlla già LeggiIndice; qui si ricontrolla perché l'indice può arrivare
// costruito a mano).
func CompilaInsieme(ind grammatica.IndiceRegole, contenuti map[string][]byte) (InsiemeRegole, []evidenze.Diagnostica, error) {
	r := InsiemeRegole{
		Indice:   ind,
		Motori:   map[uuid.UUID]*Motore{},
		Scartati: map[uuid.UUID][]evidenze.Diagnostica{},
	}
	if d := indiceNonValido(ind); len(d) > 0 {
		return r, d, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	impronta, err := jsoncanonico.ImprontaDi(ind)
	if err != nil {
		d := []evidenze.Diagnostica{{
			Codice:    grammatica.CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "forma canonica dell'indice: " + err.Error(),
		}}
		return r, d, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	r.ImprontaIndice = impronta

	var tutte []evidenze.Diagnostica
	for i, v := range ind.Grammatiche {
		m, d := compilaVoce(fmt.Sprintf("grammatiche[%d]", i), v, ind.Limiti, contenuti)
		tutte = append(tutte, d...)
		if m != nil {
			r.Motori[v.ClienteID] = m
		} else {
			r.Scartati[v.ClienteID] = d
		}
	}
	return r, tutte, nil
}

// indiceNonValido: i limiti (Limiti.Valida) e i clienti ripetuti, i due casi in cui nessuna grammatica si
// attiva.
func indiceNonValido(ind grammatica.IndiceRegole) []evidenze.Diagnostica {
	d := ind.Limiti.Valida()
	var visti []uuid.UUID
	for i, v := range ind.Grammatiche {
		for _, c := range visti {
			if c == v.ClienteID {
				d = append(d, evidenze.Diagnostica{
					Codice:    grammatica.CodiceRegoleClienteRipetuto,
					Gravita:   evidenze.GravitaErrore,
					Natura:    evidenze.NaturaContratto,
					Percorso:  fmt.Sprintf("grammatiche[%d].cliente_id", i),
					Messaggio: "il cliente compare due volte nell'indice: quale grammatica valga non si decide",
					Rif:       []string{v.ClienteID.String()},
				})
				break
			}
		}
		visti = append(visti, v.ClienteID)
	}
	return d
}

// compilaVoce: il motore di un cliente dell'indice, o nil con il motivo dello scarto.
func compilaVoce(p string, v grammatica.VoceIndice, lim grammatica.Limiti, contenuti map[string][]byte) (*Motore, []evidenze.Diagnostica) {
	cliente := v.ClienteID.String()
	raw, ok := contenuti[v.File]
	if !ok {
		return nil, []evidenze.Diagnostica{{
			Codice:    CodiceRegoleFileAssente,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p + ".file",
			Messaggio: "il file delle regole indicato dall'indice non c'è: il cliente è scartato, gli altri restano attivi",
			Rif:       []string{cliente},
		}}
	}
	// Lo sha256 in esadecimale: le maiuscole delle cifre esadecimali non cambiano il valore.
	if !strings.EqualFold(jsoncanonico.Impronta(raw), v.Sha256) {
		return nil, []evidenze.Diagnostica{{
			Codice:    CodiceRegoleSha256Discorde,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p + ".sha256",
			Messaggio: "lo sha256 del file delle regole è diverso da quello dell'indice: il cliente è scartato (R29 d)",
			Rif:       []string{cliente},
		}}
	}
	s, err := grammatica.NuovoSnapshot(raw, lim)
	if err != nil {
		return nil, scartata(p, cliente, err, nil)
	}
	if s.ClienteID != v.ClienteID {
		return nil, []evidenze.Diagnostica{{
			Codice:    CodiceRegoleClienteDiscorde,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p + ".cliente_id",
			Messaggio: "il cliente.id del file delle regole è diverso da quello della voce dell'indice: il cliente è scartato (par.3.6.3)",
			Rif:       []string{cliente, s.ClienteID.String()},
		}}
	}
	m, d, err := CompilaVerificato(s, lim)
	if err != nil {
		return nil, scartata(p, cliente, err, d)
	}
	return m, d
}

// scartata: regole.non_valide, seguita dalle diagnostiche della grammatica (quelle dell'errore di contratto,
// o quelle ricevute).
func scartata(p, cliente string, err error, d []evidenze.Diagnostica) []evidenze.Diagnostica {
	out := []evidenze.Diagnostica{{
		Codice:    CodiceRegoleNonValide,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Percorso:  p,
		Messaggio: "la grammatica del cliente non è valida: il cliente è scartato, gli altri restano attivi",
		Rif:       []string{cliente},
	}}
	if d == nil {
		var ec *evidenze.ErroreContratto
		if errors.As(err, &ec) {
			d = ec.Diagnostiche
		}
	}
	return append(out, d...)
}

// MotoreDi dà il motore del cliente, cercato per UUID e poi controllato sulla ragione sociale (par.3.6). Nil
// vuol dire nessuna grammatica A valida: il thread non si valuta, e la diagnosi lo dice.
//   - Un cliente senza voce nell'indice: regole.assenti (nota, dati).
//   - Un cliente scartato: le diagnostiche dello scarto.
//   - Una ragione sociale diversa da quella della grammatica (grammatica.ControllaRagioneSociale):
//     regole.ragione_sociale_discorde (avviso).
func (r InsiemeRegole) MotoreDi(cliente uuid.UUID, ragioneSociale string) (*Motore, []evidenze.Diagnostica) {
	m := r.Motori[cliente]
	if m == nil {
		if d, scartato := r.Scartati[cliente]; scartato {
			return nil, append([]evidenze.Diagnostica(nil), d...)
		}
		return nil, []evidenze.Diagnostica{{
			Codice:    CodiceRegoleAssenti,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Messaggio: "il cliente non ha una voce nell'indice delle regole: non si valuta, nessuna regola A",
			Rif:       []string{cliente.String()},
		}}
	}
	if d := grammatica.ControllaRagioneSociale(m.snap.Grammatica, ragioneSociale); d != nil {
		return nil, []evidenze.Diagnostica{*d}
	}
	return m, nil
}
