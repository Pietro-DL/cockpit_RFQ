//go:build integrazione

package filerfq

// L4 — V1, ondata B, corsia file, secondo passaggio: le prove dei rilievi della revisione sul ciclo dei file (la
// bonifica oltre la prima pagina e il custode a lotti, controllo e azione sotto gli stessi lucchetti, il tentativo
// chiuso su un archivio, lo scarto col gesto precedente dopo una «Riattiva», il parziale di E1′ e la pulizia della
// coda, la regola archivio → voci del tempo 2, G-15). Ognuna parte dalla sonda del revisore e prova il comportamento
// corretto. DB di prova, staging e deposito temporanei, contenuti e nomi sintetici (ACME).

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// attendiAttesaSuRiga: aspetta (al piu' 10 secondi) che un'altra connessione sia ferma su un lucchetto di riga.
func (b *bancoFile) attendiAttesaSuRiga(escluso int32) {
	b.t.Helper()
	fine := time.Now().Add(10 * time.Second)
	for time.Now().Before(fine) {
		var n int
		if err := b.p.QueryRow(b.ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> $1`, escluso).Scan(&n); err != nil {
			b.t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b.t.Fatal("nessuna transazione si e' fermata su un lucchetto di riga")
}

// scartoLegacy: il vecchio «Scarta il file» delle pagine legacy (DecidiProposta: scartata, con chi decide).
func (b *bancoFile) scartoLegacy(allegato uuid.UUID) {
	b.t.Helper()
	b.esegui(`UPDATE documento_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1 AND stato = 'aperta'`,
		allegato, b.u.UtenteID)
}

// fileVecchioPerMessaggio: un allegato il cui path_staging e' nello staging vecchio per messaggio (sotto la radice,
// fuori da _contenuti): lo «Scarta» lo lascia in errore / fuori_cache.
func (b *bancoFile) fileVecchioPerMessaggio(nome string, seme int64) (uuid.UUID, string) {
	b.t.Helper()
	p := filepath.Join(b.staging, "PER_MESSAGGIO", nome)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		b.t.Fatal(err)
	}
	c := contenuto(nome, seme, 1_000)
	if err := os.WriteFile(p, c, 0o644); err != nil {
		b.t.Fatal(err)
	}
	a, _ := b.file(b.mail(b.thread), nome, c, opzFile{fuoriStaging: p})
	return a, p
}

// Rilievo bloccante: la bonifica giornaliera prende OGNI riga in errore non ritentabile (EM §10.4), una pagina alla
// volta, anche quando le prime restano in errore perche' i loro file ci sono ancora.
func TestBonificaOltreLaPrimaPagina(t *testing.T) {
	b := nuovoBancoFile(t)
	var occ []uuid.UUID
	var file []string
	for i := 0; i < 5; i++ {
		a, p := b.fileVecchioPerMessaggio(fmt.Sprintf("ACME-BON-%d.pdf", i), int64(9100+i))
		b.deveRiuscire(b.scarta(b.thread, a))
		if got := b.rimozione(b.thread, a); got != "errore/fuori_cache" {
			t.Fatalf("rimozione %d: %s", i, got)
		}
		occ, file = append(occ, a), append(file, p)
	}
	// l'amministratore toglie i file della terza e della quinta: con pagine di 2 righe stanno nella seconda e nella terza
	for _, i := range []int{2, 4} {
		if err := os.Remove(file[i]); err != nil {
			t.Fatal(err)
		}
	}
	c := &Custode{S: b.s, Limite: 2}
	n, err := c.Bonifica(b.ctx)
	if err != nil || n != 2 {
		t.Fatalf("bonificate %d, %v; attese 2", n, err)
	}
	for i, a := range occ {
		atteso := "errore/fuori_cache"
		if i == 2 || i == 4 {
			atteso = "eseguita/bonificato"
		}
		if got := b.rimozione(b.thread, a); got != atteso {
			t.Errorf("riga %d: %s, atteso %s", i, got, atteso)
		}
	}
	for _, i := range []int{0, 1, 3} {
		if err := os.Remove(file[i]); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := c.Bonifica(b.ctx); err != nil || n != 3 {
		t.Fatalf("la seconda bonifica: %d, %v; attese 3", n, err)
	}
}

// Rilievo minore: una passata del custode prende tutte le righe scadute, una pagina alla volta (dopo lo «Scarta» di un
// archivio grande il tempo 2 e i rilasci non aspettano una passata per pagina).
func TestCustodeTutteLeRigheInUnaPassata(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	var scartati, daAltriFile []uuid.UUID
	for i := 0; i < 5; i++ {
		a, _ := b.pdf(fmt.Sprintf("ACME-CUS-%d.pdf", i))
		b.deveRiuscire(b.scarta(b.thread, a))
		scartati = append(scartati, a)
	}
	// con la base B di U2-Q01 (provvisoria [T]) si rilascia dal NAS solo per lo scarto: le tre copie da rilasciare sono
	// di file scartati da Altri file (ognuno porta anche la sua rimozione dallo staging)
	for i := 0; i < 3; i++ {
		a, _ := b.pdf(fmt.Sprintf("ACME-CUR-%d.pdf", i))
		b.deveRiuscire(b.altriFile(b.thread, a))
		b.deveRiuscire(b.scarta(b.thread, a))
		daAltriFile = append(daAltriFile, a)
	}
	e, err := (&Custode{S: b.s, Limite: 2}).Giro(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if e.Rimozioni != 8 || e.Rilasci != 3 {
		t.Errorf("passata: %+v; attese 8 rimozioni e 3 rilasci", e)
	}
	for _, a := range scartati {
		if got := b.rimozione(b.thread, a); got != "eseguita/rimosso" {
			t.Errorf("rimozione %s", got)
		}
	}
	for _, a := range daAltriFile {
		if got := b.copie(b.thread, a); !uguali(got, "rilasciata/scartato") {
			t.Errorf("copie %v", got)
		}
	}
}

// Rilievo importante (controllo e azione sotto gli stessi lucchetti), caso (a): una conferma legacy «duplicato» tiene
// la proposta mentre arriva «Scarta». Lo «Scarta» si ferma sulla proposta prima di calcolare le dipendenze; finita la
// conferma, la provenienza nuova porta D-1, e lo scarto e' rifiutato con 422 DIPENDENZE_ATTIVE (EM §10.3).
func TestConfermaLegacyDuranteLoScarta(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	a, sha := b.pdf("ACME-RV1.pdf")
	doc := b.documentoNAS(b.thread, "ACME-RV1", sha, "scritto")
	corpo := b.corpo(GestoScarta, b.thread, a, "non serve")
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	var pidLegacy int32
	if err := tx.QueryRow(b.ctx, `SELECT pg_backend_pid()`).Scan(&pidLegacy); err != nil {
		t.Fatal(err)
	}
	var proposta uuid.UUID
	if err := tx.QueryRow(b.ctx, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1 FOR UPDATE`, a).Scan(&proposta); err != nil {
		t.Fatal(err)
	}
	type esito struct {
		e   *EsitoGesto
		err error
	}
	fatto := make(chan esito, 1)
	go func() {
		e, err := b.s.Scarta(context.Background(), b.thread, a, b.u.UtenteID, corpo)
		fatto <- esito{e, err}
	}()
	b.attendiAttesaSuRiga(pidLegacy)
	// il resto di confermaProposta sul ramo «gia' presente nel fascicolo»: InsertProvenienza + DecidiProposta(duplicato)
	if _, err := tx.Exec(b.ctx, `INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())`, doc, a); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(b.ctx, `UPDATE documento_proposta SET stato = 'duplicato', deciso_da = $2, deciso_il = now()
		WHERE proposta_id = $1 AND stato = 'aperta'`, proposta, b.u.UtenteID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}
	r := <-fatto
	rf := rifiutoAtteso(t, r.err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
	if !citaDipendenza(rf, a, "D-1") {
		t.Errorf("D-1 non e' fra le dipendenze che bloccano: %v", rf.Extra["dipendenze"])
	}
	if got := b.stato(b.thread, a); got != "" {
		t.Errorf("lo scarto ha scritto lo stato %q di un documento confermato", got)
	}
	if got := b.statoProposta(a); got != "duplicato/-" {
		t.Errorf("proposta %q", got)
	}
}

// Rilievo importante (controllo e azione sotto gli stessi lucchetti), caso (b), con il rilievo dello scarto col gesto
// precedente: il vecchio «Scarta il file» durante la copia di «Metti in Altri file», su un file che ha gia' la riga
// attiva di una «Riattiva». La fase C prende le proposte, rilegge, vede lo scarto col gesto precedente e rifiuta (422
// FILE_SCARTATO, EM §10.2); la copia nuova va in rilascio e si rilascia, lo stato resta com'era.
func TestScartoLegacyDuranteLaFaseC(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-LFC.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	b.deveRiuscire(b.riattiva(b.thread, a))
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) { b.scartoLegacy(a) }}
	_, err := b.altriFile(b.thread, a)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FILE_SCARTATO")
	b.s.Agganci = nil
	if got := b.stato(b.thread, a); got != StatoAttivo {
		t.Errorf("stato %q", got)
	}
	// base B di U2-Q01 (provvisoria [T]): ne' la «Riattiva» ne' la fase C rifiutata tolgono la copia dal NAS; il
	// secondo tentativo ha riusato il file trattenuto della «Riattiva» (ceduta), e ora lo trattiene lui
	if got := b.copie(b.thread, a); !uguali(got, "ceduta/riattivato", "trattenuta/rifiutata") {
		t.Errorf("copie %v", got)
	}
	if got := b.fileDelDeposito(); len(got) != 1 {
		t.Errorf("file trattenuti nel posto delle copie: %v", got)
	}
	if f := fileDi(b.modello(b.thread, a), a); f == nil || f.Stato != StatoScartato || !f.ScartatoLegacy {
		t.Errorf("S1: %+v", f)
	}
}

