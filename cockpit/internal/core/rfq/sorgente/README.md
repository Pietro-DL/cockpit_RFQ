# sorgente

S8 «Conferma questo STEP come sorgente di ‹prodotto›» e la revoca della sorgente (direttiva 39 §2, §3 [U + T-39];
U2 §2.2 ed EM §10.9 «S8» per la forma, superati dove la direttiva li supera). Corsia persistenza.

## Che cosa fa

S8 conferma la fonte della **bozza**, mai della working (lo fa solo R5): passi idempotenti, ognuno nella sua
transazione, con la chiave d'intento del corpo e le chiavi dei passi derivate (`ChiavePasso`, UUID v5).

| Passo | Che cosa | Scrive |
|---|---|---|
| 0 | `Controlla` (senza scrivere, e di nuovo sotto il lucchetto nel passo 2): STEP `cad_3d` di un allegato della RFQ, attivo, scaricato, sha256; prodotto finito non archiviato; nessuna AR del file; caso (g); lo stato della bozza. Poi, con il calcolo del motore di chi chiama (`Preparazione`): V-7, il motivo, lo stato dell'analisi con la presa d'atto | niente |
| 1 | non c'e' piu': il legame dello STEP sorgente non e' una AR (risposta `non_serve`) | niente |
| 2 (k2) | il documento con `confermadoc.Conferma` (componente = prodotto, scelta «aggiungi»), la riga `conferma_documento` di S8 (con k2, gruppo e motivo), la copia sul NAS del solo STEP sorgente; un documento gia' presente del prodotto (anche storico) riceve solo la riga di S8. Con la sostituzione della bozza (sotto), prima della riga di S8 la revoca della sorgente vecchia (k2r) | `documento`, `documento_provenienza`, `documento_proposta.stato`, `fonte_step_evento`, `job` (`copia_nas`) |
| 3 (k3) | la bozza con R2 del core: `CreaFormato3` partenza step o `solo_prodotto`; con una conferma valida `da-conferma-1` (k3a) e poi `CambiaSorgenteFormato3` (k3b) se la fonte e' un'altra; con una bozza aperta nata da un'altra sorgente S8 prima di R5 la sostituzione (`CreaFormato3` con `Abbandona`, sotto); con un'altra bozza aperta su un'altra sorgente STEP (nata da una conferma) `CambiaSorgenteFormato3`; con la bozza manuale aperta (senza sorgente, o `solo_prodotto`) la riconciliazione sulla stessa bozza: partenza step → `CambiaSorgenteFormato3` (cambio-fonte-2), partenza `solo_prodotto` su una bozza senza sorgente → `SoloProdottoSuBozzaFormato3`; il corpo dichiara la bozza (altrimenti 409 `BOZZA_VERSIONE` `altra_bozza_aperta` con la bozza); mai una seconda bozza attiva | come R2 |

S8 non scrive `componente.step_strutturale_id`, non marca `sostituito_da`, non chiude righe legacy, non crea AR, non
tocca la working. L'intento (`conferma_sorgente`) si scrive nella transazione dell'ultimo passo; dopo un rifiuto
definitivo in una transazione propria (esito `parziale`); dopo un errore temporaneo (40P01, 55P03) no: lo stesso reinvio
riprende dal primo passo non fatto.

La gara fra due S8 sulla stessa RFQ (blocco «R4 completo», 20/10): i passi prendono il lucchetto della RFQ, ma il contesto
del passo 3 si legge fuori; se la creazione della bozza trova la bozza aperta nata intanto da un'altra S8 sulla stessa
sorgente (sha256 e gruppo), il passo 3 e' «gia' fatto» come nel caso sequenziale (`bozzaNataNellaGara`); con un'altra
sorgente resta il rifiuto `altra_bozza_aperta` (esito parziale: la conferma del passo 2 c'e'). Mai due bozze aperte; prova
L4 `s8_gara_db_test.go`.

