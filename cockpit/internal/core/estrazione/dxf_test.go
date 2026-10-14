// L1 — l'adattatore DXF (K4; mappatura-dxf-1): oltre ai golden dxf_01…03 (golden_test.go), le proprietà che un
// golden da solo non dice: la mappatura la guida la chiave testo_dxf e non l'estensione, e un DXF senza testo_dxf
// resta il documento di prima; i campi del cartiglio stanno con il disegno solo per EntitaID; il riferimento 3D ha
// il suo selettore; un DXF non letto, troncato o senza cartiglio non è mai un DXF «senza codici»; stessi fatti,
// stesso documento.
//
// Tutti i dati sono sintetici (ACME, codici di fantasia, UUID 00000000-0000-4000-8000-0000000000nn): il
// repository è pubblico.
package estrazione

import (
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// testoDXFACME: un payload con il testo di un DXF, i campi già scritti in JSON.
func testoDXFACME(versione, esito, motivo, troncato, annidati, cartiglio string) string {
	return `{"esito":{"tipo_proposto":"sviluppo_dxf","codice":"ACME7000200","rev":"00"},"testo_dxf":{"versione":` + versione +
		`,"esito":"` + esito + `","motivo":"` + motivo + `","versione_dxf":"AC1014","codifica":"cp1252","testi":40,"annidati":` + annidati +
		`,"troncato":` + troncato + `,"cartiglio":` + cartiglio + `,"limiti":{"byte_max":67108864,"testi_max":5000,"testo_max":512,"campi_max":24}}}`
}

// campoDXFACME: un campo come lo scrive workers/dxf_testo.py (zona «pagina», fonte «nativo», riquadro in unità del
// disegno con la y verso l'alto).
func campoDXFACME(etichetta, letta, valore string) string {
	return `{"etichetta":"` + etichetta + `","letta":"` + letta + `","valore":"` + valore +
		`","pagina":1,"zona":"pagina","fonte":"nativo","riquadro":[-145.73,1.75,-122.63,5.25],"confidenza":null}`
}

// cartiglioDXFACME: il cartiglio della forma di quello che K4 legge: il materiale, il documento 2D con la sua
// revisione e il documento 3D della stessa parte.
func cartiglioDXFACME() string {
	return `[` + campoDXFACME("riferimento_3d", "ref.3D doc./Ref.3D Dok.", "ACME7100400") + `,` +
		campoDXFACME("codice", "part no./Mat. Nr.", "ACME7000200") + `,` +
		campoDXFACME("numero_disegno", "doc. no./Dok. Nr.", "ACME7100300") + `,` +
		campoDXFACME("revisione", "rev./Rev.", "00") + `]`
}

// dxfACME: il documento di un DXF sintetico, con il nome e il payload dati.
func dxfACME(t *testing.T, nome, payload string) evidenze.DocumentoEvidenze {
	t.Helper()
	est := nome[strings.LastIndexByte(nome, '.')+1:]
	a := allegatoACME(nome, ptr(est))
	a.Sha256 = ptr(shaACME)
	d, err := DaAllegato(a, fattiACME(t, shaACME, payload), nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestIlCartiglioDelDXFStaConIlDisegno: ogni campo con un valore è un'unità cartiglio.<etichetta> legata alla sola
// entità «disegno» del DXF, con l'etichetta letta, il parser testo_dxf.cartiglio.<etichetta>, mappatura-dxf-1 e il
// localizzatore dxf in decimi interi. riferimento_3d ha il suo selettore (non «sconosciuto»). Un campo vuoto non dà
// unità ma conta per l'ID del campo dopo con la stessa etichetta; un'etichetta che la foglia non ha diventa
// cartiglio.sconosciuto e conserva la sua nel parser. Nessun legame: l'appartenenza sta in EntitaID (legami-1).
func TestIlCartiglioDelDXFStaConIlDisegno(t *testing.T) {
	cartiglio := strings.TrimSuffix(cartiglioDXFACME(), "]") + `,` + campoDXFACME("codice", "part no./Mat. Nr.", "") + `,` +
		campoDXFACME("codice", "PART NO.", "ACME7000201") + `,` + campoDXFACME("futuro_campo", "next/Nächst", "X1") + `]`
	d := dxfACME(t, "ACME7000200_D42_ACME7100300_000_00.dxf", testoDXFACME("1", "letto", "", "false", "0", cartiglio))

	attese := map[string][3]string{ // ID → selettore, testo, etichetta letta
		"u:dxf:cartiglio:riferimento_3d:1": {"cartiglio.riferimento_3d", "ACME7100400", "ref.3D doc./Ref.3D Dok."},
		"u:dxf:cartiglio:codice:1":         {"cartiglio.codice", "ACME7000200", "part no./Mat. Nr."},
		"u:dxf:cartiglio:codice:3":         {"cartiglio.codice", "ACME7000201", "PART NO."},
		"u:dxf:cartiglio:numero_disegno:1": {"cartiglio.numero_disegno", "ACME7100300", "doc. no./Dok. Nr."},
		"u:dxf:cartiglio:revisione:1":      {"cartiglio.revisione", "00", "rev./Rev."},
		"u:dxf:cartiglio:futuro_campo:1":   {"cartiglio.sconosciuto", "X1", "next/Nächst"},
	}
	n := 0
	for _, u := range d.Unita {
		if u.Selettore.Contesto != evidenze.ContestoCartiglio {
			continue
		}
		n++
		a, ok := attese[u.ID]
		if !ok {
			t.Errorf("unità inattesa %s", u.ID)
			continue
		}
		campo := strings.TrimPrefix(u.ID, "u:dxf:cartiglio:")
		campo = campo[:strings.LastIndexByte(campo, ':')]
		if u.Selettore.String() != a[0] || u.Testo != a[1] || u.CampoOriginale.Etichetta != a[2] ||
			u.CampoOriginale.Parser != "testo_dxf.cartiglio."+campo || u.CampoOriginale.Mappatura != mappaturaDXF {
			t.Errorf("%s: %+v", u.ID, u)
		}
		if u.EntitaID != idEntitaCartiglioDXF || u.Posizione.Tipo != "dxf" || u.Posizione.DXF == nil ||
			u.Posizione.DXF.RiquadroDecimi == nil || *u.Posizione.DXF.RiquadroDecimi != [4]int{-1457, 18, -1226, 53} {
			t.Errorf("%s: entità %q, posizione %+v", u.ID, u.EntitaID, u.Posizione)
		}
		if u.Qualita.Localizzazione != localizzazioneParziale || u.Qualita.Metodo != metodoNativo {
			t.Errorf("%s: qualità %+v", u.ID, u.Qualita)
		}
	}
	if n != len(attese) {
		t.Errorf("%d unità del cartiglio, attese %d", n, len(attese))
	}
	if len(d.Entita) != 1 || d.Entita[0].ID != idEntitaCartiglioDXF || d.Entita[0].Tipo != tipoEntitaDisegno || d.Entita[0].Posizione.DXF == nil {
		t.Errorf("entità %+v: attesa la sola entità «disegno» del DXF", d.Entita)
	}
	if len(d.Legami) != 0 {
		t.Errorf("legami %+v: l'appartenenza sta in EntitaID", d.Legami)
	}
	if c, ok := capacitaDi(d, capCartiglio); !ok || c.Stato != statoDisponibile {
		t.Errorf("capacità del cartiglio %+v", c)
	}
	if _, ok := capacitaDi(d, capTesto); ok {
		t.Error("il DXF dichiara la capacità del testo: il worker non riporta i testi del disegno, e non è una capacità del DXF")
	}
	if d.Qualita.Stato != statoDisponibile || d.Qualita.Metodo != metodoNativo || len(d.Qualita.Diagnostiche) != 0 {
		t.Errorf("qualità %+v", d.Qualita)
	}
}

// TestUnDXFSenzaTestoDXFEIlDocumentoDiPrima: la mappatura la guida la chiave dei fatti. Un DXF con il solo esito
// del worker (analizzato prima del K4) ha il solo nome, con la capacità del contenuto e il motivo di prima, e
// nessuna unità del cartiglio: il codice dell'esito non entra mai. Con testo_dxf, il contenuto si legge anche se
// l'estensione non lo dice.
func TestUnDXFSenzaTestoDXFEIlDocumentoDiPrima(t *testing.T) {
	d := dxfACME(t, "ACME7000200.dxf", `{"esito":{"tipo_proposto":"sviluppo_dxf","codice":"ACME7000200","rev":"00"}}`)
	if c, ok := capacitaDi(d, capContenuto); !ok || c.Stato != statoNonDisponibile || !strings.Contains(c.Motivo, "solo l'esito del worker") {
		t.Errorf("capacità del contenuto %+v", c)
	}
	if len(d.Unita) != 1 || d.Unita[0].ID != "u:nome" || len(d.Entita) != 0 || d.Qualita.Metodo != metodoAdattatore {
		t.Errorf("unità %+v, entità %+v, metodo %q: atteso il solo nome", d.Unita, d.Entita, d.Qualita.Metodo)
	}
	altra := dxfACME(t, "ACME7000200.bin", testoDXFACME("1", "letto", "", "false", "0", cartiglioDXFACME()))
	if _, ok := unitaDi(altra, "u:dxf:cartiglio:riferimento_3d:1"); !ok {
		t.Error("con testo_dxf nei fatti il cartiglio si legge anche con un'altra estensione")
	}
}

// TestUnDXFNonLettoNonEUnDXFSenzaCodici: un DXF binario (esito «non_letto»), un testo_dxf di una versione
// sconosciuta, un DXF troncato, un DXF letto senza nessun campo riconosciuto: ognuno ha la sua diagnostica, la
// capacità del cartiglio non disponibile o parziale con il motivo, e mai «nessun codice» in silenzio. Gli INSERT
// annidati non seguiti, da soli, non peggiorano niente (sui DXF veri ci sono anche con il cartiglio letto): si
// dicono nel messaggio dove possono spiegare un campo che manca; lo stesso per gli INSERT ruotati (il motivo di un
// testo letto).
func TestUnDXFNonLettoNonEUnDXFSenzaCodici(t *testing.T) {
	// Il motivo di un testo letto: gli INSERT ruotati che il worker non segue (dxf_testo.py). Come gli annidati, da
	// solo non peggiora niente, e si dice nei messaggi.
	const motivoRuotatiACME = "1 INSERT ruotati non seguiti: i testi dei loro blocchi non sono letti (la regola del cartiglio vale per i blocchi diritti)"
	for _, c := range []struct {
		nome, payload  string
		codice         string
		cartiglio      string // lo stato della capacità
		motivo         string
		stato          string // della fonte
		unitaCartiglio int
	}{
		{"binario", testoDXFACME("1", "non_letto", "DXF binario: si leggono solo i DXF ASCII", "false", "0", "[]"),
			CodiceDXFNonLetto, statoNonDisponibile, "DXF binario", statoParziale, 0},
		{"versione sconosciuta", testoDXFACME("0", "letto", "", "false", "0", cartiglioDXFACME()),
			CodiceDXFNonLetto, statoNonDisponibile, "versione", statoParziale, 0},
		{"troncato", testoDXFACME("1", "letto", "", "true", "0", cartiglioDXFACME()),
			CodiceDXFTroncato, statoParziale, "tetto", statoParziale, 4},
		{"troncato con annidati", testoDXFACME("1", "letto", "", "true", "2", cartiglioDXFACME()),
			CodiceDXFTroncato, statoParziale, "2 INSERT annidati", statoParziale, 4},
		{"annidati con il cartiglio", testoDXFACME("1", "letto", "", "false", "2", cartiglioDXFACME()),
			"", statoDisponibile, "", statoDisponibile, 4},
		{"senza cartiglio", testoDXFACME("1", "letto", "", "false", "0", "[]"),
			CodiceDXFSenzaCartiglio, statoDisponibile, "", statoDisponibile, 0},
		{"senza cartiglio con annidati", testoDXFACME("1", "letto", "", "false", "3", "[]"),
			CodiceDXFSenzaCartiglio, statoDisponibile, "", statoDisponibile, 0},
		{"senza cartiglio con ruotati", testoDXFACME("1", "letto", motivoRuotatiACME, "false", "0", "[]"),
			CodiceDXFSenzaCartiglio, statoDisponibile, "", statoDisponibile, 0},
		{"troncato con ruotati", testoDXFACME("1", "letto", motivoRuotatiACME, "true", "0", cartiglioDXFACME()),
			CodiceDXFTroncato, statoParziale, "1 INSERT ruotati", statoParziale, 4},
		{"ruotati con il cartiglio", testoDXFACME("1", "letto", motivoRuotatiACME, "false", "0", cartiglioDXFACME()),
			"", statoDisponibile, "", statoDisponibile, 4},
	} {
		t.Run(c.nome, func(t *testing.T) {
			d := dxfACME(t, "ACME7000200.dxf", c.payload)
			var attese []string
			if c.codice != "" {
				attese = []string{c.codice}
			}
			if codici := codiciDi(d); !slices.Equal(codici, attese) {
				t.Errorf("diagnostiche %v, attese %v", codici, attese)
			}
			if strings.Contains(c.nome, "annidati") && c.codice != "" && !strings.Contains(d.Qualita.Diagnostiche[0].Messaggio, "INSERT annidati") {
				t.Errorf("il messaggio non dice gli INSERT annidati: %q", d.Qualita.Diagnostiche[0].Messaggio)
			}
			if strings.Contains(c.nome, "ruotati") && c.codice != "" && !strings.Contains(d.Qualita.Diagnostiche[0].Messaggio, "INSERT ruotati") {
				t.Errorf("il messaggio non dice gli INSERT ruotati (il motivo di un testo letto): %q", d.Qualita.Diagnostiche[0].Messaggio)
			}
			cc, ok := capacitaDi(d, capCartiglio)
			if !ok || cc.Stato != c.cartiglio || !strings.Contains(cc.Motivo, c.motivo) {
				t.Errorf("capacità del cartiglio %+v, attesa %s con «%s»", cc, c.cartiglio, c.motivo)
			}
			if d.Qualita.Stato != c.stato {
				t.Errorf("stato della fonte %q, atteso %q", d.Qualita.Stato, c.stato)
			}
			n := 0
			for _, u := range d.Unita {
				if u.Selettore.Contesto == evidenze.ContestoCartiglio {
					n++
				}
			}
			if n != c.unitaCartiglio || (n == 0) != (len(d.Entita) == 0) {
				t.Errorf("%d unità del cartiglio e %d entità, attese %d unità", n, len(d.Entita), c.unitaCartiglio)
			}
		})
	}
}

// TestStessiFattiDelDXFStessoDocumento: due letture degli stessi fatti danno lo stesso documento, byte per byte nel
// JSON canonico, con lo stesso BundleID; un valore diverso nel cartiglio cambia il BundleID.
func TestStessiFattiDelDXFStessoDocumento(t *testing.T) {
	p := testoDXFACME("1", "letto", "", "false", "0", cartiglioDXFACME())
	a, b := dxfACME(t, "ACME7000200.dxf", p), dxfACME(t, "ACME7000200.dxf", p)
	ca, err := jsoncanonico.Codifica(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := jsoncanonico.Codifica(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(ca) != string(cb) || a.BundleID == "" || a.BundleID != b.BundleID {
		t.Fatal("stessi fatti, documenti diversi")
	}
	altro := dxfACME(t, "ACME7000200.dxf", strings.Replace(p, "ACME7100400", "ACME7100401", 1))
	if altro.BundleID == a.BundleID {
		t.Error("un riferimento 3D diverso non cambia il BundleID")
	}
}
