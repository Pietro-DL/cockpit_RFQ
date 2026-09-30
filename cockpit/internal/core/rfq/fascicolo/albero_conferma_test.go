package fascicolo

// L1 — la conferma dell'albero e i ritocchi del riepilogo (giro 4, fase 4.4a.1b; domande 28 = A, 29b = A, 30 seconda
// risposta, 6a, 6b; dalla verifica della 4.4a.1a): la tendina e' il ✓, un pezzo aggiunto sotto un padre che va via si
// rifiuta, un componente archiviato segue 6a e 6b come un attivo, una foglia senza codice riceve la proposta
// commerciale quando prende un codice, la revisione nuova di un componente che c'e', il segno letto da F7, il corpo
// della conferma; dalla verifica della 4.4a.1b, l'aggancio per codice di prima che resta da decidere e la firma del
// riepilogo con l'effetto dei cambi di tipo. Scene ACME inventate.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// scenaViteL1: il prodotto 7120001 con lo STEP del prodotto: 7120010 e la vite normata 7121007.
func scenaViteL1(t *testing.T) *scena {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=VITE TCEI ISO 4762 M6X16/7121007"}, []string{"#1>#2", "#1>#3*4"})
	return sc
}

// Prova (fase 4.4a.1b, dalla verifica della 4.4a.1a): scegliere «particolare commerciale» con la tendina per la vite
// con la proposta e' il ✓ della proposta, un gesto esplicito della persona: la proposta e' «si» (con la tendina), il
// riepilogo e' confermabile e il pezzo nasce commerciale. Un altro tipo scelto con la tendina non e' il ✗: la proposta
// resta aperta, e la conferma aspetta la risposta.
func TestRiepilogoLaTendinaEIlSi(t *testing.T) {
	sc := scenaViteL1(t)
	r := sc.riepilogoOk(BozzaAlbero{Tipi: []TipoBozza{{Nodo: "cod:7121007", Tipo: db.TipoComponenteCommerciale}}})
	if len(r.Commerciali) != 1 || r.Commerciali[0].Stato != CommercialeSi || !r.Commerciali[0].Tendina || !r.Confermabile {
		t.Errorf("la tendina: %+v, blocchi %v", r.Commerciali, r.Blocchi)
	}
	if p := nuovo(r, "7121007"); p == nil || !p.Commerciale || p.Tipo != db.TipoComponenteCommerciale {
		t.Errorf("7121007 con la tendina: %+v", p)
	}
	r = sc.riepilogoOk(BozzaAlbero{Tipi: []TipoBozza{{Nodo: "cod:7121007", Tipo: db.TipoComponenteSciolto}}})
	if r.Commerciali[0].Stato != CommercialeAperta || r.Commerciali[0].Tendina || r.Confermabile {
		t.Errorf("un particolare scelto con la tendina non e' il ✗: %+v", r.Commerciali)
	}
	// il ✓ cliccato resta il ✓, senza la tendina
	r = sc.riepilogoOk(BozzaAlbero{Commerciali: []RispostaBozza{{Nodo: "cod:7121007", Risposta: RispostaSi}}})
	if r.Commerciali[0].Stato != CommercialeSi || r.Commerciali[0].Tendina {
		t.Errorf("il ✓: %+v", r.Commerciali)
	}
}

