// L1 — le correzioni manuali prima e dopo (A1c-L1-20, la parte di confronto; parte 1 §1.1, C-01; R30 f A), con la
// definizione di R114, precisata dall'utente il 07/10 (domande-a1c.md): un indicatore ricostruito delle correzioni
// necessarie, sullo stesso campione per «prima» e «dopo». Sui soli file decisi: il denominatore (Valutabili) e gli
// esclusi per motivo, che fanno sempre i decisi; «prima» in tre esiti (correzione, nessuna correzione, non
// determinabile), mai per la sola fonte «operatore» e mai per stringa; un'informazione storica mancante (gli export) che
// esce dal denominatore invece di valere zero; il marcatore contato a parte e non sommato a «prima» (D-R114 aperta);
// «dopo» diviso in false associazioni, ambiguità (anche con il badge uguale) e astensioni; la sola revisione a parte;
// la somma campo per campo. La parte di valutazione (le strutture con i figli senza file e i nodi senza lettura, il
// gemello CodiceLettoMarcatore) sta nelle prove di valutazione.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn): il repository
// è pubblico.

package confronto_test

import (
	"reflect"
	"testing"

	"promatec/cockpit/internal/core/confronto"
)

// conLettura: il vecchio con la lettura del vecchio motore, la sua base e il suo marcatore, come li dà valutazione.
func conLettura(v confronto.Vecchio, letto, base, marcatore string) confronto.Vecchio {
	v.CodiceLetto, v.CodiceLettoBase, v.CodiceLettoMarcatore = letto, base, marcatore
	return v
}

// proposto: il nuovo di un file valutato con un solo candidato, il componente n con la base data.
func proposto(n int, base string) confronto.Nuovo {
	return valutato("figlio", "candidato_unico", []string{base}, candidato(n, base))
}

// senzaCandidati: il nuovo di un file valutato senza candidati: «dopo» si può dire (un'astensione), qualunque sia la
// base del vecchio.
func senzaCandidati() confronto.Nuovo {
	return valutato("non_determinabile", "nessun_candidato", nil)
}

// misura: le correzioni di Confronta sui file dati, senza atteso, con le due somme che valgono sempre.
func misura(t *testing.T, file ...confronto.File) confronto.CorrezioniManuali {
	t.Helper()
	c := confronto.Confronta(file, nil, nil).Correzioni
	if c.Decisi != c.Valutabili+c.Esclusi.Totale() {
		t.Errorf("decisi %d, valutabili %d più esclusi %d: %+v", c.Decisi, c.Valutabili, c.Esclusi.Totale(), c)
	}
	if c.Dopo != c.FalseAssociazioni+c.Ambiguita+c.Astensioni {
		t.Errorf("«dopo» %d non è la somma delle sue parti: %+v", c.Dopo, c)
	}
	if c.Prima+c.PrimaMarcatore+c.MarcatoreSoloDaUnLato > c.Valutabili || c.Dopo > c.Valutabili || c.SoloRevisione > c.Valutabili {
		t.Errorf("conteggi fuori dal denominatore: %+v", c)
	}
	return c
}

