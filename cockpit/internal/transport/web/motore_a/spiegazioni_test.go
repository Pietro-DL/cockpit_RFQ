package motore_a

// L1 — giro 5, A1d, notte 14/10, WP-U (U2): le spiegazioni dei codici dell'ancoraggio (spiegazioni.go) e le ricerche
// dell'indice che le portano alla pagina: DiagnosticheAncoraggio (del prodotto), DiagnosticheAncoraggioDellaRFQ (di
// nessun prodotto), MotiviAncoraggio (di un file), OrigineStruttura (il badge dell'origine di una struttura candidata,
// letto per nome finché WP-A non porta il campo in questo ramo).
//
// Solo dati ACME inventati: codici ACM-nnnn, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// codiciAttesiWPA: le voci della tabella che il motore di questo ramo non produce ancora: le introduce WP-A
// (ancoraggio, notte 14/10) in un altro ramo. Dopo l'unione diventano costanti di core/ancoraggio e la prova le
// ritrova anche fra le sorgenti; qui servono a non lasciar passare un refuso nella tabella.
var codiciAttesiWPA = map[string]bool{"radice_dal_nome_file": true, "namespace_non_confrontabile": true}

func TestSpiegaCodiciNotiESconosciuti(t *testing.T) {
	for _, c := range []struct {
		codice, titolo string
		frase          bool
	}{
		{"ancoraggio.target_senza_struttura", "prodotto senza struttura", true},
		{"target_senza_struttura", "prodotto senza struttura", true},
		{"nessuna_lettura_identita", "nessun codice letto dal file", true},
		{"namespace_non_confrontabile", "codice di un'altra famiglia", true},
		{"ancoraggio.radice_dal_nome_file", "radice dal nome del file", true},
		// un codice dell'ancoraggio senza spiegazione ma con l'etichetta della diagnostica: il titolo è l'etichetta
		{"ancoraggio.righe_stessa_base", Etichetta("codice_diagnostica", "ancoraggio.righe_stessa_base"), false},
		// un codice che nessuno conosce: il titolo dal codice, mai fra quadre
		{"ancoraggio.caso_mai_visto", "caso mai visto", false},
		{"motivo_inventato", "motivo inventato", false},
		{"", "motivo senza codice", false},
	} {
		s := Spiega(c.codice)
		if s.Codice != c.codice || s.Titolo != c.titolo || (s.Frase != "") != c.frase {
			t.Errorf("Spiega(%q) = %+v, attesi titolo %q e frase %v", c.codice, s, c.titolo, c.frase)
		}
		if strings.ContainsAny(s.Titolo+s.Frase, "[]<>") {
			t.Errorf("Spiega(%q): quadre o HTML nel testo: %+v", c.codice, s)
		}
	}
}

// TestSpiegazioniCopronoICodiciDiAncoraggio: ogni motivo dell'ancoraggio di un file (il blocco const di
// MotivoAncoraggioTargetIgnoti) e ogni codice delle strutture dei prodotti (il blocco di CodiceTargetSenzaStruttura) ha
// la sua spiegazione; ogni voce della tabella viene da una di quelle costanti o è fra quelle attese da WP-A. Riscritta
// per K4: anche il blocco delle equivalenze del cartiglio (CodiceEquivalenzaCartiglioContesa).
func TestSpiegazioniCopronoICodiciDiAncoraggio(t *testing.T) {
	const anc = "../../../core/ancoraggio"
	costanti := maCostantiDi(t, anc)
	blocco := func(nome string) []string {
		b := -1
		for _, c := range costanti {
			if c.nome == nome {
				b = c.blocco
			}
		}
		var out []string
		for _, c := range costanti {
			if c.blocco == b {
				out = append(out, c.valore)
			}
		}
		if len(out) == 0 {
			t.Fatalf("il blocco const di %s non dà valori (un nome cambiato?)", nome)
		}
		return out
	}
	visti := map[string]bool{}
	for _, v := range append(append(blocco("MotivoAncoraggioTargetIgnoti"), blocco("CodiceTargetSenzaStruttura")...), blocco("CodiceEquivalenzaCartiglioContesa")...) {
		caso := strings.TrimPrefix(v, prefissoAncoraggio)
		visti[caso] = true
		if _, ok := spiegazioni[caso]; !ok {
			t.Errorf("%q non ha una spiegazione", v)
		}
	}
	// MotivoAncoraggioGrafoIncompleto e MotivoAncoraggioTargetSenzaStruttura sono alias delle costanti dei codici: il
	// loro valore sta nel blocco dei codici, già letto
	var voci []string
	for k := range spiegazioni {
		voci = append(voci, k)
	}
	sort.Strings(voci)
	for _, k := range voci {
		if !visti[k] && !codiciAttesiWPA[k] {
			t.Errorf("la spiegazione %q non viene da nessuna costante di core/ancoraggio né è attesa da WP-A (un refuso?)", k)
		}
		if v := spiegazioni[k]; v.titolo == "" || v.frase == "" || strings.ContainsAny(v.titolo+v.frase, "[]<>") ||
			strings.TrimSpace(v.frase) != v.frase {
			t.Errorf("la spiegazione %q non è pulita: %+v", k, v)
		}
	}
}

