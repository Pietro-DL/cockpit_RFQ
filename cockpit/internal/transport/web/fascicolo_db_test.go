//go:build integrazione

// L4 — B8.3: la conferma di un allegato non decide la struttura tecnica, e l'assegnazione dei
// documenti ai componenti segue le regole di A1.4 (decisione D18).
//
// Le prove guardano tre cose. Che la conferma non crei componenti e agganci solo quello scelto. Che
// assegnare un file a un componente con un altro codice sia una correzione ESPLICITA, e che una
// correzione cambi il percorso sul NAS solo finche' il file sul NAS non c'e': prima della copia, mai
// in corsa con lei, mai per un file gia' scritto (lo spostamento e' B8.8). E che il confine della RFQ
// lo tenga il database: la FK (thread, componente, codice) rifiuta da sola quello che la rotta rifiuta.
package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

var tutteAccese = coda.Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}

// rfqFascicolo e' una RFQ con un messaggio agganciato, i cui allegati hanno un contenuto VERO in
// staging: la copia sul NAS verifica l'hash, e una prova che copia davvero non puo' usare hash finti.
type rfqFascicolo struct {
	b        *bancoWeb
	thread   uuid.UUID
	msg      uuid.UUID
	utente   uuid.UUID
	cartella string // cartella_relativa della RFQ
	staging  string
	n        int
}

func (b *bancoWeb) rfqFascicolo(cliente uuid.UUID, chiave string) *rfqFascicolo {
	b.t.Helper()
	r := &rfqFascicolo{b: b, cartella: `ACME\WIP\2026 09 23 ` + chiave, staging: b.t.TempDir()}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook', now(), $2, $3, 1) RETURNING thread_id`, cliente, "RFQ "+chiave, r.cartella).Scan(&r.thread); err != nil {
		b.t.Fatal(err)
	}
	r.msg = b.messaggioIn("FASC-" + chiave)
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, r.msg, r.thread); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&r.utente); err != nil {
		b.t.Fatal(err)
	}
	return r
}

func (r *rfqFascicolo) componente(codice string) uuid.UUID {
	r.b.t.Helper()
	var id uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da)
		VALUES ($1, $2, 'sciolto', $3) RETURNING componente_id`, r.thread, codice, r.utente).Scan(&id); err != nil {
		r.b.t.Fatal(err)
	}
	return id
}

// allegato mette in staging un contenuto nuovo (ogni chiamata un hash diverso) e ne fa un allegato
// del messaggio della RFQ.
func (r *rfqFascicolo) allegato(nome string) (uuid.UUID, string) {
	r.b.t.Helper()
	r.n++
	percorso := filepath.Join(r.staging, fmt.Sprintf("%d_%s", r.n, nome))
	if err := os.WriteFile(percorso, []byte(fmt.Sprintf("contenuto %d di %s in %s", r.n, nome, r.cartella)), 0o644); err != nil {
		r.b.t.Fatal(err)
	}
	sha, n, err := nas.Sha256File(percorso)
	if err != nil {
		r.b.t.Fatal(err)
	}
	var id uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,$2,$3,'pdf','file','outlook',$4,$5,$6,'analizzato',now()) RETURNING allegato_id`,
		r.msg, r.n, nome, n, sha, percorso).Scan(&id); err != nil {
		r.b.t.Fatal(err)
	}
	return id, sha
}

// documento e' un disegno 2D gia' confermato nel fascicolo, con il percorso che la conferma gli
// avrebbe dato. In coda = con la sua copia pendente, come dopo una conferma vera.
func (r *rfqFascicolo) documento(nome, codice string, comp uuid.UUID, stato db.StatoNas) uuid.UUID {
	r.b.t.Helper()
	return r.documentoConID(uuid.New(), nome, codice, comp, stato)
}

func (r *rfqFascicolo) documentoConID(id uuid.UUID, nome, codice string, comp uuid.UUID, stato db.StatoNas) uuid.UUID {
	r.b.t.Helper()
	_, sha := r.allegato(nome)
	cid := uuid.NullUUID{UUID: comp, Valid: comp != uuid.Nil}
	if _, err := r.b.pool.Exec(r.b.ctx, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, nome_file, estensione,
		sha256, bytes, path_relativo, stato_nas, errore_nas, confermato_da)
		VALUES ($1,$2,$3,'disegno_2d',$4,$5,'pdf',$6,10,$7,$8, CASE WHEN $8::stato_nas = 'errore' THEN 'NAS non raggiungibile' END, $9)`,
		id, r.thread, cid, codice, nome, sha, `ELENCO DISEGNI\`+strings.ToUpper(codice)+`\`+nome, stato, r.utente); err != nil {
		r.b.t.Fatal(err)
	}
	if stato == db.StatoNasInCoda {
		if _, err := coda.AccodaCopia(r.b.ctx, r.b.q, id); err != nil {
			r.b.t.Fatal(err)
		}
	}
	return id
}

// proposta e' un file non ancora confermato; codice "" = il worker non l'ha letto; comp = gia' assegnata.
func (r *rfqFascicolo) proposta(nome, codice string, comp uuid.UUID) uuid.UUID {
	r.b.t.Helper()
	a, _ := r.allegato(nome)
	var id uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		VALUES ($1,$2,'disegno_2d',NULLIF($3,''),$4,60,'estensione') RETURNING proposta_id`,
		a, r.thread, codice, uuid.NullUUID{UUID: comp, Valid: comp != uuid.Nil}).Scan(&id); err != nil {
		r.b.t.Fatal(err)
	}
	return id
}