func TestCorrezioniManualiPrimaEDopo(t *testing.T) {
	conRevisione := func(n confronto.Nuovo, revisioni string) confronto.Nuovo { n.Revisioni = revisioni; return n }
	operatore := decisoSu(2, "7120200", "7120200")
	operatore.Fonte = "operatore"
	senzaComponente := conLettura(decisoSu(11, "7121100", "7121100"), "7121100", "7121100", "")
	senzaComponente.Componente = nil
	file := []confronto.File{
		// 1: la lettura del vecchio motore ha la base del codice deciso, il motore A propone lo stesso: nessuna correzione.
		{AllegatoID: id(1), Vecchio: conLettura(decisoSu(1, "7120100", "7120100"), "7120100", "7120100", ""), Nuovo: proposto(1, "7120100")},
		// 2: la sola fonte operatore, senza la lettura del vecchio motore: non determinabile, fuori dal denominatore (R114).
		{AllegatoID: id(2), Vecchio: operatore, Nuovo: proposto(2, "7120200")},
		// 3: il vecchio motore aveva letto un'altra base, il motore A propone un altro componente con la base decisa:
		// «prima» e una falsa associazione.
		{AllegatoID: id(3), Vecchio: conLettura(decisoSu(3, "7120300", "7120300"), "7120399", "7120399", ""), Nuovo: proposto(9, "7120300")},
		// 4: lettura uguale, il motore A propone lo stesso con un'altra revisione: solo revisione.
		{AllegatoID: id(4), Vecchio: conLettura(decisoSu(4, "7120400", "7120400"), "7120400", "7120400", ""),
			Nuovo: conRevisione(proposto(4, "7120400"), "diverse")},
		// 5: dagli export, senza lettura, in un thread non valutato: escluso per la lettura che manca (il primo motivo).
		{AllegatoID: id(5), Vecchio: decisoSu(5, "7120500", "7120500"), Nuovo: confronto.Nuovo{Valutato: false, Motivo: "thread_non_valutato"}},
		// 6: non deciso, fonte operatore: fuori dalla misura.
		{AllegatoID: id(6), Vecchio: confronto.Vecchio{Stato: "aperta", Fonte: "operatore", Codice: "7120600", Base: "7120600", Leggibile: true},
			Nuovo: proposto(6, "7120600")},
		// 7: la lettura c'è, ma il motore A non ha valutato il thread: fuori dalla copertura del nuovo.
		{AllegatoID: id(7), Vecchio: conLettura(decisoSu(7, "7120700", "7120700"), "7120799", "7120799", ""),
			Nuovo: confronto.Nuovo{Valutato: false, Motivo: "thread_non_valutato"}},
		// 8: il file non valutato in un thread valutato (un documento che non si legge): un'astensione.
		{AllegatoID: id(8), Vecchio: conLettura(decisoSu(8, "7120800", "7120800"), "7120800", "7120800", ""),
			Nuovo: confronto.Nuovo{Valutato: false, Motivo: "documento_non_leggibile", Associazione: "non_valutata"}},
		// 9: valutato senza candidati: un'astensione.
		{AllegatoID: id(9), Vecchio: conLettura(decisoSu(9, "7120900", "7120900"), "7120900", "7120900", ""), Nuovo: senzaCandidati()},
		// 10: il componente deciso fra due candidati: il badge resta uguale, la misura conta un'ambiguità.
		{AllegatoID: id(10), Vecchio: conLettura(decisoSu(10, "7121000", "7121000"), "7121000", "7121000", ""),
			Nuovo: valutato("figlio", "ambiguo", []string{"7121000"}, candidato(12, "7121000"), candidato(10, "7121000"))},
		// 11: il documento deciso senza componente: con la decisione non c'è niente da mettere a fianco.
		{AllegatoID: id(11), Vecchio: senzaComponente, Nuovo: proposto(13, "7121100")},
	}
	e := confronto.Confronta(file, nil, nil)
	attese := confronto.CorrezioniManuali{
		Decisi: 10, Valutabili: 6,
		Esclusi: confronto.EsclusiCorrezioni{SoloOrigineManuale: 1, SenzaLetturaVecchia: 1, DocumentoSenzaComponente: 1, NuovoNonValutato: 1},
		Prima:   1, Dopo: 4, FalseAssociazioni: 1, Ambiguita: 1, Astensioni: 2, SoloRevisione: 1,
	}
	if c := misura(t, file...); c != attese {
		t.Errorf("correzioni\n%+v\nattese\n%+v", c, attese)
	}
	// Il badge non cambia: l'ambiguità è uguale, e «dopo» non è più il numero delle regressioni.
	if r := e.File[9]; r.Badge != confronto.BadgeUguale || r.File.AllegatoID != id(10) {
		t.Errorf("il file ambiguo: %+v", r)
	}
	if e.Conteggi[confronto.BadgeRegressioneSuConfermata] == e.Correzioni.Dopo {
		t.Errorf("«dopo» %d coincide con le regressioni: l'ambiguità non è contata", e.Correzioni.Dopo)
	}
}

