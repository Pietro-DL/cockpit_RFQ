package fascicolo

// L1 — il riepilogo dell'albero proposto e la bozza dell'operatore (giro 4, fase 4.4a.1a; domande 27 = A, 28 = A, 29b = A, 30
// seconda risposta, P4, P13, 6b, 6c; studio docs/specs/studio_albero_distinta_29-09.md § 2.5 e § 2.8): scene ACME
// inventate.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// contesto e' il contesto del riepilogo sulla scena, come lo legge LeggiRiepilogo (senza storia: la si mette a mano).
func (sc *scena) contesto() ContestoRiepilogo {
	cx := ContestoRiepilogo{Motore: sc.s.Motore, Componenti: sc.s.Componenti, Relazioni: sc.s.Relazioni, Storia: map[uuid.UUID][]string{}}
	for _, i := range sc.s.Identificativi {
		cx.Richiesta = append(cx.Richiesta, i.Codice)
	}
	return cx
}

// riepilogo e' il riepilogo della bozza sulla scena; la base e' l'albero di adesso se la bozza non ne ha una.
func (sc *scena) riepilogo(b BozzaAlbero) (RiepilogoAlbero, error) {
	a := sc.albero()
	if b.Formato == 0 {
		b.Formato = FormatoBozza
	}
	if b.Base == "" {
		b.Base = a.Firma
	}
	return Riepilogo(a, b, sc.contesto())
}

func (sc *scena) riepilogoOk(b BozzaAlbero) RiepilogoAlbero {
	sc.t.Helper()
	r, err := sc.riepilogo(b)
	if err != nil {
		sc.t.Fatalf("il riepilogo: %v", err)
	}
	return r
}

func codiciNuovi(r RiepilogoAlbero) string {
	var out []string
	for _, p := range r.Nuovi {
		out = append(out, p.Codice)
	}
	return strings.Join(out, " ")
}

func righeLegami(ll []LegameRiepilogo) string {
	var out []string
	for _, l := range ll {
		x := fmt.Sprintf("%s → %s ×%d", l.Padre, l.Figlio, l.Qta)
		if l.Motivo != "" {
			x += " (" + l.Motivo + ")"
		}
		out = append(out, x)
	}
	return strings.Join(out, "; ")
}

// Prova (studio § 2.8): la bozza vuota sull'albero del primo giro (nella working c'e' solo il prodotto) dice che
// nascono tutti i pezzi proposti, con il codice e la fonte (lo STEP), e tutti i legami, riga per riga, con i file.
// Niente si ritrova, niente cambia tipo, niente si toglie: e' confermabile.
func TestRiepilogoLaBozzaVuota(t *testing.T) {
	r := scenaAlbero(t).riepilogoOk(BozzaAlbero{})
	if got := codiciNuovi(r); got != "7120010 7120011 7120012 7121001 7121002 7121003 7121004 7121006 7121009" {
		t.Errorf("i pezzi nuovi: %s", got)
	}
	for _, p := range r.Nuovi {
		if p.Fonte != FonteCodiceStep {
			t.Errorf("%s: fonte %q", p.Codice, p.Fonte)
		}
	}
	if n := nuovo(r, "7121003"); n == nil || strings.Join(n.Padri, " ") != "7120010 7120011 7120012" || n.Tipo != db.TipoComponenteSciolto {
		t.Errorf("7121003: %+v", n)
	}
	if len(r.LegamiNuovi) != 11 || len(r.LegamiTolti) != 0 || len(r.Ritrovati) != 0 || len(r.Tipi) != 0 || len(r.Cascata) != 0 {
		t.Errorf("legami nuovi %d, tolti %d, ritrovati %d, tipi %d, cascata %d", len(r.LegamiNuovi), len(r.LegamiTolti), len(r.Ritrovati),
			len(r.Tipi), len(r.Cascata))
	}
	for _, l := range r.LegamiNuovi {
		if l.Padre == "7120010" && l.Figlio == "7121003" && (l.Qta != 2 || strings.Join(l.File, ",") != "7120010.stp") {
			t.Errorf("7120010 → 7121003: %+v", l)
		}
	}
	if !r.Confermabile || len(r.Blocchi) != 0 || r.Vecchia || r.Firma == "" || r.AlberoFirma == "" {
		t.Errorf("confermabile %v, blocchi %v, vecchia %v, firma %q", r.Confermabile, r.Blocchi, r.Vecchia, r.Firma)
	}
}

