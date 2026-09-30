-- Il censimento delle forme (Smistamento, giro 4, fase 4.17a): per cliente, come sono fatti i codici, i nomi
-- dei file, gli oggetti e i riferimenti che arrivano, e che cosa ne hanno deciso le persone. Serve a scrivere
-- regole piu' precise (risposte 2, 5, 13 e 14 del 29/09) e a misurare prima e dopo ogni cambio dell'estrazione.
--
-- SOLA LETTURA: nessuna di queste query scrive, e il Go le esegue in una transazione READ ONLY
-- (registro/censimento.Leggi), cosi' che una scrittura, anche sbagliata, fallisca invece di passare. Le
-- mette insieme il Go: il cliente di una riga e' quello della RFQ in cui sta o, per un messaggio orfano, la
-- controparte che l'anagrafica ha risolto.
--
-- Il tipo di un pezzo si legge com'e' (c.tipo) e si confronta nel Go: il censimento conta le decisioni, e
-- nessuna query qui nomina un tipo di componente.
--
-- Niente migrazione: tutte le tabelle ci sono gia'.

-- name: CensimentoClienti :many
-- Tutti i clienti, anche quelli non piu' attivi: la loro posta vecchia resta nel database e fa parte delle
-- forme viste.
SELECT cliente_id, cartella_nas, ragione_sociale, regole, attivo FROM cliente ORDER BY cartella_nas;

