# `platform/config` — la lettura di `cockpit.toml`

## Scopo

Legge `cockpit.toml`, mette i default, legge i percorsi relativi dalla cartella del file, sostituisce la
rete del file con quella della riga di comando quando c'è, valida e normalizza. Risolve `[sicurezza]`
nelle tre capacità di scrittura (`Capacita()`) e calcola le letture derivate che il resto del server usa:
dove va il log, per quanto resta un contenuto in cache, ogni quanto gira il ricognitore del NAS, se il
listener parla `http` o `https`. Raccoglie in `Config.Avvisi` le voci del file che non hanno effetto.

Un errore qui ferma l'avvio: meglio non partire che partire con un routing, una rete o una capacità
sbagliati. Una voce sconosciuta o senza effetto invece non ferma niente: diventa un avviso nel log.

## Non appartiene qui

Il seed in database (`platform/fondazioni`), il certificato e il filtro del listener (`platform/rete`),
l'applicazione delle capacità alla coda (`platform/coda`), la lettura di `worker.toml` (i worker Python).
La scelta del file quando manca `-config` (`cmd/cockpit/main.go:percorsoConfig`), il livello del log
(`app/runtime/esegui.go:LivelloLog`) e l'apertura del file di log con la rotazione (`app/runtime/avvio.go:ApriLog`,
`platform/logfile`).

## File

| File | Responsabilità |
|---|---|
| `config.go` | i tipi `Config`, `Server`, `DB`, `NAS`, `Outlook`, `Sicurezza`, `Retention`, `Analisi`, `Staging`, `Agente`, `Utente`, `Casella`, `Postazione`, `Worker`, `Rete`; `Carica` / `CaricaConRete`; le `normalizza*` e `leggiRete`; i percorsi (`cartellaDelFile`, `assoluto`, `conDisco`); gli avvisi (`vociSenzaEffetto` → `Config.Avvisi`); le letture derivate (`Capacita`, `RetentionCache`, `CacheMaxByte`, `IntervalloIntegrita`, `PercorsoLog`, `ETLS`, `Schema`, `HostPubblico`, `RadiceDiProduzione`, `SuLoopback`, `DataDal`, `RuoliAmmessi`) |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `CaricaConRete(percorso, Rete)` | `app/runtime.Esegui` (il solo caricamento in produzione); il percorso lo sceglie `cmd/cockpit/main.go:percorsoConfig`: quello di `-config`, altrimenti il `cockpit.toml` della cartella corrente se c'è, altrimenti quello accanto a `cockpit.exe` (se non c'è nessuno dei due, l'errore dice dove ha cercato) |
| `Carica(percorso)` | i test (è `CaricaConRete` con una `Rete` vuota) |
| `Config.Avvisi` | `app/runtime.Esegui`: una riga `WARN` «cockpit.toml» per avviso, subito dopo l'apertura del log |
| `Rete` | `cmd/cockpit/main.go` la compila da `-ascolto`, `-tls-cert`, `-tls-key`, `-url-pubblico`, `-reti` |
| `(*Config).Capacita()` | `app/runtime.ImpostaCapacita` (→ `coda.ImpostaCapacita`); anche `normalizzaModalita` |
| `PercorsoLog`, `RetentionCache`, `CacheMaxByte`, `IntervalloIntegrita` | `app/runtime` (`ApriLog`, `CostruisciServizi`) |
| `ETLS`, `Schema`, `HostPubblico`, `SuLoopback` | `app/runtime` (`PreparaTLS`, `Ascolta`) |
| `DataDal` | `normalizzaOutlook` (validazione) e `app/runtime.CostruisciServizi` (override del sync) |
| `Casella.EAttiva` | `platform/fondazioni.Semina` |
| `*Config` intero | `platform/fondazioni.Semina`; nei test anche `coda`, `web`, `workerapi` |

## Dati

Nessuna tabella. Sul disco: legge il file TOML e **crea la cartella dello staging** (`os.MkdirAll` di
`[nas].staging`) durante `CaricaConRete`. È l'unico effetto del package.

### Le chiavi

