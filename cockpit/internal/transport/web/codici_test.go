package web

// L1 — B8.6: il pannello dei codici della RFQ con dati sintetici, una riga per situazione. Dallo
// Smistamento (F2, R1) il pannello si legge e basta: ogni codice dice la sua situazione (la proposta STEP
// aperta, il componente con i suoi documenti, l'archiviato, il codice della richiesta, il codice nuovo) e
// nessuno porta un gesto. Con la BOM congelata le righe restano, e il pannello lo dice.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// rigaDelCodice e' l'HTML della riga di un codice nel pannello, fino alla riga successiva.
func rigaDelCodice(t *testing.T, html, chiave string) string {
	t.Helper()
	i := strings.Index(html, `id="codice-`+chiave+`"`)
	if i < 0 {
		t.Fatalf("la riga del codice %s non c'e'", chiave)
	}
	resto := html[i:]
	fine := len(resto)
	for _, dopo := range []string{`<div class="codice-riga"`, `<details class="codici"`, `<h3`} {
		if j := strings.Index(resto[1:], dopo); j >= 0 && j+1 < fine {
			fine = j + 1
		}
	}
	return resto[:fine]
}

func pannelloSintetico(bloccata int32) (*threadDati, map[string]uuid.UUID) {
	_, _, thd := datiSintetici()
	msg := uuid.New()
	riga := func(codice, rev, sorgente, origine, dove, tipo string, punti int32) db.ListCodiciCandidatiThreadRow {
		return db.ListCodiciCandidatiThreadRow{Codice: codice, Rev: rev, Sorgente: sorgente, Origine: origine, Evidenza: dove, TipoFile: tipo,
			Punteggio: punti, MessaggioID: msg, Famiglia: map[bool]string{true: "disegni 777"}[origine == "famiglia"]}
	}
	comp := db.Componente{ComponenteID: uuid.New(), Codice: "77740000", Tipo: db.TipoComponenteSottoassieme, Rev: txtT("B")}
	ieri := time.Now().Add(-24 * time.Hour)
	arch := db.Componente{ComponenteID: uuid.New(), Codice: "77731111", Tipo: db.TipoComponenteSciolto, ArchiviatoIl: &ieri,
		MotivoArchiviazione: txtT("tolto dal cliente")}
	prop := db.ListProposteNodoAperteRow{PropostaID: uuid.New(), AllegatoID: uuid.New(), Chiave: "#2", Codice: "77720517", NomeGrezzo: "77720517",
		NomeFile: "assieme.stp", TipoProposto: db.NullTipoComponente{TipoComponente: db.TipoComponenteSottoassieme, Valid: true}}
	righe := []db.ListCodiciCandidatiThreadRow{
		riga("77720517", "", "step", "famiglia", "77720517 — assieme.stp", "", 80),
		riga("77740000", "", "messaggio", "famiglia", "corpo", "", 80),
		riga("77731111", "", "messaggio", "famiglia", "corpo", "", 80),
		riga("77750000", "", "messaggio", "famiglia", "oggetto", "", 80),
		riga("77760000", "A", "messaggio", "famiglia", "oggetto", "", 80),
		riga("77760000", "B", "proposta_documento", "cartiglio", "77760000.pdf", "disegno_2d", 95),
		riga("20260908", "", "messaggio", "generico", "corpo", "", 30),
	}
	thd.Codici = fascicolo.Unisci(righe, fascicolo.ContestoCodici{HaFamiglie: true, Bloccata: bloccata,
		Componenti: []db.Componente{comp, arch}, Proposte: []db.ListProposteNodoAperteRow{prop},
		Identificativi: []db.IdentificativoThread{{Codice: "77750000"}}})
	thd.DocumentiDi = map[uuid.UUID][]db.Documento{comp.ComponenteID: {{NomeFile: "77740000 foglio 1.pdf", Tipo: db.TipoDocumentoDisegno2d, StatoNas: db.StatoNasScritto}}}
	return thd, map[string]uuid.UUID{"proposta": prop.PropostaID, "archiviato": arch.ComponenteID}
}

