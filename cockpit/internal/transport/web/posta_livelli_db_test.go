//go:build integrazione

// L4 — Smistamento M1 sulla porta HTTP vera (A5.16.4, A5.16.5, A5.10): la mail preparata dal Cockpit
// dice da quale RFQ nasce senza scrivere niente alla GET, le righe delle regole di prima si leggono con
// prudenza e si ricalcolano con un POST, il pannello raggruppa per RFQ. Nomi e domini inventati.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/platform/testutil"
)

// conteggiPosta sono le righe che una GET della posta non deve toccare: candidati, registro degli
// agganci, proposte, messaggi agganciati.
func (b *bancoWeb) conteggiPosta() [4]int {
	b.t.Helper()
	var n [4]int
	for i, sql := range []string{`SELECT count(*) FROM candidato_aggancio`, `SELECT count(*) FROM messaggio_aggancio_log`,
		`SELECT count(*) FROM proposta_triage`, `SELECT count(*) FROM messaggio WHERE thread_id IS NOT NULL`} {
		if err := b.pool.QueryRow(b.ctx, sql).Scan(&n[i]); err != nil {
			b.t.Fatal(err)
		}
	}
	return n
}

func (b *bancoWeb) chiaveDi(id uuid.UUID) string {
	b.t.Helper()
	var k string
	if err := b.pool.QueryRow(b.ctx, `SELECT chiave_esterna FROM messaggio WHERE messaggio_id = $1`, id).Scan(&k); err != nil {
		b.t.Fatal(err)
	}
	return k
}

