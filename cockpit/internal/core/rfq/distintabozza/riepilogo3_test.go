package distintabozza

// L1 — R4 formato 3, la parte pura (riepilogo3_regole.go) sopra ingressi costruiti a mano (direttiva 39 §1, §2, §4, §5,
// §6; EM §9.4, §7.4, §7.7, §8.1-§8.3, App. H.7, App. I): la bozza senza STEP con il riepilogo completo e il gate, mono_pezzo
// senza doppioni, il caso a zero figli, la fonte A della working diversa dalla sorgente B, i gruppi D-PB-04, la riga «legato come
// sorgente», gia_associata_altrove e la sostituzione (Prova 7), CICLO sul grafo bozza ∪ working, FILE_CONTESO_FRA_PRODOTTI,
// condivisione[] e ARCO_CONDIVISO_QTA (Prova 8), D-ASS-01 (Prova 4), D-ASS-02 (Prova 5), ASSENZA_NON_DECISA, la firma.
// Dati sintetici.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

type fixR4 struct {
	t                                 *testing.T
	thread, prodotto, cliente, utente uuid.UUID
	comp                              []confermabom.Componente
	archiW                            []confermabom.Arco
	conferme                          []confermabom.TestataConferma
	fonti                             map[uuid.UUID]confermabom.FonteAutorizzata
	nodi, archi, decisioni, pb        []any
	sorgente                          map[string]any
	sorg                              SorgenteR4
	fatti                             associazioni.FattiPiano
	eventi                            []EventoLetto
	altre                             []BozzaDellaRfq
	scrittiNAS                        map[uuid.UUID]int
	stessoStep                        map[string][]ProdottoStessoStep
	congelata                         bool
	conBozza                          bool
	// senzaMotore: il riepilogo leggero (senza ?avvisi_motore=1); altrimenti un motore finto che non valuta il prodotto
	senzaMotore bool
}

func nuovaFixR4(t *testing.T) *fixR4 {
	f := &fixR4{t: t, thread: idDi("r4-rfq"), prodotto: idDi("r4-prodotto"), cliente: idDi("r4-cliente"), utente: idDi("r4-utente"),
		fonti: map[uuid.UUID]confermabom.FonteAutorizzata{}, scrittiNAS: map[uuid.UUID]int{}, stessoStep: map[string][]ProdottoStessoStep{},
		sorg: SorgenteR4{Motivo: SorgenteMotivoSenzaSorgente},
		fatti: associazioni.FattiPiano{Occorrenze: map[uuid.UUID]associazioni.OccorrenzaPiano{}, Attive: map[uuid.UUID]associazioni.Associazione{},
			Chiuse: map[[2]uuid.UUID]bool{}, CopieSenzaAR: map[uuid.UUID]uuid.UUID{}, Documenti: map[uuid.UUID][]associazioni.DocumentoPiano{},
			DelProdotto: map[uuid.UUID]bool{}, ProdottiDi: map[uuid.UUID][]uuid.UUID{}}}
	f.comp = append(f.comp, confermabom.Componente{ID: f.prodotto, CodiceLetterale: "ACM-1000", Tipo: "finito"})
	f.nodoReg("k-root", f.prodotto, "finito")
	return f
}

func (f *fixR4) emittente() string { return EmittenteCliente(f.cliente) }

// componente aggiunge un componente della working.
func (f *fixR4) componente(nome, codice, tipo string) uuid.UUID {
	id := idDi("r4-" + nome)
	f.comp = append(f.comp, confermabom.Componente{ID: id, CodiceLetterale: codice, Tipo: tipo})
	return id
}

func (f *fixR4) arcoW(p, c uuid.UUID, qta int32) {
	f.archiW = append(f.archiW, confermabom.Arco{Padre: p, Figlio: c, Qta: qta})
}

func (f *fixR4) codiceDi(id uuid.UUID) (string, string) {
	for _, c := range f.comp {
		if c.ID == id {
			return c.CodiceLetterale, c.Tipo
		}
	}
	f.t.Fatalf("componente %s assente", id)
	return "", ""
}

// nodoReg: un nodo registrato (riusa) con l'identita' registrata presunta del cliente.
func (f *fixR4) nodoReg(chiave string, id uuid.UUID, tipo string) {
	cod, _ := f.codiceDi(id)
	f.nodi = append(f.nodi, map[string]any{"chiave_bozza": chiave, "componente_id": id, "op": "riusa", "tipo": tipo, "origine": "registrato",
		"identita": map[string]any{"emittente": f.emittente(), "emittente_presunto": true, "codice": cod, "modo": "registrato"}})
}

// nodoMan: un nodo scritto a mano (crea, manuale).
func (f *fixR4) nodoMan(chiave, codice, tipo string) {
	id := map[string]any{"emittente": f.emittente(), "emittente_presunto": true, "modo": "manuale"}
	if codice != "" {
		id["codice"] = codice
	}
	f.nodi = append(f.nodi, map[string]any{"chiave_bozza": chiave, "op": "crea", "tipo": tipo, "origine": "utente", "identita": id})
}

func (f *fixR4) arco(p, c string, qta int) {
	f.archi = append(f.archi, map[string]any{"padre": p, "figlio": c, "qta": qta, "origine": "utente"})
}

func (f *fixR4) preassocia(a uuid.UUID, chiave, provenienza, registro string) {
	m := map[string]any{"allegato_id": a, "chiave_bozza": chiave}
	if provenienza != "" {
		m["provenienza"] = provenienza
	}
	if registro != "" {
		m["importazione"] = registro
	}
	f.pb = append(f.pb, m)
}

// occorrenza: un file attivo della RFQ con il codice letto.
func (f *fixR4) occorrenza(nome, codice string) uuid.UUID {
	a := idDi("r4-file-" + nome)
	f.fatti.Occorrenze[a] = associazioni.OccorrenzaPiano{DellaRfq: true, Attivo: true, Preassociabile: true, Sha256: shaProva('d'),
		NomeFile: nome, ConProposta: true, TipoProposto: "disegno_2d", CodiceLetto: codice}
	return a
}

