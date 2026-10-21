//go:build integrazione

// L4 — direttiva 35, blocco C-2 (corsia persistenza): le preassociazioni del formato 3 per allegato_id sul database di
// prova, nel package che le rotte R1 e R2 usano:
//   - la Prova 2 della direttiva a livello di R2: un allegato della RFQ senza documento_proposta si preassocia; R1 lo
//     rilegge e D-6 (ListDipendenzeBozzaPerFile) lo dice «preassociazione» dal ramo del formato 3;
//   - le verifiche della PB nuova o spostata: FUORI_RFQ, natura_non_preassociabile (contenitore, inline),
//     legato_come_sorgente per lo STEP sorgente (direttiva 39 §5, D-2 (c) [U]: uno STEP documentale si preassocia),
//     FILE_NON_ATTIVO anche per lo scarto col vecchio «Scarta il file», lo stesso
//     allegato due volte (provvisorio, base [T] D-ASS-04 A); una PB gia' salvata e scaduta dopo passa, spostata no;
//   - lo svincolo {allegato_id, componente_id} cita una AR attiva (EM §3.6.2): senza AR o con la AR di un altro
//     componente 422 svincolo_senza_associazione; D-6 lo dice «svincolo»;
//   - la conversione formato 2 → 3: la proposta che non e' della RFQ ferma la conversione (la bozza formato 2 resta
//     aperta); l'occorrenza non attiva si copia e si segnala;
//   - la rilettura di una versione e di una conferma formato 2 gia' salvate, con {proposta_id}: come prima (R1, R7).
// Solo dati sintetici: la scena di persistenza_db_test.go e gli aiuti di formato3_aiuti_test.go e ondatab_db_test.go.

package distintabozza

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// salvaPB: la scena con un salvataggio alla volta sulla versione corrente; conPre aggiunge una PB con il suo evento.
type salvaPB struct {
	sc       scena3DB
	versione int
	cur      map[string]any
}

func nuovoSalvaPB(t *testing.T) *salvaPB {
	t.Helper()
	sc := preparaBozza3(t)
	return &salvaPB{sc: sc, versione: 1, cur: copiaProfonda(t, sc.base)}
}

func (s *salvaPB) salva(t *testing.T, snap map[string]any, azioni ...Azione) (Salvato3, error) {
	t.Helper()
	r, err := s.sc.salva3(t, s.versione, uuid.New(), snap, azioni)
	if err == nil && r.NuovaVersione {
		s.versione, s.cur = r.Versione, snap
	}
	return r, err
}

func (s *salvaPB) conPre(t *testing.T, allegato uuid.UUID, chiave string) (map[string]any, Azione) {
	t.Helper()
	snap := copiaProfonda(t, s.cur)
	x := map[string]any{"allegato_id": allegato.String(), "chiave_bozza": chiave}
	snap["documenti_preassociati"] = append(snap["documenti_preassociati"].([]any), x)
	return snap, azione3(t, AzFilePreassociato, chiave, nil, x)
}

// dipendenzeD6: i tipi di D-6 per allegato della bozza della scena.
func dipendenzeD6(t *testing.T, sc scena3DB) map[uuid.UUID]map[string]bool {
	t.Helper()
	dd, err := db.New(sc.p).ListDipendenzeBozzaPerFile(context.Background(), sc.thread)
	if err != nil {
		t.Fatal(err)
	}
	out := map[uuid.UUID]map[string]bool{}
	for _, d := range dd {
		if d.BozzaID != sc.bozza {
			continue
		}
		if out[d.AllegatoID] == nil {
			out[d.AllegatoID] = map[string]bool{}
		}
		out[d.AllegatoID][d.Tipo] = true
	}
	return out
}

