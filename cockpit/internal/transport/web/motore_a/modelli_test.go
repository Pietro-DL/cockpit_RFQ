package motore_a

// L1 — giro 5, A1d-Q2: i modelli della pagina e il JSON di /dati (contratto A1d v1, §2.2 e §2.4). Derivate dalle prove
// della PR #18 di Stefano Valo (67dcc02: TestMotoreAComponiLaPagina, TestMotoreADatiJSON), sul sottopackage.
//   - I campi di Pagina, Testa e Fase con il loro tag: le chiavi di §2.4, e niente indice né Scrive nel JSON (Q-A1d-11).
//   - Il round trip: il JSON della pagina composta sulla scena ACME torna, con DisallowUnknownFields, in una forma
//     scritta qui con le sole chiavi del contratto; «errore» è vuoto. Riscritte per V1 (A-28): le chiavi e i nomi di EM
//     §9.9 (gruppi[], fonte, f8_stato, bozza_del_prodotto, motivo_senza_gruppi), anche sui tag dei tipi.
//   - ComponiPagina: la testa dalla fotografia e dalla fase, l'esito e il confronto del thread, l'indice sullo stesso
//     esito, il prodotto scelto, Calcolato = PresaIl; VistaDa e ProdottoScelto.
//
// I clienti di questi test sono inventati: la scena ACME dell'indice (indice_test.go).

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

func TestModelliIcampiEITag(t *testing.T) {
	campi := func(v any) []string {
		var out []string
		tipo := reflect.TypeOf(v)
		for i := range tipo.NumField() {
			f := tipo.Field(i)
			out = append(out, f.Name+" "+f.Tag.Get("json"))
		}
		return out
	}
	for _, c := range []struct {
		v      any
		atteso []string
	}{
		{Pagina{}, []string{"ThreadID thread_id", "Vista vista", "Prodotto prodotto", "Testa testa", "Esito esito",
			"Confronto confronto", "Errore errore", "Scrive -", "Indice -", "Calcolato calcolato",
			// riscritta per V1 (EM §9.9; A-28)
			"Gruppi gruppi,omitempty", "MotivoSenzaGruppi motivo_senza_gruppi,omitempty", "Fonte fonte,omitempty", "F8Stato f8_stato,omitempty",
			"BozzaDelProdotto bozza_del_prodotto,omitempty",
			// ondata C (U2 §5.3)
			"ConfermaCorrente conferma_corrente,omitempty", "Modello -"}},
		{Testa{}, []string{"Cliente cliente", "Oggetto oggetto", "RiferimentoCliente riferimento_cliente",
			"StatoThread stato_thread", "Fase fase"}},
		{Fase{}, []string{"Nome nome", "Etichetta etichetta", "Inizio inizio", "GgInFase gg_in_fase", "Semaforo semaforo",
			"Terminale terminale"}},
	} {
		if got := campi(c.v); !slices.Equal(got, c.atteso) {
			t.Errorf("%T:\n %q\natteso:\n %q", c.v, got, c.atteso)
		}
	}
}

// maRisultatoACME: il risultato di un calcolo sulla scena ACME, con la fase e il confronto.
func maRisultatoACME() *Risultato {
	f, e, c := scenaIndiceACME()
	r := &Risultato{Foto: f, Esito: e, Confronto: c, Fase: &Fase{Nome: "FATTIBILITA", Etichetta: "Fattibilità ACME",
		Inizio: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC), GgInFase: 3, Semaforo: "verde"}}
	r.Thread, r.EsitoThread = &r.Foto.Thread[0], &r.Esito.Thread[0]
	return r
}

