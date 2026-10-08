#!/bin/sh
# Ricompone il mockup in un solo file: ../cockpit-inbox-promatec.html (quello da pubblicare).
# I pezzi vanno in quest'ordine: testa e stili, poi gli script dentro un unico <script>.
cd "$(dirname "$0")" || exit 1
{ cat 00_head.html 05_css.html; echo "<script>"; cat 10_icone.js 15_contratto.js 20_dati.js 30_inbox.js 40_rfq.js 45_distinta.js 47_ciclo.js 48_conferma.js 50_set.js 60_mount.js; } > ../cockpit-frontend-mockup.html
echo "Fatto: ../cockpit-frontend-mockup.html"
