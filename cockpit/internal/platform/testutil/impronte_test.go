// L1 — le impronte di contenuto della copia intatta, senza database (R117 b, ratificata e ampliata; E2 §2.10;
// A1c-L4D-01 ne è la prova sul dump): con un lettore finto, a righe uguali un'impronta diversa è l'errore «valori
// cambiati», con le tabelle cambiate tutte nominate; una copia senza impronte dichiarate è una parte non eseguita,
// con il motivo, mai un verde, e PoolDump la segna NON ESEGUITA senza fermare la prova; una sezione impronte che non
// si sa calcolare è un errore del manifest; le colonne che mancano o che la formula non rende si nominano; il testo
// SQL della formula, versione 1, è fisso. La formula sui valori veri la prova la L4 sul database di prova
// (impronte_db_test.go).
//
// Copie, ruoli, tabelle, colonne e impronte sono inventati: i valori veri stanno solo nel manifest privato, e il
// repository è pubblico.

package testutil

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/dataset"
)

// TestVersioneImprontaFissa: la formula cambia solo con una versione nuova e le impronte ricalcolate.
func TestVersioneImprontaFissa(t *testing.T) {
	if VersioneImpronta != 1 {
		t.Fatalf("VersioneImpronta = %d: cambiarla vuol dire ricalcolare le impronte del manifest, con la riga di storico, e riscrivere queste prove", VersioneImpronta)
	}
}

// TestImpronteValoriCambiatiARigheUguali (R117 §4 b): le sentinelle passano (le righe sono quelle del manifest), ma
// l'impronta no. L'errore dice «valori cambiati», la tabella, l'impronta trovata e quella del manifest. Lo stesso
// nel ricontrollo di t.Cleanup.
func TestImpronteValoriCambiatiARigheUguali(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	attesa, risposte := copiaACME(t)
	imp := improntaComponenteACME()
	if err := ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa); err != nil {
		t.Fatalf("la copia giusta, con l'impronta giusta: %v", err)
	}
	trovata := strings.Repeat("0", 64)
	risposte[sqlImpronta("componente", imp.Colonne, imp.Ordine, classiComponenteACME)] = "3 " + trovata
	leggi := lettoreDiProva(risposte, nil)
	if err := controllaSentinelle(ctx, leggi, attesa.Sentinelle); err != nil {
		t.Fatalf("le sentinelle dovevano passare (le righe non sono cambiate): %v", err)
	}
	for nome, controlla := range map[string]func() error{
		"ControllaCopia":   func() error { return ControllaCopia(ctx, leggi, attesa) },
		"ricontrollaCopia": func() error { return ricontrollaCopia(ctx, leggi, attesa) },
	} {
		err := controlla()
		if err == nil {
			t.Errorf("%s: un'impronta diversa a righe uguali passa", nome)
			continue
		}
		for _, frase := range []string{"valori cambiati", "componente", trovata, improntaACME, "3 righe"} {
			if !strings.Contains(err.Error(), frase) {
				t.Errorf("%s: %v; atteso che dica %q", nome, err, frase)
			}
		}
		if errors.Is(err, ErrImpronteNonDichiarate) {
			t.Errorf("%s: un'impronta diversa non è una parte non eseguita: %v", nome, err)
		}
	}
}

// TestImpronteTutteLeTabelleCambiate: le tabelle cambiate si dicono tutte, in ordine di nome, non solo la prima; le
// maiuscole dello sha256 dichiarato non contano.
func TestImpronteTutteLeTabelleCambiate(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	attesa, risposte := copiaACME(t)
	allegato := dataset.ImprontaTabella{Colonne: []string{"id", "nome"}, Ordine: []string{"id"}, Sha256: strings.ToUpper(strings.Repeat("be", 32))}
	attesa.Impronte.Tabelle["allegato"] = allegato
	classi := map[string]string{"id": classeTesto, "nome": classeTesto}
	risposte[sqlClassiColonne("allegato", allegato.Colonne)] = "id:testo nome:testo"
	risposte[sqlImpronta("allegato", allegato.Colonne, allegato.Ordine, classi)] = "5 " + strings.Repeat("be", 32)
	if err := ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa); err != nil {
		t.Fatalf("lo sha256 dichiarato in maiuscolo è lo stesso: %v", err)
	}
	risposte[sqlImpronta("allegato", allegato.Colonne, allegato.Ordine, classi)] = "5 " + strings.Repeat("1", 64)
	imp := improntaComponenteACME()
	risposte[sqlImpronta("componente", imp.Colonne, imp.Ordine, classiComponenteACME)] = "3 " + strings.Repeat("2", 64)
	err := ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa)
	if err == nil {
		t.Fatal("due impronte diverse passano")
	}
	a, c := strings.Index(err.Error(), "allegato ("), strings.Index(err.Error(), "componente (")
	if a < 0 || c < 0 || a > c {
		t.Errorf("le due tabelle, in ordine di nome: %v", err)
	}
}

