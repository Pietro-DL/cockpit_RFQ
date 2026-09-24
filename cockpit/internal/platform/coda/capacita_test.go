package coda

import (
	"slices"
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// L1 — blocco 4: la tabella delle capacità, guardata da sola.
//
// I test L4 accanto a questo provano il giro intero (accodamento, claim, allineamento della coda).
// Qui si guarda la sola tabella, perché ha una proprietà che dal giro non si vede: che cosa succede a
// un tipo di job la cui capacità non esiste. È il caso di domani — qualcuno aggiunge un tipo nuovo,
// gli dà una capacità e dimentica di aggiungerla a Ha — e l'unico modo di sbagliarlo bene è che quel
// job resti fermo.

// Una capacità che non esiste non è mai concessa, nemmeno con tutto acceso.
func TestUnaCapacitaSconosciutaNonSiConcede(t *testing.T) {
	tutte := Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}
	for _, nome := range []string{"", "nas", "outlook", "scrittura", "NAS_SCRITTURA", "capacita_di_domani"} {
		if tutte.Ha(nome) {
			t.Errorf("la capacità %q risulta concessa: un nome che non riconosciamo deve valere NO", nome)
		}
	}
	for _, nome := range TutteLeCapacita {
		if !tutte.Ha(nome) {
			t.Errorf("la capacità %q non risulta concessa con tutto acceso", nome)
		}
	}
}

// Ogni tipo di job dell'enum deve chiedere una capacità CHE ESISTE, oppure nessuna. Un nome scritto
// male qui dentro non darebbe errore da nessuna parte: quel job semplicemente non partirebbe mai, e
// il motivo sarebbe invisibile.
func TestOgniTipoDiJobChiedeUnaCapacitaCheEsiste(t *testing.T) {
	for _, tipo := range db.AllTipoJobValues() {
		cap := CapacitaPer(tipo)
		if cap == "" {
			continue
		}
		if !slices.Contains(TutteLeCapacita, cap) {
			t.Errorf("%s chiede la capacità %q, che non è fra quelle dichiarate (%v)", tipo, cap, TutteLeCapacita)
		}
	}
}

// Le tre capacità governano cinque tipi di job, e quei cinque soltanto. L'elenco è scritto qui per
// esteso di proposito: se domani un tipo cambiasse capacità — o un tipo nuovo ne prendesse una senza
// che nessuno se ne accorgesse — questo test lo direbbe con il nome.
func TestQualeCapacitaGovernaQuale(t *testing.T) {
	atteso := map[db.TipoJob]string{
		db.TipoJobSegnaLetto:         CapOutlookScrittura,
		db.TipoJobSpostaInCartella:   CapOutlookScrittura,
		db.TipoJobCreaBozzaOutlook:   CapBozze,
		db.TipoJobCopiaNas:           CapNasScrittura,
		db.TipoJobCreaCartellaThread: CapNasScrittura,
		// A4 (0019): lo spostamento copia, promuove e toglie file sul NAS. In shadow non si accoda.
		db.TipoJobSpostaNas: CapNasScrittura,
	}
	for _, tipo := range db.AllTipoJobValues() {
		got, vuole := CapacitaPer(tipo), atteso[tipo]
		if got != vuole {
			t.Errorf("%s: capacità %q, attesa %q", tipo, got, vuole)
		}
	}
	// «Apri in Outlook» merita una riga sua: è l'unica azione Outlook che non modifica niente, ed è
	// quella che si vuole sempre disponibile — anche su un server che non deve toccare la posta.
	if CapacitaPer(db.TipoJobApriElementoOutlook) != "" {
		t.Error("«Apri in Outlook» risulta una scrittura: apre una finestra e non modifica niente")
	}
}

// Attive e Spente sono le due metà dello stesso elenco: insieme fanno le tre, e non si sovrappongono.
// Vanno nel log di avvio, e un log che dice due volte la stessa capacità non lo legge più nessuno.
func TestAttiveESpenteSonoLeDueMetaDellaStessaCosa(t *testing.T) {
	casi := []Capacita{
		{},
		{NasScrittura: true},
		{OutlookScrittura: true, Bozze: true},
		{OutlookScrittura: true, Bozze: true, NasScrittura: true},
	}
	for _, c := range casi {
		attive, spente := c.Attive(), c.Spente()
		if len(attive)+len(spente) != len(TutteLeCapacita) {
			t.Errorf("%+v: %d attive + %d spente ≠ %d", c, len(attive), len(spente), len(TutteLeCapacita))
		}
		for _, a := range attive {
			if slices.Contains(spente, a) {
				t.Errorf("%+v: %q compare fra le attive e fra le spente", c, a)
			}
		}
		if c.TuttoSpento() != (len(attive) == 0) {
			t.Errorf("%+v: TuttoSpento = %v con %d capacità attive", c, c.TuttoSpento(), len(attive))
		}
	}
}

// TipiBloccati è l'elenco che il claim esclude. Deve essere VUOTO e non nil quando non c'è niente da
// escludere: `tipo <> ALL (NULL)` non è vero per nessuna riga, e un claim che non assegna mai niente
// è il modo più silenzioso di fermare un sistema.
func TestTipiBloccatiEVuotoENonNil(t *testing.T) {
	tutte := Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}
	bloccati := TipiBloccati(tutte)
	if bloccati == nil {
		t.Fatal("con tutte le capacità accese l'elenco è nil invece che vuoto")
	}
	if len(bloccati) != 0 {
		t.Errorf("con tutte le capacità accese risultano bloccati: %v", bloccati)
	}
	soloNas := TipiBloccati(Capacita{NasScrittura: true})
	if slices.Contains(soloNas, string(db.TipoJobCopiaNas)) {
		t.Error("con nas_scrittura accesa copia_nas risulta ancora bloccato")
	}
	for _, t2 := range []db.TipoJob{db.TipoJobSegnaLetto, db.TipoJobCreaBozzaOutlook} {
		if !slices.Contains(soloNas, string(t2)) {
			t.Errorf("con la sola nas_scrittura accesa %s non risulta bloccato: %v", t2, soloNas)
		}
	}
	if slices.Contains(soloNas, string(db.TipoJobSyncOutlook)) {
		t.Error("il sync risulta bloccato: le letture non si spengono")
	}
}
