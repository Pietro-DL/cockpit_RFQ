package classificazione

import (
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// TestLaTabellaDeiPunteggi (Smistamento, prova 222, A5.14.3): ogni regola della tabella S1 ha una dimensione,
// una fonte dell'enum fonte_proposta (la destinazione nessuna), uno score fra 0 e 95 (100 e' solo la decisione
// di una persona), le parole per la schermata e un perche'. L'impronta della tabella e' la costante accanto a
// TabellaPunteggi: chi cambia uno score la cambia, e con lei la versione.
func TestLaTabellaDeiPunteggi(t *testing.T) {
	if TabellaPunteggi != "S1" {
		t.Errorf("versione della tabella: %q", TabellaPunteggi)
	}
	if got := improntaTabella(); got != ImprontaPunteggi {
		t.Fatalf("la tabella S1 e' cambiata (impronta %s, la costante dice %s): chi cambia uno score cambia anche la "+
			"versione TabellaPunteggi e la costante ImprontaPunteggi", got, ImprontaPunteggi)
	}
	if len(Punteggi) != len(tabellaS1) {
		t.Errorf("regole con lo stesso nome: %d righe, %d nomi", len(tabellaS1), len(Punteggi))
	}
	dimensioni := map[string]bool{DimTipo: true, DimCodice: true, DimRev: true, DimDestinazione: true}
	per := map[string]int{}
	for _, r := range tabellaS1 {
		per[r.Dimensione]++
		if r.Parole == "" || r.Perche == "" {
			t.Errorf("%s: senza parole o senza perche'", r.id)
		}
		if precedenza(r.id) >= len(tabellaS1) {
			t.Errorf("%s: senza precedenza", r.id)
		}
		if r.id == RegolaOperatore {
			if r.Score != 100 || r.Fonte != "operatore" || r.Dimensione != "" {
				t.Errorf("la decisione di una persona: %+v", r.RegolaScore)
			}
			continue
		}
		if !dimensioni[r.Dimensione] {
			t.Errorf("%s: dimensione %q", r.id, r.Dimensione)
		}
		if r.Score < 0 || r.Score > 95 {
			t.Errorf("%s: score %d fuori da 0–95 (100 vuol dire «l'ha deciso una persona»)", r.id, r.Score)
		}
		switch {
		case r.Dimensione == DimDestinazione && r.Fonte != "":
			t.Errorf("%s: la destinazione non ha una fonte di documento_proposta: %q", r.id, r.Fonte)
		case r.Dimensione != DimDestinazione && !db.FonteProposta(r.Fonte).Valid():
			t.Errorf("%s: fonte %q fuori dall'enum fonte_proposta", r.id, r.Fonte)
		case r.Fonte == "operatore":
			t.Errorf("%s: solo una decisione ha la fonte operatore", r.id)
		}
	}
	for d := range dimensioni {
		if per[d] == 0 {
			t.Errorf("nessuna regola per la dimensione %s", d)
		}
	}
	// i numeri della raccomandazione A della Domanda 4, e le costanti di oggi riusate dove la regola e' la stessa
	for regola, score := range map[string]int{"ext_3d": 95, "nome_codice_generico": 45, "nome_codice_famiglia": 70,
		"rev_suffisso_nome": 40, "rev_esplicita_nome": 55, "step_radice_famiglia": PuntiFamiglia, "step_radice_generico": PuntiGenerico,
		"step_primo_product": PuntiGenerico, "pdf_termini_cartiglio": 75, "nome_contiene_codice": 25} {
		if Punteggi[regola].Score != score {
			t.Errorf("%s: score %d, atteso %d", regola, Punteggi[regola].Score, score)
		}
	}
	// la mappa regola → fonte di A5.14.3: un codice dal nome non e' mai «cartiglio», la radice di famiglia e'
	// del cliente
	for regola, fonte := range map[string]string{"ext_3d": "estensione", "nome_codice_generico": "nome_file", "nome_codice_famiglia": "nome_file",
		"rev_nas": "nome_file", "step_primo_product": "step", "step_radice_famiglia": "regola_cliente", "pdf_termini_cartiglio": "cartiglio",
		"nome_so_uscita": "direzione", "risposta_fornitore": "direzione", "rumore_immagine": "rumore"} {
		if Punteggi[regola].Fonte != fonte {
			t.Errorf("%s: fonte %q, attesa %q", regola, Punteggi[regola].Fonte, fonte)
		}
	}
}