func nuovo(r RiepilogoAlbero, codice string) *PezzoRiepilogo {
	for i := range r.Nuovi {
		if r.Nuovi[i].Codice == codice {
			return &r.Nuovi[i]
		}
	}
	return nil
}

// scenaConfermata e' scenaAlbero con l'albero gia' nella working (una conferma di prima): tutti i pezzi e i legami ci
// sono, e le righe degli STEP sono ancora aperte (le ritrova per codice).
func scenaConfermata(t *testing.T) (*scena, map[string]db.Componente) {
	sc := scenaAlbero(t)
	c := map[string]db.Componente{"7120001": sc.s.Componenti[0]}
	for _, x := range []struct {
		codice string
		tipo   db.TipoComponente
	}{{"7120010", db.TipoComponenteSottoassieme}, {"7120011", db.TipoComponenteSottoassieme}, {"7120012", db.TipoComponenteSottoassieme},
		{"7121001", db.TipoComponenteSciolto}, {"7121002", db.TipoComponenteSciolto}, {"7121003", db.TipoComponenteSciolto},
		{"7121004", db.TipoComponenteSottoassieme}, {"7121006", db.TipoComponenteSciolto}, {"7121009", db.TipoComponenteSciolto}} {
		c[x.codice] = sc.componente(x.codice, x.tipo, db.OrigineComponenteManuale)
	}
	for _, a := range []struct {
		p, f string
		q    int32
	}{{"7120001", "7120010", 1}, {"7120001", "7120011", 2}, {"7120001", "7120012", 1}, {"7120010", "7121001", 1}, {"7120010", "7121002", 1},
		{"7120010", "7121003", 2}, {"7120011", "7121003", 1}, {"7120011", "7121004", 2}, {"7121004", "7121009", 3}, {"7120012", "7121003", 1},
		{"7120012", "7121006", 1}} {
		sc.arco(c[a.p], c[a.f], a.q)
	}
	return sc, c
}

// Prova (29b = A): cancellare un assieme con un figlio in comune. Tolto 7120011: vanno via 7120011, 7121004 e 7121009
// (solo suoi), con i loro legami; 7121003 resta, sotto 7120010 e 7120012, e perde solo il legame da 7120011. Un
// componente che resta senza padri si archivia se ha documenti o storia (7121004 ha dei documenti), si elimina se no
// (7120011, 7121009): il riepilogo lo dice pezzo per pezzo.
func TestRiepilogoLaCascataConUnFiglioInComune(t *testing.T) {
	sc, c := scenaConfermata(t)
	cx := sc.contesto()
	cx.Storia[c["7121004"].ComponenteID] = []string{"ha dei documenti"}
	a := sc.albero()
	r, err := Riepilogo(a, BozzaAlbero{Formato: FormatoBozza, Base: a.Firma, Tolti: []ToltoBozza{{Nodo: "cod:7120011"}}}, cx)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cascata) != 1 || strings.Join(r.Cascata[0].Vanno, " ") != "7120011 7121004 7121009" || len(r.Cascata[0].Restano) != 1 ||
		r.Cascata[0].Restano[0].Codice != "7121003" || strings.Join(r.Cascata[0].Restano[0].Padri, " ") != "7120010 7120012" {
		t.Errorf("la cascata: %+v", r.Cascata)
	}
	atteso := "7120001 → 7120011 ×2 (7120011 è tolto dall'albero); 7120011 → 7121003 ×1 (7120011 è tolto dall'albero); " +
		"7120011 → 7121004 ×2 (7120011 è tolto dall'albero); 7121004 → 7121009 ×3 (a cascata: 7121004 non si raggiunge più da un prodotto)"
	if got := righeLegami(r.LegamiTolti); got != atteso {
		t.Errorf("i legami tolti:\n%s\natteso:\n%s", got, atteso)
	}
	var fuori []string
	for _, f := range r.Fuori {
		fuori = append(fuori, f.Codice+":"+f.Esito)
	}
	if strings.Join(fuori, " ") != "7120011:si_elimina 7121004:si_archivia 7121009:si_elimina" {
		t.Errorf("i componenti senza padri: %v", fuori)
	}
	if len(r.Nuovi) != 0 || len(r.LegamiNuovi) != 0 || !r.Confermabile {
		t.Errorf("nuovi %v, legami nuovi %v, confermabile %v %v", r.Nuovi, r.LegamiNuovi, r.Confermabile, r.Blocchi)
	}
}

