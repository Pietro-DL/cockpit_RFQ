package config

import (
	"strings"
	"testing"
	"time"
)

// L1 — [outlook]: la finestra iniziale e l'override `dal` (checkpoint del 16/09/2026, voce 2.8).
//
// Qui si prova solo che cosa entra dal file. Che cosa se ne fa chi accoda il sync — il cursore che
// vince, `dal` che vale solo per le cartelle che un cursore non ce l'hanno — è SI1–SI4, in
// `platform/coda`, dove la finestra si calcola davvero.

// Assente = il valore non arriva dal file, e la finestra la decide coda.GiorniSyncInizialeDefault.
// Il file NON porta un proprio 7 di riserva: due numeri in due posti sono due numeri che prima o poi
// si allontanano, e il momento in cui se ne accorge qualcuno è quando le due finestre non
// coincidono più.
func TestSenzaLaRigaLaFinestraInizialeLaDecideChiAccodaIlSync(t *testing.T) {
	c, err := Carica(scrivi(t, "[outlook]\ncartelle = [\"Inbox\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Outlook.GiorniSyncIniziale != 0 {
		t.Errorf("giorni_sync_iniziale = %d senza la riga nel file: atteso 0 («non dichiarato»)", c.Outlook.GiorniSyncIniziale)
	}
}

func TestLaFinestraInizialeSiScriveNelFile(t *testing.T) {
	c, err := Carica(scrivi(t, "[outlook]\ngiorni_sync_iniziale = 14\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Outlook.GiorniSyncIniziale != 14 {
		t.Errorf("giorni_sync_iniziale = %d, atteso 14", c.Outlook.GiorniSyncIniziale)
	}
}

// Un numero di giorni negativo è un errore di scrittura, non una finestra: presa alla lettera
// aprirebbe una finestra che comincia nel futuro, che è il modo in cui il sync smette di leggere
// senza dare errore (la stessa forma di guasto della correzione del fuso del 16/09).
func TestUnaFinestraInizialeNegativaFermaLAvvio(t *testing.T) {
	_, err := Carica(scrivi(t, "[outlook]\ngiorni_sync_iniziale = -3\n"))
	if err == nil {
		t.Fatal("il server è partito con una finestra iniziale negativa")
	}
	for _, atteso := range []string{"giorni_sync_iniziale", "-3"} {
		if !strings.Contains(err.Error(), atteso) {
			t.Errorf("l'errore non dice %q: %v", atteso, err)
		}
	}
}

// `dal` scritto male non deve passare in silenzio. Prima veniva scartato senza una riga da nessuna
// parte: chi credeva di stare importando settembre importava la finestra iniziale, e se ne accorgeva
// dalle mail che non c'erano.
func TestUnDalScrittoMaleFermaLAvvio(t *testing.T) {
	for _, riga := range []string{`dal = "01/09/2026"`, `dal = "2026-13-01"`, `dal = "settembre"`} {
		t.Run(riga, func(t *testing.T) {
			_, err := Carica(scrivi(t, "[outlook]\n"+riga+"\n"))
			if err == nil {
				t.Fatal("il server è partito con un `dal` che non sa leggere")
			}
			if !strings.Contains(err.Error(), "dal") || !strings.Contains(err.Error(), "AAAA-MM-GG") {
				t.Errorf("l'errore non dice come si scrive: %v", err)
			}
		})
	}
}

// Un `dal` valido si legge nel fuso del server, e lo legge una funzione sola: la validazione
// all'avvio e chi accoda il sync devono ottenere lo stesso istante, o sarebbero due finestre diverse
// a seconda di chi guarda.
func TestUnDalValidoSiLeggeNelFusoDelServer(t *testing.T) {
	c, err := Carica(scrivi(t, "[outlook]\ndal = \"  2026-09-01  \"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Outlook.Dal != "2026-09-01" {
		t.Errorf("dal = %q: gli spazi intorno alla data non sono un errore da respingere", c.Outlook.Dal)
	}
	d, err := DataDal(c.Outlook.Dal)
	if err != nil {
		t.Fatal(err)
	}
	atteso := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	if !d.Equal(atteso) {
		t.Errorf("dal = %v, atteso %v (mezzanotte locale)", d, atteso)
	}
}
