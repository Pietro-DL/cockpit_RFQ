package bancoa

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/dataset"
)

// RapportoRegole: l'esito della modalità «regole» («profili attivi validi», A1a-P1).
type RapportoRegole struct {
	ImprontaIndice  string                 `json:"impronta_indice,omitempty"` // sha256 del canonico dell'indice: copre anche i limiti
	VersioneLimiti  string                 `json:"versione_limiti"`           // la versione_limiti dell'indice (R43 B)
	ClientiAttivi   int                    `json:"clienti_attivi"`
	ClientiScartati int                    `json:"clienti_scartati"`
	Clienti         []RegoleCliente        `json:"clienti,omitempty"` // nell'ordine dell'indice
	IndiceNonValido []evidenze.Diagnostica `json:"indice_non_valido,omitempty"`
	// CoerenzaEseguita: gli attesi sono stati letti e gli esempi con rif_caso confrontati con i loro casi.
	// Falso se gli attesi non c'erano: il controllo è non eseguito, mai «coerente».
	CoerenzaEseguita bool `json:"coerenza_eseguita"`
	Incoerenze       int  `json:"incoerenze"`
	Lacune           int  `json:"lacune"` // informative (R20 b): non cambiano l'esito
}

// RegoleCliente: un cliente dell'indice. Stato: attivo | scartato.
type RegoleCliente struct {
	ClienteID    string                 `json:"cliente_id"`
	Profili      []string               `json:"profili,omitempty"` // i profili del manifest legati a questo cliente
	File         string                 `json:"file"`
	Stato        string                 `json:"stato"`
	Hash         string                 `json:"hash,omitempty"`
	Famiglie     []FamigliaRapporto     `json:"famiglie,omitempty"`
	Riserve      []RiservaRapporto      `json:"riserve,omitempty"`
	Esempi       ConteggioEsempi        `json:"esempi"`
	Lacune       []Lacuna               `json:"lacune,omitempty"`
	Coerenza     []CoerenzaEsempio      `json:"coerenza,omitempty"`
	Diagnostiche []evidenze.Diagnostica `json:"diagnostiche,omitempty"` // avvisi e note della compilazione, o i motivi dello scarto
}

// Gli stati di un cliente nel rapporto delle regole.
const (
	ClienteAttivo   = "attivo"
	ClienteScartato = "scartato"
)

// FamigliaRapporto: una famiglia con ruoli, categorie e forme, secondo lo stato dichiarato nel file.
type FamigliaRapporto struct {
	ID             string   `json:"id"`
	Ruoli          []string `json:"ruoli"`
	Categorie      []string `json:"categorie,omitempty"`
	FormeAttive    []string `json:"forme_attive,omitempty"`
	FormeRiservate []string `json:"forme_riservate,omitempty"`
}

// RiservaRapporto: una voce di profilo.riserve.
type RiservaRapporto struct {
	ID     string   `json:"id"`
	Motivo string   `json:"motivo"`
	Regole []string `json:"regole,omitempty"`
}

// ConteggioEsempi: gli esempi del file. I non verificati (forme che non entrano nel motore) non sono mai
// passati: si contano a parte, con il loro «famiglia/esempio».
type ConteggioEsempi struct {
	Totali          int      `json:"totali"`
	Verificati      int      `json:"verificati"`
	NonVerificati   int      `json:"non_verificati"`
	NonVerificatiID []string `json:"non_verificati_id,omitempty"`
}

// Lacuna: un pezzo della copertura minima di P1 §5.4 che manca a una regola attiva (R20 b). Le lacune vanno
// al proprietario degli attesi; la regola resta attiva.
type Lacuna struct {
	Famiglia  string `json:"famiglia"`
	Regola    string `json:"regola"` // la forma o la revisione in campo separato
	Selettore string `json:"selettore"`
	Manca     string `json:"manca"` // positivo | negativo_vicino | prova_di_confine
}

// Le lacune di copertura che il banco sa vedere.
const (
	MancaPositivo       = "positivo"
	MancaNegativoVicino = "negativo_vicino"
	MancaProvaConfine   = "prova_di_confine"
)

// CoerenzaEsempio: un esempio con rif_caso confrontato con il caso degli attesi che ripete (CP-17, R47 b).
type CoerenzaEsempio struct {
	Famiglia string   `json:"famiglia"`
	Esempio  string   `json:"esempio"`
	Caso     string   `json:"caso"`
	Coerente bool     `json:"coerente"`
	Motivi   []string `json:"motivi,omitempty"`
}