// Prova (fase 4.4a.1b, dalla verifica della 4.4a.1a): un pezzo aggiunto sotto un padre tolto nella stessa bozza si
// rifiuta, come un legame «anche sotto»: senza, andrebbe via con la cascata senza che il riepilogo lo dica. Lo stesso
// se il padre va via a cascata (7121004, tolto 7120011 che era il suo solo padre).
func TestRiepilogoUnAggiuntoSottoUnPadreCheVaVia(t *testing.T) {
	sc := scenaAlbero(t)
	aggiunto := func(padre string) AggiuntoBozza {
		return AggiuntoBozza{ID: "nuovo:1", Codice: "7129001", Tipo: db.TipoComponenteSciolto, Padre: padre, Qta: 1}
	}
	for _, c := range []struct {
		nome, frase string
		b           BozzaAlbero
	}{
		{"padre tolto", "7129001 sotto 7120012: uno dei due è tolto dall'albero",
			BozzaAlbero{Aggiunti: []AggiuntoBozza{aggiunto("cod:7120012")}, Tolti: []ToltoBozza{{Nodo: "cod:7120012"}}}},
		{"padre che va via a cascata", "7129001 sotto 7121004: 7121004 non si raggiunge più da un prodotto",
			BozzaAlbero{Aggiunti: []AggiuntoBozza{aggiunto("cod:7121004")}, Tolti: []ToltoBozza{{Nodo: "cod:7120011"}}}},
	} {
		if _, err := sc.riepilogo(c.b); err == nil || !strings.Contains(err.Error(), c.frase) {
			t.Errorf("%s: %v, atteso «%s»", c.nome, err, c.frase)
		}
	}
	// sotto un padre che resta, si'
	if r := sc.riepilogoOk(BozzaAlbero{Aggiunti: []AggiuntoBozza{aggiunto("cod:7121004")}}); nuovo(r, "7129001") == nil {
		t.Errorf("sotto 7121004 che resta: %s", codiciNuovi(r))
	}
}

