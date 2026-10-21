//go:build integrazione

// L4 — direttiva 35, blocco C-4: la fonte «manuale» del Motore A dalle AR attive, lette dal servizio unico nella stessa
// fotografia (leggiV1, registrateDelThread, valutazione.Ingressi.Registrate), con il ripiego transitorio [T] per le
// copie senza AR (piano delle dipendenze §7.2 C-4). La prova dell'impronta degli ancoraggi prima/dopo, sulla scena ACME
// del database di prova (scenaMA), con il calcolo vero:
//  1. senza AR: le Registrate sono nil e l'esito è quello di un calcolo senza Registrate (impronta dell'esito e degli
//     ancoraggi uguali);
//  2. la copia senza AR (una pre-assegnazione legacy): il «manuale» viene dal ripiego, l'esito è quello di prima;
//  3. una AR uguale alla copia: le Registrate la portano, l'esito (e l'impronta degli ancoraggi) non cambia;
//  4. la copia tolta fuori dal servizio con la AR ancora attiva: il «manuale» resta quello della AR (la copia non è una
//     seconda fonte) e l'impronta degli ancoraggi è quella del passo 3;
//  5. la lettura non scrive niente.
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx), come nella scena.
package motore_a

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/testutil"
)

// arDiProva: una AR attiva (origine conferma_bom, con la sua testata di conferma) scritta a mano con i trigger delle due
// tabelle spenti solo nella sua transazione, come arAttivaDiProva delle prove della persistenza: la scena non ha una
// conferma formato 3 vera, e qui conta solo che il servizio unico la legga fra le righe attive.
func arDiProva(t *testing.T, p *pgxpool.Pool, thread, prodotto, allegato, componente, utente uuid.UUID) uuid.UUID {
	t.Helper()
	conferma, ar := uuid.New(), uuid.New()
	inTransazione(t, p,
		passoSQL{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		passoSQL{`ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER`, nil},
		passoSQL{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`, nil},
		passoSQL{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			VALUES ($1, $2, $3, (SELECT COALESCE(max(numero), 0) + 1 FROM conferma_strutturale WHERE prodotto_id = $3), 1, true,
				repeat('3', 64), 0, 0, 0, '{"rotta":"prova"}', '{"formato":1}', repeat('9', 64), '{}', '{}', 'nessuno', $4)`,
			[]any{conferma, thread, prodotto, utente}},
		passoSQL{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, codice, tipo)
			SELECT $1, c.thread_id, c.componente_id, c.codice, c.tipo FROM componente c WHERE c.componente_id = $2`, []any{conferma, componente}},
		passoSQL{`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
				chiave_idempotenza, creata_da, provenienza_pb)
			VALUES ($1, $2, $3, $4, 'conferma_bom', $5, 'k-prova', gen_random_uuid(), $6, 'manuale')`, []any{ar, thread, allegato, componente, conferma, utente}},
		passoSQL{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		passoSQL{`ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, nil},
		passoSQL{`ALTER TABLE conferma_strutturale_componente ENABLE TRIGGER USER`, nil},
		passoSQL{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil})
	return ar
}

// calcoloDoppio: il calcolo vero del servizio e, sulla stessa fotografia, lo stesso calcolo senza le Registrate.
type calcoloDoppio struct {
	registrate    map[uuid.UUID]map[uuid.UUID]uuid.UUID
	con, senza    valutazione.Esito
	foto          fotorfq.Fotografia
	thread        uuid.UUID
	manualeDelPdf *uuid.UUID
}