Ogni campo della struttura si legge dal file. Una chiave che la struttura non conosce (un refuso come
`reti_consentit`, o `passwrd` dentro `[[utenti]]`) non ferma l'avvio: vale come se la riga non ci fosse, e
`vociSenzaEffetto` la mette in `Config.Avvisi` con il suo nome completo (`server.reti_consentit`). Nello
stesso elenco finiscono le due voci che si leggono ma non hanno effetto: `[server].segreto_sessione` (se
non è vuota) e `[outlook].consenti_invio` (se è scritta, qualunque valore abbia). Gli avvisi si scrivono
a livello `warn`: con `log_livello = "error"` non compaiono.

**I percorsi relativi** di `[nas].radice`, `[nas].staging`, `[server].log_file`, `tls_cert` e `tls_key`
(anche quelli di `-tls-cert` e `-tls-key`) si leggono dalla cartella del file di configurazione, resa
assoluta (`cartellaDelFile`), non dalla cartella da cui parte il processo. Restano come sono (`assoluto`):
il valore vuoto, un percorso assoluto, un UNC (`\\server-nas\PREVENTIVI`, anche con le barre `/`), un
percorso che comincia con `\` o `/`, uno che comincia con la lettera di un disco (`C:`, anche letto da un
server Linux), e `log_file = "-"`.

**`[server]`**

| Chiave | Default | Obbligatoria | Verifica / effetto |
|---|---|---|---|
| `indirizzo` | `127.0.0.1:8080` | no | fuori da loopback serve TLS, o `consenti_lan_in_chiaro` |
| `tls_cert`, `tls_key` | vuoti | in coppia | relativi alla cartella del file; se i file mancano li genera `rete.Prepara` |
| `tls_nomi` | vuoto = `rete.NomiPredefiniti` | no | nomi del certificato generato |
| `consenti_lan_in_chiaro` | `false` | no | unica via per ascoltare in chiaro fuori da loopback |
| `url_pubblico` | vuoto (derivato dal bind) | no | `http(s)://host[:porta]` senza percorso, schema uguale a `Schema()` |
| `reti_consentite` | vuoto = nessun filtro | no | ogni voce passa da `leggiRete`: solo reti della LAN |
| `log_livello` | `info` | no | `debug`, `info`, `warn` (o `warning`), `error`, senza distinguere le maiuscole; vale per tutto il log del server, a schermo e su file. Un'altra parola vale `info` e l'avvio lo scrive nel log (`app/runtime/esegui.go:LivelloLog`) |
| `log_file` | vuoto = `<staging>/log/cockpit.log` | no | `-` = solo stdout; relativo = dalla cartella del file. Rotazione in `platform/logfile`: il file corrente più 5 copie, da 5 MB; se il file corrente non si lascia rinominare (tenuto aperto da un altro processo) si continua a scriverci oltre la soglia, le copie vecchie restano e si riprova dopo un altro decimo della soglia |
| `modalita` | `shadow` | no | `shadow` o `produzione`, altro = errore |
| `max_upload_mb` | `64` | no | almeno 1, altrimenti errore (anche `0`) |
| `token_worker` | vuoto | no | deprecata: non autentica, l'avvio lo scrive nel log |
| `segreto_sessione` | vuoto | no | senza effetto (le sessioni stanno nel database); scritta, dà un avviso all'avvio |

**`[db]`**: `dsn`, **obbligatoria** (vuota = errore).

**`[nas]`**

| Chiave | Default | Verifica / effetto |
|---|---|---|
| `radice` | vuota | relativa = dalla cartella del file; nessuna verifica; confrontata con `radici_produzione` |
| `staging` | `<cartella del file>/staging` | relativa = dalla cartella del file; creata al caricamento |
| `radici_produzione` | vuoto | confronto senza maiuscole, separatori e barra finale (`normalizzaPercorso`) |
| `intervallo_integrita_s` | assente = 900 | `0` = nessun giro automatico del ricognitore |
| `dry_run` | `false` | deprecata: `true` spegne `nas_scrittura` e lo dice fra gli avvisi |

**`[sicurezza]`**: `outlook_scrittura`, `bozze`, `nas_scrittura` (assenti = spente),
`consenti_nas_produzione` (serve quando `nas_scrittura` è accesa e la radice sta sotto una radice di
produzione). Sono puntatori: assente e `false` restano distinguibili.

**`[outlook]`**

