//go:build integrazione

// L4 — Smistamento G (addendum A5.4.8, U2; decisioni del 27/09 bis e ter): il gate definitivo sulla distinzione
// GUIDA/AUTORITA' di F5. F6 (il gate diviso fra logica e materializzazione) e' nata prima di F5: qui ogni
// condizione del gate si prova da sola, contro PostgreSQL vero, su una RFQ che senza quella condizione si
// congela davvero (la scena di partenza passa, e le prove che dicono «non ferma» congelano la V1).
//
// Le condizioni, una prova ciascuna:
//   - la guida (nodi e archi fuori dall'autorita' a qualunque profondita', file non autorizzati, discendenti
//     di un commerciale) non ferma e non conta;
//   - un figlio diretto aperto nell'autorita', anche di una delega, ferma;
//   - un aggancio automatico di prima dentro l'autorita' ferma, finche' una persona non lo conferma;
//   - le autorizzazioni da sistemare (conflitto, superata, incoerente, P7, delega con la catena rotta) fermano;
//   - una dichiarazione sospesa (commerciale, sospensione registrata non riattivata, delega sotto di loro) e'
//     un avviso;
//   - le copie sul NAS in coda o in errore e le anomalie non fermano e stanno nella materializzazione, anche
//     per un requisito bloccante (prova 114, parte materializzazione, sulla scena dello Smistamento);
//   - avvisi: l'assieme con i figli solo in guida, il file autorizzato letto in parte con le rimozioni sospese.
//
// La condizione L1 (i file tecnici non smistati) non e' qui: entra con la decisione (F11), e si prova dopo
// l'unione con F8/F9.

package fascicolo_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// casoGate: lo STEP completo del prodotto 7120001. I figli diretti sono 7120010 e 7120012 ×2; sotto 7120010 c'e'
// 7120011 ×2, e sotto 7120011 c'e' 7120013 (la guida profonda: terzo e quarto livello); sotto 7120012 c'e'
// 7120014.
func casoGate() fattiSTEP {
	return fattiSTEP{nodi: []string{"#1=7120001", "#2=7120010", "#3=7120012", "#4=7120011", "#5=7120013", "#6=7120014"},
		archi: []string{"#1>#2", "#1>#3*2", "#2>#4*2", "#4>#5", "#3>#6"}}
}

// scenaGate: la RFQ ACME del prodotto 7120001 con lo STEP completo autorizzato dal gesto di F5b; se figli, i
// figli diretti 7120010 e 7120012 accettati da una persona, con i loro archi, e i documenti bloccanti di tutti.
// Senza figli resta da decidere l'autorita' del file.
type scenaGate struct {
	prodotto, doc uuid.UUID
	a             db.Allegato
}

func (b *banco) scenaGate(figli bool) scenaGate {
	b.t.Helper()
	b.acme()
	var sc scenaGate
	sc.prodotto = b.componente("7120001", db.TipoComponenteFinito)
	sc.doc, sc.a = b.stepDelProdotto(sc.prodotto, "7120001", casoGate())
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.prodotto, Documento: sc.doc})
	if figli {
		b.decidi(sc.a, "#2", "#3")
	}
	b.documentiRichiesti()
	return sc
}

// decidi accetta i nodi del file, come una persona, e poi l'arco dal loro padre nel file.
func (b *banco) decidi(a db.Allegato, chiavi ...string) {
	b.t.Helper()
	if err := b.accettaIn(a, chiavi...); err != nil {
		b.t.Fatalf("accetta %v: %v", chiavi, err)
	}
	for _, k := range chiavi {
		padre := uno[string](b, `SELECT padre_chiave FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2 AND figlio_chiave = $3`,
			b.thread, a.AllegatoID, k)
		if _, err := b.accettaArco(a, padre, k); err != nil {
			b.t.Fatalf("accetta l'arco %s → %s: %v", padre, k, err)
		}
	}
}