func TestComponiPagina(t *testing.T) {
	r := maRisultatoACME()
	p := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Vista: "documenti", Prodotto: maP2, Scrive: true})
	if p.ThreadID != maUUID(1) || p.Vista != VistaDocumenti || p.Prodotto != maP2 || !p.Scrive {
		t.Errorf("pagina %+v", p)
	}
	if p.Testa != (Testa{Cliente: "ACME S.p.A.", Oggetto: "RFQ 7120001", RiferimentoCliente: "ACME-030P7120001", StatoThread: "APERTA", Fase: r.Fase}) {
		t.Errorf("testa %+v", p.Testa)
	}
	if p.Esito != r.EsitoThread || p.Confronto == nil || p.Confronto.Impronta != r.Confronto.Impronta || !p.Calcolato.Equal(r.Foto.PresaIl) {
		t.Errorf("esito %p (atteso %p), confronto %v, calcolato %s", p.Esito, r.EsitoThread, p.Confronto, p.Calcolato)
	}
	if p.Indice == nil || p.Indice.Prodotto(maP1) != &r.EsitoThread.ProdottiValutati[0] {
		t.Error("l'indice non è sullo stesso esito della pagina")
	}
	if p.Errore != "" {
		t.Errorf("errore %q", p.Errore)
	}
	// un prodotto che non c'è: il primo; un risultato vuoto: la pagina senza esito, senza panic
	if q := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Prodotto: "identificativo:7120003"}); q.Prodotto != maP1 || q.Vista != VistaDistinta {
		t.Errorf("prodotto %q, vista %q", q.Prodotto, q.Vista)
	}
	if q := ComponiPagina(nil, RichiestaPagina{Thread: maUUID(1)}); q.Esito != nil || q.Indice != nil {
		t.Errorf("senza risultato: %+v", q)
	}
	for v, atteso := range map[string]string{"": VistaDistinta, "rfq": VistaRFQ, "documenti": VistaDocumenti, "distinta": VistaDistinta,
		"RFQ": VistaDistinta, "boh": VistaDistinta} {
		if got := VistaDa(v); got != atteso {
			t.Errorf("VistaDa(%q) = %q", v, got)
		}
	}
	if ProdottoScelto(nil, maP1) != "" || ProdottoScelto(&valutazione.EsitoThread{}, maP1) != "" {
		t.Error("senza prodotti il prodotto scelto è vuoto")
	}
}

// TestComponiPaginaSenzaV1 (revisione della corsia motore, ondata B: i tempi di /dati): con SenzaV1 la pagina non calcola
// le aggiunte di V1 (un'importazione per gruppo con uno STEP), che la pagina HTML e il suo frammento non leggono; il resto
// della pagina è lo stesso, e senza SenzaV1 (/dati) le aggiunte ci sono.
func TestComponiPaginaSenzaV1(t *testing.T) {
	con := ComponiPagina(maRisultatoACME(), RichiestaPagina{Thread: maUUID(1), Scrive: true})
	senza := ComponiPagina(maRisultatoACME(), RichiestaPagina{Thread: maUUID(1), Scrive: true, SenzaV1: true})
	if len(con.Gruppi) == 0 || con.Fonte == nil {
		t.Fatalf("senza SenzaV1 le aggiunte di V1 mancano: %+v", con.Gruppi)
	}
	if senza.Gruppi != nil || senza.Fonte != nil || senza.MotivoSenzaGruppi != nil || senza.BozzaDelProdotto != nil || senza.F8Stato != "" {
		t.Errorf("con SenzaV1 le aggiunte di V1 ci sono: %+v", senza)
	}
	if senza.Prodotto != con.Prodotto || senza.Esito != nil && con.Esito != nil && senza.Esito.ThreadID != con.Esito.ThreadID || senza.Indice == nil {
		t.Errorf("con SenzaV1 cambia il resto della pagina: %+v", senza)
	}
	b, _ := json.Marshal(senza)
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal(b, &chiavi); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"gruppi", "fonte", "bozza_del_prodotto", "motivo_senza_gruppi", "f8_stato"} {
		if _, ok := chiavi[k]; ok {
			t.Errorf("con SenzaV1 il JSON porta %q", k)
		}
	}
}

