package motorea

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — le sei grammatiche sintetiche del motore A (piano A, par.4.7.3) e gli aiuti che le altre prove del
// pacchetto usano: ognuna compila da sola e tutte insieme, con gli esempi verificati, e gli esempi della
// forma riservata restano non verificati (A1a-ESE; R20 a, b).
//
// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un
// dato dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il
// giorno in cui un cliente cambia convenzione. Il cliente è ACME, le famiglie hanno i nomi neutri del
// par.4.7.3 (acme-prefisso, acme-marcatore, acme-documento, acme-punti, acme-etichetta, acme-campo-separato);
// basi, letterali ed esempi sono inventati e attivano un meccanismo, mai il significato di un cliente (R20 a).
// Le prove citano i requisiti (A-C06, D2, R7, A1a-CNF), mai i casi degli attesi.

// clienteACME: l'UUID inventato del cliente delle grammatiche sintetiche.
var clienteACME = uuid.MustParse("00000000-0000-4000-8000-00000000ac01")

const ragioneSocialeACME = "ACME S.p.A."

// limitiACME: limiti sintetici con i valori iniziali del piano (par.4.6.1) e una versione inventata. Nelle
// prove arrivano così, come li darebbe l'indice (R43 B): il motore non ne ha di suoi.
func limitiACME() grammatica.Limiti {
	return grammatica.Limiti{
		Versione: "limiti-acme-1",
		Grammatica: grammatica.LimitiGrammatica{
			MaxFamiglie: 20, MaxFormePerFamiglia: 16, MaxPartiPerForma: 12, MaxEsempi: 64,
			MaxLunghezzaPattern: 64, MaxRipetizione: 40, MaxLunghezzaLetterale: 40,
		},
		Riconoscimento: grammatica.LimitiRiconoscimento{
			MaxByteUnita: 1048576, MaxUnitaDocumento: 10000, MaxLetturePerUnita: 1000, MaxLettureDocumento: 20000,
		},
		Anteprima: grammatica.LimitiAnteprima{TempoMassimoMs: 20000},
	}
}

// ---- i pezzi delle grammatiche ----

func pBase() grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}
}

func pRipetizione() grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteRipetizioneBase, Min: 1, Max: 1}
}

func pSep(letterali ...string) grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteSeparatore, Letterali: letterali, Min: 1, Max: 1}
}

func pMarcatore(letterali ...string) grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteMarcatore, Letterali: letterali, Min: 1, Max: 1}
}

func pToken(rimando string, letterali ...string) grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteToken, Letterali: letterali, Rimando: rimando, Min: 1, Max: 1}
}

// pRif: una parte che cita una regola della famiglia (etichetta, affisso, revisione, decorazione).
func pRif(tipo, rif string, min int) grammatica.Parte {
	return grammatica.Parte{Tipo: tipo, Rif: rif, Min: min, Max: 1}
}

func classe(c grammatica.ClasseConfine) *grammatica.ClasseConfine { return &c }

func forma(id string, selettori []string, parti ...grammatica.Parte) grammatica.FormaCodice {
	return grammatica.FormaCodice{ID: id, Selettori: selettori, Stato: grammatica.StatoAttiva, Completa: true, Parti: parti}
}

func base(segmenti ...grammatica.SegmentoBase) grammatica.Base {
	return grammatica.Base{
		Segmenti: segmenti, Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
}

func segPattern(nome, pattern, separatore string) grammatica.SegmentoBase {
	return grammatica.SegmentoBase{Nome: nome, Pattern: pattern, Separatore: separatore, Identitario: true}
}

func segLetterale(nome, letterale, separatore string) grammatica.SegmentoBase {
	return grammatica.SegmentoBase{Nome: nome, Letterale: letterale, Separatore: separatore, Identitario: true}
}

func sel(s ...string) []string { return s }

func positivo(id, selettore, testo string, letture ...grammatica.LetturaAttesa) grammatica.EsempioCodice {
	return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: selettore, Testo: testo,
		Atteso: grammatica.AttesoEsempio{Letture: letture}}
}

func negativo(id, selettore, testo string) grammatica.EsempioCodice {
	return grammatica.EsempioCodice{ID: id, Origine: grammatica.OrigineSintetico, Selettore: selettore, Testo: testo,
		Atteso: grammatica.AttesoEsempio{Nessuna: true}}
}

// ---- le sei grammatiche del par.4.7.3 ----

