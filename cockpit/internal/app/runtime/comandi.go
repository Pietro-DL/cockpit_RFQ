// I lavori amministrativi della riga di comando: seminare un'anagrafica, leggere o applicare il seme
// dei fornitori, contare che cosa c'e' gia' in anagrafica, stampare le misure della calibrazione,
// riaprire gli agganci automatici di prima dello Smistamento (il comando U5).
//
// Ognuno fa il suo e poi ESCE: nessuno di questi mette il server in ascolto, e nessuno parte da solo
// all'avvio normale. Seminare un'anagrafica e' una decisione, non un effetto collaterale (7B.5).

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/calibrazione"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/registro/anagrafica"
	"promatec/cockpit/internal/core/registro/fornitori"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/migrazioni"
)

// Il seme dell'anagrafica (blocco 3). Si legge e si CONVALIDA prima di scrivere: se una sola
// regola di un solo cliente ha un esempio che non corrisponde alla propria regex, non parte
// niente. Un cliente gia' presente viene saltato per intero — il file e' una fotografia di un
// foglio, il database e' dove qualcuno ha gia' corretto a mano cio' che il foglio sbagliava.
func SeminaAnagrafica(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, file string) error {
	seme, err := anagrafica.LeggiFile(file)
	if err != nil {
		return err
	}
	esito, err := anagrafica.Semina(ctx, pool, seme)
	if err != nil {
		return err
	}
	log.Info("anagrafica seminata", "creati", len(esito.ClientiCreati), "gia_presenti", len(esito.ClientiPresenti),
		"domini", esito.DominiAggiunti, "buyer", esito.BuyerCreati)
	for _, c := range esito.ClientiCreati {
		log.Info("cliente creato", "cliente", c)
	}
	for _, c := range esito.ClientiPresenti {
		log.Info("cliente gia' presente: lasciato com'era", "cliente", c)
	}
	for _, a := range esito.Avvisi {
		log.Warn("seme anagrafica", "avviso", a)
	}
	if err := ricalcola(ctx, pool, log, esito.IndirizziScritti, esito.DominiScritti); err != nil {
		return err
	}
	return nil
}

// Il seme dei fornitori (7A.4) dalla riga di comando: LO STESSO motore della schermata
// Admin > Anagrafica > Fornitori > Importa, cioe' `fornitori.Leggi`, `Calcola`, `Applica`. Non
// c'e' un secondo lettore del file e non c'e' un secondo importatore: PowerShell orchestra, Go
// convalida e scrive. Senza `-importa-fornitori` non viene scritta nemmeno una riga.
func SemeFornitori(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, file string, applica bool) error {
	seme, err := fornitori.LeggiFile(file)
	if err != nil {
		return err
	}
	var ant fornitori.Anteprima
	if applica {
		ant, err = fornitori.Applica(ctx, pool, seme)
	} else {
		ant, err = fornitori.Calcola(ctx, db.New(pool), seme)
	}
	if err != nil {
		return err
	}
	verbo := "da creare"
	if applica {
		verbo = "creati"
	}
	log.Info("seme fornitori: "+verbo, "fornitori", len(ant.FornitoriDaCreare), "gia_presenti", len(ant.FornitoriPresenti),
		"righe", len(ant.DaAggiungere), "gia_in_database", len(ant.Presenti), "non_risolti", len(ant.NonRisolti))
	for _, f := range ant.FornitoriDaCreare {
		log.Info("fornitore "+verbo, "fornitore", f)
	}
	for _, r := range ant.NonRisolti {
		log.Warn("seme fornitori: NON risolto, non viene scritto", "riga", r.String())
	}
	for _, r := range ant.Avvisi {
		log.Warn("seme fornitori", "avviso", r.String())
	}
	if !applica {
		fmt.Println("anteprima: non e' stato scritto niente. Per applicare: -importa-fornitori " + file)
		return nil
	}
	if err := ricalcola(ctx, pool, log, ant.IndirizziScritti, ant.DominiScritti); err != nil {
		return err
	}
	return nil
}

