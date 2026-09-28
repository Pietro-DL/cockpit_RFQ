package fascicolo

// Il gate del congelamento (addendum A4.6, passo 3; A4.5 per lo STEP del prodotto finito).
//
// La lettura dei dati e la regola sono separate: LeggiGate legge (i conteggi, lo STEP di ogni finito,
// gli archi della working), Valuta decide su quello che ha letto ed e' una funzione pura, che si prova
// senza database. Ogni condizione non soddisfatta diventa una frase per l'operatore.
//
// Smistamento F6 (A5.4.8, U2): il gate separa la completezza LOGICA (file deciso, componente deciso,
// struttura decisa), che ferma il congelamento, dalla MATERIALIZZAZIONE (la copia sul NAS riuscita), che
// non lo ferma. Un errore di rete non rende falsa una BOM: impedira' la chiusura operativa della RFQ,
// quando quel gesto esistera', ma e' un problema diverso. Un documento in errore al congelamento entra
// nell'istantanea; la copia riuscita dopo non tocca la baseline (BOM01 lascia liberi path_relativo e
// stato_nas, 0020:595-605).
//
// Una condizione di A4.6 non c'e' ancora: «zero proposte di sostituzione pendenti». Il modello non
// registra da nessuna parte una proposta di sostituzione, ne' la risposta «si aggiunge» (il secondo
// foglio di un 2D), quindi «pendente» non si puo' calcolare. E' una decisione aperta, non un'omissione.

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Gli esiti di v_step_prodotto (0020), nell'ordine in cui la vista li decide.
const (
	StepRadiceSenzaQualifica = "radice_senza_qualifica"
	StepRiferimentoSuperato  = "riferimento_superato"
	StepNonAnalizzato        = "presente_non_analizzato"
	StepAnalizzato           = "presente_analizzato"
	StepParziale             = "presente_parziale"
	StepDaScegliere          = "da_scegliere"
	StepSoloAltro3d          = "solo_altro_3d"
	StepDaConfermare         = "da_confermare"
	StepSulPortale           = "sul_portale"
	StepMancante             = "mancante"
)

// EtichettaStep e' la frase di un esito per la schermata e per la barra di completezza (A4.5). E'
// deterministica: stessa base, stessa risposta.
func EtichettaStep(esito string) string {
	switch esito {
	case StepMancante:
		return "STEP prodotto finito: MANCANTE — da sollecitare"
	case StepSulPortale:
		return "STEP prodotto finito: SUL PORTALE — da scaricare"
	case StepDaConfermare:
		return "STEP prodotto finito: DA CONFERMARE — c'è una proposta aperta"
	case StepSoloAltro3d:
		return "STEP prodotto finito: SOLO UN ALTRO 3D — manca lo STEP"
	case StepDaScegliere:
		return "STEP prodotto finito: DA SCEGLIERE — quale STEP è la distinta"
	case StepRiferimentoSuperato:
		return "STEP prodotto finito: RIFERIMENTO SUPERATO — scegliere il nuovo"
	case StepNonAnalizzato:
		return "STEP prodotto finito: PRESENTE, NON ANALIZZATO"
	case StepParziale:
		return "STEP prodotto finito: PRESENTE, LETTO IN PARTE"
	case StepAnalizzato:
		return "STEP prodotto finito: PRESENTE"
	case StepRadiceSenzaQualifica:
		return "radice senza qualifica di prodotto"
	}
	return "STEP prodotto finito: esito sconosciuto " + esito
}

// Gate e' l'esito del controllo: problemi (bloccano il congelamento), materializzazione e avvisi (no).
type Gate struct {
	Problemi []string
	// Materializzazione e' il terzo elenco (A5.4.8): le copie sul NAS in coda o in errore e le anomalie
	// aperte, in frasi. Si mostra, non ferma il congelamento. Nas sono gli stessi fatti in numeri, per la
	// barra «Sul NAS».
	Materializzazione []string
	Nas               Materializzazione
	Avvisi            []string
}

// Passa dice se si puo' congelare.
func (g Gate) Passa() bool { return len(g.Problemi) == 0 }

// Motivo e' l'elenco dei problemi in una frase sola, per il rifiuto.
func (g Gate) Motivo() string { return strings.Join(g.Problemi, "; ") }

// Arco e' un arco della BOM working fra due componenti attivi.
type Arco struct {
	Padre, Figlio uuid.UUID
}

