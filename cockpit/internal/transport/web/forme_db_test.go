//go:build integrazione

// L4 — Smistamento, giro 4, fase 4.17a: la pagina «Forme viste» dell'Anagrafica e il suo export Markdown. Sono
// dell'amministratore (403 per chi lavora e per chi consulta, anche dalla barra degli indirizzi), leggono il
// database vero e non lo cambiano; l'export e' un download (text/markdown, allegato, mai in cache) con lo
// stesso contenuto della pagina.
package web

import (
	"net/http"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/contratti/worker"
)

func TestLeFormeVisteSonoDellAmministratoreENonScrivono(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	msg := b.mailConAllegati("RDO 400012345 - Richiesta offerta 7120001", "In allegato il disegno 7120001.",
		worker.AllegatoIn{Indice: 1, NomeFile: "7120001_PRT.stp", Estensione: "stp", Natura: "file", Bytes: 9000},
		worker.AllegatoIn{Indice: 2, NomeFile: "7120001A_1.pdf", Estensione: "pdf", Natura: "file", Bytes: 9000})
	thread := b.rfqDa(msg, acme, `ACME\WIP\2026 09 29 RFQ 7120001`, "7120001")
	utente := uuidSQL(t, b, `SELECT utente_id FROM utente WHERE sigla = 'LU'`)
	if _, err := b.pool.Exec(b.ctx, `UPDATE identificativo_thread SET confermato_da = $2 WHERE thread_id = $1`, thread, utente); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da, descrizione)
		VALUES ($1, '7120001', 'finito', $2, 'SUPPORTO'), ($1, '7120010', 'commerciale', $2, 'VITE TE M8X20 UNI 5739')`, thread, utente); err != nil {
		t.Fatal(err)
	}

	op := operatore(b)
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	prima := fotoDelDatabase(t, b)

	for _, percorso := range []string{"/admin/anagrafica/forme", "/admin/anagrafica/forme.md"} {
		for _, chi := range []struct {
			nome string
			w    *browser
		}{{"operatore", op}, {"consultazione", co}} {
			for _, hx := range []bool{false, true} {
				resp, corpo := chi.w.fai(http.MethodGet, percorso, nil, hx)
				if resp.StatusCode != http.StatusForbidden {
					t.Errorf("%s, GET %s (htmx %v): %d, atteso 403", chi.nome, percorso, hx, resp.StatusCode)
				}
				if strings.Contains(corpo, "7120001") {
					t.Errorf("%s, GET %s: il 403 porta i dati del censimento", chi.nome, percorso)
				}
			}
		}
	}

	resp, html := ad.fai(http.MethodGet, "/admin/anagrafica/forme", nil, false)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("la pagina: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, atteso := range []string{"<title>Anagrafica ·", `href="/admin/anagrafica" class="attivo" aria-current="page"`, "<h1>Forme viste</h1>",
		"Acme S.p.A.", `href="/admin/anagrafica/forme.md"`, "transazione di sola lettura: sì",
		"9999999_AAA", "_PRT", "Pezzi decisi: 1 particolari commerciali, 1 altri", "<td>VITE</td>", "arriva con la fase 4.4a.1"} {
		if !strings.Contains(html, atteso) {
			t.Errorf("manca %q nella pagina", atteso)
		}
	}

	resp, md := ad.fai(http.MethodGet, "/admin/anagrafica/forme.md", nil, false)
	if resp.StatusCode != 200 {
		t.Fatalf("l'export: %d", resp.StatusCode)
	}
	h := resp.Header
	if h.Get("Content-Type") != "text/markdown; charset=utf-8" || !strings.HasPrefix(h.Get("Content-Disposition"), `attachment; filename="forme_viste_`) ||
		!strings.HasSuffix(h.Get("Content-Disposition"), `.md"`) || h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("le intestazioni del download: %v", h)
	}
	for _, atteso := range []string{"# Forme viste\n", "## ACME — Acme S.p.A.\n", "transazione di sola lettura: sì", "| `_PRT` | `_AAA` |",
		"| `RDO 999999999` |", "| `9999999` | 1 | 1 |"} {
		if !strings.Contains(md, atteso) {
			t.Errorf("manca %q nell'export", atteso)
		}
	}

	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("le forme viste hanno scritto in: %s", d)
	}
}
