package filerfq

// L1 — V1, ondata B, corsia file, secondo passaggio: le prove dei rilievi della revisione che non vogliono il database
// (SenzaPercorsi con gli spazi, i parziali del deposito senza Glob, S1 allineata a S5, richiede[] sempre presente, i
// parziali del ciclo dei file e la loro pulizia, gli esiti degli eventi senza percorsi). Nomi e percorsi sintetici.

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// TestSenzaPercorsiConGliSpazi (rilievo della revisione, T-15): i percorsi con gli spazi escono interi, il separatore
// «: » dei messaggi d'errore resta, un indirizzo web non e' un percorso.
func TestSenzaPercorsiConGliSpazi(t *testing.T) {
	casi := []struct{ in, atteso string }{
		{`open C:\Users\Mario Rossi\AppData\Local\Temp\x.pdf: Accesso negato`, `open ‹percorso›: Accesso negato`},
		{`copia su \\server-nas\PREVENTIVI DA FARE\ACME\RFQ 1\x.pdf non riuscita`, `copia su ‹percorso›`},
		{`remove D:\Dati Cockpit\staging\_contenuti\ab\ab.pdf: file in uso`, `remove ‹percorso›: file in uso`},
		{`lettura di /mnt/staging/_contenuti/ab/ab.pdf non riuscita`, `lettura di ‹percorso›`},
		{`due percorsi: C:\a b\c.pdf e D:/x y/z.pdf: fine`, `due percorsi: ‹percorso›: fine`},
		{`esteso \\?\C:\Dati Cockpit\x.pdf: no`, `esteso ‹percorso›: no`},
		{`"C:\Program Files\x.exe" non parte`, `"‹percorso›" non parte`},
		{`vedi https://example.com/a/b e ftp://host/x`, `vedi https://example.com/a/b e ftp://host/x`},
		{`file_C:\x non e' un percorso a inizio parola`, `file_C:\x non e' un percorso a inizio parola`},
		{`nessun percorso qui: 3/4 dei file`, `nessun percorso qui: 3/4 dei file`},
		{``, ``},
	}
	for _, c := range casi {
		if got := SenzaPercorsi(c.in); got != c.atteso {
			t.Errorf("SenzaPercorsi(%q) = %q, atteso %q", c.in, got, c.atteso)
		}
	}
	// nessun pezzo di percorso resta, qualunque sia la frase
	for _, in := range []string{
		`open C:\Users\Nome Cognome\x.pdf`, `\\nas\PREVENTIVI DA FARE\CLIENTE\x`, `D:\Dati Cockpit\staging`,
	} {
		out := SenzaPercorsi(in)
		if strings.Contains(out, `\`) || strings.Contains(out, "Cognome") || strings.Contains(out, "DA FARE") || strings.Contains(out, "Cockpit") {
			t.Errorf("resta un pezzo di percorso: %q → %q", in, out)
		}
	}
}

// TestEsitoDegliEventiSenzaPercorsi (PF-16, rilievo della revisione): l'esito di un evento passa da SenzaPercorsi in
// ogni testo, anche annidato; i numeri restano numeri.
func TestEsitoDegliEventiSenzaPercorsi(t *testing.T) {
	grezzo := json.RawMessage(`{"stato":"errore","conservazione_id":1234567890123,"errore":"remove C:\\Dati Cockpit\\x.pdf: in uso",` +
		`"altro":{"percorsi":["\\\\nas\\cond\\y z.pdf"]}}`)
	v := esitoSenzaPercorsi(grezzo)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, `\\`) || strings.Contains(s, "Cockpit") || !strings.Contains(s, "1234567890123") ||
		!strings.Contains(s, "‹percorso›: in uso") {
		t.Errorf("esito: %s", s)
	}
	if esitoSenzaPercorsi(json.RawMessage(`non json`)) != nil {
		t.Error("un esito che non si legge si mostra")
	}
}

// TestPartiDellaRigaSenzaGlob (rilievo della revisione): con una radice che contiene «[» e «]» i parziali di una riga
// sono solo quelli della sua cartella _parti, con il prefisso esatto «<id>.»; un file di una cartella sorella che
// combacerebbe con il pattern non esce mai.
func TestPartiDellaRigaSenzaGlob(t *testing.T) {
	base := t.TempDir()
	dep := filepath.Join(base, "dep[1]")
	sorella := filepath.Join(base, "dep1")
	for _, d := range []string{filepath.Join(dep, CartellaPartiDeposito), filepath.Join(sorella, CartellaPartiDeposito)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scrivi := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scrivi(filepath.Join(sorella, CartellaPartiDeposito, "5.abc"))
	mio := filepath.Join(dep, CartellaPartiDeposito, "5.def")
	scrivi(mio)
	scrivi(filepath.Join(dep, CartellaPartiDeposito, "50.abc"))
	scrivi(filepath.Join(dep, CartellaPartiDeposito, "x5.abc"))
	d := &Deposito{Radice: dep, ID: uuid.New()}
	got := d.partiDellaRiga(5)
	if len(got) != 1 || !strings.EqualFold(got[0], mio) {
		t.Errorf("partiDellaRiga(5) = %q, atteso solo %q", got, mio)
	}
	if id, ok := idDelParziale("12.a1b2"); !ok || id != 12 {
		t.Errorf("idDelParziale: %d %v", id, ok)
	}
	for _, n := range []string{".abc", "abc", "x.1"} {
		if _, ok := idDelParziale(n); ok {
			t.Errorf("idDelParziale(%q) riconosciuto", n)
		}
	}
}

// TestS1ApreSoloCioCheS5Serve (rilievi della revisione): byte.staging guarda il disco solo nel posto del contenuto; lo
// staging vecchio per messaggio e un percorso fuori dalla radice sono «non_verificato» e non accendono l'apertura; il
// posto del contenuto senza il file e' «assente»; con il file, «presente» e l'apertura c'e'.
func TestS1ApreSoloCioCheS5Serve(t *testing.T) {
	radice := t.TempDir()
	sha := strings.Repeat("ab", 32)
	vecchio := filepath.Join(radice, "a1b2c3d4", "ACME-1.pdf")
	if err := os.MkdirAll(filepath.Dir(vecchio), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vecchio, []byte("%PDF-1.4 sintetico"), 0o644); err != nil {
		t.Fatal(err)
	}
	fuori := filepath.Join(t.TempDir(), "ACME-2.pdf")
	if err := os.WriteFile(fuori, []byte("%PDF-1.4 sintetico 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	posto, err := staging.PercorsoContenuto(radice, sha, "ACME-3.pdf")
	if err != nil {
		t.Fatal(err)
	}
	s := &Servizio{Staging: radice}
	riga := func(p string, origine db.OrigineAllegato) db.ListFileSorgenteRow {
		return db.ListFileSorgenteRow{AllegatoID: uuid.New(), MessaggioID: uuid.New(), NomeFile: filepath.Base(p),
			PathStaging: pgtype.Text{String: p, Valid: true}, Sha256: pgtype.Text{String: sha, Valid: true}, Stato: StatoAttivo,
			StatoAllegato: db.StatoAllegatoAnalizzato, Natura: db.NaturaAllegatoFile, Origine: origine, Canale: db.CanaleOutlook}
	}
	for _, c := range []struct {
		nome, percorso, staging string
		disponibile             bool
		richiede                string
	}{
		{"staging vecchio per messaggio", vecchio, "non_verificato", false, "ricarica"},
		{"fuori dalla radice", fuori, "non_verificato", false, "ricarica"},
		{"un UNC rimasto", `\\server-vecchio\staging\ACME-4.pdf`, "non_verificato", false, "ricarica"},
		{"il posto del contenuto senza il file", posto, "assente", false, "ricarica"},
	} {
		f := s.fileDaRiga(riga(c.percorso, db.OrigineAllegatoPortale), nil, nil, false)
		if f.Byte.Staging != c.staging || f.Azioni.Contenuto.Disponibile != c.disponibile ||
			len(f.Azioni.Contenuto.Richiede) != 1 || f.Azioni.Contenuto.Richiede[0] != c.richiede {
			t.Errorf("%s: byte.staging %q, contenuto %+v", c.nome, f.Byte.Staging, f.Azioni.Contenuto)
		}
	}
	// con il file al suo posto: presente, apertura disponibile, richiede vuoto ma presente nel JSON
	if err := os.MkdirAll(filepath.Dir(posto), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(posto, []byte("%PDF-1.4 sintetico 3"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := s.fileDaRiga(riga(posto, db.OrigineAllegatoOutlook), nil, nil, false)
	if f.Byte.Staging != "presente" || !f.Azioni.Contenuto.Disponibile {
		t.Errorf("il posto del contenuto con il file: %q %+v", f.Byte.Staging, f.Azioni.Contenuto)
	}
	b, err := json.Marshal(f.Azioni)
	if err != nil {
		t.Fatal(err)
	}
	var az map[string]map[string]any
	if err := json.Unmarshal(b, &az); err != nil {
		t.Fatal(err)
	}
	if r, ok := az["contenuto"]["richiede"].([]any); !ok || len(r) != 0 {
		t.Errorf("azioni.contenuto senza richiede[]: %s", b)
	}
	for _, nome := range []string{"scarta", "altri_file", "riattiva"} {
		if _, c := az[nome]["richiede"]; c {
			t.Errorf("azioni.%s porta richiede (PE-06: {disponibile, motivo}): %s", nome, b)
		}
	}
	// un file in Altri file senza nessuna sorgente: l'apertura e' spenta, con il motivo dell'anomalia
	r := riga(fuori, db.OrigineAllegatoPortale)
	r.Stato = StatoAltriFile
	if f := s.fileDaRiga(r, nil, nil, false); f.Azioni.Contenuto.Disponibile || !strings.Contains(f.Azioni.Contenuto.Motivo, "anomalia") ||
		f.Azioni.Contenuto.Richiede == nil {
		t.Errorf("in Altri file senza byte: %+v", f.Azioni.Contenuto)
	}
}

// TestParzialiDelCicloDeiFile (rilievi della revisione): il parziale di E1′ non ha il nome dei parziali dei tentativi
// dei worker (la pulizia della coda non lo riconosce); il custode toglie solo quelli del ciclo dei file piu' vecchi di
// EtaPartiCiclo, e non tocca gli altri file di _parti.
func TestParzialiDelCicloDeiFile(t *testing.T) {
	radice := t.TempDir()
	f, err := ScriviParziale(radice, strings.NewReader("contenuto sintetico ACME"), "ACME-1.pdf", "application/pdf", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := staging.TokenDiParte(filepath.Base(f.Parte)); ok {
		t.Errorf("il parziale di E1′ %q ha il nome dei parziali dei tentativi", filepath.Base(f.Parte))
	}
	if !strings.HasPrefix(filepath.Base(f.Parte), PrefissoParteCiclo) {
		t.Errorf("il parziale di E1′ %q senza il prefisso del ciclo dei file", filepath.Base(f.Parte))
	}
	parti := filepath.Join(radice, staging.CartellaParti)
	vecchio := filepath.Join(parti, PrefissoParteCiclo+uuid.NewString())
	tentativo := filepath.Join(parti, uuid.NewString()+".parte."+uuid.NewString())
	for _, p := range []string{vecchio, tentativo} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		ieri := time.Now().Add(-25 * time.Hour)
		if err := os.Chtimes(p, ieri, ieri); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if n := PulisciPartiCiclo(radice, time.Now(), log); n != 1 {
		t.Errorf("parziali del ciclo tolti: %d, atteso 1", n)
	}
	if esiste(vecchio) || !esiste(f.Parte) || !esiste(tentativo) {
		t.Errorf("dopo la pulizia: vecchio %v, giovane %v, tentativo %v", esiste(vecchio), esiste(f.Parte), esiste(tentativo))
	}
	if n := PulisciPartiCiclo("", time.Now(), log); n != 0 {
		t.Errorf("senza staging: %d", n)
	}
}