// Conti sono i conteggi che il gate legge in una volta: i bloccanti logici, le proposte strutturali, la
// materializzazione.
type Conti struct {
	// BloccantiLogici: requisiti bloccanti del fascicolo senza documento e senza deroga
	// (ContaBloccantiLogici). Un documento deciso ma in errore sul NAS non c'e', a differenza di
	// v_thread_bloccanti.n_bloccanti.
	BloccantiLogici int
	// Strutturali: le proposte aperte. Con F5 le legge GateStrutturale, solo nell'autorita'.
	Strutturali db.GateCongelamentoRow
	// Nas: la materializzazione (StatoMaterializzazione). Non entra nei problemi.
	Nas Materializzazione
}

// Valuta applica la regola del gate a dati gia' letti. codici serve solo alle frasi.
func Valuta(c Conti, step []db.ListGateStepRow, archi []Arco, codici map[uuid.UUID]string) Gate {
	g := Gate{Nas: c.Nas, Materializzazione: c.Nas.Frasi()}
	problema := func(f string, a ...any) { g.Problemi = append(g.Problemi, fmt.Sprintf(f, a...)) }
	// La logica. (L1) I file tecnici della RFQ non smistati entrano con la decisione (F11): prima, un gate
	// che si ferma sui file non smistati non avrebbe il gesto per smistarli. (L2, L3) Le decisioni
	// strutturali aperte e le autorizzazioni da sistemare nell'autorita' entrano con F5 (GateStrutturale);
	// fino ad allora contano tutte le proposte aperte, come prima.
	if c.BloccantiLogici > 0 {
		problema("%d requisiti bloccanti del fascicolo non soddisfatti né derogati", c.BloccantiLogici)
	}
	s := c.Strutturali
	if n := s.NProposteComponente + s.NProposteRelazione + s.NProposteRimozione; n > 0 {
		problema("%d proposte strutturali aperte (%d componenti, %d relazioni, %d rimozioni): si decidono prima di congelare",
			n, s.NProposteComponente, s.NProposteRelazione, s.NProposteRimozione)
	}
	for _, r := range step {
		s := r.VStepProdotto
		etichetta := s.Codice + ": " + EtichettaStep(s.Esito)
		switch s.Esito {
		case StepAnalizzato:
		case StepParziale, StepNonAnalizzato:
			if !s.DerogaStrutturaID.Valid {
				problema("%s (%s): per congelare serve una deroga strutturale su questo STEP e questa lettura", etichetta, s.MotivoParziale.String)
			}
		case StepDaScegliere, StepRiferimentoSuperato:
			problema("%s: si sceglie lo STEP strutturale, una deroga non basta", etichetta)
		case StepMancante, StepSulPortale, StepDaConfermare, StepSoloAltro3d:
			if !r.DerogaCad {
				problema("%s: serve lo STEP, oppure la deroga del fabbisogno cad_3d di questo componente", etichetta)
			}
		case StepRadiceSenzaQualifica:
			g.Avvisi = append(g.Avvisi, etichetta)
		default:
			problema("%s", etichetta)
		}
	}
	if ciclo := Ciclo(archi); ciclo != nil {
		// il ciclo si racconta a partire dal codice minore: la stessa struttura, la stessa frase
		giro := make([]string, len(ciclo)-1)
		for i, id := range ciclo[:len(ciclo)-1] {
			giro[i] = codici[id]
			if giro[i] == "" {
				giro[i] = id.String()
			}
		}
		primo := 0
		for i := range giro {
			if giro[i] < giro[primo] {
				primo = i
			}
		}
		giro = append(giro[primo:], giro[:primo]...)
		problema("la struttura ha un ciclo: %s → %s", strings.Join(giro, " → "), giro[0])
	}
	return g
}

// Ciclo restituisce un ciclo degli archi (il primo nodo ripetuto in fondo), oppure nil. Un pezzo
// dentro se stesso, anche passando per altri, non e' una distinta.
func Ciclo(archi []Arco) []uuid.UUID {
	figli := map[uuid.UUID][]uuid.UUID{}
	var nodi []uuid.UUID
	visto := map[uuid.UUID]bool{}
	for _, a := range archi {
		figli[a.Padre] = append(figli[a.Padre], a.Figlio)
		for _, n := range []uuid.UUID{a.Padre, a.Figlio} {
			if !visto[n] {
				visto[n] = true
				nodi = append(nodi, n)
			}
		}
	}
	const (
		nuovo = iota
		inCorso
		chiuso
	)
	stato := map[uuid.UUID]int{}
	var pila []uuid.UUID
	var trovato []uuid.UUID
	var visita func(n uuid.UUID) bool
	visita = func(n uuid.UUID) bool {
		stato[n] = inCorso
		pila = append(pila, n)
		for _, f := range figli[n] {
			switch stato[f] {
			case inCorso:
				for i, x := range pila {
					if x == f {
						trovato = append(append([]uuid.UUID{}, pila[i:]...), f)
						return true
					}
				}
			case nuovo:
				if visita(f) {
					return true
				}
			}
		}
		pila = pila[:len(pila)-1]
		stato[n] = chiuso
		return false
	}
	for _, n := range nodi {
		if stato[n] == nuovo && visita(n) {
			return trovato
		}
	}
	return nil
}

