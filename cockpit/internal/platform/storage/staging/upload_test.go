package staging

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// L1 — blocco 4A: lo staging ha due cartelle, e i percorsi li decide il server.
//
// Prima il percorso definitivo di un allegato veniva composto con la sottocartella dichiarata nel
// payload e il nome del file: due dati che passano per il mondo esterno, e che quindi andavano
// entrambi controllati per non uscire dallo staging. Ora il nome di un contenuto E' il suo sha256,
// e il controllo si riduce a una domanda sola — «e' un hash?» — che non si puo' aggirare con un nome
// scelto bene.
func TestPercorsoContenuto(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	p, err := PercorsoContenuto(`C:\staging`, sha, "disegno.PDF")
	if err != nil {
		t.Fatal(err)
	}
	atteso := filepath.Join(`C:\staging`, CartellaContenuti, "01", sha+".pdf")
	if p != atteso {
		t.Errorf("percorso = %s, atteso %s", p, atteso)
	}
	// lo stesso contenuto con lo stesso nome sta in un posto solo: e' tutta la deduplica
	if q, _ := PercorsoContenuto(`C:\staging`, strings.ToUpper(sha), "DISEGNO.pdf"); q != atteso {
		t.Errorf("maiuscole e minuscole danno due file diversi: %s", q)
	}

	// niente che non sia un hash: e' il punto in cui un percorso non puo' uscire dallo staging
	for _, cattivo := range []string{"", "..", `..\altrove`, strings.Repeat("g", 64), sha[:63], sha + "0", sha + "/x"} {
		if _, err := PercorsoContenuto(`C:\staging`, cattivo, "x.pdf"); err == nil {
			t.Errorf("sha256 %q accettato", cattivo)
		}
	}
}

// L'estensione serve solo a chi guarda la cartella: il tipo lo decide il nome del file nel payload.
// Per questo qui dentro non deve poter comparire niente che non sia stato scelto da noi.
func TestEstensioneDiUnContenuto(t *testing.T) {
	casi := map[string]string{
		"disegno.pdf":             ".pdf",
		"DISEGNO.PDF":             ".pdf",
		"pezzo.STEP":              ".step",
		"senza estensione":        ".bin",
		"":                        ".bin",
		`cattivo.p\df`:            ".bin",
		"cattivo.nome-con-trace":  ".bin",
		"troppolunga.abcdefghijk": ".bin",
	}
	for nome, atteso := range casi {
		if got := estensioneDi(nome); got != atteso {
			t.Errorf("estensioneDi(%q) = %q, atteso %q", nome, got, atteso)
		}
	}
}

// Il token nel nome del .parte e' cio' che impedisce a un tentativo scaduto di consegnare il file al
// posto del tentativo che gli e' subentrato (M13): due tentativi devono scrivere due file diversi.
func TestPercorsoParte(t *testing.T) {
	a := uuid.New()
	tokA, tokB := uuid.New(), uuid.New()
	pa := PercorsoParte(`C:\staging`, a, tokA)
	pb := PercorsoParte(`C:\staging`, a, tokB)
	if pa == pb {
		t.Fatalf("due tentativi dello stesso allegato scrivono lo stesso file: %s", pa)
	}
	if filepath.Base(filepath.Dir(pa)) != CartellaParti {
		t.Errorf("il file di un trasferimento non sta in %s: %s", CartellaParti, pa)
	}
	got, ok := TokenDiParte(filepath.Base(pa))
	if !ok || got != tokA {
		t.Errorf("TokenDiParte(%s) = %v, %v", filepath.Base(pa), got, ok)
	}
	if _, ok := TokenDiParte("01_x.pdf.parte"); ok {
		t.Error("un .parte senza token (quello del NAS) non deve essere riconosciuto")
	}
}

