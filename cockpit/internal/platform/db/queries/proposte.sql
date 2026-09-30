-- Le proposte di struttura (Blocco 8, B8.5): dai fatti di uno STEP ai nodi, agli archi e alle
-- rimozioni proposti per una RFQ (addendum A1.1, A1.2, A4.4).
--
-- Tre strati, e qui se ne scrive uno solo: i FATTI stanno in analisi_fatti e non si toccano, le
-- DECISIONI stanno in componente e componente_relazione e le prende una persona. Queste query scrivono
-- l'interpretazione. Una proposta gia' decisa non si riscrive: gli upsert cambiano solo le righe aperte.

-- name: ListComponenteProposteFile :many
SELECT * FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 ORDER BY chiave;

-- name: ListRelazioneProposteFile :many
SELECT * FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2 ORDER BY padre_chiave, figlio_chiave;

-- name: ListComponenteProposteStessoFile :many
-- I nodi proposti da un contenuto in una RFQ, qualunque allegato li porti: servono a riconoscere nello
-- STEP strutturale i nodi gia' decisi.
SELECT * FROM componente_proposta WHERE thread_id = $1 AND sha256 = $2 ORDER BY chiave, creato_il;

-- name: PortatoreDelFile :one
-- L'allegato che in questa RFQ porta le proposte di un contenuto. Lo stesso file arrivato due volte
-- nella stessa RFQ propone una volta sola: due serie di nodi uguali sarebbero doppio lavoro e doppio
-- conteggio nel gate.
SELECT allegato_id FROM componente_proposta WHERE thread_id = $1 AND sha256 = $2
ORDER BY creato_il, allegato_id LIMIT 1;

-- name: UpsertComponenteProposta :exec
-- Un nodo proposto. Si riscrive solo finche' e' aperto; un codice scritto a mano dall'operatore
-- (origine_codice = 'operatore') resta, anche se le regole del cliente cambiano: la riclassificazione
-- vale per cio' che ha classificato il server.
--
-- Smistamento F5 (E33, P30). Si riscrive anche una riga chiusa da un automatismo (scartata senza chi l'ha
-- decisa: un file sostituito, un riferimento cambiato): nessuno l'ha decisa, e la lettura di adesso la
-- riapre. Una riga decisa da una persona resta com'e'. La storia della riga (evidenza.storia: una revoca,
-- una riapertura) sopravvive alla rilettura: l'evidenza nuova e' quella del file, la storia e' della riga.
INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, descrizione, codice, rev,
                                 origine_codice, famiglia, tipo_proposto, fonte, confidenza, evidenza, stato, componente_id, nota, deciso_il)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(sha256), sqlc.arg(chiave), sqlc.arg(nome_grezzo), sqlc.arg(id_grezzo),
        sqlc.narg(descrizione), sqlc.narg(codice), sqlc.narg(rev), sqlc.narg(origine_codice), sqlc.arg(famiglia),
        sqlc.narg(tipo_proposto), 'step', sqlc.arg(confidenza), sqlc.arg(evidenza), sqlc.arg(stato), sqlc.narg(componente_id),
        sqlc.narg(nota), CASE WHEN sqlc.arg(stato)::stato_proposta = 'aperta' THEN NULL ELSE now() END)
ON CONFLICT (thread_id, allegato_id, chiave) DO UPDATE SET
    sha256         = EXCLUDED.sha256,
    nome_grezzo    = EXCLUDED.nome_grezzo,
    id_grezzo      = EXCLUDED.id_grezzo,
    descrizione    = EXCLUDED.descrizione,
    codice         = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.codice ELSE EXCLUDED.codice END,
    rev            = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.rev ELSE EXCLUDED.rev END,
    origine_codice = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.origine_codice ELSE EXCLUDED.origine_codice END,
    famiglia       = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.famiglia ELSE EXCLUDED.famiglia END,
    confidenza     = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.confidenza ELSE EXCLUDED.confidenza END,
    tipo_proposto  = EXCLUDED.tipo_proposto,
    evidenza       = CASE WHEN componente_proposta.evidenza ? 'storia'
                          THEN EXCLUDED.evidenza || jsonb_build_object('storia', componente_proposta.evidenza -> 'storia')
                          ELSE EXCLUDED.evidenza END,
    stato          = EXCLUDED.stato,
    componente_id  = EXCLUDED.componente_id,
    nota           = EXCLUDED.nota,
    deciso_il      = EXCLUDED.deciso_il
WHERE componente_proposta.stato = 'aperta'
   OR (componente_proposta.stato = 'scartata' AND componente_proposta.deciso_da IS NULL);