// VerificaRegole: per ogni voce dell'indice, NuovoSnapshot e CompilaVerificato (attraverso CompilaInsieme),
// con i limiti dell'indice: non c'è un parametro di limiti (R43 B). Riporta per cliente l'hash, le famiglie
// con ruoli e categorie, le forme attive e riservate, le riserve, gli esempi verificati e non verificati, la
// copertura minima di P1 §5.4 per ogni regola attiva con le lacune (R20 b) e la coerenza degli esempi con
// rif_caso con il caso degli attesi (R47 b). contenuti sono i byte delle grammatiche, per nome come l'indice
// li scrive: un file che manca è un cliente scartato. Se gli attesi non sono stati letti (a.Casi nil) la
// coerenza resta non eseguita.
//
// Il riferimento al caso (rif_caso) lo legge solo questo pacchetto: il motore non lo vede mai (R47 b). Per la
// coerenza un esempio con rif_caso deve avere un caso con quell'ID, dello stesso cliente (attraverso i profili
// del manifest), con lo stesso selettore dopo R19 e lo stesso testo, e con valori che non si contraddicono;
// in caso di conflitto vincono gli attesi, e si corregge l'esempio (par.4.10 n.3).
func VerificaRegole(m dataset.Manifest, ind grammatica.IndiceRegole, contenuti map[string][]byte, a Attesi) RapportoRegole {
	r := RapportoRegole{VersioneLimiti: ind.Limiti.Versione, CoerenzaEseguita: a.Casi != nil}
	ins, diag, err := motorea.CompilaInsieme(ind, contenuti)
	if err != nil {
		var ec *evidenze.ErroreContratto
		if errors.As(err, &ec) {
			r.IndiceNonValido = ec.Diagnostiche
		} else {
			r.IndiceNonValido = diag
		}
		return r
	}
	r.ImprontaIndice = ins.ImprontaIndice

	casi := map[string]CasoContratto{}
	for _, c := range a.Casi {
		casi[c.ID] = c
	}
	for _, v := range ind.Grammatiche {
		rc := RegoleCliente{ClienteID: v.ClienteID.String(), File: v.File, Profili: profiliDi(m, v.ClienteID)}
		mot := ins.Motori[v.ClienteID]
		if mot == nil {
			rc.Stato = ClienteScartato
			rc.Diagnostiche = ins.Scartati[v.ClienteID]
			r.ClientiScartati++
			r.Clienti = append(r.Clienti, rc)
			continue
		}
		r.ClientiAttivi++
		rc.Stato = ClienteAttivo
		s := mot.Snapshot()
		rc.Hash = s.Hash
		rc.Diagnostiche = diagnosticheDi(s, ind.Limiti)
		g := s.Grammatica
		for _, ris := range g.Profilo.Riserve {
			rc.Riserve = append(rc.Riserve, RiservaRapporto{ID: ris.ID, Motivo: ris.Motivo, Regole: ris.Regole})
		}
		nonVerificati := esempiNonVerificati(rc.Diagnostiche)
		for _, f := range g.Famiglie {
			fr := FamigliaRapporto{ID: f.ID}
			for _, x := range f.Ruoli {
				fr.Ruoli = append(fr.Ruoli, string(x))
			}
			for _, x := range f.Categorie {
				fr.Categorie = append(fr.Categorie, string(x))
			}
			for _, fo := range f.Forme {
				if fo.Stato == grammatica.StatoAttiva {
					fr.FormeAttive = append(fr.FormeAttive, fo.ID)
				} else {
					fr.FormeRiservate = append(fr.FormeRiservate, fo.ID)
				}
			}
			rc.Famiglie = append(rc.Famiglie, fr)
			for _, e := range f.Esempi {
				rc.Esempi.Totali++
				id := f.ID + "/" + e.ID
				if dentro(id, nonVerificati) {
					rc.Esempi.NonVerificati++
					rc.Esempi.NonVerificatiID = append(rc.Esempi.NonVerificatiID, id)
				} else {
					rc.Esempi.Verificati++
				}
				if e.RifCaso != "" && r.CoerenzaEseguita {
					ce := coerenza(f.ID, e, casi, m.Profili, v.ClienteID)
					if !ce.Coerente {
						r.Incoerenze++
					}
					rc.Coerenza = append(rc.Coerenza, ce)
				}
			}
			rc.Lacune = append(rc.Lacune, copertura(f, mot)...)
		}
		r.Lacune += len(rc.Lacune)
		r.Clienti = append(r.Clienti, rc)
	}
	return r
}