// ContaAnagrafiche stampa quante righe ci sono in anagrafica: una riga per voce, `chiave=valore`.
// Serve allo script di bootstrap, che le mostra e basta, senza dover sapere com'e' fatto lo schema.
func ContaAnagrafiche(ctx context.Context, pool *pgxpool.Pool) error {
	c, err := db.New(pool).ContaAnagrafiche(ctx)
	if err != nil {
		return err
	}
	// Una riga per voce, `chiave=valore`: lo script di bootstrap le mostra e basta, senza dover
	// sapere com'e' fatto lo schema.
	for _, v := range []struct {
		nome string
		n    int32
	}{
		{"clienti", c.Clienti}, {"domini_cliente", c.DominiCliente}, {"buyer", c.Buyer},
		{"fornitori", c.Fornitori}, {"domini_fornitore", c.DominiFornitore},
		{"contatti_fornitore", c.ContattiFornitore}, {"lavorazioni_fornitore", c.LavorazioniFornitore},
		{"qualifiche", c.Qualifiche},
	} {
		fmt.Printf("%s=%d\n", v.nome, v.n)
	}
	return nil
}

// Calibrazione stampa le misure dello Smistamento (M3, A5.14.6, A5.16.6): quante volte il primo proposto
// era quello giusto, per livello, per tipo e per fascia di score, dalle fotografie delle decisioni e,
// per i dati di prima, dalla «retro». Legge soltanto: il pool arriva da ApriDatabaseInLettura e le misure
// stanno in una transazione READ ONLY. La prima riga dice quale database si sta leggendo.
func Calibrazione(ctx context.Context, pool *pgxpool.Pool, w io.Writer, dal *time.Time) error {
	r, err := calibrazione.Misura(ctx, pool, dal)
	if err != nil {
		return err
	}
	return r.Scrivi(w)
}

// ------------------------------------------------------------------ il comando U5 (Smistamento F7, A5.15)

// RiapriAgganci e' il comando U5: `-anteprima-riapri-agganci` (o.DatabaseRiapertura vuoto) e `-riapri-agganci
// <nome-database>`. La logica sta in core/rfq/fascicolo (riapertura.go); qui c'e' il database giusto, la stampa
// e il rapporto JSON.
//
// Il database giusto, prima di tutto (P38; memoria del progetto: il comando scrive dove dice il file, non
// dove si ha in mente):
//  1. la prima riga, prima di aprire qualunque cosa, dice il database di destinazione come lo leggera' pgx
//     (host:porta/nome come utente), senza password, e il percorso assoluto del .toml da cui viene;
//  2. in applicazione il nome scritto sulla riga di comando deve essere quello del DSN, prima di collegarsi, e
//     quello che il server dice di essere (SELECT current_database()), dopo: altrimenti si esce senza scrivere;
//  3. non migra: con uno schema diverso da quello del binario si ferma (ApriDatabaseSenzaMigrare);
//  4. l'anteprima apre il database in sola lettura (ApriDatabaseInLettura): una scrittura per errore la
//     rifiuterebbe PostgreSQL. Il comando lo richiede al pool appena aperto, e in applicazione gli chiede
//     current_database(): tutti e due i controlli stanno in controllaIlCollegamento, e la seconda riga li dice.
//
// Sui dati veri si applica solo con lo Smistamento F10/F11 in produzione e dopo il backup (decisione
// dell'utente del 27/09 sera): lo dice l'aiuto del comando, e lo ripetono l'anteprima e il rapporto.
func RiapriAgganci(ctx context.Context, cfg *config.Config, cfgPath string, w io.Writer, o Opzioni) error {
	file := cfgPath
	if abs, err := filepath.Abs(cfgPath); err == nil {
		file = abs
	}
	dest, nome, err := DestinazioneDelDSN(cfg.DB.DSN)
	if err != nil {
		fmt.Fprintf(w, "database di destinazione: non leggibile (dal file %s)\n", file)
		return err
	}
	fmt.Fprintf(w, "database di destinazione: %s (dal file %s)\n", dest, file)
	applica := o.DatabaseRiapertura != ""
	if applica {
		if err := ControllaNomeDatabase(o.DatabaseRiapertura, nome, ""); err != nil {
			return err
		}
	}
	var pool *pgxpool.Pool
	if applica {
		pool, err = ApriDatabaseSenzaMigrare(ctx, cfg, risorse.FS)
	} else {
		pool, err = ApriDatabaseInLettura(ctx, cfg, risorse.FS)
	}
	if err != nil {
		return err
	}
	defer pool.Close()
	leggi := func(ctx context.Context, sql string) (string, error) {
		var v string
		err := pool.QueryRow(ctx, sql).Scan(&v)
		return v, err
	}
	if err := controllaIlCollegamento(ctx, w, o.DatabaseRiapertura, nome, leggi); err != nil {
		return err
	}
	uscita := o.UscitaRiapertura
	if uscita == "" {
		uscita = percorsoRapportoRiapertura(cfg, file, time.Now())
	}
	if abs, err := filepath.Abs(uscita); err == nil {
		uscita = abs
	}
	fmt.Fprintf(w, "rapporto: %s\n", uscita)
	rap, errComando := fascicolo.RiapriAgganci(ctx, pool, o.RfqRiapertura, applica, func(r fascicolo.RapportoRiapertura) error {
		return scriviRapportoRiapertura(uscita, r)
	})
	StampaRiapertura(w, rap)
	if errComando != nil {
		return errComando
	}
	if !applica {
		fmt.Fprintf(w, "anteprima: non e' stato scritto niente. Per applicare: -riapri-agganci %s "+
			"(sui dati veri solo con lo Smistamento F10/F11 in produzione, dopo il backup)\n", nome)
	}
	return nil
}