// TestPrimaSoloOrigineManuale (R114 §4, 1): la sola fonte «operatore», senza la lettura del vecchio motore, non è una
// correzione: esce dal denominatore con solo_origine_manuale. Con la lettura, la fonte non conta: si confrontano le basi.
func TestPrimaSoloOrigineManuale(t *testing.T) {
	soloOrigine := decisoSu(1, "7120100", "7120100")
	soloOrigine.Fonte = "operatore"
	c := misura(t, confronto.File{AllegatoID: id(1), Vecchio: soloOrigine, Nuovo: proposto(1, "7120100")})
	if c.Prima != 0 || c.Valutabili != 0 || c.Esclusi != (confronto.EsclusiCorrezioni{SoloOrigineManuale: 1}) {
		t.Errorf("la sola origine manuale: %+v", c)
	}
	conLaLettura := conLettura(soloOrigine, "7120100", "7120100", "")
	c = misura(t, confronto.File{AllegatoID: id(1), Vecchio: conLaLettura, Nuovo: proposto(1, "7120100")})
	if c.Prima != 0 || c.Valutabili != 1 || c.Esclusi.Totale() != 0 {
		t.Errorf("fonte operatore con la lettura della stessa base: %+v", c)
	}
}

// TestPrimaSenzaLetturaVecchia (R114 §4, 2): un deciso senza la lettura del vecchio motore (gli export) esce dal
// denominatore e si conta, invece di valere «nessuna correzione»; anche «dopo», sullo stesso campione, non lo conta.
func TestPrimaSenzaLetturaVecchia(t *testing.T) {
	c := misura(t,
		confronto.File{AllegatoID: id(1), Vecchio: decisoSu(1, "7120100", "7120100"), Nuovo: proposto(1, "7120100")},
		confronto.File{AllegatoID: id(2), Vecchio: decisoSu(2, "7120200", "7120200"), Nuovo: senzaCandidati()})
	attese := confronto.CorrezioniManuali{Decisi: 2, Esclusi: confronto.EsclusiCorrezioni{SenzaLetturaVecchia: 2}}
	if c != attese {
		t.Errorf("gli export: %+v, attese %+v", c, attese)
	}
}

// TestPrimaConLeBasiLette (R114 §4, 3; R30 f A, R31 c A; F0-19, R-44): con le due basi lette dalla grammatica la lettura
// del vecchio motore e il codice deciso si confrontano per base; se una delle due basi non si legge, due stringhe
// identiche restano «nessuna correzione» e due stringhe diverse sono non determinabili, mai una correzione per stringa.
// Il nuovo non ha candidati, così «dopo» si può dire sempre (un'astensione) e conta solo «prima».
func TestPrimaConLeBasiLette(t *testing.T) {
	casi := []struct {
		nome                           string
		codice, base, letto, lettoBase string
		prima                          int
		esclusi                        confronto.EsclusiCorrezioni
	}{
		{"stessa base, grafie diverse: non è una correzione", "7120100A", "7120100", "7120100", "7120100", 0, confronto.EsclusiCorrezioni{}},
		{"stessa base, la lettura con la revisione", "7120100", "7120100", "7120100_01", "7120100", 0, confronto.EsclusiCorrezioni{}},
		{"basi diverse: correzione", "7120300", "7120300", "7120399", "7120399", 1, confronto.EsclusiCorrezioni{}},
		{"lettura non leggibile, stringhe diverse: non determinabile", "7120300", "7120300", "ACME-??", "", 0,
			confronto.EsclusiCorrezioni{CodiceNonLeggibile: 1}},
		{"codice deciso non leggibile, stringhe identiche: nessuna correzione", "ACME-77", "", "ACME-77", "", 0, confronto.EsclusiCorrezioni{}},
		{"codice deciso non leggibile, stringhe diverse: non determinabile", "ACME-77", "", "7120300", "7120300", 0,
			confronto.EsclusiCorrezioni{CodiceNonLeggibile: 1}},
		{"nessuna lettura del vecchio motore (export): non determinabile", "7120300", "7120300", "", "", 0,
			confronto.EsclusiCorrezioni{SenzaLetturaVecchia: 1}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			v := conLettura(decisoSu(1, c.codice, c.base), c.letto, c.lettoBase, "")
			m := misura(t, confronto.File{AllegatoID: id(1), Vecchio: v, Nuovo: senzaCandidati()})
			valutabili := 1 - c.esclusi.Totale()
			if m.Prima != c.prima || m.Esclusi != c.esclusi || m.Valutabili != valutabili {
				t.Errorf("prima %d, valutabili %d, esclusi %+v; attesi %d, %d, %+v", m.Prima, m.Valutabili, m.Esclusi, c.prima, valutabili, c.esclusi)
			}
		})
	}
}