func calcolaDoppio(t *testing.T, p *pgxpool.Pool, r Regole, s scenaMA) calcoloDoppio {
	t.Helper()
	var out calcoloDoppio
	svc := Servizio{Regole: r, Pool: p, valuta: func(f fotorfq.Fotografia, ins *motorea.InsiemeRegole, in valutazione.Ingressi) (valutazione.Esito, error) {
		out.registrate, out.foto = in.Registrate, f
		senza := in
		senza.Registrate = nil
		e0, err := valutazione.Calcola(f, ins, senza)
		if err != nil {
			return e0, err
		}
		out.senza = e0
		return valutazione.Calcola(f, ins, in)
	}}
	ris, err := svc.Calcola(context.Background(), s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	out.con, out.thread = ris.Esito, s.thread
	for _, a := range ris.EsitoThread.Associazioni {
		if a.AllegatoID == s.aPdf && a.Origine == ancoraggio.OrigineManuale {
			out.manualeDelPdf = a.Manuale
		}
	}
	return out
}

func (c calcoloDoppio) ancoraggi(e valutazione.Esito) string {
	for _, et := range e.Thread {
		if et.ThreadID == c.thread {
			return et.Ancoraggi.Impronta
		}
	}
	return ""
}

func TestL4V1C4LaFonteManualeDalleAR(t *testing.T) {
	p, s, r := preparaMA(t)
	prima := testutil.FotoDelDatabase(t, p)

	// 1. senza AR
	c1 := calcolaDoppio(t, p, r, s)
	if c1.registrate != nil || c1.con.Impronta != c1.senza.Impronta || c1.ancoraggi(c1.con) != c1.ancoraggi(c1.senza) || c1.ancoraggi(c1.con) == "" ||
		c1.manualeDelPdf != nil {
		t.Fatalf("senza AR: registrate %v, impronta %s / %s, ancoraggi %s / %s, manuale %v", c1.registrate, c1.con.Impronta, c1.senza.Impronta,
			c1.ancoraggi(c1.con), c1.ancoraggi(c1.senza), c1.manualeDelPdf)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo ha cambiato il database: %v", d)
	}

	// 2. la copia senza AR (pre-assegnazione legacy): il ripiego
	// la copia scritta fuori dal servizio come un dato legacy P1: con trg_proposta_copia_derivata acceso (0023 20261019d)
	// si prepara con testutil.CopiaLegacy (aggancio della corsia interfaccia all'unione con v1-persistenza a9b7aa9)
	testutil.CopiaLegacy(t, p, `UPDATE documento_proposta SET componente_id = $2, codice = '9120101' WHERE allegato_id = $1`, s.aPdf, s.figlio)
	c2 := calcolaDoppio(t, p, r, s)
	if c2.registrate != nil || c2.con.Impronta != c2.senza.Impronta || c2.manualeDelPdf == nil || *c2.manualeDelPdf != s.figlio {
		t.Fatalf("la copia senza AR: registrate %v, impronta %s / %s, manuale %v", c2.registrate, c2.con.Impronta, c2.senza.Impronta, c2.manualeDelPdf)
	}
	if c2.ancoraggi(c2.con) == c1.ancoraggi(c1.con) {
		t.Log("la copia senza AR non cambia l'impronta degli ancoraggi della scena (il PDF era già sul figlio)")
	}

	// 3. una AR uguale alla copia
	arDiProva(t, p, s.thread, s.prodotto, s.aPdf, s.figlio, s.utente)
	c3 := calcolaDoppio(t, p, r, s)
	if c3.registrate[s.thread][s.aPdf] != s.figlio || len(c3.registrate[s.thread]) != 1 {
		t.Fatalf("le Registrate dalla AR: %v", c3.registrate)
	}
	if c3.con.Impronta != c3.senza.Impronta || c3.ancoraggi(c3.con) != c2.ancoraggi(c2.con) || c3.manualeDelPdf == nil || *c3.manualeDelPdf != s.figlio {
		t.Errorf("la AR uguale alla copia cambia l'esito: impronta %s / %s, ancoraggi %s / %s, manuale %v", c3.con.Impronta, c3.senza.Impronta,
			c3.ancoraggi(c3.con), c2.ancoraggi(c2.con), c3.manualeDelPdf)
	}

	// 4. la copia tolta fuori dal servizio, la AR ancora attiva: vince la AR
	inTransazione(t, p,
		passoSQL{`ALTER TABLE documento_proposta DISABLE TRIGGER USER`, nil},
		passoSQL{`UPDATE documento_proposta SET componente_id = NULL, codice = '9120101' WHERE allegato_id = $1`, []any{s.aPdf}},
		passoSQL{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		passoSQL{`ALTER TABLE documento_proposta ENABLE TRIGGER USER`, nil})
	c4 := calcolaDoppio(t, p, r, s)
	if c4.manualeDelPdf == nil || *c4.manualeDelPdf != s.figlio || c4.ancoraggi(c4.con) != c3.ancoraggi(c3.con) || c4.con.Impronta == c4.senza.Impronta {
		t.Errorf("la copia tolta, la AR attiva: manuale %v, ancoraggi %s / %s, impronta con %s, senza le Registrate %s", c4.manualeDelPdf,
			c4.ancoraggi(c4.con), c3.ancoraggi(c3.con), c4.con.Impronta, c4.senza.Impronta)
	}
	for _, x := range c4.foto.Thread {
		for _, pr := range x.Proposte {
			if pr.AllegatoID == s.aPdf && pr.ComponenteID != nil {
				t.Errorf("la fotografia non è quella del database: la proposta del PDF ha il componente %v", pr.ComponenteID)
			}
		}
	}
	t.Logf("impronte degli ancoraggi: senza AR %s, copia %s, AR = copia %s, AR senza copia %s", c1.ancoraggi(c1.con), c2.ancoraggi(c2.con),
		c3.ancoraggi(c3.con), c4.ancoraggi(c4.con))
}