// DestinazioneDelDSN dice dove porta un DSN come lo leggera' pgx, senza la password: «host:porta/nome come
// utente», e il nome del database. Il nome e' quello risolto (un DSN chiave=valore, un `?dbname=` che vince sul
// percorso, un DSN senza database), non il testo del DSN: e' lo stesso criterio di testutil.DatabaseDiTest.
//
// E' un involucro di migrazioni.Destinazione (A1c, P-02), che serve anche al banco del motore A; qui resta il
// testo dell'errore del Cockpit, che rimanda al file di configurazione.
func DestinazioneDelDSN(dsn string) (testo, nome string, err error) {
	testo, nome, err = migrazioni.Destinazione(dsn)
	if err != nil {
		// l'errore di pgx puo' citare il DSN, password compresa: non si ripete
		return "", "", fmt.Errorf("[db].dsn non si legge: controlla il file di configurazione")
	}
	return testo, nome, nil
}

// ControllaNomeDatabase e' la guardia dell'applicazione (P38): il nome scritto sulla riga di comando deve
// essere quello del DSN del file e, dopo la connessione, quello che il server dice di essere (corrente; vuoto
// = non ancora collegati). Pura.
func ControllaNomeDatabase(argomento, delDSN, corrente string) error {
	switch {
	case strings.TrimSpace(argomento) == "":
		return fmt.Errorf("-riapri-agganci vuole il nome del database da scrivere: il file punta a %q", delDSN)
	case argomento != delDSN:
		return fmt.Errorf("il file punta a %q, hai scritto %q: controlla -config (non e' stato scritto niente)", delDSN, argomento)
	case corrente != "" && corrente != argomento:
		return fmt.Errorf("il file punta a %q, ma il server collegato dice di essere %q: controlla -config (non e' stato scritto niente)", delDSN, corrente)
	}
	return nil
}