-- name: UpsertRelazioneProposta :exec
-- Un arco proposto, una riga per coppia (padre, figlio) del file. Solo finche' e' aperto, o chiuso da un
-- automatismo (scartato senza chi l'ha deciso: E33); la storia dell'arco resta (P30).
INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta, evidenza, stato, nota, deciso_il)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(padre_chiave), sqlc.arg(figlio_chiave), sqlc.arg(qta),
        sqlc.arg(evidenza), sqlc.arg(stato), sqlc.narg(nota), CASE WHEN sqlc.arg(stato)::stato_proposta = 'aperta' THEN NULL ELSE now() END)
ON CONFLICT (thread_id, allegato_id, padre_chiave, figlio_chiave) DO UPDATE SET
    qta       = EXCLUDED.qta,
    evidenza  = CASE WHEN relazione_proposta.evidenza ? 'storia'
                     THEN EXCLUDED.evidenza || jsonb_build_object('storia', relazione_proposta.evidenza -> 'storia')
                     ELSE EXCLUDED.evidenza END,
    stato     = EXCLUDED.stato,
    nota      = EXCLUDED.nota,
    deciso_il = EXCLUDED.deciso_il
WHERE relazione_proposta.stato = 'aperta'
   OR (relazione_proposta.stato = 'scartata' AND relazione_proposta.deciso_da IS NULL);

-- name: BloccaComponenteProposta :one
SELECT * FROM componente_proposta WHERE proposta_id = $1 FOR UPDATE;

-- name: DecidiComponenteProposta :execrows
UPDATE componente_proposta SET stato = sqlc.arg(stato), componente_id = sqlc.narg(componente_id),
       deciso_da = sqlc.narg(deciso_da), deciso_il = now()
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta';

-- name: ConfermaNodoAgganciato :execrows
-- Un nodo agganciato per codice da un automatismo di prima dello Smistamento (duplicato senza chi l'ha
-- deciso, forma F-A) che sta nell'autorita' di un file autorizzato: una persona lo conferma come quel
-- componente (Smistamento F5, A5.4.8: il gate lo conta da decidere finche' qualcuno non lo fa). Da qui e'
-- una decisione, con la storia di com'era nato.
UPDATE componente_proposta SET deciso_da = sqlc.arg(deciso_da)::uuid, deciso_il = now(),
       evidenza = evidenza || jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                  jsonb_build_object('evento', 'aggancio_per_codice_confermato', 'agganciato_il', deciso_il,
                                     'confermato_da', sqlc.arg(deciso_da)::uuid, 'confermato_il', now())))
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'duplicato' AND deciso_da IS NULL AND componente_id IS NOT NULL;

-- name: SetCodiceComponenteProposta :execrows
-- Il codice lo scrive l'operatore, su un nodo che il server non ha saputo classificare (A1.2): origine
-- 'operatore', e da qui una riclassificazione non lo tocca piu'.
--
-- Smistamento F5 (K2, E34): la classificazione del server non si perde. Ogni lettura la scrive in
-- evidenza.classificato; una riga letta prima che ci fosse la riceve qui, prima che il codice
-- dell'operatore prenda il posto di quello del server. E' una correzione dell'evidenza, non della BOM.
UPDATE componente_proposta SET codice = sqlc.arg(codice), rev = sqlc.narg(rev), origine_codice = 'operatore',
       famiglia = '', confidenza = 100,
       evidenza = CASE WHEN evidenza ? 'classificato' OR origine_codice = 'operatore' THEN evidenza
                       ELSE evidenza || jsonb_build_object('classificato', jsonb_build_object(
                            'codice', COALESCE(codice, ''), 'rev', COALESCE(rev, ''), 'origine', COALESCE(origine_codice::text, ''),
                            'famiglia', famiglia, 'confidenza', confidenza)) END
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta';

