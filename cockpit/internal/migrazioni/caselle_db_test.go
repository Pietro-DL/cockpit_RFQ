//go:build integrazione

// S2 per la 0004 — la prima migrazione che SPOSTA RIGHE, provata su un dump della versione 3 CON DATI.
//
// È la differenza che conta: una migrazione di sole colonne, su un database vuoto, riesce sempre. Qui
// il travaso `messaggio_outlook` → `messaggio_casella` e il rimappaggio dei cursori devono conservare
// esattamente ciò che c'era, e ciò che c'era sono gli EntryID con cui si riaprono gli elementi in
// Outlook: perderli non darebbe nessun errore oggi, e renderebbe impossibile domani aprire un
// messaggio o scaricarne un allegato — con settimane di distanza fra la causa e l'effetto.
package migrazioni_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/migrazioni"
	"promatec/cockpit/internal/testutil"
)

func TestS2CaselleEPresenzeSuDBConDati(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaFinoA(t, p, 3)
	ctx := context.Background()

	if _, err := p.Exec(ctx, `
		INSERT INTO casella (canale, indirizzo, nome, condivisa) VALUES ('outlook','commerciale@azienda.it','Commerciale',true);
		INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook','CONV-S2', now());
		INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto)
		SELECT 'outlook', '<s2-' || g || '@acme.example>', conversazione_id, 'entrata',
		       timestamptz '2026-09-01 08:00:00Z' + (g || ' hours')::interval, 'RFQ S2 ' || g
		  FROM conversazione, generate_series(1,3) g;
		INSERT INTO messaggio_outlook (messaggio_id, entry_id, store_id, cartella, non_letto, flag_stato, categorie)
		SELECT messaggio_id, 'ENTRY-' || chiave_esterna, 'STORE-1', 'Posta in arrivo', true, 2, ARRAY['RFQ']
		  FROM messaggio;
		INSERT INTO sync_cursore (cartella, ultimo_received, storico_fino_a, n_messaggi)
		VALUES ('Posta in arrivo', timestamptz '2026-09-03 08:00:00Z', timestamptz '2026-08-01 00:00:00Z', 3),
		       ('Posta inviata',   timestamptz '2026-09-02 08:00:00Z', NULL, 1);`); err != nil {
		t.Fatal(err)
	}
	primaMessaggi := testutil.Conta(t, p, "messaggio")
	primaOutlook := testutil.Conta(t, p, "messaggio_outlook")
	primaCursori := testutil.Conta(t, p, "sync_cursore")

	if _, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso()); err != nil {
		t.Fatalf("0004 su un DB con dati: %v", err)
	}

	// i conteggi non cambiano: la migrazione sposta, non perde e non inventa
	if n := testutil.Conta(t, p, "messaggio"); n != primaMessaggi {
		t.Errorf("messaggi %d → %d", primaMessaggi, n)
	}
	if n := testutil.Conta(t, p, "messaggio_outlook"); n != primaOutlook {
		t.Errorf("messaggio_outlook %d → %d", primaOutlook, n)
	}
	if n := testutil.Conta(t, p, "messaggio_casella"); n != primaOutlook {
		t.Fatalf("presenze = %d, attese %d: una per ogni messaggio Outlook che c'era", n, primaOutlook)
	}
	if n := testutil.Conta(t, p, "sync_cursore"); n != primaCursori {
		t.Errorf("cursori %d → %d", primaCursori, n)
	}

	// ogni presenza porta ciò che stava in messaggio_outlook, e la casella è l'unica attiva
	righe, err := p.Query(ctx, `SELECT mc.entry_id, mc.cartella, mc.non_letto, mc.flag_stato, mc.categorie,
	       mc.ricevuto_il, m.data_evento, c.indirizzo
	  FROM messaggio_casella mc JOIN messaggio m USING (messaggio_id) JOIN casella c USING (casella_id)
	 ORDER BY m.chiave_esterna`)
	if err != nil {
		t.Fatal(err)
	}
	defer righe.Close()
	n := 0
	for righe.Next() {
		var entry, cartella, indirizzo string
		var nonLetto bool
		var flag int16
		var categorie []string
		var ricevuto, evento time.Time
		if err := righe.Scan(&entry, &cartella, &nonLetto, &flag, &categorie, &ricevuto, &evento, &indirizzo); err != nil {
			t.Fatal(err)
		}
		n++
		if !strings.HasPrefix(entry, "ENTRY-<s2-") {
			t.Errorf("entry_id travasato male: %q", entry)
		}
		if cartella != "Posta in arrivo" || !nonLetto || flag != 2 || len(categorie) != 1 {
			t.Errorf("metadati della copia persi: cartella=%q non_letto=%v flag=%d categorie=%v", cartella, nonLetto, flag, categorie)
		}
		if !ricevuto.Equal(evento) {
			t.Errorf("ricevuto_il = %v, atteso data_evento %v (alla 0003 è l'unico valore disponibile)", ricevuto, evento)
		}
		if indirizzo != "commerciale@azienda.it" {
			t.Errorf("presenza assegnata a %q", indirizzo)
		}
	}
	if n != primaOutlook {
		t.Errorf("righe lette = %d, attese %d", n, primaOutlook)
	}

	// i cursori sono rimappati sulla stessa casella, con i valori intatti
	var senzaCasella int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM sync_cursore WHERE casella_id IS NULL`).Scan(&senzaCasella); err != nil {
		t.Fatal(err)
	}
	if senzaCasella != 0 {
		t.Errorf("%d cursori senza casella dopo la migrazione", senzaCasella)
	}
	var cartelle []string
	if err := p.QueryRow(ctx, `SELECT array_agg(sc.cartella ORDER BY sc.cartella) FROM sync_cursore sc
		JOIN casella c USING (casella_id) WHERE c.indirizzo = 'commerciale@azienda.it'`).Scan(&cartelle); err != nil {
		t.Fatal(err)
	}
	if len(cartelle) != 2 || cartelle[0] != "Posta in arrivo" || cartelle[1] != "Posta inviata" {
		t.Errorf("cursori rimappati = %v", cartelle)
	}
	var ultimo, storico *time.Time
	if err := p.QueryRow(ctx, `SELECT ultimo_received, storico_fino_a FROM sync_cursore WHERE cartella = 'Posta in arrivo'`).Scan(&ultimo, &storico); err != nil {
		t.Fatal(err)
	}
	if ultimo == nil || !ultimo.Equal(time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)) || storico == nil {
		t.Errorf("valori del cursore alterati: ultimo=%v storico=%v", ultimo, storico)
	}

	// la chiave è ora (casella, cartella): due caselle possono avere la stessa cartella
	var altra uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO casella (canale, indirizzo, nome) VALUES ('outlook','francesco@azienda.it','Francesco')
		RETURNING casella_id`).Scan(&altra); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO sync_cursore (casella_id, cartella) VALUES ($1, 'Posta in arrivo')`, altra); err != nil {
		t.Errorf("due caselle non possono avere la stessa cartella: la chiave non è (casella, cartella): %v", err)
	}

	// e le colonne per copia non sono rimaste anche in messaggio_outlook: due verità sull'EntryID
	// sarebbero peggio di nessuna, perché quella sbagliata si scopre solo quando un'azione fallisce
	var residue int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'messaggio_outlook' AND column_name IN ('entry_id','store_id','cartella','non_letto','flag_stato','categorie')`).Scan(&residue); err != nil {
		t.Fatal(err)
	}
	if residue != 0 {
		t.Errorf("%d colonne per copia sono rimaste in messaggio_outlook", residue)
	}
}