// Rilievo importante: lo scarto col vecchio «Scarta il file» fatto DOPO una «Riattiva» (la riga di file_rfq c'e', in
// attivo) e' uno scarto col gesto precedente (EM §10.2, TR-17): S1 lo mostra scartato, «Metti in Altri file» e
// «Riattiva» rispondono 422 FILE_SCARTATO, «Scarta» c'e' (per togliere la copia dallo staging), e un ricaricamento crea
// un'occorrenza nuova con l'evento ricaricato_dopo_scarto (non gia_presente).
func TestScartoLegacyDopoUnaRiattiva(t *testing.T) {
	b := nuovoBancoFile(t)
	c := contenuto("ACME-RV6.pdf", 9600, 2_000)
	es, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, b.arrivato(c, "ACME-RV6.pdf"))
	if err != nil || es.Esito != "caricato" {
		t.Fatalf("caricamento: %+v %v", es, err)
	}
	a := *es.AllegatoID
	// la proposta che la strada d'ingresso vera crea (qui la strada e' finta)
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', 60, 'estensione')`, a, b.thread)
	b.deveRiuscire(b.altriFile(b.thread, a))
	b.deveRiuscire(b.riattiva(b.thread, a))
	b.scartoLegacy(a)
	f := fileDi(b.modello(b.thread, a), a)
	if f == nil || f.Stato != StatoScartato || !f.ScartatoLegacy || f.Azioni.AltriFile.Disponibile || f.Azioni.Riattiva.Disponibile ||
		!f.Azioni.Scarta.Disponibile {
		t.Fatalf("S1: %+v", f)
	}
	prima := b.foto()
	_, err = b.altriFile(b.thread, a)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FILE_SCARTATO")
	_, err = b.riattiva(b.thread, a)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FILE_SCARTATO")
	b.nienteScritto(prima, "Altri file e Riattiva su uno scarto col gesto precedente")
	// il ricaricamento: un'occorrenza nuova, non gia_presente
	es2, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, b.arrivato(c, "ACME-RV6.pdf"))
	if err != nil || es2.Esito != "caricato" || !es2.RicaricatoDopoScarto || es2.AllegatoID == nil || *es2.AllegatoID == a {
		t.Fatalf("ricaricamento: %+v %v", es2, err)
	}
	if got := b.azioni(b.thread, a); len(got) == 0 || got[len(got)-1] != "ricaricato_dopo_scarto" {
		t.Errorf("eventi della vecchia occorrenza: %v", got)
	}
	// «Scarta» chiude lo scarto col gesto precedente nel ciclo dei file (versione 3)
	e := b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	if e.Versione != 3 || b.stato(b.thread, a) != StatoScartato {
		t.Errorf("Scarta: versione %d, stato %q", e.Versione, b.stato(b.thread, a))
	}
}