-- name: RiapriComponenteProposta :execrows
-- «Riapri il nodo» (Smistamento F5, A5.4.5): un nodo scartato torna aperto, con la storia di chi l'aveva
-- scartato e di chi lo riapre. E' una correzione dell'evidenza (vale per un nodo di guida come per uno
-- nell'autorita'), non tocca la BOM. Un nodo accettato non si riapre da qui: ha fatto nascere o ritrovato
-- un componente, e quella e' una decisione sulla working.
--
-- Giro 4, fase 4.4a.1b: un nodo tolto nell'albero confermato porta il segno evidenza.albero (DecidiNodoNellAlbero)
-- e la nota «tolto nell'albero confermato»; riaperto non e' piu' una decisione, e il segno e la nota vanno nella
-- storia con lo scarto (la nota di una riga senza il segno resta com'e'). Gli archi che la stessa conferma aveva
-- chiuso li riapre RiapriArchiToltiNellAlbero.
UPDATE componente_proposta SET stato = 'aperta', componente_id = NULL, deciso_da = NULL, deciso_il = NULL,
       nota = CASE WHEN evidenza ? 'albero' THEN NULL ELSE nota END,
       evidenza = (evidenza - 'albero') || jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                  jsonb_build_object('evento', 'nodo_riaperto', 'scartato_da', deciso_da, 'scartato_il', deciso_il,
                                     'albero', evidenza -> 'albero', 'nota', nota,
                                     'riaperto_da', sqlc.arg(utente)::uuid, 'riaperto_il', now())))
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'scartata';

-- name: RiapriArchiToltiNellAlbero :execrows
-- «Riapri il nodo» su un nodo che la conferma dell'albero ha tolto (giro 4, fase 4.4a.1b; studio
-- docs/specs/studio_albero_distinta_29-09.md § 2.5: «RiapriNodo li riporta»). La stessa conferma aveva chiuso,
-- scartate da una persona con la nota e il segno, anche le righe degli archi dello stesso file che toccano il nodo:
-- senza riaprirle il nodo non si raggiunge piu' da un prodotto (le strutture saltano gli archi scartati da una
-- persona) e non torna nell'albero. Si riaprono solo quelle chiuse da quella conferma (la stessa firma del riepilogo
-- nel segno, la stessa nota): uno scarto a mano resta. La decisione, il segno e la nota vanno nella storia.
UPDATE relazione_proposta SET stato = 'aperta', deciso_da = NULL, deciso_il = NULL, nota = NULL,
       evidenza = (evidenza - 'albero') || jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                  jsonb_build_object('evento', 'arco_riaperto', 'scartato_da', deciso_da, 'scartato_il', deciso_il,
                                     'albero', evidenza -> 'albero', 'nota', nota,
                                     'riaperto_da', sqlc.arg(utente)::uuid, 'riaperto_il', now())))
WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id)
  AND (padre_chiave = sqlc.arg(chiave) OR figlio_chiave = sqlc.arg(chiave))
  AND stato = 'scartata' AND deciso_da IS NOT NULL AND nota = sqlc.arg(nota)::text
  AND evidenza -> 'albero' ->> 'firma' = sqlc.arg(firma)::text;

-- name: DecidiNodoNellAlbero :execrows
-- La conferma dell'albero (giro 4, fase 4.4a.1b; studio docs/specs/studio_albero_distinta_29-09.md § 2.9): la riga di
-- un nodo che l'albero confermato copre diventa la decisione di una persona, con gli stati di sempre (confermata: ha
-- fatto nascere il componente; duplicato: lo ritrova; scartata: tolta nell'albero, con la nota) e con il segno
-- «confermato nell'albero» in evidenza.albero (chi, quando, la firma del riepilogo; il ✓ o il ✗ della proposta
-- commerciale). Senza migrazione: evidenza c'e' gia', e una riga decisa da una persona non si riscrive piu' (le
-- letture riscrivono solo le righe aperte o chiuse da un automatismo: UpsertComponenteProposta). Si decide una riga
-- aperta, o l'aggancio per codice di prima (duplicato con il componente e senza chi l'ha deciso: AgganciatoPerCodice,
-- che il riepilogo elenca), e com'era va nella storia, con il segno di una conferma di prima se c'era. Una riga gia'
-- decisa da una persona si tocca solo con anche_decise: quando la conferma di adesso toglie un pezzo che una decisione
-- di prima aveva preso, e' una decisione nuova di una persona sulla stessa riga, e quella di prima va nella storia; una
-- riga gia' scartata da una persona resta com'e'.
--
-- Fase 4.4a.1br. Una riga chiusa da un automatismo (senza chi l'ha decisa, e non un aggancio: scartata per un file
-- sostituito o un riferimento cambiato) resta com'e', anche con anche_decise: il riepilogo non la conta, e deve restare
-- una riga che la rilettura riscrive (E33). Prima la conferma la faceva diventare lo scarto di una persona. La nota: una
-- nota vuota lascia quella della riga (la lettura puo' averne scritta una su una riga aperta), come
-- DecidiRelazioneProposta; una nota nuova sulla nota di una riga aperta la manda nella storia.
UPDATE componente_proposta SET stato = sqlc.arg(stato), componente_id = sqlc.narg(componente_id),
       deciso_da = sqlc.arg(deciso_da)::uuid, deciso_il = now(), nota = COALESCE(sqlc.narg(nota)::text, nota),
       evidenza = evidenza || jsonb_build_object('albero', sqlc.arg(segno)::jsonb) ||
                  CASE WHEN stato <> 'aperta'
                       THEN jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                            jsonb_build_object('evento', 'deciso_nell_albero', 'stato', stato, 'componente_id', componente_id,
                                               'deciso_da', deciso_da, 'deciso_il', deciso_il, 'nota', nota,
                                               'albero', evidenza -> 'albero', 'il', now())))
                       WHEN nota IS NOT NULL AND sqlc.narg(nota)::text <> nota
                       THEN jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                            jsonb_build_object('evento', 'nota_della_lettura', 'nota', nota, 'il', now())))
                       ELSE '{}'::jsonb END
