// L1 — l'esito di confronto nel suo insieme (piano A, par.3.3.8 e 6.4.6; par.3.4.5; A1c-L1-32, la parte dei codici):
// il determinismo (due esecuzioni, file permutati, atteso permutato: stessi byte e stessa impronta); l'impronta come
// sha256 del canonico senza le parti contro l'atteso; la versione fissa; i conteggi con tutti e cinque i badge; ogni
// codice che il pacchetto emette è dichiarato nel suo codici_diagnostica.go (l'elenco d'oro lo controlla A1a-CAT); la
// forma dei DTO piatti, campo per campo, quella congelata nella fase 0 di B6 (CP.2), che i gemelli di valutazione hanno
// identica (A1c-L1-31 la confronta nel chiamante).
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn): il repository
// è pubblico.

package confronto_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// scenaACME: i file di un thread inventato, uno per riga della tavola dei badge, con una revisione non confrontabile.
func scenaACME() []confronto.File {
	il := time.Date(2026, 10, 2, 8, 40, 0, 123_000_000, time.UTC)
	deciso := decisoSu(1, "7120100A", "7120100")
	deciso.DecisoIl = &il
	deciso.Destinazione = []string{rifComp(1), "identificativo:P7120100"}
	nonConfrontabile := valutato("figlio", "candidato_unico", []string{"7120100"}, conRadici(candidato(1, "7120100"), "7120000"))
	nonConfrontabile.Revisione, nonConfrontabile.Revisioni, nonConfrontabile.MotivoRevisioni = "00", "non_confrontabili", "token_non_attribuito"
	return []confronto.File{
		{AllegatoID: id(11), Vecchio: deciso, Nuovo: nonConfrontabile},
		{AllegatoID: id(12), Vecchio: decisoSu(2, "7120200", "7120200"), Nuovo: valutato("figlio", "candidato_unico", nil, candidato(3, "7120300"))},
		{AllegatoID: id(13), Vecchio: confronto.Vecchio{Stato: "aperta", Codice: "7120500", Base: "7120500", Leggibile: true},
			Nuovo: valutato("fuori_richiesta", "nessun_candidato", []string{"7120500"})},
		{AllegatoID: id(14), Vecchio: confronto.Vecchio{}, Nuovo: valutato("figlio", "ambiguo", nil, candidato(1, "7120100"), candidato(2, "7120200"))},
		{AllegatoID: id(15), Vecchio: confronto.Vecchio{Stato: "scartata", Codice: "7120600", Base: "7120600", Leggibile: true},
			Nuovo: valutato("figlio", "candidato_unico", nil, candidato(6, "7120600"))},
	}
}