// famPrefisso: acme-prefisso. Base 712 più quattro cifre; affisso P di fase prototipo, riconosciuto e
// attribuito su nome_file, corpo e storia (D2; R48 A); involucro «ACME-030»; stato PDM «IN_WORK»; token «00»
// conservato (Q1). La forma del nome PDF ha il confine destro alnum_ascii_o_spazio: senza, leggerebbe il
// codice anche dentro il nome con il token (A1a-CNF).
func famPrefisso() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-prefisso", Namespace: "acme-prefisso",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("codice", "712[0-9]{4}", "")),
		Forme: []grammatica.FormaCodice{
			forma("cartiglio", sel("cartiglio.codice"), pBase()),
			forma("mail", sel("corpo", "storia"), pRif(grammatica.TipoParteAffisso, "P", 1), pBase()),
			func() grammatica.FormaCodice {
				f := forma("nome-pdf", sel("nome_file"),
					pRif(grammatica.TipoParteDecorazione, "pacchetto", 1), pRif(grammatica.TipoParteAffisso, "P", 1), pBase())
				f.ConfineDopo = classe(grammatica.ConfineAlnumASCIIOSpazio)
				return f
			}(),
			forma("nome-step", sel("nome_file"),
				pRif(grammatica.TipoParteDecorazione, "pacchetto", 1), pRif(grammatica.TipoParteAffisso, "P", 1), pBase(),
				pSep(" "), pToken("Q1", "00"), pSep(" "), pRif(grammatica.TipoParteDecorazione, "pdm", 1)),
		},
		Affissi: []grammatica.Affisso{{
			ID: "P", Letterali: []string{"P"}, Posizione: grammatica.PosizionePrefisso,
			Riconoscimento: sel("corpo", "nome_file", "storia"), Attribuzione: sel("corpo", "nome_file", "storia"),
			Valore: &grammatica.ValoreQualificatore{Fase: grammatica.FasePrototipo},
		}},
		Decorazioni: []grammatica.Decorazione{
			{ID: "pacchetto", Tipo: grammatica.TipoDecorazioneInvolucro, Sottotipo: grammatica.SottotipoRiferimentoPacchetto,
				Letterali: []string{"ACME-030"}, Selettori: sel("nome_file")},
			{ID: "pdm", Tipo: grammatica.TipoDecorazioneStatoPDM, Letterali: []string{"IN_WORK"}, Selettori: sel("nome_file")},
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-cartiglio", "cartiglio.codice", "7120100", grammatica.LetturaAttesa{Forma: "cartiglio", Base: "7120100"}),
			positivo("e-mail", "corpo", "P7120100", grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Affissi: []string{"P"}}),
			positivo("e-mail-storia", "storia", "P7120100", grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Affissi: []string{"P"}}),
			positivo("e-nome-pdf", "nome_file", "ACME-030P7120100.pdf",
				grammatica.LetturaAttesa{Forma: "nome-pdf", Base: "7120100", Affissi: []string{"P"}, Decorazioni: []string{"ACME-030"}}),
			positivo("e-nome-step", "nome_file", "ACME-030P7120100 00 IN_WORK.stp",
				grammatica.LetturaAttesa{Forma: "nome-step", Base: "7120100", Affissi: []string{"P"}, Decorazioni: []string{"ACME-030", "IN_WORK"}}),
			negativo("n-cartiglio-con-p", "cartiglio.codice", "P7120100"),
			negativo("n-corpo-con-involucro", "corpo", "ACME-030P7120100"),
		},
	}
}

