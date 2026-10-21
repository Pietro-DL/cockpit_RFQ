package migrazioni

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"testing/fstest"

	risorse "promatec/cockpit"
)

// S1 (parte statica): nessun file referenzia una tabella creata dopo di lui, ogni file registra la
// propria versione, le versioni sono 1..n senza buchi. Gira senza DB, a ogni commit.
func TestS1VerificaStaticaDelleMigrazioniReali(t *testing.T) {
	migs, err := Elenca(risorse.FS)
	if err != nil {
		t.Fatalf("elenco migrazioni: %v", err)
	}
	if len(migs) < 2 {
		t.Fatalf("attese almeno 2 migrazioni (0001 schema, 0002 fondazioni), trovate %d", len(migs))
	}
	if err := VerificaStatica(migs); err != nil {
		t.Fatalf("verifica statica: %v", err)
	}
	for i, m := range migs {
		if m.Versione != i+1 {
			t.Errorf("%s: versione %d in posizione %d", m.Nome, m.Versione, i+1)
		}
	}
}

// TestDopoV1LUltimaMigrazioneEla23 — Riscritta per V1 (piano della 0023, §9.2). Era TestDopoP4LUltimaMigrazioneEla22,
// e prima ancora TestInA1LUltimaMigrazioneEla21 (G10; P-02). La ratifica 05 §1 ammette un'altra migrazione P4 solo per
// un'impossibilita' tecnica provata; la direttiva 23 (D23:125-127) la prescrive per il formato 3, con il primo numero
// libero, senza riscrivere la 0022. Quindi: l'ultima e' la 0023 di V1, e nessun'altra migrazione la segue finche'
// AM0, A2 o la tappa dopo non la riscrivono a loro volta («Riscritta per ...»).
func TestDopoV1LUltimaMigrazioneEla23(t *testing.T) {
	migs, err := Elenca(risorse.FS)
	if err != nil {
		t.Fatalf("elenco migrazioni: %v", err)
	}
	ultima := migs[len(migs)-1]
	if ultima.Versione != 23 || ultima.Nome != "0023_distinta_formato3.sql" {
		t.Fatalf("l'ultima migrazione è %s (versione %d): dopo V1 deve essere la 0023_distinta_formato3.sql", ultima.Nome, ultima.Versione)
	}
}