// Differenze: quanti problemi rendono il rapporto «con differenze»: l'indice non valido, i clienti scartati,
// gli esempi incoerenti con gli attesi. Le lacune di copertura non contano (R20 b).
func (r RapportoRegole) Differenze() int {
	n := r.ClientiScartati + r.Incoerenze
	if len(r.IndiceNonValido) > 0 {
		n++
	}
	return n
}

// profiliDi: i profili del manifest legati a un cliente, in ordine di nome.
func profiliDi(m dataset.Manifest, cliente uuid.UUID) []string {
	var out []string
	for nome, id := range m.Profili {
		if u, err := uuid.Parse(id); err == nil && u == cliente {
			out = append(out, nome)
		}
	}
	return unici(out)
}

// diagnosticheDi: gli avvisi e le note della compilazione di un cliente attivo. CompilaInsieme le mette in
// fila voce per voce senza separarle, e il motore non le conserva: si ricompila il cliente da solo, con lo
// stesso snapshot e gli stessi limiti dell'indice. Stesso ingresso, stesse diagnostiche: il motore è
// deterministico.
func diagnosticheDi(s grammatica.SnapshotRegole, lim grammatica.Limiti) []evidenze.Diagnostica {
	_, d, _ := motorea.CompilaVerificato(s, lim) // lo stesso snapshot ha già compilato: nessun errore
	return d
}

// esempiNonVerificati: «famiglia/esempio» degli esempi che la compilazione dichiara non verificati.
func esempiNonVerificati(d []evidenze.Diagnostica) []string {
	var out []string
	for _, x := range d {
		if x.Codice != motorea.CodiceEsempioNonVerificato || len(x.Rif) == 0 {
			continue
		}
		fam := ""
		if s, ok := strings.CutPrefix(x.Percorso, "famiglie_codice["); ok {
			fam, _, _ = strings.Cut(s, "]")
		}
		out = append(out, fam+"/"+x.Rif[0])
	}
	return out
}

// copertura: la copertura minima di P1 §5.4 che il banco sa vedere, per ogni forma attiva e per ogni suo
// selettore attivo: un esempio positivo, un negativo vicino (un esempio della famiglia sullo stesso selettore
// senza letture), una prova di confine (un positivo in cui la lettura non copre tutto il testo). Per le
// revisioni in campo separato attive: un positivo per selettore. Il resto della copertura di P1 §5.4 (classi,
// coesistenze) dipende da A1b e non si misura qui.
func copertura(f grammatica.FamigliaCodice, m *motorea.Motore) []Lacuna {
	var out []Lacuna
	for _, fo := range f.Forme {
		if !formaAttiva(fo, f.Base) {
			continue
		}
		for _, s := range fo.Selettori {
			if !grammatica.SelettoreAttivo(s) {
				continue
			}
			sel, _ := evidenze.LeggiSelettore(s)
			positivo, negativo, confine := false, false, false
			for _, e := range f.Esempi {
				es, err := evidenze.LeggiSelettore(e.Selettore)
				if err != nil || es != sel {
					continue
				}
				if e.Atteso.Nessuna {
					negativo = true
					continue
				}
				if !attendeForma(e, f.ID, fo.ID) {
					continue
				}
				positivo = true
				ls, _ := m.Riconosci(sel, e.Testo)
				for _, l := range ls {
					if l.Famiglia == f.ID && l.Forma == fo.ID && (l.Intervallo.Inizio > 0 || l.Intervallo.Fine < len(e.Testo)) {
						confine = true
					}
				}
			}
			if !positivo {
				out = append(out, Lacuna{Famiglia: f.ID, Regola: fo.ID, Selettore: sel.String(), Manca: MancaPositivo})
			}
			if !negativo {
				out = append(out, Lacuna{Famiglia: f.ID, Regola: fo.ID, Selettore: sel.String(), Manca: MancaNegativoVicino})
			}
			if !confine {
				out = append(out, Lacuna{Famiglia: f.ID, Regola: fo.ID, Selettore: sel.String(), Manca: MancaProvaConfine})
			}
		}
	}
	for _, rv := range f.Revisioni {
		if rv.Stato != grammatica.StatoAttiva || rv.Sorgente != grammatica.SorgenteCampoSeparato {
			continue
		}
		for _, s := range rv.Selettori {
			if !grammatica.SelettoreAttivo(s) {
				continue
			}
			sel, _ := evidenze.LeggiSelettore(s)
			positivo := false
			for _, e := range f.Esempi {
				es, err := evidenze.LeggiSelettore(e.Selettore)
				if err != nil || es != sel {
					continue
				}
				for _, la := range e.Atteso.Letture {
					if la.Forma == "" && la.Revisione != "" && (la.Famiglia == "" || la.Famiglia == f.ID) {
						positivo = true
					}
				}
			}
			if !positivo {
				out = append(out, Lacuna{Famiglia: f.ID, Regola: rv.ID, Selettore: sel.String(), Manca: MancaPositivo})
			}
		}
	}
	return out
}

