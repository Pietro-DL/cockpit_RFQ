//go:build integrazione

// L4 — la cache delle anteprime nel browser (cache C1, domanda 31a = A), contro un PostgreSQL vero, file veri
// su disco e il giro HTTP completo con la sessione: lo stesso banco delle altre prove dell'anteprima
// (preparaAnteprima).
//
// Che cosa si guarda, per ogni risposta: l'intestazione `Cache-Control` e la riga «anteprima servita» del log.
//
//	dallo staging, con l'impronta giusta     → immutable, uguale per il 200, il 206 e il 304
//	dal NAS, anche con l'impronta giusta     → must-revalidate, come prima
//	un percorso o un'impronta diversi        → must-revalidate, come prima
//	ogni errore (400, 404, 409, 415, 416)    → no-store
//	la riga di log                           → lo stato HTTP e chi ha chiesto (la misura, domanda 31d)
//	una copia presa altrove, poi lo staging  → il file intero e verificato, non un 304 definitivo
package web

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// conImpronta e' l'indirizzo che le pagine costruiscono: quello dell'allegato con lo sha256 del suo contenuto.
func (s *scenaAnteprima) conImpronta() string { return s.url() + "?v=" + shaDi(s.pdf) }

// campiDellaServita legge la riga «anteprima servita» della richiesta appena fatta e controlla i campi dati.
func (s *scenaAnteprima) campiDellaServita(t *testing.T, attesi ...string) {
	t.Helper()
	riga := s.reg.rigaServita(t)
	for _, a := range attesi {
		if !strings.Contains(riga, a) {
			t.Errorf("la riga «anteprima servita» non dice %q:\n%s", a, riga)
		}
	}
}

// cache C1, prova 5 — dallo staging, con l'impronta dell'allegato, la risposta e' definitiva: il browser non
// richiede piu' niente. Il 304 e il 206 della stessa richiesta portano la stessa intestazione del 200: un 304
// rinfresca la copia che il browser ha, e con un'intestazione diversa la cambierebbe. La riga di log dice lo
// stato e chi ha chiesto.
func TestCacheC1DalloStagingConLImprontaLaRispostaEDefinitiva(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(64*1024), true, false)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.conImpronta(), nil)
	if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
		t.Fatalf("stato %d, %d byte: %s", resp.StatusCode, len(corpo), primi400(string(corpo)))
	}
	if cc := resp.Header.Get("Cache-Control"); cc != cacheDefinitiva {
		t.Errorf("200 dallo staging con l'impronta: Cache-Control = %q, atteso %q", cc, cacheDefinitiva)
	}
	etag := resp.Header.Get("ETag")
	if etag != `"`+shaDi(s.pdf)+`"` {
		t.Errorf("ETag = %q", etag)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("le intestazioni che impediscono al PDF di eseguire qualcosa non ci sono piu': %v", resp.Header)
	}
	s.campiDellaServita(t, "stato=200", "da=staging", "cache=definitiva", "utente=FP", "ip=10.0.0.5")

	resp2, corpo2 := s.fp.chiedi(http.MethodGet, s.conImpronta(), map[string]string{"If-None-Match": etag})
	if resp2.StatusCode != http.StatusNotModified || len(corpo2) != 0 {
		t.Fatalf("seconda richiesta con If-None-Match: stato %d, %d byte", resp2.StatusCode, len(corpo2))
	}
	if cc := resp2.Header.Get("Cache-Control"); cc != cacheDefinitiva {
		t.Errorf("304: Cache-Control = %q, atteso lo stesso del 200 (%q)", cc, cacheDefinitiva)
	}
	s.campiDellaServita(t, "stato=304", "da=staging", "cache=definitiva", "byte_letti=5", "utente=FP")

	resp3, corpo3 := s.fp.chiedi(http.MethodGet, s.conImpronta(), map[string]string{"Range": "bytes=0-1023"})
	if resp3.StatusCode != http.StatusPartialContent || len(corpo3) != 1024 {
		t.Fatalf("Range: stato %d, %d byte", resp3.StatusCode, len(corpo3))
	}
	if cc := resp3.Header.Get("Cache-Control"); cc != cacheDefinitiva {
		t.Errorf("206: Cache-Control = %q, atteso %q", cc, cacheDefinitiva)
	}
	s.campiDellaServita(t, "stato=206", "cache=definitiva")
}

