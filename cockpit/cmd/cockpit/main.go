// cockpit.exe: API JSON per i worker, HTML per il browser, coda job, unico scrittore del NAS.
//
// Qui c'e' solo la riga di comando: i flag, le Opzioni che ne nascono, e il codice di uscita.
// L'avvio vero e' `runtime.Esegui`.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"promatec/cockpit/internal/app/runtime"
	"promatec/cockpit/internal/platform/config"
)

func main() {
	cfgPath := flag.String("config", "cockpit.toml", "percorso di cockpit.toml (senza: quello della cartella corrente, altrimenti quello accanto a cockpit.exe)")
	var o runtime.Opzioni
	flag.BoolVar(&o.SoloMigrazioni, "migra", false, "applica le migrazioni e il seed, poi esce (nessun ascolto HTTP)")
	flag.StringVar(&o.SemeAnagrafica, "semina-anagrafica", "", "file JSON di clienti, domini e buyer da seminare; poi esce")
	anteprimaFornitori := flag.String("anteprima-fornitori", "", "file JSON del seme fornitori: dice che cosa scriverebbe e NON scrive; poi esce")
	importaFornitori := flag.String("importa-fornitori", "", "file JSON del seme fornitori: lo applica davvero; poi esce")
	flag.BoolVar(&o.ContaAnagrafiche, "conta-anagrafiche", false, "stampa quante righe ci sono in anagrafica (clienti, buyer, fornitori...); poi esce")
	// La rete dalla riga di comando (scripts/avvio-rete): con -ascolto vale PER INTERO al posto delle
	// voci di rete di [server] nel file (config.Rete).
	var rete config.Rete
	flag.StringVar(&rete.Indirizzo, "ascolto", "", "indirizzo:porta su cui ascoltare, al posto di [server].indirizzo; con questo le voci di rete di [server] nel file non valgono")
	flag.StringVar(&rete.TLSCert, "tls-cert", "", "certificato del listener (con -ascolto), relativo alla cartella del file di configurazione; se manca con la chiave, si genera")
	flag.StringVar(&rete.TLSKey, "tls-key", "", "chiave del certificato (con -ascolto)")
	flag.StringVar(&rete.URLPubblico, "url-pubblico", "", "l'indirizzo con cui browser e worker chiamano il server (con -ascolto)")
	reti := flag.String("reti", "", "reti da cui accettare connessioni, separate da virgole: 10.0.0.0/24,fd12:3456:789a:1::/64 (con -ascolto)")
	flag.Parse()
	for _, r := range strings.Split(*reti, ",") {
		if r = strings.TrimSpace(r); r != "" {
			rete.Reti = append(rete.Reti, r)
		}
	}
	o.SemeFornitori, o.ApplicaFornitori = *anteprimaFornitori, false
	if *importaFornitori != "" {
		if o.SemeFornitori != "" {
			fmt.Fprintln(os.Stderr, "errore: -anteprima-fornitori e -importa-fornitori insieme non hanno senso: prima si guarda, poi si scrive")
			os.Exit(1)
		}
		o.SemeFornitori, o.ApplicaFornitori = *importaFornitori, true
	}
	esplicito := false
	flag.Visit(func(f *flag.Flag) { esplicito = esplicito || f.Name == "config" })
	percorso, cercati := percorsoConfig(esplicito, *cfgPath, esiste, os.Executable)
	if !esplicito && !esiste(percorso) {
		fmt.Fprintf(os.Stderr, "errore: nessun cockpit.toml (cercato in %s). Metterlo accanto a cockpit.exe, "+
			"oppure indicarlo con -config e un percorso assoluto\n", strings.Join(cercati, " e in "))
		os.Exit(1)
	}
	if err := runtime.Esegui(percorso, rete, o); err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
}

// percorsoConfig sceglie il cockpit.toml da leggere. Con -config, quello e basta. Senza, quello della
// cartella corrente se c'e' (lo sviluppo: `go run ./cmd/cockpit` dalla cartella cockpit), altrimenti
// quello accanto all'eseguibile, che e' dove il README lo mette: un'attivita' pianificata o un servizio
// partono da un'altra cartella (C:\Windows\System32), e prima cercavano il file li'. cercati sono i posti
// guardati, per dirli se il file non c'e' in nessuno dei due.
func percorsoConfig(esplicito bool, dato string, esiste func(string) bool, eseguibile func() (string, error)) (percorso string, cercati []string) {
	if esplicito {
		return dato, []string{dato}
	}
	cercati = []string{dato}
	if abs, err := filepath.Abs(dato); err == nil {
		cercati[0] = abs
	}
	if esiste(dato) {
		return dato, cercati
	}
	exe, err := eseguibile()
	if err != nil {
		return dato, cercati
	}
	accanto := filepath.Join(filepath.Dir(exe), "cockpit.toml")
	if !strings.EqualFold(accanto, cercati[0]) {
		cercati = append(cercati, accanto)
	}
	return accanto, cercati
}

// esiste dice se al percorso c'e' un file (non una cartella).
func esiste(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