// La Prova 2 della direttiva 35 a livello di R2: un PDF della RFQ senza riga di documento_proposta si preassocia.
func TestV1C2LaPreassociazioneSenzaProposta(t *testing.T) {
	s := nuovoSalvaPB(t)
	pdf := nuovoAllegato(t, s.sc.p, idDi("msg-0"), 3, "ACM-1109.pdf", strings.Repeat("d", 64))
	if n := conta3(t, s.sc.p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1`, pdf); n != 0 {
		t.Fatalf("la prova vuole un allegato senza documento_proposta: %d", n)
	}
	snap, a := s.conPre(t, pdf, "k-0009")
	r, err := s.salva(t, snap, a)
	if err != nil || r.Versione != 2 || r.Eventi != 1 {
		t.Fatalf("PB senza proposta: %+v %v", r, err)
	}
	l := s.sc.leggiR1(t, OpzioniR1{Eventi: true})
	// la provenienza la decide il server (direttiva 39 §4): una PB nuova senza registro e' manuale, nello snapshot e nell'evento
	if !strings.Contains(string(l.Snapshot), `{"allegato_id":"`+pdf.String()+`","chiave_bozza":"k-0009","provenienza":"manuale"}`) || strings.Contains(string(l.Snapshot), "proposta_id") {
		t.Fatalf("R1 dopo la PB: %s", l.Snapshot)
	}
	ultimo := l.Eventi[len(l.Eventi)-1]
	if ultimo.Azione != AzFilePreassociato || string(ultimo.Dopo) != `{"allegato_id":"`+pdf.String()+`","chiave_bozza":"k-0009","provenienza":"manuale"}` || ultimo.Chiave != "k-0009" {
		t.Fatalf("evento file_preassociato: %+v %s", ultimo, ultimo.Dopo)
	}
	// D-6 dal ramo del formato 3: la PB salvata e' una dipendenza dell'occorrenza, senza passare da documento_proposta
	if d := dipendenzeD6(t, s.sc); !d[pdf]["preassociazione"] || !d[s.sc.id.pdf]["preassociazione"] {
		t.Fatalf("D-6 delle PB: %+v", d)
	}
	// nessuna riga di documento_proposta nasce dal salvataggio (R2 non scrive proposte, C3 §6.2)
	if n := conta3(t, s.sc.p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1`, pdf); n != 0 {
		t.Fatalf("R2 ha scritto una proposta: %d", n)
	}
	// la PB si sposta su un altro nodo con svincolo e preassociazione (una sola PB per allegato)
	sposta := copiaProfonda(t, s.cur)
	for _, p := range sposta["documenti_preassociati"].([]any) {
		if m := p.(map[string]any); m["allegato_id"] == pdf.String() {
			m["chiave_bozza"] = "k-0011"
		}
	}
	if _, err := s.salva(t, sposta,
		azione3(t, AzFileSvincolato, "k-0009", map[string]any{"allegato_id": pdf.String(), "chiave_bozza": "k-0009"},
			map[string]any{"allegato_id": pdf.String(), "motivo": SvincoloUtente}),
		azione3(t, AzFilePreassociato, "k-0011", nil, map[string]any{"allegato_id": pdf.String(), "chiave_bozza": "k-0011"})); err != nil {
		t.Fatalf("spostamento della PB: %v", err)
	}
	// file_preassociato di un allegato gia' preassociato, senza lo svincolo: le azioni non danno lo snapshot
	doppio := copiaProfonda(t, s.cur)
	_, err = s.salva(t, doppio, azione3(t, AzFilePreassociato, "k-0009", nil, map[string]any{"allegato_id": pdf.String(), "chiave_bozza": "k-0009"}))
	codice3(t, err, CodiceCorpoNonValido, MotivoAzioniNonRiconciliate)
	// lo stesso allegato due volte nello snapshot: 422 CORPO_NON_VALIDO (provvisorio, base [T] D-ASS-04 A)
	due, a2 := s.conPre(t, pdf, "k-0009")
	e := codice3(t, func() error { _, err := s.salva(t, due, a2); return err }(), CodiceCorpoNonValido, "")
	if !strings.Contains(e.Campo, "documenti_preassociati[") || !strings.Contains(e.Campo, "allegato_id") {
		t.Fatalf("campo dello stesso allegato due volte: %+v", e)
	}
}

