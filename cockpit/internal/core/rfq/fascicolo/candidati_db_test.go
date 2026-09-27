//go:build integrazione

// L4 — B8.6: i codici candidati di una RFQ contro PostgreSQL vero. La vista v_codici_candidati_thread (0018)
// unisce messaggi, proposte dei documenti e nodi degli STEP; CandidatiDellaRfq la legge con quello che la
// RFQ ha gia' deciso (componenti, proposte STEP aperte, codici della richiesta, BOM congelata).
//
// La prova del piano e' TestLaVistaDeiCandidatiUnisceMessaggiAllegatiEStep; le altre sono i casi chiesti
// per B8.6: componente, proposta, archiviato, revisioni discordanti, BOM congelata.
//
// Smistamento F2 (R1): il gesto «+ Prodotto / + Assieme / + Particolare» (AggiungiDaCodice) e' tolto. Le
// prove che lo usavano sono riscritte: un codice trovato si legge e non fa nascere niente; un componente nasce
// dal nodo di uno STEP, da un codice della richiesta (AssicuraProdottiDellaRichiesta) o dal codice scritto
// nell'editor (la carta «n:», con origine manuale).

package fascicolo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

const famiglia777 = `{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})(?:_(?P<rev>[A-Z]))?", "descrizione": "disegni 777", "rev_nel_codice": true, "esempio": "77722757_B"}]}`

// conFamiglie da' al cliente del banco la famiglia 777: il generico da solo non e' piu' un candidato.
func (b *banco) conFamiglie() {
	b.t.Helper()
	b.esegui(`UPDATE cliente SET regole = $1 FROM thread_offerta t WHERE t.cliente_id = cliente.cliente_id AND t.thread_id = $2`, famiglia777, b.thread)
}