// Prova (fase 4.4a.1b, dalla verifica della 4.4a.1a): un nodo che ritrova un componente ARCHIVIATO segue 6a e 6b come un
// attivo, e il riepilogo dice il suo cambio di tipo. 7121004 archiviato, un particolare, nello STEP di 7120011 ha il
// figlio 7121009: e' proposto come assieme (6b), ritrovato (la conferma lo ripristina) e il riepilogo dice «particolare →
// assieme». 7120012 archiviato e commerciale e' una foglia (6a): i pezzi che lo STEP del prodotto gli mette sotto
// restano guida, e 7121006 (che stava solo sotto di lui) non e' nell'albero.
func TestAlberoPropostoUnArchiviatoSegue6aE6b(t *testing.T) {
	sc := scenaAlbero(t)
	ieri := sc.t0
	sc.componente("7121004", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	sc.s.Componenti[len(sc.s.Componenti)-1].ArchiviatoIl = &ieri
	sc.componente("7120012", db.TipoComponenteCommerciale, db.OrigineComponenteManuale)
	sc.s.Componenti[len(sc.s.Componenti)-1].ArchiviatoIl = &ieri
	a := sc.albero()
	n := nodoDi(t, a, "7121004")
	if n.Tipo != db.TipoComponenteSottoassieme || !strings.Contains(n.TipoMotivo, "archiviato") || !strings.Contains(n.TipoMotivo, "6b") ||
		n.Ritrovato == nil || !n.Ritrovato.Archiviato {
		t.Errorf("7121004 archiviato con un figlio: %+v", n)
	}
	c := nodoDi(t, a, "7120012")
	if c.Figli != 0 || !strings.Contains(strings.Join(c.Note, " "), "particolare commerciale") {
		t.Errorf("7120012 archiviato e commerciale: figli %d, note %v", c.Figli, c.Note)
	}
	if _, ok := a.Nodo(ChiaveDelCodice("7121006")); ok {
		t.Errorf("7121006 sta solo sotto il commerciale: non e' nell'albero (%s)", righeAlbero(a))
	}
	r := sc.riepilogoOk(BozzaAlbero{})
	var tipi []string
	for _, x := range r.Tipi {
		tipi = append(tipi, x.Codice+":"+string(x.Da)+">"+string(x.A))
	}
	if strings.Join(tipi, " ") != "7121004:sciolto>sottoassieme" {
		t.Errorf("i tipi che cambiano: %v", tipi)
	}
}

// Prova (fase 4.4a.1b, dalla verifica della 4.4a.1a): una foglia senza codice con un nome da minuteria riceve la
// proposta commerciale; conta quando nella bozza prende un codice (prima la ferma il codice che manca, e il riepilogo
// non la elenca). Rinominata, la proposta e' aperta e ferma la conferma; con il ✓ il pezzo nasce commerciale.
func TestAlberoPropostoLaMinuteriaSenzaCodice(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=VITE TCEI ISO 4762 M6X16/"}, []string{"#1>#2", "#1>#3*4"})
	var vite *NodoAlbero
	a := sc.albero()
	for i := range a.Nodi {
		if a.Nodi[i].Codice == "" {
			vite = &a.Nodi[i]
		}
	}
	if vite == nil || vite.Commerciale == nil || !strings.Contains(vite.Commerciale.Motivo, "ISO 4762") {
		t.Fatalf("la vite senza codice: %+v", vite)
	}
	r := sc.riepilogoOk(BozzaAlbero{})
	if len(r.Commerciali) != 0 || len(r.SenzaCodice) != 1 || r.Confermabile {
		t.Errorf("senza codice: commerciali %+v, senza codice %v", r.Commerciali, r.SenzaCodice)
	}
	rinomina := []RinominaBozza{{Nodo: vite.Chiave, Codice: "7121007"}}
	r = sc.riepilogoOk(BozzaAlbero{Rinomine: rinomina})
	if len(r.Commerciali) != 1 || r.Commerciali[0].Stato != CommercialeAperta || r.Commerciali[0].Codice != "7121007" || r.Confermabile {
		t.Errorf("rinominata: %+v, blocchi %v", r.Commerciali, r.Blocchi)
	}
	r = sc.riepilogoOk(BozzaAlbero{Rinomine: rinomina, Commerciali: []RispostaBozza{{Nodo: vite.Chiave, Risposta: RispostaSi}}})
	if p := nuovo(r, "7121007"); p == nil || !p.Commerciale || !r.Confermabile {
		t.Errorf("rinominata con il ✓: %+v, blocchi %v", p, r.Blocchi)
	}
}

// Prova (risposta 28: una revisione aggiorna lo stesso pezzo): nella distinta 7121003 ha la rev A; le righe ancora
// aperte dello STEP di 7120010 dicono B. Il riepilogo dice «7121003: A → B», e il pezzo non e' nuovo. Con due file che
// dicono B e C non decide nessuno: la revisione resta, e il riepilogo dice le due. Le righe gia' decise non contano.
func TestRiepilogoLaRevisioneNuovaDiUnComponenteCheCe(t *testing.T) {
	riga := func(sc *scena, file, chiave, rev string) {
		sha := sc.f(file).Sha
		for i, n := range sc.s.Nodi {
			if n.Sha256 == sha && n.Chiave == chiave {
				sc.s.Nodi[i].Rev = pgtype.Text{String: rev, Valid: true}
				return
			}
		}
		sc.t.Fatalf("nessuna riga %s in %s", chiave, file)
	}
	sc, c := scenaConfermata(t)
	for i := range sc.s.Componenti {
		if sc.s.Componenti[i].ComponenteID == c["7121003"].ComponenteID {
			sc.s.Componenti[i].Rev = pgtype.Text{String: "A", Valid: true}
		}
	}
	riga(sc, "7120010.stp", "#4", "b")
	r := sc.riepilogoOk(BozzaAlbero{})
	if len(r.Revisioni) != 1 || r.Revisioni[0].Codice != "7121003" || r.Revisioni[0].Da != "A" || r.Revisioni[0].A != "B" ||
		r.Revisioni[0].Componente != c["7121003"].ComponenteID || nuovo(r, "7121003") != nil {
		t.Errorf("la revisione nuova: %+v", r.Revisioni)
	}
	riga(sc, "7120011.stp", "#2", "C")
	r = sc.riepilogoOk(BozzaAlbero{})
	if len(r.Revisioni) != 1 || r.Revisioni[0].A != "" || strings.Join(r.Revisioni[0].Discordi, " ") != "B C" {
		t.Errorf("due revisioni diverse: %+v", r.Revisioni)
	}
	// una riga decisa da una persona non conta: resta solo la B
	sc.accetta("7120011.stp", "#2", c["7121003"])
	if r = sc.riepilogoOk(BozzaAlbero{}); len(r.Revisioni) != 1 || r.Revisioni[0].A != "B" {
		t.Errorf("con la riga C decisa: %+v", r.Revisioni)
	}
}

// Prova (studio § 2.9, F7, pura): una riga decisa da una persona fuori dall'autorita' di un file autorizzato non basta
// a togliere «da rivedere» a un componente nato dallo STEP; con il segno dell'albero si'. Lo stesso per un arco: con
// il segno (e i due componenti che dice) e' deciso.
func TestIlSegnoDellAlberoPerF7(t *testing.T) {
	comp := db.Componente{ComponenteID: idDa("componente:7121003"), Codice: "7121003", Tipo: db.TipoComponenteSciolto, Origine: db.OrigineComponenteStep}
	riga := db.ComponenteProposta{PropostaID: idDa("riga"), AllegatoID: idDa("allegato"), Chiave: "#4", Stato: db.StatoPropostaConfermata,
		ComponenteID: uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}, DecisoDa: uuid.NullUUID{UUID: idDa("persona"), Valid: true},
		Evidenza: json.RawMessage(`{}`)}
	aut := CalcolaAutorita(ValutaDichiarazioni(nil), nil, nil)
	if et := Provenienza(ProvenienzaDi{Componente: comp, Righe: []RigaDelComponente{{Proposta: riga}}}, aut); strings.Join(et, ",") != ProvenienzaStepNonAutorizzato {
		t.Errorf("senza il segno: %v", et)
	}
	riga.Evidenza = json.RawMessage(`{"albero": {"da": "` + idDa("persona").String() + `", "firma": "f"}}`)
	if et := Provenienza(ProvenienzaDi{Componente: comp, Righe: []RigaDelComponente{{Proposta: riga}}}, aut); len(et) != 0 || !ConfermatoNellAlbero(riga) {
		t.Errorf("con il segno: %v", et)
	}
	// il segno su una riga che nessuna persona ha deciso non vale
	aperta := riga
	aperta.Stato, aperta.DecisoDa = db.StatoPropostaAperta, uuid.NullUUID{}
	if ConfermatoNellAlbero(aperta) {
		t.Error("una riga aperta con il segno non e' confermata nell'albero")
	}

	padre, figlio := idDa("componente:7120010"), comp.ComponenteID
	arco := db.RelazioneProposta{AllegatoID: idDa("allegato"), PadreChiave: "#1", FiglioChiave: "#4", Stato: db.StatoPropostaConfermata,
		DecisoDa: uuid.NullUUID{UUID: idDa("persona"), Valid: true},
		Evidenza: json.RawMessage(`{"albero": {"padre": "` + padre.String() + `", "figlio": "` + figlio.String() + `"}}`)}
	decisi := ArchiDecisiNellAutorita(aut, nil, []db.RelazioneProposta{arco})
	r := db.ComponenteRelazione{PadreID: padre, FiglioID: figlio, Origine: db.OrigineComponenteStep}
	if !decisi[CoppiaDiComponenti{padre, figlio}] || ArcoDaRivedereDi(r, decisi) {
		t.Errorf("l'arco con il segno: %v", decisi)
	}
	arco.Evidenza = json.RawMessage(`{}`)
	if decisi := ArchiDecisiNellAutorita(aut, nil, []db.RelazioneProposta{arco}); !ArcoDaRivedereDi(r, decisi) {
		t.Error("l'arco senza il segno, fuori dall'autorita': da rivedere")
	}
}