func canonico(t *testing.T, v any) []byte {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestConfrontaDeterministico(t *testing.T) {
	file := scenaACME()
	atteso := &confronto.Atteso{ThreadID: id(99), File: []confronto.FileAtteso{
		{AllegatoID: id(11), Atteso: "figlio", TargetBase: "7120100", Radici: []string{"7120000"}, Sezione: "scenario"},
		{AllegatoID: id(12), TargetBase: "7120200", Sezione: "baseline"},
		{AllegatoID: id(13), Atteso: "fuori", Sezione: "scenario"},
		{AllegatoID: id(15), Sezione: "da_rivedere"},
	}, Prodotti: []confronto.ProdottoAtteso{{Base: "7120100"}, {Base: "7120200"}}}
	prodotti := []confronto.ProdottoNuovo{{CodiceRichiesto: "P7120200", Base: "7120200"}, {CodiceRichiesto: "P7120100", Base: "7120100"}}

	primo := confronto.Confronta(file, prodotti, atteso)
	secondo := confronto.Confronta(file, prodotti, atteso)
	permutato := confronto.Confronta([]confronto.File{file[3], file[0], file[4], file[2], file[1]},
		[]confronto.ProdottoNuovo{prodotti[1], prodotti[0]},
		&confronto.Atteso{ThreadID: id(99), File: []confronto.FileAtteso{atteso.File[2], atteso.File[3], atteso.File[0], atteso.File[1]},
			Prodotti: []confronto.ProdottoAtteso{atteso.Prodotti[1], atteso.Prodotti[0]}})
	b1, b2, b3 := canonico(t, primo), canonico(t, secondo), canonico(t, permutato)
	if !bytes.Equal(b1, b2) || !bytes.Equal(b1, b3) {
		t.Fatalf("stessi file, esiti diversi:\n%s\n%s\n%s", b1, b2, b3)
	}
	if len(primo.Impronta) != 64 || primo.Impronta != permutato.Impronta {
		t.Errorf("impronte %q e %q", primo.Impronta, permutato.Impronta)
	}
	for i, r := range primo.File {
		if i > 0 && bytes.Compare(primo.File[i-1].File.AllegatoID[:], r.File.AllegatoID[:]) >= 0 {
			t.Errorf("righe non in ordine di allegato")
		}
	}

	// L'impronta: lo sha256 del canonico senza le parti contro l'atteso, con Impronta vuota; la stessa nell'anteprima.
	copia := primo
	copia.Impronta, copia.ControAtteso, copia.ProdottiControAtteso = "", nil, nil
	if h, err := jsoncanonico.ImprontaDi(copia); err != nil || h != primo.Impronta {
		t.Errorf("impronta %q, ricalcolata %q (%v)", primo.Impronta, h, err)
	}
	if anteprima := confronto.Confronta(file, nil, nil); anteprima.Impronta != primo.Impronta {
		t.Errorf("l'atteso cambia l'impronta: %q e %q", anteprima.Impronta, primo.Impronta)
	}

	// Un cambio che tocca un badge cambia l'impronta.
	cambiato := scenaACME()
	cambiato[1].Nuovo.Candidati[0] = candidato(2, "7120200")
	if confronto.Confronta(cambiato, nil, nil).Impronta == primo.Impronta {
		t.Errorf("un badge diverso, la stessa impronta")
	}

	// I badge della scena, uno per riga della tavola, e i conteggi.
	var badge []string
	for _, r := range primo.File {
		badge = append(badge, string(r.Badge))
	}
	if strings.Join(badge, " ") != "uguale regressione_su_confermata fuori_richiesta nuovo_ancoraggio diverso" {
		t.Errorf("badge %v", badge)
	}
	if fmt.Sprint(primo.Conteggi) != "map[diverso:1 fuori_richiesta:1 nuovo_ancoraggio:1 regressione_su_confermata:1 uguale:1]" {
		t.Errorf("conteggi %v", primo.Conteggi)
	}
}

// TestEsitoVuotoEVersione: senza file l'esito ha la versione, i cinque badge a zero e un'impronta; la versione è fissa
// (cambiarla cambia l'impronta, e questa prova si riscrive). confronto-2: la misura delle correzioni di R114;
// confronto-3: la provenienza della revisione vecchia di R113 (RevisioneDa, ProvenienzaVecchia; T-B6-205).
func TestEsitoVuotoEVersione(t *testing.T) {
	if confronto.VersioneConfronto != "confronto-3" {
		t.Errorf("VersioneConfronto = %q", confronto.VersioneConfronto)
	}
	e := confronto.Confronta(nil, nil, nil)
	if e.VersioneConfronto != confronto.VersioneConfronto || len(e.File) != 0 || len(e.Conteggi) != 5 || len(e.Impronta) != 64 ||
		e.Correzioni != (confronto.CorrezioniManuali{}) || e.Diagnostiche != nil {
		t.Errorf("esito vuoto %+v", e)
	}
	for b, n := range e.Conteggi {
		if n != 0 {
			t.Errorf("conteggio %s = %d", b, n)
		}
	}
}

// TestCodiciEmessiDichiarati (A1c-L1-32, la parte di confronto): ogni diagnostica che il pacchetto emette sulle scene
// delle prove ha un codice dichiarato in codici_diagnostica.go, con gravità e natura del vocabolario della foglia.
func TestCodiciEmessiDichiarati(t *testing.T) {
	dichiarati := map[string]bool{confronto.CodiceRevisioneNonConfrontabile: true}
	file := scenaACME()
	fuori := file[1]
	fuori.AllegatoID = id(16)
	fuori.Nuovo.Revisioni = "forse"
	e := confronto.Confronta(append(file, fuori), nil, nil)
	if len(e.Diagnostiche) != 2 {
		t.Fatalf("diagnostiche %+v", e.Diagnostiche)
	}
	for _, d := range e.Diagnostiche {
		if !dichiarati[d.Codice] || d.Gravita != "nota" || d.Natura != "dati" {
			t.Errorf("diagnostica %+v", d)
		}
	}
	if e.Diagnostiche[0].Rif[0] != id(11).String() || e.Diagnostiche[1].Rif[0] != id(16).String() {
		t.Errorf("ordine delle diagnostiche: %+v", e.Diagnostiche)
	}
}

// formaAttesa: i campi dei DTO piatti, congelati nella fase 0 di B6 (CP.2, con l'emendamento F0-19: CodiceLettoBase
// dopo CodiceLetto; con R114, precisata dall'utente il 07/10: CodiceLettoMarcatore dopo CodiceLettoBase; con R113 B
// ratificata, E2 §2.6: RevisioneDa dopo CodiceLettoMarcatore, 19 campi in Vecchio): nome, tag JSON e tipo,
// nell'ordine. I gemelli di valutazione (FileConfrontabile, VecchioPiatto,
// NuovoPiatto, CandidatoPiatto, ProdottoConfrontabile) hanno gli stessi; cambiare un campo qui vuol dire cambiarlo là,
// prima del commit.
var formaAttesa = map[string][]string{
	"File": {
		"AllegatoID allegato_id uuid.UUID", "Vecchio vecchio confronto.Vecchio", "Nuovo nuovo confronto.Nuovo",
	},
	"Vecchio": {
		"Stato stato string", "Fonte fonte string", "Codice codice string", "Rev rev string", "Base base string",
		"Marcatore marcatore string", "Revisione revisione string", "Leggibile leggibile bool",
		"MotivoLettura motivo_lettura string", "CodiceLetto codice_letto string", "CodiceLettoBase codice_letto_base string",
		"CodiceLettoMarcatore codice_letto_marcatore string", "RevisioneDa revisione_da string",
		"Componente componente,omitempty *uuid.UUID",
		"Documento documento,omitempty *uuid.UUID", "ComponenteProposta componente_proposta,omitempty *uuid.UUID", "SostituitoDa sostituito_da,omitempty *uuid.UUID",
		"DecisoIl deciso_il,omitempty *time.Time", "Destinazione destinazione,omitempty []string",
	},
	"Nuovo": {
		"Valutato valutato bool", "Motivo motivo string", "Basi basi,omitempty []string",
		"Candidati candidati,omitempty []confronto.Candidato", "Collocazione collocazione string",
		"Associazione associazione string", "Disponibilita disponibilita string", "Revisione revisione string",
		"Revisioni revisioni string", "MotivoRevisioni motivo_revisioni string",
	},
	"Candidato": {
		"Target target string", "Livello livello string", "Base base string", "Autorita autorita string",
		"Radici radici,omitempty []string",
	},
	"ProdottoNuovo": {
		"CodiceRichiesto codice_richiesto string", "Base base string", "Fase fase string",
		"Quantita quantita,omitempty *int", "QuantitaDaCella quantita_da_cella bool",
	},
}

// TestFormaDeiDTOPiatti (CP.2; R53 B): i cinque DTO piatti hanno i campi congelati, nell'ordine, con i tag snake_case e
// solo tipi delle foglie: string, bool, int, uuid.UUID, time.Time, i loro puntatori, gli slice e le struct gemelle. Mai
// map, array, interfacce, canali o funzioni (determinismo), mai un campo non esportato.
func TestFormaDeiDTOPiatti(t *testing.T) {
	tipi := map[string]reflect.Type{
		"File": reflect.TypeFor[confronto.File](), "Vecchio": reflect.TypeFor[confronto.Vecchio](),
		"Nuovo": reflect.TypeFor[confronto.Nuovo](), "Candidato": reflect.TypeFor[confronto.Candidato](),
		"ProdottoNuovo": reflect.TypeFor[confronto.ProdottoNuovo](),
	}
	for nome, tipo := range tipi {
		var campi []string
		for i := range tipo.NumField() {
			c := tipo.Field(i)
			if !c.IsExported() {
				t.Errorf("%s.%s non è esportato", nome, c.Name)
			}
			campi = append(campi, c.Name+" "+c.Tag.Get("json")+" "+c.Type.String())
			controllaTipoFoglia(t, nome+"."+c.Name, c.Type)
		}
		if strings.Join(campi, "\n") != strings.Join(formaAttesa[nome], "\n") {
			t.Errorf("%s:\n%s\natteso:\n%s", nome, strings.Join(campi, "\n"), strings.Join(formaAttesa[nome], "\n"))
		}
	}
	// I tag sono quelli che escono nel JSON: un valore pieno li porta tutti.
	b, err := json.Marshal(scenaACME()[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, chiave := range []string{`"allegato_id"`, `"vecchio"`, `"nuovo"`, `"codice_letto"`, `"codice_letto_base"`, `"codice_letto_marcatore"`, `"deciso_il"`, `"motivo_revisioni"`, `"radici"`} {
		if !bytes.Contains(b, []byte(chiave)) {
			t.Errorf("manca %s in %s", chiave, b)
		}
	}
}

func controllaTipoFoglia(t *testing.T, dove string, tipo reflect.Type) {
	t.Helper()
	switch tipo.Kind() {
	case reflect.Pointer, reflect.Slice:
		controllaTipoFoglia(t, dove, tipo.Elem())
	case reflect.String, reflect.Bool, reflect.Int:
		if tipo.PkgPath() != "" {
			t.Errorf("%s: tipo con nome %s, non un tipo delle foglie", dove, tipo)
		}
	case reflect.Struct:
		switch tipo.String() {
		case "uuid.UUID", "time.Time", "confronto.Vecchio", "confronto.Nuovo", "confronto.Candidato":
		default:
			t.Errorf("%s: struct %s fuori dall'elenco", dove, tipo)
		}
	case reflect.Array:
		if tipo.String() != "uuid.UUID" {
			t.Errorf("%s: array %s", dove, tipo)
		}
	default:
		t.Errorf("%s: tipo %s (%s) vietato nei DTO piatti", dove, tipo, tipo.Kind())
	}
}
