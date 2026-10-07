//go:build integrazione

// L4 — la formula delle impronte di contenuto sul database di prova (R117 b; E2 §2.10): su una tabella inventata,
// l'impronta calcolata da PostgreSQL è lo sha256 del testo che la formula della versione 1 descrive, ricostruito
// qui in Go; non cambia con il TimeZone e il DateStyle della sessione, né con l'ordine fisico delle righe, anche
// con le chiavi dell'ordine ripetute; a righe uguali, un valore cambiato è l'errore «valori cambiati», che le
// sentinelle da sole non vedono; il catalogo dà la classe giusta alle colonne (anche attraverso un dominio), e le
// colonne che la formula non rende, quelle che mancano e una tabella che non c'è si nominano; i tipi fuori
// dall'elenco della versione 1 (range, multirange, compositi, array di un dominio con il fuso, …), il cui testo
// cambia davvero con la sessione, fanno dell'impronta della loro tabella una parte non eseguita, e le altre tabelle
// si controllano lo stesso (R-141). La copia intatta del dump non c'entra: ControllaCopia la vuole senza «test» nel
// nome, e qui si provano le sue parti sul database di prova. La collazione "C" della formula qui si vede solo se il
// database di prova ne ha un'altra: la fissa il testo SQL, che la L1 confronta per intero
// (TestSQLDellaFormulaVersione1).
//
// Tabelle, codici e valori sono inventati (ACME): nessun dato reale, il repository è pubblico.

package testutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/dataset"
)

// righeImprontaACME: le righe della tabella inventata, e il testo che la formula ne fa (una riga per riga, in
// ordine di inserimento). I timestamptz sono scritti con +02, e nel testo compaiono in UTC.
var righeImprontaACME = []struct{ valori, testo string }{
	{`(2, 'ACME-030P7120002', '2026-10-07 12:00:00+02', '2026-10-07', 12.50, true, '{"b": 1, "a": [1, 2]}', NULL, '00000000-0000-4000-8000-000000000002')`,
		`[2,"ACME-030P7120002","2026-10-07T10:00:00","2026-10-07",12.50,true,{"a": [1, 2], "b": 1},null,"00000000-0000-4000-8000-000000000002"]`},
	{`(10, 'ACME-030P7120010', '2026-01-31 23:30:00+02', '2026-01-31', 0.00, false, '[]', 'nota "acme"' || chr(10) || 'a capo', '00000000-0000-4000-8000-000000000010')`,
		`[10,"ACME-030P7120010","2026-01-31T21:30:00","2026-01-31",0.00,false,[],"nota \"acme\"\na capo","00000000-0000-4000-8000-000000000010"]`},
	{`(1, 'ACME-030P7120001', '2026-10-07 00:15:00+02', '2026-10-06', 1, NULL, NULL, 'àè', '00000000-0000-4000-8000-000000000001')`,
		`[1,"ACME-030P7120001","2026-10-06T22:15:00","2026-10-06",1.00,null,null,"àè","00000000-0000-4000-8000-000000000001"]`},
}

// colonneImprontaACME: le colonne della tabella inventata, nell'ordine del manifest inventato.
var colonneImprontaACME = []string{"id", "codice", "creato_il", "giorno", "importo", "attivo", "dati", "nota", "rif"}

