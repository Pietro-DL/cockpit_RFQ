//go:build integrazione

package fondazioni_test

import (
	"context"
	"strings"
	"testing"

	"promatec/cockpit/internal/config"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/fondazioni"
	"promatec/cockpit/internal/testutil"
	"promatec/cockpit/internal/web"
)

// configDiProva è la configurazione delle quattro caselle simulate su cui poggeranno i test L4 delle
// fasi 1 e 2: una condivisa (Commerciale) e tre personali, due postazioni, tre worker.
func configDiProva() *config.Config {
	c := &config.Config{}
	c.Outlook.CasellaDefault = "francesco@azienda.example"
	c.Utenti = []config.Utente{
		{Sigla: "FP", Nome: "Francesco", Ufficio: "Commerciale", Ruolo: "admin", Password: "prova"},
		{Sigla: "LU", Nome: "Luigi", Ufficio: "Tecnico", Ruolo: "tecnico", Password: "prova"},
		{Sigla: "FI", Nome: "Filippo", Ufficio: "Commerciale", Ruolo: "operatore", Password: "prova"},
	}
	c.Caselle = []config.Casella{
		{Indirizzo: "commerciale@azienda.example", Nome: "Commerciale", Canale: "outlook", Condivisa: true},
		{Indirizzo: "francesco@azienda.example", Nome: "Francesco", Canale: "outlook", Utente: "FP"},
		{Indirizzo: "luigi@azienda.example", Nome: "Luigi", Canale: "outlook", Utente: "LU"},
		{Indirizzo: "filippo@azienda.example", Nome: "Filippo", Canale: "outlook", Utente: "FI"},
	}
	c.Postazioni = []config.Postazione{
		{NomeHost: "PC-FRANCESCO", Utente: "FP", Descrizione: "postazione di prova"},
		{NomeHost: "PC-LUIGI", Utente: "LU"},
	}
	c.Worker = []config.Worker{
		{Nome: "outlook@PC-FRANCESCO", Tipo: "outlook", Token: "segreto-francesco", Postazione: "PC-FRANCESCO",
			Caselle: []string{"francesco@azienda.example", "commerciale@azienda.example"}},
		{Nome: "outlook@PC-LUIGI", Tipo: "outlook", Token: "segreto-luigi", Postazione: "PC-LUIGI",
			Caselle: []string{"luigi@azienda.example"}},
		{Nome: "analisi@PC-FRANCESCO", Tipo: "analisi", Token: "segreto-analisi", Postazione: "PC-FRANCESCO"},
	}
	return c
}

func seminaUtenti(t *testing.T, ctx context.Context, q *db.Queries, cfg *config.Config) {
	t.Helper()
	utenti := make([]struct{ Sigla, Nome, Ufficio, Ruolo, Password string }, 0, len(cfg.Utenti))
	for _, u := range cfg.Utenti {
		utenti = append(utenti, struct{ Sigla, Nome, Ufficio, Ruolo, Password string }{u.Sigla, u.Nome, u.Ufficio, u.Ruolo, u.Password})
	}
	if err := web.SeedUtenti(ctx, q, utenti, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
}

// Il seed porta in DB quello che il file dichiara, e il secondo avvio non cambia nulla.
func TestSeminaEIdempotenza(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)
	cfg := configDiProva()
	seminaUtenti(t, ctx, q, cfg)

	e, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso())
	if err != nil {
		t.Fatalf("semina: %v", err)
	}
	if e.Caselle != 4 || e.Postazioni != 2 || e.Worker != 3 {
		t.Fatalf("attese 4 caselle, 2 postazioni, 3 worker; ottenuti %d/%d/%d", e.Caselle, e.Postazioni, e.Worker)
	}
	if !e.CasellaDefault.Valid {
		t.Fatal("casella predefinita non risolta")
	}
	if len(e.UtentiMancanti) != 0 {
		t.Errorf("sigle utente non risolte: %v", e.UtentiMancanti)
	}

	// La condivisa non ha proprietario; la personale sì.
	com, err := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.example"})
	if err != nil {
		t.Fatal(err)
	}
	if !com.Condivisa || com.UtenteID.Valid {
		t.Errorf("Commerciale: condivisa=%v proprietario=%v; attese true/assente", com.Condivisa, com.UtenteID.Valid)
	}
	fra, err := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "francesco@azienda.example"})
	if err != nil {
		t.Fatal(err)
	}
	if fra.Condivisa || !fra.UtenteID.Valid {
		t.Error("la casella di Francesco dovrebbe essere personale e avere un proprietario")
	}

	// Il token non deve mai finire in chiaro nel database.
	cred, err := q.GetWorkerCredenziale(ctx, "outlook@PC-FRANCESCO")
	if err != nil {
		t.Fatal(err)
	}
	if cred.TokenHash != fondazioni.HashToken("segreto-francesco") {
		t.Error("token_hash non corrisponde all'impronta del token")
	}
	if strings.Contains(cred.TokenHash, "segreto") {
		t.Error("il token è finito in chiaro nel database")
	}
	if len(cred.Caselle) != 2 {
		t.Errorf("il worker di Francesco è autorizzato a %d caselle, attese 2", len(cred.Caselle))
	}
	if !cred.PostazioneID.Valid {
		t.Error("credenziale senza postazione: il routing della fase 2 non funzionerebbe")
	}

	// Secondo avvio: stessi identificativi, nessun duplicato.
	idPrima := fra.CasellaID
	if _, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatalf("seconda semina: %v", err)
	}
	if n := testutil.Conta(t, p, "casella"); n != 4 {
		t.Errorf("dopo il secondo avvio ci sono %d caselle, attese 4", n)
	}
	fra2, _ := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "francesco@azienda.example"})
	if fra2.CasellaID != idPrima {
		t.Error("l'identificativo della casella è cambiato al riavvio: i riferimenti nei job si romperebbero")
	}
}