// controllaIlCollegamento guarda il database appena collegato, prima di qualunque lettura del comando U5, e lo
// dice nella seconda riga. In applicazione (argomento = il nome scritto sulla riga di comando) il server deve
// dire di essere quel database (SELECT current_database(), P38); in anteprima (argomento vuoto) il pool deve
// essere quello di ApriDatabaseInLettura, con default_transaction_read_only: e' il secondo lucchetto, dopo le
// transazioni ReadOnly del core, e un pool scrivibile qui vorrebbe dire che l'anteprima non e' aperta come
// deve. leggi fa una domanda al database e torna il primo valore: le prove L1 la sostituiscono.
func controllaIlCollegamento(ctx context.Context, w io.Writer, argomento, delDSN string, leggi func(context.Context, string) (string, error)) error {
	if argomento == "" {
		ro, err := leggi(ctx, "SHOW default_transaction_read_only")
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		if ro != "on" {
			return fmt.Errorf("l'anteprima non ha aperto il database in sola lettura (default_transaction_read_only=%s): non si legge niente", ro)
		}
		fmt.Fprintf(w, "collegato in sola lettura (default_transaction_read_only=%s)\n", ro)
		return nil
	}
	corrente, err := leggi(ctx, "SELECT current_database()")
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	if err := ControllaNomeDatabase(argomento, delDSN, corrente); err != nil {
		return err
	}
	fmt.Fprintf(w, "collegato a %s (SELECT current_database())\n", corrente)
	return nil
}

// percorsoRapportoRiapertura e' il rapporto JSON di default: riapri-agganci-AAAAMMGG-hhmmss.json nella cartella
// del log, oppure, senza log su file, accanto al file di configurazione.
func percorsoRapportoRiapertura(cfg *config.Config, file string, ora time.Time) string {
	dir := filepath.Dir(file)
	if p := cfg.PercorsoLog(); p != "" {
		dir = filepath.Dir(p)
	}
	return filepath.Join(dir, "riapri-agganci-"+ora.Format("20060102-150405")+".json")
}

// scriviRapportoRiapertura scrive il rapporto intero, prima di cominciare e dopo ogni RFQ: prima un file
// accanto, poi al suo posto, cosi' chi lo apre durante il comando (o dopo un'interruzione) legge sempre un JSON
// intero. La prima scrittura, prima di qualunque RFQ, e' la prova che il percorso si puo' scrivere.
func scriviRapportoRiapertura(percorso string, r fascicolo.RapportoRiapertura) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err != nil {
		return fmt.Errorf("rapporto: %w", err)
	}
	tmp := percorso + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("rapporto: %w", err)
	}
	if err := os.Rename(tmp, percorso); err != nil {
		return fmt.Errorf("rapporto: %w", err)
	}
	return nil
}

