//go:build integrazione

// L4 — direttiva 39, corsia motore: la fonte del prodotto in /dati con le conferme di S8 vere sul database di prova
// (scena ACME di servizio_db_test.go: lo STEP 9120100A con la radice e un figlio, autorizzato per il prodotto):
//   - prima di S8: il documento legacy è «confermata» e autorizzato, lo STEP storico univoco è il legame storico, la
//     conferma ammessa è S8 (gesto conferma_sorgente), la struttura dello STEP è con_struttura con i figli estratti;
//   - dopo la riga conferma_documento di S8 (con la chiave del passo 2): sorgente_confermata, la conferma non è più
//     ammessa, il legame storico non c'è più, sorgente_s8 valida con data e autore (confermabom.SorgenteValida);
//   - dopo la revoca di S8 scritta dalla funzione vera del core (sorgente.Revoca: fonte_step_evento con l'azione
//     revoca_sorgente della variante 20261019c della 0023): sorgente_s8 non valida con il motivo revocata, S8 di nuovo
//     ammessa, e l'autorizzazione della working resta com'era, con la sua data (R39-10); in S9 lo STEP non è più
//     «legato come sorgente»;
//   - S9 mostra la provenienza della PB scritta nello snapshot (D-PB03 A [U]; richiesta 1 del blocco precedente);
//   - il calcolo non scrive.
package motore_a

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
	"promatec/cockpit/internal/platform/testutil"
)

