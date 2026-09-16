-- ============================================================================
--  0007_anagrafica.sql — Cockpit RFQ, blocco 3 dell'addendum v2 (voci 6.6, 6.11, 8.8)
--
--  Tre cose, e nessuna di più. L'anagrafica esiste dalla 0001 (`cliente`, `dominio_cliente`,
--  `buyer`, `fabbisogno_documento`): qui non si rifà, si completa nei punti in cui il blocco 3
--  chiede di scriverci dentro davvero.
--
--  Il numero: la 0006 aveva già dichiarato che l'anagrafica sarebbe diventata `0007_anagrafica`,
--  perché i numeri seguono l'ordine in cui le migrazioni vengono applicate e non l'ordine dei
--  blocchi del piano. Il `0007_igiene` della fase 7 del piano scalerà a valle.
--
--  Che cosa NON c'è qui, di proposito: `articolo`, `articolo_rev`, `distinta`, `distinta_proposta`
--  e `v_fascicolo_rfq` sono i blocchi 5 e 8. Anticiparne anche una colonna sola vorrebbe dire
--  scegliere oggi la forma di un modello che il blocco 5 deve ancora provare sui dati veri.
-- ============================================================================

-- ---------------------------------------------------------------- 1. cliente.peso (D27)
--
-- Il peso del cliente è un dato di ANAGRAFICA, non una proprietà della singola richiesta: «questo
-- cliente passa avanti» è una decisione commerciale che vale per tutte le sue RFQ, e ripeterla su
-- ogni richiesta significa dimenticarsene su quella nuova. Viene anticipato qui, dal blocco 5, perché
-- la lista delle Richieste lo usa già per ordinare.
--
-- Il dominio è 0–15 ed è un CHECK, non una convenzione scritta in un commento: un peso 150 battuto
-- per errore in un campo di testo non deve poter diventare l'ordine di lavoro di tutti.
-- 0 = nessuna priorità dichiarata, ed è il valore con cui nascono tutti: il peso lo mette una
-- persona, non un valore predefinito che finge una decisione mai presa.
ALTER TABLE cliente ADD COLUMN peso smallint NOT NULL DEFAULT 0 CHECK (peso BETWEEN 0 AND 15);

COMMENT ON COLUMN cliente.peso IS
    'Priorità commerciale del cliente, 0–15 (D27). Entra nell''ordinamento della lista Richieste. 0 = non dichiarata. NON è il punteggio della priorità dell''addendum 2: quella formula non esiste ancora.';

-- ---------------------------------------------------------------- 2. cliente.regole è un oggetto (D17)
--
-- `regole` è jsonb dalla 0001 e resta jsonb: i clienti sono una ventina, e una tabella di regole
-- generica per una ventina di righe costa più di quel che rende (D17, chiusa). Lo schema lo valida
-- il Go in scrittura — `domain.Regole` — e qui sotto resta il solo vincolo che il Go non può
-- garantire da solo: che quello che c'è dentro sia un OGGETTO. Un `[]` o un `"ciao"` passerebbero
-- il tipo jsonb e farebbero fallire ogni lettura successiva, che è il modo peggiore di scoprirlo.
ALTER TABLE cliente ADD CONSTRAINT cliente_regole_oggetto CHECK (jsonb_typeof(regole) = 'object');

COMMENT ON COLUMN cliente.regole IS
    'Regole di riconoscimento del cliente (D17): famiglie_codice, riferimento_rfq, canale_atteso, frasi_portale, lingua_risposta, richiede_cbd, numero_ordine_anticipato, finestra_aggancio_gg. Lo schema è `domain.Regole` e si valida in scrittura; ogni regola porta un esempio che deve corrispondere, altrimenti non viene usata.';