// foto e' lo stato di un documento che una correzione puo' toccare: componente, codice, percorso, NAS.
func (b *bancoWeb) foto(doc uuid.UUID) string {
	b.t.Helper()
	d, err := b.q.GetDocumento(b.ctx, doc)
	if err != nil {
		b.t.Fatal(err)
	}
	comp := "-"
	if d.ComponenteID.Valid {
		comp = d.ComponenteID.UUID.String()[:8]
	}
	return fmt.Sprintf("componente=%s codice=%s path=%s nas=%s", comp, d.Codice.String, d.PathRelativo, d.StatoNas)
}

func (b *bancoWeb) fotoProposta(p uuid.UUID) string {
	b.t.Helper()
	x, err := b.q.GetProposta(b.ctx, p)
	if err != nil {
		b.t.Fatal(err)
	}
	comp := "-"
	if x.ComponenteID.Valid {
		comp = x.ComponenteID.UUID.String()[:8]
	}
	return fmt.Sprintf("componente=%s codice=%s stato=%s", comp, x.Codice.String, x.Stato)
}

func (b *bancoWeb) codiceComponente(c uuid.UUID) string {
	b.t.Helper()
	x, err := b.q.GetComponente(b.ctx, c)
	if err != nil {
		b.t.Fatal(err)
	}
	return x.Codice
}

// copieDi sono tutti i job copia_nas di un documento, di qualunque stato.
func (b *bancoWeb) copieDi(doc uuid.UUID) []db.Job {
	b.t.Helper()
	var out []db.Job
	for _, j := range b.copieInCoda() {
		if j.ChiaveIdempotenza.String == coda.ChiaveCopia(doc) {
			out = append(out, j)
		}
	}
	return out
}

var reAvviso = regexp.MustCompile(`(?s)<div class="avviso">(.*?)</div>`)

// avvisoDi e' il testo dell'avviso in testa alla pagina della RFQ, come lo legge l'operatore.
func avvisoDi(html string) string {
	if m := reAvviso.FindStringSubmatch(html); m != nil {
		return leggibile(m[1])
	}
	return ""
}

func (r *rfqFascicolo) assegna(w *browser, form url.Values) string {
	r.b.t.Helper()
	_, html := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/assegna", form, true)
	return avvisoDi(html)
}

func (r *rfqFascicolo) correggiCodice(w *browser, comp uuid.UUID, codice string) string {
	r.b.t.Helper()
	_, html := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/componente/"+comp.String()+"/codice",
		url.Values{"codice": {codice}}, true)
	return avvisoDi(html)
}

func operatore(b *bancoWeb) *browser {
	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	return w
}

// ---------------------------------------------------------------- la conferma

