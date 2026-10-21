//go:build integrazione

package filerfq

// L4 — V1, direttiva 35, blocco C-6a (corsia file): la rimozione di una copia degli Altri file dalla cartella ALTRO
// della RFQ (U2 §3.7), fuori transazione: in_rimozione con il token della colonna token_rimozione, la rinomina nel nome
// di servizio, la verifica, la cancellazione, l'esito nella colonna esito_rimozione. Prove SC-24, SC-29, AF-24 di PM23
// sul conservatore delle prove (una cartella temporanea): mai il NAS vero. Nomi e contenuti sintetici (ACME).

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/storage/nas"
)

// copiaDiAllegato: la riga piu' recente della copia dell'occorrenza, con il file assoluto.
func (b *bancoFile) copiaDiAllegato(allegato uuid.UUID) (id int64, file string) {
	b.t.Helper()
	var cartella, rel string
	b.rigaCon(`SELECT conservazione_id, coalesce(cartella_thread, ''), percorso_relativo FROM file_rfq_conservazione
		WHERE allegato_id = $1 ORDER BY conservazione_id DESC LIMIT 1`, []any{allegato}, &id, &cartella, &rel)
	return id, b.cons.Percorso(cartella, rel)
}

// esitoRimozione: lo stato, l'esito della rimozione dal NAS e il token di una copia.
func (b *bancoFile) esitoRimozione(id int64) (stato, esito string, token *uuid.UUID) {
	b.t.Helper()
	b.rigaCon(`SELECT stato, coalesce(esito_rimozione, '-'), token_rimozione FROM file_rfq_conservazione WHERE conservazione_id = $1`,
		[]any{id}, &stato, &esito, &token)
	return stato, esito, token
}

// SC-24 (U2 §3.7): lo scarto di un file in Altri file toglie la copia dalla cartella ALTRO senza tenere una
// transazione aperta durante l'I/O sul NAS: mentre il conservatore lavora, la riga e' in_rimozione con il suo token e
// un'altra transazione la prende subito (NOWAIT). Alla fine: rilasciata / rimosso, file e nome di servizio tolti, il
// testo dice «eliminato» (solo con rimosso).
func TestSC24RimozioneFuoriTransazione(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-2400.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	id, file := b.copiaDiAllegato(a)
	visto := ""
	b.s.Agganci = &Agganci{PrimaDellaRimozioneNAS: func(x int64) {
		tx, err := b.p.Begin(b.ctx)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(b.ctx)
		var stato string
		var tok *uuid.UUID
		err = tx.QueryRow(b.ctx, `SELECT stato, token_rimozione FROM file_rfq_conservazione WHERE conservazione_id = $1 FOR UPDATE NOWAIT`,
			x).Scan(&stato, &tok)
		visto = fmt.Sprintf("%s/%v/%v", stato, tok != nil, err)
	}}
	e := b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	if visto != "in_rimozione/true/<nil>" {
		t.Errorf("durante la rimozione dal NAS la riga era %q (attesa in_rimozione, con il token, libera)", visto)
	}
	if stato, esito, tok := b.esitoRimozione(id); stato != "rilasciata" || esito != nas.MotivoRimosso || tok == nil {
		t.Errorf("copia %s/%s, token %v", stato, esito, tok)
	}
	if esiste(file) || len(b.cons.NomiDiServizio()) != 0 || len(b.fileDelDeposito()) != 0 {
		t.Errorf("rimasti: file %v, nomi di servizio %v, altri %v", esiste(file), b.cons.NomiDiServizio(), b.fileDelDeposito())
	}
	if !strings.Contains(e.Testo, "File eliminato anche dalla cartella ALTRO della RFQ.") {
		t.Errorf("testo %q", e.Testo)
	}
}