// TestP4LeMigrazioniStoricheNonCambiano (ratifica 2026-10-12-05 par.1: nessun file 0001..0021 modificato,
// rimosso o riscritto; README di migrations, «Un file applicato non si tocca più»): l'impronta sha256 di ogni
// file applicato, letta dal binario. I fine riga si portano a LF prima dell'impronta, perché il checkout su
// Windows può scriverli CRLF (core.autocrlf): l'impronta è quella del file nel repository, al commit
// 2c3c185 (QA), che dopo la 0021 non tocca migrations/. Con V1 la 0022 diventa storica anche lei (piano della 0023,
// §9.2): la 0023 la estende senza riscriverla.
func TestP4LeMigrazioniStoricheNonCambiano(t *testing.T) {
	attese := map[string]string{
		"0001_schema.sql":              "3b4f7d86d160a78f300065160f69d3b72e704b1dfd58c812c365746d75f9212c",
		"0002_fondazioni.sql":          "2333804cf8a0f479ddb0891893532811f7ebf145b91bbd41697470bbab53a8f4",
		"0003_coda_ingest.sql":         "1dff4a0ee2d1c92e32937a7b1cd3d85771ee9e47c73f97e7f0327ada80d3dae8",
		"0004_caselle_presenza.sql":    "7797e27a7d481435bd16cbf9fdb4d5303d510a800590ec448f9ad53321d6bcde",
		"0005_postazioni_presenza.sql": "7410adef2230315785ad8a930a8ef23e6343ebe95ad26c382163938e77afb911",
		"0006_inbox_viva.sql":          "c0a62708487f8c7bbb560179f884e1d7f50e5a256c7e1890fba50a2176e31734",
		"0007_anagrafica.sql":          "e231b57052f95e889d4f2ee486b08da6033837a846f17253285ba025ccab9428",
		"0008_interpretazione.sql":     "bdcdbab228f02a195d9fabcc064298c46b331806d20cc648d25a31b7a5aa89c2",
		"0009_presenza_contatto.sql":   "b40dc6f2ae02c1f2bd537fdd48c1ae93a1c7de28a4a4c1ed5e21c41dca4557ee",
		"0010_copertura_sync.sql":      "cc051f6ff0ef132cdc8feee0218a002919402f9172b3a88b4bc1047973aa12cf",
		"0011_sync_apertura_inbox.sql": "95c1258516c60f8f159e5dc698c9dce81c12a7a91e6c2638dba15975dea8d646",
		"0012_estrai_archivio.sql":     "43bace97bdd6dcb9deb66abd317a11fdbfac0a55d4402e089eaf8514dabe347f",
		"0013_integrita_nas.sql":       "fb314d864a92cee9e1a2581e701c8522888e782c345b6c656a5e2232f20dbc2e",
		"0014_fornitori.sql":           "36db711f7ce61956639ec4a3c85574937df1472e0fe1870373352a4cfc9a7bd6",
		"0015_richiesta_fornitore.sql": "60870e378a2abf9534e9a0e4583dffbb960dcacc460d670820f30c0006f0e371",
		"0016_classificazione.sql":     "59d2bd72617c113043b836fafe0bdcabe27314843d96f085c9e6f66b5857cbed",
		"0017_controparte_altro.sql":   "d6066ac4d5bcb3a212fd056b1127dd42611508389f5f18e79251eacef3bfca77",
		"0018_fascicolo.sql":           "5ec4769723397d6ded551368ef96127cd61ca94af8da1352260a7605cf0251ef",
		"0019_sposta_nas.sql":          "3eda6d2565fc5faae17c2c866ed7b104d520bbaba712ecb26f1e49f5e9bf6671",
		"0020_bom_versioni.sql":        "91596dcf7a107baa1a62ebe7c8f14e72bd74ae829a1681f3f8c94d4d5396414c",
		"0021_annotazioni_pdf.sql":     "60400c47bdd5ea8d58cfc4e0c26ff080fb8010ed183cf472e15a52eedaae6c48",
		// V1 (piano della 0023, §9.2): la 0022 e' distribuita e da storica non si tocca piu' (variante
		// 0022-p4-dir11-20261013b, la stessa impronta di TestP4VarianteDella0022)
		"0022_p4_conferme_working.sql": "b4974dc5a0068421ead99b9fb8d261f4cee9c38befd6d86c89dfb57868b74f66",
	}
	migs, err := Elenca(risorse.FS)
	if err != nil {
		t.Fatalf("elenco migrazioni: %v", err)
	}
	visti := 0
	for _, m := range migs {
		if m.Versione > 22 {
			continue
		}
		visti++
		h := sha256.Sum256([]byte(strings.ReplaceAll(m.SQL, "\r\n", "\n")))
		if got, want := hex.EncodeToString(h[:]), attese[m.Nome]; got != want {
			t.Errorf("%s: impronta %s, attesa %q: un file applicato non si tocca più", m.Nome, got, want)
		}
	}
	if visti != len(attese) {
		t.Errorf("migrazioni storiche lette %d, attese %d", visti, len(attese))
	}
}

func fsFinto(file map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for nome, sql := range file {
		out["migrations/"+nome] = &fstest.MapFile{Data: []byte(sql)}
	}
	return out
}

