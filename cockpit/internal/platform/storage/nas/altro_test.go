package nas

// L1 — V1, ondata C, corsia file (U2 §3.2, §3.3, §3.5, §3.7, §3.9, §3.12): le copie del cockpit in una cartella che e'
// anche delle persone. Cartelle temporanee, nomi e contenuti sintetici (ACME): mai il NAS vero.
//   - la copia: parziale esclusivo, verifica dello sha, promozione senza sostituzione; un file sul nome (prima o al
//     momento della promozione) e' un conflitto e resta com'e'; nessun ripiego sulla copia diretta; DryRun non scrive;
//   - la lettura: lo sha verificato dallo stesso handle, ErrCopiaMancante e ErrCopiaCambiata, mai byte diversi;
//   - la rimozione con la rinomina: rimosso, file cambiato, gia' assente, scrittura spenta, nome ripreso, ripresa;
//   - i parziali: tolti solo se scaduti e non vivi.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func shaDi(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// bancoAltro: una radice temporanea con la cartella di una RFQ sintetica, e il posto di una copia in ALTRO.
func bancoAltro(t *testing.T, contenuto []byte) (*Conservatore, Posto, string) {
	t.Helper()
	radice := t.TempDir()
	cartella := `ACME\WIP\RFQ prova`
	if err := os.MkdirAll(filepath.Join(radice, cartella), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Conservatore{Radice: radice}
	p := Posto{RadiceID: c.RadiceID(), Cartella: cartella, Percorso: `ALTRO\offerta.pdf`, Sha256: shaDi(contenuto),
		Bytes: int64(len(contenuto))}
	return c, p, filepath.Join(radice, cartella, "ALTRO", "offerta.pdf")
}

// vociDi: i nomi nella cartella (anche i nascosti).
func vociDi(t *testing.T, dir string) []string {
	t.Helper()
	v, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range v {
		out = append(out, e.Name())
	}
	return out
}

// TestAltroScritturaLettaAlMomento (U2 §3.5): la capacita' nas_scrittura si legge a ogni gesto, non solo all'avvio:
// spenta, niente si scrive ne' si toglie e la risposta lo dice (nas_scrittura), mai un falso successo.
func TestAltroScritturaLettaAlMomento(t *testing.T) {
	dati := []byte("%PDF-1.4 ACME capacita'")
	c, p, dst := bancoAltro(t, dati)
	accesa := false
	c.Scrittura = func() bool { return accesa }
	if ok, motivo := c.Disponibile(); ok || motivo != "nas_scrittura" {
		t.Errorf("con la capacita' spenta: %v %q", ok, motivo)
	}
	if e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); !errors.Is(err, ErrScritturaSpenta) || e.Creato {
		t.Errorf("copia con la capacita' spenta: %+v %v", e, err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("il definitivo c'e': %v", err)
	}
	accesa = true
	if e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); err != nil || !e.Creato {
		t.Fatalf("copia con la capacita' accesa: %+v %v", e, err)
	}
	accesa = false
	if e := c.Rimuovi(p, uuid.New(), false); e.Esito != RimozioneInAttesa || e.Motivo != MotivoScritturaSpenta {
		t.Errorf("rimozione con la capacita' spenta: %+v", e)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("il file non c'e' piu': %v", err)
	}
	if s := c.Cartelle(); !s.DryRun {
		t.Error("lo scrittore delle cartelle scrive con la capacita' spenta")
	}
}