// Se non si può dire con certezza di CHI sono i messaggi già in archivio, la 0004 si ferma. Assegnarli
// alla casella sbagliata sarebbe un EntryID che non apre niente, e non lo scoprirebbe nessuno fino al
// primo clic su «Apri in Outlook»: la migrazione preferisce non partire, e dire che cosa fare.
func TestS2CaselleAmbigueFermanoLaMigrazione(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaFinoA(t, p, 3)
	ctx := context.Background()

	if _, err := p.Exec(ctx, `
		INSERT INTO casella (canale, indirizzo, nome) VALUES ('outlook','uno@azienda.it','Uno'), ('outlook','due@azienda.it','Due');
		INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook','CONV-AMB', now());
		INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento)
		SELECT 'outlook','<amb@acme.example>', conversazione_id, 'entrata', now() FROM conversazione;
		INSERT INTO messaggio_outlook (messaggio_id, entry_id, store_id) SELECT messaggio_id, 'E', 'S' FROM messaggio;`); err != nil {
		t.Fatal(err)
	}

	_, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
	if err == nil {
		t.Fatal("la 0004 ha assegnato i messaggi a una casella scelta a caso")
	}
	if !strings.Contains(err.Error(), "caselle attive") {
		t.Errorf("il messaggio non dice che cosa fare: %v", err)
	}
	// e non ha lasciato niente a metà: la transazione della migrazione è annullata (S3)
	fatte, err := migrazioni.Applicate(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if fatte[4] {
		t.Error("la 0004 risulta applicata dopo essere fallita")
	}
	var esiste bool
	if err := p.QueryRow(ctx, "SELECT to_regclass('public.messaggio_casella') IS NOT NULL").Scan(&esiste); err != nil {
		t.Fatal(err)
	}
	if esiste {
		t.Error("messaggio_casella è rimasta dopo una migrazione fallita")
	}
}
