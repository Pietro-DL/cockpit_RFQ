//go:build integrazione

// L4 — giro 4, fase 4.1b (domanda 9b = A; A5.3.6 e U3: una conferma scritta prima di un gesto che non torna
// indietro): «Conferma Fascicolo» sulla porta HTTP vera. Il bottone apre il riepilogo con una GET che non scrive;
// la POST senza la firma del riepilogo, senza la conferma esplicita o con la firma di un riepilogo che nel frattempo
// e' cambiato (anche solo un percorso sul NAS) non scrive niente e ridisegna il riepilogo, con htmx e senza; con la
// firma giusta e «Conferma e copia sul NAS» fa quello che faceva prima: i documenti, nei percorsi che il riepilogo
// mostrava, e le copie sul NAS in coda. Il «✓ Conferma» di un file solo resta com'era.

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// scenaRiepilogo: la RFQ di ACME in fattibilita' con il prodotto 7120001, i due fogli del suo disegno arrivati
// insieme e un capitolato deciso dall'operatore. Tre file pronti, nessuno ancora nel fascicolo.
type scenaRiepilogo struct {
	*rfqFascicolo
	prodotto                     uuid.UUID
	foglio1, foglio2, capitolato uuid.UUID // proposte
}

// I percorsi che la conferma darebbe ai tre file, nella cartella della RFQ.
const (
	percorsoFoglio1    = `ELENCO DISEGNI\7120001\7120001_REV_ND.pdf`
	percorsoFoglio2    = `ELENCO DISEGNI\7120001\7120001_REV_ND_2.pdf`
	percorsoCapitolato = `CAPITOLATI\Capitolato ACME.pdf`
)

func (b *bancoWeb) scenaRiepilogo(chiave string) *scenaRiepilogo {
	b.t.Helper()
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	s := &scenaRiepilogo{rfqFascicolo: r}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("7120001", "finito")
	s.foglio1, _ = r.propostaDa("7120001.pdf", "7120001")
	s.foglio2, _ = r.propostaDa("7120001 foglio 2.pdf", "7120001")
	s.capitolato, _ = r.propostaDa("Capitolato ACME.pdf", "")
	r.esegui(`UPDATE documento_proposta SET tipo_proposto = 'capitolato', fonte = 'operatore', confidenza = 100 WHERE proposta_id = $1`, s.capitolato)
	// il flusso ha gia' girato sulla RFQ, come dopo il primo gesto di una RFQ vera: le destinazioni dei file sono
	// scritte, e il giro che segue ogni gesto (anche uno rifiutato) non riscrive niente. Senza, il primo giro dopo
	// una POST rifiutata le scriverebbe, e sembrerebbe la POST
	b.ws.rismista(b.ctx, r.thread)
	return s
}

// riepilogo apre il riepilogo come lo apre «Conferma Fascicolo» (la GET del cassetto, con htmx) e ne restituisce la
// firma e l'HTML.
func (s *scenaRiepilogo) riepilogo(w *browser) (string, string) {
	s.b.t.Helper()
	resp, html := w.daFascicolo(http.MethodGet, s.base()+"/parti?cassetto=piano", nil, s.thread, "")
	if resp.StatusCode != 200 {
		s.b.t.Fatalf("il riepilogo: %d", resp.StatusCode)
	}
	return firmaDellaPagina(s.b.t, html), html
}

// conferma e' la POST della conferma dalla schermata con il riepilogo aperto: l'avviso, la risposta, l'HTML.
func (s *scenaRiepilogo) conferma(w *browser, form url.Values) (string, *http.Response, string) {
	s.b.t.Helper()
	resp, html := w.daFascicolo(http.MethodPost, s.base()+"/conferma", form, s.thread, "?cassetto=piano")
	return avvisoF(html), resp, html
}