// «Togli da qui»: togliere un legame solo. 7121003 tolto da sotto 7120010 resta sotto gli altri due, e niente va via;
// 7121006 tolto da sotto 7120012, il suo solo padre, va via, e resta senza padri. Il legame deve esserci.
func TestRiepilogoToglieUnLegameSolo(t *testing.T) {
	sc, _ := scenaConfermata(t)
	r := sc.riepilogoOk(BozzaAlbero{Tolti: []ToltoBozza{{Nodo: "cod:7121003", Padre: "cod:7120010"}, {Nodo: "cod:7121006", Padre: "cod:7120012"}}})
	if len(r.Cascata) != 2 || len(r.Cascata[0].Vanno) != 0 || strings.Join(r.Cascata[0].Restano[0].Padri, " ") != "7120011 7120012" ||
		strings.Join(r.Cascata[1].Vanno, " ") != "7121006" || r.Cascata[1].Padre != "7120012" {
		t.Errorf("la cascata: %+v", r.Cascata)
	}
	if got := righeLegami(r.LegamiTolti); got != "7120010 → 7121003 ×2 (tolto nella bozza); 7120012 → 7121006 ×1 (tolto nella bozza)" {
		t.Errorf("i legami tolti: %s", got)
	}
	if len(r.Fuori) != 1 || r.Fuori[0].Codice != "7121006" {
		t.Errorf("fuori: %+v", r.Fuori)
	}
	if _, err := sc.riepilogo(BozzaAlbero{Tolti: []ToltoBozza{{Nodo: "cod:7121006", Padre: "cod:7120010"}}}); err == nil ||
		!strings.Contains(err.Error(), "non sta sotto") {
		t.Errorf("un legame che non c'e': %v", err)
	}
}

