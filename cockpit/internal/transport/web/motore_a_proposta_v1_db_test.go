//go:build integrazione

// L4 — direttiva 39, corsia motore: R15 e la proposta di adesso con la fotografia del calcolo. Il difetto segnalato dalla
// corsia persistenza in C-3: propostaDiAdessoV1 chiamava propostabom.ImportaGruppo senza Ingresso.Foto, l'adattatore
// rifiuta sempre senza la fotografia, quindi la proposta di adesso era sempre «non disponibile» e R15 rispondeva sempre
// ANTEPRIMA_SUPERATA. Qui il calcolo vero del Motore A (regole ACME, scena B87 di motore_a_db_test.go) e il gruppo dello
// STEP del prodotto: la proposta di adesso è disponibile, con la stessa impronta_contenuto dell'adattatore chiamato con la
// fotografia; senza la fotografia l'adattatore rifiuta (la controprova del difetto); un gruppo che non c'è resta «non
// disponibile»; nessuna scrittura.
package web

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestL4V1R15LaPropostaDiAdessoConLaFotografia(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MR15")
	mwAccendi(t, b, s.regole)
	ris, err := b.ws.servizioMotoreA().Calcola(b.ctx, s.thread)
	if err != nil {
		t.Fatalf("calcolo del motore: %v", err)
	}
	rif, gruppo := "", ""
	for _, pv := range ris.EsitoThread.ProdottiValutati {
		if pv.ComponenteID == nil || *pv.ComponenteID != s.prodotto {
			continue
		}
		rif = pv.Rif
		for _, g := range propostabom.GruppiDelProdotto(pv, ris.EsitoThread.Ancoraggi.Strutture) {
			if g.AllegatoID != nil && *g.AllegatoID == s.allStep && g.Tipo != propostabom.GruppoSenzaSTEP && gruppo == "" {
				gruppo = g.Chiave
			}
		}
	}
	if rif == "" || gruppo == "" {
		t.Fatalf("la scena non ha il gruppo dello STEP del prodotto (rif %q, gruppo %q)", rif, gruppo)
	}
	in := propostabom.Ingresso{Esito: &ris.Esito, Thread: ris.EsitoThread, Regole: ris.Regole, Prodotto: rif, Gruppo: gruppo, NonAttivi: ris.V1.NonAttivi}
	if _, err := propostabom.ImportaGruppo(in); err == nil {
		t.Fatal("controprova: senza la fotografia l'adattatore dovrebbe rifiutare (il difetto di R15)")
	}
	in.Foto = &ris.Foto
	atteso, err := propostabom.ImportaGruppo(in)
	if err != nil {
		t.Fatalf("con la fotografia il gruppo si importa: %v", err)
	}
	prima := fotoDelDatabase(t, b)
	out, err := b.ws.propostaDiAdessoV1(b.ctx, s.thread, distintabozza.RichiestaR15{ProdottoID: s.prodotto, ConfermaID: uuid.New(), Gruppo: gruppo})
	if err != nil {
		t.Fatalf("propostaDiAdessoV1: %v", err)
	}
	if !out.Disponibile || out.ImprontaContenuto != atteso.Contenuto.ImprontaContenuto || out.ImprontaContenuto == "" {
		t.Errorf("la proposta di adesso: disponibile %v, impronta %q; attesa disponibile con %q", out.Disponibile, out.ImprontaContenuto,
			atteso.Contenuto.ImprontaContenuto)
	}
	if len(out.Differenze) != 0 {
		t.Errorf("senza una conferma formato 3 con un registro non ci sono differenze: %s", out.Differenze)
	}
	// un gruppo che non c'è: la proposta che l'ingegnere ha visto non c'è più
	no, err := b.ws.propostaDiAdessoV1(b.ctx, s.thread, distintabozza.RichiestaR15{ProdottoID: s.prodotto, ConfermaID: uuid.New(), Gruppo: "gruppo-che-non-esiste"})
	if err != nil || no.Disponibile {
		t.Errorf("un gruppo che non c'è: disponibile %v, err %v", no.Disponibile, err)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("la proposta di adesso ha scritto nel database: %s", d)
	}
}
