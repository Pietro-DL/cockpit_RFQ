package fascicolo

// L1 — l'albero proposto della Distinta (giro 4, fase 4.4a.1a; domande 27 = A, 29a, 29b = A, 30 seconda risposta, 6a,
// 6b; studio docs/specs/studio_albero_distinta_29-09.md § 2.1 e § 2.7): scene ACME inventate, con la scena delle prove
// del flusso di F8 (smistamento_scena_test.go).

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// albero calcola l'albero proposto sullo stato della scena, come LeggiAlberoProposto.
func (sc *scena) albero() AlberoProposto {
	sc.s.der = nil
	sc.s.Dichiarazioni = ValutaDichiarazioni(sc.dich)
	p := ProdottiDellaRfq(&sc.s)
	a := Ancore(p, &sc.s)
	return NuovoAlberoProposto(&sc.s, p, a, IndiceCodici(p, a, &sc.s), nil)
}

// nodoDi e' il nodo con il codice dato; la prova si ferma se non c'e'.
func nodoDi(t *testing.T, a AlberoProposto, codice string) *NodoAlbero {
	t.Helper()
	n, ok := a.Nodo(ChiaveDelCodice(codice))
	if !ok {
		t.Fatalf("nessun nodo %s nell'albero: %s", codice, righeAlbero(a))
	}
	return n
}

// righeAlbero e' l'albero in righe «padre → figlio ×qta [stato]», per le prove e i messaggi.
func righeAlbero(a AlberoProposto) string {
	var out []string
	for _, x := range a.Archi {
		r := fmt.Sprintf("%s → %s ×%d [%s]", strings.TrimPrefix(x.Padre, "cod:"), strings.TrimPrefix(x.Figlio, "cod:"), x.Qta, x.Stato)
		if x.QtaDiscordi {
			r += " (discordi)"
		}
		out = append(out, r)
	}
	return strings.Join(out, "; ")
}

// scenaAlbero e' la RFQ ACME del prodotto 7120001 (ancora piena: lo STEP del prodotto, con la radice 7120001) con tre
// assiemi, due dei quali con il loro STEP, e un quinto STEP che non si raggiunge:
//
//	7120001.stp: 7120001 → 7120010 ×1, 7120011 ×2, 7120012 ×1; 7120012 → 7121003 ×1, 7121006 ×1
//	7120010.stp: 7120010 → 7121001 ×1, 7121002 ×1, 7121003 ×2
//	7120011.stp: 7120011 → 7121003 ×1, 7121004 ×2; 7121004 → 7121009 ×3
//	7120014.stp: 7120014 → 7121006 ×1, 7121009 ×1 (nessun prodotto lo raggiunge)
//
// 7121003 sta sotto tre assiemi di tre file; 7121009 sta al quarto livello, in uno STEP di un sottoassieme.
func scenaAlbero(t *testing.T) *scena {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7120011", "#4=7120012", "#5=7121003", "#6=7121006"},
		[]string{"#1>#2", "#1>#3*2", "#1>#4", "#4>#5", "#4>#6"})
	sc.step("7120010.stp", []string{"#1=7120010", "#2=7121001", "#3=7121002", "#4=7121003"}, []string{"#1>#2", "#1>#3", "#1>#4*2"})
	sc.step("7120011.stp", []string{"#1=7120011", "#2=7121003", "#3=7121004", "#4=7121009"}, []string{"#1>#2", "#1>#3*2", "#3>#4*3"})
	sc.step("7120014.stp", []string{"#1=7120014", "#2=7121006", "#3=7121009"}, []string{"#1>#2", "#1>#3"})
	return sc
}