// cache C1, prova 6 — lo stesso file dallo staging, ma senza l'impronta o con quella di un altro contenuto: si
// rivalida, come prima di C1. Il file si serve lo stesso (e' il contenuto di adesso): solo il browser non puo'
// tenerlo senza chiedere, perche' l'indirizzo non promette quel contenuto.
func TestCacheC1SenzaLImprontaGiustaSiRivalida(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(32*1024), true, false)
	altro := shaDi(pdfFinto(16 * 1024))

	for _, u := range []string{s.url(), s.url() + "?v=" + altro, s.url() + "?v=" + strings.ToUpper(shaDi(s.pdf))} {
		resp, corpo := s.fp.chiedi(http.MethodGet, u, nil)
		if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
			t.Fatalf("%s: stato %d", u, resp.StatusCode)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != cacheRivalida {
			t.Errorf("%s: Cache-Control = %q, atteso %q", u, cc, cacheRivalida)
		}
		s.campiDellaServita(t, "stato=200", "da=staging", "cache=rivalida")

		resp2, _ := s.fp.chiedi(http.MethodGet, u, map[string]string{"If-None-Match": resp.Header.Get("ETag")})
		if resp2.StatusCode != http.StatusNotModified {
			t.Fatalf("%s con If-None-Match: stato %d", u, resp2.StatusCode)
		}
		if cc := resp2.Header.Get("Cache-Control"); cc != cacheRivalida {
			t.Errorf("%s, 304: Cache-Control = %q, atteso lo stesso del 200 (%q)", u, cc, cacheRivalida)
		}
		s.campiDellaServita(t, "stato=304", "cache=rivalida")
	}
}

