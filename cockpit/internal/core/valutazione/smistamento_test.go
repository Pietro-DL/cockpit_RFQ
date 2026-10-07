// L1 — la regola dello smistamento di un prodotto, sugli ingressi astratti (B6, V2; R81, T-B0-25; E1: T-E1-11, T-E1-13;
// R95 A; R64 A; contratto §1.5, il blocco SmistamentoVerificato; T-B6-07, T-B6-08, F0-17): le cinque condizioni, ognuna
// con il suo motivo; il documento richiesto che manca, che conta solo per la completezza (T-B6-08); lo stato in_revisione
// con un percorso di revisione sintetico (PO-24, origine fattibilita), che vince sul conflitto e non spegne le condizioni;
// PO-25 nella parte dello smistamento (il conflitto nuovo_file, il percorso inbox_nuovo_cad solo per i prodotti coinvolti);
// lo smistamento non calcolato (il target senza componente, T-12), sempre da_verificare; il determinismo. Più R106 B e
// R108 A sulle loro funzioni (le risposte dell'utente del 07/10: R106 B precisata, R108 A confermata con il requisito
// operativo per lo spazio di verifica), e l'attribuzione dei conflitti identita_documento (T-E1R-08).
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

const (
	rifP1  = "componente:00000000-0000-4000-8000-000000000b01"
	rifP2s = "componente:00000000-0000-4000-8000-000000000b02"
	rifK1  = "componente:00000000-0000-4000-8000-000000000b11"
)

var (
	fA = uid(0xb21)
	fB = uid(0xb22)
	fC = uid(0xb23)
)

// fileS: un file pertinente della regola.
func fileS(id uuid.UUID, terminale bool, a ancoraggio.Associazione, c ancoraggio.Collocazione, manuale bool) valutazione.FileDelloSmistamento {
	return valutazione.FileDelloSmistamento{AllegatoID: id, Terminale: terminale, Associazione: a, Collocazione: c, Manuale: manuale}
}

// ingressoVerificato: P1 con tre file terminali, le voci certe presenti o che mancano del tutto, nessun conflitto.
func ingressoVerificato() valutazione.IngressoSmistamento {
	return valutazione.IngressoSmistamento{Prodotto: rifP1, Elementi: []string{rifK1}, Calcolata: true,
		File: []valutazione.FileDelloSmistamento{
			fileS(fC, true, ancoraggio.AssociazioneDiscordante, ancoraggio.CollocazioneNonDeterminabile, false),
			fileS(fA, true, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneRadice, false),
			fileS(fB, true, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneFuoriRichiesta, false)},
		Voci: []valutazione.VoceFabbisogno{
			{TipoDocumento: "disegno_2d", Esito: valutazione.EsitoPresente},
			{TipoDocumento: "cad_3d", Esito: valutazione.EsitoManca, Motivo: valutazione.MotivoFabbisognoNessunDocumento},
			{TipoDocumento: "disegno_2d", Esito: valutazione.EsitoDaVerificare, Motivo: valutazione.MotivoFabbisognoContenutoNonAnalizzato}}}
}

// percorso: un percorso di revisione tecnica sintetico (LD-18: in A1c nessun adattatore), aperto o chiuso.
func percorso(id string, origine valutazione.OriginePercorso, aperto bool, elementi ...string) valutazione.PercorsoRevisione {
	p := valutazione.PercorsoRevisione{ID: id, Origine: origine, Motivo: "revisione di prova", Elementi: elementi,
		CondizioneChiusura: "nuova revisione confermata", Aperto: aperto, ApertoDa: operatore, ApertoIl: dataACME}
	if !aperto {
		da, il := operatore, dataACME.Add(24*time.Hour)
		p.ChiusoDa, p.ChiusoIl = &da, &il
	}
	return p
}

// conflittoS: un conflitto sintetico dell'asse e del prodotto dati.
func conflittoS(tipo valutazione.TipoConflitto, asse valutazione.AsseConflitto, rif, prodotto string) valutazione.Conflitto {
	return valutazione.Conflitto{Tipo: tipo, Asse: asse, Rif: rif, Prodotto: prodotto, Decisione: rifK1, OrigineDecisione: ancoraggio.OrigineConfermato}
}