// TestAmbiguitaContata (R114 §4, 4): un deciso con il nuovo ambiguo che contiene il componente giusto ha il badge uguale,
// e la misura conta un'ambiguità in «dopo»; anche l'associazione discordante con un candidato solo. Il candidato unico
// non è un'ambiguità.
func TestAmbiguitaContata(t *testing.T) {
	v := conLettura(decisoSu(1, "7120100", "7120100"), "7120100", "7120100", "")
	casi := []struct {
		nome      string
		n         confronto.Nuovo
		ambiguita int
	}{
		{"ambiguo con il componente giusto", valutato("figlio", "ambiguo", []string{"7120100"}, candidato(2, "7120100"), candidato(1, "7120100")), 1},
		{"discordante con il solo componente giusto", valutato("figlio", "discordante", []string{"7120100", "7120200"}, candidato(1, "7120100")), 1},
		{"candidato unico", proposto(1, "7120100"), 0},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			f := confronto.File{AllegatoID: id(1), Vecchio: v, Nuovo: c.n}
			if r := unoSolo(t, f); r.Badge != confronto.BadgeUguale {
				t.Fatalf("badge %s (%s): il badge non cambia", r.Badge, r.Motivo)
			}
			m := misura(t, f)
			if m.Ambiguita != c.ambiguita || m.Dopo != c.ambiguita || m.Valutabili != 1 || m.FalseAssociazioni != 0 || m.Astensioni != 0 {
				t.Errorf("misura %+v, ambiguità attese %d", m, c.ambiguita)
			}
		})
	}
}

// TestRegressioneConPiuCandidatiSbagliati (R-133, RC2; T-B6-187): un deciso con più candidati, nessuno il componente
// deciso con la sua base, e l'associazione ambigua è una regressione. La misura la conta fra le false associazioni, mai
// fra le ambiguità: l'ambiguità vale solo con il componente deciso fra i candidati (il badge uguale), e una falsa
// associazione non si nasconde sotto un'ambiguità (R114: restano visibili tutte e due).
func TestRegressioneConPiuCandidatiSbagliati(t *testing.T) {
	v := conLettura(decisoSu(1, "7120100", "7120100"), "7120100", "7120100", "")
	casi := []struct {
		nome   string
		n      confronto.Nuovo
		motivo string
	}{
		{"due candidati con un'altra base", valutato("figlio", "ambiguo", []string{"7120200"}, candidato(2, "7120200"), candidato(3, "7120200")),
			confronto.MotivoComponenteNonProposto},
		{"due candidati, uno con la base decisa su un altro target", valutato("figlio", "ambiguo", []string{"7120100", "7120200"},
			candidato(2, "7120100"), candidato(3, "7120200")), confronto.MotivoStessaBaseAltroTarget},
		{"discordante con due candidati sbagliati", valutato("figlio", "discordante", []string{"7120200", "7120300"},
			candidato(2, "7120200"), candidato(3, "7120300")), confronto.MotivoComponenteNonProposto},
	}
	attese := confronto.CorrezioniManuali{Decisi: 1, Valutabili: 1, Dopo: 1, FalseAssociazioni: 1}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			f := confronto.File{AllegatoID: id(1), Vecchio: v, Nuovo: c.n}
			if r := unoSolo(t, f); r.Badge != confronto.BadgeRegressioneSuConfermata || r.Motivo != c.motivo {
				t.Fatalf("badge %s (%s), atteso regressione (%s)", r.Badge, r.Motivo, c.motivo)
			}
			if m := misura(t, f); m != attese {
				t.Errorf("misura\n%+v\nattesa\n%+v", m, attese)
			}
		})
	}
}

