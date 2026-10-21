//go:build integrazione

// L4 — le query sqlc della 0023 (piano della 0023, §6 e §7; internal/platform/db/queries/v1_formato3.sql e le aggiunte
// a p4_conferme.sql) girano sullo schema 23 e dicono quello che il commento promette: la bozza formato 3 e il suo
// registro, la conferma formato 3 scritta con le query (componenti con l'emittente, archi, nodi non inclusi, esito,
// F8, associazione), le letture di R4/R6/R7 e quelle del ciclo dei file. Righe di base: schema23ConStoria.

package migrazioni_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

func uuid23(nome string) uuid.UUID { return uuid.MustParse(u23(nome)) }

func testo23(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func nullUUID23(nome string) uuid.NullUUID { return uuid.NullUUID{UUID: uuid23(nome), Valid: true} }

func TestV1LeQueryDella0023(t *testing.T) {
	p := schema23ConStoria(t)
	// le sole proposte di righeViste: con la baseline congelata la working non si scriverebbe (la ricodifica legacy sotto)
	esegui23(t, p, righeViste[1])
	ctx := context.Background()
	q := db.New(p)
	T, P, S, U, A := uuid23("{T}"), uuid23("{P}"), uuid23("{S}"), uuid23("{U}"), uuid23("{A}")

	// ------------------------------------------------------------ la bozza formato 3 e il suo registro
	chiave := uuid.MustParse(uno23[string](t, p, `SELECT chiave_salvataggio::text FROM distinta_bozza_versione WHERE bozza_id = '{B3}' AND versione = 1`))
	v, err := q.GetVersionePerChiaveSalvataggio(ctx, db.GetVersionePerChiaveSalvataggioParams{ChiaveSalvataggio: uuid.NullUUID{UUID: chiave, Valid: true}, ThreadID: T})
	if err != nil || v.Versione != 1 || v.ImportazioneID.UUID != uuid23("{R1}") || v.ImprontaRichiesta.String != sha("4") {
		t.Errorf("GetVersionePerChiaveSalvataggio: %+v %v", v, err)
	}
	if _, err := q.GetVersionePerChiaveSalvataggio(ctx, db.GetVersionePerChiaveSalvataggioParams{ChiaveSalvataggio: uuid.NullUUID{UUID: chiave, Valid: true},
		ThreadID: uuid23("{T2}")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("la versione si legge da un'altra RFQ: %v", err)
	}
	sb, err := q.GetSorgenteBozzaAperta(ctx, db.GetSorgenteBozzaApertaParams{ThreadID: T, ProdottoID: P})
	if err != nil || sb.BozzaID != uuid23("{B3}") || sb.Formato != 3 || sb.RegistroGruppo.String != "bom" || sb.RegistroFonteRadice.String != rifB("#1") ||
		sb.RegistroImpronta.String != sha("6") || sb.RegistroImprontaContenuto.String != sha("7") || sb.RegistroFonteAllegatoID.UUID != A {
		t.Errorf("GetSorgenteBozzaAperta: %+v %v", sb, err)
	}
	if _, err := q.GetSorgenteBozzaAperta(ctx, db.GetSorgenteBozzaApertaParams{ThreadID: T, ProdottoID: uuid23("{P2}")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("la bozza formato 2 di P2 non e' una sorgente formato 3: %v", err)
	}
	contenuto := uno23[string](t, p, `SELECT `+contenutoDimp("{P2}", "9", "bom", "NULL", nil)+`::text`)
	nuovo, err := q.InsertDistintaImportazione(ctx, db.InsertDistintaImportazioneParams{ThreadID: T, ProdottoID: uuid23("{P2}"), Impronta: sha("8"),
		ImprontaContenuto: sha("9"), ImprontaEvidenze: sha("9"), ImprontaRegole: sha("9"), FormatoRegistro: 1, VersioneImportazione: "importazione-1",
		Gruppo: "bom", AllegatiCitati: []uuid.UUID{}, Contenuto: json.RawMessage(contenuto), CreataDa: U})
	if err != nil || nuovo.CreataIl.IsZero() {
		t.Fatalf("InsertDistintaImportazione: %+v %v", nuovo, err)
	}
	r, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: nuovo.ImportazioneID, ThreadID: T})
	if err != nil || r.Impronta != sha("8") || r.FonteAllegatoID.Valid || len(r.AllegatiCitati) != 0 || !strings.Contains(string(r.Contenuto), `"prodotto_id"`) {
		t.Errorf("GetDistintaImportazione: %+v %v", r, err)
	}
	r1, err := q.GetDistintaImportazionePerImpronta(ctx, db.GetDistintaImportazionePerImprontaParams{ThreadID: T, ProdottoID: P, Impronta: sha("6")})
	if err != nil || r1.ImportazioneID != uuid23("{R1}") || len(r1.AllegatiCitati) != 1 || r1.AllegatiCitati[0] != A {
		t.Errorf("GetDistintaImportazionePerImpronta: %+v %v", r1, err)
	}
	lista, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: T, ProdottoID: P, Impronte: []string{sha("6"), sha("e"), sha("f")}})
	if err != nil || len(lista) != 2 {
		t.Errorf("ListImportazioniPerImpronte: %d %v (R3 e' di P2)", len(lista), err)
	}
	prodotto, err := q.ListImportazioniProdotto(ctx, db.ListImportazioniProdottoParams{ThreadID: T, ProdottoID: P})
	if err != nil || len(prodotto) != 2 {
		t.Errorf("ListImportazioniProdotto: %d %v", len(prodotto), err)
	}

	// ------------------------------------------------------------ D-6 con la bozza B3 aperta
	dip, err := q.ListDipendenzeBozzaPerFile(ctx, T)
	if err != nil {
		t.Fatal(err)
	}
	tipi := map[string]bool{}
	for _, d := range dip {
		if d.BozzaID == uuid23("{B3}") && d.AllegatoID == A {
			tipi[d.Tipo] = true
		}
	}
	for _, tp := range []string{"fonte", "casella", "non_includere", "registro"} {
		if !tipi[tp] {
			t.Errorf("ListDipendenzeBozzaPerFile: manca la dipendenza %s di B3 su A (%+v)", tp, dip)
		}
	}

	// ------------------------------------------------------------ la conferma formato 3 scritta con le query
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qt := db.New(tx)
	if err := qt.BloccaContenuto(ctx, sha("b")); err != nil {
		t.Fatalf("BloccaContenuto: %v", err)
	}
	cont, err := qt.GetWorkingContatore(ctx, T)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := json.Marshal(snapshot3())
	if err != nil {
		t.Fatal(err)
	}
	mappa, _ := json.Marshal(map[string]string{"k-p": P.String(), "k-s": S.String()})
	conf, err := qt.InsertConfermaStrutturale(ctx, db.InsertConfermaStrutturaleParams{ThreadID: T, ProdottoID: P, Numero: 2, PrecedenteID: nullUUID23("{C1}"),
		Formato: 3, FirmaStruttura: sha("3"), FirmaAnteprima: testo23(sha("2")), BaseConfermaID: nullUUID23("{C1}"), FirmaBase: testo23(sha("1")),
		Contatore: cont.Struttura, ContatoreDati: cont.Dati, BozzaID: nullUUID23("{B3}"), BozzaVersione: pgtype.Int4{Int32: 1, Valid: true},
		FonteAllegatoID: uuid.NullUUID{UUID: A, Valid: true}, FonteSha256: testo23(sha("b")), FonteGruppo: testo23("bom"),
		Origine: json.RawMessage(`{"rotta":"motore-a/bom/conferma"}`), Snapshot: snap, ImprontaSnapshot: sha("5"), Delta: json.RawMessage(`{"formato":3}`),
		Mappa: mappa, Decisioni: json.RawMessage(`[]`), CriterioVerifica: "nessuno", Verifica: json.RawMessage(`{"regola": "prima-istanza-1"}`),
		ChiaveIdempotenza: nullUUID23("{K3}"), ConfermatoDa: U, ImportazioneID: nullUUID23("{R1}"), ImportazioniCitate: []uuid.UUID{uuid23("{R1}")}})
	if err != nil {
		t.Fatalf("InsertConfermaStrutturale: %v", err)
	}
	cliente := testo23("cliente:" + u23("{C}"))
	presunto := pgtype.Bool{Bool: true, Valid: true}
	for _, c := range []db.InsertConfermaStrutturaleComponenteParams{
		{ConfermaID: conf.ConfermaID, ThreadID: T, ComponenteID: P, Radice: true, ChiaveBozza: testo23("k-p"), Codice: "ACM-1000", Rev: testo23("A"),
			Descrizione: testo23("Gruppo base"), Tipo: db.TipoComponenteFinito, Emittente: cliente, EmittentePresunto: presunto},
		{ConfermaID: conf.ConfermaID, ThreadID: T, ComponenteID: S, ChiaveBozza: testo23("k-s"), Codice: "ACM-1100", Rev: testo23("A"),
			Descrizione: testo23("Telaio"), Tipo: db.TipoComponenteSottoassieme, Emittente: cliente, EmittentePresunto: presunto},
	} {
		if err := qt.InsertConfermaStrutturaleComponente(ctx, c); err != nil {
			t.Fatalf("InsertConfermaStrutturaleComponente: %v", err)
		}
	}
	if err := qt.InsertConfermaStrutturaleArco(ctx, db.InsertConfermaStrutturaleArcoParams{ConfermaID: conf.ConfermaID, PadreID: P, FiglioID: S, Qta: 2,
		Origine: testo23("conferma")}); err != nil {
		t.Fatalf("InsertConfermaStrutturaleArco: %v", err)
	}
	segnale := json.RawMessage(`{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"}`)
	if err := qt.InsertConfermaNonIncluso(ctx, db.InsertConfermaNonInclusoParams{ConfermaID: conf.ConfermaID, ThreadID: T, Sha256: sha("b"), Chiave: "#31",
		AllegatoID: A, Gruppo: "bom", Origine: "non_includere", DecisioneID: testo23("ni-1"), AzioneID: nullUUID23("{AZ}"),
		MotivoCodice: testo23("elemento_cad"), Motivo: testo23("cordone di saldatura"),
		Segnale: &segnale, RigheChiuse: []uuid.UUID{}, ArchiChiusi: json.RawMessage(`[]`)}); err != nil {
		t.Fatalf("InsertConfermaNonIncluso: %v", err)
	}
	if n, err := qt.ConfermaDistintaBozza(ctx, db.ConfermaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: U, Valid: true}, BozzaID: uuid23("{B3}"),
		ThreadID: T, Versione: 1}); err != nil || n != 1 {
		t.Fatalf("ConfermaDistintaBozza: %d %v", n, err)
	}
	if _, err := qt.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: uuid23("{K3}"), ThreadID: T, Gesto: "conferma_bom_v3", UtenteID: U,
		ImprontaCorpo: sha("a"), Esito: json.RawMessage(`{"esito":"confermata"}`), ConfermaID: uuid.NullUUID{UUID: conf.ConfermaID, Valid: true}}); err != nil {
		t.Fatalf("InsertEsitoIntento: %v", err)
	}
	if _, err := qt.InsertF8StatoConferma(ctx, db.InsertF8StatoConfermaParams{ThreadID: T, ChiaveIntento: nullUUID23("{K3}"),
		ConfermaID: uuid.NullUUID{UUID: conf.ConfermaID, Valid: true}}); err != nil {
		t.Fatalf("InsertF8StatoConferma: %v", err)
	}
	if _, err := qt.InsertAssociazioneEvento(ctx, db.InsertAssociazioneEventoParams{ThreadID: T, PropostaID: nullUUID23("{DP}"), Azione: "scarta",
		Origine: "ponte", ChiaveIdempotenza: nullUUID23("{K4}"), AutoreID: U}); err != nil {
		t.Fatalf("InsertAssociazioneEvento: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT della conferma formato 3 scritta con le query: %v", err)
	}

	// ------------------------------------------------------------ R4, R6, R7
	if c, err := q.GetConfermaPerChiave(ctx, db.GetConfermaPerChiaveParams{ChiaveIdempotenza: nullUUID23("{K3}"), ThreadID: T}); err != nil ||
		c.Formato != 3 || c.ImportazioneID.UUID != uuid23("{R1}") || c.Numero != 2 {
		t.Errorf("GetConfermaPerChiave: %+v %v", c, err)
	}
	conferme, err := q.ListConfermeStrutturali(ctx, db.ListConfermeStrutturaliParams{ThreadID: T, ProdottoID: P})
	if err != nil || len(conferme) != 2 || conferme[1].Formato != 3 || len(conferme[1].ImportazioniCitate) != 1 || conferme[0].ImportazioniCitate != nil {
		t.Errorf("ListConfermeStrutturali: %+v %v", conferme, err)
	}
	if c, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: T, ConfermaID: conf.ConfermaID}); err != nil ||
		c.ImportazioneID.UUID != uuid23("{R1}") || !strings.Contains(string(c.Snapshot), `"formato": 3`) {
		t.Errorf("GetConfermaStrutturale: %v", err)
	}
	comp, err := q.ListConfermaStrutturaleComponenti(ctx, db.ListConfermaStrutturaleComponentiParams{ConfermaID: conf.ConfermaID, ThreadID: T})
	if err != nil || len(comp) != 2 || comp[0].Emittente.String != "cliente:"+u23("{C}") || !comp[0].EmittentePresunto.Bool || comp[0].Namespace.Valid {
		t.Errorf("ListConfermaStrutturaleComponenti: %+v %v", comp, err)
	}
	ni, err := q.ListNonInclusiConferma(ctx, db.ListNonInclusiConfermaParams{ConfermaID: conf.ConfermaID, ThreadID: T})
	if err != nil || len(ni) != 1 || ni[0].DecisioneID.String != "ni-1" || ni[0].Origine != "non_includere" || ni[0].Numero != 2 {
		t.Errorf("ListNonInclusiConferma: %+v %v", ni, err)
	}
	// senza conferma di base (NULL); la base e il ripristino con piu' conferme sono in TestV1OT06IlRipristinoDiUnNodoNonIncluso
	rip, err := q.ListNonInclusiPerRif(ctx, db.ListNonInclusiPerRifParams{ThreadID: T, ProdottoID: P, Shas: []string{sha("b"), sha("b")},
		Chiavi: []string{"#31", "#99"}})
	if err != nil || len(rip) != 1 || rip[0].ConfermaID != conf.ConfermaID || rip[0].Gruppo != "bom" {
		t.Errorf("ListNonInclusiPerRif: %+v %v", rip, err)
	}
	ident, err := q.ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: T, Componenti: []uuid.UUID{P, S, uuid23("{P2}")}})
	if err != nil || len(ident) != 2 || !ident[0].Valida || !ident[0].EmittentePresunto.Bool {
		t.Errorf("ListIdentitaRegistrate: %+v %v", ident, err)
	}
	// una ricodifica legacy dopo la conferma supera la certificazione (valida falso), anche per le sole maiuscole
	riuscita23(t, p, "ricodifica legacy di S", `SET CONSTRAINTS ALL DEFERRED`,
		`UPDATE componente SET codice = 'Acm-1100' WHERE componente_id = '{S}'`,
		`UPDATE documento SET codice = 'Acm-1100' WHERE componente_id = '{S}'`,
		`UPDATE documento_proposta SET codice = 'Acm-1100' WHERE componente_id = '{S}'`)
	ident, err = q.ListIdentitaRegistrate(ctx, db.ListIdentitaRegistrateParams{ThreadID: T, Componenti: []uuid.UUID{S}})
	if err != nil || len(ident) != 1 || ident[0].Valida || ident[0].CodiceWorking != "Acm-1100" {
		t.Errorf("ListIdentitaRegistrate dopo una ricodifica legacy: %+v %v", ident, err)
	}
	chiavi, err := q.ChiaviWorking(ctx, db.ChiaviWorkingParams{Codici: []string{"acm-1000", "NUOVO-1"}, ThreadID: T})
	perCodice := map[string]db.ChiaviWorkingRow{}
	for _, c := range chiavi {
		perCodice[c.Codice] = c
	}
	if err != nil || len(chiavi) != 2 || perCodice["acm-1000"].Chiave != "ACM-1000" || perCodice["acm-1000"].OccupanteID.UUID != P ||
		perCodice["NUOVO-1"].OccupanteID.Valid {
		t.Errorf("ChiaviWorking: %+v %v", chiavi, err)
	}
	righe, err := q.ListRighePerShaChiave(ctx, db.ListRighePerShaChiaveParams{ThreadID: T, Shas: []string{sha("4"), sha("4")}, Chiavi: []string{"#1", "#2"}})
	if err != nil || len(righe) != 2 {
		t.Errorf("ListRighePerShaChiave: %+v %v", righe, err)
	}
	archi, err := q.ListArchiPerShaChiave(ctx, db.ListArchiPerShaChiaveParams{ThreadID: T, Shas: []string{sha("4")}, Chiavi: []string{"#2"}})
	if err != nil || len(archi) != 1 || archi[0].PadreChiave != "#1" {
		t.Errorf("ListArchiPerShaChiave: %+v %v", archi, err)
	}
	stessi, err := q.ListProdottiConLoStessoStep(ctx, db.ListProdottiConLoStessoStepParams{ThreadID: T, Sha256: sha("b")})
	if err != nil || len(stessi) != 1 || stessi[0].ProdottoID != P || stessi[0].Come != "fonte_conferma" {
		t.Errorf("ListProdottiConLoStessoStep: %+v %v", stessi, err)
	}
	if ev, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: T, ProdottoID: P, Ruolo: "prodotto", Azione: "scelta", AllegatoID: A,
		Sha256: sha("b"), Gruppo: testo23("bom"), AutoreID: U}); err != nil || ev.EventoID == 0 {
		t.Errorf("InsertFonteStepEvento: %v", err)
	}
	fonte, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: T, ProdottoID: P})
	if err != nil || len(fonte) != 2 {
		t.Errorf("ListFonteStepProdotto: %d %v (la scelta della 22 e quella di adesso)", len(fonte), err)
	}
	if att, err := q.ListAttestazioniProdotto(ctx, db.ListAttestazioniProdottoParams{ThreadID: T, ProdottoID: P}); err != nil || len(att) != 1 {
		t.Errorf("ListAttestazioniProdotto: %d %v", len(att), err)
	}
	// direttiva 35, C-1c: la ricerca e' ristretta al gesto associa_step («assegna», origine ponte, senza riga della AR):
	// lo «scarta» scritto con {K4} non e' di quel gesto
	if as, err := q.GetAssociazioneEventoPerChiave(ctx, db.GetAssociazioneEventoPerChiaveParams{ChiaveIdempotenza: nullUUID23("{K4}"), ThreadID: T}); err != nil ||
		len(as) != 0 {
		t.Errorf("GetAssociazioneEventoPerChiave: %+v %v", as, err)
	}
	if _, err := q.InsertComponenteSenzaFusione(ctx, db.InsertComponenteSenzaFusioneParams{ThreadID: T, Codice: "ACM-5000", Qta: 1,
		Tipo: db.TipoComponenteSciolto, Origine: db.OrigineComponenteManuale, ConfermatoDa: U}); err != nil {
		t.Errorf("InsertComponenteSenzaFusione: %v", err)
	}
	if _, err := q.InsertComponenteSenzaFusione(ctx, db.InsertComponenteSenzaFusioneParams{ThreadID: T, Codice: "acm-5000", Qta: 1,
		Tipo: db.TipoComponenteSciolto, Origine: db.OrigineComponenteManuale, ConfermatoDa: U}); vincoloViolato(err) == "" || codice(err) != "23505" {
		t.Errorf("InsertComponenteSenzaFusione con la stessa chiave della working: atteso 23505, ottenuto %v (nessuna fusione)", err)
	}

	// ------------------------------------------------------------ il ciclo dei file
	A3, A4 := uuid23("{A3}"), uuid23("{A4}")
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt = db.New(tx)
	cons, err := qt.InsertConservazione(ctx, db.InsertConservazioneParams{ThreadID: T, AllegatoID: A3, Sha256: sha("3"), RadiceID: uuid.New()})
	if err != nil || !strings.HasPrefix(cons.PercorsoRelativo, `ALTRO\`+u23("{A3}")+".") {
		t.Fatalf("InsertConservazione: %+v %v", cons, err)
	}
	if n, err := qt.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "copiata", Tentativi: 1, SorgenteByte: testo23("staging"),
		ConservazioneID: cons.ConservazioneID, StatoAtteso: "in_copia"}); err != nil || n != 1 {
		t.Fatalf("UpdateConservazione: %d %v", n, err)
	}
	if f, err := qt.InsertFileRfq(ctx, db.InsertFileRfqParams{Stato: "altri_file", UtenteID: U, AllegatoID: A3, ThreadID: T}); err != nil || f.Versione != 1 {
		t.Fatalf("InsertFileRfq: %+v %v", f, err)
	}
	if _, err := qt.InsertFileRfqEvento(ctx, db.InsertFileRfqEventoParams{ThreadID: T, AllegatoID: A3, Azione: "altri_file", StatoPrima: testo23("attivo"),
		StatoDopo: testo23("altri_file"), Fotografia: rawPtr(`{}`), Dipendenze: rawPtr(`{}`), Origine: "ponte", AutoreID: uuid.NullUUID{UUID: U, Valid: true},
		ChiaveIdempotenza: nullUUID23("{K4}")}); err != nil {
		t.Fatalf("InsertFileRfqEvento: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT di «Metti in Altri file» con le query: %v", err)
	}
	if _, err := q.InsertFileRfq(ctx, db.InsertFileRfqParams{Stato: "altri_file", UtenteID: U, AllegatoID: uuid23("{AX}"), ThreadID: T}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("InsertFileRfq con un allegato di un'altra RFQ: %v", err)
	}
	if f, err := q.GetFileRfq(ctx, db.GetFileRfqParams{ThreadID: T, AllegatoID: A3}); err != nil || f.Stato != "altri_file" || f.NomeFile != "ACM-1100_b.pdf" {
		t.Errorf("GetFileRfq: %+v %v", f, err)
	}
	if c, err := q.GetConservazioneCopiata(ctx, db.GetConservazioneCopiataParams{ThreadID: T, AllegatoID: A3}); err != nil || c.VerificatoIl == nil {
		t.Errorf("GetConservazioneCopiata: %+v %v", c, err)
	}
	stati, err := q.ListFileRfqPerThread(ctx, T)
	if err != nil {
		t.Fatal(err)
	}
	visti := map[uuid.UUID]string{}
	for _, s := range stati {
		visti[s.AllegatoID] = s.Stato + " " + itoa(int64(s.Versione))
	}
	if visti[A3] != "altri_file 1" || visti[A4] != "attivo 0" || visti[uuid23("{AI}")] != "" {
		t.Errorf("ListFileRfqPerThread: %v", visti)
	}
	// Riattiva, con la versione attesa; la stessa con una versione superata non tocca niente
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt = db.New(tx)
	if n, err := qt.UpdateFileRfqStato(ctx, db.UpdateFileRfqStatoParams{Stato: "attivo", UtenteID: U, ThreadID: T, AllegatoID: A3, VersioneAttesa: 1}); err != nil || n != 1 {
		t.Fatalf("UpdateFileRfqStato: %d %v", n, err)
	}
	if _, err := qt.InsertFileRfqEvento(ctx, db.InsertFileRfqEventoParams{ThreadID: T, AllegatoID: A3, Azione: "riattiva", StatoPrima: testo23("altri_file"),
		StatoDopo: testo23("attivo"), Fotografia: rawPtr(`{}`), Origine: "ponte", AutoreID: uuid.NullUUID{UUID: U, Valid: true}}); err != nil {
		t.Fatalf("InsertFileRfqEvento riattiva: %v", err)
	}
	if n, err := qt.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "da_rilasciare", Causa: testo23("riattivato"), Tentativi: 1,
		ConservazioneID: cons.ConservazioneID, StatoAtteso: "copiata"}); err != nil || n != 1 {
		t.Fatalf("UpdateConservazione da_rilasciare: %d %v", n, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT di «Riattiva» con le query: %v", err)
	}
	if n, err := q.UpdateFileRfqStato(ctx, db.UpdateFileRfqStatoParams{Stato: "scartato", UtenteID: U, ThreadID: T, AllegatoID: A3, VersioneAttesa: 1}); err != nil || n != 0 {
		t.Errorf("UpdateFileRfqStato con la versione superata: %d %v", n, err)
	}
	// Scarta, con la rimozione; il custode la riprende e la chiude
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt = db.New(tx)
	if _, err := qt.InsertFileRfq(ctx, db.InsertFileRfqParams{Stato: "scartato", UtenteID: U, AllegatoID: A4, ThreadID: T}); err != nil {
		t.Fatalf("InsertFileRfq scartato: %v", err)
	}
	ev, err := qt.InsertFileRfqEvento(ctx, db.InsertFileRfqEventoParams{ThreadID: T, AllegatoID: A4, Azione: "scarta", StatoPrima: testo23("attivo"),
		StatoDopo: testo23("scartato"), Motivo: testo23("non serve"), Fotografia: rawPtr(`{}`), Dipendenze: rawPtr(`{}`), Origine: "ponte",
		AutoreID: uuid.NullUUID{UUID: U, Valid: true}})
	if err != nil {
		t.Fatalf("InsertFileRfqEvento scarta: %v", err)
	}
	if _, err := qt.InsertRimozione(ctx, db.InsertRimozioneParams{ThreadID: T, AllegatoID: A4, EventoID: ev.EventoID, Sha256: testo23(sha("4"))}); err != nil {
		t.Fatalf("InsertRimozione: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT di «Scarta» con le query: %v", err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	qt = db.New(tx)
	rim, err := qt.ListRimozioniDaRiprendere(ctx, 50)
	if err != nil || len(rim) != 1 || rim[0].AllegatoID != A4 {
		t.Fatalf("ListRimozioniDaRiprendere: %+v %v", rim, err)
	}
	if n, err := qt.UpdateRimozione(ctx, db.UpdateRimozioneParams{Stato: "eseguita", Motivo: "rimosso", Ritentabile: true, Tentativi: 1,
		PercorsoTrovato: testo23(`staging\44\44`), RimozioneID: rim[0].RimozioneID, StatoAtteso: "in_attesa"}); err != nil || n != 1 {
		t.Fatalf("UpdateRimozione: %d %v", n, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if rim, err := q.ListRimozioniNonRitentabili(ctx, 50); err != nil || len(rim) != 0 {
		t.Errorf("ListRimozioniNonRitentabili: %+v %v", rim, err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cs, err := db.New(tx).ListConservazioniDaRiprendere(ctx, 50); err != nil || len(cs) != 1 || cs[0].Stato != "da_rilasciare" {
		t.Errorf("ListConservazioniDaRiprendere: %+v %v", cs, err)
	}
	_ = tx.Rollback(ctx)
	if evs, err := q.GetFileEventoPerChiave(ctx, db.GetFileEventoPerChiaveParams{ChiaveIdempotenza: nullUUID23("{K4}"), ThreadID: T}); err != nil || len(evs) != 1 {
		t.Errorf("GetFileEventoPerChiave: %+v %v", evs, err)
	}
	if evs, err := q.ListFileRfqEventi(ctx, db.ListFileRfqEventiParams{ThreadID: T, AllegatoID: A3}); err != nil || len(evs) != 2 || evs[1].Azione != "riattiva" {
		t.Errorf("ListFileRfqEventi: %+v %v", evs, err)
	}
	// dopo la conferma B3 non e' piu' aperta: nessuna dipendenza D-6 su A dalle bozze
	if dip, err := q.ListDipendenzeBozzaPerFile(ctx, T); err != nil || len(dip) != 0 {
		t.Errorf("ListDipendenzeBozzaPerFile dopo la conferma: %+v %v", dip, err)
	}
}

// TestV1OT06IlRipristinoDiUnNodoNonIncluso (PM23 §6 e §7 riga R4, terzo giro su O-01, OT-06; EM §7.6 punto 3; MAT
// NI-07, BZ-20): #31 non incluso nella conferma n. 2 di P (C3), reincluso e confermato nella n. 3 (C4). Per una bozza
// nata dalla n. 3 il nodo non e' un ripristino (sta fra i nodi della conferma di base): ListNonInclusiPerRif non da'
// righe, quindi R4 non dice «ripristino» e R5 non riapre niente. Con la base n. 2, o senza base, la riga e' quella della
// n. 2.
func TestV1OT06IlRipristinoDiUnNodoNonIncluso(t *testing.T) {
	p := schema23ConStoria(t)
	ctx := context.Background()
	q := db.New(p)
	riuscita23(t, p, "conferma n. 2 di P: #31 non incluso", nuovaC3().istruzioni(t)...)

	// la bozza formato 3 B4 di P nata dalla n. 2, con #31 reincluso (un nodo con la casella del suo rif)
	s4 := snapshot3()
	s4["versione_bom_base"] = map[string]any{"conferma_id": u23("{C3}"), "numero": 2, "contatore": 0}
	s4["decisioni"] = []any{}
	s4["nodi"] = append(s4["nodi"].([]any), map[string]any{"chiave_bozza": "k-31", "op": "crea", "tipo": "sciolto", "origine": "step",
		"caselle":  []any{map[string]any{"allegato_id": u23("{A}"), "gruppo": "bom", "rif": rifB("#31")}},
		"identita": map[string]any{"emittente": "cliente:" + u23("{C}"), "codice": "ACM-1310", "modo": "proposta"}})
	esegui23(t, p, `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, creata_da, aggiornata_da)
		 VALUES ('{B4}', '{T}', '{P}', 3, '{C3}', 2, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}', '{U}');
		 INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
		 VALUES ('{B4}', 1, `+jsonSQL(t, s4)+`, repeat('d', 64), '{C3}', 2, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}',
			'{R1}', gen_random_uuid(), repeat('4', 64))`)
	// la conferma n. 3 (C4) che la chiude: #31 e' fra i suoi nodi, nessun nodo non incluso
	c4 := nuovaC3()
	c4.snap = s4
	c4.nonInclusi = nil
	for k, v := range map[string]string{"conferma_id": "'{C4}'", "numero": "3", "precedente_id": "'{C3}'", "base_conferma_id": "'{C3}'",
		"bozza_id": "'{B4}'", "impronta_snapshot": "repeat('d', 64)", "chiave_idempotenza": "'{K4}'"} {
		c4.testata[k] = v
	}
	for i := range c4.componenti {
		c4.componenti[i] = strings.Replace(c4.componenti[i], "'{C3}'", "'{C4}'", 1)
	}
	c4.archi = []string{strings.Replace(arcoC3, "'{C3}'", "'{C4}'", 1)}
	c4.nonChiude = true
	c4.dopo = []string{`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{B4}'`}
	riuscita23(t, p, "conferma n. 3 di P: #31 reincluso", c4.istruzioni(t)...)

	ripristino := func(base uuid.NullUUID) []db.ListNonInclusiPerRifRow {
		t.Helper()
		righe, err := q.ListNonInclusiPerRif(ctx, db.ListNonInclusiPerRifParams{ThreadID: uuid23("{T}"), ProdottoID: uuid23("{P}"),
			Shas: []string{sha("b")}, Chiavi: []string{"#31"}, BaseConfermaID: base})
		if err != nil {
			t.Fatal(err)
		}
		return righe
	}
	if r := ripristino(nullUUID23("{C4}")); len(r) != 0 {
		t.Errorf("bozza nata dalla n. 3, dove #31 e' un nodo: nessun ripristino atteso, righe %+v", r)
	}
	for nome, base := range map[string]uuid.NullUUID{"base n. 2": nullUUID23("{C3}"), "senza base": {}} {
		r := ripristino(base)
		if len(r) != 1 || r[0].ConfermaID != uuid23("{C3}") || r[0].Numero != 2 || r[0].Chiave != "#31" || r[0].DecisioneID.String != "ni-1" {
			t.Errorf("%s: atteso il ripristino di #31 non incluso nella n. 2, righe %+v", nome, r)
		}
	}
}

func rawPtr(s string) *json.RawMessage {
	r := json.RawMessage(s)
	return &r
}
