package web

// distinta_unica_pagina.go: la pagina della Distinta unica di V1, il guscio della pagina nuova (giro 5, A1d, V1;
// direttiva 23 §6; emendamento di V1 = EM §9, §10.9, §12; matrice dei gesti = MAT TS-01, §1.3). È il solo file Go della
// corsia interfaccia e ponte: le rotte dei dati (R1-R7, R11, E1′, S1-S7) e il modello /motore-a/dati le scrivono le
// corsie persistenza, file e motore.
//
// Che cosa fa il gestore. Serve il guscio HTML: layout del Cockpit, il contenitore della pagina con la RFQ e l'utente
// negli attributi data-du-*, il foglio di stile e il modulo della pagina (web/static/distinta_unica.mjs), che chiede
// tutto il resto al server SOLO tramite il ponte di V1 (web/static/distinta_unica_ponte.mjs). Nessun calcolo del motore:
// il modello lo legge la pagina con /motore-a/dati, che dice l'errore di calcolo con un 200 (D-18 = A [U], DEC:102).
//
// Le regole (EM §12, BL V1-Q07 A; MAT §1.2):
//   - la catena è quella delle rotte di motore-a (motore_a.go): conMotoreA (flag [motore_a] spento = il 404 del router,
//     prima della sessione) → conNoStore → soloAdmin → gestore, con la riga gemella senza metodo (405 a flag acceso,
//     404 a flag spento);
//   - all'apertura nessuna scrittura (D-18; D23:337; EM §6.10 n. 4): la sola lettura è l'esistenza della RFQ con la sua
//     testata (GetThread, una SELECT con il pool, senza transazione); la riga della sessione la tocca autenticato, come
//     su ogni rotta (eccezione RO-DOMINIO già dichiarata);
//   - id non valido = 400; RFQ inesistente = 404 (anche con HEAD); un errore della lettura non è un 404: la pagina 200
//     con l'errore scritto e il dettaglio solo nel log (D-18 = A: «mai una pagina vuota»);
//   - nessun percorso del disco, nessun dato del motore nel guscio: solo UUID e testi della testata.
//
// Dove si monta [T]. La matrice (MAT TS-01) mette il guscio su GET /thread/{id}/motore-a, dove oggi sta la pagina di
// A1d-P4 (motore_a_pagina.go, fuori da questa corsia). Finché l'integratore non la sostituisce, la pagina di V1 sta su
// percorsoDistintaUnica; il modulo prende la RFQ dall'attributo data-du-thread del guscio, non dall'indirizzo, quindi
// lo stesso guscio vale su tutti e due i percorsi. Il montaggio (s.registraPaginaDistintaUnica(mux) in registraMotoreA)
// è una richiesta alla corsia che possiede motore_a.go, scritta nel resoconto della corsia.
//
// Il template si compone qui, non in Init (server.go, fuori dalla corsia): registraPaginaDistintaUnica lo compone una
// volta, al montaggio, con le funzioni di tutte le pagine e statico. Un template che non si compone va nel log e la
// pagina risponde 500 «pagina non disponibile»; la prova L1 TestDistintaUnicaTemplateSiCompone lo sorveglia.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// percorsoDistintaUnica: dove sta la pagina di V1 finché non sostituisce quella di A1d-P4 su /thread/{id}/motore-a
// (commento di testa).
const percorsoDistintaUnica = "/thread/{id}/distinta-unica"

// titoloDistintaUnica: il titolo della pagina («<titolo> · Cockpit RFQ»).
const titoloDistintaUnica = "Distinta unica"

// I testi delle risposte del guscio che non sono la pagina.
const (
	testoDistintaUnicaID      = "id non valido"
	testoDistintaUnicaRFQ     = "RFQ non trovata"
	testoDistintaUnicaRotta   = "pagina non disponibile"
	testoDistintaUnicaLettura = "La testata della RFQ non si legge: il dettaglio è nel log del server. La pagina prova lo stesso a leggere il modello."
)

// errRFQDistintaUnica: la RFQ chiesta non c'è (il solo errore della lettura che diventa un 404).
var errRFQDistintaUnica = errors.New("RFQ inesistente")

// testataDistintaUnica: quello che il guscio scrive della RFQ, letto dal database. Mai un dato del motore.
type testataDistintaUnica struct {
	Oggetto     string
	Riferimento string
	Stato       string
}

// lettoreDistintaUnica: la sola lettura del guscio. errRFQDistintaUnica per una RFQ che non c'è; ogni altro errore è
// un problema della lettura (la pagina con l'errore, D-18).
type lettoreDistintaUnica func(ctx context.Context, thread uuid.UUID) (testataDistintaUnica, error)

// paginaDistintaUnica: il gestore del guscio, con la lettura e il template passati da fuori (le prove li danno finti).
type paginaDistintaUnica struct {
	s       *Server
	leggi   lettoreDistintaUnica
	tmpl    *template.Template
	errTmpl error
}