// Le verifiche della PB nuova o spostata contro il database; una PB gia' salvata e scaduta dopo passa.
func TestV1C2LeVerificheDellaPreassociazione(t *testing.T) {
	s := nuovoSalvaPB(t)
	sc := s.sc
	prova := func(nome string, allegato uuid.UUID, codice, motivo string) {
		t.Helper()
		snap, a := s.conPre(t, allegato, "k-0009")
		prima := conta3(t, sc.p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, sc.bozza)
		_, err := s.salva(t, snap, a)
		e := codice3(t, err, codice, motivo)
		if !strings.HasPrefix(e.Campo, "snapshot.documenti_preassociati[") {
			t.Errorf("%s: campo %q", nome, e.Campo)
		}
		if dopo := conta3(t, sc.p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, sc.bozza); dopo != prima {
			t.Errorf("%s: il rifiuto ha scritto una versione", nome)
		}
	}
	// un allegato di un'altra RFQ, uno che non c'e'
	prova("altra RFQ", sc.allegatoT2, CodiceFuoriRFQ, "")
	prova("allegato inesistente", uuid.MustParse("00000000-0000-4000-8000-0000000a0fff"), CodiceFuoriRFQ, "")
	// direttiva 39 §5, D-2 (c) [U]: lo STEP sorgente della bozza no (legato_come_sorgente); ne' contenitori ne' altre nature
	prova("STEP sorgente", sc.id.allegato, CodiceCorpoNonValido, MotivoLegatoComeSorgente)
	zip := nuovoAllegato(t, sc.p, idDi("msg-0"), 5, "ACM-ARCH.zip", strings.Repeat("5", 64))
	voce := uuid.New()
	esegui3(t, sc.p, `INSERT INTO allegato (allegato_id, messaggio_id, contenitore_id, indice, nome_file, estensione, sha256, ricevuto_il)
		VALUES ($1, $2, $3, 1, 'ACM-1102.pdf', 'pdf', repeat('6', 64), now())`, voce, idDi("msg-0"), zip)
	prova("contenitore", zip, CodiceCorpoNonValido, MotivoNaturaNonPreassociabile)
	inline := uuid.New()
	esegui3(t, sc.p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, natura, sha256, ricevuto_il)
		VALUES ($1, $2, 6, 'firma.png', 'png', 'inline', repeat('7', 64), now())`, inline, idDi("msg-0"))
	prova("inline", inline, CodiceCorpoNonValido, MotivoNaturaNonPreassociabile)
	// la voce estratta dall'archivio e' un file: si preassocia
	snap, a := s.conPre(t, voce, "k-0009")
	if _, err := s.salva(t, snap, a); err != nil {
		t.Fatalf("voce estratta da un archivio: %v", err)
	}
	// uno STEP documentale (di una parte, non legato come sorgente) si preassocia come un PDF (D-2 (c) [U])
	stepDoc := nuovoAllegato(t, sc.p, idDi("msg-0"), 10, "ACM-1102.stp", strings.Repeat("ab", 32))
	snap, a = s.conPre(t, stepDoc, "k-0009")
	if _, err := s.salva(t, snap, a); err != nil {
		t.Fatalf("STEP documentale: %v", err)
	}
	// la provenienza della PB la decide il server (direttiva 39 §4, R39-24): una PB nuova che la porta dalla pagina no
	// (controllaDichiarate, prima della riconciliazione: provenienza_dal_server)
	conProv := nuovoAllegato(t, sc.p, idDi("msg-0"), 11, "ACM-1106.pdf", strings.Repeat("cd", 32))
	xProv := map[string]any{"allegato_id": conProv.String(), "chiave_bozza": "k-0009", "provenienza": "motore"}
	snapProv := copiaProfonda(t, s.cur)
	snapProv["documenti_preassociati"] = append(snapProv["documenti_preassociati"].([]any), xProv)
	_, errProv := s.salva(t, snapProv, azione3(t, AzFilePreassociato, "k-0009", nil, xProv))
	codice3(t, errProv, CodiceCorpoNonValido, MotivoProvenienzaDelServer)
	// lo scarto col vecchio «Scarta il file»: la proposta scartata da una persona, nessuna riga del ciclo
	vecchio := nuovoAllegato(t, sc.p, idDi("msg-0"), 7, "ACM-1103.pdf", strings.Repeat("8", 64))
	pv := nuovaProposta(t, sc.p, vecchio, sc.thread, nil)
	esegui3(t, sc.p, `UPDATE documento_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now() WHERE proposta_id = $1`, pv, sc.utente)
	prova("scarto legacy", vecchio, CodiceFileNonAttivo, "")
	// una PB gia' salvata, il cui file va poi in Altri file: un salvataggio che non la tocca passa; spostarla no
	altri := nuovoAllegato(t, sc.p, idDi("msg-0"), 8, "ACM-1104.pdf", strings.Repeat("9", 64))
	snap, a = s.conPre(t, altri, "k-0011")
	if _, err := s.salva(t, snap, a); err != nil {
		t.Fatalf("PB prima di Altri file: %v", err)
	}
	inAltriFile(t, sc.p, altri, sc.utente)
	tocca := copiaProfonda(t, s.cur)
	nodoIn3(tocca, "k-man")["descrizione"] = "Vite di prova"
	if _, err := s.salva(t, tocca, azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil}, map[string]any{"descrizione": "Vite di prova"})); err != nil {
		t.Fatalf("salvataggio che non tocca la PB scaduta: %v", err)
	}
	sposta := copiaProfonda(t, s.cur)
	for _, p := range sposta["documenti_preassociati"].([]any) {
		if m := p.(map[string]any); m["allegato_id"] == altri.String() {
			m["chiave_bozza"] = "k-0009"
		}
	}
	_, err := s.salva(t, sposta,
		azione3(t, AzFileSvincolato, "k-0011", map[string]any{"allegato_id": altri.String(), "chiave_bozza": "k-0011"},
			map[string]any{"allegato_id": altri.String(), "motivo": SvincoloUtente}),
		azione3(t, AzFilePreassociato, "k-0009", nil, map[string]any{"allegato_id": altri.String(), "chiave_bozza": "k-0009"}))
	codice3(t, err, CodiceFileNonAttivo, "")
	// la forma di prima della direttiva 35 ({proposta_id}) e' rifiutata da R2
	vecchiaForma := copiaProfonda(t, s.cur)
	pp := nuovaProposta(t, sc.p, nuovoAllegato(t, sc.p, idDi("msg-0"), 9, "ACM-1105.pdf", strings.Repeat("0", 64)), sc.thread, nil)
	x := map[string]any{"proposta_id": pp.String(), "chiave_bozza": "k-0009"}
	vecchiaForma["documenti_preassociati"] = append(vecchiaForma["documenti_preassociati"].([]any), x)
	_, err = s.salva(t, vecchiaForma, azione3(t, AzFilePreassociato, "k-0009", nil, x))
	codice3(t, err, CodiceCorpoNonValido, "")
}

// Lo svincolo scollega_documento {allegato_id, componente_id} cita una AR attiva (EM §3.6.2; provvisorio, base [T] D-4 A).
func TestV1C2LoSvincoloCitaUnaARAttiva(t *testing.T) {
	s := nuovoSalvaPB(t)
	sc := s.sc
	comp, altroComp := idDi("k-p1-1"), idDi("k-s1")
	pdf := nuovoAllegato(t, sc.p, idDi("msg-0"), 4, "ACM-1101.pdf", strings.Repeat("f", 64))
	svincolo := func(id string, allegato, componente uuid.UUID) (map[string]any, Azione) {
		d := map[string]any{"id": id, "tipo": "scollega_documento", "allegato_id": allegato.String(), "componente_id": componente.String()}
		snap := copiaProfonda(t, s.cur)
		snap["decisioni"] = append(snap["decisioni"].([]any), d)
		return snap, azione3(t, AzDecisioneAggiunta, "", nil, d)
	}
	// senza AR: anche se la copia di documento_proposta dice il componente (pre-assegnazione legacy, INC-S2/INC-S3)
	nuovaProposta(t, sc.p, pdf, sc.thread, &comp)
	snap, a := svincolo("sv-0010", pdf, comp)
	_, err := s.salva(t, snap, a)
	e := codice3(t, err, CodiceCorpoNonValido, MotivoSvincoloSenzaAssociazione)
	if !strings.HasPrefix(e.Campo, "snapshot.decisioni[") {
		t.Fatalf("campo dello svincolo senza AR: %+v", e)
	}
	// la AR attiva e' di un altro componente
	arAttivaDiProva(t, sc.p, sc.thread, sc.prodotto, pdf, comp, sc.utente)
	if n := conta3(t, sc.p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND thread_id = $2`, altroComp, sc.thread); n != 1 {
		t.Fatalf("il secondo componente della scena: %d", n)
	}
	snap, a = svincolo("sv-0010", pdf, altroComp)
	_, err = s.salva(t, snap, a)
	codice3(t, err, CodiceCorpoNonValido, MotivoSvincoloSenzaAssociazione)
	// un allegato di un'altra RFQ
	snap, a = svincolo("sv-0010", sc.allegatoT2, comp)
	_, err = s.salva(t, snap, a)
	codice3(t, err, CodiceFuoriRFQ, "")
	// la AR attiva della coppia: lo svincolo si salva, e D-6 lo dice «svincolo» dell'occorrenza
	snap, a = svincolo("sv-0010", pdf, comp)
	if _, err := s.salva(t, snap, a); err != nil {
		t.Fatalf("svincolo di una AR attiva: %v", err)
	}
	if d := dipendenzeD6(t, sc); !d[pdf]["svincolo"] {
		t.Fatalf("D-6 dello svincolo: %+v", d)
	}
	// gia' salvato, lo svincolo non si ricontrolla: la AR chiusa dopo (Scollega e scarta, D-ASS-03) non ferma R2
	esegui3(t, sc.p, `ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`)
	esegui3(t, sc.p, `UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_il = now(), chiusa_da = $2, motivo_chiusura = 'scollega_file', chiusa_tx = pg_current_xact_id()
		WHERE allegato_id = $1`, pdf, sc.utente)
	esegui3(t, sc.p, `ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`)
	tocca := copiaProfonda(t, s.cur)
	nodoIn3(tocca, "k-man")["descrizione"] = "Vite di prova"
	if _, err := s.salva(t, tocca, azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil}, map[string]any{"descrizione": "Vite di prova"})); err != nil {
		t.Fatalf("salvataggio con lo svincolo gia' salvato e la AR chiusa: %v", err)
	}
}