// TestFonteDelProdottoIMotivi (revisione della corsia motore, ondata B): il motivo della fonte di un prodotto senza un
// componente della RFQ è il suo (non «senza STEP»); con lo STEP del prodotto in «Altri file» la conferma della fonte non
// è ammessa, e il motivo dice di riattivarlo; scartato, di ricaricarlo (DEC-B B-3, OB-10 la parte del motore).
func TestFonteDelProdottoIMotivi(t *testing.T) {
	r := maRisultatoACME()
	p := &Pagina{Prodotto: maP1}
	m := &propostabom.ModelloDelProdotto{}
	if f := fonteDelProdotto(r, p, nil, m); f.Motivo != MotivoFonteSenzaComponente || f.ConfermaAmmessa {
		t.Errorf("senza componente: %+v", f)
	}
	// il componente 12 della scena non ha uno STEP autorizzato: senza gruppi con uno STEP, perché l'unico è in Altri file
	a := maUUID(99)
	pid := maUUID(12)
	m = &propostabom.ModelloDelProdotto{StepNonAttivi: []uuid.UUID{a}}
	r.V1 = DatiV1{NonAttivi: propostabom.NonAttivi{a: "altri_file"}}
	if f := fonteDelProdotto(r, p, &pid, m); f.Motivo != propostabom.TestoStepInAltriFile || f.ConfermaAmmessa {
		t.Errorf("lo STEP non attivo fuori dai gruppi: %+v", f)
	}
	// scartato: «ricaricalo», mai «riattivalo» (DEC-B B-3; Q20 [U])
	r.V1.NonAttivi[a] = "scartato"
	if f := fonteDelProdotto(r, p, &pid, m); f.Motivo != propostabom.TestoStepScartato || strings.Contains(f.Motivo, "riattiv") {
		t.Errorf("lo STEP scartato fuori dai gruppi: %+v", f)
	}
	// lo STEP della bozza aperta del prodotto, non confermato e in Altri file
	r.V1.NonAttivi[a] = "altri_file"
	r.V1.Bozze = map[uuid.UUID]BozzaAperta{pid: {BozzaID: maUUID(70), FonteAllegato: &a, FonteSha256: maSha(9)}}
	if f := fonteDelProdotto(r, p, &pid, m); f.Motivo != propostabom.TestoStepInAltriFile || f.ConfermaAmmessa || f.Confermata {
		t.Errorf("lo STEP della bozza non attivo: %+v", f)
	}
	r.V1.NonAttivi[a] = "scartato"
	if f := fonteDelProdotto(r, p, &pid, m); f.Motivo != propostabom.TestoStepScartato || f.ConfermaAmmessa {
		t.Errorf("lo STEP della bozza scartato: %+v", f)
	}
}