WHERE proposta_id = sqlc.arg(proposta_id)
  AND (stato = 'aperta'
       OR (stato = 'duplicato' AND deciso_da IS NULL AND componente_id IS NOT NULL)
       OR (sqlc.arg(anche_decise)::bool AND deciso_da IS NOT NULL AND stato <> 'scartata'));

-- name: DecidiArcoNellAlbero :execrows
-- La riga di un arco che l'albero confermato copre (fase 4.4a.1b), come DecidiNodoNellAlbero: confermata o duplicato
-- per un legame che l'albero tiene, scartata con la nota per uno che la conferma toglie. Il segno evidenza.albero porta
-- anche i due componenti del legame (padre, figlio): la radice del prodotto non e' una riga decisa, e il segno dice lo
-- stesso quale arco della working la persona ha confermato. anche_decise come per i nodi: un legame tolto adesso che
-- una decisione di prima aveva preso.
--
-- Fase 4.4a.1br, come per i nodi: si decide solo una riga aperta (o, con anche_decise, una decisa da una persona che
-- non e' uno scarto). Una chiusa da un automatismo — la «tenuta dei conti» di Pianifica (duplicato senza chi l'ha
-- deciso), lo stesso componente, un file sostituito — resta com'e', di un legame tenuto come di uno tolto: l'albero
-- porta le righe degli archi senza lo stato, e la guardia sta qui. La nota della lettura su una riga aperta («qta
-- diversa: …») resta con una nota vuota, e va nella storia se la conferma ne scrive un'altra.
UPDATE relazione_proposta SET stato = sqlc.arg(stato), nota = COALESCE(sqlc.narg(nota)::text, nota), deciso_da = sqlc.arg(deciso_da)::uuid,
       deciso_il = now(),
       evidenza = evidenza || jsonb_build_object('albero', sqlc.arg(segno)::jsonb) ||
                  CASE WHEN stato <> 'aperta'
                       THEN jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                            jsonb_build_object('evento', 'deciso_nell_albero', 'stato', stato, 'deciso_da', deciso_da,
                                               'deciso_il', deciso_il, 'nota', nota, 'albero', evidenza -> 'albero', 'il', now())))
                       WHEN nota IS NOT NULL AND sqlc.narg(nota)::text <> nota
                       THEN jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                            jsonb_build_object('evento', 'nota_della_lettura', 'nota', nota, 'il', now())))
                       ELSE '{}'::jsonb END
WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id) AND padre_chiave = sqlc.arg(padre_chiave)
  AND figlio_chiave = sqlc.arg(figlio_chiave)
  AND (stato = 'aperta' OR (sqlc.arg(anche_decise)::bool AND deciso_da IS NOT NULL AND stato <> 'scartata'));

-- name: BloccaRelazioneProposta :one
SELECT * FROM relazione_proposta
WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = $3 AND figlio_chiave = $4 FOR UPDATE;

-- name: DecidiRelazioneProposta :execrows
UPDATE relazione_proposta SET stato = sqlc.arg(stato), nota = COALESCE(sqlc.narg(nota), nota),
       deciso_da = sqlc.narg(deciso_da), deciso_il = now()
WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id)
  AND padre_chiave = sqlc.arg(padre_chiave) AND figlio_chiave = sqlc.arg(figlio_chiave) AND stato = 'aperta';

