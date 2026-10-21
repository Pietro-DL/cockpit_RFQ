//go:build integrazione

// L4 — R5 formato 3 completo (direttiva 39, blocco «R5 completo» della corsia persistenza; EM §7.5, §7.6, §9.5; direttiva 35
// D-ASS-01, D-ASS-02, prove 3, 4, 8, 10), sul database di prova:
//   - la bozza nata da S8 con il registro d'importazione: la PB iniziale del motore diventa in R5 una AR con provenienza_pb
//     motore e importazione_id del registro d'origine; la voce della proposta tolta dalla bozza senza decisione (assenza):
//     R4 ne dichiara le righe da chiudere, R5 le chiude con la nota fissa e ne scrive la traccia in
//     conferma_strutturale_non_incluso (origine assenza), le esclusioni legacy solo con la traccia.
// Dati sintetici (ACME, codici ACM-xxxx).

package distintabozza

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// registroAltroDB: il registro di prova (registroProva3) per l'altro finito della scena (ACM-6000), sullo STEP stp (sha) con
// il PDF pdf proposto per il telaio #7 (ACM-6610): la radice e' il prodotto registrato, #9 ha un codice proposto (ACM-6611),
// #11 e' il pezzo che la bozza togliera' (l'assenza), il cordone #31 non c'e'.
func registroAltroDB(t *testing.T, s scenaDB, stp uuid.UUID, sha string, pdf uuid.UUID, shaPdf string) importazione.Registro {
	t.Helper()
	id := idProva3{thread: s.thread, prodotto: s.altroFinito, cliente: idDi("cliente"), allegato: stp, pdf: pdf, sha: sha, shaPdf: shaPdf, utente: s.utente}
	c := registroProva3(t, id)
	cliente := id.clienteE()
	var nodi []importazione.Voce
	for _, v := range c.Proposta.Nodi {
		switch v.Origine.Chiave {
		case "#1":
			v.IdentitaProposta = rawDi(t, letturaProva3(stp, "l:u:step:#1:id:acme-num/sei:0-8", cliente, "acme", "ACM-6000", map[string]any{}, nil))
			v.CodiceProposto, v.RevProposta = "ACM-6000", ""
			v.CodiceComposto = rawDi(t, map[string]any{"testo": "ACM-6000", "motivo": "cartiglio"})
			v.DecisioneEsistente = rawDi(t, map[string]any{"componente_id": s.altroFinito.String(), "codice": "ACM-6000"})
			v.Riferimenti = nil
		case "#7":
			v.IdentitaProposta = rawDi(t, letturaProva3(stp, "l:u:step:#7:id:acme-num/sei:0-8", cliente, "acme", "ACM-6610", map[string]any{}, nil))
			v.CodiceProposto, v.RevProposta = "ACM-6610", ""
		case "#9":
			v.IdentitaProposta = rawDi(t, letturaProva3(stp, "l:u:step:#9:id:acme-num/sei:0-8", cliente, "acme", "ACM-6611", map[string]any{}, nil))
			v.CodiceProposto, v.RevProposta = "ACM-6611", ""
			v.MotivoProposto, v.MotivoLettura, v.Alternative = nil, nil, nil
		case "#31":
			continue
		}
		nodi = append(nodi, v)
	}
	c.Proposta.Nodi = nodi
	var archi []importazione.ArcoRegistro
	for _, a := range c.Proposta.Archi {
		if !strings.HasSuffix(a.Figlio, ":#31") {
			archi = append(archi, a)
		}
	}
	c.Proposta.Archi = archi
	return componi(t, c)
}

