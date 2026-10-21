//go:build integrazione

// L4 — G39-01 [U] (20/10 sera; D-PB02 A, D-PB03 A), corsia persistenza parte A: i file arrivati dopo l'apertura della bozza,
// prima di R5, entrano in preassociazioni_proposte[] solo se il Motore A gli trova una destinazione univoca; il suggerimento
// accettato senza modifiche e' una PB «motore» con il riferimento al calcolo che l'ha proposto (il registro del calcolo di
// adesso, scritto da R2 in distinta_importazione), spostato su un altro componente e' «corretta»; l'ambiguo non entra e,
// accettato con il registro dichiarato, e' rifiutato. Un file arrivato dopo R5 non tocca la bozza confermata e va nella
// revisione successiva. Il calcolo del motore e' un registro di prova al posto del calcolo vero (registroConFigli), come
// nelle altre prove di questo pacchetto; la strada S8 → bozza → R1 → R2 → R4 → R5 e' quella vera. Dati sintetici.

package sorgente

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// leggiR1G39: R1 del prodotto della scena, con il calcolo di adesso se dato.
func leggiR1G39(t *testing.T, p *pgxpool.Pool, s scenaS8, adesso *distintabozza.CalcoloAdesso) distintabozza.LetturaR1 {
	t.Helper()
	l, err := distintabozza.LeggiR1(context.Background(), db.New(p), s.thread, s.prodotto, distintabozza.OpzioniR1{Adesso: adesso})
	if err != nil {
		t.Fatalf("R1: %v", err)
	}
	return l
}

// aggiungiPBG39: R2 salvataggio con una PB in piu' (il registro dichiarato, se dato) e il calcolo di adesso, se dato.
func aggiungiPBG39(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, versione int, a uuid.UUID, chiave, registro string,
	adesso *distintabozza.CalcoloAdesso) (distintabozza.Salvato3, error) {
	t.Helper()
	sv := richiestaPBG39(t, p, s, bozza, versione, a, chiave, registro)
	sv.Adesso = adesso
	return distintabozza.SalvaFormato3(context.Background(), p, sv)
}