// documentiRichiesti mette un documento, gia' scritto sul NAS, per ogni requisito bloccante che manca: il
// fascicolo e' completo, e il gate guarda solo la struttura. Le prove del NAS cambiano poi lo stato che serve.
func (b *banco) documentiRichiesti() {
	b.t.Helper()
	righe, err := b.p.Query(b.ctx, `SELECT componente_id, codice, tipo_documento::text FROM v_fascicolo
		WHERE thread_id = $1 AND bloccante AND esito = 'manca'`, b.thread)
	if err != nil {
		b.t.Fatal(err)
	}
	type req struct {
		comp       uuid.UUID
		codice, tp string
	}
	var mancano []req
	for righe.Next() {
		var r req
		if err := righe.Scan(&r.comp, &r.codice, &r.tp); err != nil {
			b.t.Fatal(err)
		}
		mancano = append(mancano, r)
	}
	righe.Close()
	for _, r := range mancano {
		ext := map[string]string{"cad_3d": "stp", "disegno_2d": "pdf", "sviluppo_dxf": "dxf"}[r.tp]
		if ext == "" {
			ext = "pdf"
		}
		b.documento(r.comp, db.TipoDocumento(r.tp), r.codice, ext)
	}
	b.esegui(`UPDATE documento SET stato_nas = 'scritto', scritto_il = now() WHERE thread_id = $1 AND stato_nas <> 'scritto'`, b.thread)
}

// passa dice che il gate passa, senza problemi, e restituisce il gate per gli avvisi.
func (b *banco) passa(cosa string) fascicolo.Gate {
	b.t.Helper()
	g := b.gateOra()
	if !g.Passa() {
		b.t.Fatalf("%s: il gate doveva passare, si ferma su %v", cosa, g.Problemi)
	}
	return g
}

// fermaSolo dice che il gate si ferma su un solo problema, con la frase, e che il congelamento si rifiuta con la
// stessa frase senza lasciare niente.
func (b *banco) fermaSolo(cosa, frase string) fascicolo.Gate {
	b.t.Helper()
	return b.fermaSu(cosa, frase, 1)
}

// fermaSu dice che il gate si ferma su quanti problemi, tutti con la frase (una condizione sola), e che il
// congelamento si rifiuta senza lasciare niente.
func (b *banco) fermaSu(cosa, frase string, quanti int) fascicolo.Gate {
	b.t.Helper()
	g := b.gateOra()
	if len(g.Problemi) != quanti {
		b.t.Fatalf("%s: attesi %d problemi con «%s», ottenuti %v", cosa, quanti, frase, g.Problemi)
	}
	for _, p := range g.Problemi {
		if !strings.Contains(p, frase) {
			b.t.Fatalf("%s: il problema %q non dice «%s»", cosa, p, frase)
		}
	}
	_, err := b.congela("prova")
	deveRifiutare(b.t, err, frase)
	if n := uno[int64](b, `SELECT count(*) FROM bom_versione`); n != 0 {
		b.t.Errorf("%s: un congelamento rifiutato ha lasciato %d versioni", cosa, n)
	}
	return g
}

// congelaOk congela la V1: il gate che passa non e' solo una frase.
func (b *banco) congelaOk(cosa string) {
	b.t.Helper()
	c, err := b.congela(cosa)
	if err != nil {
		b.t.Fatalf("%s: il congelamento doveva riuscire: %v", cosa, err)
	}
	if c.Versione.Numero != 1 {
		b.t.Fatalf("%s: congelata la V%d", cosa, c.Versione.Numero)
	}
}

func avvisiDi(g fascicolo.Gate) string { return strings.Join(g.Avvisi, " | ") }

