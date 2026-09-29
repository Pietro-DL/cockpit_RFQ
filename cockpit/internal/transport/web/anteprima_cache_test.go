// L1 — la cache delle anteprime nel browser (cache C1, domanda 31a = A), senza database.
//
// La decisione che conta sta tutta in una funzione pura, cacheAnteprima: qui la si guarda caso per caso, e
// soprattutto nei casi in cui deve dire no. Un `immutable` dato per sbaglio non lo vede nessuno: il browser
// smette di chiedere, e il file sbagliato resta sullo schermo di chi l'ha aperto una volta, per un anno.
//
// Poi l'altra meta' di C1: le pagine che costruiscono l'indirizzo lo costruiscono con l'impronta, perche' e'
// l'impronta a rendere l'indirizzo di un contenuto e non di un allegato (lo sha256 di un allegato puo' cambiare
// a parita' di id). Le prove contro il database vero e il giro HTTP completo sono in anteprima_cache_db_test.go.
package web

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/db"
)

// cache C1, prova 1 — la decisione dell'intestazione. Definitiva solo con tutte le condizioni vere insieme:
// l'impronta chiesta e' lo sha256 dell'allegato, i byte vengono dallo staging, dal posto del contenuto con quello
// sha256 (_contenuti\<ab>\<sha256>.<ext>), e nessun dubbio e' scattato. Ogni altro caso si rivalida, come prima.
func TestCacheC1LaDecisioneDellIntestazione(t *testing.T) {
	radice := filepath.Join(t.TempDir(), "staging")
	sha := strings.Repeat("ab", 32)
	altro := strings.Repeat("cd", 32)
	contenuto := func(h, ext string) string { return filepath.Join(radice, "_contenuti", h[:2], h+ext) }
	dalloStaging := func(p string) *sorgente { return &sorgente{da: "staging", percorso: p} }
	sep := string(filepath.Separator)

	casi := []struct {
		nome   string
		radice string
		sha, v string
		src    *sorgente
		atteso string
	}{
		{"staging, il posto del contenuto, l'impronta giusta", radice, sha, sha, dalloStaging(contenuto(sha, ".pdf")), cacheDefinitiva},
		{"lo stesso contenuto arrivato con un altro nome (.bin)", radice, sha, sha, dalloStaging(contenuto(sha, ".bin")), cacheDefinitiva},
		{"la radice della configurazione in maiuscolo (NTFS)", strings.ToUpper(radice), sha, sha, dalloStaging(contenuto(sha, ".pdf")), cacheDefinitiva},
		{"la radice con la barra in fondo", radice + sep, sha, sha, dalloStaging(contenuto(sha, ".pdf")), cacheDefinitiva},
		{"lo sha256 dell'allegato scritto in maiuscolo", radice, strings.ToUpper(sha), sha, dalloStaging(contenuto(sha, ".pdf")), cacheDefinitiva},

		// l'indirizzo
		{"senza impronta", radice, sha, "", dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},
		{"l'impronta di un altro contenuto", radice, sha, altro, dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},
		{"l'impronta troncata", radice, sha, sha[:12], dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},
		{"l'impronta in maiuscolo (nessuna pagina la scrive cosi')", radice, sha, strings.ToUpper(sha), dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},
		{"l'allegato senza sha256", radice, "", "", dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},
		{"l'allegato con uno sha256 che non e' un hash", radice, "non-un-hash", "non-un-hash", dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},

		// da dove e come
		{"dal NAS", radice, sha, sha, &sorgente{da: "NAS", percorso: contenuto(sha, ".pdf")}, cacheRivalida},
		{"dallo staging, ma un dubbio e' scattato", radice, sha, sha, &sorgente{da: "staging", percorso: contenuto(sha, ".pdf"), inDubbio: true}, cacheRivalida},
		{"nessuna sorgente", radice, sha, sha, nil, cacheRivalida},
		{"la radice dello staging non dichiarata", "", sha, sha, dalloStaging(contenuto(sha, ".pdf")), cacheRivalida},

		// il percorso
		{"il file si chiama come un ALTRO hash (l'allegato ha cambiato contenuto)", radice, sha, sha, dalloStaging(contenuto(altro, ".pdf")), cacheRivalida},
		// correzione 1: la sottocartella giusta e il nome di un altro hash che comincia allo stesso modo. E' il solo
		// caso in cui a dire no e' il confronto del nome con l'hash, e non quello della sottocartella.
		{"la sottocartella giusta, il nome di un altro hash con lo stesso inizio", radice, sha, sha,
			dalloStaging(filepath.Join(radice, "_contenuti", sha[:2], sha[:2]+altro[2:]+".pdf")), cacheRivalida},
		{"la sottocartella di un altro hash", radice, sha, sha, dalloStaging(filepath.Join(radice, "_contenuti", "zz", sha+".pdf")), cacheRivalida},
		{"lo staging vecchio, per messaggio", radice, sha, sha, dalloStaging(filepath.Join(radice, "3f2a9c1d", "disegno.pdf")), cacheRivalida},
		{"lo staging vecchio, con il nome dell'hash ma fuori da _contenuti", radice, sha, sha, dalloStaging(filepath.Join(radice, "3f2a9c1d", sha+".pdf")), cacheRivalida},
		{"una parte in caricamento", radice, sha, sha, dalloStaging(filepath.Join(radice, "_parti", uuid.NewString()+".parte."+uuid.NewString())), cacheRivalida},
		{"un livello in piu' sotto _contenuti", radice, sha, sha, dalloStaging(filepath.Join(radice, "_contenuti", sha[:2], "x", sha+".pdf")), cacheRivalida},
		{"la cartella dei contenuti scritta diversamente", radice, sha, sha, dalloStaging(filepath.Join(radice, "_CONTENUTI", sha[:2], sha+".pdf")), cacheRivalida},
		{"l'estensione in maiuscolo (non l'ha scritta PercorsoContenuto)", radice, sha, sha, dalloStaging(contenuto(sha, ".PDF")), cacheRivalida},
		{"senza estensione", radice, sha, sha, dalloStaging(contenuto(sha, "")), cacheRivalida},
		{"fuori dalla radice, in un'altra cartella di contenuti", radice, sha, sha,
			dalloStaging(filepath.Join(filepath.Dir(radice), "altro", "_contenuti", sha[:2], sha+".pdf")), cacheRivalida},
		{"una radice che comincia come questa", radice, sha, sha,
			dalloStaging(filepath.Join(radice+"-vecchia", "_contenuti", sha[:2], sha+".pdf")), cacheRivalida},
		// correzione 1: un'altra radice lunga quanto questa, con sotto lo stesso _contenuti\<ab>\<sha256>. Qui a
		// dire no e' solo il confronto della radice: il numero delle parti e i loro nomi tornano.
		{"un'altra radice lunga quanto questa", radice, sha, sha,
			dalloStaging(filepath.Join(filepath.Dir(radice), "stagin2", "_contenuti", sha[:2], sha+".pdf")), cacheRivalida},
	}
	for _, c := range casi {
		if got := cacheAnteprima(c.radice, c.sha, c.v, c.src); got != c.atteso {
			t.Errorf("%s: %q, atteso %q", c.nome, got, c.atteso)
		}
	}
}