// La bozza del primo giro, con tutto: una rinomina (il nodo senza codice prende il suo), un pezzo aggiunto con il
// padre, un tipo scelto, il ✓ di una proposta commerciale, il «diverso» di un pezzo con un vicino. Senza le risposte le
// domande aperte fermano la conferma; con le risposte il riepilogo e' confermabile e dice le fonti.
func TestRiepilogoLaBozzaDelPrimoGiro(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120012", "#3=VITE TCEI ISO 4762 M6X16/7121007", "#4=7121001", "#5=TELAIO/", "#6=7121005",
		"#7=7121003"}, []string{"#1>#2", "#1>#3*4", "#1>#4", "#1>#5", "#5>#6", "#2>#7"})
	sc.componente("7121001A", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	vuota := sc.riepilogoOk(BozzaAlbero{})
	if vuota.Confermabile || len(vuota.Blocchi) != 3 || len(vuota.SenzaCodice) != 1 || len(vuota.Commerciali) != 1 ||
		vuota.Commerciali[0].Stato != CommercialeAperta || len(vuota.Vicini) != 1 || vuota.Vicini[0].Diverso {
		t.Fatalf("la bozza vuota: blocchi %v, senza codice %v, commerciali %+v, vicini %+v", vuota.Blocchi, vuota.SenzaCodice, vuota.Commerciali, vuota.Vicini)
	}
	telaio := ""
	for _, n := range sc.albero().Nodi {
		if n.Codice == "" {
			telaio = n.Chiave
		}
	}
	b := BozzaAlbero{
		Rinomine:    []RinominaBozza{{Nodo: telaio, Codice: "7120020", Rev: "a"}},
		Aggiunti:    []AggiuntoBozza{{ID: "nuovo:1", Codice: "7129001", Tipo: db.TipoComponenteSciolto, Padre: "cod:7120012", Qta: 2}},
		Tipi:        []TipoBozza{{Nodo: "cod:7121001", Tipo: db.TipoComponenteSottoassieme}},
		Commerciali: []RispostaBozza{{Nodo: "cod:7121007", Risposta: RispostaSi}},
		Diversi:     []string{"cod:7121001"},
	}
	r := sc.riepilogoOk(b)
	if !r.Confermabile || len(r.Blocchi) != 0 {
		t.Fatalf("con le risposte: blocchi %v", r.Blocchi)
	}
	if got := codiciNuovi(r); got != "7120012 7121001 7121003 7121005 7121007 7120020 7129001" {
		t.Errorf("i pezzi nuovi: %s", got)
	}
	if p := nuovo(r, "7120020"); p == nil || p.Fonte != "scritto" || p.Rev != "A" || p.Tipo != db.TipoComponenteSottoassieme {
		t.Errorf("7120020, rinominato: %+v", p)
	}
	if p := nuovo(r, "7129001"); p == nil || p.Fonte != "scritto" || strings.Join(p.Padri, " ") != "7120012" {
		t.Errorf("7129001, aggiunto: %+v", p)
	}
	if p := nuovo(r, "7121007"); p == nil || !p.Commerciale || p.Tipo != db.TipoComponenteSciolto {
		t.Errorf("7121007, ✓: %+v", p)
	}
	if len(r.Commerciali) != 1 || r.Commerciali[0].Stato != CommercialeSi || r.Commerciali[0].Motivo != "normato: ISO 4762 · M6X16 · TCEI" {
		t.Errorf("le proposte commerciali: %+v", r.Commerciali)
	}
	if len(r.Tipi) != 1 || r.Tipi[0].Codice != "7121001" || r.Tipi[0].Da != db.TipoComponenteSciolto || r.Tipi[0].A != db.TipoComponenteSottoassieme {
		t.Errorf("i tipi: %+v", r.Tipi)
	}
	if len(r.Vicini) != 1 || !r.Vicini[0].Diverso || r.Vicini[0].Vicini[0].Codice != "7121001A" {
		t.Errorf("i vicini: %+v", r.Vicini)
	}
	if !strings.Contains(righeLegami(r.LegamiNuovi), "7120012 → 7129001 ×2 (scritto nella bozza)") ||
		!strings.Contains(righeLegami(r.LegamiNuovi), "7120020 → 7121005 ×1") {
		t.Errorf("i legami nuovi: %s", righeLegami(r.LegamiNuovi))
	}
	// il ✗: la proposta ha la sua risposta, il pezzo nasce particolare
	b.Commerciali[0].Risposta = RispostaNo
	if r := sc.riepilogoOk(b); r.Commerciali[0].Stato != CommercialeNo || nuovo(r, "7121007").Commerciale || !r.Confermabile {
		t.Errorf("il ✗: %+v", r.Commerciali)
	}
}

// P13: il riepilogo dice i pezzi legati a un componente che c'e' per lo stesso codice, uno per riga, con il perche'; un
// archiviato si ripristina. Non sono pezzi nuovi.
func TestRiepilogoIRitrovati(t *testing.T) {
	sc := scenaAlbero(t)
	c02 := sc.componente("7121002", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	sc.componente("7121006", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	ieri := sc.t0
	sc.s.Componenti[len(sc.s.Componenti)-1].ArchiviatoIl = &ieri
	r := sc.riepilogoOk(BozzaAlbero{})
	if len(r.Ritrovati) != 2 || r.Ritrovati[0].Codice != "7121002" || r.Ritrovati[0].Componente != c02.ComponenteID || r.Ritrovati[0].Archiviato ||
		r.Ritrovati[0].Perche != "le righe dello STEP hanno il suo codice" || r.Ritrovati[1].Codice != "7121006" || !r.Ritrovati[1].Archiviato {
		t.Errorf("i ritrovati: %+v", r.Ritrovati)
	}
	if nuovo(r, "7121002") != nil || nuovo(r, "7121006") != nil {
		t.Errorf("un ritrovato non e' un pezzo nuovo: %s", codiciNuovi(r))
	}
	// un pezzo aggiunto con il codice di un componente che c'e' fuori dall'albero e' quel componente, non uno nuovo
	c99 := sc.componente("7129500", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	r = sc.riepilogoOk(BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "nuovo:1", Codice: "7129500", Tipo: db.TipoComponenteSciolto, Padre: "cod:7120011", Qta: 1}}})
	if len(r.Ritrovati) != 3 || r.Ritrovati[2].Componente != c99.ComponenteID || r.Ritrovati[2].Nodo != "nuovo:1" ||
		!strings.Contains(r.Ritrovati[2].Perche, "scritto nella bozza") || nuovo(r, "7129500") != nil {
		t.Errorf("l'aggiunto con il codice di un componente che c'e': %+v", r.Ritrovati)
	}
}

