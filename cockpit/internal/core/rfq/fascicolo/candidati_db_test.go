//go:build integrazione

// L4 — B8.6: i codici candidati di una RFQ contro PostgreSQL vero. La vista v_codici_candidati_thread (0018)
// unisce messaggi, proposte dei documenti e nodi degli STEP; CandidatiDellaRfq la legge con quello che la
// RFQ ha gia' deciso (componenti, proposte STEP aperte, codici della richiesta, BOM congelata), e il gesto
// «+ Prodotto / + Assieme / + Particolare» rilegge tutto con la RFQ bloccata.
//
// La prova del piano e' TestLaVistaDeiCandidatiUnisceMessaggiAllegatiEStep; le altre sono i casi chiesti
// per B8.6: componente, proposta, archiviato, revisioni discordanti, BOM congelata.

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

func (b *banco) aggiungi(codice string, tipo db.TipoComponente, rev string) (string, error) {
	return b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AggiungiDaCodice(b.ctx, q, b.thread, b.utente, codice, tipo, rev)
	})
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
	if got := elenco(c.Altri); got != "12345678:nuovo 20260908:nuovo" {
		t.Errorf("altri riferimenti = %s", got)
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

// ---------------------------------------------------------------- il gesto

// Un codice nuovo diventa un componente con un gesto: il tipo scelto, origine codice_rilevato, chi l'ha
// deciso. Dopo, lo stesso codice e' nella BOM e si apre; non si aggiunge due volte.
func TestUnCodiceNuovoSiAggiungeConIlTipoScelto(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	m := b.mail()
	b.candidato(m, "77760000", "", "prodotto", "famiglia", "corpo")

	for _, sbagliato := range []struct {
		codice string
		tipo   db.TipoComponente
		frase  string
	}{
		{"77769999", db.TipoComponenteSottoassieme, "non è fra i codici trovati"},
		{"77760000", db.TipoComponenteCommerciale, "prodotto, assieme o particolare"},
	} {
		_, err := b.aggiungi(sbagliato.codice, sbagliato.tipo, "")
		deveRifiutare(t, err, sbagliato.frase)
	}
	if got := b.bom(); got != "" {
		t.Fatalf("un rifiuto ha cambiato la BOM: %s", got)
	}
	msg, err := b.aggiungi("77760000", db.TipoComponenteSottoassieme, "")
	ok(t, err)
	if msg != "77760000 entra nella BOM come assieme." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT codice || ':' || tipo || ':' || origine || ':' || coalesce(rev, '-') || ':' || (confermato_da = $2)::text
		FROM componente WHERE thread_id = $1`, b.thread, b.utente); got != "77760000:sottoassieme:codice_rilevato:-:true" {
		t.Errorf("componente = %s", got)
	}
	if s := trovaCodice(t, b.candidati(), "77760000").Stato.Situazione; s != fascicolo.SituazioneComponente {
		t.Errorf("dopo: %s", s)
	}
	_, err = b.aggiungi("77760000", db.TipoComponenteFinito, "")
	deveRifiutare(t, err, "è già nella BOM come assieme: si apre quello")
	if n := uno[int64](b, `SELECT count(*) FROM componente WHERE thread_id = $1`, b.thread); n != 1 {
		t.Errorf("%d componenti", n)
	}
}

// Un codice con un nodo STEP aperto si decide li': «+» lo rifiuta, e la proposta resta com'era; accettare
// il nodo crea il componente, e il codice diventa «nel Fascicolo».
func TestUnCodiceConUnaPropostaApertaSiDecideNellaProposta(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.candidato(b.mail(), "77720517", "", "prodotto", "famiglia", "corpo")
	b.applica(b.allegatoStep("assieme.stp", strings.Repeat("7", 64)),
		fattiSTEP{nodi: []string{"#1=77722757", "#2=77720517"}, archi: []string{"#1>#2*2"}}.json())
	prima := b.nodiProposti()

	_, err := b.aggiungi("77720517", db.TipoComponenteSciolto, "")
	deveRifiutare(t, err, "ha una proposta aperta dallo STEP assieme.stp (nodo «77720517»): si decide quella")
	if b.bom() != "" || b.nodiProposti() != prima {
		t.Fatalf("il rifiuto ha cambiato qualcosa: bom %q, proposte %q", b.bom(), b.nodiProposti())
	}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.AccettaNodo(b.ctx, q, b.thread, b.proposta("#2"), b.utente, "")
	})
	ok(t, err)
	if s := trovaCodice(t, b.candidati(), "77720517").Stato; s.Situazione != fascicolo.SituazioneComponente || s.Componente.Origine != db.OrigineComponenteStep {
		t.Errorf("dopo l'accettazione: %s, %+v", s.Situazione, s.Componente)
	}
}

// Un codice di un componente archiviato non ne crea un altro: si ripristina, lo stesso, con la sua storia.
// Il ripristino ritrova anche le proposte di nodo ancora aperte con quel codice.
func TestUnCodiceArchiviatoSiRipristinaNonSiRicrea(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.candidato(b.mail(), "77731111", "", "prodotto", "famiglia", "corpo")
	comp := b.componente("77731111", db.TipoComponenteSciolto)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ArchiviaComponente(b.ctx, q, b.thread, comp, b.utente, "tolto dal cliente")
	})
	ok(t, err)

	_, err = b.aggiungi("77731111", db.TipoComponenteSciolto, "")
	deveRifiutare(t, err, "è archiviato: si ripristina")
	if n := uno[int64](b, `SELECT count(*) FROM componente WHERE thread_id = $1`, b.thread); n != 1 {
		t.Fatalf("%d componenti: il rifiuto ne ha creato un altro", n)
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
	if got := uno[string](b, `SELECT (archiviato_il IS NULL)::text || ':' || componente_id::text FROM componente WHERE thread_id = $1`, b.thread); got != "true:"+comp.String() {
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

// Revisioni diverse fra le evidenze: il componente non nasce finche' chi aggiunge non sceglie, e sceglie
// fra quelle viste. Il punteggio piu' alto non decide.
func TestLeRevisioniDiscordantiVoglionoLaScelta(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	m := b.mail()
	b.candidato(m, "77770000", "A", "prodotto", "famiglia", "oggetto")
	b.fileConProposta(m, "77770000_B.pdf", "disegno_2d", "77770000", "B", "cartiglio", "aperta", "")

	k := trovaCodice(t, b.candidati(), "77770000")
	if !k.Conflitto || len(k.Revisioni) != 2 {
		t.Fatalf("revisioni = %+v", k.Revisioni)
	}
	_, err := b.aggiungi("77770000", db.TipoComponenteSciolto, "")
	deveRifiutare(t, err, "revisioni discordanti")
	_, err = b.aggiungi("77770000", db.TipoComponenteSciolto, "C")
	deveRifiutare(t, err, "la revisione C non è fra quelle viste")
	if b.bom() != "" {
		t.Fatalf("un rifiuto ha cambiato la BOM: %s", b.bom())
	}
	msg, err := b.aggiungi("77770000", db.TipoComponenteSciolto, "a")
	ok(t, err)
	if msg != "77770000 entra nella BOM come particolare, rev A." {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT coalesce(rev, '-') FROM componente WHERE thread_id = $1`, b.thread); got != "A" {
		t.Errorf("rev del componente = %s", got)
	}
}

