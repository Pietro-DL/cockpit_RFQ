// L1 — i valori e i campi del contratto dell'esito (contratto §2.3; fase 0 di B6, F.2, CP.2, IM.2; F0-03, F0-09, F0-18;
// T-B6-06, F0-05; A1c-L1-32, la parte dei codici di valutazione; R2 per la parte dei tipi di A1c-L1-16): i nomi e i tag
// JSON di ogni tipo nuovo, i valori delle costanti, le versioni fissate, i record piatti fatti solo di tipi delle foglie
// (perché confronto ne ha i gemelli, e la prova del passaggio, A1c-L1-31, li confronta per struttura), nessun tipo che
// porti l'atteso.
//
// Qui non c'è nessun cliente: si leggono solo i tipi del pacchetto.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/valutazione"
)

// TestLeVersioniFissate (IM.2): le costanti di versione dell'esito; se una cambia, la prova si riscrive con il commit che
// la dichiara, e cambia Esito.Impronta.
func TestLeVersioniFissate(t *testing.T) {
	if valutazione.VersioneValutazione != "valutazione-1" || valutazione.VersioneImprontaProdotto != 1 {
		t.Errorf("versioni %q %d", valutazione.VersioneValutazione, valutazione.VersioneImprontaProdotto)
	}
}

