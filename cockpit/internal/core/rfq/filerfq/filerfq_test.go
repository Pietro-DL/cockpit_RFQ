package filerfq

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/filerfq/conservatoreprova"
	"promatec/cockpit/internal/platform/db"
)

// L1 — la logica pura del ciclo dei file (V1, EM §10): il corpo dei gesti, il perimetro, l'elenco delle dipendenze e
// l'ordine dei controlli, i testi degli stati della rimozione, le attese dei ritentativi, il contenimento del deposito,
// nessun percorso nelle risposte.

const chiaveV4 = "00000000-0000-4000-8000-0000000c0001"

func corpo(t *testing.T, s string, gesto string) (CorpoGesto, *Rifiuto) {
	t.Helper()
	return LeggiCorpo(strings.NewReader(s), gesto)
}

func TestLeggiCorpoForma(t *testing.T) {
	imp := strings.Repeat("a", 64)
	ok := `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"doppione","impronta_dipendenze":"` + imp + `"}`
	if _, rf := corpo(t, ok, GestoScarta); rf != nil {
		t.Fatalf("corpo valido rifiutato: %v", rf)
	}
	casi := []struct {
		nome, corpo, gesto, codice, campo string
		stato                             int
	}{
		{"un campo percorso (D23:234)", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"x","impronta_dipendenze":"` + imp + `","percorso":"C:\\x"}`, GestoScarta, "CORPO_NON_VALIDO", "percorso", 422},
		{"path_staging", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"x","impronta_dipendenze":"` + imp + `","path_staging":"x"}`, GestoScarta, "CORPO_NON_VALIDO", "path_staging", 422},
		{"radice", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","impronta_dipendenze":"` + imp + `","radice":"x"}`, GestoAltriFile, "CORPO_NON_VALIDO", "radice", 422},
		{"senza versione attesa", `{"chiave_idempotenza":"` + chiaveV4 + `","motivo":"x","impronta_dipendenze":"` + imp + `"}`, GestoScarta, "CORPO_NON_VALIDO", "versione_attesa", 422},
		{"chiave non v4", `{"versione_attesa":0,"chiave_idempotenza":"00000000-0000-1000-8000-000000000001","motivo":"x","impronta_dipendenze":"` + imp + `"}`, GestoScarta, "CORPO_NON_VALIDO", "chiave_idempotenza", 422},
		{"scarta senza motivo", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","impronta_dipendenze":"` + imp + `"}`, GestoScarta, "MOTIVO_OBBLIGATORIO", "motivo", 422},
		{"scarta con il motivo di soli spazi", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"   ","impronta_dipendenze":"` + imp + `"}`, GestoScarta, "MOTIVO_OBBLIGATORIO", "motivo", 422},
		{"motivo oltre 1000 caratteri", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"` + strings.Repeat("è", 1001) + `","impronta_dipendenze":"` + imp + `"}`, GestoScarta, "CORPO_NON_VALIDO", "motivo", 422},
		{"senza impronta delle dipendenze", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"x"}`, GestoScarta, "CORPO_NON_VALIDO", "impronta_dipendenze", 422},
		{"riattiva con l'impronta delle dipendenze", `{"versione_attesa":1,"chiave_idempotenza":"` + chiaveV4 + `","impronta_dipendenze":"` + imp + `"}`, GestoRiattiva, "CORPO_NON_VALIDO", "impronta_dipendenze", 422},
		{"n_voci senza impronta_voci", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","motivo":"x","impronta_dipendenze":"` + imp + `","n_voci":3}`, GestoScarta, "CORPO_NON_VALIDO", "n_voci", 422},
		{"conferma dell'unica copia in Altri file", `{"versione_attesa":0,"chiave_idempotenza":"` + chiaveV4 + `","impronta_dipendenze":"` + imp + `","conferma_unica_copia":true}`, GestoAltriFile, "CORPO_NON_VALIDO", "conferma_unica_copia", 422},
		{"due valori JSON", ok + ok, GestoScarta, "CORPO_NON_VALIDO", "", 422},
		{"oltre 64 KiB", `{"versione_attesa":0,"motivo":"` + strings.Repeat("x", MaxCorpo) + `"}`, GestoScarta, "TROPPO_GRANDE", "", 413},
	}
	for _, c := range casi {
		_, rf := corpo(t, c.corpo, c.gesto)
		if rf == nil {
			t.Errorf("%s: accettato", c.nome)
			continue
		}
		if rf.Codice != c.codice || rf.Stato != c.stato {
			t.Errorf("%s: %d %s, atteso %d %s", c.nome, rf.Stato, rf.Codice, c.stato, c.codice)
		}
		if c.campo != "" && rf.Extra["campo"] != c.campo {
			t.Errorf("%s: campo %v, atteso %s", c.nome, rf.Extra["campo"], c.campo)
		}
	}
}