// cache C1, prova 7 — dal NAS mai definitiva, anche con l'impronta giusta: li' il file e' verificato solo per
// dimensione e data, e un file sostituito resterebbe in cache per sempre sotto l'indirizzo dell'hash giusto.
// Anche il 304 si rivalida, e anche quando il dubbio fa rileggere il file e l'hash torna.
func TestCacheC1DalNasSiRivalidaAncheConLImpronta(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(64*1024), false, true)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.conImpronta(), nil)
	if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if cc := resp.Header.Get("Cache-Control"); cc != cacheRivalida {
		t.Errorf("200 dal NAS: Cache-Control = %q, atteso %q", cc, cacheRivalida)
	}
	s.campiDellaServita(t, "stato=200", "da=NAS", "cache=rivalida", "utente=FP", "ip=10.0.0.5")

	resp2, _ := s.fp.chiedi(http.MethodGet, s.conImpronta(), map[string]string{"If-None-Match": resp.Header.Get("ETag")})
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("seconda richiesta: stato %d", resp2.StatusCode)
	}
	if cc := resp2.Header.Get("Cache-Control"); cc != cacheRivalida {
		t.Errorf("304 dal NAS: Cache-Control = %q, atteso %q", cc, cacheRivalida)
	}
	s.campiDellaServita(t, "stato=304", "da=NAS", "cache=rivalida")

	// il dubbio: il file risulta toccato dopo l'ultimo sguardo, si rilegge per intero e l'hash torna
	if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE documento SET scritto_il = now() - interval '1 day' WHERE documento_id=$1`, s.doc); err != nil {
		t.Fatal(err)
	}
	resp3, _ := s.fp.chiedi(http.MethodGet, s.conImpronta(), nil)
	if resp3.StatusCode != 200 {
		t.Fatalf("con il dubbio: stato %d", resp3.StatusCode)
	}
	if cc := resp3.Header.Get("Cache-Control"); cc != cacheRivalida {
		t.Errorf("con il dubbio: Cache-Control = %q, atteso %q", cc, cacheRivalida)
	}
	s.campiDellaServita(t, "stato=200", "da=NAS", "cache=rivalida")
}

// cache C1, prova 8 — dallo staging, con l'impronta che la pagina costruirebbe, ma i byte non stanno nel posto
// del contenuto con lo sha256 dell'allegato: si rivalida. Tre modi: lo staging vecchio per messaggio, un file che
// si chiama come un altro hash (in un'altra sottocartella, o nella stessa perche' i due hash cominciano allo stesso
// modo), e l'allegato che ha cambiato contenuto a parita' di id (il suo sha256 non e' piu' quello del file che
// path_staging nomina). E l'ETag non e' quello nudo del contenuto verificato (correzione 1).
func TestCacheC1UnPercorsoOUnImprontaDiversiSiRivalidano(t *testing.T) {
	sposta := func(t *testing.T, s *scenaAnteprima, dove string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(dove), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dove, s.pdf, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := esegui(s.b, `UPDATE allegato SET path_staging=$2 WHERE allegato_id=$1`, s.allegato, dove); err != nil {
			t.Fatal(err)
		}
	}
	altro := shaDi(pdfFinto(12 * 1024)) // un contenuto diverso da quello della scena (16 KB)
	casi := []struct {
		nome  string
		prima func(t *testing.T, s *scenaAnteprima) string // prepara, e dice l'impronta da chiedere
	}{
		{"lo staging vecchio, per messaggio", func(t *testing.T, s *scenaAnteprima) string {
			sposta(t, s, filepath.Join(s.radiceStaging, "3f2a9c1d", "disegno.pdf"))
			return shaDi(s.pdf)
		}},
		{"un file che si chiama come un altro hash", func(t *testing.T, s *scenaAnteprima) string {
			sposta(t, s, filepath.Join(s.radiceStaging, "_contenuti", altro[:2], altro+".pdf"))
			return shaDi(s.pdf)
		}},
		// correzione 1: la sottocartella e' quella giusta, e a dire no resta solo il confronto del nome con l'hash
		{"un file con il nome di un altro hash, nella sottocartella giusta", func(t *testing.T, s *scenaAnteprima) string {
			sha := shaDi(s.pdf)
			sposta(t, s, filepath.Join(s.radiceStaging, "_contenuti", sha[:2], sha[:2]+altro[2:]+".pdf"))
			return sha
		}},
		{"l'allegato con un altro sha256, chiesto con quello", func(t *testing.T, s *scenaAnteprima) string {
			if err := esegui(s.b, `UPDATE allegato SET sha256=$2 WHERE allegato_id=$1`, s.allegato, altro); err != nil {
				t.Fatal(err)
			}
			return altro
		}},
		{"l'allegato con un altro sha256, chiesto con quello di prima", func(t *testing.T, s *scenaAnteprima) string {
			if err := esegui(s.b, `UPDATE allegato SET sha256=$2 WHERE allegato_id=$1`, s.allegato, altro); err != nil {
				t.Fatal(err)
			}
			return shaDi(s.pdf)
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			s := preparaAnteprima(t, pdfFinto(16*1024), true, false)
			v := c.prima(t, s)
			resp, corpo := s.fp.chiedi(http.MethodGet, s.url()+"?v="+v, nil)
			if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
				t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
			}
			if cc := resp.Header.Get("Cache-Control"); cc != cacheRivalida {
				t.Errorf("Cache-Control = %q, atteso %q", cc, cacheRivalida)
			}
			if etag := resp.Header.Get("ETag"); !strings.HasPrefix(etag, `"staging-`) {
				t.Errorf("ETag = %q: questi byte non sono il contenuto verificato, e l'ETag nudo e' solo suo", etag)
			}
			s.campiDellaServita(t, "stato=200", "da=staging", "cache=rivalida")
		})
	}
}

