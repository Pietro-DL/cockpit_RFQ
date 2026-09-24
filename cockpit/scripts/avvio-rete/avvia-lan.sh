#!/usr/bin/env bash
# Avvia il Cockpit sulla LAN in IPv4: HTTPS, e connessioni accettate solo dalla rete locale.
#
#   bash scripts/avvio-rete/avvia-lan.sh                          # IPv4 della scheda con il gateway, la sua rete
#   bash scripts/avvio-rete/avvia-lan.sh -c 10.0.0.15,10.0.0.16   # solo questi due PC (e questo)
#   bash scripts/avvio-rete/avvia-lan.sh --mostra                 # stampa il comando, non avvia
#
# L'indirizzo e' quello della scheda che porta il gateway predefinito; la rete ammessa e' la sua
# (10.0.0.7/24 → 10.0.0.0/24), o quella data con -c, che puo' essere piu' stretta: un elenco di PC.
# Il filtro sta nel server, prima del TLS (config.Rete, rete.SoloDalleReti). Il certificato e' in
# tls/lan/. Le opzioni sono in comune.sh (--help).
source "$(dirname "${BASH_SOURCE[0]}")/comune.sh"
avvia_in_rete 4 lan avvia-lan.sh "$@"