// sorgenteValida: la versione ha la sorgente STEP confermata da S8 (gruppo solo_prodotto: senza caselle).
func (f *fixR4) sorgenteValida(sha string) uuid.UUID {
	a := idDi("r4-step")
	d := idDi("r4-documento-" + sha[:4])
	f.sorgente = map[string]any{"allegato_id": a, "sha256": sha, "gruppo": GruppoSoloProdotto}
	f.sorg = SorgenteR4{AllegatoID: &a, Sha256: sha, Gruppo: GruppoSoloProdotto, Valida: true, DocumentoID: &d}
	return d
}

func (f *fixR4) snapshot() Snapshot3 {
	f.t.Helper()
	m := map[string]any{"formato": 3, "prodotto_id": f.prodotto, "versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0},
		"firma_base": firmaProva, "nodi": f.nodi, "archi": nonNilAny(f.archi), "decisioni": nonNilAny(f.decisioni),
		"documenti_preassociati": nonNilAny(f.pb)}
	if f.sorgente != nil {
		m["sorgente"] = f.sorgente
	}
	raw, err := json.Marshal(m)
	if err != nil {
		f.t.Fatal(err)
	}
	s, err := LeggiSnapshot3(raw)
	if err != nil {
		f.t.Fatalf("snapshot di prova: %v\n%s", err, raw)
	}
	return s
}

func nonNilAny(xx []any) []any {
	if xx == nil {
		return []any{}
	}
	return xx
}

func (f *fixR4) ingressi() *ingressiR4 {
	f.t.Helper()
	s := f.snapshot()
	can, err := s.Canonico()
	if err != nil {
		f.t.Fatal(err)
	}
	st := confermabom.NuovoStatoRFQ(f.thread, 7, f.comp, f.archiW, f.conferme, f.fonti)
	vc, err := st.VersioneCorrente(f.prodotto)
	if err != nil {
		f.t.Fatal(err)
	}
	in := &ingressiR4{thread: f.thread, prodotto: f.prodotto, utente: f.utente, snap: s, canonico: can, stato: st, cert: map[uuid.UUID]Certificazione{},
		cliente: f.emittente(), vc: vc, base: StatoBase{Stato: BaseValida, Motivi: []string{}}, scaduti: []RifScaduto{}, sorgente: f.sorg,
		congelata: f.congelata, fatti: f.fatti, altreBozze: f.altre, scrittiNAS: f.scrittiNAS, effettiTipo: map[string]string{},
		stessoStep: f.stessoStep, shaS8: map[string]bool{}, eventi: f.eventi, righeSha: map[string][]db.ListRigheStrutturaR4Row{},
		archiNI: map[string][]db.ListArchiPerShaChiaveRow{}}
	if !f.senzaMotore {
		in.avvisiMotore, in.esitoMotore = true, &EsitoMotoreR4{Ingresso: &IngressoMotoreR4{}}
	}
	if f.conBozza {
		v := 1
		in.versione = &v
		b := idDi("r4-bozza")
		in.richiesta.BozzaID = &b
	}
	in.completaFattiPiano()
	return in
}

func (f *fixR4) calcola() RiepilogoR4 {
	f.t.Helper()
	r, err := calcolaR4(f.ingressi())
	if err != nil {
		f.t.Fatalf("riepilogo: %v", err)
	}
	return r
}

func codici(rr []Rilievo) []string {
	var out []string
	for _, r := range rr {
		out = append(out, r.Codice)
	}
	return out
}

func trova(rr []Rilievo, codice string) []Rilievo {
	var out []Rilievo
	for _, r := range rr {
		if r.Codice == codice {
			out = append(out, r)
		}
	}
	return out
}

var reFirma64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// D39 §1 [U]: la bozza senza STEP, partenza vuota con PB manuali: R4 completo, il blocco firmato, mai un errore.
func TestR4BozzaSenzaStepRiepilogoCompletoConIlGate(t *testing.T) {
	f := nuovaFixR4(t)
	f.nodoMan("k-telaio", "ACM-1100", "sottoassieme")
	f.nodoMan("k-vite", "ACM-9001", "commerciale")
	f.arco("k-root", "k-telaio", 1)
	f.arco("k-telaio", "k-vite", 8)
	a := f.occorrenza("ACM-1100.pdf", "ACM-1100")
	f.preassocia(a, "k-telaio", ProvenienzaManuale, "")
	r := f.calcola()
	g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata)
	if len(g) != 1 || g[0].Motivo != SorgenteMotivoSenzaSorgente || g[0].Sorgente != nil || g[0].MonoPezzo {
		t.Fatalf("gate: %+v (blocchi %v)", g, codici(r.Blocchi))
	}
	if len(r.Blocchi) != 1 || r.Confermabile {
		t.Fatalf("una bozza manuale corretta ha solo il gate: %v", codici(r.Blocchi))
	}
	if len(r.DocumentiPreassociati) != 1 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoApplicabile ||
		r.DocumentiPreassociati[0].Gruppo != GruppoManuale || len(r.EffettiDocumentali.Manuale) != 1 {
		t.Fatalf("la PB manuale e' mostrata con il suo effetto: %+v %+v", r.DocumentiPreassociati, r.EffettiDocumentali)
	}
	if r.CriterioPrevisto.Criterio != "nessuno" || r.VerificaPrevista.Stato != confermabom.StatoVerificaNonCalcolata || r.VerificaPrevista.Verificata != nil {
		t.Fatalf("criterio e verifica: %+v %+v", r.CriterioPrevisto, r.VerificaPrevista)
	}
	if r.SorgenteLegata != nil || r.EffettoSullaFonte.Promozione || r.Sorgente.Valida {
		t.Fatalf("nessuna sorgente: %+v %+v", r.SorgenteLegata, r.EffettoSullaFonte)
	}
	if !reFirma64.MatchString(r.FirmaAnteprima) || len(trova(r.Avvisi, AvvisoVerificaDopoCommit)) != 1 {
		t.Fatalf("firma e avvisi: %s %v", r.FirmaAnteprima, codici(r.Avvisi))
	}
	if len(r.Identita) != 3 || r.Identita[1].ChiaveBozza != "k-telaio" || !r.Identita[1].NonAncoraConfermato || r.Identita[1].EsitoIdentita != "manuale" {
		t.Fatalf("identita': %+v", r.Identita)
	}
	if len(r.Archi) != 2 || r.Archi[0].Azione != "aggiunto" {
		t.Fatalf("archi: %+v", r.Archi)
	}
}