// Rilievo importante (TR-05): «Metti in Altri file» di un archivio; uno «Scarta» di una voce dopo la copia chiude la sua
// riga: la fase C risponde 409 tentativo_chiuso, e le ALTRE righe del gesto (l'archivio, l'altra voce) vanno in rilascio
// e si rilasciano, invece di restare in copia con il battito fermo; un nuovo tentativo riesce subito.
func TestTentativoChiusoSuUnArchivioNellaFaseC(t *testing.T) {
	b := nuovoBancoFile(t)
	zip, voci := b.archivio(b.thread, "ACME-RV3.zip", []string{"V1.pdf", "V2.pdf"}, opzFile{})
	corpoZip := b.corpo(GestoAltriFile, b.thread, zip, "da conservare")
	corpoV1 := b.corpo(GestoScarta, b.thread, voci[0], "non serve")
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) {
		if _, err := b.s.Scarta(context.Background(), b.thread, voci[0], b.u.UtenteID, corpoV1); err != nil {
			t.Errorf("lo scarto della voce durante la copia: %v", err)
		}
	}}
	_, err := b.s.MettiInAltriFile(b.ctx, b.thread, zip, b.u.UtenteID, corpoZip)
	rf := rifiutoAtteso(t, err, http.StatusConflict, "COPIA_IN_CORSO")
	if rf.Extra["motivo"] != "tentativo_chiuso" {
		t.Errorf("motivo %v", rf.Extra["motivo"])
	}
	b.s.Agganci = nil
	// le altre righe del gesto escono dalla copia: con la base B di U2-Q01 (provvisoria [T]) restano trattenute, il
	// loro file resta; la voce scartata si toglie (lo scarto e' l'unica causa che toglie una copia dal NAS)
	for _, x := range []uuid.UUID{zip, voci[1]} {
		if got := b.copie(b.thread, x); !uguali(got, "trattenuta/rifiutata") {
			t.Errorf("copie di %s: %v", x, got)
		}
	}
	if got := b.copie(b.thread, voci[0]); !uguali(got, "rilasciata/scartato") {
		t.Errorf("copie della voce scartata: %v", got)
	}
	if got := b.fileDelDeposito(); len(got) != 2 {
		t.Errorf("file trattenuti nel posto delle copie: %v", got)
	}
	// il nuovo tentativo, con una chiave nuova e il perimetro riletto (la voce scartata non c'e' piu')
	b.deveRiuscire(b.altriFile(b.thread, zip))
	if b.stato(b.thread, zip) != StatoAltriFile || b.stato(b.thread, voci[1]) != StatoAltriFile {
		t.Errorf("stati dopo il nuovo tentativo: %q %q", b.stato(b.thread, zip), b.stato(b.thread, voci[1]))
	}
}