// 6b nel riepilogo: il particolare che c'e' e a cui lo STEP mette sotto dei pezzi diventa assieme, e il riepilogo lo
// elenca fra i tipi che cambiano, con il motivo.
func TestRiepilogoIlParticolareCheDiventaAssieme(t *testing.T) {
	sc := scenaAlbero(t)
	p := sc.s.Componenti[0]
	a11 := sc.componente("7120011", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale)
	p04 := sc.componente("7121004", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	sc.arco(p, a11, 2)
	sc.arco(a11, p04, 2)
	r := sc.riepilogoOk(BozzaAlbero{})
	if len(r.Tipi) != 1 || r.Tipi[0].Codice != "7121004" || r.Tipi[0].Da != db.TipoComponenteSciolto || r.Tipi[0].A != db.TipoComponenteSottoassieme ||
		!strings.Contains(r.Tipi[0].Motivo, "6b") {
		t.Errorf("i tipi che cambiano: %+v", r.Tipi)
	}
	// e la bozza non lo puo' rimettere particolare con i figli sotto
	if _, err := sc.riepilogo(BozzaAlbero{Tipi: []TipoBozza{{Nodo: "cod:7121004", Tipo: db.TipoComponenteSciolto}}}); err == nil ||
		!strings.Contains(err.Error(), "sotto non ci va niente") {
		t.Errorf("un particolare con i figli: %v", err)
	}
}

// Le rimozioni proposte dallo STEP: con la bozza vuota il legame resta («tenuta»: la conferma chiudera' la proposta);
// togliendolo nella bozza la rimozione e' accettata, e il legame tolto lo dice.
func TestRiepilogoLeRimozioniDelloStep(t *testing.T) {
	sc, c := scenaConfermata(t)
	step := idDa("documento:7120012.stp")
	a := func() AlberoProposto {
		sc.s.der = nil
		p := ProdottiDellaRfq(&sc.s)
		anc := Ancore(p, &sc.s)
		return NuovoAlberoProposto(&sc.s, p, anc, IndiceCodici(p, anc, &sc.s),
			[]db.RimozioneProposta{{StepDocumentoID: step, PadreID: c["7120012"].ComponenteID, FiglioID: c["7121006"].ComponenteID, QtaWorking: 1}})
	}()
	var tolto *ArcoAlbero
	for i := range a.Archi {
		if a.Archi[i].Padre == "cod:7120012" && a.Archi[i].Figlio == "cod:7121006" {
			tolto = &a.Archi[i]
		}
	}
	if tolto == nil || tolto.Stato != StatoAlberoTolto || tolto.Rimozione == nil || tolto.Rimozione.Step != step {
		t.Fatalf("il legame con la rimozione: %+v", tolto)
	}
	if n, _ := a.Nodo("cod:7121006"); n.Stato != StatoAlberoTolto {
		t.Errorf("7121006, tutto tolto dallo STEP: %s", n.Stato)
	}
	r, err := Riepilogo(a, BozzaAlbero{Formato: FormatoBozza, Base: a.Firma}, sc.contesto())
	if err != nil || len(r.Rimozioni) != 1 || r.Rimozioni[0].Stato != RimozioneTenuta || len(r.LegamiTolti) != 0 {
		t.Fatalf("la bozza vuota: %v %+v %v", err, r.Rimozioni, r.LegamiTolti)
	}
	r, err = Riepilogo(a, BozzaAlbero{Formato: FormatoBozza, Base: a.Firma, Tolti: []ToltoBozza{{Nodo: "cod:7121006", Padre: "cod:7120012"}}}, sc.contesto())
	if err != nil || r.Rimozioni[0].Stato != RimozioneTolta || righeLegami(r.LegamiTolti) != "7120012 → 7121006 ×1 (tolto nella bozza: la rimozione proposta dallo STEP è accettata)" {
		t.Errorf("la rimozione accettata: %v %+v %s", err, r.Rimozioni, righeLegami(r.LegamiTolti))
	}
}

// 6c nel riepilogo: una quantita' discorde fra due file ferma la conferma finche' la bozza non ne sceglie una; la
// scelta la dice. Le discordi con il disegno arrivano con la fase 4.12.
func TestRiepilogoLeQuantitaDiscordi(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7121003", "#4=7121005"}, []string{"#1>#2", "#2>#3*2", "#2>#4"})
	sc.step("7120010.stp", []string{"#1=7120010", "#2=7121003"}, []string{"#1>#2*3"})
	r := sc.riepilogoOk(BozzaAlbero{})
	if len(r.QuantitaDiscordi) != 1 || r.QuantitaDiscordi[0].Scelta != 0 || len(r.QuantitaDiscordi[0].Fonti) != 2 || r.Confermabile {
		t.Fatalf("senza scelta: %+v, blocchi %v", r.QuantitaDiscordi, r.Blocchi)
	}
	r = sc.riepilogoOk(BozzaAlbero{Quantita: []LegameBozza{{Padre: "cod:7120010", Figlio: "cod:7121003", Qta: 3}}})
	if len(r.QuantitaDiscordi) != 1 || r.QuantitaDiscordi[0].Scelta != 3 || !r.Confermabile || !strings.Contains(righeLegami(r.LegamiNuovi), "7120010 → 7121003 ×3") {
		t.Errorf("con la scelta: %+v, blocchi %v, legami %s", r.QuantitaDiscordi, r.Blocchi, righeLegami(r.LegamiNuovi))
	}
}

// «E' lo stesso pezzo»: rinominare un nodo con il codice di un altro nodo dell'albero li fa uno. Due figli diversi
// sotto lo stesso nodo diventano due posizioni dello stesso pezzo, e le quantita' si sommano; la proposta commerciale
// di un nodo che si unisce a un altro decade.
func TestRiepilogoLaRinominaUnisceDueNodi(t *testing.T) {
	sc := scenaAlbero(t)
	r := sc.riepilogoOk(BozzaAlbero{Rinomine: []RinominaBozza{{Nodo: "cod:7121002", Codice: "7121001"}}})
	if nuovo(r, "7121002") != nil || !strings.Contains(righeLegami(r.LegamiNuovi), "7120010 → 7121001 ×2") {
		t.Errorf("l'unione: nuovi %s, legami %s", codiciNuovi(r), righeLegami(r.LegamiNuovi))
	}
	// una foglia rinominata con il codice di un assieme che c'e' (7121006 e' 7120010, gia' nella distinta): e' quel
	// componente, ritrovato per la rinomina, con il suo tipo e i suoi figli; ora sta anche sotto 7120012
	sc2 := scenaAlbero(t)
	a10 := sc2.componente("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale)
	sc2.arco(sc2.s.Componenti[0], a10, 1)
	r = sc2.riepilogoOk(BozzaAlbero{Rinomine: []RinominaBozza{{Nodo: "cod:7121006", Codice: "7120010"}}})
	var rinominato *RitrovatoRiepilogo
	for i := range r.Ritrovati {
		if r.Ritrovati[i].Nodo == "cod:7121006" {
			rinominato = &r.Ritrovati[i]
		}
	}
	if rinominato == nil || rinominato.Componente != a10.ComponenteID || !strings.Contains(rinominato.Perche, "rinominato") ||
		len(r.Tipi) != 0 || !strings.Contains(righeLegami(r.LegamiNuovi), "7120012 → 7120010 ×1") || nuovo(r, "7121006") != nil {
		t.Errorf("la foglia rinominata come un assieme che c'e': ritrovati %+v, tipi %+v, legami %s", r.Ritrovati, r.Tipi, righeLegami(r.LegamiNuovi))
	}
	// un nodo che si rinomina con il codice del padre starebbe sotto se stesso
	if _, err := sc.riepilogo(BozzaAlbero{Rinomine: []RinominaBozza{{Nodo: "cod:7121002", Codice: "7120010"}}}); err == nil ||
		!strings.Contains(err.Error(), "sotto se stesso") {
		t.Errorf("sotto se stesso: %v", err)
	}
}

// La bozza si legge o si rifiuta con le parole giuste: il formato, i riferimenti, i prodotti, i codici, i tipi, i
// pezzi sotto un particolare (6b), i cicli, le risposte, i doppioni, i limiti. Un rifiuto e' un Rifiuto (la pagina lo
// mostra), mai un errore del programma.
func TestRiepilogoLaBozzaSiRifiuta(t *testing.T) {
	sc, _ := scenaConfermata(t)
	nuovaSc := scenaAlbero(t)
	troppi := make([]string, MaxVociBozza+1)
	for i := range troppi {
		troppi[i] = "cod:7121001"
	}
	for _, c := range []struct {
		nome  string
		sc    *scena
		b     BozzaAlbero
		frase string
	}{
		{"formato", nuovaSc, BozzaAlbero{Formato: 2}, "formato 2"},
		{"nodo che non c'e'", nuovaSc, BozzaAlbero{Tolti: []ToltoBozza{{Nodo: "cod:7129999"}}}, "non è nell'albero di adesso"},
		{"prodotto tolto", nuovaSc, BozzaAlbero{Tolti: []ToltoBozza{{Nodo: "cod:7120001"}}}, "non si toglie dall'albero"},
		{"tolto due volte", nuovaSc, BozzaAlbero{Tolti: []ToltoBozza{{Nodo: "cod:7121001"}, {Nodo: "cod:7121001"}}}, "tolto due volte"},
		{"rinomina di un componente", sc, BozzaAlbero{Rinomine: []RinominaBozza{{Nodo: "cod:7121001", Codice: "7121099"}}}, "si corregge nel Fascicolo"},
		{"codice vuoto", nuovaSc, BozzaAlbero{Rinomine: []RinominaBozza{{Nodo: "cod:7121001", Codice: " "}}}, "vuoto"},
		{"codice con uno spazio", nuovaSc, BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "nuovo:1", Codice: "71 29", Tipo: db.TipoComponenteSciolto,
			Padre: "cod:7120010", Qta: 1}}}, "caratteri non ammessi"},
		{"chiave dell'aggiunto", nuovaSc, BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "x", Codice: "7129001", Tipo: db.TipoComponenteSciolto,
			Padre: "cod:7120010", Qta: 1}}}, "nuovo:<n>"},
		{"tipo prodotto", nuovaSc, BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "nuovo:1", Codice: "7129001", Tipo: db.TipoComponenteFinito,
			Padre: "cod:7120010", Qta: 1}}}, "assieme, un particolare"},
		{"quantita'", nuovaSc, BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "nuovo:1", Codice: "7129001", Tipo: db.TipoComponenteSciolto,
			Padre: "cod:7120010", Qta: 0}}}, "la quantità va da 1"},
		{"sotto un particolare", nuovaSc, BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "nuovo:1", Codice: "7129001", Tipo: db.TipoComponenteSciolto,
			Padre: "cod:7121001", Qta: 1}}}, "sotto non ci va niente"},
		{"gia' nell'albero", nuovaSc, BozzaAlbero{Aggiunti: []AggiuntoBozza{{ID: "nuovo:1", Codice: "7121003", Tipo: db.TipoComponenteSciolto,
			Padre: "cod:7120010", Qta: 1}}}, "è già nell'albero"},
		{"ciclo", nuovaSc, BozzaAlbero{Tipi: []TipoBozza{{Nodo: "cod:7121003", Tipo: db.TipoComponenteSottoassieme}},
			Legami: []LegameBozza{{Padre: "cod:7121003", Figlio: "cod:7120010", Qta: 1}}}, "ciclo"},
		{"prodotto sotto un pezzo", nuovaSc, BozzaAlbero{Legami: []LegameBozza{{Padre: "cod:7120010", Figlio: "cod:7120001", Qta: 1}}}, "non va sotto"},
		{"legame che c'e'", nuovaSc, BozzaAlbero{Legami: []LegameBozza{{Padre: "cod:7120010", Figlio: "cod:7121001", Qta: 1}}}, "sta già sotto"},
		{"risposta senza proposta", nuovaSc, BozzaAlbero{Commerciali: []RispostaBozza{{Nodo: "cod:7121001", Risposta: RispostaSi}}}, "non ha una proposta commerciale"},
		{"troppi diversi", nuovaSc, BozzaAlbero{Diversi: troppi}, "il limite è"},
	} {
		_, err := c.sc.riepilogo(c.b)
		var rf Rifiuto
		if err == nil || !errors.As(err, &rf) || !strings.Contains(err.Error(), c.frase) {
			t.Errorf("%s: %v, atteso un rifiuto con «%s»", c.nome, err, c.frase)
		}
	}
	// il tipo scelto e la risposta alla proposta commerciale devono dire la stessa cosa
	sc2 := nuovaScena(t)
	sc2.prodotto("7120001")
	sc2.step("7120001.stp", []string{"#1=7120001", "#2=VITE TCEI ISO 4762 M6X16/7121007"}, []string{"#1>#2"})
	if _, err := sc2.riepilogo(BozzaAlbero{Tipi: []TipoBozza{{Nodo: "cod:7121007", Tipo: db.TipoComponenteSciolto}},
		Commerciali: []RispostaBozza{{Nodo: "cod:7121007", Risposta: RispostaSi}}}); err == nil || !strings.Contains(err.Error(), "non dicono la stessa cosa") {
		t.Errorf("tipo e risposta discordi: %v", err)
	}
	if _, err := sc2.riepilogo(BozzaAlbero{Commerciali: []RispostaBozza{{Nodo: "cod:7121007", Risposta: "forse"}}}); err == nil ||
		!strings.Contains(err.Error(), "✓ o ✗") {
		t.Errorf("una risposta che non e' ✓ o ✗: %v", err)
	}
}