// TestAltroParzialeDelloStessoToken (U2 §3.9): il parziale e' <definitivo>.parte.<token>, con il token della riga; se
// c'e' gia' (un altro tentativo con lo stesso token, o un processo morto), la copia non lo tocca e non si fa.
func TestAltroParzialeDelloStessoToken(t *testing.T) {
	dati := []byte("%PDF-1.4 ACME parziale")
	c, p, dst := bancoAltro(t, dati)
	tok := uuid.New()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	parte := dst + ".parte." + tok.String()
	if err := os.WriteFile(parte, []byte("di un altro tentativo"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := c.Copia(p, tok, bytes.NewReader(dati))
	if !errors.Is(err, ErrParzialePresente) || e.Causa != "parziale_presente" || e.Creato {
		t.Fatalf("copia sul parziale di un altro tentativo: %+v %v", e, err)
	}
	if got, _ := os.ReadFile(parte); string(got) != "di un altro tentativo" {
		t.Errorf("il parziale dell'altro tentativo e' stato toccato: %q", got)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("il definitivo c'e': %v", err)
	}
}

func TestAltroCopiaCreaSenzaParziali(t *testing.T) {
	dati := []byte("%PDF-1.4 ACME offerta")
	c, p, dst := bancoAltro(t, dati)
	e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
	if err != nil || !e.Creato || e.Bytes != int64(len(dati)) {
		t.Fatalf("copia: %+v %v", e, err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, dati) {
		t.Fatalf("il definitivo non ha i byte: %q", got)
	}
	if v := vociDi(t, filepath.Dir(dst)); len(v) != 1 {
		t.Errorf("nella cartella resta altro oltre al definitivo: %v", v)
	}
}

func TestAltroCopiaNonSostituisceMai(t *testing.T) {
	dati := []byte("ACME byte del cockpit")
	t.Run("file di una persona gia' sul nome", func(t *testing.T) {
		c, p, dst := bancoAltro(t, dati)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := os.WriteFile(dst, []byte("di una persona"), 0o644); err != nil {
			t.Fatal(err)
		}
		e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
		if !errors.Is(err, ErrNomeOccupato) || e.Creato || e.Causa != "conflitto" || e.GiaPresenteStessoSha {
			t.Fatalf("esito %+v %v", e, err)
		}
		if got, _ := os.ReadFile(dst); string(got) != "di una persona" {
			t.Errorf("il file della persona e' cambiato: %q", got)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 1 {
			t.Errorf("parziali rimasti: %v", v)
		}
	})
	t.Run("stesso contenuto gia' sul nome: non e' una prova di creazione", func(t *testing.T) {
		c, p, dst := bancoAltro(t, dati)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, dati, 0o644)
		e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
		if !errors.Is(err, ErrNomeOccupato) || e.Creato || !e.GiaPresenteStessoSha {
			t.Fatalf("esito %+v %v", e, err)
		}
	})
	t.Run("file comparso sul nome al momento della promozione", func(t *testing.T) {
		c, p, dst := bancoAltro(t, dati)
		c.Promozione = func(parte, d string) error {
			if err := os.WriteFile(d, []byte("arrivato adesso"), 0o644); err != nil {
				return err
			}
			return rinominaSenzaSostituire(parte, d)
		}
		e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
		if !errors.Is(err, ErrNomeOccupato) || e.Creato || e.Causa != "conflitto" {
			t.Fatalf("esito %+v %v", e, err)
		}
		if got, _ := os.ReadFile(dst); string(got) != "arrivato adesso" {
			t.Errorf("la promozione ha sostituito il file comparso: %q", got)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 1 {
			t.Errorf("parziali rimasti: %v", v)
		}
	})
	t.Run("nessun ripiego senza rinomina esclusiva", func(t *testing.T) {
		c, p, dst := bancoAltro(t, dati)
		c.Promozione = func(string, string) error { return ErrPromozioneNonDisponibile }
		e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
		if !errors.Is(err, ErrPromozioneNonDisponibile) || e.Creato {
			t.Fatalf("esito %+v %v", e, err)
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("senza promozione esclusiva il definitivo c'e' lo stesso: %v", err)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 0 {
			t.Errorf("parziali rimasti: %v", v)
		}
	})
}

func TestAltroCopiaRifiuti(t *testing.T) {
	dati := []byte("ACME")
	t.Run("DryRun: niente scritto", func(t *testing.T) {
		c, p, dst := bancoAltro(t, dati)
		c.DryRun = true
		if ok, motivo := c.Disponibile(); ok || motivo != "nas_scrittura" {
			t.Errorf("Disponibile con DryRun: %v %q", ok, motivo)
		}
		e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
		if !errors.Is(err, ErrScritturaSpenta) || e.Creato || e.Causa != "nas_scrittura" {
			t.Fatalf("esito %+v %v", e, err)
		}
		if _, err := os.Stat(filepath.Dir(dst)); !os.IsNotExist(err) {
			t.Errorf("con DryRun la cartella ALTRO e' stata creata: %v", err)
		}
	})
	t.Run("sha diverso", func(t *testing.T) {
		c, p, dst := bancoAltro(t, dati)
		e, err := c.Copia(p, uuid.New(), strings.NewReader("altro contenuto"))
		if err == nil || e.Causa != "verifica_sha" || e.Creato {
			t.Fatalf("esito %+v %v", e, err)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 0 {
			t.Errorf("file rimasti: %v", v)
		}
	})
	t.Run("sorgente illeggibile", func(t *testing.T) {
		c, p, _ := bancoAltro(t, dati)
		e, err := c.Copia(p, uuid.New(), io.MultiReader(strings.NewReader("AC"), lettoreRotto{}))
		if err == nil || e.Causa != "sorgente_illeggibile" {
			t.Fatalf("esito %+v %v", e, err)
		}
	})
	t.Run("cartella della RFQ assente", func(t *testing.T) {
		c, p, _ := bancoAltro(t, dati)
		p.Cartella = `ACME\WIP\non esiste`
		e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati))
		if err == nil || e.Causa != "cartella_assente" {
			t.Fatalf("esito %+v %v", e, err)
		}
		if _, err := os.Stat(filepath.Join(c.Radice, p.Cartella)); !os.IsNotExist(err) {
			t.Errorf("la copia ha creato la cartella della RFQ: %v", err)
		}
	})
	t.Run("radice diversa", func(t *testing.T) {
		c, p, _ := bancoAltro(t, dati)
		p.RadiceID = uuid.New()
		if _, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); !errors.Is(err, ErrRadiceDiversa) {
			t.Fatalf("radice diversa: %v", err)
		}
	})
	t.Run("percorsi che escono", func(t *testing.T) {
		c, p, _ := bancoAltro(t, dati)
		for _, cattivo := range []string{`..\fuori.pdf`, `ALTRO\..\..\fuori.pdf`, `C:\fuori.pdf`, `\fuori.pdf`, `ALTRO\a.pdf:flusso`, ``, `ALTRO\.`, `ALTRO\ ..`} {
			q := p
			q.Percorso = cattivo
			if _, err := c.Copia(q, uuid.New(), bytes.NewReader(dati)); !errors.Is(err, ErrPercorsoNonValido) {
				t.Errorf("%q: %v", cattivo, err)
			}
		}
		q := p
		q.Cartella = `ACME\..\..`
		if _, err := c.Copia(q, uuid.New(), bytes.NewReader(dati)); !errors.Is(err, ErrPercorsoNonValido) {
			t.Errorf("cartella che esce: %v", err)
		}
		q = p
		q.Percorso = `ALTRO\sotto\due\livelli.pdf`
		if _, err := c.Copia(q, uuid.New(), bytes.NewReader(dati)); !errors.Is(err, ErrPercorsoNonValido) {
			t.Errorf("piu' di un livello sotto la cartella: %v", err)
		}
	})
}

type lettoreRotto struct{}

func (lettoreRotto) Read([]byte) (int, error) { return 0, errors.New("lettura rotta") }

func TestAltroApriEStato(t *testing.T) {
	dati := []byte("ACME byte da servire")
	c, p, dst := bancoAltro(t, dati)
	if s, _ := c.Stato(p, true); s != CopiaNonTrovata {
		t.Errorf("prima della copia: %s", s)
	}
	if _, _, err := c.Apri(p); !errors.Is(err, ErrCopiaMancante) {
		t.Errorf("Apri senza copia: %v", err)
	}
	if _, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); err != nil {
		t.Fatal(err)
	}
	f, st, err := c.Apri(p)
	if err != nil || st.Size() != int64(len(dati)) {
		t.Fatalf("Apri: %v", err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if !bytes.Equal(got, dati) {
		t.Errorf("Apri serve altri byte: %q", got)
	}
	if s, _ := c.Stato(p, true); s != CopiaPresente {
		t.Errorf("stato: %s", s)
	}
	q := p
	q.RadiceID = uuid.New()
	if s, _ := c.Stato(q, true); s != CopiaEstranea {
		t.Errorf("radice diversa: %s", s)
	}
	// cambiata con la stessa dimensione: solo lo sha lo vede
	stessa := bytes.ToUpper(dati)
	if err := os.WriteFile(dst, stessa, 0o644); err != nil {
		t.Fatal(err)
	}
	if s, _ := c.Stato(p, false); s != CopiaPresente {
		t.Errorf("il giro veloce guarda solo la dimensione: %s", s)
	}
	if s, _ := c.Stato(p, true); s != CopiaCambiata {
		t.Errorf("con lo sha: %s", s)
	}
	if _, _, err := c.Apri(p); !errors.Is(err, ErrCopiaCambiata) {
		t.Errorf("Apri di una copia cambiata: %v", err)
	}
	_ = os.WriteFile(dst, []byte("piu' corto"), 0o644)
	if s, _ := c.Stato(p, false); s != CopiaCambiata {
		t.Errorf("dimensione diversa: %s", s)
	}
	_ = os.Remove(dst)
	if s, _ := c.Stato(p, false); s != CopiaNonTrovata {
		t.Errorf("tolta: %s", s)
	}
}

func TestAltroRimuovi(t *testing.T) {
	dati := []byte("ACME copia del cockpit")
	copiata := func(t *testing.T) (*Conservatore, Posto, string) {
		c, p, dst := bancoAltro(t, dati)
		if _, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); err != nil {
			t.Fatal(err)
		}
		return c, p, dst
	}
	t.Run("rimosso", func(t *testing.T) {
		c, p, dst := copiata(t)
		e := c.Rimuovi(p, uuid.New(), false)
		if e.Esito != RimozioneEseguita || e.Motivo != MotivoRimosso {
			t.Fatalf("esito %+v", e)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 0 {
			t.Errorf("resta qualcosa: %v", v)
		}
	})
	t.Run("cambiato da una persona: torna al suo nome", func(t *testing.T) {
		c, p, dst := copiata(t)
		_ = os.WriteFile(dst, []byte("rivisto a mano"), 0o644)
		e := c.Rimuovi(p, uuid.New(), false)
		if e.Esito != RimozioneNonNecessaria || e.Motivo != MotivoFileCambiato {
			t.Fatalf("esito %+v", e)
		}
		if got, _ := os.ReadFile(dst); string(got) != "rivisto a mano" {
			t.Errorf("il file della persona: %q", got)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 1 {
			t.Errorf("nomi di servizio rimasti: %v", v)
		}
	})
	t.Run("gia' assente", func(t *testing.T) {
		c, p, dst := copiata(t)
		_ = os.Remove(dst)
		if e := c.Rimuovi(p, uuid.New(), false); e.Esito != RimozioneEseguita || e.Motivo != MotivoGiaAssente {
			t.Fatalf("esito %+v", e)
		}
		p.Cartella = `ACME\WIP\spostata`
		if e := c.Rimuovi(p, uuid.New(), false); e.Esito != RimozioneEseguita || e.Motivo != MotivoGiaAssente {
			t.Fatalf("cartella spostata: %+v", e)
		}
	})
	t.Run("scrittura spenta: niente tolto", func(t *testing.T) {
		c, p, dst := copiata(t)
		c.DryRun = true
		if e := c.Rimuovi(p, uuid.New(), false); e.Esito != RimozioneInAttesa || e.Motivo != MotivoScritturaSpenta || !e.Ritentabile() {
			t.Fatalf("esito %+v", e)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Errorf("con la scrittura spenta il file e' sparito: %v", err)
		}
	})
	t.Run("radice e percorso", func(t *testing.T) {
		c, p, dst := copiata(t)
		q := p
		q.RadiceID = uuid.New()
		if e := c.Rimuovi(q, uuid.New(), false); e.Esito != RimozioneErrore || e.Motivo != MotivoRadiceCambiata {
			t.Errorf("radice: %+v", e)
		}
		q = p
		q.Percorso = `..\..\fuori.pdf`
		if e := c.Rimuovi(q, uuid.New(), false); e.Esito != RimozioneErrore || e.Motivo != MotivoFuoriCartella {
			t.Errorf("fuori: %+v", e)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Errorf("il file e' sparito: %v", err)
		}
	})
	t.Run("ripresa con il nome di servizio presente", func(t *testing.T) {
		c, p, dst := copiata(t)
		tok := uuid.New()
		servizio := filepath.Join(filepath.Dir(dst), NomeServizio("offerta.pdf", tok))
		if err := os.Rename(dst, servizio); err != nil { // la prova simula il passo (a) di un'esecuzione interrotta
			t.Fatal(err)
		}
		if e := c.Rimuovi(p, tok, true); e.Esito != RimozioneEseguita || e.Motivo != MotivoRimosso {
			t.Fatalf("esito %+v", e)
		}
		if v := vociDi(t, filepath.Dir(dst)); len(v) != 0 {
			t.Errorf("resta qualcosa: %v", v)
		}
	})
	t.Run("ripresa senza nome di servizio", func(t *testing.T) {
		c, p, dst := copiata(t)
		_ = os.WriteFile(dst, []byte("di una persona adesso"), 0o644)
		if e := c.Rimuovi(p, uuid.New(), true); e.Esito != RimozioneNonNecessaria || e.Motivo != MotivoFileCambiato {
			t.Fatalf("esito %+v", e)
		}
		if got, _ := os.ReadFile(dst); string(got) != "di una persona adesso" {
			t.Errorf("toccato: %q", got)
		}
		_ = os.Remove(dst)
		if e := c.Rimuovi(p, uuid.New(), true); e.Esito != RimozioneEseguita || e.Motivo != MotivoGiaAssente {
			t.Fatalf("assente: %+v", e)
		}
	})
	t.Run("nome ripreso", func(t *testing.T) {
		c, p, dst := copiata(t)
		tok := uuid.New()
		servizio := filepath.Join(filepath.Dir(dst), NomeServizio("offerta.pdf", tok))
		// il nome di servizio con un contenuto cambiato, e sul nome originale un file nuovo di una persona
		if err := os.WriteFile(servizio, []byte("cambiato prima della rinomina"), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(dst, []byte("nuovo file della persona"), 0o644)
		e := c.Rimuovi(p, tok, true)
		if e.Esito != RimozioneErrore || e.Motivo != MotivoNomeRipreso || !strings.HasSuffix(e.NomeServizio, NomeServizio("offerta.pdf", tok)) {
			t.Fatalf("esito %+v", e)
		}
		if got, _ := os.ReadFile(dst); string(got) != "nuovo file della persona" {
			t.Errorf("il file nuovo: %q", got)
		}
		if got, _ := os.ReadFile(servizio); string(got) != "cambiato prima della rinomina" {
			t.Errorf("il file col nome di servizio: %q", got)
		}
	})
}

func TestAltroPulisciParti(t *testing.T) {
	dati := []byte("ACME")
	c, p, dst := bancoAltro(t, dati)
	dir := filepath.Dir(dst)
	_ = os.MkdirAll(dir, 0o755)
	vivo, morto, giovane := uuid.New(), uuid.New(), uuid.New()
	vecchio := time.Now().Add(-48 * time.Hour)
	for _, tok := range []uuid.UUID{vivo, morto, giovane} {
		f := dst + suffissoParte + tok.String()
		_ = os.WriteFile(f, dati, 0o644)
		if tok != giovane {
			_ = os.Chtimes(f, vecchio, vecchio)
		}
	}
	servizio := filepath.Join(dir, NomeServizio("offerta.pdf", morto))
	_ = os.WriteFile(servizio, dati, 0o644)
	_ = os.Chtimes(servizio, vecchio, vecchio)
	n, err := c.PulisciPartiCopie(p, map[uuid.UUID]bool{vivo: true}, time.Now().Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("tolti %d: %v", n, err)
	}
	if _, err := os.Stat(dst + suffissoParte + morto.String()); !os.IsNotExist(err) {
		t.Error("il parziale scaduto e non vivo resta")
	}
	for _, f := range []string{dst + suffissoParte + vivo.String(), dst + suffissoParte + giovane.String(), servizio} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("tolto %s: %v", filepath.Base(f), err)
		}
	}
	c.DryRun = true
	if n, _ := c.PulisciPartiCopie(p, nil, time.Now()); n != 0 {
		t.Errorf("con DryRun tolti %d", n)
	}
}

func TestImprontaRadiceNormalizzata(t *testing.T) {
	if ImprontaRadice(`\\nas\Preventivi\`) != ImprontaRadice(`//NAS/preventivi`) {
		t.Error("la stessa radice scritta in modo diverso ha due impronte")
	}
	if ImprontaRadice(`\\nas\a`) == ImprontaRadice(`\\nas\b`) {
		t.Error("due radici diverse hanno la stessa impronta")
	}
}