// Rilievo importante (TR-05), nella copia: uno «Scarta» chiude la riga di una voce prima che la copia la scriva; la
// copia risponde 409 tentativo_chiuso e porta le altre righe del gesto a uno stato finale (errore_copia / rifiutata se
// non hanno un definitivo, rilasciata se ce l'hanno); nessuna resta in copia, e un nuovo tentativo riesce.
func TestTentativoChiusoSuUnArchivioNellaCopia(t *testing.T) {
	b := nuovoBancoFile(t)
	zip, voci := b.archivio(b.thread, "ACME-TCC.zip", []string{"V1.pdf", "V2.pdf", "V3.pdf"}, opzFile{})
	corpoZip := b.corpo(GestoAltriFile, b.thread, zip, "da conservare")
	bersaglio := voci[1]
	corpoV := b.corpo(GestoScarta, b.thread, bersaglio, "non serve")
	// SaltaTentativi: lo «Scarta» della voce non fa il tempo 2 subito, cosi' il file della voce resta nello staging e la
	// copia arriva alla rinomina, dove trova la riga chiusa
	b.s.Agganci = &Agganci{SaltaTentativi: true, PrimaDiCopiare: func(a uuid.UUID) {
		if a != bersaglio {
			return
		}
		if _, err := b.s.Scarta(context.Background(), b.thread, bersaglio, b.u.UtenteID, corpoV); err != nil {
			t.Errorf("lo scarto della voce durante la copia: %v", err)
		}
	}}
	_, err := b.s.MettiInAltriFile(b.ctx, b.thread, zip, b.u.UtenteID, corpoZip)
	rf := rifiutoAtteso(t, err, http.StatusConflict, "COPIA_IN_CORSO")
	if rf.Extra["motivo"] != "tentativo_chiuso" {
		t.Errorf("motivo %v", rf.Extra["motivo"])
	}
	b.s.Agganci = nil
	// la riga chiusa dallo «Scarta» la rilascia il custode (come avrebbe fatto il tentativo dopo il gesto)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	var nInCopia int
	b.rigaCon(`SELECT count(*) FROM file_rfq_conservazione WHERE thread_id = $1 AND stato = 'in_copia'`, []any{b.thread}, &nInCopia)
	if nInCopia != 0 {
		t.Errorf("%d righe restano in copia", nInCopia)
	}
	for _, x := range append([]uuid.UUID{zip}, voci...) {
		got := b.copie(b.thread, x)
		ok := uguali(got, "trattenuta/rifiutata") || uguali(got, "errore_copia/rifiutata")
		if x == bersaglio {
			ok = uguali(got, "rilasciata/scartato")
		}
		if !ok {
			t.Errorf("copie di %s: %v", x, got)
		}
	}
	trattenute := valore[int](b, `SELECT count(*) FROM file_rfq_conservazione WHERE thread_id = $1 AND stato = 'trattenuta'`, b.thread)
	if got := b.fileDelDeposito(); len(got) != trattenute {
		t.Errorf("file nel posto delle copie %v, copie trattenute %d", got, trattenute)
	}
	if got := b.partiDelDeposito(); len(got) != 0 {
		t.Errorf("parziali rimasti nel deposito: %v", got)
	}
	b.deveRiuscire(b.altriFile(b.thread, zip))
	if b.stato(b.thread, zip) != StatoAltriFile {
		t.Errorf("stato dell'archivio dopo il nuovo tentativo: %q", b.stato(b.thread, zip))
	}
}

