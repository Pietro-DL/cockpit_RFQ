//go:build integrazione

// L4 — Smistamento M3 sulla porta HTTP vera (A5.16.6): ogni decisione sulla posta lascia nel registro degli
// agganci la fotografia di ciò che l'operatore aveva davanti. Nomi, domini e codici inventati.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/aggancio"
	"promatec/cockpit/internal/core/inbox/classificazione"
)

// ultimoLog è l'ultima riga del registro degli agganci di un messaggio: azione, RFQ e fotografia.
func (b *bancoWeb) ultimoLog(m uuid.UUID) (azione string, thread uuid.NullUUID, utente bool, s classificazione.Scelta, motivo string) {
	b.t.Helper()
	var u uuid.NullUUID
	if err := b.pool.QueryRow(b.ctx, `SELECT azione, thread_id, utente_id, COALESCE(motivo, '') FROM messaggio_aggancio_log
		WHERE messaggio_id = $1 ORDER BY log_id DESC LIMIT 1`, m).Scan(&azione, &thread, &u, &motivo); err != nil {
		b.t.Fatalf("il registro non ha la decisione: %v", err)
	}
	s, ok := classificazione.LeggiScelta(motivo)
	if !ok {
		b.t.Fatalf("il motivo non è la fotografia della decisione: %q", motivo)
	}
	return azione, thread, u.Valid, s, motivo
}

