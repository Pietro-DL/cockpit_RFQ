package web

// Anagrafica › Forme viste (Smistamento, giro 4, fase 4.17a): il censimento delle forme in sola lettura, per
// cliente, e il suo export Markdown.
//
// Tutte e due sono GET dell'amministratore e non scrivono niente (prova 98): il censimento legge in una
// transazione READ ONLY (registro/censimento.Leggi). L'export e' un download dalla stessa pagina, non un file
// scritto dal server: i dati reali dei clienti vanno in `docs/`, e ce li mette chi scarica, sul suo PC. Un
// file scritto dal server finirebbe in una cartella del server, fuori dal repository e fuori dalla vista di
// chi l'ha chiesto, e renderebbe la GET una scrittura.

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"promatec/cockpit/internal/core/registro/censimento"
)

// formeDati e' la pagina: il censimento, l'ora della lettura, le fonti nell'ordine delle colonne e una
// scheda per cliente con i suoi elenchi gia' pronti per i frammenti (html/template non costruisce strutture).
type formeDati struct {
	C      censimento.Censimento
	Letto  time.Time
	Fonti  []string
	Schede []schedaForme
}

type schedaForme struct {
	S                                                 censimento.Scheda
	Fonti                                             []string
	RestoForme, RestoFirme, RestoParole               restoForme
	Code, Teste                                       aggiunteForme
	FormeRif, Letture, Oggetti, Riferimenti, Mittenti vociForme
	Scarti, NonViste                                  minuteriaForme
}

// restoForme e' la riga «altre N voci (M occorrenze)» di un elenco tagliato.
type restoForme struct {
	R       censimento.Resto
	Voci, N string
}

type aggiunteForme struct {
	V       []censimento.Aggiunta
	Colonna string
	Resto   restoForme
}

type vociForme struct {
	V               []censimento.Voce
	Colonna, Quante string
	Resto           restoForme
}

type minuteriaForme struct {
	V      []censimento.RigaMinuteria
	Altri  int
	Titolo string
}

// vistaForme prepara la pagina dal censimento.
func vistaForme(c censimento.Censimento, letto time.Time) formeDati {
	d := formeDati{C: c, Letto: letto, Fonti: censimento.Fonti}
	for _, s := range c.Schede {
		d.Schede = append(d.Schede, schedaForme{
			S: s, Fonti: censimento.Fonti,
			RestoForme:  restoForme{s.AltreForme, "forme", "stringhe"},
			RestoFirme:  restoForme{s.Firme.AltreForme, "forme", "pezzi"},
			RestoParole: restoForme{s.Firme.AltreParole, "parole", "pezzi"},
			Code:        aggiunteForme{s.Code, "Coda", restoForme{s.AltreCode, "code", "stringhe"}},
			Teste:       aggiunteForme{s.Teste, "Testa", restoForme{s.AltreTeste, "teste", "stringhe"}},
			FormeRif:    vociForme{s.Mail.FormeRiferimento, "Forma del riferimento", "Mail", restoForme{}},
			Letture:     vociForme{s.Nome.Letture, "Lettura del nome", "File", restoForme{s.Nome.AltreLetture, "letture", "file"}},
			Oggetti:     vociForme{s.Oggetti, "Forma dell'oggetto", "Mail", restoForme{s.AltriOggetti, "forme", "mail"}},
			Riferimenti: vociForme{s.Riferimenti, "Parola e forma", "Mail", restoForme{s.AltriRiferimenti, "voci", "mail"}},
			Mittenti:    vociForme{s.Mittenti, "Indirizzo", "Mail", restoForme{s.AltriMittenti, "indirizzi", "mail"}},
			Scarti:      minuteriaForme{s.Minuteria.Scarti, s.Minuteria.AltriScarti, "Proposte scartate dall'ingegnere"},
			NonViste:    minuteriaForme{s.Minuteria.NonViste, s.Minuteria.AltriNonVisti, "Particolari commerciali decisi senza una proposta"},
		})
	}
	return d
}

// adminForme e' la pagina «Forme viste».
func (s *Server) adminForme(w http.ResponseWriter, r *http.Request) {
	c, err := censimento.Raccogli(r.Context(), s.Pool)
	if err != nil {
		s.Log.Error("forme viste", "err", err)
		http.Error(w, "censimento: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// il titolo del layout e' quello dell'Anagrafica, come per Fornitori e Importa: e' la voce della rail che
	// si accende (layout.html, eq $.Titolo .Titolo); «Forme viste» e' il titolo della pagina (h1) e della sua scheda
	s.rendi(w, r, "forme.html", "", "Anagrafica", vistaForme(c, time.Now()))
}

// adminFormeMarkdown e' l'export della stessa pagina, da scaricare: text/markdown come allegato, mai in cache
// (sono dati reali dei clienti), con la data e l'ora nel nome del file, cosi' che due export si salvino uno
// accanto all'altro e si confrontino.
func (s *Server) adminFormeMarkdown(w http.ResponseWriter, r *http.Request) {
	c, err := censimento.Raccogli(r.Context(), s.Pool)
	if err != nil {
		s.Log.Error("forme viste, export", "err", err)
		http.Error(w, "censimento: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ora := time.Now()
	h := w.Header()
	h.Set("Content-Type", "text/markdown; charset=utf-8")
	h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "forme_viste_"+ora.Format("2006-01-02_1504")+".md"))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, censimento.Markdown(c, ora))
}
