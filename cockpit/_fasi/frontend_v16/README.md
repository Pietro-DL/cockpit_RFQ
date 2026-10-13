# Frontend v16 · mockup della Distinta

> **⚠ Mockup da visionare e collaudare da Stefano: non è ancora approvato; non va implementato prima della sua verifica e del suo collaudo.**

Mockup del frontend del Cockpit RFQ, versione 16: Struttura e documenti, Come, Ciclo, Sequenza (albero del processo), Esito, richieste ai fornitori, Costi e Offerta. **Solo file nuovi**: nessun file del backend cambia, e `_fasi/` non entra nel binario.

Tutti i dati sono di fantasia (ACME). Prezzi, tariffe e ricarichi sono valori di esempio.

## Che cosa c'è

| File | Che cosa è |
|---|---|
| `RELAZIONE_MOCKUP_v16.md` | **da leggere per prima**: le 9 richieste di Stefano, le zone A/B/C, la modifica di template proposta, il modello dati di zona B, le richieste R-xx, la prova su un caso reale anonimizzato, le decisioni aperte |
| `cockpit-frontend-mockup-v16.html` | il mockup in un solo file: si apre con un browser, senza server |
| `sorgenti/` | i pezzi del mockup (`00_…` → `60_…`), `assembla.sh` che li ricompone, `package.json` e le cinque prove |

Le prove (jsdom), in `sorgenti/`:

| Prova | Che cosa guarda | Esito |
|---|---|---|
| `test_clic.cjs` | tutti i percorsi del mockup | 437 / 0 |
| `test_v16_c1.cjs` | tema, guscio, Struttura e documenti, Come, Esito, Offerta, cassetti, Inbox | 192 / 0 |
| `test_v16_c2.cjs` | Ciclo, pre-unioni, preparazioni, Sequenza, albero del processo | 215 / 0 |
| `test_tirante.cjs` | un caso da zero: tirante saldato a 3 componenti, terzisti, due lotti, rev 0 e rev 1 | 96 / 0 |
| `conta_passi.cjs` | i gesti dei percorsi contro i bersagli | OK |

Errori JS: 0 in tutte.

## Come si prova

```
cd sorgenti
npm install     # jsdom
npm test        # le cinque prove
sh assembla.sh  # rifà ../cockpit-frontend-mockup-v16.html dai pezzi
```
