// L1 — DaAllegato con il solo nome (A1b-17; piano A, 5.4.5; REG §5.10, «Conseguenze»): un file senza fatti,
// con il solo esito del worker, con una natura diversa da «file», una voce di zip con il suo contenitore, un
// archivio .7z senza voci. Ogni volta la capacità del contenuto è dichiarata con il motivo, e la fonte è
// parziale: mai un file «senza codici». I record incoerenti sono errori, mai documenti.
//
// Tutti i dati sono sintetici: ACME, codici di fantasia (ACME-030PB07XX0001, 9999999X_1, Q+700.099999.010),
// UUID della forma 00000000-0000-4000-8000-0000000000nn. Il repository è pubblico.
package estrazione

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// uuidDi: gli UUID sintetici delle prove, 00000000-0000-4000-8000-0000000000nn.
func uuidDi(n int) uuid.UUID {
	return uuid.MustParse("00000000-0000-4000-8000-0000000000" + string("0123456789abcdef"[n/16]) + string("0123456789abcdef"[n%16]))
}

func ptr[T any](v T) *T { return &v }

// allegatoACME: un allegato diretto con il nome e l'estensione come li scriverebbe l'acquisizione.
func allegatoACME(nome string, estensione *string) fotorfq.Allegato {
	return fotorfq.Allegato{
		ID:          uuidDi(1),
		MessaggioID: uuidDi(0x90),
		Indice:      1,
		NomeFile:    nome,
		Estensione:  estensione,
		ContentType: ptr("application/octet-stream"),
		Natura:      "file",
		Origine:     "outlook",
		Bytes:       ptr(int64(2048)),
		RicevutoIl:  time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC),
	}
}