// Il formato della bozza: il JSON si legge solo con i campi del formato e con il suo numero.
func TestLeggiBozza(t *testing.T) {
	b, err := LeggiBozza([]byte(`{"formato": 1, "base": "abc", "tolti": [{"nodo": "cod:7121001"}], "diversi": ["cod:7121002"]}`))
	if err != nil || b.Base != "abc" || len(b.Tolti) != 1 || b.Tolti[0].Nodo != "cod:7121001" || len(b.Diversi) != 1 {
		t.Errorf("la bozza: %+v %v", b, err)
	}
	for _, raw := range []string{`{"formato": 2}`, `{"formato": 1, "altro": true}`, `{"formato": 1} {"formato": 1}`, `[]`, `{`} {
		if _, err := LeggiBozza([]byte(raw)); err == nil {
			t.Errorf("%s: la bozza si legge", raw)
		}
	}
}

// La firma del riepilogo: la stessa bozza sullo stesso albero, la stessa firma; un'altra bozza, un'altra firma. Una bozza
// disegnata su un albero che nel frattempo e' cambiato e' «vecchia»: si legge, e non e' confermabile.
func TestRiepilogoLaFirmaELaBozzaVecchia(t *testing.T) {
	sc := scenaAlbero(t)
	a, b := sc.riepilogoOk(BozzaAlbero{}), sc.riepilogoOk(BozzaAlbero{})
	if a.Firma != b.Firma {
		t.Errorf("la stessa bozza, due firme: %s %s", a.Firma, b.Firma)
	}
	c := sc.riepilogoOk(BozzaAlbero{Tolti: []ToltoBozza{{Nodo: "cod:7121001"}}})
	if c.Firma == a.Firma || c.AlberoFirma != a.AlberoFirma {
		t.Errorf("un'altra bozza: %s %s", c.Firma, a.Firma)
	}
	v := sc.riepilogoOk(BozzaAlbero{Base: "firma di prima"})
	if !v.Vecchia || v.Confermabile || len(v.Blocchi) != 1 || !strings.Contains(v.Blocchi[0], "rivedila") {
		t.Errorf("la bozza vecchia: %v %v %v", v.Vecchia, v.Confermabile, v.Blocchi)
	}
}