// famMarcatore: acme-marcatore, con la A fra la base e la revisione di una cifra (D1). Le forme del nome con
// e senza «_», del cartiglio, del pacchetto con due basi, del DXF con il token «_drw_<n>» conservato (D10),
// dello STEP con il confine destro parola_ascii e dello STEP con il token «PRT» (D10).
func famMarcatore() grammatica.FamigliaCodice {
	step := forma("step", sel("nodo_step.id", "radice_step.id"), pBase(), pMarcatore("A"))
	step.ConfineDopo = classe(grammatica.ConfineParolaASCII)
	return grammatica.FamigliaCodice{
		ID: "acme-marcatore", Namespace: "acme-marcatore",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("numero", "9[1-3][0-9]{5}", "")),
		Forme: []grammatica.FormaCodice{
			forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(grammatica.TipoParteRevisione, "rev-a", 1)),
			forma("dxf", sel("nome_file"), pSep("dxf_"), pBase(), pMarcatore("a"), pSep("_drw_"), pToken("D10", "1", "2")),
			forma("nome", sel("nome_file"), pBase(), pMarcatore("A"), pRif(grammatica.TipoParteRevisione, "rev-a", 1)),
			forma("nome-sottolineato", sel("nome_file"), pBase(), pMarcatore("A"), pSep("_"), pRif(grammatica.TipoParteRevisione, "rev-a", 1)),
			forma("pacchetto", sel("nome_file"), pBase()),
			step,
			forma("step-prt", sel("radice_step.id"), pBase(), pMarcatore("A"), pSep("_"), pToken("D10", "PRT")),
		},
		Revisioni: []grammatica.RegolaRevisione{{
			ID: "rev-a", Selettori: sel("cartiglio.codice", "nome_file"), Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]", Significato: grammatica.SignificatoNessuno}},
		}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-cartiglio", "cartiglio.codice", "9123456A2",
				grammatica.LetturaAttesa{Forma: "cartiglio", Base: "9123456", Marcatore: "A", Revisione: "2"}),
			positivo("e-dxf", "nome_file", "dxf_9123456a_drw_1.dxf", grammatica.LetturaAttesa{Forma: "dxf", Base: "9123456", Marcatore: "a"}),
			positivo("e-nome", "nome_file", "9123456A2.pdf", grammatica.LetturaAttesa{Forma: "nome", Base: "9123456", Marcatore: "A", Revisione: "2"}),
			positivo("e-nome-sottolineato", "nome_file", "9123456A_2.pdf",
				grammatica.LetturaAttesa{Forma: "nome-sottolineato", Base: "9123456", Marcatore: "A", Revisione: "2"}),
			positivo("e-pacchetto", "nome_file", "9123456_9123457.zip",
				grammatica.LetturaAttesa{Forma: "pacchetto", Base: "9123456"}, grammatica.LetturaAttesa{Forma: "pacchetto", Base: "9123457"}),
			positivo("e-step-nodo", "nodo_step.id", "9123456A", grammatica.LetturaAttesa{Forma: "step", Base: "9123456", Marcatore: "A"}),
			positivo("e-step-prt", "radice_step.id", "9123456A_PRT", grammatica.LetturaAttesa{Forma: "step-prt", Base: "9123456", Marcatore: "A"}),
			positivo("e-step-radice", "radice_step.id", "9123456A", grammatica.LetturaAttesa{Forma: "step", Base: "9123456"}),
			negativo("n-cartiglio-cifra", "cartiglio.codice", "3"),
			negativo("n-nodo-prt", "nodo_step.id", "9123456A_PRT"),
		},
	}
}

// famMarcatoreX: acme-marcatore-x, la famiglia X dello stesso spazio di codici (R7): base di sei o sette
// cifre, X come la A, revisione facoltativa sui nodi e sulle radici, ruoli {componente} e la categoria
// minuteria dichiarata dalla famiglia.
func famMarcatoreX() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-marcatore-x", Namespace: "acme-marcatore",
		Ruoli:     []grammatica.Ruolo{grammatica.RuoloComponente},
		Categorie: []grammatica.Categoria{grammatica.CategoriaMinuteria},
		Base:      base(segPattern("numero", "[0-9]{6,7}", "")),
		Forme: []grammatica.FormaCodice{
			forma("x-nodo", sel("nodo_step.id"), pBase(), pMarcatore("X"), pRif(grammatica.TipoParteRevisione, "rev-x", 0)),
			forma("x-nome", sel("nome_file"), pBase(), pMarcatore("X"), pSep("_"), pRif(grammatica.TipoParteRevisione, "rev-x", 1)),
			forma("x-radice", sel("radice_step.id"), pBase(), pMarcatore("X"), pRif(grammatica.TipoParteRevisione, "rev-x", 0),
				pSep("_"), pToken("D10", "PRT")),
		},
		Revisioni: []grammatica.RegolaRevisione{{
			ID: "rev-x", Selettori: sel("nodo_step.id", "nome_file", "radice_step.id"), Stato: grammatica.StatoAttiva,
			Sorgente: grammatica.SorgenteInline,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]", Significato: grammatica.SignificatoNessuno}},
		}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-x-nodo", "nodo_step.id", "912345X1", grammatica.LetturaAttesa{Forma: "x-nodo", Base: "912345", Marcatore: "X", Revisione: "1"}),
			positivo("e-x-nodo-senza-rev", "nodo_step.id", "9123456X", grammatica.LetturaAttesa{Forma: "x-nodo", Base: "9123456", Marcatore: "X"}),
			positivo("e-x-nome", "nome_file", "912345X_1.stp", grammatica.LetturaAttesa{Forma: "x-nome", Base: "912345", Revisione: "1"}),
			positivo("e-x-radice", "radice_step.id", "912345X1_PRT", grammatica.LetturaAttesa{Forma: "x-radice", Base: "912345", Revisione: "1"}),
		},
	}
}