// TestIValoriDellEsito: le costanti nuove con i valori del contratto e della fase 0, e i codici di V1 (A1c-L1-32).
func TestIValoriDellEsito(t *testing.T) {
	for _, c := range [][2]string{
		{string(valutazione.MotivoThreadSenzaGrammatica), "senza_grammatica_a"}, {string(valutazione.MotivoThreadGrammaticaScartata), "grammatica_scartata"},
		{string(valutazione.MotivoThreadRagioneSocialeDiscorde), "ragione_sociale_discorde"}, {string(valutazione.MotivoThreadErroreValutazione), "errore_valutazione"},
		{valutazione.MotivoFileContenitore, "contenitore"}, {valutazione.MotivoFileNaturaNonFile, "natura_non_file"},
		{valutazione.MotivoFileDocumentoNonLeggibile, "documento_non_leggibile"}, {valutazione.MotivoFileThreadNonValutato, "thread_non_valutato"},
		{valutazione.MotivoLetturaCodiceVuoto, "codice_vuoto"}, {valutazione.MotivoLetturaSenzaGrammatica, "senza_grammatica"},
		{valutazione.MotivoLetturaNessunaLettura, "nessuna_lettura"}, {valutazione.MotivoLetturaBasiDiverse, "basi_diverse"},
		{valutazione.RevisioniUguali, "uguali"}, {valutazione.RevisioniDiverse, "diverse"}, {valutazione.RevisioniNonConfrontabili, "non_confrontabili"},
		{valutazione.MotivoRevisioniEquivalente, "equivalente"}, {valutazione.MotivoRevisioniNuovoNonValutato, "nuovo_non_valutato"},
		{valutazione.MotivoRevisioniVecchiaNonLetta, "revisione_vecchia_non_letta"}, {valutazione.MotivoRevisioniNuovaNonLetta, "revisione_nuova_non_letta"},
		{valutazione.MotivoRevisioniSoloInColonna, "revisione_solo_in_colonna"},
		{valutazione.MotivoRevisioniNuoveDiscordi, "revisioni_nuove_discordi"},
		{string(valutazione.SmistamentoDaVerificare), "da_verificare"}, {string(valutazione.SmistamentoVerificato), "verificato"},
		{string(valutazione.SmistamentoConflitto), "conflitto"}, {string(valutazione.SmistamentoInRevisione), "in_revisione"},
		{string(valutazione.MotivoSmistamentoFileDaSmistare), "file_da_smistare"}, {string(valutazione.MotivoSmistamentoNessunCandidato), "nessun_candidato"},
		{string(valutazione.MotivoSmistamentoAssociazioneAmbigua), "associazione_ambigua"},
		{string(valutazione.MotivoSmistamentoAssociazioneDiscordante), "associazione_discordante"},
		{string(valutazione.MotivoSmistamentoCollocazioneNonDeterminabile), "collocazione_non_determinabile"},
		{string(valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato), "fuori_richiesta_non_confermato"},
		{string(valutazione.MotivoSmistamentoAssociazioneNonConfermata), "associazione_non_confermata"},
		{string(valutazione.MotivoSmistamentoAssociazioneInConflitto), "associazione_in_conflitto"},
		{string(valutazione.MotivoSmistamentoNuovoFile), "nuovo_file_su_componente_deciso"},
		{string(valutazione.MotivoSmistamentoRevisioneTecnicaAperta), "revisione_tecnica_aperta"},
		{string(valutazione.MotivoSmistamentoIdentitaDocumento), "identita_documento_in_conflitto"},
		{valutazione.PerimetroDentro, "dentro"}, {valutazione.PerimetroInline, "inline"}, {valutazione.PerimetroElementoOutlook, "elemento_outlook"},
		{valutazione.PerimetroContenitoreEstratto, "contenitore_estratto"}, {valutazione.PerimetroCollegamento, "collegamento"},
		{valutazione.PerimetroMessaggioInUscita, "messaggio_in_uscita"}, {valutazione.PerimetroAltraControparte, "altra_controparte"},
		{string(valutazione.ProdottoNonPronto), "non_pronto"}, {string(valutazione.ProdottoProntoFattibilita), "pronto_fattibilita"},
		{string(valutazione.ProdottoDaRiesaminare), "da_riesaminare"},
		{string(valutazione.MotivoProdottoTargetNonConfermato), "target_non_confermato"},
		{string(valutazione.MotivoProdottoFonte), "fonte_strutturale_step_mancante_o_non_confermata"},
		{string(valutazione.MotivoProdottoFonteSuperata), "fonte_superata"}, {string(valutazione.MotivoProdottoNomenclatura), "nomenclatura_non_verificata"},
		{string(valutazione.MotivoProdottoGerarchia), "gerarchia_non_verificata"}, {string(valutazione.MotivoProdottoSmistamento), "smistamento_non_verificato"},
		{string(valutazione.MotivoProdottoDocumentazioneIncompleta), "documentazione_incompleta"},
		{string(valutazione.MotivoProdottoDocumentazioneNonCalc), "documentazione_non_calcolabile"},
		{string(valutazione.MotivoProdottoConflitto), "conflitto"}, {string(valutazione.MotivoProdottoRevisioneTecnicaAperta), "revisione_tecnica_aperta"},
		{valutazione.NonCongelabileThreadNonValutato, "thread_non_valutato"}, {valutazione.NonCongelabileNessunTarget, "nessun_target"},
		{valutazione.NonCongelabileBOMVersioneNonVerificati, "bom_versione_con_prodotti_non_verificati"},
		{valutazione.NonCongelabileProdottiNonPronti, "prodotti_non_pronti"}, {valutazione.NonCongelatoGestoNonRegistrato, "gesto_non_registrato"},
		{valutazione.CongelamentoNonPiuCongelabile, "non_piu_congelabile"}, {valutazione.PrefissoCongelamentoImprontaCambiata, "impronta_cambiata:"},
		{valutazione.PrefissoCongelamentoTargetNuovo, "target_nuovo:"},
		{valutazione.MotivoConflittoComponenteDiverso, "componente_diverso"}, {valutazione.MotivoConflittoCodiceConfermato, "codice_confermato"},
		{valutazione.CodiceErroreValutazione, "valutazione.errore_valutazione"},
		{valutazione.CodiceCodiceRegistratoNonLeggibile, "confronto.codice_registrato_non_leggibile"},
		{valutazione.CodiceMessaggioFuoriRFQSenzaCaso, "valutazione.messaggio_fuori_rfq_senza_caso"},
		// le correzioni dopo le risposte del 07/10 (R106 B, precisata)
		{valutazione.CodiceContestoDiscorde, "valutazione.contesto_discorde"},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, atteso %q", c[0], c[1])
		}
	}
}

