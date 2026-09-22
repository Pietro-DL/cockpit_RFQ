// L1 — dove si apre il file di un documento, e dove NON si apre (blocco 8, B8.1).
//
// Nessuno di questi percorsi puo' arrivare da una richiesta HTTP: l'anteprima riceve
// l'identificativo di un allegato e basta. Quindi non si prova qui un attacco, si prova un BUG —
// una migrazione sbagliata, una riga cambiata a mano, un `path_relativo` composto male da codice
// futuro. E' il controllo che non serve mai, tranne la volta in cui serve.
package documenti

import (
	"strings"
	"testing"
)

const radiceProva = `C:\nas`
const cartellaRFQ = `ACME\WIP\2026 09 22 Rossi Supporto cofano`

func TestIlPercorsoSiComponeDaiTrePezzi(t *testing.T) {
	p, err := PercorsoSulNas(radiceProva, cartellaRFQ, `ELENCO DISEGNI\0.000.0000.0\disegno.pdf`)
	if err != nil {
		t.Fatal(err)
	}
	vogliamo := cartellaRFQ + `\ELENCO DISEGNI\0.000.0000.0\disegno.pdf`
	if p.Relativo != vogliamo {
		t.Errorf("relativo = %q, atteso %q", p.Relativo, vogliamo)
	}
	if p.Assoluto != radiceProva+`\`+vogliamo {
		t.Errorf("assoluto = %q", p.Assoluto)
	}
	// La radice con la barra finale e' la stessa radice: in configurazione la scrivono in tutti e due
	// i modi, e un percorso con due barre in mezzo non si apre.
	q, err := PercorsoSulNas(radiceProva+`\`, cartellaRFQ, `disegno.pdf`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.Assoluto, `\\`) {
		t.Errorf("la barra finale della radice e' finita nel percorso: %q", q.Assoluto)
	}
}

func TestUnPercorsoCheEsceDallaRadiceNonSiApre(t *testing.T) {
	casi := []struct {
		nome     string
		cartella string
		path     string
	}{
		{"risalita nel percorso del documento", cartellaRFQ, `..\..\..\Windows\win.ini`},
		{"risalita nella cartella della RFQ", `ACME\..\..\..\Windows`, `win.ini`},
		{"percorso assoluto con unita'", cartellaRFQ, `C:\Windows\win.ini`},
		{"dalla radice del disco", cartellaRFQ, `\Windows\win.ini`},
		{"dalla radice, con la barra dall'altra parte", cartellaRFQ, `/Windows/win.ini`},
		{"flusso alternativo NTFS", cartellaRFQ, `disegno.pdf:nascosto`},
		{"condivisione di rete di qualcun altro", `\\altro-server\share`, `disegno.pdf`},
		{"niente", "", ""},
		{"solo spazi", "  ", "  "},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			p, err := PercorsoSulNas(radiceProva, c.cartella, c.path)
			if err == nil {
				t.Fatalf("si aprirebbe %q: un percorso fuori dalla radice del NAS e' un errore del server, non un caso d'uso", p.Assoluto)
			}
			if p.Assoluto != "" || p.Relativo != "" {
				t.Errorf("l'errore porta comunque un percorso con se': %+v", p)
			}
		})
	}
}

// Un file puo' chiamarsi `a..b.pdf`, e rifiutarlo vorrebbe dire non mostrare un disegno che c'e'.
// `..` si cerca come PEZZO del percorso, non come sottostringa.
func TestUnNomeConDuePuntiNonEUnaRisalita(t *testing.T) {
	p, err := PercorsoSulNas(radiceProva, cartellaRFQ, `ELENCO DISEGNI\rev..vecchia.pdf`)
	if err != nil {
		t.Fatalf("un nome di file legittimo e' stato scambiato per una risalita: %v", err)
	}
	if !strings.HasSuffix(p.Assoluto, `rev..vecchia.pdf`) {
		t.Errorf("assoluto = %q", p.Assoluto)
	}
}

// La radice stessa non e' un documento: `Join` di una radice con «.» torna la radice, e un `os.Open`
// su una cartella riesce.
func TestLaRadiceStessaNonEUnDocumento(t *testing.T) {
	for _, path := range []string{".", `.\`, `ELENCO\..`} {
		if p, err := PercorsoSulNas(radiceProva, "", path); err == nil {
			t.Errorf("con path %q si aprirebbe %q", path, p.Assoluto)
		}
	}
}

// Il prefisso long-path si aggiunge DOPO il controllo: aggiunto prima, il confronto fra il percorso e
// la radice avverrebbe fra due stringhe di forma diversa e non direbbe piu' niente.
func TestIlPrefissoLongPathArrivaDopoIlControllo(t *testing.T) {
	lungo := strings.Repeat("sottoassieme lunghissimo ", 12)
	p, err := PercorsoSulNas(radiceProva, cartellaRFQ+`\`+lungo, `disegno.pdf`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Assoluto, `\\?\`) {
		t.Errorf("un percorso di %d caratteri non ha il prefisso long-path: %q", len(p.Assoluto), p.Assoluto)
	}
	if strings.HasPrefix(p.Relativo, `\\?\`) {
		t.Errorf("il prefisso e' finito nel percorso relativo, che si scrive nelle anomalie: %q", p.Relativo)
	}
}

// dentroLaRadice confronta senza distinguere maiuscole e minuscole, perche' NTFS non le distingue: un
// confronto sensibile direbbe che `C:\NAS\...` non sta dentro `C:\nas`, e sul disco invece ci sta.
func TestIlConfrontoConLaRadiceNonGuardaLeMaiuscole(t *testing.T) {
	if !dentroLaRadice(`C:\NAS\ACME\disegno.pdf`, `C:\nas`) {
		t.Error("un percorso dentro la radice e' stato dichiarato fuori per via delle maiuscole")
	}
	if dentroLaRadice(`C:\nasaltro\disegno.pdf`, `C:\nas`) {
		t.Error("«C:\\nasaltro» non sta dentro «C:\\nas»: il confronto guarda il prefisso senza il separatore")
	}
	if dentroLaRadice(`C:\nas`, `C:\nas`) {
		t.Error("la radice stessa e' stata accettata come documento")
	}
}