func statoMotivi(s valutazione.VerificaSmistamento) string {
	return string(s.Stato) + "/" + motiviDi(s)
}

// TestSmistamentoSullaRegola (R81, T-B0-25, T-B6-08; R64 A; R95 A): le cinque condizioni del blocco SmistamentoVerificato
// sulla regola, una per volta, con il loro motivo; i file terminali non bloccano qualunque sia la loro proposta; una voce
// certa che manca del tutto, o con il contenuto ancora da verificare, non tocca lo smistamento (T-B6-08: lo dice la
// completezza); i conflitti di un altro asse o di un altro prodotto non contano.
func TestSmistamentoSullaRegola(t *testing.T) {
	t.Run("tutto terminale e nessun documento presente non confermato: verificato", func(t *testing.T) {
		s := valutazione.Smistamento(ingressoVerificato(), nil)
		if s.Stato != valutazione.SmistamentoVerificato || len(s.Motivi)+len(s.FileNonTerminali)+len(s.Conflitti)+len(s.Percorsi) != 0 || !s.Calcolata {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("la prima condizione: un documento presente non confermato", func(t *testing.T) {
		in := ingressoVerificato()
		in.Voci = append(in.Voci, valutazione.VoceFabbisogno{TipoDocumento: "disegno_2d", Esito: valutazione.EsitoDaVerificare,
			Motivo: valutazione.MotivoFabbisognoAssociazioneNonConfermata})
		if s := valutazione.Smistamento(in, nil); statoMotivi(s) != "da_verificare/associazione_non_confermata" || len(s.FileNonTerminali) != 0 {
			t.Errorf("smistamento %+v", s)
		}
	})
	casi := []struct {
		nome   string
		file   valutazione.FileDelloSmistamento
		motivi string
	}{
		{"la seconda condizione: un file ambiguo", fileS(fA, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneFiglio, false),
			"file_da_smistare associazione_ambigua"},
		{"la seconda condizione: un file discordante (R64 A)", fileS(fA, false, ancoraggio.AssociazioneDiscordante, ancoraggio.CollocazioneRadice, false),
			"file_da_smistare associazione_discordante"},
		{"la seconda condizione: la collocazione non determinabile", fileS(fA, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneNonDeterminabile, false),
			"file_da_smistare associazione_ambigua collocazione_non_determinabile"},
		{"la terza condizione: la pre-associazione non confermata", fileS(fA, false, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneFiglio, false),
			"file_da_smistare associazione_non_confermata"},
		{"la terza condizione: un fuori_richiesta proposto", fileS(fA, false, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneFuoriRichiesta, false),
			"file_da_smistare nessun_candidato fuori_richiesta_non_confermato"},
		{"la terza condizione: un «assegna» non confermato", fileS(fA, false, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, true),
			"file_da_smistare nessun_candidato collocazione_non_determinabile associazione_non_confermata"},
		{"la terza condizione: un file che l'adattatore non legge", fileS(fA, false, ancoraggio.AssociazioneNonValutata, "", false),
			"file_da_smistare nessun_candidato"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			in := ingressoVerificato()
			in.File = append(in.File, c.file)
			s := valutazione.Smistamento(in, nil)
			if s.Stato != valutazione.SmistamentoDaVerificare || motiviDi(s) != c.motivi || !reflect.DeepEqual(s.FileNonTerminali, []uuid.UUID{fA}) {
				t.Errorf("smistamento %+v, motivi attesi %q", s, c.motivi)
			}
		})
	}
	t.Run("la quarta condizione: un conflitto aperto sullo smistamento del prodotto (R95 A)", func(t *testing.T) {
		for _, tc := range []struct {
			tipo   valutazione.TipoConflitto
			motivo string
		}{{valutazione.ConflittoAssociazione, "associazione_in_conflitto"}, {valutazione.ConflittoNuovoFile, "nuovo_file_su_componente_deciso"},
			{valutazione.ConflittoIdentitaDocumento, "identita_documento_in_conflitto"}} {
			in := ingressoVerificato()
			in.Conflitti = []valutazione.Conflitto{conflittoS(tc.tipo, valutazione.AsseSmistamento, "documento:x", rifP1),
				conflittoS(tc.tipo, valutazione.AsseSmistamento, "documento:x", rifP1)}
			s := valutazione.Smistamento(in, nil)
			if statoMotivi(s) != "conflitto/"+tc.motivo || !reflect.DeepEqual(s.Conflitti, []string{"documento:x"}) {
				t.Errorf("%s: smistamento %+v", tc.tipo, s)
			}
		}
	})
	t.Run("i conflitti di un altro asse o di un altro prodotto non contano", func(t *testing.T) {
		in := ingressoVerificato()
		in.Conflitti = []valutazione.Conflitto{conflittoS(valutazione.ConflittoCodice, valutazione.AsseNomenclatura, rifK1, rifP1),
			conflittoS(valutazione.ConflittoAssociazione, valutazione.AsseSmistamento, "documento:y", rifP2s)}
		if s := valutazione.Smistamento(in, nil); s.Stato != valutazione.SmistamentoVerificato {
			t.Errorf("smistamento %+v", s)
		}
	})
}

// TestPO24InRevisioneSullaRegola (PO-24, la parte dello smistamento; T-E1-13; LD-18): con un percorso di revisione
// sintetico aperto (origine fattibilita) che nomina il prodotto, lo smistamento è in_revisione, con il motivo e l'ID del
// percorso; vince sul conflitto, e le condizioni restano calcolate e mostrate (servono a chiudere il percorso). Un
// percorso che nomina un elemento del perimetro del prodotto lo coinvolge; uno chiuso, o su un altro prodotto, no.
func TestPO24InRevisioneSullaRegola(t *testing.T) {
	t.Run("un percorso aperto sul prodotto", func(t *testing.T) {
		s := valutazione.Smistamento(ingressoVerificato(), []valutazione.PercorsoRevisione{percorso("pr-1", valutazione.OriginePercorsoFattibilita, true, rifP1)})
		if statoMotivi(s) != "in_revisione/revisione_tecnica_aperta" || !reflect.DeepEqual(s.Percorsi, []string{"pr-1"}) {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("vince sul conflitto e non spegne le condizioni", func(t *testing.T) {
		in := ingressoVerificato()
		in.File = append(in.File, fileS(fA, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneFiglio, false))
		in.Conflitti = []valutazione.Conflitto{conflittoS(valutazione.ConflittoAssociazione, valutazione.AsseSmistamento, "documento:x", rifP1)}
		s := valutazione.Smistamento(in, []valutazione.PercorsoRevisione{percorso("pr-2", valutazione.OriginePercorsoOperatore, true, rifK1),
			percorso("pr-1", valutazione.OriginePercorsoFattibilita, true, rifP1)})
		if statoMotivi(s) != "in_revisione/file_da_smistare associazione_ambigua associazione_in_conflitto revisione_tecnica_aperta" ||
			!reflect.DeepEqual(s.Percorsi, []string{"pr-1", "pr-2"}) || len(s.Conflitti) != 1 || len(s.FileNonTerminali) != 1 {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("un percorso chiuso, o su un altro prodotto, non coinvolge", func(t *testing.T) {
		s := valutazione.Smistamento(ingressoVerificato(), []valutazione.PercorsoRevisione{percorso("pr-1", valutazione.OriginePercorsoFattibilita, false, rifP1),
			percorso("pr-3", valutazione.OriginePercorsoFattibilita, true, rifP2s)})
		if s.Stato != valutazione.SmistamentoVerificato || len(s.Percorsi) != 0 {
			t.Errorf("smistamento %+v", s)
		}
	})
}

// TestPO25SmistamentoSullaRegola (PO-25, la parte dello smistamento sulla regola; T-B0-29; T-E1-13): lo STEP nuovo con la
// radice di P, pertinente solo a P, porta il conflitto nuovo_file: lo smistamento di P va in conflitto, quello di P2 resta
// verificato. Con un percorso sintetico di origine inbox_nuovo_cad sugli elementi dichiarati (il prodotto P), in_revisione
// solo per i prodotti coinvolti.
func TestPO25SmistamentoSullaRegola(t *testing.T) {
	p := ingressoVerificato()
	p.File = append(p.File, fileS(fA, false, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneRadice, false))
	p.Conflitti = []valutazione.Conflitto{conflittoS(valutazione.ConflittoNuovoFile, valutazione.AsseSmistamento, rifK1, rifP1)}
	p2 := valutazione.IngressoSmistamento{Prodotto: rifP2s, Calcolata: true}
	if s := valutazione.Smistamento(p, nil); statoMotivi(s) != "conflitto/file_da_smistare associazione_non_confermata nuovo_file_su_componente_deciso" {
		t.Errorf("P: %+v", s)
	}
	if s := valutazione.Smistamento(p2, nil); s.Stato != valutazione.SmistamentoVerificato {
		t.Errorf("P2: %+v", s)
	}
	pr := []valutazione.PercorsoRevisione{percorso("inbox-1", valutazione.OriginePercorsoInboxNuovoCAD, true, rifP1)}
	if s := valutazione.Smistamento(p, pr); s.Stato != valutazione.SmistamentoInRevisione {
		t.Errorf("P con il percorso: %+v", s)
	}
	if s := valutazione.Smistamento(p2, pr); s.Stato != valutazione.SmistamentoVerificato || len(s.Percorsi) != 0 {
		t.Errorf("P2 con il percorso di P: %+v", s)
	}
}

// TestSmistamentoNonCalcolato (T-B6-07, F0-17; T-12): senza calcolo lo smistamento è sempre da_verificare, anche con tutti
// i file terminali (nessun motivo: lo dice Calcolata), con file non terminali (file_da_smistare), con un conflitto o un
// percorso aperto: le condizioni restano mostrate, lo stato no.
func TestSmistamentoNonCalcolato(t *testing.T) {
	in := ingressoVerificato()
	in.Calcolata = false
	if s := valutazione.Smistamento(in, nil); s.Stato != valutazione.SmistamentoDaVerificare || len(s.Motivi) != 0 || s.Calcolata {
		t.Errorf("tutto terminale: %+v", s)
	}
	in.File = append(in.File, fileS(fA, false, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, false))
	in.Conflitti = []valutazione.Conflitto{conflittoS(valutazione.ConflittoAssociazione, valutazione.AsseSmistamento, "documento:x", rifP1)}
	s := valutazione.Smistamento(in, []valutazione.PercorsoRevisione{percorso("pr-1", valutazione.OriginePercorsoFattibilita, true, rifP1)})
	if s.Stato != valutazione.SmistamentoDaVerificare || motiviDi(s) != "file_da_smistare nessun_candidato collocazione_non_determinabile associazione_in_conflitto revisione_tecnica_aperta" ||
		len(s.Percorsi) != 1 || len(s.Conflitti) != 1 {
		t.Errorf("con un file, un conflitto e un percorso: %+v", s)
	}
}

// TestSmistamentoDeterministico (par.3.4.5): gli ingressi permutati danno lo stesso smistamento, un file ripetuto conta una
// volta, e gli ingressi di chi chiama non cambiano.
func TestSmistamentoDeterministico(t *testing.T) {
	in := ingressoVerificato()
	in.File = append(in.File, fileS(fB, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneFiglio, false),
		fileS(fA, false, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneFiglio, false),
		fileS(fA, false, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneFiglio, false))
	in.Conflitti = []valutazione.Conflitto{conflittoS(valutazione.ConflittoNuovoFile, valutazione.AsseSmistamento, "z", rifP1),
		conflittoS(valutazione.ConflittoAssociazione, valutazione.AsseSmistamento, "a", rifP1)}
	pr := []valutazione.PercorsoRevisione{percorso("b", valutazione.OriginePercorsoOperatore, true, rifP1), percorso("a", valutazione.OriginePercorsoFattibilita, true, rifK1)}
	prima := canonicoDi(t, in)
	s1 := valutazione.Smistamento(in, pr)
	rov := in
	rov.File, rov.Conflitti, rov.Voci, rov.Elementi = rovescia(in.File), rovescia(in.Conflitti), rovescia(in.Voci), rovescia(in.Elementi)
	s2 := valutazione.Smistamento(rov, rovescia(pr))
	if canonicoDi(t, s1) != canonicoDi(t, s2) || canonicoDi(t, in) != prima {
		t.Errorf("non deterministico o ingresso cambiato:\n%s\n%s", canonicoDi(t, s1), canonicoDi(t, s2))
	}
	if !reflect.DeepEqual(s1.FileNonTerminali, []uuid.UUID{fA, fB}) || !reflect.DeepEqual(s1.Conflitti, []string{"a", "z"}) || !reflect.DeepEqual(s1.Percorsi, []string{"a", "b"}) {
		t.Errorf("ordine: %+v", s1)
	}
}

// TestR106LaFunzione (R106 B, precisata dall'utente il 07/10; T-E1R-11): il contesto vale come ripiego per ogni file
// senza evidenze di pertinenza, anche con un'identità letta; mai per un file che ha già un'evidenza.
func TestR106LaFunzione(t *testing.T) {
	for _, c := range []struct {
		evidenze, identita, vale bool
	}{{false, false, true}, {false, true, true}, {true, false, false}, {true, true, false}} {
		if got := valutazione.ContestoApplicabilePerProva(c.evidenze, c.identita); got != c.vale {
			t.Errorf("evidenze %v, identità %v: il contesto vale %v, atteso %v", c.evidenze, c.identita, got, c.vale)
		}
	}
}

// TestR108LaFunzione (R108 A, confermata dall'utente il 07/10 con il requisito operativo per lo spazio di verifica;
// contratto §1.5, §2.3, E1): «da smistare» ha i file che contano, non sono terminali e hanno uno dei cinque motivi, più
// gli orfani, sempre con uno dei cinque. La pre-associazione (candidato_unico) non entra; un file fuori perimetro o
// terminale mai. Il motivo della collocazione non determinabile c'è con i candidati o con un codice letto (non orfano);
// un orfano senza candidati, o un file senza letture, è nessun_candidato. Un orfano senza candidati segue la tabella di
// E1 (§2.5) anche se è discordante (R-74 della revisione di V2): fuori_richiesta_non_confermato se la collocazione è
// fuori richiesta, altrimenti nessun_candidato; il discordante con candidati, o pertinente per il contesto, resta
// discordante.
func TestR108LaFunzione(t *testing.T) {
	dentro := valutazione.PerimetroDentro
	for _, c := range []struct {
		nome               string
		perimetro          string
		terminale, orfano  bool
		a                  ancoraggio.Associazione
		col                ancoraggio.Collocazione
		candidati, letture int
		motivo             valutazione.MotivoSmistamento
		entra              bool
	}{
		{"ambiguo", dentro, false, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneNonDeterminabile, 2, 1, valutazione.MotivoSmistamentoAssociazioneAmbigua, true},
		{"discordante senza candidati, orfano (R-74)", dentro, false, true, ancoraggio.AssociazioneDiscordante, ancoraggio.CollocazioneNonDeterminabile, 0, 2, valutazione.MotivoSmistamentoNessunCandidato, true},
		{"discordante senza candidati, orfano, fuori richiesta (R-74)", dentro, false, true, ancoraggio.AssociazioneDiscordante, ancoraggio.CollocazioneFuoriRichiesta, 0, 2, valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato, true},
		{"discordante senza candidati, pertinente per il contesto", dentro, false, false, ancoraggio.AssociazioneDiscordante, ancoraggio.CollocazioneFuoriRichiesta, 0, 2, valutazione.MotivoSmistamentoAssociazioneDiscordante, true},
		{"discordante con candidati fuori dai target, orfano", dentro, false, true, ancoraggio.AssociazioneDiscordante, ancoraggio.CollocazioneFiglio, 1, 2, valutazione.MotivoSmistamentoAssociazioneDiscordante, true},
		{"il codice letto e il target senza struttura", dentro, false, false, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, 0, 1, valutazione.MotivoSmistamentoCollocazioneNonDeterminabile, true},
		{"lo stesso, orfano", dentro, false, true, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, 0, 1, valutazione.MotivoSmistamentoNessunCandidato, true},
		{"nessuna lettura, pertinente per il contesto", dentro, false, false, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneNonDeterminabile, 0, 0, valutazione.MotivoSmistamentoNessunCandidato, true},
		{"fuori richiesta proposto", dentro, false, true, ancoraggio.AssociazioneNessunCandidato, ancoraggio.CollocazioneFuoriRichiesta, 0, 1, valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato, true},
		{"un file che l'adattatore non legge, orfano", dentro, false, true, "", "", 0, 0, valutazione.MotivoSmistamentoNessunCandidato, true},
		{"la pre-associazione: non entra", dentro, false, false, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneFiglio, 1, 1, "", false},
		{"terminale: mai", dentro, true, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneFiglio, 2, 1, "", false},
		{"fuori perimetro: mai", valutazione.PerimetroMessaggioInUscita, false, false, ancoraggio.AssociazioneAmbiguo, ancoraggio.CollocazioneFiglio, 2, 1, "", false},
	} {
		m, entra := valutazione.VoceDaSmistarePerProva(c.perimetro, c.terminale, c.orfano, c.a, c.col, c.candidati, c.letture)
		if m != c.motivo || entra != c.entra {
			t.Errorf("%s: motivo %q, entra %v; attesi %q, %v", c.nome, m, entra, c.motivo, c.entra)
		}
	}
}

// TestConflittiIdentitaAttribuiti (T-E1R-08; F0-13): un pezzo identita_documento di B5, senza prodotto, va sullo
// smistamento dei prodotti a cui il documento è pertinente (gli allegati del documento), uno per prodotto; un pezzo che
// ha già il prodotto resta com'è; senza prodotti resta con il prodotto vuoto, mai perso. Gli allegati del documento sono
// in conflitto, e l'uscita non condivide memoria con i pezzi.
func TestConflittiIdentitaAttribuiti(t *testing.T) {
	doc, all := uid(0xb31), uid(0xb32)
	altroDoc, altroAll := uid(0xb33), uid(0xb34)
	documenti := []fotorfq.DocumentoConfermato{{ID: doc, Allegati: []uuid.UUID{all}}, {ID: altroDoc, Allegati: []uuid.UUID{altroAll}}}
	pezzo := func(d uuid.UUID, prodotto string) valutazione.Conflitto {
		dd := d
		return valutazione.Conflitto{Tipo: valutazione.ConflittoIdentitaDocumento, Asse: valutazione.AsseSmistamento, Rif: valutazione.RifDocumento(d),
			Prodotto: prodotto, Decisione: "7120200A 1", Proposta: "7120200A2", Motivo: ancoraggio.ParteDiscordanzaRevisione,
			EvidenzaProposta: valutazione.EvidenzaProposta{DocumentoID: &dd, Riferimenti: []string{"nodo:x"}}}
	}
	pezzi := []valutazione.Conflitto{pezzo(doc, ""), pezzo(altroDoc, ""), pezzo(doc, rifP2s)}
	out, segnati := valutazione.AttribuisciIdentitaPerProva(pezzi, documenti, map[uuid.UUID][]string{all: {rifP1, rifP2s}, altroAll: nil})
	var prodotti []string
	for _, c := range out {
		prodotti = append(prodotti, c.Prodotto)
	}
	if !reflect.DeepEqual(prodotti, []string{rifP1, rifP2s, "", rifP2s}) || !segnati[all] || !segnati[altroAll] {
		t.Errorf("prodotti %q, segnati %v", prodotti, segnati)
	}
	out[0].EvidenzaProposta.Riferimenti[0] = "cambiato"
	*out[0].EvidenzaProposta.DocumentoID = uid(0xfff)
	if pezzi[0].EvidenzaProposta.Riferimenti[0] != "nodo:x" || *pezzi[0].EvidenzaProposta.DocumentoID != doc {
		t.Error("l'uscita condivide memoria con i pezzi")
	}
}