// famDocumento: acme-documento. Involucri tecnici «ACME_MOD_3D» e «ACME_DRW_2D», livello B1/B2 (D8: mai
// revisione), suffisso documento con il token «1» (Q1) e la base ripetuta, che deve concordare (D2).
func famDocumento() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-documento", Namespace: "acme-documento",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("codice", "9[78][0-9]{6}", "")),
		Forme: []grammatica.FormaCodice{
			forma("bozza", sel("nome_file"), pBase(), pSep("-BOZZA")),
			forma("2d-dxf", sel("nome_file"), pBase(), pSep("-"), pRif(grammatica.TipoParteDecorazione, "drw2d", 1)),
			forma("3d", sel("nome_file"), pBase(), pSep("-"), pRif(grammatica.TipoParteDecorazione, "mod3d", 1), pSep("-"),
				pRif(grammatica.TipoParteDecorazione, "livello", 1)),
			forma("documento", sel("nome_file"), pBase(), pRif(grammatica.TipoParteDecorazione, "doc", 1)),
		},
		Decorazioni: []grammatica.Decorazione{
			{ID: "doc", Tipo: grammatica.TipoDecorazioneSuffissoDocumento, Selettori: sel("nome_file"),
				Parti: []grammatica.Parte{pSep("#"), pToken("Q1", "1"), pSep("#R"), pRipetizione(), pSep("#")}},
			{ID: "drw2d", Tipo: grammatica.TipoDecorazioneInvolucro, Sottotipo: grammatica.SottotipoTecnico,
				Letterali: []string{"ACME_DRW_2D"}, Selettori: sel("nome_file")},
			{ID: "livello", Tipo: grammatica.TipoDecorazioneLivelloNomeFile, Letterali: []string{"B1", "B2"}, Selettori: sel("nome_file")},
			{ID: "mod3d", Tipo: grammatica.TipoDecorazioneInvolucro, Sottotipo: grammatica.SottotipoTecnico,
				Letterali: []string{"ACME_MOD_3D"}, Selettori: sel("nome_file")},
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-bozza", "nome_file", "97123456-BOZZA.pdf", grammatica.LetturaAttesa{Forma: "bozza", Base: "97123456"}),
			positivo("e-2d-dxf", "nome_file", "97123456-ACME_DRW_2D-coda.dxf",
				grammatica.LetturaAttesa{Forma: "2d-dxf", Base: "97123456", Decorazioni: []string{"ACME_DRW_2D"}}),
			positivo("e-3d", "nome_file", "97123456-ACME_MOD_3D-B1.stp",
				grammatica.LetturaAttesa{Forma: "3d", Base: "97123456", Decorazioni: []string{"ACME_MOD_3D", "B1"}}),
			positivo("e-documento", "nome_file", "97123456#1#R97123456#.pdf",
				grammatica.LetturaAttesa{Forma: "documento", Base: "97123456"}),
		},
	}
}

// famPunti: acme-punti, la base a punti con il segmento T. Affisso S riconosciuto e non attribuito (D5);
// revisione «/nn» con forte e debole e il token sospeso «xx» (D4, D5); revisione «_nn» solo come token
// (D-06); forma parziale senza T, con il confine destro parola_ascii_o_punto_cifra (A-C07).
func famPunti() grammatica.FamigliaCodice {
	completa := forma("completa", sel("corpo", "oggetto"),
		pRif(grammatica.TipoParteAffisso, "S", 0), pBase(), pRif(grammatica.TipoParteRevisione, "rev-barra", 0))
	completa.ConfineDopo = classe(grammatica.ConfineParolaASCII)
	parziale := forma("parziale", sel("corpo", "oggetto"),
		pRif(grammatica.TipoParteAffisso, "S", 0), pBase(), pRif(grammatica.TipoParteRevisione, "rev-barra", 0))
	parziale.Completa = false
	parziale.SegmentiMancanti = []string{"T"}
	parziale.ConfineDopo = classe(grammatica.ConfineParolaASCIIOPuntoCifra)
	return grammatica.FamigliaCodice{
		ID: "acme-punti", Namespace: "acme-punti",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base: base(segLetterale("prefisso", "9", ""), segPattern("gruppo", "[0-9]{3}", "."),
			segPattern("numero", "[0-9]{4}", "."), segPattern("T", "[0-9]", ".")),
		Forme: []grammatica.FormaCodice{
			completa,
			forma("completa-sottolineato", sel("corpo", "oggetto"),
				pRif(grammatica.TipoParteAffisso, "S", 0), pBase(), pRif(grammatica.TipoParteRevisione, "rev-sottolineato", 1)),
			parziale,
		},
		Affissi: []grammatica.Affisso{{ID: "S", Letterali: []string{"S"}, Posizione: grammatica.PosizionePrefisso,
			Riconoscimento: sel("corpo", "oggetto")}},
		Revisioni: []grammatica.RegolaRevisione{
			{ID: "rev-barra", Selettori: sel("corpo", "oggetto"), Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
				Separatori: []string{"/"},
				Segmenti: []grammatica.SegmentoRevisione{
					{Nome: "forte", Pattern: "[0-9]", Significato: grammatica.SignificatoForte},
					{Nome: "debole", Pattern: "[0-9]", Significato: grammatica.SignificatoDebole},
				},
				TokenSospesi: []grammatica.TokenSospeso{{ID: "xx", Pattern: "xx", Riserva: "Q-ACME-1"}}},
			{ID: "rev-sottolineato", Selettori: sel("corpo", "oggetto"), Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
				Separatori:   []string{"_"},
				TokenSospesi: []grammatica.TokenSospeso{{ID: "nn", Pattern: "[0-9]{2}", Riserva: "Q-ACME-1"}}},
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-completa", "corpo", "9.123.4567.3/01", grammatica.LetturaAttesa{Forma: "completa", Base: "9.123.4567.3", Revisione: "01"}),
			positivo("e-completa-xx", "corpo", "9.123.4567.3/xx", grammatica.LetturaAttesa{Forma: "completa", Base: "9.123.4567.3"}),
			positivo("e-completa-s", "oggetto", "S9.123.4567.3", grammatica.LetturaAttesa{Forma: "completa", Base: "9.123.4567.3", Affissi: []string{"S"}}),
			positivo("e-parziale", "corpo", "9.123.4567",
				grammatica.LetturaAttesa{Forma: "parziale", Base: "9.123.4567", Mancanti: []string{"T"}}),
			positivo("e-sottolineato", "oggetto", "9.123.4567.3_30",
				grammatica.LetturaAttesa{Forma: "completa-sottolineato", Base: "9.123.4567.3"}),
			negativo("n-lettere-dopo", "corpo", "9.123.4567.3A"),
		},
	}
}