`Revoca`: la riga `revoca_sorgente` motivata di `fonte_step_evento` (con il documento, la chiave, la bozza aperta su
quella sorgente se c'e') e l'intento `revoca_sorgente` (azione e gesto propri dalla variante `20261019c` della 0023,
R39-10/R39-11: l'azione `revoca` resta la revoca dell'autorizzazione legacy). La sorgente revocata non vale piu' per R5
(`confermabom.SorgenteValida`) finche' S8 non ne conferma una valida. Giro G39 (G39-03 [U], forma [T-39]): prima di R5
(lo STEP non e' entrato in una conferma della BOM del prodotto) la bozza aperta nata da quella sorgente si annulla nella
stessa transazione (abbandonata con lo stato e il registro esistenti, storia conservata: `bozza_abbandonata`); dopo R5 la
revoca resta un evento (`usata_da_conferma`) e il cambio di sorgente e' una revisione. Il corpo accetta
`sorgente_errata: true` (la causa in testa al motivo registrato) e `bozza: {bozza_id, versione_attesa}` (se non e' piu'
quella, 409 `BOZZA_VERSIONE` senza scritture).

Il codice della vecchia lettura nel passo 2 (G39-02 [U]; scelta [T-39 integrazione, da ratificare], difetto trovato nel
collaudo da dati azzerati del 21/10, ristretta nel giro G39-bis su rilievo della revisione del contratto): lo STEP che il
Motore A, nello stesso calcolo che prepara S8, da' come candidatura UNIVOCA di questo prodotto (`candidatoDelMotore`, con
`distintabozza.StepCandidati.UnivocaDel`: la candidatura che la pagina propone in S9) si conferma senza il motivo anche se
`documento_proposta.codice` non e' il codice del prodotto (il nome del file con la sola base, 9120001 contro 9120001A, come
in PM-01 il marcatore finale): il documento prende il codice del prodotto e la riga di S8 ha come motivo la traccia del
server con le evidenze del motore (`tracciaCodiceDalMotore`, che comincia con `PrefissoTracciaMotore`: «codice dal Motore
A: letto «9120001», prodotto «9120001A»; candidatura univoca dello STEP del finito; evidenze: …»; con la correzione della
sorgente la causa `sorgente_errata` resta in testa; nessun DDL). Il reinvio dello stesso corpo riconosce la traccia
(`stessoMotivoDelPasso2`) e resta «gia' fatto». Senza correzione del codice (letto uguale al prodotto) la riga resta senza
motivo. Prima finiva «parziale» con `MOTIVO_OBBLIGATORIO` nel passo 2, e la proposta della pagina non chiede il motivo.
Senza il calcolo del motore (partenza `solo_prodotto`, ripiego), con una candidatura non univoca (che la pagina non
propone) o con lo STEP candidato di un altro prodotto resta il motivo obbligatorio. Prove: L4
`s8_codice_dal_motore_g39_db_test.go`, L1 `s8_traccia_motore_test.go`.

## La bozza nata da un'altra sorgente S8 prima di R5 (giro G39-ter, `sostituzione.go`)

G39-03 [U]: «la bozza appena creata deve essere annullata siccome la sorgente step cambia»; messaggio 43, caso 1 [U]: la
bozza sbagliata si cancella (nello schema: stato `abbandonata`), in `fonte_step_evento` la revoca e poi la conferma giusta,
i registri delle bozze annotano revoca e riassegnamento, la storia non si cancella. Prima (fino a `9eb44e3`) il passo 3
faceva il cambio della fonte sulla stessa bozza. La forma e' [T-39] della corsia sorgente, senza DDL:

- **Quando:** la bozza aperta del prodotto e' nel formato 3, e' nata da S8 con la partenza step (l'evento `bozza_aperta`
  della versione 1), la sua sorgente e' uno STEP con struttura confermato da S8 per il prodotto, il prodotto non ha nessuna
  conferma della BOM, e S8 conferma con la partenza step un contenuto diverso. Restano come prima la bozza nata da una
  conferma (`da-conferma-1` e cambio della fonte: la revisione del messaggio 41), la bozza manuale o `solo_prodotto` (la
  riconciliazione), lo stesso contenuto con un altro gruppo, «Sposta la sorgente su questo prodotto» e la revoca dopo R5.