// Togliere una casella dal file non la disattiva: il seed lo segnala e lascia decidere a una persona.
func TestCasellaTolaDalFileNonVieneDisattivata(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)
	cfg := configDiProva()
	seminaUtenti(t, ctx, q, cfg)
	if _, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}

	ridotta := configDiProva()
	ridotta.Caselle = ridotta.Caselle[:2] // via Luigi e Filippo
	ridotta.Worker = ridotta.Worker[:1]
	e, err := fondazioni.Semina(ctx, q, ridotta, testutil.LogSilenzioso())
	if err != nil {
		t.Fatal(err)
	}
	if n := testutil.Conta(t, p, "casella"); n != 4 {
		t.Errorf("caselle in DB %d, attese 4: il seed ne ha cancellata una", n)
	}
	var attive int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM casella WHERE attiva").Scan(&attive); err != nil {
		t.Fatal(err)
	}
	if attive != 4 {
		t.Errorf("caselle attive %d, attese 4: il seed ha disattivato d'ufficio", attive)
	}
	// due caselle (Luigi, Filippo) e due worker (outlook@PC-LUIGI, analisi@PC-FRANCESCO)
	atteso := map[string]bool{
		"casella luigi@azienda.example":   true,
		"casella filippo@azienda.example": true,
		"worker outlook@PC-LUIGI":         true,
		"worker analisi@PC-FRANCESCO":     true,
	}
	if len(e.NonPiuNelFile) != len(atteso) {
		t.Fatalf("segnalazioni %v, attese %d", e.NonPiuNelFile, len(atteso))
	}
	for _, r := range e.NonPiuNelFile {
		if !atteso[r] {
			t.Errorf("segnalazione inattesa: %s", r)
		}
	}
}

// Una sigla utente che non esiste non fa fallire l'avvio ma non passa inosservata.
func TestSiglaUtenteSconosciuta(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)
	cfg := configDiProva()
	seminaUtenti(t, ctx, q, cfg)
	cfg.Caselle[1].Utente = "ZZ"

	e, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso())
	if err != nil {
		t.Fatalf("il seed non deve fallire per una sigla sconosciuta: %v", err)
	}
	if len(e.UtentiMancanti) != 1 || !strings.HasPrefix(e.UtentiMancanti[0], "ZZ") {
		t.Errorf("attesa la segnalazione della sigla ZZ, ottenuto %v", e.UtentiMancanti)
	}
	c, _ := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "francesco@azienda.example"})
	if c.UtenteID.Valid {
		t.Error("la casella ha un proprietario benché la sigla non esista")
	}
}