func TestLeggiCorpoMotivoInNFC(t *testing.T) {
	// «e» + U+0301 diventa «é»: il motivo e' un testo umano (EM §3.10)
	c, rf := corpo(t, `{"versione_attesa":0,"chiave_idempotenza":"`+chiaveV4+`","motivo":"  caffe\u0301  "}`, GestoRiattiva)
	if rf != nil {
		t.Fatal(rf)
	}
	if c.motivo() != "caffé" {
		t.Errorf("motivo %q, atteso in NFC e senza spazi ai lati", c.motivo())
	}
}

// TestImprontaDelCorpo: la chiave non entra; il gesto, la RFQ e l'allegato del percorso si' (giro 1, SIC-07: due file
// con lo stesso corpo non hanno la stessa impronta).
func TestImprontaDelCorpo(t *testing.T) {
	imp := strings.Repeat("b", 64)
	c1, _ := corpo(t, `{"versione_attesa":0,"chiave_idempotenza":"`+chiaveV4+`","motivo":"x","impronta_dipendenze":"`+imp+`"}`, GestoScarta)
	c2, _ := corpo(t, `{"versione_attesa":0,"chiave_idempotenza":"00000000-0000-4000-8000-0000000c0002","motivo":"x","impronta_dipendenze":"`+imp+`"}`, GestoScarta)
	th, a, b := uuid.New(), uuid.New(), uuid.New()
	i1, _ := improntaCorpo(GestoScarta, th, a, c1)
	i2, _ := improntaCorpo(GestoScarta, th, a, c2)
	i3, _ := improntaCorpo(GestoScarta, th, b, c1)
	i4, _ := improntaCorpo(GestoAltriFile, th, a, c1)
	if i1 != i2 {
		t.Error("la chiave cambia l'impronta del corpo")
	}
	if i1 == i3 || i1 == i4 {
		t.Error("un altro allegato o un altro gesto danno la stessa impronta")
	}
}

func occTest(id uuid.UUID, stato string) Occorrenza {
	return Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: id, Stato: stato, NomeFile: "voce.pdf"}}
}

func TestPerimetroImprontaStabile(t *testing.T) {
	a, b := uuid.MustParse("00000000-0000-4000-8000-00000000000b"), uuid.MustParse("00000000-0000-4000-8000-00000000000a")
	p1 := Perimetro{Archivio: true, Voci: []Occorrenza{occTest(a, StatoAttivo), occTest(b, StatoAttivo)}}
	p2 := Perimetro{Archivio: true, Voci: []Occorrenza{occTest(b, StatoAttivo), occTest(a, StatoAttivo)}}
	if p1.Impronta() != p2.Impronta() {
		t.Error("l'impronta delle voci dipende dall'ordine")
	}
	vuoto := Perimetro{}
	if vuoto.Impronta() == "" || vuoto.Impronta() == p1.Impronta() {
		t.Error("impronta del perimetro vuoto")
	}
	n, imp := 2, p1.Impronta()
	if rf := controllaPerimetro(p1, &n, &imp); rf != nil {
		t.Errorf("perimetro giusto rifiutato: %v", rf)
	}
	n3 := 3
	if rf := controllaPerimetro(p1, &n3, &imp); rf == nil || rf.Codice != "ANTEPRIMA_SUPERATA" || rf.Extra["anteprima"] != "voci_archivio" {
		t.Errorf("perimetro sbagliato: %v", rf)
	}
	if rf := controllaPerimetro(p1, nil, nil); rf == nil {
		t.Error("un archivio senza perimetro nel corpo passa")
	}
	if rf := controllaPerimetro(Perimetro{}, nil, nil); rf != nil {
		t.Errorf("un file senza voci con il perimetro assente: %v", rf)
	}
}