// conCandidati fa arrivare una mail di ACME e le dà esattamente questi candidati (l'ingest li avrebbe
// calcolati dal messaggio; qui la lista deve essere nota).
func (b *bancoWeb) conCandidati(oggetto string, candidati ...classificazione.Candidato) uuid.UUID {
	b.t.Helper()
	m := b.posta("entrata", "buyer@acme.example", oggetto, true)
	if _, err := b.pool.Exec(b.ctx, `DELETE FROM candidato_aggancio WHERE messaggio_id = $1`, m); err != nil {
		b.t.Fatal(err)
	}
	for _, k := range candidati {
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato)
			VALUES ($1, $2, $3, $4, $5, 'APERTA')`, m, k.ThreadID, k.Regola, k.Punteggio, k.Evidenza); err != nil {
			b.t.Fatal(err)
		}
	}
	return m
}

// eventoSalvato è l'evento del messaggio come lo dice il triage (M2): dall'atto salvato.
func (b *bancoWeb) eventoSalvato(m uuid.UUID) (evento, atto string) {
	b.t.Helper()
	var controparte, direzione, legame string
	if err := b.pool.QueryRow(b.ctx, `SELECT m.controparte_tipo::text, m.direzione::text, COALESCE(p.atto, ''), COALESCE(p.legame::text, '')
		FROM messaggio m JOIN proposta_triage p USING (messaggio_id) WHERE m.messaggio_id = $1`, m).Scan(&controparte, &direzione, &atto, &legame); err != nil {
		b.t.Fatal(err)
	}
	return classificazione.EventoDa(controparte, direzione, atto, legame), atto
}

// 256 — la decisione registra il rango della RFQ scelta. Tre card (T1 molto forte, T2 medio-forte, T3
// debole) e quattro gesti: «Aggancia» alla seconda → rango 2 su 3, con il primo; «Crea RFQ» → nuova_rfq,
// rango 0; «Altra RFQ…» verso una RFQ che non era fra le card → fuori_lista, rango 0; «Ignora» → nessuna
// scelta. Il rango è la posizione della card nel pannello, calcolata dal server.
func TestLaDecisioneRegistraIlRangoDelCandidatoScelto(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	w := operatore(b)
	t1 := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 01 RFQ 7120001`, "7120001")
	t2 := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 02 RFQ 7120002`, "7120002")
	t3 := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 03 RFQ 7120003`, "7120003")
	t4 := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 04 RFQ 7120004`, "7120004")
	tre := func(oggetto string) uuid.UUID {
		return b.conCandidati(oggetto,
			classificazione.NuovoCandidato(t3.String(), classificazione.TipoR1Solo, "stessa conversazione", false),
			classificazione.NuovoCandidato(t2.String(), classificazione.TipoR3CodiceBuyer, "codice 7120002 dello stesso buyer", false),
			classificazione.NuovoCandidato(t1.String(), classificazione.TipoR0InReplyTo, "risponde a una mail di questa richiesta", false))
	}
	primo := classificazione.PrimoRFQ{Thread: t1.String(), Livello: "molto_forte", Score: 98, Tipi: []string{classificazione.TipoR0InReplyTo}}

	// «Aggancia» alla seconda card
	m1 := tre("RFQ 7120009 disegni")
	l, err := aggancio.InLettura(b.ctx, b.q, m1)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Candidati) != 3 || l.Candidati[1].ThreadID != t2.String() {
		t.Fatalf("la seconda card del pannello deve essere T2: %+v", l.Candidati)
	}
	evento, atto := b.eventoSalvato(m1)
	w.fai(http.MethodPost, "/messaggio/"+m1.String()+"/aggancia", url.Values{"thread_id": {t2.String()}}, true)
	azione, thread, utente, s, motivo := b.ultimoLog(m1)
	if azione != "aggancia" || !thread.Valid || thread.UUID != t2 || !utente {
		t.Errorf("la riga del registro: %s verso %v, utente %v", azione, thread, utente)
	}
	if s.Gesto != classificazione.GestoAggancia || s.Scelto == nil || s.Scelto.Rango != 2 || s.Scelto.Livello != "medio_forte" ||
		s.Scelto.Score != 72 || s.Su != 3 || s.FuoriLista || s.PariMerito {
		t.Errorf("aggancio alla seconda card: %s", motivo)
	}
	if s.Primo == nil || s.Primo.Thread != primo.Thread || s.Primo.Livello != primo.Livello || s.Primo.Score != primo.Score {
		t.Errorf("il primo proposto: %s", motivo)
	}
	if s.Evento != evento || s.Atto != atto || atto == "" || s.Regole != classificazione.VersioneRegoleAggancio || s.Frase != "decisione dell'operatore" {
		t.Errorf("evento %q e atto %q attesi (%q, %q): %s", evento, atto, s.Evento, s.Atto, motivo)
	}

	// «Crea RFQ» con tre candidati: nessuno era quello giusto
	m2 := tre("RFQ 7120010 disegni")
	w.fai(http.MethodPost, "/messaggio/"+m2.String()+"/rfq", url.Values{"cliente_id": {acme.String()}, "oggetto": {"RFQ 7120010"}, "priorita": {"1"}}, true)
	azione, thread, _, s, motivo = b.ultimoLog(m2)
	var nuova uuid.NullUUID
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, m2).Scan(&nuova); err != nil || !nuova.Valid {
		t.Fatalf("la RFQ nuova non è nata: %v", err)
	}
	if azione != "aggancia" || thread.UUID != nuova.UUID {
		t.Errorf("la RFQ nuova si registra come «aggancia» verso la RFQ nata: %s verso %v", azione, thread)
	}
	if s.Gesto != classificazione.GestoNuovaRFQ || s.Scelto == nil || s.Scelto.Rango != 0 || s.FuoriLista || s.Su != 3 || s.Primo == nil || s.Primo.Thread != t1.String() {
		t.Errorf("RFQ nuova con candidati: %s", motivo)
	}

	// «Altra RFQ…»: T4 non era fra le card
	m3 := tre("RFQ 7120011 disegni")
	w.fai(http.MethodPost, "/messaggio/"+m3.String()+"/aggancia", url.Values{"thread_id": {t4.String()}}, true)
	_, thread, _, s, motivo = b.ultimoLog(m3)
	if thread.UUID != t4 || s.Gesto != classificazione.GestoAggancia || s.Scelto == nil || s.Scelto.Rango != 0 || !s.FuoriLista || s.Scelto.Livello != "nessuno" {
		t.Errorf("fuori lista: %s", motivo)
	}

	// «Ignora»: nessuna RFQ scelta, il primo resta
	m4 := tre("RFQ 7120012 disegni")
	w.fai(http.MethodPost, "/messaggio/"+m4.String()+"/ignora", nil, true)
	azione, thread, _, s, motivo = b.ultimoLog(m4)
	if azione != "ignora" || thread.Valid || s.Gesto != classificazione.GestoIgnora || s.Scelto != nil || s.Su != 3 || s.Primo == nil || s.Primo.Thread != t1.String() {
		t.Errorf("ignora: %s verso %v, %s", azione, thread, motivo)
	}
	if !strings.Contains(motivo, `"scelto":null`) || s.Frase != "chiuso senza RFQ" {
		t.Errorf("ignora deve avere scelto null e la frase di sempre: %s", motivo)
	}
}

// 235 — il motivo del registro è un JSON (insieme alla 256): ogni riga che una decisione scrive è un JSON
// valido per il database (pg_input_is_valid), con la versione e la scala delle regole. Anche il giro degli
// orfani: la riga `candidato` porta la frase di sempre dentro il JSON, il gesto `candidato` (automatico,
// fuori dalle misure) e il rango del candidato fra quelli dell'orfano.
func TestIlMotivoDelLogDelleMailEUnJSON(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	w := operatore(b)
	t1 := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 01 RFQ 7120001`, "7120001")
	// due mail della stessa conversazione: si decide la prima, la seconda è un orfano che riceve il candidato
	primaMail := b.mail("entrata", "buyer@acme.example", "RFQ 7120001", "Buongiorno, ecco i disegni.")
	primaMail.ConversationID = "CONV-CAL-235"
	seconda := b.mail("entrata", "buyer@acme.example", "R: RFQ 7120001", "Ecco anche il resto.")
	seconda.ConversationID = "CONV-CAL-235"
	m1, m2 := b.postaMsg(primaMail), b.postaMsg(seconda)
	w.fai(http.MethodPost, "/messaggio/"+m1.String()+"/aggancia", url.Values{"thread_id": {t1.String()}}, true)

	azione, thread, _, s, motivo := b.ultimoLog(m2)
	if azione != "candidato" || thread.UUID != t1 {
		t.Fatalf("l'orfano della conversazione: %s verso %v", azione, thread)
	}
	if s.Gesto != classificazione.GestoCandidato || !strings.Contains(s.Frase, "orfano della stessa conversazione: candidato") ||
		!strings.Contains(motivo, " · score ") || s.Scelto == nil || s.Scelto.Rango < 1 || s.Su < 1 {
		t.Errorf("la riga del giro degli orfani: %s", motivo)
	}
	l, err := aggancio.InLettura(b.ctx, b.q, m2)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Candidati[s.Scelto.Rango-1].ThreadID; got != t1.String() {
		t.Errorf("il rango %d dell'orfano non è la posizione di T1 fra i suoi candidati (%s)", s.Scelto.Rango, got)
	}
	// una decisione in più, «Ignora», e poi il controllo del database su tutte le righe
	w.fai(http.MethodPost, "/messaggio/"+b.posta("entrata", "buyer@acme.example", "Newsletter", false).String()+"/ignora", nil, true)
	var righe, valide, versionate int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*),
		count(*) FILTER (WHERE pg_input_is_valid(motivo, 'jsonb')),
		count(*) FILTER (WHERE CASE WHEN pg_input_is_valid(motivo, 'jsonb')
			THEN motivo::jsonb->>'v' = '1' AND motivo::jsonb->>'regole' = 'aggancio-2' ELSE false END)
		FROM messaggio_aggancio_log`).Scan(&righe, &valide, &versionate); err != nil {
		t.Fatal(err)
	}
	if righe < 3 || valide != righe || versionate != righe {
		t.Errorf("righe del registro %d, JSON validi %d, con versione e regole %d: devono essere tutte", righe, valide, versionate)
	}
}
