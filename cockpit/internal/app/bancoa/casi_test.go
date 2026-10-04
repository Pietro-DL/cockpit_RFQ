package bancoa

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/dataset"
)

// L1 — i casi_contratto sul riconoscimento per forma (A1a-BA; R24 a, R25, R19 a): esiti passato, parziale,
// fallito, riservato e rimandato sui dieci casi ACME di testdata/attesi_acme.yaml; un caso definito con
// dipende_da si valuta e il rapporto lo annota; un caso con stato_atteso riservato non si valuta; un predicato
// su una forma riservata dà «riservato»; figlio_step diventa nodo_step; profilo senza cliente, cliente
// scartato e contesto illeggibile fanno fallire il caso; due esecuzioni danno gli stessi esiti. Qui ci sono
// anche gli aiuti che le altre prove del pacchetto usano (il dataset ACME in t.TempDir()).
//
// I clienti di questi test sono inventati. Non è pigrizia: gli attesi veri, le grammatiche vere e il
// manifest vero sono dati privati, e questo repository è pubblico. Il cliente è ACME, con un UUID inventato;
// i file sintetici hanno nomi con _acme, diversi da quelli del dataset privato; gli ID dei casi sono
// «caso-acme-NN». Le prove citano i requisiti (A1a-BA, R19, R25, R44), mai i casi degli attesi.

var clienteACME = uuid.MustParse("00000000-0000-4000-8000-00000000ac01")

var zero64 = strings.Repeat("0", 64)