// famEtichetta: acme-etichetta, la base a punti con l'affisso GT in coda, attribuito con la destinazione
// ricambio sul corpo e sull'oggetto (D3).
func famEtichetta() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-etichetta", Namespace: "acme-etichetta",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base: base(segPattern("primo", "[0-9]{2}", ""), segPattern("secondo", "[0-9]{4}", "."),
			segPattern("terzo", "[0-9]{4}", ".")),
		Forme: []grammatica.FormaCodice{
			forma("corpo", sel("corpo", "oggetto"), pBase(), pRif(grammatica.TipoParteAffisso, "GT", 0)),
		},
		Affissi: []grammatica.Affisso{{ID: "GT", Letterali: []string{"GT"}, Posizione: grammatica.PosizioneSuffisso,
			Riconoscimento: sel("corpo", "oggetto"), Attribuzione: sel("corpo", "oggetto"),
			Valore: &grammatica.ValoreQualificatore{Destinazione: grammatica.DestinazioneRicambio}}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-gt", "corpo", "12.3456.7890GT", grammatica.LetturaAttesa{Forma: "corpo", Base: "12.3456.7890", Affissi: []string{"GT"}}),
			positivo("e-senza-gt", "oggetto", "12.3456.7890", grammatica.LetturaAttesa{Forma: "corpo", Base: "12.3456.7890"}),
		},
	}
}

// famEtichettaPN: acme-etichetta-pn, lo stesso spazio di codici con l'etichetta obbligatoria «PN», a uno
// spazio al più (A-C08).
func famEtichettaPN() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-etichetta-pn", Namespace: "acme-etichetta",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("numero", "[0-9]{7}", "")),
		Etichette: []grammatica.Etichetta{{ID: "pn", Letterali: []string{"PN"}, SpaziMax: 1, Selettori: sel("corpo", "oggetto"),
			Obbligatoria: true}},
		Forme: []grammatica.FormaCodice{
			forma("pn", sel("corpo", "oggetto"), pRif(grammatica.TipoParteEtichetta, "pn", 1), pBase()),
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-pn", "corpo", "PN 7654321", grammatica.LetturaAttesa{Forma: "pn", Base: "7654321"}),
			positivo("e-pn-oggetto", "oggetto", "PN 7654321", grammatica.LetturaAttesa{Forma: "pn", Base: "7654321"}),
			negativo("n-senza-etichetta", "corpo", "7654321"),
			negativo("n-etichetta-attaccata", "corpo", "XPN 7654321"),
			negativo("n-due-spazi", "corpo", "PN  7654321"),
		},
	}
}