// TestSoloRevisioneSuiValutabili (R-133, RC1; T-B6-184): la sola revisione si conta sui valutabili, come il resto della
// misura. Un deciso escluso (la sola fonte operatore, senza la lettura del vecchio motore) con il badge uguale e la
// revisione «diverse» non la conta; lo stesso file con la lettura entra nel campione e la conta.
func TestSoloRevisioneSuiValutabili(t *testing.T) {
	n := proposto(1, "7120100")
	n.Revisioni = "diverse"
	soloOrigine := decisoSu(1, "7120100", "7120100")
	soloOrigine.Fonte = "operatore"
	if r := unoSolo(t, confronto.File{AllegatoID: id(1), Vecchio: soloOrigine, Nuovo: n}); r.Badge != confronto.BadgeUguale ||
		r.Revisione.Valore != confronto.RevisioneDiversa {
		t.Fatalf("badge %s, indicatore %+v", r.Badge, r.Revisione)
	}
	escluso := misura(t, confronto.File{AllegatoID: id(1), Vecchio: soloOrigine, Nuovo: n})
	if escluso != (confronto.CorrezioniManuali{Decisi: 1, Esclusi: confronto.EsclusiCorrezioni{SoloOrigineManuale: 1}}) {
		t.Errorf("la sola revisione di un escluso: %+v", escluso)
	}
	valutabile := misura(t, confronto.File{AllegatoID: id(1), Vecchio: conLettura(soloOrigine, "7120100", "7120100", ""), Nuovo: n})
	if valutabile != (confronto.CorrezioniManuali{Decisi: 1, Valutabili: 1, SoloRevisione: 1}) {
		t.Errorf("la sola revisione di un valutabile: %+v", valutabile)
	}
}

// TestPrimaMarcatoreSeparato (R114 §4, 5): la stessa base con i marcatori A e B scritti da tutte e due le parti conta in
// PrimaMarcatore, e Prima non cambia: i due conteggi restano separati e non si sommano, finché l'utente non sceglie il
// titolo della misura (D-R114). Lo stesso marcatore non è niente.
func TestPrimaMarcatoreSeparato(t *testing.T) {
	deciso := decisoSu(1, "7120100B", "7120100")
	deciso.Marcatore = "B"
	diversi := misura(t, confronto.File{AllegatoID: id(1), Vecchio: conLettura(deciso, "7120100A", "7120100", "A"), Nuovo: proposto(1, "7120100")})
	if diversi.PrimaMarcatore != 1 || diversi.Prima != 0 || diversi.MarcatoreSoloDaUnLato != 0 || diversi.Valutabili != 1 {
		t.Errorf("marcatori A e B: %+v", diversi)
	}
	uguali := misura(t, confronto.File{AllegatoID: id(1), Vecchio: conLettura(deciso, "7120100B", "7120100", "B"), Nuovo: proposto(1, "7120100")})
	if uguali.PrimaMarcatore != 0 || uguali.Prima != 0 || uguali.MarcatoreSoloDaUnLato != 0 {
		t.Errorf("lo stesso marcatore: %+v", uguali)
	}
	// Con basi diverse conta solo Prima: il marcatore si guarda soltanto con la stessa base.
	altraBase := misura(t, confronto.File{AllegatoID: id(1), Vecchio: conLettura(deciso, "7120199A", "7120199", "A"), Nuovo: proposto(1, "7120100")})
	if altraBase.Prima != 1 || altraBase.PrimaMarcatore != 0 || altraBase.MarcatoreSoloDaUnLato != 0 {
		t.Errorf("basi diverse e marcatori diversi: %+v", altraBase)
	}
	// Senza le due basi lette il marcatore non si guarda, nemmeno con due stringhe identiche e un marcatore riempito
	// da un lato (un ingresso fuori contratto: valutazione lascia il marcatore vuoto se il codice non si legge).
	illeggibile := decisoSu(1, "ACME-77", "")
	illeggibile.Marcatore = "B"
	senzaBasi := misura(t, confronto.File{AllegatoID: id(1), Vecchio: conLettura(illeggibile, "ACME-77", "", "A"), Nuovo: senzaCandidati()})
	if senzaBasi.Valutabili != 1 || senzaBasi.Prima != 0 || senzaBasi.PrimaMarcatore != 0 || senzaBasi.MarcatoreSoloDaUnLato != 0 {
		t.Errorf("marcatori senza le basi lette: %+v", senzaBasi)
	}
}