// mail e' un messaggio agganciato alla RFQ del banco.
func (b *banco) mail() uuid.UUID {
	b.t.Helper()
	b.n++
	conv := uno[uuid.UUID](b, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("C-B86-%d", b.n))
	return uno[uuid.UUID](b, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3) RETURNING messaggio_id`, fmt.Sprintf("<b86-%d@prova>", b.n), conv, b.thread)
}

// candidato e' una riga di candidato_codice, come la scrive il triage.
func (b *banco) candidato(msg uuid.UUID, codice, rev, ruolo, origine, dove string) {
	b.t.Helper()
	famiglia, punti := "", 30
	switch origine {
	case "famiglia":
		famiglia, punti = "disegni 777", 80
	case "riferimento":
		punti = 90
	}
	b.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		msg, codice, ruolo, rev, origine, famiglia, punti, dove)
}

// fileConProposta e' un allegato della mail con la sua proposta di documento.
func (b *banco) fileConProposta(msg uuid.UUID, nome, tipo, codice, rev, fonte, stato, dettagli string) uuid.UUID {
	b.t.Helper()
	b.n++
	a := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il)
		VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, now()) RETURNING allegato_id`, msg, b.n, nome, fmt.Sprintf("%064x", 90000+b.n))
	if dettagli == "" {
		dettagli = "{}"
	}
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, stato, dettagli)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), 70, $6, $7, $8)`, a, b.thread, tipo, codice, rev, fonte, stato, dettagli)
	return a
}

func (b *banco) candidati() fascicolo.Candidati {
	b.t.Helper()
	var c fascicolo.Candidati
	if err := b.tx(func(q *db.Queries) (err error) {
		c, err = fascicolo.CandidatiDellaRfq(b.ctx, q, b.thread)
		return err
	}); err != nil {
		b.t.Fatalf("candidati della RFQ: %v", err)
	}
	return c
}

// nellEditor conferma nell'editor della struttura, sotto il prodotto radice, la BOM com'e' piu' gli archi dati
// (riferimenti c:, p:, n:, k:) e i componenti scritti: e' «Conferma struttura».
func (b *banco) nellEditor(radice uuid.UUID, archi []fascicolo.ArcoVoluto, nuovi ...fascicolo.ComponenteNuovo) (string, error) {
	b.t.Helper()
	v := fascicolo.StrutturaVoluta{Radice: radice, Nuovi: nuovi}
	righe, err := b.p.Query(b.ctx, `SELECT padre_id, figlio_id, qta FROM componente_relazione WHERE thread_id = $1`, b.thread)
	ok(b.t, err)
	for righe.Next() {
		var p, f uuid.UUID
		var q int32
		ok(b.t, righe.Scan(&p, &f, &q))
		a := fascicolo.ArcoVoluto{Padre: "c:" + p.String(), Figlio: "c:" + f.String(), Qta: q}
		v.Archi, v.Visti = append(v.Archi, a), append(v.Visti, a)
	}
	righe.Close()
	v.Archi = append(v.Archi, archi...)
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ApplicaStrutturaVoluta(b.ctx, q, b.thread, b.utente, v)
	})
}

// sotto e' l'arco radice → ref, per nellEditor.
func sotto(radice uuid.UUID, ref string) []fascicolo.ArcoVoluto {
	return []fascicolo.ArcoVoluto{{Padre: "c:" + radice.String(), Figlio: ref, Qta: 1}}
}

func elenco(l []fascicolo.CodiceCandidato) string {
	s := make([]string, len(l))
	for i, k := range l {
		s[i] = k.Codice + ":" + string(k.Stato.Situazione)
	}
	return strings.Join(s, " ")
}

func trovaCodice(t *testing.T, c fascicolo.Candidati, codice string) fascicolo.CodiceCandidato {
	t.Helper()
	k, ok := c.Trova(codice)
	if !ok {
		t.Fatalf("%s non c'e': prodotto [%s], altri [%s]", codice, elenco(c.Prodotto), elenco(c.Altri))
	}
	return k
}

// ---------------------------------------------------------------- la vista

// La vista unisce le tre sorgenti e il Go le aggrega per codice: le evidenze di un messaggio, del nome di
// un disegno e di uno STEP fanno una riga sola; il riferimento della richiesta, le proposte scartate e i
// nodi senza codice non ci sono; componenti e codici della richiesta dicono la situazione senza essere
// evidenze.
//
// Riscritta per lo Smistamento (F4, `Unisci` con `nome_contiene_codice`): prima gli altri riferimenti erano
// «12345678 20260908», con il codice citato nel nome dell'offerta a 50 come la vista lo scrive. Adesso vale lo
// score della sua regola (25), sotto il generico del corpo (30).
func TestLaVistaDeiCandidatiUnisceMessaggiAllegatiEStep(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.esegui(`UPDATE thread_offerta SET riferimento_cliente = 'RDO 400012345' WHERE thread_id = $1`, b.thread)
	m := b.mail()
	b.candidato(m, "77722757", "", "prodotto", "famiglia", "oggetto")
	b.candidato(m, "20260908", "", "non_classificato", "generico", "corpo")
	b.candidato(m, "RDO 400012345", "", "riferimento_rfq", "riferimento", "oggetto")
	b.candidato(m, "77731111", "", "prodotto", "famiglia", "corpo")
	b.candidato(m, "77740000", "", "prodotto", "famiglia", "corpo")
	b.candidato(m, "77750000", "", "prodotto", "famiglia", "oggetto")
	b.fileConProposta(m, "77722757_B.pdf", "disegno_2d", "77722757", "B", "nome_file", "aperta", "")
	b.fileConProposta(m, "Offerta 12345678 RDO 400012345.pdf", "commerciale", "", "", "estensione", "aperta",
		`{"codici_nel_nome": ["12345678", "400012345"]}`)
	b.fileConProposta(m, "vecchio.pdf", "disegno_2d", "77799999", "", "nome_file", "scartata", "")
	b.applica(b.allegatoStep("assieme.stp", strings.Repeat("6", 64)),
		fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517", "#3=Part1"}, archi: []string{"#1>#2", "#1>#3"}}.json())
	arch := b.componente("77731111", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'tolto dal cliente' WHERE componente_id = $1`, arch, b.utente)
	b.componente("77740000", db.TipoComponenteSottoassieme)
	b.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, '77750000', 'manuale')`, b.thread)

	// la vista, riga per riga: le quattro sorgenti ci sono, cio' che deve mancare manca
	righe, err := db.New(b.p).ListCodiciCandidatiThread(b.ctx, uuid.NullUUID{UUID: b.thread, Valid: true})
	ok(t, err)
	sorgenti := map[string]int{}
	for _, r := range righe {
		sorgenti[r.Sorgente]++
		switch {
		case r.Ruolo == "riferimento_rfq", r.Codice == "77799999", strings.EqualFold(r.Codice, "Part1"):
			t.Errorf("la vista non doveva dare %+v", r)
		case r.Sorgente == "nome_file" && r.TipoFile != "commerciale":
			t.Errorf("il tipo del file va con la riga del suo nome: %+v", r)
		}
	}
	if sorgenti["messaggio"] != 5 || sorgenti["proposta_documento"] != 1 || sorgenti["nome_file"] != 2 || sorgenti["step"] != 2 {
		t.Errorf("righe per sorgente = %v", sorgenti)
	}

	c := b.candidati()
	if got := elenco(c.Prodotto); got != "77720517:proposta 77722757:proposta 77731111:archiviato 77740000:componente 77750000:richiesta" {
		t.Errorf("candidati prodotto = %s", got)
	}
	if got := elenco(c.Altri); got != "20260908:nuovo 12345678:nuovo" {
		t.Errorf("altri riferimenti = %s", got)
	}
	if k := trovaCodice(t, c, "12345678"); k.Punteggio != 25 {
		t.Errorf("il codice citato nel nome dell'offerta: score %d, atteso 25 (nome_contiene_codice)", k.Punteggio)
	}
	k := trovaCodice(t, c, "77722757")
	if len(k.Evidenze) != 3 || len(k.Revisioni) != 1 || k.Revisioni[0].Rev != "B" || k.Conflitto {
		t.Errorf("77722757: %d evidenze, revisioni %+v", len(k.Evidenze), k.Revisioni)
	}
	if p := k.Stato.Proposta(); p.NomeFile != "assieme.stp" || p.Chiave != "#1" {
		t.Errorf("77722757 porta al nodo #1 di assieme.stp: %+v", p)
	}
	if s := trovaCodice(t, c, "77731111").Stato; s.Componente == nil || s.Componente.ComponenteID != arch {
		t.Errorf("l'archiviato e' il componente di prima: %+v", s.Componente)
	}
	if _, c := c.Trova("400012345"); c {
		t.Error("il riferimento della richiesta non e' un codice, nemmeno dal nome di un file")
	}
}

// ---------------------------------------------------------------- nessun componente da un codice trovato

// Nessun componente nasce da un codice trovato (Smistamento R1). Sostituisce TestUnCodiceNuovoSiAggiungeConIlTipoScelto,
// tolta con AggiungiDaCodice (fase F2): fissava che un codice trovato nella RFQ diventasse un componente con un
// gesto, con il tipo scelto e origine codice_rilevato, e che non si aggiungesse due volte. Adesso: la RFQ vede
// codici di ogni forza (la famiglia nel corpo, il nome di un disegno, il cartiglio, il generico) e leggerli non
// scrive niente; l'editor rifiuta la carta «k:» di un codice trovato; il componente nasce solo dal codice
// scritto (la carta «n:»), con origine manuale e mai codice_rilevato, e una volta sola.
func TestNessunComponenteNasceDaUnCodiceTrovato(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	m := b.mail()
	b.candidato(m, "77760000", "", "prodotto", "famiglia", "corpo")
	b.candidato(m, "20260908", "", "non_classificato", "generico", "corpo")
	b.fileConProposta(m, "77761111_A.pdf", "disegno_2d", "77761111", "A", "nome_file", "aperta", "")
	b.fileConProposta(m, "foglio.pdf", "disegno_2d", "77762222", "", "cartiglio", "aperta", "")
	prodotto := b.componente("77750000", db.TipoComponenteFinito)
	prima := b.bom()

	c := b.candidati()
	for _, codice := range []string{"77760000", "20260908", "77761111", "77762222"} {
		if s := trovaCodice(t, c, codice).Stato; s.Situazione != fascicolo.SituazioneNuovo || s.Componente != nil {
			t.Errorf("%s: %+v", codice, s)
		}
	}
	if b.bom() != prima {
		t.Fatalf("leggere i codici ha cambiato la BOM:\nprima %s\ndopo  %s", prima, b.bom())
	}
	for _, codice := range []string{"77760000", "77761111", "77762222", "20260908"} {
		_, err := b.nellEditor(prodotto, sotto(prodotto, "k:"+codice))
		deveRifiutare(t, err, codice+": un codice trovato nella RFQ non diventa un componente dall'editor")
	}
	if b.bom() != prima {
		t.Fatalf("un rifiuto ha cambiato la BOM: %s", b.bom())
	}

	// scritto, nasce: con il tipo scelto, origine manuale, chi l'ha deciso
	msg, err := b.nellEditor(prodotto, sotto(prodotto, "n:77760000"), fascicolo.ComponenteNuovo{Codice: "77760000", Tipo: db.TipoComponenteSottoassieme})
	ok(t, err)
	if msg != "Struttura di 77750000 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT codice || ':' || tipo || ':' || origine || ':' || coalesce(rev, '-') || ':' || (confermato_da = $2)::text
		FROM componente WHERE thread_id = $1 AND codice = '77760000'`, b.thread, b.utente); got != "77760000:sottoassieme:manuale:-:true" {
		t.Errorf("componente = %s", got)
	}
	if n := uno[int64](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND origine = 'codice_rilevato'`, b.thread); n != 0 {
		t.Errorf("%d componenti nati da un codice rilevato", n)
	}
	if s := trovaCodice(t, b.candidati(), "77760000").Stato.Situazione; s != fascicolo.SituazioneComponente {
		t.Errorf("dopo: %s", s)
	}
	// scritto una seconda volta, sotto un altro padre, e' lo stesso pezzo: la carta «n:» ritrova il componente
	// (identita' (thread, upper(codice)), U4), non nasce una seconda riga e l'arco nuovo punta a lui. Il tipo
	// scritto la seconda volta non cambia quello deciso la prima. La carta sta in un arco: una carta che nessun
	// arco usa non si risolve, e non proverebbe niente
	nato := uno[uuid.UUID](b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77760000'`, b.thread)
	assieme := b.componente("77763333", db.TipoComponenteSottoassieme)
	b.arco(prodotto, assieme, 1)
	msg, err = b.nellEditor(prodotto, []fascicolo.ArcoVoluto{{Padre: "c:" + assieme.String(), Figlio: "n:77760000", Qta: 3}},
		fascicolo.ComponenteNuovo{Codice: "77760000", Tipo: db.TipoComponenteSciolto})
	ok(t, err)
	if msg != "Struttura di 77750000 confermata: 1 codice scritto già nella BOM, collegato lo stesso componente, 1 legame aggiunto." {
		t.Errorf("messaggio: %q", msg)
	}
	if n := uno[int64](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND upper(codice) = '77760000'`, b.thread); n != 1 {
		t.Errorf("%d componenti 77760000, atteso 1", n)
	}
	if n := uno[int64](b, `SELECT count(*) FROM componente WHERE thread_id = $1`, b.thread); n != 3 {
		t.Errorf("%d componenti, attesi 3 (il prodotto, l'assieme e 77760000)", n)
	}
	if got := uno[string](b, `SELECT string_agg(p.codice || '>' || f.codice || '*' || r.qta || ':' || (r.figlio_id = $2)::text, ' ' ORDER BY p.codice)
		FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id
		WHERE r.thread_id = $1 AND f.codice = '77760000'`, b.thread, nato); got != "77750000>77760000*1:true 77763333>77760000*3:true" {
		t.Errorf("i padri di 77760000 = %s", got)
	}
	if got := uno[string](b, `SELECT tipo::text || ':' || origine::text FROM componente WHERE componente_id = $1`, nato); got != "sottoassieme:manuale" {
		t.Errorf("77760000 = %s, atteso sottoassieme:manuale (quello deciso la prima volta)", got)
	}
}

// Un codice con un nodo STEP aperto si decide li': leggere il pannello non tocca niente; accettare il nodo
// crea il componente, e il codice diventa «nel Fascicolo».
//
// Riscritta per lo Smistamento (R1, fase F2): prima fissava che «+» su quel codice si rifiutasse («si decide
// quella»); il «+» non c'e' piu'. Fissa che la lettura porta al nodo e non cambia ne' la BOM ne' le proposte.
func TestUnCodiceConUnaPropostaApertaSiDecideNellaProposta(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.candidato(b.mail(), "77720517", "", "prodotto", "famiglia", "corpo")
	b.applica(b.allegatoStep("assieme.stp", strings.Repeat("7", 64)),
		fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517"}, archi: []string{"#1>#2*2"}}.json())
	prima := b.nodiProposti()

	s := trovaCodice(t, b.candidati(), "77720517").Stato
	if s.Situazione != fascicolo.SituazioneProposta || s.Proposta().NomeFile != "assieme.stp" || s.Proposta().NomeGrezzo != "77720517" {
		t.Errorf("il codice porta al nodo «77720517» di assieme.stp: %s %+v", s.Situazione, s.Proposta())
	}
	if b.bom() != "" || b.nodiProposti() != prima {
		t.Fatalf("la lettura ha cambiato qualcosa: bom %q, proposte %q", b.bom(), b.nodiProposti())
	}
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
	})
	ok(t, err)
	if s := trovaCodice(t, b.candidati(), "77720517").Stato; s.Situazione != fascicolo.SituazioneComponente || s.Componente.Origine != db.OrigineComponenteStep {
		t.Errorf("dopo l'accettazione: %s, %+v", s.Situazione, s.Componente)
	}
}

// Un codice di un componente archiviato non ne crea un altro: si ripristina, lo stesso, con la sua storia.
// Il ripristino ritrova anche le proposte di nodo ancora aperte con quel codice.
//
// Riscritta per lo Smistamento (R1, U4, fase F2): prima il rifiuto veniva da «+» sul codice («è archiviato:
// si ripristina»). Adesso viene dall'unica strada per un componente scritto, l'editor: la carta «n:» con il
// codice di un archiviato si rifiuta, e non nasce una seconda riga.
func TestUnCodiceArchiviatoSiRipristinaNonSiRicrea(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.candidato(b.mail(), "77731111", "", "prodotto", "famiglia", "corpo")
	comp := b.componente("77731111", db.TipoComponenteSciolto)
	prodotto := b.componente("77750000", db.TipoComponenteFinito)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ArchiviaComponente(b.ctx, q, b.thread, comp, b.utente, "tolto dal cliente")
	})
	ok(t, err)

	_, err = b.nellEditor(prodotto, sotto(prodotto, "n:77731111"), fascicolo.ComponenteNuovo{Codice: "77731111", Tipo: db.TipoComponenteSciolto})
	deveRifiutare(t, err, "77731111 c'è già ed è archiviato: si ripristina dalla Struttura BOM")
	if n := uno[int64](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND upper(codice) = '77731111'`, b.thread); n != 1 {
		t.Fatalf("%d componenti 77731111: il rifiuto ne ha creato un altro", n)
	}

	// arriva uno STEP con lo stesso codice: il nodo resta aperto (accettarlo lo ripristinerebbe) e il codice
	// porta a lui; ripristinare dal componente lo ritrova
	b.applica(b.allegatoStep("nuovo.stp", strings.Repeat("9", 64)), fattiSTEP{nodi: []string{"#1=77731111"}}.json())
	if s := trovaCodice(t, b.candidati(), "77731111").Stato.Situazione; s != fascicolo.SituazioneProposta {
		t.Errorf("con il nodo aperto: %s", s)
	}
	msg, err := b.gesto(func(q *db.Queries) (string, error) { return fascicolo.RipristinaComponente(b.ctx, q, b.thread, comp) })
	ok(t, err)
	if !strings.Contains(msg, "77731111 ripristinato") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT (archiviato_il IS NULL)::text || ':' || componente_id::text FROM componente WHERE thread_id = $1 AND codice = '77731111'`, b.thread); got != "true:"+comp.String() {
		t.Errorf("ripristinato: %s", got)
	}
	if got := uno[string](b, `SELECT stato || ':' || coalesce((componente_id = $2)::text, 'senza componente') FROM componente_proposta WHERE thread_id = $1`,
		b.thread, comp); got != "duplicato:true" {
		t.Errorf("il nodo aperto ritrova il componente ripristinato: %s", got)
	}
	if s := trovaCodice(t, b.candidati(), "77731111").Stato.Situazione; s != fascicolo.SituazioneComponente {
		t.Errorf("dopo il ripristino: %s", s)
	}
}

// Revisioni diverse fra le evidenze: si vedono tutte e due, il punteggio piu' alto non decide, e nessun
// componente nasce dal codice. Chi scrive il componente nell'editor scrive anche la revisione.
//
// Riscritta per lo Smistamento (R1, U4, fase F2): prima era TestLeRevisioniDiscordantiVoglionoLaScelta e
// fissava la scelta della revisione dentro «+» (rifiuto senza scelta, rifiuto di una non vista, «a» → A). Il
// «+» non c'e' piu': la revisione del componente scritto e' quella che l'operatore scrive, in maiuscolo.
func TestLeRevisioniDiscordantiSiVedonoENonDecidono(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	m := b.mail()
	b.candidato(m, "77770000", "A", "prodotto", "famiglia", "oggetto")
	b.fileConProposta(m, "77770000_B.pdf", "disegno_2d", "77770000", "B", "cartiglio", "aperta", "")
	prodotto := b.componente("77750000", db.TipoComponenteFinito)

	k := trovaCodice(t, b.candidati(), "77770000")
	if !k.Conflitto || len(k.Revisioni) != 2 {
		t.Fatalf("revisioni = %+v", k.Revisioni)
	}
	if k.Punteggio != 80 || k.Revisioni[0].Rev != "A" || k.Stato.Situazione != fascicolo.SituazioneNuovo || k.Stato.Componente != nil {
		t.Errorf("la famiglia nell'oggetto (80, rev A) ordina e non decide: punteggio %d, %+v, %+v", k.Punteggio, k.Revisioni, k.Stato)
	}
	if got := b.bom(); got != "77750000:finito:-" {
		t.Fatalf("leggere i codici ha cambiato la BOM: %s", got)
	}
	msg, err := b.nellEditor(prodotto, sotto(prodotto, "n:77770000"), fascicolo.ComponenteNuovo{Codice: "77770000", Tipo: db.TipoComponenteSciolto, Rev: "a"})
	ok(t, err)
	if msg != "Struttura di 77750000 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT coalesce(rev, '-') || ':' || origine FROM componente WHERE thread_id = $1 AND codice = '77770000'`, b.thread); got != "A:manuale" {
		t.Errorf("rev e origine del componente = %s", got)
	}
}