// S1 (negativo): una REFERENCES in avanti viene rifiutata prima di toccare il DB — è l'errore N43.
func TestS1ReferenzaInAvantiRifiutata(t *testing.T) {
	migs, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "CREATE TABLE a (id uuid PRIMARY KEY);\nINSERT INTO schema_versione (versione) VALUES (1);",
		"0002_due.sql": "CREATE TABLE b (id uuid PRIMARY KEY, a_id uuid REFERENCES c);\nINSERT INTO schema_versione (versione) VALUES (2);",
		"0003_tre.sql": "CREATE TABLE c (id uuid PRIMARY KEY);\nINSERT INTO schema_versione (versione) VALUES (3);",
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = VerificaStatica(migs)
	if err == nil || !strings.Contains(err.Error(), "REFERENCES c") {
		t.Fatalf("attesa segnalazione della referenza in avanti, ottenuto %v", err)
	}
}

// S4 (negativo): un valore di enum aggiunto e usato nello stesso file viene rifiutato, perché
// PostgreSQL non lo accetta prima del commit.
func TestS4EnumUsatoNelloStessoFile(t *testing.T) {
	migs, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "CREATE TYPE stato AS ENUM ('a');\nCREATE TABLE t (s stato);\nINSERT INTO schema_versione (versione) VALUES (1);",
		"0002_due.sql": "ALTER TYPE stato ADD VALUE 'annullato';\nUPDATE t SET s = 'annullato';\nINSERT INTO schema_versione (versione) VALUES (2);",
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = VerificaStatica(migs)
	if err == nil || !strings.Contains(err.Error(), "annullato") {
		t.Fatalf("atteso rifiuto del valore enum usato nello stesso file, ottenuto %v", err)
	}
}

// S4 (positivo): aggiungere il valore senza usarlo è corretto.
func TestS4EnumAggiuntoSenzaUsarlo(t *testing.T) {
	migs, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "CREATE TYPE stato AS ENUM ('a');\nINSERT INTO schema_versione (versione) VALUES (1);",
		"0002_due.sql": "ALTER TYPE stato ADD VALUE 'annullato';\nINSERT INTO schema_versione (versione) VALUES (2);",
		"0003_tre.sql": "CREATE TABLE t (s stato NOT NULL DEFAULT 'annullato');\nINSERT INTO schema_versione (versione) VALUES (3);",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerificaStatica(migs); err != nil {
		t.Fatalf("verifica statica: %v", err)
	}
}

// Un file che non registra la propria versione non deve poter essere applicato.
func TestVersioneNonRegistrata(t *testing.T) {
	migs, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "CREATE TABLE a (id uuid);\nINSERT INTO schema_versione (versione) VALUES (1);",
		"0002_due.sql": "CREATE TABLE b (id uuid);",
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = VerificaStatica(migs)
	if err == nil || !strings.Contains(err.Error(), "schema_versione") {
		t.Fatalf("atteso rifiuto del file che non si registra, ottenuto %v", err)
	}
}

// Una numerazione con buchi è un errore di confezionamento, non un caso da tollerare.
func TestVersioniConBuchi(t *testing.T) {
	_, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "INSERT INTO schema_versione (versione) VALUES (1);",
		"0003_tre.sql": "INSERT INTO schema_versione (versione) VALUES (3);",
	}))
	if err == nil || !strings.Contains(err.Error(), "0002") {
		t.Fatalf("atteso errore sul buco di numerazione, ottenuto %v", err)
	}
}

func TestNomeFileNonValido(t *testing.T) {
	_, err := Elenca(fsFinto(map[string]string{"schema.sql": "INSERT INTO schema_versione (versione) VALUES (1);"}))
	if err == nil || !strings.Contains(err.Error(), "nome non valido") {
		t.Fatalf("atteso rifiuto del nome file, ottenuto %v", err)
	}
}