-- Le fonti del cliente (Smistamento F5, P27, E26, E27). Un file entra nella lettura della struttura di una
-- RFQ solo se viene dal cliente o dal progetto: non da un messaggio di un fornitore, non da una nostra mail
-- in uscita (un caricamento interno sta nel canale `nota`, ed e' del progetto), e non se una persona l'ha
-- escluso (la sua proposta di documento scartata con chi l'ha decisa). Gli altri file restano visibili e si
-- analizzano lo stesso; la loro struttura non diventa ne' guida ne' autorita'. La stessa condizione, scritta
-- tre volte qui sotto: una vista sarebbe una migrazione.

-- name: ListRfqAperteConContenuto :many
-- Ogni allegato con questo contenuto in una RFQ aperta, non unita a un'altra, da una fonte del cliente e
-- non escluso: i fatti di uno STEP diventano proposte in ognuna (A1.2, fan-out D50, A5.4.10), ciascuna con
-- le regole del suo cliente. Prima (ListAllegatiStessoFileConRfq) erano tutte le RFQ, anche chiuse o unite,
-- con i file dei fornitori.
SELECT sqlc.embed(a), m.thread_id AS rfq_id
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  JOIN thread_offerta t ON t.thread_id = m.thread_id
 WHERE a.sha256 = $1 AND t.unito_in IS NULL AND t.stato = 'APERTA'
   AND m.controparte_tipo <> 'fornitore' AND (m.direzione = 'entrata' OR m.canale = 'nota')
   AND NOT EXISTS (SELECT 1 FROM documento_proposta p
                    WHERE p.allegato_id = a.allegato_id AND p.stato = 'scartata' AND p.deciso_da IS NOT NULL)
 ORDER BY m.thread_id, a.ricevuto_il, a.allegato_id;

-- name: ListStepDellaRfq :many
-- Gli STEP arrivati in una RFQ e scaricati (lo SHA si sa) da cui la struttura si legge: da una fonte del
-- cliente e non esclusi (Smistamento F5, E26). Uno STEP escluso non si autorizza, e resta inerte.
SELECT a.* FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = $1 AND lower(a.estensione) IN ('stp', 'step') AND a.sha256 IS NOT NULL
   AND m.controparte_tipo <> 'fornitore' AND (m.direzione = 'entrata' OR m.canale = 'nota')
   AND NOT EXISTS (SELECT 1 FROM documento_proposta p
                    WHERE p.allegato_id = a.allegato_id AND p.stato = 'scartata' AND p.deciso_da IS NOT NULL)
 ORDER BY a.ricevuto_il, a.allegato_id;

-- name: ListStepDaAnalizzareDellaRfq :many
-- Tutti gli STEP della RFQ scaricati, per l'analisi: tutti gli STEP si analizzano, sempre (decisione
-- dell'utente del 27/09), anche quelli esclusi o di un fornitore. L'analisi e' un fatto del contenuto; e'
-- la lettura della struttura nella RFQ (ListStepDellaRfq) che li lascia fuori.
SELECT a.* FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = $1 AND lower(a.estensione) IN ('stp', 'step') AND a.sha256 IS NOT NULL
 ORDER BY a.ricevuto_il, a.allegato_id;

-- name: FileNelFlusso :one
-- L'allegato e' di una fonte del cliente e non e' escluso: la sua struttura si legge nella RFQ. La
-- guardia di ApplicaStruttura, per chi ci arriva senza passare dalle due query sopra (i fatti riusati
-- dopo uno stage).
SELECT (m.controparte_tipo <> 'fornitore' AND (m.direzione = 'entrata' OR m.canale = 'nota')
        AND NOT EXISTS (SELECT 1 FROM documento_proposta p
                         WHERE p.allegato_id = a.allegato_id AND p.stato = 'scartata' AND p.deciso_da IS NOT NULL))::bool AS nel_flusso
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE a.allegato_id = $1;

-- name: GetAnalisiCorrente :one
-- I fatti di un contenuto con la chiave dell'analizzatore corrente (A4.5). Nessuna riga se
-- analizzatore_corrente e' vuota: nessuna analisi e' corrente, ed e' il lato che non concede.
SELECT af.* FROM analisi_fatti af
  JOIN analizzatore_corrente ac ON ac.versione_analizzatore = af.versione_analizzatore
                               AND ac.hash_configurazione = af.hash_configurazione
 WHERE af.sha256 = $1;

-- name: MotivoParziale :one
-- Perche' una struttura non e' completa, con la definizione della 0020 (struttura_motivo_parziale):
-- '' = completa. Il Go la chiede al database invece di ricalcolarla (A4.4: la definizione e' una).
SELECT COALESCE(struttura_motivo_parziale(sqlc.narg(struttura)::jsonb), '')::text AS motivo;

-- name: ListDichiarazioniRfq :many
-- Le sorgenti delle autorizzazioni della RFQ (Smistamento F5, A5.4.3): «questo STEP e' autorizzato a
-- proporre i figli diretti di C». Due forme, in una riga per sorgente:
--   - smistamento: la riga di un nodo che una persona ha deciso come C, con la marcatura
--     evidenza.strutturale (radice o raggruppamento);
--   - step_strutturale_id: la forma di prima dello Smistamento, lo STEP strutturale di un finito con le
--     radici del suo file, riconosciute dalla relazione e non dalla nota (K1: le righe dello sha del
--     documento senza un arco entrante nel file), qualunque sia il loro stato. Una radice ancora aperta
--     resta aperta: e' sorgente solo qui, e chi ne ha bisogno prende C da questa riga.
-- La validita' (un componente, un'autorizzazione; superata; incoerente; P7; sospesa) la decide il Go, in
-- una funzione pura (ValutaDichiarazioni): il predicato e' uno solo. Per questo la query porta anche il
-- componente, il documento del file nella RFQ e lo sha dello STEP strutturale di C (la coerenza fra la
-- marcatura e la colonna di un finito, I10).
--
-- Smistamento F5b (Domanda 1 = B, A5.4.6): una delega vale solo se il suo nodo e' figlio diretto, nel file,
-- di una sorgente valida dello stesso file, fino a una radice o a un raggruppamento. Per questo ogni riga
-- porta anche i padri del suo nodo nel file (padri): la catena la controlla il Go.
WITH riga AS (
    SELECT cp.proposta_id, cp.allegato_id, cp.sha256, cp.chiave, cp.stato, cp.componente_id AS riga_componente_id,
           cp.deciso_da, cp.creato_il, (cp.evidenza -> 'strutturale')::jsonb AS marcatura, cp.componente_id AS dichiarato_per,
           'smistamento'::text AS origine
      FROM componente_proposta cp
     WHERE cp.thread_id = sqlc.arg(thread_id) AND cp.stato IN ('confermata', 'duplicato') AND cp.evidenza ? 'strutturale'
    UNION ALL
    SELECT cp.proposta_id, cp.allegato_id, cp.sha256, cp.chiave, cp.stato, cp.componente_id,
           cp.deciso_da, cp.creato_il, NULL::jsonb, c.componente_id,
           'step_strutturale_id'::text
      FROM componente c
      JOIN documento sd ON sd.documento_id = c.step_strutturale_id
      JOIN componente_proposta cp ON cp.thread_id = c.thread_id AND cp.sha256 = sd.sha256
     WHERE c.thread_id = sqlc.arg(thread_id) AND NOT (cp.evidenza ? 'strutturale')
       AND NOT EXISTS (SELECT 1 FROM relazione_proposta r
                        WHERE r.thread_id = cp.thread_id AND r.allegato_id = cp.allegato_id AND r.figlio_chiave = cp.chiave)
)
SELECT r.proposta_id, r.allegato_id, r.sha256, r.chiave, r.stato, r.riga_componente_id, r.deciso_da, r.creato_il,
       r.marcatura, r.origine, al.nome_file, sqlc.embed(c), ss.sha256 AS step_sha256,
       d.documento_id AS documento_id, d.componente_id AS documento_componente_id, d.sostituito_da AS documento_sostituito_da,
       ARRAY(SELECT rp.padre_chiave FROM relazione_proposta rp
              WHERE rp.thread_id = sqlc.arg(thread_id) AND rp.allegato_id = r.allegato_id AND rp.figlio_chiave = r.chiave
              ORDER BY rp.padre_chiave)::text[] AS padri
  FROM riga r
  JOIN allegato al ON al.allegato_id = r.allegato_id
  JOIN componente c ON c.componente_id = r.dichiarato_per
  LEFT JOIN documento ss ON ss.documento_id = c.step_strutturale_id
  LEFT JOIN documento d ON d.thread_id = c.thread_id AND d.sha256 = r.sha256
 ORDER BY c.codice, r.sha256, r.origine, r.creato_il, r.allegato_id, r.chiave;

-- name: RiapriArchiAutomaticiDiUnFile :execrows
-- Gli archi di un file chiusi da un automatismo (duplicato o scartato senza chi li ha decisi: la tenuta dei
-- conti con la working, lo «stesso componente») tornano aperti, con la storia (Smistamento F5, E33). Serve
-- alla revoca di un'autorizzazione (F5b): senza, resterebbero chiusure che nessuno ha deciso su un file
-- che non ha piu' autorita'. Gli archi decisi da una persona restano: sono decisioni.
UPDATE relazione_proposta r SET stato = 'aperta', deciso_il = NULL, nota = NULL,
       evidenza = r.evidenza || jsonb_build_object('storia', COALESCE(r.evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                  jsonb_build_object('evento', 'arco_automatico_riaperto', 'stato', r.stato, 'nota', r.nota, 'il', now())))
  FROM allegato a
 WHERE a.allegato_id = r.allegato_id AND a.sha256 = sqlc.arg(sha256) AND r.thread_id = sqlc.arg(thread_id)
   AND r.deciso_da IS NULL AND r.stato IN ('duplicato', 'scartata');

-- name: MarcaDichiarazione :execrows
-- La marcatura dell'autorizzazione (Smistamento F5b, A5.4.2): la riga del nodo sorgente, gia' decisa da una
-- persona come il componente, riceve evidenza.strutturale (ruolo, componente, documento, chi, quando, presa
-- d'atto). Una marcatura che c'era gia' (l'autorizzazione rifatta, una sospensione riattivata) non si perde:
-- va nella storia della riga. La scrive solo DichiaraStrutturale.
UPDATE componente_proposta SET evidenza = CASE WHEN evidenza ? 'strutturale'
           THEN evidenza || jsonb_build_object('strutturale', sqlc.arg(marcatura)::jsonb,
                'storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
                          'evento', 'autorizzazione_rifatta', 'marcatura', evidenza -> 'strutturale', 'il', now())))
           ELSE evidenza || jsonb_build_object('strutturale', sqlc.arg(marcatura)::jsonb) END