func TestL4V1D39LaFonteConLeConfermeDiS8(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	fonte := func() *FonteDelProdotto {
		t.Helper()
		prima := testutil.FotoDelDatabase(t, p)
		pg, err := svc.Pagina(ctx, RichiestaPagina{Thread: s.thread})
		if err != nil {
			t.Fatal(err)
		}
		if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
			t.Errorf("il calcolo ha cambiato il database: %v", d)
		}
		if pg.Fonte == nil || pg.Fonte.AllegatoID == nil || *pg.Fonte.AllegatoID != s.aStep {
			t.Fatalf("la fonte del prodotto è lo STEP della scena: %+v", pg.Fonte)
		}
		return pg.Fonte
	}
	sha := ""
	if err := p.QueryRow(ctx, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.aStep).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	// un'autorizzazione registrata (come la scriverebbe R5 o la pagina legacy), per vedere che la revoca di S8 non la tocca
	inTransazione(t, p, passoSQL{`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, autore_id)
		VALUES ($1, $2, 'prodotto', 'autorizzazione', $3, $4, $5, $6)`, []any{s.thread, s.prodotto, s.aStep, sha, s.docStep, s.utente}})

	f := fonte()
	if !f.Confermata || !f.Autorizzata || f.SorgenteConfermata || !f.ConfermaAmmessa || f.Gesto != GestoConfermaSorgente || f.SorgenteS8 != nil {
		t.Errorf("prima di S8: %+v", f)
	}
	if f.LegameStorico == nil || f.LegameStorico.DocumentoID != s.docStep || f.LegameStorico.Sha256 != sha {
		t.Errorf("lo STEP storico univoco: %+v", f.LegameStorico)
	}
	if st := f.StrutturaSTEP; st == nil || st.StatoAnalisi != propostabom.AnalisiPronta || st.Struttura != propostabom.StrutturaConFigli || st.FigliEstratti < 1 ||
		st.Partenza != propostabom.PartenzaStep || st.PresaDAttoRichiesta {
		t.Errorf("la struttura dello STEP della scena: %+v", f.StrutturaSTEP)
	}
	autorizzataIl := f.AutorizzataIl
	if autorizzataIl == nil || f.AutorizzataMotivo != "" {
		t.Fatalf("l'autorizzazione registrata: %+v", f)
	}

	// la conferma di S8 (passo 2: conferma_documento con la chiave k2)
	inTransazione(t, p, passoSQL{`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id,
		chiave_idempotenza, autore_id) VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, 'bom', $5, $6, $7)`,
		[]any{s.thread, s.prodotto, s.aStep, sha, s.docStep, uuid.New(), s.utente}})
	f = fonte()
	if !f.SorgenteConfermata || f.ConfermaAmmessa || f.Gesto != "" || f.Motivo != MotivoFonteGiaConfermata || f.LegameStorico != nil {
		t.Errorf("dopo S8: %+v", f)
	}
	if x := f.SorgenteS8; x == nil || !x.Valida || x.Motivo != "" || x.AllegatoID != s.aStep || x.DocumentoID == nil || *x.DocumentoID != s.docStep ||
		x.ConfermataIl == nil || x.ConfermataDa == nil || *x.ConfermataDa != s.utente {
		t.Errorf("sorgente_s8 dopo S8: %+v", f.SorgenteS8)
	}

	// S9 con la sorgente confermata: lo STEP è «legato come sorgente»
	legato := func() int {
		t.Helper()
		so, err := svc.Sorgente(ctx, s.thread)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, ps := range so.Prodotti {
			for _, rg := range ps.Righe {
				if rg.AllegatoID == s.aStep && rg.Stato == StatoRigaLegatoComeSorgente {
					n++
				}
			}
		}
		return n
	}
	if n := legato(); n != 1 {
		t.Errorf("S9 dopo S8: %d righe legato_come_sorgente dello STEP, attesa 1", n)
	}
	// una bozza aperta con la PB del PDF sul figlio, con la provenienza nello snapshot (D-PB03 A [U]): S9 la mostra nella riga
	em := distintabozza.EmittenteCliente(s.cliente)
	bozza3DiProva(t, p, s.thread, s.prodotto, s.utente, map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"nodi":      []any{nodoDiProva("k-p", &s.prodotto, "finito", "9120100", "", em), nodoDiProva("k-f", &s.figlio, "sciolto", "9120101", "", em)},
		"archi":     []any{map[string]any{"padre": "k-p", "figlio": "k-f", "qta": 2, "origine": "utente"}},
		"decisioni": []any{}, "documenti_preassociati": []any{map[string]any{"allegato_id": s.aPdf.String(), "chiave_bozza": "k-f", "provenienza": "manuale"}}})
	if so, err := svc.Sorgente(ctx, s.thread); err != nil {
		t.Fatal(err)
	} else {
		trovata := false
		for _, ps := range so.Prodotti {
			for _, rg := range ps.Righe {
				if rg.AllegatoID == s.aPdf && rg.Origine == OrigineRigaBozza {
					trovata = true
					if rg.Stato != StatoRigaSalvato || rg.Provenienza != "manuale" || rg.Destinazione.ChiaveBozza != "k-f" {
						t.Errorf("la PB del PDF in S9: %+v", rg)
					}
				}
			}
		}
		if !trovata {
			t.Errorf("la PB del PDF non è fra le righe di S9: %+v", so.Prodotti)
		}
	}

	// la revoca di S8 con la funzione vera del core (azione revoca_sorgente): la sorgente non vale più, S8 è di nuovo
	// ammessa, la working e la sua autorizzazione restano com'erano
	rr, impronta, err := sorgentev1.LeggiRichiestaRevoca([]byte(`{"chiave_idempotenza":"` + uuid.New().String() + `","prodotto_id":"` +
		s.prodotto.String() + `","motivo":"STEP scelto per errore"}`))
	if err != nil {
		t.Fatal(err)
	}
	if es, err := sorgentev1.Revoca(ctx, p, s.thread, s.aStep, s.utente, rr, impronta); err != nil || es.Esito != "revocata" {
		t.Fatalf("la revoca di S8: %+v %v", es, err)
	}
	var azione string
	if err := p.QueryRow(ctx, `SELECT azione FROM fonte_step_evento WHERE thread_id = $1 ORDER BY evento_id DESC LIMIT 1`, s.thread).Scan(&azione); err != nil ||
		azione != confermabom.AzioneRevocaSorgente {
		t.Fatalf("l'evento della revoca: %q %v", azione, err)
	}
	f = fonte()
	if f.SorgenteConfermata || !f.ConfermaAmmessa || f.SorgenteS8 == nil || f.SorgenteS8.Valida || f.SorgenteS8.Motivo != confermabom.SorgenteRevocata {
		t.Errorf("dopo la revoca di S8: %+v %+v", f, f.SorgenteS8)
	}
	if !f.Autorizzata || f.AutorizzataIl == nil || !f.AutorizzataIl.Equal(*autorizzataIl) || f.AutorizzataMotivo != "" {
		t.Errorf("la revoca di S8 ha toccato l'autorizzazione della working: %+v", f)
	}
	if n := legato(); n != 0 {
		t.Errorf("S9 dopo la revoca: %d righe legato_come_sorgente dello STEP, attese 0", n)
	}
}