// ContenutoGiaPresente ricalcola l'hash invece di fidarsi del nome. Un file corrotto sul disco
// porterebbe il nome giusto e il contenuto sbagliato, e finirebbe sul NAS al posto del disegno vero.
func TestContenutoGiaPresenteVerificaLHash(t *testing.T) {
	dir := t.TempDir()
	vero := []byte("il contenuto vero")
	somma := sha256.Sum256(vero)
	sha := hex.EncodeToString(somma[:])

	p, err := PercorsoContenuto(dir, sha, "x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if ContenutoGiaPresente(p, sha) {
		t.Error("un file che non c'e' risulta presente")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, vero, 0o644); err != nil {
		t.Fatal(err)
	}
	if !ContenutoGiaPresente(p, sha) {
		t.Error("il contenuto c'e' con l'hash giusto e non risulta presente")
	}
	// stesso nome, contenuto diverso: non e' quel contenuto, e va riscritto
	if err := os.WriteFile(p, []byte("qualcos'altro"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ContenutoGiaPresente(p, sha) {
		t.Error("un file con il nome giusto e il contenuto sbagliato e' stato accettato")
	}
}

// cache C1, prova 10 — EContenuto riconosce tutto cio' che PercorsoContenuto scrive per quell'hash, con qualunque nome
// d'origine, e nient'altro: lo stesso percorso con un altro hash, o un percorso che PercorsoContenuto non
// scriverebbe mai, non e' «il contenuto». E' su questa risposta che l'anteprima dice al browser `immutable`.
func TestEContenutoRiconosceSoloIlPostoDelContenuto(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const altro = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	radice := filepath.Join(t.TempDir(), "staging")
	for _, nome := range []string{"disegno.PDF", "7120001A_1.stp", "senza estensione", "strano.e$t", "archivio.tar.gz"} {
		p, err := PercorsoContenuto(radice, sha, nome)
		if err != nil {
			t.Fatal(err)
		}
		if !EContenuto(radice, p, sha) {
			t.Errorf("%s: il percorso che PercorsoContenuto scrive (%s) non e' riconosciuto", nome, p)
		}
		if !EContenuto(radice, p, strings.ToUpper(sha)) {
			t.Errorf("%s: l'hash in maiuscolo e' lo stesso hash", nome)
		}
		if EContenuto(radice, p, altro) {
			t.Errorf("%s: riconosciuto come il contenuto di un altro hash", nome)
		}
	}
	p, _ := PercorsoContenuto(radice, sha, "disegno.pdf")
	for _, c := range []struct{ nome, radice, percorso, sha string }{
		{"hash vuoto", radice, p, ""},
		{"non un hash", radice, p, "disegno"},
		{"radice vuota", "", p, sha},
		{"un'altra radice", filepath.Join(t.TempDir(), "altro"), p, sha},
		{"la radice stessa", radice, radice, sha},
		{"una parte in caricamento", radice, PercorsoParte(radice, uuid.New(), uuid.New()), sha},
		{"la sottocartella di un altro hash", radice, filepath.Join(radice, CartellaContenuti, altro[:2], sha+".pdf"), sha},
		{"lo staging vecchio per messaggio", radice, filepath.Join(radice, CartellaStaging("<MSG-1@acme.example>"), sha+".pdf"), sha},
		{"risalendo fuori dalla radice", radice, filepath.Join(radice, CartellaContenuti) + `\..\..\altro\` + CartellaContenuti + `\` + sha[:2] + `\` + sha + ".pdf", sha},
		// correzione 1 della cache C1: i due casi in cui a dire no e' un solo confronto. La sottocartella giusta
		// con il nome di un altro hash che comincia allo stesso modo (solo il nome = hash); un'altra radice lunga
		// quanto questa, da tutte e due le parti (solo il confronto della radice).
		{"la sottocartella giusta, il nome di un altro hash con lo stesso inizio", radice,
			filepath.Join(radice, CartellaContenuti, sha[:2], sha[:2]+altro[2:]+".pdf"), sha},
		{"un'altra radice lunga quanto questa, dichiarata", filepath.Join(filepath.Dir(radice), "stagin2"), p, sha},
		{"un'altra radice lunga quanto questa, nel percorso", radice,
			filepath.Join(filepath.Dir(radice), "stagin2", CartellaContenuti, sha[:2], sha+".pdf"), sha},
	} {
		if EContenuto(c.radice, c.percorso, c.sha) {
			t.Errorf("%s: %s riconosciuto come il contenuto", c.nome, c.percorso)
		}
	}
}
