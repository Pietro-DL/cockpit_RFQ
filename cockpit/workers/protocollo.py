"""I tempi del protocollo dei worker. Speculare a internal/platform/contratti/worker/protocollo.go.

`genera_contratti.py` esporta TEMPI in contracts/tempi_protocollo.json e un test Go verifica che le
due copie coincidano: sono numeri che devono essere d'accordo fra processi diversi, e finche' non lo
verificava nessuno non lo erano. Il blocco 2 del checkpoint 3R nasce proprio da li': il server
chiamava offline chi taceva da 60 secondi, il worker si faceva vivo ogni 20 quando era in attesa e
ogni `lease_s / 4` — anche 150 secondi — mentre lavorava. Tre numeri in tre file che nessuno aveva
mai messo uno accanto all'altro, e che insieme dicevano che un worker occupato e' spento.

Solo libreria standard, come cockpit_client: questo modulo lo importano tutti e non deve portarsi
dietro niente.

LA REGOLA, una sola: un worker vivo si fa riconoscere almeno ogni ATTESA_CLAIM_S; chi tace per il
doppio e' offline. Il doppio, non il triplo: perdere un giro intero e' tollerato, perderne due e' un
guasto da mostrare.
"""
from __future__ import annotations

# Quanto il worker chiede di restare appeso in POST /jobs/claim quando non c'e' lavoro. E' anche il
# ritmo con cui, da fermo, un worker si fa vivo.
ATTESA_CLAIM_S = 20

# Oltre questo il server tronca: un long-poll piu' lungo comincia a somigliare a una connessione
# dimenticata.
ATTESA_CLAIM_MAX_S = 25

# Oltre questo silenzio dall'ultimo contatto autenticato, la testata dice OFFLINE. Non e' una
# costante indipendente: sono due giri di claim.
PRESENZA_ONLINE_ENTRO_S = 2 * ATTESA_CLAIM_S

# Cadenza MASSIMA del battito durante un job. Dentro un job il worker non entra piu' in claim, quindi
# il battito e' l'unica cosa che lo tiene riconoscibile: se battesse piu' lentamente sparirebbe dalla
# testata mentre lavora. Piu' fitto va sempre bene.
BATTITO_MAX_S = ATTESA_CLAIM_S

TEMPI = {
    "attesa_claim_s": ATTESA_CLAIM_S,
    "attesa_claim_max_s": ATTESA_CLAIM_MAX_S,
    "presenza_online_entro_s": PRESENZA_ONLINE_ENTRO_S,
    "battito_max_s": BATTITO_MAX_S,
}