// Prova (studio § 2.1): l'albero intero, a tutti i livelli. Sotto 7120001 i tre assiemi dello STEP del prodotto; sotto
// 7120010 e 7120011 i pezzi dei loro STEP (raggiunti per codice, 27 = A); 7121009 al quarto livello; 7121003 un nodo
// solo con tre padri e tre archi; 7120014, che nessun prodotto raggiunge, fra i file senza posto, e il suo 7121009 non
// e' un secondo nodo. Tutto e' «proposto» tranne il prodotto, che c'e'.
func TestAlberoPropostoATuttiILivelli(t *testing.T) {
	a := scenaAlbero(t).albero()
	atteso := "7120001 → 7120010 ×1 [proposto]; 7120001 → 7120011 ×2 [proposto]; 7120001 → 7120012 ×1 [proposto]; " +
		"7120010 → 7121001 ×1 [proposto]; 7120010 → 7121002 ×1 [proposto]; 7120010 → 7121003 ×2 [proposto]; " +
		"7120011 → 7121003 ×1 [proposto]; 7120011 → 7121004 ×2 [proposto]; 7120012 → 7121003 ×1 [proposto]; " +
		"7120012 → 7121006 ×1 [proposto]; 7121004 → 7121009 ×3 [proposto]"
	if got := righeAlbero(a); got != atteso {
		t.Fatalf("l'albero:\n%s\natteso:\n%s", got, atteso)
	}
	if len(a.Prodotti) != 1 || a.Prodotti[0].Codice != "7120001" || a.Prodotti[0].Ancora != LivelloPiena ||
		strings.Join(a.Prodotti[0].File, ",") != "7120001.stp" || a.Prodotti[0].SoloProdotto {
		t.Errorf("il prodotto: %+v", a.Prodotti)
	}
	if n := nodoDi(t, a, "7121003"); strings.Join(n.Padri, " ") != "cod:7120010 cod:7120011 cod:7120012" || len(n.Righe) != 3 {
		t.Errorf("7121003: padri %v, righe %d", n.Padri, len(n.Righe))
	}
	for _, c := range []struct{ codice, file string }{{"7120010", "7120010.stp"}, {"7120011", "7120011.stp"}} {
		if n := nodoDi(t, a, c.codice); n.Step == nil || strings.Join(n.Step.File, ",") != c.file || n.Step.Livello != LivelloPiena {
			t.Errorf("lo STEP di %s: %+v", c.codice, n.Step)
		}
	}
	if n := nodoDi(t, a, "7121009"); strings.Join(n.Padri, " ") != "cod:7121004" || strings.Join(n.Prodotti, " ") != "7120001" {
		t.Errorf("7121009: padri %v, prodotti %v", n.Padri, n.Prodotti)
	}
	if len(a.SenzaPosto) != 1 || a.SenzaPosto[0].File != "7120014.stp" || strings.Join(a.SenzaPosto[0].Radici, ",") != "7120014" {
		t.Errorf("i file senza posto: %+v", a.SenzaPosto)
	}
	if _, ok := a.Nodo(ChiaveDelCodice("7120014")); ok {
		t.Error("7120014 non si raggiunge: non e' un nodo dell'albero")
	}
	for _, n := range a.Nodi {
		atteso := StatoAlberoProposto
		if n.Codice == "7120001" {
			atteso = StatoAlberoNellaDistinta
		}
		if n.Stato != atteso {
			t.Errorf("%s: stato %s, atteso %s", n.Chiave, n.Stato, atteso)
		}
	}
}

// Il tipo proposto: assieme se nell'albero ha figli, particolare se e' una foglia; il prodotto e' il prodotto. Mai
// commerciale (la proposta commerciale e' una domanda a parte).
func TestAlberoPropostoIlTipoDaiFigli(t *testing.T) {
	a := scenaAlbero(t).albero()
	for c, tipo := range map[string]db.TipoComponente{"7120001": db.TipoComponenteFinito, "7120010": db.TipoComponenteSottoassieme,
		"7120012": db.TipoComponenteSottoassieme, "7121004": db.TipoComponenteSottoassieme, "7121003": db.TipoComponenteSciolto,
		"7121009": db.TipoComponenteSciolto} {
		if n := nodoDi(t, a, c); n.Tipo != tipo {
			t.Errorf("%s: tipo %s (%s), atteso %s", c, n.Tipo, n.TipoMotivo, tipo)
		}
	}
}