- **Presa d'atto:** il corpo dichiara la bozza (`bozza_id`, `versione_attesa`; senza, 409 `BOZZA_VERSIONE` come prima). Se
  la bozza ha modifiche dell'utente (eventi del giornale con origine `utente` o `step`) serve anche `sostituisci:
  {conferma: true}`, altrimenti 422 `CORPO_NON_VALIDO` `presa_d_atto_mancante` (campo `sostituisci`) senza scritture.
- **Passo 2:** sotto il lucchetto, prima della riga di S8, la revoca della sorgente vecchia: `revoca_sorgente`, ruolo
  prodotto, il documento della sua conferma S8, la bozza vecchia, chiave `ChiavePasso(intento, "2r")`, motivo del server
  «sorgente_errata: sostituita dalla conferma S8 di ‹sha256 a 12 cifre›» (con il motivo dell'utente in coda). Una
  sorgente vecchia gia' revocata non si revoca di nuovo.
- **Passo 3:** `CreaFormato3` con `Abbandona` e la partenza step: la bozza vecchia `abbandonata` (motivo di chiusura «‹motivo›
  — ripartita dalla proposta nella bozza ‹id›», il registro lo scrive il trigger), la nuova dalla proposta con le PB
  iniziali; nella stessa transazione l'evento `riassegnazione_sorgente` sulla versione 1 della bozza nuova
  (`distintabozza.ScriviRiassegnazione`: prima la bozza abbandonata, la versione, la sorgente vecchia; dopo la sorgente
  nuova e `revoca_evento`) e l'intento.
- **Risposta:** `sostituzione` (`bozza_abbandonata`, `versione_abbandonata`, `allegato_precedente`, `sha256_precedente`,
  `revoca_evento`, `modifiche_utente`, `motivo`, `testo`) e il testo «la bozza precedente … è stata sostituita».
  `SostituzioneDellaBozza` (sola lettura) da' la stessa regola all'anteprima S6 con `?bozza=`
  (`effetto_sulla_bozza.sostituzione`), perche' il dialogo lo dica prima.

Il reinvio con la stessa chiave e' «riproposto»; una chiave nuova sulla stessa sorgente trova la bozza gia' su di lei
(«gia' fatto»). Non tocca la working, `step_strutturale_id`, `sostituito_da` ne' le AR. Lo scambio dopo R5 (G39-05 caso 2)
e le correzioni sul NAS restano dopo il push (messaggio 44 [U]).

## Slegare e riassociare uno STEP (G39-03, `sposta.go`)

Mai la stessa sorgente S8 valida su due prodotti; lo STEP legato al prodotto sbagliato si slega e si riassocia (G39-03 [U];
la forma e' [T-39] della corsia persistenza, senza DDL). Il caso (g) di S8 (lo STEP e' un documento di un altro finito
‹A›) e la copia P1 su un altro componente si decidono nel passo 0:

| Lo STEP rispetto ad ‹A› | Senza gesto | Con il gesto |
|---|---|---|
| nella BOM confermata di ‹A› (`step_strutturale_id`, una conferma strutturale con quella fonte, una baseline) | 422 `STEP_SORGENTE_DI_ALTRO_PRODOTTO` `bom_confermata` con `altro_prodotto` e il rimedio «apri la revisione di ‹A›», con il suo limite: dopo la revisione lo STEP resta nella storia di ‹A› | lo stesso rifiuto, niente scritto |
| sorgente S8 valida di ‹A›, non usata da una R5 | 422 `STEP_SORGENTE_DI_ALTRO_PRODOTTO` `sorgente_spostabile` con `altro_prodotto` | `sposta_da: ‹A›` nel corpo di S8: «Sposta la sorgente su questo prodotto» |
| solo documento di ‹A› (legacy, legame storico, sorgente gia' revocata) | 422 `MOTIVO_OBBLIGATORIO` `riassociazione` con `altro_prodotto` | il motivo scritto (anche con `sposta_da`: la causa da sola non basta) |
| copia P1 (proposta aperta con il componente, senza AR) su un altro componente | 422 `MOTIVO_OBBLIGATORIO` `riassociazione` | il motivo scritto (anche con `sposta_da`: la causa da sola non basta) |

Restano rifiuti prudenti (`GIA_DOCUMENTO`): la revisione di un altro documento di ‹A›, lo STEP autorizzato dallo
Smistamento e lo STEP gia' sostituito nella storia della BOM di ‹A› (motivo `sostituito_in_altro_prodotto` con
`altro_prodotto`). Quest'ultimo e' il caso dopo R5: lo STEP confermato per errore nella BOM di ‹A›, dopo la revisione di
‹A› resta un documento sostituito di ‹A› e non diventa la sorgente del prodotto giusto, perche' `documento` e' UNIQUE
(thread_id, sha256) e la storia di ‹A› resta com'e'. Servirebbe un cambio di schema: aperto, domanda all'utente. Il testo
non propone di annullare la sostituzione (nel formato 3 la sostituzione dello STEP strutturale la scrive solo R5), e
ricaricare il file non aiuta (stesso contenuto). Lo spostamento si scrive nel passo 2, nella transazione e con la chiave k2 della riga di S8 (atomico; chiave
della revoca la' `ChiavePasso(intento, "2s")`): la revoca motivata su ‹A› (causa `sorgente_errata`) se ‹A› ha ancora una
riga `conferma_documento` aperta per quel contenuto; la bozza di ‹A› nata da quella sorgente abbandonata; il documento
passa al prodotto (componente e codice; il percorso sul NAS si ricalcola solo se il file non e' ancora sul NAS:
`percorso_nas` `aggiornato` o `invariato`); la copia P1 tolta da `associazioni.Scollega` (evento «sgancia» senza AR,
origine ponte, motivo nella nota). La risposta di S8 porta `spostamento` (caso, da_prodotto_id, revoca_evento,
bozza_abbandonata, documento_id, copia_tolta, percorso_nas, motivo, testo). Con `correzione_sorgente: true` il motivo
della riga di S8 ha la causa `sorgente_errata` in testa: dopo R5 la conferma successiva e' segnata «correzione della
sorgente» (R4 `effetto_sulla_fonte.correzione_sorgente`, R5 `origine.correzione_sorgente` e `origine.revisione =
"interna"`, `fonte.correzione_sorgente` della risposta, R7 `correzione_sorgente` e `revisione`). Lo stesso vale se prima
di S8, nella stessa revisione, e' stata revocata con la causa `sorgente_errata` una sorgente con cui la BOM del prodotto
era stata confermata (`confermabom.UsataDaConferma`), anche se dopo si sono tolte altre sorgenti sbagliate; dopo la R5
della correzione la stessa sorgente non e' piu' una correzione. Il gesto `sposta_da` scrive la causa solo sulla revoca dell'altro prodotto, non sulla riga
di S8: una sorgente sbagliata revocata o spostata prima di R5 non fa della revisione che segue una correzione, che resta
la revisione del cliente (`confermabom.StatoSorgente.Correzione`).
Aperto [T-39]: con il file gia' scritto sul NAS il documento resta nella cartella di ‹A› finche' non c'e' `sposta_nas` (B8.8).

## Codici propri

`STEP_SORGENTE_DI_ALTRO_PRODOTTO` (422, motivi `sorgente_spostabile` e `bom_confermata` dal giro G39, con `altro_prodotto`),
`GIA_DOCUMENTO` (422; motivo `sostituito_in_altro_prodotto` con `altro_prodotto` dal giro G39), `NON_STEP` (422), `RICONCILIAZIONE_NON_DISPONIBILE`
(409, motivi `bozza_da_convertire`, `solo_prodotto_su_bozza`, `solo_prodotto_su_conferma`; `bozza_manuale` non si emette
piu' dalla riconciliazione della direttiva 39 §3); `MOTIVO_OBBLIGATORIO` con il motivo `riassociazione` (G39); gli altri sono
quelli di `distintabozza`.

## Dipendenze

`rfq/distintabozza` (R2 del core, errori, l'abbandono della bozza), `rfq/documenti` (il percorso sul NAS), `platform/coda`, `rfq/confermadoc` (la conferma del documento), `rfq/confermabom` (la
validita' della conferma e della sorgente), `rfq/associazioni` (la AR attiva), `rfq/fascicolo`, `rfq/filerfq`,
`propostabom` (il motivo della fonte), `platform/db`, `platform/jsoncanonico`.

## Prove

L1 `s8_test.go`; L4 `s8_db_test.go`, `s8_sposta_db_test.go` (G39-03: lo spostamento, la riassociazione del documento e della copia
P1, il rifiuto dopo R5, la gara con R5 sull'altro prodotto nei due ordini, la correzione della sorgente in R4/R5/R7, la revoca
con la bozza dichiarata), `s8_correzione_g39_db_test.go` (la revisione del cliente non e' segnata correzione dopo una
sorgente sbagliata revocata prima di R5, ne' con `sposta_da` nei due versi), `s8_correzione_due_errate_g39_db_test.go`
(la correzione resta con due sorgenti sbagliate nella stessa revisione, revoca o `sposta_da`, e vale una volta sola),
`s8_sostituzione_g39ter_db_test.go` (giro G39-ter: la bozza nata da un'altra sorgente S8 prima di R5 abbandonata e non
riadattata, la revoca prima della conferma, la bozza nuova dalla proposta con le PB iniziali, l'evento
`riassegnazione_sorgente`, i registri; la bozza modificata con e senza la presa d'atto; il reinvio; lo scambio incrociato
prima di R5), `s8_sostituzione_s6_g39ter_db_test.go` (la regola per l'anteprima S6, in sola lettura),
`s8_codice_dal_motore_g39_db_test.go` (la candidatura univoca del motore senza il motivo, con la traccia delle evidenze nella riga di S8 e il reinvio «gia' fatto»; la non univoca e gli altri casi con il motivo), `s8_storia_g39_db_test.go` (dopo la
revisione di ‹A› lo STEP resta nella sua storia: il rifiuto lo dice senza rimedi che non funzionano), `s8_revisione_db_test.go` (la revisione con un secondo STEP sulla strada S8 → R4 →
R5 → S8 di B → R4 → R5, con i guasti iniettati, e il caso non certo), `s8_step_del_finito_db_test.go` (lo STEP del
finito proposto dal registro non diventa PB ne' AR, e S8 su di lui resta ammessa) e `s8_riconcilia_db_test.go` (la bozza manuale
riconciliata; con un registro di
prova al posto del motore) e, per la rotta,
`transport/web/motore_a_sorgente_v1_db_test.go`.

## Allineamento al resoconto della corsia contratto (direttiva 39, C-1a-bis)

- `sostituisci: {conferma: true}` nel corpo e' accettato come presa d'atto senza effetto sul documento (R39-05);
  `{conferma: false}` e' 422. Il documento della working resta corrente: lo sostituisce R5. Dal giro G39-ter e'
  obbligatorio quando S8 sostituisce una bozza con modifiche dell'utente (sopra).
- La presa d'atto dello stato dell'analisi della partenza `solo_prodotto` va nell'evento `bozza_aperta`
  (`presa_d_atto_analisi`).
- `confermabom.SorgenteValida` usa i motivi proposti per `STEP_SORGENTE_NON_CONFERMATA` (`non_confermata`, `revocata`,
  `rifiutata`, `superata`, `non_coerente`); una conferma S8 piu' recente di un altro STEP dello stesso prodotto supera la
  precedente (I.2).
- Non allineato (aperto nel resoconto della corsia): il passo 1 `caricamento` con la copia derivata (il compito della
  corsia diceva passi 0, 2, 3). Il gesto `revoca_sorgente` e l'azione dedicata della revoca ci sono dalla variante
  `20261019c`.
