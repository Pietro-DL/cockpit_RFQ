//go:build integrazione

// L4 — LS-01 (direttiva 23 §4.1 e §8; 17/10/2026): la struttura STEP che il worker scrive attraversa tutta la catena
// che decide la completezza, e «Rianalizza» recupera, e solo lui, gli STEP letti nella forma che il server non accetta.
//
// GP-3 aveva fatto scrivere al worker la struttura `versione: 4`. La prova di parità di allora confrontava lo schema
// Python con la costante Go, ed era verde; ma l'unica definizione di «lettura completa», la funzione SQL della 0020,
// accetta solo la 3, e ogni STEP letto da quel worker era «parziale» per il Motore A, per la vista dello STEP del
// prodotto e per la gerarchia. Le prove di questo file chiudono il buco dove si era aperto:
//
//   - TestLS01UscitaVeraDelWorkerAttraversaLaCatena: il worker VERO (worker_analisi.py, --una-volta) legge uno STEP
//     sintetico, e la sua uscita passa per il DTO (DecodificaStruttura, DiagnosticaSTEP, StrutturaDaRileggere), la
//     funzione SQL (struttura_motivo_parziale, struttura_completa), la vista v_step_prodotto, le proposte di struttura
//     (fascicolo.ApplicaStruttura), la fotografia del Motore A (caricatore), l'adattatore dell'estrazione
//     (grafo_completo), la struttura dell'ancoraggio (GrafoCompleto) e la regola della gerarchia (GerarchiaStepDi).
//     Completa con la forma di oggi; parziale, con lo stesso motivo in tutti i consumatori, con la v4 di GP-3, con una
//     versione sconosciuta e con dati incompleti (lettura troncata, occorrenze non risolte, scarti mancanti, due
//     radici con lo stato «valida»). Nessuna scorciatoia: la diagnostica non rende completa nessuna lettura.
//   - TestLS01RianalizzaRiaccodaSoloGliStepLettiInUnaFormaNonAccettata (P-10): la riaccodatura mirata, la sua
//     idempotenza, il limite, lo staging, la deroga che decade, le righe decise da una persona che restano, la flotta
//     mista.
//   - TestLS01RianalizzaPerContenutoConIlLimiteCondiviso (P-10b, correzioni dei verificatori): una riaccodatura per
//     contenuto anche con più copie, la copia nello staging preferita, i compatibili prima quando c'è il limite, il
//     limite residuo condiviso con le analisi mancanti e con i PDF.
//   - TestLS01IlLogNominaIlWorkerDiOgniStrutturaNonAggiornata (correzioni dei verificatori): il log del server, con il
//     worker, per la forma non accettata e per la forma 3 senza versione_diagnostica (un worker di prima di GP-3).
//
// Gli STEP, i codici e i nomi sono inventati (ACME, 712xxxx): il repository è pubblico.

package workerapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// stepLS01: uno STEP Part 21 sintetico a una radice. Un assieme senza geometria propria (solo la terna), due pezzi con
// un solido (il primo due volte), una foglia «saldatura» con la sola terna. Per la 0020 è una lettura completa: una
// radice, tre relazioni, niente scartato. È la forma di test_step_v4.py, in un assieme che la funzione SQL accetta.
const stepLS01 = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('7120500.stp','2026-10-17T00:00:00',(''),(''),'','','');
FILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 3 1 1 }'));
ENDSEC;
DATA;
#1=APPLICATION_CONTEXT('prova');
#10=PRODUCT('7120500','7120500','ASSIEME ACME',(#1));
#11=PRODUCT_DEFINITION_FORMATION('A','',#10);
#12=PRODUCT_DEFINITION('design','',#11,#1);
#20=PRODUCT('7120510','7120510','PIASTRA ACME',(#1));
#21=PRODUCT_DEFINITION_FORMATION('1','',#20);
#22=PRODUCT_DEFINITION('design','',#21,#1);
#30=PRODUCT('7120520','7120520','STAFFA ACME',(#1));
#31=PRODUCT_DEFINITION_FORMATION('1','',#30);
#32=PRODUCT_DEFINITION('design','',#31,#1);
#40=PRODUCT('Saldatura d''angolo ACME','Saldatura d''angolo ACME','',(#1));
#41=PRODUCT_DEFINITION_FORMATION('','',#40);
#42=PRODUCT_DEFINITION('design','',#41,#1);
#50=PRODUCT_DEFINITION_SHAPE('','',#12);
#51=SHAPE_REPRESENTATION('',(#90),#2);
#52=SHAPE_DEFINITION_REPRESENTATION(#50,#51);
#60=PRODUCT_DEFINITION_SHAPE('','',#22);
#61=ADVANCED_BREP_SHAPE_REPRESENTATION('',(#91,#92),#2);
#62=SHAPE_DEFINITION_REPRESENTATION(#60,#61);
#70=PRODUCT_DEFINITION_SHAPE('','',#32);
#71=ADVANCED_BREP_SHAPE_REPRESENTATION('',(#93,#94),#2);
#72=SHAPE_DEFINITION_REPRESENTATION(#70,#71);
#80=PRODUCT_DEFINITION_SHAPE('','',#42);
#81=SHAPE_REPRESENTATION('',(#95),#2);
#82=SHAPE_DEFINITION_REPRESENTATION(#80,#81);
#90=AXIS2_PLACEMENT_3D('',#3,#4,#5);
#91=MANIFOLD_SOLID_BREP('P1',#96);
#92=AXIS2_PLACEMENT_3D('',#3,#4,#5);
#93=MANIFOLD_SOLID_BREP('P2',#97);
#94=AXIS2_PLACEMENT_3D('',#3,#4,#5);
#95=AXIS2_PLACEMENT_3D('',#3,#4,#5);
#100=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO1','7120510:1','',#12,#22,$);
#101=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO2','7120510:2','',#12,#22,$);
#102=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO3','7120520:1','',#12,#32,$);
#103=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO4','Saldatura d''angolo ACME:1','',#12,#42,$);
ENDSEC;
END-ISO-10303-21;
`

// analisiDaiFatti è il costruttore DI PROVA di valutazione.AnalisiStruttura, dai numeri della struttura decodificata e
// dal motivo della 0020 (V0-D31). Il costruttore di produzione è dell'innesto B3.5, che non c'è ancora (GerarchiaStepDi
// non ha chiamanti di produzione): quando ci sarà, questa prova dovrà chiamare quello. Scarti somma i tre contatori che
// la 0020 guarda; le occorrenze di un pezzo dentro se stesso no, perché sono archi impossibili e non tolgono completezza.
func analisiDaiFatti(st *worker.StrutturaSTEP, motivo string) valutazione.AnalisiStruttura {
	a := valutazione.AnalisiStruttura{Letta: st != nil, MotivoParziale: motivo}
	if st == nil {
		return a
	}
	a.Versione, a.Troncato, a.Radici, a.Relazioni = st.Versione, st.Limiti.Troncato, len(st.Radici), len(st.Relazioni)
	if st.Scarti != nil {
		a.Scarti = st.Scarti.ProdottiSenzaDefinizione + st.Scarti.OccorrenzeNonRisolte + st.Scarti.TestiTroncati
	}
	return a
}

// capacitaDelDocumento: la capacità del documento delle evidenze con quel nome; vuota se non c'è.
func capacitaDelDocumento(d evidenze.DocumentoEvidenze, nome string) evidenze.Capacita {
	for _, c := range d.Qualita.Capacita {
		if c.Nome == nome {
			return c
		}
	}
	return evidenze.Capacita{}
}

// strutturaDeiFatti: la parte «struttura» dei fatti, com'è nel JSON.
func strutturaDeiFatti(t *testing.T, fatti json.RawMessage) json.RawMessage {
	t.Helper()
	var involucro struct {
		Struttura json.RawMessage `json:"struttura"`
	}
	if err := json.Unmarshal(fatti, &involucro); err != nil || len(involucro.Struttura) == 0 {
		t.Fatalf("i fatti non hanno la struttura: %v %s", err, fatti)
	}
	return involucro.Struttura
}

func TestLS01UscitaVeraDelWorkerAttraversaLaCatena(t *testing.T) {
	if os.Getenv("COCKPIT_TEST_SENZA_PYTHON") != "" {
		t.Skip("COCKPIT_TEST_SENZA_PYTHON: il worker vero non viene avviato, prova non verificata")
	}
	b := preparaBancoAnalisiCon(t, "7120500.stp", []byte(stepLS01))
	ctx := b.ctx
	riga := func(sql string, arg ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := b.pool.QueryRow(ctx, sql, arg...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return id
	}
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := b.pool.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	cliente := riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('LS01', 'ACME LS-01') RETURNING cliente_id`)
	thread := riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente)
	esegui(`UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, b.allegato.MessaggioID, thread)

	uscita, _, _ := eseguiWorkerAnalisiVero(t, b)
	j, err := b.q.GetJob(ctx, b.analisi.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Stato != db.StatoJobFatto {
		t.Fatalf("job di analisi in stato %s (errore %q).\nOutput del worker:\n%s", j.Stato, j.Errore.String, uscita)
	}
	an := b.s.Analizzatore
	chiaveFatti := db.GetAnalisiFattiParams{Sha256: b.sha, VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: an.Hash()}
	fatti, err := b.q.GetAnalisiFatti(ctx, chiaveFatti)
	if err != nil {
		t.Fatalf("i fatti del worker vero non sono stati conservati: %v", err)
	}
	uscitaVera := strutturaDeiFatti(t, fatti.Fatti)

	// (a) Il DTO, sull'uscita vera: la forma che il server accetta, la diagnostica numerata, nessun campo perso.
	st, ok := worker.DecodificaStruttura(fatti.Fatti)
	if !ok {
		t.Fatalf("struttura non decodificata: %s", fatti.Fatti)
	}
	if st.Versione != worker.VersioneStruttura || st.VersioneDiagnostica != worker.VersioneDiagnostica || !worker.DiagnosticaSTEP(st) ||
		worker.StrutturaDaRileggere(fatti.Fatti) {
		t.Fatalf("il worker vero scrive versione %d e diagnostica %d (attese %d e %d)", st.Versione, st.VersioneDiagnostica,
			worker.VersioneStruttura, worker.VersioneDiagnostica)
	}
	if st.Stato != worker.StrutturaValida || len(st.Radici) != 1 || len(st.Nodi) != 4 || len(st.Relazioni) != 3 ||
		st.Scarti == nil || *st.Scarti != (worker.ScartiSTEP{}) || st.Limiti.Troncato {
		t.Fatalf("struttura letta male: %+v", st)
	}
	geometria := map[string]worker.GeometriaSTEP{}
	for _, n := range st.Nodi {
		if n.Geometria == nil {
			t.Fatalf("il nodo %s ha perso la geometria nella decodifica: %s", n.Chiave, uscitaVera)
		}
		geometria[n.Chiave] = *n.Geometria
	}
	if !geometria["#10"].Vuota || geometria["#20"].Solidi != 1 || geometria["#30"].Solidi != 1 || !geometria["#40"].Vuota {
		t.Errorf("geometria per nodo: %+v", geometria)
	}
	istanze := 0
	for _, r := range st.Relazioni {
		istanze += len(r.Istanze)
		if r.Figlio == "#20" && (r.Qta != 2 || len(r.Istanze) != 2 || r.Istanze[1].Nome != "7120510:2") {
			t.Errorf("relazione con il pezzo ripetuto: %+v", r)
		}
	}
	if istanze != 4 {
		t.Errorf("istanze NAUO decodificate: %d, attese 4", istanze)
	}
	// il JSON del worker porta esattamente i campi che il DTO conosce: niente si perde passando dal Go
	var campi map[string]json.RawMessage
	if err := json.Unmarshal(uscitaVera, &campi); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"versione", "versione_diagnostica", "schema", "radici", "nodi", "relazioni", "avvisi", "scarti", "limiti",
		"stato", "motivo_stato"} {
		if _, c := campi[k]; !c {
			t.Errorf("l'uscita del worker non porta %q: %s", k, uscitaVera)
		}
	}
	if len(campi) != 11 {
		t.Errorf("l'uscita del worker ha %d campi, attesi 11 (un campo nuovo va aggiunto al DTO): %s", len(campi), uscitaVera)
	}

	// Lo STEP autorizzato per il finito, con l'analizzatore corrente scritto come all'avvio del server.
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, an.Versione, an.Hash())
	utente := riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('L1', 'Prova LS-01', 'Tecnico') RETURNING utente_id`)
	finito := riga(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120500', 'finito', 'manuale', $2)
		RETURNING componente_id`, thread, utente)
	testutil.AutorizzaStep(t, b.pool, thread, finito, b.allegato.AllegatoID, utente)
	motore, err := fascicolo.MotoreDellaRfq(ctx, b.q, thread)
	if err != nil {
		t.Fatal(err)
	}

	type atteso struct {
		motivo      string // della 0020: "" = completa
		gerarchia   valutazione.GerarchiaStep
		daRileggere bool
		diagnostica bool // DiagnosticaSTEP
	}
	// verifica: la catena intera sui fatti che stanno adesso in analisi_fatti.
	verifica := func(t *testing.T, a atteso) {
		t.Helper()
		f, err := b.q.GetAnalisiFatti(ctx, chiaveFatti)
		if err != nil {
			t.Fatal(err)
		}
		completa := a.motivo == ""
		// (a) DTO: si decodifica sempre (nessun rifiuto), la diagnostica arriva tutta (nessun campo perso), e chi la
		// legge lo decide dal numero
		st, ok := worker.DecodificaStruttura(f.Fatti)
		if !ok {
			t.Fatalf("il DTO rifiuta la struttura: %s", f.Fatti)
		}
		for _, n := range st.Nodi {
			if n.Geometria == nil {
				t.Errorf("il nodo %s ha perso la geometria", n.Chiave)
			}
		}
		if worker.DiagnosticaSTEP(st) != a.diagnostica || worker.StrutturaDaRileggere(f.Fatti) != a.daRileggere {
			t.Errorf("DTO: diagnostica leggibile %v (attesa %v), da rileggere %v (atteso %v)", worker.DiagnosticaSTEP(st), a.diagnostica,
				worker.StrutturaDaRileggere(f.Fatti), a.daRileggere)
		}
		// (b) la funzione SQL, l'unica definizione di «completa»
		grezza := strutturaDeiFatti(t, f.Fatti)
		motivo, err := b.q.MotivoParziale(ctx, &grezza)
		if err != nil {
			t.Fatal(err)
		}
		var sqlCompleta bool
		if err := b.pool.QueryRow(ctx, `SELECT struttura_completa(fatti -> 'struttura') FROM analisi_fatti
			WHERE sha256 = $1 AND versione_analizzatore = $2 AND hash_configurazione = $3`, b.sha, an.Versione, an.Hash()).Scan(&sqlCompleta); err != nil {
			t.Fatal(err)
		}
		if motivo != a.motivo || sqlCompleta != completa {
			t.Errorf("SQL: motivo %q completa %v, atteso %q", motivo, sqlCompleta, a.motivo)
		}
		// (c) la vista dello STEP del prodotto
		var esito string
		var motivoVista *string
		var analisiCompleta bool
		if err := b.pool.QueryRow(ctx, `SELECT esito, motivo_parziale, analisi_completa FROM v_step_prodotto WHERE componente_id = $1`, finito).
			Scan(&esito, &motivoVista, &analisiCompleta); err != nil {
			t.Fatal(err)
		}
		esitoAtteso := map[bool]string{true: "presente_analizzato", false: "presente_parziale"}[completa]
		if esito != esitoAtteso || analisiCompleta != completa || (completa && motivoVista != nil) || (!completa && (motivoVista == nil || *motivoVista != a.motivo)) {
			t.Errorf("v_step_prodotto: esito %s, completa %v, motivo %v; attesi %s e %q", esito, analisiCompleta, motivoVista, esitoAtteso, a.motivo)
		}
		// (d) le proposte di struttura del Fascicolo (quantità e rimozioni solo da una lettura completa)
		es, err := fascicolo.ApplicaStruttura(ctx, b.q, thread, b.allegato, f.Fatti, motore)
		if err != nil {
			t.Fatal(err)
		}
		if es.Completa != completa || es.MotivoParziale != a.motivo {
			t.Errorf("ApplicaStruttura: completa %v motivo %q, atteso %q", es.Completa, es.MotivoParziale, a.motivo)
		}
		// (e) la fotografia del Motore A, letta in sola lettura dal caricatore
		foto, err := caricatore.Carica(ctx, b.pool, caricatore.Richiesta{Thread: []uuid.UUID{thread}})
		if err != nil {
			t.Fatal(err)
		}
		if len(foto.Thread) != 1 {
			t.Fatalf("thread nella fotografia: %d", len(foto.Thread))
		}
		th := foto.Thread[0]
		ff, c := th.Fatti[b.sha]
		if !c || ff.MotivoParziale == nil || *ff.MotivoParziale != a.motivo {
			t.Errorf("fotografia: fatti presenti %v, motivo %v, atteso %q", c, ff.MotivoParziale, a.motivo)
		}
		vistaNellaFoto := false
		for _, r := range th.StepProdotto {
			if r.ComponenteID == finito {
				vistaNellaFoto = r.Esito == esitoAtteso && r.AnalisiCompleta == completa
			}
		}
		if !vistaNellaFoto {
			t.Errorf("fotografia: la riga della vista del finito non dice %s", esitoAtteso)
		}
		// (f) l'adattatore dell'estrazione e la struttura dell'ancoraggio
		var al *fotorfq.Allegato
		for i := range th.Allegati {
			if th.Allegati[i].ID == b.allegato.AllegatoID {
				al = &th.Allegati[i]
			}
		}
		if al == nil {
			t.Fatal("l'allegato dello STEP non è nella fotografia")
		}
		d, err := estrazione.DaAllegato(*al, &ff, nil)
		if err != nil {
			t.Fatalf("estrazione: %v", err)
		}
		g, s := capacitaDelDocumento(d, "grafo_completo"), capacitaDelDocumento(d, "struttura")
		if s.Stato != "disponibile" && s.Stato != "parziale" {
			t.Errorf("estrazione: la struttura non è disponibile (%+v): il Motore A ha rifiutato i fatti", s)
		}
		if (completa && g.Stato != "disponibile") || (!completa && (g.Stato != "parziale" || g.Motivo != a.motivo)) {
			t.Errorf("estrazione: grafo_completo %+v, atteso %q", g, a.motivo)
		}
		sf, ok := ancoraggio.StrutturaDa(ancoraggio.FileInterpretato{AllegatoID: al.ID, Documento: d,
			Interpretazione: motorea.Interpretazione{BundleID: d.BundleID}})
		if !ok {
			t.Fatal("ancoraggio: lo STEP letto non dà una struttura")
		}
		if sf.GrafoCompleto != completa || (!completa && sf.MotivoGrafo != a.motivo) || len(sf.Nodi) != len(st.Nodi) {
			t.Errorf("ancoraggio: grafo completo %v motivo %q nodi %d; attesi %v %q %d", sf.GrafoCompleto, sf.MotivoGrafo, len(sf.Nodi),
				completa, a.motivo, len(st.Nodi))
		}
		// (g) la regola della gerarchia, con i numeri decodificati e il motivo della 0020
		if gs := valutazione.GerarchiaStepDi(analisiDaiFatti(st, motivo)); gs != a.gerarchia {
			t.Errorf("gerarchia: %s, attesa %s", gs, a.gerarchia)
		}
	}
	// variante: i fatti conservati diventano l'uscita vera con una modifica (nella stessa riga di analisi_fatti).
	variante := func(t *testing.T, cambia func(m map[string]any)) {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(uscitaVera, &m); err != nil {
			t.Fatal(err)
		}
		cambia(m)
		nuova, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		esegui(`UPDATE analisi_fatti SET fatti = jsonb_set(fatti, '{struttura}', $4::jsonb)
			WHERE sha256 = $1 AND versione_analizzatore = $2 AND hash_configurazione = $3`, b.sha, an.Versione, an.Hash(), string(nuova))
	}
	sotto := func(m map[string]any, chiave string) map[string]any {
		x, _ := m[chiave].(map[string]any)
		if x == nil {
			t.Fatalf("l'uscita del worker non ha %q come oggetto", chiave)
		}
		return x
	}

	t.Run("uscita vera del worker: completa", func(t *testing.T) {
		verifica(t, atteso{motivo: "", gerarchia: valutazione.GerarchiaStepRiconosciuta, daRileggere: false, diagnostica: true})
	})
	casi := []struct {
		nome   string
		cambia func(m map[string]any)
		atteso atteso
	}{
		{"la v4 di GP-3: parziale, e si rilegge", func(m map[string]any) {
			m["versione"] = worker.VersioneStrutturaGP3
			delete(m, "versione_diagnostica")
		}, atteso{"struttura v4: gli scarti non sono numerati, serve la rianalisi v3", valutazione.GerarchiaStepNonAnalizzabile, true, false}},
		{"una versione sconosciuta (5): parziale, e si rilegge", func(m map[string]any) { m["versione"] = 5 },
			atteso{"struttura v5: gli scarti non sono numerati, serve la rianalisi v3", valutazione.GerarchiaStepNonAnalizzabile, true, true}},
		{"dati incompleti: lettura troncata", func(m map[string]any) {
			l := sotto(m, "limiti")
			l["troncato"], l["motivo"] = true, "nodi"
		}, atteso{"lettura troncata: nodi", valutazione.GerarchiaStepParziale, false, true}},
		{"dati incompleti: un'occorrenza non risolta", func(m map[string]any) { sotto(m, "scarti")["occorrenze_non_risolte"] = 1 },
			atteso{"1 occorrenze con estremi non risolti", valutazione.GerarchiaStepParziale, false, true}},
		{"dati incompleti: gli scarti mancano", func(m map[string]any) { delete(m, "scarti") },
			atteso{"scarti non indicati", valutazione.GerarchiaStepParziale, false, true}},
		{"lo stato «valida» non rende completa una lettura con due radici", func(m map[string]any) {
			m["radici"] = []any{"#10", "#99999"}
			m["stato"] = worker.StrutturaValida
		}, atteso{"2 radici nel file: una distinta ne ha una", valutazione.GerarchiaStepParziale, false, true}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			variante(t, c.cambia)
			verifica(t, c.atteso)
		})
	}
	// e l'uscita vera, rimessa, torna completa: le varianti non hanno lasciato niente
	t.Run("uscita vera rimessa: di nuovo completa", func(t *testing.T) {
		variante(t, func(map[string]any) {})
		verifica(t, atteso{motivo: "", gerarchia: valutazione.GerarchiaStepRiconosciuta, daRileggere: false, diagnostica: true})
	})
}

// strutturaACME: i dettagli di uno STEP nella forma del worker, radice #10 e due figli, per le prove della riaccodatura.
// forma: "oggi" (versione 3 e diagnostica 1, il worker corretto), "gp3" (versione 4 con i campi della diagnostica ma
// senza il numero, il worker di GP-3), "prima" (versione 3 senza diagnostica, un worker di prima di GP-3), "v2" (la
// forma senza gli scarti in numeri, un worker di prima di B8.5).
func strutturaACME(t *testing.T, codici [3]string, forma string) json.RawMessage {
	t.Helper()
	diagnostica := forma == "oggi" || forma == "gp3"
	nodi := []map[string]any{}
	for i, c := range codici {
		n := map[string]any{"chiave": "#" + strconv.Itoa(10*(i+1)), "id_grezzo": c, "nome_grezzo": c, "descrizione_grezza": "ACME",
			"rev_grezza": "", "evidenza": map[string]any{"entita": "PRODUCT", "riga": "#" + strconv.Itoa(10*(i+1))}}
		if diagnostica {
			n["geometria"] = map[string]any{"solidi": map[bool]int{true: 0, false: 1}[i == 0], "superfici": 0, "curve": 0, "mappati": 0,
				"rappresentazioni": 1, "vuota": i == 0}
		}
		nodi = append(nodi, n)
	}
	relazioni := []map[string]any{}
	for _, figlio := range []string{"#20", "#30"} {
		r := map[string]any{"padre": "#10", "figlio": figlio, "qta": 1, "evidenza": map[string]any{"entita": "NEXT_ASSEMBLY_USAGE_OCCURRENCE"}}
		if diagnostica {
			r["istanze"] = []map[string]any{{"rif": "#9" + figlio[1:], "id": "NAUO", "nome": figlio}}
		}
		relazioni = append(relazioni, r)
	}
	s := map[string]any{"versione": 3, "schema": "AP214", "radici": []string{"#10"}, "nodi": nodi, "relazioni": relazioni, "avvisi": []string{},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
		"limiti": map[string]any{"nodi_max": 2000, "occorrenze_max": 50000, "tempo_max_s": 120.0, "troncato": false, "motivo": ""}}
	switch forma {
	case "oggi":
		s["versione_diagnostica"], s["stato"], s["motivo_stato"] = worker.VersioneDiagnostica, worker.StrutturaValida, ""
	case "gp3":
		s["versione"], s["stato"], s["motivo_stato"] = worker.VersioneStrutturaGP3, worker.StrutturaValida, ""
	case "prima":
	case "v2":
		s["versione"] = 2
		delete(s, "scarti")
	default:
		t.Fatalf("forma %q sconosciuta", forma)
	}
	raw, err := json.Marshal(map[string]any{"struttura": s, "product_step": codici[0]})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Le due righe del log del server per una struttura STEP scritta da un worker non aggiornato (workerapi.go,
// applicaRisultato): l'inizio del messaggio, che le distingue. Tutte e due dicono anche «worker non aggiornato».
const (
	avvisoForma       = "struttura STEP in una forma che il server non accetta"
	avvisoDiagnostica = "struttura STEP senza diagnostica"
)

// analisiPendenti: i job di analisi con una chiave, pronti o in corso, con la loro priorità (i risultati scritti dal
// banco con b.analisi non ne lasciano).
func analisiPendenti(t *testing.T, b *bancoLetture) map[string]int16 {
	t.Helper()
	righe, err := b.pool.Query(b.ctx, `SELECT chiave_idempotenza, priorita FROM job WHERE tipo = 'analizza_allegato'
		AND chiave_idempotenza IS NOT NULL AND stato IN ('pronto', 'in_corso')`)
	if err != nil {
		t.Fatal(err)
	}
	defer righe.Close()
	out := map[string]int16{}
	for righe.Next() {
		var k string
		var p int16
		if err := righe.Scan(&k, &p); err != nil {
			t.Fatal(err)
		}
		out[k] = p
	}
	if err := righe.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// rispondeAlJob: il worker workerID prende il job di analisi pendente con quella chiave, ne applica il risultato r (con
// l'allegato, la versione e la configurazione del job, come farebbe il worker) attraverso applicaRisultato in una
// transazione, come /result, e lo chiude come /complete. Restituisce il payload del job.
func rispondeAlJob(t *testing.T, b *bancoLetture, chiave, workerID string, r worker.RisultatoAnalisi) worker.PayloadAnalizzaAllegato {
	t.Helper()
	var jobID int64
	if err := b.pool.QueryRow(b.ctx, `UPDATE job SET stato = 'in_corso', worker_id = $2 WHERE chiave_idempotenza = $1
		AND stato IN ('pronto', 'in_corso') RETURNING job_id`, chiave, workerID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	var p worker.PayloadAnalizzaAllegato
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatal(err)
	}
	r.AllegatoID, r.VersioneAnalizzatore, r.HashConfigurazione = p.AllegatoID, p.VersioneAnalizzatore, p.HashConfigurazione
	dati, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.s.applicaRisultato(b.ctx, db.New(tx), &j, dati, nil); err != nil {
		_ = tx.Rollback(b.ctx)
		t.Fatalf("il risultato non si applica: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1`, jobID)
	return p
}

