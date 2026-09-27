//go:build integrazione

// L4 — Smistamento M3 (A5.14.6, A5.16.6): le query della calibrazione della posta su una scena con i
// risultati noti, in una transazione READ ONLY. Nomi e codici inventati (ACME, 7120001).
package calibrazione

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/testutil"
)

// 236 — le query di calibrazione (parte posta). Tre decisioni con la fotografia, due al primo posto →
// 0,667, e «campione insufficiente» nella stampa, perché tre non sono venti. Fuori dal campione: il giro
// degli orfani (automatico), una decisione senza candidati, una riga di prima (una frase) e un motivo che
// sembra un JSON e non lo è — che non deve rompere la query. La retro vede solo i messaggi agganciati a
// mano senza fotografia, ordinati come il pannello. La transazione è in sola lettura.
func TestLeQueryDiCalibrazioneEIlComando(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	uno := func(sql string, args ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := p.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	esegui := func(sql string, args ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	utente := uno(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('FP', 'Operatore', 'commerciale') RETURNING utente_id`)
	cliente := uno(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme S.p.A.') RETURNING cliente_id`)
	rfq := func(nome string, giorni int) uuid.UUID {
		return uno(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
			VALUES ($1, 'outlook', now() - make_interval(days => $3), $2, $2) RETURNING thread_id`, cliente, nome, giorni)
	}
	t1, t2, t3 := rfq("RFQ 7120001", 3), rfq("RFQ 7120002", 2), rfq("RFQ 7120003", 1)
	conv := uno(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'CONV-CAL', now()) RETURNING conversazione_id`)
	n := 0
	messaggio := func() uuid.UUID {
		n++
		return uno(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, mittente_indirizzo, oggetto)
			VALUES ('outlook', $1, $2, 'entrata', now(), 'buyer@acme.example', 'RFQ 7120001') RETURNING messaggio_id`,
			"<cal-"+string(rune('a'+n))+"@acme.example>", conv)
	}
	log := func(m, thread uuid.UUID, azione string, u *uuid.UUID, motivo string) {
		t.Helper()
		var tid, uid any
		if thread != uuid.Nil {
			tid = thread
		}
		if u != nil {
			uid = *u
		}
		esegui(`INSERT INTO messaggio_aggancio_log (messaggio_id, thread_id, azione, utente_id, motivo) VALUES ($1, $2, $3, $4, $5)`,
			m, tid, azione, uid, motivo)
	}
	// l'elenco che l'operatore aveva davanti: T1 molto forte, T2 medio-forte, T3 debole
	elenco := classificazione.RaggruppaEOrdina([]classificazione.Candidato{
		classificazione.NuovoCandidato(t1.String(), classificazione.TipoR0InReplyTo, "risponde a una mail della RFQ", false),
		classificazione.NuovoCandidato(t2.String(), classificazione.TipoR3CodiceBuyer, "codice 7120001", false),
		classificazione.NuovoCandidato(t3.String(), classificazione.TipoR1Solo, "stessa conversazione", false),
	})
	foto := func(gesto string, scelto uuid.UUID) string {
		return classificazione.SceltaDa(elenco, gesto, scelto.String(), classificazione.EventoArrivoCAD, classificazione.AttoDocumentiAggiuntivi).Motivo()
	}
	// tre decisioni con la fotografia: al primo, al primo, al secondo
	for _, scelto := range []uuid.UUID{t1, t1, t2} {
		m := messaggio()
		esegui(`UPDATE messaggio SET thread_id = $2, aggancio = 'operatore', agganciato_il = now() WHERE messaggio_id = $1`, m, scelto)
		log(m, scelto, "aggancia", &utente, foto(classificazione.GestoAggancia, scelto))
	}
	// fuori dal campione
	orfano := messaggio()
	candidato := classificazione.SceltaDa(elenco, classificazione.GestoCandidato, t3.String(), "", "")
	candidato.Frase = "orfano della stessa conversazione: candidato debole · score 40, non agganciato"
	log(orfano, t3, "candidato", &utente, candidato.Motivo())
	senzaLista := messaggio()
	log(senzaLista, t1, "aggancia", &utente, classificazione.SceltaDa(nil, classificazione.GestoNuovaRFQ, t1.String(), "", "").Motivo())
	log(messaggio(), t1, "aggancia", &utente, "decisione dell'operatore")
	log(messaggio(), uuid.Nil, "ignora", &utente, "{non è un JSON")

	// la retro: tre messaggi agganciati a mano PRIMA della fotografia
	retro1, retro2, retro3 := messaggio(), messaggio(), messaggio()
	for _, m := range []uuid.UUID{retro1, retro2, retro3} {
		esegui(`UPDATE messaggio SET thread_id = $2, aggancio = 'operatore', agganciato_il = now() WHERE messaggio_id = $1`, m, t1)
		log(m, t1, "aggancia", &utente, "decisione dell'operatore")
	}
	cand := func(m, thread uuid.UUID, regola string, punti int, evidenza string) {
		esegui(`INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato)
			VALUES ($1, $2, $3, $4, $5, 'APERTA')`, m, thread, regola, punti, evidenza)
	}
	// retro1: il R1 delle regole di prima (95) sta sotto il R3 del buyer nel pannello di oggi: primo giusto
	cand(retro1, t2, "R1_conversazione", 95, "stessa conversazione")
	cand(retro1, t1, "R3_codice", 72, "codice 7120001")
	// retro2: il primo era T2, l'operatore ha scelto T1 (che non era fra i candidati)
	cand(retro2, t2, "R0_reply", 98, "risponde a una mail della RFQ")
	// retro3: nessun candidato
	// e un messaggio con la fotografia e i candidati NON entra nella retro
	var conFoto uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio_aggancio_log WHERE motivo LIKE '{%' AND azione = 'aggancia' LIMIT 1`).Scan(&conFoto); err != nil {
		t.Fatal(err)
	}
	cand(conFoto, t3, "R2_oggetto", 20, "stesso oggetto")

	contaLog := testutil.Conta(t, p, "messaggio_aggancio_log")
	r, err := Misura(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.SolaLettura {
		t.Error("le misure non stanno in una transazione READ ONLY")
	}
	if testutil.Conta(t, p, "messaggio_aggancio_log") != contaLog {
		t.Error("le misure hanno scritto")
	}
	cella := func(per, livello, tipo, fascia string) RigaPosta {
		t.Helper()
		for _, x := range r.Posta.Misure {
			if x.Per == per && x.Livello == livello && x.Tipo == tipo && x.Fascia == fascia {
				return x
			}
		}
		t.Fatalf("manca la cella %s/%s/%s/%s in %+v", per, livello, tipo, fascia, r.Posta.Misure)
		return RigaPosta{}
	}
	tot := cella(PerTotale, "", "", "")
	if tot.Regole != classificazione.VersioneRegoleAggancio || tot.Decisioni != 3 || tot.AlPrimo != 2 || tot.NeiPrimi3 != 3 || tot.Agganci != 3 ||
		tot.AgganciAlPrimo != 2 || tot.NuoveRFQ != 0 || tot.Ignorati != 0 || tot.FuoriLista != 0 || tot.Ridecisi != 0 {
		t.Errorf("totale: %+v", tot)
	}
	if p, ok := tot.Precisione(); ok || p < 0.666 || p > 0.667 {
		t.Errorf("precision@1 = %v (sufficiente %v), attesa 0,667 e campione insufficiente", p, ok)
	}
	if p, ok := tot.PrimoGiusto(); ok || p < 0.666 || p > 0.667 {
		t.Errorf("primo giusto = %v (sufficiente %v): con soli agganci coincide con la precision@1", p, ok)
	}
	if tot.MRR != 0.833 {
		t.Errorf("MRR = %v, atteso 0,833 (1, 1, 1/2)", tot.MRR)
	}
	if c := cella(PerLivello, "molto_forte", "", ""); c.Decisioni != 3 {
		t.Errorf("per livello del primo: %+v", c)
	}
	if c := cella(PerTipoFascia, "", classificazione.TipoR0InReplyTo, "90-100"); c.Decisioni != 3 || c.AlPrimo != 2 {
		t.Errorf("per tipo e fascia: %+v", c)
	}
	cella(PerTipo, "", classificazione.TipoR0InReplyTo, "")
	cella(PerFascia, "", "", "90-100")
	if len(r.Posta.Misure) != 5 {
		t.Errorf("celle: %d, attese 5 (una sola versione, un solo livello, tipo e fascia)", len(r.Posta.Misure))
	}
	// la retro
	var rtot RigaRetro
	for _, x := range r.Posta.Retro {
		if x.Per == PerTotale {
			rtot = x
		}
	}
	if rtot.Messaggi != 2 || rtot.AlPrimo != 1 || rtot.FuoriLista != 1 || r.Posta.RetroSenzaCandidati != 1 {
		t.Errorf("retro: %+v, senza candidati %d", rtot, r.Posta.RetroSenzaCandidati)
	}

	// la stampa: prima riga il database, «campione insufficiente», nessuna password
	var b bytes.Buffer
	if err := r.Scrivi(&b); err != nil {
		t.Fatal(err)
	}
	t.Log("la stampa del comando:\n" + b.String())
	cc := p.Config().ConnConfig
	prima := strings.SplitN(b.String(), "\n", 2)[0]
	if !strings.Contains(prima, cc.Database) || !strings.Contains(prima, cc.Host) {
		t.Errorf("la prima riga non dice che cosa si legge: %q", prima)
	}
	if cc.Password != "" && strings.Contains(b.String(), ":"+cc.Password+"@") || strings.Contains(b.String(), "password") {
		t.Error("la stampa contiene la password")
	}
	if !strings.Contains(b.String(), FraseCampioneInsufficiente) {
		t.Errorf("tre decisioni devono dire «campione insufficiente»:\n%s", b.String())
	}

	// -calibrazione-dal: da domani non c'è niente
	domani := time.Now().Add(24 * time.Hour)
	r, err = Misura(ctx, p, &domani)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Posta.Misure) != 0 || len(r.Posta.Retro) != 0 || r.Posta.RetroSenzaCandidati != 0 {
		t.Errorf("dal giorno dopo: %+v", r.Posta)
	}
}

// 236 (parte posta, una decisione per messaggio) — il log è append-only, e lo stesso messaggio può avere
// più fotografie: un secondo clic su «Aggancia» verso la stessa RFQ (il ramo «già deciso, stessa RFQ»
// rifà l'aggancio e riscrive la fotografia), un «Ignora» e poi un «Aggancia» (un messaggio ignorato non ha
// thread e resta decidibile), due «Ignora» nello stesso istante, un «Aggancia» corretto in un «Ignora».
// Conta la decisione di ogni messaggio con la sua ULTIMA fotografia: per «ignora e poi aggancia» conta
// l'aggancio, per «aggancia e poi ignora» l'ignora, e a parità di istante la riga scritta dopo (log_id).
// Se l'ultima decisione è senza candidati il messaggio esce dal campione, anche se una di prima li aveva.
// `Ridecisi` conta a parte i messaggi con più fotografie.
//
// La scena è ASIMMETRICA di proposito: con la prima fotografia al posto dell'ultima «corretto» diventa un
// ignora, «ripensato» e «pari» agganci alla seconda card, «troncato» un aggancio alla prima, e i totali
// cambiano. Controprove (ognuna fa fallire la prova): senza il DISTINCT ON; con `eseguito_il ASC, log_id
// ASC` (la prima fotografia); con il solo `log_id ASC` (lo spareggio rovesciato); con il filtro `su > 0`
// dentro `g`, prima del DISTINCT ON.
func TestLaCalibrazioneContaUnaDecisionePerMessaggio(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	uno := func(sql string, args ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := p.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	esegui := func(sql string, args ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	utente := uno(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('FP', 'Operatore', 'commerciale') RETURNING utente_id`)
	cliente := uno(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme S.p.A.') RETURNING cliente_id`)
	rfq := func(nome string) uuid.UUID {
		return uno(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
			VALUES ($1, 'outlook', now(), $2, $2) RETURNING thread_id`, cliente, nome)
	}
	t1, t2, nuova := rfq("RFQ 7120001"), rfq("RFQ 7120002"), rfq("RFQ 7120003")
	conv := uno(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'CONV-RIP', now()) RETURNING conversazione_id`)
	messaggio := func(chiave string) uuid.UUID {
		return uno(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, mittente_indirizzo, oggetto)
			VALUES ('outlook', $1, $2, 'entrata', now(), 'buyer@acme.example', 'RFQ 7120001') RETURNING messaggio_id`, chiave, conv)
	}
	// l'elenco davanti all'operatore: T1 primo, T2 secondo
	elenco := classificazione.RaggruppaEOrdina([]classificazione.Candidato{
		classificazione.NuovoCandidato(t1.String(), classificazione.TipoR0InReplyTo, "risponde a una mail della RFQ", false),
		classificazione.NuovoCandidato(t2.String(), classificazione.TipoR3CodiceBuyer, "codice 7120001", false),
	})
	// l'istante di ogni fotografia è scritto esplicito, `minuti` prima di adesso: due righe con gli stessi
	// minuti hanno lo STESSO eseguito_il, e le separa solo il log_id (la riga scritta dopo ha il log_id più
	// alto). Con now() le due INSERT in autocommit avrebbero due istanti diversi.
	adesso := time.Now()
	log := func(m uuid.UUID, lista []classificazione.CandidatoRFQ, gesto string, scelto uuid.UUID, minuti int) {
		t.Helper()
		var tid any
		azione, id := "ignora", ""
		if scelto != uuid.Nil {
			tid, azione, id = scelto, "aggancia", scelto.String()
		}
		motivo := classificazione.SceltaDa(lista, gesto, id, classificazione.EventoArrivoCAD, classificazione.AttoDocumentiAggiuntivi).Motivo()
		esegui(`INSERT INTO messaggio_aggancio_log (messaggio_id, thread_id, azione, utente_id, motivo, eseguito_il)
			VALUES ($1, $2, $3, $4, $5, $6)`, m, tid, azione, utente, motivo, adesso.Add(-time.Duration(minuti)*time.Minute))
	}
	// doppio aggancio alla seconda card: conta una volta, rango 2
	doppio := messaggio("<rip-a@acme.example>")
	log(doppio, elenco, classificazione.GestoAggancia, t2, 3)
	log(doppio, elenco, classificazione.GestoAggancia, t2, 2)
	// ignora, poi aggancia alla prima card: conta l'aggancio al primo (la prima fotografia sarebbe un ignora)
	corretto := messaggio("<rip-b@acme.example>")
	log(corretto, elenco, classificazione.GestoIgnora, uuid.Nil, 3)
	log(corretto, elenco, classificazione.GestoAggancia, t1, 2)
	// ignora due volte nello stesso istante: un ignora
	ignorato := messaggio("<rip-c@acme.example>")
	log(ignorato, elenco, classificazione.GestoIgnora, uuid.Nil, 1)
	log(ignorato, elenco, classificazione.GestoIgnora, uuid.Nil, 1)
	// nello stesso istante, prima alla seconda card e poi alla prima: vince la riga scritta dopo, il rango 1
	pari := messaggio("<rip-d@acme.example>")
	log(pari, elenco, classificazione.GestoAggancia, t2, 1)
	log(pari, elenco, classificazione.GestoAggancia, t1, 1)
	// un aggancio alla seconda card corretto in un ignora: conta l'ignora
	ripensato := messaggio("<rip-e@acme.example>")
	log(ripensato, elenco, classificazione.GestoAggancia, t2, 5)
	log(ripensato, elenco, classificazione.GestoIgnora, uuid.Nil, 4)
	// un aggancio alla prima card, poi una RFQ nuova decisa senza nessun candidato: la decisione che conta
	// non ha un primo da misurare, e il messaggio esce dal campione (la fotografia di prima non lo rimette)
	troncato := messaggio("<rip-f@acme.example>")
	log(troncato, elenco, classificazione.GestoAggancia, t1, 7)
	log(troncato, nil, classificazione.GestoNuovaRFQ, nuova, 6)
	if n := testutil.Conta(t, p, "messaggio_aggancio_log"); n != 12 {
		t.Fatalf("righe nel log: %d, attese 12", n)
	}

	r, err := Misura(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tot RigaPosta
	for _, x := range r.Posta.Misure {
		if x.Per == PerTotale {
			tot = x
		}
	}
	// cinque messaggi nel campione: tre agganci (rango 2, 1, 1) e due ignora; «troncato» è fuori
	if tot.Decisioni != 5 || tot.Agganci != 3 || tot.AgganciAlPrimo != 2 || tot.AlPrimo != 2 || tot.NeiPrimi3 != 3 ||
		tot.Ignorati != 2 || tot.NuoveRFQ != 0 || tot.Ridecisi != 5 {
		t.Errorf("una decisione per messaggio, l'ultima: %+v", tot)
	}
	if p, _ := tot.Precisione(); p < 0.666 || p > 0.667 {
		t.Errorf("precision@1 sugli agganci: %v, attesa 0,667 (due su tre)", p)
	}
	if p, _ := tot.PrimoGiusto(); p != 0.4 {
		t.Errorf("primo giusto su tutte: %v, atteso 0,4 (due su cinque)", p)
	}
	if tot.MRR != 0.5 {
		t.Errorf("MRR = %v, atteso 0,5 ((1/2 + 1 + 0 + 1 + 0) / 5)", tot.MRR)
	}
	// un periodo che taglia fuori le fotografie di più di due minuti e mezzo fa: «ripensato» e «troncato»
	// escono, «doppio» e «corretto» hanno una fotografia sola nel periodo (e non sono ridecisi), «ignorato»
	// e «pari» ne hanno ancora due
	dal := adesso.Add(-150 * time.Second)
	if r, err = Misura(ctx, p, &dal); err != nil {
		t.Fatal(err)
	}
	visto := false
	for _, x := range r.Posta.Misure {
		if x.Per != PerTotale {
			continue
		}
		visto = true
		if x.Decisioni != 4 || x.Ridecisi != 2 || x.Ignorati != 1 || x.Agganci != 3 || x.AgganciAlPrimo != 2 {
			t.Errorf("dal %s: %+v", dal.Format(time.TimeOnly), x)
		}
	}
	if !visto {
		t.Errorf("dal %s: manca il totale in %+v", dal.Format(time.TimeOnly), r.Posta.Misure)
	}
}