// Il corpo della conferma: la firma e la bozza, nient'altro; senza la firma si rifiuta; la bozza con il suo formato.
func TestLeggiConferma(t *testing.T) {
	b, firma, err := LeggiConferma([]byte(`{"firma": " abc ", "bozza": {"formato": 1, "base": "x", "tolti": [{"nodo": "cod:7121001"}]}}`))
	if err != nil || firma != "abc" || b.Base != "x" || len(b.Tolti) != 1 {
		t.Errorf("il corpo: %+v %q %v", b, firma, err)
	}
	for _, c := range []struct{ raw, frase string }{
		{`{"bozza": {"formato": 1}}`, "senza la firma"},
		{`{"firma": "abc", "bozza": {"formato": 2}}`, "formato 2"},
		{`{"firma": "abc", "bozza": {"formato": 1}, "altro": 1}`, "non si legge"},
		{`non e' json`, "non si legge"},
	} {
		if _, _, err := LeggiConferma([]byte(c.raw)); err == nil || !strings.Contains(err.Error(), c.frase) {
			t.Errorf("%s: %v", c.raw, err)
		}
	}
}

// Lo scenario MG (fase 4.3, dati fittizi, i prodotti come dice il cartiglio): il piano della conferma
// della bozza vuota fa un pezzo solo per un figlio in comune, con un legame per padre e la quantita' di ciascuno:
// 7120111 sotto 7120100 e 7120101, 7120133 ×2 sotto 7120103 e 7120104. E' quello che la conferma scrive (un componente,
// due archi), con i conti del riepilogo.
func TestLaConfermaDelloScenarioMGUnPezzoPerIlFiglioInComune(t *testing.T) {
	sc := scenaMGCon(t, "7120100", "7120103", "7120101", "7120104")
	a := sc.albero()
	r, pc, err := riepiloga(a, BozzaAlbero{Formato: FormatoBozza, Base: a.Firma}, sc.contesto())
	if err != nil {
		t.Fatal(err)
	}
	legami := func(figlio string) string {
		var out []string
		for _, l := range pc.legami {
			if l.figlio.codice == figlio {
				out = append(out, fmt.Sprintf("%s×%d", l.padre.codice, l.qta))
			}
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	gruppi := 0
	for _, g := range pc.gruppi {
		if g.codice == "7120111" || g.codice == "7120133" {
			gruppi++
		}
	}
	if gruppi != 2 || legami("7120111") != "7120100×1 7120101×1" || legami("7120133") != "7120103×2 7120104×2" {
		t.Errorf("i figli in comune: %d gruppi, 7120111 %s, 7120133 %s", gruppi, legami("7120111"), legami("7120133"))
	}
	if p := nuovo(r, "7120111"); p == nil || strings.Join(p.Padri, " ") != "7120100 7120101" {
		t.Errorf("7120111 nel riepilogo: %+v", p)
	}
}

// Prova (P13, U5; fase 4.4a.1b, dalla verifica): un aggancio per codice di prima dello Smistamento (la riga di 7121002
// nello STEP di 7120010: duplicato con il componente, senza chi l'ha deciso) e' una riga ancora da decidere. L'albero
// lo ritrova, e lo dice; il riepilogo lo elenca fra i ritrovati con il perche'; la rinomina del nodo si rifiuta (la
// rinomina scrive il codice solo sulle righe aperte, e l'aggancio resterebbe al componente di prima). Confermato da
// una persona (ConfermaNodoAgganciato) non e' piu' da decidere, e la firma dell'albero lo vede anche quando nient'altro
// cambia: 7121002 nella working sotto il prodotto e con una riga aperta in un altro file resta nella distinta e
// ritrovato, e cambia solo l'aggancio.
func TestAlberoPropostoLAggancioPerCodiceDiPrimaEDaDecidere(t *testing.T) {
	sc := scenaAlbero(t)
	c02 := sc.componente("7121002", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	sha := sc.f("7120010.stp").Sha
	i := -1
	for j, n := range sc.s.Nodi {
		if n.Sha256 == sha && n.Chiave == "#3" {
			i = j
		}
	}
	if i < 0 {
		t.Fatal("la riga di 7121002 nello STEP di 7120010 non c'e'")
	}
	sc.s.Nodi[i].Stato, sc.s.Nodi[i].ComponenteID = db.StatoPropostaDuplicato, uuid.NullUUID{UUID: c02.ComponenteID, Valid: true}
	a := sc.albero()
	n, ok := a.Nodo("cod:7121002")
	if !ok || n.Ritrovato == nil || !n.Ritrovato.Agganciato || n.Ritrovato.Componente != c02.ComponenteID || len(n.Righe) != 1 || !n.Righe[0].Agganciata ||
		n.Stato != StatoAlberoProposto {
		t.Fatalf("il nodo agganciato: %+v", n)
	}
	r := sc.riepilogoOk(BozzaAlbero{})
	if len(r.Ritrovati) != 1 || r.Ritrovati[0].Codice != "7121002" || !strings.Contains(r.Ritrovati[0].Perche, "agganciato per codice prima dello Smistamento") ||
		nuovo(r, "7121002") != nil {
		t.Errorf("i ritrovati: %+v; nuovi %s", r.Ritrovati, codiciNuovi(r))
	}
	if _, err := sc.riepilogo(BozzaAlbero{Rinomine: []RinominaBozza{{Nodo: "cod:7121002", Codice: "7129002"}}}); err == nil ||
		!strings.Contains(err.Error(), "agganciato per codice") {
		t.Errorf("la rinomina dell'agganciato: %v", err)
	}
	// confermato da una persona non e' piu' da decidere; con un'altra riga aperta e un arco della working cambia solo
	// l'aggancio, e la firma lo vede
	sc.step("7120012.stp", []string{"#1=7120012", "#2=7121002"}, []string{"#1>#2"})
	sc.arco(sc.s.Componenti[0], c02, 1)
	a = sc.albero()
	if n, _ := a.Nodo("cod:7121002"); n.Ritrovato == nil || !n.Ritrovato.Agganciato || n.Stato != StatoAlberoNellaDistinta {
		t.Fatalf("con la riga aperta e l'arco della working: %+v", n)
	}
	sc.s.Nodi[i].DecisoDa = uuid.NullUUID{UUID: idDa("persona"), Valid: true}
	b := sc.albero()
	m, _ := b.Nodo("cod:7121002")
	agganciate := 0
	for _, x := range m.Righe {
		if x.Agganciata {
			agganciate++
		}
	}
	if m.Ritrovato == nil || m.Ritrovato.Agganciato || agganciate != 0 || m.Stato != StatoAlberoNellaDistinta || b.Firma == a.Firma {
		t.Errorf("l'aggancio confermato: %+v, firma cambiata %v", m, b.Firma != a.Firma)
	}
}

// Prova (fase 4.4a.1b, dalla verifica): la firma del riepilogo porta l'effetto dei cambi di tipo sui componenti che
// ci sono (TipoRiepilogo.Effetti e Spento, che LeggiRiepilogo calcola): lo stesso effetto, la stessa firma;
// un'autorizzazione che si sospende, o un cambio spento, un'altra firma. Il resto (l'albero, la bozza) uguale.
func TestLaFirmaDelRiepilogoPortaLEffettoDeiTipi(t *testing.T) {
	tipi := func(effetti []string, spento string) []TipoRiepilogo {
		return []TipoRiepilogo{{Nodo: "cod:7120010", Codice: "7120010", Da: db.TipoComponenteSottoassieme, A: db.TipoComponenteCommerciale,
			Motivo: "scelto nella bozza", Effetti: effetti, Spento: spento}}
	}
	b := BozzaAlbero{Formato: FormatoBozza, Base: "albero"}
	const sospensione = "si sospende l'autorizzazione di 7120010.stp per 7120010"
	senza := firmaRiepilogo("albero", b, nil, tipi(nil, ""))
	sospende := firmaRiepilogo("albero", b, nil, tipi([]string{sospensione}, ""))
	spento := firmaRiepilogo("albero", b, nil, tipi(nil, "la BOM è congelata nella V1"))
	if senza == sospende || senza == spento || sospende == spento {
		t.Errorf("le firme: %s %s %s", senza, sospende, spento)
	}
	if ancora := firmaRiepilogo("albero", b, nil, tipi([]string{sospensione}, "")); ancora != sospende {
		t.Errorf("lo stesso effetto, due firme: %s %s", ancora, sospende)
	}
}