-- ---------------------------------------------------------------- 3. fabbisogno: da dove arriva
--
-- `fabbisogno_documento` dice gia' oggi «per questo cliente, per questo tipo di componente, questo
-- documento serve, e serve in modo bloccante», e regge meglio di quanto sembri: `v_fascicolo`
-- risolve il fabbisogno IN BLOCCO per `tipo_componente` — se un cliente scrive anche una sola
-- riga per «sciolto», per lui valgono le sue righe e NON piu' i default per «sciolto». Quindi
-- «questo documento a me non serve» e' gia' esprimibile: si scrive l'insieme del cliente senza
-- quella riga. Una colonna `richiesto` sarebbe un secondo modo di dire la stessa cosa, e due modi
-- di dire la stessa cosa finiscono sempre per dirne due diverse. Non si aggiunge.
--
-- La regola della risoluzione in blocco pero' non era scritta da nessuna parte: stava dentro un
-- LATERAL di sessanta righe, ed e' il genere di cosa che si scopre il giorno in cui un cliente
-- perde meta' del suo fascicolo per una riga aggiunta con le migliori intenzioni. Ora e' un
-- COMMENT, cioe' sta nel database accanto alla tabella che descrive.
COMMENT ON TABLE fabbisogno_documento IS
    'Che cosa deve esserci nel fascicolo perche'' la fattibilita'' possa iniziare. RISOLUZIONE IN BLOCCO: per un dato tipo_componente valgono le righe del cliente SE NE HA ALMENO UNA, altrimenti i default (cliente_id IS NULL). Non si mescolano: scrivere una riga per un cliente significa prendersi in carico tutte le righe di quel tipo_componente per quel cliente. E'' cosi'' che la risolve v_fascicolo, ed e'' cosi'' che la mostra Admin - Anagrafica.';

-- Quello che invece manca davvero: da DOVE ci si aspetta il documento.
--
-- Il fascicolo del blocco 10 deve poter dire perche' un documento non c'e' e chi lo porta. «Il DXF
-- lo facciamo noi» non e' un documento mancante: e' un documento che nessuno deve sollecitare.
-- «Il capitolato sta sul portale» non e' una mail da aspettare: e' un link su cui cliccare (la
-- cella azzurra del mockup). Oggi questa distinzione esiste solo per RFQ — `riferimento_portale`
-- nasce da una frase in una mail — e non come proprieta' del cliente, che e' quello che e':
-- «da questo cliente il capitolato passa sempre dal portale» vale per tutte le sue richieste.
--
-- Senza questa colonna le tre celle sono indistinguibili, e l'unico modo di distinguerle sarebbe
-- scrivere il nome del cliente dentro il Go: esattamente cio' che il blocco 3 vieta.
CREATE TYPE fonte_fabbisogno AS ENUM ('cliente', 'portale', 'promatec');

ALTER TABLE fabbisogno_documento ADD COLUMN fonte_attesa fonte_fabbisogno;

COMMENT ON COLUMN fabbisogno_documento.fonte_attesa IS
    'Da dove ci si aspetta il documento: cliente (arriva per mail e si sollecita), portale (link del cliente, cella azzurra del fascicolo), promatec (lo produciamo noi: non si sollecita). NULL = non dichiarato, ci si comporta come con «cliente».';

-- ---------------------------------------------------------------- 4. anagrafica non distruttiva (6.6)
--
-- Le due unicità che contano ci sono già (`cliente.cartella_nas UNIQUE`, `dominio_cliente.dominio`
-- PRIMARY KEY): a essere distruttive erano le QUERY, non lo schema. `UpsertCliente` faceva
-- `ON CONFLICT (cartella_nas) DO UPDATE`, cioè creare un cliente nuovo su una cartella già presa
-- RINOMINAVA quello esistente lasciandogli domini, buyer e RFQ; `UpsertDominioCliente` faceva
-- `DO UPDATE SET cliente_id`, cioè spostava un dominio da un cliente all'altro in silenzio.
-- Le sostituiscono `InsertCliente` e `InsertDominioCliente` (queries/anagrafica.sql), che lasciano
-- salire il conflitto. Qui resta solo il commento che dice perché quelle unicità sono unicità.
COMMENT ON COLUMN cliente.cartella_nas IS
    'Nome della cartella sotto PREVENTIVI DA FARE (es. MECCANICA NORD). UNIQUE: due clienti nella stessa cartella vorrebbe dire due clienti che si sovrascrivono i disegni sul NAS. Non si riusa con un ON CONFLICT DO UPDATE (voce 6.6, T7).';
COMMENT ON TABLE dominio_cliente IS
    'Dominio del mittente → cliente. Il dominio è la PRIMARY KEY: appartiene a UN cliente solo. Assegnarlo a un secondo cliente è un errore che deve arrivare a chi lo sta facendo, non una UPDATE silenziosa che cambia il riconoscimento di tutta la posta passata (voce 6.6, T8).';

INSERT INTO schema_versione (versione) VALUES (7);
