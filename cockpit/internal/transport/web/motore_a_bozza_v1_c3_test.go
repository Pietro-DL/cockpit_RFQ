package web

// L1 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: la traduzione dei rifiuti delle forme di R2 che importano
// (EM §6.1, V-7; U2 §2.8): FONTE_NON_CONFERMATA (422, fuori dall'elenco confrontato con il ponte finche' il ponte non lo
// conosce), il 409 ANTEPRIMA_SUPERATA con anteprima, impronta del contenuto di adesso e gruppo ritrovato,
// gruppo_su_altra_occorrenza con l'allegato del gruppo.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestBozzaV1C3RifiutiDelleFormeCheImportano(t *testing.T) {
	stato, corpo := rispostaErroreBozzaV1(&distintabozza.Errore{Codice: distintabozza.CodiceFonteNonConfermata, Campo: "crea.sorgente", Messaggio: "x"})
	if stato != http.StatusUnprocessableEntity || corpo.Esito != "rifiutato" || corpo.Codice != distintabozza.CodiceFonteNonConfermata {
		t.Fatalf("FONTE_NON_CONFERMATA: %d %+v", stato, corpo)
	}
	impronta := strings.Repeat("a", 64)
	stato, corpo = rispostaErroreBozzaV1(&distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Motivo: distintabozza.MotivoPropostaStep,
		Anteprima: distintabozza.MotivoPropostaStep, ImprontaContenuto: impronta,
		GruppoCorrente: &distintabozza.GruppoRitrovato{Chiave: "bom", Come: "stessa_occorrenza", ImprontaContenuto: impronta}, Messaggio: "x"})
	b, _ := json.Marshal(corpo)
	if stato != http.StatusConflict || corpo.Esito != "conflitto" || !strings.Contains(string(b), `"anteprima":"proposta_step"`) ||
		!strings.Contains(string(b), `"impronta_contenuto":"`+impronta+`"`) || !strings.Contains(string(b), `"gruppo_corrente":{"chiave":"bom","come":"stessa_occorrenza"`) {
		t.Fatalf("ANTEPRIMA_SUPERATA: %d %s", stato, b)
	}
	a := uuid.New()
	stato, corpo = rispostaErroreBozzaV1(&distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Motivo: distintabozza.MotivoGruppoSuAltraOccorrenza,
		AllegatoID: &a, Messaggio: "x"})
	if stato != http.StatusUnprocessableEntity || corpo.AllegatoID == nil || *corpo.AllegatoID != a {
		t.Fatalf("gruppo_su_altra_occorrenza: %d %+v", stato, corpo)
	}
	for _, c := range []string{distintabozza.CodicePropostaNonDisponibile, distintabozza.CodiceImportazioneTroppoGrande} {
		if stato, _ := rispostaErroreBozzaV1(&distintabozza.Errore{Codice: c}); stato != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", c, stato)
		}
	}
}