// famCampoSeparato: acme-campo-separato. Base con il «+» nel prefisso letterale; forma del nome con il
// confine destro alnum_ascii_o_spazio, forma del nome con due spazi e il token «00» (Q1), forma del
// cartiglio; revisione in campo separato a due cifre, senza forte e debole (D9), verificata con D-07.
func famCampoSeparato() grammatica.FamigliaCodice {
	nome := forma("nome", sel("nome_file"), pBase())
	nome.ConfineDopo = classe(grammatica.ConfineAlnumASCIIOSpazio)
	return grammatica.FamigliaCodice{
		ID: "acme-campo-separato", Namespace: "acme-campo-separato",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base: base(segLetterale("prefisso", "T+300", ""), segPattern("centrale", "0[0-9]{5}", "."),
			segPattern("successivo", "0[0-9]0", ".")),
		Forme: []grammatica.FormaCodice{
			forma("cartiglio", sel("cartiglio.codice"), pBase()),
			nome,
			forma("nome-token", sel("nome_file"), pBase(), pSep("  "), pToken("Q1", "00")),
		},
		Revisioni: []grammatica.RegolaRevisione{{
			ID: "rev-campo", Selettori: sel("cartiglio.revisione"), Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteCampoSeparato,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]{2}", Significato: grammatica.SignificatoNessuno}},
		}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-cartiglio", "cartiglio.codice", "T+300.012345.010", grammatica.LetturaAttesa{Forma: "cartiglio", Base: "T+300.012345.010"}),
			positivo("e-nome", "nome_file", "T+300.012345.010.pdf", grammatica.LetturaAttesa{Forma: "nome", Base: "T+300.012345.010"}),
			positivo("e-nome-token", "nome_file", "T+300.012345.010  00.zip", grammatica.LetturaAttesa{Forma: "nome-token", Base: "T+300.012345.010"}),
			positivo("e-revisione", "cartiglio.revisione", "01", grammatica.LetturaAttesa{Revisione: "01"}),
			negativo("n-centrale", "nome_file", "T+300.112345.010.pdf"),
		},
	}
}

// famLavagna: acme-campo-separato-lavagna, una famiglia con la sola forma riservata: i suoi esempi non si
// verificano e restano «non verificati», mai «passati» (par.2.0 deciso A; R34 b).
func famLavagna() grammatica.FamigliaCodice {
	lavagna := forma("lavagna", sel("cartiglio.codice", "nome_file"), pBase())
	lavagna.Stato = grammatica.StatoRiservata
	return grammatica.FamigliaCodice{
		ID: "acme-campo-separato-lavagna", Namespace: "acme-campo-separato",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base: base(segLetterale("prefisso", "T300", ""), segPattern("centrale", "0[0-9]{5}", "."),
			segPattern("successivo", "0[0-9]0", ".")),
		Forme: []grammatica.FormaCodice{lavagna},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-lavagna", "nome_file", "T300.012345.010.pdf", grammatica.LetturaAttesa{Forma: "lavagna", Base: "T300.012345.010"}),
			negativo("n-lavagna", "nome_file", "T300.012345.011.pdf"),
		},
	}
}

// grammaticaACME: le famiglie date, con il cliente ACME, un profilo parziale e una riserva inventata.
func grammaticaACME(famiglie ...grammatica.FamigliaCodice) grammatica.Grammatica {
	return grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: ragioneSocialeACME},
		Profilo: grammatica.Profilo{Stato: grammatica.ProfiloParziale,
			Riserve: []grammatica.Riserva{{ID: "Q-ACME-1", Motivo: "domanda aperta inventata"}}},
		Famiglie: famiglie,
	}
}

// tutteLeFamiglie: le sei grammatiche del par.4.7.3, con le famiglie gemelle (X, PN, lavagna).
func tutteLeFamiglie() []grammatica.FamigliaCodice {
	return []grammatica.FamigliaCodice{famPrefisso(), famMarcatore(), famMarcatoreX(), famDocumento(), famPunti(),
		famEtichetta(), famEtichettaPN(), famCampoSeparato(), famLavagna()}
}

// ---- compilazione e lettura ----