// StampaRiapertura stampa il rapporto: una riga per RFQ del perimetro che ha qualcosa da dire (le RFQ in
// revisione a parte, dopo le altre), poi i totali, una voce per riga, `chiave=valore` (come ContaAnagrafiche),
// e gli avvisi. Il dettaglio riga per riga sta nel rapporto JSON.
func StampaRiapertura(w io.Writer, r fascicolo.RapportoRiapertura) {
	fmt.Fprintf(w, "modalita=%s\n", r.Modalita)
	if r.Modalita == fascicolo.ModalitaAnteprima {
		fmt.Fprintf(w, "sola_lettura=%s\n", map[bool]string{true: "sì", false: "no"}[r.SolaLettura])
	}
	for _, revisione := range []bool{false, true} {
		intestata := false
		for _, x := range r.Rfq {
			if x.InRevisione != revisione || (x.Forme.Vuote() && (x.Esito == fascicolo.EsitoAnteprima || x.Esito == fascicolo.EsitoNiente)) {
				continue
			}
			if revisione && !intestata {
				fmt.Fprintln(w, "RFQ in revisione (si riapre solo la bozza; le versioni congelate e le istantanee non si toccano):")
				intestata = true
			}
			f := x.Forme
			fmt.Fprintf(w, "rfq=%s cartella=%q bom=%q fa_nodi=%d fa_nell_autorita=%d fb_archi=%d fc_radici_tenute=%d fd_conflitti=%d "+
				"fe_chiusure_automatiche=%d ff_radici_a_mano=%d fg_preassegnazioni=%d fh_codici_riscritti=%d fi_gruppi_in_blocco=%d "+
				"fj_componenti_da_rivedere=%d fk_archi_da_rivedere=%d fl_anomalie=%d esito=%q",
				x.ThreadID, x.Cartella, x.Versione, len(f.Agganci), f.Effetto.FigliDaConfermare, len(f.Archi), len(f.RadiciTenute),
				len(f.Conflitti), f.ChiusureAutomatiche.Totale(), len(f.RadiciAMano), len(f.Preassegnazioni), len(f.CodiciRiscritti),
				len(f.ConfermatiInBlocco), len(f.ComponentiDaRivedere), len(f.ArchiDaRivedere), len(f.Anomalie), x.Esito)
			if x.Motivo != "" {
				fmt.Fprintf(w, " motivo=%q", x.Motivo)
			}
			if x.Esito == fascicolo.EsitoRiaperta {
				fmt.Fprintf(w, " nodi_riaperti=%d archi_riaperti=%d", x.NodiRiaperti, x.ArchiRiaperti)
			}
			fmt.Fprintf(w, "\n  effetto: %s\n", f.Effetto.Frase)
			for _, c := range f.Conflitti {
				fmt.Fprintf(w, "  conflitto: %s\n", c)
			}
			for _, a := range f.Anomalie {
				fmt.Fprintf(w, "  anomalia: %s\n", a)
			}
		}
	}
	t := r.Totali
	for _, v := range []struct {
		nome string
		n    int
	}{
		{"rfq_nel_perimetro", t.NelPerimetro}, {"rfq_in_revisione", t.InRevisione},
		{"rfq_escluse_congelate", t.EscluseCongelate}, {"rfq_escluse_chiuse", t.EscluseChiuse}, {"rfq_escluse_unite", t.EscluseUnite},
		{"fa_nodi", t.Agganci}, {"fa_nodi_nell_autorita", t.AgganciNellAutorita}, {"fb_archi", t.Archi},
		{"fc_radici_tenute", t.RadiciTenute}, {"fd_conflitti", t.Conflitti}, {"fe_chiusure_automatiche", t.ChiusureAutomatiche},
		{"ff_radici_a_mano", t.RadiciAMano}, {"fg_preassegnazioni", t.Preassegnazioni}, {"fh_codici_riscritti", t.CodiciRiscritti},
		{"fi_gruppi_confermati_in_blocco", t.ConfermatiInBlocco}, {"fj_componenti_da_rivedere", t.ComponentiDaRivedere},
		{"fk_archi_da_rivedere", t.ArchiDaRivedere}, {"fl_anomalie", t.Anomalie},
		{"rfq_riaperte", t.Riaperte}, {"rfq_saltate", t.Saltate}, {"rfq_fallite", t.Fallite},
		{"nodi_riaperti", t.NodiRiaperti}, {"archi_riaperti", t.ArchiRiaperti},
	} {
		fmt.Fprintf(w, "%s=%d\n", v.nome, v.n)
	}
	for _, a := range r.Avvisi {
		fmt.Fprintf(w, "avviso: %s\n", a)
	}
}

// ricalcola riguarda i messaggi gia' arrivati dopo un seed: quelli che parlano con i domini e gli
// indirizzi appena scritti, e che nessuno ha ancora deciso (7B.5).
//
// Senza questo passo, seminare l'anagrafica su un database che ha gia' dentro la posta non sposta
// di un messaggio il quadrante Da validare: la controparte e' un FATTO scritto sul messaggio
// all'ingest (0014), non una domanda che l'Inbox rifa' a ogni lettura. Riavviare il server non
// basterebbe: il ricalcolo dell'avvio guarda solo i messaggi che una controparte non ce l'hanno.
func ricalcola(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, indirizzi, domini []string) error {
	if len(indirizzi) == 0 && len(domini) == 0 {
		log.Info("niente da riguardare: il seme non ha scritto nessun dominio e nessun indirizzo")
		return nil
	}
	esito, err := (&ingest.Servizio{Pool: pool, Log: log}).RitriageMolti(ctx, indirizzi, domini)
	if err != nil {
		return fmt.Errorf("ricalcolo dei messaggi gia' arrivati: %w", err)
	}
	log.Info("messaggi gia' arrivati riguardati", "esito", esito.String())
	for _, d := range esito.Dettagli {
		log.Info("ricalcolo", "cambiato", d)
	}
	return nil
}