// SC-29 (U2 §3.7 (a), (b′)): il file aperto da un altro programma senza la condivisione in cancellazione: in attesa /
// file_occupato, la riga torna da_rilasciare con l'esito e il token; chiuso il file, il custode riprende con lo stesso
// token e toglie il file.
func TestSC29FileOccupatoPoiRipresa(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-2900.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	id, file := b.copiaDiAllegato(a)
	aperto, err := os.Open(file) // senza FILE_SHARE_DELETE su Windows
	if err != nil {
		t.Fatal(err)
	}
	chiuso := false
	defer func() {
		if !chiuso {
			aperto.Close()
		}
	}()
	e := b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	stato, esito, tok := b.esitoRimozione(id)
	if stato != "da_rilasciare" || esito != nas.MotivoFileOccupato || tok == nil {
		t.Fatalf("con il file aperto: %s/%s token %v", stato, esito, tok)
	}
	if strings.Contains(e.Testo, "cartella ALTRO della RFQ") || !strings.Contains(e.Testo, "aperta da un altro programma") {
		t.Errorf("testo %q", e.Testo)
	}
	if !esiste(file) || len(b.cons.NomiDiServizio()) != 0 {
		t.Errorf("il file non e' tornato al suo nome: %v %v", esiste(file), b.cons.NomiDiServizio())
	}
	aperto.Close()
	chiuso = true
	b.avanti(2 * time.Hour)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	stato2, esito2, tok2 := b.esitoRimozione(id)
	if stato2 != "rilasciata" || esito2 != nas.MotivoRimosso || tok2 == nil || *tok2 != *tok {
		t.Errorf("dopo il custode: %s/%s, token %v (prima %v)", stato2, esito2, tok2, tok)
	}
	if esiste(file) {
		t.Error("il file c'e' ancora")
	}
}

// AF-24 (U2 §3.5, §3.7, U2-P42): con la scrittura sul NAS spenta lo scarto si registra, la copia resta da_rilasciare
// con scrittura_nas_spenta e il custode non la riprende da solo, nemmeno dopo ore e con la scrittura riaccesa (come
// AllineaCoda): la riprende solo un gesto («Riprova la rimozione dalla cartella ALTRO»).
func TestAF24ScritturaSpentaNessunaRipresaDaSola(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-2401.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	id, file := b.copiaDiAllegato(a)
	b.cons.DryRun = true
	e := b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	if stato, esito, _ := b.esitoRimozione(id); stato != "da_rilasciare" || esito != nas.MotivoScritturaSpenta {
		t.Fatalf("con la scrittura spenta: %s/%s", stato, esito)
	}
	if !strings.Contains(e.Testo, "la scrittura sul NAS è spenta su questo server") || strings.Contains(e.Testo, "eliminato anche") {
		t.Errorf("testo %q", e.Testo)
	}
	b.cons.DryRun = false
	b.avanti(7 * time.Hour)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if stato, esito, _ := b.esitoRimozione(id); stato != "da_rilasciare" || esito != nas.MotivoScritturaSpenta || !esiste(file) {
		t.Errorf("il custode l'ha ripresa da solo: %s/%s, file %v", stato, esito, esiste(file))
	}
	// «Riprova la rimozione dalla cartella ALTRO» (U2 §3.5): spento con la scrittura spenta, acceso con la scrittura
	// accesa; lo stesso protocollo; dopo, niente da riprovare
	riprova := func() (*EsitoGesto, error) {
		return b.s.RiprovaRimozione(b.ctx, b.thread, a, b.u.UtenteID, b.corpo(GestoRiprovaRimozione, b.thread, a, ""))
	}
	b.cons.DryRun = true
	if f := fileDi(b.modello(b.thread, a), a); f == nil || f.Azioni.RiprovaRimozione.Disponibile ||
		f.Conservazione == nil || !strings.Contains(f.Conservazione.Motivo, "scrittura sul NAS è spenta") {
		t.Errorf("S1 con la scrittura spenta: %+v", f)
	}
	_, err := riprova()
	if rf := rifiutoAtteso(t, err, 503, "NON_CONFIGURATO"); rf.Extra["motivo"] != "nas_scrittura" {
		t.Errorf("motivo %v", rf.Extra["motivo"])
	}
	b.cons.DryRun = false
	if f := fileDi(b.modello(b.thread, a), a); f == nil || !f.Azioni.RiprovaRimozione.Disponibile {
		t.Errorf("S1 con la scrittura accesa: %+v", f)
	}
	e2 := b.deveRiuscire(riprova())
	if e2.Esito != "riprovata" || e2.Stato != StatoScartato || !strings.Contains(e2.Testo, "File eliminato anche dalla cartella ALTRO della RFQ.") {
		t.Errorf("esito %+v", e2)
	}
	if stato, esito, _ := b.esitoRimozione(id); stato != "rilasciata" || esito != nas.MotivoRimosso || esiste(file) {
		t.Errorf("dopo «Riprova»: %s/%s, file %v", stato, esito, esiste(file))
	}
	_, err = riprova()
	rifiutoAtteso(t, err, 422, "RIMOZIONE_NON_DISPONIBILE")
	if f := fileDi(b.modello(b.thread, a), a); f == nil || f.Azioni.RiprovaRimozione.Disponibile {
		t.Errorf("S1 dopo la rimozione: %+v", f)
	}
}