// TestImpronteNonDichiarateParteNonEseguita (R117 §4 b): senza impronte, e con tutto il resto che passa,
// ControllaCopia dà ErrImpronteNonDichiarate, con il motivo, mai nil. Una deviazione che viene prima vince: la
// parte non eseguita vuol dire che gli altri controlli sono passati. Il ricontrollo di t.Cleanup non la ripete.
func TestImpronteNonDichiarateParteNonEseguita(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	attesa, risposte := copiaACME(t)
	attesa.Impronte = nil
	err := ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa)
	if !errors.Is(err, ErrImpronteNonDichiarate) {
		t.Fatalf("senza impronte: %v; attesa ErrImpronteNonDichiarate, mai nil", err)
	}
	for _, frase := range []string{"non è eseguita", "valore cambiato", "R117 b"} {
		if !strings.Contains(err.Error(), frase) {
			t.Errorf("il motivo %q non dice %q", err, frase)
		}
	}
	risposte[sqlUtenteCorrente] = "proprietario_acme"
	if err := ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa); err == nil || errors.Is(err, ErrImpronteNonDichiarate) ||
		!strings.Contains(err.Error(), "utente") {
		t.Errorf("un altro ruolo, senza impronte: %v; atteso l'errore del ruolo", err)
	}
	risposte[sqlUtenteCorrente] = "lettore_acme"
	if err := ricontrollaCopia(ctx, lettoreDiProva(risposte, nil), attesa); err != nil {
		t.Errorf("il ricontrollo senza impronte ripete la parte non eseguita, o non passa: %v", err)
	}
	risposte[sqlRighe("allegato")] = "6"
	if err := ricontrollaCopia(ctx, lettoreDiProva(risposte, nil), attesa); err == nil || !strings.Contains(err.Error(), "sentinelle: allegato") {
		t.Errorf("il ricontrollo senza impronte guarda ancora le sentinelle: %v", err)
	}
}

// tbEsito: un testing.TB che registra Errorf e Fatalf, per vedere che cosa fa esitoDiControllaCopia alla prova.
type tbEsito struct {
	testing.TB
	errori []string
	fatale string
}

func (f *tbEsito) Helper() {}

func (f *tbEsito) Errorf(format string, args ...any) {
	f.errori = append(f.errori, fmt.Sprintf(format, args...))
}