// TestICampiDellEsito (contratto §2.3; fase 0, F.2 e CP.2): i campi di ogni tipo nuovo, nell'ordine, con il tag JSON. I
// record piatti sono uguali, campo per campo, ai DTO di confronto: un campo cambiato qui si annuncia prima del commit.
func TestICampiDellEsito(t *testing.T) {
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{valutazione.Esito{}, "VersioneValutazione:versione_valutazione VersioneImprontaProdotto:versione_impronta_prodotto VersioneFormati2D:versione_formati_2d " +
			"VersioneComposizione:versione_composizione ImprontaFotografia:impronta_fotografia ImprontaIndice:impronta_indice VersioneLimiti:versione_limiti " +
			"Thread:thread FuoriRFQ:fuori_rfq Diagnostiche:diagnostiche Impronta:impronta"},
		{valutazione.EsitoThread{}, "ThreadID:thread_id ClienteID:cliente_id Valutato:valutato Motivo:motivo HashSnapshot:hash_snapshot Richiesta:richiesta " +
			"File:file Prodotti:prodotti Ancoraggi:ancoraggi VecchiProdotti:vecchi_prodotti Evidenze:evidenze Confrontabili:confrontabili " +
			"ProdottiConfrontabili:prodotti_confrontabili ProdottiValutati:prodotti_valutati Associazioni:associazioni DaSmistare:da_smistare " +
			"Conflitti:conflitti Fascicolo:fascicolo Diagnostiche:diagnostiche"},
		{valutazione.EsitoFuoriRFQ{}, "Caso:caso MessaggioID:messaggio_id ClienteID:cliente_id Valutato:valutato Motivo:motivo HashSnapshot:hash_snapshot " +
			"Messaggio:messaggio File:file Prodotti:prodotti Diagnostiche:diagnostiche"},
		{valutazione.EvidenzaFile{}, "AllegatoID:allegato_id UnitaID:unita_id LetturaID:lettura_id Testo:testo Intervallo:intervallo Selettore:selettore Posizione:posizione"},
		{valutazione.FileInterpretato{}, "AllegatoID:allegato_id Documento:documento Interpretazione:interpretazione Disponibilita:disponibilita Disegno:disegno Motivo:motivo"},
		{valutazione.MessaggioInterpretato{}, "MessaggioID:messaggio_id Documento:documento Interpretazione:interpretazione"},
		{valutazione.LetturaRegistrata{}, "Originale:originale Base:base Marcatore:marcatore Revisione:revisione Leggibile:leggibile Motivo:motivo"},
		{valutazione.FileConfrontabile{}, "AllegatoID:allegato_id Vecchio:vecchio Nuovo:nuovo"},
		{valutazione.VecchioPiatto{}, "Stato:stato Fonte:fonte Codice:codice Rev:rev Base:base Marcatore:marcatore Revisione:revisione Leggibile:leggibile " +
			"MotivoLettura:motivo_lettura CodiceLetto:codice_letto CodiceLettoBase:codice_letto_base CodiceLettoMarcatore:codice_letto_marcatore " +
			"Componente:componente Documento:documento " +
			"ComponenteProposta:componente_proposta " +
			"SostituitoDa:sostituito_da DecisoIl:deciso_il Destinazione:destinazione"},
		{valutazione.NuovoPiatto{}, "Valutato:valutato Motivo:motivo Basi:basi Candidati:candidati Collocazione:collocazione Associazione:associazione " +
			"Disponibilita:disponibilita Revisione:revisione Revisioni:revisioni MotivoRevisioni:motivo_revisioni"},
		{valutazione.CandidatoPiatto{}, "Target:target Livello:livello Base:base Autorita:autorita Radici:radici"},
		{valutazione.ProdottoConfrontabile{}, "CodiceRichiesto:codice_richiesto Base:base Fase:fase Quantita:quantita QuantitaDaCella:quantita_da_cella"},
		{valutazione.ProdottoValutato{}, "Rif:rif Autorita:autorita ComponenteID:componente_id CodiceRichiesto:codice_richiesto Base:base Identita:identita " +
			"Fonte:fonte Struttura:struttura BOM:bom Nodi:nodi Smistamento:smistamento Documenti:documenti Verificato:prodotto_verificato Stato:stato " +
			"Motivi:motivi Impronta:impronta"},
		{valutazione.VerificaSmistamento{}, "Stato:stato FileNonTerminali:file_non_terminali Motivi:motivi Conflitti:conflitti Calcolata:calcolata Percorsi:percorsi"},
		{valutazione.AssociazioneFile{}, "AllegatoID:allegato_id Associazione:associazione Collocazione:collocazione Proposta:proposta DestinazioneF8:destinazione_f8 " +
			"Manuale:manuale Confermata:confermata DocumentoID:documento_id Origine:origine DestinazioneCoincide:destinazione_coincide Conflitto:conflitto " +
			"Esclusa:esclusa Terminale:terminale Pertinente:pertinente Perimetro:perimetro PertinenzaContesto:pertinenza_contesto Contesto:contesto"},
		// R106 B e R107, precisate il 07/10: il contesto del messaggio con la provenienza.
		{valutazione.ContestoMessaggio{}, "MessaggioID:messaggio_id Letture:letture Prodotti:prodotti"},
		{valutazione.FileDaSmistare{}, "AllegatoID:allegato_id NomeFile:nome_file Motivo:motivo Prodotti:prodotti Proposta:proposta Orfano:orfano ProdottiContesto:prodotti_contesto"},
		{valutazione.NodoBOM{}, "Nodo:nodo Descrizione:descrizione Parentela:parentela Disegni:disegni Associazione:associazione Motivi:motivi " +
			"Classificazione:classificazione AncheIn:anche_in"},
		{valutazione.Parentela{}, "Padre:padre Quantita:quantita Decisa:decisa"},
		{valutazione.StatoFascicolo{}, "NumeroTarget:numero_target NumeroVerificati:numero_verificati Pronti:pronti Bloccati:bloccati Congelabile:congelabile " +
			"MotivoNonCongelabile:motivo_non_congelabile Congelato:congelato CongelatoDa:congelato_da CongelatoIl:congelato_il Calcolato:calcolato " +
			"MotivoNonCongelato:motivo_non_congelato ConflittiCongelamento:conflitti_congelamento Legacy:legacy Orfani:orfani Avvisi:avvisi FaseThread:fase_thread"},
		{valutazione.ProdottoBloccato{}, "Rif:rif Motivi:motivi"},
		{valutazione.CongelamentoLegacy{}, "VersioneCorrente:versione_corrente StatoCorrente:stato_corrente UltimaCongelata:ultima_congelata " +
			"CongelataDa:congelata_da CongelataIl:congelata_il Riaperta:riaperta"},
		// B6, V3: gli ingressi delle regole del fascicolo, dello stato e dell'impronta (contratto §2.5; fase 0, F.4 e IM.3).
		{valutazione.GestoCongelamento{}, "Da:da Il:il Impronte:impronte"},
		{valutazione.IngressoFascicolo{}, "Valutato:valutato Calcolato:calcolato Prodotti:prodotti Legacy:legacy FaseThread:fase_thread DaSmistare:da_smistare"},
		{valutazione.CondizioniNuove{}, "Nomenclatura:nomenclatura Gerarchia:gerarchia"},
		{valutazione.DatiDecisiProdotto{}, "Versione:versione Rif:rif Codice:codice Fonte:fonte Componenti:componenti Relazioni:relazioni Documenti:documenti"},
		{valutazione.ImprontaFonte{}, "DocumentoID:documento_id Sha256:sha256 Radice:radice Superato:superato"},
		{valutazione.ImprontaComponente{}, "ID:id Codice:codice Rev:rev"},
		{valutazione.ImprontaRelazione{}, "Padre:padre Figlio:figlio Qta:qta"},
		{valutazione.ImprontaDocumento{}, "ComponenteID:componente_id ID:id Tipo:tipo Sha256:sha256 Rev:rev Confermata:confermata CodiceConfermato:codice_confermato"},
	} {
		if got := campiJSON(c.tipo); got != c.campi {
			t.Errorf("%T:\ncampi %q\nattesi %q", c.tipo, got, c.campi)
		}
	}
}