// Smistamento G, condizione 1 (A5.4.5, A5.4.8, U2): la guida non ferma e non conta. Nel file autorizzato del
// prodotto restano aperti 7120011 e 7120013 (terzo e quarto livello) con i loro archi, e 7120014 sotto 7120012;
// 7120012 e' commerciale, e il suo STEP (7120012.stp, con due figli aperti) e' solo guida; un altro STEP con i
// codici della RFQ non e' autorizzato per nessuno. GateStrutturale conta zero, il gate passa e la V1 si congela.
// L'avviso dice l'assieme 7120010 con il figlio in guida, e non il commerciale: i suoi discendenti sono guida per
// scelta, non un assieme che aspetta il suo STEP.
func TestGateLaGuidaNonFermaENonConta(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaGate(true)
	f := b.codiceComp("7120012")
	b.cambiaTipoOk(f, db.TipoComponenteCommerciale)
	_, fa := b.stepDelProdotto(f, "7120012", fattiSTEP{nodi: []string{"#1=7120012", "#2=7120015", "#3=7120016"}, archi: []string{"#1>#2", "#1>#3*4"}})
	b.applica(fa, fattiSTEP{nodi: []string{"#1=7120012", "#2=7120015", "#3=7120016"}, archi: []string{"#1>#2", "#1>#3*4"}}.json())
	altro := b.allegatoStep("altro.stp", strings.Repeat("9a", 32))
	b.applica(altro, casoGate().json())
	b.documentiRichiesti()

	// la guida c'e', aperta, a ogni profondita'
	if got := uno[string](b, `SELECT string_agg(chiave || ':' || stato, ' ' ORDER BY chiave) FROM componente_proposta
		WHERE thread_id = $1 AND allegato_id = $2 AND chiave IN ('#4', '#5', '#6')`, b.thread, sc.a.AllegatoID); got != "#4:aperta #5:aperta #6:aperta" {
		t.Fatalf("la guida profonda del file del prodotto: %s", got)
	}
	if got := uno[string](b, `SELECT string_agg(padre_chiave || '>' || figlio_chiave || ':' || stato, ' ' ORDER BY padre_chiave) FROM relazione_proposta
		WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave IN ('#2', '#3', '#4')`, b.thread, sc.a.AllegatoID); got != "#2>#4:aperta #3>#6:aperta #4>#5:aperta" {
		t.Fatalf("gli archi della guida profonda: %s", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND stato = 'aperta' AND allegato_id IN ($2, $3)`,
		b.thread, fa.AllegatoID, altro.AllegatoID); n != 3+6 {
		t.Fatalf("le righe aperte dello STEP del commerciale e del file non autorizzato: %d", n)
	}
	if s := b.strutturali(); s != (db.GateStrutturaleRow{}) {
		t.Errorf("la guida non conta: %+v", s)
	}
	g := b.passa("con la guida aperta")
	av := avvisiDi(g)
	if !strings.Contains(av, "7120010 ha 1 figlio che resta guida") {
		t.Errorf("l'avviso dell'assieme con il figlio in guida: %s", av)
	}
	if strings.Contains(av, "7120012") {
		t.Errorf("i discendenti del commerciale non sono un avviso: %s", av)
	}
	b.congelaOk("con la guida aperta")
}

// Smistamento G, condizione 2 (A5.4.8, L2; Domanda 1 = B): un figlio diretto aperto nell'autorita' ferma, anche
// quello di una delega. Nel file del prodotto 7120012 e il suo arco sono da decidere; decisi, il gate passa. La
// delega dello stesso file a 7120010 porta nell'autorita' 7120011 (e il suo arco): il gate si ferma di nuovo, e
// dice anche l'assieme 7120011 che ha il figlio in guida; deciso 7120011, passa. 7120013, nipote della delega,
// resta guida e non conta.
func TestGateUnFiglioDirettoApertoNellAutoritaFerma(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaGate(false)
	b.decidi(sc.a, "#2")
	b.documentiRichiesti()
	b.fermaSolo("7120012 da decidere", "2 decisioni strutturali aperte negli STEP autorizzati (1 figli diretti, 1 relazioni, 0 rimozioni)")
	b.decidi(sc.a, "#3")
	b.documentiRichiesti()
	b.passa("figli diretti decisi")

	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.prodotto, Documento: sc.doc, Deleghe: []string{"#2"}})
	if s := b.strutturali(); s != (db.GateStrutturaleRow{NFigliDaDecidere: 1, NArchiDaDecidere: 1}) {
		t.Errorf("la delega porta 7120011 nell'autorita': %+v", s)
	}
	b.fermaSolo("7120011 da decidere nella delega", "2 decisioni strutturali aperte negli STEP autorizzati (1 figli diretti, 1 relazioni, 0 rimozioni)")
	b.decidi(sc.a, "#4")
	b.documentiRichiesti()
	if got := b.statoNodo(sc.a, "#5"); got != "aperta - -" {
		t.Errorf("7120013, nipote della delega, resta guida aperta: %s", got)
	}
	g := b.passa("la delega decisa")
	if av := avvisiDi(g); !strings.Contains(av, "7120011 ha 1 figlio che resta guida") || strings.Contains(av, "7120010 ha") {
		t.Errorf("l'avviso passa da 7120010 (delegato) a 7120011: %s", av)
	}
	b.congelaOk("la delega decisa")
}

// Smistamento G, condizione 3 (A5.4.8, L2; A5.15): un aggancio automatico di prima dello Smistamento dentro
// l'autorita' (un duplicato senza chi l'ha deciso, con l'arco automatico) ferma: non e' una decisione. Lo stesso
// aggancio nella guida no. Confermato da una persona, il gate passa, e l'arco automatico non conta piu'.
func TestGateUnAgganciatoPerCodiceDiPrimaNellAutoritaFerma(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaGate(false)
	b.decidi(sc.a, "#2")
	// la forma di prima: 7120012 nella working con l'arco, e la riga agganciata per codice da un automatismo
	f := b.componente("7120012", db.TipoComponenteSciolto)
	b.arco(sc.prodotto, f, 2)
	b.esegui(`UPDATE componente_proposta SET stato = 'duplicato', componente_id = $3, deciso_il = now()
		WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#3'`, b.thread, sc.a.AllegatoID, f)
	b.esegui(`UPDATE relazione_proposta SET stato = 'duplicato', deciso_il = now()
		WHERE thread_id = $1 AND allegato_id = $2 AND figlio_chiave = '#3'`, b.thread, sc.a.AllegatoID)
	// lo stesso nella guida: 7120011 sotto 7120010, agganciato per codice
	n := b.componente("7120011", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente_proposta SET stato = 'duplicato', componente_id = $3, deciso_il = now()
		WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#4'`, b.thread, sc.a.AllegatoID, n)
	b.documentiRichiesti()
	if s := b.strutturali(); s != (db.GateStrutturaleRow{NFigliDaDecidere: 1, NArchiDaDecidere: 1}) {
		t.Errorf("l'aggancio di prima nell'autorita' conta, quello nella guida no: %+v", s)
	}
	b.fermaSolo("l'aggancio di prima da confermare", "2 decisioni strutturali aperte negli STEP autorizzati (1 figli diretti, 1 relazioni, 0 rimozioni)")

	msg, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.nodoIn(sc.a, "#3"), b.utente, "")
	})
	ok(t, err)
	if !strings.Contains(msg, "confermato") {
		t.Errorf("la conferma di una persona: %q", msg)
	}
	if got := b.statoNodo(sc.a, "#3"); got != "duplicato 7120012 persona" {
		t.Errorf("confermato da una persona: %s", got)
	}
	if got := uno[string](b, `SELECT stato || ':' || (deciso_da IS NULL)::text FROM relazione_proposta
		WHERE thread_id = $1 AND allegato_id = $2 AND figlio_chiave = '#3'`, b.thread, sc.a.AllegatoID); got != "duplicato:true" {
		t.Errorf("l'arco automatico resta com'era (tenuta dei conti): %s", got)
	}
	b.passa("l'aggancio confermato")
	b.congelaOk("l'aggancio confermato")
}