// cache C1, prova 11 (correzione 1) — l'ETag nudo, lo sha256 e basta, solo per il contenuto verificato: lo
// staging, il posto del contenuto con quello sha256, nessun dubbio. Ogni altra fonte ha l'ETag con davanti da
// dove viene. E' cio' che impedisce a un 304 definitivo di arrivare a una copia presa altrove: un 304 rinfresca
// la copia che il browser ha, e se il NAS e lo staging dessero lo stesso ETag la copia del NAS varrebbe un anno.
// L'ETag non dipende dall'indirizzo: lo stesso file ha lo stesso ETag con o senza l'impronta.
func TestCacheC1LETagNudoSoloPerIlContenutoVerificato(t *testing.T) {
	radice := filepath.Join(t.TempDir(), "staging")
	sha := strings.Repeat("ab", 32)
	altro := strings.Repeat("cd", 32)
	contenuto := filepath.Join(radice, "_contenuti", sha[:2], sha+".pdf")
	src := func(da, percorso string, dubbio bool) *sorgente {
		return &sorgente{da: da, percorso: percorso, etag: sha, inDubbio: dubbio}
	}
	nudo, dalNas, dalloStaging := `"`+sha+`"`, `"nas-`+sha+`"`, `"staging-`+sha+`"`

	casi := []struct {
		nome   string
		radice string
		sha    string
		src    *sorgente
		atteso string
	}{
		{"staging, il posto del contenuto", radice, sha, src("staging", contenuto, false), nudo},
		{"lo sha256 dell'allegato in maiuscolo: l'ETag e' in minuscolo, come l'impronta", radice, strings.ToUpper(sha),
			&sorgente{da: "staging", percorso: contenuto, etag: strings.ToUpper(sha)}, nudo},

		{"dal NAS", radice, sha, src("NAS", `\\nas\rfq\ACME\7120001.pdf`, false), dalNas},
		{"dal NAS, riletto per il dubbio e con l'hash tornato", radice, sha, src("NAS", `\\nas\rfq\ACME\7120001.pdf`, true), dalNas},
		{"dal NAS, anche se il percorso fosse quello di un contenuto", radice, sha, src("NAS", contenuto, false), dalNas},
		{"lo staging vecchio, per messaggio", radice, sha, src("staging", filepath.Join(radice, "3f2a9c1d", "disegno.pdf"), false), dalloStaging},
		{"lo staging, un file con il nome di un altro hash", radice, sha,
			src("staging", filepath.Join(radice, "_contenuti", sha[:2], sha[:2]+altro[2:]+".pdf"), false), dalloStaging},
		{"lo staging, il posto del contenuto, ma un dubbio e' scattato", radice, sha, src("staging", contenuto, true), dalloStaging},
		{"lo staging, la radice non dichiarata", "", sha, src("staging", contenuto, false), dalloStaging},
		{"l'allegato ha un altro sha256 (il file e' quello di prima)", radice, altro,
			&sorgente{da: "staging", percorso: contenuto, etag: altro}, `"staging-` + altro + `"`},

		{"senza sha256 nel database", radice, "", &sorgente{da: "staging", percorso: contenuto}, ""},
		{"nessuna sorgente", radice, sha, nil, ""},
	}
	for _, c := range casi {
		got := etagAnteprima(c.radice, c.sha, c.src)
		if got != c.atteso {
			t.Errorf("%s: ETag %q, atteso %q", c.nome, got, c.atteso)
		}
		// la stessa garanzia della risposta definitiva, e nessun'altra
		nudoQui, verificato := got != "" && !strings.Contains(got, "-"), contenutoVerificato(c.radice, c.sha, c.src)
		if nudoQui != verificato {
			t.Errorf("%s: ETag nudo = %v, contenuto verificato = %v", c.nome, nudoQui, verificato)
		}
		if cacheAnteprima(c.radice, c.sha, strings.ToLower(c.sha), c.src) == cacheDefinitiva && got != nudo {
			t.Errorf("%s: la risposta definitiva ha l'ETag %q, non quello nudo", c.nome, got)
		}
	}
}