// R39-20: mono_pezzo senza STEP: il gate porta mono_pezzo, nessun MONO_PEZZO_SENZA_STEP in piu'.
func TestR4MonoPezzoSenzaStepAllineatoAlGate(t *testing.T) {
	f := nuovaFixR4(t)
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-mono", "tipo": "mono_pezzo", "motivo": "pezzo unico tornito"})
	r := f.calcola()
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || !g[0].MonoPezzo {
		t.Fatalf("gate con mono_pezzo: %+v", r.Blocchi)
	}
	if len(trova(r.Blocchi, BloccoMonoPezzoSenzaStep)) != 0 || len(r.Blocchi) != 1 {
		t.Fatalf("nessun doppione: %v", codici(r.Blocchi))
	}
	if r.CriterioPrevisto.Criterio != "nessuno" {
		t.Fatalf("mono_pezzo senza sorgente non e' un criterio: %+v", r.CriterioPrevisto)
	}
	// con la sorgente valida: criterio mono_pezzo, nessun blocco
	f.sorgenteValida(shaProva('e'))
	r = f.calcola()
	if len(r.Blocchi) != 0 || r.CriterioPrevisto.Criterio != confermabom.CriterioMonoPezzo {
		t.Fatalf("mono_pezzo con lo STEP: %v %+v", codici(r.Blocchi), r.CriterioPrevisto)
	}
	if r.VerificaPrevista.Verificata != nil && *r.VerificaPrevista.Verificata {
		t.Fatalf("mai «si'» senza i conflitti del motore: %+v", r.VerificaPrevista)
	}
}

// D39 §1, §3: zero figli, STEP S8 valido con zero figli: R5 possibile se il resto passa, ma niente mono_pezzo ne' verifica dedotti.
func TestR4ZeroFigliNienteDedotto(t *testing.T) {
	f := nuovaFixR4(t)
	d := f.sorgenteValida(shaProva('e'))
	r := f.calcola()
	if len(r.Blocchi) != 0 || !r.Confermabile {
		t.Fatalf("confermabile: %v", codici(r.Blocchi))
	}
	if r.CriterioPrevisto.Criterio != "nessuno" || !strings.Contains(r.CriterioPrevisto.Motivo, "mono_pezzo") {
		t.Fatalf("criterio: %+v", r.CriterioPrevisto)
	}
	if r.VerificaPrevista.Verificata != nil && *r.VerificaPrevista.Verificata {
		t.Fatalf("verifica dedotta: %+v", r.VerificaPrevista)
	}
	for _, x := range r.RegistroDecisioni {
		if x.Tipo == DecisioneMonoPezzo {
			t.Fatalf("mono_pezzo dedotto nel registro: %+v", x)
		}
	}
	if len(trova(r.Avvisi, AvvisoProdottoSenzaFigli)) != 1 || len(trova(r.Avvisi, AvvisoSorgenteNonAutorizzata)) != 1 {
		t.Fatalf("avvisi: %v", codici(r.Avvisi))
	}
	if r.SorgenteLegata == nil || !r.SorgenteLegata.Confermata || r.EffettoSullaFonte.StepDopo == nil || *r.EffettoSullaFonte.StepDopo != d ||
		!r.EffettoSullaFonte.Promozione {
		t.Fatalf("riga legato come sorgente ed effetto sulla fonte: %+v %+v", r.SorgenteLegata, r.EffettoSullaFonte)
	}
}

// D39 §7: la fonte A della working diversa dalla sorgente B della bozza non e' da sola BASE_SUPERATA; R4 mostra la promozione
// e la sostituzione documentale di A.
func TestR4FonteADellaWorkingDiversaDaB(t *testing.T) {
	f := nuovaFixR4(t)
	docA := idDi("r4-documento-A")
	f.fonti[f.prodotto] = confermabom.FonteAutorizzata{DocumentoID: docA, Sha256: shaProva('a')}
	f.sorgenteValida(shaProva('e'))
	r := f.calcola()
	if len(trova(r.Blocchi, BloccoBaseSuperata)) != 0 || r.Base.Stato != BaseValida {
		t.Fatalf("base superata per la sola fonte: %v", codici(r.Blocchi))
	}
	e := r.EffettoSullaFonte
	if !e.Promozione || e.SostituzioneDi == nil || *e.SostituzioneDi != docA || e.ShaPrima != shaProva('a') || e.ShaDopo != shaProva('e') {
		t.Fatalf("effetto sulla fonte: %+v", e)
	}
	if a := trova(r.Avvisi, AvvisoSorgenteNonAutorizzata); len(a) != 1 || !strings.Contains(a[0].Testo, "al posto di") {
		t.Fatalf("avviso informativo: %+v", r.Avvisi)
	}
	// con il gate l'avviso SORGENTE_NON_AUTORIZZATA non si da'
	f.sorg.Valida, f.sorg.Motivo = false, SorgenteMotivoRevocata
	r = f.calcola()
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoRevocata || g[0].Sorgente == nil {
		t.Fatalf("revoca: %+v", r.Blocchi)
	}
	if len(trova(r.Avvisi, AvvisoSorgenteNonAutorizzata)) != 0 || r.EffettoSullaFonte.Promozione {
		t.Fatalf("con il gate niente avviso ne' promozione: %v %+v", codici(r.Avvisi), r.EffettoSullaFonte)
	}
	// lo STEP storico senza S8 (D-AR04 (iii)): blocco con il legame storico
	f.sorg.Motivo, f.sorg.LegameStorico = SorgenteMotivoNonConfermata, true
	r = f.calcola()
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || !g[0].LegameStorico || !strings.Contains(g[0].Testo, "legame storico") {
		t.Fatalf("legame storico: %+v", r.Blocchi)
	}
}