// Smistamento G, condizione 4 (A5.4.3, A5.4.8, L3): le autorizzazioni da sistemare fermano il gate, una per
// banco, ciascuna da sola: il conflitto (7120010 con due file autorizzati, il secondo scritto fuori dal gesto),
// la superata (lo STEP di 7120010 sostituito, e alla domanda si e' risposto «no»), l'incoerente (la marcatura
// senza chi l'ha decisa), il P7 della forma di prima (la radice dello STEP strutturale decisa come un altro
// componente), la delega con la catena rotta (la delega di 7120010, che teneva quella di 7120011, tolta fuori dal
// gesto). Per la catena rotta c'e' anche la strada: la delega si revoca, e il gate passa.
func TestGateLeAutorizzazioniDaSistemareFermano(t *testing.T) {
	// 7120010.stp associato a 7120010 e autorizzato, con il suo figlio diretto deciso
	stepDi7120010 := func(b *banco) (uuid.UUID, db.Allegato) {
		b.t.Helper()
		s := b.codiceComp("7120010")
		doc, sa := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}})
		b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: doc})
		b.decidi(sa, "#2")
		b.documentiRichiesti()
		b.passa("7120010.stp autorizzato e deciso")
		return doc, sa
	}
	casi := []struct {
		nome, frase string
		quanti      int // i file in conflitto sono due, e ciascuno ha la sua riga
		prep        func(b *banco, sc scenaGate)
	}{
		{"conflitto", "7120010 ha 2 STEP autorizzati", 2, func(b *banco, sc scenaGate) {
			stepDi7120010(b)
			bis := b.allegatoStep("7120010_bis.stp", strings.Repeat("7b", 32))
			b.applica(bis, fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}}.json())
			testutil.AutorizzaStep(b.t, b.p, b.thread, b.codiceComp("7120010"), bis.AllegatoID, b.utente)
		}},
		{"superata", "superata: il file è stato sostituito", 1, func(b *banco, sc scenaGate) {
			vecchio, _ := stepDi7120010(b)
			nuovo, _ := b.stepDelProdotto(b.codiceComp("7120010"), "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120014"}, archi: []string{"#1>#2*2", "#1>#3"}})
			b.esegui(`UPDATE documento SET stato_nas = 'scritto', scritto_il = now() WHERE documento_id = $1`, nuovo)
			if _, err := b.gesto(func(q *db.Queries) (string, error) {
				return fascicolo.Sostituisci(b.ctx, q, b.thread, vecchio, nuovo, false, b.utente)
			}); err != nil {
				b.t.Fatal(err)
			}
		}},
		{"incoerente", "incoerente: il nodo #1 è marcato ma nessuna persona l'ha deciso", 1, func(b *banco, sc scenaGate) {
			_, sa := stepDi7120010(b)
			b.esegui(`UPDATE componente_proposta SET deciso_da = NULL WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#1'`, b.thread, sa.AllegatoID)
		}},
		{"delega con la catena rotta", "7120011 · STEP autorizzato 7120001.stp (delega): catena rotta", 1, func(b *banco, sc scenaGate) {
			b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.prodotto, Documento: sc.doc, Deleghe: []string{"#2"}})
			b.decidi(sc.a, "#4")
			b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: b.codiceComp("7120011"), Nodo: fascicolo.NodoFile{Allegato: sc.a.AllegatoID, Chiave: "#4"}})
			b.decidi(sc.a, "#5")
			b.documentiRichiesti()
			b.passa("le due deleghe decise")
			// la delega di 7120010 se ne va fuori dal gesto (un intervento a mano): quella di 7120011 resta senza
			// la catena, e non l'ha tolta nessuno
			b.esegui(`UPDATE componente_proposta SET evidenza = evidenza - 'strutturale' WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`,
				b.thread, sc.a.AllegatoID)
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			b := nuovoBanco(t)
			sc := b.scenaGate(true)
			b.passa("la scena")
			c.prep(b, sc)
			g := b.fermaSu(c.nome, "autorizzazione da sistemare: ", c.quanti)
			for _, p := range g.Problemi {
				if !strings.Contains(p, c.frase) {
					t.Errorf("il problema: %q, atteso che dica %q", p, c.frase)
				}
			}
			if strings.Contains(avvisiDi(g), "autorizzazione sospesa") {
				t.Errorf("da sistemare non e' sospesa: %s", avvisiDi(g))
			}
			if c.nome == "delega con la catena rotta" {
				// la strada: la delega rotta si revoca, e il gate passa; il nodo resta 7120011, deciso da una persona
				_, err := b.revoca(b.codiceComp("7120011"), sc.a.Sha256.String)
				ok(t, err)
				if got := b.statoNodo(sc.a, "#4"); got != "confermata 7120011 persona" {
					t.Errorf("il nodo della delega revocata: %s", got)
				}
				b.passa("la delega rotta revocata")
			}
		})
	}

	// il P7 della forma di prima: lo STEP strutturale del prodotto senza marcatura, con la radice decisa da una
	// persona come un altro componente
	t.Run("P7", func(t *testing.T) {
		b := nuovoBanco(t)
		b.acme()
		prodotto := b.componente("7120001", db.TipoComponenteFinito)
		f := fattiSTEP{nodi: []string{"#1=7120001", "#2=7120012"}, archi: []string{"#1>#2"}}
		doc, a := b.stepDelProdotto(prodotto, "7120001", f)
		b.applica(a, f.json())
		b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, prodotto, doc)
		b.documentiRichiesti()
		// la forma di prima con la radice aperta vale (K1): il figlio diretto e' da decidere
		b.fermaSolo("la forma di prima", "1 figli diretti, 1 relazioni")
		altro := b.componente("7120099", db.TipoComponenteSciolto)
		b.esegui(`UPDATE componente_proposta SET stato = 'confermata', componente_id = $3, deciso_da = $4, deciso_il = now()
			WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#1'`, b.thread, a.AllegatoID, altro, b.utente)
		b.documentiRichiesti()
		g := b.fermaSolo("P7", "autorizzazione da sistemare: 7120001 · STEP autorizzato")
		if !strings.Contains(g.Problemi[0], "conflitto P7") {
			t.Errorf("il P7: %q", g.Problemi[0])
		}
	})
}