// maDatiContratto: la forma del JSON di /dati con le sole chiavi del contratto §2.4 e, riscritta per V1, quelle di EM
// §9.9 con i nomi di PE-01, PE-15 e PE-16 (A-28). Le forme di gruppi[], fonte e bozza_del_prodotto le controlla anche
// TestDatiV1INomiDeiCampi, sui tag.
type maDatiContratto struct {
	ThreadID string `json:"thread_id"`
	Vista    string `json:"vista"`
	Prodotto string `json:"prodotto"`
	Testa    struct {
		Cliente            string `json:"cliente"`
		Oggetto            string `json:"oggetto"`
		RiferimentoCliente string `json:"riferimento_cliente"`
		StatoThread        string `json:"stato_thread"`
		Fase               *struct {
			Nome      string    `json:"nome"`
			Etichetta string    `json:"etichetta"`
			Inizio    time.Time `json:"inizio"`
			GgInFase  int       `json:"gg_in_fase"`
			Semaforo  string    `json:"semaforo"`
			Terminale bool      `json:"terminale"`
		} `json:"fase"`
	} `json:"testa"`
	Esito     *valutazione.EsitoThread `json:"esito"`
	Confronto *confronto.Esito         `json:"confronto"`
	Errore    string                   `json:"errore"`
	Calcolato time.Time                `json:"calcolato"`

	// V1 (EM §9.9)
	Gruppi []struct {
		Chiave            string  `json:"chiave"`
		Tipo              string  `json:"tipo"`
		AllegatoID        *string `json:"allegato_id"`
		Radice            string  `json:"radice"`
		NomeFile          string  `json:"nome_file"`
		ImprontaContenuto string  `json:"impronta_contenuto"`
		RadiceLetterale   string  `json:"radice_letterale"`
		Compatibilita     string  `json:"compatibilita"`
		Struttura         *struct {
			Versione int    `json:"versione"`
			Completa bool   `json:"completa"`
			Motivo   string `json:"motivo"`
		} `json:"struttura"`
		Segnalati int `json:"segnalati"`
		// i tre nomi in più, per la proposta di emendamento della corsia motore (resoconto dell'ondata B, §5, B-3)
		Importazione *struct {
			Codice string `json:"codice"`
			Motivo string `json:"motivo"`
			Testo  string `json:"testo"`
		} `json:"importazione"`
		Diagnostiche json.RawMessage   `json:"diagnostiche"`
		Nodi         []json.RawMessage `json:"nodi"`
	} `json:"gruppi"`
	MotivoSenzaGruppi *struct {
		Codice   string   `json:"codice"`
		Testo    string   `json:"testo"`
		Allegati []string `json:"allegati"`
	} `json:"motivo_senza_gruppi"`
	Fonte *struct {
		Voce            *string `json:"voce"`
		DocumentoID     *string `json:"documento_id"`
		Sha256          string  `json:"sha256"`
		ConfermaAmmessa bool    `json:"conferma_ammessa"`
		Motivo          string  `json:"motivo"`
		Confermata      bool    `json:"confermata"`
		Autorizzata     bool    `json:"autorizzata"`
		Sorgente        string  `json:"sorgente"`
		StepSecondario  *struct {
			AllegatoID string `json:"allegato_id"`
			NomeFile   string `json:"nome_file"`
			Motivo     string `json:"motivo"`
		} `json:"step_secondario"`
		// ondata C (U2 §2.4)
		ConfermataIl      *time.Time     `json:"confermata_il"`
		ConfermataDa      *maPersonaJSON `json:"confermata_da"`
		AutorizzataIl     *time.Time     `json:"autorizzata_il"`
		AutorizzataDa     *maPersonaJSON `json:"autorizzata_da"`
		AutorizzataMotivo string         `json:"autorizzata_motivo"`
		// direttiva 39
		Gesto              string `json:"gesto"`
		SorgenteConfermata bool   `json:"sorgente_confermata"`
		SorgenteS8         *struct {
			AllegatoID   string     `json:"allegato_id"`
			Sha256       string     `json:"sha256"`
			DocumentoID  *string    `json:"documento_id"`
			Valida       bool       `json:"valida"`
			Motivo       string     `json:"motivo"`
			ConfermataIl *time.Time `json:"confermata_il"`
			ConfermataDa *string    `json:"confermata_da"`
		} `json:"sorgente_s8"`
		LegameStorico *struct {
			DocumentoID string  `json:"documento_id"`
			Sha256      string  `json:"sha256"`
			AllegatoID  *string `json:"allegato_id"`
		} `json:"legame_storico"`
		StrutturaSTEP *struct {
			StatoAnalisi        string `json:"stato_analisi"`
			Struttura           string `json:"struttura"`
			Gruppo              string `json:"gruppo"`
			ImprontaContenuto   string `json:"impronta_contenuto"`
			FigliEstratti       int    `json:"figli_estratti"`
			Partenza            string `json:"partenza"`
			PresaDAttoRichiesta bool   `json:"presa_d_atto_richiesta"`
			Testo               string `json:"testo"`
		} `json:"struttura_step"`
		// giro G39 (G39-02 [U]): le proposte di sorgente del Motore A
		ProposteSorgente []struct {
			AllegatoID     string `json:"allegato_id"`
			NomeFile       string `json:"nome_file"`
			CodiceProdotto string `json:"codice_prodotto"`
			Evidenze       []struct {
				Lettura        string `json:"lettura"`
				Selettore      string `json:"selettore"`
				CodiceLetto    string `json:"codice_letto"`
				BaseLetta      string `json:"base_letta"`
				MarcatoreLetto string `json:"marcatore_letto"`
				EsitoBase      string `json:"esito_base"`
				EsitoMarcatore string `json:"esito_marcatore"`
			} `json:"evidenze"`
			SorgenteDi *string `json:"sorgente_di"`
			Testo      string  `json:"testo"`
		} `json:"proposte_sorgente"`
	} `json:"fonte"`
	// ondata C (U2 §5.3)
	ConfermaCorrente *struct {
		ConfermaID     string        `json:"conferma_id"`
		Numero         int           `json:"numero"`
		Valida         bool          `json:"valida"`
		ConfermataIl   time.Time     `json:"confermata_il"`
		ConfermataDa   maPersonaJSON `json:"confermata_da"`
		VerificaAdesso struct {
			Stato      string   `json:"stato"`
			Verificata *bool    `json:"verificata"`
			Criterio   string   `json:"criterio"`
			Motivi     []string `json:"motivi"`
			Variazioni []string `json:"variazioni"`
		} `json:"verifica_adesso"`
		PropostaAggiornata     json.RawMessage `json:"proposta_aggiornata"`
		PropostaNonConfrontata string          `json:"proposta_non_confrontata"`
	} `json:"conferma_corrente"`
	F8Stato          string `json:"f8_stato"`
	BozzaDelProdotto *struct {
		BozzaID        string `json:"bozza_id"`
		Versione       int    `json:"versione"`
		GruppoCorrente struct {
			Chiave     string  `json:"chiave"`
			Come       string  `json:"come"`
			AllegatoID *string `json:"allegato_id"`
			Motivo     string  `json:"motivo"`
		} `json:"gruppo_corrente"`
	} `json:"bozza_del_prodotto"`
}