// D-PB-04 A [U]: i gruppi per provenienza, la legacy da verificare bloccata, le proposte rimosse dal giornale.
func TestR4GruppiDiProvenienza(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.nodoMan("k-b", "ACM-1200", "sciolto")
	f.nodoMan("k-c", "ACM-1300", "sciolto")
	f.nodoMan("k-d", "ACM-1400", "sciolto")
	for _, k := range []string{"k-a", "k-b", "k-c", "k-d"} {
		f.arco("k-root", k, 1)
	}
	reg := shaProva('9')
	a1, a2, a3, a4 := f.occorrenza("a.pdf", "ACM-1100"), f.occorrenza("b.pdf", "ACM-1200"), f.occorrenza("c.pdf", "ACM-1300"), f.occorrenza("d.pdf", "ACM-1400")
	f.preassocia(a1, "k-a", ProvenienzaMotore, reg)
	f.preassocia(a2, "k-b", ProvenienzaCorretta, reg)
	f.preassocia(a3, "k-c", ProvenienzaManuale, "")
	f.preassocia(a4, "k-d", ProvenienzaLegacy, "")
	rimossa := idDi("r4-file-rimossa")
	f.conBozza = true
	f.eventi = []EventoLetto{{N: 1, Versione: 1, Azione: AzBozzaAperta, Origine: "server", UtenteID: f.utente,
		Dopo: canonico(map[string]any{"partenza": "step", "preassociazioni_iniziali": []any{
			map[string]any{"allegato_id": a1, "chiave_bozza": "k-a", "provenienza": "motore", "importazione": reg},
			map[string]any{"allegato_id": rimossa, "chiave_bozza": "k-c", "provenienza": "motore", "importazione": reg}}})}}
	in := f.ingressi()
	r, err := calcolaR4(in)
	if err != nil {
		t.Fatal(err)
	}
	e := r.EffettiDocumentali
	if len(e.MotoreIntatta) != 1 || e.MotoreIntatta[0].AllegatoID != a1 || e.MotoreIntatta[0].Importazione != reg ||
		len(e.Corretta) != 1 || e.Corretta[0].AllegatoID != a2 || len(e.Manuale) != 1 || e.Manuale[0].AllegatoID != a3 ||
		len(e.LegacyDaVerificare) != 1 || e.LegacyDaVerificare[0].AllegatoID != a4 {
		t.Fatalf("gruppi: %+v", e)
	}
	if len(e.ProposteRimosse) != 1 || e.ProposteRimosse[0].AllegatoID != rimossa || e.ProposteRimosse[0].Importazione != reg {
		t.Fatalf("proposte rimosse: %+v", e.ProposteRimosse)
	}
	if b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Motivo != associazioni.MotivoLegacyDaVerificare {
		t.Fatalf("legacy da verificare: %+v", r.Blocchi)
	}
	for _, p := range r.DocumentiPreassociati {
		if p.Allegato == a1 && (p.Gruppo != GruppoMotoreIntatta || p.Esito != associazioni.EsitoApplicabile) {
			t.Fatalf("PB motore: %+v", p)
		}
	}
}

// D39 §5: lo STEP sorgente non e' una PB: preassociato a un nodo e' legato_come_sorgente; la riga «legato come sorgente» sta
// fuori dalle PB.
func TestR4StepSorgenteFuoriDallePB(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.arco("k-root", "k-a", 1)
	step := idDi("r4-step")
	f.fatti.Occorrenze[step] = associazioni.OccorrenzaPiano{DellaRfq: true, Attivo: true, Preassociabile: true, Sha256: shaProva('e'), NomeFile: "ACM-1000.stp",
		ConProposta: true, TipoProposto: "cad_3d", CodiceLetto: "ACM-1100"}
	f.preassocia(step, "k-a", ProvenienzaManuale, "")
	r := f.calcola()
	if b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Motivo != associazioni.MotivoLegatoComeSorgente {
		t.Fatalf("STEP sorgente come PB: %+v", r.Blocchi)
	}
	if r.SorgenteLegata == nil || r.SorgenteLegata.AllegatoID != step {
		t.Fatalf("riga legato come sorgente: %+v", r.SorgenteLegata)
	}
	// uno STEP di un particolare (contenuto diverso) si preassocia come un PDF (D-2 (c) [U])
	f2 := nuovaFixR4(t)
	f2.sorgenteValida(shaProva('e'))
	f2.nodoMan("k-a", "ACM-1100", "sciolto")
	f2.arco("k-root", "k-a", 1)
	p := idDi("r4-file-particolare-stp")
	f2.fatti.Occorrenze[p] = associazioni.OccorrenzaPiano{DellaRfq: true, Attivo: true, Preassociabile: true, Sha256: shaProva('f'), NomeFile: "ACM-1100.stp",
		ConProposta: true, TipoProposto: "cad_3d", CodiceLetto: "ACM-1100"}
	f2.preassocia(p, "k-a", ProvenienzaManuale, "")
	r = f2.calcola()
	if len(r.Blocchi) != 0 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoApplicabile {
		t.Fatalf("STEP documentale: %v %+v", codici(r.Blocchi), r.DocumentiPreassociati)
	}
}

// Prova 7 (dir. 35) nella parte di R4: la AR attiva verso un altro componente blocca (mai una sovrascrittura); con lo
// svincolo nella stessa bozza e' una sostituzione mostrata negli spostamenti di AR.
func TestR4Prova7FileGiaAssociatoAltrove(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	x := f.componente("x", "ACM-1100", "sciolto")
	y := f.componente("y", "ACM-1200", "sciolto")
	f.arcoW(f.prodotto, x, 1)
	f.arcoW(f.prodotto, y, 1)
	f.nodoReg("k-x", x, "sciolto")
	f.nodoReg("k-y", y, "sciolto")
	f.arco("k-root", "k-x", 1)
	f.arco("k-root", "k-y", 1)
	a := f.occorrenza("doc.pdf", "ACM-1200")
	ar := idDi("r4-ar")
	f.fatti.Attive[a] = associazioni.Associazione{ID: ar, Allegato: a, Componente: x, Stato: associazioni.StatoAttiva}
	f.preassocia(a, "k-y", ProvenienzaManuale, "")
	r := f.calcola()
	b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab)
	if len(b) != 1 || b[0].Motivo != associazioni.MotivoGiaAssociataAltrove || b[0].Variante != associazioni.VarianteStessoProdotto ||
		b[0].ComponenteID == nil || *b[0].ComponenteID != x || len(b[0].Rimedi) != 2 {
		t.Fatalf("gia' associata: %+v", b)
	}
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-sv", "tipo": "scollega_documento", "allegato_id": a, "componente_id": x})
	r = f.calcola()
	if len(r.Blocchi) != 0 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoSostituzione {
		t.Fatalf("sostituzione: %v %+v", codici(r.Blocchi), r.DocumentiPreassociati)
	}
	sp := r.EffettiDocumentali.SpostamentiAR
	if len(sp) != 2 || sp[0].Tipo != associazioni.EsitoSostituzione || *sp[0].DaComponente != x || *sp[0].AComponente != y || sp[1].Tipo != "svincolo" ||
		sp[1].Esito != associazioni.EsitoApplicabile {
		t.Fatalf("spostamenti di AR: %+v", sp)
	}
}

