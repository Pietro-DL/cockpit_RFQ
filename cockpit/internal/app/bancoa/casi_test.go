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

// L1 — i casi_contratto con DaTesto più Interpreta e l'uso sconosciuto (A1a-BA, A1b-24; R24 a, R25, R19 a,
// R52 A; 5.4.5): esiti passato, parziale, fallito, riservato e rimandato sui quattordici casi ACME di
// testdata/attesi_acme.yaml; un caso definito con dipende_da si valuta e il rapporto lo annota; un caso con
// stato_atteso riservato non si valuta; un predicato su una forma riservata dà «riservato»; figlio_step diventa
// nodo_step; la funzione del router e gli attributi nel rapporto; le precondizioni della revisione in campo
// separato; profilo senza cliente, cliente scartato, contesto illeggibile e testo che DaTesto rifiuta fanno
// fallire il caso; due esecuzioni danno gli stessi esiti. Qui ci sono anche gli aiuti che le altre prove del
// pacchetto usano (il dataset ACME in t.TempDir()).
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

// TestCasiACMEEsiti: i quattordici casi ACME, uno per meccanismo. Da A1b.11 (A1b-24) le chiavi del router e
// degli attributi si valutano: il parziale di A1a sul ripiego generico, la menzione nel testo del PDF e la
// revisione in campo separato con le precondizioni passano; restano un parziale e un rimandato solo per una
// chiave decaduta.
func TestCasiACMEEsiti(t *testing.T) {
	es := EseguiCasiContratto(attesiACME(t, nil), insiemeACME(t), profiliACME)
	if len(es) != 14 {
		t.Fatalf("%d esiti, attesi 14", len(es))
	}
	attesi := map[string]string{
		"caso-acme-01": CasoPassato, "caso-acme-02": CasoPassato, "caso-acme-03": CasoPassato,
		"caso-acme-04": CasoPassato, "caso-acme-05": CasoPassato, "caso-acme-06": CasoPassato,
		"caso-acme-07": CasoRiservato, "caso-acme-08": CasoRiservato, "caso-acme-09": CasoPassato,
		"caso-acme-10": CasoPassato, "caso-acme-11": CasoPassato, "caso-acme-12": CasoPassato,
		"caso-acme-13": CasoParziale, "caso-acme-14": CasoRimandato,
	}
	got := perID(es)
	for id, esito := range attesi {
		if got[id].Esito != esito {
			t.Errorf("%s: esito %q, atteso %q (%s; chiavi %+v)", id, got[id].Esito, esito, got[id].Motivo, got[id].Chiavi)
		}
	}
	if n := Conta(es); n != (ConteggiCasi{Totale: 14, Passati: 10, Parziali: 1, Riservati: 2, Rimandati: 1}) {
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
	// Il ripiego generico che non promuove (R25 e) si valuta in A1b; la lettura del corpo con l'uso sconosciuto è
	// una richiesta, e conta fra le letture d'identità (R25 a; 5.4.5).
	p := got["caso-acme-06"]
	if k := chiave(p, "fallback_generico_non_promuove"); k.Stato != ChiavePassata || k.Sessione != SessioneA1b {
		t.Errorf("ripiego generico: %+v", k)
	}
	if k := chiave(p, "letture_identita"); k.Stato != ChiavePassata || k.Ottenuto != "1" {
		t.Errorf("richiesta fra le letture d'identità: %+v", k)
	}
	if len(p.Letture) != 1 || p.Letture[0].Funzione != string(motorea.FunzRichiesta) {
		t.Errorf("funzione nel rapporto: %+v", p.Letture)
	}
	// Il parziale: la chiave controllata passa, quella decaduta è rimandata con la sessione.
	pz := got["caso-acme-13"]
	if k := chiave(pz, "confronto_legacy"); k.Stato != ChiaveRimandata || k.Sessione != SessioneDecaduta {
		t.Errorf("chiave decaduta: %+v", k)
	}
	if k := chiave(pz, "base"); k.Stato != ChiavePassata {
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
	// La menzione nel testo del PDF (router-1 riga 16): fuori dall'insieme d'identità, letta da base_menzionata.
	m := got["caso-acme-09"]
	if k := chiave(m, "letture_identita"); k.Stato != ChiavePassata || !strings.Contains(k.Ottenuto, "menzione") {
		t.Errorf("menzione fuori dall'insieme: %+v", k)
	}
	if len(m.Letture) != 1 || m.Letture[0].Funzione != string(motorea.FunzMenzione) {
		t.Errorf("funzione nel rapporto: %+v", m.Letture)
	}
	// L'attributo della revisione in campo separato (5.4.6 punto 12): l'unica regola attiva sul selettore, con le
	// precondizioni verificate sul codice della stessa entità.
	r10 := got["caso-acme-10"]
	if k := chiave(r10, chiavePrecondizioni); k.Stato != ChiavePassata || k.Sessione != SessioneA1b ||
		!strings.Contains(k.Ottenuto, "acme-campo/rev-campo") {
		t.Errorf("precondizioni: %+v", k)
	}
	if len(r10.Attributi) != 1 || r10.Attributi[0].Stato != motorea.StatoAttribuito || r10.Attributi[0].Normalizzato != "01" {
		t.Errorf("attributi nel rapporto: %+v", r10.Attributi)
	}
	// Un valore che la regola non legge: non_interpretabile, l'originale resta, nessun valore (C-34, A-C09).
	r11 := got["caso-acme-11"]
	if k := chiave(r11, "revisione"); k.Ottenuto != "null" {
		t.Errorf("revisione non interpretabile: %+v", k)
	}
	if len(r11.Attributi) != 1 || r11.Attributi[0].Grezzo != "VEDI NOTA 00" || r11.Attributi[0].Normalizzato != "" {
		t.Errorf("originale conservato: %+v", r11.Attributi)
	}
	// Nessuna chiave resta rimandata se Interpreta sa rispondere: solo le decadute.
	for _, e := range es {
		for _, k := range e.Chiavi {
			if k.Stato == ChiaveRimandata && k.Sessione != SessioneDecaduta {
				t.Errorf("%s: chiave %s rimandata: %+v", e.ID, k.Chiave, k)
			}
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

// casoPrecondizioni: un caso sulla revisione in campo separato «01», con le precondizioni date.
func casoPrecondizioni(pc *Precondizioni) Attesi {
	return Attesi{Casi: []CasoContratto{{ID: "caso-acme-p", Profilo: "acme", StatoAtteso: StatoAttesoDefinito,
		Contesto: "cartiglio.revisione", Testo: "01", Precondizioni: pc,
		Atteso: []ChiaveAttesa{{Chiave: "revisione", Valore: ValoreAtteso{Tipo: TipoStringa, Testo: "01"}}}}}}
}

// TestPrecondizioniDellaRevisione (A-C02; 5.4.6 punto 12): DaTesto non costruisce l'entità condivisa con il
// codice; il banco verifica che la revisione venga da una regola della famiglia che legge il codice della
// precondizione sul campo del codice della stessa entità. Senza entità condivisa non c'è niente da verificare;
// una base che non si legge da sola resta rimandata, mai passata; una revisione di un'altra famiglia fallisce.
func TestPrecondizioniDellaRevisione(t *testing.T) {
	ins := insiemeACME(t)
	for _, x := range []struct {
		nome  string
		pc    Precondizioni
		stato string
		esito string
		dice  string
	}{
		{"il codice della stessa famiglia", Precondizioni{BaseStrutturata: "Q+700.099999.010", EntitaCondivisaConCodice: true},
			ChiavePassata, CasoPassato, "acme-campo/rev-campo"},
		{"nessuna entità condivisa", Precondizioni{EntitaCondivisaConCodice: false}, ChiavePassata, CasoPassato, "nessun codice"},
		{"una base che non si legge da sola sul codice del cartiglio", Precondizioni{BaseStrutturata: "9123456", EntitaCondivisaConCodice: true},
			ChiaveRimandata, CasoParziale, ""},
		{"entità condivisa senza base", Precondizioni{EntitaCondivisaConCodice: true}, ChiaveFallita, CasoFallito, ""},
	} {
		t.Run(x.nome, func(t *testing.T) {
			pc := x.pc
			e := EseguiCasiContratto(casoPrecondizioni(&pc), ins, profiliACME)[0]
			k := chiave(e, chiavePrecondizioni)
			if k.Stato != x.stato || e.Esito != x.esito || !strings.Contains(k.Ottenuto, x.dice) || k.Sessione != SessioneA1b {
				t.Fatalf("esito %q, precondizioni %+v", e.Esito, k)
			}
		})
	}

	// Una famiglia che legge da sola la base sul codice del cartiglio, ma la revisione viene dalla regola di
	// un'altra famiglia: nell'entità vera la regola sarebbe un'altra (o nessuna).
	ind, contenuti := regoleACME(t, mutazioni{"regole/acme.v1.json": func(s string) string {
		return strings.Replace(s, "\"id\": \"pacchetto\",\n          \"selettori\": [\n            \"nome_file\"\n",
			"\"id\": \"pacchetto\",\n          \"selettori\": [\n            \"nome_file\",\n            \"cartiglio.codice\"\n", 1)
	}})
	altra, _, err := motorea.CompilaInsieme(ind, contenuti)
	if err != nil || altra.Motori[clienteACME] == nil {
		t.Fatalf("grammatica con la forma pacchetto sul cartiglio: %v %v", err, altra.Scartati[clienteACME])
	}
	e := EseguiCasiContratto(casoPrecondizioni(&Precondizioni{BaseStrutturata: "9123456", EntitaCondivisaConCodice: true}), altra, profiliACME)[0]
	if k := chiave(e, chiavePrecondizioni); k.Stato != ChiaveFallita || e.Esito != CasoFallito ||
		!strings.Contains(k.Ottenuto, "acme-marcatore") || !strings.Contains(k.Ottenuto, "acme-campo/rev-campo") {
		t.Fatalf("revisione di un'altra famiglia: %q %+v", e.Esito, k)
	}
}

// TestTestoCheDaTestoRifiuta: un testo che non è UTF-8 non diventa un documento; il caso fallisce con il motivo,
// non si salta.
func TestTestoCheDaTestoRifiuta(t *testing.T) {
	a := Attesi{Casi: []CasoContratto{{ID: "caso-acme-u", Profilo: "acme", StatoAtteso: StatoAttesoDefinito,
		Contesto: "nome_file", Testo: "9123456A_2\xff.pdf",
		Atteso: []ChiaveAttesa{{Chiave: "base", Valore: ValoreAtteso{Tipo: TipoIntero, Testo: "9123456"}}}}}}
	e := EseguiCasiContratto(a, insiemeACME(t), profiliACME)[0]
	if e.Esito != CasoFallito || !strings.Contains(e.Motivo, "DaTesto") || chiave(e, "base").Stato != ChiaveFallita {
		t.Fatalf("testo non UTF-8: %+v", e)
	}
}

// TestUnaInterpretazioneParzialeNonSiGiudica (5.4.6 punto 16; A1b-22): un testo oltre max_byte_unita dà
// un'interpretazione parziale; anche se le chiavi tornano, il caso fallisce con il motivo, mai un passato su un
// risultato tagliato.
func TestUnaInterpretazioneParzialeNonSiGiudica(t *testing.T) {
	testo := "PN 7654321" + strings.Repeat(" ", 1<<20)
	a := Attesi{Casi: []CasoContratto{{ID: "caso-acme-lungo", Profilo: "acme", StatoAtteso: StatoAttesoDefinito,
		Contesto: "corpo", Testo: testo,
		Atteso: []ChiaveAttesa{{Chiave: "letture_identita", Valore: ValoreAtteso{Tipo: TipoIntero, Testo: "1"}}}}}}
	e := EseguiCasiContratto(a, insiemeACME(t), profiliACME)[0]
	if e.Esito != CasoFallito || !strings.Contains(e.Motivo, "parziale") {
		t.Fatalf("interpretazione parziale: esito %q, motivo %q, chiavi %+v", e.Esito, e.Motivo, e.Chiavi)
	}
	if k := chiave(e, "letture_identita"); k.Stato != ChiavePassata {
		t.Errorf("la chiave resta nel rapporto come informazione: %+v", k)
	}
}

// TestLePrecondizioniDaSoleNonFannoPassare: un caso senza chiavi dell'atteso, con le sole precondizioni che
// passano, non controlla niente: rimandato, mai passato.
func TestLePrecondizioniDaSoleNonFannoPassare(t *testing.T) {
	a := casoPrecondizioni(&Precondizioni{EntitaCondivisaConCodice: false})
	a.Casi[0].Atteso = nil
	e := EseguiCasiContratto(a, insiemeACME(t), profiliACME)[0]
	if e.Esito != CasoRimandato {
		t.Fatalf("solo le precondizioni: esito %q, chiavi %+v", e.Esito, e.Chiavi)
	}
}