// TestDatiJSONNelContratto (riscritta per V1; A-28): il JSON di /dati della scena ACME, con le letture di V1 (F8 in attesa,
// la bozza aperta formato 3 del prodotto con un registro), ha le chiavi del contratto A1d più quelle di EM §9.9, e torna
// con DisallowUnknownFields nella forma scritta qui: un campo in più o con un altro nome fa fallire la prova, che è anche
// la prova di contratto di modelloDa della pagina.
func TestDatiJSONNelContratto(t *testing.T) {
	r := maRisultatoACME()
	a31 := maUUID(31)
	r.V1 = DatiV1{F8: F8InAttesa, Bozze: map[uuid.UUID]BozzaAperta{maUUID(10): {BozzaID: maUUID(70), Versione: 2, FonteAllegato: &a31,
		FonteSha256: maSha(1), FonteGruppo: "bom", ConRegistro: true, RegistroGruppo: "bom", FonteRadice: maNodo(1, "#1")}}}
	p := ComponiPagina(r, RichiestaPagina{Thread: maUUID(1), Scrive: true})
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal(b, &chiavi); err != nil {
		t.Fatal(err)
	}
	var nomi []string
	for k := range chiavi {
		nomi = append(nomi, k)
	}
	slices.Sort(nomi)
	if !slices.Equal(nomi, []string{"bozza_del_prodotto", "calcolato", "confronto", "errore", "esito", "f8_stato", "fonte", "gruppi", "prodotto",
		"testa", "thread_id", "vista"}) {
		t.Errorf("chiavi del JSON %q", nomi)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var d maDatiContratto
	if err := dec.Decode(&d); err != nil {
		t.Fatalf("il JSON di /dati non sta nella forma del contratto: %v", err)
	}
	if d.ThreadID != maUUID(1).String() || d.Vista != VistaDistinta || d.Prodotto != maP1 || d.Errore != "" ||
		d.Testa.Fase == nil || d.Testa.Fase.Nome != "FATTIBILITA" || d.Esito == nil || d.Confronto == nil {
		t.Errorf("dati %+v", d)
	}
	if len(d.Gruppi) == 0 || d.Gruppi[0].Chiave != "bom" || d.F8Stato != F8InAttesa || d.Fonte == nil || d.BozzaDelProdotto == nil ||
		d.BozzaDelProdotto.BozzaID != maUUID(70).String() || d.BozzaDelProdotto.Versione != 2 || d.BozzaDelProdotto.GruppoCorrente.Come == "" {
		t.Errorf("le aggiunte di V1: gruppi %+v, f8 %q, fonte %+v, bozza %+v", d.Gruppi, d.F8Stato, d.Fonte, d.BozzaDelProdotto)
	}
	// senza F8 e senza bozza le due chiavi non ci sono; i gruppi del prodotto sì
	q := ComponiPagina(maRisultatoACME(), RichiestaPagina{Thread: maUUID(1), Scrive: true})
	b, _ = json.Marshal(q)
	if bytes.Contains(b, []byte(`"f8_stato"`)) || bytes.Contains(b, []byte(`"bozza_del_prodotto"`)) || !bytes.Contains(b, []byte(`"gruppi":[`)) {
		t.Errorf("senza F8 e senza bozza: %.400s", b)
	}
}

// TestDatiV1INomiDeiCampi (A-28; EM §9.9; PE-01, PE-10, PE-15, PE-16, PE-28): i nomi JSON dei tipi che /dati scrive per V1,
// sui tag. gruppi[] ha i dieci nomi di PE-01 più «importazione», «diagnostiche» e «nodi» (proposta di emendamento della
// corsia motore: EM non dice dove stanno i campi per nodo di PE-10 e PE-28, né il rifiuto dell'adattatore, né le
// diagnostiche di più nodi di MAT PN-07); un nodo ha completezza {requisito, esito, deroga?}, diagnostiche[] e
// decisione_esistente con identita_registrata {emittente, emittente_presunto, namespace?, codice, rev?} (più il modo e
// la certificazione superata); motivo_senza_gruppi ha in più gli allegati (step_non_attivo); fonte e step_secondario i
// nomi di PE-01 e PE-15, senza l'allegato e il nodo, che restano per chi chiama.
func TestDatiV1INomiDeiCampi(t *testing.T) {
	nomi := func(v any) []string {
		var out []string
		tipo := reflect.TypeOf(v)
		for i := range tipo.NumField() {
			if n, _, _ := strings.Cut(tipo.Field(i).Tag.Get("json"), ","); n != "-" {
				out = append(out, n)
			}
		}
		return out
	}
	for _, c := range []struct {
		v      any
		atteso []string
	}{
		{propostabom.GruppoDelModello{}, []string{"chiave", "tipo", "allegato_id", "radice", "nome_file", "impronta_contenuto", "radice_letterale",
			"compatibilita", "struttura", "segnalati", "importazione", "diagnostiche", "nodi"}},
		{propostabom.StrutturaDelGruppo{}, []string{"versione", "completa", "motivo"}},
		{propostabom.ErroreImportazione{}, []string{"codice", "motivo", "testo"}},
		{propostabom.NodoDelModello{}, []string{"rif", "voce", "completezza", "diagnostiche", "decisione_esistente", "identificativo_cliente"}},
		{propostabom.IdentificativoCliente{}, []string{"testo", "motivo"}},
		{propostabom.CompletezzaDelNodo{}, []string{"requisito", "esito", "deroga"}},
		{propostabom.DecisioneEsistenteDelModello{}, []string{"componente_id", "identita_registrata", "certificazione_superata", "identificativo_cliente"}},
		{propostabom.IdentitaRegistrata{}, []string{"emittente", "emittente_presunto", "namespace", "codice", "rev", "modo"}},
		{propostabom.MotivoSenzaGruppi{}, []string{"codice", "testo", "allegati"}},
		{propostabom.StepSecondario{}, []string{"allegato_id", "nome_file", "motivo"}},
		{FonteDelProdotto{}, []string{"voce", "documento_id", "sha256", "conferma_ammessa", "motivo", "confermata", "autorizzata", "sorgente", "step_secondario",
			"confermata_il", "confermata_da", "autorizzata_il", "autorizzata_da", "autorizzata_motivo",
			"gesto", "sorgente_confermata", "sorgente_s8", "legame_storico", "struttura_step",
			"proposte_sorgente"}},
		{SorgenteS8DellaFonte{}, []string{"allegato_id", "sha256", "documento_id", "valida", "motivo", "confermata_il", "confermata_da"}},
		{LegameStorico{}, []string{"documento_id", "sha256", "allegato_id"}},
		{propostabom.StrutturaSTEP{}, []string{"stato_analisi", "struttura", "gruppo", "impronta_contenuto", "figli_estratti", "partenza",
			"presa_d_atto_richiesta", "testo"}},
		{Persona{}, []string{"utente_id", "nome"}},
		{ConfermaCorrente{}, []string{"conferma_id", "numero", "valida", "confermata_il", "confermata_da", "verifica_adesso", "proposta_aggiornata",
			"proposta_non_confrontata"}},
		{VerificaAdesso{}, []string{"stato", "verificata", "criterio", "motivi", "variazioni"}},
		{propostabom.PropostaAggiornata{}, []string{"impronta_contenuto", "gruppo", "differenze", "per_tipo", "regole_cambiate"}},
		{propostabom.PerTipo{}, []string{"identita", "revisione", "metadati", "quantita", "relazioni", "gerarchia"}},
		{BozzaDelProdotto{}, []string{"bozza_id", "versione", "gruppo_corrente"}},
		{propostabom.GruppoCorrente{}, []string{"chiave", "come", "allegato_id", "motivo"}},
	} {
		if got := nomi(c.v); !slices.Equal(got, c.atteso) {
			t.Errorf("%T:\n %q\natteso:\n %q", c.v, got, c.atteso)
		}
	}
}
