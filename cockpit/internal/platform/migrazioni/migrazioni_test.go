package migrazioni

import (
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
