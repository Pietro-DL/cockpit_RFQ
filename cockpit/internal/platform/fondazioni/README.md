# `platform/fondazioni` — `cockpit.toml` portato in database

## Scopo

A ogni avvio, dopo le migrazioni e prima di qualunque servizio, porta in database ciò che il file
dichiara: gli utenti (`utenti.go:SeedUtenti`), poi caselle, postazioni e credenziali dei worker
(`fondazioni.go:Semina`). Non gira con i comandi che leggono soltanto (`-conta-anagrafiche`,
`-anteprima-fornitori`), che aprono il database senza migrarlo né seminarlo. Il seed è **non
distruttivo**: aggiorna per chiave naturale e non disattiva mai una riga che nel file non c'è più, la
segnala soltanto. Un utente **nuovo** però non nasce senza una password vera: la password vuota o il
segnaposto dell'esempio fermano l'avvio. Dice anche, prima del primo 401, quali credenziali non potranno
autenticare (`StatoCredenziali`).

## Non appartiene qui

La lettura e la validazione del file (`platform/config`), il login e la pagina Postazioni
(`transport/web`), l'autenticazione dei worker (`transport/workerapi`), l'anagrafica dei clienti e dei
fornitori (`core/registro`, riga di comando).

## File

| File | Responsabilità |
|---|---|
| `fondazioni.go` | `Semina` ed `Esito`; `HashToken`; la diagnosi delle credenziali (`StatoCredenziale`, `CredenzialeOk` / `CredenzialeDaGenerare` / `CredenzialeCondivisa`, `DiagnosiCredenziale`, `StatoCredenziali`); `UnaSolaCasellaAttiva` con `VersioneCaselleMultiple`; `CasellaPerIndirizzo` |
| `utenti.go` | `SeedUtenti` (la password del file serve a nascere), `PasswordEsempio` (il segnaposto `INSERISCI_PASSWORD_INIZIALE`), `segnaposto`, `passwordIniziale`, `RuoliAmmessi`, `passwordImpostata` |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `SeedUtenti(ctx, q, utenti, log)` | `app/runtime.Semina`, prima di `Semina`; nei test `web` e `fondazioni`. Con `log` nil usa `slog.Default()` |
| `PasswordEsempio` | i test di `fondazioni` |
| `Semina(ctx, q, cfg, log)` | `app/runtime.Semina`; nei test `coda`, `web`, `workerapi` |
| `UnaSolaCasellaAttiva(ctx, q, versione)` | `app/runtime.Semina`, con la versione restituita dalle migrazioni |
| `StatoCredenziali(righe)` | `Semina` (log) e `transport/web/postazioni_admin.go` (pagina Postazioni) |
| `CredenzialeOk`, `CredenzialeDaGenerare`, `CredenzialeCondivisa` | `transport/web/postazioni_admin.go` |
| `CasellaPerIndirizzo` | nessuno |

`SeedUtenti` riceve gli utenti come `[]struct{ Sigla, Nome, Ufficio, Ruolo, Password string }`:
`app/runtime.Semina` li copia da `config.Utente`.

## Dati

| Tabella | Letta | Scritta | Query (`db/queries`) |
|---|---|---|---|
| `utente` | sì | upsert per `sigla`: nome, ufficio, ruolo, `attivo = true`; `password_hash` solo se in database non c'è un hash bcrypt leggibile e il file ha una password che non è il segnaposto | `ListUtenti`, `UpsertUtente` (`utenti.sql`) |
| `casella` | sì | upsert per `(canale, indirizzo)`: nome, condivisa, proprietario, `attiva` dal file | `UpsertCasella`, `ListCaselle`, `ListCaselleAttive` |
| `postazione` | — | upsert per `nome_host`: descrizione, proprietario, `attiva = true` | `UpsertPostazione` |
| `worker_credenziale` | sì | upsert per `worker_nome`: tipo, postazione, caselle, `attivo = true`; `token_hash` = sha256 del token se il file lo scrive, altrimenti invariato (alla prima creazione `rete.ImprontaNonGenerata`) | `UpsertWorkerCredenziale`, `UpsertWorkerCredenzialeMantieniToken`, `ListWorkerCredenziali` |

Nessuna transazione: ogni upsert è a sé, e il seed è idempotente. Nessun lucchetto.

## Flussi principali

**`SeedUtenti`**:

1. legge gli utenti già in database;
2. controlla **tutti** gli utenti del file prima di scrivere (un rifiuto a metà elenco non lascia nel
   database metà degli utenti): il ruolo deve essere dell'enum; per un utente che il database non ha
   ancora, la password non può essere vuota né il segnaposto `INSERISCI_PASSWORD_INIZIALE` (confrontato
   senza spazi ai lati e senza maiuscole, `segnaposto`). Altrimenti l'avvio si ferma con un errore che
   nomina la sigla e dice che cosa scrivere (`passwordIniziale`);
3. per ogni utente del file: se in database c'è già un hash bcrypt valido non lo tocca (e se il file ha
   una password lo dice nel log, livello info); se la password del file è il segnaposto non la usa e lo
   dice nel log (utente già in database senza hash: resta senza password); altrimenti hasha la password
   del file; un utente già in database senza hash e senza password nel file riceve un avviso («non potrà
   entrare»);
4. per ogni utente **attivo** in database che non è più fra gli `[[utenti]]` del file, un avviso nel log
   (sigla e ruolo): può ancora entrare, e il seed non lo disattiva.

**`Semina`**:

1. risolve le sigle degli utenti in `utente_id` (una sigla sconosciuta finisce in `UtentiMancanti` e
   lascia `NULL`);