// TestControllaDipendenzeOrdine: blocchi, poi risoluzioni, poi impronta (EM §10.4, tempo 1 passo 4).
func TestControllaDipendenzeOrdine(t *testing.T) {
	a := uuid.New()
	blocco := Dipendenza{AllegatoID: a, Codice: "D-1", Esito: EsitoBlocca, Testo: "x"}
	scoll := Dipendenza{AllegatoID: a, Codice: "D-7", Esito: EsitoRisoluzione, Risoluzione: RisoluzioneScollega, Testo: "x"}
	unica := Dipendenza{AllegatoID: a, Codice: "D-14", Esito: EsitoRisoluzione, Risoluzione: RisoluzioneUnicaCopia, Testo: "x"}
	mostra := Dipendenza{AllegatoID: a, Codice: "D-11", Esito: EsitoMostra, Testo: "x"}
	el, _ := chiudiElenco([]Dipendenza{scoll, blocco, mostra})
	if rf := controllaDipendenze(el, "diversa", false, false, true, nil); rf == nil || rf.Codice != "DIPENDENZE_ATTIVE" {
		t.Errorf("il blocco non vince: %v", rf)
	}
	el, _ = chiudiElenco([]Dipendenza{scoll, unica, mostra})
	rf := controllaDipendenze(el, "diversa", false, false, true, nil)
	if rf == nil || rf.Codice != "RISOLUZIONE_RICHIESTA" {
		t.Fatalf("la risoluzione non e' chiesta: %v", rf)
	}
	if cc, _ := rf.Extra["campi"].([]string); strings.Join(cc, ",") != "conferma_unica_copia,scollega" {
		t.Errorf("campi: %v", rf.Extra["campi"])
	}
	// con Altri file l'unica copia non si chiede (i byte si conservano)
	if rf := controllaDipendenze(el, el.Impronta, true, false, false, nil); rf != nil {
		t.Errorf("Altri file chiede la conferma dell'unica copia: %v", rf)
	}
	if rf := controllaDipendenze(el, "diversa", true, true, true, nil); rf == nil || rf.Codice != "ANTEPRIMA_SUPERATA" || rf.Extra["anteprima"] != "dipendenze_file" {
		t.Errorf("impronta diversa: %v", rf)
	}
	if rf := controllaDipendenze(el, el.Impronta, true, true, true, nil); rf != nil {
		t.Errorf("tutto in ordine: %v", rf)
	}
}

func TestElencoOrdinatoEImprontaDeterministica(t *testing.T) {
	a, b := uuid.MustParse("00000000-0000-4000-8000-0000000000aa"), uuid.MustParse("00000000-0000-4000-8000-0000000000bb")
	x := []Dipendenza{{AllegatoID: b, Codice: "D-1", Esito: EsitoBlocca}, {AllegatoID: a, Codice: "D-9", Esito: EsitoMostra},
		{AllegatoID: a, Codice: "D-4", Esito: EsitoMostra}}
	y := []Dipendenza{x[2], x[0], x[1]}
	e1, _ := chiudiElenco(x)
	e2, _ := chiudiElenco(y)
	if e1.Impronta != e2.Impronta {
		t.Error("l'impronta dipende dall'ordine di calcolo")
	}
	if e1.Voci[0].AllegatoID != a || e1.Voci[0].Codice != "D-4" || e1.Voci[2].AllegatoID != b {
		t.Errorf("ordine: %+v", e1.Voci)
	}
	vuoto, _ := chiudiElenco(nil)
	if vuoto.Voci == nil || vuoto.Impronta == "" {
		t.Error("l'elenco vuoto deve essere [] con la sua impronta")
	}
}

