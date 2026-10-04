package bancoa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// Gli esiti di un caso degli attesi (par.4.7.4). Un parziale, un riservato o un rimandato non è mai un passato.
const (
	CasoPassato   = "passato"   // tutte le chiavi controllate passano, nessuna rimandata
	CasoParziale  = "parziale"  // le chiavi controllate passano, ma alcune sono rimandate (decadute o non verificabili)
	CasoFallito   = "fallito"   // almeno una chiave non passa, o il caso non si può eseguire
	CasoRiservato = "riservato" // stato_atteso riservato, o un predicato su una forma riservata
	CasoRimandato = "rimandato" // nessuna chiave si controlla
)

// chiavePrecondizioni: il nome con cui le precondizioni di un caso entrano nell'elenco delle chiavi del rapporto
// (par.4.7.5: «precondizioni», attributo della revisione in campo separato, A1b). Non è una chiave di atteso:
// la tabella delle chiavi non la conosce.
const chiavePrecondizioni = "precondizioni"

// EsitoCaso: come è andato un caso, con ciò che serve a capirlo senza rileggere gli attesi: per ogni chiave
// l'atteso e l'ottenuto, le letture con famiglia, forma e funzione, gli attributi, le forme riservate sul
// selettore del caso, le diagnostiche di Interpreta. L'ID e il profilo sono dati privati: stanno solo nei
// rapporti del banco.
type EsitoCaso struct {
	ID                    string                   `json:"id"`
	Profilo               string                   `json:"profilo"`
	Selettore             string                   `json:"selettore"`
	Esito                 string                   `json:"esito"`
	Motivo                string                   `json:"motivo,omitempty"`
	DipendeDa             string                   `json:"dipende_da,omitempty"` // un caso definito con dipende_da si valuta, e il rapporto lo annota
	Chiavi                []EsitoChiave            `json:"chiavi,omitempty"`
	Letture               []LetturaRapporto        `json:"letture,omitempty"`
	Attributi             []motorea.AttributoLetto `json:"attributi,omitempty"`               // da A1b.11: gli attributi di Interpreta
	RiservateSulSelettore []string                 `json:"riservate_sul_selettore,omitempty"` // «famiglia/forma»
	Diagnostiche          []evidenze.Diagnostica   `json:"diagnostiche,omitempty"`
}

// EsitoChiave: una chiave dell'atteso. Stato: passata | fallita | rimandata | riservata.
type EsitoChiave struct {
	Chiave   string `json:"chiave"`
	Sessione string `json:"sessione"` // A1a | A1b | decaduta
	Stato    string `json:"stato"`
	Atteso   string `json:"atteso"`
	Ottenuto string `json:"ottenuto,omitempty"`
	Motivo   string `json:"motivo,omitempty"`
}

// LetturaRapporto: una lettura come la scrive il rapporto: la regola che l'ha prodotta (famiglia e forma), la
// funzione che le ha dato il router (da A1b.11), che cosa ha letto e dove.
type LetturaRapporto struct {
	Famiglia  string `json:"famiglia"`
	Forma     string `json:"forma"`
	Funzione  string `json:"funzione,omitempty"`
	Originale string `json:"originale"`
	Base      string `json:"base"`
	Revisione string `json:"revisione,omitempty"`
	Stato     string `json:"stato"`
	Inizio    int    `json:"inizio"`
	Fine      int    `json:"fine"`
}

// preparato: un caso con il suo selettore, il suo motore e la sua interpretazione, prima di valutarne le
// chiavi.
type preparato struct {
	caso    CasoContratto
	sel     evidenze.Selettore
	motore  *motorea.Motore
	cliente uuid.UUID
	interp  motorea.Interpretazione
	diag    []evidenze.Diagnostica
	errore  string
}

// EseguiCasiContratto: ogni caso sul motore del suo profilo. Da A1b.11 il testo del caso diventa un documento di
// una sola unità con estrazione.DaTesto, e motorea.Interpreta lo legge con l'uso sconosciuto del documento
// (5.4.5, «L'uso dei segmenti nei casi di contratto»; R52 A): nessuna selezione viene inventata, quindi su
// oggetto e corpo la riga 2 del router dà «richiesta» con l'incertezza nella lettura, sul testo del PDF
// «menzione», sui campi della revisione «attributo». Le chiavi si leggono dall'Interpretazione divisa per
// funzione (traduzione.go; R25 a). Esiti: passato, parziale, fallito, riservato, rimandato. Un parziale, un
// riservato o un rimandato non è mai un passato.
//
// Il profilo si lega al cliente con i profili del manifest (D-09), poi al motore per UUID: nel codice nessun
// nome di profilo o di cliente. Un profilo senza cliente, un cliente senza motore (scartato o assente
// dall'indice), un contesto che non si legge o un testo che DaTesto o Interpreta rifiutano fanno fallire il
// caso, con il motivo; così un'interpretazione parziale (un limite superato), anche se le chiavi tornano. Un
// caso con stato_atteso riservato non si valuta: le sue letture restano nel rapporto come informazione. Le precondizioni di un caso si verificano (verificaPrecondizioni) e stanno fra le chiavi
// del rapporto. Gli esiti seguono l'ordine dei casi negli attesi.
func EseguiCasiContratto(a Attesi, r motorea.InsiemeRegole, profili map[string]uuid.UUID) []EsitoCaso {
	pp := make([]preparato, 0, len(a.Casi))
	for _, c := range a.Casi {
		pp = append(pp, prepara(c, r, profili))
	}
	out := make([]EsitoCaso, 0, len(pp))
	for i, p := range pp {
		out = append(out, valutaCaso(p, pp, i))
	}
	return out
}