// Lo StoreID è locale al profilo: due postazioni registrano valori diversi per la stessa casella (N44).
func TestStoreIDDiversiPerPostazione(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)
	cfg := configDiProva()
	seminaUtenti(t, ctx, q, cfg)
	if _, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}

	casella, _ := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.example"})
	pcA, _ := q.GetPostazionePerHost(ctx, "PC-FRANCESCO")
	pcB, _ := q.GetPostazionePerHost(ctx, "PC-LUIGI")
	if _, err := q.UpsertCasellaStore(ctx, db.UpsertCasellaStoreParams{PostazioneID: pcA.PostazioneID, CasellaID: casella.CasellaID, StoreID: "STORE-LOCALE-A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertCasellaStore(ctx, db.UpsertCasellaStoreParams{PostazioneID: pcB.PostazioneID, CasellaID: casella.CasellaID, StoreID: "STORE-LOCALE-B"}); err != nil {
		t.Fatal(err)
	}
	a, err := q.GetCasellaStore(ctx, db.GetCasellaStoreParams{PostazioneID: pcA.PostazioneID, CasellaID: casella.CasellaID})
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.GetCasellaStore(ctx, db.GetCasellaStoreParams{PostazioneID: pcB.PostazioneID, CasellaID: casella.CasellaID})
	if err != nil {
		t.Fatal(err)
	}
	if a.StoreID == b.StoreID {
		t.Error("le due postazioni hanno lo stesso StoreID: la tabella non sta rappresentando profili distinti")
	}
	righe, err := q.ListCasellaStorePerPostazione(ctx, pcA.PostazioneID)
	if err != nil {
		t.Fatal(err)
	}
	if len(righe) != 1 || righe[0].Indirizzo != "commerciale@azienda.example" {
		t.Errorf("elenco degli store di PC-FRANCESCO inatteso: %+v", righe)
	}
	// Una nuova rilevazione aggiorna, non duplica.
	if _, err := q.UpsertCasellaStore(ctx, db.UpsertCasellaStoreParams{PostazioneID: pcA.PostazioneID, CasellaID: casella.CasellaID, StoreID: "STORE-LOCALE-A2"}); err != nil {
		t.Fatal(err)
	}
	if n := testutil.Conta(t, p, "casella_store"); n != 2 {
		t.Errorf("righe in casella_store %d, attese 2", n)
	}
}

// Avvertenza 1 della revisione del 15/09: con lo schema alla 0003 il cursore di sincronizzazione è per
// sola cartella, quindi due caselle attive se lo sovrascriverebbero a vicenda perdendo messaggi in
// silenzio. Il server deve rifiutarsi di partire, non arrangiarsi.
func TestPiuCaselleAttiveRifiutateFinoAllaVersione4(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)

	prima, err := q.UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: "francesco@azienda.it", Nome: "Francesco"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fondazioni.UnaSolaCasellaAttiva(ctx, q, 3); err != nil {
		t.Fatalf("una sola casella attiva alla versione 3 deve bastare: %v", err)
	}

	seconda, err := q.UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.it", Nome: "Commerciale", Condivisa: true})
	if err != nil {
		t.Fatal(err)
	}
	err = fondazioni.UnaSolaCasellaAttiva(ctx, q, 3)
	if err == nil {
		t.Fatal("due caselle attive alla versione 3 sono state accettate: i due cursori si sovrascriverebbero")
	}
	// l'errore deve dire quali caselle e perché: chi lo legge alle 8 del mattino deve poter agire
	for _, atteso := range []string{"francesco@azienda.it", "commerciale@azienda.it", "cursore"} {
		if !strings.Contains(err.Error(), atteso) {
			t.Errorf("il messaggio non contiene %q: %v", atteso, err)
		}
	}

	// disattivarne una basta a ripartire
	if _, err := p.Exec(ctx, `UPDATE casella SET attiva = false WHERE casella_id = $1`, seconda.CasellaID); err != nil {
		t.Fatal(err)
	}
	if err := fondazioni.UnaSolaCasellaAttiva(ctx, q, 3); err != nil {
		t.Errorf("con una sola casella attiva: %v", err)
	}

	// dalla 0004 in poi il controllo non interviene più: il cursore è per (casella, cartella)
	if _, err := p.Exec(ctx, `UPDATE casella SET attiva = true WHERE casella_id = $1`, seconda.CasellaID); err != nil {
		t.Fatal(err)
	}
	if err := fondazioni.UnaSolaCasellaAttiva(ctx, q, 4); err != nil {
		t.Errorf("alla versione 4 le caselle multiple sono previste: %v", err)
	}
	_ = prima
}
