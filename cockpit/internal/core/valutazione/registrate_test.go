// L1 — la fonte «manuale» del Motore A dalle associazioni registrate (direttiva 35 §8 C-4 [U]; piano delle dipendenze
// della direttiva 35 §7.2 C-4 e §12 V-02 [T]; registrate.go), sulla scena dello smistamento con due prodotti di
// TestPO27GliOrfani (il fuori_richiesta 7120555A_2.pdf con l'«assegna» sul secondo prodotto: la copia di una proposta
// aperta, senza AR):
//   - la prova dell'impronta prima/dopo: senza AR (Registrate nil, vuota, o con le AR di un altro thread) l'esito è
//     quello di prima byte per byte (Impronta dell'esito, impronta degli ancoraggi, impronta della fotografia); con una AR
//     uguale alla copia, anche;
//   - una AR su un altro componente vince sulla copia (la copia non è una seconda fonte): «manuale» e pertinenza seguono
//     la AR; l'impronta della fotografia resta quella della fotografia letta, che non cambia;
//   - una AR su una proposta aperta senza copia dà il «manuale»; una AR su una proposta non aperta non cambia niente.
//
// I clienti, i codici e gli ID sono inventati (ACME, 712xxxx), come nelle altre scene.

package valutazione_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// scenaRegistrate: la scena di TestPO27GliOrfani con il fuori_richiesta fG assegnato a mano al secondo prodotto (la
// copia della proposta aperta) e una proposta aperta senza copia sul figlio condiviso fS.
func scenaRegistrate(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaSmistamento(t)
	secondoProdotto(t, &th, "7120200A")
	conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
	fileNelMessaggio(&th, fS, mNessuno, "7120200A_1.pdf", "pdf", shaN(83), scansione(t, shaN(83)))
	fileNelMessaggio(&th, fG, mNessuno, "7120555A_2.pdf", "pdf", shaN(85), scansione(t, shaN(85)))
	p2 := cP2
	conProposta(&th, uid(0xb85), fG, "disegno_2d", "nome_file", "aperta", &p2, nil)
	conProposta(&th, uid(0xb86), fS, "disegno_2d", "nome_file", "aperta", nil, nil)
	vistaCome(&th)
	return th
}

// esitoConRegistrate: l'esito della fotografia del thread con le AR date (nil = nessuna).
func esitoConRegistrate(t *testing.T, th fotorfq.Thread, ar map[uuid.UUID]map[uuid.UUID]uuid.UUID) valutazione.Esito {
	t.Helper()
	return calcola(t, fotografia(th), insiemeACME(t), valutazione.Ingressi{Versione: valutazione.VersioneCasi, Registrate: ar})
}

func jsonRegistrate(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestV1C4LaFonteManualeDalleAR(t *testing.T) {
	th := scenaRegistrate(t)
	prima := esitoConRegistrate(t, th, nil)
	etPrima := esitoThread(t, prima, threadACME)
	if a := associazioneDi(t, etPrima, fG); a.Manuale == nil || *a.Manuale != cP2 || a.Origine != ancoraggio.OrigineManuale {
		t.Fatalf("la scena: il fuori_richiesta con l'«assegna» (la copia) %+v", a)
	}
	if a := associazioneDi(t, etPrima, fS); a.Manuale != nil {
		t.Fatalf("la scena: il figlio condiviso senza copia %+v", a)
	}

	t.Run("senza AR l'esito è quello di prima, byte per byte", func(t *testing.T) {
		altro := uuid.MustParse("00000000-0000-4000-8000-00000000fff1")
		for nome, ar := range map[string]map[uuid.UUID]map[uuid.UUID]uuid.UUID{
			"vuota":              {},
			"thread senza AR":    {threadACME: {}},
			"AR di altri thread": {altro: {fG: cSciolto}},
		} {
			dopo := esitoConRegistrate(t, th, ar)
			etDopo := esitoThread(t, dopo, threadACME)
			if dopo.Impronta != prima.Impronta || dopo.ImprontaFotografia != prima.ImprontaFotografia ||
				etDopo.Ancoraggi.Impronta != etPrima.Ancoraggi.Impronta || jsonRegistrate(t, dopo) != jsonRegistrate(t, prima) {
				t.Errorf("%s: l'esito cambia senza AR (impronta %s / %s, ancoraggi %s / %s)", nome, dopo.Impronta, prima.Impronta,
					etDopo.Ancoraggi.Impronta, etPrima.Ancoraggi.Impronta)
			}
		}
		t.Logf("impronta dell'esito %s, degli ancoraggi %s, della fotografia %s", prima.Impronta, etPrima.Ancoraggi.Impronta, prima.ImprontaFotografia)
	})

	t.Run("una AR uguale alla copia non cambia l'esito", func(t *testing.T) {
		dopo := esitoConRegistrate(t, th, map[uuid.UUID]map[uuid.UUID]uuid.UUID{threadACME: {fG: cP2}})
		if dopo.Impronta != prima.Impronta || esitoThread(t, dopo, threadACME).Ancoraggi.Impronta != etPrima.Ancoraggi.Impronta {
			t.Errorf("la AR uguale alla copia cambia l'esito: %s / %s", dopo.Impronta, prima.Impronta)
		}
	})

	t.Run("la AR su un altro componente vince sulla copia", func(t *testing.T) {
		copia := append([]fotorfq.PropostaAttuale(nil), th.Proposte...)
		dopo := esitoConRegistrate(t, th, map[uuid.UUID]map[uuid.UUID]uuid.UUID{threadACME: {fG: cSciolto}})
		et := esitoThread(t, dopo, threadACME)
		a := associazioneDi(t, et, fG)
		if a.Manuale == nil || *a.Manuale != cSciolto || a.Origine != ancoraggio.OrigineManuale || !reflect.DeepEqual(a.Pertinente, []string{rifProdB}) {
			t.Errorf("il «manuale» dalla AR: %+v", a)
		}
		if dopo.Impronta == prima.Impronta {
			t.Error("con la AR su un altro componente l'esito non cambia")
		}
		if dopo.ImprontaFotografia != prima.ImprontaFotografia || !reflect.DeepEqual(th.Proposte, copia) {
			t.Error("la fotografia di chi chiama è cambiata")
		}
	})

	t.Run("la AR su una proposta aperta senza copia dà il «manuale»", func(t *testing.T) {
		et := esitoThread(t, esitoConRegistrate(t, th, map[uuid.UUID]map[uuid.UUID]uuid.UUID{threadACME: {fS: cP2}}), threadACME)
		if a := associazioneDi(t, et, fS); a.Manuale == nil || *a.Manuale != cP2 || a.Origine != ancoraggio.OrigineManuale {
			t.Errorf("il figlio condiviso con la AR: %+v", a)
		}
	})

	t.Run("una proposta non aperta non cambia", func(t *testing.T) {
		th2 := scenaRegistrate(t)
		for i := range th2.Proposte {
			if th2.Proposte[i].AllegatoID == fS {
				th2.Proposte[i].Stato = "scartata"
			}
		}
		senza := esitoConRegistrate(t, th2, nil)
		con := esitoConRegistrate(t, th2, map[uuid.UUID]map[uuid.UUID]uuid.UUID{threadACME: {fS: cP2}})
		if con.Impronta != senza.Impronta {
			t.Errorf("una AR su una proposta scartata cambia l'esito: %s / %s", con.Impronta, senza.Impronta)
		}
	})
}