// Rilievo importante: il parziale di E1′ non ha il nome dei tentativi dei worker, e la pulizia della coda
// (staging.PulisciParti, file piu' vecchi di 10 minuti con un token non vivo) non lo toglie anche se il file e' arrivato
// da 11 minuti e il resto della richiesta sta ancora arrivando: la registrazione lo trova e lo promuove. I parziali del
// ciclo dei file rimasti da un processo morto li toglie il custode, dopo 24 ore.
func TestParzialeDiE1ELaPuliziaDellaCoda(t *testing.T) {
	b := nuovoBancoFile(t)
	c := contenuto("ACME-RV7.pdf", 9300, 2_000)
	f := b.arrivato(c, "ACME-RV7.pdf")
	vecchio := time.Now().Add(-11 * time.Minute)
	if err := os.Chtimes(f.Parte, vecchio, vecchio); err != nil {
		t.Fatal(err)
	}
	if _, err := staging.PulisciParti(b.ctx, b.q, b.staging, 10*time.Minute, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	if !esiste(f.Parte) {
		t.Fatal("la pulizia della coda ha tolto il parziale di E1′")
	}
	es, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, f)
	if err != nil || es.Esito != "caricato" {
		t.Fatalf("registrazione: %+v %v", es, err)
	}
	if got := b.pathStaging(*es.AllegatoID); got != b.percorsoContenuto(shaDi(c), "ACME-RV7.pdf") || !esiste(got) {
		t.Errorf("contenuto: %q", got)
	}
	// un parziale rimasto da un processo morto, di ieri: il custode lo toglie; uno di oggi no
	morto := b.arrivato(contenuto("ACME-RV7b.pdf", 9301, 500), "ACME-RV7b.pdf")
	vivo := b.arrivato(contenuto("ACME-RV7c.pdf", 9302, 500), "ACME-RV7c.pdf")
	ieri := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(morto.Parte, ieri, ieri); err != nil {
		t.Fatal(err)
	}
	e, err := (&Custode{S: b.s}).Giro(b.ctx)
	if err != nil || e.PartiStagingRimosse != 1 || esiste(morto.Parte) || !esiste(vivo.Parte) {
		t.Errorf("custode: %+v %v; morto %v, vivo %v", e, err, esiste(morto.Parte), esiste(vivo.Parte))
	}
	staging.RimuoviParte(vivo.Parte)
}