// cache C1, prova 2 — l'indirizzo porta l'impronta quando c'e' un hash, e solo allora; sempre in minuscolo,
// perche' la rotta la confronta cosi' com'e'.
func TestCacheC1LIndirizzoPortaLImpronta(t *testing.T) {
	id := uuid.New()
	base := "/allegato/" + id.String() + "/anteprima"
	sha := strings.Repeat("0f", 32)
	casi := []struct{ sha, atteso string }{
		{sha, base + "?v=" + sha},
		{" " + strings.ToUpper(sha) + " ", base + "?v=" + sha},
		{"", base},
		{"non-un-hash", base},
		{sha[:63], base},
		{sha[:63] + "g", base},
	}
	for _, c := range casi {
		if got := indirizzoAnteprima(id, c.sha); got != c.atteso {
			t.Errorf("sha %q: %q, atteso %q", c.sha, got, c.atteso)
		}
	}
}

// cache C1, prova 3 — le pagine costruite dal server passano l'impronta: il link «Anteprima» dell'Inbox, l'iframe
// del pannello del Fascicolo, l'iframe delle schede della pagina Richieste. Senza impronta (un allegato senza
// hash) l'indirizzo resta quello di prima.
func TestCacheC1LePagineDelServerPassanoLImpronta(t *testing.T) {
	sha := strings.Repeat("e1", 32)
	s := serverTest(t)

	// l'Inbox: il link della riga dell'allegato
	a := AllegatoUI{Allegato: db.Allegato{AllegatoID: uuid.New(), NomeFile: "7120001.pdf", Estensione: txtT("pdf"),
		Natura: db.NaturaAllegatoFile, Stato: db.StatoAllegatoAnalizzato, PathStaging: txtT(`C:\staging\x.pdf`), Sha256: txtT(sha)}}
	riga := func(a AllegatoUI) string {
		t.Helper()
		var buf bytes.Buffer
		if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "allegato_riga", rigaAllegato{A: a}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if html := riga(a); !strings.Contains(html, `href="/allegato/`+a.AllegatoID.String()+`/anteprima?v=`+sha+`"`) {
		t.Errorf("il link dell'Inbox non porta l'impronta:\n%s", html)
	}
	senza := a
	senza.Sha256 = txtT("")
	if html := riga(senza); !strings.Contains(html, `href="/allegato/`+a.AllegatoID.String()+`/anteprima"`) {
		t.Errorf("senza hash il link resta quello di prima:\n%s", html)
	}

	// il Fascicolo: l'iframe del pannello
	fz := fascicoloSintetico()
	fz.d.filtraFile()
	fz.d.Stato.File = fz.pdf
	fz.d.File[0].A.Sha256 = txtT(sha)
	fz.d.Anteprima = &anteprimaDati{F: fz.d.File[0]}
	html := rendiFascicolo(t, "fasc_corpo", fz.d)
	haTesto(t, "iframe del Fascicolo", html, `<iframe class="ant-pdf" id="anteprima-pdf" src="/allegato/`+fz.pdf.String()+`/anteprima?v=`+sha+`"`)

	// la pagina Richieste: l'iframe della scheda del prodotto, con l'impronta prima del frammento del viewer
	thread, comp := uuid.New(), uuid.New()
	pr := db.ListProdottiPanoramicaRow{ThreadID: thread, Codice: "7120001", ComponenteID: uuid.NullUUID{UUID: comp, Valid: true}}
	conSha := candidato(thread, comp, "7120001", "documento", time.Now())
	conSha.Sha256 = txtT(sha)
	schede := raggruppaProdotti([]db.ListProdottiPanoramicaRow{pr}, []db.ListAnteprimePanoramicaRow{conSha})[thread]
	if len(schede) != 1 || schede[0].Anteprima != "/allegato/"+conSha.AllegatoID.String()+"/anteprima?v="+sha+"#toolbar=0&navpanes=0&view=Fit" {
		t.Errorf("la scheda della pagina Richieste: %+v", schede)
	}
}

// cache C1, prova 4 — la vista Documenti del Fascicolo: l'impronta sta nei dati che il viewer legge (l'indice di
// ogni file, la revisione precedente) e nella miniatura del filmstrip (data-v), e fascicolo.mjs costruisce ogni
// indirizzo del PDF con quella. Nessun indirizzo «nudo» e' rimasto nel modulo.
func TestCacheC1LaVistaDocumentiPassaLImpronta(t *testing.T) {
	s := fascicoloSintetico()
	d := s.d
	shaVecchio, shaNuovo := strings.Repeat("b", 64), strings.Repeat("d", 64)
	nuovo := db.Documento{DocumentoID: uuid.New(), ComponenteID: uuid.NullUUID{UUID: s.particolare.ComponenteID, Valid: true}, Tipo: db.TipoDocumentoDisegno2d,
		NomeFile: "7120001 rev B.pdf", Estensione: "pdf", Sha256: shaNuovo, StatoNas: db.StatoNasInCoda}
	vecchio := db.Documento{DocumentoID: uuid.New(), ComponenteID: nuovo.ComponenteID, Tipo: db.TipoDocumentoDisegno2d, NomeFile: "7120001 rev A.pdf",
		Estensione: "pdf", Sha256: shaVecchio, StatoNas: db.StatoNasScritto, SostituitoDa: uuid.NullUUID{UUID: nuovo.DocumentoID, Valid: true}}
	aNuovo, aVecchio := uuid.New(), uuid.New()
	d.File = append(d.File,
		rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: aNuovo, NomeFile: nuovo.NomeFile, Estensione: txtT("pdf"), Sha256: txtT(shaNuovo), PathStaging: txtT(`C:\s\n.pdf`)}, Doc: &nuovo, Tipo: "disegno_2d"},
		rigaFile{A: db.ListAllegatiFascicoloRow{AllegatoID: aVecchio, NomeFile: vecchio.NomeFile, Estensione: txtT("pdf"), Sha256: txtT(shaVecchio)}, Doc: &vecchio, Tipo: "disegno_2d"})
	d.DocumentiDi[s.particolare.ComponenteID] = []db.Documento{vecchio, nuovo}
	d.AllegatoDi = map[uuid.UUID]uuid.UUID{vecchio.DocumentoID: aVecchio, nuovo.DocumentoID: aNuovo}
	note := []db.ListAnnotazioniThreadRow{{AnnotazioneID: uuid.New(), Sha256: txtT(shaVecchio), Pagina: 1, X: 0.5, Y: 0.5, Testo: "sulla rev A", CreataIl: time.Now()}}
	d.Stato = statoFascicolo{Nodo: s.particolare.ComponenteID}
	v := costruisciDocumenti(d, note, uuid.Nil)

	trovato := false
	for _, e := range v.Indice.Elementi {
		for _, f := range e.File {
			if f.A == aNuovo.String() {
				trovato = true
				if f.V != shaNuovo {
					t.Errorf("il file nell'indice del viewer ha l'impronta %q, attesa %q", f.V, shaNuovo)
				}
			}
		}
	}
	if !trovato {
		t.Fatal("il file del particolare non e' nell'indice")
	}
	if p := v.Indice.Prec[aNuovo.String()]; p.Allegato != aVecchio.String() || p.V != shaVecchio {
		t.Errorf("la revisione precedente nell'indice: %+v, attesa l'impronta %q", p, shaVecchio)
	}
	var tile *tileDoc
	for i := range v.Film {
		if v.Film[i].Anteprima == aNuovo {
			tile = &v.Film[i]
		}
	}
	if tile == nil || tile.Impronta != shaNuovo {
		t.Fatalf("la miniatura del particolare: %+v", tile)
	}
	d.Documenti = v
	html := rendiParte(t, "fasc_docv", d)
	haTesto(t, "filmstrip", html, `data-anteprima="`+aNuovo.String()+`" data-v="`+shaNuovo+`"`)

	b, err := fs.ReadFile(risorse.FS, "web/static/fascicolo.mjs")
	if err != nil {
		t.Fatal(err)
	}
	mjs := string(b)
	if strings.Contains(mjs, `"/anteprima")`) || strings.Contains(mjs, `"/anteprima",`) {
		t.Error("fascicolo.mjs costruisce ancora un indirizzo dell'anteprima senza l'impronta")
	}
	for _, atteso := range []string{`"?v=" + encodeURIComponent(v)`, "apriFile(a, improntaFile)", `const improntaFile = S.prec ? (pr && pr.v) || "" : f.v || ""`,
		"src: indirizzoPdf(a, v)", "opzioniPdf(indirizzoPdf(a, v))", "opzioniPdf(indirizzoPdf(a, t.dataset.v)"} {
		if !strings.Contains(mjs, atteso) {
			t.Errorf("fascicolo.mjs: manca %q", atteso)
		}
	}
}