// TestMarcatoreSoloDaUnLato (R114 §4, 6): la stessa base con il marcatore scritto da un lato solo, dall'uno o dall'altro,
// conta in MarcatoreSoloDaUnLato, e Prima non cambia.
func TestMarcatoreSoloDaUnLato(t *testing.T) {
	conMarcatore := decisoSu(1, "7120100A", "7120100")
	conMarcatore.Marcatore = "A"
	casi := []struct {
		nome string
		v    confronto.Vecchio
	}{
		{"il marcatore nel codice deciso", conLettura(conMarcatore, "7120100", "7120100", "")},
		{"il marcatore nella lettura del vecchio motore", conLettura(decisoSu(1, "7120100", "7120100"), "7120100A", "7120100", "A")},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			m := misura(t, confronto.File{AllegatoID: id(1), Vecchio: c.v, Nuovo: proposto(1, "7120100")})
			if m.MarcatoreSoloDaUnLato != 1 || m.Prima != 0 || m.PrimaMarcatore != 0 || m.Valutabili != 1 {
				t.Errorf("un marcatore solo: %+v", m)
			}
		})
	}
}

// TestStessoCampionePrimaEDopo (R114): «prima» e «dopo» si contano sullo stesso campione. Un deciso con «prima»
// determinabile, anche una correzione, non conta se «dopo» non si può dire: il thread non valutato dal motore A, il
// documento senza componente, il componente deciso proposto con la base del codice deciso che non si legge. Un file
// non valutato dentro un thread valutato resta nel campione, come astensione.
func TestStessoCampionePrimaEDopo(t *testing.T) {
	correzione := func(n int) confronto.Vecchio {
		return conLettura(decisoSu(n, "7120100", "7120100"), "7120199", "7120199", "")
	}
	senzaComponente := correzione(2)
	senzaComponente.Componente = nil
	nonLeggibile := conLettura(decisoSu(3, "ACME-77", ""), "ACME-77", "", "")
	casi := []struct {
		nome   string
		f      confronto.File
		attese confronto.CorrezioniManuali
	}{
		{"thread non valutato dal motore A",
			confronto.File{AllegatoID: id(1), Vecchio: correzione(1), Nuovo: confronto.Nuovo{Valutato: false, Motivo: "thread_non_valutato"}},
			confronto.CorrezioniManuali{Decisi: 1, Esclusi: confronto.EsclusiCorrezioni{NuovoNonValutato: 1}}},
		{"documento senza componente",
			confronto.File{AllegatoID: id(2), Vecchio: senzaComponente, Nuovo: proposto(2, "7120100")},
			confronto.CorrezioniManuali{Decisi: 1, Esclusi: confronto.EsclusiCorrezioni{DocumentoSenzaComponente: 1}}},
		{"componente deciso proposto, base decisa non leggibile",
			confronto.File{AllegatoID: id(3), Vecchio: nonLeggibile, Nuovo: valutato("figlio", "candidato_unico", nil, candidato(3, ""))},
			confronto.CorrezioniManuali{Decisi: 1, Esclusi: confronto.EsclusiCorrezioni{CodiceNonLeggibile: 1}}},
		{"base decisa non leggibile, il motore A propone un altro componente: falsa associazione",
			confronto.File{AllegatoID: id(4), Vecchio: conLettura(decisoSu(4, "ACME-77", ""), "ACME-77", "", ""), Nuovo: proposto(9, "7120900")},
			confronto.CorrezioniManuali{Decisi: 1, Valutabili: 1, Dopo: 1, FalseAssociazioni: 1}},
		{"file non valutato in un thread valutato: astensione",
			confronto.File{AllegatoID: id(5), Vecchio: correzione(5), Nuovo: confronto.Nuovo{Valutato: false, Motivo: "contenitore", Associazione: "non_valutata"}},
			confronto.CorrezioniManuali{Decisi: 1, Valutabili: 1, Prima: 1, Dopo: 1, Astensioni: 1}},
		{"fuori richiesta su un deciso: astensione",
			confronto.File{AllegatoID: id(6), Vecchio: correzione(6), Nuovo: valutato("fuori_richiesta", "nessun_candidato", []string{"7120100"})},
			confronto.CorrezioniManuali{Decisi: 1, Valutabili: 1, Prima: 1, Dopo: 1, Astensioni: 1}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			if m := misura(t, c.f); m != c.attese {
				t.Errorf("misura\n%+v\nattesa\n%+v", m, c.attese)
			}
		})
	}
}