// TestTestiDellaRimozione: i testi esatti di EM §10.4; «File eliminato» solo con eseguita / rimosso (D23:222).
func TestTestiDellaRimozione(t *testing.T) {
	casi := map[[2]string]string{
		{"in_attesa", "da_eseguire"}:                 "Scarto registrato; rimozione dallo staging ancora da completare.",
		{"in_attesa", "lavoro_in_corso"}:             "Scarto registrato; rimozione dallo staging ancora da completare (analisi in corso su questo file).",
		{"in_attesa", "copia_nas_in_attesa"}:         "Scarto registrato; rimozione dallo staging ancora da completare (copia sul NAS in attesa).",
		{"in_attesa", "anomalia_nas"}:                "Scarto registrato; rimozione dallo staging ancora da completare (segnalazione aperta sul NAS).",
		{"in_attesa", "file_giovane"}:                "Scarto registrato; rimozione dallo staging ancora da completare (file appena scritto: si riprova fra qualche minuto).",
		{"in_attesa", "pin_transitorio"}:             "Scarto registrato; rimozione dallo staging ancora da completare (un lavoro in corso usa ancora questo contenuto).",
		{"errore", "file_occupato"}:                  "Scarto registrato; rimozione dallo staging ancora da completare (il file è aperto da un altro programma: si riprova da solo).",
		{"errore", "errore_io"}:                      "Scarto registrato; rimozione dallo staging ancora da completare (il file è aperto da un altro programma: si riprova da solo).",
		{"errore", "fuori_cache"}:                    "Scarto registrato; il file non è stato eliminato: sta fuori dalla cache dello staging. Serve una bonifica dell'amministratore.",
		{"errore", "percorso_non_valido"}:            "Scarto registrato; il file non è stato eliminato: sta fuori dalla cache dello staging. Serve una bonifica dell'amministratore.",
		{"eseguita", "rimosso"}:                      "File scartato ed eliminato dallo staging.",
		{"eseguita", "gia_assente"}:                  "File scartato. Nello staging non c'era una copia da eliminare.",
		{"eseguita", "bonificato"}:                   "File scartato. La copia fuori dalla cache dello staging non c'è più: l'ha tolta la bonifica dell'amministratore.",
		{"non_necessaria_condiviso", "condiviso"}:    "File scartato. La copia nello staging resta perché la usano altri file con lo stesso contenuto.",
		{"non_necessaria_condiviso", "rfq_cambiata"}: "File scartato. Il file ora appartiene a un'altra RFQ: la sua copia nello staging resta.",
	}
	for k, atteso := range casi {
		if got := TestoRimozione(k[0], k[1]); got != atteso {
			t.Errorf("%s/%s:\n %q\natteso\n %q", k[0], k[1], got, atteso)
		}
		if strings.Contains(TestoRimozione(k[0], k[1]), "eliminato dallo staging") != (k == [2]string{"eseguita", "rimosso"}) {
			t.Errorf("%s/%s: «eliminato» fuori da eseguita / rimosso", k[0], k[1])
		}
	}
}

func TestAtteseCrescenti(t *testing.T) {
	attese := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 6 * time.Hour}
	for i, a := range attese {
		if got := attesaDopo(int32(i + 1)); got != a {
			t.Errorf("tentativo %d: %s, atteso %s", i+1, got, a)
		}
	}
}

// TestCopiaNelDepositoPerLaCache (EM §10.5; U2 §3.13): la verifica che la cache usa per non contare il pin di un file in
// Altri file guarda la copia con la dimensione attesa, solo sotto la radice del conservatore; senza conservatore non
// c'e' (il pin resta). Il contenimento dei percorsi lo provano le L1 di storage/nas.
func TestCopiaNelDepositoPerLaCache(t *testing.T) {
	if (&Servizio{}).CopiaNelDeposito() != nil {
		t.Error("senza conservatore la verifica della cache c'e'")
	}
	cons := conservatoreprova.Nuovo(t.TempDir())
	s := &Servizio{Conservatore: cons}
	cartella, buono := `ACME\WIP\2026 10 19 RFQ-L1`, `ALTRO\ACME-7.pdf`
	p := cons.Percorso(cartella, buono)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("%PDF-1.4 prova"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := s.CopiaNelDeposito()
	if !c(cartella, buono, cons.RadiceID(), 14) {
		t.Error("la copia presente non risulta")
	}
	if c(cartella, buono, cons.RadiceID(), 13) || c(cartella, buono, uuid.New(), 14) || c(cartella, `..\fuori.pdf`, cons.RadiceID(), 14) ||
		c(`..\ACME`, buono, cons.RadiceID(), 14) || c("", buono, cons.RadiceID(), 14) {
		t.Error("una copia con un'altra dimensione, di un'altra radice, fuori dalla radice o in un'altra cartella risulta presente")
	}
}

// TestNessunPercorsoNelleRisposte (T-15, L1): il modello di S1 composto da una riga con path_staging e path_interno
// pieni di percorsi del disco, e i testi liberi, non porta lettere di unita' ne' barre rovesciate.
func TestNessunPercorsoNelleRisposte(t *testing.T) {
	s := &Servizio{Staging: `C:\Cockpit\server\staging`}
	r := db.ListFileSorgenteRow{AllegatoID: uuid.New(), MessaggioID: uuid.New(), NomeFile: "ACM-1000.pdf",
		PathInterno: pgtype.Text{String: `cartella\sotto\ACM-1000.pdf`, Valid: true},
		PathStaging: pgtype.Text{String: `C:\Cockpit\server\staging\_contenuti\ab\ab.pdf`, Valid: true},
		Sha256:      pgtype.Text{String: strings.Repeat("a", 64), Valid: true}, Stato: StatoAttivo,
		StatoAllegato: db.StatoAllegatoErrore, Errore: pgtype.Text{String: `file non trovato: C:\Users\x\y.pdf e \\nas\cond\z.pdf`, Valid: true},
		Natura: db.NaturaAllegatoFile, Origine: db.OrigineAllegatoOutlook, Canale: db.CanaleOutlook}
	f := s.fileDaRiga(r, nil, nil, false)
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`[A-Za-z]:\\\\`).Match(b) || strings.Contains(string(b), `\\`) {
		t.Errorf("percorso nella risposta: %s", b)
	}
}