// Un codice della richiesta e' gia' deciso come prodotto: da qui entra come prodotto e basta.
func TestUnCodiceDellaRichiestaEntraComeProdotto(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.candidato(b.mail(), "77780000", "", "prodotto", "famiglia", "oggetto")
	b.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, '77780000', 'proposta_famiglia')`, b.thread)

	_, err := b.aggiungi("77780000", db.TipoComponenteSottoassieme, "")
	deveRifiutare(t, err, "è un codice della richiesta: entra come prodotto")
	msg, err := b.aggiungi("77780000", db.TipoComponenteFinito, "")
	ok(t, err)
	if msg != "77780000 entra nella BOM come prodotto." {
		t.Errorf("messaggio: %q", msg)
	}
}

// Con la BOM congelata i codici si leggono, con la loro situazione, ma niente li porta nella working:
// ne' «+», ne' il ripristino (D26).
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
		if s := trovaCodice(t, c, codice).Stato; s.Situazione != attesa || len(s.Tipi) != 0 {
			t.Errorf("%s: %s con tipi %v", codice, s.Situazione, s.Tipi)
		}
	}
	_, err = b.aggiungi("77790000", db.TipoComponenteSciolto, "")
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	_, err = b.gesto(func(q *db.Queries) (string, error) { return fascicolo.RipristinaComponente(b.ctx, q, b.thread, arch) })
	deveRifiutare(t, err, "la BOM è congelata nella V1")
	if b.bom() != prima {
		t.Errorf("la BOM congelata e' cambiata:\nprima %s\ndopo  %s", prima, b.bom())
	}
	_ = r
}

// D16 chiusa (24/09/2026): la radice di famiglia scrive fonte = 'regola_cliente' e regola_id NULL, anche
// se la proposta ne aveva uno; famiglia, dove e testo del riconoscimento stanno nei dettagli.
func TestLaRadiceDiFamigliaNonScriveUnaRegolaDellaTabellaRegola(t *testing.T) {
	b := nuovoBanco(t)
	b.conFamiglie()
	b.esegui(`INSERT INTO regola (regola_id, descrizione) VALUES ('esempio.nome_file', 'il codice dal nome del file') ON CONFLICT DO NOTHING`)
	a := b.allegatoStep("assieme 7.stp", strings.Repeat("4", 64))
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, regola_id)
		VALUES ($1, $2, 'cad_3d', 'ASSIEME 7', 40, 'nome_file', 'esempio.nome_file')`, a.AllegatoID, b.thread)
	b.applica(a, fattiSTEP{nodi: []string{"#1=77722757_B", "#2=77720517"}, archi: []string{"#1>#2"}}.json())
	got := uno[string](b, `SELECT codice || ':' || coalesce(rev, '-') || ':' || fonte || ':' || coalesce(regola_id, 'NULL') || ':' ||
		(dettagli ->> 'famiglia') || ':' || (dettagli ->> 'dove') || ':' || (dettagli ->> 'testo') FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
	if got != "77722757:B:regola_cliente:NULL:disegni 777:id:77722757_B" {
		t.Errorf("proposta del documento = %q", got)
	}
}