// Un codice della richiesta e' gia' deciso come prodotto: diventa prodotto con AssicuraProdottiDellaRichiesta
// (la creazione della RFQ nel triage, o l'apertura di una revisione della BOM congelata), e non entra nella
// BOM come altro dall'editor.
//
// Riscritta per lo Smistamento (R1, fase F2): prima entrava come prodotto dal «+ Prodotto» del pannello (e
// «+ Assieme» si rifiutava). Adesso la strada e' AssicuraProdottiDellaRichiesta, e la carta «n:» con quel
// codice si rifiuta.
func TestUnCodiceDellaRichiestaEntraComeProdotto(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.candidato(b.mail(), "77780000", "", "prodotto", "famiglia", "oggetto")
	b.identificativo("77780000", "proposta_famiglia", true)
	altro := b.componente("77750000", db.TipoComponenteFinito)

	if s := trovaCodice(t, b.candidati(), "77780000").Stato; s.Situazione != fascicolo.SituazioneRichiesta || !s.Identificativo {
		t.Errorf("codice della richiesta: %+v", s)
	}
	_, err := b.nellEditor(altro, sotto(altro, "n:77780000"), fascicolo.ComponenteNuovo{Codice: "77780000", Tipo: db.TipoComponenteSottoassieme})
	deveRifiutare(t, err, "77780000 è un codice della richiesta: il prodotto nasce da una decisione del triage, "+
		"o aprendo la revisione se il codice è stato confermato con la BOM congelata; non è un componente dall'editor")
	es := b.assicura()
	if len(es.Creati) != 1 || es.Creati[0] != "77780000" {
		t.Errorf("AssicuraProdottiDellaRichiesta crea il prodotto: %+v", es)
	}
	if got := uno[string](b, `SELECT tipo::text FROM componente WHERE thread_id = $1 AND codice = '77780000'`, b.thread); got != "finito" {
		t.Errorf("77780000 = %s, atteso finito", got)
	}
}