// fileJSON: la grammatica come file v1, come la scriverebbe il dataset.
func fileJSON(t *testing.T, g grammatica.Grammatica) []byte {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// diagnosticheDi: le diagnostiche di un *evidenze.ErroreContratto.
func diagnosticheDi(err error) []evidenze.Diagnostica {
	var ec *evidenze.ErroreContratto
	if errors.As(err, &ec) {
		return ec.Diagnostiche
	}
	return nil
}

// compila passa dalla porta del prodotto: il file v1, NuovoSnapshot con i limiti, poi CompilaVerificato con
// gli stessi limiti. Con uno snapshot che non nasce restituisce le diagnostiche della validazione.
func compila(t *testing.T, g grammatica.Grammatica, lim grammatica.Limiti) (*Motore, []evidenze.Diagnostica, error) {
	t.Helper()
	s, err := grammatica.NuovoSnapshot(fileJSON(t, g), lim)
	if err != nil {
		return nil, diagnosticheDi(err), err
	}
	return CompilaVerificato(s, lim)
}

// compilaBene: la grammatica deve compilare, senza errori, con gli esempi verificati.
func compilaBene(t *testing.T, g grammatica.Grammatica) (*Motore, []evidenze.Diagnostica) {
	t.Helper()
	m, d, err := compila(t, g, limitiACME())
	if err != nil || m == nil {
		t.Fatalf("la grammatica non compila: %v\n%s", err, elenco(d))
	}
	for _, x := range d {
		if x.Gravita == evidenze.GravitaErrore {
			t.Fatalf("diagnostica d'errore con un motore valido: %+v", x)
		}
	}
	return m, d
}

// compilaErrore: la grammatica non deve attivarsi, con un *evidenze.ErroreContratto che porta il codice.
func compilaErrore(t *testing.T, g grammatica.Grammatica, codice string) []evidenze.Diagnostica {
	t.Helper()
	m, d, err := compila(t, g, limitiACME())
	if m != nil {
		t.Fatalf("motore compilato, atteso il rifiuto con %s", codice)
	}
	var ec *evidenze.ErroreContratto
	if !errors.As(err, &ec) {
		t.Fatalf("errore %T %v, atteso un *evidenze.ErroreContratto", err, err)
	}
	if len(conCodice(ec.Diagnostiche, codice)) == 0 {
		t.Fatalf("l'errore non porta %s:\n%s", codice, elenco(ec.Diagnostiche))
	}
	for _, x := range conCodice(d, codice) {
		if x.Gravita != evidenze.GravitaErrore {
			t.Errorf("%s con gravità %s, attesa errore", codice, x.Gravita)
		}
	}
	return ec.Diagnostiche
}

// selettore: la forma testuale letta con la funzione della foglia.
func selettore(t *testing.T, s string) evidenze.Selettore {
	t.Helper()
	x, err := evidenze.LeggiSelettore(s)
	if err != nil {
		t.Fatalf("selettore %q: %v", s, err)
	}
	return x
}

// riconosci chiama Riconosci e controlla gli invarianti di ogni lettura: intervallo su confini di runa,
// Originale uguale al testo dell'intervallo, selettore della lettura, ordine (inizio, fine, famiglia, forma),
// parti dentro la lettura.
func riconosci(t *testing.T, m *Motore, s, testo string) ([]LetturaForma, []evidenze.Diagnostica) {
	t.Helper()
	x := selettore(t, s)
	letture, diag := m.Riconosci(x, testo)
	for i, l := range letture {
		iv := l.Intervallo
		if iv.Inizio < 0 || iv.Fine > len(testo) || iv.Inizio >= iv.Fine {
			t.Fatalf("lettura %d %s/%s: intervallo %v fuori dal testo di %d byte", i, l.Famiglia, l.Forma, iv, len(testo))
		}
		if !utf8.RuneStart(testo[iv.Inizio]) || (iv.Fine < len(testo) && !utf8.RuneStart(testo[iv.Fine])) {
			t.Errorf("lettura %d: intervallo %v a metà di una runa", i, iv)
		}
		if l.Originale != testo[iv.Inizio:iv.Fine] {
			t.Errorf("lettura %d: Originale %q, il testo dell'intervallo è %q", i, l.Originale, testo[iv.Inizio:iv.Fine])
		}
		if l.Selettore != x {
			t.Errorf("lettura %d: selettore %v, atteso %v", i, l.Selettore, x)
		}
		dentro := func(cosa string, p evidenze.Intervallo) {
			if p.Inizio < iv.Inizio || p.Fine > iv.Fine || p.Inizio > p.Fine {
				t.Errorf("lettura %d: %s in %v, fuori dalla lettura %v", i, cosa, p, iv)
			}
		}
		for _, sg := range l.Base.Segmenti {
			dentro("segmento "+sg.Nome, sg.Intervallo)
			if sg.Originale != testo[sg.Intervallo.Inizio:sg.Intervallo.Fine] {
				t.Errorf("lettura %d: segmento %s %q, il testo dice %q", i, sg.Nome, sg.Originale, testo[sg.Intervallo.Inizio:sg.Intervallo.Fine])
			}
		}
		for _, a := range l.Affissi {
			dentro("affisso", a.Intervallo)
		}
		for _, d := range l.Decorazioni {
			dentro("decorazione", d.Intervallo)
		}
		for _, p := range []*ParteLetta{l.Marcatore, l.Token, l.Etichetta} {
			if p != nil {
				dentro("parte", p.Intervallo)
			}
		}
		if l.Revisione != nil {
			dentro("revisione", l.Revisione.Intervallo)
		}
		if i > 0 && primaDiAttesa(l, letture[i-1]) {
			t.Errorf("letture fuori ordine: %s/%s %v prima di %s/%s %v", letture[i-1].Famiglia, letture[i-1].Forma,
				letture[i-1].Intervallo, l.Famiglia, l.Forma, l.Intervallo)
		}
	}
	return letture, diag
}

// primaDiAttesa: l'ordine del piano per le letture, (inizio, fine, famiglia, forma), scritto qui una seconda
// volta per non fidarsi di quello del prodotto.
func primaDiAttesa(a, b LetturaForma) bool {
	if a.Intervallo.Inizio != b.Intervallo.Inizio {
		return a.Intervallo.Inizio < b.Intervallo.Inizio
	}
	if a.Intervallo.Fine != b.Intervallo.Fine {
		return a.Intervallo.Fine < b.Intervallo.Fine
	}
	if a.Famiglia != b.Famiglia {
		return a.Famiglia < b.Famiglia
	}
	return a.Forma < b.Forma
}

// unaLettura: esattamente una lettura, di quella famiglia e forma.
func unaLettura(t *testing.T, letture []LetturaForma, famiglia, nomeForma string) LetturaForma {
	t.Helper()
	if len(letture) != 1 {
		t.Fatalf("attesa una lettura %s/%s, trovate %d: %s", famiglia, nomeForma, len(letture), riassunto(letture))
	}
	if letture[0].Famiglia != famiglia || letture[0].Forma != nomeForma {
		t.Fatalf("attesa %s/%s, letta %s/%s", famiglia, nomeForma, letture[0].Famiglia, letture[0].Forma)
	}
	return letture[0]
}

func conCodice(d []evidenze.Diagnostica, codice string) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, x := range d {
		if x.Codice == codice {
			out = append(out, x)
		}
	}
	return out
}