// Smistamento G, condizione 5 (A5.4.8; decisioni del 27/09 ter, Domanda 5 = B; precisazione dell'utente del
// 27/09 sera): una dichiarazione sospesa e' un avviso. 7120010, con il suo STEP autorizzato, diventa commerciale:
// la dichiarazione e' sospesa, il figlio diretto riaperto nel suo file non conta, il gate passa con l'avviso.
// Tornato sottoassieme resta sospesa (la sospensione e' registrata e nessuno l'ha riattivata): ancora un avviso,
// e il gate passa. La riattivazione esplicita rimette l'autorita': il figlio aperto torna a fermare.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che il cambio di tipo portasse a commerciale 7120010, che
// nella working ha il figlio 7120011. Con la regola della PR #7, confermata dall'utente il 29/09 sera (domanda 6a:
// il commerciale e' sempre una foglia), quel gesto si rifiuta (lo prova commercialeDiPrima); il commerciale con il
// suo figlio resta il caso dei dati di prima, che commercialeDiPrima ricostruisce, e il resto della prova e'
// com'era. Asserzioni (righe con t.Error, t.Fatal, deveRifiutare): prima 6, dopo 7, piu' il rifiuto che
// commercialeDiPrima controlla.
func TestGateUnaDichiarazioneSospesaEUnAvviso(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaGate(true)
	s := b.codiceComp("7120010")
	doc, sa := b.stepDelProdotto(s, "7120010", fattiSTEP{nodi: []string{"#1=7120010", "#2=7120011", "#3=7120099"}, archi: []string{"#1>#2*2", "#1>#3"}})
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: doc})
	b.decidi(sa, "#2")
	// 7120099 non e' un pezzo della distinta: il nodo e il suo arco si scartano (una decisione per riga, A1.1)
	for _, f := range []func(q *db.Queries) (string, error){
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.nodoIn(sa, "#3"), b.utente)
		},
		func(q *db.Queries) (string, error) {
			return fascicolo.ScartaRelazione(b.ctx, q, b.thread, fascicolo.ChiaveRelazione{Allegato: sa.AllegatoID, Padre: "#1", Figlio: "#3"}, b.utente)
		},
	} {
		if _, err := b.gesto(f); err != nil {
			t.Fatal(err)
		}
	}
	b.documentiRichiesti()
	b.passa("7120010.stp deciso")

	if msg := b.commercialeDiPrima(s); !strings.Contains(msg, "Sospesa l'autorizzazione di 7120010.stp per 7120010") {
		t.Errorf("il passaggio a commerciale (dati di prima): %q", msg)
	}
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.RiapriNodo(b.ctx, q, b.thread, b.nodoIn(sa, "#3"), b.utente)
	}); err != nil {
		t.Fatal(err)
	}
	sospesa := "autorizzazione sospesa: 7120010 · STEP autorizzato 7120010.stp: sospesa: 7120010: è diventato commerciale"
	for _, passo := range []string{"commerciale", "tornato sottoassieme"} {
		if passo == "tornato sottoassieme" {
			b.cambiaTipoOk(s, db.TipoComponenteSottoassieme)
		}
		if s := b.strutturali(); s != (db.GateStrutturaleRow{}) {
			t.Errorf("%s: il file sospeso non conta: %+v", passo, s)
		}
		g := b.passa(passo)
		if !strings.Contains(avvisiDi(g), sospesa) {
			t.Errorf("%s: la sospesa e' un avviso: %s", passo, avvisiDi(g))
		}
	}

	if _, err := b.riattiva(s, sa.Sha256.String); err != nil {
		t.Fatal(err)
	}
	g := b.fermaSolo("riattivata", "decisioni strutturali aperte negli STEP autorizzati (1 figli diretti")
	if strings.Contains(avvisiDi(g), "autorizzazione sospesa") {
		t.Errorf("riattivata, non e' piu' sospesa: %s", avvisiDi(g))
	}
}