// creaStepAltroDB: R2 crea partenza step dell'altro finito con il registro (la proposta calcolata), dopo S8, con i fatti
// correnti dello STEP (le chiavi delle voci, piu' #20).
func creaStepAltroDB(t *testing.T, p *pgxpool.Pool, s scenaDB, stp uuid.UUID, reg importazione.Registro) Creata {
	t.Helper()
	esegui3(t, p, `INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (3, $1)
		ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = 3, hash_configurazione = $1`, hashCfg3)
	esegui3(t, p, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)`,
		reg.Contenuto.Gruppo.Sha256, hashCfg3, fattiStep3(t, "#1", "#7", "#9", "#11", "#20"))
	r := formaR2DB(t, s.altroFinito, nil, 0, uuid.New(), FormaCrea, map[string]any{"partenza": "step",
		"sorgente": map[string]any{"allegato_id": stp, "gruppo": "bom"}, "impronta_contenuto": reg.Contenuto.ImprontaContenuto})
	c, err := CreaFormato3(context.Background(), p, r.CreaDaRichiesta(s.thread, s.utente, &PropostaCalcolata{Registro: reg}))
	if err != nil {
		t.Fatalf("crea step dell'altro finito: %v", err)
	}
	return c
}

// rigaLegacyDB: una riga componente_proposta dello STEP (portatore stp) per la chiave, nello stato dato.
func rigaLegacyDB(t *testing.T, p *pgxpool.Pool, s scenaDB, stp uuid.UUID, sha, chiave, stato string, persona bool, nota string) uuid.UUID {
	t.Helper()
	var deciso, n any
	if persona {
		deciso = s.utente
	}
	if nota != "" {
		n = nota
	}
	var id uuid.UUID
	if err := p.QueryRow(context.Background(), `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza,
		stato, deciso_da, nota) VALUES ($1, $2, $3, $4, 'pezzo', 'step', 50, $5::stato_proposta, $6, $7) RETURNING proposta_id`,
		s.thread, stp, sha, chiave, stato, deciso, n).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// D-PB03 A, D39 §4 [U]; EM §7.5, §7.8 con U2 §6.1: la bozza dal registro con la PB del motore, il pezzo #11 tolto senza
// decisione (assenza) con le sue righe legacy aperte, una riga scartata da una persona nelle pagine di prima (legacy).
func TestV1D39R5RegistroAssenzaETracceDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	sha := shaProva('d')
	stp := stepDB(t, p, s.thread, "ACM-6000_R.stp", 'd')
	pdf := allegatoFileDB(t, p, s.thread, "ACM-6610.pdf", "ACM-6610", '5')
	s8DB(t, p, s, s.altroFinito, stp, sha, "ACM-6000", true)
	reg := registroAltroDB(t, s, stp, sha, pdf, shaProva('5'))
	c := creaStepAltroDB(t, p, s, stp, reg)
	k7, k11 := ChiaveDellaVoce(stp, "nodo:"+sha+":#7"), ChiaveDellaVoce(stp, "nodo:"+sha+":#11")
	if pb := pbDi(t, c.Snapshot, pdf); pb.Provenienza != ProvenienzaMotore || pb.Importazione != reg.Impronta || pb.ChiaveBozza != k7 {
		t.Fatalf("PB iniziale del motore: %+v", pb)
	}
	// le righe legacy dello STEP: #11 aperta con l'arco che entra (l'assenza le chiude), #20 scartata da una persona (legacy)
	r7 := rigaLegacyDB(t, p, s, stp, sha, "#7", "aperta", false, "")
	r11 := rigaLegacyDB(t, p, s, stp, sha, "#11", "aperta", false, "")
	rigaLegacyDB(t, p, s, stp, sha, "#20", "scartata", true, "doppione nel file")
	esegui3(t, p, `INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta, stato) VALUES ($1, $2, '#7', '#11', 3, 'aperta')`,
		s.thread, stp)
	// R2: #11 tolto senza decisione
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	var nodi, archi []any
	var tolto, arcoTolto map[string]any
	for _, x := range m["nodi"].([]any) {
		if x.(map[string]any)["chiave_bozza"] == k11 {
			tolto = x.(map[string]any)
			continue
		}
		nodi = append(nodi, x)
	}
	for _, x := range m["archi"].([]any) {
		if x.(map[string]any)["figlio"] == k11 {
			arcoTolto = x.(map[string]any)
			continue
		}
		archi = append(archi, x)
	}
	if tolto == nil || arcoTolto == nil {
		t.Fatalf("il nodo #11 non c'e' nella bozza nata dal registro: %v", m["nodi"])
	}
	m["nodi"], m["archi"] = nodi, archi
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{azione3(t, AzArcoTolto, k11, arcoTolto, nil),
		azione3(t, AzNodoTolto, k11, tolto, nil)}); err != nil {
		t.Fatalf("R2 #11 tolto: %v", err)
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente)
	var assenza, legacy *EsclusoR4
	for i, e := range rp.Esclusi {
		switch {
		case e.Origine == OrigineEsclusoAssenza && e.Casella != nil && strings.HasSuffix(e.Casella.Rif, ":#11"):
			assenza = &rp.Esclusi[i]
		case e.Origine == OrigineEsclusoLegacy && e.Casella != nil && strings.HasSuffix(e.Casella.Rif, ":#20"):
			legacy = &rp.Esclusi[i]
		}
	}
	if assenza == nil || assenza.Effetti == nil || len(assenza.Effetti.RigheDaChiudere) != 1 || assenza.Effetti.RigheDaChiudere[0].PropostaID != r11 ||
		len(assenza.Effetti.ArchiDaChiudere) != 1 || legacy == nil {
		t.Fatalf("R4: assenza %+v legacy %+v (esclusi %+v)", assenza, legacy, rp.Esclusi)
	}
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4 non confermabile: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	if out.AssenzeApplicate != 1 || out.AssociazioniApplicate != 1 {
		t.Fatalf("risposta: %+v", out)
	}
	// la AR della PB del motore: provenienza_pb motore, importazione_id del registro d'origine
	var impID uuid.UUID
	if err := p.QueryRow(ctx, `SELECT importazione_id FROM distinta_importazione WHERE impronta = $1`, reg.Impronta).Scan(&impID); err != nil {
		t.Fatal(err)
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND stato = 'attiva'
		AND origine = 'conferma_bom' AND conferma_id = $3 AND provenienza_pb = 'motore' AND importazione_id = $4`, pdf, out.Mappa[k7], out.ConfermaID, impID); n != 1 {
		t.Fatal("la AR della PB del motore con importazione_id")
	}
	// l'assenza: la riga e l'arco chiusi con la nota fissa e il segno, la traccia con le righe chiuse
	var stato, nota, decisione, gesto string
	if err := p.QueryRow(ctx, `SELECT stato::text, nota, evidenza -> 'albero' ->> 'decisione', evidenza -> 'albero' ->> 'gesto'
		FROM componente_proposta WHERE proposta_id = $1`, r11).Scan(&stato, &nota, &decisione, &gesto); err != nil {
		t.Fatal(err)
	}
	if stato != "scartata" || nota != "esclusa con la conferma cumulativa n. 1, senza motivo scritto" || decisione != "assenza" || gesto != "conferma_bom" {
		t.Fatalf("riga dell'assenza: %s %q %s %s", stato, nota, decisione, gesto)
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_proposta WHERE proposta_id = $1 AND stato = 'confermata' AND componente_id = $2`, r7, out.Mappa[k7]); n != 1 {
		t.Fatal("la riga del telaio tenuto segnata da R5")
	}
	if n := conta3(t, p, `SELECT count(*) FROM relazione_proposta WHERE allegato_id = $1 AND figlio_chiave = '#11' AND stato = 'scartata'`, stp); n != 1 {
		t.Fatal("l'arco dell'assenza chiuso")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_non_incluso WHERE conferma_id = $1 AND origine = 'assenza' AND chiave = '#11'
		AND decisione_id IS NULL AND motivo IS NULL AND righe_chiuse = ARRAY[$2]::uuid[] AND jsonb_array_length(archi_chiusi) = 1`, out.ConfermaID, r11); n != 1 {
		t.Fatal("la traccia dell'assenza")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_non_incluso WHERE conferma_id = $1 AND origine = 'legacy' AND chiave = '#20'
		AND motivo = 'doppione nel file' AND righe_chiuse = '{}'`, out.ConfermaID); n != 1 {
		t.Fatal("la traccia dell'esclusione legacy")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_non_incluso WHERE conferma_id = $1 AND origine = 'non_includere'`, out.ConfermaID); n != 0 {
		t.Fatal("nessuna non_includere")
	}
}

