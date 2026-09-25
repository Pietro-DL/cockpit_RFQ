//go:build integrazione

// L4 — Admin › Anagrafica › Riconoscimento, il form delle famiglie di codice (revisione del 25/09), e la
// riga del fabbisogno con la sua fonte attesa.
//
// Il difetto: «Rev nel codice» e' una casella di spunta, e il browser manda solo quelle spuntate. Il
// server le leggeva come un array parallelo alle regex: con F1 senza e F2 con la revisione arrivava un
// valore solo, che finiva su F1. Il salvataggio si rifiutava su una famiglia che nessuno aveva toccato
// (F1 non ha il gruppo `rev`), o salvava i flag scambiati.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

func TestRevNelCodiceRestaSullaFamigliaGiusta(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	// F1 senza revisione, F2 con: il browser manda fam_rev solo per la seconda riga, con il suo numero
	form := url.Values{
		"fam_regex":       {`\bPZ-\d{3}\b`, `(?P<codice>AC\d{5})(?P<rev>[A-Z])`, ""},
		"fam_descrizione": {"particolari", "assiemi con la revisione attaccata", ""},
		"fam_esempio":     {"PZ-001", "AC12345B", ""},
		"fam_rev":         {"1"},
		"fam_ruolo":       {"prodotto", "prodotto", "prodotto"},
	}
	resp, html := ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/regole/form", form, false)
	if resp.StatusCode != 200 || !strings.Contains(leggibile(html), "Regole salvate") {
		t.Fatalf("salvataggio rifiutato (%d): %s", resp.StatusCode, estrai(html, "errore"))
	}
	r, _ := regole.LeggiRegole(b.cliente(acme).Regole)
	if len(r.FamiglieCodice) != 2 {
		t.Fatalf("famiglie salvate: %d", len(r.FamiglieCodice))
	}
	if r.FamiglieCodice[0].RevNelCodice || !r.FamiglieCodice[1].RevNelCodice {
		t.Errorf("flag scambiati: F1 rev=%v, F2 rev=%v", r.FamiglieCodice[0].RevNelCodice, r.FamiglieCodice[1].RevNelCodice)
	}

	// il form riletto porta il numero della riga in ogni casella, e la spunta dove va
	_, pagina := ad.fai(http.MethodGet, "/admin/anagrafica?cliente="+acme.String()+"&sez=riconoscimento", nil, false)
	for _, atteso := range []string{`name="fam_rev" value="0" >`, `name="fam_rev" value="1" checked`, `name="fam_rev" value="2"`} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("il form non contiene %q:\n%s", atteso, estratto(pagina, `name="fam_rev"`))
		}
	}

	// e salvato di nuovo cosi' com'e' (nessuna spunta toccata), resta uguale
	form["fam_rev"] = []string{"1"}
	if _, html := ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/regole/form", form, false); !strings.Contains(leggibile(html), "Regole salvate") {
		t.Fatalf("secondo salvataggio rifiutato: %s", estrai(html, "errore"))
	}
	if r, _ := regole.LeggiRegole(b.cliente(acme).Regole); r.FamiglieCodice[0].RevNelCodice || !r.FamiglieCodice[1].RevNelCodice {
		t.Error("il secondo salvataggio ha spostato i flag")
	}
}

// Una riga del fabbisogno nasce con la sua fonte attesa, nella stessa transazione: la riga e la fonte
// sono una cosa sola.
func TestLaRigaDelFabbisognoNasceConLaSuaFonte(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	_, html := ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/fabbisogno", url.Values{
		"tipo_componente": {"finito"}, "tipo": {"capitolato"}, "bloccante": {"1"}, "fonte_attesa": {"portale"}}, false)
	if !strings.Contains(leggibile(html), "Riga aggiunta") {
		t.Fatalf("riga non aggiunta: %s", estrai(html, "errore"))
	}
	var fonte string
	if err := b.pool.QueryRow(b.ctx, `SELECT COALESCE(fonte_attesa::text, '') FROM fabbisogno_documento
		WHERE cliente_id = $1 AND tipo_componente = 'finito' AND tipo = 'capitolato'`, acme).Scan(&fonte); err != nil {
		t.Fatal(err)
	}
	if fonte != "portale" {
		t.Errorf("fonte attesa %q, attesa «portale»", fonte)
	}
}