// 6b: un particolare non ha mai figli. Il componente 7121004, gia' nella distinta come particolare sotto 7120011, a
// cui lo STEP di 7120011 mette sotto 7121009 ×3, e' proposto come assieme, con il motivo; il tipo che ha resta scritto
// accanto. 7120011 e il suo legame sono «nella distinta»; il legame 7121004 → 7121009 e' «proposto».
func TestAlberoPropostoUnParticolareConFigliEPropostoComeAssieme(t *testing.T) {
	sc := scenaAlbero(t)
	p := sc.s.Componenti[0]
	a11 := sc.componente("7120011", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale)
	p04 := sc.componente("7121004", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	sc.arco(p, a11, 2)
	sc.arco(a11, p04, 2)
	a := sc.albero()
	n := nodoDi(t, a, "7121004")
	if n.Tipo != db.TipoComponenteSottoassieme || n.TipoAttuale != db.TipoComponenteSciolto || !strings.Contains(n.TipoMotivo, "6b") ||
		n.Stato != StatoAlberoNellaDistinta || !n.Componente.Valid || n.FonteCodice != FonteCodiceWorking {
		t.Errorf("7121004: %+v", n)
	}
	if n.Commerciale != nil || n.Ritrovato == nil {
		t.Errorf("7121004 c'e': nessuna proposta commerciale, e le righe aperte dello STEP lo ritrovano per codice: %+v %+v", n.Commerciale, n.Ritrovato)
	}
	for _, r := range a.Archi {
		switch r.Padre + ">" + r.Figlio {
		case "cod:7120001>cod:7120011", "cod:7120011>cod:7121004":
			if r.Stato != StatoAlberoNellaDistinta || r.QtaWorking != 2 || r.Qta != 2 {
				t.Errorf("%s → %s: %+v", r.Padre, r.Figlio, r)
			}
		case "cod:7121004>cod:7121009":
			if r.Stato != StatoAlberoProposto || r.Qta != 3 {
				t.Errorf("7121004 → 7121009: %+v", r)
			}
		}
	}
}

// P13: un nodo che lo STEP propone ancora, con il codice di un componente che c'e' (7121002, scritto a mano e rimasto
// fra le radici da sistemare), e' «ritrovato per codice»: lo stato dice «proposto» (il legame non c'e'), il ritrovato
// dice quale componente, e la conferma lo dovra' dichiarare. Un archiviato con lo stesso codice e' ritrovato e detto
// archiviato.
func TestAlberoPropostoIlRitrovatoPerCodice(t *testing.T) {
	sc := scenaAlbero(t)
	c02 := sc.componente("7121002", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	c06 := sc.componente("7121006", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	ieri := sc.t0
	sc.s.Componenti[len(sc.s.Componenti)-1].ArchiviatoIl = &ieri
	a := sc.albero()
	n := nodoDi(t, a, "7121002")
	if n.Stato != StatoAlberoProposto || n.Ritrovato == nil || n.Ritrovato.Componente != c02.ComponenteID || n.Ritrovato.Archiviato ||
		!n.Componente.Valid || len(n.Vicini) != 0 || n.Commerciale != nil {
		t.Errorf("7121002: %+v, ritrovato %+v", n, n.Ritrovato)
	}
	if n := nodoDi(t, a, "7121006"); n.Ritrovato == nil || n.Ritrovato.Componente != c06.ComponenteID || !n.Ritrovato.Archiviato || n.Componente.Valid {
		t.Errorf("7121006, archiviato: %+v", n.Ritrovato)
	}
	if n := nodoDi(t, a, "7121001"); n.Ritrovato != nil {
		t.Errorf("7121001 non c'e': nessun ritrovato, %+v", n.Ritrovato)
	}
}

// P4: un pezzo che nascerebbe con un codice quasi uguale a uno che c'e' (7121001 accanto a 7121001A) porta i vicini,
// con il motivo, e nient'altro: nessun «diverso» segnato da solo (l'albero non ha nemmeno il campo). Un pezzo che c'e'
// non ha vicini: non nasce.
func TestAlberoPropostoIVicini(t *testing.T) {
	sc := scenaAlbero(t)
	sc.componente("7121001A", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	a := sc.albero()
	n := nodoDi(t, a, "7121001")
	if len(n.Vicini) != 1 || n.Vicini[0].Codice != "7121001A" || n.Vicini[0].Motivo != VicinoLetteraFinale {
		t.Errorf("i vicini di 7121001: %+v", n.Vicini)
	}
	if n := nodoDi(t, a, "7120001"); len(n.Vicini) != 0 {
		t.Errorf("il prodotto non nasce: %+v", n.Vicini)
	}
	if n := nodoDi(t, a, "7121002"); len(n.Vicini) != 0 {
		t.Errorf("7121002 non ha codici vicini: %+v", n.Vicini)
	}
}

// Domanda 30, seconda risposta: la proposta commerciale e' una domanda aperta, solo per una foglia che nascerebbe, dal
// NOME del pezzo (il PRODUCT). «VITE TCEI ISO 4762 M6X16» foglia: proposta, «normato», con il motivo; lo stesso genere
// di nome su un assieme (7120012 ha dei figli): nessuna proposta, e il tipo resta assieme; una rondella tagliata: «da
// vedere», nessuna proposta, solo la frase; un pezzo che c'e' con un nome da minuteria: nessuna proposta. Il tipo
// proposto resta «particolare»: il commerciale non e' mai un tipo scritto dall'albero.
func TestAlberoPropostoLaMinuteriaSoloSulleFoglie(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=VITE M8X20 UNI 5739 8.8/7120012", "#3=VITE TCEI ISO 4762 M6X16/7121007",
		"#4=RONDELLA D.40 SP.4 S235JR/7121008", "#5=DADO M8 DIN 934/7121002", "#6=7121003"},
		[]string{"#1>#2", "#1>#3*4", "#1>#4", "#1>#5", "#2>#6"})
	sc.componente("7121002", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	a := sc.albero()
	n := nodoDi(t, a, "7121007")
	if n.Commerciale == nil || n.Commerciale.Esito != classificazione.MinuteriaNormato ||
		n.Commerciale.Motivo != "normato: ISO 4762 · M6X16 · TCEI" || n.Commerciale.Nome != "VITE TCEI ISO 4762 M6X16" ||
		n.Tipo != db.TipoComponenteSciolto {
		t.Errorf("7121007: tipo %s, proposta %+v", n.Tipo, n.Commerciale)
	}
	if n := nodoDi(t, a, "7120012"); n.Commerciale != nil || n.Tipo != db.TipoComponenteSottoassieme {
		t.Errorf("7120012 ha un figlio: nessuna proposta, tipo assieme; %s %+v", n.Tipo, n.Commerciale)
	}
	if n := nodoDi(t, a, "7121008"); n.Commerciale != nil || len(n.Note) != 1 || !strings.Contains(n.Note[0], "da vedere") {
		t.Errorf("7121008, da vedere: %+v, note %v", n.Commerciale, n.Note)
	}
	if n := nodoDi(t, a, "7121002"); n.Commerciale != nil {
		t.Errorf("7121002 c'e' gia': nessuna proposta, %+v", n.Commerciale)
	}
	for _, n := range a.Nodi {
		if n.Tipo == db.TipoComponenteCommerciale {
			t.Errorf("%s: l'albero ha scritto il tipo commerciale", n.Chiave)
		}
	}
}

// 6a: un commerciale che c'e' e' una foglia. Lo STEP gli mette sotto dei pezzi: restano guida, l'albero non scende, e
// il nodo lo dice. Il suo STEP proprio non si cammina.
func TestAlberoPropostoSottoUnCommercialeNonSiScende(t *testing.T) {
	sc := scenaAlbero(t)
	sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale)
	a := sc.albero()
	n := nodoDi(t, a, "7120010")
	if n.Figli != 0 || n.Step != nil || n.Tipo != db.TipoComponenteCommerciale || len(n.Note) == 0 || !strings.Contains(n.Note[0], "6a") {
		t.Errorf("7120010 commerciale: figli %d, step %+v, tipo %s, note %v", n.Figli, n.Step, n.Tipo, n.Note)
	}
	if _, ok := a.Nodo(ChiaveDelCodice("7121001")); ok {
		t.Error("7121001 sta solo sotto il commerciale: non e' nell'albero")
	}
	if n := nodoDi(t, a, "7121003"); strings.Join(n.Padri, " ") != "cod:7120011 cod:7120012" {
		t.Errorf("7121003 resta sotto gli altri due assiemi: %v", n.Padri)
	}
}

// Una riga scartata da una persona si vede, «scartata», e l'albero non scende sotto: 7120010 scartato nello STEP del
// prodotto porta via i figli che erano solo suoi (7121001, 7121002: non si raggiungono piu'), e 7121003 resta sotto
// gli altri due padri. Un legame scartato e basta lascia il figlio proposto dove un altro legame lo tiene.
func TestAlberoPropostoLeRigheScartate(t *testing.T) {
	sc := scenaAlbero(t)
	sha := sc.f("7120001.stp").Sha
	for i, n := range sc.s.Nodi {
		if n.Sha256 == sha && n.Chiave == "#2" {
			sc.s.Nodi[i].Stato, sc.s.Nodi[i].DecisoDa = db.StatoPropostaScartata, nullo(sc.chi, true)
		}
	}
	for i, r := range sc.s.Archi {
		if r.AllegatoID == idDa("allegato:7120001.stp") && r.PadreChiave == "#4" && r.FiglioChiave == "#5" {
			sc.s.Archi[i].Stato, sc.s.Archi[i].DecisoDa = db.StatoPropostaScartata, nullo(sc.chi, true)
		}
	}
	a := sc.albero()
	if n := nodoDi(t, a, "7120010"); n.Stato != StatoAlberoScartato || n.Figli != 0 || n.Step != nil {
		t.Errorf("7120010 scartato: %+v", n)
	}
	for _, c := range []string{"7121001", "7121002"} {
		if _, ok := a.Nodo(ChiaveDelCodice(c)); ok {
			t.Errorf("%s era solo sotto lo scartato: non e' nell'albero", c)
		}
	}
	if n := nodoDi(t, a, "7121003"); n.Stato != StatoAlberoProposto || strings.Join(n.Padri, " ") != "cod:7120011" {
		t.Errorf("7121003: %s, padri %v", n.Stato, n.Padri)
	}
	scartati := 0
	for _, r := range a.Archi {
		if r.Stato == StatoAlberoScartato {
			scartati++
			if len(r.Fonti) != 1 || !r.Fonti[0].Scartata {
				t.Errorf("il legame scartato %s → %s: %+v", r.Padre, r.Figlio, r.Fonti)
			}
		}
	}
	if scartati != 2 {
		t.Errorf("legami scartati: %d, attesi 2 (7120001 → 7120010, 7120012 → 7121003): %s", scartati, righeAlbero(a))
	}
}

// 6c: due file che dicono quantita' diverse dello stesso legame (lo STEP del prodotto elenca 7121003 ×2 sotto 7120010,
// lo STEP di 7120010 ×3): il legame e' uno, con le due fonti, «discorde», e non vince nessuna (Qta 0). Con la
// struttura diversa dei due file la voce dell'indice lo dice.
func TestAlberoPropostoLeQuantitaDiscordi(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7121003", "#4=7121005"}, []string{"#1>#2", "#2>#3*2", "#2>#4"})
	sc.step("7120010.stp", []string{"#1=7120010", "#2=7121003"}, []string{"#1>#2*3"})
	a := sc.albero()
	var trovato bool
	for _, r := range a.Archi {
		if r.Padre == "cod:7120010" && r.Figlio == "cod:7121003" {
			trovato = true
			if !r.QtaDiscordi || r.Qta != 0 || len(r.Fonti) != 2 || r.Fonti[0].Qta+r.Fonti[1].Qta != 5 {
				t.Errorf("7120010 → 7121003: %+v", r)
			}
		}
	}
	if !trovato {
		t.Fatalf("il legame manca: %s", righeAlbero(a))
	}
	if n := nodoDi(t, a, "7120010"); !contiene(n.Discordanze, DiscStrutturaDiversa) {
		t.Errorf("7120010: discordanze %v", n.Discordanze)
	}
}

// Due nodi con lo stesso codice sotto lo stesso nodo di un file sono pezzi in piu': le quantita' si sommano. Un
// PRODUCT con lo stesso codice del padre dentro il padre e' lo stesso pezzo: i suoi figli sono del padre.
func TestAlberoPropostoLoStessoCodiceNelloStessoFile(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7121003", "#3=7121003", "#4=7120001", "#5=7121004"},
		[]string{"#1>#2*2", "#1>#3", "#1>#4", "#4>#5*3"})
	a := sc.albero()
	if got := righeAlbero(a); got != "7120001 → 7121003 ×3 [proposto]; 7120001 → 7121004 ×3 [proposto]" {
		t.Errorf("l'albero: %s", got)
	}
}

// L'albero parte dallo STEP che F8 riconosce come l'ancora del prodotto. Uno STEP la cui radice e' vicina al prodotto
// (7120001A per 7120001) e' un'ancora da confermare: l'albero scende lo stesso, e il prodotto lo dice (il livello e le
// discordanze, che il passo 1 della fase 4.4a.2 chiedera' a una persona). Autorizzato per il prodotto dalla sua radice,
// e' l'ancora autorizzata: stesso albero. La radice del file e' il prodotto, non un nodo in piu'.
func TestAlberoPropostoDalloStepDellAncora(t *testing.T) {
	sc := nuovaScena(t)
	p := sc.prodotto("7120001")
	sc.step("7120001A_1.stp", []string{"#1=7120001A", "#2=7120010", "#3=7120011"}, []string{"#1>#2*2", "#1>#3"})
	for _, x := range []struct{ livello, disc string }{{LivelloDaConfermare, DiscRadiceDiversa}, {LivelloAutorizzata, ""}} {
		if x.livello == LivelloAutorizzata {
			sc.autorizza(p, "7120001A_1.stp", "#1")
		}
		a := sc.albero()
		if got := righeAlbero(a); got != "7120001 → 7120010 ×2 [proposto]; 7120001 → 7120011 ×1 [proposto]" {
			t.Errorf("%s: l'albero %s", x.livello, got)
		}
		if pa := a.Prodotti[0]; pa.Ancora != x.livello || strings.Join(pa.File, ",") != "7120001A_1.stp" ||
			(x.disc != "" && !contiene(pa.Discordanze, x.disc)) {
			t.Errorf("%s: il prodotto %+v", x.livello, pa)
		}
		if _, ok := a.Nodo(ChiaveDelCodice("7120001A")); ok {
			t.Errorf("%s: la radice del file e' il prodotto, non un nodo", x.livello)
		}
	}
}

// Senza uno STEP del prodotto l'albero ha un livello solo, il prodotto, e dice perche': con il solo disegno le righe
// dell'elenco particolari entrano con la fase 4.12. Un altro prodotto della stessa RFQ con il suo STEP ha il suo albero.
func TestAlberoPropostoSenzaStepUnLivelloSolo(t *testing.T) {
	sc := scenaAlbero(t)
	sc.prodotto("7120002")
	sc.disegno("7120002.pdf")
	a := sc.albero()
	if len(a.Prodotti) != 2 {
		t.Fatalf("i prodotti: %+v", a.Prodotti)
	}
	p := a.Prodotti[1]
	if p.Codice != "7120002" || !p.SoloProdotto || p.Perche == "" || p.Ancora != LivelloAssente {
		t.Errorf("7120002: %+v", p)
	}
	if n := nodoDi(t, a, "7120002"); n.Figli != 0 || n.Stato != StatoAlberoNellaDistinta || !n.Prodotto {
		t.Errorf("7120002: %+v", n)
	}
	if n := nodoDi(t, a, "7121003"); strings.Join(n.Prodotti, " ") != "7120001" {
		t.Errorf("7121003 sta solo sotto 7120001: %v", n.Prodotti)
	}
}

// Un nodo senza codice e' un nodo a se', con la sua riga, e lo dice: il codice si scrive prima della conferma.
func TestAlberoPropostoIlNodoSenzaCodice(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=TELAIO ANTERIORE/", "#3=7121003"}, []string{"#1>#2", "#2>#3"})
	a := sc.albero()
	var senza *NodoAlbero
	for i := range a.Nodi {
		if a.Nodi[i].Codice == "" {
			senza = &a.Nodi[i]
		}
	}
	if senza == nil || !strings.HasPrefix(senza.Chiave, "nodo:") || senza.Nome != "TELAIO ANTERIORE" || senza.Figli != 1 ||
		senza.Tipo != db.TipoComponenteSottoassieme || len(senza.Note) != 1 || senza.Stato != StatoAlberoProposto {
		t.Fatalf("il nodo senza codice: %+v", senza)
	}
	if n := nodoDi(t, a, "7121003"); strings.Join(n.Padri, " ") != senza.Chiave {
		t.Errorf("7121003 sta sotto il nodo senza codice: %v", n.Padri)
	}
}

// FP7: la stessa RFQ con i file in un altro ordine e' lo stesso albero, con la stessa firma. Un cambio di stato cambia
// la firma: un legame della working, un componente, una riga scartata.
func TestAlberoPropostoLaFirma(t *testing.T) {
	a := scenaAlbero(t).albero()
	sc := scenaAlbero(t)
	for i, j := 0, len(sc.s.File)-1; i < j; i, j = i+1, j-1 {
		sc.s.File[i], sc.s.File[j] = sc.s.File[j], sc.s.File[i]
	}
	sort.SliceStable(sc.s.Nodi, func(i, j int) bool { return sc.s.Nodi[i].Chiave > sc.s.Nodi[j].Chiave })
	b := sc.albero()
	if a.Firma == "" || a.Firma != b.Firma || righeAlbero(a) != righeAlbero(b) {
		t.Fatalf("lo stesso stato in un altro ordine: %s contro %s", a.Firma, b.Firma)
	}
	sc.componente("7129999", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	if c := sc.albero(); c.Firma == a.Firma {
		t.Error("un componente nuovo nella working non cambia la firma")
	}
	sc = scenaAlbero(t)
	sc.s.Nodi[3].Stato, sc.s.Nodi[3].DecisoDa = db.StatoPropostaScartata, nullo(sc.chi, true)
	if c := sc.albero(); c.Firma == a.Firma {
		t.Error("una riga scartata non cambia la firma")
	}
}

// Lo scenario MG della fase 4.3 (dati fittizi, i prodotti come dice il cartiglio, senza la P): i figli in comune fra
// due assiemi sono un nodo solo con due padri (7120111 sotto 7120100 e 7120101; 7120133 ×2 sotto 7120103 e 7120104),
// e 7120102, che la mail non chiede, sta fra i file senza posto con i suoi figli. Con i prodotti della mail (con la P) gli STEP non
// ancorano: gli alberi hanno un livello solo, finche' l'alias della regola del cliente (fase 4.9) non lega P7120100 a
// 7120100 (da fare).
func TestAlberoPropostoScenarioMG(t *testing.T) {
	a := scenaMGCon(t, "7120100", "7120103", "7120101", "7120104").albero()
	if n := nodoDi(t, a, "7120111"); strings.Join(n.Padri, " ") != "cod:7120100 cod:7120101" {
		t.Errorf("7120111: padri %v", n.Padri)
	}
	var qta []string
	for _, r := range a.Archi {
		if r.Figlio == "cod:7120133" {
			qta = append(qta, fmt.Sprintf("%s×%d", strings.TrimPrefix(r.Padre, "cod:"), r.Qta))
		}
	}
	if strings.Join(qta, " ") != "7120103×2 7120104×2" {
		t.Errorf("7120133: %v", qta)
	}
	var senza []string
	for _, f := range a.SenzaPosto {
		senza = append(senza, f.File)
	}
	// 7120102 e i suoi figli 7120120 e 7120121; 7120105 e 7120106, che nessun elenco nomina
	if atteso := []string{nomeSTEPMG("7120102"), nomeSTEPMG("7120105"), nomeSTEPMG("7120106"), nomeSTEPMG("7120120"),
		nomeSTEPMG("7120121")}; strings.Join(senza, ",") != strings.Join(atteso, ",") {
		t.Errorf("i file senza posto: %v", senza)
	}

	conP := scenaMG(t).albero()
	t.Run("da fare: con i prodotti della mail l'albero scende nello STEP dell'assieme", func(t *testing.T) {
		p := conP.Prodotti[0]
		daFareMG(t, !p.SoloProdotto && len(conP.Archi) > 0,
			"P7120100 della mail e la radice 7120100 dello STEP sono lo stesso pezzo (alias della regola del cliente, fase 4.9): l'albero di P7120100 ha i figli dello STEP",
			fmt.Sprintf("%s: %s, %d legami", p.Codice, p.Perche, len(conP.Archi)))
	})
}