// Prova 5 (dir. 35, D-ASS-02 [U]) nella parte di R4: lo spostamento di un sottoassieme dentro il prodotto conserva il
// sottoalbero e le PB, non e' un'assenza, e porta la voce dell'arco spostato; un ciclo con la working degli altri prodotti
// blocca.
func TestR4Prova5SpostamentoECiclo(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	s := f.componente("s", "ACM-1100", "sottoassieme")
	x := f.componente("x", "ACM-1110", "sciolto")
	y := f.componente("y", "ACM-1200", "sottoassieme")
	z := f.componente("z", "ACM-1210", "sciolto")
	f.arcoW(f.prodotto, s, 1)
	f.arcoW(s, x, 2)
	f.arcoW(f.prodotto, y, 1)
	f.arcoW(y, z, 1)
	for _, n := range []struct {
		k  string
		id uuid.UUID
		t  string
	}{{"k-s", s, "sottoassieme"}, {"k-x", x, "sciolto"}, {"k-y", y, "sottoassieme"}, {"k-z", z, "sciolto"}} {
		f.nodoReg(n.k, n.id, n.t)
	}
	f.arco("k-root", "k-y", 1)
	f.arco("k-y", "k-s", 1)
	f.arco("k-s", "k-x", 2)
	f.arco("k-y", "k-z", 1)
	a := f.occorrenza("x.pdf", "ACM-1110")
	f.preassocia(a, "k-x", ProvenienzaManuale, "")
	r := f.calcola()
	if len(r.Blocchi) != 0 {
		t.Fatalf("spostamento dentro il prodotto: %v %+v", codici(r.Blocchi), r.Blocchi)
	}
	spostati := 0
	for _, x := range r.Archi {
		if x.Azione == "spostato" {
			spostati++
			if x.Figlio.ChiaveBozza != "k-s" || x.PadrePrima == nil || x.PadrePrima.ComponenteID == nil || *x.PadrePrima.ComponenteID != f.prodotto {
				t.Fatalf("arco spostato: %+v", x)
			}
		}
		if x.Azione == "tolto" {
			t.Fatalf("lo spostamento non toglie archi: %+v", x)
		}
	}
	if spostati != 1 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoApplicabile {
		t.Fatalf("archi e PB: %+v %+v", r.Archi, r.DocumentiPreassociati)
	}
	trovata := false
	for _, v := range r.RegistroDecisioni {
		trovata = trovata || (v.OrigineVoce == "arco" && v.Tipo == "spostato")
	}
	if !trovata {
		t.Fatalf("registro senza l'arco spostato: %+v", r.RegistroDecisioni)
	}
	// il ciclo con la working di un altro prodotto: nella working di P2 c'e' W → K; la bozza riusa K e W e mette W sotto K
	p2 := f.componente("p2", "ACM-2000", "finito")
	w := f.componente("w", "ACM-2100", "sottoassieme")
	k := f.componente("k", "ACM-2110", "sottoassieme")
	f.arcoW(p2, w, 1)
	f.arcoW(w, k, 1)
	f.nodoReg("k-k", k, "sottoassieme")
	f.nodoReg("k-w", w, "sottoassieme")
	f.arco("k-root", "k-k", 1)
	f.arco("k-k", "k-w", 1)
	r = f.calcola()
	if c := trova(r.Blocchi, BloccoCiclo); len(c) != 1 || len(c[0].Chiavi) != 2 || len(trova(r.Blocchi, BloccoAssenzaNonDecisa)) != 0 {
		t.Fatalf("ciclo bozza ∪ working: %+v", r.Blocchi)
	}
}

// ASSENZA_NON_DECISA: la partenza vuota su una working con figli; l'esclusione con la decisione la risolve, e la AR del
// componente escluso resta («Mantieni»: avviso, nessun blocco); un registrato tolto con la AR blocca anche per la AR.
func TestR4AssenzeEsclusiEUscite(t *testing.T) {
	f := nuovaFixR4(t)
	x := f.componente("x", "ACM-1100", "sciolto")
	f.arcoW(f.prodotto, x, 1)
	a := f.occorrenza("x.pdf", "ACM-1100")
	f.fatti.Attive[a] = associazioni.Associazione{ID: idDi("r4-ar-x"), Allegato: a, Componente: x, Stato: associazioni.StatoAttiva}
	r := f.calcola()
	if b := trova(r.Blocchi, BloccoAssenzaNonDecisa); len(b) != 1 || *b[0].ComponenteID != x {
		t.Fatalf("assenza: %+v", r.Blocchi)
	}
	if b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Motivo != associazioni.MotivoNodoUscenteConAR {
		t.Fatalf("uscita con AR: %+v", r.Blocchi)
	}
	if len(r.Fuori) != 1 || r.Fuori[0].Motivo != "assente" {
		t.Fatalf("fuori: %+v", r.Fuori)
	}
	// «Escludi» con il motivo: niente assenza, la AR resta (avviso ESCLUSO_CON_DOCUMENTI, ar_invariate)
	f.nodi = append(f.nodi, map[string]any{"chiave_bozza": "k-x", "componente_id": x, "op": "esclude", "tipo": "sciolto", "origine": "registrato",
		"identita": map[string]any{"emittente": f.emittente(), "emittente_presunto": true, "codice": "ACM-1100", "modo": "registrato"}})
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-escl", "tipo": "escludi", "componente_id": x, "motivo": "non serve piu'"})
	r = f.calcola()
	if len(trova(r.Blocchi, BloccoAssenzaNonDecisa)) != 0 || len(trova(r.Blocchi, BloccoPreassociazioneNonApplicab)) != 0 {
		t.Fatalf("escluso: %v", codici(r.Blocchi))
	}
	if len(trova(r.Avvisi, AvvisoEsclusoConDocumenti)) != 1 || len(r.EffettiDocumentali.ARInvariate) != 1 ||
		r.EffettiDocumentali.ARInvariate[0].Motivo != "escluso_mantieni" {
		t.Fatalf("Mantieni: %v %+v", codici(r.Avvisi), r.EffettiDocumentali.ARInvariate)
	}
	if len(r.Esclusi) != 1 || r.Esclusi[0].Origine != DecisioneEscludi || len(r.Fuori) != 1 || r.Fuori[0].Motivo != "escluso" {
		t.Fatalf("esclusi e fuori: %+v %+v", r.Esclusi, r.Fuori)
	}
}