// Rilievo importante (EM §10.4 passo 8): il tempo 2 calcola i pin con la regola archivio → voci della cache. Un archivio
// scartato resta nello staging (in_attesa / pin_transitorio) finche' una sua voce, per sha, ha un pin (qui la proposta
// aperta di un'altra occorrenza attiva con lo stesso contenuto della voce); finito il pin, si toglie.
func TestTempo2RegolaArchivioVoci(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	zip, voci := b.archivio(b.thread, "ACME-AV.zip", []string{"V1.pdf"}, opzFile{})
	cv, err := os.ReadFile(b.pathStaging(voci[0]))
	if err != nil {
		t.Fatal(err)
	}
	altra, _ := b.file(b.mail(b.thread), "V1-copia.pdf", cv, opzFile{}) // lo stesso contenuto della voce, attivo, proposta aperta
	pZip := b.pathStaging(zip)
	// prima la voce, poi lo ZIP: «Scarta» di uno ZIP con voci attive e' rifiutato (U2 §3.10)
	b.deveRiuscire(b.scarta(b.thread, voci[0]))
	b.deveRiuscire(b.scarta(b.thread, zip))
	if _, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, voci[0]), false); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, voci[0]); got != "non_necessaria_condiviso/condiviso" {
		t.Errorf("rimozione della voce: %s", got)
	}
	if _, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, zip), false); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, zip); got != "in_attesa/pin_transitorio" || !esiste(pZip) {
		t.Fatalf("rimozione dell'archivio con una voce pinnata: %s, file %v", got, esiste(pZip))
	}
	b.scartoLegacy(altra) // la proposta dell'altra occorrenza non e' piu' aperta: il pin cade
	if _, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, zip), false); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, zip); got != "eseguita/rimosso" || esiste(pZip) {
		t.Errorf("rimozione dell'archivio dopo il pin: %s, file %v", got, esiste(pZip))
	}
}

// G-15 (EM §10.7; rilievo della revisione: mancava la prova): lo scarto non chiude righe di nodi ne' di archi
// (componente_proposta, relazione_proposta): restano com'erano, come storia (D-4).
func TestG15LoScartoNonChiudeNodiNeArchi(t *testing.T) {
	b := nuovoBancoFile(t)
	s, sha := b.file(b.mail(b.thread), "ACME-G15.stp", contenuto("ACME-G15.stp", 1500, 3_000), opzFile{})
	for _, chiave := range []string{"n1", "n2"} {
		b.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza)
			VALUES ($1, $2, $3, $4, $5, 'step', 50)`, b.thread, s, sha, chiave, "ACME "+chiave)
	}
	b.esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave) VALUES ($1, $2, 'n1', 'n2')`, b.thread, s)
	b.deveRiuscire(b.scarta(b.thread, s, conUnicaCopia))
	if got := valore[[]string](b, `SELECT coalesce(array_agg(chiave || '/' || stato::text ORDER BY chiave), '{}') FROM componente_proposta
		WHERE allegato_id = $1`, s); !uguali(got, "n1/aperta", "n2/aperta") {
		t.Errorf("nodi dopo lo scarto: %v", got)
	}
	if got := valore[[]string](b, `SELECT coalesce(array_agg(padre_chiave || '>' || figlio_chiave || '/' || stato::text), '{}')
		FROM relazione_proposta WHERE allegato_id = $1`, s); !uguali(got, "n1>n2/aperta") {
		t.Errorf("archi dopo lo scarto: %v", got)
	}
}