// La conversione formato 2 → 3: l'unico punto di traduzione delle PB (PD35 §6.3).
func TestV1C2LaConversioneTraduceLePreassociazioni(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	conv := func(b uuid.UUID) (Convertita, error) {
		return Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: b, VersioneAttesa: 1,
			Motivo: "si passa al formato 3", ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("6", 64)})
	}
	bozza := func(pre ...Preassociazione2) uuid.UUID {
		sn := soloRadice(s.prodotto, "ACM-1000")
		sn.Nodi = append(sn.Nodi, Nodo{ChiaveBozza: "k-1", Op: OpCrea, Codice: "ACM-1100", Tipo: "sottoassieme", Origine: "utente"})
		sn.Archi = []Arco{{Padre: "r", Figlio: "k-1", Qta: 1, Origine: "utente"}}
		sn.DocumentiPreassociati = pre
		return bozza2DB(t, p, s.thread, s.prodotto, s.utente, sn)
	}
	// la proposta di un'altra RFQ: 422 conversione_proposta_non_risolta, la bozza formato 2 resta aperta
	estranea := nuovaProposta(t, p, s.allegatoT2, s.thread2, nil)
	b := bozza(Preassociazione2{PropostaID: estranea, ChiaveBozza: "k-1"})
	_, err := conv(b)
	codice3(t, err, CodiceCorpoNonValido, MotivoConversionePropostaNonRisolta)
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'aperta'`, b); n != 1 {
		t.Fatalf("la bozza formato 2 non e' piu' aperta dopo il rifiuto: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND formato = 3`, s.thread); n != 0 {
		t.Fatalf("il rifiuto ha scritto una bozza formato 3: %d", n)
	}
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: b, VersioneAttesa: 1, Motivo: "prova", UtenteID: s.utente}); err != nil {
		t.Fatal(err)
	}
	// l'occorrenza in Altri file si copia e si segnala; il conteggio sta nella traccia
	pdf := nuovoAllegato(t, p, idDi("msg-0"), 3, "ACM-1100.pdf", strings.Repeat("d", 64))
	pp := nuovaProposta(t, p, pdf, s.thread, nil)
	inAltriFile(t, p, pdf, s.utente)
	b = bozza(Preassociazione2{PropostaID: pp, ChiaveBozza: "k-1"})
	c, err := conv(b)
	if err != nil {
		t.Fatalf("conversione con l'occorrenza in Altri file: %v", err)
	}
	if c.PreassociazioniTradotte != 1 || len(c.PreassociazioniSegnalate) != 1 || c.PreassociazioniSegnalate[0] != (PreassociazioneSegnalata{
		PropostaID: pp, AllegatoID: pdf, ChiaveBozza: "k-1", Motivo: PreassociazioneFileNonAttivo}) {
		t.Fatalf("traccia: %d %+v", c.PreassociazioniTradotte, c.PreassociazioniSegnalate)
	}
	n3, err := rileggiSnapshot3V(t, p, c.BozzaID, 1)
	if err != nil || len(n3.DocumentiPreassociati) != 1 || n3.DocumentiPreassociati[0] != (Preassociazione3{AllegatoID: pdf, ChiaveBozza: "k-1",
		Provenienza: ProvenienzaManuale}) {
		t.Fatalf("PB della versione 1: %+v %v", n3.DocumentiPreassociati, err)
	}
	// la PB copiata e scaduta resta: R1 la rilegge, e un salvataggio che non la tocca passa
	sc := scena3DB{scenaDB: s, p: p, bozza: c.BozzaID}
	l := sc.leggiR1(t, OpzioniR1{})
	if l.Formato != FormatoSnapshot3 || !strings.Contains(string(l.Snapshot), `"allegato_id":"`+pdf.String()+`"`) {
		t.Fatalf("R1 della bozza convertita: %s", l.Snapshot)
	}
}