// fattiACME: dei fatti sintetici con il payload dato e l'impronta calcolata come la calcola chi riempie un Fatti.
func fattiACME(t *testing.T, sha, payload string) *fotorfq.Fatti {
	t.Helper()
	d, err := fotorfq.ImprontaPayload(json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	return &fotorfq.Fatti{
		Sha256:      sha,
		Terna:       fotorfq.Terna{Versione: 5, HashConfigurazione: "acme-configurazione-1"},
		CalcolatoIl: time.Date(2026, 10, 1, 8, 5, 0, 123456789, time.FixedZone("CEST", 2*3600)),
		Payload:     json.RawMessage(payload),
		Digest:      d,
	}
}

const shaACME = "00000000000000000000000000000000000000000000000000000000000000a1"

// codiciDi: i codici delle diagnostiche del documento, in ordine.
func codiciDi(d evidenze.DocumentoEvidenze) []string {
	var out []string
	for _, x := range d.Qualita.Diagnostiche {
		out = append(out, x.Codice)
	}
	return out
}

func capacitaDi(d evidenze.DocumentoEvidenze, nome string) (evidenze.Capacita, bool) {
	for _, c := range d.Qualita.Capacita {
		if c.Nome == nome {
			return c, true
		}
	}
	return evidenze.Capacita{}, false
}

func unitaDi(d evidenze.DocumentoEvidenze, id string) (evidenze.UnitaEvidenza, bool) {
	for _, u := range d.Unita {
		if u.ID == id {
			return u, true
		}
	}
	return evidenze.UnitaEvidenza{}, false
}

// TestUnFileSenzaFattiHaSoloIlNome (A1b-17).
func TestUnFileSenzaFattiHaSoloIlNome(t *testing.T) {
	zip := uuidDi(0x20)
	voce := allegatoACME("9999999X_1.IGS", ptr("igs"))
	voce.ContenitoreID = &zip
	voce.PathInterno = ptr("9999999A1/9999999X_1.IGS")
	contenitore := allegatoACME("9999999A1.zip", ptr("zip"))
	contenitore.ID = zip

	elemento := allegatoACME("ACME richiesta.msg", ptr("msg"))
	elemento.Natura = "elemento_outlook"

	casi := []struct {
		nome        string
		a           fotorfq.Allegato
		f           *fotorfq.Fatti
		contenitore *fotorfq.Allegato
		motivo      string   // un pezzo del motivo della capacità del contenuto
		codici      []string // le diagnostiche attese
		unita       []string
	}{
		{"senza fatti", allegatoACME("ACME-030PB07XX0001.igs", ptr("igs")), nil, nil, "nessun fatto", nil, []string{"u:nome"}},
		{"con il solo esito", allegatoACME("ACME-030PB07XX0001.igs", ptr("igs")),
			fattiACME(t, shaACME, `{"esito":{"tipo_proposto":"cad_3d","codice":"ACME9999999","rev":"7","confidenza":60,"fonte":"estensione"}}`),
			nil, "solo l'esito", nil, []string{"u:nome"}},
		{"con fatti che nessuna mappatura legge", allegatoACME("ACME-030PB07XX0001.igs", ptr("igs")),
			fattiACME(t, shaACME, `{"esito":{"tipo_proposto":"cad_3d"},"misure":{"pezzi":1}}`),
			nil, "non hanno una parte", nil, []string{"u:nome"}},
		{"natura diversa da file", elemento, nil, nil, "elemento_outlook", nil, []string{"u:nome"}},
		{"voce di zip con il contenitore", voce, nil, &contenitore, "nessun fatto", nil, []string{"u:nome", "u:percorso"}},
		{"voce di zip senza il record del contenitore", voce, nil, nil, "nessun fatto", nil, []string{"u:nome", "u:percorso"}},
		{"7z senza voci", allegatoACME("Q+700.099999.010  00_DESCRIZIONE.7z", ptr("7z")), nil, nil, "archivio .7z",
			[]string{CodiceArchivioNonEstraibile}, []string{"u:nome"}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d, err := DaAllegato(c.a, c.f, c.contenitore)
			if err != nil {
				t.Fatal(err)
			}
			if diag := evidenze.ValidaDocumento(d); diag != nil {
				t.Fatalf("documento non valido: %v", diag)
			}
			// Mai «nessun codice»: la fonte è parziale e il contenuto è dichiarato, con il motivo.
			if d.Qualita.Stato != statoParziale {
				t.Errorf("stato della fonte %q, atteso %q", d.Qualita.Stato, statoParziale)
			}
			cap, ok := capacitaDi(d, capContenuto)
			if !ok || cap.Stato != statoNonDisponibile || !strings.Contains(cap.Motivo, c.motivo) {
				t.Errorf("capacità del contenuto %+v, attesa non disponibile con un motivo che dice %q", cap, c.motivo)
			}
			if strings.Join(codiciDi(d), " ") != strings.Join(c.codici, " ") {
				t.Errorf("diagnostiche %v, attese %v", codiciDi(d), c.codici)
			}
			var ids []string
			for _, u := range d.Unita {
				ids = append(ids, u.ID)
				if strings.Contains(u.Testo, "ACME9999999") {
					t.Errorf("l'unità %s porta il codice dell'esito del worker, che non si usa", u.ID)
				}
			}
			if strings.Join(ids, " ") != strings.Join(c.unita, " ") {
				t.Errorf("unità %v, attese %v", ids, c.unita)
			}
			if c.f == nil && d.Fonte.RiferimentoFatti.Tipo != riferimentoNessuno {
				t.Errorf("senza fatti il riferimento è %q", d.Fonte.RiferimentoFatti.Tipo)
			}
			if c.f != nil {
				r := d.Fonte.RiferimentoFatti
				if r.Tipo != riferimentoAnalisiFile || r.DigestPayload != c.f.Digest || r.Sha256 != shaACME ||
					r.VersioneAnalizzatore != 5 || !r.CalcolatoIl.Equal(time.Date(2026, 10, 1, 6, 5, 0, 123000000, time.UTC)) ||
					r.CalcolatoIl.Location() != time.UTC {
					t.Errorf("riferimento ai fatti %+v: terna, digest e calcolato_il (UTC, al millisecondo) attesi", r)
				}
			}
			if d.BundleID == "" || len(d.BundleID) != 64 {
				t.Errorf("BundleID %q", d.BundleID)
			}
			if c.a.ContenitoreID == nil && d.Fonte.ContenitoreID != "messaggio:"+c.a.MessaggioID.String() {
				t.Errorf("il contenitore di un allegato diretto è il suo messaggio, non %q", d.Fonte.ContenitoreID)
			}
		})
	}

	t.Run("la voce porta il contenitore e il percorso", func(t *testing.T) {
		d, err := DaAllegato(voce, nil, &contenitore)
		if err != nil {
			t.Fatal(err)
		}
		if d.Fonte.ContenitoreID != "allegato:"+zip.String() || d.Fonte.Provenienza.PercorsoRicevuto != *voce.PathInterno {
			t.Errorf("fonte %+v: contenitore e percorso attesi", d.Fonte)
		}
		u, _ := unitaDi(d, idUnitaPercorso)
		if u.Selettore.String() != "voce_archivio" || u.Testo != *voce.PathInterno || u.Posizione.NomeFile.Campo != campoPercorso {
			t.Errorf("unità del percorso %+v", u)
		}
		if !strings.Contains(strings.Join(d.Fonte.Provenienza.Ignoti, "|"), "CP-437") {
			t.Errorf("il nome grezzo della voce va fra gli ignoti: %v", d.Fonte.Provenienza.Ignoti)
		}
		for _, x := range d.Unita {
			if strings.Contains(x.Testo, "9999999A1.zip") {
				t.Errorf("il nome del contenitore è entrato fra le unità della voce: %s", x.ID)
			}
		}
	})

	t.Run("i byte dell'allegato restano fuori dal BundleID", func(t *testing.T) {
		a := allegatoACME("ACME-030PB07XX0001.igs", ptr("igs"))
		b := a
		b.Bytes = ptr(int64(4096))
		da, errA := DaAllegato(a, nil, nil)
		db, errB := DaAllegato(b, nil, nil)
		if errA != nil || errB != nil {
			t.Fatal(errA, errB)
		}
		if da.BundleID != db.BundleID || *db.Fonte.Provenienza.Bytes != 4096 {
			t.Errorf("BundleID diversi per i soli byte (R51 A): %s %s", da.BundleID, db.BundleID)
		}
		c := a
		c.NomeFile = "ACME-030PB07XX0002.igs"
		dc, err := DaAllegato(c, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if dc.BundleID == da.BundleID {
			t.Error("due nomi diversi danno lo stesso BundleID")
		}
	})
}

// TestUnRecordIncoerenteEUnErrore: i record e i fatti che non stanno insieme sono errori, mai un documento.
// Attaccare a un file i fatti di un altro contenuto sarebbe una relazione inventata (5.0).
func TestUnRecordIncoerenteEUnErrore(t *testing.T) {
	zip, altroZip := uuidDi(0x20), uuidDi(0x21)
	voce := allegatoACME("9999999X_1.IGS", ptr("igs"))
	voce.ContenitoreID = &zip
	voce.PathInterno = ptr("9999999A1/9999999X_1.IGS")
	altro := allegatoACME("9999999A1.zip", ptr("zip"))
	altro.ID = altroZip

	percorsoSenzaZip := allegatoACME("9999999X_1.IGS", ptr("igs"))
	percorsoSenzaZip.PathInterno = ptr("9999999A1/9999999X_1.IGS")

	conSha := allegatoACME("ACME-030PB07XX0001.igs", ptr("igs"))
	conSha.Sha256 = ptr(shaACME)
	fattiAltri := fattiACME(t, "00000000000000000000000000000000000000000000000000000000000000b2", `{"esito":{}}`)
	fattiDigest := fattiACME(t, shaACME, `{"esito":{}}`)
	fattiDigest.Digest = strings.Repeat("0", 64)

	for _, c := range []struct {
		nome        string
		a           fotorfq.Allegato
		f           *fotorfq.Fatti
		contenitore *fotorfq.Allegato
	}{
		{"contenitore diverso da quello del record", voce, nil, &altro},
		{"contenitore dato per un allegato diretto", allegatoACME("ACME-030PB07XX0001.igs", ptr("igs")), nil, &altro},
		{"percorso interno senza contenitore", percorsoSenzaZip, nil, nil},
		{"fatti di un altro contenuto", conSha, fattiAltri, nil},
		{"digest diverso dall'impronta del payload", conSha, fattiDigest, nil},
		{"payload vuoto", conSha, &fotorfq.Fatti{Sha256: shaACME}, nil},
		{"payload che non è un oggetto", conSha, &fotorfq.Fatti{Sha256: shaACME, Payload: json.RawMessage(`[1,2]`)}, nil},
	} {
		t.Run(c.nome, func(t *testing.T) {
			d, err := DaAllegato(c.a, c.f, c.contenitore)
			if err == nil {
				t.Fatalf("nessun errore, documento %s", d.BundleID)
			}
			if d.BundleID != "" || d.Unita != nil {
				t.Errorf("con l'errore torna anche un documento: %+v", d)
			}
		})
	}

	t.Run("un documento che non passa la porta della foglia torna come errore di contratto", func(t *testing.T) {
		c := nuovoDocumento(fonteDiTesto())
		c.unita(evidenze.UnitaEvidenza{ID: "u:x", Selettore: selettoreDi(evidenze.ContestoCorpo), Testo: "ACME1111",
			Posizione: evidenze.Localizzatore{Tipo: "testo", Testo: &evidenze.PosTesto{TestoID: "manca", Intervallo: intero("ACME1111")}},
			Qualita:   evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta}})
		_, err := c.chiudi()
		var ec *evidenze.ErroreContratto
		if !errors.As(err, &ec) || len(ec.Diagnostiche) == 0 || ec.Diagnostiche[0].Codice != evidenze.CodiceDocumentoRiferimentoPendente {
			t.Errorf("errore %v, atteso un *ErroreContratto con documento.riferimento_pendente", err)
		}
		c2 := nuovoDocumento(fonteDiTesto())
		c2.capacita("x", statoDisponibile, "")
		c2.capacita("x", statoParziale, "")
		if _, err := c2.chiudi(); err == nil {
			t.Error("una capacità dichiarata due volte passa")
		}
	})
}
