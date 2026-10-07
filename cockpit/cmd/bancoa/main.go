// Il comando bancoa è il banco del motore A (giro 5): legge il manifest del dataset privato, fa girare una modalità e
// scrive il rapporto, con l'esito in testa. È sottile come cmd/cockpit: i flag, il contesto con i segnali e il codice
// d'uscita; il lavoro lo fa internal/app/bancoa.
//
//	bancoa -modalita regole|casi -dataset <manifest del dataset privato> -uscita <cartella dei rapporti>
//	bancoa -dsn <DSN della copia, senza password> -dataset <manifest del dataset privato> (-thread <uuid> ... | -tutti) [-attesi [-gate]] -uscita <cartella dei rapporti>
//	bancoa -exports <cartella degli export> -dataset <manifest del dataset privato> [-thread <uuid> ...] [-attesi [-gate]] -uscita <cartella dei rapporti>
//
// Le modalità regole e casi (A1a) sono senza DB e non cambiano. Da A1c ci sono dsn ed exports (piano 6.4.9; T-B6-03,
// F0-01): la modalità si scrive con -modalita, oppure la dice -dsn o -exports. -dsn ed -exports si escludono; -tutti
// vale solo con -dsn; -manifest è un sinonimo di -dataset. Il DSN non porta la password (la legge pgx da pgpass.conf),
// e con PGPASSWORD impostata il banco non parte (R32 c). Le migrazioni incorporate (cockpit.FS) servono al controllo
// dello schema della copia: le passa questo comando, perché il runner non importa la radice del modulo (F0-16).
//
// Codici d'uscita, gli stessi del riepilogo delle prove e del controllo prima del push (R44):
//   - 0 eseguito e conforme (con -gate, gate superato);
//   - 1 eseguito, con differenze (un errore di grammatica o di contratto, un caso fallito, un controllo del runner
//     fallito; con -gate, una voce del gate non superata);
//   - 2 uso o configurazione (flag sbagliati o mancanti, DSN con la password, PGPASSWORD impostata, uscita o dataset
//     dentro il modulo): nessun rapporto, la causa su stderr;
//   - 3 NON ESEGUITO (il manifest manca o non si legge, una sua voce manca o ha un'impronta diversa, la copia non si
//     raggiunge o non è quella del manifest; con -gate, una voce non eseguita e nessuna fallita): il rapporto comincia
//     con «ESITO: NON ESEGUITO — <motivo>».
//
// Non legge cockpit.toml. L'aiuto usa segnaposti, mai il nome di una copia, di un ruolo, di uno script o un percorso
// reale (E-17).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"promatec/cockpit"
	"promatec/cockpit/internal/app/bancoa"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	codice := esegui(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(codice)
}

// esegui: i flag, Esegui o EseguiBanco, e il codice d'uscita. Le prove la chiamano senza processo.
func esegui(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bancoa", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o bancoa.Opzioni
	var manifest string
	fs.StringVar(&o.Modalita, "modalita", "", "regole | casi | dsn | exports (dsn ed exports li dicono anche -dsn ed -exports)")
	fs.StringVar(&o.Dataset, "dataset", "", "il manifest del dataset privato: <percorso del manifest>, fuori dal repository")
	fs.StringVar(&manifest, "manifest", "", "sinonimo di -dataset: <manifest del dataset privato>")
	fs.StringVar(&o.Uscita, "uscita", "", "la cartella dei rapporti: <cartella dei rapporti>, fuori dal repository")
	fs.StringVar(&o.DSN, "dsn", "", "la copia del dump in sola lettura: <DSN della copia, senza password>")
	fs.StringVar(&o.Exports, "exports", "", "la cartella degli export: <cartella degli export>, fuori dal repository")
	fs.Var(bancoa.FlagThread(&o.Thread), "thread", "una RFQ da valutare: <uuid>; ripetibile")
	fs.BoolVar(&o.Tutti, "tutti", false, "tutte le RFQ, nella stessa transazione (solo con -dsn)")
	fs.BoolVar(&o.Attesi, "attesi", false, "gli esiti contro gli attesi del manifest, i controlli del runner e il gate")
	fs.BoolVar(&o.Gate, "gate", false, "il gate decide l'uscita: superato 0, non superato 1, incompleto 3 (con -attesi)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "uso: bancoa -modalita regole|casi -dataset <manifest del dataset privato> -uscita <cartella dei rapporti>")
		fmt.Fprintln(stderr, "     bancoa -dsn <DSN della copia, senza password> -dataset <manifest del dataset privato> (-thread <uuid> ... | -tutti) [-attesi [-gate]] -uscita <cartella dei rapporti>")
		fmt.Fprintln(stderr, "     bancoa -exports <cartella degli export> -dataset <manifest del dataset privato> [-thread <uuid> ...] [-attesi [-gate]] -uscita <cartella dei rapporti>")
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
	if manifest != "" {
		if o.Dataset != "" && o.Dataset != manifest {
			fmt.Fprintln(stderr, "bancoa: -dataset e -manifest sono lo stesso flag: due valori diversi non si scelgono")
			return bancoa.UscitaUso
		}
		o.Dataset = manifest
	}
	if o.Modalita == "" {
		switch {
		case o.DSN != "" && o.Exports != "":
			fmt.Fprintln(stderr, "bancoa: -dsn ed -exports si escludono")
			return bancoa.UscitaUso
		case o.DSN != "":
			o.Modalita = bancoa.ModalitaDSN
		case o.Exports != "":
			o.Modalita = bancoa.ModalitaExports
		}
	}
	var esito bancoa.Esito
	var err error
	switch o.Modalita {
	case bancoa.ModalitaDSN, bancoa.ModalitaExports:
		o.Migrazioni = cockpit.FS
		var r bancoa.RapportoBanco
		r, err = bancoa.EseguiBanco(ctx, o, stdout)
		esito = r.Esito
	default:
		var r bancoa.Rapporto
		r, err = bancoa.Esegui(ctx, o, stdout)
		esito = r.Esito
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		if esito == "" {
			return bancoa.UscitaUso // nessun rapporto valido: uso o configurazione
		}
	}
	return esito.CodiceUscita()
}
