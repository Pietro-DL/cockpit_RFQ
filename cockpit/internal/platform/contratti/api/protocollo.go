package api

import "time"

// I tempi del protocollo dei worker stanno qui, e solo qui.
//
// Il blocco 2 del checkpoint 3R nasce da un difetto che non era «60 e' poco»: era che il valore 60 non
// discendeva da niente. La testata chiamava offline un worker fermo da 60 secondi; il worker si faceva
// vivo ogni 20 secondi quando era in attesa e ogni `lease_s / 4` — anche centocinquanta secondi —
// mentre lavorava. Tre numeri scritti in tre file che nessuno aveva mai messo uno accanto all'altro,
// e che insieme dicevano che un worker occupato e' spento.
//
// La regola, unica, e' questa: un worker vivo si fa riconoscere almeno ogni AttesaClaim; chi non si fa
// riconoscere per il doppio di quel tempo e' offline. Il doppio, non il triplo: perdere un giro intero
// e' tollerato, perderne due e' un guasto da mostrare. Cambiare AttesaClaim qui muove tutto il resto
// con se', compresa la cadenza massima del battito nei worker Python, che legge gli stessi numeri da
// workers/contratti.py (e un test di contratto verifica che le due copie coincidano).
const (
	// AttesaClaim: quanto il worker chiede di restare appeso in POST /jobs/claim quando non c'e'
	// lavoro. E' anche il ritmo con cui, da fermo, un worker si fa vivo.
	AttesaClaim = 20 * time.Second

	// AttesaClaimMax: il tetto che il server impone a `attesa_s`. Sopra questo, il long-poll comincia a
	// somigliare a una connessione dimenticata.
	AttesaClaimMax = 25 * time.Second

	// PresenzaOnlineEntro: oltre questo tempo dall'ultimo contatto autenticato, un worker e' OFFLINE.
	// Non e' una costante indipendente: e' due giri di claim.
	PresenzaOnlineEntro = 2 * AttesaClaim

	// BattitoMax: la cadenza MASSIMA del heartbeat durante un job. Dentro un job il worker non entra
	// piu' in claim, quindi il battito e' l'unica cosa che lo tiene riconoscibile: se battesse piu'
	// lentamente di cosi' sparirebbe dalla testata mentre lavora, che e' esattamente il difetto del
	// blocco 2. Un battito piu' fitto (job con lease corto) va bene: costa una riga di UPDATE.
	BattitoMax = AttesaClaim
)
