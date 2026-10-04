// Il comando bancoa è il banco del motore A senza DB (giro 5, A1a): legge il manifest del dataset privato,
// fa girare la modalità «regole» o «casi» e scrive il rapporto, con l'esito in testa. È sottile come
// cmd/cockpit: i flag, il contesto con i segnali e il codice d'uscita; il lavoro lo fa internal/app/bancoa.
//
//	bancoa -modalita regole|casi -dataset <manifest del dataset privato> -uscita <cartella dei rapporti>
//
// Codici d'uscita, gli stessi del riepilogo delle prove e del controllo prima del push (R44):
//   - 0 eseguito e conforme;
//   - 1 eseguito, con differenze (un errore di grammatica, un caso fallito, un controllo del runner fallito);
//   - 2 uso o configurazione (flag sbagliati o mancanti, uscita o dataset dentro il modulo): nessun rapporto,
//     la causa su stderr;
//   - 3 NON ESEGUITO (il manifest manca o non si legge, una sua voce manca o ha un'impronta diversa): il
//     rapporto comincia con «ESITO: NON ESEGUITO — <motivo>».
//
// Non legge cockpit.toml e non apre il DB. L'aiuto usa segnaposti, mai un percorso reale.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"promatec/cockpit/internal/app/bancoa"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	codice := esegui(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(codice)
}

// esegui: i flag, Esegui e il codice d'uscita. Le prove la chiamano senza processo.
func esegui(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bancoa", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o bancoa.Opzioni
	fs.StringVar(&o.Modalita, "modalita", "", "regole | casi")
	fs.StringVar(&o.Dataset, "dataset", "", "il manifest del dataset privato: <percorso del manifest>, fuori dal repository")
	fs.StringVar(&o.Uscita, "uscita", "", "la cartella dei rapporti: <cartella dei rapporti>, fuori dal repository")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "uso: bancoa -modalita regole|casi -dataset <manifest del dataset privato> -uscita <cartella dei rapporti>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return bancoa.UscitaUso // il messaggio (o l'aiuto, con -h) l'ha già scritto il FlagSet
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "bancoa: argomenti in più: %q\n", fs.Args())
		fs.Usage()
		return bancoa.UscitaUso
	}
	r, err := bancoa.Esegui(ctx, o, stdout)
	if err != nil {
		fmt.Fprintln(stderr, err)
		if r.Esito == "" {
			return bancoa.UscitaUso // nessun rapporto valido: uso o configurazione
		}
	}
	return r.Esito.CodiceUscita()
}