func rendiPannello(t *testing.T, thd *threadDati) string {
	t.Helper()
	var buf bytes.Buffer
	if err := serverTest(t).pagine["thread.html"].ExecuteTemplate(&buf, "thread_corpo", vista{Dati: thd, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Riscritta per lo Smistamento (R1, fase F2): prima era TestIlPannelloDeiCodiciPortaOgniCodiceAlSuoGesto e
// fissava il gesto di ogni riga (Accetta la proposta con il tipo preselezionato, Ripristina, «+ Prodotto» per
// il codice della richiesta, i tre «+» con la scelta della revisione per il codice nuovo, «+ Prodotto» anche
// per il generico). Adesso fissa, riga per riga, che la situazione si legge e che nessun gesto c'e'; e che il
// pannello intero non ha moduli, bottoni ne' POST.
func TestIlPannelloDeiCodiciSiLegge(t *testing.T) {
	thd, id := pannelloSintetico(0)
	html := rendiPannello(t, thd)
	for _, atteso := range []string{"Codici della richiesta", "Codici prodotto candidati</b> (5)", "Altri riferimenti trovati</b> (1)", "si leggono e basta"} {
		if !strings.Contains(html, atteso) {
			t.Errorf("manca %q", atteso)
		}
	}
	casi := []struct {
		chiave    string
		ci, manca []string
	}{
		{"77720517", []string{"proposta aperta dallo STEP <b>assieme.stp</b>", "si decide nel Fascicolo"},
			[]string{"/fascicolo/nodo/" + id["proposta"].String(), "Accetta la proposta", "/scarta", `name="tipo"`, "+ Prodotto", "/codice/aggiungi"}},
		{"77740000", []string{"✓ nel Fascicolo", "come assieme, rev B", "77740000 foglio 1.pdf"}, []string{"hx-post", "+ Prodotto"}},
		{"77731111", []string{"archiviato", "tolto dal cliente", "Archiviati della Struttura BOM"},
			[]string{"/fascicolo/componente/" + id["archiviato"].String() + "/ripristina", ">Ripristina<", "+ Prodotto"}},
		{"77750000", []string{"codice della richiesta senza componente", fraseRichiestaSenzaRevisione}, []string{"+ Prodotto", `"tipo":"finito"`, "+ Assieme", fraseRichiestaConRevisione}},
		{"77760000", []string{"revisioni discordanti", ">rev A<", ">rev B<", "nessun componente con questo codice"},
			[]string{`name="rev"`, "+ Prodotto", "+ Assieme", "+ Particolare", `"tipo":"sottoassieme"`, `name="codice"`, "/codice/aggiungi"}},
		{"20260908", []string{"solo dall&#39;estrattore generico", "nessun componente con questo codice"}, []string{"+ Prodotto", "revisioni discordanti"}},
	}
	for _, c := range casi {
		r := rigaDelCodice(t, html, c.chiave)
		for _, s := range c.ci {
			if !strings.Contains(r, s) {
				t.Errorf("%s: manca %q in\n%s", c.chiave, s, r)
			}
		}
		for _, s := range c.manca {
			if strings.Contains(r, s) {
				t.Errorf("%s: non doveva esserci %q in\n%s", c.chiave, s, r)
			}
		}
	}
	pannello := sezioneCodici(t, html)
	for _, vietato := range []string{"<form", "<button", "hx-post", "<select", "<input"} {
		if strings.Contains(pannello, vietato) {
			t.Errorf("il pannello dei codici ha ancora %q", vietato)
		}
	}
}

// sezioneCodici e' il pannello dei codici nella pagina della RFQ: dal suo titolo al titolo che segue.
func sezioneCodici(t *testing.T, html string) string {
	t.Helper()
	i := strings.Index(html, `<h3 id="codici">`)
	if i < 0 {
		t.Fatal("la pagina della RFQ non ha il pannello dei codici")
	}
	resto := html[i+1:]
	if j := strings.Index(resto, "<h3"); j >= 0 {
		resto = resto[:j]
	}
	return resto
}

func TestConLaBomCongelataIlPannelloNonOffreGestiCheLaCambiano(t *testing.T) {
	thd, _ := pannelloSintetico(1)
	html := rendiPannello(t, thd)
	if !strings.Contains(html, "La BOM è congelata nella V1") {
		t.Error("il pannello deve dire che la BOM e' congelata")
	}
	for _, vietato := range []string{"+ Prodotto", "+ Assieme", "+ Particolare", ">Ripristina<", "Accetta la proposta", "/codice/aggiungi", "/ripristina"} {
		if strings.Contains(html, vietato) {
			t.Errorf("con la BOM congelata il pannello offre ancora %q", vietato)
		}
	}
	for _, chiave := range []string{"77720517", "77740000", "77731111", "77750000", "77760000", "20260908"} {
		rigaDelCodice(t, html, chiave) // le righe restano: i codici si leggono
	}
}

// Le due frasi della riga di un codice della richiesta senza componente (Smistamento F3, rilievo della
// verifica di F2): la strada della revisione solo per il codice che il congelamento ha fermato.
const (
	fraseRichiestaConRevisione   = "il prodotto entra aprendo una revisione della BOM"
	fraseRichiestaSenzaRevisione = "il prodotto non rinasce da solo, né aprendo le pagine né aprendo una revisione: rientra solo con una decisione del triage"
)

// Smistamento F3 (rilievo della verifica di F2; R1, E11): con la BOM congelata la riga promette la revisione
// solo al codice della richiesta che il congelamento ha fermato, confermato da una persona dopo l'ultimo
// congelamento (la regola di AssicuraProdottiDellaRevisione). Il codice confermato prima, il cui prodotto era
// stato tolto, e quello che nessuno ha confermato non la promettono: aprirla non li farebbe nascere. Con la
// working libera non la promette nessuno.
func TestIlCodiceDellaRichiestaPromettelaRevisioneSoloSeIlCongelamentoLoHaFermato(t *testing.T) {
	congelata := time.Now().Add(-time.Hour)
	chi := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	msg := uuid.New()
	riga := func(codice string) db.ListCodiciCandidatiThreadRow {
		return db.ListCodiciCandidatiThreadRow{Codice: codice, Sorgente: "messaggio", Origine: "famiglia", Evidenza: "oggetto", Punteggio: 80,
			MessaggioID: msg, Famiglia: "disegni 777"}
	}
	righe := []db.ListCodiciCandidatiThreadRow{riga("77750000"), riga("77750001"), riga("77750002")}
	ids := []db.IdentificativoThread{
		{Codice: "77750000", ConfermatoDa: chi, CreatoIl: congelata.Add(time.Minute)},  // fermato dal congelamento
		{Codice: "77750001", ConfermatoDa: chi, CreatoIl: congelata.Add(-time.Minute)}, // c'era gia': il prodotto era stato tolto
		{Codice: "77750002", CreatoIl: congelata.Add(time.Minute)},                     // nessuno l'ha confermato
	}
	for _, bloccata := range []int32{1, 0} {
		thd, _ := pannelloSintetico(bloccata)
		cx := fascicolo.ContestoCodici{HaFamiglie: true, Bloccata: bloccata, Identificativi: ids}
		if bloccata != 0 {
			cx.CongelataIl = &congelata
		}
		thd.Codici = fascicolo.Unisci(righe, cx)
		html := rendiPannello(t, thd)
		for _, codice := range []string{"77750000", "77750001", "77750002"} {
			r := rigaDelCodice(t, html, codice)
			con := bloccata != 0 && codice == "77750000"
			if !strings.Contains(r, "codice della richiesta senza componente") {
				t.Errorf("V%d %s: non dice che e' un codice della richiesta senza componente:\n%s", bloccata, codice, r)
			}
			if con != strings.Contains(r, fraseRichiestaConRevisione) || con == strings.Contains(r, fraseRichiestaSenzaRevisione) {
				t.Errorf("V%d %s: la strada della revisione promessa = %v, attesa %v:\n%s", bloccata, codice,
					strings.Contains(r, fraseRichiestaConRevisione), con, r)
			}
			if con && !strings.Contains(r, "confermato dopo il congelamento della V1") {
				t.Errorf("%s: non dice perche' entra con la revisione:\n%s", codice, r)
			}
		}
	}
}