// TestIRecordPiattiSoloTipiDelleFoglie (fase 0, CP.2; F0-09): i record piatti sono fatti solo di string, bool, int,
// uuid.UUID, time.Time, dei loro puntatori, degli slice e delle struct gemelle di questo pacchetto; nessuna mappa, nessun
// array, nessuna interfaccia, nessun tipo con nome del motore, nemmeno per un enumerato. Ogni campo è esportato e ha un
// tag JSON.
func TestIRecordPiattiSoloTipiDelleFoglie(t *testing.T) {
	foglie := map[reflect.Type]bool{reflect.TypeFor[string](): true, reflect.TypeFor[bool](): true, reflect.TypeFor[int](): true,
		reflect.TypeFor[uuid.UUID](): true, reflect.TypeFor[time.Time](): true}
	gemelli := map[string]bool{"FileConfrontabile": true, "VecchioPiatto": true, "NuovoPiatto": true, "CandidatoPiatto": true, "ProdottoConfrontabile": true}
	var visita func(tp reflect.Type, dove string)
	visita = func(tp reflect.Type, dove string) {
		switch {
		case foglie[tp]:
			return
		case tp.Kind() == reflect.Pointer || tp.Kind() == reflect.Slice:
			visita(tp.Elem(), dove+"[]")
			return
		case tp.Kind() == reflect.Struct && tp.PkgPath() == reflect.TypeFor[valutazione.FileConfrontabile]().PkgPath() && gemelli[tp.Name()]:
			for i := 0; i < tp.NumField(); i++ {
				f := tp.Field(i)
				if !f.IsExported() || f.Tag.Get("json") == "" {
					t.Errorf("%s.%s: campo non esportato o senza tag JSON", dove, f.Name)
				}
				visita(f.Type, dove+"."+f.Name)
			}
			return
		}
		t.Errorf("%s: il tipo %s non è una foglia né un gemello (CP.2)", dove, tp)
	}
	visita(reflect.TypeFor[valutazione.FileConfrontabile](), "FileConfrontabile")
	visita(reflect.TypeFor[valutazione.ProdottoConfrontabile](), "ProdottoConfrontabile")
	// Il numero dei campi di CP.2, con l'emendamento F0-19 (CodiceLettoBase) e il gemello di R114, precisata dall'utente il
	// 07/10 (CodiceLettoMarcatore): 18 campi nel vecchio.
	for _, c := range []struct {
		tp     reflect.Type
		numero int
	}{{reflect.TypeFor[valutazione.FileConfrontabile](), 3}, {reflect.TypeFor[valutazione.VecchioPiatto](), 18},
		{reflect.TypeFor[valutazione.NuovoPiatto](), 10}, {reflect.TypeFor[valutazione.CandidatoPiatto](), 5},
		{reflect.TypeFor[valutazione.ProdottoConfrontabile](), 5}} {
		if c.tp.NumField() != c.numero {
			t.Errorf("%s: %d campi, attesi %d (CP.2, F0-19, R114)", c.tp.Name(), c.tp.NumField(), c.numero)
		}
	}
}