| Chiave | Default | Verifica / effetto |
|---|---|---|
| `cartelle` | `["Inbox", "Sent Items"]` | — |
| `intervallo_sync_s` | `60` | `0` = nessun sync periodico |
| `giorni_sync_iniziale` | `0` = `coda.GiorniSyncInizialeDefault` (7) | negativo = errore |
| `sync_apertura_inbox` | assente = `true` | — |
| `dal` | vuoto | `AAAA-MM-GG` nel fuso del server (`DataDal`), altrimenti errore |
| `lotto` | `50` | nessuna verifica |
| `consenti_invio` | `false` | senza effetto sul server, che prepara bozze e non invia (l'invio lo decide il `consenti_invio` del `worker.toml`); scritta, dà un avviso all'avvio |
| `casella_default` | con una sola `[[casella]]` è quella | con più caselle è obbligatoria; deve essere fra le `[[casella]]` |

**`[retention]`**: `giorni_job` (30; 0 = mai), `cache_gg` (assente = 30; 0 = mai per età; negativo = 0),
`giorni_staging` (deprecata: vale come `cache_gg` se `cache_gg` manca), `cache_max_mb` (0 = nessun limite).

**`[analisi]`**: `versione` (1), `parametri` (tabella libera; il suo hash entra nella chiave dei fatti).

**`[staging]`**: `automatico` (`false`), `bootstrap` (`false`), `max_mb` (20; `0` lascia la soglia
all'ingest, che usa la propria).

**`[agente]`**: `attivo` (`false`), `modello`, `url`, `chiave_env` (il NOME della variabile), `caselle`.

**Le tabelle ripetute**

| Sezione | Campi | Regole |
|---|---|---|
| `[[utenti]]` | `sigla`, `nome`, `ufficio`, `ruolo`, `password` | sigla obbligatoria, unica, in maiuscolo; ruolo obbligatorio e dell'enum `ruolo_utente`; almeno un `admin` se c'è almeno un utente |
| `[[casella]]` | `indirizzo`, `nome`, `canale`, `condivisa`, `utente`, `attiva` | indirizzo obbligatorio, unico, in minuscolo; `nome` = parte locale se manca; `canale` = `outlook` se manca (validato solo al seed); condivisa senza `utente`; `attiva` assente = vera |
| `[[postazione]]` | `nome_host`, `descrizione`, `utente` | nome host obbligatorio, unico, in maiuscolo |
| `[[worker]]` | `nome`, `tipo`, `token`, `postazione`, `caselle` | nome obbligatorio e unico; tipo `outlook` o `analisi`; token facoltativo ma mai uguale a quello di un altro worker; postazione e caselle devono essere dichiarate |

La password di `[[utenti]]` qui non si controlla: il rifiuto della password vuota o del segnaposto
`INSERISCI_PASSWORD_INIZIALE` per un utente nuovo sta nel seed (`platform/fondazioni/utenti.go:SeedUtenti`).

**`cockpit.toml.example`** è la procedura per un server nuovo, in ordine, con percorsi segnaposto sotto
`C:\Cockpit\` (radice di prova `C:\Cockpit\nas_prova\PREVENTIVI DA FARE`, staging `C:\Cockpit\server\staging`),
un NAS di produzione fittizio (`\\server-nas\preventivi`) fra le `radici_produzione`, indirizzi
`@azienda.example`, sigle di `[[casella]]` e `[[postazione]]` dichiarate in `[[utenti]]`. Parte così
com'è cambiando solo `[db].dsn` e le password (`esempio_test.go`). Tre voci dell'esempio sono diverse dal
default, e togliere la riga cambia il comportamento: `intervallo_sync_s = 30` (default 60),
`[analisi].versione = 3` (default 1, e la versione entra nella chiave dei fatti), `[staging].automatico = true`
(default `false`).

## Flussi principali

**Il caricamento** (`CaricaConRete`):

1. default nei campi (`indirizzo`, `log_livello`, `max_upload_mb`, `modalita`, `cartelle`,
   `intervallo_sync_s`, `lotto`, `giorni_job`, `versione`, `max_mb`);
2. `toml.DecodeFile`: ogni chiave presente sovrascrive il default; `vociSenzaEffetto` riempie
   `Config.Avvisi` (chiavi sconosciute, `segreto_sessione`, `[outlook].consenti_invio`);
3. `dsn` vuota → errore;
4. `[nas].radice`, `[nas].staging` e `[server].log_file` (tranne `-`) resi assoluti rispetto alla
   cartella del file (`assoluto`);
5. `Rete.applica`: con `-ascolto` la rete della riga di comando sostituisce **per intero** indirizzo,
   certificato, `tls_nomi` (azzerati), `url_pubblico`, `reti_consentite` e `consenti_lan_in_chiaro`
   (forzato a falso), e segna `DaRigaDiComando`; senza `-ascolto` gli altri flag di rete sono un errore;
6. `normalizzaRete`: percorsi TLS assoluti rispetto alla cartella del file, TLS in coppia, rifiuto del
   chiaro fuori da loopback, `url_pubblico` coerente con lo schema, `reti_consentite` lette in `Server.Reti`;
7. `max_upload_mb ≥ 1`;
8. `normalizzaModalita`: vedi sotto;
9. staging di default (`<cartella del file>/staging`) e sua creazione;
10. `normalizzaOutlook`, `normalizzaUtenti`, `normalizzaFondazioni` (forma canonica e riferimenti
    incrociati; `casella_default` dedotta con una casella sola).

**Le capacità** (`Capacita`), in quest'ordine: `shadow` spegne tutto e lo dice; nessuna delle tre voci
di `[sicurezza]` scritta = tutto spento, con l'avviso; poi le tre voci come sono; `dry_run = true`
spegne `nas_scrittura`; se `nas_scrittura` resta accesa su una radice di produzione, un avviso lo dice.

**La modalità** (`normalizzaModalita`): in `shadow` una `[nas].radice` sotto una radice di produzione
ferma l'avvio; in `produzione` lo ferma solo se `nas_scrittura` è accesa e manca
`consenti_nas_produzione`.

**Le reti** (`leggiRete`): una rete o un indirizzo solo (/32, /128), senza zona; un IPv4 in forma 4in6
torna IPv4; il prefisso si maschera; passa se sta dentro uno dei `blocchiLAN` (IPv4 privati,
link-local, loopback, IPv6 ULA e link-local, `::1`) o se è un IPv6 globale di `/64` o più stretto.

## Invarianti

- `[db].dsn` c'è.
- Mezzo TLS non esiste: `tls_cert` senza `tls_key`, o il contrario, ferma l'avvio.
- Senza TLS, fuori da loopback si parte solo con `consenti_lan_in_chiaro = true`; un indirizzo senza
  host (`:8080`) conta come LAN.
- `url_pubblico` ha lo schema che il listener parla davvero.
- `reti_consentite` non lascia entrare internet.
- `modalita` vale `shadow` o `produzione`; assente vale `shadow`.
- Il silenzio vale «non scrivere»: una capacità assente è spenta; `produzione` da sola non accende niente.
- Scrivere sul NAS di produzione richiede due dichiarazioni (`nas_scrittura` e `consenti_nas_produzione`).
- Un ruolo sconosciuto ferma l'avvio; con utenti dichiarati c'è almeno un `admin`.
- Nessun riferimento incrociato verso una casella o una postazione non dichiarata; nessun token in comune
  fra due worker.
- La rete della riga di comando passa dalle stesse verifiche del file.
- Un percorso relativo del file vale dalla cartella del file, qualunque sia la cartella corrente del
  processo (un'attività pianificata o un servizio partono da un'altra cartella).
- Una voce sconosciuta o senza effetto non ferma l'avvio e non passa in silenzio: è un avviso.

## Dipendenze

Importa `platform/db` (per l'enum dei ruoli) e librerie (`BurntSushi/toml`). È importato da
`cmd/cockpit`, `app/runtime` e `platform/fondazioni`; nei test anche da `platform/coda`,
`transport/web`, `transport/workerapi`. Nessuna violazione della tabella di `internal/README.md`.

## Test

Tutti L1 (nessun tag, nessun database), su file TOML scritti in una cartella temporanea:

| File | Che cosa prova |
|---|---|
| `config_test.go` | forma canonica di caselle e postazioni, `casella_default` dedotta, i rifiuti dei riferimenti incrociati e dei token duplicati, `attiva` assente = vera, `[analisi]` |
| `esempio_test.go` | `cockpit.toml.example` parte cambiando solo le password (lo staging spostato in una cartella temporanea): nessun avviso, ogni sigla di `[[casella]]` e `[[postazione]]` dichiarata in `[[utenti]]`, indirizzi `.example`, radice fuori dalle radici di produzione |
| `finestra_test.go` | `giorni_sync_iniziale` (assente, scritta, negativa) e `dal` (malformata, fuso locale) |
| `modalita_test.go` | assente = shadow, valore sbagliato rifiutato, shadow su radice di produzione, produzione su radice di produzione |
| `percorsi_test.go` | `radice`, `staging` e `log_file` relativi letti dalla cartella del file anche da un'altra cartella corrente (e con `-config` relativo); assoluti, UNC, «dalla radice» e con disco invariati; `log_file = "-"`; chiavi sconosciute e senza effetto negli avvisi senza fermare l'avvio; `max_upload_mb = 0` rifiutato |
| `radiceprod_test.go` | il secondo consenso per il NAS vero e il suo avviso; in shadow il consenso non sblocca niente |
| `retention_test.go` | `cache_gg`, `giorni_staging` deprecata, capienza |
| `rete_test.go` | `SuLoopback`, rifiuto del chiaro in LAN, mezzo TLS, percorsi relativi al file, radici di produzione POSIX |
| `reti_test.go` | `leggiRete` (ammesse e rifiutate), `reti_consentite` dal file, la rete della riga di comando che vale per intero |
| `ruoli_test.go` | ruolo inesistente, i quattro ruoli, sigla doppia, nessun admin, senza utenti |
| `sicurezza_test.go` | le regole di `Capacita()`, `dry_run` deprecata |
| `urlpubblico_test.go` | forma e schema di `url_pubblico`, `HostPubblico` anche IPv6 |

Fuori dal package: `app/runtime/esegui_test.go` prova `LivelloLog` (le quattro parole, `warning`, una
parola sconosciuta); `cmd/cockpit/config_test.go` prova la scelta del file senza `-config`.
Non provato: `IntervalloIntegrita`.

## Stato dell'implementazione

Completo per il blocco 4 (capacità), la voce 2.4 (TLS, credenziali individuali), 7C.1 (`url_pubblico`),
l'avvio in rete (`reti_consentite`, `Rete`), i percorsi relativi alla cartella del file e gli avvisi sulle
voci senza effetto. Limiti noti:

- `[nas].radice` vuota non è un errore nemmeno con `nas_scrittura` accesa (le scritture restano in coda
  perché la radice non risulta raggiungibile);
- `segreto_sessione` e `[outlook].consenti_invio` restano lette (per dirle nel log) invece di essere
  rifiutate: un file scritto per un binario di prima deve continuare a partire.

## Dove intervenire

| Voglio… | Apro |
|---|---|
| una chiave nuova | il tipo della sezione in `config.go`, il default in `CaricaConRete` (se è un percorso, anche `assoluto` lì), la verifica in una `normalizza*`, poi i due `*.example` (`esempio_test.go` controlla che l'esempio del server non dia avvisi) |
| rendere «senza effetto» una chiave che c'era | lasciarla nel tipo e aggiungere l'avviso in `vociSenzaEffetto` (toglierla dal tipo la farebbe diventare «sconosciuta») |
| capire perché una riga del file non conta | le righe «cockpit.toml» del log di avvio (`Config.Avvisi`) |
| cambiare che cosa ferma l'avvio | `normalizzaRete`, `normalizzaModalita`, `normalizzaUtenti`, `normalizzaFondazioni`, `normalizzaOutlook` |
| capire perché una capacità è spenta | `Capacita()` e i suoi `Avvisi` nel log di avvio |
| ammettere un altro tipo di rete | `blocchiLAN` e `leggiRete` |
| un flag di rete nuovo | `Rete`, `Rete.applica`, `cmd/cockpit/main.go` |

## Leggi anche

`internal/platform/README.md`, `internal/platform/rete/README.md`, `internal/platform/fondazioni/README.md`,
`internal/app/README.md`, `cockpit.toml.example`, il README principale («Configurazione del server»,
«Mettere il Cockpit in rete»).
