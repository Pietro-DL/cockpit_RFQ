//go:build integrazione

// L4 — due gesti dell'amministratore che scrivono piu' righe, e che adesso le scrivono tutte o nessuna
// (revisione del 25/09): il pacchetto della postazione, che ruotava i token uno per volta prima di
// costruire lo zip, e le lavorazioni di un fornitore, che si toglievano una per volta prima di scoprire
// che la successiva non si poteva aggiungere.
package web

import (
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/db"
)

// fsGuasto e' il filesystem dei file del worker con un disco che non legge i .py: l'elenco c'e', i
// file no. E' il modo di far fallire lo zip DOPO che i token sono stati generati.
type fsGuasto struct{ fs.FS }

func (g fsGuasto) Open(nome string) (fs.File, error) {
	if strings.HasPrefix(nome, "workers/") && path.Ext(nome) == ".py" {
		return nil, errors.New("disco guasto")
	}
	return g.FS.Open(nome)
}

// Uno zip che non nasce non lascia la postazione senza segreti: i token restano quelli di prima, e il
// worker continua a entrare. Quando lo zip nasce, i token cambiano tutti insieme.
func TestUnPacchettoNonCostruitoNonCambiaITokenDellaPostazione(t *testing.T) {
	b := preparaBancoWeb(t)
	const nome = "outlook@PC-FRANCESCO"
	if s := b.claimConToken(tokenDelWorker(nome), nome); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("il worker non entra nemmeno prima: %d", s)
	}
	ad := b.browser("10.0.0.9:5000")
	ad.login("AD", "prova-ad")

	b.ws.Workers = fsGuasto{risorse.FS}
	resp, _ := ad.fai(http.MethodPost, "/admin/postazioni/PC-FRANCESCO/pacchetto", url.Values{}, false)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("pacchetto con il disco guasto: %d, atteso 500", resp.StatusCode)
	}
	if s := b.claimConToken(tokenDelWorker(nome), nome); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("dopo uno zip non costruito il token di prima non vale piu' (%d): il segreto nuovo e' andato perso", s)
	}

	b.ws.Workers = risorse.FS
	dentro := scarica(t, ad, "PC-FRANCESCO")
	if !strings.Contains(dentro["worker.toml"], `worker_id = "`+nome+`"`) {
		t.Fatalf("il pacchetto non porta il worker:\n%s", dentro["worker.toml"])
	}
	if s := b.claimConToken(tokenDelWorker(nome), nome); s != http.StatusUnauthorized {
		t.Errorf("con il pacchetto nuovo il token di prima vale ancora: %d", s)
	}
}

// Salvare le lavorazioni e' tutto o niente: una lavorazione che non c'e' nel catalogo ferma il
// salvataggio, e quelle tolte nello stesso giro restano dov'erano.
func TestLeLavorazioniDiUnFornitoreSiSalvanoTutteONiente(t *testing.T) {
	b := preparaBancoWeb(t)
	f := b.unFornitore("Fornitore Esempio", db.TipoFornitoreVerniciatore, "fornitore.example", "cataforesi")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	_, html := ad.fai(http.MethodPost, "/admin/fornitori/"+f.FornitoreID.String()+"/lavorazioni",
		url.Values{"lavorazione": {"inesistente"}}, false)
	if !strings.Contains(leggibile(html), `la lavorazione "inesistente" non esiste nel catalogo`) {
		t.Errorf("il rifiuto non dice perche':\n%s", estrai(html, "errore"))
	}
	attuali, err := b.q.ListLavorazioniFornitore(b.ctx, f.FornitoreID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attuali) != 1 || attuali[0].Codice != "cataforesi" {
		t.Errorf("dopo il rifiuto le lavorazioni sono %v: la cataforesi e' stata tolta a meta' giro", attuali)
	}
}