// TestLAttesoNonCompareNeiTipi (A1c-L1-16, R2): nessun tipo raggiungibile dall'esito di Calcola o dagli ingressi dei casi
// viene da confronto o da una libreria YAML, e nessun campo si chiama atteso o attesi.
func TestLAttesoNonCompareNeiTipi(t *testing.T) {
	visti := map[reflect.Type]bool{}
	var visita func(tp reflect.Type, dove string)
	visita = func(tp reflect.Type, dove string) {
		if visti[tp] {
			return
		}
		visti[tp] = true
		if p := tp.PkgPath(); strings.Contains(p, "/core/confronto") || strings.Contains(p, "yaml") {
			t.Errorf("%s: il tipo %s viene da %s", dove, tp, p)
		}
		switch tp.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			visita(tp.Elem(), dove+"[]")
		case reflect.Map:
			visita(tp.Key(), dove+"{chiave}")
			visita(tp.Elem(), dove+"{}")
		case reflect.Struct:
			for i := 0; i < tp.NumField(); i++ {
				f := tp.Field(i)
				nome, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				if l := strings.ToLower(f.Name); l == "atteso" || l == "attesi" || nome == "atteso" || nome == "attesi" {
					t.Errorf("%s.%s: un campo dell'atteso", dove, f.Name)
				}
				visita(f.Type, dove+"."+f.Name)
			}
		}
	}
	visita(reflect.TypeFor[valutazione.Esito](), "Esito")
	visita(reflect.TypeFor[valutazione.Ingressi](), "Ingressi")
	if len(visti) < 50 {
		t.Errorf("tipi visitati %d: la prova non scende nell'esito", len(visti))
	}
}