func (f *tbEsito) Fatalf(format string, args ...any) {
	f.fatale = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// conTBEsito esegue corpo con un tbEsito, in una goroutine sua; dopo dice se corpo è arrivato in fondo.
func conTBEsito(t *testing.T, corpo func(tb testing.TB)) (f *tbEsito, finito bool) {
	t.Helper()
	f = &tbEsito{TB: t}
	fatto := make(chan struct{})
	go func() {
		defer close(fatto)
		corpo(f)
		finito = true
	}()
	<-fatto
	return f, finito
}

// TestPoolDumpSenzaImpronteParteNonEseguita: la decisione di PoolDump dopo ControllaCopia. Senza impronte la prova
// va avanti, ma con un «NON ESEGUITA:» e il motivo, quindi non è mai verde; ogni altro errore la ferma con
// NON ESEGUITA; nil non la tocca.
func TestPoolDumpSenzaImpronteParteNonEseguita(t *testing.T) {
	f, finito := conTBEsito(t, func(tb testing.TB) { esitoDiControllaCopia(tb, nil) })
	if !finito || len(f.errori) != 0 || f.fatale != "" {
		t.Errorf("nil: errori %q, fatale %q, finito %v", f.errori, f.fatale, finito)
	}
	f, finito = conTBEsito(t, func(tb testing.TB) { esitoDiControllaCopia(tb, ErrImpronteNonDichiarate) })
	if !finito || f.fatale != "" || len(f.errori) != 1 ||
		f.errori[0] != "NON ESEGUITA: "+ErrImpronteNonDichiarate.Error() {
		t.Errorf("senza impronte: errori %q, fatale %q, finito %v; attesa una parte NON ESEGUITA che non ferma la prova", f.errori, f.fatale, finito)
	}
	f, finito = conTBEsito(t, func(tb testing.TB) { esitoDiControllaCopia(tb, errors.New("utente: collegato come x")) })
	if finito || !strings.HasPrefix(f.fatale, "NON ESEGUITA: la copia del dump non è quella del manifest: utente") || len(f.errori) != 0 {
		t.Errorf("un altro errore: errori %q, fatale %q, finito %v; attesa una NON ESEGUITA che ferma la prova", f.errori, f.fatale, finito)
	}
}

// TestImpronteDichiarazioneDelManifest: una sezione impronte che questo codice non sa calcolare è un errore del
// manifest, detto prima di qualunque domanda al database.
func TestImpronteDichiarazioneDelManifest(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	nessunaDomanda := func(_ context.Context, sql string) (string, error) {
		return "", fmt.Errorf("domanda al database prima del controllo del manifest: %s", sql)
	}
	conTabella := func(cambia func(*dataset.ImprontaTabella)) func(*dataset.ImpronteCopia) {
		return func(imp *dataset.ImpronteCopia) {
			d := imp.Tabelle["componente"]
			cambia(&d)
			imp.Tabelle["componente"] = d
		}
	}
	for _, c := range []struct {
		nome   string
		cambia func(*dataset.ImpronteCopia)
		frase  string
	}{
		{"un'altra versione", func(imp *dataset.ImpronteCopia) { imp.Versione = 2 }, "versione_impronta 2"},
		{"versione zero", func(imp *dataset.ImpronteCopia) { imp.Versione = 0 }, "versione_impronta 0"},
		{"nessuna tabella", func(imp *dataset.ImpronteCopia) { imp.Tabelle = nil }, "nessuna tabella"},
		{"un nome di tabella non valido", func(imp *dataset.ImpronteCopia) {
			imp.Tabelle["x; drop"] = improntaComponenteACME()
		}, "non è un nome di tabella"},
		{"una tabella senza sentinella", func(imp *dataset.ImpronteCopia) {
			imp.Tabelle["documento"] = improntaComponenteACME()
		}, "documento non ha la sentinella"},
		{"senza colonne", conTabella(func(d *dataset.ImprontaTabella) { d.Colonne = nil }), "non dichiara le colonne"},
		{"senza ordine", conTabella(func(d *dataset.ImprontaTabella) { d.Ordine = nil }), "non dichiara l'ordine"},
		{"sha256 corto", conTabella(func(d *dataset.ImprontaTabella) { d.Sha256 = "abc" }), "64 cifre"},
		{"sha256 non esadecimale", conTabella(func(d *dataset.ImprontaTabella) { d.Sha256 = strings.Repeat("g", 64) }), "64 cifre"},
		{"un nome di colonna non valido", conTabella(func(d *dataset.ImprontaTabella) { d.Colonne = append(d.Colonne, "Codice") }), "non è un nome di colonna"},
		{"una colonna ripetuta", conTabella(func(d *dataset.ImprontaTabella) { d.Colonne = append(d.Colonne, "codice") }), "compare due volte"},
		{"l'ordine fuori dalle colonne", conTabella(func(d *dataset.ImprontaTabella) { d.Ordine = []string{"posizione"} }), "non è fra le colonne"},
		{"l'ordine ripetuto", conTabella(func(d *dataset.ImprontaTabella) { d.Ordine = []string{"id", "id"} }), "due volte nell'ordine"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			attesa, _ := copiaACME(t)
			c.cambia(attesa.Impronte)
			err := ControllaCopia(ctx, nessunaDomanda, attesa)
			if err == nil || !strings.HasPrefix(err.Error(), "manifest: impronte") || !strings.Contains(err.Error(), c.frase) {
				t.Errorf("errore %v, atteso un errore del manifest che dica %q", err, c.frase)
			}
		})
	}
}