// Prova 8 (dir. 35) e D-TAB-03 A nella parte di R4: un componente condiviso con un altro prodotto confermato: condivisione[]
// attribuita a chi conferma, PRODOTTI_TOCCATI, e ARCO_CONDIVISO_QTA finche' non c'e' la decisione arco_condiviso.
func TestR4Prova8ComponenteCondiviso(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	s := f.componente("s", "ACM-1100", "sottoassieme")
	x := f.componente("x", "ACM-1110", "sciolto")
	p2 := f.componente("p2", "ACM-2000", "finito")
	f.arcoW(f.prodotto, s, 1)
	f.arcoW(s, x, 2)
	f.arcoW(p2, s, 1)
	f.nodoReg("k-s", s, "sottoassieme")
	f.nodoReg("k-x", x, "sciolto")
	f.arco("k-root", "k-s", 1)
	f.arco("k-s", "k-x", 3)
	// la conferma valida di P2 (contatore uguale a quello della working)
	st := confermabom.NuovoStatoRFQ(f.thread, 7, f.comp, f.archiW, nil, nil)
	fp, err := st.Firma(p2)
	if err != nil {
		t.Fatal(err)
	}
	f.conferme = []confermabom.TestataConferma{{Conferma: confermabom.Conferma{ID: idDi("r4-conf-p2"), Numero: 1, Contatore: 7, FirmaStruttura: fp}, Prodotto: p2, Formato: 3}}
	r := f.calcola()
	if len(r.Condivisione) != 1 || r.Condivisione[0].ProdottoID != p2 || !r.Condivisione[0].ConfermaValida ||
		!strings.Contains(r.Condivisione[0].Conseguenza, "chi la conferma") || len(r.Condivisione[0].Elementi) != 1 ||
		*r.Condivisione[0].Elementi[0].ComponenteID != s {
		t.Fatalf("condivisione: %+v", r.Condivisione)
	}
	if b := trova(r.Blocchi, BloccoArcoCondivisoQta); len(b) != 1 || len(b[0].Prodotti) != 1 || b[0].Prodotti[0] != p2 {
		t.Fatalf("arco condiviso: %+v", r.Blocchi)
	}
	if a := trova(r.Avvisi, AvvisoProdottiToccati); len(a) != 1 {
		t.Fatalf("prodotti toccati: %v", codici(r.Avvisi))
	}
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-ac", "tipo": "arco_condiviso", "padre_id": s, "figlio_id": x, "qta": 3, "prodotti": []any{p2}})
	r = f.calcola()
	if len(trova(r.Blocchi, BloccoArcoCondivisoQta)) != 0 {
		t.Fatalf("con arco_condiviso: %v", codici(r.Blocchi))
	}
}

// Prova 4 (dir. 35, D-ASS-01 [U]) nella parte di R4: la ricodifica di un componente con una AR: la AR resta e si allinea
// (ar_invariate); una PB sul nodo con il codice vecchio va in conflitto (il dialogo non e' una deroga); documenti gia' sul
// NAS: RICODIFICA_FILE_NAS; il codice di un altro componente: RICODIFICA_COLLIDE.
func TestR4Prova4ModificaDiCodice(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	x := f.componente("x", "ACM-1100", "sciolto")
	f.componente("occupante", "ACM-1300", "sciolto")
	f.arcoW(f.prodotto, x, 1)
	f.nodoReg("k-x", x, "sciolto")
	f.arco("k-root", "k-x", 1)
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-ric", "tipo": "ricodifica", "componente_id": x, "codice_prima": "ACM-1100",
		"codice_dopo": "ACM-1200", "motivo": "codice sbagliato", "emittente_dopo": f.emittente()})
	ar := f.occorrenza("ar.pdf", "ACM-1100")
	f.fatti.Attive[ar] = associazioni.Associazione{ID: idDi("r4-ar-ric"), Allegato: ar, Componente: x, Stato: associazioni.StatoAttiva}
	pb := f.occorrenza("pb.pdf", "ACM-1100")
	f.preassocia(pb, "k-x", ProvenienzaManuale, "")
	r := f.calcola()
	if inv := r.EffettiDocumentali.ARInvariate; len(inv) != 1 || inv[0].Motivo != "ricodifica" || inv[0].AllegatoID != ar {
		t.Fatalf("AR mantenuta con la ricodifica: %+v", inv)
	}
	if b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Motivo != associazioni.MotivoCodiceDiverso || *b[0].AllegatoID != pb {
		t.Fatalf("la PB con il codice di prima resta in conflitto (il dialogo non e' una deroga): %+v", r.Blocchi)
	}
	if r.Identita[1].Codice.Dopo != "ACM-1200" || r.Identita[1].Codice.Prima == nil || *r.Identita[1].Codice.Prima != "ACM-1100" {
		t.Fatalf("identita' prima e dopo: %+v", r.Identita[1])
	}
	f.scrittiNAS[x] = 2
	f.decisioni[0].(map[string]any)["codice_dopo"] = "ACM-1300"
	r = f.calcola()
	if len(trova(r.Blocchi, BloccoRicodificaFileNAS)) != 1 || len(trova(r.Blocchi, BloccoRicodificaCollide)) != 1 {
		t.Fatalf("ricodifica: %v", codici(r.Blocchi))
	}
}