// Una versione e una conferma formato 2 gia' salvate, con {proposta_id}, si rileggono come prima (PD35 §6.3).
func TestV1C2IlFormato2SalvatoSiRilegge(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	proposta := nuovaProposta(t, p, nuovoAllegato(t, p, idDi("msg-0"), 3, "ACM-1100.pdf", strings.Repeat("d", 64)), s.thread, nil)
	sn := soloRadice(s.prodotto, "ACM-1000")
	sn.Nodi = append(sn.Nodi, Nodo{ChiaveBozza: "k-1", Op: OpCrea, Codice: "ACM-1100", Tipo: "sottoassieme", Origine: "utente"})
	sn.Archi = []Arco{{Padre: "r", Figlio: "k-1", Qta: 1, Origine: "utente"}}
	sn.DocumentiPreassociati = []Preassociazione2{{PropostaID: proposta, ChiaveBozza: "k-1"}}
	b := bozza2DB(t, p, s.thread, s.prodotto, s.utente, sn)
	var salvato string
	if err := p.QueryRow(ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, b).Scan(&salvato); err != nil {
		t.Fatal(err)
	}
	// R1 della bozza formato 2: lo snapshot salvato, con la PB {proposta_id}, e la conversione da fare
	l, err := leggiR1DB(t, p, s.thread, s.prodotto, OpzioniR1{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Formato != FormatoSnapshot || l.Conversione == nil || !l.Conversione.Necessaria ||
		!strings.Contains(string(l.Snapshot), `"documenti_preassociati":[{"chiave_bozza":"k-1","proposta_id":"`+proposta.String()+`"}]`) {
		t.Fatalf("R1 della bozza formato 2: %+v %s", l, l.Snapshot)
	}
	v, err := db.New(p).GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b, ThreadID: s.thread, Versione: 1})
	if err != nil {
		t.Fatal(err)
	}
	letto, err := RileggiVersione(FormatoSnapshot, v.Snapshot, v.Impronta)
	if err != nil || letto.Formato2 == nil || letto.Formato2.DocumentiPreassociati[0] != (Preassociazione2{PropostaID: proposta, ChiaveBozza: "k-1"}) {
		t.Fatalf("rilettura della versione formato 2: %+v %v", letto, err)
	}
	// una conferma formato 2 con la stessa PB, scritta a mano (trigger della tabella spenti): R7 la rilegge
	can, err := sn.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	conferma := uuid.New()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa,
				criterio_verifica, chiave_idempotenza, confermato_da)
			VALUES ($1, $2, $3, 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64), 0, 0, '{"rotta":"prova"}',
				$4, $5, '{}', '{}', 'nessuno', gen_random_uuid(), $6)`, []any{conferma, s.thread, s.prodotto, can, jsoncanonico.Impronta(can), s.utente}},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rtx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Rollback(ctx) //nolint:errcheck
	st, err := LeggiStorico(ctx, db.New(rtx), s.thread, s.prodotto, OpzioniR7{Conferma: &conferma})
	if err != nil {
		t.Fatal(err)
	}
	if st.Dettaglio == nil || st.Dettaglio.ContenutoNonLeggibile || string(st.Dettaglio.Snapshot) != string(can) {
		t.Fatalf("R7 della conferma formato 2: %+v", st.Dettaglio)
	}
	// la versione salvata non e' cambiata
	var dopo string
	if err := p.QueryRow(ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, b).Scan(&dopo); err != nil || dopo != salvato {
		t.Fatalf("la versione formato 2 e' cambiata: %v", err)
	}
}