// TestImprontaColonneDalCatalogo: le colonne che la tabella non ha (o una tabella che non c'è), quelle che la
// formula non rende e le risposte di forma inattesa si nominano, mai un'impronta.
func TestImprontaColonneDalCatalogo(t *testing.T) {
	ctx := context.Background()
	imp := improntaComponenteACME()
	classi := sqlClassiColonne("componente", imp.Colonne)
	impronta := sqlImpronta("componente", imp.Colonne, imp.Ordine, classiComponenteACME)
	for _, c := range []struct {
		nome     string
		risposte map[string]string
		frase    string
	}{
		{"una colonna che manca", map[string]string{classi: "codice:testo id:testo"}, "non ha le colonne creato_il"},
		{"una tabella che non c'è", map[string]string{classi: ""}, "non c'è, o non ha le colonne id, codice, creato_il"},
		{"una colonna che la formula non rende", map[string]string{classi: "codice:sessione creato_il:fuso id:testo"}, "le colonne codice: tipo non reso dalla versione 1"},
		{"una classe sconosciuta", map[string]string{classi: "codice:altro creato_il:fuso id:testo"}, "classe \"altro\""},
		{"le classi senza i due punti", map[string]string{classi: "codice creato_il:fuso id:testo"}, "risposta"},
		{"l'impronta senza le righe", map[string]string{classi: "codice:testo creato_il:fuso id:testo", impronta: improntaACME}, "risposta"},
		{"le righe non sono un numero", map[string]string{classi: "codice:testo creato_il:fuso id:testo", impronta: "tre " + improntaACME}, "risposta"},
		{"l'impronta in maiuscolo", map[string]string{classi: "codice:testo creato_il:fuso id:testo", impronta: "3 " + strings.ToUpper(improntaACME)}, "risposta"},
		{"l'impronta corta", map[string]string{classi: "codice:testo creato_il:fuso id:testo", impronta: "3 abc"}, "risposta"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			_, _, err := improntaDellaTabella(ctx, lettoreDiProva(c.risposte, nil), "componente", imp)
			if err == nil || !strings.Contains(err.Error(), c.frase) {
				t.Errorf("errore %v, atteso che dica %q", err, c.frase)
			}
		})
	}
	righe, sha, err := improntaDellaTabella(ctx, lettoreDiProva(map[string]string{classi: "codice:testo creato_il:fuso id:testo", impronta: "3 " + improntaACME}, nil), "componente", imp)
	if err != nil || righe != "3" || sha != improntaACME {
		t.Errorf("la risposta giusta: %q %q %v", righe, sha, err)
	}
}

// TestSQLDellaFormulaVersione1: il testo SQL della formula, versione 1, è fisso. Le colonne con il fuso passano da
// UTC, le altre no; la riga ha le colonne nell'ordine del manifest, la chiave quelle dell'ordine; le righe si
// ordinano per chiave e poi per riga, con la collazione "C"; gli sha256 delle righe si uniscono con un a capo, e
// l'impronta è lo sha256 di questo testo, sempre dei byte UTF-8.
// Cambiarlo vuol dire una versione nuova (VersioneImpronta) e le impronte del manifest ricalcolate.
func TestSQLDellaFormulaVersione1(t *testing.T) {
	got := sqlImpronta("componente", []string{"id", "creato_il", "user"}, []string{"user", "id"},
		map[string]string{"id": classeTesto, "creato_il": classeFuso, "user": classeTesto})
	const atteso = `SELECT count(*)::text || ' ' || encode(sha256(convert_to(coalesce(string_agg(encode(sha256(convert_to(r.riga, 'UTF8')), 'hex'), chr(10)` +
		` ORDER BY r.chiave COLLATE "C", r.riga COLLATE "C"), ''), 'UTF8')), 'hex')` +
		` FROM (SELECT '[' || coalesce(to_json(x."id")::text, 'null') || ',' || coalesce(to_json((x."creato_il" AT TIME ZONE 'UTC'))::text, 'null')` +
		` || ',' || coalesce(to_json(x."user")::text, 'null') || ']' AS riga,` +
		` '[' || coalesce(to_json(x."user")::text, 'null') || ',' || coalesce(to_json(x."id")::text, 'null') || ']' AS chiave` +
		` FROM public.componente x) r`
	if got != atteso {
		t.Errorf("la formula della versione 1 è cambiata:\n got %s\nwant %s", got, atteso)
	}
	// le classi: un elenco chiuso di tipi resi, e tutto il resto fuori (R-141)
	classi := sqlClassiColonne("componente", []string{"id", "creato_il"})
	const stabili = `'bool'::regtype, 'int2'::regtype, 'int4'::regtype, 'int8'::regtype, 'numeric'::regtype, 'text'::regtype, ` +
		`'varchar'::regtype, 'bpchar'::regtype, 'uuid'::regtype, 'json'::regtype, 'jsonb'::regtype, 'date'::regtype, ` +
		`'timestamp'::regtype, 'inet'::regtype`
	const attese = `SELECT coalesce(string_agg(a.attname::text || ':' || CASE` +
		` WHEN b.oid IN ('timestamptz'::regtype, 'timetz'::regtype) THEN 'fuso'` +
		` WHEN b.oid IN (` + stabili + `) OR b.typtype = 'e' THEN 'testo'` +
		` WHEN b.typcategory = 'A' AND (eb.oid IN (` + stabili + `) OR eb.typtype = 'e') THEN 'testo'` +
		` ELSE 'sessione' END, ' ' ORDER BY a.attname::text COLLATE "C"), '')` +
		` FROM pg_attribute a JOIN pg_type t ON t.oid = a.atttypid` +
		` JOIN pg_type b ON b.oid = CASE WHEN t.typtype = 'd' THEN t.typbasetype ELSE t.oid END` +
		` LEFT JOIN pg_type e ON e.oid = b.typelem AND b.typcategory = 'A'` +
		` LEFT JOIN pg_type eb ON eb.oid = CASE WHEN e.typtype = 'd' THEN e.typbasetype ELSE e.oid END` +
		` WHERE a.attrelid = to_regclass('public.componente') AND a.attnum > 0 AND NOT a.attisdropped` +
		` AND a.attname::text = ANY (ARRAY['id', 'creato_il']::text[])`
	if classi != attese {
		t.Errorf("le classi delle colonne della versione 1 sono cambiate:\n got %s\nwant %s", classi, attese)
	}
}