// Identita': due nodi scritti a mano con lo stesso codice e dati diversi; un nodo nuovo con il codice di un registrato.
func TestR4IdentitaECollisioni(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	f.componente("libero", "ACM-5000", "sciolto")
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.nodoMan("k-b", "ACM-1100", "commerciale")
	f.nodoMan("k-c", "ACM-5000", "sciolto")
	f.nodoMan("k-d", "", "sciolto")
	for _, k := range []string{"k-a", "k-b", "k-c", "k-d"} {
		f.arco("k-root", k, 1)
	}
	r := f.calcola()
	for _, c := range []string{BloccoDueCreaStessoCodice, BloccoCreaSuEsistente, BloccoCodiceDaAssegnare, BloccoArcoDuplicato} {
		if len(trova(r.Blocchi, c)) != 1 {
			t.Fatalf("%s: %v", c, codici(r.Blocchi))
		}
	}
	if len(r.Identita) != 5 {
		t.Fatalf("identita': %d", len(r.Identita))
	}
}

// La struttura: D1, E37, FIGLIO_SENZA_PADRE, PRODOTTO_PROTETTO, MONO_PEZZO_CON_FIGLI, CONGELATA.
func TestR4BlocchiDiStruttura(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	f.nodoMan("k-sa", "ACM-1100", "sottoassieme")
	f.nodoMan("k-pa", "ACM-1200", "sciolto")
	f.nodoMan("k-co", "ACM-1300", "commerciale")
	f.nodoMan("k-fi", "ACM-1400", "sciolto")
	f.nodoMan("k-orfano", "ACM-1500", "sciolto")
	f.arco("k-root", "k-sa", 1)
	f.arco("k-root", "k-pa", 1)
	f.arco("k-root", "k-co", 1)
	f.arco("k-pa", "k-fi", 1)
	f.arco("k-co", "k-orfano", 1)
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-mono", "tipo": "mono_pezzo", "motivo": "pezzo unico"})
	f.congelata = true
	r := f.calcola()
	for _, c := range []string{BloccoD1UltimoFiglio, BloccoD1ParticolareConFigli, BloccoE37NonContenitore,
		BloccoMonoPezzoConFigli, BloccoCongelata} {
		if len(trova(r.Blocchi, c)) == 0 {
			t.Fatalf("%s: %v", c, codici(r.Blocchi))
		}
	}
	f2 := nuovaFixR4(t)
	f2.nodoMan("k-orfano", "ACM-1500", "sciolto")
	if r := f2.calcola(); len(trova(r.Blocchi, BloccoFiglioSenzaPadre)) != 1 {
		t.Fatalf("figlio senza padre: %v", codici(r.Blocchi))
	}
}

// FILE_CONTESO_FRA_PRODOTTI: lo stesso file preassociato dalla bozza di un altro prodotto a un altro componente, finche'
// nessuna delle due e' confermata.
func TestR4FileContesoFraProdotti(t *testing.T) {
	f := nuovaFixR4(t)
	f.sorgenteValida(shaProva('e'))
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.arco("k-root", "k-a", 1)
	a := f.occorrenza("a.pdf", "ACM-1100")
	f.preassocia(a, "k-a", ProvenienzaManuale, "")
	p2 := idDi("r4-p2")
	f.altre = []BozzaDellaRfq{{ProdottoID: p2, Formato: 3, Preassociazioni: []PreassociazioneDellaBozza{{AllegatoID: a, ChiaveBozza: "k-z",
		Nodo: NodoDellaBozza{ChiaveBozza: "k-z", Codice: "ACM-1100"}}}}}
	r := f.calcola()
	if c := trova(r.Avvisi, AvvisoFileContesoFraProdotti); len(c) != 1 || len(c[0].Prodotti) != 1 || c[0].Prodotti[0] != p2 {
		t.Fatalf("file conteso: %+v", r.Avvisi)
	}
	if len(r.Blocchi) != 0 {
		t.Fatalf("finche' nessuna e' confermata e' un avviso: %v", codici(r.Blocchi))
	}
	// dopo la prima R5 c'e' la AR: blocco gia_associata_altrove (altro prodotto), niente avviso
	x2 := idDi("r4-x2")
	f.fatti.Attive[a] = associazioni.Associazione{ID: idDi("r4-ar-p2"), Allegato: a, Componente: x2, Stato: associazioni.StatoAttiva}
	r = f.calcola()
	if b := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Variante != associazioni.VarianteAltroProdotto {
		t.Fatalf("dopo la prima conferma: %+v", r.Blocchi)
	}
	if len(trova(r.Avvisi, AvvisoFileContesoFraProdotti)) != 0 {
		t.Fatalf("avviso dopo la AR: %v", codici(r.Avvisi))
	}
}

// La firma: stessa risposta, stessa firma; cambia con lo snapshot e con un blocco; non cambia con i file in analisi; la
// risposta si codifica.
func TestR4Firma(t *testing.T) {
	f := nuovaFixR4(t)
	f.nodoMan("k-a", "ACM-1100", "sciolto")
	f.arco("k-root", "k-a", 1)
	r1, r2 := f.calcola(), f.calcola()
	if r1.FirmaAnteprima != r2.FirmaAnteprima {
		t.Fatal("firma non deterministica")
	}
	in := f.ingressi()
	in.inAnalisi = []FileInAnalisi{{AllegatoID: idDi("r4-analisi"), NomeFile: "x.pdf", Stato: AnalisiInAnalisi}}
	r3, err := calcolaR4(in)
	if err != nil || r3.FirmaAnteprima != r1.FirmaAnteprima || len(r3.FileInAnalisi) != 1 {
		t.Fatalf("file in analisi fuori firma: %v %s", err, r3.FirmaAnteprima)
	}
	// il riepilogo leggero (senza il motore) ha un'altra firma e non e' confermabile; il motore occupato da' la stessa firma
	// del leggero (l'errore e' fuori firma) con l'errore
	leggero := f.ingressi()
	leggero.avvisiMotore, leggero.esitoMotore = false, nil
	r4, _ := calcolaR4(leggero)
	if r4.FirmaAnteprima == r1.FirmaAnteprima || r4.AvvisiMotoreCalcolati || r4.AvvisiMotoreErrore != nil || r4.Confermabile || !r1.AvvisiMotoreCalcolati {
		t.Fatalf("riepilogo leggero: %s %+v", r4.FirmaAnteprima, r4.AvvisiMotoreErrore)
	}
	occupato := f.ingressi()
	occupato.esitoMotore = &EsitoMotoreR4{Err: ErrMotoreOccupato}
	r4b, _ := calcolaR4(occupato)
	if r4b.FirmaAnteprima != r4.FirmaAnteprima || r4b.AvvisiMotoreErrore == nil || r4b.AvvisiMotoreErrore.Codice != MotoreErroreOccupato || r4b.Confermabile {
		t.Fatalf("motore occupato: %s %+v", r4b.FirmaAnteprima, r4b.AvvisiMotoreErrore)
	}
	f.archi[0].(map[string]any)["qta"] = 2
	if r5 := f.calcola(); r5.FirmaAnteprima == r1.FirmaAnteprima {
		t.Fatal("la firma non cambia con lo snapshot")
	}
	f.archi[0].(map[string]any)["qta"] = 1
	f.sorgenteValida(shaProva('e'))
	if r6 := f.calcola(); r6.FirmaAnteprima == r1.FirmaAnteprima {
		t.Fatal("la firma non cambia con la sorgente e il gate")
	}
	raw, err := json.Marshal(r1)
	if err != nil || !strings.Contains(string(raw), `"blocchi"`) || !strings.Contains(string(raw), `"effetti_documentali"`) ||
		strings.Contains(string(raw), "esitiPiano") {
		t.Fatalf("JSON: %v %s", err, raw)
	}
	// la firma si ricalcola dalla risposta e dallo snapshot
	f2, err := firmaR4(r1, f.ingressiConArchi(1).canonico)
	if err != nil || f2 != r1.FirmaAnteprima {
		t.Fatalf("firma ricalcolata: %v %s %s", err, f2, r1.FirmaAnteprima)
	}
}