// variantiNote: per ogni migrazione che dichiara la sua variante (dalla 0022 in poi), la variante del testo di oggi con
// la sua impronta (LF) e le varianti gia' usate da testi precedenti, con l'impronta di quei testi. Se un file cambia
// prima della spinta (regola R-D11 della nota B1), cambiano insieme la variante nel file e la riga qui: un database
// con il testo precedente non passa per «schema presente». Chi cambia il testo e aggiorna solo l'impronta, lasciando
// la variante di prima, fa fallire la prova (revisione SQL della notte, R-06).
var variantiNote = map[int]struct {
	variante, impronta string
	giaUsate           map[string]string
}{
	22: {"0022-p4-dir11-20261013b", "b4974dc5a0068421ead99b9fb8d261f4cee9c38befd6d86c89dfb57868b74f66",
		map[string]string{"0022-p4-dir11-20261013a": "ddf3e2d2351791d050a4f132c63268c3f693863d4627f08dd20ac47ad58f794c"}},
	// V1 (piano della 0023, §2): variante 0023-v1-formato3-file-<aaaammgg><lettera>; la «a» e' il testo del primo
	// passaggio della corsia persistenza, la «b» quello dopo la sua revisione, la «c» con i bianchi di ck_csc_identita
	// tutti come sequenze di escape, la «d» con il delta di U2 §3.14 e di DEC-B dell'ondata C, la 20261019a con la
	// direttiva 35 (blocco C-1b: file_rfq_associazione e il giornale delle AR), la 20261019b con l'indice
	// d'idempotenza del giornale NULLS NOT DISTINCT, la 20261019c con la direttiva 39 (provenienza della PB sulla AR,
	// gesto_legacy e conversione_legacy, lo STEP legato come sorgente, revoca_sorgente, il segno della RFQ formato 3), la
	// 20261019d con trg_proposta_copia_derivata acceso dopo C-6b (mai spinti, solo database di prova)
	23: {"0023-v1-formato3-file-20261019d", "4e633575d9cbc712899c89174a3bd594f630b57d114dee8074a63230b667e885",
		map[string]string{"0023-v1-formato3-file-20261018a": "6a315baf5d825916731146399ae52052aca64b812dd2e8ad35caa760921c9b58",
			"0023-v1-formato3-file-20261018b": "37c698df889d4539cdedee29d3da17cc52ae804f1abb4e81fb9d459777056e9b",
			"0023-v1-formato3-file-20261018c": "074a08efd0a67f159d0c185188eedebe7138e5a840667fd53c6e82c48e81c484",
			"0023-v1-formato3-file-20261018d": "e9e3344b4fcc26bc75c02039ad614a928ac8575ab349bcb58b8abea84f433df4",
			"0023-v1-formato3-file-20261019a": "949d9e36624c56957b42f24f59578442034679a5790f0b9586e629dafdbd5206",
			"0023-v1-formato3-file-20261019b": "949ea5caebd4daf29f17d539d2a46694ec6d311f93515032848a91e885f4562d",
			"0023-v1-formato3-file-20261019c": "ccf67543ff2fa698154bc00ea0a679417156930239b062b0906fa33e7ffc19dc"}},
}

// controllaVariante: la migrazione m dichiara la variante nota e ha la sua impronta.
func controllaVarianteNota(t *testing.T, m Migrazione) {
	t.Helper()
	nota, ok := variantiNote[m.Versione]
	if !ok {
		t.Fatalf("%s dichiara la variante %q, ma la prova non la conosce: aggiungerla a variantiNote", m.Nome, m.Variante)
	}
	if m.Variante != nota.variante {
		t.Errorf("%s dichiara la variante %q, attesa %q", m.Nome, m.Variante, nota.variante)
	}
	h := sha256.Sum256([]byte(strings.ReplaceAll(m.SQL, "\r\n", "\n")))
	got := hex.EncodeToString(h[:])
	if got != nota.impronta {
		t.Errorf("%s ha l'impronta %s, attesa %s: il testo e' cambiato, quindi cambiano anche la variante nel file e qui", m.Nome, got, nota.impronta)
	}
	if vecchia, usata := nota.giaUsate[m.Variante]; usata && vecchia != got {
		t.Errorf("%s dichiara la variante %q, gia' usata dal testo con l'impronta %s: un testo nuovo vuole una variante nuova", m.Nome, m.Variante, vecchia)
	}
	for v, imp := range nota.giaUsate {
		if imp == got && v != m.Variante {
			t.Errorf("il testo di %s e' quello della variante %q, ma dichiara %q", m.Nome, v, m.Variante)
		}
	}
}