func (b *bancoWeb) bozzaDi(thread uuid.NullUUID, inRispostaA uuid.UUID, sigla string) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO bozza (thread_id, in_risposta_a, tipo, creata_da)
		VALUES ($1, $2, 'risposta', (SELECT utente_id FROM utente WHERE sigla = $3)) RETURNING bozza_id`, thread, inRispostaA, sigla).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	return id
}

// inviataDalCockpit fa tornare dalla Posta inviata la nostra mail di quella bozza, al buyer di ACME.
func (b *bancoWeb) inviataDalCockpit(bozza uuid.UUID, oggetto string) uuid.UUID {
	b.t.Helper()
	m := b.mail("uscita", "commerciale@azienda.example", oggetto, "Ecco la nostra offerta.", "buyer@acme.example")
	m.Marcatori = map[string]string{ingest.MarcatoreBozza: bozza.String()}
	return b.postaMsg(m)
}

// 252 — la mail inviata dal Cockpit dice da quale RFQ nasce (D84). Il pannello mostra il box e la RFQ come
// candidato molto forte con il suo bottone; la lista ha il chip «dal Cockpit». Le GET non scrivono niente
// e la mail non si aggancia da sola. Varianti: la bozza nata su un orfano agganciato dopo (l'origine è la
// RFQ di adesso della mail a cui rispondeva) e due origini discordi (si mostrano tutte e due, nessuna è
// proposta).
func TestLaMailInviataDalCockpitDiceDaQualeRFQNasce(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	w := operatore(b)

	// la bozza per la RFQ T in risposta alla mail M (che sta in T)
	m := b.posta("entrata", "buyer@acme.example", "RFQ 7120001", false)
	thread := b.rfqDa(m, acme, `ACME\WIP\2026 09 12 Rossi RFQ 7120001`, "7120001")
	s := b.inviataDalCockpit(b.bozzaDi(uuid.NullUUID{UUID: thread, Valid: true}, m, "FP"), "R: RFQ 7120001")

	prima := b.conteggiPosta()
	_, pannello := w.fai(http.MethodGet, "/messaggio/"+s.String(), nil, true)
	_, lista := w.fai(http.MethodGet, "/inbox?q=tutti&filtro=tutti", nil, true)
	if dopo := b.conteggiPosta(); dopo != prima {
		t.Errorf("le GET hanno scritto: candidati, log, proposte, agganciati %v → %v", prima, dopo)
	}
	for _, atteso := range []string{"Preparata dal Cockpit da FP", "per la RFQ «RFQ 7120001»", "La mail non si aggancia da sola",
		"molto forte · score 97", "Aggancia a questa RFQ", `value="` + thread.String() + `"`, "nostra mail preparata dal Cockpit da FP"} {
		if !strings.Contains(pannello, atteso) {
			t.Errorf("il pannello della mail inviata non dice %q", atteso)
		}
	}
	if !strings.Contains(lista, "dal Cockpit · RFQ 7120001") {
		t.Error("la lista non ha il chip «dal Cockpit · RFQ …»")
	}
	var orfana bool
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id IS NULL FROM messaggio WHERE messaggio_id = $1`, s).Scan(&orfana); err != nil || !orfana {
		t.Fatalf("la mail preparata dal Cockpit si è agganciata da sola (%v)", err)
	}
	// il clic di una persona la aggancia
	w.fai(http.MethodPost, "/messaggio/"+s.String()+"/aggancia", url.Values{"thread_id": {thread.String()}}, true)
	var tid uuid.NullUUID
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, s).Scan(&tid); err != nil || !tid.Valid || tid.UUID != thread {
		t.Errorf("dopo «Aggancia a questa RFQ»: %v (%v)", tid, err)
	}
	var azione string
	if err := b.pool.QueryRow(b.ctx, `SELECT azione FROM messaggio_aggancio_log WHERE messaggio_id = $1`, s).Scan(&azione); err != nil || azione != "aggancia" {
		t.Errorf("la decisione non è nel registro: %q (%v)", azione, err)
	}
	// Smistamento M3 (A5.16.6): la fotografia dice che la RFQ scelta era la prima card, quella del marcatore
	if _, _, _, f, motivo := b.ultimoLog(s); f.Scelto == nil || f.Scelto.Rango != 1 || len(f.Scelto.Tipi) == 0 ||
		f.Scelto.Tipi[0] != classificazione.TipoMarcatore || f.Gesto != classificazione.GestoAggancia {
		t.Errorf("la fotografia dell'aggancio alla card del marcatore: %s", motivo)
	}

	// variante: la bozza nacque su un orfano (thread NULL), e l'orfano è stato agganciato DOPO
	m2 := b.posta("entrata", "buyer@acme.example", "RFQ 7120002", false)
	bozza2 := b.bozzaDi(uuid.NullUUID{}, m2, "FP")
	thread2 := b.rfqDa(m2, acme, `ACME\WIP\2026 09 13 Rossi RFQ 7120002`, "7120002")
	s2 := b.inviataDalCockpit(bozza2, "R: RFQ 7120002")
	_, pannello = w.fai(http.MethodGet, "/messaggio/"+s2.String(), nil, true)
	if !strings.Contains(pannello, `value="`+thread2.String()+`"`) || !strings.Contains(pannello, "oggi sta in questa richiesta") {
		t.Error("la bozza nata su un orfano agganciato dopo: l'origine deve essere la RFQ di adesso della mail a cui rispondeva")
	}

	// variante: due origini discordi. La bozza è per la RFQ 7120001, la mail a cui rispondeva oggi sta
	// nella 7120003: si mostrano tutte e due, nessuna è proposta
	m3 := b.posta("entrata", "buyer@acme.example", "RFQ 7120003", false)
	thread3 := b.rfqDa(m3, acme, `ACME\WIP\2026 09 14 Rossi RFQ 7120003`, "7120003")
	s3 := b.inviataDalCockpit(b.bozzaDi(uuid.NullUUID{UUID: thread, Valid: true}, m3, "FP"), "R: RFQ 7120003")
	_, pannello = w.fai(http.MethodGet, "/messaggio/"+s3.String(), nil, true)
	if !strings.Contains(pannello, `value="`+thread.String()+`"`) || !strings.Contains(pannello, `value="`+thread3.String()+`"`) {
		t.Error("due origini discordi: le due RFQ devono vedersi tutte e due")
	}
	if !strings.Contains(pannello, "le due origini non coincidono") || !strings.Contains(pannello, "nessuna RFQ è proposta") || strings.Contains(pannello, " primo\"") {
		t.Error("due origini discordi: nessuna deve essere proposta né marcata come prima")
	}
}