// Smistamento G, condizione 5 per le deleghe (A5.4.6, fase T): le deleghe sotto una sospensione aspettano con
// lei, e sono un avviso, non una catena rotta. Nello STEP del prodotto 7120010 e 7120011 sono delegati e decisi;
// 7120010 diventa commerciale: la sua delega e quella di 7120011, che dipendeva da lei, sono sospese, e il gate
// passa con i due avvisi (a differenza della catena rotta di TestGateLeAutorizzazioniDaSistemareFermano).
//
// Riscritta per lo Smistamento (Distinta): prima fissava che il cambio di tipo portasse a commerciale 7120010, che
// nella working ha il figlio 7120011. Con la regola della PR #7 (domanda 6a, 29/09 sera: il commerciale e' sempre
// una foglia) quel gesto si rifiuta (lo prova commercialeDiPrima), e il commerciale con il figlio e' il caso dei
// dati di prima, che commercialeDiPrima ricostruisce; il resto e' com'era. Asserzioni (righe con t.Error, t.Fatal,
// deveRifiutare): prima 2, dopo 3, piu' il rifiuto che commercialeDiPrima controlla.
func TestGateLeDelegheSottoUnaSospensioneSonoUnAvviso(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaGate(true)
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: sc.prodotto, Documento: sc.doc, Deleghe: []string{"#2"}})
	b.decidi(sc.a, "#4")
	n := b.codiceComp("7120011")
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: n, Nodo: fascicolo.NodoFile{Allegato: sc.a.AllegatoID, Chiave: "#4"}})
	b.decidi(sc.a, "#5")
	b.documentiRichiesti()
	b.passa("le due deleghe decise")

	if msg := b.commercialeDiPrima(b.codiceComp("7120010")); !strings.Contains(msg, "Sospesa la delega di 7120001.stp per 7120010") {
		t.Errorf("il passaggio a commerciale (dati di prima): %q", msg)
	}
	g := b.passa("7120010 commerciale")
	av := avvisiDi(g)
	for _, c := range []string{"autorizzazione sospesa: 7120010 · STEP autorizzato 7120001.stp (delega): sospesa",
		"autorizzazione sospesa: 7120011 · STEP autorizzato 7120001.stp (delega): sospesa"} {
		if !strings.Contains(av, c) {
			t.Errorf("manca l'avviso %q in %s", c, av)
		}
	}
	if strings.Contains(av, "catena rotta") {
		t.Errorf("sotto una sospensione la catena non e' rotta: %s", av)
	}
	b.congelaOk("le deleghe sospese")
}

