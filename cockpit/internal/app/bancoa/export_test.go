package bancoa

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/dataset"
)

// L1 — A1c-L1-22: FotografiaDaExport sugli export sintetici con la stessa forma di quelli veri ({query: righe}, jsonb
// come stringa, istanti ISO al millisecondo, NULL, un file senza l'ora nel nome): Coerente falso, la terna e lo schema
// del manifest, le sezioni filtrate, parziali e assenti dichiarate (A1c.md §3.4; T-12), l'HTML «non esportato» diverso
// da «senza HTML», le proposte senza dettagli né deciso_da, i fatti a un'altra terna in diagnosi, MotivoParziale nil,
// uno sha256 sbagliato rifiutato, il triage di un'altra fonte escluso (R32 a; v3 §4.1). La fotografia passa
// ValidaFotografia, e due letture danno la stessa impronta.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

// fotoDellaScena: la fotografia degli export di una scena, con i messaggi fuori RFQ del caso di censimento.
func fotoDellaScena(t *testing.T, s scenaBanco) fotorfq.Fotografia {
	t.Helper()
	raw, err := os.ReadFile(s.manifest)
	if err != nil {
		t.Fatal(err)
	}
	m, err := dataset.Leggi(raw, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	f, mancanti, err := fotografiaDaExport(os.DirFS(s.export), m, []uuid.UUID{msgFuori1, msgFuori2, msgNonEsportato})
	if err != nil {
		t.Fatal(err)
	}
	if len(mancanti) != 1 || mancanti[0] != msgNonEsportato {
		t.Fatalf("il messaggio di un caso che gli export non hanno è fra i mancanti, e la fotografia c'è: %v", mancanti)
	}
	return f
}

func TestFotografiaDaExportSezioniEFonte(t *testing.T) {
	s := preparaBanco(t, mutaBanco{})
	f := fotoDellaScena(t, s)

	if f.Origine != fotorfq.OrigineExport || f.Coerente || f.SchemaDB != 21 || !f.PresaIl.IsZero() {
		t.Fatalf("testata della fotografia: origine %q, coerente %t, schema %d, presa %v", f.Origine, f.Coerente, f.SchemaDB, f.PresaIl)
	}
	if f.Analizzatore == nil || f.Analizzatore.Versione != versioneTernaACME || f.Analizzatore.HashConfigurazione != hashTernaACME {
		t.Fatalf("la terna è quella del manifest: %+v", f.Analizzatore)
	}
	for k, stato := range map[string]string{
		fotorfq.SezioneMessaggi: fotorfq.StatoSezioneParziale, fotorfq.SezioneAllegati: fotorfq.StatoSezioneParziale,
		fotorfq.SezioneFatti: fotorfq.StatoSezioneParziale, fotorfq.SezioneProposteDocumento: fotorfq.StatoSezioneParziale,
		fotorfq.SezioneDocumenti: fotorfq.StatoSezioneParziale, fotorfq.SezioneIdentificativi: fotorfq.StatoSezioneCompleta,
		fotorfq.SezioneComponenti: fotorfq.StatoSezioneAssente, fotorfq.SezioneStepProdotto: fotorfq.StatoSezioneAssente,
		fotorfq.SezioneCandidatiCodice: fotorfq.StatoSezioneAssente, fotorfq.SezioneTriage: fotorfq.StatoSezioneFiltrata,
		fotorfq.SezioneAgganci: fotorfq.StatoSezioneCompleta,
	} {
		if f.Sezioni[k].Stato != stato {
			t.Errorf("sezione %s: %q, attesa %q", k, f.Sezioni[k].Stato, stato)
		}
	}
	if m := f.Sezioni[fotorfq.SezioneMessaggi].Motivo; !strings.Contains(m, "non esportato") || !strings.Contains(m, "canale") {
		t.Errorf("i messaggi dichiarano l'HTML «non esportato» e le colonne che mancano: %q", m)
	}
	if m := f.Sezioni[fotorfq.SezioneProposteDocumento].Motivo; !strings.Contains(m, "dettagli") || !strings.Contains(m, "deciso_da") {
		t.Errorf("le proposte dichiarano che cosa manca: %q", m)
	}
	if len(f.Thread) != 2 || len(f.FuoriRFQ) != 2 || len(f.Clienti) != 1 {
		t.Fatalf("thread %d, fuori RFQ %d, clienti %d", len(f.Thread), len(f.FuoriRFQ), len(f.Clienti))
	}
	if d := fotorfq.ValidaFotografia(f); len(d) > 0 {
		t.Fatalf("la fotografia degli export non passa ValidaFotografia: %+v", d)
	}

	var scen, dec *fotorfq.Thread
	for i := range f.Thread {
		switch f.Thread[i].ID {
		case threadScenario:
			scen = &f.Thread[i]
		case threadDecisioni:
			dec = &f.Thread[i]
		}
	}
	if scen == nil || dec == nil {
		t.Fatal("thread della scena mancanti")
	}
	// L'HTML c'è solo per il messaggio con la tabella; gli altri non hanno HTML, e la sezione dice «non esportato».
	if m := scen.Messaggi[0]; m.CorpoHTML == nil || m.Canale != "" || m.DataEvento.Location().String() != "UTC" {
		t.Errorf("messaggio dello scenario: html %v, canale %q (non esportato), zona %v", m.CorpoHTML, m.Canale, m.DataEvento.Location())
	}
	if m := dec.Messaggi[0]; m.CorpoHTML != nil {
		t.Errorf("un corpo senza tabella non ha HTML negli export: %q", *m.CorpoHTML)
	}
	if len(scen.Agganci) != 1 || scen.Agganci[0].Aggancio != "operatore" {
		t.Errorf("il gesto 1 dalle colonne del messaggio: %+v", scen.Agganci)
	}
	// Le proposte: senza dettagli né deciso_da; quella con l'allegato fuori dagli export non entra, con la diagnosi.
	if len(scen.Proposte) != 1 || scen.Proposte[0].Dettagli != nil || scen.Proposte[0].DecisoDa != nil {
		t.Errorf("proposte dello scenario: %+v", scen.Proposte)
	}
	if len(dec.Proposte) != 2 || dec.Proposte[0].DecisoIl == nil || dec.Proposte[0].DecisoIl.Format("2006-01-02T15:04:05.000Z07:00") != istante1 {
		t.Errorf("proposte decise, al millisecondo in UTC: %+v", dec.Proposte)
	}
	// I documenti: una riga per provenienza, raccolta per documento; la provenienza fuori dal thread non entra.
	if len(dec.Documenti) != 2 {
		t.Fatalf("documenti %d", len(dec.Documenti))
	}
	for _, d := range dec.Documenti {
		if len(d.Allegati) != 1 {
			t.Errorf("documento %s: provenienze %v (solo gli allegati del thread)", d.ID, d.Allegati)
		}
	}
	// I fatti: alla terna del manifest entrano, con il payload come i byte della stringa jsonb e il digest; a un'altra
	// terna no, e la fotografia lo dice. MotivoParziale resta nil (R32 b).
	x, ok := scen.Fatti[shaACME(0x104)]
	if !ok || string(x.Payload) != `{"archivio": {"voci": []}}` || x.MotivoParziale != nil || len(x.Digest) != 64 {
		t.Fatalf("fatti alla terna del manifest: %+v", x)
	}
	if _, ok := scen.Fatti[shaACME(0x103)]; ok {
		t.Error("i fatti a un'altra terna non entrano")
	}
	codici := map[string]int{}
	for _, d := range f.Diagnostiche {
		codici[d.Codice]++
	}
	if codici[fotorfq.CodiceFattiAssenti] != 1 || codici[fotorfq.CodiceSezioneParziale] == 0 {
		t.Errorf("diagnostiche della fotografia: %v", codici)
	}
	// L'allegato di un messaggio non esportato non è attribuibile: la sezione lo conta, e la proposta e la provenienza
	// che lo citano non entrano, con la diagnosi.
	if m := f.Sezioni[fotorfq.SezioneAllegati].Motivo; !strings.Contains(m, "1 non attribuibili") {
		t.Errorf("allegati non attribuibili: %q", m)
	}
	var messaggi []string
	for _, d := range f.Diagnostiche {
		messaggi = append(messaggi, d.Messaggio)
	}
	if testo := strings.Join(messaggi, "\n"); !strings.Contains(testo, "1 proposte con l'allegato fuori") || !strings.Contains(testo, "1 provenienze con l'allegato fuori") {
		t.Errorf("le proposte e le provenienze non attribuibili:\n%s", testo)
	}
	// Il triage di un'altra fonte non entra (MOTORE-SENZA-LLM): la sezione è filtrata.
	if len(scen.Triage) != 1 || len(scen.Triage[0].Identificativi) != 1 || scen.Triage[0].Identificativi[0] != "ACME26-030" {
		t.Errorf("triage: %+v", scen.Triage)
	}
	// Due letture, la stessa impronta.
	h1, err1 := fotorfq.ImprontaFotografia(f)
	h2, err2 := fotorfq.ImprontaFotografia(fotoDellaScena(t, s))
	if err1 != nil || err2 != nil || h1 != h2 || len(h1) != 64 {
		t.Fatalf("impronte %q %q (%v %v)", h1, h2, err1, err2)
	}
}

func TestFotografiaDaExportFileNonDelManifest(t *testing.T) {
	leggi := func(s scenaBanco) error {
		raw, err := os.ReadFile(s.manifest)
		if err != nil {
			t.Fatal(err)
		}
		m, err := dataset.Leggi(raw, s.dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = FotografiaDaExport(os.DirFS(s.export), m)
		return err
	}

	// Uno sha256 sbagliato: il file è cambiato dopo il manifest.
	s := preparaBanco(t, mutaBanco{})
	p := s.export + "/" + nomeFileExport(ExportProposte)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, append(b, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := leggi(s); !errors.Is(err, dataset.ErrImprontaDiversa) {
		t.Errorf("sha256 sbagliato: %v", err)
	}

	// Un file che manca, anche quello senza l'ora nel nome.
	s = preparaBanco(t, mutaBanco{})
	if err := os.Remove(s.export + "/" + nomeFileExport(ExportDocumenti)); err != nil {
		t.Fatal(err)
	}
	if err := leggi(s); !errors.Is(err, dataset.ErrFileMancante) {
		t.Errorf("file mancante: %v", err)
	}

	// Una voce obbligatoria che il manifest non ha.
	s = preparaBanco(t, mutaBanco{testi: map[string]func(string) string{"manifest": func(m string) string {
		return strings.Replace(m, `"nome": "export.thread"`, `"nome": "export.thread_vecchio"`, 1)
	}}})
	if err := leggi(s); !errors.Is(err, dataset.ErrFileMancante) {
		t.Errorf("voce export.<sezione> sconosciuta o mancante: %v", err)
	}

	// Un export con l'impronta giusta e la forma sbagliata: non è un NON ESEGUITO, è un export non valido.
	s = preparaBanco(t, mutaBanco{righe: func(r map[string][]riga) {
		delete(r[ExportAllegati][0], "nome_file")
	}})
	if err := leggi(s); !errors.Is(err, ErrExportNonValido) || !strings.Contains(err.Error(), "nome_file") {
		t.Errorf("colonna obbligatoria mancante: %v", err)
	}
	s = preparaBanco(t, mutaBanco{testi: map[string]func(string) string{"export." + ExportThread: func(string) string {
		return `{"a": [], "b": []}`
	}}})
	if err := leggi(s); !errors.Is(err, ErrExportNonValido) {
		t.Errorf("due query in un export: %v", err)
	}
}

// TestFotografiaDaExportSenzaSezioniFacoltative: senza le voci facoltative (HTML, fatti, proposte, documenti), le
// sezioni sono assenti, mai vuote per difetto (T-12), e l'HTML non esportato lo dice la sezione dei messaggi.
func TestFotografiaDaExportSenzaSezioniFacoltative(t *testing.T) {
	s := preparaBanco(t, mutaBanco{righe: func(r map[string][]riga) {
		for _, sez := range []string{ExportHTML, ExportFatti, ExportProposte, ExportDocumenti, ExportTriage} {
			delete(r, sez)
		}
	}})
	f := fotoDellaScena(t, s)
	for _, k := range []string{fotorfq.SezioneFatti, fotorfq.SezioneProposteDocumento, fotorfq.SezioneDocumenti,
		fotorfq.SezioneProvenienze, fotorfq.SezioneTriage} {
		if f.Sezioni[k].Stato != fotorfq.StatoSezioneAssente {
			t.Errorf("sezione %s: %q, attesa assente", k, f.Sezioni[k].Stato)
		}
	}
	if m := f.Sezioni[fotorfq.SezioneMessaggi].Motivo; !strings.Contains(m, "l'HTML non è esportato") {
		t.Errorf("motivo dei messaggi: %q", m)
	}
}

func TestArrayPostgres(t *testing.T) {
	casi := map[string][]string{`{}`: {}, `{A,B}`: {"A", "B"}, `{"A B",C}`: {"A B", "C"}, `{"a\"b"}`: {`a"b`}}
	for in, atteso := range casi {
		got, err := arrayPostgres(in)
		if err != nil || strings.Join(got, "|") != strings.Join(atteso, "|") || len(got) != len(atteso) {
			t.Errorf("%s → %q (%v)", in, got, err)
		}
	}
	if _, err := arrayPostgres("A,B"); err == nil {
		t.Error("un text[] senza graffe accettato")
	}
}