// U2 §3.6 regola 2: lo scarto non tocca mai un documento. Un documento confermato che nomina lo stesso percorso della
// copia (una riga scritta a mano: la riserva dei nomi non lo lascerebbe succedere) chiude la riga con
// percorso_di_un_documento, e il file resta.
func TestRimozioneSulPercorsoDiUnDocumento(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-2402.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	id, file := b.copiaDiAllegato(a)
	b.documentoAltro(`ALTRO\ACME-2402.pdf`, "1111111111111111111111111111111111111111111111111111111111111111")
	b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	if stato, esito, _ := b.esitoRimozione(id); stato != "rilasciata" || esito != nas.MotivoPercorsoDiUnDocumento {
		t.Errorf("copia %s/%s", stato, esito)
	}
	if !esiste(file) {
		t.Error("il file del documento e' stato tolto")
	}
}

// U2 §3.7, la ripresa: un'esecuzione interrotta dopo la rinomina (la riga in_rimozione, il file con il nome di
// servizio del suo token). Il custode, dopo 15 minuti, riprende dal passo (b) con il token della colonna e toglie il
// nome di servizio; prima dei 15 minuti non la tocca (un'altra esecuzione puo' essere ancora sul NAS).
func TestRipresaDiUnaRimozioneInterrotta(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-2403.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	id, file := b.copiaDiAllegato(a)
	tok := uuid.New()
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	b.esegui(`UPDATE file_rfq_conservazione SET stato = 'in_rimozione', token_rimozione = $2 WHERE conservazione_id = $1`, id, tok)
	nome := file[strings.LastIndexAny(file, `\/`)+1:]
	servizio := file[:len(file)-len(nome)] + nas.NomeServizio(nome, tok)
	if err := os.Rename(file, servizio); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if stato, _, _ := b.esitoRimozione(id); stato != "in_rimozione" || !esiste(servizio) {
		t.Fatalf("prima dei 15 minuti il custode l'ha toccata: %s, nome di servizio %v", stato, esiste(servizio))
	}
	b.avanti(16 * time.Minute)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if stato, esito, tok2 := b.esitoRimozione(id); stato != "rilasciata" || esito != nas.MotivoRimosso || tok2 == nil || *tok2 != tok {
		t.Errorf("dopo la ripresa: %s/%s token %v", stato, esito, tok2)
	}
	if esiste(servizio) || esiste(file) {
		t.Errorf("rimasti: nome di servizio %v, file %v", esiste(servizio), esiste(file))
	}
}
