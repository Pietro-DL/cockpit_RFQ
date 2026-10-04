// L1 — il nome del file e la voce d'archivio (A1b-13; piano A, 5.4.5, «Mappatura nome file e voce
// d'archivio»; C-33; R19 d): la regola dello stem dichiarata, gli intervalli in byte sul nome com'è nel DB, il
// percorso della voce con le cartelle, NFC e NFD, emoji, spazi ai bordi, nessuna estensione, punto iniziale,
// più punti, maiuscole, un nome di 300 rune, un archivio .7z; le diagnostiche nome.* e archivio.*.
//
// La tabella è F-NOME (5.7.2): nomi inventati nella forma di quelli veri (ACME-030PB07XX0001, 9999999A_2,
// Q+700.099999.010, «IN_LAVORO»). Il repository è pubblico.
package estrazione

import (
	"strings"
	"testing"
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// codiciDelNome: le diagnostiche del nome e dell'archivio, le sole che questa prova guarda (quelle del
// contenuto dipendono dalle mappature dei fatti).
func codiciDelNome(d evidenze.DocumentoEvidenze) []string {
	var out []string
	for _, c := range codiciDi(d) {
		if strings.HasPrefix(c, "nome.") || strings.HasPrefix(c, "archivio.") {
			out = append(out, c)
		}
	}
	return out
}

// parti: stem ed estensione di un localizzatore del nome, come testo del nome. "-" vuol dire «assente».
func parti(testo string, p *evidenze.PosNomeFile) (stem, est string) {
	stem, est = "-", "-"
	if p.Stem != nil {
		stem = testo[p.Stem.Inizio:p.Stem.Fine]
	}
	if p.Estensione != nil {
		est = testo[p.Estensione.Inizio:p.Estensione.Fine]
	}
	return stem, est
}

// TestIlNomeDelFile (A1b-13).
func TestIlNomeDelFile(t *testing.T) {
	nfc := "Disegno \u00e8 ACME1111.pdf"                     // «è» composta, 2 byte
	nfd := "Disegno e\u0300 ACME1111.pdf"                    // «e» più l'accento combinante, 3 byte
	lungo := "ACME" + strings.Repeat("\u00e8", 292) + ".pdf" // 300 rune, 592 byte
	casi := []struct {
		nome       string
		estensione *string // allegato.estensione come la scrive l'acquisizione; nil = NULL
		percorso   string  // per una voce d'archivio
		stem, est  string  // attesi sul nome; "-" = assente
		pStem      string  // stem atteso sul percorso, per una voce
		codici     []string
	}{
		{"ACME-030PB07XX0001.pdf", ptr("pdf"), "", "ACME-030PB07XX0001", "pdf", "", nil},
		{"ACME-030PB07XX0001 00 IN_LAVORO.stp", ptr("stp"), "", "ACME-030PB07XX0001 00 IN_LAVORO", "stp", "", nil},
		{"9999999A_2.STP", ptr("stp"), "", "9999999A_2", "STP", "", nil},
		{"9999999X_1.IGS", ptr("igs"), "", "9999999X_1", "IGS", "", nil},
		// La voce con la cartella: il nome è l'ultimo pezzo, il percorso intero è un'unità sua (R19 d).
		{"9999999X_1.IGS", ptr("igs"), "9999999A1/9999999X_1.IGS", "9999999X_1", "IGS", "9999999A1/9999999X_1", nil},
		{"Q+700.099999.010  00_DESCRIZIONE.7z", ptr("7z"), "", "Q+700.099999.010  00_DESCRIZIONE", "7z", "",
			[]string{CodiceArchivioNonEstraibile}},
		{nfc, ptr("pdf"), "", strings.TrimSuffix(nfc, ".pdf"), "pdf", "", nil},
		{nfd, ptr("pdf"), "", strings.TrimSuffix(nfd, ".pdf"), "pdf", "", nil},
		{"\U0001F527ACME3333.pdf", ptr("pdf"), "", "\U0001F527ACME3333", "pdf", "", nil},
		// Spazi ai bordi: solo una voce d'archivio li può avere (il nome diretto è già ripulito). La regola non
		// li toglie, e l'estensione dell'acquisizione (filepath.Ext) ha lo stesso spazio.
		{" ACME1111.pdf ", ptr("pdf "), "cartella/ ACME1111.pdf ", " ACME1111", "pdf ", "cartella/ ACME1111", nil},
		{"ACME1111", nil, "", "ACME1111", "-", "", nil},
		// Il punto iniziale non apre un'estensione: l'acquisizione ne vede una, e si dice.
		{".nascosto", ptr("nascosto"), "", ".nascosto", "-", "", []string{CodiceNomeEstensioneDiscorde}},
		{"..doppio.pdf", ptr("pdf"), "", "..doppio", "pdf", "", nil},
		// Un punto finale senza niente dopo: nessuna estensione (qui la regola si separa da splitext).
		{"ACME1111.", nil, "", "ACME1111.", "-", "", nil},
		{"a.b.c.pdf", ptr("pdf"), "", "a.b.c", "pdf", "", nil},
		{"FILE.PDF", ptr("pdf"), "", "FILE", "PDF", "", nil},
		{"ACME1111.pdf", ptr("stp"), "", "ACME1111", "pdf", "", []string{CodiceNomeEstensioneDiscorde}},
		{lungo, ptr("pdf"), "", strings.TrimSuffix(lungo, ".pdf"), "pdf", "", []string{CodiceNomeForseTroncato}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			a := allegatoACME(c.nome, c.estensione)
			var contenitore *fotorfq.Allegato
			if c.percorso != "" {
				zip := uuidDi(0x20)
				a.ContenitoreID = &zip
				a.PathInterno = ptr(c.percorso)
			}
			d, err := DaAllegato(a, nil, contenitore)
			if err != nil {
				t.Fatal(err)
			}
			u, ok := unitaDi(d, idUnitaNome)
			if !ok {
				t.Fatal("manca l'unità del nome")
			}
			p := u.Posizione.NomeFile
			if u.Testo != c.nome || u.Selettore.String() != "nome_file" || u.Qualita.Localizzazione != localizzazioneEsatta ||
				p == nil || p.Campo != campoNomeFile || p.Intervallo != intero(c.nome) || p.Percorso != c.percorso {
				t.Fatalf("unità del nome %+v", u)
			}
			stem, est := parti(c.nome, p)
			if stem != c.stem || est != c.est {
				t.Errorf("stem %q ed estensione %q, attesi %q e %q", stem, est, c.stem, c.est)
			}
			if p.Estensione != nil && c.nome[:p.Stem.Fine]+"."+c.nome[p.Estensione.Inizio:] != c.nome {
				t.Errorf("stem, punto ed estensione non ricostruiscono il nome")
			}
			for _, iv := range []*evidenze.Intervallo{p.Stem, p.Estensione} {
				if iv != nil && (!utf8.ValidString(c.nome[iv.Inizio:iv.Fine])) {
					t.Errorf("intervallo %v a metà di una runa", *iv)
				}
			}
			if got := codiciDelNome(d); strings.Join(got, " ") != strings.Join(c.codici, " ") {
				t.Errorf("diagnostiche %v, attese %v", got, c.codici)
			}

			pu, voce := unitaDi(d, idUnitaPercorso)
			if voce != (c.percorso != "") {
				t.Fatalf("unità del percorso presente %v, attesa %v", voce, c.percorso != "")
			}
			if voce {
				pp := pu.Posizione.NomeFile
				pStem, pEst := parti(c.percorso, pp)
				if pu.Testo != c.percorso || pu.Selettore.String() != "voce_archivio" || pp.Campo != campoPercorso ||
					pp.Intervallo != intero(c.percorso) || pStem != c.pStem || pEst != c.est {
					t.Errorf("unità del percorso %+v: stem %q estensione %q", pu, pStem, pEst)
				}
			}
		})
	}

	t.Run("NFC e NFD restano due nomi", func(t *testing.T) {
		if len(nfc) == len(nfd) || nfc == nfd {
			t.Fatal("la prova non ha due forme diverse")
		}
		dc, _ := DaAllegato(allegatoACME(nfc, ptr("pdf")), nil, nil)
		dd, _ := DaAllegato(allegatoACME(nfd, ptr("pdf")), nil, nil)
		if dc.BundleID == dd.BundleID {
			t.Error("nessuna normalizzazione in A1 (limite dichiarato): NFC e NFD devono restare diversi")
		}
	})

	t.Run("le voci d'archivio non si dicono troncate", func(t *testing.T) {
		a := allegatoACME(lungo, ptr("pdf"))
		zip := uuidDi(0x20)
		a.ContenitoreID = &zip
		a.PathInterno = ptr("cartella/" + lungo)
		d, err := DaAllegato(a, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := codiciDelNome(d); len(got) != 0 {
			t.Errorf("diagnostiche %v: il nome di una voce non si tronca (zip.go:71)", got)
		}
	})

	t.Run("la provenienza dice le trasformazioni dell'acquisizione", func(t *testing.T) {
		d, err := DaAllegato(allegatoACME("ACME1111.pdf", ptr("pdf")), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ignoti := strings.Join(d.Fonte.Provenienza.Ignoti, "|")
		if !strings.Contains(ignoti, "300 rune") || d.Fonte.Provenienza.NomeRicevuto != "ACME1111.pdf" {
			t.Errorf("provenienza %+v", d.Fonte.Provenienza)
		}
	})
}

// TestLaRegolaDelloStem: dividiNome da sola, sui casi di confine della regola (C-33).
func TestLaRegolaDelloStem(t *testing.T) {
	for _, c := range []struct{ p, stem, est string }{
		{"", "-", "-"},
		{".", ".", "-"},
		{"..", "..", "-"},
		{"...pdf", "...pdf", "-"},
		{"a.pdf", "a", "pdf"},
		{"cartella.v2/nome", "cartella.v2/nome", "-"},
		{"cartella/.nascosto", "cartella/.nascosto", "-"},
		{"cartella/..a.b", "cartella/..a", "b"},
	} {
		stem, est := dividiNome(c.p)
		gotStem, gotEst := "-", "-"
		if stem != nil {
			gotStem = c.p[stem.Inizio:stem.Fine]
		}
		if est != nil {
			gotEst = c.p[est.Inizio:est.Fine]
		}
		if gotStem != c.stem || gotEst != c.est {
			t.Errorf("%q: stem %q estensione %q, attesi %q e %q", c.p, gotStem, gotEst, c.stem, c.est)
		}
	}
}