// Smistamento G, condizione 6 (A5.4.8, U2; prova 114, parte materializzazione, sulla scena dello Smistamento):
// le copie sul NAS in coda o in errore e le anomalie aperte non fermano il congelamento e stanno nel terzo
// elenco. Il 2D di 7120012, requisito bloccante, e' in errore sul NAS: v_thread_bloccanti lo conta ancora, il
// gate no; il 2D del prodotto aspetta la copia fra 7 minuti; il 2D di 7120010 ha un'anomalia aperta. Il
// documento in errore entra nell'istantanea.
func TestGateLeCopieSulNasNonFermanoEStannoNellaMaterializzazione(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaGate(true)
	duedi := func(codice string) uuid.UUID {
		return uno[uuid.UUID](b, `SELECT documento_id FROM documento WHERE thread_id = $1 AND codice = $2 AND tipo = 'disegno_2d'`, b.thread, codice)
	}
	errato, inCoda, anomalo := duedi("7120012"), duedi("7120001"), duedi("7120010")
	b.esegui(`UPDATE documento SET stato_nas = 'errore', errore_nas = 'NAS assente', scritto_il = NULL WHERE documento_id = $1`, errato)
	b.esegui(`UPDATE documento SET stato_nas = 'in_coda', scritto_il = NULL WHERE documento_id = $1`, inCoda)
	b.esegui(`INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, non_prima_di)
		VALUES ('copia_nas', 'server', jsonb_build_object('documento_id', $1::uuid), 'nas:' || $1::text, now() + interval '7 minutes')`, inCoda)
	b.esegui(`INSERT INTO nas_anomalia (documento_id, thread_id, problema, stato_db, percorso, sha_atteso)
		VALUES ($1, $2, 'mancante', 'scritto', 'ACME\WIP\rfq\7120010.pdf', $3)`, anomalo, b.thread, b.sha(anomalo))

	if n := uno[int64](b, `SELECT n_bloccanti FROM v_thread_bloccanti WHERE thread_id = $1`, b.thread); n != 2 {
		t.Fatalf("v_thread_bloccanti.n_bloccanti = %d: la vista conta i due 2D in 'ok_errore_nas'", n)
	}
	if n := uno[int64](b, `SELECT count(*) FROM v_fascicolo WHERE thread_id = $1 AND bloccante AND esito NOT IN ('ok', 'ok_in_coda', 'ok_errore_nas', 'derogato')`, b.thread); n != 0 {
		t.Fatalf("i bloccanti logici: %d", n)
	}
	g := b.passa("le copie in coda e in errore")
	docs := uno[int](b, `SELECT count(*) FROM documento WHERE thread_id = $1 AND sostituito_da IS NULL`, b.thread)
	atteso := fascicolo.Materializzazione{Documenti: docs, Scritti: docs - 2, InCoda: 1, Errore: 1, Anomalie: 1, FraMinuti: 7}
	if g.Nas != atteso {
		t.Errorf("materializzazione %+v, attesa %+v", g.Nas, atteso)
	}
	if got := strings.Join(g.Materializzazione, " | "); got != "1 documento in coda per la copia sul NAS (la prima fra 7 min) | 1 documento in errore sul NAS | 1 anomalia NAS aperta sui documenti" {
		t.Errorf("terzo elenco: %s", got)
	}
	if strings.Contains(strings.Join(g.Problemi, " ")+avvisiDi(g), "NAS") {
		t.Errorf("il NAS non e' fra i problemi ne' fra gli avvisi del gate: %v %v", g.Problemi, g.Avvisi)
	}
	c, err := b.congela("con le copie in sospeso")
	ok(t, err)
	if n := uno[int64](b, `SELECT count(*) FROM bom_versione_documento WHERE bom_versione_id = $1 AND documento_id = $2`, c.Versione.BomVersioneID, errato); n != 1 {
		t.Errorf("il documento in errore non e' entrato nell'istantanea")
	}
}