// LeggiGate legge i dati del gate della RFQ e li valuta.
func LeggiGate(ctx context.Context, q *db.Queries, thread uuid.UUID) (Gate, error) {
	var c Conti
	var err error
	if c.Strutturali, err = q.GateCongelamento(ctx, thread); err != nil {
		return Gate{}, err
	}
	n, err := q.ContaBloccantiLogici(ctx, thread)
	if err != nil {
		return Gate{}, err
	}
	c.BloccantiLogici = int(n)
	if c.Nas, err = LeggiMaterializzazione(ctx, q, thread); err != nil {
		return Gate{}, err
	}
	step, err := q.ListGateStep(ctx, thread)
	if err != nil {
		return Gate{}, err
	}
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return Gate{}, err
	}
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return Gate{}, err
	}
	codici := map[uuid.UUID]string{}
	for _, x := range comp {
		codici[x.ComponenteID] = x.Codice
	}
	archi := make([]Arco, len(rel))
	for i, r := range rel {
		archi[i] = Arco{Padre: r.PadreID, Figlio: r.FiglioID}
	}
	return Valuta(c, step, archi, codici), nil
}

// ------------------------------------------------------------------ la materializzazione (A5.4.8)

// Materializzazione e' lo stato delle copie sul NAS dei documenti decisi della RFQ (U2): quanti sono
// scritti su quanti decisi, quanti aspettano la copia, quanti sono in errore, quante anomalie di
// integrita' sono aperte. E' la barra «Sul NAS: 9 di 11», in testata e nel Fascicolo. Non e' un
// conteggio del gate.
type Materializzazione struct {
	Documenti int // i documenti decisi (correnti) della RFQ, generali compresi
	Scritti   int
	InCoda    int
	Errore    int
	Anomalie  int // anomalie NAS aperte su quei documenti
	FraMinuti int // fra quanti minuti parte la prima copia in coda che aspetta un orario; 0 = nessuna
}

// LeggiMaterializzazione legge la materializzazione della RFQ (StatoMaterializzazione).
func LeggiMaterializzazione(ctx context.Context, q *db.Queries, thread uuid.UUID) (Materializzazione, error) {
	r, err := q.StatoMaterializzazione(ctx, thread)
	if err != nil {
		return Materializzazione{}, err
	}
	return Materializzazione{Documenti: int(r.NDocumenti), Scritti: int(r.NScritti), InCoda: int(r.NInCoda),
		Errore: int(r.NErrore), Anomalie: int(r.NAnomalie), FraMinuti: int(r.MinutiProssimaCopia)}, nil
}

// Completa dice se ogni documento deciso e' sul NAS, senza anomalie aperte.
func (m Materializzazione) Completa() bool {
	return m.Scritti == m.Documenti && m.Anomalie == 0
}

// Guasti dice se c'e' qualcosa che una persona deve guardare: copie in errore o anomalie. Una copia in
// coda no, arriva da sola.
func (m Materializzazione) Guasti() bool { return m.Errore > 0 || m.Anomalie > 0 }

// Breve e' la voce della testata: «Sul NAS: 9 di 11».
func (m Materializzazione) Breve() string {
	return fmt.Sprintf("Sul NAS: %d di %d", m.Scritti, m.Documenti)
}

// Frasi e' il terzo elenco del gate: una frase per ogni cosa che manca sul NAS. Vuoto = tutto scritto.
func (m Materializzazione) Frasi() []string {
	var f []string
	if m.InCoda > 0 {
		s := fmt.Sprintf("%d %s in coda per la copia sul NAS", m.InCoda, plurale(m.InCoda, "documento", "documenti"))
		if m.FraMinuti > 0 {
			s += fmt.Sprintf(" (la prima fra %d min)", m.FraMinuti)
		}
		f = append(f, s)
	}
	if m.Errore > 0 {
		f = append(f, fmt.Sprintf("%d %s in errore sul NAS", m.Errore, plurale(m.Errore, "documento", "documenti")))
	}
	if m.Anomalie > 0 {
		f = append(f, fmt.Sprintf("%d %s sui documenti", m.Anomalie, plurale(m.Anomalie, "anomalia NAS aperta", "anomalie NAS aperte")))
	}
	return f
}

func plurale(n int, uno, molti string) string {
	if n == 1 {
		return uno
	}
	return molti
}