// cache C1, prova 9 — ogni risposta d'errore esce con `no-store`: un 404 o un 409 tenuto dal browser
// resterebbe sullo schermo anche quando il file torna. Anche gli errori che scrive ServeContent (un Range fuori
// dal file: 416), che il Cache-Control messo per il file lo toglie.
func TestCacheC1GliErroriNonSiTengono(t *testing.T) {
	controlla := func(t *testing.T, cosa string, resp *http.Response, stato int) {
		t.Helper()
		if resp.StatusCode != stato {
			t.Fatalf("%s: stato %d, atteso %d", cosa, resp.StatusCode, stato)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != cacheMai {
			t.Errorf("%s (%d): Cache-Control = %q, atteso %q", cosa, stato, cc, cacheMai)
		}
	}

	s := preparaAnteprima(t, pdfFinto(8*1024), true, false)
	resp, _ := s.fp.chiedi(http.MethodGet, "/allegato/"+uuid.NewString()+"/anteprima?v="+shaDi(s.pdf), nil)
	controlla(t, "allegato inesistente", resp, http.StatusNotFound)
	resp, _ = s.fp.chiedi(http.MethodGet, "/allegato/non-un-id/anteprima", nil)
	controlla(t, "id non valido", resp, http.StatusBadRequest)
	resp, _ = s.fp.chiedi(http.MethodGet, s.conImpronta(), map[string]string{"HX-Request": "true"})
	controlla(t, "chiesta da htmx", resp, http.StatusBadRequest)
	resp, _ = s.fp.chiedi(http.MethodGet, s.conImpronta(), map[string]string{"Range": "bytes=999999-"})
	controlla(t, "Range fuori dal file", resp, http.StatusRequestedRangeNotSatisfiable)

	nonPdf := preparaAnteprima(t, []byte("PK\x03\x04 uno zip con il nome sbagliato"), true, false)
	resp, _ = nonPdf.fp.chiedi(http.MethodGet, nonPdf.conImpronta(), nil)
	controlla(t, "non e' un PDF", resp, http.StatusUnsupportedMediaType)

	// nessun documento con questo contenuto e niente staging: «usa Riscarica»
	nulla := preparaAnteprima(t, pdfFinto(8*1024), false, true)
	if err := esegui(nulla.b, `UPDATE documento SET sha256=repeat('0',64) WHERE thread_id=$1`, nulla.thread); err != nil {
		t.Fatal(err)
	}
	resp, _ = nulla.fp.chiedi(http.MethodGet, nulla.conImpronta(), nil)
	controlla(t, "il contenuto non e' su questo server", resp, http.StatusNotFound)

	// il documento dice «scritto» e il file sul NAS non c'e': la segnalazione nasce, e il 409 non si tiene
	sparito := preparaAnteprima(t, pdfFinto(8*1024), false, false)
	resp, _ = sparito.fp.chiedi(http.MethodGet, sparito.conImpronta(), nil)
	controlla(t, "il file sparito dal NAS", resp, http.StatusConflict)

	segnalato := preparaAnteprima(t, pdfFinto(8*1024), false, true)
	if _, err := segnalato.b.q.ApriAnomaliaNas(segnalato.b.ctx, db.ApriAnomaliaNasParams{
		DocumentoID: segnalato.doc, ThreadID: segnalato.thread, Problema: db.ProblemaNasConflitto,
		StatoDb: db.StatoNasScritto, Percorso: pathDocProva, ShaAtteso: shaDi(segnalato.pdf), Dettaglio: "segnalato dal ricognitore",
	}); err != nil {
		t.Fatal(err)
	}
	resp, _ = segnalato.fp.chiedi(http.MethodGet, segnalato.conImpronta(), nil)
	controlla(t, "una segnalazione aperta sul NAS", resp, http.StatusConflict)
}

// cache C1, prova 12 (correzione 1) — una copia presa altrove non diventa definitiva con un 304. Il browser ha il
// file dal NAS, o da un posto dello staging che non e' un contenuto verificato; poi lo stesso contenuto torna al
// suo posto nello staging (il file esce dopo 30 giorni, «Riscarica» lo riporta; oppure c'era gia', per un altro
// allegato con gli stessi byte). Un 304 rinfresca la copia che il browser ha: con `immutable` quella copia
// varrebbe un anno. Quindi la richiesta successiva, con le intestazioni della copia del browser, riceve il file
// intero e verificato: con If-None-Match, con If-Range (un pezzo da completare) e con la sola data
// (If-Modified-Since). Solo con l'ETag del contenuto verificato il 304 definitivo torna.
func TestCacheC1UnaCopiaPresaAltroveNonDiventaDefinitivaCon304(t *testing.T) {
	// alSuoPosto mette il contenuto in _contenuti\<ab>\<sha256>.pdf e ci fa puntare l'allegato. Il file ha la
	// data di un mese fa: e' il caso in cui la data della copia del browser e' PIU' RECENTE di quella del file.
	alSuoPosto := func(t *testing.T, s *scenaAnteprima) {
		t.Helper()
		sha := shaDi(s.pdf)
		dove := filepath.Join(s.radiceStaging, "_contenuti", sha[:2], sha+".pdf")
		if err := os.MkdirAll(filepath.Dir(dove), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dove, s.pdf, 0o644); err != nil {
			t.Fatal(err)
		}
		unMeseFa := time.Now().Add(-30 * 24 * time.Hour)
		if err := os.Chtimes(dove, unMeseFa, unMeseFa); err != nil {
			t.Fatal(err)
		}
		if err := esegui(s.b, `UPDATE allegato SET path_staging=$2 WHERE allegato_id=$1`, s.allegato, dove); err != nil {
			t.Fatal(err)
		}
	}
	casi := []struct {
		nome  string
		scena func(t *testing.T) *scenaAnteprima // il file dove il browser lo prende la prima volta
		da    string
		etag  string // l'inizio dell'ETag della prima risposta
	}{
		{"dal NAS", func(t *testing.T) *scenaAnteprima {
			return preparaAnteprima(t, pdfFinto(16*1024), false, true)
		}, "da=NAS", `"nas-`},
		{"dallo staging vecchio, per messaggio", func(t *testing.T) *scenaAnteprima {
			s := preparaAnteprima(t, pdfFinto(16*1024), false, false)
			vecchio := filepath.Join(s.radiceStaging, "3f2a9c1d", "disegno.pdf")
			if err := os.MkdirAll(filepath.Dir(vecchio), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(vecchio, s.pdf, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := esegui(s.b, `UPDATE allegato SET path_staging=$2 WHERE allegato_id=$1`, s.allegato, vecchio); err != nil {
				t.Fatal(err)
			}
			return s
		}, "da=staging", `"staging-`},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			s := c.scena(t)
			nudo := `"` + shaDi(s.pdf) + `"`

			// la prima volta: la copia del browser, che si rivalida, con la sua data e il suo ETag
			resp, corpo := s.fp.chiedi(http.MethodGet, s.conImpronta(), nil)
			if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
				t.Fatalf("prima richiesta: stato %d: %s", resp.StatusCode, primi400(string(corpo)))
			}
			copia, data := resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
			if resp.Header.Get("Cache-Control") != cacheRivalida || !strings.HasPrefix(copia, c.etag) || data == "" {
				t.Errorf("prima richiesta: Cache-Control %q, ETag %q, Last-Modified %q", resp.Header.Get("Cache-Control"), copia, data)
			}
			s.campiDellaServita(t, "stato=200", c.da, "cache=rivalida")

			alSuoPosto(t, s)
			definitiva := func(t *testing.T, cosa string, intestazioni map[string]string) {
				t.Helper()
				resp, corpo := s.fp.chiedi(http.MethodGet, s.conImpronta(), intestazioni)
				if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
					t.Errorf("%s: stato %d, %d byte: la copia presa altrove deve ricevere il file intero e verificato",
						cosa, resp.StatusCode, len(corpo))
				}
				if cc := resp.Header.Get("Cache-Control"); cc != cacheDefinitiva {
					t.Errorf("%s: Cache-Control = %q, atteso %q", cosa, cc, cacheDefinitiva)
				}
				if etag := resp.Header.Get("ETag"); etag != nudo {
					t.Errorf("%s: ETag = %q, atteso quello del contenuto verificato %s", cosa, etag, nudo)
				}
				if lm := resp.Header.Get("Last-Modified"); lm != "" {
					t.Errorf("%s: la risposta definitiva porta la data (%s): si rivalida solo con l'ETag", cosa, lm)
				}
				s.campiDellaServita(t, "stato=200", "da=staging", "cache=definitiva")
			}
			definitiva(t, "If-None-Match della copia", map[string]string{"If-None-Match": copia})
			definitiva(t, "If-Range della copia, per completare un pezzo", map[string]string{"Range": "bytes=1024-2047", "If-Range": copia})
			definitiva(t, "If-Modified-Since della copia, senza ETag", map[string]string{"If-Modified-Since": data})

			// con l'ETag del contenuto verificato, invece, il 304 definitivo c'e'
			resp, corpo = s.fp.chiedi(http.MethodGet, s.conImpronta(), map[string]string{"If-None-Match": nudo})
			if resp.StatusCode != http.StatusNotModified || len(corpo) != 0 {
				t.Fatalf("con l'ETag del contenuto verificato: stato %d, %d byte", resp.StatusCode, len(corpo))
			}
			if cc := resp.Header.Get("Cache-Control"); cc != cacheDefinitiva {
				t.Errorf("304 del contenuto verificato: Cache-Control = %q, atteso %q", cc, cacheDefinitiva)
			}
			s.campiDellaServita(t, "stato=304", "da=staging", "cache=definitiva")
		})
	}
}