func prepara(c CasoContratto, r motorea.InsiemeRegole, profili map[string]uuid.UUID) preparato {
	p := preparato{caso: c}
	sel, err := TraduciContesto(c.Contesto)
	if err != nil {
		p.errore = fmt.Sprintf("il contesto %q non si legge come selettore (R19)", c.Contesto)
		return p
	}
	p.sel = sel
	cliente, ok := profili[c.Profilo]
	if !ok {
		p.errore = "il profilo non è legato a un cliente nei profili del manifest (D-09)"
		return p
	}
	p.cliente = cliente
	m := r.Motori[cliente]
	if m == nil {
		p.diag = append(p.diag, r.Scartati[cliente]...)
		if _, scartato := r.Scartati[cliente]; scartato {
			p.errore = "la grammatica del cliente è scartata: il caso non si può eseguire"
		} else {
			p.errore = "il cliente del profilo non ha una voce nell'indice delle regole"
		}
		return p
	}
	p.motore = m
	p.interp, p.errore = interpreta(m, sel, c.Testo)
	p.diag = p.interp.Diagnostiche
	return p
}

// interpreta: il documento di una sola unità di DaTesto (5.4.5) letto da Interpreta con l'uso sconosciuto del
// documento: per il modo casi nessuna selezione dei segmenti viene inventata. Un errore di DaTesto (testo non
// UTF-8) o di Interpreta (contratto, motore usato male) torna come motivo, con le diagnostiche.
func interpreta(m *motorea.Motore, sel evidenze.Selettore, testo string) (motorea.Interpretazione, string) {
	doc, err := estrazione.DaTesto(sel, testo)
	if err != nil {
		return motorea.Interpretazione{}, "il testo non diventa un documento (DaTesto): " + testoDiagnostiche(err)
	}
	in, err := m.Interpreta(doc, evidenze.UsoSconosciuto(doc.BundleID))
	if err != nil {
		return motorea.Interpretazione{}, "Interpreta non legge il documento del caso: " + testoDiagnostiche(err)
	}
	return in, ""
}