// datiDistintaUnica: i dati del template (.Dati). ThreadID e UtenteID vanno negli attributi del contenitore; la pagina
// li usa per gli indirizzi del ponte e per le chiavi della memoria di sessione della scheda (EM §10.9, ultimo paragrafo).
type datiDistintaUnica struct {
	ThreadID uuid.UUID
	UtenteID string
	Testata  testataDistintaUnica
	Errore   string
}

// registraPaginaDistintaUnica monta il guscio e la sua riga gemella. Stessa catena delle rotte di motore-a: prima il
// flag, poi il no-store, poi soloAdmin. La chiama chi possiede registraMotoreA (richiesta della corsia, commento di testa).
func (s *Server) registraPaginaDistintaUnica(mux *http.ServeMux) {
	p := s.nuovaPaginaDistintaUnica(s.testataDistintaUnicaDalDB)
	mux.HandleFunc("GET "+percorsoDistintaUnica, s.conMotoreA(conNoStore(s.soloAdmin(p.servi))))
	mux.HandleFunc(percorsoDistintaUnica, s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA)))
}

// nuovaPaginaDistintaUnica compone il template una volta e prepara il gestore con la lettura data.
func (s *Server) nuovaPaginaDistintaUnica(leggi lettoreDistintaUnica) *paginaDistintaUnica {
	p := &paginaDistintaUnica{s: s, leggi: leggi}
	p.tmpl, p.errTmpl = s.templateDistintaUnica()
	if p.errTmpl != nil && s.Log != nil {
		s.Log.Error("distinta unica: il template della pagina non si compone", "err", p.errTmpl)
	}
	return p
}

// templateDistintaUnica: layout.html + frammenti.html + distinta_unica.html, con le funzioni di tutte le pagine e statico
// (lo stesso schema di Init, server.go).
func (s *Server) templateDistintaUnica() (*template.Template, error) {
	if s.Templ == nil {
		return nil, errors.New("i template non ci sono (Server.Templ nil)")
	}
	t, err := template.New("layout").Funcs(funzioni).Funcs(template.FuncMap{"statico": s.statico}).
		ParseFS(s.Templ, "layout.html", "frammenti.html", "distinta_unica.html")
	if err != nil {
		return nil, fmt.Errorf("template distinta_unica.html: %w", err)
	}
	return t, nil
}

// testataDistintaUnicaDalDB: GetThread con il pool, nessuna transazione, nessuna scrittura.
func (s *Server) testataDistintaUnicaDalDB(ctx context.Context, thread uuid.UUID) (testataDistintaUnica, error) {
	if s.Pool == nil {
		return testataDistintaUnica{}, errors.New("nessun database")
	}
	t, err := db.New(s.Pool).GetThread(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return testataDistintaUnica{}, errRFQDistintaUnica
	}
	if err != nil {
		return testataDistintaUnica{}, err
	}
	return testataDistintaUnica{Oggetto: t.Oggetto.String, Riferimento: t.RiferimentoCliente.String, Stato: string(t.Stato)}, nil
}

// servi: GET (e HEAD) di percorsoDistintaUnica.
//   - id non valido: 400;
//   - RFQ inesistente: 404, anche con HEAD, senza corpo con HEAD;
//   - HEAD: le intestazioni della GET, senza corpo e senza comporre la pagina;
//   - GET: la pagina intera; un errore della lettura scrive l'errore nella pagina (200), mai un 500 per una lettura.
func (p *paginaDistintaUnica) servi(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, testoDistintaUnicaID, http.StatusBadRequest)
		return
	}
	testata, err := p.leggi(r.Context(), thread)
	if errors.Is(err, errRFQDistintaUnica) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.Error(w, testoDistintaUnicaRFQ, http.StatusNotFound)
		return
	}
	d := datiDistintaUnica{ThreadID: thread, Testata: testata}
	if err != nil {
		if p.s.Log != nil {
			p.s.Log.Warn("distinta unica: la testata della RFQ non si legge", "thread", thread, "err", err)
		}
		d.Errore = testoDistintaUnicaLettura
	}
	if r.Method == http.MethodHead {
		intestazioniDistintaUnica(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	if p.tmpl == nil {
		http.Error(w, testoDistintaUnicaRotta, http.StatusInternalServerError)
		return
	}
	u := utenteDa(r.Context())
	if u != nil {
		d.UtenteID = u.UtenteID.String()
	}
	v := vista{Utente: u, Titolo: titoloDistintaUnica, Dati: d}
	v.Admin = almeno(u, db.RuoloUtenteAdmin)
	if u != nil && p.s.Pool != nil {
		v.Stato = p.s.stato(r.Context(), sessioneDa(r.Context()))
	}
	var buf bytes.Buffer
	if err := p.tmpl.ExecuteTemplate(&buf, "layout", v); err != nil {
		if p.s.Log != nil {
			p.s.Log.Error("template", "pagina", "distinta_unica.html", "err", err)
		}
		http.Error(w, testoDistintaUnicaRotta, http.StatusInternalServerError)
		return
	}
	intestazioniDistintaUnica(w)
	_, _ = w.Write(buf.Bytes())
}

// intestazioniDistintaUnica: le intestazioni di una risposta 200 del guscio.
func intestazioniDistintaUnica(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
