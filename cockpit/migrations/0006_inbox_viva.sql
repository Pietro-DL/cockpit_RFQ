-- ============================================================================
--  0006_inbox_viva.sql — Cockpit RFQ, blocco 1 dell'addendum (voce 2.16)
--
--  Una colonna sola, e serve a rispondere alla domanda che l'operatore si fa dieci volte al giorno:
--  «che cosa è arrivato da quando ho guardato l'ultima volta?».
--
--  Fino a qui l'Inbox sapeva dire quante mail ci sono, non quante sono NUOVE. La differenza non è
--  estetica: senza, chi torna alla scrivania dopo un'ora deve ricordarsi a memoria dove era arrivato,
--  e in una lista ordinata per data una mail che si infila fra due già viste non la nota nessuno.
--
--  `ultima_vista_inbox` è per UTENTE e non per sessione: la visita è un fatto della persona, non del
--  cookie. Un operatore che chiude il browser la sera e riapre la mattina deve trovare «12 nuove»,
--  non un contatore azzerato dal login. Il valore si aggiorna quando la pagina Inbox viene aperta
--  DAVVERO (caricamento completo), non ai poll HTMX ogni 15 s: se si aggiornasse a ogni poll il
--  contatore direbbe sempre zero, che è il modo più efficace di rendere inutile un contatore.
--
--  NULL = non ha mai aperto l'Inbox: tutto è «nuovo» finché non la apre la prima volta.
--
--  Nota di numerazione: nell'addendum questa migrazione non era prevista (il blocco 1 risultava
--  «senza migrazione») e il numero 0006 era assegnato a `0006_anagrafica` del blocco 3. Una colonna
--  serve comunque, e i numeri delle migrazioni seguono l'ordine in cui vengono applicate, non
--  l'ordine dei blocchi del piano: l'anagrafica diventa quindi `0007_anagrafica`, e a scalare le
--  successive.
-- ============================================================================

ALTER TABLE utente ADD COLUMN ultima_vista_inbox timestamptz;
COMMENT ON COLUMN utente.ultima_vista_inbox IS
    'Quando questo utente ha aperto l''Inbox l''ultima volta. I messaggi con registrato_il successivo sono «nuovi dall''ultima visita» (voce 2.16). NULL = mai aperta. Aggiornata al caricamento della pagina, non ai poll HTMX: altrimenti il conteggio sarebbe sempre zero.';

INSERT INTO schema_versione (versione) VALUES (6);