// Con la BOM congelata i codici si leggono, con la loro situazione, ma niente li porta nella working:
// ne' un componente scritto nell'editor, ne' il ripristino (D26).
func TestConLaBomCongelataICodiciNonCambianoLaBom(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	r := b.rfqCongelabile()
	m := b.mail()
	b.candidato(m, "77790000", "", "prodotto", "famiglia", "corpo")
	b.candidato(m, "77791111", "", "prodotto", "famiglia", "corpo")
	arch := b.componente("77791111", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'prova' WHERE componente_id = $1`, arch, b.utente)
	_, err := b.congela("prima baseline")
	ok(t, err)
	prima := b.bom()

	c := b.candidati()
	if c.Bloccata != 1 {
		t.Errorf("Bloccata = %d, attesa 1", c.Bloccata)
	}
	for codice, attesa := range map[string]fascicolo.Situazione{"77790000": fascicolo.SituazioneNuovo, "77791111": fascicolo.SituazioneArchiviato} {
		if s := trovaCodice(t, c, codice).Stato; s.Situazione != attesa || s.Bloccata != 1 {
			t.Errorf("%s: %s, bloccata %d", codice, s.Situazione, s.Bloccata)
		}
	}
	// riscritta per lo Smistamento (F2): prima si rifiutava «+» sul codice; adesso il componente scritto nell'editor
	_, err = b.nellEditor(r.p1, sotto(r.p1, "n:77790000"), fascicolo.ComponenteNuovo{Codice: "77790000", Tipo: db.TipoComponenteSciolto})
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	_, err = b.gesto(func(q *db.Queries) (string, error) { return fascicolo.RipristinaComponente(b.ctx, q, b.thread, arch) })
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	if b.bom() != prima {
		t.Errorf("la BOM congelata e' cambiata:\nprima %s\ndopo  %s", prima, b.bom())
	}
}

// D16 chiusa (24/09/2026): la radice di famiglia scrive fonte = 'regola_cliente' e regola_id NULL, anche
// se la proposta ne aveva uno.
//
// Riscritta per lo Smistamento (F4, D16 → evidenza): prima fissava anche famiglia, dove e testo del
// riconoscimento come chiavi sciolte dei dettagli, scritte dalla D16 insieme al codice. Adesso stanno
// nell'evidenza della radice dentro `valutazione.codice`, e codice, fonte e confidenza in colonna sono il
// riepilogo: con un nome che non e' un codice, la radice di famiglia (80, `regola_cliente`), e la rev che la
// famiglia separa (65). regola_id resta NULL: gli score stanno nella tabella S1, non in `regola`.
func TestLaRadiceDiFamigliaNonScriveUnaRegolaDellaTabellaRegola(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.esegui(`INSERT INTO regola (regola_id, descrizione) VALUES ('esempio.nome_file', 'il codice dal nome del file') ON CONFLICT DO NOTHING`)
	a := b.allegatoStep("assieme 7.stp", strings.Repeat("4", 64))
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, regola_id)
		VALUES ($1, $2, 'cad_3d', 'ASSIEME 7', 40, 'nome_file', 'esempio.nome_file')`, a.AllegatoID, b.thread)
	b.applica(a, fattiSTEP{nodi: []string{"#1=77722757_B", "#2=77720517"}, archi: []string{"#1>#2"}}.json())
	got := uno[string](b, `SELECT codice || ':' || coalesce(rev, '-') || ':' || fonte || ':' || confidenza || ':' || coalesce(regola_id, 'NULL') || ':' ||
		(dettagli #>> '{valutazione,codice,evidenze,0,famiglia}') || ':' || (dettagli #>> '{valutazione,codice,evidenze,0,dove}') || ':' ||
		(dettagli #>> '{valutazione,codice,evidenze,0,testo}') || ':' || (dettagli #>> '{valutazione,codice,regola}') || ':' ||
		(dettagli #>> '{valutazione,rev,regola}') FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
	if got != "77722757:B:regola_cliente:80:NULL:disegni 777:id:77722757_B:step_radice_famiglia:rev_famiglia_cliente" {
		t.Errorf("proposta del documento = %q", got)
	}
}