func TestV1G39_01FileDopoLAperturaDB(t *testing.T) {
	p, s, regA, bozza, pdf := avvioG39(t)
	s1, _ := snapshotVersione(t, p, bozza, 1)
	radice, k3 := chiaviG39(t, s1, s.prodotto, "nodo:"+s.shaA+":#3")
	if x := pbDiG39(s1, pdf); x.Provenienza != distintabozza.ProvenienzaMotore || x.ChiaveBozza != k3 {
		t.Fatalf("la PB iniziale del PDF: %+v", x)
	}
	// tre file arrivano dopo l'apertura: il calcolo di adesso propone f1 e f2 sul figlio (univoci), f3 sulla radice e sul
	// figlio (ambiguo)
	f1 := pdfDi(t, p, s, "ACM-1150-F2", '7')
	f2 := pdfDi(t, p, s, "ACM-1150-F3", '8')
	f3 := pdfDi(t, p, s, "ACM-1150-F4", '9')
	adesso := conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf, f1, f2, f3}, "#1": {f3}})
	calcolo := &distintabozza.CalcoloAdesso{Registro: &adesso}

	// senza il calcolo di adesso (R1 di prima, D-18): nessuno dei tre e' un suggerimento
	l := leggiR1G39(t, p, s, nil)
	for _, a := range []uuid.UUID{f1, f2, f3} {
		if x, ok := suggerimentoDi(l, a); ok {
			t.Fatalf("senza il calcolo, un file nuovo e' un suggerimento: %+v", x)
		}
	}
	if l.SuggerimentiMotore != nil || l.StepCandidati != nil {
		t.Fatalf("senza il parametro R1 non dice niente del motore: %+v %+v", l.SuggerimentiMotore, l.StepCandidati)
	}
	// con il calcolo di adesso: f1 e f2 sono suggerimenti dopo_apertura con il registro di adesso; f3 (ambiguo) no; il PDF
	// gia' PB nemmeno
	l = leggiR1G39(t, p, s, calcolo)
	for _, a := range []uuid.UUID{f1, f2} {
		x, ok := suggerimentoDi(l, a)
		if !ok || x.ChiaveBozza != k3 || x.Motivo != distintabozza.PropostaDopoApertura || x.Provenienza != distintabozza.ProvenienzaMotore ||
			x.Importazione != adesso.Impronta {
			t.Fatalf("il suggerimento del file %s: %+v (%v) in %+v", a, x, ok, *l.PreassociazioniProposte)
		}
	}
	for _, a := range []uuid.UUID{f3, pdf} {
		if x, ok := suggerimentoDi(l, a); ok {
			t.Fatalf("suggerimento che non deve esserci: %+v", x)
		}
	}
	if l.SuggerimentiMotore == nil || !l.SuggerimentiMotore.Calcolati {
		t.Fatalf("suggerimenti_motore: %+v", l.SuggerimentiMotore)
	}
	// la stessa lista per S9 (SuggerimentiDellaBozzaApertaCon, la funzione per la corsia motore)
	s9, err := distintabozza.SuggerimentiDellaBozzaApertaCon(context.Background(), db.New(p), s.thread, s.prodotto, calcolo)
	if err != nil {
		t.Fatal(err)
	}
	a1, _ := json.Marshal(s9)
	a2, _ := json.Marshal(*l.PreassociazioniProposte)
	if string(a1) != string(a2) {
		t.Fatalf("S9 e R1 diversi: %s / %s", a1, a2)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione WHERE impronta = $1`, adesso.Impronta); n != 0 {
		t.Fatalf("R1 ha scritto il registro di adesso: %d", n)
	}

	// 1. accettato senza modifiche. Senza il calcolo il registro di adesso non si conosce (la strada di prima: 422
	// importazione_sconosciuta, niente scritto); con il calcolo e' una PB «motore» con il registro di adesso, scritto
	_, err = aggiungiPBG39(t, p, s, bozza, 1, f1, k3, adesso.Impronta, nil)
	codiceG39(t, err, distintabozza.CodiceCorpoNonValido, distintabozza.MotivoImportazioneSconosciuta)
	sv, err := aggiungiPBG39(t, p, s, bozza, 1, f1, k3, adesso.Impronta, calcolo)
	if err != nil || sv.Versione != 2 {
		t.Fatalf("suggerimento accettato: %+v %v", sv, err)
	}
	s2, _ := snapshotVersione(t, p, bozza, 2)
	if x := pbDiG39(s2, f1); x.Provenienza != distintabozza.ProvenienzaMotore || x.Importazione != adesso.Impronta || x.ChiaveBozza != k3 {
		t.Fatalf("la PB accettata: %+v", x)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione WHERE thread_id = $1 AND prodotto_id = $2 AND impronta = $3`,
		s.thread, s.prodotto, adesso.Impronta); n != 1 {
		t.Fatalf("il registro di adesso in distinta_importazione: %d", n)
	}
	var dopo []byte
	if err := p.QueryRow(context.Background(), `SELECT dopo FROM distinta_bozza_evento WHERE bozza_id = $1 AND versione = 2 AND azione = 'file_preassociato'`,
		bozza).Scan(&dopo); err != nil {
		t.Fatal(err)
	}
	var ev struct{ Provenienza, Importazione string }
	if err := json.Unmarshal(dopo, &ev); err != nil || ev.Provenienza != distintabozza.ProvenienzaMotore || ev.Importazione != adesso.Impronta {
		t.Fatalf("l'evento della PB accettata: %s", dopo)
	}

	// 2. spostato dall'ingegnere sulla radice: «corretta», con il riferimento al calcolo che l'aveva proposto (il registro
	// ora c'e', il calcolo non serve)
	if _, err := aggiungiPBG39(t, p, s, bozza, 2, f2, radice, adesso.Impronta, nil); err != nil {
		t.Fatalf("suggerimento spostato: %v", err)
	}
	s3, _ := snapshotVersione(t, p, bozza, 3)
	if x := pbDiG39(s3, f2); x.Provenienza != distintabozza.ProvenienzaCorretta || x.Importazione != adesso.Impronta || x.ChiaveBozza != radice {
		t.Fatalf("la PB spostata: %+v", x)
	}

	// 3. l'ambiguo con il registro dichiarato: rifiutato (non e' mai stato un suggerimento); a mano e' «manuale»
	_, err = aggiungiPBG39(t, p, s, bozza, 3, f3, k3, adesso.Impronta, calcolo)
	codiceG39(t, err, distintabozza.CodiceCorpoNonValido, distintabozza.MotivoPropostaNonCorrispondente)
	if _, err := aggiungiPBG39(t, p, s, bozza, 3, f3, k3, "", calcolo); err != nil {
		t.Fatalf("l'ambiguo a mano: %v", err)
	}
	s4, _ := snapshotVersione(t, p, bozza, 4)
	if x := pbDiG39(s4, f3); x.Provenienza != distintabozza.ProvenienzaManuale || x.Importazione != "" {
		t.Fatalf("l'ambiguo a mano: %+v", x)
	}

	// R4 vede le tre provenienze; nessuna AR scritta prima di R5
	rp := riepilogoRev(t, p, s, bozza, 4)
	prov := map[uuid.UUID]string{}
	for _, x := range rp.DocumentiPreassociati {
		prov[x.Allegato] = x.Provenienza
	}
	if prov[f1] != distintabozza.ProvenienzaMotore || prov[f2] != distintabozza.ProvenienzaCorretta || prov[f3] != distintabozza.ProvenienzaManuale {
		t.Fatalf("R4, le provenienze: %v", prov)
	}
	if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = ANY($1)`, []uuid.UUID{f1, f2, f3}); n != 0 {
		t.Fatalf("AR prima di R5: %d", n)
	}
}

func TestV1G39_01FileDopoR5NellaRevisioneDB(t *testing.T) {
	p, s, regA, bozza, pdf := avvioG39(t)
	rp := riepilogoRev(t, p, s, bozza, 1)
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4: %v", codiciR4(rp))
	}
	c1, err := confermaRev(t, p, s, bozza, 1, rp.FirmaAnteprima)
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	// un file arriva dopo R5; il calcolo di adesso lo propone sul figlio
	nuovo := pdfDi(t, p, s, "ACM-1150-F5", '5')
	adesso := conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf, nuovo}})
	versioni := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, bozza)
	// la bozza confermata e' chiusa: R2 la rifiuta (BOZZA_VERSIONE), niente scritto
	s1, _ := snapshotVersione(t, p, bozza, versioni)
	_, k3 := chiaviG39(t, s1, s.prodotto, "nodo:"+s.shaA+":#3")
	_, err = aggiungiPBG39(t, p, s, bozza, versioni, nuovo, k3, adesso.Impronta, &distintabozza.CalcoloAdesso{Registro: &adesso})
	codiceG39(t, err, distintabozza.CodiceBozzaVersione, "")
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, bozza); n != versioni {
		t.Fatalf("la bozza confermata ha una versione nuova: %d", n)
	}
	// R1: nessuna bozza aperta, nessun suggerimento, anche con il calcolo
	l := leggiR1G39(t, p, s, &distintabozza.CalcoloAdesso{Registro: &adesso})
	if l.Stato != distintabozza.StatoNessunaBozza || l.PreassociazioniProposte != nil {
		t.Fatalf("R1 dopo R5: %+v", l)
	}
	// la revisione (S8 su B, da-conferma-1 e cambio della fonte): il file nuovo, proposto dal calcolo della revisione, e'
	// una PB iniziale «motore» della bozza nuova; il PDF con la AR del figlio resta senza PB
	regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#3", codice: "ACM-1150", qta: 4}})
	regB = conProposteG39(t, regB, map[string][]uuid.UUID{"#3": {pdf, nuovo}})
	fattiRevisione(t, p, s.shaB, "#1", "#3")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "revisione 2"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Bozza == nil || esB.Bozza.BozzaID == bozza {
		t.Fatalf("S8 su B dopo la conferma: %+v %v", esB, err)
	}
	sb, _ := snapshotVersione(t, p, esB.Bozza.BozzaID, esB.Bozza.Versione)
	if x := pbDiG39(sb, nuovo); x.Provenienza != distintabozza.ProvenienzaMotore || x.Importazione != regB.Impronta {
		t.Fatalf("il file arrivato dopo R5 nella revisione: %+v (snapshot %+v)", x, sb.DocumentiPreassociati)
	}
	if x := pbDiG39(sb, pdf); x.AllegatoID != uuid.Nil {
		t.Fatalf("il PDF con la AR attiva: %+v", x)
	}
	if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, nuovo); n != 0 {
		t.Fatalf("AR del file nuovo prima della R5 della revisione: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND stato = 'attiva' AND conferma_id = $2`, pdf, c1.ConfermaID); n != 1 {
		t.Fatal("la AR del PDF della prima conferma")
	}
}

