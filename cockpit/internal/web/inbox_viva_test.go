package web

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// L1 — SV2: dallo stato del worker e dallo stato del lavoro alla chip della testata, senza database.
//
// La regola che conta è una sola, ed è quella che un'implementazione distratta sbaglia: «in corso» si
// scrive solo dove il lavoro può davvero succedere. Un sync in coda per una casella il cui worker è
// spento non è un lavoro in corso, è un lavoro che non partirà; scriverlo lo stesso significa
// mostrare una rotella che gira su un PC dove non gira niente, e far aspettare l'operatore.
func TestSV2ChipDellaTestataConSyncENovita(t *testing.T) {
	fra, com, lui := uuid.New(), uuid.New(), uuid.New()
	ieri := time.Date(2026, 9, 15, 16, 41, 0, 0, time.UTC)
	base := []statoCasella{
		{ID: fra, Nome: "Francesco", Stato: "attiva", Dettaglio: "attiva su PC-FRANCESCO"},
		{ID: com, Nome: "Commerciale", Stato: "attiva", Dettaglio: "attiva su PC-FRANCESCO"},
		{ID: lui, Nome: "Luigi", Stato: "offline", Dettaglio: "worker su PC-LUIGI OFFLINE"},
	}
	lavoro := lavoroSync{
		Ultimo:  map[uuid.UUID]time.Time{fra: ieri},
		InCorso: map[uuid.UUID]bool{com: true, lui: true},
	}
	out := conSync(base, lavoro, map[uuid.UUID]int{fra: 3})
	per := map[string]statoCasella{}
	for _, c := range out {
		per[c.Nome] = c
	}

	if s := per["Francesco"]; s.Stato != "attiva" || s.Nuove != 3 || s.UltimoSync == nil || !s.UltimoSync.Equal(ieri) {
		t.Errorf("Francesco: stato=%s nuove=%d ultimo=%v; atteso attiva, 3 nuove, 16:41", s.Stato, s.Nuove, s.UltimoSync)
	}
	if s := per["Commerciale"]; s.Stato != "in_corso" || !strings.Contains(s.Dettaglio, "in corso") {
		t.Errorf("Commerciale: sync in coda su casella attiva → atteso in_corso, ottenuto %s (%q)", s.Stato, s.Dettaglio)
	}
	if s := per["Luigi"]; s.Stato != "offline" {
		t.Errorf("Luigi: worker spento e sync in coda → deve restare offline, non «in corso» (ottenuto %s)", s.Stato)
	}
	if per["Commerciale"].Classe != classiStato["in_corso"] || per["Luigi"].Classe != classiStato["offline"] {
		t.Errorf("le classi CSS non seguono lo stato: %q, %q", per["Commerciale"].Classe, per["Luigi"].Classe)
	}
	if s := per["Commerciale"]; s.UltimoSync != nil {
		t.Errorf("Commerciale non ha mai sincronizzato: l'ora dell'ultimo sync non deve esserci (%v)", s.UltimoSync)
	}
}

// La testata si ricarica spesso solo finché c'è qualcosa da vedere: un poll ogni 3 secondi per
// sempre sarebbe una richiesta ogni tre secondi per operatore per non dire niente.
func TestLaTestataStringeIlPassoSoloQuandoServe(t *testing.T) {
	fermo := &statoUI{Caselle: []statoCasella{{Stato: "attiva"}, {Stato: "offline"}}}
	if fermo.SyncInCorso() || fermo.AttesaTestata() != "15s" {
		t.Errorf("senza sync: inCorso=%v attesa=%s", fermo.SyncInCorso(), fermo.AttesaTestata())
	}
	lavora := &statoUI{Caselle: []statoCasella{{Stato: "attiva"}, {Stato: "in_corso"}}}
	if !lavora.SyncInCorso() || lavora.AttesaTestata() != "3s" {
		t.Errorf("con un sync in corso: inCorso=%v attesa=%s", lavora.SyncInCorso(), lavora.AttesaTestata())
	}
}

// SV1 lato parole: «Aggiorna ora» dice sempre che cosa è successo, anche quando non ha fatto niente.
// Un pulsante che non risponde è un pulsante che si preme di nuovo.
func TestFraseDiAggiornaOra(t *testing.T) {
	casi := []struct {
		accodati, gia int
		dentro        string
	}{
		{0, 0, "Nessuna casella"},
		{0, 2, "già in corso"},
		{3, 0, "richiesta per 3 caselle"},
		{1, 2, "2 erano già in corso"},
	}
	for _, c := range casi {
		if f := frase(c.accodati, c.gia); !strings.Contains(f, c.dentro) {
			t.Errorf("accodati=%d già=%d → %q, atteso contenesse %q", c.accodati, c.gia, f, c.dentro)
		}
	}
}

// Voce 9.5: in shadow la testata lo dice, e dice anche dove si cambia. Un badge che avvisa e basta
// lascia l'operatore a chiedersi se è rotto qualcosa.
func TestAvvisoShadowNominaCiòCheNonSuccedeEDoveSiCambia(t *testing.T) {
	s := &Server{Modalita: "shadow"}
	a := s.avvisoShadow()
	for _, parola := range []string{"SHADOW", "segna letto", "NAS", "Apri in Outlook", "modalita"} {
		if !strings.Contains(a, parola) {
			t.Errorf("l'avviso della shadow non nomina %q: %q", parola, a)
		}
	}
	if (&Server{Modalita: "produzione"}).avvisoShadow() != "" {
		t.Error("in produzione la testata non deve mostrare nessun avviso di shadow")
	}
}
