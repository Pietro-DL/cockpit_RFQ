// cockpit.exe: API JSON per i worker, HTML per il browser, coda job, unico scrittore del NAS.
//
// Qui c'e' solo la riga di comando: i flag, le Opzioni che ne nascono, e il codice di uscita.
// L'avvio vero e' `runtime.Esegui`.
package main

import (
	"flag"
	"fmt"
	"os"

	"promatec/cockpit/internal/app/runtime"
)

func main() {
	cfgPath := flag.String("config", "cockpit.toml", "percorso di cockpit.toml")
	var o runtime.Opzioni
	flag.BoolVar(&o.SoloMigrazioni, "migra", false, "applica le migrazioni e il seed, poi esce (nessun ascolto HTTP)")
	flag.StringVar(&o.SemeAnagrafica, "semina-anagrafica", "", "file JSON di clienti, domini e buyer da seminare; poi esce")
	anteprimaFornitori := flag.String("anteprima-fornitori", "", "file JSON del seme fornitori: dice che cosa scriverebbe e NON scrive; poi esce")
	importaFornitori := flag.String("importa-fornitori", "", "file JSON del seme fornitori: lo applica davvero; poi esce")
	flag.BoolVar(&o.ContaAnagrafiche, "conta-anagrafiche", false, "stampa quante righe ci sono in anagrafica (clienti, buyer, fornitori...); poi esce")
	flag.Parse()
	o.SemeFornitori, o.ApplicaFornitori = *anteprimaFornitori, false
	if *importaFornitori != "" {
		if o.SemeFornitori != "" {
			fmt.Fprintln(os.Stderr, "errore: -anteprima-fornitori e -importa-fornitori insieme non hanno senso: prima si guarda, poi si scrive")
			os.Exit(1)
		}
		o.SemeFornitori, o.ApplicaFornitori = *importaFornitori, true
	}
	if err := runtime.Esegui(*cfgPath, o); err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
}