// maScenaAncoraggi: due prodotti ACME e un esito con le diagnostiche e i motivi dell'ancoraggio.
func maScenaAncoraggi() (*fotorfq.Thread, *valutazione.EsitoThread) {
	th := &fotorfq.Thread{ID: maUUID(1), ClienteID: maUUID(2),
		Allegati:   []fotorfq.Allegato{{ID: maUUID(31), NomeFile: "ACM-1000_A.stp"}, {ID: maUUID(32), NomeFile: "ACM-2000.pdf"}},
		Componenti: []fotorfq.Componente{{ID: maUUID(10), Codice: "ACM-1000"}, {ID: maUUID(20), Codice: "ACM-2000"}}}
	e := &valutazione.EsitoThread{ThreadID: maUUID(1), Valutato: true,
		ProdottiValutati: []valutazione.ProdottoValutato{
			{Rif: maP1, ComponenteID: maUUIDp(10), CodiceRichiesto: "ACM-1000"},
			{Rif: maP2, ComponenteID: maUUIDp(20), CodiceRichiesto: "ACM-2000"},
		}}
	e.Ancoraggi.Diagnostiche = []evidenze.Diagnostica{
		{Codice: ancoraggio.CodiceTargetSenzaStruttura, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
			Messaggio: "messaggio ACME del prodotto 1", Rif: []string{maP1}},
		{Codice: "ancoraggio.radice_dal_nome_file", Gravita: evidenze.GravitaNota, Natura: evidenze.NaturaDati,
			Messaggio: "messaggio ACME del file", Rif: []string{"allegato:" + maUUID(31).String()}},
		{Codice: ancoraggio.CodiceGrafoIncompleto, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
			Messaggio: "messaggio ACME del prodotto 2", Rif: []string{maP2, "allegato:" + maUUID(31).String()}},
	}
	e.Ancoraggi.File = []ancoraggio.AncoraggioFile{
		{AllegatoID: maUUID(31), Motivi: []string{ancoraggio.MotivoAncoraggioTargetSenzaStruttura,
			ancoraggio.MotivoAncoraggioNessunaLettura, "namespace_non_confrontabile"}},
		{AllegatoID: maUUID(32)},
	}
	return th, e
}

func TestDiagnosticheAncoraggioDelProdottoEDellaRFQ(t *testing.T) {
	th, e := maScenaAncoraggi()
	x := NuovoIndice(th, e, nil, nil)
	titoli := func(ss []Spiegazione) []string {
		var out []string
		for _, s := range ss {
			out = append(out, s.Titolo+"|"+s.Messaggio+"|"+string(s.Gravita))
		}
		return out
	}
	if got, want := titoli(x.DiagnosticheAncoraggio(maP1)), []string{"prodotto senza struttura|messaggio ACME del prodotto 1|avviso"}; !reflect.DeepEqual(got, want) {
		t.Errorf("del prodotto 1: %v, attese %v", got, want)
	}
	if got, want := titoli(x.DiagnosticheAncoraggio(maP2)), []string{"struttura dello STEP incompleta|messaggio ACME del prodotto 2|avviso"}; !reflect.DeepEqual(got, want) {
		t.Errorf("del prodotto 2: %v, attese %v", got, want)
	}
	if got, want := titoli(x.DiagnosticheAncoraggioDellaRFQ()), []string{"radice dal nome del file|messaggio ACME del file|nota"}; !reflect.DeepEqual(got, want) {
		t.Errorf("della RFQ: %v, attese %v", got, want)
	}
	if x.DiagnosticheAncoraggio("") != nil || x.DiagnosticheAncoraggio(maComp(99)) != nil {
		t.Error("un prodotto vuoto o che non c'è ha delle diagnostiche")
	}
	var nilx *Indice
	if nilx.DiagnosticheAncoraggio(maP1) != nil || nilx.DiagnosticheAncoraggioDellaRFQ() != nil || nilx.MotiviAncoraggio(maUUID(31)) != nil ||
		nilx.OrigineStruttura(maP1, maUUID(31), "x") != nil {
		t.Error("l'indice nil risponde con qualcosa")
	}
}