2. caselle: canale validato contro l'enum `canale`, upsert, mappa indirizzo → id;
3. postazioni: upsert, mappa nome host → id;
4. worker: tipo `outlook` o `analisi` (mai `server`), postazione e caselle risolte; con `token` nel file
   si scrive la sua impronta, senza si mantiene quella che c'è;
5. `casella_default` risolta in id;
6. righe **attive** in database e assenti dal file (caselle e worker) → `NonPiuNelFile`, solo avviso;
7. `StatoCredenziali` sulle credenziali attive → `CredenzialiDaGenerare` (segreto mai generato o
   condiviso con un altro worker), con una riga di log per ciascuna.

**`StatoCredenziali`** (pura): raggruppa per impronta le credenziali attive con un segreto vero; una
credenziale con `ImprontaNonGenerata` è `da_generare`, una che condivide l'impronta è `condivisa` (con
chi), le altre `ok`.

## Invarianti

- Una riga che sparisce dal file non viene disattivata: la segnala il log (per utenti, caselle e worker).
- Un utente nuovo nasce solo con una password vera: vuota o segnaposto dell'esempio fermano l'avvio, prima
  di qualunque scrittura. Il segnaposto non diventa mai la password di nessuno.
- Un token scritto nel file vince a ogni avvio; un token vuoto non cancella quello generato dalla
  pagina Postazioni.
- In database va solo l'impronta del token (`rete.ImprontaToken`, la stessa funzione dell'autenticazione).
- Una credenziale senza segreto ha un'impronta che nessuna richiesta può presentare
  (`rete.ImprontaNonGenerata`).
- La password del file non sovrascrive un hash valido già in database.
- Un ruolo, un canale o un tipo di worker sconosciuto ferma l'avvio.
- Con lo schema sotto la versione 4, più di una casella attiva ferma l'avvio.

## Dipendenze

Importa `platform/config`, `platform/db`, `platform/rete`, `bcrypt`. È importato da `app/runtime` e da
`transport/web`; nei test anche da `platform/coda` e `transport/workerapi`. Nessuna violazione.

## Test

`fondazioni_db_test.go` e `utenti_db_test.go`, L4 (`//go:build integrazione`, `COCKPIT_TEST_DSN`):

| Test | Che cosa prova |
|---|---|
| `TestSeminaEIdempotenza` | il seed completo, l'impronta del token, un secondo seed identico |
| `TestCasellaTolaDalFileNonVieneDisattivata` | una casella tolta dal file resta attiva e finisce in `NonPiuNelFile` |
| `TestSiglaUtenteSconosciuta` | la sigla ignota finisce in `UtentiMancanti` e il proprietario resta vuoto |
| `TestStoreIDDiversiPerPostazione` | lo StoreID per (postazione, casella) |
| `TestPiuCaselleAttiveRifiutateFinoAllaVersione4` | `UnaSolaCasellaAttiva` |
| `TestUnUtenteNuovoNonNasceConLaPasswordDellEsempio` | segnaposto (anche con spazi e minuscole) e password vuota fermano il seed con la sigla nel messaggio, e nessun utente del file viene scritto |
| `TestGliUtentiGiaInDatabaseNonCambianoPerIlSegnaposto` | un hash valido resta; il segnaposto non diventa la password di un utente senza hash, e il log lo dice |
| `TestChiNonENelFileMaEAttivoSiDiceNelLog` | l'avviso per l'utente attivo tolto dal file, non per quello disattivato né per chi è nel file |

Fuori dal package: `transport/web/migrazione_credenziali_db_test.go` (token condivisi, credenziali da
generare), `transport/web/rbac_db_test.go` (la password del file non riscrive quella del database).
Nessun test L1: `StatoCredenziali` è pura e si prova solo con il database.

## Stato dell'implementazione

Completo per la voce 0.6, la voce 2.4 e gli utenti (B7); della voce 6.4 c'è la regola «la password del
file serve solo a far nascere l'utente», non il cambio della password dalla UI. Limiti noti:

- un utente tolto dal file resta attivo e può entrare (il log lo dice a ogni avvio; per chiudergli la
  porta si mette `utente.attivo = false` in database); una postazione tolta dal file non è segnalata
  (`NonPiuNelFile` guarda solo caselle e worker) e resta attiva;
- il cambio della password dalla UI non c'è: una password si reimposta svuotando `utente.password_hash`
  in database e riavviando con quella nuova nel file;
- una riga disattivata a mano in database e ancora presente nel file torna attiva al riavvio
  (utenti, postazioni, credenziali), tranne la casella, la cui `attiva` viene dal file;
- `CasellaPerIndirizzo` non ha chiamanti.

## Dove intervenire

| Voglio… | Apro |
|---|---|
| seminare un campo nuovo di `[[casella]]`, `[[postazione]]`, `[[worker]]` | `config.go` (il campo), `fondazioni.sql` (l'upsert), `Semina` |
| cambiare la regola della password del file | `utenti.go:SeedUtenti`, `utenti.go:passwordIniziale` (utente nuovo), `UpsertUtente` in `utenti.sql` |
| cambiare il segnaposto dell'esempio | `utenti.go:PasswordEsempio` insieme a `cockpit.toml.example` (lo legge `config/esempio_test.go`) |
| capire perché un worker prende 401 | `StatoCredenziali`, il log di avvio, la pagina Postazioni |
| togliere la guardia della casella unica | `UnaSolaCasellaAttiva` (vale solo sotto la versione 4) |

## Leggi anche

`internal/platform/config/README.md`, `internal/platform/rete/README.md`, `internal/platform/README.md`,
`internal/app/README.md`, `cockpit.toml.example` (sezioni `[[utenti]]`, `[[casella]]`, `[[worker]]`).
