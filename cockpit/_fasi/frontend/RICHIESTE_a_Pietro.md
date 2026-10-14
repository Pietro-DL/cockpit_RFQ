# Richieste al backend (per Pietro)

08/10/2026 · da Stefano (frontend). Ogni voce dice:
- che cosa serve al frontend e perché;
- che cosa c'è già;
- una proposta, che decidi tu.

Nessuna è urgente per chiudere A1c, e nessuna va dentro A1c: le collochi tu (A1d, B7, A2, SV, o dopo). Il dettaglio schermata per schermata è in `VERIFICA_mockup_backend.md`.

## Decisioni di significato

**R-01 · Il mockup è il riferimento UI per B7?**
- Il piano 6.0.7 dice che la UI l'abbiamo definita insieme e che non si riapre.
- Ti chiediamo di usare la versione 10 del mockup come riferimento visivo del contratto DTO di B7, con l'elenco dei campi di `CONTRATTI_frontend_proposta.md`.

**R-02 · «Richiesta» e «RFQ».**
- Nel mockup una richiesta vive nell'Inbox anche senza RFQ, e l'operatore la apre con «Crea l'RFQ».
- Nel backend il thread nasce al triage `nuova_rfq`, quindi non esiste una richiesta senza thread.
- **Proposta:**
  - la richiesta del mockup è il thread in fase RICEVUTA;
  - «Crea l'RFQ» è il gesto che crea la cartella sul NAS (job `crea_cartella_thread`, `cartella_creata`) e porta il lavoro nella pagina RFQ.
- Va bene, o preferisci un'altra corrispondenza?

**R-03 · Il numero dell'RFQ.**
- Quando il cliente non dà un numero, il mockup ne crea uno nostro (`P-2026-0142`).
- Il backend ha solo `riferimento_cliente`.
- Va bene un progressivo interno? Oppure usiamo il numero SO dell'offerta quando c'è, e fino ad allora niente?

**R-04 · Sospendere un singolo mittente.**
- Nel mockup ogni mittente autorizzato ha un interruttore.
- `buyer` ha `confermato` ma non `attivo`: serve una colonna, o il significato è «non confermato»?

**R-05 · «Consegnata».**
- Nel catalogo delle fasi non c'è una consegna, e dopo PRODUZIONE non c'è niente.
- **Abbiamo tolto «consegnata» dal mockup:** un ordine evaso è un thread `CHIUSO` in fase PRODUZIONE.
- Se vuoi una fase di consegna, la rimettiamo.

## Gesti dello spazio di verifica (dopo A2), già previsti dal piano 6.0.7 e da E2 §3.3

**R-06 · Caricare un file dalla riga di un componente.**
- Oggi `fascicolo/carica` carica solo a livello di thread.
- Nel mockup ogni slot vuoto ha «Carica dal PC», che va dritto su quel componente e quel tipo.

**R-07 · Sotto un particolare o un commerciale non si mette niente.**
- Oggi l'editor promuove il particolare ad assieme quando gli si mette qualcosa sotto.
- Il mockup lo rifiuta con un messaggio, come la pagina Distinta del giro 4.

**R-08 · La conferma della categoria (minuteria).**
- Con R103 C la sola esenzione dal 2D è la minuteria confermata.
- Il mockup ha «Conferma minuteria» nel passo Documenti e nella scheda del pezzo, e la conferma fa parte del gesto cumulativo.
- Serve il gesto che la registri: oggi il legacy la confonde con `commerciale` (LD-19).

**R-09 · Il codice interno di un assieme aggiunto a mano.**
- Il mockup propone `<codice del prodotto>-A01`, modificabile.
- RC-01 propone alias sintetici come `codiceProdotto_P1` per le posizioni della BOM documentale.
- Un solo formato interno per tutti e due?

## Ciclo di produzione (Lino, fase FATTIBILITA): oggi non c'è niente

**R-10 · Il ciclo di un componente.** Sono tabelle nuove; i nomi sono solo una proposta:
- `ciclo` per componente: stato proposto o confermato, fonte, chi conferma e quando; materiale, semilavorato, dimensione, chi fornisce il materiale;
- `fase_ciclo`: l'ordine, la lavorazione del catalogo, interna o esterna, la spunta «in maschera» per le saldature, la norma, i filetti, le pieghe, la nota, e per le esterne il terzista, la specifica, le istruzioni e il materiale nostro o suo;
- `fase_componente`: quali figli e commerciali entrano in una fase, e in che quantità.
- Lo stato del ciclo diventa un dato della fase FATTIBILITA: «tutti i cicli confermati» potrebbe essere il fatto richiesto della transizione FATTIBILITA → SCHEDA_COSTO.

**R-11 · Le preparazioni, fuori dalla sequenza delle fasi.**
- Per ogni ciclo, le spunte «da realizzare» con una nota: maschera di saldatura, programma del robot, attrezzatura di piega, attrezzatura di curvatura, stampo, dima di foratura, calibro di controllo.
- Sono costi una tantum: servono a Franco.

**R-12 · Il catalogo delle lavorazioni.**
- `lavorazione` ha 13 codici.
- Mancano le lavorazioni interne della ricerca:
  - taglio: laser tubo, sega;
  - formatura: stampaggio;
  - lavorazioni meccaniche: foratura, filettatura, sbavatura;
  - saldatura: puntatura, distinzione robot, MIG/MAG, TIG;
  - assemblaggio: assemblaggio, montaggio, insertaggio, rivettatura, piantaggio;
  - trattamenti: verniciatura a liquido;
  - finitura: raddrizzatura, marcatura, controllo.
- Serve anche la categoria per il menu.
- Uno o due cataloghi (interno ed esterno)? È la vecchia P11.

**R-13 · La richiesta al terzista completa, e il suo storico.**
- A `richiesta_fornitore` mancano:
  - le istruzioni;
  - gli allegati scelti (il PDF del pezzo, altri dall'archivio, un PDF caricato) e «con le note stampate»;
  - il materiale nostro o suo;
  - per l'offerta ricevuta: prezzo, lotto, validità.
- Con questi dati lo storico «già fatta per questo codice o cliente» del mockup diventa una query, senza una tabella nuova.

**R-14 · Una nota sul disegno legata a una fase.**
- `annotazione_pdf` con una `fase_ciclo_id` facoltativa.
- Così la nota «fase 20 saldatura» si vede nell'albero e nel ciclo.

## Inbox (con AM, dopo il gate di A1c)

**R-15 · I prodotti del messaggio per il filtro della conversazione.**
- Il filtro «un prodotto alla volta» del mockup usa i prodotti che AM0 calcola per ogni messaggio, con il ruolo (`richiesta`, `contesto`, `menzione`).
- Ti chiediamo che il DTO dell'Inbox li esponga così, con la provenienza (messaggio e segmento).
- Il messaggio iniziale di una richiesta congiunta si vede in tutti i prodotti; quelli dopo solo dove servono (R107).

## La PR del frontend

**R-16 · Dove e come.** Te la proponiamo così:
- un ramo `frontend-mockup-v10` da `smistamento-giro5`;
- **solo file nuovi** in `cockpit/_fasi/frontend/` (accanto al tuo `_fasi/mockup_distinta.html`):
  - il mockup con **dati ACME**;
  - i sorgenti e il test che clicca tutto;
  - il contratto proposto e queste richieste.
- Nessun file esistente toccato: si fonde senza conflitti anche mentre B6 va avanti.
- Va lanciato `controlla-privati.ps1` prima del merge: il dataset privato ce l'hai tu.
- La vuoi così, o preferisci un'altra cartella?