func TestMotiviAncoraggioDelFile(t *testing.T) {
	th, e := maScenaAncoraggi()
	x := NuovoIndice(th, e, nil, nil)
	var got []string
	for _, s := range x.MotiviAncoraggio(maUUID(31)) {
		got = append(got, s.Codice+"="+s.Titolo)
	}
	want := []string{"ancoraggio.target_senza_struttura=prodotto senza struttura", "nessuna_lettura_identita=nessun codice letto dal file",
		"namespace_non_confrontabile=codice di un'altra famiglia"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("motivi: %v, attesi %v", got, want)
	}
	if x.MotiviAncoraggio(maUUID(32)) != nil || x.MotiviAncoraggio(maUUID(99)) != nil {
		t.Error("un file senza motivi, o che non c'è, ha dei motivi")
	}
	if af := x.ancoraggioFile(maUUID(31)); af == nil || af.AllegatoID != maUUID(31) {
		t.Errorf("ancoraggioFile: %+v", af)
	}
}

// TestOrigineStrutturaLettaPerNome: il badge dell'origine viene dal campo Origine letto per nome (campoTesto), anche di
// un tipo con nome; senza il campo (questo ramo, prima di WP-A) niente badge; le origini di sempre non hanno badge.
func TestOrigineStrutturaLettaPerNome(t *testing.T) {
	type origine string
	type conOrigine struct {
		Radice  string
		Origine origine
	}
	type conNumero struct{ Origine int }
	if got := campoTesto(conOrigine{Origine: OrigineStrutturaNomeFile}, "Origine"); got != OrigineStrutturaNomeFile {
		t.Errorf("campoTesto con un tipo con nome: %q", got)
	}
	if campoTesto(conNumero{Origine: 3}, "Origine") != "" || campoTesto(struct{ Radice string }{"x"}, "Origine") != "" ||
		campoTesto("non una struct", "Origine") != "" {
		t.Error("campoTesto: un campo che non c'è o non è una stringa dà un testo")
	}
	for o, testo := range map[string]string{OrigineStrutturaNomeFile: "radice dal nome del file", OrigineStrutturaAssociazione: "radice dallo STEP associato"} {
		b := badgeOrigine(o)
		if b == nil || b.Testo != testo || !strings.Contains(b.Titolo, "origine = "+o) {
			t.Errorf("badgeOrigine(%q) = %+v", o, b)
		}
	}
	for _, o := range []string{"", "base_del_target", "radice_scelta", "inventata"} {
		if badgeOrigine(o) != nil {
			t.Errorf("badgeOrigine(%q) dà un badge", o)
		}
	}
	// nell'indice: la struttura del prodotto nell'allegato con la radice; con StrutturaProdotto di questo ramo (senza il
	// campo) nessun badge, e l'indice non va in panic
	th, e := maScenaAncoraggi()
	e.Ancoraggi.Strutture = []ancoraggio.StrutturaProdotto{{Target: maP1, AllegatoID: maUUID(31), Radice: maNodo(31, "#1"),
		Stato: ancoraggio.StatoStrutturaCandidata}}
	x := NuovoIndice(th, e, nil, nil)
	_, ha := reflect.TypeOf(ancoraggio.StrutturaProdotto{}).FieldByName("Origine")
	if b := x.OrigineStruttura(maP1, maUUID(31), maNodo(31, "#1")); !ha && b != nil {
		t.Errorf("senza il campo Origine un badge: %+v", b)
	}
	if x.OrigineStruttura(maP1, uuid.Nil, "") != nil {
		t.Error("una struttura che non c'è dà un badge")
	}
}