func (f *fixR4) ingressiConArchi(qta int) *ingressiR4 {
	f.archi[0].(map[string]any)["qta"] = qta
	f.sorgente, f.sorg = nil, SorgenteR4{Motivo: SorgenteMotivoSenzaSorgente}
	return f.ingressi()
}

// Il corpo di R4: un corpo formato 2 e' formato_da_convertire; bozza_versione solo con bozza_id; i tre campi della richiesta.
func TestR4LeggiRichiesta(t *testing.T) {
	p := idDi("r4-prodotto")
	base := `"prodotto_id":"` + p.String() + `","versione_bom_base":{"conferma_id":null,"numero":0,"contatore":0},"firma_base":"` + firmaProva + `",` +
		`"nodi":[{"chiave_bozza":"k-root","componente_id":"` + p.String() + `","op":"riusa","tipo":"finito","origine":"registrato",` +
		`"identita":{"emittente":"cliente:` + idDi("r4-cliente").String() + `","emittente_presunto":true,"codice":"ACM-1000","modo":"registrato"}}],` +
		`"archi":[],"decisioni":[],"documenti_preassociati":[]`
	if _, err := LeggiRichiestaR4([]byte(`{"formato":2,` + base + `}`)); err == nil || err.(*Errore).Motivo != MotivoFormatoDaConvertire {
		t.Fatalf("formato 2: %v", err)
	}
	b := idDi("r4-bozza")
	r, err := LeggiRichiestaR4([]byte(`{"formato":3,` + base + `,"bozza_id":"` + b.String() + `","bozza_versione":3,"chiave_idempotenza":"` + idDi("r4-k").String() + `"}`))
	if err != nil || r.BozzaID == nil || *r.BozzaID != b || r.BozzaVersione != 3 || r.ChiaveIdempotenza == nil || r.Snapshot.ProdottoID != p {
		t.Fatalf("corpo: %+v %v", r, err)
	}
	if _, err := LeggiRichiestaR4([]byte(`{"formato":3,` + base + `,"bozza_versione":3}`)); err == nil || err.(*Errore).Campo != "bozza_versione" {
		t.Fatalf("bozza_versione senza bozza_id: %v", err)
	}
	if _, err := LeggiRichiestaR4([]byte(`{"formato":3,` + base + `,"sconosciuto":1}`)); err == nil {
		t.Fatal("campo sconosciuto accettato")
	}
	if _, err := LeggiRichiestaR4([]byte(`[]`)); err == nil {
		t.Fatal("un elenco accettato")
	}
}

// Prova 10 (dir. 35) nella parte di R4 e contenuto-atto-1 (U2 §6.2): una bozza uguale alla conferma di base formato 3 dice
// uguale_alla_base (R5 rifiutera' NESSUNA_MODIFICA); una quantita' cambiata la rende diversa, con la differenza firmata.
func TestR4UgualeAllaBase(t *testing.T) {
	f := nuovaFixR4(t)
	sha := shaProva('e')
	f.sorgenteValida(sha)
	x := f.componente("x", "ACM-1100", "sciolto")
	f.arcoW(f.prodotto, x, 2)
	f.nodoReg("k-x", x, "sciolto")
	f.arco("k-root", "k-x", 2)
	f.fonti[f.prodotto] = confermabom.FonteAutorizzata{DocumentoID: *f.sorg.DocumentoID, Sha256: sha}
	base := f.snapshot()
	in := f.ingressi()
	in.confBase = &confermaBaseR4{formato: FormatoSnapshot3, snap: &base, mappa: map[string]uuid.UUID{}, criterio: confermabom.CriterioStep, radiceSha: sha}
	r, err := calcolaR4(in)
	if err != nil {
		t.Fatal(err)
	}
	if !r.UgualeAllaBase || len(r.DifferenzeDallaBase) != 0 {
		t.Fatalf("uguale alla base: %v %+v", r.UgualeAllaBase, r.DifferenzeDallaBase)
	}
	f.archi[0].(map[string]any)["qta"] = 3
	in = f.ingressi()
	in.confBase = &confermaBaseR4{formato: FormatoSnapshot3, snap: &base, mappa: map[string]uuid.UUID{}, criterio: confermabom.CriterioStep, radiceSha: sha}
	r2, err := calcolaR4(in)
	if err != nil {
		t.Fatal(err)
	}
	if r2.UgualeAllaBase || len(r2.DifferenzeDallaBase) == 0 || r2.FirmaAnteprima == r.FirmaAnteprima {
		t.Fatalf("diversa dalla base: %v %+v", r2.UgualeAllaBase, r2.DifferenzeDallaBase)
	}
	// una base formato 2: mai uguale (la prima conferma formato 3 e' sempre ammessa)
	in.confBase.formato = FormatoSnapshot
	if r3, _ := calcolaR4(in); r3.UgualeAllaBase {
		t.Fatal("uguale a una base formato 2")
	}
}