func conGravita(d []evidenze.Diagnostica, g evidenze.Gravita) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, x := range d {
		if x.Gravita == g {
			out = append(out, x)
		}
	}
	return out
}

func elenco(d []evidenze.Diagnostica) string {
	var b strings.Builder
	for _, x := range d {
		fmt.Fprintf(&b, "  %s %s %s: %s %v\n", x.Gravita, x.Codice, x.Percorso, x.Messaggio, x.Rif)
	}
	return b.String()
}

func riassunto(letture []LetturaForma) string {
	var parti []string
	for _, l := range letture {
		parti = append(parti, fmt.Sprintf("%s/%s%v %q", l.Famiglia, l.Forma, l.Intervallo, l.Originale))
	}
	return "[" + strings.Join(parti, ", ") + "]"
}

// ---- le prove ----

// TestSinteticheCompilanoDaSoleEInsieme: ogni grammatica del par.4.7.3 compila da sola, e tutte insieme
// compilano nello stesso motore: gli esempi di ognuna passano anche con le altre famiglie sullo stesso
// selettore (verifica sul motore intero, par.4.4.5).
func TestSinteticheCompilanoDaSoleEInsieme(t *testing.T) {
	casi := map[string][]grammatica.FamigliaCodice{
		"acme-prefisso":       {famPrefisso()},
		"acme-marcatore":      {famMarcatore(), famMarcatoreX()},
		"acme-documento":      {famDocumento()},
		"acme-punti":          {famPunti()},
		"acme-etichetta":      {famEtichetta(), famEtichettaPN()},
		"acme-campo-separato": {famCampoSeparato(), famLavagna()},
		"tutte":               tutteLeFamiglie(),
	}
	for _, nome := range []string{"acme-prefisso", "acme-marcatore", "acme-documento", "acme-punti", "acme-etichetta",
		"acme-campo-separato", "tutte"} {
		t.Run(nome, func(t *testing.T) {
			m, _ := compilaBene(t, grammaticaACME(casi[nome]...))
			if m.Snapshot().ClienteID != clienteACME {
				t.Errorf("snapshot del cliente %v, atteso %v", m.Snapshot().ClienteID, clienteACME)
			}
		})
	}
}

// TestSinteticheGliEsempiDellaFormaRiservataNonSonoVerificati (A1a-ESE; R20 b): gli esempi della forma
// riservata danno grammatica.esempio_non_verificato, una nota per esempio, e il motore nasce; tutti gli altri
// esempi non danno note di questo tipo.
func TestSinteticheGliEsempiDellaFormaRiservataNonSonoVerificati(t *testing.T) {
	_, d := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	nv := conCodice(d, CodiceEsempioNonVerificato)
	if len(nv) != 2 {
		t.Fatalf("attesi 2 esempi non verificati (la forma riservata), trovati %d:\n%s", len(nv), elenco(nv))
	}
	for _, x := range nv {
		if x.Gravita != evidenze.GravitaNota || x.Natura != evidenze.NaturaCapacita {
			t.Errorf("esempio non verificato con gravità %s e natura %s, attese nota e capacità", x.Gravita, x.Natura)
		}
		if !strings.Contains(x.Percorso, "acme-campo-separato-lavagna") {
			t.Errorf("esempio non verificato fuori dalla famiglia riservata: %s", x.Percorso)
		}
	}
	if fr := conCodice(d, grammatica.CodiceFormaRiservata); len(fr) == 0 {
		t.Errorf("la forma riservata non è diagnosticata (grammatica.forma_riservata):\n%s", elenco(d))
	}
}