// 260 — le righe scritte con le regole di prima (R1 a 95, R2 a 55) si leggono con prudenza: «debole» e
// «molto debole», con l'avviso e il bottone «Ricalcola». La GET non scrive; il POST ricalcola il solo
// messaggio, e dopo non restano punteggi di prima.
func TestLeRigheVecchieSiLeggonoConPrudenza(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	w := operatore(b)
	thread := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 12 Rossi RFQ 7120001`, "7120001")
	m := b.posta("entrata", "buyer@acme.example", "RFQ 7120001", false)
	if _, err := b.pool.Exec(b.ctx, `DELETE FROM candidato_aggancio WHERE messaggio_id = $1`, m); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato) VALUES
		($1, $2, 'R1_conversazione', 95, 'la conversazione di Outlook è stata collegata a questa richiesta da un operatore', 'APERTA'),
		($1, $2, 'R2_oggetto', 55, 'stesso oggetto di questa richiesta, entro i giorni della finestra', 'APERTA')`, m, thread); err != nil {
		t.Fatal(err)
	}
	prima := b.conteggiPosta()
	_, pannello := w.fai(http.MethodGet, "/messaggio/"+m.String(), nil, true)
	if dopo := b.conteggiPosta(); dopo != prima {
		t.Errorf("la GET ha scritto: %v → %v", prima, dopo)
	}
	for _, atteso := range []string{"debole · score 40", "(molto debole)", "calcolati con le regole di prima", "/ricalcola"} {
		if !strings.Contains(pannello, atteso) {
			t.Errorf("il pannello non dice %q", atteso)
		}
	}
	for _, vietato := range []string{"score 95", "score 55", "95%", "55%"} {
		if strings.Contains(pannello, vietato) {
			t.Errorf("la riga di prima si legge con il punteggio di prima: %q", vietato)
		}
	}
	// «Ricalcola»: il solo messaggio, con le regole di adesso
	_, html := w.fai(http.MethodPost, "/messaggio/"+m.String()+"/ricalcola", nil, true)
	if !strings.Contains(html, "Candidati ricalcolati") {
		t.Errorf("la risposta di «Ricalcola»: %s", estratto(html, "avviso"))
	}
	var vecchie, nuove int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FILTER (WHERE punteggio IN (95, 55)), count(*) FILTER (WHERE regola = 'R2_oggetto' AND punteggio = 20)
		FROM candidato_aggancio WHERE messaggio_id = $1`, m).Scan(&vecchie, &nuove); err != nil {
		t.Fatal(err)
	}
	if vecchie != 0 || nuove != 1 {
		t.Errorf("dopo il ricalcolo: %d righe di prima, %d R2 a 20", vecchie, nuove)
	}
	_, pannello = w.fai(http.MethodGet, "/messaggio/"+m.String(), nil, true)
	if strings.Contains(pannello, "regole di prima") {
		t.Error("dopo il ricalcolo il pannello parla ancora delle regole di prima")
	}
	// un messaggio già deciso non si ricalcola
	agganciato := b.posta("entrata", "buyer@acme.example", "RFQ 7120001", false)
	b.rfqDa(agganciato, acme, `ACME\WIP\2026 09 15 Rossi RFQ 7120009`, "7120009")
	_, html = w.fai(http.MethodPost, "/messaggio/"+agganciato.String()+"/ricalcola", nil, true)
	if !strings.Contains(html, "già stato deciso") {
		t.Errorf("«Ricalcola» su un messaggio deciso: %s", estratto(html, "avviso"))
	}
}

// 263 — il pannello raggruppa per RFQ: R0 e R2 sulla stessa RFQ sono UNA card, «molto forte», con due
// evidenze e un bottone `thread_id`; nessun radio, niente di spuntato.
func TestIlPannelloRaggruppaPerRFQ(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	w := operatore(b)
	m := b.posta("entrata", "buyer@acme.example", "RFQ 7120001", false)
	thread := b.rfqDa(m, acme, `ACME\WIP\2026 09 12 Rossi RFQ 7120001`, "7120001")
	r := b.mail("entrata", "buyer@acme.example", "R: RFQ 7120001", "Ecco la revisione.")
	r.InReplyTo, r.Riferimenti = b.chiaveDi(m), []string{b.chiaveDi(m)}
	risposta := b.postaMsg(r)
	if n := testutil.Conta(t, b.pool, "candidato_aggancio"); n < 2 {
		t.Fatalf("la scena vuole R0 e R2 sulla stessa RFQ: %d candidati", n)
	}
	_, pannello := w.fai(http.MethodGet, "/messaggio/"+risposta.String(), nil, true)
	if c := strings.Count(pannello, `value="`+thread.String()+`"`); c != 1 {
		t.Errorf("la RFQ compare in %d card, attesa una", c)
	}
	for _, atteso := range []string{"molto forte · score 98", "In-Reply-To punta alla mail", "stesso oggetto di questa richiesta", `name="thread_id"`, "(molto debole)"} {
		if !strings.Contains(pannello, atteso) {
			t.Errorf("la card non dice %q", atteso)
		}
	}
	i := strings.Index(pannello, `class="candidati-aggancio"`)
	if i < 0 {
		t.Fatal("manca la lista dei candidati")
	}
	if carte := pannello[i:]; strings.Contains(carte, `type="radio"`) || strings.Contains(carte[:strings.Index(carte, "</section>")], "checked") {
		t.Error("una card ha un radio o nasce spuntata")
	}
}