WHERE proposta_id = sqlc.arg(proposta_id) AND stato IN ('confermata', 'duplicato')
  AND componente_id = sqlc.arg(componente_id) AND deciso_da IS NOT NULL;

-- name: RevocaDichiarazione :execrows
-- La revoca di una marcatura (Smistamento F5b, A5.4.7): evidenza.strutturale va nella storia della riga,
-- con l'evento, chi, quando e perche'. Con riapri (la revoca di una radice o di un raggruppamento) la riga
-- torna aperta, senza componente: nessuno l'ha decisa se non per autorizzare il file. Senza (una delega: il
-- nodo resta il componente che una persona ha deciso; una sostituzione: la radice e' davvero quel componente
-- nella revisione di prima, evento sostituita_da) la riga resta decisa com'e'.
UPDATE componente_proposta SET
       stato         = CASE WHEN sqlc.arg(riapri)::bool THEN 'aperta'::stato_proposta ELSE stato END,
       componente_id = CASE WHEN sqlc.arg(riapri)::bool THEN NULL ELSE componente_id END,
       deciso_da     = CASE WHEN sqlc.arg(riapri)::bool THEN NULL ELSE deciso_da END,
       deciso_il     = CASE WHEN sqlc.arg(riapri)::bool THEN NULL ELSE deciso_il END,
       evidenza      = (evidenza - 'strutturale') || jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) ||
                       jsonb_build_array(jsonb_build_object('evento', sqlc.arg(evento)::text, 'marcatura', evidenza -> 'strutturale',
                                         'stato', stato, 'componente_id', componente_id, 'deciso_da', deciso_da,
                                         'revocata_da', sqlc.narg(revocata_da)::uuid, 'revocata_il', now(),
                                         'motivo', sqlc.arg(motivo)::text)))
