// I lavori amministrativi della riga di comando: seminare un'anagrafica, leggere o applicare il seme
// dei fornitori, contare che cosa c'e' gia' in anagrafica.
//
// Ognuno fa il suo e poi ESCE: nessuno di questi mette il server in ascolto, e nessuno parte da solo
// all'avvio normale. Seminare un'anagrafica e' una decisione, non un effetto collaterale (7B.5).

package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/registro/anagrafica"
	"promatec/cockpit/internal/core/registro/fornitori"
	"promatec/cockpit/internal/platform/db"
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