func shaDi(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// leggiTestdata: un file di testdata con i fine riga LF. Con core.autocrlf il checkout può riscriverli in
// CRLF: gli sha256 si calcolano sempre sui byte che la prova scrive, mai su quelli del checkout.
func leggiTestdata(t testing.TB, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// mutazioni: per nome del file di testdata, una modifica del testo prima di calcolare gli sha256. Per il
// manifest la modifica vale sul JSON finale.
type mutazioni map[string]func(string) string

func (m mutazioni) applica(rel, s string) string {
	if f := m[rel]; f != nil {
		return f(s)
	}
	return s
}

// datasetACME: il dataset sintetico scritto in t.TempDir(), fuori dal modulo, con gli sha256 e i byte veri.
type datasetACME struct {
	dir, manifest string
}

func (d datasetACME) percorso(rel string) string {
	return filepath.Join(d.dir, filepath.FromSlash(rel))
}

// preparaDataset copia attesi, indice e grammatica ACME in t.TempDir(), calcola gli sha256 (della grammatica
// nell'indice; di indice e attesi nel manifest) e scrive il manifest con i nomi delle voci di A1a.
func preparaDataset(t testing.TB, muta mutazioni) datasetACME {
	t.Helper()
	d := datasetACME{dir: t.TempDir()}
	scrivi := func(rel, s string) {
		p := d.percorso(rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g := muta.applica("regole/acme.v1.json", leggiTestdata(t, "regole/acme.v1.json"))
	scrivi("regole/acme.v1.json", g)
	ix := muta.applica("regole/indice_acme.v1.json", strings.Replace(leggiTestdata(t, "regole/indice_acme.v1.json"), zero64, shaDi([]byte(g)), 1))
	scrivi("regole/indice_acme.v1.json", ix)
	at := muta.applica("attesi_acme.yaml", leggiTestdata(t, "attesi_acme.yaml"))
	scrivi("attesi_acme.yaml", at)

	m, err := dataset.Leggi([]byte(leggiTestdata(t, "manifest_acme.json")), d.dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range m.Voci {
		switch v.Nome {
		case VoceAttesi:
			m.Voci[i].Sha256, m.Voci[i].Byte = shaDi([]byte(at)), int64(len(at))
		case VoceIndice:
			m.Voci[i].Sha256, m.Voci[i].Byte = shaDi([]byte(ix)), int64(len(ix))
		}
	}
	js, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	scrivi("manifest_acme.json", muta.applica("manifest_acme.json", string(js)))
	d.manifest = d.percorso("manifest_acme.json")
	return d
}

// regoleACME: l'indice letto con LeggiIndice e i contenuti delle grammatiche, con gli sha256 veri.
func regoleACME(t testing.TB, muta mutazioni) (grammatica.IndiceRegole, map[string][]byte) {
	t.Helper()
	g := muta.applica("regole/acme.v1.json", leggiTestdata(t, "regole/acme.v1.json"))
	ix := muta.applica("regole/indice_acme.v1.json", strings.Replace(leggiTestdata(t, "regole/indice_acme.v1.json"), zero64, shaDi([]byte(g)), 1))
	ind, err := grammatica.LeggiIndice([]byte(ix))
	if err != nil {
		t.Fatal(err)
	}
	return ind, map[string][]byte{"acme.v1.json": []byte(g)}
}

// insiemeACME: i motori ACME, compilati come li compila il banco.
func insiemeACME(t testing.TB) motorea.InsiemeRegole {
	t.Helper()
	ind, contenuti := regoleACME(t, nil)
	ins, _, err := motorea.CompilaInsieme(ind, contenuti)
	if err != nil {
		t.Fatal(err)
	}
	if ins.Motori[clienteACME] == nil {
		t.Fatalf("la grammatica ACME non compila: %v", ins.Scartati[clienteACME])
	}
	return ins
}

func attesiACME(t testing.TB, muta mutazioni) Attesi {
	t.Helper()
	a, err := LeggiAttesi([]byte(muta.applica("attesi_acme.yaml", leggiTestdata(t, "attesi_acme.yaml"))))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

var profiliACME = map[string]uuid.UUID{"acme": clienteACME}

func perID(es []EsitoCaso) map[string]EsitoCaso {
	out := map[string]EsitoCaso{}
	for _, e := range es {
		out[e.ID] = e
	}
	return out
}

func chiave(e EsitoCaso, nome string) EsitoChiave {
	for _, k := range e.Chiavi {
		if k.Chiave == nome {
			return k
		}
	}
	return EsitoChiave{}
}

// TestCasiACMEEsiti: i dieci casi ACME, uno per meccanismo.
func TestCasiACMEEsiti(t *testing.T) {
	es := EseguiCasiContratto(attesiACME(t, nil), insiemeACME(t), profiliACME)
	if len(es) != 10 {
		t.Fatalf("%d esiti, attesi 10", len(es))
	}
	attesi := map[string]string{
		"caso-acme-01": CasoPassato, "caso-acme-02": CasoPassato, "caso-acme-03": CasoPassato,
		"caso-acme-04": CasoPassato, "caso-acme-05": CasoPassato, "caso-acme-06": CasoParziale,
		"caso-acme-07": CasoRiservato, "caso-acme-08": CasoRiservato, "caso-acme-09": CasoRimandato,
		"caso-acme-10": CasoRimandato,
	}
	got := perID(es)
	for id, esito := range attesi {
		if got[id].Esito != esito {
			t.Errorf("%s: esito %q, atteso %q (%s; chiavi %+v)", id, got[id].Esito, esito, got[id].Motivo, got[id].Chiavi)
		}
	}
	if n := Conta(es); n != (ConteggiCasi{Totale: 10, Passati: 5, Parziali: 1, Riservati: 2, Rimandati: 2}) {
		t.Errorf("conteggi %+v", n)
	}

	// Un caso definito con dipende_da si valuta, e il rapporto lo annota: non è riservato.
	if e := got["caso-acme-01"]; e.DipendeDa != "Q-ACME-1" || e.Esito != CasoPassato {
		t.Errorf("dipende_da: %+v", e)
	}
	// R19 a: figlio_step.id diventa nodo_step.id.
	if e := got["caso-acme-03"]; e.Selettore != "nodo_step.id" {
		t.Errorf("figlio_step tradotto in %q", e.Selettore)
	}
	// Il parziale: la chiave controllata passa, quella di A1b è rimandata con la sessione.
	p := got["caso-acme-06"]
	if k := chiave(p, "fallback_generico_non_promuove"); k.Stato != ChiaveRimandata || k.Sessione != SessioneA1b {
		t.Errorf("chiave rimandata: %+v", k)
	}
	if k := chiave(p, "etichetta"); k.Stato != ChiavePassata {
		t.Errorf("chiave controllata: %+v", k)
	}
	// stato_atteso riservato: non si valuta.
	if k := chiave(got["caso-acme-07"], "match_forma_lavagna"); k.Stato != ChiaveRiservata {
		t.Errorf("caso riservato: %+v", k)
	}
	// Un predicato su una forma riservata non si decide.
	r := got["caso-acme-08"]
	if k := chiave(r, "match_forma_lavagna"); k.Stato != ChiaveRiservata || !strings.Contains(k.Motivo, "riservata") {
		t.Errorf("predicato su forma riservata: %+v", k)
	}
	if !reflect.DeepEqual(r.RiservateSulSelettore, []string{"acme-lavagna/lavagna"}) {
		t.Errorf("forme riservate sul selettore: %v", r.RiservateSulSelettore)
	}
	// Letture d'identità diverse da zero e menzioni: A1b.
	if k := chiave(got["caso-acme-09"], "letture_identita"); k.Stato != ChiaveRimandata {
		t.Errorf("letture_identita > 0: %+v", k)
	}
	// L'attributo della revisione in campo separato: tutte le chiavi rimandate, anche revisione.
	for _, k := range got["caso-acme-10"].Chiavi {
		if k.Stato != ChiaveRimandata || k.Motivo != motivoAttributo {
			t.Errorf("attributo della revisione: %+v", k)
		}
	}
	// Nessun parziale, riservato o rimandato ha esito «passato».
	for _, e := range es {
		for _, k := range e.Chiavi {
			if e.Esito == CasoPassato && k.Stato != ChiavePassata {
				t.Errorf("%s passato con la chiave %s %s", e.ID, k.Chiave, k.Stato)
			}
		}
	}
}

// TestCasoFallitoDiceAttesoEOttenuto: un valore atteso diverso da quello letto fa fallire il caso, e l'esito
// dice atteso, ottenuto e la regola che ha letto (famiglia e forma).
func TestCasoFallitoDiceAttesoEOttenuto(t *testing.T) {
	a := attesiACME(t, mutazioni{"attesi_acme.yaml": func(s string) string {
		return strings.Replace(s, "      base: 9123456\n      marcatore: A\n      revisione: \"2\"", "      base: 9123457\n      marcatore: A\n      revisione: \"2\"", 1)
	}})
	e := perID(EseguiCasiContratto(a, insiemeACME(t), profiliACME))["caso-acme-01"]
	if e.Esito != CasoFallito {
		t.Fatalf("esito %q", e.Esito)
	}
	k := chiave(e, "base")
	if k.Stato != ChiaveFallita || k.Atteso != "9123457" || k.Ottenuto != "9123456" {
		t.Fatalf("chiave base: %+v", k)
	}
	if len(e.Letture) != 1 || e.Letture[0].Famiglia != "acme-marcatore" || e.Letture[0].Forma != "nome-sottolineato" {
		t.Fatalf("letture nel rapporto: %+v", e.Letture)
	}
}

// TestCasoNonEseguibile: profilo senza cliente nel manifest, cliente senza motore, contesto che non si legge.
// Il caso fallisce con il motivo; non si salta.
func TestCasoNonEseguibile(t *testing.T) {
	ins := insiemeACME(t)
	a := attesiACME(t, nil)
	for _, e := range EseguiCasiContratto(a, ins, map[string]uuid.UUID{}) {
		if e.Esito != CasoFallito || !strings.Contains(e.Motivo, "profilo") {
			t.Fatalf("senza profili: %+v", e)
		}
	}

	ind, contenuti := regoleACME(t, nil)
	contenuti["acme.v1.json"] = append(append([]byte(nil), contenuti["acme.v1.json"]...), ' ')
	scartato, _, err := motorea.CompilaInsieme(ind, contenuti)
	if err != nil {
		t.Fatal(err)
	}
	e := EseguiCasiContratto(a, scartato, profiliACME)[0]
	if e.Esito != CasoFallito || !strings.Contains(e.Motivo, "scartata") || len(e.Diagnostiche) == 0 ||
		e.Diagnostiche[0].Codice != motorea.CodiceRegoleSha256Discorde {
		t.Fatalf("cliente scartato: %+v", e)
	}

	a.Casi = []CasoContratto{{ID: "caso-acme-x", Profilo: "acme", StatoAtteso: StatoAttesoDefinito, Contesto: "pagina.codice", Testo: "9123456"}}
	if e := EseguiCasiContratto(a, ins, profiliACME)[0]; e.Esito != CasoFallito || !strings.Contains(e.Motivo, "contesto") {
		t.Fatalf("contesto illeggibile: %+v", e)
	}
}

// TestCasiDeterministici: due esecuzioni sugli stessi ingressi danno gli stessi esiti, nello stesso ordine.
func TestCasiDeterministici(t *testing.T) {
	a := attesiACME(t, nil)
	uno := EseguiCasiContratto(a, insiemeACME(t), profiliACME)
	due := EseguiCasiContratto(a, insiemeACME(t), profiliACME)
	if !reflect.DeepEqual(uno, due) {
		t.Fatal("due esecuzioni danno esiti diversi")
	}
}
