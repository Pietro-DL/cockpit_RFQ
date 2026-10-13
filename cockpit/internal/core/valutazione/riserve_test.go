// L1 — le radici di riserva nella valutazione (estensione di R76 b, notte 14/10; ancoraggio, passo 3 bis di
// ProponiStrutture): un finito senza strutture dalla regola 1, con uno STEP fra gli allegati che si chiama con il suo
// materiale e ha la radice letta solo come documento. La struttura candidata arriva nel prodotto con la sua origine
// (StrutturaCandidata.Origine), e lo STEP è fra i candidati della fonte (motore A, con il motivo nome_file_step e senza
// radice compatibile), con lo stesso criterio delle strutture (T-B2-02, controllato da calcola). Resta un candidato: la
// fonte è in attesa di conferma, mai confermata (R59 A, T-B0-07).
package valutazione_test

import (
	"reflect"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (vedi scena_test.go). Due spazi di codici ACME: il materiale «acme» (21 più
// sette cifre) e il documento «acme-doc» (101 più sei cifre, il foglio dopo il trattino basso).

// insiemeRiserve: le regole ACME con le due famiglie, sui campi d'identità del file e sugli id dello STEP.
func insiemeRiserve(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	pBase := []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}
	base := func(segmenti ...grammatica.SegmentoBase) grammatica.Base {
		return grammatica.Base{Segmenti: segmenti, Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
			ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII}
	}
	esempio := func(testo string) grammatica.EsempioCodice {
		return grammatica.EsempioCodice{ID: "e-step", Origine: grammatica.OrigineSintetico, Selettore: "radice_step.id", Testo: testo,
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "codice", Base: testo}}}}
	}
	ruoli := []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente}
	materiale := grammatica.FamigliaCodice{ID: "acme", Namespace: "acme", Ruoli: ruoli,
		Base:   base(grammatica.SegmentoBase{Nome: "codice", Pattern: "21[0-9]{7}", Identitario: true}),
		Forme:  []grammatica.FormaCodice{{ID: "codice", Selettori: selCodice, Stato: grammatica.StatoAttiva, Completa: true, Parti: pBase}},
		Esempi: []grammatica.EsempioCodice{esempio("210000001")}}
	documento := grammatica.FamigliaCodice{ID: "acme-doc", Namespace: "acme-doc", Ruoli: ruoli,
		Base: base(grammatica.SegmentoBase{Nome: "documento", Pattern: "101[0-9]{6}", Identitario: true},
			grammatica.SegmentoBase{Nome: "foglio", Pattern: "[0-9]{2}", Separatore: "_", Identitario: true}),
		Forme:  []grammatica.FormaCodice{{ID: "codice", Selettori: selCodice, Stato: grammatica.StatoAttiva, Completa: true, Parti: pBase}},
		Esempi: []grammatica.EsempioCodice{esempio("101000001_01")}}
	return insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", materiale, documento)})
}

// scenaRiserva: il finito manuale 210000001 senza STEP confermato (la vista: mancante), con uno STEP fra gli allegati che
// si chiama con il materiale e il documento della radice; la radice e il figlio sono letti solo come documento.
func scenaRiserva(t *testing.T, nome string) fotorfq.Thread {
	t.Helper()
	th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
	th.Componenti = []fotorfq.Componente{componente(cFinito, "210000001", "finito", "manuale")}
	f := fattiSTEP(t, shaStep, []string{"#1"}, []nodoSTEP{{"#1", "101000001_01"}, {"#2", "101000002_01"}}, [][2]string{{"#1", "#2"}}, "")
	conAllegato(&th, allegato(aStep, 1, nome, "stp", shaStep), &f)
	th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cFinito, Codice: "210000001", Esito: "mancante"}}
	return th
}

// TestLaRadiceDalNomeNellaValutazione: con il nome dello STEP che dice il finito, il prodotto ha la struttura candidata
// con l'origine nome_file_step, e la fonte è in attesa di conferma con il candidato del motore A dal nome (il motivo, la
// radice da scegliere fra le radici, nessuna radice compatibile). Con un nome che non dice il finito, niente: la
// struttura è nessuna e lo STEP non è un candidato della fonte; lo stesso con un nome che dice il finito e un documento
// diverso da quello della radice.
func TestLaRadiceDalNomeNellaValutazione(t *testing.T) {
	r := insiemeRiserve(t)
	rif := ancoraggio.RifComponente(cFinito)

	p := prodottoDi(t, esitoSmistamentoCon(t, scenaRiserva(t, "210000001_101000001_01.stp"), r), rif)
	if p.Struttura != ancoraggio.StatoStrutturaCandidata || len(p.StruttureCandidate) != 1 ||
		p.StruttureCandidate[0].Origine != ancoraggio.OrigineStrutturaNomeFileSTEP || p.StruttureCandidate[0].Compatibilita != motorea.CompatibilitaNonDeterminabile {
		t.Fatalf("la struttura candidata dal nome dello STEP: %s %+v", p.Struttura, p.StruttureCandidate)
	}
	if p.Fonte.Stato != valutazione.FonteInAttesaDiConferma || p.Fonte.Motivo != valutazione.MotivoFonteDocumentoCandidato || len(p.Fonte.Candidati) != 1 {
		t.Fatalf("la fonte in attesa di conferma con il candidato dal nome: %+v", p.Fonte)
	}
	c := p.Fonte.Candidati[0]
	if c.Origine != ancoraggio.CandidatoDaMotoreA || c.Motivo != ancoraggio.MotivoFonteNomeFileSTEP || c.RadiceCompatibile != "" ||
		!reflect.DeepEqual(c.Radici, []string{"#1"}) || c.AllegatoID == nil || *c.AllegatoID != aStep || c.Sha256 != shaStep {
		t.Errorf("il candidato della fonte dal nome dello STEP: %+v", c)
	}

	// senza il nome che dice il finito, o con il nome che dice anche un documento che non è quello della radice
	// (ancoraggio.NomeContraddiceLaRadice, revisione della notte 14/10), niente cambia
	for _, nome := range []string{"assieme.stp", "210000001_101000555_01.stp"} {
		p = prodottoDi(t, esitoSmistamentoCon(t, scenaRiserva(t, nome), r), rif)
		if p.Struttura != ancoraggio.StatoStrutturaNessuna || len(p.StruttureCandidate) != 0 || len(p.Fonte.Candidati) != 0 || p.Fonte.Stato != valutazione.FonteAssente {
			t.Errorf("%s: niente cambia: %s %+v %+v", nome, p.Struttura, p.StruttureCandidate, p.Fonte)
		}
	}
}
