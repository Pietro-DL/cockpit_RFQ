//go:build integrazione

// L4 — V1, ondata C: S9 (emendamento U2 §2.3; prove R-36…R-40, la parte L4 della corsia motore) sulla scena ACME di
// servizio_db_test.go (scenaMA), sul database di prova (COCKPIT_TEST_DSN):
//   - lo STEP autorizzato fra gli STEP con la candidata del prodotto (gruppo bom, impronta_contenuto di 64 esadecimali,
//     i nodi segnalati come in /dati); la sezione del prodotto con il codice, la fonte confermata e autorizzata con la
//     data e l'autore della conferma, lo stato del calcolo;
//   - il PDF del figlio: una riga con il riferimento stabile (mai il solo codice) e, senza bozza, nessuna azione sulla
//     bozza disponibile, ognuna con il motivo; i conti uguali alle righe;
//   - il PDF in «Altri file»: fuori dalle righe e dai file da associare, dentro altri_file, e la firma cambia;
//   - un file senza i fatti: mai fra i file da associare («non ancora pronto» contro «nessun abbinamento»);
//   - S9 non scrive niente.
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx), come nella scena.
package motore_a

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/platform/testutil"
)

func TestL4S9LaSorgenteDellaScena(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	prima := testutil.FotoDelDatabase(t, p)
	so, err := svc.Sorgente(ctx, s.thread)
	if err != nil {
		t.Fatalf("Sorgente: %v", err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("S9 ha cambiato il database: %v", d)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(so.Firma) || len(so.Prodotti) != 1 {
		t.Fatalf("S9: firma %q, prodotti %d", so.Firma, len(so.Prodotti))
	}
	var bom *CandidataDelloStep
	for _, st := range so.Step {
		for i := range st.Candidate {
			if st.AllegatoID == s.aStep && st.Candidate[i].Gruppo == propostabom.GruppoBOM {
				bom = &st.Candidate[i]
			}
		}
	}
	if bom == nil || bom.ProdottoID == nil || *bom.ProdottoID != s.prodotto || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(bom.ImprontaContenuto) ||
		bom.RadiceLetterale != "9120100A" || bom.SorgenteDi != nil {
		t.Errorf("lo STEP della scena fra gli STEP di S9: %+v (step %+v)", bom, so.Step)
	}
	pr := so.Prodotti[0]
	t.Logf("S9 L4: prodotto %s conti %+v calcolo %+v righe %+v in attesa %+v da associare %+v", pr.Codice, pr.Conti, pr.Calcolo, pr.Righe,
		pr.InAttesaSorgente, so.DaAssociare)
	if pr.ProdottoID == nil || *pr.ProdottoID != s.prodotto || pr.Codice != "9120100" || pr.Bozza != nil ||
		!pr.Fonte.Confermata || !pr.Fonte.Autorizzata || pr.Fonte.AllegatoID == nil || *pr.Fonte.AllegatoID != s.aStep ||
		pr.Fonte.ConfermataIl == nil || pr.Fonte.ConfermataDa == nil || pr.Fonte.AutorizzataMotivo != AutorizzazioneNonRegistrata {
		t.Errorf("la sezione del prodotto: %+v", pr)
	}
	if pr.Calcolo.Stato != CalcoloPronto || len(pr.Calcolo.FileInAnalisi) != 0 {
		t.Errorf("il calcolo della scena: %+v", pr.Calcolo)
	}
	righePdf, proposti, registrati, conflitti := 0, 0, 0, 0
	for _, riga := range pr.Righe {
		switch riga.Stato {
		case StatoRigaProposto:
			proposti++
		case StatoRigaRegistrato:
			registrati++
		case StatoRigaInConflitto:
			conflitti++
		}
		if riga.AllegatoID != s.aPdf {
			continue
		}
		righePdf++
		if riga.Destinazione.Voce == "" && riga.Destinazione.ComponenteID == nil {
			t.Errorf("la riga del PDF senza riferimento stabile: %+v", riga)
		}
		for _, x := range []AzioneSorgente{riga.Azioni.SalvaNellaBozza, riga.Azioni.TogliDallaBozza, riga.Azioni.Sposta} {
			if x.Disponibile || x.Motivo == "" {
				t.Errorf("senza bozza un'azione sulla bozza: %+v", riga.Azioni)
			}
		}
	}
	inAttesa := 0
	for _, f := range pr.InAttesaSorgente {
		if f.AllegatoID == s.aPdf {
			inAttesa++
		}
	}
	if righePdf+inAttesa == 0 {
		t.Errorf("il PDF del figlio non è né fra le righe né in attesa della sorgente: %+v", pr)
	}
	if pr.Conti.Proposti != proposti || pr.Conti.Registrati != registrati || pr.Conti.InConflitto != conflitti || pr.Conti.Salvati != 0 {
		t.Errorf("i conti %+v non sono le righe (%d, %d, %d)", pr.Conti, proposti, registrati, conflitti)
	}

	// il PDF in Altri file
	inTransazione(t, p,
		passoSQL{`INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, bytes, percorso_relativo, radice_id)
			SELECT $1, a.allegato_id, a.sha256, 10, 'ALTRO\' || a.allegato_id || '.pdf', gen_random_uuid() FROM allegato a WHERE a.allegato_id = $2`, []any{s.thread, s.aPdf}},
		passoSQL{`UPDATE file_rfq_conservazione SET stato = 'copiata', sorgente_byte = 'staging' WHERE thread_id = $1 AND allegato_id = $2 AND stato = 'in_copia'`,
			[]any{s.thread, s.aPdf}},
		passoSQL{`INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
			estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
		  SELECT m.thread_id, a.allegato_id, 'altri_file', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno,
			a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, $2, $2
		    FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{s.aPdf, s.utente}},
		passoSQL{`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, dipendenze, origine, autore_id)
			VALUES ($1, $2, 'altri_file', 'attivo', 'altri_file', '{}', '{}', 'ponte', $3)`, []any{s.thread, s.aPdf, s.utente}})
	dopo, err := svc.Sorgente(ctx, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	if dopo.Firma == so.Firma || len(dopo.AltriFile) != 1 || dopo.AltriFile[0].AllegatoID != s.aPdf {
		t.Errorf("il PDF in Altri file: firma uguale %v, altri file %+v", dopo.Firma == so.Firma, dopo.AltriFile)
	}
	for _, riga := range dopo.Prodotti[0].Righe {
		if riga.AllegatoID == s.aPdf {
			t.Errorf("il PDF in Altri file sta ancora fra le righe: %+v", riga)
		}
	}
	for _, d := range dopo.DaAssociare {
		if d.AllegatoID == s.aPdf {
			t.Errorf("il PDF in Altri file sta fra i file da associare: %+v", d)
		}
	}

	// un file nuovo senza i fatti (analisi non ancora fatta): «non ancora pronto», mai fra i file da associare
	var nuovo uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
		VALUES ($1, 4, '9120101A_3.pdf', 'pdf', 'file', 'outlook', 100, $2, 'analizzato', now()) RETURNING allegato_id`, s.messaggio,
		strings.Repeat("a4", 32)).Scan(&nuovo); err != nil {
		t.Fatal(err)
	}
	ancora, err := svc.Sorgente(ctx, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("S9 L4 con un file senza fatti: calcolo %+v, da associare %+v", ancora.Prodotti[0].Calcolo, ancora.DaAssociare)
	for _, d := range ancora.DaAssociare {
		if d.AllegatoID == nuovo {
			t.Errorf("un file senza fatti sta fra i file da associare: %+v", d)
		}
	}
}