// attendeForma: l'esempio attende una lettura di quella forma della famiglia.
func attendeForma(e grammatica.EsempioCodice, famiglia, forma string) bool {
	for _, la := range e.Atteso.Letture {
		if la.Forma == forma && (la.Famiglia == "" || la.Famiglia == famiglia) {
			return true
		}
	}
	return false
}

// coerenza: un esempio con rif_caso e il caso che ripete. I valori si confrontano solo dove tutti e due li
// dicono: base, basi, marcatore, revisione, e «nessuna lettura» contro letture_identita = 0.
func coerenza(famiglia string, e grammatica.EsempioCodice, casi map[string]CasoContratto, profili map[string]string, cliente uuid.UUID) CoerenzaEsempio {
	ce := CoerenzaEsempio{Famiglia: famiglia, Esempio: e.ID, Caso: e.RifCaso}
	c, ok := casi[e.RifCaso]
	if !ok {
		ce.Motivi = append(ce.Motivi, "il caso non c'è negli attesi")
		return ce
	}
	if u, err := uuid.Parse(profili[c.Profilo]); err != nil || u != cliente {
		ce.Motivi = append(ce.Motivi, "il caso è di un profilo che il manifest non lega a questo cliente")
	}
	selCaso, errCaso := TraduciContesto(c.Contesto)
	selEs, errEs := evidenze.LeggiSelettore(e.Selettore)
	if errCaso != nil || errEs != nil || selCaso != selEs {
		ce.Motivi = append(ce.Motivi, fmt.Sprintf("selettore diverso: esempio %q, caso %q (dopo R19)", e.Selettore, c.Contesto))
	}
	if c.Testo != e.Testo {
		ce.Motivi = append(ce.Motivi, "testo diverso da quello del caso")
	}
	atteso := map[string]ValoreAtteso{}
	for _, k := range c.Atteso {
		atteso[k.Chiave] = k.Valore
	}
	var basiEs, marcatoriEs, revisioniEs []string
	for _, la := range e.Atteso.Letture {
		basiEs = append(basiEs, la.Base)
		marcatoriEs = append(marcatoriEs, la.Marcatore)
		revisioniEs = append(revisioniEs, la.Revisione)
	}
	basiEs, marcatoriEs, revisioniEs = unici(basiEs), unici(marcatoriEs), unici(revisioniEs)
	contraddice := func(chiave string, valori []string) {
		v, ok := atteso[chiave]
		if !ok || len(valori) == 0 {
			return
		}
		attesi, ok := testiAttesi(v)
		if !ok {
			return
		}
		if strings.Join(unici(attesi), "\x00") != strings.Join(valori, "\x00") {
			ce.Motivi = append(ce.Motivi, fmt.Sprintf("%s: il caso dice %s, l'esempio %s", chiave, v.String(), elenco(valori)))
		}
	}
	contraddice("base", basiEs)
	contraddice("basi", basiEs)
	contraddice("marcatore", marcatoriEs)
	if v, ok := atteso["revisione"]; ok {
		if v.Tipo == TipoNullo && len(revisioniEs) > 0 {
			ce.Motivi = append(ce.Motivi, "revisione: il caso dice null, l'esempio "+elenco(revisioniEs))
		} else {
			contraddice("revisione", revisioniEs)
		}
	}
	if v, ok := atteso["letture_identita"]; ok && v.Tipo == TipoIntero && v.Testo == "0" && len(e.Atteso.Letture) > 0 {
		ce.Motivi = append(ce.Motivi, "letture_identita: il caso dice 0, l'esempio attende letture")
	}
	if e.Atteso.Nessuna {
		for _, k := range []string{"base", "basi", "marcatore"} {
			if _, ok := atteso[k]; ok {
				ce.Motivi = append(ce.Motivi, k+": il caso attende una lettura, l'esempio nessuna")
			}
		}
	}
	ce.Coerente = len(ce.Motivi) == 0
	return ce
}
