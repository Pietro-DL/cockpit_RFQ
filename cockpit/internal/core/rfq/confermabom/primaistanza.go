package confermabom

// prima-istanza-1: la verifica di V1 della BOM confermata in formato 3 (U2 §5.4; [U] Q14, [U] RV3-00 A, [U] D-SHOCK-S1 B).
// Una funzione pura: la calcolano con gli stessi ingressi R4 (verifica_prevista, dentro la firma), R5 (la ricalcola
// sotto lucchetto nella seconda connessione, e non salva l'esito) e /dati (conferma_corrente.verifica_adesso). Nessuno
// la salva positiva: in conferma_strutturale.verifica il formato 3 porta solo la regola (RegolaPrimaIstanza; CHECK
// ck_conferma_verifica della 0023). Non e' la BOM.Verificata del fascicolo: nessuna guardia la usa in V1.
//
// Vera se valgono tutte (U2 §5.4; le condizioni (a), (c), (d), (e), (f) sono la lettura [T] di «prerequisiti», U2-P25):
//   (a) il criterio e' step (radice con lo STEP autorizzato e almeno un figlio) oppure mono_pezzo (dichiarazione con
//       motivo, nessun figlio);
//   (b) la fonte e' confermata, corrente (non sostituita) e autorizzata; la sorgente della bozza e' quello STEP, per
//       sha256 e radice ([U] D-P4B1-02 A, D-P4A-03 A′);
//   (c) nessuna riga della struttura della fonte da decidere (aperta o decisa da un automatismo), contando i nodi
//       raggiunti, radice esclusa; nessuna rimozione proposta aperta sul perimetro;
//   (d) la gerarchia e' ricostruibile (motivo parziale vuoto sui fatti correnti) oppure c'e' una deroga_struttura valida;
//   (e) nessuna riga aperta di uno STEP secondario autorizzato nel perimetro (U2-P26);
//   (f) nessun conflitto aperto del motore sul perimetro (R95 A [U]); se i conflitti non sono calcolati la verifica non
//       e' mai «si'» da sola: e' «non calcolata».
// Con un blocco aperto del riepilogo la previsione di R4 e' «non calcolata: blocchi aperti».

import "encoding/json"

// RegolaPrimaIstanza e' il nome della regola, l'unico valore che il formato 3 salva in conferma_strutturale.verifica.
const RegolaPrimaIstanza = "prima-istanza-1"

// ColonnaVerificaFormato3: il valore di conferma_strutturale.verifica per una conferma formato 3 (U2 §3.14 riga 8).
func ColonnaVerificaFormato3() json.RawMessage {
	return json.RawMessage(`{"regola": "` + RegolaPrimaIstanza + `"}`)
}

// Criteri della verifica (U2 §5.4, condizione (a)).
const (
	CriterioStep      = "step"
	CriterioMonoPezzo = "mono_pezzo"
)

// Stati della verifica calcolata.
const (
	StatoVerificaCalcolata    = "calcolata"
	StatoVerificaNonCalcolata = "non_calcolata"
)

// I motivi della verifica (codici [T] della corsia persistenza, U2 §5.4). L'ordine dell'elenco e' quello delle condizioni.
const (
	MotivoCriterioAssente           = "criterio_assente"
	MotivoStepSenzaFigli            = "step_senza_figli"
	MotivoMonoPezzoConFigli         = "mono_pezzo_con_figli"
	MotivoMonoPezzoSenzaMotivo      = "mono_pezzo_senza_motivo"
	MotivoFonteNonConfermata        = "fonte_non_confermata"
	MotivoFonteSostituita           = "fonte_sostituita"
	MotivoFonteNonAutorizzata       = "fonte_non_autorizzata"
	MotivoSorgenteDiversa           = "sorgente_diversa_dalla_fonte"
	MotivoRigheDaDecidere           = "righe_da_decidere"
	MotivoRimozioniAperte           = "rimozioni_aperte"
	MotivoGerarchiaNonRicostruibile = "gerarchia_non_ricostruibile"
	MotivoSecondariAperti           = "secondari_aperti"
	MotivoConflittiMotore           = "conflitti_motore_aperti"
	MotivoConflittiNonCalcolati     = "conflitti_motore_non_calcolati"
	MotivoBlocchiAperti             = "blocchi_aperti"
)

// Origini della gerarchia (D-SHOCK-S1 B [U]: criterio, origine_gerarchia, gerarchia_step, stato e motivazione della deroga
// esposti in R4/R5/R7).
const (
	OrigineGerarchiaStep   = "step"
	OrigineGerarchiaDeroga = "deroga"
	OrigineGerarchiaNessun = "nessuna"
)