func valutaCaso(p preparato, tutti []preparato, i int) EsitoCaso {
	c := p.caso
	e := EsitoCaso{ID: c.ID, Profilo: c.Profilo, Selettore: c.Contesto, DipendeDa: c.DipendeDa, Diagnostiche: p.diag}
	if p.errore == "" {
		e.Selettore = p.sel.String()
		e.Letture = rapportoLetture(p.interp.Letture)
		e.Attributi = p.interp.Attributi
		e.RiservateSulSelettore = riservateSul(p.motore, p.sel)
	}
	if p.errore != "" {
		e.Esito, e.Motivo = CasoFallito, p.errore
		for _, k := range c.Atteso {
			e.Chiavi = append(e.Chiavi, EsitoChiave{Chiave: k.Chiave, Sessione: SessioneChiave(k.Chiave), Stato: ChiaveFallita,
				Atteso: k.Valore.String(), Motivo: "caso non eseguito"})
		}
		return e
	}
	if c.StatoAtteso == StatoAttesoRiservato {
		e.Esito, e.Motivo = CasoRiservato, "stato_atteso riservato negli attesi: non si valuta; le letture sono un'informazione"
		for _, k := range c.Atteso {
			e.Chiavi = append(e.Chiavi, EsitoChiave{Chiave: k.Chiave, Sessione: SessioneChiave(k.Chiave), Stato: ChiaveRiservata,
				Atteso: k.Valore.String()})
		}
		return e
	}

	sc := nuovaScena(p.sel, c.Testo, p.interp, p.motore, p.cliente)
	for _, k := range c.Atteso {
		sc.atteso[k.Chiave] = k.Valore
	}
	for j, q := range tutti {
		if j != i && q.errore == "" && q.caso.Profilo == c.Profilo && q.sel == p.sel {
			identita, _ := dividiLetture(q.interp.Letture)
			sc.formeAltri = append(sc.formeAltri, formeDi(identita))
		}
	}

	for _, k := range c.Atteso {
		regola := tabellaChiavi[k.Chiave]
		ek := EsitoChiave{Chiave: k.Chiave, Sessione: regola.sessione, Atteso: k.Valore.String()}
		if regola.valuta == nil {
			ek.Stato, ek.Motivo = ChiaveRimandata, regola.motivo
		} else {
			v := regola.valuta(sc, k.Valore)
			ek.Stato, ek.Ottenuto, ek.Motivo = v.stato, v.ottenuto, v.motivo
		}
		e.Chiavi = append(e.Chiavi, ek)
	}
	if c.Precondizioni != nil {
		e.Chiavi = append(e.Chiavi, verificaPrecondizioni(p, sc))
	}

	// passate conta solo le chiavi dell'atteso: le precondizioni da sole non fanno passare un caso che non
	// controlla niente (mai un passato per vuoto).
	passate, fallite, rimandate, riservate := 0, 0, 0, 0
	for _, ek := range e.Chiavi {
		switch ek.Stato {
		case ChiavePassata:
			if ek.Chiave != chiavePrecondizioni {
				passate++
			}
		case ChiaveFallita:
			fallite++
		case ChiaveRimandata:
			rimandate++
		case ChiaveRiservata:
			riservate++
		}
	}
	switch {
	case fallite > 0:
		e.Esito = CasoFallito
	case riservate > 0:
		e.Esito, e.Motivo = CasoRiservato, "un predicato su una forma riservata non si decide"
	case passate == 0:
		e.Esito = CasoRimandato
		if len(c.Atteso) == 0 {
			e.Motivo = "l'atteso non ha chiavi"
		} else {
			e.Motivo = "nessuna chiave si controlla"
		}
	case rimandate > 0:
		e.Esito, e.Motivo = CasoParziale, "alcune chiavi sono rimandate"
	default:
		e.Esito = CasoPassato
	}
	// Un'interpretazione parziale (un limite superato, 5.4.6 punto 16) non è una base per giudicare: le chiavi che
	// passano sul vuoto (letture_identita 0, fallback_generico_non_promuove…) passerebbero per un taglio, non per
	// il testo (A1b-22: mai un successo vuoto). Le chiavi restano nel rapporto come informazione.
	if p.interp.Stato == motorea.StatoInterpretazioneParziale {
		e.Esito, e.Motivo = CasoFallito, "interpretazione parziale (vedi le diagnostiche): un risultato tagliato non si giudica (5.4.6 punto 16; A1b-22)"
	}
	return e
}

// verificaPrecondizioni: le precondizioni di un caso sull'attributo della revisione in campo separato
// (par.4.7.5; A-C02). DaTesto fa una sola unità, quindi l'entità del caso non ha letture di codice, e Interpreta
// applica «l'unica regola attiva sul selettore» (5.4.6 punto 12). Il banco non costruisce l'entità condivisa:
// verifica che il risultato sarebbe lo stesso. Provenance: la precondizione degli attesi dichiara che il campo e
// il codice stanno nella stessa entità; regola: la RegolaRevisione della famiglia che legge quel codice.
//   - entita_condivisa_con_codice falso: niente da verificare, DaTesto non mette codici nell'entità (passata);
//   - vero: il codice base_strutturata, letto da solo sul campo del codice della stessa entità (cartiglio.codice,
//     o l'id dello STEP) con DaTesto e Interpreta, dà le famiglie delle sue letture d'identità con quella base.
//     Ogni revisione del caso deve venire da una regola di una di quelle famiglie (l'ID dell'attributo): allora
//     passata; una revisione senza regola, o di un'altra famiglia, fallita (nell'entità vera la regola sarebbe
//     un'altra). Se la base non si legge da sola sul campo del codice la precondizione non si costruisce con
//     DaTesto: rimandata, con il motivo, mai passata.
func verificaPrecondizioni(p preparato, sc *scena) EsitoChiave {
	pc := p.caso.Precondizioni
	ek := EsitoChiave{Chiave: chiavePrecondizioni, Sessione: SessioneA1b,
		Atteso: "base_strutturata " + pc.BaseStrutturata + ", entita_condivisa_con_codice " + strconv.FormatBool(pc.EntitaCondivisaConCodice)}
	if !pc.EntitaCondivisaConCodice {
		ek.Stato, ek.Ottenuto = ChiavePassata, "nessun codice nell'entità, come nel documento di DaTesto"
		return ek
	}
	campo, ok := campoDelCodice(p.sel)
	if !ok {
		ek.Stato, ek.Motivo = ChiaveFallita, fmt.Sprintf("il selettore %q non ha un campo del codice nella stessa entità", p.sel.String())
		return ek
	}
	if pc.BaseStrutturata == "" {
		ek.Stato, ek.Motivo = ChiaveFallita, "entità condivisa con il codice senza base_strutturata: precondizione incompleta"
		return ek
	}
	in, errore := interpreta(p.motore, campo, pc.BaseStrutturata)
	if errore != "" {
		ek.Stato, ek.Motivo = ChiaveFallita, errore
		return ek
	}
	var famiglie []string
	for _, l := range in.Letture {
		if funzioneDIdentita(l.Funzione) && l.Forma.Base.Normalizzata == pc.BaseStrutturata && !dentro(l.Forma.Famiglia, famiglie) {
			famiglie = append(famiglie, l.Forma.Famiglia)
		}
	}
	if len(famiglie) == 0 {
		ek.Stato, ek.Motivo = ChiaveRimandata, fmt.Sprintf("la base della precondizione non si legge da sola su %s: con DaTesto l'entità condivisa non si costruisce", campo.String())
		return ek
	}
	if len(sc.attributi) == 0 {
		ek.Stato, ek.Ottenuto, ek.Motivo = ChiaveFallita, "codice letto da "+elenco(unici(famiglie)), "nessuna revisione in campo separato nel caso"
		return ek
	}
	var regole []string
	ok = true
	for _, a := range sc.attributi {
		fam, reg, conRegola := regolaDellAttributo(a)
		if !conRegola {
			regole = append(regole, a.Stato+" senza regola")
			ok = false
			continue
		}
		regole = append(regole, fam+"/"+reg)
		ok = ok && dentro(fam, famiglie)
	}
	ek.Ottenuto = "codice letto da " + elenco(unici(famiglie)) + "; revisione da " + strings.Join(unici(regole), ", ")
	if ok {
		ek.Stato = ChiavePassata
	} else {
		ek.Stato, ek.Motivo = ChiaveFallita, "la revisione non viene da una regola della famiglia del codice dell'entità (5.4.6 punto 12)"
	}
	return ek
}