// creaTabellaImpronta crea la tabella inventata con le righe nell'ordine dato.
func creaTabellaImpronta(t *testing.T, p *pgxpool.Pool, nome string, ordine []int) {
	t.Helper()
	ctx := context.Background()
	if _, err := p.Exec(ctx, `CREATE TABLE `+nome+` (id bigint PRIMARY KEY, codice text NOT NULL, creato_il timestamptz NOT NULL,
		giorno date NOT NULL, importo numeric(10,2) NOT NULL, attivo boolean, dati jsonb, nota text, rif uuid NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, i := range ordine {
		if _, err := p.Exec(ctx, `INSERT INTO `+nome+` VALUES `+righeImprontaACME[i].valori); err != nil {
			t.Fatal(err)
		}
	}
}

// improntaAttesaACME: la formula della versione 1 rifatta in Go sul testo atteso: chiave = l'array del solo id,
// righe ordinate per chiave e poi per riga, byte per byte (la collazione "C"); lo sha256 di ogni riga in esadecimale,
// questi uniti con un a capo, e lo sha256 del testo.
func improntaAttesaACME() string {
	type riga struct{ chiave, testo string }
	var righe []riga
	for _, r := range righeImprontaACME {
		id, _, _ := strings.Cut(strings.TrimPrefix(r.testo, "["), ",")
		righe = append(righe, riga{"[" + id + "]", r.testo})
	}
	sort.Slice(righe, func(i, j int) bool {
		if righe[i].chiave != righe[j].chiave {
			return righe[i].chiave < righe[j].chiave
		}
		return righe[i].testo < righe[j].testo
	})
	impronte := make([]string, len(righe))
	for i, r := range righe {
		s := sha256.Sum256([]byte(r.testo))
		impronte[i] = hex.EncodeToString(s[:])
	}
	s := sha256.Sum256([]byte(strings.Join(impronte, "\n")))
	return hex.EncodeToString(s[:])
}

// poolConSessione: un pool sul database di prova con TimeZone e DateStyle diversi da quelli di partenza.
func poolConSessione(t *testing.T, fuso, stile string) *pgxpool.Pool {
	t.Helper()
	pc, err := pgxpool.ParseConfig(DSN(t))
	if err != nil {
		t.Fatal("il DSN di prova non si legge")
	}
	pc.ConnConfig.RuntimeParams["timezone"] = fuso
	pc.ConnConfig.RuntimeParams["datestyle"] = stile
	p, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestL4ImpronteDiContenuto(t *testing.T) {
	p := Pool(t)
	SchemaVuoto(t, p)
	ctx := context.Background()
	creaTabellaImpronta(t, p, "impronta_acme", []int{0, 1, 2})
	d := dataset.ImprontaTabella{Colonne: colonneImprontaACME, Ordine: []string{"id"}}

	righe, impronta, err := improntaDellaTabella(ctx, leggiDalPool(p), "impronta_acme", d)
	if err != nil {
		t.Fatal(err)
	}
	if righe != "3" || impronta != improntaAttesaACME() {
		t.Fatalf("la formula della versione 1: %s righe, impronta %s; attese 3 righe e %s (il testo descritto in dump.go)", righe, impronta, improntaAttesaACME())
	}

	t.Run("indipendente dalla sessione", func(t *testing.T) {
		for _, s := range []struct{ fuso, stile string }{{"America/New_York", "SQL, DMY"}, {"Pacific/Auckland", "German"}, {"UTC", "ISO, MDY"}} {
			altro := poolConSessione(t, s.fuso, s.stile)
			var fuso, testo string
			if err := altro.QueryRow(ctx, `SELECT current_setting('TimeZone'), to_json(creato_il)::text FROM impronta_acme WHERE id = 2`).Scan(&fuso, &testo); err != nil {
				t.Fatal(err)
			}
			if fuso != s.fuso {
				t.Fatalf("la sessione ha il TimeZone %q, non %q: la prova non prova niente", fuso, s.fuso)
			}
			if s.fuso != "UTC" && strings.Contains(testo, "10:00:00") {
				t.Fatalf("senza la resa in UTC il testo non cambierebbe con il fuso (%s): la prova non prova niente", testo)
			}
			_, altra, err := improntaDellaTabella(ctx, leggiDalPool(altro), "impronta_acme", d)
			if err != nil || altra != impronta {
				t.Errorf("TimeZone %s, DateStyle %s: impronta %s (%v), attesa %s", s.fuso, s.stile, altra, err, impronta)
			}
		}
	})

	t.Run("indipendente dall'ordine fisico", func(t *testing.T) {
		creaTabellaImpronta(t, p, "impronta_acme_bis", []int{2, 1, 0})
		_, altra, err := improntaDellaTabella(ctx, leggiDalPool(p), "impronta_acme_bis", d)
		if err != nil || altra != impronta {
			t.Errorf("le stesse righe inserite al contrario: impronta %s (%v), attesa %s", altra, err, impronta)
		}
	})

	t.Run("chiavi ripetute: decide la riga intera", func(t *testing.T) {
		// senza chiave primaria e con l'ordine su una colonna che si ripete, l'ordine delle righe con la stessa
		// chiave lo decide la riga intera, non l'ordine fisico
		dup := dataset.ImprontaTabella{Colonne: []string{"k", "v"}, Ordine: []string{"k"}}
		var impronte []string
		for i, valori := range []string{`(1, 'acme-b'), (1, 'acme-a'), (2, 'acme-z'), (1, 'acme-c')`, `(1, 'acme-c'), (2, 'acme-z'), (1, 'acme-a'), (1, 'acme-b')`} {
			nome := []string{"chiavi_acme_uno", "chiavi_acme_due"}[i]
			if _, err := p.Exec(ctx, `CREATE TABLE `+nome+` (k int, v text); INSERT INTO `+nome+` VALUES `+valori); err != nil {
				t.Fatal(err)
			}
			_, imp, err := improntaDellaTabella(ctx, leggiDalPool(p), nome, dup)
			if err != nil {
				t.Fatal(err)
			}
			impronte = append(impronte, imp)
		}
		if impronte[0] != impronte[1] {
			t.Errorf("le stesse righe in un altro ordine fisico, con chiavi ripetute: %s e %s", impronte[0], impronte[1])
		}
	})

	t.Run("a righe uguali, valori cambiati", func(t *testing.T) {
		attesa := dataset.CopiaAttesa{
			Sentinelle: map[string]int64{"impronta_acme": 3},
			Impronte: &dataset.ImpronteCopia{Versione: VersioneImpronta, Tabelle: map[string]dataset.ImprontaTabella{
				"impronta_acme": {Colonne: d.Colonne, Ordine: d.Ordine, Sha256: impronta}}},
		}
		if err := ricontrollaCopia(ctx, leggiDalPool(p), attesa); err != nil {
			t.Fatalf("la tabella com'era: %v", err)
		}
		if _, err := p.Exec(ctx, `UPDATE impronta_acme SET creato_il = creato_il + interval '1 microsecond' WHERE id = 10`); err != nil {
			t.Fatal(err)
		}
		if err := controllaSentinelle(ctx, leggiDalPool(p), attesa.Sentinelle); err != nil {
			t.Fatalf("le sentinelle contano le righe, che non sono cambiate: %v", err)
		}
		err := ricontrollaCopia(ctx, leggiDalPool(p), attesa)
		if err == nil || !strings.Contains(err.Error(), "valori cambiati: impronta_acme (3 righe") {
			t.Errorf("un microsecondo in più in una riga: %v; atteso «valori cambiati»", err)
		}
	})

	t.Run("le classi dal catalogo", func(t *testing.T) {
		if _, err := p.Exec(ctx, `CREATE DOMAIN quando_acme AS timestamptz;
			CREATE TABLE classi_acme (a quando_acme, b timestamptz, c timetz, d timestamp, e date, f real, g double precision,
				h interval, i bytea, j money, k timestamptz[], l text[], m numeric, n jsonb, o uuid)`); err != nil {
			t.Fatal(err)
		}
		tutte := strings.Split("a b c d e f g h i j k l m n o", " ")
		v, err := leggiDalPool(p)(ctx, sqlClassiColonne("classi_acme", tutte))
		if err != nil {
			t.Fatal(err)
		}
		const attese = "a:fuso b:fuso c:fuso d:testo e:testo f:sessione g:sessione h:sessione i:sessione j:sessione k:sessione " +
			"l:testo m:testo n:testo o:testo"
		if v != attese {
			t.Errorf("le classi: %q, attese %q", v, attese)
		}
		_, _, err = improntaDellaTabella(ctx, leggiDalPool(p), "classi_acme", dataset.ImprontaTabella{Colonne: []string{"a", "f", "k"}, Ordine: []string{"a"}})
		if !errors.Is(err, ErrTipoNonReso) || !strings.Contains(err.Error(), "le colonne f, k: tipo non reso dalla versione 1") {
			t.Errorf("le colonne che la formula non rende: %v", err)
		}
		_, _, err = improntaDellaTabella(ctx, leggiDalPool(p), "classi_acme", dataset.ImprontaTabella{Colonne: []string{"a", "z"}, Ordine: []string{"a"}})
		if err == nil || !strings.Contains(err.Error(), "non ha le colonne z") {
			t.Errorf("una colonna che manca: %v", err)
		}
		_, _, err = improntaDellaTabella(ctx, leggiDalPool(p), "tabella_che_non_c_e", dataset.ImprontaTabella{Colonne: []string{"a"}, Ordine: []string{"a"}})
		if err == nil || !strings.Contains(err.Error(), "non c'è") {
			t.Errorf("una tabella che non c'è: %v", err)
		}
		// la tabella vuota: lo sha256 del testo vuoto
		righe, vuota, err := improntaDellaTabella(ctx, leggiDalPool(p), "classi_acme", dataset.ImprontaTabella{Colonne: []string{"a", "b"}, Ordine: []string{"a"}})
		if s := sha256.Sum256(nil); err != nil || righe != "0" || vuota != hex.EncodeToString(s[:]) {
			t.Errorf("la tabella vuota: %s righe, impronta %s (%v)", righe, vuota, err)
		}
	})

	t.Run("fuori dall'elenco, una parte non eseguita", func(t *testing.T) {
		// R-141: i tipi che la versione 1 non rende in modo stabile (range e multirange, compositi, array di un
		// dominio con il fuso, un dominio su un dominio, time) sono fuori, e il loro testo cambia davvero con la
		// sessione; i tipi dello schema di oggi (enum, varchar, char, smallint, integer, bigint, inet, uuid[], boolean,
		// json, gli array di timestamp, date ed enum) restano resi come prima
		if _, err := p.Exec(ctx, `CREATE TYPE coppia_acme AS (t timestamptz, s text);
			CREATE TYPE stato_acme AS ENUM ('aperto', 'chiuso');
			CREATE DOMAIN quando_tre_acme AS timestamptz;
			CREATE DOMAIN quando_bis_acme AS quando_tre_acme;
			CREATE TABLE fuori_acme (id int PRIMARY KEY, a tstzrange, b daterange, c int4range, d tstzmultirange, e coppia_acme,
				f quando_tre_acme[], g quando_bis_acme, h time, i stato_acme, j stato_acme[], k varchar(10), l char(3), m smallint,
				n bigint, o inet, q uuid[], r boolean, s json, u timestamp[], v date[], w timestamptz[]);
			INSERT INTO fuori_acme (id, a, b, e, f) VALUES (1, tstzrange('2026-10-07 12:00+02', '2026-10-07 13:00+02'),
				daterange('2026-10-01', '2026-10-07'), ROW('2026-10-07 12:00+02', 'acme'), ARRAY['2026-10-07 12:00+02'::timestamptz]::quando_tre_acme[])`); err != nil {
			t.Fatal(err)
		}
		tutte := strings.Split("a b c d e f g h i j k l m n o q r s u v w", " ")
		v, err := leggiDalPool(p)(ctx, sqlClassiColonne("fuori_acme", tutte))
		if err != nil {
			t.Fatal(err)
		}
		const attese = "a:sessione b:sessione c:sessione d:sessione e:sessione f:sessione g:sessione h:sessione " +
			"i:testo j:testo k:testo l:testo m:testo n:testo o:testo q:testo r:testo s:testo u:testo v:testo w:sessione"
		if v != attese {
			t.Errorf("le classi: %q, attese %q", v, attese)
		}
		// il testo di questi tipi cambia davvero con la sessione: senza l'elenco chiuso l'impronta cambierebbe
		var utc, altra string
		domanda := `SELECT to_json(a)::text || to_json(b)::text || to_json(e)::text || to_json(f)::text FROM fuori_acme`
		if err := poolConSessione(t, "UTC", "ISO, MDY").QueryRow(ctx, domanda).Scan(&utc); err != nil {
			t.Fatal(err)
		}
		if err := poolConSessione(t, "America/New_York", "SQL, DMY").QueryRow(ctx, domanda).Scan(&altra); err != nil {
			t.Fatal(err)
		}
		if utc == altra {
			t.Fatalf("il testo dei tipi fuori dall'elenco non cambia con la sessione (%s): la prova non prova niente", utc)
		}

		// due tabelle: una con un tipo fuori, una resa. La prima è una parte non eseguita, la seconda si controlla
		_, giusta, err := improntaDellaTabella(ctx, leggiDalPool(p), "impronta_acme", d)
		if err != nil {
			t.Fatal(err)
		}
		attesa := dataset.CopiaAttesa{
			Sentinelle: map[string]int64{"impronta_acme": 3, "fuori_acme": 1},
			Impronte: &dataset.ImpronteCopia{Versione: VersioneImpronta, Tabelle: map[string]dataset.ImprontaTabella{
				"impronta_acme": {Colonne: d.Colonne, Ordine: d.Ordine, Sha256: giusta},
				"fuori_acme":    {Colonne: []string{"id", "i", "a"}, Ordine: []string{"id"}, Sha256: giusta}}},
		}
		err = controllaImpronte(ctx, leggiDalPool(p), attesa)
		if !errors.Is(err, ErrTipoNonReso) || !strings.Contains(err.Error(), "fuori_acme (le colonne a: tipo non reso dalla versione 1") {
			t.Errorf("una tabella con un tipo fuori dall'elenco: %v; attesa una parte non eseguita", err)
		}
		if err := ricontrollaCopia(ctx, leggiDalPool(p), attesa); err != nil {
			t.Errorf("il ricontrollo ripete la parte non eseguita: %v", err)
		}
		cambiata := attesa.Impronte.Tabelle["impronta_acme"]
		cambiata.Sha256 = strings.Repeat("0", 64)
		attesa.Impronte.Tabelle["impronta_acme"] = cambiata
		err = controllaImpronte(ctx, leggiDalPool(p), attesa)
		if err == nil || errors.Is(err, ErrTipoNonReso) || !strings.Contains(err.Error(), "valori cambiati: impronta_acme") {
			t.Errorf("con l'altra tabella cambiata: %v; attesi i valori cambiati", err)
		}
	})
}