func TestStatoAnalisi(t *testing.T) {
	casi := []struct {
		stato    db.StatoAllegato
		errore   string
		pendenti map[string]bool
		atteso   string
	}{
		{db.StatoAllegatoGrezzo, "", nil, "non_scaricato"},
		{db.StatoAllegatoGrezzo, "in coda", nil, "download_in_coda"},
		{db.StatoAllegatoGrezzo, "", map[string]bool{"stage_allegato": true}, "download_in_coda"},
		{db.StatoAllegatoInStaging, "", nil, "in_staging"},
		{db.StatoAllegatoInStaging, "", map[string]bool{"estrai_archivio": true}, "estrazione_in_coda"},
		{db.StatoAllegatoInStaging, "", map[string]bool{"analizza_allegato": true}, "in_analisi"},
		{db.StatoAllegatoAnalizzato, "", nil, "letto"},
		{db.StatoAllegatoErrore, "x", nil, "errore"},
	}
	for _, c := range casi {
		if got := statoAnalisi(c.stato, c.errore, c.pendenti).Stato; got != c.atteso {
			t.Errorf("%s %q %v: %s, atteso %s", c.stato, c.errore, c.pendenti, got, c.atteso)
		}
	}
}

func TestContestoCitaLOccorrenza(t *testing.T) {
	a := uuid.New()
	o := Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: a, NomeFile: "telaio.stp",
		Sha256: pgtype.Text{String: strings.Repeat("c", 64), Valid: true}}}
	casi := []struct {
		p  OccorrenzaContesto
		si bool
	}{
		{OccorrenzaContesto{AllegatoID: a.String()}, true},
		{OccorrenzaContesto{AllegatoID: uuid.NewString()}, false},
		{OccorrenzaContesto{Sha256: strings.Repeat("c", 64), Nome: "telaio.stp"}, true},
		{OccorrenzaContesto{Sha256: strings.Repeat("c", 64)}, false}, // mai per sola impronta
		{OccorrenzaContesto{Sha256: strings.Repeat("c", 64), Nome: "altro.stp"}, false},
		{OccorrenzaContesto{Sha256: strings.Repeat("c", 64), Nome: "telaio.stp", Contenitore: uuid.NewString()}, false},
	}
	for i, c := range casi {
		if got := citaOccorrenza(Contesto{ID: "ctx", Perimetro: []OccorrenzaContesto{c.p}}, o); got != c.si {
			t.Errorf("caso %d: %v, atteso %v", i, got, c.si)
		}
	}
}

func TestSenzaPercorsi(t *testing.T) {
	in := `remove C:\Cockpit\x\y.pdf: The process cannot access the file; \\server\cond\z e D:/a/b`
	out := SenzaPercorsi(in)
	if strings.Contains(out, `C:\`) || strings.Contains(out, `\\`) || strings.Contains(out, `D:/`) {
		t.Errorf("percorsi rimasti: %q", out)
	}
}
