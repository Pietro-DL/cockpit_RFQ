package ingest

import (
	"encoding/json"
	"testing"
	"time"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// L1 — blocco 4A: chi decide se gli allegati scendono da soli, e che cosa fa quando non lo capisce.
//
// I test L4 accanto a questo provano la strada intera (job accodato, preso in carico, lotto
// consegnato dentro il tentativo). Qui si guarda la sola decisione, perche' due dei suoi rami non si
// possono raggiungere dalla strada: un payload illeggibile non si riesce ad accodare — Accoda lo
// serializza lui — e «staging automatico spento» e' una configurazione, non un percorso.
func TestScendonoDaSoli(t *testing.T) {
	sync := func(modo string, al *time.Time) *db.Job {
		raw, err := json.Marshal(api.PayloadSyncOutlook{Modo: modo, Al: al})
		if err != nil {
			t.Fatal(err)
		}
		return &db.Job{Tipo: db.TipoJobSyncOutlook, Payload: raw}
	}
	quando := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	casi := []struct {
		nome   string
		s      *Servizio
		j      *db.Job
		atteso bool
	}{
		{"spento, qualunque modo", &Servizio{}, sync(api.ModoAggiornamento, nil), false},
		{"aggiornamento", &Servizio{StagingAutomatico: true}, sync(api.ModoAggiornamento, &quando), true},
		{"storico", &Servizio{StagingAutomatico: true}, sync(api.ModoStorico, &quando), false},
		{"storico, anche con bootstrap acceso", &Servizio{StagingAutomatico: true, StagingBootstrap: true}, sync(api.ModoStorico, &quando), false},
		{"bootstrap non dichiarato", &Servizio{StagingAutomatico: true}, sync(api.ModoBootstrap, &quando), false},
		{"bootstrap dichiarato", &Servizio{StagingAutomatico: true, StagingBootstrap: true}, sync(api.ModoBootstrap, &quando), true},
		{"payload vecchio senza modo, con al", &Servizio{StagingAutomatico: true}, sync("", &quando), false},
		{"payload vecchio senza modo, senza al", &Servizio{StagingAutomatico: true}, sync("", nil), true},
		{"rilettura di un elemento", &Servizio{StagingAutomatico: true}, &db.Job{Tipo: db.TipoJobRileggiElemento}, true},
		{"riprova di uno scarto (nessun job)", &Servizio{StagingAutomatico: true}, nil, true},
		// Un payload che non si riesce a leggere non e' un permesso: qui si decide se aprire Outlook e
		// scaricare file, e «non ho capito di che sync si tratta» deve valere no.
		{"payload illeggibile", &Servizio{StagingAutomatico: true},
			&db.Job{Tipo: db.TipoJobSyncOutlook, Payload: json.RawMessage(`{"modo":`)}, false},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ok, perche := c.s.scendonoDaSoli(c.j)
			if ok != c.atteso {
				t.Errorf("scendonoDaSoli = %v (%s), atteso %v", ok, perche, c.atteso)
			}
			if !ok && perche == "" {
				t.Error("un no senza motivo: la frase finisce nel log e serve a chi legge il log")
			}
		})
	}
}

// ModoEffettivo e' la scaletta di compatibilita' che leggono in tre: chi applica il risultato di un
// sync, chi decide lo staging automatico, e il worker in Python con `modo_effettivo()`.
func TestModoEffettivo(t *testing.T) {
	quando := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	casi := []struct {
		p      api.PayloadSyncOutlook
		atteso string
	}{
		{api.PayloadSyncOutlook{Modo: api.ModoStorico}, api.ModoStorico},
		{api.PayloadSyncOutlook{Modo: api.ModoBootstrap, Al: &quando}, api.ModoBootstrap},
		{api.PayloadSyncOutlook{Modo: api.ModoAggiornamento, Al: &quando}, api.ModoAggiornamento},
		// i due payload pre-blocco 3: l'unico segnale era il limite superiore
		{api.PayloadSyncOutlook{Al: &quando}, api.ModoStorico},
		{api.PayloadSyncOutlook{}, api.ModoAggiornamento},
	}
	for _, c := range casi {
		if got := c.p.ModoEffettivo(); got != c.atteso {
			t.Errorf("ModoEffettivo(modo=%q, al=%v) = %q, atteso %q", c.p.Modo, c.p.Al != nil, got, c.atteso)
		}
	}
}