// TestImpronteTipoNonResoParteNonEseguita (R-141, T-B6-214 precisata): una tabella con una colonna di un tipo fuori
// dall'elenco della versione 1 non ha l'impronta: errore «tipo non reso dalla versione 1», che nomina tabella e
// colonna, e una parte non eseguita, mai nil e mai un'impronta che dipende dalla sessione (la sua impronta non si
// chiede nemmeno al database). Le altre tabelle si controllano lo stesso: un valore cambiato vince, e l'errore dice
// anche la parte non eseguita. PoolDump non si ferma, e il ricontrollo di t.Cleanup non ripete la parte.
func TestImpronteTipoNonResoParteNonEseguita(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	attesa, risposte := copiaACME(t)
	allegato := dataset.ImprontaTabella{Colonne: []string{"id", "periodo"}, Ordine: []string{"id"}, Sha256: strings.Repeat("be", 32)}
	attesa.Impronte.Tabelle["allegato"] = allegato
	risposte[sqlClassiColonne("allegato", allegato.Colonne)] = "id:testo periodo:sessione"

	err := ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa)
	if !errors.Is(err, ErrTipoNonReso) || errors.Is(err, ErrImpronteNonDichiarate) {
		t.Fatalf("una tabella con un tipo non reso: %v; attesa ErrTipoNonReso, mai nil", err)
	}
	for _, frase := range []string{"tipo non reso dalla versione 1", "parte non eseguita", "allegato", "le colonne periodo"} {
		if !strings.Contains(err.Error(), frase) {
			t.Errorf("%v; atteso che dica %q", err, frase)
		}
	}
	f, finito := conTBEsito(t, func(tb testing.TB) { esitoDiControllaCopia(tb, err) })
	if !finito || f.fatale != "" || len(f.errori) != 1 || !strings.HasPrefix(f.errori[0], "NON ESEGUITA: impronte:") {
		t.Errorf("PoolDump con un tipo non reso: errori %q, fatale %q, finito %v; attesa una parte NON ESEGUITA che non ferma la prova", f.errori, f.fatale, finito)
	}
	if err := ricontrollaCopia(ctx, lettoreDiProva(risposte, nil), attesa); err != nil {
		t.Errorf("il ricontrollo ripete la parte non eseguita: %v", err)
	}

	// l'altra tabella si controlla lo stesso: un valore cambiato vince
	imp := improntaComponenteACME()
	risposte[sqlImpronta("componente", imp.Colonne, imp.Ordine, classiComponenteACME)] = "3 " + strings.Repeat("0", 64)
	for nome, controlla := range map[string]func() error{
		"ControllaCopia":   func() error { return ControllaCopia(ctx, lettoreDiProva(risposte, nil), attesa) },
		"ricontrollaCopia": func() error { return ricontrollaCopia(ctx, lettoreDiProva(risposte, nil), attesa) },
	} {
		err := controlla()
		if err == nil || errors.Is(err, ErrTipoNonReso) || !strings.Contains(err.Error(), "valori cambiati: componente") ||
			!strings.Contains(err.Error(), "parte non eseguita: allegato") {
			t.Errorf("%s: %v; attesi i valori cambiati di componente, con la parte non eseguita di allegato", nome, err)
		}
	}
}