// Smistamento G, avvisi (A5.4.5, A5.4.8): l'assieme con i figli solo in guida, e il file autorizzato letto in
// parte con le rimozioni sospese. 7120010 ha 7120011 nel file del prodotto: l'avviso lo dice finche' 7120010 non
// ha uno STEP suo. Autorizzato 7120010.stp, letto in parte, con il figlio diretto deciso: l'avviso della guida se
// ne va, arriva quello della lettura parziale, e la rimozione dell'arco 7120010 → 7120098, che il file non ha,
// non si propone (una mancanza in una lettura parziale non e' un'informazione). Il gate passa. Lo STEP del
// prodotto non ha l'avviso: lo dice v_step_prodotto.
func TestGateAvvisaDellaGuidaEDellaLetturaParziale(t *testing.T) {
	b := nuovoBanco(t)
	b.scenaGate(true)
	g := b.passa("la scena")
	if av := avvisiDi(g); !strings.Contains(av, "7120010 ha 1 figlio che resta guida") || !strings.Contains(av, "7120012 ha 1 figlio che resta guida") ||
		!strings.Contains(av, ": le loro proposte sono guida, non entrano nella BOM finché uno STEP non è autorizzato") {
		t.Errorf("gli assiemi con il figlio solo in guida: %s", av)
	}
	s := b.codiceComp("7120010")
	vecchio := b.componente("7120098", db.TipoComponenteSciolto)
	b.arco(s, vecchio, 1)
	parziale := fattiSTEP{troncato: true, nodi: []string{"#1=7120010", "#2=7120011"}, archi: []string{"#1>#2*2"}}
	doc, sa := b.stepDelProdotto(s, "7120010", parziale)
	e := b.effetto(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: doc})
	if !strings.Contains(e.RimozioniSospese, "letto in parte") {
		t.Errorf("l'anteprima dice le rimozioni sospese: %q", e.RimozioniSospese)
	}
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: s, Documento: doc})
	b.decidi(sa, "#2")
	b.documentiRichiesti()
	g = b.passa("7120010.stp letto in parte")
	av := avvisiDi(g)
	if !strings.Contains(av, "7120010 · STEP autorizzato 7120010.stp letto in parte (") || !strings.Contains(av, "): le rimozioni restano sospese") {
		t.Errorf("l'avviso della lettura parziale: %s", av)
	}
	if strings.Contains(av, "7120010 ha 1 figlio") || strings.Contains(av, "7120001 · STEP autorizzato") {
		t.Errorf("con lo STEP suo 7120010 non ha piu' figli in guida, e lo STEP del prodotto non e' un avviso: %s", av)
	}
	if got := b.rimozioni(); got != "" {
		t.Errorf("una lettura parziale non propone rimozioni: %s", got)
	}
	b.congelaOk("con la lettura parziale")
}