-- name: CensimentoMessaggi :many
-- La posta che il cliente ha scritto: in entrata, non di un fornitore. Il corpo si legge fino a 4000
-- caratteri: riferimenti e codici stanno in testa, e il censimento non deve portarsi in memoria le catene di
-- risposta di anni. `allegati` sono i nomi dei file diretti, come li vede il triage.
--
-- `salvati` sono i codici che l'ingest ha trovato all'arrivo (candidato_codice, senza il riferimento della
-- richiesta), da confrontare con quelli che il motore di oggi trova negli stessi testi; `interpretato` dice se
-- l'ingest il messaggio l'ha letto (ha una proposta del triage deterministico). Con il corpo tagliato
-- (`corpo_troncato`) si confrontano solo i codici dell'oggetto e dei nomi degli allegati: quelli del corpo oltre
-- il taglio il motore di oggi non li vede, e sembrerebbero persi.
--
-- `estratto` dice se di quella lettura resta un'estrazione con cui confrontarsi: un esito diverso da «ignora»,
-- o almeno un candidato di codice salvato. Alcuni rami del triage rispondono «ignora» senza estrarre (la
-- controparte ambigua, la posta di un cliente che non e' di lavoro: classificazione.triageEsito) e non salvano
-- niente; nel database non si distinguono da un «ignora» che ha estratto e non ha trovato niente, e contarli
-- nel confronto farebbe sembrare nuovo ogni codice che il motore di oggi trova in quei testi. Il Go li conta a
-- parte.
SELECT m.messaggio_id,
       COALESCE(t.cliente_id, m.controparte_cliente_id)::uuid AS cliente_id,
       m.thread_id,
       COALESCE(m.oggetto, '')::text AS oggetto,
       left(COALESCE(m.corpo_testo, ''), 4000)::text AS corpo,
       (length(COALESCE(m.corpo_testo, '')) > 4000)::boolean AS corpo_troncato,
       lower(COALESCE(m.mittente_indirizzo, ''))::text AS mittente,
       (m.controparte_tipo = 'cliente' AND COALESCE(m.controparte_via = 'dominio', false))::boolean AS via_dominio,
       ARRAY(SELECT a.nome_file FROM allegato a
              WHERE a.messaggio_id = m.messaggio_id AND a.contenitore_id IS NULL AND a.natura = 'file'
              ORDER BY a.indice)::text[] AS allegati,
       EXISTS (SELECT 1 FROM proposta_triage p WHERE p.messaggio_id = m.messaggio_id AND p.fonte = 'deterministico')::boolean AS interpretato,
       EXISTS (SELECT 1 FROM proposta_triage p WHERE p.messaggio_id = m.messaggio_id AND p.fonte = 'deterministico'
                  AND (p.esito <> 'ignora' OR EXISTS (SELECT 1 FROM candidato_codice c WHERE c.messaggio_id = m.messaggio_id)))::boolean AS estratto,
       ARRAY(SELECT c.codice FROM candidato_codice c
              WHERE c.messaggio_id = m.messaggio_id AND c.ruolo <> 'riferimento_rfq'
                AND (length(COALESCE(m.corpo_testo, '')) <= 4000 OR c.evidenza = 'oggetto' OR c.evidenza LIKE 'allegato %')
              ORDER BY c.codice)::text[] AS salvati
  FROM messaggio m
  LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
 WHERE m.direzione = 'entrata' AND m.controparte_tipo <> 'fornitore'
   AND COALESCE(t.cliente_id, m.controparte_cliente_id) IS NOT NULL
 ORDER BY m.data_evento, m.messaggio_id;

-- name: CensimentoFile :many
-- I file che vengono dal cliente o dal progetto (la stessa condizione delle fonti del cliente di F5, in
-- proposte.sql), anche quelli estratti dagli archivi, con la decisione di una persona sullo stesso contenuto
-- nella stessa RFQ quando c'e' (un documento per (RFQ, sha256)) e con la proposta di file salvata (una per
-- allegato). Il contenuto (sha256, vuoto per un file non ancora scaricato) serve al Go: un file rimandato con lo
-- stesso nome e un contenuto nuovo (una revisione) e' un altro file, con la sua decisione.
SELECT a.allegato_id,
       COALESCE(t.cliente_id, m.controparte_cliente_id)::uuid AS cliente_id,
       m.thread_id,
       a.nome_file,
       COALESCE(a.sha256, '')::text AS sha256,
       (d.documento_id IS NOT NULL)::boolean AS deciso,
       COALESCE(d.codice, '')::text AS codice_deciso,
       COALESCE(d.rev, '')::text AS rev_decisa,
       (p.proposta_id IS NOT NULL)::boolean AS proposto,
       COALESCE(p.codice, '')::text AS codice_proposto,
       COALESCE(p.rev, '')::text AS rev_proposta
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
  LEFT JOIN documento d ON d.thread_id = m.thread_id AND d.sha256 = a.sha256
  LEFT JOIN documento_proposta p ON p.allegato_id = a.allegato_id
 WHERE a.natura = 'file'
   AND m.controparte_tipo <> 'fornitore' AND (m.direzione = 'entrata' OR m.canale = 'nota')
   AND COALESCE(t.cliente_id, m.controparte_cliente_id) IS NOT NULL
 ORDER BY a.allegato_id;

-- name: CensimentoNodi :many
-- I PRODUCT degli STEP proposti nelle RFQ, grezzi come li ha letti il worker: l'id e il nome.
SELECT p.thread_id, t.cliente_id, p.id_grezzo, p.nome_grezzo
  FROM componente_proposta p
  JOIN thread_offerta t ON t.thread_id = p.thread_id
 ORDER BY p.thread_id, p.allegato_id, p.chiave;

-- name: CensimentoCartigli :many
-- I fatti del testo dei PDF del cliente (analizzatore 4, `testo_pdf`) che portano campi del probabile
-- cartiglio, ancora grezzi: li legge la classificazione (CampiIdentificativiDelPDF, con il controllo della
-- versione), perche' i fatti del worker si leggono solo li' (F9). Qui si tolgono soltanto i frammenti, che
-- sono il grosso del testo e il censimento non guarda; il filtro sull'elenco dei campi non porta nel Go i PDF
-- che non ne hanno. Uno stesso contenuto analizzato con piu' configurazioni ha piu' righe di fatti, e lo stesso
-- file arrivato due volte nella stessa RFQ ne ha una: DISTINCT, e il Go conta ogni lettura una volta per RFQ,
-- contenuto e fonte. L'ordine e' fisso, come quello delle altre letture.
SELECT DISTINCT COALESCE(t.cliente_id, m.controparte_cliente_id)::uuid AS cliente_id,
       m.thread_id,
       a.sha256::text AS sha256,
       jsonb_build_object('testo_pdf', (f.fatti -> 'testo_pdf') - 'frammenti')::jsonb AS fatti
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
  JOIN analisi_fatti f ON f.sha256 = a.sha256
 WHERE a.natura = 'file'
   AND m.controparte_tipo <> 'fornitore' AND (m.direzione = 'entrata' OR m.canale = 'nota')
   AND COALESCE(t.cliente_id, m.controparte_cliente_id) IS NOT NULL
   AND jsonb_typeof(f.fatti -> 'testo_pdf' -> 'cartiglio') = 'array'
 ORDER BY 1, 2, 3, 4;

-- name: CensimentoPezzi :many
-- I pezzi decisi da una persona (componente, A2.4: nessuno nasce da un automatismo), con il tipo che ha
-- scelto e i nomi con cui sono arrivati: la descrizione e il PRODUCT dei nodi che la persona ha accettato o
-- riconciliato con il pezzo. Non quelli archiviati: la persona li ha tolti dalla distinta (A4.9: un nodo
-- sbagliato, un doppione), e contarli metterebbe fra le decisioni proprio quello che ha scartato. Come le
-- altre letture della BOM (bom.sql, struttura.sql, panoramica.sql).
SELECT c.componente_id,
       c.thread_id,
       t.cliente_id,
       c.codice,
       COALESCE(c.rev, '')::text AS rev,
       c.tipo,
       COALESCE(c.descrizione, '')::text AS descrizione,
       ARRAY(SELECT DISTINCT p.nome_grezzo FROM componente_proposta p
              WHERE p.componente_id = c.componente_id AND p.stato IN ('confermata', 'duplicato')
              ORDER BY p.nome_grezzo)::text[] AS nomi
  FROM componente c
  JOIN thread_offerta t ON t.thread_id = c.thread_id
 WHERE c.archiviato_il IS NULL
 ORDER BY c.thread_id, c.codice;

-- name: CensimentoCodiciDecisi :many
-- Gli altri codici decisi da una persona: i prodotti della richiesta confermati e i codici dei documenti
-- confermati.
SELECT i.thread_id, t.cliente_id, i.codice::text AS codice
  FROM identificativo_thread i
  JOIN thread_offerta t ON t.thread_id = i.thread_id
 WHERE i.confermato_da IS NOT NULL
UNION
SELECT d.thread_id, t.cliente_id, d.codice::text AS codice
  FROM documento d
  JOIN thread_offerta t ON t.thread_id = d.thread_id
 WHERE btrim(COALESCE(d.codice, '')) <> ''
 ORDER BY 1, 3;

-- name: CensimentoDominiNonCensiti :many
-- I domini dei mittenti che l'anagrafica non conosce (controparte sconosciuta o ambigua), in entrata: sono
-- i clienti, i fornitori e i sistemi che nessuno ha ancora censito.
SELECT lower(split_part(m.mittente_indirizzo, '@', 2))::text AS dominio, count(*)::int AS n
  FROM messaggio m
 WHERE m.direzione = 'entrata' AND m.controparte_tipo IN ('sconosciuto', 'ambiguo')
   AND m.mittente_indirizzo LIKE '%_@_%'
 GROUP BY 1
 ORDER BY n DESC, dominio
 LIMIT 100;