// IngressiPrimaIstanza: gli ingressi della regola, letti da chi la chiama nella sua fotografia (R4, la S di R5, /dati).
type IngressiPrimaIstanza struct {
	// Criterio: step o mono_pezzo, come la conferma lo certificherebbe (o lo ha certificato); vuoto se nessuno.
	Criterio string
	// FigliDellaRadice: gli archi in uscita dalla radice nella BOM certificata.
	FigliDellaRadice int
	// MotivoMonoPezzo: il motivo della dichiarazione mono_pezzo (vuoto se non c'e').
	MotivoMonoPezzo string

	FonteConfermata  bool
	FonteSostituita  bool
	FonteAutorizzata bool
	// SorgenteUgualeAllaFonte: la sorgente della bozza (o della conferma) e' lo STEP della fonte, per sha256 e radice.
	SorgenteUgualeAllaFonte bool

	// RigheDaDecidere: righe della struttura della fonte aperte o decise da un automatismo, sui nodi raggiunti, radice
	// esclusa. RimozioniAperte: rimozioni proposte aperte sul perimetro.
	RigheDaDecidere int
	RimozioniAperte int

	// MotivoParzialeStruttura: il motivo parziale della struttura (0020) sui fatti correnti; vuoto = ricostruibile.
	MotivoParzialeStruttura string
	// Deroga: la deroga_struttura valida, se c'e'.
	Deroga *DerogaStruttura

	// RigheSecondarieAperte: righe aperte degli STEP secondari autorizzati nel perimetro.
	RigheSecondarieAperte int

	// ConflittiCalcolati: il motore ha calcolato i conflitti; ConflittiAperti: quelli aperti sul perimetro.
	ConflittiCalcolati bool
	ConflittiAperti    int

	// BlocchiAperti: il riepilogo (R4) ha blocchi aperti: la previsione non si calcola.
	BlocchiAperti bool
}

// DerogaStruttura: lo stato e il motivo della deroga di struttura usata (D-SHOCK-S1 B [U]).
type DerogaStruttura struct {
	Stato  string `json:"stato"`
	Motivo string `json:"motivo"`
}

// VerificaPrimaIstanza: l'esito della regola (verifica_prevista di R4, verifica_adesso di /dati). Mai salvato.
type VerificaPrimaIstanza struct {
	Stato                    string           `json:"stato"`
	Verificata               *bool            `json:"verificata,omitempty"`
	Regola                   string           `json:"regola"`
	Criterio                 string           `json:"criterio"`
	Motivi                   []string         `json:"motivi"`
	ConflittiMotoreCalcolati bool             `json:"conflitti_motore_calcolati"`
	OrigineGerarchia         string           `json:"origine_gerarchia"`
	GerarchiaStep            bool             `json:"gerarchia_step"`
	Deroga                   *DerogaStruttura `json:"deroga,omitempty"`
}

// PrimaIstanza calcola la regola prima-istanza-1 (commento di testa). Pura e deterministica: gli stessi ingressi danno
// sempre lo stesso esito, con i motivi nell'ordine delle condizioni.
func PrimaIstanza(in IngressiPrimaIstanza) VerificaPrimaIstanza {
	v := VerificaPrimaIstanza{Regola: RegolaPrimaIstanza, Criterio: in.Criterio, Motivi: []string{},
		ConflittiMotoreCalcolati: in.ConflittiCalcolati, GerarchiaStep: in.MotivoParzialeStruttura == ""}
	switch {
	case in.MotivoParzialeStruttura == "":
		v.OrigineGerarchia = OrigineGerarchiaStep
	case in.Deroga != nil:
		v.OrigineGerarchia = OrigineGerarchiaDeroga
	default:
		v.OrigineGerarchia = OrigineGerarchiaNessun
	}
	if in.Deroga != nil {
		d := *in.Deroga
		v.Deroga = &d
	}
	if in.BlocchiAperti {
		v.Stato = StatoVerificaNonCalcolata
		v.Motivi = append(v.Motivi, MotivoBlocchiAperti)
		return v
	}
	m := func(c string) { v.Motivi = append(v.Motivi, c) }
	// (a)
	switch in.Criterio {
	case CriterioStep:
		if in.FigliDellaRadice < 1 {
			m(MotivoStepSenzaFigli)
		}
	case CriterioMonoPezzo:
		if in.FigliDellaRadice > 0 {
			m(MotivoMonoPezzoConFigli)
		}
		if in.MotivoMonoPezzo == "" {
			m(MotivoMonoPezzoSenzaMotivo)
		}
	default:
		m(MotivoCriterioAssente)
	}
	// (b): senza «Autorizza» la BOM non risulta mai verificata
	if !in.FonteConfermata {
		m(MotivoFonteNonConfermata)
	}
	if in.FonteSostituita {
		m(MotivoFonteSostituita)
	}
	if !in.FonteAutorizzata {
		m(MotivoFonteNonAutorizzata)
	}
	if !in.SorgenteUgualeAllaFonte {
		m(MotivoSorgenteDiversa)
	}
	// (c)
	if in.RigheDaDecidere > 0 {
		m(MotivoRigheDaDecidere)
	}
	if in.RimozioniAperte > 0 {
		m(MotivoRimozioniAperte)
	}
	// (d)
	if in.MotivoParzialeStruttura != "" && in.Deroga == nil {
		m(MotivoGerarchiaNonRicostruibile)
	}
	// (e)
	if in.RigheSecondarieAperte > 0 {
		m(MotivoSecondariAperti)
	}
	// (f)
	if in.ConflittiCalcolati && in.ConflittiAperti > 0 {
		m(MotivoConflittiMotore)
	}
	if !in.ConflittiCalcolati {
		// mai «si'» da sola: se le altre condizioni valgono, la verifica e' «non calcolata»; se una non vale, e' «no»
		if len(v.Motivi) == 0 {
			v.Stato = StatoVerificaNonCalcolata
			v.Motivi = append(v.Motivi, MotivoConflittiNonCalcolati)
			return v
		}
		m(MotivoConflittiNonCalcolati)
	}
	v.Stato = StatoVerificaCalcolata
	ok := len(v.Motivi) == 0
	v.Verificata = &ok
	return v
}
