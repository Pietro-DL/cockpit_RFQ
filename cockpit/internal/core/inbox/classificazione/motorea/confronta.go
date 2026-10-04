package motorea

// Compatibilita: uguale | equivalente | compatibile_parziale | discordante | non_determinabile (parte 1 §5.2,
// più compatibile_parziale per le forme parziali, A-C07). Un'ambiguità resta un'ambiguità: nessun
// completamento e nessun ordinamento.
type Compatibilita string

const (
	CompatibilitaUguale           Compatibilita = "uguale"
	CompatibilitaEquivalente      Compatibilita = "equivalente"
	CompatibilitaParziale         Compatibilita = "compatibile_parziale"
	CompatibilitaDiscordante      Compatibilita = "discordante"
	CompatibilitaNonDeterminabile Compatibilita = "non_determinabile"
)

// ConfrontaBasi confronta due basi segmento per segmento, per nome, sulle forme normalizzate (A-C07; P1
// §5.2).
//   - Basi con una struttura diversa (i nomi dei segmenti, letti più mancanti, non coincidono) o senza
//     segmenti: non_determinabile. È il modo in cui il confronto vede due spazi di codici diversi: BaseLetta
//     non porta il namespace, che chi confronta due letture deve controllare prima su LetturaForma.Namespace
//     (un namespace diverso è non_determinabile).
//   - Due basi complete: uguale se ogni segmento coincide, altrimenti discordante.
//   - Una base parziale e una completa, o due parziali: compatibile_parziale se i segmenti letti da tutte e
//     due coincidono, discordante se uno differisce. Due basi parziali che hanno entrambe un completamento
//     restano compatibili: l'ambiguità resta (A-C07), e il segmento mancante non si inventa mai.
func ConfrontaBasi(a, b BaseLetta) Compatibilita {
	if len(a.Segmenti) == 0 || len(b.Segmenti) == 0 || !stessoInsieme(nomiBase(a), nomiBase(b)) {
		return CompatibilitaNonDeterminabile
	}
	for _, sa := range a.Segmenti {
		for _, sb := range b.Segmenti {
			if sa.Nome == sb.Nome && sa.Normalizzato != sb.Normalizzato {
				return CompatibilitaDiscordante
			}
		}
	}
	if a.Completa && b.Completa {
		return CompatibilitaUguale
	}
	return CompatibilitaParziale
}

// nomiBase: i nomi dei segmenti di una base, letti e mancanti.
func nomiBase(b BaseLetta) []string {
	out := make([]string, 0, len(b.Segmenti)+len(b.Mancanti))
	for _, s := range b.Segmenti {
		out = append(out, s.Nome)
	}
	return append(out, b.Mancanti...)
}

// ConfrontaRevisioni: solo le equivalenze dichiarate; nessun ordinamento delle revisioni (P1 §5.2: «00.00 e
// 00 non sono equivalenti per ipotesi»).
//   - Una delle due non è «letta» (un token sospeso, una revisione assente): non_determinabile.
//   - La stessa stringa normalizzata, zeri compresi: uguale.
//   - Una coppia che la regola della revisione dichiara equivalente: equivalente.
//   - Altrimenti: discordante.
func ConfrontaRevisioni(a, b RevisioneLetta) Compatibilita {
	if a.Stato != StatoRevisioneLetta || b.Stato != StatoRevisioneLetta {
		return CompatibilitaNonDeterminabile
	}
	if a.Normalizzata == b.Normalizzata {
		return CompatibilitaUguale
	}
	for _, eq := range append(append([][2]string(nil), a.equivalenze...), b.equivalenze...) {
		if (eq[0] == a.Normalizzata && eq[1] == b.Normalizzata) || (eq[0] == b.Normalizzata && eq[1] == a.Normalizzata) {
			return CompatibilitaEquivalente
		}
	}
	return CompatibilitaDiscordante
}