func TestLS01RianalizzaRiaccodaSoloGliStepLettiInUnaFormaNonAccettata(t *testing.T) {
	b := nuovoBancoLetture(t)
	var registro bytes.Buffer
	b.s.Log = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelWarn}))
	// le righe del log per la forma non accettata e per la forma 3 senza diagnostica, contate a parte
	avvisi := func() int { return strings.Count(registro.String(), avvisoForma) }
	avvisiDiagnostica := func() int { return strings.Count(registro.String(), avvisoDiagnostica) }
	an := coda.Analizzatore{Versione: 4, Parametri: map[string]any{}}
	b.s.Analizzatore = an
	thread := b.rfq(`{}`)
	b.esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, an.Versione, an.Hash())
	utente := b.id(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('L1', 'Prova LS-01', 'Tecnico') RETURNING utente_id`)
	cartella := t.TempDir()

	// uno STEP per forma dei fatti, ciascuno con il suo contenuto; i fatti arrivano dalla strada normale del risultato
	step := func(codici [3]string, forma string, inStaging bool) db.Allegato {
		t.Helper()
		contenuto := []byte("ISO-10303-21; LS-01 " + codici[0] + " " + forma)
		sha := shaDi(contenuto)
		a := b.allegato(b.mail(thread), codici[0]+".stp", sha, int64(len(contenuto)))
		var percorso any
		if inStaging {
			p := filepath.Join(cartella, sha+".stp")
			if err := os.WriteFile(p, contenuto, 0o600); err != nil {
				t.Fatal(err)
			}
			percorso = p
		}
		b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'cad_3d', 60, 'estensione')`,
			a.AllegatoID, thread)
		b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "cad_3d", Codice: codici[0], Confidenza: 95, Fonte: "step",
			Dettagli: strutturaACME(t, codici, forma)})
		b.esegui(`UPDATE allegato SET stato = 'analizzato', path_staging = $2 WHERE allegato_id = $1`, a.AllegatoID, percorso)
		a, err := b.q.GetAllegato(b.ctx, a.AllegatoID)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	gp3 := step([3]string{"7120600", "7120601", "7120602"}, "gp3", true)         // da riaccodare
	gp3Sparito := step([3]string{"7120700", "7120701", "7120702"}, "gp3", false) // da riaccodare, ma non più in staging
	prima := step([3]string{"7120800", "7120801", "7120802"}, "prima", true)     // v3 senza diagnostica: completa, resta
	oggi := step([3]string{"7120900", "7120901", "7120902"}, "oggi", true)       // il worker corretto: resta
	if n := avvisi(); n != 2 {
		t.Errorf("il server avvisa nel log per le due v4 arrivate: %d avvisi\n%s", n, registro.String())
	}
	// e una riga a parte per la v3 senza diagnostica (un worker di prima di GP-3): la lettura vale, la diagnostica manca
	if n := avvisiDiagnostica(); n != 1 || strings.Count(registro.String(), "worker non aggiornato") != 3 {
		t.Errorf("il server avvisa nel log per la v3 senza diagnostica: %d avvisi\n%s", n, registro.String())
	}
	// un PDF letto con il testo di oggi: il gesto non lo tocca
	pdf := b.allegato(b.mail(thread), "7120900.pdf", shaDi([]byte("%PDF-1.4 LS-01")), 14)
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', 60, 'nome_file')`,
		pdf.AllegatoID, thread)
	b.analisi(pdf, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120900", Confidenza: 95, Fonte: "cartiglio", Dettagli: dettagliDisegno(t)})
	filePdf := filepath.Join(cartella, pdf.Sha256.String+".pdf")
	if err := os.WriteFile(filePdf, []byte("%PDF-1.4 LS-01"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.esegui(`UPDATE allegato SET stato = 'analizzato', path_staging = $2 WHERE allegato_id = $1`, pdf.AllegatoID, filePdf)

	fattiDi := func(a db.Allegato) db.AnalisiFatti {
		t.Helper()
		f, err := b.q.GetAnalisiFatti(b.ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String, VersioneAnalizzatore: int16(an.Versione),
			HashConfigurazione: an.Hash()})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	for _, c := range []struct {
		a      db.Allegato
		forma  string
		atteso bool
	}{{gp3, "gp3", true}, {gp3Sparito, "gp3", true}, {prima, "prima", false}, {oggi, "oggi", false}} {
		if got := worker.StrutturaDaRileggere(fattiDi(c.a).Fatti); got != c.atteso {
			t.Errorf("%s (%s): da rileggere %v, atteso %v", c.a.NomeFile, c.forma, got, c.atteso)
		}
	}
	pendenti := func() map[string]int16 { return analisiPendenti(t, b) }
	chiave := coda.ChiaveAnalisi(gp3.Sha256.String, an)

	// Il finito 7120600 con lo STEP v4 autorizzato e una deroga strutturale data su quella lettura.
	finito := b.id(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120600', 'finito', 'manuale', $2)
		RETURNING componente_id`, thread, utente)
	testutil.AutorizzaStep(t, b.pool, thread, finito, gp3.AllegatoID, utente)
	if _, err := fascicolo.ConcediDerogaStruttura(b.ctx, b.q, thread, finito, utente, "prova LS-01: la distinta la controlliamo a mano"); err != nil {
		t.Fatalf("la deroga sulla lettura v4: %v", err)
	}
	vista := func() (esito, motivo, deroga string) {
		t.Helper()
		if err := b.pool.QueryRow(b.ctx, `SELECT esito, coalesce(motivo_parziale, ''), coalesce(deroga_struttura_id::text, '')
			FROM v_step_prodotto WHERE componente_id = $1`, finito).Scan(&esito, &motivo, &deroga); err != nil {
			t.Fatal(err)
		}
		return
	}
	if e, m, d := vista(); e != "presente_parziale" || !strings.HasPrefix(m, "struttura v4:") || d == "" {
		t.Fatalf("prima della rilettura: esito %s, motivo %q, deroga %q", e, m, d)
	}
	radice := func() string {
		return b.testo(`SELECT concat_ws(':', stato, deciso_da, componente_id) FROM componente_proposta
			WHERE allegato_id = $1 AND chiave = '#10'`, gp3.AllegatoID)
	}
	radicePrima := radice()

	// La preparazione della pagina non riaccoda niente: nessuna rianalisi parte da sola.
	if ri, err := fascicolo.AccodaAnalisiMancantiDaSola(b.ctx, b.q, thread, an, 10); err != nil || ri.Accodati != 0 || len(pendenti()) != 0 {
		t.Fatalf("la preparazione accoda da sola: %+v, pendenti %v (%v)", ri, pendenti(), err)
	}

	rianalizza := func(max int) fascicolo.Rianalisi {
		t.Helper()
		ri, err := fascicolo.RianalizzaRfq(b.ctx, b.q, thread, an, max)
		if err != nil {
			t.Fatal(err)
		}
		return ri
	}
	// «Rianalizza»: lo STEP v4 in staging si riaccoda, con la priorità dei file compatibili con un prodotto; quello senza
	// staging si conta; le v3, con o senza diagnostica, e il PDF no.
	ri := rianalizza(10)
	if ri.StepRiaccodati != 1 || ri.StepSenzaStaging != 1 || ri.StepGiaInCoda != 0 || ri.StepRimandati != 0 || ri.Accodati != 0 ||
		ri.PdfAccodati != 0 || ri.Riletti != 4 {
		t.Errorf("prima rianalisi: %+v", ri)
	}
	if p := pendenti(); len(p) != 1 || p[chiave] != coda.PrioritaAnalisiCompatibile {
		t.Errorf("pendenti dopo la rianalisi: %v, atteso solo %s con priorità %d", p, chiave, coda.PrioritaAnalisiCompatibile)
	}
	// ripetuta: già in coda, nessun job in più; con il limite a zero: rimandato
	if ri := rianalizza(10); ri.StepRiaccodati != 0 || ri.StepGiaInCoda != 1 || len(pendenti()) != 1 {
		t.Errorf("rianalisi ripetuta: %+v, pendenti %v", ri, pendenti())
	}
	if ri := rianalizza(0); ri.StepRiaccodati != 0 || ri.StepRimandati != 1 || len(pendenti()) != 1 {
		t.Errorf("rianalisi con il limite a zero: %+v", ri)
	}

	// risponde: il worker dato prende il job accodato, ne applica il risultato con i dettagli dati, e lo chiude come farebbe
	// /complete.
	risponde := func(workerID string, dett json.RawMessage) {
		t.Helper()
		p := rispondeAlJob(t, b, chiave, workerID, worker.RisultatoAnalisi{TipoProposto: "cad_3d", Codice: "7120600", Confidenza: 95,
			Fonte: "step", Dettagli: dett})
		if p.AllegatoID != gp3.AllegatoID || p.VersioneAnalizzatore != an.Versione {
			t.Fatalf("payload del job: %+v", p)
		}
	}

	// Flotta mista: il job accodato lo prende una postazione rimasta al worker di GP-3, che risponde di nuovo con la v4.
	// I fatti si riscrivono (la deroga data sulla lettura di prima decade comunque: è un'altra analisi), restano
	// parziali, il log lo dice, e niente si riaccoda da solo: solo una nuova pressione del gesto.
	primaDellaRilettura := fattiDi(gp3).CalcolatoIl
	time.Sleep(10 * time.Millisecond) // calcolato_il è now() della transazione: due letture devono avere due istanti
	risponde("analisi@PC-GP3", strutturaACME(t, [3]string{"7120600", "7120601", "7120602"}, "gp3"))
	if f := fattiDi(gp3); !worker.StrutturaDaRileggere(f.Fatti) || !f.CalcolatoIl.After(primaDellaRilettura) {
		t.Errorf("la v4 riscritta da un worker non aggiornato va riletta (calcolato_il %v, prima %v)", f.CalcolatoIl, primaDellaRilettura)
	}
	if e, m, d := vista(); e != "presente_parziale" || !strings.HasPrefix(m, "struttura v4:") || d != "" {
		t.Errorf("la v4 riscritta: esito %s, motivo %q, deroga %q (decade: è un'altra analisi)", e, m, d)
	}
	// il log dice quale postazione è rimasta indietro: è la contromisura della flotta mista
	if n := avvisi(); n != 3 || !strings.Contains(registro.String(), "worker=analisi@PC-GP3") {
		t.Errorf("il server avvisa della v4 riscritta, con il worker: %d avvisi\n%s", n, registro.String())
	}
	if ri, err := fascicolo.AccodaAnalisiMancantiDaSola(b.ctx, b.q, thread, an, 10); err != nil || ri.Accodati != 0 || len(pendenti()) != 0 {
		t.Errorf("la v4 riscritta si riaccoda da sola: %+v, pendenti %v (%v)", ri, pendenti(), err)
	}
	if ri := rianalizza(10); ri.StepRiaccodati != 1 || len(pendenti()) != 1 {
		t.Errorf("il gesto ripremuto riaccoda la v4 riscritta: %+v, pendenti %v", ri, pendenti())
	}

	// Il worker corretto risponde: i fatti si riscrivono sotto la stessa chiave nella forma accettata, la lettura è
	// completa, la radice decisa da una persona resta com'era, e il gesto non ha più niente da riaccodare.
	primaDellaRilettura = fattiDi(gp3).CalcolatoIl
	time.Sleep(10 * time.Millisecond)
	risponde("analisi@PC-NUOVO", strutturaACME(t, [3]string{"7120600", "7120601", "7120602"}, "oggi"))
	f := fattiDi(gp3)
	st, ok := worker.DecodificaStruttura(f.Fatti)
	if !ok || st.Versione != worker.VersioneStruttura || st.VersioneDiagnostica != worker.VersioneDiagnostica || !f.CalcolatoIl.After(primaDellaRilettura) {
		t.Errorf("i fatti riletti: %+v, calcolato_il %v (prima %v)", st, f.CalcolatoIl, primaDellaRilettura)
	}
	if n := b.testo(`SELECT count(*)::text FROM analisi_fatti WHERE sha256 = $1`, gp3.Sha256.String); n != "1" {
		t.Errorf("i fatti si riscrivono sotto la loro chiave: %s righe", n)
	}
	if worker.StrutturaDaRileggere(f.Fatti) {
		t.Error("i fatti riletti sono ancora da rileggere")
	}
	if e, m, d := vista(); e != "presente_analizzato" || m != "" || d != "" {
		t.Errorf("dopo la rilettura: esito %s, motivo %q, deroga %q", e, m, d)
	}
	if r := radice(); r != radicePrima {
		t.Errorf("la radice decisa da una persona è cambiata: %s (prima %s)", r, radicePrima)
	}
	if n, d := avvisi(), avvisiDiagnostica(); n != 3 || d != 1 {
		t.Errorf("la struttura del worker corretto non dà avvisi: %d per la forma, %d per la diagnostica", n, d)
	}
	if ri := rianalizza(10); ri.StepRiaccodati != 0 || ri.StepGiaInCoda != 0 || ri.StepSenzaStaging != 1 || len(pendenti()) != 0 {
		t.Errorf("dopo la rilettura non c'è più niente da riaccodare (resta lo STEP senza staging): %+v, pendenti %v", ri, pendenti())
	}
	// le v3, con o senza diagnostica, non sono mai state riaccodate
	for _, a := range []db.Allegato{prima, oggi} {
		if n := b.testo(`SELECT count(*)::text FROM job WHERE chiave_idempotenza = $1`, coda.ChiaveAnalisi(a.Sha256.String, an)); n != "0" {
			t.Errorf("%s è stato riaccodato: %s job", a.NomeFile, n)
		}
	}

	// «Riscarica» sullo STEP v4 non più in staging (correzioni dei verificatori, punto 1): il file torna nello staging e
	// il risultato dello stage passa per dopoStaging, come sempre. Da solo non lo rilegge: i fatti v4 stanno sotto la
	// chiave corrente, AccodaAnalisi si ferma a loro e applicaFattiEsistenti li riapplica com'erano. Lo riaccoda un nuovo
	// «Rianalizza», che lo trova nello staging: per questo la frase del gesto dice «Riscarica, poi di nuovo Rianalizza».
	riscaricato := filepath.Join(cartella, gp3Sparito.Sha256.String+".stp")
	if err := os.WriteFile(riscaricato, []byte("ISO-10303-21; LS-01 7120700 gp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.esegui(`UPDATE allegato SET path_staging = $2, stato = 'in_staging' WHERE allegato_id = $1`, gp3Sparito.AllegatoID, riscaricato)
	b.scende(gp3Sparito)
	if p := pendenti(); len(p) != 0 {
		t.Errorf("«Riscarica» da solo ha accodato un'analisi: %v", p)
	}
	if f := fattiDi(gp3Sparito); !worker.StrutturaDaRileggere(f.Fatti) {
		t.Errorf("dopo «Riscarica» i fatti dovevano restare quelli v4: %s", f.Fatti)
	}
	if s := b.testo(`SELECT stato::text FROM allegato WHERE allegato_id = $1`, gp3Sparito.AllegatoID); s != string(db.StatoAllegatoAnalizzato) {
		t.Errorf("dopo «Riscarica» l'allegato è %s: applicaFattiEsistenti doveva riapplicare i fatti presenti", s)
	}
	chiaveSparito := coda.ChiaveAnalisi(gp3Sparito.Sha256.String, an)
	if ri := rianalizza(10); ri.StepRiaccodati != 1 || ri.StepSenzaStaging != 0 || ri.StepGiaInCoda != 0 {
		t.Errorf("«Rianalizza» dopo «Riscarica»: %+v", ri)
	}
	if p := pendenti(); len(p) != 1 || p[chiaveSparito] != coda.PrioritaAnalisi {
		t.Errorf("pendenti dopo «Riscarica» e «Rianalizza»: %v, atteso solo %s", p, chiaveSparito)
	}
}

// TestLS01RianalizzaPerContenutoConIlLimiteCondiviso (P-10b; correzioni dei verificatori di LS-01, punto 2): quello che
// P-10 non fissava della riaccodatura degli STEP da rileggere (fascicolo.AccodaStepDaRileggere, dentro RianalizzaRfq),
// con STEP sintetici letti in v4:
//   - una riaccodatura per CONTENUTO: lo stesso file in due mail si accoda, si conta e si rimanda una volta sola;
//   - la copia che va al worker è quella nello staging, anche quando la prima copia arrivata non c'è più;
//   - con il limite, prima i nomi compatibili con un prodotto della RFQ, qualunque sia l'ordine di arrivo;
//   - il limite è uno per tutto il gesto: le analisi mancanti lo usano per prime, gli STEP da rileggere con quello che
//     resta, i PDF da rileggere con quello che resta dopo di loro.
//
// Uccide i mutanti MV2-MV5 della verifica: il PDF che dimentica gli STEP nel limite, gli STEP con tutto il limite, la
// riaccodatura senza prima i compatibili, una volta per allegato e non per contenuto.
func TestLS01RianalizzaPerContenutoConIlLimiteCondiviso(t *testing.T) {
	b := nuovoBancoLetture(t)
	an := coda.Analizzatore{Versione: 4, Parametri: map[string]any{}}
	b.s.Analizzatore = an
	thread := b.rfq(`{}`)
	utente := b.id(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('L1', 'Prova LS-01', 'Tecnico') RETURNING utente_id`)
	// il prodotto della RFQ: 7121000.stp è compatibile con lui, gli altri nomi no
	b.esegui(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7121000', 'finito', 'manuale', $2)`,
		thread, utente)
	cartella := t.TempDir()
	arrivo := 0
	// copia: un allegato della RFQ con quel contenuto, in una mail sua, arrivato dopo i precedenti (le query degli STEP li
	// ordinano per ricevuto_il), nello staging o no; uno STEP porta anche la proposta del documento, come dopo lo stage
	copia := func(nome string, contenuto []byte, nelloStaging bool) db.Allegato {
		t.Helper()
		arrivo++
		a := b.allegato(b.mail(thread), nome, shaDi(contenuto), int64(len(contenuto)))
		var percorso any
		if nelloStaging {
			p := filepath.Join(cartella, strconv.Itoa(arrivo)+"_"+nome)
			if err := os.WriteFile(p, contenuto, 0o600); err != nil {
				t.Fatal(err)
			}
			percorso = p
		}
		b.esegui(`UPDATE allegato SET stato = 'analizzato', path_staging = $2, ricevuto_il = $3 WHERE allegato_id = $1`,
			a.AllegatoID, percorso, time.Date(2026, 10, 17, 6, arrivo, 0, 0, time.UTC))
		if strings.HasSuffix(nome, ".stp") {
			b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'cad_3d', 60, 'estensione')`,
				a.AllegatoID, thread)
		}
		a, err := b.q.GetAllegato(b.ctx, a.AllegatoID)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	// lettoInV4: i fatti del worker di GP-3 per il contenuto dell'allegato, dalla strada normale del risultato
	lettoInV4 := func(a db.Allegato, codici [3]string) {
		t.Helper()
		b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "cad_3d", Codice: codici[0], Confidenza: 95, Fonte: "step",
			Dettagli: strutturaACME(t, codici, "gp3")})
	}
	contenutoA := []byte("ISO-10303-21; LS-01 P-10b 7121100")
	primaCopia := copia("7121100.stp", contenutoA, false)  // A, arrivato per primo: non più nello staging
	secondaCopia := copia("7121100.stp", contenutoA, true) // A di nuovo, in un'altra mail: nello staging
	altro := copia("7121200.stp", []byte("ISO-10303-21; LS-01 P-10b 7121200"), true)
	compatibile := copia("7121000.stp", []byte("ISO-10303-21; LS-01 P-10b 7121000"), true) // arrivato dopo A e l'altro
	mancante := copia("7121300.stp", []byte("ISO-10303-21; LS-01 P-10b 7121300"), true)    // nessun fatto: analisi mancante
	pdf := copia("7121900.pdf", []byte("%PDF-1.4 LS-01 P-10b"), true)                      // senza i fatti correnti
	lettoInV4(primaCopia, [3]string{"7121100", "7121101", "7121102"})
	lettoInV4(altro, [3]string{"7121200", "7121201", "7121202"})
	lettoInV4(compatibile, [3]string{"7121000", "7121001", "7121002"})
	if n := b.testo(`SELECT count(*)::text FROM analisi_fatti`); n != "3" {
		t.Fatalf("fatti scritti: %s, attesi 3 (uno per contenuto letto)", n)
	}

	if primaCopia.Sha256.String != secondaCopia.Sha256.String || primaCopia.AllegatoID == secondaCopia.AllegatoID {
		t.Fatalf("le due copie di A: due allegati con un contenuto solo, invece %+v e %+v", primaCopia, secondaCopia)
	}
	chiaveA, chiaveAltro := coda.ChiaveAnalisi(primaCopia.Sha256.String, an), coda.ChiaveAnalisi(altro.Sha256.String, an)
	chiaveComp, chiaveMancante := coda.ChiaveAnalisi(compatibile.Sha256.String, an), coda.ChiaveAnalisi(mancante.Sha256.String, an)
	chiavePdf := coda.ChiaveAnalisi(pdf.Sha256.String, an)
	rianalizza := func(max int) fascicolo.Rianalisi {
		t.Helper()
		ri, err := fascicolo.RianalizzaRfq(b.ctx, b.q, thread, an, max)
		if err != nil {
			t.Fatal(err)
		}
		ri.Proposte = 0 // le righe di proposta riscritte dalla rilettura non sono di questa prova
		return ri
	}
	uguali := func(x, y map[string]int16) bool {
		if len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if w, c := y[k]; !c || w != v {
				return false
			}
		}
		return true
	}

	// Primo giro, limite 2. L'analisi mancante ne usa uno; dei tre STEP da rileggere passa solo il compatibile, arrivato
	// per ultimo; A (due copie, un contenuto) e l'altro si rimandano una volta ciascuno; per il PDF non resta niente.
	ri := rianalizza(2)
	if atteso := (fascicolo.Rianalisi{Riletti: 4, Accodati: 1, StepRiaccodati: 1, StepRimandati: 2, PdfRimandati: 1}); ri != atteso {
		t.Errorf("primo giro con il limite 2:\n  %+v\natteso\n  %+v", ri, atteso)
	}
	if p, atteso := analisiPendenti(t, b), map[string]int16{chiaveMancante: coda.PrioritaAnalisi, chiaveComp: coda.PrioritaAnalisiCompatibile}; !uguali(p, atteso) {
		t.Errorf("pendenti dopo il primo giro: %v, attesi l'analisi mancante e il compatibile %v", p, atteso)
	}

	// Secondo giro, limite 10: l'analisi mancante e il compatibile sono già in coda; A si accoda una volta, con la copia
	// nello staging; l'altro si accoda; il PDF ha il limite che resta.
	ri = rianalizza(10)
	if atteso := (fascicolo.Rianalisi{Riletti: 4, GiaInCoda: 1, StepRiaccodati: 2, StepGiaInCoda: 1, PdfAccodati: 1}); ri != atteso {
		t.Errorf("secondo giro con il limite 10:\n  %+v\natteso\n  %+v", ri, atteso)
	}
	atteso := map[string]int16{chiaveMancante: coda.PrioritaAnalisi, chiaveComp: coda.PrioritaAnalisiCompatibile,
		chiaveA: coda.PrioritaAnalisi, chiaveAltro: coda.PrioritaAnalisi, chiavePdf: coda.PrioritaAnalisi}
	if p := analisiPendenti(t, b); !uguali(p, atteso) {
		t.Errorf("pendenti dopo il secondo giro: %v, attesi %v", p, atteso)
	}
	if n := b.testo(`SELECT count(*)::text FROM job WHERE chiave_idempotenza = $1`, chiaveA); n != "1" {
		t.Errorf("A, un contenuto in due mail: %s job, atteso uno", n)
	}
	var payload json.RawMessage
	if err := b.pool.QueryRow(b.ctx, `SELECT payload FROM job WHERE chiave_idempotenza = $1`, chiaveA).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var p worker.PayloadAnalizzaAllegato
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.AllegatoID != secondaCopia.AllegatoID {
		t.Errorf("A va al worker con la copia %s, attesa quella nello staging %s (non la prima, %s)", p.AllegatoID,
			secondaCopia.AllegatoID, primaCopia.AllegatoID)
	}

	// Terzo giro: tutto è già in coda, e ogni contenuto si conta una volta.
	if ri := rianalizza(10); ri != (fascicolo.Rianalisi{Riletti: 4, GiaInCoda: 1, StepGiaInCoda: 3, PdfGiaInCoda: 1}) {
		t.Errorf("terzo giro: %+v", ri)
	}
	if p := analisiPendenti(t, b); !uguali(p, atteso) {
		t.Errorf("pendenti dopo il terzo giro: %v, attesi gli stessi %v", p, atteso)
	}
}

// TestLS01IlLogNominaIlWorkerDiOgniStrutturaNonAggiornata (correzioni dei verificatori di LS-01, punto 3; flotta mista):
// il server scrive nel log, con il worker, il file e l'allegato, ogni struttura STEP che arriva da un worker non
// aggiornato. Due righe distinte, tutte e due con «worker non aggiornato»:
//   - la forma che il server non accetta (la v4 di GP-3, una v2 di prima di B8.5): la lettura resta parziale, e
//     «Rianalizza» la rilegge;
//   - la forma 3 senza versione_diagnostica (un worker di prima di GP-3): la lettura vale e non si rilegge, ma la
//     diagnostica (geometria, istanze, stato) manca, e senza la riga si perderebbe in silenzio.
//
// La forma del worker di oggi e i fatti senza struttura (un PDF) non scrivono niente.
func TestLS01IlLogNominaIlWorkerDiOgniStrutturaNonAggiornata(t *testing.T) {
	b := nuovoBancoLetture(t)
	var registro bytes.Buffer
	b.s.Log = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelWarn}))
	an := coda.Analizzatore{Versione: 4, Parametri: map[string]any{}}
	b.s.Analizzatore = an
	thread := b.rfq(`{}`)
	step := func(codice, forma string) worker.RisultatoAnalisi {
		return worker.RisultatoAnalisi{TipoProposto: "cad_3d", Codice: codice, Confidenza: 95, Fonte: "step",
			Dettagli: strutturaACME(t, [3]string{codice, codice + "1", codice + "2"}, forma)}
	}
	casi := []struct {
		nome, file, workerID string
		risultato            worker.RisultatoAnalisi
		forma, diagnostica   int  // le righe attese: forma non accettata, forma 3 senza diagnostica
		daRileggere          bool // per uno STEP: worker.StrutturaDaRileggere sui fatti scritti
	}{
		{"il worker di oggi: niente", "7121400.stp", "analisi@PC-NUOVO", step("712140", "oggi"), 0, 0, false},
		{"un worker di prima di GP-3: la riga della diagnostica", "7121500.stp", "analisi@PC-PRIMA", step("712150", "prima"), 0, 1, false},
		{"il worker di GP-3: la riga della forma", "7121600.stp", "analisi@PC-GP3", step("712160", "gp3"), 1, 0, true},
		{"un worker di prima di B8.5, la v2: la riga della forma", "7121700.stp", "analisi@PC-B8", step("712170", "v2"), 1, 0, true},
		{"un PDF, senza struttura: niente", "7121800.pdf", "analisi@PC-PRIMA", worker.RisultatoAnalisi{TipoProposto: "disegno_2d",
			Codice: "7121800", Confidenza: 95, Fonte: "cartiglio", Dettagli: dettagliDisegno(t)}, 0, 0, false},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			contenuto := []byte("LS-01 log " + c.file)
			a := b.allegato(b.mail(thread), c.file, shaDi(contenuto), int64(len(contenuto)))
			tipo := map[bool]string{true: "cad_3d", false: "disegno_2d"}[strings.HasSuffix(c.file, ".stp")]
			b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, $3, 60, 'estensione')`,
				a.AllegatoID, thread, tipo)
			if _, err := coda.RiaccodaAnalisi(b.ctx, b.q, a, uuid.NullUUID{UUID: thread, Valid: true}, an); err != nil {
				t.Fatal(err)
			}
			inizio := registro.Len()
			rispondeAlJob(t, b, coda.ChiaveAnalisi(a.Sha256.String, an), c.workerID, c.risultato)
			righe := registro.String()[inizio:]
			forma, diagnostica := strings.Count(righe, avvisoForma), strings.Count(righe, avvisoDiagnostica)
			if forma != c.forma || diagnostica != c.diagnostica {
				t.Errorf("righe nel log: %d per la forma (attese %d), %d per la diagnostica (attese %d)\n%s", forma, c.forma,
					diagnostica, c.diagnostica, righe)
			}
			if c.forma+c.diagnostica > 0 {
				for _, deve := range []string{"worker non aggiornato", "worker=" + c.workerID, "file=" + c.file, "allegato=" + a.AllegatoID.String()} {
					if !strings.Contains(righe, deve) {
						t.Errorf("la riga non dice %q:\n%s", deve, righe)
					}
				}
			}
			if c.diagnostica > 0 && (!strings.Contains(righe, "versione_diagnostica=0") || !strings.Contains(righe, "attesa="+strconv.Itoa(worker.VersioneDiagnostica))) {
				t.Errorf("la riga della diagnostica non dice quale manca:\n%s", righe)
			}
			if strings.HasSuffix(c.file, ".stp") {
				f, err := b.q.GetAnalisiFatti(b.ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String, VersioneAnalizzatore: int16(an.Versione),
					HashConfigurazione: an.Hash()})
				if err != nil {
					t.Fatal(err)
				}
				if worker.StrutturaDaRileggere(f.Fatti) != c.daRileggere {
					t.Errorf("da rileggere %v, atteso %v", worker.StrutturaDaRileggere(f.Fatti), c.daRileggere)
				}
			}
		})
	}
}
