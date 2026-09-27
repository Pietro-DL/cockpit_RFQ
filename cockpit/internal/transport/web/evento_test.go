package web

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// L1 — Smistamento M2 sulla schermata (A5.16.4): il chip dell'atto grezzo diventa il chip dell'evento,
// nella lista e nel pannello; le evidenze dell'evento stanno a parte da quelle della proposta di
// aggancio, e l'evento non ha bottoni. Codici finti (7120001A).

func motiviJSON(t *testing.T, m ...string) *json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	r := json.RawMessage(b)
	return &r
}

func TestIlChipDellEventoNellaListaENelPannello(t *testing.T) {
	s := serverTest(t)
	ev := classificazione.EsitoEvento{Evento: classificazione.EventoRevisioneCAD, Atto: classificazione.AttoRevisioneDocumenti,
		Forza: classificazione.ForzaProbabile, Evidenze: []string{"7120001A_2.stp porta la rev 2"}}
	motivi := append(classificazione.MotiviEvento(ev), "In-Reply-To verso la richiesta")
	riga := db.VInbox{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("R: RFQ 7120001"),
		ControparteTipo: "cliente", TriageAtto: classificazione.AttoRevisioneDocumenti, TriageLegame: classificazione.LegameRisposta,
		TriageEsito: txtT("aggancia"), TriageConfidenza: pgtype.Int2{Int16: 98, Valid: true}, TriageMotivi: motiviJSON(t, motivi...),
		ThreadProposto: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
	newsletter := db.VInbox{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("Novità"),
		ControparteTipo: "cliente", TriageAtto: classificazione.AttoNonBusiness, TriageLegame: classificazione.LegameNessuno,
		TriageEsito: txtT("ignora"), TriageConfidenza: pgtype.Int2{Valid: true}}
	// una riga di prima del M2: l'atto c'è, le righe «evento ·» no
	diPrima := db.VInbox{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("Richiesta"),
		ControparteTipo: "cliente", TriageAtto: classificazione.AttoRichiestaOfferta, TriageLegame: classificazione.LegameNuovo,
		TriageEsito: txtT("nuova_rfq"), TriageConfidenza: pgtype.Int2{Int16: 70, Valid: true}, TriageMotivi: motiviJSON(t, "testo contiene «rfq»")}

	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "inbox_lista", vista{Dati: inboxDati{Filtro: "tutti", Righe: []db.VInbox{riga, newsletter, diPrima}}, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	lista := buf.String()
	for _, a := range []string{
		`class="chip atto revisione_documenti" data-evento="REVISIONE_CAD"`, ">Revisione CAD<", "7120001A_2.stp porta la rev 2", "legame: risposta",
		`class="chip atto non_business" data-evento="ALTRO"`, ">Altro · non business<",
		`data-evento="NUOVA_RFQ"`, ">Nuova RFQ<", "senza evidenze",
	} {
		if !strings.Contains(lista, a) {
			t.Errorf("lista: manca %q", a)
		}
	}
	// il chip grezzo non c'è più, e il chip del triage non si porta dietro le righe dell'evento
	if strings.Contains(lista, ">revisione_documenti<") || strings.Contains(lista, ">richiesta_offerta<") {
		t.Error("lista: c'è ancora il chip con l'atto grezzo")
	}
	if i := strings.Index(lista, `class="chip triage"`); i < 0 || strings.Contains(lista[i:i+300], "evento ·") {
		t.Errorf("lista: il chip del triage mescola l'evento alla proposta di aggancio")
	}

	// il pannello di un orfano: la riga «Evento», la forza, l'evidenza, e che l'evento non aggancia
	md, _, _ := datiSintetici()
	md.Thread, md.Allegati = nil, nil
	md.Riga = riga
	buf.Reset()
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	pannello := buf.String()
	for _, a := range []string{"<b>Evento:</b>", ">Revisione CAD<", "(probabile)", "— 7120001A_2.stp porta la rev 2", "L'evento non aggancia: la RFQ la scegli sotto."} {
		if !strings.Contains(pannello, a) {
			t.Errorf("pannello: manca %q", a)
		}
	}
	if i := strings.Index(pannello, "<b>Proposta:</b>"); i < 0 || !strings.Contains(pannello[i:], "In-Reply-To verso la richiesta") {
		t.Error("pannello: i motivi della proposta sono spariti")
	} else if j := strings.Index(pannello[i:], "</small>"); j < 0 || strings.Contains(pannello[i:i+j], "evento ·") {
		t.Error("pannello: le righe dell'evento stanno fra i motivi della proposta")
	}
	// l'evento non ha bottoni: nessun form né hx-post dentro la riga dell'evento
	if i := strings.Index(pannello, `<div class="evento">`); i >= 0 {
		riga := pannello[i:]
		riga = riga[:strings.Index(riga, "</div>")]
		if strings.Contains(riga, "<button") || strings.Contains(riga, "hx-post") || strings.Contains(riga, "thread_id") {
			t.Errorf("la riga dell'evento ha un gesto: %s", riga)
		}
	} else {
		t.Error("pannello: manca la riga dell'evento")
	}

	// un messaggio già in una RFQ: l'evento si vede, la frase sull'aggancio no
	md2, _, _ := datiSintetici()
	md2.Allegati = nil
	md2.Riga = riga
	buf.Reset()
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md2, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), ">Revisione CAD<") || strings.Contains(buf.String(), "L'evento non aggancia") {
		t.Error("pannello di un messaggio agganciato: l'evento si vede, l'invito a scegliere la RFQ no")
	}

	// la nostra mail a un cliente, che il triage non interpreta: l'evento calcolato adesso
	md3, _, _ := datiSintetici()
	md3.Thread, md3.Allegati = nil, nil
	md3.Riga = db.VInbox{MessaggioID: md3.M.MessaggioID, Direzione: db.DirezioneUscita, ControparteTipo: "cliente"}
	md3.EventoLettura = &vistaEvento{Codice: classificazione.EventoAltro, Etichetta: "Altro", Forza: classificazione.ForzaChiaro,
		Evidenze: []string{"nostra mail in uscita", "preparata dal Cockpit"}, Atto: classificazione.AttoOfferta, InLettura: true}
	buf.Reset()
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md3, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{">Altro · offerta<", "nostra mail in uscita; preparata dal Cockpit", "letto adesso dal messaggio"} {
		if !strings.Contains(buf.String(), a) {
			t.Errorf("pannello in lettura: manca %q", a)
		}
	}
	for nome, html := range map[string]string{"lista": lista, "pannello": pannello} {
		if m := rePercentuale.FindString(html); m != "" {
			t.Errorf("%s: una percentuale: %q", nome, m)
		}
	}
}

// eventoDellaRiga non legge evidenze di un altro evento: una riga i cui motivi parlano di un evento
// diverso da quello che l'atto salvato dice (una proposta ritoccata, un atto cambiato a mano) mostra
// l'evento dell'atto, senza evidenze.
func TestLEventoDellaRigaSiFidaDellAtto(t *testing.T) {
	r := db.VInbox{Direzione: db.DirezioneEntrata, ControparteTipo: "cliente", TriageAtto: classificazione.AttoSollecito,
		TriageMotivi: motiviJSON(t, "evento · REVISIONE_CAD (chiaro): 7120001A_2.stp porta la rev 2")}
	v := eventoDellaRiga(r)
	if v == nil || v.Codice != classificazione.EventoSollecito || len(v.Evidenze) != 0 || v.Forza != "" {
		t.Fatalf("%+v", v)
	}
	if eventoDellaRiga(db.VInbox{}) != nil {
		t.Error("senza atto non c'è un evento salvato da mostrare")
	}
	if got := motiviProposta(r.TriageMotivi); len(got) != 0 {
		t.Errorf("le righe dell'evento non sono motivi della proposta: %q", got)
	}
}
