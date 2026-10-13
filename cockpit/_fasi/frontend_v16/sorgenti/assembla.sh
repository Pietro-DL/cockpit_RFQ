#!/bin/sh
# Ricompone il mockup in un solo file: ../cockpit-frontend-mockup-v16.html (quello da pubblicare).
# v16 (C2 passo 0: 08_css_v16, 49b_albero_processo); v15, passata 2 (SPEC_v15 §7.1): testa e stili, poi gli script dentro un unico <script>.
cd "$(dirname "$0")" || exit 1
{ cat 00_head.html 05_css.html 06_css_distinta.html 07_css_lavoro.html 08_css_v16.html; echo "<script>"; cat 10_icone.js 15_ponte_a.js 16_etichette.js 17_server_a.js 18_server_a2.js 20_dati.js 21_fixture_a.js 30_inbox.js 40_guscio.js 45_struttura.js 46_cataloghi.js 46b_magazzino.js 46c_capacita.js 47_come.js 48_ciclo.js 49_sequenza.js 49b_albero_processo.js 50_set.js 51_esito.js 52_offerta.js 53_richieste.js 54_cassetti.js 58_tasti.js 60_mount.js; } > ../cockpit-frontend-mockup-v16.html
echo "Fatto: ../cockpit-frontend-mockup-v16.html"