WHERE proposta_id = sqlc.arg(proposta_id) AND evidenza ? 'strutturale';

-- name: SospendiDichiarazione :execrows
-- La sospensione di una marcatura (Smistamento, fase T; precisazione dell'utente del 27/09 sera): il componente
-- e' diventato commerciale, e la sua autorizzazione si ferma senza sparire. La sospensione si registra in
-- evidenza.strutturale.sospesa (motivo, tipo, chi, quando: la forma che ValutaDichiarazioni legge) e l'evento
-- va nella storia. La riga resta decisa com'e': una sospensione non e' una revoca. Una sospensione gia'
-- registrata non si riscrive: la prima dice perche'.
UPDATE componente_proposta SET
       evidenza = jsonb_set(evidenza, '{strutturale,sospesa}', sqlc.arg(sospensione)::jsonb) ||
                  jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                  jsonb_build_object('evento', 'autorizzazione_sospesa', 'sospesa', sqlc.arg(sospensione)::jsonb, 'il', now())))
WHERE proposta_id = sqlc.arg(proposta_id) AND evidenza ? 'strutturale' AND NOT ((evidenza -> 'strutturale') ? 'sospesa');

-- name: RiattivaDichiarazione :execrows
-- La riattivazione esplicita di una marcatura sospesa (Smistamento, fase T): la scelta di una persona, dalla
-- scheda del componente, con l'anteprima e la firma. La sospensione va nella storia con chi l'ha tolta e
-- quando; la marcatura resta quella di prima (chi aveva autorizzato il file, e per che cosa).
UPDATE componente_proposta SET
       evidenza = (evidenza #- '{strutturale,sospesa}') || jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) ||
                  jsonb_build_array(jsonb_build_object('evento', 'autorizzazione_riattivata', 'sospesa', evidenza -> 'strutturale' -> 'sospesa',
                                    'riattivata_da', sqlc.arg(riattivata_da)::uuid, 'riattivata_il', now())))
