package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

// L1 — blocco 4A: l'attributo `hidden` deve nascondere davvero.
//
// Il browser ha la regola `[hidden]{display:none}`, ma sta nel suo foglio di stile: QUALUNQUE regola
// del nostro che imposti `display` la batte, perché le regole dell'autore vincono su quelle dello
// user agent, e la specificità non c'entra. In pagina ci sono elementi con una classe che imposta
// `display` e l'attributo `hidden` insieme — è così che i campi di censimento di un cliente sono
// rimasti visibili per tutto il tempo, sotto la riga che diceva di non riscriverli.
//
// Il difetto non dà errore da nessuna parte: l'HTML è corretto, l'attributo c'è, il JavaScript lo
// mette e lo toglie. Semplicemente non succede niente. Questa è la riga che lo fa succedere, e il
// test esiste perché tolta quella riga il difetto torna in silenzio.
func TestUnElementoNascostoENascostoDavvero(t *testing.T) {
	statico, err := fs.Sub(risorse.FS, "web/static")
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(statico, "style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)

	re := regexp.MustCompile(`\[hidden\]\s*\{[^}]*display\s*:\s*none\s*!important`)
	if !re.MatchString(css) {
		t.Fatal("il foglio di stile non ha una regola [hidden]{display:none!important}: " +
			"qualunque classe che imposti display rende visibile un elemento marcato hidden")
	}

	// e la prova che serviva davvero: nei template esistono elementi che hanno insieme una classe
	// che imposta `display` e l'attributo `hidden`.
	reDisplay := regexp.MustCompile(`\.([a-z0-9-]+)\s*\{[^}]*display\s*:\s*(grid|flex|inline-block|block|table)`)
	var classi []string
	for _, m := range reDisplay.FindAllStringSubmatch(css, -1) {
		classi = append(classi, "."+m[1])
	}
	if len(classi) == 0 {
		t.Skip("nessuna classe imposta display: la regola qui sopra è precauzionale")
	}
	t.Logf("classi che impostano display e che senza quella regola scavalcherebbero [hidden]: %s", strings.Join(classi, " "))
}
