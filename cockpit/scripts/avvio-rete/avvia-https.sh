#!/usr/bin/env bash
# Avvia il Cockpit in HTTPS su un indirizzo IPv6, con connessioni accettate solo dal suo segmento.
#
#   bash scripts/avvio-rete/avvia-https.sh -a fd12:3456:789a:1::7   # l'IPv6 statico (sulla VM, quello dato dall'IT)
#   bash scripts/avvio-rete/avvia-https.sh                          # scelto da solo fra gli IPv6 di questo PC
#   bash scripts/avvio-rete/avvia-https.sh -a ::1                   # prova su questo PC soltanto
#   bash scripts/avvio-rete/avvia-https.sh --mostra                 # stampa il comando, non avvia
#
# Il posto di questa modalita' e' la VM del NAS, con l'IPv6 statico che le dara' l'IT. Fino ad
# allora si usa l'IPv6 del PC che fa da server; un link-local (fe80::) non basta, perche' i browser
# non aprono un indirizzo con la zona. La rete ammessa e' il segmento dell'indirizzo (/64), o quella
# data con -c. Il certificato e' in tls/https/. Le opzioni sono in comune.sh (--help).
source "$(dirname "${BASH_SOURCE[0]}")/comune.sh"
avvia_in_rete 6 https avvia-https.sh "$@"