// togliNodo3: lo snapshot senza il nodo k e i suoi archi, con le azioni (archi tolti, poi il nodo).
func togliNodo3(t *testing.T, m map[string]any, k string) (map[string]any, []Azione) {
	t.Helper()
	c := copiaProfonda(t, m)
	var nodi, archi []any
	var az []Azione
	var nodo map[string]any
	for _, x := range c["nodi"].([]any) {
		if x.(map[string]any)["chiave_bozza"] == k {
			nodo = x.(map[string]any)
			continue
		}
		nodi = append(nodi, x)
	}
	for _, x := range c["archi"].([]any) {
		a := x.(map[string]any)
		if a["figlio"] == k || a["padre"] == k {
			az = append(az, azione3(t, AzArcoTolto, a["figlio"].(string), a, nil))
			continue
		}
		archi = append(archi, x)
	}
	if nodo == nil {
		t.Fatalf("il nodo %s non c'e'", k)
	}
	if archi == nil {
		archi = []any{}
	}
	c["nodi"], c["archi"] = nodi, archi
	return c, append(az, azione3(t, AzNodoTolto, k, nodo, nil))
}

// EM §7.5 con U2 §6.1, U2-P28: le tracce ereditata (la non_includere della conferma di base che la bozza nuova non ripete) ed
// escludi (il registrato escluso, con la casella del suo nodo), senza chiudere righe; la decisione si cita solo se e' dello
// snapshot.
func TestV1D39R5TracceEreditataEdEscludiDB(t *testing.T) {
	p, s := poolPulito(t)
	sha := shaProva('d')
	stp := stepDB(t, p, s.thread, "ACM-6000_R.stp", 'd')
	pdf := allegatoFileDB(t, p, s.thread, "ACM-6610.pdf", "ACM-6610", '5')
	s8DB(t, p, s, s.altroFinito, stp, sha, "ACM-6000", true)
	reg := registroAltroDB(t, s, stp, sha, pdf, shaProva('5'))
	c := creaStepAltroDB(t, p, s, stp, reg)
	k7, k9, k11 := ChiaveDellaVoce(stp, "nodo:"+sha+":#7"), ChiaveDellaVoce(stp, "nodo:"+sha+":#9"), ChiaveDellaVoce(stp, "nodo:"+sha+":#11")
	for _, ch := range []string{"#7", "#9", "#11"} {
		rigaLegacyDB(t, p, s, stp, sha, ch, "aperta", false, "")
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	// la conferma 1: #9 non incluso (con la sua decisione), #11 non incluso, #7 particolare
	azione := uuid.New().String()
	m1, az1 := togliNodo3(t, m, k9)
	m2, az2 := togliNodo3(t, m1, k11)
	ni := func(id, ch string) map[string]any {
		return map[string]any{"id": id, "tipo": "non_includere", "casella": map[string]any{"allegato_id": stp.String(), "gruppo": "bom", "rif": "nodo:" + sha + ":" + ch},
			"motivo_codice": "altro", "motivo": "non serve nella BOM", "azione_id": azione, "importazione": reg.Impronta}
	}
	d1 := map[string]any{"id": "d1-7", "tipo": "d1_a_particolare", "chiave_bozza": k7}
	m2["decisioni"] = append(m2["decisioni"].([]any), ni("ni-9", "#9"), ni("ni-11", "#11"), d1)
	az := append(append([]Azione{}, az1...), az2...)
	az = append(az, azione3(t, AzDecisioneAggiunta, "", nil, ni("ni-9", "#9")), azione3(t, AzDecisioneAggiunta, "", nil, ni("ni-11", "#11")),
		azione3(t, AzDecisioneAggiunta, "", nil, d1))
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m2, az); err != nil {
		t.Fatalf("R2 della prima conferma: %v", err)
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 {
		t.Fatalf("R4 della prima conferma: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	c1, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("conferma 1: %v", err)
	}
	if c1.NonInclusiApplicati != 2 {
		t.Fatalf("conferma 1: %+v", c1)
	}
	pezzo := c1.Mappa[k7]
	// la conferma 2 (da-conferma-1): la bozza non ripete le non_includere della base; il telaio registrato escluso
	b2, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	n2, _ := snapshotDellaVersione(t, p, b2.BozzaID, 1)
	for _, x := range n2["decisioni"].([]any) {
		if x.(map[string]any)["tipo"] == "non_includere" {
			t.Logf("da-conferma-1 ripete la non_includere %v", x.(map[string]any)["id"])
		}
	}
	kp := ""
	for _, x := range n2["nodi"].([]any) {
		if id, _ := x.(map[string]any)["componente_id"].(string); id == pezzo.String() {
			kp = x.(map[string]any)["chiave_bozza"].(string)
			x.(map[string]any)["op"] = OpEsclude
		}
	}
	if kp == "" {
		t.Fatalf("il telaio registrato non c'e' nella bozza da-conferma-1: %v", n2["nodi"])
	}
	ex := map[string]any{"id": "ex-7", "tipo": "escludi", "componente_id": pezzo.String(), "motivo": "non fa parte del prodotto"}
	n2["decisioni"] = append(n2["decisioni"].([]any), ex)
	var nodoPrima map[string]any
	m1b, _ := snapshotDellaVersione(t, p, b2.BozzaID, 1)
	nodoPrima = nodoIn3(m1b, kp)
	if _, err := sc.salva3Su(t, b2.BozzaID, s.altroFinito, 1, uuid.New(), n2, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ex),
		azione3(t, AzNodoModificato, kp, map[string]any{"op": nodoPrima["op"]}, map[string]any{"op": OpEsclude})}); err != nil {
		t.Fatalf("R2 dell'esclusione: %v", err)
	}
	rp2 := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, b2.BozzaID, 2), s.utente)
	if len(rp2.Blocchi) != 0 {
		t.Fatalf("R4 della seconda conferma: %v %+v", codici(rp2.Blocchi), rp2.Blocchi)
	}
	c2, err := confermaR5DB(t, p, s, richiestaR5(t, p, b2.BozzaID, 2, rp2.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("conferma 2: %v", err)
	}
	if len(c2.IdentitaArchiviate) != 1 || c2.IdentitaArchiviate[0] != pezzo {
		t.Fatalf("conferma 2: %+v", c2)
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_non_incluso WHERE conferma_id = $1 AND origine = 'ereditata' AND chiave IN ('#9', '#11')
		AND decisione_id IS NULL AND motivo_codice = 'altro' AND motivo = 'non serve nella BOM' AND righe_chiuse = '{}'`, c2.ConfermaID); n != 2 {
		t.Fatalf("tracce ereditate: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_non_incluso WHERE conferma_id = $1 AND origine = 'escludi' AND chiave = '#7'
		AND decisione_id = 'ex-7' AND motivo = 'non fa parte del prodotto'`, c2.ConfermaID); n != 1 {
		t.Fatal("traccia dell'escluso")
	}
}