// TestAggiungiSommaOgniCampo (T-B6-186): Aggiungi somma ogni campo intero della misura, anche quelli degli esclusi; un
// campo nuovo lasciato fuori fa cadere la prova.
func TestAggiungiSommaOgniCampo(t *testing.T) {
	var piena confronto.CorrezioniManuali
	n := 0
	var riempi func(v reflect.Value)
	riempi = func(v reflect.Value) {
		for i := range v.NumField() {
			switch f := v.Field(i); f.Kind() {
			case reflect.Int:
				n++
				f.SetInt(int64(n))
			case reflect.Struct:
				riempi(f)
			default:
				t.Fatalf("campo %s di tipo %s: la somma non sa che cosa farne", v.Type().Field(i).Name, f.Kind())
			}
		}
	}
	riempi(reflect.ValueOf(&piena).Elem())
	var somma confronto.CorrezioniManuali
	somma.Aggiungi(piena)
	somma.Aggiungi(piena)
	var controlla func(a, b reflect.Value, dove string)
	controlla = func(a, b reflect.Value, dove string) {
		for i := range a.NumField() {
			nome := dove + a.Type().Field(i).Name
			if a.Field(i).Kind() == reflect.Struct {
				controlla(a.Field(i), b.Field(i), nome+".")
				continue
			}
			if a.Field(i).Int() != 2*b.Field(i).Int() {
				t.Errorf("%s: %d, atteso %d", nome, a.Field(i).Int(), 2*b.Field(i).Int())
			}
		}
	}
	controlla(reflect.ValueOf(somma), reflect.ValueOf(piena), "")
	// Totale conta ogni motivo degli esclusi, anche uno aggiunto dopo.
	totale, esclusi := 0, reflect.ValueOf(piena.Esclusi)
	for i := range esclusi.NumField() {
		totale += int(esclusi.Field(i).Int())
	}
	if piena.Esclusi.Totale() != totale {
		t.Errorf("Totale %d, la somma dei motivi %d: %+v", piena.Esclusi.Totale(), totale, piena.Esclusi)
	}
}