// campoDelCodice: il campo del codice nella stessa entità del selettore: cartiglio.codice per il cartiglio (il
// disegno), l'id per un nodo o una radice STEP.
func campoDelCodice(s evidenze.Selettore) (evidenze.Selettore, bool) {
	campo := ""
	switch s.Contesto {
	case evidenze.ContestoCartiglio:
		campo = "codice"
	case evidenze.ContestoRadiceSTEP, evidenze.ContestoNodoSTEP:
		campo = "id"
	default:
		return evidenze.Selettore{}, false
	}
	sel, err := evidenze.LeggiSelettore(string(s.Contesto) + "." + campo)
	return sel, err == nil
}

func rapportoLetture(ls []motorea.LetturaCodice) []LetturaRapporto {
	var out []LetturaRapporto
	for _, lc := range ls {
		l := lc.Forma
		lr := LetturaRapporto{Famiglia: l.Famiglia, Forma: l.Forma, Funzione: string(lc.Funzione), Originale: l.Originale,
			Base: l.Base.Normalizzata, Stato: l.Stato, Inizio: l.Intervallo.Inizio, Fine: l.Intervallo.Fine}
		if l.Revisione != nil {
			lr.Revisione = l.Revisione.Originale
		}
		out = append(out, lr)
	}
	return out
}

// riservateSul: le forme della grammatica del profilo dichiarate sul selettore che non entrano nel motore
// (riservate, proiezioni riservate, selettore riservato): la «riserva» che può spiegare un caso senza letture.
func riservateSul(m *motorea.Motore, sel evidenze.Selettore) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, f := range m.Snapshot().Grammatica.Famiglie {
		for _, fo := range f.Forme {
			for _, s := range fo.Selettori {
				if x, err := evidenze.LeggiSelettore(s); err == nil && x == sel && !formaAttiva(fo, f.Base) {
					out = append(out, f.ID+"/"+fo.ID)
					break
				}
			}
		}
	}
	return out
}

// ConteggiCasi: quanti casi per esito. La somma è il numero dei casi letti.
type ConteggiCasi struct {
	Totale    int `json:"totale"`
	Passati   int `json:"passati"`
	Parziali  int `json:"parziali"`
	Falliti   int `json:"falliti"`
	Riservati int `json:"riservati"`
	Rimandati int `json:"rimandati"`
}

// Conta: i conteggi di un elenco di esiti.
func Conta(es []EsitoCaso) ConteggiCasi {
	n := ConteggiCasi{Totale: len(es)}
	for _, e := range es {
		switch e.Esito {
		case CasoPassato:
			n.Passati++
		case CasoParziale:
			n.Parziali++
		case CasoFallito:
			n.Falliti++
		case CasoRiservato:
			n.Riservati++
		case CasoRimandato:
			n.Rimandati++
		}
	}
	return n
}