WHERE proposta_id = sqlc.arg(proposta_id) AND (evidenza -> 'strutturale') ? 'sospesa'
  AND stato IN ('confermata', 'duplicato') AND deciso_da IS NOT NULL;

-- name: PrendiNodoAutomatico :execrows
-- Un nodo chiuso da un automatismo di prima dello Smistamento (duplicato o scartato senza chi l'ha deciso,
-- forme F-A e simili) che una persona indica come sorgente di un'autorizzazione (Smistamento F5b, A5.4.7
-- passo 5): l'autorizzazione lo prende come il componente, e lo stato di prima va nella storia.
UPDATE componente_proposta SET stato = 'duplicato', componente_id = sqlc.arg(componente_id)::uuid,
       deciso_da = sqlc.arg(deciso_da)::uuid, deciso_il = now(),
       evidenza = evidenza || jsonb_build_object('storia', COALESCE(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(
                  jsonb_build_object('evento', 'preso_dall_autorizzazione', 'stato', stato, 'componente_id', componente_id,
                                     'deciso_il', deciso_il, 'nota', nota, 'preso_da', sqlc.arg(deciso_da)::uuid, 'preso_il', now())))
WHERE proposta_id = sqlc.arg(proposta_id) AND stato IN ('duplicato', 'scartata') AND deciso_da IS NULL;

-- name: ListRimozioniDiUnoStep :many
SELECT * FROM rimozione_proposta WHERE thread_id = $1 AND step_documento_id = $2 ORDER BY padre_id, figlio_id;

-- name: ChiudiRimozione :execrows
-- Una rimozione aperta che non vale piu' (l'arco non e' piu' nella working, o lo STEP strutturale lo
-- contiene di nuovo): scartata senza chi l'ha decisa, con la nota. E' interpretazione, non decisione.
UPDATE rimozione_proposta SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
WHERE thread_id = sqlc.arg(thread_id) AND step_documento_id = sqlc.arg(step_documento_id)
  AND padre_id = sqlc.arg(padre_id) AND figlio_id = sqlc.arg(figlio_id) AND stato = 'aperta';

-- name: BloccaRimozioneProposta :one
SELECT * FROM rimozione_proposta
WHERE thread_id = $1 AND step_documento_id = $2 AND padre_id = $3 AND figlio_id = $4 FOR UPDATE;

-- name: AggiornaValutazioneProposta :execrows
-- Una lettura nuova di un file che arriva senza un risultato del worker (la radice dello STEP classificata
-- con le regole della RFQ, il gesto «e' la risposta del fornitore»): la valutazione per dimensione va nei
-- dettagli e le colonne sono il suo riepilogo (Smistamento F4, A5.14.7; prima era la D16, che riscriveva
-- codice e fonte con la radice). regola_id = NULL: gli score stanno nella tabella S1, non in `regola`, e
-- un regola_id rimasto da prima attribuirebbe la lettura a un'altra regola.
--
-- Solo una proposta aperta, e non con fonte = 'operatore': una lettura non corregge una decisione. Il codice
-- di una proposta gia' assegnata a un componente e' quello del componente e resta (B8.7: la FK lo vuole).
UPDATE documento_proposta SET tipo_proposto = sqlc.arg(tipo_proposto),
       codice = CASE WHEN componente_id IS NULL THEN sqlc.narg(codice) ELSE codice END,
       rev = sqlc.narg(rev), confidenza = sqlc.arg(confidenza), fonte = sqlc.arg(fonte), regola_id = NULL,
       dettagli = dettagli || sqlc.arg(dettagli)::jsonb
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta' AND fonte <> 'operatore';

-- name: GetPropostaDocumentoDiAllegato :one
SELECT * FROM documento_proposta WHERE allegato_id = $1;

-- name: GetComponentePropostaPerChiave :one
SELECT * FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3;