// Fino al B8.2 confermare un disegno con un codice creava il componente di quel codice, «finito» se il
// codice era fra quelli della richiesta. Adesso la conferma crea il documento e basta: anche con il
// codice del prodotto della richiesta, i componenti restano zero.
func TestLaConfermaNonCreaComponenti(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	thread, proposta := b.propostaTecnica("NC1", "cad_3d", "52920517")
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, '52920517', 'proposta_oggetto')`, thread); err != nil {
		t.Fatal(err)
	}

	_, html := operatore(b).fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma", url.Values{}, true)
	if !strings.Contains(html, "Confermato:") || !strings.Contains(leggibile(html), "Senza componente") {
		t.Fatalf("la conferma non e' riuscita, o non dice che il documento e' senza componente:\n%s", estrai(html, "avviso"))
	}
	var comp uuid.NullUUID
	var codice string
	if err := b.pool.QueryRow(b.ctx, `SELECT componente_id, codice FROM documento WHERE thread_id = $1`, thread).Scan(&comp, &codice); err != nil {
		t.Fatal(err)
	}
	if comp.Valid {
		t.Errorf("il documento e' stato agganciato al componente %s: la conferma non sceglie componenti", comp.UUID)
	}
	if codice != "52920517" {
		t.Errorf("codice del documento %q, atteso quello confermato, 52920517", codice)
	}
	if n := b.contaNelThread("componente", thread); n != 0 {
		t.Errorf("la conferma ha creato %d componenti: la struttura non la decide un allegato", n)
	}
	if n := b.contaNelThread("componente_relazione", thread); n != 0 {
		t.Errorf("la conferma ha creato %d relazioni", n)
	}
}

// Il componente scelto nel form, oppure quello a cui la proposta era gia' stata assegnata: il
// documento lo prende, con il codice del componente scritto com'e' scritto li'.
func TestLaConfermaAssegnaIlComponenteScelto(t *testing.T) {
	t.Run("scelto nel form", func(t *testing.T) {
		b := preparaBancoWeb(t)
		ImpostaCapacitaProva(t, tutteAccese)
		thread, proposta := b.propostaTecnica("CS1", "cad_3d", "AB12")
		r := &rfqFascicolo{b: b, thread: thread}
		if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&r.utente); err != nil {
			t.Fatal(err)
		}
		comp := r.componente("ab12")

		_, html := operatore(b).fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma",
			url.Values{"componente_id": {comp.String()}}, true)
		if !strings.Contains(html, "Assegnato al componente ab12") {
			t.Fatalf("la conferma non dice a quale componente e' andato il documento:\n%s", estrai(html, "avviso"))
		}
		var agganciato uuid.NullUUID
		var codice, path string
		if err := b.pool.QueryRow(b.ctx, `SELECT componente_id, codice, path_relativo FROM documento WHERE thread_id = $1`, thread).
			Scan(&agganciato, &codice, &path); err != nil {
			t.Fatal(err)
		}
		if agganciato.UUID != comp || codice != "ab12" {
			t.Errorf("documento su %v con codice %q; atteso il componente scelto e il suo codice, ab12", agganciato, codice)
		}
		if path != `ELENCO DISEGNI\AB12\assieme.stp` {
			t.Errorf("percorso %q", path)
		}
		if n := b.contaNelThread("componente", thread); n != 1 {
			t.Errorf("componenti: %d, atteso 1", n)
		}
	})

	t.Run("gia' assegnato alla proposta", func(t *testing.T) {
		b := preparaBancoWeb(t)
		ImpostaCapacitaProva(t, tutteAccese)
		cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
		r := b.rfqFascicolo(cliente, "CS2")
		comp := r.componente("CD34")
		p := r.proposta("cd34.pdf", "", uuid.Nil)
		w := operatore(b)

		// una proposta senza codice si assegna senza correzioni: prende il codice del componente (A2.2)
		if a := r.assegna(w, url.Values{"componente": {comp.String()}, "proposta": {p.String()}}); !strings.Contains(a, "assegnato al componente CD34") {
			t.Fatalf("assegnazione della proposta: %q", a)
		}
		if got, want := b.fotoProposta(p), "componente="+comp.String()[:8]+" codice=CD34 stato=aperta"; got != want {
			t.Fatalf("proposta: %s, atteso %s", got, want)
		}
		_, html := w.fai(http.MethodPost, "/proposta/"+p.String()+"/conferma", url.Values{}, true)
		if !strings.Contains(html, "Assegnato al componente CD34") {
			t.Fatalf("la conferma non ha usato il componente della proposta:\n%s", estrai(html, "avviso"))
		}
		var agganciato uuid.NullUUID
		if err := b.pool.QueryRow(b.ctx, `SELECT componente_id FROM documento WHERE thread_id = $1`, r.thread).Scan(&agganciato); err != nil {
			t.Fatal(err)
		}
		if agganciato.UUID != comp {
			t.Errorf("documento agganciato a %v, atteso %s", agganciato, comp)
		}
	})

	t.Run("codice diverso: solo con la correzione", func(t *testing.T) {
		b := preparaBancoWeb(t)
		ImpostaCapacitaProva(t, tutteAccese)
		thread, proposta := b.propostaTecnica("CS3", "cad_3d", "XY99")
		r := &rfqFascicolo{b: b, thread: thread}
		if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&r.utente); err != nil {
			t.Fatal(err)
		}
		comp := r.componente("AB12")
		w := operatore(b)

		_, html := w.fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma", url.Values{"componente_id": {comp.String()}}, true)
		if !strings.Contains(leggibile(html), "codice diverso: XY99 contro AB12") {
			t.Fatalf("un codice diverso da quello del componente e' passato senza correzione:\n%s", estrai(html, "avviso"))
		}
		if n := b.contaNelThread("documento", thread); n != 0 {
			t.Fatalf("%d documenti dopo una conferma rifiutata", n)
		}
		_, html = w.fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma",
			url.Values{"componente_id": {comp.String()}, "correggi_codice": {"1"}}, true)
		if !strings.Contains(html, "Assegnato al componente AB12") {
			t.Fatalf("con la correzione la conferma doveva riuscire:\n%s", estrai(html, "avviso"))
		}
		var codice, path string
		if err := b.pool.QueryRow(b.ctx, `SELECT codice, path_relativo FROM documento WHERE thread_id = $1`, thread).Scan(&codice, &path); err != nil {
			t.Fatal(err)
		}
		if codice != "AB12" || path != `ELENCO DISEGNI\AB12\assieme.stp` {
			t.Errorf("codice %q, percorso %q: attesi quelli del componente", codice, path)
		}
	})
}

// ---------------------------------------------------------------- l'assegnazione

// Agganciare a un componente dello stesso codice non tocca il NAS: niente job nuovi, percorso e stato
// come prima, anche per un file gia' scritto. Le maiuscole diverse non sono una correzione: il
// documento prende la stringa del componente, e il percorso (che usa il codice in maiuscolo) non cambia.
func TestAssegnareUnDocumentoNonToccaIlNas(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "NAS1")
	comp := r.componente("ab12")
	inCoda := r.documento("in-coda.pdf", "AB12", uuid.Nil, db.StatoNasInCoda)
	scritto := r.documento("scritto.pdf", "AB12", uuid.Nil, db.StatoNasScritto)
	copie := b.copieInCoda()

	a := r.assegna(operatore(b), url.Values{"componente": {comp.String()}, "documento": {inCoda.String(), scritto.String()}})
	if !strings.Contains(a, "2 file assegnati al componente ab12") {
		t.Fatalf("avviso: %q", a)
	}
	for doc, atteso := range map[uuid.UUID]string{
		inCoda:  "componente=" + comp.String()[:8] + ` codice=ab12 path=ELENCO DISEGNI\AB12\in-coda.pdf nas=in_coda`,
		scritto: "componente=" + comp.String()[:8] + ` codice=ab12 path=ELENCO DISEGNI\AB12\scritto.pdf nas=scritto`,
	} {
		if got := b.foto(doc); got != atteso {
			t.Errorf("documento: %s\natteso:    %s", got, atteso)
		}
	}
	dopo := b.copieInCoda()
	if len(dopo) != len(copie) || dopo[0].JobID != copie[0].JobID || dopo[0].Stato != db.StatoJobPronto {
		t.Errorf("le copie sono cambiate: prima %v, dopo %v", copie, dopo)
	}
}

// Regola 1 di A1.4 (D18): un file con un codice diverso da quello del componente non si assegna, a meno
// di chiederlo. Con correggi_codice il codice diventa quello del componente e il percorso segue.
func TestAssegnareAUnCodiceDiversoRichiedeLaCorrezione(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "DIV1")
	comp := r.componente("BBB222")
	doc := r.documento("d1.pdf", "AAA111", uuid.Nil, db.StatoNasInCoda)
	p := r.proposta("p1.pdf", "AAA111", uuid.Nil)
	job := b.copieDi(doc)[0]
	primaDoc, primaProp := b.foto(doc), b.fotoProposta(p)
	w := operatore(b)

	a := r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {doc.String()}, "proposta": {p.String()}})
	if !strings.Contains(a, "codice diverso: AAA111 contro BBB222") || !strings.Contains(a, "nessun file cambiato") {
		t.Fatalf("senza correzione l'assegnazione doveva essere rifiutata con il motivo: %q", a)
	}
	if b.foto(doc) != primaDoc || b.fotoProposta(p) != primaProp {
		t.Fatalf("un rifiuto ha cambiato qualcosa:\n%s\n%s", b.foto(doc), b.fotoProposta(p))
	}

	a = r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {doc.String()}, "proposta": {p.String()}, "correggi_codice": {"1"}})
	if !strings.Contains(a, "2 file assegnati al componente BBB222") {
		t.Fatalf("con la correzione l'assegnazione doveva riuscire: %q", a)
	}
	if got, want := b.foto(doc), "componente="+comp.String()[:8]+` codice=BBB222 path=ELENCO DISEGNI\BBB222\d1.pdf nas=in_coda`; got != want {
		t.Errorf("documento: %s\natteso:    %s", got, want)
	}
	if got, want := b.fotoProposta(p), "componente="+comp.String()[:8]+" codice=BBB222 stato=aperta"; got != want {
		t.Errorf("proposta: %s, atteso %s", got, want)
	}
	if copie := b.copieDi(doc); len(copie) != 1 || copie[0].JobID != job.JobID || copie[0].Stato != db.StatoJobPronto {
		t.Errorf("la correzione doveva lasciare la stessa copia in attesa, non farne un'altra: %v", copie)
	}
}

// Regole 2–4 di A1.4, una per stato: il percorso si corregge finche' il file non e' sul NAS (in coda,
// in errore); per uno scritto si rifiuta, perche' correggerlo vorrebbe dire spostarlo (B8.8).
func TestIlCodiceSiCorreggeSoloFinchéInCoda(t *testing.T) {
	casi := []struct {
		stato    db.StatoNas
		riesce   bool
		percorso string
	}{
		{db.StatoNasInCoda, true, `ELENCO DISEGNI\BBB222\d.pdf`},
		{db.StatoNasErrore, true, `ELENCO DISEGNI\BBB222\d.pdf`},
		{db.StatoNasScritto, false, `ELENCO DISEGNI\AAA111\d.pdf`},
	}
	for _, c := range casi {
		t.Run(string(c.stato), func(t *testing.T) {
			b := preparaBancoWeb(t)
			ImpostaCapacitaProva(t, tutteAccese)
			cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
			r := b.rfqFascicolo(cliente, "STATO-"+string(c.stato))
			comp := r.componente("BBB222")
			doc := r.documento("d.pdf", "AAA111", uuid.Nil, c.stato)
			prima := b.foto(doc)

			a := r.assegna(operatore(b), url.Values{"componente": {comp.String()}, "documento": {doc.String()}, "correggi_codice": {"1"}})
			d, err := b.q.GetDocumento(b.ctx, doc)
			if err != nil {
				t.Fatal(err)
			}
			if d.PathRelativo != c.percorso || d.StatoNas != c.stato {
				t.Errorf("percorso %q stato %s; atteso %q stato %s", d.PathRelativo, d.StatoNas, c.percorso, c.stato)
			}
			if c.riesce {
				if !strings.Contains(a, "assegnato") || d.Codice.String != "BBB222" {
					t.Errorf("la correzione doveva riuscire: %q, codice %q", a, d.Codice.String)
				}
				return
			}
			if !strings.Contains(a, "è già sul NAS in un'altra cartella") || !strings.Contains(a, "la correzione arriva con lo spostamento") {
				t.Errorf("per un file gia' scritto il rifiuto deve dire perche': %q", a)
			}
			if b.foto(doc) != prima {
				t.Errorf("il rifiuto ha cambiato il documento: %s, era %s", b.foto(doc), prima)
			}
		})
	}
}

// Una correzione di codice sposta il file nella cartella del codice nuovo e non lo rinomina: il nome
// resta quello che la conferma gli aveva dato, anche quando ripassarlo per NomeFileSicuro lo
// cambierebbe (un'estensione maiuscola salvata cosi'; fino a B8.A4-0 anche un nome senza estensione).
func TestLaCorrezioneCambiaLaCartellaNonIlNome(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "NOME")
	comp := r.componente("BBB222")
	senzaEstensione := r.documento("LEGGIMI", "AAA111", uuid.Nil, db.StatoNasErrore)
	maiuscola := r.documento("Tavola 1.PDF", "AAA111", uuid.Nil, db.StatoNasErrore)

	a := r.assegna(operatore(b), url.Values{"componente": {comp.String()}, "documento": {senzaEstensione.String(), maiuscola.String()}, "correggi_codice": {"1"}})
	if !strings.Contains(a, "2 file assegnati") {
		t.Fatalf("avviso: %q", a)
	}
	for doc, atteso := range map[uuid.UUID]string{senzaEstensione: `ELENCO DISEGNI\BBB222\LEGGIMI`, maiuscola: `ELENCO DISEGNI\BBB222\Tavola 1.PDF`} {
		d, err := b.q.GetDocumento(b.ctx, doc)
		if err != nil {
			t.Fatal(err)
		}
		if d.PathRelativo != atteso {
			t.Errorf("percorso %q, atteso %q: la correzione ha rinominato il file", d.PathRelativo, atteso)
		}
	}
}

// Regola 2: il percorso nuovo si scrive PRIMA della copia. Mentre la correzione e' aperta la copia in
// attesa e' bloccata e l'esecutore non la prende; dopo il COMMIT la prende — la stessa, non un'altra —
// e il file finisce nella cartella del codice nuovo. Le due strade che correggono un codice: assegnare
// correggendo, e correggere il codice del componente.
func TestLaCorrezioneInCodaRiscriveIlPercorsoPrimaDellaCopia(t *testing.T) {
	strade := map[string]func(r *rfqFascicolo, tx pgx.Tx, doc, vecchio, nuovo uuid.UUID) error{
		"assegna e correggi": func(r *rfqFascicolo, tx pgx.Tx, doc, _, nuovo uuid.UUID) error {
			_, err := assegnaAlComponente(r.b.ctx, db.New(tx), r.thread, uuid.NullUUID{UUID: nuovo, Valid: true}, []uuid.UUID{doc}, nil, true)
			return err
		},
		"codice del componente": func(r *rfqFascicolo, tx pgx.Tx, _, vecchio, _ uuid.UUID) error {
			_, err := correggiCodiceComponente(r.b.ctx, tx, db.New(tx), r.thread, vecchio, "BBB222")
			return err
		},
	}
	for nome, correggi := range strade {
		t.Run(nome, func(t *testing.T) {
			b := preparaBancoWeb(t)
			ImpostaCapacitaProva(t, tutteAccese)
			cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
			r := b.rfqFascicolo(cliente, "PRIMA")
			vecchio := r.componente("AAA111")
			nuovo := uuid.Nil
			if nome == "assegna e correggi" {
				nuovo = r.componente("BBB222")
			}
			doc := r.documento("d1.pdf", "AAA111", vecchio, db.StatoNasInCoda)
			job := b.copieDi(doc)[0]

			tx, err := b.pool.Begin(b.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(b.ctx)
			if err := correggi(r, tx, doc, vecchio, nuovo); err != nil {
				t.Fatalf("correzione: %v", err)
			}
			// la transazione e' aperta: la copia bloccata non la prende nessuno
			if j, err := coda.Claim(b.ctx, b.q, db.WorkerTipoServer, "server-prova", coda.Destinazione{}, 0); err != nil || j != nil {
				t.Fatalf("durante la correzione l'esecutore ha preso %v (err %v): avrebbe copiato al percorso vecchio", j, err)
			}
			if err := tx.Commit(b.ctx); err != nil {
				t.Fatal(err)
			}
			j, err := coda.Claim(b.ctx, b.q, db.WorkerTipoServer, "server-prova", coda.Destinazione{}, 0)
			if err != nil || j == nil {
				t.Fatalf("dopo la correzione la copia non si prende: %v %v", j, err)
			}
			if j.JobID != job.JobID {
				t.Errorf("e' partita la copia %d, non quella in attesa (%d)", j.JobID, job.JobID)
			}

			radice := t.TempDir()
			if _, err := documenti.CopiaSulNas(b.ctx, b.q, &nas.Scrittore{Radice: radice}, j, doc); err != nil {
				t.Fatalf("copia: %v", err)
			}
			cartella := filepath.Join(radice, "ACME", "WIP", "2026 09 23 PRIMA", "ELENCO DISEGNI")
			if _, err := os.Stat(filepath.Join(cartella, "BBB222", "d1.pdf")); err != nil {
				t.Errorf("il file non e' nella cartella del codice nuovo: %v", err)
			}
			if _, err := os.Stat(filepath.Join(cartella, "AAA111")); !os.IsNotExist(err) {
				t.Errorf("sul NAS e' nata la cartella del codice vecchio (err %v)", err)
			}
			if got := b.foto(doc); !strings.HasSuffix(got, `codice=BBB222 path=ELENCO DISEGNI\BBB222\d1.pdf nas=scritto`) {
				t.Errorf("documento dopo la copia: %s", got)
			}
		})
	}
}

// Regola 2, l'altra meta': se la copia e' gia' partita non si fa una corsa con lei. La correzione si
// rifiuta con «copia in corso, riprova fra poco», e niente cambia: la copia finisce dove il database
// diceva quando e' partita, e DB e NAS continuano a dire la stessa cosa.
func TestLaCorrezioneConCopiaInCorsoAspetta(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "INCORSO")
	vecchio := r.componente("AAA111")
	nuovo := r.componente("BBB222")
	doc := r.documento("d1.pdf", "AAA111", vecchio, db.StatoNasInCoda)
	j, err := coda.Claim(b.ctx, b.q, db.WorkerTipoServer, "server-prova", coda.Destinazione{}, 0)
	if err != nil || j == nil {
		t.Fatalf("la copia non parte: %v %v", j, err)
	}
	prima := b.foto(doc)
	w := operatore(b)

	a := r.assegna(w, url.Values{"componente": {nuovo.String()}, "documento": {doc.String()}, "correggi_codice": {"1"}})
	if !strings.Contains(a, "copia in corso, riprova fra poco") {
		t.Errorf("assegnare correggendo durante la copia: %q", a)
	}
	a = r.correggiCodice(w, vecchio, "CCC333")
	if !strings.Contains(a, "copia in corso, riprova fra poco") {
		t.Errorf("correggere il codice del componente durante la copia: %q", a)
	}
	if b.foto(doc) != prima || b.codiceComponente(vecchio) != "AAA111" {
		t.Fatalf("durante la copia qualcosa e' cambiato: %s, componente %s", b.foto(doc), b.codiceComponente(vecchio))
	}
	if c := b.copieDi(doc); len(c) != 1 || c[0].Stato != db.StatoJobInCorso || c[0].LeaseToken != j.LeaseToken {
		t.Errorf("la copia in corso e' stata toccata: %v", c)
	}

	// la copia finisce al percorso con cui era partita, e il database dice lo stesso
	radice := t.TempDir()
	if _, err := documenti.CopiaSulNas(b.ctx, b.q, &nas.Scrittore{Radice: radice}, j, doc); err != nil {
		t.Fatalf("copia: %v", err)
	}
	if _, err := os.Stat(filepath.Join(radice, "ACME", "WIP", "2026 09 23 INCORSO", "ELENCO DISEGNI", "AAA111", "d1.pdf")); err != nil {
		t.Errorf("il file non e' dove il database dice: %v", err)
	}
	if got := b.foto(doc); !strings.HasSuffix(got, `codice=AAA111 path=ELENCO DISEGNI\AAA111\d1.pdf nas=scritto`) {
		t.Errorf("documento dopo la copia: %s", got)
	}
}

// Regola 5: correggere il codice di un componente porta il codice nuovo su tutto quello che vi e'
// agganciato — documenti in coda e in errore con il loro percorso, proposte — in una transazione sola,
// con la FK differita al COMMIT. Le copie in attesa restano quelle, e partiranno verso il percorso nuovo.
func TestCorreggereIlCodiceDelComponenteAggiornaIDocumentiInCoda(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "COMP1")
	comp := r.componente("AAA111")
	inCoda := r.documento("coda.pdf", "AAA111", comp, db.StatoNasInCoda)
	inErrore := r.documento("errore.pdf", "AAA111", comp, db.StatoNasErrore)
	p := r.proposta("proposta.pdf", "AAA111", comp)
	job := b.copieDi(inCoda)[0]

	a := r.correggiCodice(operatore(b), comp, "BBB222")
	if !strings.Contains(a, "Codice corretto: AAA111 → BBB222 (2 documenti, 1 proposta)") {
		t.Fatalf("avviso: %q", a)
	}
	if c := b.codiceComponente(comp); c != "BBB222" {
		t.Errorf("componente: %q", c)
	}
	id := comp.String()[:8]
	for doc, atteso := range map[uuid.UUID]string{
		inCoda:   "componente=" + id + ` codice=BBB222 path=ELENCO DISEGNI\BBB222\coda.pdf nas=in_coda`,
		inErrore: "componente=" + id + ` codice=BBB222 path=ELENCO DISEGNI\BBB222\errore.pdf nas=errore`,
	} {
		if got := b.foto(doc); got != atteso {
			t.Errorf("documento: %s\natteso:    %s", got, atteso)
		}
	}
	if got, want := b.fotoProposta(p), "componente="+id+" codice=BBB222 stato=aperta"; got != want {
		t.Errorf("proposta: %s, atteso %s", got, want)
	}
	if c := b.copieDi(inCoda); len(c) != 1 || c[0].JobID != job.JobID || c[0].Stato != db.StatoJobPronto {
		t.Errorf("la copia in attesa doveva restare quella: %v", c)
	}
	if n := len(b.copieInCoda()); n != 1 {
		t.Errorf("copie in coda: %d, attesa 1 (il documento in errore non si riaccoda da solo)", n)
	}
}

// Regola 5, il rifiuto: un solo documento gia' scritto che dovrebbe cambiare cartella ferma tutta la
// correzione, anche quello in coda che viene prima (documento_id piu' basso) e che da solo si sarebbe
// potuto correggere. Una correzione solo di maiuscole non sposta niente, e passa anche con lo scritto.
func TestCorreggereIlCodiceConUnDocumentoScrittoERifiutato(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "COMP2")
	comp := r.componente("AAA111")
	inCoda := r.documentoConID(uuid.MustParse("00000000-0000-0000-0000-000000000001"), "coda.pdf", "AAA111", comp, db.StatoNasInCoda)
	scritto := r.documentoConID(uuid.MustParse("00000000-0000-0000-0000-000000000002"), "scritto.pdf", "AAA111", comp, db.StatoNasScritto)
	p := r.proposta("proposta.pdf", "AAA111", comp)
	prima := []string{b.foto(inCoda), b.foto(scritto), b.fotoProposta(p)}
	w := operatore(b)

	a := r.correggiCodice(w, comp, "BBB222")
	if !strings.Contains(a, "scritto.pdf: il file è già sul NAS in un'altra cartella") || !strings.Contains(a, "la correzione arriva con lo spostamento") {
		t.Fatalf("il rifiuto deve nominare il file gia' scritto e dire perche': %q", a)
	}
	if c := b.codiceComponente(comp); c != "AAA111" {
		t.Errorf("il componente ha cambiato codice nonostante il rifiuto: %q", c)
	}
	if dopo := []string{b.foto(inCoda), b.foto(scritto), b.fotoProposta(p)}; strings.Join(dopo, "\n") != strings.Join(prima, "\n") {
		t.Errorf("il rifiuto non ha annullato tutto:\nprima %v\ndopo  %v", prima, dopo)
	}

	a = r.correggiCodice(w, comp, "aaa111")
	if !strings.Contains(a, "Codice corretto: AAA111 → aaa111") {
		t.Fatalf("una correzione di sole maiuscole non sposta niente e doveva riuscire: %q", a)
	}
	if got := b.foto(scritto); !strings.HasSuffix(got, `codice=aaa111 path=ELENCO DISEGNI\AAA111\scritto.pdf nas=scritto`) {
		t.Errorf("documento scritto dopo la correzione di maiuscole: %s", got)
	}
}

// Regola 6: sganciare non tocca ne' il codice ne' il percorso. Il documento torna «della RFQ» con il suo
// codice, anche se e' gia' sul NAS; la proposta tiene il suo.
func TestScollegareNonToccaCodiceNePercorso(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "SGANCIA")
	comp := r.componente("AAA111")
	doc := r.documento("d.pdf", "AAA111", comp, db.StatoNasScritto)
	p := r.proposta("p.pdf", "AAA111", comp)

	a := r.assegna(operatore(b), url.Values{"componente": {""}, "documento": {doc.String()}, "proposta": {p.String()}})
	if !strings.Contains(a, "2 file senza componente") {
		t.Fatalf("avviso: %q", a)
	}
	if got, want := b.foto(doc), `componente=- codice=AAA111 path=ELENCO DISEGNI\AAA111\d.pdf nas=scritto`; got != want {
		t.Errorf("documento: %s\natteso:    %s", got, want)
	}
	if got, want := b.fotoProposta(p), "componente=- codice=AAA111 stato=aperta"; got != want {
		t.Errorf("proposta: %s, atteso %s", got, want)
	}
}

// ---------------------------------------------------------------- il confine della RFQ

// Un componente di un'altra RFQ non si assegna: ne' a un documento, ne' a una proposta, ne' alla
// conferma. Anche quando ha lo stesso codice, che fra due RFQ dello stesso cliente e' il caso normale.
func TestNonSiAssegnaUnComponenteDiUnAltraRfq(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	qui := b.rfqFascicolo(cliente, "QUI")
	altrove := b.rfqFascicolo(cliente, "ALTROVE")
	estraneo := altrove.componente("AAA111")
	doc := qui.documento("d.pdf", "AAA111", uuid.Nil, db.StatoNasInCoda)
	p := qui.proposta("p.pdf", "AAA111", uuid.Nil)
	prima := []string{b.foto(doc), b.fotoProposta(p)}
	w := operatore(b)

	a := qui.assegna(w, url.Values{"componente": {estraneo.String()}, "documento": {doc.String()}, "proposta": {p.String()}})
	if !strings.Contains(a, "il componente AAA111 non è di questa RFQ") {
		t.Errorf("assegna: %q", a)
	}
	// e dalla pagina dell'altra RFQ: e' il documento a non essere suo
	a = altrove.assegna(w, url.Values{"componente": {estraneo.String()}, "documento": {doc.String()}})
	if !strings.Contains(a, "un documento scelto non è di questa RFQ") {
		t.Errorf("assegna dall'altra RFQ: %q", a)
	}
	_, html := w.fai(http.MethodPost, "/proposta/"+p.String()+"/conferma", url.Values{"componente_id": {estraneo.String()}}, true)
	if !strings.Contains(leggibile(html), "il componente AAA111 non è di questa RFQ") {
		t.Errorf("conferma:\n%s", estrai(html, "avviso"))
	}
	if dopo := []string{b.foto(doc), b.fotoProposta(p)}; strings.Join(dopo, "\n") != strings.Join(prima, "\n") {
		t.Errorf("qualcosa e' cambiato:\nprima %v\ndopo  %v", prima, dopo)
	}
	if n := b.contaNelThread("documento", qui.thread); n != 1 {
		t.Errorf("documenti nella RFQ: %d, atteso 1 (la conferma rifiutata non ne crea)", n)
	}
}

// Il controllo in Go serve a dire all'operatore che cosa non va; chi lo impedisce e' il database. Le
// stesse UPDATE che usano le rotte, lanciate senza nessun controllo davanti, falliscono sulla FK
// (thread, componente, codice): un componente di un'altra RFQ, o un codice diverso dal suo.
func TestLaFkRestaLAutoritaSullAssegnazione(t *testing.T) {
	b := preparaBancoWeb(t)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	qui := b.rfqFascicolo(cliente, "FK1")
	altrove := b.rfqFascicolo(cliente, "FK2")
	mio := qui.componente("AAA111")
	estraneo := altrove.componente("AAA111")
	doc := qui.documento("d.pdf", "AAA111", uuid.Nil, db.StatoNasScritto)
	p := qui.proposta("p.pdf", "AAA111", uuid.Nil)
	testo := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	nullo := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }

	scritture := map[string]func() error{
		"documento su un componente di un'altra RFQ": func() error {
			_, err := b.q.SetComponenteDocumento(b.ctx, db.SetComponenteDocumentoParams{DocumentoID: doc, ComponenteID: nullo(estraneo), Codice: testo("AAA111")})
			return err
		},
		"documento con un codice diverso da quello del componente": func() error {
			_, err := b.q.SetComponenteDocumento(b.ctx, db.SetComponenteDocumentoParams{DocumentoID: doc, ComponenteID: nullo(mio), Codice: testo("aaa111")})
			return err
		},
		"proposta su un componente di un'altra RFQ": func() error {
			_, err := b.q.SetComponenteProposta(b.ctx, db.SetComponentePropostaParams{PropostaID: p, ComponenteID: nullo(estraneo), Codice: testo("AAA111")})
			return err
		},
		"codice del componente senza differire la FK": func() error {
			if _, err := b.q.SetComponenteDocumento(b.ctx, db.SetComponenteDocumentoParams{DocumentoID: doc, ComponenteID: nullo(mio), Codice: testo("AAA111")}); err != nil {
				return fmt.Errorf("preparazione: %w", err)
			}
			return b.q.SetCodiceComponente(b.ctx, db.SetCodiceComponenteParams{ComponenteID: mio, Codice: "BBB222"})
		},
	}
	for nome, scrivi := range scritture {
		t.Run(nome, func(t *testing.T) {
			err := scrivi()
			var pg *pgconn.PgError
			if !errors.As(err, &pg) || (pg.ConstraintName != "fk_documento_componente" && pg.ConstraintName != "fk_proposta_componente") {
				t.Errorf("la scrittura diretta doveva fallire sulla FK composita, invece: %v", err)
			}
		})
	}
}

// Il ruolo consultazione vede e non cambia: la regola per metodo del server vale anche qui.
func TestLaConsultazioneNonAssegna(t *testing.T) {
	b := preparaBancoWeb(t)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "RUOLO")
	comp := r.componente("AAA111")
	doc := r.documento("d.pdf", "AAA111", uuid.Nil, db.StatoNasScritto)
	prima := b.foto(doc)
	w := b.browser("10.0.0.1")
	w.login("CO", "prova-co")

	resp, _ := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/assegna",
		url.Values{"componente": {comp.String()}, "documento": {doc.String()}}, true)
	if resp.StatusCode != http.StatusForbidden || b.foto(doc) != prima {
		t.Errorf("assegna dalla consultazione: HTTP %d, documento %s", resp.StatusCode, b.foto(doc))
	}
	resp, _ = w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/componente/"+comp.String()+"/codice",
		url.Values{"codice": {"BBB222"}}, true)
	if c := b.codiceComponente(comp); resp.StatusCode != http.StatusForbidden || c != "AAA111" {
		t.Errorf("correzione del codice dalla consultazione: HTTP %d, componente %q", resp.StatusCode, c)
	}
}