// Il calcolo del motore cambia fra la lettura (R1) e il salvataggio (R2) (rilievo minore 7 della revisione G39 parte A): la
// pagina ha letto i suggerimenti con un registro di adesso (letto, mai scritto) e rimanda la PB accettata senza modifiche
// con quella impronta; il server ha davanti un altro calcolo (adesso).
//   - f2: il calcolo di adesso lo propone ancora, univoco, sullo stesso nodo: «motore» con il riferimento al registro che il
//     server ha davanti (scritto in distinta_importazione);
//   - f1: il calcolo di adesso lo propone, univoco, sulla radice: il suggerimento letto e' superato. Non e' l'ingegnere che
//     l'ha spostato, quindi non e' «corretta»: 422 proposta_non_corrispondente, niente scritto. Sul codice di prima la PB
//     era salvata «corretta» con il registro di adesso.
func TestV1G39_01CalcoloCambiatoDopoLaLetturaDB(t *testing.T) {
	p, s, regA, bozza, pdf := avvioG39(t)
	s1, _ := snapshotVersione(t, p, bozza, 1)
	_, k3 := chiaviG39(t, s1, s.prodotto, "nodo:"+s.shaA+":#3")
	f1 := pdfDi(t, p, s, "ACM-1150-F6", '6')
	f2 := pdfDi(t, p, s, "ACM-1150-F7", '7')
	letto := conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf, f1, f2}})
	adesso := conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf, f2}, "#1": {f1}})
	if letto.Impronta == adesso.Impronta {
		t.Fatal("la prova vuole due calcoli diversi")
	}
	calcolo := &distintabozza.CalcoloAdesso{Registro: &adesso}
	// f1, superato: 422, nessuna versione nuova, nessun registro scritto
	_, err := aggiungiPBG39(t, p, s, bozza, 1, f1, k3, letto.Impronta, calcolo)
	codiceG39(t, err, distintabozza.CodiceCorpoNonValido, distintabozza.MotivoPropostaNonCorrispondente)
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, bozza); n != 1 {
		t.Fatalf("il suggerimento superato ha scritto una versione: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione WHERE impronta = ANY($1)`, []string{letto.Impronta, adesso.Impronta}); n != 0 {
		t.Fatalf("il suggerimento superato ha scritto un registro: %d", n)
	}
	// f2, ancora proposto sullo stesso nodo: «motore» con il registro di adesso
	if _, err := aggiungiPBG39(t, p, s, bozza, 1, f2, k3, letto.Impronta, calcolo); err != nil {
		t.Fatalf("il suggerimento ancora valido: %v", err)
	}
	s2, _ := snapshotVersione(t, p, bozza, 2)
	if x := pbDiG39(s2, f2); x.Provenienza != distintabozza.ProvenienzaMotore || x.Importazione != adesso.Impronta || x.ChiaveBozza != k3 {
		t.Fatalf("il suggerimento ancora valido: %+v", x)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione WHERE thread_id = $1 AND prodotto_id = $2 AND impronta = $3`,
		s.thread, s.prodotto, adesso.Impronta); n != 1 {
		t.Fatalf("il registro di adesso in distinta_importazione: %d", n)
	}
}

// R4 su un corpo non salvato con il calcolo del motore (?avvisi_motore=1) (rilievo minore 6 della revisione G39 parte A): i
// suggerimenti dopo_apertura accettati nel corpo, con il registro di adesso che il database non ha ancora, si vedono come li
// salverebbe R2: f1 accettato sul nodo proposto «motore», f2 spostato sulla radice «corretta», con l'impronta del registro
// di adesso. Senza il calcolo (R4 leggero) restano «manuale». Sul codice di prima R4 li mostrava «manuale» anche con il
// calcolo. Il calcolo e' un ingresso di prova: il gruppo della sorgente della bozza e il registro di adesso come importazione.
func TestV1G39_01R4ConIlCalcoloDiAdessoDB(t *testing.T) {
	p, s, regA, bozza, pdf := avvioG39(t)
	s1, _ := snapshotVersione(t, p, bozza, 1)
	radice, k3 := chiaviG39(t, s1, s.prodotto, "nodo:"+s.shaA+":#3")
	f1 := pdfDi(t, p, s, "ACM-1150-F8", '6')
	f2 := pdfDi(t, p, s, "ACM-1150-F9", '7')
	adesso := conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf, f1, f2}})
	src, ok, err := distintabozza.SorgenteDellaBozza(context.Background(), db.New(p), s.thread, s.prodotto, bozza)
	if err != nil || !ok {
		t.Fatalf("la sorgente della bozza: %v %v", ok, err)
	}
	importati := 0
	in := &distintabozza.IngressoMotoreR4{
		Gruppi: []propostabom.Gruppo{{Chiave: src.Gruppo, Tipo: propostabom.GruppoBOM, AllegatoID: &src.AllegatoID, Sha256: src.Sha256, Radice: src.Radice}},
		Importa: func(string) (importazione.Registro, error) {
			importati++
			return adesso, nil
		}}
	if src.Gruppo != propostabom.GruppoBOM {
		in.Gruppi[0].Tipo = propostabom.GruppoCandidata
	}
	m := corpoVersioneRev(t, p, bozza, 1)
	pbs, _ := m["documenti_preassociati"].([]any)
	m["documenti_preassociati"] = append(pbs,
		map[string]any{"allegato_id": f1, "chiave_bozza": k3, "importazione": adesso.Impronta},
		map[string]any{"allegato_id": f2, "chiave_bozza": radice, "importazione": adesso.Impronta})
	raw, _ := json.Marshal(m)
	r, err := distintabozza.LeggiRichiestaR4(raw)
	if err != nil {
		t.Fatalf("corpo di R4: %v", err)
	}
	r4 := func(motore *distintabozza.EsitoMotoreR4) map[uuid.UUID]distintabozza.PreassociazioneR4 {
		ctx := context.Background()
		tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		rp, err := distintabozza.Riepilogo3(ctx, db.New(tx), s.thread, r, distintabozza.OpzioniR4{UtenteID: s.utente, AvvisiMotore: motore != nil, Motore: motore})
		if err != nil {
			t.Fatalf("R4: %v", err)
		}
		out := map[uuid.UUID]distintabozza.PreassociazioneR4{}
		for _, x := range rp.DocumentiPreassociati {
			out[x.Allegato] = x
		}
		return out
	}
	leggero := r4(nil)
	if leggero[f1].Provenienza != distintabozza.ProvenienzaManuale || leggero[f2].Provenienza != distintabozza.ProvenienzaManuale || importati != 0 {
		t.Fatalf("R4 leggero: %+v %+v (importati %d)", leggero[f1], leggero[f2], importati)
	}
	con := r4(&distintabozza.EsitoMotoreR4{Ingresso: in})
	if con[f1].Provenienza != distintabozza.ProvenienzaMotore || con[f2].Provenienza != distintabozza.ProvenienzaCorretta || importati == 0 ||
		con[f1].Importazione != adesso.Impronta || con[f2].Importazione != adesso.Impronta {
		t.Fatalf("R4 con il calcolo di adesso: %+v %+v (importati %d)", con[f1], con[f2], importati)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione WHERE impronta = $1`, adesso.Impronta); n != 0 {
		t.Fatalf("R4 ha scritto il registro di adesso: %d", n)
	}
}