// percorsi sono i documenti della RFQ, «nome_file@path_relativo» in ordine di nome.
func (s *scenaRiepilogo) percorsi() string {
	s.b.t.Helper()
	var v *string
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT string_agg(nome_file || '@' || path_relativo, ' | ' ORDER BY nome_file COLLATE "C") FROM documento WHERE thread_id = $1`,
		s.thread).Scan(&v); err != nil {
		s.b.t.Fatal(err)
	}
	if v == nil {
		return ""
	}
	return *v
}

// Il riepilogo e' una GET che non scrive, e dice per ogni file il componente, il tipo e il percorso sul NAS. La POST
// senza la sua firma, senza la conferma esplicita, con la firma del piano (quella che il bottone di prima portava
// dentro) o con la firma di un riepilogo cambiato — qui cambia solo un percorso: un nome sul NAS nel frattempo
// occupato — si rifiuta, non scrive niente e ridisegna il riepilogo di adesso, con la sua firma nuova. Con la firma
// giusta e «Conferma e copia sul NAS» nascono i documenti, nei percorsi del riepilogo, con le copie in coda.
func TestConfermaFascicoloVuoleIlRiepilogoELaConferma(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaRiepilogo("RIE41B")
	w := operatore(b)

	_, pagina := w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(pagina, "<b>3</b> pronti") || !strings.Contains(pagina, `id="conferma-fascicolo" href="`+s.base()+`?cassetto=piano"`) {
		t.Fatalf("il piano in fondo: 3 pronti e «Conferma Fascicolo» che apre il riepilogo:\n%s", estratto(pagina, `id="piano"`))
	}
	piano, err := fascicolo.LeggiPianoFascicolo(b.ctx, b.q, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pagina, piano.Firma()) {
		t.Error("la pagina porta ancora la firma del piano")
	}

	// il riepilogo, con htmx e senza: non scrive
	prima := fotoDelDatabase(t, b)
	firma, rie := s.riepilogo(w)
	if resp, html := w.fai(http.MethodGet, s.base()+"?cassetto=piano", nil, false); resp.StatusCode != 200 || firmaDellaPagina(t, html) != firma {
		t.Errorf("il riepilogo senza htmx: %d, la stessa firma", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("il riepilogo ha scritto in: %s", d)
	}
	for _, c := range []string{`ACME\WIP\2026 09 23 RIE41B`, `di <span class="mono">7120001</span>`, "capitolato della RFQ, senza componente",
		"sul NAS: <span class=\"mono\">" + percorsoFoglio1 + "</span>", "sul NAS: <span class=\"mono\">" + percorsoFoglio2 + "</span>",
		"sul NAS: <span class=\"mono\">" + percorsoCapitolato + "</span>", `name="conferma" value="1"`, `name="selezione" value="1"`,
		`method="post" action="` + s.base() + `/conferma"`} {
		if !strings.Contains(leggibile(rie), leggibile(c)) {
			t.Errorf("il riepilogo: manca %q", c)
		}
	}
	if firma == piano.Firma() {
		t.Error("la firma del riepilogo e' quella del piano")
	}

	// le POST che non scrivono: ognuna ridisegna il riepilogo, nell'indirizzo e nel cassetto
	for _, c := range []struct {
		nome  string
		form  url.Values
		frase string
	}{
		{"senza niente (il bottone di prima senza la firma)", url.Values{}, "si conferma dal riepilogo"},
		{"la firma del piano (il bottone di prima)", url.Values{"firma": {piano.Firma()}}, "il riepilogo è cambiato"},
		{"la firma del piano e la conferma", url.Values{"firma": {piano.Firma()}, "conferma": {"1"}}, "il riepilogo è cambiato"},
		{"la firma senza la conferma", url.Values{"firma": {firma}, "selezione": {"1"}, "voce": {s.foglio1.String(), s.foglio2.String(), s.capitolato.String()}}, "manca la conferma"},
		{"la conferma senza la firma", url.Values{"conferma": {"1"}, "voce": {s.foglio1.String(), s.foglio2.String()}}, "si conferma dal riepilogo"},
		{"una firma inventata", url.Values{"firma": {"deadbeef00000000"}, "conferma": {"1"}}, "il riepilogo è cambiato"},
	} {
		a, resp, html := s.conferma(w, c.form)
		if !strings.HasPrefix(a, "Niente è cambiato: ") || !strings.Contains(a, c.frase) {
			t.Errorf("%s: avviso %q, attesa la frase %q", c.nome, a, c.frase)
		}
		if resp.StatusCode != 200 || !strings.HasSuffix(resp.Header.Get("HX-Push-Url"), "?cassetto=piano") || firmaDellaPagina(t, html) != firma ||
			!strings.Contains(html, `<div id="cassetto" hx-swap-oob="innerHTML">`) {
			t.Errorf("%s: la risposta ridisegna il riepilogo (%d, %q)", c.nome, resp.StatusCode, resp.Header.Get("HX-Push-Url"))
		}
		if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
			t.Fatalf("%s: la POST rifiutata ha scritto in: %s", c.nome, d)
		}
	}

	// il riepilogo cambia sotto gli occhi: un nome sul NAS nel frattempo occupato (un orfano). Il piano e' lo stesso,
	// il riepilogo no: la firma vecchia non scrive, e la risposta mostra i percorsi nuovi con la firma nuova
	s.esegui(`INSERT INTO nas_orfano (thread_id, percorso, motivo) VALUES ($1, $2, 'non_nostro')`, s.thread, percorsoFoglio1)
	if p, err := fascicolo.LeggiPianoFascicolo(b.ctx, b.q, s.thread); err != nil || p.Firma() != piano.Firma() {
		t.Fatalf("l'orfano non cambia il piano: %v", err)
	}
	prima = fotoDelDatabase(t, b)
	a, _, html := s.conferma(w, url.Values{"firma": {firma}, "conferma": {"1"}, "selezione": {"1"},
		"voce": {s.foglio1.String(), s.foglio2.String(), s.capitolato.String()}})
	if !strings.HasPrefix(a, "Niente è cambiato: il riepilogo è cambiato mentre lo guardavi") {
		t.Errorf("la firma di un riepilogo cambiato: %q", a)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("la firma vecchia ha scritto in: %s", d)
	}
	nuova := firmaDellaPagina(t, html)
	if nuova == firma || !strings.Contains(html, `ELENCO DISEGNI\7120001\7120001_REV_ND_2.pdf`) || !strings.Contains(html, `ELENCO DISEGNI\7120001\7120001_REV_ND_3.pdf`) {
		t.Fatalf("il riepilogo ridisegnato ha i percorsi nuovi e una firma nuova (%s → %s)", firma, nuova)
	}

	// con la firma nuova e la conferma: i documenti, nei percorsi del riepilogo, e le copie in coda
	a, _, _ = s.conferma(w, url.Values{"firma": {nuova}, "conferma": {"1"}, "selezione": {"1"},
		"voce": {s.foglio1.String(), s.foglio2.String(), s.capitolato.String()}})
	if a != "Fascicolo confermato: 3 documenti (copie sul NAS in coda)." {
		t.Fatalf("la conferma: %q", a)
	}
	if got := s.percorsi(); got != `7120001 foglio 2.pdf@ELENCO DISEGNI\7120001\7120001_REV_ND_3.pdf | 7120001.pdf@ELENCO DISEGNI\7120001\7120001_REV_ND_2.pdf | Capitolato ACME.pdf@`+percorsoCapitolato {
		t.Errorf("i documenti non stanno dove diceva il riepilogo: %s", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 3 {
		t.Errorf("copie sul NAS in coda: %d, attese 3", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM documento WHERE thread_id = $1 AND componente_id = $2`, s.thread, s.prodotto); n != 2 {
		t.Errorf("i due fogli vanno al prodotto: %d", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM documento_proposta WHERE thread_id = $1 AND stato = 'confermata'`, s.thread); n != 3 {
		t.Errorf("proposte confermate: %d", n)
	}
	_, pagina = w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(pagina, "<b>0</b> pronti") || !strings.Contains(pagina, `id="conferma-fascicolo" disabled`) {
		t.Error("dopo la conferma non resta niente di pronto")
	}
}

// Senza htmx (il modulo del riepilogo mandato da un browser senza JavaScript) la conferma si comporta allo stesso
// modo: senza la firma o senza la conferma risponde 422 con la pagina intera, il riepilogo aperto e l'avviso, e non
// scrive; con la firma e la conferma risponde con la pagina intera e l'esito, e i documenti sono nati.
func TestConfermaFascicoloSenzaHtmx(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaRiepilogo("RIE41N")
	w := operatore(b)
	firma, _ := s.riepilogo(w)
	tutte := []string{s.foglio1.String(), s.foglio2.String(), s.capitolato.String()}

	prima := fotoDelDatabase(t, b)
	for _, form := range []url.Values{{}, {"firma": {firma}, "selezione": {"1"}, "voce": tutte}, {"conferma": {"1"}, "selezione": {"1"}, "voce": tutte}} {
		resp, html := w.fai(http.MethodPost, s.base()+"/conferma", form, false)
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(html, `<html lang="it">`) ||
			!strings.HasPrefix(avvisoF(html), "Niente è cambiato: ") || firmaDellaPagina(t, html) != firma ||
			!strings.Contains(html, "Conferma e copia sul NAS") {
			t.Errorf("senza htmx %v: %d, avviso %q: attesa la pagina intera con il riepilogo aperto", form, resp.StatusCode, avvisoF(html))
		}
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("le POST rifiutate senza htmx hanno scritto in: %s", d)
	}
	resp, html := w.fai(http.MethodPost, s.base()+"/conferma", url.Values{"firma": {firma}, "conferma": {"1"}, "selezione": {"1"}, "voce": tutte}, false)
	if resp.StatusCode != 200 || !strings.Contains(html, `<html lang="it">`) || avvisoF(html) != "Fascicolo confermato: 3 documenti (copie sul NAS in coda)." {
		t.Fatalf("la conferma senza htmx: %d, %q", resp.StatusCode, avvisoF(html))
	}
	if strings.Contains(html, "Conferma e copia sul NAS") {
		t.Error("dopo la conferma la pagina non riapre il riepilogo")
	}
	if got := s.percorsi(); got != "7120001 foglio 2.pdf@"+percorsoFoglio2+" | 7120001.pdf@"+percorsoFoglio1+" | Capitolato ACME.pdf@"+percorsoCapitolato {
		t.Errorf("documenti: %s", got)
	}
}

// Le caselle del riepilogo: senza spunte non si conferma niente (senza voci la conferma e' tutto il pronto, e un
// riepilogo lasciato senza spunte non deve diventarlo); togliere il primo foglio lascerebbe al secondo il nome del
// primo, un percorso diverso da quello visto, e la conferma si rifiuta senza scrivere; togliere il capitolato va, e
// i due fogli stanno dove diceva il riepilogo. Il «✓ Conferma» di un file solo (una voce, senza firma) resta un
// gesto a parte, com'era: il capitolato entra cosi'.
func TestLeCaselleDelRiepilogoEIlFileSolo(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaRiepilogo("RIE41C")
	w := operatore(b)
	firma, _ := s.riepilogo(w)

	prima := fotoDelDatabase(t, b)
	if a, _, _ := s.conferma(w, url.Values{"firma": {firma}, "conferma": {"1"}, "selezione": {"1"}}); !strings.Contains(a, "non è rimasto spuntato niente") {
		t.Errorf("senza spunte: %q", a)
	}
	a, _, _ := s.conferma(w, url.Values{"firma": {firma}, "conferma": {"1"}, "selezione": {"1"}, "voce": {s.foglio2.String(), s.capitolato.String()}})
	if !strings.Contains(a, "7120001 foglio 2.pdf: sul NAS andrebbe in "+percorsoFoglio1+", non in "+percorsoFoglio2+" come diceva il riepilogo") {
		t.Errorf("senza il primo foglio: %q", a)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("le conferme rifiutate hanno scritto in: %s", d)
	}

	a, _, _ = s.conferma(w, url.Values{"firma": {firma}, "conferma": {"1"}, "selezione": {"1"}, "voce": {s.foglio1.String(), s.foglio2.String()}})
	if a != "Fascicolo confermato: 2 documenti (copie sul NAS in coda)." {
		t.Fatalf("senza il capitolato: %q", a)
	}
	if got := s.percorsi(); got != "7120001 foglio 2.pdf@"+percorsoFoglio2+" | 7120001.pdf@"+percorsoFoglio1 {
		t.Errorf("documenti: %s", got)
	}
	if got := s.valore(`SELECT stato::text FROM documento_proposta WHERE proposta_id = $1`, s.capitolato); got != "aperta" {
		t.Errorf("il capitolato tolto dalla conferma: %s", got)
	}

	// il capitolato, con il «✓ Conferma» del pannello (una voce e nient'altro): il gesto di un file solo
	_, html := w.daFascicolo(http.MethodPost, s.base()+"/conferma", url.Values{"voce": {s.capitolato.String()}}, s.thread, "")
	if a := avvisoF(html); a != "Fascicolo confermato: 1 documento (copie sul NAS in coda)." {
		t.Errorf("il file solo: %q", a)
	}
	if got := s.percorsi(); !strings.HasSuffix(got, "Capitolato ACME.pdf@"+percorsoCapitolato) {
		t.Errorf("il capitolato: %s", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 3 {
		t.Errorf("copie sul NAS in coda: %d", n)
	}
}

// valore e' una colonna di una riga, come testo ("NULL" se manca).
func (s *scenaRiepilogo) valore(sql string, arg ...any) string {
	s.b.t.Helper()
	var v *string
	if err := s.b.pool.QueryRow(s.b.ctx, sql, arg...).Scan(&v); err != nil {
		s.b.t.Fatalf("%v\n%s", err, sql)
	}
	if v == nil {
		return "NULL"
	}
	return *v
}