// TestP4VarianteDella0022 (direttiva 11 par.2: «impossibilita' di applicare erroneamente due varianti della 0022
// allo stesso DB»). schema_versione registra solo il numero; la 0022 dichiara la sua variante e il migratore la
// confronta con quella del database (ControllaVarianti). Riscritta per V1 (piano della 0023, §9.2): la 0022 si cerca
// per numero, non come l'ultima, e resta 0022-p4-dir11-20261013b / b4974dc5…; ogni migrazione dalla 22 in poi dichiara
// la sua variante, legata alla sua impronta (TestV1VarianteDella0023 per la 23), e nessuna prima della 22.
func TestP4VarianteDella0022(t *testing.T) {
	migs, err := Elenca(risorse.FS)
	if err != nil {
		t.Fatalf("elenco migrazioni: %v", err)
	}
	var m22 *Migrazione
	for i := range migs {
		if migs[i].Versione == 22 {
			m22 = &migs[i]
		}
	}
	if m22 == nil {
		t.Fatal("la 0022 non c'e'")
	}
	controllaVarianteNota(t, *m22)
	for _, x := range migs {
		switch {
		case x.Versione < 22 && x.Variante != "":
			t.Errorf("%s dichiara una variante: dalla 0022 in poi", x.Nome)
		case x.Versione >= 22 && x.Variante == "":
			t.Errorf("%s non dichiara la sua variante: dalla 0022 in poi ogni migrazione la dichiara", x.Nome)
		case x.Versione >= 22:
			controllaVarianteNota(t, x)
		}
	}
}

// TestV1VarianteDella0023 (M-20, parte L1): la 0023 dichiara la sua variante, legata alla sua impronta. La parte L4
// (un database con la 23 di un'altra variante non parte) e' in v1_0023_db_test.go.
func TestV1VarianteDella0023(t *testing.T) {
	migs, err := Elenca(risorse.FS)
	if err != nil {
		t.Fatalf("elenco migrazioni: %v", err)
	}
	for _, m := range migs {
		if m.Versione == 23 {
			if m.Variante == "" {
				t.Fatalf("%s non dichiara la sua variante", m.Nome)
			}
			controllaVarianteNota(t, m)
			return
		}
	}
	t.Fatal("la 0023 non c'e'")
}

// La variante dichiarata si legge fuori dai commenti e deve essere quella della versione del file.
func TestVarianteDichiarata(t *testing.T) {
	migs, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "CREATE TABLE schema_variante (versione int PRIMARY KEY, variante text NOT NULL);\n-- INSERT INTO schema_variante (versione, variante) VALUES (1, 'commentata');\nINSERT INTO schema_versione (versione) VALUES (1);",
		"0002_due.sql": "INSERT INTO schema_variante (versione, variante) VALUES (2, 'due-a');\nINSERT INTO schema_versione (versione) VALUES (2);",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if migs[0].Variante != "" || migs[1].Variante != "due-a" {
		t.Errorf("varianti lette: %q, %q", migs[0].Variante, migs[1].Variante)
	}
	if err := VerificaStatica(migs); err != nil {
		t.Errorf("verifica statica: %v", err)
	}
	sbagliata, err := Elenca(fsFinto(map[string]string{
		"0001_uno.sql": "INSERT INTO schema_variante (versione, variante) VALUES (7, 'altra');\nINSERT INTO schema_versione (versione) VALUES (1);",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerificaStatica(sbagliata); err == nil || !strings.Contains(err.Error(), "variante della versione 7") {
		t.Errorf("variante di un'altra versione: %v", err)
	}
}
